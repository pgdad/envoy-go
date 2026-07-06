package lua

// httpcall_test.go — Task 11 (phase 22.2 IMPL) behavioral tests for the
// :httpCall(cluster, headers, body, timeout_ms, asynchronous?) bridge
// per 22.2 PLAN Task 11 + 22.2 SPEC §3.4 + §11.7 D6 closure +
// AMEND-22.2-3 (PURE FIRE-AND-FORGET on async=true) + §6 arm-20
// (httpcall-cluster-name-required byte-stable wording per W2).
//
// 8 test functions per the PLAN Task 11 Step 1 enumeration:
//   - Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording
//   - Test_HTTPCall_sync_happy_path_roundtrip
//   - Test_HTTPCall_sync_timeout_increments_httpcall_timeouts
//   - Test_HTTPCall_sync_5xx_increments_httpcall_failures
//   - Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield
//   - Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts
//   - Test_HTTPCall_total_counter_covers_sync_and_async
//   - Test_HTTPCall_coroutine_yield_resume_timing_sync

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/httpclient"
	luaprim "github.com/pgdad/envoy-go/internal/lua"
	"github.com/pgdad/envoy-go/internal/stats"
)

// newHTTPCallBridgeFilter constructs a per-test *filter with the
// httpCall-bridge scaffolding wired: a VM + bridge metatables + request_handle
// + response_handle contexts bound to globals rh / resp + a 5-counter
// hand-rolled filterStats (so tests can assert on the 3 NEW Task 11
// counters + the 2 Task 7 counters). httpClient + clusterMgr are passed
// in; nil values are tolerated for the arm-20 / no-plumbing test paths.
func newHTTPCallBridgeFilter(t *testing.T, httpClient *httpclient.Client, clusterMgr *cluster.Manager) *filter {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats: &filterStats{
			errors:                 reg.NewCounter("test.errors"),
			executions:             reg.NewCounter("test.executions"),
			respondCalls:           reg.NewCounter("test.respond_calls"),
			bodyBufferedBytesTotal: reg.NewCounter("test.body_buffered_bytes_total"),
			coroutineYieldsTotal:   reg.NewCounter("test.coroutine_yields_total"),
			httpcallTotal:          reg.NewCounter("test.httpcall_total"),
			httpcallFailures:       reg.NewCounter("test.httpcall_failures"),
			httpcallTimeouts:       reg.NewCounter("test.httpcall_timeouts"),
		},
	}
	f := &filter{cc: cc, httpClient: httpClient, clusterMgr: clusterMgr}
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	f.vm = vm

	ctx, cancelCtx := context.WithCancel(context.Background())
	t.Cleanup(cancelCtx)
	vm.State().SetContext(ctx)

	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)

	f.reqCtx = &requestHandleContext{headers: nil, filterRef: f}
	rud := L.NewUserData()
	rud.Value = f.reqCtx
	L.SetMetatable(rud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", rud)

	f.respCtx = &responseHandleContext{headers: nil, filterRef: f}
	pud := L.NewUserData()
	pud.Value = f.respCtx
	L.SetMetatable(pud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", pud)

	return f
}

// httpCallSplitHostPort parses a "host:port" into (host, portUint32).
// Mirrors httpclient_test.go::splitHostPort.
func httpCallSplitHostPort(t *testing.T, addr string) (string, uint32) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	return host, uint32(p)
}

// httpCallMkClusterMgr builds a *cluster.Manager with a single plaintext
// STATIC cluster `name` → host:port. Mirrors httpclient_test.go::
// mkPlainClusterMgr.
func httpCallMkClusterMgr(t *testing.T, name, host string, port uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
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
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// driveSyncHTTPCall encapsulates the sync :httpCall yield+resume timing.
// Mints a child *LState, calls parent.Resume(child, fn) which runs the
// LGFunction up to the YieldFromBridge point, waits on f.httpCallDone
// for the dispatch goroutine's Resume call to complete, and returns the
// resume state at observation time. Mirrors body_test.go's pattern but
// with the goroutine-coordinated dispatch model.
func driveSyncHTTPCall(t *testing.T, f *filter, fnName string) {
	t.Helper()
	child, cancel := f.vm.NewThread()
	if cancel != nil {
		defer cancel()
	}
	fnVal := f.vm.State().GetGlobal(fnName)
	fn, ok := fnVal.(*lua.LFunction)
	if !ok {
		t.Fatalf("global %q is not a function (got %v)", fnName, fnVal)
	}
	state, rerr, _ := f.vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("parent.Resume[1] err = %v; want nil", rerr)
	}
	if state != lua.ResumeYield {
		t.Fatalf("parent.Resume[1] state = %v; want ResumeYield (sync httpCall must yield)", state)
	}
	// Release the dispatch goroutine's gate now that the outer Resume has
	// returned — per §11.1 D2 goroutine-safety the gate prevents the
	// dispatch goroutine from touching the child *LState until the outer
	// Resume's switchToParentThread Push has unwound. Production HCM
	// dispatch coordination at a future phase does the same gate-close.
	close(f.httpCallReady)
	// Wait for dispatch goroutine to complete its Resume call. Bounded
	// to avoid hangs under regression.
	select {
	case <-f.httpCallDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for httpCall dispatch goroutine completion")
	}
}

