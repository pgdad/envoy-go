package lua

// filterstate.go — Task 13 (phase 22.2 IMPL) filter-state bridge per
// SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4 + PLAN Task 13 + ADR-0192
// §Decision body anticipation.
//
// # Surface
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
// # Marshaling table (per SPEC §11.8 D4 + AMEND-22.2-4)
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
// # Per-stream lifecycle
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
// # 2 envoy-go-strict departures from upstream
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
	"fmt"

	lua "github.com/yuin/gopher-lua"
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

// filterStateGet implements filterState:get(name) per SPEC §3.4 +
// §11.8 D4 + AMEND-22.2-4. Returns the marshaled Lua value at key `name`
// or lua.LNil if absent / map is nil.
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
	v, ok := m[name]
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(anyToLua(L, v))
	return 1
}

// filterStateSet implements filterState:set(name, value) per
// AMEND-22.2-4 (envoy-go-strict — upstream is strictly read-only).
// Marshals the Lua value to `any` (see luaToAny) and stores at key
// `name` on the underlying per-stream map.
//
// Lazy-allocation: if the underlying map is nil at :set time, a fresh
// map is allocated AND installed via ref.set so subsequent :get calls
// observe the freshly-allocated map.
//
// Runtime error: unsupported Lua types (LFunction / LChannel /
// LUserData (non-filterstate) / LState) raise a Lua runtime error per
// AMEND-22.2-4 marshaling contract. Captures envoy-go-strict marshaling
// expectations at the bridge layer.
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
	m := ref.get()
	if m == nil {
		m = make(map[string]any)
		ref.set(m)
	}
	m[name] = v
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
// the structpb marshaling helper at metadata.go (luaTableToStructpb).
func luaTableToAny(L *lua.LState, t *lua.LTable) any {
	// Detect list-shape: at least one numeric 1-indexed key AND every
	// key is a contiguous integer in [1, N]. Empty tables fall through
	// to map-shape (Lua has no intrinsic empty-list-vs-empty-map
	// distinction; we prefer map as the default for envoy-go-strict
	// match with the dynamicMetadata helper).
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
		count := 0
		t.ForEach(func(k, _ lua.LValue) {
			if _, ok := k.(lua.LNumber); ok {
				count++
			}
		})
		if count == maxIdx {
			values := make([]any, 0, maxIdx)
			for i := 1; i <= maxIdx; i++ {
				lv := t.RawGetInt(i)
				values = append(values, luaToAny(L, lv))
			}
			return values
		}
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

// _ unused-helper suppression: fmt is used by the marshaling-error
// formatting only when L.RaiseError is invoked at the unsupported-type
// path. The Go compiler requires the import to be referenced to compile
// cleanly; the RaiseError format-string call itself satisfies that.
var _ = fmt.Sprintf
