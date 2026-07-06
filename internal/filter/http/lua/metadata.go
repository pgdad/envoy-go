package lua

// metadata.go — Task 9 (phase 22.2 IMPL) metadata + dynamic-metadata
// bridge per SPEC §3.4 + §11.6 D1 closure + PLAN Task 9 + ADR-0192
// §Decision body anticipation.
//
// # Surface
//
// Two distinct bridge surfaces:
//
//   - :metadata() — ALWAYS-CALLABLE EMPTY USERDATA per §11.6 D1 closure
//     (NEVER nil; matches upstream `MetadataMapWrapper` always-non-nil
//     pattern at v1.32.4 binding-gap). The wrapper's `:get(k)` returns
//     `lua.LNil` for any key; `pairs()` yields zero iterations. Source-
//     of-data flips on at v1.32.4 → v1.37.x binding bump per parent
//     AMEND-12 (future phase) — this Task 9 surface is the callable-empty
//     stub.
//
//   - :streamInfo():dynamicMetadata() + :dynamicTypedMetadata(filterName)
//     — consume the per-stream `*dynamicmetadata.Bucket` from Task 1
//     (`internal/dynamicmetadata/`) via the `RequestHandleCallbacks`
//     adapter's `DynamicMetadata()` accessor (Task 9 extension).
//
//     `:dynamicMetadata()` returns a userdata wrapping the *Bucket;
//     `:get(filterName, key)` returns the proto-Value-marshaled Lua value
//     (via `structpbToLua`) or `lua.LNil`; `:set(filterName, key, value)`
//     marshals the Lua value back to `*structpb.Value` (via
//     `luaToStructpb`) and stores in the bucket.
//
//     `:dynamicTypedMetadata(filterName)` returns the entire filterName-
//     keyed sub-map as a Lua table (typed-iteration access per SPEC
//     §3.4 + §11.6). All values are marshaled via `structpbToLua`. The
//     returned table is independent of the bucket — mutations to the
//     table do NOT propagate back. Returns `lua.LNil` when filterName is
//     absent from the bucket OR when the bucket itself is nil per
//     ADR-0085.
//
// # Marshaling helpers — structpb.Value ↔ Lua-value
//
//   - structpbToLua(L, v) — converts a *structpb.Value to its Lua-value
//     equivalent. Null/nil → lua.LNil; Bool → lua.LBool; Number →
//     lua.LNumber; String → lua.LString; List → lua.LTable (1-indexed
//     per Lua-convention); Struct → lua.LTable (hash-keyed).
//
//   - luaToStructpb(L, lv) — inverse direction. lua.LNil → NullValue;
//     lua.LBool → BoolValue; lua.LNumber → NumberValue; lua.LString →
//     StringValue; lua.LTable → ListValue (when contiguous 1-N integer
//     keys) OR StructValue (otherwise; string keys). Unsupported types
//     (LFunction / LUserData / LChannel / LState) → NullValue (defensive
//     — matches upstream Lua filter wrappers.cc luaToProtoValue's
//     return-empty-on-unsupported pattern).
//
// # Nil-bucket tolerance per ADR-0085
//
// Per ADR-0085 nil-receiver-tolerant Bucket discipline + the bridge-side
// short-circuit at `dynamicMetadataFromUD`, the surface accommodates:
//
//   - cb returns nil from DynamicMetadata() (synthetic test path).
//   - Post-Destroy access (cb.DynamicMetadata returns nil after Reset).
//
// Behavior on nil-bucket:
//
//   - :streamInfo():dynamicMetadata() returns a userdata wrapping a nil
//     *Bucket (NOT lua.LNil — the bridge surface stays callable).
//   - :get on a nil-bucket-wrapper returns lua.LNil (bucket.Get is
//     nil-receiver-tolerant; returns ok=false).
//   - :set on a nil-bucket-wrapper is a silent no-op (bucket.Set is
//     nil-receiver-tolerant).
//   - :streamInfo():dynamicTypedMetadata(filterName) on nil-bucket
//     returns lua.LNil.
//
// # ADR-0192 cross-references
//
// ADR-0192 §Decision body anticipation: this file lands the metadata +
// dynamic-metadata bridge per SPEC §3.4. The :metadata() empty-userdata
// shape pins §11.6 D1 closure verbatim — NEVER returns lua.LNil. The
// `:streamInfo():dynamicMetadata()` consumer adapter to the Task 1
// `*Bucket` primitive lands here as the first-co-consumer per ADR-0190
// §Consequences cross-phase deferral-lift expectation.

