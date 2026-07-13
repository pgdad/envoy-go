package listener

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/filter/network/builtins"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
	"github.com/pgdad/envoy-go/internal/stats"
	internaltls "github.com/pgdad/envoy-go/internal/tls"
	xds "github.com/pgdad/envoy-go/internal/xds"
)

// Info is the after-Start description of one bound listener: the name
// from the bootstrap and the actually-bound socket address (resolved from the
// configured address; differs when the bootstrap requested port_value: 0).
type Info struct {
	Name string
	Addr string
}

// extractListenerPrincipal returns the listener TLS server-cert principal
// extracted from the chain's *stdtls.Config.Certificates[0] leaf certificate
// (parsed via x509.ParseCertificate on Certificate[0]). Priority order
// mirrors extractTLSPrincipals (used for the CLIENT-cert side at HCM
// dispatch — see internal/filter/hcm/connection.go): the FIRST URI SAN, then
// the FIRST DNS SAN, then the Subject DN Common Name. Returns "" for nil
// configs (plaintext chains), empty cert chains, parse failures, or certs
// carrying no matching identity.
//
// Per ADR-0165 §Decision (phase-18.2 Task 4 — the ADR-0044 escape-valve
// firing per planner-time decision D3 + D12). Pre-extracted at listener-
// build time into network.FactoryCtx.ListenerPrincipal → hcm.ListenerCtx →
// *Filter → chain.SetListenerPrincipal on every per-stream chain. Distinct from
// DownstreamPrincipal (which extracts the CLIENT cert at HCM dispatch via
// *tls.Conn.ConnectionState() — per-connection, not per-listener).
//
// Cross-phase reusable; ext_proc / global_ratelimit / future ext_authz
// extensions consume the same surface via DecoderFilterCallbacks.ListenerPrincipal().
func extractListenerPrincipal(cfg *stdtls.Config) string {
	if cfg == nil || len(cfg.Certificates) == 0 {
		return ""
	}
	c := cfg.Certificates[0]
	// Prefer the pre-parsed Leaf cert when available (some loaders populate
	// it). Otherwise parse Certificate[0] (the DER-encoded leaf cert).
	leaf := c.Leaf
	if leaf == nil {
		if len(c.Certificate) == 0 {
			return ""
		}
		parsed, err := x509.ParseCertificate(c.Certificate[0])
		if err != nil || parsed == nil {
			return ""
		}
		leaf = parsed
	}
	if ps := certPrincipals(leaf); len(ps) > 0 {
		return ps[0]
	}
	return ""
}

// certPrincipals returns the priority-ordered principals (URI SANs → DNS
// SANs → Subject CN) extracted from one *x509.Certificate. Shared by
// extractListenerPrincipal (which consumes only the first element — the
// listener SERVER-cert identity) and downstreamPrincipals (which consumes
// the full ordered list — the mTLS CLIENT-cert identities); both walks were
// previously duplicated inline with byte-identical output.
func certPrincipals(cert *x509.Certificate) []string {
	var principals []string
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		principals = append(principals, u.String())
	}
	principals = append(principals, cert.DNSNames...)
	if cn := cert.Subject.CommonName; cn != "" {
		principals = append(principals, cn)
	}
	return principals
}

// chainInfo holds the per-chain state built at NewManager time.
type chainInfo struct {
	serverNames []string       // from filter_chain_match.server_names; nil/empty = catch-all
	tlsCfg      *stdtls.Config // nil if plaintext chain
	// netChainFactory is the phase-26.2 (§3.3) unified network-filter chain
	// path: a closure capturing the boot-resolved []network.FilterInstanceFactory
	// that allocates a fresh []network.NetworkFilter per accepted connection.
	// Always non-nil post-Task-10 (netReg is the SOLE registry; every chain —
	// pure-read, pure-terminal, or mixed — is built through
	// buildNetworkChainFactory). serveNetworkChain classifies the per-conn chain
	// shape at dispatch time.
	netChainFactory func() []network.NetworkFilter
}

// listenerRuntime is the phase-03 replacement for builtListener. It holds
// everything needed to bind, accept, and dispatch connections on one listener.
//
// Phase 07.2 (Task 10): the unified pre/post-handshake dispatch path consults
// chainSpecs / defaultSpec / chainByName via listenerfilter.SelectChain. The
// legacy SNI-only `chains` slice + listener-level `tlsCfg` (with the
// GetConfigForClient SNI dispatch callback) introduced by phase 03 / preserved
// through Task 9 are gone — chain selection now happens BEFORE the TLS
// handshake, and each selected chainInfo carries its own per-chain
// *stdtls.Config that is passed directly to stdtls.Server.
type listenerRuntime struct {
	name    string
	addr    string
	netLn   net.Listener
	tlsMode bool
	// 07.2 (Task 9, ADR-0078) chain-match fields. chainSpecs and defaultSpec
	// describe the parsed `filter_chains[]` + `default_filter_chain` shape per
	// ADR-0081's 8-dimension algorithm; `chainByName` looks up the per-chain
	// `chainInfo` (filter handler, TLS config) by the winning ChainSpec's
	// Name. defaultChain is the per-connection terminal-filter state for
	// default_filter_chain.
	chainSpecs   []*listenerfilter.ChainSpec
	defaultSpec  *listenerfilter.ChainSpec
	defaultChain *chainInfo
	chainByName  map[string]*chainInfo
	// 07.2 (Task 9, ADR-0079) listener-filter pipeline plumbing. Populated
	// from listener_filters[] at build time; consumed by serveConnection's
	// per-conn pipeline. lfTimeoutMs is in [1000, 60000] (ADR-0082); default
	// 15000.
	listenerFilterFactories []listenerfilter.FilterInstanceFactory
	lfTimeoutMs             uint32
	continueOnLfTimeout     bool
	// lfPeekBufSize is the LARGEST listenerfilter.InitialReadBufferSizer hint
	// across the listener's listener_filters[] (tls_inspector's parsed
	// initial_read_buffer_size), probed once at build time. 0 = no filter
	// hinted; serveConnection then uses the default-size peek buffer. This is
	// what plumbs a configured initial_read_buffer_size > 4096 into the
	// per-connection peekerConn (previously the fixed-4096 NewPeekerConn made
	// larger configured sizes silently ineffective).
	lfPeekBufSize int
	// 06.1 metric fields (per SPEC §6 — listener-scope only, 2 metrics per
	// listener). Allocated by registerListenerMetrics at Start time (post-bind,
	// pre-Freeze) and Inc/Dec'd from the accept-loop hot path.
	downstreamCxTotal  *stats.Counter
	downstreamCxActive *stats.Gauge
	// 08.2 (Task 5) drain fast-path field. Field-local (not chasing back through
	// *Manager) to minimize hot-path indirection per ADR-0094.
	dm *drain.Manager
}

