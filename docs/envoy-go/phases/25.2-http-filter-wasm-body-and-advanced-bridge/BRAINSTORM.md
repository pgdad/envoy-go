# Phase 25.2 Brainstorm — `envoy.filters.http.wasm` (advanced bridge surface delta)

**Sub-phase:** `25.2-http-filter-wasm-body-and-advanced-bridge` (second sub-phase of parent 25 per parent BRAINSTORM Q1 3-way PRE-SPLIT)
**Parent row:** `25-http-filter-wasm` (status `in-progress` per ROADMAP; parent row stays in-progress until 25.3 phase-done per parent SPEC §1 closure pattern matching 18/19/22/24 ROLLUP precedent)
**Predecessor:** `25.1-http-filter-wasm-runtime-and-headers-bridge` (status `done` at 2026-05-25; squash `feded64`; landed NEW `internal/wasm/` framework primitive + 24-hostcall headers-bridge ABI + 13-callback subset + 18-arm PARSE-REJECT roster + 5-counter stat surface + ADR-0202+0203+0204 + 3 envoy-go-strict departure records + R8 STANDS WEAK-default at 61µs/stream; 20 HTTP filters wired; 119 stat names; 34 fuzzers; 37 differential fixture dirs; DECISIONS.md tail at ADR-0204; next-free ADR-0205 UNCONSUMED)
**Successor:** `25.3-http-filter-wasm-perroute-and-conformance` (status `planned`; lands per-route TPFC 5th-canonical REUSE-by-absence per AMEND-A3 + multi-plugin VM-sharing + `VmConfig.environment_variables` activation + `failure_policy = FAIL_RELOAD` + conformance harness seed at 62.5% threshold per AMEND-A8)
**Authored at:** this BRAINSTORM commit (worktree `phase-25.2-http-filter-wasm-body-and-advanced-bridge-brainstorm` branched from master tip `3f5a448`; squash-merge to master per `feedback_git_worktrees.md` + ADR-0005 §Decision 4)

This sub-phase BRAINSTORM follows the per-sub-phase BRAINSTORM convention per ADR-0106 + matches the discipline shape of phase-22.2 (`../22.2-http-filter-lua-full-bridge/BRAINSTORM.md` — 14 Qs across 12 §2 numbered sub-sections + 4 transverse) + the parent 25 BRAINSTORM (`../25-http-filter-wasm/BRAINSTORM.md` — 9 Qs + 3 confirmations). Parent BRAINSTORM Q1 settled 25.1's headers-only foundational third + 25.2's advanced-bridge envelope and forward-pointed the full body+buffer+trailers+timer+metrics+shared-data+httpCall+foreign-function+full-property surface to this sub-phase; this BRAINSTORM settles every per-surface decision + the transverse framework / stat / fixture / ADR-consumption / D-hypothesis / scope-shape decisions for 25.2.

---

## 1. Mission and scope confirmation (25.2 only)

### 1.1 What 25.2 delivers as a self-contained whole

Phase 25.2 lands the FULL Envoy↔WASM advanced-bridge surface delta on top of 25.1's headers-only foundation, taking parent BRAINSTORM Q1 envelope D to its conclusion. By 25.2 phase-done every upstream-parity hostcall outside the per-route TPFC + multi-plugin VM-sharing + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` surfaces (which defer to 25.3) is active, plus ~6 envoy-go-strict departure records (covering body buffer + shared-data + tick period floor + foreign-function 0-vs-10 default registry + dynamic-stats namespace + 5-counter envoy-go-strict counter bundle per §5.3) + the foreign-function registration interface with EMPTY default registry per AMEND-A9.

The 25.2 surface delta (11 design dimensions; 11 Q-decisions):

