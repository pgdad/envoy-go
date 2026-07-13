package xdsgrpc

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/xds"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

// Compile-time assertion: *Opener satisfies xds.StreamOpener. This also
// compile-proves that the *grpcclient SDS stream (returned by
// (*grpcclient.SDSClient).StreamSecrets) structurally satisfies xds.Stream —
// the whole point of this adapter package existing outside internal/xds.
var _ xds.StreamOpener = (*Opener)(nil)

// httpProtocolOptionsKey is the TypedExtensionProtocolOptions map key the
// cluster manager inspects for the explicit_http_config discriminator
// (internal/cluster/manager.go's unexported constant of the same value).
const httpProtocolOptionsKey = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

// mkPlainH2ClusterMgr builds a *cluster.Manager containing a single plaintext
// (h2c, no transport_socket) STATIC cluster `name` at 127.0.0.1:port with
// UseH2() == true (explicit_http_config.http2_protocol_options{}). Mirrors
// internal/grpcclient/grpcclient_test.go's mkPlainClusterMgr +
// mkH2ClusterMgr shapes combined, without TLS — grpcclient.Dialer only
// requires UseH2(); internal/cluster/manager.go's extractH2Mode explicitly
// supports a nil transport_socket (plaintext h2c upstream).
func mkPlainH2ClusterMgr(t testing.TB, name string, port uint32) *cluster.Manager {
	t.Helper()
	hpo := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpo)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					httpProtocolOptionsKey: hpoAny,
				},
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(plain h2): %v", err)
	}
	return cm
}

// TestOpener_StreamSecrets_RoundTrip verifies the full production stack —
// grpcclient.NewSDSClient dialed via a real *cluster.Manager, wrapped by
// NewOpener — opens a stream whose Send/Recv round-trips a
// DiscoveryRequest/DiscoveryResponse pair against test/helpers/sdsserver, the
// same fake server internal/xds's Provider tests use. This proves the
// grpcclient stream return satisfies xds.Stream in a real (not just
// compile-time) sense.
func TestOpener_StreamSecrets_RoundTrip(t *testing.T) {
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", nil, nil))

	_, portStr, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", srv.Addr(), err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("strconv.ParseUint(%q): %v", portStr, err)
	}

	mgr := mkPlainH2ClusterMgr(t, "c_sds", uint32(port))
	d := grpcclient.New(mgr)

	client, err := grpcclient.NewSDSClient(d, "c_sds")
	if err != nil {
		t.Fatalf("grpcclient.NewSDSClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	opener := NewOpener(client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := opener.StreamSecrets(ctx)
	if err != nil {
		t.Fatalf("StreamSecrets: %v", err)
	}
	if stream == nil {
		t.Fatalf("StreamSecrets: nil stream")
	}
	if err := stream.Send(&discoveryv3.DiscoveryRequest{VersionInfo: "v1", ResourceNames: []string{"server_cert"}}); err != nil {
		t.Fatalf("stream.Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv: %v", err)
	}
	if resp == nil {
		t.Fatalf("stream.Recv: nil response")
	}
	if got := resp.GetVersionInfo(); got != "v1" {
		t.Errorf("stream.Recv: VersionInfo = %q; want %q (echoed)", got, "v1")
	}
	if len(resp.GetResources()) != 1 {
		t.Errorf("stream.Recv: len(Resources) = %d; want 1", len(resp.GetResources()))
	}
}