// Manager owns every listener materialized from static_resources.listeners[]
// and supervises their accept loops.
type Manager struct {
	runtimes  []*listenerRuntime
	registry  *stats.Registry // captured at NewManager; consumed at Start to register the 2 listener-scope metrics per resolved bind address
	dm        *drain.Manager  // 08.2 Task 5: central drain manager; nil if drain is not wired (legacy callers)
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
	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{ClusterManager: cm, StatsRegistry: registry, HTTPRegistry: httpRegistry})
	netReg.Freeze()
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, "", false, registry, nil, httpRegistry, nil, nil, nil, netReg, nil)
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
	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{ClusterManager: cm, StatsRegistry: registry, HTTPRegistry: httpRegistry})
	netReg.Freeze()
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, false, registry, nil, httpRegistry, nil, nil, nil, netReg, nil)
}

// NewManagerWithBaseDirAndAllowH2C is the phase-05.1 constructor variant. It
// threads the --allow-h2c boolean from cmd/envoy-go/main into each chain's
// network.FactoryCtx (AllowH2C), consumed by the HCM network-filter factory at
// build time. allowH2C permits HCM codec_type=HTTP2 on plaintext listeners (for
// h2spec conformance); default false.
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
//
// Phase 07.2 (Task 9, ADR-0078 + ADR-0079): the lfRegistry parameter is the
// boot-populated, frozen *listenerfilter.ListenerFilterRegistry threaded
// through to each per-listener loop for `listener_filters[]` type_url
// resolution. May be nil — but if any listener in bs has a non-empty
// `listener_filters[]`, a non-nil frozen registry is required (the parser
// errors otherwise).
// Phase 26.1 (Task 10, §3.5): netReg is the boot-populated, frozen network
// read-filter *network.Registry consulted by the build-time dual-dispatch
// pre-check at each filter-chain. When non-nil and a chain's filters[0]
// type_url resolves in netReg, the chain is built on the new read-filter path
// (chainInfo.netChainFactory) instead of the terminal-filter path; mixed
// read+terminal chains are boot-rejected. May be nil — the old terminal-filter
// path (tcp_proxy/HCM) is then taken for every chain (R4 back-compat).
func NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry, lfRegistry *listenerfilter.ListenerFilterRegistry, dm *drain.Manager, httpClient *httpclient.Client, netReg *network.Registry, sdsProvider xds.SecretProvider) (*Manager, error) {
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	// Phase 24.2 Task 1: extract the NODE's service-cluster name once at
	// manager-construction time and thread it through into each listener's
	// HCM filter via network.FactoryCtx.NodeServiceCluster → hcm.ListenerCtx.
	// Empty when the bootstrap omits `node.cluster`. Consumed by the ratelimit
	// filter's `source_cluster` descriptor action per parent SPEC §4.1 row 1.
	nodeServiceCluster := bs.GetNode().GetCluster()
	m := &Manager{runtimes: make([]*listenerRuntime, 0, len(ls)), registry: registry, dm: dm}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		rt, err := buildListenerRuntimeWithCtx(l, i, cm, baseDir, allowH2C, registry, accessLogSinks, httpRegistry, lfRegistry, dm, httpClient, nodeServiceCluster, netReg, sdsProvider)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[rt.name]; dup {
			return nil, fmt.Errorf("listener: duplicate listener name %q", rt.name)
		}
		seen[rt.name] = struct{}{}
		rt.dm = dm // 08.2 Task 5: propagate drain manager to each runtime for fast-path
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
// listenerRuntime (including all chainInfo entries, ChainSpecs, default chain,
// listener-filter factories, and lf-pipeline timeout). allowH2C is threaded
// into each chain's network.FactoryCtx consumed by the network-filter factories.
// registry is captured into the HCM network-filter factory for per-HCM-instance
// metric allocation. accessLogSinks are the opened AsyncFileSinks
// from main.go (Phase 06.2 Task 14); nil means no access logging.
// httpRegistry is the boot-populated, frozen *filter_http.HTTPRegistry
// captured into the HCM-factory closure for http_filters[] type_url
// resolution (Phase 07.1 Task 14, per ADR-0072). lfRegistry is the
// boot-populated, frozen *listenerfilter.ListenerFilterRegistry consulted to
// resolve `listener_filters[]` type_urls (Phase 07.2 Task 9, per ADR-0079);
// may be nil if no listener has any `listener_filters[]`. No socket is bound
// here.
//
// ADR-0078 supersession (Task 9): the previous phase-03 `validateFilterChainMatch`
// function has been replaced by `parseChainSpec`, which accepts all 8 chain-match
// dimensions per ADR-0081 and silently ignores `direct_source_prefix_ranges`
// per SPEC §12. The phase-03 parse-time errors on `default_filter_chain` and
// `listener_filters[]` (ADR-0033 clauses 3 and 8) are also superseded; both
// fields are now honored.
func buildListenerRuntimeWithCtx(l *listenerv3.Listener, idx int, cm *cluster.Manager, baseDir string, allowH2C bool, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter_http.HTTPRegistry, lfRegistry *listenerfilter.ListenerFilterRegistry, dm *drain.Manager, httpClient *httpclient.Client, nodeServiceCluster string, netReg *network.Registry, sdsProvider xds.SecretProvider) (*listenerRuntime, error) {
	name := l.GetName()
	if name == "" {
		return nil, fmt.Errorf("listener: listeners[%d]: missing name", idx)
	}
	addr := l.GetAddress().GetSocketAddress()
	if addr == nil {
		return nil, fmt.Errorf("listener: %q: address is not a socket_address", name)
	}

	chains := l.GetFilterChains()
	if len(chains) == 0 && l.GetDefaultFilterChain() == nil {
		// ADR-0078 clause-1 (PRESERVED with caveat): the union of
		// filter_chains[] and default_filter_chain must contribute at least
		// one chain.
		return nil, fmt.Errorf("listener: %q: filter_chains must be non-empty", name)
	}

	catchAllCount := 0
	anyTLS := false
	anyPlaintext := false
	anyServerNames := false
	var cis []*chainInfo
	var chainSpecs []*listenerfilter.ChainSpec
	chainByName := make(map[string]*chainInfo, len(chains))

	for i, fc := range chains {
		// ADR-0078 (Task 9): parse the FilterChainMatch into a ChainSpec
		// accepting all 8 dimensions per ADR-0081.
		specName := fmt.Sprintf("%s/filter_chains[%d]", name, i)
		spec, err := parseChainSpec(specName, fc.GetFilterChainMatch())
		if err != nil {
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, err)
		}
		serverNames := spec.ServerNames // empty = no SNI requirement
		if len(serverNames) > 0 {
			anyServerNames = true
		}

		// Decode transport_socket (nil = plaintext, non-nil = TLS).
		var chainTLS *stdtls.Config
		if ts := fc.GetTransportSocket(); ts != nil {
			// sdsProvider: nil at all pre-60.2 call sites (NewManager,
			// NewManagerWithBaseDir, and test callers), so an
			// SDS-bound tls_certificate_sds_secret_configs here still errors
			// ("requires a live SDS provider"), matching pre-60.2 behavior.
			dc, err := internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)
			if err != nil {
				return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, err)
			}
			chainTLS = dc.TLSConfig
			anyTLS = true
		} else {
			anyPlaintext = true
		}

		// Phase 26.2 (§3.3): build the unified network-filter chain. netReg is the
		// SOLE registry — every chain (pure-read, pure-terminal, or mixed) resolves
		// through buildNetworkChainFactory; an unknown filter type_url is the
		// unified boot-reject. Phase 18.2 Task 4 (ADR-0165): listenerPrincipal is
		// pre-extracted from the chain's leaf TLS cert (URI SAN[0] → DNS SAN[0] →
		// Subject CN); empty for plaintext chains.
		ncfPrefix := fmt.Sprintf("listener: %q: filter_chains[%d]", name, i)
		ncfCtx := network.FactoryCtx{
			BaseDir:            baseDir,
			HasTLS:             chainTLS != nil,
			AllowH2C:           allowH2C,
			ListenerPrincipal:  extractListenerPrincipal(chainTLS),
			NodeServiceCluster: nodeServiceCluster,
		}
		netChainFactory, err := buildNetworkChainFactory(ncfPrefix, fc.GetFilters(), netReg, ncfCtx)
		if err != nil {
			return nil, err
		}

		if spec.Empty {
			catchAllCount++
		}
		ci := &chainInfo{serverNames: serverNames, tlsCfg: chainTLS, netChainFactory: netChainFactory}
		cis = append(cis, ci)
		chainSpecs = append(chainSpecs, spec)
		chainByName[spec.Name] = ci
	}

	// ADR-0033 clause 5 / ADR-0078 clause-5 (PRESERVED): mixed TLS+plaintext
	// WITHIN filter_chains[] is rejected.
	if anyTLS && anyPlaintext {
		// Find the first plaintext chain for a clean error message.
		for i, ci := range cis {
			if ci.tlsCfg == nil {
				return nil, fmt.Errorf("listener: %q: filter_chains[%d]: mixed TLS and plaintext chains on one listener are not supported", name, i)
			}
		}
	}

	// ADR-0033 clause 6 / ADR-0078 clause-6 (PARTIALLY SUPERSEDED): a plaintext
	// listener with multiple chains is now permitted as long as no chain
	// populates server_names[] (SNI cannot match on plaintext). Only the
	// SNI-on-plaintext-multi-chain combination retains the legacy error.
	if !anyTLS && anyServerNames && len(cis) > 1 {
		return nil, fmt.Errorf("listener: %q: plaintext listener with multiple filter_chains is not supported (SNI match requires TLS)", name)
	}

	// ADR-0033: at most one catch-all chain per listener (preserved within
	// filter_chains[]; default_filter_chain is a separate slot per ADR-0080).
	if catchAllCount > 1 {
		return nil, fmt.Errorf("listener: %q: at most one filter_chain may omit filter_chain_match.server_names (catch-all); got %d", name, catchAllCount)
	}

	// ADR-0081 final-tie detection: scan for two ChainSpecs with identical
	// match shapes (Name excluded).
	if dupI, dupJ, dup := findIdenticalChainSpecs(chainSpecs); dup {
		return nil, fmt.Errorf("listener: %q: filter_chains[%d] and filter_chains[%d] have identical filter_chain_match — ambiguous selection", name, dupI, dupJ)
	}

	// ADR-0078 clause-3 (TOTALLY SUPERSEDED): parse default_filter_chain.
	var defaultSpec *listenerfilter.ChainSpec
	var defaultChain *chainInfo
	if dfc := l.GetDefaultFilterChain(); dfc != nil {
		var dfcTLS *stdtls.Config
		if ts := dfc.GetTransportSocket(); ts != nil {
			// sdsProvider: see the filter_chains[] call site above.
			dc, err := internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)
			if err != nil {
				return nil, fmt.Errorf("listener: %q: default_filter_chain: %w", name, err)
			}
			dfcTLS = dc.TLSConfig
		}
		// Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture
		// from filter_chains[] — no mixed-TLS-rule cross-check here.
		// Phase 26.2 (§3.3): build the default_filter_chain's unified
		// network-filter chain, mirroring the per-chain path above. The net-chain
		// error already carries the full prefix.
		dfcNetPrefix := fmt.Sprintf("listener: %q: default_filter_chain", name)
		dfcNetCtx := network.FactoryCtx{
			BaseDir:            baseDir,
			HasTLS:             dfcTLS != nil,
			AllowH2C:           allowH2C,
			ListenerPrincipal:  extractListenerPrincipal(dfcTLS),
			NodeServiceCluster: nodeServiceCluster,
		}
		dfcNetFactory, err := buildNetworkChainFactory(dfcNetPrefix, dfc.GetFilters(), netReg, dfcNetCtx)
		if err != nil {
			return nil, err
		}
		defaultSpec = &listenerfilter.ChainSpec{Name: name + "/default_filter_chain", Empty: true}
		defaultChain = &chainInfo{serverNames: nil, tlsCfg: dfcTLS, netChainFactory: dfcNetFactory}
		chainByName[defaultSpec.Name] = defaultChain
	}

	// ADR-0078 clause-8 (TOTALLY SUPERSEDED) + ADR-0079: parse listener_filters[].
	var lfFactories []listenerfilter.FilterInstanceFactory
	if lfs := l.GetListenerFilters(); len(lfs) > 0 {
		if lfRegistry == nil {
			return nil, fmt.Errorf("listener: %q: listener_filters[] is non-empty but no listener-filter registry was supplied", name)
		}
		lfFactories = make([]listenerfilter.FilterInstanceFactory, 0, len(lfs))
		for li, lf := range lfs {
			tc := lf.GetTypedConfig()
			typeURL := tc.GetTypeUrl()
			factory, ok := lfRegistry.Lookup(typeURL)
			if !ok {
				return nil, fmt.Errorf("listener: %q: listener_filters[%d]: unknown filter type_url %q", name, li, typeURL)
			}
			instFactory, err := factory(tc, listenerfilter.FactoryCtx{})
			if err != nil {
				return nil, fmt.Errorf("listener: %q: listener_filters[%d]: %w", name, li, err)
			}
			lfFactories = append(lfFactories, instFactory)
		}
	}

	// Probe each per-connection filter ONCE at build time for the optional
	// InitialReadBufferSizer hint and keep the largest value; serveConnection
	// sizes the per-connection peek buffer accordingly so a ClientHello larger
	// than the 4096 default is peekable in full. The sample instances are
	// discarded immediately (OnDestroy honors the per-connection lifecycle).
	lfPeekBufSize := 0
	for _, fac := range lfFactories {
		sample := fac()
		if h, ok := sample.(listenerfilter.InitialReadBufferSizer); ok {
			if v := h.InitialReadBufferSize(); v > lfPeekBufSize {
				lfPeekBufSize = v
			}
		}
		sample.OnDestroy()
	}

	// ADR-0082: parse listener_filters_timeout (default 15000ms; envelope
	// [1000, 60000]).
	lfTimeoutMs, err := parseListenerFiltersTimeout(name, l.GetListenerFiltersTimeout())
	if err != nil {
		return nil, err
	}

	// Phase-07.2 Task 10: unified pre/post-handshake dispatch consumes
	// chainSpecs / defaultSpec / chainByName directly via
	// listenerfilter.SelectChain — there is no listener-level *stdtls.Config
	// (each chainInfo carries its own per-chain config) and no legacy chain
	// ordering. The `cis` accumulator is consumed only by the build-time
	// mixed-TLS check above; chainByName is the canonical post-build
	// chainInfo lookup.

	rt := &listenerRuntime{
		name:                    name,
		addr:                    fmt.Sprintf("%s:%d", addr.GetAddress(), addr.GetPortValue()),
		tlsMode:                 anyTLS,
		chainSpecs:              chainSpecs,
		defaultSpec:             defaultSpec,
		defaultChain:            defaultChain,
		chainByName:             chainByName,
		listenerFilterFactories: lfFactories,
		lfTimeoutMs:             lfTimeoutMs,
		continueOnLfTimeout:     l.GetContinueOnListenerFiltersTimeout(),
		lfPeekBufSize:           lfPeekBufSize,
	}
	return rt, nil
}

