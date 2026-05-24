# Phase 25.1 — HTTP filter `envoy.filters.http.wasm` (filter scaffold + `internal/wasm/` framework primitive + headers bridge + default-deny capability sandbox) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the foundational third of `envoy.filters.http.wasm` (the EIGHTEENTH and FINAL §9 production HTTP filter) — VM + headers-bridge mode per parent BRAINSTORM Q6 + parent SPEC §3.1 surface-mapping — by shipping the NEW `internal/wasm/` framework primitive (8 production + 8 test Go files in package root + `abi/` subdirectory with 1 production + 1 test file: `doc.go` + `vm.go` + `compile.go` + `sandbox.go` + `registration.go` + `bytecode_util.go` + `pairs.go` + `wasi.go` + `abi/types.go` + per-file `*_test.go` companions; wazero v1.10.1 per-stream VM lifecycle + per-module `*Module` compile cache + default-deny `SandboxConfig` per AMEND-A5 + ABI-registration interface (`ABICallbacks`) + the in-house proxy-wasm v0.2.1 host ABI implementation for the headers-bridge hostcall subset + the WASI 8-stub custom implementation per R4 + the byte-faithful pairs wire format per R3 + the byte-faithful ABI-version detection per AMEND-A6 + `WithPanicHandler` + `WithLogSink` VMOptions + panic-wrapper; ADR-0202 §Decision + §Consequences body) + the NEW `internal/filter/http/wasm/` package (8 production + 5 test Go files: `doc.go` + `wasm.go` + `compiled_config.go` + `datasource.go` + `abi_callbacks.go` + `decode_headers.go` + `encode_headers.go` + `stats.go` + 5 test files; 4-arm `AsyncDataSource.Local` resolution `Filename`/`InlineBytes`/`InlineString`/`EnvironmentVariable` + 18-arm PARSE-REJECT roster per parent §6.2 + 24-hostcall ABICallbacks implementation [16 `proxy_*` env-namespace per §5.1 + 8 `wasi_snapshot_preview1.*` custom-shim per §5.2] + 13-callback guest-export surface [5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks per §5.3] + 5-counter stat surface per AMEND-A2 `wasm.wazero.{created,active}` Group-B upstream-parity + `wasm.<plugin_name>.{executions, hostcall_denied, envoy_go.failures}` envoy-go-strict-extensions tri-group prefix structure with HCM-stats_prefix DROPPED + per-stream wazero `*Runtime` construction per parent §4.2 WEAK-default; ADR-0203 §Decision + §Consequences body) + the default-deny capability sandbox materialized at `internal/wasm/sandbox.go` (~80-key capability roster; `WasmResult::InternalFailure`=10 denial semantic + integration error log + `wasm.<plugin_name>.hostcall_denied` counter; `SanitizationConfig` accept-empty discipline per AMEND-A1 §11.4; envoy-go-strict DEPARTURE from upstream's bare-empty-map-allow-all posture per AMEND-A5; ADR-0204 §Decision + §Consequences body) + the boot-registration insertion at `cmd/envoy-go/main.go` alphabetical after `router` per ADR-0100 §2.2 (20 HTTP filters wired post-25.1; was 19) + the 34th project-wide fuzzer `FuzzWasmConfigParse` (count CONFIRMED at 25.1 SPEC §11.1 D-S1 closure — 33 unique pre-25.1; `FuzzWasmConfigParse` is the 34th) + the NEW `BackendKind=HTTPWasm` constant at `test/differential/runner_test.go` per parent §8.1.3 + the differential fixture `0034-http-wasm-headers-bridge` (7 cross-side scenarios full byte-exact via existing `CompareBytes` per parent §8.1 + §4.5 D6 guardrails + vendored Rust-sourced `.wasm` bytecode under `bytecode/` + `scripts/` reproduction-source subdirectory per Q9 + AMEND-A1 `proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target + scenario (e) `StatsAsserter.AssertStats` subject-side stat assertion discipline per `reference_differential_asserter_dispatch`) + the differential fixture `0035-http-wasm-boot-reject` (single-arm boot-reject parity at anticipated arm 5 `vm-config-code-required` per D-P6 + per `reference_differential_fixture_dispatch_constraint` one-fixture-dir-equals-one-runner-branch discipline) — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on the 7 wire-interactive fixture-0034 scenarios + boot-reject substring parity on fixture-0035, modulo the 4-5 envoy-go-strict documented divergence-windows (default-deny capability sandbox per AMEND-A5; ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6; `AsyncDataSource.Remote` PARSE-REJECT per parent §2.1; runtime-name discriminator PARSE-REJECT per parent §2.3; 3 envoy-go-strict counters per AMEND-A2). **Sub-phase landing (`25.1` ROADMAP row) per parent SPEC §3.1 + BRAINSTORM Q1 3-way PRE-SPLIT discipline** — the 25.1 PLAN closes ROADMAP row `25.1` only at phase-done; parent row `25` STAYS `in-progress` until 25.3 IMPL phase-done (sub-row rollup discipline per ADR-0106 + phase-18.1/18.2 + phase-19.1/19.2 + phase-22.1/22.2/22.3 + phase-24.1/24.2 precedent). 25.2 (full Envoy↔Wasm advanced bridge delta — body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full stream-info) + 25.3 (per-route 5th-canonical wholesale-override REUSE-by-absence per AMEND-A3 + multi-plugin VM-sharing + conformance harness seed at 62.5% pass-threshold per AMEND-A8) are OUT OF SCOPE for 25.1.

**Architecture:** The 25.1 IMPL adds TWO new packages (`internal/wasm/` 8-production-file + 1-subdirectory-1-file framework primitive + `internal/filter/http/wasm/` 8-production-file consumer package) + extends 3 existing files (`cmd/envoy-go/main.go` +1 register + +1 import per ADR-0100 §2.2 alphabetical after `router`; `test/differential/fixture/fixture.go` +1 `BackendKind` enum value + dispatcher metadata; `test/differential/runner_test.go` +blank-imports + `BackendKind=HTTPWasm` switch-case) + adds 2 new fixture directories (`test/fixtures/0034-http-wasm-headers-bridge/` with `README.md` + `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `inputs/driver.go` + `scripts/` subdirectory with 7 per-scenario Rust sources + `bytecode/` subdirectory with 7 vendored `.wasm` blobs per Q9 + AMEND-A1; `test/fixtures/0035-http-wasm-boot-reject/` with `README.md` + `envoy.yaml` + `envoy-go.yaml` + `inputs/driver.go` implementing the boot-reject `BootRejectFixture` interface). The NEW `internal/wasm/` framework primitive follows the phase-22.1 `internal/lua/` EXTRACT-NOW-at-first-consumer precedent (SECOND occurrence after phase-22.1 `internal/lua/`) but with a larger surface — 8 production files in package root + 1 `abi/` subdirectory file vs phase-22.1's 4 root files — because the proxy-wasm v0.2.1 host ABI surface (47 hostcalls + the in-house implementations) is substantially larger than gopher-lua's bridge surface. The 25.1 SPEC §3.1 refines parent SPEC §4.1 sketch into production signatures (function-option `VMOption` pattern instead of sealed interface; `Run(module, rootContextID)` for root-context lifecycle + per-callback methods `CallProxyOnContextCreate`/`CallProxyOnRequestHeaders`/`CallProxyOnResponseHeaders`/`CallProxyOnDone`/`CallProxyOnLog`/`CallProxyOnDelete`; `State() wazero.Runtime` escape-hatch for filter consumers to register additional host modules or query module exports beyond the 25.1 surface; `WithPanicHandler` naming clarification — Go cannot RECOVER from a wazero-side trap mid-Call so the handler is only for genuine Go panics from hostcall Go callbacks; `WithLogSink(w io.Writer)` redirects `proxy_log` output for the lifetime of the VM; `CompileCache` nil-tolerant per ADR-0085; `ABICallbacks` interface at `registration.go` — consumer registers per-context callbacks via `vm.RegisterABICallbacks(cb)`). The NEW `internal/filter/http/wasm/` package follows the multi-file split per parent §4.4 + this 25.1 SPEC §3.5 (8 production + 5 test files; `wasm` single-token Go package identifier matching `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac`/`lua` precedent — no underscore needed). The filter shape: BOTH `StreamDecoderFilter` AND `StreamEncoderFilter` (`Decoder: non-nil`; `Encoder: non-nil`) mirroring phase-22.1 lua's both-sides shape — `proxy_on_request_headers` fires at `DecodeHeaders`; `proxy_on_response_headers` fires at `EncodeHeaders`; per-stream `*VM` constructed at `DecodeHeaders` entry via `wasm.NewVM(opts...)`; `vm.RegisterABICallbacks(&abiCallbacks{filter: f, ...})` wires the HTTP-context bundle; `vm.Run(ctx, cfg.module, cfg.rootContextID)` executes module-init lifecycle (instantiates `*wazero.CompiledModule` onto fresh `*wazero.Runtime` + `_initialize` or `_start` + `proxy_on_vm_start` + `proxy_on_configure`); `vm.CallProxyOnContextCreate(ctx, streamContextID, rootContextID)` constructs per-stream context; `vm.CallProxyOnRequestHeaders(ctx, streamContextID, headerCount, endOfStream)` returns `ProxyAction::CONTINUE` (=0) or `ProxyAction::PAUSE` (=1); captured `proxy_send_local_response` state on filter struct → `cb.SendLocalReply` per parent §4.2 if fired; `cfg.stats.executions++` after CallProxyOnRequestHeaders regardless of outcome; wazero trap or hostcall-denial chain → `cfg.stats.envoy_go.failures++` + log; encode-side mirrors for `proxy_on_response_headers`; `OnDestroy` calls `vm.CallProxyOnDone(streamContextID)` + `vm.CallProxyOnLog(streamContextID)` + `vm.CallProxyOnDelete(streamContextID)` + `vm.Close()` releasing the `*wazero.Runtime`. The compiled-config holds: `module *wasm.Module` (pre-compiled wasm bytecode — single module at 25.1; multi-plugin lands at 25.3) + `compileCache *wasm.CompileCache` (filter-config-instance scope; GC-driven eviction; no per-route override at 25.1) + `sandbox wasm.SandboxConfig` (from `PluginConfig.capability_restriction_config`; zero-value = StrictDefaultDeny per AMEND-A5 + ADR-0204) + `pluginName string` (from `PluginConfig.name`; Group-C stat-prefix discriminator per AMEND-A2) + `rootContextID uint32` (plugin-context discriminator from `PluginConfig.root_id`; allocated as u32 counter at config-load) + `vmConfig []byte` + `pluginConfig []byte` (passed to `proxy_on_vm_start` + `proxy_on_configure`; deferred-hostcall surface at 25.1) + `stats *filterStats` (5 counters SHARED across listener — no per-route at 25.1). The 5 counters allocate unconditionally at `New()` time mirroring phase-17/18.1/19.1/20/21/22.1 unconditional-allocation discipline; per ADR-0085 nil-tolerance: `buildCompiledConfig` guards `if ctx.Stats != nil` before `newFilterStats`. The default-deny capability sandbox per §3.3 + AMEND-A5 implementation discipline: `NewVM` registers the 24-active-hostcall env-namespace host module + the 8-active-WASI-shim `wasi_snapshot_preview1`-namespace custom-shim host module + 23 deferred-25.2/25.3 stub-returns-Unimplemented hostcalls per parent §4.2 Option B (modules importing deferred hostcalls succeed at instantiation but receive `WasmResult::Unimplemented` (=12) when invoked); each hostcall body reads `vm.sandbox.IsAllowed(capabilityName)` before invoking the ABICallbacks method; denied calls return `WasmResult::InternalFailure` (=10) + emit integration error log + increment `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict counter per AMEND-A2); WASI denials use `WasiErrno::ENOTCAPABLE` (=76) anticipated per D-P1 first-action scrape at Task 2 OR fallback `WasiErrno::NOTSUP` (=58) if wazero's WASI semantics prevent the exact return code. The byte-faithful pairs wire format at `internal/wasm/pairs.go` (per R3 + parent §13-R3): reimplements `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc` byte-faithfully — `u32 num_pairs / u32 key_len, u32 value_len (repeated num_pairs times) / key_bytes NUL value_bytes NUL (repeated num_pairs times)`. The byte-faithful ABI-version detection at `internal/wasm/bytecode_util.go` (per AMEND-A6 + parent §13-R2): reimplements `proxy-wasm-cpp-host:bytecode_util.cc:32-97` byte-faithfully — scans wasm module export section (type 7) for function-kind exports named `proxy_abi_version_0_2_1` / `proxy_abi_version_0_2_0` / `proxy_abi_version_0_1_0`; returns detected version OR `AbiVersionUnknown`; v0.1.0 + v0.2.0 + missing-sentinel all PARSE-REJECT per AMEND-A6 (envoy-go-strict-stricter departure NOT parity — upstream accepts all 3 ABI versions and version-dispatches). The WASI custom 8-stub implementation at `internal/wasm/wasi.go` (per R4 + parent §13-R4): do NOT use wazero's built-in `imports/wasi_snapshot_preview1` package (which routes `fd_write` to host stdout/stderr; we need it routed through `proxy_log` per the proxy-wasm semantics) — `fd_write` routes to `proxy_log` (fd=1 → INFO, fd=2 → ERROR, other → `WasiErrno::BADF`=8); `clock_time_get` host-accuracy (CLOCK_REALTIME=0 wall + CLOCK_MONOTONIC=1 monotonic); `random_get` via `crypto/rand`; `environ_sizes_get`/`environ_get`/`args_sizes_get`/`args_get` return 0/0 at 25.1+25.2 (deferred to 25.3 for `environ_*`; never-implemented for `args_*`); `proc_exit` traps via `sys.NewExitError(exit_code)` — well-behaved guests MUST NOT call. The per-stream `*wazero.Runtime` construction WEAK-default per parent §13-R8 STANDS at 25.1 IMPL Task 17 benchmark sub-task; if benchmark surfaces > 1ms unacceptable overhead, the ADR-0205 escape-valve slot anchors a "per-module wazero Runtime pool with pre-instantiated entries" decision (§Context + §Decision + §Consequences body all land at the same Task 17 commit per ADR-0044). The differential fixture-0034 driver implements the 7 cross-side scenarios (add-fixed-header / replace-header / remove-header / respond-shortcircuit / log-only-passthrough / header-iteration-count / property-read-method) per §4.5 D6 guardrails (no memory traps; HTTP/1.1; no float-formatted logs; only 24-hostcall surface); scenario (e) `log-only-passthrough` uses `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch` (subject-side `wasm.<plugin>.executions` counter delta assertion on the cross-side runner branch; deliberately-break test verifies liveness) — NOT `SubjectAsserter.AssertSubject` (which is dead on the cross-side path). The differential fixture-0035 driver implements the `BootRejectFixture` interface (existing harness from phase-22.1 Task 13 `BootRejectFixture` infrastructure); anticipated boot-reject arm = 5 (`vm-config-code-required`) per D-P6 with anticipated common stderr substring `"required"` reproduced by envoy-go's mirror wording. The 25.1 SPEC §6 17-task breakdown across 4 tiers is the load-bearing input to this PLAN; each Task corresponds 1:1 to a PLAN entry below (Tier A framework primitive Tasks 1-7; Tier B filter package Tasks 8-13; Tier C tests + fixtures Tasks 14-16; Tier D atomic landing Task 17). The 17 tasks comfortably fit ADR-0045's 25-task split-gate; the LoC envelope per parent §3.0 estimate (~3,650-5,360 production+test+fixture IMPL) sits above the ~1500 LoC PLAN-size soft-gate (the PLAN gate is about PLAN.md size, not IMPL LoC; the IMPL LoC sizing per Task is settled at the 25.1 SPEC §6) but acceptable per the EXTRACT-NOW-primitive-bring-up precedent from phase-22.1 (which also exceeded the LoC-arm at framework-primitive bring-up).

**Tech Stack:** Go 1.26.2 (Go-floor STAYS at `go 1.23.0` per wazero v1.10.1's Go-1.23 floor per AMEND-A1); `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/wasm/v3` for the `Wasm` proto; `envoy/extensions/wasm/v3` for `PluginConfig` + `VmConfig` + `CapabilityRestrictionConfig` + `SanitizationConfig`; `envoy/config/core/v3` for `AsyncDataSource` + `DataSource` + `WatchedDirectory` sibling); **NEW direct dependency** `github.com/tetratelabs/wazero v1.10.1` (pure-Go WebAssembly runtime; Apache-2.0 license per AMEND-A1 correcting BRAINSTORM "MIT-licensed" typo; CNCF Sandbox; Go 1.23 floor; NO CGO — fits envoy-go's pure-Go portability constraint per ADR-0008) — `go.mod` + `go.sum` updates at Task 1 first action; stdlib `crypto/sha256` for the `CompileCache` content-hash key (32-byte sha256 of source); stdlib `crypto/rand` for the WASI `random_get` shim; stdlib `os` (`os.ReadFile` for the `DataSource.Filename` arm; `os.LookupEnv` for the `DataSource.EnvironmentVariable` arm); stdlib `sync` (`sync.RWMutex` for the `CompileCache` concurrent-read-add discipline); stdlib `time` (host-accuracy `clock_time_get` shim); stdlib `log` (the `proxy_log` host-side integration log per the existing filter project precedent + the captured-trap-fallback path); stdlib `encoding/binary` (pairs wire format little-endian u32 encoding per `proxy-wasm-cpp-host:pairs_util.cc`); stdlib `bytes` + `io` (proxy-log sink writer + WASI shim buffer construction); stdlib `fmt` (PARSE-REJECT wording wrapping); stdlib `errors` (`errors.New` for byte-stable PARSE-REJECT consts); stdlib `context` (per-stream context threading; no new contracts vs prior phases); reference Envoy `envoyproxy/envoy:v1.37.2` SHA per ADR-0008 + ENVOY_TARGET.md (unchanged); proxy-wasm specification v0.2.1 (sentinel export `proxy_abi_version_0_2_1`); proxy-wasm-rust-sdk `=0.2.4` + `wasm32-wasip1` Rust target (per AMEND-A1; reproduction-source language for the 7 fixture-0034 plugins under `scripts/` subdirectory; pre-built `.wasm` bytecode vendored under `bytecode/` subdirectory per Q9); proxy-wasm-cpp-host `da3ce05d` reference (transcription source for `bytecode_util.cc` + `pairs_util.{h,cc}` byte-faithful reimplementations); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (NO TLS surface at phase-25.1).

---

## Scope check — why phase 25.1 ships as one sub-phase row (settled at parent BRAINSTORM Q1 PRE-SPLIT discipline)

Phase 25 was PRE-SPLIT THREE-way at the parent BRAINSTORM commit per BRAINSTORM Q1 (envelope D delivered across 25.1 + 25.2 + 25.3 — `25.1-http-filter-wasm-runtime-and-headers-bridge` foundational third; `25.2-http-filter-wasm-body-and-advanced-bridge` full Envoy↔Wasm advanced bridge delta; `25.3-http-filter-wasm-per-route-and-conformance` per-route 5th-canonical wholesale-override + multi-plugin VM-sharing + conformance harness seed). This PLAN is for the 25.1 sub-phase ONLY; no further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward; matches phase-18.1 + phase-19.1 + phase-22.1 + phase-24.1 sub-phase PLAN precedent). The 25.2 + 25.3 sibling stubs at `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/README.md` + `docs/envoy-go/phases/25.3-http-filter-wasm-per-route-and-conformance/README.md` document the deferred surface; the full 25.2 + 25.3 SPECs are drafted at each sub-phase's lifecycle-state 1 after 25.1 phase-done.

The PLAN-time re-evaluation per `superpowers:writing-plans` GATE + ADR-0045 §6 confirms single-sub-phase landing:

- **Task count: 17** — comfortably under the ADR-0045 25-task split-gate. Matches the 25.1 SPEC §6 17-task breakdown verbatim (Tier A 1-7 framework primitive + Tier B 8-13 filter package + Tier C 14-16 fuzzer + 2 fixtures + Tier D 17 atomic landing).
- **LoC: ~4,200-4,800 production+test+fixture+docs** (per parent §3.0 estimate ~3,650-5,360; ~2,200-2,800 LoC `internal/wasm/` production + tests per 25.1 SPEC §3.2; ~1,200-1,500 LoC `internal/filter/http/wasm/` production + tests per 25.1 SPEC §3.5; ~800-1,000 LoC fuzzer + 2 fixtures + benchmark + BEHAVIOR_CONTRACT.md + ADR bodies + STATE.md + ROADMAP.md + PROGRESS.md + REVIEW.md). The IMPL LoC sits above the ~1500 LoC PLAN-size soft-gate, **but the PLAN gate is about PLAN.md size** (this PLAN at ~1300-1500 lines sits at the soft-gate boundary), not IMPL LoC — the IMPL LoC sizing per Task is settled at the 25.1 SPEC §6. The phase-22.1 EXTRACT-NOW-primitive-bring-up precedent (which also exceeded the LoC-arm at framework-primitive bring-up; no further nested split was triggered) RATIFIES this disposition.
- **Phase 25.1 ships as the single sub-phase row it is** — no further nested split. The 25.1 phase-done squash-merge **CLOSES row 25.1** (in-progress → done) at the same commit; parent row `25` STAYS `in-progress` until 25.3 IMPL phase-done per the sub-row rollup discipline per ADR-0106 + phase-18.1 + phase-18.2 + phase-19.1 + phase-19.2 + phase-22.1/22.2/22.3 + phase-24.1/24.2 precedent.

Net change estimate for 25.1 (mirroring the phase-09..24 + phase-22.1 PLAN component-table convention):