import (
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
)

// ---------------------------------------------------------------------
// :metadata() — ALWAYS-CALLABLE EMPTY USERDATA per §11.6 D1 closure
// ---------------------------------------------------------------------

// metadataMethods is the method-name → LGFunction dispatch table for the
// metadata userdata's __index metafield. Single method (:get) per §11.6
// D1 closure — the wrapper is intentionally minimal (no :set; no list
// iteration outside __pairs) until the source-of-data flips on at the
// v1.32.4 → v1.37.x binding bump (future phase per parent AMEND-12).
var metadataMethods = map[string]lua.LGFunction{
	"get": metadataGet,
}

// requestHandleMetadata implements request_handle:metadata() per SPEC
// §3.4 + §11.6 D1 closure. ALWAYS returns a userdata wrapping the empty
// metadata source — NEVER lua.LNil. Matches upstream
// `MetadataMapWrapper` always-non-nil pattern at v1.32.4 binding-gap.
//
// Argument shape:
//
//	rh:metadata() -> userdata
//
// The returned userdata's metatable is envoy_metadata. The wrapper's
// `:get(k)` returns lua.LNil for any key; `pairs(wrapper)` yields zero
// iterations.
func requestHandleMetadata(L *lua.LState) int {
	ud := L.CheckUserData(1)
	if _, ok := ud.Value.(*requestHandleContext); !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	return pushMetadataUD(L)
}

// responseHandleMetadata implements response_handle:metadata() symmetric
// to the request-side per upstream parity. Same ALWAYS-CALLABLE EMPTY
// USERDATA contract.
func responseHandleMetadata(L *lua.LState) int {
	ud := L.CheckUserData(1)
	if _, ok := ud.Value.(*responseHandleContext); !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	return pushMetadataUD(L)
}

// pushMetadataUD allocates a fresh LUserData with nil Value (the wrapper
// holds no source — empty-source per §11.6 D1 closure) + attaches the
// envoy_metadata metatable + pushes it onto the stack. Returns 1.
//
// The Value is intentionally nil — the LGFunctions on the wrapper
// (metadataGet + metadataPairs) do NOT consult the Value (they emit
// empty-source results unconditionally). Future v1.37.x binding bump
// will populate Value with a per-route Struct source pointer.
func pushMetadataUD(L *lua.LState) int {
	ud := L.NewUserData()
	// ud.Value left nil — empty-source contract per §11.6 D1 closure.
	L.SetMetatable(ud, L.GetTypeMetatable(metadataTypeName))
	L.Push(ud)
	return 1
}

// metadataGet implements metadata:get(name) per §11.6 D1 closure —
// ALWAYS returns lua.LNil regardless of the key argument. Matches
// upstream `MetadataMapWrapper::luaGet` on an empty source-Struct
// (returns nil for any key lookup).
//
// Future v1.32.4 → v1.37.x binding bump (per parent AMEND-12) will
// populate the wrapper's Value with a per-route Struct source pointer +
// activate the source-of-data; this LGFunction body would change to
// consult the Value for the key lookup.
func metadataGet(L *lua.LState) int {
	_ = L.CheckUserData(1) // receiver — ArgError on type mismatch
	_ = L.CheckString(2)   // key — discarded per empty-source contract
	L.Push(lua.LNil)
	return 1
}

