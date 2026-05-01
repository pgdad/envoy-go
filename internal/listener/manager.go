package listener

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/stats"
	internaltls "github.com/esalaine/envoy-go/internal/tls"
)

// Info is the after-Start description of one bound listener: the name
// from the bootstrap and the actually-bound socket address (resolved from the
// configured address; differs when the bootstrap requested port_value: 0).
type Info struct {
	Name string
	Addr string
}

// filterHandler is the abstract behavior every constructed filter must offer.
// Inline registry value type — phase 07 lifts to an exported interface.
type filterHandler interface {
	Handle(ctx context.Context, downstream net.Conn)
}

// listenerCtx carries per-chain context that filter constructors consult at
// build time. Phase 05.1 introduces this to plumb the --allow-h2c flag through
// to the HCM constructor (per ADR-0049). Future phases may extend.
type listenerCtx struct {
	hasTLS   bool
	allowH2C bool
}

// filterConstructor builds a filterHandler from a typed_config Any, the
// resolved cluster manager, per-chain listenerCtx, the Registry the HCM
// constructor will use to allocate its 5 per-instance metrics (06.1 Task 11),
// the accessLogSinks slice threaded from main.go (06.2 Task 14), and the
// boot-populated, frozen *filter_http.HTTPRegistry threaded from main.go
// (07.1 Task 14, per ADR-0072). The TCP-proxy constructor ignores everything
// past cm (no per-tcp_proxy metrics, access logging, or HTTP-filter chain
// in the L4 path).
type filterConstructor func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry) (filterHandler, error)

// filterRegistry maps a filter typed_config.type_url to its constructor.
// SPEC §5.3: inline; phase 07 generalises.
var filterRegistry = map[string]filterConstructor{
	tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, _ listenerCtx, _ *stats.Registry, _ []accesslog.Sink, _ *filter_http.HTTPRegistry) (filterHandler, error) {
		f, err := tcpproxy.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
	hcm.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry) (filterHandler, error) {
		// Bridge listenerCtx into hcm.ListenerCtx (the public shape exposed by
		// hcm so that the listener manager doesn't import hcm-internal types).
		// Phase 06.1 Task 11: the Registry is consumed by the HCM constructor
		// to allocate the 5 HCM-scope per-instance metrics per SPEC §6.
		// Phase 06.2 Task 14: accessLogSinks are the opened AsyncFileSinks from main.go.
		// Phase 07.1 Task 14: httpRegistry is the boot-populated, frozen
		// *filter_http.HTTPRegistry threaded from main.go (Task 20 wires the
		// real boot-time population; ADR-0072 freeze-after-boot contract).
		f, err := hcm.NewFilterWithCtxAndSinksAndRegistry(tc, cm, hcm.ListenerCtx{HasTLS: lc.hasTLS, AllowH2C: lc.allowH2C}, registry, accessLogSinks, httpRegistry)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
}

// chainInfo holds the per-chain state built at NewManager time.
type chainInfo struct {
	serverNames []string       // from filter_chain_match.server_names; nil/empty = catch-all
	tlsCfg      *stdtls.Config // nil if plaintext chain
	filter      filterHandler  // exactly one terminal filter (phase-02 registry)
}

// listenerRuntime is the phase-03 replacement for builtListener. It holds
// everything needed to bind, accept, and dispatch connections on one listener.
type listenerRuntime struct {
	name    string
	addr    string
	netLn   net.Listener
	tlsMode bool
	tlsCfg  *stdtls.Config // top-level config with GetConfigForClient wired; nil for plaintext
	chains  []*chainInfo   // sorted most-specific-first (exact > suffix-wildcard > catch-all)
	// 06.1 metric fields (per SPEC §6 — listener-scope only, 2 metrics per
	// listener). Allocated by registerListenerMetrics at Start time (post-bind,
	// pre-Freeze) and Inc/Dec'd from the accept-loop hot path.
	downstreamCxTotal  *stats.Counter
	downstreamCxActive *stats.Gauge
}