// -----------------------------------------------------------------------
// Test 1: empty cluster raises arm-20 byte-stable wording per SPEC §6 + W2.
// -----------------------------------------------------------------------

func Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording(t *testing.T) {
	f := newHTTPCallBridgeFilter(t, nil, nil)
	chunk, err := luaprim.CompileScript([]byte(`rh:httpCall("", {}, "", 1000)`), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	err = f.vm.Run(chunk)
	if err == nil {
		t.Fatalf("vm.Run err = nil; want runtime error (arm-20)")
	}
	want := "lua: httpCall: cluster name must not be empty"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %q; want substring %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------
// Test 2: sync happy-path roundtrip — backend returns 200 + body; script
// gets (headers_table, body_string) via the dispatch goroutine's Resume.
// -----------------------------------------------------------------------

func Test_HTTPCall_sync_happy_path_roundtrip(t *testing.T) {
	const wantBody = "sync-happy-body"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "abc")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer srv.Close()
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_happy", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	f := newHTTPCallBridgeFilter(t, c, cm)

	src := `
function consume()
    local hdrs, body = rh:httpCall("c_happy", {[":method"]="GET", [":path"]="/x"}, "", 5000)
    capturedStatus = hdrs[":status"]
    capturedCustom = hdrs["x-custom"]
    capturedBody = body
end
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v", err)
	}

	driveSyncHTTPCall(t, f, "consume")

	status := f.vm.State().GetGlobal("capturedStatus")
	if got := status.String(); got != "200" {
		t.Fatalf("capturedStatus = %q; want %q", got, "200")
	}
	custom := f.vm.State().GetGlobal("capturedCustom")
	if got := custom.String(); got != "abc" {
		t.Fatalf("capturedCustom = %q; want %q", got, "abc")
	}
	body := f.vm.State().GetGlobal("capturedBody")
	if got := body.String(); got != wantBody {
		t.Fatalf("capturedBody = %q; want %q", got, wantBody)
	}
}

// -----------------------------------------------------------------------
// Test 3: sync timeout increments httpcall_timeouts (+ httpcall_failures
// since transport-level err != nil) per SPEC §11.7 D6 + AMEND-22.2-3.
// -----------------------------------------------------------------------

func Test_HTTPCall_sync_timeout_increments_httpcall_timeouts(t *testing.T) {
	// Server hangs longer than the per-call timeout (50ms). LIFO defer
	// order matters: srv.Close() must run AFTER close(hang) so the hung
	// handler can unblock + close finishes cleanly.
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-hang // unblocked at test exit via close(hang) below
	}))
	defer srv.Close()
	defer close(hang)
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_hang", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 0}) // no client-level timeout; rely on per-call ctx
	f := newHTTPCallBridgeFilter(t, c, cm)

	src := `
function consume()
    local hdrs, body = rh:httpCall("c_hang", {[":method"]="GET", [":path"]="/x"}, "", 50)
    captured_hdrs_is_nil = (hdrs == nil)
    captured_err = body or ""
end
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v", err)
	}

	beforeTimeouts := f.cc.stats.httpcallTimeouts.Load()
	beforeFailures := f.cc.stats.httpcallFailures.Load()
	driveSyncHTTPCall(t, f, "consume")
	afterTimeouts := f.cc.stats.httpcallTimeouts.Load()
	afterFailures := f.cc.stats.httpcallFailures.Load()

	if afterTimeouts-beforeTimeouts != 1 {
		t.Fatalf("httpcall_timeouts delta = %d; want 1", afterTimeouts-beforeTimeouts)
	}
	if afterFailures-beforeFailures != 1 {
		t.Fatalf("httpcall_failures delta = %d; want 1 (timeout is also a failure)", afterFailures-beforeFailures)
	}
	// Sanity: script saw the (nil, err) shape.
	if v := f.vm.State().GetGlobal("captured_hdrs_is_nil"); v != lua.LTrue {
		t.Fatalf("captured_hdrs_is_nil = %v; want true (sync timeout → nil hdrs)", v)
	}
}

// -----------------------------------------------------------------------
// Test 4: sync 5xx increments httpcall_failures (per upstream synthetic-
// 503 parity disposition).
// -----------------------------------------------------------------------

