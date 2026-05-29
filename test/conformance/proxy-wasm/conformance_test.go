package proxywasm

import "testing"

// TestProxyWasmConformance is the in-process proxy-wasm conformance gate per
// phase-25.3 Task 13 (ADR-0212; D-25.3-P4). It ranges over the
// conformanceFamilies registry, running each family as a t.Run subtest.
//
// At Task 13 the registry is EMPTY → this test PASSES VACUOUSLY (a `go test`
// green with zero subtests). Task 14 appends the 10 ported families (logging,
// stop_iteration, shared_data, endianness, exports, security, runtime,
// wasm_vm, bytecode_util, pairs_util) + vendors their prebuilt `.wasm` blobs
// under bytecode/. Each family MUST be proven live via deliberate-break (flip
// an assertion / corrupt a blob → confirm the subtest FAILS, then revert) per
// README.md. The 6 deferred families (shared_queue, signature_util,
// wasm[TLS-cache], vm_id_handle, null_vm, fuzz) are documented as forward-
// pointers in BOOTSTRAP_PROMPT.md §7.5 (roster lands at Task 15).
func TestProxyWasmConformance(t *testing.T) {
	if len(conformanceFamilies) == 0 {
		// Task 13 scaffold: empty registry. The vacuous pass is intentional —
		// it proves the harness builds + the driver shape is correct before
		// Task 14 fills the families. NOT t.Skip (a skip would hide a future
		// regression where the registry is accidentally emptied).
		t.Log("proxy-wasm conformance registry is empty (Task 13 scaffold); Task 14 fills the 10 families")
		return
	}
	for _, fam := range conformanceFamilies {
		fam := fam
		t.Run(fam.name, fam.run)
	}
}
