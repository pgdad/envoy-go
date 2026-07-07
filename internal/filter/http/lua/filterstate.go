package lua

// filterstate.go — Task 13 (phase 22.2 IMPL) filter-state bridge per
// SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4 + PLAN Task 13 + ADR-0192
// §Decision body anticipation.
//
// # 25.2 IMPL Task 10 MIGRATION (ADR-0207 §3.4 + 25.2 SPEC §14.5)
//
// The phase-22.2 IMPL of this file stored its per-stream data in a
// `map[string]any` field on *filter (lua.go) accessed via the cb adapter's
// FilterState() / SetFilterState() pair. At phase 25.2 IMPL Task 10 the
// bridge's :get + :set LGFunctions delegate through the NEW shared
// `internal/filterstate/*Bucket` primitive (per ADR-0207 §3.4 MIGRATES) —
// each :get/:set constructs an ephemeral *Bucket populated from the
// current map, exercises the Bucket's Get/Set API (including the
// FilterStateObject adapter + StateType discriminator) + materializes the
// bucket state back into the map for cb-adapter visibility.
//
// MIGRATION SCOPE per 25.2 SPEC §14.5 (non-breaking discipline):
//   - The `:filterState()` Lua surface stays BYTE-IDENTICAL to phase-22.2.
//   - The `*filter.filterState map[string]any` field STAYS as the cb-adapter
//     observable storage (RequestHandleCallbacks.FilterState() returns
//     map[string]any; phase-22.2 lua filterstate tests pin this).
//   - The 2 envoy-go-strict divergences from AMEND-22.2-4 (mutation
//     exposure + typed Lua-value marshaling) carry forward UNCHANGED.
//   - All phase-22.2 lua filterstate tests pass UNCHANGED (no test file
//     modifications).
//
// The Bucket primitive's value at the lua bridge is API-shape alignment:
//   - The `luaFilterStateObject` adapter wraps each Lua-marshaled value
//     + satisfies `filterstate.FilterStateObject` (Marshal/Unmarshal via
//     gob-encoded payload; HasData; StateType returns Mutable per the
//     phase-22.2 mutation-exposure divergence per AMEND-22.2-4).
//   - The Bucket's Set conflict-check (Mutable-vs-ReadOnly rejection) is
//     a no-op for the lua surface (the adapter always returns Mutable
//     per AMEND-22.2-4) — but the dispatch surface is shared verbatim
//     with the wasm consumer at Task 13 (which DOES emit Mutable +
//     ReadOnly objects per the proxy-wasm `filter_state.*` semantics).
//
// # Surface (unchanged from phase-22.2)
//
// `streamInfo:filterState()` returns a filterstate userdata wrapping
// the per-stream string-keyed `map[string]any` stored on the *filter
// struct (lua.go field). The userdata exposes 2 methods:
//
//   - :get(name)        — returns the marshaled Lua value at key `name`
//                         or lua.LNil if absent / map nil.
//   - :set(name, value) — marshals the Lua value to `any` and stores
//                         at key `name`. Envoy-go-strict per AMEND-22.2-4
//                         (upstream FilterStateWrapper is strictly
//                         read-only).
//
// # Marshaling table (per SPEC §11.8 D4 + AMEND-22.2-4) — UNCHANGED
//
//   - Go-side any → Lua value at :get
//
//	string                  → LString
//	int / int64 / int32     → LNumber
//	float32 / float64       → LNumber
//	bool                    → LBool
//	map[string]any          → LTable (string-keyed; recursive)
//	[]any                   → LTable (1-indexed; recursive)
//	nil                     → LNil
//	unknown type            → LNil (defensive — never panic)
//
//   - Lua value → Go-side any at :set
//
//	LNil                    → nil
//	LString                 → string
//	LNumber                 → float64
//	LBool                   → bool
//	LTable (contiguous 1..N integer keys) → []any (1-indexed)
//	LTable (other / mixed)                → map[string]any (string keys)
//	LFunction / LChannel /
//	LUserData / LState      → runtime error (envoy-go-strict; documents
//	                          the marshaling contract per SPEC §11.8 D4)
//
// # Per-stream lifecycle (unchanged from phase-22.2)
//
//   - The `map[string]any` lives on *filter (lua.go) — lazy-allocated
//     on first :set (or pre-populated via FilterState() accessor on
//     the cb adapter).
//
//   - At *filter.OnDestroy: the map is set to nil. Defensive cleanup
//     against unintended retention through the cb adapter or any
//     externally-held pointers.
//
//   - Cross-stream isolation: each *filter has its own map. N parallel
//     streams each get a separate map; no shared state.
//
// # 2 envoy-go-strict departures from upstream (unchanged from phase-22.2)
//
// Per AMEND-22.2-4 (anchored at SPEC §11.8 D4 CLOSURE):
//
//   1. `:set(name, value)` exposed at Lua surface (upstream
//      FilterStateWrapper is strictly read-only).
//
//   2. `:get(name)` returns typed Lua values (LString / LNumber / LBool
//      / LTable recursive) per the marshaling table; upstream returns
//      `serializeAsString()` Lua strings always.
//
// Both depart from upstream parity for envoy-go-specific operational
// reasons: envoy-go has no in-stream filter that mutates FilterState
// at the C++ level, so :set on the Lua surface is the most natural
// mutation seam. And the typed return shape aligns with the rest of
// the bridge surface (dynamicMetadata + headers all return typed
// values; not stringified). 2 envoy-go-strict departure records
// anticipated at BEHAVIOR_CONTRACT.md §13.6 (Task 19 atomic landing).

