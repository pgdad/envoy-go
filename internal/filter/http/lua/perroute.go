package lua

// perroute.go — real 3-arm LuaPerRoute validator per phase 22.3 Task 2 +
// ADR-0110 single-chokepoint + parent SPEC §6.2 arm 18. Replaces the
// deferred one-liner in lua.go with a fully-featured per-route config
// validator covering the `disabled` / `name` / `source_code` oneof arms.
//
// # Validator contract (ADR-0110 single-chokepoint)
//
// validatePerRouteLua (in lua.go) delegates to parsePerRouteLua here.
// parsePerRouteLua performs the full validation + returns the typed
// *luav3.LuaPerRoute (or nil + error). RegisterPerRouteValidator wiring
// in lua.go stays UNCHANGED — the validator signature is still
// `func(proto.Message) error`.
//
// # 3-arm oneof dispatch
//
// The LuaPerRoute.override oneof carries exactly 3 concrete types:
//
//   - *LuaPerRoute_Disabled  — bool must be true (PGV const:true)
//   - *LuaPerRoute_Name      — string must be non-empty (≥1 rune)
//   - *LuaPerRoute_SourceCode — DataSource runs through the SAME
//     resolveDataSource gauntlet used by default_source_code + source_codes;
//     the resolved bytes are compile-to-validated via CompileScript(src, nil)
//     (nil cache = no memoisation; discard the chunk; validation only).
//
// # Byte-stable wording discipline (ADR-0080 + parent SPEC §6.2)
//
// All 4 consts below follow the `"lua: per-route: <reason>"` prefix per
// parent §6.1. Byte-exact wording is pinned at perroute_test.go::
// TestParsePerRouteConsts_ByteExactWording. Any wording change requires
// a lockstep edit of this file + the test + parent SPEC §6.2 per
// ADR-0044 atomic-edit discipline.

import (
	"errors"
	"fmt"

	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/proto"

	internallua "github.com/pgdad/envoy-go/internal/lua"
)

// -----------------------------------------------------------------------------
// Byte-stable wording constants per ADR-0080 + parent SPEC §6.2 arm 18.
// Pinned at TestParsePerRouteConsts_ByteExactWording.
// -----------------------------------------------------------------------------

const (
	// parseRejectPerRouteOneofRequired fires when LuaPerRoute.Override is nil
	// (an empty &LuaPerRoute{} proto with no oneof arm set).
	parseRejectPerRouteOneofRequired = "lua: per-route: override oneof is required"

	// parseRejectPerRouteDisabledFalse fires when the `disabled` arm is set
	// but carries the value false — PGV const:true violation.
	parseRejectPerRouteDisabledFalse = "lua: per-route: disabled must be true (PGV const:true violation)"

	// parseRejectPerRouteNameEmpty fires when the `name` arm is set but carries
	// the empty string — length must be ≥1 rune per PGV min_len:1.
	parseRejectPerRouteNameEmpty = "lua: per-route: name length must be at least 1 rune"

	// wrapParseRejectPerRouteSourceCodeFmt is the fmt.Errorf format used to wrap
	// resolveDataSource + CompileScript errors from the `source_code` arm.
	// The %w verb carries the inner error for errors.Is/As unwrapping.
	wrapParseRejectPerRouteSourceCodeFmt = "lua: per-route: source_code: %w"
)

// -----------------------------------------------------------------------------
// parsePerRouteLua — 4-group validator.
// -----------------------------------------------------------------------------