// Manager owns every listener materialized from static_resources.listeners[]
// and supervises their accept loops.
type Manager struct {
	runtimes  []*listenerRuntime
	registry  *stats.Registry // captured at NewManager; consumed at Start to register the 2 listener-scope metrics per resolved bind address
	startedMu sync.Mutex
	started   bool
}

// NewManager validates the listeners in bs against the phase-03 filter-chain
// subset (ADR-0033, supersedes ADR-0025) and constructs each listener's
// terminal filter(s) against cm. Returns an error on the first violation;
// subsequent listeners are not validated. No sockets are touched.
//
// Every error begins with "listener: ".
//
// Phase 06.1 (Task 10) widened the signature to accept a *stats.Registry; for
// each listener the manager allocates the 2 listener-scope metrics from SPEC
// §6 on the supplied Registry at Start time (after net.Listen resolves the
// configured port — see registerListenerMetrics). The caller MUST pass a
// non-nil Registry; the Registry MUST not yet be Frozen (cmd/envoy-go/main.go's
// boot sequence freezes only after the listener manager and admin server are
// up — Task 12 owns that ordering per SPEC §5.4).
//
// Phase 07.1 (Task 14) widened the signature to accept a
// *filter_http.HTTPRegistry; the manager threads this into the HCM-construction
// closure for each filter_chain that builds an HCM filter. The registry MUST
// be non-nil and Frozen at call time per ADR-0072 (boot-time-populated,
// freeze-after-boot). Task 20 wires the real boot-time population; until then
// callers (test bootstraps) build a router-only frozen registry.
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, registry *stats.Registry, httpRegistry *filter_http.HTTPRegistry) (*Manager, error) {
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, "", false, registry, nil, httpRegistry)
}

// NewManagerWithBaseDir is the phase-03 variant of NewManager. baseDir is
// passed to internal/tls.NewDownstreamConfig so that filename-based
// DataSources in transport_socket are resolved relative to the config file
// location. Pass "" to resolve relative to the process working directory
// (phase-02 compat).
//
// Phase 06.1 (Task 10): see NewManager doc — same Registry contract applies.
// Phase 07.1 (Task 14): see NewManager doc — same HTTPRegistry contract applies.
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, registry *stats.Registry, httpRegistry *filter_http.HTTPRegistry) (*Manager, error) {
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, false, registry, nil, httpRegistry)
}

// NewManagerWithBaseDirAndAllowH2C is the phase-05.1 constructor variant. It
// threads the --allow-h2c boolean from cmd/envoy-go/main into a per-chain
// listenerCtx passed into the HCM filter constructor at build time. allowH2C
// permits HCM codec_type=HTTP2 on plaintext listeners (for h2spec conformance);
// default false.
//
// Existing callers NewManager and NewManagerWithBaseDir delegate here with
// allowH2C=false.
//
// Phase 06.1 (Task 10): the registry parameter is captured into each chain's
// HCM-factory closure so per-HCM-instance metric allocation works at Task 11
// filter-build time, AND it is the Registry on which the 2 listener-scope
// metrics are allocated by registerListenerMetrics at this constructor's
// per-listener loop.
//
// Phase 07.1 (Task 14): httpRegistry is the boot-populated, frozen
// *filter_http.HTTPRegistry captured into each chain's HCM-factory closure
// for http_filters[] type_url resolution per ADR-0072.
func NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry) (*Manager, error) {
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	m := &Manager{runtimes: make([]*listenerRuntime, 0, len(ls)), registry: registry}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		rt, err := buildListenerRuntimeWithCtx(l, i, cm, baseDir, allowH2C, registry, accessLogSinks, httpRegistry)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[rt.name]; dup {
			return nil, fmt.Errorf("listener: duplicate listener name %q", rt.name)
		}
		seen[rt.name] = struct{}{}
		m.runtimes = append(m.runtimes, rt)
	}
	return m, nil
}

