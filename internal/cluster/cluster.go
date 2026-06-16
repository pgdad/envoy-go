package cluster

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	// Phase 05.2 (ADR-0016 amendment, no separate ADR per ADR-0016): the
	// cluster-side HttpProtocolOptions extension proto must be registered
	// with protoregistry.GlobalTypes so protojson can round-trip the
	// typed_extension_protocol_options entry in Manager.buildCluster
	// (Task 10). The blank import is the load-it-and-register-it idiom.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"github.com/esalaine/envoy-go/internal/stats"
)

// defaultConnectTimeout is used when a cluster's connect_timeout is unset.
// Matches Envoy v1.37.2's documented default (SPEC §10 #2 settled).
const defaultConnectTimeout = 5 * time.Second

// errNoEndpoints is returned by PickEndpoint when the cluster has no endpoints.
// Build-time validation in NewManager prevents this in normal operation; the
// runtime check exists for defense-in-depth.
var errNoEndpoints = errors.New("cluster: no endpoints")

// Endpoint is a single upstream socket destination.
type Endpoint struct {
	Host string
	Port uint32
	// Metadata is the parsed envoy.lb scalar key→value namespace (the subset
	// dimension). nil when absent. NOT part of the dial identity: Addr() ignores
	// it, so ring_hash/maglev table keys stay "IP:PORT".
	Metadata map[string]SubsetValue
}

// Addr returns the dial-string form "host:port".
func (e Endpoint) Addr() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// PooledH1Conn bundles a pooled HTTP/1.1 upstream connection with its
// bufio.Reader so the next request can resume parsing the response stream
// without losing bytes already buffered (e.g. read-ahead of the next
// response's :status line). The reader is created at first use and reused
// for the lifetime of the pooled connection.
type PooledH1Conn struct {
	Conn net.Conn
	Br   *Bufio // opaque wrapper (cluster owns the bufio.Reader type alias)
	ep   Endpoint
}

// Endpoint returns the upstream endpoint this connection is dialed to.
func (p *PooledH1Conn) Endpoint() Endpoint { return p.ep }

// Bufio is an opaque alias for *bufio.Reader so the cluster package doesn't
// have to expose bufio in its exported API; router/router.go uses the
// helper PooledH1Conn.BufioReader() to retrieve it as a *bufio.Reader.
type Bufio = bufio.Reader

// BufioReader returns the bufio.Reader for resumable response parsing.
func (p *PooledH1Conn) BufioReader() *bufio.Reader { return p.Br }

// h1PoolMaxPerEndpoint caps idle conn count per endpoint. 1024 is generous
// for the high-concurrency benchmark workloads we care about; for typical
// production usage 64-256 is plenty. Bursts above this cap simply drop the
// extra connection rather than queue it.
const h1PoolMaxPerEndpoint = 1024

// Cluster is a named pool of endpoints with a load-balancing policy. Phase 02
// shipped round-robin; phase 34 added least_request (ADR-0233); future phases
// may grow the LB family.
// upstreamCfg is nil for plaintext clusters and non-nil for TLS clusters.
type Cluster struct {
	name           string
	endpoints      []Endpoint
	connectTimeout time.Duration
	lb             loadBalancer
	upstreamCfg    *stdtls.Config

	// h1Pool is a per-endpoint LIFO of idle keep-alive HTTP/1.1 connections.
	// Keyed by endpoint Addr(). AcquireH1 pops one (or dials fresh);
	// PutIdleH1 pushes back at end-of-response if reusable. Capped at
	// h1PoolMaxPerEndpoint per endpoint to avoid unbounded growth.
	h1PoolMu sync.Mutex
	h1Pool   map[string][]*PooledH1Conn
	// useH2 reports whether this cluster's HttpProtocolOptions selects H2
	// upstream origination. Set by Manager.buildCluster (Task 10) from the
	// typed_extension_protocol_options entry; defaults to false for every
	// existing cluster build path. When true, the HCM filter-build path
	// constructs routerActionH2 instead of routerAction (per ADR-0056;
	// phase 05.2 SPEC §5.5).
	useH2 bool
	// 06.1 metric fields (per ADR-0063 — cluster-level only; per-endpoint
	// expansion deferred). All fields are non-nil after Manager.buildCluster
	// completes; all are concurrent-safe (atomic primitives). Allocated by
	// registerClusterMetrics in manager.go at boot, pre-Freeze.
	upstreamRqTotal  *stats.Counter
	upstreamRq2xx    *stats.Counter
	upstreamRq3xx    *stats.Counter
	upstreamRq4xx    *stats.Counter
	upstreamRq5xx    *stats.Counter
	upstreamCxTotal  *stats.Counter
	upstreamCxActive *stats.Gauge
	membershipTotal  *stats.Gauge

	// health is the per-cluster active-HC registry (ADR-0243). nil for a cluster
	// with no health_checks. Built + threaded into the LB constructs in
	// buildCluster; consumed in registerClusterMetrics (stat injection) +
	// StartHealthChecks (runtime).
	health *clusterHealth

	// outlier is the per-cluster passive outlier detector (ADR-0245). nil for a
	// cluster with no outlier_detection. Built in buildCluster over the shared
	// health registry; fed by RecordUpstreamResult (the router success sites are
	// wired in Task 7).
	outlier *outlierDetector

	// checkers are the active-HC background probers (one per configured
	// health_check). nil for a cluster with no health_checks. Built in
	// buildCluster, stat-registered in registerClusterMetrics, and run by
	// Manager.StartHealthChecks (stopped by Manager.Drain / ctx cancel).
	checkers []*healthChecker
}

