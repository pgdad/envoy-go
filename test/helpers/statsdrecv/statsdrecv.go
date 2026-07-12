// Package statsdrecv provides a minimal in-process statsd UDP receiver for tests
// and differential fixtures. It is the statsd counterpart of the metricsservice
// helper (the gRPC MetricsService receiver): a driver-owned receiver that the
// proxy WRITES UDP datagrams to, so per project convention it is a test helper
// rather than a runner BackendKind (reference_differential_grpc_receiver_driver_owned;
// BackendKind STAYS 38).
package statsdrecv

import (
	"bufio"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Server reads statsd/DogStatsd/graphite line-protocol UDP datagrams
// (<prefix>.<name>[;k1=v1;k2=v2...]:<value>|<type>[|#tag1:val1,...]) and
// accumulates, per TAG-STRIPPED wire name: the running SUM of every |c
// (counter) value (DeltaSum — under the always-delta statsd counter model the
// per-flush deltas sum to the cumulative total, == K after K requests), the
// last-seen |g (gauge) absolute value (Gauge), and the datagram COUNT
// (SeenCount). Because the sink emits exactly one datagram per metric name
// per flush, SeenCount(name) is the number of flushes that included name —
// the delta stability-barrier signal (an idle counter's 0-delta |c line still
// increments SeenCount).
//
// A DogStatsd sink's tag-hoisting (stats.ExtractTags — SN1/SN2 hoist e.g. a
// cluster/HCM-prefix identifier OUT of the name and into a "|#k:v,..." tag
// suffix) and a graphite sink's tag-folding (the SAME hoisted tags instead
// embedded IN the name as ";k=v" pairs, e.g.
// "grpfx.cluster.upstream_rq_total;envoy.cluster_name=b:7|c", ADR-0275) both
// mean a single WIRE NAME can legitimately carry MULTIPLE, DISTINCT
// tag-variants — e.g. a bootstrap's admin interface has its OWN internal
// http_conn_manager stats under stat_prefix "admin", which collapses to the
// SAME residual wire name (e.g. "http.downstream_rq_total") as a test
// listener's stat_prefix, differing only by the envoy.http_conn_manager_prefix
// tag VALUE. A plain per-name SUM (DeltaSum) or per-name last-seen tag set
// (Tags) is therefore ambiguous/cross-contaminated whenever more than one
// tag-variant shares a name (e.g. the admin interface's own startup
// readiness-probe traffic bleeds into a test listener's counter of the same
// wire name). DeltaSumTagged disambiguates by requiring an EXACT tag-set
// match, accumulating a SEPARATE running sum per (name, tag-signature) pair.
// Both tag grammars feed this SAME machinery, keyed by the tag-stripped name.
type Server struct {
	conn *net.UDPConn

	lis   net.Listener // TCP mode: nil in UDP mode
	conns []net.Conn   // TCP mode: accepted conns, hard-closed by Close

	mu                 sync.RWMutex
	deltaSums          map[string]float64            // |c running sum per name (ALL tag-variants combined)
	sumsByTags         map[string]map[string]float64 // |c running sum per name, keyed further by tagSignature — disambiguates same-name/different-tag lines (e.g. admin vs test-listener HCM instances)
	gauges             map[string]float64            // |g last-seen per name
	seen               map[string]int                // datagram count per name
	tags               map[string]map[string]string  // last-seen |# tag set per name
	maxLinesInDatagram int                           // Server-WIDE: the largest nlines observed in any ingested datagram
	linesInDatagram    map[string]int                // last-seen per name: how many total lines were in the datagram that most recently carried this name

	connCount int // TCP mode: total accepted connections (the long-lived-conn proof)
	unparsed  int // lines the receiver could not account (parse failure or unknown type)

	// closed guards the acceptLoop/Close TOCTOU: lis.Accept() can return a conn
	// that was in flight the instant Close() ran (Close closes the listener,
	// THEN snapshots+nils conns under mu). Without this flag, acceptLoop could
	// append that conn to s.conns AFTER Close's snapshot, so Close would never
	// close it and its streamLoop goroutine would block in Read() forever.
	// Close sets closed=true in the SAME critical section where it snapshots
	// and nils conns; acceptLoop re-checks it under mu right after Accept and,
	// if set, closes the just-accepted conn itself instead of registering it.
	// Reset() must NOT touch this — it is a one-way lifecycle latch, not a
	// value accumulator.
	closed bool

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
		conn:            conn,
		deltaSums:       make(map[string]float64),
		sumsByTags:      make(map[string]map[string]float64),
		gauges:          make(map[string]float64),
		seen:            make(map[string]int),
		tags:            make(map[string]map[string]string),
		linesInDatagram: make(map[string]int),
	}
	go s.readLoop()
	return s, nil
}

