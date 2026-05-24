# Phase 25.2 — `http-filter-wasm-body-and-advanced-bridge`

**Status:** planned (lifecycle-state 0; pre-created at parent BRAINSTORM per phase-22 precedent). This sub-phase directory was pre-created at the phase-25 parent BRAINSTORM session (commit landing this file). No SPEC / PLAN / PROGRESS / REVIEW artifact exists yet — they land when this sub-phase enters lifecycle.

**Parent:** [`../25-http-filter-wasm/BRAINSTORM.md`](../25-http-filter-wasm/BRAINSTORM.md) — read first; this README is a forward-pointer.

**Scope at 25.2** (per parent BRAINSTORM §1.1 sub-phase 25.2): the full advanced-bridge delta on top of 25.1's headers-only foundation — body + buffer hostcalls (proxy_get/set_buffer_bytes for HttpRequestBody + HttpResponseBody + HttpCallResponseBody buffer types) + trailers (proxy_on_request_trailers / proxy_on_response_trailers + trailer hostcalls) + timer dispatch (proxy_set_tick_period_milliseconds + proxy_on_tick) + metrics (proxy_define_metric / proxy_increment_metric / proxy_record_metric / proxy_get_metric — plugin-defined dynamic stats in `wasm.<vm_id>.<custom_name>` namespace) + shared data (proxy_set/get_shared_data — cross-stream + cross-plugin within the same vm_id) + outbound HTTP dispatch (proxy_dispatch_http_call / proxy_on_http_call_response — reuses phase-20 `internal/httpclient/` per ADR-0177 third-or-later co-consumer) + foreign-function call (proxy_call_foreign_function — default-denied at envoy-go-strict sandbox; SPEC settles registration interface OR PARSE-REJECT entirely) + full stream-info surface (proxy_get_property for upstreamHost / upstreamCluster / downstreamSslConnection / requestedServerName / filterState) + additional envoy-go-strict counters (`tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied`) + differential fixture `0036-http-wasm-body-and-advanced` (mixed-mode per phase-22.2 ADR-0192 precedent for non-deterministic legs) + boot-reject fixture `0037-http-wasm-body-and-advanced-boot-reject` + 35th project-wide fuzzer `FuzzWasmHostcallEnvelope`. PARSE-REJECT still active on per-route TPFC + multi-plugin VM sharing (deferred to 25.3).

**Anticipated artifacts at 25.2 IMPL phase-done:**
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/SPEC.md` + PLAN.md + PROGRESS.md + REVIEW.md (lifecycle states 1 → 5 → 6)
- `internal/wasm/` 25.2 ABI extensions (body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function default-deny) + `internal/filter/http/wasm/` 25.2 package extensions
- `test/fixtures/0036-http-wasm-body-and-advanced/` + `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/`
- Additional vendored Rust-sourced bytecode under `test/fixtures/wasm-plugins/` (body_*.wasm, tick_*.wasm, http_call_*.wasm, shared_data_*.wasm, define_metric_*.wasm)
- ADR landings: ADR-0206 (25.2 ABI extensions) + ADR-0207 (filter package extensions + mixed-mode fixture discipline) + escape-valve ADRs (ADR-0208 + ADR-0209 if 2-slot buffer fires)
- BEHAVIOR_CONTRACT.md 25.2 completion bundle per ADR-0052 atomic landing

**Predecessor:** Phase 25.1 DONE.

**Successor:** Phase 25.3 (`25.3-http-filter-wasm-perroute-and-conformance`) — depends-on 25.2.
