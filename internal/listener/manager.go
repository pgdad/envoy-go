package listener

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
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
}

// builtListener is an internal record between NewManager (build) and Start
// (bind + serve). The filter is constructed at NewManager time so that bad
// configurations fail before any socket is touched.
type builtListener struct {
	name     string
	bindHost string
	bindPort uint32
	filter   filterHandler

	// Set during Start.
	socket net.Listener
}

// Manager owns every listener materialized from static_resources.listeners[]
// and supervises their accept loops.
type Manager struct {
	built     []*builtListener
	startedMu sync.Mutex
	started   bool
}

// NewManager validates the listeners in bs against the phase-02 filter-chain
// subset (ADR-0025) and constructs each listener's terminal filter against cm.
// Returns an error on the first violation; subsequent listeners are not
// validated. No sockets are touched during NewManager.
//
// Every error begins with "listener: ".
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager) (*Manager, error) {
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	m := &Manager{built: make([]*builtListener, 0, len(ls))}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		bl, err := buildListener(l, i, cm)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[bl.name]; dup {
			return nil, fmt.Errorf("listener: duplicate listener name %q", bl.name)
		}
		seen[bl.name] = struct{}{}
		m.built = append(m.built, bl)
	}
	return m, nil
}

func buildListener(l *listenerv3.Listener, idx int, cm *cluster.Manager) (*builtListener, error) {
	name := l.GetName()
	if name == "" {
		return nil, fmt.Errorf("listener: listeners[%d]: missing name", idx)
	}
	addr := l.GetAddress().GetSocketAddress()
	if addr == nil {
		return nil, fmt.Errorf("listener: %q: address is not a socket_address", name)
	}
	chains := l.GetFilterChains()
	if len(chains) != 1 {
		return nil, fmt.Errorf("listener: %q: expected exactly one filter_chain, got %d", name, len(chains))
	}
	chain := chains[0]
	if m := chain.GetFilterChainMatch(); m != nil && !proto.Equal(m, &listenerv3.FilterChainMatch{}) {
		return nil, fmt.Errorf("listener: %q: filter_chain_match must be empty in phase 02 (full match protocol = phase 07)", name)
	}
	if chain.GetTransportSocket() != nil {
		return nil, fmt.Errorf("listener: %q: transport_socket is not supported in phase 02 (TLS = phase 03)", name)
	}
	filters := chain.GetFilters()
	if len(filters) != 1 {
		return nil, fmt.Errorf("listener: %q: expected exactly one filter, got %d", name, len(filters))
	}
	tc := filters[0].GetTypedConfig()
	if tc == nil {
		return nil, fmt.Errorf("listener: %q: filter typed_config is nil", name)
	}
	ctor, ok := filterRegistry[tc.GetTypeUrl()]
	if !ok {
		return nil, fmt.Errorf("listener: %q: unknown filter type_url %q", name, tc.GetTypeUrl())
	}
	fh, err := ctor(tc, cm)
	if err != nil {
		return nil, fmt.Errorf("listener: %q: %w", name, err)
	}
	return &builtListener{
		name:     name,
		bindHost: addr.GetAddress(),
		bindPort: addr.GetPortValue(),
		filter:   fh,
	}, nil
}

// Start binds every built listener, captures its bound address, and launches
// one Accept goroutine per listener. Returns after every bind succeeds (or on
// the first bind failure, after unwinding any already-bound sockets). Every
// error begins with "listener: ".
//
// Per SPEC §5.2 and SPEC §10 #7 (settled): a single ctx is captured once and
// shared across every Accept loop and dispatched filter Handle invocation.
// Canceling ctx (or calling Stop) drops the loops within one accept call.
func (m *Manager) Start(ctx context.Context) error {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	if m.started {
		return fmt.Errorf("listener: Start already called")
	}
	for i, bl := range m.built {
		addr := fmt.Sprintf("%s:%d", bl.bindHost, bl.bindPort)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			// Unwind: close any already-bound listeners.
			for j := 0; j < i; j++ {
				_ = m.built[j].socket.Close()
				m.built[j].socket = nil
			}
			return fmt.Errorf("listener: %q: bind %s: %w", bl.name, addr, err)
		}
		bl.socket = ln
	}
	for _, bl := range m.built {
		bl := bl
		go acceptLoop(ctx, bl, bl.socket)
	}
	m.started = true
	return nil
}

// acceptLoop runs until the listener is closed (Stop) or ctx is canceled.
// Each accepted connection is handed to filter.Handle in its own goroutine.
func acceptLoop(ctx context.Context, bl *builtListener, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("listener: %q: accept: %v", bl.name, err)
			continue
		}
		go bl.filter.Handle(ctx, conn)
	}
}

// Listeners returns one Info per bound listener. Empty before Start
// or after a Start that errored out (the unwind clears every socket).
func (m *Manager) Listeners() []Info {
	out := make([]Info, 0, len(m.built))
	for _, bl := range m.built {
		if bl.socket == nil {
			continue
		}
		out = append(out, Info{Name: bl.name, Addr: bl.socket.Addr().String()})
	}
	return out
}

// Stop closes every bound listener socket. Idempotent. In-flight Handle
// goroutines are not waited on (drain semantics = phase 08).
func (m *Manager) Stop() {
	m.startedMu.Lock()
	defer m.startedMu.Unlock()
	for _, bl := range m.built {
		if bl.socket != nil {
			_ = bl.socket.Close()
			bl.socket = nil
		}
	}
	m.started = false
}