// parsePerRouteLua validates a per-route LuaPerRoute proto message per
// phase 22.3 Task 2 PLAN + ADR-0110 single-chokepoint. Returns the typed
// *luav3.LuaPerRoute on success; returns (nil, error) on any validation
// failure.
//
// Validation groups (in order):
//  1. Type-assert to *luav3.LuaPerRoute (wrong proto.Message type → error).
//  2. Override oneof nil → byte-exact parseRejectPerRouteOneofRequired.
//  3. Oneof dispatch:
//     - *LuaPerRoute_Disabled: false → byte-exact parseRejectPerRouteDisabledFalse
//     - *LuaPerRoute_Name: "" → byte-exact parseRejectPerRouteNameEmpty
//     - *LuaPerRoute_SourceCode: resolveDataSource + CompileScript(src, nil);
//     wrap any error with wrapParseRejectPerRouteSourceCodeFmt.
//
// This function is the sole implementation behind validatePerRouteLua in
// lua.go; the exported RegisterPerRouteValidator wiring is UNCHANGED.
func parsePerRouteLua(m proto.Message) (*luav3.LuaPerRoute, error) {
	// Group 1: type-assert. The ADR-0110 validator signature accepts any
	// proto.Message; a misconfigured registry or test passing the wrong type
	// gets a clean, actionable type-assert error rather than a nil-pointer
	// panic.
	pr, ok := m.(*luav3.LuaPerRoute)
	if !ok {
		return nil, fmt.Errorf("lua: per-route: expected *luav3.LuaPerRoute, got %T", m)
	}

	// Group 2: oneof nil check. An empty &LuaPerRoute{} with no override
	// arm is misconfiguration — the operator must explicitly choose one of
	// disabled / name / source_code.
	if pr.GetOverride() == nil {
		return nil, errors.New(parseRejectPerRouteOneofRequired)
	}

	// Group 3: oneof dispatch.
	switch pr.GetOverride().(type) {
	case *luav3.LuaPerRoute_Disabled:
		// PGV const:true: the only meaningful value for `disabled` is true.
		// A `disabled: false` config is indistinguishable from "not set" and
		// is almost certainly a misconfiguration; reject it with the PGV
		// annotation wording.
		if !pr.GetDisabled() {
			return nil, errors.New(parseRejectPerRouteDisabledFalse)
		}

	case *luav3.LuaPerRoute_Name:
		// PGV min_len:1: the name must reference an actual source_codes entry;
		// an empty name is a misconfiguration.
		if pr.GetName() == "" {
			return nil, errors.New(parseRejectPerRouteNameEmpty)
		}

	case *luav3.LuaPerRoute_SourceCode:
		// Run through the SAME resolveDataSource gauntlet used by
		// buildCompiledConfig (default_source_code + source_codes paths).
		// Rejects the same 10-leaf failure cases (ENOENT, WatchedDirectory,
		// etc.). Then compile-to-validate via CompileScript(src, nil) — pass
		// nil cache for validation-only (no memoisation; discard the chunk).
		// This is HCM-build-time validation; per-stream binding/memo is Task 3.
		src, dsErr := resolveDataSource(pr.GetSourceCode())
		if dsErr != nil {
			return nil, fmt.Errorf(wrapParseRejectPerRouteSourceCodeFmt, dsErr)
		}
		if _, compErr := internallua.CompileScript(src, nil); compErr != nil {
			return nil, fmt.Errorf(wrapParseRejectPerRouteSourceCodeFmt, compErr)
		}

	default:
		// Defensive: unreachable today (the proto oneof carries exactly 3
		// concrete types), but guards any hypothetical future 4th arm from
		// silently passing validation per ADR-0018 never-panic posture.
		return nil, fmt.Errorf("lua: per-route: unknown override type %T", pr.GetOverride())
	}

	return pr, nil
}

// -----------------------------------------------------------------------------
// resolveDecodeScript — phase 22.3 Task 3 per-route 3-tier dispatch.
// -----------------------------------------------------------------------------