// normalizeAddr turns a "host:port" listener bind-address string into the
// metric-name-segment form the SN3 flattening rule consumes: every ":" and
// "." is replaced with "_" so the resulting segment contains no dots and
// thus collapses to a single dot-segment in the hierarchical-dotted name
// `listener.<addr>.<rest>`. Example: "0.0.0.0:10000" → "0_0_0_0_10000".
//
// SPEC §6 shows the example "0.0.0.0_10000" with dots preserved; the planner
// selected the all-underscore form (PLAN.md Task 10 Step 2) because the SN3
// extractor in internal/stats/name.go uses `strings.Index(tail, ".")` to find
// the single delimiter — leaving dots in the address would cause the address
// segment to be truncated to its first IPv4 octet. The all-underscore form is
// also what name_test.go's `flattenToProm("listener.0_0_0_0_10000.…")`
// fixture verifies as canonical.
//
// IPv6 forms — net.Listener.Addr().String() returns IPv6 hosts wrapped in
// square brackets (e.g., "[::]:45259", "[::1]:8080") — also have "[" and "]"
// stripped here because brackets are not in the SN-name regex's permitted
// character class (`[a-zA-Z0-9_.]*`). "[::]:45259" thus normalizes to
// "___45259" (three colons → three underscores; brackets dropped). h2spec
// listens on IPv6 by default so this path is exercised by the conformance
// gate.
func normalizeAddr(addr string) string {
	return strings.NewReplacer(":", "_", ".", "_", "[", "", "]", "").Replace(addr)
}

// registerListenerMetrics allocates the 2 listener-scope metrics per SPEC §6
// and stores the pointers on rt for the accept-loop hot path. Called once per
// listener at Start time, after net.Listen resolves the configured port (so
// the metric name reflects the actual bound address, and so two listeners
// configured with port 0 don't collide on the same registered name pre-bind).
// Pre-Freeze (Task 12 owns the Freeze call after the admin server is up).
func registerListenerMetrics(r *stats.Registry, rt *listenerRuntime) {
	prefix := "listener." + normalizeAddr(rt.addr) + "."
	rt.downstreamCxTotal = r.NewCounter(prefix + "downstream_cx_total")
	rt.downstreamCxActive = r.NewGauge(prefix + "downstream_cx_active")
}