import (
	"bytes"
	"encoding/gob"
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/pgdad/envoy-go/internal/filterstate"
)

// filterStateTypeName is the metatable registry-key used by gopher-lua's
// NewTypeMetatable. Distinct from streamInfoTypeName so script authors
// who consult getmetatable(streamInfo():filterState()) observe a
// distinct identity.
const filterStateTypeName = "envoy_filter_state"

// filterStateMethods is the method-name → LGFunction dispatch table for
// the filterstate userdata's __index metafield. 2 methods per AMEND-22.2-4
// — :get + :set. Both tolerate a nil-map receiver (the bridge LGFunctions
// short-circuit on nil — :get returns LNil; :set is a silent no-op).
var filterStateMethods = map[string]lua.LGFunction{
	"get": filterStateGet,
	"set": filterStateSet,
}

// installFilterStateMetatable registers the metatable for the filterstate
// userdata under filterStateTypeName per Task 13 (phase 22.2 IMPL). The
// metatable holds:
//   - __index → table of 2 filterstate methods (:get / :set)
//
// The per-stream-VM setup (decode_headers.go) calls this helper once at
// NewVM time alongside the other install* helpers. The filterstate
// userdata is allocated on-demand inside
// streamInfo:filterState() — it wraps a *filterStateRef (a thin
// pointer-indirection to the per-stream map; see filterStateRef
// docstring for the rationale).
func installFilterStateMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(filterStateTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), filterStateMethods))
	return mt
}

// filterStateRef is the Go-side value wrapped by the filterstate
// userdata. It holds a function returning the underlying per-stream
// map[string]any AND a function to install a new map (used by :set on
// first write to lazily allocate a fresh map if the cb returned nil).
//
// Why a ref struct rather than wrapping the map directly?
//
//   - The map[string]any may be nil at :filterState() construction time
//     (lazy-allocation discipline — the per-stream map is only allocated
//     on first :set, not pre-allocated at *filter construction). Passing
//     the map by value (as LUserData.Value) would freeze the nil pointer
//     and break the :set lazy-allocation path — once the userdata is
//     constructed with a nil-map, no :set could lazy-allocate AND have
//     the new map observable via the cb accessor.
//
//   - The ref's getter + setter closures route :set lazy-allocation
//     through the cb adapter (typically a *filter back-pointer), so the
//     freshly-allocated map is observable on subsequent :filterState()
//     calls + on the Go-side *filter struct.
type filterStateRef struct {
	get func() map[string]any
	set func(map[string]any)
}