- `internal/wasm/doc.go` ~80-130 (package overview + Q1-Q9 BRAINSTORM decision summary + AMEND-A1..A9 cross-refs + wazero v1.10.1 + proxy-wasm v0.2.1 ABI pin + API surface summary; lands at Task 1)
- `internal/wasm/abi/types.go` ~150-220 (`WasmResult` 10 named values with value-gaps at 5/9/11 per AMEND-A7 + `WasmBufferType` 9 values; value 8 = `FOREIGN_FUNCTION_ARGUMENTS` per AMEND-A7 + `WasmHeaderMapType` 8 values + `LogLevel` 6 values + `ProxyAction` 2 values + `WasiErrno` subset roster; lands at Task 1)
- `internal/wasm/abi/types_test.go` ~150-220 (value-faithful encoding tests per AMEND-A7; value-gap preservation critical — guest modules check specific integer values; lands at Task 1)
- `internal/wasm/bytecode_util.go` ~200-300 (byte-faithful reimplementation of `proxy-wasm-cpp-host:bytecode_util.cc:32-97` per AMEND-A6 — scans wasm module export section type 7 for the `proxy_abi_version_0_2_1` sentinel; lands at Task 2)
- `internal/wasm/bytecode_util_test.go` ~250-380 (crafted-wasm-binary fixtures ~5-10 binaries: valid v0.2.1 + v0.1.0 + v0.2.0 + Unknown + malformed export section + truncated module; lands at Task 2)
- `internal/wasm/pairs.go` ~120-180 (byte-faithful serialization + deserialization per R3 + parent §13-R3 — transcribed from `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc`; lands at Task 3)
- `internal/wasm/pairs_test.go` ~200-300 (golden-bytes table per cpp-host oracle + round-trip tests + boundary checks + malformed inputs; lands at Task 3)
- `internal/wasm/wasi.go` ~250-350 (custom 8-stub WASI implementation per R4 + parent §13-R4 — `fd_write` routes to `proxy_log`, `proc_exit` traps, `clock_time_get` + `random_get` at host accuracy; lands at Task 4)
- `internal/wasm/wasi_test.go` ~250-380 (each shim's golden semantics + bad-fd/bad-arg error paths; lands at Task 4)
- `internal/wasm/compile.go` ~150-220 (`Module` + `CompileCache` + `NewCompileCache` + `CompileModule` + `AbiVersion` enum + sha256-keyed caching + `sync.RWMutex` discipline + `ErrUnsupportedAbiVersion` sentinel; lands at Task 5)
- `internal/wasm/compile_test.go` ~200-300 (cache-hit-on-same-content + cache-miss-on-different + concurrent read/add + AbiVersion detection tests; lands at Task 5)
- `internal/wasm/sandbox.go` ~250-380 (`SandboxConfig` + `SanitizationConfig` + `IsAllowed` + the ~80-key capability roster constants per §3.3; lands at Task 6)
- `internal/wasm/sandbox_test.go` ~300-450 (per-capability ALLOW/DENY exhaustive; verifies default-deny posture per AMEND-A5; lands at Task 6)
- `internal/wasm/vm.go` ~450-650 (`VM` type per §3.1 + `VMOption` function-option pattern + `NewVM` + `State` + `RegisterABICallbacks` + `Run` + `HasGlobalFunc` + `CallProxyOnContextCreate`/`CallProxyOnRequestHeaders`/`CallProxyOnResponseHeaders`/`CallProxyOnDone`/`CallProxyOnLog`/`CallProxyOnDelete` + `Close` + panic-wrapper + `WithSandboxConfig` + `WithPanicHandler` + `WithLogSink`; lands at Task 7)
- `internal/wasm/registration.go` ~350-550 (`ABICallbacks` interface + `HeaderPair` + host-module wiring for 24 active hostcalls + 23 deferred stubs registered against `wazero.Runtime`; lands at Task 7)
- `internal/wasm/vm_test.go` ~300-450 (per-stream construction round-trip + sandbox-deny dispatch for each capability key + panic-wrapper behavior + concurrent VMs share no state; lands at Task 7)
- `internal/wasm/registration_test.go` ~250-380 (ABICallbacks interface invocation + host-module wiring round-trip tests; lands at Task 7)
- `internal/filter/http/wasm/doc.go` ~60-100 (package overview + Q1-Q9 BRAINSTORM decision summary + AMEND-A1..A9 cross-refs + D-P1..D-P6 cross-refs + API surface summary; lands at Task 8)
- `internal/filter/http/wasm/wasm.go` ~120-180 (filter struct + factory `New` + `TypeURL` + `filterName` + per-route validator registration; skeleton at Task 8; full body wiring extends across Tasks 11+12; ~150-220 LoC cumulative)
- `internal/filter/http/wasm/stats.go` ~80-120 (5-counter `filterStats` per AMEND-A2 tri-group prefix structure with HCM-stats_prefix DROPPED; `newFilterStats` constructor; lands at Task 8)
- `internal/filter/http/wasm/wasm_test.go` ~400-700 (filter + factory + filterStats + decode/encode wiring + per-stream VM lifecycle integration; ~1500-2000 LoC cumulative across Tasks 8+11+12; stat-name table-driven verification 114 → 119)
- `internal/filter/http/wasm/compiled_config.go` ~300-450 (`compiledConfig` struct per §4.2 + `buildCompiledConfig` with 18-arm PARSE-REJECT roster per parent §6.2 byte-stable wording per D-P5; `parseReject*` package-private consts; lands at Task 9)
- `internal/filter/http/wasm/compiled_config_test.go` ~450-650 (18-arm PARSE-REJECT table-driven + `TestParseRejectConstants_ByteStable` per D-P5; lands at Task 9)
- `internal/filter/http/wasm/datasource.go` ~150-220 (4-arm `AsyncDataSource.Local` resolution + `WatchedDirectory` PARSE-REJECT + `Remote` PARSE-REJECT + empty-oneof PARSE-REJECT; lands at Task 10)
- `internal/filter/http/wasm/datasource_test.go` ~300-450 (4-arm + ENOENT + unset env var + empty-inline-bytes failure paths; lands at Task 10)
- `internal/filter/http/wasm/abi_callbacks.go` ~500-750 (implements `wasm.ABICallbacks` for the per-stream HTTP-filter context — 7 header-map methods + GetProperty (minimal property tree) + SetProperty + SendLocalResponse + GetStatus (consumes ADR-0196 per D-P3 + R7) + Log + GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done; lands at Task 11)
- `internal/filter/http/wasm/abi_callbacks_test.go` ~500-700 (all 13-callback subset round-trip tests + minimal property tree exhaustive coverage + GetStatus ADR-0196 first co-consumer round-trip + sandbox-deny dispatch for each capability key; lands at Task 11)
- `internal/filter/http/wasm/decode_headers.go` ~150-220 (`DecodeHeaders` per §4.3 dispatch + per-stream VM construction + Run + CallProxyOnContextCreate + CallProxyOnRequestHeaders + ProxyAction handling + captured-local-response handoff; lands at Task 12)
- `internal/filter/http/wasm/encode_headers.go` ~100-150 (`EncodeHeaders` symmetric to decode for `proxy_on_response_headers`; lands at Task 12)
- `internal/filter/http/wasm/fuzz_test.go` ~100-150 (34th project-wide fuzzer `FuzzWasmConfigParse` + ~30 corpus seeds covering all 18 PARSE-REJECT arms + valid configs + adversarial wasm bytecode; lands at Task 14)
- `internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/` (corpus seeds ~30 total per D-P-PLAN-7 below; lands at Task 14)
- `internal/filter/http/wasm/wasm_bench_test.go` ~50-80 (`BenchmarkPerStreamVM_Construction_Headers` per R8 gate; lands at Task 17)
- `cmd/envoy-go/main.go` ~+2 LoC + +1 import (`wasm "github.com/esalaine/envoy-go/internal/filter/http/wasm"` alphabetical-among-imports; `httpReg.Register(wasm.TypeURL, wasm.New)` inserted alphabetically after `router` per ADR-0100 §2.2 — `wasm` sorts after `router` so it appends to the tail of the 19-entry roster); lands at Task 13
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind=HTTPWasm` enum value after the highest existing BackendKind; dispatcher metadata mirroring phase-22.1's `HTTPLua` precedent; lands at Task 15)
- `test/differential/runner_test.go` ~+12 (blank import for `internal/filter/http/wasm` test consumer; switch-case for `HTTPWasm`; lands at Task 15) + ~+5 LoC blank import for fixture-0035 (lands at Task 16)
- `test/fixtures/0034-http-wasm-headers-bridge/README.md` ~150-250 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/envoy.yaml` ~150-250 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/envoy-go.yaml` ~150-250 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/expectations.yaml` ~100-180 (Task 15; human-readable; NOT consumed by runner)
- `test/fixtures/0034-http-wasm-headers-bridge/inputs/driver.go` ~400-600 (Task 15 — registered `Driver` impl + per-scenario probes + classifyBody + `StatsAsserter.AssertStats` implementation for scenario (e) per `reference_differential_asserter_dispatch`)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/a_add_header/{Cargo.toml,src/lib.rs}` ~30 (Task 15; Rust source per Q9 + AMEND-A1)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/b_replace_header/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/c_remove_header/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/d_respond/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/e_log_only/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/f_headers_count/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/g_property_method/{Cargo.toml,src/lib.rs}` ~30 (Task 15)
- `test/fixtures/0034-http-wasm-headers-bridge/scripts/README.md` ~60-100 (Task 15; reproduction script + pinned rustup toolchain + cargo build invocation)
- `test/fixtures/0034-http-wasm-headers-bridge/bytecode/{a..g}.wasm` (Task 15; 7 vendored pre-built `.wasm` binary blobs committed to git per Q9 + AMEND-A1)
- `test/fixtures/0035-http-wasm-boot-reject/README.md` ~80-120 (Task 16)
- `test/fixtures/0035-http-wasm-boot-reject/envoy.yaml` ~100-150 (Task 16; reference Envoy bootstrap with missing `vm_config.code` per anticipated D-P6 arm 5)
- `test/fixtures/0035-http-wasm-boot-reject/envoy-go.yaml` ~100-150 (Task 16; subject bootstrap symmetric)
- `test/fixtures/0035-http-wasm-boot-reject/inputs/driver.go` ~150-250 (Task 16; implements `BootRejectFixture` interface)
- `go.mod` + `go.sum` ~+8 LoC delta (NEW `github.com/tetratelabs/wazero v1.10.1` direct dep + transitive; Task 1)
- `docs/envoy-go/DECISIONS.md` — 3 ADR §Decision + §Consequences bodies anchored at Task 17 (ADR-0202 + ADR-0203 + ADR-0204; CONDITIONAL ADR-0205 only if R8 escape-valve fires per D-P-PLAN-10); ~+500-800 LoC delta
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+300-450 LoC (Task 17 6-edit bundle per parent §13.5 + this 25.1 SPEC §14)
- `docs/envoy-go/ROADMAP.md` row 25.1 flips `in-progress → done` at Task 17; per-cell IMPL-done annotation; parent row `25` UNCHANGED `in-progress`; sub-rows `25.2` + `25.3` UNCHANGED `planned`; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place at Task 17
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` (NEW) ~800-1100 across 17 task entries + Pre-Task 0
- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/REVIEW.md` (NEW) ~300-400

**Production code: ~3,000-4,400 LoC** (`internal/wasm/` ~2,000-2,950 + `internal/filter/http/wasm/` ~1,000-1,450 + boot-reg ~30-50 + enum +15 + runner-switch +12 + fuzzer corpus ~100-200) **+ ~2,500-3,800 LoC tests** + ~1,100-1,800 LoC fixture-0034 + fixture-0035 (including 7 Rust sources + 7 vendored `.wasm` blobs + 2 boot-reject configs) + ~1,200-1,700 LoC docs ≈ **~7,800-11,700 LoC total**. **Task count: 17** — comfortably under the ADR-0045 25-task split-gate; LoC-arm above 1500 threshold but acceptable per phase-22.1 EXTRACT-NOW-primitive-bring-up precedent.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/wasm/doc.go` | NEW | Package doc per 25.1 SPEC §3.2 — package overview; Q1-Q9 BRAINSTORM decision summary; AMEND-A1..A9 cross-references; wazero v1.10.1 Apache-2.0 + Go-1.23-floor pin per AMEND-A1; proxy-wasm v0.2.1 ABI pin per AMEND-A6; API surface summary (`VM` + `VMOption` + `NewVM` + `State`/`RegisterABICallbacks`/`Run`/`HasGlobalFunc`/per-callback methods/`Close` + `Module` + `CompileCache` + `NewCompileCache` + `CompileModule` + `SandboxConfig` + `PanicHandlerFn` + zero-value `StrictDefaultDeny` defaults per AMEND-A5); ADR-0202 cross-reference. ~80-130 LoC. Lands at Task 1. |
| `internal/wasm/abi/types.go` | NEW | `WasmResult` 10 named values with value-gaps at 5/9/11 per AMEND-A7 (`Ok=0`, `NotFound=1`, `BadArgument=2`, `SerializationFailure=3`, `ParseFailure=4`, [GAP 5], `BadExpression=6` IF v0.2.1 RATIFIES — verify at Task 1; `InvalidMemoryAccess=7`, `Empty=8`, [GAP 9], `InternalFailure=10`, [GAP 11], `Unimplemented=12`); `WasmBufferType` 9 values; value 8 = `FOREIGN_FUNCTION_ARGUMENTS` per AMEND-A7 (NOT `CallData` as BRAINSTORM hypothesized); `WasmHeaderMapType` 8 values; `LogLevel` 6 values; `ProxyAction` 2 values (`CONTINUE=0`, `PAUSE=1`); `WasiErrno` subset roster (`BADF=8`, `NOTSUP=58`, `ENOTCAPABLE=76`); value-faithful encoding is CRITICAL — guest modules check specific integer values. ~150-220 LoC. Lands at Task 1. |
| `internal/wasm/abi/types_test.go` | NEW | Value-faithful round-trip tests per AMEND-A7 — assert specific integer values for each enum constant; value-gap preservation tests (assert no 5, 9, 11 value collisions); WasmBufferType=8 = `FOREIGN_FUNCTION_ARGUMENTS` byte-exact assertion. ~150-220 LoC. Lands at Task 1. |
| `internal/wasm/bytecode_util.go` | NEW | Byte-faithful reimplementation of `proxy-wasm-cpp-host:bytecode_util.cc:32-97` per AMEND-A6 + parent §13-R2. `BytecodeUtil.GetAbiVersion(src []byte) (AbiVersion, error)` scans wasm module export section (type 7) for function-kind exports named `proxy_abi_version_0_2_1` / `proxy_abi_version_0_2_0` / `proxy_abi_version_0_1_0`; returns detected version OR `AbiVersionUnknown`. Internal helpers: `parseSection` (parses section header — section type + section size LEB128); `parseExportSection` (parses export section — count + per-export name length + name bytes + kind + index); `parseFunctionExportName` (compares against the 3 sentinel strings byte-exactly — 24 / 24 / 24 UTF-8 ASCII bytes). Returns wrapped error on malformed input. ~200-300 LoC. Lands at Task 2. |
| `internal/wasm/bytecode_util_test.go` | NEW | Crafted-wasm-binary fixtures (~5-10 binaries): valid v0.2.1 (`proxy_abi_version_0_2_1` exported as function-kind) + valid v0.1.0 + valid v0.2.0 + missing-sentinel + malformed export section (truncated count; bogus kind byte) + truncated module (cuts off mid-section); each test asserts the returned AbiVersion + error disposition byte-exactly. Helper `mustBuildModule(t, name)` synthesizes binaries at runtime (avoids vendored fixtures). ~250-380 LoC. Lands at Task 2. |
| `internal/wasm/pairs.go` | NEW | Byte-faithful pairs wire format reimplementation per R3 + parent §13-R3. Transcribed from `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc`. Wire format: `u32 num_pairs / u32 key_len, u32 value_len (repeated num_pairs times) / key_bytes NUL value_bytes NUL (repeated num_pairs times)` little-endian. `HeaderPair struct { Key, Value string }`; `EncodePairs(pairs []HeaderPair) []byte`; `DecodePairs(buf []byte) ([]HeaderPair, error)`. Errors on: truncated buffer; pair-count overflow; key-len + value-len overflow; missing NUL terminator. ~120-180 LoC. Lands at Task 3. |
| `internal/wasm/pairs_test.go` | NEW | Golden-bytes table-driven tests (per cpp-host pairs_util.cc oracle): empty pairs `[u32(0)]` → `[]`; single pair `{"foo": "bar"}` byte-output; multi-pair byte-output. Round-trip tests: `DecodePairs(EncodePairs(x)) == x`. Malformed input: truncated header; oversize length count; missing NUL; out-of-bounds offsets. ~200-300 LoC. Lands at Task 3. |
| `internal/wasm/wasi.go` | NEW | Custom 8-stub WASI implementation per R4 + parent §13-R4. Do NOT use wazero's built-in `imports/wasi_snapshot_preview1`. Implements `fd_write` (fd=1 → `proxy_log(INFO, msg)`; fd=2 → `proxy_log(ERROR, msg)`; other → `WasiErrno::BADF`=8); `clock_time_get` (CLOCK_REALTIME=0 via `time.Now()`; CLOCK_MONOTONIC=1 via `time.Now()` monotonic; host-time accuracy at 25.1 — no fake-time seam); `random_get` (`crypto/rand.Read(buf)`); `environ_sizes_get` (0/0); `environ_get` (writes nothing); `args_sizes_get` (0/0); `args_get` (writes nothing); `proc_exit(exit_code)` (traps via `sys.NewExitError(exit_code)`). All stubs honor sandbox capability gate (`sandbox.IsAllowed("fd_write")`, etc.); denied path returns `WasiErrno::ENOTCAPABLE=76` (or `NOTSUP=58` per D-P1 first-action scrape at Task 2). ~250-350 LoC. Lands at Task 4. |
| `internal/wasm/wasi_test.go` | NEW | Each shim's golden semantics + bad-fd/bad-arg paths: `fd_write` with fd=1, fd=2, fd=99 (BADF), zero-length iovec; `clock_time_get` returns sensible monotonic-increasing values; `random_get` writes the requested byte count + buffer is non-zero; `environ_sizes_get` returns 0/0; `args_sizes_get` returns 0/0; `proc_exit` traps with expected exit code; sandbox-deny returns `ENOTCAPABLE` or `NOTSUP` per D-P1. ~250-380 LoC. Lands at Task 4. |
| `internal/wasm/compile.go` | NEW | `Module struct { compiled wazero.CompiledModule; abiVer AbiVersion; hash [32]byte }`; `CompileCache struct { mu sync.RWMutex; store map[[32]byte]*Module; rt wazero.Runtime }`; `NewCompileCache(ctx context.Context) *CompileCache` constructs shared compile-only runtime; `CompileModule(ctx, src, cache) (*Module, error)` computes `sha256(src)` → cache lookup (RLock); miss → compile via wazero parser → `bytecode_util.GetAbiVersion(src)` → if `AbiVersion_0_2_1` → cache add (Lock) + return; if other → `ErrUnsupportedAbiVersion` wrapped per AMEND-A6 PARSE-REJECT wording (arm 16). `cache == nil` → compile uncached (ADR-0085 nil-tolerance). `AbiVersion` enum + `ErrUnsupportedAbiVersion` sentinel. `CompileCache.Close()` releases compile-only runtime (idempotent). ~150-220 LoC. Lands at Task 5. |
| `internal/wasm/compile_test.go` | NEW | `NewCompileCache` returns non-nil; `CompileModule` happy path (valid wasm v0.2.1 bytecode); cache-hit-on-same-content-hash (same src → same `*Module` pointer); cache-miss-on-different-source; nil-cache tolerance (compiles uncached, no side effects); concurrent read/add tests (`-race` clean under N goroutines mixing same-src + new-src compiles); `ErrUnsupportedAbiVersion` returned for v0.1.0/v0.2.0/missing-sentinel bytecode; compile-error path (malformed wasm bytecode → wrapped wazero error). ~200-300 LoC. Lands at Task 5. |
| `internal/wasm/sandbox.go` | NEW | `SandboxConfig struct { AllowedCapabilities map[string]SanitizationConfig }`; `SanitizationConfig struct{}` (empty per AMEND-A1 §11.4 accept-empty-as-no-op); `IsAllowed(capabilityName string) bool` returns `_, ok := sb.AllowedCapabilities[capabilityName]; return ok` (empty map → DENY ALL per AMEND-A5 — INVERTS upstream's empty-map-allow-all semantic). Package-private capability-key constants for the 25.1 surface — 7 headers-bridge family + 1 local-response + 2 property + 2 log + 1 status + 1 time + 2 context-lifecycle + 8 WASI bare-names + 5 module-init/allocator (gated per D-P2 at Task 6 first-action; anticipated ungated) + 8 lifecycle/HTTP module-getter capability keys = ~37-39 unique capability keys at 25.1 (out of the ~80-key full roster). ~250-380 LoC. Lands at Task 6. |
| `internal/wasm/sandbox_test.go` | NEW | Per-capability ALLOW/DENY exhaustive table-driven: empty map denies all 37+ capability keys; populated map allows only named keys (verifies set membership semantic); `SanitizationConfig` non-empty values parse-and-discard (no PARSE-REJECT; matches phase-24's INERT acceptance per AMEND-4). D-P2 closure verification: assert the 5 module-init/allocator keys are NOT capability-gated (ungated by design — required for instantiation). ~300-450 LoC. Lands at Task 6. |
| `internal/wasm/vm.go` | NEW | `VM` type per 25.1 SPEC §3.1 production signatures (`VM struct { runtime wazero.Runtime; module wazero.CompiledModule; instance api.Module; sandbox SandboxConfig; panicH PanicHandlerFn; logSink io.Writer; cb ABICallbacks; ctxStore map[uint32]*context.Context }`); `VMOption func(*VM)`; `WithSandboxConfig(sb SandboxConfig) VMOption`; `WithPanicHandler(h PanicHandlerFn) VMOption`; `WithLogSink(w io.Writer) VMOption`; `NewVM(ctx, opts...) *VM` (constructs `wazero.Runtime` interpreter-default per parent §2.7 + registers env-namespace + wasi_snapshot_preview1-namespace host modules via `registration.go`); `State() wazero.Runtime` escape-hatch; `RegisterABICallbacks(cb ABICallbacks)`; `Run(ctx, module, rootContextID) error` (instantiate `*wazero.CompiledModule` + `_initialize` OR `_start` + `proxy_on_vm_start` + `proxy_on_configure`); `HasGlobalFunc(name string) bool`; per-callback methods `CallProxyOnContextCreate`/`CallProxyOnRequestHeaders`/`CallProxyOnResponseHeaders`/`CallProxyOnDone`/`CallProxyOnLog`/`CallProxyOnDelete`; `Close() error` (idempotent); panic-wrapper invokes `WithPanicHandler` after `recover()`. ~450-650 LoC. Lands at Task 7. |
| `internal/wasm/registration.go` | NEW | `ABICallbacks` interface — 24-method headers-bridge subset (7 header-map methods + GetProperty + SetProperty + SendLocalResponse + GetStatus + Log + GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done + 8 WASI shim methods); `HeaderPair struct { Key, Value string }` (re-export from pairs.go); `registerHostModules(rt wazero.Runtime, vm *VM)` registers the 16 active `proxy_*` env-namespace hostcalls + the 8 active `wasi_snapshot_preview1.*` custom-shim hostcalls + the 23 deferred-25.2/25.3 stub-Unimplemented hostcalls per parent §4.2 Option B. Each hostcall body reads `vm.sandbox.IsAllowed(capabilityName)` before invoking ABICallbacks method; denied → `WasmResult::InternalFailure`=10 + log + `hostcall_denied` counter bump signal (counter bump landed at filter package via the integration hook). ~350-550 LoC. Lands at Task 7. |
| `internal/wasm/vm_test.go` | NEW | Per-stream construction round-trip (NewVM → RegisterABICallbacks → Run with crafted-wasm + ABI callbacks → CallProxyOnRequestHeaders → assert ProxyAction → Close); option application verification (WithSandboxConfig + WithPanicHandler + WithLogSink independently); panic-wrapper behavior (Go panic in ABICallbacks Go method → recover() invokes PanicHandlerFn → converts to error return); sandbox-deny dispatch tests (deny each capability key → assert `WasmResult::InternalFailure` returned to guest); concurrent VMs share no state (N goroutines each NewVM → Run → Close against same `*Module`; assert no cross-VM leak; race-free under `-race`); Close idempotent. ~300-450 LoC. Lands at Task 7. |
| `internal/wasm/registration_test.go` | NEW | ABICallbacks interface invocation round-trip (guest invokes each `proxy_*` hostcall → ABICallbacks method fires with expected args → returns expected result → result returns to guest); host-module wiring verification (24 active + 23 deferred = 47 total hostcalls registered); deferred-stub returns `WasmResult::Unimplemented`=12 when invoked. ~250-380 LoC. Lands at Task 7. |
| `internal/filter/http/wasm/doc.go` | NEW | Package doc per 25.1 SPEC §3.5 — package overview; Q1-Q9 BRAINSTORM decision summary (envelope D + 3-way pre-split + wazero + EXTRACT-NOW framework primitive + 4-arm AsyncDataSource.Local + default-deny capability sandbox + 5th-canonical REUSE-by-absence + cross-side fixture + Rust-sourced vendored bytecode); AMEND-A1..A9 cross-references; D-P1..D-P6 cross-references; API surface summary (`TypeURL` + `New` factory); ADR-0203 cross-reference. ~60-100 LoC. Lands at Task 8. |
| `internal/filter/http/wasm/wasm.go` | NEW | Filter struct + `New` factory (`HTTPFilterFactory` per ADR-0072); `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"`; `filterName = "envoy.filters.http.wasm"`; per-route validator registration (one-liner returning the arm-18 PARSE-REJECT). Filter struct holds the per-stream state: `cfg *compiledConfig` + `vm *wasm.VM` + `streamContextID uint32` + `sentLocalResponse *capturedLocalResponse`. `var _ StreamDecoderFilter = (*filter)(nil)` + `var _ StreamEncoderFilter = (*filter)(nil)` compile-time assertions. Task 8 lands SKELETON (filter struct + `New` returning sentinel error; real parse lands at Task 9); Tasks 11+12 wire the full body. ~120-180 LoC cumulative. |
| `internal/filter/http/wasm/stats.go` | NEW | 5-counter `filterStats` per AMEND-A2 tri-group prefix structure with HCM-stats_prefix DROPPED. Group B (wasm.<runtime>.* prefix; <runtime> = "wazero"): `created *stats.Counter` (`wasm.wazero.created`) + `active *stats.Gauge` (`wasm.wazero.active`). envoy-go-strict extensions (wasm.<plugin_name>.* prefix): `executions *stats.Counter` (`wasm.<plugin_name>.executions`) + `hostcallDenied *stats.Counter` (`wasm.<plugin_name>.hostcall_denied`) + `envoyGoFailures *stats.Counter` (`wasm.<plugin_name>.envoy_go.failures`). `newFilterStats(reg *stats.Registry, pluginName string) *filterStats` constructs via `reg.NewCounter` + `reg.NewGauge`. Package-level const declarations for the 5 stat names. ~80-120 LoC. Lands at Task 8. |
| `internal/filter/http/wasm/wasm_test.go` | NEW | Filter + factory integration tests (`TypeURL` constant assertion; `New` happy path); `filterStats` 5-counter registration + cardinality assertion (114 → 119 project delta) + tri-group prefix structure verification; `TestStatNames_Equal_*` table-driven 5-stat-name byte-exact assertion. Task 8 lands skeleton; Task 11 + Task 12 extend across abi_callbacks + decode/encode integration via test-double `DecoderFilterCallbacks` + `EncoderFilterCallbacks`. ~400-700 LoC cumulative. |
| `internal/filter/http/wasm/compiled_config.go` | NEW | `compiledConfig struct { module *wasm.Module; compileCache *wasm.CompileCache; sandbox wasm.SandboxConfig; pluginName string; rootContextID uint32; vmConfig []byte; pluginConfig []byte; stats *filterStats }`; `buildCompiledConfig(typedConfig *anypb.Any, ctx FilterFactoryContext) (*compiledConfig, error)` full body covering 18-arm PARSE-REJECT roster per parent §6.2 with byte-stable error wording per D-P5: arms 1-2 universal `typed-config-*`; arm 3 `name-required`; arm 4 `root-id-required` (provisional — verify at Task 9 first-action against upstream); arm 5 `vm-config-required` (`"wasm: config.vm_config is required"`); arm 6 `code-async-remote-deferred` (`"wasm: config.vm_config.code.remote is not yet supported; use local AsyncDataSource (envoy-go-strict)"`); arm 7 `watched-directory-deferred` (`"wasm: config.vm_config.code.local.watched_directory is not yet supported"`); arm 8 `data-source-specifier-required`; arms 9-10 `fail-reload-deferred` + `fail-open-deferred`; arm 11 `runtime-discriminator-rejected` (`"wasm: config.vm_config.runtime: only '' or 'envoy.wasm.runtime.wazero' supported (envoy-go-strict)"`); arm 12 `duplicate-vm-id-deferred`; arm 13 `environment-variables-deferred`; arm 14 `allow-precompiled-deferred`; arm 15 `nack-on-code-cache-miss-deferred`; arm 16 `abi-version-rejected` (`"wasm: config.vm_config.code: unsupported ABI version; only proxy_abi_version_0_2_1 supported (envoy-go-strict)"`); arm 17 `compile-failed` (`"wasm: config.vm_config.code: compile: %w"` wrapping wazero error); arm 18 `per-route-deferred` (registered via `RegisterPerRouteValidator`). Defaults: `sandbox` zero-value = `StrictDefaultDeny`. ~300-450 LoC. Lands at Task 9. |
| `internal/filter/http/wasm/compiled_config_test.go` | NEW | 18-arm table-driven PARSE-REJECT tests per parent §6.2: each row is `{name string, configMutator func(*wasmv3.Wasm), wantErrSubstring string}`. ~20-25 rows covering arms 1-18 + 5-10 valid-config rows (each AsyncDataSource arm with valid contents). `TestParseRejectConstants_ByteStable` table-driven verifies each `parseReject*` constant string byte-exact per D-P5. D-P5 closure at this Task; commit-time enforcement. ~450-650 LoC. Lands at Task 9. |
| `internal/filter/http/wasm/datasource.go` | NEW | `resolveDataSource(local *corev3.DataSource) ([]byte, error)` dispatches across the 4-arm DataSource oneof (`Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → byte-cast `[]byte(s)`; `EnvironmentVariable` → `os.LookupEnv`); `WatchedDirectory` sibling field PARSE-REJECT (arm 7); empty `specifier` oneof PARSE-REJECT (arm 8). Per-arm empty-content PARSE-REJECTs (filename name-empty / ENOENT / zero-byte; env-var name-empty / unset / empty-value). Byte-stable wording per parent §6.2 arms 6-15. ~150-220 LoC. Lands at Task 10. |
| `internal/filter/http/wasm/datasource_test.go` | NEW | 4-arm resolution + WatchedDirectory PARSE-REJECT + empty-oneof PARSE-REJECT + per-arm failure paths: `Filename` happy + ENOENT + EACCES + EISDIR + zero-byte + name-empty (via `t.TempDir()` synthetic files); `InlineBytes` happy + zero-byte; `InlineString` happy + empty-string; `EnvironmentVariable` happy + unset + empty-value + name-empty. ~300-450 LoC. Lands at Task 10. |
| `internal/filter/http/wasm/abi_callbacks.go` | NEW | `abiCallbacks struct { filter *filter; cfg *compiledConfig; decoderCb DecoderFilterCallbacks; encoderCb EncoderFilterCallbacks }` implementing `wasm.ABICallbacks` for the per-stream HTTP-filter context: 7 header-map methods (`GetHeaderMapPairs(typ) []HeaderPair`, `SetHeaderMapPairs(typ, []HeaderPair)`, `GetHeaderMapValue(typ, key) (string, bool)`, `AddHeaderMapValue(typ, key, value)`, `ReplaceHeaderMapValue(typ, key, value)`, `RemoveHeaderMapValue(typ, key)`, `GetHeaderMapSize(typ) uint32`); `GetProperty(path string) ([]byte, WasmResult)` minimal property tree (`request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host`); `SetProperty(key, value)`; `SendLocalResponse(statusCode, statusMsg, body, additionalHeaders, grpcStatus)` (captures `*capturedLocalResponse` on filter struct; consumed in decode/encode handlers); `GetStatus() (statusCode uint32, value string)` (RE-CONSUMES `EncoderFilterCallbacks.ResponseStatus()` per ADR-0196 + D-P3 + R7 — FIRST co-consumer of phase-23 primitive); `Log(level, msg)` (routes to filter log sink); `GetLogLevel() LogLevel`; `GetCurrentTimeNanoseconds() uint64` (`time.Now().UnixNano()`); `SetEffectiveContext(contextID)`; `Done()`. ~500-750 LoC. Lands at Task 11. |
| `internal/filter/http/wasm/abi_callbacks_test.go` | NEW | All 13-callback subset round-trip tests + minimal property tree exhaustive coverage (all 5 property paths return correct values; unknown property returns NotFound); GetStatus ADR-0196 first co-consumer round-trip (verifies the encoder-callback shape works on encode-path); sandbox-deny dispatch for each capability key returns `WasmResult::InternalFailure`; SendLocalResponse captures all 8 args byte-faithfully on the filter struct. ~500-700 LoC. Lands at Task 11. |
| `internal/filter/http/wasm/decode_headers.go` | NEW | `DecodeHeaders(headers Header, endStream bool)` per 25.1 SPEC §4.3 dispatch: construct per-stream `*VM` via `wasm.NewVM(ctx, wasm.WithSandboxConfig(cfg.sandbox), wasm.WithLogSink(filterLog))`; `vm.RegisterABICallbacks(&abiCallbacks{filter: f, cfg: cfg, decoderCb: f.dcb, encoderCb: nil})`; `vm.Run(ctx, cfg.module, cfg.rootContextID)` (module-init lifecycle); `vm.CallProxyOnContextCreate(ctx, f.streamContextID, cfg.rootContextID)`; `cfg.stats.executions++`; `vm.CallProxyOnRequestHeaders(ctx, f.streamContextID, uint32(len(headers)), endStream)`; if `f.sentLocalResponse != nil` → `cb.SendLocalReply(captured.statusCode, captured.body, captured.additionalHeaders)` + return `StopIteration`; else `ProxyAction::CONTINUE` → `Continue`; `ProxyAction::PAUSE` w/o local-response → log + `Continue` (stream-control hostcalls deferred to 25.2 per §1 architectural primitive 6). If `vm.CallProxyOnRequestHeaders` returned an error (wazero trap or hostcall-denial chain) → `cfg.stats.envoyGoFailures++` + log + `Continue` (NO wire-side error surface). `OnDestroy` calls `vm.CallProxyOnDone(streamContextID)` + `vm.CallProxyOnLog(streamContextID)` + `vm.CallProxyOnDelete(streamContextID)` + `vm.Close()`. ~150-220 LoC. Lands at Task 12. |
| `internal/filter/http/wasm/encode_headers.go` | NEW | `EncodeHeaders(headers Header, endStream bool)` symmetric to decode for `proxy_on_response_headers`; same ProxyAction handling + captured-local-response handoff. The per-stream `*VM` constructed at DecodeHeaders is RE-USED at EncodeHeaders (filter struct holds `vm *wasm.VM` — single VM per stream across decode + encode + OnDestroy phases). ~100-150 LoC. Lands at Task 12. |
| `internal/filter/http/wasm/fuzz_test.go` | NEW | 34th project-wide fuzzer `FuzzWasmConfigParse` per ADR-0018 baseline. Must-never-panic across `buildCompiledConfig()`. Corpus seeds ~30 total per D-P-PLAN-7: 18 PARSE-REJECT arms (1 per arm) + 5 valid-config seeds (1 per AsyncDataSource arm with valid contents) + 7 adversarial-bytecode seeds (malformed wasm headers / oversize sections / sentinel-spoof attempts). Lands at Task 14. ~100-150 LoC + `testdata/fuzz/FuzzWasmConfigParse/` corpus directory. |
| `internal/filter/http/wasm/wasm_bench_test.go` | NEW | `BenchmarkPerStreamVM_Construction_Headers(b *testing.B)` measures per-stream `*wazero.Runtime` construction cost at the headers-only bridge surface (constructs N=b.N fresh VMs back-to-back with a canned-valid `*Module` from shared CompileCache; reports `ns/op`). Threshold gate per R8 + D-P-PLAN-10: if `ns/op > 1_000_000` (= 1ms), the ADR-0205 escape-valve FIRES at Task 17; ADR-0205 §Context + §Decision + §Consequences body all land at the same Task 17 commit per ADR-0044. If `ns/op <= 1_000_000`, the WEAK-default per-stream construction STANDS; no ADR-0205 fires. Benchmark output quoted verbatim in Task 17 PROGRESS.md entry. ~50-80 LoC. Lands at Task 17. |
| `cmd/envoy-go/main.go` | MODIFY | +1 LoC + +1 import per ADR-0100 §2.2 alphabetical after `router`. Task 13: add `wasm "github.com/esalaine/envoy-go/internal/filter/http/wasm"` alphabetical-among-imports; add `httpReg.Register(wasm.TypeURL, wasm.New)` insertion at the line immediately before `httpReg.Freeze()` (no entry after `router` at master tip; `wasm` sorts after `router` so it appends to the tail of the 19-entry roster — verify at Task 13 first-action via `grep httpReg.Register cmd/envoy-go/main.go`). 19 → 20 HTTP filters wired post-25.1. Per ADR-0072 registration order does NOT affect runtime behavior; stylistic discipline only. **NO `RegisterPerRouteValidator` call delegation** at boot — instead, the per-route validator is registered via `reg.RegisterPerRouteValidator(filterName, validatePerRouteWasm)` from inside `wasm.New` itself per the parent §5.2 + ADR-0110 single-chokepoint discipline (matches phase-10/20/19.1/22.1 precedent); the validator function is a one-liner returning the arm-18 PARSE-REJECT. |
| `test/differential/fixture/fixture.go` | MODIFY | +1 enum value `HTTPWasm BackendKind = <next>` (after the highest existing BackendKind; ~+15 LoC including doc-comment per existing BackendKind comment style). Lands at Task 15. |
| `test/differential/runner_test.go` | MODIFY | +blank import for `internal/filter/http/wasm`; +switch-case for `HTTPWasm` (~+12 LoC) — Task 15. +blank import for `test/fixtures/0035-http-wasm-boot-reject` (~+1 LoC) — Task 16. Fixture-0035 uses the existing `BootRejectFixture` harness from phase-22.1 Task 13 (no harness delta at 25.1). |
| `test/fixtures/0034-http-wasm-headers-bridge/README.md` | NEW | Top-level fixture-directory README — scope + 7-scenario table + topology + cross-refs to parent SPEC §8 + this 25.1 SPEC §9.1 + ADR-0202 + ADR-0203 + ADR-0204. ~150-250 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/envoy.yaml` | NEW | Reference Envoy bootstrap; single listener + wasm filter consuming `Wasm.config.vm_config.code` via `Filename` arm pointing to `bytecode/<scenario>.wasm`; templated `{{.BackendPort}}`. ~150-250 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/envoy-go.yaml` | NEW | Subject bootstrap; same topology; templated `{{.AdminPort}} {{.ListenerPort}} {{.BackendPort}} {{.FixtureDir}}`. ~150-250 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/expectations.yaml` | NEW | Human-readable declarative scenario expectations (NOT consumed by runner; documentation aid). ~100-180 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/inputs/driver.go` | NEW | Registered `Driver` impl + per-scenario probes via `driveProxy` + `emitScenario` + `classifyBody` mirroring fixture-0026 (phase-22.1) pattern; for scenario (e) implements `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)` per `reference_differential_asserter_dispatch` (NOT `SubjectAsserter` which is dead on the cross-side path); scrapes `/stats?format=text` admin endpoints + diffs `wasm.<plugin>.executions` counter delta; asserts cross-side `executions_delta = 1` per probe. ~400-600 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/a_add_header/Cargo.toml + src/lib.rs` | NEW | Rust source per Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target); `proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-wasm-injected", "hello")`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/b_replace_header/Cargo.toml + src/lib.rs` | NEW | `proxy_replace_header_map_value(HTTP_REQUEST_HEADERS, "user-agent", "envoy-go-wasm/1.0")`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/c_remove_header/Cargo.toml + src/lib.rs` | NEW | `proxy_remove_header_map_value(HTTP_REQUEST_HEADERS, "x-blocked")`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/d_respond/Cargo.toml + src/lib.rs` | NEW | `proxy_send_local_response(403, "Forbidden", "denied", &[], 0)`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/e_log_only/Cargo.toml + src/lib.rs` | NEW | `proxy_log(INFO, "wasm hit")`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/f_headers_count/Cargo.toml + src/lib.rs` | NEW | `proxy_get_header_map_pairs` + count + add `x-headers-count: N`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/g_property_method/Cargo.toml + src/lib.rs` | NEW | `proxy_get_property("request.method")` + add `x-request-method: GET`. ~30 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/scripts/README.md` | NEW | Reproduction script + pinned rustup toolchain + cargo build invocation (`rustup target add wasm32-wasip1` + `cargo build --release --target wasm32-wasip1`). ~60-100 LoC. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/a_add_header.wasm` | NEW | Vendored pre-built `.wasm` binary blob committed to git per Q9 + AMEND-A1. Lands at Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/b_replace_header.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/c_remove_header.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/d_respond.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/e_log_only.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/f_headers_count.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0034-http-wasm-headers-bridge/bytecode/g_property_method.wasm` | NEW | (same) Task 15. |
| `test/fixtures/0035-http-wasm-boot-reject/README.md` | NEW | Boot-reject fixture README — scope + arm 5 disposition + cross-side common stderr substring per D-P6. ~80-120 LoC. Lands at Task 16. |
| `test/fixtures/0035-http-wasm-boot-reject/envoy.yaml` | NEW | Reference Envoy bootstrap with deliberately-malformed Wasm config triggering anticipated D-P6 arm 5 (`vm-config-code-required` — missing `vm_config.code`). ~100-150 LoC. Lands at Task 16. |
| `test/fixtures/0035-http-wasm-boot-reject/envoy-go.yaml` | NEW | Subject bootstrap symmetric. ~100-150 LoC. Lands at Task 16. |
| `test/fixtures/0035-http-wasm-boot-reject/inputs/driver.go` | NEW | Implements `BootRejectFixture` interface (`BootRejectConfig() string` returns the bootstrap path; `ExpectedBootErrorSubstring() string` returns the anticipated common stderr substring — verified at Task 16 first-action against upstream Envoy v1.37.2 boot stderr). Per `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch; this is the boot-reject branch (NOT cross-side). ~150-250 LoC. Lands at Task 16. |
| `go.mod` + `go.sum` | MODIFY | +`github.com/tetratelabs/wazero v1.10.1` direct dep; transitive deps if any; `go mod tidy` clean. Go-floor STAYS at `go 1.23.0` per wazero's Go-1.23 floor per AMEND-A1. Lands at Task 1. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 3 ADR §Decision + §Consequences bodies anchored at Task 17 (ADR-0202 + ADR-0203 + ADR-0204; CONDITIONAL ADR-0205 only if R8 escape-valve fires per D-P-PLAN-10). ~+500-800 LoC delta. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | Task 17 6-edit bundle per parent §13.5 + this 25.1 SPEC §14: (1) NEW `### envoy.filters.http.wasm` subsection ~150-250 LoC; (2) Stat-table 114 → 119 extension under `## Stat surface` + tri-group prefix structural note + HCM-stats_prefix DROPPED divergence-from-§9-family-pattern note; (3) envoy-go-strict departure record #1: default-deny capability sandbox per AMEND-A5 + ADR-0204; (4) envoy-go-strict departure record #2: ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6; (5) envoy-go-strict departure record #3 (consolidated 4-5-record bundle): `AsyncDataSource.Remote` PARSE-REJECT + runtime-name discriminator PARSE-REJECT + 3 envoy-go-strict counters (`executions` + `hostcall_denied` + `envoy_go.failures`); (6) NEW `### Phase 25.1 forward-pointer notes` subsection ~50-80 LoC (25.2 + 25.3 anticipated additions). ~+300-450 LoC delta. |
| `docs/envoy-go/ROADMAP.md` | MODIFY | Row 25.1 flips `in-progress → done` at Task 17; per-cell IMPL-done annotation; parent row `25` STAYS `in-progress`; sub-rows `25.2` + `25.3` UNCHANGED `planned`. ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFY | Rewrite-in-place at Task 17: `lifecycle-state: phase 25.1 IMPL done; awaiting 25.2 SPEC`; `next-skill: superpowers:brainstorming` (25.2 BRAINSTORM scoped to 25.2 sub-phase) OR `superpowers:writing-plans` (if 25.2 BRAINSTORM-skip per the parent-BRAINSTORM-settled-enough pattern); `next-free ADR: ADR-0205` (UNCHANGED if R8 escape-valve does NOT fire) or `ADR-0206` (if fires); 114 → 119 stat-count update; 19 → 20 HTTP filter count update; 33 → 34 fuzzer count; ADR tail advance to ADR-0204 (or ADR-0205 if escape-valve fires); per-task SHA-fill follow-up commit per phase-09..24 convention. |
| `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` | NEW | Append-only task log per phase-22.1+23+24.1 IMPL precedent + `superpowers:verification-before-completion` discipline; 17 task entries + Pre-Task 0; each entry quotes command outputs verbatim + records acceptance-criteria evidence per task + records D-P1..D-P6 closure evidence at the relevant Tasks 2/6/9/11/16/17. ~800-1100 LoC. |
| `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/REVIEW.md` | NEW | Task 17 reviewer artifact per `superpowers:requesting-code-review` per phase-22.1+23+24.1 IMPL precedent; per-task review notes + cross-cutting review notes + green-light evidence + 30-item acceptance checklist closure per this 25.1 SPEC §15.3. ~300-400 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC D-questions discipline + PLAN-emerged decisions)