// buildNetworkChainFactory is the phase-26.2 (§3.3/§3.6) unified network-filter
// chain builder — the SOLE chain-build path post-Task-10 (netReg is the only
// registry; the old inline terminal-filter registry path is retired). prefix
// is the chain's error prefix (e.g. `listener: %q: filter_chains[%d]` or
// `listener: %q: default_filter_chain`). fctx is the PER-CHAIN
// network.FactoryCtx supplied by the caller (BaseDir + the Task-3 per-chain
// terminal-build fields HasTLS/AllowH2C/ListenerPrincipal/NodeServiceCluster),
// threaded into each NetworkFilterFactory.
//
// netReg must be non-nil and filters non-empty (a missing or empty filters[]
// is the documented `non-empty filter chain required` boot error). EVERY filter
// in filters is resolved against netReg, each NetworkFilterFactory invoked ONCE
// at boot (validating typed_config and yielding a per-conn FilterInstanceFactory).
// Any unresolved type_url — including filters[0] — is the UNIFIED unknown-type-url
// reject, byte-identical to the wording the retired terminal path emitted.
// The classified chain is validated against the [read*, terminal?] shape via a
// one-shot sample chain: a terminal filter must be last
// (network-filter-terminal-not-last) and unique (network-filter-multiple-terminals).
// On success it returns the netChainFactory closure that allocates a fresh
// []network.NetworkFilter per accepted connection.
func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netReg *network.Registry, fctx network.FactoryCtx) (func() []network.NetworkFilter, error) {
	if netReg == nil {
		return nil, fmt.Errorf("%s: no network-filter registry was supplied", prefix)
	}
	if len(filters) == 0 {
		return nil, fmt.Errorf("%s: a non-empty filter chain is required", prefix)
	}
	// Resolve EVERY filter against netReg and invoke each factory once at boot.
	insts := make([]network.FilterInstanceFactory, 0, len(filters))
	for idx, f := range filters {
		tc := f.GetTypedConfig()
		tu := tc.GetTypeUrl()
		factory, ok := netReg.Lookup(tu)
		if !ok {
			// Any unresolved type_url (filters[0] or later) is the unified
			// unknown-type-url reject — byte-identical to the retired terminal
			// path's wording.
			return nil, fmt.Errorf("%s: unknown filter type_url %q", prefix, tu)
		}
		instFactory, err := factory(tc, fctx)
		if err != nil {
			return nil, fmt.Errorf("%s: filters[%d]: %w", prefix, idx, err)
		}
		insts = append(insts, instFactory)
	}
	// Classify + validate the [read*, terminal?] shape on a one-shot sample
	// chain (the per-conn allocations are discarded; only their dynamic types
	// matter). A terminal filter must be unique and last.
	sample := make([]network.NetworkFilter, len(insts))
	for i, mk := range insts {
		sample[i] = mk()
	}
	// Pass 1: classify every filter and detect a second terminal
	// (network-filter-multiple-terminals takes precedence — a chain carrying two
	// terminals is rejected on the second terminal regardless of position).
	seenTerminal := false
	for idx, nf := range sample {
		switch nf.(type) {
		case network.TerminalFilter:
			if seenTerminal {
				return nil, fmt.Errorf("%s: network-filter-multiple-terminals: filters[%d] is a second terminal filter (a chain may carry at most one tcp_proxy/HCM)", prefix, idx)
			}
			seenTerminal = true
		case network.ReadFilter:
			// read filters are positionally validated in pass 2
		default:
			return nil, fmt.Errorf("%s: filters[%d]: resolved filter is neither a read nor a terminal network filter", prefix, idx)
		}
	}
	// Pass 2: the single terminal (if any) must be last in the chain
	// (network-filter-terminal-not-last — a terminal owns the connection; nothing
	// may follow it).
	for idx, nf := range sample {
		if _, ok := nf.(network.TerminalFilter); ok && idx != len(sample)-1 {
			return nil, fmt.Errorf("%s: network-filter-terminal-not-last: filters[%d] is a terminal filter but is not last in the chain (a terminal filter owns the connection; nothing may follow it)", prefix, idx)
		}
	}
	netChainFactory := func() []network.NetworkFilter {
		rf := make([]network.NetworkFilter, len(insts))
		for i, mk := range insts {
			rf[i] = mk()
		}
		return rf
	}
	return netChainFactory, nil
}

