package cluster

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	// Phase 05.2 (ADR-0016 amendment, no separate ADR per ADR-0016): the
	// cluster-side HttpProtocolOptions extension proto must be registered
	// with protoregistry.GlobalTypes so protojson can round-trip the
	// typed_extension_protocol_options entry in Manager.buildCluster
	// (Task 10). The blank import is the load-it-and-register-it idiom.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// defaultConnectTimeout is used when a cluster's connect_timeout is unset.
// Matches Envoy v1.37.2's documented default (SPEC §10 #2 settled).
const defaultConnectTimeout = 5 * time.Second

// errNoEndpoints is returned by PickEndpoint when the cluster has no endpoints.
// Build-time validation in NewManager prevents this in normal operation; the
// runtime check exists for defense-in-depth.
var errNoEndpoints = errors.New("cluster: no endpoints")

// LocalityID is the proto-free (region, zone, sub_zone) identity captured from
// the owning LocalityLbEndpoints.locality (phase 52). The zero value is a
// valid single group — an endpoint whose group carried no locality at all.
// Comparable (usable as a map key — newLocalityWeightedLB groups endpoints by
// this identity, locality.go, Task 3).
type LocalityID struct {
	Region, Zone, SubZone string
}

// Endpoint is a single upstream socket destination.
type Endpoint struct {
	Host string
	Port uint32
	// Metadata is the parsed envoy.lb scalar key→value namespace (the subset
	// dimension, phase 38). nil when absent. NOT part of the dial identity:
	// Addr() ignores it, so ring_hash/maglev table keys stay "IP:PORT".
	Metadata map[string]SubsetValue
	// Locality is the (region, zone, sub_zone) identity captured from the
	// owning LocalityLbEndpoints group (phase 52; the SECOND per-endpoint
	// dimension after Metadata). Only consulted by localityWeightedLB — every
	// other LB construct ignores it. NOT part of the dial identity.
	Locality LocalityID
	// LocalityWeight is the RAW load_balancing_weight captured from the owning
	// group (0 when the field was unset on that group — AMEND-LW2, NO
	// default-to-1 substitution: the reference assigns an omitted-weight
	// locality ZERO load whenever a sibling locality has an explicit weight).
	// Only consulted by localityWeightedLB.
	LocalityWeight uint32
	// Priority is the tier number captured from the owning LocalityLbEndpoints
	// group's priority field (phase 53; the THIRD per-endpoint dimension after
	// Metadata and Locality/LocalityWeight). Unlike LocalityWeight/
	// overprovisioning_factor, priority is a PLAIN uint32 proto field (no
	// wrapper type) — 0 is both the explicit-zero AND the omitted value,
	// meaning "highest-priority tier" (AMEND-P3). Only consulted by
	// priorityLB. NOT part of the dial identity.
	Priority uint32

	// filterMetadata retains the endpoint's RAW per-namespace static metadata
	// (lb_endpoints[].metadata.filter_metadata), ALIASING the already-parsed
	// proto map — zero new allocation. Added phase 72 so a HOST-kind tracing
	// custom_tag can address ANY namespace and walk a NESTED path; the phase-38
	// Metadata projection above stays the envoy.lb scalars-only subset-LB
	// dimension and is BYTE-UNCHANGED. NOT part of the dial identity: Addr()
	// ignores it.
	filterMetadata map[string]*structpb.Struct

	// clusterFilterMetadata retains the OWNING CLUSTER's RAW per-namespace static
	// metadata (clusters[].metadata.filter_metadata), ALIASING the already-parsed
	// proto map — zero new allocation. Added phase 73 so a CLUSTER-kind tracing
	// custom_tag can address ANY namespace and walk a NESTED path. It rides the
	// ENDPOINT rather than cluster.Cluster because the reference gates a
	// CLUSTER-kind tag on the PICKED UPSTREAM HOST, not on the matched route
	// (probed at the phase-73 SPEC). Every endpoint of one cluster shares the
	// SAME map. NOT part of the dial identity: Addr() ignores it.
	clusterFilterMetadata map[string]*structpb.Struct

	// addr is the Addr() string precomputed at extractEndpoints time (the pick
	// hot path — health scans, pool/hash keys — calls Addr() per host; caching
	// removes the per-call formatting allocation). Empty for directly-constructed
	// Endpoints (tests, zero values): Addr() falls back to computing it.
	addr string
}