// pushFilterStateUDFromCb is the production helper consumed by
// streamInfoFilterState (streaminfo.go). It wires both the getter +
// setter through the RequestHandleCallbacks adapter's FilterState +
// SetFilterState methods (Task 13 interface extension). The lazy-
// allocation path (a :set on a previously-nil map) installs the
// freshly-allocated map back onto *filter.filterState via
// cb.SetFilterState — making the new map observable on subsequent
// :filterState() calls + on the Go-side *filter struct.
//
// nil-cb tolerance: a nil cb yields a userdata whose :get always
// returns LNil + whose :set is silently dropped (matches ADR-0085
// nil-receiver discipline).
func pushFilterStateUDFromCb(L *lua.LState, cb RequestHandleCallbacks) int {
	var ref *filterStateRef
	if cb != nil {
		ref = &filterStateRef{
			get: func() map[string]any { return cb.FilterState() },
			set: func(m map[string]any) { cb.SetFilterState(m) },
		}
	}
	ud := L.NewUserData()
	ud.Value = ref
	L.SetMetatable(ud, L.GetTypeMetatable(filterStateTypeName))
	L.Push(ud)
	return 1
}

// filterStateFromUD extracts the *filterStateRef from the userdata at
// the supplied stack index. Returns nil on:
//   - ud.Value is nil (defensive)
//   - ud.Value is not a *filterStateRef (type mismatch)
//
// ArgError + nil-return on type mismatch.
func filterStateFromUD(L *lua.LState, idx int) *filterStateRef {
	ud := L.CheckUserData(idx)
	if ud.Value == nil {
		return nil
	}
	ref, ok := ud.Value.(*filterStateRef)
	if !ok {
		L.ArgError(idx, "expected filter_state")
		return nil
	}
	return ref
}

// ---------------------------------------------------------------------
// 25.2 IMPL Task 10 MIGRATION — luaFilterStateObject adapter
// ---------------------------------------------------------------------

// luaFilterStateObject is the lua-bridge adapter that wraps a Lua-side
// marshaled value (any-typed per the AMEND-22.2-4 typed Lua-value
// marshaling table) and satisfies `filterstate.FilterStateObject`. The
// adapter is constructed at :set time (wrapping the luaToAny output) and
// queried at :get time (anyToLua unwraps the value back to a Lua value).
//
// StateType — returns filterstate.StateTypeMutable for ALL lua-bridge
// entries per the phase-22.2 AMEND-22.2-4 mutation-exposure divergence
// (upstream FilterStateWrapper is strictly read-only; the envoy-go-strict
// surface exposes :set with full Mutable semantics).
//
// Marshal / Unmarshal — serializes the wrapped `any` value via gob
// encoding (the simplest serializer that handles the AMEND-22.2-4 typed
// marshaling table's any-typed cells: string, float64, int64, bool,
// nested map[string]any, nested []any). The Marshal/Unmarshal pair is
// EXERCISED ONLY by the wasm property-dispatch consumer at Task 13 (the
// lua bridge never calls Marshal/Unmarshal — :get/:set route through
// .value direct access). Defensive copy at Marshal per the
// filterstate.FilterStateObject contract: each call returns a fresh
// byte slice.
//
// HasData — true when the wrapped value is non-nil (mirrors upstream
// Envoy StreamInfo::FilterState::Object::hasData semantic). nil-valued
// entries are explicitly allowed (the lua marshaling table maps LNil →
// nil so a `fs:set("k", nil)` stores a non-empty entry whose HasData
// returns false).
type luaFilterStateObject struct {
	value any
}

// Marshal serializes the wrapped value to a fresh byte slice. Uses gob
// encoding (registered for the AMEND-22.2-4 typed cells in the package
// init below). Returns a fresh slice on each call per the
// filterstate.FilterStateObject contract.
func (o *luaFilterStateObject) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	// Wrap in a single-field envelope so gob can decode the any-typed
	// payload via interface dispatch on the decoder side. Direct
	// gob.Encode of an `any` value would discard the dynamic type info.
	envelope := struct{ V any }{V: o.value}
	if err := enc.Encode(envelope); err != nil {
		return nil, fmt.Errorf("luaFilterStateObject: marshal: %w", err)
	}
	// Defensive copy: bytes.Buffer's underlying slice may be reused on
	// subsequent calls; return an independent slice owned by the caller.
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