// statusClassCounter returns the upstream_rq_<Nxx> counter for the given
// HTTP status code per the integer-divide code/100 discipline (Rule SN4 of
// SPEC §10.1). Returns nil for codes outside [200, 599] (1xx informational
// responses are NOT bucketed; status-class counters cover only the response-
// terminating range). Used by Task 10 in actions.go to Inc the right
// counter on response status finalization.
func (c *Cluster) statusClassCounter(code int) *stats.Counter {
	switch code / 100 {
	case 2:
		return c.upstreamRq2xx
	case 3:
		return c.upstreamRq3xx
	case 4:
		return c.upstreamRq4xx
	case 5:
		return c.upstreamRq5xx
	default:
		return nil
	}
}

// IncUpstreamRqTotal Inc's the cluster-scope upstream_rq_total counter once.
// Phase 06.1 Task 11 hot path — called from internal/filter/hcm/actions.go's
// routerAction.do and routerActionH2.doH2 at dispatch entry per SPEC §5.5
// (Increment paths table). Exported so the hcm package can drive it without
// reaching across the package boundary into unexported fields.
func (c *Cluster) IncUpstreamRqTotal() { c.upstreamRqTotal.Inc() }

// IncStatusClass Inc's the cluster-scope upstream_rq_<Nxx> counter for the
// status-class implied by code (integer-divide code/100 per Rule SN4 of
// SPEC §10.1). No-op for codes outside [200, 599] — 1xx informational
// responses are NOT bucketed per SPEC §2.1. Phase 06.1 Task 11 hot path —
// called from the H1 + H2 router actions on response status finalization
// (including the local-reply 502/503 paths) per SPEC §5.5.
func (c *Cluster) IncStatusClass(code int) {
	if cnt := c.statusClassCounter(code); cnt != nil {
		cnt.Inc()
	}
}

// UpstreamResult is one completed upstream request's outcome (SPEC §3.1).
// LocalOriginErr is unread at 40.1 (reserved for 40.2). (ADR-0245)
type UpstreamResult struct {
	StatusCode     int
	LocalOriginErr bool
}

// RecordUpstreamResult feeds one request outcome to the cluster's outlier
// detector. A no-op for clusters without outlier_detection. (ADR-0245)
func (c *Cluster) RecordUpstreamResult(ep Endpoint, r UpstreamResult) {
	if c.outlier == nil {
		return
	}
	c.outlier.record(ep, r.StatusCode)
}

// Name returns the cluster's name.
func (c *Cluster) Name() string { return c.name }

// UseH2 reports whether this cluster's HttpProtocolOptions selects HTTP/2
// upstream origination. When true, the HCM filter-build path constructs
// routerActionH2 instead of routerAction (per ADR-0056; phase 05.2 SPEC §5.5).
// Defaults to false for every existing cluster build path; Task 10 wires up
// the actual setter from typed_extension_protocol_options parsing.
func (c *Cluster) UseH2() bool { return c.useH2 }