// parseListenerFiltersTimeout parses Listener.listener_filters_timeout per
// ADR-0082: nil/zero defaults to 15000ms; values outside [1000, 60000]ms
// error.
func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error) {
	const defaultMs = 15000
	if d == nil || (d.GetSeconds() == 0 && d.GetNanos() == 0) {
		return defaultMs, nil
	}
	total := d.AsDuration()
	ms := total / time.Millisecond
	if ms < 1000 || ms > 60000 {
		return 0, fmt.Errorf("listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope", name, total)
	}
	return uint32(ms), nil
}

// parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
// package's *ChainSpec, accepting all 8 dimensions per ADR-0081 + ADR-0078.
// `direct_source_prefix_ranges` is silently ignored per SPEC §12. A nil or
// all-zero `fm` produces an Empty ChainSpec (universally eligible per
// ADR-0080).
func parseChainSpec(name string, fm *listenerv3.FilterChainMatch) (*listenerfilter.ChainSpec, error) {
	if fm == nil {
		return &listenerfilter.ChainSpec{Name: name, Empty: true}, nil
	}
	spec := &listenerfilter.ChainSpec{Name: name}
	// destination_port: 0 means unspecified.
	if dp := fm.GetDestinationPort(); dp != nil {
		spec.DestinationPort = dp.GetValue()
	}
	// prefix_ranges → []*net.IPNet.
	for ri, cr := range fm.GetPrefixRanges() {
		ipnet, err := parseCidrRange(cr)
		if err != nil {
			return nil, fmt.Errorf("prefix_ranges[%d]: %w", ri, err)
		}
		spec.PrefixRanges = append(spec.PrefixRanges, ipnet)
	}
	// server_names: copy slice (preserve nil-vs-empty distinction is
	// unnecessary; chainmatch consumes len() only).
	if sn := fm.GetServerNames(); len(sn) > 0 {
		spec.ServerNames = append([]string(nil), sn...)
	}
	// transport_protocol: validate against the v3 enum domain.
	switch tp := fm.GetTransportProtocol(); tp {
	case "", "tls", "raw_buffer":
		spec.TransportProtocol = tp
	default:
		return nil, fmt.Errorf("transport_protocol %q must be \"tls\" or \"raw_buffer\" or empty", tp)
	}
	// application_protocols: copy slice.
	if ap := fm.GetApplicationProtocols(); len(ap) > 0 {
		spec.ApplicationProtocols = append([]string(nil), ap...)
	}
	// source_type: ANY (0) → both flags false; SAME_IP_OR_LOOPBACK (1) →
	// SourceTypeLocal=true; EXTERNAL (2) → SourceTypeExternal=true.
	switch fm.GetSourceType() {
	case listenerv3.FilterChainMatch_ANY:
		// default: both flags stay false.
	case listenerv3.FilterChainMatch_SAME_IP_OR_LOOPBACK:
		spec.SourceTypeLocal = true
	case listenerv3.FilterChainMatch_EXTERNAL:
		spec.SourceTypeExternal = true
	}
	// source_prefix_ranges → []*net.IPNet.
	for ri, cr := range fm.GetSourcePrefixRanges() {
		ipnet, err := parseCidrRange(cr)
		if err != nil {
			return nil, fmt.Errorf("source_prefix_ranges[%d]: %w", ri, err)
		}
		spec.SourcePrefixRanges = append(spec.SourcePrefixRanges, ipnet)
	}
	// source_ports: copy slice.
	if sp := fm.GetSourcePorts(); len(sp) > 0 {
		spec.SourcePorts = append([]uint32(nil), sp...)
	}
	// direct_source_prefix_ranges: silently ignored per SPEC §12.

	if isAllZeroChainSpec(spec) {
		spec.Empty = true
	}
	return spec, nil
}

