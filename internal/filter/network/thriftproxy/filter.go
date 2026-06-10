package thriftproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// resolveStatus distinguishes the three route-HIT resolution outcomes the pump
// accounts separately: a usable cluster (resolveOK), an unresolvable cluster name
// (resolveUnknownCluster → unknown_cluster), and a resolvable cluster with no
// healthy host (resolveNoHealthyUpstream → no_healthy_upstream). The redis pump
// folds these into one transport error; thrift splits them per the router roster
// (SPEC §3.3).
type resolveStatus int

const (
	resolveOK resolveStatus = iota
	resolveUnknownCluster
	resolveNoHealthyUpstream
)

// resolveFunc resolves a route's cluster name to an upstream dial closure + the
// per-request hook (cluster.IncUpstreamRqTotal). Production wires it to
// resolveCluster (cm.Get → PickEndpoint → Cluster.Dial); unit tests inject a fake
// (newTestFilter) so the pump is tested without a real cluster.Manager.
type resolveFunc func(ctx context.Context, clusterName string) (network.UpstreamDialFunc, func(), resolveStatus)

// filter is the boot-parsed, per-listener-shared thrift_proxy terminal filter. It
// is conn-stateless at the struct level — per-connection state (the bufio.Reader +
// the *network.UpstreamConn) lives on Handle's stack. The shared instance is
// read-only after boot; the roster counters/gauge are atomic. (The redisproxy
// filter shape — §3.5.)
type filter struct {
	network.Marker
	cfg     *compiledConfig
	st      *thriftStats
	cm      *cluster.Manager
	resolve resolveFunc
}

var _ network.TerminalFilter = (*filter)(nil)

// NewFactory returns the thriftproxy NetworkFilterFactory. Like redisproxy (and
// UNLIKE the stats-only mongo/kafka factories) it needs BOTH the cluster Manager
// (to resolve a route's cluster → *cluster.Cluster at Handle time — the tcp_proxy
// precedent) AND the stats registry (the thrift.<sp> roster). Both are
// closure-captured from builtins.Deps (the network FactoryCtx carries neither).
// Parses + validates ONCE at boot (ADR-0079) and creates the fixed roster once per
// distinct stat_prefix.
func NewFactory(cm *cluster.Manager, reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		if got := tc.GetTypeUrl(); got != TypeURL {
			return nil, fmt.Errorf("thrift_proxy: wrong type_url %q (want %q)", got, TypeURL)
		}
		msg := &thrift_proxyv3.ThriftProxy{}
		if err := tc.UnmarshalTo(msg); err != nil {
			return nil, fmt.Errorf("thrift_proxy: unmarshal: %w", err)
		}
		cfg, err := parseConfig(msg)
		if err != nil {
			return nil, err
		}
		st := newThriftStats(reg, cfg.statPrefix)
		f := &filter{cfg: cfg, st: st, cm: cm}
		f.resolve = f.resolveCluster // production resolver
		return func() network.NetworkFilter { return f }, nil
	}
}

// resolveCluster resolves clusterName → an upstream dial closure LAZILY (tolerant
// of an unknown cluster at config time — §3.3). cm.Get-miss → resolveUnknownCluster;
// a no-healthy-host cluster (PickEndpoint fails) → resolveNoHealthyUpstream; else a
// dial closure over Cluster.Dial (Endpoint discarded — §8) + IncUpstreamRqTotal.
func (f *filter) resolveCluster(_ context.Context, clusterName string) (network.UpstreamDialFunc, func(), resolveStatus) {
	cl, ok := f.cm.Get(clusterName)
	if !ok {
		return nil, nil, resolveUnknownCluster
	}
	if _, err := cl.PickEndpoint(); err != nil {
		return nil, nil, resolveNoHealthyUpstream
	}
	dial := func(ctx context.Context) (net.Conn, error) {
		c, _, err := cl.Dial(ctx) // Endpoint discarded (§8)
		return c, err
	}
	return dial, cl.IncUpstreamRqTotal, resolveOK
}