// UpstreamTLSConfig returns the per-cluster upstream *tls.Config for TLS
// clusters, or nil for plaintext clusters. The returned pointer is shared with
// the cluster's internal state — callers MUST NOT mutate fields on it; they
// should treat the value as read-only or .Clone() it before mutation.
//
// Phase 22.2 (ADR-0177 IN-PLACE AMENDMENT per SPEC §3.3 + §11.4): the
// internal/httpclient/ ClusterDispatch consumer reads this to wire the per-
// cluster TLS posture into its temporary *http.Client without owning a copy
// of the cluster's TLS state. The cluster manager remains the single source
// of truth for upstream TLS configuration; ClusterDispatch is a read-only
// consumer.
func (c *Cluster) UpstreamTLSConfig() *stdtls.Config { return c.upstreamCfg }

type hashKeyCtxKey struct{}

// WithHashKey returns ctx carrying a request-derived upstream-selection hash for a
// ring_hash cluster to consume at Dial/AcquireH1. Exported (the producers live in
// other packages — tcpproxy, http/router). Non-ring_hash clusters ignore it. An
// additive exported symbol — not a signature change (ADR-0235).
func WithHashKey(ctx context.Context, key uint64) context.Context {
	return context.WithValue(ctx, hashKeyCtxKey{}, key)
}

func hashKeyFrom(ctx context.Context) (uint64, bool) {
	v, ok := ctx.Value(hashKeyCtxKey{}).(uint64)
	return v, ok
}

type subsetMatchCtxKey struct{}

// WithSubsetMatch attaches a resolved route subset match to ctx (the producer
// sets it in the HTTP router; the cluster funnels extract it for subsetLB). The
// exported Cluster surface stays byte-stable (the match rides ctx like the hash
// key). An additive exported symbol — not a signature change (ADR-0239).
func WithSubsetMatch(ctx context.Context, match SubsetMatch) context.Context {
	return context.WithValue(ctx, subsetMatchCtxKey{}, match)
}

func subsetMatchFrom(ctx context.Context) (SubsetMatch, bool) {
	v, ok := ctx.Value(subsetMatchCtxKey{}).(SubsetMatch)
	return v, ok
}

// PickEndpoint selects the next upstream endpoint per the cluster's LB policy.
// Safe for concurrent use. The picked unit is released IMMEDIATELY: direct-pick
// consumers (httpclient ClusterDispatch, the thriftproxy no-healthy-host probe)
// have no observable conn lifecycle, so their load is invisible to least_request
// (a documented coverage note — SPEC §2 / §3.2). Dial / AcquireH1 do NOT route
// through here; they call c.lb.Pick() directly so they can hold the release
// until final conn Close (ADR-0232 OPTION C). The signature stays byte-stable so
// the two direct consumers compile unchanged.
func (c *Cluster) PickEndpoint() (Endpoint, error) {
	ep, release, err := c.lb.Pick(0, false, SubsetMatch{}, false)
	if err != nil {
		return Endpoint{}, err
	}
	release()
	return ep, nil
}

// ConnectTimeout returns the cluster's TCP connect timeout (default 5s if the
// bootstrap left connect_timeout unset).
func (c *Cluster) ConnectTimeout() time.Duration {
	return c.connectTimeout
}

// Dial opens a new connection to an endpoint picked from the cluster's LB
// state. For plaintext clusters it returns the raw TCP connection wrapped in
// a *connWithGauge. For TLS clusters it returns a *connWithGauge wrapping a
// *stdtls.Conn whose HandshakeContext has already completed. connect_timeout
// bounds the TCP dial only; the TLS handshake is bounded by ctx.
//
// On every successful dial (post-TLS-handshake, when configured), Dial Incs
// upstream_cx_total (monotonic counter) AND upstream_cx_active (gauge), and
// wraps the returned conn in a *connWithGauge whose Close() Decs the active
// gauge exactly once via sync.Once (per ADR-0063's Inc-then-wrap discipline;
// SPEC §6 cluster.<n>.upstream_cx_{total,active} semantics). Caller-side
// double-Close is safe (no Dec underflow). DialH2 unwraps the *connWithGauge
// internally to reach the inner *stdtls.Conn for the ALPN check, but passes
// the wrapper to h2.NewClientConn so the *h2.ClientConn.Close path Decs the
// gauge through the wrapper.
func (c *Cluster) Dial(ctx context.Context) (net.Conn, Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, Endpoint{}, err
	}
	// ADR-0232 OPTION C: pick via c.lb.Pick() directly (not PickEndpoint) so we
	// can HOLD the release until the conn's final Close. release is always
	// non-nil; it must fire on every error path after a successful pick.
	hk, ok := hashKeyFrom(ctx)
	match, hasMatch := subsetMatchFrom(ctx)
	ep, release, err := c.lb.Pick(hk, ok, match, hasMatch)
	if err != nil {
		return nil, Endpoint{}, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		release()
		return nil, Endpoint{}, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			release()
			return nil, Endpoint{}, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	// Compose release into the existing connWithGauge dec closure. The
	// connWithGauge sync.Once guards BOTH the gauge Dec and the LB release, so
	// double-Close cannot double-release. The struct is unchanged.
	return &connWithGauge{Conn: final, dec: func() { c.upstreamCxActive.Dec(); release() }}, ep, nil
}

