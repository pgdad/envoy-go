package proxywasm

// Batch-A conformance families (phase-25.3 Task 14): logging, stop_iteration,
// shared_data, pairs_util, endianness. Each family builds a vendored guest
// .wasm (sources/<family>/), drives it through the proxy-wasm lifecycle on an
// in-process RootVM, and asserts host-observable behavior. Every family is
// proven LIVE via the deliberate-break cycle recorded in PROGRESS.md.
//
// Registration happens in init() (test-binary scope) so the production-side
// conformanceFamilies global in conformance.go stays an empty literal — the
// families attach only when the _test.go file compiles into the test binary.

import (
	"bytes"
	"context"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

//nolint:gochecknoinits // test-scope family registration; keeps the production global empty
func init() {
	conformanceFamilies = append(conformanceFamilies,
		conformanceFamily{name: "logging", run: runLogging},
		conformanceFamily{name: "stop_iteration", run: runStopIteration},
		conformanceFamily{name: "shared_data", run: runSharedData},
		conformanceFamily{name: "pairs_util", run: runPairsUtil},
		conformanceFamily{name: "endianness", run: runEndianness},
	)
}

// ─── logging ────────────────────────────────────────────────────────────────
//
// Guest: on_request_headers calls hostcalls::log at all five severities with
// distinctive messages. Host-observable: the recording ABICallbacks Log
// forwarder lands each message (with its level tag) in the captured-log
// buffer; assertLogContains verifies each.
func runLogging(t *testing.T) {
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "logging"))
	ctx := context.Background()

	sc, err := cvm.RootVM.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}
	if _, err := sc.CallProxyOnRequestHeaders(ctx, 0, true); err != nil {
		t.Fatalf("CallProxyOnRequestHeaders: %v", err)
	}

	assertLogContains(t, cvm.Logs, "[wasm trace] conformance-logging trace-msg")
	assertLogContains(t, cvm.Logs, "[wasm debug] conformance-logging debug-msg")
	assertLogContains(t, cvm.Logs, "[wasm info] conformance-logging info-msg")
	assertLogContains(t, cvm.Logs, "[wasm warn] conformance-logging warn-msg")
	assertLogContains(t, cvm.Logs, "[wasm error] conformance-logging error-msg")
}

// ─── stop_iteration ─────────────────────────────────────────────────────────
//
// Two guest variants: one returns Action::Pause, one Action::Continue, from
// proxy_on_request_headers. Host-observable: CallProxyOnRequestHeaders returns
// the corresponding abi.ProxyAction. Proves the Pause/Continue wire distinction
// survives the guest->host return path.
func runStopIteration(t *testing.T) {
	ctx := context.Background()

	t.Run("pause", func(t *testing.T) {
		cvm := newConformanceRootVM(t, loadFamilyWasm(t, "stop_iteration_pause"))
		sc, err := cvm.RootVM.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		action, err := sc.CallProxyOnRequestHeaders(ctx, 0, true)
		if err != nil {
			t.Fatalf("CallProxyOnRequestHeaders: %v", err)
		}
		if action != abi.ProxyActionPause {
			t.Errorf("pause variant: got action %d, want ProxyActionPause (%d)", action, abi.ProxyActionPause)
		}
	})

	t.Run("continue", func(t *testing.T) {
		cvm := newConformanceRootVM(t, loadFamilyWasm(t, "stop_iteration_continue"))
		sc, err := cvm.RootVM.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		action, err := sc.CallProxyOnRequestHeaders(ctx, 0, true)
		if err != nil {
			t.Fatalf("CallProxyOnRequestHeaders: %v", err)
		}
		if action != abi.ProxyActionContinue {
			t.Errorf("continue variant: got action %d, want ProxyActionContinue (%d)", action, abi.ProxyActionContinue)
		}
	})
}

