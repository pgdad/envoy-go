package wasm

// property.go — 25.2 IMPL Task 17 per-stream property resolver glue per
// AMEND-B4 + 25.2 SPEC §3.6 + §5.3 + 25.2 PLAN Task 17.
//
// # Overview
//
// The bulk of the per-stream property resolver — the 60-method
// `filterPropertyResolver` implementing `internalwasm.PropertyResolver`
// (Task 13 internal/wasm/property.go) + the `(*abiCallbacks).GetProperty`
// hostcall entry that re-marshals framework-parsed segments into the
// canonical NUL-delimited form + dispatches through
// `internalwasm.ResolveProperty` — lives in `abi_callbacks.go` from Task 15.
//
// This file is the SLIM per-stream property resolver orchestration layer
// added at Task 17. It exists for two reasons:
//
//   1. **Filter-package-private `getProperty` wrapper** for use by tests +
//      future per-stream callers that need a single `*filter` -> `[]byte`
//      property-resolve seam without going through the public
//      `(*abiCallbacks).GetProperty` hostcall surface. The wrapper takes
//      the framework-parsed segment slice (per the proxy-wasm spec README
//      §Serialization NUL-delimited form) AND the canonical NUL-delimited
//      byte path; both shapes are useful test inputs. Production hostcall
//      dispatch continues to flow through `(*abiCallbacks).GetProperty`.
//
//   2. **Task-15-deferred query-string resolution** for `request.query` +
//      `request.url_path` per AMEND-B4. Task 15's `filterPropertyResolver`
//      methods `RequestQuery` + `RequestURLPath` were explicit STUBS with
//      "Task 17 property.go lands the query-string extraction" + "A future
//      refinement (Task 17 property.go extensions) may strip the query
//      bytes" comments. Task 17 lands the helpers in this file + the
//      `filterPropertyResolver` methods upgrade their bodies to call into
//      `splitPathQuery` (defined here) — single source of truth for the
//      :path -> (url_path, query) split.
//
// # Scope
//
// Per the PLAN Task 17 outline — the file is INTENTIONALLY thin (~150-250
// LoC) since:
//   - Task 13 (internal/wasm/property.go) holds the ~70-path dispatcher.
//   - Task 15 (abi_callbacks.go) holds the 60-method `filterPropertyResolver`
//     impl + the `(*abiCallbacks).GetProperty` hostcall entry +
//     `joinNULDelimited` segment-to-wire reconstruction.
//   - Task 17 (this file) holds only the orchestration helper +
//     query-string split logic.
//
// # Cross-references
//
//   - 25.2 SPEC §3.6 (property tree shape).
//   - 25.2 SPEC §5.3 row "property" (full ~70-path enumeration per AMEND-B4).
//   - AMEND-B4 (full proxy-wasm property-path tree mapping).
//   - Task 13 — internal/wasm/property.go ResolveProperty + 60-method
//     PropertyResolver interface.
//   - Task 15 — abi_callbacks.go filterPropertyResolver impl + GetProperty.
//   - ADR-0085 nil-tolerance (every entry point here is nil-receiver tolerant
//     + nil-filter tolerant; absent property -> false return -> NotFound at
//     the framework dispatcher).
//   - ADR-0144 / ADR-0177 / ADR-0190 / ADR-0207 (the 4 RE-USE primitives
//     consumed by filterPropertyResolver — UNCHANGED at this Task).

