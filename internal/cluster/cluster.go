package cluster

import (
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
}

// Addr returns the dial-string form "host:port".
func (e Endpoint) Addr() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// Cluster is a named pool of endpoints with a load-balancing policy. Phase 02
// supports only round-robin; future phases may grow the LB family.
// upstreamCfg is nil for plaintext clusters and non-nil for TLS clusters.
type Cluster struct {
	name           string
	endpoints      []Endpoint
	connectTimeout time.Duration
	lb             loadBalancer
	upstreamCfg    *stdtls.Config
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

// Name returns the cluster's name.
func (c *Cluster) Name() string { return c.name }

// UseH2 reports whether this cluster's HttpProtocolOptions selects HTTP/2
// upstream origination. When true, the HCM filter-build path constructs
// routerActionH2 instead of routerAction (per ADR-0056; phase 05.2 SPEC §5.5).
// Defaults to false for every existing cluster build path; Task 10 wires up
// the actual setter from typed_extension_protocol_options parsing.
func (c *Cluster) UseH2() bool { return c.useH2 }

// PickEndpoint selects the next upstream endpoint per the cluster's LB policy.
// Safe for concurrent use.
func (c *Cluster) PickEndpoint() (Endpoint, error) {
	return c.lb.Pick()
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
func (c *Cluster) Dial(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, nil
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
