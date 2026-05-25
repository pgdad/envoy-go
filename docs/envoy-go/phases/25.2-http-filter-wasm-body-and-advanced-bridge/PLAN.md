# Phase 25.2 — HTTP filter `envoy.filters.http.wasm` (full advanced-bridge surface delta — body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full property + NEW `internal/filterstate/` + NEW `internal/stats/dynamic/` + phase-22.2 lua MIGRATES) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the full Envoy↔WASM advanced-bridge surface delta of `envoy.filters.http.wasm` on top of the 25.1 headers-only foundation — the SECOND-of-3 sub-phases of parent BRAINSTORM Q1 envelope D — by evolving the existing `internal/wasm/` framework primitive (RETIRE 25.1 per-stream `*VM`; introduce ONE long-lived `*RootVM` per `*compiledConfig` + per-stream `*StreamContext` children sharing wazero Runtime+Module per Q3 + ADR-0205; EXTEND `ABICallbacks` interface from 13 to 20 methods per ADR-0206; EXTEND `SandboxConfig` capability key roster from 37 to 58 keys per AMEND-B5 with gate-at-registration discipline; ADD per-`*RootVM` tick goroutine with 10ms envoy-go-strict period floor + ADR-0186 `Clock` seam FIRST co-consumer beyond phase-21 per Q5 + R-25.2-9; ADD per-`*RootVM` shared-data CAS-protected K-V map with envoy-go-strict caps per Q6 + R-25.2-10; ADD per-`*RootVM` httpCall dispatch with cancel-at-destruction + `http_call_response_after_close` defensive counter per AMEND-B3 + R-25.2-3; ADD `internal/wasm/foreign.go` `ForeignFunctionRegistry` with EMPTY default registry per AMEND-A9 + R-25.2-8 + D-25.2-P3 PLAN-time closure (mutex-per-RootVM dispatch concurrency model); ADD `internal/wasm/property.go` full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4 + NUL-delimited path parsing; ADD `internal/wasm/dynamic_stats.go` per-plugin `*dynamic.Registry` wiring with signed-i64 delta + unsigned-u64 value per AMEND-B2 + R-25.2-2; ADD per-family ABI dispatch files at `internal/wasm/abi/` (`body_bridge.go` with buffer-clamp wire-contract per AMEND-B1 + R-25.2-1 + `stream_control.go` + `timer.go` + `metrics.go` + `shared_data.go` + `http_call.go` + `foreign.go`); ADR-0205 + ADR-0206 §Decision + §Consequences bodies at IMPL atomic landing) + the NEW `internal/filterstate/` framework primitive at second-consumer scope per Q7 + ADR-0207 + R-25.2-6 (consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking — the `:filterState()` Lua surface stays UNCHANGED; consumer #2 = 25.2 wasm `filter_state.*` + `upstream_filter_state.*` property branches; `upstream_filter_state` is a DISTINCT root co-equal to `filter_state` per AMEND-B4 SUBSTANTIVE REFINEMENT) + the NEW `internal/stats/dynamic/` infrastructure subpackage per Q9 + ADR-0208 + R-25.2-7 (per-plugin `*Registry` scoping the `wasmcustom.<custom_name>` dynamic-stats namespace byte-faithful to upstream per AMEND-B2 — namespace is `wasmcustom.<custom_name>` ONLY without plugin prefix; per-plugin isolation via per-plugin Registry SCOPE) + the EXTENSIONS to `internal/filter/http/wasm/` per ADR-0208 (`compiled_config.go` gains 4 envoy-go-strict-only `PluginConfig` config fields + 6 NEW PARSE-REJECT arms per §6 + `*RootVM` construction at `New` time replacing per-stream `wasm.NewVM`; `abi_callbacks.go` gains 7 NEW methods + 4 RE-USE primitive consumers (ADR-0144 + ADR-0177 + ADR-0190 + NEW ADR-0207); NEW `body.go` + `trailers.go` + `tick_clock.go` + `property.go`; `stats.go` gains 9 NEW envoy-go-strict counters per Q9 + AMEND-B3 — project stat count 119 → 128; `decode_headers.go` + `encode_headers.go` extend to construct per-stream context via `*RootVM.NewStreamContext`) + the 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + R-25.2-12 + D-25.2-P4 PLAN-time corpus seed roster closure + the differential fixture `0036-http-wasm-body-and-advanced` (single-listener mixed-mode per Q8 + ADR-0192 precedent; 14 scenarios partitioned by assertion-class — 10 deterministic cross-side via `CompareBytes` + 4 non-deterministic subject-only via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`; httpCall scenarios use a SECOND upstream cluster definition NOT a second listener per phase-22.2 REVIEW §7.4 `freeTCPPort` flake mitigation; every subject-only StatsAsserter arm gets a deliberate-break liveness verification per `reference_differential_asserter_dispatch` + 25.1 fixture-0030 lesson) + the differential fixture `0037-http-wasm-body-and-advanced-boot-reject` (subject-only single-arm boot-reject per D-25.2-P1 IMPL-time closure; anticipated arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`; reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field — silent drop; per `reference_differential_fixture_dispatch_constraint` one-fixture-dir-equals-one-runner-branch — runner-branch shape settles at IMPL via extending `BootRejectFixture` with `subjectOnly: true` flag OR introducing `SubjectOnlyBootRejectFixture` runner branch) + the 6-edit ~150-250 LoC BEHAVIOR_CONTRACT.md ~7-edit bundle per §13.4 + ADR-0052 atomic landing + the ADR-0205+0206+0207+0208 §Decision+§Consequences body landings + the ADR-0202 §Consequences one-line in-place AMEND acknowledgment paragraph per §10.2 + ADR-0044 in-place edit discipline + the STATE.md re-advance + ROADMAP row 25.2 `in-progress → done` per ADR-0106 per-cell IMPL-done annotation — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on the 10 fixture-0036 deterministic cross-side scenarios + subject-only StatsAsserter assertions GREEN on the 4 non-deterministic scenarios + subject-only boot-reject GREEN on fixture-0037, modulo the ~27 envoy-go-strict documented divergence-windows (21 inherited from 25.1 + ~6 NEW at 25.2: 9-counter consolidated bundle + body-buffer cap + shared-data cap + tick period 10ms floor + foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace clarification per §9). **Sub-phase landing (`25.2` ROADMAP row) per parent SPEC §3.1 + BRAINSTORM Q1 3-way PRE-SPLIT discipline** — the 25.2 PLAN closes ROADMAP row `25.2` only at phase-done (Task 22 atomic landing); parent row `25` STAYS `in-progress` until 25.3 IMPL phase-done (sub-row rollup discipline per ADR-0106 + phase-18.1/18.2 + phase-19.1/19.2 + phase-22.1/22.2/22.3 + phase-24.1/24.2 + phase-25.1 precedent). 25.3 (per-route 5th-canonical wholesale-override REUSE-by-absence per AMEND-A3 + multi-plugin VM-sharing via duplicate `vm_id` + `VmConfig.environment_variables` activation + `failure_policy = FAIL_RELOAD` + conformance harness seed at 62.5% pass-threshold per AMEND-A8) is OUT OF SCOPE for 25.2.

**Architecture:** The 25.2 IMPL evolves the existing `internal/wasm/` framework primitive (3 NEW production files in package root — `root_vm.go` + `stream_context.go` + `tick.go` + `shared_data.go` + `property.go` + `dynamic_stats.go` + `foreign.go` + `http_call.go` — wait, that's 8 NEW; the existing `vm.go` 25.1 per-stream `*VM` file is DELETED at Task 1 per D-P-PLAN-6 in favor of `root_vm.go` + `stream_context.go`; `sandbox.go` + `registration.go` are EXTENDED in-place; `abi/types.go` is UNCHANGED at 25.2 — the existing enums cover the 25.2 surface, with WasmBufferType values 0/1/4 + WasmHeaderMapType values 1/3 ACTIVATED via host-module registration extensions; NEW `abi/body_bridge.go` + `abi/stream_control.go` + `abi/timer.go` + `abi/metrics.go` + `abi/shared_data.go` + `abi/http_call.go` + `abi/foreign.go` per-family ABI dispatch files) + introduces TWO NEW packages (`internal/filterstate/` framework primitive — `filterstate.go` with `Bucket` + `FilterStateObject` + `StateType` + sync semantics per ADR-0207; `internal/stats/dynamic/` infrastructure subpackage — `dynamic.go` with `Registry` + `MetricID` + `MetricType` + per-plugin scope per ADR-0208 + AMEND-B2) + REWRITES phase-22.2's `internal/filter/http/lua/filterstate.go` non-breaking to delegate to `internal/filterstate/*Bucket` per ADR-0207 MIGRATION (the `:filterState()` Lua surface stays byte-identical; 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 carry forward unchanged) + extends the existing `internal/filter/http/wasm/` package (`compiled_config.go` + `abi_callbacks.go` + `stats.go` + `decode_headers.go` + `encode_headers.go` EXTENDED in-place; NEW `body.go` + `trailers.go` + `tick_clock.go` + `property.go`) + extends ZERO files at the boot-registration `cmd/envoy-go/main.go` (per §3.7 — 20 HTTP filters wired UNCHANGED at 25.2; the existing `httpReg.Register(wasm.TypeURL, wasm.New)` call serves the 25.2 surface via `wasm.New` returning the EXTENDED filter factory) + adds 1 enum value `BackendKind=HTTPWasmAdvanced` (OR REUSES `HTTPWasm` — settles at IMPL Task 20) at `test/differential/fixture/fixture.go` + adds 2 new fixture directories (`test/fixtures/0036-http-wasm-body-and-advanced/` with `README.md` + `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `inputs/driver.go` + `scripts/` with 14 per-scenario Rust sources + `bytecode/` with 14 vendored `.wasm` blobs per Q8 + inherited Q9 + AMEND-A1 discipline; `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/` with `README.md` + `envoy.yaml` + `envoy-go.yaml` + `inputs/driver.go` implementing the subject-only boot-reject `BootRejectFixture` variant per D-25.2-P1). The 25.2 root VM lifecycle architecture per Q3 + ADR-0205: ONE long-lived `*wasm.RootVM` per `*compiledConfig` (upstream-byte-faithful per proxy-wasm-cpp-host's `Wasm`/`Plugin` model) constructed at `New()` time + `Configure(vmConfigBytes, pluginConfigBytes)` invokes `_initialize` OR `_start` + `proxy_on_vm_start` + `proxy_on_configure` on the root context ONCE at config-load; persists for plugin lifetime owning the tick goroutine + tick state + shared-data map + httpCall response routing + call_id allocation + foreign-function registry view + per-plugin dynamic-stats `*Registry`. Per-stream contexts are CHILDREN of the root VM sharing the same wazero Runtime + Module: each `DecodeHeaders` creates a child stream-context ID via `cfg.rootVM.NewStreamContext(ctx)` which allocates a monotonically-increasing `streamCtxID` + invokes `proxy_on_context_create(streamCtxID, rootCtxID)`; `OnDestroy` fires `proxy_on_done(streamCtxID) → bool` + `proxy_on_log(streamCtxID)` + `proxy_on_delete(streamCtxID)` + cancels any outstanding httpCalls dispatched from this stream per AMEND-B3 cancel-at-destruction discipline. The 25.1 per-stream `*VM` (each stream constructing a fresh `*wazero.Runtime` at 61µs/stream per the 25.1 Task 17 `BenchmarkPerStreamVM_Construction_Headers` measurement) is RETIRED at 25.2; per-stream `*StreamContext` creation becomes microseconds (just `proxy_on_context_create` + bookkeeping; no wazero Runtime construction). The per-stream Module instantiation pattern (fresh-per-stream WEAK-default vs pooled vs shared-Module-with-mutex-serialization) carries forward to 25.2 IMPL R8 escape-valve at Task 22 — if `BenchmarkPerStreamModule_Instantiation > 1ms` per D-25.2-P2 + parent §13-R8 threshold, ADR-0209 escape-valve fires (§Context + §Decision + §Consequences body all land at the same Task 22 commit per ADR-0044). The 25.2 ABI extensions per ADR-0206: 14 NEW env-namespace hostcalls (3 body/buffer per AMEND-B1 clamp-on-overflow + 2 stream-control + 1 timer per Q5 + 4 metrics per AMEND-B2 signed-i64 delta + 2 shared-data per Q6 + 1 outbound HTTP per Q4 + AMEND-B3 + 1 foreign-function per AMEND-A9) + 7 NEW guest-export callbacks (`proxy_on_request_body` + `proxy_on_response_body` + `proxy_on_request_trailers` + `proxy_on_response_trailers` + `proxy_on_tick` + `proxy_on_http_call_response` + `proxy_on_foreign_function`); the 25.1 13-method `ABICallbacks` interface grows to 20 methods; the 25.1 37-key `SandboxConfig` capability roster grows to 58 keys per AMEND-B5 with gate-at-`registerCallback`-time discipline (the env-namespace hostcalls are gated at host-module wiring per upstream `wasm.cc:176-189` `_REGISTER_PROXY` macro — denied capabilities → NOT registered on wazero Runtime; the guest's import resolution fails at module-instantiation OR runtime trap fires on call; the 25.1 default-deny posture per AMEND-A5 INHERITS unchanged). The `proxy_get_buffer_bytes` host shim clamps silently on `start + max_size > buffer.length` (returns `WasmResult::Ok` with truncated length) byte-faithful to cpp-host `src/exports.cc:get_buffer_bytes` per AMEND-B1 — only the `start + max_size` i32-overflow path returns `BadArgument`. The metric hostcalls follow AMEND-B2: `proxy_increment_metric(metric_id, delta)` delta is SIGNED `int64` (allows negative gauge deltas); `proxy_record_metric(metric_id, value)` value is UNSIGNED `uint64`; MetricType enum CONFIRMED as Counter=0/Gauge=1/Histogram=2; dynamic-stats namespace is `wasmcustom.<custom_name>` ONLY (NO plugin prefix as BRAINSTORM Q9 hypothesized — per-plugin isolation via per-plugin Registry SCOPE). The `proxy_http_call` 10-arg hostcall dispatches via the per-`*RootVM` `*httpclient.Client` (RE-CONSUMES phase-20 ADR-0177 at third-or-later co-consumer; CLOSES parent SPEC §13-R6) + allocates a monotonic `call_id` + tracks the originating `streamCtxID` in the per-`*RootVM` `httpCalls` map; response arrival routes to `proxy_on_http_call_response(streamCtxID, call_id, ...)` via the originating stream's `*StreamContext`; the `*StreamContext.Close` cancels any outstanding httpCalls dispatched from this stream per AMEND-B3 cancel-at-destruction discipline; the defensive `http_call_response_after_close` envoy-go-strict counter increments on the rare race where a stray response slips through the cancellation. The `proxy_call_foreign_function` host shim looks up the function name in the per-`*RootVM` foreign-function registry view (which points at the process-global `wasm.DefaultForeignFunctionRegistry` by default per AMEND-A9 — operators register at boot via `wasm.RegisterForeignFunction(name, fn)`); unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream cpp-host `src/exports.cc:147-184`. The foreign-function dispatch concurrency model per D-25.2-P3 PLAN-time closure: mutex-per-RootVM — the `*RootVM` holds its own dispatch lock during foreign-function invocation; the `*ForeignFunctionRegistry` uses `sync.RWMutex` on the registry map (RLock on `Get`); the dispatched function executes synchronously inside `*RootVM.CallForeignFunction` (no goroutine offload at 25.2; the function's compute cost is the operator's responsibility); panic-recovery wrapper applies (same wrapper as other host-side callbacks); the `foreign_function_denied` envoy-go-strict counter increments on the NotFound path. The full ~70-path `proxy_get_property` roster per AMEND-B4: ~10 dispatched roots (`request` 16 sub-paths + `response` 6 + `connection` 12+id + `source` 2 + `destination` 2 + `upstream` ~14 + `xds` 12 + `metadata` + `filter_state` + `upstream_filter_state` + `wasm.<key>` proxy) + 4 direct tokens (`plugin_name` + `plugin_root_id` + `plugin_vm_id` + `connection_id`); NUL-delimited path serialization per spec README §Serialization + cpp-host `context.cc:1047-1058`; envoy-go-internal primitive mapping: stream-local accessors for request/response/source/destination/wasm-direct-tokens; RE-CONSUMES ADR-0144 `DownstreamPrincipal()` for connection TLS cert sub-paths (second co-consumer beyond phase-04); RE-CONSUMES ADR-0177 `internal/httpclient/` for upstream cluster + address sub-paths (third-or-later co-consumer); RE-CONSUMES ADR-0190 `internal/dynamicmetadata/` for metadata + xds metadata branches (third-or-later co-consumer); EXTRACTS NEW `internal/filterstate/` per ADR-0207 for filter_state + upstream_filter_state + wasm.<key> proxy branches (consumer #2; phase-22.2 lua is consumer #1 via MIGRATION). The NEW `internal/filterstate/` framework primitive at consumer #2 scope per Q7 + ADR-0207: `Bucket struct { mu sync.RWMutex; items map[string]FilterStateObject }` per-stream accessor + `FilterStateObject` interface (Marshal/Unmarshal/HasData/StateType) + `StateType` discriminator (StateTypeReadOnly vs StateTypeMutable; mutable-state-type entries OVERRIDE existing entries; read-only entries with same key as Mutable entry are rejected); EXPLICIT API-REVISION ALLOWANCE clause for consumer #3+ (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families). The phase-22.2 lua MIGRATION at Task 18: `internal/filter/http/lua/filterstate.go` REWRITES to delegate to `internal/filterstate/*Bucket` via a thin adapter; the `:filterState()` Lua surface stays UNCHANGED (the `:filterState():get(name)` + `:filterState():set(name, value)` accessors delegate to `*Bucket.Get` / `*Bucket.Set`); the 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 (mutation exposure + typed Lua-value marshaling) carry forward unchanged; the migration delta is ~50-100 LoC inside `internal/filter/http/lua/`; existing phase-22.2 lua filterstate tests MUST stay byte-identical (non-breaking) — 100% green run + no test wording change. The NEW `internal/stats/dynamic/` infrastructure subpackage per ADR-0208 + AMEND-B2: `Registry` is per-plugin-config (one `*Registry` per `*compiledConfig`); the `*compiledConfig` constructs the Registry at config-load via `dynamic.NewRegistry(pluginScope, maxEntries)` where `pluginScope = stats.RootScope.Subscope("wasm").Subscope(pluginName)`; the Registry produces stat names `wasmcustom.<custom_name>` under that scope (admin /stats surfaces as `wasm.<plugin>.wasmcustom.<custom_name>` via parent scope nesting; in-wire stat name from proxy-wasm wire perspective is `wasmcustom.<custom_name>` byte-faithful to upstream); 1024-entry cap envoy-go-strict + `dynamic_stats_cap_exceeded` counter + `envoy_go_strict_dynamic_stats_max_entries` config field. The fixture-0036 mixed-mode discipline per Q8 + ADR-0192 precedent: single-listener single-HCM hosting the wasm filter + router terminator + TWO upstream clusters (`cluster_a` primary + `cluster_b` httpCall target — both bound to the SAME differential echobackend at different cluster definitions per phase-22.2 REVIEW §7.4 `freeTCPPort` flake mitigation); 14 scenarios partitioned by assertion-class (10 deterministic cross-side via `CompareBytes` covering body-read-only / body-mutate-passthrough / body-mutate-replace / trailers-add / trailers-read / shared-data-read-after-write / foreign-function-deny-default / property-stream-info / metric-define-only / env-vars-rejected-passthrough + 4 non-deterministic subject-only via `StatsAsserter.AssertStats` covering tick-fires-counter / httpCall-success / httpCall-unknown-cluster / body-cap-exceeded); every subject-only StatsAsserter arm gets a deliberate-break liveness verification at IMPL Task 19 per `reference_differential_asserter_dispatch` (deliberately break the stat assertion → expect FAIL → restore + verify GREEN) — mandatory; per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch) the mixed-mode dispatch lives at the existing cross-side runner branch with the StatsAsserter subject-side arms folded in. The fixture-0037 subject-only boot-reject per D-25.2-P1 IMPL-time closure: anticipated arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`; reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field (silent drop by upstream's protobuf parser); subject envoy-go boot-rejects; runner-branch shape settles at IMPL Task 20 — recommended: extend `BootRejectFixture` with `subjectOnly: true` flag (minimal infrastructure delta); alternative: NEW `SubjectOnlyBootRejectFixture` runner branch (cleaner type-discriminated dispatch); final disposition at IMPL.

**Tech Stack:** Go 1.26.2 (Go-floor STAYS at `go 1.23.0` per wazero v1.10.1's Go-1.23 floor inherited from 25.1 + AMEND-A1); `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/wasm/v3` for the `Wasm` proto — UNCHANGED at 25.2; `envoy/extensions/wasm/v3` for `PluginConfig` + `VmConfig` + `CapabilityRestrictionConfig` + `SanitizationConfig` — UNCHANGED at 25.2; the 4 NEW envoy-go-strict-only `PluginConfig` config fields per Qs 2/6/9 live on the envoy-go-internal `*compiledConfig` after parse, populated via a custom envoy-go protobuf extension OR JSON sidecar — final mechanism settles at IMPL Task 13 first-action; per the project's discipline these envoy-go-strict-only fields stay envoy-go-strict-only with NO upstream impact); `envoy/config/core/v3` for `AsyncDataSource` + `DataSource` UNCHANGED at 25.2; **NO new direct go.mod dependency at 25.2** (wazero v1.10.1 + proxy-wasm-rust-sdk =0.2.4 inherited from 25.1 per AMEND-A1; no new wazero version bump anticipated); stdlib `sync` (`sync.RWMutex` for `*RootVM.sharedDataMu` + `*ForeignFunctionRegistry.mu` + `*dynamic.Registry.mu` + `*filterstate.Bucket.mu`; `sync.Mutex` for `*RootVM.httpCallsMu` + `*RootVM.tickMu`); stdlib `time` (`time.Duration` for tick period + httpCall timeout; `time.Now()` for deadline computation); stdlib `context` (per-stream context threading); stdlib `bytes` + `io` (body-buffer accumulation + `proxy_get_buffer_bytes` slice manipulation); stdlib `encoding/binary` (NUL-delimited property-path parsing — re-uses pairs wire-format conventions from 25.1 `internal/wasm/pairs.go`); stdlib `errors` (`errors.New` for byte-stable PARSE-REJECT consts at the 6 NEW 25.2 arms); stdlib `fmt` (PARSE-REJECT wording wrapping); reference Envoy `envoyproxy/envoy:v1.37.2` SHA per ADR-0008 + ENVOY_TARGET.md (UNCHANGED from 25.1); proxy-wasm specification v0.2.1 (sentinel export `proxy_abi_version_0_2_1` UNCHANGED); proxy-wasm-rust-sdk `=0.2.4` + `wasm32-wasip1` Rust target (per AMEND-A1; reproduction-source language for the 14 fixture-0036 plugins under `scripts/` subdirectory; pre-built `.wasm` bytecode vendored under `bytecode/` subdirectory per Q8 + inherited 25.1 fixture-0034 reproduction-discipline); proxy-wasm-cpp-host `da3ce05d` reference (transcription source for AMEND-B1 buffer-clamp at `src/exports.cc:get_buffer_bytes`; AMEND-B3 cancel-at-destruction at `context.cc:1900-1905`; AMEND-B5 gate-at-registerCallback at `wasm.cc:176-189`); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (NO TLS surface at phase-25.2 fixture-0036; cluster_b HttpCall uses plaintext); ADR-0186 `clock.Clock` seam (FIRST co-consumer beyond phase-21 itself per Q5 + R-25.2-9 — RATIFIES the phase-21 Clock-seam extraction); ADR-0144 `DownstreamPrincipal()` (second co-consumer beyond phase-04 itself for `connection.{subject,uri_san,dns_san,sha256,tls_version}_*` property sub-paths per AMEND-B4); ADR-0177 `internal/httpclient/` (third-or-later co-consumer for `proxy_http_call` cluster dispatch + `upstream.*` property sub-paths per AMEND-B4 — phase-22.2 `:httpCall()` was second; CLOSES parent §13-R6); ADR-0190 `internal/dynamicmetadata/` (third-or-later co-consumer for `metadata.*` + `xds.*_metadata` property branches per AMEND-B4).

---

## Scope check — why phase 25.2 ships as one sub-phase row (settled at parent BRAINSTORM Q1 + BRAINSTORM Q11 + PLAN-time split-gate confirms)

Phase 25 was PRE-SPLIT THREE-way at the parent BRAINSTORM commit per Q1 (envelope D delivered across 25.1 + 25.2 + 25.3); the 25.1 sub-phase landed at squash `feded64` (SHA-fill `de4f853`); the 25.2 BRAINSTORM landed at `0589f85`; the 25.2 SPEC landed at `f0eae39`. The 25.2 BRAINSTORM Q11 settled the within-25.2-split question (stay single sub-phase 25.2; PLAN-stage split-gate arbiter per phase-22.2 Q14 precedent). This PLAN is for the 25.2 sub-phase ONLY; no further nested split per Q11 + ADR-0106 (sub-sub-phase splits are structurally awkward; matches phase-22.2 + phase-25.1 stay-single sub-phase PLAN precedent).

The PLAN-time re-evaluation per `superpowers:writing-plans` GATE + ADR-0045 §6 confirms single-sub-phase landing:

- **Task count: 22** — comfortably under the ADR-0045 25-task split-gate. Maps the SPEC §15 46-item acceptance checklist + §3 framework primitive evolutions (~12 internal/wasm/ files + 2 NEW packages + lua MIGRATION) + §3.6 filter package extensions (~9 files) + §6 6 NEW PARSE-REJECT arms + §7 9 NEW envoy-go-strict counters + §8 2 NEW fixtures + §8.4 35th fuzzer + §10 4 ADR §Decision body landings + §13.4 ~7-edit BEHAVIOR_CONTRACT.md bundle + §15 atomic-landing meta-tasks into 22 discrete tasks across 6 tiers (Tier A `internal/wasm/` root-VM core Tasks 1-3; Tier B `internal/wasm/abi/` family dispatches Tasks 4-8 — partly 5-way parallel; Tier C NEW packages + property roster Tasks 9-13 — partly 3-way parallel; Tier D `internal/filter/http/wasm/` extensions Tasks 14-18 — partly 3-way parallel; Tier E fuzzer + fixtures Tasks 19-21 — partly 2-way parallel; Tier F atomic landing Task 22).
- **LoC: ~5,200-7,400 production+test+fixture+docs** (per SPEC §1 envelope ~5,000-7,500 LIVE production + ~6,000-9,000 TEST; ~2,400-3,200 LoC `internal/wasm/` production + test extensions per §3.5 file split; ~1,000-1,400 LoC `internal/filter/http/wasm/` production + test extensions per §3.6 file split; ~400-600 LoC NEW `internal/filterstate/` package per §3.2; ~500-700 LoC NEW `internal/stats/dynamic/` package per §3.3; ~50-100 LoC phase-22.2 lua MIGRATION delta per §3.4 MIGRATES; ~1,200-1,700 LoC fixture-0036 + fixture-0037 + 14 Rust sources + 14 vendored `.wasm` blobs + 1 boot-reject driver; ~1,200-1,700 LoC docs — BEHAVIOR_CONTRACT.md ~7-edit bundle + 4 ADR §Decision+§Consequences bodies + ADR-0202 one-line AMEND + STATE.md + ROADMAP.md + PROGRESS.md + REVIEW.md). The IMPL LoC sits above the ~1500 LoC PLAN-size soft-gate, **but the PLAN gate is about PLAN.md size** (this PLAN at ~1700-1900 lines sits at the soft-gate boundary), not IMPL LoC — the IMPL LoC sizing per Task is settled at the SPEC §3 + §6 + §15. The phase-22.2 + phase-25.1 EXTRACT-NOW-primitive-bring-up precedent (which also exceeded the LoC-arm at framework-primitive bring-up; no further nested split was triggered) RATIFIES this disposition.
- **Phase 25.2 ships as the single sub-phase row it is** — no further nested split. The 25.2 phase-done squash-merge **CLOSES row 25.2** (in-progress → done) at the same commit; parent row `25` STAYS `in-progress` until 25.3 IMPL phase-done per the sub-row rollup discipline per ADR-0106 + phase-18.1 + phase-18.2 + phase-19.1 + phase-19.2 + phase-22.1/22.2/22.3 + phase-24.1/24.2 + phase-25.1 precedent.

Net change estimate for 25.2 (mirroring the phase-25.1 PLAN component-table convention; LoC per file is the SPEC §3.5 + §3.6 envelope projected forward — IMPL may shift within ±20% of these ranges):

- `internal/wasm/doc.go` ~+30-50 (EXTENDED at 25.2 with Q1-Q11 BRAINSTORM cross-refs + AMEND-B1..B5 cross-refs + 25.2 ABI surface summary; lands at Task 1)
- `internal/wasm/vm.go` DELETE (per D-P-PLAN-6 — replaced by `root_vm.go` + `stream_context.go`; Task 1)
- `internal/wasm/root_vm.go` NEW ~400-550 (RootVM type per §3.1 + RootVMOption pattern + NewRootVM + Configure + NewStreamContext + Close + the embedded tick goroutine spawn + shared-data map init + httpCall routing state + foreign-function registry view + per-plugin dynamic-stats Registry plumbing; Task 1)
- `internal/wasm/stream_context.go` NEW ~250-400 (StreamContext type per §3.1 + per-callback methods CallProxyOn{RequestHeaders,ResponseHeaders,RequestBody,ResponseBody,RequestTrailers,ResponseTrailers,Done,Log,Delete} + Close idempotent + cancel-at-destruction discipline per AMEND-B3; Task 1)
- `internal/wasm/root_vm_test.go` NEW ~400-550 (RootVM lifecycle + tick goroutine start/stop + shared-data Set/Get/CAS + httpCall DispatchHttpCall + cancel-at-destruction + http_call_response_after_close counter + foreign-function CallForeignFunction Get-Hit + NotFound paths; Task 1)
- `internal/wasm/stream_context_test.go` NEW ~300-450 (per-stream lifecycle + per-stream isolation under concurrent dispatch + panic-wrapper integration; Task 1)
- `internal/wasm/sandbox.go` EXTEND ~+150-220 (21 NEW capability key constants per AMEND-B5; 14 hostcall keys + 7 lifecycle keys; total roster 37 → 58; Task 2)
- `internal/wasm/sandbox_test.go` EXTEND ~+200-300 (per-NEW-key ALLOW/DENY exhaustive + gate-at-registration assertion: deny `proxy_set_tick_period_milliseconds` capability → assert host function NOT registered on the wazero Runtime per AMEND-B5; Task 2)
- `internal/wasm/registration.go` EXTEND ~+250-400 (14 NEW env-namespace hostcall registrations per §5.1 + 7 NEW callback dispatch entries per §5.3 + gate-at-registration discipline per AMEND-B5 — for each NEW capability key, if SandboxConfig.IsAllowed returns false, do NOT register the host function on the wazero Runtime; the buffer-clamp wire-contract per AMEND-B1 + R-25.2-1 + R-25.2-5 host-module wiring assertion; Task 3)
- `internal/wasm/registration_test.go` EXTEND ~+200-300 (14 NEW hostcall registration round-trip tests + 7 NEW callback dispatch tests + gate-at-registration assertions per R-25.2-5; Task 3)
- `internal/wasm/abi/types.go` UNCHANGED at 25.2 (per §2.21 + §3.5 — existing enums cover the 25.2 surface; values activated via host-module registration extensions in Task 3)
- `internal/wasm/abi/body_bridge.go` NEW ~150-220 (proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_status dispatch with AMEND-B1 clamp-on-overflow per R-25.2-1; WasmBufferType values 0/1/4 active; Task 4)
- `internal/wasm/abi/body_bridge_test.go` NEW ~200-300 (AMEND-B1 buffer-clamp golden table: start_in_bounds + max_in_bounds → Ok with full length; start_in_bounds + max_overflows → Ok with truncated length; start_at_end + max_anything → Ok with length=0; start_beyond_end → Ok with length=0; start+max_size i32-overflow → BadArgument; per R-25.2-1 wire-shape pin; Task 4)
- `internal/wasm/abi/stream_control.go` NEW ~80-120 (proxy_continue_stream + proxy_close_stream dispatch; paired with PAUSE-buffer dispatch on body callbacks; Task 4)
- `internal/wasm/tick.go` NEW ~200-300 (per-RootVM tick goroutine + 10ms envoy-go-strict period floor + Clock seam injection via WithRootClock — FIRST co-consumer of phase-21 ADR-0186 Clock seam beyond phase-21 itself per Q5 + R-25.2-9; `effectivePeriod = max(period_ms, 10ms)`; period=0 cancels; Task 5)
- `internal/wasm/abi/timer.go` NEW ~80-120 (proxy_set_tick_period_milliseconds dispatch + delegate to *RootVM.SetTickPeriod; Task 5)
- `internal/wasm/tick_test.go` NEW ~250-400 (tick goroutine fake-time tests via clock.FakeClock from phase-21 internal/clock; 10ms floor enforcement — set period=5ms → assert effectivePeriod=10ms → assert tick fires at 10ms intervals; period=0 cancels; concurrent stream contexts share one tick goroutine; Task 5)
- `internal/wasm/shared_data.go` NEW ~200-280 (per-RootVM CAS-protected K-V map per Q6 + R-25.2-10; sync.RWMutex; sharedDataEntry struct {value []byte; cas uint32}; CAS semantic byte-exact from cpp-host — cas=0 unconditionally writes returning new CAS in subsequent get; cas>0 writes only if existing entry's CAS matches returning WasmResult::CasMismatch on mismatch; per-value 1 MiB cap + 1024-entry cap envoy-go-strict + cap-exceeded WasmResult::InternalFailure + counter; Task 6)
- `internal/wasm/abi/shared_data.go` NEW ~100-150 (proxy_get_shared_data + proxy_set_shared_data dispatch + delegate to *RootVM.SetSharedData / *RootVM.GetSharedData; Task 6)
- `internal/wasm/shared_data_test.go` NEW ~300-450 (CAS golden table: cas=0 always writes; cas>0 writes only on match; mismatch returns CasMismatch; cap-boundary tests: value-cap-exceeded → InternalFailure + counter; entry-cap-exceeded → InternalFailure + counter; concurrent-Set stress under sync.RWMutex; Task 6)
- `internal/wasm/foreign.go` NEW ~150-220 (ForeignFunctionRegistry per AMEND-A9 + R-25.2-8 + D-25.2-P3 PLAN-time closure; sync.RWMutex; NewForeignFunctionRegistry; Register/Get; EMPTY default registry; process-global wasm.DefaultForeignFunctionRegistry; Task 7)
- `internal/wasm/abi/foreign.go` NEW ~120-180 (proxy_call_foreign_function dispatch + delegate to *RootVM.CallForeignFunction; foreign_function_denied counter increment on NotFound path; Task 7)
- `internal/wasm/foreign_test.go` NEW ~250-380 (ForeignFunctionRegistry Register/Get + EMPTY-default NotFound behavior; capability-gated default-deny — proxy_call_foreign_function denied → InternalFailure per ADR-0204 inherited; registered-then-deregister-then-call sequence; D-P3 closure: concurrent dispatch from N streams to same registered function — verify mutex-per-RootVM serialization + no cross-stream argument leak; Task 7)
- `internal/wasm/http_call.go` NEW ~300-450 (proxy_http_call dispatch via per-RootVM *httpclient.Client per Q4 + R-25.2-3 + AMEND-B3; AsyncClient request lifecycle; call_id allocation + httpCalls map tracking; cancel-at-destruction via *StreamContext.Close → httpclient.Cancel for outstanding call_ids dispatched from this stream; defensive token-miss guard at response arrival → http_call_response_after_close counter increment; BadArgument-on-unknown-cluster per Q4 + counter http_call_dispatch_unknown_cluster; Task 8)
- `internal/wasm/abi/http_call.go` NEW ~150-220 (proxy_http_call host shim — pairs decode for headers/trailers + delegate to *RootVM.DispatchHttpCall; proxy_on_http_call_response callback dispatch route to *StreamContext; Task 8)
- `internal/wasm/http_call_test.go` NEW ~400-550 (proxy_http_call dispatch with mock httpclient.Client + cluster_b second-upstream-cluster scenario; cancel-at-destruction race-test — StreamContext.Close cancels in-flight requests + late-response-after-close → http_call_response_after_close counter increment + defensive token-miss path; BadArgument on unknown cluster; concurrent N=100 httpCall dispatches from same RootVM verify call_id allocation isolation; Task 8)
- `internal/filterstate/filterstate.go` NEW ~250-400 (per ADR-0207 + R-25.2-6; Bucket + FilterStateObject interface + StateType discriminator + Set/Get/Keys; EXPLICIT API-REVISION ALLOWANCE clause; Task 9)
- `internal/filterstate/filterstate_test.go` NEW ~250-380 (Set/Get/Keys round-trip + read-only-vs-mutable conflict + nil-handling; Task 9)
- `internal/filterstate/bucket_concurrency_test.go` NEW ~150-220 (RWMutex discipline + concurrent-read concurrent-add tests; Task 9)
- `internal/filterstate/filterstateobject_test.go` NEW ~150-220 (interface conformance + edge cases; Task 9)
- `internal/filter/http/lua/filterstate.go` REWRITE ~50-100 LoC delta (delegate to *filterstate.Bucket per ADR-0207 MIGRATION; non-breaking; `:filterState()` Lua surface UNCHANGED; Task 10)
- `internal/stats/dynamic/dynamic.go` NEW ~300-450 (per ADR-0208 + AMEND-B2 + R-25.2-7; Registry + MetricID + MetricType + NewRegistry + Register/Increment/Record/Get/EnumerateForAdmin; per-plugin Registry scope; signed-i64 delta; unsigned-u64 value; 1024-entry cap envoy-go-strict; Task 11)
- `internal/stats/dynamic/dynamic_test.go` NEW ~300-450 (Register/Increment/Record/Get round-trip + signed-delta semantics + idempotent-Register + ErrCapExceeded threshold + ErrBadArgument enforcement — Increment on Histogram; Record on Counter; Task 11)
- `internal/stats/dynamic/dynamic_admin_test.go` NEW ~200-300 (admin /stats enumeration round-trip + name format `wasmcustom.<custom_name>` byte-pin per R-25.2-2; Task 11)
- `internal/stats/dynamic/dynamic_concurrency_test.go` NEW ~150-220 (RWMutex discipline + concurrent-Register stress at cap-boundary race; Task 11)
- `internal/wasm/dynamic_stats.go` NEW ~150-220 (wraps per-plugin *dynamic.Registry; proxy_define_metric dispatch + signed-i64 delta plumbing per AMEND-B2 + 1024-entry cap + dynamic_stats_cap_exceeded counter increment on cap-exceeded; Task 12)
- `internal/wasm/abi/metrics.go` NEW ~150-220 (proxy_define_metric + proxy_increment_metric + proxy_record_metric + proxy_get_metric dispatch + delegate to *dynamic.Registry methods; Task 12)
- `internal/wasm/dynamic_stats_test.go` NEW ~300-450 (proxy_define_metric + Increment + Record + Get round-trip; idempotent re-Register; 1024-entry cap-boundary; signed-i64 delta extremes per AMEND-B2; Task 12)
- `internal/wasm/abi/metrics_test.go` NEW ~150-220 (MetricType enum byte-pin: Counter=0, Gauge=1, Histogram=2 per AMEND-B2; ErrBadArgument enforcement; Task 12)
- `internal/wasm/property.go` NEW ~450-650 (full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4; NUL-delimited path parsing; per-root dispatch covering ~10 dispatched roots + 4 direct tokens; co-consumed primitive mapping — stream-local for request/response/source/destination/wasm-direct-tokens; RE-CONSUMES ADR-0144 for connection.tls.*; RE-CONSUMES ADR-0177 for upstream cluster + address; RE-CONSUMES ADR-0190 for metadata + xds metadata; CONSUMES NEW internal/filterstate/ for filter_state + upstream_filter_state + wasm.<key> proxy; Task 13)
- `internal/wasm/property_test.go` NEW ~500-700 (table-driven tests for ~70 sub-paths per AMEND-B4 + NUL-delimited path parsing: empty path; trailing NUL tolerated; non-NUL separator → NotFound; absent-property NotFound semantics; co-consumed primitive integration round-trip; Task 13)
- `internal/filter/http/wasm/doc.go` EXTEND ~+30-50 (25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 + D-25.2-P1..P5 cross-refs + API surface evolution summary; Task 14)
- `internal/filter/http/wasm/compiled_config.go` EXTEND ~+200-300 (4 envoy-go-strict-only PluginConfig config fields per Qs 2/6/9 + 6 NEW PARSE-REJECT arms per §6.2 + RootVM construction at New() via wasm.NewRootVM — NOT per-stream wasm.NewVM; per-plugin dynamic.Registry construction; per-plugin filterstate.Bucket access pattern; Task 14)
- `internal/filter/http/wasm/compiled_config_test.go` EXTEND ~+200-300 (6 NEW PARSE-REJECT arm coverage per §6.2 + envoy-go-strict-only config field validators + TestParseRejectConstants_ByteStable EXTENDED with 6 NEW arms per D-25.2-P5; Task 14)
- `internal/filter/http/wasm/abi_callbacks.go` EXTEND ~+400-600 (7 NEW methods per §5.3: OnRequestBody + OnResponseBody + OnRequestTrailers + OnResponseTrailers + OnTick + OnHttpCallResponse + OnForeignFunction + 4 RE-USE primitive consumer integration — ADR-0144 DownstreamPrincipal for connection.tls.*; ADR-0177 httpclient for upstream.*; ADR-0190 dynamicmetadata for metadata.*; NEW ADR-0207 filterstate for filter_state.*; Task 15)
- `internal/filter/http/wasm/abi_callbacks_test.go` EXTEND ~+400-550 (7 NEW method coverage + 4 RE-USE primitive round-trip tests; Task 15)
- `internal/filter/http/wasm/body.go` NEW ~200-280 (DecodeData + EncodeData glue; body-buffer accumulation per-stream; cap enforcement via envoy_go_strict_body_buffer_cap_bytes default 16 MiB per Q2; 413-on-exceed via SendLocalReply (decode side) or response terminate (encode side); body_buffer_cap_exceeded counter + envoy_go.failures counter per §2.25; NO-op if proxy_on_request_body not exported by guest; Task 16)
- `internal/filter/http/wasm/trailers.go` NEW ~120-180 (DecodeTrailers + EncodeTrailers glue; CallProxyOnRequestTrailers / CallProxyOnResponseTrailers; reuses 25.1 pairs wire-format; Task 16)
- `internal/filter/http/wasm/tick_clock.go` NEW ~80-120 (Clock seam injection plumbing — WithRootClock; fake-time test seam for fixture-0036 tick scenarios; Task 16)
- `internal/filter/http/wasm/body_test.go` NEW ~300-450 (body-buffer accumulation + cap enforcement + 413-on-exceed dispatch + body_buffer_cap_exceeded counter assertion; Task 16)
- `internal/filter/http/wasm/trailers_test.go` NEW ~200-300 (trailer hostcall dispatch round-trip + WasmHeaderMapType values 1/3 activation; Task 16)
- `internal/filter/http/wasm/property.go` NEW ~250-400 (per-stream property resolver dispatch — delegates to wasm.RootVM.property tree + the 4 RE-USE primitives per AMEND-B4; Task 17)
- `internal/filter/http/wasm/stats.go` EXTEND ~+100-150 (9 NEW envoy-go-strict counters per §7.1 + AMEND-B3: tick_invocations + http_call_dispatched + http_call_response + foreign_function_denied + body_buffer_cap_exceeded + http_call_dispatch_unknown_cluster + shared_data_cap_exceeded + dynamic_stats_cap_exceeded + http_call_response_after_close; raises 119 → 128 project total; per-plugin dynamic.Registry plumbing; Task 17)
- `internal/filter/http/wasm/property_test.go` NEW ~300-450 (per-stream property resolver dispatch coverage for ~70 sub-paths per AMEND-B4 + co-consumed primitive round-trips + absent-property NotFound semantics; Task 17)
- `internal/filter/http/wasm/decode_headers.go` EXTEND ~+50-80 (per-stream construction goes through cfg.rootVM.NewStreamContext(ctx) — NOT wasm.NewVM; streamCtx field replaces vm field on filter struct; Task 18)
- `internal/filter/http/wasm/encode_headers.go` EXTEND ~+30-50 (per-stream context shared with decode side; Task 18)
- `internal/filter/http/wasm/dispatch_test.go` EXTEND ~+200-300 (body/trailer/tick/httpCall integration round-trips + per-stream concurrency tests under root-VM model; Task 18)
- `internal/filter/http/wasm/fuzz_hostcall_test.go` NEW ~150-220 (35th project-wide fuzzer FuzzWasmHostcallEnvelope per §8.4 + R-25.2-12; ~30-40 corpus seeds per D-P-PLAN-10 across 10 dimensions; Task 19)
- `internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/` corpus directory (~30-40 seeds; Task 19)
- `internal/filter/http/wasm/wasm_bench_test.go` EXTEND ~+50-80 (BenchmarkPerStreamModule_Instantiation per R8 gate + D-25.2-P2; Task 22)
- `test/differential/fixture/fixture.go` MODIFY ~+15 (NEW BackendKind=HTTPWasmAdvanced enum value OR REUSE existing HTTPWasm — settles at IMPL Task 20 first-action; Task 20)
- `test/differential/runner_test.go` MODIFY ~+15 (blank import + switch-case for HTTPWasmAdvanced; Task 20) + ~+5 (blank import for fixture-0037; Task 21)
- `test/fixtures/0036-http-wasm-body-and-advanced/README.md` NEW ~200-300 (scope + 14-scenario table + topology + cross-refs to SPEC §8 + ADR-0205+0206+0207+0208; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/envoy.yaml` NEW ~200-300 (reference Envoy bootstrap; single listener + wasm filter + 2 upstream clusters cluster_a + cluster_b; templated {{.BackendPort}} + {{.UpstreamBPort}}; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/envoy-go.yaml` NEW ~200-300 (subject bootstrap; same topology + 4 envoy-go-strict-only PluginConfig extension fields populated; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/expectations.yaml` NEW ~120-200 (human-readable declarative scenario expectations; NOT consumed by runner; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/inputs/driver.go` NEW ~800-1200 (registered Driver impl + per-scenario probes + classifyBody + StatsAsserter.AssertStats implementations for 4 subject-only scenarios per `reference_differential_asserter_dispatch`; deliberate-break liveness verification mandatory per IMPL Task 20 final-action; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/scripts/{a..n}_<name>/{Cargo.toml,src/lib.rs}` NEW ~30 LoC each × 14 scenarios = ~420 LoC (Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/scripts/README.md` NEW ~80-120 (reproduction script + pinned rustup toolchain + cargo build invocation; Task 20)
- `test/fixtures/0036-http-wasm-body-and-advanced/bytecode/{a..n}_<name>.wasm` NEW 14 vendored pre-built `.wasm` binary blobs committed to git per Q8 inheriting 25.1 Q9 + AMEND-A1 discipline (Task 20)
- `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/README.md` NEW ~100-150 (Task 21)
- `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy.yaml` NEW ~100-150 (reference Envoy bootstrap; deliberately-malformed config triggering anticipated D-25.2-P1 arm 19 — envoy_go_strict_body_buffer_cap_bytes=0; reference Envoy v1.37.2 accepts the unknown field silent-drop; Task 21)
- `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy-go.yaml` NEW ~100-150 (subject bootstrap symmetric; envoy-go boot-rejects on arm 19; Task 21)
- `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/inputs/driver.go` NEW ~150-250 (implements subject-only BootRejectFixture variant per D-25.2-P1 runner-branch shape decision; Task 21)
- `docs/envoy-go/DECISIONS.md` MODIFY (ADR-0205+0206+0207+0208 §Decision+§Consequences bodies + ADR-0202 §Consequences one-line in-place AMEND acknowledgment per §10.2 + CONDITIONAL ADR-0209 if R8 escape-valve fires per D-P-PLAN-11; ~+600-900 LoC delta; Task 22)
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` MODIFY ~+350-500 LoC (Task 22 ~7-edit bundle per §13.4: edit #1 extend `### envoy.filters.http.wasm` subsection + body/buffer/trailers/timer/metrics/shared-data/httpCall/foreign-function/property bridge details + AMEND-B1..B5 cross-refs ~150-250 lines; edit #2 stat-table 119 → 128 extension + per-plugin Registry scope discipline note ~25-40 lines; edit #3 envoy-go-strict departure record #1 9-counter consolidated bundle per AMEND-B3 ~20-30 lines; edit #4 envoy-go-strict departure record #2 body-buffer cap discipline per Q2 ~15-25 lines; edit #5 envoy-go-strict departure record #3 shared-data cap + tick period 10ms floor consolidated per Q5+Q6 ~25-40 lines; edit #6 envoy-go-strict departure record #4 foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace clarification per AMEND-A9+Q9+AMEND-B2 ~30-45 lines; edit #7 EXTEND/RENAME `### Phase 25.1 forward-pointer notes` → `### Phase 25.2 forward-pointer notes` subsection ~50-80 lines; Task 22)
- `docs/envoy-go/ROADMAP.md` MODIFY (row 25.2 flips `in-progress → done` at Task 22; per-cell IMPL-done annotation per ADR-0106; parent row 25 STAYS in-progress; sub-row 25.3 UNCHANGED planned; ~+1 net; Task 22)
- `docs/envoy-go/STATE.md` MODIFY (rewrite-in-place at Task 22; Task 22)
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` NEW ~900-1300 across 22 task entries + Pre-Task 0 (Task 22)
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/REVIEW.md` NEW ~350-450 (Task 22)

**Production code: ~4,500-6,400 LoC** (`internal/wasm/` evolution ~2,100-2,900 + `internal/filter/http/wasm/` extensions ~1,000-1,400 + NEW `internal/filterstate/` ~250-400 + NEW `internal/stats/dynamic/` ~300-450 + lua MIGRATION ~50-100 + fuzzer corpus ~100-200 + runner switch +20 + enum +15 + 14 Rust source ~420) **+ ~3,800-5,200 LoC tests** + ~1,500-2,200 LoC fixture-0036 + fixture-0037 (including 14 Rust sources + 14 vendored `.wasm` blobs + 1 boot-reject driver) + ~1,400-2,100 LoC docs ≈ **~11,200-15,900 LoC total**. **Task count: 22** — comfortably under the ADR-0045 25-task split-gate; LoC-arm above 1500 threshold but acceptable per phase-22.2 + phase-25.1 EXTRACT-NOW-primitive-bring-up precedent.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/wasm/doc.go` | EXTEND | Append 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs + 25.2 ABI surface evolution summary (RootVM lifecycle + ABICallbacks 13→20 methods + SandboxConfig 37→58 keys + tick goroutine + shared-data + httpCall + foreign-function + property roster + dynamic-stats wrap). ~+30-50 LoC. Lands at Task 1. |
| `internal/wasm/vm.go` | DELETE | The 25.1 per-stream `*VM` type RETIRES at 25.2 per D-P-PLAN-6 + Q3 + ADR-0205. Replaced by `root_vm.go` + `stream_context.go`. The 25.1 per-stream `*VM` had ZERO consumers outside `internal/filter/http/wasm/` which migrates in Task 18 (decode/encode_headers EXTEND). Lands at Task 1. |
| `internal/wasm/root_vm.go` | NEW | `*RootVM` lifecycle anchor per §3.1 — RootVM struct + RootVMOption pattern + `NewRootVM(ctx, module, rootCtxID, opts...)` + `Configure(vmConfigBytes, pluginConfigBytes)` invokes `_initialize`/`_start` + `proxy_on_vm_start` + `proxy_on_configure` on root context ONCE at config-load; `NewStreamContext(ctx)` allocates monotonic streamCtxID + invokes `proxy_on_context_create(streamCtxID, rootCtxID)` + returns `*StreamContext`; tick goroutine spawn (initially idle); shared-data map init; httpCall routing state init; foreign-function registry view init; per-plugin dynamic-stats Registry plumbing; `Close()` idempotent (stops tick + cancels outstanding httpCalls + closes dynamic-stats Registry + releases wazero Runtime). RootVMOption setters: WithRootSandboxConfig + WithRootClock (FIRST co-consumer of ADR-0186 beyond phase-21) + WithRootPanicHandler + WithRootLogSink + WithRootHttpClient + WithRootForeignRegistry + WithRootDynamicStatsRegistry + WithRootSharedDataCaps. ~400-550 LoC. Lands at Task 1. |
| `internal/wasm/stream_context.go` | NEW | `*StreamContext` per-stream context per §3.1 — StreamContext struct (rootVM + ctxID + cb ABICallbacks + sentLocalResponse captured-state); per-callback methods CallProxyOn{RequestHeaders, ResponseHeaders, RequestBody, ResponseBody, RequestTrailers, ResponseTrailers, Done, Log, Delete}; RegisterABICallbacks; HasGlobalFunc lookup pass-through; Close() idempotent (fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels any outstanding httpCalls dispatched from this stream per AMEND-B3 + R-25.2-3). NOT goroutine-safe per-stream — access is per-stream-single-goroutine by envoy-go's filter dispatch model (inherited from 25.1). ~250-400 LoC. Lands at Task 1. |
| `internal/wasm/root_vm_test.go` | NEW | RootVM lifecycle round-trip (NewRootVM → Configure → NewStreamContext → CallProxyOn* → Close); tick goroutine start/stop via Clock seam fake-time; shared-data Set/Get/CAS round-trip; httpCall DispatchHttpCall + cancel-at-destruction race-test + http_call_response_after_close counter increment on stray response; foreign-function CallForeignFunction Get-Hit + NotFound paths; concurrent N=100 stream contexts share one RootVM verify no cross-stream state leak. ~400-550 LoC. Lands at Task 1. |
| `internal/wasm/stream_context_test.go` | NEW | Per-stream lifecycle (NewStreamContext → CallProxyOn* → Close); per-stream isolation under concurrent dispatch (N=100 × `-count=10` per 25.1 dispatch_test precedent); panic-wrapper integration; Close idempotent. ~300-450 LoC. Lands at Task 1. |
| `internal/wasm/compile.go` | UNCHANGED at 25.2 | Per §3.5 + D-P-PLAN-5 — CompileCache scope STAYS at `*compiledConfig`-instance per 25.1 D-P-PLAN-5; no API delta. |
| `internal/wasm/sandbox.go` | EXTEND | 21 NEW capability key constants per AMEND-B5 (14 hostcall: proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_status + proxy_continue_stream + proxy_close_stream + proxy_set_tick_period_milliseconds + proxy_define_metric + proxy_increment_metric + proxy_record_metric + proxy_get_metric + proxy_set_shared_data + proxy_get_shared_data + proxy_http_call + proxy_call_foreign_function; 7 lifecycle: proxy_on_request_body + proxy_on_response_body + proxy_on_request_trailers + proxy_on_response_trailers + proxy_on_tick + proxy_on_http_call_response + proxy_on_foreign_function); total cumulative roster 37 → 58. ~+150-220 LoC. Lands at Task 2. |
| `internal/wasm/sandbox_test.go` | EXTEND | Per-NEW-key ALLOW/DENY exhaustive table-driven; gate-at-registration assertion per R-25.2-5 (deny `proxy_set_tick_period_milliseconds` → assert host function NOT registered on wazero Runtime via integration with registration.go). ~+200-300 LoC. Lands at Task 2. |
| `internal/wasm/registration.go` | EXTEND | 14 NEW env-namespace hostcall registrations per §5.1 + 7 NEW callback dispatch entries per §5.3; gate-at-`registerCallback`-time discipline per AMEND-B5 — for each NEW capability key, if `vm.sandbox.IsAllowed(key)` returns false, do NOT register the host function on the wazero Runtime (denied capabilities → guest's import resolution fails at module-instantiation OR runtime trap fires on call; matches upstream cpp-host `wasm.cc:176-189` `_REGISTER_PROXY` macro). Buffer-clamp wire-contract per AMEND-B1 lives at the `proxy_get_buffer_bytes` host shim (delegates to `abi/body_bridge.go`). ~+250-400 LoC. Lands at Task 3. |
| `internal/wasm/registration_test.go` | EXTEND | 14 NEW hostcall registration round-trip tests + 7 NEW callback dispatch tests + gate-at-registration assertions per R-25.2-5 — assert capability-denied hostcall is NOT in the wazero `*Module`'s import set. ~+200-300 LoC. Lands at Task 3. |
| `internal/wasm/abi/types.go` | UNCHANGED at 25.2 | Per §2.21 + §3.5 — existing enums cover the 25.2 surface; WasmBufferType values 0/1/4 + WasmHeaderMapType values 1/3 ACTIVATED via host-module registration extensions in Task 3 (no code change at abi/types.go). |
| `internal/wasm/abi/body_bridge.go` | NEW | proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_status dispatch per §5.1 #25-27. AMEND-B1 buffer-clamp wire-contract per R-25.2-1: `proxy_get_buffer_bytes` clamps silently on `start + max_size > buffer.length` (returns `WasmResult::Ok` with truncated length); only `start + max_size` i32-overflow returns `BadArgument`. WasmBufferType values 0 (HttpRequestBody) + 1 (HttpResponseBody) + 4 (HttpCallResponseBody) ACTIVATED. Delegates body accumulation reads to ABICallbacks via `internal/filter/http/wasm/body.go` (consumer-side adapter). ~150-220 LoC. Lands at Task 4. |
| `internal/wasm/abi/body_bridge_test.go` | NEW | AMEND-B1 buffer-clamp golden table per R-25.2-1: (a) start_in_bounds + max_in_bounds → Ok with full length; (b) start_in_bounds + max_overflows → Ok with truncated length (clamp); (c) start_at_end + max_anything → Ok with length=0; (d) start_beyond_end → Ok with length=0; (e) start+max_size i32-overflow → BadArgument; (f) round-trip via abi callbacks. ~200-300 LoC. Lands at Task 4. |
| `internal/wasm/abi/stream_control.go` | NEW | proxy_continue_stream + proxy_close_stream dispatch per §5.1 #28-29. Paired with PAUSE-buffer dispatch on body callbacks; stream-type discriminator (HttpRequest=0, HttpResponse=1, HttpUpstream=2). ~80-120 LoC. Lands at Task 4. |
| `internal/wasm/tick.go` | NEW | Per-RootVM tick goroutine per Q5 + R-25.2-9 + ADR-0205. `for { select { case <-clock.After(effectivePeriod): rootVM.lockAndCall(proxy_on_tick, rootCtxID); case <-stop: return } }`. `effectivePeriod = max(period_ms, 10ms)` per Q5 envoy-go-strict floor. Uses ADR-0186 `Clock` seam injection at NewRootVM time via `WithRootClock` for fixture fake-time support. SetTickPeriod(period) re-schedules with the given period (period=0 cancels). FIRST co-consumer of phase-21 ADR-0186 Clock seam beyond phase-21 itself — RATIFIES the phase-21 extraction. ~200-300 LoC. Lands at Task 5. |
| `internal/wasm/abi/timer.go` | NEW | proxy_set_tick_period_milliseconds dispatch per §5.1 #30. Validates period_ms (must be >= 0); delegates to `*RootVM.SetTickPeriod(time.Duration(period_ms) * time.Millisecond)`. ~80-120 LoC. Lands at Task 5. |
| `internal/wasm/tick_test.go` | NEW | Tick goroutine + Clock seam fake-time tests using `clock.FakeClock` from phase-21 `internal/clock`; 10ms floor enforcement (set period=5ms → assert effectivePeriod=10ms → assert tick fires at 10ms intervals on fake clock); set period=0 cancels (assert tick goroutine receives stop signal); set period=50ms after period=10ms re-schedules; concurrent stream contexts share one tick goroutine (no per-stream tick storm); panic-recovery in tick callback (proxy_on_tick panics → panic-wrapper recovers + log + envoy_go.failures counter increment + tick goroutine survives). ~250-400 LoC. Lands at Task 5. |
| `internal/wasm/shared_data.go` | NEW | Per-RootVM CAS-protected K-V map per Q6 + R-25.2-10. sharedDataEntry struct {value []byte; cas uint32}; `sharedData map[string]sharedDataEntry`; sync.RWMutex. CAS semantic byte-exact from cpp-host: cas=0 unconditionally writes (returns new CAS value in subsequent get); cas>0 writes only if existing entry's CAS matches (returns WasmResult::CasMismatch on mismatch). envoy-go-strict caps: per-value 1 MiB cap (configurable via envoy_go_strict_shared_data_value_cap_bytes default 1048576); 1024-entry cap (configurable via envoy_go_strict_shared_data_max_entries default 1024). Cap exceeded returns WasmResult::InternalFailure + shared_data_cap_exceeded envoy-go-strict counter + envoy_go.failures counter increment per §2.25 + integration error log. ~200-280 LoC. Lands at Task 6. |
| `internal/wasm/abi/shared_data.go` | NEW | proxy_get_shared_data + proxy_set_shared_data dispatch per §5.1 #35-36. Delegates to `*RootVM.SetSharedData` / `*RootVM.GetSharedData`. ~100-150 LoC. Lands at Task 6. |
| `internal/wasm/shared_data_test.go` | NEW | CAS golden table per R-25.2-10: cas=0 always writes; cas>0 writes only on match; mismatch returns CasMismatch + cas value unchanged; cap-boundary tests: value-cap-exceeded (value > 1 MiB) → InternalFailure + counter + envoy_go.failures bump; entry-cap-exceeded (entry count > 1024) → InternalFailure + counter + envoy_go.failures bump; concurrent-Set stress test (N=100 goroutines under sync.RWMutex; -race clean). ~300-450 LoC. Lands at Task 6. |
| `internal/wasm/foreign.go` | NEW | ForeignFunctionRegistry per AMEND-A9 + R-25.2-8 + D-25.2-P3 PLAN-time closure. `ForeignFunctionFn func(ctx context.Context, args []byte) (result []byte, status WasmResult)`. ForeignFunctionRegistry struct {mu sync.RWMutex; fns map[string]ForeignFunctionFn}. NewForeignFunctionRegistry constructor. Register(name, fn) error (returns error if name already registered). Get(name) (ForeignFunctionFn, bool) under RLock. Process-global var `DefaultForeignFunctionRegistry = NewForeignFunctionRegistry()` consumed by wasm filter factory. EMPTY default registry per AMEND-A9 — envoy-go-strict departure record #5 at BEHAVIOR_CONTRACT.md. ~150-220 LoC. Lands at Task 7. |
| `internal/wasm/abi/foreign.go` | NEW | proxy_call_foreign_function dispatch per §5.1 #38. Looks up function name in per-RootVM foreign-function registry view (RLock); if not registered → returns WasmResult::NotFound (=1) byte-faithful to upstream cpp-host `src/exports.cc:147-184` + foreign_function_denied envoy-go-strict counter increment. If registered → invokes function synchronously inside the per-stream call frame (mutex-per-RootVM concurrency model per D-25.2-P3 — RootVM dispatch lock held during invocation; panic-recovery wrapper applies). ~120-180 LoC. Lands at Task 7. |
| `internal/wasm/foreign_test.go` | NEW | ForeignFunctionRegistry Register/Get + EMPTY-default NotFound behavior; capability-gated default-deny (proxy_call_foreign_function denied → InternalFailure per ADR-0204 inherited); registered-then-deregister-then-call sequence; D-P3 closure verification: concurrent dispatch from N streams to same registered function — verify mutex-per-RootVM serialization + no cross-stream argument leak (the function executes inside the per-stream call frame, NOT the root context). ~250-380 LoC. Lands at Task 7. |
| `internal/wasm/http_call.go` | NEW | proxy_http_call dispatch via per-RootVM `*httpclient.Client` per Q4 + R-25.2-3 + AMEND-B3. AsyncClient request lifecycle; call_id monotonic allocation; httpCalls map tracking {call_id → pendingHttpCall{streamCtxID, deadline}}; cancel-at-destruction via `*StreamContext.Close` → iterate httpCalls + filter by streamCtxID + invoke `httpclient.Cancel(handle)` for each outstanding call; defensive token-miss guard at response arrival — if `httpCalls[call_id]` is absent OR the originating streamCtxID's stream context is gone → http_call_response_after_close envoy-go-strict counter increment + drop (NO host-side panic). BadArgument-on-unknown-cluster per Q4 — cluster lookup fails → return BadArgument + http_call_dispatch_unknown_cluster counter increment. Anchored at parent §13-R6 closure (RE-CONSUMES phase-20 ADR-0177 at 3rd-or-later co-consumer; NO API extension on httpclient — phase-22.2 cluster-based dispatch covers byte-for-byte). ~300-450 LoC. Lands at Task 8. |
| `internal/wasm/abi/http_call.go` | NEW | proxy_http_call host shim per §5.1 #37 — pairs decode for headers/trailers payload per 25.1 `internal/wasm/pairs.go` + delegate to `*RootVM.DispatchHttpCall(streamCtxID, cluster, headers, body, trailers, timeout_ms)`. proxy_on_http_call_response callback dispatch — when response arrives async, route to the originating `*StreamContext.CallProxyOnHttpCallResponse(call_id, num_headers, body_size, num_trailers)` via the RootVM's httpCalls map lookup. ~150-220 LoC. Lands at Task 8. |
| `internal/wasm/http_call_test.go` | NEW | proxy_http_call dispatch with mock `*httpclient.Client` + cluster_b second-upstream-cluster scenario; cancel-at-destruction race-test (StreamContext.Close cancels in-flight requests + late-response-after-close path → http_call_response_after_close counter increment + defensive token-miss guard); BadArgument on unknown cluster + http_call_dispatch_unknown_cluster counter; concurrent N=100 httpCall dispatches from same RootVM verify call_id allocation isolation + response routing isolation. ~400-550 LoC. Lands at Task 8. |
| `internal/filterstate/filterstate.go` | NEW | Per ADR-0207 + R-25.2-6 + Q7 + AMEND-B4. Generic per-stream filter-state primitive at consumer-#2 scope. `FilterStateObject interface { Marshal() ([]byte, error); Unmarshal([]byte) error; HasData() bool; StateType() StateType }`. `StateType` const (StateTypeReadOnly=0; StateTypeMutable=1). `Bucket struct { mu sync.RWMutex; items map[string]FilterStateObject }`. NewBucket constructor. Set(key, obj) error (mutable overrides; read-only with same key as Mutable → reject). Get(key) (FilterStateObject, bool). Keys() []string (for property-tree enumeration). EXPLICIT API-REVISION ALLOWANCE clause anchored at ADR-0207 §Decision body at Task 22 (the primitive's API is provisional at consumer #2; future consumers MAY require API revision). ~250-400 LoC. Lands at Task 9. |
| `internal/filterstate/filterstate_test.go` | NEW | Set/Get/Keys round-trip + read-only-vs-mutable conflict + nil-handling + Marshal/Unmarshal round-trip. ~250-380 LoC. Lands at Task 9. |
| `internal/filterstate/bucket_concurrency_test.go` | NEW | RWMutex discipline + concurrent-read concurrent-add tests (-race clean under N=100 goroutines). ~150-220 LoC. Lands at Task 9. |
| `internal/filterstate/filterstateobject_test.go` | NEW | Interface conformance + edge cases (HasData false on empty; StateType discriminator consistency). ~150-220 LoC. Lands at Task 9. |
| `internal/filter/http/lua/filterstate.go` | REWRITE | Per ADR-0207 MIGRATION + §3.4. The phase-22.2 `internal/filter/http/lua/filterstate.go` REWRITES non-breaking to delegate to `internal/filterstate/*Bucket` via a thin adapter. The `:filterState()` Lua surface stays UNCHANGED (the `:filterState():get(name)` + `:filterState():set(name, value)` accessors delegate to `*Bucket.Get` / `*Bucket.Set`). The 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 (mutation exposure + typed Lua-value marshaling) carry forward unchanged. Migration delta ~50-100 LoC inside `internal/filter/http/lua/`. Lands at Task 10. |
| `internal/stats/dynamic/dynamic.go` | NEW | Per ADR-0208 + AMEND-B2 + R-25.2-7. Thin wrapper over `internal/stats/` registry for `wasmcustom.<custom_name>` dynamic-stats namespace. `MetricID uint32` opaque token. `MetricType int` const: MetricTypeCounter=0 + MetricTypeGauge=1 + MetricTypeHistogram=2 per AMEND-B2. `Registry struct { mu sync.RWMutex; pluginScope *stats.Scope; maxEntries uint32; byID map[MetricID]registryEntry; byName map[string]MetricID; nextID MetricID }`. NewRegistry(pluginScope, maxEntries) — pluginScope = `stats.RootScope.Subscope("wasm").Subscope(pluginName)`. Register(metricType, name) (MetricID, error) — idempotent (returns cached MetricID if name already registered); allocates new MetricID otherwise; cap-exceeded → ErrCapExceeded. Increment(id, delta int64) — signed delta per AMEND-B2; ErrBadArgument on Histogram. Record(id, value uint64) — unsigned value per AMEND-B2; ErrBadArgument on Counter. Get(id) (uint64, error). EnumerateForAdmin walks registry for /stats lazy enumeration. ~300-450 LoC. Lands at Task 11. |
| `internal/stats/dynamic/dynamic_test.go` | NEW | Register/Increment/Record/Get round-trip + signed-delta semantics (delta=-1; delta=int64 min/max) + idempotent-Register (re-register same name → same MetricID) + ErrCapExceeded threshold (1024 → 1025) + ErrBadArgument enforcement (Increment on Histogram; Record on Counter); MetricType byte-pin per AMEND-B2 (Counter=0, Gauge=1, Histogram=2). ~300-450 LoC. Lands at Task 11. |
| `internal/stats/dynamic/dynamic_admin_test.go` | NEW | Admin /stats enumeration round-trip + name format `wasmcustom.<custom_name>` byte-pin per R-25.2-2 (under per-plugin scope = `wasm.<plugin>.wasmcustom.<custom_name>` at admin /stats; in-wire = `wasmcustom.<custom_name>` byte-faithful to upstream). ~200-300 LoC. Lands at Task 11. |
| `internal/stats/dynamic/dynamic_concurrency_test.go` | NEW | RWMutex discipline + concurrent-Register stress test at cap-boundary race (N goroutines racing to register near 1024 cap; verify exactly 1024 succeed). ~150-220 LoC. Lands at Task 11. |
| `internal/wasm/dynamic_stats.go` | NEW | Wraps per-plugin `*dynamic.Registry` constructed at config-load by `*compiledConfig`. proxy_define_metric dispatch + signed-i64 delta plumbing per AMEND-B2; 1024-entry cap; dynamic_stats_cap_exceeded counter + envoy_go.failures counter increment on cap-exceeded per §2.25. ~150-220 LoC. Lands at Task 12. |
| `internal/wasm/abi/metrics.go` | NEW | proxy_define_metric (host shim allocates from per-plugin dynamic.Registry; returns MetricID as proxy-wasm `i32` cast from uint32) + proxy_increment_metric (signed-i64 delta dispatch) + proxy_record_metric (unsigned-u64 value dispatch) + proxy_get_metric dispatch per §5.1 #31-34. ~150-220 LoC. Lands at Task 12. |
| `internal/wasm/dynamic_stats_test.go` | NEW | proxy_define_metric + Increment + Record + Get round-trip; idempotent re-Register; 1024-entry cap-boundary test; dynamic_stats_cap_exceeded counter assertion; envoy_go.failures co-increment per §2.25 verification. ~300-450 LoC. Lands at Task 12. |
| `internal/wasm/abi/metrics_test.go` | NEW | MetricType enum byte-pin per AMEND-B2 + R-25.2-2 (Counter=0, Gauge=1, Histogram=2); ErrBadArgument enforcement on cross-type operations. ~150-220 LoC. Lands at Task 12. |
| `internal/wasm/property.go` | NEW | Full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4. NUL-delimited path parsing: `parsePathSegments(path []byte) []string` splits on 0x00; empty segment → NotFound; trailing NUL tolerated. Per-root dispatch (~10 dispatched roots + 4 direct tokens). Co-consumed primitive mapping per AMEND-B4: stream-local accessors for request/response/source/destination/wasm-direct-tokens; RE-CONSUMES ADR-0144 DownstreamPrincipal for connection.tls.*; RE-CONSUMES ADR-0177 httpclient for upstream.* + xds.cluster_*; RE-CONSUMES ADR-0190 dynamicmetadata for metadata.* + xds.*_metadata; CONSUMES NEW internal/filterstate/ for filter_state.* + upstream_filter_state.* + wasm.<key> proxy branches. Absent-property returns WasmResult::NotFound (=1) byte-faithful to upstream `context.cc:1065/1072/1078/1083/1103/1106/1110`. ~450-650 LoC. Lands at Task 13. |
| `internal/wasm/property_test.go` | NEW | Table-driven tests for ~70 sub-paths per AMEND-B4: each row is `{rootName string, subPath []byte, wantBytes []byte, wantStatus WasmResult}` covering request 16 + response 6 + connection 12 + source 2 + destination 2 + upstream 14 + xds 12 + metadata + filter_state + upstream_filter_state + wasm.<key> + 4 direct tokens. NUL-delimited path parsing tests (empty path; trailing NUL tolerated; non-NUL separator → NotFound); absent-property NotFound semantics; co-consumed primitive integration round-trip. ~500-700 LoC. Lands at Task 13. |
| `internal/filter/http/wasm/doc.go` | EXTEND | Append 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 + D-25.2-P1..P5 cross-refs + API surface evolution summary. ~+30-50 LoC. Lands at Task 14. |
| `internal/filter/http/wasm/wasm.go` | UNCHANGED at 25.2 | Per §4.1 — TypeURL + filterName + New + filter struct UNCHANGED in shape; the filter struct's vm field replaces with streamCtx (per Task 18 decode_headers EXTEND). |
| `internal/filter/http/wasm/compiled_config.go` | EXTEND | 4 NEW envoy-go-strict-only PluginConfig config fields per Qs 2/6/9 (envoy_go_strict_body_buffer_cap_bytes default 16777216; envoy_go_strict_shared_data_value_cap_bytes default 1048576; envoy_go_strict_shared_data_max_entries default 1024; envoy_go_strict_dynamic_stats_max_entries default 1024); 6 NEW PARSE-REJECT arms per §6.2 (arms 19-23 envoy-go-strict-only config field validators + arm 26 cross-PluginConfig-duplicate-pluginconfig-name); RootVM construction at New() via `wasm.NewRootVM(ctx, cfg.module, cfg.rootContextID, opts...)` — NOT per-stream `wasm.NewVM`; per-plugin `*dynamic.Registry` construction via `dynamic.NewRegistry(pluginScope, cfg.dynStatsMaxEntries)`; per-plugin foreign-function registry view points at `wasm.DefaultForeignFunctionRegistry` by default; the 25.1 `module *wasm.Module + compileCache *wasm.CompileCache + sandbox + pluginName + rootContextID + vmConfig + pluginConfig + stats` UNCHANGED. ~+200-300 LoC. Lands at Task 14. |
| `internal/filter/http/wasm/compiled_config_test.go` | EXTEND | 6 NEW PARSE-REJECT arm table-driven coverage per §6.2 (arms 19/20/21/22/23/26); envoy-go-strict-only config field validators (zero / overlarge); TestParseRejectConstants_ByteStable EXTENDED with 6 NEW byte-stable constants per D-25.2-P5 (final wording finalized at IMPL Task 14 — anticipated per §6.2 table). ~+200-300 LoC. Lands at Task 14. |
| `internal/filter/http/wasm/abi_callbacks.go` | EXTEND | 7 NEW methods per §5.3: OnRequestBody(streamCtxID, bodySize, endOfStream) ProxyAction; OnResponseBody (same signature); OnRequestTrailers(streamCtxID, numTrailers) ProxyAction; OnResponseTrailers (same); OnTick(rootCtxID); OnHttpCallResponse(streamCtxID, callID, numHeaders, bodySize, numTrailers); OnForeignFunction(contextID, foreignFunctionID, dataSize). 4 RE-USE primitive consumer integration: ADR-0144 DownstreamPrincipal for connection.tls.* property branches; ADR-0177 httpclient for upstream.* + httpCall dispatch; ADR-0190 dynamicmetadata for metadata.* + xds metadata branches; NEW ADR-0207 filterstate for filter_state.* property branches. ~+400-600 LoC. Lands at Task 15. |
| `internal/filter/http/wasm/abi_callbacks_test.go` | EXTEND | 7 NEW method coverage + 4 RE-USE primitive round-trip tests (DownstreamPrincipal returns expected sub-symbols on TLS test connection; httpclient mock dispatches to test cluster; dynamicmetadata test injection round-trips through metadata.* property; filterstate Bucket round-trips through filter_state.* property). ~+400-550 LoC. Lands at Task 15. |
| `internal/filter/http/wasm/body.go` | NEW | DecodeData + EncodeData glue per §4.3 + Q1 + Q2. Body-buffer accumulation per-stream — `decodeBody []byte` + `encodeBody []byte` on filter struct + grow on each OnDecodeBuffer/OnEncodeBuffer; cap enforcement via cfg.bodyBufferCapBytes — if accumulated > cap and not already cap-exceeded, set `decodeBodyCapExceeded = true` (sticky) + bump cfg.stats.bodyBufferCapExceeded counter + cfg.stats.envoyGoFailures counter + `decoderCb.SendLocalReply(413, "Payload Too Large", ...)` + return StopAllIteration; else if streamCtx.HasGlobalFunc("proxy_on_request_body") → streamCtx.CallProxyOnRequestBody(ctx, uint32(len(decodeBody)), endStream) + ProxyAction handling; NO-op if proxy_on_request_body not exported (guest doesn't opt into body callbacks). Per Q1: body_size is accumulated total available (NOT just-new-chunk delta); per AMEND-B1 buffer-clamp applied at proxy_get_buffer_bytes shim. ~200-280 LoC. Lands at Task 16. |
| `internal/filter/http/wasm/trailers.go` | NEW | DecodeTrailers + EncodeTrailers glue per §4.3. If streamCtx.HasGlobalFunc("proxy_on_request_trailers") → streamCtx.CallProxyOnRequestTrailers(ctx, numTrailers) + ProxyAction handling. Mirrors encode side. Reuses 25.1 pairs wire-format (HttpRequestTrailers value 1 + HttpResponseTrailers value 3 ACTIVATED). ~120-180 LoC. Lands at Task 16. |
| `internal/filter/http/wasm/tick_clock.go` | NEW | Clock seam injection plumbing per Q5 + R-25.2-9. WithRootClock(clk) option exposed for fixture-0036 fake-time test seam (tick-fires-counter scenario uses `clock.FakeClock` to make tick fires deterministic). Production path uses `clock.RealClock` (default). ~80-120 LoC. Lands at Task 16. |
| `internal/filter/http/wasm/body_test.go` | NEW | Body-buffer accumulation + cap enforcement test + 413-on-exceed dispatch + body_buffer_cap_exceeded counter assertion + envoy_go.failures counter co-increment per §2.25 + sticky cap-exceeded flag (subsequent OnDecodeBuffer calls NO-op after cap exceeded). ~300-450 LoC. Lands at Task 16. |
| `internal/filter/http/wasm/trailers_test.go` | NEW | Trailer hostcall dispatch round-trip + WasmHeaderMapType values 1/3 activation + pairs wire-format reuse + ProxyAction handling. ~200-300 LoC. Lands at Task 16. |
| `internal/filter/http/wasm/property.go` | NEW | Per-stream property resolver dispatch per AMEND-B4. Delegates to `wasm.RootVM.property` tree + the 4 RE-USE primitives. The resolver is per-stream — uses the per-stream streamCtx + accesses the 4 primitives via the per-stream filter callbacks (decoderCb.StreamInfo / encoderCb.StreamInfo for connection + upstream sub-paths; filterCb.GetDynamicMetadata for metadata; per-stream `*filterstate.Bucket` for filter_state). ~250-400 LoC. Lands at Task 17. |
| `internal/filter/http/wasm/stats.go` | EXTEND | 9 NEW envoy-go-strict counters per §7.1 + AMEND-B3: tickInvocations + httpCallDispatched + httpCallResponse + foreignFunctionDenied + bodyBufferCapExceeded + httpCallDispatchUnknownCluster + sharedDataCapExceeded + dynamicStatsCapExceeded + httpCallResponseAfterClose; project stat count 119 → 128. Per-plugin `*dynamic.Registry` field added to filterStats (NOT a counter — the Registry instance itself; the namespace `wasmcustom.<custom_name>` populated lazily via proxy_define_metric calls). ~+100-150 LoC. Lands at Task 17. |
| `internal/filter/http/wasm/property_test.go` | NEW | Per-stream property resolver dispatch coverage for ~70 sub-paths per AMEND-B4; co-consumed primitive round-trips; absent-property NotFound semantics. ~300-450 LoC. Lands at Task 17. |
| `internal/filter/http/wasm/decode_headers.go` | EXTEND | Per §4.3 — per-stream construction goes through cfg.rootVM.NewStreamContext(ctx) (NOT per-stream wasm.NewVM). The filter struct's vm field RENAMES to streamCtx (`*wasm.StreamContext`). Same ProxyAction handling + captured-local-response handoff as 25.1; OnDestroy delegates to `streamCtx.Close(ctx)` (which fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels outstanding httpCalls per AMEND-B3). ~+50-80 LoC. Lands at Task 18. |
| `internal/filter/http/wasm/encode_headers.go` | EXTEND | Per-stream context shared with decode side — filter struct's single streamCtx used for both decode + encode. Same ProxyAction handling as 25.1. ~+30-50 LoC. Lands at Task 18. |
| `internal/filter/http/wasm/dispatch_test.go` | EXTEND | Body/trailer/tick/httpCall integration round-trips; per-stream concurrency tests under root-VM model (N=100 stream contexts on one RootVM; verify shared-data writes from one stream visible to another via stat assertion; verify httpCall response from one stream NOT visible to another). ~+200-300 LoC. Lands at Task 18. |
| `internal/filter/http/wasm/fuzz_hostcall_test.go` | NEW | 35th project-wide fuzzer FuzzWasmHostcallEnvelope per §8.4 + R-25.2-12. Must-never-panic across the 14 NEW hostcall envelope surfaces + foreign-function dispatch + dynamic-stats Register + shared-data CAS race + body-buffer cap boundary + property-path NUL-delimited adversarials. Corpus seeds at `testdata/fuzz/FuzzWasmHostcallEnvelope/` (~30-40 seeds per D-P-PLAN-10). Lands at Task 19. ~150-220 LoC. |
| `internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/` | NEW corpus dir | ~30-40 corpus seeds per D-P-PLAN-10 across 10 dimensions; each seed a stand-alone Go fuzz corpus file. Lands at Task 19. |
| `internal/filter/http/wasm/wasm_bench_test.go` | EXTEND | BenchmarkPerStreamModule_Instantiation per R8 gate + D-25.2-P2. Measures per-stream Module instantiation cost under the new root-VM model (constructs N=b.N fresh stream contexts on a shared `*RootVM` + `*Module`; reports ns/op). Threshold gate per parent §13-R8 + D-P-PLAN-11: if ns/op > 1_000_000 (1ms), ADR-0209 escape-valve FIRES at Task 22 (§Context + §Decision + §Consequences body all land at the same Task 22 commit per ADR-0044 anchoring "pooled vs shared-Module-with-mutex-serialization" decision). If ns/op <= 1_000_000, WEAK-default fresh-per-stream STANDS; ADR-0209 STAYS UNCONSUMED. Anticipated answer per D-P-PLAN-11 + §2.16: WELL UNDER 1ms threshold (the root-VM model shrinks per-stream cost vs 25.1's 61µs; per-stream now ~microseconds for context creation + Module instantiation). Benchmark output quoted verbatim in Task 22 PROGRESS.md entry. ~+50-80 LoC. Lands at Task 22. |
| `cmd/envoy-go/main.go` | UNCHANGED at 25.2 | Per §3.7 — 20 HTTP filters wired UNCHANGED at 25.2; existing `httpReg.Register(wasm.TypeURL, wasm.New)` call serves the 25.2 surface via `wasm.New` returning the EXTENDED filter factory. NO boot-registration delta. |
| `test/differential/fixture/fixture.go` | MODIFY | NEW BackendKind=HTTPWasmAdvanced enum value (OR REUSE existing HTTPWasm — settles at IMPL Task 20 first-action; anticipated NEW per the per-fixture-dir-1-runner-branch discipline + assertion-class partitioning differences from 25.1 HTTPWasm) ~+15 LoC. Lands at Task 20. |
| `test/differential/runner_test.go` | MODIFY | +blank import for fixture-0036; +switch-case for HTTPWasmAdvanced (or HTTPWasm if REUSED) ~+15 LoC — Task 20. +blank import for fixture-0037 ~+5 LoC — Task 21. Fixture-0037 runner-branch shape settles at IMPL Task 21 (extend BootRejectFixture with `subjectOnly: true` flag — anticipated; alternative NEW SubjectOnlyBootRejectFixture). |
| `test/fixtures/0036-http-wasm-body-and-advanced/README.md` | NEW | Top-level fixture-directory README — scope + 14-scenario table + topology (2 upstream clusters cluster_a + cluster_b) + cross-refs to SPEC §8.1 + ADR-0205+0206+0207+0208. ~200-300 LoC. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/envoy.yaml` | NEW | Reference Envoy bootstrap; single listener + wasm filter + 2 upstream clusters cluster_a (primary) + cluster_b (httpCall target — both bound to SAME differential echobackend); templated {{.BackendPort}} + {{.UpstreamBPort}}. ~200-300 LoC. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/envoy-go.yaml` | NEW | Subject bootstrap; same topology + 4 envoy-go-strict-only PluginConfig extension fields populated. ~200-300 LoC. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/expectations.yaml` | NEW | Human-readable declarative scenario expectations (NOT consumed by runner; documentation aid). ~120-200 LoC. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/inputs/driver.go` | NEW | Registered Driver impl ~800-1200 LoC. Per-scenario probes via driveProxy + emitScenario + classifyBody for the 10 cross-side scenarios (a)-(j) (use existing `CompareBytes`); per-subject-only-scenario StatsAsserter.AssertStats implementations for (k) tick-fires-counter (assert tick_invocations >= 5 + wasmcustom.tick_count >= 5 after 250ms probe wait) / (l) httpCall-success (assert http_call_dispatched + http_call_response + x-httpcall-status:200 response header) / (m) httpCall-unknown-cluster (assert http_call_dispatch_unknown_cluster + x-httpcall-result:2) / (n) body-cap-exceeded (assert body_buffer_cap_exceeded + envoy_go.failures + HTTP 413 + sticky cap-exceeded flag). Deliberate-break liveness verification mandatory at IMPL Task 20 final-action per `reference_differential_asserter_dispatch` — deliberately break each StatsAsserter assertion → expect FAIL → restore + verify GREEN. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/scripts/{a..n}_<name>/{Cargo.toml,src/lib.rs}` | NEW × 14 | Rust source per Q8 + inherited Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target). 14 scenarios: a_body_read_only / b_body_mutate_passthrough / c_body_mutate_replace / d_trailers_add / e_trailers_read / f_shared_data_rw / g_foreign_function_deny / h_property_stream_info / i_metric_define_only / j_env_vars_rejected / k_tick_fires_counter / l_http_call_success / m_http_call_unknown / n_body_cap_exceeded. ~30 LoC each. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/scripts/README.md` | NEW | Reproduction script + pinned rustup toolchain + cargo build invocation (`rustup target add wasm32-wasip1` + per-scenario `cargo build --release --target wasm32-wasip1`). ~80-120 LoC. Lands at Task 20. |
| `test/fixtures/0036-http-wasm-body-and-advanced/bytecode/{a..n}_<name>.wasm` | NEW × 14 | Vendored pre-built `.wasm` binary blobs committed to git per Q8 + inherited Q9 + AMEND-A1. Lands at Task 20. |
| `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/README.md` | NEW | Boot-reject fixture README — scope + arm 19 disposition (anticipated; D-25.2-P1 final at Task 21 first-action) + subject-only runner-branch shape rationale per D-25.2-P1 + reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field note. ~100-150 LoC. Lands at Task 21. |
| `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy.yaml` | NEW | Reference Envoy bootstrap; deliberately-malformed config (envoy_go_strict_body_buffer_cap_bytes=0 if final arm 19; OR alternative if D-25.2-P1 selects a different arm). Reference side accepts (unknown extension field silently dropped). ~100-150 LoC. Lands at Task 21. |
| `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy-go.yaml` | NEW | Subject bootstrap symmetric. Subject envoy-go boot-rejects on the chosen arm. ~100-150 LoC. Lands at Task 21. |
| `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/inputs/driver.go` | NEW | Implements subject-only BootRejectFixture variant per D-25.2-P1 runner-branch shape decision. Recommended: extend BootRejectFixture with `subjectOnly: true` flag (minimal infrastructure delta); alternative: NEW SubjectOnlyBootRejectFixture (cleaner type-discriminated dispatch). Final disposition at IMPL Task 21 first-action. ~150-250 LoC. Lands at Task 21. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Decision + §Consequences bodies anchored at Task 22 (the §Context drafts already at the 25.2 SPEC commit `f0eae39` per ADR-0044). ADR-0202 §Consequences gains one-line in-place AMEND acknowledgment paragraph per §10.2. CONDITIONAL ADR-0209 if R8 escape-valve fires per D-P-PLAN-11. ~+600-900 LoC delta. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | Task 22 ~7-edit bundle per §13.4 + ADR-0052 atomic landing. ~+350-500 LoC delta. |
| `docs/envoy-go/ROADMAP.md` | MODIFY | Row 25.2 flips `in-progress → done` at Task 22; per-cell IMPL-done annotation per ADR-0106; parent row 25 STAYS in-progress; sub-row 25.3 UNCHANGED planned. ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFY | Rewrite-in-place at Task 22. |
| `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` | NEW | Append-only task log per phase-22.2+25.1 IMPL precedent + `superpowers:verification-before-completion` discipline; 22 task entries + Pre-Task 0; each entry quotes command outputs verbatim + records acceptance-criteria evidence per task + records D-25.2-P1..P5 closure evidence at the relevant Tasks 7/19/20/21/22. ~900-1300 LoC. |
| `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/REVIEW.md` | NEW | Task 22 reviewer artifact per `superpowers:requesting-code-review` per phase-22.2+25.1 IMPL precedent; per-task review notes + cross-cutting review notes + green-light evidence + 46-item SPEC §15 acceptance checklist closure. ~350-450 LoC. |

---

## Planner-time deferred-decision resolution (settles 2 SPEC D-questions per §12 + PLAN-emerged execution decisions)

The 25.2 SPEC §12 D-questions D-25.2-P1..D-25.2-P5 anchor at SPEC commit + close at PLAN/IMPL-time per their §12 anchors: D-25.2-P1 (fixture-0037 single-arm finalization) closes at IMPL Task 21 first-action; D-25.2-P2 (per-stream Module instantiation R8 escape-valve) closes at IMPL Task 22 benchmark gate; **D-25.2-P3 (foreign-function dispatch concurrency model) closes at THIS PLAN session — see D-P-PLAN-9 below**; **D-25.2-P4 (FuzzWasmHostcallEnvelope corpus seed final roster) closes at THIS PLAN session — see D-P-PLAN-10 below**; D-25.2-P5 (BEHAVIOR_CONTRACT.md edit bundle line counts) closes at IMPL Task 22 final. The 11 BRAINSTORM Q-decisions Q1-Q11 are CLOSED at BRAINSTORM commit + 25.2 SPEC §12.6; this PLAN does NOT re-litigate them. The 5 §11 AMEND-B1..B5 empirical-pin REFINEMENTS resolved IN-SESSION at the 25.2 SPEC drafting per ADR-0004 hard-gate; this PLAN does NOT re-execute them. Additional PLAN-emerged execution decisions D-P-PLAN-1..D-P-PLAN-12 settle here.

1. **D-P-PLAN-1 — SPEC §15 46-item acceptance checklist transcribed into a 22-task TDD graph; PROGRESS.md preamble + precondition check is "Pre-Task 0" (NOT a renumbered Task 1).** Settle: the SPEC §15 46-item checklist is the load-bearing input to this PLAN — items 1-12 (framework primitive evolutions) map to Tasks 1-13; items 13-15 (NEW packages + lua MIGRATION) map to Tasks 9 + 10 + 11; items 16-23 (filter package extensions) map to Tasks 14-18; items 24-26 (PARSE-REJECT + stat + departure records) map to Tasks 14 + 17 + 22; items 27-30 (fuzzer + 2 fixtures) map to Tasks 19-21; items 31-35 (wire-shape pins per AMEND-B1..B5) are consumed at code Tasks 4/12/8/13/3 respectively; items 36-38 (ADR landings) map to Task 22; items 39-40 (STATE + ROADMAP + boot-registration) map to Task 22; item 41 (R8 escape-valve gate) maps to Task 22 benchmark; items 42-46 (D-question closures) map to per-task first-actions (D-P1 at Task 21; D-P2 at Task 22; D-P3 at this PLAN; D-P4 at this PLAN; D-P5 at Task 22). The PROGRESS.md preamble + 15-precondition verification is labeled **Pre-Task 0** + executed at IMPL session cold-start before Task 1 begins. Mirrors phase-22.2 + phase-25.1 PLAN precedent. *Anchored: 25.2 SPEC §15 + phase-22.2+25.1 PROGRESS.md ritual precedent.*

2. **D-P-PLAN-2 — Per-task subagent dispatch type LOCKED at `general-purpose` for all 21 code Tasks 1-21; Task 22 atomic landing dispatched via `general-purpose` with explicit acceptance-checklist reference; REVIEW.md via `superpowers:code-reviewer`.** Settle: per project memory `feedback_execution_style.md` (user always wants subagent-driven over inline execution for plans), each Task's IMPL session subagent-dispatches per `superpowers:subagent-driven-development`. Dispatch type per Task: Tasks 1-21 use `general-purpose` agent (Go code work + fixture YAML/Rust authoring at Tasks 20+21); Task 22 uses `general-purpose` with explicit reference to 25.2 SPEC §15 46-item acceptance checklist + the BEHAVIOR_CONTRACT.md ~7-edit bundle anatomy + the ADR-0205+0206+0207+0208 §Decision + §Consequences body sketches from 25.2 SPEC §3 + §5 + §6 + §7 + §9 + §10 + the ADR-0202 one-line AMEND wording from §10.2. REVIEW.md at IMPL Task 22 final step dispatched via `superpowers:code-reviewer`. *Anchored: project memory `feedback_execution_style.md` + phase-22.2+25.1 IMPL precedent + `superpowers:subagent-driven-development` skill.*

3. **D-P-PLAN-3 — Per-task PROGRESS.md entry shape LOCKED per phase-25.1 IMPL precedent.** Settle: each Task's PROGRESS.md entry contains the following sections in order:
   - **Task ID + title** (matches the SPEC §15 acceptance-checklist item set served by the task + this PLAN's task heading verbatim);
   - **Acceptance criteria** (verbatim cross-reference to this PLAN's Task heading's `Acceptance:` line + the SPEC §15 item numbers served);
   - **Files touched** (the precise list from this PLAN's Task heading's `Files:` block);
   - **Verification command outputs** (the exact commands from this PLAN's Task Step bodies' Run-tests-verify-they-pass phase + the verbatim stdout/stderr quoted in fenced code blocks per `superpowers:verification-before-completion` discipline);
   - **Acceptance-criteria evidence** (per-criterion pass/fail with brief reasoning + cross-reference to the verification command output);
   - **D-question disposition update** (if the task closes a D-question — D-25.2-P3 + D-25.2-P4 at this PLAN session; D-25.2-P1 at Task 21; D-25.2-P5 at Task 22; D-25.2-P2 at Task 22 benchmark; the entry records the empirical evidence + resolved disposition);
   - **Commit SHA** (`git log -1 --format=%H` for the task's commit);
   - **Tier + Task-number cross-reference** (e.g., "Tier A internal/wasm/ root-VM evolution (Task 1 of 3 in tier; Task 1 of 22 overall)").
   *Anchored: phase-22.2+25.1 PROGRESS.md format precedent + `superpowers:verification-before-completion` + this PLAN's per-Task structure.*

4. **D-P-PLAN-4 — Per-task TDD ordering LOCKED at test-first for ALL 21 code Tasks (1-21) per `superpowers:test-driven-development` rigid discipline; Task 22 is the atomic-landing meta-task.** Settle: every Task that lands production code (Tasks 1-18; Task 19 fuzzer follows TDD with seed corpus first; Tasks 20+21 fixture bundles follow relaxed test-with-implementation — the differential fixture IS the integration test) follows the rigid TDD ordering: (Step 1) write the failing test in the corresponding `*_test.go` file; (Step 2) run the test to verify it fails; (Step 3) implement the minimal production code; (Step 4) run the test to verify it passes; (Step 5) run `go build ./... + go vet ./... + golangci-lint run` clean; (Step 6) append PROGRESS.md Task entry per D-P-PLAN-3; (Step 7) commit. Tasks 20+21 (fixture bundles) follow: author bootstrap configs + driver.go + Rust sources + vendor `.wasm` blobs + register BackendKind → run `go test ./test/differential -run TestDifferential/0036` (or 0037) → assert GREEN → deliberate-break liveness verification (Task 20 only per `reference_differential_asserter_dispatch`) → append PROGRESS → commit. The Skill's documentation classifies TDD as RIGID — adherence is mandatory. *Anchored: `superpowers:test-driven-development` rigid discipline + phase-22.2+25.1 IMPL precedent.*

5. **D-P-PLAN-5 — `CompileCache` scope INHERITED from 25.1 D-P-PLAN-5 (compiledConfig-instance scope; NOT cross-stream global; NOT cross-listener global).** Settle: the `*wasm.CompileCache` STAYS at `*compiledConfig`-instance scope per the 25.1 D-P-PLAN-5 ratification. The 25.2 evolution adds the per-`*RootVM` shared-data map + httpCalls map + foreign-function registry view + dynamic-stats Registry — these are per-`*RootVM` (= per-`*compiledConfig`) scope, NOT per-stream. No API delta on `CompileCache`. Rationale: (i) the 25.2 single-module-per-compiledConfig discipline carries forward from 25.1; multi-plugin VM-sharing defers to 25.3; (ii) the CompileCache's primary purpose is to forward-pin the API shape for 25.3 when multi-plugin VM-sharing adds multiple modules per listener; (iii) GC-driven eviction matches the project's `sync.Pool`-free precedent. *Anchored: 25.1 D-P-PLAN-5 + 25.2 SPEC §3.5 vm.go UNCHANGED + this PLAN-time emerge.*

6. **D-P-PLAN-6 — 25.1 `internal/wasm/vm.go` DELETED at Task 1 in favor of `root_vm.go` + `stream_context.go` (NO transitional shim).** Settle: the 25.1 per-stream `*VM` type RETIRES at 25.2 — `vm.go` is DELETED outright. There are ZERO transitional consumers: the 25.1 per-stream `*VM` had ONE consumer (`internal/filter/http/wasm/decode_headers.go` per 25.1 SPEC §4.3) which is itself EXTENDED at Task 18 to construct per-stream context via `cfg.rootVM.NewStreamContext(ctx)` instead. The `*VM` type's exported method-set (CallProxyOnContextCreate + CallProxyOnRequestHeaders + CallProxyOnResponseHeaders + CallProxyOnDone + CallProxyOnLog + CallProxyOnDelete + Run + Close + RegisterABICallbacks + State + HasGlobalFunc) migrates to `*StreamContext` (CallProxyOn* + Close + RegisterABICallbacks + HasGlobalFunc) + `*RootVM` (Configure + State + Close); the per-stream Runtime construction is RETIRED. Why delete (vs transitional shim with deprecation): (i) the 25.1 `*VM` has zero external consumers — no compatibility surface to preserve; (ii) per CLAUDE.md "Avoid backwards-compatibility hacks" — a transitional shim adds dead weight; (iii) the migration is atomic at Task 18 — no half-finished state. *Anchored: 25.2 SPEC §3.5 vm.go disposition + CLAUDE.md backwards-compatibility discipline + this PLAN-time emerge.*

7. **D-P-PLAN-7 — Task graph parallelization LOCKED per planner-time emerge.** Settle: after Pre-Task 0 (PROGRESS.md preamble + precondition check) lands, the 22-task graph allows parallelization at multiple points:

   - **After Task 1** (NEW root_vm.go + stream_context.go + DELETE vm.go): Task 2 (sandbox.go EXTEND) can start immediately — file-disjoint within `internal/wasm/`.
   - **After Tasks 1 + 2**: Task 3 (registration.go EXTEND — depends on sandbox.go for new capability key constants + RootVM/StreamContext for host-module wiring). Sequential bottleneck.
   - **After Task 3**: Tasks 4 (abi/body_bridge.go + abi/stream_control.go) + 5 (tick.go + abi/timer.go) + 6 (shared_data.go + abi/shared_data.go) + 7 (foreign.go + abi/foreign.go) + 8 (http_call.go + abi/http_call.go) + 9 (NEW internal/filterstate/) + 11 (NEW internal/stats/dynamic/) can run in PARALLEL (7-way) — each is file-disjoint + each depends only on Task 1-3's RootVM + sandbox + registration scaffolding (the registration.go host-module wiring extension at Task 3 covers all 14 NEW hostcall registrations, but each abi/* dispatch file is independent).
   - **After Task 9** (NEW internal/filterstate/): Task 10 (phase-22.2 lua MIGRATION) — file-disjoint within `internal/filter/http/lua/`; depends on Task 9 filterstate primitive. Can run in PARALLEL with Tasks 4-8 + 11.
   - **After Task 11** (NEW internal/stats/dynamic/): Task 12 (NEW internal/wasm/dynamic_stats.go + abi/metrics.go) — depends on Task 11's `*dynamic.Registry` API.
   - **After Task 9** (filterstate primitive only — the Task 7 partial-dependency hypothesized at SPEC-time is NOT load-bearing because the `wasm.<key>` property class proxies via filter_state → upstream_filter_state per cpp-host `context.cc:987-1019` with NO foreign-function dispatch involvement): Task 13 (NEW internal/wasm/property.go) — depends on Task 9's filterstate primitive for filter_state.* + upstream_filter_state.* + wasm.<key> sub-paths + the existing ADR-0144/0177/0190 primitives for connection/upstream/metadata sub-paths.
   - **After Tasks 4-13**: Task 14 (compiled_config.go EXTEND — 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms + RootVM construction at New). Sequential bottleneck for Tier D.
   - **After Task 14**: Tasks 15 (abi_callbacks.go EXTEND — 7 NEW methods + 4 RE-USE primitive consumers) + 16 (body.go + trailers.go + tick_clock.go NEW) + 17 (property.go NEW + stats.go EXTEND — 9 NEW counters) can run in PARALLEL (3-way) — each is file-disjoint within `internal/filter/http/wasm/`.
   - **After Tasks 15 + 16 + 17**: Task 18 (decode_headers.go + encode_headers.go EXTEND — per-stream construction via RootVM.NewStreamContext; depends on body.go for OnDecodeBuffer hook wiring + trailers.go for OnDecodeTrailers hook wiring + abi_callbacks for the 7 NEW methods). Sequential bottleneck.
   - **After Task 18**: Tasks 19 (fuzzer) + 20 (fixture-0036) can run in PARALLEL (2-way) — Task 19 depends only on Task 14's compiled_config + the 14 NEW hostcall surfaces; Task 20 needs the full filter wired (Tasks 1-18). Task 21 (fixture-0037) lands after Task 20 (runner_test.go blank-import conflict).
   - **After Tasks 19 + 20 + 21**: Task 22 (atomic landing — benchmark + BEHAVIOR_CONTRACT.md ~7-edit bundle + ADR-0205+0206+0207+0208 §Decision+§Consequences + ADR-0202 one-line AMEND + STATE.md re-advance + ROADMAP row 25.2 IMPL-done + CONDITIONAL ADR-0209 if R8 fires + REVIEW.md authoring). Depends on everything.

   **Parallel-dispatch opportunities**: 7-way at Tasks 4+5+6+7+8+9+11; 2-way at Tasks 10+12 (after 9+11 respectively); 1-way at Task 13 (after 9 + most of 7); 3-way at Tasks 15+16+17; 2-way at Tasks 19+20. **Sequential bottlenecks**: Pre-Task 0 → Task 1 → Task 2 → Task 3 → {4,5,6,7,8,9,11}; subset → {10,12,13}; → Task 14 → {15,16,17} → Task 18 → {19,20} → Task 21 → Task 22. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits these parallel opportunities. *Anchored: 25.2 SPEC §15 + §3 file split + this PLAN-time emerge.*

8. **D-P-PLAN-8 — Cross-package regression-test command shape LOCKED.** Settle: after each task lands its production code, the implementer runs the package-local test command (`go test -count=1 -race ./internal/wasm/...` for Tasks 1-8 + 12 + 13; `go test -count=1 -race ./internal/filterstate/...` for Task 9; `go test -count=1 -race ./internal/filter/http/lua/...` for Task 10 — MUST be GREEN without modification per §14.5 non-breaking MIGRATION; `go test -count=1 -race ./internal/stats/dynamic/...` for Task 11; `go test -count=1 -race ./internal/filter/http/wasm/...` for Tasks 14-18; `go test -count=1 ./test/differential -run TestDifferential/0036` for Task 20; `go test -count=1 ./test/differential -run TestDifferential/0037` for Task 21; full `go test -count=1 -race ./...` at Task 22 final Gate C; full `go test -count=1 ./test/differential/...` at Task 22 Gate D — verify 39/39 fixture dirs GREEN). Per 25.2 SPEC §14.10 6-gate checklist: zero regression. *Anchored: 25.2 SPEC §14.10 + phase-25.1 D-P-PLAN-9 precedent + this PLAN-time emerge.*

9. **D-P-PLAN-9 — D-25.2-P3 CLOSED at this PLAN session: foreign-function dispatch concurrency model = mutex-per-RootVM (synchronous dispatch inside per-stream call frame; RootVM dispatch lock held during invocation; ForeignFunctionRegistry uses sync.RWMutex on registry map with RLock on Get; panic-recovery wrapper applies).** Settle: per 25.2 SPEC §12 D-25.2-P3 + the cpp-host reference model (cpp-host uses sync.Mutex held during dispatch; the foreign function executes synchronously inside the dispatch frame). envoy-go follows: (a) `internal/wasm/foreign.go` `*ForeignFunctionRegistry.Get(name)` holds an `mu.RLock()` only (read-only access — the registry is mutated only at boot via `Register` calls; runtime Get traffic is the common case + the RLock allows concurrent Get from multiple goroutines); (b) the dispatched `ForeignFunctionFn` executes synchronously inside `*RootVM.CallForeignFunction(ctx, streamCtxID, name, args)` (no goroutine offload at 25.2; the function's compute cost is the operator's responsibility; if compute-heavy foreign functions emerge, future scope MAY add an opt-in async dispatch surface via API revision per ADR-0207 EXPLICIT API-REVISION ALLOWANCE — but at 25.2 dispatch is synchronous); (c) the `*RootVM` lock IS held during dispatch (same lock as per-stream call frame — the per-`*StreamContext` call frame holds the `*RootVM` dispatch lock for the duration of the proxy_on_* invocation; foreign-function calls are nested INSIDE that frame so the lock is already held; no additional lock acquired); (d) panic-recovery wrapper applies (same wrapper as other host-side callbacks — Go panic in `ForeignFunctionFn` → recover() → log + envoy_go.failures counter increment + return WasmResult::InternalFailure to guest; the wrapper lives inside `*RootVM.CallForeignFunction` per the panic-wrapper discipline from 25.1 vm.go); (e) the `foreign_function_denied` envoy-go-strict counter increments on the NotFound path (unregistered name) per AMEND-A9. Test coverage at `internal/wasm/foreign_test.go` (Task 7) verifies concurrent dispatch from N=100 streams to same registered function via `*RootVM.CallForeignFunction` — assert no cross-stream argument leak (each call receives the args it passed); assert mutex-per-RootVM serialization (concurrent calls observe the lock contention via a probe counter the test function increments). **ALTERNATIVE REJECTED**: event-loop-per-RootVM — adds non-trivial complexity (Go goroutine pool + select-loop dispatch) for a use case that doesn't yet exist (no operator has requested async foreign-function dispatch); YAGNI per CLAUDE.md. **ALTERNATIVE REJECTED**: caller-goroutine (dispatch from the per-stream goroutine directly without crossing the RootVM lock) — breaks the upstream byte-faithful semantic (cpp-host serializes via the Wasm-level lock; envoy-go must mirror to keep guest behavior identical); risk of guest-observable concurrency divergence is unacceptable. *Anchored: 25.2 SPEC §12 D-25.2-P3 + cpp-host reference model + AMEND-A9 + this PLAN-time scrape.*

10. **D-P-PLAN-10 — D-25.2-P4 CLOSED at this PLAN session: FuzzWasmHostcallEnvelope corpus seed roster (35 seeds across 10 dimensions).** Settle: per 25.2 SPEC §8.4 + ADR-0018 baseline. The corpus seeds at `internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/` materialize the following 10 dimensions; per-dimension seed count enumerated:

    | Dim | Description | Seed count | Seed shape |
    |---|---|---|---|
    | 1 | Hostcall argument-envelope edge cases (proxy_get_buffer_bytes start/max combinations per AMEND-B1) | 5 | (start=0, max=0); (start=0, max=u32::MAX); (start=u32::MAX, max=1); (start=u32::MAX, max=u32::MAX) i32-overflow; (start=10, max=u32::MAX) clamp |
    | 2 | proxy-wasm pairs serialization adversarial (re-uses 25.1 pairs.go corpus + trailer-map extension) | 4 | truncated pair header; malformed key/value sizes; reused-key duplicate pairs; max-size headers payload |
    | 3 | Foreign-function call name length boundary | 3 | name=empty bytes; name=1024 bytes; name=u16::MAX bytes |
    | 4 | Dynamic-stats name validation | 4 | name=empty; name with NUL byte; name with non-UTF-8 bytes; name=1024-entry cap-boundary trigger |
    | 5 | Shared-data CAS-mismatch race patterns | 3 | cas=0 race; cas=u32::MAX race; key=empty bytes |
    | 6 | Body-buffer cap boundary cases (per AMEND-B1) | 3 | exactly-at-cap (16 MiB); one-byte-over-cap (16 MiB + 1); one-byte-under-cap |
    | 7 | Property-path NUL-delimited adversarial (per AMEND-B4) | 4 | malformed NUL-delimited (no terminator); empty segment (NUL NUL); >MAX_PATH depth (100 levels); unknown root |
    | 8 | Tick period parsing (per Q5 envoy-go-strict floor) | 3 | period=0 (cancel); period=1ms (below floor → clamp to 10ms); period=i64::MAX |
    | 9 | httpCall envelope adversarial | 4 | cluster_name=empty; headers wire malformed; timeout=0; timeout=u32::MAX |
    | 10 | Metric type out-of-range + signed-i64 delta extremes (per AMEND-B2) | 2 | MetricType=99 → expect ErrBadArgument; delta=i64::MIN |
    | **Total** | | **35** | |

    **Must-never-panic invariant:** any of these inputs to the host-side hostcall dispatch MUST NOT crash the envoy-go process (must return WasmResult error code + log + continue). Clean at 30s per seed. Project-wide fuzzer count post-25.2: **34 → 35** (per §8.5 D-S2 closure at 25.2 SPEC commit). *Anchored: 25.2 SPEC §8.4 + ADR-0018 baseline + this PLAN-time emerge.*

11. **D-P-PLAN-11 — `BenchmarkPerStreamModule_Instantiation` LOCKED at Task 22 with explicit > 1ms threshold gating per parent §13-R8 + D-25.2-P2.** Settle: Task 22 (atomic landing) ALSO includes the benchmark `BenchmarkPerStreamModule_Instantiation` at `internal/filter/http/wasm/wasm_bench_test.go` measuring per-stream Module instantiation cost under the new root-VM model (constructs N=b.N fresh stream contexts on a shared `*RootVM` + `*Module` back-to-back; reports `ns/op` via `b.N` discipline). The threshold gate per parent §13-R8 + D-25.2-P2: if `ns/op > 1_000_000` (= 1ms), the ADR-0209 escape-valve FIRES at Task 22; ADR-0209 §Context + §Decision + §Consequences body all land at the same Task 22 commit per ADR-0044 anchoring a "pooled vs shared-Module-with-mutex-serialization" decision (the decision body settles between pooled-Module-with-pre-instantiated-entries vs shared-Module-with-mutex-serialization-on-each-call based on empirical evidence). If `ns/op <= 1_000_000`, the WEAK-default fresh-per-stream Module instantiation STANDS; no ADR-0209 fires; next-free ADR-0209 stays UNCONSUMED carried forward to 25.3 BRAINSTORM as the 25.3 IMPL escape-valve slot. The benchmark result quoted verbatim in Task 22 PROGRESS.md entry. **Anticipated answer per D-25.2-P2 + 25.2 SPEC §2.16**: WELL UNDER 1ms — the 25.2 root-VM model retires per-stream `*wazero.Runtime` construction (25.1's 61µs); per-stream cost shrinks to microseconds for context creation + Module instantiation. The new per-stream cost surfaces (body buffer alloc + property-tree lookups) are anticipated at low microseconds each. ADR-0209 anticipated UNCONSUMED. *Anchored: parent §13-R8 + 25.2 SPEC §13.1 R8 row + §2.16 + this PLAN-time emerge.*

12. **D-P-PLAN-12 — Vendored .wasm bytecode reproduction discipline INHERITED from 25.1 fixture-0034 scripts/ pattern.** Settle: the 14 fixture-0036 scenario plugins + 1 fixture-0037 boot-reject plugin (if needed — boot-reject typically uses inline malformed config, not a plugin .wasm) follow the 25.1 fixture-0034 reproduction discipline byte-for-byte. Each `scripts/<scenario>/{Cargo.toml,src/lib.rs}` directory contains the Rust source (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target per AMEND-A1); `scripts/README.md` pins the rustup toolchain (`rustup target add wasm32-wasip1` + recent stable Rust per the 25.1 IMPL Task 15 pinning) + lists the `cargo build --release --target wasm32-wasip1` invocation per scenario. The compiled `.wasm` binaries are vendored to `bytecode/<scenario>.wasm` committed to git per Q8 + inherited Q9 + AMEND-A1. Per the 25.1 fixture-0034 precedent at SPEC §9.1: vendored bytecode is a one-time-author + diff-on-rebuild pattern (no CI build of the plugins — the differential harness consumes the vendored blobs directly). The 14 scenario plugins reuse the proxy-wasm-rust-sdk imports + the `proxy_wasm::traits::HttpContext` + `RootContext` patterns from the 25.1 fixture-0034 7-scenario reproduction. Tick scenario (k) uses `proxy_wasm::types::Bytes` for the `wasmcustom.tick_count` dynamic-stats define + increment. HttpCall scenarios (l) + (m) use `self.dispatch_http_call(...)` from the SDK. Foreign-function scenario (g) uses `self.call_foreign_function(...)` from the SDK. *Anchored: 25.1 fixture-0034 scripts/+bytecode/ discipline + AMEND-A1 Rust toolchain pin + Q8 + this PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The 25.2 IMPL lands 4 ADR §Decision + §Consequences bodies at Task 22 atomic landing per ADR-0044 (the §Context drafts already anchored at the 25.2 SPEC commit `f0eae39` per SPEC §10.1); 1 in-place §Consequences AMEND acknowledgment on ADR-0202 at Task 22 per §10.2 (NO new ADR number consumed); 1 CONDITIONAL ADR landing at Task 22 only if R8 escape-valve fires per D-P-PLAN-11. **NO new ADRs consumed at any task before Task 22.** The ADR-0125 §canonical-per-route-roster STAYS at 10 across all of phase 25 per AMEND-A3 (NO §(xvi) amendment); NO in-place ADR-0125 amendment at this PLAN commit + at Task 22.

| ADR | Subject (25.2 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0205** | Root VM lifecycle evolution per Q3 — ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per cpp-host `Wasm`/`Plugin` model); per-stream contexts as CHILDREN sharing wazero Runtime+Module; tick + httpCall + shared-data state at root; per-`*RootVM` tick goroutine + 10ms envoy-go-strict period floor + Clock seam FIRST co-consumer beyond phase-21 (RATIFIES phase-21 ADR-0186 extraction); per-stream Module instantiation pattern (fresh vs pooled vs shared) deferred to 25.2 IMPL R8 escape-valve at the > 1ms threshold (D-25.2-P2 + parent §13-R8 carry-forward); the 25.1 per-stream `*VM` (61µs/stream construction) RETIRED per D-P-PLAN-6. | Task 22 |
| **ADR-0206** | 25.2 ABI extensions — 14 NEW env-namespace hostcalls (3 body/buffer + 2 stream-control + 1 timer + 4 metrics + 2 shared-data + 1 outbound HTTP + 1 foreign-function per §5.1) + 7 NEW guest-export callbacks (proxy_on_request_body / proxy_on_response_body / proxy_on_request_trailers / proxy_on_response_trailers / proxy_on_tick / proxy_on_http_call_response / proxy_on_foreign_function per §5.3) + 21 NEW capability keys at 25.2 with gate-at-`registerCallback` discipline per AMEND-B5 (denied capabilities → NOT registered on wazero Runtime; matches cpp-host `wasm.cc:176-189` `_REGISTER_PROXY` macro) + buffer-clamp wire-contract per AMEND-B1 (`proxy_get_buffer_bytes` clamps on overflow; only i32-overflow returns BadArgument; byte-faithful to cpp-host `src/exports.cc:get_buffer_bytes`) + metric signedness per AMEND-B2 (`proxy_increment_metric` SIGNED int64 delta; `proxy_record_metric` UNSIGNED uint64 value; MetricType enum Counter=0/Gauge=1/Histogram=2) + NUL-delimited property-path wire format per AMEND-B4 + `internal/wasm/foreign.go` ForeignFunctionRegistry with EMPTY default registry per AMEND-A9 + foreign-function dispatch concurrency model = mutex-per-RootVM per D-P-PLAN-9 + full ~70-path proxy_get_property roster per AMEND-B4 (~10 dispatched roots + 4 direct tokens; `xds.*` consolidates listener+route+cluster; `upstream_filter_state` distinct root co-equal to `filter_state`) + ABICallbacks interface 13→20 methods + the 25.1 SandboxConfig 37→58 capability keys extension. | Task 22 |
| **ADR-0207** | NEW `internal/filterstate/` framework primitive at 25.2 second-consumer scope per Q7 — generic per-stream filter-state Bucket + FilterStateObject interface (Marshal/Unmarshal/HasData/StateType) + StateType discriminator (read-only vs mutable; mutable overrides; read-only-vs-mutable conflict rejected) + sync semantics matching phase-22.2's in-package implementation; consumer #1 = phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking (the `:filterState()` Lua surface stays UNCHANGED; only the underlying storage layer flips from in-package map to shared primitive; ~50-100 LoC migration delta); consumer #2 = phase-25.2 wasm `proxy_get_property "filter_state.*"` + `"upstream_filter_state.*"` paths per AMEND-B4 + the `upstream_filter_state` DISTINCT root co-equal to `filter_state` per AMEND-B4 REFINEMENT; ADR-0188 API-revision allowance NOT consumed (the `internal/lua/` framework primitive itself is untouched; only the in-package filterstate.go file migrates); EXPLICIT API-REVISION ALLOWANCE clause for consumer #3+ (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families). | Task 22 |
| **ADR-0208** | NEW `internal/filter/http/wasm/` 25.2 package extensions — full hostcall wiring per §3.6 + 9 envoy-go-strict counters per §7.1 + AMEND-B3 (counter 14 `http_call_response_after_close` per AMEND-B3 recommendation; project stat count 119 → 128) + 4 envoy-go-strict-only `PluginConfig` config fields per Qs 2/6/9 (envoy_go_strict_body_buffer_cap_bytes default 16 MiB + envoy_go_strict_shared_data_value_cap_bytes default 1 MiB + envoy_go_strict_shared_data_max_entries default 1024 + envoy_go_strict_dynamic_stats_max_entries default 1024) + dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2 via NEW `internal/stats/dynamic/` infrastructure subpackage with per-plugin Registry SCOPE discipline (NOT plugin-prefix interpolation as BRAINSTORM Q9 hypothesized) + mixed-mode fixture-0036 discipline per Q8 (single-listener + 2 upstream clusters + 14 scenarios partitioned by assertion-class — 10 deterministic cross-side + 4 non-deterministic subject-only via StatsAsserter; deliberate-break liveness verification mandatory) + subject-only boot-reject fixture-0037 per D-25.2-P1 (anticipated arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`; reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field; runner-branch shape settles at IMPL — recommended: extend BootRejectFixture with `subjectOnly: true` flag) + 25.2 BEHAVIOR_CONTRACT.md ~7-edit bundle per ADR-0052 atomic landing + 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + per-stream body-buffer accumulation with envoy-go-strict cap enforcement (16 MiB default; 413-on-exceed via SendLocalReply; body_buffer_cap_exceeded counter + envoy_go.failures co-increment per §2.25) + envoy-go-strict departure record bundle (6 records consolidated per §13.4: 9-counter bundle + body-buffer cap + shared-data cap + tick floor + foreign-function 0-vs-10 + dynamic-stats namespace + cap). | Task 22 |

### In-place §Consequences AMEND acknowledgment on ADR-0202 (no new ADR number consumed)

Per §10.2 + ADR-0044 in-place edit discipline. ADR-0202 (NEW `internal/wasm/` framework primitive — anchored at 25.1 IMPL per phase-25.1 SPEC §3.1) gains a one-line acknowledgment paragraph in §Consequences. Provisional wording per 25.2 SPEC §10.2 (settles at Task 22):

> *"Phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern carries forward to 25.2 IMPL R8 escape-valve). The EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent."*

Lands at Task 22 per ADR-0044 in-place edit discipline (the §Consequences section of ADR-0202 in DECISIONS.md gains the paragraph; the `**Status:**` line gains an AMEND timestamp note `; AMENDED 2026-MM-DD per phase-25.2 one-line acknowledgment in §Consequences`).

### CONDITIONAL ADR landing (only if R8 escape-valve fires per D-P-PLAN-11)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0209** (CONDITIONAL) | Per-stream Module instantiation pattern — pooled-Module vs shared-Module-with-mutex-serialization. Anchors only if Task 22 `BenchmarkPerStreamModule_Instantiation` reports `ns/op > 1_000_000` (= 1ms threshold per parent §13-R8 + D-25.2-P2 + this PLAN's D-P-PLAN-11). §Context + §Decision + §Consequences body all land at the same Task 22 commit per ADR-0044. If unconsumed: next-free ADR-0209 carries forward to 25.3 BRAINSTORM as the 25.3 IMPL escape-valve slot per parent §1.2 + 25.2 SPEC §1.2. **Anticipated UNCONSUMED** per 25.2 SPEC §2.16 + the 25.1 R8 observed 61µs/stream observation (the 25.2 root-VM model retires per-stream Runtime construction so per-stream cost shrinks WELL UNDER 1ms). | Task 22 (CONDITIONAL) |

The implementer at Task 22 AUTHORS the 4 ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the 25.2 SPEC commit per ADR-0044), includes the ADRs + the ADR-0202 acknowledgment in the Task 22 commit message, and verifies via `grep -nE '^## ADR-0205:' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0206 + ADR-0207 + ADR-0208). If R8 escape-valve fires per D-P-PLAN-11, ADR-0209 §Context + §Decision + §Consequences body also lands at the same commit.

**NO in-place ADR-0125 amendment at this PLAN commit + at Task 22** — per AMEND-A3 the 5th-canonical REUSE-by-absence is DEFINITIVE; ADR-0125 STAYS at 10 across all of phase 25; the AMENDMENT-anticipation paragraph that would land at 25.3 IMPL is REPLACED by ADR-0210 EXPLICIT-NO-NEW-CANONICAL classification at 25.3 (per parent §10.3 + 25.2 SPEC §10.3 25.3 anticipated ADRs).

**ADR-0044 escape-valve held in reserve per D-P-PLAN-11** — `ADR-0209` is the conditional escape-valve slot; the 25.2 SPEC's STRENGTHENED-WEAK-HOLD-with-1-slot-buffer per §1.2 + parent §13-R8 STANDS UNCHANGED at this PLAN commit.

---

## Task graph (sequential vs parallelizable per D-P-PLAN-7)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task dependency graph:

- **Pre-Task 0** (PROGRESS.md preamble + 15-precondition verification) — sequential prerequisite for everything.
- **Task 1** (NEW `internal/wasm/root_vm.go` + `stream_context.go` + DELETE 25.1 `vm.go` per D-P-PLAN-6) — sequential prerequisite for Tasks 2-22.
- **Task 2** (`internal/wasm/sandbox.go` EXTEND — 21 NEW capability keys per AMEND-B5) — sequential after Task 1; file-disjoint from Task 1 but conceptually depends on RootVM/StreamContext for the gate-at-registration discipline.
- **Task 3** (`internal/wasm/registration.go` EXTEND — 14 NEW hostcall registrations + 7 NEW callback dispatch + gate-at-registration discipline per AMEND-B5) — sequential after Tasks 1+2; depends on RootVM + new capability keys.
- **Tasks 4, 5, 6, 7, 8, 9, 11** — **PARALLELIZABLE (7-way)** after Task 3; file-disjoint within their respective packages:
  - **Task 4** — NEW `internal/wasm/abi/body_bridge.go` + `abi/stream_control.go` (body+buffer hostcalls with AMEND-B1 clamp + stream-control hostcalls per R-25.2-1).
  - **Task 5** — NEW `internal/wasm/tick.go` + `abi/timer.go` (per-RootVM tick goroutine + 10ms floor + Clock seam FIRST co-consumer per Q5 + R-25.2-9).
  - **Task 6** — NEW `internal/wasm/shared_data.go` + `abi/shared_data.go` (CAS + envoy-go-strict caps per Q6 + R-25.2-10).
  - **Task 7** — NEW `internal/wasm/foreign.go` + `abi/foreign.go` (ForeignFunctionRegistry + EMPTY default + D-P-PLAN-9 mutex-per-RootVM dispatch model per AMEND-A9 + R-25.2-8).
  - **Task 8** — NEW `internal/wasm/http_call.go` + `abi/http_call.go` (proxy_http_call dispatch + cancel-at-destruction + http_call_response_after_close counter per R-25.2-3 + AMEND-B3).
  - **Task 9** — NEW `internal/filterstate/` package per ADR-0207 + R-25.2-6 (Bucket + FilterStateObject + StateType).
  - **Task 11** — NEW `internal/stats/dynamic/` package per ADR-0208 + AMEND-B2 + R-25.2-7 (Registry + MetricID + per-plugin scope).
- **Task 10** (phase-22.2 lua MIGRATION — `internal/filter/http/lua/filterstate.go` rewrites to delegate to `*filterstate.Bucket`) — sequential after Task 9; file-disjoint from Tasks 4-8 + 11; can run in PARALLEL with Tasks 4-8 + 11 + 12 + 13.
- **Task 12** (NEW `internal/wasm/dynamic_stats.go` + `abi/metrics.go` — wraps per-plugin `*dynamic.Registry`) — sequential after Task 11; can run in PARALLEL with Tasks 4-8 + 9 + 10 + 13.
- **Task 13** (NEW `internal/wasm/property.go` — full ~70-path roster per AMEND-B4 + R-25.2-4) — depends on Task 9 ONLY (filterstate for filter_state.* + upstream_filter_state.* + wasm.<key>); the SPEC-time hypothesized Task 7 partial-dependency is REMOVED — the `wasm.<key>` property class proxies via filter_state → upstream_filter_state per cpp-host `context.cc:987-1019` with NO foreign-function dispatch involvement. Can run in PARALLEL with Tasks 4-8 + 10 + 11 + 12 after Task 9 lands.
- **Task 14** (`internal/filter/http/wasm/compiled_config.go` EXTEND — 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms + RootVM construction at New) — sequential after Tasks 4-13 (compiledConfig consumes RootVM + dynamic.Registry + foreign-function registry).
- **Tasks 15, 16, 17** — **PARALLELIZABLE (3-way)** after Task 14; file-disjoint within `internal/filter/http/wasm/`:
  - **Task 15** — `abi_callbacks.go` EXTEND (7 NEW methods + 4 RE-USE primitive consumers).
  - **Task 16** — NEW `body.go` + `trailers.go` + `tick_clock.go` (body-buffer accumulation + cap + 413-on-exceed + trailer dispatch + Clock seam injection).
  - **Task 17** — NEW `property.go` (per-stream property resolver) + `stats.go` EXTEND (9 NEW envoy-go-strict counters).
- **Task 18** (`internal/filter/http/wasm/decode_headers.go` + `encode_headers.go` EXTEND — per-stream construction via `cfg.rootVM.NewStreamContext(ctx)`; depends on body.go for OnDecodeBuffer hook wiring + trailers.go for OnDecodeTrailers hook wiring + abi_callbacks for the 7 NEW methods) — sequential after Tasks 15 + 16 + 17.
- **Tasks 19, 20** — **PARALLELIZABLE (2-way)** after Task 18; file-disjoint:
  - **Task 19** — 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` + ~35 corpus seeds per D-P-PLAN-10.
  - **Task 20** — Differential fixture `0036-http-wasm-body-and-advanced` (14 scenarios; mixed-mode per Q8 + ADR-0192 precedent) + NEW BackendKind=HTTPWasmAdvanced.
- **Task 21** (Differential fixture `0037-http-wasm-body-and-advanced-boot-reject` + D-25.2-P1 closure first-action — arm 19 anticipated + runner-branch shape) — sequential after Task 20 (runner_test.go blank-import conflict).
- **Task 22** (Benchmark + BEHAVIOR_CONTRACT.md ~7-edit bundle + ADR-0205+0206+0207+0208 §Decision+§Consequences body landing + ADR-0202 one-line in-place AMEND acknowledgment + STATE.md re-advance + ROADMAP row 25.2 flip + CONDITIONAL ADR-0209 if R8 fires per D-P-PLAN-11 + REVIEW.md authoring per `superpowers:requesting-code-review`. **D-25.2-P2 closure** at benchmark gate + **D-25.2-P5 closure** at BEHAVIOR_CONTRACT.md bundle landing.) — depends on everything.

**Parallel-dispatch opportunities**: 7-way at Tasks 4+5+6+7+8+9+11; sub-parallel 3-way at Tasks 10+12+13 after their predecessors; 3-way at Tasks 15+16+17; 2-way at Tasks 19+20. **Sequential bottlenecks**: Pre-Task 0 → Task 1 → Task 2 → Task 3 → {4,5,6,7,8,9,11} → {10,12,13} → Task 14 → {15,16,17} → Task 18 → {19,20} → Task 21 → Task 22.

---

## Execution preconditions

Before Pre-Task 0 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl \
                 -b phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 15 preconditions verified at Pre-Task 0 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the phase-25.2-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-25.2-SPEC.md squash commit `f0eae39` + its SHA-fill follow-up `ec50365` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server; `rustc --version` reports a recent stable Rust (for Task 20 fixture-0036 Rust source reproduction; pinned toolchain in `scripts/README.md`).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `208` (ADR-0208 — the highest ADR §Context anchored as of master tip per the 25.2 SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0205:' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0205 §Context already at the 25.2 SPEC commit `f0eae39` per ADR-0044). Same for ADR-0206 + ADR-0207 + ADR-0208. `grep -nE '^## ADR-0209:' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0209 stays unconsumed UNLESS D-P-PLAN-11 R8 escape-valve fires at Task 22).
6. **ADR-0125 STAYS at 10 canonicals per AMEND-A3.** `grep -nE 'canonical|11th canonical' docs/envoy-go/DECISIONS.md` shows ADR-0125 body block with 10-canonical roster + no §(xvi) AMENDMENT-anticipation paragraph.
7. **NO 25.3-bound code at this 25.2 worktree.** Per BOOTSTRAP §4.1 invariant 2 — phase-25.3 surfaces (per-route 5th-canonical wholesale-override + multi-plugin VM-sharing + `VmConfig.environment_variables` activation + `failure_policy = FAIL_RELOAD` + conformance harness seed) MUST NOT land at 25.2 IMPL. If any 25.3-surface partial implementation has been started, halt + escalate to user.
8. **25.2 SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/SPEC.md` returns `f0eae39` (or descendant). If different, re-read 25.2 SPEC.
9. **25.2 PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
10. **25.1 IMPL inheritance.** `ls -d internal/wasm internal/filter/http/wasm test/fixtures/0034-http-wasm-headers-bridge test/fixtures/0035-http-wasm-boot-reject 2>/dev/null | wc -l` returns `4` (the 25.1 IMPL surfaces ALL present at master tip per 25.1 IMPL squash `feded64`). If any of the 4 is absent, 25.1 has not landed — halt + escalate.
11. **Pristine tree.** `git status --porcelain` returns empty.
12. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean (master tip; ALL 37 differential fixtures + 34 fuzzers passing).
13. **Pre-existing differential suite green.** `ls -d test/fixtures/00*/ | wc -l` returns `37` (the master-tip baseline post-25.1; fixtures 0000-0035 + any sub-fixture pairs). `go test -count=1 ./test/differential/...` returns every pre-existing fixture PASS — the regression baseline. Phase 25.2 adds the NEXT `BackendKind=HTTPWasmAdvanced` enum value + 2 new fixture directories (`0036-http-wasm-body-and-advanced` per Task 20 + `0037-http-wasm-body-and-advanced-boot-reject` per Task 21); post-25.2 dir count = 37 + 2 = 39.
14. **Pre-existing fuzzers run clean at 30s.** The 34 fuzzers from phases 02-25.1 run clean. Phase 25.2 adds the 35th (`FuzzWasmHostcallEnvelope` per Task 19). Quick smoke: `grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*') | wc -l` returns `34` (per 25.2 SPEC §8.5 D-S2 closure evidence).
15. **Pre-existing `internal/filterstate/` + `internal/stats/dynamic/` packages + `test/fixtures/0036-http-wasm-body-and-advanced/` + `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/` directories + `BackendKind=HTTPWasmAdvanced` enum value do NOT exist.** `test ! -d internal/filterstate && test ! -d internal/stats/dynamic && test ! -d test/fixtures/0036-http-wasm-body-and-advanced && test ! -d test/fixtures/0037-http-wasm-body-and-advanced-boot-reject && ! grep -q 'HTTPWasmAdvanced' test/differential/fixture/fixture.go && echo "ok: phase-25.2-new-surfaces absent"` returns success.

If all 15 preconditions pass, proceed to Pre-Task 0 (PROGRESS.md preamble) + Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 15-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md`

This pre-task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Context drafts are at the 25.2 SPEC commit `f0eae39`; ADR-0209 is CONDITIONAL (PLAN hypothesis per D-P-PLAN-11: it does NOT fire unless Task 22 benchmark surfaces > 1ms threshold). The PROGRESS preamble ANTICIPATES the 4 NEW ADR §Decision+§Consequences body landings + the ADR-0202 one-line in-place AMEND acknowledgment at Task 22 + records the 12 PLAN-time decisions D-P-PLAN-1..D-P-PLAN-12 + records the 5 SPEC-time D-question anticipated dispositions D-25.2-P1..D-25.2-P5.

Pre-Task 0 is NOT a SPEC §15 numbered acceptance-checklist item — the SPEC §15 46-item checklist begins at item 1 (root_vm.go materialized). Per D-P-PLAN-1, the SPEC §15 numbering is mapped to Tasks 1-22; PROGRESS.md preamble + precondition verification is the ritual prefix.

**Precondition:** worktree exists at `phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl`; branch base is master tip after the 25.2 PLAN.md SHA-fill follow-up; all 15 preconditions report green.
**Artifact:** `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` returns the Pre-Task 0 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` with: (a) Preamble summarizing the 15-precondition verification (verbatim command outputs captured); (b) the 4-NEW-ADR table + ADR-0202 one-line AMEND row + CONDITIONAL ADR-0209 row from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 12 PLAN-time decisions D-P-PLAN-1..D-P-PLAN-12 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) the 5 SPEC-time D-question anticipated dispositions D-25.2-P1..D-25.2-P5 from 25.2 SPEC §12; (e) a Pre-Task 0 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "phase 25.2 Pre-Task 0: PROGRESS.md preamble + 15-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
# expect: a 40-char SHA (Pre-Task 0 commit)
```

---

## Tier A — internal/wasm/ root-VM evolution (Tasks 1-3)

## Task 1: NEW `internal/wasm/root_vm.go` + `stream_context.go` + DELETE 25.1 `vm.go` per D-P-PLAN-6 + ADR-0205

**Files:**
- Create: `internal/wasm/root_vm.go` (~400-550 LoC)
- Create: `internal/wasm/stream_context.go` (~250-400 LoC)
- Create: `internal/wasm/root_vm_test.go` (~400-550 LoC)
- Create: `internal/wasm/stream_context_test.go` (~300-450 LoC)
- Modify: `internal/wasm/doc.go` (~+30-50 LoC; append 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs + 25.2 ABI surface evolution summary)
- Delete: `internal/wasm/vm.go` (25.1 per-stream `*VM` retired per D-P-PLAN-6)
- Delete: `internal/wasm/vm_test.go` (test coverage migrates to root_vm_test.go + stream_context_test.go)
- Append: `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md` (Task 1 entry per D-P-PLAN-3)

This task lands the root-VM lifecycle architecture per Q3 + ADR-0205 + 25.2 SPEC §3.1. The 25.1 per-stream `*VM` (each stream constructing a fresh `*wazero.Runtime` at 61µs/stream per the 25.1 Task 17 R8 benchmark) is RETIRED at 25.2; the new architecture has ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per cpp-host `Wasm`/`Plugin` model) + per-stream contexts as CHILDREN sharing the same wazero Runtime + Module. The `*RootVM` owns: tick goroutine + tick state (stub at this Task; activation in Task 5); shared-data map (stub; activation in Task 6); httpCall response routing + call_id allocation (stub; activation in Task 8); foreign-function registry view (stub; activation in Task 7); per-plugin dynamic-stats `*Registry` (stub; activation in Task 12). The `*StreamContext` carries per-stream state — `proxy_on_context_create(streamCtxID, rootCtxID)` invocation at construction + `proxy_on_done`/`proxy_on_log`/`proxy_on_delete` invocations at Close + cancel-at-destruction for outstanding httpCalls (stub; activation in Task 8). The 25.1 per-stream `*VM` is DELETED per D-P-PLAN-6 (no transitional shim — zero external consumers; migration is atomic at Task 18 decode/encode_headers EXTEND). **Sequential prerequisite for Tasks 2-22.**

**Precondition:** Pre-Task 0 complete; all 15 preconditions green; the 25.1 IMPL surfaces (internal/wasm/ vm.go + vm_test.go + compile.go + sandbox.go + registration.go + bytecode_util.go + pairs.go + wasi.go + abi/types.go + 8-file test split) present at master tip per precondition 10.
**Artifact:** `internal/wasm/root_vm.go` + `stream_context.go` + their test files materialized; `vm.go` + `vm_test.go` deleted; `doc.go` EXTENDED; build + tests pass.
**Acceptance:** `go build ./internal/wasm/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/wasm/...` clean; `go test -count=1 -race ./internal/wasm/...` passes (RootVM lifecycle + StreamContext per-stream + concurrent N=100 stream contexts share one RootVM no-state-leak verification); `ls internal/wasm/vm.go internal/wasm/vm_test.go 2>&1` returns "No such file or directory" (deletion verified); the existing `internal/filter/http/wasm/` package's import of `internal/wasm` still compiles (the filter package's `wasm.NewVM` references migrate at Task 18 — at Task 1 the filter package may temporarily fail to build at `wasm.NewVM` references; this is acceptable INSIDE Task 1's scope because Task 1 is the architectural-flip task; build cleanliness at the *whole-repo* scope is only required at Task 18). **SCOPED ACCEPTANCE**: at Task 1 commit time, `go build ./internal/wasm/...` MUST be clean (the new package surface compiles); `go build ./...` whole-repo MAY fail at the `internal/filter/http/wasm/` references to `wasm.NewVM` — Task 1's PROGRESS.md entry documents this expected breakage + records that Task 18 closes the build at whole-repo scope. ALTERNATIVE: keep a minimal transitional `wasm.NewVM(opts...) *StreamContext` shim that delegates to a process-singleton RootVM — REJECTED per D-P-PLAN-6 (introduces process-global state with no test coverage; the architectural-flip-with-internally-broken-build-window is cleaner per CLAUDE.md "no half-finished implementations" + the IMPL session's per-task commit boundary tolerates internal breakage as long as each Task IS individually verifiable at its own scope).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author the Task 1 files at the 4 listed paths per the 25.2 PLAN Task 1 + 25.2 SPEC §3.1 + §3.5. The 2 NEW production files (root_vm.go + stream_context.go) materialize per 25.2 SPEC §3.1 production signatures: RootVM struct + RootVMOption pattern (WithRootSandboxConfig + WithRootClock + WithRootPanicHandler + WithRootLogSink + WithRootHttpClient + WithRootForeignRegistry + WithRootDynamicStatsRegistry + WithRootSharedDataCaps) + NewRootVM constructor (constructs the wazero Runtime + instantiates the Module onto it + sets up tick goroutine STUB initially idle + initializes shared-data map empty + initializes httpCalls map empty + initializes streamCtxs map empty); Configure method (invokes _initialize/_start + proxy_on_vm_start + proxy_on_configure on root context); NewStreamContext method (allocates monotonic streamCtxID via nextStreamCtxID counter + invokes proxy_on_context_create(streamCtxID, rootCtxID) + returns *StreamContext); Close idempotent (stops tick + cancels outstanding httpCalls + closes dynamic-stats Registry + releases wazero Runtime). StreamContext struct + per-callback methods + Close idempotent (fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels any outstanding httpCalls dispatched from this stream — at Task 1 the cancel-at-destruction logic is a STUB that just deletes the streamCtxID entry from rootVM.streamCtxs; full cancel-at-destruction integration lands at Task 8 http_call.go). DELETE vm.go + vm_test.go (the 25.1 per-stream `*VM` retires). Update doc.go with 25.2 cross-refs. Tests at root_vm_test.go + stream_context_test.go cover RootVM lifecycle + StreamContext per-stream + concurrent N=100 stream contexts no-state-leak. The whole-repo build will FAIL at internal/filter/http/wasm/ references to wasm.NewVM — document this expected breakage in Task 1 PROGRESS.md entry; Task 18 closes the whole-repo build. Commit per Step 7 message template.

- [ ] **Step 1: Write failing tests** at `internal/wasm/root_vm_test.go` + `internal/wasm/stream_context_test.go`

```bash
go test -count=1 -v ./internal/wasm/ -run 'TestRootVM|TestStreamContext'
# Expected: FAIL (types not yet defined)
```

- [ ] **Step 2: Author `internal/wasm/root_vm.go` + `internal/wasm/stream_context.go`** per 25.2 SPEC §3.1 production signatures. The tick/shared-data/httpCall/foreign-function/dynamic-stats state is STUB at this Task (full activation at Tasks 5-8 + 12).

- [ ] **Step 3: DELETE 25.1 `internal/wasm/vm.go` + `internal/wasm/vm_test.go`** per D-P-PLAN-6.

```bash
rm internal/wasm/vm.go internal/wasm/vm_test.go
```

- [ ] **Step 4: Extend `internal/wasm/doc.go`** with 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs + 25.2 ABI surface evolution summary (RootVM lifecycle + ABICallbacks 13→20 methods + SandboxConfig 37→58 keys cross-refs + tick goroutine + shared-data + httpCall + foreign-function + property + dynamic-stats wrap).

- [ ] **Step 5: Run package-scoped tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run 'TestRootVM|TestStreamContext'
# Expected: PASS (lifecycle + per-stream + concurrent no-state-leak tests pass)
go vet ./internal/wasm/...
golangci-lint run ./internal/wasm/...
# Expected: each clean at the internal/wasm/ package scope
```

- [ ] **Step 6: Document whole-repo build expected-failure** at `internal/filter/http/wasm/` references — verify the failure is ONLY at the `wasm.NewVM` reference + that no other `internal/wasm/` consumer breaks (re-verify via `go build ./... 2>&1 | grep -v 'internal/filter/http/wasm' | grep error || echo 'no other consumer breaks'`). The whole-repo build closure happens at Task 18.

- [ ] **Step 7: Append PROGRESS.md Task 1 entry + commit**

```bash
git add internal/wasm/root_vm.go internal/wasm/stream_context.go internal/wasm/root_vm_test.go internal/wasm/stream_context_test.go internal/wasm/doc.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git rm internal/wasm/vm.go internal/wasm/vm_test.go
git commit -m "feat(internal/wasm): root-VM lifecycle evolution per Q3 + ADR-0205 — RETIRE per-stream *VM

Phase 25.2 Task 1 (Tier A internal/wasm/ root-VM evolution). NEW root_vm.go +
stream_context.go materialize per 25.2 SPEC §3.1 production signatures.
*RootVM is per-compiledConfig long-lived VM (upstream-byte-faithful per
cpp-host Wasm/Plugin model). Owns tick goroutine + shared-data map + httpCall
routing + foreign-function registry view + per-plugin dynamic-stats Registry
(state STUB at this Task; activation at Tasks 5-8 + 12). *StreamContext is
per-stream child sharing wazero Runtime+Module — proxy_on_context_create
invocation at construction; proxy_on_done/proxy_on_log/proxy_on_delete +
cancel-at-destruction at Close (STUB cancel logic; activation at Task 8).

DELETED 25.1 internal/wasm/vm.go + vm_test.go per D-P-PLAN-6 (no transitional
shim — zero external consumers; migration is atomic at Task 18 decode/encode_
headers EXTEND). Whole-repo build temporarily fails at internal/filter/http/
wasm/ references to wasm.NewVM — DOCUMENTED expected breakage; Task 18 closes
whole-repo build. internal/wasm/ package-scoped build + tests clean."
```

---

## Task 2: `internal/wasm/sandbox.go` EXTEND — 21 NEW capability keys per AMEND-B5 + R-25.2-5

**Files:**
- Modify: `internal/wasm/sandbox.go` (~+150-220 LoC; 21 NEW capability key constants)
- Modify: `internal/wasm/sandbox_test.go` (~+200-300 LoC; per-NEW-key ALLOW/DENY exhaustive)
- Append: PROGRESS.md (Task 2 entry per D-P-PLAN-3)

This task EXTENDS the 25.1 `*SandboxConfig` capability roster from 37 keys to 58 keys per AMEND-B5 + 25.2 SPEC §3.4 REUSE 8. The 21 NEW keys per §11.5 D-25.2-5: 14 hostcall keys (proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_status + proxy_continue_stream + proxy_close_stream + proxy_set_tick_period_milliseconds + proxy_define_metric + proxy_increment_metric + proxy_record_metric + proxy_get_metric + proxy_set_shared_data + proxy_get_shared_data + proxy_http_call + proxy_call_foreign_function) + 7 lifecycle keys (proxy_on_request_body + proxy_on_response_body + proxy_on_request_trailers + proxy_on_response_trailers + proxy_on_tick + proxy_on_http_call_response + proxy_on_foreign_function). Per AMEND-B5: env-namespace hostcalls gate at `registerCallback` time in `wasm.cc:176-189` `_REGISTER_PROXY` macro; lifecycle hooks gate at `getFunction` time in `wasm.cc:238-247` `_GET_PROXY` macro. The 25.1 default-deny posture per AMEND-A5 + ADR-0204 INHERITS unchanged — empty `AllowedCapabilities` map → DENY ALL.

**Precondition:** Task 1 complete (root_vm.go + stream_context.go provide RootVM/StreamContext types that consume the SandboxConfig at NewRootVM time).
**Artifact:** `internal/wasm/sandbox.go` EXTENDED with 21 NEW capability key constants + `internal/wasm/sandbox_test.go` EXTENDED with per-key ALLOW/DENY coverage.
**Acceptance:** `go test -count=1 -v ./internal/wasm/ -run TestSandbox` passes (per-NEW-key ALLOW/DENY exhaustive + total key count assertion 58); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 2 per 25.2 SPEC §11.5 + AMEND-B5. Extend `internal/wasm/sandbox.go` with 21 NEW capability key constants — 14 hostcall keys per §11.5 D-25.2-5 hostcall-keys table + 7 lifecycle keys per §11.5 D-25.2-5 lifecycle-keys table. Mirror the 25.1 package-private constant naming convention (`capProxyGetBufferBytes = "proxy_get_buffer_bytes"`, etc.). Update the 25.1 `allCapabilityKeys() []string` helper (if present) to include the 21 NEW keys → total 58 keys. Tests at `sandbox_test.go` verify each NEW key's ALLOW (`AllowedCapabilities = map[string]SanitizationConfig{capProxyGetBufferBytes: {}}` → `IsAllowed("proxy_get_buffer_bytes")` returns true) + DENY (empty map → all 21 NEW keys DENY) + total-roster-count = 58 via `len(allCapabilityKeys())` assertion. The gate-at-registration assertion lives at `registration_test.go` (Task 3) — at Task 2 sandbox_test.go only verifies the IsAllowed semantic, NOT the host-module wiring assertion.

- [ ] **Step 1: Write failing tests** at `internal/wasm/sandbox_test.go` (extend existing test file)

```bash
go test -count=1 -v ./internal/wasm/ -run TestSandbox
# Expected: FAIL (NEW capability key constants not yet defined; total-roster-count assertion fails 37 vs expected 58)
```

- [ ] **Step 2: Implement `internal/wasm/sandbox.go` extensions** per AMEND-B5 — 21 NEW capability key constants + extend the allCapabilityKeys() helper.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run TestSandbox
# Expected: PASS (per-NEW-key ALLOW/DENY + total-roster-count 58 verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 2 entry + commit**

```bash
git add internal/wasm/sandbox.go internal/wasm/sandbox_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): sandbox.go — 21 NEW capability keys per AMEND-B5 + R-25.2-5

Phase 25.2 Task 2 (Tier A internal/wasm/ root-VM evolution). EXTEND
*SandboxConfig capability roster 37 → 58 keys per AMEND-B5. 14 NEW hostcall
keys (proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_
status + proxy_continue_stream + proxy_close_stream + proxy_set_tick_period_
milliseconds + proxy_define_metric + proxy_increment_metric + proxy_record_
metric + proxy_get_metric + proxy_set_shared_data + proxy_get_shared_data +
proxy_http_call + proxy_call_foreign_function) + 7 NEW lifecycle keys
(proxy_on_request_body + proxy_on_response_body + proxy_on_request_trailers
+ proxy_on_response_trailers + proxy_on_tick + proxy_on_http_call_response
+ proxy_on_foreign_function). 25.1 default-deny posture per AMEND-A5 +
ADR-0204 INHERITS unchanged. Per-key ALLOW/DENY exhaustive + total-roster-
count 58 verified."
```

---

## Task 3: `internal/wasm/registration.go` EXTEND — 14 NEW hostcall registrations + 7 NEW callback dispatch + gate-at-registration discipline per AMEND-B5 + R-25.2-5

**Files:**
- Modify: `internal/wasm/registration.go` (~+250-400 LoC; 14 NEW hostcall registrations + 7 NEW callback dispatch + gate-at-registration discipline + buffer-clamp wire-contract hook for AMEND-B1)
- Modify: `internal/wasm/registration_test.go` (~+200-300 LoC; 14 NEW hostcall registration round-trip tests + 7 NEW callback dispatch tests + gate-at-registration assertions per R-25.2-5)
- Append: PROGRESS.md (Task 3 entry per D-P-PLAN-3)

This task EXTENDS `internal/wasm/registration.go` host-module wiring with 14 NEW env-namespace hostcalls per §5.1 + 7 NEW callback dispatch entries per §5.3 + the gate-at-registration discipline per AMEND-B5 + R-25.2-5. The gate-at-registration discipline mirrors upstream cpp-host `wasm.cc:176-189` `_REGISTER_PROXY` macro: for each NEW capability key (from Task 2 sandbox extensions), if `vm.sandbox.IsAllowed(key)` returns false at NewRootVM time, the corresponding host function is NOT registered on the wazero Runtime; the guest's import resolution fails at module-instantiation OR the runtime trap fires on call. This is structurally DIFFERENT from the 25.1 gate-at-call-site discipline — at 25.1 each hostcall body called `vm.sandbox.IsAllowed(...)` at every call and returned `WasmResult::InternalFailure` if denied; at 25.2 the gate moves to the registration time. The 14 NEW hostcall registrations dispatch through the per-family abi/* files (`abi/body_bridge.go` per Task 4 + `abi/stream_control.go` per Task 4 + `abi/timer.go` per Task 5 + `abi/metrics.go` per Task 12 + `abi/shared_data.go` per Task 6 + `abi/http_call.go` per Task 8 + `abi/foreign.go` per Task 7) — at Task 3 the registration.go wires the hostcall envelope (signature + capability gate + delegation to a forward-declared abi function) but the actual abi/* dispatch implementations land at Tasks 4-8 + 12. **The buffer-clamp wire-contract per AMEND-B1 + R-25.2-1 is enforced at the `proxy_get_buffer_bytes` host shim — at Task 3 this is a forward-decl reference to `abi.GetBufferBytesShim` which Task 4 implements.** The 7 NEW callback dispatch entries route to `*StreamContext.CallProxyOnRequestBody` (etc.) per the §5.3 lifecycle callback table — at Task 3 the registration.go wires the callback envelope; the abi/* layer doesn't apply to callbacks (callbacks are guest-export functions looked up via wazero's `Module.ExportedFunction(name)` + invoked from host-side; the host-side caller logic lives in `internal/wasm/{tick.go,http_call.go,foreign.go}` per Tasks 5+7+8 — at Task 3 the registration.go just publishes the per-callback gate-at-getFunction discipline per AMEND-B5).

**Precondition:** Tasks 1 + 2 complete (RootVM/StreamContext types + extended SandboxConfig).
**Artifact:** `internal/wasm/registration.go` EXTENDED + `internal/wasm/registration_test.go` EXTENDED; 14 NEW hostcalls + 7 NEW callbacks + gate-at-registration discipline + buffer-clamp wire-contract forward-decl reference.
**Acceptance:** `go test -count=1 -v ./internal/wasm/ -run 'TestRegistration|TestGateAtRegistration'` passes (14 NEW hostcall registration round-trip tests + 7 NEW callback dispatch tests + gate-at-registration assertion per R-25.2-5: deny `proxy_set_tick_period_milliseconds` → assert host function NOT in wazero `*Module`'s import set + guest's call traps); `golangci-lint run ./internal/wasm/...` clean. **SCOPED ACCEPTANCE**: at Task 3 commit time, the abi/* dispatch implementations (body_bridge.go + stream_control.go + timer.go + metrics.go + shared_data.go + http_call.go + foreign.go) do NOT YET exist — Task 3 registration.go uses forward-decl references that compile against stub abi/* function declarations (which Task 3 introduces as `func GetBufferBytesShim(ctx, ...) WasmResult { panic("Task 4 not yet landed") }` placeholders inside `internal/wasm/abi/stubs_25_2.go` — a temporary placeholder file deleted at Tasks 4+5+6+7+8+12 as the real implementations land). ALTERNATIVE: defer the registration.go wiring until Tasks 4-8 + 12 have all landed — REJECTED because that breaks the 7-way parallel dispatch opportunity (Tasks 4-8 + 9 + 11 would all be blocked on Task 3); the placeholder-file pattern enables the parallel dispatch + each Task 4-8 + 12 lands by deleting its placeholder line(s) from `abi/stubs_25_2.go`.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 3 per 25.2 SPEC §5.1 + §5.3 + AMEND-B5 + R-25.2-5. EXTEND `internal/wasm/registration.go` host-module wiring to register the 14 NEW env-namespace hostcalls per §5.1 table (rows 25-38) + 7 NEW callback dispatch entries per §5.3 table (rows C14-C20). For each NEW hostcall: (a) define the wazero host-function signature (param types from §11.5 cpp-host pin); (b) before calling `wazero.Runtime.NewHostModuleBuilder().NewFunctionBuilder().WithFunc(...)`, check `vm.sandbox.IsAllowed(capabilityKey)` — if false, SKIP the registration (gate-at-registration per AMEND-B5); if true, register the function with a body that delegates to a forward-declared abi function (e.g., `func proxy_get_buffer_bytes(ctx, ...) WasmResult { return abi.GetBufferBytesShim(ctx, ...) }`). Create a temporary `internal/wasm/abi/stubs_25_2.go` file with `panic("Task N not yet landed")` placeholder bodies for each forward-decl — each placeholder gets deleted (and the real impl added) at the corresponding Task 4/5/6/7/8/12. For the 7 NEW callbacks: extend the `*StreamContext.HasGlobalFunc(name)` lookup to gate at `vm.sandbox.IsAllowed(callbackKey)` per AMEND-B5 _GET_PROXY discipline — if denied, return false (the host treats the missing function as if the guest hadn't exported it). Tests at `registration_test.go` verify: (a) each NEW hostcall registered when capability is ALLOWED; (b) NOT registered when capability is DENIED — assert via inspecting the wazero `*Module`'s import set (the `wazero.CompiledModule.ImportedFunctions()` API); (c) each NEW callback's HasGlobalFunc returns false when capability is DENIED. Update the host-module total count assertion (24 active at 25.1 + 14 NEW = 38 active at 25.2; plus 9 stub-Unimplemented = 47 total registered).

- [ ] **Step 1: Write failing tests** at `internal/wasm/registration_test.go`

```bash
go test -count=1 -v ./internal/wasm/ -run 'TestRegistration|TestGateAtRegistration'
# Expected: FAIL (14 NEW hostcall registrations + 7 NEW callbacks + gate-at-registration assertion not yet implemented)
```

- [ ] **Step 2: Implement `internal/wasm/registration.go` extensions** per §5.1 + §5.3 + AMEND-B5 gate-at-registration discipline + buffer-clamp wire-contract forward-decl reference; create `internal/wasm/abi/stubs_25_2.go` placeholder file.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run 'TestRegistration|TestGateAtRegistration'
# Expected: PASS (14 NEW hostcall registrations + 7 NEW callbacks + gate-at-registration assertion verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 3 entry + commit**

```bash
git add internal/wasm/registration.go internal/wasm/registration_test.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): registration.go — 14 NEW hostcalls + 7 NEW callbacks + gate-at-registration per AMEND-B5 + R-25.2-5

Phase 25.2 Task 3 (Tier A internal/wasm/ root-VM evolution). EXTEND
registration.go host-module wiring with 14 NEW env-namespace hostcalls per
§5.1 + 7 NEW callback dispatch entries per §5.3. Gate-at-registration
discipline per AMEND-B5 mirrors upstream cpp-host wasm.cc:176-189 _REGISTER_
PROXY macro — denied capabilities → NOT registered on wazero Runtime; guest's
import resolution fails at module-instantiation OR runtime trap on call. 7
NEW callbacks gate at HasGlobalFunc lookup per _GET_PROXY macro. Buffer-
clamp wire-contract per AMEND-B1 enforced at proxy_get_buffer_bytes shim
(forward-decl to abi.GetBufferBytesShim; Task 4 lands real impl). Temporary
internal/wasm/abi/stubs_25_2.go placeholder file (panic-on-call for each
forward-decl; placeholders deleted at Tasks 4/5/6/7/8/12 as real impls land).
14 NEW hostcalls + 7 NEW callbacks + gate-at-registration assertion per
R-25.2-5 GREEN. Host-module total: 24 active 25.1 + 14 NEW = 38 active +
9 stub-Unimplemented = 47 total registered."
```

---

## Tier B — internal/wasm/abi/ family dispatches + root-VM-anchored impls (Tasks 4-8; 5-way parallelizable per D-P-PLAN-7)

## Task 4: NEW `internal/wasm/abi/body_bridge.go` + `abi/stream_control.go` + AMEND-B1 buffer-clamp wire-contract per R-25.2-1

**Files:**
- Create: `internal/wasm/abi/body_bridge.go` (~150-220 LoC)
- Create: `internal/wasm/abi/body_bridge_test.go` (~200-300 LoC; AMEND-B1 clamp golden table per R-25.2-1)
- Create: `internal/wasm/abi/stream_control.go` (~80-120 LoC)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE the body+buffer + stream-control placeholders; ~-30 LoC delta)
- Append: PROGRESS.md (Task 4 entry per D-P-PLAN-3)

This task lands the body+buffer hostcall dispatch (proxy_get_buffer_bytes + proxy_set_buffer_bytes + proxy_get_buffer_status per §5.1 #25-27) with the AMEND-B1 buffer-clamp wire-contract per R-25.2-1 + the stream-control hostcalls (proxy_continue_stream + proxy_close_stream per §5.1 #28-29). AMEND-B1 buffer-clamp semantic per §11.1: `proxy_get_buffer_bytes` clamps silently on `start + max_size > buffer.length` (returns `WasmResult::Ok` with truncated length); only `start + max_size` i32-overflow returns `BadArgument`. Byte-faithful to cpp-host `src/exports.cc:get_buffer_bytes`. WasmBufferType values 0 (HttpRequestBody) + 1 (HttpResponseBody) + 4 (HttpCallResponseBody) ACTIVATED via the dispatch table inside `body_bridge.go` (the host shims delegate to the consumer-side body accumulation via the ABICallbacks methods OnDecodeBuffer/OnEncodeBuffer/OnHttpCallResponseBody — those methods live at `internal/filter/http/wasm/body.go` per Task 16 + `abi_callbacks.go` per Task 15). The stream-control hostcalls dispatch to the consumer-side resume logic (paired with PAUSE-buffer dispatch on body callbacks per Q1).

**Precondition:** Tasks 1 + 2 + 3 complete (RootVM + StreamContext + SandboxConfig extensions + registration.go forward-decl wiring).
**Artifact:** `internal/wasm/abi/body_bridge.go` + `abi/stream_control.go` + `abi/body_bridge_test.go` materialized; `abi/stubs_25_2.go` placeholders for body+buffer + stream-control deleted; AMEND-B1 buffer-clamp golden table GREEN.
**Acceptance:** `go test -count=1 -v ./internal/wasm/abi/ -run 'TestBodyBridge|TestStreamControl'` passes (AMEND-B1 buffer-clamp golden table per R-25.2-1: clamp-on-overflow returns Ok+truncated; i32-overflow returns BadArgument; round-trip through ABICallbacks); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 4 per 25.2 SPEC §5.1 #25-29 + AMEND-B1 + R-25.2-1. NEW `internal/wasm/abi/body_bridge.go` materializes the `GetBufferBytesShim` + `SetBufferBytesShim` + `GetBufferStatusShim` host functions referenced from registration.go (Task 3 forward-decls). The clamp wire-contract per AMEND-B1: in `GetBufferBytesShim(ctx, vm, bufferType, start, maxSize, retDataPtr, retSizePtr) WasmResult`, after fetching the underlying buffer via the consumer-side ABICallbacks method (e.g., `vm.cb.GetRequestBodyChunk()`), apply the clamp: `if start > uint32(len(buf)) { length = 0 } else if start + maxSize > uint32(len(buf)) { length = uint32(len(buf)) - start } else { length = maxSize }`; if `start + maxSize < start` (i32 overflow detected) → return `WasmResult::BadArgument`; else write `buf[start:start+length]` to guest memory at retDataPtr + write length to retSizePtr + return `WasmResult::Ok`. WasmBufferType values 0/1/4 dispatch to the appropriate consumer-side ABICallbacks accessor. NEW `internal/wasm/abi/stream_control.go` materializes `ContinueStreamShim(ctx, vm, streamType) WasmResult` + `CloseStreamShim(ctx, vm, streamType) WasmResult` — both delegate to consumer-side ABICallbacks methods (`vm.cb.ContinueStream(streamType)` / `vm.cb.CloseStream(streamType)`). Stream-type discriminator: HttpRequest=0, HttpResponse=1, HttpUpstream=2. Tests at `body_bridge_test.go` materialize the AMEND-B1 golden table per R-25.2-1: row (a) start_in_bounds + max_in_bounds → Ok + length=full; (b) start_in_bounds + max_overflows → Ok + length=truncated; (c) start_at_end + max_anything → Ok + length=0; (d) start_beyond_end → Ok + length=0; (e) start+max_size i32-overflow → BadArgument; (f) WasmBufferType=99 (out-of-range) → BadArgument. Delete the body+buffer + stream-control placeholder lines from `abi/stubs_25_2.go`.

- [ ] **Step 1: Write failing tests** at `internal/wasm/abi/body_bridge_test.go`

```bash
go test -count=1 -v ./internal/wasm/abi/ -run 'TestBodyBridge|TestStreamControl'
# Expected: FAIL (functions not yet defined; AMEND-B1 clamp golden table assertions fail)
```

- [ ] **Step 2: Implement `internal/wasm/abi/body_bridge.go`** per AMEND-B1 clamp wire-contract per R-25.2-1.

- [ ] **Step 3: Implement `internal/wasm/abi/stream_control.go`** per §5.1 #28-29.

- [ ] **Step 4: Delete body+buffer + stream-control placeholder lines** from `internal/wasm/abi/stubs_25_2.go`.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -v ./internal/wasm/abi/ -run 'TestBodyBridge|TestStreamControl'
# Expected: PASS (AMEND-B1 clamp golden table + stream-control dispatch verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 4 entry + commit**

```bash
git add internal/wasm/abi/body_bridge.go internal/wasm/abi/body_bridge_test.go internal/wasm/abi/stream_control.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm/abi): body+buffer + stream-control dispatch per §5.1 + AMEND-B1 + R-25.2-1

Phase 25.2 Task 4 (Tier B internal/wasm/abi/ family dispatches). NEW
body_bridge.go materializes proxy_get_buffer_bytes + proxy_set_buffer_bytes
+ proxy_get_buffer_status host shims per §5.1 #25-27. AMEND-B1 buffer-clamp
wire-contract per R-25.2-1: proxy_get_buffer_bytes clamps silently on
start+max_size > buffer.length (returns Ok with truncated length); only
start+max_size i32-overflow returns BadArgument. Byte-faithful to cpp-host
src/exports.cc:get_buffer_bytes. WasmBufferType values 0 (HttpRequestBody) +
1 (HttpResponseBody) + 4 (HttpCallResponseBody) ACTIVATED via dispatch
table. NEW stream_control.go materializes proxy_continue_stream + proxy_
close_stream per §5.1 #28-29 (paired with PAUSE-buffer dispatch on body
callbacks per Q1). AMEND-B1 golden table per R-25.2-1 GREEN: clamp-on-
overflow + i32-overflow BadArgument + round-trip through ABICallbacks."
```

---

## Task 5: NEW `internal/wasm/tick.go` + `abi/timer.go` + per-RootVM tick goroutine + 10ms envoy-go-strict floor + Clock seam FIRST co-consumer per Q5 + R-25.2-9 + ADR-0205

**Files:**
- Create: `internal/wasm/tick.go` (~200-300 LoC; per-RootVM tick goroutine + 10ms floor + Clock seam)
- Create: `internal/wasm/tick_test.go` (~250-400 LoC; fake-clock fixture tests)
- Create: `internal/wasm/abi/timer.go` (~80-120 LoC; proxy_set_tick_period_milliseconds dispatch)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE timer placeholder; ~-5 LoC delta)
- Modify: `internal/wasm/root_vm.go` (~+30-50 LoC; integrate tick goroutine spawn at NewRootVM + Close stops the goroutine)
- Append: PROGRESS.md (Task 5 entry per D-P-PLAN-3)

This task lands the per-`*RootVM` tick goroutine per Q5 + R-25.2-9 + ADR-0205. The tick goroutine runs `for { select { case <-clock.After(effectivePeriod): rootVM.lockAndCall(proxy_on_tick, rootCtxID); case <-stop: return } }` where `effectivePeriod = max(period_ms, 10ms)` per Q5 envoy-go-strict floor (10ms clamp prevents guest-driven CPU spin attacks; period=0 cancels). Uses ADR-0186 `clock.Clock` seam injection at `NewRootVM` time via `WithRootClock(clk)` for fixture fake-time support. The `SetTickPeriod(period)` method re-schedules the tick (period=0 cancels via close(tickStop)). FIRST co-consumer of phase-21 ADR-0186 Clock seam beyond phase-21 itself — RATIFIES the phase-21 extraction discipline. The `proxy_on_tick` callback is invoked on the root context (NOT a stream context) — the host-side dispatch uses `*RootVM.lockAndCall` which acquires the per-RootVM dispatch lock + calls `wazero.Module.ExportedFunction("proxy_on_tick").Call(ctx, rootCtxID)` with panic-recovery wrapper. The tick callback dispatched panic → recover() → log + `envoy_go.failures` counter increment + tick goroutine survives (does NOT die on guest panic).

**Precondition:** Tasks 1 + 2 + 3 complete (RootVM/StreamContext + SandboxConfig + registration.go wiring; the timer placeholder in stubs_25_2.go is referenced by Task 3's registration.go).
**Artifact:** `internal/wasm/tick.go` + `abi/timer.go` + `tick_test.go` materialized; `root_vm.go` integrates tick goroutine spawn at NewRootVM + Close stops the goroutine; FIRST co-consumer of ADR-0186 Clock seam.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestTick` passes (fake-clock fixture tests: 10ms floor enforcement; period=0 cancels; tick fires at expected intervals; concurrent stream contexts share one tick goroutine; panic-recovery in tick callback); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 5 per 25.2 SPEC §3.1 tick.go + §5.1 #30 + Q5 + R-25.2-9. NEW `internal/wasm/tick.go` materializes the per-RootVM tick goroutine + 10ms floor enforcement + Clock seam injection via WithRootClock. The tick goroutine lives at `*RootVM.tickRun(ctx)` (started at NewRootVM time as `go rv.tickRun(ctx)`); runs `for { select { case <-rv.clk.After(rv.tickPeriod): rv.lockAndCall(proxy_on_tick, rv.rootCtxID); case <-rv.tickStop: return } }`. SetTickPeriod(period time.Duration) acquires rv.tickMu + computes effectivePeriod = max(period, 10*time.Millisecond) per Q5 — if period == 0, close(rv.tickStop) + reset tickStop = make(chan struct{}); if period > 0, signal the goroutine to re-schedule (via a re-schedule channel OR by closing the old stop + spawning a fresh goroutine with the new period — anticipated answer per IMPL: re-spawn pattern is simpler + has correct semantics). At Close(), close(rv.tickStop) + wait for goroutine to exit (sync.WaitGroup or similar). The lockAndCall wraps the proxy_on_tick invocation with the per-RootVM dispatch lock (rv.dispatchMu.Lock() + defer Unlock()) + a panic-recovery wrapper (deferred recover() → log + envoy_go.failures counter increment via the per-plugin stats). NEW `internal/wasm/abi/timer.go` materializes `SetTickPeriodMillisecondsShim(ctx, vm, periodMs uint32) WasmResult` — validates periodMs (must be non-negative; uint32 so always non-negative; period_ms=0 cancels per Q5; period_ms < 10 clamped to 10 per Q5 floor); delegates to `vm.rootVM.SetTickPeriod(time.Duration(periodMs) * time.Millisecond)`. Tests at `tick_test.go` use phase-21 `internal/clock.FakeClock` for fake-time fixtures: (a) NewRootVM with FakeClock + SetTickPeriod(50ms) + fake-clock advance 250ms + assert proxy_on_tick invoked exactly 5 times; (b) SetTickPeriod(5ms) → assert effectivePeriod=10ms; (c) SetTickPeriod(0) → assert tick goroutine receives stop signal; (d) SetTickPeriod(50ms) then SetTickPeriod(10ms) → assert re-schedule with new period; (e) panic in proxy_on_tick → assert tick goroutine survives + envoy_go.failures counter increment; (f) concurrent N=100 stream contexts on one RootVM → assert no per-stream tick storm (tick fires once per RootVM tick period). Delete the timer placeholder from stubs_25_2.go.

- [ ] **Step 1: Write failing tests** at `internal/wasm/tick_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestTick
# Expected: FAIL (tick goroutine + SetTickPeriod not yet implemented)
```

- [ ] **Step 2: Implement `internal/wasm/tick.go`** per Q5 + R-25.2-9 + ADR-0186 Clock seam.

- [ ] **Step 3: Implement `internal/wasm/abi/timer.go`** per §5.1 #30.

- [ ] **Step 4: Integrate tick goroutine spawn into `internal/wasm/root_vm.go`** — at NewRootVM, `go rv.tickRun(ctx)`; at Close, `close(rv.tickStop)` + WaitGroup wait. Delete the timer placeholder from stubs_25_2.go.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestTick
# Expected: PASS (fake-clock fixtures + 10ms floor + period=0 cancel + re-schedule + panic-recovery + concurrent-stream no-tick-storm verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 5 entry + commit**

```bash
git add internal/wasm/tick.go internal/wasm/abi/timer.go internal/wasm/tick_test.go internal/wasm/root_vm.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): tick.go + abi/timer.go — per-RootVM tick goroutine + 10ms floor + Clock seam FIRST co-consumer per Q5 + R-25.2-9

Phase 25.2 Task 5 (Tier B internal/wasm/abi/ family dispatches). NEW tick.go
materializes per-*RootVM tick goroutine per Q5 + R-25.2-9 + ADR-0205. for
{ select { case <-clock.After(effectivePeriod): rv.lockAndCall(proxy_on_tick,
rv.rootCtxID); case <-stop: return } }. effectivePeriod = max(period_ms,
10ms) per Q5 envoy-go-strict floor — prevents guest-driven CPU spin attacks.
SetTickPeriod(0) cancels via close(tickStop). FIRST co-consumer of phase-21
ADR-0186 Clock seam beyond phase-21 itself via WithRootClock — RATIFIES the
phase-21 extraction discipline. NEW abi/timer.go materializes proxy_set_
tick_period_milliseconds dispatch per §5.1 #30. tick.go panic-recovery
wrapper around proxy_on_tick — goroutine survives guest panic + envoy_go.
failures counter increment. Tests fake-clock fixtures + 10ms floor + period=
0 cancel + re-schedule + concurrent-stream no-tick-storm verified."
```

---

## Task 6: NEW `internal/wasm/shared_data.go` + `abi/shared_data.go` + CAS + envoy-go-strict caps per Q6 + R-25.2-10

**Files:**
- Create: `internal/wasm/shared_data.go` (~200-280 LoC; per-RootVM CAS-protected K-V map)
- Create: `internal/wasm/shared_data_test.go` (~300-450 LoC; CAS golden table + cap-boundary)
- Create: `internal/wasm/abi/shared_data.go` (~100-150 LoC; proxy_get/set_shared_data dispatch)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE shared-data placeholders; ~-10 LoC delta)
- Append: PROGRESS.md (Task 6 entry per D-P-PLAN-3)

This task lands the per-`*RootVM` CAS-protected shared-data map per Q6 + R-25.2-10. `sharedDataEntry struct { value []byte; cas uint32 }`; `sharedData map[string]sharedDataEntry`; `sync.RWMutex`. CAS semantic byte-exact from cpp-host: cas=0 unconditionally writes (returns new CAS value in subsequent get); cas>0 writes only if existing entry's CAS matches (returns `WasmResult::CasMismatch` (=8) on mismatch). envoy-go-strict caps: per-value 1 MiB cap (configurable via `envoy_go_strict_shared_data_value_cap_bytes` default 1048576); 1024-entry cap (configurable via `envoy_go_strict_shared_data_max_entries` default 1024). Cap exceeded returns `WasmResult::InternalFailure` (=10) + `wasm.<plugin>.shared_data_cap_exceeded` envoy-go-strict counter + `envoy_go.failures` co-increment per §2.25 + integration error log. The counter increments happen via per-RootVM stats reference (which Task 17 wires up via the `stats.go` extension — at Task 6 the counter-increment integration is via a forward-decl `vm.stats.SharedDataCapExceededInc()` reference that compiles against a placeholder method that Task 17 implements).

**Precondition:** Tasks 1 + 2 + 3 complete.
**Artifact:** `internal/wasm/shared_data.go` + `abi/shared_data.go` + `shared_data_test.go` materialized; CAS golden table + cap-boundary tests GREEN; shared-data placeholders deleted from stubs_25_2.go.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestSharedData` passes (CAS golden table per R-25.2-10: cas=0 always writes; cas>0 writes only on match; mismatch returns CasMismatch; cap-boundary tests: value-cap-exceeded → InternalFailure + counter; entry-cap-exceeded → InternalFailure + counter; concurrent-Set stress under sync.RWMutex); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 6 per 25.2 SPEC §3.1 shared_data.go + §5.1 #35-36 + Q6 + R-25.2-10. NEW `internal/wasm/shared_data.go` materializes the per-*RootVM `sharedData map[string]sharedDataEntry` + `sharedDataMu sync.RWMutex` + `sharedDataValCap uint32` + `sharedDataMaxEntries uint32`. `(*RootVM).SetSharedData(key string, value []byte, cas uint32) WasmResult`: acquire sharedDataMu Lock; if len(value) > sharedDataValCap → return InternalFailure + counter; if entry := sharedData[key]; entry exists then if cas != 0 && entry.cas != cas → return CasMismatch + cas value unchanged; else entry.value = value; entry.cas++; sharedData[key] = entry; return Ok; if entry doesn't exist then if len(sharedData) >= sharedDataMaxEntries → return InternalFailure + counter; else create new entry with value + cas=1; return Ok. `(*RootVM).GetSharedData(key string) (value []byte, cas uint32, status WasmResult)`: acquire sharedDataMu RLock; if entry := sharedData[key]; entry exists then return entry.value, entry.cas, Ok; else return nil, 0, NotFound. NEW `internal/wasm/abi/shared_data.go` materializes `GetSharedDataShim(ctx, vm, keyPtr, keySize, retValuePtrPtr, retValueSizePtr, retCasPtr) WasmResult` + `SetSharedDataShim(ctx, vm, keyPtr, keySize, valuePtr, valueSize, cas) WasmResult` — read key/value bytes from guest memory + delegate to *RootVM.GetSharedData / *RootVM.SetSharedData + write results to guest memory. Tests at `shared_data_test.go` materialize CAS golden table per R-25.2-10 + cap-boundary tests + concurrent-Set stress (N=100 goroutines under sync.RWMutex; -race clean). Delete the shared-data placeholders from stubs_25_2.go.

- [ ] **Step 1: Write failing tests** at `internal/wasm/shared_data_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestSharedData
# Expected: FAIL (functions not yet defined; CAS + cap-boundary assertions fail)
```

- [ ] **Step 2: Implement `internal/wasm/shared_data.go`** per Q6 + R-25.2-10 + CAS semantic byte-exact from cpp-host.

- [ ] **Step 3: Implement `internal/wasm/abi/shared_data.go`** per §5.1 #35-36.

- [ ] **Step 4: Delete shared-data placeholders** from `internal/wasm/abi/stubs_25_2.go`.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestSharedData
# Expected: PASS (CAS golden table + cap-boundary + concurrent-Set stress verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 6 entry + commit**

```bash
git add internal/wasm/shared_data.go internal/wasm/shared_data_test.go internal/wasm/abi/shared_data.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): shared_data.go + abi/shared_data.go — CAS + envoy-go-strict caps per Q6 + R-25.2-10

Phase 25.2 Task 6 (Tier B internal/wasm/abi/ family dispatches). NEW
shared_data.go materializes per-*RootVM CAS-protected K-V map per Q6 +
R-25.2-10. sharedDataEntry {value []byte; cas uint32}; sync.RWMutex. CAS
byte-exact from cpp-host: cas=0 unconditionally writes; cas>0 writes only
on match returning CasMismatch on mismatch. envoy-go-strict caps: per-value
1 MiB (configurable envoy_go_strict_shared_data_value_cap_bytes default
1048576); 1024-entry (configurable envoy_go_strict_shared_data_max_entries
default 1024). Cap exceeded → InternalFailure + shared_data_cap_exceeded
counter + envoy_go.failures co-increment per §2.25. NEW abi/shared_data.go
materializes proxy_get_shared_data + proxy_set_shared_data dispatch per §5.1
#35-36. CAS golden table per R-25.2-10 + cap-boundary + concurrent-Set
stress verified."
```

---

## Task 7: NEW `internal/wasm/foreign.go` + `abi/foreign.go` + EMPTY default registry per AMEND-A9 + R-25.2-8 + D-25.2-P3 closure (mutex-per-RootVM dispatch concurrency model per D-P-PLAN-9)

**Files:**
- Create: `internal/wasm/foreign.go` (~150-220 LoC; ForeignFunctionRegistry + EMPTY default + process-global)
- Create: `internal/wasm/foreign_test.go` (~250-380 LoC; D-P3 concurrent dispatch verification)
- Create: `internal/wasm/abi/foreign.go` (~120-180 LoC; proxy_call_foreign_function dispatch)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE foreign-fn placeholders; ~-5 LoC delta)
- Append: PROGRESS.md (Task 7 entry per D-P-PLAN-3 + **D-25.2-P3 closure evidence** — mutex-per-RootVM dispatch concurrency model RATIFIED per D-P-PLAN-9)

This task lands the foreign-function registration interface per AMEND-A9 + R-25.2-8 + D-25.2-P3 closure (mutex-per-RootVM dispatch concurrency model per PLAN-time D-P-PLAN-9). `ForeignFunctionFn func(ctx context.Context, args []byte) (result []byte, status WasmResult)`. `ForeignFunctionRegistry struct { mu sync.RWMutex; fns map[string]ForeignFunctionFn }`. NewForeignFunctionRegistry constructor. Register(name, fn) error returns error if name already registered. Get(name) (ForeignFunctionFn, bool) under RLock. Process-global var `DefaultForeignFunctionRegistry = NewForeignFunctionRegistry()` consumed by wasm filter factory. **EMPTY default registry per AMEND-A9** — envoy-go ships ZERO default foreign functions (vs upstream's 10: verify_signature + sign + compress + uncompress + set_envoy_filter_state + clear_route_cache + expr_create + expr_evaluate + expr_delete + declare_property); operators MUST explicitly enable the `proxy_call_foreign_function` capability AND register specific foreign functions via `wasm.RegisterForeignFunction(name, fn)` at boot; unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream cpp-host `src/exports.cc:147-184`. envoy-go-strict departure record #5 at BEHAVIOR_CONTRACT.md per §9 record #5 (Task 22). The dispatch concurrency model per D-25.2-P3 PLAN-time closure (D-P-PLAN-9): mutex-per-RootVM — the dispatched function executes synchronously inside `*RootVM.CallForeignFunction(ctx, streamCtxID, name, args)`; the RootVM dispatch lock IS held during invocation (same lock as per-stream call frame); panic-recovery wrapper applies (Go panic in `ForeignFunctionFn` → recover() → log + `envoy_go.failures` counter increment + return `WasmResult::InternalFailure` to guest). The `foreign_function_denied` envoy-go-strict counter increments on the NotFound path (unregistered name).

**Precondition:** Tasks 1 + 2 + 3 complete.
**Artifact:** `internal/wasm/foreign.go` + `abi/foreign.go` + `foreign_test.go` materialized; EMPTY default registry per AMEND-A9; foreign-fn placeholders deleted from stubs_25_2.go; D-25.2-P3 closure evidence recorded (mutex-per-RootVM dispatch concurrency model RATIFIED + concurrent-dispatch test GREEN).
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestForeignFunction` passes (Register/Get + EMPTY-default NotFound behavior + capability-gated default-deny + registered-then-deregister-then-call sequence + D-P3 closure: concurrent N=100 streams dispatch same foreign function → verify mutex-per-RootVM serialization via probe counter + no cross-stream argument leak); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 7 per 25.2 SPEC §3.1 foreign.go + §5.1 #38 + AMEND-A9 + R-25.2-8 + D-P-PLAN-9 mutex-per-RootVM dispatch concurrency model. NEW `internal/wasm/foreign.go` materializes `ForeignFunctionFn` type + `ForeignFunctionRegistry struct` + NewForeignFunctionRegistry constructor + Register/Get methods + process-global `DefaultForeignFunctionRegistry = NewForeignFunctionRegistry()`. Register acquires Lock + checks for duplicate name (returns error if already registered) + adds to map. Get acquires RLock + looks up + returns (fn, ok). NEW `internal/wasm/abi/foreign.go` materializes `CallForeignFunctionShim(ctx, vm, nameDataPtr, nameSize, argsDataPtr, argsSize, retResultsDataPtr, retResultsSizePtr) WasmResult` — read name + args bytes from guest memory + delegate to `vm.rootVM.CallForeignFunction(ctx, vm.streamCtxID, name, args)` which (a) looks up function via foreignReg.Get(name) RLock; (b) if not found → return NotFound + increment foreign_function_denied counter; (c) if found → invoke function synchronously inside per-stream call frame WITH RootVM dispatch lock HELD (same lock as per-stream call frame; no additional lock acquired) WITH panic-recovery wrapper (deferred recover() → log + envoy_go.failures counter increment + return InternalFailure). The CallForeignFunction method lives on *RootVM (anchored at root_vm.go) — Task 7 extends root_vm.go with this method. Tests at `foreign_test.go` materialize: (a) Register adds + Get retrieves; (b) duplicate Register returns error; (c) EMPTY default registry — Get("verify_signature") returns (nil, false); (d) capability-gated — proxy_call_foreign_function capability denied → InternalFailure per ADR-0204 inherited; (e) registered-then-deregister-then-call sequence; (f) D-P3 closure: concurrent N=100 stream contexts dispatch same registered function via *RootVM.CallForeignFunction → assert serialization via probe counter (the function increments a probe counter under its own mutex; the test verifies no race; verifies mutex-per-RootVM serialization holds — each call observes the others); (g) no cross-stream argument leak (each call receives its own args bytes). PROGRESS.md Task 7 entry records D-P3 closure evidence + the concurrency model RATIFIED per D-P-PLAN-9. Delete foreign-fn placeholders from stubs_25_2.go.

- [ ] **Step 1: Write failing tests** at `internal/wasm/foreign_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestForeignFunction
# Expected: FAIL (ForeignFunctionRegistry + CallForeignFunction not yet implemented)
```

- [ ] **Step 2: Implement `internal/wasm/foreign.go`** per AMEND-A9 + EMPTY default registry.

- [ ] **Step 3: Implement `internal/wasm/abi/foreign.go`** per §5.1 #38 + D-P-PLAN-9 mutex-per-RootVM dispatch.

- [ ] **Step 4: Extend `internal/wasm/root_vm.go`** with `CallForeignFunction(ctx, streamCtxID, name, args) (result []byte, status WasmResult)` method per D-P-PLAN-9. Delete foreign-fn placeholders from stubs_25_2.go.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestForeignFunction
# Expected: PASS (Register/Get + EMPTY-default NotFound + capability-gated + D-P3 concurrent-dispatch + no cross-stream leak verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 7 entry with D-25.2-P3 closure evidence + commit**

```bash
git add internal/wasm/foreign.go internal/wasm/foreign_test.go internal/wasm/abi/foreign.go internal/wasm/root_vm.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): foreign.go + abi/foreign.go — ForeignFunctionRegistry per AMEND-A9 + R-25.2-8 + D-25.2-P3 mutex-per-RootVM CLOSED

Phase 25.2 Task 7 (Tier B internal/wasm/abi/ family dispatches). NEW
foreign.go materializes ForeignFunctionRegistry per AMEND-A9 + R-25.2-8.
EMPTY default registry — envoy-go ships ZERO default foreign functions
(vs upstream's 10). Operators MUST explicitly enable proxy_call_foreign_
function capability AND register via wasm.RegisterForeignFunction(name, fn)
at boot; unregistered names return WasmResult::NotFound (=1) byte-faithful
to cpp-host src/exports.cc:147-184. envoy-go-strict departure record #5 at
BEHAVIOR_CONTRACT.md (Task 22). NEW abi/foreign.go materializes proxy_call_
foreign_function dispatch per §5.1 #38. *RootVM.CallForeignFunction extended.
D-25.2-P3 CLOSED at this PLAN session per D-P-PLAN-9 — mutex-per-RootVM
dispatch concurrency model RATIFIED: dispatched function executes synchronously
inside per-stream call frame; RootVM dispatch lock HELD during invocation
(same lock as per-stream call frame; no additional lock); panic-recovery
wrapper applies (Go panic → recover() + log + envoy_go.failures counter +
return InternalFailure). foreign_function_denied counter increments on
NotFound path. Concurrent N=100 dispatch + no cross-stream leak verified."
```

---

## Task 8: NEW `internal/wasm/http_call.go` + `abi/http_call.go` + proxy_http_call dispatch + cancel-at-destruction per AMEND-B3 + R-25.2-3 + http_call_response_after_close counter

**Files:**
- Create: `internal/wasm/http_call.go` (~300-450 LoC; proxy_http_call dispatch via *httpclient.Client + cancel-at-destruction)
- Create: `internal/wasm/http_call_test.go` (~400-550 LoC; cancel-at-destruction race-test + http_call_response_after_close counter)
- Create: `internal/wasm/abi/http_call.go` (~150-220 LoC; proxy_http_call dispatch + callback route)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE http_call placeholders; ~-5 LoC delta)
- Modify: `internal/wasm/stream_context.go` (~+50-80 LoC; integrate cancel-at-destruction at Close per AMEND-B3)
- Append: PROGRESS.md (Task 8 entry per D-P-PLAN-3)

This task lands the proxy_http_call dispatch via per-`*RootVM` `*httpclient.Client` per Q4 + R-25.2-3 + AMEND-B3. AsyncClient request lifecycle; call_id monotonic allocation; httpCalls map tracking {call_id → pendingHttpCall{streamCtxID, deadline}}; cancel-at-destruction via `*StreamContext.Close` → iterate httpCalls + filter by streamCtxID + invoke `httpclient.Cancel(handle)` for each outstanding call dispatched from this stream; defensive token-miss guard at response arrival — if `httpCalls[call_id]` is absent OR the originating streamCtxID's stream context is gone → `http_call_response_after_close` envoy-go-strict counter increment + drop (NO host-side panic). BadArgument-on-unknown-cluster per Q4 — cluster lookup fails → return `BadArgument` + `http_call_dispatch_unknown_cluster` counter increment. RE-CONSUMES phase-20 ADR-0177 `internal/httpclient/` at 3rd-or-later co-consumer (phase-22.2 `:httpCall()` was second) — CLOSES parent §13-R6 RATIFIED-PENDING-IMPL anchor. NO API extension on httpclient (phase-22.2 cluster-based dispatch covers byte-for-byte). The response callback `proxy_on_http_call_response(streamCtxID, call_id, num_headers, body_size, num_trailers)` routes asynchronously to the originating `*StreamContext.CallProxyOnHttpCallResponse` via the RootVM's httpCalls map lookup.

**Precondition:** Tasks 1 + 2 + 3 complete; the existing phase-20 `internal/httpclient/` ADR-0177 + phase-22.2 cluster-based dispatch API are at master tip.
**Artifact:** `internal/wasm/http_call.go` + `abi/http_call.go` + `http_call_test.go` materialized; `stream_context.go` integrates cancel-at-destruction at Close; http_call placeholders deleted from stubs_25_2.go.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestHttpCall` passes (proxy_http_call dispatch with mock *httpclient.Client; cancel-at-destruction race-test: StreamContext.Close cancels in-flight requests + late-response-after-close → http_call_response_after_close counter increment + defensive token-miss path; BadArgument on unknown cluster + http_call_dispatch_unknown_cluster counter; concurrent N=100 httpCall dispatches from same RootVM verify call_id allocation isolation + response routing isolation); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 8 per 25.2 SPEC §3.1 http_call.go + §5.1 #37 + Q4 + R-25.2-3 + AMEND-B3. NEW `internal/wasm/http_call.go` materializes per-*RootVM httpCall state: `pendingHttpCall struct { streamCtxID uint32; deadline time.Time; cancelFn func() }`; `httpCalls map[uint32]*pendingHttpCall`; `httpCallsMu sync.Mutex`; `nextCallID uint32`. `(*RootVM).DispatchHttpCall(ctx, streamCtxID, cluster, headers []HeaderPair, body []byte, trailers []HeaderPair, timeoutMs uint32) (callID uint32, result WasmResult)`: acquire httpCallsMu Lock; if cluster lookup via vm.httpClient.GetCluster(cluster) returns nil → return 0, BadArgument + http_call_dispatch_unknown_cluster counter increment; else allocate callID = nextCallID; nextCallID++; deadline = time.Now() + timeoutMs*time.Millisecond; dispatch via httpclient.Dispatch(...) returns a cancelFn handle; insert into httpCalls map; increment http_call_dispatched counter; return callID, Ok. On async response arrival via httpclient callback: acquire httpCallsMu Lock; lookup pendingHttpCall by callID; if absent → http_call_response_after_close counter increment + drop (no panic); else if originating *StreamContext is gone → http_call_response_after_close counter increment + drop; else delete from httpCalls + RELEASE Lock + route to streamCtx.CallProxyOnHttpCallResponse(callID, num_headers, body_size, num_trailers) + increment http_call_response counter. `(*StreamContext).Close(ctx)` extension: iterate parent rootVM.httpCalls + filter entries where streamCtxID == sc.ctxID + invoke entry.cancelFn() for each + delete from httpCalls. NEW `internal/wasm/abi/http_call.go` materializes `HttpCallShim(ctx, vm, clusterDataPtr, clusterSize, headersDataPtr, headersSize, bodyDataPtr, bodySize, trailersDataPtr, trailersSize, timeoutMs uint32, retCallIDPtr) WasmResult` — read cluster name + decode pairs for headers/trailers via existing 25.1 pairs.go + read body bytes + delegate to vm.rootVM.DispatchHttpCall + write callID to guest memory at retCallIDPtr. Tests at `http_call_test.go` materialize: (a) DispatchHttpCall with mock httpclient.Client to cluster_a → assert http_call_dispatched counter increment + valid callID; (b) cancel-at-destruction race-test: dispatch call → before response → StreamContext.Close → assert cancelFn invoked + httpCalls entry deleted; (c) late-response after Close → assert http_call_response_after_close counter increment + drop (no panic); (d) defensive token-miss path (response with non-existent callID) → assert http_call_response_after_close counter increment + drop; (e) BadArgument on unknown cluster → assert http_call_dispatch_unknown_cluster counter increment; (f) concurrent N=100 httpCall dispatches from same RootVM → assert call_id allocation isolation (all unique IDs) + response routing isolation (each response routes to its originating StreamContext). Delete http_call placeholders from stubs_25_2.go.

- [ ] **Step 1: Write failing tests** at `internal/wasm/http_call_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestHttpCall
# Expected: FAIL (DispatchHttpCall not yet implemented)
```

- [ ] **Step 2: Implement `internal/wasm/http_call.go`** per Q4 + R-25.2-3 + AMEND-B3 cancel-at-destruction + http_call_response_after_close counter.

- [ ] **Step 3: Implement `internal/wasm/abi/http_call.go`** per §5.1 #37.

- [ ] **Step 4: Extend `internal/wasm/stream_context.go`** with cancel-at-destruction at Close per AMEND-B3. Delete http_call placeholders from stubs_25_2.go.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestHttpCall
# Expected: PASS (dispatch + cancel-at-destruction + late-response counter + BadArgument + concurrent isolation verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 8 entry + commit**

```bash
git add internal/wasm/http_call.go internal/wasm/http_call_test.go internal/wasm/abi/http_call.go internal/wasm/stream_context.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): http_call.go + abi/http_call.go — proxy_http_call dispatch + cancel-at-destruction + http_call_response_after_close per Q4 + R-25.2-3 + AMEND-B3

Phase 25.2 Task 8 (Tier B internal/wasm/abi/ family dispatches). NEW
http_call.go materializes proxy_http_call dispatch via per-*RootVM
*httpclient.Client per Q4 + R-25.2-3 + AMEND-B3. AsyncClient request
lifecycle; call_id monotonic; httpCalls map tracking; cancel-at-destruction
via *StreamContext.Close — iterate + cancel outstanding calls dispatched
from this stream byte-faithful to cpp-host context.cc:1900-1905. Defensive
token-miss guard at response arrival → http_call_response_after_close
envoy-go-strict counter (per AMEND-B3 recommendation; defensive observability
for stray late responses). BadArgument-on-unknown-cluster per Q4 + http_call_
dispatch_unknown_cluster counter. RE-CONSUMES phase-20 ADR-0177 at 3rd-or-
later co-consumer (phase-22.2 was second) — CLOSES parent §13-R6 RATIFIED-
PENDING-IMPL anchor. NO API extension on httpclient (phase-22.2 cluster-
based dispatch covers byte-for-byte). NEW abi/http_call.go materializes
proxy_http_call host shim per §5.1 #37. Concurrent N=100 dispatch + cancel +
late-response counter + BadArgument verified."
```

---

## Tier C — NEW packages + lua MIGRATION + property roster (Tasks 9-13; partly parallelizable per D-P-PLAN-7)

## Task 9: NEW `internal/filterstate/` framework primitive per ADR-0207 + R-25.2-6 + Q7 + AMEND-B4

**Files:**
- Create: `internal/filterstate/filterstate.go` (~250-400 LoC; Bucket + FilterStateObject + StateType + Set/Get/Keys)
- Create: `internal/filterstate/filterstate_test.go` (~250-380 LoC; Set/Get round-trip + StateType conflict)
- Create: `internal/filterstate/bucket_concurrency_test.go` (~150-220 LoC; RWMutex discipline)
- Create: `internal/filterstate/filterstateobject_test.go` (~150-220 LoC; interface conformance)
- Create: `internal/filterstate/doc.go` (~40-60 LoC; package doc per ADR-0207)
- Append: PROGRESS.md (Task 9 entry per D-P-PLAN-3)

This task lands the NEW `internal/filterstate/` framework primitive per ADR-0207 + R-25.2-6 + Q7 + AMEND-B4. Generic per-stream filter-state primitive at consumer-#2 scope per the EXTRACT-NOW-on-second-consumer discipline established at phase-22.1 for `internal/lua/`. `FilterStateObject` interface (Marshal/Unmarshal/HasData/StateType); `StateType` const discriminator (StateTypeReadOnly=0 vs StateTypeMutable=1); `Bucket struct { mu sync.RWMutex; items map[string]FilterStateObject }` per-stream accessor + NewBucket constructor + Set(key, obj) error (mutable overrides; read-only with same key as Mutable → reject) + Get(key) (FilterStateObject, bool) + Keys() []string (for property-tree enumeration at proxy-wasm property-path `filter_state.*` dispatch). Per the AMEND-B4 SUBSTANTIVE REFINEMENT: `upstream_filter_state` is a DISTINCT root co-equal to `filter_state` — BRAINSTORM Q7 OMITTED this. The Bucket primitive serves BOTH roots (the `*compiledConfig` constructs TWO `*Bucket` instances per `*RootVM` — one for downstream filter_state, one for upstream_filter_state — Task 14 wires this into compiledConfig). EXPLICIT API-REVISION ALLOWANCE clause anchored at ADR-0207 §Decision body at Task 22 (the primitive's API is provisional at consumer #2; future consumers — rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families — MAY require API revision after empirical validation). Mirrors phase-22.1 ADR-0188 + phase-22.2 ADR-0190 + phase-25.1 ADR-0202 allowance pattern at the symmetric scope.

**Precondition:** Task 1 complete (the `*RootVM` type exists for future integration; at Task 9 the filterstate package is fully self-contained — no internal/wasm/ dependency at Task 9; the wiring lands at Task 13 + Task 14).
**Artifact:** `internal/filterstate/` package materialized with 4 production files (doc.go + filterstate.go) + 3 test files; Bucket + FilterStateObject + StateType API GREEN.
**Acceptance:** `go test -count=1 -race -v ./internal/filterstate/...` passes (Set/Get/Keys round-trip + read-only-vs-mutable conflict + nil-handling + Marshal/Unmarshal round-trip + RWMutex discipline + concurrent-read concurrent-add tests + interface conformance + edge cases); `golangci-lint run ./internal/filterstate/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 9 per 25.2 SPEC §3.2 + ADR-0207 + R-25.2-6 + Q7 + AMEND-B4. NEW package `internal/filterstate/` with: (a) `doc.go` covering package overview + ADR-0207 cross-ref + Q7 + AMEND-B4 (upstream_filter_state distinct root co-equal to filter_state) + EXPLICIT API-REVISION ALLOWANCE clause for consumer #3+; (b) `filterstate.go` materializes the `FilterStateObject` interface (Marshal() ([]byte, error); Unmarshal([]byte) error; HasData() bool; StateType() StateType) + `StateType` const (StateTypeReadOnly=0; StateTypeMutable=1) + `Bucket` struct + NewBucket + Set + Get + Keys methods per 25.2 SPEC §3.2. Set logic: acquire mu Lock; if existing entry has StateTypeMutable AND new obj has StateTypeReadOnly → return error ("filterstate: cannot replace mutable entry with read-only object"); otherwise items[key] = obj + return nil. Get logic: acquire mu RLock; return items[key], ok. Keys logic: acquire mu RLock; return sorted slice of keys (for deterministic property-tree enumeration). Tests at `filterstate_test.go` materialize: Set/Get round-trip; Set with nil obj; read-only-vs-mutable conflict (try to replace Mutable with ReadOnly → expect error; try to replace ReadOnly with Mutable → expect override OK); Keys returns sorted list; Marshal/Unmarshal via test FilterStateObject implementation. Tests at `bucket_concurrency_test.go` materialize: N=100 goroutines concurrent Get + Set under sync.RWMutex (-race clean). Tests at `filterstateobject_test.go` materialize: interface conformance verification (test type satisfies FilterStateObject); HasData false on empty obj; StateType discriminator consistency.

- [ ] **Step 1: Write failing tests** at `internal/filterstate/filterstate_test.go` + `bucket_concurrency_test.go` + `filterstateobject_test.go`

```bash
go test -count=1 -race -v ./internal/filterstate/...
# Expected: FAIL (package not yet exists; functions not yet defined)
```

- [ ] **Step 2: Author `internal/filterstate/doc.go` + `filterstate.go`** per 25.2 SPEC §3.2 + ADR-0207.

- [ ] **Step 3: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/filterstate/...
# Expected: PASS (Set/Get/Keys + StateType conflict + concurrency + interface conformance verified)
go vet ./internal/filterstate/... && golangci-lint run ./internal/filterstate/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 9 entry + commit**

```bash
git add internal/filterstate/ docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filterstate): NEW framework primitive per ADR-0207 + R-25.2-6 + Q7 + AMEND-B4

Phase 25.2 Task 9 (Tier C NEW packages + lua MIGRATION + property roster).
NEW internal/filterstate/ framework primitive at consumer-#2 scope per
ADR-0207 + R-25.2-6 + Q7 + AMEND-B4. Generic per-stream filter-state Bucket
+ FilterStateObject interface (Marshal/Unmarshal/HasData/StateType) +
StateType discriminator (read-only vs mutable; mutable overrides; read-only-
vs-mutable conflict rejected). EXTRACT-NOW-on-second-consumer discipline
mirrors phase-22.1 internal/lua/. Consumer #1 = phase-22.2 lua MIGRATES at
Task 10; consumer #2 = phase-25.2 wasm filter_state.* + upstream_filter_state.*
property branches per AMEND-B4 (upstream_filter_state DISTINCT root co-equal
to filter_state — BRAINSTORM Q7 OMITTED this). EXPLICIT API-REVISION ALLOWANCE
clause anchored at ADR-0207 §Decision body Task 22 for consumer #3+ (rbac
filter-state read; ext_authz inject; ext_proc pass-through; new filter
families). Set/Get/Keys + RWMutex discipline + StateType conflict verified."
```

---

## Task 10: phase-22.2 lua MIGRATION — `internal/filter/http/lua/filterstate.go` REWRITE per ADR-0207 §3.4 MIGRATES

**Files:**
- Modify: `internal/filter/http/lua/filterstate.go` (~+50-100 LoC delta — rewrite to delegate to `*filterstate.Bucket`)
- Append: PROGRESS.md (Task 10 entry per D-P-PLAN-3)

This task REWRITES phase-22.2's `internal/filter/http/lua/filterstate.go` to delegate to `internal/filterstate/*Bucket` via a thin adapter per ADR-0207 + §3.4 MIGRATES + §14.5. The `:filterState()` Lua surface stays UNCHANGED — the Lua bridge's `:filterState():get(name)` + `:filterState():set(name, value)` accessors delegate to `*Bucket.Get` / `*Bucket.Set`. The 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 (mutation exposure + typed Lua-value marshaling) carry forward UNCHANGED — the migration is INTERNAL only (replacing the in-package `map[string]any` storage with delegation to `*filterstate.Bucket`); no Lua-visible behavior change. Migration delta ~50-100 LoC inside `internal/filter/http/lua/`; no test wording change. Per §14.5: the existing phase-22.2 lua filterstate tests MUST stay byte-identical (non-breaking) — 100% green run + no test wording change. **CRITICAL ACCEPTANCE per §14.5:** `go test -count=1 -race ./internal/filter/http/lua/...` MUST be GREEN without modifying any existing phase-22.2 test files; the only acceptable test change is updating the in-package map[string]any references to *filterstate.Bucket references inside the production filterstate.go file (the test files MUST stay UNCHANGED).

**Precondition:** Task 9 complete (`internal/filterstate/*Bucket` API materialized).
**Artifact:** `internal/filter/http/lua/filterstate.go` REWRITES non-breaking to delegate to `*filterstate.Bucket`; existing phase-22.2 lua filterstate tests pass UNCHANGED.
**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/...` GREEN without modifying any test files (verify via `git diff --stat HEAD -- internal/filter/http/lua/*_test.go` returns empty after the task); `:filterState()` Lua surface unchanged (the SPEC's §3.4 MIGRATES discipline + the §14.5 Layer E verification); the 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 carry forward unchanged (mutation exposure + typed Lua-value marshaling).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 10 per 25.2 SPEC §3.4 MIGRATES + §14.5. REWRITE `internal/filter/http/lua/filterstate.go` to delegate to `internal/filterstate/*Bucket` via a thin adapter. The existing phase-22.2 implementation has an in-package `map[string]any` (per phase-22.2 SPEC §3.X); the rewrite replaces the map with a *filterstate.Bucket field. The `:filterState():get(name)` Lua bridge method maps Lua call → `bucket.Get(name)` → unwrap FilterStateObject → return to Lua. The `:filterState():set(name, value)` Lua bridge method maps Lua call → wrap value in a FilterStateObject implementation (the existing phase-22.2 typed Lua-value marshaling per AMEND-22.2-4 carries forward) → `bucket.Set(name, obj)` → return to Lua. The 2 envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 (mutation exposure + typed Lua-value marshaling) MUST carry forward unchanged — they are Lua-bridge-layer behaviors, NOT primitive-layer behaviors; the *Bucket primitive doesn't know about Lua values, it just stores opaque FilterStateObject implementations. The migration is INTERNAL — no public API changes; the `:filterState()` Lua surface stays UNCHANGED. **CRITICAL:** the existing phase-22.2 lua filterstate test files MUST stay UNCHANGED — verify via `git diff --stat HEAD~1 -- internal/filter/http/lua/*_test.go` returns empty AFTER the task commit. Any modification of phase-22.2 lua filterstate tests is a RED FLAG (the migration MUST be non-breaking per §14.5). After implementing the rewrite, run `go test -count=1 -race ./internal/filter/http/lua/...` → must be GREEN.

- [ ] **Step 1: Re-read existing phase-22.2 `internal/filter/http/lua/filterstate.go` + its test file** to understand the in-package map[string]any pattern + the typed Lua-value marshaling per AMEND-22.2-4.

- [ ] **Step 2: Rewrite `internal/filter/http/lua/filterstate.go`** to delegate to `*filterstate.Bucket`. Add import of `internal/filterstate`. Replace in-package map[string]any field with *filterstate.Bucket field. Implement a small `luaFilterStateObject` adapter type that wraps a Lua value + satisfies `filterstate.FilterStateObject` interface (Marshal serializes via existing phase-22.2 typed marshaling; Unmarshal deserializes via existing reverse; HasData returns whether the value is non-nil; StateType returns Mutable by default per the phase-22.2 mutation-exposure divergence per AMEND-22.2-4).

- [ ] **Step 3: Run the existing phase-22.2 lua filterstate tests UNCHANGED**

```bash
go test -count=1 -race -v ./internal/filter/http/lua/... -run TestFilterState
# Expected: PASS (existing phase-22.2 tests pass without modification)
git diff --stat HEAD -- internal/filter/http/lua/*_test.go
# Expected: empty (NO test file modifications per §14.5 non-breaking migration)
```

- [ ] **Step 4: Run vet + lint clean**

```bash
go vet ./internal/filter/http/lua/... && golangci-lint run ./internal/filter/http/lua/...
# Expected: each clean
```

- [ ] **Step 5: Append PROGRESS.md Task 10 entry + commit**

```bash
git add internal/filter/http/lua/filterstate.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "refactor(internal/filter/http/lua): MIGRATE filterstate.go to *filterstate.Bucket per ADR-0207 §3.4

Phase 25.2 Task 10 (Tier C NEW packages + lua MIGRATION + property roster).
REWRITE internal/filter/http/lua/filterstate.go to delegate to internal/
filterstate/*Bucket per ADR-0207 + §3.4 MIGRATES. INTERNAL migration only —
:filterState() Lua surface stays UNCHANGED. luaFilterStateObject adapter
wraps Lua value + satisfies filterstate.FilterStateObject interface. The 2
envoy-go-strict divergences from phase-22.2 AMEND-22.2-4 (mutation exposure
+ typed Lua-value marshaling) carry forward UNCHANGED. Migration delta
~50-100 LoC inside internal/filter/http/lua/. CRITICAL: existing phase-22.2
lua filterstate tests pass UNCHANGED (NO test file modifications per §14.5
non-breaking migration; verified via git diff --stat HEAD -- *_test.go
returns empty). Consumer #1 of new internal/filterstate/ primitive lands;
consumer #2 = phase-25.2 wasm filter_state.* property branches at Task 13."
```

---

## Task 11: NEW `internal/stats/dynamic/` infrastructure subpackage per ADR-0208 + AMEND-B2 + R-25.2-7

**Files:**
- Create: `internal/stats/dynamic/dynamic.go` (~300-450 LoC; Registry + MetricID + MetricType + per-plugin scope)
- Create: `internal/stats/dynamic/dynamic_test.go` (~300-450 LoC; Register/Increment/Record/Get + signed-delta semantics)
- Create: `internal/stats/dynamic/dynamic_admin_test.go` (~200-300 LoC; admin /stats enumeration + name format pin per R-25.2-2)
- Create: `internal/stats/dynamic/dynamic_concurrency_test.go` (~150-220 LoC; RWMutex discipline + cap-boundary race)
- Create: `internal/stats/dynamic/doc.go` (~40-60 LoC; package doc per ADR-0208 + AMEND-B2)
- Append: PROGRESS.md (Task 11 entry per D-P-PLAN-3)

This task lands the NEW `internal/stats/dynamic/` infrastructure subpackage per ADR-0208 + AMEND-B2 + R-25.2-7. Thin wrapper over `internal/stats/` registry for `wasmcustom.<custom_name>` dynamic-stats namespace. `MetricID uint32` opaque token. `MetricType int` const: MetricTypeCounter=0 + MetricTypeGauge=1 + MetricTypeHistogram=2 per AMEND-B2 byte-pin. `Registry struct { mu sync.RWMutex; pluginScope *stats.Scope; maxEntries uint32; byID map[MetricID]registryEntry; byName map[string]MetricID; nextID MetricID }`. NewRegistry(pluginScope, maxEntries) — pluginScope = `stats.RootScope.Subscope("wasm").Subscope(pluginName)`. Register(metricType, name) (MetricID, error) — idempotent (returns cached MetricID if name already registered); allocates new MetricID otherwise; cap-exceeded → ErrCapExceeded. Increment(id, delta int64) — signed delta per AMEND-B2; ErrBadArgument on Histogram. Record(id, value uint64) — unsigned value per AMEND-B2; ErrBadArgument on Counter. Get(id) (uint64, error) — returns current value. EnumerateForAdmin(fn func(name string, value uint64)) walks registry for /stats lazy enumeration. **Per-plugin Registry SCOPE discipline per AMEND-B2 REFINEMENT** — each `*compiledConfig` constructs its own `*Registry` from `internal/stats/dynamic.go` rooted at the plugin's stat scope; cross-plugin name collisions are namespaced via the parent scope, NOT by prefix interpolation (BRAINSTORM Q9 hypothesized `wasmcustom.<plugin_name>.<custom_name>` — REFUTED; actual is `wasmcustom.<custom_name>` ONLY).

**Precondition:** None — Task 11 is self-contained within `internal/stats/dynamic/` (depends on the existing `internal/stats/` package which is at master tip per phase-06.1).
**Artifact:** `internal/stats/dynamic/` package materialized with 2 production files (doc.go + dynamic.go) + 3 test files; Registry + MetricID + signed-i64 delta + name format pin GREEN.
**Acceptance:** `go test -count=1 -race -v ./internal/stats/dynamic/...` passes (Register/Increment/Record/Get round-trip + signed-delta extremes per AMEND-B2 + idempotent-Register + ErrCapExceeded threshold + ErrBadArgument enforcement + name format pin `wasmcustom.<custom_name>` per R-25.2-2 + concurrent-Register stress at cap-boundary race); `golangci-lint run ./internal/stats/dynamic/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 11 per 25.2 SPEC §3.3 + ADR-0208 + AMEND-B2 + R-25.2-7. NEW package `internal/stats/dynamic/` with: (a) `doc.go` covering ADR-0208 cross-ref + AMEND-B2 (signedness + namespace per-plugin Registry SCOPE) + Q9; (b) `dynamic.go` materializes Registry + MetricID + MetricType + NewRegistry + Register + Increment + Record + Get + EnumerateForAdmin per 25.2 SPEC §3.3 production signatures. Register logic: acquire mu Lock; if existing entry byName[name] → return cached MetricID + nil (idempotent); if len(byID) >= maxEntries → return 0 + ErrCapExceeded; allocate id = nextID; nextID++; construct stats Counter/Gauge/Histogram via pluginScope.NewCounter("wasmcustom."+name) (per AMEND-B2 namespace pin — `wasmcustom.<custom_name>` ONLY); add to byID + byName; return id + nil. Increment logic: acquire mu RLock; lookup id → entry; if entry not found → return ErrNotFound; if entry.metricType == Histogram → return ErrBadArgument; entry.counter.Add(delta) for Counter (cast int64→int64; the stats.Counter API uses int64 per phase-06.1) OR entry.gauge.Add(delta) for Gauge. Record logic: similar; ErrBadArgument on Counter. Get logic: acquire mu RLock; lookup; return current value (Counter.Value() / Gauge.Value() / Histogram.Sum() or similar). EnumerateForAdmin acquires mu RLock + iterates byID + invokes fn(entry.name, current_value) for each. Tests materialize Register/Increment/Record/Get round-trip + signed-i64 delta extremes (delta=-1; delta=int64::MIN; delta=int64::MAX) + idempotent-Register (re-register same name → same MetricID) + ErrCapExceeded (Register 1024 names → 1025th returns ErrCapExceeded) + ErrBadArgument enforcement (Increment on Histogram; Record on Counter); name format pin `wasmcustom.<custom_name>` via admin /stats enumeration round-trip + concurrent-Register stress at cap-boundary (N goroutines racing to register near 1024 cap → exactly 1024 succeed).

- [ ] **Step 1: Write failing tests** at `internal/stats/dynamic/dynamic_test.go` + `dynamic_admin_test.go` + `dynamic_concurrency_test.go`

```bash
go test -count=1 -race -v ./internal/stats/dynamic/...
# Expected: FAIL (package not yet exists; functions not yet defined)
```

- [ ] **Step 2: Author `internal/stats/dynamic/doc.go` + `dynamic.go`** per 25.2 SPEC §3.3 + ADR-0208 + AMEND-B2.

- [ ] **Step 3: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/stats/dynamic/...
# Expected: PASS (Register/Increment/Record/Get + signed-delta + idempotent + cap-boundary + name format pin verified)
go vet ./internal/stats/dynamic/... && golangci-lint run ./internal/stats/dynamic/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 11 entry + commit**

```bash
git add internal/stats/dynamic/ docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/stats/dynamic): NEW infrastructure subpackage per ADR-0208 + AMEND-B2 + R-25.2-7

Phase 25.2 Task 11 (Tier C NEW packages + lua MIGRATION + property roster).
NEW internal/stats/dynamic/ infrastructure subpackage per ADR-0208 + AMEND-
B2 + R-25.2-7. Thin wrapper over internal/stats/ registry for wasmcustom.
<custom_name> dynamic-stats namespace. Registry + MetricID (uint32) +
MetricType const (Counter=0, Gauge=1, Histogram=2 per AMEND-B2). NewRegistry
takes pluginScope = stats.RootScope.Subscope('wasm').Subscope(pluginName) +
maxEntries. Register idempotent + ErrCapExceeded threshold. Increment signed
int64 delta per AMEND-B2 (allows negative gauge); ErrBadArgument on Histogram.
Record unsigned uint64 value per AMEND-B2; ErrBadArgument on Counter. Per-
plugin Registry SCOPE discipline per AMEND-B2 REFINEMENT — namespace is
wasmcustom.<custom_name> ONLY (NO plugin prefix as BRAINSTORM Q9 hypothesized;
per-plugin isolation via per-plugin Registry SCOPE). Concurrent-Register
cap-boundary stress + signed-delta extremes + name format pin per R-25.2-2
admin /stats enumeration verified."
```

---

## Task 12: NEW `internal/wasm/dynamic_stats.go` + `abi/metrics.go` — wraps per-plugin `*dynamic.Registry` per AMEND-B2

**Files:**
- Create: `internal/wasm/dynamic_stats.go` (~150-220 LoC; wraps per-plugin *dynamic.Registry)
- Create: `internal/wasm/dynamic_stats_test.go` (~300-450 LoC; round-trip + cap-exceeded counter)
- Create: `internal/wasm/abi/metrics.go` (~150-220 LoC; 4-hostcall dispatch)
- Create: `internal/wasm/abi/metrics_test.go` (~150-220 LoC; MetricType byte-pin per AMEND-B2)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE metrics placeholders; ~-10 LoC delta)
- Append: PROGRESS.md (Task 12 entry per D-P-PLAN-3)

This task lands `internal/wasm/dynamic_stats.go` (wraps per-plugin `*dynamic.Registry` constructed at config-load by `*compiledConfig`) + `internal/wasm/abi/metrics.go` (4-hostcall dispatch per §5.1 #31-34). proxy_define_metric dispatch + signed-i64 delta plumbing per AMEND-B2; 1024-entry cap; dynamic_stats_cap_exceeded counter + envoy_go.failures counter increment on cap-exceeded per §2.25. `proxy_increment_metric(metric_id, delta)` with SIGNED `int64` delta per AMEND-B2; `proxy_record_metric(metric_id, value)` with UNSIGNED `uint64` value per AMEND-B2.

**Precondition:** Task 11 complete (`internal/stats/dynamic.Registry` API materialized).
**Artifact:** `internal/wasm/dynamic_stats.go` + `abi/metrics.go` + their test files materialized; metrics placeholders deleted from stubs_25_2.go; MetricType byte-pin per AMEND-B2 + R-25.2-2 GREEN.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/... -run 'TestDynamicStats|TestMetrics'` passes (proxy_define_metric + Increment + Record + Get round-trip + signed-i64 delta extremes + 1024-entry cap-boundary + dynamic_stats_cap_exceeded counter + envoy_go.failures co-increment per §2.25 + MetricType byte-pin Counter=0/Gauge=1/Histogram=2 per AMEND-B2); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 12 per 25.2 SPEC §3.1 dynamic_stats.go + §5.1 #31-34 + AMEND-B2 + R-25.2-2 + R-25.2-7. NEW `internal/wasm/dynamic_stats.go` materializes the per-RootVM dynamic-stats wrapper: `(*RootVM).DefineMetric(metricType uint32, name string) (MetricID uint32, status WasmResult)` — delegates to `vm.dynStats.Register(dynamic.MetricType(metricType), name)`; on ErrCapExceeded → dynamic_stats_cap_exceeded counter increment + envoy_go.failures counter increment + return InternalFailure; else return MetricID + Ok. `(*RootVM).IncrementMetric(id uint32, delta int64) WasmResult` — delegates to `vm.dynStats.Increment(MetricID(id), delta)`. `(*RootVM).RecordMetric(id uint32, value uint64) WasmResult`. `(*RootVM).GetMetric(id uint32) (value uint64, status WasmResult)`. NEW `internal/wasm/abi/metrics.go` materializes the 4 host shims per §5.1 #31-34: `DefineMetricShim(ctx, vm, metricType, nameDataPtr, nameSize, retMetricIDPtr) WasmResult` (read name bytes + delegate to *RootVM.DefineMetric + write MetricID to retMetricIDPtr); `IncrementMetricShim(ctx, vm, metricID uint32, delta int64) WasmResult` (delegate); `RecordMetricShim(ctx, vm, metricID uint32, value uint64) WasmResult` (delegate); `GetMetricShim(ctx, vm, metricID uint32, retValuePtr) WasmResult` (delegate + write to retValuePtr). Tests at `dynamic_stats_test.go` materialize round-trip + 1024-entry cap-boundary + dynamic_stats_cap_exceeded counter assertion + envoy_go.failures co-increment verification + signed-i64 delta extremes (delta=-1; delta=int64::MIN); tests at `abi/metrics_test.go` materialize MetricType enum byte-pin per AMEND-B2 (assert MetricTypeCounter=0; MetricTypeGauge=1; MetricTypeHistogram=2) + ErrBadArgument enforcement on cross-type operations. Delete metrics placeholders from stubs_25_2.go.

- [ ] **Step 1: Write failing tests** at `internal/wasm/dynamic_stats_test.go` + `internal/wasm/abi/metrics_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/... -run 'TestDynamicStats|TestMetrics'
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement `internal/wasm/dynamic_stats.go`** per AMEND-B2 + R-25.2-7.

- [ ] **Step 3: Implement `internal/wasm/abi/metrics.go`** per §5.1 #31-34 + R-25.2-2.

- [ ] **Step 4: Delete metrics placeholders** from `internal/wasm/abi/stubs_25_2.go`.

- [ ] **Step 5: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/... -run 'TestDynamicStats|TestMetrics'
# Expected: PASS (round-trip + cap-boundary + counter assertions + MetricType byte-pin verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 6: Append PROGRESS.md Task 12 entry + commit**

```bash
git add internal/wasm/dynamic_stats.go internal/wasm/dynamic_stats_test.go internal/wasm/abi/metrics.go internal/wasm/abi/metrics_test.go internal/wasm/abi/stubs_25_2.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): dynamic_stats.go + abi/metrics.go — wraps per-plugin *dynamic.Registry per AMEND-B2 + R-25.2-2

Phase 25.2 Task 12 (Tier C NEW packages + lua MIGRATION + property roster).
NEW dynamic_stats.go wraps per-plugin *dynamic.Registry — *RootVM.Define
Metric / IncrementMetric / RecordMetric / GetMetric methods delegate.
ErrCapExceeded → dynamic_stats_cap_exceeded counter + envoy_go.failures
co-increment per §2.25 + InternalFailure return. NEW abi/metrics.go
materializes proxy_define_metric + proxy_increment_metric + proxy_record_
metric + proxy_get_metric dispatch per §5.1 #31-34. proxy_increment_metric
delta SIGNED int64 per AMEND-B2 (allows negative gauge deltas); proxy_record_
metric value UNSIGNED uint64 per AMEND-B2. MetricType enum byte-pin per
R-25.2-2: Counter=0, Gauge=1, Histogram=2. ErrBadArgument enforcement on
cross-type operations (Increment on Histogram; Record on Counter)."
```

---

## Task 13: NEW `internal/wasm/property.go` — full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4

**Files:**
- Create: `internal/wasm/property.go` (~450-650 LoC; full ~70-path property roster + NUL-delimited path parsing)
- Create: `internal/wasm/property_test.go` (~500-700 LoC; table-driven ~70 sub-paths + co-consumed primitive integration)
- Modify: `internal/wasm/abi/stubs_25_2.go` (DELETE property placeholder if any was added at Task 3; ~-5 LoC delta — proxy_get_property is at 25.1 surface already; Task 13 EXTENDS the existing 25.1 minimal property tree to the full ~70-path roster)
- Append: PROGRESS.md (Task 13 entry per D-P-PLAN-3)

This task lands the full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4. NUL-delimited path parsing per §11.4 + cpp-host `context.cc:1047-1058`. Per-root dispatch covering ~10 dispatched roots (request 16 sub-paths + response 6 + connection 12+id + source 2 + destination 2 + upstream 14 + xds 12 + metadata + filter_state + upstream_filter_state + wasm.<key> proxy) + 4 direct tokens (plugin_name + plugin_root_id + plugin_vm_id + connection_id). Co-consumed primitive mapping per AMEND-B4: stream-local accessors for request/response/source/destination/wasm-direct-tokens (no co-consumed primitive needed; the property resolver receives the per-stream filter callbacks at dispatch time); RE-CONSUMES ADR-0144 `DownstreamPrincipal()` for connection.{subject,uri_san,dns_san,sha256,tls_version}_* sub-paths (second co-consumer beyond phase-04); RE-CONSUMES ADR-0177 `internal/httpclient/` for upstream.{cluster fields via xds.cluster_*, address, port, local_address} + xds.cluster_name (third-or-later co-consumer); RE-CONSUMES ADR-0190 `internal/dynamicmetadata/` for metadata.* + xds.*_metadata branches (third-or-later co-consumer); CONSUMES NEW `internal/filterstate/` per ADR-0207 for filter_state.* + upstream_filter_state.* + wasm.<key> proxy branches (consumer #2). Absent-property returns `WasmResult::NotFound` (=1) byte-faithful to upstream `context.cc:1065/1072/1078/1083/1103/1106/1110`. **Note on architecture**: the `internal/wasm/property.go` resolver lives at the framework primitive layer (NOT the filter layer); it receives the necessary per-stream context (filter callbacks + per-stream filterstate bucket + per-stream dynamicmetadata bucket) via the ABICallbacks interface — Task 15 extends abi_callbacks.go with the per-callback GetProperty implementation that delegates to this framework-side resolver. The split is: framework resolver owns the path-parsing + per-root dispatch logic; consumer-side ABICallbacks owns the per-stream context plumbing.

**Precondition:** Task 9 complete (`*filterstate.Bucket` API materialized for filter_state.* + upstream_filter_state.* sub-paths) + parts of Task 7 (foreign-function dispatch for wasm.<key> property class if that branch needs foreign-function semantics — actually it doesn't; the wasm.<key> proxy class proxies to filter_state then upstream_filter_state per cpp-host `context.cc:987-1019`, NO foreign-function dispatch; the Task 7 dependency is removed).
**Artifact:** `internal/wasm/property.go` + `property_test.go` materialized; ~70 sub-paths table-driven GREEN; NUL-delimited path parsing GREEN; absent-property NotFound semantics GREEN; co-consumed primitive integration GREEN.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestProperty` passes (table-driven tests for ~70 sub-paths per AMEND-B4 + NUL-delimited path parsing edge cases + absent-property NotFound semantics + co-consumed primitive integration round-trip); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 13 per 25.2 SPEC §3.1 property.go + §11.4 + AMEND-B4 + R-25.2-4. NEW `internal/wasm/property.go` materializes: (a) `parsePathSegments(path []byte) []string` — splits on 0x00; empty segment → return nil (caller treats as NotFound); trailing NUL tolerated per spec README. (b) `PropertyResolver` interface — defines accessors needed by the per-root dispatch (e.g., `GetRequest() RequestAccessor`, `GetConnection() ConnectionAccessor` — these accessors are populated by the consumer-side ABICallbacks at Task 15). (c) `ResolveProperty(resolver PropertyResolver, path []byte) (value []byte, status WasmResult)` — entry point that parses path + dispatches to per-root resolvers per AMEND-B4 roster. (d) per-root dispatch functions (`resolveRequest(path []string, acc RequestAccessor) ([]byte, WasmResult)` per 16 sub-paths; similar for response/connection/source/destination/upstream/xds/metadata/filter_state/upstream_filter_state/wasm.<key>; 4 direct tokens). Each per-root resolver returns NotFound for unknown sub-paths byte-faithful to context.cc. (e) `serializeValue(v any) ([]byte, error)` — converts the typed value (string, []byte, int64, []HeaderPair, etc.) to wire bytes per spec README §Serialization. Tests at `property_test.go` materialize table-driven tests for ~70 sub-paths per AMEND-B4: each row is `{rootName string, subPath []byte, mockAccessor PropertyResolver, wantBytes []byte, wantStatus WasmResult}`. Cover request 16 + response 6 + connection 12 + source 2 + destination 2 + upstream 14 + xds 12 + metadata + filter_state + upstream_filter_state + wasm.<key> + 4 direct tokens. NUL-delimited path parsing tests: empty path → NotFound; trailing NUL tolerated; non-NUL separator → NotFound; double-NUL (empty segment) → NotFound. Co-consumed primitive integration round-trip: test PropertyResolver impl that wraps the 4 RE-USE primitives (ADR-0144 DownstreamPrincipal mock + ADR-0177 httpclient.Cluster mock + ADR-0190 dynamicmetadata.Bucket mock + ADR-0207 filterstate.Bucket from Task 9). Absent-property NotFound semantics for each root.

- [ ] **Step 1: Write failing tests** at `internal/wasm/property_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestProperty
# Expected: FAIL (PropertyResolver + ResolveProperty not yet implemented)
```

- [ ] **Step 2: Implement `internal/wasm/property.go`** per §11.4 + AMEND-B4 + R-25.2-4.

- [ ] **Step 3: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestProperty
# Expected: PASS (~70 sub-paths + NUL-delimited + absent-property + co-consumed primitive integration verified)
go vet ./internal/wasm/... && golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 13 entry + commit**

```bash
git add internal/wasm/property.go internal/wasm/property_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): property.go — full ~70-path proxy_get_property roster per AMEND-B4 + R-25.2-4

Phase 25.2 Task 13 (Tier C NEW packages + lua MIGRATION + property roster).
NEW property.go materializes full ~70-path proxy_get_property roster per
AMEND-B4 + R-25.2-4. NUL-delimited path parsing per §11.4 + cpp-host
context.cc:1047-1058. Per-root dispatch ~10 dispatched roots (request 16
sub-paths + response 6 + connection 12+id + source 2 + destination 2 +
upstream 14 + xds 12 + metadata + filter_state + upstream_filter_state +
wasm.<key> proxy) + 4 direct tokens (plugin_name + plugin_root_id +
plugin_vm_id + connection_id). Co-consumed primitive mapping per AMEND-B4:
stream-local accessors for request/response/source/destination/wasm-direct-
tokens; RE-CONSUMES ADR-0144 DownstreamPrincipal for connection.tls.*;
RE-CONSUMES ADR-0177 httpclient for upstream.* + xds.cluster_*; RE-CONSUMES
ADR-0190 dynamicmetadata for metadata.* + xds.*_metadata; CONSUMES NEW
internal/filterstate/ per ADR-0207 for filter_state.* + upstream_filter_state.*
+ wasm.<key> proxy (consumer #2). Absent-property NotFound byte-faithful.
~70 sub-paths table-driven + NUL-delimited edge cases + co-consumed primitive
integration verified."
```

---

## Tier D — internal/filter/http/wasm/ package extensions (Tasks 14-18; partly 3-way parallelizable)

## Task 14: `internal/filter/http/wasm/compiled_config.go` EXTEND — 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms per §6.2 + RootVM construction at New + D-25.2-P5 first-action

**Files:**
- Modify: `internal/filter/http/wasm/compiled_config.go` (~+200-300 LoC; 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms + RootVM construction)
- Modify: `internal/filter/http/wasm/compiled_config_test.go` (~+200-300 LoC; 6 NEW PARSE-REJECT arm coverage + envoy-go-strict-only config field validators)
- Modify: `internal/filter/http/wasm/doc.go` (~+30-50 LoC; append 25.2 cross-refs)
- Append: PROGRESS.md (Task 14 entry per D-P-PLAN-3 + **D-25.2-P5 partial closure** — byte-stable wording for 6 NEW arms anticipated per §6.2; final closure at Task 22 BEHAVIOR_CONTRACT.md bundle landing)

This task EXTENDS `internal/filter/http/wasm/compiled_config.go` per ADR-0208 + §4.2 + §6.2 + Qs 2/6/9. Adds 4 envoy-go-strict-only `PluginConfig` config fields (envoy_go_strict_body_buffer_cap_bytes default 16777216 = 16 MiB per Q2; envoy_go_strict_shared_data_value_cap_bytes default 1048576 = 1 MiB per Q6; envoy_go_strict_shared_data_max_entries default 1024 per Q6; envoy_go_strict_dynamic_stats_max_entries default 1024 per Q9). The envoy-go-strict-only fields live on the envoy-go-internal `*compiledConfig` after parse, populated via a custom envoy-go protobuf extension OR JSON sidecar — final mechanism settles at this Task 14 first-action; anticipated: envoy-go-strict-only fields parsed from a `PluginConfig.configuration` Any value whose top-level Struct includes an `envoy_go_strict_*` subobject (similar to phase-11 `envoy_go_strict_rate_limit_overrides` precedent). Adds 6 NEW PARSE-REJECT arms per §6.2: arm 19 envoy-go-strict-body-buffer-cap-bytes-zero; arm 20 envoy-go-strict-shared-data-value-cap-bytes-zero; arm 21 envoy-go-strict-shared-data-max-entries-zero; arm 22 envoy-go-strict-dynamic-stats-max-entries-zero; arm 23 envoy-go-strict-body-buffer-cap-bytes-overlarge (>1 GiB ceiling); arm 26 cross-pluginconfig-duplicate-pluginconfig-name. RootVM construction at New() via `wasm.NewRootVM(ctx, cfg.module, cfg.rootContextID, opts...)` — NOT per-stream `wasm.NewVM`; the per-plugin `*dynamic.Registry` construction via `dynamic.NewRegistry(pluginScope, cfg.dynStatsMaxEntries)`; per-plugin foreign-function registry view points at `wasm.DefaultForeignFunctionRegistry` by default. **D-25.2-P5 first-action** at this task: byte-stable wording for the 6 NEW arms pins via `TestParseRejectConstants_ByteStable` extension table — the anticipated wordings per §6.2 are provisional; the final wording lands at this Task 14 (the byte-stable wording test enforces commit-time). The 18-arm 25.1 PARSE-REJECT roster STAYS ACTIVE at 25.2 verbatim (per ADR-0080 byte-stable discipline + 25.1 D-P5 closure at 25.1 Task 9) — Task 14 EXTENDS the existing arm table.

**Precondition:** Tasks 1-13 complete (all `internal/wasm/` + `internal/filterstate/` + `internal/stats/dynamic/` surfaces materialized; compiledConfig consumes RootVM + Registry + foreign-function registry).
**Artifact:** `internal/filter/http/wasm/compiled_config.go` EXTENDED + `compiled_config_test.go` EXTENDED; 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms GREEN; RootVM construction at New() replaces per-stream wasm.NewVM; D-25.2-P5 wording for 6 NEW arms byte-stable.
**Acceptance:** `go test -count=1 -v ./internal/filter/http/wasm/ -run 'TestBuildCompiledConfig|TestParseRejectConstants_ByteStable'` passes (6 NEW PARSE-REJECT arm coverage + envoy-go-strict-only config field validators zero/overlarge + TestParseRejectConstants_ByteStable EXTENDED with 6 NEW byte-stable constants); `golangci-lint run ./internal/filter/http/wasm/...` clean; the whole-repo build is STILL broken at internal/filter/http/wasm/decode_headers.go references to wasm.NewVM — Task 18 closes the whole-repo build (per D-P-PLAN-6 + Task 1 documented expected breakage).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 14 per 25.2 SPEC §4.2 + §6.2 + Qs 2/6/9 + ADR-0208. EXTEND `internal/filter/http/wasm/compiled_config.go` with: (a) 4 envoy-go-strict-only fields on compiledConfig struct (bodyBufferCapBytes uint32 default 16777216; sharedDataValueCapBytes uint32 default 1048576; sharedDataMaxEntries uint32 default 1024; dynStatsMaxEntries uint32 default 1024); (b) rootVM *wasm.RootVM field (constructed at New() via wasm.NewRootVM); (c) dynStats *dynamic.Registry field (constructed via dynamic.NewRegistry); (d) foreignReg *wasm.ForeignFunctionRegistry field (points at wasm.DefaultForeignFunctionRegistry by default); (e) PARSE-REJECT parsing for the 4 envoy-go-strict-only fields with the 5 NEW arms (19, 20, 21, 22, 23 per §6.2) + the cross-PluginConfig duplicate name arm 26 (which requires a per-HCM lookup — settles at IMPL Task 14 first-action; anticipated: arm 26 fires when buildCompiledConfig is called twice for the same listener with the same PluginConfig.name). buildCompiledConfig extension: parse the configuration Any → typed struct OR JSON-decoded map → extract envoy_go_strict_* subobject → populate the 4 fields with defaults if absent; PARSE-REJECT arms 19-23 if any field is 0 OR > 1<<30 (arm 23). Construct rootVM via wasm.NewRootVM(...) at New() time (replacing the 25.1 per-stream wasm.NewVM pattern; wasm.NewVM no longer exists per Task 1 D-P-PLAN-6 — the decode_headers.go reference closes at Task 18). Tests at compiled_config_test.go: (a) 6 NEW PARSE-REJECT arm table-driven coverage; (b) envoy-go-strict-only config field validators (zero / overlarge / valid); (c) TestParseRejectConstants_ByteStable EXTENDED with parseRejectEnvoyGoStrictBodyBufferCapBytesZero + parseRejectEnvoyGoStrictSharedDataValueCapBytesZero + parseRejectEnvoyGoStrictSharedDataMaxEntriesZero + parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero + parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge + parseRejectCrossPluginConfigDuplicatePluginConfigName constants byte-exact per D-25.2-P5; (d) valid-config path constructs *RootVM + *dynamic.Registry + foreignReg correctly.

- [ ] **Step 1: D-25.2-P5 first-action** — settle byte-stable wording for the 6 NEW arms per §6.2 table; record empirical wording choices in scratch notes for PROGRESS.md entry.

- [ ] **Step 2: Write failing tests** at `internal/filter/http/wasm/compiled_config_test.go`

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run 'TestBuildCompiledConfig|TestParseRejectConstants_ByteStable'
# Expected: FAIL (6 NEW arms not yet implemented; envoy-go-strict-only fields not yet parsed)
```

- [ ] **Step 3: Implement `internal/filter/http/wasm/compiled_config.go` extensions** per §4.2 + §6.2 + Qs 2/6/9 + D-P5 byte-stable wording. Update `doc.go` with 25.2 cross-refs.

- [ ] **Step 4: Run tests + lint clean**

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run 'TestBuildCompiledConfig|TestParseRejectConstants_ByteStable'
# Expected: PASS (6 NEW PARSE-REJECT arms + envoy-go-strict-only fields verified)
go vet ./internal/filter/http/wasm/... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: each clean at package scope (whole-repo build still broken at decode_headers.go wasm.NewVM reference per Task 1 D-P-PLAN-6)
```

- [ ] **Step 5: Append PROGRESS.md Task 14 entry with D-25.2-P5 partial closure + commit**

```bash
git add internal/filter/http/wasm/compiled_config.go internal/filter/http/wasm/compiled_config_test.go internal/filter/http/wasm/doc.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): compiled_config EXTEND — 4 envoy-go-strict-only config fields + 6 NEW PARSE-REJECT arms per §6.2 + RootVM at New + D-25.2-P5 partial closure

Phase 25.2 Task 14 (Tier D internal/filter/http/wasm/ extensions). EXTEND
compiled_config.go per ADR-0208 + §4.2 + §6.2 + Qs 2/6/9. 4 envoy-go-strict-
only PluginConfig fields: envoy_go_strict_body_buffer_cap_bytes default 16
MiB (Q2); envoy_go_strict_shared_data_value_cap_bytes default 1 MiB (Q6);
envoy_go_strict_shared_data_max_entries default 1024 (Q6); envoy_go_strict_
dynamic_stats_max_entries default 1024 (Q9). 6 NEW PARSE-REJECT arms per
§6.2: arm 19 body-buffer-cap-bytes-zero + arm 20 shared-data-value-cap-bytes-
zero + arm 21 shared-data-max-entries-zero + arm 22 dynamic-stats-max-entries-
zero + arm 23 body-buffer-cap-bytes-overlarge (>1 GiB ceiling) + arm 26
cross-pluginconfig-duplicate-pluginconfig-name. Total 25.2 PARSE-REJECT roster
24 arms (18 inherited from 25.1 + 6 NEW). RootVM construction at New() via
wasm.NewRootVM (replacing 25.1 per-stream wasm.NewVM). per-plugin *dynamic.
Registry + foreignReg view constructed. D-25.2-P5 partial closure: 6 NEW arm
byte-stable wording pinned; TestParseRejectConstants_ByteStable EXTENDED.
Whole-repo build still broken at decode_headers.go wasm.NewVM reference per
Task 1 D-P-PLAN-6 — Task 18 closes whole-repo build."
```

---

## Task 15: `internal/filter/http/wasm/abi_callbacks.go` EXTEND — 7 NEW methods + 4 RE-USE primitive consumers per §5.3

**Files:**
- Modify: `internal/filter/http/wasm/abi_callbacks.go` (~+400-600 LoC; 7 NEW methods + 4 RE-USE primitive consumers)
- Modify: `internal/filter/http/wasm/abi_callbacks_test.go` (~+400-550 LoC; 7 NEW method coverage + 4 RE-USE primitive round-trip)
- Append: PROGRESS.md (Task 15 entry per D-P-PLAN-3)

This task EXTENDS `internal/filter/http/wasm/abi_callbacks.go` with 7 NEW methods per §5.3 + 4 RE-USE primitive consumer integration. The 7 NEW methods: `OnRequestBody(streamCtxID, bodySize uint32, endOfStream bool) ProxyAction`; `OnResponseBody(streamCtxID, bodySize uint32, endOfStream bool) ProxyAction`; `OnRequestTrailers(streamCtxID, numTrailers uint32) ProxyAction`; `OnResponseTrailers(streamCtxID, numTrailers uint32) ProxyAction`; `OnTick(rootCtxID)`; `OnHttpCallResponse(streamCtxID, callID, numHeaders, bodySize, numTrailers uint32)`; `OnForeignFunction(contextID, foreignFunctionID, dataSize uint32)`. The 4 RE-USE primitive consumer integration: (1) `GetProperty` for `connection.{subject,uri_san,dns_san,sha256,tls_version}_*` sub-paths → RE-CONSUMES ADR-0144 `DownstreamPrincipal()` via `decoderCb.StreamInfo().DownstreamPrincipal()` (second co-consumer beyond phase-04); (2) for `upstream.*` + `xds.cluster_*` sub-paths → RE-CONSUMES ADR-0177 `internal/httpclient/` via the per-RootVM `*httpclient.Client` from Task 8 + via `decoderCb.UpstreamHost()` for the originating-stream's upstream binding (third-or-later co-consumer); (3) for `metadata.*` + `xds.*_metadata` branches → RE-CONSUMES ADR-0190 `internal/dynamicmetadata/` via `decoderCb.GetDynamicMetadata(filterName)` (third-or-later co-consumer); (4) for `filter_state.*` + `upstream_filter_state.*` + `wasm.<key>` proxy branches → CONSUMES NEW `internal/filterstate/*Bucket` from Task 9 (consumer #2 — phase-22.2 lua is consumer #1 via Task 10 MIGRATION). The implementation pattern: each NEW method routes through the per-stream `*StreamContext` and delegates to the appropriate consumer-side filter callback or framework primitive; the GetProperty implementation constructs a PropertyResolver (per Task 13) wrapping the 4 RE-USE primitives + delegates to `wasm.ResolveProperty(resolver, path)`.

**Precondition:** Task 14 complete (compiledConfig with RootVM + dynamic.Registry + foreignReg constructed); Tasks 9-13 complete (4 RE-USE primitives + property resolver available).
**Artifact:** `abi_callbacks.go` EXTENDED with 7 NEW methods + 4 RE-USE primitive integration; `abi_callbacks_test.go` EXTENDED with NEW method coverage + RE-USE round-trip tests.
**Acceptance:** `go test -count=1 -race -v ./internal/filter/http/wasm/ -run TestAbiCallbacks` passes (7 NEW method round-trip + 4 RE-USE primitive integration: DownstreamPrincipal returns expected sub-symbols on TLS test connection; httpclient mock dispatches to test cluster; dynamicmetadata test injection round-trips through metadata.* property; filterstate Bucket round-trips through filter_state.* property); `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 15 per 25.2 SPEC §3.6 abi_callbacks.go + §5.3 + AMEND-B4 RE-USE mapping. EXTEND `abi_callbacks.go` with 7 NEW methods per §5.3 (each delegates through the per-stream context to the appropriate filter-side accessor). For the 4 RE-USE primitive integration: extend GetProperty implementation to construct a PropertyResolver impl (`type filterPropertyResolver struct { decoderCb DecoderFilterCallbacks; encoderCb EncoderFilterCallbacks; downstreamBucket *filterstate.Bucket; upstreamBucket *filterstate.Bucket; rootVM *wasm.RootVM }`) that delegates to: (a) `decoderCb.StreamInfo().DownstreamPrincipal()` for connection TLS sub-paths per ADR-0144; (b) `decoderCb.UpstreamHost()` or `rootVM.httpClient.GetCluster(...)` for upstream.* sub-paths per ADR-0177; (c) `decoderCb.GetDynamicMetadata(filterName)` for metadata.* sub-paths per ADR-0190; (d) `downstreamBucket` / `upstreamBucket` for filter_state.* / upstream_filter_state.* sub-paths per ADR-0207. The compiledConfig (Task 14) constructs 2 *filterstate.Bucket instances per stream (downstream + upstream) + passes them through to the abiCallbacks struct via the per-stream construction at Task 18 decode_headers.go. Tests at abi_callbacks_test.go: 7 NEW method round-trip (mock StreamContext + assert per-method dispatch) + 4 RE-USE primitive round-trip (mock DecoderFilterCallbacks with test DownstreamPrincipal / UpstreamHost / GetDynamicMetadata; mock *filterstate.Bucket; assert GetProperty paths return expected bytes for each RE-USE source).

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/abi_callbacks_test.go`

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run TestAbiCallbacks
# Expected: FAIL (7 NEW methods + RE-USE integration not yet implemented)
```

- [ ] **Step 2: Implement `internal/filter/http/wasm/abi_callbacks.go` extensions** per §3.6 + §5.3 + AMEND-B4 RE-USE mapping.

- [ ] **Step 3: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run TestAbiCallbacks
# Expected: PASS (7 NEW methods + 4 RE-USE primitive round-trip verified)
go vet ./internal/filter/http/wasm/... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: each clean at package scope
```

- [ ] **Step 4: Append PROGRESS.md Task 15 entry + commit**

```bash
git add internal/filter/http/wasm/abi_callbacks.go internal/filter/http/wasm/abi_callbacks_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): abi_callbacks EXTEND — 7 NEW methods + 4 RE-USE primitive consumers per §5.3 + AMEND-B4

Phase 25.2 Task 15 (Tier D internal/filter/http/wasm/ extensions). EXTEND
abi_callbacks.go with 7 NEW methods per §5.3 (OnRequestBody + OnResponseBody
+ OnRequestTrailers + OnResponseTrailers + OnTick + OnHttpCallResponse +
OnForeignFunction). 4 RE-USE primitive consumer integration for GetProperty
per AMEND-B4: ADR-0144 DownstreamPrincipal for connection.tls.*; ADR-0177
httpclient for upstream.* + xds.cluster_*; ADR-0190 dynamicmetadata for
metadata.* + xds.*_metadata; NEW ADR-0207 filterstate for filter_state.* +
upstream_filter_state.* + wasm.<key> proxy (consumer #2). PropertyResolver
impl wraps the 4 RE-USE primitives + delegates to wasm.ResolveProperty per
Task 13. ABICallbacks interface 13 → 20 methods. 4 RE-USE primitive round-
trip + 7 NEW method dispatch verified."
```

---

## Task 16: NEW `internal/filter/http/wasm/body.go` + `trailers.go` + `tick_clock.go` per §4.3 + Q1 + Q2 + Q5

**Files:**
- Create: `internal/filter/http/wasm/body.go` (~200-280 LoC; DecodeData + EncodeData glue; body-buffer accumulation + cap enforcement + 413-on-exceed)
- Create: `internal/filter/http/wasm/trailers.go` (~120-180 LoC; DecodeTrailers + EncodeTrailers glue)
- Create: `internal/filter/http/wasm/tick_clock.go` (~80-120 LoC; Clock seam injection plumbing)
- Create: `internal/filter/http/wasm/body_test.go` (~300-450 LoC; body-buffer accumulation + cap enforcement)
- Create: `internal/filter/http/wasm/trailers_test.go` (~200-300 LoC; trailer hostcall dispatch)
- Append: PROGRESS.md (Task 16 entry per D-P-PLAN-3)

This task lands the 3 NEW filter-package files per §4.3 + Q1 + Q2 + Q5. `body.go`: DecodeData + EncodeData glue per Q1 per-chunk-invoke + accumulating buffer pattern + Q2 cap enforcement. Per-stream body accumulation in `filter.decodeBody []byte` + `filter.encodeBody []byte` + grow on each OnDecodeBuffer/OnEncodeBuffer; cap enforcement via `cfg.bodyBufferCapBytes` — if accumulated > cap and not already cap-exceeded, set `filter.decodeBodyCapExceeded = true` (sticky) + bump `cfg.stats.bodyBufferCapExceeded` counter + `cfg.stats.envoyGoFailures` counter + `decoderCb.SendLocalReply(413, "Payload Too Large", ...)` + return `api.StopAllIteration`; else if `streamCtx.HasGlobalFunc("proxy_on_request_body")` → `streamCtx.CallProxyOnRequestBody(ctx, uint32(len(decodeBody)), endStream)` + ProxyAction handling; NO-op if proxy_on_request_body not exported (guest doesn't opt into body callbacks). Per Q1: body_size is accumulated total available (NOT just-new-chunk delta); per AMEND-B1 buffer-clamp applied at proxy_get_buffer_bytes shim (Task 4 — body.go doesn't need to apply the clamp here since the clamp is at the host shim level). `trailers.go`: DecodeTrailers + EncodeTrailers glue per §4.3. If `streamCtx.HasGlobalFunc("proxy_on_request_trailers")` → `streamCtx.CallProxyOnRequestTrailers(ctx, numTrailers)` + ProxyAction handling. Mirrors encode side. Reuses 25.1 pairs wire-format (HttpRequestTrailers value 1 + HttpResponseTrailers value 3 ACTIVATED). `tick_clock.go`: Clock seam injection plumbing per Q5 + R-25.2-9. The compiledConfig at New() time may receive an injectable `clock.Clock` via a test-only seam (production uses `clock.RealClock`); the fixture-0036 tick-fires-counter scenario uses `clock.FakeClock` to make tick fires deterministic. tick_clock.go provides the WithRootClock option pass-through from compiledConfig to wasm.NewRootVM.

**Precondition:** Tasks 14 + 15 complete (compiledConfig + abi_callbacks support body/trailer/tick callbacks).
**Artifact:** `body.go` + `trailers.go` + `tick_clock.go` + their test files materialized; body-buffer cap enforcement + 413-on-exceed + trailer dispatch + Clock seam plumbing GREEN.
**Acceptance:** `go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestBody|TestTrailers|TestTickClock'` passes (body-buffer accumulation + cap enforcement + 413-on-exceed dispatch + body_buffer_cap_exceeded counter + envoy_go.failures co-increment per §2.25 + sticky cap-exceeded flag + trailer dispatch + Clock seam injection); `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 16 per 25.2 SPEC §4.3 + Q1 + Q2 + Q5. NEW `body.go` materializes DecodeData + EncodeData per the §4.3 dispatch shape. The filter struct (defined at internal/filter/http/wasm/wasm.go per 25.1 + extended here) gains: decodeBody []byte; encodeBody []byte; decodeBodyCapExceeded bool; encodeBodyCapExceeded bool. DecodeData(buf api.BufferInstance, endStream bool) api.StatusType: append buf bytes to filter.decodeBody; if len(decodeBody) > cfg.bodyBufferCapBytes && !filter.decodeBodyCapExceeded → filter.decodeBodyCapExceeded = true + cfg.stats.bodyBufferCapExceeded.Inc() + cfg.stats.envoyGoFailures.Inc() + filter.decoderCb.SendLocalReply(413, "Payload Too Large", "", nil, 0) + return api.StopAllIteration; else if filter.streamCtx.HasGlobalFunc("proxy_on_request_body") → action, err := filter.streamCtx.CallProxyOnRequestBody(ctx, uint32(len(decodeBody)), endStream); on action == CONTINUE → return api.Continue; on action == PAUSE → return api.StopAndBuffer; on err → cfg.stats.envoyGoFailures.Inc() + log + return api.Continue. EncodeData mirrors. NEW `trailers.go` materializes DecodeTrailers + EncodeTrailers per §4.3. DecodeTrailers(trailers api.RequestTrailerMap) api.StatusType: if !filter.streamCtx.HasGlobalFunc("proxy_on_request_trailers") → return api.Continue; else action, err := filter.streamCtx.CallProxyOnRequestTrailers(ctx, uint32(trailers.Len())); same ProxyAction handling. NEW `tick_clock.go` materializes Clock seam injection plumbing per Q5: a small WithClock helper that production callers ignore (RealClock default) + fixture-0036 tick-fires-counter scenario uses to inject FakeClock. Tests at body_test.go: (a) body-buffer accumulation via DecodeData with multiple chunks → assert filter.decodeBody contains all chunks concatenated; (b) cap enforcement: configure bodyBufferCapBytes=1024 + DecodeData with 2KB → assert SendLocalReply(413) invoked + bodyBufferCapExceeded counter incremented + envoyGoFailures counter incremented + return StopAllIteration; (c) sticky cap-exceeded flag: after cap exceeded, subsequent DecodeData calls do NOT re-invoke SendLocalReply; (d) NO-op if proxy_on_request_body not exported (mock HasGlobalFunc returns false) → DecodeData returns Continue without invoking CallProxyOnRequestBody. Tests at trailers_test.go: trailer dispatch round-trip + WasmHeaderMapType values 1/3 activation + ProxyAction handling.

- [ ] **Step 1: Write failing tests** at `body_test.go` + `trailers_test.go`

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestBody|TestTrailers|TestTickClock'
# Expected: FAIL (body.go + trailers.go + tick_clock.go not yet exist)
```

- [ ] **Step 2: Implement `body.go` + `trailers.go` + `tick_clock.go`** per §4.3 + Q1 + Q2 + Q5.

- [ ] **Step 3: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestBody|TestTrailers|TestTickClock'
# Expected: PASS (body cap + 413-on-exceed + sticky flag + trailer dispatch + Clock seam verified)
go vet ./internal/filter/http/wasm/... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: each clean at package scope
```

- [ ] **Step 4: Append PROGRESS.md Task 16 entry + commit**

```bash
git add internal/filter/http/wasm/body.go internal/filter/http/wasm/trailers.go internal/filter/http/wasm/tick_clock.go internal/filter/http/wasm/body_test.go internal/filter/http/wasm/trailers_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): body.go + trailers.go + tick_clock.go per §4.3 + Q1 + Q2 + Q5

Phase 25.2 Task 16 (Tier D internal/filter/http/wasm/ extensions). NEW
body.go materializes DecodeData + EncodeData per Q1 per-chunk-invoke +
accumulating buffer + Q2 cap enforcement. Per-stream filter.decodeBody +
encodeBody accumulation; cap exceeded → SendLocalReply(413) + body_buffer_
cap_exceeded counter + envoy_go.failures co-increment per §2.25 + sticky
cap-exceeded flag + StopAllIteration. NO-op if proxy_on_request_body not
exported. NEW trailers.go materializes DecodeTrailers + EncodeTrailers per
§4.3. WasmHeaderMapType values 1/3 ACTIVATED. NEW tick_clock.go materializes
Clock seam injection plumbing per Q5 + R-25.2-9 — production uses RealClock
default; fixture-0036 tick-fires-counter scenario injects FakeClock. Body cap
+ 413-on-exceed + sticky flag + trailer dispatch + Clock seam verified."
```

---

## Task 17: NEW `internal/filter/http/wasm/property.go` + `internal/filter/http/wasm/stats.go` EXTEND — per-stream property resolver + 9 NEW envoy-go-strict counters per §7.1 + AMEND-B3

**Files:**
- Create: `internal/filter/http/wasm/property.go` (~250-400 LoC; per-stream property resolver)
- Create: `internal/filter/http/wasm/property_test.go` (~300-450 LoC; ~70 sub-paths coverage)
- Modify: `internal/filter/http/wasm/stats.go` (~+100-150 LoC; 9 NEW envoy-go-strict counters)
- Append: PROGRESS.md (Task 17 entry per D-P-PLAN-3)

This task lands the NEW `internal/filter/http/wasm/property.go` per-stream property resolver dispatch per AMEND-B4 + the EXTENDED `stats.go` with 9 NEW envoy-go-strict counters per §7.1 + AMEND-B3. The `property.go` per-stream resolver delegates to the `wasm.RootVM.property` tree (via Task 13's `ResolveProperty` + the PropertyResolver from Task 15's abi_callbacks integration) + the 4 RE-USE primitives. The `stats.go` extension adds 9 NEW envoy-go-strict counters to `filterStats`: tickInvocations + httpCallDispatched + httpCallResponse + foreignFunctionDenied + bodyBufferCapExceeded + httpCallDispatchUnknownCluster + sharedDataCapExceeded + dynamicStatsCapExceeded + httpCallResponseAfterClose. Project stat count 119 → 128. Plus the per-plugin `*dynamic.Registry` field added to filterStats (NOT a counter — the Registry instance itself; the namespace `wasmcustom.<custom_name>` populated lazily via proxy_define_metric calls).

**Precondition:** Tasks 13 (wasm.property.go + ResolveProperty) + 15 (abi_callbacks PropertyResolver) complete.
**Artifact:** `property.go` + `property_test.go` materialized; `stats.go` EXTENDED with 9 NEW counters; project stat count 119 → 128 verified byte-exact.
**Acceptance:** `go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestProperty|TestStats'` passes (per-stream property resolver dispatch coverage for ~70 sub-paths per AMEND-B4 + co-consumed primitive round-trips + absent-property NotFound + 9 NEW counter byte-exact stat-name verification + project stat count assertion 128); `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 17 per 25.2 SPEC §3.6 property.go + §7.1 + AMEND-B3 + AMEND-B4. NEW `property.go` materializes per-stream property resolver dispatch. Constructs the PropertyResolver impl (Task 15 abi_callbacks) wrapping the 4 RE-USE primitives + delegates to wasm.ResolveProperty (Task 13). EXTEND `stats.go` with 9 NEW envoy-go-strict counter fields on filterStats struct: tickInvocations + httpCallDispatched + httpCallResponse + foreignFunctionDenied + bodyBufferCapExceeded + httpCallDispatchUnknownCluster + sharedDataCapExceeded + dynamicStatsCapExceeded + httpCallResponseAfterClose (per §7.1 table). Each counter constructed via reg.NewCounter("wasm." + pluginName + "." + counterName) at newFilterStats. Update the package-level stat-name const declarations. Tests at property_test.go materialize per-stream property resolver round-trip + ~70 sub-paths via mock PropertyResolver + assertion against expected wire bytes; tests at stats_test.go (extension of existing 25.1 stats_test.go) materialize 9 NEW counter byte-exact stat-name verification (TestStatNames_Equal_Wasm_TickInvocations etc.) + project stat count assertion `len(allStatNames()) == 128` (was 119 at 25.1).

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/property_test.go` + extend `stats_test.go`

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestProperty|TestStats'
# Expected: FAIL (property.go not yet exists; stats_test.go counter assertions fail)
```

- [ ] **Step 2: Implement `internal/filter/http/wasm/property.go`** per §3.6 + AMEND-B4.

- [ ] **Step 3: Extend `internal/filter/http/wasm/stats.go`** with 9 NEW envoy-go-strict counters per §7.1 + AMEND-B3.

- [ ] **Step 4: Run tests + lint clean**

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run 'TestProperty|TestStats'
# Expected: PASS (property resolver + ~70 sub-paths + 9 NEW counters + 128 stat-count verified)
go vet ./internal/filter/http/wasm/... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: each clean at package scope
```

- [ ] **Step 5: Append PROGRESS.md Task 17 entry + commit**

```bash
git add internal/filter/http/wasm/property.go internal/filter/http/wasm/property_test.go internal/filter/http/wasm/stats.go internal/filter/http/wasm/stats_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): property.go + stats.go EXTEND — per-stream property resolver + 9 NEW envoy-go-strict counters per §7.1 + AMEND-B3

Phase 25.2 Task 17 (Tier D internal/filter/http/wasm/ extensions). NEW
property.go materializes per-stream property resolver dispatch per AMEND-B4.
Delegates to wasm.RootVM.property tree via Task 13 ResolveProperty + Task 15
abi_callbacks PropertyResolver wrapping 4 RE-USE primitives (DownstreamPrincipal
+ httpclient + dynamicmetadata + filterstate). EXTEND stats.go with 9 NEW
envoy-go-strict counters per §7.1 + AMEND-B3: tickInvocations + httpCall
Dispatched + httpCallResponse + foreignFunctionDenied + bodyBufferCap
Exceeded + httpCallDispatchUnknownCluster + sharedDataCapExceeded + dynamic
StatsCapExceeded + httpCallResponseAfterClose. Counter 14 http_call_response_
after_close per AMEND-B3 recommendation (defensive observability for stray
late responses). Project stat count 119 → 128. Per-plugin *dynamic.Registry
plumbed. ~70 sub-paths property resolver + 9 NEW counters + stat count 128
verified."
```

---

## Task 18: `internal/filter/http/wasm/decode_headers.go` + `encode_headers.go` EXTEND — per-stream construction via cfg.rootVM.NewStreamContext

**Files:**
- Modify: `internal/filter/http/wasm/decode_headers.go` (~+50-80 LoC; per-stream construction via RootVM.NewStreamContext + streamCtx field replaces vm field)
- Modify: `internal/filter/http/wasm/encode_headers.go` (~+30-50 LoC; per-stream context shared with decode side)
- Modify: `internal/filter/http/wasm/wasm.go` (~+10-20 LoC; filter struct: vm field RENAMES to streamCtx)
- Modify: `internal/filter/http/wasm/dispatch_test.go` (~+200-300 LoC; body/trailer/tick/httpCall integration + per-stream concurrency)
- Append: PROGRESS.md (Task 18 entry per D-P-PLAN-3)

This task EXTENDS `decode_headers.go` + `encode_headers.go` + the filter struct (at wasm.go) per §4.3 — per-stream construction goes through `cfg.rootVM.NewStreamContext(ctx)` (NOT per-stream `wasm.NewVM`). The filter struct's `vm` field RENAMES to `streamCtx` (`*wasm.StreamContext`). Same ProxyAction handling + captured-local-response handoff as 25.1; OnDestroy delegates to `streamCtx.Close(ctx)` (which fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels outstanding httpCalls per AMEND-B3). **This task CLOSES the whole-repo build that has been broken since Task 1 per D-P-PLAN-6** — after Task 18 lands, `go build ./...` whole-repo is clean again.

**Precondition:** Tasks 15 + 16 + 17 complete (abi_callbacks + body/trailers/tick_clock + property+stats all wired); Task 14's RootVM construction at New() is the upstream-side state that this Task consumes.
**Artifact:** `decode_headers.go` + `encode_headers.go` + `wasm.go` EXTENDED; `dispatch_test.go` EXTENDED; whole-repo build CLOSES.
**Acceptance:** `go test -count=1 -race -v ./internal/filter/http/wasm/...` passes (full per-stream VM lifecycle + body/trailer/tick/httpCall integration + per-stream concurrency under root-VM model); `go build ./...` whole-repo CLEAN (closing the D-P-PLAN-6 expected-breakage); `golangci-lint run ./...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 18 per 25.2 SPEC §4.3. EXTEND `wasm.go` filter struct: rename `vm *wasm.VM` field to `streamCtx *wasm.StreamContext`. EXTEND `decode_headers.go` per-stream construction: replace `vm, err := wasm.NewVM(ctx, wasm.WithSandboxConfig(cfg.sandbox), wasm.WithLogSink(filterLog))` + `vm.RegisterABICallbacks(...)` + `vm.Run(ctx, cfg.module, cfg.rootContextID)` + `vm.CallProxyOnContextCreate(...)` with `streamCtx, err := cfg.rootVM.NewStreamContext(ctx)` + `streamCtx.RegisterABICallbacks(&abiCallbacks{filter: f, cfg: cfg, decoderCb: f.dcb, encoderCb: nil})` + (NO Run call — RootVM already configured at New) + `cfg.stats.executions.Inc()`. Then `streamCtx.CallProxyOnRequestHeaders(ctx, uint32(len(headers)), endStream)` + ProxyAction handling (CONTINUE → Continue; PAUSE → log + Continue; captured local-response → SendLocalReply + StopIteration); on err → cfg.stats.envoyGoFailures.Inc() + log + Continue. EXTEND `encode_headers.go` mirror for `proxy_on_response_headers` — same streamCtx instance shared. EXTEND `OnDestroy` to call `streamCtx.Close(ctx)` (which fires proxy_on_done + proxy_on_log + proxy_on_delete + cancels outstanding httpCalls per AMEND-B3 + R-25.2-3). EXTEND `dispatch_test.go` with body/trailer/tick/httpCall integration round-trips + per-stream concurrency tests under root-VM model (N=100 stream contexts on one RootVM; verify shared-data writes from one stream visible to another via stat assertion; verify httpCall response from one stream NOT visible to another). Verify whole-repo build CLEAN.

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/dispatch_test.go` extensions

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/ -run TestDispatch
# Expected: FAIL (streamCtx not yet wired; body/trailer/tick/httpCall integration not yet implemented)
```

- [ ] **Step 2: Implement decode_headers.go + encode_headers.go + wasm.go extensions** per §4.3.

- [ ] **Step 3: Verify whole-repo build CLEAN**

```bash
go build ./...
# Expected: CLEAN (closes the D-P-PLAN-6 expected-breakage that has been outstanding since Task 1)
go vet ./...
golangci-lint run ./...
# Expected: each clean
```

- [ ] **Step 4: Run tests**

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/...
# Expected: PASS (full lifecycle + body/trailer/tick/httpCall + per-stream concurrency verified)
```

- [ ] **Step 5: Append PROGRESS.md Task 18 entry + commit**

```bash
git add internal/filter/http/wasm/decode_headers.go internal/filter/http/wasm/encode_headers.go internal/filter/http/wasm/wasm.go internal/filter/http/wasm/dispatch_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): decode/encode_headers EXTEND — per-stream construction via *RootVM.NewStreamContext; CLOSES whole-repo build per D-P-PLAN-6

Phase 25.2 Task 18 (Tier D internal/filter/http/wasm/ extensions). EXTEND
decode_headers.go + encode_headers.go + wasm.go per §4.3 — per-stream
construction goes through cfg.rootVM.NewStreamContext (NOT per-stream
wasm.NewVM which was DELETED at Task 1 per D-P-PLAN-6). Filter struct's
vm field RENAMES to streamCtx (*wasm.StreamContext). OnDestroy delegates
to streamCtx.Close which fires proxy_on_done + proxy_on_log + proxy_on_
delete + cancels outstanding httpCalls per AMEND-B3 + R-25.2-3. **CLOSES
whole-repo build** that has been broken since Task 1 per D-P-PLAN-6 documented
expected breakage — go build ./... clean again. dispatch_test.go EXTENDED
with body/trailer/tick/httpCall integration + per-stream concurrency under
root-VM model (N=100 stream contexts on one RootVM; shared-data cross-stream
visibility + httpCall response routing isolation verified)."
```

---

## Tier E — Fuzzer + differential fixtures (Tasks 19-21; 2-way parallelizable per D-P-PLAN-7)

## Task 19: 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` + ~35 corpus seeds per D-P-PLAN-10 + R-25.2-12

**Files:**
- Create: `internal/filter/http/wasm/fuzz_hostcall_test.go` (~150-220 LoC; FuzzWasmHostcallEnvelope per §8.4)
- Create: `internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/` (~35 corpus seeds per D-P-PLAN-10 across 10 dimensions)
- Append: PROGRESS.md (Task 19 entry per D-P-PLAN-3 + **D-25.2-P4 closure evidence** — 35-seed corpus enumerated per D-P-PLAN-10)

This task lands the 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + R-25.2-12 + ADR-0018 baseline. Must-never-panic across the 14 NEW hostcall envelope surfaces + foreign-function dispatch + dynamic-stats Register + shared-data CAS race + body-buffer cap boundary + property-path NUL-delimited adversarials. Corpus seeds at `testdata/fuzz/FuzzWasmHostcallEnvelope/` covering the 10 dimensions per D-P-PLAN-10: 35 total seeds. The fuzzer takes byte inputs that encode (a) hostcall ID + (b) wire-format args bytes; mutates these via `f.Add(...)` seed corpus + Go fuzz engine random walks. The fuzz body invokes the relevant abi/* host shim with the input bytes + asserts WasmResult return (must be Ok/NotFound/BadArgument/InternalFailure — never panic + never crash). Project-wide fuzzer count: **34 → 35 at 25.2 phase-done** (per §8.5 D-S2 closure at 25.2 SPEC commit; this Task LANDS the 35th).

**Precondition:** Tasks 1-18 complete (all 14 NEW hostcall surfaces materialized + abi_callbacks integration).
**Artifact:** `fuzz_hostcall_test.go` + 35 corpus seeds materialized; fuzzer 30s-clean per ADR-0018 baseline.
**Acceptance:** `go test -count=1 -fuzz=FuzzWasmHostcallEnvelope -fuzztime=30s ./internal/filter/http/wasm/` clean (no panics; must-never-panic invariant verified); project-wide fuzzer count = 35 verified via `grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*') | wc -l`; `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 19 per 25.2 SPEC §8.4 + R-25.2-12 + ADR-0018 + this PLAN's D-P-PLAN-10 35-seed corpus roster. NEW `internal/filter/http/wasm/fuzz_hostcall_test.go` materializes `FuzzWasmHostcallEnvelope(f *testing.F)`. The fuzz harness: (a) construct a mock *RootVM with permissive sandbox + mock httpclient + empty foreign registry + empty dynamic stats Registry; (b) for each fuzz input byte slice, dispatch to a randomly-selected abi/* host shim with the bytes interpreted as the hostcall's wire args; (c) assert no panic + WasmResult in {Ok, NotFound, BadArgument, InternalFailure, Unimplemented, CasMismatch}. f.Add() seed corpus per D-P-PLAN-10 35-seed roster — enumerate per the 10 dimensions: 5 seeds for hostcall envelope edges (proxy_get_buffer_bytes start/max combinations per AMEND-B1) + 4 for pairs serialization adversarial + 3 for foreign-fn name length boundary + 4 for dynamic-stats name validation + 3 for shared-data CAS race + 3 for body-buffer cap boundary + 4 for property-path NUL-delimited adversarial + 3 for tick period parsing (0/1ms/i64::MAX) + 4 for httpCall envelope + 2 for metric type out-of-range + signed-i64 delta extremes. Per-seed file format: encode the hostcall ID byte + the wire args bytes (the test harness decodes the ID + dispatches). Run with `go test -fuzz=FuzzWasmHostcallEnvelope -fuzztime=30s ./internal/filter/http/wasm/` to verify must-never-panic across all 35 seeds + random mutations. Document D-25.2-P4 closure evidence in PROGRESS.md Task 19 entry — 35-seed corpus roster reproduced verbatim from D-P-PLAN-10.

- [ ] **Step 1: Author seed corpus files** at `internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/` per D-P-PLAN-10 35-seed roster.

- [ ] **Step 2: Author `internal/filter/http/wasm/fuzz_hostcall_test.go`** per §8.4 + ADR-0018 baseline.

- [ ] **Step 3: Run fuzz smoke + project-wide count verification**

```bash
go test -count=1 -fuzz=FuzzWasmHostcallEnvelope -fuzztime=30s ./internal/filter/http/wasm/
# Expected: clean (no panics across 30s fuzzing across the 35 seeds + random mutations)
grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*') | wc -l
# Expected: 35 (was 34 at master tip; +1 for FuzzWasmHostcallEnvelope)
go vet ./internal/filter/http/wasm/... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: each clean
```

- [ ] **Step 4: Append PROGRESS.md Task 19 entry with D-25.2-P4 closure evidence + commit**

```bash
git add internal/filter/http/wasm/fuzz_hostcall_test.go internal/filter/http/wasm/testdata/fuzz/FuzzWasmHostcallEnvelope/ docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): 35th project-wide fuzzer FuzzWasmHostcallEnvelope per §8.4 + R-25.2-12 + D-25.2-P4 CLOSED per D-P-PLAN-10

Phase 25.2 Task 19 (Tier E fuzzer + differential fixtures). 35th project-
wide fuzzer FuzzWasmHostcallEnvelope per §8.4 + R-25.2-12 + ADR-0018
baseline. Must-never-panic across 14 NEW hostcall envelope surfaces +
foreign-function dispatch + dynamic-stats Register + shared-data CAS race
+ body-buffer cap boundary + property-path NUL-delimited adversarials.
35-seed corpus per D-P-PLAN-10 across 10 dimensions: 5 hostcall envelope
edges (AMEND-B1 clamp) + 4 pairs serialization + 3 foreign-fn name length
+ 4 dynamic-stats name validation + 3 shared-data CAS race + 3 body-buffer
cap boundary + 4 property-path NUL-delimited + 3 tick period parsing + 4
httpCall envelope + 2 metric type out-of-range + signed-i64 delta extremes.
D-25.2-P4 CLOSED per D-P-PLAN-10 — 35-seed corpus enumerated. Fuzzer
30s-clean. Project-wide fuzzer count: 34 → 35."
```

---

## Task 20: Differential fixture `0036-http-wasm-body-and-advanced` (single-listener mixed-mode per Q8 + ADR-0192 precedent + R-25.2-11; 14 scenarios) + NEW BackendKind=HTTPWasmAdvanced + deliberate-break liveness verification

**Files:**
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/README.md` (~200-300 LoC)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/envoy.yaml` (~200-300 LoC)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/envoy-go.yaml` (~200-300 LoC)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/expectations.yaml` (~120-200 LoC; human-readable)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/inputs/driver.go` (~800-1200 LoC)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/scripts/{a..n}_<name>/{Cargo.toml,src/lib.rs}` × 14 (~30 LoC each = ~420 LoC)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/scripts/README.md` (~80-120 LoC; reproduction discipline)
- Create: `test/fixtures/0036-http-wasm-body-and-advanced/bytecode/{a..n}_<name>.wasm` × 14 (vendored)
- Modify: `test/differential/fixture/fixture.go` (~+15 LoC; NEW BackendKind=HTTPWasmAdvanced enum value)
- Modify: `test/differential/runner_test.go` (~+15 LoC; blank import + switch-case for HTTPWasmAdvanced)
- Append: PROGRESS.md (Task 20 entry per D-P-PLAN-3 + **deliberate-break liveness verification evidence per `reference_differential_asserter_dispatch`**)

This task lands the mixed-mode differential fixture `0036-http-wasm-body-and-advanced` per Q8 + §8.1 + ADR-0192 precedent. Single-listener single-HCM hosting the wasm filter + router terminator + TWO upstream clusters (`cluster_a` primary + `cluster_b` httpCall target — both bound to the SAME differential echobackend at different cluster definitions per phase-22.2 REVIEW §7.4 `freeTCPPort` flake mitigation). 14 scenarios partitioned by assertion-class (10 deterministic cross-side via `CompareBytes` + 4 non-deterministic subject-only via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`). The 10 cross-side scenarios per §8.1.1: (a) body-read-only / (b) body-mutate-passthrough / (c) body-mutate-replace / (d) trailers-add / (e) trailers-read / (f) shared-data-read-after-write / (g) foreign-function-deny-default / (h) property-stream-info / (i) metric-define-only / (j) env-vars-rejected-passthrough. The 4 subject-only scenarios per §8.1.1: (k) tick-fires-counter / (l) httpCall-success / (m) httpCall-unknown-cluster / (n) body-cap-exceeded. NEW `BackendKind=HTTPWasmAdvanced` constant (OR REUSE existing `HTTPWasm` from 25.1 — settles at this Task 20 first-action; anticipated NEW per the per-fixture-dir-1-runner-branch discipline + assertion-class partitioning differences from 25.1 HTTPWasm). NEW `cluster_b` second-upstream-cluster definition. **Deliberate-break liveness verification MANDATORY per `reference_differential_asserter_dispatch`** at this Task 20 final-action: for each of the 4 subject-only StatsAsserter arms, deliberately break the stat assertion (e.g., change expected value from `>= 5` to `>= 99999`) → expect FAIL → restore + verify GREEN. This proves the assertion is LIVE (NOT dead-vacuous per the phase-23 fixture-0030 lesson + 25.1 Task 15+17 follow-up). 14 vendored `.wasm` blobs reproduced via the 25.1 fixture-0034 scripts/ pattern per D-P-PLAN-12.

**Precondition:** Tasks 1-18 complete (full filter wired); Task 19 fuzzer landed (independent — runs in parallel).
**Artifact:** `test/fixtures/0036-http-wasm-body-and-advanced/` directory with README + envoy.yaml + envoy-go.yaml + expectations.yaml + inputs/driver.go + scripts/ (14 Rust sources + reproduction README) + bytecode/ (14 vendored .wasm blobs); BackendKind=HTTPWasmAdvanced enum value + runner switch-case; 14 scenarios GREEN; deliberate-break liveness verification PASSED.
**Acceptance:** `go test -count=1 ./test/differential -run TestDifferential/0036` GREEN (10 cross-side scenarios pass via CompareBytes; 4 subject-only scenarios pass via StatsAsserter); deliberate-break liveness verification documented in PROGRESS.md Task 20 entry per `reference_differential_asserter_dispatch` (deliberately break each of the 4 subject-only StatsAsserter arm → expect FAIL → restore + verify GREEN); `ls -d test/fixtures/00*/ | wc -l` returns `38` (37 pre-existing + 1 NEW); `golangci-lint run ./test/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 20 per 25.2 SPEC §8.1 + Q8 + ADR-0192 precedent + D-P-PLAN-12 reproduction discipline. NEW fixture-0036 directory with: (a) envoy.yaml + envoy-go.yaml bootstraps for single-listener + 2 upstream clusters (cluster_a primary + cluster_b httpCall target); both YAMLs use {{.BackendPort}} + {{.UpstreamBPort}} template vars + the wasm filter consuming Wasm.config.vm_config.code via Filename arm pointing to bytecode/<scenario>.wasm; envoy-go.yaml additionally populates the 4 envoy-go-strict-only PluginConfig extension fields per Task 14 — body_buffer_cap_bytes=16777216 (matches default) + shared_data_value_cap_bytes=1048576 + shared_data_max_entries=1024 + dynamic_stats_max_entries=1024; (b) README.md scope + 14-scenario table per §8.1.1; (c) expectations.yaml human-readable; (d) inputs/driver.go (~800-1200 LoC) implementing the per-scenario probes for 10 cross-side scenarios (a)-(j) using existing CompareBytes runner + 4 subject-only scenarios (k)-(n) using StatsAsserter.AssertStats per `reference_differential_asserter_dispatch`. Scenarios (k)-(n) details per §8.1.1: (k) tick-fires-counter — wasm plugin sets 50ms tick period via proxy_set_tick_period_milliseconds at proxy_on_configure + proxy_on_tick increments wasmcustom.tick_count via proxy_define_metric+proxy_increment_metric; after 250ms probe wait subject's wasm.<plugin>.tick_invocations >= 5 + wasmcustom.tick_count value >= 5; (l) httpCall-success — wasm plugin invokes proxy_http_call("cluster_b", headers, nil, nil, 5000) at proxy_on_request_headers + proxy_on_http_call_response adds response header x-httpcall-status:<status>; assert subject's http_call_dispatched + http_call_response increment + x-httpcall-status:200 header; (m) httpCall-unknown-cluster — wasm plugin invokes proxy_http_call("nonexistent_cluster", ...) + response header x-httpcall-result:<wasmresult>; assert subject's http_call_dispatch_unknown_cluster increment + x-httpcall-result:2 (BadArgument); (n) body-cap-exceeded — wasm plugin returns PAUSE indefinitely in proxy_on_request_body + probe sends 32 MiB body; assert stream closes with HTTP 413 + body_buffer_cap_exceeded counter >= 1 + envoy_go.failures counter >= 1. 14 Rust source scripts/<name>/{Cargo.toml,src/lib.rs} per the 14 scenarios + 14 pre-built .wasm blobs vendored under bytecode/ per D-P-PLAN-12 (proxy-wasm-rust-sdk =0.2.4 + wasm32-wasip1 target per AMEND-A1). scripts/README.md reproduction discipline (rustup target add wasm32-wasip1 + cargo build --release --target wasm32-wasip1). NEW BackendKind=HTTPWasmAdvanced at fixture/fixture.go + blank import + switch-case at runner_test.go. **DELIBERATE-BREAK LIVENESS VERIFICATION MANDATORY** at final-action: for each of the 4 subject-only StatsAsserter arms, modify the expected value (e.g., from ">= 5" to ">= 99999") + run test → expect FAIL + record the FAIL output → restore expected value + run test → expect GREEN; document each deliberate-break/restore cycle in PROGRESS.md Task 20 entry per `reference_differential_asserter_dispatch` (NOT dead-vacuous per phase-23 fixture-0030 lesson + 25.1 Task 15+17 follow-up).

- [ ] **Step 1: Author 2 bootstrap YAMLs** (envoy.yaml + envoy-go.yaml) with single-listener + 2 upstream clusters + wasm filter wiring.

- [ ] **Step 2: Author 14 Rust source scenarios** at scripts/{a..n}_<name>/{Cargo.toml,src/lib.rs}; build via `cargo build --release --target wasm32-wasip1`; vendor compiled .wasm blobs to bytecode/.

- [ ] **Step 3: Author inputs/driver.go** with 14 scenario probes + StatsAsserter.AssertStats for 4 subject-only arms.

- [ ] **Step 4: Add NEW BackendKind=HTTPWasmAdvanced + runner switch-case** at fixture/fixture.go + runner_test.go.

- [ ] **Step 5: Run fixture-0036 differential test**

```bash
go test -count=1 -v ./test/differential -run TestDifferential/0036
# Expected: PASS (10 cross-side via CompareBytes + 4 subject-only via StatsAsserter)
```

- [ ] **Step 6: DELIBERATE-BREAK LIVENESS VERIFICATION (mandatory per `reference_differential_asserter_dispatch`)**

For each of the 4 subject-only StatsAsserter arms (k/l/m/n):
- Modify the expected stat value in driver.go (e.g., change `expectedTickInvocations := 5` to `expectedTickInvocations := 99999`)
- Run `go test -count=1 -v ./test/differential -run TestDifferential/0036/k` → expect FAIL (assertion now too strict; test catches that the live counter is < 99999)
- Restore the expected value (`expectedTickInvocations := 5`)
- Run `go test -count=1 -v ./test/differential -run TestDifferential/0036/k` → expect GREEN
- Record both outputs in PROGRESS.md (the FAIL output proves the assertion is LIVE; the GREEN restoration proves the test was correctly restored)
- Repeat for (l) httpCall-success + (m) httpCall-unknown-cluster + (n) body-cap-exceeded

- [ ] **Step 7: Verify fixture-dir count + lint clean**

```bash
ls -d test/fixtures/00*/ | wc -l
# Expected: 38 (37 pre-existing + 1 NEW)
go vet ./test/... && golangci-lint run ./test/...
# Expected: each clean
```

- [ ] **Step 8: Append PROGRESS.md Task 20 entry with deliberate-break liveness verification evidence + commit**

```bash
git add test/fixtures/0036-http-wasm-body-and-advanced/ test/differential/fixture/fixture.go test/differential/runner_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "test(differential): fixture-0036-http-wasm-body-and-advanced — mixed-mode 14 scenarios per Q8 + ADR-0192 + deliberate-break liveness verified per reference_differential_asserter_dispatch

Phase 25.2 Task 20 (Tier E fuzzer + differential fixtures). NEW differential
fixture 0036-http-wasm-body-and-advanced per Q8 + §8.1 + ADR-0192 precedent.
Single-listener single-HCM + 2 upstream clusters cluster_a + cluster_b
(both bound to same echobackend per phase-22.2 REVIEW §7.4 freeTCPPort
flake mitigation). 14 scenarios per §8.1.1: 10 deterministic cross-side
via CompareBytes (body-read-only / body-mutate-passthrough / body-mutate-
replace / trailers-add / trailers-read / shared-data-read-after-write /
foreign-function-deny-default / property-stream-info / metric-define-only
/ env-vars-rejected-passthrough) + 4 non-deterministic subject-only via
StatsAsserter.AssertStats per reference_differential_asserter_dispatch
(tick-fires-counter / httpCall-success / httpCall-unknown-cluster / body-
cap-exceeded). NEW BackendKind=HTTPWasmAdvanced enum value + runner switch-
case. 14 Rust source scenarios + 14 vendored .wasm blobs per D-P-PLAN-12
+ AMEND-A1 (proxy-wasm-rust-sdk =0.2.4 + wasm32-wasip1). DELIBERATE-BREAK
LIVENESS VERIFICATION GREEN per reference_differential_asserter_dispatch +
phase-23 fixture-0030 lesson + 25.1 Task 15+17 follow-up — each of 4
subject-only StatsAsserter arms broken + verified FAIL + restored + verified
GREEN. Fixture-dir count 37 → 38."
```

---

## Task 21: Differential fixture `0037-http-wasm-body-and-advanced-boot-reject` (subject-only per D-25.2-P1 + R-25.2-11) + D-25.2-P1 closure first-action (arm 19 anticipated)

**Files:**
- Create: `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/README.md` (~100-150 LoC)
- Create: `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy.yaml` (~100-150 LoC)
- Create: `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/envoy-go.yaml` (~100-150 LoC)
- Create: `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/inputs/driver.go` (~150-250 LoC)
- Modify: `test/differential/runner_test.go` (~+5 LoC; blank import for fixture-0037)
- Append: PROGRESS.md (Task 21 entry per D-P-PLAN-3 + **D-25.2-P1 closure evidence** — chosen arm + substring per IMPL-time empirical-scrape)

This task lands the subject-only boot-reject fixture `0037-http-wasm-body-and-advanced-boot-reject` per D-25.2-P1 + §8.2. Anticipated arm per D-25.2-P1: arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"`. Reference Envoy v1.37.2 accepts the unknown envoy-go-strict-only field (silent drop by upstream's protobuf parser); subject envoy-go boot-rejects on the chosen arm. Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch), the boot-reject subject-only branch is a NEW runner variant: anticipated runner-branch shape per D-25.2-P1 = extend `BootRejectFixture` with `subjectOnly: true` flag (recommended — minimal infrastructure delta); alternative = NEW `SubjectOnlyBootRejectFixture` runner branch (cleaner type-discriminated dispatch). **D-25.2-P1 closure at this task first-action**: empirical-scrape against the 6 candidate arms {19, 20, 21, 22, 23, 26} per §6.2 to pick the canonical arm + the substring; record the chosen arm + substring in PROGRESS.md Task 21 entry. Anticipated answer: arm 19 (most distinctive substring; deterministic config shape; upstream Envoy v1.37.2 has no such field).

**Precondition:** Task 20 complete (runner_test.go blank-import conflict + the existing fixture-0036 runner branch is in place).
**Artifact:** `test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/` directory materialized; subject-only BootRejectFixture variant; D-25.2-P1 closure evidence recorded (chosen arm + substring + runner-branch shape decision).
**Acceptance:** `go test -count=1 ./test/differential -run TestDifferential/0037` GREEN (subject-only boot-reject: subject envoy-go boot-rejects on the chosen arm + reference Envoy accepts); `ls -d test/fixtures/00*/ | wc -l` returns `39` (38 post-Task-20 + 1 NEW); D-25.2-P1 closure recorded in PROGRESS.md.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-25.2-P1 closure): empirical-scrape against the 6 candidate arms {19, 20, 21, 22, 23, 26} per §6.2 — for each candidate arm, construct a minimal envoy-go-strict config that triggers the arm + run envoy-go boot to confirm the boot-reject substring + record. Anticipated answer per §8.2: arm 19 `envoy-go-strict-body-buffer-cap-bytes-zero` with substring `"envoy_go_strict_body_buffer_cap_bytes"` — distinctive substring + deterministic config shape + upstream Envoy v1.37.2 has no such field (the unknown extension field is silently dropped by upstream's protobuf parser). Record the chosen arm + substring + the runner-branch shape decision (recommended: extend BootRejectFixture with subjectOnly: true flag — minimal infrastructure delta) in PROGRESS.md Task 21 entry. Then author Task 21 per 25.2 SPEC §8.2 + D-25.2-P1: NEW fixture-0037 directory with envoy.yaml (reference Envoy bootstrap; deliberately-malformed config triggering the chosen arm; reference side accepts unknown extension field silent-drop) + envoy-go.yaml (subject bootstrap symmetric; envoy-go boot-rejects on the chosen arm) + README.md scope + chosen-arm disposition + reference-accepts-unknown-field rationale + inputs/driver.go implementing subject-only BootRejectFixture variant per the chosen runner-branch shape. EXTEND `test/differential/runner_test.go` with blank import + switch-case if needed for the new runner-branch shape; if extending BootRejectFixture with subjectOnly: true flag, also EXTEND BootRejectFixture interface at the test infra (the existing BootRejectFixture from phase-22.1 Task 13).

- [ ] **Step 1: D-25.2-P1 first-action** — empirical-scrape against 6 candidate arms; record chosen arm + substring + runner-branch shape in scratch notes for PROGRESS.md.

- [ ] **Step 2: Author bootstrap YAMLs + README + driver.go** for fixture-0037.

- [ ] **Step 3: Extend BootRejectFixture (or runner switch-case)** per chosen runner-branch shape.

- [ ] **Step 4: Run fixture-0037 differential test**

```bash
go test -count=1 -v ./test/differential -run TestDifferential/0037
# Expected: PASS (subject-only boot-reject: subject envoy-go boot-rejects on chosen arm with chosen substring; reference Envoy accepts)
ls -d test/fixtures/00*/ | wc -l
# Expected: 39 (38 post-Task-20 + 1 NEW)
go vet ./test/... && golangci-lint run ./test/...
# Expected: each clean
```

- [ ] **Step 5: Append PROGRESS.md Task 21 entry with D-25.2-P1 closure evidence + commit**

```bash
git add test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/ test/differential/runner_test.go docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md
git commit -m "test(differential): fixture-0037-http-wasm-body-and-advanced-boot-reject — subject-only per D-25.2-P1 CLOSED at IMPL Task 21 first-action

Phase 25.2 Task 21 (Tier E fuzzer + differential fixtures). NEW subject-
only boot-reject fixture 0037-http-wasm-body-and-advanced-boot-reject per
D-25.2-P1 + §8.2. D-25.2-P1 CLOSED at Task 21 first-action via empirical-
scrape: chosen arm <19 | other> 'envoy-go-strict-body-buffer-cap-bytes-
zero' with substring '<envoy_go_strict_body_buffer_cap_bytes | other>'.
Reference Envoy v1.37.2 accepts unknown envoy-go-strict-only field (silent
drop by upstream's protobuf parser); subject envoy-go boot-rejects.
Runner-branch shape decision per reference_differential_fixture_dispatch_
constraint: <extend BootRejectFixture with subjectOnly: true flag |
introduce SubjectOnlyBootRejectFixture>. Fixture-dir count 38 → 39."
```

---

## Tier F — Atomic landing (Task 22)

## Task 22: Benchmark + BEHAVIOR_CONTRACT.md ~7-edit bundle + ADR-0205+0206+0207+0208 §Decision+§Consequences body landing + ADR-0202 one-line in-place AMEND + STATE.md re-advance + ROADMAP row 25.2 IMPL-done + REVIEW.md + CONDITIONAL ADR-0209 if R8 fires per D-P-PLAN-11 + D-25.2-P5 closure

**Files:**
- Create: `internal/filter/http/wasm/wasm_bench_test.go` (extension; ~+50-80 LoC; BenchmarkPerStreamModule_Instantiation per R8 gate + D-25.2-P2)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (~+350-500 LoC; ~7-edit bundle per §13.4 + ADR-0052 atomic landing)
- Modify: `docs/envoy-go/DECISIONS.md` (~+600-900 LoC; ADR-0205+0206+0207+0208 §Decision+§Consequences bodies + ADR-0202 §Consequences one-line AMEND acknowledgment + CONDITIONAL ADR-0209 if R8 fires)
- Modify: `docs/envoy-go/ROADMAP.md` (~+1 net; row 25.2 flips `in-progress → done`)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place; lifecycle-state advances to `phase 25.2 IMPL done; awaiting 25.3 BRAINSTORM (or 25.3 SPEC if BRAINSTORM-skip)`)
- Create: `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/REVIEW.md` (~350-450 LoC; reviewer artifact per `superpowers:requesting-code-review`)
- Append: PROGRESS.md (final Task 22 entry per D-P-PLAN-3 + **D-25.2-P2 closure at benchmark gate** + **D-25.2-P5 closure at BEHAVIOR_CONTRACT.md bundle landing** + R8 disposition)

This task is the atomic-landing meta-task per ADR-0052 atomic landing discipline + 25.2 SPEC §15 final-task discipline. Includes: (a) `BenchmarkPerStreamModule_Instantiation` per R8 gate + D-25.2-P2 + D-P-PLAN-11 — measures per-stream Module instantiation cost under the new root-VM model (constructs N=b.N fresh stream contexts on a shared `*RootVM` + `*Module`; reports `ns/op`). Threshold gate: if `ns/op > 1_000_000` (1ms), ADR-0209 escape-valve FIRES (§Context + §Decision + §Consequences body all land at the same Task 22 commit per ADR-0044 anchoring "pooled vs shared-Module-with-mutex-serialization" decision); if `ns/op <= 1_000_000`, WEAK-default fresh-per-stream STANDS; ADR-0209 STAYS UNCONSUMED. (b) BEHAVIOR_CONTRACT.md ~7-edit bundle per §13.4: edit #1 extend `### envoy.filters.http.wasm` subsection + body/buffer/trailers/timer/metrics/shared-data/httpCall/foreign-function/property bridge details + AMEND-B1..B5 cross-refs ~150-250 lines; edit #2 stat-table 119 → 128 extension + per-plugin Registry scope discipline note ~25-40 lines; edit #3 envoy-go-strict departure record #1 9-counter consolidated bundle per AMEND-B3 ~20-30 lines; edit #4 envoy-go-strict departure record #2 body-buffer cap discipline per Q2 ~15-25 lines; edit #5 envoy-go-strict departure record #3 shared-data cap + tick period 10ms floor consolidated per Q5+Q6 ~25-40 lines; edit #6 envoy-go-strict departure record #4 foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace clarification per AMEND-A9+Q9+AMEND-B2 ~30-45 lines; edit #7 EXTEND/RENAME `### Phase 25.1 forward-pointer notes` → `### Phase 25.2 forward-pointer notes` subsection ~50-80 lines. **D-25.2-P5 closure** at this bundle landing — final wording + line counts pinned. (c) ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Decision + §Consequences bodies land per ADR-0044 (the §Context drafts already at the 25.2 SPEC commit `f0eae39`). (d) ADR-0202 §Consequences gains one-line in-place AMEND acknowledgment paragraph per §10.2 (no new ADR number consumed). (e) CONDITIONAL ADR-0209 §Context + §Decision + §Consequences body if R8 escape-valve fires per (a). (f) STATE.md rewrite-in-place — lifecycle-state advances. (g) ROADMAP row 25.2 flips `in-progress → done` per ADR-0106; parent row 25 STAYS in-progress; sub-row 25.3 UNCHANGED planned. (h) REVIEW.md reviewer artifact per `superpowers:requesting-code-review` covering 6 phase-done gates + SPEC §15 46-item acceptance checklist closure + D-P-PLAN-1..D-P-PLAN-12 disposition record + D-25.2-P1..D-25.2-P5 closure evidence + R8 disposition.

**Precondition:** Tasks 1-21 complete; whole-repo build clean post-Task-18; all 21 production tasks GREEN.
**Artifact:** All listed files materialized/modified; 6 phase-done gates GREEN; ADR landings complete; STATE + ROADMAP advanced; REVIEW.md authored.
**Acceptance:** 6 phase-done gates ALL GREEN per 25.2 SPEC §14.10: Gate A (build) `go build ./...` clean; Gate B (vet+lint) `go vet ./...` + `golangci-lint run` clean no new suppressions; Gate C (race) `go test -count=1 -race ./...` clean; Gate D (differential) `go test -count=1 ./test/differential/...` 39/39 fixtures GREEN (37 pre-existing + 0036 + 0037); Gate E (fuzz) `FuzzWasmHostcallEnvelope` clean at 30s/seed + 35 project-wide fuzzers; Gate F (h2spec) `make test-h2spec` 53/53 PASS at ADR-0051 v1.32.4 pin. SPEC §15 46-item acceptance checklist ALL GREEN. ADR-0205+0206+0207+0208 §Decision+§Consequences bodies present in DECISIONS.md (`grep -nE '^## ADR-0205:|^## ADR-0206:|^## ADR-0207:|^## ADR-0208:' docs/envoy-go/DECISIONS.md` returns 4 matches; each has §Decision + §Consequences sub-headings); ADR-0202 §Consequences contains the one-line AMEND acknowledgment paragraph; CONDITIONAL ADR-0209 only if R8 fired. STATE.md `lifecycle-state` field shows `phase 25.2 IMPL done; awaiting 25.3 BRAINSTORM (or 25.3 SPEC if BRAINSTORM-skip)`; ROADMAP row 25.2 shows `done`; parent row 25 STAYS `in-progress`. REVIEW.md present (~350-450 LoC).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose` with explicit reference to 25.2 SPEC §15 46-item acceptance checklist + §13.4 ~7-edit bundle anatomy + §10 ADR anchor map):

> Author Task 22 per 25.2 SPEC §15 + §13.4 + §10 + ADR-0052 atomic landing discipline. The IMPL session should consult: (a) 25.2 SPEC §15 46-item acceptance checklist for the criteria; (b) §13.4 for the ~7-edit BEHAVIOR_CONTRACT.md bundle anatomy; (c) §10 for the ADR anchor map (ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Decision+§Consequences sketches + ADR-0202 one-line AMEND); (d) §9 for the 10 high-level semantic changes; (e) §7 for the 9 NEW envoy-go-strict counter rationales + the 4 envoy-go-strict-only config field rationales. The Task 22 workflow: Step 1 add benchmark + run + record output; Step 2 run all 6 phase-done gates A-F + record outputs verbatim; Step 3 vet/lint clean; Step 4 race clean; Step 5 differential 39/39 GREEN; Step 6 fuzz 35-fuzzer count clean at 30s; Step 7 h2spec 53/53; Step 8 author BEHAVIOR_CONTRACT.md ~7-edit bundle per §13.4 (edits 1-7); Step 9 author ADR-0205+0206+0207+0208 §Decision+§Consequences bodies in DECISIONS.md per §10 anchor map; Step 10 author ADR-0202 §Consequences one-line AMEND acknowledgment per §10.2; Step 11 IF R8 escape-valve fires per D-P-PLAN-11 (BenchmarkPerStreamModule_Instantiation > 1ms) author ADR-0209 §Context+§Decision+§Consequences body per ADR-0044 anchoring pooled-vs-shared-Module-with-mutex-serialization decision; Step 12 update STATE.md (active-phase: `25 (in-progress) — 25.2 IMPL done at 2026-MM-DD; awaiting 25.3 BRAINSTORM` parent stays in-progress; lifecycle-state: `phase 25.2 IMPL done; awaiting 25.3 BRAINSTORM (or 25.3 SPEC if BRAINSTORM-skip)`; next-skill: `superpowers:brainstorming` scoped to 25.3 BRAINSTORM OR `superpowers:writing-plans` scoped to 25.3 SPEC if BRAINSTORM-skip per parent-BRAINSTORM-settled-enough pattern; last-commit: `<TBD-25.2-IMPL-SQUASH>` placeholder; last-updated: today; next-free ADR: `ADR-0209` UNCHANGED if R8 not fired OR `ADR-0210` if R8 fired; verbose summary 22 tasks landed + 4 NEW ADR bodies + ADR-0202 one-line AMEND + CONDITIONAL ADR-0209 if R8 + 35th fuzzer FuzzWasmHostcallEnvelope clean + 39/39 differential fixture directories green + all 6 phase-done gates green + SPEC §15 46-item acceptance ALL GREEN + 119 → 128 stat count + 20 HTTP filters wired UNCHANGED + 34 → 35 fuzzer count + ADR tail advance to ADR-0208 (or ADR-0209 if R8) + D-25.2-P1..D-25.2-P5 closure evidence recorded + D-P-PLAN-1..D-P-PLAN-12 disposition record); Step 13 update ROADMAP.md row 25.2 — status flips in-progress → done; per-cell IMPL-done annotation per ADR-0106 documenting 22-task IMPL landing + 6-gate outputs + the SECOND occurrence of EXTRACT-NOW-on-second-consumer (after phase-22.1+22.2's internal/lua/+internal/dynamicmetadata/) + the NEW internal/filterstate/ framework primitive milestone + the NEW internal/stats/dynamic/ infrastructure subpackage milestone + the SPEC §15 46-item acceptance + the D-25.2-P1..P5 disposition record; parent row 25 STAYS in-progress; sub-row 25.3 UNCHANGED planned; Step 14 author REVIEW.md per `superpowers:requesting-code-review` (~350-450 LoC) covering: 6-gate outputs verbatim; BenchmarkPerStreamModule_Instantiation ns/op + R8 disposition; SPEC §15 46-item checklist verification with cite-to-PROGRESS-entry per item; D-P-PLAN-1..D-P-PLAN-12 decision-disposition record (which decisions HELD, which were AMENDED at IMPL); D-25.2-P1 + D-25.2-P3 + D-25.2-P4 + D-25.2-P5 closure evidence from Tasks 21/7/19/22; D-25.2-P2 + R8 disposition (STANDS WEAK-default fresh-per-stream OR ADR-0209 escape-valve FIRED); next-phase handoff state (25.3 BRAINSTORM scope hand-off — per-route 5th-canonical REUSE-by-absence per AMEND-A3 + multi-plugin VM-sharing + VmConfig.environment_variables activation + failure_policy = FAIL_RELOAD + fail_open deprecated + conformance harness seed at 62.5% per AMEND-A8); Step 15 verify nothing left uncommitted; Step 16 commit (Task 22 final IMPL-worktree commit).

- [ ] **Step 1: Add `BenchmarkPerStreamModule_Instantiation` benchmark + run** at `internal/filter/http/wasm/wasm_bench_test.go`

```bash
go test -bench=BenchmarkPerStreamModule_Instantiation -benchmem -count=1 -run='^$' ./internal/filter/http/wasm/
# Expected: ns/op output; record verbatim in PROGRESS.md Task 22 entry + R8 disposition per D-P-PLAN-11
```

- [ ] **Step 2: Gate A — build** — `go build ./...` clean. Capture output verbatim.

- [ ] **Step 3: Gate B — vet + lint** — `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture output verbatim.

- [ ] **Step 4: Gate C — race** — `go test -race -count=1 ./...` clean; zero data-race violations including the per-RootVM tick goroutine + per-stream context concurrent dispatch + shared-data CAS contention + httpCall response routing concurrency + concurrent foreign-function dispatch + concurrent dynamic-stats Register cap-boundary. Capture output verbatim.

- [ ] **Step 5: Gate D — differential + cross-package regression matrix per D-P-PLAN-8** — `go test -count=1 ./test/differential/...` clean (39/39 fixture directories GREEN: pre-existing 0000-0035 baseline + new 0036 + 0037). Capture output verbatim. Verify count: `ls -d test/fixtures/00*/ | wc -l` returns `39`.

- [ ] **Step 6: Gate E — fuzz** — `go test -fuzz=FuzzWasmHostcallEnvelope -fuzztime=30s ./internal/filter/http/wasm/` clean (no panics). 34 pre-existing fuzzers re-run clean at 30s per seed via per-package iteration. Capture output verbatim. Verify project-wide fuzzer count = 35 via `grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*') | wc -l`.

- [ ] **Step 7: Gate F — h2spec** — `make test-h2spec` 53/53 PASS at ADR-0051 v1.32.4 pin. Capture output verbatim.

- [ ] **Step 8: Author BEHAVIOR_CONTRACT.md ~7-edit bundle** per §13.4 — edits 1-7 land atomically in this commit. **D-25.2-P5 closure** at this bundle landing — record final wording + line counts in PROGRESS.md.

- [ ] **Step 9: Author ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 §Decision + §Consequences bodies in DECISIONS.md** per §10.1 anchor map + the §Decision body sketches from 25.2 SPEC §3.1 (ADR-0205 root VM lifecycle) + §3.1 + §5.1 + §5.3 + AMEND-B1 + AMEND-B2 + AMEND-B5 + AMEND-A9 (ADR-0206 25.2 ABI extensions) + §3.2 + AMEND-B4 (ADR-0207 internal/filterstate/) + §3.3 + §3.6 + §4 + §5 + §6 + §7 + §8 + §9 + §13.4 (ADR-0208 internal/filter/http/wasm/ extensions).

- [ ] **Step 10: Author ADR-0202 §Consequences one-line in-place AMEND acknowledgment paragraph** per §10.2 + the provisional wording. Update ADR-0202 `**Status:**` line with `; AMENDED 2026-MM-DD per phase-25.2 one-line acknowledgment in §Consequences`.

- [ ] **Step 11: IF D-P-PLAN-11 R8 escape-valve fires** (per Step 1 benchmark > 1ms threshold): author ADR-0209 §Context + §Decision + §Consequences body per ADR-0044 anchoring "pooled-Module vs shared-Module-with-mutex-serialization" decision based on empirical evidence. Otherwise skip (ADR-0209 STAYS UNCONSUMED per anticipated D-P-PLAN-11 outcome).

- [ ] **Step 12: Update STATE.md** to post-phase-25.2-IMPL state per BOOTSTRAP §4.1 invariant 1.

- [ ] **Step 13: Update ROADMAP.md row 25.2** — status flips `in-progress → done`; per-cell IMPL-done annotation per ADR-0106; parent row 25 STAYS `in-progress`; sub-row 25.3 UNCHANGED `planned`.

- [ ] **Step 14: Author REVIEW.md** per `superpowers:requesting-code-review` (~350-450 LoC) covering all 6 gate outputs + SPEC §15 46-item checklist verification + D-P-PLAN-1..D-P-PLAN-12 disposition + D-25.2-P1..P5 closures + R8 disposition + next-phase handoff.

- [ ] **Step 15: Append final PROGRESS.md Task 22 entry** with all 6 gate outputs verbatim + SPEC §15 46-item closure checklist + D-decision disposition status + R8 benchmark gate disposition.

- [ ] **Step 16: Verify nothing left uncommitted**

```bash
git status --porcelain
# Expect: empty
```

- [ ] **Step 17: Commit (Task 22 final IMPL-worktree commit)**

```bash
git add internal/filter/http/wasm/wasm_bench_test.go \
        docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/STATE.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/PROGRESS.md \
        docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/REVIEW.md
git commit -m "phase 25.2 Task 22: atomic landing + 6-gate phase-done verification + R8 benchmark gate + ADR-0205+0206+0207+0208 bodies + ADR-0202 one-line AMEND

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(39/39 fixture directories incl. new 0036 + 0037 — 10 deterministic cross-
side via CompareBytes + 4 non-deterministic subject-only via StatsAsserter
on 0036; subject-only single-arm boot-reject parity at D-25.2-P1 chosen arm
on 0037 with subjectOnly: true BootRejectFixture variant) / E fuzz (35
fuzzers clean; 35th FuzzWasmHostcallEnvelope per §8.4 + R-25.2-12) / F
h2spec 53/53 PASS.

25.2 SPEC §15 46-item acceptance checklist all GREEN. D-25.2-P1 closure at
Task 21 first-action (boot-reject arm <19 | other> chosen with common
substring '<envoy_go_strict_body_buffer_cap_bytes | other>'). D-25.2-P2 +
R8 gate: <STANDS WEAK-default fresh-per-stream Module instantiation |
ADR-0209 escape-valve FIRED with §Context+§Decision+§Consequences body
landed at this commit>; BenchmarkPerStreamModule_Instantiation reported
<ns/op> per-stream (anticipated WELL UNDER 1ms per D-P-PLAN-11 + 25.2 SPEC
§2.16 — root-VM model retires per-stream Runtime construction). D-25.2-P3
CLOSED at PLAN session per D-P-PLAN-9 (foreign-function dispatch concurrency
model: mutex-per-RootVM RATIFIED; concurrent N=100 dispatch + no cross-stream
leak verified at Task 7). D-25.2-P4 CLOSED at PLAN session per D-P-PLAN-10
(FuzzWasmHostcallEnvelope 35-seed corpus enumerated; fuzzer 30s-clean at
Task 19). D-25.2-P5 CLOSED at this Task 22 BEHAVIOR_CONTRACT.md bundle
landing (final wording + line counts pinned).

BEHAVIOR_CONTRACT.md ~7-edit bundle landed atomically per ADR-0052 + §13.4.
ADR-0205 §Decision + §Consequences body anchored (Root VM lifecycle
evolution per Q3 — ONE long-lived *RootVM per *compiledConfig upstream-byte-
faithful per cpp-host Wasm/Plugin model + per-stream contexts as CHILDREN
sharing wazero Runtime+Module + tick goroutine + 10ms envoy-go-strict floor
+ Clock seam FIRST co-consumer beyond phase-21 RATIFIES ADR-0186 + shared-
data + httpCall + foreign-function-registry state at root + per-stream
Module instantiation pattern <fresh-per-stream STANDS WEAK-default | ADR-
0209 escape-valve FIRED>; 25.1 per-stream *VM RETIRED per D-P-PLAN-6).
ADR-0206 §Decision + §Consequences body anchored (25.2 ABI extensions —
14 NEW env-namespace hostcalls + 7 NEW guest-export callbacks + 21 NEW
capability keys with gate-at-registerCallback per AMEND-B5 + buffer-clamp
wire-contract per AMEND-B1 + metric signedness per AMEND-B2 + foreign-
function dispatch concurrency mutex-per-RootVM per D-P-PLAN-9 + full ~70-
path proxy_get_property roster per AMEND-B4 + EMPTY default foreign-function
registry per AMEND-A9 + foreign_function_denied counter on NotFound).
ADR-0207 §Decision + §Consequences body anchored (NEW internal/filterstate/
framework primitive at second-consumer scope per Q7 — Bucket + FilterStateObject
+ StateType discriminator + sync semantics; consumer #1 = phase-22.2 lua
MIGRATES non-breaking at Task 10; consumer #2 = phase-25.2 wasm filter_state.*
+ upstream_filter_state.* per AMEND-B4 — upstream_filter_state DISTINCT root
co-equal; EXPLICIT API-REVISION ALLOWANCE for consumer #3+). ADR-0208
§Decision + §Consequences body anchored (NEW internal/filter/http/wasm/
25.2 package extensions — 9 envoy-go-strict counters per §7.1 + AMEND-B3
incl. http_call_response_after_close + 4 envoy-go-strict-only config fields
per Qs 2/6/9 + dynamic-stats namespace wasmcustom.<custom_name> per AMEND-
B2 via NEW internal/stats/dynamic/ infrastructure with per-plugin Registry
SCOPE discipline + mixed-mode fixture-0036 + subject-only boot-reject
fixture-0037 + ~7-edit BEHAVIOR_CONTRACT.md bundle + 35th fuzzer + envoy-
go-strict departure record bundle 6 records consolidated).
ADR-0202 §Consequences body gains one-line in-place AMEND acknowledgment
paragraph per §10.2 — 25.2 consumer-#1-internal-scope API evolution absorbed
under NEW ADRs per phase-22.2 Q10 strict-scope precedent; ADR-0202 EXPLICIT
API-REVISION ALLOWANCE for consumer #2 STAYS scoped to consumer #2. No new
ADR number consumed.
<+ CONDITIONAL ADR-0209 if R8 fired: per-stream Module instantiation pattern
— pooled-Module vs shared-Module-with-mutex-serialization decision based on
empirical evidence>.

EIGHTEENTH and FINAL §9 family-row second-of-3 sub-phase landed (full
advanced-bridge surface delta — body + buffer + trailers + timer + metrics
+ shared-data + httpCall + foreign-function + full property all ACTIVATED).
Parent row 25 STAYS in-progress until 25.3 phase-done; sub-row 25.3
UNCHANGED planned. SECOND occurrence of EXTRACT-NOW-on-second-consumer
after phase-22.1+22.2's internal/lua/+internal/dynamicmetadata/ — anchors
NEW internal/filterstate/ framework primitive for the broader §9 filter-
state family (rbac filter-state read; ext_authz inject; ext_proc pass-
through; new filter families) per ADR-0207 EXPLICIT API-REVISION ALLOWANCE.
NEW internal/stats/dynamic/ infrastructure subpackage per ADR-0208 + AMEND-
B2 (per-plugin Registry SCOPE; wasmcustom.<custom_name> namespace). Phase-
22.2 internal/filter/http/lua/filterstate.go MIGRATES non-breaking per ADR-
0207 §3.4 MIGRATES (Lua surface UNCHANGED). 20 HTTP filters wired UNCHANGED
post-25.2. Stat surface 119 → 128 names (+9 envoy-go-strict counters per
§7.1 + AMEND-B3); plus open-ended wasmcustom.<custom_name> dynamic-stats
family NOT counted in static total. Project fuzzer count 34 → 35. Project
differential fixture-dir count 37 → 39 (+2: 0036 mixed-mode + 0037 subject-
only boot-reject).

Six envoy-go-strict departures documented per §13.4 edits 3-6 + §9: (1)
9-counter consolidated bundle per AMEND-B3 (RAISES BRAINSTORM Q9 8 → 9);
(2) body-buffer cap discipline per Q2 (16 MiB default; 413-on-exceed); (3)
shared-data cap discipline per Q6 (1 MiB value + 1024 entries); (4) tick
period 10ms floor per Q5; (5) foreign-function 0-vs-10 default registry per
AMEND-A9; (6) dynamic-stats cap + namespace clarification per Q9 + AMEND-
B2. Cumulative envoy-go-strict departure record count post-25.2: ~27 (21
inherited from 25.1 + 6 NEW at 25.2 consolidated bundle). ROADMAP row 25.2
flipped in-progress → done; parent row 25 STAYS in-progress until 25.3
phase-done. STATE.md re-advanced to post-25.2-IMPL state. REVIEW.md
authored per superpowers:requesting-code-review."
```

---

## Phase-done squash-merge + push to origin

After Task 22 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl
# Resolve commit message — body must include the 22-task summary + the 4-NEW-ADR-bodies + ADR-0202-one-line-AMEND + CONDITIONAL ADR-0209 + the closes-row-25.2 + EIGHTEENTH §9-row-second-of-3 + the parent-row-25-STAYS-in-progress note + the SPEC §15 46-item acceptance ALL GREEN note + the 6-gate outputs summary.
git commit -m "$(cat <<'EOF'
Squash merge phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl

Closes ROADMAP row 25.2 (in-progress → done) — EIGHTEENTH §9 family-row
second-of-3 sub-phase (advanced-bridge surface delta; parent row 25 STAYS
in-progress until 25.3 phase-done; sub-row 25.3 UNCHANGED planned per ADR-
0106 sub-row rollup discipline + phase-18.1/18.2 + phase-19.1/19.2 + phase-
22.1/22.2/22.3 + phase-24.1/24.2 + phase-25.1 precedent).

22 tasks landed. 4 NEW ADR §Decision+§Consequences bodies anchored (ADR-
0205 root VM lifecycle evolution — ONE long-lived *RootVM per *compiledConfig
upstream-byte-faithful; per-stream contexts as CHILDREN sharing wazero
Runtime+Module; tick goroutine + 10ms envoy-go-strict floor + Clock seam
FIRST co-consumer beyond phase-21 RATIFIES ADR-0186; shared-data + httpCall
+ foreign-function-registry state at root; per-stream Module instantiation
pattern <fresh STANDS WEAK-default | pooled per CONDITIONAL ADR-0209 if R8
fired>; ADR-0206 25.2 ABI extensions — 14 NEW env-namespace hostcalls + 7
NEW guest-export callbacks + 21 NEW capability keys gate-at-registerCallback
per AMEND-B5 + buffer-clamp per AMEND-B1 + metric signedness per AMEND-B2
+ foreign-function dispatch concurrency mutex-per-RootVM per D-P-PLAN-9 +
full ~70-path proxy_get_property roster per AMEND-B4 + EMPTY default
foreign-function registry per AMEND-A9; ADR-0207 NEW internal/filterstate/
framework primitive at second-consumer scope — Bucket + FilterStateObject
+ StateType; consumer #1 = phase-22.2 lua MIGRATES non-breaking; consumer
#2 = wasm filter_state.* + upstream_filter_state.* per AMEND-B4 distinct
root; EXPLICIT API-REVISION ALLOWANCE for consumer #3+; ADR-0208 NEW
internal/filter/http/wasm/ 25.2 package extensions — 9 envoy-go-strict
counters per §7.1 + AMEND-B3 incl. http_call_response_after_close + 4
envoy-go-strict-only config fields + dynamic-stats namespace wasmcustom.<
custom_name> per AMEND-B2 via NEW internal/stats/dynamic/ infrastructure
+ per-plugin Registry SCOPE discipline + mixed-mode fixture-0036 + subject-
only boot-reject fixture-0037 + ~7-edit BEHAVIOR_CONTRACT.md bundle + 35th
fuzzer + 6-record envoy-go-strict departure bundle). ADR-0202 §Consequences
gains one-line in-place AMEND acknowledgment paragraph per §10.2 (no new
ADR number consumed; phase-22.2 Q10 strict-scope precedent).
<+ CONDITIONAL ADR-0209 if D-P-PLAN-11 R8 fired: per-stream Module
instantiation pattern — pooled-Module vs shared-Module-with-mutex-serialization
based on empirical evidence>.

35th fuzzer FuzzWasmHostcallEnvelope clean at 30s per ADR-0018 baseline.
39/39 differential fixture directories GREEN (0000-0037; 14 mixed-mode
scenarios on fixture-0036 — 10 deterministic cross-side via CompareBytes
+ 4 non-deterministic subject-only via StatsAsserter with deliberate-break
liveness verified per reference_differential_asserter_dispatch + phase-23
fixture-0030 lesson + 25.1 Task 15+17 follow-up; subject-only single-arm
boot-reject parity at D-25.2-P1 chosen arm on fixture-0037 via subjectOnly:
true BootRejectFixture variant per reference_differential_fixture_dispatch_
constraint). All 6 phase-done gates GREEN. 25.2 SPEC §15 46-item acceptance
checklist all GREEN.

EIGHTEENTH §9 family-row second-of-3 sub-phase landed (full advanced-bridge
surface delta — body + buffer + trailers + timer + metrics + shared-data +
httpCall + foreign-function + full ~70-path property all ACTIVATED). SECOND
occurrence of EXTRACT-NOW-on-second-consumer after phase-22.1+22.2's
internal/lua/+internal/dynamicmetadata/ — anchors NEW internal/filterstate/
framework primitive for the broader §9 filter-state family (rbac filter-
state read; ext_authz inject; ext_proc pass-through; new filter families)
per ADR-0207 EXPLICIT API-REVISION ALLOWANCE. NEW internal/stats/dynamic/
infrastructure subpackage per ADR-0208 + AMEND-B2 (per-plugin Registry
SCOPE; wasmcustom.<custom_name> namespace). Phase-22.2 internal/filter/
http/lua/filterstate.go MIGRATES non-breaking per ADR-0207 §3.4 MIGRATES
(Lua surface UNCHANGED — Lua filterstate tests pass UNCHANGED). 20 HTTP
filters wired UNCHANGED post-25.2 (boot-registration UNCHANGED per §3.7).
Stat surface 119 → 128 names (+9 envoy-go-strict counters per §7.1 + AMEND-
B3); plus open-ended wasmcustom.<custom_name> dynamic-stats family NOT
counted in static total per AMEND-B2 per-plugin Registry SCOPE. Project
fuzzer count 34 → 35. Project differential fixture-dir count 37 → 39 (+2:
0036 mixed-mode + 0037 subject-only boot-reject).

Six envoy-go-strict departures documented per §13.4 edits 3-6 + §9 (consolidated
where related): (1) 9-counter consolidated bundle per AMEND-B3 (RAISES
BRAINSTORM Q9 8 → 9); (2) body-buffer cap discipline per Q2 (16 MiB default;
413-on-exceed via SendLocalReply); (3) shared-data cap discipline per Q6
(1 MiB value + 1024 entries); (4) tick period 10ms floor per Q5; (5)
foreign-function 0-vs-10 default registry per AMEND-A9; (6) dynamic-stats
cap + namespace clarification per Q9 + AMEND-B2 (wasmcustom.<custom_name>
namespace ONLY without plugin prefix; per-plugin isolation via per-plugin
Registry SCOPE). Cumulative envoy-go-strict departure record count post-
25.2: ~27 (21 inherited from 25.1 + 6 NEW at 25.2 consolidated bundle).

D-25.2-P1 CLOSED at Task 21 first-action (fixture-0037 single-arm finalization
+ runner-branch shape). D-25.2-P2 + parent §13-R8 CLOSED at Task 22 benchmark
gate (per-stream Module instantiation: <STANDS WEAK-default | ADR-0209
escape-valve FIRED>). D-25.2-P3 CLOSED at PLAN session per D-P-PLAN-9
(foreign-function dispatch concurrency mutex-per-RootVM RATIFIED). D-25.2-
P4 CLOSED at PLAN session per D-P-PLAN-10 (FuzzWasmHostcallEnvelope 35-
seed corpus enumerated). D-25.2-P5 CLOSED at Task 22 BEHAVIOR_CONTRACT.md
bundle landing (final wording + line counts pinned). §13-R6 (ADR-0177
internal/httpclient/ co-consumer at 25.2 proxy_http_call) CLOSED at Task 8
(third-or-later co-consumer; RATIFIES phase-20 framework-primitive
extraction). §13-R8 (wazero per-stream Module instantiation benchmark)
CLOSED at Task 22.

ADR-0125 STAYS at 10 canonicals per AMEND-A3 (NO §(xvi) amendment at 25.3
IMPL — REUSE-by-absence is DEFINITIVE). DECISIONS.md tail at ADR-0208 (or
ADR-0209 if R8 fired). Next-free ADR-0209 (or ADR-0210 if R8 fired) —
STRENGTHENED-WEAK-HOLD-with-1-slot-buffer STANDS per 25.2 SPEC §1.2.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..25.1 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 22):
# Edit docs/envoy-go/STATE.md replacing "<TBD-25.2-IMPL-SQUASH>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 25.2 IMPL follow-up: STATE.md SHA-fill (TBD-25.2-IMPL-SQUASH → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-25.2-http-filter-wasm-body-and-advanced-bridge-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember

- Exact file paths always.
- Complete code shapes are in the 25.2 SPEC §3.1 + §3.2 + §3.3 + §3.5 + §3.6 + §4 + §5 + §6 + §11 references — the PLAN points to SPEC §3 + §5 + §6 + §11 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor); the per-Task File-structure table rows + per-Task Step bodies above describe the IMPL surface in implementer-actionable detail.
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 1 root_vm or Task 8 http_call cancel-at-destruction or Task 16 body cap-enforcement paths), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit per D-P-PLAN-4), `@superpowers:requesting-code-review` (Task 22 REVIEW.md), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 22 + per-Task PROGRESS.md entry quoted command outputs per D-P-PLAN-3).
- DRY, YAGNI, TDD, frequent commits.






