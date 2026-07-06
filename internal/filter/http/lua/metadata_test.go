package lua

// metadata_test.go — Task 9 (phase 22.2 IMPL) metadata + dynamicMetadata
// bridge tests per SPEC §3.4 + §11.6 D1 closure + PLAN Task 9 acceptance.
//
// Coverage:
//
//   - Test_RequestHandleMetadata_returns_callable_empty_userdata —
//     `:metadata()` returns userdata (NEVER nil) per §11.6 D1 closure +
//     upstream MetadataMapWrapper always-callable pattern.
//   - TestMetadata_get_returns_nil — `:metadata():get("anykey")` returns
//     lua.LNil for ANY key per §11.6.4 binding-gap empty-source.
//   - TestMetadata_pairs_yields_zero_iterations — `pairs(:metadata())`
//     runs 0 iterations (empty iterator) per §11.6.3.
//   - TestDynamicMetadata_set_get_roundtrip — `:streamInfo():dynamicMetadata():set()`
//     followed by `:get()` round-trips for string + number + bool + list +
//     struct value types.
//   - TestDynamicMetadata_cross_filter_key_independence — same key under
//     different filterNames are independent.
//   - TestDynamicTypedMetadata_returns_typed_value — per the SPEC's
//     typed-vs-untyped semantic: `:dynamicTypedMetadata(filterName)`
//     returns the entire filterName-keyed sub-map as a Lua table
//     containing typed structpb.Value-marshaled entries.
//   - TestDynamicMetadata_nil_bucket_tolerance — test-double cb returns
//     nil from DynamicMetadata(); script still works (returns nil for
//     any get; no panic).
//   - TestStructpbToLua_table_driven + TestLuaToStructpb_table_driven —
//     marshaling helpers covering null/number/string/list/struct/bool.

import (
	"net/http"
	"testing"

	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// fakeCallbacksWithBucket extends fakeCallbacks (bridge_test.go) with a
// per-stream *dynamicmetadata.Bucket so :streamInfo():dynamicMetadata()
// has a non-nil backing store. Used by the round-trip + cross-filter
// tests.
type fakeCallbacksWithBucket struct {
	fakeCallbacks
	bucket *dynamicmetadata.Bucket
}

func (f *fakeCallbacksWithBucket) DynamicMetadata() *dynamicmetadata.Bucket {
	return f.bucket
}

// newBridgedVMWithBucket constructs a VM with the bridge metatables +
// streamInfo metatable + metadata + dynamicMetadata metatables + a
// fakeCallbacksWithBucket wired into reqCtx.cb so the script can access
// :metadata() and :streamInfo():dynamicMetadata() + :dynamicTypedMetadata().
func newBridgedVMWithBucket(t *testing.T, bucket *dynamicmetadata.Bucket) *luaprim.VM {
	t.Helper()
	cb := &fakeCallbacksWithBucket{bucket: bucket}
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installMetadataMetatable(L)
	installDynamicMetadataMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: http.Header{}, cb: cb}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// ---------------------------------------------------------------------
// :metadata() callable empty userdata (NEVER nil per §11.6 D1 closure)
// ---------------------------------------------------------------------

// TestMetadata_RequestHandleMetadata_returns_callable_empty_userdata
// pins the D1 CLOSURE contract: `:metadata()` returns userdata (NEVER
// nil) regardless of source-of-data presence — matches upstream
// MetadataMapWrapper always-callable pattern (§11.6.2 test evidence).
func TestMetadata_RequestHandleMetadata_returns_callable_empty_userdata(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `
		local m = rh:metadata()
		result_type = type(m)
		result_is_nil = (m == nil)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(rh:metadata()) = %q; want %q", got, "userdata")
	}
	if isTruthy := vm.State().GetGlobal("result_is_nil"); isTruthy != lua.LFalse {
		t.Fatalf("rh:metadata() == nil; want non-nil per §11.6 D1 closure")
	}
}

// TestMetadata_ResponseHandleMetadata_returns_callable_empty_userdata
// mirrors the request-side test on the encode side (response_handle:metadata()
// behaves identically per upstream parity).
func TestMetadata_ResponseHandleMetadata_returns_callable_empty_userdata(t *testing.T) {
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installMetadataMetatable(L)
	installDynamicMetadataMetatable(L)
	installPairsShim(L)
	ctx := &responseHandleContext{headers: http.Header{}}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", ud)
	runScript(t, vm, `
		local m = resp:metadata()
		result_type = type(m)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(resp:metadata()) = %q; want %q", got, "userdata")
	}
}