// buildListenerRuntimeWithCtx validates one Listener proto and constructs its
// listenerRuntime (including all chainInfo entries). allowH2C is threaded into
// each per-chain listenerCtx passed to the filter constructors. registry is
// captured into the HCM-factory closure for per-HCM-instance metric allocation
// at Task 11. accessLogSinks are the opened AsyncFileSinks from main.go
// (Phase 06.2 Task 14); nil means no access logging. httpRegistry is the
// boot-populated, frozen *filter_http.HTTPRegistry captured into the HCM-factory
// closure for http_filters[] type_url resolution (Phase 07.1 Task 14, per
// ADR-0072). No socket is bound here.
func buildListenerRuntimeWithCtx(l *listenerv3.Listener, idx int, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry) (*listenerRuntime, error) {
	name := l.GetName()
	if name == "" {
		return nil, fmt.Errorf("listener: listeners[%d]: missing name", idx)
	}
	addr := l.GetAddress().GetSocketAddress()
	if addr == nil {
		return nil, fmt.Errorf("listener: %q: address is not a socket_address", name)
	}

	// ADR-0033: Listener.default_filter_chain is not supported in phase 03.
	if l.GetDefaultFilterChain() != nil {
		return nil, fmt.Errorf("listener: %q: default_filter_chain is not supported in phase 03 (ADR-0033)", name)
	}

	chains := l.GetFilterChains()
	if len(chains) == 0 {
		return nil, fmt.Errorf("listener: %q: filter_chains must be non-empty", name)
	}

	catchAllCount := 0
	anyTLS := false
	var cis []*chainInfo

	for i, fc := range chains {
		// Validate filter_chain_match against the phase-03 whitelist.
		fm := fc.GetFilterChainMatch()
		if err := validateFilterChainMatch(fm); err != nil {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, err)
		}
		serverNames := fm.GetServerNames() // nil/empty = catch-all

		// Decode transport_socket (nil = plaintext, non-nil = TLS).
		var chainTLS *stdtls.Config
		if ts := fc.GetTransportSocket(); ts != nil {
			dc, err := internaltls.NewDownstreamConfig(ts, baseDir)
			if err != nil {
				return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, err)
			}
			chainTLS = dc.TLSConfig
			anyTLS = true
		}

		// Build the terminal filter (same single-filter rule as phase 02).
		filters := fc.GetFilters()
		if len(filters) != 1 {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: expected exactly one filter, got %d", name, i, len(filters))
		}
		tc := filters[0].GetTypedConfig()
		if tc == nil {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: filter typed_config is nil", name, i)
		}
		ctor, ok := filterRegistry[tc.GetTypeUrl()]
		if !ok {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: unknown filter type_url %q", name, i, tc.GetTypeUrl())
		}
		lc := listenerCtx{hasTLS: chainTLS != nil, allowH2C: allowH2C}
		fh, err := ctor(tc, cm, lc, registry, accessLogSinks, httpRegistry)
		if err != nil {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, err)
		}

		if len(serverNames) == 0 {
			catchAllCount++
		}
		cis = append(cis, &chainInfo{serverNames: serverNames, tlsCfg: chainTLS, filter: fh})
	}

	// ADR-0033: at most one catch-all chain per listener.
	if catchAllCount > 1 {
		return nil, fmt.Errorf("listener: %q: at most one filter_chain may omit filter_chain_match.server_names (catch-all); got %d", name, catchAllCount)
	}

	if anyTLS {
		// All chains must be TLS — mixed TLS/plaintext is not supported.
		for i, ci := range cis {
			if ci.tlsCfg == nil {
				return nil, fmt.Errorf("listener: %q: filter_chains[%d]: mixed TLS and plaintext chains on one listener are not supported", name, i)
			}
		}
	} else if len(cis) > 1 {
		// Plaintext with more than one chain: SNI cannot match on plaintext;
		// this is almost always a misconfiguration.
		return nil, fmt.Errorf("listener: %q: plaintext listener with multiple filter_chains is not supported (SNI match requires TLS)", name)
	}

	// Sort chains most-specific-first: exact > suffix-wildcard > catch-all.
	sort.SliceStable(cis, func(i, j int) bool {
		return chainSpecificityRank(cis[i].serverNames) < chainSpecificityRank(cis[j].serverNames)
	})

	rt := &listenerRuntime{
		name:    name,
		addr:    fmt.Sprintf("%s:%d", addr.GetAddress(), addr.GetPortValue()),
		tlsMode: anyTLS,
		chains:  cis,
	}
	if anyTLS {
		rt.tlsCfg = &stdtls.Config{
			GetConfigForClient: makeGetConfigForClient(rt),
		}
	}
	return rt, nil
}

// chainSpecificityRank returns a sort key for a chain's server_names slice.
// Lower rank = more specific = matched first.
//
//	0: exact pattern (any non-wildcard, non-"*" element)
//	1: suffix wildcard (leading "*.")
//	2: universal wildcard ("*")
//	3: catch-all (empty patterns slice)
func chainSpecificityRank(patterns []string) int {
	if len(patterns) == 0 {
		return 3
	}
	rank := 4
	for _, p := range patterns {
		switch {
		case p == "*":
			if 2 < rank {
				rank = 2
			}
		case strings.HasPrefix(p, "*."):
			if 1 < rank {
				rank = 1
			}
		default:
			// Any exact pattern immediately makes this chain rank 0.
			return 0
		}
	}
	return rank
}

