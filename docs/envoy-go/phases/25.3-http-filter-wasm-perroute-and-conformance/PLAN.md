# Phase 25.3 — HTTP filter `envoy.filters.http.wasm` (per-route TPFC wholesale-override + multi-plugin `vm_id`-shared VM registry + `failure_policy = FAIL_RELOAD` reload state machine + `VmConfig.environment_variables` + phase-21 clock MIGRATION + `test/conformance/proxy-wasm/` harness seed) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the LAST four operator-visible config surfaces of `envoy.filters.http.wasm` on top of the 25.1 headers-foundation + 25.2 full-advanced-bridge, and seed the proxy-wasm conformance harness — by (1) introducing the process-global `vm_id`-keyed `*Registry` at `internal/wasm/registry.go` (`makeVMKey = Sha256(vm_id‖"||"‖vm_configuration‖"||"‖code)` per AMEND-C2; refcounted `AcquireRootVM`/`Release` lifecycle; runtime NOT in key — wazero-single-runtime; cpp-host two-layer collapse to ONE process-global map); (2) broadening cross-plugin shared-data to **raw-`vm_id` scope** (AMEND-C2; distinct `*RootVM` under the same `vm_id` observe one namespace); (3) materializing the per-`*RootVM` reload state machine `{Running, Reloading, Failed}` at `internal/wasm/reload.go` (`FAIL_RELOAD` gated to guest `RuntimeError` only per AMEND-C3; request-driven + backoff-rate-limited via `BackoffStrategy.base_interval` only — `max_interval` DEAD upstream; envoy-go-strict `base_interval = max(operator, 100ms)` floor; Group-C `vm_reload_success`/`vm_reload_runtime_failure`/`vm_reload_backoff` triplet; `internal/clock.Clock` THIRD co-consumer of ADR-0186); (4) assembling `VmConfig.environment_variables` at `internal/wasm/env_vars.go` (cross-field collision PARSE-REJECT per AMEND-C4 — NOT override; null-runtime reject subsumed by wazero-only reject; silent-skip absent host keys; feed the WASI `environ_get`/`environ_sizes_get` shims as `KEY=VALUE\0`; envoy-go-strict 64-entry/4-KiB cap → PARSE-REJECT + `env_vars_cap_exceeded` counter); (5) materializing per-route wholesale `Wasm` TPFC override at `internal/filter/http/wasm/perroute.go` via the existing phase-13/14/15 3-tier resolver (REUSE-1; ADR-0210 EXPLICIT-NO-NEW-CANONICAL; NO `WasmPerRoute` message — AMEND-A3 + AMEND-C1; "dangling vm_id" MOOT); (6) lifting 5 deferred PARSE-REJECT arms (9 `failure_policy`/`reload_config`, 10 `fail_open`, 12 `vm_id`-duplicate, 13 `environment_variables`, 18 per-route) to CONSUMED dispatch + adding ~3 NEW arms (env_vars key-collision [upstream parity] + `fail_open`⊕`failure_policy` both-set [upstream parity] + env_vars cap-exceeded [envoy-go-strict]) at `compiled_config.go`; (7) MIGRATING phase-21 `internal/filter/http/adaptive_concurrency/clock.go` to consume the unified `internal/clock` seam (Q5 debt #1 fold-in; RATIFIES ADR-0186 at consumer-real-migration scope — requires reconciling the channel-based `internal/clock.Clock` with the callback-based `adaptive_concurrency.Clock` per PLAN-finding **F1** below); (8) adding 4 NEW counters (vm_reload triplet + `env_vars_cap_exceeded`) at `stats.go` — project stat surface **128 → 132**; (9) differential fixture `0038-http-wasm-perroute-and-multi-plugin` (cross-side per-route + multi-plugin + subject-side reload via `StatsAsserter`; deliberate-break liveness mandatory) + `0039-http-wasm-perroute-boot-reject` (boot-reject; arm + branch settled at IMPL via D-25.3-P1) — **39 → 41 fixture dirs**; (10) seeding `test/conformance/proxy-wasm/` (10-of-16 cpp-host unit-test families ported as in-process `go test` sub-tests per AMEND-C5 + ADR-0212; 100%-of-10 phase-done gate; deliberate-break liveness per family; 6 deferred families documented); (11) FOLDING the per-route + reload + env_vars parse surface into the existing `FuzzWasmConfigParse` seed corpus (no 36th fuzzer per D-25.3-6; **35 fuzzers UNCHANGED**); (12) the IMPL-Final atomic-landing Task (R8 benchmark re-run + 2 supplementary benchmarks; ADR-0210 + ADR-0211 + ADR-0212 §Decision+§Consequences bodies; ADR-0205 §Consequences one-line in-place AMEND; ADR-0186 §Consequences RATIFIES clause; BEHAVIOR_CONTRACT.md ~6-8-edit bundle; STATE.md re-advance + ROADMAP sub-row 25.3 + parent row 25 ROLLUP closure). By 25.3 phase-done the `envoy.filters.http.wasm` surface is COMPLETE modulo the ~29 documented envoy-go-strict departures (27 inherited + 2 NEW: reload-floor + env_vars-cap); the §9 HTTP-filters family CLOSES. **0 NEW hostcalls + 0 NEW guest callbacks + 0 NEW capability keys** (D-25.3-4; `ABICallbacks` STAYS 25; keys STAY 58). **Sub-phase landing (`25.3` ROADMAP row):** the 25.3 IMPL phase-done squash-merge CLOSES row 25.3 (`in-progress → done`) ATOMICALLY with parent row 25 (`in-progress → done`) per the 18/19/22/24 ROLLUP precedent (ADR-0106). This PLAN is for the 25.3 sub-phase ONLY.

**Architecture:** The 25.3 IMPL extends the existing `internal/wasm/` framework primitive at intra-package scope (0 NEW `internal/*` packages; 2 NEW intra-package files: `registry.go` + `reload.go`; 1 NEW assembly file `env_vars.go`; EXTENDED `root_vm.go` + `shared_data.go`) + extends `internal/filter/http/wasm/` (1 NEW file `perroute.go`; EXTENDED `compiled_config.go` + `stats.go` + the filter-struct/`decode_headers.go` dispatch path) + MIGRATES phase-21 `internal/filter/http/adaptive_concurrency/clock.go` onto a unified `internal/clock` seam + seeds a NEW test package `test/conformance/proxy-wasm/`. The multi-plugin lifecycle per Q1 + AMEND-C2 + ADR-0211: a process-global `wasm.DefaultRegistry` holds `map[string]*registryEntry` keyed by `makeVMKey(vmID, vmConfig, code)`; `compiledConfig.New` calls `DefaultRegistry.AcquireRootVM(key, factory)` (hit → reuse + refcount++; miss → factory-construct + insert at refcount 1); `compiledConfig.Close` calls `DefaultRegistry.Release(key)` (refcount→0 → remove + `rootVM.Close`). Each `PluginConfig` sharing a `*RootVM` still gets its own per-plugin scope (own `root_id`/configuration/capabilities) materialized as a child plugin-context of the shared `*RootVM`; cross-plugin shared-data is SHARED at raw-`vm_id` scope via a `sharedDataByVMID map[string]*sharedDataStore` owned by the registry and handed to each `*RootVM` at construction (so distinct VM-instance keys under the same `vm_id` share one store — broader than the composite key, byte-faithful to cpp-host `SharedData` outer-keying). The reload state machine per Q2 + AMEND-C3 + ADR-0211: a per-`*RootVM` `reloadState{state, lastLoad, backoff}` guarded by the existing `*RootVM.dispatchMu`; on a guest `RuntimeError` surfaced from a hostcall/callback dispatch under `failure_policy = FAIL_RELOAD`, the VM transitions `Running → Failed` + records `lastLoad` via `internal/clock.Clock.Now()`; on the NEXT request-driven dispatch entry (under `dispatchMu` — per D-25.3-P3 RESOLVED below), if `now - lastLoad < backoff.NextInterval()` the host emits `vm_reload_backoff` + serves the still-failed policy (FAIL_CLOSED 503), else it attempts a reload (re-instantiate the wazero Module + replay `proxy_on_vm_start` + `proxy_on_configure`) → on success `vm_reload_success` + `Failed → Running` + reset backoff; on failure `vm_reload_runtime_failure` + stays `Failed`. Backoff is a `base_interval`-only jittered-lower-bound strategy with `effectiveBase = max(operatorBaseInterval, 100ms)` (default 1s when `reload_config` unspecified). Non-`RuntimeError` fail-states + non-`FAIL_RELOAD` policies route to `FAIL_CLOSED` (503) or `FAIL_OPEN` (bypass) at dispatch time. env_vars per Q3 + AMEND-C4: `compiled_config.go` parses `EnvironmentVariables{host_env_keys, key_values}` at config-load → `wasm.AssembleEnvVars` builds the `map[string]string` (collision-reject across the two fields; cap-enforce; silent-skip absent host keys via `os.Getenv`) → the assembled map is fed to `*RootVM` at construction → `wasiEnvironGet`/`wasiEnvironSizesGet` (which returned 0/0 at 25.1/25.2 per AMEND-A6 deferral) now emit `KEY=VALUE\0` entries. Per-route per AMEND-C1 + ADR-0210: there is NO `WasmPerRoute` message — the per-route override is a wholesale `Wasm` message replacement; `perroute.go` reuses `buildCompiledConfig` to compile a per-route `*compiledConfig` and the filter resolves per-route → listener-default via the existing 3-tier resolver at `DecodeHeaders` entry (the entire `*compiledConfig`, hence its `*RootVM`, swaps per-route). The phase-21 clock MIGRATION per Q5 + F1: `internal/clock.Clock` is EXTENDED to a SUPERSET interface (`Now` + `After` [channel, wasm tick consumer] + `AfterFunc` [callback + `Stop` handle, adaptive_concurrency consumer]); `RealClock` + `FakeClock` gain `AfterFunc` implementations; `adaptive_concurrency` deletes its inline `Clock`/`Stop`/`defaultClock`/`timerStop`/`fakeClock` types and re-points to `clock.*` — its `Clock`-typed field shape is preserved in spirit (Now + AfterFunc still available). Boot-registration UNCHANGED (20 HTTP filters; `cmd/envoy-go/main.go` UNCHANGED). The conformance harness per Q7 + AMEND-C5 + ADR-0212 + D-25.3-P4: a pure in-process `go test` package (NO Docker — unlike phase-05 h2spec, the proxy-wasm guest runs in-process via wazero) at `test/conformance/proxy-wasm/`; each of 10 ported families is a sub-test under `families/<family_name>/` that loads a vendored prebuilt `.wasm` blob (committed to git per AMEND-A1 vendored-bytecode discipline) into a `*RootVM` and asserts host-observable behavior; each family proven live via a deliberate-break cycle.