// TestMetadata_get_returns_nil verifies `:metadata():get(k)` returns
// lua.LNil for any key per §11.6 D1 closure (empty metadata source at
// v1.32.4 binding-gap; all lookups return nil but the wrapper itself is
// non-nil).
func TestMetadata_get_returns_nil(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `result = rh:metadata():get("foo.bar")`)
	if !isGlobalNil(vm, "result") {
		t.Fatalf("rh:metadata():get(\"foo.bar\") = %v; want nil",
			vm.State().GetGlobal("result"))
	}
}

// TestMetadata_get_returns_nil_for_any_key exercises multiple keys to
// pin the "all lookups return nil regardless of key" contract.
func TestMetadata_get_returns_nil_for_any_key(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `
		r1 = rh:metadata():get("")
		r2 = rh:metadata():get("a.b.c")
		r3 = rh:metadata():get("envoy.filters.http.lua")
	`)
	for _, name := range []string{"r1", "r2", "r3"} {
		if !isGlobalNil(vm, name) {
			t.Errorf("%s = %v; want nil for any key per §11.6 D1 closure",
				name, vm.State().GetGlobal(name))
		}
	}
}

// TestMetadata_pairs_yields_zero_iterations verifies pairs() over the
// metadata userdata yields zero iterations (empty iterator) per
// §11.6.3 + upstream MetadataMapWrapper empty-Struct pattern.
func TestMetadata_pairs_yields_zero_iterations(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `
		count = 0
		for k, v in pairs(rh:metadata()) do
			count = count + 1
		end
	`)
	if n := getGlobalInt(t, vm, "count"); n != 0 {
		t.Fatalf("pairs(rh:metadata()) yielded %d iterations; want 0", n)
	}
}

// ---------------------------------------------------------------------
// :streamInfo():dynamicMetadata() — Bucket consumer
// ---------------------------------------------------------------------

// TestDynamicMetadata_set_get_roundtrip_string verifies set/get
// round-trip for a string-valued metadata entry.
func TestDynamicMetadata_set_get_roundtrip_string(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		rh:streamInfo():dynamicMetadata():set("envoy.lua", "k", "hello")
		result = rh:streamInfo():dynamicMetadata():get("envoy.lua", "k")
	`)
	if got := getGlobalString(t, vm, "result"); got != "hello" {
		t.Fatalf("round-trip string = %q; want %q", got, "hello")
	}
	// Verify the Bucket has the value persisted (Go-side observation).
	v, ok := bucket.Get("envoy.lua", "k")
	if !ok {
		t.Fatalf("bucket.Get(envoy.lua, k) ok=false; want true")
	}
	if v.GetStringValue() != "hello" {
		t.Fatalf("bucket.Get value = %q; want %q", v.GetStringValue(), "hello")
	}
}

// TestDynamicMetadata_set_get_roundtrip_number verifies set/get
// round-trip for a number-valued metadata entry.
func TestDynamicMetadata_set_get_roundtrip_number(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		rh:streamInfo():dynamicMetadata():set("envoy.lua", "n", 42)
		result = rh:streamInfo():dynamicMetadata():get("envoy.lua", "n")
	`)
	if got := getGlobalInt(t, vm, "result"); got != 42 {
		t.Fatalf("round-trip number = %d; want 42", got)
	}
}