func Test_HTTPCall_sync_5xx_increments_httpcall_failures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // 502
		_, _ = io.WriteString(w, "bad-gateway")
	}))
	defer srv.Close()
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_502", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	f := newHTTPCallBridgeFilter(t, c, cm)

	src := `
function consume()
    local hdrs, body = rh:httpCall("c_502", {[":method"]="GET", [":path"]="/x"}, "", 5000)
    capturedStatus = hdrs[":status"]
    capturedBody = body
end
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v", err)
	}

	beforeFailures := f.cc.stats.httpcallFailures.Load()
	beforeTimeouts := f.cc.stats.httpcallTimeouts.Load()
	driveSyncHTTPCall(t, f, "consume")
	afterFailures := f.cc.stats.httpcallFailures.Load()
	afterTimeouts := f.cc.stats.httpcallTimeouts.Load()

	if afterFailures-beforeFailures != 1 {
		t.Fatalf("httpcall_failures delta = %d; want 1 (5xx is a failure)", afterFailures-beforeFailures)
	}
	if afterTimeouts != beforeTimeouts {
		t.Fatalf("httpcall_timeouts incremented on 5xx (got delta %d); want 0", afterTimeouts-beforeTimeouts)
	}
	// Script still observed the response (headers + body), NOT (nil, err).
	if v := f.vm.State().GetGlobal("capturedStatus"); v.String() != "502" {
		t.Fatalf("capturedStatus = %q; want %q", v.String(), "502")
	}
}

// -----------------------------------------------------------------------
// Test 5: async=true returns 0 values + does NOT yield. The script
// continues execution synchronously per AMEND-22.2-3.
// -----------------------------------------------------------------------

func Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield(t *testing.T) {
	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_async", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	f := newHTTPCallBridgeFilter(t, c, cm)

	// The script calls :httpCall with asynchronous=true. The call site
	// MUST return 0 values; the script continues synchronously. Capture
	// a flag both before AND after the call to prove no yield occurred.
	src := `
before_call = true
rh:httpCall("c_async", {[":method"]="POST", [":path"]="/y"}, "payload", 5000, true)
after_call = true
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	// Run directly via vm.Run — NOT via a coroutine. If async incorrectly
	// yields, vm.Run would surface a script-side error or the script's
	// `after_call` would not be set. vm.Run runs the chunk to completion
	// without a coroutine envelope, so YieldFromBridge from the bridge
	// would crash with "attempt to yield from outside a coroutine".
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v (async path must NOT yield)", err)
	}

	// `after_call` must be set (proves no yield occurred).
	if v := f.vm.State().GetGlobal("after_call"); v != lua.LTrue {
		t.Fatalf("after_call = %v; want true (async must not yield)", v)
	}

	// Wait briefly for the async goroutine to dispatch + the backend to
	// observe the request. Bounded retry; non-deterministic by design but
	// the request volume is 1 so the test is reliable in CI.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got.Load() != 1 {
		t.Fatalf("backend observed %d requests; want 1 (async dispatch did not reach backend)", got.Load())
	}
}

// -----------------------------------------------------------------------
// Test 6: async transport-failure does NOT increment failures/timeouts
// per AMEND-22.2-3 D6 closure (async failures invisible at filter-stats
// per upstream parity). httpcall_total STILL increments.
// -----------------------------------------------------------------------

func Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts(t *testing.T) {
	// Construct a cluster pointing at an unroutable address that will
	// trigger transport-failure (connect refused at high local port).
	cm := httpCallMkClusterMgr(t, "c_dead", "127.0.0.1", 1)
	c := httpclient.New(httpclient.Options{Timeout: 200 * time.Millisecond})
	f := newHTTPCallBridgeFilter(t, c, cm)

	src := `
rh:httpCall("c_dead", {[":method"]="GET", [":path"]="/x"}, "", 100, true)
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}

	beforeTotal := f.cc.stats.httpcallTotal.Load()
	beforeFailures := f.cc.stats.httpcallFailures.Load()
	beforeTimeouts := f.cc.stats.httpcallTimeouts.Load()

	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v", err)
	}

	// Wait for the goroutine to finish (no signal channel for async; sleep
	// covering both connect-refused and timeout-ms windows).
	time.Sleep(500 * time.Millisecond)

	afterTotal := f.cc.stats.httpcallTotal.Load()
	afterFailures := f.cc.stats.httpcallFailures.Load()
	afterTimeouts := f.cc.stats.httpcallTimeouts.Load()

	if afterTotal-beforeTotal != 1 {
		t.Fatalf("httpcall_total delta = %d; want 1 (async still counts as dispatch)", afterTotal-beforeTotal)
	}
	if afterFailures != beforeFailures {
		t.Fatalf("httpcall_failures incremented on async transport-failure (delta %d); want 0 per AMEND-22.2-3", afterFailures-beforeFailures)
	}
	if afterTimeouts != beforeTimeouts {
		t.Fatalf("httpcall_timeouts incremented on async timeout (delta %d); want 0 per AMEND-22.2-3", afterTimeouts-beforeTimeouts)
	}
}

// -----------------------------------------------------------------------
// Test 7: httpcall_total covers BOTH sync AND async dispatches per
// SPEC §7.1 + §11.7 D6.
// -----------------------------------------------------------------------

func Test_HTTPCall_total_counter_covers_sync_and_async(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_total", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	f := newHTTPCallBridgeFilter(t, c, cm)

	before := f.cc.stats.httpcallTotal.Load()

	// 1 sync dispatch via coroutine.
	syncSrc := `function go1() rh:httpCall("c_total", {[":method"]="GET",[":path"]="/"}, "", 5000) end`
	chunk1, err := luaprim.CompileScript([]byte(syncSrc), nil)
	if err != nil {
		t.Fatalf("CompileScript[sync]: %v", err)
	}
	if err := f.vm.Run(chunk1); err != nil {
		t.Fatalf("vm.Run[sync]: %v", err)
	}
	driveSyncHTTPCall(t, f, "go1")

	// 1 async dispatch in the main thread.
	asyncSrc := `rh:httpCall("c_total", {[":method"]="GET",[":path"]="/"}, "", 5000, true)`
	chunk2, err := luaprim.CompileScript([]byte(asyncSrc), nil)
	if err != nil {
		t.Fatalf("CompileScript[async]: %v", err)
	}
	if err := f.vm.Run(chunk2); err != nil {
		t.Fatalf("vm.Run[async]: %v", err)
	}
	// Sleep briefly so the async goroutine's dispatch fires before we
	// observe the counter (httpcall_total increments synchronously in the
	// LGFunction body — before goroutine spawn — but we still cover the
	// "dispatch fires" sub-case for fixture parity).
	time.Sleep(200 * time.Millisecond)

	after := f.cc.stats.httpcallTotal.Load()
	delta := after - before
	if delta != 2 {
		t.Fatalf("httpcall_total delta = %d; want 2 (1 sync + 1 async)", delta)
	}
}

// -----------------------------------------------------------------------
// Test 8: sync coroutine yield+resume timing — script yields on the
// httpCall LGFunction; parent.Resume returns ResumeYield; dispatch
// goroutine fires Resume with (headers, body); script continues.
// -----------------------------------------------------------------------

func Test_HTTPCall_coroutine_yield_resume_timing_sync(t *testing.T) {
	const wantBody = "yield-resume-body"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer srv.Close()
	host, port := httpCallSplitHostPort(t, srv.Listener.Addr().String())
	cm := httpCallMkClusterMgr(t, "c_yield", host, port)
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	f := newHTTPCallBridgeFilter(t, c, cm)

	src := `
function consume()
    yielded_before = true
    local hdrs, body = rh:httpCall("c_yield", {[":method"]="GET",[":path"]="/"}, "", 5000)
    after_resume_body = body
end
`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run: %v", err)
	}

	beforeYields := f.cc.stats.coroutineYieldsTotal.Load()

	// Custom drive to observe the ResumeYield state at the parent.Resume
	// return site (proves timing — body_test.go test 4 idiom but for
	// httpCall with a dispatch goroutine).
	child, cancel := f.vm.NewThread()
	if cancel != nil {
		defer cancel()
	}
	fn := f.vm.State().GetGlobal("consume").(*lua.LFunction)
	state, rerr, _ := f.vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("parent.Resume[1] err = %v; want nil", rerr)
	}
	if state != lua.ResumeYield {
		t.Fatalf("parent.Resume[1] state = %v; want ResumeYield (sync httpCall must yield)", state)
	}
	if v := f.vm.State().GetGlobal("yielded_before"); v != lua.LTrue {
		t.Fatalf("yielded_before = %v; want true (script executed up to yield)", v)
	}

	// Release the dispatch goroutine's gate — per §11.1 D2 goroutine-safety
	// the gate prevents the dispatch goroutine from touching the child
	// *LState until the outer Resume has unwound.
	close(f.httpCallReady)

	// Wait for dispatch goroutine to complete its Resume call.
	select {
	case <-f.httpCallDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for httpCall dispatch goroutine completion")
	}

	// Script continued after resume — observed the body via the
	// after_resume_body global.
	got := f.vm.State().GetGlobal("after_resume_body")
	if got.String() != wantBody {
		t.Fatalf("after_resume_body = %q; want %q", got.String(), wantBody)
	}

	// coroutine_yields_total incremented ONCE per yield event.
	afterYields := f.cc.stats.coroutineYieldsTotal.Load()
	if afterYields-beforeYields != 1 {
		t.Fatalf("coroutine_yields_total delta = %d; want 1 (ONCE per yield)", afterYields-beforeYields)
	}
}