// Addr returns the dial-string form "host:port" (IPv6 hosts bracketed:
// "[::1]:80" — net.JoinHostPort is byte-identical to the previous
// fmt.Sprintf("%s:%d") form for IPv4/hostname hosts, and produces the
// dialable/reference form for IPv6, which the Sprintf form got wrong).
func (e Endpoint) Addr() string {
	if e.addr != "" {
		return e.addr
	}
	return net.JoinHostPort(e.Host, strconv.FormatUint(uint64(e.Port), 10))
}

// IsZero reports whether the Endpoint is the zero value (no host picked).
// Endpoint is NOT comparable (it carries a Metadata map), so the "a host was
// picked" guard at the connect-failure seam sites uses !ep.IsZero() rather
// than ep != Endpoint{}.
func (e Endpoint) IsZero() bool { return e.Host == "" && e.Port == 0 }

// MetaLookup returns the endpoint's static metadata for the namespace ns,
// wrapped as a structpb StructValue (or (nil,false) when the endpoint carries
// no metadata / the namespace is absent). The HOST analog of the HTTP filter
// chain's RouteMetaLookup (internal/filter/http/chain.go); threaded as a
// method value at the three tracing emit sites. Safe on the ZERO Endpoint
// (nil map → (nil,false)).
func (e Endpoint) MetaLookup(ns string) (*structpb.Value, bool) {
	st := e.filterMetadata[ns]
	if st == nil {
		return nil, false
	}
	return structpb.NewStructValue(st), true
}