The 25.1 SPEC §12 D-questions D-P1..D-P6 are ANTICIPATED at SPEC commit + close at IMPL-time first-action scrapes (D-P1 at Task 2; D-P2 at Task 6; D-P3 at Task 11; D-P5 at Task 9; D-P6 at Task 16) + at the R8 benchmark gate (D-P4 at Task 17). This PLAN does NOT re-litigate D-P1..D-P6 anticipated answers — they carry forward as IMPL-time first-action steps in the relevant Tasks per the SPEC. Additional PLAN-emerged decisions D-P-PLAN-1..D-P-PLAN-10 settle here.

1. **D-P-PLAN-1 — SPEC §6 17-task numbering INHERITED VERBATIM; PROGRESS.md preamble + precondition check is "Pre-Task 0" (NOT a renumbered Task 1).** Settle: this PLAN's Tasks 1-17 map 1:1 to the 25.1 SPEC §6 17-task breakdown per the cold-start prompt's explicit instruction. The PROGRESS.md preamble + 15-precondition verification (which the phase-21 PLAN names Task 1 because that SPEC did not pre-allocate task numbers in §6) is here labeled **Pre-Task 0** + executed at IMPL session cold-start before SPEC §6 Task 1 begins. Mirrors phase-22.1 + phase-23 + phase-24.1 PLAN precedent. *Anchored: 25.1 PLAN cold-start prompt verbatim + phase-22.1 PROGRESS.md ritual precedent.*

2. **D-P-PLAN-2 — Per-task subagent dispatch type LOCKED at `general-purpose` for code Tasks 1-16; Task 17 atomic landing dispatched via `general-purpose` with explicit acceptance-checklist reference; REVIEW.md via `superpowers:code-reviewer` per `superpowers:requesting-code-review`.** Settle: per project memory `feedback_execution_style.md` (user always wants subagent-driven over inline execution for plans), each Task's IMPL session subagent-dispatches per `superpowers:subagent-driven-development`. Dispatch type per Task: Tasks 1-16 use `general-purpose` agent (Go code work); Task 17 uses `general-purpose` with explicit reference to 25.1 SPEC §15.3 30-item acceptance checklist + the BEHAVIOR_CONTRACT.md 6-edit bundle anatomy + the ADR-0202/0203/0204 §Decision + §Consequences body sketches from this 25.1 SPEC §3.1 + §3.5 + §13. REVIEW.md at IMPL Task 17 final step dispatched via `superpowers:code-reviewer`. *Anchored: project memory `feedback_execution_style.md` + phase-22.1+23+24.1 IMPL precedent + `superpowers:subagent-driven-development` skill.*

