package snicluster

import (
	"net"
	"testing"

	sni_clusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	"github.com/esalaine/envoy-go/internal/filter/network"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type fakeConn struct {
	sni string
}

func (c *fakeConn) Write(_ []byte, _ bool)         {}
func (c *fakeConn) Close(_ network.CloseType)      {}
func (c *fakeConn) LocalAddr() net.Addr            { return nil }
func (c *fakeConn) RemoteAddr() net.Addr           { return nil }
func (c *fakeConn) RequestedServerName() string    { return c.sni }
func (c *fakeConn) DownstreamPrincipals() []string { return nil }

type fakeCB struct {
	sni      string
	setCalls int
	lastSet  string
}

func (cb *fakeCB) Connection() network.Connection { return &fakeConn{sni: cb.sni} }
func (cb *fakeCB) ContinueReading()               {}
func (cb *fakeCB) DynamicMetadata() *dynamicmetadata.Bucket {
	return dynamicmetadata.NewBucket()
}
func (cb *fakeCB) SetUpstreamCluster(name string) {
	cb.setCalls++
	cb.lastSet = name
}
func (cb *fakeCB) Draining() bool                         { return false }
func (cb *fakeCB) CloseDirection() network.CloseDirection { return network.CloseDirectionUnset }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustAny(t *testing.T, m *sni_clusterv3.SniCluster) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// newFilterForTest builds a filter instance via New(...)() and wires cb.
func newFilterForTest(t *testing.T, cb *fakeCB) network.ReadFilter {
	t.Helper()
	fif, err := New(mustAny(t, &sni_clusterv3.SniCluster{}), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("New unexpectedly failed: %v", err)
	}
	rf := fif().(network.ReadFilter)
	rf.SetReadFilterCallbacks(cb)
	return rf
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTypeURLHasExtensionsSegment(t *testing.T) {
	want := "type.googleapis.com/" + string(proto.MessageName(&sni_clusterv3.SniCluster{}))
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

// TestTypeURLByteStable pins the exact string so a proto rename is caught.
func TestTypeURLByteStable(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

func TestNew_AcceptsEmptyAndAbsentConfig(t *testing.T) {
	for _, tc := range []*anypb.Any{nil, {}, mustAny(t, &sni_clusterv3.SniCluster{})} {
		if _, err := New(tc, network.FactoryCtx{}); err != nil {
			t.Fatalf("New(%v) error = %v, want nil", tc, err)
		}
	}
}

func TestNew_MalformedAnyRejected(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(bad, network.FactoryCtx{})
	if err == nil {
		t.Fatal("New(malformed) error = nil, want non-nil")
	}
	const prefix = "sni_cluster: invalid typed_config: "
	if msg := err.Error(); len(msg) < len(prefix) || msg[:len(prefix)] != prefix {
		t.Fatalf("error %q does not start with %q", msg, prefix)
	}
}

// TestOnNewConnection_SetsOverrideFromSNI: a non-empty SNI must produce a
// verbatim SetUpstreamCluster call — the live assertion that proves the call is
// non-vacuous (mandatory per PLAN).
func TestOnNewConnection_SetsOverrideFromSNI(t *testing.T) {
	cb := &fakeCB{sni: "foo.example.com"}
	f := newFilterForTest(t, cb)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection status = %v, want Continue", got)
	}
	if cb.setCalls != 1 || cb.lastSet != "foo.example.com" {
		t.Fatalf("SetUpstreamCluster calls=%d last=%q, want 1 / foo.example.com",
			cb.setCalls, cb.lastSet)
	}
}

// TestOnNewConnection_EmptySNINoOp: empty SNI → NO SetUpstreamCluster call,
// still Continue.
func TestOnNewConnection_EmptySNINoOp(t *testing.T) {
	cb := &fakeCB{sni: ""}
	f := newFilterForTest(t, cb)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("status = %v, want Continue", got)
	}
	if cb.setCalls != 0 {
		t.Fatalf("SetUpstreamCluster called %d times on empty SNI, want 0", cb.setCalls)
	}
}

// TestOnData_PassThroughContinue: OnData is a pass-through Continue (does not
// drain, does not halt).
func TestOnData_PassThroughContinue(t *testing.T) {
	cb := &fakeCB{}
	f := newFilterForTest(t, cb)
	buf := &network.Buffer{}
	buf.Append([]byte("hello"))
	if got := f.OnData(buf, false); got != network.Continue {
		t.Fatalf("OnData status = %v, want Continue", got)
	}
	if buf.Len() != 5 {
		t.Fatalf("OnData drained the buffer (len=%d), want pass-through (5)", buf.Len())
	}
}