// Unmarshal re-hydrates the wrapped value from the serialized payload.
// nil input is treated as the empty payload (sets value to nil) per the
// filterstate.FilterStateObject contract.
func (o *luaFilterStateObject) Unmarshal(b []byte) error {
	if len(b) == 0 {
		o.value = nil
		return nil
	}
	dec := gob.NewDecoder(bytes.NewReader(b))
	var envelope struct{ V any }
	if err := dec.Decode(&envelope); err != nil {
		return fmt.Errorf("luaFilterStateObject: unmarshal: %w", err)
	}
	o.value = envelope.V
	return nil
}

// HasData reports whether the wrapped value is non-nil. Mirrors upstream
// Envoy StreamInfo::FilterState::Object::hasData semantic.
func (o *luaFilterStateObject) HasData() bool {
	return o.value != nil
}

// StateType returns filterstate.StateTypeMutable for ALL lua-bridge
// entries per the phase-22.2 AMEND-22.2-4 mutation-exposure divergence
// (envoy-go-strict — upstream FilterStateWrapper is strictly read-only;
// the envoy-go lua surface exposes :set with full Mutable semantics).
func (o *luaFilterStateObject) StateType() filterstate.StateType {
	return filterstate.StateTypeMutable
}

// Compile-time witness: *luaFilterStateObject satisfies the
// filterstate.FilterStateObject contract. Catches drift between the
// adapter's method set and the primitive's interface signature at
// build time rather than runtime.
var _ filterstate.FilterStateObject = (*luaFilterStateObject)(nil)

func init() {
	// Register the AMEND-22.2-4 typed marshaling cells with gob so the
	// envelope's any-typed payload round-trips correctly through
	// Marshal/Unmarshal. The wasm consumer at Task 13 wraps its own
	// payload bytes directly (no gob round-trip) — gob is the lua-side
	// serializer only.
	gob.Register(map[string]any{})
	gob.Register([]any{})
	gob.Register("")
	gob.Register(int64(0))
	gob.Register(float64(0))
	gob.Register(false)
}

// bucketFromMap constructs a fresh *filterstate.Bucket populated from
// the supplied map. Each map entry is wrapped in a fresh
// *luaFilterStateObject adapter. Used at the start of each :get/:set
// dispatch so the bridge logic delegates through the *Bucket primitive's
// Get/Set API per ADR-0207 §3.4 MIGRATES.
//
// nil input → empty bucket (filterstate.NewBucket() returns a non-nil
// empty bucket; callers may proceed without a nil guard).
func bucketFromMap(m map[string]any) *filterstate.Bucket {
	b := filterstate.NewBucket()
	for k, v := range m {
		// Set on a fresh bucket cannot fail (no existing entry to
		// conflict against + the adapter is non-nil); the error return
		// is defensive against future primitive evolution.
		_ = b.Set(k, &luaFilterStateObject{value: v})
	}
	return b
}

// materializeBucketIntoMap copies the bucket's entries back into a
// fresh map[string]any with each *luaFilterStateObject unwrapped to its
// inner Lua-marshaled value. Used at the end of :set dispatch so the
// cb-adapter-observable storage stays in sync with the bucket. The
// returned map preserves the cb-adapter contract (RequestHandleCallbacks.
// FilterState() returns map[string]any per phase-22.2 + 25.2 §14.5
// non-breaking discipline).
func materializeBucketIntoMap(b *filterstate.Bucket) map[string]any {
	keys := b.Keys()
	m := make(map[string]any, len(keys))
	for _, k := range keys {
		if obj, ok := b.Get(k); ok {
			if lobj, ok := obj.(*luaFilterStateObject); ok {
				m[k] = lobj.value
			}
		}
	}
	return m
}