// resolveDecodeScript performs the per-stream per-route chunk selection ONCE at
// DecodeHeaders per the load-bearing dispatch invariant (PLAN Task 3). Returns
// (chunk, disabled):
//
//   - chunk — the *internallua.Chunk to vm.Run for this stream (defines BOTH
//     the envoy_on_request + envoy_on_response hooks). A nil chunk means
//     "no script for this stream → pass through" (decode early-returns Continue
//     → no VM → encode's f.vm == nil guard naturally skips both hooks).
//   - disabled — true when the route explicitly disables the filter
//     (per-route `disabled: true`). Decode treats disabled identically to a nil
//     chunk for VM-construction purposes; the bool is returned distinctly so a
//     caller could observe the disable reason.
//
// # 3-tier resolution (most-specific wins via the chain's Resolve merge)
//
//  1. No per-route config (nil) OR nil decoder callbacks → listener default
//     `(f.cc.chunk, false)`. The chain's RequestRouteConfig already performs
//     the Route > VirtualHost > RouteConfiguration merge per ADR-0073, so a nil
//     return here means no tier carries a lua override for this route.
//  2. Per-route present → type-assert to *LuaPerRoute and switch the 3-arm
//     oneof DIRECTLY. The proto is already fully validated at HCM-build time via
//     the RegisterPerRouteValidator chokepoint (parsePerRouteLua); the per-stream
//     hot path MUST NOT re-validate. Re-running parsePerRouteLua here would
//     re-run resolveDataSource + CompileScript purely to discard the chunk,
//     re-reading a Filename DataSource from disk on EVERY stream BEFORE the memo
//     is ever consulted — defeating the D-P1(b') "read+compile ONCE per route"
//     guarantee. So we switch the oneof directly and let the memo own the single
//     source_code read:
//     - `disabled: true` → `(nil, true)`.
//     - `name` → `(f.cc.sourceCodes[name], false)`. A registry MISS returns
//     `(nil, false)` — the AMEND-22.3-1 dangling-name SILENT NO-OP (NOT an
//     error; upstream-parity pass-through).
//     - `source_code` → the memoized override chunk: under perRouteMu, look up
//     cc.perRouteChunks[pr]; on miss resolveDataSource + CompileScript into
//     the SHARED compileCache (content-hash dedups byte-identical scripts
//     across routes), store keyed by the *LuaPerRoute pointer.
//
// Nil-tolerant on f.dcb (no decoder callbacks → behave as no per-route).
// A type-assert failure on the per-route proto (should not happen
// post-HCM-validate) degrades to the listener default rather than panicking
// (ADR-0018 never-panic + the buffer filter's resolveEffective fall-back
// precedent).
func (f *filter) resolveDecodeScript() (*internallua.Chunk, bool) {
	// Tier 2/3: no decoder callbacks → no per-route → listener default.
	if f.dcb == nil {
		return f.cc.chunk, false
	}

	resolved := f.dcb.RequestRouteConfig() // proto.Message or nil
	if resolved == nil {
		// No tier carries a lua override for this route → listener default.
		return f.cc.chunk, false
	}

	// Per-route present: type-assert to switch the 3-arm oneof directly. The
	// proto was already validated at HCM-build time (RegisterPerRouteValidator
	// → parsePerRouteLua); we DO NOT re-validate on the hot path (that would
	// re-read a Filename DataSource per stream — the D-P1(b') re-read bug). A
	// type-assert failure degrades to the listener default (defensive — mirrors
	// buffer.resolveEffective's unparseable fall-back; never panics per
	// ADR-0018).
	pr, ok := resolved.(*luav3.LuaPerRoute)
	if !ok {
		return f.cc.chunk, false
	}

	switch ov := pr.GetOverride().(type) {
	case *luav3.LuaPerRoute_Disabled:
		if ov.Disabled {
			// disabled: true → no script for this stream.
			return nil, true
		}
		// Defensive: disabled:false is rejected at HCM-build time
		// (parseRejectPerRouteDisabledFalse); treat as listener default here.
		return f.cc.chunk, false

	case *luav3.LuaPerRoute_Name:
		// name arm → registry lookup. A MISS returns (nil, false) — the
		// AMEND-22.3-1 dangling-name SILENT NO-OP (upstream-parity pass-through,
		// NOT an error). cc.sourceCodes may be nil (no source_codes configured);
		// a nil-map read returns the zero value safely.
		return f.cc.sourceCodes[ov.Name], false

	case *luav3.LuaPerRoute_SourceCode:
		// source_code arm → the memoized override chunk, keyed by the stable
		// *LuaPerRoute pointer (the chain's Resolve returns a STABLE proto
		// pointer per routeIdx). The memo owns the single read+compile;
		// compiled into the SHARED compileCache so byte-identical scripts
		// across routes dedup via content-hash.
		return f.resolvePerRouteSourceCode(pr), false

	default:
		// Defensive: nil/unknown oneof → listener default. Unreachable today
		// (HCM-build validation rejects nil-oneof + the proto carries exactly 3
		// concrete types) per ADR-0018 never-panic posture.
		return f.cc.chunk, false
	}
}

// resolvePerRouteSourceCode returns the memoized compiled chunk for a
// source_code per-route override, keyed by the *LuaPerRoute pointer. The memo
// (cc.perRouteChunks guarded by cc.perRouteMu) is lazily allocated on first use.
// On a memo miss the DataSource is resolved + compiled into the SHARED
// compileCache. A resolve/compile failure (should not happen post-HCM-validate)
// returns nil → silent no-op pass-through (ADR-0018 never-panic).
func (f *filter) resolvePerRouteSourceCode(pr *luav3.LuaPerRoute) *internallua.Chunk {
	cc := f.cc
	cc.perRouteMu.Lock()
	defer cc.perRouteMu.Unlock()

	if cc.perRouteChunks != nil {
		if ch, ok := cc.perRouteChunks[pr]; ok {
			return ch
		}
	}

	src, dsErr := resolveDataSource(pr.GetSourceCode())
	if dsErr != nil {
		return nil
	}
	ch, compErr := internallua.CompileScript(src, cc.compileCache)
	if compErr != nil {
		return nil
	}

	if cc.perRouteChunks == nil {
		cc.perRouteChunks = make(map[*luav3.LuaPerRoute]*internallua.Chunk)
	}
	cc.perRouteChunks[pr] = ch
	return ch
}