// AcquireH1 returns an HTTP/1.1 upstream connection ready to write a request
// on. It first tries to pop an idle keep-alive connection from the per-
// endpoint pool; on miss it dials fresh via the same code path as Dial. The
// returned *PooledH1Conn carries both the net.Conn and a bufio.Reader for
// resumable response parsing across consecutive requests on the same conn.
//
// The caller MUST either:
//   - call PutIdleH1 after a successful response read with keep-alive
//     semantics (response not Close, body fully drained), to return it to
//     the pool for reuse; OR
//   - call (*PooledH1Conn).Conn.Close() on any failure path, to drop the
//     connection (the Dial-time *connWithGauge wrapper decrements the
//     upstream_cx_active gauge exactly once via sync.Once).
//
// AcquireH1 is the high-throughput replacement for Dial in the router H1
// upstream-dispatch hot path. Dial remains for non-pooled use sites (TCP
// proxy, single-shot upstream calls).
func (c *Cluster) AcquireH1(ctx context.Context) (*PooledH1Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// ADR-0232 OPTION C: pick via c.lb.Pick() directly so we can HOLD the
	// release until the fresh-dialed conn's final Close. release is always
	// non-nil. On a pool HIT the fresh pick is redundant and is released
	// immediately; on a MISS it composes into the connWithGauge dec closure.
	hk, ok := hashKeyFrom(ctx)
	match, hasMatch := subsetMatchFrom(ctx)
	ep, release, err := c.lb.Pick(hk, ok, match, hasMatch)
	if err != nil {
		return nil, err
	}
	addr := ep.Addr()

	// Fast path: pop an idle pooled conn for this endpoint.
	c.h1PoolMu.Lock()
	list := c.h1Pool[addr]
	n := len(list)
	if n > 0 {
		p := list[n-1]
		c.h1Pool[addr] = list[:n-1]
		c.h1PoolMu.Unlock()
		// Clear any stale deadline left by the prior request before handing
		// back to the caller (the caller will reset its own deadline anyway,
		// but this defends against the SetDeadline-after-Close pattern).
		_ = p.Conn.SetDeadline(time.Time{})
		// Pool HIT: the pooled conn carries its DIAL-TIME hold (cx-as-rq — it
		// persists until final close, incl. PutIdleH1-overflow drop and
		// closePool drain). The fresh pick is redundant → release it
		// immediately. The pooled conn carries its dial-time ep, so use that.
		release()
		return p, nil
	}
	c.h1PoolMu.Unlock()

	// Slow path: dial fresh (mirrors the Dial code path verbatim so the
	// connWithGauge / TLS handshake / counters semantics stay aligned —
	// release-on-failure + dec composition).
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		release()
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			release()
			return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	wrapped := &connWithGauge{Conn: final, dec: func() { c.upstreamCxActive.Dec(); release() }}
	return &PooledH1Conn{
		Conn: wrapped,
		Br:   bufio.NewReaderSize(wrapped, 4096),
		ep:   ep,
	}, nil
}

