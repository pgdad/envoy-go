package redisproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// filter is the boot-parsed, per-listener-shared redis_proxy terminal filter. It
// is conn-stateless at the struct level — per-connection state (the bufio.Reader
// + the *network.UpstreamConn) lives on Handle's stack. The shared instance is
// read-only after boot; the roster counters/gauges are atomic.
type filter struct {
	network.Marker
	cfg *compiledConfig
	st  *redisStats
	cm  *cluster.Manager

	// dialSource resolves the upstream dial closure + the per-request hook for a
	// proxied command. Production wires it to cm.Get(catch_all) → Cluster.Dial /
	// IncUpstreamRqTotal; unit tests inject a fake (newTestFilter). Returns an
	// error on an unresolvable cluster (→ Handle graceful-closes; D-S32.1-6).
	dialSource func(ctx context.Context) (network.UpstreamDialFunc, func(), error)
}

var _ network.TerminalFilter = (*filter)(nil)

// NewFactory returns the redisproxy NetworkFilterFactory. UNLIKE the stats-only
// zookeeper/mongo/kafka factories, redisproxy needs BOTH the cluster Manager (to
// resolve catch_all → *cluster.Cluster at Handle time — the tcp_proxy precedent)
// AND the stats registry (the redis.<sp> roster). Both are closure-captured from
// builtins.Deps (the network FactoryCtx carries neither). Parses + validates ONCE
// at boot (ADR-0079) and creates the 10 fixed stats once per distinct stat_prefix.
func NewFactory(cm *cluster.Manager, reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		if got := tc.GetTypeUrl(); got != TypeURL {
			return nil, fmt.Errorf("redis_proxy: wrong type_url %q (want %q)", got, TypeURL)
		}
		msg := &redis_proxyv3.RedisProxy{}
		if err := tc.UnmarshalTo(msg); err != nil {
			return nil, fmt.Errorf("redis_proxy: unmarshal: %w", err)
		}
		cfg, err := parseConfig(msg)
		if err != nil {
			return nil, err
		}
		st := newRedisStats(reg, cfg.statPrefix)
		f := &filter{cfg: cfg, st: st, cm: cm}
		f.dialSource = f.resolveCatchAll // production dial source
		return func() network.NetworkFilter { return f }, nil
	}
}

// resolveCatchAll resolves catch_all → *cluster.Cluster LAZILY (tolerant of an
// unknown cluster at config time — §3.3). On a miss it returns an error → Handle
// graceful-closes (no -ERR synthesized at 32.1; D-S32.1-6).
func (f *filter) resolveCatchAll(_ context.Context) (network.UpstreamDialFunc, func(), error) {
	cl, ok := f.cm.Get(f.cfg.catchAllCluster)
	if !ok {
		return nil, nil, fmt.Errorf("redis_proxy: catch_all cluster %q not found", f.cfg.catchAllCluster)
	}
	dial := func(ctx context.Context) (net.Conn, error) {
		c, _, err := cl.Dial(ctx) // Endpoint discarded (§4.2)
		return c, err
	}
	return dial, cl.IncUpstreamRqTotal, nil
}

// Handle takes ownership of the downstream connection and runs the RESP
// command→reply pump to connection close (the tcp_proxy.Handle shape). PING/AUTH
// are answered locally (zero upstream); data commands round-trip through a lazily
// dialed one-conn-per-downstream upstream seam with synchronous single-flight
// FIFO/positional reply correlation. The downstream_cx_active gauge is
// inc/dec-balanced across the connection lifetime; downstream_rq_active and the
// per-command total/success/error counters are managed per-request in serveRequest.
// A malformed frame increments downstream_cx_protocol_error; QUIT closes the pump
// after writing +OK.
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	f.st.incCxTotal()
	f.st.incCxActive()
	defer f.st.decCxActive()
	dr := bufio.NewReader(downstream)

	var up *network.UpstreamConn
	defer func() {
		if up != nil {
			_ = up.Close()
		}
	}()
	// ensureUpstream lazily prepares the seam on the first proxied command.
	ensureUpstream := func() (*network.UpstreamConn, error) {
		if up != nil {
			return up, nil
		}
		dial, hook, err := f.dialSource(ctx)
		if err != nil {
			return nil, err
		}
		up = network.NewUpstreamConn(dial, hook)
		return up, nil
	}

	for {
		cmd, args, raw, err := decodeRequest(dr)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				f.st.incProtocolError() // a malformed frame (§4.5); a clean EOF does not
			}
			return
		}
		f.st.incRqTotal()
		f.st.addRxBytes(len(raw))
		if !f.serveRequest(ctx, downstream, cmd, args, raw, ensureUpstream) {
			return
		}
	}
}

// serveRequest dispatches one decoded request and reports whether the pump should
// continue. It owns the downstream_rq_active gauge inc/dec (defer-balanced across
// every exit path — §4.4).
func (f *filter) serveRequest(ctx context.Context, downstream net.Conn, cmd string, args [][]byte, raw []byte,
	ensureUpstream func() (*network.UpstreamConn, error)) (cont bool) {
	f.st.incRqActive()
	defer f.st.decRqActive()

	v := classify(cmd, args)
	switch v.action {
	case actLocal:
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return !v.closeAfter // QUIT ends the pump
	case actUnsupported:
		f.st.incSplitterUnsupported()
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return true
	case actInvalid:
		f.st.incSplitterInvalid()
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return true
	default: // actProxy
		f.st.incCommandTotal(v.statCmd)
		u, err := ensureUpstream()
		if err != nil {
			f.st.incCommandError(v.statCmd) // unresolvable upstream is a transport failure
			return false
		}
		if err := u.Send(ctx, raw); err != nil {
			f.st.incCommandError(v.statCmd)
			return false
		}
		reply, err := decodeReply(u.Reader())
		if err != nil {
			f.st.incCommandError(v.statCmd)
			return false
		}
		f.st.incCommandSuccess(v.statCmd)
		if _, err := downstream.Write(reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(reply))
		return true
	}
}