// ClusterMetaLookup returns the OWNING CLUSTER's static metadata for the
// namespace ns, wrapped as a structpb StructValue (or (nil,false) when the
// cluster carries no metadata / the namespace is absent). The CLUSTER analog of
// MetaLookup above; threaded as a method value at the three tracing emit sites.
// Safe on the ZERO Endpoint (nil map → (nil,false)).
func (e Endpoint) ClusterMetaLookup(ns string) (*structpb.Value, bool) {
	st := e.clusterFilterMetadata[ns]
	if st == nil {
		return nil, false
	}
	return structpb.NewStructValue(st), true
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

// h2DefaultMaxConcurrentStreams is the per-conn stream cap applied to an H2-upstream
// cluster when http2_protocol_options.max_concurrent_streams is absent or 0. Effectively
// unbounded (the reference's default is ≈2^31-1 — D-H2-DEFAULT/AMEND-H2-3) so the pool
// multiplexes onto a single connection unless the cap is configured small.
const h2DefaultMaxConcurrentStreams int64 = 1 << 30

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
	// h2MaxConcurrentStreams is the per-connection outbound stream budget for an
	// H2-upstream cluster — the cluster's OWN http2_protocol_options.max_concurrent_streams
	// (the LOCAL cap; AMEND-H2-1). It drives multi-conn growth in the h2Pool: a new
	// pooled ClientConn opens only when every existing conn holds this many in-flight
	// streams. Absent/0 ⇒ h2DefaultMaxConcurrentStreams (effectively single-conn
	// multiplex; AMEND-H2-3). Set by buildCluster from extractH2Mode; 0 for non-H2
	// clusters (unread there). (ADR-0253)
	h2MaxConcurrentStreams int64
	// h2Pool is the per-endpoint slice of multiplexed upstream H2 connections
	// (keyed by ep.Addr()); h2Waiters is the SEPARATE per-endpoint stream-aware
	// pending FIFO (woken on stream-completion OR conn-eviction, NOT only on
	// conn-close like connPool.waiters — the crux of SPEC §3.5). ALL of the pool
	// admission state (pooledH2Conn.inFlight, the waiter FIFO, the streams_active
	// gauge) is guarded SOLELY by h2PoolMu (D-H2-MUTEX; single-mutator). nil maps
	// until first use (lazy-init in the lifecycle, Task 5). (phase 43.2a, ADR-0253)
	h2PoolMu  sync.Mutex
	h2Pool    map[string][]*pooledH2Conn
	h2Waiters map[string][]*h2Waiter
	// h2Connecting is the per-endpoint set of IN-FLIGHT dials (Task 9.5,
	// connect-time coalescing). A connecting entry sits between a stream-HIT and a
	// fresh dial: concurrent demand ATTACHES to it (reserving a promised slot)
	// rather than each acquirer dialing its own conn, so K concurrent streams at
	// cap C open exactly ceil(K/C) conns. Guarded SOLELY by h2PoolMu; nil until
	// first use (lazy-init in AcquireH2Stream). (phase 43.2a, ADR-0253)
	h2Connecting map[string][]*connectingH2Conn
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

	// http2StreamsActive is the cluster.<name>.http2.streams_active gauge — the
	// live multiplexed-stream count across the cluster's H2 conns. Declared here
	// so the h2pool nil-guarded helpers compile; nil until Task 6's
	// registerClusterMetrics binds it (useH2-gated). (phase 43.2a, ADR-0253)
	http2StreamsActive *stats.Gauge

	// upstreamCxHTTP2Total is the cluster.<name>.upstream_cx_http2_total counter
	// (++ per new pooled H2 conn dialed by the multiplex pool). Declared here so
	// the Task-5 AcquireH2Stream MISS path can Inc it nil-guarded; nil until
	// Task 6's registerClusterMetrics binds it (useH2-gated; AMEND-H2-2). There
	// is NO upstream_cx_http2_active. (phase 43.2a, ADR-0253)
	upstreamCxHTTP2Total *stats.Counter

	// http2RxReset / http2TxReset are the cluster.<name>.http2.rx_reset /
	// .http2.tx_reset counters — ++ per RST_STREAM the upstream codec RECEIVES /
	// SENDS (the FIRST codec→cluster stat wiring; the pool injects these via
	// h2.WithResetHooks at dial). Registered useH2-gated; nil for non-H2 clusters
	// (the h2pool helpers nil-guard). NO http2.goaway_sent / stream_refused_errors
	// (AMEND-H2B-3). (phase 43.2b, ADR-0254)
	http2RxReset *stats.Counter
	http2TxReset *stats.Counter

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

	// circuitBreaker holds the per-priority max_requests budgets + overflow
	// counters (ADR-0248). nil for a cluster with no circuit_breakers. Built in
	// buildCluster from parseCircuitBreakers; stat handles injected in
	// registerClusterMetrics (Task 4). The enforce seam is wired in Task 5.
	circuitBreaker *circuitBreaker

	// statsReg + statsPrefix are stashed by registerClusterMetrics so the +5
	// retry counters can be registered lazily, on first EnsureRetryStats() —
	// the scoped-registration departure of ADR-0249 (the reference is always-on;
	// we register only for retry-policy clusters so every non-retry fixture's
	// /stats stays byte-stable). retry is nil until EnsureRetryStats() runs.
	statsReg    *stats.Registry
	statsPrefix string
	retry       *retryStats
}

// retryStats are the +6 cluster-scope retry counters (ADR-0249). backoffRL is
// registered-only / emit-0 (no Inc method — there is no ratelimited-backoff
// path in the MVP; it exists for reference-parity name presence). The other
// five are Inc'd from the retry executor (Tasks 6/7).
type retryStats struct {
	rq, success, limitExceeded, backoffExp, backoffRL, perTryTimeout *stats.Counter
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
	c.outlier.record(ep, r.StatusCode, r.LocalOriginErr)
}

