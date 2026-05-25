# Phase 25.2 SPEC — `envoy.extensions.filters.http.wasm.v3.Wasm` (full advanced-bridge surface delta on top of 25.1 headers-only foundation)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `25.2` flips `planned → in-progress` at this SPEC commit (parent row `25` STAYS `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `25.1` `done`, `25.3` `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-22.2 + phase-25.1 sub-phase SPEC → PLAN precedent. This SPEC is the authoritative input to the 25.2 PLAN.

**Parent:** `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` (parent master SPEC — §1.1 9-AMEND catalog (A1-A9); §3.1 sub-phase surface-mapping table; §4 framework primitive sketch; §6.2 18-arm 25.1 PARSE-REJECT roster + §6.3 25.2 + 25.3 forward-pointers; §7 5-counter 25.1 stat surface + AMEND-A2 tri-group structure + DROPPED HCM-stats_prefix; §11 9-pin empirical-pin block (D1-D9) resolved IN-SESSION at parent SPEC drafting; §13 8 RATIFIED-PENDING-IMPL items (R1-R8); §13.5 BEHAVIOR_CONTRACT bundle anticipation; §15 24-item acceptance checklist). The 25.2 SPEC INHERITS the 9-AMEND catalog + the parent §3.1 surface-mapping (25.2 column) verbatim; it does NOT re-litigate parent-settled scope.

**Predecessors:**
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/BRAINSTORM.md` (686 lines; 12 sections; 11 Q-decisions settled — Q1 body per-chunk + accumulating buffer; Q2 16 MiB envoy-go-strict body-buffer cap + configurable; Q3 ONE long-lived `*RootVM` per `*compiledConfig`; Q4 BadArgument + envoy-go-strict counter for unknown cluster; Q5 per-root-VM tick goroutine + 10ms envoy-go-strict period floor + Clock seam first co-consumer; Q6 shared-data caps 1 MiB value + 1024-entry envoy-go-strict; Q7 full property surface + EXTRACT-NOW `internal/filterstate/` consumer-#2; Q8 single-listener mixed-mode fixture-0036 + 12-14 scenarios; Q9 8-counter envoy-go-strict tally + dynamic-stats namespace; Q10 strict-scope ADR consumption + 4 NEW + 1 reserve; Q11 stay single sub-phase 25.2).
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/{SPEC,PLAN,PROGRESS,REVIEW}.md` (the predecessor sub-phase's full artifact set — load-bearing precedents for structure + 25.1 IMPL inheritance state).

**Sub-phase scope (per parent SPEC §3.1 25.2 column + BRAINSTORM §1.1):** Phase 25.2 lands the FULL Envoy↔WASM advanced-bridge surface delta on top of 25.1's headers-only foundation — every upstream-parity hostcall outside the per-route TPFC + multi-plugin VM-sharing + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` surfaces (which defer to 25.3) is active, plus ~6 envoy-go-strict departure records (body-buffer cap + shared-data caps + tick period floor + foreign-function 0-vs-10 default registry + dynamic-stats cap + 5-counter envoy-go-strict counter bundle) + the foreign-function registration interface with EMPTY default registry per AMEND-A9.

Specifically:
- **Body bridge** CONSUMED — per-chunk-invoke + accumulating buffer (Q1). Activates `WasmBufferType` values 0 (HttpRequestBody) + 1 (HttpResponseBody) + 4 (HttpCallResponseBody) which were defined-but-unused at 25.1.
- **Buffer hostcalls** CONSUMED — `proxy_get_buffer_bytes` + `proxy_set_buffer_bytes` + `proxy_get_buffer_status` with the **clamp-on-overflow** semantic per AMEND-B1 (REFINES BRAINSTORM Q1).
- **Trailer hostcalls + callbacks** CONSUMED — `proxy_on_request_trailers` + `proxy_on_response_trailers` callbacks invoked when trailers arrive; trailer-map accessors REUSE the 25.1 7-method header-map family with `WasmHeaderMapType` values 1 (HttpRequestTrailers) + 3 (HttpResponseTrailers) activated.
- **Timer dispatch** CONSUMED — `proxy_set_tick_period_milliseconds` + `proxy_on_tick` callback via per-root-VM goroutine + 10ms envoy-go-strict period floor (Q5); FIRST co-consumer of phase-21 ADR-0186 `Clock` seam beyond phase-21 itself (RATIFIES the extraction).
- **Metric hostcalls** CONSUMED — `proxy_define_metric` + `proxy_increment_metric` (signed `int64` delta per AMEND-B2) + `proxy_record_metric` (unsigned `uint64` value per AMEND-B2) + `proxy_get_metric` + plugin-defined dynamic-stats `wasmcustom.<custom_name>` namespace per AMEND-B2 (REFINES BRAINSTORM Q9 prefix shape — no per-plugin prefix; per-plugin isolation via per-plugin Registry scope) via NEW `internal/stats/dynamic.go` (Q9).
- **Shared-data hostcalls** CONSUMED — `proxy_set_shared_data` + `proxy_get_shared_data` with CAS atomic-compare-and-set + envoy-go-strict caps (1 MiB value + 1024 entries; 2 envoy-go-strict-only config fields) per Q6.
- **Outbound HTTP dispatch** CONSUMED — `proxy_http_call` (10-arg hostcall) + `proxy_on_http_call_response` callback. RE-CONSUMES phase-20 `internal/httpclient/` per ADR-0177 at third-or-later co-consumer (phase-22.2's `:httpCall()` was second). NO API extension on httpclient (phase-22.2 already added cluster-based dispatch). RATIFIES the phase-20 framework-primitive extraction; CLOSES parent SPEC §13-R6 anchor.
- **httpCall unknown-cluster** CONSUMED — runtime `WasmResult::BadArgument` (=2) + integration error log + envoy-go-strict counter `wasm.<plugin>.http_call_dispatch_unknown_cluster` per Q4. Late-response-after-stream-closed disposition pinned at AMEND-B3 (cancel-at-destruction + defensive token-miss guard; NOT silent drop as BRAINSTORM hypothesized; envoy-go-strict counter `wasm.<plugin>.http_call_response_after_close` ADDED per AMEND-B3 recommendation).
- **Foreign-function call** CONSUMED — `proxy_call_foreign_function` + `proxy_on_foreign_function` callback. envoy-go ships NEW `internal/wasm/foreign.go` `ForeignFunctionRegistry` (Register/Get) with EMPTY default registry per AMEND-A9; unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream; capability-gated via default-deny `proxy_call_foreign_function` capability key per AMEND-A5. envoy-go-strict departure record (upstream registers 10 by default; envoy-go ZERO).
- **Full stream-info property surface** CONSUMED — `proxy_get_property` extended from 25.1's 5-path minimal tree to the FULL upstream Envoy CEL property tree per AMEND-B4 (REFINES BRAINSTORM Q7 root count): **~10 dispatched roots + 4 direct tokens; ~70 documented sub-paths** (request 16 + response 6 + connection 12+id + source 2 + destination 2 + upstream ~14 + xds 12 + metadata + filter_state + upstream_filter_state + wasm-proxy + direct tokens plugin_name + plugin_root_id + plugin_vm_id + connection_id). NUL-delimited wire format CONFIRMED. Three RE-CONSUMES (phase-04 ADR-0144 + phase-20 ADR-0177 + phase-22.2 ADR-0190) + ONE NEW primitive extraction (`internal/filterstate/` per Q7 + ADR-0207).
- **NEW `internal/filterstate/` framework primitive** EXTRACTED at consumer #2 scope per Q7 + ADR-0207. Consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking under the same ADR (the `:filterState()` Lua surface stays unchanged; only the underlying storage layer flips from in-package map to shared primitive). Mirrors the discipline ADR-0188 established for `internal/lua/` at phase-22.1.
- **NEW `internal/stats/dynamic.go` infrastructure** per Q9 — thin wrapper over `internal/stats/` registry for the `wasmcustom.<custom_name>` dynamic-stats namespace; per-plugin Registry instance scoping; envoy-go-strict 1024-entry cap + `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict counter + envoy-go-strict-only `envoy_go_strict_dynamic_stats_max_entries` config field.
- **Root VM lifecycle evolution** per Q3 + ADR-0205 — ONE long-lived `*RootVM` per `*compiledConfig`; per-stream contexts as CHILDREN sharing wazero Runtime+Module. 25.1's per-stream `*wazero.Runtime` construction model is RETIRED. The per-stream Module instantiation pattern (fresh vs pooled vs shared) carries forward to 25.2 IMPL R8 escape-valve.
- **8 NEW envoy-go-strict counters** at 25.2 per Q9 (`tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied` + `body_buffer_cap_exceeded` + `http_call_dispatch_unknown_cluster` + `shared_data_cap_exceeded` + `dynamic_stats_cap_exceeded`) + 1 ADDITIONAL counter per AMEND-B3 recommendation (`http_call_response_after_close`) = **9 envoy-go-strict counters at 25.2**. Project stat count **119 → 128** at 25.2 phase-done.
- **4 envoy-go-strict-only `PluginConfig` config fields** (Qs 2/6/9): `envoy_go_strict_body_buffer_cap_bytes` (default 16 MiB) + `envoy_go_strict_shared_data_value_cap_bytes` (default 1 MiB) + `envoy_go_strict_shared_data_max_entries` (default 1024) + `envoy_go_strict_dynamic_stats_max_entries` (default 1024). 10ms tick period floor is a compile-time constant per Q5 (NOT a config field).
- **35th project-wide fuzzer `FuzzWasmHostcallEnvelope`** — ~30-40 corpus seeds covering hostcall envelope edge cases + must-never-panic invariant.
- **Differential fixture `0036-http-wasm-body-and-advanced`** — single-listener single-HCM mixed-mode per Q8 + phase-22.2 ADR-0192 precedent; 12-14 scenarios partitioned by assertion-class (8-10 deterministic cross-side via `CompareBytes` + 3-4 non-deterministic subject-only via `StatsAsserter.AssertStats`). httpCall scenarios use a SECOND upstream cluster definition (NOT a second listener — avoids `freeTCPPort` flake per phase-22.2 REVIEW §7.4).
- **Differential fixture `0037-http-wasm-body-and-advanced-boot-reject`** — single-arm PGV-mirror reject (final arm selection deferred to IMPL via empirical-scrape per D-25.2-P1; anticipated `envoy_go_strict_body_buffer_cap_bytes = 0` envoy-go-strict-only validator).
- **18-arm 25.1 PARSE-REJECT roster EXTENDED** with ~8-12 NEW arms at 25.2 (timer-period-required; metric-name-required; httpCall-cluster-required; foreign-function-name-required; envoy-go-strict-only config field validators for body-buffer cap / shared-data caps / dynamic-stats cap). Final arm count post-25.2: ~26-30.

**25.3 (per-route 5th-canonical wholesale-override + multi-plugin VM-sharing via duplicate `vm_id` + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` + `fail_open` deprecated field + `test/conformance/proxy-wasm/` harness seed at 62.5% pass-threshold) is OUT OF SCOPE for 25.2.**

**ADR continuity:** Phase 25.1 IMPL closed at ADR-0204 §Decision + §Consequences body lands. **At THIS 25.2 SPEC commit: 4 NEW ADR §Context drafts anchor** (ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208) per ADR-0044 §Context-draft discipline + 1 reserve slot (ADR-0209) per Q10 STRENGTHENED-WEAK-HOLD-with-1-slot disposition. §Decision + §Consequences bodies LAND at 25.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. **In-place AMENDMENT acknowledgment paragraph on ADR-0202** also lands at 25.2 IMPL (no new ADR number consumed; matches phase-22.2's strict-scope Q10 precedent — the consumer-#1-internal-scope API evolution at 25.2 is absorbed under NEW ADRs rather than amending ADR-0202's §Decision body). **Conditional ADR-0209** (escape-valve reserve) — anchors AT 25.2 IMPL only if a per-stream Module instantiation R8 escape-valve trigger fires OR a SPEC-time-unanticipated surface fires at IMPL. **Next-free ADR after THIS 25.2 SPEC commit: `ADR-0209`** (4 numbers consumed for §Context drafts: ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208). Anticipated next-free after 25.2 phase-done: **`ADR-0210`** if reserve UNCONSUMED; **`ADR-0211`** if reserve consumed (parent §10.3 25.3 anticipated ADRs would then start at +1).

**Authored:** 2026-05-25.

**Base commit:** `b4720ab` (master tip at session entry — `next-prompt.txt: repoint master-tip references to 404a16e (actual HEAD)`; docs-only repoint above the 25.2 BRAINSTORM SHA-fill `e800769` above the 25.2 BRAINSTORM squash `0589f85`). Predecessors: `e800769` (25.2 BRAINSTORM stage-close SHA-fill) + `0589f85` (25.2 BRAINSTORM squash-merge) + `3f5a448` (next-prompt.txt repoint for 25.2 BRAINSTORM cold-start) + `7d4fa33` (next-prompt.txt rewrite for 25.2 BRAINSTORM) + `de4f853` (25.1 IMPL SHA-fill) + `feded64` (25.1 IMPL squash) + `2c1455d` (parent 25 SPEC squash).

---

## 1. Purpose / Mission

Phase 25.2 lands the FULL Envoy↔WASM advanced-bridge surface delta on top of 25.1's headers-only foundation, taking parent BRAINSTORM Q1 envelope D to its conclusion. By 25.2 phase-done every upstream-parity hostcall outside the per-route TPFC + multi-plugin VM-sharing + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` surfaces (which defer to 25.3) is ACTIVE, plus ~6 envoy-go-strict departure records (body buffer cap + shared-data caps + tick period floor + foreign-function 0-vs-10 default registry + dynamic-stats cap + 9-counter envoy-go-strict counter bundle) + the foreign-function registration interface with EMPTY default registry per AMEND-A9.

The eight architectural primitives landing at 25.2:

1. **Root VM lifecycle evolution per Q3 + ADR-0205** — ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per proxy-wasm-cpp-host's `Wasm`/`Plugin` model). Constructed at config-load via `proxy_on_vm_start` + `proxy_on_configure` (fire once at root context). Persists for plugin lifetime. Per-stream contexts as CHILDREN of the root VM sharing the same wazero Runtime + Module: each `DecodeHeaders` creates a child stream-context ID via `proxy_on_context_create(stream_ctx_id, root_ctx_id)`; `OnDestroy` fires `proxy_on_done(stream_ctx_id) → bool` + `proxy_on_delete(stream_ctx_id)`. The root VM owns: tick goroutine + tick state; shared-data map; httpCall response routing + call_id allocation; foreign-function registry view. The 25.1 per-stream `*VM` (each stream constructing a fresh `*wazero.Runtime` at 61µs/stream per the R8 benchmark) is RETIRED at 25.2; per-stream `*StreamContext` creation becomes ~microseconds (just calling `proxy_on_context_create` + bookkeeping; no wazero Runtime construction). The per-stream Module instantiation pattern (fresh vs pooled vs shared) carries forward to 25.2 IMPL R8 escape-valve per parent §13-R8 threshold (> 1ms per-stream cost triggers ADR-0209 reserve consumption).

2. **25.2 ABI surface extension per ADR-0206** — ~14 NEW env-namespace hostcalls (3 body/buffer + 2 stream-control + 1 timer + 4 metrics + 2 shared-data + 1 outbound HTTP + 1 foreign-function — see §5 for the full enumeration) + ~7 NEW guest-export callbacks (`proxy_on_request_body` + `proxy_on_response_body` + `proxy_on_request_trailers` + `proxy_on_response_trailers` + `proxy_on_tick` + `proxy_on_http_call_response` + `proxy_on_foreign_function`). EXTENDS `internal/wasm/registration.go` `ABICallbacks` interface with the new methods. EXTENDS `internal/wasm/sandbox.go` capability key roster with **21 NEW capability keys** per AMEND-B5 (14 hostcall + 7 lifecycle; per-key gating at `registerCallback` time per AMEND-B5 structural refinement). NEW production files: `internal/wasm/tick.go` (per-root-VM tick goroutine + 10ms period floor) + `internal/wasm/shared_data.go` (per-RootVM CAS-protected K-V map + 1 MiB value cap + 1024-entry cap) + `internal/wasm/property.go` (full proxy-wasm property-path tree mapping per AMEND-B4) + `internal/wasm/dynamic_stats.go` (proxy_define_metric dispatch with signed-i64 delta per AMEND-B2 + 1024-entry cap) + `internal/wasm/foreign.go` (ForeignFunctionRegistry with EMPTY default registry per AMEND-A9) + `internal/wasm/root_vm.go` (the root-VM lifecycle anchor + httpCall routing + tick state + shared-data state). EXTENDS `internal/wasm/registration.go` host-module wiring to register 38 hostcalls active at 25.2 (24 from 25.1 + 14 NEW; see §5.5).

3. **Buffer-bounds clamp semantic per AMEND-B1** — `proxy_get_buffer_bytes` clamps silently (returns `WasmResult::Ok` with truncated length) when `start + max_size > buffer.length` rather than returning `BAD_ARGUMENT` as the spec README text states. envoy-go MUST mirror cpp-host clamp behavior (real wasm guests rely on it; Istio/Envoy production guests expect clamp). The clamp wire-contract pins at §11.1 + lands at `internal/wasm/registration.go` (or the proxy-wasm-cpp-host-equivalent dispatcher).

4. **NEW `internal/filterstate/` framework primitive per Q7 + ADR-0207** — per-stream `*Bucket` accessor + `FilterStateObject` interface (Set/Get/Marshal/HasData/StateType) + sync semantics matching phase-22.2's in-package implementation. Consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking under the same ADR (the `:filterState()` Lua surface stays UNCHANGED; only the underlying storage layer flips from in-package map to shared primitive — ~50-100 LoC migration delta inside `internal/filter/http/lua/`). Consumer #2 = 25.2 wasm `proxy_get_property "filter_state.*"` + `"upstream_filter_state.*"` paths per AMEND-B4 (upstream_filter_state is DISTINCT root co-equal to filter_state — REFINES BRAINSTORM Q7). EXTRACT-NOW-on-second-consumer trigger fires per the discipline ADR-0188 established at phase-22.1 for `internal/lua/`.

5. **NEW `internal/stats/dynamic.go` infrastructure per Q9 + ADR-0208** — thin wrapper over `internal/stats/` registry for the `wasmcustom.<custom_name>` dynamic-stats namespace per AMEND-B2 (REFINES BRAINSTORM Q9 prefix shape — upstream Envoy v1.37.2 uses `wasmcustom.<custom_name>` only, NO plugin prefix; per-plugin isolation via per-plugin Registry scope). Per-plugin `*Registry` instance constructed at config-load + plumbed through to the per-VM metric-define dispatch path. Lazy-enumerated at admin `/stats` endpoint. envoy-go-strict 1024-entry cap on dynamic namespace + `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict counter + envoy-go-strict-only `envoy_go_strict_dynamic_stats_max_entries` config field.

6. **`internal/filter/http/wasm/` 25.2 package extensions per ADR-0208** — extends 25.1's package files: `compiled_config.go` gains the 4 envoy-go-strict-only `PluginConfig` config fields + ~8-12 NEW PARSE-REJECT arms; `abi_callbacks.go` extends `wasm.ABICallbacks` impl with the body/trailer/tick/httpCall/foreign-function/property-surface methods (consumes 4 RE-USE primitives — ADR-0144 DownstreamPrincipal + ADR-0177 httpclient + ADR-0190 dynamicmetadata + NEW ADR-0207 filterstate); `decode_headers.go` + `encode_headers.go` extend the dispatch lifecycle to honor `OnDecodeBuffer` + `OnEncodeBuffer` chunked dispatch + body-buffer cap enforcement; NEW `body.go` (body-buffer accumulation per-stream + cap enforcement + 413-on-exceed); NEW `trailers.go` (`DecodeTrailers` + `EncodeTrailers` glue); NEW `tick_clock.go` (Clock-seam injection plumbing per Q5 + ADR-0186 first co-consumer); NEW `stats.go` extensions for 9 envoy-go-strict counters per Q9 + AMEND-B3 + per-plugin `internal/stats/dynamic` Registry plumbing.

7. **`internal/wasm/` API evolution per Q10 strict-scope** — ADR-0202 gains a one-line in-place AMEND acknowledgment paragraph in §Consequences (consumer-#1-internal evolution at 25.2 absorbed under ADR-0205+0206+0207+0208; ADR-0202's API-REVISION ALLOWANCE clause STAYS scoped to consumer #2 = WASM host family). NO new ADR number consumed for the in-place AMEND. Mirrors phase-22.2 Q10 strict-scope precedent (the analogous decision for `internal/lua/` evolution at 22.2 — ADR-0188 stayed scoped to consumer-#2 future revisions while ADR-0190+0191+0192 anchored the 22.2 consumer-#1-internal evolutions).

8. **Differential fixture `0036-http-wasm-body-and-advanced` (mixed-mode) + fixture `0037-http-wasm-body-and-advanced-boot-reject`** per Q8 + ADR-0208. Single-listener single-HCM hosting the wasm filter + router terminator; httpCall scenarios use a SECOND upstream cluster definition (NOT a second listener — avoids `freeTCPPort` flake per phase-22.2 REVIEW §7.4); 12-14 scenarios partitioned by assertion-class (8-10 deterministic cross-side via `CompareBytes` + 3-4 non-deterministic subject-only via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`); every subject-side StatsAsserter arm gets a deliberate-break liveness verification (NOT dead-vacuous per phase-23 fixture-0030 lesson + 25.1 Task 15+17 follow-up). 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` lands per §8.4.

After phase 25.2, the project has the FULL advanced-bridge `envoy.filters.http.wasm` surface: body/buffer/trailer hostcalls active; per-root-VM tick goroutine with 10ms envoy-go-strict floor; metric hostcalls + plugin-defined `wasmcustom.<name>` namespace; shared-data with CAS + envoy-go-strict caps; httpCall via cluster-based dispatch (RE-CONSUMES phase-20 `internal/httpclient/` at 3rd-or-later co-consumer); foreign-function registration interface with EMPTY default registry (envoy-go-strict departure); full ~70-path stream-info property surface via NEW `internal/filterstate/` framework primitive (with phase-22.2 lua MIGRATES) + RE-CONSUMES of ADR-0144 + ADR-0177 + ADR-0190; OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the deterministic fixture-0036 scenarios + REFERENCE-LESS-equivalent on the non-deterministic scenarios — modulo the ~27 envoy-go-strict documented divergence-windows (21 inherited from phase-25.1 + ~6 NEW at 25.2: body buffer cap + shared-data caps + tick period floor + foreign-function 0-vs-10 default registry + dynamic-stats cap + 9-counter envoy-go-strict bundle).

Phase 25.3 then activates the per-route 5th-canonical wholesale-override (REUSE-by-absence per AMEND-A3) + multi-plugin VM-sharing + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` + conformance harness seed at 62.5% threshold per AMEND-A8 against the same package surface.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044 — 5 AMEND-B entries from §11)

The 5 §11 D-25.2-1..D-25.2-5 empirical pins (executed at this SPEC session via parallel-subagent fan-out against `envoyproxy/envoy@v1.37.2` C++ source + `proxy-wasm/proxy-wasm-cpp-host@da3ce05d` + `proxy-wasm/spec@main` ABI v0.2.1 README + `proxy-wasm/proxy-wasm-rust-sdk@v0.2.4` per ADR-0004) generated the following **5 AMEND-block entries** load-bearing for 25.2:

- **AMEND-B1 (buffer-bounds CLAMP semantic — REFINES BRAINSTORM Q1 anticipation):** Per §11.1 D-25.2-1 scrape. The proxy-wasm v0.2.1 spec README text states `proxy_get_buffer_bytes` returns `WasmResult::BAD_ARGUMENT` "in case of buffer overflow due to invalid `start` and/or `max_size` values". The proxy-wasm-cpp-host reference implementation REFUTES this strict reading: `src/exports.cc:get_buffer_bytes` **clamps silently** — `if (start > buffer->size()) { length = 0; } else if (start + length > buffer->size()) { length = buffer->size() - start; }` and returns `WasmResult::Ok`. Only the `start + length` i32-overflow path returns `BadArgument`. **Implication for 25.2:** envoy-go MUST mirror cpp-host clamp behavior for compat with real wasm guests (Istio/Envoy production guests rely on the clamp); the SPEC body pins the clamp as the wire contract and treats the README text as imprecise. Lands at `internal/wasm/registration.go` (the `proxy_get_buffer_bytes` host shim).

