// http_call_response_cache_test.go — phase 82 Task 10: end-to-end exercise of
// the proxy_http_call response cache through the PRODUCTION abiCallbacks
// dispatch path.
//
// Two layers, both against a capability-ENABLED *compiledConfig
// (newTestCompiledConfigWithCaps): the package's other dispatch tests are
// capability-DENIED and go vacuously green, so a cap-denied build here would
// prove nothing.
//
//	Layer 1 (TestHttpCallResponseCache_RealGuest_ReadsStatusThroughHostcalls)
//	  drives the REAL VENDORED GUEST BLOB from differential fixture 0036
//	  scenario (l) with NOTHING STUBBED on either side: a real cluster.Manager
//	  + httpclient.Client + a local backend, so the guest's proxy_http_call goes
//	  out over a socket and the real handleHttpCallResponse publishes the cache.
//	  The guest's on_http_call_response does
//	      let headers = self.get_http_call_response_headers();
//	      for (k, v) in headers.iter() { if k == ":status" { ... } }
//	  which reaches the host as proxy_get_header_map_pairs(map_type=6) and lands
//	  in abiCallbacks.GetHeaderMap through the wazero host shims. The guest then
//	  republishes what it read as `x-httpcall-status`, so the assertion reads a
//	  value that PROVABLY round-tripped through the guest.
//
//	  The full producer is REQUIRED here, not merely nicer: with a Stats-only
//	  FactoryCtx the HTTPDispatcher is not wired, proxy_http_call returns
//	  InternalFailure, the proxy-wasm-rust-sdk records no token, and
//	  proxy_on_http_call_response TRAPS inside the sdk's own token lookup before
//	  reaching any host response wiring. MEASURED — that was this test's first
//	  red.
//
//	Layer 2 (TestHttpCallResponseCache_AbiSurfaces_ReadOnlyAndCleared)
//	  covers the surfaces the vendored guest does not touch, against a directly
//	  published cache: the trailers map (type 7), the response body buffer
//	  (type 4), the value-expanded pair count, all four mutators returning
//	  Unimplemented for both read-only map types, and the cleared-cache posture.

package wasm

import (
	"context"
	"net"
	gohttp "net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/stats"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// httpCallGuestHostcallCaps is the hostcall capability roster the vendored
// scenario-(l) guest needs. Copied from the fixture's own allowed_capabilities
// block (test/fixtures/0036-http-wasm-body-and-advanced/envoy-go.yaml) — a
// hostcall whose capability is denied is not registered as a host import, so a
// guest importing it fails to INSTANTIATE rather than failing an assertion.
var httpCallGuestHostcallCaps = []string{
	"proxy_log",
	"proxy_get_log_level",
	"proxy_get_header_map_pairs",
	"proxy_get_header_map_value",
	"proxy_get_header_map_size",
	"proxy_set_header_map_pairs",
	"proxy_add_header_map_value",
	"proxy_replace_header_map_value",
	"proxy_remove_header_map_value",
	"proxy_send_local_response",
	"proxy_get_property",
	"proxy_set_property",
	"proxy_get_status",
	"proxy_get_current_time_nanoseconds",
	"proxy_set_effective_context",
	"proxy_done",
	"proxy_get_buffer_bytes",
	"proxy_set_buffer_bytes",
	"proxy_get_buffer_status",
	"proxy_continue_stream",
	"proxy_close_stream",
	"proxy_set_tick_period_milliseconds",
	"proxy_http_call",
	"proxy_call_foreign_function",
	"proxy_define_metric",
	"proxy_increment_metric",
	"proxy_record_metric",
	"proxy_get_metric",
	"proxy_set_shared_data",
	"proxy_get_shared_data",
}

// loadHttpCallSuccessGuest reads the vendored scenario-(l) guest blob.
func loadHttpCallSuccessGuest(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "..", "..",
		"test", "fixtures", "0036-http-wasm-body-and-advanced",
		"bytecode", "l_httpcall_success.wasm")
	b, err := os.ReadFile(p) //nolint:gosec // fixed in-repo test fixture path.
	if err != nil {
		t.Fatalf("reading vendored guest blob %s: %v", p, err)
	}
	if len(b) == 0 {
		t.Fatalf("vendored guest blob %s is EMPTY", p)
	}
	return b
}