- **Body bridge** (Q1): `proxy_on_request_body` + `proxy_on_response_body` per-chunk-invoke + accumulating buffer + CONTINUE/PAUSE dispatch — upstream-parity. Each `OnDecodeBuffer` / `OnEncodeBuffer` chunk triggers ONE `proxy_on_*_body(ctx, chunk_size_added, end_of_stream)` invocation; guest reads via `proxy_get_buffer_bytes(HTTP_REQUEST_BODY|HTTP_RESPONSE_BODY|HTTP_CALL_RESPONSE_BODY, start, max_size)` against the accumulated buffer; PAUSE buffers further chunks until guest returns CONTINUE on a subsequent invocation. Activates WasmBufferType values 0 (HttpRequestBody), 1 (HttpResponseBody), and 4 (HttpCallResponseBody) which were defined-but-unused at 25.1 per AMEND-A7.
- **Buffer-cap discipline** (Q2): envoy-go-strict 16 MiB hard cap on accumulated body buffer; operator-configurable via `PluginConfig.envoy_go_strict_body_buffer_cap_bytes`. On cap exceeded: stream closes with 413 Payload Too Large (decode-side) or response terminates (encode-side) + `wasm.<plugin>.body_buffer_cap_exceeded` envoy-go-strict counter + integration error log. Matches phase-22.2 `:fileBytes()` 16 MiB cap + envoy-go safety-first defaults pattern.
- **Trailers bridge**: `proxy_on_request_trailers(ctx, num_trailers) → CONTINUE|PAUSE` + `proxy_on_response_trailers` invoked only when trailers actually arrive (upstream-parity; operators rely on body callback `end_of_stream=true` for end-of-stream signaling). Trailer-map hostcalls reuse 25.1's `proxy_get/set/add/replace/remove_header_map_value` machinery + `proxy_get_header_map_pairs` + `proxy_get_header_map_size` against WasmHeaderMapType values 1 (HttpRequestTrailers) and 3 (HttpResponseTrailers).
- **Root VM lifecycle evolution** (Q3): ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per proxy-wasm-cpp-host's `Wasm`/`Plugin` model); per-stream contexts are CHILDREN sharing the root VM's wazero Runtime + Module. The root VM lifecycle owns `proxy_on_vm_start` + `proxy_on_configure` (fire once at config-load), the tick goroutine, the shared-data map, and the httpCall response routing. Per-stream `*StreamContext` creation becomes cheap (just calling `proxy_on_context_create(stream_id, root_id)`); the 25.1 per-stream `*wazero.Runtime` construction model is RETIRED. The per-stream Module instantiation pattern (fresh vs pooled vs single-Module-with-serialization) carries forward to 25.2 IMPL R8 escape-valve.
- **Timer dispatch** (Q5): `proxy_set_tick_period_milliseconds(period_ms)` from guest schedules per-root-VM `proxy_on_tick(root_ctx_id)` invocations. Implementation: per-root-VM dedicated goroutine running `for { select { case <-clock.After(period): vm.lockAndCall(proxy_on_tick); case <-stop: return } }`. envoy-go-strict 10ms period floor (clamps `max(period_ms, 10ms)` to prevent guest-driven CPU spin attacks). FIRST co-consumer of phase-21 ADR-0186 `Clock` seam. RATIFIES the phase-21 Clock-seam extraction discipline.
- **Outbound HTTP dispatch**: `proxy_http_call(cluster_name, headers, body, trailers, timeout, &call_id) → WasmResult` (10 arguments per proxy-wasm v0.2.1 spec); response routes to `proxy_on_http_call_response(ctx_id, call_id, num_headers, body_size, num_trailers)` via call_id token. RE-CONSUMES phase-20 `internal/httpclient/` primitive at THIRD-or-later co-consumer per ADR-0177 (phase-22.2's `:httpCall()` was second). NO API extension on httpclient (phase-22.2 already added cluster-based dispatch via in-place AMEND of ADR-0177). RATIFIES the phase-20 framework-primitive extraction discipline. CLOSES parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor.
- **httpCall unknown-cluster runtime disposition** (Q4): runtime `WasmResult::BadArgument` (=2) + integration error log + envoy-go-strict counter `wasm.<plugin>.http_call_dispatch_unknown_cluster`. Cluster-name pre-validation at config-load is structurally impossible (operator's `.wasm` bytecode is opaque to envoy-go's static analysis). Upstream-parity at the wire level + envoy-go-strict observability extension for operator visibility into bytecode-vs-cluster-name drift.
- **Shared-data** (Q6): `proxy_set_shared_data(key, value, cas)` + `proxy_get_shared_data(key, value, cas)` with CAS atomic-compare-and-set + WasmResult::CasMismatch (=8) on mismatch. Scope at 25.2: cross-stream within ONE PluginConfig (which has a singleton `vm_id` per 25.1 PARSE-REJECT arm 12). Storage: per-`*RootVM` `sharedDataMap` with sync.RWMutex. envoy-go-strict cap discipline: per-value 1 MiB cap + 1024-entry cap; cap exceeded returns `WasmResult::InternalFailure` + `wasm.<plugin>.shared_data_cap_exceeded` envoy-go-strict counter + integration error log; 2 envoy-go-strict-only `PluginConfig` config fields (`envoy_go_strict_shared_data_value_cap_bytes`, `envoy_go_strict_shared_data_max_entries`). Cross-plugin shared-data (multi-PluginConfig sharing one `vm_id`) extends to 25.3 with the multi-plugin VM-sharing surface.
- **Metrics + dynamic-stats namespace** (Q9): `proxy_define_metric(MetricType, name) → metric_id` + `proxy_increment_metric(id, offset)` + `proxy_record_metric(id, value)` + `proxy_get_metric(id) → value`. MetricType enum {Counter=0, Gauge=1, Histogram=2} per spec README. Dynamic stats land under `wasmcustom.<plugin_name>.<custom_name>` namespace (upstream Envoy convention) via NEW `internal/stats/dynamic.go` runtime-registration API (lazy-enumerated at admin `/stats` endpoint). envoy-go-strict 1024-entry cap on dynamic namespace + `wasm.<plugin>.dynamic_stats_cap_exceeded` envoy-go-strict counter + envoy-go-strict-only `envoy_go_strict_dynamic_stats_max_entries` config field.
- **Foreign-function call**: `proxy_call_foreign_function(name, args) → WasmResult` + `proxy_on_foreign_function` callback. Per AMEND-A9 RATIFIED-with-extensions: envoy-go ships NEW `internal/wasm/foreign.go` `ForeignFunctionRegistry` (Register/Get; sync.RWMutex + map[string]ForeignFunction) with EMPTY default registry; unregistered name returns `WasmResult::NotFound` (=1) byte-faithful to upstream cpp-host `exports.cc:147-184`; capability-gated via default-deny `proxy_call_foreign_function` capability key per AMEND-A5. envoy-go-strict departure record: upstream registers 10 foreign functions by default (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`); envoy-go registers ZERO. Operators MUST explicitly enable the capability + register specific foreign functions at multi-consumer scope. The registration interface lands NOW (at 25.2) rather than deferring to the WASM host family per the BRAINSTORM API-REVISION ALLOWANCE clause — extracting the small (~100-150 LIVE LoC) interface at 25.2 frees future cluster-specifier-wasm + access-logger-wasm + network-filter-wasm consumers from re-litigating the framework primitive's API at consumer #2.
- **Full stream-info property surface** (Q7): `proxy_get_property` extended from 25.1's 5-path minimal tree (`request.headers.*`, `response.headers.*`, `request.path`, `request.method`, `request.host`) to the FULL upstream Envoy CEL property tree (~25 distinct roots; ~80-100 sub-paths covering request/response/connection/upstream/downstream/source/destination/listener/route/xds/metadata/filter_state). Three co-consumed primitives + ONE NEW primitive extracted:
  - RE-CONSUMES phase-04 ADR-0144 `DownstreamPrincipal()` for `connection.tls.*` + `downstream.tls.*` branches (second co-consumer; first beyond phase-04 itself).
  - RE-CONSUMES phase-20 ADR-0177 `internal/httpclient/` cluster info for `upstream.cluster.*` branches (alongside the `proxy_http_call` co-consumption).
  - RE-CONSUMES phase-22.2 ADR-0190 `internal/dynamicmetadata/` for `metadata.*` branches (THIRD-or-later co-consumer per the cross-filter primitive design).
  - EXTRACTS NEW `internal/filterstate/` framework primitive for `filter_state.*` branches: consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` (in-package at 22.2 per Q9 EXTRACT-NOW-only-when-trigger-fires); consumer #2 = 25.2 wasm (TRIGGER FIRES). Phase-22.2's in-package implementation MIGRATES to consume the new primitive in a follow-up edit during 25.2 IMPL. Anchored at NEW ADR-0207.
- **Differential fixture `0036-http-wasm-body-and-advanced`** (Q8): single-listener single-HCM mixed-mode per phase-22.2 ADR-0192 precedent + the `freeTCPPort` flake mitigation per phase-22.2 REVIEW §7.4. Twelve-to-fourteen scenarios partitioned by assertion-class:
  - **Deterministic cross-side `CompareBytes`** (8-10 scenarios): body-read-only, body-mutate-passthrough, body-mutate-replace, trailers-add, trailers-read, shared-data-read-after-write, foreign-function-deny-default (returns NotFound per AMEND-A9), property-stream-info (request.method, response.code, connection.tls.version, upstream.cluster.name), metric-define-only (subject-side `wasmcustom.<plugin>.my_counter` discoverability), env-vars-rejected-passthrough (verifies 25.3-deferred PARSE-REJECT at 25.2).
  - **Non-deterministic subject-only `StatsAsserter.AssertStats`** (3-4 scenarios): tick-fires-counter (50ms period + 250ms probe wait; tick_invocations ≥ 5), httpCall-success (subject increments http_call_dispatched + http_call_response), httpCall-unknown-cluster (subject increments http_call_dispatch_unknown_cluster), body-cap-exceeded (PAUSE-loop + 32 MiB body → 413 + body_buffer_cap_exceeded increment).
  - Per `reference_differential_asserter_dispatch` memo: every subject-side assertion arm gets a deliberate-break liveness verification (NOT dead-vacuous per phase-23 fixture-0030 lesson). httpCall scenarios use a SECOND upstream cluster definition (NOT a second listener — avoids `freeTCPPort` flake).
- **Boot-reject fixture `0037-http-wasm-body-and-advanced-boot-reject`**: PGV-mirror reject single-arm for a 25.2-new PARSE-REJECT arm. Anticipated arms (final selection deferred to IMPL via empirical-scrape per 25.1 D-P6 precedent): malformed `envoy_go_strict_body_buffer_cap_bytes = 0` envoy-go-strict-only validator OR cross-PluginConfig duplicate `PluginConfig.name`. Single-arm boot-reject parity matches phase-22.2 + phase-23 + 25.1 fixture-0035 precedent.
- **35th project-wide fuzzer `FuzzWasmHostcallEnvelope`**: ~30-40 corpus seeds covering hostcall argument-envelope edge cases (wasm linear memory pointer/size bounds; max-size buffer reads), proxy-wasm pairs serialization adversarial inputs, foreign-function call name length boundaries, dynamic-stats name validation, shared-data CAS-mismatch races, body-buffer cap boundary cases, property-path syntax adversarials, tick period parsing. Must-never-panic invariant covers all 25.2 hostcall surfaces.

Plus transverse decisions:

- **ADR consumption discipline** (Q10): strict scope per phase-22.2 Q10 precedent. 4 NEW ADRs at 25.2 IMPL (ADR-0205 root VM lifecycle + ADR-0206 25.2 ABI extensions + foreign-function registration interface per AMEND-A9 + ADR-0207 `internal/filterstate/` extraction + ADR-0208 filter package extensions + envoy-go-strict counter bundle + mixed-mode fixture + dynamic-stats namespace) + reserve ADR-0209 for escape-valve. ADR-0202 gains a one-line in-place AMEND acknowledgment paragraph; ADR-0202's API-REVISION ALLOWANCE clause remains SCOPED to consumer #2 (WASM host family). STRENGTHENED-WEAK-HOLD-with-1-slot-buffer disposition at SPEC commit.
- **Stat surface delta**: +8 envoy-go-strict counters at 25.2 (project 119 → 127); 4 envoy-go-strict-only `PluginConfig` config fields; ~6 NEW envoy-go-strict departure records (consolidated bundle); dynamic-stats namespace `wasmcustom.<plugin_name>.<custom_name>` operator-extensible at runtime (NOT counted in static total).
- **Scope shape** (Q11): stay single sub-phase 25.2 at this BRAINSTORM commit; PLAN-stage split-gate fires if needed. Avoids speculative split-at-BRAINSTORM with imperfect task estimates. Matches phase-22.2 Q14 precedent exactly. ROADMAP row 25.2 stays `planned` as a single sub-row at this BRAINSTORM commit; flips to `in-progress` at 25.2 SPEC; flips to `done` at 25.2 IMPL phase-done.

### 1.2 What 25.2 does NOT deliver (forward to 25.3 / WASM host family / future)

Items DEFERRED to 25.3 (per parent BRAINSTORM Q1 + parent SPEC §3.1):

- **Per-route `Wasm` 5th-canonical wholesale-override via TPFC** (25.3) — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 18. 5th-canonical REUSE-by-absence per AMEND-A3; ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment. ADR-0210 anchors at 25.3 IMPL.
- **Multi-plugin VM-sharing via `vm_id`** (25.3) — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 12 (duplicate vm_id PARSE-REJECT). At 25.3 multiple `PluginConfig` entries with the same `vm_id` share a single VM instance, instantiating distinct plugin contexts under it. Mirrors upstream Envoy's `Wasm Service` discipline scoped to the HTTP filter. Cross-plugin shared-data scoping (shared-data visibility across PluginConfigs sharing one vm_id) opens at 25.3 BRAINSTORM/SPEC; the 25.2 shared-data scope is intentionally narrower (cross-stream within ONE PluginConfig only).
- **`VmConfig.environment_variables` activation** (25.3) — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 13. WASI `environ_*` shims return zeros at 25.2 (inherited from 25.1); 25.3 feeds them from `EnvironmentVariables.host_env_keys` + `key_values`.
- **`PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig`** (25.3) — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 9. 25.3 activates with the `wasm.<plugin>.{vm_reload, vm_reload_backoff, vm_reload_success, vm_reload_failure}` Group-C counter surface per AMEND-A2.
- **`PluginConfig.fail_open` deprecated bool** (25.3) — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 10. 25.3 maps onto `failure_policy = FAIL_OPEN` per AMEND-A1 ladder.
- **`test/conformance/proxy-wasm/` conformance harness seed** (25.3) — opens at 25.3 IMPL with 62.5% starting pass-threshold per AMEND-A8 + ADR-0212.

Items DEFERRED to WASM host family (out-of-row entirely per parent SPEC §2.x):

- **Shared-queue hostcalls (4) + `proxy_on_queue_ready` callback** (parent SPEC §2.15). Cross-VM (cross-vm_id) coordination at WasmService scope.
- **Outbound gRPC hostcalls (5) + 4 gRPC callbacks** (parent SPEC §2.17). Large surface intersecting `internal/grpcclient/` at multiple integration points; deferred to WASM host family.
- **TCP/network-filter hostcalls + callbacks** (parent SPEC §2.6). network-filter-wasm row out-of-scope at phase 25.
- **WasmService singleton plugin loaders** + **cluster-specifier-wasm** + **access-logger-wasm** + **network-filter-wasm** (parent SPEC §2.5 + §2.6). Multi-consumer §9 WASM host family.

Items DEFERRED forward (envelope choices):

- **Cross-side byte-exact for 25.2 non-deterministic scenarios** — partial cross-side at 25.2 with subject-only fallback for tick + httpCall + metric-emission per phase-22.2 ADR-0192 precedent (Q8 mixed-mode taxonomy).
- **wazero JIT/AOT compiler backend** — interpreter default at 25.x; opt-in deferred to future ops-tuning phase per parent SPEC §2.7.
- **Memory-trap fixture scenarios** — parent SPEC §2.9 OUT OF SCOPE at 25.1 + 25.2 per §4.5 D6 guardrails (wazero traps with different error strings than V8). May land at 25.3-or-later with mixed-mode discipline.

### 1.3 25.2's relationship to parent 25 BRAINSTORM Q1 envelope D + parent SPEC §3.1 surface-mapping

Parent BRAINSTORM Q1 settled the ambition: envelope D = full upstream parity by phase-25 phase-done across the 3-way pre-split. Parent SPEC §3.1 surface-mapping table pre-decided WHICH fields/surfaces CONSUMED at 25.2 vs PARSE-REJECT-deferred to 25.3. 25.2 takes parent SPEC §3.1's hand-off and lands the FULL ADVANCED-BRIDGE DELTA: every hostcall registered-as-stub-Unimplemented at 25.1 becomes a real implementation at 25.2 (except shared-queue + gRPC + network-filter hostcalls which stay stub-Unimplemented through 25.3 — those are WASM host family scope per parent SPEC §2.15 + §2.17).

The 25.2 surface is substantively larger than 25.1 along three structural axes:

1. **Hostcall count**: 25.1 = 24 hostcalls (16 `proxy_*` env-namespace + 8 `wasi_snapshot_preview1.*` shims); 25.2 adds ~20-25 NEW hostcalls (body/buffer × 3 buffer types; trailer map hostcalls × 4 trailer types; timer; metrics × 4; shared-data × 2; httpCall; foreign-function call; extended property paths). Total post-25.2 = ~44-49 hostcalls registered (out of the 47-hostcall full v0.2.1 surface; the remaining 3-4 stub-Unimplemented are shared-queue + gRPC subsets deferred to WASM host family).
2. **Callback count**: 25.1 = 13 callbacks (5 module-init/allocator + 6 lifecycle + 2 HTTP); 25.2 adds ~7-8 NEW callbacks (`proxy_on_request_body`, `proxy_on_response_body`, `proxy_on_request_trailers`, `proxy_on_response_trailers`, `proxy_on_tick`, `proxy_on_http_call_response`, `proxy_on_foreign_function`). Total post-25.2 = ~20-21 callbacks.
3. **Primitive evolution**: 25.1 introduced `internal/wasm/` framework primitive (consumer #1 = HTTP wasm filter). 25.2 evolves it via root VM lifecycle (Q3) + foreign-function registration interface (AMEND-A9) + extends via the 25.2 ABI surface. 25.2 also EXTRACTS NEW `internal/filterstate/` framework primitive at consumer #2 (after phase-22.2's in-package landing at consumer #1).

Two intentional envoy-go-strict scope-expansions beyond bare upstream parity at 25.2:

1. **Cap-discipline at body buffer + shared-data + dynamic-stats namespace** (Qs 2/6/9). Three new envoy-go-strict-only `PluginConfig` config fields + 4 envoy-go-strict counters (body_buffer_cap_exceeded + shared_data_cap_exceeded + dynamic_stats_cap_exceeded + http_call_dispatch_unknown_cluster from Q4). Rationale: upstream Envoy relies on operator-configured HCM-level + listener-level memory ceilings; envoy-go-strict adds in-filter caps as defense-in-depth (matches phase-22.2 `:fileBytes()` 16 MiB cap pattern + the security-first defaults discipline established at phase-04 TLS strict + phase-10 header_mutation pre-mutation-allowlist + phase-22 lua default-deny SandboxConfig).
2. **Tick period 10ms floor** (Q5). envoy-go-strict departure record clamps `max(period_ms, 10ms)` to prevent guest-driven CPU spin attacks. Upstream Envoy v1.37.2 has no floor; envoy-go-strict adds one. The phase-25 default-deny capability gate already provides first defense (proxy_set_tick_period_milliseconds must be in `allowed_capabilities`), but the floor defends against behavioral risk once the capability is enabled.

### 1.4 ADR-0045 split-by-surface readiness — staying single-phase at BRAINSTORM (Q11)

Per Q11: 25.2 stays as ONE ROADMAP sub-row at this BRAINSTORM commit. Estimated scope from Q1-Q10 + transverse decisions: **~22-28 tasks + ~5,000-7,500 LIVE production LoC**. The LoC half of the ADR-0045 split-gate (~1500 LoC) fires comfortably; the task-arm gate (~25 tasks) is borderline at the upper bound. Per parent SPEC §3.0's estimate (20-24 tasks + 2,730-4,250 LoC), this BRAINSTORM revises UPWARD due to Q3 root-VM lifecycle evolution (~800-1,000 LoC) + Q7 full property surface (~600-800 LoC) + filterstate primitive extraction + migration (~250-400 LoC including phase-22.2 lua MIGRATES delta) + Qs 2/6/9 cap-discipline infrastructure (~400-500 LoC).

The PLAN session does precise estimation against the gate; if it exceeds the task-arm gate, 25.2 splits into 25.2.1 + 25.2.2 at PLAN time per the phase-09 → phase-11 + phase-13 + phase-22.2-stayed-single split-at-PLAN precedent (ROADMAP + STATE update; BRAINSTORM not invalidated). Pre-splitting at BRAINSTORM was rejected on rationale grounds matching phase-22.2 Q14: imperfect task estimates this early force a suboptimal split-axis; the natural axes for a 25.2 sub-split (per-stream-extension vs root-context-extension; OR body-bridge vs control-bridge; OR pre-filterstate-extraction vs post-filterstate-extraction) are not unambiguously cleaner than letting PLAN see the real Task graph and decide.

### 1.5 Phase 25.1 IMPL inheritance state

25.2 inherits the following state from 25.1 IMPL (master tip `3f5a448` = `next-prompt.txt: repoint master-tip references to 7d4fa33 (actual HEAD)` — docs-only repoint post-25.1; predecessor `7d4fa33` = `next-prompt.txt: rewrite for 25.2 BRAINSTORM cold-start (post-25.1-IMPL feded64)`; predecessor `de4f853` = `phase 25.1 IMPL stage-close: STATE.md SHA-fill (TBD-25.1-IMPL -> feded64)`; predecessor `feded64` = the 25.1 IMPL squash):

- **20 HTTP filters wired** (`envoy.filters.http.wasm` is the EIGHTEENTH and FINAL §9 family-row; 25.2 does NOT add a §9 row — it extends the existing wasm filter).
- **119 stat names** (5 new at 25.1: `wasm.wazero.{created,active}` Group-B + `wasm.<plugin>.{executions, hostcall_denied, envoy_go.failures}` envoy-go-strict per AMEND-A2; 25.2 anticipates +8 → 127 per §1.1 + Q9).
- **34 fuzzers** (25.1 added `FuzzWasmConfigParse`; 25.2 anticipates +1: `FuzzWasmHostcallEnvelope`).
- **37 differential fixture directories** (25.1 added `0034-http-wasm-headers-bridge` + `0035-http-wasm-boot-reject`; 25.2 anticipates +2: `0036-http-wasm-body-and-advanced` mixed-mode + `0037-http-wasm-body-and-advanced-boot-reject`).
- **DECISIONS.md tail at ADR-0204** with full §Decision + §Consequences bodies (ADR-0202 NEW `internal/wasm/` framework primitive; ADR-0203 NEW `internal/filter/http/wasm/` package shape; ADR-0204 default-deny capability sandbox). ADR-0125 §(xv) at 10 canonicals UNCHANGED (NO §(xvi) amendment per AMEND-A3 — RETIRED at parent SPEC commit).
- **Next-free ADR-0205** carries forward UNCONSUMED from 25.1 IMPL (D-P4 R8 STANDS WEAK-default at `BenchmarkPerStreamVM_Construction_Headers` `ns/op = 61000` ~61µs/stream — well under 1ms threshold; ADR-0205 NOT consumed; carries forward as the 25.2 IMPL escape-valve slot per R8 signaling protocol). 25.2 anticipates ADR-0205 → ADR-0208 consumption per Qs 3/7/10 (root VM lifecycle + 25.2 ABI extensions + foreign-function registration + filterstate extraction + filter package extensions + envoy-go-strict counter bundle + mixed-mode fixture + dynamic-stats namespace) + conditional ADR-0209 reserve.
- **`internal/wasm/` framework primitive** anchored at ADR-0202 — 25.2 EVOLVES via NEW ADR-0205 + ADR-0206 per Q10 strict scope (root VM lifecycle + ABI extensions + foreign-function registration interface per AMEND-A9); ADR-0202's API-REVISION ALLOWANCE clause STAYS scoped to consumer #2 (WASM host family). ADR-0202 gains a one-line in-place AMEND acknowledgment paragraph in §Consequences noting the consumer-#1-internal-scope evolution at 25.2.
- **`internal/filter/http/wasm/` package shape** anchored at ADR-0203 — 25.2 EXTENDS via NEW ADR-0208 (full hostcall wiring + 8 envoy-go-strict counters + dynamic-stats namespace + 4 envoy-go-strict-only config fields + mixed-mode fixture-0036 discipline + 25.2 BEHAVIOR_CONTRACT bundle).
- **`internal/httpclient/` framework primitive** anchored at ADR-0177 (phase-20) — 25.2 RE-CONSUMES at third-or-later co-consumer (phase-22.2 was second per `:httpCall()` cluster-based dispatch in-place AMEND). NO API extension; the cluster-based dispatch API added at phase-22.2 IMPL covers 25.2's `proxy_http_call` shape byte-for-byte.
- **`internal/dynamicmetadata/` framework primitive** anchored at ADR-0190 (phase-22.2) — 25.2 RE-CONSUMES at third-or-later co-consumer for `proxy_get_property "metadata.*"` paths. NO API extension; ADR-0190's per-stream `*Bucket` accessor + map[(filter_name, key)]google.protobuf.Value shape maps cleanly.
- **`internal/lua/` framework primitive** anchored at ADR-0188 (phase-22.1) — UNCHANGED at 25.2 (independent VM-class primitive; 25.2 wasm does NOT consume).
- **`Clock` seam** anchored at ADR-0186 (phase-21) — 25.2 FIRST co-consumer beyond phase-21 itself via the tick dispatcher goroutine. RATIFIES the phase-21 Clock-seam extraction.
- **3 envoy-go-strict departure records** at 25.1 (default-deny capability sandbox per ADR-0204; ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6; consolidated bundle covering Remote-AsyncDataSource + runtime-name PARSE-REJECTs + 3 envoy-go-strict counters); 25.2 adds 6 more per §1.1 + Section 5.4 (body cap + shared-data cap + tick period floor + foreign-function 0-vs-10 default registry + dynamic-stats cap + 5-counter envoy-go-strict bundle).
- **18-arm 25.1 PARSE-REJECT roster** + byte-stable wording (D-P5 CLOSED at 25.1 Task 9) — 25.2 may EXTEND with new arms (e.g., timer-period-required; metric-name-required; httpCall-cluster-required; foreign-function-name-required). Anticipated +8-12 arms at 25.2 (settled at SPEC).

### 1.6 Cross-phase EXTRACT-NOW-on-second-consumer discipline — filter-state at 25.2 (intentional per Q7)

Phase 22.2 lua landed `:filterState()` IN-PACKAGE at `internal/filter/http/lua/filterstate.go` per Q9 EXTRACT-NOW-only-when-trigger-fires posture — at the time, the project had no cross-filter state primitive and no committed second consumer. The IN-PACKAGE landing was the conservative posture; phase-22.2's `:streamInfo():filterState()` Lua surface lives in-package.

Phase 25.2's `proxy_get_property "filter_state.*"` paths require the same primitive surface. This is the second consumer — the EXTRACT-NOW-on-second-consumer trigger fires per the discipline ADR-0188 established at phase-22.1 for `internal/lua/`. 25.2 EXTRACTS `internal/filterstate/` as NEW framework primitive (~150-250 LIVE LoC: per-stream `*Bucket` accessor + `FilterStateObject` interface (Set/Get/Marshal/HasData/StateType) + sync semantics matching phase-22.2's in-package implementation).

**MIGRATION**: phase-22.2's `internal/filter/http/lua/filterstate.go` REWRITES to consume the new primitive (the lua bridge's `:filterState()` accessor delegates to `internal/filterstate/*Bucket`; ~50-100 LoC migration delta inside `internal/filter/http/lua/`). The migration is non-breaking — the `:filterState()` Lua surface stays the same; only the underlying storage layer flips from in-package map to shared primitive.

Anchored at NEW ADR-0207 (Q7 + Section 3.2 + Section 7.3). ADR-0188's API-revision allowance clause is NOT consumed by this filterstate extraction (the `internal/lua/` framework primitive itself is untouched; only the in-package filterstate.go file migrates).

Future phase BRAINSTORMs that need filter-state access from their respective filters (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families) consume `internal/filterstate/` rather than re-litigate the primitive shape. ADR-0207 §Consequences body documents this cross-phase deferral-lift expectation.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 11 Q-decisions. Each is anchored here with rationale + the anticipated ADR or REUSE classification.

### 2.1 Body callback dispatch model: per-chunk invoke + accumulating buffer *(Q1 → ADR-0206)*

**Decision:** Each `OnDecodeBuffer` / `OnEncodeBuffer` chunk triggers ONE `proxy_on_request_body(ctx, chunk_size, end_of_stream)` invocation (similarly for response body). Guest reads via `proxy_get_buffer_bytes(HttpRequestBody|HttpResponseBody|HttpCallResponseBody, start, max_size)` against an accumulated buffer that grows per chunk. If guest returns PAUSE, filter buffers further chunks downstream-side; next chunk's invocation passes the new accumulated `body_size`. Matches upstream Envoy behavior + proxy-wasm-cpp-host semantics; matches phase-22.2 lua's `:bodyChunks()` chunked-iterator philosophy (option (b) in phase-22.2 Q1).

**Rationale:** Buffer-to-end + single invoke (option (b) in this Q1) was rejected on three grounds: (i) breaks upstream byte-exact for chunked-aware guests porting from upstream Envoy — operators get subtle behavioral differences; (ii) forces always-buffer at the filter, wasting perf on body-passthrough scripts; (iii) cannot be opt-out without a new envoy-go-strict config field (option (c) hybrid would have added that field but with no upstream-parity user-base requesting it). Per-chunk-invoke (option (a)) is the upstream-parity default and the operator-pattern-coverage maximum.

Activates WasmBufferType values 0 (HttpRequestBody) + 1 (HttpResponseBody) + 4 (HttpCallResponseBody) which were defined-but-unused at 25.1 per AMEND-A7.

**Anticipated ADR:** ADR-0206 §Decision section details the 25.2 ABI extensions including the body dispatch model + buffer-bounds error semantics. ADR-0208 §Decision documents the filter-side `OnDecodeBuffer` / `OnEncodeBuffer` consumption pattern.

### 2.2 Body-buffer cap discipline: 16 MiB envoy-go-strict default + configurable *(Q2 → ADR-0208 + BEHAVIOR_CONTRACT departure record)*

**Decision:** envoy-go-strict 16 MiB hard cap on accumulated body buffer; operator-configurable via `PluginConfig.envoy_go_strict_body_buffer_cap_bytes` (uint32; default 16777216). On cap exceeded (accumulated body bytes > cap): stream closes with HTTP 413 Payload Too Large (decode-side) or response terminates (encode-side) + `wasm.<plugin>.body_buffer_cap_exceeded` envoy-go-strict counter increments + integration error log emitted. envoy-go-strict departure from upstream's reliance on operator-configured HCM-level + listener-level memory ceilings.

**Rationale:** Q1's per-chunk accumulating buffer creates unbounded-memory risk if guest returns PAUSE every invocation. Upstream Envoy has no built-in cap; relies on HCM `per_connection_buffer_limit_bytes` (default 1 MiB upstream) + listener memory ceilings. Operators may not realize the PAUSE-loop risk pattern; envoy-go-strict adds in-filter cap as defense-in-depth. The cap is operator-overridable via the envoy-go-strict-only config field — operators with legitimate large-body inspection use cases (e.g., file-upload AV scanning) can raise the cap. The configuration-not-compile-time-constant choice (vs option (c) envoy-go-strict-stricter compile-time constant) preserves operator flexibility.

The 16 MiB default matches phase-22.2 `:fileBytes()` cap + the broader project security-first defaults pattern (phase-04 TLS strict; phase-10 header_mutation pre-mutation-allowlist; phase-22 lua default-deny `SandboxConfig`).

**Anticipated ADR:** ADR-0208 §Decision section documents the cap discipline + the envoy-go-strict-only `envoy_go_strict_body_buffer_cap_bytes` config field + the 413-on-exceed semantic + the counter increment. BEHAVIOR_CONTRACT departure record at the 25.2 IMPL final Task bundle.

### 2.3 Root VM lifecycle: ONE long-lived root VM per *compiledConfig + per-stream contexts as CHILDREN *(Q3 → NEW ADR-0205; ADR-0202 in-place AMEND acknowledgment)*

**Decision:** ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per proxy-wasm-cpp-host's `Wasm`/`Plugin` model). Constructed at config-load via `proxy_on_vm_start(root_ctx_id, vm_configuration_size)` + `proxy_on_configure(plugin_ctx_id, plugin_configuration_size)` (fire once at root context). Persists for plugin lifetime. Per-stream contexts are CHILDREN of the root VM (sharing the same wazero Runtime + Module): each `DecodeHeaders` creates a child stream-context ID via `proxy_on_context_create(stream_ctx_id, root_ctx_id)`; `OnDestroy` fires `proxy_on_done(stream_ctx_id) → bool` + `proxy_on_delete(stream_ctx_id)`. The root VM owns: tick goroutine + tick state; shared-data map; httpCall response routing + call_id allocation; foreign-function registry view.

The 25.1 per-stream `*VM` (each stream constructing a fresh `*wazero.Runtime` at 61µs/stream cost per the R8 benchmark) is RETIRED at 25.2. 25.2's per-stream `*StreamContext` creation is ~microseconds (just calling `proxy_on_context_create` + bookkeeping; no wazero Runtime construction).

**Per-stream Module instantiation pattern** (fresh vs pooled vs single-Module-with-serialization) carries forward to 25.2 IMPL R8 escape-valve per the parent SPEC §13-R8 threshold (> 1ms per-stream cost triggers ADR-0209 reserve consumption). 25.1's Task 7 follow-up cross-runtime CompiledModule re-compile pattern becomes UNNECESSARY at 25.2 (per-stream contexts share Module instance via context-IDs, not via re-compile); `Module.Source()` retained-bytes mechanism survives only for the root-VM-construction code path.

**Rationale:** Hybrid model (option (b)) was rejected on upstream-byte-exact grounds — separate root + per-stream Runtime+Module memory spaces break shared-data visibility timing (a per-stream `proxy_get_shared_data` would proxy to the root VM's memory map; the value visibility timing differs from upstream cpp-host where shared-data lives in a host-managed Go map accessible from any context). Option (c) (context-switching via `proxy_set_effective_context` on shared Module) was rejected on idiomatic-mapping-to-cpp-host grounds — cpp-host's Context class hierarchy maps cleanly onto option (a)'s root + child contexts.

The model is structurally healthier than 25.1's per-stream Runtime because: (i) per-stream context creation cost is dramatically lower (microseconds vs 61µs); (ii) the wazero CompilationCache is hit once per config-load + shared across all stream contexts; (iii) cross-stream state (shared-data + dynamic-stats + tick state) lives in one well-defined home (the root VM); (iv) upstream cpp-host byte-exact for hostcall dispatch + shared-data + tick + httpCall response routing.

The model is structurally riskier in ONE dimension: per-stream isolation. wazero's `Module` instance is NOT goroutine-safe (concurrent stream callbacks on the same Module instance must be serialized OR each per-stream context gets its own Module instance from the shared CompiledModule). The per-stream Module instantiation pattern decision (fresh per-stream Module instantiation vs pooled instances vs shared-Module-with-mutex-serialization) is the R8 escape-valve territory; SPEC anchors the decision; IMPL benchmarks gate.

**Anticipated ADR:** NEW ADR-0205 §Decision details the root VM lifecycle + `*RootVM` API shape (NewRootVM + Configure + Tick + DispatchHttpCall + Close) + per-stream context model + the per-stream Module instantiation R8 escape-valve discipline. ADR-0202 gains a one-line in-place AMEND acknowledgment paragraph in §Consequences (per Q10 strict-scope precedent).

### 2.4 httpCall unknown-cluster runtime disposition: BadArgument + envoy-go-strict counter *(Q4 → ADR-0206 + ADR-0208 + BEHAVIOR_CONTRACT departure record)*

**Decision:** When the guest calls `proxy_http_call("unknown_cluster", ...)`, envoy-go matches upstream proxy-wasm-cpp-host wire: dispatch returns `WasmResult::BadArgument` (=2); call_id token NOT allocated; no `proxy_on_http_call_response` invocation. Plus envoy-go-strict observability extension: `wasm.<plugin>.http_call_dispatch_unknown_cluster` counter increments + integration error log emitted. Cluster-name pre-validation at config-load is structurally impossible (operator's `.wasm` bytecode is opaque; envoy-go cannot know which cluster names the guest will dispatch to at runtime).

**Rationale:** Option (b) (return InternalFailure) was rejected on upstream-parity-wire-divergence grounds (cpp-host returns BadArgument; would be envoy-go-strict-stricter departure without clear rationale). Option (c) (pure upstream-parity; no envoy-go-strict counter) was rejected on operator-observability grounds — bytecode-vs-cluster-name drift is exactly the kind of operator-actionable signal envoy-go's stat surface enriches over upstream (matches phase-19 ext_proc envoy-go-strict additions + phase-21 RTT-gauge departure + phase-22 lua executions + respond_calls extensions). The envoy-go-strict counter pattern is consistent with the broader `wasm.<plugin>.<metric>` family established at 25.1.

**Anticipated ADRs:** ADR-0206 §Decision documents the `proxy_http_call` 10-argument hostcall + wire-level BadArgument return semantic. ADR-0208 §Decision documents the envoy-go-strict counter addition + the consolidated 5-counter envoy-go-strict departure record at the 25.2 IMPL final Task bundle.

### 2.5 Timer dispatch model + period floor: per-root-VM goroutine + 10ms envoy-go-strict floor + Clock seam *(Q5 → ADR-0205 + ADR-0208 + BEHAVIOR_CONTRACT departure record + RATIFIES ADR-0186)*

**Decision:** `proxy_set_tick_period_milliseconds(period_ms)` from guest schedules per-root-VM `proxy_on_tick(root_ctx_id)` invocations. Implementation: each `*RootVM` owns ONE dedicated goroutine running:

```go
for {
    select {
    case <-clock.After(effectivePeriod):
        rootVM.lockAndCall(proxy_on_tick, rootCtxID)
    case <-stop:
        return
    }
}
```

`effectivePeriod = max(period_ms, 10ms)` — envoy-go-strict 10ms period floor + envoy-go-strict departure record. Uses ADR-0186 `Clock` seam injection at construction time for fixture fake-time support (`clock.After(d)` returns a channel; fake clock implementations advance discretely per test discipline). The tick callback acquires the root VM's mutex (per §2.3 root-VM model). If a per-stream context is concurrently invoking a hostcall on the root-shared Module, the tick waits.

**Rationale:** Shared dispatcher (option (b)) was rejected on shared-global-state + Clock-seam-complication grounds — Clock seam injection becomes more awkward when shared across goroutines vs per-VM-owned. NO envoy-go-strict floor (option (c)) was rejected on guest-behavioral-risk grounds — a malicious or buggy plugin can set period=0 → hot-loop on a goroutine → CPU saturation. The phase-25 default-deny capability gate already addresses capability-level risk (proxy_set_tick_period_milliseconds must be in `allowed_capabilities`), but the floor addresses behavioral risk once the capability is enabled. The 10ms floor matches the broader project safety-first defaults pattern; operators with legitimate < 10ms timer use cases can override at runtime by re-issuing `proxy_set_tick_period_milliseconds(period < 10ms)` which gets silently clamped at the host side (envoy-go-strict departure record documents the clamp).

Goroutine count grows linearly with #PluginConfigs (typically 1-5 in production, sometimes up to dozens at multi-plugin-VM-sharing scale at 25.3). Not a scalability concern at envoy-go's anticipated deployment scale.

FIRST co-consumer of phase-21 ADR-0186 `Clock` seam beyond phase-21 itself; RATIFIES the phase-21 Clock-seam extraction discipline.

**Anticipated ADRs:** ADR-0205 §Decision documents the per-root-VM tick goroutine + Clock seam injection. ADR-0208 §Decision documents the `wasm.<plugin>.tick_invocations` envoy-go-strict counter. BEHAVIOR_CONTRACT departure record at 25.2 IMPL final Task bundle covers the 10ms period floor envoy-go-strict departure.

### 2.6 Shared-data scope + caps: per-RootVM map + 1 MiB value cap + 1024-entry cap *(Q6 → ADR-0206 + ADR-0208 + BEHAVIOR_CONTRACT departure record)*

**Decision:** `proxy_set_shared_data(key_ptr, key_size, value_ptr, value_size, cas) → WasmResult` + `proxy_get_shared_data(key_ptr, key_size, value_ptr_ptr, value_size_ptr, cas_ptr) → WasmResult`. CAS atomic-compare-and-set: `cas=0` unconditionally writes (returns new CAS value in subsequent get); `cas>0` writes only if existing entry's CAS value matches (returns `WasmResult::CasMismatch` (=8) on mismatch). Scope at 25.2: cross-stream within ONE PluginConfig (per 25.1 PARSE-REJECT arm 12 singleton vm_id constraint; cross-plugin defers to 25.3 with multi-plugin VM-sharing per parent BRAINSTORM §1.1).

Storage: per-`*RootVM` `sharedDataMap` (Go `map[string]sharedDataEntry`; `sharedDataEntry struct { value []byte; cas uint32 }`; sync.RWMutex). envoy-go-strict cap discipline: per-value 1 MiB cap (configurable via `envoy_go_strict_shared_data_value_cap_bytes`; default 1048576); 1024-entry cap (configurable via `envoy_go_strict_shared_data_max_entries`; default 1024). Cap exceeded returns `WasmResult::InternalFailure` (=10) + `wasm.<plugin>.shared_data_cap_exceeded` envoy-go-strict counter + integration error log.

**Rationale:** Option (b) (no envoy-go-strict caps; upstream-parity) was rejected on phase-22.2 + Q2 + project safety-first defaults pattern grounds — once the capability gate enables `proxy_set_shared_data`, the host has no defense against unbounded namespace creation; the cap is exactly the structural defense. Option (c) (per-value cap only; no entry-count cap) was rejected on per-key-accumulation-risk grounds — a guest that uses `proxy_set_shared_data` in a loop with distinct keys (e.g., per-request-id key) can fill the namespace without exceeding any per-value cap; the entry-count cap defends against that pattern.

The cap defaults (1 MiB value + 1024 entries) are conservative; operators with legitimate large shared-state use cases can raise either via the envoy-go-strict-only config fields. The CAS semantics are upstream-byte-exact (matches proxy-wasm-cpp-host `wasm.cc:Wasm::setSharedData`/`Wasm::getSharedData`).

**Anticipated ADRs:** ADR-0206 §Decision documents the 2 shared-data hostcalls + CAS semantic. ADR-0208 §Decision documents the cap discipline + 2 envoy-go-strict-only config fields + cap_exceeded counter + envoy-go-strict departure record. BEHAVIOR_CONTRACT departure record at 25.2 IMPL final Task bundle (consolidated with body-cap record per §5.4).

### 2.7 Full stream-info property surface + filterstate primitive disposition: full upstream parity + EXTRACT-NOW *(Q7 → ADR-0207; co-consumes ADR-0144 + ADR-0177 + ADR-0190)*

**Decision:** Land the COMPLETE upstream property tree at 25.2 (~25 distinct roots; ~80-100 sub-paths covering request/response/connection/upstream/downstream/source/destination/listener/route/xds/metadata/filter_state). The roster maps to four primitive sources:

1. **Stream-local accessors (no co-consumed primitive)** — `request.*` (headers, body, trailers, path, method, host, scheme, time, duration, total_size, size); `response.*` (headers, body, trailers, code); `source.*` + `destination.*` (address, port); `listener.*` (direction, metadata, address); `route.*` (name, metadata); `xds.*` (cluster_name, cluster_metadata, route_name, route_metadata, virtual_host_name). Implementation: new `internal/wasm/property.go` (or similar) maps proxy-wasm property paths to per-stream callbacks + bookkeeping. ~400-600 LIVE LoC.

2. **RE-CONSUMES phase-04 ADR-0144 `DownstreamPrincipal()`** — `connection.tls.*` + `downstream.tls.*` branches (TLS principal: subject, SANs local/peer, validFrom, expirationPeer, sessionId, ciphersuiteId, tlsVersion, urlEncodedPemEncoded*Certificate*, sha256PeerCertificateDigest). Second co-consumer beyond phase-04 itself.

3. **RE-CONSUMES phase-20 ADR-0177 `internal/httpclient/`** — `upstream.*` branches (address, port, cluster_name, cluster_metadata, host) via the same cluster manager + LB integration that powers `proxy_http_call`. Co-consumption alongside the dispatch hostcall.

4. **RE-CONSUMES phase-22.2 ADR-0190 `internal/dynamicmetadata/`** — `metadata.*` branches (filter dynamic metadata; typed dynamic metadata) via `*Bucket.Get(filter_name, key) → google.protobuf.Value`. THIRD-or-later co-consumer (phase-22.2 lua bridge was first; phase-22.2 deferred-pickup is second; 25.2 is third or later depending on whether any intervening phase touched).

5. **EXTRACTS NEW `internal/filterstate/` framework primitive** — `filter_state.*` branches. Per Q7 EXTRACT-NOW-on-second-consumer trigger (phase-22.2 lua's in-package landing was consumer #1; 25.2 wasm is consumer #2). Anchored at NEW ADR-0207. ~150-250 LIVE LoC primitive + ~50-100 LoC migration delta in `internal/filter/http/lua/filterstate.go` (consumer #1 rewrites to delegate to the new primitive; the `:filterState()` Lua surface stays the same — non-breaking).

**Rationale:** Pragmatic-middle (option (c) ~30 sub-paths) was rejected on operator-demand-under-anticipation grounds — phase-22.2 chose full parity for similar reasons; pragmatic-middle decisions at BRAINSTORM time tend to under-anticipate operator demand. In-package filterstate (option (b) phase-22.2-precedent-no-extraction) was rejected on EXTRACT-NOW-on-second-consumer-discipline grounds — the discipline ADR-0188 established at phase-22.1 (for `internal/lua/`) applies symmetrically to `internal/filterstate/` at 25.2 (the second consumer triggers extraction). Staying in-package would force duplicate filter-state access code between `internal/filter/http/lua/` and `internal/filter/http/wasm/` — violates the project's design discipline.

The full surface is ~25-30 distinct property roots × ~3-5 sub-paths each = ~80-100 total paths. Implementation effort is non-trivial (~600-900 LIVE LoC across `internal/wasm/property.go` + `internal/filterstate/` primitive + `internal/filter/http/lua/` migration), but the API surface is mostly mechanical (property-path string → primitive accessor mapping).

**Anticipated ADR:** ADR-0207 §Decision details the new `internal/filterstate/` framework primitive (per-stream `*Bucket` accessor + `FilterStateObject` interface + sync semantics) + the cross-phase consumer roster (consumer #1 = 22.2 lua MIGRATES; consumer #2 = 25.2 wasm) + the EXTRACT-NOW-on-second-consumer discipline. ADR-0206 §Decision documents the full property-path roster at 25.2.

### 2.8 Mixed-mode fixture-0036 topology + scenario count: single-listener + per-scenario assertion-class dispatch + 12-14 scenarios *(Q8 → ADR-0208 + fixture-0036 directory + RATIFIES ADR-0192)*

**Decision:** Single-listener single-HCM hosting the wasm filter + router terminator. Each scenario chooses its own assertion strategy: deterministic scenarios use `CompareBytes` (cross-side byte-exact); non-deterministic scenarios use `StatsAsserter.AssertStats` (subject-only per `reference_differential_asserter_dispatch`). 12-14 scenarios partitioned by class (8-10 deterministic + 3-4 non-deterministic; see Section 6.2 for the full taxonomy).

httpCall scenarios use a SECOND upstream cluster definition (NOT a second listener — avoids the `freeTCPPort` combined-run flake risk per phase-22.2 REVIEW §7.4). Every subject-side assertion arm gets a deliberate-break liveness verification per `reference_differential_asserter_dispatch` memo.

**Rationale:** Multi-listener (option (b) phase-22.2 fixture-0027 pattern) was rejected on `freeTCPPort` flake-risk grounds — phase-22.2 REVIEW §7.4 flagged this exact risk; phase-22.2 + phase-25.1 fixtures successfully avoided it by using single-listener; 25.2 inherits the pattern. Deterministic-only at 25.2 (option (c) with deferred subject-only follow-up fixture) was rejected on integration-test-coverage grounds — tick + httpCall need *some* end-to-end test surface at 25.2 IMPL; unit tests can't exercise the full stream-runner + filter + wazero integration; the StatsAsserter pattern proves these surfaces work end-to-end without forcing cross-side comparability.

The 12-14 scenario count balances breadth (covers all 25.2 hostcall surfaces) with flake-risk-reduction (fewer scenarios per directory = less concurrent state to coordinate). The 2-listener configuration via a second upstream cluster (not a second listener) maps cleanly to envoy-go's existing cluster manager + LB integration.

**Anticipated ADR:** ADR-0208 §Decision documents the single-listener mixed-mode discipline + the 12-14 scenario partition + the deliberate-break liveness-verification contract per `reference_differential_asserter_dispatch`. RATIFIES phase-22.2 ADR-0192 mixed-mode precedent at second-occurrence scope.

### 2.9 Dynamic-stats namespace + 25.2 envoy-go-strict counter tally: 8 counters + 1024-entry dynamic cap *(Q9 → ADR-0208 + BEHAVIOR_CONTRACT departure record)*

**Decision:** Land 3 envoy-go-strict counters from Qs 2/4/6 (`body_buffer_cap_exceeded` per Q2, `http_call_dispatch_unknown_cluster` per Q4, `shared_data_cap_exceeded` per Q6) + 4 envoy-go-strict counters from parent SPEC §5.2 forward-pointers (`tick_invocations`, `http_call_dispatched`, `http_call_response`, `foreign_function_denied`) + 1 ADDITIONAL counter from this Q9 (`dynamic_stats_cap_exceeded`) = **8 envoy-go-strict counters at 25.2**. Plus the open-ended dynamic-stats namespace `wasmcustom.<plugin_name>.<custom_name>` via NEW `internal/stats/dynamic.go` Register-at-define-metric + admin /stats lazy-enumerate. envoy-go-strict 1024-entry cap on dynamic namespace; envoy-go-strict-only `envoy_go_strict_dynamic_stats_max_entries` config field. Project total stat count: **119 → 127 at 25.2 phase-done** (parent SPEC §5.2 anticipated 119 → 123; revised UPWARD by 4 due to Qs 2/4/6/9 envoy-go-strict cap-discipline departures).

**Rationale:** Option (b) (unbounded dynamic-stats namespace) was rejected on safety-first-defaults grounds — a malicious plugin can fill the stats namespace via `proxy_define_metric` loop → unbounded memory growth at admin /stats enumeration; the dynamic-stats cap is a structural defense against unbounded namespace creation. Option (c) (drop the 3 cap-exceeded counters; rely on integration error logs only) was rejected on operator-observability grounds — the cap-exceeded events are exactly the operator-visible signals that the cap discipline is firing; dropping them defeats the cap discipline's diagnostic value.

The dynamic-stats namespace `wasmcustom.<plugin_name>.<custom_name>` matches upstream Envoy convention byte-faithfully. Registration uses envoy-go's existing `internal/stats/` registry's runtime-extensibility (proven at phase-14 per-library stats + phase-18 per-grpc-service stats); the NEW `internal/stats/dynamic.go` is a thin wrapper exposing Register-at-runtime + Lookup-by-id.

**Anticipated ADR:** ADR-0208 §Decision details the 8 envoy-go-strict counter additions + the 4 envoy-go-strict-only config fields (body buffer cap + shared-data value cap + shared-data max entries + dynamic-stats max entries) + the `wasmcustom.<plugin_name>.<custom_name>` namespace registration via `internal/stats/dynamic.go` + the consolidated envoy-go-strict departure record bundle at the 25.2 IMPL final Task per ADR-0052 atomic landing.

### 2.10 ADR consumption discipline at 25.2: strict scope + 4 NEW + 1 reserve *(Q10 → ADR-0205+0206+0207+0208+0209; ADR-0202 one-line in-place AMEND)*

**Decision:** Strict scope per phase-22.2 Q10 precedent. The Q3 root-VM lifecycle evolution + Q7 filterstate extraction land as NEW ADRs (not in-place AMENDs to ADR-0202 or ADR-0203). 5 ADRs at 25.2 (4 NEW + 1 reserve):

- **ADR-0205** anchors root VM lifecycle + per-stream `*StreamContext` model + per-stream Module instantiation R8 escape-valve discipline.
- **ADR-0206** anchors 25.2 ABI extensions (body/buffer/trailers/timer/metrics/shared-data/httpCall/foreign-function/extended-property) + the in-house `internal/wasm/foreign.go` ForeignFunctionRegistry per AMEND-A9.
- **ADR-0207** anchors NEW `internal/filterstate/` framework primitive + the consumer #1 (22.2 lua) MIGRATES + EXTRACT-NOW-on-second-consumer discipline.
- **ADR-0208** anchors `internal/filter/http/wasm/` 25.2 package extensions + 8 envoy-go-strict counters + dynamic-stats namespace + 4 envoy-go-strict-only config fields + fixture-0036 mixed-mode discipline + the 25.2 BEHAVIOR_CONTRACT bundle.
- **ADR-0209** reserved for escape-valve (likely candidates: per-stream Module instantiation R8 escape-valve if benchmark > 1ms surfaces; OR a SPEC-time empirical-discovery surface in any 25.2 hostcall implementation).

ADR-0202 gains a **one-line in-place AMEND acknowledgment paragraph** in §Consequences (per ADR-0044 in-place edit discipline): *"Phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern carries forward to 25.2 IMPL R8 escape-valve). The EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent."* No new ADR number consumed for the in-place AMEND.

**Rationale:** In-place AMEND of ADR-0202 (option (b)) was rejected on lineage-clarity grounds — ADR-0202's lineage covers consumer-#1 EXTRACT (the framework primitive's birth at 25.1); consumer-#1-internal-scope evolution at 25.2 is structurally distinct from consumer-#2 API-revision; mixing them in one ADR §Decision body makes downstream traceability harder. Phase-22.2 Q10 set the strict-scope precedent (the analogous decision for `internal/lua/` evolution at 22.2); 25.2 inherits the discipline. Option (c) (WEAK-HOLD-with-2-slot-buffer) was rejected as over-conservative — the parent SPEC §1.2 STRENGTHENED-WEAK-HOLD-with-1-slot-buffer disposition stands; 25.2's additional empirical-pin closures (parent SPEC AMEND-A1..A9 carries forward) further reduces escape-valve risk vs the BRAINSTORM-time 2-slot estimate.

Anticipated next-free ADR after 25.2 phase-done: **ADR-0210** if reserve UNCONSUMED (4 numbers consumed: ADR-0205..ADR-0208); **ADR-0211** if reserve consumed (5 numbers: ADR-0205..ADR-0209).

**Anticipated ADR work at this BRAINSTORM commit:** zero — the 4 NEW ADR §Context drafts anchor at 25.2 SPEC commit (per ADR-0044). This BRAINSTORM only anticipates the consumption discipline.

### 2.11 25.2 scope shape: stay single sub-phase + PLAN-stage split-gate arbiter *(Q11 → ROADMAP row 25.2 stays single)*

**Decision:** Stay single sub-phase 25.2 at this BRAINSTORM commit. PLAN session does precise task-graph estimation against the ADR-0045 split-gate; if it exceeds the task-arm gate (~25 tasks), 25.2 splits into 25.2.1 + 25.2.2 at PLAN time per the phase-09 → phase-11 + phase-13 + phase-22.2-stayed-single split-at-PLAN precedent (ROADMAP + STATE update; BRAINSTORM not invalidated). ROADMAP row 25.2 stays `planned` as a single sub-row at this BRAINSTORM commit; flips to `in-progress` at 25.2 SPEC; flips to `done` at 25.2 IMPL phase-done.

**Rationale:** Pre-splitting at BRAINSTORM (option (b) into 25.2.1 + 25.2.2) was rejected on rationale grounds matching phase-22.2 Q14: imperfect task estimates this early force a suboptimal split-axis. The natural axes for a 25.2 sub-split (per-stream-extension vs root-context-extension; OR body-bridge vs control-bridge; OR pre-filterstate-extraction vs post-filterstate-extraction) are not unambiguously cleaner than letting PLAN see the real Task graph and decide. Trim-scope (option (c) deferring metric hostcalls to 25.3) was rejected on AMEND-A9-RATIFIES-at-25.2 implication grounds + 25.3-overload-risk grounds — parent SPEC §3.0 estimated 25.3 at 9-12 tasks; adding metric hostcalls would push 25.3 into split territory by itself.

The BRAINSTORM-time scope estimate (~22-28 tasks + ~5,000-7,500 LIVE LoC) is borderline on the task-arm gate at the upper bound; PLAN session is the right arbiter. The LoC-arm gate (~1500 LoC) fires comfortably; that's the expected pattern for advanced-hostcall-bundle sub-phases (phase-22.2 similarly exceeded the LoC-arm half of the gate at landing time; no further split was triggered).

**Anticipated ADR:** None — the stay-single decision is a precedent extension not requiring new ADR (ADR-0045 § 6 already covers the split-discipline; the application here is the standard BRAINSTORM-defers-to-PLAN pattern).

---

## 3. Framework-survey result — 1 NEW package-level primitive + 1 NEW package + 3 internal/wasm/ API evolutions + 7 REUSES

Phase 25.2 introduces **1 NEW package-level framework primitive** (`internal/filterstate/` per Q7 + AMEND-A9-adjacent EXTRACT-NOW discipline) + **1 NEW infrastructure package** (`internal/stats/dynamic.go` per Q9) + **3 internal/wasm/ API evolutions** (root VM lifecycle per Q3; ABI surface extension per Section 2.1/2.4/2.5/2.6/2.9; foreign-function registration interface per AMEND-A9) + **0 in-place ADR amendments at 25.2 except the ADR-0202 one-line acknowledgment** + **7 framework REUSES** + **1 MIGRATES** (phase-22.2 `internal/filter/http/lua/filterstate.go` rewrites to consume the new `internal/filterstate/` primitive).

### 3.1 `internal/wasm/` API evolutions *(per Q3 + Q5 + Q6 + Q9 + AMEND-A9; anchored at ADR-0205 + ADR-0206)*

**Three API evolutions** within the existing `internal/wasm/` framework primitive at 25.1 IMPL:

1. **Root VM lifecycle** (Q3 → ADR-0205): NEW `internal/wasm/root_vm.go` carrying `*RootVM` type + `NewRootVM(opts...) *RootVM` constructor + `RootVM.Configure(vm_config_bytes, plugin_config_bytes) error` + `RootVM.NewStreamContext() *StreamContext` + `RootVM.Close()` + tick goroutine internals + shared-data map + httpCall response routing. The 25.1 per-stream `*VM` type RENAMES + EVOLVES into `*StreamContext` (no functional break — same exported method set but the lifecycle is parent-anchored at RootVM rather than self-anchored).

2. **25.2 ABI surface extension** (Sections 2.1-2.9 → ADR-0206): EXTEND `internal/wasm/registration.go` ABICallbacks interface with ~7-8 new methods (`OnRequestBody`/`OnResponseBody`/`OnRequestTrailers`/`OnResponseTrailers`/`OnTick`/`OnHttpCallResponse`/`OnForeignFunction`). EXTEND `internal/wasm/sandbox.go` capability key roster with ~20-25 NEW capability keys. NEW `internal/wasm/tick.go` per-root-VM tick goroutine + 10ms period floor. NEW `internal/wasm/shared_data.go` per-RootVM CAS-protected K-V map + 1 MiB value cap + 1024-entry cap. NEW `internal/wasm/property.go` full proxy-wasm property-path tree mapping. NEW `internal/wasm/dynamic_stats.go` proxy_define_metric dispatch + 1024-entry cap. EXTEND `internal/wasm/wasi.go` body-buffer routing if needed (likely UNCHANGED).

3. **Foreign-function registration interface** (AMEND-A9 → ADR-0206): NEW `internal/wasm/foreign.go` `ForeignFunctionRegistry` (Register/Get; sync.RWMutex + map[string]ForeignFunction). EMPTY default registry per AMEND-A9; envoy-go-strict departure from upstream's 10-default-functions. The registration interface lands NOW (at 25.2) rather than deferring to the WASM host family per the BRAINSTORM API-REVISION ALLOWANCE clause — extracting the small (~100-150 LIVE LoC) interface at 25.2 frees future cluster-specifier-wasm + access-logger-wasm + network-filter-wasm consumers from re-litigating the framework primitive's API at consumer #2.

ADR-0202 gains the one-line in-place AMEND acknowledgment paragraph per Q10 strict scope (the consumer-#1-internal evolution at 25.2 absorbed under ADR-0205+0206; ADR-0202's API-REVISION ALLOWANCE clause remains SCOPED to consumer #2 WASM host family).

### 3.2 NEW `internal/filterstate/` framework primitive *(per Q7; anchored at ADR-0207; lands at 25.2)*

**Decision:** Extract `internal/filterstate/` as NEW framework primitive at 25.2 second-consumer scope (consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES; consumer #2 = 25.2 wasm `internal/filter/http/wasm/property_filterstate.go` or similar). Per-stream `*Bucket` accessor + `FilterStateObject` interface (Set/Get/Marshal/HasData/StateType) + sync semantics matching phase-22.2's in-package implementation.

**Package boundary**: `internal/filterstate/` hosts the GENERIC per-stream filter-state primitive (cross-filter-reusable; no HTTP-filter-specific knowledge). `internal/filter/http/lua/filterstate.go` consumes the primitive via thin adapter (`:filterState()` Lua surface stays UNCHANGED — non-breaking). `internal/filter/http/wasm/property_filterstate.go` (or equivalent) consumes the primitive for `proxy_get_property "filter_state.*"` paths.

**API shape (provisional; settled at 25.2 SPEC; this BRAINSTORM anchors the shape per ADR-0188 + ADR-0190 cross-filter primitive precedent):**

```go
// Bucket is the per-stream filter-state accessor. NOT goroutine-safe;
// access is per-stream-single-goroutine by envoy-go's filter dispatch model.
type Bucket struct { /* unexported */ }

// FilterStateObject is the value stored in the bucket.
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

// NewBucket constructs a per-stream bucket.
func NewBucket() *Bucket

// Set stores a FilterStateObject under the given key.
func (b *Bucket) Set(key string, obj FilterStateObject) error

// Get retrieves a FilterStateObject by key.
func (b *Bucket) Get(key string) (FilterStateObject, bool)

// Keys lists all currently-set keys for iteration.
func (b *Bucket) Keys() []string
```

**MIGRATION**: phase-22.2's `internal/filter/http/lua/filterstate.go` REWRITES to consume the new primitive (the lua bridge's `:filterState()` accessor delegates to `internal/filterstate/*Bucket`; ~50-100 LoC migration delta inside `internal/filter/http/lua/`). The migration is non-breaking — the `:filterState()` Lua surface stays the same; only the underlying storage layer flips from in-package map to shared primitive.

**Anticipated ADR:** ADR-0207 §Decision documents the NEW `internal/filterstate/` framework primitive + the consumer #1 (22.2 lua) MIGRATES + EXTRACT-NOW-on-second-consumer discipline + the future-consumer roster (anticipated consumers in future phases: rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families).

### 3.3 NEW `internal/stats/dynamic.go` infrastructure *(per Q9; anchored at ADR-0208; lands at 25.2)*

**Decision:** NEW thin wrapper `internal/stats/dynamic.go` exposing Register-at-runtime + Lookup-by-id for the `wasmcustom.<plugin_name>.<custom_name>` dynamic-stats namespace. Uses envoy-go's existing `internal/stats/` registry's runtime-extensibility (proven at phase-14 per-library stats + phase-18 per-grpc-service stats). API shape (~100-150 LIVE LoC):

```go
// MetricID is an opaque token returned from Register.
type MetricID uint32

// MetricType identifies counter/gauge/histogram.
type MetricType int
const (
    MetricTypeCounter MetricType = iota
    MetricTypeGauge
    MetricTypeHistogram
)

// Registry is the per-plugin-config dynamic-stats namespace.
type Registry struct { /* unexported; sync.RWMutex + map */ }

// NewRegistry constructs a per-plugin-config registry.
func NewRegistry(pluginName string, maxEntries int) *Registry

// Register stores a new metric under wasmcustom.<plugin_name>.<name>
// and returns the MetricID. Returns NOT_FOUND if cap exceeded.
func (r *Registry) Register(metricType MetricType, name string) (MetricID, error)

// Increment / Record / Get operate by MetricID.
func (r *Registry) Increment(id MetricID, offset int64) error
func (r *Registry) Record(id MetricID, value int64) error
func (r *Registry) Get(id MetricID) (int64, error)

// EnumerateForAdmin walks the registry for /stats lazy enumeration.
func (r *Registry) EnumerateForAdmin(fn func(name string, value int64))
```

**Anticipated ADR:** Folded into ADR-0208 §Decision (alongside the filter package extensions + envoy-go-strict counter bundle).

### 3.4 REUSES (7 frameworks at 25.2 + 1 capability-gate sub-reuse; +1 MIGRATES)

- **REUSE 1: HCM-parse-time PARSE-REJECT path** — adds 25.2-new PARSE-REJECT arms (anticipated +8-12 arms; e.g., timer-period-required; metric-name-required; httpCall-cluster-required; foreign-function-name-required; envoy-go-strict-only config field validators). Total arm count post-25.2: ~26-30.
- **REUSE 2: per-request filter interface (decode + encode hooks + body + trailer hooks)** — decode-side body via `OnDecodeBuffer`; encode-side body via `OnEncodeBuffer`; trailers via `DecodeTrailers` / `EncodeTrailers`. The async-resume pattern for `proxy_http_call` reuses the fault / ext_authz precedent (filter returns `StopAndBuffer` / `StopAndAllIteration`; resumes via filter callbacks when response arrives). Body buffer cap exceeded uses `SendLocalReply` (413 Payload Too Large).
- **REUSE 3: phase-20 `internal/httpclient/`** — 25.2 RE-CONSUMES at third-or-later co-consumer per ADR-0177 (phase-22.2's `:httpCall()` was second). NO API extension; the cluster-based dispatch API added at phase-22.2 IMPL covers 25.2's `proxy_http_call` shape byte-for-byte. RATIFIES the phase-20 framework-primitive extraction discipline; closes parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor.
- **REUSE 4: phase-21 `Clock` seam** — ADR-0186 FIRST co-consumer beyond phase-21 itself. The tick dispatcher goroutine injects `Clock` at construction time for fixture fake-time support. RATIFIES the phase-21 Clock-seam extraction.
- **REUSE 5: phase-22.2 `internal/dynamicmetadata/`** — ADR-0190 third-or-later co-consumer for `proxy_get_property "metadata.*"` paths. NO API extension; the per-stream `*Bucket` accessor + map[(filter_name, key)]google.protobuf.Value shape maps cleanly.
- **REUSE 6: phase-04 ADR-0144 `DownstreamPrincipal()`** — second co-consumer beyond phase-04 itself; powers `connection.tls.*` + `downstream.tls.*` property tree branches.
- **REUSE 7: 25.1 `internal/wasm/abi/types.go`** — the 10-named-value `WasmResult` enum (with value-gaps at 5/9/11 per AMEND-A7) + `WasmBufferType` (values 0/1/4 activated at 25.2: HttpRequestBody/HttpResponseBody/HttpCallResponseBody) + `WasmHeaderMapType` (values 1/3 activated at 25.2: HttpRequestTrailers/HttpResponseTrailers).
- **REUSE 8 (capability-gate sub-reuse): 25.1 `internal/wasm/sandbox.go` default-deny gate** — extends with 25.2 hostcall capability keys (~20-25 NEW keys); the gate function itself unchanged; default-deny posture inherited per AMEND-A5.

**MIGRATES (1):**
- **phase-22.2 `internal/filter/http/lua/filterstate.go`** — REWRITES to consume the new `internal/filterstate/` primitive under ADR-0207 (Section 3.2). Non-breaking; `:filterState()` Lua surface unchanged.

NO new `internal/` package beyond `internal/filterstate/` (+ the thin `internal/stats/dynamic.go` infrastructure). NO top-level primitive package outside `internal/`. NO new go.mod direct dependency (wazero v1.10.1 + proxy-wasm-rust-sdk =0.2.4 inherited from 25.1 per AMEND-A1).

---

## 4. Per-route shape — out-of-scope at 25.2 (defers to 25.3)

Per parent BRAINSTORM Q5 + AMEND-A3: per-route `Wasm` override is the 5th-canonical wholesale-override pattern via `typed_per_filter_config` (REUSE-by-absence; NO `WasmPerRoute` proto exists in v1.32.4 binding OR v1.37.2 IDL). ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment.

At 25.2: PARSE-REJECT `typed_per_filter_config` Wasm entries per parent SPEC §6.2 arm 18 (via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint). 25.3 activates the wholesale-override resolver (REUSES existing TPFC mechanism per parent SPEC §3.3 REUSE 4) under NEW ADR-0210 anchoring the EXPLICIT-NO-NEW-CANONICAL classification (analogous to ADR-0173 / ADR-0180).

25.2 BRAINSTORM does NOT re-litigate the per-route shape decision; the parent BRAINSTORM Q5 + parent SPEC AMEND-A3 set the disposition.

---

## 5. Stat surface hypothesis

### 5.1 25.2 envoy-go-strict counter delta (BRAINSTORM disposition; SPEC ratifies at empirical-pin)

| # | Name | Type | Source | Semantics | Origin |
|---|---|---|---|---|---|
| 1 | `wasm.<plugin>.tick_invocations` | counter | envoy-go-strict | Increments per `proxy_on_tick` invocation. Operator visibility into tick dispatch rate. | parent SPEC §5.2 |
| 2 | `wasm.<plugin>.http_call_dispatched` | counter | envoy-go-strict | Increments per `proxy_http_call` invocation (successful dispatch + dispatched to upstream cluster). | parent SPEC §5.2 |
| 3 | `wasm.<plugin>.http_call_response` | counter | envoy-go-strict | Increments per `proxy_on_http_call_response` invocation. | parent SPEC §5.2 |
| 4 | `wasm.<plugin>.foreign_function_denied` | counter | envoy-go-strict | Increments per `proxy_call_foreign_function` invocation that returns NotFound (empty default registry per AMEND-A9). | parent SPEC §5.2 |
| 5 | `wasm.<plugin>.body_buffer_cap_exceeded` | counter | envoy-go-strict | Increments when accumulated body buffer exceeds `envoy_go_strict_body_buffer_cap_bytes` (default 16 MiB); stream closes with 413 or response terminates. | Q2 |
| 6 | `wasm.<plugin>.http_call_dispatch_unknown_cluster` | counter | envoy-go-strict | Increments per `proxy_http_call` to unknown cluster (returns BadArgument). | Q4 |
| 7 | `wasm.<plugin>.shared_data_cap_exceeded` | counter | envoy-go-strict | Increments when `proxy_set_shared_data` exceeds value cap (1 MiB default) OR entry-count cap (1024 default). | Q6 |
| 8 | `wasm.<plugin>.dynamic_stats_cap_exceeded` | counter | envoy-go-strict | Increments when `proxy_define_metric` exceeds dynamic-stats entry cap (1024 default). | Q9 |

**Plus operator-extensible dynamic namespace**: `wasmcustom.<plugin_name>.<custom_name>` per upstream Envoy convention; counter/gauge/histogram via `proxy_define_metric`; runtime-registered via `internal/stats/dynamic.go`; lazy-enumerated at admin `/stats` endpoint. NOT counted in static stat name total.

**4 NEW envoy-go-strict-only `PluginConfig` config fields** (Qs 2/6/9):
- `envoy_go_strict_body_buffer_cap_bytes` (uint32; default 16777216 = 16 MiB)
- `envoy_go_strict_shared_data_value_cap_bytes` (uint32; default 1048576 = 1 MiB)
- `envoy_go_strict_shared_data_max_entries` (uint32; default 1024)
- `envoy_go_strict_dynamic_stats_max_entries` (uint32; default 1024)

(Q5's 10ms tick period floor is a compile-time constant per Section 2.5, NOT a config field.)

### 5.2 Project stat-count delta

**119 → 127 at 25.2** (+8 envoy-go-strict counters; project total). Parent SPEC §5.2 anticipated +4; revised UPWARD by 4 due to Qs 2/4/6/9 cap-discipline departures. Anticipated 25-family-final stat count at 25.3 phase-done: **~127-131** (+0-4 if Group-C `wasm.<plugin>.vm_reload*` activates at 25.3 per parent SPEC §2.22 + AMEND-A2; settles at 25.3 BRAINSTORM/SPEC).

### 5.3 envoy-go-strict departure record delta

Baseline post-25.1: **~21 records** (18 pre-25.1 + 3 at 25.1). 25.2 adds **~6 NEW records** (consolidated where related):

- **Record #1 (25.2)**: 5-counter consolidated bundle covering tick_invocations + http_call_dispatched + http_call_response + foreign_function_denied + http_call_dispatch_unknown_cluster envoy-go-strict counter additions per Q4 + parent SPEC §5.2.
- **Record #2 (25.2)**: body buffer cap discipline per Q2 (16 MiB default + `envoy_go_strict_body_buffer_cap_bytes` envoy-go-strict-only config + body_buffer_cap_exceeded counter + 413-on-exceed semantic).
- **Record #3 (25.2)**: shared-data cap discipline per Q6 (1 MiB value cap + 1024-entry cap defaults + 2 envoy-go-strict-only config fields + shared_data_cap_exceeded counter).
- **Record #4 (25.2)**: tick period 10ms floor per Q5 (envoy-go-strict clamp departure from upstream's no-floor behavior).
- **Record #5 (25.2)**: foreign-function 0-vs-10 default registry per AMEND-A9 (envoy-go ships EMPTY registry; upstream registers 10 by default).
- **Record #6 (25.2)**: dynamic-stats cap discipline per Q9 (1024-entry cap default + `envoy_go_strict_dynamic_stats_max_entries` envoy-go-strict-only config + dynamic_stats_cap_exceeded counter + `wasmcustom.<plugin_name>.<custom_name>` namespace registration via `internal/stats/dynamic.go`).

Post-25.2 departure record count: **~27** (21 + 6). 25.3 adds more if cross-plugin VM-sharing surfaces additional departures.

### 5.4 BEHAVIOR_CONTRACT.md edit bundle at 25.2 IMPL final Task (per ADR-0052 atomic landing)

Per parent SPEC §13.5 anticipation + Qs accumulated. ~7-edit bundle at 25.2 IMPL final-Task (slightly larger than 25.1's 6-edit bundle):

1. EXTEND `### envoy.filters.http.wasm` subsection with body+buffer+trailers+timer+metrics+shared-data+httpCall+foreign-function+full-property bridge details (~150-250 NEW lines on top of 25.1 content).
2. Stat-table 119 → 127 extension with 8 new rows + `wasmcustom.<plugin_name>.<custom_name>` dynamic namespace structural-note row.
3. envoy-go-strict departure record #1: 5-counter consolidated bundle (Q4 + parent §5.2).
4. envoy-go-strict departure record #2: body buffer cap discipline (Q2).
5. envoy-go-strict departure record #3: shared-data cap discipline (Q6) + tick period 10ms floor (Q5).
6. envoy-go-strict departure record #4: foreign-function 0-vs-10 default registry (AMEND-A9) + dynamic-stats cap discipline (Q9) + `wasmcustom.*` namespace.
7. EXTEND/RENAME `### Phase 25.1 forward-pointer notes` → `### Phase 25.2 forward-pointer notes` subsection: 25.2 hand-off lifts items + 25.3-anticipated additions (per-route TPFC 5th-canonical REUSE-by-absence per AMEND-A3; multi-plugin VM-sharing; conformance harness seed at 62.5% per AMEND-A8; `VmConfig.environment_variables` activation; `failure_policy = FAIL_RELOAD` activation).

---

## 6. Differential fixture envelope — two directories (1 cross-side-mixed + 1 boot-reject) at 25.2

Per project memory `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side-mixed + boot-reject surfaces live in SEPARATE directories. Total fixture-dir delta at 25.2: **+2 dirs** (37 → 39 at 25.2 phase-done; family total 35 → 41 across all 25.x).

### 6.1 25.2 fixture-0036 cross-side-mixed (single-listener mixed-mode per Q8 + ADR-0192 precedent)

`0036-http-wasm-body-and-advanced` lands single-listener single-HCM mixed-mode per Q8 + the phase-22.2 ADR-0192 precedent. Subdirectory layout follows 25.1 fixture-0034 precedent: `inputs/driver.go`, `scripts/<scenario>/{Cargo.toml,src/lib.rs}`, `bytecode/<scenario>.wasm` vendored Rust-sourced.

**Scenario taxonomy (12-14 scenarios)** partitioned by assertion-class:

**Deterministic — cross-side `CompareBytes` (8-10 scenarios):**

| # | Name | Plugin behavior | Wire assertion |
|---|---|---|---|
| (a) | body-read-only | `proxy_on_request_body(ctx, size, end_of_stream)` reads body via `proxy_get_buffer_bytes`; logs size; CONTINUE | Reflected body unchanged at upstream |
| (b) | body-mutate-passthrough | Read body; modify via `proxy_set_buffer_bytes`; CONTINUE | Reflected body modified byte-exactly |
| (c) | body-mutate-replace | Replace full body in `proxy_on_request_body(end_of_stream=true)`; CONTINUE | Reflected body replaced |
| (d) | trailers-add | `proxy_add_header_map_value(HttpRequestTrailers, ...)` in `proxy_on_request_trailers` | Reflected trailer present at upstream |
| (e) | trailers-read | Read trailer count + values; add response header `x-trailer-count: N` | Reflected `x-trailer-count` header |
| (f) | shared-data-read-after-write | Stream 1 writes `proxy_set_shared_data("key", "value", 0)`; Stream 2 reads `proxy_get_shared_data("key")` and echoes via response header `x-shared-value` | Stream 2 response has `x-shared-value: value`; CAS conflict path tested via additional probe |
| (g) | foreign-function-deny-default | `proxy_call_foreign_function("verify_signature", ...)` returns NotFound (=1; AMEND-A9 EMPTY default registry) | Plugin records WasmResult into response header `x-ff-result: 1`; envoy-go-strict counter `foreign_function_denied` increments (subject-side StatsAsserter assertion alongside cross-side) |
| (h) | property-stream-info | `proxy_get_property("request.method"/"response.code"/"connection.tls.version"/"upstream.cluster.name")` → response headers `x-prop-*` | Reflected `x-prop-*` headers byte-exact (scenarios chosen with deterministic property values) |
| (i) | metric-define-only | `proxy_define_metric(COUNTER, "my_counter")` at `proxy_on_configure` (root context); NO increment | Subject-side: dynamic stat `wasmcustom.<plugin>.my_counter` appears at `/stats` with value 0; cross-side: response unchanged (deterministic) |
| (j) | env-vars-rejected-passthrough | (Verifies 25.3-deferred `VmConfig.environment_variables` PARSE-REJECT at 25.2; plugin reads via `wasi_snapshot_preview1.environ_*` and gets zeros) | Reflected response with `x-env-count: 0` |

**Non-deterministic — subject-only `StatsAsserter.AssertStats` (3-4 scenarios):**

| # | Name | Plugin behavior | Subject-side assertion |
|---|---|---|---|
| (k) | tick-fires-counter | Set 50ms period; `proxy_on_tick` increments custom dynamic stat `wasmcustom.<plugin>.tick_count` | After 250ms probe wait, dynamic stat ≥ 5; built-in counter `wasm.<plugin>.tick_invocations` ≥ 5 |
| (l) | httpCall-success | `proxy_http_call("cluster_a", ...)` to a second upstream cluster; `proxy_on_http_call_response` adds response header | Subject: `wasm.<plugin>.http_call_dispatched` + `http_call_response` both increment; response header `x-httpcall-status: 200` present (timing non-deterministic vs reference) |
| (m) | httpCall-unknown-cluster | `proxy_http_call("nonexistent_cluster", ...)` | Subject: `wasm.<plugin>.http_call_dispatch_unknown_cluster` increments; response header `x-httpcall-result: 2` (BadArgument) |
| (n) | body-cap-exceeded | Plugin returns PAUSE indefinitely; subject probes with 32 MiB body | Subject: stream closes with 413; `wasm.<plugin>.body_buffer_cap_exceeded` increments |

**Topology**: ONE listener + HCM with the wasm filter + router terminator. Scenario (l) httpCall uses a SECOND upstream cluster definition (NOT a second listener — avoids the `freeTCPPort` flake risk per phase-22.2 REVIEW §7.4). NEW `BackendKind=HTTPWasmAdvanced` constant or REUSE existing `HTTPWasm` (settled at SPEC).

**Liveness verification** per `reference_differential_asserter_dispatch`: every subject-side `StatsAsserter.AssertStats` arm gets a deliberate-break liveness verification (NOT dead-vacuous per phase-23 fixture-0030 lesson + 25.1 Task 15+17 follow-up).

### 6.2 25.2 fixture-0037 boot-reject

`0037-http-wasm-body-and-advanced-boot-reject`: PGV-mirror reject single-arm for a 25.2-new PARSE-REJECT arm. Anticipated arms (final selection deferred to IMPL via empirical-scrape per 25.1 D-P6 precedent):
- malformed `envoy_go_strict_body_buffer_cap_bytes = 0` envoy-go-strict-only validator (must be > 0; PARSE-REJECT)
- malformed `envoy_go_strict_shared_data_value_cap_bytes = 0` envoy-go-strict-only validator
- malformed `envoy_go_strict_dynamic_stats_max_entries = 0` envoy-go-strict-only validator
- cross-PluginConfig duplicate `PluginConfig.name` (collision PARSE-REJECT for stat-prefix-uniqueness)
- timer-period-when-capability-disabled (operator sets period via wasm bytecode but `proxy_set_tick_period_milliseconds` capability is denied; SPEC settles whether this is config-load-PARSE-REJECT or runtime-deny)

Single-arm boot-reject parity matches phase-22.2 + phase-23 + 25.1 fixture-0035 precedent. Final arm decision deferred to 25.2 IMPL D-25.2-P1 closure (analogous to 25.1 D-P6 which DEVIATED from anticipated arm 5 to substring `"specifier"`).

### 6.3 Total fixture-dir count

37 → 39 at 25.2 (+2: `0036` + `0037`); 39 → 41 at 25.3 (+2 forward-pointer per parent SPEC §8.5). **Total +2 at 25.2.** Plus 1 new conformance harness directory `test/conformance/proxy-wasm/` at 25.3 (NOT counted in the fixture-dir total per `reference_differential_fixture_dispatch_constraint`).

### 6.4 35th project-wide fuzzer `FuzzWasmHostcallEnvelope`

Lands at 25.2 IMPL per parent SPEC §10.2 ADR-0206 anchor. Adversarial corpus seeds (~30-40 seeds per ADR-0018 baseline) covering:
- Hostcall argument-envelope edge cases (wasm linear memory pointer/size bounds; max-size buffer reads)
- proxy-wasm pairs serialization adversarial inputs (malformed key/value sizes; truncated wire bytes)
- Foreign-function call name length boundary cases
- Dynamic-stats name validation (UTF-8 + length + collision)
- Shared-data CAS-mismatch race patterns
- Body-buffer cap boundary cases (exactly-at-cap; one-byte-over-cap)
- Property-path syntax adversarial inputs (malformed CEL-like paths)
- Tick period parsing (negative; > i64 max; 0; below 10ms floor)

Must-never-panic invariant: any of these inputs to the host-side hostcall dispatch MUST NOT crash the envoy-go process (must return WasmResult error code + log + continue). Project fuzzer count: **34 → 35 at 25.2 phase-done**.

### 6.5 Listener topology

Single listener with a single HCM containing the wasm filter (alphabetical position; UNCHANGED at 20 HTTP filters per parent SPEC §3.1 row "HTTP filter wiring") + router terminator. Scenario (l) httpCall uses a SECOND upstream cluster definition (NOT a second listener) per Section 6.1 to avoid `freeTCPPort` flake.

---

## 7. Anticipated ADRs — 5 ADRs at 25.2 (4 NEW + 1 reserve)

### 7.1 25.2 anticipated ADRs (ADR-0205 .. ADR-0209)

| ADR | Subject | Anchor §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0205** | Root VM lifecycle evolution per Q3 — ONE long-lived `*RootVM` per `*compiledConfig`; per-stream contexts are CHILDREN sharing wazero Runtime+Module; tick + httpCall + shared-data state at root; per-stream Module instantiation pattern (fresh vs pooled vs shared) deferred to 25.2 IMPL R8 escape-valve at the > 1ms threshold | §1.1; §2.3; §3.1 | First task that materializes `internal/wasm/root_vm.go` |
| **ADR-0206** | 25.2 ABI extensions — ~20-25 NEW hostcalls (body/buffer × 3 buffer types; trailer-map hostcalls; timer; metrics × 4; shared-data × 2; httpCall; foreign-function; extended-property surface) + ~7-8 NEW ABICallbacks methods + `internal/wasm/foreign.go` ForeignFunctionRegistry with EMPTY default registry per AMEND-A9 + 25.2 capability-roster extension (~20-25 NEW keys) + 25.2 PARSE-REJECT roster extension (~8-12 NEW arms) | §1.1; §2.1, §2.4-§2.6, §2.9; §3.1 | First task that lands body/buffer hostcalls |
| **ADR-0207** | NEW `internal/filterstate/` framework primitive at 25.2 second-consumer scope per Q7 + EXTRACT-NOW-on-second-consumer discipline + consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES + ADR-0188 API-revision allowance NOT consumed (lua primitive untouched) + future-consumer roster | §2.7; §3.2 | First task that materializes `internal/filterstate/` + migration follow-up task in `internal/filter/http/lua/` |
| **ADR-0208** | NEW `internal/filter/http/wasm/` 25.2 package extensions — full hostcall wiring per §3.1 ABI surface row + 8 envoy-go-strict counters per Q9 + 4 envoy-go-strict-only config fields per Qs 2/6/9 + dynamic-stats namespace `wasmcustom.<plugin_name>.<custom_name>` via NEW `internal/stats/dynamic.go` + mixed-mode fixture-0036 discipline per Q8 + 25.2 BEHAVIOR_CONTRACT.md ~7-edit bundle per ADR-0052 | §1.1; §2.2, §2.4-§2.6, §2.8, §2.9; §3.3; §5; §6 | Final Task atomic landing |
| **ADR-0209** | (Escape-valve reserve at 25.2) — likely candidates: per-stream Module instantiation R8 escape-valve if 25.2 IMPL benchmark surfaces > 1ms per-stream cost from body buffer alloc OR property-tree lookups OR foreign-function dispatch overhead; OR a SPEC-time empirical-discovery surface in any 25.2 hostcall implementation (e.g., wazero CompilationCache eviction semantic edge case; pairs wire-format buffer-bounds error class refinement) | §1.2-style WEAK-HOLD-with-1-slot disposition | Only consumed if escape-valve fires; otherwise carries forward to 25.3 BRAINSTORM as the 25.3 IMPL escape-valve slot per R8 signaling protocol |

### 7.2 ADR-0202 one-line in-place AMEND (per Q10 strict-scope precedent)

ADR-0202 gains a one-line acknowledgment paragraph in §Consequences (per ADR-0044 in-place edit discipline). NO new ADR number consumed for the in-place AMEND. The acknowledgment paragraph reads (provisional wording; settles at 25.2 IMPL final Task):

*"Phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern carries forward to 25.2 IMPL R8 escape-valve). The EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent."*

### 7.3 25.3 anticipated ADRs (forward-pointer)

Per parent SPEC §10.3:
- **ADR-0210** (or +1) — Per-route Wasm 5th-canonical REUSE-by-absence EXPLICIT-NO-NEW-CANONICAL classification per AMEND-A3 (analogous to ADR-0173 / ADR-0180).
- **ADR-0211** (or +1) — Multi-plugin VM-sharing semantics — `vm_id`-keyed VM reuse + plugin-context isolation discipline + cross-plugin shared-data scoping.
- **ADR-0212** (or +1) — `test/conformance/proxy-wasm/` conformance harness seed + pin SHA `proxy-wasm-cpp-host@da3ce05d` per AMEND-A8 + 10-of-16 test family port + 62.5% starting pass-threshold.
- **ADR-0213** (or +1; reserve at 25.3) — Escape-valve reserve for any 25.3-IMPL-time-unanticipated surface.

### 7.4 Next-free ADR + D-hypothesis at this BRAINSTORM commit

Anticipated next-free ADR after 25.2 phase-done: **ADR-0210** if reserve UNCONSUMED (4 numbers consumed at 25.2: ADR-0205..ADR-0208); **ADR-0211** if reserve consumed (5 numbers: ADR-0205..ADR-0209). Anticipated next-free after 25.3 phase-done: **ADR-0213** if all 25.3 reserves UNCONSUMED; **ADR-0214 or ADR-0215** if reserves consumed.

**D-hypothesis at this BRAINSTORM commit:** STRENGTHENED-WEAK-HOLD-with-1-slot-buffer (inherits parent SPEC §1.2 disposition for the family). The 4 NEW ADRs (ADR-0205..ADR-0208) land cleanly at 25.2 IMPL; 0-1 escape-valve slot consumption from the per-stream Module instantiation R8 escape-valve (most-likely surface) OR a SPEC-time empirical-discovery surface (less likely, given the parent SPEC AMEND-A1..A9 already closed six SPEC-time-surface-risks). The probability of escape-valve firing is LOW (25.1 Task 17 R8 observed 61µs/stream WELL UNDER 1ms threshold; the 25.2 root-VM model makes per-stream context creation EVEN cheaper; new per-stream cost surfaces — body buffer alloc + property-tree lookups — are anticipated at low microseconds each). WEAK-HOLD stands.

---

## 8. Deferred items

1. **Per-route `Wasm` 5th-canonical wholesale-override via TPFC** — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 18; CONSUMED at 25.3 per ADR-0210 (EXPLICIT-NO-NEW-CANONICAL per AMEND-A3).
2. **Multi-plugin VM-sharing via `vm_id`** — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 12 (duplicate-vm_id); CONSUMED at 25.3 per ADR-0211 (plugin-context isolation + cross-plugin shared-data scoping).
3. **`VmConfig.environment_variables` activation** — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 13; CONSUMED at 25.3 (WASI `environ_*` shims feed from `EnvironmentVariables.host_env_keys` + `key_values`).
4. **`PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig`** — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 9; CONSUMED at 25.3 (with Group-C `vm_reload*` counters per AMEND-A2).
5. **`PluginConfig.fail_open` deprecated bool** — PARSE-REJECT at 25.2 per parent SPEC §6.2 arm 10; CONSUMED at 25.3 (mapped onto `failure_policy = FAIL_OPEN` per AMEND-A1 ladder).
6. **`test/conformance/proxy-wasm/` conformance harness seed** — opens at 25.3 IMPL per AMEND-A8 + ADR-0212 (62.5% pass-threshold target).
7. **Shared-queue hostcalls (4) + `proxy_on_queue_ready` callback** — DEFER to WASM host family per parent SPEC §2.15 + 25.2 BRAINSTORM confirmation. Re-evaluate if/when a Network filter Wasm or WasmService consumer materializes that needs cross-VM coordination.
8. **Outbound gRPC hostcalls (5) + 4 gRPC callbacks** — DEFER to WASM host family per parent SPEC §2.17. The gRPC surface intersects `internal/grpcclient/` at multiple integration points; carries non-trivial scope.
9. **TCP/network-filter hostcalls + callbacks** — NOT REGISTERED at any sub-phase of phase 25; network-filter-wasm row out-of-scope per parent SPEC §2.6.
10. **WasmService singleton plugin loaders** — separate top-level config (`envoy.extensions.wasm.v3.WasmService`); NOT an HTTP filter; lives in the broader §9 WASM host family beyond phase 25.
11. **Cluster-specifier-wasm / access-logger-wasm / network-filter-wasm consumers** — separate filter / extension hosts; consume `internal/wasm/` at consumer #2+ scope under ADR-0202's API-REVISION ALLOWANCE clause. WASM host family phases.
12. **Cross-side byte-exact for 25.2 non-deterministic scenarios** — partial cross-side at 25.2 with subject-only fallback for tick + httpCall + metric-emission per phase-22.2 ADR-0192 precedent + Q8 mixed-mode taxonomy.
13. **wazero JIT/AOT compiler backend** — interpreter default at 25.x; opt-in deferred to future ops-tuning phase per parent SPEC §2.7.
14. **Memory-trap fixture scenarios** — parent SPEC §2.9 OUT OF SCOPE at 25.1 + 25.2 per §4.5 D6 guardrails; may land at 25.3-or-later with mixed-mode discipline.
15. **proxy-wasm v0.3.0 ABI** (WASI threads + extended host ABI) — UPCOMING upstream; envoy-go targets v0.2.1 exclusively at phase 25; v0.3.0 deferred to a future WASM host family phase if/when adoption justifies.
16. **proxy_get_status full HTTP/gRPC code surface refinement** — RATIFIES ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` first co-consumer at 25.1 D-P3 closure; 25.2 EXTENDS via the property-tree `response.code` path (which sources the same accessor). No deferral; just a cross-reference closure.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 25.2 picks up closures from THREE prior phases:

- **Phase-20 oauth2 ADR-0177 `internal/httpclient/` co-consumer validation forward-pointer** — parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor; 25.2 IMPL `proxy_http_call` task lands the third-or-later co-consumer (phase-22.2's `:httpCall()` was second). RATIFIES the phase-20 framework-primitive extraction discipline; CLOSES R6.
- **Phase-21 adaptive_concurrency ADR-0186 `Clock` seam co-consumer validation** — parent SPEC §2.28 anticipation; 25.2 IMPL `proxy_set_tick_period_milliseconds` task lands the FIRST co-consumer beyond phase-21 itself. RATIFIES the phase-21 Clock-seam extraction.
- **Phase-22.2 lua `internal/dynamicmetadata/` cross-filter consumption + filterstate IN-PACKAGE landing** — 25.2 IMPL `proxy_get_property "metadata.*"` task lands ADR-0190 third-or-later co-consumer; 25.2 EXTRACT-NOW `internal/filterstate/` triggers MIGRATION of phase-22.2's in-package `filterstate.go` to consume the new primitive under ADR-0207 (Section 3.2). RATIFIES phase-22.2's cross-filter primitive extraction discipline; CLOSES the implicit EXTRACT-NOW-on-second-consumer forward-pointer that ADR-0188 set for VM-class framework primitives.

Outbound forward-pointers remaining open at 25.2 phase-done (for 25.3 / future):

- **Per-route TPFC 5th-canonical REUSE-by-absence per AMEND-A3** (ADR-0125 STAYS at 10; NO §(xvi) amendment) — closes at 25.3 IMPL ADR-0210.
- **Multi-plugin VM-sharing semantics** — opens at 25.3 BRAINSTORM/SPEC; cross-plugin shared-data scoping settles then.
- **`test/conformance/proxy-wasm/` conformance harness seed at 62.5% threshold per AMEND-A8** — opens at 25.3 IMPL ADR-0212.
- **`VmConfig.environment_variables` activation + `failure_policy = FAIL_RELOAD` + `ReloadConfig` activation + `fail_open` deprecated mapping** — opens at 25.3 SPEC.
- **Per-stream Module instantiation R8 escape-valve** — 25.2 IMPL benchmarks gate; if > 1ms, ADR-0209 fires; if under, escape-valve carries forward to 25.3 IMPL.

The `BOOTSTRAP_PROMPT.md §7.3` anticipation of `test/conformance/proxy-wasm/` is CONSUMED at phase 25.3 (the conformance harness seed lands per AMEND-A8 + Q6 confirmation at parent BRAINSTORM).

---

## 10. BRAINSTORM-time open questions for SPEC-time + PLAN/IMPL-time resolution

Most decisions are CLOSED at this BRAINSTORM commit. The remaining open questions surface unresolved details that the 25.2 SPEC author anchors for SPEC-time resolution (via the parallel-subagent empirical-pin discipline per ADR-0004) or for PLAN-time/IMPL-time resolution per ADR-0044. The numbering uses D-25.2-x to disambiguate from the 25.1 D-Px series.

### 10.1 SPEC-time empirical-pin obligations (D-25.2-1 .. D-25.2-5)

- **D-25.2-1 (proxy-wasm v0.2.1 body+buffer+trailers hostcall signature byte-exact pin)** — pin the exact wire signatures + WasmResult return-code disposition + buffer-bounds error semantics for the ~10 body/buffer/trailer hostcalls (`proxy_get_buffer_bytes`/`proxy_set_buffer_bytes` × 3 buffer types; `proxy_get_buffer_status`; trailer-map family ×4 across the 4 additional WasmHeaderMapType arms) against the proxy-wasm v0.2.1 spec README + proxy-wasm-cpp-host@`da3ce05d` source. **Resolution at:** 25.2 SPEC via parallel-subagent fan-out per ADR-0004; anticipated to RATIFY parent SPEC AMEND-A7's enum + value-gap discipline.

- **D-25.2-2 (proxy-wasm v0.2.1 metrics hostcall signature byte-exact pin + MetricType enum)** — pin `proxy_define_metric(MetricType, name_ptr, name_size, metric_id_ptr_ptr) → WasmResult`, `proxy_increment_metric(metric_id, offset)`, `proxy_record_metric(metric_id, value)`, `proxy_get_metric(metric_id, value_ptr_ptr)` + the MetricType enum (Counter=0, Gauge=1, Histogram=2 per spec README). **Resolution at:** 25.2 SPEC.

- **D-25.2-3 (proxy_http_call wire shape + response routing semantics)** — pin the 10-argument hostcall + response routing via call_id (root-context vs stream-context dispatch); pin `proxy_on_http_call_response(ctx_id, call_id, num_headers, body_size, num_trailers)` invocation discipline when the originating stream context is destroyed (cpp-host drops; envoy-go matches per Q4 + a NEW `wasm.<plugin>.http_call_response_after_close` envoy-go-strict counter — verify whether this counter should be added to the §5.1 roster at SPEC time). **Resolution at:** 25.2 SPEC; counter decision pinned at §5.1 reconciliation.

- **D-25.2-4 (full proxy_get_property roster + path serialization byte-exact pin)** — pin the upstream Envoy CEL property tree against `source/extensions/common/wasm/context.cc` + `proxy-wasm-cpp-host:exports.cc`; pin the path serialization (NUL-delimited per upstream); pin how envoy-go-only properties (e.g., absent upstream cluster info before routing completes) map. **Resolution at:** 25.2 SPEC. **Anticipated:** ~80-100 property paths in the full roster; 25.2 lands all of them.

- **D-25.2-5 (capability roster extensions for 25.2 hostcalls)** — pin the ~20-25 NEW capability keys at the proxy-wasm-cpp-host `capabilityAllowed` gate site; verify the key format matches AMEND-A5 + 25.1 §4.3.1 (bare hostcall name; `proxy_<base>` for env-namespace; bare WASI name for `wasi_snapshot_preview1.*`). **Resolution at:** 25.2 SPEC.

### 10.2 PLAN/IMPL-time decisions (D-25.2-P1 .. D-25.2-P5)

- **D-25.2-P1 (mixed-mode fixture-0037 boot-reject single-arm finalization)** — analogous to 25.1 D-P6; final arm selection deferred to 25.2 IMPL via empirical-scrape against upstream Envoy v1.37.2 boot stderr. Anticipated arms in Section 6.2; final choice settles at IMPL.

- **D-25.2-P2 (per-stream Module instantiation pattern: fresh vs pooled vs shared R8 escape-valve trigger threshold)** — Q3 + Q10 deferred the per-stream Module instantiation discipline to 25.2 IMPL R8 escape-valve. Threshold stays at parent SPEC §13-R8's > 1ms per-stream cost. **Resolution at:** 25.2 IMPL benchmark task; anticipated UNCONSUMED; ADR-0209 reserve slot covers if it fires.

- **D-25.2-P3 (foreign-function dispatch concurrency model: mutex-per-VM vs event-loop-per-VM)** — Q-decisions left this as IMPL-detail. The cpp-host model is mutex-per-Wasm; envoy-go follows. **Resolution at:** 25.2 SPEC OR 25.2 PLAN (architectural-but-implementation-detail).

- **D-25.2-P4 (FuzzWasmHostcallEnvelope corpus seed final roster)** — 30-40 seed envelope per Section 6.4 is BRAINSTORM-anticipated; exact roster pinned at 25.2 SPEC or PLAN.

- **D-25.2-P5 (BEHAVIOR_CONTRACT 25.2 edit bundle exact line counts + departure-record consolidation)** — settled at the 25.2 IMPL final task per ADR-0052 atomic-landing discipline.

Additional SPEC-time D-questions may surface during 25.2 SPEC empirical-pin scrapes (analogous to parent SPEC §11 empirical pins surfacing AMEND-A1..A9); these 10 are the BRAINSTORM-anchored set.

---

## 11. Prior-phase lessons applied

- **Phase-22 lesson — full cross-side byte-exact discipline at first sub-phase, mixed-mode fallback at subsequent sub-phases.** Applied to §6: 25.2 fixture-0036 mixed-mode single-listener with explicit deterministic/subject-only partition per phase-22.2 ADR-0192 + Q8.
- **Phase-22.2 lesson — large advanced-bridge sub-phase stays single sub-phase at BRAINSTORM; PLAN-time split-gate is the arbiter.** Applied to §1.4 + Q11: stay single sub-phase 25.2 at this BRAINSTORM commit; PLAN session estimates against ADR-0045 split-gate; splits at PLAN if needed.
- **Phase-22.2 lesson — strict scope for API-revision allowance: anchor consumer-#1-internal evolution under NEW ADRs rather than in-place AMENDING the original primitive ADR.** Applied to Q10 + §7.2: ADR-0205+0206+0207+0208 (NEW) anchor 25.2 evolutions; ADR-0202 in-place AMEND is ONLY the one-line acknowledgment paragraph; ADR-0202's API-REVISION ALLOWANCE clause remains SCOPED to consumer #2 (WASM host family).
- **Phase-22.2 lesson — EXTRACT-NOW-on-second-consumer for cross-filter primitives even if consumer #1 is in-package.** Applied to Q7 + §3.2: `internal/filterstate/` extracts at 25.2 (consumer #2 = wasm); phase-22.2 lua's in-package `filterstate.go` MIGRATES under ADR-0207. Mirrors the discipline ADR-0188 established for `internal/lua/` at phase-22.1.
- **Phase-22 + phase-19 + phase-20 lesson — REUSE existing framework primitives (httpclient, dynamicmetadata, Clock seam, DownstreamPrincipal) without API extension at third-or-later co-consumer scope.** Applied to §3.4 REUSEs 3-6: zero API extensions needed; RATIFIES prior extraction disciplines + closes parent SPEC §13-R6 anchor.
- **Phase-22.1 lesson — the next-free-ADR-stays-UNCONSUMED hypothesis with a 1-slot buffer is a useful planning anchor.** Applied to Q10 + §7.4 ADR-0209: STRENGTHENED-WEAK-HOLD-with-1-slot disposition; reserve slot covers per-stream Module R8 escape-valve OR a SPEC-time empirical-discovery surface.
- **Phase-23 lesson — cross-side fixtures put subject-side assertions in `StatsAsserter.AssertStats` (NOT `SubjectAsserter`) and prove them live via deliberate-break per `reference_differential_asserter_dispatch`.** Applied to §6: every subject-only assertion arm in fixture-0036 gets a deliberate-break liveness verification.
- **Phase-25.1 lesson — boot-reject fixture single-arm selection settles at IMPL via empirical-scrape (D-P6 substring `"specifier"` chosen by post-scrape evidence; DEVIATED from anticipated arm 5).** Applied to D-25.2-P1: 25.2 fixture-0037 single-arm finalization deferred to IMPL via the same discipline.
- **Phase-25.1 lesson — Task 7 follow-up cross-runtime CompiledModule fix established that wazero's `CompiledModule` is bound to the compiling Runtime; the shared `wazero.CompilationCache` is the cross-Runtime cache.** Applied to Q3 + §3.1: the 25.2 root-VM model makes per-stream contexts share the root's Runtime+Module, retiring the 25.1 cross-Runtime re-compile pattern. Task 7 follow-up's `Module.Source()` retained-bytes mechanism becomes UNNECESSARY for per-stream contexts at 25.2 (per-stream contexts share Module instance via context-IDs, not via re-compile); survives only for the root-VM-construction code path.
- **Phase-22 lesson — envoy-go-strict default-deny sandbox posture for VM-class filters extends with new capability keys at each sub-phase landing.** Applied to §3.4 REUSE 8 + capability-roster extension at 25.2 (~20-25 NEW capability keys for body/buffer/trailers/timer/metrics/shared-data/httpCall/foreign-function/extended-property).
- **Phase-20 lesson — RTDS-deferral pattern PARSE-REJECTs runtime-coupled fields without breaking config-load.** Applied to §1.2 non-purposes (parent SPEC §2.x roster unchanged at 25.2; the per-route TPFC + multi-plugin + `environment_variables` remain PARSE-REJECTed at 25.2 lifting to 25.3).
- **Phase-22.2 lesson — listener topology: avoid multi-listener `freeTCPPort` flake via single-listener + second-cluster pattern for cross-cluster scenarios.** Applied to §6.5 + Q8: single-listener fixture-0036; httpCall scenarios use second upstream cluster definition (NOT a second listener).
- **Phase-04 lesson — envoy-go-strict safety-first defaults pattern (TLS strict-by-default; phase-10 header_mutation pre-mutation-allowlist; phase-22 lua default-deny SandboxConfig).** Applied to Qs 2/5/6/9: 25.2 lands envoy-go-strict cap-discipline departures (body buffer + shared-data + tick period floor + dynamic-stats namespace cap) as defense-in-depth beyond upstream's reliance on operator-configured HCM-level + listener-level memory ceilings.

---

## 12. Section closeout

This BRAINSTORM.md is **lifecycle-state 0 → 1 complete** for sub-phase 25.2. The next session (lifecycle-state 1 → 2, skill `superpowers:writing-plans` scoped to **25.2 SPEC authoring** per the phase-22.1/22.2/22.3/25.1 precedent) authors `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/SPEC.md` based on:

1. The 11 user-decided Q-decisions (Q1 body per-chunk + accumulating buffer; Q2 16 MiB body cap envoy-go-strict default + configurable; Q3 upstream-byte-faithful root VM lifecycle; Q4 BadArgument + envoy-go-strict counter for unknown cluster; Q5 per-root-VM tick goroutine + 10ms envoy-go-strict period floor + Clock seam; Q6 shared-data caps + 2 envoy-go-strict-only config fields; Q7 full property surface + EXTRACT-NOW `internal/filterstate/` consumer-#2 primitive; Q8 single-listener mixed-mode fixture-0036 + 12-14 scenarios; Q9 8-counter envoy-go-strict tally + dynamic-stats namespace + 1024-entry cap; Q10 strict-scope + 4 NEW + 1 reserve ADR; Q11 stay single sub-phase 25.2).
2. The framework-survey result (§3): 1 NEW package-level framework primitive (`internal/filterstate/` consumer #2 EXTRACT-NOW per Q7); 1 NEW infrastructure package (`internal/stats/dynamic.go` per Q9); 3 `internal/wasm/` API surface evolutions (root VM lifecycle per Q3 + ABI extensions per Qs 1/2/4/5/6/9 + foreign-function registration interface per AMEND-A9); 7 REUSES + 1 MIGRATES (phase-22.2 lua filterstate.go).
3. The 25.2-extends-25.1 stat-surface delta (§5): +8 envoy-go-strict counters (project 119 → 127) + open-ended `wasmcustom.<plugin_name>.<custom_name>` dynamic namespace via `internal/stats/dynamic.go` + 4 envoy-go-strict-only `PluginConfig` config fields + ~6 NEW envoy-go-strict departure records (departure record count 21 → 27).
4. The 2-directory differential fixture envelope (§6): `0036-http-wasm-body-and-advanced` mixed-mode single-listener with 12-14 scenarios (8-10 deterministic cross-side + 3-4 non-deterministic subject-only) + `0037-http-wasm-body-and-advanced-boot-reject` single-arm PGV-mirror reject; fixtures 37 → 39 at 25.2 phase-done.
5. The 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` (§6.4): ~30-40 corpus seeds covering hostcall envelope adversarial inputs; must-never-panic invariant.
6. The anticipated ADRs (§7): 4 NEW (ADR-0205 root VM + ADR-0206 25.2 ABI extensions + foreign-function registration per AMEND-A9 + ADR-0207 `internal/filterstate/` extraction + ADR-0208 filter package extensions + envoy-go-strict counter bundle + mixed-mode fixture + dynamic-stats namespace) + ADR-0209 reserve; ADR-0202 one-line in-place AMEND acknowledgment paragraph.
7. The ~7-edit BEHAVIOR_CONTRACT.md bundle at 25.2 IMPL final Task per ADR-0052 atomic landing (§5.4).
8. The 5 SPEC-time D-questions (D-25.2-1 .. D-25.2-5) + 5 PLAN/IMPL-time D-Px-style sub-questions (D-25.2-P1 .. D-25.2-P5) (§10).
9. The 13 prior-phase lessons applied (§11).
10. The cross-phase closure pickup (§9): phase-20 ADR-0177 third-or-later co-consumer (CLOSES parent SPEC §13-R6) + phase-21 ADR-0186 Clock seam first co-consumer (RATIFIES) + phase-22.2 ADR-0190 third-or-later co-consumer + phase-22.2 in-package filterstate MIGRATES.

The 25.2 SPEC author is responsible for executing the §10.1 empirical-pin obligations IN-SESSION against `envoyproxy/envoy@v1.37.2` C++ source + `go-control-plane@v1.32.4` proto bindings + `proxy-wasm/proxy-wasm-cpp-host@da3ce05d` + the `proxy-wasm/spec@main` ABI v0.2.1 README + wazero `v1.10.1` per ADR-0004 — covering D-25.2-1 (body+buffer+trailers signatures) through D-25.2-5 (capability roster extensions). The 5 D-Px sub-questions (D-25.2-P1 fixture-0037 single-arm finalization + D-25.2-P2 per-stream Module instantiation R8 escape-valve + D-25.2-P3 foreign-function dispatch concurrency model + D-25.2-P4 FuzzWasmHostcallEnvelope corpus seed roster + D-25.2-P5 BEHAVIOR_CONTRACT bundle line-count finalization) settle at PLAN or IMPL time per their natural anchor points (PLAN for SPEC-time-architectural-but-implementation-detail; IMPL benchmark for R8; IMPL final task for BEHAVIOR_CONTRACT bundle).

**Anticipated 25.2 scope at SPEC commit:** ~22-28 tasks + ~5,000-7,500 LIVE production LoC; stay single sub-phase at this BRAINSTORM commit (per Q11); PLAN session estimates against the ADR-0045 split-gate. The LoC half of the gate fires comfortably; the task-arm half is borderline at the upper bound. If PLAN exceeds the task-arm gate, splits to 25.2.1 + 25.2.2.

**Anticipated next-free ADR after 25.2 phase-done:** ADR-0210 if reserve UNCONSUMED; ADR-0211 if consumed.

**§9 family-row position at end of 25.2 phase-done:** 19 landed at master (07.1 / 09 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23 / 24 / 25.1 / 25.2) + 1 in-flight (parent row 25 stays `in-progress`; sub-row 25.3 flips to next). §9 family closes completely at 25.3 phase-done.

**Hand-off:** BRAINSTORM-time scope is complete. 25.2 SPEC author proceeds via `superpowers:writing-plans` scoped to 25.2 SPEC authoring at the next session (lifecycle-state 1 → 2 per ADR-0005 §Decision 4; the parent SPEC author already executed the §11 empirical-pin block for the cross-cutting envelope; the 25.2 SPEC focuses on the sub-phase-specific surface detail + the D-25.2-1..D-25.2-5 empirical pins per ADR-0004).
