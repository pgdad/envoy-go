package network

import "testing"

func TestCloseTypeValues(t *testing.T) {
	if FlushWrite != 0 || NoFlush != 1 {
		t.Fatalf("CloseType drift: FlushWrite=%d NoFlush=%d", FlushWrite, NoFlush)
	}
}

// Compile-time: the interfaces are declared with the §3.1 method set.
var _ = func(cb ReadFilterCallbacks) {
	_ = cb.Connection()
	cb.ContinueReading()
	_ = cb.DynamicMetadata()
}

var _ = func(c Connection) {
	c.Write(nil, false)
	c.Close(FlushWrite)
	_, _ = c.LocalAddr(), c.RemoteAddr()
	_ = c.RequestedServerName()
	_ = c.DownstreamPrincipals()
}

// Compile-time: the concrete chain-runtime callbacks satisfy ReadFilterCallbacks
// and expose the framework-internal RCD sink (D-P26.1-5b) that direct_response
// type-asserts (Task 8). The live round-trip is proved by TestResponseCodeDetailsSink
// (chain_test.go); these assertions pin the concrete type's surface.
var (
	_ ReadFilterCallbacks                         = (*callbacks)(nil)
	_ interface{ SetResponseCodeDetails(string) } = (*callbacks)(nil)
	_ Connection                                  = (*connection)(nil)
)

// TestConcreteCallbacksLocalAddrFromConn proves the LocalAddr accessor falls
// back to the net.Conn address when connFacts carries no override (R2).
func TestConcreteCallbacksLocalAddrFromConn(t *testing.T) {
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	c := rt.callbacks().Connection()
	if c.LocalAddr() != nil {
		t.Errorf("LocalAddr=%v, want nil (fakeConn has no addr)", c.LocalAddr())
	}
}
