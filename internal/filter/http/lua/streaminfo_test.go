package lua

// streaminfo_test.go — Task 13 (phase 22.2 IMPL) streamInfo extension
// tests per SPEC §3.4 + PLAN Task 13 acceptance. Covers the 5 NEW
// methods on top of the 6 (4 + 2) already exercised at bridge_test.go +
// metadata_test.go:
//
//   - :upstreamHost()            — Task 13
//   - :upstreamCluster()         — Task 13
//   - :requestedServerName()     — Task 13 (derived from TLS SNI when present)
//   - :filterState()             — Task 13 (returns filterstate userdata;
//                                  detailed get/set lives at
//                                  filterstate_test.go)
//   - :downstreamSslConnection() — Task 13 (symmetric to :connection():ssl();
//                                  returns ssl userdata or lua.LNil for
//                                  plaintext)
//
// The 4 + 2 inherited methods (Task 8 + Task 9) remain tested at
// bridge_test.go (TestBridge_StreamInfo_* tests stay green post-
// extraction — compile-level move only; behavior is unchanged).
//
// Test discipline mirrors bridge_test.go's Task 8 pattern: a
// fakeCallbacksFull test-double satisfies the EXTENDED 10-method
// RequestHandleCallbacks interface (4 base + 2 Task 9 + 4 new for
// Task 13 — UpstreamHost / UpstreamCluster / RequestedServerName /
// FilterState; DownstreamTLSConnectionState already exists from
// Task 10). A per-test helper newBridgedVMWithFullCallbacks wires the
// test-double + ALL bridge metatables (request_handle +
// response_handle + headers + streamInfo + metadata + dynamicMetadata
// + connection + ssl + filterstate) so the script can exercise the
// full streamInfo surface.

import (
	"crypto/tls"
	"net/http"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// fakeCallbacksFull is the EXTENDED test-double satisfying the full
// 10-method RequestHandleCallbacks interface at Task 13. The 6 inherited
// methods (4 from Task 8 + 2 from Task 10's TLS extension) are wired via
// embedded fakeCallbacks; the 4 NEW fields (upstreamHost / upstreamCluster
// / requestedServerName / filterState) hold canned values per test.
//
// filterState is a per-test *map[string]any pointer so the bridge's
// :filterState():get/set can mutate the underlying map AND the test code
// can inspect the post-mutation map directly.
type fakeCallbacksFull struct {
	fakeCallbacks
	upstreamHost    string
	upstreamCluster string
	requestedSrv    string
	filterState     map[string]any
	tlsState        *tls.ConnectionState
	bucket          *dynamicmetadata.Bucket
}

func (f *fakeCallbacksFull) UpstreamHost() string                               { return f.upstreamHost }
func (f *fakeCallbacksFull) UpstreamCluster() string                            { return f.upstreamCluster }
func (f *fakeCallbacksFull) RequestedServerName() string                        { return f.requestedSrv }
func (f *fakeCallbacksFull) FilterState() map[string]any                        { return f.filterState }
func (f *fakeCallbacksFull) SetFilterState(m map[string]any)                    { f.filterState = m }
func (f *fakeCallbacksFull) DownstreamTLSConnectionState() *tls.ConnectionState { return f.tlsState }
func (f *fakeCallbacksFull) DynamicMetadata() *dynamicmetadata.Bucket           { return f.bucket }

// newBridgedVMWithFullCallbacks constructs a VM with the full Task 13
// bridge metatable set (request_handle + response_handle + headers +
// streamInfo + metadata + dynamicMetadata + connection + ssl +
// filterstate) and wires the supplied fakeCallbacksFull into reqCtx.cb
// so the streamInfo extension surface is exercisable.
func newBridgedVMWithFullCallbacks(t *testing.T, cb *fakeCallbacksFull) *luaprim.VM {
	t.Helper()
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installMetadataMetatable(L)
	installDynamicMetadataMetatable(L)
	installConnectionMetatable(L)
	installSSLMetatable(L)
	installFilterStateMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: http.Header{}, cb: cb}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// ---------------------------------------------------------------------
// :upstreamHost()
// ---------------------------------------------------------------------

func TestStreamInfo_UpstreamHost_returns_canned_value(t *testing.T) {
	cb := &fakeCallbacksFull{upstreamHost: "10.0.0.42:8443"}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():upstreamHost()`)
	if got := getGlobalString(t, vm, "result"); got != "10.0.0.42:8443" {
		t.Fatalf(":upstreamHost() = %q; want %q", got, "10.0.0.42:8443")
	}
}

func TestStreamInfo_UpstreamHost_empty_when_unset(t *testing.T) {
	cb := &fakeCallbacksFull{} // upstreamHost = ""
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():upstreamHost()`)
	if got := getGlobalString(t, vm, "result"); got != "" {
		t.Fatalf(":upstreamHost() empty = %q; want %q", got, "")
	}
}