// ---------------------------------------------------------------------
// Bridge LGFunctions — :get + :set delegate through *filterstate.Bucket
// ---------------------------------------------------------------------

// filterStateGet implements filterState:get(name) per SPEC §3.4 +
// §11.8 D4 + AMEND-22.2-4. Returns the marshaled Lua value at key `name`
// or lua.LNil if absent / map is nil.
//
// 25.2 IMPL Task 10 MIGRATION: routes through *filterstate.Bucket per
// ADR-0207 §3.4 MIGRATES — constructs an ephemeral *Bucket populated
// from the cb-adapter's current map state, calls bucket.Get to exercise
// the Bucket primitive's Get API + FilterStateObject interface, then
// unwraps the *luaFilterStateObject.value via anyToLua.
//
// Marshaling: see anyToLua + the file header docstring table.
func filterStateGet(L *lua.LState) int {
	ref := filterStateFromUD(L, 1)
	name := L.CheckString(2)
	if ref == nil {
		L.Push(lua.LNil)
		return 1
	}
	m := ref.get()
	if m == nil {
		L.Push(lua.LNil)
		return 1
	}
	// Route through *filterstate.Bucket per ADR-0207 §3.4 MIGRATES.
	b := bucketFromMap(m)
	obj, ok := b.Get(name)
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	lobj, ok := obj.(*luaFilterStateObject)
	if !ok {
		// Defensive — the bucket was populated by bucketFromMap with
		// luaFilterStateObject entries only. A type mismatch here would
		// signal a primitive-side evolution; surface as LNil rather than
		// panic.
		L.Push(lua.LNil)
		return 1
	}
	L.Push(anyToLua(L, lobj.value))
	return 1
}

// filterStateSet implements filterState:set(name, value) per
// AMEND-22.2-4 (envoy-go-strict — upstream is strictly read-only).
// Marshals the Lua value to `any` (see luaToAny) and stores at key
// `name` on the underlying per-stream map.
//
// 25.2 IMPL Task 10 MIGRATION: routes through *filterstate.Bucket per
// ADR-0207 §3.4 MIGRATES — wraps the luaToAny output in a fresh
// *luaFilterStateObject adapter, constructs an ephemeral *Bucket from
// the current map state, calls bucket.Set to exercise the Bucket
// primitive's Set API + StateType conflict semantics + ErrNilFilterStateObject
// rejection, then materializes the bucket back into a fresh map[string]any
// for the cb-adapter (RequestHandleCallbacks.SetFilterState).
//
// Lazy-allocation: if the underlying map is nil at :set time, the
// ephemeral bucket starts empty + the materialized map is installed
// via ref.set so subsequent :get calls observe the freshly-allocated
// map. The cb-adapter's *filter back-pointer writes the map through
// to *filter.filterState.
//
// Runtime error: unsupported Lua types (LFunction / LChannel /
// LUserData (non-filterstate) / LState) raise a Lua runtime error per
// AMEND-22.2-4 marshaling contract. Captures envoy-go-strict marshaling
// expectations at the bridge layer.
//
// Bucket.Set conflict semantics on the lua surface: the lua adapter
// returns StateTypeMutable for ALL entries (per AMEND-22.2-4 mutation
// exposure), so the Mutable-vs-ReadOnly conflict check in *Bucket.Set
// never rejects — every set replaces the existing entry. The ignored
// error from bucket.Set is a defensive guard against future adapter
// evolution (if a ReadOnly variant is introduced, the rejection would
// silently no-op the :set, matching upstream's read-only FilterState
// posture).
func filterStateSet(L *lua.LState) int {
	ref := filterStateFromUD(L, 1)
	name := L.CheckString(2)
	lv := L.CheckAny(3)
	if ref == nil {
		// nil-receiver case — silent no-op per ADR-0085.
		return 0
	}
	// Marshal Lua value first (catches the unsupported-type case before
	// we touch the map). luaToAny raises L.RaiseError on unsupported
	// types — control does not return on the error path.
	v := luaToAny(L, lv)
	// Route through *filterstate.Bucket per ADR-0207 §3.4 MIGRATES.
	m := ref.get()
	b := bucketFromMap(m)
	// The lua adapter always reports StateTypeMutable; the Bucket's
	// conflict check never rejects on the lua surface. Defensive
	// error-ignore per the adapter-evolution rationale in the docstring.
	_ = b.Set(name, &luaFilterStateObject{value: v})
	// Materialize the bucket back into the cb-adapter-observable map.
	// Always re-install via ref.set so the lazy-allocation path covers
	// the nil-map case (the cb-adapter writes the new map through to
	// *filter.filterState).
	ref.set(materializeBucketIntoMap(b))
	return 0
}