// TestDynamicMetadata_set_get_roundtrip_bool verifies set/get
// round-trip for a boolean-valued metadata entry.
func TestDynamicMetadata_set_get_roundtrip_bool(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		rh:streamInfo():dynamicMetadata():set("envoy.lua", "b", true)
		result = rh:streamInfo():dynamicMetadata():get("envoy.lua", "b")
	`)
	v := vm.State().GetGlobal("result")
	if v != lua.LTrue {
		t.Fatalf("round-trip bool = %v; want true", v)
	}
}

// TestDynamicMetadata_set_get_roundtrip_list verifies set/get round-trip
// for a list-valued metadata entry (Lua array table → structpb.ListValue).
func TestDynamicMetadata_set_get_roundtrip_list(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		rh:streamInfo():dynamicMetadata():set("envoy.lua", "l", {"a", "b", "c"})
		local r = rh:streamInfo():dynamicMetadata():get("envoy.lua", "l")
		result_type = type(r)
		result_len = #r
		result_a = r[1]
		result_b = r[2]
		result_c = r[3]
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "table" {
		t.Fatalf("round-trip list type = %q; want %q", got, "table")
	}
	if n := getGlobalInt(t, vm, "result_len"); n != 3 {
		t.Fatalf("round-trip list len = %d; want 3", n)
	}
	if got := getGlobalString(t, vm, "result_a"); got != "a" {
		t.Errorf("list[1] = %q; want %q", got, "a")
	}
	if got := getGlobalString(t, vm, "result_b"); got != "b" {
		t.Errorf("list[2] = %q; want %q", got, "b")
	}
	if got := getGlobalString(t, vm, "result_c"); got != "c" {
		t.Errorf("list[3] = %q; want %q", got, "c")
	}
}

// TestDynamicMetadata_set_get_roundtrip_struct verifies set/get round-trip
// for a struct-valued metadata entry (Lua hash table → structpb.Struct).
func TestDynamicMetadata_set_get_roundtrip_struct(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		rh:streamInfo():dynamicMetadata():set("envoy.lua", "s", {name = "alice", age = 30})
		local r = rh:streamInfo():dynamicMetadata():get("envoy.lua", "s")
		result_type = type(r)
		result_name = r.name
		result_age = r.age
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "table" {
		t.Fatalf("round-trip struct type = %q; want %q", got, "table")
	}
	if got := getGlobalString(t, vm, "result_name"); got != "alice" {
		t.Errorf("struct.name = %q; want %q", got, "alice")
	}
	if got := getGlobalInt(t, vm, "result_age"); got != 30 {
		t.Errorf("struct.age = %d; want 30", got)
	}
}

// TestDynamicMetadata_get_returns_nil_for_absent_key verifies :get
// returns lua.LNil when the (filterName, key) coordinate is not present
// in the bucket.
func TestDynamicMetadata_get_returns_nil_for_absent_key(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `result = rh:streamInfo():dynamicMetadata():get("envoy.lua", "nope")`)
	if !isGlobalNil(vm, "result") {
		t.Fatalf("get(absent) = %v; want nil", vm.State().GetGlobal("result"))
	}
}

// TestDynamicMetadata_cross_filter_key_independence verifies that the
// SAME key under DIFFERENT filterNames is fully independent — set under
// one filterName does not leak into another's namespace.
func TestDynamicMetadata_cross_filter_key_independence(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		local dm = rh:streamInfo():dynamicMetadata()
		dm:set("filter.a", "shared_key", "value_a")
		dm:set("filter.b", "shared_key", "value_b")
		result_a = dm:get("filter.a", "shared_key")
		result_b = dm:get("filter.b", "shared_key")
	`)
	if got := getGlobalString(t, vm, "result_a"); got != "value_a" {
		t.Errorf("filter.a:shared_key = %q; want value_a", got)
	}
	if got := getGlobalString(t, vm, "result_b"); got != "value_b" {
		t.Errorf("filter.b:shared_key = %q; want value_b", got)
	}
}