// parseCidrRange decodes a corev3.CidrRange into a *net.IPNet. The proto's
// `address_prefix` is the IP and `prefix_len` is the mask bit count. An
// empty/invalid IP or out-of-range prefix length returns an error.
func parseCidrRange(cr *corev3.CidrRange) (*net.IPNet, error) {
	if cr == nil {
		return nil, fmt.Errorf("nil CidrRange")
	}
	addr := cr.GetAddressPrefix()
	prefix := cr.GetPrefixLen().GetValue()
	if addr == "" {
		return nil, fmt.Errorf("address_prefix is empty")
	}
	cidr := fmt.Sprintf("%s/%d", addr, prefix)
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	return ipnet, nil
}

// isAllZeroChainSpec reports whether spec has zero values on every match
// dimension — the canonical empty/catch-all case where Pass-1 eligibility
// is universal.
func isAllZeroChainSpec(spec *listenerfilter.ChainSpec) bool {
	return spec.DestinationPort == 0 &&
		len(spec.PrefixRanges) == 0 &&
		len(spec.ServerNames) == 0 &&
		spec.TransportProtocol == "" &&
		len(spec.ApplicationProtocols) == 0 &&
		!spec.SourceTypeLocal &&
		!spec.SourceTypeExternal &&
		len(spec.SourcePrefixRanges) == 0 &&
		len(spec.SourcePorts) == 0
}

