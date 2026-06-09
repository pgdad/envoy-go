package builtins

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/filter/network/kafkabroker"
	"github.com/esalaine/envoy-go/internal/filter/network/mongoproxy"
	networkrbac "github.com/esalaine/envoy-go/internal/filter/network/rbac"
	"github.com/esalaine/envoy-go/internal/filter/network/redisproxy"
	"github.com/esalaine/envoy-go/internal/filter/network/snicluster"
	"github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/stats"
)

// mustAny marshals a proto message into *anypb.Any (mirrors the zookeeperproxy_test.go
// and rbac_test.go shape; generic proto.Message to remain usable for any typed_config).
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestRegisterBuiltinsRegistersAllTen proves RegisterBuiltins wires all ten
// built-in network filters (echo, direct_response, tcp_proxy, HCM,
// rbac_network, sni_cluster, zookeeper_proxy, mongo_proxy, kafka_broker,
// redis_proxy) into a fresh Registry. Registration only stores factory closures
// (it builds no filter), so a zero-valued Deps{} is sufficient — rbac_network's,
// zookeeper_proxy's, mongo_proxy's, kafka_broker's and redis_proxy's
// StatsRegistry are nil here, which is fine because registration only captures
// the closure.
// reg.Freeze() is called to exercise the post-boot lookup path, consistent with
// the sibling registration tests.
func TestRegisterBuiltinsRegistersAllTen(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{})
	reg.Freeze()
	for _, tu := range []string{echo.TypeURL, directresponse.TypeURL, tcpproxy.TypeURL, hcm.TypeURL, networkrbac.TypeURL, snicluster.TypeURL, zookeeperproxy.TypeURL, mongoproxy.TypeURL, kafkabroker.TypeURL, redisproxy.TypeURL} {
		if _, ok := reg.Lookup(tu); !ok {
			t.Errorf("RegisterBuiltins did not register %q", tu)
		}
	}
}

// TestRegisterBuiltins_RegistersRBACNetwork proves rbac_network is wired as the
// 5th built-in network filter (D-26.3-3: the stats Registry is closure-captured
// from deps.StatsRegistry, mirroring tcpproxy/hcm; the network FactoryCtx carries
// no stats registry). A non-nil StatsRegistry is supplied because the rbac_network
// factory predeclares its counters at parse — registration only stores the closure
// here, but a real registry mirrors the boot wiring.
func TestRegisterBuiltins_RegistersRBACNetwork(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(networkrbac.TypeURL); !ok {
		t.Fatal("rbac_network not registered as the 5th built-in")
	}
}

// TestRegisterBuiltins_RegistersSniCluster proves sni_cluster is wired as the
// 6th built-in network filter (D27-S2: no Deps needed — config-less / echo
// parity; ADR-0220). Registration only captures the factory closure.
func TestRegisterBuiltins_RegistersSniCluster(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{})
	reg.Freeze()
	if _, ok := reg.Lookup(snicluster.TypeURL); !ok {
		t.Fatal("sni_cluster not registered as the 6th built-in")
	}
}

// TestRegisterBuiltins_RegistersZookeeperProxy proves zookeeper_proxy is wired
// as the 7th built-in network filter (28.1; ADR-0222). A non-nil StatsRegistry
// is supplied because zookeeper_proxy's factory eagerly creates the 201-counter
// roster at parse; registration only stores the closure here, but a real
// registry mirrors the boot wiring.
func TestRegisterBuiltins_RegistersZookeeperProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(zookeeperproxy.TypeURL); !ok {
		t.Fatal("zookeeper_proxy not registered as the 7th built-in")
	}
}

// TestZookeeperProxyBootSmoke is the boot-smoke for the [zookeeper_proxy,
// tcp_proxy] chain: a zookeeper_proxy filter chain resolves through the
// registry; parsing the zookeeper config eagerly creates the 201 counters at 0
// (mirrors the 26.3 Task-12 [rbac_network, tcp_proxy] boot-smoke shape).
func TestZookeeperProxyBootSmoke(t *testing.T) {
	sreg := stats.NewRegistry()
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: sreg})
	reg.Freeze()

	factory, ok := reg.Lookup(zookeeperproxy.TypeURL)
	if !ok {
		t.Fatal("zookeeper_proxy factory not found")
	}
	tc := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zkboot"})
	instFactory, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	inst := instFactory()
	// Both-directions classification: the instance satisfies BOTH interfaces.
	if _, isRead := inst.(network.ReadFilter); !isRead {
		t.Fatal("zookeeper_proxy instance must be a ReadFilter")
	}
	if _, isWrite := inst.(network.WriteFilter); !isWrite {
		t.Fatal("zookeeper_proxy instance must be a WriteFilter")
	}
	// Eager roster: 201 counters exist at 0 (spot-check response-side names).
	for _, name := range []string{"zkboot.zookeeper.getdata_resp", "zkboot.zookeeper.watch_event",
		"zkboot.zookeeper.connect_rq", "zkboot.zookeeper.decoder_error"} {
		if got := sreg.NewCounterIfAbsent(name).Load(); got != 0 {
			t.Errorf("counter %s = %d at boot, want 0", name, got)
		}
	}
}

