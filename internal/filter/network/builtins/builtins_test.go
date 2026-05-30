package builtins

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
)

// TestRegisterBuiltinsRegistersAllFour proves RegisterBuiltins wires all four
// built-in network filters (echo, direct_response, tcp_proxy, HCM) into a fresh
// Registry. Registration only stores factory closures (it builds no filter), so
// a zero-valued Deps{} is sufficient — the boot singletons are captured but not
// invoked here.
func TestRegisterBuiltinsRegistersAllFour(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{})
	for _, tu := range []string{echo.TypeURL, directresponse.TypeURL, tcpproxy.TypeURL, hcm.TypeURL} {
		if _, ok := reg.Lookup(tu); !ok {
			t.Errorf("RegisterBuiltins did not register %q", tu)
		}
	}
}
