package echo_test

import (
	"net"
	"testing"

	echov3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/echo/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type fakeConnection struct {
	written   []byte
	endStream bool
}

func (c *fakeConnection) Write(data []byte, endStream bool) {
	c.written = append(c.written, data...)
	c.endStream = endStream
}
func (c *fakeConnection) Close(_ network.CloseType)      {}
func (c *fakeConnection) LocalAddr() net.Addr            { return nil }
func (c *fakeConnection) RemoteAddr() net.Addr           { return nil }
func (c *fakeConnection) RequestedServerName() string    { return "" }
func (c *fakeConnection) DownstreamPrincipals() []string { return nil }

type fakeCallbacks struct {
	conn fakeConnection
}

func (cb *fakeCallbacks) Connection() network.Connection {
	return &cb.conn
}
func (cb *fakeCallbacks) ContinueReading() {}
func (cb *fakeCallbacks) DynamicMetadata() *dynamicmetadata.Bucket {
	return dynamicmetadata.NewBucket()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEchoParseEmptyConfig(t *testing.T) {
	cfgAny, _ := anypb.New(&echov3.Echo{})
	fif, err := echo.New(cfgAny, network.FactoryCtx{})
	if err != nil || fif == nil {
		t.Fatalf("New(empty Echo) = %v, %v", fif, err)
	}
	if rf := fif(); rf == nil {
		t.Fatalf("instance factory returned nil")
	}
}

func TestEchoRejectsInvalidTypedConfig(t *testing.T) {
	fif, err := echo.New(&anypb.Any{TypeUrl: echo.TypeURL, Value: []byte("not-valid-proto-garbage")}, network.FactoryCtx{})
	if err == nil {
		t.Fatal("New(invalid typed_config) want error, got nil")
	}
	if fif != nil {
		t.Errorf("New(invalid typed_config) want nil factory, got non-nil")
	}
	const want = "echo: invalid typed_config"
	if msg := err.Error(); len(msg) < len(want) || msg[:len(want)] != want {
		t.Errorf("error message %q does not start with %q", msg, want)
	}
}

func TestEchoOnNewConnectionContinues(t *testing.T) {
	fif, _ := echo.New(&anypb.Any{TypeUrl: echo.TypeURL}, network.FactoryCtx{})
	rf := fif().(network.ReadFilter)
	if st := rf.OnNewConnection(); st != network.Continue {
		t.Errorf("OnNewConnection()=%v want Continue", st)
	}
}

func TestEchoOnDataWritesBackAndDrains(t *testing.T) {
	fif, _ := echo.New(&anypb.Any{TypeUrl: echo.TypeURL}, network.FactoryCtx{}) // empty body accepted
	rf := fif().(network.ReadFilter)
	cb := &fakeCallbacks{}
	rf.SetReadFilterCallbacks(cb)

	// endStream=false: basic write-back and drain.
	buf := &network.Buffer{}
	buf.Append([]byte("ping"))
	st := rf.OnData(buf, false)
	if st != network.StopIteration {
		t.Errorf("OnData status=%v want StopIteration", st)
	}
	if string(cb.conn.written) != "ping" {
		t.Errorf("echo wrote %q want ping", cb.conn.written)
	}
	if buf.Len() != 0 {
		t.Errorf("echo did not drain buffer, Len=%d", buf.Len())
	}
	if cb.conn.endStream != false {
		t.Errorf("endStream propagated as true, want false")
	}

	// endStream=true: echo must forward the downstream end_stream flag.
	cb2 := &fakeCallbacks{}
	rf2 := fif().(network.ReadFilter)
	rf2.SetReadFilterCallbacks(cb2)
	buf2 := &network.Buffer{}
	buf2.Append([]byte("fin"))
	rf2.OnData(buf2, true)
	if !cb2.conn.endStream {
		t.Errorf("endStream not forwarded: got false, want true")
	}
}