// ---------------------------------------------------------------------
// :upstreamCluster()
// ---------------------------------------------------------------------

func TestStreamInfo_UpstreamCluster_returns_canned_value(t *testing.T) {
	cb := &fakeCallbacksFull{upstreamCluster: "service_backend"}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():upstreamCluster()`)
	if got := getGlobalString(t, vm, "result"); got != "service_backend" {
		t.Fatalf(":upstreamCluster() = %q; want %q", got, "service_backend")
	}
}

func TestStreamInfo_UpstreamCluster_empty_when_unset(t *testing.T) {
	cb := &fakeCallbacksFull{}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():upstreamCluster()`)
	if got := getGlobalString(t, vm, "result"); got != "" {
		t.Fatalf(":upstreamCluster() empty = %q; want %q", got, "")
	}
}

// ---------------------------------------------------------------------
// :requestedServerName()
// ---------------------------------------------------------------------

func TestStreamInfo_RequestedServerName_returns_canned_value(t *testing.T) {
	cb := &fakeCallbacksFull{requestedSrv: "example.envoy-go.local"}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():requestedServerName()`)
	if got := getGlobalString(t, vm, "result"); got != "example.envoy-go.local" {
		t.Fatalf(":requestedServerName() = %q; want %q", got, "example.envoy-go.local")
	}
}

func TestStreamInfo_RequestedServerName_empty_for_plaintext(t *testing.T) {
	cb := &fakeCallbacksFull{} // requestedSrv = ""
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `result = rh:streamInfo():requestedServerName()`)
	if got := getGlobalString(t, vm, "result"); got != "" {
		t.Fatalf(":requestedServerName() empty = %q; want %q", got, "")
	}
}

// ---------------------------------------------------------------------
// :filterState() — returns userdata wrapping per-stream map (the
// detailed :get/:set marshaling tests live at filterstate_test.go)
// ---------------------------------------------------------------------

func TestStreamInfo_FilterState_returns_userdata(t *testing.T) {
	cb := &fakeCallbacksFull{filterState: map[string]any{}}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `
		local fs = rh:streamInfo():filterState()
		result_type = type(fs)
		result_is_nil = (fs == nil)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(rh:streamInfo():filterState()) = %q; want %q", got, "userdata")
	}
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LFalse {
		t.Fatalf("filterState() == nil; want non-nil userdata")
	}
}

// ---------------------------------------------------------------------
// :downstreamSslConnection() — symmetric to :connection():ssl(); returns
// ssl userdata on TLS, lua.LNil for plaintext.
// ---------------------------------------------------------------------

