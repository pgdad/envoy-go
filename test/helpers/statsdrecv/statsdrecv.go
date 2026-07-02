// Package statsdrecv provides a minimal in-process statsd UDP receiver for tests
// and differential fixtures. It is the statsd counterpart of the metricsservice
// helper (the gRPC MetricsService receiver): a driver-owned receiver that the
// proxy WRITES UDP datagrams to, so per project convention it is a test helper
// rather than a runner BackendKind (reference_differential_grpc_receiver_driver_owned;
// BackendKind STAYS 38).
package statsdrecv

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Server reads statsd/DogStatsd line-protocol UDP datagrams
// (<prefix>.<name>:<value>|<type>[|#tag1:val1,...]) and accumulates, per full
// wire name: the running SUM of every |c (counter) value (DeltaSum — under
// the always-delta statsd counter model the per-flush deltas sum to the
// cumulative total, == K after K requests), the last-seen |g (gauge) absolute
// value (Gauge), and the datagram COUNT (SeenCount). Because the sink emits
// exactly one datagram per metric name per flush, SeenCount(name) is the
// number of flushes that included name — the delta stability-barrier signal
// (an idle counter's 0-delta |c line still increments SeenCount).
//
// A DogStatsd sink's tag-hoisting (stats.ExtractTags — SN1/SN2 hoist e.g. a
// cluster/HCM-prefix identifier OUT of the name and into a tag) means a single
// WIRE NAME can legitimately carry MULTIPLE, DISTINCT tag-variants — e.g. a
// bootstrap's admin interface has its OWN internal http_conn_manager stats
// under stat_prefix "admin", which collapses to the SAME residual wire name
// (e.g. "http.downstream_rq_total") as a test listener's stat_prefix, differing
// only by the envoy.http_conn_manager_prefix tag VALUE. A plain per-name SUM
// (DeltaSum) or per-name last-seen tag set (Tags) is therefore ambiguous/
// cross-contaminated whenever more than one tag-variant shares a name (e.g.
// the admin interface's own startup readiness-probe traffic bleeds into a
// test listener's counter of the same wire name). DeltaSumTagged disambiguates
// by requiring an EXACT tag-set match, accumulating a SEPARATE running sum per
// (name, tag-signature) pair.
type Server struct {
	conn *net.UDPConn

	mu         sync.RWMutex
	deltaSums  map[string]float64            // |c running sum per name (ALL tag-variants combined)
	sumsByTags map[string]map[string]float64 // |c running sum per name, keyed further by tagSignature — disambiguates same-name/different-tag lines (e.g. admin vs test-listener HCM instances)
	gauges     map[string]float64            // |g last-seen per name
	seen       map[string]int                // datagram count per name
	tags       map[string]map[string]string  // last-seen |# tag set per name

	closeOnce sync.Once
}

// tagSignature builds a canonical, order-independent string key for a tag map
// (sorted "k1=v1,k2=v2"; "" for a nil/empty map — a tagless line) so distinct
// tag-variants of the same wire name accumulate into separate DeltaSumTagged
// buckets.
func tagSignature(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(tags[k])
	}
	return b.String()
}

// NewAtAddr binds a UDP listener on the caller-supplied host:port (e.g.
// "0.0.0.0:<port>" so a Docker reference-Envoy can write to the host, or
// "127.0.0.1:0" for an ephemeral loopback port) and starts a reader goroutine.
// Lifecycle is the caller's responsibility via Close (the metricsservice.NewAtAddr
// precedent — no t.Cleanup).
func NewAtAddr(addr string) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: resolve %q: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: listen %q: %w", addr, err)
	}
	s := &Server{
		conn:       conn,
		deltaSums:  make(map[string]float64),
		sumsByTags: make(map[string]map[string]float64),
		gauges:     make(map[string]float64),
		seen:       make(map[string]int),
		tags:       make(map[string]map[string]string),
	}
	go s.readLoop()
	return s, nil
}

// readLoop reads datagrams until the conn is closed (ReadFromUDP returns an error),
// ingesting each. A 64 KiB buffer comfortably holds one statsd line (max ~77 bytes
// observed; §11).
func (s *Server) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return // conn closed
		}
		s.ingest(buf[:n])
	}
}

