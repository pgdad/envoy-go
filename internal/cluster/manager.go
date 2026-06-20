package cluster

import (
	"context"
	stdtls "crypto/tls"
	"fmt"
	"sort"
	"sync"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"github.com/esalaine/envoy-go/internal/stats"
	internaltls "github.com/esalaine/envoy-go/internal/tls"
)

// upstreamTLSContextTypeURL is the well-known Any type URL for
// UpstreamTlsContext. Declared locally to avoid adding an exported symbol to
// internal/tls just for this comparison.
const upstreamTLSContextTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"

// httpProtocolOptionsKey is the well-known key under which the cluster-side
// HttpProtocolOptions extension lives in
// cluster.typed_extension_protocol_options. The Envoy proto authoritatively
// reserves this key on the v3 cluster surface; see SPEC §5.5.
const httpProtocolOptionsKey = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

// Manager owns every Cluster materialized from static_resources.clusters[].
// Get is the dataplane-side lookup the TCP proxy filter uses to resolve a
// cluster name at filter-construction time.
type Manager struct {
	clusters map[string]*Cluster

	// hcCancel cancels the active-HC runtime launched by StartHealthChecks;
	// hcWG tracks the per-checker goroutines so Drain can join them. Both are
	// zero-valued until StartHealthChecks runs (phase 39.1, ADR-0243).
	hcCancel context.CancelFunc
	hcWG     sync.WaitGroup

	// odCancel/odWG mirror hcCancel/hcWG for the passive-outlier sweep runtime
	// (StartOutlierDetection; phase 40.3, ADR-0247). Zero-valued until started.
	odCancel context.CancelFunc
	odWG     sync.WaitGroup
}

// NewManager walks bs.GetStaticResources().GetClusters() and materializes one
// Cluster per entry. Errors are returned at the first violation; subsequent
// clusters are not validated. Every error begins with "cluster: ".
//
// Phase-02 surface (SPEC §2 + §5.4):
//   - cluster.type must be STATIC. STRICT_DNS, LOGICAL_DNS, EDS, ORIGINAL_DST
//     all error explicitly.
//   - cluster.lb_policy must be unset (proto default ROUND_ROBIN), ROUND_ROBIN,
//     LEAST_REQUEST (phase 34; least_request_lb_config.choice_count parsed,
//     default 2), or RANDOM (phase 35; no config message — bare construction).
//     Anything else (RING_HASH/MAGLEV/...) errors.
//   - load_assignment.endpoints[*].lb_endpoints[*] must collectively contain
//     ≥1 endpoint, each with endpoint.address.socket_address (no pipe, no
//     envoy_internal_address). Total endpoint count across all locality
//     groups must be ≥1.
//   - cluster.connect_timeout, when unset, defaults to 5s (defaultConnectTimeout).
//
// Phase-02 callers that do not need baseDir resolution should continue using
// NewManager. Phase-03 callers that load filename-based DataSources should use
// NewManagerWithBaseDir and pass filepath.Dir(configPath).
//
// Phase 06.1 (Task 8) widened the signature to accept a *stats.Registry; for
// each cluster the manager allocates the 8 cluster-scope metrics from SPEC §6
// (per ADR-0063 — cluster-level only) on the supplied Registry at boot time.
// The caller MUST pass a non-nil Registry; the Registry MUST not yet be
// Frozen (cmd/envoy-go/main.go's boot sequence freezes only after the listener
// manager and admin server are up — see SPEC §5.4).
func NewManager(bs *bootstrapv3.Bootstrap, registry *stats.Registry) (*Manager, error) {
	return NewManagerWithBaseDir(bs, "", registry)
}

// NewManagerWithBaseDir is the phase-03 variant of NewManager. baseDir is
// passed to internal/tls.NewUpstreamConfig so that filename-based DataSources
// in transport_socket are resolved relative to the config file location. Pass
// "" to resolve relative to the process working directory (phase-02 compat).
//
// Phase 06.1 (Task 8): see NewManager doc — same Registry contract applies.
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string, registry *stats.Registry) (*Manager, error) {
	cs := bs.GetStaticResources().GetClusters()
	if len(cs) == 0 {
		return nil, fmt.Errorf("cluster: zero clusters in bootstrap")
	}
	m := &Manager{clusters: make(map[string]*Cluster, len(cs))}
	for i, c := range cs {
		built, err := buildCluster(c, i, baseDir)
		if err != nil {
			return nil, err
		}
		if _, dup := m.clusters[built.name]; dup {
			return nil, fmt.Errorf("cluster: duplicate cluster name %q", built.name)
		}
		registerClusterMetrics(registry, built)
		m.clusters[built.name] = built
	}
	return m, nil
}