// findIdenticalChainSpecs returns the indexes of two ChainSpecs with
// identical match shapes (Name excluded) per ADR-0081's "final ties at
// NewManager-build time error" rule. Returns (i, j, true) on detection;
// (0, 0, false) otherwise.
func findIdenticalChainSpecs(specs []*listenerfilter.ChainSpec) (int, int, bool) {
	keys := make([]string, len(specs))
	for i, s := range specs {
		keys[i] = chainSpecKey(s)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] == keys[j] {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// chainSpecKey deterministically serializes a ChainSpec's match dimensions
// (Name excluded) into a comparable string. Used by findIdenticalChainSpecs
// for ambiguous-selection detection at NewManager-build time.
//
// Each multi-element field is order-canonicalized (sorted on a copy — the
// input *ChainSpec is documented immutable post-build) before serialization
// so that two chains differing only in slice declaration order — which
// matches() treats as semantically identical because every multi-element
// dimension is set-based (sniMatchAny / alpnMatchAny / portInAny / ipInAny)
// — produce the same key. Without this, the boot-time duplicate-detection
// would miss the duplicate, and at runtime SelectChain.breakTie would return
// nil → ErrAmbiguousChainMatch on the first matching connection.
func chainSpecKey(s *listenerfilter.ChainSpec) string {
	if s.Empty {
		return "EMPTY"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "dp=%d|", s.DestinationPort)
	b.WriteString("pr=")
	prefixRanges := make([]string, len(s.PrefixRanges))
	for i, n := range s.PrefixRanges {
		prefixRanges[i] = n.String()
	}
	sort.Strings(prefixRanges)
	for _, n := range prefixRanges {
		b.WriteString(n)
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString("sn=")
	serverNames := append([]string(nil), s.ServerNames...)
	sort.Strings(serverNames)
	for _, sn := range serverNames {
		b.WriteString(sn)
		b.WriteByte(',')
	}
	b.WriteByte('|')
	fmt.Fprintf(&b, "tp=%s|", s.TransportProtocol)
	b.WriteString("ap=")
	appProtos := append([]string(nil), s.ApplicationProtocols...)
	sort.Strings(appProtos)
	for _, ap := range appProtos {
		b.WriteString(ap)
		b.WriteByte(',')
	}
	b.WriteByte('|')
	fmt.Fprintf(&b, "stl=%t|", s.SourceTypeLocal)
	fmt.Fprintf(&b, "ste=%t|", s.SourceTypeExternal)
	b.WriteString("spr=")
	srcPrefixRanges := make([]string, len(s.SourcePrefixRanges))
	for i, n := range s.SourcePrefixRanges {
		srcPrefixRanges[i] = n.String()
	}
	sort.Strings(srcPrefixRanges)
	for _, n := range srcPrefixRanges {
		b.WriteString(n)
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString("sp=")
	srcPorts := append([]uint32(nil), s.SourcePorts...)
	sort.Slice(srcPorts, func(i, j int) bool { return srcPorts[i] < srcPorts[j] })
	for _, p := range srcPorts {
		fmt.Fprintf(&b, "%d,", p)
	}
	return b.String()
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
// Each accepted connection is dispatched to serveConnection in its own
// goroutine (single-goroutine-per-connection per SPEC §5.6).
//
// Phase 06.1 (Task 10) hot-path discipline (SPEC §5.5): on each accepted
// connection Inc downstream_cx_total (counter, monotonic) AND
// downstream_cx_active (gauge, +1); serveConnection defers
// downstream_cx_active.Dec() so the gauge falls back when the per-conn
// pipeline + dispatch returns. Inc/Dec is exactly once per conn — Inc here
// on accept, Dec on serveConnection return.
//
// Phase 07.2 (Task 10): the previous TLS / non-TLS branching is gone — the
// unified serveConnection helper runs (1) listener-filter pipeline → (2)
// chain-match → (3) handshake (if the selected chain has TLS) → (4)
// terminal-filter dispatch on every connection regardless of TLS posture.
//
// ln is captured by Start before launching the goroutine to keep the read off
// the rt.netLn field that Stop nil-writes (the race detector flags the in-loop
// read otherwise — surfaced by Task 10).
func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
	// backoff paces retries on persistent non-fatal Accept errors (e.g.
	// EMFILE fd exhaustion): 5ms doubling to a 100ms cap, reset on the next
	// successful Accept — the standard net/http.Server pattern. Without it
	// the loop hot-spins, flooding the log and pegging a CPU exactly when
	// the process is resource-starved.
	var backoff time.Duration
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
			if backoff == 0 {
				backoff = 5 * time.Millisecond
			} else {
				backoff *= 2
				if backoff > 100*time.Millisecond {
					backoff = 100 * time.Millisecond
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}
		backoff = 0
		// 08.2 (Task 5) drain fast-path per ADR-0094 + SPEC §11.5: on drain,
		// the accepted conn is immediately closed without filter-chain
		// dispatch (TCP handshake completes; the client observes accept-then-
		// FIN — "Empty reply from server" per curl error 52). The 06.1 +2-LoC
		// accept-site Inc lines are NOT executed for the drained-conn case
		// (the conn never enters serveConnection).
		if rt.dm != nil && rt.dm.IsDraining() {
			_ = raw.Close()
			continue
		}
		// 06.1 hot path (SPEC §5.5): +2 LoC at the accept site; the matching
		// Dec is deferred in serveConnection.
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		go rt.serveConnection(ctx, raw)
	}
}

// serveConnection is the per-connection unified pre/post-handshake dispatch
// helper introduced by phase 07.2 Task 10 (SPEC §5.2 lifecycle). The 8 steps:
//
//  1. Build ChainMatchInputs from the connection's local/remote addresses.
//  2. Wrap raw in a peekerConn so listener filters can Peek without consuming
//     (skipped when the listener has zero listener_filters[]; sized to the
//     largest build-time InitialReadBufferSizer hint when filters exist).
//  3. Construct per-connection ListenerFilter instances from the per-listener
//     factory slice (one allocation per filter per connection).
//  4. Run the listener-filter pipeline; on error, honor
//     `continue_on_listener_filters_timeout` — false aborts the connection,
//     true falls through with whatever inputs were populated so far.
//  5. Run listenerfilter.SelectChain over chainSpecs / defaultSpec; abort on
//     ErrNoChainMatched or ErrAmbiguousChainMatch.
//  6. If the selected chain has TLS, hand the peekerConn to stdtls.Server with
//     the chain's per-chain *stdtls.Config and run HandshakeContext; abort on
//     handshake error.
//  7. Dispatch to the selected chain's terminal filter.Handle; the filter
//     owns the conn-close lifecycle.
//
// The deferred downstreamCxActive.Dec() at the top of the function fires when
// the per-conn pipeline + dispatch goroutine returns (i.e., when Handle
// returns or any of the abort paths runs); the matching Inc happens in
// acceptLoop.
//
// Per ADR-0079 the listener-filter Pipeline.Run already calls OnDestroy on
// every constructed filter instance via its own deferred loop (pipeline.go
// lines 33-37) — the helper does NOT need to re-defer that here.
func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
	defer rt.downstreamCxActive.Dec()

	// (1) ChainMatchInputs from connection-level facts.
	inputs := listenerfilter.ChainMatchInputs{
		DestinationIP:   localIP(raw),
		DestinationPort: localPort(raw),
		SourceIP:        remoteIP(raw),
		SourcePort:      remotePort(raw),
	}

	// (2) Wrap in peekerConn so listener filters can read without consuming.
	// The wrap is UNCONDITIONAL even with zero listener_filters[]: the wrapper
	// deliberately does NOT implement SetLinger, so the serveNetworkChain
	// NoFlush close path's SO_LINGER-0 (RST) branch stays dormant and a
	// NoFlush close surfaces to the client as a plain FIN — socket-level
	// behavior pinned by the 0043-network-rbac differential (deny verdict
	// closed_no_bytes, not a reset). When filters exist, the buffer is sized
	// to the largest build-time InitialReadBufferSizer hint (tls_inspector's
	// initial_read_buffer_size), defaulting to 4096 when no filter hinted.
	var pkConn net.Conn
	if rt.lfPeekBufSize > 0 {
		pkConn = listenerfilter.NewPeekerConnSize(raw, rt.lfPeekBufSize)
	} else {
		pkConn = listenerfilter.NewPeekerConn(raw)
	}
	peeker := listenerfilter.AsPeeker(pkConn)

	// (3) Construct per-connection ListenerFilter instances from factories.
	filters := make([]listenerfilter.ListenerFilter, len(rt.listenerFilterFactories))
	for i, fac := range rt.listenerFilterFactories {
		filters[i] = fac()
	}

	// (4) Run listener-filter pipeline.
	var p listenerfilter.Pipeline
	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
		if !rt.continueOnLfTimeout {
			log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, err)
			_ = pkConn.Close()
			return
		}
		// continue_on_listener_filters_timeout=true: fall through with partial inputs.
	}

	// (5) Run chain-match algorithm.
	selectedSpec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)
	if err != nil {
		log.Printf("listener %q: chain-match: %v", rt.name, err)
		_ = pkConn.Close()
		return
	}
	selected := rt.chainByName[selectedSpec.Name]
	if selected == nil {
		// Defensive — chainByName is populated for every ChainSpec at build time;
		// reaching here would be a logic bug, not a runtime path.
		log.Printf("listener %q: chain-match: no chainInfo for selected spec %q (logic bug)", rt.name, selectedSpec.Name)
		_ = pkConn.Close()
		return
	}

	// (6) If selected chain has TLS, run handshake.
	var dispatchConn net.Conn = pkConn
	if selected.tlsCfg != nil {
		tlsConn := stdtls.Server(pkConn, selected.tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			log.Printf("listener %q: handshake: %v", rt.name, err)
			_ = pkConn.Close()
			return
		}
		dispatchConn = tlsConn
	}

	// (7) Dispatch through the unified network-filter chain. netReg is the SOLE
	// registry post-Task-10, so netChainFactory is always non-nil; serveNetworkChain
	// classifies the per-conn chain shape (pure-read / pure-terminal / mixed).
	rt.serveNetworkChain(ctx, dispatchConn, *selected)
}