// TestDynamicTypedMetadata_returns_typed_value verifies that
// `:dynamicTypedMetadata(filterName)` returns a Lua table containing all
// the entries for that filterName, marshaled from structpb.Value via the
// same structpbToLua helpers used by :get(). Empty filterName returns
// nil (or an empty table per typed-iteration semantic).
func TestDynamicTypedMetadata_returns_typed_value(t *testing.T) {
	bucket := dynamicmetadata.NewBucket()
	bucket.Set("envoy.lua", "k1", structpb.NewStringValue("v1"))
	bucket.Set("envoy.lua", "k2", structpb.NewNumberValue(123))
	vm := newBridgedVMWithBucket(t, bucket)
	runScript(t, vm, `
		local t = rh:streamInfo():dynamicTypedMetadata("envoy.lua")
		result_type = type(t)
		result_k1 = t.k1
		result_k2 = t.k2
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "table" {
		t.Fatalf("typed-metadata type = %q; want %q", got, "table")
	}
	if got := getGlobalString(t, vm, "result_k1"); got != "v1" {
		t.Errorf("typed.k1 = %q; want v1", got)
	}
	if got := getGlobalInt(t, vm, "result_k2"); got != 123 {
		t.Errorf("typed.k2 = %d; want 123", got)
	}
}

// TestDynamicTypedMetadata_absent_filtername_returns_nil verifies that
// requesting a filterName not present in the bucket returns nil (or an
// empty table — pin the actual semantic).
func TestDynamicTypedMetadata_absent_filtername_returns_nil(t *testing.T) {
	vm := newBridgedVMWithBucket(t, dynamicmetadata.NewBucket())
	runScript(t, vm, `result = rh:streamInfo():dynamicTypedMetadata("envoy.absent")`)
	if !isGlobalNil(vm, "result") {
		t.Fatalf("dynamicTypedMetadata(absent) = %v; want nil",
			vm.State().GetGlobal("result"))
	}
}

// ---------------------------------------------------------------------
// nil-bucket tolerance per ADR-0085
// ---------------------------------------------------------------------

// TestDynamicMetadata_nil_bucket_tolerance verifies the bridge surface
// works even when cb.DynamicMetadata() returns nil — script still works
// (returns nil for any get; no panic on set; consistent with ADR-0085
// nil-receiver-tolerant Bucket).
func TestDynamicMetadata_nil_bucket_tolerance(t *testing.T) {
	// Pass nil bucket so cb.DynamicMetadata() returns nil.
	vm := newBridgedVMWithBucket(t, nil)
	// :get should return nil; :set should no-op silently; the surface
	// must not panic.
	runScript(t, vm, `
		local dm = rh:streamInfo():dynamicMetadata()
		dm_is_userdata = (type(dm) == "userdata")
		dm:set("envoy.lua", "k", "v")
		result_get = dm:get("envoy.lua", "k")
	`)
	if got := vm.State().GetGlobal("dm_is_userdata"); got != lua.LTrue {
		t.Errorf("dynamicMetadata() userdata? = %v; want true", got)
	}
	if !isGlobalNil(vm, "result_get") {
		t.Errorf(":get on nil-bucket = %v; want nil (per ADR-0085)",
			vm.State().GetGlobal("result_get"))
	}
}

// TestDynamicTypedMetadata_nil_bucket_tolerance verifies the typed-
// metadata accessor also tolerates a nil Bucket and returns nil.
func TestDynamicTypedMetadata_nil_bucket_tolerance(t *testing.T) {
	vm := newBridgedVMWithBucket(t, nil)
	runScript(t, vm, `result = rh:streamInfo():dynamicTypedMetadata("envoy.lua")`)
	if !isGlobalNil(vm, "result") {
		t.Fatalf("dynamicTypedMetadata on nil-bucket = %v; want nil",
			vm.State().GetGlobal("result"))
	}
}

// ---------------------------------------------------------------------
// structpb.Value ↔ Lua-value marshaling (table-driven)
// ---------------------------------------------------------------------

// TestStructpbToLua_table_driven covers the structpbToLua helper for
// null/bool/number/string/list/struct values — verifies each shape
// marshals to the correct gopher-lua type.
func TestStructpbToLua_table_driven(t *testing.T) {
	cases := []struct {
		name string
		in   *structpb.Value
		want lua.LValueType
	}{
		{"nil", nil, lua.LTNil},
		{"null", structpb.NewNullValue(), lua.LTNil},
		{"true", structpb.NewBoolValue(true), lua.LTBool},
		{"false", structpb.NewBoolValue(false), lua.LTBool},
		{"number", structpb.NewNumberValue(3.14), lua.LTNumber},
		{"string", structpb.NewStringValue("hello"), lua.LTString},
		{"list", func() *structpb.Value {
			lv, _ := structpb.NewList([]interface{}{"a", "b"})
			return structpb.NewListValue(lv)
		}(), lua.LTTable},
		{"struct", func() *structpb.Value {
			sv, _ := structpb.NewStruct(map[string]interface{}{"k": "v"})
			return structpb.NewStructValue(sv)
		}(), lua.LTTable},
	}
	L := lua.NewState()
	defer L.Close()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := structpbToLua(L, c.in)
			if got.Type() != c.want {
				t.Errorf("structpbToLua(%v).Type() = %v; want %v",
					c.in, got.Type(), c.want)
			}
		})
	}
}

// TestStructpbToLua_values verifies value-equality (not just type) for
// representative cases.
func TestStructpbToLua_values(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	if got := structpbToLua(L, structpb.NewStringValue("abc")); got != lua.LString("abc") {
		t.Errorf("string value = %v; want abc", got)
	}
	if got := structpbToLua(L, structpb.NewNumberValue(42)); got != lua.LNumber(42) {
		t.Errorf("number value = %v; want 42", got)
	}
	if got := structpbToLua(L, structpb.NewBoolValue(true)); got != lua.LTrue {
		t.Errorf("bool true = %v; want LTrue", got)
	}
}

// TestLuaToStructpb_table_driven covers the luaToStructpb helper for
// nil/bool/number/string/list/struct Lua values.
func TestLuaToStructpb_table_driven(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// LNil → null
	if got := luaToStructpb(L, lua.LNil); got == nil || got.GetNullValue() != 0 {
		t.Errorf("LNil → %v; want NullValue (0)", got)
	}
	// LBool(true) → true
	if got := luaToStructpb(L, lua.LTrue); got == nil || !got.GetBoolValue() {
		t.Errorf("LTrue → %v; want BoolValue(true)", got)
	}
	// LBool(false) → false
	if got := luaToStructpb(L, lua.LFalse); got == nil || got.GetBoolValue() {
		t.Errorf("LFalse → %v; want BoolValue(false)", got)
	}
	// LNumber → number
	if got := luaToStructpb(L, lua.LNumber(42.5)); got == nil || got.GetNumberValue() != 42.5 {
		t.Errorf("LNumber(42.5) → %v; want NumberValue(42.5)", got)
	}
	// LString → string
	if got := luaToStructpb(L, lua.LString("xyz")); got == nil || got.GetStringValue() != "xyz" {
		t.Errorf("LString(xyz) → %v; want StringValue(xyz)", got)
	}

	// Lua array table → list
	tbl := L.NewTable()
	tbl.Append(lua.LString("a"))
	tbl.Append(lua.LString("b"))
	got := luaToStructpb(L, tbl)
	lv := got.GetListValue()
	if lv == nil {
		t.Fatalf("array table → %v; want ListValue", got)
	}
	if n := len(lv.Values); n != 2 {
		t.Fatalf("ListValue len = %d; want 2", n)
	}
	if v := lv.Values[0].GetStringValue(); v != "a" {
		t.Errorf("list[0] = %q; want a", v)
	}

	// Lua hash table → struct
	tbl2 := L.NewTable()
	tbl2.RawSetString("name", lua.LString("alice"))
	tbl2.RawSetString("age", lua.LNumber(30))
	got2 := luaToStructpb(L, tbl2)
	sv := got2.GetStructValue()
	if sv == nil {
		t.Fatalf("hash table → %v; want StructValue", got2)
	}
	if v := sv.Fields["name"].GetStringValue(); v != "alice" {
		t.Errorf("struct.name = %q; want alice", v)
	}
	if v := sv.Fields["age"].GetNumberValue(); v != 30 {
		t.Errorf("struct.age = %v; want 30", v)
	}
}

// TestLuaToStructpb_roundtrip_via_marshaling verifies that
// luaToStructpb → structpbToLua is the identity (modulo numeric type
// coercion to float64) for the values the bridge marshals across.
func TestLuaToStructpb_roundtrip_via_marshaling(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cases := []lua.LValue{
		lua.LString("hello"),
		lua.LNumber(7),
		lua.LTrue,
		lua.LFalse,
	}
	for _, in := range cases {
		spv := luaToStructpb(L, in)
		out := structpbToLua(L, spv)
		if out.Type() != in.Type() {
			t.Errorf("round-trip type drift: in=%v out=%v", in.Type(), out.Type())
		}
		if out.String() != in.String() {
			t.Errorf("round-trip value drift: in=%v out=%v", in, out)
		}
	}
}