// metadataPairs implements the __pairs metamethod for the envoy_metadata
// userdata. Returns a zero-iteration iterator per §11.6 D1 closure —
// `pairs(rh:metadata())` runs 0 iterations.
//
// Mirrors the headersPairs shape (returns 3 values: iter-fn + state +
// init-ctrl) per the installPairsShim Lua-5.2-compat protocol. The iter
// function unconditionally returns nil at first call → loop terminates
// immediately.
func metadataPairs(L *lua.LState) int {
	_ = L.CheckUserData(1) // receiver — ArgError on type mismatch
	iter := L.NewFunction(func(L2 *lua.LState) int {
		L2.Push(lua.LNil)
		return 1
	})
	L.Push(iter)
	L.Push(lua.LNil) // state (unused)
	L.Push(lua.LNil) // initial control (unused)
	return 3
}

// ---------------------------------------------------------------------
// :streamInfo():dynamicMetadata() — Bucket consumer
// ---------------------------------------------------------------------

// dynamicMetadataMethods is the method-name → LGFunction dispatch table
// for the dynamicMetadata userdata's __index metafield. 2 methods:
//   - :get(filterName, key) → marshaled Lua value or lua.LNil
//   - :set(filterName, key, value) → no return (marshals Lua → structpb)
//
// Both methods tolerate a nil Bucket per ADR-0085.
var dynamicMetadataMethods = map[string]lua.LGFunction{
	"get": dynamicMetadataGet,
	"set": dynamicMetadataSet,
}

// streamInfoDynamicMetadata implements streamInfo:dynamicMetadata() —
// returns a userdata wrapping the per-stream *dynamicmetadata.Bucket
// from the RequestHandleCallbacks adapter. The userdata stays callable
// even on a nil Bucket (per ADR-0085 nil-receiver discipline at the
// Bucket type + the dynamicMetadataGet/Set short-circuit on nil).
func streamInfoDynamicMetadata(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	var bucket *dynamicmetadata.Bucket
	if cb != nil {
		bucket = cb.DynamicMetadata()
	}
	return pushDynamicMetadataUD(L, bucket)
}

// streamInfoDynamicTypedMetadata implements
// streamInfo:dynamicTypedMetadata(filterName) — returns the entire
// filterName-keyed sub-map of the bucket as a Lua table, with each value
// marshaled via structpbToLua. Returns lua.LNil when:
//   - the bucket is nil (per ADR-0085 nil-tolerance);
//   - filterName is absent from the bucket's snapshot.
//
// The returned table is INDEPENDENT of the bucket — mutating the table
// does NOT mutate the bucket. Per SPEC §3.4 + §11.6 the typed-metadata
// access is a read-only snapshot.
func streamInfoDynamicTypedMetadata(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	filterName := L.CheckString(2)
	if cb == nil {
		L.Push(lua.LNil)
		return 1
	}
	bucket := cb.DynamicMetadata()
	if bucket == nil {
		L.Push(lua.LNil)
		return 1
	}
	snap := bucket.Snapshot()
	inner, ok := snap[filterName]
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	tbl := L.NewTable()
	for k, v := range inner {
		tbl.RawSetString(k, structpbToLua(L, v))
	}
	L.Push(tbl)
	return 1
}

// pushDynamicMetadataUD allocates a fresh LUserData wrapping the supplied
// *dynamicmetadata.Bucket (may be nil per ADR-0085 nil-tolerance) +
// attaches the envoy_dynamic_metadata metatable + pushes it onto the
// stack. Returns 1.
func pushDynamicMetadataUD(L *lua.LState, bucket *dynamicmetadata.Bucket) int {
	ud := L.NewUserData()
	ud.Value = bucket
	L.SetMetatable(ud, L.GetTypeMetatable(dynamicMetadataTypeName))
	L.Push(ud)
	return 1
}