// TryAcquireRequest reserves a DEFAULT-priority max_requests slot. Returns false
// (overflow -> caller emits 503) when exhausted. No-op true when no circuit_breakers. (ADR-0248)
func (c *Cluster) TryAcquireRequest() bool {
	if c.circuitBreaker == nil {
		return true
	}
	return c.circuitBreaker.tryAcquire(0) // DEFAULT
}

// ReleaseRequest returns the slot acquired by TryAcquireRequest. No-op when no circuit_breakers.
func (c *Cluster) ReleaseRequest() {
	if c.circuitBreaker != nil {
		c.circuitBreaker.release(0)
	}
}

// acquireConnSlot reserves a connection-creation permit (clusters WITH a connPool).
// Returns nil (no pool, or permit held — caller MUST pair with releaseConnSlot /
// connDec), errConnPoolOverflow (queue full → fail fast 503), or ctx.Err(). (ADR-0252)
func (c *Cluster) acquireConnSlot(ctx context.Context) error {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return nil
	}
	return c.circuitBreaker.pool.acquireConnOrPend(ctx)
}

// tryAcquireConnSlot is the non-blocking permit reserve used by the H2 pool.
// true ⇒ permit held (pair with releaseConnSlot/connDec); false ⇒ at cap.
// No pool (no circuit_breakers) ⇒ always true (unbounded conn growth). (ADR-0253)
func (c *Cluster) tryAcquireConnSlot() bool {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return true
	}
	return c.circuitBreaker.pool.tryAcquireConnSlot()
}

// releaseConnSlot returns a permit acquired by acquireConnSlot. Called on the
// post-acquire dial/handshake FAILURE paths (the success path composes the
// release into connDec). No-op when no pool. (ADR-0252)
func (c *Cluster) releaseConnSlot() {
	if c.circuitBreaker != nil && c.circuitBreaker.pool != nil {
		c.circuitBreaker.pool.releaseConn()
	}
}

// connDec composes upstream_cx_active.Dec() + the LB release() + (for pooled
// clusters) pool.releaseConn() into the single dec closure connWithGauge runs
// exactly once on Close. The pool release on Close is the conn-Close wake seam
// (hands a permit to the head waiter). (ADR-0252)
func (c *Cluster) connDec(release func()) func() {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return func() { c.upstreamCxActive.Dec(); release() }
	}
	pool := c.circuitBreaker.pool
	return func() { c.upstreamCxActive.Dec(); release(); pool.releaseConn() }
}

// TryAcquireRetry reserves a retry_budget slot. Returns false (overflow) when the
// retry budget is exhausted. No-op true when no circuit_breakers / retry_budget. (ADR-0249)
func (c *Cluster) TryAcquireRetry() bool {
	if c.circuitBreaker == nil {
		return true
	}
	return c.circuitBreaker.tryAcquireRetry()
}

// ReleaseRetry returns the slot acquired by TryAcquireRetry. No-op when no circuit_breakers. (ADR-0249)
func (c *Cluster) ReleaseRetry() {
	if c.circuitBreaker != nil {
		c.circuitBreaker.releaseRetry()
	}
}

// EnsureRetryStats registers the +5 retry counters on first call (idempotent —
// config build is single-threaded). Scoped: called only for retry-policy
// clusters (a recorded departure from the reference's always-on). (ADR-0249)
// A nil statsReg means the Cluster was built without registerClusterMetrics
// (e.g. a directly-constructed test cluster) — no registry to register against,
// so this no-ops defensively.
func (c *Cluster) EnsureRetryStats() {
	if c.retry != nil || c.statsReg == nil {
		return
	}
	p := c.statsPrefix
	c.retry = &retryStats{
		rq:            c.statsReg.NewCounter(p + "upstream_rq_retry"),
		success:       c.statsReg.NewCounter(p + "upstream_rq_retry_success"),
		limitExceeded: c.statsReg.NewCounter(p + "upstream_rq_retry_limit_exceeded"),
		backoffExp:    c.statsReg.NewCounter(p + "upstream_rq_retry_backoff_exponential"),
		backoffRL:     c.statsReg.NewCounter(p + "upstream_rq_retry_backoff_ratelimited"), // emit-0
		perTryTimeout: c.statsReg.NewCounter(p + "upstream_rq_per_try_timeout"),
	}
}