3. **D-P-PLAN-3 — Per-task PROGRESS.md entry shape LOCKED per phase-22.1 IMPL precedent.** Settle: each Task's PROGRESS.md entry contains the following sections in order:
   - **Task ID + title** (matches the SPEC §6 task ID + title verbatim + this PLAN's task heading);
   - **Acceptance criteria** (verbatim cross-reference to 25.1 SPEC §6's "Acceptance:" line for the task + this PLAN's Task heading's `Acceptance:` line);
   - **Files touched** (the precise list from this PLAN's Task heading's `Files:` block);
   - **Verification command outputs** (the exact commands from this PLAN's Task Step bodies' Run-tests-verify-they-pass phase + the verbatim stdout/stderr quoted in fenced code blocks per `superpowers:verification-before-completion` discipline);
   - **Acceptance-criteria evidence** (per-criterion pass/fail with brief reasoning + cross-reference to the verification command output);
   - **D-question disposition update** (if the task closes a D-question — D-P1 at Task 2, D-P2 at Task 6, D-P3 at Task 11, D-P5 at Task 9, D-P6 at Task 16, D-P4 at Task 17; the entry records the empirical evidence + resolved disposition);
   - **Commit SHA** (`git log -1 --format=%H` for the task's commit);
   - **Tier + Task-number cross-reference** (e.g., "Tier A framework primitive (Task 2 of 7 in tier; Task 2 of 17 overall)").
   *Anchored: phase-22.1 + phase-23 + phase-24.1 PROGRESS.md format precedent + `superpowers:verification-before-completion` + this PLAN's per-Task structure.*

4. **D-P-PLAN-4 — Per-task TDD ordering LOCKED at test-first for ALL 16 code Tasks (1-16) per `superpowers:test-driven-development` rigid discipline; Task 17 is the atomic-landing meta-task.** Settle: every Task that lands production code (Tasks 1-13; Task 14 fuzzer follows TDD with seed corpus first; Tasks 15+16 fixture bundles follow relaxed test-with-implementation — the differential fixture IS the integration test) follows the rigid TDD ordering: (Step 1) write the failing test in the corresponding `*_test.go` file; (Step 2) run the test to verify it fails; (Step 3) implement the minimal production code; (Step 4) run the test to verify it passes; (Step 5) run `go build ./... + go vet ./... + golangci-lint run` clean; (Step 6) append PROGRESS.md Task entry per D-P-PLAN-3; (Step 7) commit. Tasks 15+16 (fixture bundles) follow: author bootstrap configs + driver.go + Rust sources + vendor `.wasm` blobs + register BackendKind → run `go test ./test/differential -run TestDifferential/0034` → assert GREEN → append PROGRESS → commit. The Skill's documentation classifies TDD as RIGID — adherence is mandatory. *Anchored: `superpowers:test-driven-development` rigid discipline + phase-22.1+23+24.1 IMPL precedent.*

5. **D-P-PLAN-5 — `CompileCache` scope LOCKED at `compiledConfig`-instance (not cross-stream global; not cross-listener global).** Settle: the `*wasm.CompileCache` is owned by the `*compiledConfig` (filter-config-instance scope; one cache per listener filter-chain mounting a wasm filter); GC-driven eviction (the cache lifetime equals the `compiledConfig` lifetime; eviction happens when the listener drains + the `compiledConfig` is released). NO cross-listener / cross-process global cache. Rationale: (i) 25.1 has a single module per `compiledConfig` (the single `vm_config.code`); the cache's primary purpose is to forward-pin the API shape for 25.3 when multi-plugin VM-sharing adds multiple modules per listener; (ii) cross-listener sharing is unsafe (different listeners may have different sandbox configs — though at 25.1 all share the StrictDefaultDeny zero-value); (iii) `sync.RWMutex` discipline at the cache level scales to N concurrent compile calls per stream per phase 25.3 multi-plugin; (iv) GC-driven eviction matches the project's existing `sync.Pool`-free precedent. Mirrors phase-22.1 D-P5 disposition. *Anchored: 25.1 SPEC §3.1 + §3.4 + this PLAN-time emerge + phase-22.1 D-P5 precedent.*

6. **D-P-PLAN-6 — Boot-registration position EMPIRICAL-VERIFIED at Task 13 first-action via `grep httpReg.Register` against master tip; LOCKED at alphabetical after `router` per ADR-0100 §2.2.** Settle: `cmd/envoy-go/main.go` currently registers 19 HTTP-filter entries before `httpReg.Freeze()` per 25.1 SPEC §3.6 (alphabetical roster: `adaptive_concurrency`, `admissioncontrol`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `extauthz`, `extproc`, `fault`, `globalratelimit`, `header_mutation`, `jwtauthn`, `localratelimit`, `lua`, `oauth2`, `rbac`, `router`). The 20th entry `wasm.New` inserts alphabetically **between `router` and the freeze** (no entry after `router` at master tip; `wasm` sorts after `router` so it appends to the tail of the registration block). Insertion line is immediately before `httpReg.Freeze()`. Task 13 first-action: re-verify the alphabetical roster via `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` — if a successor row landed between this PLAN session and IMPL session, adjust the insertion line accordingly per the SPEC §3.6 D-P-AUX note. Per ADR-0072 + ADR-0100 §2.2 — registration order does not affect runtime behavior; stylistic discipline only. NO `RegisterPerRouteValidator` call delegation at boot — the per-route validator is registered via `reg.RegisterPerRouteValidator(filterName, validatePerRouteWasm)` from inside `wasm.New` itself per parent §5.2 + ADR-0110 single-chokepoint discipline; the validator function is a one-liner returning the arm-18 PARSE-REJECT (`"wasm: per-route configuration is not yet supported (lands in phase 25.3)"`). *Anchored: 25.1 SPEC §3.6 + ADR-0100 §2.2 + ADR-0072 + ADR-0110 + this PLAN-time emerge.*

7. **D-P-PLAN-7 — Fuzzer corpus seed roster for `FuzzWasmConfigParse` LOCKED per 25.1 SPEC §6 Task 14 + parent §15 Layer C.** Settle: corpus seeds at `internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/` covering:
   - **Per PARSE-REJECT arm** (18 seeds; 1 per arm) — one fixture triggering each of the 18 PARSE-REJECT arms from parent §6.2;
   - **Valid-config seeds** (5 seeds) — `vm_config.code.Filename` valid path; `vm_config.code.InlineBytes` valid wasm v0.2.1 bytecode; `vm_config.code.InlineString` valid wasm v0.2.1 bytecode (cast); `vm_config.code.EnvironmentVariable` valid var name; valid config with non-empty `PluginConfig.capability_restriction_config`;
   - **Adversarial-wasm-bytecode seeds** (7 seeds) — malformed wasm headers (bad magic; bad version); oversize section size (claimed bigger than buffer); sentinel-spoof attempts (export `proxy_abi_version_0_2_1` as a non-function kind); truncated module; null bytes; structurally-valid-but-semantically-broken (e.g., function with no body); broken control flow (unbalanced blocks). Must-never-panic invariant via wazero compile error path (arm 17 wrapping).

   Total corpus floor: ~30 seeds. Must-never-panic across `buildCompiledConfig()` per ADR-0018. Clean at 30s per seed. *Anchored: 25.1 SPEC §6 Task 14 + parent §15 Layer C + ADR-0018 + this PLAN-time emerge.*

8. **D-P-PLAN-8 — Task graph parallelization LOCKED per planner-time emerge.** Settle: after Pre-Task 0 (PROGRESS.md preamble + precondition check) lands, the 17-task graph allows parallelization at multiple points:

   - **After Task 1** (package skeleton + abi/types.go + wazero dep): Tasks 2 (`bytecode_util.go`) + 3 (`pairs.go`) + 4 (`wasi.go`) can run in PARALLEL (3-way) — each is file-disjoint within `internal/wasm/` package root + each depends only on Task 1's skeleton.
   - **After Tasks 2 + 3 + 4**: Task 5 (`compile.go` — depends on `bytecode_util.go` for ABI-version gating). Task 6 (`sandbox.go`) can also start in PARALLEL with Task 5 — file-disjoint + no compile.go dependency.
   - **After Task 5 + Task 6**: Task 7 (`vm.go` + `registration.go` — depends on compile.go for Module type + sandbox.go for SandboxConfig + wasi.go for WASI shim + pairs.go for HeaderPair + bytecode_util.go for ABI types). Sequential bottleneck.
   - **After Task 7**: Tasks 8 (filter skeleton + stats) + 9 (compiled_config + 18-arm PARSE-REJECT) + 10 (datasource) can run in PARALLEL (3-way) — file-disjoint within `internal/filter/http/wasm/`. Task 9 also depends on Task 7's `*wasm.Module` + `wasm.CompileModule` API. Task 8 depends only on package skeleton (could partially start earlier, but the filter struct needs the abi_callbacks shape from Task 11 — skeleton at Task 8 is fine).
   - **After Tasks 8 + 9 + 10**: Task 11 (`abi_callbacks.go` — depends on `compiled_config.go` for compiledConfig struct + `datasource.go` for resolved bytes path). Sequential bottleneck (D-P3 closure first-action).
   - **After Task 11**: Task 12 (`decode_headers.go` + `encode_headers.go` — depends on Task 11's abi_callbacks for `*abiCallbacks{filter: f, ...}` construction). Sequential.
   - **After Task 12**: Task 13 (boot-registration at `cmd/envoy-go/main.go`). Sequential.
   - **After Task 13**: Tasks 14 (fuzzer) + 15 (fixture-0034) can run in PARALLEL (2-way) — Task 14 only depends on Task 9's `buildCompiledConfig` being non-skeleton; Task 15 needs the full filter wired (Tasks 1-13). Task 16 (fixture-0035) lands after Task 15 (small `runner_test.go` blank-import conflict).
   - **After Tasks 14 + 15 + 16**: Task 17 (atomic landing — benchmark + BEHAVIOR_CONTRACT.md 6-edit bundle + ADR-0202+0203+0204 §Decision+§Consequences + STATE.md re-advance + ROADMAP row 25.1 flip + CONDITIONAL ADR-0205 if R8 fires + REVIEW.md authoring). Depends on everything.

   **Parallel-dispatch opportunities**: 3-way at Tasks 2+3+4; 2-way at Tasks 5+6; 3-way at Tasks 8+9+10; 2-way at Tasks 14+15. **Sequential bottlenecks**: Pre-Task 0 → Task 1 → {2,3,4}; {2,3,4} → {5,6}; {5,6} → Task 7; Task 7 → {8,9,10}; {8,9,10} → Task 11 → Task 12 → Task 13; Task 13 → {14,15} → Task 16 → Task 17. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits these parallel opportunities. *Anchored: 25.1 SPEC §6 4-tier breakdown + this PLAN-time emerge.*

9. **D-P-PLAN-9 — Cross-package regression-test command shape LOCKED.** Settle: after each task lands its production code, the implementer runs the package-local test command (`go test -count=1 -race ./internal/wasm/...` for Tasks 1-7; `go test -count=1 -race ./internal/wasm/... ./internal/filter/http/wasm/...` for Tasks 8-12; `go test -count=1 ./test/differential -run TestDifferential/0034` for Task 15; `go test -count=1 ./test/differential -run TestDifferential/0035` for Task 16; full `go test -count=1 -race ./...` at Task 17 final gate). At Task 17 Gate D the full regression `go test -count=1 ./test/differential/...` runs ALL fixture directories (pre-existing + the 2 new 0034+0035); verify count post-25.1 = pre-existing + 2 via `ls -d test/fixtures/00*/ | wc -l` (anticipated 37 if pre-existing baseline confirms at 35 per Pre-Task 0 precondition 13). Per 25.1 SPEC §15 expected outcome: zero regression. *Anchored: 25.1 SPEC §15 Layer E + phase-22.1 D-P9 precedent + this PLAN-time emerge.*

10. **D-P-PLAN-10 — `BenchmarkPerStreamVM_Construction_Headers` LOCKED at Task 17 with explicit > 1ms threshold gating per parent §13-R8.** Settle: Task 17 (atomic landing) ALSO includes a benchmark `BenchmarkPerStreamVM_Construction_Headers` at `internal/filter/http/wasm/wasm_bench_test.go` measuring per-stream `*wazero.Runtime` construction cost at the headers-only bridge surface (constructs N=b.N fresh VMs back-to-back; reports `ns/op` via `b.N` discipline). The threshold gate per parent §13-R8 + D-P4: if `ns/op > 1_000_000` (= 1ms), the ADR-0205 escape-valve FIRES at Task 17; ADR-0205 §Context + §Decision + §Consequences body all land at the same Task 17 commit per ADR-0044 anchoring a "per-module wazero Runtime pool with pre-instantiated entries" decision. If `ns/op <= 1_000_000`, the WEAK-default per-stream construction STANDS; no ADR-0205 fires; next-free ADR-0205 stays UNCONSUMED carried forward to 25.2 BRAINSTORM. The benchmark result quoted verbatim in Task 17 PROGRESS.md entry. **Anticipated answer per D-P4**: under threshold — phase-22.1 observed 70µs at the analogous benchmark; wazero compiler-mode initialization is comparable or faster per parent §1.2 hypothesis. *Anchored: parent §13-R8 + this 25.1 SPEC §13 R8 STANDS + 25.1 SPEC §2.28 + this PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The 25.1 IMPL lands 3 ADR §Decision + §Consequences bodies at Task 17 atomic landing per ADR-0044 (the §Context drafts already anchored at parent SPEC commit `2c1455d` per parent §4.1 + §4.4 + §4.3); 1 CONDITIONAL ADR landing at Task 17 only if R8 escape-valve fires per D-P-PLAN-10. **NO new ADRs consumed at any task before Task 17.** The ADR-0125 §canonical-per-route-roster STAYS at 10 across all of phase 25 per AMEND-A3 (NO §(xvi) amendment); NO in-place ADR-0125 amendment at this PLAN commit + at Task 17.

| ADR | Subject (25.1 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0202** | NEW `internal/wasm/` framework primitive — wazero v1.10.1 Apache-2.0 per AMEND-A1 + per-stream VM lifecycle (`*VM` + `*wazero.Runtime` construction + `*Module` compile cache + `SandboxConfig` per-capability ALLOW/DENY zero-value `StrictDefaultDeny` posture per AMEND-A5 + ABI-registration interface `ABICallbacks` + panic-wrapper + log-sink discipline) + the in-house proxy-wasm v0.2.1 host ABI types (`WasmResult` 10 named values with value-gaps at 5/9/11 per AMEND-A7 + `WasmBufferType` 9 values; value 8 = `FOREIGN_FUNCTION_ARGUMENTS`) + the byte-faithful `bytecode_util.go` ABI-version detection (transcribed from `proxy-wasm-cpp-host:bytecode_util.cc:32-97` per AMEND-A6) + the byte-faithful `pairs.go` wire-format reimplementation (transcribed from `proxy-wasm-cpp-host:pairs_util.h` per R3) + the WASI custom 8-stub implementation (per R4) + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per parent §4.1 + BRAINSTORM Q3 + Q4 (the broader §9 WASM host family — cluster-specifier-wasm at `envoy.router.cluster_specifiers.wasm`; access-logger-wasm at `envoy.access_loggers.wasm`; network-filter-wasm at `envoy.filters.network.wasm`; WasmService singleton plugin loaders — whichever materializes first). Refined production signatures per 25.1 SPEC §3.1 vs parent §4.1 sketch (`VMOption` function-option pattern; `Run/HasGlobalFunc/CallProxyOnX` per-callback method split; `State()` escape-hatch; `WithPanicHandler`/`WithLogSink` naming clarification). 8 production + 8 test files in package root + 1 `abi/` subdirectory per §3.2. | Task 17 |
| **ADR-0203** | NEW `internal/filter/http/wasm/` package shape — 8 production + 5 test files per parent §4.4 + this 25.1 SPEC §3.5; `compiledConfig` + 5-counter `filterStats` per §4 with tri-group prefix structure per AMEND-A2 (Group-B upstream-parity `wasm.wazero.{created,active}` + envoy-go-strict extensions `wasm.<plugin_name>.{executions, hostcall_denied, envoy_go.failures}`; HCM-stats_prefix DROPPED structural note); 18-arm PARSE-REJECT roster per parent §6.2 + D-P5 byte-stable wording finalization at Task 9; 4-arm `AsyncDataSource.Local` resolution per parent §5.4; 24-hostcall ABICallbacks implementation (16 `proxy_*` env-namespace + 8 `wasi_snapshot_preview1.*` custom-shim) per §5; 13-callback guest-export surface (5 module-init/allocator + 6 lifecycle + 2 HTTP) per §5.3; minimal property tree (`request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host`); `proxy_send_local_response` captured-state handoff per parent §4.2; ADR-0196 first co-consumer at `proxy_get_status` per D-P3 + R7; fixture-0034 7-scenario disposition per parent §8.1 + §4.5 D6 guardrails + `StatsAsserter.AssertStats` for scenario (e); fixture-0035 single-arm boot-reject parity per D-P6 (anticipated arm 5); per-route 5th-canonical REUSE-by-absence per AMEND-A3 (ADR-0125 STAYS at 10; NO §(xvi) amendment at 25.3 IMPL); D-S1 34-fuzzer count CONFIRMED at SPEC §11.1 (33 → 34); D-P5 18-arm byte-stable wording pinned at Task 9; vendored Rust-sourced `.wasm` bytecode under `bytecode/` + `scripts/` reproduction-source subdirectory per Q9 + AMEND-A1. | Task 17 |
| **ADR-0204** | proxy-wasm capability-restriction default-deny + envoy-go-strict sandbox posture — per parent §4.3 + AMEND-A5 + this 25.1 SPEC §3.3 + Task 6 D-P2 closure first-action (anticipated ungated for the 5 module-init/allocator callbacks). ~80-key capability roster materialized at `internal/wasm/sandbox.go` (37 core `proxy_*` + 7 ABI-versioned + 12-of-43 implemented WASI + 24 module-getter exports); 25.1 IMPL materializes the 24-hostcall subset + 13 module-getter callback subset. Denial semantic: `WasmResult::InternalFailure` (=10) + integration error log + `wasm.<plugin_name>.hostcall_denied` counter bump; WASI denials use `WasiErrno::ENOTCAPABLE` (=76) per D-P1 first-action scrape at Task 2 OR fallback `NOTSUP` (=58). `SanitizationConfig` accept-empty discipline per AMEND-A1 §11.4 (upstream's `SanitizationConfig` proto is EMPTY; accept empty value + ignore non-empty parse-and-discard). envoy-go-strict DEPARTURE recorded at BEHAVIOR_CONTRACT.md per parent §13.5 edit #3 (departure rationale: WASM exposes substantially larger and riskier hostcall surface than Lua; upstream's 3 sandbox runtimes V8/WAMR/Wasmtime marked `status: alpha` + `security_posture: unknown` at `source/extensions/extensions_metadata.yaml:1631-1635`; alpha-status incompatible with envoy-go's safe-by-default discipline). | Task 17 |

### CONDITIONAL ADR landing (only if R8 escape-valve fires per D-P-PLAN-10)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0205** (CONDITIONAL) | Per-module wazero Runtime pool with pre-instantiated entries — anchors only if Task 17 `BenchmarkPerStreamVM_Construction_Headers` reports `ns/op > 1_000_000` (= 1ms threshold per parent §13-R8 + this 25.1 SPEC §2.28 + this PLAN's D-P-PLAN-10). §Context + §Decision + §Consequences body all land at the same Task 17 commit per ADR-0044. If unconsumed: next-free ADR-0205 carries forward to 25.2 BRAINSTORM as the 25.2 IMPL escape-valve slot per parent §1.2. **Anticipated UNCONSUMED** per parent §1.2 hypothesis + phase-22.1 70µs analogous-benchmark precedent. | Task 17 (CONDITIONAL) |

The implementer at Task 17 AUTHORS the 3 ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the parent SPEC commit per ADR-0044), includes the ADRs in the Task 17 commit message, and verifies via `grep -nE '^## ADR-0202' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0203 + ADR-0204). If R8 escape-valve fires per D-P-PLAN-10, ADR-0205 §Context body also lands at the same commit.

**NO in-place ADR-0125 amendment at this PLAN commit + at Task 17** — per AMEND-A3 the 5th-canonical REUSE-by-absence is DEFINITIVE; ADR-0125 STAYS at 10 across all of phase 25; the AMENDMENT-anticipation paragraph that would land at 25.3 IMPL is REPLACED by ADR-0210 EXPLICIT-NO-NEW-CANONICAL classification at 25.3 (per parent §10.3).

**ADR-0044 escape-valve held in reserve per D-P-PLAN-10** — `ADR-0205` is the conditional escape-valve slot; the 25.1 SPEC's STRENGTHENED-WEAK-HOLD-with-1-slot-buffer per §1.2 + §13-R8 STANDS UNCHANGED at this PLAN commit. If at IMPL time a surface DOES warrant a new ADR beyond ADR-0205 (highly unlikely per the SPEC-time scrape closure of AMEND-A1..A9), it is ADR-0206 + the SPEC-anchored hypothesis is recorded as falsified in PROGRESS.md.

---

## Task graph (sequential vs parallelizable per D-P-PLAN-8)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task dependency graph:

- **Pre-Task 0** (PROGRESS.md preamble + 15-precondition verification) — sequential prerequisite for everything.
- **Task 1** (`internal/wasm/` skeleton + `doc.go` + `abi/types.go` + wazero v1.10.1 dep) — sequential prerequisite for Tasks 2-17.
- **Tasks 2, 3, 4** — **PARALLELIZABLE** (3-way) after Task 1; file-disjoint within `internal/wasm/`:
  - **Task 2** — `bytecode_util.go` byte-faithful ABI-version detection per AMEND-A6 + **D-P1 first-action** (WASI denial errno scrape).
  - **Task 3** — `pairs.go` byte-faithful pairs wire format per R3.
  - **Task 4** — `wasi.go` custom 8-stub WASI implementation per R4.
- **Tasks 5, 6** — **PARALLELIZABLE** (2-way) after Tasks 2+3+4; file-disjoint:
  - **Task 5** — `compile.go` `Module` + `CompileCache` + ABI-version gating (depends on Task 2 `bytecode_util.go`).
  - **Task 6** — `sandbox.go` default-deny capability roster + **D-P2 closure first-action** (module-init callback gating scrape).
- **Task 7** — `vm.go` + `registration.go` VM lifecycle + ABICallbacks interface + panic-wrapper (depends on Tasks 5 + 6 + the WASI shim from Task 4 + the pairs codec from Task 3).
- **Tasks 8, 9, 10** — **PARALLELIZABLE** (3-way) after Task 7; file-disjoint within `internal/filter/http/wasm/`:
  - **Task 8** — package skeleton + `doc.go` + `wasm.go` + `stats.go` (5-counter stat surface per AMEND-A2).
  - **Task 9** — `compiled_config.go` 18-arm PARSE-REJECT roster + **D-P5 closure** (byte-stable wording finalization).
  - **Task 10** — `datasource.go` 4-arm AsyncDataSource.Local resolution.
- **Task 11** — `abi_callbacks.go` ABICallbacks implementation + **D-P3 closure first-action** (ADR-0196 first co-consumer signature confirmation).
- **Task 12** — `decode_headers.go` + `encode_headers.go` (depends on Task 11).
- **Task 13** — Boot-registration at `cmd/envoy-go/main.go` alphabetical position per §3.6.
- **Tasks 14, 15** — **PARALLELIZABLE** (2-way) after Task 13; file-disjoint:
  - **Task 14** — 34th project-wide fuzzer `FuzzWasmConfigParse` + ~30 corpus seeds per D-P-PLAN-7.
  - **Task 15** — Differential fixture `0034-http-wasm-headers-bridge` (7 scenarios; full cross-side per parent §8.1 + §4.5 D6 guardrails) + NEW `BackendKind=HTTPWasm`.
- **Task 16** — Differential fixture `0035-http-wasm-boot-reject` + **D-P6 closure first-action** (boot-reject arm finalization). Sequential after Task 15 (runner_test.go blank-import conflict).
- **Task 17** — Benchmark + BEHAVIOR_CONTRACT.md 6-edit bundle + ADR-0202+0203+0204 §Decision+§Consequences body landing + STATE.md re-advance + ROADMAP row 25.1 flip + CONDITIONAL ADR-0205 if R8 fires per D-P-PLAN-10 + REVIEW.md authoring per `superpowers:requesting-code-review`. **D-P4 closure** at benchmark gate. Depends on everything.

**Parallel-dispatch opportunities**: 3-way at Tasks 2+3+4; 2-way at Tasks 5+6; 3-way at Tasks 8+9+10; 2-way at Tasks 14+15. **Sequential bottlenecks**: Pre-Task 0 → Task 1 → {2,3,4} → {5,6} → Task 7 → {8,9,10} → Task 11 → Task 12 → Task 13 → {14,15} → Task 16 → Task 17.

---

## Execution preconditions

Before Pre-Task 0 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl \
                 -b phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 15 preconditions verified at Pre-Task 0 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the phase-25.1-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-25.1-SPEC.md squash commit `b7fa3d7` + its SHA-fill follow-up `b924578` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server; `rustc --version` reports a recent stable Rust (for Task 15 fixture-0034 Rust source reproduction; pinned toolchain in `scripts/README.md`).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `204` (ADR-0204 — the highest ADR anchored as of master tip per the phase-25 parent SPEC + phase-25.1 SPEC commits). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0202' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0202 §Context already at parent SPEC commit `2c1455d` per ADR-0044). Same for ADR-0203 + ADR-0204. `grep -nE '^## ADR-0205' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0205 stays unconsumed UNLESS D-P-PLAN-10 R8 escape-valve fires at Task 17).
6. **ADR-0125 STAYS at 10 canonicals per AMEND-A3.** `grep -nE 'canonical|11th canonical' docs/envoy-go/DECISIONS.md` shows ADR-0125 body block with 10-canonical roster + no §(xvi) AMENDMENT-anticipation paragraph (per AMEND-A3 — REUSE-by-absence is DEFINITIVE; NO amendment at 25.3 IMPL).
7. **NO 25.2/25.3-bound code at this 25.1 worktree.** Per BOOTSTRAP §4.1 invariant 2 — phase-25.2 surfaces (body+buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full stream-info) + phase-25.3 surfaces (per-route 5th-canonical + multi-plugin VM-sharing + conformance harness seed) MUST NOT land at 25.1 IMPL. If any 25.2/25.3-surface partial implementation has been started, halt + escalate to user.
8. **Parent SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` returns `2c1455d` (or descendant). If different, re-read parent SPEC.
9. **25.1 SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/SPEC.md` returns `b7fa3d7` (or descendant). If different, re-read 25.1 SPEC.
10. **25.1 PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
11. **Pristine tree.** `git status --porcelain` returns empty.
12. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
13. **Pre-existing differential suite green.** `ls -d test/fixtures/00*/ | wc -l` returns the pre-existing fixture-dir count (next-prompt.txt at master tip records `35`; numeric range `0000-0033` plus any `00NNa`/`00NNb` sub-fixture pairs from prior phases — re-verify the exact count at Pre-Task 0 + record in PROGRESS preamble). `go test -count=1 ./test/differential/...` returns every pre-existing fixture PASS — the regression baseline. Phase 25.1 adds the NEXT `BackendKind` enum value + 2 new fixture directories (`0034-http-wasm-headers-bridge` per Task 15 + `0035-http-wasm-boot-reject` per Task 16); post-25.1 dir count = pre-existing + 2.
14. **Pre-existing fuzzers run clean at 30s.** The 33 fuzzers from phases 02-24 run clean. Phase 25.1 adds the 34th (`FuzzWasmConfigParse` per Task 14). Quick smoke: `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `33`.
15. **Pre-existing `internal/wasm/` + `internal/filter/http/wasm/` directories + `test/fixtures/0034-http-wasm-headers-bridge/` + `test/fixtures/0035-http-wasm-boot-reject/` directories + `BackendKind=HTTPWasm` enum value do NOT exist.** `test ! -d internal/wasm && test ! -d internal/filter/http/wasm && test ! -d test/fixtures/0034-http-wasm-headers-bridge && test ! -d test/fixtures/0035-http-wasm-boot-reject && ! grep -q 'HTTPWasm' test/differential/fixture/fixture.go && echo "ok: phase-25.1-new-surfaces absent"` returns success.

If all 15 preconditions pass, proceed to Pre-Task 0 (PROGRESS.md preamble) + Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 15-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md`

This pre-task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0202 + ADR-0203 + ADR-0204 §Context drafts are at the parent SPEC commit `2c1455d`; ADR-0205 is CONDITIONAL (PLAN hypothesis per D-P-PLAN-10: it does NOT fire unless Task 17 benchmark surfaces > 1ms threshold). The PROGRESS preamble ANTICIPATES the 3 NEW ADR landings at Task 17 + records the 10 PLAN-time decisions D-P-PLAN-1..D-P-PLAN-10 + records the 6 SPEC-time D-question anticipated dispositions D-P1..D-P6.

Pre-Task 0 is NOT a SPEC §6 numbered task — the SPEC §6 17-task breakdown begins at Task 1. Per D-P-PLAN-1, the SPEC §6 numbering is inherited verbatim; PROGRESS.md preamble + precondition verification is the ritual prefix.

**Precondition:** worktree exists at `phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl`; branch base is master tip after the 25.1 PLAN.md SHA-fill follow-up; all 15 preconditions report green.
**Artifact:** `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` returns the Pre-Task 0 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` with: (a) Preamble summarizing the 15-precondition verification (verbatim command outputs captured); (b) the 3-NEW-ADR table + CONDITIONAL ADR-0205 row from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 10 PLAN-time decisions D-P-PLAN-1..D-P-PLAN-10 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) the 6 SPEC-time D-question anticipated dispositions D-P1..D-P6 from 25.1 SPEC §12; (e) a Pre-Task 0 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "phase 25.1 Pre-Task 0: PROGRESS.md preamble + 15-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
# expect: a 40-char SHA (Pre-Task 0 commit)
```

---

## Tier A — Framework primitive (`internal/wasm/`) (Tasks 1-7)

## Task 1: NEW `internal/wasm/` package skeleton + `doc.go` + `abi/types.go` + go.mod wazero dep

**Files:**
- Create: `internal/wasm/doc.go` (~80-130 LoC)
- Create: `internal/wasm/abi/types.go` (~150-220 LoC)
- Create: `internal/wasm/abi/types_test.go` (~150-220 LoC)
- Modify: `go.mod` + `go.sum` — add `github.com/tetratelabs/wazero v1.10.1` direct dep per AMEND-A1
- Append: `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md` (Task 1 entry per D-P-PLAN-3)

This task lands the `internal/wasm/` package directory + the wazero dep + the proxy-wasm v0.2.1 ABI type enumerations. Per 25.1 SPEC §6 Task 1: wazero v1.10.1 added as direct dep; `go.mod` Go-floor STAYS at `go 1.23.0`; `abi/types.go` defines all required enums with byte-faithful value-gap preservation per AMEND-A7; `doc.go` covers Q1-Q9 BRAINSTORM decisions + AMEND-A1..A9 cross-refs + the API surface summary; build clean; `abi/types_test.go` round-trip tests pass. **Sequential prerequisite for Tasks 2-17.**

**Precondition:** Pre-Task 0 complete; all 15 preconditions green; wazero v1.10.1 is the pinned version per AMEND-A1.
**Artifact:** `internal/wasm/` package directory with `doc.go` + `abi/types.go`; wazero dep added; build + tests pass.
**Acceptance:** `go build ./internal/wasm/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/wasm/...` clean; `go test -count=1 ./internal/wasm/abi/...` passes (value-faithful + value-gap preservation tests); `go mod tidy` clean (no orphaned modules); `go.sum` includes wazero entries; `go.mod` declares `go 1.23.0` floor per wazero requirement.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author the Task 1 files at the 3 listed paths per the 25.1 PLAN Task 1 + 25.1 SPEC §3.1 + §3.2. The package directory is NEW (both `internal/wasm/` and `internal/wasm/abi/`). Add `github.com/tetratelabs/wazero v1.10.1` to go.mod as a direct dep; run `go mod tidy`. The `abi/types.go` defines `WasmResult` (10 named values with VALUE-GAPS at 5/9/11 — see SPEC §1 architectural primitive 1 for the full enum), `WasmBufferType` (9 values; value 8 = `FOREIGN_FUNCTION_ARGUMENTS` per AMEND-A7 — NOT `CallData`), `WasmHeaderMapType` (8 values), `LogLevel` (6 values), `ProxyAction` (2 values; CONTINUE=0, PAUSE=1), `WasiErrno` subset roster (`BADF=8`, `NOTSUP=58`, `ENOTCAPABLE=76`). The value-gap preservation is CRITICAL — guest modules check specific integer values. Tests in `abi/types_test.go` assert each enum value byte-exactly. The `doc.go` covers package overview + Q1-Q9 + AMEND-A1..A9 cross-refs + the API surface summary (`VM`, `VMOption`, `NewVM`, `Module`, `CompileCache`, `SandboxConfig`, etc.). Commit per Step 6 message template. Quote `go build`, `go vet`, `golangci-lint`, `go test`, `go mod tidy` outputs verbatim in PROGRESS.md Task 1 entry per D-P-PLAN-3.

- [ ] **Step 1: Add wazero dep to go.mod**

```bash
go get github.com/tetratelabs/wazero@v1.10.1
go mod tidy
# verify go.mod includes:  require github.com/tetratelabs/wazero v1.10.1
# verify go.mod has:       go 1.23.0  (or higher; STAYS at the wazero floor)
```

- [ ] **Step 2: Author `internal/wasm/abi/types.go` + `internal/wasm/abi/types_test.go`** per 25.1 SPEC §3.2 + AMEND-A7 value-faithful encoding.

- [ ] **Step 3: Run the value-faithful tests**

```bash
go test -count=1 -v ./internal/wasm/abi/...
# Expected: PASS (all WasmResult/WasmBufferType/etc. value assertions pass; value-gap preservation verified)
```

- [ ] **Step 4: Author `internal/wasm/doc.go`** per 25.1 SPEC §3.2 + the API surface summary from §3.1.

- [ ] **Step 5: Verify build + lint clean**

```bash
go build ./internal/wasm/...
go vet ./...
golangci-lint run ./internal/wasm/...
# Expected: each clean (no diagnostics)
```

- [ ] **Step 6: Append PROGRESS.md Task 1 entry + commit**

```bash
git add internal/wasm/ go.mod go.sum docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): bootstrap framework primitive — doc.go + abi/types.go + wazero v1.10.1 dep

Phase 25.1 Task 1 (Tier A framework primitive). NEW internal/wasm/ package +
abi/ subdirectory. Adds tetratelabs/wazero v1.10.1 (Apache-2.0 per AMEND-A1
correcting BRAINSTORM 'MIT-licensed' typo). Go-floor STAYS at 1.23.0 per
wazero's floor. abi/types.go defines WasmResult (10 named values with
value-gaps at 5/9/11 per AMEND-A7) + WasmBufferType (9 values; value 8 =
FOREIGN_FUNCTION_ARGUMENTS per AMEND-A7) + WasmHeaderMapType + LogLevel +
ProxyAction + WasiErrno subset. Value-faithful encoding tests pass (value-gap
preservation verified)."
```

## Task 2: `internal/wasm/bytecode_util.go` byte-faithful ABI-version detection per AMEND-A6 + D-P1 first-action

**Files:**
- Create: `internal/wasm/bytecode_util.go` (~200-300 LoC)
- Create: `internal/wasm/bytecode_util_test.go` (~250-380 LoC)
- Append: PROGRESS.md (Task 2 entry per D-P-PLAN-3 + **D-P1 closure evidence**)

This task lands the byte-faithful ABI-version detection per AMEND-A6 (transcribed from `proxy-wasm-cpp-host:bytecode_util.cc:32-97`) + closes D-P1 (WASI denial errno disposition). The detection scans the wasm module export section (type 7) for function-kind exports named `proxy_abi_version_0_2_1` / `proxy_abi_version_0_2_0` / `proxy_abi_version_0_1_0`. **D-P1 first-action**: scrape upstream `proxy-wasm-cpp-host/include/proxy-wasm/proxy_wasm_exports.h` lines 232-249 against the WASI denial errno semantic; record the chosen errno (`NOTSUP=58` or `ENOTCAPABLE=76`) in this task's PROGRESS.md entry + use the chosen value in §3.3 sandbox.go WASI denial path (consumed at Task 6 + Task 4 wasi.go).

**Precondition:** Task 1 complete; wazero v1.10.1 dep available; abi/types.go provides `WasmResult`, `WasiErrno` enums.
**Artifact:** `internal/wasm/bytecode_util.go` + test file; ABI-version detection works on crafted-wasm binaries; D-P1 closure evidence recorded.
**Acceptance:** `go test -count=1 -v ./internal/wasm/...` passes (all crafted-wasm fixture variants assert expected AbiVersion); `golangci-lint run ./internal/wasm/...` clean; D-P1 closure evidence quoted in PROGRESS.md entry + the chosen errno value referenced for consumption at Task 6 sandbox.go.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-P1 closure): fetch + scrape `proxy-wasm-cpp-host@da3ce05d:include/proxy-wasm/proxy_wasm_exports.h:232-249` to determine the WASI denial errno semantic. Anticipated answer per 25.1 SPEC §12 D-P1: mirror upstream `ENOTCAPABLE=76` for byte-faithfulness; if wazero's WASI semantics prevent the exact return code, fall back to `NOTSUP=58` + record a sub-pin envoy-go-strict departure. Record the empirical evidence (quoted source lines) + chosen errno in PROGRESS.md Task 2 entry. Then implement `bytecode_util.go` byte-faithfully per AMEND-A6 transcribing `proxy-wasm-cpp-host:bytecode_util.cc:32-97`. Tests at `bytecode_util_test.go` cover ~5-10 crafted-wasm-binary fixtures via runtime synthesis (avoid vendored fixtures). Each fixture asserts `AbiVersion` + error disposition byte-exactly. Commit per Step 5 template. Quote test outputs + D-P1 evidence in PROGRESS.md.

- [ ] **Step 1: D-P1 first-action scrape** — fetch `proxy-wasm-cpp-host@da3ce05d:include/proxy-wasm/proxy_wasm_exports.h:232-249`; record empirical errno semantic in scratch notes for PROGRESS.md entry.

- [ ] **Step 2: Write failing tests** at `internal/wasm/bytecode_util_test.go`

```bash
# Author tests covering: valid v0.2.1 fixture, valid v0.1.0, valid v0.2.0, missing sentinel,
# malformed export section, truncated module. Each asserts AbiVersion + error disposition.
go test -count=1 -v ./internal/wasm/ -run TestGetAbiVersion
# Expected: FAIL (function not yet defined)
```

- [ ] **Step 3: Implement `internal/wasm/bytecode_util.go`** per AMEND-A6 transcription.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run TestGetAbiVersion
# Expected: PASS (all crafted-wasm fixtures detected correctly)
go vet ./...
golangci-lint run ./internal/wasm/...
# Expected: each clean
```

- [ ] **Step 5: Append PROGRESS.md Task 2 entry with D-P1 closure evidence + commit**

```bash
git add internal/wasm/bytecode_util.go internal/wasm/bytecode_util_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): byte-faithful bytecode_util ABI-version detection per AMEND-A6 + D-P1 errno pin

Phase 25.1 Task 2 (Tier A framework primitive). Byte-faithful reimplementation
of proxy-wasm-cpp-host:bytecode_util.cc:32-97 scanning wasm module export
section type 7 for proxy_abi_version_0_2_1 sentinel per AMEND-A6. Crafted-wasm
fixture tests cover valid v0.2.1 + v0.1.0 + v0.2.0 + missing-sentinel +
malformed-section + truncated-module. D-P1 CLOSED: WASI denial errno
<NOTSUP=58 | ENOTCAPABLE=76> chosen per upstream proxy_wasm_exports.h:232-249
scrape evidence; consumed at Task 4 wasi.go + Task 6 sandbox.go."
```

---

## Task 3: `internal/wasm/pairs.go` byte-faithful pairs wire format per R3

**Files:**
- Create: `internal/wasm/pairs.go` (~120-180 LoC)
- Create: `internal/wasm/pairs_test.go` (~200-300 LoC)
- Append: PROGRESS.md (Task 3 entry per D-P-PLAN-3)

This task lands the byte-faithful pairs wire format per R3 + parent §13-R3 — transcribed from `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc`. Wire format: `u32 num_pairs / u32 key_len, u32 value_len (repeated num_pairs times) / key_bytes NUL value_bytes NUL (repeated num_pairs times)` little-endian. `HeaderPair struct { Key, Value string }`; `EncodePairs([]HeaderPair) []byte`; `DecodePairs([]byte) ([]HeaderPair, error)`. Errors on truncated buffer + pair-count overflow + key-len/value-len overflow + missing NUL.

**Precondition:** Task 1 complete (`encoding/binary` stdlib available; no other internal/wasm/ deps).
**Artifact:** `internal/wasm/pairs.go` + test file; byte-faithful round-trip vs cpp-host oracle.
**Acceptance:** `go test -count=1 -v ./internal/wasm/ -run TestPairs` passes (golden-bytes + round-trip + malformed-input tests); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `pairs.go` byte-faithfully per R3 transcribing `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc`. Wire format: `u32 num_pairs / u32 key_len + u32 value_len (repeated num_pairs times) / key_bytes NUL value_bytes NUL (repeated num_pairs times)` little-endian via `encoding/binary.LittleEndian`. Tests at `pairs_test.go` use golden-bytes table-driven approach: each row is `{name, pairs []HeaderPair, wantBytes []byte}`. Round-trip tests: `DecodePairs(EncodePairs(x)) == x`. Malformed: truncated header, oversize length, missing NUL, out-of-bounds offsets — assert wrapped errors.

- [ ] **Step 1: Write failing tests** at `internal/wasm/pairs_test.go`

```bash
go test -count=1 -v ./internal/wasm/ -run TestPairs
# Expected: FAIL (function not yet defined)
```

- [ ] **Step 2: Implement `internal/wasm/pairs.go`** per R3 transcription.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run TestPairs
# Expected: PASS (golden-bytes + round-trip + malformed-input tests)
go vet ./... && golangci-lint run ./internal/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 3 entry + commit**

```bash
git add internal/wasm/pairs.go internal/wasm/pairs_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): byte-faithful pairs wire format per R3

Phase 25.1 Task 3 (Tier A framework primitive). Byte-faithful reimplementation
of proxy-wasm-cpp-host:pairs_util.h + pairs_util.cc per R3. Wire format u32
num_pairs / u32 key_len, u32 value_len * N / key_bytes NUL value_bytes NUL * N
little-endian. Golden-bytes table-driven tests + round-trip + malformed-input
coverage."
```

---

## Task 4: `internal/wasm/wasi.go` custom 8-stub WASI implementation per R4

**Files:**
- Create: `internal/wasm/wasi.go` (~250-350 LoC)
- Create: `internal/wasm/wasi_test.go` (~250-380 LoC)
- Append: PROGRESS.md (Task 4 entry per D-P-PLAN-3; reference D-P1 errno pin from Task 2)

This task lands the custom 8-stub WASI implementation per R4 + parent §13-R4. Do NOT use wazero's built-in `imports/wasi_snapshot_preview1` package — it routes `fd_write` to host stdout/stderr; we need it routed through `proxy_log` per the proxy-wasm semantics. 8 shims: `fd_write` (fd=1 → INFO, fd=2 → ERROR, other → `WasiErrno::BADF`=8); `clock_time_get` (host-accuracy CLOCK_REALTIME=0 wall + CLOCK_MONOTONIC=1 monotonic); `random_get` (`crypto/rand.Read`); `environ_sizes_get` (0/0); `environ_get` (writes nothing); `args_sizes_get` (0/0); `args_get` (writes nothing); `proc_exit` (traps via `sys.NewExitError(exit_code)`). Sandbox capability gate applied; denial errno per D-P1 (anticipated `ENOTCAPABLE=76`).

**Precondition:** Tasks 1 + 2 complete; D-P1 errno disposition from Task 2 PROGRESS.md available.
**Artifact:** `internal/wasm/wasi.go` + test file; 8 shims implement proxy-wasm semantics.
**Acceptance:** `go test -count=1 -v ./internal/wasm/ -run TestWasi` passes; `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `wasi.go` per R4 + parent §13-R4 + 25.1 SPEC §5.2. The 8 shims implement proxy-wasm semantics: `fd_write` routes fd=1 → `proxy_log(INFO, msg)`, fd=2 → `proxy_log(ERROR, msg)`, other → `WasiErrno::BADF=8`. Use D-P1 errno from Task 2 (`ENOTCAPABLE=76` anticipated; or `NOTSUP=58` fallback) for sandbox-denial path. Tests verify each shim's golden semantics + bad-fd/bad-arg paths + sandbox-deny path.

- [ ] **Step 1: Write failing tests** at `internal/wasm/wasi_test.go`

```bash
go test -count=1 -v ./internal/wasm/ -run TestWasi
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement `internal/wasm/wasi.go`** per R4 + D-P1 errno.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run TestWasi
# Expected: PASS
go vet ./... && golangci-lint run ./internal/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 4 entry + commit**

```bash
git add internal/wasm/wasi.go internal/wasm/wasi_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): custom 8-stub WASI implementation per R4

