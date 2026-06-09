package redisproxy

import (
	"bufio"
	"context"
	"fmt"
	"net"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
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
// FIFO/positional reply correlation.
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	f.st.incCxTotal()
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
		cmd, raw, err := decodeRequest(dr)
		if err != nil {
			return // io.EOF clean close / a decode error → close (protocol_error is 32.2)
		}
		f.st.incRqTotal()
		f.st.addRxBytes(len(raw))

		if isLocalReply(cmd) {
			reply := localReply(cmd)
			if _, err := downstream.Write(reply); err != nil {
				return
			}
			f.st.addTxBytes(len(reply))
			continue
		}
		u, err := ensureUpstream()
		if err != nil {
			return // unresolvable cluster → graceful close (D-S32.1-6)
		}
		if err := u.Send(ctx, raw); err != nil {
			return
		}
		reply, err := decodeReply(u.Reader())
		if err != nil {
			return
		}
		if _, err := downstream.Write(reply); err != nil {
			return
		}
		f.st.addTxBytes(len(reply))
	}
}