// NewTCPAtAddr binds a TCP listener on the caller-supplied host:port (e.g.
// "0.0.0.0:<port>" so a Docker reference-Envoy can reach the host, or
// "127.0.0.1:0" for an ephemeral loopback port) and starts an accept loop.
// Lifecycle is the caller's responsibility via Close.
//
// Unlike the UDP receiver this is a STREAM parser: a TCP read carries no line
// framing (the reference's ~200 KB post-reconnect write arrived in <=65536-byte
// chunks that split lines mid-token, SPEC-55 §11.5), so each connection is read
// line-at-a-time through bufio with a carried remainder, and an INCOMPLETE
// TRAILING LINE AT EOF IS DISCARDED — the property the sink's line-aligned
// resume relies on to avoid duplication (SPEC-55 §3.5).
func NewTCPAtAddr(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: listen tcp %q: %w", addr, err)
	}
	s := &Server{
		lis:             lis,
		deltaSums:       make(map[string]float64),
		sumsByTags:      make(map[string]map[string]float64),
		gauges:          make(map[string]float64),
		seen:            make(map[string]int),
		tags:            make(map[string]map[string]string),
		linesInDatagram: make(map[string]int),
	}
	go s.acceptLoop()
	return s, nil
}

// acceptLoop accepts until the listener is closed, counting every connection and
// serving each on its own stream goroutine.
//
// Accept() can race Close(): Close closes the listener first, then takes mu to
// snapshot+nil conns. A conn that was already returned by Accept() but not yet
// registered here can therefore land AFTER Close's snapshot. The closed check
// (taken under the same mu Close uses) closes such a conn itself and never
// registers it or starts its streamLoop, so no conn or goroutine is orphaned.
// connCount only counts conns this receiver actually accepted-and-served, so a
// conn rejected here (post-Close) does not increment it.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.lis.Accept()
		if err != nil {
			return // listener closed
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.connCount++
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.streamLoop(conn)
	}
}

// streamLoop reads complete, '\n'-terminated lines. bufio.Reader.ReadString
// accumulates across as many underlying Reads as it takes to find the delimiter,
// so a chunk boundary that lands mid-token is transparently rejoined.
//
// A non-nil error from ReadString means NO delimiter was found: whatever bytes
// it returns are an INCOMPLETE TRAILING LINE, and they are DISCARDED. That is
// exactly what a real statsd server does at EOF, and it is what makes the sink's
// re-send of the straddling line non-duplicating.
func (s *Server) streamLoop(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return // EOF / reset: `line` holds an unterminated remainder — DISCARD it
		}
		s.mu.Lock()
		s.ingestLine(line) // TrimSpace inside strips the '\n' (and any '\r')
		s.mu.Unlock()
	}
}

// ConnCount returns the total number of TCP connections accepted over this
// receiver's lifetime. 0 for a UDP receiver (connectionless). The statsd TCP
// sink must hold ONE long-lived connection, so a correct subject yields 1; a
// per-flush redial yields one per flush.
func (s *Server) ConnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connCount
}

// UnparsedCount returns the number of ingested lines the receiver could not
// account for: a structural parse failure, or a known-good structure carrying an
// unknown metric type (the reference's |ms timers — legitimate; or a corrupted
// type token produced by \n-SEPARATED rather than \n-TERMINATED framing).
//
// envoy-go emits only |c and |g (no histograms, ADR-0060), so a correct SUBJECT
// yields exactly 0. The reference legitimately yields >0 (|ms), so this is a
// SUBJECT-EXACT signal — never assert it cross-side.
func (s *Server) UnparsedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unparsed
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