// serveNetworkChain drives the phase-26.2 (§3.3) unified network-filter chain
// over the dispatch net.Conn. It builds the per-connection []NetworkFilter from
// the chain's boot-resolved factory, constructs the exported
// network.ChainRuntime with the connection's extracted SNI / mTLS principals /
// addresses, then dispatches one of the three chain shapes the unified runner
// classifies (SPEC §3.3):
//
//   - PURE-TERMINAL (tcp_proxy, HCM via netReg): TerminalReady() is true before
//     any read → immediate HandleTerminal — byte-identical to the retired
//     selected.filter.Handle(ctx, dispatchConn). The terminal owns conn-close.
//   - PURE-READ (echo, direct_response): no terminal → the phase-26.1 read loop,
//     unchanged: eager OnNewConnection, then loop feeding socket reads into
//     OnData until a filter requests close (D-P26.1-5a) or the downstream
//     EOFs/errors; this helper then owns the socket close.
//   - MIXED (read* → terminal): the read loop runs until a read filter Continues
//     past itself to the terminal (TerminalReady), at which point control hands
//     off to HandleTerminal. 26.2 has no production read filter that Continues
//     to a terminal (echo halts; direct_response closes), so this branch is
//     exercised only by unit tests (chain_test.go); rbac_network (26.3) is the
//     first production mixed chain.
//
// Pure-read CloseRequested() is checked twice per iteration so a filter that
// closes in OnNewConnection (direct_response) exits before the first blocking
// Read, and a filter that closes during OnData (after writing its response)
// exits before the next Read. echo never closes, so its loop runs until the
// downstream half-closes (EOF → OnData(nil, true) → break).
func (rt *listenerRuntime) serveNetworkChain(ctx context.Context, dispatchConn net.Conn, selected chainInfo) {
	facts := network.ConnFacts{
		ServerName: requestedServerName(dispatchConn, selected),
		Principals: downstreamPrincipals(dispatchConn),
		Local:      dispatchConn.LocalAddr(),
		Remote:     dispatchConn.RemoteAddr(),
	}
	filters := selected.netChainFactory()
	rtChain := network.NewChainRuntime(filters, dispatchConn, facts)
	if rt.dm != nil { // 29.3 (§3.4): the drain decider for cx_drain_close.
		// GUARD MANDATORY (risk register line 1846): rt.dm is *drain.Manager and MAY
		// be nil (legacy/test callers). Storing a typed-nil *drain.Manager into the
		// DrainState interface yields a NON-nil interface → Draining()'s `!= nil`
		// guard passes → IsDraining() panics on a nil receiver. The `if rt.dm != nil`
		// guard keeps rt.drain a true-nil interface so Draining() returns false.
		rtChain.SetDrainState(rt.dm)
	}
	defer rtChain.OnDestroy()

	// Pure-terminal chain: hand off immediately — byte-identical to the retired
	// selected.filter.Handle(ctx, dispatchConn). HandleTerminal returns BEFORE
	// the pure-read dispatchConn.Close() below so the terminal's own
	// defer conn.Close() owns the close lifecycle (R3).
	if rtChain.TerminalReady() {
		rtChain.HandleTerminal(ctx)
		return
	}

	rtChain.OnNewConnection()
	if rtChain.TerminalReady() { // a read filter Continued to the terminal in OnNewConnection
		rtChain.HandleTerminal(ctx)
		return
	}

	buf := make([]byte, 16*1024)
	for {
		if rtChain.CloseRequested() {
			break
		}
		n, err := dispatchConn.Read(buf)
		if n > 0 {
			rtChain.OnData(buf[:n], false)
		}
		// Deliberate precedence (Task-4 review): a terminal handoff wins over a
		// pending close. A filter that both Continued-to-terminal AND requested
		// close is contradictory; 26.2 has no such production read filter (echo
		// halts; direct_response StopIterations+closes, neither Continues to a
		// terminal), so this fires only for mixed chains (rbac_network, 26.3),
		// unit-tested in chain_test.go.
		if rtChain.TerminalReady() {
			rtChain.HandleTerminal(ctx)
			return // terminal owns the conn-close lifecycle
		}
		if rtChain.CloseRequested() {
			break
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				rtChain.OnData(nil, true)
			}
			break
		}
	}
	// Honor the close semantics the filter requested (F3, D-26.3-2).
	// FlushWrite (the zero value) drains then closes; NoFlush is
	// minimal-correct per D-26.3-2 (SO_LINGER 0 → RST, discarding any pending
	// write), set best-effort on a *net.TCPConn before Close. Socket-level
	// parity is fixture-verified by the Task-14 differential.
	if rtChain.CloseType() == network.NoFlush {
		if tc, ok := dispatchConn.(interface{ SetLinger(int) error }); ok {
			_ = tc.SetLinger(0)
		}
	}
	_ = dispatchConn.Close() // pure-read close (echo/direct_response); terminal path returned above
}