Phase 25.1 Task 4 (Tier A framework primitive). Custom 8-stub WASI per R4 +
parent §13-R4 — do NOT use wazero's built-in imports/wasi_snapshot_preview1.
fd_write routes to proxy_log (fd=1 INFO, fd=2 ERROR, other BADF=8);
clock_time_get host-accuracy; random_get via crypto/rand; environ_*+args_*
return 0/0; proc_exit traps. Sandbox-deny path uses D-P1 errno from Task 2
PROGRESS evidence."
```

---

## Task 5: `internal/wasm/compile.go` Module + CompileCache + ABI-version gating

**Files:**
- Create: `internal/wasm/compile.go` (~150-220 LoC)
- Create: `internal/wasm/compile_test.go` (~200-300 LoC)
- Append: PROGRESS.md (Task 5 entry per D-P-PLAN-3)

This task lands the `Module` + `CompileCache` types + the `CompileModule` function + ABI-version gating. `Module` wraps `wazero.CompiledModule` + `AbiVersion` + sha256 hash. `CompileCache` is content-addressed with `sync.RWMutex`; nil-tolerant per ADR-0085. `CompileModule(ctx, src, cache)` compiles via wazero's parser + gates via `bytecode_util.GetAbiVersion` (`AbiVersionUnknown` / `AbiVersion_0_2_0` / `AbiVersion_0_1_0` → `ErrUnsupportedAbiVersion` per AMEND-A6).

**Precondition:** Tasks 1 + 2 complete; wazero dep available; `bytecode_util.GetAbiVersion` from Task 2 + `AbiVersion` enum.
**Artifact:** `internal/wasm/compile.go` + test file; cache-hit + cache-miss + nil-cache + concurrent + ABI-gating tests pass.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/ -run TestCompile` passes (cache-hit-on-same-content-hash + cache-miss + nil-cache + concurrent read/add + ABI-version gating + compile-error path); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `compile.go` per 25.1 SPEC §3.1. `Module struct { compiled wazero.CompiledModule; abiVer AbiVersion; hash [32]byte }`. `CompileCache struct { mu sync.RWMutex; store map[[32]byte]*Module; rt wazero.Runtime }` — owns a shared compile-only `wazero.Runtime` (separate from per-stream runtimes). `NewCompileCache(ctx)` constructs cache + initializes compile-runtime. `CompileModule(ctx, src, cache)`: computes `sha256(src)` → RLock cache lookup → miss → compile via `cache.rt.CompileModule(ctx, src)` → `bytecode_util.GetAbiVersion(src)` → if not `AbiVersion_0_2_1` → return `ErrUnsupportedAbiVersion` (per AMEND-A6); else Lock cache add → return Module. `cache == nil` → compile uncached. `cache.Close()` releases compile-runtime (idempotent). Tests cover all paths with `-race` clean.

- [ ] **Step 1: Write failing tests** at `internal/wasm/compile_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestCompile
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement `internal/wasm/compile.go`** per §3.1.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -race -v ./internal/wasm/ -run TestCompile
# Expected: PASS
go vet ./... && golangci-lint run ./internal/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 5 entry + commit**

```bash
git add internal/wasm/compile.go internal/wasm/compile_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): Module + CompileCache + ABI-version gating per AMEND-A6

Phase 25.1 Task 5 (Tier A framework primitive). Module wraps
wazero.CompiledModule + detected AbiVersion + sha256(src). CompileCache
content-addressed with sync.RWMutex; nil-tolerant per ADR-0085. CompileModule
gates via bytecode_util.GetAbiVersion — only AbiVersion_0_2_1 accepted per
AMEND-A6 envoy-go-strict-stricter; v0.1.0/v0.2.0/missing-sentinel return
ErrUnsupportedAbiVersion (PARSE-REJECT arm 16 wording at compiled_config.go
Task 9). Concurrent read/add race-clean."
```

---

## Task 6: `internal/wasm/sandbox.go` default-deny capability roster + D-P2 closure

**Files:**
- Create: `internal/wasm/sandbox.go` (~250-380 LoC)
- Create: `internal/wasm/sandbox_test.go` (~300-450 LoC)
- Append: PROGRESS.md (Task 6 entry per D-P-PLAN-3 + **D-P2 closure evidence**)

This task lands the default-deny capability sandbox per AMEND-A5 + ADR-0204. `SandboxConfig` zero-value posture = `StrictDefaultDeny` — empty `AllowedCapabilities` map ⇒ DENY ALL hostcalls (INVERTS upstream's empty-map-allow-all semantic). Package-private capability-key constants for the 25.1 surface (24 hostcalls + 13 module-getter/callback keys). `SanitizationConfig` empty struct (accept-empty-as-no-op per AMEND-A1 §11.4 + parent §4.3.5). **D-P2 closure first-action**: scrape `proxy-wasm-cpp-host:wasm.cc:298-302` getFunction discipline to confirm whether the 5 module-init/allocator callbacks (`_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate`) participate in capability gating; record the disposition in this task's PROGRESS.md entry (anticipated: ungated — they are required for instantiation; gating them would break every module).

**Precondition:** Task 1 complete (abi/types.go provides type references); Tasks 2 + 4 ideally complete for D-P1 errno consumption in WASI denial path.
**Artifact:** `internal/wasm/sandbox.go` + test file; D-P2 closure evidence recorded.
**Acceptance:** `go test -count=1 -v ./internal/wasm/ -run TestSandbox` passes (per-capability ALLOW/DENY exhaustive + D-P2 module-init ungating verification + SanitizationConfig accept-empty); `golangci-lint run ./internal/wasm/...` clean; D-P2 closure evidence quoted in PROGRESS.md.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-P2 closure): fetch + scrape `proxy-wasm-cpp-host@da3ce05d:src/wasm.cc:298-302` to determine whether the 5 module-init/allocator callbacks participate in capability gating. Anticipated answer per 25.1 SPEC §12 D-P2: ungated — module-init callbacks are required for instantiation; gating them would break every module. Record empirical evidence + disposition in PROGRESS.md Task 6 entry. Then implement `sandbox.go` per 25.1 SPEC §3.3. `SandboxConfig.IsAllowed(name)` returns `_, ok := sb.AllowedCapabilities[name]; return ok` — empty map → DENY ALL per AMEND-A5. Package-private capability-key constants per §3.3 (7 headers-bridge family + 1 local-response + 2 property + 2 log + 1 status + 1 time + 2 context-lifecycle + 8 WASI bare-names + 5 module-init/allocator [ungated per D-P2] + 8 lifecycle/HTTP module-getter capability keys). Tests verify empty map denies all + populated map allows only named keys + SanitizationConfig non-empty parse-and-discard + D-P2 module-init NOT capability-gated.

- [ ] **Step 1: D-P2 first-action scrape** — fetch `proxy-wasm-cpp-host@da3ce05d:src/wasm.cc:298-302`; record empirical module-init callback gating disposition for PROGRESS.md.

- [ ] **Step 2: Write failing tests** at `internal/wasm/sandbox_test.go`

```bash
go test -count=1 -v ./internal/wasm/ -run TestSandbox
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 3: Implement `internal/wasm/sandbox.go`** per §3.3 + D-P2 disposition.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/wasm/ -run TestSandbox
# Expected: PASS (per-capability ALLOW/DENY exhaustive + D-P2 verification)
go vet ./... && golangci-lint run ./internal/wasm/...
# Expected: clean
```

- [ ] **Step 5: Append PROGRESS.md Task 6 entry with D-P2 closure evidence + commit**

```bash
git add internal/wasm/sandbox.go internal/wasm/sandbox_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): default-deny capability sandbox per AMEND-A5 + D-P2 closure

