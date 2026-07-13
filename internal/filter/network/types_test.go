package network

import "testing"

func TestStatusValues(t *testing.T) {
	if Continue != 0 || StopIteration != 1 {
		t.Fatalf("Status enum drift: Continue=%d StopIteration=%d", Continue, StopIteration)
	}
}

func TestFactoryCtxPerChainFields(t *testing.T) {
	ctx := FactoryCtx{
		BaseDir:            "/cfg",
		HasTLS:             true,
		AllowH2C:           true,
		ListenerPrincipal:  "spiffe://x",
		IsQUIC:             true,
		NodeServiceCluster: "svc-a",
	}
	if !ctx.HasTLS || !ctx.AllowH2C || ctx.ListenerPrincipal != "spiffe://x" || ctx.NodeServiceCluster != "svc-a" || ctx.BaseDir != "/cfg" || !ctx.IsQUIC {
		t.Fatalf("FactoryCtx field round-trip failed: %+v", ctx)
	}
}

// TestFactoryCtxIsQUICDefaultsFalse verifies the zero-value FactoryCtx (as
// built on every pre-existing TCP call site) carries IsQUIC=false, so
// threading the field in (phase 61.2) leaves every TCP path unaffected.
func TestFactoryCtxIsQUICDefaultsFalse(t *testing.T) {
	var ctx FactoryCtx
	if ctx.IsQUIC {
		t.Fatalf("zero-value FactoryCtx.IsQUIC = true, want false")
	}
}

// Compile-time assertion that a minimal ReadFilter satisfies the interface.
type noopFilter struct{ Marker }

func (noopFilter) OnNewConnection() Status                      { return Continue }
func (noopFilter) OnData(_ *Buffer, _ bool) Status              { return Continue }
func (noopFilter) SetReadFilterCallbacks(_ ReadFilterCallbacks) {}
func (noopFilter) OnDestroy()                                   {}

var _ ReadFilter = noopFilter{}