import (
	"strings"

	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// getProperty — filter-package-private per-stream property-resolver wrapper.
// -----------------------------------------------------------------------------

// getProperty resolves a property path for the per-stream *filter through
// the canonical Task 13 dispatcher. Construction of the per-stream
// PropertyResolver is funneled here so every call site uses the SAME
// resolver shape (the abi_callbacks.go GetProperty hostcall flows through
// the SAME helper via *abiCallbacks; this file's helper is for direct
// per-stream tests + future per-stream consumers that need the resolver
// without an abiCallbacks instance).
//
// `pathBytes` is the canonical NUL-delimited wire path per proxy-wasm spec
// README §Serialization (e.g. `"request\x00path"` for request.path,
// `"connection\x00mtls"` for connection.mtls, etc.). Returns (value bytes,
// status):
//
//   - status == abi.WasmResultOk    → value is the property's serialized bytes.
//   - status == abi.WasmResultNotFound → value is nil; property absent.
//
// Per ADR-0085 nil-tolerance: a nil receiver returns (nil, NotFound) for
// every path (the resolver dispatch sees a resolver whose every accessor
// returns the absent-zero pair).
func (f *filter) getProperty(pathBytes []byte) ([]byte, abi.WasmResult) {
	resolver := &filterPropertyResolver{filter: f}
	return internalwasm.ResolveProperty(resolver, pathBytes)
}

// getPropertySegments is the segment-slice convenience form of getProperty.
// Mirrors the abi_callbacks.GetProperty entry shape: takes []string (the
// framework-parsed segments per the hostcall dispatch layer) + reconstructs
// the NUL-delimited wire path via joinNULDelimited before delegating to
// the canonical dispatcher.
//
// Useful for tests that exercise specific path-segment combinations
// without manually constructing the NUL-delimited byte form.
//
// Empty segments slice returns (nil, NotFound) — mirrors the empty-path
// short-circuit at internalwasm.ResolveProperty.
func (f *filter) getPropertySegments(segments []string) ([]byte, abi.WasmResult) {
	if len(segments) == 0 {
		return nil, abi.WasmResultNotFound
	}
	return f.getProperty(joinNULDelimited(segments))
}

// -----------------------------------------------------------------------------
// :path query-string split — single source of truth for request.url_path +
// request.query resolution per AMEND-B4.
// -----------------------------------------------------------------------------

// splitPathQuery splits an HTTP/2 `:path` pseudo-header value into the
// (url_path, query) pair per upstream Envoy's request.url_path / request.query
// property-tree semantics (cpp-host context.cc:1019-1033 +
// `Http::Utility::stripQueryString`). The split is on the FIRST '?'
// character:
//
//   - `:path` = "/foo"            → url_path="/foo",     query=""
//   - `:path` = "/foo?"           → url_path="/foo",     query=""
//   - `:path` = "/foo?a=1"        → url_path="/foo",     query="a=1"
//   - `:path` = "/foo?a=1&b=2"    → url_path="/foo",     query="a=1&b=2"
//   - `:path` = "/foo?a=1?b=2"    → url_path="/foo",     query="a=1?b=2"
//     (only the FIRST '?' splits; subsequent '?' are part of query — matches
//     Go's net/url.URL{Path, RawQuery} behavior + upstream Envoy.)
//   - `:path` = ""                → url_path="",         query=""
//   - `:path` = "?"               → url_path="",         query=""
//   - `:path` = "?a=1"            → url_path="",         query="a=1"
//
// Per AMEND-B4: the upstream Envoy semantic for request.url_path strips the
// query string; request.query returns the query (sans the leading '?').
// envoy-go byte-faithful to this semantic at Task 17.
//
// Pure function (no filter receiver) — tested directly + RE-CONSUMED by the
// `filterPropertyResolver` methods at abi_callbacks.go via the
// resolveURLPath + resolveQuery one-liner helpers below.
func splitPathQuery(rawPath string) (urlPath, query string) {
	if rawPath == "" {
		return "", ""
	}
	if i := strings.IndexByte(rawPath, '?'); i >= 0 {
		return rawPath[:i], rawPath[i+1:]
	}
	return rawPath, ""
}

// -----------------------------------------------------------------------------
// resolveURLPath + resolveQuery — per-stream :path-derived property helpers
// consumed by the filterPropertyResolver methods RequestURLPath + RequestQuery
// at abi_callbacks.go (replacing the Task 15 STUBS per the inline Task-17
// upgrade-pointer comments).
// -----------------------------------------------------------------------------

// resolveURLPath returns the :path with the query string stripped per
// AMEND-B4 (upstream Envoy request.url_path semantic). Returns (value, true)
// when :path is present + non-empty; ("", false) when :path absent (the
// framework dispatcher converts false -> NotFound).
//
// Per ADR-0085 nil-tolerance: nil receiver / nil filter / nil requestHeaders
// all gracefully return ("", false).
//
// CONSUMED by filterPropertyResolver.RequestURLPath at abi_callbacks.go via
// the Task 17 upgrade. Standalone testable here.
func (r *filterPropertyResolver) resolveURLPath() (string, bool) {
	raw, ok := r.pseudoHeader(":path")
	if !ok {
		return "", false
	}
	urlPath, _ := splitPathQuery(raw)
	if urlPath == "" {
		// `:path` was "?..." — guest sees ok=false for url_path since the
		// path component is empty (mirrors upstream NotFound on absent
		// path component).
		return "", false
	}
	return urlPath, true
}

// resolveQuery returns the query-string portion of :path (the substring
// AFTER the first '?'), per AMEND-B4 (upstream Envoy request.query
// semantic). Returns (value, true) when the query component is present +
// non-empty; ("", false) otherwise (the framework dispatcher converts false
// -> NotFound).
//
// Per ADR-0085 nil-tolerance: nil receiver / nil filter / nil requestHeaders
// all gracefully return ("", false). A `:path` of "/foo" (no '?') returns
// ("", false) — query absent.
//
// CONSUMED by filterPropertyResolver.RequestQuery at abi_callbacks.go via
// the Task 17 upgrade. Standalone testable here.
func (r *filterPropertyResolver) resolveQuery() (string, bool) {
	raw, ok := r.pseudoHeader(":path")
	if !ok {
		return "", false
	}
	_, query := splitPathQuery(raw)
	if query == "" {
		return "", false
	}
	return query, true
}