Phase 25.1 Task 6 (Tier A framework primitive). SandboxConfig zero-value =
StrictDefaultDeny per AMEND-A5 — empty AllowedCapabilities ⇒ DENY ALL
hostcalls (INVERTS upstream's empty-map-allow-all semantic). Package-private
capability-key constants for 25.1 surface (24 hostcalls + 13 callback/module-
getter keys). SanitizationConfig accept-empty per AMEND-A1 §11.4. D-P2 CLOSED:
5 module-init/allocator callbacks (_initialize, _start, main, malloc,
proxy_on_memory_allocate) <ungated | gated> per proxy-wasm-cpp-host:wasm.cc:
298-302 scrape evidence."
```

---

## Task 7: `internal/wasm/vm.go` + `registration.go` VM lifecycle + ABICallbacks interface + panic-wrapper

**Files:**
- Create: `internal/wasm/vm.go` (~450-650 LoC)
- Create: `internal/wasm/registration.go` (~350-550 LoC)
- Create: `internal/wasm/vm_test.go` (~300-450 LoC)
- Create: `internal/wasm/registration_test.go` (~250-380 LoC)
- Append: PROGRESS.md (Task 7 entry per D-P-PLAN-3)

This task lands the VM lifecycle + ABICallbacks interface + panic-wrapper + host-module wiring. Depends on Tasks 2 (bytecode_util — AbiVersion enum), 3 (pairs — HeaderPair), 4 (wasi — WASI shims), 5 (compile — Module type), 6 (sandbox — SandboxConfig + IsAllowed). The `VM` type per 25.1 SPEC §3.1 production signatures with `VMOption` function-option pattern; `NewVM(ctx, opts...)` constructs `wazero.Runtime` (interpreter-default per parent §2.7) + registers env-namespace + wasi_snapshot_preview1-namespace host modules; `Run(ctx, module, rootContextID)` executes module-init lifecycle (`_initialize` OR `_start` + `proxy_on_vm_start` + `proxy_on_configure`); per-callback methods invoke guest exports via `wazero.Module.ExportedFunction("proxy_on_X").Call(ctx, args...)`. `ABICallbacks` interface at `registration.go` carries the 13-method headers-bridge subset. Panic-wrapper invokes `WithPanicHandler` after `recover()` (NOT for catching wazero-side traps which return via Call's error path).

**Precondition:** Tasks 2, 3, 4, 5, 6 complete.
**Artifact:** `internal/wasm/vm.go` + `registration.go` + 2 test files; per-stream VM round-trip works; sandbox-deny dispatches correctly; panic-wrapper behaves; concurrent VMs share no state.
**Acceptance:** `go test -count=1 -race -v ./internal/wasm/...` passes (per-stream construction round-trip; sandbox-deny dispatch for each capability key; panic-wrapper behavior; concurrent VMs race-clean; Close idempotent); `golangci-lint run ./internal/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `vm.go` + `registration.go` per 25.1 SPEC §3.1 production signatures + §5 hostcall/callback surface enumeration. `VM` struct with unexported fields (runtime, module, instance, sandbox, panicH, logSink, cb, ctxStore). `VMOption func(*VM)`; `WithSandboxConfig`/`WithPanicHandler`/`WithLogSink`. `NewVM(ctx, opts...)` constructs `wazero.NewRuntime(ctx)` interpreter-default + calls `registerHostModules(rt, vm)` from registration.go. `State() wazero.Runtime` escape-hatch. `RegisterABICallbacks(cb ABICallbacks)`. `Run(ctx, module, rootContextID)` instantiates `*wazero.CompiledModule` onto runtime + executes `_initialize` OR `_start` (guest exports exactly one) + `proxy_on_vm_start(rootContextID, vmConfigSize)` + `proxy_on_configure(rootContextID, pluginConfigSize)`. `HasGlobalFunc(name)` checks `vm.instance.ExportedFunction(name) != nil`. Per-callback methods (`CallProxyOnContextCreate`/`CallProxyOnRequestHeaders`/`CallProxyOnResponseHeaders`/`CallProxyOnDone`/`CallProxyOnLog`/`CallProxyOnDelete`) wrap `ExportedFunction(...).Call(ctx, args...)` with the panic-wrapper. `Close()` idempotent. `registration.go` defines `ABICallbacks` interface (24-method headers-bridge subset per §5.1+§5.2) + `HeaderPair` re-export + `registerHostModules(rt, vm)` which registers the 16 active proxy_* env-namespace hostcalls + the 8 active wasi_snapshot_preview1.* custom-shim hostcalls + the 23 deferred-25.2/25.3 stub-Unimplemented hostcalls per parent §4.2 Option B. Each hostcall body reads `vm.sandbox.IsAllowed(capabilityName)` before invoking ABICallbacks; denied → `WasmResult::InternalFailure=10` + log + signal hostcall_denied counter bump (counter lives at filter package; the framework primitive emits the signal via the configured log sink + the filter's integration hook reads the failures from VM error returns). Tests cover per-stream round-trip, sandbox-deny for each capability key, panic-wrapper Go-panic in ABICallbacks Go method, concurrent VMs share no state, Close idempotent.

- [ ] **Step 1: Write failing tests** at `internal/wasm/vm_test.go` + `registration_test.go`

```bash
go test -count=1 -race -v ./internal/wasm/ -run 'TestVM|TestRegistration'
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement `internal/wasm/vm.go` + `registration.go`** per §3.1 + §5.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -race -v ./internal/wasm/
# Expected: PASS (all internal/wasm/ tests)
go vet ./... && golangci-lint run ./internal/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 7 entry + commit**

```bash
git add internal/wasm/vm.go internal/wasm/registration.go internal/wasm/vm_test.go internal/wasm/registration_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/wasm): VM lifecycle + ABICallbacks interface + panic-wrapper

Phase 25.1 Task 7 (Tier A framework primitive). VM type per 25.1 SPEC §3.1
production signatures with VMOption function-option pattern (WithSandboxConfig
/ WithPanicHandler / WithLogSink). NewVM constructs wazero.Runtime
interpreter-default per parent §2.7 + registers env-namespace + wasi_*-
namespace host modules (24 active hostcalls + 23 deferred stubs per parent
§4.2 Option B). Per-callback methods (CallProxyOnContextCreate /
CallProxyOnRequestHeaders / CallProxyOnResponseHeaders / CallProxyOnDone /
CallProxyOnLog / CallProxyOnDelete) wrap guest-export calls with panic-wrapper.
ABICallbacks interface carries 13-method headers-bridge subset. Sandbox gate
applied per hostcall via vm.sandbox.IsAllowed. Concurrent VMs race-clean."
```

---

## Tier B — Filter package (`internal/filter/http/wasm/`) (Tasks 8-13)

## Task 8: NEW `internal/filter/http/wasm/` package skeleton + `doc.go` + `wasm.go` + `stats.go`

**Files:**
- Create: `internal/filter/http/wasm/doc.go` (~60-100 LoC)
- Create: `internal/filter/http/wasm/wasm.go` (skeleton ~120 LoC; filter struct + TypeURL + filterName + `New` factory stub returning sentinel error; full body wiring extends across Tasks 11+12)
- Create: `internal/filter/http/wasm/stats.go` (~80-120 LoC)
- Create: `internal/filter/http/wasm/wasm_test.go` (skeleton tests; stat-name table-driven verification)
- Append: PROGRESS.md (Task 8 entry per D-P-PLAN-3)

This task lands the filter package skeleton + the 5-counter stat surface per AMEND-A2. `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"`; `filterName = "envoy.filters.http.wasm"`; per-route validator registration. `filterStats` per AMEND-A2 tri-group prefix structure (Group B `wasm.wazero.{created,active}` + envoy-go-strict `wasm.<plugin_name>.{executions, hostcall_denied, envoy_go.failures}`); HCM-stats_prefix DROPPED. `New(ctx)` returns sentinel error at skeleton stage; full parse lands at Task 9.

**Precondition:** Task 7 complete (internal/wasm/ package available).
**Artifact:** `internal/filter/http/wasm/` package directory with skeleton files; 5-counter stat surface registered; stat-name byte-exact tests pass.
**Acceptance:** `go build ./internal/filter/http/wasm/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/wasm/...` clean; `go test -count=1 ./internal/filter/http/wasm/...` skeleton tests pass; 5-counter stat surface registered byte-exact (project total 114 → 119 verified via `TestStatNames_Equal_*` table-driven).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author Task 8 skeleton files per 25.1 SPEC §3.5 + §4. Package directory NEW (`internal/filter/http/wasm/`). `wasm.go` skeleton: filter struct + TypeURL + filterName + `var _ StreamDecoderFilter = (*filter)(nil)` + `var _ StreamEncoderFilter = (*filter)(nil)` compile-time assertions + `New(ctx)` stub returning sentinel error (real parse at Task 9) + per-route validator registration via `reg.RegisterPerRouteValidator(filterName, validatePerRouteWasm)` where `validatePerRouteWasm` is a one-liner returning `errors.New("wasm: per-route configuration is not yet supported (lands in phase 25.3)")` per ADR-0110 single-chokepoint discipline. `stats.go` per AMEND-A2 tri-group structure: `filterStats struct { created *stats.Counter; active *stats.Gauge; executions *stats.Counter; hostcallDenied *stats.Counter; envoyGoFailures *stats.Counter }`. `newFilterStats(reg, pluginName)` constructs counters under tri-group prefix (`wasm.wazero.{created,active}` + `wasm.<plugin_name>.{executions,hostcall_denied,envoy_go.failures}`); HCM-stats_prefix DROPPED. `doc.go` covers package overview + Q1-Q9 + AMEND-A1..A9 + D-P1..D-P6 + ADR-0203. Skeleton tests in wasm_test.go: TypeURL constant assertion; stat-name table-driven byte-exact verification (`TestStatNames_Equal_Wazero_Created`, etc.).

- [ ] **Step 1: Write skeleton failing tests** at `internal/filter/http/wasm/wasm_test.go`

```bash
go test -count=1 -v ./internal/filter/http/wasm/
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement skeleton `wasm.go` + `stats.go` + `doc.go`** per §3.5 + §4 + AMEND-A2.

- [ ] **Step 3: Run skeleton tests to verify they pass**

```bash
go test -count=1 -v ./internal/filter/http/wasm/
# Expected: PASS (skeleton tests pass)
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 8 entry + commit**

```bash
git add internal/filter/http/wasm/ docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): package skeleton + 5-counter stat surface per AMEND-A2

Phase 25.1 Task 8 (Tier B filter package). NEW internal/filter/http/wasm/
package skeleton + TypeURL + filterName + per-route validator stub
(arm-18 PARSE-REJECT) + New() factory stub returning sentinel error (real
parse at Task 9). filterStats per AMEND-A2 tri-group prefix structure:
Group B wasm.wazero.{created,active} upstream-parity + envoy-go-strict
wasm.<plugin_name>.{executions,hostcall_denied,envoy_go.failures}; HCM
stats_prefix DROPPED. Project stat count 114 → 119 verified byte-exact
via TestStatNames_Equal_* table-driven."
```

---

## Task 9: `internal/filter/http/wasm/compiled_config.go` 18-arm PARSE-REJECT roster + D-P5 byte-stable wording

**Files:**
- Create: `internal/filter/http/wasm/compiled_config.go` (~300-450 LoC)
- Create: `internal/filter/http/wasm/compiled_config_test.go` (~450-650 LoC)
- Append: PROGRESS.md (Task 9 entry per D-P-PLAN-3 + **D-P5 closure: 18-arm byte-stable wording pinned**)

This task lands the `compiledConfig` struct + the `buildCompiledConfig` full body covering the 18-arm PARSE-REJECT roster per parent §6.2 with byte-stable error wording per D-P5. Package-private `parseReject*` consts; `TestParseRejectConstants_ByteStable` table-driven test enforces. **D-P5 closure at this task**: pin the exact byte-stable wording for all 18 arms per parent §6.2 byte-stable wording requirement; commit-time `TestParseRejectConstants_ByteStable` enforces. Mirrors phase-22.1 Task 2 PARSE-REJECT roster discipline.

**Precondition:** Tasks 1-7 complete (internal/wasm/ provides Module + CompileCache + CompileModule + SandboxConfig + ErrUnsupportedAbiVersion); Task 8 skeleton complete (filterStats + compiledConfig struct declared).
**Artifact:** `internal/filter/http/wasm/compiled_config.go` + test file; 18-arm PARSE-REJECT byte-stable; D-P5 closed.
**Acceptance:** `go test -count=1 -v ./internal/filter/http/wasm/ -run TestBuildCompiledConfig` passes (each of 18 PARSE-REJECT arms triggered + exact wording asserted; valid-config path produces non-nil compiledConfig + sha256-keyed module cache lookup); `TestParseRejectConstants_ByteStable` passes byte-exact for all 18 constants; `golangci-lint run ./internal/filter/http/wasm/...` clean; D-P5 closure recorded in PROGRESS.md.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `compiled_config.go` per 25.1 SPEC §4.2 + parent §6.2 18-arm PARSE-REJECT roster. Package-private `parseReject*` const declarations for all 18 arms; `buildCompiledConfig(typedConfig *anypb.Any, ctx FilterFactoryContext) (*compiledConfig, error)` dispatches across the roster. D-P5 closure: pin byte-stable wording for each arm per parent §6.2; e.g., arm 5 `parseRejectVmConfigCodeRequired = "wasm: config.vm_config.code is required"`; arm 6 `parseRejectRemoteDeferred = "wasm: config.vm_config.code.remote is not yet supported; use local AsyncDataSource (envoy-go-strict)"`; arm 11 `parseRejectRuntimeDiscriminator = "wasm: config.vm_config.runtime: only '' or 'envoy.wasm.runtime.wazero' supported (envoy-go-strict)"`; arm 16 `parseRejectAbiVersion = "wasm: config.vm_config.code: unsupported ABI version; only proxy_abi_version_0_2_1 supported (envoy-go-strict)"`; arm 17 `parseRejectCompileFailed = "wasm: config.vm_config.code: compile: %w"` (wraps wazero error). Returns wrapped `errors.New(parseReject...)` for byte-stability. `TestParseRejectConstants_ByteStable` table-driven test enforces byte-exact wording for all 18 constants. Tests cover each PARSE-REJECT arm triggered + exact wording asserted + valid-config path. Quote command outputs + D-P5 closure ratification in PROGRESS.md.

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/compiled_config_test.go`

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run 'TestBuildCompiledConfig|TestParseRejectConstants_ByteStable'
# Expected: FAIL (functions not yet defined)
```

- [ ] **Step 2: Implement `internal/filter/http/wasm/compiled_config.go`** per §4.2 + parent §6.2 + D-P5 byte-stable wording.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run 'TestBuildCompiledConfig|TestParseRejectConstants_ByteStable'
# Expected: PASS (all 18 PARSE-REJECT arms + byte-stable wording verified)
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 9 entry with D-P5 closure + commit**

```bash
git add internal/filter/http/wasm/compiled_config.go internal/filter/http/wasm/compiled_config_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): 18-arm PARSE-REJECT roster + D-P5 byte-stable wording

Phase 25.1 Task 9 (Tier B filter package). compiledConfig struct per §4.2 +
buildCompiledConfig full body covering 18-arm PARSE-REJECT roster per parent
§6.2 + AMEND-A1 + AMEND-A6. Package-private parseReject* const declarations;
byte-stable wording per D-P5 closure at this task. TestParseRejectConstants_
ByteStable table-driven enforces each constant string byte-exact.
D-P5 CLOSED: 18-arm wording pinned per parent §6.2 wording-discipline."
```

---

## Task 10: `internal/filter/http/wasm/datasource.go` 4-arm AsyncDataSource.Local resolution

**Files:**
- Create: `internal/filter/http/wasm/datasource.go` (~150-220 LoC)
- Create: `internal/filter/http/wasm/datasource_test.go` (~300-450 LoC)
- Append: PROGRESS.md (Task 10 entry per D-P-PLAN-3)

This task lands the 4-arm `AsyncDataSource.Local` dispatch per parent §5.4. `resolveDataSource(local *corev3.DataSource) ([]byte, error)` returns bytes for downstream `CompileModule` consumption. `WatchedDirectory` PARSE-REJECT + `Remote` PARSE-REJECT + empty-oneof PARSE-REJECT. Byte-stable wording per parent §6.2 arms 6-15 (cross-references Task 9's `parseReject*` consts).

**Precondition:** Task 8 skeleton complete (package + filter struct declared); Task 9 complete (parseReject* consts available).
**Artifact:** `internal/filter/http/wasm/datasource.go` + test file; 4-arm + WatchedDirectory PARSE-REJECT + empty-oneof tested.
**Acceptance:** `go test -count=1 -v ./internal/filter/http/wasm/ -run TestResolveDataSource` passes (4-arm + WatchedDirectory PARSE-REJECT + empty-oneof + per-arm failure paths); `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `datasource.go` per parent §5.4 + AMEND-5 10-arm refinement. `resolveDataSource(local *corev3.DataSource) ([]byte, error)` dispatches across the 4-arm DataSource oneof: `Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → `[]byte(s)`; `EnvironmentVariable` → `os.LookupEnv`; `WatchedDirectory` sibling → PARSE-REJECT (arm 7); empty specifier oneof → PARSE-REJECT (arm 8). Per-arm empty-content PARSE-REJECTs (filename name-empty / ENOENT / zero-byte; env-var name-empty / unset / empty-value). Byte-stable wording cross-references Task 9's `parseReject*` consts. Tests use `t.TempDir()` synthetic files for ENOENT / EACCES / EISDIR paths.

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/datasource_test.go`

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run TestResolveDataSource
# Expected: FAIL (function not yet defined)
```

- [ ] **Step 2: Implement `internal/filter/http/wasm/datasource.go`** per parent §5.4.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run TestResolveDataSource
# Expected: PASS
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 10 entry + commit**

```bash
git add internal/filter/http/wasm/datasource.go internal/filter/http/wasm/datasource_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): 4-arm AsyncDataSource.Local resolution

Phase 25.1 Task 10 (Tier B filter package). resolveDataSource dispatches
across 4-arm AsyncDataSource.Local oneof (Filename → os.ReadFile;
InlineBytes verbatim; InlineString byte-cast; EnvironmentVariable →
os.LookupEnv). WatchedDirectory PARSE-REJECT (arm 7); Remote PARSE-REJECT
(arm 6); empty-oneof PARSE-REJECT (arm 8). Byte-stable wording cross-refs
Task 9 parseReject* consts. Per-arm failure paths (ENOENT, unset env var,
empty inline-bytes) tested via t.TempDir() synthetic files."
```

---

## Task 11: `internal/filter/http/wasm/abi_callbacks.go` ABICallbacks implementation + D-P3 closure (ADR-0196 first co-consumer)

**Files:**
- Create: `internal/filter/http/wasm/abi_callbacks.go` (~500-750 LoC)
- Create: `internal/filter/http/wasm/abi_callbacks_test.go` (~500-700 LoC)
- Append: PROGRESS.md (Task 11 entry per D-P-PLAN-3 + **D-P3 closure: ADR-0196 first co-consumer ratified**)

This task lands the `abiCallbacks` struct implementing `wasm.ABICallbacks` for the per-stream HTTP-filter context. Implements the 13-method headers-bridge subset per §5.1+§5.2+§5.3: 7 header-map methods + GetProperty (minimal property tree: `request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host`) + SetProperty + SendLocalResponse + GetStatus (RE-CONSUMES `EncoderFilterCallbacks.ResponseStatus()` per ADR-0196 + D-P3 + R7 — FIRST co-consumer of phase-23 primitive) + Log + GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done. **D-P3 closure first-action**: scrape ADR-0196's accessor signature + the encoder-callback shape; confirm consumption discipline. Anticipated answer per 25.1 SPEC §12 D-P3: Consume ADR-0196 (RATIFIES the phase-23 extraction discipline). Mirrors phase-22.2's first co-consumer of phase-20 `internal/httpclient/`.

**Precondition:** Tasks 1-7 + Task 8 + Task 9 + Task 10 complete (compiledConfig + datasource available).
**Artifact:** `internal/filter/http/wasm/abi_callbacks.go` + test file; D-P3 closure evidence recorded.
**Acceptance:** `go test -count=1 -v ./internal/filter/http/wasm/ -run TestAbiCallbacks` passes (each method round-trips correctly + minimal-property-tree exhaustive coverage + GetStatus ADR-0196 first co-consumer round-trip + sandbox-deny dispatch for each capability key returns `WasmResult::InternalFailure`); `golangci-lint run ./internal/filter/http/wasm/...` clean; D-P3 closure recorded.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-P3 closure): re-read ADR-0196 (`docs/envoy-go/DECISIONS.md` ADR-0196 entry — `EncoderFilterCallbacks.ResponseStatus()` from phase-23 admission_control primitive); confirm accessor signature + encoder-callback shape. Anticipated answer per 25.1 SPEC §12 D-P3: Consume ADR-0196 — this is the FIRST co-consumer of the phase-23 primitive (RATIFIES the extraction discipline analogous to phase-22.2's first co-consumer of phase-20 internal/httpclient/). Record empirical evidence + signature confirmation in PROGRESS.md Task 11 entry. Then author `abi_callbacks.go` per 25.1 SPEC §3.5 + §5. `abiCallbacks struct { filter *filter; cfg *compiledConfig; decoderCb DecoderFilterCallbacks; encoderCb EncoderFilterCallbacks }` implements `wasm.ABICallbacks` interface. 7 header-map methods route to the appropriate decoder/encoder callbacks' header maps. GetProperty implements minimal property tree (5 paths). SendLocalResponse captures `*capturedLocalResponse` on filter struct (consumed by Task 12 decode/encode handlers). GetStatus uses `f.encoderCb.ResponseStatus()` per ADR-0196 — only callable on encode-path; decode-path returns Unimplemented/empty. Log routes to filter log sink. Tests verify each method round-trips + minimal-property-tree exhaustive + GetStatus ADR-0196 round-trip + sandbox-deny path.

- [ ] **Step 1: D-P3 first-action** — re-read ADR-0196; confirm accessor signature + encoder-callback shape in scratch notes for PROGRESS.md.

- [ ] **Step 2: Write failing tests** at `internal/filter/http/wasm/abi_callbacks_test.go`

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run TestAbiCallbacks
# Expected: FAIL (struct not yet defined)
```

- [ ] **Step 3: Implement `internal/filter/http/wasm/abi_callbacks.go`** per §3.5 + §5 + D-P3 ADR-0196 consumption.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -count=1 -v ./internal/filter/http/wasm/ -run TestAbiCallbacks
# Expected: PASS (all 13-method round-trips + minimal property tree + GetStatus + sandbox-deny)
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 5: Append PROGRESS.md Task 11 entry with D-P3 closure + commit**

```bash
git add internal/filter/http/wasm/abi_callbacks.go internal/filter/http/wasm/abi_callbacks_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): ABICallbacks impl + D-P3 ADR-0196 first co-consumer

Phase 25.1 Task 11 (Tier B filter package). abiCallbacks struct implements
wasm.ABICallbacks for per-stream HTTP-filter context — 7 header-map methods
+ GetProperty (minimal property tree: request.headers.*, response.headers.*,
request.path, request.method, request.host) + SetProperty + SendLocalResponse
(captures *capturedLocalResponse on filter struct) + GetStatus (RE-CONSUMES
EncoderFilterCallbacks.ResponseStatus() per ADR-0196 + D-P3 + R7 — FIRST
co-consumer of phase-23 primitive; RATIFIES the extraction discipline
analogous to phase-22.2's first co-consumer of phase-20 internal/httpclient/)
+ Log + GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done.
D-P3 CLOSED: ADR-0196 first co-consumer confirmed."
```

---

## Task 12: `internal/filter/http/wasm/decode_headers.go` + `encode_headers.go`

**Files:**
- Create: `internal/filter/http/wasm/decode_headers.go` (~150-220 LoC)
- Create: `internal/filter/http/wasm/encode_headers.go` (~100-150 LoC)
- Modify: `internal/filter/http/wasm/wasm.go` (extend filter struct + `New` factory body wiring; cross-references Task 8 skeleton + Task 11 abiCallbacks)
- Modify: `internal/filter/http/wasm/wasm_test.go` (add decode/encode integration tests)
- Append: PROGRESS.md (Task 12 entry per D-P-PLAN-3)

This task lands the filter dispatch shape per 25.1 SPEC §4.3. `DecodeHeaders` constructs per-stream `*wasm.VM` via `wasm.NewVM(opts...)`, registers ABICallbacks, runs module-init lifecycle, dispatches `proxy_on_request_headers`, handles `ProxyAction` (CONTINUE → Continue; PAUSE w/o local-response → log + Continue; captured local-response → `StopIteration` + `cb.SendLocalReply`). `EncodeHeaders` mirrors for `proxy_on_response_headers`. `cfg.stats.executions++` per dispatch. `OnDestroy` calls `vm.CallProxyOnDone` + `vm.CallProxyOnLog` + `vm.CallProxyOnDelete` + `vm.Close()`.

**Precondition:** Tasks 7 + 8 + 11 complete (internal/wasm/ VM + abiCallbacks struct + filter skeleton).
**Artifact:** `decode_headers.go` + `encode_headers.go` + `wasm.go` `New` factory full body; full per-stream VM lifecycle integration tested.
**Acceptance:** `go test -count=1 -race -v ./internal/filter/http/wasm/` passes (full lifecycle integration via test-double `DecoderFilterCallbacks` + `EncoderFilterCallbacks`; per-stream VM created → dispatched → destroyed; panic-wrapper bumps `envoy_go.failures`; sandbox-deny bumps `hostcall_denied`); `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `decode_headers.go` + `encode_headers.go` per 25.1 SPEC §4.3. `DecodeHeaders(headers Header, endStream bool)` constructs `vm := wasm.NewVM(ctx, wasm.WithSandboxConfig(cfg.sandbox), wasm.WithLogSink(filterLog))` → `vm.RegisterABICallbacks(&abiCallbacks{filter: f, cfg: cfg, decoderCb: f.dcb, encoderCb: nil})` → `vm.Run(ctx, cfg.module, cfg.rootContextID)` → `vm.CallProxyOnContextCreate(ctx, f.streamContextID, cfg.rootContextID)` → `cfg.stats.executions.Inc()` → `vm.CallProxyOnRequestHeaders(ctx, f.streamContextID, uint32(len(headers)), endStream)`. If `f.sentLocalResponse != nil` → `cb.SendLocalReply(captured.statusCode, captured.body, captured.additionalHeaders)` + return `StopIteration`. Else `ProxyAction::CONTINUE` → `Continue`. `ProxyAction::PAUSE` w/o local-response → integration error log + `Continue` (stream-control deferred per §1 primitive 6). On error (wazero trap or hostcall-denial chain) → `cfg.stats.envoyGoFailures.Inc()` + log + `Continue`. `EncodeHeaders` mirrors for proxy_on_response_headers but RE-USES the per-stream `*VM` constructed at DecodeHeaders (filter struct holds `vm *wasm.VM`). `OnDestroy` calls `vm.CallProxyOnDone(ctx, f.streamContextID)` + `vm.CallProxyOnLog` + `vm.CallProxyOnDelete` + `vm.Close()`. Extend `wasm.go` `New` factory: parse via `buildCompiledConfig` → construct `compiledConfig` → return `FilterInstanceFactory` closure producing per-stream `*filter` values. Tests verify full lifecycle integration; panic-wrapper bumps envoy_go.failures; sandbox-deny bumps hostcall_denied.

- [ ] **Step 1: Write failing tests** at `internal/filter/http/wasm/wasm_test.go` (extend with decode/encode integration)

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/
# Expected: FAIL (functions not yet defined / New factory still returns sentinel)
```

- [ ] **Step 2: Implement `decode_headers.go` + `encode_headers.go` + extend `wasm.go` New factory** per §4.3.

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test -count=1 -race -v ./internal/filter/http/wasm/
# Expected: PASS (full lifecycle integration; counter bumps verified)
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 4: Append PROGRESS.md Task 12 entry + commit**

```bash
git add internal/filter/http/wasm/ docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(internal/filter/http/wasm): DecodeHeaders + EncodeHeaders dispatch shape

Phase 25.1 Task 12 (Tier B filter package). DecodeHeaders constructs per-stream
*wasm.VM via wasm.NewVM(opts) + registers ABICallbacks + runs module-init
lifecycle (Run with _initialize or _start + proxy_on_vm_start +
proxy_on_configure) + CallProxyOnContextCreate + executions.Inc() +
CallProxyOnRequestHeaders + ProxyAction handling (CONTINUE → Continue;
captured-local-response → StopIteration + SendLocalReply; PAUSE w/o
local-response → log + Continue per §1 primitive 6 stream-control deferred).
On error → envoy_go.failures.Inc() + log + Continue. EncodeHeaders mirrors
for proxy_on_response_headers; RE-USES per-stream *VM. OnDestroy calls
CallProxyOnDone + CallProxyOnLog + CallProxyOnDelete + Close. New factory
full body wires buildCompiledConfig + FilterInstanceFactory closure."
```

---

## Task 13: Boot-registration at `cmd/envoy-go/main.go` (alphabetical position per §3.6)

**Files:**
- Modify: `cmd/envoy-go/main.go` (+1 LoC + +1 import per ADR-0100 §2.2)
- Append: PROGRESS.md (Task 13 entry per D-P-PLAN-3)

This task wires the `envoy.filters.http.wasm` filter at boot. Per 25.1 SPEC §3.6 + D-P-PLAN-6: `httpReg.Register(wasm.TypeURL, wasm.New)` insertion at the line immediately before `httpReg.Freeze()` (alphabetical position; `wasm` sorts after `router` → appends to tail of the 19-entry roster). 19 → 20 HTTP filters wired post-25.1. **D-P-PLAN-6 first-action**: re-verify the alphabetical roster via `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` — if a successor row landed between this PLAN session and IMPL session, adjust the insertion line accordingly.

**Precondition:** Tasks 1-12 complete (full filter package wired + `wasm.New` factory works).
**Artifact:** `cmd/envoy-go/main.go` registers `wasm.New`; filter discoverable via registry.
**Acceptance:** `go build ./...` clean; `go test -count=1 ./cmd/envoy-go/...` passes (integration test verifies the filter is discoverable via the registry); `grep -c 'httpReg.Register' cmd/envoy-go/main.go` returns 20 (was 19).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-P-PLAN-6 verify): `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` to re-verify the alphabetical roster at master tip. If successor rows landed between this PLAN session and IMPL session, adjust the insertion line accordingly. Then add `wasm "github.com/esalaine/envoy-go/internal/filter/http/wasm"` alphabetical-among-imports + `httpReg.Register(wasm.TypeURL, wasm.New)` insertion at the line immediately before `httpReg.Freeze()`. Per ADR-0072 — registration order does not affect runtime behavior; stylistic discipline only. NO RegisterPerRouteValidator call delegation at boot — the per-route validator is registered via `reg.RegisterPerRouteValidator(filterName, validatePerRouteWasm)` from inside `wasm.New` itself per parent §5.2 + ADR-0110 (matches phase-10 + phase-20 + phase-22.1 precedent).

- [ ] **Step 1: Verify alphabetical roster** at master tip

```bash
grep -nE 'httpReg.Register' cmd/envoy-go/main.go
# Verify the 19-entry roster + identify the insertion line (immediately before httpReg.Freeze())
```

- [ ] **Step 2: Implement the insertion** per §3.6 + D-P-PLAN-6.

- [ ] **Step 3: Verify build + count**

```bash
go build ./...
# Expected: clean
grep -c 'httpReg.Register' cmd/envoy-go/main.go
# Expected: 20 (was 19)
go test -count=1 ./cmd/envoy-go/...
# Expected: PASS (filter discoverable via registry)
```

- [ ] **Step 4: Append PROGRESS.md Task 13 entry + commit**

```bash
git add cmd/envoy-go/main.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "feat(cmd/envoy-go): wire envoy.filters.http.wasm at 20th HTTP filter

Phase 25.1 Task 13 (Tier B filter package). httpReg.Register(wasm.TypeURL,
wasm.New) insertion at the line immediately before httpReg.Freeze() per
ADR-0100 §2.2 (alphabetical position; wasm sorts after router → appends to
tail of 19-entry roster per §3.6). 19 → 20 HTTP filters wired. Per ADR-0072
registration order does NOT affect runtime behavior; stylistic discipline."
```

---

## Tier C — Fuzzer + differential fixtures (Tasks 14-16)

## Task 14: 34th project-wide fuzzer `FuzzWasmConfigParse`

**Files:**
- Create: `internal/filter/http/wasm/fuzz_test.go` (~100-150 LoC)
- Create: `internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/` directory with ~30 corpus seeds per D-P-PLAN-7
- Append: PROGRESS.md (Task 14 entry per D-P-PLAN-3)

This task lands the 34th project-wide fuzzer `FuzzWasmConfigParse` per ADR-0018 baseline. Per 25.1 SPEC §6 Task 14 + D-P-PLAN-7 corpus roster (~30 seeds: 18 PARSE-REJECT arms + 5 valid + 7 adversarial). Must-never-panic invariant via wazero compile error path (arm 17 wrapping — adversarial bytecode must not crash the parser). `grep -c "^func Fuzz"` project-wide goes from 33 → 34.

**Precondition:** Tasks 1-12 complete (`buildCompiledConfig` non-skeleton + wired).
**Artifact:** `fuzz_test.go` + `testdata/fuzz/FuzzWasmConfigParse/` corpus dir; fuzzer runs clean at 30s.
**Acceptance:** `go test -run=^$ -fuzz=FuzzWasmConfigParse -fuzztime=30s ./internal/filter/http/wasm/` clean (no panics); `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns 34; `golangci-lint run ./internal/filter/http/wasm/...` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author `fuzz_test.go` per ADR-0018 baseline. `FuzzWasmConfigParse(f *testing.F)` consumes `*anypb.Any` bytes + invokes `buildCompiledConfig`; must-never-panic invariant. Author ~30 corpus seeds per D-P-PLAN-7: 18 PARSE-REJECT arms (1 per arm from parent §6.2) + 5 valid-config seeds (1 per AsyncDataSource arm + 1 with non-empty CapabilityRestrictionConfig) + 7 adversarial-wasm-bytecode seeds (malformed wasm headers, oversize sections, sentinel-spoof attempts, truncated module, null bytes, no-body function, unbalanced control flow). Each seed is a serialized `*anypb.Any` wrapping a `wasmv3.Wasm` proto fixture. Verify `go test -run=^$ -fuzz=FuzzWasmConfigParse -fuzztime=30s ./internal/filter/http/wasm/` clean. Verify project-wide fuzzer count = 34 via find/grep oneliner.

- [ ] **Step 1: Write the fuzzer skeleton** at `internal/filter/http/wasm/fuzz_test.go`

```bash
# Author FuzzWasmConfigParse skeleton + corpus seed loader.
go test -count=1 -v ./internal/filter/http/wasm/ -run TestFuzzWasmConfigParse
# Expected: PASS (skeleton smoke test)
```

- [ ] **Step 2: Add corpus seeds** at `internal/filter/http/wasm/testdata/fuzz/FuzzWasmConfigParse/` per D-P-PLAN-7 roster.

- [ ] **Step 3: Run fuzzer at 30s budget**

```bash
go test -run=^$ -fuzz=FuzzWasmConfigParse -fuzztime=30s ./internal/filter/http/wasm/
# Expected: clean (no panics; no new failing inputs)
```

- [ ] **Step 4: Verify project-wide fuzzer count**

```bash
find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l
# Expected: 34 (was 33)
go vet ./... && golangci-lint run ./internal/filter/http/wasm/...
# Expected: clean
```

- [ ] **Step 5: Append PROGRESS.md Task 14 entry + commit**

```bash
git add internal/filter/http/wasm/fuzz_test.go internal/filter/http/wasm/testdata/ docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "test(internal/filter/http/wasm): 34th project-wide fuzzer FuzzWasmConfigParse

Phase 25.1 Task 14 (Tier C fuzzer). 34th project-wide fuzzer per ADR-0018
baseline. ~30 corpus seeds per D-P-PLAN-7: 18 PARSE-REJECT arms + 5 valid +
7 adversarial wasm bytecode. Must-never-panic invariant via wazero compile
error path (arm 17 wrapping). Project-wide fuzzer count 33 → 34 confirmed
via find/grep oneliner. D-S1 from 25.1 SPEC §11.1 RATIFIED at IMPL."
```

---

## Task 15: Differential fixture `0034-http-wasm-headers-bridge` (7 scenarios; full cross-side per parent §8.1 + §4.5 D6 guardrails) + NEW `BackendKind=HTTPWasm`

**Files:**
- Create: `test/fixtures/0034-http-wasm-headers-bridge/README.md` (~150-250 LoC)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/envoy.yaml` (~150-250 LoC)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/envoy-go.yaml` (~150-250 LoC)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/expectations.yaml` (~100-180 LoC; human-readable; NOT consumed by runner)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/inputs/driver.go` (~400-600 LoC)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/scripts/{a..g}_*/Cargo.toml + src/lib.rs` (~30 LoC each × 7)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/scripts/README.md` (~60-100 LoC)
- Create: `test/fixtures/0034-http-wasm-headers-bridge/bytecode/{a..g}_*.wasm` (vendored pre-built blobs)
- Modify: `test/differential/fixture/fixture.go` (+1 `BackendKind=HTTPWasm` enum value + dispatcher metadata; ~+15 LoC)
- Modify: `test/differential/runner_test.go` (+blank import + switch-case for `HTTPWasm`; ~+12 LoC)
- Append: PROGRESS.md (Task 15 entry per D-P-PLAN-3)

This task lands the 7-scenario cross-side fixture per parent §8.1 + §4.5 D6 guardrails. 7 scenarios per 25.1 SPEC §9.1 table (add-fixed-header / replace-header / remove-header / respond-shortcircuit / log-only-passthrough / header-iteration-count / property-read-method) all full cross-side byte-exact via existing `CompareBytes`. Scenario (e) `log-only-passthrough` uses `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch` (subject-side stat-counter assertion on the cross-side runner branch; NOT `SubjectAsserter` which is dead on the cross-side path). Each scenario complies with §4.5 D6 guardrails (no memory traps; HTTP/1.1; no float-formatted logs; only 24-hostcall surface). Vendored Rust source per Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target). NEW `BackendKind=HTTPWasm` added to runner switch-case. 35 → 36 differential fixture dirs at this Task.

**Precondition:** Tasks 1-13 complete (full filter package + boot-registration); rust toolchain available (for reproduction script — vendored blobs commit anyway).
**Artifact:** `test/fixtures/0034-http-wasm-headers-bridge/` complete + `BackendKind=HTTPWasm` registered + 7 vendored `.wasm` blobs committed.
**Acceptance:** `go test -count=1 ./test/differential -run TestDifferential/0034` GREEN — 7 scenarios full cross-side byte-exact via `CompareBytes`; scenario (e) `StatsAsserter.AssertStats` cross-side stat-counter delta verified (deliberately-break test verifies liveness: break the assertion → expect FAIL → restore); `go build ./...` clean; `grep -c 'HTTPWasm' test/differential/fixture/fixture.go` returns ≥1 (enum + dispatcher); `golangci-lint run` clean.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> Author fixture-0034 bundle per 25.1 SPEC §9.1 + parent §8.1 + §4.5 D6 guardrails. Directory structure per parent §8.1.3: README + envoy.yaml + envoy-go.yaml + expectations.yaml + inputs/driver.go + scripts/{a..g}_*/Cargo.toml + src/lib.rs + scripts/README.md + bytecode/{a..g}_*.wasm. The 7 scenarios (per 25.1 SPEC §9.1 table; verbatim plugin behavior + wire-output assertion + asserter columns). For scenario (e) `log-only-passthrough`: subject-side stat assertion via `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)` per `reference_differential_asserter_dispatch` — scrape `/stats?format=text` admin endpoints + diff `wasm.<plugin>.executions` counter delta + assert cross-side `executions_delta = 1` per probe. DO NOT place the stat assertion at `SubjectAsserter.AssertSubject` (that interface is dead on the cross-side path; would result in vacuous-pass per `reference_differential_asserter_dispatch` bug #1). After authoring: deliberately-break test (modify expected stat name to wrong string; verify test FAILS; restore). NEW `BackendKind=HTTPWasm` at `test/differential/fixture/fixture.go` + dispatcher metadata (mirror phase-22.1 HTTPLua precedent + the highest existing BackendKind++). Runner switch-case + blank-import at `test/differential/runner_test.go`. Vendored Rust sources at scripts/{a..g}_*/{Cargo.toml,src/lib.rs} per Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target); reproduction script at scripts/README.md (`rustup target add wasm32-wasip1; cargo build --release --target wasm32-wasip1`). Pre-built `.wasm` blobs committed to bytecode/ per Q9. Tests via `go test -count=1 ./test/differential -run TestDifferential/0034`.

- [ ] **Step 1: Author fixture directory structure + Rust sources + vendored bytecode** at `test/fixtures/0034-http-wasm-headers-bridge/` per parent §8.1.3 + Q9.

- [ ] **Step 2: Register `BackendKind=HTTPWasm` + runner switch-case + blank-import**

```bash
# Edit test/differential/fixture/fixture.go (NEW enum + dispatcher)
# Edit test/differential/runner_test.go (blank import + switch case for HTTPWasm)
go build ./...
# Expected: clean
```

- [ ] **Step 3: Author driver.go + run fixture-0034**

```bash
go test -count=1 -v ./test/differential -run TestDifferential/0034
# Expected: PASS (7 scenarios full cross-side byte-exact + scenario (e) StatsAsserter cross-side delta)
```

- [ ] **Step 4: Verify scenario (e) liveness via deliberately-break test**

```bash
# Modify expected stat name in driver.go to wrong string → run → expect FAIL → restore → re-run → PASS
go test -count=1 -v ./test/differential -run TestDifferential/0034
# Expected: PASS after restore (liveness verified per reference_differential_asserter_dispatch)
go vet ./... && golangci-lint run
# Expected: clean
```

- [ ] **Step 5: Append PROGRESS.md Task 15 entry + commit**

```bash
git add test/fixtures/0034-http-wasm-headers-bridge/ test/differential/ docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "test(test/fixtures/0034-http-wasm-headers-bridge): 7-scenario full cross-side byte-exact per parent §8.1

Phase 25.1 Task 15 (Tier C fixture). 7 scenarios full cross-side byte-exact
via existing CompareBytes runner branch per parent §8.1 + §4.5 D6 guardrails
(no memory traps; HTTP/1.1; no float-formatted logs; only 24-hostcall surface).
Scenario (e) log-only-passthrough uses StatsAsserter.AssertStats per
reference_differential_asserter_dispatch (NOT SubjectAsserter which is dead
on cross-side path; deliberately-break test verified liveness). Vendored
Rust sources at scripts/ per Q9 + AMEND-A1 (proxy-wasm-rust-sdk =0.2.4 +
wasm32-wasip1 target); pre-built .wasm blobs vendored at bytecode/.
NEW BackendKind=HTTPWasm + runner switch-case + blank-import. 35 → 36
differential fixture dirs."
```

---

## Task 16: Differential fixture `0035-http-wasm-boot-reject` + D-P6 arm-finalization

**Files:**
- Create: `test/fixtures/0035-http-wasm-boot-reject/README.md` (~80-120 LoC)
- Create: `test/fixtures/0035-http-wasm-boot-reject/envoy.yaml` (~100-150 LoC; reference Envoy bootstrap with anticipated arm 5 misconfiguration)
- Create: `test/fixtures/0035-http-wasm-boot-reject/envoy-go.yaml` (~100-150 LoC; subject bootstrap symmetric)
- Create: `test/fixtures/0035-http-wasm-boot-reject/inputs/driver.go` (~150-250 LoC; implements `BootRejectFixture`)
- Modify: `test/differential/runner_test.go` (+blank import for 0035; ~+1 LoC)
- Append: PROGRESS.md (Task 16 entry per D-P-PLAN-3 + **D-P6 closure: boot-reject arm selection**)

This task lands single-arm boot-reject parity per D-P6 — anticipated arm 5 (`vm-config-code-required`) per parent §8.2. **D-P6 closure first-action**: empirically-test arm 5 + alternative arms {3, 4, 8, 17} against upstream Envoy v1.37.2 boot stderr; pick the arm with the most distinctive substring + cleanest config shape; record disposition in this task's PROGRESS.md entry. Anticipated answer per 25.1 SPEC §12 D-P6: arm 5 with anticipated common substring `"required"` reproduced by envoy-go's mirror wording. 36 → 37 differential fixture dirs (+1). Cross-side common stderr substring via existing `BootRejectFixture` runner discipline from phase-22.1 (no harness delta at 25.1). Per `reference_differential_fixture_dispatch_constraint` — one fixture dir = one runner branch (this is the boot-reject branch).

**Precondition:** Task 15 complete (BackendKind=HTTPWasm registered + fixture-0034 green); existing `BootRejectFixture` harness from phase-22.1 available.
**Artifact:** `test/fixtures/0035-http-wasm-boot-reject/` complete; D-P6 closure evidence recorded.
**Acceptance:** `go test -count=1 ./test/differential -run TestDifferential/0035` GREEN — both reference + subject fail to boot AND both sides' stderr contain the anticipated common substring (per the chosen arm from D-P6 closure); `go build ./...` clean; `golangci-lint run` clean; 37 differential fixture dirs (verify via `ls -d test/fixtures/00*/ | wc -l` returns 37).

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose`):

> FIRST ACTION (D-P6 closure): empirically test boot-reject arms {3, 4, 5, 8, 17} against upstream Envoy v1.37.2 — for each arm, author a minimal envoy.yaml triggering ONLY that arm + run upstream Envoy + capture boot stderr + identify common-substring candidates. Pick the arm with the most distinctive substring + cleanest config shape. Anticipated answer per 25.1 SPEC §12 D-P6: arm 5 (`vm-config-code-required`) with anticipated common substring `"required"` reproduced by envoy-go's mirror wording. Record empirical evidence + chosen arm + chosen common substring in PROGRESS.md Task 16 entry. Then author fixture-0035 bundle: README + envoy.yaml (reference Envoy bootstrap with deliberately-malformed config triggering the chosen arm — anticipated missing `vm_config.code`) + envoy-go.yaml (symmetric) + inputs/driver.go implementing `BootRejectFixture` interface (`BootRejectConfig() string` returns bootstrap path; `ExpectedBootErrorSubstring() string` returns the chosen common substring). Per `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch; this is the boot-reject branch. Re-use existing `BootRejectFixture` harness from phase-22.1 Task 13 (no harness delta at 25.1). Add blank-import at `test/differential/runner_test.go`. Run `go test -count=1 ./test/differential -run TestDifferential/0035` — both reference + subject fail to boot AND both sides' stderr contain the chosen common substring.

- [ ] **Step 1: D-P6 first-action empirical test** — author minimal envoy.yaml fixtures for arms {3, 4, 5, 8, 17}; run upstream Envoy v1.37.2 on each; capture boot stderr; identify common-substring candidates; pick best arm. Record evidence in scratch notes for PROGRESS.md.

- [ ] **Step 2: Author fixture directory** per chosen arm

```bash
# Create test/fixtures/0035-http-wasm-boot-reject/{README.md,envoy.yaml,envoy-go.yaml,inputs/driver.go}
# Add blank-import at test/differential/runner_test.go
go build ./...
# Expected: clean
```

- [ ] **Step 3: Run fixture-0035**

```bash
go test -count=1 -v ./test/differential -run TestDifferential/0035
# Expected: PASS (both reference + subject fail to boot; common-substring matched)
go vet ./... && golangci-lint run
# Expected: clean
```

- [ ] **Step 4: Verify fixture count**

```bash
ls -d test/fixtures/00*/ | wc -l
# Expected: 37 (was 35; +2 from 0034 + 0035)
```

- [ ] **Step 5: Append PROGRESS.md Task 16 entry with D-P6 closure + commit**

```bash
git add test/fixtures/0035-http-wasm-boot-reject/ test/differential/runner_test.go docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md
git commit -m "test(test/fixtures/0035-http-wasm-boot-reject): single-arm boot-reject parity per D-P6

Phase 25.1 Task 16 (Tier C fixture). Single-arm boot-reject parity per
parent §8.2 + 25.1 SPEC §9.2 + D-P6 closure. D-P6 CLOSED: arm <N>
(<arm-name>) chosen per upstream Envoy v1.37.2 boot stderr empirical
test against arms {3, 4, 5, 8, 17}; chosen common substring '<substring>'.
Per reference_differential_fixture_dispatch_constraint — one fixture dir
= ONE runner branch; this is the boot-reject branch using the existing
phase-22.1 BootRejectFixture harness (no harness delta at 25.1).
36 → 37 differential fixture dirs."
```

---

## Tier D — Atomic landing (Task 17)

## Task 17: Benchmark + BEHAVIOR_CONTRACT.md 6-edit bundle + ADR-0202+0203+0204 §Decision+§Consequences body landing + STATE.md re-advance + ROADMAP row 25.1 IMPL-done + REVIEW.md + R8 escape-valve gate

**Files:**
- Create: `internal/filter/http/wasm/wasm_bench_test.go` (~50-80 LoC; `BenchmarkPerStreamVM_Construction_Headers` per R8 gate)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (6-edit bundle per parent §13.5 + this 25.1 SPEC §14; ~+300-450 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences body landings; ~+500-800 LoC; CONDITIONAL ADR-0205 §Context+§Decision+§Consequences if R8 escape-valve fires per D-P-PLAN-10)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place per BOOTSTRAP §4.1 invariant 1)
- Modify: `docs/envoy-go/ROADMAP.md` (row 25.1 flips `in-progress → done` + per-cell IMPL-done annotation per ADR-0106; parent row `25` STAYS `in-progress`; sub-rows `25.2` + `25.3` UNCHANGED `planned`)
- Create: `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/REVIEW.md` (~300-400 LoC; per `superpowers:requesting-code-review`)
- Append: PROGRESS.md (Task 17 entry per D-P-PLAN-3 — final task; 6-gate outputs captured verbatim + **D-P4 closure: R8 benchmark gate disposition**)

This task lands the atomic landing per ADR-0052 atomic landing discipline + parent SPEC §13.5 6-edit bundle + 3 ADR §Decision + §Consequences body landings + the STATE/ROADMAP advance + the REVIEW.md authoring + the R8 benchmark gate evaluation. **Depends on Tasks 1-16** (consumes all prior surfaces; closes the 25.1 SPEC §15.3 30-item acceptance checklist). **D-P4 closure at R8 benchmark gate**: if `BenchmarkPerStreamVM_Construction_Headers` reports `ns/op > 1_000_000` (= 1ms), the ADR-0205 escape-valve FIRES; §Context + §Decision + §Consequences body all land at this same Task 17 commit per ADR-0044 anchoring "per-module wazero Runtime pool with pre-instantiated entries". If `ns/op <= 1_000_000`: WEAK-default per-stream construction STANDS; no ADR-0205 fires. Anticipated: under threshold per parent §1.2 + phase-22.1 70µs analogous-benchmark precedent.

**Precondition:** Tasks 1-16 complete; all 6 phase-done gates A/B/C/D/E/F GREEN.
**Artifact:** Benchmark result + all 6 BEHAVIOR_CONTRACT.md edits + 3 ADR bodies + STATE.md + ROADMAP + REVIEW.md; phase 25.1 IMPL phase-done ready for squash-merge.
**Acceptance:** Benchmark `BenchmarkPerStreamVM_Construction_Headers` runs clean + records `ns/op` value (R8 gate: `<1_000_000` → STANDS; `>1_000_000` → ADR-0205 fires); All 6 BEHAVIOR_CONTRACT.md edits land in one atomic commit; ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences bodies complete + grep-verified; STATE.md re-advance reflects post-25.1-IMPL state (`lifecycle-state: phase 25.1 IMPL done; awaiting 25.2 SPEC`; `next-skill: superpowers:brainstorming` scoped to 25.2 OR `superpowers:writing-plans` if BRAINSTORM-skip; `next-free ADR: ADR-0205` UNCHANGED if R8 does NOT fire OR `ADR-0206` if fires; 114 → 119 stat count; 19 → 20 HTTP filter count; 33 → 34 fuzzer count; ADR tail advance to ADR-0204 or ADR-0205); ROADMAP row 25.1 flipped `in-progress → done`; per-task PROGRESS.md entries complete across all 17 tasks + Pre-Task 0; REVIEW.md authored per `superpowers:requesting-code-review`; all 30 items from 25.1 SPEC §15.3 acceptance checklist closed.

**Subagent dispatch outline** (per D-P-PLAN-2 `general-purpose` with explicit reference to 25.1 SPEC §15.3 + parent §13.5 + ADR-0202 + ADR-0203 + ADR-0204):

> Author Task 17 atomic landing per 25.1 PLAN Task 17 + 25.1 SPEC §6 Task 17 + parent SPEC §13.5 + ADR-0052 atomic landing discipline. First author `wasm_bench_test.go` `BenchmarkPerStreamVM_Construction_Headers(b *testing.B)` measuring per-stream `*wazero.Runtime` construction cost; record ns/op verbatim. Evaluate R8 gate per D-P-PLAN-10: `< 1_000_000` → STANDS (WEAK-default per-stream construction); `> 1_000_000` → ADR-0205 escape-valve FIRES (author §Context + §Decision + §Consequences body at this commit per ADR-0044 anchoring per-module wazero Runtime pool design). Then run all 6 phase-done gates A-F verbatim. Then author the 6 BEHAVIOR_CONTRACT.md edits per parent §13.5: (1) NEW `### envoy.filters.http.wasm` subsection ~150-250 lines headers-bridge-focused for 25.1 + forward-pointers to 25.2 + 25.3; (2) Stat-table 114 → 119 extension under `## Stat surface` (5 new rows under tri-group prefix structure per AMEND-A2) + tri-group prefix structural note + HCM-stats_prefix DROPPED divergence-from-§9-family-pattern note; (3) envoy-go-strict departure record #1: default-deny capability sandbox per AMEND-A5 + ADR-0204 (departure rationale: WASM exposes substantially larger and riskier hostcall surface than Lua; upstream's 3 sandbox runtimes V8/WAMR/Wasmtime marked status: alpha + security_posture: unknown); (4) envoy-go-strict departure record #2: ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6 (envoy-go-strict-stricter NOT parity — upstream accepts all 3 ABI versions; detection point bytecode_util.go byte-faithful sentinel scan); (5) envoy-go-strict departure record #3 (consolidated 4-5-record bundle): `AsyncDataSource.Remote` PARSE-REJECT + runtime-name discriminator PARSE-REJECT + 3 envoy-go-strict counters (`executions` + `hostcall_denied` + `envoy_go.failures`); (6) NEW `### Phase 25.1 forward-pointer notes` subsection ~50-80 lines (25.2 anticipated additions per AMEND-A9 foreign-function + 25.3 anticipated additions per AMEND-A3 5th-canonical REUSE-by-absence + AMEND-A8 conformance harness). Plus: ADR-0202 §Decision + §Consequences body landing (extends parent SPEC commit §Context per ADR-0044; covers `internal/wasm/` API surface per 25.1 SPEC §3.1 + sandbox roster per §3.3 + per-stream lifecycle per §3.4 + WASI 8-stub custom impl per R4 + pairs byte-faithful per R3 + bytecode_util byte-faithful per AMEND-A6 + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per parent §4.1 + BRAINSTORM Q3 + Q4); ADR-0203 §Decision + §Consequences body landing (extends parent SPEC commit §Context per ADR-0044; covers `internal/filter/http/wasm/` package shape per 25.1 SPEC §3.5 + §4 + parent §6.2 18-arm PARSE-REJECT roster + §5 24-hostcall + 13-callback surface + parent §7 5-counter stat surface per AMEND-A2 tri-group prefix structure + fixture-0034 disposition per §9.1 + §11.1 D-S1 closure + Task 2 D-P1 + Task 6 D-P2 + Task 9 D-P5 + Task 11 D-P3 + Task 16 D-P6 closures + R3 + R4 + R7 RATIFIED-PENDING items); ADR-0204 §Decision + §Consequences body landing (extends parent SPEC commit §Context per ADR-0044; covers proxy-wasm capability-restriction default-deny per AMEND-A5 + §3.3 ~80-key roster + denial semantic `WasmResult::InternalFailure`=10 + `hostcall_denied` counter + WASI denial errno per D-P1 + `SanitizationConfig` accept-empty discipline per AMEND-A1 §11.4). CONDITIONAL ADR-0205 §Context + §Decision + §Consequences if D-P-PLAN-10 R8 escape-valve fires (per-module wazero Runtime pool design). STATE.md re-advance per BOOTSTRAP §4.1 invariant 1. ROADMAP row 25.1 flip per ADR-0106. REVIEW.md per `superpowers:requesting-code-review` covering 30-item 25.1 SPEC §15.3 acceptance checklist + per-task review notes + cross-cutting review notes + green-light evidence + D-question-disposition record. 6 phase-done gates A/B/C/D/E/F outputs captured verbatim in PROGRESS.md final Task 17 entry. Cross-check ADR-0125 STAYS at 10 (NO §(xvi) amendment per AMEND-A3).

- [ ] **Step 1: Author `wasm_bench_test.go` + run benchmark for R8 gate (D-P4 closure)**

```bash
# Author internal/filter/http/wasm/wasm_bench_test.go with BenchmarkPerStreamVM_Construction_Headers
go test -count=1 -benchmem -bench=BenchmarkPerStreamVM_Construction_Headers -run=^$ ./internal/filter/http/wasm/
# Capture ns/op verbatim for PROGRESS.md.
# R8 gate: if ns/op > 1_000_000 (1ms) → ADR-0205 escape-valve fires.
#         if ns/op ≤ 1_000_000 → WEAK-default per-stream construction STANDS.
# Anticipated: under threshold (~70µs analogous to phase-22.1 LState benchmark).
```

- [ ] **Step 2: Gate A — build** — `go build ./...` clean. Capture output verbatim in PROGRESS.md.

- [ ] **Step 3: Gate B — vet + lint** — `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture output verbatim.

- [ ] **Step 4: Gate C — race** — `go test -race -count=1 ./...` clean; zero data-race violations across all packages including the new `internal/wasm/` + `internal/filter/http/wasm/` race tests + the per-stream VM construction. Capture output verbatim.

- [ ] **Step 5: Gate D — differential + cross-package regression matrix per D-P-PLAN-9** — `go test -count=1 ./test/differential/...` clean (ALL fixture directories GREEN: pre-existing baseline + new 0034 + 0035). Capture output verbatim. Verify count: `ls -d test/fixtures/00*/ | wc -l` returns the pre-existing baseline + 2 (anticipated 37 if pre-existing was 35 per precondition 13).

- [ ] **Step 6: Gate E — fuzz** — `go test -fuzz=FuzzWasmConfigParse -fuzztime=30s ./internal/filter/http/wasm/` clean (no panics). 33 pre-existing fuzzers re-run clean at 30s per seed via per-package iteration. Capture output verbatim. Verify project-wide fuzzer count = 34 via `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l`.

- [ ] **Step 7: Gate F — h2spec** — `make test-h2spec` 53/53 PASS at ADR-0051 pin. Capture output verbatim.

- [ ] **Step 8: Author BEHAVIOR_CONTRACT.md 6-edit bundle** per parent §13.5 + this 25.1 SPEC §14.

- [ ] **Step 9: Author ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences bodies in DECISIONS.md** per parent §4 + this 25.1 SPEC §3.1 + §3.3 + §3.4 + §3.5 + §4 + §5 + parent §6.2 + parent §7 + §9.1 + §11.1 D-S1 + Task 2 D-P1 + Task 6 D-P2 + Task 9 D-P5 + Task 11 D-P3 + Task 16 D-P6 closures + R3 + R4 + R7 + AMEND-A1..A9 cross-references.

- [ ] **Step 10: IF D-P-PLAN-10 R8 escape-valve fires (per Step 1 benchmark > 1ms threshold)**: author ADR-0205 §Context + §Decision + §Consequences body per ADR-0044 anchoring per-module wazero Runtime pool with pre-instantiated entries. Otherwise skip (ADR-0205 STAYS UNCONSUMED).

- [ ] **Step 11: Update STATE.md** to post-phase-25.1-IMPL state per BOOTSTRAP §4.1 invariant 1:
  - `active-phase`: `25-http-filter-wasm` (parent row STAYS in-progress)
  - `lifecycle-state`: `phase 25.1 IMPL done; awaiting 25.2 SPEC`
  - `next-skill`: `superpowers:brainstorming` (the 25.2 BRAINSTORM scoped to 25.2 sub-phase) OR `superpowers:writing-plans` if BRAINSTORM-skip per parent-BRAINSTORM-settled-enough pattern
  - `last-commit`: `<TBD — SHA-fill follow-up after squash-merge>` placeholder
  - `last-updated`: today's date
  - `next-free ADR`: `ADR-0205` (UNCHANGED if D-P-PLAN-10 R8 does NOT fire) OR `ADR-0206` (if fires)
  - Verbose summary: 17 tasks landed; 3 NEW ADRs anchored (ADR-0202 + ADR-0203 + ADR-0204) + CONDITIONAL ADR-0205 if R8 fires; 34th fuzzer FuzzWasmConfigParse clean; 37/37 differential fixture directories green; all 6 phase-done gates green; SPEC §15.3 30 items all GREEN; 114 → 119 stat count; 19 → 20 HTTP filter count; 33 → 34 fuzzer count; D-P1..D-P6 closure evidence recorded; D-S1 confirmed at SPEC + RATIFIED at IMPL.

- [ ] **Step 12: Update ROADMAP.md row 25.1** — status flips `in-progress → done`; per-cell IMPL-done annotation appended per ADR-0106 documenting the 17-task IMPL landing + 6-gate outputs + the EIGHTEENTH and FINAL §9 family-row sub-phase milestone + the SECOND occurrence of EXTRACT-NOW-at-first-consumer (after phase-22.1) + the NEW `internal/wasm/` framework primitive milestone + the SPEC §15.3 30-item acceptance + the D-P1..D-P6 + D-S1 disposition record. Parent row `25` STAYS `in-progress`. Sub-rows `25.2` + `25.3` UNCHANGED `planned`.

- [ ] **Step 13: Author REVIEW.md** per `superpowers:requesting-code-review` — ~300-400 LoC reviewer artifact covering: the 6-gate outputs verbatim; the benchmark `BenchmarkPerStreamVM_Construction_Headers` ns/op + R8 gate disposition; the 25.1 SPEC §15.3 30-item checklist verification with cite-to-PROGRESS-entry per item; the D-P-PLAN-1..D-P-PLAN-10 PLAN-time decision-disposition record (which decisions HELD, which were AMENDED at IMPL); the D-P1 + D-P2 + D-P3 + D-P5 + D-P6 closure evidence from Tasks 2/6/11/9/16; the D-P4 + R8 disposition (STANDS WEAK-default OR ADR-0205 escape-valve FIRED); the next-phase handoff state (25.2 BRAINSTORM scope hand-off).

- [ ] **Step 14: Append final PROGRESS.md Task 17 entry** with all 6 gate outputs verbatim + the 25.1 SPEC §15.3 30-item closure checklist + the D-decision disposition status + the R8 benchmark gate disposition.

- [ ] **Step 15: Verify nothing left uncommitted**

```bash
git status --porcelain
# Expect: empty
```

- [ ] **Step 16: Commit (Task 17 final IMPL-worktree commit)**

```bash
git add internal/filter/http/wasm/wasm_bench_test.go \
        docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/STATE.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/PROGRESS.md \
        docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/REVIEW.md
git commit -m "phase 25.1 Task 17: atomic landing + 6-gate phase-done verification + R8 benchmark gate

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(37/37 fixture directories incl. new 0034 + 0035 — 7 scenarios full cross-side
byte-exact via CompareBytes + scenario (e) StatsAsserter cross-side delta on
0034; single-arm boot-reject parity at chosen D-P6 arm on 0035) / E fuzz
(34 fuzzers clean; 34th FuzzWasmConfigParse confirmed per 25.1 SPEC §11.1
D-S1 closure) / F h2spec 53/53 PASS.

25.1 SPEC §15.3 30-item acceptance checklist all GREEN. D-S1 RATIFIED at IMPL
(34th-fuzzer count CONFIRMED). D-P1 closure recorded at Task 2 PROGRESS
(WASI denial errno <NOTSUP=58 | ENOTCAPABLE=76> chosen per proxy_wasm_exports.h
:232-249 scrape evidence). D-P2 closure at Task 6 (5 module-init/allocator
callbacks <ungated | gated> per wasm.cc:298-302 scrape). D-P3 closure at
Task 11 (ADR-0196 EncoderFilterCallbacks.ResponseStatus() FIRST co-consumer
RATIFIED). D-P5 closure at Task 9 (18-arm byte-stable wording pinned;
TestParseRejectConstants_ByteStable enforces). D-P6 closure at Task 16
(boot-reject arm <N> chosen with common substring '<substring>'). D-P4 + R8
gate: <STANDS WEAK-default per-stream construction | ADR-0205 escape-valve
FIRED with §Context+§Decision+§Consequences body landed at this commit>;
BenchmarkPerStreamVM_Construction_Headers reported <ns/op> per-stream.

BEHAVIOR_CONTRACT.md 6-edit bundle landed atomically per ADR-0052 + parent
§13.5 + this 25.1 SPEC §14. ADR-0202 §Decision + §Consequences body anchored
(NEW internal/wasm/ framework primitive — wazero v1.10.1 Apache-2.0 per
AMEND-A1 + per-stream VM lifecycle + per-module Module compile cache +
SandboxConfig zero-value StrictDefaultDeny per AMEND-A5 + ABICallbacks
registration interface + in-house proxy-wasm v0.2.1 host ABI for headers-
bridge subset + WASI 8-stub custom per R4 + pairs byte-faithful per R3 +
bytecode_util byte-faithful per AMEND-A6 + EXPLICIT API-REVISION ALLOWANCE
for consumer #2 per BRAINSTORM Q3+Q4). ADR-0203 §Decision + §Consequences
body anchored (NEW internal/filter/http/wasm/ package shape — 8 prod + 5
test files; compiledConfig + 5-counter filterStats tri-group prefix per
AMEND-A2; 18-arm PARSE-REJECT; 4-arm AsyncDataSource.Local; 24-hostcall
+ 13-callback surface; ADR-0196 first co-consumer at proxy_get_status per
D-P3 + R7; fixture-0034 + 0035 dispositions; vendored Rust-sourced .wasm
bytecode under bytecode/ per Q9 + AMEND-A1). ADR-0204 §Decision + §Consequences
body anchored (proxy-wasm capability-restriction default-deny per AMEND-A5;
~80-key roster; WasmResult::InternalFailure=10 denial semantic + integration
error log + hostcall_denied counter; SanitizationConfig accept-empty per
AMEND-A1 §11.4).

EIGHTEENTH and FINAL §9 family-row sub-phase landed (foundational third;
parent row 25 STAYS in-progress until 25.3 phase-done). SECOND occurrence
of EXTRACT-NOW-at-first-consumer after phase-22.1 internal/lua/ — anchors
NEW internal/wasm/ framework primitive for the broader §9 WASM host family
(cluster-specifier-wasm + access-logger-wasm + network-filter-wasm +
WasmService) per BOOTSTRAP_PROMPT.md §9 line 116. 20 HTTP filters wired
post-25.1 (was 19). Stat surface 114 → 119 names per AMEND-A2 tri-group
prefix structure (wasm.wazero.{created,active} Group-B upstream-parity +
wasm.<plugin_name>.{executions,hostcall_denied,envoy_go.failures}
envoy-go-strict extensions; HCM-stats_prefix DROPPED structural note).

Three envoy-go-strict departures documented per parent §13.5 + this 25.1
SPEC §14 edits 3 + 4 + 5: default-deny capability sandbox per AMEND-A5
+ ADR-0204 (WASM exposes substantially larger and riskier hostcall surface
than Lua; upstream's 3 sandbox runtimes marked status: alpha +
security_posture: unknown); ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6
(envoy-go-strict-stricter NOT parity); consolidated 4-5-record bundle:
AsyncDataSource.Remote PARSE-REJECT + runtime-name discriminator PARSE-REJECT
+ envoy-go-strict counters consolidated. ROADMAP row 25.1 flipped
in-progress → done; parent row 25 STAYS in-progress until 25.3 phase-done;
sub-rows 25.2 + 25.3 UNCHANGED planned. STATE.md re-advanced to
post-25.1-IMPL state. REVIEW.md authored per superpowers:requesting-code-
review."
```

---

## Phase-done squash-merge + push to origin

After Task 17 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl
# Resolve commit message — body must include the 17-task summary + the 3-NEW-ADR
# (+ CONDITIONAL ADR-0205 if R8 fired) roster + the closes-row-25.1 + EIGHTEENTH
# §9-row-foundational-third + the parent-row-25-STAYS-in-progress note + the
# 30-item acceptance checklist GREEN note + the 6-gate outputs summary.
git commit -m "$(cat <<'EOF'
Squash merge phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl

Closes ROADMAP row 25.1 (in-progress → done) — EIGHTEENTH and FINAL §9
family-row foundational third (parent row 25 STAYS in-progress until 25.3
phase-done; sub-rows 25.2 + 25.3 UNCHANGED planned per ADR-0106 sub-row
rollup discipline + phase-18.1/18.2 + phase-19.1/19.2 + phase-22.1/22.2/22.3
+ phase-24.1/24.2 precedent).

17 tasks landed. 3 NEW ADRs anchored (ADR-0202 NEW internal/wasm/ framework
primitive — wazero v1.10.1 Apache-2.0 per AMEND-A1 + per-stream VM lifecycle
+ Module compile cache + StrictDefaultDeny sandbox per AMEND-A5 + ABICallbacks
registration interface + in-house proxy-wasm v0.2.1 host ABI headers-bridge
subset + WASI 8-stub custom per R4 + pairs byte-faithful per R3 + bytecode_util
byte-faithful per AMEND-A6 + EXPLICIT API-REVISION ALLOWANCE for consumer #2
per BRAINSTORM Q3+Q4; ADR-0203 NEW internal/filter/http/wasm/ package shape
— 8 prod + 5 test files; compiledConfig + 5-counter filterStats tri-group
prefix per AMEND-A2; 18-arm PARSE-REJECT; 4-arm AsyncDataSource.Local;
24-hostcall + 13-callback surface; ADR-0196 first co-consumer per D-P3 + R7;
fixture-0034 + 0035 dispositions; vendored Rust-sourced .wasm bytecode
under bytecode/ per Q9 + AMEND-A1; ADR-0204 proxy-wasm capability-restriction
default-deny per AMEND-A5 — ~80-key roster; WasmResult::InternalFailure=10
denial semantic + hostcall_denied counter; SanitizationConfig accept-empty
per AMEND-A1 §11.4).
<+ CONDITIONAL ADR-0205 if D-P-PLAN-10 R8 fired: per-module wazero Runtime
pool with pre-instantiated entries — only if Task 17 benchmark surfaced > 1ms
per-stream construction cost>.

34th fuzzer FuzzWasmConfigParse clean at 30s. 37/37 differential fixture
directories GREEN (0000-0035; 7 scenarios full cross-side byte-exact for
fixture-0034 + single-arm boot-reject parity at chosen D-P6 arm on fixture-
0035 via the existing phase-22.1 BootRejectFixture harness). All 6 phase-
done gates GREEN. 25.1 SPEC §15.3 30-item acceptance checklist all GREEN.

EIGHTEENTH and FINAL §9 production HTTP filter sub-phase landed (foundational
third of envelope D; 25.2 advanced bridge + 25.3 per-route + conformance
remain). SECOND occurrence of EXTRACT-NOW-at-first-consumer after phase-22.1
internal/lua/ — anchors NEW internal/wasm/ framework primitive for the
broader §9 WASM host family (cluster-specifier-wasm + access-logger-wasm +
network-filter-wasm + WasmService) per BOOTSTRAP_PROMPT.md §9 line 116;
FIRST §9 row to introduce Apache-2.0-licensed VM-class dep (wazero v1.10.1
pure-Go; CNCF Sandbox; no CGO). 20 HTTP filters wired post-25.1 (was 19).
Stat surface 114 → 119 names per AMEND-A2 tri-group prefix structure
(wasm.wazero.{created,active} Group-B upstream-parity + wasm.<plugin_name>.
{executions,hostcall_denied,envoy_go.failures} envoy-go-strict extensions;
HCM-stats_prefix DROPPED structural note).

Three envoy-go-strict departures documented per parent §13.5 + this 25.1
SPEC §14 edits 3 + 4 + 5: default-deny capability sandbox per AMEND-A5
+ ADR-0204; ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6
(envoy-go-strict-stricter NOT parity); consolidated 4-5-record bundle
(AsyncDataSource.Remote PARSE-REJECT + runtime-name discriminator
PARSE-REJECT + envoy-go-strict counters).

D-S1 RATIFIED at IMPL (34-fuzzer count CONFIRMED). D-P1 closed at Task 2
first-action (proxy_wasm_exports.h:232-249 scrape; WASI denial errno
chosen). D-P2 closed at Task 6 first-action (wasm.cc:298-302 scrape;
5 module-init/allocator callbacks gating disposition). D-P3 closed at
Task 11 first-action (ADR-0196 EncoderFilterCallbacks.ResponseStatus()
FIRST co-consumer RATIFIED — analogous to phase-22.2's first co-consumer
of phase-20 internal/httpclient/). D-P5 closed at Task 9 (18-arm
byte-stable wording pinned; TestParseRejectConstants_ByteStable
enforces). D-P6 closed at Task 16 first-action (boot-reject arm chosen
empirically against upstream Envoy v1.37.2 boot stderr; common substring
chosen).

§13-R1 (fixture-0034 7-scenario cross-side + D6 guardrail compliance),
§13-R2 (ABI v0.1.0+v0.2.0 PARSE-REJECT byte-faithful detection),
§13-R3 (pairs byte-faithful), §13-R4 (WASI 8-stub custom), §13-R5
(34th fuzzer count CLOSED at SPEC §11.1; RATIFIED at IMPL), §13-R7
(ADR-0196 first co-consumer at Task 11 — RATIFIED) — ALL CLOSED.
§13-R6 (ADR-0177 internal/httpclient/ co-consumer) settles at 25.2.
§13-R8 (wazero per-stream Runtime construction benchmark + ADR-0205
escape-valve gate) <STANDS WEAK-default | ADR-0205 escape-valve FIRED>.

ADR-0125 STAYS at 10 canonicals per AMEND-A3 (NO §(xvi) amendment at
25.3 IMPL — REUSE-by-absence is DEFINITIVE). DECISIONS.md tail at
ADR-0204 (or ADR-0205 if R8 fired). Next-free ADR-0205 (or ADR-0206
if R8 fired) — STRENGTHENED-WEAK-HOLD STANDS per parent §1.2.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..24 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 17):
# Edit docs/envoy-go/STATE.md replacing "<TBD — SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 25.1 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-25.1-http-filter-wasm-runtime-and-headers-bridge-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember

- Exact file paths always.
- Complete code shapes are in the 25.1 SPEC §3.1 + §3.5 + §4 + §5 + §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor); the per-Task File-structure table rows + per-Task Step bodies above describe the IMPL surface in implementer-actionable detail.
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 7 or Task 12 wazero concurrent paths), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit per D-P-PLAN-4), `@superpowers:requesting-code-review` (Task 17 REVIEW.md), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 17 + per-Task PROGRESS.md entry quoted command outputs per D-P-PLAN-3).
- DRY, YAGNI, TDD, frequent commits.