// dynamicMetadataFromUD type-checks + extracts the *dynamicmetadata.Bucket
// from the userdata at the supplied stack index. Returns nil on:
//   - ud.Value is nil (nil-Bucket-wrapping userdata per ADR-0085 path)
//   - ud.Value is a typed nil *Bucket
//
// ArgError + nil-return on type mismatch.
//
// Per ADR-0085 the returned nil is observable but tolerated — the
// caller's :get/:set proceed via the Bucket's nil-receiver-tolerant
// methods.
func dynamicMetadataFromUD(L *lua.LState, idx int) *dynamicmetadata.Bucket {
	ud := L.CheckUserData(idx)
	if ud.Value == nil {
		return nil
	}
	b, ok := ud.Value.(*dynamicmetadata.Bucket)
	if !ok {
		L.ArgError(idx, "expected dynamic_metadata")
		return nil
	}
	return b
}

// dynamicMetadataGet implements dynamicMetadata:get(filterName, key) —
// looks up the (filterName, key) coordinate in the Bucket; returns the
// structpb.Value marshaled to a Lua value via structpbToLua, OR lua.LNil
// if absent or the bucket is nil.
func dynamicMetadataGet(L *lua.LState) int {
	b := dynamicMetadataFromUD(L, 1)
	filterName := L.CheckString(2)
	key := L.CheckString(3)
	v, ok := b.Get(filterName, key) // nil-receiver-tolerant per ADR-0085
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(structpbToLua(L, v))
	return 1
}

// dynamicMetadataSet implements dynamicMetadata:set(filterName, key,
// value) — marshals the Lua value to a *structpb.Value via luaToStructpb
// + stores at the (filterName, key) coordinate. No-op on a nil bucket
// per ADR-0085 (Bucket.Set is nil-receiver-tolerant).
//
// Returns 0 to Lua (no return value).
func dynamicMetadataSet(L *lua.LState) int {
	b := dynamicMetadataFromUD(L, 1)
	filterName := L.CheckString(2)
	key := L.CheckString(3)
	lv := L.CheckAny(4)
	spv := luaToStructpb(L, lv)
	b.Set(filterName, key, spv) // nil-receiver-tolerant per ADR-0085
	return 0
}

// ---------------------------------------------------------------------
// structpb.Value ↔ Lua-value marshaling helpers
// ---------------------------------------------------------------------

// structpbToLua converts a *structpb.Value to its Lua-value equivalent.
//
// Mapping:
//
//   - nil / NullValue        → lua.LNil
//   - BoolValue              → lua.LBool
//   - NumberValue            → lua.LNumber
//   - StringValue            → lua.LString
//   - ListValue              → lua.LTable (1-indexed per Lua convention)
//   - StructValue            → lua.LTable (hash-keyed)
//
// Recursive: List + Struct values are walked element-by-element with
// each inner value re-marshaled via structpbToLua. Cycle-free per
// structpb's tree-shape (Value is a oneof; structpb does NOT support
// self-referential values).
func structpbToLua(L *lua.LState, v *structpb.Value) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch k := v.GetKind().(type) {
	case *structpb.Value_NullValue:
		return lua.LNil
	case *structpb.Value_BoolValue:
		if k.BoolValue {
			return lua.LTrue
		}
		return lua.LFalse
	case *structpb.Value_NumberValue:
		return lua.LNumber(k.NumberValue)
	case *structpb.Value_StringValue:
		return lua.LString(k.StringValue)
	case *structpb.Value_ListValue:
		tbl := L.NewTable()
		if k.ListValue != nil {
			for _, item := range k.ListValue.Values {
				tbl.Append(structpbToLua(L, item))
			}
		}
		return tbl
	case *structpb.Value_StructValue:
		tbl := L.NewTable()
		if k.StructValue != nil {
			for kk, vv := range k.StructValue.Fields {
				tbl.RawSetString(kk, structpbToLua(L, vv))
			}
		}
		return tbl
	default:
		// Unknown variant — defensive null. Cannot happen at the current
		// structpb schema but future schema extensions would surface here.
		return lua.LNil
	}
}