**Tech Stack:** Go 1.26.2 (Go-floor STAYS `go 1.23.0` per wazero v1.10.1's Go-1.23 floor inherited from 25.1 + AMEND-A1); `go-control-plane` proto pin per ADR-0008 (`envoy/extensions/filters/http/wasm/v3` for `Wasm`; `envoy/extensions/wasm/v3` for `PluginConfig` + `VmConfig` + `ReloadConfig` + `FailurePolicy` + `EnvironmentVariables`; `envoy/config/core/v3` for `BackoffStrategy`); **NO new direct go.mod dependency at 25.3** (wazero v1.10.1 + proxy-wasm-rust-sdk =0.2.4 inherited); stdlib `crypto/sha256` (registry `makeVMKey`); stdlib `sync` (`sync.Mutex` for `*Registry.mu` + the existing `*RootVM.dispatchMu` serializing reload-vs-dispatch; `sync.RWMutex` for the vm_id-scoped shared-data store); stdlib `os` (`os.Getenv` for `host_env_keys` passthrough); stdlib `time` (backoff interval arithmetic); stdlib `errors` + `fmt` (byte-stable PARSE-REJECT consts); `internal/clock` (THIRD co-consumer of ADR-0186 + the F1 superset extension); reference Envoy `envoyproxy/envoy:v1.37.2` per ADR-0008 + ENVOY_TARGET.md (UNCHANGED); proxy-wasm spec v0.2.1 (sentinel `proxy_abi_version_0_2_1` UNCHANGED); proxy-wasm-rust-sdk `=0.2.4` + `wasm32-wasip1` Rust target (per AMEND-A1; reproduction-source language for the fixture-0038 plugins + the ported conformance `test_data` plugins under `sources/`; prebuilt `.wasm` vendored under `bytecode/`); proxy-wasm-cpp-host `da3ce05d` reference (transcription source for `makeVmKey` at `src/wasm.cc:90-92`; `FailState`/`maybeReloadHandleIfNeeded` at `wasm.cc:482-536`+`573-606`; env_vars assembly at `plugin.cc:25-51`; WASI environ at `exports.cc:802-846`; the 16-file conformance roster at `test/`); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness (NOT the conformance harness).

---

## Scope check — why phase 25.3 ships as one sub-phase row (STAY-SINGLE; PLAN-time split-gate confirms)

Phase 25 was PRE-SPLIT THREE-way at the parent BRAINSTORM per Q1 (envelope D delivered across 25.1 + 25.2 + 25.3). The 25.3 BRAINSTORM §1.4 estimated ~15-22 tasks + ~2,500-4,500 LIVE LoC. This PLAN re-evaluates the ADR-0045 split-gate against the actual Task graph and confirms STAY-SINGLE (no 25.3.1/25.3.2 split), matching the 22.2 + 22.3 stayed-single sub-phase precedent:

- **Task count: 15** — comfortably under the ADR-0045 25-task split-gate. Maps the SPEC §15 32-item acceptance checklist + §3 framework evolutions (2 NEW intra-package files + 1 assembly file + 2 EXTENDED + 1 MIGRATION) + §6 ~3 NEW PARSE-REJECT arms + 5 lifted arms + §7 4 NEW counters + §8 2 NEW fixtures + conformance harness seed + fuzzer FOLD + §10 3 ADR body landings into 15 discrete tasks across 6 tiers (Tier A `internal/wasm/` core evolution Tasks 1-4; Tier B phase-21 clock MIGRATION Task 5; Tier C `internal/filter/http/wasm/` extensions Tasks 6-9; Tier D fuzzer FOLD + differential fixtures Tasks 10-12; Tier E conformance harness seed Tasks 13-14; Tier F atomic landing Task 15).
- **LoC: ~2,600-4,400 production+test+fixture+docs** (per SPEC §3 per-surface envelopes: registry ~150-300 LIVE + ~250-400 TEST; reload ~200-350 LIVE + ~300-450 TEST; env_vars ~120-220 LIVE + ~200-300 TEST; perroute ~80-150 LIVE; compiled_config + stats extensions ~250-400 LIVE + ~300-450 TEST; phase-21 migration ~80-150 LIVE delta [F1 superset adds ~60-100 LoC over the SPEC's "mechanical 50-100" estimate]; fixtures 0038 + 0039 + Rust sources + vendored blobs ~600-1,000; conformance harness + 10 families + vendored blobs ~700-1,100; docs ~600-900). Within the BRAINSTORM §1.4 envelope. The IMPL LoC sits above the ~1,500-LoC soft-gate, but the soft-gate is about PLAN.md size, not IMPL LoC; this PLAN sits within the precedent envelope. The 22.2 + 25.1 + 25.2 stayed-single precedent RATIFIES the disposition.
- **Phase 25.3 ships as the single sub-phase row it is** — no nested split. The 25.3 IMPL phase-done squash-merge CLOSES row 25.3 (`in-progress → done`) ATOMICALLY with parent row 25 (`in-progress → done`) per the ROLLUP discipline (ADR-0106 + 18/19/22/24 precedent). The §9 HTTP-filters family CLOSES (0 remaining rows).

**Split disposition recorded: STAY-SINGLE 25.3 (15 tasks; no 25.3.1/25.3.2).**

---

## PLAN-time resolutions (D-25.3-P3, D-25.3-P4 settled here; F1 NEW finding; D-25.3-P1/P2 → IMPL first-actions)

Per SPEC §12, two SPEC-time D-questions settle at PLAN; two settle at IMPL first-action. This PLAN also surfaces one NEW finding (F1) the SPEC understated.

### F1 (NEW PLAN-finding) — the phase-21 clock MIGRATION is NOT a trivial re-point; it requires interface reconciliation

The SPEC §3.4 MIGRATION called the phase-21 clock fold-in "~50-100 LoC mechanical (non-breaking; re-point inline `defaultClock`/`fakeClock` to `internal/clock.Clock`)". **The two Clock interfaces have INCOMPATIBLE shapes:**

- `internal/clock.Clock` (the 25.2-extracted seam at `internal/clock/clock.go:53-66`): `Now() time.Time` + `After(d time.Duration) <-chan time.Time` — **channel-based** (the wasm tick goroutine's `case <-clk.After(d):` select-loop idiom).
- `adaptive_concurrency.Clock` (the phase-21 inline seam at `internal/filter/http/adaptive_concurrency/clock.go:42-49`): `Now() time.Time` + `AfterFunc(d time.Duration, fn func()) Stop` — **callback-based** with a `Stop` cancellation handle (the gradientController's timer-callback idiom).

A naive re-point would not compile — `internal/clock.Clock` has no `AfterFunc`. **Resolution:** extend `internal/clock` to a SUPERSET seam so BOTH consumers share ONE framework primitive (the ADR-0186 §Consequences (g) intent — "the consumer's Clock-typed field unchanged"):

1. Add `AfterFunc(d time.Duration, fn func()) Stop` to the `internal/clock.Clock` interface + a `Stop` interface (`Stop() bool`).
2. Implement `AfterFunc` on `RealClock` (wraps `time.AfterFunc` + a `*time.Timer`-backed `Stop`) and on `FakeClock` (port the phase-21 `fakeClock` AfterFunc fake-timer logic from `adaptive_concurrency/clock_test.go` into `internal/clock/clock.go`'s `FakeClock` — deadline-asc synchronous fire at `Advance`, `Stop()` semantics mirroring `time.Timer.Stop`).
3. `adaptive_concurrency` DELETES its inline `Clock` / `Stop` / `defaultClock` / `timerStop` types (clock.go) + its `fakeClock` / `fakeTimer` test types (clock_test.go) and re-points to `clock.Clock` / `clock.Stop` / `clock.RealClock` / `clock.FakeClock`. The gradientController's `Clock`-typed field keeps its `Now` + `AfterFunc` usage unchanged.
4. The wasm tick goroutine (25.2 consumer of `After`) is UNAFFECTED — adding `AfterFunc` to the interface only requires `RealClock`/`FakeClock` (framework-owned; the only implementors) to gain the method; no `After` consumer breaks.

This is non-breaking at the behavior level (phase-21 differential + unit tests STAY GREEN) but is an interface-superset extension, NOT a pure re-point. Task 5 carries this. RATIFIES ADR-0186 at consumer-real-migration scope (the §Consequences RATIFIES clause anticipated at IMPL is now CONFIRMED-warranted by the actual reconciliation).

### D-25.3-P3 RESOLVED — reload concurrency model = serialize reload-vs-dispatch under the existing `*RootVM.dispatchMu`

The per-`*RootVM` reload state machine is guarded by the **existing `*RootVM.dispatchMu sync.Mutex`** (already serializing hostcall dispatch per ADR-0205). The state-machine check + the reload attempt both execute while holding `dispatchMu`, so exactly one goroutine ever reloads a given VM; concurrent request goroutines block on `dispatchMu` and, on acquiring it, observe the post-reload state (Running on success → proceed; still-Failed within backoff → serve FAIL_CLOSED). An in-flight stream that triggered the `RuntimeError` completes its current dispatch (recording `Failed` + `lastLoad`); the NEXT stream to enter dispatch drives the reload-or-backoff decision. NO separate reload lock, NO background respawn goroutine (upstream is request-driven). This matches the existing per-RootVM serialization discipline and the SPEC §3.2 + §12 anticipated answer. IMPL inherits; `internal/wasm/reload_test.go` proves the serialization (concurrent dispatch-during-reload → no double-reload, no data race under `-race`).

### D-25.3-P4 RESOLVED — conformance harness driver = in-process `go test` + vendored prebuilt `.wasm` (NO Docker; NO Rust-build-in-CI)

The `test/conformance/proxy-wasm/` harness is a **pure in-process `go test` package** (NOT testcontainers/Docker like phase-05 h2spec — the proxy-wasm guest runs in-process via the project's own wazero `*RootVM`, so there is no external suite binary to containerize). Driver shape:
- `test/conformance/proxy-wasm/conformance.go` — shared harness helpers (load a vendored `.wasm` into a fresh `*RootVM` via the existing `wasm.NewRootVM`; capability/host-module wiring helpers; per-family assertion utilities).
- `test/conformance/proxy-wasm/conformance_test.go` — `TestProxyWasmConformance` parent test that ranges over the 10 ported families, each a `t.Run("<family_name>", ...)` sub-test delegating to `families/<family_name>/<family>_test.go`.
- `test/conformance/proxy-wasm/families/<family_name>/` — one dir per ported family; each holds the family's Go sub-test + (where needed) a small adapter.
- `test/conformance/proxy-wasm/bytecode/<family>.wasm` — **vendored prebuilt** `.wasm` blobs committed to git (AMEND-A1 vendored-bytecode discipline; mirrors fixture-0036 `bytecode/`).
- `test/conformance/proxy-wasm/sources/<family>/` — the Rust/C++ reproduction source + a `README.md` with the pinned `rustup target add wasm32-wasip1` + `cargo build --release --target wasm32-wasip1` invocation (build offline, vendor the blob — NO Rust toolchain dependency in CI, matching the 25.1 Task-15 + 25.2 fixture-0036 vendoring discipline).
- Runs in CI per-commit via `go test ./test/conformance/proxy-wasm/...` (Gate F). Phase-done gate = ALL 10 ported families PASS. Each family proven live via a deliberate-break cycle (break the assertion → expect FAIL → restore → GREEN) per `reference_differential_asserter_dispatch`.

This avoids a CI Rust-toolchain/Docker dependency for the conformance gate and matches the project's vendored-bytecode discipline. IMPL inherits.

### D-25.3-P1 → IMPL first-action (fixture-0039 boot-reject arm)

First-action at the Task 12 fixture-0039: empirical-scrape reference Envoy v1.37.2 boot stderr for the candidate arms — env_vars cap-exceeded (subject-only; reference silently drops the envoy-go-strict field) vs env_vars key-collision (cross-side; both reject). Chosen arm + substring + runner-branch (subject-only vs cross-side boot-reject) recorded in PROGRESS.md per the one-dir-one-branch constraint (`reference_differential_fixture_dispatch_constraint`). The BRAINSTORM "dangling vm_id" arm is RETIRED (AMEND-C1). Anticipated: env_vars cap-exceeded → subject-only `SubjectOnlyBootRejectFixture` (matching the 25.2 fixture-0037 shape).

### D-25.3-P2 → IMPL first-action (PARSE-REJECT byte-stable wording)

Final byte-stable `parseReject*` constant wording + arm numbering for the ~3 NEW arms (Task 7) finalized at IMPL; `compiled_config_test.go::TestParseRejectConstants_ByteStable` EXTENDED. Provisional wording proposed in Task 7; the IMPL author pins the final bytes + advances the arm-count assertion.

---

## Net change estimate (per file; IMPL may shift within ±20%)

**`internal/wasm/` (core evolution — Tier A):**
- `internal/wasm/registry.go` NEW ~150-300 (`Registry` + `registryEntry` + `DefaultRegistry` + `NewRegistry` + `AcquireRootVM` + `Release` + `makeVMKey` + `sharedDataByVMID` store + `AcquireSharedData`; Task 1+2)
- `internal/wasm/registry_test.go` NEW ~300-450 (makeVMKey golden + reuse-by-key + refcount-to-zero + distinct-keys + concurrent-acquire race + shared-data-by-vm_id visibility; Task 1+2)
- `internal/wasm/reload.go` NEW ~200-350 (`reloadState` + `{Running, Reloading, Failed}` + `noteRuntimeError` + `maybeReload` + `base_interval`-only jittered-lower-bound backoff + 100ms floor + Group-C triplet hooks; Task 3)
- `internal/wasm/reload_test.go` NEW ~300-450 (fake-clock backoff progression + 100ms floor + RuntimeError-gating + non-RuntimeError fall-through + reload-vs-dispatch serialization race per D-25.3-P3; Task 3)
- `internal/wasm/env_vars.go` NEW ~120-220 (`AssembleEnvVars` collision-reject + cap-enforce + silent-skip-absent + `KEY=VALUE\0` formatting; Task 4)
- `internal/wasm/env_vars_test.go` NEW ~200-300 (collision-reject + cap-reject [64 entries / 4 KiB] + absent-host-key skip + WASI environ round-trip; Task 4)
- `internal/wasm/root_vm.go` EXTEND ~+120-220 (vm_id-scoped shared-data store consumption [replaces the per-RootVM map]; reload-state field + `dispatchMu`-guarded reload hook; env field + WASI environ feed at instantiation; Tasks 1/2/3/4)
- `internal/wasm/shared_data.go` EXTEND ~+40-80 (store lookup keyed by raw vm_id; Task 2)
- `internal/wasm/wasi.go` EXTEND ~+40-70 (`wasiEnvironGet` + `wasiEnvironSizesGet` emit the assembled env instead of 0/0; Task 4)
- `internal/wasm/root_vm_test.go` EXTEND ~+80-150 (reload-state field + env-feed integration; Tasks 3/4)

**`internal/clock/` + phase-21 (Tier B):**
- `internal/clock/clock.go` EXTEND ~+80-140 (F1: add `AfterFunc(d, fn) Stop` to `Clock` + `Stop` interface + `RealClock.AfterFunc` + `FakeClock.AfterFunc`/`fakeTimer`; Task 5)
- `internal/clock/clock_test.go` EXTEND ~+100-180 (AfterFunc fire/Stop/Advance tests ported from the phase-21 fakeClock determinism suite; Task 5)
- `internal/filter/http/adaptive_concurrency/clock.go` REWRITE ~-60 net (DELETE inline `Clock`/`Stop`/`defaultClock`/`timerStop`; re-point to `clock.*`; Task 5)
- `internal/filter/http/adaptive_concurrency/clock_test.go` REWRITE ~-80 net (DELETE inline `fakeClock`/`fakeTimer`; re-point tests to `clock.FakeClock`; Task 5)
- `internal/filter/http/adaptive_concurrency/*.go` MODIFY ~+10-30 (type references `Clock`→`clock.Clock`, `Stop`→`clock.Stop`, `defaultClock{}`→`clock.RealClock{}`; Task 5)

**`internal/filter/http/wasm/` (extensions — Tier C):**
- `internal/filter/http/wasm/perroute.go` NEW ~80-150 (per-route wholesale `Wasm` TPFC parse → `buildCompiledConfig`; validator chokepoint; Task 6)
- `internal/filter/http/wasm/perroute_test.go` NEW ~150-250 (per-route compile + 3-tier resolve precedence + per-route disabled; Task 6)
- `internal/filter/http/wasm/compiled_config.go` EXTEND ~+250-400 (lift arms 9/10/12/13/18 to CONSUMED; failure_policy/reload_config/fail_open parse + mutual-exclusivity reject; env_vars parse + collision-reject + cap; multi-plugin `AcquireRootVM`/`Release` wiring at New/Close; ~3 NEW PARSE-REJECT arms; Task 7)
- `internal/filter/http/wasm/compiled_config_test.go` EXTEND ~+300-450 (failure_policy mapping table + RuntimeError-gating + mutual-exclusivity reject + env_vars collision/cap + lifted-arm CONSUMED-not-rejected coverage + `TestParseRejectConstants_ByteStable` EXTEND per D-25.3-P2; Task 7)
- `internal/filter/http/wasm/stats.go` EXTEND ~+60-100 (4 NEW counters: vm_reload triplet + env_vars_cap_exceeded; 128 → 132; Task 8)
- `internal/filter/http/wasm/stats_test.go` EXTEND ~+40-80 (4 NEW counter wiring + count assertion; Task 8)
- `internal/filter/http/wasm/decode_headers.go` EXTEND ~+40-80 (per-route resolution at entry; reload-on-RuntimeError dispatch integration; Task 9)
- `internal/filter/http/wasm/dispatch_test.go` EXTEND ~+120-200 (per-route end-to-end + reload-triggered FAIL_CLOSED/FAIL_OPEN/recovery; Task 9)

**Fuzzer FOLD + differential fixtures (Tier D):**
- `internal/filter/http/wasm/fuzz_config_test.go` + `testdata/fuzz/FuzzWasmConfigParse/` EXTEND ~+15-30 seeds (per-route + reload + env_vars; no 36th fuzzer; Task 10)
- `test/differential/fixture/fixture.go` MODIFY ~+15 (BackendKind for fixture-0038 OR REUSE; Task 11)
- `test/differential/runner_test.go` MODIFY ~+10 (blank imports for 0038 + 0039; Tasks 11/12)
- `test/fixtures/0038-http-wasm-perroute-and-multi-plugin/` NEW ~600-900 (README + envoy.yaml + envoy-go.yaml + expectations.yaml + inputs/driver.go + 2-4 Rust sources + vendored `.wasm` blobs; Task 11)
- `test/fixtures/0039-http-wasm-perroute-boot-reject/` NEW ~250-400 (README + envoy.yaml + envoy-go.yaml + inputs/driver.go; arm + branch at IMPL per D-25.3-P1; Task 12)

**Conformance harness seed (Tier E):**
- `test/conformance/proxy-wasm/conformance.go` NEW ~150-250 (shared harness helpers; Task 13)
- `test/conformance/proxy-wasm/conformance_test.go` NEW ~80-150 (`TestProxyWasmConformance` family iterator; Task 13)
- `test/conformance/proxy-wasm/README.md` NEW ~100-180 (roster + 10-port/6-defer rationale + reproduction discipline; Task 13)
- `test/conformance/proxy-wasm/families/<family>/<family>_test.go` NEW ~80-200 each × 10 (Task 14)
- `test/conformance/proxy-wasm/bytecode/<family>.wasm` NEW vendored blobs × ≤10 (Task 14)
- `test/conformance/proxy-wasm/sources/<family>/` NEW Rust/C++ sources + README (Task 14)

**Atomic landing (Tier F):**
- `internal/filter/http/wasm/wasm_bench_test.go` EXTEND ~+60-100 (`BenchmarkPerStreamModule_Instantiation` re-run + `BenchmarkPerStreamPluginContextLookup` + `BenchmarkPerRouteResolve`; Task 15)
- `docs/envoy-go/DECISIONS.md` MODIFY ~+500-800 (ADR-0210 + ADR-0211 + ADR-0212 §Decision+§Consequences bodies + ADR-0205 §Consequences one-line AMEND + ADR-0186 §Consequences RATIFIES clause + ADR-0209/0213 reserve disposition; Task 15)
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` MODIFY ~+250-400 (~6-8-edit bundle per SPEC §13.3; Task 15)
- `docs/envoy-go/STATE.md` + `ROADMAP.md` MODIFY (re-advance + ROLLUP closure; Task 15)
- `BOOTSTRAP_PROMPT.md` §7.5 + `ENVOY_TARGET.md` MODIFY (6-deferred-conformance-families forward-pointer roster; Task 15)
- `docs/envoy-go/phases/25.3-.../PROGRESS.md` + `REVIEW.md` NEW (Task 15)

---

## Task graph (sequential vs parallelizable)

```
Tier A (internal/wasm/ core):   1 → 2 → 3 → 4         (1 before 2: registry owns the vm_id store 2 broadens;
                                                        3 + 4 depend on root_vm fields from 1/2 but are
                                                        independent of each other → 3 ‖ 4 after 2)
Tier B (clock MIGRATION):       5                       (independent of Tier A; can run in parallel with A;
                                                        gated only by "internal/clock exists" — already true)
Tier C (filter extensions):     6 ‖ (7 → 8) → 9        (6 perroute independent; 7 compiled_config needs Tier A
                                                        registry+reload+env_vars; 8 stats after 7; 9 dispatch
                                                        needs 6 + 7 + 8)
Tier D (fuzzer + fixtures):     10 ‖ 11 ‖ 12           (all need Tier C green; mutually independent)
Tier E (conformance):           13 → 14                 (needs Tier A green; independent of Tier C/D)
Tier F (atomic landing):        15                      (needs ALL prior tasks green)
```

**Recommended execution order:** 1, 2, 3, 4, 5 (5 may interleave any time), 6, 7, 8, 9, 10, 11, 12, 13, 14, 15. Tier B (Task 5) is fully independent and may be done first if a subagent is idle. Within Tier D, Tasks 10/11/12 are independent. Each task ends GREEN (build + vet + lint + race-short + relevant gates) and commits before the next starts.

---

## Execution preconditions

1. **Worktree:** IMPL runs in a fresh worktree `.worktrees/phase-25.3-http-filter-wasm-perroute-and-conformance-impl` branched off master tip per `feedback_git_worktrees.md` + ADR-0003. (This PLAN was authored in `...-plan`.)
2. **Baseline GREEN at entry:** confirm `go build ./... && go vet ./... && golangci-lint run && go test -race -short ./... && (differential 39/39) && h2spec 53/53` before Task 1. Record the baseline in PROGRESS.md.
3. **Rust toolchain:** `rustup target add wasm32-wasip1` present (per 25.1 Task 15 + 25.2 Task 20) for building the fixture-0038 + conformance `.wasm` blobs offline; vendored blobs are committed so CI does NOT need Rust.
4. **SPEC is authoritative:** consume `SPEC.md` §15 (32-item checklist) + §11 (5 AMEND-C wire-shape pins) + §13.2 (R-25.3-1..R-25.3-6) as the Task-graph input. DO NOT re-litigate settled scope.
5. **Six-gate checklist per task** (per 25.1/25.2 precedent): Gate A `go build ./...`; Gate B `go vet ./...` + `golangci-lint run`; Gate C `go test -race -short ./...`; Gate D differential (39/39 until Task 11/12 → 41/41); Gate E fuzzers (35; corpus extended at Task 10); Gate F h2spec 53/53 + (from Task 13) `go test ./test/conformance/proxy-wasm/...`.

---

## Tier A — `internal/wasm/` core evolution (Tasks 1-4)

## Task 1: NEW `internal/wasm/registry.go` — process-global `vm_id`-keyed registry + `makeVMKey` + refcount lifecycle (per Q1 + AMEND-C2 + ADR-0211; R-25.3-1)

**Files:**
- Create: `internal/wasm/registry.go`
- Test: `internal/wasm/registry_test.go`

- [ ] **Step 1: Write the failing golden test for `makeVMKey` (R-25.3-1 wire-shape pin)**

```go
// internal/wasm/registry_test.go
package wasm

import "testing"

// TestMakeVMKey_ByteStable pins the registry key to cpp-host makeVmKey
// (src/wasm.cc:90-92): Sha256(vm_id || "||" || vm_configuration || "||" || code).
// Golden hex computed via: printf 'vm1||cfg||code' | sha256sum.
func TestMakeVMKey_ByteStable(t *testing.T) {
	cases := []struct {
		name           string
		vmID           string
		vmConfig, code []byte
		wantHex        string
	}{
		{"populated", "vm1", []byte("cfg"), []byte("code"),
			"31ac38d3d1be49b4258d350d2566947678c1a39a97d31ecc51c79201e0397813"},
		{"empty_vm_id", "", nil, []byte("code"),
			"0f94acb29bf7edf4c4dd4b131644d87778730b06bea27351bf4fad87de7c22a8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := makeVMKeyHex(tc.vmID, tc.vmConfig, tc.code)
			if got != tc.wantHex {
				t.Fatalf("makeVMKeyHex(%q,%q,%q) = %s, want %s",
					tc.vmID, tc.vmConfig, tc.code, got, tc.wantHex)
			}
		})
	}
}
```

(Note: the production `makeVMKey` returns the raw 32-byte digest as a `string` map key for byte-faithfulness; `makeVMKeyHex` is a thin hex wrapper used by the golden test so the pin is human-readable. The golden `wantHex` values are the verified outputs of `printf 'vm1||cfg||code' | sha256sum` and `printf '||||code' | sha256sum`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/wasm/ -run TestMakeVMKey_ByteStable -v`
Expected: FAIL — `undefined: makeVMKeyHex` (compile error).

- [ ] **Step 3: Write `makeVMKey` + `makeVMKeyHex` minimal impl**

```go
// internal/wasm/registry.go
package wasm

import (
	"crypto/sha256"
	"encoding/hex"
)

// makeVMKey mirrors proxy-wasm-cpp-host makeVmKey (src/wasm.cc:90-92):
// Sha256(vm_id || "||" || vm_configuration || "||" || code). Runtime is NOT in
// the key (envoy-go is wazero-single-runtime per AMEND-C2). Returns the raw
// 32-byte digest as a string for use as a map key.
func makeVMKey(vmID string, vmConfig, code []byte) string {
	h := sha256.New()
	h.Write([]byte(vmID))
	h.Write([]byte("||"))
	h.Write(vmConfig)
	h.Write([]byte("||"))
	h.Write(code)
	return string(h.Sum(nil))
}

// makeVMKeyHex is the hex-encoded form of makeVMKey (test/observability helper).
func makeVMKeyHex(vmID string, vmConfig, code []byte) string {
	return hex.EncodeToString([]byte(makeVMKey(vmID, vmConfig, code)))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestMakeVMKey_ByteStable -v`
Expected: PASS (both sub-cases).

- [ ] **Step 5: Write the failing refcount-lifecycle tests**

```go
// internal/wasm/registry_test.go (append)
func TestRegistry_AcquireReuseByKey(t *testing.T) {
	r := NewRegistry()
	calls := 0
	factory := func() (*RootVM, error) { calls++; return &RootVM{}, nil } // minimal stub
	key := makeVMKey("vm1", nil, []byte("code"))
	a, err := r.AcquireRootVM(key, factory)
	if err != nil { t.Fatal(err) }
	b, err := r.AcquireRootVM(key, factory)
	if err != nil { t.Fatal(err) }
	if a != b { t.Fatal("same key must return the same *RootVM") }
	if calls != 1 { t.Fatalf("factory called %d times, want 1 (reuse on hit)", calls) }
	if got := r.refcountFor(key); got != 2 { t.Fatalf("refcount = %d, want 2", got) }
}

func TestRegistry_ReleaseToZeroRemovesAndCloses(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vm2", nil, []byte("code"))
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
	_, _ = r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
	if err := r.Release(key); err != nil { t.Fatal(err) }
	if got := r.refcountFor(key); got != 1 { t.Fatalf("refcount = %d, want 1 after one Release", got) }
	if err := r.Release(key); err != nil { t.Fatal(err) }
	if r.has(key) { t.Fatal("entry must be removed at refcount 0") }
}
```

(`refcountFor` + `has` are unexported test-only observability helpers on `*Registry`. The `*RootVM{}` zero-value stub must survive `Close()` being a no-op when uninitialized; if `Close` cannot tolerate a zero-value, use a real `NewRootVM` minimal-module fixture from `fixtures_test.go` per the 25.2 test-double precedent.)

- [ ] **Step 6: Run to verify FAIL** — `go test ./internal/wasm/ -run TestRegistry -v` → FAIL (`undefined: NewRegistry`).

- [ ] **Step 7: Implement `Registry` + `AcquireRootVM` + `Release`**

```go
// internal/wasm/registry.go (append)
import "sync"

type registryEntry struct {
	rootVM   *RootVM
	refcount int
}

// Registry is the process-global VM-sharing registry per AMEND-C2 + ADR-0211.
// It collapses cpp-host's two-layer (process-global base_wasms + thread-local
// local_wasms) into ONE process-global map because Go has no Envoy thread-local
// worker model.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*registryEntry
}

// DefaultRegistry is the process-global singleton consumed by compiledConfig.
var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry { return &Registry{entries: map[string]*registryEntry{}} }

// AcquireRootVM returns an existing *RootVM for key (refcount++) or constructs
// one via factory at refcount 1.
func (r *Registry) AcquireRootVM(key string, factory func() (*RootVM, error)) (*RootVM, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[key]; ok {
		e.refcount++
		return e.rootVM, nil
	}
	vm, err := factory()
	if err != nil { return nil, err }
	r.entries[key] = &registryEntry{rootVM: vm, refcount: 1}
	return vm, nil
}

// Release decrements the refcount for key; at zero it removes the entry and
// closes the *RootVM.
func (r *Registry) Release(key string) error {
	r.mu.Lock()
	e, ok := r.entries[key]
	if !ok { r.mu.Unlock(); return nil }
	e.refcount--
	if e.refcount > 0 { r.mu.Unlock(); return nil }
	delete(r.entries, key)
	r.mu.Unlock()
	return e.rootVM.Close()
}

// unexported test-observability helpers:
func (r *Registry) refcountFor(key string) int {
	r.mu.Lock(); defer r.mu.Unlock()
	if e, ok := r.entries[key]; ok { return e.refcount }
	return 0
}
func (r *Registry) has(key string) bool {
	r.mu.Lock(); defer r.mu.Unlock()
	_, ok := r.entries[key]; return ok
}
```

- [ ] **Step 8: Write the concurrent-acquire race test**

```go
// internal/wasm/registry_test.go (append)
func TestRegistry_ConcurrentAcquireRelease(t *testing.T) {
	r := NewRegistry()
	key := makeVMKey("vmrace", nil, []byte("code"))
	const N = 64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.AcquireRootVM(key, func() (*RootVM, error) { return &RootVM{}, nil })
			if err != nil { t.Error(err) }
			_ = r.Release(key)
		}()
	}
	wg.Wait()
	if r.has(key) { t.Fatal("registry must be empty after balanced acquire/release") }
}
```

- [ ] **Step 9: Run with -race to verify GREEN** — `go test ./internal/wasm/ -run TestRegistry -race -v` → PASS.

- [ ] **Step 10: Gates + commit**

Run: `go build ./... && go vet ./... && go test -race -short ./internal/wasm/...`
Expected: PASS.
```bash
git add internal/wasm/registry.go internal/wasm/registry_test.go
git commit -m "phase-25.3 Task 1: internal/wasm/registry.go — vm_id-keyed registry + makeVMKey + refcount lifecycle (AMEND-C2; R-25.3-1)"
```

---

## Task 2: shared-data at raw-`vm_id` scope — broaden cross-plugin shared-data via a registry-owned `sharedDataByVMID` store (per AMEND-C2; R-25.3-4)

**Files:**
- Modify: `internal/wasm/registry.go` (add `sharedDataByVMID` + `AcquireSharedData`)
- Modify: `internal/wasm/shared_data.go` (store lookup by raw vm_id)
- Modify: `internal/wasm/root_vm.go` (consume the shared store instead of the per-RootVM map)
- Test: `internal/wasm/registry_test.go`

- [ ] **Step 1: Write the failing cross-plugin shared-data visibility test (R-25.3-4)**

```go
// internal/wasm/registry_test.go (append)
// Two *RootVM under the SAME vm_id but DIFFERENT composite keys (differing
// vm_configuration/code) MUST observe ONE shared-data namespace per AMEND-C2.
func TestRegistry_SharedDataAtVMIDScope(t *testing.T) {
	r := NewRegistry()
	s1 := r.AcquireSharedData("vmid-shared")
	s2 := r.AcquireSharedData("vmid-shared")
	if s1 != s2 { t.Fatal("same vm_id must yield the same shared-data store") }
	s3 := r.AcquireSharedData("vmid-other")
	if s1 == s3 { t.Fatal("distinct vm_id must yield distinct stores") }
}
```

- [ ] **Step 2: Run → FAIL** (`undefined: AcquireSharedData`). `go test ./internal/wasm/ -run TestRegistry_SharedDataAtVMIDScope -v`

- [ ] **Step 3: Implement `AcquireSharedData` + the store**

Add a `sharedDataByVMID map[string]*sharedDataStore` field to `Registry` (guarded by `mu`), where `*sharedDataStore` wraps the CAS-protected map currently inlined on `*RootVM` (`sharedData map[string]sharedDataEntry` + `sharedDataMu sync.RWMutex` + the value/entry caps). Extract that state from `root_vm.go` into a `sharedDataStore` type in `shared_data.go`; `AcquireSharedData(vmID)` returns the existing store or constructs one. `*RootVM` gains a `sharedData *sharedDataStore` pointer set at construction to `DefaultRegistry.AcquireSharedData(vmID)`, replacing the inline map. The existing `SetSharedData`/`GetSharedData` methods delegate to the store (signatures + CAS semantics UNCHANGED — this is a scope broadening, not a behavior change).

- [ ] **Step 4: Run → GREEN** — `go test ./internal/wasm/ -run TestRegistry_SharedDataAtVMIDScope -race -v` → PASS.

- [ ] **Step 5: Run the FULL existing shared-data suite to confirm non-breaking** — `go test ./internal/wasm/ -run 'SharedData|TestRootVM' -race -v` → PASS (the 25.2 CAS golden tests STAY GREEN; this only changes the store's ownership/scope).

- [ ] **Step 6: Gates + commit**
```bash
go build ./... && go vet ./... && go test -race -short ./internal/wasm/...
git add internal/wasm/registry.go internal/wasm/shared_data.go internal/wasm/root_vm.go internal/wasm/registry_test.go
git commit -m "phase-25.3 Task 2: shared-data at raw-vm_id scope via registry-owned store (AMEND-C2; R-25.3-4)"
```

---

## Task 3: NEW `internal/wasm/reload.go` — `{Running, Reloading, Failed}` reload state machine + RuntimeError-gating + base_interval-only backoff + 100ms floor + Clock THIRD co-consumer (per Q2 + AMEND-C3 + ADR-0211; R-25.3-2 + R-25.3-5; D-25.3-P3)

**Files:**
- Create: `internal/wasm/reload.go`
- Modify: `internal/wasm/root_vm.go` (reload-state field + `dispatchMu`-guarded hook + Group-C counter hooks)
- Test: `internal/wasm/reload_test.go`

- [ ] **Step 1: Write the failing backoff-floor + progression test (R-25.3-5; fake-clock)**

```go
// internal/wasm/reload_test.go
package wasm

import (
	"testing"
	"time"

	"github.com/<module>/internal/clock" // adjust import path to the project module
)

// effectiveBase = max(operatorBaseInterval, 100ms); 100ms envoy-go-strict floor.
func TestReloadBackoff_BaseIntervalFloor(t *testing.T) {
	cases := []struct{ op, want time.Duration }{
		{0, time.Second},               // unspecified → 1s default
		{50 * time.Millisecond, 100 * time.Millisecond}, // below floor → clamp
		{250 * time.Millisecond, 250 * time.Millisecond}, // above floor → honored
	}
	for _, tc := range cases {
		b := newReloadBackoff(tc.op)
		if got := b.baseInterval(); got != tc.want {
			t.Fatalf("op=%v: baseInterval=%v want %v", tc.op, got, tc.want)
		}
	}
}

// Within the backoff window → vm_reload_backoff + still-failed; past it → reload.
func TestReloadStateMachine_BackoffThenReload(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	rs := newReloadState(fc, 200*time.Millisecond) // base 200ms
	rs.noteRuntimeError() // Running -> Failed at t=0
	if rs.state() != reloadFailed { t.Fatal("want Failed after RuntimeError") }
	// t=100ms: within backoff → decision = backoff
	fc.Advance(100 * time.Millisecond)
	if d := rs.decide(); d != reloadDecisionBackoff {
		t.Fatalf("at t=100ms within 200ms window: decide=%v want backoff", d)
	}
	// t=250ms: past backoff → decision = attempt reload
	fc.Advance(150 * time.Millisecond)
	if d := rs.decide(); d != reloadDecisionAttempt {
		t.Fatalf("at t=250ms past 200ms window: decide=%v want attempt", d)
	}
}
```

- [ ] **Step 2: Run → FAIL** (`undefined: newReloadState` etc.). `go test ./internal/wasm/ -run TestReload -v`

- [ ] **Step 3: Implement the state machine + backoff**

`reload.go` defines: `reloadStateEnum {reloadRunning, reloadReloading, reloadFailed}`; `reloadDecision {reloadDecisionAttempt, reloadDecisionBackoff, reloadDecisionServe}`; `reloadBackoff` (base-interval-only jittered-lower-bound; `baseInterval()` = `max(op, 100ms)`, default 1s when op==0; `NextInterval()` returns the current jittered interval — mirror upstream `JitteredLowerBoundBackOffStrategy`; jitter MAY be deterministic-zero in tests via a seedable source); `reloadState{clk clock.Clock; mu; st; lastLoad time.Time; backoff *reloadBackoff}` with `noteRuntimeError()` (`Running → Failed`, `lastLoad = clk.Now()`), `decide()` (Failed + `clk.Now()-lastLoad < backoff.NextInterval()` → `Backoff`; Failed + past → `Attempt`; Running → `Serve`), `markReloaded()` (`Failed → Running`, reset backoff), `markReloadFailed()` (stays Failed, advance backoff). Counter hooks (`vm_reload_success`/`vm_reload_runtime_failure`/`vm_reload_backoff`) call into the `RootStatsRecorder` (Task 8 adds the methods; for Task 3 the hooks call nil-tolerant recorder methods or a local interface stub).

- [ ] **Step 4: Run → GREEN** — `go test ./internal/wasm/ -run TestReload -race -v` → PASS.

- [ ] **Step 5: Write the RuntimeError-gating + non-RuntimeError fall-through test (R-25.3-2)**

```go
// internal/wasm/reload_test.go (append)
// Only RuntimeError under FAIL_RELOAD triggers reload; other fail-states → FAIL_CLOSED.
func TestReload_GatedToRuntimeErrorUnderFailReload(t *testing.T) {
	// FAIL_RELOAD + RuntimeError → reload-eligible
	if !reloadEligible(FailurePolicyFailReload, FailStateRuntimeError) {
		t.Fatal("FAIL_RELOAD + RuntimeError must be reload-eligible")
	}
	// FAIL_RELOAD + non-RuntimeError → NOT eligible (falls back to FAIL_CLOSED)
	if reloadEligible(FailurePolicyFailReload, FailStateStartFailed) {
		t.Fatal("FAIL_RELOAD + StartFailed must NOT be reload-eligible")
	}
	// FAIL_CLOSED + RuntimeError → NOT eligible
	if reloadEligible(FailurePolicyFailClosed, FailStateRuntimeError) {
		t.Fatal("FAIL_CLOSED never reloads")
	}
}
```

(`FailurePolicy*` + `FailState*` enums land at Task 7's compiled_config parse; if Task 3 runs first, declare the minimal enum constants in `reload.go` and let Task 7 consume them — note the dependency in PROGRESS.md.)

- [ ] **Step 6: Implement `reloadEligible` + wire the `dispatchMu`-guarded reload hook into `root_vm.go`** (per D-25.3-P3: the dispatch entry, under `dispatchMu`, calls `rs.decide()`; on `Attempt` it re-instantiates the Module + replays `proxy_on_vm_start` + `proxy_on_configure`, then `markReloaded()`/`markReloadFailed()`; on `Backoff` it emits the counter + serves FAIL_CLOSED; the whole sequence holds `dispatchMu` so no double-reload).

- [ ] **Step 7: Write the reload-vs-dispatch serialization race test (D-25.3-P3)**

```go
// internal/wasm/reload_test.go (append) — N concurrent dispatch entries during a
// Failed state must drive AT MOST one reload attempt (no data race under -race).
func TestReload_SerializedUnderDispatchMu(t *testing.T) { /* spawn N goroutines
  that each call the dispatch-entry reload hook on one *RootVM in Failed state;
  assert reload attempt count <= 1 within a single backoff window; run under -race */ }
```

- [ ] **Step 8: Run → GREEN under -race** — `go test ./internal/wasm/ -run TestReload -race -v` → PASS.

- [ ] **Step 9: Gates + commit**
```bash
go build ./... && go vet ./... && go test -race -short ./internal/wasm/...
git add internal/wasm/reload.go internal/wasm/root_vm.go internal/wasm/reload_test.go
git commit -m "phase-25.3 Task 3: internal/wasm/reload.go — reload state machine + base_interval backoff + 100ms floor + RuntimeError-gating (AMEND-C3; R-25.3-2/5; D-25.3-P3)"
```

---

## Task 4: NEW `internal/wasm/env_vars.go` — `EnvironmentVariables` assembly (collision-reject + cap) + WASI environ feed (per Q3 + AMEND-C4; R-25.3-3)

**Files:**
- Create: `internal/wasm/env_vars.go`
- Modify: `internal/wasm/wasi.go` (`wasiEnvironGet` + `wasiEnvironSizesGet` emit the assembled env)
- Modify: `internal/wasm/root_vm.go` (env field set at construction; fed to WASI)
- Test: `internal/wasm/env_vars_test.go`

- [ ] **Step 1: Write the failing assembly tests (collision-reject + cap + skip-absent; R-25.3-3)**

```go
// internal/wasm/env_vars_test.go
package wasm

import (
	"os"
	"testing"
)

func TestAssembleEnvVars_CollisionReject(t *testing.T) {
	// a key in BOTH host_env_keys and key_values → reject (AMEND-C4; NOT override)
	_, err := AssembleEnvVars([]string{"DUP"}, map[string]string{"DUP": "v"})
	if err == nil { t.Fatal("cross-field key collision must reject (upstream parity)") }
}

func TestAssembleEnvVars_KeyValuesAndAbsentHostKey(t *testing.T) {
	os.Unsetenv("ENVOY_GO_ABSENT_XYZ")
	env, err := AssembleEnvVars([]string{"ENVOY_GO_ABSENT_XYZ"}, map[string]string{"K": "V"})
	if err != nil { t.Fatal(err) }
	if env["K"] != "V" { t.Fatalf("key_values not applied: %v", env) }
	if _, ok := env["ENVOY_GO_ABSENT_XYZ"]; ok {
		t.Fatal("absent host_env_key must be silently skipped")
	}
}

func TestAssembleEnvVars_CapExceeded(t *testing.T) {
	// envoy-go-strict cap: > 64 total entries OR any value > 4 KiB → reject
	big := map[string]string{}
	for i := 0; i < 65; i++ { big[string(rune('A'+i))+itoa(i)] = "x" }
	if _, err := AssembleEnvVars(nil, big); err == nil {
		t.Fatal("> 64 entries must reject (envoy-go-strict cap)")
	}
	if _, err := AssembleEnvVars(nil, map[string]string{"K": string(make([]byte, 4097))}); err == nil {
		t.Fatal("value > 4 KiB must reject (envoy-go-strict cap)")
	}
}
```

(`itoa` is a tiny test helper or use `strconv.Itoa`. The cap constants `envVarsMaxEntries = 64` + `envVarsMaxValueBytes = 4096` live in `env_vars.go`. `AssembleEnvVars` returns a typed cap-exceeded error the caller maps to the PARSE-REJECT arm + `env_vars_cap_exceeded` counter at Task 7/8.)

- [ ] **Step 2: Run → FAIL** (`undefined: AssembleEnvVars`). `go test ./internal/wasm/ -run TestAssembleEnvVars -v`

- [ ] **Step 3: Implement `AssembleEnvVars`** — build a key-set from `key_values` keys; for each `host_env_keys` key, reject on cross-field duplicate (byte-faithful to `plugin.cc:30-42`); insert `key_values` pairs; insert `host_env_keys` present in the host env via `os.Getenv` (skip absent silently); enforce `len > 64` → `ErrEnvVarsCapExceeded`, any `len(value) > 4096` → `ErrEnvVarsCapExceeded`. Return `map[string]string`.

- [ ] **Step 4: Run → GREEN** — `go test ./internal/wasm/ -run TestAssembleEnvVars -v` → PASS.

- [ ] **Step 5: Write the WASI environ feed round-trip test**

```go
// internal/wasm/env_vars_test.go (append) — environ_get/environ_sizes_get now emit
// KEY=VALUE\0 entries instead of 0/0. Drive via a *RootVM loaded with a guest that
// calls __wasi_environ_sizes_get + __wasi_environ_get and echoes them; OR unit-test
// the formatter encodeWASIEnviron(env) → [][]byte of "KEY=VALUE\0".
func TestEncodeWASIEnviron(t *testing.T) {
	got := encodeWASIEnviron(map[string]string{"A": "1", "B": "2"})
	// deterministic order: assert both entries present, NUL-terminated, KEY=VALUE form
	// (upstream uses unordered_map; envoy-go sorts for determinism — document the choice)
}
```

- [ ] **Step 6: Implement `encodeWASIEnviron` + wire into `wasi.go`** — `wasiEnvironSizesGet` writes count + total byte size; `wasiEnvironGet` writes the `KEY=VALUE\0` buffer + pointer table. The `*RootVM` holds the assembled `env map[string]string` (set at construction from the compiled config); the shims read it. Sort keys for deterministic output (document as an envoy-go-strict determinism choice — upstream iteration is unordered).

- [ ] **Step 7: Run → GREEN** — `go test ./internal/wasm/ -run 'TestAssembleEnvVars|TestEncodeWASIEnviron' -race -v` → PASS.

- [ ] **Step 8: Gates + commit**
```bash
go build ./... && go vet ./... && go test -race -short ./internal/wasm/...
git add internal/wasm/env_vars.go internal/wasm/wasi.go internal/wasm/root_vm.go internal/wasm/env_vars_test.go
git commit -m "phase-25.3 Task 4: internal/wasm/env_vars.go — env assembly (collision-reject + cap) + WASI environ feed (AMEND-C4; R-25.3-3)"
```

---

## Tier B — phase-21 clock MIGRATION (Task 5)

## Task 5: phase-21 `adaptive_concurrency/clock.go` MIGRATION onto a unified `internal/clock` superset seam (per Q5 debt #1 + F1; RATIFIES ADR-0186 at consumer-real-migration scope)

**Files:**
- Modify: `internal/clock/clock.go` (F1: add `AfterFunc` + `Stop` to the seam)
- Modify: `internal/clock/clock_test.go` (AfterFunc fire/Stop tests)
- Rewrite: `internal/filter/http/adaptive_concurrency/clock.go` (DELETE inline types; re-point)
- Rewrite: `internal/filter/http/adaptive_concurrency/clock_test.go` (DELETE inline fake; re-point)
- Modify: `internal/filter/http/adaptive_concurrency/*.go` (type references)

- [ ] **Step 1: Write the failing `internal/clock` AfterFunc test (F1)**

```go
// internal/clock/clock_test.go (append)
func TestRealClock_AfterFunc_Fires(t *testing.T) {
	done := make(chan struct{})
	clock.RealClock{}.AfterFunc(1*time.Millisecond, func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("AfterFunc fn did not fire")
	}
}

func TestFakeClock_AfterFunc_FiresOnAdvance_AndStop(t *testing.T) {
	fc := clock.NewFakeClock(time.Unix(0, 0))
	var fired int
	st := fc.AfterFunc(10*time.Millisecond, func() { fired++ })
	fc.Advance(5 * time.Millisecond)
	if fired != 0 { t.Fatal("must not fire before deadline") }
	fc.Advance(5 * time.Millisecond)
	if fired != 1 { t.Fatalf("fired=%d want 1 at deadline", fired) }
	// Stop after fire returns false (mirrors time.Timer.Stop)
	if st.Stop() { t.Fatal("Stop after fire must return false") }
	// A second timer Stopped before its deadline must NOT fire.
	st2 := fc.AfterFunc(10*time.Millisecond, func() { fired++ })
	if !st2.Stop() { t.Fatal("Stop before fire must return true") }
	fc.Advance(20 * time.Millisecond)
	if fired != 1 { t.Fatalf("stopped timer fired: fired=%d want 1", fired) }
}
```

- [ ] **Step 2: Run → FAIL** (`RealClock` / `FakeClock` have no `AfterFunc`). `go test ./internal/clock/ -run AfterFunc -v`

- [ ] **Step 3: Extend `internal/clock/clock.go` (F1)** — add `AfterFunc(d time.Duration, fn func()) Stop` to the `Clock` interface; add the `Stop` interface (`Stop() bool`); implement `RealClock.AfterFunc` (wrap `time.AfterFunc`; `Stop` adapts `*time.Timer.Stop`); implement `FakeClock.AfterFunc` (register a `fakeTimer{deadline, fn, stopped, fired}`; `Advance` fires due timers in deadline-asc/insertion-seq order calling `fn`; `Stop()` mirrors `time.Timer.Stop`). Port the proven logic from the phase-21 `adaptive_concurrency/clock_test.go` `fakeClock`/`fakeTimer`.

- [ ] **Step 4: Run → GREEN** — `go test ./internal/clock/ -race -v` → PASS (existing `After` tests + new `AfterFunc` tests).

- [ ] **Step 5: Re-point `adaptive_concurrency`** — DELETE the inline `Clock`/`Stop`/`defaultClock`/`timerStop` from `clock.go` and the inline `fakeClock`/`fakeTimer` from `clock_test.go`; replace `clock.go` with a one-line doc-comment file (or remove it and adjust the package); update every reference: `Clock`→`clock.Clock`, `Stop`→`clock.Stop`, `defaultClock{}`→`clock.RealClock{}`, `newFakeClock(...)`→`clock.NewFakeClock(...)`. Add the `internal/clock` import.

- [ ] **Step 6: Run the FULL phase-21 suite to confirm non-breaking** — `go test ./internal/filter/http/adaptive_concurrency/... -race -v` → PASS (behavior unchanged; the gradientController's `Clock`-typed field keeps Now+AfterFunc usage).

- [ ] **Step 7: Gates (incl. differential for phase-21 fixture) + commit**
```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
git add internal/clock/ internal/filter/http/adaptive_concurrency/
git commit -m "phase-25.3 Task 5: MIGRATE phase-21 clock to unified internal/clock superset (AfterFunc+Stop); RATIFIES ADR-0186 (Q5 debt #1; F1)"
```

---

## Tier C — `internal/filter/http/wasm/` extensions (Tasks 6-9)

## Task 6: NEW `internal/filter/http/wasm/perroute.go` — per-route wholesale `Wasm` TPFC override (REUSE-1; ADR-0210; SPEC §15 item 5)

**Files:**
- Create: `internal/filter/http/wasm/perroute.go`
- Test: `internal/filter/http/wasm/perroute_test.go`

- [ ] **Step 1: Write the failing per-route parse + resolve test**

```go
// internal/filter/http/wasm/perroute_test.go — per-route TPFC is a WHOLESALE Wasm
// message (NO WasmPerRoute type per AMEND-C1); parsePerRouteWasm compiles it via the
// same buildCompiledConfig path; the 3-tier resolver returns per-route over listener.
func TestParsePerRouteWasm_WholesaleCompile(t *testing.T) { /* a valid per-route Wasm
  proto compiles to a *compiledConfig; an invalid one returns the same PARSE-REJECT
  wording as the listener path */ }

func TestResolvePerRoute_PrecedenceOverListener(t *testing.T) { /* given a listener
  *compiledConfig L and a per-route *compiledConfig R, resolve(route=R, listener=L)
  returns R; resolve(route=nil, listener=L) returns L */ }
```

- [ ] **Step 2: Run → FAIL** (`undefined: parsePerRouteWasm`). `go test ./internal/filter/http/wasm/ -run PerRoute -v`

- [ ] **Step 3: Implement `perroute.go`** — `parsePerRouteWasm(m proto.Message) (*compiledConfig, error)` type-asserts to the SAME `*wasmv3.Wasm` message (per AMEND-C1 — wholesale, no oneof) and delegates to `buildCompiledConfig`; the validator chokepoint (analogous to lua's `validatePerRouteLua`) routes per-route validation through the same parse path. Add a `resolvePerRoute(routeCfg, listenerCfg *compiledConfig) *compiledConfig` 3-tier helper (per-route → listener → nil) per the phase-13/14/15 resolver precedent. NO new canonical (ADR-0210 EXPLICIT-NO-NEW-CANONICAL).

- [ ] **Step 4: Run → GREEN** — `go test ./internal/filter/http/wasm/ -run PerRoute -race -v` → PASS.

- [ ] **Step 5: Gates + commit**
```bash
go build ./... && go vet ./... && go test -race -short ./internal/filter/http/wasm/...
git add internal/filter/http/wasm/perroute.go internal/filter/http/wasm/perroute_test.go
git commit -m "phase-25.3 Task 6: perroute.go — per-route wholesale Wasm TPFC override (REUSE-1; ADR-0210; AMEND-C1)"
```

---

## Task 7: `compiled_config.go` EXTEND — lift arms 9/10/12/13/18 to CONSUMED + failure_policy/reload_config/fail_open parse + mutual-exclusivity + env_vars parse/collision/cap + multi-plugin registry wiring + ~3 NEW PARSE-REJECT arms (per AMEND-C2/C3/C4; R-25.3-2 + R-25.3-3; D-25.3-P2 first-action)

**Files:**
- Modify: `internal/filter/http/wasm/compiled_config.go`
- Modify: `internal/filter/http/wasm/wasm.go` (retire the per-route deferral; route to `parsePerRouteWasm`)
- Test: `internal/filter/http/wasm/compiled_config_test.go`

- [ ] **Step 0 (D-25.3-P2 first-action):** finalize byte-stable wording for the ~3 NEW arms (provisional below) + advance the arm-count. Record the chosen bytes in PROGRESS.md.

- [ ] **Step 1: Write the failing failure_policy mapping + mutual-exclusivity tests (R-25.3-2)**

```go
// internal/filter/http/wasm/compiled_config_test.go (append)
func TestFailurePolicy_Mapping(t *testing.T) {
	// UNSPECIFIED → FAIL_CLOSED; FAIL_RELOAD; FAIL_CLOSED; FAIL_OPEN (AMEND-C3)
	// table over parsed PluginConfig → cfg.failurePolicy
}
func TestFailurePolicy_FailOpenAndFailurePolicyBothSet_Reject(t *testing.T) {
	// fail_open=true AND failure_policy set → PARSE-REJECT (NEW arm B; upstream parity)
}
func TestFailOpen_MapsToFailOpen(t *testing.T) {
	// fail_open=true (alone) → FAIL_OPEN; fail_open=false/unset → UNSPECIFIED → FAIL_CLOSED
}
```

- [ ] **Step 2: Write the failing env_vars collision/cap PARSE-REJECT tests (R-25.3-3; NEW arms A + C)**

```go
// compiled_config_test.go (append)
func TestParse_EnvVarsCollisionReject(t *testing.T) { /* host_env_keys ∩ key_values
  nonempty → PARSE-REJECT arm A (upstream parity) */ }
func TestParse_EnvVarsCapExceededReject(t *testing.T) { /* > 64 entries OR value > 4 KiB
  → PARSE-REJECT arm C (envoy-go-strict) */ }
```

- [ ] **Step 3: Write the failing lifted-arm CONSUMED tests** — assert the 5 previously-deferred inputs (failure_policy=FAIL_RELOAD, reload_config set, fail_open, duplicate vm_id, per-route) NO LONGER return their deferral wording but parse successfully (or route to the registry/per-route path). Update the `compiled_config_test.go` cases at lines ~370-412 that currently assert the deferral wording (Arm09/10/13) to assert CONSUMED behavior; the byte-stable roster test (`TestParseRejectConstants_ByteStable`) keeps the historical constants but the dispatch no longer fires them for these inputs.

- [ ] **Step 4: Run → FAIL** — `go test ./internal/filter/http/wasm/ -run 'FailurePolicy|EnvVars|FailOpen' -v` → FAIL.

- [ ] **Step 5: Implement the parse extensions** in `compiled_config.go`:
  - `FailurePolicy` enum + parse (`UNSPECIFIED→FAIL_CLOSED`, `FAIL_RELOAD`, `FAIL_CLOSED`, `FAIL_OPEN`); `RuntimeError`-gating recorded on the cfg (consumed by the reload hook).
  - `reload_config.backoff.base_interval` parse → `max(op, 100ms)` floor (default 1s).
  - `fail_open` parse + `fail_open`⊕`failure_policy` mutual-exclusivity → NEW arm B PARSE-REJECT.
  - `environment_variables` parse → `wasm.AssembleEnvVars` → collision-reject (NEW arm A) + cap-reject (NEW arm C + `env_vars_cap_exceeded` counter); feed the assembled env to the `*RootVM` factory.
  - multi-plugin: `New()` computes `makeVMKey` + calls `DefaultRegistry.AcquireRootVM(key, factory)`; `Close()` calls `DefaultRegistry.Release(key)`. Duplicate `vm_id` across PluginConfigs now SHARES (no longer arm-12 reject).
  - Retire the per-route deferral in `wasm.go` (route `validatePerRouteWasm` → `parsePerRouteWasm`).
  - Add the 3 NEW byte-stable `parseReject*` constants (provisional wording):
    - `parseRejectEnvVarsKeyCollision = "wasm: config.vm_config.environment_variables: key %q is duplicated across host_env_keys and key_values (all keys must be unique)"`
    - `parseRejectFailOpenAndFailurePolicyBothSet = "wasm: only one of config.fail_open or config.failure_policy can be set"`
    - `parseRejectEnvVarsCapExceeded = "wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"`

- [ ] **Step 6: Run → GREEN** — `go test ./internal/filter/http/wasm/ -run 'FailurePolicy|EnvVars|FailOpen|ParseReject' -race -v` → PASS.

- [ ] **Step 7: Extend `TestParseRejectConstants_ByteStable`** with the 3 NEW arms + bump the arm-count assertion (D-25.3-P2). Run → GREEN.

- [ ] **Step 8: Gates + commit**
```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./internal/filter/http/wasm/...
git add internal/filter/http/wasm/compiled_config.go internal/filter/http/wasm/wasm.go internal/filter/http/wasm/compiled_config_test.go
git commit -m "phase-25.3 Task 7: compiled_config.go — lift 5 deferred arms; failure_policy/reload/fail_open + env_vars parse + registry wiring + 3 NEW arms (AMEND-C2/3/4; R-25.3-2/3; D-25.3-P2)"
```

---

## Task 8: `stats.go` EXTEND — 4 NEW counters (vm_reload triplet + env_vars_cap_exceeded); project stat surface 128 → 132 (per SPEC §7; SPEC §15 items 9-10)

**Files:**
- Modify: `internal/filter/http/wasm/stats.go`
- Modify: `internal/filter/http/wasm/stats_test.go`

- [ ] **Step 1: Write the failing 4-NEW-counter + 132-total test**

```go
// stats_test.go (append) — assert the 4 NEW counters exist + wire + the project count.
func TestStats_VmReloadTripletAndEnvVarsCap(t *testing.T) {
	s := newFilterStats(/* scope */)
	s.VmReloadSuccessInc(); s.VmReloadRuntimeFailureInc(); s.VmReloadBackoffInc(); s.EnvVarsCapExceededInc()
	// assert each counter == 1 via the scope's snapshot
}
```

(The single source-of-truth stat-count assertion lives in `internal/filter/http/wasm/wasm_test.go` around lines 400-441 — the `119 → 128` project-total commentary + assertion. Update its expected from 128 to 132. Confirm via `grep -rn "128" --include=*_test.go` at IMPL.)

- [ ] **Step 2: Run → FAIL** (`undefined: VmReloadSuccessInc`). `go test ./internal/filter/http/wasm/ -run TestStats_VmReload -v`

- [ ] **Step 3: Implement the 4 counters** in `stats.go`: add stat-name constants (`vm_reload_success`, `vm_reload_runtime_failure`, `vm_reload_backoff`, `env_vars_cap_exceeded`), the `*stats.Counter` fields on `filterStats`, the constructor wiring, and the nil-tolerant `Inc` wrapper methods (per ADR-0085). Wire the triplet into the `RootStatsRecorder` so the reload state machine (Task 3) increments them; wire `env_vars_cap_exceeded` into the Task 7 cap-reject path.

- [ ] **Step 4: Run → GREEN** — `go test ./internal/filter/http/wasm/ -run TestStats -race -v` → PASS.

- [ ] **Step 5: Gates + commit**
```bash
go build ./... && go vet ./... && go test -race -short ./internal/filter/http/wasm/...
git add internal/filter/http/wasm/stats.go internal/filter/http/wasm/stats_test.go
git commit -m "phase-25.3 Task 8: stats.go — vm_reload triplet + env_vars_cap_exceeded (128 -> 132)"
```

---

## Task 9: filter dispatch EXTEND — per-route resolution at `DecodeHeaders` entry + reload-on-RuntimeError integration (SPEC §15 items 5 + 2)

**Files:**
- Modify: `internal/filter/http/wasm/decode_headers.go` (+ the filter struct / `encode_headers.go` as needed)
- Modify: `internal/filter/http/wasm/dispatch_test.go`

- [ ] **Step 1: Write the failing per-route end-to-end + reload-dispatch tests**

```go
// dispatch_test.go (append)
func TestDispatch_PerRouteOverrideApplies(t *testing.T) { /* a stream on a route with a
  per-route Wasm override dispatches against the override *RootVM, not the listener one */ }
func TestDispatch_ReloadOnRuntimeError_FailClosedThenRecover(t *testing.T) { /* a guest
  that traps (RuntimeError) under FAIL_RELOAD: first stream → 503 (FAIL_CLOSED within
  backoff) + vm_reload_backoff/runtime_failure counters; after fake-clock past backoff
  → reload → vm_reload_success + subsequent stream succeeds */ }
func TestDispatch_FailOpenBypass(t *testing.T) { /* FAIL_OPEN guest failure → filter
  bypass (stream proceeds) */ }
```

- [ ] **Step 2: Run → FAIL.** `go test ./internal/filter/http/wasm/ -run TestDispatch_PerRoute -v`

- [ ] **Step 3: Implement the dispatch integration** — at `DecodeHeaders` entry, resolve the effective `*compiledConfig` via `resolvePerRoute(routeCfg, listenerCfg)` (Task 6) and create the stream context against its `*RootVM`; thread the reload-decision hook (Task 3) into the dispatch path so a guest `RuntimeError` under `FAIL_RELOAD` records `Failed` + the next entry drives reload-or-backoff; FAIL_CLOSED → 503 via `SendLocalReply`, FAIL_OPEN → bypass.

- [ ] **Step 4: Run → GREEN under -race** — `go test ./internal/filter/http/wasm/ -run TestDispatch -race -v` → PASS.

- [ ] **Step 5: Gates + commit**
```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
git add internal/filter/http/wasm/decode_headers.go internal/filter/http/wasm/encode_headers.go internal/filter/http/wasm/dispatch_test.go
git commit -m "phase-25.3 Task 9: per-route resolution at DecodeHeaders + reload-on-RuntimeError dispatch integration"
```

---

## Tier D — fuzzer FOLD + differential fixtures (Tasks 10-12)

## Task 10: `FuzzWasmConfigParse` seed-corpus EXTENSION (FOLD; no 36th fuzzer; per D-25.3-6; SPEC §15 item 14)

**Files:**
- Modify: `internal/filter/http/wasm/fuzz_test.go` (where `FuzzWasmConfigParse` lives — confirm via `grep -rn FuzzWasmConfigParse`)
- Add: `internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/` seeds

- [ ] **Step 1: Add ~15-30 NEW seed corpus entries** exercising the 25.3 parse surface: per-route wholesale `Wasm` configs; `failure_policy` ∈ {each enum}; `reload_config.backoff.base_interval` ∈ {0, sub-floor, valid}; `fail_open`+`failure_policy` both-set; `environment_variables` {collision, cap-exceeded entries, cap-exceeded value, valid}; duplicate `vm_id`. Add as `f.Add(...)` calls + corpus files.

- [ ] **Step 2: Run the fuzzer briefly to confirm must-never-panic** — `go test ./internal/filter/http/wasm/ -run FuzzWasmConfigParse -fuzz FuzzWasmConfigParse -fuzztime 30s` → no crash; then `go test ./internal/filter/http/wasm/ -run FuzzWasmConfigParse` (seed-only) → PASS.

- [ ] **Step 3: Confirm 35 fuzzers UNCHANGED** — `grep -rn "func Fuzz" --include=*_test.go | wc -l` → 35 (no 36th added).

- [ ] **Step 4: Gates + commit**
```bash
go build ./... && go vet ./... && go test -short ./internal/filter/http/wasm/...
git add internal/filter/http/wasm/fuzz_config_test.go internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/
git commit -m "phase-25.3 Task 10: FOLD per-route+reload+env_vars into FuzzWasmConfigParse seed corpus (D-25.3-6; 35 fuzzers unchanged)"
```

---

## Task 11: differential fixture `0038-http-wasm-perroute-and-multi-plugin` (cross-side + subject-side; per SPEC §8.1; R-25.3 fixture deliverable; SPEC §15 item 12)

**Files:**
- Create: `test/fixtures/0038-http-wasm-perroute-and-multi-plugin/` (README.md, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go, sources/*, bytecode/*.wasm)
- Modify: `test/differential/fixture/fixture.go` + `test/differential/runner_test.go`

- [ ] **Step 1: Author the Rust plugin sources + build + vendor the `.wasm` blobs** — 2-4 plugins (e.g. `perroute_dispatch` + `multi_plugin_shared_data` + a `fail_reload_trap` plugin that panics on a trigger header); `cargo build --release --target wasm32-wasip1`; copy to `bytecode/`. Pin proxy-wasm-rust-sdk =0.2.4.

- [ ] **Step 2: Author the bootstraps + driver** — single listener + single HCM (wasm filter alphabetical position) + router; two `PluginConfig` entries sharing one `vm_id` (refcount=2; one Module); per-route TPFC override on one route. Scenario partition (~6-10; final at IMPL):
  - **Per-route (cross-side `CompareBytes`):** per-route override applies; per-route disabled; listener-default on a no-TPFC route.
  - **Multi-plugin (cross-side + subject `StatsAsserter`):** two PluginConfigs same vm_id share one VM; distinct plugin contexts read/write the SAME shared-data namespace (vm_id scope per AMEND-C2).
  - **Reload (subject-only `StatsAsserter`; fake-clock or trigger-driven):** FAIL_RELOAD guest panics → assert `vm_reload_runtime_failure`/`vm_reload_backoff`/`vm_reload_success` progression.

- [ ] **Step 3: Write the runner wiring** — BackendKind enum value (or REUSE) + blank import + switch-case.

- [ ] **Step 4: Run the fixture** — `go test ./test/differential/... -run 0038 -v` → PASS.

- [ ] **Step 5: Deliberate-break liveness for EVERY subject-side `StatsAsserter` arm** (mandatory per `reference_differential_asserter_dispatch` — bit phase-23 fixture-0030): for each StatsAsserter arm, temporarily break the asserted stat → re-run → confirm FAIL → restore → confirm PASS. Record the cycle in PROGRESS.md.

- [ ] **Step 6: Gates + commit**
```bash
go build ./... && go vet ./... && go test ./test/differential/... -run 0038
git add test/fixtures/0038-http-wasm-perroute-and-multi-plugin/ test/differential/
git commit -m "phase-25.3 Task 11: differential fixture 0038 (per-route + multi-plugin + reload) + deliberate-break liveness"
```

---

## Task 12: differential fixture `0039-http-wasm-perroute-boot-reject` (boot-reject; per SPEC §8.2; D-25.3-P1 first-action; SPEC §15 item 13)

**Files:**
- Create: `test/fixtures/0039-http-wasm-perroute-boot-reject/` (README.md, envoy.yaml, envoy-go.yaml, inputs/driver.go)
- Modify: `test/differential/runner_test.go`

- [ ] **Step 0 (D-25.3-P1 first-action):** empirical-scrape reference Envoy v1.37.2 boot stderr for the candidate arms — env_vars cap-exceeded (subject-only; reference drops the envoy-go-strict field) vs env_vars key-collision (cross-side; both reject). Choose arm + substring + runner-branch (subject-only `SubjectOnlyBootRejectFixture` vs cross-side boot-reject) per the one-dir-one-branch constraint. Record in PROGRESS.md. (The "dangling vm_id" arm is RETIRED per AMEND-C1.)

- [ ] **Step 1: Author the bootstraps + driver** — single-arm boot-reject matching the chosen branch (anticipated: env_vars cap-exceeded → subject-only, mirroring the 25.2 fixture-0037 `SubjectOnlyBootRejectFixture` shape). Reuse a minimal valid `.wasm` from fixture-0038's bytecode.

- [ ] **Step 2: Run the fixture** — `go test ./test/differential/... -run 0039 -v` → PASS (subject boot-rejects with the chosen substring; reference boots [subject-only] or also rejects [cross-side]).

- [ ] **Step 3: Confirm fixture-dir count 39 → 41** — `ls test/fixtures/ | grep -cE '^00(3[89])'` and total dir count.

- [ ] **Step 4: Gates + commit**
```bash
go build ./... && go vet ./... && go test ./test/differential/... -run '0038|0039'
git add test/fixtures/0039-http-wasm-perroute-boot-reject/ test/differential/
git commit -m "phase-25.3 Task 12: differential fixture 0039 (boot-reject) + D-25.3-P1 closure (39 -> 41 dirs)"
```

---

## Tier E — `test/conformance/proxy-wasm/` harness seed (Tasks 13-14)

## Task 13: conformance harness scaffold + driver shape (per Q7 + AMEND-C5 + ADR-0212; D-25.3-P4 RESOLVED; SPEC §15 item 15)

**Files:**
- Create: `test/conformance/proxy-wasm/conformance.go`, `conformance_test.go`, `README.md`

- [ ] **Step 1: Write the harness helpers** (`conformance.go`) per D-25.3-P4 — load a vendored `.wasm` into a fresh `wasm.NewRootVM` with full host-module wiring; per-family assertion utilities; a `loadFamilyWasm(name)` helper reading `bytecode/<name>.wasm`.

- [ ] **Step 2: Write the family iterator** (`conformance_test.go`) — `TestProxyWasmConformance` ranges over the 10 ported family names, each a `t.Run(name, ...)` delegating to the family sub-test. Initially the family list is empty or stubbed → the parent test PASSES vacuously (Task 14 fills the families + proves each live).

- [ ] **Step 3: Author the README** — the 16-file roster + the 10-port/6-defer rationale (AMEND-C5) + the reproduction/vendoring discipline (D-25.3-P4) + the deliberate-break-liveness requirement + the 6-deferred-family forward-pointer note (cross-ref BOOTSTRAP §7.5).

- [ ] **Step 4: Run → GREEN (vacuous)** — `go test ./test/conformance/proxy-wasm/... -v` → PASS.

- [ ] **Step 5: Gates + commit**
```bash
go build ./... && go vet ./... && go test ./test/conformance/proxy-wasm/...
git add test/conformance/proxy-wasm/
git commit -m "phase-25.3 Task 13: conformance harness scaffold + driver shape (ADR-0212; D-25.3-P4)"
```

---

## Task 14: port the 10 conformance families + deliberate-break liveness per family (per AMEND-C5; R-25.3-6; SPEC §15 items 15 + 21)

**Files:**
- Create: `test/conformance/proxy-wasm/families/<family>/<family>_test.go` × 10
- Create: `test/conformance/proxy-wasm/bytecode/<family>.wasm` (vendored) + `sources/<family>/` × as needed

The 10 families (per AMEND-C5 / SPEC §11.5): logging, stop_iteration (header-maps + continue/stop), shared_data (CAS), endianness (ABI value/buffer marshalling), exports (env/clock/random WASI), security (hostcall restriction: `proxy_log` allowed / `proxy_done` gated), runtime (traps + mem-limit + host↔VM callback), wasm_vm (VM init/memory), bytecode_util (custom-section + ABI-version parse), pairs_util (header-map pairs wire format).

For EACH family (repeat the bite-sized cycle):
- [ ] **Step A: vendor/build the family's `.wasm`** (from `sources/<family>/`; offline build; commit the blob).
- [ ] **Step B: write the family sub-test** asserting the host-observable behavior the cpp-host `<family>_test.cc` exercises (re-expressed against the envoy-go `*RootVM`).
- [ ] **Step C: register the family** in the Task 13 iterator list.
- [ ] **Step D: run → GREEN** — `go test ./test/conformance/proxy-wasm/... -run 'TestProxyWasmConformance/<family>' -v` → PASS.
- [ ] **Step E: deliberate-break liveness** — break the assertion → expect FAIL → restore → GREEN. Record in PROGRESS.md.

- [ ] **Final step: run ALL 10 + gate** — `go test ./test/conformance/proxy-wasm/... -v` → 10/10 PASS. Commit (one commit per family OR one batched commit — implementer's choice; prefer per-family for reviewability).
```bash
git add test/conformance/proxy-wasm/
git commit -m "phase-25.3 Task 14: port 10 conformance families + deliberate-break liveness (AMEND-C5; R-25.3-6; 10/10 green)"
```

---

## Tier F — atomic landing (Task 15)

## Task 15: R8 benchmarks + ADR bodies + BEHAVIOR_CONTRACT bundle + STATE/ROADMAP parent-row-25 ROLLUP closure (per SPEC §10 + §13 + §15 items 22-32)

**Files:**
- Modify: `internal/filter/http/wasm/wasm_bench_test.go`
- Modify: `docs/envoy-go/DECISIONS.md`, `BEHAVIOR_CONTRACT.md`, `STATE.md`, `ROADMAP.md`, `BOOTSTRAP_PROMPT.md`, `ENVOY_TARGET.md`
- Create: `docs/envoy-go/phases/25.3-.../PROGRESS.md`, `REVIEW.md`

- [ ] **Step 1: R8 benchmarks (per Q6 + D-25.3-5; SPEC §15 item 28)** — re-run `BenchmarkPerStreamModule_Instantiation` + add `BenchmarkPerStreamPluginContextLookup` + `BenchmarkPerRouteResolve`; SAME 1ms per-stream-cost threshold. Record the R8 disposition in PROGRESS.md (anticipated STANDS WEAK-default; ADR-0209 + ADR-0213 reserves STAY UNCONSUMED). If any benchmark exceeds 1ms, fire the ADR-0209/0213 escape-valve (§Context + §Decision + §Consequences in the same commit per ADR-0044).
```bash
go test ./internal/filter/http/wasm/ -run '^$' -bench 'PerStream|PerRoute' -benchmem
```

- [ ] **Step 2: Land the 3 ADR §Decision+§Consequences bodies** (DECISIONS.md; the §Context drafts already anchored at the SPEC commit): ADR-0210 (per-route 5th-canonical REUSE-by-absence; ADR-0125 STAYS 10); ADR-0211 (multi-plugin registry + reload state machine + env_vars BUNDLED); ADR-0212 (conformance harness seed + 10-of-16 port). Plus: ADR-0205 §Consequences one-line in-place AMEND (registry-keyed refinement) + AMENDED timestamp in §Status; ADR-0186 §Consequences RATIFIES clause (the Q5/F1 consumer-real-migration completed); ADR-0202 §Consequences UNCHANGED. Record the ADR-0209 + ADR-0213 reserve disposition (STANDS-UNCONSUMED anticipated). Next-free STAYS ADR-0213.

- [ ] **Step 3: BEHAVIOR_CONTRACT.md ~6-8-edit bundle** (per SPEC §13.3): EXTEND `### envoy.filters.http.wasm` with the 25.3 EXTENSION block (per-route + multi-plugin + reload + env_vars semantics); stat-table 128 → 132; 2 NEW envoy-go-strict departure records (reload-floor `max(op,100ms)` + env_vars-cap 64/4-KiB); RESOLVE/RENAME `### Phase 25.2 forward-pointer notes` → `### Phase 25.3 forward-pointer notes` (or remove if all close); ADD the 6-deferred-conformance-families forward-pointer roster. Pin the D-25.3-P2 byte-stable wording here.

- [ ] **Step 4: BOOTSTRAP_PROMPT.md §7.5 + ENVOY_TARGET.md** — document the 6 deferred conformance families (shared_queue, signature_util, wasm[TLS-cache], vm_id_handle, null_vm, fuzz) as forward-pointers per AMEND-A8 + AMEND-C5.

- [ ] **Step 5: PROGRESS.md + REVIEW.md** — author per the 25.2 precedent (Task-by-Task progress log incl. all D-closures [P1/P2/P3/P4] + the R8 disposition + the deliberate-break liveness records; REVIEW.md retrospective + any carried debts).

- [ ] **Step 6: STATE.md re-advance + ROADMAP ROLLUP closure (SPEC §15 items 25-26)** — STATE.md → `phase 25.3 IMPL done; §9 HTTP-filters family CLOSED`; lifecycle-state → next family/phase per BOOTSTRAP §5; ROADMAP sub-row 25.3 `in-progress → done` + parent row 25 `in-progress → done` ATOMICALLY in this final commit (both lifecycle annotations in the commit-message body for grep-verifiability per ADR-0106).

- [ ] **Step 7: FULL six-gate run** — `go build ./... && go vet ./... && golangci-lint run && go test -race -short ./... && (differential 41/41) && (35 fuzzers seed) && (h2spec 53/53) && go test ./test/conformance/proxy-wasm/... (10/10)`. ALL GREEN. Record evidence in PROGRESS.md per `superpowers:verification-before-completion`.

- [ ] **Step 8: Commit (atomic landing)**
```bash
git add -A
git commit -m "phase-25.3 Task 15: ADR-0210/0211/0212 bodies + BEHAVIOR_CONTRACT bundle + R8 benchmarks; STATE/ROADMAP ROLLUP — parent row 25 done; §9 HTTP-filters family CLOSED"
```

---

## ADR landing summary (per SPEC §10)

| ADR | Disposition at 25.3 IMPL | Task |
|---|---|---|
| ADR-0210 | §Decision+§Consequences — per-route 5th-canonical REUSE-by-absence; ADR-0125 STAYS 10 | 15 (anchored at 6) |
| ADR-0211 | §Decision+§Consequences — multi-plugin registry + reload + env_vars BUNDLED | 15 (anchored at 1-4/7) |
| ADR-0212 | §Decision+§Consequences — conformance harness seed; 10-of-16 port | 15 (anchored at 13-14) |
| ADR-0205 | §Consequences one-line in-place AMEND (registry-keyed refinement) + AMENDED timestamp | 15 |
| ADR-0186 | §Consequences RATIFIES clause (Q5/F1 consumer-real-migration completed) | 15 (executed at 5) |
| ADR-0202 | UNCHANGED (API-REVISION ALLOWANCE stays scoped to consumer #2) | — |
| ADR-0209 + ADR-0213 | reserves — STAND UNCONSUMED (anticipated; R8 WEAK-default) | 15 |
| ADR-0125 | 0 amendments (RETIRED per AMEND-A3 + C1) | — |

**ZERO ADRs consumed at PLAN.** Next-free STAYS `ADR-0213` through PLAN; the 3 §Context drafts (0210/0211/0212) anchored at the SPEC commit get their bodies at Task 15 (IMPL).

## SPEC §15 acceptance-checklist → Task cross-map

| §15 items | Task(s) |
|---|---|
| 1 (registry.go) | 1 |
| 4 (root_vm shared-data + reload-state + env feed) + 19 (R-25.3-4) | 1, 2 |
| 2 (reload.go) + 17 (R-25.3-2) + 20 (R-25.3-5) | 3 |
| 3 (env_vars assembly) + 18 (R-25.3-3) | 4 |
| 7 (phase-21 clock MIGRATION) | 5 |
| 5 (perroute.go) | 6, 9 |
| 8 (compiled_config extend + 3 NEW arms) + 16 (R-25.3-1 registry key) | 1, 7 |
| 9-11 (stats 128→132 + departures) | 8, 15 |
| 6 (registration UNCHANGED) + 27 (boot UNCHANGED) | (invariant; verified at 7/9/15) |
| 12 (fixture 0038) + 14 (fuzzer FOLD) | 10, 11 |
| 13 (fixture 0039) + 29 (D-25.3-P1) | 12 |
| 15 (conformance) + 21 (R-25.3-6) | 13, 14 |
| 22-24 (ADR landings) | 15 |
| 25-26 (STATE/ROADMAP ROLLUP) | 15 |
| 28 (R8 benchmarks) | 15 |
| 30 (D-25.3-P2) | 7, 15 |
| 31 (D-25.3-P3) + 32 (D-25.3-P4) | 3, 13 (RESOLVED in this PLAN) |

---

## Plan review loop

After this PLAN is authored, dispatch a single `plan-document-reviewer` subagent with the PLAN path + the SPEC path (NOT this session's history). If ❌ issues: fix + re-dispatch. If ✅: advance STATE.md to `phase 25.3 PLAN done; awaiting 25.3 IMPL` (next-skill `superpowers:subagent-driven-development` per `feedback_execution_style.md`), add the ROADMAP row-25.3 PLAN-anchored annotation (row STAYS `in-progress`), commit PLAN.md + STATE.md + ROADMAP.md, squash-merge to master, push to origin, clean up the worktree.

**End of phase 25.3 PLAN.**