// registerClusterMetrics allocates the 8 cluster-scope metrics per ADR-0063
// and stores the pointers on c. Called once per cluster at Manager build time;
// pre-Freeze (the listener manager and admin server precede the
// registry.Freeze() call in cmd/envoy-go/main.go — see SPEC §5.4 for the boot
// ordering invariant). The membership_total gauge is Set once at register
// time to len(c.endpoints) per SPEC §6 (cluster-level only; per-endpoint
// stats are deferred per ADR-0063).
func registerClusterMetrics(r *stats.Registry, c *Cluster) {
	prefix := "cluster." + c.name + "."
	// Stash the registry + prefix so EnsureRetryStats can lazily register the +5
	// retry counters on retry-policy clusters only (scoped departure; ADR-0249).
	c.statsReg = r
	c.statsPrefix = prefix
	c.upstreamRqTotal = r.NewCounter(prefix + "upstream_rq_total")
	c.upstreamRq2xx = r.NewCounter(prefix + "upstream_rq_2xx")
	c.upstreamRq3xx = r.NewCounter(prefix + "upstream_rq_3xx")
	c.upstreamRq4xx = r.NewCounter(prefix + "upstream_rq_4xx")
	c.upstreamRq5xx = r.NewCounter(prefix + "upstream_rq_5xx")
	c.upstreamCxTotal = r.NewCounter(prefix + "upstream_cx_total")
	c.upstreamCxActive = r.NewGauge(prefix + "upstream_cx_active")
	c.membershipTotal = r.NewGauge(prefix + "membership_total")
	c.membershipTotal.Set(int64(len(c.endpoints))) // SPEC §6: Set once at register, equals N endpoints
	// NOTE: when c.lb is a *subsetLB wrapping a *ringHashLB/*maglevLB (a cluster with
	// lb_subset_config), neither assert below fires, so the ring_hash_lb/maglev_lb
	// gauges are not registered for that cluster — a known MVP gap (no subset+ring_hash
	// fixture exists yet; deferred).
	if rh, ok := c.lb.(*ringHashLB); ok {
		// AMEND-RH4: 3 mirrored static gauges, Set once at register (the ring is
		// immutable). RING_HASH-only (reference parity); cross-side-exact (keyed on
		// ring-config + host count, not addresses) — D-S36-6.
		r.NewGauge(prefix + "ring_hash_lb.size").Set(int64(rh.size))
		r.NewGauge(prefix + "ring_hash_lb.min_hashes_per_host").Set(int64(rh.minPerHost))
		r.NewGauge(prefix + "ring_hash_lb.max_hashes_per_host").Set(int64(rh.maxPerHost))
	}
	if mg, ok := c.lb.(*maglevLB); ok {
		// AMEND-M4: 2 mirrored static gauges, set once at register (the table is
		// immutable). MAGLEV-only (reference parity); cross-side-exact (floor(M/N)/
		// ceil(M/N), keyed on table_size + host count + weights, address-independent).
		// NO size gauge — the table size is config-known (D-M4).
		r.NewGauge(prefix + "maglev_lb.min_entries_per_host").Set(int64(mg.minPerHost))
		r.NewGauge(prefix + "maglev_lb.max_entries_per_host").Set(int64(mg.maxPerHost))
	}
	if s, ok := c.lb.(*subsetLB); ok {
		// active/created = the distinct-subset count (×1 — NOT the reference's 33×
		// magnitude artifact, a recorded departure; AMEND-SS4 / ADR-0240).
		r.NewGauge(prefix + "lb_subsets_active").Set(int64(s.numSubsets))
		r.NewCounter(prefix + "lb_subsets_created").Add(uint64(s.numSubsets))
		// selected/fallback are Inc'd from subsetLB.Pick — allocate + INJECT onto the
		// live wrapper (the FIRST LB to touch stats from its own Pick path; D-S38-4).
		s.selected = r.NewCounter(prefix + "lb_subsets_selected")
		s.fallbackC = r.NewCounter(prefix + "lb_subsets_fallback")
	}
	// Phase 39.1 (ADR-0243): the +7 active-HC stats, on clusters WITH
	// health_checks only. membership_healthy starts at the full endpoint count
	// (all hosts begin healthy); the per-checker health_check.* handles are
	// injected by registerStats. MVP assumes ≤1 checker per cluster (the fixture
	// uses one); a multi-check cluster would duplicate-register and panic — out
	// of MVP scope.
	if c.health != nil {
		c.health.membershipHealthy = r.NewGauge(prefix + "membership_healthy")
		c.health.membershipHealthy.Set(int64(len(c.endpoints))) // all hosts start healthy
		c.health.panicCounter = r.NewCounter(prefix + "lb_healthy_panic")
		for _, hc := range c.checkers {
			hc.registerStats(r, prefix)
		}
	}
	// Phase 40.1 (ADR-0245) + 40.2 (ADR-0246): the +9 passive outlier_detection
	// stats (the 40.1 five + the +4 gateway/local-origin detector counters), on
	// clusters WITH outlier_detection only. The ejections_active gauge is injected onto
	// BOTH the detector and the shared clusterHealth — the lazy un-eject in
	// clusterHealth.isEjected decrements the SAME handle the detector increments
	// on eject (they MUST be the same *stats.Gauge instance).
	if c.outlier != nil {
		c.outlier.registerStats(r, prefix, c.health)
	}
	// Phase 41 (ADR-0248): the +14 circuit_breakers stats (10 per-priority *_open
	// gauges + 4 cluster overflow counters), on clusters WITH circuit_breakers
	// only. Only default.rq_open + upstream_rq_pending_overflow bind LIVE handles;
	// the other 12 register-and-stay-at-0 (deferred budgets / AMEND-CB1/CB2).
	if c.circuitBreaker != nil {
		c.circuitBreaker.registerStats(r, prefix)
	}
}

