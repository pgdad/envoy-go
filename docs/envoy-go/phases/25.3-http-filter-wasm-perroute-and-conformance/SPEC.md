# Phase 25.3 SPEC — `envoy.extensions.filters.http.wasm.v3.Wasm` (per-route TPFC + multi-plugin VM-sharing + `failure_policy = FAIL_RELOAD` + `VmConfig.environment_variables` + `test/conformance/proxy-wasm/` harness seed)

> **Lifecycle state:** SPEC.md authored; ROADMAP sub-row `25.3` flips `planned → in-progress` at this SPEC commit (parent row `25` STAYS `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `25.1` `done`, `25.2` `done`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-22.2 + 22.3 + 25.1 + 25.2 sub-phase SPEC → PLAN precedent. This SPEC is the authoritative input to the 25.3 PLAN; it CLOSES parent row 25 at the downstream 25.3 IMPL phase-done.

**Parent:** `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` (parent master SPEC — §1.1 9-AMEND catalog A1-A9; §3.1 sub-phase surface-mapping table; §5 proto-field roster; §6.2 18-arm 25.1 PARSE-REJECT roster + §6.3 25.2 + 25.3 forward-pointers; §7 tri-group stat surface per AMEND-A2; §10.3 25.3 anticipated ADRs ADR-0210 + ADR-0211 + ADR-0212; §11 9-pin empirical-pin block; §13 R1-R8 RATIFIED-PENDING items). The 25.3 SPEC INHERITS the 9-AMEND catalog + the parent §3.1 surface-mapping (25.3 column) verbatim; it does NOT re-litigate parent-settled scope. **AMEND-A3 ABSENCE-DEFINITIVE** (no `WasmPerRoute` message; ADR-0125 STAYS at 10 canonicals) + **AMEND-A8** (conformance source pinned to `proxy-wasm-cpp-host@da3ce05d`) are load-bearing at 25.3.

**Predecessors:**
- `docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/BRAINSTORM.md` (355 lines; 12 sections; 7 Q-decisions settled — Q1 multi-plugin process-global `vm_id`-keyed registry; Q2 `FAIL_RELOAD` upstream `BackoffStrategy` + envoy-go-strict floor; Q3 `environment_variables` upstream-parity + envoy-go-strict cap; Q4 fixture-0036 cross-side parity gap defer-to-follow-up; Q5 debt #1 (phase-21 clock migration) fold-in + debt #2 (lua filterstate storage) defer; Q6 ADR-0209 R8 carry-forward unchanged; Q7 conformance harness `go test`-driven + CI per-commit + 100%-of-10 phase-done gate). 7 SPEC-time D-questions surfaced (D-25.3-1..D-25.3-7).
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/{SPEC,PLAN,PROGRESS,REVIEW}.md` (the closest precedent sub-phase artifact set — load-bearing for SPEC structure + 25.2 IMPL inheritance state).
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/{SPEC,PLAN,PROGRESS,REVIEW}.md` (the foundation sub-phase).

**Sub-phase scope (per parent SPEC §3.1 25.3 column + BRAINSTORM §1.1):** Phase 25.3 lifts the LAST PARSE-REJECT residuals in the wasm filter — per-route `typed_per_filter_config` (TPFC) wholesale-override + multi-plugin VM-sharing via a `vm_id`-keyed process-global registry + `VmConfig.environment_variables` activation + `failure_policy = FAIL_RELOAD` + `reload_config` + deprecated `fail_open` mapping — taking parent BRAINSTORM Q1 envelope D (full upstream parity) to its FULL conclusion across the 3-way pre-split. **Plus** seeds the `test/conformance/proxy-wasm/` Go-test conformance harness per `BOOTSTRAP_PROMPT.md §7.3` + AMEND-A8. By 25.3 phase-done, every operator-visible surface in the v1.37.2 + go-control-plane v1.32.4 `envoy.extensions.filters.http.wasm.v3.Wasm` + `envoy.extensions.wasm.v3.{PluginConfig,VmConfig,ReloadConfig,FailurePolicy,EnvironmentVariables,CapabilityRestrictionConfig}` proto rosters is CONSUMED (except the deferred-to-WASM-host-family surfaces enumerated at §2 + parent SPEC §2.x).

Specifically:
- **Per-route TPFC 5th-canonical REUSE-by-absence** CONSUMED — wholesale-override via `typed_per_filter_config` of the same `Wasm` message; NO dedicated `WasmPerRoute` proto exists (AMEND-A3 ABSENCE-DEFINITIVE; re-CONFIRMED at §11.1 D-25.3-1). ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment. ADR-0210 anchors the EXPLICIT-NO-NEW-CANONICAL classification analogous to ADR-0173 / ADR-0180 / ADR-0197.
- **Multi-plugin VM-sharing** CONSUMED — process-global `*Registry` keyed by `Sha256(vm_id ‖ vm_configuration ‖ code)` with refcount + `sync.Mutex` mirroring cpp-host's `base_wasms` shared-handle pattern (`proxy-wasm-cpp-host@da3ce05d:src/wasm.cc:90-92` + `546-589`). Lifts 25.2's per-`*compiledConfig` `*RootVM` to a per-`(vm_id, vm_configuration, code)` shared `*RootVM`. NEW intra-package primitive at `internal/wasm/registry.go`. Cross-plugin shared-data SHARED at raw-`vm_id` scope per AMEND-C2 (REFINES BRAINSTORM Q1 keying).
- **`failure_policy = FAIL_RELOAD`** CONSUMED — `FailurePolicy` enum `{UNSPECIFIED=0, FAIL_RELOAD=1, FAIL_CLOSED=2, FAIL_OPEN=3}` (default `FAIL_CLOSED`); `FAIL_RELOAD` gated to guest `RuntimeError` only (all other fail-states fall back to `FAIL_CLOSED` per AMEND-C3); reload backoff via `ReloadConfig.backoff` (`config.core.v3.BackoffStrategy.base_interval`; `max_interval` is DEAD in the upstream wasm path) + envoy-go-strict 100ms floor; state machine `{Running, Reloading, Failed}` per-`*RootVM`; `internal/clock/` Clock seam THIRD co-consumer of ADR-0186; Group-C `vm_reload_success` + `vm_reload_runtime_failure` + `vm_reload_backoff` triplet activates. `FAIL_OPEN` maps to bypass-on-failure; deprecated `fail_open: bool` maps onto `FAIL_OPEN` per AMEND-A1 ladder (and is mutually-exclusive with `failure_policy` per AMEND-C3 — both-set is PARSE-REJECT).
- **`VmConfig.environment_variables`** CONSUMED — `host_env_keys: []string` (host-process-env allowlist passthrough) + `key_values: map<string,string>` (explicit pairs); **collisions across the two fields PARSE-REJECT at config-load** per upstream (AMEND-C4 REFUTES BRAINSTORM "key_values overrides"); `key_values` forbidden on the null runtime (subsumed by envoy-go's wazero-only runtime PARSE-REJECT); host-env-keys absent from the host env silently skipped; feeds WASI `environ_get` + `environ_sizes_get` shims as `KEY=VALUE\0` entries. envoy-go-strict cap (64 total entries + per-value 4 KiB; hardcoded NOT operator-tunable; no upstream cap — legit departure) → PARSE-REJECT + `env_vars_cap_exceeded` envoy-go-strict counter. **Resolves fixture-0036 scenario-(j) `std::env::vars()` RefCell panic** (debt #4).
- **`test/conformance/proxy-wasm/` conformance harness seed** per Q7 + AMEND-A8 — `go test`-driven harness; the 16 `proxy-wasm-cpp-host@da3ce05d:test/*_test.cc` UNIT-test families re-expressed as Go-test sub-tests under `families/<family_name>/`; 10 ported / 6 deferred per the host-capability alignment at §11.3 D-25.3-3 (AMEND-C5 REFINES the BRAINSTORM "hostcall-family" framing — the cpp-host suite is engine/runtime unit-tests, NOT an ABI-conformance corpus); 62.5% (10/16) numeric threshold HOLDS; phase-done gate = ALL 10 ported families PASS. INDEPENDENT of the differential harness (mirrors phase-05 h2spec at `test/conformance/h2spec/`). ADR-0212 anchors.
- **Architectural debt #1 fold-in** (Q5) — phase-21 `internal/filter/http/adaptive_concurrency/clock.go` migrated to consume the 25.2-extracted `internal/clock/` package (~50-100 LoC mechanical); RATIFIES ADR-0186 at consumer-real-migration scope.
- **0 NEW hostcalls + 0 NEW guest callbacks + 0 NEW capability keys** (D-25.3-4 CONFIRMS at §11.6) — all 25.3 work is host-side (registry + reload state machine + env assembly feeding existing WASI shims); the proxy-wasm v0.2.1 ABI boundary is unchanged. `ABICallbacks` STAYS at 25 methods; capability roster STAYS at 58 keys.
- **Differential fixture `0038-http-wasm-perroute-and-multi-plugin`** (cross-side) + **`0039-http-wasm-perroute-boot-reject`** (subject-only boot-reject) per `reference_differential_fixture_dispatch_constraint`. **39 → 41 fixture dirs.**
- **35 fuzzers UNCHANGED** — per-route + reload + env_vars parse coverage FOLDS into the existing `FuzzWasmConfigParse` seed corpus per D-25.3-6 (no 36th fuzzer; per-route is wholesale-override with no novel parse surface, and reload/env_vars parse extend the same `PluginConfig`/`VmConfig` parse path the existing fuzzer already exercises).

**Out of scope for 25.3** (forward to follow-up phases / WASM host family): HCM upstream-buffering parity fix (fixture-0036 arms a-j stay skip-token per Q4 → `http-body-buffering-parity-fix` ROADMAP candidate); phase-22.2 lua filterstate storage migration (debt #2, defer per Q5); cluster-specifier-wasm / access-logger-wasm / network-filter-wasm / WasmService (consumer #2+ of `internal/wasm/`; ADR-0202 API-REVISION ALLOWANCE clause activates there); shared-queue + outbound-gRPC hostcalls; wazero JIT/AOT; `AsyncDataSource.Remote`; ABI v0.1.0+v0.2.0 (PARSE-REJECT continues per AMEND-A6); the 6 deferred conformance families.

**ADR continuity:** Phase 25.2 IMPL closed at ADR-0208 §Decision + §Consequences body landed; DECISIONS.md tail at **ADR-0208**; ADR-0209 reserved UNCONSUMED (carries forward as the 25.3 IMPL R8 escape-valve slot). **At THIS 25.3 SPEC commit: 3 NEW ADR §Context drafts anchor** (ADR-0210 + ADR-0211 + ADR-0212) per ADR-0044 §Context-draft discipline. §Decision + §Consequences bodies LAND at 25.3 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. **In-place AMEND acknowledgment paragraphs** land at 25.3 IMPL on ADR-0205 §Consequences (registry-keyed-by-`(vm_id, vm_configuration, code)` refinement; no new ADR number) and ADR-0202 §Consequences STAYS UNCHANGED (API-REVISION ALLOWANCE clause remains scoped to consumer #2). **Next-free ADR after THIS 25.3 SPEC commit: `ADR-0213`** (3 numbers consumed: ADR-0210 + ADR-0211 + ADR-0212; ADR-0209 + ADR-0213 reserved as the STRENGTHENED-WEAK-HOLD-with-1-slot-buffer escape-valve pair per Q6 + §10.5). Anticipated next-free after 25.3 phase-done: **`ADR-0213`** if all reserves UNCONSUMED.

**Authored:** 2026-05-28.

**Base commit:** master tip at session entry — `58a484d` (`next-prompt.txt: repoint master-tip references to 032fb7a (actual HEAD)`; docs-only repoint). Predecessors: `032fb7a` (next-prompt.txt rewrite for 25.3 SPEC cold-start) + `1cda294` (25.3 BRAINSTORM stage-close SHA-fill) + `8d64f4d` (25.3 BRAINSTORM squash-merge) + `368bc9f`/`5629408` (25.2 IMPL squash + SHA-fill).

---

## 1. Purpose / Mission

Phase 25.3 closes the `envoy.filters.http.wasm` family by activating the last four operator-visible config surfaces (per-route TPFC, multi-plugin VM-sharing, `failure_policy = FAIL_RELOAD`, `VmConfig.environment_variables`) on top of the 25.1 headers-foundation + 25.2 full-advanced-bridge, and by seeding the `test/conformance/proxy-wasm/` Go-test harness. By 25.3 phase-done the project has the COMPLETE upstream-parity `envoy.filters.http.wasm` surface modulo the documented envoy-go-strict departures; the §9 HTTP-filters family CLOSES (parent row 25 flips `in-progress → done` at the 25.3 IMPL final Task per the 18/19/22/24 ROLLUP precedent).

The five architectural surfaces landing at 25.3:

1. **Per-route TPFC 5th-canonical REUSE-by-absence (ADR-0210).** The v1.37.2 IDL + go-control-plane binding define exactly ONE message in the HTTP filter namespace — `Wasm { PluginConfig config = 1; }` — and the wasm filter factory does NOT override `createRouteSpecificFilterConfig` (re-CONFIRMED at §11.1 D-25.3-1). Per-route override is therefore a wholesale `Wasm`-message replacement resolved through the existing 3-tier per-route resolver from phase 13/14/15 (REUSE 1). ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment; ADR-0210 anchors the EXPLICIT-NO-NEW-CANONICAL classification. **There is NO "dangling vm_id" concept** (AMEND-C1): the `vm` oneof carries an inline `VmConfig` per plugin, and referential VM configs are an explicit unimplemented upstream TODO — `vm_id` is a pure sharing key with no cross-reference to dangle.

2. **Multi-plugin VM-sharing process-global registry (ADR-0211; REFINES ADR-0205).** NEW `internal/wasm/registry.go` materializes a process-global `*Registry` keyed by `Sha256(vm_id ‖ vm_configuration ‖ code)` (AMEND-C2; runtime NOT in key because envoy-go is wazero-single-runtime; `vm_configuration` IS in the key + the full `code` bytes are hashed, matching cpp-host `makeVmKey`). On `compiledConfig.New` the registry looks up an existing `*RootVM` by the composite key; on hit it reuses + bumps refcount; on miss it constructs fresh + inserts. Each `PluginConfig` sharing a key gets a distinct `*PluginContext` child (own `root_id` + configuration bytes + capabilities; cpp-host plugin-key = `Sha256(root_id ‖ plugin_configuration ‖ key)`). Cross-plugin shared-data is SHARED at **raw-`vm_id` scope** (broader than the VM-instance key — AMEND-C2), byte-faithful to cpp-host `SharedData` keying. envoy-go COLLAPSES cpp-host's two-layer (process-global `base_wasms` + thread-local `local_wasms` clone) model into ONE process-global registry because Go's goroutine concurrency model has no Envoy thread-local-worker equivalent (AMEND-C2 divergence note). REFINES ADR-0205's anchor `*RootVM per *compiledConfig` → `*RootVM per (vm_id, vm_configuration, code)` shared across `*compiledConfig` instances.

3. **`failure_policy = FAIL_RELOAD` reload state machine (ADR-0211; bundled per D-25.3-2).** `FailurePolicy` enum `{UNSPECIFIED=0, FAIL_RELOAD=1, FAIL_CLOSED=2, FAIL_OPEN=3}`; default (UNSPECIFIED) resolves to `FAIL_CLOSED` (503), NOT bypass (AMEND-C3 REFINES BRAINSTORM). `FAIL_RELOAD` is gated to guest `RuntimeError` only; all other fail-states (StartFailed, ConfigureFailed, UnableToInitializeCode, …) fall back to `FAIL_CLOSED`. State machine `{Running, Reloading, Failed}` per-`*RootVM`; reload is request-driven + backoff-rate-limited (NOT a background respawn) per upstream. Backoff honors `ReloadConfig.backoff` (`config.core.v3.BackoffStrategy.base_interval`; default 1s when `reload_config` unspecified; `max_interval` is in the proto but DEAD in the upstream wasm path — envoy-go mirrors upstream's `base_interval`-only `JitteredLowerBoundBackOffStrategy` and RETIRES the BRAINSTORM-anticipated `max_interval` 1s floor as MOOT) + envoy-go-strict `base_interval = max(operator_value, 100ms)` floor. Group-C `vm_reload_success` + `vm_reload_runtime_failure` + `vm_reload_backoff` triplet activates (mirrors upstream `VmReloadSuccess`/`VmReloadFailure`/`VmReloadBackoff`). `fail_open: bool` ⊕ `failure_policy` is mutually-exclusive → both-set is PARSE-REJECT (AMEND-C3). The reload state machine consumes `internal/clock.Clock` (THIRD co-consumer of ADR-0186 after phase-21 + 25.2 tick goroutine).

4. **`VmConfig.environment_variables` activation (ADR-0211; bundled).** `EnvironmentVariables { repeated string host_env_keys = 1; map<string,string> key_values = 2; }` on `VmConfig` (field 7). **Collisions across `host_env_keys` and `key_values` PARSE-REJECT at config-load** byte-faithful to upstream (`source/extensions/common/wasm/plugin.cc:30-42` throws `EnvoyException`) — AMEND-C4 REFUTES the BRAINSTORM Q3 "key_values overrides on collision" hypothesis. `key_values` forbidden on the null runtime (subsumed by envoy-go's existing wazero-only-runtime PARSE-REJECT). Host-env-keys absent from the host process env are silently skipped. The assembled map feeds the WASI `environ_get` + `environ_sizes_get` shims (which returned zeros at 25.2 per AMEND-A6 deferral) as `KEY=VALUE\0` entries. envoy-go-strict cap (64 total entries + per-value 4 KiB; hardcoded; no upstream cap — legit departure) → PARSE-REJECT + `env_vars_cap_exceeded` envoy-go-strict counter. Resolves the fixture-0036 scenario-(j) RefCell panic (debt #4) by populating env from config.

5. **`test/conformance/proxy-wasm/` harness seed (ADR-0212).** `go test`-driven harness mirroring phase-05 h2spec at `test/conformance/h2spec/`. The 16 `proxy-wasm-cpp-host@da3ce05d:test/*_test.cc` files are the family roster (AMEND-C5 — the upstream suite is engine/runtime UNIT tests, NOT a hostcall-by-hostcall ABI-conformance corpus; the BRAINSTORM/parent-AMEND-A8 "10-of-16 hostcall families" framing is REFINED to "10-of-16 cpp-host unit-test families re-expressed as Go-test sub-tests"). 10 PORT / 6 DEFER per the §11.3 capability alignment; 62.5% (10/16) numeric threshold HOLDS; phase-done gate = ALL 10 ported families PASS. Vendored bytecode/`test_data` fixtures ported to Go-test scaffolding under `families/<family_name>/`. Each family's subject-side assertion proven live via a deliberate-break cycle per `reference_differential_asserter_dispatch`. INDEPENDENT of the differential harness (subject-only; no reference-Envoy cross-side comparison).

Plus the **phase-21 clock migration** (debt #1 fold-in per Q5) and **0 ABI / 0 capability-key growth** (D-25.3-4).

After phase 25.3, the `envoy.filters.http.wasm` surface is COMPLETE: per-route wholesale-override; multi-plugin `vm_id`-shared VMs with refcounted process-global lifecycle + shared-data at vm_id scope; `FAIL_RELOAD` request-driven reload with backoff + Group-C observability + `FAIL_CLOSED`/`FAIL_OPEN`; `environment_variables` feeding WASI environ with collision-reject + envoy-go-strict cap; OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the deterministic fixture-0038 scenarios + REFERENCE-LESS-equivalent on the non-deterministic scenarios — modulo the ~29 envoy-go-strict documented divergence-windows (27 inherited + 2 NEW: reload-floor + env_vars-cap). The §9 HTTP-filters family CLOSES.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044 — 5 AMEND-C entries from §11)

The 7 §11 D-25.3-1..D-25.3-7 empirical pins (executed at this SPEC session via parallel-subagent fan-out against `envoyproxy/envoy@v1.37.2` IDL + C++ source + go-control-plane v1.32.4 binding + `proxy-wasm/proxy-wasm-cpp-host@da3ce05d` + `proxy-wasm/spec@main` ABI v0.2.1 README + `proxy-wasm/proxy-wasm-rust-sdk@v0.2.4` per ADR-0004) generated the following **5 AMEND-C entries** load-bearing for 25.3:

- **AMEND-C1 (per-route is wholesale TPFC; "dangling vm_id" MOOT — CONFIRMS AMEND-A3 + RETIRES a fixture-0039 arm candidate):** Per §11.1 D-25.3-1. The HTTP filter proto defines exactly `message Wasm { envoy.extensions.wasm.v3.PluginConfig config = 1; }` (`api/envoy/extensions/filters/http/wasm/v3/wasm.proto:19-22`); the factory `WasmFilterConfig` (`source/extensions/filters/http/wasm/config.h:20-66`) overrides only `createFilterFactoryFromProto*` and does NOT register a route-specific config — so per-route override is a generic wholesale `Wasm` TPFC override resolved by HCM machinery. `vm_id` (string, `VmConfig` field 1) is documented as a pure VM-sharing key (`api/envoy/extensions/wasm/v3/wasm.proto:76-81`); the `vm` oneof (`:165-168`) carries an inline `VmConfig` with a `// TODO: add referential VM configurations.` comment — referential (named) VM configs are UNIMPLEMENTED upstream, so **there is no cross-reference for a "dangling vm_id" to dangle against.** The BRAINSTORM §6.1 / §10 D-25.3-1 candidate "dangling vm_id reference" boot-reject arm for fixture-0039 is RETIRED; the fixture-0039 arm shifts to env_vars cap-exceeded OR env_vars key-collision (final at IMPL per D-25.3-P1).

- **AMEND-C2 (multi-plugin registry key + shared-data scope + two-layer collapse — REFINES BRAINSTORM Q1):** Per §11.2 D-25.3-Q1. cpp-host `makeVmKey(vm_id, vm_configuration, code) = Sha256({vm_id, "||", vm_configuration, "||", code})` (`src/wasm.cc:90-92`) — the key folds in `vm_configuration` AND the full `code` bytes (not a precomputed "code hash"), and **runtime is NOT in the key** (the engine arrives via the factory, not the key). The process-global `base_wasms` map holds `weak_ptr<WasmHandleBase>` (`src/wasm.cc:39-40`); reuse is `.lock()` on a matching key; ownership/refcount lives in the returned `shared_ptr` plugin handles (`src/wasm.cc:546-589`). Plugin identity = `Sha256(root_id ‖ plugin_configuration ‖ key)` (`src/context.cc:92-94`) — distinct per `PluginConfig`. **Shared-data scope is keyed by the raw user `vm_id`** (`src/shared_data.cc:44-79`), NOT the composite vm_key — broader than VM-instance scope (plugins with the same `vm_id` but differing config/code still share the namespace). **Disposition for envoy-go:** the 25.3 registry key is `Sha256(vm_id ‖ vm_configuration ‖ code)` (drop runtime — wazero-only); shared-data scoped by raw `vm_id`; envoy-go COLLAPSES cpp-host's two-layer (process-global `base_wasms` + thread-local `local_wasms` clones) into ONE process-global registry because Go has no Envoy thread-local-worker model (the wazero Runtime+Module is shared across goroutines with the existing per-RootVM serialization discipline from ADR-0205). REFINES BRAINSTORM Q1 `(vm_id, code_hash, runtime)` → `(vm_id, vm_configuration, code)`.

- **AMEND-C3 (`FailurePolicy` enum + RuntimeError-gating + base_interval-only backoff + mutual-exclusivity — REFINES BRAINSTORM Q2):** Per §11.3 D-25.3-Q2. `FailurePolicy { UNSPECIFIED=0, FAIL_RELOAD=1, FAIL_CLOSED=2, FAIL_OPEN=3 }` (`api/envoy/extensions/wasm/v3/wasm.proto:24-42`); **default is FAIL_CLOSED** (not bypass). `FAIL_RELOAD` applies ONLY to `proxy_wasm::FailState::RuntimeError` (=7); all other fail-states fall back to `FAIL_CLOSED` (`wasm.proto:33` + `source/extensions/common/wasm/wasm.cc:482-536` `maybeReloadHandleIfNeeded`). `fail_open` (PluginConfig field 5, deprecated-at-3.0) and `failure_policy` (field 7) are **mutually exclusive** — setting both throws `"only one of fail_open or failure_policy can be set"` (`wasm.cc:573-574`); `fail_open=true` maps to `FAIL_OPEN`, `false`/unset resolves UNSPECIFIED → `FAIL_CLOSED` (`wasm.cc:578-584`). `reload_config` is a distinct `ReloadConfig { config.core.v3.BackoffStrategy backoff = 1; }` (`wasm.proto:44-48`, PluginConfig field 8); upstream consumes ONLY `backoff.base_interval` (default 1000ms when unspecified; PGV `gte 1ms`) feeding a `JitteredLowerBoundBackOffStrategy` (`wasm.cc:603-606`); `max_interval` exists in the proto but is DEAD in the wasm reload path. **Disposition for envoy-go:** honor `base_interval` with envoy-go-strict floor `max(operator, 100ms)`; mirror upstream's `base_interval`-only backoff; RETIRE the BRAINSTORM Q2 `max_interval` 1s floor as MOOT; implement `FAIL_CLOSED` (503) as the UNSPECIFIED default; PARSE-REJECT the `fail_open`⊕`failure_policy` both-set case (NEW arm). Group-C triplet `vm_reload_success`/`vm_reload_runtime_failure`/`vm_reload_backoff` mirrors upstream.

- **AMEND-C4 (env_vars collision-REJECT, not override; null-runtime reject; no upstream cap — REFUTES BRAINSTORM Q3):** Per §11.4 D-25.3-Q3. `EnvironmentVariables { repeated string host_env_keys=1; map<string,string> key_values=2; }` on `VmConfig` field 7 (`wasm.proto:131-149`). **Collisions across the two fields are REJECTED at config-load** (`source/extensions/common/wasm/plugin.cc:30-42` builds a key-set and throws `EnvoyException` on any duplicate; "All the keys must be unique") — there is NO override semantic. `key_values` is additionally rejected for the null runtime (`plugin.cc:25-28`; subsumed by envoy-go's wazero-only PARSE-REJECT). Host-env-keys absent from the host env are silently skipped (`plugin.cc:49-51`). The assembled `unordered_map` feeds WASI `environ_get`/`environ_sizes_get` as `KEY=VALUE\0` (`src/exports.cc:802-846`). There is NO upstream cap on count or size. **Disposition for envoy-go:** implement collision-reject (NEW PARSE-REJECT arm; upstream parity) + the envoy-go-strict 64-entry/4-KiB cap (departure; PARSE-REJECT + `env_vars_cap_exceeded` counter). REFUTES BRAINSTORM Q3 override hypothesis.

- **AMEND-C5 (conformance roster = 16 cpp-host unit-test files; 10-port/6-defer — REFINES BRAINSTORM Q7 + parent AMEND-A8 framing):** Per §11.5 D-25.3-3. `proxy-wasm-cpp-host@da3ce05d:test/` holds 16 `*_test.cc` files; the suite is engine/runtime UNIT tests, NOT a hostcall-by-hostcall ABI-conformance corpus (there is NO dedicated `http_call`/`metric`/`property`/`foreign_function`/`grpc`/`tick` test file at the pin — those ride along inside `logging`/`runtime` drivers + `test_data` fixtures). The "16 families" map to the 16 test files. 10 PORT (logging, stop_iteration/header-maps, shared_data, endianness, exports[env/clock/random], security, runtime/traps, wasm_vm, bytecode_util, pairs_util) / 6 DEFER (shared_queue → WasmService cross-VM queues; signature_util → signed/remote code fetch; wasm[TLS-cache] → WasmService singleton model; vm_id_handle → cross-VM scoping substrate; null_vm → N/A for a Go host with no NullVM engine; fuzz → libFuzzer harnesses, not gtest). 62.5% (10/16) numeric threshold HOLDS. REFINES the BRAINSTORM Q7 / parent AMEND-A8 "hostcall-family" framing to "cpp-host unit-test-family port" + flags that `grpc`/`http_call`/`metric` have no dedicated test family to port at this pin.

This 25.3 SPEC's §3-§15 incorporate all 5 AMEND-C entries. AMEND-C1 (per-route shape) + AMEND-C2 (registry key) + AMEND-C3 (failure_policy) + AMEND-C4 (env_vars) carry forward to IMPL as wire-shape pins; AMEND-C5 (conformance roster) carries forward to PLAN as the family-port transcription target.

### 1.2 ADR continuity + D-hypothesis at 25.3 SPEC commit

Phase 25.2 IMPL closed at ADR-0208 §Decision + §Consequences body; DECISIONS.md tail at ADR-0208; ADR-0209 reserved UNCONSUMED (R8 STANDS WEAK-default at `BenchmarkPerStreamModule_Instantiation = ~98 ns/op << 1ms`). **At THIS 25.3 SPEC commit: 3 NEW ADR §Context drafts anchor** per ADR-0044 §Context-draft discipline:

- **ADR-0210 §Context** — Per-route Wasm 5th-canonical REUSE-by-absence EXPLICIT-NO-NEW-CANONICAL classification per AMEND-A3 + AMEND-C1 (analogous to ADR-0173 / ADR-0180 / ADR-0197). ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment. §Context anchored at THIS SPEC commit; §Decision + §Consequences land at 25.3 IMPL.
- **ADR-0211 §Context** — Multi-plugin VM-sharing process-global registry per Q1 + AMEND-C2 + reload state machine per Q2 + AMEND-C3 + env_vars activation per Q3 + AMEND-C4 (BUNDLED per D-25.3-2 — the three surfaces are cohesive: reload invalidates all sharing plugin contexts; env_vars is a small VM-lifecycle surface). REFINES ADR-0205 anchor (in-place §Consequences acknowledgment at IMPL). §Context anchored at THIS SPEC commit.
- **ADR-0212 §Context** — `test/conformance/proxy-wasm/` harness seed per Q7 + AMEND-A8 + AMEND-C5 + pin SHA `proxy-wasm-cpp-host@da3ce05d` + 10-of-16 family port + 100%-of-10 phase-done gate + deliberate-break liveness discipline + 6-deferred-family forward-pointer roster. §Context anchored at THIS SPEC commit.

**D-25.3-2 bundle-vs-split disposition CLOSED at this SPEC commit: BUNDLE.** Q1 + Q2 + Q3 land under ONE ADR-0211. Rationale: (a) the surfaces are operationally cohesive (multi-plugin sharing + reload + env_vars are all VM-lifecycle concerns; reload of a shared `*RootVM` invalidates ALL plugin contexts under its key); (b) the combined §Decision body is anticipated ~400-550 LoC — within the ADR-body envelope (the empirical refinements add precision, not volume); (c) ADR-economy precedent at 22.3 + 25.2 (the 25.2 SPEC bundled body+buffer+trailer under one ADR-0206). The split alternative (ADR-0211 multi-plugin / ADR-0212 reload+env_vars / ADR-0213 conformance) is REJECTED — it would consume the reserve slot for no cohesion benefit. Conformance harness STAYS at ADR-0212; ADR-0213 STAYS a reserve.

**Next-free ADR after THIS 25.3 SPEC commit: `ADR-0213`** (3 numbers consumed: ADR-0210 + ADR-0211 + ADR-0212). ADR-0209 (carried from 25.2) + ADR-0213 (new reserve) are the STRENGTHENED-WEAK-HOLD-with-1-slot-buffer escape-valve pair per Q6.

**In-place AMEND acknowledgments at 25.3 IMPL** (no new ADR numbers): ADR-0205 §Consequences gains a one-line paragraph noting the registry-keyed-by-`(vm_id, vm_configuration, code)` refinement to the anchor `*RootVM per *compiledConfig`; ADR-0202 §Consequences STAYS UNCHANGED (API-REVISION ALLOWANCE clause remains scoped to consumer #2). ADR-0186 may gain a §Consequences RATIFIES-at-consumer-real-migration clause from the Q5 phase-21 clock migration (SPEC settles → anticipated yes; final at IMPL).

**D-hypothesis at 25.3 SPEC commit:** STRENGTHENED-WEAK-HOLD-with-1-slot-buffer STANDS (UNCHANGED from BRAINSTORM §7.3). 3 anticipated ADRs (ADR-0210 + ADR-0211 + ADR-0212) land cleanly at IMPL; ADR-0209 + ADR-0213 reserves anticipated UNCONSUMED (the §11 empirical pins produced 4 REFINES + 1 REFUTE that all absorb into the ADR-0210/0211/0212 §Decision anchors — NONE escalates a new ADR). The only escape-valve candidate is the R8 supplementary-benchmark surface (LOW probability per the 10,000× margin at 25.2; per Q6 the per-stream-context-lookup + per-route-resolve additions are tens-to-hundreds of ns).

---

## 2. Non-purposes

Phase 25.3 does NOT:

1. **Add a §9 HTTP-filters row.** `envoy.filters.http.wasm` is the EIGHTEENTH and FINAL §9 family-row landed at 25.1; 25.3 extends the SAME filter (per-route + multi-plugin + reload + env_vars + conformance are config-surface extensions + framework evolutions). Boot-registration UNCHANGED at 20 filters; `cmd/envoy-go/main.go` UNCHANGED.
2. **Introduce a new `internal/*` package.** 0 NEW package-level primitives; 1 NEW intra-package primitive (`internal/wasm/registry.go`) per §3.1. (The `test/conformance/proxy-wasm/` scaffolding is a test package, not an `internal/*` framework primitive.)
3. **Extend the proxy-wasm ABI.** 0 NEW hostcalls + 0 NEW guest callbacks + 0 NEW capability keys (D-25.3-4 / §11.6). `ABICallbacks` STAYS at 25 methods; capability roster STAYS at 58 keys; the v0.2.1 ABI boundary is unchanged (multi-plugin/reload are host-side; env_vars feeds existing WASI shims).
4. **Fix the HCM upstream-buffering parity gap** (fixture-0036 arms a-j) — DEFERRED to `http-body-buffering-parity-fix` cross-filter follow-up per Q4; arms stay skip-token at 25.3 phase-done; `//nolint:unused` annotations on the fixture-0036 driver helpers retained.
5. **Migrate phase-22.2 lua filterstate storage** (debt #2) — DEFERRED per Q5 to a consumer-#3-triggered cross-filter migration phase.
6. **Activate shared-queue / outbound-gRPC hostcalls, WasmService singletons, cluster-specifier-wasm / access-logger-wasm / network-filter-wasm, wazero JIT/AOT, `AsyncDataSource.Remote`, ABI v0.1.0/v0.2.0, or memory-trap fixtures** — all DEFERRED per parent SPEC §2.x + BRAINSTORM §8 (PARSE-REJECT continues per AMEND-A6 for the ABI versions; the WASM host family consumes `internal/wasm/` at consumer #2+).
7. **Run the 6 deferred conformance families** (shared_queue / signature_util / wasm-TLS-cache / vm_id_handle / null_vm / fuzz) — documented as forward-pointers in `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md` per AMEND-A8 + AMEND-C5.

This 25.3 SPEC INHERITS parent §2 (non-purposes + REUSES-not-consumed) + 25.1/25.2 §2 verbatim.

---

## 3. Framework primitive evolutions — 0 NEW package + 1 NEW intra-package (`internal/wasm/registry.go`) + 4 REUSES + 1 phase-21 MIGRATION

Phase 25.3 introduces 0 NEW package-level framework primitives + 1 NEW intra-package primitive (`internal/wasm/registry.go`) + 0 NEW go.mod direct dependencies + 1 in-place AMEND acknowledgment paragraph at ADR-0205 §Consequences (no new ADR number) + 4 framework REUSES + 1 phase-21 clock MIGRATION (Q5 debt #1 fold-in). STAYS the 22.3 + 25.2 CONSUME + DISPATCH + EXTEND posture at intra-package scope.

### 3.1 NEW intra-package: `internal/wasm/registry.go` (per Q1 + AMEND-C2; anchored at ADR-0211)

**Decision:** Extend `internal/wasm/` with a process-global `vm_id`-keyed registry. NOT a new `internal/*` package — lives at `internal/wasm/registry.go` as an extension of the existing framework primitive (matches ADR-0203 + ADR-0207 intra-package extension precedent). Anticipated production shape (provisional; production signatures land at 25.3 IMPL):

```go
// internal/wasm/registry.go
type Registry struct { /* unexported: mu sync.Mutex; entries map[string]*registryEntry */ }
type registryEntry struct { /* rootVM *RootVM; refcount int */ }

