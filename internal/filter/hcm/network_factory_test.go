package hcm

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

// Compile-time proof that *Filter satisfies network.TerminalFilter (26.2 Task 6;
// R-T). The sealed isNetworkFilter() comes from the network.Marker embed on
// Filter; Handle is byte-identical to TerminalFilter.Handle.
var _ network.TerminalFilter = (*Filter)(nil)

// TestHCMNewNetworkFactoryBridgesFactoryCtx proves the adapter builds the HCM
// terminal filter once per chain, yields the SHARED *Filter per accepted
// connection (HCM is conn-stateless across its Handle serve loop), and bridges
// the per-chain network.FactoryCtx into hcm.ListenerCtx — registering the same
// 5 HCM-scope per-instance metrics the manager-path build registers (R-A).
func TestHCMNewNetworkFactoryBridgesFactoryCtx(t *testing.T) {
	cm := mkClusterManager(t)
	reg := stats.NewRegistry()
	httpReg := testHTTPRegistry()
	factory := NewNetworkFactory(cm, reg, nil, httpReg, nil, nil, nil)
	tc := mkHCM(nil)
	mk, err := factory(tc, network.FactoryCtx{HasTLS: true, AllowH2C: true, ListenerPrincipal: "spiffe://p", NodeServiceCluster: "svc"})
	if err != nil {
		t.Fatalf("NewNetworkFactory err: %v", err)
	}
	a, b := mk(), mk()
	if a != b {
		t.Errorf("HCM adapter must yield the SAME shared instance per call")
	}
	if _, ok := a.(network.TerminalFilter); !ok {
		t.Errorf("yielded instance is not a network.TerminalFilter")
	}

	// R-A stat-registration parity: assert the SAME 5 HCM-scope per-instance
	// metrics the manager-path build registers (mirrors
	// TestNewFilter_Allocates5HCMMetrics in filter_test.go) are present on reg
	// after the adapter build. stat_prefix "ingress_http" comes from mkHCM.
	want := map[string]bool{
		"http.ingress_http.downstream_rq_total": false,
		"http.ingress_http.downstream_rq_2xx":   false,
		"http.ingress_http.downstream_rq_3xx":   false,
		"http.ingress_http.downstream_rq_4xx":   false,
		"http.ingress_http.downstream_rq_5xx":   false,
	}
	reg.Walk(func(m stats.Metric) {
		if _, ok := want[m.Name()]; ok {
			want[m.Name()] = true
		}
	})
	for name, seen := range want {
		if !seen {
			t.Errorf("missing HCM-scope metric %q in Registry after adapter build", name)
		}
	}
}

// TestHCMNewNetworkFactoryParseRejectPassthrough proves a typed_config parse
// error surfaces through the adapter (the boot-time fail-fast contract): the
// adapter returns the constructor's error rather than swallowing it.
func TestHCMNewNetworkFactoryParseRejectPassthrough(t *testing.T) {
	cm := mkClusterManager(t)
	reg := stats.NewRegistry()
	httpReg := testHTTPRegistry()
	factory := NewNetworkFactory(cm, reg, nil, httpReg, nil, nil, nil)
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff}}
	if _, err := factory(bad, network.FactoryCtx{}); err == nil {
		t.Fatalf("expected HCM parse-reject through adapter")
	}
}
