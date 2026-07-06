package wasm

// property_test.go — 25.2 IMPL Task 17 unit tests for the per-stream property
// resolver glue + the splitPathQuery / resolveURLPath / resolveQuery helpers
// landed at property.go.
//
// Scope per PLAN Task 17:
//
//   1. splitPathQuery — pure string split on first '?' across the 8
//      enumerated edge cases per AMEND-B4 (no '?' / single '?' / multi '?'
//      / leading '?' / empty / etc).
//   2. resolveURLPath + resolveQuery — :path-derived per-stream helpers
//      under varied requestHeaders states (nil filter / nil requestHeaders /
//      missing :path / empty :path / typical :path / query-bearing :path).
//   3. getProperty + getPropertySegments — filter-package-private wrappers
//      that funnel through internalwasm.ResolveProperty. Verifies the
//      round-trip across a representative spread of property roots:
//      direct token (plugin_name) + dispatched root (request.path /
//      request.query / request.url_path) + absent path -> NotFound.
//   4. End-to-end integration via abi_callbacks.GetProperty — verifies the
//      Task 17 RequestQuery + RequestURLPath upgrades flow correctly through
//      the canonical NUL-delimited dispatcher.
//
// The ~70 sub-path roster coverage on the framework-side dispatcher is at
// internal/wasm/property_test.go (Task 13); this file covers the per-filter
// integration layer only. Counter-tests for the 8 NEW envoy-go-strict
// counters are in wasm_test.go (extends existing TestStatNames_Equal_* +
// TestNewFilterStats_* table).