// DefaultRegistry is the process-global singleton (constructed in package init).
var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry
// AcquireRootVM looks up an existing *RootVM by makeVMKey(vmID, vmConfig, code); on hit
// bumps refcount + returns it; on miss constructs via factory + inserts at refcount 1.
func (r *Registry) AcquireRootVM(key string, factory func() (*RootVM, error)) (*RootVM, error)
// Release decrements refcount; at zero, removes the entry + calls rootVM.Close().
func (r *Registry) Release(key string) error

// makeVMKey mirrors cpp-host makeVmKey (src/wasm.cc:90-92): Sha256(vmID || "||" || vmConfig || "||" || code).
// Runtime is NOT in the key (envoy-go is wazero-single-runtime per AMEND-C2).
func makeVMKey(vmID string, vmConfig, code []byte) string
```

Cross-plugin shared-data visibility: the `*RootVM.sharedDataMap` is the shared-data store; at 25.3 the shared-data namespace is keyed by raw `vm_id` (AMEND-C2) so that distinct `*RootVM` instances under the same `vm_id` (differing only in `vm_configuration`/`code`) still observe one namespace — this requires a per-process `sharedDataByVmID map[string]*sharedDataStore` keyed by raw `vm_id`, looked up at `*RootVM` construction and shared by reference (NOT owned by a single `*RootVM`). The plugin-context isolation (one `*PluginContext` per `PluginConfig` sharing a VM, each with own `root_id`/configuration/capabilities) is materialized as a `*PluginContext` child of the shared `*RootVM`.

EXPLICIT-NO-NEW-PACKAGE rationale: the registry is internal to the framework primitive's lifecycle discipline + not consumed by other packages; promoting it to a sibling `internal/*` package would over-abstract a ~150-250 LoC lifecycle helper. **Anticipated LoC: ~150-300 LIVE + ~250-400 TEST** (registry refcount lifecycle + key construction + concurrent-acquire race tests + shared-data-by-vm_id scope tests).

**Anticipated ADRs:** ADR-0211 §Decision body anchors the registry shape + refcount lifecycle hooks (at `compiledConfig.New`/`compiledConfig.Close`) + the composite-key construction + the raw-`vm_id` shared-data scope + the two-layer-collapse divergence note. NO new ADR for the intra-package classification.

### 3.2 Reload state machine (per Q2 + AMEND-C3; anchored at ADR-0211)

**Decision:** NEW `internal/wasm/reload.go` (or an extension to `root_vm.go`; final file placement at PLAN) materializes the per-`*RootVM` reload state machine `{Running, Reloading, Failed}`. On a guest `RuntimeError` (panic/trap surfaced from a hostcall dispatch) under `failure_policy = FAIL_RELOAD`: transition `Running → Failed`, record `last_load` timestamp via `internal/clock.Clock`, and on the NEXT request-driven entry compute `interval = backoff.NextBackOffMs()`; if `now - last_load < interval` emit `vm_reload_backoff` + serve per the still-failed policy (FAIL_CLOSED 503 for the RuntimeError-but-within-backoff window); else attempt reload (re-instantiate the wazero Module + replay `proxy_on_vm_start` + `proxy_on_configure`) → on success `vm_reload_success` + `Failed → Running` + reset backoff; on failure `vm_reload_runtime_failure` + stay `Failed`. Backoff = `JitteredLowerBoundBackOffStrategy(base = max(operator_base_interval, 100ms); default 1s when reload_config unspecified)` — `max_interval` NOT consumed (AMEND-C3). Non-`RuntimeError` fail-states + non-`FAIL_RELOAD` policies route to `FAIL_CLOSED` (503) or `FAIL_OPEN` (bypass) at filter-build/dispatch time. Consumes `internal/clock.Clock` (THIRD co-consumer of ADR-0186). **Anticipated LoC: ~200-350 LIVE + ~300-450 TEST** (state machine + backoff + fake-clock reload-timing tests + RuntimeError-gating tests).

### 3.3 env_vars assembly (per Q3 + AMEND-C4; anchored at ADR-0211)

**Decision:** env-var assembly extends `internal/filter/http/wasm/compiled_config.go` (parse + collision-reject + cap-enforce at config-load) + `internal/wasm/root_vm.go` (feed the assembled `map[string]string` into the wazero WASI environ at Module instantiation; the WASI `environ_get`/`environ_sizes_get` shims read it as `KEY=VALUE\0`). Assembly order byte-faithful to upstream `plugin.cc`: (1) build a key-set from `key_values` keys ∪ `host_env_keys`; reject (PARSE-REJECT) on any cross-field duplicate; (2) insert `key_values` pairs; (3) insert `host_env_keys` keys present in the host process env (`os.Getenv`); skip absent ones silently. envoy-go-strict cap: reject if total entries > 64 OR any value > 4 KiB → PARSE-REJECT + `env_vars_cap_exceeded` counter. **Anticipated LoC: ~120-220 LIVE + ~200-300 TEST.**

### 3.4 REUSES (4 frameworks) + phase-21 clock MIGRATION (Q5)

- **REUSE 1: per-route 3-tier resolution** — the per-route Wasm wholesale-override via TPFC resolves through the existing 3-tier (per-route → listener-level → no-op) resolver from phase 13/14/15. NO API extension; the per-route override is a wholesale `Wasm`-message replacement (entire `*compiledConfig` swapped per-route), handled natively by the existing resolver. Lands at NEW `internal/filter/http/wasm/perroute.go` (intra-package; ~80-150 LoC) per the 22.3 `internal/filter/http/lua/perroute.go` precedent.
- **REUSE 2: `internal/clock/`** — Q2's reload state machine (§3.2) is the THIRD co-consumer of ADR-0186 (after phase-21 + 25.2 tick goroutine). RATIFIES ADR-0186 at multi-consumer-real-scope.
- **REUSE 3: ADR-0125 §canonical-per-route-roster** — the per-route Wasm TPFC override is the 5th-canonical REUSE-by-absence per AMEND-A3 + AMEND-C1. ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment. ADR-0210 anchors the EXPLICIT-NO-NEW-CANONICAL classification.
- **REUSE 4: HCM-parse-time PARSE-REJECT path** — lifts arms (failure_policy/reload_config, fail_open, multi-plugin vm_id, environment_variables, per-route TPFC) from PARSE-REJECT to CONSUMED; adds ~3 NEW arms (§6). Reuses the existing `compiled_config.go` parse path.
- **MIGRATION (Q5 debt #1):** phase-21 `internal/filter/http/adaptive_concurrency/clock.go` re-points its inline `defaultClock`/`fakeClock` references to consume the 25.2-extracted `internal/clock.Clock` interface + deletes the inline types (~50-100 LoC mechanical; non-breaking; phase-21 differential + unit tests STAY GREEN). RATIFIES ADR-0186 at consumer-real-migration scope (may close ADR-0186 via a §Consequences clause — SPEC settles → anticipated yes; final at IMPL).

NO new `internal/` package. NO new go.mod direct dependency.

### 3.5 `internal/wasm/` 25.3 file split (extends 25.2 split)

NEW: `internal/wasm/registry.go` (§3.1) + `internal/wasm/reload.go` (§3.2). EXTENDED: `internal/wasm/root_vm.go` (shared-data-by-vm_id scope + reload-state field + env feed) + `internal/wasm/shared_data.go` (vm_id-scoped store lookup). UNCHANGED: the ABI dispatch files (0 new hostcalls per D-25.3-4). Final file placement at PLAN.

### 3.6 `internal/filter/http/wasm/` 25.3 file split (extends 25.2 split)

NEW: `internal/filter/http/wasm/perroute.go` (REUSE 1; per-route 3-tier resolve). EXTENDED: `compiled_config.go` (failure_policy + reload_config + fail_open parse + mutual-exclusivity reject + environment_variables parse + collision-reject + cap; ~3 NEW PARSE-REJECT arms; multi-plugin registry `AcquireRootVM`/`Release` wiring at New/Close) + `stats.go` (4 NEW counters: vm_reload triplet + env_vars_cap_exceeded) + the filter struct (per-route resolution at DecodeHeaders entry). Final placement at PLAN.

### 3.7 Boot-registration UNCHANGED (per ADR-0072 + 25.1 SPEC §3.6)

20 HTTP filters wired; `cmd/envoy-go/main.go` UNCHANGED. The wasm filter's alphabetical boot-registration position is unchanged.

---

## 4. Per-route shape — 5th-canonical REUSE-by-absence (AMEND-A3 ABSENCE-DEFINITIVE + AMEND-C1; not re-Q'd)

Per parent SPEC §11.3 D3 + AMEND-A3 + §11.1 D-25.3-1 re-CONFIRMATION: the v1.37.2 + go-control-plane v1.32.4 `envoy.extensions.filters.http.wasm.v3` namespace defines ONLY `message Wasm { PluginConfig config = 1; }` — NO dedicated `WasmPerRoute` message; the factory does NOT register a route-specific config type. Per-route override is wholesale-override via `typed_per_filter_config` of the same `Wasm` message. ADR-0125 roster STAYS at 10 canonicals; NO §(xvi) amendment. The 5th-canonical wholesale-override pattern mirrors the phase-20 oauth2 + phase-21 adaptive_concurrency + phase-23 admission_control REUSE-by-absence precedent (EXPLICIT-NO-NEW-CANONICAL per ADR-0173 / ADR-0180 / ADR-0197).

ADR-0210 anchors the EXPLICIT-NO-NEW-CANONICAL classification + the per-route override resolution pattern + the cross-reference to AMEND-A3 ABSENCE-DEFINITIVE + AMEND-C1 (the "dangling vm_id" concept is MOOT — referential VM configs are unimplemented upstream). NO escalation to an 11th canonical (RETIRED at parent SPEC + INHERITED at 25.1 + 25.2 + 25.3). The per-route override is a wholesale `*compiledConfig` swap; per-route `disabled`/`override` semantics flow through the existing 3-tier resolver (REUSE 1).

---

## 5. Hostcall + Callback surface delta — 0 NEW (per D-25.3-4 / §11.6)

Per §11.6 D-25.3-4: the per-route TPFC + multi-plugin `vm_id` sharing + VM reload + `environment_variables` surfaces introduce **0 NEW guest-imported hostcalls + 0 NEW host-imported guest callbacks** beyond the proxy-wasm v0.2.1 ABI surface materialized at 25.1 + 25.2.

- **Multi-plugin sharing is host-side.** The guest sees `proxy_on_vm_start` (once per shared VM) + `proxy_on_configure`/`proxy_on_context_create` (per plugin/context) exactly as before; VM sharing is mediated entirely by the host registry. No sharing-specific ABI.
- **VM reload is host-side.** No `proxy_on_reload`/`proxy_on_restart` callback exists; the host re-instantiates + replays the existing lifecycle callbacks; the guest cannot distinguish a reload from a fresh start.
- **env_vars uses standard WASI** `environ_get` + `environ_sizes_get` (already registered shims at 25.1 + 25.2; they returned zeros pending AMEND-A6 deferral — 25.3 populates them). NO `proxy_*` env hostcall.

`internal/wasm/registration.go` host-module wiring UNCHANGED (no new registrations). `ABICallbacks` STAYS at 25 methods. The cumulative hostcall + callback count is UNCHANGED from 25.2 phase-done.

---

## 6. PARSE-REJECT roster — lifts 5 deferred arms to CONSUMED + ~3 NEW arms (24 → ~27 cumulative)

### 6.1 Wording discipline + arm-name convention UNCHANGED from 25.1/25.2

Byte-stable `parseReject*` constant convention per 25.1 SPEC §6.1; final byte-stable wording + arm numbering at IMPL via D-25.3-P2.

### 6.2 Arms LIFTED from PARSE-REJECT to CONSUMED at 25.3

- `failure_policy = FAIL_RELOAD` + `reload_config` (was PARSE-REJECT at 25.1/25.2) → CONSUMED per Q2 + AMEND-C3.
- deprecated `fail_open: bool` → CONSUMED (maps to `FAIL_OPEN`) per Q2 + AMEND-C3.
- multi-plugin `vm_id` sharing (duplicate `vm_id` across PluginConfigs) → CONSUMED per Q1 + AMEND-C2.
- `VmConfig.environment_variables` → CONSUMED per Q3 + AMEND-C4.
- per-route `typed_per_filter_config` Wasm wholesale-override → CONSUMED per AMEND-C1.

### 6.3 NEW PARSE-REJECT arms at 25.3 (~3 arms; final wording + count at IMPL via D-25.3-P2)

| # | Arm | Class | Source pin |
|---|---|---|---|
| A | env_vars key-collision (a key in BOTH `host_env_keys` and `key_values`) | UPSTREAM PARITY | `plugin.cc:30-42` `EnvoyException` "All the keys must be unique" |
| B | `fail_open` AND `failure_policy` both set | UPSTREAM PARITY | `wasm.cc:573-574` "only one of fail_open or failure_policy can be set" |
| C | env_vars cap-exceeded (> 64 total entries OR any value > 4 KiB) | envoy-go-strict | §11.4 (no upstream cap; envoy-go departure) → also increments `env_vars_cap_exceeded` |

Note: `key_values` on the null runtime (`plugin.cc:25-28`) is SUBSUMED by envoy-go's existing wazero-only-runtime PARSE-REJECT (envoy-go rejects any non-`envoy.wasm.runtime.v8`/non-wazero runtime upstream of env_vars parsing). `reload_config.backoff.base_interval` PGV violation (< 1ms) is handled by the existing PGV-mirror parse + the envoy-go-strict 100ms floor clamp (not a distinct reject arm — the floor clamps; below-PGV is a PGV-mirror reject already covered by the generic Duration validator).

### 6.4 Cumulative PARSE-REJECT roster

24 arms (post-25.2) → ~27 at 25.3 phase-done (+3 NEW; final count at IMPL). The 5 lifted arms become CONSUMED dispatch paths (not removed from the roster's historical record; reclassified).

---

## 7. Stat surface — +3 Group-C `vm_reload_*` triplet + 1 envoy-go-strict `env_vars_cap_exceeded` (128 → 132)

### 7.1 25.3 counter roster

| Name | Type | Source | Semantics |
|---|---|---|---|
| `wasm.<plugin>.vm_reload_success` | Counter | upstream Group-C | per successful VM reload after a `RuntimeError` failure under `FAIL_RELOAD` (mirrors upstream `VmReloadSuccess`) |
| `wasm.<plugin>.vm_reload_runtime_failure` | Counter | upstream Group-C | per failed VM reload attempt (mirrors upstream `VmReloadFailure`) |
| `wasm.<plugin>.vm_reload_backoff` | Counter | upstream Group-C | per backoff-deferred reload attempt within the backoff window (mirrors upstream `VmReloadBackoff`) |
| `wasm.<plugin>.env_vars_cap_exceeded` | Counter | envoy-go-strict | per `EnvironmentVariables` cap-exceeded PARSE-REJECT at config-load (§6.3 arm C) |

### 7.2 Stat-prefix template UNCHANGED from 25.1/25.2 per AMEND-A2 (tri-group structure; Group-C `wasm.<plugin_name>.<stat>`).

### 7.3 Project stat-count delta

128 → **132** at 25.3 phase-done (+3 Group-C `vm_reload_*` triplet per Q2 + 1 envoy-go-strict `env_vars_cap_exceeded` per Q3). The `wasmcustom.<custom_name>` dynamic-stats family (25.2) is unchanged + remains uncounted in the static total.

### 7.4 envoy-go-strict departure records (2 NEW at 25.3)

- **Departure A (Q2 reload-floor):** envoy-go-strict hardcoded floor `BackoffStrategy.base_interval = max(operator_value, 100ms)`; upstream honors the operator value verbatim with PGV `gte 1ms`. Rationale: prevent operator-misconfigured reload-storm. (Note: the BRAINSTORM-anticipated `max_interval` 1s floor is RETIRED — `max_interval` is DEAD in the upstream wasm path per AMEND-C3.)
- **Departure B (Q3 env_vars cap):** envoy-go-strict cap on `EnvironmentVariables` total entries (64 max) + per-value 4 KiB; upstream has no cap (§11.4). Rationale: defense-in-depth against operator-misconfigured massive env-bag injection.

Cumulative envoy-go-strict departure-record count ~27 → ~29 at 25.3 phase-done.

---

## 8. Differential fixtures (0038 cross-side + 0039 boot-reject) + conformance harness seed + fuzzer FOLD

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), each surface family gets a SEPARATE directory.

### 8.1 Fixture `0038-http-wasm-perroute-and-multi-plugin` (cross-side; deterministic)

Single-listener single-HCM hosting the wasm filter (alphabetical position) + router terminator. Two vendored Rust plugins (e.g. `perroute_dispatch.wasm` + `multi_plugin_shared_data.wasm`); two `PluginConfig` entries sharing one `vm_id`. Scenario partition (~6-10 scenarios; final at IMPL):
- **Per-route** (deterministic cross-side via `CompareBytes`): per-route override applies (override `*compiledConfig` swaps in a distinct plugin behavior); per-route disabled; listener-default applies on a route with no TPFC.
- **Multi-plugin** (deterministic + subject-side stats via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`): two PluginConfigs with the same `vm_id` share one VM (refcount = 2; one Module instance); distinct plugin contexts read/write the SAME shared-data namespace byte-faithfully (cross-plugin shared-data at vm_id scope per AMEND-C2).
- **Reload** (subject-only stats; non-deterministic timing → `StatsAsserter`): a plugin configured `failure_policy = FAIL_RELOAD` that panics on a trigger header → assert `vm_reload_runtime_failure`/`vm_reload_backoff`/`vm_reload_success` progression under fake-clock. (May move to a dedicated arm or fixture-0036-style subset at IMPL if cross-side determinism is unattainable.)

Every subject-side StatsAsserter arm gets a deliberate-break liveness verification (NOT dead-vacuous per phase-23 fixture-0030 + `reference_differential_asserter_dispatch`). Each scenario chosen for byte-exact wazero-vs-V8 determinism per AMEND-A4 + §4.5 D6 guardrails.

### 8.2 Fixture `0039-http-wasm-perroute-boot-reject` (subject-only boot-reject)

Single-arm boot-reject matching the 25.2 fixture-0037 `SubjectOnlyBootRejectFixture` shape (D-25.2-P1 precedent). Anticipated arm (final at IMPL via D-25.3-P1 empirical-scrape): env_vars cap-exceeded (> 64 entries OR per-value > 4 KiB) OR env_vars key-collision (§6.3 arm A/C). The BRAINSTORM-anticipated "dangling vm_id reference" arm is RETIRED (AMEND-C1: no dangling-vm_id concept). Subject-only because reference Envoy v1.37.2 silently drops the envoy-go-strict-only cap field per its protobuf parser (the collision-reject arm, being upstream-parity, would also reject on reference Envoy — so if the collision arm is chosen, the runner branch may be cross-side boot-reject; final dispatch-branch selection at IMPL per the one-dir-one-branch constraint).

### 8.3 Fixture-dir count

39 → **41** at 25.3 phase-done (+2: `0038-…` + `0039-…`). Total +6 across the phase-25 family (parent BRAINSTORM §6.4 anticipated +6 — CONFIRMED at family close). `test/conformance/proxy-wasm/` is OUTSIDE the differential fixture-dir count.

### 8.4 Conformance harness seed `test/conformance/proxy-wasm/` (per Q7 + AMEND-A8 + AMEND-C5 + ADR-0212)

NEW Go-test package + driver, SEPARATE from `test/fixtures/` + `test/differential/`. Mirrors phase-05 h2spec at `test/conformance/h2spec/`. The 16 `proxy-wasm-cpp-host@da3ce05d:test/*_test.cc` families re-expressed as Go-test sub-tests under `families/<family_name>/`; vendored `test_data` plugin fixtures ported to Go-test scaffolding. **10 PORT** (logging, stop_iteration/header-maps, shared_data, endianness, exports[env/clock/random], security, runtime/traps, wasm_vm, bytecode_util, pairs_util) / **6 DEFER** (shared_queue, signature_util, wasm[TLS-cache], vm_id_handle, null_vm, fuzz) per §11.5. Runs in CI per-commit via `go test ./test/conformance/proxy-wasm/...` (6-gate phase-done Gate F posture). Phase-done gate = ALL 10 ported families PASS (100% of the in-scope subset; 62.5% of the 16-family roster). Each family's subject-side assertion proven live via a deliberate-break cycle. The 6 deferred families' absence documented in `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md` + the 25.3 BEHAVIOR_CONTRACT bundle.

### 8.5 Fuzzer — FOLD into `FuzzWasmConfigParse` (no 36th fuzzer; per D-25.3-6)

35 fuzzers UNCHANGED. The 25.3 parse surface (per-route TPFC wholesale-override, failure_policy/reload_config, fail_open mutual-exclusivity, environment_variables collision + cap) FOLDS into the existing `FuzzWasmConfigParse` seed corpus — per-route is a wholesale `Wasm`-message reparse (no novel grammar) and reload/env_vars parse extend the same `PluginConfig`/`VmConfig` parse path the fuzzer already drives. The 35th fuzzer `FuzzWasmHostcallEnvelope` (25.2) + the must-never-panic invariant cover the hostcall surface (unchanged at 25.3). The BRAINSTORM/ROADMAP-anticipated `FuzzWasmPerRouteConfig` 36th fuzzer is RETIRED.

### 8.6 Listener topology

Single listener with a single HCM (wasm filter + router terminator). Anticipated NO second listener (avoid the `freeTCPPort` combined-run flake per phase-22.2 REVIEW §7.4). SPEC confirms NO second listener needed for any 25.3 scenario; multi-plugin VM-sharing + per-route override + reload probes all fit one listener.

---

## 9. Behavior-contract delta (semantic; 2 NEW departure records at 25.3)

Extends the `### envoy.filters.http.wasm` BEHAVIOR_CONTRACT.md subsection (25.1 + 25.2 bodies) at the 25.3 IMPL atomic-landing bundle per ADR-0052:

1. **Reload-floor departure** (§7.4 A) — `base_interval = max(operator, 100ms)`; floor hardcoded NOT operator-tunable.
2. **env_vars-cap departure** (§7.4 B) — 64 entries / 4 KiB; cap hardcoded; upstream has none.

Plus NON-departure wire-shape notes (parity, not divergence): per-route wholesale-override semantics (AMEND-C1); env_vars collision-reject (AMEND-C4; upstream parity); `fail_open`⊕`failure_policy` mutual-exclusivity (AMEND-C3; upstream parity); `FAIL_RELOAD`-gated-to-RuntimeError + `FAIL_CLOSED`-default (AMEND-C3; upstream parity); shared-data at vm_id scope (AMEND-C2; upstream parity). The `### Phase 25.2 forward-pointer notes` subsection is RENAMED/RESOLVED to `### Phase 25.3 forward-pointer notes` (or removed if all 25.2 forward-pointers close) at the 25.3 IMPL bundle. 25.3 SPEC defers the exact edit set to the IMPL ADR-0052 bundle (anticipated ~6-8 edits).

---

## 10. ADR anchor map (3 NEW §Context drafts at 25.3 SPEC commit; ADR-0205 + ADR-0186 in-place AMEND at IMPL; ADR-0209 + ADR-0213 reserves; ZERO ADR-0125 amendments)

Per ADR-0044 §Context-draft discipline. The ADR-0210 + ADR-0211 + ADR-0212 §Context drafts anchor at THIS 25.3 SPEC commit (appended to `DECISIONS.md`); §Decision + §Consequences bodies land at 25.3 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.

### 10.1 25.3 NEW ADRs (ADR-0210 + ADR-0211 + ADR-0212)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0210** | Per-route Wasm 5th-canonical REUSE-by-absence EXPLICIT-NO-NEW-CANONICAL classification per AMEND-A3 + AMEND-C1 (analogous to ADR-0173 / ADR-0180 / ADR-0197); ADR-0125 STAYS at 10 canonicals; NO §(xvi); "dangling vm_id" MOOT (referential VM configs unimplemented upstream); per-route wholesale-override via REUSE-1 3-tier resolver | §1.1 AMEND-C1; §4; §11.1 D-25.3-1 | 25.3 IMPL task that lands `internal/filter/http/wasm/perroute.go` |
| **ADR-0211** | Multi-plugin VM-sharing process-global registry per Q1 + AMEND-C2 (`Sha256(vm_id‖vm_configuration‖code)` key; runtime-not-in-key; raw-vm_id shared-data scope; two-layer-collapse) + reload state machine per Q2 + AMEND-C3 (`FailurePolicy` enum + RuntimeError-gating + `FAIL_CLOSED` default + base_interval-only backoff + 100ms floor + Group-C triplet + mutual-exclusivity reject) + env_vars activation per Q3 + AMEND-C4 (collision-reject + WASI feed + envoy-go-strict cap) BUNDLED per D-25.3-2; REFINES ADR-0205 anchor (in-place §Consequences acknowledgment at IMPL) | §1.1 AMEND-C2 + C3 + C4; §3.1 + §3.2 + §3.3 + §3.4 REUSE-2 + MIGRATION; §6.2 + §6.3; §7; §8.1; §11.2 + §11.3 + §11.4 + §11.6 | 25.3 IMPL task that materializes `internal/wasm/registry.go` + reload state machine |
| **ADR-0212** | `test/conformance/proxy-wasm/` harness seed per Q7 + AMEND-A8 + AMEND-C5 + pin SHA `proxy-wasm-cpp-host@da3ce05d` + 10-of-16 cpp-host unit-test family port + 100%-of-10 phase-done gate + deliberate-break liveness per family + 6-deferred-family forward-pointer roster | §1.1 AMEND-C5; §8.4; §11.5 D-25.3-3 | 25.3 IMPL task that materializes `test/conformance/proxy-wasm/` |

### 10.2 In-place AMEND acknowledgments at 25.3 IMPL (no new ADR numbers)

- **ADR-0205 §Consequences one-line acknowledgment** — registry-keyed-by-`(vm_id, vm_configuration, code)` refinement to the anchor `*RootVM per *compiledConfig` (provisional wording; settles at IMPL): *"Phase 25.3 lifts the per-`*compiledConfig` `*RootVM` to a per-`(vm_id, vm_configuration, code)` shared `*RootVM` via the process-global registry per ADR-0211; the ONE-`*RootVM`-per-`*compiledConfig` invariant becomes ONE-`*RootVM`-per-composite-key shared across `*compiledConfig` instances with matching identity. Cross-plugin shared-data scoped at raw-`vm_id` per AMEND-C2."* `**Status:**` line gains `AMENDED 2026-MM-DD per phase-25.3 one-line acknowledgment in §Consequences`.
- **ADR-0202 §Consequences STAYS UNCHANGED** — the API-REVISION ALLOWANCE clause for consumer #2 (WASM host family) remains scoped to consumer #2; 25.3's consumer-#1-internal evolutions land under ADR-0211.
- **ADR-0186 §Consequences RATIFIES-clause (anticipated)** — the Q5 phase-21 clock migration completes the EXTRACT-NOW-on-second-consumer loop; ADR-0186 may gain a one-line §Consequences clause noting the consumer-real-migration RATIFICATION (SPEC anticipates yes; final at IMPL).

### 10.3 ZERO ADR-0125 amendments at 25.3

Per AMEND-A3 STAYS-DEFINITIVE + AMEND-C1: ADR-0125's canonical roster STAYS at 10 entries — NO §(xvi) amendment. The BRAINSTORM-anticipated escape-valve "ADR-0125 amendment 10 → 11" is RETIRED at parent SPEC + INHERITED through 25.1/25.2/25.3.

### 10.4 Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts at 25.3 SPEC commit (this commit) | 3 | ADR-0210; ADR-0211; ADR-0212 |
| In-place §Consequences AMEND at 25.3 IMPL (no new ADR number) | 1-2 | ADR-0205 (registry refinement); ADR-0186 (RATIFIES clause, anticipated) |
| ADR-0044 escape-valve reserves (carry + new) | 2 (0 anticipated consumed) | ADR-0209 (carried from 25.2); ADR-0213 (new reserve) |
| ADR-0125 amendments | 0 (RETIRED per AMEND-A3 + C1) | NONE |
| In-place §Decision AMENDMENTs at 25.3 SPEC | 0 | NONE |

**Next-free ADR post-25.3-SPEC commit: `ADR-0213`** (3 NEW §Context drafts consumed: ADR-0210..ADR-0212; DECISIONS.md tail advances ADR-0208 → ADR-0212). Anticipated next-free after 25.3 phase-done: **`ADR-0213`** if ADR-0209 + ADR-0213 reserves UNCONSUMED; **`ADR-0214`** if one reserve fires. Phase-25 full ADR tail at family-close: anticipated **ADR-0202 .. ADR-0212** (matching parent §10.6 forward-projection with ADR-0205 + ADR-0213 reserves UNCONSUMED).

### 10.5 ADR-0044 escape-valve reserve + STRENGTHENED-WEAK-HOLD D-hypothesis

ADR-0209 (carried from 25.2; R8 escape-valve for per-stream Module instantiation) + ADR-0213 (new reserve for any 25.3-IMPL-time-unanticipated surface — e.g. wazero CompilationCache eviction under multi-plugin registry refcount lifecycle; reload-timing concurrency edge) form the STRENGTHENED-WEAK-HOLD-with-1-slot-buffer per Q6 + BRAINSTORM §7.3. Anticipated BOTH UNCONSUMED at 25.3 IMPL phase-done.

---

## 11. Empirical-pin block (D-25.3-1..D-25.3-7 resolved at this SPEC session)

This block contains the parallel-subagent-fan-out scrape evidence executed during this 25.3 SPEC drafting session per ADR-0004's hard-gate discipline. Mirrors the parent §11 + 25.2 §11 block structure scoped to the 25.3 surface. **Probe date: 2026-05-28.**

**Reference source corpus** (multi-axis verification per the phase-15..25.2 discipline):

1. **`envoyproxy/envoy@v1.37.2` IDL + C++ source** via WebFetch: `api/envoy/extensions/filters/http/wasm/v3/wasm.proto`; `api/envoy/extensions/wasm/v3/wasm.proto`; `api/envoy/config/core/v3/backoff.proto`; `source/extensions/filters/http/wasm/{config.h,config.cc,wasm_filter.h,wasm_filter.cc}`; `source/extensions/common/wasm/{wasm.cc,wasm.h,plugin.cc,plugin.h}`. Authoritative for the proto rosters + per-route factory + failure_policy mapping + env_vars assembly.
2. **`proxy-wasm/proxy-wasm-cpp-host@da3ce05d`** via WebFetch: `include/proxy-wasm/{wasm.h,wasm_vm.h,exports.h,context.h,context_interface.h}`; `src/{wasm.cc,context.cc,shared_data.cc,exports.cc}`; `src/shared_data.h`; `test/` directory listing. Authoritative for the registry/`makeVmKey`/refcount pattern + `FailState` enum + shared-data scope + WASI environ shims + the 16-file test roster.
3. **`go-control-plane v1.32.4` binding** (the project's pinned data-plane-api revision) — message-set confirmation for the HTTP wasm filter + wasm extension (single `Wasm` struct; `PluginConfig`/`VmConfig`/`ReloadConfig`/`FailurePolicy`/`EnvironmentVariables` present). *Empirical caveat (recorded for the PLAN author): the `go-control-plane` Go module tags as `v0.x` (latest `v0.14.0`); the project's "v1.32.4" anchor is the data-plane-api/Envoy API revision, NOT the gcp module tag. The v0.14.0 binding content AGREES with the v1.37.2 IDL on every 25.3-relevant message — no scope impact.*
4. **`proxy-wasm/spec@main:abi-versions/v0.2.1/README.md`** + **`proxy-wasm/proxy-wasm-rust-sdk@v0.2.4`** via WebFetch — ABI-surface confirmation (0 new hostcalls/callbacks for the 25.3 surface; queue/gRPC remain out-of-scope).

### Summary disposition table (7 pins → 5 AMEND-C entries + 2 internal closures)

| Pin | Topic | Disposition | AMEND / D-closure |
|---|---|---|---|
| §11.1 | D-25.3-1 — per-route TPFC parse + dangling vm_id | CONFIRMS no `WasmPerRoute` + wholesale TPFC + factory-no-route-config; REFINES — "dangling vm_id" MOOT (referential VM configs unimplemented upstream) | AMEND-C1 |
| §11.2 | Q1 — multi-plugin registry key + shared-data scope | REFINES — key = `Sha256(vm_id‖vm_configuration‖code)`; runtime NOT in key; shared-data at raw-vm_id scope; two-layer-collapse for envoy-go | AMEND-C2 |
| §11.3 | Q2 — failure_policy + reload backoff | REFINES — `FAIL_CLOSED` default; `FAIL_RELOAD` only on RuntimeError; base_interval-only backoff (max_interval dead); fail_open⊕failure_policy mutual-exclusive | AMEND-C3 |
| §11.4 | Q3 — environment_variables semantics | REFUTES "key_values overrides" → collision-REJECT; null-runtime reject; no upstream cap | AMEND-C4 |
| §11.5 | D-25.3-3 — conformance family roster | REFINES — cpp-host test/ is 16 UNIT-test files (not an ABI-conformance corpus); 10 port / 6 defer; 62.5% holds | AMEND-C5 |
| §11.6 | D-25.3-4 — ABI/capability delta | CONFIRMS — 0 NEW hostcalls + 0 NEW callbacks + 0 NEW capability keys (host-side-only surface) | D-25.3-4 CLOSED (58 keys; 25 ABICallbacks UNCHANGED) |
| §11.7 | D-25.3-2/5/6/7 — internal dispositions | D-25.3-2 BUNDLE; D-25.3-5 SAME 1ms threshold; D-25.3-6 FOLD (no 36th fuzzer); D-25.3-7 fixture-0039 arm → IMPL (dangling-vm_id RETIRED) | §10.1 + §12 |

### 11.1 D-25.3-1 — per-route TPFC parse + dangling vm_id semantic (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS AMEND-A3 ABSENCE-DEFINITIVE; REFINES the dangling-vm_id hypothesis to MOOT. → **AMEND-C1.**

- **No `WasmPerRoute` message.** `api/envoy/extensions/filters/http/wasm/v3/wasm.proto:19-22` defines exactly `message Wasm { envoy.extensions.wasm.v3.PluginConfig config = 1; }` (the whole file body). The go-control-plane binding declares exactly one struct `type Wasm struct` — no per-route type.
- **Factory registers no route-specific config.** `source/extensions/filters/http/wasm/config.h:20-66` `WasmFilterConfig` overrides only the `createFilterFactoryFromProto*` overloads + the private template; it does NOT override `createRouteSpecificFilterConfig`. `config.cc:1-20` only `REGISTER_FACTORY`s the filter + the upstream alias. `wasm_filter.{h,cc}` define only `FilterConfig` (a thin wrapper over `Common::Wasm::PluginConfig`), no per-route class. → per-route override is a generic HCM-machinery wholesale `Wasm` TPFC override.
- **`vm_id` is a pure sharing key; no dangling-reference concept.** `api/envoy/extensions/wasm/v3/wasm.proto:76-81`: `string vm_id = 1;` with the comment "An ID which will be used along with a hash of the wasm code … to determine which VM will be used … All plugins which use the same `vm_id` and code will use the same VM. May be left blank." The `vm` oneof (`:165-168`) carries an inline `VmConfig vm_config = 3;` with `// TODO: add referential VM configurations.` — referential (named cross-reference) VM configs are UNIMPLEMENTED. Every `PluginConfig` self-contains its `VmConfig`; there is no cross-reference for a dangling `vm_id` to dangle against. → the BRAINSTORM fixture-0039 "dangling vm_id" arm is RETIRED.
- **`PluginConfig` roster** (`api/envoy/extensions/wasm/v3/wasm.proto:153-200`, `[#next-free-field: 10]`): `name`(1), `root_id`(2), `vm_config`(3, in `oneof vm`), `configuration`(4, Any), `fail_open`(5, bool, deprecated-at-3.0), `capability_restriction_config`(6), `failure_policy`(7, FailurePolicy), `reload_config`(8, ReloadConfig), `allow_on_headers_stop_iteration`(9, BoolValue). **`VmConfig` roster** (`:75-140`): `vm_id`(1), `runtime`(2), `code`(3, AsyncDataSource), `configuration`(4, Any), `allow_precompiled`(5), `nack_on_code_cache_miss`(6), `environment_variables`(7, EnvironmentVariables).

### 11.2 Q1 — multi-plugin registry key + refcount + shared-data scope (driver: ADR-0004 hard-gate)

**Disposition:** REFINES BRAINSTORM Q1 keying. → **AMEND-C2.**

- **Key:** `makeVmKey(vm_id, vm_configuration, code) = Sha256({vm_id, "||", vm_configuration, "||", code})` (`src/wasm.cc:90-92`). Folds in `vm_configuration` + the full `code` bytes; **runtime NOT in the key** (engine arrives via the `WasmHandleFactory`, not the key). The `wasm.h:302-303` comment "vm_id + hash of code" is imprecise (omits vm_configuration).
- **Registry:** process-global `std::unordered_map<std::string, std::weak_ptr<WasmHandleBase>> *base_wasms` under `base_wasms_mutex` (`src/wasm.cc:39-40`); `createWasm` (`:546-589`) `.lock()`s on a matching key for reuse, else `factory(vm_key)` for fresh + inserts. Thread-local `local_wasms` clone layer (`:35-38`) sits in front for Envoy's per-worker model. Refcount via the returned `shared_ptr` plugin handles (`PluginHandleBase` pins `wasm_handle_`); maps hold `weak_ptr`; destructor `startShutdown` (`wasm.h:242-246`); stale weak_ptrs lazily evicted.
- **Plugin identity:** `makePluginKey(root_id, plugin_configuration, key) = Sha256(...)` (`src/context.cc:92-94`); `getOrCreateThreadLocalPlugin` (`:627-668`) keys `local_plugins` by `vm_key || plugin_key` → distinct per `PluginConfig`.
- **Shared-data scope:** `SharedData::data_` outer-keyed by the **raw user `vm_id`** (`src/shared_data.cc:44-79`; `deleteByVmId(vm_id)`), NOT the composite vm_key — broader than VM-instance scope (plugins with the same `vm_id` but differing config/code share the namespace).
- **envoy-go disposition:** key `Sha256(vm_id‖vm_configuration‖code)` (runtime dropped — wazero-only); shared-data at raw-vm_id scope; collapse the two-layer model into ONE process-global registry (no Go thread-local-worker equivalent).

### 11.3 Q2 — failure_policy + reload backoff state machine (driver: ADR-0004 hard-gate)

**Disposition:** REFINES BRAINSTORM Q2. → **AMEND-C3.**

- **`FailurePolicy` enum** (`api/envoy/extensions/wasm/v3/wasm.proto:24-42`): `UNSPECIFIED=0` (default → `FAIL_CLOSED`), `FAIL_RELOAD=1` (scoped to `FailState::RuntimeError`; falls back to `FAIL_CLOSED` for all other fail-states), `FAIL_CLOSED=2` (HTTP 503), `FAIL_OPEN=3` (bypass filter chain).
- **`fail_open` (deprecated)** PluginConfig field 5; **mutually exclusive** with `failure_policy` — `source/extensions/common/wasm/wasm.cc:573-574` throws "only one of fail_open or failure_policy can be set"; `fail_open=true → FAIL_OPEN` (`:578`); `false`/unset → UNSPECIFIED → `FAIL_CLOSED` (`:584`).
- **`reload_config`** is `message ReloadConfig { config.core.v3.BackoffStrategy backoff = 1; }` (`wasm.proto:44-48`), PluginConfig field 8, "only applied when failure_policy is FAIL_RELOAD". Upstream reads ONLY `backoff.base_interval` (`PROTOBUF_GET_MS_OR_DEFAULT(..., base_interval, 1000)` → default 1000ms; `wasm.cc:603-606`) feeding a `JitteredLowerBoundBackOffStrategy`; **`max_interval` is in the proto but DEAD in the wasm path.** `BackoffStrategy.base_interval` (`api/envoy/config/core/v3/backoff.proto:20-37`): Duration, `required:true`, PGV `gte 1ms`; `max_interval`: Duration, optional, PGV `gt 0`.
- **State machine:** `FailState { Ok=0, …, RuntimeError=7 }` (`wasm_vm.h:163-172`); `maybeReloadHandleIfNeeded` (`wasm.cc:482-536`) reloads ONLY if `fail_state == RuntimeError` AND `failure_policy == FAIL_RELOAD`; request-driven + backoff-rate-limited (`VmReloadBackoff` if within window; `VmReloadSuccess`/`VmReloadFailure` on attempt); terminal handling in `createContext` (`:653-663`): `FAIL_OPEN → nullptr` (bypass), else empty Context → 503.
- **envoy-go disposition:** honor `base_interval` + envoy-go-strict `max(·, 100ms)` floor; base_interval-only backoff; RETIRE the `max_interval` 1s floor (MOOT); implement `FAIL_CLOSED`(503)/`FAIL_OPEN`(bypass)/UNSPECIFIED→`FAIL_CLOSED`; PARSE-REJECT both-set; Group-C triplet mirrors upstream.

### 11.4 Q3 — environment_variables semantics + WASI feed (driver: ADR-0004 hard-gate)

**Disposition:** REFUTES BRAINSTORM Q3 "key_values overrides". → **AMEND-C4.**

- **Message:** `EnvironmentVariables { repeated string host_env_keys=1; map<string,string> key_values=2; }` on `VmConfig` field 7 (`wasm.proto:131-149`). Comment: "Envoy rejects the configuration if there's conflict of key space."
- **Merge = collision-REJECT, NOT override.** `source/extensions/common/wasm/plugin.cc:30-42` builds a key-set from `key_values` keys, then for each `host_env_keys` key throws `EnvoyException("Key {} is duplicated … All the keys must be unique.")` on any duplicate — BEFORE merging. Then `:44-51` inserts `key_values` pairs + `host_env_keys` present in the host env (`std::getenv`; absent keys silently skipped). `key_values` also rejected for the null runtime (`:25-28`). Stored as `std::unordered_map` (`plugin.h:20,33`; unordered iteration).
- **WASI feed:** `wasm.cc:75` passes `config.environmentVariables()` to the `WasmBase` ctor → `envs_` (`wasm.h:223`); `wasi_unstable_environ_get` (`src/exports.cc:802-826`) emits `KEY=VALUE\0` per entry; `wasi_unstable_environ_sizes_get` (`:830-846`) reports count + total size. No cap anywhere.
- **envoy-go disposition:** collision-reject (NEW arm; upstream parity); null-runtime reject (subsumed by wazero-only PARSE-REJECT); silent-skip absent host keys; feed wazero WASI environ; envoy-go-strict 64-entry/4-KiB cap (departure).

### 11.5 D-25.3-3 — conformance family roster (driver: ADR-0004 hard-gate)

**Disposition:** REFINES BRAINSTORM Q7 / parent AMEND-A8 framing. → **AMEND-C5.**

- **16 `*_test.cc` files at `proxy-wasm-cpp-host@da3ce05d:test/`:** bytecode_util, endianness, exports, logging, null_vm, pairs_util, runtime, security, shared_data, shared_queue, signature_util, stop_iteration, vm_id_handle, wasm, wasm_vm + `fuzz/` (+ `test_data/` plugin fixtures: abi_export.rs, callback.rs, clock.rs, env.rs, http_logging.cc, random.rs, stop_iteration.cc, trap.rs, …). The suite is engine/runtime UNIT tests — there is NO dedicated `http_call`/`metric`/`property`/`foreign_function`/`grpc`/`tick` test file at this pin (those ride along inside `logging`/`runtime` drivers + fixtures).
- **10 PORT** (host-capability-aligned): logging, stop_iteration (header maps + continue/stop-iteration), shared_data (CAS), endianness (ABI value/buffer marshalling), exports (env/clock/random WASI), security (hostcall restriction: `proxy_log` allowed / `proxy_done` gated), runtime (traps + mem-limit + host↔VM callback), wasm_vm (VM init/clone/memory), bytecode_util (custom-section + ABI-version parse), pairs_util (header-map pairs wire format).
- **6 DEFER** (with rationale): shared_queue (WasmService cross-VM queues + `proxy_on_queue_ready` — not implemented); signature_util (Ed25519 signed/remote code fetch — not implemented); wasm (thread-local WasmHandle cache + canary presupposes WasmService singleton model); vm_id_handle (cross-VM scoping substrate — defer with shared_queue/WasmService); null_vm (compiled-in NullVM C++-plugins-as-host-code — N/A for a Go host, no NullVM engine); fuzz (libFuzzer harnesses, not gtest conformance).
- **62.5% (10/16) numeric threshold HOLDS.** Flag for PLAN: `grpc`/`http_call`/`metric`/`property`/`foreign_function` have no dedicated cpp-host test family to port at this pin — the ported families exercise those surfaces only indirectly via the logging/runtime drivers + `test_data` fixtures.

### 11.6 D-25.3-4 — ABI + capability delta (driver: ADR-0004 hard-gate)

**Disposition:** CONFIRMS 0 NEW ABI surface → D-25.3-4 CLOSED.

- The proxy-wasm v0.2.1 roster (`proxy-wasm/spec@main:abi-versions/v0.2.1/README.md`) + cpp-host `exports.h` + `context_interface.h` contain NO vm_id/sharing/reload/failure-policy/env-specific `proxy_*` hostcall or `proxy_on_*` callback. Multi-plugin sharing + reload are HOST-SIDE (registry + state machine; the guest replays existing lifecycle callbacks); env_vars uses standard WASI `environ_get`/`environ_sizes_get`. Queue (`proxy_*_shared_queue`, `proxy_on_queue_ready`) + gRPC (`proxy_grpc_*`, `proxy_on_grpc_*`) hostcalls remain out-of-scope.
- **envoy-go disposition:** 0 NEW hostcalls + 0 NEW guest callbacks + **0 NEW capability keys** (env_vars feeds the already-registered WASI shims; multi-plugin/reload are host-side lifecycle, not capability-gated hostcalls). `ABICallbacks` STAYS at 25 methods; capability roster STAYS at 58 keys.

### 11.7 D-25.3-2/5/6/7 — internal-disposition pins

- **D-25.3-2 (bundle-vs-split): BUNDLE** — Q1+Q2+Q3 under ADR-0211 per §1.2 rationale.
- **D-25.3-5 (R8 supplementary-benchmark thresholds): SAME 1ms** — `BenchmarkPerStreamPluginContextLookup` + `BenchmarkPerRouteResolve` both gate at the `BenchmarkPerStreamModule_Instantiation` 1ms per-stream-cost threshold per D-25.2-P2 + parent §13-R8 (the threshold is per-stream-cost-total, not per-component).
- **D-25.3-6 (fuzzer): FOLD** — extend `FuzzWasmConfigParse` seed corpus; no 36th fuzzer (§8.5). 35 fuzzers UNCHANGED.
- **D-25.3-7 (fixture-0039 arm): → IMPL first-action** — candidate arms env_vars cap-exceeded OR env_vars key-collision (the BRAINSTORM "dangling vm_id" candidate RETIRED per AMEND-C1); final at IMPL via D-25.3-P1 empirical-scrape per the 25.2 D-25.2-P1 precedent.

---

## 12. SPEC-time D-questions for PLAN-time / IMPL-time resolution (D-25.3-P1 .. D-25.3-P4)

- **D-25.3-P1 (fixture-0039 boot-reject arm finalization):** first-action at the 25.3 IMPL fixture-0039 task — empirical-scrape reference Envoy v1.37.2 boot stderr for the candidate arms (env_vars cap-exceeded [subject-only; reference drops the envoy-go-strict field] vs env_vars key-collision [cross-side; both reject]); chosen arm + substring + runner-branch (subject-only vs cross-side boot-reject) recorded in PROGRESS.md per the one-dir-one-branch constraint.
- **D-25.3-P2 (PARSE-REJECT byte-stable wording + arm numbering):** final byte-stable `parseReject*` constant wording for the ~3 NEW arms (§6.3) finalized at IMPL; `compiled_config_test.go::TestParseRejectConstants_ByteStable` EXTENDED.
- **D-25.3-P3 (reload concurrency model):** the per-`*RootVM` reload state-machine concurrency discipline (mutex-per-RootVM serializing reload-vs-dispatch; how an in-flight stream observes a mid-reload VM) — PLAN settles the locking model; IMPL inherits. Anticipated: reload acquires the per-RootVM dispatch mutex; in-flight streams during a reload serve per the still-failed policy.
- **D-25.3-P4 (conformance harness driver shape):** the Go-test scaffolding shape for vendored cpp-host bytecode + `test_data` fixtures (how the ported plugins are built/vendored; whether the harness compiles the Rust `test_data` fixtures or vendors prebuilt `.wasm`) — PLAN settles per the phase-05 h2spec + 25.1 Task-15 Rust-toolchain pinning precedent.

Additional SPEC-time D-questions (D-25.3-2/5/6/7 from BRAINSTORM §10) are CLOSED in-session at §11.7.

---

## 13. RATIFIED-PENDING-IMPL items + BEHAVIOR_CONTRACT.md edit bundle

### 13.1 Parent §13 R1-R8 + 25.2 R-25.2-x disposition at 25.3 SPEC commit

| Item | Disposition at 25.3 SPEC |
|---|---|
| Parent R8 (per-stream Module instantiation escape-valve) | Carries to 25.3 IMPL; `BenchmarkPerStreamModule_Instantiation` re-run + 2 NEW supplementary benchmarks (`BenchmarkPerStreamPluginContextLookup` + `BenchmarkPerRouteResolve`) per Q6; SAME 1ms threshold per D-25.3-5; anticipated UNCONSUMED (10,000× margin at 25.2) |
| ADR-0209 reserve | Carries forward as the 25.3 IMPL R8 escape-valve slot (STAYS UNCONSUMED anticipated) |
| ADR-0186 Clock seam | RATIFIES at THIRD co-consumer (Q2 reload) + consumer-real-migration (Q5 phase-21 fold-in); anticipated §Consequences RATIFIES clause at IMPL |
| ADR-0205 root-VM lifecycle | REFINED to per-`(vm_id, vm_configuration, code)` shared `*RootVM`; in-place §Consequences acknowledgment at IMPL |

### 13.2 25.3-specific RATIFIED-PENDING-IMPL items (R-25.3-x)

- **R-25.3-1** (registry key wire-shape per AMEND-C2): golden test pinning `makeVMKey` = `Sha256(vm_id‖"||"‖vm_configuration‖"||"‖code)` byte-faithful to cpp-host (`internal/wasm/registry_test.go`).
- **R-25.3-2** (failure_policy mapping per AMEND-C3): table-test pinning `{UNSPECIFIED→FAIL_CLOSED, FAIL_RELOAD, FAIL_CLOSED→503, FAIL_OPEN→bypass}` + RuntimeError-gating + fail_open⊕failure_policy reject (`internal/filter/http/wasm/compiled_config_test.go` + `internal/wasm/reload_test.go`).
- **R-25.3-3** (env_vars collision-reject + cap per AMEND-C4): table-test pinning collision-reject + cap-reject + WASI environ `KEY=VALUE\0` feed (`internal/filter/http/wasm/compiled_config_test.go` + `internal/wasm/env_vars_test.go`).
- **R-25.3-4** (shared-data at vm_id scope per AMEND-C2): cross-plugin shared-data visibility test (two `*RootVM` under same vm_id share the namespace) (`internal/wasm/registry_test.go`).
- **R-25.3-5** (reload backoff timing per AMEND-C3): fake-clock test pinning base_interval floor + backoff progression + Group-C counter increments (`internal/wasm/reload_test.go`).
- **R-25.3-6** (conformance 10-of-16 family port + deliberate-break liveness per AMEND-C5): each ported family proven live (`test/conformance/proxy-wasm/families/*`).

### 13.3 BEHAVIOR_CONTRACT.md edit bundle anticipation (per ADR-0052 atomic landing)

~6-8 edits land atomically at the 25.3 IMPL final Task: EXTEND `### envoy.filters.http.wasm` with the 25.3 EXTENSION block (per-route + multi-plugin + reload + env_vars semantics); stat-table 128 → 132; 2 NEW envoy-go-strict departure records (reload-floor + env_vars-cap); RESOLVE/RENAME `### Phase 25.2 forward-pointer notes` → `### Phase 25.3 forward-pointer notes` (or remove if all close); ADD the 6-deferred-conformance-families forward-pointer roster (cross-ref `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md`). D-25.3-P2 byte-stable wording pinned at this bundle.

---

## 14. Test surface

- **Layer A:** unit tests at `internal/filter/http/wasm/` (per-route resolve + failure_policy/reload_config/fail_open parse + env_vars parse/collision/cap + ~3 NEW PARSE-REJECT arms + 4 NEW counters wiring).
- **Layer B:** unit tests at `internal/wasm/` (registry refcount lifecycle + concurrent-acquire race + shared-data-by-vm_id scope + reload state machine + fake-clock backoff + env_vars WASI feed).
- **Layer C:** phase-21 clock MIGRATION verification (`internal/filter/http/adaptive_concurrency/` tests STAY GREEN after re-pointing to `internal/clock/`).
- **Layer D:** `FuzzWasmConfigParse` seed-corpus extension (per-route + reload + env_vars; must-never-panic) — Layer for the FOLD per D-25.3-6.
- **Layer E:** differential fixture `0038-http-wasm-perroute-and-multi-plugin` GREEN (§8.1; deliberate-break liveness mandatory for subject-side StatsAsserter arms).
- **Layer F:** differential fixture `0039-http-wasm-perroute-boot-reject` GREEN (§8.2; arm + branch settled at IMPL).
- **Layer G:** NEW `test/conformance/proxy-wasm/` — 10 ported families GREEN; each proven live via deliberate-break (§8.4).
- **Layer H:** race + concurrency tests (registry concurrent-acquire/release; reload-vs-dispatch serialization per D-25.3-P3).
- **Six-gate checklist** (per 25.1/25.2 precedent): Gate A `go build`; Gate B `go vet` + `golangci-lint`; Gate C `go test -race -short ./...`; Gate D differential 41/41; Gate E fuzzers (35; corpus extended); Gate F h2spec 53/53 + NEW `go test ./test/conformance/proxy-wasm/...` 10/10.

---

## 15. 25.3 IMPL acceptance checklist (per phase-25.2 SPEC §15 precedent)

The 25.3 IMPL Task graph that lands the framework evolutions + package extensions + tests + fixtures + conformance harness + ADR landings + STATE.md re-advance + ROADMAP parent-row closure MUST satisfy ALL of:

**Framework primitive evolutions (per §3):**

1. `internal/wasm/registry.go` materialized per §3.1 (process-global `*Registry` + `makeVMKey` = `Sha256(vm_id‖vm_configuration‖code)` per AMEND-C2 + `AcquireRootVM`/`Release` refcount lifecycle + raw-vm_id shared-data scope).
2. `internal/wasm/reload.go` (or `root_vm.go` extension) materialized per §3.2 + AMEND-C3 (`{Running, Reloading, Failed}` state machine + RuntimeError-gating + base_interval-only backoff + 100ms floor + Group-C triplet).
3. env_vars assembly per §3.3 + AMEND-C4 (collision-reject + null-runtime reject + silent-skip absent host keys + WASI `KEY=VALUE\0` feed + 64/4-KiB cap).
4. `internal/wasm/root_vm.go` EXTENDED (shared-data-by-vm_id store + reload-state field + env feed at Module instantiation).
5. `internal/filter/http/wasm/perroute.go` materialized per REUSE-1 (per-route 3-tier wholesale-override resolve).
6. `internal/wasm/registration.go` UNCHANGED (0 NEW hostcalls per §5 + D-25.3-4); `ABICallbacks` STAYS at 25 methods; capability roster STAYS at 58 keys.

**Phase-21 clock MIGRATION (Q5 debt #1):**

7. `internal/filter/http/adaptive_concurrency/clock.go` re-points to `internal/clock.Clock` + deletes inline `defaultClock`/`fakeClock`; phase-21 tests + differential STAY GREEN (Layer C). RATIFIES ADR-0186 at consumer-real-migration scope.

**Filter package extensions + PARSE-REJECT:**

8. `internal/filter/http/wasm/compiled_config.go` EXTENDED: failure_policy + reload_config + fail_open parse + mutual-exclusivity reject; environment_variables parse + collision-reject + cap; multi-plugin `AcquireRootVM`/`Release` wiring at New()/Close(); ~3 NEW PARSE-REJECT arms (§6.3) byte-stable per D-25.3-P2.
9. `internal/filter/http/wasm/stats.go` EXTENDED: 4 NEW counters (vm_reload triplet + env_vars_cap_exceeded); 128 → 132.

**Stat surface + departures:**

10. 4 NEW counters wired (§7.1); BEHAVIOR_CONTRACT.md 128 → 132 (§13.3).
11. 2 NEW envoy-go-strict departure records (reload-floor + env_vars-cap) at BEHAVIOR_CONTRACT.md (§7.4 + §13.3); cumulative ~29.

**Fixtures + fuzzer + conformance:**

12. Differential fixture `0038-http-wasm-perroute-and-multi-plugin` GREEN — ~6-10 scenarios (§8.1); deliberate-break liveness for subject-side StatsAsserter arms.
13. Differential fixture `0039-http-wasm-perroute-boot-reject` GREEN — arm + runner-branch settled at IMPL per D-25.3-P1; 39 → 41 fixture dirs.
14. `FuzzWasmConfigParse` seed corpus EXTENDED (per-route + reload + env_vars); 35 fuzzers UNCHANGED (no 36th per D-25.3-6).
15. NEW `test/conformance/proxy-wasm/` — 10 ported families GREEN (§8.4 + AMEND-C5); each proven live via deliberate-break; runs via `go test ./test/conformance/proxy-wasm/...` in CI per-commit + Gate F; 6 deferred families documented in `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md`.

**Wire-shape pins (per §11 AMEND-C1..C5 + §13.2 R-25.3-x):**

16. Registry key per AMEND-C2 + R-25.3-1 (golden test).
17. failure_policy mapping + RuntimeError-gating + mutual-exclusivity per AMEND-C3 + R-25.3-2.
18. env_vars collision-reject + cap + WASI feed per AMEND-C4 + R-25.3-3.
19. shared-data at vm_id scope per AMEND-C2 + R-25.3-4.
20. reload backoff timing per AMEND-C3 + R-25.3-5 (fake-clock).
21. conformance 10-of-16 port + deliberate-break per AMEND-C5 + R-25.3-6.

**ADR landings:**

22. ADR-0210 + ADR-0211 + ADR-0212 §Decision + §Consequences bodies landed per the §Context anchors at THIS 25.3 SPEC commit (ADR-0044 in-place edit discipline).
23. ADR-0205 §Consequences one-line in-place AMEND acknowledgment (registry refinement per §10.2) + AMENDED timestamp in §Status; ADR-0202 §Consequences UNCHANGED; ADR-0186 §Consequences RATIFIES clause (anticipated, final at IMPL).
24. ADR-0209 + ADR-0213 reserve disposition closed at IMPL: STANDS-UNCONSUMED (anticipated) OR CONSUMED (§Decision + §Consequences for the surface that fired).

**STATE + ROADMAP (parent-row closure):**

25. STATE.md re-advance to `phase 25.3 IMPL done; §9 HTTP-filters family CLOSED` (or the family-close lifecycle marker per BOOTSTRAP §5).
26. ROADMAP sub-row 25.3 flipped `in-progress → done` + parent row 25 flipped `in-progress → done` ATOMICALLY in the same final-Task commit (ROLLUP per the 18/19/22/24 precedent; both lifecycle annotations in the commit-message body for grep-verifiability per ADR-0106).
27. Boot-registration UNCHANGED (20 HTTP filters; `cmd/envoy-go/main.go` UNCHANGED).

**R8 + benchmark gate (per Q6 + D-25.3-5):**

28. `BenchmarkPerStreamModule_Instantiation` re-run + 2 NEW supplementary benchmarks (`BenchmarkPerStreamPluginContextLookup` + `BenchmarkPerRouteResolve`); SAME 1ms per-stream-cost threshold; R8 disposition recorded in PROGRESS.md (anticipated STANDS WEAK-default; ADR-0209 + ADR-0213 reserves STAY UNCONSUMED).

**SPEC-time D-question closures (per §12):**

29. D-25.3-P1 closure at fixture-0039 task first-action (arm + branch in PROGRESS.md).
30. D-25.3-P2 closure at the BEHAVIOR_CONTRACT.md bundle landing (byte-stable wording).
31. D-25.3-P3 closure at PLAN (reload concurrency model) + IMPL inheritance.
32. D-25.3-P4 closure at PLAN (conformance harness driver shape).

The 25.3 IMPL is the FINAL phase-25 Task graph; its phase-done CLOSES the §9 HTTP-filters family (0 remaining rows).

---

## Appendix A — Cross-references to parent SPEC + 25.1 + 25.2 SPECs

This 25.3 SPEC cross-references (inherited verbatim; NOT duplicated):

- **Parent §1.1** (9-AMEND catalog A1-A9) — INHERITED; AMEND-A1 (wazero pin) + A3 (WasmPerRoute absence — load-bearing at §4 + §11.1) + A5 (default-deny sandbox; 0 new keys at 25.3) + A6 (ABI v0.1.0/v0.2.0 PARSE-REJECT carries forward) + A8 (conformance source pin — load-bearing at §8.4 + §11.5) are the 25.3-relevant entries.
- **Parent §3.1** (sub-phase surface-mapping table; 25.3 column) — the 25.3 dispositions (per-route + multi-plugin + reload + env_vars all CONSUMED; conformance seeded) materialize at this SPEC §1 + §3-§8.
- **Parent §11** (D1-D9 empirical-pin block) — INHERITED verbatim (NOT re-executed); this SPEC §11 ADDS D-25.3-1..D-25.3-7 resolved IN-SESSION.
- **Parent §13** (R1-R8) — INHERITED; R8 maps to the 25.3 IMPL benchmark gate (§13.1).
- **25.2 SPEC §3.1** (root-VM lifecycle / ADR-0205) — REFINED at §3.1 + §10.2 (registry-keyed shared `*RootVM`).
- **25.2 SPEC §5** (38 hostcalls / 25 ABICallbacks / 58 capability keys) — UNCHANGED at 25.3 per §5 + §11.6 (0 new ABI/keys).
- **25.2 SPEC §7** (128 stat names) — EXTENDED to 132 at §7.
- **25.2 SPEC §8** (fixture-0036 + 0037; 39 dirs) — EXTENDED with 0038 + 0039 (41 dirs) at §8.
- **25.2 REVIEW §7** (architectural debts) — debt #1 fold-in (§3.4 MIGRATION); debts #2 + #3 deferred (§2); debt #4 resolved via env_vars (§1).

## Appendix B — Phase 25.3 ADR landings summary

At THIS 25.3 SPEC commit: **3 NEW ADR §Context drafts consumed** (ADR-0210 + ADR-0211 + ADR-0212). DECISIONS.md tail advances ADR-0208 → ADR-0212. Next-free ADR advances to **ADR-0213** (reserve, with ADR-0209 carried from 25.2 — the STRENGTHENED-WEAK-HOLD-with-1-slot-buffer pair).

At 25.3 IMPL Final Task atomic landing:

- **ADR-0210 §Decision + §Consequences** — per-route 5th-canonical REUSE-by-absence per §4 + AMEND-C1.
- **ADR-0211 §Decision + §Consequences** — multi-plugin registry (Q1/AMEND-C2) + reload state machine (Q2/AMEND-C3) + env_vars (Q3/AMEND-C4) BUNDLED per §3.1-§3.3 + §6 + §7 + §8.1.
- **ADR-0212 §Decision + §Consequences** — conformance harness seed per §8.4 + AMEND-C5 + 10-of-16 port + 100%-of-10 gate.
- **ADR-0205 §Consequences** — one-line in-place AMEND acknowledgment (registry refinement) per §10.2.
- **ADR-0186 §Consequences** — RATIFIES clause from the Q5 phase-21 clock migration (anticipated).
- **ADR-0209 + ADR-0213 reserves** — STAY UNCONSUMED (anticipated) OR §Decision + §Consequences for the surface that fired.

At 25.3 IMPL phase-done: ROADMAP parent row 25 flips `in-progress → done` atomically with sub-row 25.3 `in-progress → done`; the §9 HTTP-filters family CLOSES (0 remaining rows); ADR tail at the phase-25 family-close anticipated `ADR-0202 .. ADR-0212`.

**End of phase 25.3 SPEC.**