- **AMEND-B2 (metric hostcall signedness + dynamic-stats namespace shape — REFINES BRAINSTORM Q9):** Per §11.2 D-25.2-2 scrape. Two REFINES: (1) `proxy_increment_metric(metric_id, delta)` delta is **SIGNED `int64`** (NOT unsigned as a careless reading would suggest) per cpp-host `src/exports.cc:1065-1068` + Rust-SDK `hostcalls.rs:1395-1397`; allows negative gauge deltas. `proxy_record_metric(metric_id, value)` value is UNSIGNED `uint64` per cpp-host L1070-1073. (2) Dynamic-stats namespace is **`wasmcustom.<custom_name>`** only (NO plugin prefix) per Envoy v1.37.2 `source/extensions/common/wasm/stats_handler.h:16` (`constexpr absl::string_view CustomStatNamespace = "wasmcustom";`) + `context.cc:1623-1625` (`Stats::Utility::counterFromElements(*envoyWasm()->scope_, {envoyWasm()->custom_stat_namespace_, stat_name})` — only namespace + raw user name; plugin name NOT prefixed). BRAINSTORM Q9 hypothesized `wasmcustom.<plugin_name>.<custom_name>` — REFUTED. **Disposition at envoy-go:** per-plugin isolation via per-plugin Registry SCOPE (each `*compiledConfig` constructs its own `*Registry` from `internal/stats/dynamic.go` rooted at the plugin's stat scope; stat names emerge as `wasmcustom.<custom_name>` byte-faithful to upstream; cross-plugin name collisions are namespaced via the parent scope, not by prefix interpolation). MetricType enum CONFIRMED: Counter=0, Gauge=1, Histogram=2.

- **AMEND-B3 (proxy_http_call wire shape CONFIRMS + late-response-after-stream-closed REFINES — substantive AMEND):** Per §11.3 D-25.2-3 scrape. proxy_http_call signature is the 10-arg shape per BRAINSTORM Q4 + cpp-host `src/exports.cc:664-687`; return is `WasmResult` (i32). Unknown-cluster disposition is `WasmResult::BadArgument` (=2) per Envoy v1.37.2 `context.cc:1547-1550` — CONFIRMS BRAINSTORM Q4. Late-response-after-stream-closed REFINES BRAINSTORM §10.1 D-25.2-3 hypothesis: cpp-host does NOT "silently drop" — instead it uses a TWO-LAYER cancellation mechanism: (1) Envoy v1.37.2 `context.cc:1900-1905` destructor iterates `http_request_` and calls `p.second.request_->cancel()` on every outstanding `AsyncClient::Request` at context teardown — in-flight requests CANCELLED; (2) defensive token-lookup at `context.cc:1693-1696` (`auto handler = http_request_.find(token); if (handler == http_request_.end()) { return; }`) silently drops any stray callback. NEITHER path increments a counter upstream. **Recommendation:** ADD envoy-go-strict counter `wasm.<plugin>.http_call_response_after_close` (defensive observability for the rare race where envoy-go's cancellation has a bug + a stray response arrives; non-zero signal pages an operator). The counter is upstream-superset — operationally safe + does not break wire parity. RAISES the §7 envoy-go-strict counter tally from 8 (per BRAINSTORM Q9) to **9** at 25.2; project stat count anticipated 119 → 128 (was 127 per BRAINSTORM).

- **AMEND-B4 (full proxy_get_property roster + serialization — SUBSTANTIVE REFINEMENT to BRAINSTORM Q7 root count):** Per §11.4 D-25.2-4 scrape against `envoyproxy/envoy@v1.37.2:source/extensions/filters/common/expr/context.h` + `source/extensions/common/wasm/context.cc:1040-1115`. Path serialization is **NUL-delimited byte segments** (e.g., `["foo","bar"]` → `0x66 0x6f 0x6f 0x00 0x62 0x61 0x72`) per spec README §Serialization + context.cc:1047-1058 host-parsing — CONFIRMS BRAINSTORM hypothesis. Roster REFINEMENT: actual upstream is **~10 dispatched roots + 4 direct tokens** (NOT ~25 roots as BRAINSTORM Q7 hypothesized):
  - **`request`** (16 sub-paths): path, url_path, host, scheme, method, referer, headers, headers_bytes, time, id, useragent, size, total_size, duration, protocol, query.
  - **`response`** (6 sub-paths): code, code_details, trailers, flags, grpc_status, backend_latency.
  - **`connection`** (12+id sub-paths): mtls, requested_server_name, tls_version, termination_details, subject_local_certificate, subject_peer_certificate, uri_san_local_certificate, uri_san_peer_certificate, dns_san_local_certificate, dns_san_peer_certificate, sha256_peer_certificate_digest, transport_failure_reason, id.
  - **`source`** (2 sub-paths): address, port.
  - **`destination`** (2 sub-paths): address, port.
  - **`upstream`** (~14 sub-paths): address, port, local_address, locality, transport_failure_reason, request_attempt_count, cx_pool_ready_duration, num_endpoints + TLS cert sub-symbols re-used from Connection (subject/uri_san/dns_san/sha256/tls_version).
  - **`xds`** (12 sub-paths — CONSOLIDATES listener+route+cluster metadata; NO standalone `listener.*` or `route.*` roots): cluster_name, cluster_metadata, route_name, route_metadata, virtual_host_name, virtual_host_metadata, upstream_host_metadata, upstream_host_locality_metadata, filter_chain_name, listener_metadata, listener_direction, node.
  - **`metadata`** (filter-name keyed; `google.protobuf.Struct`).
  - **`filter_state`** (key-keyed; `FilterStateObject`).
  - **`upstream_filter_state`** (key-keyed; upstream-scoped FilterStateObject) — **DISTINCT root co-equal to `filter_state`**; BRAINSTORM Q7 OMITTED this.
  - **`wasm.<key>`** (special; proxied to filter_state then upstream filter_state at `context.cc:987-1019`).
  - **Direct tokens** (4): `plugin_name`, `plugin_root_id`, `plugin_vm_id`, `connection_id`.
  - **NO standalone `listener.*`/`route.*`/`downstream.*` roots** — folded into `xds.*` + `connection.*` + `source.*`. **NO `wasm` root for self-introspection** beyond the `wasm.<filter_state_key>` proxy.
  - **Totals:** ~10 dispatched roots + 4 direct tokens; ~70 documented sub-paths excluding map/message recursion. Absent-property behavior returns `WasmResult::NotFound` (=1) per context.cc:1065/1072/1078/1083/1103/1106/1110.
  - **envoy-go-internal primitive mapping (for §3.2 ADR-0207 filterstate scope):** stream-local accessors handle request/response/source/destination/wasm-direct-tokens; ADR-0144 RE-CONSUMED for connection TLS cert sub-paths; ADR-0177 RE-CONSUMED for upstream cluster + address sub-paths; ADR-0190 RE-CONSUMED for metadata + xds metadata branches; NEW `internal/filterstate/` per ADR-0207 handles filter_state + upstream_filter_state + wasm.<key>.

- **AMEND-B5 (capability gating at registerCallback time — CONFIRMS Q3/AMEND-A5 with structural REFINEMENT):** Per §11.5 D-25.2-5 scrape. CONFIRMS key formats (env-namespace `proxy_<base>`; WASI bare names; lifecycle `proxy_on_<event>`). Structural REFINEMENT: env-namespace hostcall gating happens at `wasm.cc:176-189` `_REGISTER_PROXY` macro at `registerCallback` TIME — `exports.cc` call sites contain ZERO `capabilityAllowed` invocations. The gate is enforced by NOT REGISTERING the hostcall on the wasm runtime when the capability is denied; a guest invocation of a non-registered hostcall would trigger a wazero "missing-import" trap at module-instantiation OR a runtime "function-not-found" trap on call (depending on import-vs-call semantics). **Implication for envoy-go:** the `internal/wasm/registration.go` host-module wiring at 25.2 mirrors the gate-at-registration discipline — for denied capabilities, do NOT register the host function on the wazero Runtime; the guest's import resolution fails at module-instantiation OR the runtime trap fires on call. This matches upstream byte-faithfully + amplifies the default-deny posture: a denied hostcall is not just rejected at runtime, it is invisible to the guest from instantiation. **21-22 NEW capability keys at 25.2** (14 hostcall + 7 lifecycle; +1 if `proxy_on_queue_ready` is in 25.2 scope — per parent SPEC §2.15 shared-queue is DEFER to WASM host family, so 25.2 final = 21 NEW). Post-25.2 cumulative roster size: 37 (25.1) + 21 = **58 keys**.

This 25.2 SPEC's §3-§14 incorporate all 5 AMENDs. AMEND-B1 (clamp) + AMEND-B2 (metric signedness + namespace) + AMEND-B3 (late-response counter) carry forward to IMPL as wire-shape pins. AMEND-B4 (property roster) carries forward to PLAN as the canonical roster transcription target. AMEND-B5 (gate-at-registration) carries forward to IMPL as the host-module wiring architectural pin.

### 1.2 ADR continuity + D-hypothesis at 25.2 SPEC commit

Phase 25.1 IMPL closed at ADR-0204 §Decision + §Consequences body lands. **At THIS 25.2 SPEC commit: 4 NEW ADR §Context drafts anchor** per ADR-0044 §Context-draft discipline:

- **ADR-0205 §Context** — Root VM lifecycle evolution per Q3 (ONE long-lived `*RootVM` per `*compiledConfig` + per-stream contexts as CHILDREN sharing wazero Runtime+Module + tick goroutine + Clock seam + per-stream Module instantiation R8 escape-valve discipline). §Context anchored at THIS SPEC commit; §Decision + §Consequences body lands at 25.2 IMPL atomic-landing Task.
- **ADR-0206 §Context** — 25.2 ABI extensions (~14 NEW env-namespace hostcalls + ~7 NEW guest-export callbacks + 21 NEW capability keys + `internal/wasm/foreign.go` `ForeignFunctionRegistry` with EMPTY default registry per AMEND-A9 + the buffer-clamp wire-contract pin per AMEND-B1 + the metric signedness pin per AMEND-B2 + the gate-at-registration architectural pin per AMEND-B5). §Context anchored at THIS SPEC commit.
- **ADR-0207 §Context** — NEW `internal/filterstate/` framework primitive at 25.2 second-consumer scope per Q7 + EXTRACT-NOW-on-second-consumer discipline + consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES + ADR-0188 API-revision allowance NOT consumed (the `internal/lua/` framework primitive itself is untouched; only the in-package filterstate.go file migrates). §Context anchored at THIS SPEC commit.
- **ADR-0208 §Context** — `internal/filter/http/wasm/` 25.2 package extensions — full hostcall wiring per §3.1 ABI surface row + 9 envoy-go-strict counters per Q9 + AMEND-B3 + 4 envoy-go-strict-only config fields per Qs 2/6/9 + dynamic-stats namespace `wasmcustom.<custom_name>` via NEW `internal/stats/dynamic.go` per AMEND-B2 + mixed-mode fixture-0036 discipline per Q8 + 25.2 BEHAVIOR_CONTRACT.md ~7-edit bundle per ADR-0052. §Context anchored at THIS SPEC commit.

**Next-free ADR after THIS 25.2 SPEC commit: `ADR-0209`** (4 numbers consumed: ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208). ADR-0044 escape-valve held in reserve at `ADR-0209` for the STRENGTHENED-WEAK-HOLD-with-1-slot conditional consumption surface per Q10.

**In-place AMENDMENT acknowledgment on ADR-0202** (one-line paragraph in §Consequences) anchored AT 25.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. **No new ADR number consumed** for the acknowledgment. Provisional wording (settles at 25.2 IMPL final Task): *"Phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern carries forward to 25.2 IMPL R8 escape-valve). The EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent."*

**D-hypothesis at 25.2 SPEC commit:** BRAINSTORM Q10 STRENGTHENED-WEAK-HOLD-with-1-slot-buffer predicted 4 anticipated NEW ADRs (ADR-0205..ADR-0208) landing cleanly + 0-1 escape-valve consumption at 25.2 IMPL (likely candidate: per-stream Module instantiation R8 escape-valve if benchmark > 1ms surfaces; secondary candidate: SPEC-time empirical-discovery surface in any 25.2 hostcall implementation — e.g., wazero CompilationCache eviction semantic edge case; pairs wire-format buffer-bounds error class refinement). This SPEC's §11 empirical-pin scrape produces ONE substantive structural refinement (AMEND-B5 gate-at-registration architecture vs gate-at-call-site) and FOUR wire-shape refinements (AMEND-B1 clamp; AMEND-B2 signedness + namespace; AMEND-B3 cancel-vs-drop + counter recommendation; AMEND-B4 root consolidation + upstream_filter_state addition); none of the 5 AMENDs ESCALATES a new ADR consumption (all absorb cleanly into the ADR-0205+0206+0207+0208 §Decision body anchors at IMPL).

**SPEC-time disposition:** STRENGTHENED-WEAK-HOLD-with-1-slot-buffer STANDS (UNCHANGED from BRAINSTORM Q10). 4 anticipated ADRs (ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208) land cleanly + ADR-0202 one-line in-place AMEND acknowledgment + 0-1 escape-valve slot consumption at ADR-0209. The only remaining escape-valve candidate post-AMEND-B1..B5 closure is the per-stream Module instantiation R8 benchmark surface (most-likely candidate; LOW probability per 25.1 Task 17 R8 observed 61µs/stream WELL UNDER 1ms threshold; the 25.2 root-VM model makes per-stream context creation EVEN cheaper).

---

## 2. Non-purposes

Phase 25.2 is the second sub-phase of the phase-25 BRAINSTORM-time 3-way pre-split. It does NOT extend the framework beyond the minimum needed to land the full advanced-bridge surface delta + the 4 NEW ADRs + the ADR-0202 in-place AMEND acknowledgment.

- **2.1 Per-route `Wasm` 5th-canonical wholesale-override via TPFC OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 18 UNCHANGED from 25.1; via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint). 25.3 activates with 5th-canonical REUSE-by-absence per AMEND-A3 + ADR-0210 EXPLICIT-NO-NEW-CANONICAL classification ADR; ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment.
- **2.2 Multi-plugin VM-sharing via duplicate `vm_id` OUT OF SCOPE.** PARSE-REJECT UNCHANGED from 25.1 (parent §6.2 arm 12). At 25.2 each `PluginConfig` still constructs its own `*RootVM`; cross-plugin VM-sharing via shared `vm_id` defers to 25.3 per ADR-0211. Cross-plugin shared-data scoping (shared-data visibility across PluginConfigs sharing one vm_id) opens at 25.3 BRAINSTORM/SPEC.
- **2.3 `VmConfig.environment_variables` activation OUT OF SCOPE.** PARSE-REJECT UNCHANGED from 25.1 (parent §6.2 arm 13). At 25.2 the WASI `environ_*` shims still return zeros; 25.3 activates feeding from `EnvironmentVariables.host_env_keys` + `key_values`.
- **2.4 `PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig` OUT OF SCOPE.** PARSE-REJECT UNCHANGED from 25.1 (parent §6.2 arm 9). 25.3 activates with `wasm.<plugin>.{vm_reload, vm_reload_backoff, vm_reload_success, vm_reload_failure}` Group-C counter surface per AMEND-A2.
- **2.5 `PluginConfig.fail_open` deprecated bool OUT OF SCOPE.** PARSE-REJECT UNCHANGED from 25.1 (parent §6.2 arm 10). 25.3 maps onto `failure_policy = FAIL_OPEN` per AMEND-A1 ladder.
- **2.6 `test/conformance/proxy-wasm/` conformance harness seed OUT OF SCOPE.** Opens at 25.3 IMPL per AMEND-A8 + ADR-0212 (62.5% pass-threshold target against `proxy-wasm-cpp-host@da3ce05d:test/`).
- **2.7 Shared-queue hostcalls (4) + `proxy_on_queue_ready` callback OUT OF SCOPE.** Per parent §2.15 + BRAINSTORM Q10 confirmation: DEFER to WASM host family (cross-VM cross-vm_id coordination at WasmService scope; structurally distinct from 25.2's per-RootVM scope). The 4 shared-queue hostcalls (`proxy_register_shared_queue` + `proxy_resolve_shared_queue` + `proxy_enqueue_shared_queue` + `proxy_dequeue_shared_queue`) + `proxy_on_queue_ready` callback STAY stub-Unimplemented at 25.2 + 25.3.
- **2.8 Outbound gRPC hostcalls (5) + 4 gRPC callbacks OUT OF SCOPE at all sub-phases.** Per parent §2.17. The gRPC surface intersects `internal/grpcclient/` at multiple integration points; carries non-trivial scope. STAY stub-Unimplemented through 25.3.
- **2.9 TCP/network-filter hostcalls + callbacks OUT OF SCOPE.** Per parent §2.6. Network-filter-wasm row out-of-row at phase 25; consumes `internal/wasm/` at consumer #2+ scope under ADR-0202's API-REVISION ALLOWANCE clause at a future WASM host family phase.
- **2.10 Foreign-function default registry STAYS EMPTY.** Per AMEND-A9 + parent §2.18: `internal/wasm/foreign.go` `ForeignFunctionRegistry` lands at 25.2 with ZERO default registrations. The upstream 10 foreign functions (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`) are NOT ported at phase 25. envoy-go-strict departure record #4 at 25.2 BEHAVIOR_CONTRACT.md (operators MUST explicitly enable the `proxy_call_foreign_function` capability AND register specific foreign functions via the NEW `wasm.RegisterForeignFunction(name, fn)` API at boot; unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream).
- **2.11 Cross-side byte-exact for 25.2 non-deterministic scenarios OUT OF SCOPE.** Per Q8 + AMEND-A4 §4.5 D6 guardrails: tick-fires-counter + httpCall-success + httpCall-unknown-cluster + body-cap-exceeded scenarios use the subject-only `StatsAsserter.AssertStats` discipline per `reference_differential_asserter_dispatch` (each subject-side arm gets a deliberate-break liveness verification per phase-23 fixture-0030 lesson). Mixed-mode fixture-0036 carries the 8-10 deterministic scenarios via `CompareBytes` + the 3-4 non-deterministic scenarios via StatsAsserter.
- **2.12 wazero JIT/AOT compiler backend opt-in OUT OF SCOPE.** Per parent §2.7. Interpreter default at 25.x; compiler opt-in is a future ops-tuning phase.
- **2.13 Memory-trap fixture scenarios OUT OF SCOPE.** Per parent §4.5 D6 guardrail (a): wazero traps with different error strings than V8. Memory-OOM probes DEFERRED beyond 25.2; may land at 25.3-or-later with mixed-mode discipline.
- **2.14 HTTP/2 header iteration order fixture dependence OUT OF SCOPE.** Per parent §4.5 D6 guardrail (b): fixture-0036 scenarios use HTTP/1.1 OR sort-on-assertion (HPACK reorder divergence between wazero/Go's `net/http.Header` and V8/Envoy's HeaderMap).
- **2.15 Float-formatted log lines OUT OF SCOPE.** Per parent §4.5 D6 guardrail (c): no float-formatted numbers on the 25.2 wire (operators that hit float divergence get scoped-fix at a future ops-tuning phase). Histogram-metric record values (`proxy_record_metric` with histogram type) use `uint64` per AMEND-B2 — no float exposure.
- **2.16 Per-stream Module instantiation pattern HOLDS at fresh-per-stream WEAK-default (R8 escape-valve only).** Per Q3 + parent §13-R8 + D-25.2-P2: 25.2 IMPL benchmarks gate the disposition. If `BenchmarkPerStreamModule_Instantiation > 1ms`, ADR-0209 escape-valve fires with the "pooled vs shared-Module-with-mutex-serialization" decision. Until benchmarked, hold fresh-per-stream Module instantiation as the WEAK-default (25.1 R8 observed 61µs/stream for full per-stream `*wazero.Runtime`+Module construction; the 25.2 root-VM model SHOULD shrink this further since Runtime is no longer re-constructed per-stream).
- **2.17 `proxy_set_effective_context` host-side context switching at 25.2 is per-callback scope.** The hostcall fires from tick + httpCall + foreign-function callbacks (root-context dispatch) to switch into a stream-context for downstream hostcalls (e.g., setting a header on the originating stream). At 25.2, the implementation honors the upstream byte-faithful semantic (per cpp-host `wasm.cc:setEffectiveContext`) — switches the active context for the lifetime of the current hostcall dispatch frame; the next root-context-frame entry re-defaults. NO cross-stream state leak via the context switch.
- **2.18 No filter-chain ordering surgery.** UNCHANGED from 25.1: wasm stays at one entry in the existing extension registry; the HCM filter-chain iteration protocol is unchanged. 20 HTTP filters wired (UNCHANGED from 25.1).
- **2.19 No `response_code_details` emission** — unchanged from prior §9 rows.
- **2.20 NO MIGRATION of phase-22.2's `:filterState()` Lua surface** — the migration is INTERNAL only (replacing the in-package `map[string]any` storage with delegation to `internal/filterstate/*Bucket`). The Lua-visible `:filterState():get(name)` + `:filterState():set(name, value)` surface stays byte-identical to phase-22.2 IMPL. The 2 envoy-go-strict divergences (mutation exposure + typed Lua-value marshaling per phase-22.2 AMEND-22.2-4) carry forward UNCHANGED.
- **2.21 No `internal/wasm/abi/` subdirectory restructuring.** UNCHANGED from 25.1 IMPL (ADR-0202 §Decision pinned the `abi/types.go` single-file shape inside the subdirectory). The 25.2 abi-type extensions (any new enum values; the 25.2 hostcall stub signatures move from 25.1 stubs to active impls) edit `abi/types.go` in place + the per-family ABI dispatch files (`abi/body_bridge.go` + `abi/timer.go` + `abi/metrics.go` + `abi/shared_data.go` + `abi/http_call.go` + `abi/foreign.go`) flip from stub-Unimplemented to real-impl.
- **2.22 NO ADR-0205 `wazero-VM-pool` design at 25.2.** Per Q10 strict-scope: BRAINSTORM REDIRECTED ADR-0205 to anchor the root VM lifecycle evolution (NOT the original 25.1 escape-valve target). ADR-0209 takes the escape-valve reserve slot at 25.2 (likely candidates: per-stream Module instantiation R8 escape-valve if benchmark fires; OR a SPEC-time empirical-discovery surface in any 25.2 hostcall implementation).
- **2.23 NO PARSE-REJECT for 25.2-deferred-to-25.3 fields beyond the 25.1 roster.** The 25.1 PARSE-REJECT arms for `failure_policy = FAIL_RELOAD` + `reload_config` + `environment_variables` + `fail_open` + duplicate-vm_id + per-route TPFC STAY active at 25.2. The 25.2 NEW PARSE-REJECT arms cover only 25.2-introduced surfaces (envoy-go-strict-only config field validators; capability-restriction-config arm structure validation).
- **2.24 Cross-plugin shared-data visibility OUT OF SCOPE.** Per Q6 + parent §2.20: shared-data scope at 25.2 = cross-stream within ONE PluginConfig (each `*RootVM` owns its own `sharedDataMap`). Cross-plugin shared-data (multi-PluginConfig sharing one `vm_id`) defers to 25.3 with the multi-plugin VM-sharing surface per ADR-0211.
- **2.25 `wasm.<plugin>.envoy_go.failures` counter EXTENDED scope.** At 25.1 the counter incremented on `proxy_on_*` host-side panic-wrapper trips only. At 25.2 the counter ALSO increments on: tick-goroutine panic-wrapper trips; httpCall response-callback panic-wrapper trips; foreign-function host-side panic-wrapper trips; shared-data-cap-exceeded events (cap exceeded is BOTH a `shared_data_cap_exceeded` counter increment AND an `envoy_go.failures` increment because the VM enters a degraded-state for the cap-exceeded plugin context). No NEW counter name; semantic extension only. Recorded at 25.2 BEHAVIOR_CONTRACT.md sub-section.
- **2.26 NO ADR-0125 amendments** per AMEND-A3 STAYS at 10 canonicals (UNCHANGED from 25.1 SPEC).
- **2.27 NEVER-DEFERRED — v1.32.4 vs v1.37.2 binding-gap `PluginConfig.allow_on_headers_stop_iteration`** (parent §5.7 forward-pointer). At 25.2 the field is STILL absent from the consumed v1.32.4 binding; PAUSE return semantics still treated per parent §5.7 (CONTINUE fallback at 25.1 — but 25.2 ADDS the body-bridge surface where PAUSE has meaningful semantics for body-chunk buffering). At 25.2 the PAUSE-buffer pattern works for `proxy_on_request_body` + `proxy_on_response_body` (host buffers further chunks until guest returns CONTINUE on a subsequent invocation) but the headers-bridge PAUSE return at 25.2 STILL falls through to CONTINUE per the v1.32.4 binding-gap. Activates fully when go-control-plane bumps to v1.37.x.
- **2.28 NO MIGRATION of phase-22.2's `internal/dynamicmetadata/` cross-filter consumer adapter** — UNCHANGED from 25.1: phases 16/17/18/19/20's "operator-visibility deferred to future" BEHAVIOR_CONTRACT.md notes stay AS-IS until their respective next-touchpoint phases. 25.2 RE-CONSUMES the primitive as a THIRD-or-later co-consumer (phase-22.2 lua was first; phase-22.2 deferred-pickup was second; 25.2 wasm via `proxy_get_property "metadata.*"` is third or later depending on intervening phases).

---

## 3. Framework primitive evolutions — `internal/wasm/` API extensions + NEW `internal/filterstate/` + NEW `internal/stats/dynamic.go`

Per BRAINSTORM §3 framework-survey + parent SPEC §4 + AMEND-B1..B5 §11 pins. Phase 25.2 introduces **1 NEW package-level framework primitive** (`internal/filterstate/` per Q7 + ADR-0207) + **1 NEW infrastructure package** (`internal/stats/dynamic.go` per Q9 + ADR-0208) + **3 `internal/wasm/` API evolutions** (root VM lifecycle per Q3 + ADR-0205; ABI surface extension per ADR-0206; foreign-function registration interface per AMEND-A9 + ADR-0206) + **0 in-place ADR amendments except the ADR-0202 one-line acknowledgment** + **7 framework REUSES** + **1 MIGRATES** (phase-22.2 `internal/filter/http/lua/filterstate.go` rewrites to consume the new `internal/filterstate/` primitive).

### 3.1 `internal/wasm/` API evolutions — refined from BRAINSTORM §3.1 sketch (lands at IMPL across Tasks 1-9)

Refines BRAINSTORM §3.1 sketch into production signatures. Key refinements vs BRAINSTORM:

- **Root VM lifecycle anchored at NEW `internal/wasm/root_vm.go`** — `*RootVM` type + `NewRootVM(opts...) *RootVM` constructor + `(*RootVM).Configure(vmConfigBytes, pluginConfigBytes []byte) error` + `(*RootVM).NewStreamContext() *StreamContext` + `(*RootVM).Close() error` + tick goroutine internals + shared-data map + httpCall response routing + foreign-function registry view.
- **`*StreamContext` REPLACES the 25.1 per-stream `*VM`** — same exported method-set surface (CallProxyOnRequestHeaders + CallProxyOnResponseHeaders + ...) but the lifecycle is parent-anchored at `*RootVM` rather than self-anchored. The per-stream-context creation cost is microseconds (just bookkeeping + a `proxy_on_context_create(streamCtxID, rootCtxID)` invocation on the shared Module instance); no wazero Runtime construction.
- **Module instantiation pattern AT 25.2 IMPL R8 escape-valve scope** — fresh-per-stream Module instantiation is the WEAK-default at SPEC commit per D-25.2-P2 + parent §13-R8. The shared-Module-with-mutex-serialization OR pooled-Module alternatives carry forward to IMPL benchmark; if `BenchmarkPerStreamModule_Instantiation > 1ms`, ADR-0209 escape-valve fires.
- **ABICallbacks interface EXTENDED with 7 NEW methods** per ADR-0206 — `OnRequestBody` + `OnResponseBody` + `OnRequestTrailers` + `OnResponseTrailers` + `OnTick` + `OnHttpCallResponse` + `OnForeignFunction`. The 25.1 13-method interface grows to 20 methods at 25.2.
- **Gate-at-registration host-module wiring per AMEND-B5** — `internal/wasm/registration.go` mirrors upstream cpp-host: for each capability in the 58-key 25.2 cumulative roster (37 from 25.1 + 21 NEW), if the per-`*RootVM` SandboxConfig.IsAllowed(key) returns false, the corresponding host function is NOT registered on the wazero Runtime. The guest's import resolution fails at module-instantiation OR the runtime trap fires on call. This matches upstream byte-faithfully + amplifies the default-deny posture.
- **`internal/wasm/foreign.go` NEW file per AMEND-A9** — `ForeignFunctionRegistry` (Register / Get; sync.RWMutex + map[string]ForeignFunction) with EMPTY default registry; the `proxy_call_foreign_function` host shim returns `WasmResult::NotFound` (=1) for unregistered names byte-faithful to upstream cpp-host `src/exports.cc:147-184`; capability-gated via default-deny `proxy_call_foreign_function` capability key per AMEND-A5 + AMEND-B5 gate-at-registration. envoy-go-strict departure record #4 at 25.2 BEHAVIOR_CONTRACT.md (0-vs-10 default registrations).
- **`internal/wasm/tick.go` NEW file per Q5 + ADR-0205** — per-`*RootVM` dedicated goroutine running `for { select { case <-clock.After(effectivePeriod): rootVM.lockAndCall(proxy_on_tick, rootCtxID); case <-stop: return } }`. `effectivePeriod = max(period_ms, 10ms)` — envoy-go-strict 10ms period floor + envoy-go-strict departure record #4 at 25.2 BEHAVIOR_CONTRACT.md (consolidated with shared-data + body-buffer caps per §9). Uses ADR-0186 `Clock` seam injection at construction time for fixture fake-time support. FIRST co-consumer of phase-21 ADR-0186 Clock seam beyond phase-21 itself — RATIFIES the extraction.
- **`internal/wasm/shared_data.go` NEW file per Q6 + ADR-0206** — per-`*RootVM` `sharedDataMap` (Go `map[string]sharedDataEntry`; `sharedDataEntry struct { value []byte; cas uint32 }`; sync.RWMutex). CAS semantic byte-exact from cpp-host: `cas=0` unconditionally writes (returns new CAS value in subsequent get); `cas>0` writes only if existing entry's CAS value matches (returns `WasmResult::CasMismatch` (=8) on mismatch). envoy-go-strict cap discipline: per-value 1 MiB cap (configurable via `envoy_go_strict_shared_data_value_cap_bytes`; default 1048576); 1024-entry cap (configurable via `envoy_go_strict_shared_data_max_entries`; default 1024). Cap exceeded returns `WasmResult::InternalFailure` (=10) + `wasm.<plugin>.shared_data_cap_exceeded` envoy-go-strict counter + integration error log.
- **`internal/wasm/property.go` NEW file per Q7 + AMEND-B4 + ADR-0206** — full proxy-wasm property-path tree mapping. NUL-delimited path parsing per AMEND-B4. Per-root dispatch covering ~10 dispatched roots + 4 direct tokens; ~70 documented sub-paths. Co-consumed primitives per AMEND-B4 mapping: stream-local accessors (request/response/source/destination/wasm-direct-tokens); RE-CONSUMES ADR-0144 (connection TLS sub-paths); RE-CONSUMES ADR-0177 (upstream cluster + address sub-paths); RE-CONSUMES ADR-0190 (metadata + xds metadata branches); EXTRACTS NEW `internal/filterstate/` per ADR-0207 (filter_state + upstream_filter_state + wasm.<key> proxy branches).
- **`internal/wasm/dynamic_stats.go` NEW file per Q9 + AMEND-B2 + ADR-0208** — wraps a per-`*RootVM` `*Registry` instance from `internal/stats/dynamic.go` (one `*Registry` per `*compiledConfig`); the `proxy_define_metric(MetricType, name) → metric_id` host shim allocates from the per-plugin Registry which scopes under the per-plugin stat scope to produce `wasmcustom.<custom_name>` byte-faithful to upstream per AMEND-B2. Increment uses SIGNED `int64` delta; Record uses UNSIGNED `uint64` value per AMEND-B2. envoy-go-strict 1024-entry cap on dynamic namespace + `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict counter + envoy-go-strict-only `envoy_go_strict_dynamic_stats_max_entries` config field.
- **Buffer-clamp wire-contract per AMEND-B1** — `proxy_get_buffer_bytes` host shim clamps on `start + max_size > buffer.length` (returns `WasmResult::Ok` with truncated length). Lands at `internal/wasm/registration.go` (or wherever the buffer-bytes shim lives). Test coverage: golden table at `registration_test.go` covering `start_in_bounds + max_in_bounds`, `start_in_bounds + max_overflows`, `start_at_end + max_anything`, `start_beyond_end`, `start + max_size i32-overflow → BadArgument`.

Production signatures (lands at IMPL across Tasks 1-9):

```go
// internal/wasm/root_vm.go — RootVM lifecycle + StreamContext + tick + shared-data + httpCall

package wasm

import (
    "context"
    "sync"
    "time"

    "github.com/tetratelabs/wazero"
    "github.com/esalaine/envoy-go/internal/clock"
    "github.com/esalaine/envoy-go/internal/stats/dynamic"
)

// RootVM is the per-compiledConfig long-lived VM. ONE per *compiledConfig.
// Owns: tick goroutine + tick state; shared-data map; httpCall response
// routing + call_id allocation; foreign-function registry view; the per-plugin
// dynamic-stats *Registry; the per-stream-context map.
type RootVM struct {
    runtime  wazero.Runtime          // single per-RootVM
    module   wazero.CompiledModule   // per-Module compile-cache reference
    instance api.Module              // instantiated once at NewRootVM
    sandbox  SandboxConfig
    rootCtxID uint32                 // typically 1

    // tick state (per Q5 + ADR-0186 Clock seam):
    clk      clock.Clock
    tickPeriod time.Duration         // 0 = no tick scheduled
    tickStop chan struct{}
    tickMu   sync.Mutex              // protects tickPeriod / tickStop

    // shared-data state (per Q6):
    sharedData    map[string]sharedDataEntry
    sharedDataMu  sync.RWMutex
    sharedDataValCap uint32          // from PluginConfig envoy_go_strict_shared_data_value_cap_bytes
    sharedDataMaxEntries uint32      // from PluginConfig envoy_go_strict_shared_data_max_entries

    // httpCall state (per Q4 + AMEND-B3):
    httpCallsMu  sync.Mutex
    httpCalls    map[uint32]*pendingHttpCall  // call_id → state
    nextCallID   uint32

    // foreign-function registry view (per AMEND-A9):
    foreignReg   *ForeignFunctionRegistry

    // per-plugin dynamic-stats Registry (per Q9 + AMEND-B2):
    dynStats     *dynamic.Registry

    // per-stream-context map (per Q3 — children sharing root VM):
    streamCtxsMu sync.RWMutex
    streamCtxs   map[uint32]*StreamContext   // streamCtxID → StreamContext
    nextStreamCtxID uint32                   // monotonic per-RootVM
}

type sharedDataEntry struct {
    value []byte
    cas   uint32
}

type pendingHttpCall struct {
    streamCtxID uint32   // originating stream context; 0 if root-context dispatched
    deadline    time.Time
    // cancelled  bool  — set at OnDestroy of originating stream; defensive guard at response arrival
}

// RootVMOption configures RootVM construction.
type RootVMOption func(*RootVM)

func WithRootSandboxConfig(sb SandboxConfig) RootVMOption
func WithRootClock(clk clock.Clock) RootVMOption           // FIRST co-consumer of ADR-0186 beyond phase-21
func WithRootPanicHandler(h PanicHandlerFn) RootVMOption
func WithRootLogSink(w io.Writer) RootVMOption
func WithRootHttpClient(c *httpclient.Client) RootVMOption   // RE-CONSUMES phase-20 ADR-0177
func WithRootForeignRegistry(reg *ForeignFunctionRegistry) RootVMOption
func WithRootDynamicStatsRegistry(reg *dynamic.Registry) RootVMOption
func WithRootSharedDataCaps(valCap, maxEntries uint32) RootVMOption

// NewRootVM constructs the per-compiledConfig long-lived VM. Applies sandbox
// config + creates the underlying wazero.Runtime + registers the 35-37 active
// hostcalls per AMEND-B5 gate-at-registration (denied capabilities → NOT
// registered) + sets up tick goroutine (initially idle; activates via
// proxy_set_tick_period_milliseconds). Caller responsibility: Close at
// compiledConfig shutdown.
func NewRootVM(ctx context.Context, module *Module, rootCtxID uint32, opts ...RootVMOption) (*RootVM, error)

// Configure invokes _initialize OR _start + proxy_on_vm_start +
// proxy_on_configure on the root context. Fires ONCE at config-load.
func (rv *RootVM) Configure(ctx context.Context, vmConfigBytes, pluginConfigBytes []byte) error

// NewStreamContext allocates a streamCtxID + invokes proxy_on_context_create.
// Returns a per-stream context handle for filter-side use.
func (rv *RootVM) NewStreamContext(ctx context.Context) (*StreamContext, error)

// SetTickPeriod schedules the tick goroutine with the given period (clamped
// to floor 10ms per Q5 envoy-go-strict). period=0 cancels the tick.
func (rv *RootVM) SetTickPeriod(period time.Duration)

// DispatchHttpCall starts an outbound HTTP request via the configured
// httpclient.Client and returns the allocated call_id. Response routes
// asynchronously to OnHttpCallResponse via the streamCtxID's StreamContext.
// per AMEND-B3 + ADR-0177 RE-CONSUMER.
func (rv *RootVM) DispatchHttpCall(ctx context.Context, streamCtxID uint32, cluster string, headers []HeaderPair, body []byte, trailers []HeaderPair, timeoutMs uint32) (callID uint32, result WasmResult)

// CallForeignFunction looks up the registered function and invokes it. Returns
// NotFound if name is not registered (per AMEND-A9).
func (rv *RootVM) CallForeignFunction(ctx context.Context, streamCtxID uint32, name string, args []byte) (result []byte, status WasmResult)

// SetSharedData CAS-protected. Returns CasMismatch on conflict + InternalFailure
// on cap exceeded.
func (rv *RootVM) SetSharedData(key string, value []byte, cas uint32) WasmResult
func (rv *RootVM) GetSharedData(key string) (value []byte, cas uint32, status WasmResult)

// Close releases the wazero Runtime + stops the tick goroutine + cancels
// outstanding httpCalls + closes the dynamic-stats Registry. Idempotent.
func (rv *RootVM) Close() error
```

```go
// internal/wasm/stream_context.go — per-stream context (replaces 25.1 per-stream *VM)

package wasm

// StreamContext is the per-stream filter dispatch context. NOT goroutine-safe;
// access is per-stream-single-goroutine by envoy-go's filter dispatch model.
// Each per-stream filter dispatch creates a StreamContext via RootVM.NewStreamContext;
// OnDestroy releases via Close.
type StreamContext struct {
    rootVM    *RootVM
    ctxID     uint32  // assigned by RootVM
    cb        ABICallbacks  // consumer-side (HTTP filter) callbacks
    // captured local-response state (per 25.1 abi_callbacks)
    sentLocalResponse *capturedLocalResponse
}

func (sc *StreamContext) RegisterABICallbacks(cb ABICallbacks)

func (sc *StreamContext) CallProxyOnRequestHeaders(ctx context.Context, numHeaders uint32, endOfStream bool) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnResponseHeaders(ctx context.Context, numHeaders uint32, endOfStream bool) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnRequestBody(ctx context.Context, bodySize uint32, endOfStream bool) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnResponseBody(ctx context.Context, bodySize uint32, endOfStream bool) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnRequestTrailers(ctx context.Context, numTrailers uint32) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnResponseTrailers(ctx context.Context, numTrailers uint32) (ProxyAction, error)
func (sc *StreamContext) CallProxyOnDone(ctx context.Context) (bool, error)
func (sc *StreamContext) CallProxyOnLog(ctx context.Context) error
func (sc *StreamContext) CallProxyOnDelete(ctx context.Context) error

// Close fires proxy_on_done + proxy_on_log + proxy_on_delete on the stream
// context + cancels any outstanding httpCalls dispatched from this stream
// context (cancel-at-destruction per AMEND-B3). Idempotent.
func (sc *StreamContext) Close(ctx context.Context) error
```

```go
// internal/wasm/foreign.go — ForeignFunctionRegistry per AMEND-A9

package wasm

import "sync"

// ForeignFunctionFn is the host-side foreign-function callback.
// Receives raw bytes from proxy_call_foreign_function; returns raw bytes
// + WasmResult status.
type ForeignFunctionFn func(ctx context.Context, args []byte) (result []byte, status WasmResult)

// ForeignFunctionRegistry is the per-process foreign-function registry.
// EMPTY default registry per AMEND-A9 (operators register at boot).
type ForeignFunctionRegistry struct {
    mu  sync.RWMutex
    fns map[string]ForeignFunctionFn
}

func NewForeignFunctionRegistry() *ForeignFunctionRegistry

// Register adds a foreign function. Returns error if name already registered.
func (r *ForeignFunctionRegistry) Register(name string, fn ForeignFunctionFn) error

// Get looks up the registered function. Returns nil + false if not registered;
// the proxy_call_foreign_function host shim returns WasmResult::NotFound for
// unregistered names.
func (r *ForeignFunctionRegistry) Get(name string) (ForeignFunctionFn, bool)

// DefaultForeignFunctionRegistry is the process-global registry consumed by
// the wasm filter factory. Operators register via this.
var DefaultForeignFunctionRegistry = NewForeignFunctionRegistry()
```

### 3.2 NEW `internal/filterstate/` framework primitive (ADR-0207; lands at 25.2 IMPL)

Per Q7 + AMEND-B4 + BRAINSTORM §3.2. Package boundary: `internal/filterstate/` hosts the GENERIC per-stream filter-state primitive (cross-filter-reusable; no HTTP-filter-specific knowledge). Consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking (thin adapter delegates to `*Bucket`; the `:filterState()` Lua surface stays UNCHANGED). Consumer #2 = 25.2 wasm via `proxy_get_property "filter_state.*"` + `"upstream_filter_state.*"` paths per AMEND-B4 (upstream_filter_state is a DISTINCT root co-equal to filter_state).

```go
// internal/filterstate/filterstate.go — generic per-stream filter-state primitive

package filterstate

// FilterStateObject is the value stored in a Bucket. Carries the typed data
// + serialization + state-type discriminator (read-only vs mutable).
type FilterStateObject interface {
    Marshal() ([]byte, error)
    Unmarshal([]byte) error
    HasData() bool
    StateType() StateType
}

type StateType int

const (
    StateTypeReadOnly StateType = iota
    StateTypeMutable
)

// Bucket is the per-stream filter-state accessor. NOT goroutine-safe;
// access is per-stream-single-goroutine by envoy-go's filter dispatch model.
type Bucket struct {
    mu     sync.RWMutex  // (defensive; access path is single-goroutine in practice)
    items  map[string]FilterStateObject
}

func NewBucket() *Bucket

// Set stores a FilterStateObject under the given key. Mutable-state-type
// entries OVERRIDE existing entries; read-only-state-type entries with the
// same key as a Mutable entry are rejected.
func (b *Bucket) Set(key string, obj FilterStateObject) error

// Get retrieves a FilterStateObject by key. Returns nil + false if absent.
func (b *Bucket) Get(key string) (FilterStateObject, bool)

// Keys lists all currently-set keys (for property-tree enumeration at the
// proxy-wasm property-path `filter_state.*` dispatch).
func (b *Bucket) Keys() []string
```

**Tests (5 files):**
- `filterstate_test.go` — Set/Get/Keys round-trip + read-only-vs-mutable conflict + nil-handling.
- `bucket_concurrency_test.go` — RWMutex discipline + concurrent-read concurrent-add tests.
- `filterstateobject_test.go` — interface conformance + edge cases.
- (filter package adapter tests live in the consumer packages: phase-22.2 lua + 25.2 wasm.)

**MIGRATION (mandatory at 25.2 IMPL Task NN — see §6):** phase-22.2's `internal/filter/http/lua/filterstate.go` REWRITES to consume the new primitive. The `:filterState()` Lua surface stays UNCHANGED — the Lua bridge's `:filterState():get(name)` + `:filterState():set(name, value)` accessors delegate to `internal/filterstate/*Bucket` via a thin adapter. The 2 envoy-go-strict divergences from upstream lua (per phase-22.2 AMEND-22.2-4 — mutation exposure + typed Lua-value marshaling) carry forward UNCHANGED. The migration delta is ~50-100 LoC inside `internal/filter/http/lua/`; no test breakage.

**EXPLICIT API-REVISION ALLOWANCE clause** (anchored at ADR-0207 §Decision body at 25.2 IMPL Lands-in-Task): the primitive's API shape is provisional at consumer #2 (25.2 wasm); future consumers (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families) MAY require API revision after empirical validation. Mirrors phase-22.1 ADR-0188 + phase-22.2 ADR-0190 + phase-25.1 ADR-0202 allowance pattern at the symmetric scope.

### 3.3 NEW `internal/stats/dynamic.go` infrastructure (ADR-0208; lands at 25.2 IMPL)

Per Q9 + AMEND-B2 + BRAINSTORM §3.3. NEW thin wrapper exposing Register-at-runtime + Lookup-by-id for the `wasmcustom.<custom_name>` dynamic-stats namespace per AMEND-B2 (REFINES BRAINSTORM Q9 — namespace is `wasmcustom.<custom_name>` ONLY, no plugin prefix; per-plugin isolation via per-plugin Registry SCOPE).

```go
// internal/stats/dynamic/dynamic.go — per-plugin dynamic stats Registry

package dynamic

import (
    "sync"

    "github.com/esalaine/envoy-go/internal/stats"
)

// MetricID is an opaque token returned from Register.
type MetricID uint32

// MetricType identifies counter/gauge/histogram.
type MetricType int

const (
    MetricTypeCounter MetricType = iota   // = 0 per AMEND-B2
    MetricTypeGauge                       // = 1
    MetricTypeHistogram                   // = 2
)

// Registry is the per-plugin-config dynamic-stats namespace. Constructed at
// config-load via NewRegistry(pluginScope, maxEntries) where pluginScope is
// the per-plugin stat scope (e.g., the *stats.Scope rooted at the plugin
// stat-prefix). Stat names registered via Register emerge under
// pluginScope/wasmcustom.<name> byte-faithful to upstream Envoy per AMEND-B2.
type Registry struct {
    mu          sync.RWMutex
    pluginScope *stats.Scope
    maxEntries  uint32
    byID        map[MetricID]registryEntry
    byName      map[string]MetricID  // name → MetricID for re-define dedup
    nextID      MetricID
}

type registryEntry struct {
    name       string  // "wasmcustom.<custom_name>"
    metricType MetricType
    counter    *stats.Counter  // populated if Counter
    gauge      *stats.Gauge    // populated if Gauge
    histogram  *stats.Histogram // populated if Histogram
}

func NewRegistry(pluginScope *stats.Scope, maxEntries uint32) *Registry

// Register allocates a MetricID + registers the metric under
// pluginScope/wasmcustom.<name>. Returns the cached MetricID if name is
// already registered (idempotent). Returns ErrCapExceeded if entries ≥
// maxEntries.
func (r *Registry) Register(metricType MetricType, name string) (MetricID, error)

// Increment adds delta (signed int64 per AMEND-B2) to the named counter or gauge.
// Returns ErrNotFound for unknown id. ErrBadArgument if applied to Histogram
// (which doesn't support increment).
func (r *Registry) Increment(id MetricID, delta int64) error

// Record sets value (unsigned uint64 per AMEND-B2) for the named gauge or histogram.
// Returns ErrNotFound for unknown id. ErrBadArgument if applied to Counter.
func (r *Registry) Record(id MetricID, value uint64) error

// Get returns the current value of the named metric. Returns ErrNotFound for
// unknown id.
func (r *Registry) Get(id MetricID) (uint64, error)

// EnumerateForAdmin walks the registry for /stats lazy enumeration.
func (r *Registry) EnumerateForAdmin(fn func(name string, value uint64))
```

**Tests (3 files):**
- `dynamic_test.go` — Register/Increment/Record/Get round-trip + signed-delta semantics + idempotent-Register + ErrCapExceeded threshold + ErrBadArgument enforcement.
- `dynamic_admin_test.go` — admin /stats enumeration round-trip + name format `wasmcustom.<custom_name>` byte-pin.
- `dynamic_concurrency_test.go` — RWMutex discipline + concurrent-Register stress test (cap-boundary race).

**Anchored at ADR-0208** §Decision body (alongside the filter package extensions + envoy-go-strict counter bundle + mixed-mode fixture-0036 discipline + 25.2 BEHAVIOR_CONTRACT.md bundle).

### 3.4 REUSES (7 frameworks + 1 capability-gate sub-reuse + 1 MIGRATES)

- **REUSE 1: HCM-parse-time PARSE-REJECT path** — adds ~8-12 NEW PARSE-REJECT arms at 25.2 (timer-period-required-when-tick-capability-enabled; metric-name-required; httpCall-cluster-required; foreign-function-name-required; envoy-go-strict-only config field validators for body-buffer cap / shared-data caps / dynamic-stats cap; root-id-vs-vm-id collision). Total arm count post-25.2: ~26-30. See §6 for the per-arm enumeration.
- **REUSE 2: per-request filter interface (decode + encode hooks + body + trailer hooks)** — 25.2 ACTIVATES `OnDecodeBuffer` + `OnEncodeBuffer` (body) + `DecodeTrailers` + `EncodeTrailers` (trailers). The async-resume pattern for `proxy_http_call` reuses the fault / ext_authz precedent (filter returns StopAndBuffer / StopAndAllIteration; resumes via filter callbacks when response arrives). Body buffer cap exceeded uses `SendLocalReply` (413 Payload Too Large per Q2).
- **REUSE 3: phase-20 `internal/httpclient/` via ADR-0177** — 25.2 RE-CONSUMES at third-or-later co-consumer (phase-22.2's `:httpCall()` was second). NO API extension; the cluster-based dispatch API added at phase-22.2 IMPL covers 25.2's `proxy_http_call` shape byte-for-byte. RATIFIES the phase-20 framework-primitive extraction; CLOSES parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor.
- **REUSE 4: phase-21 `Clock` seam via ADR-0186** — FIRST co-consumer beyond phase-21 itself. The tick dispatcher goroutine injects `clock.Clock` at `NewRootVM` time via `WithRootClock(clk)` for fixture fake-time support. RATIFIES the phase-21 Clock-seam extraction.
- **REUSE 5: phase-22.2 `internal/dynamicmetadata/` via ADR-0190** — third-or-later co-consumer for `proxy_get_property "metadata.*"` + `"xds.*_metadata"` paths per AMEND-B4. NO API extension; the per-stream `*Bucket` accessor + map[(filter_name, key)]google.protobuf.Value shape maps cleanly.
- **REUSE 6: phase-04 ADR-0144 `DownstreamPrincipal()`** — second co-consumer beyond phase-04 itself; powers `connection.tls.*` sub-paths per AMEND-B4 (`connection.subject_local_certificate` + `connection.subject_peer_certificate` + `connection.uri_san_local_certificate` + `connection.uri_san_peer_certificate` + `connection.dns_san_local_certificate` + `connection.dns_san_peer_certificate` + `connection.sha256_peer_certificate_digest` + `connection.tls_version` + `connection.mtls`).
- **REUSE 7: 25.1 `internal/wasm/abi/types.go`** — the 10-named-value `WasmResult` enum (with value-gaps at 5/9/11 per AMEND-A7) + `WasmBufferType` (values 0/1/4 ACTIVATED at 25.2: HttpRequestBody / HttpResponseBody / HttpCallResponseBody) + `WasmHeaderMapType` (values 1/3 ACTIVATED at 25.2: HttpRequestTrailers / HttpResponseTrailers).
- **REUSE 8 (capability-gate sub-reuse): 25.1 `internal/wasm/sandbox.go` default-deny gate** — extends with 21 NEW capability keys at 25.2 per AMEND-B5; the gate function itself unchanged; default-deny posture inherited per AMEND-A5; gate-at-registration discipline per AMEND-B5 (host-module wiring at `registration.go` does NOT register denied capabilities on the wazero Runtime).

**MIGRATES (1):**
- **phase-22.2 `internal/filter/http/lua/filterstate.go`** — REWRITES to consume the new `internal/filterstate/` primitive under ADR-0207 (§3.2). Non-breaking; `:filterState()` Lua surface unchanged. Migration delta ~50-100 LoC inside `internal/filter/http/lua/`.

NO new `internal/` package beyond `internal/filterstate/` (+ the thin `internal/stats/dynamic` infrastructure subpackage). NO top-level primitive package outside `internal/`. NO new go.mod direct dependency (wazero v1.10.1 + proxy-wasm-rust-sdk =0.2.4 inherited from 25.1 per AMEND-A1).

### 3.5 `internal/wasm/` 25.2 file split (extends 25.1 8-file production split + abi/ subdirectory)

Extends the 25.1 IMPL-landed `internal/wasm/` package per ADR-0202 §Decision. NEW files at 25.2:

```
internal/wasm/
  doc.go              # 25.1 — updated at 25.2 to add 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs
  vm.go               # 25.1 per-stream VM — RETIRED at 25.2 (see migration note in 25.2 IMPL Task notes); the file remains (carries the deprecated *VM type for backwards compat in any transitional code) OR is deleted in favor of root_vm.go + stream_context.go (decision at IMPL Task 1)
  root_vm.go          # NEW 25.2 — *RootVM lifecycle anchor; tick goroutine; shared-data; httpCall routing; foreign-function registry view; dynamic-stats Registry
  stream_context.go   # NEW 25.2 — per-stream context (replaces 25.1 per-stream *VM)
  compile.go          # 25.1 — UNCHANGED at 25.2
  sandbox.go          # 25.1 — EXTENDED with 21 NEW capability key constants per AMEND-B5
  registration.go     # 25.1 — EXTENDED with 14 NEW hostcall registrations + 7 NEW callback dispatch entries; gate-at-registration discipline per AMEND-B5; buffer-clamp wire-contract per AMEND-B1 at proxy_get_buffer_bytes shim
  bytecode_util.go    # 25.1 — UNCHANGED at 25.2
  pairs.go            # 25.1 — UNCHANGED at 25.2 (pairs wire format reused by trailers + httpCall headers/trailers)
  wasi.go             # 25.1 — UNCHANGED at 25.2 (no new WASI hostcalls at 25.2 per AMEND-B5)
  tick.go             # NEW 25.2 — per-RootVM tick goroutine + 10ms floor + Clock seam injection
  shared_data.go      # NEW 25.2 — per-RootVM CAS-protected K-V map + envoy-go-strict caps
  property.go         # NEW 25.2 — full proxy-wasm property-path tree mapping per AMEND-B4
  dynamic_stats.go    # NEW 25.2 — proxy_define_metric dispatch + signed-i64 delta per AMEND-B2 + 1024-entry cap; wraps internal/stats/dynamic.Registry
  foreign.go          # NEW 25.2 — ForeignFunctionRegistry per AMEND-A9 + EMPTY default registry
  http_call.go        # NEW 25.2 — proxy_http_call dispatch + callout_id allocation + AsyncClient request lifecycle + cancel-at-destruction per AMEND-B3 + http_call_response_after_close counter increment guard
  abi/types.go        # 25.1 — UNCHANGED at 25.2 (existing enums cover the 25.2 surface; WasmBufferType values 0/1/4 + WasmHeaderMapType values 1/3 ACTIVATED via host-module registration)
  abi/body_bridge.go  # NEW 25.2 — body+buffer hostcall dispatch (proxy_get/set_buffer_bytes + proxy_get_buffer_status)
  abi/timer.go        # NEW 25.2 — proxy_set_tick_period_milliseconds dispatch
  abi/metrics.go      # NEW 25.2 — proxy_define_metric / proxy_increment_metric / proxy_record_metric / proxy_get_metric dispatch
  abi/shared_data.go  # NEW 25.2 — proxy_get/set_shared_data dispatch
  abi/http_call.go    # NEW 25.2 — proxy_http_call + proxy_on_http_call_response dispatch
  abi/foreign.go      # NEW 25.2 — proxy_call_foreign_function dispatch + foreign-function-name resolution
  abi/stream_control.go # NEW 25.2 — proxy_continue_stream + proxy_close_stream dispatch

  # 25.1 test files UNCHANGED; new 25.2 test files:
  root_vm_test.go     # NEW 25.2 — RootVM lifecycle + tick + shared-data + httpCall + foreign-function round-trip tests
  stream_context_test.go # NEW 25.2 — per-stream context + concurrent dispatch tests
  tick_test.go        # NEW 25.2 — tick goroutine + Clock seam fake-time tests + 10ms floor enforcement
  shared_data_test.go # NEW 25.2 — CAS semantics + cap-boundary tests
  property_test.go    # NEW 25.2 — full property roster tests (per AMEND-B4 ~70 sub-paths) + NUL-delimited path parsing + absent-property NotFound
  dynamic_stats_test.go # NEW 25.2 — proxy_define_metric / Increment / Record / Get round-trip + 1024-entry cap test
  foreign_test.go     # NEW 25.2 — ForeignFunctionRegistry Register / Get + EMPTY-default behavior
  http_call_test.go   # NEW 25.2 — proxy_http_call dispatch + cancel-at-destruction + late-response-after-close counter increment
  abi/body_bridge_test.go # NEW 25.2 — buffer-clamp golden table per AMEND-B1 (clamp on overflow; BadArgument on i32-overflow only)
  abi/metrics_test.go # NEW 25.2 — MetricType enum byte-pin (Counter=0; Gauge=1; Histogram=2 per AMEND-B2)
  abi/shared_data_test.go # NEW 25.2 — CAS golden table; cap-exceeded WasmResult::InternalFailure
```

### 3.6 `internal/filter/http/wasm/` 25.2 file split (extends 25.1 8-file production split)

Extends the 25.1 IMPL-landed `internal/filter/http/wasm/` package per ADR-0203 §Decision. NEW files at 25.2:

```
internal/filter/http/wasm/
  doc.go              # 25.1 — updated at 25.2 to add 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs
  wasm.go             # 25.1 — UNCHANGED at 25.2 (TypeURL + filterName + New + filter struct)
  compiled_config.go  # 25.1 — EXTENDED at 25.2: parse 4 NEW envoy-go-strict-only config fields; ~8-12 NEW PARSE-REJECT arms; construct *RootVM via wasm.NewRootVM; construct per-plugin dynamic.Registry; construct per-plugin filterstate.Bucket access pattern
  datasource.go       # 25.1 — UNCHANGED at 25.2
  abi_callbacks.go    # 25.1 — EXTENDED at 25.2 with 7 NEW methods (OnRequestBody + OnResponseBody + OnRequestTrailers + OnResponseTrailers + OnTick + OnHttpCallResponse + OnForeignFunction) + 4 RE-USE primitive consumers (DownstreamPrincipal + httpclient + dynamicmetadata + filterstate)
  decode_headers.go   # 25.1 — EXTENDED at 25.2: per-stream construction goes through RootVM.NewStreamContext (not wasm.NewVM)
  encode_headers.go   # 25.1 — EXTENDED at 25.2: per-stream context shared with decode side
  stats.go            # 25.1 — EXTENDED at 25.2 with 9 NEW envoy-go-strict counters per Q9 + AMEND-B3 (raises 119 → 128) + per-plugin dynamic.Registry plumbing
  body.go             # NEW 25.2 — DecodeData / EncodeData glue; body-buffer accumulation per-stream; cap enforcement (envoy_go_strict_body_buffer_cap_bytes default 16 MiB per Q2); 413-on-exceed via SendLocalReply; body_buffer_cap_exceeded counter
  trailers.go         # NEW 25.2 — DecodeTrailers / EncodeTrailers glue; invokes CallProxyOnRequestTrailers / CallProxyOnResponseTrailers; reuses 25.1 pairs wire-format
  tick_clock.go       # NEW 25.2 — Clock seam injection plumbing (WithRootClock); fake-time test seam for fixture-0036 tick scenarios
  property.go         # NEW 25.2 — per-stream property resolver dispatch (delegates to wasm.RootVM.property tree + the 4 RE-USE primitives)

  # 25.1 test files UNCHANGED; new 25.2 test files:
  compiled_config_test.go  # 25.1 — EXTENDED with new PARSE-REJECT arm coverage + envoy-go-strict-only config field validators
  abi_callbacks_test.go    # 25.1 — EXTENDED with 7 NEW method coverage + 4 RE-USE primitive round-trip tests
  body_test.go             # NEW 25.2 — body-buffer accumulation + cap enforcement + 413-on-exceed dispatch
  trailers_test.go         # NEW 25.2 — trailer hostcall dispatch round-trip
  property_test.go         # NEW 25.2 — per-stream property resolver dispatch (per AMEND-B4 ~70 sub-paths)
  dispatch_test.go         # 25.1 — EXTENDED with body/trailer/tick/httpCall integration round-trips
  fuzz_test.go             # 25.1 — UNCHANGED (FuzzWasmConfigParse stays at standard ADR-0018 baseline; FuzzWasmHostcallEnvelope lives at internal/wasm/ per §8.4)
```

### 3.7 Boot-registration UNCHANGED (per ADR-0072 + 25.1 SPEC §3.6)

`cmd/envoy-go/main.go` STAYS at 20 HTTP-filter entries post-phase-25.1 (UNCHANGED at 25.2). The 25.2 work extends `internal/filter/http/wasm/` in-place — no new registration entry; the existing `httpReg.Register(wasm.TypeURL, wasm.New)` call serves the 25.2 surface via `wasm.New` returning the EXTENDED filter factory.

---

## 4. compiledConfig + code shapes (extends 25.1)

### 4.1 Public surface UNCHANGED

`TypeURL`, `filterName`, `New` per 25.1 ADR-0203 + 25.1 SPEC §4.1. Extension-registry registration UNCHANGED (no boot-registration delta at 25.2). The `New` factory signature is the same; the internal construction at 25.2 builds the EXTENDED `*compiledConfig` with the 4 NEW envoy-go-strict-only config fields + the `*wasm.RootVM` + the per-plugin `*dynamic.Registry`.

### 4.2 `compiledConfig` shape (EXTENDED at 25.2)

```go
package wasm

import (
    "github.com/esalaine/envoy-go/internal/stats"
    "github.com/esalaine/envoy-go/internal/stats/dynamic"
    "github.com/esalaine/envoy-go/internal/wasm"
)

// compiledConfig is the immutable post-parse listener-level config.
// EXTENDED at 25.2 with 25.2-specific state.
type compiledConfig struct {
    module       *wasm.Module          // 25.1 — pre-compiled wasm bytecode
    compileCache *wasm.CompileCache    // 25.1 — module-cache holder
    sandbox      wasm.SandboxConfig    // 25.1 — from PluginConfig.capability_restriction_config
    pluginName   string                // 25.1 — from PluginConfig.name
    rootContextID uint32               // 25.1 — plugin-context discriminator
    vmConfig     []byte                // 25.1 — from VmConfig.configuration
    pluginConfig []byte                // 25.1 — from PluginConfig.configuration
    stats        *filterStats          // 25.1 — EXTENDED at 25.2 with 9 new envoy-go-strict counters

    // NEW 25.2 — long-lived RootVM (replaces 25.1 per-stream *wasm.VM construction):
    rootVM       *wasm.RootVM          // constructed at New(); shared across all per-stream contexts
                                       // owns: tick goroutine + shared-data map + httpCall routing + foreign-fn view + dynamic-stats Registry

    // NEW 25.2 envoy-go-strict-only config fields (per Qs 2/6/9):
    bodyBufferCapBytes      uint32     // default 16 MiB (16777216); from envoy_go_strict_body_buffer_cap_bytes
    sharedDataValueCapBytes uint32     // default 1 MiB (1048576); from envoy_go_strict_shared_data_value_cap_bytes
    sharedDataMaxEntries    uint32     // default 1024; from envoy_go_strict_shared_data_max_entries
    dynStatsMaxEntries      uint32     // default 1024; from envoy_go_strict_dynamic_stats_max_entries

    // NEW 25.2 — per-plugin dynamic-stats Registry (per Q9 + AMEND-B2):
    dynStats     *dynamic.Registry     // pluginScope=stats.RootScope.Subscope("wasm").Subscope(pluginName); name format "wasmcustom.<custom_name>"

    // NEW 25.2 — per-plugin foreign-function registry view (per AMEND-A9):
    foreignReg   *wasm.ForeignFunctionRegistry  // points at the process-global DefaultForeignFunctionRegistry by default; testable seam
}

// filterStats — EXTENDED at 25.2 from 5 to 14 counters (5 from 25.1 + 9 new envoy-go-strict per Q9 + AMEND-B3).
type filterStats struct {
    // 25.1 carry-forward (tri-group prefix per AMEND-A2):
    created       *stats.Counter // wasm.wazero.created
    active        *stats.Gauge   // wasm.wazero.active
    executions    *stats.Counter // wasm.<plugin_name>.executions
    hostcallDenied *stats.Counter // wasm.<plugin_name>.hostcall_denied
    envoyGoFailures *stats.Counter // wasm.<plugin_name>.envoy_go.failures (scope EXTENDED at 25.2 per §2.25)

    // NEW 25.2 envoy-go-strict counters (9 — 8 from BRAINSTORM Q9 + 1 from AMEND-B3 recommendation):
    tickInvocations               *stats.Counter // wasm.<plugin>.tick_invocations
    httpCallDispatched            *stats.Counter // wasm.<plugin>.http_call_dispatched
    httpCallResponse              *stats.Counter // wasm.<plugin>.http_call_response
    foreignFunctionDenied         *stats.Counter // wasm.<plugin>.foreign_function_denied
    bodyBufferCapExceeded         *stats.Counter // wasm.<plugin>.body_buffer_cap_exceeded
    httpCallDispatchUnknownCluster *stats.Counter // wasm.<plugin>.http_call_dispatch_unknown_cluster
    sharedDataCapExceeded         *stats.Counter // wasm.<plugin>.shared_data_cap_exceeded
    dynamicStatsCapExceeded       *stats.Counter // wasm.<plugin>.dynamic_stats_cap_exceeded
    httpCallResponseAfterClose    *stats.Counter // wasm.<plugin>.http_call_response_after_close (per AMEND-B3 recommendation)
}
```

The `compiledConfig` is constructed at `New` and shared (read-only) across the listener's `*filter` per-stream instances. Each per-stream filter dispatch calls `cfg.rootVM.NewStreamContext(ctx)` to obtain a fresh `*wasm.StreamContext` per §3.1 (cheap; no Runtime construction). The `*RootVM` is constructed ONCE at `New` + the tick goroutine starts immediately (idle until `proxy_set_tick_period_milliseconds` fires) + the shared-data map is initialized empty + the httpCall routing state is empty + the foreign-function registry view points at `wasm.DefaultForeignFunctionRegistry`. **No per-route stat at 25.2** (no per-route Wasm at 25.2; per-route TPFC PARSE-REJECTs per parent §6.2 arm 18 UNCHANGED).

### 4.3 Filter struct + per-stream dispatch shape (EXTENDED at 25.2)

```go
package wasm

import (
    "github.com/esalaine/envoy-go/internal/wasm"
)

// filter is the per-stream filter instance. NOT goroutine-safe.
// EXTENDED at 25.2: vm field replaced with streamCtx (pointing into the
// shared *RootVM); body-buffer accumulation state added.
type filter struct {
    cfg           *compiledConfig

    // 25.2: per-stream context handle (replaces 25.1 *wasm.VM):
    streamCtx     *wasm.StreamContext  // allocated at DecodeHeaders entry via cfg.rootVM.NewStreamContext

    // 25.2: body-buffer accumulation state (per Q1 + Q2):
    decodeBody    []byte               // accumulated request body (grows on each OnDecodeBuffer)
    encodeBody    []byte               // accumulated response body (grows on each OnEncodeBuffer)
    decodeBodyCapExceeded bool         // sticky flag after first cap-exceeded event on decode side
    encodeBodyCapExceeded bool         // sticky flag for encode side

    // 25.1 carry-forward — captured local-response state (now also set by body/trailer/httpCall paths):
    sentLocalResponse *capturedLocalResponse
}

// Compile-time interface assertions — UNCHANGED:
var (
    _ http.StreamDecoderFilter = (*filter)(nil)
    _ http.StreamEncoderFilter = (*filter)(nil)
)
```

The dispatch shape per-stream lifecycle (EXTENDED at 25.2):
- **DecodeHeaders**: `streamCtx, err := cfg.rootVM.NewStreamContext(ctx)` → `streamCtx.RegisterABICallbacks(&abiCallbacks{filter: f, ...})` → (`*RootVM` already configured at `New` time, so NO Run call here) → `streamCtx.CallProxyOnRequestHeaders(ctx, headerCount, endOfStream)` → `cfg.stats.executions++` → ProxyAction handling (CONTINUE / PAUSE / captured-local-response).
- **DecodeData** (NEW at 25.2): accumulate `data` into `f.decodeBody`; if `len(f.decodeBody) > cfg.bodyBufferCapBytes` and not already cap-exceeded, set `f.decodeBodyCapExceeded = true` + `cfg.stats.bodyBufferCapExceeded++` + `cfg.stats.envoyGoFailures++` + return `StopAllIteration` + `decoderCb.SendLocalReply(413, "Payload Too Large", ...)`; else if `f.streamCtx.HasGlobalFunc("proxy_on_request_body")`: `streamCtx.CallProxyOnRequestBody(ctx, uint32(len(f.decodeBody)), endStream)` + ProxyAction handling. NO-op if `proxy_on_request_body` is not exported (guest doesn't opt into body callbacks).
- **DecodeTrailers** (NEW at 25.2): if `f.streamCtx.HasGlobalFunc("proxy_on_request_trailers")`: `streamCtx.CallProxyOnRequestTrailers(ctx, numTrailers)` + ProxyAction handling.
- **EncodeHeaders / EncodeData / EncodeTrailers**: mirror decode side for `proxy_on_response_headers` / `proxy_on_response_body` / `proxy_on_response_trailers`.
- **OnDestroy**: `streamCtx.Close(ctx)` (fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels outstanding httpCalls per AMEND-B3); UNCHANGED from 25.1 conceptually but the cleanup path is delegated to `*StreamContext.Close` rather than `*wasm.VM.Close`.

---

## 5. Hostcall + Callback surface delta (38 active hostcalls at 25.2 + 20 callbacks at 25.2)

Per BRAINSTORM §1.1 + §3 + AMEND-B1..B5 + parent SPEC §4.2 Option B. Phase 25.2 EXTENDS the 25.1 24-hostcall + 13-callback surface to **38 active hostcalls (24 from 25.1 + 14 NEW activated; 9 STILL stub-Unimplemented = shared-queue 4 + gRPC 5 deferred to WASM host family per §2.7 + §2.8) + 20 guest-export callbacks (13 from 25.1 + 7 NEW)**.

### 5.1 14 NEW env-namespace hostcalls active at 25.2

Per BRAINSTORM §1.1 + §3.1 + parent SPEC §3.1 surface-mapping table + AMEND-B1..B5 pins. Each row pins the hostcall + the source ADR + the file-of-implementation + the capability key per AMEND-B5.

| # | Hostcall (full args) | Returns | Capability key | File-of-impl | AMEND/Q anchor |
|---|---|---|---|---|---|
| 25 | `proxy_get_buffer_bytes(buffer_type, start, max_size, ret_data_ptr, ret_size_ptr)` | WasmResult | `proxy_get_buffer_bytes` | `internal/wasm/registration.go` + `abi/body_bridge.go` | AMEND-B1 (clamp-on-overflow semantic; cpp-host divergence from spec README) |
| 26 | `proxy_set_buffer_bytes(buffer_type, start, size, data_ptr, data_size)` | WasmResult | `proxy_set_buffer_bytes` | `internal/wasm/registration.go` + `abi/body_bridge.go` | Q1 |
| 27 | `proxy_get_buffer_status(buffer_type, ret_size_ptr, ret_flags_ptr)` | WasmResult | `proxy_get_buffer_status` | `internal/wasm/registration.go` + `abi/body_bridge.go` | Q1 |
| 28 | `proxy_continue_stream(stream_type)` | WasmResult | `proxy_continue_stream` | `internal/wasm/registration.go` + `abi/stream_control.go` | Q1 (paired with PAUSE-buffer dispatch on body callbacks) |
| 29 | `proxy_close_stream(stream_type)` | WasmResult | `proxy_close_stream` | `internal/wasm/registration.go` + `abi/stream_control.go` | Q1 |
| 30 | `proxy_set_tick_period_milliseconds(period_ms)` | WasmResult | `proxy_set_tick_period_milliseconds` | `internal/wasm/registration.go` + `abi/timer.go` + `internal/wasm/tick.go` | Q5 (10ms envoy-go-strict floor; Clock seam FIRST co-consumer) |
| 31 | `proxy_define_metric(metric_type, name_data, name_size, ret_metric_id_ptr)` | WasmResult | `proxy_define_metric` | `internal/wasm/registration.go` + `abi/metrics.go` + `internal/wasm/dynamic_stats.go` + `internal/stats/dynamic/` | Q9 + AMEND-B2 (MetricType Counter=0/Gauge=1/Histogram=2; namespace `wasmcustom.<name>`) |
| 32 | `proxy_increment_metric(metric_id, delta)` (delta = SIGNED `int64`) | WasmResult | `proxy_increment_metric` | `internal/wasm/registration.go` + `abi/metrics.go` | AMEND-B2 (signed-i64 delta) |
| 33 | `proxy_record_metric(metric_id, value)` (value = UNSIGNED `uint64`) | WasmResult | `proxy_record_metric` | `internal/wasm/registration.go` + `abi/metrics.go` | AMEND-B2 (unsigned-u64 value) |
| 34 | `proxy_get_metric(metric_id, ret_value_ptr)` | WasmResult | `proxy_get_metric` | `internal/wasm/registration.go` + `abi/metrics.go` | Q9 |
| 35 | `proxy_set_shared_data(key_ptr, key_size, value_ptr, value_size, cas)` | WasmResult | `proxy_set_shared_data` | `internal/wasm/registration.go` + `abi/shared_data.go` + `internal/wasm/shared_data.go` | Q6 (CAS + 1 MiB value cap + 1024-entry cap envoy-go-strict) |
| 36 | `proxy_get_shared_data(key_ptr, key_size, ret_value_ptr_ptr, ret_value_size_ptr, ret_cas_ptr)` | WasmResult | `proxy_get_shared_data` | `internal/wasm/registration.go` + `abi/shared_data.go` | Q6 |
| 37 | `proxy_http_call(cluster_data, cluster_size, headers_data, headers_size, body_data, body_size, trailers_data, trailers_size, timeout_ms, ret_call_id_ptr)` (10 args; timeout_ms = `uint32`) | WasmResult | `proxy_http_call` | `internal/wasm/registration.go` + `abi/http_call.go` + `internal/wasm/http_call.go` | Q4 + AMEND-B3 (BadArgument-on-unknown-cluster; cancel-at-destruction; http_call_response_after_close counter) |
| 38 | `proxy_call_foreign_function(name_data, name_size, args_data, args_size, ret_results_data_ptr, ret_results_size_ptr)` | WasmResult | `proxy_call_foreign_function` | `internal/wasm/registration.go` + `abi/foreign.go` + `internal/wasm/foreign.go` | AMEND-A9 (NotFound for unregistered; EMPTY default registry; envoy-go-strict departure record #4) |

**Total 25.2 active env-namespace hostcalls: 16 (25.1) + 14 (NEW above) = 30 hostcalls.**

### 5.2 8 `wasi_snapshot_preview1.*` namespace shims UNCHANGED at 25.2

Per AMEND-B5 (no new WASI hostcalls at 25.2 — 25.2 adds env-namespace hostcalls only). The 8 WASI shims from 25.1 IMPL Task 4 (per R4 + parent §4.2) STAY active UNCHANGED: `fd_write` + `clock_time_get` + `random_get` + `environ_sizes_get` + `environ_get` + `args_sizes_get` + `args_get` + `proc_exit`. `environ_*` STILL returns zeros at 25.2 (lifts at 25.3 per parent §2.21).

**Total 25.2 active WASI hostcalls: 8 (UNCHANGED from 25.1).**

### 5.3 7 NEW guest-export callbacks at 25.2

The host invokes these via `wazero.Module.ExportedFunction("proxy_on_X").Call(ctx, args...)` lookups; the corresponding `*StreamContext` methods (per §3.1) OR `*RootVM` methods (for root-context callbacks tick + httpCallResponse + foreignFunction) wrap each call with the capability gate + the panic-wrapper. Per AMEND-B5 the gate for guest-export callbacks is at `getFunction` lookup time (NOT at call site, in contrast to env-namespace hostcalls which gate at `registerCallback` time per §3.1): if the corresponding capability key is denied, the function pointer is set to nullptr-equivalent + the host treats the missing function as if the guest hadn't exported it.

| # | Callback (full args) | Returns | Capability key | Caller | AMEND/Q anchor |
|---|---|---|---|---|---|
| C14 | `proxy_on_request_body(stream_context_id, body_size, end_of_stream)` | ProxyAction | `proxy_on_request_body` | `*StreamContext.CallProxyOnRequestBody` | Q1 (body_size = accumulated total; per-chunk invoke) |
| C15 | `proxy_on_response_body(stream_context_id, body_size, end_of_stream)` | ProxyAction | `proxy_on_response_body` | `*StreamContext.CallProxyOnResponseBody` | Q1 |
| C16 | `proxy_on_request_trailers(stream_context_id, num_trailers)` | ProxyAction | `proxy_on_request_trailers` | `*StreamContext.CallProxyOnRequestTrailers` | Q1 (trailer hostcalls reuse 25.1 header-map family; values 1+3) |
| C17 | `proxy_on_response_trailers(stream_context_id, num_trailers)` | ProxyAction | `proxy_on_response_trailers` | `*StreamContext.CallProxyOnResponseTrailers` | Q1 |
| C18 | `proxy_on_tick(root_context_id)` | none | `proxy_on_tick` | `*RootVM.tick goroutine` | Q5 (per-RootVM goroutine + 10ms floor + Clock seam) |
| C19 | `proxy_on_http_call_response(plugin_context_id, call_id, num_headers, body_size, num_trailers)` | none | `proxy_on_http_call_response` | `*RootVM.dispatchHttpCallResponse` (after AsyncClient response arrives + token lookup succeeds; or `httpCallResponseAfterClose` counter increments if token lookup misses per AMEND-B3) | Q4 + AMEND-B3 |
| C20 | `proxy_on_foreign_function(context_id, foreign_function_id, data_size)` | none | `proxy_on_foreign_function` | `*StreamContext.callForeignFunctionResponse` | AMEND-A9 |

**Total 25.2 active guest-export callbacks: 13 (25.1) + 7 (NEW above) = 20 callbacks.**

### 5.4 Stub-Unimplemented hostcalls at 25.2

Per parent §4.2 Option B: 25.2 STILL registers ALL hostcalls (active + deferred = 47 total) so module-instantiation succeeds for modules that import the deferred surface; the deferred stubs return `WasmResult::Unimplemented` (=12) when invoked.

Hostcalls STAYING stub-Unimplemented at 25.2 (deferred to 25.3 OR WASM host family OR never-on-roadmap):

- **Shared-queue family (4)** — `proxy_register_shared_queue` + `proxy_resolve_shared_queue` + `proxy_enqueue_shared_queue` + `proxy_dequeue_shared_queue`. DEFER to WASM host family per §2.7.
- **gRPC family (5)** — `proxy_grpc_call` + `proxy_grpc_stream` + `proxy_grpc_send` + `proxy_grpc_cancel` + `proxy_grpc_close`. DEFER to WASM host family per §2.8.
- **Total stub-Unimplemented at 25.2: 9 hostcalls** (down from 23 stub-Unimplemented at 25.1 IMPL — 14 of 23 LIFTED at 25.2 per §5.1).

Callbacks STAYING stub-Unimplemented at 25.2 (host never invokes them; if the guest exports them, the host's getFunction lookup may still succeed but the host never calls them):

- **Shared-queue callback (1)** — `proxy_on_queue_ready` (DEFER to WASM host family per §2.7).
- **gRPC callbacks (4)** — `proxy_on_grpc_receive_initial_metadata` + `proxy_on_grpc_receive` + `proxy_on_grpc_receive_trailing_metadata` + `proxy_on_grpc_close` (DEFER to WASM host family per §2.8).

### 5.5 Cumulative hostcall + callback count at 25.2 phase-done

- **Active env-namespace hostcalls:** 30 (16 from 25.1 + 14 NEW at 25.2)
- **Active WASI hostcalls:** 8 (UNCHANGED from 25.1)
- **Total active hostcalls at 25.2:** 38
- **Stub-Unimplemented hostcalls at 25.2:** 9 (shared-queue 4 + gRPC 5)
- **Total registered hostcalls at 25.2:** 47 (UNCHANGED — Option B per parent §4.2)
- **Active guest-export callbacks at 25.2:** 20 (13 from 25.1 + 7 NEW)
- **Total guest-export callback roster (proxy-wasm v0.2.1 spec):** 30; ~10 unused by host at 25.2 (gRPC callbacks + queue-ready + a few lifecycle variants like proxy_on_upstream_data unused by HTTP filter).

---

## 6. PARSE-REJECT roster extension (6 NEW arms at 25.2; 18 → 24 arms cumulative)

Per BRAINSTORM §3.4 REUSE 1 + parent SPEC §6.3 25.2 forward-pointer. The 18-arm 25.1 PARSE-REJECT roster STAYS active at 25.2 verbatim (per ADR-0080 byte-stable wording discipline + 25.1 D-P5 closure at Task 9). The 25.2 EXTENSIONS cover 25.2-introduced surfaces — envoy-go-strict-only config field validators + capability-restriction-config arm structure validators + 25.2-hostcall-prerequisite validators.

### 6.1 Wording discipline + arm-name convention UNCHANGED from 25.1

Per 25.1 SPEC §6 + ADR-0080. Format: `"wasm: <field_path>: <reason> [; <forward-pointer hint>]"`. Filter-proto-name prefix `wasm:` invariant on every arm. Constants live as package-private `parseReject*` consts at `internal/filter/http/wasm/compiled_config.go`, returned via `errors.New(parseReject...)` for byte-stability. Kebab-case arm identifiers (used for SPEC cross-reference + test-name suffixes).

### 6.2 25.2 NEW PARSE-REJECT roster (6 arms; final byte-stable wording + arm 27 boot-reject selection settle at IMPL via D-25.2-P1 + D-25.2-P5)

Each arm extends the 25.1 18-arm roster. Anticipated arms (final wording byte-pinned at IMPL via `compiled_config_test.go::TestParseRejectConstants_ByteStable` extension table). The 25.2 IMPL Task NN (analogous to 25.1 Task 9) finalizes the byte-stable wording via D-25.2-P5.

| arm# | arm-name (kebab-case) | trigger condition | byte-exact wording (provisional; settles at IMPL via D-25.2-P5) |
|---|---|---|---|
| 19 | `envoy-go-strict-body-buffer-cap-bytes-zero` | `pc.EnvoyGoStrictBodyBufferCapBytes == 0` (envoy-go-strict-only config field validator) | `"wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"` |
| 20 | `envoy-go-strict-shared-data-value-cap-bytes-zero` | `pc.EnvoyGoStrictSharedDataValueCapBytes == 0` | `"wasm: config.envoy_go_strict_shared_data_value_cap_bytes must be > 0 (envoy-go-strict)"` |
| 21 | `envoy-go-strict-shared-data-max-entries-zero` | `pc.EnvoyGoStrictSharedDataMaxEntries == 0` | `"wasm: config.envoy_go_strict_shared_data_max_entries must be > 0 (envoy-go-strict)"` |
| 22 | `envoy-go-strict-dynamic-stats-max-entries-zero` | `pc.EnvoyGoStrictDynamicStatsMaxEntries == 0` | `"wasm: config.envoy_go_strict_dynamic_stats_max_entries must be > 0 (envoy-go-strict)"` |
| 23 | `envoy-go-strict-body-buffer-cap-bytes-overlarge` | `pc.EnvoyGoStrictBodyBufferCapBytes > 1<<30` (1 GiB ceiling per defense-in-depth — operators wanting >1 GiB body buffers should re-architect with body-streaming rather than buffer-and-process) | `"wasm: config.envoy_go_strict_body_buffer_cap_bytes %d exceeds 1 GiB ceiling (envoy-go-strict)"` |
| 24 | `capability-restriction-config-non-empty-sanitization-rejected-with-rationale` (advisory PARSE-REJECT — per BRAINSTORM Q4 disposition + parent §4.3.5 SanitizationConfig accept-empty discipline INHERITS; this arm is a STAYS — UNCHANGED at 25.2; the 25.1 stance "accept-and-discard non-empty" carries forward. NO PARSE-REJECT arm at 25.2 for SanitizationConfig.) | (NOT a 25.2 arm) | (omit from roster) |
| 25 | `vm-config-runtime-discriminator-deferred` STAYS UNCHANGED FROM 25.1 ARM 11 | (NOT a 25.2-new arm) | (already covered by 25.1 roster) |
| 26 | `cross-pluginconfig-duplicate-pluginconfig-name` | two `PluginConfig` entries within the same listener carry the same non-empty `PluginConfig.name` (collision for per-plugin stat-scope uniqueness — required because the 5-counter envoy-go-strict tri-group bundle keys off `wasm.<plugin_name>.*` + the per-plugin dynamic.Registry keys off pluginScope = `wasm.<plugin_name>`) | `"wasm: config.name %q is duplicated across PluginConfig entries (per-plugin stat-scope uniqueness; envoy-go-strict)"` |
| 27 | `tick-period-when-capability-disabled` (advisory PARSE-REJECT — config-load-time detection that the operator sets `proxy_set_tick_period_milliseconds` capability + the bytecode invokes it OR pre-validates the capability per the operator-runbook discipline; FINAL DISPOSITION: NO config-load PARSE-REJECT — runtime-deny via the default-deny sandbox handles this; the operator gets an integration error log + the `hostcall_denied` counter increment when the bytecode invokes tick without the capability. This arm is REJECTED at SPEC commit per D-25.2-P1 anticipated answer; NOT in the final roster.) | (NOT a 25.2 arm) | (omit from roster) |

**Refined 25.2 NEW arm count: 6 arms (19, 20, 21, 22, 23, 26).** Final selection of the 25.2 BOOT-REJECT fixture-0037 arm settles at IMPL via D-25.2-P1 (anticipated: arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` — distinctive substring `"envoy_go_strict_body_buffer_cap_bytes"`, deterministic config shape, upstream Envoy v1.37.2 has no such field so it does NOT boot-reject the same config; this is an envoy-go-strict-only boot-reject parity arm WITHOUT upstream-equivalent — see §8.2 for the boot-reject fixture-0037 single-arm taxonomy refinement).

### 6.3 Cumulative 25.2 PARSE-REJECT roster: 18 (from 25.1) + 6 (NEW at 25.2) = 24 arms

The full byte-stable wording for the 6 NEW arms pins at IMPL Task NN per D-25.2-P5 carry-forward (analogous to 25.1 D-P5 + Task 9). 25.3 anticipates +6-10 additional arms (per-route TPFC structural validators + multi-plugin VM-sharing semantic arms + environment_variables activation arms). Authoritative 25.3 roster lives in 25.3 SPEC §6.

**Cross-validation:** `TestParseRejectConstants_ByteStable` table at `internal/filter/http/wasm/compiled_config_test.go` is EXTENDED at 25.2 IMPL to include the 6 NEW arms (gold-pinned per ADR-0080).

### 6.4 25.2 BOOT-REJECT fixture-0037 arm (D-25.2-P1 closure at IMPL)

Per D-25.2-P1 + §8.2 + 25.1 D-P6 precedent (where the BOOT-REJECT arm DEVIATED from anticipated arm 5 to substring `"specifier"` based on empirical-scrape evidence at IMPL):

The 25.2 BOOT-REJECT fixture-0037 single-arm selection is DEFERRED to IMPL via empirical-scrape against the candidate arms {19, 20, 21, 22, 23}. Anticipated arm: 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`. Note: arms 19-23 are envoy-go-strict-only validators with NO upstream-Envoy-equivalent (upstream has no such config fields). This means fixture-0037 is BOOT-REJECT-on-envoy-go-only — the reference Envoy side accepts the config (the unknown envoy-go-strict-only fields are silently dropped by upstream's protobuf parser). The BOOT-REJECT fixture asserts envoy-go-strict-stricter behavior — **subject-side BOOT-REJECT only**; reference side BOOTS successfully. Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch), the BOOT-REJECT subject-only branch is a NEW runner variant added at fixture-0037 (or the existing `BootRejectFixture` discipline accommodates subject-only via a new `subjectOnly: true` flag at the fixture definition). 25.2 IMPL Task NN settles the runner-branch shape + the chosen arm + the substring.

---

## 7. Stat surface (9 NEW envoy-go-strict counters + dynamic-stats namespace; project 119 → 128)

Per BRAINSTORM §5 + Q9 + AMEND-B3. The 5-counter 25.1 stat surface per AMEND-A2 STAYS UNCHANGED at 25.2 (carried forward verbatim). The 25.2 EXTENSIONS add 9 envoy-go-strict counters + the open-ended `wasmcustom.<custom_name>` dynamic namespace.

### 7.1 25.2 stat-surface delta — 9 NEW envoy-go-strict counters (119 → 128 at 25.2)

| # | Internal name | Type | Source | Description | AMEND/Q anchor |
|---|---|---|---|---|---|
| 6 | `wasm.<plugin>.tick_invocations` | counter | envoy-go-strict | Increments per `proxy_on_tick` invocation. Operator visibility into tick dispatch rate. | parent SPEC §5.2 + Q5 |
| 7 | `wasm.<plugin>.http_call_dispatched` | counter | envoy-go-strict | Increments per `proxy_http_call` invocation that successfully dispatches to an upstream cluster (cluster lookup OK; AsyncClient request started). | parent SPEC §5.2 + Q4 + AMEND-B3 |
| 8 | `wasm.<plugin>.http_call_response` | counter | envoy-go-strict | Increments per `proxy_on_http_call_response` invocation (response routed to a live stream context). | parent SPEC §5.2 + Q4 |
| 9 | `wasm.<plugin>.foreign_function_denied` | counter | envoy-go-strict | Increments per `proxy_call_foreign_function` invocation that returns `WasmResult::NotFound` (=1) — typically the EMPTY default registry path per AMEND-A9. | parent SPEC §5.2 + AMEND-A9 |
| 10 | `wasm.<plugin>.body_buffer_cap_exceeded` | counter | envoy-go-strict | Increments when accumulated body buffer exceeds `envoy_go_strict_body_buffer_cap_bytes` (default 16 MiB); stream closes with 413 (decode side) or response terminates (encode side); `envoy_go.failures` ALSO increments per §2.25. | Q2 |
| 11 | `wasm.<plugin>.http_call_dispatch_unknown_cluster` | counter | envoy-go-strict | Increments per `proxy_http_call` to unknown cluster (returns BadArgument per upstream + AMEND-B3). | Q4 |
| 12 | `wasm.<plugin>.shared_data_cap_exceeded` | counter | envoy-go-strict | Increments when `proxy_set_shared_data` exceeds value cap (1 MiB default per Q6) OR entry-count cap (1024 default); returns `WasmResult::InternalFailure`; `envoy_go.failures` ALSO increments per §2.25. | Q6 |
| 13 | `wasm.<plugin>.dynamic_stats_cap_exceeded` | counter | envoy-go-strict | Increments when `proxy_define_metric` exceeds dynamic-stats entry cap (1024 default per Q9); the define call returns ErrCapExceeded → `WasmResult::InternalFailure`. | Q9 |
| 14 | `wasm.<plugin>.http_call_response_after_close` | counter | envoy-go-strict | Increments when an outbound HTTP call's response arrives AFTER the originating stream context has been closed (defensive observability for the cancel-at-destruction race per AMEND-B3; near-zero in healthy operation; non-zero signal pages an operator that envoy-go's cancellation path has a bug). | **AMEND-B3 (NEW vs BRAINSTORM Q9 8-counter tally)** |

**9 NEW envoy-go-strict counters at 25.2** (BRAINSTORM Q9 hypothesized 8; AMEND-B3 added counter 14 — `http_call_response_after_close`). Project stat count **119 → 128 at 25.2** (BRAINSTORM hypothesized 127; AMEND-B3 raises to 128).

### 7.2 Stat-prefix template UNCHANGED from 25.1 per AMEND-A2

Per AMEND-A2 + 25.1 SPEC §8 + parent §7. `wasm.<plugin_name>.<stat>` for envoy-go-strict counters; `wasm.<runtime>.<stat>` for Group B upstream-parity (`created` + `active`); HCM-stats_prefix DROPPED structurally per upstream divergence-from-§9-family-pattern. NOT an envoy-go-strict departure — upstream-parity preservation.

### 7.3 Plugin-defined dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2

Per BRAINSTORM Q9 + AMEND-B2 REFINEMENT. The plugin-defined dynamic stats land under `wasmcustom.<custom_name>` (NO plugin prefix in the namespace) per upstream Envoy convention. Per-plugin isolation via per-plugin Registry SCOPE — each `*compiledConfig` constructs its own `*dynamic.Registry` rooted at the per-plugin stat scope (`stats.RootScope.Subscope("wasm").Subscope(pluginName)`); the Registry produces stat names `wasmcustom.<custom_name>` under that scope. From the operator's perspective, the admin `/stats` endpoint enumerates these as `wasm.<plugin_name>.wasmcustom.<custom_name>` (the parent scope's name + the wasmcustom child) — but the in-wire stat name (from the proxy-wasm wire perspective) is `wasmcustom.<custom_name>` byte-faithful to upstream.

**NOT counted in the static stat name total.** Operator-extensible at runtime via `proxy_define_metric`; capped at 1024 entries envoy-go-strict (cap-exceeded → counter 13 + `WasmResult::InternalFailure` per Q9).

**1 envoy-go-strict-only config field at 25.2:** `envoy_go_strict_dynamic_stats_max_entries` (uint32; default 1024).

### 7.4 4 envoy-go-strict-only `PluginConfig` config fields at 25.2

Per Qs 2/6/9 + §4.2. Added to `PluginConfig` as envoy-go-strict-only extensions (NOT in the v1.32.4 binding nor v1.37.2 IDL — envoy-go ships its own ext-PluginConfig wrapper proto OR consumes via Any-encoded extension; final mechanism settles at IMPL Task NN — anticipated: envoy-go-strict-only fields live on the envoy-go-internal `*compiledConfig` after parse, populated from a custom envoy-go protobuf extension OR JSON sidecar; per the project's discipline this stays envoy-go-strict-only with NO upstream impact).

- `envoy_go_strict_body_buffer_cap_bytes` (uint32; default 16777216 = 16 MiB) — Q2
- `envoy_go_strict_shared_data_value_cap_bytes` (uint32; default 1048576 = 1 MiB) — Q6
- `envoy_go_strict_shared_data_max_entries` (uint32; default 1024) — Q6
- `envoy_go_strict_dynamic_stats_max_entries` (uint32; default 1024) — Q9

(Q5's 10ms tick period floor is a compile-time constant per §2.16 + §3.1 `internal/wasm/tick.go`, NOT a config field.)

### 7.5 Project stat-count delta

**119 → 128 at 25.2 (+9 envoy-go-strict counters per §7.1).** 25.3 anticipated additions: 0-4 (Group C `wasm.<plugin>.vm_reload*` IF `failure_policy = FAIL_RELOAD` activates; 0 otherwise per AMEND-A2 + parent §7.3 25.3 forward-pointer). Family-final stat count anticipated **~128-132 at 25.3 phase-done** (was BRAINSTORM-estimated 127-131; revised UPWARD by 1 due to AMEND-B3).

### 7.6 envoy-go-strict departure rationale (consolidated at 25.2 IMPL bundle per §9 + §13.5)

Per AMEND-A2 + AMEND-B3 + Q9. The 9 envoy-go-strict additions at 25.2 are NOT in upstream's stat surface; they are envoy-go-only for operator-visibility into tick/httpCall/foreign-function dispatch rate + cap-exceeded events + late-response-after-close defensive signal. The departure rationale + the BEHAVIOR_CONTRACT departure-record discipline land at the 25.2 IMPL final Task's edit bundle per §9 + §13.5 + ADR-0052 atomic landing. **~6 envoy-go-strict departure records** at 25.2 IMPL bundle: (1) consolidated 5-counter bundle (Qs 2/4/6 counters + parent §5.2 counters + AMEND-B3 counter); (2) body-buffer cap discipline (Q2); (3) shared-data cap discipline (Q6); (4) tick period 10ms floor (Q5); (5) foreign-function 0-vs-10 default registry (AMEND-A9); (6) dynamic-stats cap discipline + `wasmcustom.<custom_name>` namespace (Q9 + AMEND-B2).

---

## 8. Differential fixture taxonomy (fixture-0036 mixed-mode + fixture-0037 boot-reject subject-only)

Per BRAINSTORM §6 + Q8 + parent SPEC §8.3 25.2 forward-pointer + `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch) + `reference_differential_asserter_dispatch` (cross-side fixtures use StatsAsserter for subject-side stat assertions; deliberate-break liveness verification mandatory). Fixture-dir count **37 → 39 at 25.2** (+2: `0036-http-wasm-body-and-advanced` + `0037-http-wasm-body-and-advanced-boot-reject`).

### 8.1 Fixture `0036-http-wasm-body-and-advanced` (single-listener mixed-mode per Q8 + ADR-0192 precedent)

Single-listener single-HCM hosting the wasm filter + router terminator. httpCall scenarios use a SECOND upstream cluster definition (NOT a second listener — avoids `freeTCPPort` flake per phase-22.2 REVIEW §7.4). 12-14 scenarios partitioned by assertion-class (8-10 deterministic cross-side via `CompareBytes` + 3-4 non-deterministic subject-only via `StatsAsserter.AssertStats`).

#### 8.1.1 Scenario taxonomy (12-14 scenarios)

**Deterministic — cross-side `CompareBytes` (8-10 scenarios):**

| # | Name | Plugin behavior | Wire assertion |
|---|---|---|---|
| (a) | body-read-only | `proxy_on_request_body(ctx, size, end_of_stream)` reads body via `proxy_get_buffer_bytes`; logs size via `proxy_log(INFO, ...)`; CONTINUE | Reflected body unchanged at upstream |
| (b) | body-mutate-passthrough | Read body; modify (e.g., uppercase first 16 bytes) via `proxy_set_buffer_bytes`; CONTINUE | Reflected body modified byte-exactly |
| (c) | body-mutate-replace | Replace full body in `proxy_on_request_body(end_of_stream=true)` via `proxy_set_buffer_bytes(start=0, size=existing, data=new)`; CONTINUE | Reflected body replaced |
| (d) | trailers-add | `proxy_add_header_map_value(HttpRequestTrailers, "x-trailer-added", "yes")` in `proxy_on_request_trailers` | Reflected trailer present at upstream |
| (e) | trailers-read | Read trailer count via `proxy_get_header_map_size(HttpRequestTrailers, ...)` + add response header `x-trailer-count: N` | Reflected `x-trailer-count` header |
| (f) | shared-data-read-after-write | Stream 1 writes `proxy_set_shared_data("key", "value", 0)`; Stream 2 reads `proxy_get_shared_data("key")` and echoes via response header `x-shared-value` | Stream 2 response has `x-shared-value: value`; CAS conflict path tested via additional probe |
| (g) | foreign-function-deny-default | `proxy_call_foreign_function("verify_signature", ...)` returns NotFound (=1) (per AMEND-A9 EMPTY default registry); plugin records WasmResult into response header `x-ff-result: 1` | Reflected `x-ff-result: 1` (subject-side `foreign_function_denied` counter ALSO incremented; StatsAsserter ALSO asserts the counter delta) |
| (h) | property-stream-info | `proxy_get_property("request.method")` → response header `x-prop-method`; `proxy_get_property("response.code")` → `x-prop-code`; `proxy_get_property("upstream.cluster.name")` → `x-prop-cluster`; `proxy_get_property("connection.tls.version")` → `x-prop-tls` (the TLS branch sources empty bytes on plaintext per AMEND-B4 fall-through) | Reflected `x-prop-*` headers byte-exact (scenarios chosen with deterministic property values) |
| (i) | metric-define-only | `proxy_define_metric(COUNTER, "my_counter")` at `proxy_on_configure` (root context); NO Increment/Record | Subject-side: dynamic stat `wasmcustom.my_counter` appears at `/stats` with value 0; cross-side: response unchanged (deterministic) |
| (j) | env-vars-rejected-passthrough | (Verifies 25.3-deferred `VmConfig.environment_variables` PARSE-REJECT at 25.2; plugin reads via `wasi_snapshot_preview1.environ_*` and gets zeros) | Reflected response with `x-env-count: 0` |

**Non-deterministic — subject-only `StatsAsserter.AssertStats` (3-4 scenarios):**

| # | Name | Plugin behavior | Subject-side assertion |
|---|---|---|---|
| (k) | tick-fires-counter | `proxy_on_configure` sets 50ms period via `proxy_set_tick_period_milliseconds(50)`; `proxy_on_tick` increments custom dynamic stat `wasmcustom.tick_count` (via `proxy_define_metric` at configure + `proxy_increment_metric` at tick) | After 250ms probe wait, subject's `wasm.<plugin>.tick_invocations` counter ≥ 5; `wasmcustom.tick_count` value ≥ 5 |
| (l) | httpCall-success | `proxy_on_request_headers` invokes `proxy_http_call("upstream_cluster_b", headers={":path":"/echo",":method":"GET",":authority":"echo.local"}, body=nil, trailers=nil, timeout_ms=5000)`; `proxy_on_http_call_response` adds response header `x-httpcall-status: <upstream_status>` to the original stream | Subject: `wasm.<plugin>.http_call_dispatched` + `http_call_response` both increment; response header `x-httpcall-status: 200` present (timing non-deterministic vs reference) |
| (m) | httpCall-unknown-cluster | `proxy_http_call("nonexistent_cluster", ...)` | Subject: `wasm.<plugin>.http_call_dispatch_unknown_cluster` increments; response header `x-httpcall-result: 2` (BadArgument) |
| (n) | body-cap-exceeded | Plugin returns PAUSE indefinitely in `proxy_on_request_body`; subject probes with 32 MiB body (exceeds 16 MiB envoy-go-strict cap) | Subject: stream closes with HTTP 413 "Payload Too Large"; `wasm.<plugin>.body_buffer_cap_exceeded` counter ≥ 1; `wasm.<plugin>.envoy_go.failures` counter ≥ 1 (per §2.25 scope extension) |

**Total: 14 scenarios** (10 deterministic + 4 non-deterministic). Final count may trim to 12 at IMPL if any scenario surfaces empirically-impossible-to-byte-pin behavior (e.g., scenario (h) connection.tls.version on plaintext may have empty-bytes vs literal "" divergence between wazero and V8; if so, MOVE the scenario to subject-only OR delete).

#### 8.1.2 Topology

ONE listener (port `:templated{{.SubjectListenerPort}}`) + HCM with the wasm filter (alphabetical position; UNCHANGED at 20 HTTP filters per parent SPEC §3.1) + router terminator. TWO upstream clusters: `cluster_a` (primary; bound to the differential echobackend) + `cluster_b` (httpCall target; bound to the SAME echobackend at a different cluster definition — `freeTCPPort` flake mitigation per phase-22.2 REVIEW §7.4). httpCall scenarios (l) + (m) dispatch to `cluster_b` (or non-existent for (m)). The runner probes against `cluster_a` (the primary stream); the httpCall fires from inside the per-stream wasm dispatch.

#### 8.1.3 §4.5 D6 guardrail compliance

All 14 scenarios comply with parent SPEC §4.5 D6 guardrails: (a) no memory-trap probes; (b) HTTP/1.1 transport; (c) no float-formatted log lines (`proxy_log` payloads are byte-exact-equivalent); (d) all hostcalls within the 25.2 active 30-hostcall env-namespace + 8-WASI surface (per §5.5).

#### 8.1.4 Asserter dispatch discipline (per `reference_differential_asserter_dispatch`)

Cross-side scenarios (a)-(j) use existing `CompareBytes` runner branch. Scenarios (g) ALSO uses `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)` for the subject-side counter delta assertion (foreign_function_denied — even though scenario (g) IS cross-side via the response-header echo, the subject-side stat delta is meaningful additional coverage). Non-deterministic scenarios (k)-(n) use ONLY `StatsAsserter.AssertStats` (subject-only — no reference-side comparison; reference Envoy v1.37.2's wire output for these scenarios is non-deterministic vs envoy-go's timing). Per `reference_differential_asserter_dispatch` memo: every subject-only StatsAsserter arm gets a deliberate-break liveness verification (deliberately break the stat assertion → expect FAIL → restore + verify GREEN). Mandatory at IMPL Task NN (analogous to 25.1 Task 15+17 follow-up which surfaced + addressed the dead-vacuous-assertion risk).

#### 8.1.5 Recommended fixture-0036 directory structure (per parent §8.1.3 + 25.1 fixture-0034 precedent)

```
test/fixtures/0036-http-wasm-body-and-advanced/
  README.md             # ~200-300 lines: scope + 14-scenario table + topology + cross-refs to SPEC §8 + ADR-0205+0206+0207+0208
  envoy.yaml            # reference Envoy bootstrap; single listener + wasm filter + 2 upstream clusters; templated {{.BackendPort}} + {{.UpstreamBPort}}
  envoy-go.yaml         # subject bootstrap; same topology + envoy-go-strict-only PluginConfig extension fields populated
  expectations.yaml     # human-readable declarative scenario expectations (NOT consumed by runner)
  inputs/
    driver.go           # registered Driver impl (~800-1200 LoC); per-scenario probes + classifyBody + StatsAsserter.AssertStats implementations
  scripts/              # per-scenario Rust source + reproduction build script
    a_body_read_only/Cargo.toml + src/lib.rs
    b_body_mutate_passthrough/Cargo.toml + src/lib.rs
    c_body_mutate_replace/Cargo.toml + src/lib.rs
    d_trailers_add/Cargo.toml + src/lib.rs
    e_trailers_read/Cargo.toml + src/lib.rs
    f_shared_data_rw/Cargo.toml + src/lib.rs
    g_foreign_function_deny/Cargo.toml + src/lib.rs
    h_property_stream_info/Cargo.toml + src/lib.rs
    i_metric_define_only/Cargo.toml + src/lib.rs
    j_env_vars_rejected/Cargo.toml + src/lib.rs
    k_tick_fires_counter/Cargo.toml + src/lib.rs
    l_http_call_success/Cargo.toml + src/lib.rs
    m_http_call_unknown/Cargo.toml + src/lib.rs
    n_body_cap_exceeded/Cargo.toml + src/lib.rs
    README.md           # reproduction script + pinned rustup toolchain + cargo build invocation
  bytecode/             # vendored pre-built .wasm files (binary blobs committed to git)
    a_body_read_only.wasm
    b_body_mutate_passthrough.wasm
    ...                 # one per scenario (a)-(n)
```

NEW `BackendKind=HTTPWasmAdvanced` constant added at `test/differential/runner_test.go` (alternative: REUSE the existing `HTTPWasm` constant from 25.1 — settles at IMPL Task NN; anticipated NEW constant per the per-fixture-dir-1-runner-branch discipline). The `cluster_b` second-upstream-cluster definition is constructed via an existing test-infra helper (extend the existing single-cluster bootstrap-generator with a `withSecondCluster: true` flag).

### 8.2 Fixture `0037-http-wasm-body-and-advanced-boot-reject` (subject-only boot-reject per D-25.2-P1)

`0037-http-wasm-body-and-advanced-boot-reject`: subject-only PGV-mirror boot-reject for a 25.2-new envoy-go-strict-only PARSE-REJECT arm. Anticipated arm (per D-25.2-P1 closure at IMPL Task NN): **arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero`** with distinctive substring `"envoy_go_strict_body_buffer_cap_bytes"`. The reference Envoy side does NOT boot-reject the same config (upstream has no such field; the unknown extension field is silently dropped by upstream's protobuf parser).

**Runner-branch shape decision at IMPL Task NN (anticipated):** the existing `BootRejectFixture` runner branch ASSUMES both reference + subject fail to boot + share a common stderr substring; the 25.2-new envoy-go-strict-only boot-reject is **subject-only** (reference boots successfully). The runner branch SHAPE either: (a) extend `BootRejectFixture` to accept a `subjectOnly: true` flag (recommended — minimal infrastructure delta; preserves the one-fixture-dir-one-runner-branch invariant); OR (b) introduce a NEW `SubjectOnlyBootRejectFixture` runner branch (heavier — but cleaner type-discriminated dispatch). Final disposition settles at IMPL Task NN.

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch — applies to fixture-0037 boot-reject single-runner-branch invariant).

### 8.3 Fixture-dir count

37 → 39 at 25.2 phase-done (+2: `0036` + `0037`). 25.3 anticipated additions: +2 (fixture-0038 cross-side per-route + multi-plugin + fixture-0039 boot-reject) per parent §8.5. **Total +2 at 25.2.**

### 8.4 35th project-wide fuzzer `FuzzWasmHostcallEnvelope`

Lands at 25.2 IMPL per BRAINSTORM §6.4 + ADR-0206. Adversarial corpus seeds (~30-40 seeds per ADR-0018 baseline) covering:
- Hostcall argument-envelope edge cases (wasm linear memory pointer/size bounds; max-size buffer reads; clamp-vs-BadArgument boundary per AMEND-B1)
- proxy-wasm pairs serialization adversarial inputs (malformed key/value sizes; truncated wire bytes; reused from 25.1 `pairs.go` corpus + extended for trailer-map use)
- Foreign-function call name length boundary cases (name=empty; name>1024 bytes)
- Dynamic-stats name validation (UTF-8 + length + collision; cap-boundary at 1024 entries)
- Shared-data CAS-mismatch race patterns (concurrent set/get under cap)
- Body-buffer cap boundary cases (exactly-at-cap; one-byte-over-cap; per AMEND-B1 clamp)
- Property-path syntax adversarial inputs (malformed NUL-delimited paths; empty segments; >MAX_PATH depth; per AMEND-B4)
- Tick period parsing (negative; > i64 max; 0; below 10ms floor per Q5)
- httpCall envelope (cluster_name empty; headers wire malformed; timeout=0; timeout>i32 max)
- Metric type out-of-range (MetricType=99 → expect ErrBadArgument); signed-i64 delta extremes (delta=int64 min/max per AMEND-B2)

**Must-never-panic invariant:** any of these inputs to the host-side hostcall dispatch MUST NOT crash the envoy-go process (must return WasmResult error code + log + continue). Project fuzzer count: **34 → 35 at 25.2 phase-done.**

### 8.5 D-S2 sub-pin closure at this 25.2 SPEC commit (per parent §13-R5 lineage)

Per parent §13-R5 + 25.1 §11.1 D-S1 RATIFIED at IMPL (34-fuzzer count confirmed). **D-S2 (35th-fuzzer count VERIFIED at this SPEC commit; per phase-25.1 §11.1 D-S1 precedent):**

```
$ cd /home/esa/git/envoy-go && grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path './.worktrees/*' -not -path './.claude/*') | wc -l
34
```

**Disposition:** CONFIRMED at this 25.2 SPEC commit. ADR-0206 §Decision body + 25.2 BEHAVIOR_CONTRACT.md §13.4 patch pin to **35** at 25.2 IMPL final Task. R5-equivalent at 25.2 CLOSED.

---

## 9. Behavior-contract delta (semantic; ~6 NEW departure records at 25.2)

Per BRAINSTORM §5.3 + Q9 + Qs 2/4/5/6 + AMEND-A9 + AMEND-B3. The 25.2 IMPL final-Task BEHAVIOR_CONTRACT.md ~7-edit bundle lands at IMPL Task NN per ADR-0052 atomic landing (see §13.5). The high-level semantic deltas at 25.2:

1. **Full advanced-bridge surface ACTIVATED.** The 24-hostcall 25.1 surface grows to 30 active env-namespace hostcalls (+ 8 WASI carry-forward unchanged) at 25.2. Body+buffer hostcalls active per AMEND-B1 clamp-on-overflow semantic; trailer hostcalls reuse 25.1 header-map family with WasmHeaderMapType values 1/3 activated; timer hostcall + 10ms envoy-go-strict floor active; metric hostcalls with `wasmcustom.<custom_name>` namespace per AMEND-B2; shared-data hostcalls with CAS + envoy-go-strict caps active per Q6; httpCall hostcall active + cancel-at-destruction discipline per AMEND-B3; foreign-function hostcall with EMPTY default registry per AMEND-A9. Observable: operator-authored `.wasm` plugins at 25.2 IMPL phase-done invoke any hostcall from the 38-hostcall active surface (gated by the default-deny capability sandbox per AMEND-A5 + the gate-at-registration discipline per AMEND-B5).

2. **Body-buffer cap discipline envoy-go-strict departure** (per Q2 + ADR-0208). 16 MiB envoy-go-strict default cap on accumulated body buffer; operator-configurable via `envoy_go_strict_body_buffer_cap_bytes`. On cap exceeded: 413 Payload Too Large (decode side) or response terminate (encode side); `wasm.<plugin>.body_buffer_cap_exceeded` counter + `envoy_go.failures` counter (per §2.25 scope extension) + integration error log. Rationale: upstream Envoy has no in-filter cap; relies on HCM-level + listener-level memory ceilings. envoy-go-strict adds defense-in-depth against PAUSE-loop guest patterns. envoy-go-strict departure record #2 at 25.2 BEHAVIOR_CONTRACT.md.

3. **Shared-data cap discipline envoy-go-strict departure** (per Q6 + ADR-0208). 1 MiB value cap + 1024-entry cap; both operator-configurable. On cap exceeded: `WasmResult::InternalFailure` + counter + log. Rationale: cap defends against unbounded namespace creation via guest loop pattern. envoy-go-strict departure record #3 at 25.2 BEHAVIOR_CONTRACT.md.

4. **Tick period 10ms floor envoy-go-strict departure** (per Q5 + ADR-0205 + ADR-0208). `proxy_set_tick_period_milliseconds(period < 10)` is silently clamped to 10ms host-side. Rationale: prevents guest-driven CPU spin attacks (period=0 → hot loop). Operators with legitimate sub-10ms timer use cases are NOT supported at envoy-go (defensive scope-narrowing per the safety-first defaults pattern). envoy-go-strict departure record #4 at 25.2 BEHAVIOR_CONTRACT.md.

5. **Foreign-function 0-vs-10 default registry envoy-go-strict departure** (per AMEND-A9 + ADR-0206 + ADR-0208). Upstream registers 10 foreign functions by default (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`); envoy-go registers ZERO. Operators MUST explicitly enable the `proxy_call_foreign_function` capability AND register specific foreign functions via `wasm.RegisterForeignFunction(name, fn)` at boot; unregistered names return `WasmResult::NotFound` byte-faithful to upstream per AMEND-A9. envoy-go-strict departure record #5 at 25.2 BEHAVIOR_CONTRACT.md.

6. **Dynamic-stats cap discipline + namespace clarification envoy-go-strict departure** (per Q9 + AMEND-B2 + ADR-0208). 1024-entry cap on `wasmcustom.<custom_name>` dynamic-stats namespace; cap-exceeded counter + log. envoy-go-strict-only config field `envoy_go_strict_dynamic_stats_max_entries`. Namespace shape per AMEND-B2: `wasmcustom.<custom_name>` only (NO plugin prefix in the namespace); per-plugin isolation via per-plugin Registry SCOPE (operator sees `wasm.<plugin>.wasmcustom.<custom_name>` at admin /stats by virtue of the parent stat-scope nesting). envoy-go-strict departure record #6 at 25.2 BEHAVIOR_CONTRACT.md.

7. **9-counter envoy-go-strict tally consolidation** (per AMEND-B3 + Q9). 9 NEW envoy-go-strict counters land at 25.2 (8 from BRAINSTORM Q9 + 1 from AMEND-B3 `http_call_response_after_close`). Consolidated into the 25.2 BEHAVIOR_CONTRACT.md envoy-go-strict departures table as a single bundle entry. Operator-visibility rationale: tick rate + httpCall rate + foreign-function denial rate + cap-exceeded events + late-response-after-close defensive signal. Bundled with the 4 envoy-go-strict-only `PluginConfig` config fields. envoy-go-strict departure record #1 at 25.2 BEHAVIOR_CONTRACT.md (the 5-counter consolidated bundle from BRAINSTORM §5.3 IS this record #1 — but AMEND-B3 expanded it from 5 to 9 counters; the 9-counter bundle is record #1; records #2-#6 are the Q2/Q5/Q6/A9/Q9 cap + floor + registry + namespace records).

8. **Buffer-bounds CLAMP wire-contract** (per AMEND-B1). `proxy_get_buffer_bytes` clamps on overflow (returns Ok with truncated length) byte-faithful to cpp-host reference implementation; the spec README text saying BAD_ARGUMENT is REFINED here. NOT an envoy-go-strict departure (upstream-parity preservation against the reference host). Recorded at BEHAVIOR_CONTRACT.md `### envoy.filters.http.wasm` subsection 25.2 EXTENSION as a wire-shape note (NOT a departure record).

9. **NUL-delimited property path serialization** (per AMEND-B4). `proxy_get_property` accepts NUL-delimited byte segments (e.g., `request\0headers\0x-foo`) per spec README + upstream context.cc; the segment count + sub-path roster pins per AMEND-B4. NOT an envoy-go-strict departure (upstream-parity preservation). Recorded at BEHAVIOR_CONTRACT.md as a wire-shape note + the ~70-sub-path roster (with cross-refs to RE-CONSUMED primitives ADR-0144 + ADR-0177 + ADR-0190 + NEW ADR-0207).

10. **Cancel-at-destruction discipline for outbound HTTP calls** (per AMEND-B3). In-flight `proxy_http_call` requests are cancelled at the originating stream context's `OnDestroy` (cpp-host byte-faithful); the defensive `http_call_response_after_close` counter increments if a stray response slips through (operationally near-zero; non-zero pages an operator). NOT an envoy-go-strict departure on the cancel side (upstream-parity); the counter IS an envoy-go-strict observability extension (recorded in departure record #1 consolidated counter bundle).

---

## 10. ADR anchor map (4 NEW §Context drafts at 25.2 SPEC commit; ADR-0202 one-line in-place AMEND at IMPL; ADR-0209 reserve; ZERO ADR-0125 amendments)

Per ADR-0044 §Context-draft discipline + BRAINSTORM Q10 strict-scope. The ADR-0205..ADR-0208 §Context drafts anchor at THIS 25.2 SPEC commit (appended to `DECISIONS.md`); §Decision + §Consequences bodies land at 25.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.

### 10.1 25.2 NEW ADRs (ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0205** | Root VM lifecycle evolution per Q3 — ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per cpp-host `Wasm`/`Plugin` model); per-stream contexts as CHILDREN sharing wazero Runtime+Module; tick + httpCall + shared-data state at root; per-`*RootVM` tick goroutine + 10ms envoy-go-strict period floor + Clock seam FIRST co-consumer; per-stream Module instantiation pattern (fresh vs pooled vs shared) deferred to 25.2 IMPL R8 escape-valve at the > 1ms threshold (D-25.2-P2 + parent §13-R8 carry-forward) | §1; §1.1 AMEND-B5 gate-at-registration; §2.16 + §2.17 + §2.22 + §2.25 + §2.27; §3.1; §5.3 (callbacks C18 + C19 + C20 root-context dispatch) | 25.2 IMPL first task that materializes `internal/wasm/root_vm.go` |
| **ADR-0206** | 25.2 ABI extensions — ~14 NEW env-namespace hostcalls + ~7 NEW ABICallbacks methods + buffer-clamp wire-contract pin per AMEND-B1 + metric signedness pin per AMEND-B2 + `internal/wasm/foreign.go` ForeignFunctionRegistry with EMPTY default registry per AMEND-A9 + 21 NEW capability keys per AMEND-B5 + gate-at-registration architectural pin per AMEND-B5 + 25.2 PARSE-REJECT roster extension (~6 NEW arms per §6) + full ~70-path property roster per AMEND-B4 | §1.1 AMEND-A9 + AMEND-B1 + AMEND-B2 + AMEND-B4 + AMEND-B5; §3.1 + §3.4 REUSEs 3-6; §5 (full hostcall + callback delta); §6 (PARSE-REJECT extension); §11.X D-25.2-1 + D-25.2-2 + D-25.2-3 + D-25.2-4 + D-25.2-5 | 25.2 IMPL first task that lands body/buffer hostcalls (anticipated Task NN per the production file split + ABI-version-detection-paired-first-task precedent) |
| **ADR-0207** | NEW `internal/filterstate/` framework primitive at 25.2 second-consumer scope per Q7 + EXTRACT-NOW-on-second-consumer discipline + consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking + `upstream_filter_state` distinct root co-equal to `filter_state` per AMEND-B4 + future-consumer roster (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families) + ADR-0188 API-revision allowance NOT consumed (the `internal/lua/` framework primitive itself is untouched; only the in-package filterstate.go file migrates) + EXPLICIT API-REVISION ALLOWANCE clause for consumer #3+ | §1.1 AMEND-B4 upstream_filter_state addition; §2.20 (lua migration scope); §3.2 (primitive API + Bucket + FilterStateObject + StateType); §3.4 MIGRATES | 25.2 IMPL first task that materializes `internal/filterstate/` + migration follow-up task in `internal/filter/http/lua/` |
| **ADR-0208** | NEW `internal/filter/http/wasm/` 25.2 package extensions — full hostcall wiring per §3.6 + 9 envoy-go-strict counters per Q9 + AMEND-B3 (counter 14 `http_call_response_after_close` per AMEND-B3 recommendation) + 4 envoy-go-strict-only `PluginConfig` config fields per Qs 2/6/9 + dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2 via NEW `internal/stats/dynamic.go` infrastructure + mixed-mode fixture-0036 discipline per Q8 + fixture-0037 subject-only boot-reject per D-25.2-P1 + 25.2 BEHAVIOR_CONTRACT.md ~7-edit bundle per ADR-0052 + 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + per-plugin Registry scope discipline | §1.1 AMEND-B2 + AMEND-B3; §3.3 + §3.6; §4 compiledConfig + filterStats; §5 (cross-ref ADR-0206); §6 (cross-ref); §7; §8; §9; §13.5 | 25.2 IMPL Final Task atomic landing |
| **ADR-0209** | (Escape-valve reserve at 25.2 — STRENGTHENED-WEAK-HOLD-with-1-slot per Q10 + §1.2 disposition) — likely candidate: per-stream Module instantiation R8 escape-valve if 25.2 IMPL `BenchmarkPerStreamModule_Instantiation` surfaces > 1ms (D-25.2-P2 + parent §13-R8 threshold); secondary candidate: SPEC-time empirical-discovery surface in any 25.2 hostcall implementation (e.g., wazero CompilationCache eviction semantic edge case; pairs wire-format buffer-bounds error class refinement; foreign-function dispatch concurrency model surfaces architectural-but-implementation-detail decisions warranting their own ADR per D-25.2-P3) | §1.2; §2.16; §3.1 Module instantiation R8 escape-valve discipline; §12 D-25.2-P2 + D-25.2-P3 | Only consumed if escape-valve fires; otherwise carries forward to 25.3 BRAINSTORM as the 25.3 IMPL escape-valve slot per R8 signaling protocol |

### 10.2 ADR-0202 one-line in-place AMEND acknowledgment (per Q10 strict-scope precedent + ADR-0044 in-place edit discipline)

ADR-0202 gains a **one-line acknowledgment paragraph** in §Consequences. NO new ADR number consumed for the in-place AMEND. The acknowledgment paragraph (provisional wording; settles at 25.2 IMPL final Task):

> *"Phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern carries forward to 25.2 IMPL R8 escape-valve). The EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent."*

Lands at 25.2 IMPL final Task per ADR-0044 in-place edit discipline (the §Consequences section of ADR-0202 in DECISIONS.md gains the paragraph; the `**Status:**` line gains an AMEND timestamp note `; AMENDED 2026-MM-DD per phase-25.2 one-line acknowledgment in §Consequences`).

### 10.3 25.3 anticipated ADRs (forward-pointer; UNCHANGED from parent SPEC §10.3 + 25.1 SPEC §10.3)

| ADR | Anticipated subject |
|---|---|
| **ADR-0210** (or +1 if ADR-0209 fires at 25.2) | Per-route Wasm 5th-canonical REUSE-by-absence EXPLICIT-NO-NEW-CANONICAL classification per AMEND-A3 (analogous to ADR-0173 / ADR-0180). ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment. |
| **ADR-0211** (or +1) | Multi-plugin VM-sharing semantics — `vm_id`-keyed VM reuse + plugin-context isolation discipline + cross-plugin shared-data scoping (lifting the 25.2 per-PluginConfig shared-data scope). |
| **ADR-0212** (or +1) | `test/conformance/proxy-wasm/` conformance harness seed + pin SHA `proxy-wasm-cpp-host@da3ce05d` per AMEND-A8 + 10-of-16 test family port + 62.5% starting pass-threshold. |
| **ADR-0213** (or +1; reserve at 25.3) | Escape-valve reserve for any 25.3-IMPL-time-unanticipated surface. |

### 10.4 ZERO IN-PLACE §Decision AMENDMENTs + ZERO ADR-0125 amendments at 25.2

Per AMEND-A3 STAYS-DEFINITIVE: ADR-0125's canonical roster STAYS at 10 entries — **NO §(xvi) amendment** at phase 25.3 IMPL. The BRAINSTORM-anticipated escape-valve "ADR-0125 amendment 10 → 11" is RETIRED at parent SPEC commit + INHERITED at 25.2 SPEC. No other in-place ADR §Decision body amendments anticipated at 25.2 (only the ADR-0202 §Consequences one-line acknowledgment per §10.2).

### 10.5 Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts at 25.2 SPEC commit (this commit) | 4 | ADR-0205; ADR-0206; ADR-0207; ADR-0208 |
| Anticipated NEW ADRs at 25.3 | 3 | ADR-0210 (or +1); ADR-0211 (or +1); ADR-0212 (or +1) |
| In-place §Consequences AMEND at 25.2 IMPL | 1 (one-line; no new ADR number) | ADR-0202 |
| ADR-0044 escape-valve reserve at 25.2 IMPL | 0-1 | ADR-0209 reserved (consumed only if R8 escape-valve fires per D-25.2-P2) |
| ADR-0125 amendments | 0 (RETIRED per AMEND-A3) | NONE |
| In-place §Decision AMENDMENTs at 25.2 SPEC | 0 | NONE |

**Next-free ADR post-25.2-SPEC commit: `ADR-0209`** (4 NEW §Context drafts consumed: ADR-0205..ADR-0208). Anticipated next-free after 25.2 phase-done: **`ADR-0210`** if reserve UNCONSUMED; **`ADR-0211`** if reserve consumed (the 25.3 anticipated ADRs would then start at +1 from the consumed slot). Anticipated next-free after 25.3 phase-done: **`ADR-0213`** if all reserves UNCONSUMED; **`ADR-0214` or ADR-0215`** if reserves consumed.

---

## 11. Empirical-pin block (D-25.2-1..D-25.2-5 resolved at this SPEC session)

This block contains the parallel-subagent-fan-out scrape evidence executed during this 25.2 SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors the parent SPEC §11 9-pin block structure but scoped to the 25.2-specific surface. **Probe date: 2026-05-25.** The 5 pins are 25.2-specific (the 9 parent-SPEC pins span all 3 sub-phases + are NOT re-executed here; they apply at 25.2 verbatim).

**Reference source corpus** (multi-axis verification per the phase-15..25.1 discipline):

1. **`proxy-wasm/spec@main:abi-versions/v0.2.1/README.md`** via WebFetch — the 63KB proxy-wasm v0.2.1 ABI specification document. Authoritative for hostcall wire signatures + enum integer values + dispatch semantics.
2. **`proxy-wasm/proxy-wasm-cpp-host@main` reference host implementation** via WebFetch (SHA pinning: `da3ce05d8d59ebccbfcad434bb4784c98a4ece6a` per parent §11.5 D4): `include/proxy-wasm/{wasm.h, wasm_vm.h, exports.h, bytecode_util.h}`; `src/{wasm.cc, exports.cc}`. Authoritative for buffer-clamp behavior + capability-gate site + foreign-function dispatch shape + httpCall cancel-at-destruction.
3. **`envoyproxy/envoy@v1.37.2` C++ source** via WebFetch: `source/extensions/common/wasm/{context.cc, context.h, stats_handler.h}`; `source/extensions/filters/common/expr/context.h` (the authoritative property tree); `source/extensions/filters/http/wasm/{wasm_filter.h, config.h}`. Authoritative for property-tree roster + dynamic-stats namespace + httpCall cluster-lookup + late-response cancellation.
4. **`proxy-wasm/proxy-wasm-rust-sdk@v0.2.4`** via WebFetch: `src/{lib.rs, hostcalls.rs, types.rs}`. Guest-side verification of hostcall signatures + MetricType enum + Status enum.

### Summary disposition table (5 pins → 5 AMEND-B entries)

| Pin | Topic | Disposition | AMEND cross-ref |
|---|---|---|---|
| §11.1 | D-25.2-1 — Body+buffer+trailers hostcall signatures + buffer-bounds error semantic | CONFIRMS signatures + REFINES buffer-bounds semantic (spec README says BAD_ARGUMENT; cpp-host CLAMPS silently — envoy-go MUST mirror cpp-host); trailer hostcalls REUSE 25.1 header-map family with values 1/3 activated | AMEND-B1 |
| §11.2 | D-25.2-2 — Metrics hostcall signatures + MetricType enum + dynamic-stats namespace | CONFIRMS enum + namespace prefix; REFINES `proxy_increment_metric` delta is SIGNED `int64` (NOT unsigned); REFINES dynamic-stats namespace to `wasmcustom.<custom_name>` only (NO plugin prefix as BRAINSTORM Q9 hypothesized) | AMEND-B2 |
| §11.3 | D-25.2-3 — proxy_http_call wire shape + response routing + late-response disposition | CONFIRMS BadArgument-on-unknown-cluster; REFINES late-response handling (cpp-host cancels at destruction + has defensive token-miss guard; NOT silent drop); RECOMMEND envoy-go-strict counter `http_call_response_after_close` for defensive observability | AMEND-B3 (RAISES envoy-go-strict counter tally 8 → 9) |
| §11.4 | D-25.2-4 — Full proxy_get_property roster + path serialization | CONFIRMS NUL-delimited wire format; SUBSTANTIVE REFINEMENT of roster: ~10 dispatched roots + 4 direct tokens (NOT ~25 as BRAINSTORM Q7 hypothesized); `xds.*` CONSOLIDATES listener+route+cluster; `upstream_filter_state` is DISTINCT root co-equal to `filter_state` | AMEND-B4 |
| §11.5 | D-25.2-5 — Capability roster extensions for 25.2 | CONFIRMS key formats; STRUCTURAL REFINEMENT of gate location (gate-at-registerCallback time in wasm.cc; NOT gate-at-call-site in exports.cc); 21 NEW keys at 25.2 (14 hostcall + 7 lifecycle); post-25.2 cumulative roster 58 keys | AMEND-B5 |

### 11.1 D-25.2-1 — Body+buffer+trailers hostcall signatures (driver: ADR-0004 hard-gate)

**Disposition:** REFINES BRAINSTORM Q1 anticipation — signatures CONFIRM, but buffer-bounds error semantic REFUTES the strict-BAD_ARGUMENT reading: the v0.2.1 spec text says `BAD_ARGUMENT`, while the proxy-wasm-cpp-host reference implementation **clamps silently** (returns `Ok` with truncated length).

**Hostcalls pinned (line-numbered citations):**

| # | Hostcall | Args (full type list) | Return | Source pin |
|---|---|---|---|---|
| 1 | `proxy_get_buffer_bytes` | `i32 buffer_id, i32 start, i32 max_size, i32 *return_value_data, i32 *return_value_size` | `i32 proxy_status_t` | `proxy-wasm/spec@main:abi-versions/v0.2.1/README.md` §Functions exposed by the host / Buffer; cpp-host `include/proxy-wasm/exports.h:107` |
| 2 | `proxy_set_buffer_bytes` | `i32 buffer_id, i32 start, i32 size, i32 *value_data, i32 value_size` | `i32 proxy_status_t` | spec README §Buffer; cpp-host `exports.h:109` |
| 3 | `proxy_get_buffer_status` | `i32 buffer_id, i32 *return_buffer_size, i32 *return_unused` | `i32 proxy_status_t` | spec README §Buffer; cpp-host `exports.h:108` |
| 4 | `proxy_on_request_body` (guest) | `i32 stream_context_id, i32 body_size, i32 end_of_stream` | `i32 proxy_action_t` | spec README §HTTP |
| 5 | `proxy_on_response_body` (guest) | `i32 stream_context_id, i32 body_size, i32 end_of_stream` | `i32 proxy_action_t` | spec README §HTTP |
| 6 | `proxy_on_request_trailers` (guest) | `i32 stream_context_id, i32 num_trailers` | `i32 proxy_action_t` | spec README §HTTP |
| 7 | `proxy_on_response_trailers` (guest) | `i32 stream_context_id, i32 num_trailers` | `i32 proxy_action_t` | spec README §HTTP |

C++ host declarations cross-confirm (`include/proxy-wasm/exports.h:107-109`): `Word get_buffer_bytes(Word type, Word start, Word length, Word ptr_ptr, Word size_ptr); Word get_buffer_status(Word type, Word length_ptr, Word flags_ptr); Word set_buffer_bytes(Word type, Word start, Word length, Word data_ptr, Word data_size);`. Rust SDK v0.2.4 `src/hostcalls.rs:73-79, 110-116` confirms `usize`/`*mut u8` mapping.

**Buffer-bounds error semantic pin (SOURCE DIVERGENCE):**
- **Spec README:** `BAD_ARGUMENT` for unknown `buffer_id` **"or in case of buffer overflow due to invalid `start` and/or `max_size` values."**
- **proxy-wasm-cpp-host `src/exports.cc:get_buffer_bytes`:** silently clamps — `if (start > buffer->size()) { length = 0; } else if (start + length > buffer->size()) { length = buffer->size() - start; }` and returns `WasmResult::Ok`. Only `start > start + length` (i32 overflow) returns `BadArgument`.
- **Implication for 25.2:** envoy-go MUST mirror cpp-host clamp behavior for compat with real wasm filters (Istio/Envoy guests rely on clamp); SPEC body pins the clamp as the wire contract and treats the README text as imprecise.

**Body callback dispatch semantic pin:** Per-chunk invocation; `body_size` is **accumulated total available** (NOT just-new-chunk delta), grows monotonically when host buffers under PAUSE. Quote: *"body_size represents the total available size of the body that can be retrieved, and its value will increment if the processing is paused and the body is buffered by the host and not forwarded upstream."* — spec README §proxy_on_request_body. **Q1 anticipation CONFIRMED.**

**WasmBufferType values activated at 25.2:** `HTTP_REQUEST_BODY=0`, `HTTP_RESPONSE_BODY=1`, `HTTP_CALL_RESPONSE_BODY=4` (spec README §proxy_buffer_type_t). Matches AMEND-A7 from parent SPEC. Values 2/3/5-8 remain inactive in 25.2.

**WasmHeaderMapType values activated at 25.2:** `HTTP_REQUEST_TRAILERS=1`, `HTTP_RESPONSE_TRAILERS=3` (spec README §proxy_map_type_t). **NO trailer-specific hostcalls exist** — confirmed against spec, cpp-host, and Rust SDK. Reuses the 25.1 7-method header-map family verbatim.

**Action enum:** `CONTINUE=0`, `PAUSE=1` (spec README §proxy_action_t).

**AMEND-B1 disposition:** **Substantive AMEND warranted.** The buffer-bounds semantic divergence requires SPEC §11.1 to explicitly pin the clamp behavior as the conformance target, citing both sources. Signatures themselves are straight CONFIRMS — no surprise args, return types, or hostcall counts vs BRAINSTORM Q1 anticipation. Trailer-family reuse is straight CONFIRMS.

### 11.2 D-25.2-2 — Metric hostcall signatures + MetricType enum (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS BRAINSTORM Q9 anticipation (Counter=0, Gauge=1, Histogram=2; `wasmcustom` namespace prefix). REFINES `proxy_increment_metric` delta type to **signed `i64`** (not unsigned), and pins `proxy_record_metric` value to **unsigned `u64`**. REFINES dynamic-stats namespace to `wasmcustom.<custom_name>` only (NO plugin prefix as BRAINSTORM Q9 anticipated).

**Hostcalls pinned:**

| # | Hostcall | Args (full type list) | Return | Source pin |
|---|---|---|---|---|
| 1 | `proxy_define_metric` | `i32 metric_type, i32 *name_data, i32 name_size, i32 *return_metric_id` | `i32 (proxy_status_t)` | spec @main `abi-versions/v0.2.1/README.md` |
| 2 | `proxy_increment_metric` | `i32 metric_id (u32), i64 delta (int64_t — SIGNED)` | `i32 (proxy_status_t)` | spec README + cpp-host `src/exports.cc:1065-1068` (`int64_t offset`) + rust-sdk `hostcalls.rs:1395-1397` (`offset: i64`) |
| 3 | `proxy_record_metric` | `i32 metric_id (u32), i64 value (uint64_t — UNSIGNED)` | `i32 (proxy_status_t)` | spec README + cpp-host `src/exports.cc:1070-1073` (`uint64_t value`) + rust-sdk `hostcalls.rs:1381-1383` (`value: u64`) |
| 4 | `proxy_get_metric` | `i32 metric_id (u32), i32 *return_value (uint64_t *)` | `i32 (proxy_status_t)` | cpp-host `src/exports.cc:1075-1085` + rust-sdk `hostcalls.rs:1365-1367` |

**MetricType enum pin:**
- `Counter = 0`, `Gauge = 1`, `Histogram = 2`
- Authoritative cite: proxy-wasm-rust-sdk v0.2.4 `src/types.rs` — `#[repr(u32)] pub enum MetricType { Counter = 0, Gauge = 1, Histogram = 2 }` (non_exhaustive). Spec README confirms `proxy_metric_type_t: COUNTER=0, GAUGE=1, HISTOGRAM=2`.

**Dynamic-stats namespace pin:** `wasmcustom.<custom_name>` (NOT `wasmcustom.<plugin>.<name>` — Q9 anticipation REFINED).
- Envoy v1.37.2 `source/extensions/common/wasm/stats_handler.h:16`: `constexpr absl::string_view CustomStatNamespace = "wasmcustom";` (L18 comment: "prefix is removed from the final output of /stats endpoints").
- Envoy v1.37.2 `source/extensions/common/wasm/context.cc:1623-1625` (Counter branch): `Stats::Utility::counterFromElements(*envoyWasm()->scope_, {envoyWasm()->custom_stat_namespace_, stat_name})` — namespace and raw user name are the only two elements; plugin name is NOT interpolated into the stat name (scope is per-Wasm-instance, not per-plugin-as-prefix).

**Error semantic for invalid metric_id:**
- Spec README (`proxy_increment_metric` / `proxy_record_metric` / `proxy_get_metric`): `NOT_FOUND when the requested metric_id was not found`; `BAD_ARGUMENT when the requested delta cannot be applied to metric_id (e.g. trying to decrement counter)`.
- cpp-host `src/exports.cc:1066-1068, 1071-1073, 1077-1078` delegate to `context->{increment,record,get}Metric()`; wrapper returns whatever `WasmResult` the context returns.
- Rust-sdk `hostcalls.rs:1369-1378, 1385-1392, 1399-1408` explicitly match `Status::NotFound` and `Status::BadArgument` returns.

**AMEND-B2 disposition:** Substantive AMEND. Two REFINES: (1) `proxy_increment_metric` delta MUST be signed `int64_t` to allow negative gauge deltas; (2) dynamic-stats namespace is `wasmcustom.<custom_name>` only — plugin name is NOT prefixed into the stat. Counter/Gauge/Histogram enum values and the `wasmcustom` prefix itself are CONFIRMED.

### 11.3 D-25.2-3 — proxy_http_call wire shape + response routing (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS BRAINSTORM Q4 (unknown-cluster = `BadArgument`). REFINES §10.1 D-25.2-3 anticipation (late-response handling): cpp-host does NOT silently drop — late-response cancellation occurs at stream/context destruction time, so by the time a response *would* arrive the underlying `AsyncClient::Request` has already been cancelled; the "late drop" path is structurally rare in cpp-host (defensive token-miss guard handles strays).

**proxy_http_call signature (verbatim, proxy-wasm v0.2.1 spec + cpp-host `src/exports.cc:664-687`):**

| Arg # | Name | Type | Source pin |
|---|---|---|---|
| 1 | upstream_name_data | i32 (const char *) | spec v0.2.1 §HTTP calls; cpp-host L664 (`uri_ptr`) |
| 2 | upstream_name_size | i32 (size_t) | spec; cpp-host L664 (`uri_size`) |
| 3 | serialized_headers_data | i32 (const uint8_t *) | spec; cpp-host L665 (`header_pairs_ptr`) |
| 4 | serialized_headers_size | i32 (size_t) | spec; cpp-host L665 |
| 5 | body_data | i32 (const uint8_t *) | spec; cpp-host L666 |
| 6 | body_size | i32 (size_t) | spec; cpp-host L666 |
| 7 | serialized_trailers_data | i32 (const uint8_t *) | spec; cpp-host L667 (`trailer_pairs_ptr`) |
| 8 | serialized_trailers_size | i32 (size_t) | spec; cpp-host L667 |
| 9 | timeout_milliseconds | i32 (uint32_t) | spec; cpp-host L668; rust-sdk `hostcalls.rs:1067-1076` (`timeout: u32`); rust-sdk L1089 (`as_millis() as u32`) |
| 10 | return_call_id | i32 (uint32_t *) | spec; cpp-host L668 (`token_ptr`) |

**Return type:** `i32 (proxy_status_t)` — i.e. `WasmResult` (spec v0.2.1). Returns `Ok`, `BadArgument`, `InternalFailure`, or `InvalidMemoryAccess`.

**proxy_on_http_call_response callback signature (verbatim, spec v0.2.1):**

| Arg # | Name | Type |
|---|---|---|
| 1 | plugin_context_id | i32 (uint32_t) |
| 2 | call_id | i32 (uint32_t) |
| 3 | num_headers | i32 (size_t) |
| 4 | body_size | i32 (size_t) |
| 5 | num_trailers | i32 (size_t) |

Return: `none`.

**Unknown-cluster disposition pin:** `WasmResult::BadArgument` (=2). Envoy v1.37.2 `source/extensions/common/wasm/context.cc:1547-1550`: `clusterManager().getThreadLocalCluster(cluster_string)` → `nullptr` branch returns `WasmResult::BadArgument`. Spec v0.2.1 also enumerates `BAD_ARGUMENT` for "unknown upstream or missing required headers (`:authority`, `:method`, `:path`)". No upstream counter is incremented at this site.

**Late-response-after-stream-closed disposition pin:** Envoy v1.37.2 `context.cc:1900-1905` destructor iterates `http_request_` and calls `p.second.request_->cancel()` on every outstanding `AsyncClient::Request`. Result: in-flight requests are CANCELLED at context teardown — the response callback is never invoked for a destroyed context. Defensive token-lookup at L1693-1696 (`auto handler = http_request_.find(token); if (handler == http_request_.end()) { return; }`) silently drops any stray callback. NEITHER path increments a counter. Response is NOT routed to root context.

**Recommendation on `wasm.<plugin>.http_call_response_after_close` envoy-go-strict counter:** ADD, as a defensive observability extension. Rationale: cpp-host's silent early-return at L1693-1696 is a known operator-blind-spot; the cancel-at-destruction path means triggers should be rare under correct host implementation, but the counter gives operators a non-zero signal if envoy-go's cancellation has a bug (e.g., `httpclient.Cancel()` race vs response arrival). Counter is upstream-superset, not upstream-divergent — semantically safe.

**AMEND-B3 disposition:** Substantive AMEND. BRAINSTORM §10.1 framed late-response as "cpp-host appears to drop silently — verify"; the pin reveals a two-layer mechanism (cancellation at destruction + defensive token-miss guard) rather than a single drop. SPEC §11 pins BOTH layers and the envoy-go-strict counter recommendation. RAISES the §7 envoy-go-strict counter tally from 8 (per BRAINSTORM Q9) to **9** at 25.2; project stat count 119 → 128 (was 127 per BRAINSTORM).

### 11.4 D-25.2-4 — Full proxy_get_property roster + serialization (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS BRAINSTORM §10.1 Q7/D-25.2-4 anticipation on NUL-delimited wire; REFINES the property roster (BRAINSTORM under-estimated the XDS sub-tree and missed several Request/Connection sub-paths; Listener/Route/Wasm/Downstream are NOT separate roots — folded into XDS or unimplemented).

**proxy_get_property hostcall signature (verbatim, proxy-wasm v0.2.1 README §Serialization):**

| Arg # | Name | Type | Source pin |
|---|---|---|---|
| 1 | `path_data` | `const uint8_t *` (i32) | spec v0.2.1 README, `#proxy_get_property` |
| 2 | `path_size` | `size_t` (i32) | spec v0.2.1 README |
| 3 | `return_value_data` | `uint8_t **` (i32) | spec v0.2.1 README |
| 4 | `return_value_size` | `size_t *` (i32) | spec v0.2.1 README |
| ret | `status` | `proxy_status_t` (i32) | spec v0.2.1 README |

Host-side: `exports.cc:38-48` reads raw bytes via `wasmVm()->getMemory(path_ptr, path_size)`, forwards opaque buffer to `Context::getProperty(std::string_view path, std::string* result)` (`context.h:158`, `context.cc:1040`).

**Path serialization wire format pin:** NUL-delimited byte segments. Confirmed three ways:
- spec v0.2.1 README §Serialization: *"Path segments are separated by NULL (0x00) characters"*; example `["foo","bar"]` → `0x66 0x6f 0x6f 0x00 0x62 0x61 0x72`; trailing NUL tolerated.
- `context.cc:1047-1058`: `size_t end = path.find('\0', start); ...; auto part = path.substr(start, end-start); start = end+1;`
- NO split on `.` anywhere host-side; exports.cc passes opaque buffer through.

**Full property roster (per upstream Envoy v1.37.2, `source/extensions/filters/common/expr/context.h`):**

| Root | Sub-paths | Source pin |
|---|---|---|
| `request` | path, url_path, host, scheme, method, referer, headers, headers_bytes, time, id, useragent, size, total_size, duration, protocol, query | context.h L55-71 (16) |
| `response` | code, code_details, trailers, flags, grpc_status, backend_latency | context.h L74-80 (6) |
| `metadata` | (filter-name keyed; `google.protobuf.Struct`) | context.h L83 |
| `filter_state` | (key-keyed; `FilterStateObject`) | context.h L86 |
| `upstream_filter_state` | (key-keyed; upstream-scoped FilterStateObject) | context.h L87 |
| `connection` | mtls, requested_server_name, tls_version, termination_details, subject_local_certificate, subject_peer_certificate, uri_san_local_certificate, uri_san_peer_certificate, dns_san_local_certificate, dns_san_peer_certificate, sha256_peer_certificate_digest, transport_failure_reason, id (via `CONNECTION_ID` direct token, context.cc:1072-1097) | context.h L90-102 (12+id) |
| `source` | address, port | context.h L105-107 (2) |
| `destination` | address, port (re-uses Source sub-symbols) | context.h L110 (2) |
| `upstream` | address, port, local_address, locality, transport_failure_reason, request_attempt_count, cx_pool_ready_duration, num_endpoints, + TLS cert sub-symbols re-used from Connection (subject/uri_san/dns_san/sha256/tls_version) | context.h L113-119 (~14) |
| `xds` | cluster_name, cluster_metadata, route_name, route_metadata, virtual_host_name, virtual_host_metadata, upstream_host_metadata, upstream_host_locality_metadata, filter_chain_name, listener_metadata, listener_direction, node | context.h L122-? (12) |
| `wasm.<key>` (special) | proxied to filter_state (then upstream filter_state) | context.cc:987-1019 in `findValue` |
| direct tokens | `plugin_name`, `plugin_root_id`, `plugin_vm_id`, `connection_id` | context.cc:1072-1097 (4) |

**No separate `listener` or `route` roots** — folded into `xds.*`. **No `downstream.*` root** — covered by `connection.*` and `source.*`. **No `wasm` root for self-introspection** beyond the `wasm.<filter_state_key>` proxy.

**Totals:** ~10 dispatched roots + 4 direct tokens; ~70 documented sub-paths excluding map/message recursion (which is unbounded via the `IsMap`/`IsMessage` branches at context.cc:1068-1111).

**Absent-property behavior:** `WasmResult::NotFound` (=1) returned at context.cc:1065 (top-level miss), 1072 (map miss), 1078 (null msg), 1083 (no proto field), 1103/1106 (list parse/OOB), 1110 (non-traversable terminal). `WasmResult::InternalFailure` (=2) at 1097 if proto field extraction errors. `WasmResult::Ok` (=0) via `serializeValue` at 1114.

**envoy-go-internal primitive mapping:**
- `request.*`, `response.*`, `source.*`, `destination.*` → stream-local accessors (no co-consumed primitive needed)
- `connection.{subject,uri_san,dns_san,sha256,tls_version}_*` → RE-CONSUMES phase-04 ADR-0144 `DownstreamPrincipal()`
- `upstream.{cluster fields via xds.cluster_*}`, `upstream.address/port/local_address` → RE-CONSUMES phase-20 ADR-0177 `internal/httpclient/`
- `metadata.*`, `xds.*_metadata` → RE-CONSUMES phase-22.2 ADR-0190 `internal/dynamicmetadata/`
- `filter_state.*`, `upstream_filter_state.*`, `wasm.<key>` → EXTRACTS NEW `internal/filterstate/` framework primitive per Q7 + ADR-0207
- direct tokens (`plugin_name`, `plugin_root_id`, `plugin_vm_id`, `connection_id`) → wasm-VM-local, no primitive

**AMEND-B4 disposition:** SUBSTANTIVE REFINEMENT warranted. BRAINSTORM Q7 anticipated ~13 roots including standalone `listener`/`route`/`downstream`/`wasm`; actual is ~10 dispatched roots + `xds.*` consolidating listener/route/cluster metadata + 4 direct tokens. SPEC §11.4 pins the canonical `expr/context.h` constant table (not a hand-curated subset) and explicitly notes `upstream_filter_state` as a distinct root co-equal to `filter_state` (BRAINSTORM omitted this).

### 11.5 D-25.2-5 — Capability roster extensions for 25.2 (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS BRAINSTORM Q3 + AMEND-A5 anticipation, with one structural REFINEMENT: env-namespace hostcalls are gated at `registerCallback` time in `wasm.cc` (not at exports.cc call sites).

**Capability key format pin (verbatim from cpp-host):**

Gate function — `include/proxy-wasm/wasm.h:103-106`:
```cpp
bool capabilityAllowed(std::string capability_name) {
  return allowed_capabilities_.empty() ||
         allowed_capabilities_.find(capability_name) != allowed_capabilities_.end();
}
```
(empty map => unrestricted; non-empty => allowlist)

Registration discipline — `src/wasm.cc:176-189`:
```cpp
#define _REGISTER(module_name, name_prefix, export_prefix, _fn)            \
  if (capabilityAllowed(name_prefix #_fn)) {                                \
    wasm_vm_->registerCallback(module_name, name_prefix #_fn, ...);        \
  }
#define _REGISTER_WASI_UNSTABLE(_fn) _REGISTER("wasi_unstable", , wasi_unstable_, _fn)
#define _REGISTER_WASI_SNAPSHOT(_fn) _REGISTER("wasi_snapshot_preview1", , wasi_unstable_, _fn)
#define _REGISTER_PROXY(_fn) _REGISTER("env", "proxy_", , _fn)
```

Lifecycle gate — `src/wasm.cc:238-247` (`_GET_PROXY` macro):
```cpp
if (capabilityAllowed("proxy_" #_fn)) {
  wasm_vm_->getFunction("proxy_" #_fn, &_fn##_);
}
```

**Key formats (verbatim):**
- env hostcalls: `proxy_<base>` (e.g., `proxy_get_buffer_bytes`)
- WASI (both `wasi_unstable` and `wasi_snapshot_preview1` modules): BARE name, no module prefix (e.g., `fd_write`) — empty `name_prefix` per wasm.cc:182-183
- Lifecycle module-getters: `proxy_on_<event>` (e.g., `proxy_on_request_body`)

**25.2 NEW capability keys:**

Hostcall keys (env namespace, gated at wasm.cc:188 `_REGISTER_PROXY`; declared by `FOR_ALL_HOST_FUNCTIONS` per exports.h:148-152):

| # | Key | Family |
|---|---|---|
| 1 | proxy_get_buffer_bytes | body/buffer |
| 2 | proxy_set_buffer_bytes | body/buffer |
| 3 | proxy_get_buffer_status | body/buffer |
| 4 | proxy_continue_stream | stream-ctl (ABI-specific, exports.h:154-156) |
| 5 | proxy_close_stream | stream-ctl (ABI-specific) |
| 6 | proxy_set_tick_period_milliseconds | timer |
| 7 | proxy_define_metric | metrics |
| 8 | proxy_increment_metric | metrics |
| 9 | proxy_record_metric | metrics |
| 10 | proxy_get_metric | metrics |
| 11 | proxy_set_shared_data | shared-data |
| 12 | proxy_get_shared_data | shared-data |
| 13 | proxy_http_call | outbound-HTTP |
| 14 | proxy_call_foreign_function | foreign-fn |

Lifecycle keys (gated at wasm.cc:238 `_GET_PROXY` via `FOR_ALL_MODULE_FUNCTIONS`, wasm.h:270-279):

| # | Key |
|---|---|
| 15 | proxy_on_request_body |
| 16 | proxy_on_response_body |
| 17 | proxy_on_request_trailers |
| 18 | proxy_on_response_trailers |
| 19 | proxy_on_tick |
| 20 | proxy_on_http_call_response |
| 21 | proxy_on_foreign_function (gated separately via `_GET_PROXY(on_foreign_function)` at wasm.cc per ABI 0.2.0/0.2.1 branch) |

**WASI:** No new WASI hostcalls at 25.2 (25.2 adds env-namespace only). The bare-name format (`fd_write`, not `wasi_snapshot_preview1.fd_write`) is confirmed at wasm.cc:182-183.

**Total NEW keys at 25.2:** 21 (14 hostcall + 7 lifecycle). `proxy_on_queue_ready` is NOT in 25.2 scope per §2.7 (shared-queue defers to WASM host family).

**Post-25.2 cumulative roster size:** 37 (25.1) + 21 = **58 keys**.

**AMEND-B5 disposition:** Substantive AMEND warranted. Two findings beyond AMEND-A5: (1) env-hostcall gating happens at `registerCallback` time in `wasm.cc:176-189`, NOT at exports.cc call sites (exports.cc contains zero `capabilityAllowed` invocations) — envoy-go port must mirror gate-at-registration; (2) WASI uses empty `name_prefix` per wasm.cc:182-183, confirming bare-name keys for any future WASI entries.

Sources: `include/proxy-wasm/wasm.h:103-106, 270-279`; `src/wasm.cc:176-189, 238-247`; `include/proxy-wasm/exports.h:148-156, 165-190`.

---

## 12. SPEC-time D-questions for PLAN-time / IMPL-time resolution (D-25.2-P1 .. D-25.2-P5)

Per BRAINSTORM §10.2 + phase-25.1 SPEC §12 D-P discipline + ADR-0044. The parent SPEC has CLOSED most D-question candidates at parent §1.1 AMEND-A1..A9; the 25.2 SPEC has CLOSED the 5 §11 D-25.2-1..D-25.2-5 empirical pins via §11.1-§11.5 above. The remaining 5 D-questions are PLAN-time + IMPL-time architectural-but-implementation-detail decisions that the 25.2 SPEC author anchors for resolution at their natural anchor points:

| D# | Question | Resolution at | Anticipated answer |
|---|---|---|---|
| **D-25.2-P1** | Fixture-0037 single-arm boot-reject finalization: which 25.2 NEW PARSE-REJECT arm (from {19, 20, 21, 22, 23, 26}) is chosen for the BOOT-REJECT fixture, and what is the substring assertion? | 25.2 IMPL Task NN (fixture-0037 task) first-action | **Arm 19** `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`. Subject-only boot-reject (reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field — silent drop). Runner-branch shape: extend `BootRejectFixture` with `subjectOnly: true` flag (recommended) OR introduce `SubjectOnlyBootRejectFixture` (cleaner type-discrimination). |
| **D-25.2-P2** | Per-stream Module instantiation pattern: fresh vs pooled vs shared-Module-with-mutex-serialization? R8 escape-valve trigger threshold? | 25.2 IMPL benchmark task (`BenchmarkPerStreamModule_Instantiation`) — analogous to 25.1 Task 17 R8 gate | **Fresh-per-stream Module instantiation STANDS WEAK-default** at SPEC commit per §2.16. Threshold: `ns/op > 1_000_000` (1ms) fires ADR-0209 escape-valve. **Anticipated outcome: STANDS WEAK-default** — the 25.2 root-VM model shrinks per-stream cost vs 25.1's 61µs (Runtime is no longer re-constructed per-stream; only Module instance is per-stream); WELL UNDER threshold expected. ADR-0209 reserve carries forward to 25.3 if unconsumed. |
| **D-25.2-P3** | Foreign-function dispatch concurrency model: mutex-per-RootVM vs event-loop-per-RootVM vs caller-goroutine? | 25.2 SPEC OR 25.2 PLAN (architectural-but-implementation-detail; settles at PLAN if SPEC author defers; settles at SPEC if author chooses now) | **DEFERRED to PLAN.** The cpp-host model is mutex-per-Wasm (sync.Mutex held during dispatch + the foreign function executes synchronously inside the dispatch frame). envoy-go ANTICIPATED follows: `internal/wasm/foreign.go` uses sync.RWMutex on the registry + the dispatched function executes synchronously inside `*RootVM.CallForeignFunction` (no goroutine offload at 25.2; the function's compute cost is the operator's responsibility). PLAN session settles: (a) whether RootVM lock IS held during dispatch (yes — same lock as the per-stream call frame); (b) panic-recovery wrapper discipline (yes — same wrapper as other host-side callbacks); (c) the registry Get path holds an RLock only (read-only access). |
| **D-25.2-P4** | `FuzzWasmHostcallEnvelope` corpus seed final roster | 25.2 SPEC §8.4 (provisional roster) OR 25.2 PLAN (final roster) | **DEFERRED to PLAN.** §8.4 anticipates ~30-40 seeds across 10 dimensions (hostcall envelope edge cases; pairs serialization; foreign-function name boundary; dynamic-stats name validation; shared-data CAS race; body-buffer cap boundary per AMEND-B1; property-path NUL-delimited adversarials per AMEND-B4; tick period parsing including 10ms floor; httpCall envelope; metric type/delta extremes per AMEND-B2). PLAN session enumerates per-seed; IMPL Task NN authors the corpus + verifies must-never-panic under 30s/seed. |
| **D-25.2-P5** | 25.2 BEHAVIOR_CONTRACT.md edit bundle exact line counts + departure-record consolidation | 25.2 IMPL final Task per ADR-0052 atomic-landing discipline | **DEFERRED to IMPL final Task.** §9 + §13.5 anticipate ~7-edit bundle with ~6 envoy-go-strict departure records. Final wording + line counts settle at the bundle authoring; the discipline is `ADR-0052 atomic landing` — all edits land in ONE commit at the final task. |

**D-question discipline:** each Task's PROGRESS.md entry (at 25.2 IMPL session) quotes the scrape evidence + records the disposition; the relevant ADR §Decision body (ADR-0205 / -0206 / -0207 / -0208) carries forward the disposition at the IMPL Lands-in-Task atomic landing.

Additional SPEC-time D-questions may surface during 25.2 IMPL empirical-scrapes (analogous to 25.1 IMPL's D-P-PLAN-1..D-P-PLAN-10 from PLAN-time discoveries); these 5 are the SPEC-anchored set. PLAN session may surface 3-5 additional D-P-PLAN-x questions per the 25.1 PLAN precedent.

### 12.6 Q-question disposition matrix (carry-forward from BRAINSTORM §2)

For completeness — the 11 BRAINSTORM Q-decisions ALL CLOSED at BRAINSTORM commit (per BRAINSTORM §2 + §12). This SPEC INHERITS the closures verbatim:

| Q# | Decision | Anchor in this SPEC |
|---|---|---|
| Q1 | Body callback dispatch model: per-chunk invoke + accumulating buffer (upstream-parity); each chunk triggers ONE `proxy_on_*_body(ctx, body_size, end_of_stream)` invocation | §1; §5.3 C14+C15; AMEND-B1 |
| Q2 | Body-buffer cap discipline: 16 MiB envoy-go-strict default + configurable; 413-on-exceed | §3.1 body.go; §4.2 compiledConfig; §7.4 config field; §9 departure record #2 |
| Q3 | Root VM lifecycle: ONE long-lived `*RootVM` per `*compiledConfig`; per-stream contexts as CHILDREN | §1 + §3.1 + §3.5 root_vm.go + ADR-0205 |
| Q4 | httpCall unknown-cluster: BadArgument + envoy-go-strict counter | §1; §5.1 #37; §7.1 counters 7+11; §11.3 AMEND-B3; §9 |
| Q5 | Timer dispatch: per-root-VM goroutine + 10ms envoy-go-strict period floor + Clock seam FIRST co-consumer | §1; §3.1 tick.go; §3.4 REUSE 4 + RATIFIES ADR-0186; §5.1 #30; §5.3 C18 |
| Q6 | Shared-data caps: 1 MiB value + 1024-entry envoy-go-strict; 2 envoy-go-strict-only config fields | §3.1 shared_data.go; §7.4 config fields; §9 departure record #3 |
| Q7 | Full stream-info property surface + EXTRACT-NOW `internal/filterstate/` consumer-#2 primitive | §1; §3.2 + ADR-0207; §3.4 REUSE 5+6 + MIGRATES; §11.4 AMEND-B4 |
| Q8 | Mixed-mode fixture-0036: single-listener + 12-14 scenarios partitioned by assertion-class | §1; §8.1 |
| Q9 | Dynamic-stats namespace + 8-counter envoy-go-strict tally + 1024-entry cap | §3.3 + §7.1 counters 6-13; §7.3 namespace; §7.4 config field; §9 departure record #6; AMEND-B2 |
| Q10 | Strict-scope ADR consumption + 4 NEW + 1 reserve = ADR-0205..ADR-0209 + ADR-0202 one-line in-place AMEND | §10.1 + §10.2 |
| Q11 | Stay single sub-phase 25.2 + PLAN-stage split-gate arbiter | (PLAN session re-evaluates against task-arm gate ~25 tasks; LoC-arm gate fires comfortably) |

---

## 13. RATIFIED-PENDING-IMPL items + BEHAVIOR_CONTRACT.md edit bundle

Per phase-25.1 §13 + parent §13 + ADR-0052. The 25.2 SPEC INHERITS parent §13 items R1-R8 unchanged + 25.1 SPEC §13 sub-pin closures (D-P1..D-P6 ALL CLOSED at 25.1 IMPL per 25.1 PROGRESS) + adds 25.2-specific sub-pins.

### 13.1 Parent §13 R1-R8 disposition table at 25.2 SPEC commit

| Item | Parent §13 framing | 25.2 SPEC disposition |
|---|---|---|
| **R1** | 25.1 fixture-0034 cross-side byte-exact + §4.5 D6 guardrail compliance | **CLOSED at 25.1 IMPL Task 15** (37/37 fixture dirs GREEN; fixture-0034 cross-side byte-exact). 25.2 fixture-0036 INHERITS the §4.5 D6 guardrails for the 8-10 deterministic cross-side scenarios. |
| **R2** | ABI v0.1.0+v0.2.0 PARSE-REJECT byte-faithful detection point | **CLOSED at 25.1 IMPL Task 2** (`internal/wasm/bytecode_util.go` byte-faithful reimplementation; PARSE-REJECT arm 16). UNCHANGED at 25.2. |
| **R3** | pairs wire format byte-faithful reimplementation | **CLOSED at 25.1 IMPL Task 3** (`internal/wasm/pairs.go` byte-faithful). 25.2 RE-CONSUMES the same pairs format for trailer hostcalls (HttpRequestTrailers + HttpResponseTrailers) + httpCall headers/trailers payloads + foreign-function args wire (if name-keyed pair envelope) — NO new pairs work at 25.2. |
| **R4** | WASI shim custom 8-stub implementation | **CLOSED at 25.1 IMPL Task 4** (`internal/wasm/wasi.go`). UNCHANGED at 25.2 (no new WASI hostcalls per AMEND-B5). |
| **R5** | 34th project-wide fuzzer count verification | **CLOSED at 25.1 IMPL Task 14** (count CONFIRMED at 34; D-S1 RATIFIED). 25.2 D-S2 (35th-fuzzer count) RATIFIED at this 25.2 SPEC commit per §8.5. |
| **R6** | ADR-0177 `internal/httpclient/` co-consumer validation at 25.2 | **STANDS — lands at 25.2 IMPL** at the `proxy_http_call` task (anticipated Task NN where `*RootVM.DispatchHttpCall` is implemented). 25.2 RE-CONSUMES at third-or-later co-consumer (phase-22.2 `:httpCall()` was second). RATIFIES the phase-20 framework-primitive extraction per ADR-0177 §Consequences forward-pointer. NO API extension on httpclient (phase-22.2 cluster-based dispatch covers 25.2's `proxy_http_call` byte-for-byte). |
| **R7** | ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` first co-consumer at 25.1 | **CLOSED at 25.1 IMPL Task 11** (D-P3 closed; first co-consumer landed; RATIFIES phase-23 extraction). UNCHANGED at 25.2. |
| **R8** | wazero per-stream Runtime construction benchmark | **CLOSED at 25.1 IMPL Task 17** (`ns/op=61000`; STANDS WEAK-default; ADR-0205 NOT consumed). 25.2 CARRIES FORWARD the escape-valve slot per D-25.2-P2 + §2.16: at 25.2 IMPL benchmark `BenchmarkPerStreamModule_Instantiation` re-evaluates against the new per-stream Module instantiation cost (likely shrinks vs 25.1's 61µs because the 25.2 root-VM model retires per-stream Runtime construction). If `ns/op > 1ms`, ADR-0209 escape-valve fires. |

### 13.2 25.2-specific RATIFIED-PENDING-IMPL items (R-25.2-x)

| Item | Framing | Lands at |
|---|---|---|
| **R-25.2-1** | Buffer-clamp wire-contract per AMEND-B1 (proxy_get_buffer_bytes clamps on overflow, returns Ok with truncated length; only i32-overflow returns BadArgument) | 25.2 IMPL Task NN that lands `internal/wasm/registration.go` proxy_get_buffer_bytes shim + `internal/wasm/abi/body_bridge.go` + golden table at `registration_test.go` |
| **R-25.2-2** | Metric signedness + dynamic-stats namespace per AMEND-B2 (`proxy_increment_metric` SIGNED int64 delta; `proxy_record_metric` UNSIGNED uint64 value; namespace `wasmcustom.<custom_name>` only — NO plugin prefix) | 25.2 IMPL Tasks that land `internal/wasm/dynamic_stats.go` + `internal/stats/dynamic/` + `internal/wasm/abi/metrics.go` |
| **R-25.2-3** | httpCall cancel-at-destruction + `http_call_response_after_close` defensive counter per AMEND-B3 (cancel via httpclient.Cancel at StreamContext.Close + defensive token-miss guard at response arrival; counter increments on stray response) | 25.2 IMPL Tasks that land `internal/wasm/http_call.go` + `internal/wasm/root_vm.go` cancellation discipline + counter wiring |
| **R-25.2-4** | Full ~70-path property roster per AMEND-B4 + NUL-delimited path parsing + co-consumed primitive mapping (ADR-0144 + ADR-0177 + ADR-0190 + ADR-0207) | 25.2 IMPL Tasks that land `internal/wasm/property.go` + `internal/filter/http/wasm/property.go` + per-root sub-path table-driven tests |
| **R-25.2-5** | Gate-at-registration host-module wiring per AMEND-B5 (denied capabilities → NOT registered on wazero Runtime; guest's import resolution fails at module-instantiation OR runtime trap on call) | 25.2 IMPL Task that EXTENDS `internal/wasm/registration.go` for the 21 NEW capability keys + the gate-at-registration discipline |
| **R-25.2-6** | NEW `internal/filterstate/` framework primitive + phase-22.2 lua MIGRATES per ADR-0207 (consumer #1 = lua MIGRATES non-breaking; consumer #2 = wasm) | 25.2 IMPL Task that materializes `internal/filterstate/` + migration follow-up task in `internal/filter/http/lua/filterstate.go` |
| **R-25.2-7** | NEW `internal/stats/dynamic/` infrastructure package per ADR-0208 + AMEND-B2 (per-plugin Registry scope; `wasmcustom.<custom_name>` namespace; 1024-entry cap) | 25.2 IMPL Task that materializes `internal/stats/dynamic/dynamic.go` + `dynamic_test.go` + `dynamic_admin_test.go` |
| **R-25.2-8** | Foreign-function registration interface per AMEND-A9 + EMPTY default registry + `WasmResult::NotFound` for unregistered | 25.2 IMPL Task that lands `internal/wasm/foreign.go` + `internal/wasm/abi/foreign.go` + `wasm.DefaultForeignFunctionRegistry` process-global |
| **R-25.2-9** | Tick goroutine + 10ms envoy-go-strict floor + Clock seam FIRST co-consumer per Q5 + ADR-0205 (RATIFIES phase-21 ADR-0186 extraction at second-or-later co-consumer scope) | 25.2 IMPL Task that lands `internal/wasm/tick.go` + Clock seam injection via `WithRootClock` + fixture-0036 fake-clock test seam |
| **R-25.2-10** | Shared-data CAS + envoy-go-strict caps per Q6 + ADR-0206 (1 MiB value cap + 1024-entry cap; CasMismatch on conflict; InternalFailure on cap exceeded) | 25.2 IMPL Task that lands `internal/wasm/shared_data.go` + CAS golden table + cap-boundary tests |
| **R-25.2-11** | Mixed-mode fixture-0036 + subject-only fixture-0037 per Q8 + D-25.2-P1 + ADR-0208 (12-14 scenarios; deliberate-break liveness verification mandatory) | 25.2 IMPL Tasks that land `test/fixtures/0036-http-wasm-body-and-advanced/` + `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/` + driver.go + scripts/ Rust source + bytecode/ vendored .wasm files |
| **R-25.2-12** | 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + ADR-0206 (~30-40 corpus seeds; must-never-panic) | 25.2 IMPL Task that lands `internal/wasm/fuzz_test.go` or `internal/filter/http/wasm/fuzz_hostcall_test.go` |

### 13.3 25.2 SPEC-time sub-pin closures (per §11 + §12)

- **D-25.2-1 CLOSED at §11.1** — body+buffer+trailers hostcall signatures + AMEND-B1 buffer-clamp REFINEMENT.
- **D-25.2-2 CLOSED at §11.2** — metric hostcall signatures + AMEND-B2 signedness + namespace REFINEMENT.
- **D-25.2-3 CLOSED at §11.3** — proxy_http_call wire shape + AMEND-B3 cancel-at-destruction + `http_call_response_after_close` counter recommendation.
- **D-25.2-4 CLOSED at §11.4** — full proxy_get_property roster + AMEND-B4 root consolidation + upstream_filter_state addition.
- **D-25.2-5 CLOSED at §11.5** — capability roster extensions + AMEND-B5 gate-at-registration STRUCTURAL REFINEMENT.
- **D-S2 CLOSED at §8.5** — 35th-fuzzer count CONFIRMED at this 25.2 SPEC commit (33 + 1 at 25.1 = 34 at master tip; `FuzzWasmHostcallEnvelope` is 35th).

### 13.4 BEHAVIOR_CONTRACT.md edit bundle anticipation (per ADR-0052 atomic landing)

Per ADR-0052 in-place-edit authorization + 25.1 SPEC §14 + parent §13.5 + phase-22.2 SPEC §13.5 advanced-bridge precedent. The 25.2 IMPL final-Task **~7-edit bundle** (slightly larger than 25.1's 6-edit bundle per BRAINSTORM §5.4):

1. **EXTEND `### envoy.filters.http.wasm` subsection** with body+buffer+trailers+timer+metrics+shared-data+httpCall+foreign-function+full-property bridge details (~150-250 NEW lines on top of 25.1 content). Add cross-refs to ADR-0205+0206+0207+0208 + AMEND-B1..B5.

2. **Stat-table 119 → 128 extension** under BEHAVIOR_CONTRACT.md `## Stat surface`. 9 new rows for the 25.2 envoy-go-strict counters per §7.1 + `wasmcustom.<custom_name>` dynamic namespace structural-note row per §7.3. Plus the per-plugin Registry scope discipline note per AMEND-B2.

3. **envoy-go-strict departure record #1: 9-counter consolidated bundle** per AMEND-B3 (RAISES BRAINSTORM Q9 8 → 9). NEW row at BEHAVIOR_CONTRACT.md envoy-go-strict departures table. Counter roster + per-counter rationale + AMEND-B3 cross-ref.

4. **envoy-go-strict departure record #2: body buffer cap discipline** (per Q2). 16 MiB default + `envoy_go_strict_body_buffer_cap_bytes` envoy-go-strict-only config + body_buffer_cap_exceeded counter + 413-on-exceed semantic + `envoy_go.failures` scope extension per §2.25.

5. **envoy-go-strict departure record #3: shared-data cap discipline + tick period 10ms floor** (per Q6 + Q5; consolidated). 1 MiB value cap + 1024-entry cap envoy-go-strict + 2 envoy-go-strict-only config fields + shared_data_cap_exceeded counter + 10ms tick period floor clamp + departure rationale.

6. **envoy-go-strict departure record #4: foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace refinement** (per AMEND-A9 + Q9 + AMEND-B2; consolidated). Upstream registers 10 by default; envoy-go registers ZERO; `wasm.RegisterForeignFunction` API at boot. 1024-entry cap on dynamic-stats + `wasm.<plugin>.dynamic_stats_cap_exceeded` counter + `envoy_go_strict_dynamic_stats_max_entries` config field. Namespace `wasmcustom.<custom_name>` (NO plugin prefix) per AMEND-B2; per-plugin isolation via per-plugin Registry SCOPE.

7. **EXTEND/RENAME `### Phase 25.1 forward-pointer notes` → `### Phase 25.2 forward-pointer notes`** subsection. 25.2 hand-off lifts items (body+buffer+trailers+timer+metrics+shared-data+httpCall+foreign-function+full-property all ACTIVATED at 25.2 IMPL) + 25.3-anticipated additions (per-route TPFC 5th-canonical REUSE-by-absence per AMEND-A3; multi-plugin VM-sharing; conformance harness seed at 62.5% per AMEND-A8; `VmConfig.environment_variables` activation; `failure_policy = FAIL_RELOAD` activation + Group-C `vm_reload*` counters; `fail_open` deprecated mapping). ~80 lines.

**Anticipated post-25.2 BEHAVIOR_CONTRACT.md departure-record count: 21 (post-25.1) + 6 (25.2 records #1-#6 above; consolidated where related) = ~27 records.** Buffer-clamp wire-contract per AMEND-B1 + NUL-delimited property path serialization per AMEND-B4 + cancel-at-destruction per AMEND-B3 are recorded as wire-shape notes (NOT departure records) per §9.

25.3 IMPL final-Task bundle anticipated (settled at 25.3 BRAINSTORM/SPEC): extends with per-route + multi-plugin + conformance detail; 5th-canonical REUSE-by-absence caption note (NO §(xvi) amendment per AMEND-A3); ADR-0212 conformance harness pin + 62.5% threshold cross-reference; potentially Group-C `vm_reload*` counters if `failure_policy = FAIL_RELOAD` lands; potentially additional envoy-go-strict departure records if cross-plugin shared-data scoping surfaces.

---

## 14. Test surface

### 14.1 Layer A: unit tests at `internal/filter/http/wasm/` (25.2 IMPL)

- `wasm_test.go` (25.1 — EXTENDED at 25.2 with body/trailer/tick/httpCall/foreign-function/property integration shape).
- `compiled_config_test.go` (25.1 — EXTENDED with 6 NEW PARSE-REJECT arm coverage per §6.2 + envoy-go-strict-only config field validators).
- `abi_callbacks_test.go` (25.1 — EXTENDED with 7 NEW method coverage per §5.3 + 4 RE-USE primitive round-trip tests).
- `body_test.go` (NEW 25.2) — body-buffer accumulation + cap enforcement + 413-on-exceed dispatch + `body_buffer_cap_exceeded` counter assertion.
- `trailers_test.go` (NEW 25.2) — trailer hostcall dispatch round-trip + WasmHeaderMapType values 1/3 activation.
- `property_test.go` (NEW 25.2) — per-stream property resolver dispatch coverage for the ~70 sub-paths per AMEND-B4 + co-consumed primitive round-trips (ADR-0144 + ADR-0177 + ADR-0190 + ADR-0207) + absent-property NotFound semantics.
- `dispatch_test.go` (25.1 — EXTENDED with body/trailer/tick/httpCall integration round-trips + per-stream concurrency tests under the new root-VM model).

### 14.2 Layer B: unit tests at `internal/wasm/` (25.2 IMPL)

- `root_vm_test.go` (NEW 25.2) — `*RootVM` lifecycle (NewRootVM + Configure + NewStreamContext + Close); tick goroutine start/stop; shared-data Set/Get/CAS round-trip; httpCall DispatchHttpCall + cancel-at-destruction + http_call_response_after_close counter; foreign-function CallForeignFunction Get-Hit + NotFound paths.
- `stream_context_test.go` (NEW 25.2) — `*StreamContext` per-stream lifecycle (NewStreamContext → CallProxyOn* → Close); per-stream isolation under concurrent dispatch (N=100 × `-count=10` per the 25.1 dispatch_test precedent); panic-wrapper integration.
- `tick_test.go` (NEW 25.2) — tick goroutine + Clock seam fake-time tests (use `clock.FakeClock` from phase-21 `internal/clock`); 10ms floor enforcement (set period=5ms; assert effectivePeriod=10ms; assert tick fires at 10ms intervals); set period=0 cancels.
- `shared_data_test.go` (NEW 25.2) — CAS semantics golden table (cas=0 always writes; cas>0 writes only on match; mismatch returns CasMismatch); cap-boundary tests (value-cap-exceeded → InternalFailure + counter; entry-cap-exceeded → InternalFailure + counter).
- `property_test.go` (NEW 25.2) — full property roster tests for the ~70 sub-paths per AMEND-B4; NUL-delimited path parsing tests (empty path; trailing NUL tolerated; non-NUL separator → NotFound); absent-property NotFound semantics; co-consumed primitive integration (DownstreamPrincipal + httpclient + dynamicmetadata + filterstate).
- `dynamic_stats_test.go` (NEW 25.2) — `proxy_define_metric` + Increment + Record + Get round-trip; MetricType enum byte-pin (Counter=0; Gauge=1; Histogram=2 per AMEND-B2); signed-i64 delta (negative gauge delta; positive); unsigned-u64 record; 1024-entry cap-boundary; idempotent re-Register of same name.
- `foreign_test.go` (NEW 25.2) — ForeignFunctionRegistry Register/Get + EMPTY-default behavior (NotFound for unregistered); cap-key-default-deny (proxy_call_foreign_function denied → InternalFailure per ADR-0204); registered-then-deregister-then-call sequence.
- `http_call_test.go` (NEW 25.2) — `proxy_http_call` dispatch with mock httpclient.Client + cluster_b second-upstream-cluster scenario; cancel-at-destruction (StreamContext.Close cancels in-flight requests); late-response-after-close → http_call_response_after_close counter increment + defensive token-miss path; BadArgument on unknown cluster.
- `abi/body_bridge_test.go` (NEW 25.2) — buffer-clamp golden table per AMEND-B1 (start_in_bounds+max_in_bounds; start_in_bounds+max_overflows → clamp with Ok; start_at_end+max_anything → length=0 + Ok; start_beyond_end → length=0 + Ok; start+max_size i32-overflow → BadArgument).
- `abi/metrics_test.go` (NEW 25.2) — MetricType enum byte-pin + ErrBadArgument enforcement (Increment on Histogram; Record on Counter).
- `abi/shared_data_test.go` (NEW 25.2) — CAS golden table; cap-exceeded WasmResult::InternalFailure; concurrent-Set stress test (sync.RWMutex discipline).

### 14.3 Layer C: NEW unit tests at `internal/filterstate/` (25.2 IMPL — ADR-0207)

- `filterstate_test.go` (NEW 25.2) — Set/Get/Keys round-trip + read-only-vs-mutable conflict + nil-handling.
- `bucket_concurrency_test.go` (NEW 25.2) — RWMutex discipline + concurrent-read concurrent-add tests.
- `filterstateobject_test.go` (NEW 25.2) — interface conformance + edge cases.

### 14.4 Layer D: NEW unit tests at `internal/stats/dynamic/` (25.2 IMPL — ADR-0208)

- `dynamic_test.go` (NEW 25.2) — Register/Increment/Record/Get round-trip + signed-delta semantics + idempotent-Register + ErrCapExceeded threshold + ErrBadArgument enforcement.
- `dynamic_admin_test.go` (NEW 25.2) — admin /stats enumeration round-trip + name format `wasmcustom.<custom_name>` byte-pin.
- `dynamic_concurrency_test.go` (NEW 25.2) — RWMutex discipline + concurrent-Register stress test (cap-boundary race).

### 14.5 Layer E: phase-22.2 lua MIGRATION test verification (25.2 IMPL — ADR-0207)

- `internal/filter/http/lua/filterstate_test.go` (PHASE-22.2 — UNCHANGED EXPECTATIONS at 25.2). The MIGRATION rewrites `internal/filter/http/lua/filterstate.go` to delegate to `internal/filterstate/*Bucket`; the test file's expectations MUST stay byte-identical (non-breaking migration). 25.2 IMPL Task NN explicitly runs the existing phase-22.2 lua filterstate tests AFTER the migration to verify no test breakage. Acceptance: 100% green run + no test wording change.

### 14.6 Layer F: 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` (25.2 IMPL — §8.4)

`fuzz_test.go` at `internal/wasm/` (or `internal/filter/http/wasm/` depending on which package owns the hostcall envelope adversarial corpus seam — settles at IMPL Task NN; anticipated `internal/wasm/fuzz_test.go` since the hostcall envelope is primitive-side). Corpus seeds covering 10 dimensions per §8.4. Must-never-panic invariant covers all 14 NEW hostcall surfaces + foreign-function dispatch + dynamic-stats Register path + shared-data CAS race + body-buffer cap boundary + property-path NUL-delimited adversarials.

### 14.7 Layer G: differential fixture `0036-http-wasm-body-and-advanced` (25.2 IMPL — §8.1)

Per §8.1.1. 14 scenarios (10 deterministic cross-side via `CompareBytes` + 4 non-deterministic subject-only via `StatsAsserter.AssertStats`). NEW `BackendKind=HTTPWasmAdvanced` (or REUSE `HTTPWasm`) at `test/differential/runner_test.go`. NEW `bytecode/` subdirectory with 14 vendored pre-built `.wasm` files per Q9. NEW second-cluster bootstrap-generator extension for httpCall scenarios.

### 14.8 Layer H: differential fixture `0037-http-wasm-body-and-advanced-boot-reject` (25.2 IMPL — §8.2)

Per §8.2 + D-25.2-P1. Subject-only BOOT-REJECT (reference Envoy accepts the unknown envoy-go-strict-only field). Anticipated arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`. Runner-branch shape: extend `BootRejectFixture` with `subjectOnly: true` flag (recommended).

### 14.9 Layer I: race + concurrency tests (25.2 IMPL)

`internal/wasm/root_vm_test.go` + `internal/wasm/stream_context_test.go` + `internal/filter/http/wasm/dispatch_test.go` — extended for the 25.2 root-VM model:
- Concurrent stream contexts sharing one *RootVM + shared-data Set/Get under contention.
- Tick goroutine + per-stream callback firing concurrently — verify no cross-stream state leak.
- httpCall dispatch from N concurrent streams to the same RootVM — verify call_id allocation + response routing isolation.
- Foreign-function dispatch from concurrent streams to the same registered function — verify no cross-stream argument leak (the function executes inside the per-stream call frame, NOT the root context).

### 14.10 Six-gate checklist (per 25.1 + parent precedent)

- **Gate A — build:** `go build ./...` clean (incl. NEW `internal/filterstate/` + NEW `internal/stats/dynamic/` packages; the `internal/wasm/` extensions; the `internal/filter/http/wasm/` extensions; the `internal/filter/http/lua/filterstate.go` migration).
- **Gate B — vet + lint:** `go vet ./...` + `golangci-lint run` clean; no new suppressions.
- **Gate C — race:** `go test -race ./...` clean (incl. the NEW packages + per-RootVM tick goroutine + per-stream context concurrent dispatch + shared-data CAS contention + httpCall response routing concurrency).
- **Gate D — differential:** 39/39 fixtures GREEN at 25.2 phase-done (0000-0033 pre-existing + 0034 + 0035 from 25.1 + 0036 + 0037 NEW); cross-side byte-exact on `0036` deterministic scenarios (a)-(j); subject-only `StatsAsserter.AssertStats` GREEN on `0036` non-deterministic scenarios (k)-(n) with deliberate-break liveness verified; subject-only BOOT-REJECT GREEN on `0037`.
- **Gate E — fuzz:** `FuzzWasmHostcallEnvelope` clean at 30s/seed; no panics across the 35 project-wide fuzzers.
- **Gate F — h2spec:** 53/53 PASS at ADR-0051 v1.32.4 pin (UNCHANGED from 25.1).

---

## 15. 25.2 IMPL acceptance checklist (per phase-25.1 SPEC §15.3 precedent)

The 25.2 IMPL Task NN that lands the framework primitive evolutions + package extensions + tests + fixtures + ADR landings + STATE.md re-advance MUST satisfy ALL of:

**Framework primitive evolutions (per §3):**

1. `internal/wasm/root_vm.go` materialized per §3.1 production signatures (RootVM lifecycle + NewRootVM + Configure + NewStreamContext + Close + tick goroutine + shared-data + httpCall routing + foreign-function registry view + dynamic-stats Registry plumbing).
2. `internal/wasm/stream_context.go` materialized per §3.1 (StreamContext + CallProxyOn* methods + Close).
3. `internal/wasm/foreign.go` materialized per AMEND-A9 + §3.1 (ForeignFunctionRegistry + EMPTY default registry + Register/Get + `wasm.DefaultForeignFunctionRegistry` process-global).
4. `internal/wasm/tick.go` materialized per Q5 + §3.1 (per-RootVM tick goroutine + 10ms envoy-go-strict floor + Clock seam injection via `WithRootClock`).
5. `internal/wasm/shared_data.go` materialized per Q6 + §3.1 (per-RootVM CAS-protected K-V map + envoy-go-strict caps + WasmResult::CasMismatch on conflict + InternalFailure on cap exceeded).
6. `internal/wasm/property.go` materialized per AMEND-B4 + §3.1 (full ~70-path property roster + NUL-delimited path parsing + co-consumed primitive dispatch).
7. `internal/wasm/dynamic_stats.go` materialized per Q9 + AMEND-B2 + §3.1 (wraps per-plugin `*dynamic.Registry`; signed-i64 delta; unsigned-u64 value; 1024-entry cap; `wasmcustom.<custom_name>` namespace per AMEND-B2).
8. `internal/wasm/http_call.go` materialized per Q4 + AMEND-B3 + §3.1 (proxy_http_call dispatch + AsyncClient request lifecycle + cancel-at-destruction + defensive token-miss guard + `http_call_response_after_close` counter increment).
9. `internal/wasm/registration.go` EXTENDED per AMEND-B5 + §5.1 (14 NEW env-namespace hostcall registrations + 7 NEW callback dispatch entries; gate-at-registration discipline — denied capabilities NOT registered on wazero Runtime; buffer-clamp wire-contract per AMEND-B1 at proxy_get_buffer_bytes shim).
10. `internal/wasm/sandbox.go` EXTENDED per AMEND-B5 + §3.4 REUSE 8 (21 NEW capability key constants for the 25.2 surface; 37 → 58 cumulative roster).
11. `internal/wasm/abi/` EXTENDED with per-family ABI dispatch files (body_bridge.go + timer.go + metrics.go + shared_data.go + http_call.go + foreign.go + stream_control.go) per §3.5.
12. 25.1's `internal/wasm/vm.go` per-stream `*VM` RETIRED OR transitional-shimmed (decision at IMPL Task 1; anticipated: deleted in favor of root_vm.go + stream_context.go).

**NEW framework primitives (per §3.2 + §3.3):**

13. `internal/filterstate/` NEW package materialized per ADR-0207 + §3.2 (Bucket + FilterStateObject + StateType + Set/Get/Keys); 3 test files per §14.3.
14. `internal/stats/dynamic/` NEW infrastructure package materialized per ADR-0208 + AMEND-B2 + §3.3 (Registry + MetricID + MetricType + NewRegistry/Register/Increment/Record/Get/EnumerateForAdmin); 3 test files per §14.4.
15. **Phase-22.2 lua MIGRATION**: `internal/filter/http/lua/filterstate.go` REWRITES to delegate to `internal/filterstate/*Bucket` (non-breaking; `:filterState()` Lua surface UNCHANGED; existing phase-22.2 lua filterstate tests GREEN without modification per §14.5).

**Filter package extensions (per §3.6):**

16. `internal/filter/http/wasm/compiled_config.go` EXTENDED with 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms per §6.2 + RootVM construction at New() (NOT per-stream wasm.NewVM).
17. `internal/filter/http/wasm/abi_callbacks.go` EXTENDED with 7 NEW methods per §5.3 + 4 RE-USE primitive consumer integration.
18. `internal/filter/http/wasm/body.go` NEW per §4.3 + Q1 + Q2 (body-buffer accumulation + cap enforcement + 413-on-exceed via SendLocalReply).
19. `internal/filter/http/wasm/trailers.go` NEW per §4.3 (DecodeTrailers + EncodeTrailers glue; CallProxyOn*Trailers; reuses 25.1 pairs).
20. `internal/filter/http/wasm/tick_clock.go` NEW per Q5 (Clock seam injection plumbing; fake-time test seam for fixture-0036 tick scenarios).
21. `internal/filter/http/wasm/property.go` NEW per AMEND-B4 (per-stream property resolver dispatch).
22. `internal/filter/http/wasm/stats.go` EXTENDED per §4.2 + §7.1 (9 NEW envoy-go-strict counters added to filterStats; per-plugin `*dynamic.Registry` plumbing).
23. `internal/filter/http/wasm/decode_headers.go` + `encode_headers.go` EXTENDED per §4.3 (per-stream construction via RootVM.NewStreamContext; NOT wasm.NewVM).

**PARSE-REJECT roster:**

24. 6 NEW PARSE-REJECT arms per §6.2 byte-stable wording finalized at IMPL Task NN per D-25.2-P5 (`compiled_config_test.go::TestParseRejectConstants_ByteStable` EXTENDED with the 6 NEW arms).

**Stat surface:**

25. 9 NEW envoy-go-strict counters per §7.1 wired on the filterStats + per-plugin `*dynamic.Registry` scope per AMEND-B2 (`wasmcustom.<custom_name>` namespace); 119 → 128 BEHAVIOR_CONTRACT.md update per §13.4 edit #2.

**envoy-go-strict departure records:**

26. 6 NEW envoy-go-strict departure records at BEHAVIOR_CONTRACT.md per §9 + §13.4 edits #3-#6 (9-counter consolidated bundle + body-buffer cap + shared-data cap + tick floor consolidated + foreign-function 0-vs-10 + dynamic-stats namespace + cap); plus the AMEND-B1 buffer-clamp wire-shape note (NOT a departure record) and the AMEND-B4 property-roster wire-shape note (NOT a departure record).

**Fuzzer + fixtures:**

27. 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + ADR-0018 baseline; must-never-panic verified across 10 corpus-seed dimensions.
28. Differential fixture `0036-http-wasm-body-and-advanced` GREEN — 14 scenarios per §8.1.1 (10 deterministic cross-side + 4 non-deterministic subject-only); deliberate-break liveness verification mandatory for all subject-only StatsAsserter arms per `reference_differential_asserter_dispatch`.
29. Differential fixture `0037-http-wasm-body-and-advanced-boot-reject` GREEN per §8.2 — subject-only single-arm at anticipated arm 19 per D-25.2-P1; runner-branch shape settled at IMPL.
30. NEW `BackendKind=HTTPWasmAdvanced` constant (OR REUSE `HTTPWasm` — settles at IMPL) at `test/differential/runner_test.go`. 37 → 39 differential fixture dirs.

**Wire-shape pins (per §11 AMEND-B1..B5):**

31. Buffer-clamp wire-contract per AMEND-B1 + R-25.2-1 (golden table at `internal/wasm/abi/body_bridge_test.go`).
32. Metric signedness + dynamic-stats namespace per AMEND-B2 + R-25.2-2 (golden table at `internal/wasm/abi/metrics_test.go`; namespace assertion at `internal/stats/dynamic/dynamic_admin_test.go`).
33. httpCall cancel-at-destruction + http_call_response_after_close counter per AMEND-B3 + R-25.2-3 (race-test at `internal/wasm/http_call_test.go`).
34. Full ~70-path property roster + NUL-delimited path parsing per AMEND-B4 + R-25.2-4 (table-driven test at `internal/wasm/property_test.go`).
35. Gate-at-registration host-module wiring per AMEND-B5 + R-25.2-5 (capability-deny → wazero runtime missing-import OR runtime trap; assertion at `internal/wasm/registration_test.go` extension).

**ADR landings:**

36. ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Decision + §Consequences bodies landed in DECISIONS.md per the §Context anchor at THIS 25.2 SPEC commit; ADR-0044 in-place edit discipline.
37. ADR-0202 §Consequences body gains the one-line in-place AMEND acknowledgment paragraph per §10.2; AMENDED timestamp note in §Status line.
38. ADR-0209 reserve disposition closed at IMPL Task NN: STANDS-UNCONSUMED (carries to 25.3) OR CONSUMED (§Decision + §Consequences body landed for the actual surface that fired the escape-valve — likely per-stream Module instantiation R8 escape-valve).

**STATE + ROADMAP:**

39. STATE.md re-advance to `phase 25.2 IMPL done; awaiting 25.3 BRAINSTORM (or 25.3 SPEC if BRAINSTORM-skip)` + ROADMAP row 25.2 flipped `in-progress → done` per ADR-0106 per-cell IMPL-done annotation.
40. Boot-registration UNCHANGED (20 HTTP filters wired per §3.7); `cmd/envoy-go/main.go` UNCHANGED at 25.2.

**R8 escape-valve gate (per D-25.2-P2):**

41. `BenchmarkPerStreamModule_Instantiation` measures per-stream Module instantiation cost. If `ns/op < 1_000_000` (1ms): WEAK-default fresh-per-stream Module instantiation STANDS; ADR-0209 escape-valve STAYS UNCONSUMED. If exceeded: ADR-0209 escape-valve fires (§Context + §Decision + §Consequences all land at this same Task atomic-landing; "pooled vs shared-Module-with-mutex-serialization" decision).

**SPEC-time D-question closures (per §12):**

42. D-25.2-P1 closure at fixture-0037 task first-action — empirical-scrape against upstream Envoy v1.37.2 boot stderr for the 6 candidate arms; chosen arm + substring recorded in PROGRESS.md.
43. D-25.2-P2 closure at benchmark task — R8 disposition recorded in PROGRESS.md per item 41.
44. D-25.2-P3 closure at PLAN session (foreign-function dispatch concurrency model — mutex-per-RootVM anticipated; PLAN settles + IMPL inherits).
45. D-25.2-P4 closure at PLAN session OR fuzzer task (corpus seed final roster).
46. D-25.2-P5 closure at the BEHAVIOR_CONTRACT.md atomic-landing task per §13.4.

25.3 IMPL acceptance checklist settles at 25.3 SPEC.

---

## Appendix A — Cross-references to parent SPEC + 25.1 SPEC

This 25.2 SPEC cross-references the parent SPEC at `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` + the 25.1 SPEC at `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/SPEC.md` for the following content (inherited verbatim; NOT duplicated here):

- **Parent §1** (Mission) — envelope D + 3-way pre-split + 14-fact summary; INHERITED with the 25.2-specific advanced-bridge ACTIVATION narrative added.
- **Parent §1.1** (9-AMEND catalog A1-A9) — INHERITED; AMEND-A1 + A4 + A5 + A6 + A7 + A9 are load-bearing for 25.2 (A1 wazero pin; A4 wazero-vs-V8 byte-exact + §4.5 D6 guardrails carry forward; A5 default-deny sandbox EXTENDED with 21 new capability keys; A6 ABI v0.1.0/v0.2.0 PARSE-REJECT carries forward; A7 WasmResult value-gaps + WasmBufferType activation; A9 foreign-function registration interface LANDS at 25.2). A2 (stat-roster tri-group) + A3 (WasmPerRoute absence) + A8 (conformance source) are 25.1-internal or 25.3-territory.
- **Parent §1.2** (STRENGTHENED-WEAK-HOLD-with-1-slot D-hypothesis) — INHERITED; 25.2 IMPL escape-valve at ADR-0209 from per-stream Module instantiation benchmark per D-25.2-P2 + §13-R8 carry-forward.
- **Parent §2** (Scope — non-purposes + REUSES-not-consumed) — INHERITED; this 25.2 SPEC §2 EXTENDS with 25.2-specific non-purposes.
- **Parent §3** (Sub-phase scope summary) — INHERITED; 3-way split LOCKED at BRAINSTORM Q1; STANDS unchanged at 25.2 SPEC; per-sub-phase scope detail at each sub-phase's SPEC.
- **Parent §3.1** (Sub-phase surface-mapping table) — INHERITED; the 25.2 column dispositions (body+buffer/trailers/timer/metrics/shared-data/httpCall/foreign-function/property all CONSUMED) materialize at this 25.2 SPEC §1 + §5.
- **Parent §4** (Framework primitive sketch) — INHERITED + REFINED at §3 (NEW root_vm.go anchor; NEW filterstate/ primitive; NEW stats/dynamic infrastructure).
- **Parent §6.2** (18-arm 25.1 PARSE-REJECT roster) — INHERITED verbatim; this 25.2 SPEC §6 EXTENDS with 6 NEW arms.
- **Parent §7** (5-counter 25.1 stat surface per AMEND-A2 tri-group structure) — INHERITED verbatim; this 25.2 SPEC §7 EXTENDS with 9 NEW envoy-go-strict counters.
- **Parent §8** (fixture-0034 + fixture-0035 disposition) — INHERITED at 25.1 IMPL phase-done; this 25.2 SPEC §8 ADDS fixture-0036 + fixture-0037.
- **Parent §9** (10 high-level semantic changes) — INHERITED; this 25.2 SPEC §9 EXTENDS with 25.2-specific behavior-contract delta.
- **Parent §10** (ADR anchor map) — INHERITED + EXTENDED at §10 (4 NEW ADRs at 25.2 + ADR-0202 in-place AMEND acknowledgment + ADR-0209 reserve).
- **Parent §11** (SPEC-time empirical-pin block D1-D9 resolved IN-SESSION at parent SPEC) — INHERITED verbatim (NOT re-executed); this 25.2 SPEC §11 ADDS D-25.2-1..D-25.2-5 resolved IN-SESSION at THIS 25.2 SPEC drafting.
- **Parent §13** (R1-R8 RATIFIED-PENDING-IMPL items) — INHERITED; this 25.2 SPEC §13 disposition table maps R6 to "lands at 25.2 IMPL `proxy_http_call` task" + R8 to "25.2 IMPL `BenchmarkPerStreamModule_Instantiation` re-evaluates" + ADDS R-25.2-1..R-25.2-12 sub-phase-specific items.
- **Parent §13.5** (BEHAVIOR_CONTRACT.md edit bundle anticipation) — INHERITED; this 25.2 SPEC §13.4 anticipates ~7-edit bundle at 25.2 IMPL final Task per ADR-0052.
- **Parent §14** (Test surface — 6-layer test taxonomy) — INHERITED; this 25.2 SPEC §14 EXTENDS with new test layers C+D (NEW packages) + F (35th fuzzer) + G+H (new fixtures) + I (new race tests).
- **Parent §15** (24-item acceptance checklist) — INHERITED at 25.1 IMPL phase-done; this 25.2 SPEC §15 introduces a 46-item 25.2-specific checklist.

This 25.2 SPEC cross-references the **25.1 SPEC** for:

- **25.1 SPEC §3.1** (production API signatures for the 25.1 `internal/wasm/` surface) — INHERITED at 25.1 IMPL phase-done; the 25.2 SPEC §3.1 EVOLVES the API via root_vm.go + stream_context.go (replacing the 25.1 per-stream `*VM` with the root-VM + per-stream-context model).
- **25.1 SPEC §3.2** (8-file production split + abi/ subdirectory) — INHERITED + EXTENDED at §3.5 (3 NEW production files + per-family ABI dispatch files).
- **25.1 SPEC §3.3** (default-deny capability roster — 37-key materialized) — INHERITED + EXTENDED at §3.4 REUSE 8 (21 NEW capability keys; 37 → 58 cumulative).
- **25.1 SPEC §3.4** (per-stream `*wazero.Runtime` construction + per-module compile cache) — RETIRED at 25.2 per Q3 + ADR-0205 (root-VM lifecycle replaces per-stream Runtime construction).
- **25.1 SPEC §3.5** (`internal/filter/http/wasm/` 8-file production split) — INHERITED + EXTENDED at §3.6 (4 NEW production files + extensions to existing files).
- **25.1 SPEC §3.6** (boot-registration alphabetical position) — INHERITED UNCHANGED at 25.2 per §3.7.
- **25.1 SPEC §11.1** (D-S1 34-fuzzer count) — INHERITED; this 25.2 SPEC §8.5 adds D-S2 (35-fuzzer count VERIFIED at 25.2 SPEC commit).
- **25.1 SPEC §12** (D-P1..D-P6 SPEC-time D-questions) — ALL CLOSED at 25.1 IMPL per 25.1 PROGRESS; this 25.2 SPEC §12 introduces D-25.2-P1..D-25.2-P5.
- **25.1 SPEC §15.3** (30-item 25.1 IMPL acceptance checklist) — CLOSED at 25.1 IMPL Task 17; this 25.2 SPEC §15 introduces the 46-item 25.2 checklist.

---

## Appendix B — Phase 25.2 ADR landings summary

At THIS 25.2 SPEC commit: **4 NEW ADR §Context drafts consumed** (ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208). DECISIONS.md tail advances from ADR-0204 → ADR-0208. Next-free ADR advances to **ADR-0209** (reserved as the 25.2 IMPL escape-valve slot).

At 25.2 IMPL Final Task atomic landing:

- **ADR-0205 §Decision + §Consequences body** — Root VM lifecycle per Q3 + §3.1 + §5.3 root-context callbacks. The 25.2 SPEC §3.1 production signatures (RootVM + RootVMOption + StreamContext) + §3.5 file split (root_vm.go + stream_context.go + tick.go + shared_data.go) + §3.6 filter-package integration are ratified verbatim at IMPL. Per-stream Module instantiation R8 escape-valve discipline anchored (anticipated UNCONSUMED).
- **ADR-0206 §Decision + §Consequences body** — 25.2 ABI extensions per §3.1 + §5.1 + §5.3 + AMEND-B1 + AMEND-B2 + AMEND-B5. The 14 NEW env-namespace hostcalls + 7 NEW callbacks + 21 NEW capability keys + foreign-function registration interface per AMEND-A9 + buffer-clamp wire-contract + metric signedness + gate-at-registration architecture all ratified.
- **ADR-0207 §Decision + §Consequences body** — NEW `internal/filterstate/` framework primitive per Q7 + §3.2 + AMEND-B4 + ADR-0188 API-revision allowance NOT consumed. Phase-22.2 lua MIGRATES per §3.2 MIGRATION; consumer #2 = 25.2 wasm; future-consumer roster anchored.
- **ADR-0208 §Decision + §Consequences body** — `internal/filter/http/wasm/` 25.2 package extensions per §3.6 + 9 envoy-go-strict counters per §7.1 + AMEND-B3 + 4 envoy-go-strict-only config fields + dynamic-stats namespace per AMEND-B2 via NEW `internal/stats/dynamic/` infrastructure + mixed-mode fixture-0036 + boot-reject fixture-0037 + 25.2 BEHAVIOR_CONTRACT.md ~7-edit bundle + 35th fuzzer + per-plugin Registry scope discipline.
- **ADR-0202 §Consequences body** — one-line in-place AMEND acknowledgment paragraph per §10.2.
- **ADR-0209 §Context + §Decision + §Consequences body (CONDITIONAL — only if R8 escape-valve fires per D-25.2-P2)** — per-stream Module instantiation pattern (pooled vs shared-Module-with-mutex-serialization). Only consumes if `BenchmarkPerStreamModule_Instantiation > 1ms`. If unconsumed: ADR-0209 carries forward to 25.3 BRAINSTORM as the 25.3 IMPL escape-valve slot.

At 25.3 IMPL Final Task (forward-pointer): ADR-0210 (or +1) + ADR-0211 (or +1) + ADR-0212 (or +1) per parent §10.3 25.3 anticipated ADRs.

**End of phase 25.2 SPEC.**

