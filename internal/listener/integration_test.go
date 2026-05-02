package listener

import (
	"context"
	"net"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestIntegration is the phase-07.2 Task-12 in-process end-to-end test of the
// unified pre/post-handshake dispatch path (SPEC §5.2 / ADR-0081). Each subtest
// stands up a real `Manager.Start` → `acceptLoop` → `serveConnection`
// pipeline against a programmatically-built bootstrap and asserts the winning
// filter_chain by reading the tag byte the upstream tagged-backend writes on
// accept. The TLS+SNI dispatch dimension is already covered by
// TestUnifiedDispatchTLSWithSNI in manager_test.go; this file focuses on the
// plaintext-dimension matrix (destination_port, source_prefix_ranges, default,
// listener_filters timeout) called out by PLAN.md Task 12.
//
// All subtests run pure-Go (no Docker, no testcontainers) and complete in
// well under 5s in aggregate.
func TestIntegration(t *testing.T) {
	// Three tagged backends — D for destination_port-chain, S for
	// source_prefix-chain, X for default_filter_chain. Each subtest re-uses
	// the shared backends; the cluster.Manager threading is per-subtest because
	// the chain → cluster mapping varies and cluster.Manager is built from a
	// bootstrap so it must be re-allocated.
	addrD, cleanD := startTaggedBackend(t, 'D')
	defer cleanD()
	addrS, cleanS := startTaggedBackend(t, 'S')
	defer cleanS()
	addrX, cleanX := startTaggedBackend(t, 'X')
	defer cleanX()

	type subtest struct {
		name    string
		wantTag byte
		// configure builds the listener for this subtest. resolvedPort is the
		// OS-picked port discovered via a probe-listen; the listener is then
		// configured to bind on resolvedPort. Returning the listener lets each
		// subtest steer the chain-match dimensions independently. The slow flag
		// signals the listener-filter timeout abort case where the read should
		// fail (peer close), not deliver a tag.
		configure func(t *testing.T, resolvedPort uint32) *listenerv3.Listener
		// slowLF: install slowListenerFilter on rt[0] post-NewManager. The
		// listener is configured with continue=false + 1s lfTimeout so the
		// pipeline aborts the conn before any chain dispatches.
		slowLF bool
	}

	subtests := []subtest{
		{
			name:    "match_dstport_only",
			wantTag: 'D',
			configure: func(t *testing.T, p uint32) *listenerv3.Listener {
				// chain_dstport_only matches; chain_srcprefix_only is
				// 10.0.0.0/8 (loopback dialer is 127.x → no match).
				return mkChainsListener(t, p,
					&listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(p)},
					&listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
						{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
					}},
				)
			},
		},
		{
			name:    "match_srcprefix_only",
			wantTag: 'S',
			configure: func(t *testing.T, p uint32) *listenerv3.Listener {
				// chain_dstport_only is bound_port+1 (no match); chain_srcprefix_only
				// is 127.0.0.1/32 (loopback dialer matches).
				return mkChainsListener(t, p,
					&listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(p + 1)},
					&listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
						{AddressPrefix: "127.0.0.1", PrefixLen: wrapperspb.UInt32(32)},
					}},
				)
			},
		},
		{
			name:    "match_both_dstport_wins",
			wantTag: 'D',
			configure: func(t *testing.T, p uint32) *listenerv3.Listener {
				// Both chains match; per ADR-0081 priority vector
				// destination_port (slot 0) outranks source_prefix_ranges (slot 6).
				return mkChainsListener(t, p,
					&listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(p)},
					&listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
						{AddressPrefix: "127.0.0.1", PrefixLen: wrapperspb.UInt32(32)},
					}},
				)
			},
		},
		{
			name:    "match_neither_falls_to_default",
			wantTag: 'X',
			configure: func(t *testing.T, p uint32) *listenerv3.Listener {
				// Neither specific chain matches; default_filter_chain wins.
				return mkChainsListener(t, p,
					&listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(p + 1)},
					&listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
						{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
					}},
				)
			},
		},
		{
			name:    "listener_filters_timeout_abort",
			wantTag: 0, // no read expected
			slowLF:  true,
			configure: func(t *testing.T, p uint32) *listenerv3.Listener {
				// Single tagged-D chain + 1s lfTimeout + continue=false; the
				// slow listener filter blocks 2s, so the pipeline aborts and
				// the conn is closed before the chain ever dispatches.
				dF := mkTcpProxyFilter(t, "c_d")
				return &listenerv3.Listener{
					Name: "l_int_abort",
					Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
						SocketAddress: &corev3.SocketAddress{
							Address:       "127.0.0.1",
							PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: p},
						},
					}},
					FilterChains: []*listenerv3.FilterChain{
						{Filters: []*listenerv3.Filter{dF}},
					},
					ListenerFiltersTimeout: durationpb.New(1 * time.Second),
					// continue_on_listener_filters_timeout defaults to false.
				}
			},
		},
	}

	for _, tc := range subtests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Probe-listen on :0 to discover an OS-assigned port we can pre-
			// populate into the chain match dimensions, then immediately close
			// (the Manager will re-bind to the same port — race-free on Linux
			// because the kernel won't recycle the 4-tuple in the few µs gap).
			probe, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("probe listen: %v", err)
			}
			resolvedPort := uint32(probe.Addr().(*net.TCPAddr).Port)
			_ = probe.Close()

			// All subtests share the same 3-cluster mapping; the chain spec
			// determines which cluster (and thus which tag) wins.
			cm := threeClusterMgr(t,
				[]string{"c_d", "c_s", "c_x"},
				[]string{"127.0.0.1", "127.0.0.1", "127.0.0.1"},
				[]uint32{uint32(addrD.Port), uint32(addrS.Port), uint32(addrX.Port)},
			)

			l := tc.configure(t, resolvedPort)
			boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
			mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
			if err != nil {
				t.Fatalf("NewManager: %v", err)
			}

			if tc.slowLF {
				installSlowListenerFilter(mgr.runtimes[0], 2*time.Second)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := mgr.Start(ctx); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer mgr.Stop()

			addr := mgr.Listeners()[0].Addr
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer func() { _ = conn.Close() }()

			if tc.slowLF {
				// Pipeline aborts ~1s in; the listener closes the conn so the
				// Read returns EOF / a non-nil net error within ~3s. We do NOT
				// require an exact errno (platform-dependent).
				if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
					t.Fatalf("SetReadDeadline: %v", err)
				}
				buf := make([]byte, 1)
				n, rerr := conn.Read(buf)
				if rerr == nil && n > 0 {
					t.Errorf("expected listener-side close after lfTimeout abort; got %d bytes %q", n, buf[:n])
				}
				return
			}

			got := readByteWithTimeout(t, conn, 2*time.Second)
			if got != tc.wantTag {
				t.Errorf("tag = %q, want %q", got, tc.wantTag)
			}
		})
	}
}