// ingestLine parses ONE complete statsd/DogStatsd/graphite line
// (<name>[;k1=v1;k2=v2...]:<value>|<type>[|#tag1:val1,...]) and updates the
// value accumulators. It assumes s.mu is HELD by the caller. It returns
// (name, true) when the line was accounted, and ("", false) when it was not
// — either a structural parse failure or an unknown metric type; both bump
// unparsed (except a blank line, which is neither).
//
// The first-pipe-then-colon split (phase 49): neither name nor value contains
// '|', so the FIRST '|' unambiguously separates "name:value" from "type[|#tags]"
// — a tagged line's tag suffix contains its OWN colons, which a last-colon split
// would mis-take for the name/value boundary. This degenerates to the exact
// prior behavior on a tagless line (line-parser-extension delimiter-reuse gotcha).
//
// Graphite tag-folding (phase 57, ADR-0275): a graphite sink hoists tags INTO
// the name as ";k=v" pairs instead of a dog_statsd "|#" suffix, e.g.
// "grpfx.cluster.upstream_rq_total;envoy.cluster_name=b:7|c". This reuses the
// SAME two delimiters without disturbing either existing split: graphite tags
// precede the value colon, so "head" (everything before the first '|') is
// "name;tags:value" and the LAST-':' split still lands on the value colon
// because a tag VALUE never contains ':' (the reference's tag values here are
// response-code-classes/cluster/HCM-prefix names, and stats.IsValidName
// excludes ':' from envoy-go's own emitted names) — so name ends up holding
// "name;tags" intact, which is then split on ';' below.
func (s *Server) ingestLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false // a blank line is not an unparsed line
	}
	pipe1 := strings.IndexByte(line, '|')
	if pipe1 < 0 {
		s.unparsed++
		return "", false
	}
	head := line[:pipe1] // "name:value" — no '|' precedes here
	colon := strings.LastIndexByte(head, ':')
	if colon < 0 {
		s.unparsed++
		return "", false
	}
	name := head[:colon]
	val, err := strconv.ParseFloat(head[colon+1:], 64)
	if err != nil {
		s.unparsed++
		return "", false
	}
	// Graphite name-embedded tags (phase 57, ADR-0275): "<name>;k1=v1;k2=v2".
	// ADDITIVE by construction: statsd/dog_statsd wire names cannot contain ';'
	// (stats.IsValidName's charset — registry.go NamePattern), so this block is
	// a no-op for every pre-57 line shape (0092/0093/0094/0098 unaffected). The
	// parsed pairs feed the SAME lineTags the '|#' path feeds, so Tags()/
	// DeltaSumTagged()/tagSignature work unchanged on the TAG-STRIPPED name.
	var lineTags map[string]string
	if semi := strings.IndexByte(name, ';'); semi >= 0 {
		tagPart := name[semi+1:]
		name = name[:semi]
		lineTags = make(map[string]string, 2)
		for _, pair := range strings.Split(tagPart, ";") {
			eq := strings.IndexByte(pair, '=')
			if eq < 0 {
				s.unparsed++ // a ';'-bearing name whose pair lacks '=' is unaccountable
				return "", false
			}
			lineTags[pair[:eq]] = pair[eq+1:]
		}
	}
	rest := line[pipe1+1:] // "type[|#tag1:val1,...]"
	typ := rest
	if pipe2 := strings.IndexByte(rest, '|'); pipe2 >= 0 {
		typ = rest[:pipe2]
		tagPart := strings.TrimPrefix(rest[pipe2+1:], "#")
		if lineTags == nil {
			lineTags = make(map[string]string)
		}
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
		// An unknown metric type: the reference's |ms timers (legitimate), or a
		// corrupted type token from \n-SEPARATED framing (the break-(b) signal).
		s.unparsed++
		return "", false
	}
	if lineTags != nil {
		s.tags[name] = lineTags
	}
	return name, true
}

// ingest parses each newline-delimited line in one UDP DATAGRAM, where every
// line is complete BY CONSTRUCTION, and additionally records the datagram
// batching signals. The TCP stream path uses ingestLine directly and leaves
// maxLinesInDatagram / linesInDatagram unpopulated (SPEC-55 §3.9).
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for _, line := range lines {
		name, ok := s.ingestLine(line)
		if !ok {
			continue
		}
		if n := len(lines); n > s.maxLinesInDatagram {
			s.maxLinesInDatagram = n
		}
		s.linesInDatagram[name] = len(lines)
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

// MaxLinesInAnyDatagram returns the largest number of lines ever observed in a
// single ingested datagram (Server-wide, not per-name) — the batching-occurred
// proof for a max_bytes_per_datagram differential (phase 50, ADR-0267).
func (s *Server) MaxLinesInAnyDatagram() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxLinesInDatagram
}

// LinesInDatagram returns how many total lines were in the datagram that MOST
// RECENTLY carried name, and ok=false if name was never seen. Lets a
// differential assert a specific line stayed ALONE (== 1) even when other
// lines in the same flush co-batched (phase 50, ADR-0267).
func (s *Server) LinesInDatagram(name string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.linesInDatagram[name]
	return n, ok
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
// precedent). Call only when no datagram/line is in flight. Does NOT reset
// connCount: it is a transport fact over the receiver's lifetime (how many TCP
// connections were ever accepted), not a value accumulator meant to be re-zeroed
// between test phases.
func (s *Server) Reset() {
	s.mu.Lock()
	s.deltaSums = make(map[string]float64)
	s.sumsByTags = make(map[string]map[string]float64)
	s.gauges = make(map[string]float64)
	s.seen = make(map[string]int)
	s.tags = make(map[string]map[string]string)
	s.maxLinesInDatagram = 0
	s.linesInDatagram = make(map[string]int)
	s.unparsed = 0
	s.mu.Unlock()
}

// Addr returns the receiver's bound address, for either transport.
func (s *Server) Addr() string {
	if s.lis != nil {
		return s.lis.Addr().String()
	}
	return s.conn.LocalAddr().String()
}

// Close stops the receiver. For UDP it closes the socket. For TCP it HARD-closes
// the listener and every accepted connection — never waiting for the peer to
// hang up, since the statsd sink holds its connection open for the process
// lifetime (the metricsservice.Close hard-stop precedent). Idempotent.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.lis != nil {
			_ = s.lis.Close()
		}
		s.mu.Lock()
		s.closed = true
		conns := s.conns
		s.conns = nil
		s.mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
}