// TestRegisterBuiltins_RegistersMongoProxy proves mongo_proxy is wired as the
// 8th built-in network filter (29.1; ADR-0224). A non-nil StatsRegistry is
// supplied because mongo_proxy's factory eagerly creates the 23-stat roster.
func TestRegisterBuiltins_RegistersMongoProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(mongoproxy.TypeURL); !ok {
		t.Fatal("mongo_proxy not registered as the 8th built-in")
	}
}

// TestMongoProxyBootSmoke is the boot-smoke for the [mongo_proxy, tcp_proxy]
// chain: a mongo_proxy filter resolves through the registry; parsing the config
// eagerly creates the 23 stats at 0; the instance satisfies BOTH directions.
func TestMongoProxyBootSmoke(t *testing.T) {
	sreg := stats.NewRegistry()
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: sreg})
	reg.Freeze()

	factory, ok := reg.Lookup(mongoproxy.TypeURL)
	if !ok {
		t.Fatal("mongo_proxy factory not found")
	}
	tc := mustAny(t, &mongo_proxyv3.MongoProxy{StatPrefix: "mongoboot"})
	instFactory, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	inst := instFactory()
	if _, isRead := inst.(network.ReadFilter); !isRead {
		t.Fatal("mongo_proxy instance must be a ReadFilter")
	}
	if _, isWrite := inst.(network.WriteFilter); !isWrite {
		t.Fatal("mongo_proxy instance must be a WriteFilter")
	}
	for _, name := range []string{"mongo.mongoboot.op_query", "mongo.mongoboot.op_reply",
		"mongo.mongoboot.decoding_error", "mongo.mongoboot.delays_injected"} {
		if got := sreg.NewCounterIfAbsent(name).Load(); got != 0 {
			t.Errorf("counter %s = %d at boot, want 0", name, got)
		}
	}
	if got := sreg.NewGaugeIfAbsent("mongo.mongoboot.op_query_active").Load(); got != 0 {
		t.Errorf("gauge op_query_active = %d at boot, want 0", got)
	}
}

// TestRegisterBuiltins_IncludesKafkaBroker proves kafka_broker is wired as the
// 9th built-in network filter (31; ADR-0228; the first /contrib consumer). A
// non-nil StatsRegistry is supplied because kafka_broker's factory eagerly
// creates the 176-counter roster; registration only stores the closure here,
// but a real registry mirrors the boot wiring.
func TestRegisterBuiltins_IncludesKafkaBroker(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(kafkabroker.TypeURL); !ok {
		t.Fatal("kafka_broker not registered as the 9th built-in")
	}
}

// TestRegisterBuiltins_RegistersRedisProxy proves redis_proxy is wired as the
// 10th built-in network filter (32.1; ADR-0229/ADR-0230). UNLIKE the
// stats-only kafka/mongo/zookeeper registrations, redisproxy passes BOTH
// deps.ClusterManager (lazy catch_all resolution) AND deps.StatsRegistry (the
// redis.<sp> roster) — the tcpproxy cluster-capture + stats-capture precedents
// combined. A non-nil StatsRegistry is supplied because the redis_proxy factory
// eagerly creates its stat roster; registration only stores the closure here,
// but a real registry mirrors the boot wiring.
func TestRegisterBuiltins_RegistersRedisProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{ClusterManager: nil, StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(redisproxy.TypeURL); !ok {
		t.Fatal("redis_proxy not registered as the 10th built-in")
	}
}

// ---------------------------------------------------------------------------
// End-to-end override integration tests
//
// These tests are the centerpiece proof of phase 27: the sni_cluster
// SetUpstreamCluster write → chainRuntime.upstreamClusterOverride field →
// handleTerminal ctx wrap → tcp_proxy UpstreamClusterOverride read →
// cluster.Manager lookup → route/close. They are placed here (builtins
// package) because that is the only import-cycle-free home that can import
// both network (chain) and tcpproxy (terminal).
// ---------------------------------------------------------------------------