// ─── shared_data ────────────────────────────────────────────────────────────
//
// Guest (on_vm_start, driven by Configure): set(key,"v1",cas=0) -> get ->
// CAS-matched set("v2") -> CAS-mismatched set("v3",cas=1). Host-observable:
// RootVM.GetSharedData reflects the final value "v2" + CAS counter 2 (the
// stale-cas write is rejected and leaves the entry unchanged). Mirrors the
// 25.2 shared-data CAS golden semantics (new entry cas=1, matched update cas=2).
func runSharedData(t *testing.T) {
	// on_vm_start ran during newConformanceRootVM's Configure.
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "shared_data"))

	const key = "conformance-shared-key"
	val, cas, res := cvm.RootVM.GetSharedData(key)
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(%q): result %v, want Ok", key, res)
	}
	if got := string(val); got != "v2" {
		t.Errorf("GetSharedData(%q): value = %q, want %q (stale-cas set-v3 must NOT win)", key, got, "v2")
	}
	if cas != 2 {
		t.Errorf("GetSharedData(%q): cas = %d, want 2 (cas 1 after first write, 2 after matched update)", key, cas)
	}
}

// ─── pairs_util ─────────────────────────────────────────────────────────────
//
// Guest (on_request_headers): reads the full request-header map via get_map
// (proxy_get_header_map_pairs, host-side serialized with the pairs wire format
// — envoy-go EncodePairs), then writes response headers x-pairs-count and
// x-pairs-echo derived from what it decoded. Host-observable: the harness
// seeds a known request-header map + asserts the guest reported the correct
// pair count + echoed the seeded "x-probe" value, proving the pairs
// marshaling round-trips byte-faithfully.
func runPairsUtil(t *testing.T) {
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "pairs_util"))
	ctx := context.Background()

	seed := []wasm.HeaderPair{
		{Key: ":method", Value: "GET"},
		{Key: ":path", Value: "/conformance"},
		{Key: "x-probe", Value: "probe-value-42"},
		{Key: "x-extra", Value: "extra"},
	}
	cvm.ABI.SeedRequestHeaders(seed)

	sc, err := cvm.RootVM.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}
	//nolint:gosec // seed length fits a uint32 by construction
	if _, err := sc.CallProxyOnRequestHeaders(ctx, uint32(len(seed)), true); err != nil {
		t.Fatalf("CallProxyOnRequestHeaders: %v", err)
	}

	count, ok := cvm.ABI.WrittenResponseHeader("x-pairs-count")
	if !ok {
		t.Fatalf("guest did not write x-pairs-count response header")
	}
	if count != "4" {
		t.Errorf("x-pairs-count = %q, want %q (guest must decode all %d seeded pairs)", count, "4", len(seed))
	}

	echo, ok := cvm.ABI.WrittenResponseHeader("x-pairs-echo")
	if !ok {
		t.Fatalf("guest did not write x-pairs-echo response header")
	}
	if echo != "probe-value-42" {
		t.Errorf("x-pairs-echo = %q, want %q (guest must echo the seeded x-probe value through the pairs round-trip)", echo, "probe-value-42")
	}
}

// ─── endianness ─────────────────────────────────────────────────────────────
//
// Guest (on_vm_start): writes 0x01020304u32 and 0x0102030405060708u64 as their
// little-endian byte representation into shared-data. Host-observable:
// RootVM.GetSharedData returns the raw bytes; the harness asserts the exact LE
// byte ordering, proving guest<->host buffer marshaling preserves byte order.
func runEndianness(t *testing.T) {
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "endianness"))

	wantU32 := []byte{0x04, 0x03, 0x02, 0x01}
	wantU64 := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	gotU32, _, res := cvm.RootVM.GetSharedData("conformance-le-u32")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(conformance-le-u32): result %v, want Ok", res)
	}
	if !bytes.Equal(gotU32, wantU32) {
		t.Errorf("u32 LE bytes = % x, want % x", gotU32, wantU32)
	}

	gotU64, _, res := cvm.RootVM.GetSharedData("conformance-le-u64")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(conformance-le-u64): result %v, want Ok", res)
	}
	if !bytes.Equal(gotU64, wantU64) {
		t.Errorf("u64 LE bytes = % x, want % x", gotU64, wantU64)
	}
}
