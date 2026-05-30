package directresponse

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	drv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	"github.com/esalaine/envoy-go/internal/filter/network"
)

// ----------------------------------------------------------------------------
// Test doubles: a fakeConnection capturing Write(endStream)/Close(closeType)
// and a fakeCallbacks implementing network.ReadFilterCallbacks plus the
// optional SetResponseCodeDetails(string) sink the filter type-asserts for.
// ----------------------------------------------------------------------------

type fakeConnection struct {
	written   []byte
	endStream bool
	closeType network.CloseType
	closed    bool
}

func (c *fakeConnection) Write(data []byte, endStream bool) {
	c.written = append(c.written, data...)
	c.endStream = endStream
}
func (c *fakeConnection) Close(ct network.CloseType)     { c.closeType = ct; c.closed = true }
func (c *fakeConnection) LocalAddr() net.Addr            { return nil }
func (c *fakeConnection) RemoteAddr() net.Addr           { return nil }
func (c *fakeConnection) RequestedServerName() string    { return "" }
func (c *fakeConnection) DownstreamPrincipals() []string { return nil }

type fakeCallbacks struct {
	conn fakeConnection
	rcd  string
}

func (cb *fakeCallbacks) Connection() network.Connection { return &cb.conn }
func (cb *fakeCallbacks) ContinueReading()               {}
func (cb *fakeCallbacks) DynamicMetadata() *dynamicmetadata.Bucket {
	return nil
}

// SetResponseCodeDetails is the optional RCD sink the filter type-asserts for.
// Recording it here proves the assertion path is live in tests.
func (cb *fakeCallbacks) SetResponseCodeDetails(s string) { cb.rcd = s }

func TestDirectResponseInlineStringWritesAndCloses(t *testing.T) {
	cfg := &drv3.Config{Response: &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineString{InlineString: "BYE\n"}}}
	any, err := anypb.New(cfg)
	if err != nil {
		t.Fatalf("anypb.New = %v", err)
	}
	fif, err := New(any, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	rf := fif()
	cb := &fakeCallbacks{}
	rf.SetReadFilterCallbacks(cb)
	st := rf.OnNewConnection()
	if st != network.StopIteration {
		t.Errorf("status=%v want StopIteration", st)
	}
	if string(cb.conn.written) != "BYE\n" {
		t.Errorf("wrote %q want BYE", cb.conn.written)
	}
	if !cb.conn.endStream {
		t.Errorf("write endStream not set")
	}
	if cb.conn.closeType != network.FlushWrite {
		t.Errorf("close type=%v want FlushWrite", cb.conn.closeType)
	}
	if !cb.conn.closed {
		t.Errorf("connection not closed")
	}
	if cb.rcd != responseCodeDetails {
		t.Errorf("RCD=%q want %q", cb.rcd, responseCodeDetails)
	}
}

func TestDirectResponseFilenameRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "body.txt"), []byte("FILE-BODY"), 0o600); err != nil {
		t.Fatalf("WriteFile = %v", err)
	}
	cfg := &drv3.Config{Response: &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: "body.txt"}}}
	any, err := anypb.New(cfg)
	if err != nil {
		t.Fatalf("anypb.New = %v", err)
	}
	fif, err := New(any, network.FactoryCtx{BaseDir: dir})
	if err != nil {
		t.Fatalf("New(Filename) = %v", err)
	}
	cb := &fakeCallbacks{}
	rf := fif()
	rf.SetReadFilterCallbacks(cb)
	rf.OnNewConnection()
	if string(cb.conn.written) != "FILE-BODY" {
		t.Errorf("wrote %q want FILE-BODY", cb.conn.written)
	}
}

// ----------------------------------------------------------------------------
// Task 9: PARSE-REJECT arms + byte-stable wording table (§6.1; D-P26.1-3).
// ----------------------------------------------------------------------------

// TestParseRejectConstants_ByteStable pins the operator-visible reject wording.
// The Filename / EnvVar arms carry %-verbs (finalized empirically in Task 16 /
// D-P26.1-4); only the verb-free SpecifierRequired arm is pinned here.
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"SpecifierRequired", parseRejectResponseSpecifierRequired, "direct_response: response.specifier is required"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("byte-stable drift: %s\n const: %q\n  want: %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestDirectResponseRejectsUnsetSpecifier(t *testing.T) {
	cfg := &drv3.Config{Response: &corev3.DataSource{}} // specifier nil
	any, err := anypb.New(cfg)
	if err != nil {
		t.Fatalf("anypb.New = %v", err)
	}
	_, err = New(any, network.FactoryCtx{})
	if err == nil || err.Error() != parseRejectResponseSpecifierRequired {
		t.Fatalf("expected specifier-required reject, got %v", err)
	}
}

func TestDirectResponseRejectsAbsentResponse(t *testing.T) {
	any, err := anypb.New(&drv3.Config{}) // Response nil
	if err != nil {
		t.Fatalf("anypb.New = %v", err)
	}
	_, err = New(any, network.FactoryCtx{})
	if err == nil {
		t.Fatalf("expected reject for absent response")
	}
}