// validateFilterChainMatch checks that fm contains only fields permitted by
// the phase-03 subset (ADR-0033). Non-nil fm fields beyond server_names[] and
// transport_protocol=="tls" are errors.
func validateFilterChainMatch(fm *listenerv3.FilterChainMatch) error {
	if fm == nil {
		return nil // empty match = catch-all
	}
	if fm.GetDestinationPort() != nil {
		return fmt.Errorf("destination_port is not supported (phase 07)")
	}
	if len(fm.GetPrefixRanges()) > 0 {
		return fmt.Errorf("prefix_ranges is not supported (phase 07)")
	}
	if len(fm.GetSourcePrefixRanges()) > 0 {
		return fmt.Errorf("source_prefix_ranges is not supported (phase 07)")
	}
	if fm.GetSourceType() != listenerv3.FilterChainMatch_ANY {
		return fmt.Errorf("source_type is not supported (phase 07)")
	}
	if len(fm.GetSourcePorts()) > 0 {
		return fmt.Errorf("source_ports is not supported (phase 07)")
	}
	if len(fm.GetApplicationProtocols()) > 0 {
		return fmt.Errorf("application_protocols is not supported (phase 07 — filter chain framework)")
	}
	if tp := fm.GetTransportProtocol(); tp != "" && tp != "tls" {
		return fmt.Errorf("transport_protocol %q is not supported (phase 03 permits only \"tls\")", tp)
	}
	// server_names[] is the only substantive match field consumed — no
	// validation needed beyond permitting it.
	return nil
}

// makeGetConfigForClient returns the GetConfigForClient callback for a TLS
// listener. It runs the chain-match logic at ClientHello time, returning the
// per-chain *stdtls.Config for the best-matching chain, or (nil, error) if no
// chain matches. Chains are already sorted most-specific-first by
// buildListenerRuntime.
func makeGetConfigForClient(rt *listenerRuntime) func(*stdtls.ClientHelloInfo) (*stdtls.Config, error) {
	return func(hello *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
		sni := hello.ServerName
		for _, ci := range rt.chains {
			if len(ci.serverNames) == 0 {
				// Catch-all — matches any SNI. Most-specific-first ordering
				// guarantees this is only reached when no specific chain matched.
				return ci.tlsCfg.Clone(), nil
			}
			if internaltls.MatchServerName(ci.serverNames, sni) {
				return ci.tlsCfg.Clone(), nil
			}
		}
		return nil, fmt.Errorf("listener: %q: no filter_chain matches SNI %q", rt.name, sni)
	}
}

// dispatch re-runs the chain-match logic after a successful handshake, using
// the SNI recorded in the connection state. Because SNI is fixed from the
// ClientHello through the connection's lifetime, re-running the pure-function
// match is deterministic (ADR-0033 chain-selection propagation mechanism).
func (rt *listenerRuntime) dispatch(ctx context.Context, tlsConn *stdtls.Conn) {
	sni := tlsConn.ConnectionState().ServerName
	for _, ci := range rt.chains {
		if len(ci.serverNames) == 0 || internaltls.MatchServerName(ci.serverNames, sni) {
			ci.filter.Handle(ctx, tlsConn)
			return
		}
	}
	// Should not happen: GetConfigForClient already rejected un-matchable SNIs.
	log.Printf("listener %q: post-handshake dispatch: no chain matches SNI %q (race/logic bug)", rt.name, sni)
	_ = tlsConn.Close()
}

// Start binds every built listener, captures its bound address, and launches
// one accept goroutine per listener. Returns after every bind succeeds (or on
// the first bind failure, after unwinding any already-bound sockets). Every
// error begins with "listener: ".
//
// Per SPEC §5.2 and SPEC §10 #7 (settled): a single ctx is captured once and
// shared across every accept loop and dispatched filter Handle invocation.
// Canceling ctx (or calling Stop) drops the loops within one accept call.
func (m *Manager) Start(ctx context.Context) error {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	if m.started {
		return fmt.Errorf("listener: Start already called")
	}
	for i, rt := range m.runtimes {
		ln, err := net.Listen("tcp", rt.addr)
		if err != nil {
			// Unwind: close any already-bound listeners.
			for j := 0; j < i; j++ {
				_ = m.runtimes[j].netLn.Close()
				m.runtimes[j].netLn = nil
			}
			return fmt.Errorf("listener: %q: bind %s: %w", rt.name, rt.addr, err)
		}
		rt.netLn = ln
		rt.addr = ln.Addr().String() // capture resolved address (e.g. when port was 0)
		// 06.1 (Task 10): allocate the 2 listener-scope metrics on the bound
		// (post-resolve) address. SPEC §6's `<addr>` is necessarily the bind
		// address — pre-bind the configured port may be 0 ("OS-pick"), which
		// would (a) produce metric names that don't reflect the actual bind
		// and (b) collide between two `:0` listeners on the same host. The
		// registration sits between rt.netLn assignment and the accept-loop
		// goroutine launch below, both of which depend on the post-resolve
		// rt.addr — keeping the three operations atomically ordered.
		registerListenerMetrics(m.registry, rt)
	}
	for _, rt := range m.runtimes {
		rt := rt
		// Capture rt.netLn here (under the held startedMu, before any concurrent
		// Stop can nil-write it) and pass it as a parameter to acceptLoop. Earlier
		// versions read rt.netLn from inside the goroutine, which raced with
		// Stop's nil-out under the race detector — surfaced by Task 10's
		// AllocatesTwoMetricsPerListener test which Start+Stop's a listener
		// without any intervening real-traffic delay.
		ln := rt.netLn
		go rt.acceptLoop(ctx, ln)
	}
	m.started = true
	return nil
}