// Get looks up a cluster by name. Returns (nil, false) if not found.
func (m *Manager) Get(name string) (*Cluster, bool) {
	c, ok := m.clusters[name]
	return c, ok
}

// ClusterInfo is the public read-only summary of one cluster, returned by
// Manager.Clusters() and consumed by phase-08.1's /clusters admin handler.
// Per ADR-0087, the /clusters handler reads the snapshot once per scrape and
// formats one block per cluster (10 cluster-level lines + 18 per-endpoint
// lines per the §11.2 empirical pin).
//
//nolint:revive // ADR-0087 reserves the ClusterInfo name for the public /clusters-snapshot surface; phase-08.1 SPEC §6.2 fixes the type name verbatim.
type ClusterInfo struct {
	Name      string
	Endpoints []EndpointInfo
}

// EndpointInfo is the public read-only summary of one upstream endpoint
// within a ClusterInfo. Address is the dotted-quad / IPv6-literal host; Port
// is the TCP port. The combined "address:port" form is what the /clusters
// handler emits in the per-endpoint key prefix per SPEC §11.2.
type EndpointInfo struct {
	Address string
	Port    uint32
}

// Clusters returns a freshly-allocated snapshot of all configured clusters
// and their endpoints, in alphabetical-by-name order. Per-cluster endpoints
// are returned in their bootstrap-declared order (the order extractEndpoints
// preserves at NewManager time). The returned slice is safe for caller
// mutation: modifying it does not affect the Manager's internal state.
//
// Counters / gauges are NOT cached in the returned struct — phase-08.1's
// /clusters handler emits literal `0` for all 8 per-endpoint cx_*/rq_*
// counter fields per the planner-time decision (envoy-go has no per-endpoint
// stats per ADR-0063 deferral; cluster-level counters are surfaced via
// /stats/prometheus and would not partition meaningfully across endpoints).
//
// Phase 08.1 (Task 3) introduces this accessor; ADR-0087 records the design.
func (m *Manager) Clusters() []ClusterInfo {
	out := make([]ClusterInfo, 0, len(m.clusters))
	for _, c := range m.clusters {
		eps := make([]EndpointInfo, 0, len(c.endpoints))
		for _, ep := range c.endpoints {
			eps = append(eps, EndpointInfo{Address: ep.Host, Port: ep.Port})
		}
		out = append(out, ClusterInfo{Name: c.name, Endpoints: eps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Drain closes upstream connection pools across all configured clusters.
// Best-effort — returns no error. Walks m.clusters map and calls
// c.closePool() on each cluster.
//
// Called from cmd/envoy-go/main.go AFTER <-drainMgr.Done() fires (i.e.,
// after no in-flight downstream requests remain — therefore no in-flight
// upstream requests can remain). The pool close releases socket file
// descriptors for cleanest shutdown but is not required for correctness
// (Go's runtime will close TCP sockets on process exit regardless).
//
// Idempotent; safe to call multiple times. The m.clusters map is populated
// once at construction (NewManager) and never modified afterward, so
// concurrent range here is safe. Per-cluster pool fields, when they exist
// in future hot-restart-family expansions, must provide their own close-
// idempotency (e.g., sync.Once) per planner-time decision 6.
//
// Phase 08.2 (Task 4) introduces this accessor; ADR-0096 records the
// design (the consolidated in-flight-completion ADR; Tasks 9 + 10 cite
// ADR-0096 without re-anchoring).
func (m *Manager) Drain() {
	// Phase 39.1 (ADR-0243): stop the active-HC runtime first (cancel the
	// shared ctx, then join the per-checker goroutines) so no probe races the
	// pool close. No-op when StartHealthChecks was never called (hcCancel nil).
	if m.hcCancel != nil {
		m.hcCancel()
		m.hcWG.Wait()
	}
	// Phase 40.3 (ADR-0247): stop the passive-outlier sweep runtime symmetrically
	// with the active-HC stop above. No-op when StartOutlierDetection was never
	// called (odCancel nil).
	if m.odCancel != nil {
		m.odCancel()
		m.odWG.Wait()
	}
	for _, c := range m.clusters {
		c.closePool()
	}
}

// StartHealthChecks launches the active-HC runtime for every cluster with
// health_checks configured (one goroutine per checker). Call AFTER the stats
// registry is frozen (post-boot) so the injected stat handles are live. The
// checkers stop when ctx is canceled OR Drain() is called (Drain cancels the
// derived ctx and joins the goroutines). Phase 39.1 (ADR-0243).
func (m *Manager) StartHealthChecks(ctx context.Context) {
	hcCtx, cancel := context.WithCancel(ctx)
	m.hcCancel = cancel
	for _, c := range m.clusters {
		for _, hc := range c.checkers {
			m.hcWG.Add(1)
			go func(hc *healthChecker) {
				defer m.hcWG.Done()
				hc.run(hcCtx)
			}(hc)
		}
	}
}

// StartOutlierDetection launches the per-interval aggregation sweep for every
// cluster with outlier_detection configured (one goroutine per outlier cluster).
// Call AFTER the stats registry is frozen (post-boot) so the injected handles are
// live. The sweeps stop when ctx is canceled OR Drain() is called. Phase 40.3
// (ADR-0247) — the first outlier background goroutine.
func (m *Manager) StartOutlierDetection(ctx context.Context) {
	odCtx, cancel := context.WithCancel(ctx)
	m.odCancel = cancel
	for _, c := range m.clusters {
		if c.outlier == nil {
			continue
		}
		m.odWG.Add(1)
		go func(d *outlierDetector) {
			defer m.odWG.Done()
			d.run(odCtx)
		}(c.outlier)
	}
}

// buildLeafLB constructs the leaf load balancer for the cluster's lb_policy over
// the given endpoint set. Extracted from buildCluster so subsetLB can build a
// child per subset over a filtered sub-slice. CLUSTER_PROVIDED and every other
// unsupported policy reject HERE (the default arm) — before any subset wrap, so
// lb_subset_config + CLUSTER_PROVIDED rejects in outcome with ZERO new reject arm.
func buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint, health *clusterHealth) (loadBalancer, error) {
	switch c.GetLbPolicy() {
	case clusterv3.Cluster_ROUND_ROBIN:
		return &roundRobin{endpoints: endpoints, health: health}, nil
	case clusterv3.Cluster_LEAST_REQUEST:
		cc, err := parseLeastRequestLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		lr, err := newLeastRequest(endpoints, cc)
		if err != nil {
			return nil, err
		}
		lr.health = health
		return lr, nil
	case clusterv3.Cluster_RANDOM: // phase 35 (ADR-0234): stateless uniform pick; NO config parse (RANDOM has no config message)
		r, err := newRandom(endpoints)
		if err != nil {
			return nil, err
		}
		r.health = health
		return r, nil
	case clusterv3.Cluster_RING_HASH: // phase 36.1 (ADR-0236): Ketama consistent-hash ring
		cfg, err := parseRingHashLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		rh, err := newRingHash(endpoints, cfg)
		if err != nil {
			return nil, err
		}
		rh.health = health
		return rh, nil
	case clusterv3.Cluster_MAGLEV: // phase 37 (ADR-0238): Maglev consistent-hash table
		cfg, err := parseMaglevLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		mg, err := newMaglev(endpoints, cfg)
		if err != nil {
			return nil, err
		}
		mg.health = health
		return mg, nil
	default:
		return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV)", name, c.GetLbPolicy())
	}
}

func buildCluster(c *clusterv3.Cluster, idx int, baseDir string) (*Cluster, error) {
	name := c.GetName()
	if name == "" {
		return nil, fmt.Errorf("cluster: clusters[%d]: missing name", idx)
	}
	// Validate the assembled metric-name shape before any registry write.
	// cluster.<name> is propagated into eight "cluster.<name>.<metric>" names
	// at registerClusterMetrics; if the assembled name contains characters
	// outside the metric-name regex's permitted [a-zA-Z0-9_.] class (or
	// otherwise produces an invalid assembled name), Registry.NewCounter
	// would panic per ADR-0059's boot-time panic discipline. We reject at
	// the user-input boundary instead. Inherits ADR-0065's pattern by
	// reference per ADR-0065 Consequences (d) (no new ADR); symmetric to
	// the parseFilterWithCtx guard at internal/filter/hcm/config.go:143.
	// Validating the longest assembled name suffices because the other
	// seven assembled names differ only in suffixes within the regex's
	// permitted class (they pass/fail together).
	if !stats.IsValidName("cluster." + name + ".upstream_rq_total") {
		return nil, fmt.Errorf("cluster: %q: invalid cluster name (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)", name)
	}
	t, ok := c.GetClusterDiscoveryType().(*clusterv3.Cluster_Type)
	if !ok {
		return nil, fmt.Errorf("cluster: %q: cluster_discovery_type must be Type, got %T (only STATIC supported)", name, c.GetClusterDiscoveryType())
	}
	if t.Type != clusterv3.Cluster_STATIC {
		return nil, fmt.Errorf("cluster: %q: only STATIC clusters supported; got %s", name, t.Type)
	}
	la := c.GetLoadAssignment()
	if la == nil {
		return nil, fmt.Errorf("cluster: %q: missing load_assignment", name)
	}
	endpoints, err := extractEndpoints(la, name)
	if err != nil {
		return nil, err
	}
	timeout := defaultConnectTimeout
	if c.GetConnectTimeout() != nil {
		timeout = c.GetConnectTimeout().AsDuration()
	}
	// Phase 39.2 (ADR-0244): parse health_checks (http+tcp+grpc checkers;
	// surfaces the §6 rejects at boot) and build the per-cluster health registry
	// when configured. health is nil for a cluster with no health_checks -> the
	// LB fast path (byte-stable). hasGrpcHC is consumed below to enforce that a
	// grpc_health_check cluster supports HTTP/2 (extractH2Mode gate).
	hcSpecs, hasGrpcHC, err := parseHealthChecks(c, name)
	if err != nil {
		return nil, err
	}
	// Phase 40.1 (ADR-0245): parse outlier_detection (passive HC; surfaces the
	// §6 rejects at boot). outlierCfg is nil for a cluster with no
	// outlier_detection. A cluster with outlier_detection but no health_checks
	// still needs the per-cluster health registry (the ejection state lives
	// there); Task 6 consumes outlierCfg to build the detector.
	outlierCfg, err := parseOutlierDetection(c, name)
	if err != nil {
		return nil, err
	}
	var health *clusterHealth
	if len(hcSpecs) > 0 || outlierCfg != nil {
		health = newClusterHealth(endpoints, parsePanicThreshold(c))
	}
	cl := &Cluster{
		name:           name,
		endpoints:      endpoints,
		connectTimeout: timeout,
		// lb set by buildLeafLB below (ADR-0233; SPEC §3.4)
	}
	lb, err := buildLeafLB(c, name, endpoints, health)
	if err != nil {
		return nil, err // CLUSTER_PROVIDED / unsupported reject HERE — before any subset wrap
	}
	if sc := c.GetLbSubsetConfig(); sc != nil {
		lb = newSubsetLB(endpoints, parseLbSubsetConfig(sc), func(sub []Endpoint) (loadBalancer, error) {
			return buildLeafLB(c, name, sub, health)
		})
	}
	cl.lb = lb
	cl.health = health
	// Phase 39.1 (ADR-0243): build one background prober per configured
	// health_check over the cluster's endpoint set. Stat handles are injected
	// in registerClusterMetrics; the probers are launched by
	// Manager.StartHealthChecks (post-Freeze) and stopped by Manager.Drain.
	if health != nil {
		for _, spec := range hcSpecs {
			cl.checkers = append(cl.checkers, newHealthChecker(cl.endpoints, health, spec))
		}
	}
	// Phase 40.1 (ADR-0245): build the passive outlier detector over the same
	// endpoint set + shared health registry when outlier_detection is configured
	// (outlierCfg != nil implies health != nil per the creation-site widening
	// above). The enforce roll is a [0,100) draw from a crypto-seeded PCG (the
	// reference's enforcing_consecutive_5xx percentage gate). The 5 stat handles
	// stay nil here; registerClusterMetrics injects them (Task 8).
	if outlierCfg != nil {
		rng, err := newPCGRNG()
		if err != nil {
			return nil, err
		}
		cl.outlier = &outlierDetector{
			cfg:         *outlierCfg,
			health:      health,
			endpoints:   endpoints,
			enforceRoll: func() uint32 { return uint32(rng() % 100) },
		}
	}
	// Phase 41 (ADR-0248): parse circuit_breakers + build the per-priority budgets
	// for a cluster with circuit_breakers (nil otherwise). The stat handles stay nil
	// here; registerClusterMetrics injects them (Task 4). The enforce seam (router
	// wiring) is added in Task 5.
	cbCfg, err := parseCircuitBreakers(c, name)
	if err != nil {
		return nil, err
	}
	cl.circuitBreaker = cbCfg
	if ts := c.GetTransportSocket(); ts != nil {
		if ts.GetTypedConfig() == nil {
			return nil, fmt.Errorf("cluster: %q: transport_socket without typed_config", name)
		}
		tu := ts.GetTypedConfig().GetTypeUrl()
		if tu != upstreamTLSContextTypeURL {
			return nil, fmt.Errorf("cluster: %q: unsupported transport_socket type_url %q (phase 03 supports only UpstreamTlsContext)", name, tu)
		}
		uc, err := internaltls.NewUpstreamConfig(ts, baseDir)
		if err != nil {
			return nil, fmt.Errorf("cluster: %q: %w", name, err)
		}
		cl.upstreamCfg = uc.TLSConfig
	}
	// Phase 05.2 (Task 10, SPEC §5.5): read typed_extension_protocol_options for
	// HttpProtocolOptions and decide whether this cluster originates H2 upstream.
	useH2, err := extractH2Mode(c, cl.upstreamCfg)
	if err != nil {
		return nil, err
	}
	// Phase 39.2 (ADR-0244): grpc_health_check uses the gRPC protocol (HTTP/2);
	// reject clusters that do not declare HTTP/2 support, because the prober
	// would silently fail against an H1-only upstream.
	if hasGrpcHC && !useH2 {
		return nil, fmt.Errorf("cluster: %q: grpc_health_check requires the cluster to support HTTP/2", name)
	}
	cl.useH2 = useH2
	return cl, nil
}

// defaultChoiceCount is the P2C sample size when least_request_lb_config is
// absent or its choice_count is unset (the proto doc-comment default; reference
// parity — SPEC §5.1 / §6.3).
const defaultChoiceCount = 2

// parseLeastRequestLbConfig extracts the P2C choice_count for a LEAST_REQUEST
// cluster and applies the SPEC §6 reject matrix. GetLeastRequestLbConfig() is
// nil-safe (nil on an absent OR a mismatched lb_config oneof member — AMEND-L1):
// both fall to defaultChoiceCount (§6.3 silent-ignore parity). The choice_count
// gate is HAND-ROLLED to mirror the PGV string (the manager calls no PGV —
// AMEND-L1). active_request_bias / slow_start_config set under LEAST_REQUEST are
// behavior-bearing DEPARTURE rejects (AMEND-L5/L6 — the reference parse-accepts).
func parseLeastRequestLbConfig(c *clusterv3.Cluster, name string) (int, error) {
	lrc := c.GetLeastRequestLbConfig()
	if lrc == nil {
		return defaultChoiceCount, nil
	}
	if lrc.GetActiveRequestBias() != nil {
		return 0, fmt.Errorf("cluster: %q: least_request_lb_config.active_request_bias is not supported", name)
	}
	if lrc.GetSlowStartConfig() != nil {
		return 0, fmt.Errorf("cluster: %q: least_request_lb_config.slow_start_config is not supported", name)
	}
	if cc := lrc.GetChoiceCount(); cc != nil {
		v := cc.GetValue()
		if v < 2 {
			return 0, fmt.Errorf("cluster: %q: least_request_lb_config.choice_count: value must be greater than or equal to 2", name)
		}
		return int(v), nil // no clamp at > len(endpoints) — reference parity (AMEND-L3)
	}
	return defaultChoiceCount, nil
}

// Ring-size defaults / cap (proto doc-comment defaults; PGV lte cap — SPEC §6.4).
const (
	defaultMinRingSize = 1024
	defaultMaxRingSize = 8388608
	ringSizeCap        = 8388608
)

// parseRingHashLbConfig extracts the ring-hash parameters for a RING_HASH
// cluster (AMEND-RH4). GetRingHashLbConfig() is nil-safe (nil on an absent OR a
// mismatched lb_config oneof member): both fall to the self-supplied defaults
// (1024 / 8388608 / XX_HASH — §6.3 silent-ignore parity). The gate is
// hand-rolled in two layers (the manager calls no PGV): a per-field PGV mirror
// (value <= 8388608) plus a runtime min > max cross-field check.
func parseRingHashLbConfig(c *clusterv3.Cluster, name string) (ringHashCfg, error) {
	cfg := ringHashCfg{minRingSize: defaultMinRingSize, maxRingSize: defaultMaxRingSize, hashFunc: hashXX}
	rhc := c.GetRingHashLbConfig()
	if rhc == nil {
		return cfg, nil // absent OR mismatched oneof → defaults (§6.3 silent-ignore parity)
	}
	if v := rhc.GetMinimumRingSize(); v != nil {
		if v.GetValue() > ringSizeCap {
			return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.minimum_ring_size: value must be less than or equal to 8388608", name)
		}
		cfg.minRingSize = v.GetValue()
	}
	if v := rhc.GetMaximumRingSize(); v != nil {
		if v.GetValue() > ringSizeCap {
			return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.maximum_ring_size: value must be less than or equal to 8388608", name)
		}
		cfg.maxRingSize = v.GetValue()
	}
	if cfg.minRingSize > cfg.maxRingSize {
		return ringHashCfg{}, fmt.Errorf("cluster: %q: ring hash: minimum_ring_size (%d) > maximum_ring_size (%d)", name, cfg.minRingSize, cfg.maxRingSize)
	}
	switch rhc.GetHashFunction() {
	case clusterv3.Cluster_RingHashLbConfig_XX_HASH:
		cfg.hashFunc = hashXX
	case clusterv3.Cluster_RingHashLbConfig_MURMUR_HASH_2:
		cfg.hashFunc = hashMurmur
	default:
		return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.hash_function: unsupported value %v", name, rhc.GetHashFunction())
	}
	return cfg, nil
}

// Maglev table-size default / cap (proto doc-comment default; PGV lte cap — §6).
const (
	defaultMaglevTableSize = 65537   // doc-comment default (self-supplied; nil getter)
	maglevTableSizeCap     = 5000011 // PGV: value must be less than or equal to 5000011
)

// parseMaglevLbConfig extracts the table_size for a MAGLEV cluster.
// GetMaglevLbConfig() is nil-safe (nil on an absent OR a mismatched lb_config
// oneof member): both fall to the self-supplied default (§6.3 silent-ignore
// parity). The gate is hand-rolled (the manager calls no PGV): the PGV cap
// mirror (<= 5000011) THEN the primality check (reference parity — the reference
// throws "The table size of maglev must be prime number"; AMEND-M5).
func parseMaglevLbConfig(c *clusterv3.Cluster, name string) (maglevCfg, error) {
	cfg := maglevCfg{tableSize: defaultMaglevTableSize}
	mlc := c.GetMaglevLbConfig()
	if mlc == nil {
		return cfg, nil // absent OR mismatched oneof → default (§6.3 silent-ignore parity)
	}
	if v := mlc.GetTableSize(); v != nil {
		ts := v.GetValue()
		if ts > maglevTableSizeCap {
			return maglevCfg{}, fmt.Errorf("cluster: %q: maglev_lb_config.table_size: value must be less than or equal to 5000011", name)
		}
		if !isPrime(ts) {
			return maglevCfg{}, fmt.Errorf("cluster: %q: maglev_lb_config.table_size (%d) must be a prime number", name, ts)
		}
		cfg.tableSize = ts
	}
	return cfg, nil
}

// parseLbSubsetConfig lowers Cluster.LbSubsetConfig to the proto-free lbSubsetCfg.
// default_subset scalars are lowered (non-scalar dropped); the selector keys are
// copied; the deferred flags (single_host_per_subset, per-selector fallback,
// locality/panic/list_as_any/metadata_fallback) are ignored. Returns the zero
// cfg if sc is nil.
func parseLbSubsetConfig(sc *clusterv3.Cluster_LbSubsetConfig) lbSubsetCfg {
	cfg := lbSubsetCfg{}
	switch sc.GetFallbackPolicy() {
	case clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT:
		cfg.fallback = fallbackAnyEndpoint
	case clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET:
		cfg.fallback = fallbackDefaultSubset
	default: // NO_FALLBACK
		cfg.fallback = fallbackNoFallback
	}
	if ds := sc.GetDefaultSubset(); ds != nil {
		cfg.defaultSubset, _ = ScalarsFromStruct(ds) // drop non-scalar; all-non-scalar → empty map → zero-match → NO_FALLBACK at runtime
	}
	for _, sel := range sc.GetSubsetSelectors() {
		cfg.selectors = append(cfg.selectors, sel.GetKeys())
	}
	return cfg
}

// extractH2Mode reads the cluster's typed_extension_protocol_options and
// returns whether to enable H2 upstream origination. Per SPEC §5.5's behavior
// matrix:
//   - field absent → false (phase-04 baseline; no regression)
//   - explicit_http_config.http2_protocol_options{} → true (validated; build-time TLS-or-h2c branch)
//   - explicit_http_config.http_protocol_options{} → false (silent-ignore inner)
//   - auto_config{} → false (the 05.2 narrowing of master SPEC §5.8)
//   - nil/empty UpstreamProtocolOptions → false (defensive)
//
// When useH2==true the gate relaxes into two accepted shapes (ADR-0166):
//
//   - transport_socket PRESENT → existing TLS+h2 path bit-identical: the
//     transport_socket MUST be type tls and the parsed TLS config's
//     alpn_protocols MUST include "h2". Validation errors carry the
//     diagnostics enumerated in SPEC §4.1.
//   - transport_socket ABSENT → plaintext h2c upstream is PERMITTED
//     (h2c prior-knowledge per RFC 7540 §3.4). Reference Envoy v1.37.2
//     accepts the same shape; the gRPC ecosystem norm is plaintext h2c
//     upstream for service-mesh sidecars. ADR-0166 anchors the relaxation;
//     it supersedes the original phase-05.2 SPEC §5.5 "TLS required for h2"
//     rule by precedent.
//
// parsedTLS is the *stdtls.Config produced by internal/tls.NewUpstreamConfig
// from the cluster's transport_socket; its NextProtos field is populated from
// CommonTlsContext.alpn_protocols. Pass nil when the cluster has no
// transport_socket (plaintext h2c upstream); the runtime dial path (see
// dial_h2.go) symmetrically skips the TLS-conn assertion when parsedTLS==nil.
func extractH2Mode(c *clusterv3.Cluster, parsedTLS *stdtls.Config) (useH2 bool, err error) {
	tepo := c.GetTypedExtensionProtocolOptions()
	if tepo == nil {
		return false, nil
	}
	tepoAny, ok := tepo[httpProtocolOptionsKey]
	if !ok {
		return false, nil
	}
	var hpo upstreamshttpv3.HttpProtocolOptions
	if err := tepoAny.UnmarshalTo(&hpo); err != nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions: unmarshal: %w", c.GetName(), err)
	}
	switch up := hpo.UpstreamProtocolOptions.(type) {
	case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_:
		switch up.ExplicitHttpConfig.GetProtocolConfig().(type) {
		case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions:
			useH2 = true
		default:
			useH2 = false // H1 discriminator: silent-ignore inner.
		}
	case *upstreamshttpv3.HttpProtocolOptions_AutoConfig:
		useH2 = false // The 05.2 narrowing of master SPEC §5.8 (silent-ignore).
	default:
		useH2 = false // Defensive: nil / use_downstream_protocol_config / etc.
	}
	if !useH2 {
		return false, nil
	}
	// ADR-0166: transport_socket is OPTIONAL for h2 upstream. When absent,
	// the cluster originates plaintext h2c (prior-knowledge per RFC 7540 §3.4);
	// when present, the existing TLS+h2 validation branch applies bit-identical.
	ts := c.GetTransportSocket()
	if ts == nil {
		// Plaintext h2c upstream — PERMITTED (ADR-0166). parsedTLS is nil
		// by buildCluster's contract (no transport_socket → no cl.upstreamCfg).
		return true, nil
	}
	if ts.GetTypedConfig() == nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got transport_socket without typed_config", c.GetName())
	}
	if tu := ts.GetTypedConfig().GetTypeUrl(); tu != upstreamTLSContextTypeURL {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got %q", c.GetName(), tu)
	}
	if parsedTLS == nil {
		// transport_socket present but TLS parsing returned nil — internal
		// invariant. The earlier transport_socket parse path always sets
		// cl.upstreamCfg non-nil for the UpstreamTlsContext type_url, so this
		// branch is defense-in-depth.
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options: TLS parse returned nil", c.GetName())
	}
	hasH2 := false
	for _, alpn := range parsedTLS.NextProtos {
		if alpn == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires alpn_protocols to include %q, got %v", c.GetName(), "h2", parsedTLS.NextProtos)
	}
	return true, nil
}

func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error) {
	var out []Endpoint
	for gi, group := range la.GetEndpoints() {
		for ei, lbe := range group.GetLbEndpoints() {
			ep := lbe.GetEndpoint()
			if ep == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint is nil", clusterName, gi, ei)
			}
			addr := ep.GetAddress()
			if addr == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint.address is nil", clusterName, gi, ei)
			}
			sa := addr.GetSocketAddress()
			if sa == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d]: only socket_address endpoints supported", clusterName, gi, ei)
			}
			scalars, _ := ScalarsFromStruct(lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]) // drop non-scalar keys
			out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cluster: %q: zero endpoints across all locality groups", clusterName)
	}
	return out, nil
}