// PutIdleH1 returns a keep-alive HTTP/1.1 connection to the per-endpoint
// idle pool for reuse by the next AcquireH1 call. The caller MUST have
// fully drained the response body before calling this (otherwise the next
// reader would see stale bytes from the previous response).
//
// If the pool for the endpoint is full (>= h1PoolMaxPerEndpoint) the
// connection is closed instead. Best-effort; never blocks.
func (c *Cluster) PutIdleH1(p *PooledH1Conn) {
	if p == nil || p.Conn == nil {
		return
	}
	addr := p.ep.Addr()
	c.h1PoolMu.Lock()
	if c.h1Pool == nil {
		c.h1Pool = make(map[string][]*PooledH1Conn)
	}
	list := c.h1Pool[addr]
	if len(list) >= h1PoolMaxPerEndpoint {
		c.h1PoolMu.Unlock()
		_ = p.Conn.Close()
		return
	}
	// Clear deadlines while idle so a stale per-request deadline doesn't fire
	// while the conn sits in the pool waiting for the next caller.
	_ = p.Conn.SetDeadline(time.Time{})
	c.h1Pool[addr] = append(list, p)
	c.h1PoolMu.Unlock()
}

// connWithGauge wraps a net.Conn so its Close decrements an upstream-cx-active
// gauge exactly once (sync.Once-guarded) regardless of how many Close calls
// the wrapper sees. Per the settled SPEC §12 deferred-decision #10 the type
// lives in cluster.go (one-file rule) rather than its own file, because the
// wrapper has no consumers outside (*Cluster).Dial / DialH2. The embedded
// `Conn net.Conn` is anonymous so non-Close I/O (Read/Write/SetDeadline/etc.)
// forwards to the underlying conn automatically; only Close is overridden.
type connWithGauge struct {
	net.Conn
	dec  func()
	once sync.Once
}

// Close decrements the cluster's upstream-cx-active gauge exactly once and
// then closes the underlying conn. The sync.Once guard defends against
// double-Close from layered callers (e.g., Go's net/http stack closing a
// response body whose RoundTrip already closed the conn) — the active gauge
// MUST never go negative from a paired Inc, even if Close is invoked twice.
func (c *connWithGauge) Close() error {
	c.once.Do(c.dec)
	return c.Conn.Close()
}

// closePool closes the per-cluster connection-pool resources at drain time.
// Best-effort; no error return; idempotent.
//
// 08.2 lands this as a forward-extensible hook. The exact set of pooled
// resources to close evolves with each upstream-protocol family:
//   - HTTP/1.1 keep-alive idle conns (no exported pool field today; phase 02
//     dials per-request without keep-alive pooling — the future operator-
//     affordances phase may add a pool, in which case closePool grows to
//     drain it).
//   - HTTP/2 ClientConn instances from phase 05.2 (no exported close hook
//     today; the future operator-affordances phase may add one).
//   - TLS upstream connections from phase 03 (covered by the H1.1/H2 pool
//     close above when those land; tls.Conn instances are inside).
//
// Today, closePool is a stub with a debug log. The cm.Drain() call from
// cmd/envoy-go/main.go (post-rendezvous, before the deferred-stop chain
// runs) provides the architectural call-site for future expansion per
// SPEC §2.1 deferral note. Per planner-time decision 6.
func (c *Cluster) closePool() {
	// Drain the per-endpoint HTTP/1.1 idle conn pool. Each pooled conn's
	// Close decrements upstream_cx_active via the connWithGauge wrapper
	// (sync.Once-guarded; safe).
	c.h1PoolMu.Lock()
	pool := c.h1Pool
	c.h1Pool = nil
	c.h1PoolMu.Unlock()
	for _, list := range pool {
		for _, p := range list {
			if p != nil && p.Conn != nil {
				_ = p.Conn.Close()
			}
		}
	}
}

// CloseWrite delegates the half-close to the underlying *net.TCPConn or
// *stdtls.Conn (whichever the wrapper is carrying). tcpproxy's halfClose
// half-closes the write side after each io.Copy direction completes; before
// the connWithGauge wrapper landed (Task 9) tcpproxy did this via a
// type-switch on the concrete conn type returned by Cluster.Dial. Now that
// Dial returns *connWithGauge, the half-close needs an interface-shaped
// shim — this method satisfies the `interface{ CloseWrite() error }` shape
// tcpproxy uses post-Task-9. Returns nil if the underlying conn is neither
// a *net.TCPConn nor a *stdtls.Conn (no-op for unsupported transports).
func (c *connWithGauge) CloseWrite() error {
	switch t := c.Conn.(type) {
	case *net.TCPConn:
		return t.CloseWrite()
	case *stdtls.Conn:
		return t.CloseWrite()
	}
	return nil
}
