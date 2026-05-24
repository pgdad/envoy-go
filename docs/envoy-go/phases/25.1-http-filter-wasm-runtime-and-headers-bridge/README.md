# Phase 25.1 — `http-filter-wasm-runtime-and-headers-bridge`

**Status:** planned (lifecycle-state 0; pre-created at parent BRAINSTORM per phase-22 precedent). This sub-phase directory was pre-created at the phase-25 parent BRAINSTORM session (commit landing this file). No SPEC / PLAN / PROGRESS / REVIEW artifact exists yet — they land when this sub-phase enters lifecycle.

**Parent:** [`../25-http-filter-wasm/BRAINSTORM.md`](../25-http-filter-wasm/BRAINSTORM.md) — read first; this README is a forward-pointer.

**Scope at 25.1** (per parent BRAINSTORM §1.1 sub-phase 25.1): the NEW `internal/wasm/` framework primitive (wazero VM lifecycle + per-module compile cache + default-deny `SandboxConfig` + ABI-registration interface + the in-house proxy-wasm v0.2.1 host ABI implementation for the headers-bridge hostcall subset) + the NEW `internal/filter/http/wasm/` package skeleton + `Wasm.config` (PluginConfig) consumed + AsyncDataSource.Local code-source resolution (4-arm DataSource shape; WatchedDirectory PARSE-REJECT; AsyncDataSource.Remote PARSE-REJECT) + the headers-bridge hostcall set (proxy_get/set/add/replace/remove_header_map_pairs + proxy_send_local_response + proxy_log + proxy_get/set_property + proxy_get_status + lifecycle hooks proxy_on_context_create / proxy_on_vm_start / proxy_on_configure / proxy_on_done / proxy_on_delete / proxy_on_log + HTTP hooks proxy_on_request_headers / proxy_on_response_headers) + 7-counter stat surface (5 upstream + 2 envoy-go-strict `executions` + `hostcall_denied`) + cross-side fixture `0034-http-wasm-headers-bridge` + boot-reject fixture `0035-http-wasm-boot-reject` + 34th project-wide fuzzer `FuzzWasmConfigParse` + the BEHAVIOR_CONTRACT.md 25.1-completion bundle.

**Anticipated artifacts at 25.1 IMPL phase-done:**
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/SPEC.md` (lifecycle-state 1 → 2; `superpowers:writing-plans` scoped to SPEC)
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PLAN.md` (lifecycle-state 2 → 3; `superpowers:writing-plans` scoped to PLAN; ADR-0045 split-gate re-verification)
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` (lifecycle-state 3; per-task TDD log)
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/REVIEW.md` (lifecycle-state 5 → 6; `superpowers:requesting-code-review`)
- `internal/wasm/` (NEW package) + `internal/filter/http/wasm/` (NEW package)
- `test/fixtures/0034-http-wasm-headers-bridge/` + `test/fixtures/0035-http-wasm-boot-reject/`
- `test/fixtures/wasm-plugins/headers_*.wasm` (vendored Rust-sourced bytecode) + `test/fixtures/wasm-plugins/headers_*/src/lib.rs` (Rust source + reproduction build script)
- ADR landings: ADR-0202 (framework primitive) + ADR-0203 (filter package + ABI + fixture + stat surface) + ADR-0204 (default-deny sandbox) + escape-valve ADR-0205 if WEAK-HOLD-with-2-slot-buffer fires
- BEHAVIOR_CONTRACT.md 25.1 completion bundle per ADR-0052 atomic landing

**Predecessor:** Phase 24.2 IMPL DONE (squash `94c2b78`; SHA-fill `ec0ee80`; cold-start rewrite `35f4c73`; tip repoint `1ab2014`). ROADMAP row 25.1 depends-on 24 (parent row 24 DONE at the 24.2 ROLLUP per D-RL18 + the 18/19/22 precedent).

**Successor:** Phase 25.2 (`25.2-http-filter-wasm-body-and-advanced-bridge`) — depends-on 25.1.