// IncUpstreamRqRetry Inc's upstream_rq_retry (one per retry attempt dispatched).
// No-op when EnsureRetryStats has not run (non-retry cluster). Wired in Task 7.
func (c *Cluster) IncUpstreamRqRetry() {
	if c.retry != nil {
		c.retry.rq.Inc()
	}
}

// IncUpstreamRqRetrySuccess Inc's upstream_rq_retry_success (a retry that
// produced a non-retriable / successful response). Wired in Task 7.
func (c *Cluster) IncUpstreamRqRetrySuccess() {
	if c.retry != nil {
		c.retry.success.Inc()
	}
}

// IncUpstreamRqRetryLimitExceeded Inc's upstream_rq_retry_limit_exceeded (the
// num_retries budget was hit before a successful response). Wired in Task 7.
func (c *Cluster) IncUpstreamRqRetryLimitExceeded() {
	if c.retry != nil {
		c.retry.limitExceeded.Inc()
	}
}

// IncUpstreamRqRetryBackoffExponential Inc's upstream_rq_retry_backoff_exponential
// (one per exponential-backoff sleep before a retry attempt). Wired in Task 7.
func (c *Cluster) IncUpstreamRqRetryBackoffExponential() {
	if c.retry != nil {
		c.retry.backoffExp.Inc()
	}
}

// IncUpstreamRqPerTryTimeout Inc's upstream_rq_per_try_timeout (one per attempt
// that hits its per_try_timeout). No-op when EnsureRetryStats has not run
// (non-retry cluster). Wired in the retry executors (Tasks 6/7).
func (c *Cluster) IncUpstreamRqPerTryTimeout() {
	if c.retry != nil {
		c.retry.perTryTimeout.Inc()
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
	// ADR-0252: gate connection CREATION on the max_connections permit (pends in
	// the bounded wait-queue at the cap; errConnPoolOverflow when the queue is full).
	if err := c.acquireConnSlot(ctx); err != nil {
		release()
		return nil, ep, err
	}
	// Dial owns the permit it just acquired → ownPermit=true so dialPicked's
	// failure paths give it back (releaseConnSlot); on success it transfers to
	// the connWithGauge dec closure.
	return c.dialPicked(ctx, ep, release, true /*ownPermit*/)
}

// DialSink dials one endpoint of c for a STATS SINK and returns the bare conn.
//
// Unlike Dial it takes NO max_connections permit (ADR-0252) and increments NO
// upstream_cx_* counter — mirroring the reference, whose statsd TCP connection
// bypasses conn-pool accounting entirely: with max_connections: 0 on the stats
// cluster it still connects and still flushes, and reports upstream_cx_total: 0
// / upstream_cx_active: 0 (SPEC-55 §11.4, AMEND-TCP-CXSTATS). It DOES honor the
// LB pick, connect_timeout, and upstream TLS.
//
// The pick is released IMMEDIATELY (PickEndpoint's contract), so the sink's
// long-lived connection is invisible to least_request — the accepted, already-
// documented cost of every PickEndpoint caller.
//
// STATS SINKS ONLY. Any data-plane caller wanting a connection MUST use Dial:
// bypassing the permit and the gauges anywhere else silently breaks circuit
// breaking and upstream_cx_active.
func (c *Cluster) DialSink(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		return nil, err
	}
	return c.dialAndTLS(ctx, ep)
}