// requestedServerName returns the SNI for the new-path ConnFacts. On a TLS
// chain the dispatch conn is a *stdtls.Conn whose ConnectionState().ServerName
// is the client-offered SNI; on plaintext it is the empty string (no SNI). The
// chain's static server_names[] (the filter_chain_match SNI requirement) is NOT
// the connection's SNI, so it is only used as a fallback when the conn is not a
// *stdtls.Conn but the chain matched on a single server_name.
func requestedServerName(conn net.Conn, selected chainInfo) string {
	if tc, ok := conn.(*stdtls.Conn); ok {
		return tc.ConnectionState().ServerName
	}
	if len(selected.serverNames) == 1 {
		return selected.serverNames[0]
	}
	return ""
}

// downstreamPrincipals returns the priority-ordered mTLS peer principals (URI
// SANs → DNS SANs → Subject CN) from the dispatch conn's *stdtls.Conn client
// certificate, or nil for plaintext / no-client-cert. Mirrors the HCM-path
// extraction (internal/filter/hcm/connection.go extractTLSPrincipals) so the
// L4 Connection().DownstreamPrincipals surface matches the L7 one.
func downstreamPrincipals(conn net.Conn) []string {
	tc, ok := conn.(*stdtls.Conn)
	if !ok {
		return nil
	}
	state := tc.ConnectionState()
	if !state.HandshakeComplete || len(state.PeerCertificates) == 0 {
		return nil
	}
	return certPrincipals(state.PeerCertificates[0])
}

// localIP / localPort / remoteIP / remotePort extract the IP+port pieces from
// a net.Conn's LocalAddr / RemoteAddr. Currently only TCP addresses are
// observed in the listener pipeline — the type-assertion to *net.TCPAddr
// returns the zero value (nil IP, port 0) for any other address kind, which
// the chain-match algorithm treats as unmatched on every IP/port dimension.
func localIP(c net.Conn) net.IP {
	if a, ok := c.LocalAddr().(*net.TCPAddr); ok {
		return a.IP
	}
	return nil
}

func localPort(c net.Conn) uint32 {
	if a, ok := c.LocalAddr().(*net.TCPAddr); ok {
		return uint32(a.Port)
	}
	return 0
}

func remoteIP(c net.Conn) net.IP {
	if a, ok := c.RemoteAddr().(*net.TCPAddr); ok {
		return a.IP
	}
	return nil
}

func remotePort(c net.Conn) uint32 {
	if a, ok := c.RemoteAddr().(*net.TCPAddr); ok {
		return uint32(a.Port)
	}
	return 0
}

// Listeners returns one Info per bound listener. Empty before Start or after a
// Start that errored out (the unwind clears every socket).
//
// Order is bootstrap-declaration order; callers needing alphabetical ordering
// must sort. (Per 08.1 REVIEW N-1 carry-forward, landed inline in 08.2 Task 5
// per SPEC §10.2.)
//
// startedMu is held for the walk: rt.netLn is nil-written by Stop() and
// assigned by Start() under the same mutex, and Listeners() is called from
// the admin HTTP handlers concurrently with shutdown — the unlocked
// nil-check-then-Addr() sequence both raced and could nil-panic if Stop ran
// between the two reads.
func (m *Manager) Listeners() []Info {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	out := make([]Info, 0, len(m.runtimes))
	for _, rt := range m.runtimes {
		if rt.netLn == nil {
			continue
		}
		out = append(out, Info{Name: rt.name, Addr: rt.netLn.Addr().String()})
	}
	return out
}

// Drain transitions the manager to drain mode by calling m.dm.Drain()
// (delegates to the central drain.Manager). The per-runtime Accept loops
// already check m.dm.IsDraining() at the top of each iteration; once
// Drain has been called, the next Accept return is the first conn that
// gets the accept-then-FIN treatment per SPEC §11.5.
//
// Idempotent — calling Drain multiple times is safe (delegates to the
// sync.Once-guarded drain.Manager.Drain).
//
// This method does NOT close the listening sockets. Existing in-flight
// downstream connections continue running their HCM filter chains to
// completion. The post-drain teardown is Stop(), invoked from the
// deferred-stop chain in cmd/envoy-go/main.go AFTER <-drainMgr.Done().
//
// Phase 08.2 (Task 5) introduces this accessor; ADR-0094 records the design.
func (m *Manager) Drain() {
	if m.dm != nil {
		m.dm.Drain()
	}
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