// ---------------------------------------------------------------------
// Marshaling helpers — any ↔ Lua value
// ---------------------------------------------------------------------

// anyToLua converts a Go-side `any` value to its Lua-value equivalent
// per the marshaling table at the file header. Recursive for maps +
// slices. Unknown types yield LNil (defensive — never panic).
func anyToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(x)
	case bool:
		if x {
			return lua.LTrue
		}
		return lua.LFalse
	case int:
		return lua.LNumber(x)
	case int32:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case uint:
		return lua.LNumber(x)
	case uint32:
		return lua.LNumber(x)
	case uint64:
		return lua.LNumber(x)
	case float32:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	case map[string]any:
		tbl := L.NewTable()
		for k, vv := range x {
			tbl.RawSetString(k, anyToLua(L, vv))
		}
		return tbl
	case []any:
		tbl := L.NewTable()
		for _, item := range x {
			tbl.Append(anyToLua(L, item))
		}
		return tbl
	default:
		// Unknown type — defensive LNil. Avoid panicking; the marshaling
		// contract is best-effort for unknown Go-side types (the
		// envoy-go-strict surface only stores types we marshal at :set,
		// so the unknown-at-:get path is reachable only via Go-side test
		// code seeding the map with an exotic type).
		return lua.LNil
	}
}

// luaToAny converts a Lua value to its Go-side `any` equivalent per the
// marshaling table at the file header. Recursive for tables (list-shape
// → []any; otherwise → map[string]any).
//
// Unsupported types (LFunction / LChannel / LUserData / LState) raise a
// Lua runtime error per AMEND-22.2-4. Documents the marshaling contract
// at the bridge layer.
func luaToAny(L *lua.LState, lv lua.LValue) any {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LString:
		return string(v)
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case *lua.LTable:
		return luaTableToAny(L, v)
	default:
		// Unsupported Lua type per AMEND-22.2-4 marshaling contract.
		// Surface as a Lua runtime error so the script author sees a
		// clear failure rather than silent data loss.
		L.RaiseError("filterState:set: unsupported value type %s", lv.Type().String())
		return nil // unreachable; RaiseError longjmps out
	}
}

// luaTableToAny detects whether the table is a list-shape (contiguous
// 1..N integer keys; at least one entry) OR a map-shape (string-keyed
// OR empty), then dispatches to the appropriate Go-side type. Mirrors
// the structpb marshaling helper at metadata.go (luaTableToStructpb);
// shape detection is shared via luaTableListShape (metadata.go). Empty
// tables fall through to map-shape (Lua has no intrinsic
// empty-list-vs-empty-map distinction; we prefer map as the default
// for envoy-go-strict match with the dynamicMetadata helper).
func luaTableToAny(L *lua.LState, t *lua.LTable) any {
	if isList, maxIdx := luaTableListShape(t); isList {
		values := make([]any, 0, maxIdx)
		for i := 1; i <= maxIdx; i++ {
			lv := t.RawGetInt(i)
			values = append(values, luaToAny(L, lv))
		}
		return values
	}
	// Map-shape (covers empty + hash + mixed-skipping-non-string).
	out := map[string]any{}
	t.ForEach(func(k, v lua.LValue) {
		if ks, ok := k.(lua.LString); ok {
			out[string(ks)] = luaToAny(L, v)
		}
		// Non-string keys silently dropped per the same tolerance as
		// the dynamicMetadata marshaling helper at metadata.go.
	})
	return out
}
