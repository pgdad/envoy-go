package builtins

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	networkrbac "github.com/esalaine/envoy-go/internal/filter/network/rbac"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestRegisterBuiltinsRegistersAllFive proves RegisterBuiltins wires all five
// built-in network filters (echo, direct_response, tcp_proxy, HCM,
// rbac_network) into a fresh Registry. Registration only stores factory
// closures (it builds no filter), so a zero-valued Deps{} is sufficient for
// the first four — rbac_network's StatsRegistry is nil here, which is fine
// because registration only captures the closure. reg.Freeze() is called to
// exercise the post-boot lookup path, consistent with the sibling
// RegistersRBACNetwork test.
func TestRegisterBuiltinsRegistersAllFive(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{})
	reg.Freeze()
	for _, tu := range []string{echo.TypeURL, directresponse.TypeURL, tcpproxy.TypeURL, hcm.TypeURL, networkrbac.TypeURL} {
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