import (
	"context"
	"net/http"
	"testing"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// splitPathQuery — pure-function table-driven coverage per AMEND-B4.
// -----------------------------------------------------------------------------

// TestSplitPathQuery enumerates the 8 documented edge cases per AMEND-B4 +
// property.go splitPathQuery doc-comment. Pure function — no receiver.
func TestSplitPathQuery(t *testing.T) {
	cases := []struct {
		name        string
		rawPath     string
		wantURLPath string
		wantQuery   string
	}{
		{"empty", "", "", ""},
		{"only-path-no-query", "/foo", "/foo", ""},
		{"path-with-empty-query", "/foo?", "/foo", ""},
		{"path-with-simple-query", "/foo?a=1", "/foo", "a=1"},
		{"path-with-multi-pair-query", "/foo?a=1&b=2", "/foo", "a=1&b=2"},
		{"path-with-multi-q-only-first-splits", "/foo?a=1?b=2", "/foo", "a=1?b=2"},
		{"leading-q-only", "?", "", ""},
		{"leading-q-with-query", "?a=1", "", "a=1"},
		{"root-path-no-query", "/", "/", ""},
		{"complex-path-and-query", "/api/v1/users?id=42&filter=active", "/api/v1/users", "id=42&filter=active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotQ := splitPathQuery(tc.rawPath)
			if gotURL != tc.wantURLPath {
				t.Errorf("splitPathQuery(%q) urlPath = %q; want %q", tc.rawPath, gotURL, tc.wantURLPath)
			}
			if gotQ != tc.wantQuery {
				t.Errorf("splitPathQuery(%q) query = %q; want %q", tc.rawPath, gotQ, tc.wantQuery)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// resolveURLPath + resolveQuery — :path-derived per-stream helpers under
// nil-tolerance + varied requestHeaders states.
// -----------------------------------------------------------------------------

// newPropertyResolverFromFilter is a small test helper that constructs a
// filterPropertyResolver bound to the given *filter (mirrors the production
// abi_callbacks.go newPropertyResolver but takes the *filter directly).
func newPropertyResolverFromFilter(f *filter) *filterPropertyResolver {
	return &filterPropertyResolver{filter: f}
}

// TestResolveURLPath_NilTolerance verifies the nil-receiver / nil-filter /
// nil-requestHeaders defensive paths per ADR-0085.
func TestResolveURLPath_NilTolerance(t *testing.T) {
	// Nil filter.
	r := &filterPropertyResolver{filter: nil}
	if v, ok := r.resolveURLPath(); ok || v != "" {
		t.Errorf("resolveURLPath nil-filter = (%q, %v); want (\"\", false)", v, ok)
	}
	// Nil requestHeaders.
	r = newPropertyResolverFromFilter(&filter{})
	if v, ok := r.resolveURLPath(); ok || v != "" {
		t.Errorf("resolveURLPath nil-requestHeaders = (%q, %v); want (\"\", false)", v, ok)
	}
	// Empty requestHeaders.
	r = newPropertyResolverFromFilter(&filter{requestHeaders: http.Header{}})
	if v, ok := r.resolveURLPath(); ok || v != "" {
		t.Errorf("resolveURLPath empty-headers = (%q, %v); want (\"\", false)", v, ok)
	}
}

// TestResolveURLPath_ValueTable enumerates representative :path inputs +
// the expected (value, ok) split per the upstream Envoy request.url_path
// semantic.
func TestResolveURLPath_ValueTable(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantVal string
		wantOk  bool
	}{
		{"plain-path", "/users", "/users", true},
		{"path-with-query", "/users?id=42", "/users", true},
		{"path-with-empty-query", "/users?", "/users", true},
		{"empty-path-only-q-and-query", "?id=42", "", false},
		{"empty-path-only-q", "?", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &filter{requestHeaders: http.Header{":path": []string{tc.path}}}
			r := newPropertyResolverFromFilter(f)
			gotVal, gotOk := r.resolveURLPath()
			if gotVal != tc.wantVal || gotOk != tc.wantOk {
				t.Errorf("resolveURLPath(%q) = (%q, %v); want (%q, %v)",
					tc.path, gotVal, gotOk, tc.wantVal, tc.wantOk)
			}
		})
	}
}

// TestResolveQuery_NilTolerance verifies the nil-receiver / nil-filter /
// nil-requestHeaders defensive paths per ADR-0085.
func TestResolveQuery_NilTolerance(t *testing.T) {
	r := &filterPropertyResolver{filter: nil}
	if v, ok := r.resolveQuery(); ok || v != "" {
		t.Errorf("resolveQuery nil-filter = (%q, %v); want (\"\", false)", v, ok)
	}
	r = newPropertyResolverFromFilter(&filter{})
	if v, ok := r.resolveQuery(); ok || v != "" {
		t.Errorf("resolveQuery nil-requestHeaders = (%q, %v); want (\"\", false)", v, ok)
	}
	r = newPropertyResolverFromFilter(&filter{requestHeaders: http.Header{}})
	if v, ok := r.resolveQuery(); ok || v != "" {
		t.Errorf("resolveQuery empty-headers = (%q, %v); want (\"\", false)", v, ok)
	}
}

// TestResolveQuery_ValueTable enumerates representative :path inputs + the
// expected (value, ok) split per the upstream Envoy request.query semantic.
func TestResolveQuery_ValueTable(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantVal string
		wantOk  bool
	}{
		{"no-query", "/users", "", false},
		{"empty-query", "/users?", "", false},
		{"single-pair", "/users?id=42", "id=42", true},
		{"multi-pair", "/users?id=42&filter=active", "id=42&filter=active", true},
		{"leading-q-only", "?", "", false},
		{"leading-q-with-pair", "?id=42", "id=42", true},
		{"multi-q-in-query", "/foo?a=1?b=2", "a=1?b=2", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &filter{requestHeaders: http.Header{":path": []string{tc.path}}}
			r := newPropertyResolverFromFilter(f)
			gotVal, gotOk := r.resolveQuery()
			if gotVal != tc.wantVal || gotOk != tc.wantOk {
				t.Errorf("resolveQuery(%q) = (%q, %v); want (%q, %v)",
					tc.path, gotVal, gotOk, tc.wantVal, tc.wantOk)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// getProperty + getPropertySegments — filter-package-private wrappers.
// -----------------------------------------------------------------------------

// TestGetPropertySegments_EmptyReturnsNotFound mirrors the short-circuit at
// internalwasm.ResolveProperty's empty-segments arm.
func TestGetPropertySegments_EmptyReturnsNotFound(t *testing.T) {
	f := &filter{}
	val, status := f.getPropertySegments(nil)
	if val != nil || status != abi.WasmResultNotFound {
		t.Errorf("getPropertySegments(nil) = (%v, %v); want (nil, NotFound)", val, status)
	}
	val, status = f.getPropertySegments([]string{})
	if val != nil || status != abi.WasmResultNotFound {
		t.Errorf("getPropertySegments([]) = (%v, %v); want (nil, NotFound)", val, status)
	}
}

// TestGetProperty_RequestPath verifies the request.path dispatched root
// round-trips through Task 13's ResolveProperty + Task 15's
// filterPropertyResolver and returns the :path bytes verbatim.
func TestGetProperty_RequestPath(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/users?id=42"}},
	}
	val, status := f.getPropertySegments([]string{"request", "path"})
	if status != abi.WasmResultOk {
		t.Fatalf("getPropertySegments(request.path) status = %v; want Ok", status)
	}
	if got := string(val); got != "/users?id=42" {
		t.Errorf("request.path = %q; want %q", got, "/users?id=42")
	}
}

// TestGetProperty_RequestURLPath verifies the request.url_path dispatched
// root strips the query string per the Task 17 RequestURLPath upgrade.
func TestGetProperty_RequestURLPath(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/users?id=42"}},
	}
	val, status := f.getPropertySegments([]string{"request", "url_path"})
	if status != abi.WasmResultOk {
		t.Fatalf("getPropertySegments(request.url_path) status = %v; want Ok", status)
	}
	if got := string(val); got != "/users" {
		t.Errorf("request.url_path = %q; want %q (query stripped)", got, "/users")
	}
}

// TestGetProperty_RequestQuery verifies the request.query dispatched root
// returns the query-string substring per the Task 17 RequestQuery upgrade.
func TestGetProperty_RequestQuery(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/users?id=42&filter=active"}},
	}
	val, status := f.getPropertySegments([]string{"request", "query"})
	if status != abi.WasmResultOk {
		t.Fatalf("getPropertySegments(request.query) status = %v; want Ok", status)
	}
	if got := string(val); got != "id=42&filter=active" {
		t.Errorf("request.query = %q; want %q", got, "id=42&filter=active")
	}
}

// TestGetProperty_RequestQuery_AbsentReturnsNotFound verifies that a :path
// without a query string surfaces as NotFound (false from resolveQuery ->
// nil/NotFound at the framework dispatcher).
func TestGetProperty_RequestQuery_AbsentReturnsNotFound(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/no-query"}},
	}
	val, status := f.getPropertySegments([]string{"request", "query"})
	if status != abi.WasmResultNotFound {
		t.Errorf("getPropertySegments(request.query) status = %v; want NotFound", status)
	}
	if val != nil {
		t.Errorf("getPropertySegments(request.query) val = %q; want nil", val)
	}
}

// TestGetProperty_NilFilter exercises the nil-receiver tolerance (every
// accessor on the resolver returns the absent-zero pair; the framework
// dispatcher converts most to NotFound — direct-token paths may serialize
// the empty value with Ok per the spec's serialization rules; we only
// check that the call does NOT panic).
func TestGetProperty_NilFilter(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("getProperty on nil-aware *filter panicked: %v", r)
		}
	}()
	// Use a valid *filter with zero-value fields — the resolver tolerates
	// nil sub-pointers via guard checks per Task 15.
	f := &filter{}
	_, _ = f.getPropertySegments([]string{"request", "path"})
}