func TestStreamInfo_DownstreamSslConnection_returns_ssl_userdata_when_tls(t *testing.T) {
	state := stateWithPeerCert(t)
	cb := &fakeCallbacksFull{tlsState: state}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `
		local s = rh:streamInfo():downstreamSslConnection()
		result_type = type(s)
		result_is_nil = (s == nil)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(downstreamSslConnection()) = %q; want %q (TLS path)", got, "userdata")
	}
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LFalse {
		t.Fatalf("downstreamSslConnection() == nil on TLS; want userdata")
	}
}

func TestStreamInfo_DownstreamSslConnection_returns_nil_for_plaintext(t *testing.T) {
	cb := &fakeCallbacksFull{} // tlsState = nil
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `
		local s = rh:streamInfo():downstreamSslConnection()
		result_is_nil = (s == nil)
	`)
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LTrue {
		t.Fatalf("downstreamSslConnection() = non-nil on plaintext; want nil")
	}
}

// TestStreamInfo_DownstreamSslConnection_dispatches_to_ssl_methods asserts
// that the returned ssl userdata is the same shape exposed via
// :connection():ssl() — the 12 ssl methods are callable on the returned
// value. Spot-check via :tlsVersion() (a stateless wrapper method).
func TestStreamInfo_DownstreamSslConnection_dispatches_to_ssl_methods(t *testing.T) {
	state := stateWithPeerCert(t)
	cb := &fakeCallbacksFull{tlsState: state}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `
		local s = rh:streamInfo():downstreamSslConnection()
		result_version = s:tlsVersion()
	`)
	if got := getGlobalString(t, vm, "result_version"); got == "" {
		t.Fatalf(":tlsVersion() on streamInfo():downstreamSslConnection() = %q; want non-empty", got)
	}
}

// ---------------------------------------------------------------------
// Comprehensive 11-method surface (sanity)
// ---------------------------------------------------------------------

// TestStreamInfo_11_method_surface_all_present runs each of the 11
// :streamInfo() methods on the same VM and asserts that none crash + each
// has the expected return shape (string for the 8 string-returning methods;
// userdata/nil for filterState and downstreamSslConnection).
func TestStreamInfo_11_method_surface_all_present(t *testing.T) {
	cb := &fakeCallbacksFull{
		fakeCallbacks: fakeCallbacks{
			proto:      "HTTP/2",
			route:      "ingress",
			localAddr:  "127.0.0.1:443",
			remoteAddr: "10.0.0.1:54321",
		},
		upstreamHost:    "10.0.0.42:8443",
		upstreamCluster: "service_backend",
		requestedSrv:    "example.envoy-go.local",
		filterState:     map[string]any{},
		tlsState:        stateWithPeerCert(t),
		bucket:          dynamicmetadata.NewBucket(),
	}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	runScript(t, vm, `
		local si = rh:streamInfo()
		proto    = si:protocol()
		route    = si:routeName()
		laddr    = si:downstreamLocalAddress()
		raddr    = si:downstreamDirectRemoteAddress()
		uhost    = si:upstreamHost()
		uclus    = si:upstreamCluster()
		srvn     = si:requestedServerName()
		dm       = si:dynamicMetadata()
		dtm      = si:dynamicTypedMetadata("foo")
		fs       = si:filterState()
		ssl      = si:downstreamSslConnection()
	`)
	for _, name := range []string{"proto", "route", "laddr", "raddr", "uhost", "uclus", "srvn"} {
		v := vm.State().GetGlobal(name)
		if _, ok := v.(lua.LString); !ok {
			t.Errorf("global %q type = %s; want string", name, v.Type())
		}
	}
	// :dynamicMetadata() always returns userdata wrapping the bucket.
	if v := vm.State().GetGlobal("dm"); v.Type() != lua.LTUserData {
		t.Errorf("global %q type = %s; want userdata", "dm", v.Type())
	}
	// :filterState() returns userdata.
	if v := vm.State().GetGlobal("fs"); v.Type() != lua.LTUserData {
		t.Errorf("global %q type = %s; want userdata", "fs", v.Type())
	}
	// :downstreamSslConnection() returns userdata on TLS path.
	if v := vm.State().GetGlobal("ssl"); v.Type() != lua.LTUserData {
		t.Errorf("global %q type = %s; want userdata (TLS path)", "ssl", v.Type())
	}
}