// luaToStructpb converts a Lua value to its *structpb.Value equivalent.
//
// Mapping:
//
//   - lua.LNil                       → NullValue
//   - lua.LBool                      → BoolValue
//   - lua.LNumber                    → NumberValue
//   - lua.LString                    → StringValue
//   - lua.LTable (contig 1..N keys)  → ListValue (1-indexed)
//   - lua.LTable (other / mixed)     → StructValue (string keys; non-string
//     keys are SKIPPED per upstream
//     wrappers.cc luaToProtoValue tolerance)
//   - Unsupported (LFunction /
//     LUserData / LChannel / LState)  → NullValue (defensive)
//
// Recursive: nested tables marshal via the same shape detection.
//
// Table-shape detection: a table is treated as a list (ListValue) iff it
// has at least one 1-indexed integer key AND ALL keys are contiguous
// integers 1..N. Mixed-key tables (e.g. some string keys + some integer
// keys) fall through to StructValue (string keys preserved; non-string
// keys silently dropped per the upstream tolerance pattern).
func luaToStructpb(L *lua.LState, lv lua.LValue) *structpb.Value {
	switch v := lv.(type) {
	case *lua.LNilType:
		return structpb.NewNullValue()
	case lua.LBool:
		return structpb.NewBoolValue(bool(v))
	case lua.LNumber:
		return structpb.NewNumberValue(float64(v))
	case lua.LString:
		return structpb.NewStringValue(string(v))
	case *lua.LTable:
		return luaTableToStructpb(L, v)
	default:
		// Unsupported variant — defensive null per the upstream
		// wrappers.cc luaToProtoValue return-empty pattern.
		return structpb.NewNullValue()
	}
}

// luaTableToStructpb detects whether the table is a list (contiguous
// 1..N integer keys; at least one entry) OR a struct (string-keyed; OR
// empty), then dispatches to the appropriate structpb constructor.
func luaTableToStructpb(L *lua.LState, t *lua.LTable) *structpb.Value {
	// Detect list-shape: at least one numeric 1-indexed key AND every
	// key is a contiguous integer in [1, N]. Empty tables fall through
	// to struct-shape (Lua has no intrinsic empty-list-vs-empty-struct
	// distinction; we prefer struct as the default since :set with an
	// empty table is most-likely a struct-shaped argument).
	maxIdx := 0
	allIntKeys := true
	hasAnyKey := false
	t.ForEach(func(k, _ lua.LValue) {
		hasAnyKey = true
		if n, ok := k.(lua.LNumber); ok {
			i := int(n)
			if float64(i) == float64(n) && i >= 1 {
				if i > maxIdx {
					maxIdx = i
				}
				return
			}
		}
		allIntKeys = false
	})
	if hasAnyKey && allIntKeys && maxIdx > 0 {
		// Verify contiguity 1..maxIdx — count is preserved if no holes.
		count := 0
		t.ForEach(func(k, _ lua.LValue) {
			if _, ok := k.(lua.LNumber); ok {
				count++
			}
		})
		if count == maxIdx {
			// List-shape.
			values := make([]*structpb.Value, 0, maxIdx)
			for i := 1; i <= maxIdx; i++ {
				lv := t.RawGetInt(i)
				values = append(values, luaToStructpb(L, lv))
			}
			return structpb.NewListValue(&structpb.ListValue{Values: values})
		}
	}
	// Struct-shape (covers empty + hash + mixed-skipping-non-string).
	fields := map[string]*structpb.Value{}
	t.ForEach(func(k, v lua.LValue) {
		if ks, ok := k.(lua.LString); ok {
			fields[string(ks)] = luaToStructpb(L, v)
		}
		// Non-string keys silently dropped per upstream tolerance.
	})
	return structpb.NewStructValue(&structpb.Struct{Fields: fields})
}