// acceptLoop runs until the listener is closed (Stop) or ctx is canceled.
// For plaintext listeners, each accepted connection is handed to the single
// chain's filter.Handle in its own goroutine. For TLS listeners, each
// connection is wrapped and handed to serveTLS.
//
// Phase 06.1 (Task 10) hot-path discipline (SPEC §5.5): on each accepted
// connection Inc downstream_cx_total (counter, monotonic) AND downstream_cx_active
// (gauge, +1); the per-conn dispatch goroutine defers downstream_cx_active.Dec()
// so the gauge falls back when the filter's own deferred conn-close completes.
// The Inc/Dec discipline is exactly once per conn — Inc on accept, Dec when
// the dispatch goroutine returns.
//
// ln is captured by Start before launching the goroutine to keep the read off
// the rt.netLn field that Stop nil-writes (the race detector flags the in-loop
// read otherwise — surfaced by Task 10).
func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		raw, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("listener %q: accept: %v", rt.name, err)
			continue
		}
		// 06.1 hot path (SPEC §5.5): +2 LoC at the accept site; the matching
		// Dec is deferred in the per-conn dispatch goroutine below.
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		if !rt.tlsMode {
			// Phase-02-style: single chain, dispatch directly. The filter's
			// Handle owns the conn-close lifecycle; the deferred Dec here
			// fires after Handle returns (i.e., after the conn is closed).
			go func(c net.Conn) {
				defer rt.downstreamCxActive.Dec()
				rt.chains[0].filter.Handle(ctx, c)
			}(raw)
			continue
		}
		go func(c net.Conn) {
			defer rt.downstreamCxActive.Dec()
			rt.serveTLS(ctx, c)
		}(raw)
	}
}

// serveTLS wraps raw in a TLS server connection, performs the handshake, then
// dispatches to the correct chain via dispatch. Handshake errors are logged
// and the raw connection is closed.
func (rt *listenerRuntime) serveTLS(ctx context.Context, raw net.Conn) {
	tlsConn := stdtls.Server(raw, rt.tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		log.Printf("listener %q: handshake: %v", rt.name, err)
		_ = raw.Close()
		return
	}
	rt.dispatch(ctx, tlsConn)
}

// Listeners returns one Info per bound listener. Empty before Start or after a
// Start that errored out (the unwind clears every socket).
func (m *Manager) Listeners() []Info {
	out := make([]Info, 0, len(m.runtimes))
	for _, rt := range m.runtimes {
		if rt.netLn == nil {
			continue
		}
		out = append(out, Info{Name: rt.name, Addr: rt.netLn.Addr().String()})
	}
	return out
}

// Stop closes every bound listener socket. Idempotent. In-flight Handle
// goroutines are not waited on (drain semantics = phase 08).
func (m *Manager) Stop() {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	for _, rt := range m.runtimes {
		if rt.netLn != nil {
			_ = rt.netLn.Close()
			rt.netLn = nil
		}
	}
	m.started = false
}