// mkTwoClusterMgr builds a cluster.Manager with two STATIC clusters, each
// pointing at a distinct local backend. Both clusters use "127.0.0.1" and the
// supplied port numbers. Used by the end-to-end override integration tests to
// exercise the override-then-fallback path without re-importing tcpproxy's
// unexported test helpers.
func mkTwoClusterMgrE2E(t *testing.T, nameA string, portA uint32, nameB string, portB uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{
				{
					Name:                 nameA,
					ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
					LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
					ConnectTimeout:       durationpb.New(2 * time.Second),
					LoadAssignment: &endpointv3.ClusterLoadAssignment{
						ClusterName: nameA,
						Endpoints: []*endpointv3.LocalityLbEndpoints{{
							LbEndpoints: []*endpointv3.LbEndpoint{{
								HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
									Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
										SocketAddress: &corev3.SocketAddress{
											Address:       "127.0.0.1",
											PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: portA},
										},
									}},
								}},
							}},
						}},
					},
				},
				{
					Name:                 nameB,
					ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
					LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
					ConnectTimeout:       durationpb.New(2 * time.Second),
					LoadAssignment: &endpointv3.ClusterLoadAssignment{
						ClusterName: nameB,
						Endpoints: []*endpointv3.LocalityLbEndpoints{{
							LbEndpoints: []*endpointv3.LbEndpoint{{
								HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
									Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
										SocketAddress: &corev3.SocketAddress{
											Address:       "127.0.0.1",
											PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: portB},
										},
									}},
								}},
							}},
						}},
					},
				},
			},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("mkTwoClusterMgrE2E: cluster.NewManager: %v", err)
	}
	return cm
}

// startSentinelBackend starts a TCP server that immediately writes sentinel
// and then echoes until EOF. Returns the listener; caller must close it.
func startSentinelBackend(t *testing.T, sentinel string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startSentinelBackend: listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				// Write the sentinel so the downstream can identify which backend served it.
				_, _ = c.Write([]byte(sentinel))
				// Echo the rest.
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln
}

// mkTcpProxyAny builds an *anypb.Any for a TcpProxy proto with the given
// stat_prefix and cluster name.
func mkTcpProxyAny(t *testing.T, statPrefix, cluster string) *anypb.Any {
	t.Helper()
	a, err := anypb.New(&tcpproxyv3.TcpProxy{
		StatPrefix:       statPrefix,
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: cluster},
	})
	if err != nil {
		t.Fatalf("mkTcpProxyAny: anypb.New: %v", err)
	}
	return a
}

// buildE2EChain constructs the [sni_cluster, tcp_proxy] filter chain for
// end-to-end integration tests. It instantiates snicluster via
// snicluster.New(nil, FactoryCtx{}) and tcp_proxy via tcpproxy.NewFilter.
// The snicluster filter is the read-prefix; tcp_proxy is the terminal.
// The chain is driven via network.NewChainRuntime, mirroring serveNetworkChain.
func buildE2EChain(t *testing.T, cm *cluster.Manager, defaultCluster string, serverName string, downstream net.Conn) *network.ChainRuntime {
	t.Helper()

	// Build sni_cluster filter instance (config-less, echo shape).
	sniFIF, err := snicluster.New(nil, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("snicluster.New: %v", err)
	}
	sniFilter := sniFIF()

	// Build tcp_proxy filter instance.
	tcpProxyFilter, err := tcpproxy.NewFilter(mkTcpProxyAny(t, "e2e_test", defaultCluster), cm, nil)
	if err != nil {
		t.Fatalf("tcpproxy.NewFilter: %v", err)
	}

	filters := []network.NetworkFilter{sniFilter, tcpProxyFilter}
	facts := network.ConnFacts{ServerName: serverName}
	return network.NewChainRuntime(filters, downstream, facts)
}