// newCachedHTTPCallResponse builds the response the tests publish. It is
// deliberately shaped so key count and value count DIFFER (x-multi carries two
// values), and so the status is a value no default could produce.
func newCachedHTTPCallResponse() *internalwasm.HTTPCallResponse {
	return &internalwasm.HTTPCallResponse{
		Headers: gohttp.Header{
			":status":      []string{"207"},
			"Content-Type": []string{"application/json"},
			"X-Multi":      []string{"one", "two"},
		},
		Trailers: gohttp.Header{
			"X-Trailer": []string{"tv"},
		},
		Body: []byte(`{"cached":true}`),
	}
}

// -----------------------------------------------------------------------------
// Layer 1 — the real vendored guest, driven by the REAL PRODUCER end to end.
// -----------------------------------------------------------------------------

// httpCallBackendPort is drawn from this agent's reserved port band
// (42550-42599) so a concurrent sibling agent cannot collide with it.
const httpCallBackendPort = 42552

// startHttpCallBackend brings up the upstream the guest's proxy_http_call
// reaches. It answers 207 with a MULTI-VALUE header so the value count and the
// key count of the response differ, and the 207 is a status nothing else in
// this test could produce.
func startHttpCallBackend(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:"+itoa(httpCallBackendPort))
	if err != nil {
		t.Fatalf("listening on the reserved backend port %d: %v", httpCallBackendPort, err)
	}
	srv := &gohttp.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: gohttp.HandlerFunc(func(w gohttp.ResponseWriter, _ *gohttp.Request) {
			w.Header().Add("X-Multi", "one")
			w.Header().Add("X-Multi", "two")
			w.WriteHeader(207)
			_, _ = w.Write([]byte(`{"cached":true}`))
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// mkHttpCallClusterMgr builds a *cluster.Manager carrying a single STATIC
// cluster named `name` pointing at the local backend.
func mkHttpCallClusterMgr(t *testing.T, name string) *cluster.Manager {
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
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: httpCallBackendPort},
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

// TestHttpCallResponseCache_RealGuest_ReadsStatusThroughHostcalls is the
// full-stack Task 10 assertion, with NOTHING stubbed on the producer side:
//
//	real vendored guest -> proxy_http_call -> production wasmHTTPDispatcher ->
//	real cluster.Manager + httpclient.Client -> real local backend ->
//	handleHttpCallResponse (the phase-82 producer: `:status` synthesis, value
//	counts, publish + deferred clear) -> proxy_on_http_call_response ->
//	proxy_get_header_map_pairs(6) -> abiCallbacks.GetHeaderMap -> the guest
//
// The guest republishes what it read as `x-httpcall-status`, so the assertion
// reads a value that PROVABLY round-tripped through the guest rather than one
// the test placed there.
//
// This is the assertion that pins the EXACT KEY. The guest compares
// `k == ":status"` byte-for-byte: lowercase, colon-prefixed, un-canonicalized.
// A mangled or absent key leaves call_status at its INITIAL value 0, which
// surfaces as `x-httpcall-status: 0` — a distinguishable failure, not a silent
// pass. And since Go's net/http puts NO `:status` in resp.Header, a 207 here is
// only reachable through the phase-82 synthesis.
//
// WHY THE FULL PRODUCER IS REQUIRED (measured, not assumed): with a Stats-only
// FactoryCtx the dispatcher is not wired, proxy_http_call returns
// InternalFailure, the proxy-wasm-rust-sdk records NO token, and a subsequent
// proxy_on_http_call_response TRAPS inside the SDK's own token lookup
// (`core::option::expect_failed` in the wazero stack trace) before reaching any
// host response wiring. A stubbed-dispatch version of this test could not
// assert anything about the cache at all.
func TestHttpCallResponseCache_RealGuest_ReadsStatusThroughHostcalls(t *testing.T) {
	t.Parallel()
	startHttpCallBackend(t)

	reg := stats.NewRegistry()
	factoryCtx := envoyhttp.FactoryCtx{
		Stats:          reg,
		StatPrefix:     "ingress_http",
		ClusterManager: mkHttpCallClusterMgr(t, "cluster_b"),
		HTTPClient:     httpclient.New(httpclient.Options{Timeout: 10 * time.Second}),
	}

	cc := newTestCompiledConfigWithCapsAndFactoryCtx(t, loadHttpCallSuccessGuest(t),
		"plugin_l_cache", factoryCtx, httpCallGuestHostcallCaps...)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})

	reqHeaders := gohttp.Header{}
	reqHeaders.Set(":method", "GET")
	reqHeaders.Set(":path", "/scenario_l")
	_ = f.DecodeHeaders(reqHeaders, true)

	// NC1: the guest instantiated and bound a stream context. Without one there
	// is no dispatch at all and everything below would be vacuous.
	if f.streamCtx == nil {
		t.Fatal("NC1: filter.streamCtx = nil after DecodeHeaders; the guest never bound a stream context")
	}

	// NC2: proxy_http_call actually DISPATCHED. If it had returned
	// InternalFailure the sdk would hold no token and the callback would trap
	// in the sdk rather than exercising the cache. Poll the counter — the
	// dispatch and the response run on the http-call goroutine.
	if !waitForCounter(t, reg, "wasm.plugin_l_cache.http_call_dispatched", 1, 5*time.Second) {
		t.Fatalf("NC2: http_call_dispatched never reached 1 (got %d); proxy_http_call did not dispatch, so the response path was never entered",
			findStatCounterValue(reg, "wasm.plugin_l_cache.http_call_dispatched"))
	}

	// The response callback must complete WITHOUT trapping. Post phase-82
	// Task 4, http_call_response increments ONLY on a non-trapping return, so
	// this is simultaneously the "the guest consumed the response" gate.
	if !waitForCounter(t, reg, "wasm.plugin_l_cache.http_call_response", 1, 5*time.Second) {
		t.Fatalf("http_call_response never reached 1 (got %d); post-Task-4 that counter fires only on a NON-TRAPPING proxy_on_http_call_response, so the guest trapped or never ran. envoy_go.failures = %d",
			findStatCounterValue(reg, "wasm.plugin_l_cache.http_call_response"),
			findStatCounterValue(reg, "wasm.plugin_l_cache.envoy_go.failures"))
	}
	if got := findStatCounterValue(reg, "wasm.plugin_l_cache.envoy_go.failures"); got != 0 {
		t.Errorf("envoy_go.failures = %d; want 0 on the happy path", got)
	}

	// The cache is CALLBACK-SCOPED: once proxy_on_http_call_response returned,
	// the deferred clear in handleHttpCallResponse must have run.
	if after := cc.rootVM.HTTPCallResponse(); after != nil {
		t.Errorf("rootVM.HTTPCallResponse() = %+v after the callback completed; want nil (callback-scoped lifetime)", after)
	}

	// The guest writes what it READ onto the response headers.
	respHeaders := gohttp.Header{}
	respHeaders.Set(":status", "200")
	if got := f.EncodeHeaders(respHeaders, true); got != envoyhttp.Continue {
		t.Fatalf("EncodeHeaders = %v; want Continue", got)
	}

	switch got := respHeaders.Get("x-httpcall-status"); got {
	case "207":
		// The guest saw the synthesized `:status` under the exact key, carrying
		// the real backend's status code.
	case "0":
		t.Error(`x-httpcall-status = "0": the guest's call_status never moved off its INITIAL value — it found no ":status" key in the http-call response headers it read back through proxy_get_header_map_pairs(6). Go's net/http never emits one, so this means the phase-82 synthesis did not reach the guest.`)
	case "":
		t.Error("x-httpcall-status absent: the guest's on_http_response_headers did not run at all")
	default:
		t.Errorf("x-httpcall-status = %q; want %q (the real backend's status code, surfaced through the synthesized :status)", got, "207")
	}

	f.OnDestroy()
}

// waitForCounter polls the registry until the named counter reaches want or the
// budget expires. The http-call dispatch + response run on their own goroutine,
// so a bare read races them.
func waitForCounter(t *testing.T, reg *stats.Registry, name string, want uint64, budget time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if findStatCounterValue(reg, name) >= want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// -----------------------------------------------------------------------------
// Layer 2 — the ABI surfaces the vendored guest does not exercise.
// -----------------------------------------------------------------------------

// TestHttpCallResponseCache_AbiSurfaces_ReadOnlyAndCleared exercises the
// remaining consumer surfaces through the production abiCallbacks methods:
// header/trailer map reads at types 6/7, the body buffer at type 4, the
// value-expanded pair count, the read-only posture of all four mutators, and
// the cleared-cache posture.
func TestHttpCallResponseCache_AbiSurfaces_ReadOnlyAndCleared(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reg := stats.NewRegistry()

	cc := newTestCompiledConfigWithCaps(t, loadHttpCallSuccessGuest(t),
		"plugin_l_surfaces", reg, httpCallGuestHostcallCaps...)
	t.Cleanup(func() { _ = cc.Close() })

	f := &filter{cfg: cc}
	f.SetDecoderCallbacks(fakeDecoderCb{})
	f.SetEncoderCallbacks(fakeEncoderCb{})
	cb := &abiCallbacks{filter: f}

	const (
		hdrType  = abi.WasmHeaderMapTypeHttpCallResponseHeaders  // 6
		trlType  = abi.WasmHeaderMapTypeHttpCallResponseTrailers // 7
		bodyType = abi.WasmBufferTypeHttpCallResponseBody        // 4
	)

	// --- cache ABSENT: every surface reports absent, none of them panic ---
	if pairs, ok := cb.GetHeaderMap(ctx, 1, hdrType); ok {
		t.Errorf("pre-publish GetHeaderMap(6) ok=true pairs=%v; want ok=false (no call has completed)", pairs)
	}
	if buf, err := cb.GetBuffer(ctx, 1, bodyType); err != nil || buf != nil {
		t.Errorf("pre-publish GetBuffer(4) = (%v, %v); want (nil, nil)", buf, err)
	}

	// --- cache PUBLISHED ---
	cached := newCachedHTTPCallResponse()
	cc.rootVM.SetHTTPCallResponse(cached)

	// Headers: value-EXPANDED pair count. 1 (:status) + 1 (Content-Type) +
	// 2 (X-Multi) = 4 pairs across 3 keys.
	pairs, ok := cb.GetHeaderMap(ctx, 1, hdrType)
	if !ok {
		t.Fatal("GetHeaderMap(6) ok=false with a published cache; the rest of this test would be vacuous")
	}
	const wantPairs = 4
	if len(pairs) != wantPairs {
		t.Errorf("GetHeaderMap(6) emitted %d pairs; want %d (value-EXPANDED: X-Multi contributes 2). %d would mean key counting",
			len(pairs), wantPairs, len(cached.Headers))
	}
	if len(pairs) == len(cached.Headers) {
		t.Errorf("pair count (%d) equals KEY count — the fixture must keep them distinct for this assertion to discriminate", len(pairs))
	}
	var sawStatus bool
	for _, p := range pairs {
		if p.Key == ":status" {
			sawStatus = true
			if p.Value != "207" {
				t.Errorf("GetHeaderMap(6) %q = %q; want %q", ":status", p.Value, "207")
			}
		}
	}
	if !sawStatus {
		t.Errorf("GetHeaderMap(6) emitted no %q pair; keys = %v", ":status", pairKeys(pairs))
	}
	if v, ok := cb.GetHeaderMapValue(ctx, 1, hdrType, ":status"); !ok || v != "207" {
		t.Errorf("GetHeaderMapValue(6, %q) = (%q, %v); want (%q, true)", ":status", v, ok, "207")
	}
	// GetHeaderMapSize must agree with the pair count GetHeaderMap emits —
	// that agreement is exactly what a guest sizing a buffer relies on.
	if n := cb.GetHeaderMapSize(ctx, 1, hdrType); n != wantPairs {
		t.Errorf("GetHeaderMapSize(6) = %d; want %d (it must equal the emitted pair count)", n, wantPairs)
	}

	// Trailers at type 7 resolve to Trailers, NOT Headers — a swapped 6/7
	// wiring would surface here.
	trlPairs, ok := cb.GetHeaderMap(ctx, 1, trlType)
	if !ok {
		t.Error("GetHeaderMap(7) ok=false with a published cache")
	} else {
		if len(trlPairs) != 1 || trlPairs[0].Key != "X-Trailer" || trlPairs[0].Value != "tv" {
			t.Errorf("GetHeaderMap(7) = %v; want exactly one X-Trailer=tv pair (types 6 and 7 must not be crossed)", trlPairs)
		}
		for _, p := range trlPairs {
			if p.Key == ":status" {
				t.Error("GetHeaderMap(7) returned the HEADERS map — types 6 and 7 are crossed")
			}
		}
	}

	// Body at buffer type 4.
	buf, err := cb.GetBuffer(ctx, 1, bodyType)
	if err != nil {
		t.Errorf("GetBuffer(4) err = %v; want nil", err)
	}
	if string(buf) != `{"cached":true}` {
		t.Errorf("GetBuffer(4) = %q; want %q", string(buf), `{"cached":true}`)
	}

	// --- READ-ONLY: all four mutators, both map types. Reported per property
	// with Errorf so one regression does not hide the other seven. ---
	for _, mt := range []struct {
		name string
		typ  abi.WasmHeaderMapType
	}{{"HttpCallResponseHeaders(6)", hdrType}, {"HttpCallResponseTrailers(7)", trlType}} {
		if r := cb.AddHeaderMapValue(ctx, 1, mt.typ, "x-guest", "v"); r != abi.WasmResultUnimplemented {
			t.Errorf("AddHeaderMapValue(%s) = %v; want Unimplemented (the map is host-owned + read-only)", mt.name, r)
		}
		if r := cb.ReplaceHeaderMapValue(ctx, 1, mt.typ, ":status", "500"); r != abi.WasmResultUnimplemented {
			t.Errorf("ReplaceHeaderMapValue(%s) = %v; want Unimplemented", mt.name, r)
		}
		if r := cb.RemoveHeaderMapValue(ctx, 1, mt.typ, ":status"); r != abi.WasmResultUnimplemented {
			t.Errorf("RemoveHeaderMapValue(%s) = %v; want Unimplemented", mt.name, r)
		}
		if r := cb.SetHeaderMapPairs(ctx, 1, mt.typ, []internalwasm.HeaderPair{{Key: "x", Value: "y"}}); r != abi.WasmResultUnimplemented {
			t.Errorf("SetHeaderMapPairs(%s) = %v; want Unimplemented", mt.name, r)
		}
	}

	// The rejected mutations left the cache untouched.
	if v, ok := cb.GetHeaderMapValue(ctx, 1, hdrType, ":status"); !ok || v != "207" {
		t.Errorf("after the rejected mutators, %q = (%q, %v); want (%q, true) — a mutator leaked a write", ":status", v, ok, "207")
	}
	if _, ok := cb.GetHeaderMapValue(ctx, 1, hdrType, "x-guest"); ok {
		t.Error("a rejected AddHeaderMapValue nonetheless inserted x-guest into the host-owned map")
	}

	// --- cache CLEARED (what handleHttpCallResponse's deferred clear leaves
	// behind once the callback returns) ---
	cc.rootVM.SetHTTPCallResponse(nil)
	if pairs, ok := cb.GetHeaderMap(ctx, 1, hdrType); ok {
		t.Errorf("post-clear GetHeaderMap(6) ok=true pairs=%v; want ok=false (the cache is callback-scoped)", pairs)
	}
	if pairs, ok := cb.GetHeaderMap(ctx, 1, trlType); ok {
		t.Errorf("post-clear GetHeaderMap(7) ok=true pairs=%v; want ok=false", pairs)
	}
	if buf, err := cb.GetBuffer(ctx, 1, bodyType); err != nil || buf != nil {
		t.Errorf("post-clear GetBuffer(4) = (%q, %v); want (nil, nil)", string(buf), err)
	}
	// Read-only survives the clear — a nil cache must not become writable.
	if r := cb.AddHeaderMapValue(ctx, 1, hdrType, "x-guest", "v"); r != abi.WasmResultUnimplemented {
		t.Errorf("post-clear AddHeaderMapValue(6) = %v; want Unimplemented", r)
	}

	f.OnDestroy()
}

// pairKeys projects the keys of a pair slice for failure messages.
func pairKeys(pairs []internalwasm.HeaderPair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.Key)
	}
	return out
}
