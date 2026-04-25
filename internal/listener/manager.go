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

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
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

// filterConstructor builds a filterHandler from a typed_config Any and the
// resolved cluster manager. Phase 02 has exactly one entry: tcp_proxy.
type filterConstructor func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error)

// filterRegistry maps a filter typed_config.type_url to its constructor.
// SPEC §5.3: inline; phase 07 generalises.
var filterRegistry = map[string]filterConstructor{
	tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error) {
		f, err := tcpproxy.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
	hcm.TypeURL: func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error) {
		f, err := hcm.NewFilter(tc, cm)
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
}

// Manager owns every listener materialized from static_resources.listeners[]
// and supervises their accept loops.
type Manager struct {
	runtimes  []*listenerRuntime
	startedMu sync.Mutex
	started   bool
}

// NewManager validates the listeners in bs against the phase-03 filter-chain
// subset (ADR-0033, supersedes ADR-0025) and constructs each listener's
// terminal filter(s) against cm. Returns an error on the first violation;
// subsequent listeners are not validated. No sockets are touched.
//
// Every error begins with "listener: ".
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager) (*Manager, error) {
	return NewManagerWithBaseDir(bs, cm, "")
}

// NewManagerWithBaseDir is the phase-03 variant of NewManager. baseDir is
// passed to internal/tls.NewDownstreamConfig so that filename-based
// DataSources in transport_socket are resolved relative to the config file
// location. Pass "" to resolve relative to the process working directory
// (phase-02 compat).
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string) (*Manager, error) {
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	m := &Manager{runtimes: make([]*listenerRuntime, 0, len(ls))}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		rt, err := buildListenerRuntime(l, i, cm, baseDir)
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

// buildListenerRuntime validates one Listener proto and constructs its
// listenerRuntime (including all chainInfo entries). No socket is bound here.
func buildListenerRuntime(l *listenerv3.Listener, idx int, cm *cluster.Manager, baseDir string) (*listenerRuntime, error) {
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
		fh, err := ctor(tc, cm)
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
	}
	for _, rt := range m.runtimes {
		rt := rt
		go rt.acceptLoop(ctx)
	}
	m.started = true
	return nil
}

// acceptLoop runs until the listener is closed (Stop) or ctx is canceled.
// For plaintext listeners, each accepted connection is handed to the single
// chain's filter.Handle in its own goroutine. For TLS listeners, each
// connection is wrapped and handed to serveTLS.
func (rt *listenerRuntime) acceptLoop(ctx context.Context) {
	// Capture netLn locally so Stop()'s nil-out does not race with Accept().
	ln := rt.netLn
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
		if !rt.tlsMode {
			// Phase-02-style: single chain, dispatch directly.
			go rt.chains[0].filter.Handle(ctx, raw)
			continue
		}
		go rt.serveTLS(ctx, raw)
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