// TestSniClusterOverrideRoutesEndToEnd proves the FULL override seam
// (sni_cluster SetUpstreamCluster → chainRuntime → handleTerminal ctx →
// tcp_proxy UpstreamClusterOverride → cluster.Manager.Get → Dial) with a
// REAL chain via network.NewChainRuntime and tcp_proxy.Handle.
//
// Setup: two distinct sentinel backends ("FOO" for foo.example.com, "FALLBACK"
// for bar). SNI = "foo.example.com". tcp_proxy configured default = "bar".
// Expected: sni_cluster overrides the cluster to "foo.example.com" → the
// downstream receives "FOO" (NOT "FALLBACK"), proving the override is live.
func TestSniClusterOverrideRoutesEndToEnd(t *testing.T) {
	// Start two sentinel backends with DISTINCT payloads.
	backendFoo := startSentinelBackend(t, "FOO")
	defer func() { _ = backendFoo.Close() }()
	portFoo := uint32(backendFoo.Addr().(*net.TCPAddr).Port)

	backendBar := startSentinelBackend(t, "FALLBACK")
	defer func() { _ = backendBar.Close() }()
	portBar := uint32(backendBar.Addr().(*net.TCPAddr).Port)

	// Build a 2-cluster manager: "foo.example.com" → portFoo, "bar" → portBar.
	cm := mkTwoClusterMgrE2E(t, "foo.example.com", portFoo, "bar", portBar)

	// Build a connected downstream pipe pair (server side is the chain's conn).
	serverConn, clientConn, err := newConnPair(t)
	if err != nil {
		t.Fatalf("newConnPair: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	// Build the [sni_cluster, tcp_proxy] chain with SNI = "foo.example.com".
	// tcp_proxy default cluster = "bar" (the fallback that MUST NOT be used).
	rtChain := buildE2EChain(t, cm, "bar", "foo.example.com", serverConn)
	defer rtChain.OnDestroy()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Production call sequence (mirrors serveNetworkChain).
	// 1. OnNewConnection: sni_cluster sets the override; pure-terminal check
	//    is irrelevant here (we have a read-filter prefix).
	rtChain.OnNewConnection()
	// 2. OnData: sni_cluster passes through (Continue); resumeIdx advances past
	//    it to len(filters), making TerminalReady true.
	rtChain.OnData([]byte("ping"), false)
	// 3. Verify TerminalReady before handing off.
	if !rtChain.TerminalReady() {
		t.Fatal("TerminalReady() = false after OnData pass-through; chain did not advance to terminal")
	}
	// 4. HandleTerminal: wraps ctx with override "foo.example.com", hands
	//    serverConn (with "ping" prefix) to tcp_proxy.Handle. HandleTerminal
	//    blocks (pumping bytes) until the downstream closes, so run it in a
	//    goroutine and read from clientConn concurrently.
	done := make(chan struct{})
	go func() {
		rtChain.HandleTerminal(ctx)
		close(done)
	}()

	// Read the sentinel from the upstream backend, relayed through tcp_proxy.
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := clientConn.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("clientConn.Read: %v (n=%d)", err, n)
	}
	got := string(buf[:n])

	// Close downstream so tcp_proxy's Handle loop exits.
	_ = clientConn.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleTerminal did not return within timeout")
	}

	// Assert: "FOO" (the override cluster's backend) was reached, NOT "FALLBACK".
	// Use HasPrefix rather than exact equality: if the sentinel and the echoed
	// payload coalesce into one TCP segment the read may return "FOOping".
	if !strings.HasPrefix(got, "FOO") {
		t.Errorf("got %q, want prefix %q — override cluster was not used", got, "FOO")
	}
	if strings.HasPrefix(got, "FALLBACK") {
		t.Errorf("got %q — response came from fallback cluster, override not applied", got)
	}
}

// TestSniClusterUnknownOverrideClosesEndToEnd proves that a sni_cluster
// override naming an UNKNOWN cluster causes tcp_proxy to close the downstream
// with zero application bytes (F-NOROUTE, D27-4).
//
// Setup: same 2-cluster manager but SNI = "ghost.example.com" (not in cm).
// Expected: downstream reads EOF with ZERO application bytes.
func TestSniClusterUnknownOverrideClosesEndToEnd(t *testing.T) {
	backendFoo := startSentinelBackend(t, "FOO")
	defer func() { _ = backendFoo.Close() }()
	portFoo := uint32(backendFoo.Addr().(*net.TCPAddr).Port)

	backendBar := startSentinelBackend(t, "FALLBACK")
	defer func() { _ = backendBar.Close() }()
	portBar := uint32(backendBar.Addr().(*net.TCPAddr).Port)

	cm := mkTwoClusterMgrE2E(t, "foo.example.com", portFoo, "bar", portBar)

	serverConn, clientConn, err := newConnPair(t)
	if err != nil {
		t.Fatalf("newConnPair: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	// SNI = "ghost.example.com" — no such cluster in cm.
	rtChain := buildE2EChain(t, cm, "bar", "ghost.example.com", serverConn)
	defer rtChain.OnDestroy()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rtChain.OnNewConnection()
	rtChain.OnData([]byte("ping"), false)
	if !rtChain.TerminalReady() {
		t.Fatal("TerminalReady() = false after OnData pass-through")
	}

	done := make(chan struct{})
	go func() {
		rtChain.HandleTerminal(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleTerminal did not return within timeout")
	}

	// Downstream must see EOF with ZERO application bytes.
	_ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 256)
	n, readErr := clientConn.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 application bytes, got %d: %q", n, buf[:n])
	}
	if readErr == nil {
		t.Error("expected EOF or error reading from closed downstream, got nil")
	}
}

// newConnPair creates a local TCP connection pair for testing.
// Returns (serverSide, clientSide, error).
func newConnPair(t *testing.T) (net.Conn, net.Conn, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = ln.Close() }()

	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, e := ln.Accept()
		ch <- result{c, e}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	res := <-ch
	if res.err != nil {
		_ = clientConn.Close()
		return nil, nil, res.err
	}
	return res.conn, clientConn, nil
}