// mkChainsListener builds a 2-chain + default-chain listener at 127.0.0.1:p
// where the first chain matches via fcmA → cluster c_d, the second via fcmB
// → cluster c_s, and default_filter_chain → cluster c_x. Used by
// TestIntegration's non-timeout subtests.
func mkChainsListener(t *testing.T, p uint32, fcmA, fcmB *listenerv3.FilterChainMatch) *listenerv3.Listener {
	t.Helper()
	return &listenerv3.Listener{
		Name: "l_int",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: p},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{
				Name:             "chain_dstport_only",
				FilterChainMatch: fcmA,
				Filters:          []*listenerv3.Filter{mkTcpProxyFilter(t, "c_d")},
			},
			{
				Name:             "chain_srcprefix_only",
				FilterChainMatch: fcmB,
				Filters:          []*listenerv3.Filter{mkTcpProxyFilter(t, "c_s")},
			},
		},
		DefaultFilterChain: &listenerv3.FilterChain{
			Name:    "default",
			Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_x")},
		},
	}
}

// threeClusterMgr is the 3-cluster cousin of manager_test.go's twoClusterMgr.
// Each of the three names → host:port triples becomes a STATIC cluster on
// the resulting cluster.Manager. Used by TestIntegration so a single
// listener can expose three distinct chain → backend mappings (dstport,
// srcprefix, default) simultaneously.
func threeClusterMgr(t *testing.T, names, hosts []string, ports []uint32) *cluster.Manager {
	t.Helper()
	if len(names) != 3 || len(hosts) != 3 || len(ports) != 3 {
		t.Fatalf("threeClusterMgr: want 3 entries, got %d/%d/%d", len(names), len(hosts), len(ports))
	}
	clusters := make([]*clusterv3.Cluster, 3)
	for i := range names {
		clusters[i] = mkStaticCluster(names[i], hosts[i], ports[i])
	}
	bs := mkBoot(0, nil, clusters)
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// mkStaticCluster builds a one-endpoint STATIC cluster pointing at host:port.
// Mirrors the inline cluster construction used by twoClusterMgr but extracted
// so threeClusterMgr stays small.
func mkStaticCluster(name, host string, port uint32) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		ConnectTimeout:       durationpb.New(time.Second),
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpointv3.LocalityLbEndpoints{{
				LbEndpoints: []*endpointv3.LbEndpoint{{
					HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
						Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
							SocketAddress: &corev3.SocketAddress{
								Address:       host,
								PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
							},
						}},
					}},
				}},
			}},
		},
	}
}