// ingest parses each newline-delimited DogStatsd/statsd line in one datagram
// (<name>:<value>|<type>[|#tag1:val1,...]) and updates the accumulators.
// Malformed lines/segments are skipped, never fatal. REVISED (phase 49) from a
// last-colon split (correct only for a tagless statsd line) to a first-pipe-
// then-colon split: neither name nor value contains '|', so the FIRST '|'
// unambiguously separates "name:value" from "type[|#tags]" — a tagged line's
// tag suffix contains its OWN colons, which the old last-colon split mis-took
// for the name/value boundary. This degenerates to the EXACT prior behavior on
// a tagless line (line-parser-extension delimiter-reuse gotcha).
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pipe1 := strings.IndexByte(line, '|')
		if pipe1 < 0 {
			continue
		}
		head := line[:pipe1] // "name:value" — no '|' precedes here
		colon := strings.LastIndexByte(head, ':')
		if colon < 0 {
			continue
		}
		name := head[:colon]
		val, err := strconv.ParseFloat(head[colon+1:], 64)
		if err != nil {
			continue
		}
		rest := line[pipe1+1:] // "type[|#tag1:val1,...]"
		typ := rest
		var lineTags map[string]string
		if pipe2 := strings.IndexByte(rest, '|'); pipe2 >= 0 {
			typ = rest[:pipe2]
			tagPart := strings.TrimPrefix(rest[pipe2+1:], "#")
			lineTags = make(map[string]string)
			for _, pair := range strings.Split(tagPart, ",") {
				if c := strings.IndexByte(pair, ':'); c >= 0 {
					lineTags[pair[:c]] = pair[c+1:]
				}
			}
		}
		switch typ {
		case "c":
			s.deltaSums[name] += val
			sig := tagSignature(lineTags)
			byTag, ok := s.sumsByTags[name]
			if !ok {
				byTag = make(map[string]float64)
				s.sumsByTags[name] = byTag
			}
			byTag[sig] += val
			s.seen[name]++
		case "g":
			s.gauges[name] = val
			s.seen[name]++
		default:
			continue
		}
		if lineTags != nil {
			s.tags[name] = lineTags
		}
	}
}

// Tags returns the last-seen tag set for name (from its most recent |# suffix),
// and ok=false if name was never seen with a tag suffix (a tagless line, or no
// datagram for name at all).
func (s *Server) Tags(name string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tags[name]
	return t, ok
}

// DeltaSum returns the running SUM of every |c value received for name, and
// ok=false if none was received. NOTE: this combines ALL tag-variants sharing
// the name — see DeltaSumTagged when the name is ambiguous across variants
// (e.g. a wire name also emitted by the admin interface's own HCM instance).
func (s *Server) DeltaSum(name string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok = s.deltaSums[name]
	return
}

// DeltaSumTagged returns the running SUM of |c values received for name WHOSE
// tag suffix EXACTLY matches tags (order-independent), and ok=false if no
// datagram for (name, tags) was ever received. This disambiguates multiple
// tag-variants sharing the same wire name — e.g. a bootstrap's admin
// interface has its own internal http_conn_manager stats (stat_prefix
// "admin") that collapse, via stats.ExtractTags's SN2 hoist, to the SAME
// residual wire name as a test listener's own stat_prefix (differing only in
// the envoy.http_conn_manager_prefix tag VALUE); DeltaSum alone cannot tell
// the two apart, but DeltaSumTagged(name, wantTags) only accumulates lines
// whose tag set matches wantTags exactly, so an admin-interface line (or a
// production bug that drops/mismatches tags) contributes ZERO to the wrong
// bucket rather than silently inflating it.
func (s *Server) DeltaSumTagged(name string, tags map[string]string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byTag, present := s.sumsByTags[name]
	if !present {
		return 0, false
	}
	sum, ok = byTag[tagSignature(tags)]
	return
}

// Gauge returns the last-seen |g absolute value for name, and ok=false if none.
func (s *Server) Gauge(name string) (value float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok = s.gauges[name]
	return
}

// SeenCount returns the number of datagrams received for name (== flushes that
// included it). The delta stability-barrier signal.
func (s *Server) SeenCount(name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seen[name]
}

// Reset drops all accumulators (per-side separation, the metricsservice.Reset
// precedent). Call only when no datagram is in flight.
func (s *Server) Reset() {
	s.mu.Lock()
	s.deltaSums = make(map[string]float64)
	s.sumsByTags = make(map[string]map[string]float64)
	s.gauges = make(map[string]float64)
	s.seen = make(map[string]int)
	s.tags = make(map[string]map[string]string)
	s.mu.Unlock()
}

// Addr returns the bound host:port (load-bearing when NewAtAddr allocated an
// ephemeral port).
func (s *Server) Addr() string {
	return s.conn.LocalAddr().String()
}

// Close closes the UDP socket (unblocking the reader goroutine). Idempotent via
// sync.Once. UDP is connectionless — there is no GracefulStop-vs-hard-stop
// distinction (contrast the metricsservice gRPC receiver).
func (s *Server) Close() {
	s.closeOnce.Do(func() { _ = s.conn.Close() })
}