// dialPicked is the SHARED post-pick dial body for Dial and the H2 multiplex
// pool's dialPooledH2To (extracted so the compiler enforces parity —
// D-H2-DIALSHARE). Given an already
// LB-picked ep + its release closure + a permit already held by the caller, it:
// TCP-dials (connect_timeout-bounded) → TLS-handshakes (when configured) →
// Inc's upstream_cx_{total,active} → wraps in connWithGauge{dec: connDec(release)}.
//
// ownPermit remains the single axis of variation for the ACCOUNTED callers
// (Dial, dialPooledH2To — both pass true). The unaccounted stats-sink caller
// (DialSink) does NOT route through here: it needs no counter Inc and no
// connWithGauge wrap, so it shares only the dialAndTLS core.
func (c *Cluster) dialPicked(ctx context.Context, ep Endpoint, release func(), ownPermit bool) (net.Conn, Endpoint, error) {
	final, err := c.dialAndTLS(ctx, ep)
	if err != nil {
		if ownPermit {
			c.releaseConnSlot()
		}
		release()
		return nil, ep, err
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	// Compose release (+ pool permit release for pooled clusters) into the
	// existing connWithGauge dec closure. The connWithGauge sync.Once guards the
	// gauge Dec, the LB release AND the pool release, so double-Close cannot
	// double-release. The struct is unchanged.
	return &connWithGauge{Conn: final, dec: c.connDec(release)}, ep, nil
}

// dialAndTLS is the accounting-FREE dial core shared by dialPicked and DialSink:
// TCP-dial to ep bounded by connect_timeout, then the upstream TLS handshake
// bounded by ctx. It touches NO counter, takes NO permit, and holds NO LB
// release — every one of those is the caller's business. Extracted at phase 55
// so DialSink (the unaccounted stats-sink dial, AMEND-TCP-CXSTATS) cannot drift
// from Dial's dial/TLS semantics.
func (c *Cluster) dialAndTLS(ctx context.Context, ep Endpoint) (net.Conn, error) {
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	if c.upstreamCfg == nil {
		return raw, nil
	}
	conn := stdtls.Client(raw, c.upstreamCfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
	}
	return conn, nil
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
func (c *Cluster) AcquireH1(ctx context.Context) (*PooledH1Conn, Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, Endpoint{}, err
	}
	// ADR-0232 OPTION C: pick via c.lb.Pick() directly so we can HOLD the
	// release until the fresh-dialed conn's final Close. release is always
	// non-nil. On a pool HIT the fresh pick is redundant and is released
	// immediately; on a MISS it composes into the connWithGauge dec closure.
	hk, ok := hashKeyFrom(ctx)
	match, hasMatch := subsetMatchFrom(ctx)
	ep, release, err := c.lb.Pick(hk, ok, match, hasMatch)
	if err != nil {
		return nil, Endpoint{}, err
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
		return p, p.ep, nil
	}
	c.h1PoolMu.Unlock()

	// ADR-0252: pool MISS → gate connection creation on the max_connections permit.
	if err := c.acquireConnSlot(ctx); err != nil {
		release()
		return nil, ep, err
	}

	// Slow path: dial fresh via the SHARED post-pick dial body (dialPicked —
	// the same body Dial and dialPooledH2To use, so the compiler enforces the
	// connWithGauge / TLS handshake / counters / release-on-failure parity).
	conn, _, err := c.dialPicked(ctx, ep, release, true /*ownPermit*/)
	if err != nil {
		return nil, ep, err
	}
	return &PooledH1Conn{
		Conn: conn,
		Br:   bufio.NewReaderSize(conn, 4096),
		ep:   ep,
	}, ep, nil
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
	// ADR-0252: when a request is pending on the connection budget, CLOSE this
	// idle conn instead of pooling it — its connWithGauge.Close runs connDec →
	// pool.releaseConn, freeing a permit + waking the head waiter (which dials
	// fresh). Routes the idle-return wake through the single permit mechanism
	// (no direct conn handoff). Racy peek; at worst a missed pooling, never wrong.
	if c.circuitBreaker != nil && c.circuitBreaker.pool != nil && c.circuitBreaker.pool.hasWaiter() {
		_ = p.Conn.Close()
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
