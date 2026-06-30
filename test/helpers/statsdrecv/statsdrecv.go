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
	"strconv"
	"strings"
	"sync"
)

// Server reads statsd line-protocol UDP datagrams (<prefix>.<name>:<value>|<type>)
// and accumulates, per full dotted name: the running SUM of every |c (counter)
// value (DeltaSum — under the always-delta statsd counter model the per-flush
// deltas sum to the cumulative total, == K after K requests), the last-seen |g
// (gauge) absolute value (Gauge), and the datagram COUNT (SeenCount). Because the
// sink emits exactly one datagram per metric name per flush, SeenCount(name) is
// the number of flushes that included name — the delta stability-barrier signal
// (an idle counter's 0-delta |c line still increments SeenCount). Goroutine-safe
// via an RWMutex (the reader goroutine writes; the poll/assert surface reads)
// under the -race detector.
type Server struct {
	conn *net.UDPConn

	mu        sync.RWMutex
	deltaSums map[string]float64 // |c running sum per name
	gauges    map[string]float64 // |g last-seen per name
	seen      map[string]int     // datagram count per name

	closeOnce sync.Once
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
		conn:      conn,
		deltaSums: make(map[string]float64),
		gauges:    make(map[string]float64),
		seen:      make(map[string]int),
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

// ingest parses each newline-delimited statsd line in one datagram (the reference
// emits one line per datagram — §11 — but split-on-newline is robust to batching)
// and updates the accumulators. Malformed lines are skipped.
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.LastIndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := line[:colon]
		rest := line[colon+1:]
		pipe := strings.IndexByte(rest, '|')
		if pipe < 0 {
			continue
		}
		val, err := strconv.ParseFloat(rest[:pipe], 64)
		if err != nil {
			continue
		}
		typ := rest[pipe+1:]
		switch typ {
		case "c":
			s.deltaSums[name] += val
			s.seen[name]++
		case "g":
			s.gauges[name] = val
			s.seen[name]++
		}
	}
}

// DeltaSum returns the running SUM of every |c value received for name, and
// ok=false if none was received.
func (s *Server) DeltaSum(name string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok = s.deltaSums[name]
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
	s.gauges = make(map[string]float64)
	s.seen = make(map[string]int)
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