// TestGetProperty_DirectToken_PluginName verifies a direct-token form
// (single-segment, no dispatched root) round-trips per AMEND-B4 §Direct
// tokens. PluginName() at Task 15 returns the plugin_name from compiledConfig
// when available; absent (no cfg) returns ("", false) -> NotFound.
//
// At Task 17 (Task 18 not yet landed) we exercise the absent path — *filter
// without cfg returns NotFound on plugin_name.
func TestGetProperty_DirectToken_PluginName_Absent(t *testing.T) {
	f := &filter{} // no cfg
	val, status := f.getPropertySegments([]string{"plugin_name"})
	if status != abi.WasmResultNotFound {
		t.Errorf("getPropertySegments(plugin_name) [no cfg] status = %v; want NotFound", status)
	}
	if val != nil {
		t.Errorf("getPropertySegments(plugin_name) [no cfg] val = %q; want nil", val)
	}
}

// TestGetProperty_UnknownRoot verifies an unknown top-level segment returns
// NotFound at the framework dispatcher (Task 13 default arm).
func TestGetProperty_UnknownRoot(t *testing.T) {
	f := &filter{requestHeaders: http.Header{":path": []string{"/x"}}}
	val, status := f.getPropertySegments([]string{"nonexistent", "sub"})
	if status != abi.WasmResultNotFound {
		t.Errorf("getPropertySegments(nonexistent.sub) status = %v; want NotFound", status)
	}
	if val != nil {
		t.Errorf("getPropertySegments(nonexistent.sub) val = %q; want nil", val)
	}
}

// -----------------------------------------------------------------------------
// End-to-end integration — abi_callbacks.GetProperty + Task 17 upgrades.
// -----------------------------------------------------------------------------

// TestABICallbacksGetProperty_RequestQueryUpgrade verifies that the Task 15
// abi_callbacks.GetProperty hostcall entry flows through the Task 17
// RequestQuery upgrade end-to-end (regression-pin against the previous
// STUB returning ("", false) -> NotFound for any :path with a query).
func TestABICallbacksGetProperty_RequestQueryUpgrade(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/users?id=42"}},
	}
	ac := &abiCallbacks{filter: f}
	val, ok := ac.GetProperty(context.Background(), 0, []string{"request", "query"})
	if !ok {
		t.Fatalf("abiCallbacks.GetProperty(request.query) ok = false; want true (Task 17 upgrade)")
	}
	if got := string(val); got != "id=42" {
		t.Errorf("abiCallbacks.GetProperty(request.query) = %q; want %q", got, "id=42")
	}
}

// TestABICallbacksGetProperty_RequestURLPathUpgrade verifies that the
// Task 15 abi_callbacks.GetProperty hostcall entry flows through the Task 17
// RequestURLPath upgrade end-to-end (regression-pin against the previous
// STUB returning :path verbatim for request.url_path).
func TestABICallbacksGetProperty_RequestURLPathUpgrade(t *testing.T) {
	f := &filter{
		requestHeaders: http.Header{":path": []string{"/users?id=42"}},
	}
	ac := &abiCallbacks{filter: f}
	val, ok := ac.GetProperty(context.Background(), 0, []string{"request", "url_path"})
	if !ok {
		t.Fatalf("abiCallbacks.GetProperty(request.url_path) ok = false; want true")
	}
	if got := string(val); got != "/users" {
		t.Errorf("abiCallbacks.GetProperty(request.url_path) = %q; want %q (query stripped)", got, "/users")
	}
}