// Handle takes ownership of the downstream connection and runs the framed-binary
// message→reply pump to connection close (the tcp_proxy/redisproxy.Handle shape).
// A route MISS is answered locally with an UnknownMethod EXCEPTION (zero upstream,
// connection kept open — AMEND-T6); a route HIT round-trips the raw frame through a
// lazily dialed one-conn-per-downstream upstream seam with synchronous
// single-flight reply correlation. request_active is inc/dec-balanced per request
// in serveRequest. A malformed frame increments request_decoding_error; an invalid
// msgtype increments request_invalid_type — both end the pump.
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	dr := bufio.NewReader(downstream)

	var up *network.UpstreamConn
	upCluster := ""
	defer func() {
		if up != nil {
			_ = up.Close()
		}
	}()
	// ensureUpstream lazily prepares the seam for the resolved cluster on the first
	// proxied frame; reuses it if a later frame names the same cluster, re-dials if
	// a different cluster (defensive — the MVP fixture pins ONE route/cluster).
	ensureUpstream := func(clusterName string) (*network.UpstreamConn, resolveStatus) {
		if up != nil && upCluster == clusterName {
			return up, resolveOK
		}
		dial, hook, status := f.resolve(ctx, clusterName)
		if status != resolveOK {
			return nil, status
		}
		if up != nil {
			_ = up.Close()
		}
		up = network.NewUpstreamConn(dial, hook)
		upCluster = clusterName
		return up, resolveOK
	}

	for {
		m, err := decodeFrame(dr)
		if err != nil {
			if errors.Is(err, errInvalidType) {
				f.st.inc("request_invalid_type")
				return
			}
			if !errors.Is(err, io.EOF) {
				f.st.inc("request_decoding_error") // a malformed frame; a clean EOF does not
			}
			return
		}
		if !f.serveRequest(ctx, downstream, m, ensureUpstream) {
			return
		}
	}
}

// serveRequest dispatches one decoded message and reports whether the pump should
// continue. It owns the request_active gauge inc/dec (defer-balanced across every
// exit path — §3.3). A route MISS writes the local UnknownMethod exception and
// returns true (the connection stays open — AMEND-T6).
func (f *filter) serveRequest(ctx context.Context, downstream net.Conn, m *thriftMessage,
	ensureUpstream func(string) (*network.UpstreamConn, resolveStatus)) (cont bool) {
	f.st.inc("request")
	switch m.msgType {
	case msgTypeOneway:
		f.st.inc("request_oneway")
	default: // msgTypeCall (the MVP single-flight pump round-trips oneway like call — see PROGRESS)
		f.st.inc("request_call")
	}
	if f.cfg.payloadPassthrough {
		f.st.inc("request_passthrough")
	}
	f.st.incActive()
	defer f.st.decActive()

	clusterName, ok := f.cfg.routes.match(m.method)
	if !ok {
		// MISS: local UnknownMethod exception, zero upstream, connection kept open.
		f.st.inc("route_missing")
		f.st.inc("response_exception")
		if _, err := downstream.Write(encodeUnknownMethod(m.method, m.seqID)); err != nil {
			return false
		}
		return true
	}

	// HIT: round-trip the raw frame through the seam.
	up, status := ensureUpstream(clusterName)
	switch status {
	case resolveUnknownCluster:
		f.st.inc("unknown_cluster")
		return false
	case resolveNoHealthyUpstream:
		f.st.inc("no_healthy_upstream")
		return false
	}
	if err := up.Send(ctx, m.raw); err != nil {
		return false // a transport failure ends the pump (the redis precedent)
	}
	reply, err := decodeFrame(up.Reader())
	if err != nil {
		f.st.inc("response_decoding_error")
		return false
	}
	f.st.inc("response")
	switch reply.msgType {
	case msgTypeReply:
		f.st.inc("response_reply")
		if classifyReply(reply) == replySuccess {
			f.st.inc("response_success")
		} else {
			f.st.inc("response_error")
		}
	case msgTypeException:
		f.st.inc("response_exception")
	default:
		f.st.inc("response_invalid_type")
	}
	if f.cfg.payloadPassthrough {
		f.st.inc("response_passthrough")
	}
	if _, err := downstream.Write(reply.raw); err != nil {
		return false
	}
	return true
}
