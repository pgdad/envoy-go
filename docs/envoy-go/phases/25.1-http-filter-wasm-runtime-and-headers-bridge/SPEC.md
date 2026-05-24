# Phase 25.1 SPEC — `envoy.extensions.filters.http.wasm.v3.Wasm` (filter scaffold + `internal/wasm/` framework primitive + headers bridge)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `25.1` flips `planned → in-progress` at this SPEC commit (parent row `25` STAYS `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `25.2` + `25.3` STAY `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-09..21 + phase-22.1 + phase-22.2 + phase-22.3 + phase-23 + phase-24.1 + phase-24.2 precedent. This SPEC is the authoritative input to the 25.1 PLAN.

**Parent:** `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4 framework primitive, the **full §11 9-pin empirical-pin block** resolved IN-SESSION at the parent SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy + v1.32.4 go-control-plane proto bindings + `proxy-wasm-cpp-host@da3ce05d` + the proxy-wasm v0.2.1 spec README + `wazero@main` + `proxy-wasm-rust-sdk@v0.2.4`, the §1.1 9-AMEND catalog covering 6 substantive REFUTATIONS + 2 CONFIRMS-with-refinement + 1 RATIFIES-with-extensions, the §6 PARSE-REJECT roster (18 arms at 25.1; +N forward-pointer arms at 25.2 + 25.3), the §7 stat surface (5 counters under tri-group prefix structure per AMEND-A2), the §8 fixture-0034 + fixture-0035 disposition, the §13 8 RATIFIED-PENDING-IMPL items, and the §13.5 6-edit BEHAVIOR_CONTRACT.md bundle anticipation). This sub-phase SPEC details the 25.1 IMPL-task-level surface only; it REFERENCES the parent's §4/§5/§6/§7/§8/§9/§10/§11/§13 rather than repeating them.

**Predecessors:** `docs/envoy-go/phases/25-http-filter-wasm/BRAINSTORM.md` (the 9-Q dialogue + envelope D design rationale + 3 post-synthesis confirmations; the §11 empirical pins are resolved in the parent SPEC §11). NO sub-phase BRAINSTORM (the parent BRAINSTORM + parent SPEC settled enough design — Q1-Q9 locked, AMEND-A1..A9 anchored, parent §13 R1-R8 RATIFIED-PENDING items scoped, parent §15 24-item acceptance checklist drafted — that this 25.1 SPEC proceeds directly to SPEC authoring per the next-prompt-permitted skip + phase-22.1 SPEC precedent which also skipped a sub-phase BRAINSTORM).

**Sub-phase scope (per parent SPEC §3.1 split surface-mapping):** 25.1 lands the envelope-D-foundation core + the NEW `internal/wasm/` framework primitive at first consumer (per BRAINSTORM Q3 + Q4 EXTRACT-NOW; ADR-0202) + the NEW `internal/filter/http/wasm/` package (ADR-0203) + the default-deny capability sandbox (ADR-0204). Specifically:

- `Wasm.config` (sole top-level field per AMEND-A1) CONSUMED;
- `PluginConfig.{name, root_id, vm_config, configuration, capability_restriction_config}` CONSUMED (parent §5.2);
- `PluginConfig.{fail_open, failure_policy=FAIL_RELOAD/FAIL_OPEN, reload_config}` PARSE-REJECT (deferred-to-25.3 per parent §6.2 arms 9 + 10);
- `VmConfig.{vm_id (singleton), runtime ("" or "envoy.wasm.runtime.wazero"), code, configuration}` CONSUMED (parent §5.3);
- `VmConfig.{allow_precompiled, nack_on_code_cache_miss, environment_variables}` PARSE-REJECT (parent §6.2 arms 13 + 14 + 15; envoy-go-strict departures);
- `VmConfig.runtime ∉ {"", "envoy.wasm.runtime.wazero"}` PARSE-REJECT (envoy-go-strict departure per AMEND-A6 + parent §6.2 arm 11);
- multi-plugin VM-sharing via duplicate `vm_id` PARSE-REJECT at 25.1+25.2 (deferred-to-25.3 per parent §6.2 arm 12); singleton vm_id CONSUMED;
- `AsyncDataSource.Local` 4-arm DataSource resolution CONSUMED (Filename + InlineBytes + InlineString + EnvironmentVariable per parent §5.4 + §6.2 arms 3-8); `AsyncDataSource.Remote` PARSE-REJECT (parent §6.2 arm 6; envoy-go-strict departure); `WatchedDirectory` sibling field PARSE-REJECT (parent §6.2 arm 7);
- proxy-wasm v0.2.1 ABI sentinel-export CONSUMED (`proxy_abi_version_0_2_1`); v0.1.0 + v0.2.0 + missing-sentinel PARSE-REJECT (envoy-go-strict-stricter departure per AMEND-A6 + parent §6.2 arm 16);
- wazero `CompileModule` failure PARSE-REJECT (parent §6.2 arm 17);
- per-route `Wasm` 5th-canonical wholesale-override via TPFC PARSE-REJECT (deferred-to-25.3 per parent §6.2 arm 18; NO ADR-0125 amendment per AMEND-A3);
- **25.1 hostcall surface = 24 hostcalls (16 `proxy_*` env-namespace + 8 `wasi_snapshot_preview1.*` custom-shim namespace)** per parent §4.2;
- **25.1 guest-callback surface = 13 callbacks (5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks)** per parent §4.2;
- **Default-deny capability sandbox** per parent §4.3 + AMEND-A5 (envoy-go-strict DEPARTURE from upstream's bare-empty-map-allow-all posture; required because WASM exposes a substantially larger and riskier hostcall surface than Lua + upstream's 3 sandbox runtimes are marked `status: alpha` + `security_posture: unknown`);
- `SanitizationConfig` accept-empty-as-no-op per AMEND-A1 §11.4 + parent §4.3.5 (upstream's `SanitizationConfig` proto is EMPTY and marked "currently unimplemented and ignored, and so should be left empty");
- **5-counter stat surface** per AMEND-A2 + parent §7 (Group-B upstream-parity `wasm.wazero.{created,active}` + envoy-go-strict extensions `wasm.<plugin_name>.{executions, hostcall_denied, envoy_go.failures}`); project stat count **114 → 119**;
- 34th project-wide fuzzer `FuzzWasmConfigParse` (count VERIFIED at this SPEC commit per parent §13-R5 + §13.4 — see §11.1 D-S1 resolution below);
- Differential fixture `0034-http-wasm-headers-bridge` — **7 cross-side scenarios** full byte-exact via existing `CompareBytes` per parent §8.1 + AMEND-A4 + §4.5 D6 guardrails;
- Differential fixture `0035-http-wasm-boot-reject` — single-arm boot-reject parity per parent §8.2 (anticipated arm 5 `vm-config-code-required` per D-P6);
- NEW `BackendKind=HTTPWasm` constant addition at `test/differential/runner_test.go` per parent §8.1.3;
- NEW `bytecode/` + `scripts/` fixture subdirectory layout per parent §8.1.3 + Q9 (vendored Rust-sourced `.wasm` files per `proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target per AMEND-A1);
- 6-edit BEHAVIOR_CONTRACT.md bundle at IMPL final Task per ADR-0052 atomic landing (4-5 envoy-go-strict departure records consolidated: default-deny capability sandbox per AMEND-A5; ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6; `AsyncDataSource.Remote` PARSE-REJECT; runtime-name discriminator PARSE-REJECT; `executions` + `hostcall_denied` + `envoy_go.failures` envoy-go-strict counters per AMEND-A2; HCM-stats_prefix DROPPED structural note per AMEND-A2).

**25.2 (full Envoy↔Wasm advanced bridge delta — body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full stream-info) is OUT OF SCOPE for 25.1.** **25.3 (per-route 5th-canonical wholesale-override + multi-plugin VM-sharing via duplicate `vm_id` + `VmConfig.environment_variables` + `failure_policy = FAIL_RELOAD` + `fail_open` deprecated field + `test/conformance/proxy-wasm/` harness seed at 62.5% pass-threshold) is OUT OF SCOPE for 25.1.**

**ADR continuity:** Phase 24 closed at ADR-0201. Phase 25 parent SPEC anchored §Context drafts for **ADR-0202** (NEW `internal/wasm/` framework primitive) + **ADR-0203** (NEW `internal/filter/http/wasm/` package shape) + **ADR-0204** (default-deny capability sandbox) per ADR-0044 §Context-draft discipline; their §Decision + §Consequences bodies LAND at this 25.1 IMPL's Lands-in-Tasks per ADR-0044 in-place edit discipline (Task 17 atomic landing per §6 + parent SPEC §13.5 + §15 acceptance items 21). **At THIS 25.1 SPEC commit: NO NEW ADRs are consumed** — DECISIONS.md tail STAYS at ADR-0204; next-free ADR STAYS at **ADR-0205** (carried forward as the 25.1 IMPL escape-valve slot per parent SPEC §1.2 STRENGTHENED-WEAK-HOLD-with-1-slot-buffer + §13-R8 RATIFIED-PENDING; anticipated 0-1 consumption from `wazero-VM-pool` benchmark surface per D-P4 + R8).

**Authored:** 2026-05-24.

**Base commit:** `7f7a1e7` (master tip at session entry — `next-prompt.txt: repoint master-tip references to b91bd64 (actual HEAD)`; docs-only). Predecessors: `b91bd64` + `6a72d30` + `a1e4db6` (parent-SPEC SHA-fill + cold-start rewrite for 25.1 SPEC) + `2c1455d` (the parent-SPEC squash-merge commit landing SPEC.md + 3 ADR §Context drafts + STATE.md advance + ROADMAP per-cell SPEC-done annotation).

---

## 1. Purpose / Mission

Phase 25.1 lands the foundational `envoy.extensions.filters.http.wasm.v3.Wasm` filter in **VM + headers-bridge mode** — the canonical Envoy HTTP WebAssembly filter delegating per-request behavior to operator-authored `.wasm` bytecode compiled at config-load and dispatched per-stream into a fresh wazero VM, with the headers-bridge hostcall subset per parent SPEC §4.2 (24 hostcalls: 16 `proxy_*` env-namespace + 8 `wasi_snapshot_preview1.*` custom-shim namespace + 13 guest-exported callbacks: 5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks) — as the foundational third of the EIGHTEENTH and FINAL §9 production HTTP filter (with 25.2 + 25.3 delivering the full envelope D). It establishes the entire `internal/wasm/` framework primitive (wazero VM lifecycle + per-module compile cache + default-deny `SandboxConfig` + ABI-registration interface + the in-house proxy-wasm v0.2.1 host ABI implementation for the headers-bridge hostcall subset + the WASI 8-stub custom implementation + the byte-faithful pairs wire-format reimplementation + the byte-faithful ABI-version detection at `bytecode_util.go`; ADR-0202) + the entire `internal/filter/http/wasm/` package (filter struct + factory + parse + 4-arm AsyncDataSource.Local resolution + ABI callbacks for HTTP filter context + decode/encode headers wiring + stats + 18-arm PARSE-REJECT roster + per-route validator stub; ADR-0203) + the default-deny capability sandbox (per AMEND-A5; ADR-0204). The seven architectural primitives:

1. **NEW `internal/wasm/` framework primitive** — wazero VM lifecycle (`*VM` type + per-stream `*wazero.Runtime` construction + `*Module` compile cache + `SandboxConfig` per-capability ALLOW/DENY discipline + ABI-registration interface + panic-wrapper + log-sink discipline) + the in-house proxy-wasm v0.2.1 host ABI types (`WasmResult` 10-named-value enum with value-gaps at 5/9/11 per AMEND-A7 + `WasmBufferType` 9-named-value enum with value 8 = `FOREIGN_FUNCTION_ARGUMENTS` per AMEND-A7 + `WasmHeaderMapType` 8-value roster + `LogLevel` + `StreamType` + `MetricType` + `ProxyAction` + `WasiErrno`) + the byte-faithful `bytecode_util.go` ABI-version detection (transcribed from `proxy-wasm-cpp-host:bytecode_util.cc:32-97` per AMEND-A6) + the byte-faithful `pairs.go` wire-format reimplementation (transcribed from `proxy-wasm-cpp-host:pairs_util.h` per R3) + the WASI custom 8-stub implementation (per R4 + parent §4.2 — do NOT use wazero's built-in `imports/wasi_snapshot_preview1`). EXTRACT-NOW at first consumer per BRAINSTORM Q3 + Q4 (ENDS the phase-23 + phase-24 ZERO-NEW-package-level-primitive disposition; SECOND occurrence of EXTRACT-NOW-at-first-consumer after phase-22.1 `internal/lua/`). Anchored at ADR-0202 with an EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (the second `internal/wasm/` consumer — cluster-specifier-wasm at `envoy.router.cluster_specifiers.wasm`, access-logger-wasm at `envoy.access_loggers.wasm`, network-filter-wasm at `envoy.filters.network.wasm`, WasmService singleton plugin loaders — whichever materializes first in the broader §9 WASM host family per `BOOTSTRAP_PROMPT.md §9` line 116; the API revision risk at consumer #2 is REDUCED vs phase-22.1's speculative future-consumer roster because the broader §9 WASM host family is explicitly listed at BOOTSTRAP). API surface refined at this SPEC §3.1 + §3.2 production file split + parent SPEC §4.1.

2. **NEW `internal/filter/http/wasm/` package** owning the filter implementation. Package directory + Go-package identifier are both `wasm` at `internal/filter/http/wasm/` (single token; matches `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac`/`lua` precedent — no underscore needed). Files: `doc.go` + `wasm.go` + `compiled_config.go` + `datasource.go` + `abi_callbacks.go` + `decode_headers.go` + `encode_headers.go` + `stats.go` + 5 test files per §3.5 file split. The package exposes `TypeURL` (`"type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"`) + `New` (the `HTTPFilterFactory`) per the cors/fault/.../lua/global_ratelimit precedent. ADR-0203 codifies.

3. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 19 HTTP-filter entries — see §3.6 below for the alphabetical insertion position — before `httpReg.Freeze()`) gains a twentieth `httpReg.Register(wasm.TypeURL, wasm.New)` call before the freeze. Insertion alphabetical per ADR-0100 §2.2 — see §3.6 for the empirically-pinned position. Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only.

4. **`Wasm` proto parsing + 4-arm in-package AsyncDataSource.Local resolution + 18-arm PARSE-REJECT roster** per parent §5 + §6.2 + AMEND-A1 + AMEND-A6. Resolution at config-load: `vm_config.code` AsyncDataSource oneof dispatched across 2 arms (`local` → DataSource 4-arm in-package resolution; `remote` → PARSE-REJECT per arm 6); `DataSource.specifier` oneof dispatched across 4 arms (`Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → byte-cast; `EnvironmentVariable` → `os.LookupEnv`); `WatchedDirectory` sibling field PARSE-REJECTed (deferred to future Runtime/RTDS/hot-reload phase per parent §2.2); empty `specifier` oneof PARSE-REJECTed (PGV-mirror per parent §5.4); resolved bytes fed to wazero `CompileModule` via the `internal/wasm/` primitive's `CompileModule(ctx, src, cache)` API → returns `*Module` cached by sha256 content-hash; ABI-version detection (`bytecode_util.go` byte-faithful reimplementation per AMEND-A6) gates v0.2.1-only acceptance; compile failures surface as PARSE-REJECT arm 17 (`"wasm: config.vm_config.code: compile: %w"` wrapping wazero's compile error). The 18-arm roster lives at parent §6.2; this 25.1 SPEC §7 references it verbatim with byte-stable wording finalization per D-P5.

5. **Headers-bridge hostcall + callback surface** per parent §4.2 + AMEND-A4 (wazero-vs-V8 byte-exact CONFIRMS) + §4.5 D6 guardrails. The 25.1 hostcall surface registered at `internal/wasm/registration.go` exposes 24 hostcalls + 13 guest-callbacks total — see §5 below for the full enumeration; 23 deferred-25.2/25.3 hostcalls registered as stub-returns-Unimplemented per Option B in parent §4.2 (modules importing deferred hostcalls succeed at instantiation but receive `WasmResult::Unimplemented` (=12) when invoked). The HTTP-filter ABI-callbacks Go-side implementation (`internal/filter/http/wasm/abi_callbacks.go`) implements `wasm.ABICallbacks` for the per-stream HTTP context; the framework primitive owns the wazero-side host-module wiring (`internal/wasm/registration.go`) but does not know about HTTP-specific concepts (per the ABI-registration seam discipline).

6. **Filter-callback shape: BOTH `StreamDecoderFilter` AND `StreamEncoderFilter`** (`Decoder: non-nil`; `Encoder: non-nil`). 25.1 mirrors phase-22.1 lua's both-sides shape — `proxy_on_request_headers` fires at `DecodeHeaders`; `proxy_on_response_headers` fires at `EncodeHeaders`. Static blank-identifier compile-time checks for BOTH interfaces. The decode-side surface: `DecodeHeaders(headers, endStream)` constructs the per-stream `*VM` via `wasm.NewVM(opts...)`; calls `vm.Run(ctx, cfg.module, rootContextID)` which: (a) instantiates the `*wazero.Module` onto the new `*wazero.Runtime` (cheap — modules are pre-compiled bytecode); (b) calls the `_initialize` WASI-style module init OR the alternative `_start` per guest's choice; (c) calls `proxy_on_vm_start(rootContextID, vmConfigurationSize)` so the guest module can read `VmConfig.configuration` via `proxy_get_buffer_bytes(VM_CONFIGURATION, ...)` (deferred 25.2; at 25.1 this returns Unimplemented if the guest tries to read VM config buffer — guests using the 25.1-restricted hostcall surface treat the VM-config bytes as ignored); (d) calls `proxy_on_configure(pluginContextID, pluginConfigurationSize)` so the guest can read `PluginConfig.configuration` (same 25.1 caveat); (e) calls `proxy_on_context_create(streamContextID, rootContextID)` to construct the per-stream context; (f) calls `vm.CallProxyOnRequestHeaders(...)` which returns `ProxyAction::CONTINUE` (=0) or `ProxyAction::PAUSE` (=1). After the call returns: if a `proxy_send_local_response` fired during the hook, the filter returns `StopIteration` + `cb.SendLocalReply(status, body, headers)` per the captured local-response state; otherwise CONTINUE → `Continue`; PAUSE → `StopIteration` (returning iteration to the chain when `proxy_continue_stream` lands at 25.2 — at 25.1, PAUSE without an accompanying `proxy_send_local_response` raises an integration error log and falls through to `Continue` because the stream-control hostcalls are deferred). `DecodeData` + `DecodeTrailers` pass-through (no body access at 25.1 per parent §2.11). The encode-side mirrors decode for `proxy_on_response_headers`. `OnDestroy` calls `proxy_on_done(streamContextID)` + `proxy_on_log(streamContextID)` + `proxy_on_delete(streamContextID)` + `vm.Close()` releasing the `*wazero.Runtime`.

7. **Default-deny capability sandbox** per parent §4.3 + AMEND-A5 + ADR-0204. The 25.1 `SandboxConfig` zero-value posture is **StrictDefaultDeny** — empty `allowed_capabilities` ⇒ DENY ALL hostcalls. Operators MUST explicitly enable each capability via `PluginConfig.capability_restriction_config.allowed_capabilities[<capability_name>] = SanitizationConfig{}`. When a hostcall is denied, envoy-go returns `WasmResult::InternalFailure` (=10) + emits an integration error log + increments `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict counter per AMEND-A2). WASI denials use a separate stub returning `WasiErrno::NOTSUP` (=58) OR `WasiErrno::ENOTCAPABLE` (=76, upstream's choice) — see D-P1 resolution at IMPL Task 2. Implementation: `internal/wasm/sandbox.go` materializes the ~80-capability roster (37 core `proxy_*` hostcalls + 7 ABI-versioned hostcalls + 12-of-43 implemented WASI hostcalls + 24 module-getter exports); the host-side hostcall dispatch reads the capability-allow-set + gates the hostcall body. **envoy-go-strict departure record at BEHAVIOR_CONTRACT.md final-Task 6-edit bundle** per parent §13.5 edit #3: the default-deny posture is documented as an envoy-go-strict departure from upstream's bare-empty-map-allow-all posture; rationale: WASM has a substantially larger and riskier hostcall surface than Lua (proxy_call_foreign_function for arbitrary host-side dispatch at 25.2; proxy_dispatch_http_call for outbound network at 25.2; proxy_set_shared_data for cross-stream state at 25.2; proxy_define_metric for unbounded dynamic-stat namespace creation at 25.2); upstream Envoy v1.37.2 marks its 3 sandbox runtimes (V8, WAMR, Wasmtime) as `status: alpha` + `security_posture: unknown` (per `source/extensions/extensions_metadata.yaml:1631-1635`) — the alpha-status posture is incompatible with envoy-go's safe-by-default discipline.

After phase 25.1, the project has the foundational `envoy.filters.http.wasm` filter: a both-decode-and-encode-side filter that constructs a per-stream wazero VM at `DecodeHeaders` entry, loads the listener-config-pre-compiled `*Module`, calls the 13-callback guest-export surface (5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks) against the 24-hostcall in-house proxy-wasm v0.2.1 host ABI implementation (16 `proxy_*` env-namespace + 8 `wasi_snapshot_preview1.*` custom-shim namespace), surfaces module execution errors via panic-wrapper → `wasm.<plugin_name>.envoy_go.failures` counter + log, honors `proxy_send_local_response` with byte-faithful local-reply per parent §4.2, exposes the headers-bridge subset of the proxy-wasm v0.2.1 spec under the default-deny capability sandbox, and is OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the 7 wire-interactive fixture-0034 scenarios — modulo the 4-5 envoy-go-strict documented divergence-windows (default-deny capability sandbox per AMEND-A5; ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6; `AsyncDataSource.Remote` PARSE-REJECT per parent §2.1; runtime-name discriminator PARSE-REJECT per parent §2.3; 3 envoy-go-strict counters per AMEND-A2). Phase 25.2 then activates the full Envoy↔Wasm advanced bridge delta + the body-buffering interaction with ADR-0128 + the `internal/httpclient/` RE-consumer (ADR-0177 forward-pointer validation at second co-consumer scope) + the foreign-function registration interface per AMEND-A9. Phase 25.3 then activates the per-route 5th-canonical wholesale-override (REUSE-by-absence per AMEND-A3; ADR-0125 STAYS at 10 canonicals; NO §(xvi) amendment) + multi-plugin VM-sharing + the conformance harness seed per AMEND-A8 (62.5% pass-threshold against `proxy-wasm-cpp-host@da3ce05d`).

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 9 §1.1 AMENDs in the parent SPEC are the empirical-finding-driven scope revisions for phase 25. The amendments load-bearing for 25.1: **AMEND-A1** (wazero v1.10.1 pin + Apache-2.0 license correction + `wasm32-wasip1` target + proxy-wasm-rust-sdk =0.2.4 + `SanitizationConfig` accept-empty discipline + `allow_precompiled`/`nack_on_code_cache_miss` PARSE-REJECT additions); **AMEND-A2** (stat-roster STRUCTURAL REFUTATION → 5-counter tri-group structure; HCM-stats_prefix DROPPED; no vm_id discriminator); **AMEND-A3** (`WasmPerRoute` absence CONFIRMED; ADR-0125 STAYS at 10; NO §(xvi) amendment — informs §7 per-route classification at 25.3); **AMEND-A4** (wazero-vs-V8 byte-exact CONFIRMS with §4.5 D6 guardrails — directly informs fixture-0034 7-scenario authoring); **AMEND-A5** (default-deny capability roster CONFIRMS envoy-go-strict departure — directly informs §3.3 sandbox roster materialization + ADR-0204); **AMEND-A6** (proxy-wasm v0.1.0 + v0.2.0 PARSE-REJECT envoy-go-strict-stricter departure — directly informs §3.4 hostcall registration + bytecode_util.go byte-faithful detection + parent §6.2 arm 16); **AMEND-A7** (WasmResult 10-value with value-gaps + WasmBufferType FOREIGN_FUNCTION_ARGUMENTS=8 — directly informs §3.2 `abi/types.go` value-faithful encoding); **AMEND-A8** (conformance source REFINED — 25.3 territory, NOT 25.1 surface); **AMEND-A9** (foreign-function disposition RATIFIES option (b) — 25.2 territory, NOT 25.1 surface).

This 25.1 SPEC's §3 / §5 / §7 / §8 / §9 / §10 incorporate the 25.1-relevant amendments. The 25.1 SPEC author makes NO NEW substantive scope revisions vs the parent SPEC; all design decisions inherit cleanly. The 25.1 SPEC's ADDITIVE contributions:

- **D-S1 resolution at SPEC time** (per §11.1 below): 34th-fuzzer count VERIFIED via project-wide grep against master tip. Pins ADR-0203 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch to **34** at IMPL Task 17 atomic landing.
- **Refined `internal/wasm/` API signatures** (per §3.1 + parent §4.1): the parent SPEC §4.1 sketch is provisional; this 25.1 SPEC anchors production signatures with the function-option `VMOption` pattern + `State()` / `HasGlobalFunc()` / `CallGlobal()` split (cleaner than a generic `Run(module, hooks ...HookFn)` blob) — analogous to phase-22.1 ADR-0188 API refinements + `WithPanicHandler` / `WithLogSink` naming clarification.
- **Refined `internal/wasm/` file split** (per §3.2 below): 8 production files in the package root (vm.go + sandbox.go + compile.go + registration.go + bytecode_util.go + pairs.go + wasi.go + doc.go) + 1 subdirectory `abi/` with 1 production file (types.go) + 8 test files. Larger than phase-22.1 `internal/lua/`'s 4-file split because the proxy-wasm v0.2.1 host ABI surface (47 hostcalls + the in-house implementations) is substantially larger than the gopher-lua bridge surface.
- **Boot-registration alphabetical position pinned** (per §3.6 below): empirical position between `router` and (any successor) — see §3.6 for the exact insertion point.

---

## 2. Non-purposes

Phase 25.1 is the first sub-phase of the phase-25 BRAINSTORM-time 3-way pre-split. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land VM + headers-bridge `envoy.filters.http.wasm` under the existing 07.1 framework + the 3 NEW ADRs (ADR-0202 NEW `internal/wasm/` framework primitive + ADR-0203 NEW `internal/filter/http/wasm/` package shape + ADR-0204 default-deny capability sandbox).

- **2.1 Body + buffer hostcalls OUT OF SCOPE.** `proxy_get_buffer_bytes` + `proxy_set_buffer_bytes` + `proxy_get_buffer_status` for `HTTP_REQUEST_BODY` / `HTTP_RESPONSE_BODY` + `proxy_on_request_body` + `proxy_on_response_body` deferred to 25.2 (per parent §2.11). 25.1 modules importing these hostcalls succeed at module-instantiation (stub-returns-Unimplemented per Option B in parent §4.2) but receive `WasmResult::Unimplemented` (=12) when invoked.
- **2.2 Trailer hostcalls + callbacks OUT OF SCOPE.** `proxy_on_request_trailers` + `proxy_on_response_trailers` + the trailer-typed header_map hostcalls deferred to 25.2.
- **2.3 Stream-control hostcalls OUT OF SCOPE.** `proxy_continue_stream` + `proxy_close_stream` deferred to 25.2. At 25.1, `ProxyAction::PAUSE` returned without an accompanying `proxy_send_local_response` raises an integration error log + falls through to `Continue` (see §1 architectural primitive 6).
- **2.4 Timer dispatch OUT OF SCOPE.** `proxy_set_tick_period_milliseconds` + `proxy_on_tick` callback deferred to 25.2 (per parent §2.12).
- **2.5 Metric hostcalls OUT OF SCOPE.** `proxy_define_metric` + `proxy_increment_metric` + `proxy_record_metric` + `proxy_get_metric` + the plugin-defined dynamic-stats namespace `wasmcustom.<custom_name>` deferred to 25.2 (per parent §2.13).
- **2.6 Shared-data hostcalls OUT OF SCOPE.** `proxy_get_shared_data` + `proxy_set_shared_data` deferred to 25.2 (per parent §2.14).
- **2.7 Shared-queue hostcalls + `proxy_on_queue_ready` OUT OF SCOPE.** Deferred to 25.2 with recommend-defer-to-WASM-host-family disposition (per parent §2.15).
- **2.8 Outbound HTTP dispatch OUT OF SCOPE.** `proxy_http_call` + `proxy_on_http_call_response` deferred to 25.2 (per parent §2.16); will RE-CONSUME phase-20 `internal/httpclient/` primitive per ADR-0177 at second co-consumer.
- **2.9 Outbound gRPC dispatch OUT OF SCOPE at all sub-phases.** 5 gRPC hostcalls + 4 gRPC callbacks deferred to WASM host family (per parent §2.17).
- **2.10 Foreign-function default registry OUT OF SCOPE.** Per AMEND-A9 + parent §2.18: `internal/wasm/foreign.go` registration interface lands at 25.2 with EMPTY default registry; the upstream 10 foreign functions (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`) are NOT ported. 25.1 PARSE-REJECT does NOT apply at config-load; the `proxy_call_foreign_function` hostcall stub-returns-Unimplemented at 25.1 + capability-gated by default-deny.
- **2.11 Full stream-info property surface OUT OF SCOPE.** At 25.1, `proxy_get_property` honors only a MINIMAL property tree (`request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host`); the full upstream/downstream/connection/filter_state surface deferred to 25.2 (per parent §3.1 table row "Full stream-info property surface").
- **2.12 Per-route `Wasm` 5th-canonical wholesale-override OUT OF SCOPE.** PARSE-REJECT at any tier (parent §6.2 arm 18 via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint). 25.3 activates with 5th-canonical REUSE-by-absence per AMEND-A3 + ADR-0210 EXPLICIT-NO-NEW-CANONICAL classification ADR.
- **2.13 Multi-plugin VM-sharing OUT OF SCOPE.** At 25.1+25.2 PARSE-REJECT on duplicate `vm_id` (parent §6.2 arm 12). 25.3 activates with ADR-0211 multi-plugin VM-sharing semantics.
- **2.14 `VmConfig.environment_variables` OUT OF SCOPE.** PARSE-REJECT if non-nil (parent §6.2 arm 13). 25.3 activates feeding WASI `environ_*` shims with real data.
- **2.15 `PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig` OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 9). 25.3 activates with the `wasm.<plugin_name>.vm_reload_*` Group-C counter surface per AMEND-A2.
- **2.16 `PluginConfig.fail_open` deprecated bool OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 10). 25.3 activates via the `failure_policy` ladder.
- **2.17 `VmConfig.allow_precompiled` + `VmConfig.nack_on_code_cache_miss` OUT OF SCOPE.** PARSE-REJECT if true (parent §6.2 arms 14 + 15; envoy-go-strict departures). `allow_precompiled` is V8/wasmtime AOT-only (incompatible with wazero's interpreter-default); `nack_on_code_cache_miss` pairs with the `Remote` AsyncDataSource arm (also PARSE-REJECT'd).
- **2.18 Upstream-distinct runtime name discriminators OUT OF SCOPE.** PARSE-REJECT for `envoy.wasm.runtime.{v8,wamr,wasmtime,null}` (parent §6.2 arm 11; envoy-go-strict departure). envoy-go accepts ONLY `""` (defaults to wazero) OR explicit `"envoy.wasm.runtime.wazero"` (envoy-go-strict extension).
- **2.19 proxy-wasm v0.1.0 + v0.2.0 ABI versions OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 16; envoy-go-strict-stricter departure per AMEND-A6). envoy-go-strict targets v0.2.1 exclusively. Detection point: `internal/wasm/bytecode_util.go` byte-faithful reimplementation per AMEND-A6 — scan wasm module export section (type 7) for function-kind export named `proxy_abi_version_0_2_1` (24 UTF-8 ASCII bytes); PARSE-REJECT on any other sentinel value OR on absent sentinel.
- **2.20 `AsyncDataSource.Remote` for `VmConfig.code` OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 6; envoy-go-strict departure). Deferred to a future Runtime / RTDS / hot-reload family phase.
- **2.21 `DataSource.watched_directory` sibling field OUT OF SCOPE.** PARSE-REJECT (parent §6.2 arm 7; deferred to Runtime/hot-reload family).
- **2.22 wazero JIT/AOT compiler backend opt-in OUT OF SCOPE.** wazero defaults to the interpreter; the compiler backend (amd64+arm64 JIT) is opt-in via `wazero.NewRuntimeConfigCompiler()`. Phase 25.1 uses the interpreter default; compiler opt-in is a future ops-tuning phase (per parent §2.7).
- **2.23 Memory-trap fixture scenarios OUT OF SCOPE.** Per parent §4.5 D6 guardrail (a): wazero traps with the Go error string `"out of bounds memory access"`; V8 traps with a different string. Memory-OOM probes DEFERRED.
- **2.24 HTTP/2 header iteration order fixture dependence OUT OF SCOPE.** Per parent §4.5 D6 guardrail (b): fixture-0034 scenarios use HTTP/1.1 OR sort-on-assertion (HPACK reorder divergence between wazero/Go's `net/http.Header` and V8/Envoy's HeaderMap).
- **2.25 Float-formatted log lines OUT OF SCOPE.** Per parent §4.5 D6 guardrail (c): no float-formatted numbers on the 25.1 wire — analogous to gopher-lua-vs-LuaJIT divergence per phase-22 AMEND-9. Flagged for 25.2 fixture scenarios.
- **2.26 Calls to hostcalls outside the 24-hostcall 25.1 surface OUT OF SCOPE for fixture-0034 scenarios.** Per parent §4.5 D6 guardrail (d): 25.1 fixture-0034 scenarios use only the 24-hostcall surface listed in §5; a scenario invoking a deferred hostcall (e.g. `proxy_get_buffer_bytes`) fires the stub-returns-Unimplemented path — the cross-side assertion would diverge on `WasmResult` propagation.
- **2.27 No filter-chain ordering surgery.** wasm registers as one more entry in the existing extension registry; the HCM filter-chain iteration protocol is unchanged.
- **2.28 No wazero VM-pool at 25.1 (WEAK-default per parent §13-R8).** Per-stream wazero `*Runtime` construction with shared per-module `*Module` compile cache. If 25.1 IMPL `BenchmarkPerStreamVM_Construction_Headers` surfaces > 1ms per-stream construction cost, the ADR-0205 escape-valve consumes (per parent §1.2 + §13-R8). Until benchmarked, hold per-stream construction as the WEAK-default. See §13 (RATIFIED-PENDING items).
- **2.29 No `response_code_details` emission** — unchanged from phase-16/17/18/19/20/21/22/23/24; envoy-go's HCM does not surface `response_code_details` to local-reply callers (phase-04 scope). Documented divergence-window joint with prior §9 rows.

---

## 3. Framework primitive (NEW `internal/wasm/` + NEW `internal/filter/http/wasm/`)

Per parent SPEC §4 + BRAINSTORM Q3 + Q4 + AMEND-A1 + AMEND-A4 + AMEND-A5. The 25.1 SPEC refines the parent SPEC §4.1 API sketch into production signatures + concretizes the file split per §3.2 + materializes the default-deny capability roster per §3.3 + concretizes the per-stream VM lifecycle per §3.4 + concretizes the `internal/filter/http/wasm/` file split per §3.5 + pins the boot-registration alphabetical position per §3.6. The 25.1 SPEC author's discretion per the next-prompt-permitted scope:

- The parent SPEC §4.1 sketch is **provisional** ("settled at 25.1 SPEC"). This 25.1 SPEC's §3.1 anchors the production signatures + the cross-cutting discipline (capability roster + per-stream lifecycle + ABI-registration seam + panic-wrapper shape + log-sink discipline).
- Parent SPEC §4.2 (per-stream `*wazero.Runtime` construction + per-module compile cache discipline) STANDS UNCHANGED; this 25.1 SPEC §3.4 implements at Tasks 7-8.
- Parent SPEC §4.3 (default-deny capability sandbox per AMEND-A5) STANDS UNCHANGED; this 25.1 SPEC §3.3 implements at Task 6.
- Parent SPEC §4.4 (NEW `internal/filter/http/wasm/` package file split) STANDS — this 25.1 SPEC §3.5 confirms the file split + extends with the 5-test-file companion.
- Parent SPEC §4.5 (D6 guardrail bit) — UNCHANGED at this 25.1 SPEC commit; the guardrails inform §9 fixture-0034 authoring discipline.

### 3.1 `internal/wasm/` API signatures — refined from parent §4.1 sketch (lands at IMPL Task 1 + Task 7 + Task 8)

Refines parent SPEC §4.1 sketch into production signatures. Key refinements vs parent sketch:

- **`VMOption` pattern is function-option** (`type VMOption func(*VM)`) — idiomatic Go option pattern (matches phase-22.1 ADR-0188 + `internal/jwks/Fetcher` `Option`-pattern precedent + `internal/httpclient/` precedent). Easier to extend at consumer-#2 phases without API revision.
- **`Run(ctx, module, rootContextID)` + separate `HasGlobalFunc(name) + CallProxyOnRequestHeaders(...) + CallProxyOnResponseHeaders(...)` per-callback methods** — cleaner separation of "instantiate module + execute module-init lifecycle (`_initialize` + `proxy_on_vm_start` + `proxy_on_configure` + `proxy_on_context_create` for root context)" from "invoke a named guest-export with args" (the actual per-stream hook firing). The parent SPEC §4.1's `Run` is split here into: `Run(ctx, module, rootContextID)` for root-context lifecycle; per-callback methods for the 25.1 13-callback subset.
- **`State() *wazero.Runtime` exposed** — escape-hatch for the filter consumer to register guest exports or query module-export presence; analogous to phase-22.1's `State() *lua.LState` discipline. Not safe to call after `Close`.
- **`WithPanicHandler` (not `WithPanicRecovery`)** — Go cannot RECOVER from a wazero-side trap mid-Call (wazero's `sys.ExitError` + `wasmruntime.Error` are returned via Call's error path, not via Go panic). The handler is invoked AFTER `recover()` in the Go-side panic wrapper for genuine Go panics (e.g., from a hostcall's Go callback panicking); the wazero-side trap path is handled via the explicit `Run` / `CallProxyOnX` error returns. Naming clarification: "Handler" (not "Recovery") because Go has already `recover()`'d before the user's function runs.
- **`WithLogSink(w io.Writer)`** — redirects `proxy_log` output for the lifetime of this VM. Default nil = drop (no stdout leak; envoy-go-strict default). Naming distinct from upstream Envoy's `LogManager` (which is a process-wide construct); the per-VM sink discipline matches phase-22.1's `WithBasePrintSink` precedent.
- **`CompileCache` nil-tolerant** per ADR-0085 nil-tolerance discipline. `CompileModule(ctx, src, nil)` returns a `*Module` without caching; useful for one-shot compilation paths.
- **`ABICallbacks` interface** at `internal/wasm/registration.go` — consumer registers per-context callbacks via `vm.RegisterABICallbacks(cb)`; the primitive's host-module wiring invokes these on the appropriate hostcall dispatch. The interface at 25.1 carries the headers-bridge subset (per-stream + per-context lifecycle methods); 25.2 + 25.3 extend.

Production signatures (lands at IMPL Task 1 + Task 7 + Task 8):

```go
// internal/wasm/vm.go — VM lifecycle + options + ABI-registration

package wasm

import (
    "context"
    "io"

    "github.com/tetratelabs/wazero"
)

// VM is a per-stream wazero execution context. NOT goroutine-safe.
// Each per-stream filter dispatch constructs a fresh VM via NewVM;
// OnDestroy releases via Close.
type VM struct {
    // unexported fields:
    // runtime  wazero.Runtime
    // module   wazero.CompiledModule (after Run)
    // instance api.Module             (after Run)
    // sandbox  SandboxConfig
    // panicH   PanicHandlerFn
    // logSink  io.Writer
    // cb       ABICallbacks
    // ctxStore map[uint32]*context.Context  // per-streamContextID
}

// VMOption configures VM construction. Function-option pattern.
type VMOption func(*VM)

// WithSandboxConfig sets the per-capability ALLOW/DENY posture. Zero value =
// StrictDefaultDeny (DENY ALL ~80 capability keys per AMEND-A5). See §3.3.
func WithSandboxConfig(sb SandboxConfig) VMOption

// WithPanicHandler sets the Go-panic handler invoked after recover() in
// the VM's panic-wrapper. The handler is invoked with the recovered value.
// NOT for catching wazero-side traps (those return via sys.ExitError or
// wasmruntime.Error from Run/CallProxyOnX); the handler is for genuine Go
// panics from hostcall Go callbacks.
func WithPanicHandler(h PanicHandlerFn) VMOption

// WithLogSink redirects proxy_log output for the lifetime of this VM.
// Default nil = drop (no stdout leak; envoy-go-strict default). Naming
// distinct from upstream's LogManager (process-wide construct); per-VM
// sink discipline matches phase-22.1's WithBasePrintSink precedent.
func WithLogSink(w io.Writer) VMOption

// NewVM constructs a per-stream VM. Applies sandbox config + creates the
// underlying wazero.Runtime + registers the 24-hostcall env-namespace host
// module + the 8-hostcall wasi_snapshot_preview1-namespace custom-shim host
// module + panic-wrapper setup. Caller responsibility: Close at OnDestroy.
// Returns a non-nil *VM.
func NewVM(ctx context.Context, opts ...VMOption) *VM

// State returns the underlying wazero.Runtime. ESCAPE-HATCH for filter
// consumers that need to register additional host modules or query module
// exports beyond the 25.1 surface. Not safe to call after Close.
func (vm *VM) State() wazero.Runtime

// RegisterABICallbacks registers the consumer's per-context callback bundle.
// Lands at 25.1 with the 13-callback subset (5 module-init/allocator + 6
// lifecycle hooks + 2 HTTP hooks); extends at 25.2 + 25.3.
func (vm *VM) RegisterABICallbacks(cb ABICallbacks)

// Run instantiates the module's *wazero.CompiledModule onto this VM's
// wazero.Runtime (cheap — modules are pre-compiled bytecode) and executes
// the module-init lifecycle for the root context: (a) calls _initialize OR
// _start (mutually exclusive per the proxy-wasm v0.2.1 spec); (b) calls
// proxy_on_vm_start(rootContextID, vmConfigurationSize); (c) calls
// proxy_on_configure(rootContextID, pluginConfigurationSize). Per-stream
// contexts are created via CallProxyOnContextCreate. Returns the
// underlying wazero call error on trap or hostcall denial.
func (vm *VM) Run(ctx context.Context, module *Module, rootContextID uint32) error

// HasGlobalFunc returns true if the named guest export is a callable function.
// Used by the filter to check hook-presence after Run (supports module-shape
// PARSE-REJECT if needed — provisional at 25.1; 25.2 may extend).
func (vm *VM) HasGlobalFunc(name string) bool

// CallProxyOnContextCreate invokes proxy_on_context_create(streamContextID,
// rootContextID) — creates a new per-stream context under the root.
func (vm *VM) CallProxyOnContextCreate(ctx context.Context, streamContextID, rootContextID uint32) error

// CallProxyOnRequestHeaders invokes proxy_on_request_headers(streamContextID,
// numHeaders, endOfStream) — returns ProxyAction::CONTINUE (0) or ::PAUSE (1).
func (vm *VM) CallProxyOnRequestHeaders(ctx context.Context, streamContextID, numHeaders uint32, endOfStream bool) (ProxyAction, error)

// CallProxyOnResponseHeaders invokes proxy_on_response_headers(streamContextID,
// numHeaders, endOfStream) — returns ProxyAction::CONTINUE (0) or ::PAUSE (1).
func (vm *VM) CallProxyOnResponseHeaders(ctx context.Context, streamContextID, numHeaders uint32, endOfStream bool) (ProxyAction, error)

// CallProxyOnDone invokes proxy_on_done(streamContextID) → bool.
// Returning false defers finalize (host returns CONTINUE on the wire).
func (vm *VM) CallProxyOnDone(ctx context.Context, streamContextID uint32) (bool, error)

// CallProxyOnLog invokes proxy_on_log(streamContextID).
func (vm *VM) CallProxyOnLog(ctx context.Context, streamContextID uint32) error

// CallProxyOnDelete invokes proxy_on_delete(streamContextID).
func (vm *VM) CallProxyOnDelete(ctx context.Context, streamContextID uint32) error

// Close releases the VM's wazero.Runtime. Idempotent.
func (vm *VM) Close() error

// PanicHandlerFn is invoked after recover() in the VM's panic-wrapper.
// recovered is the value returned by recover() (typically the panic value).
type PanicHandlerFn func(recovered any)
```

```go
// internal/wasm/compile.go — Module + CompileCache

package wasm

import (
    "context"

    "github.com/tetratelabs/wazero"
)

// Module wraps a compiled *wazero.CompiledModule + the detected ABI version.
// Safe for cross-VM reuse.
type Module struct {
    // unexported fields:
    // compiled wazero.CompiledModule
    // abiVer   AbiVersion  // currently always ProxyAbi_0_2_1 (others PARSE-REJECTed)
    // hash     [32]byte    // sha256(src) — for cache identity
}

// CompileCache is a content-addressed compile cache, keyed by sha256(src).
// Owned by the *compiledConfig (filter-config-instance scope); GC-driven
// eviction (no manual evict; cache lifetime == compiledConfig lifetime).
// Safe for concurrent read/add via internal sync.RWMutex.
type CompileCache struct {
    // unexported fields:
    // mu    sync.RWMutex
    // store map[[32]byte]*Module
    // rt    wazero.Runtime  // shared compile-only runtime (separate from
    //                         the per-stream runtimes; compile is expensive)
}

// NewCompileCache constructs an empty cache.
func NewCompileCache(ctx context.Context) *CompileCache

// CompileModule compiles src via wazero's parser + the byte-faithful
// bytecode_util.go ABI-version detection (per AMEND-A6). If cache is
// non-nil, caches by sha256(src). If cache is nil, compiles uncached
// (per ADR-0085 nil-tolerance discipline). Returns wrapped wazero compile
// error on failure (PARSE-REJECT arm 17 wording) OR
// ErrUnsupportedAbiVersion if the module's ABI sentinel is not
// proxy_abi_version_0_2_1 (PARSE-REJECT arm 16 wording).
func CompileModule(ctx context.Context, src []byte, cache *CompileCache) (*Module, error)

// AbiVersion is the proxy-wasm ABI version sentinel detected from the
// module's export section (per AMEND-A6).
type AbiVersion int

const (
    AbiVersionUnknown AbiVersion = iota
    AbiVersion_0_1_0
    AbiVersion_0_2_0
    AbiVersion_0_2_1  // the only version envoy-go accepts at 25.1
)

// Close releases the compile-only wazero.Runtime owned by the cache.
// Idempotent.
func (c *CompileCache) Close() error
```

```go
// internal/wasm/sandbox.go — SandboxConfig + per-capability gate

package wasm

// SandboxConfig governs which hostcalls are allowed at the gate.
// Zero value = StrictDefaultDeny per AMEND-A5 + ADR-0204 (envoy-go-strict
// DEPARTURE from upstream's bare-empty-map-allow-all posture).
type SandboxConfig struct {
    // AllowedCapabilities is the set of hostcall keys (bare names) the
    // guest is permitted to invoke. Empty set ⇒ deny ALL hostcalls per
    // AMEND-A5 (departure from upstream which allows ALL when the map
    // is empty). See §3.3 for the full ~80-key roster.
    AllowedCapabilities map[string]SanitizationConfig
}

// SanitizationConfig is the per-capability sanitization rules. Per
// AMEND-A1 + parent §4.3.5: upstream's SanitizationConfig proto is EMPTY
// and marked "currently unimplemented and ignored, and so should be left
// empty". envoy-go matches byte-faithfully — accept empty value;
// ignore non-empty (parse-and-discard mirrors phase-24's INERT
// acceptance discipline).
type SanitizationConfig struct {
    // No fields. Reserved for future per-capability sanitization rules
    // if upstream lands them.
}

// IsAllowed returns true if the named capability is in AllowedCapabilities.
// At the strict default-deny posture (empty map), returns false for all
// keys — INVERTS upstream's empty-map-allow-all semantic per AMEND-A5.
func (sb *SandboxConfig) IsAllowed(capabilityName string) bool
```

```go
// internal/wasm/registration.go — ABICallbacks interface + host-module wiring

package wasm

import "context"

// ABICallbacks is the consumer-side interface the framework primitive
// invokes on the appropriate hostcall dispatch. The HTTP-filter package
// (internal/filter/http/wasm/) implements this interface for the per-stream
// HTTP context shape; cluster-specifier-wasm + access-logger-wasm + the
// other broader §9 WASM host family consumers will implement their own
// per-context shape variants per the EXPLICIT API-REVISION ALLOWANCE
// clause anchored at ADR-0202.
//
// At 25.1 the interface carries the 13-callback subset (lifecycle +
// HTTP hooks); 25.2 + 25.3 extend.
type ABICallbacks interface {
    // GetHeaderMap fetches the named header-map for the given context.
    // Returns sorted key/value pairs (for cross-side determinism per
    // parent §4.5 D6 guardrail (b)) + ok=false if the header-map type
    // is not available in this context.
    GetHeaderMap(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType) (pairs []HeaderPair, ok bool)

    // GetHeaderMapValue fetches a single value from the named header-map.
    GetHeaderMapValue(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType, key string) (value string, ok bool)

    // AddHeaderMapValue appends a value to the named header-map.
    AddHeaderMapValue(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType, key, value string) WasmResult

    // ReplaceHeaderMapValue replaces a value in the named header-map.
    ReplaceHeaderMapValue(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType, key, value string) WasmResult

    // RemoveHeaderMapValue removes a key from the named header-map.
    RemoveHeaderMapValue(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType, key string) WasmResult

    // SetHeaderMapPairs replaces all pairs in the named header-map.
    SetHeaderMapPairs(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType, pairs []HeaderPair) WasmResult

    // GetHeaderMapSize returns the number of pairs in the named header-map.
    GetHeaderMapSize(ctx context.Context, streamContextID uint32, mapType WasmHeaderMapType) uint32

    // GetProperty fetches a property by path. Minimal property tree at
    // 25.1 (request.headers.*, response.headers.*, request.path,
    // request.method, request.host); 25.2 extends to full surface.
    GetProperty(ctx context.Context, streamContextID uint32, path []string) (value []byte, ok bool)

    // SetProperty stores a property by path.
    SetProperty(ctx context.Context, streamContextID uint32, path []string, value []byte) WasmResult

    // SendLocalResponse short-circuits to local-reply with the given
    // status + headers + body. Consumed at REUSE point 5 (parent §3.3).
    SendLocalResponse(ctx context.Context, streamContextID uint32, statusCode uint32, statusMsg, body string, additionalHeaders []HeaderPair, grpcStatus int32) WasmResult

    // GetStatus reads the HTTP/gRPC status. Consumed inside
    // proxy_on_response_headers via ADR-0196's
    // EncoderFilterCallbacks.ResponseStatus() accessor (per D-P3 +
    // R7). Anticipated first co-consumer of phase-23's primitive.
    GetStatus(ctx context.Context, streamContextID uint32) (statusCode uint32, value []byte, ok bool)

    // Log emits a log line at the given level. Routes to the VM's
    // WithLogSink output.
    Log(ctx context.Context, streamContextID uint32, level LogLevel, msg string)

    // GetLogLevel returns the active log level (v0.2.1-new hostcall).
    GetLogLevel(ctx context.Context) LogLevel

    // GetCurrentTimeNanoseconds returns the wall-clock time. DEPRECATED
    // in v0.2.1 (guests are encouraged to use
    // wasi_snapshot_preview1.clock_time_get); implemented at 25.1 for
    // upstream byte-faithfulness.
    GetCurrentTimeNanoseconds(ctx context.Context) uint64

    // SetEffectiveContext switches the active context for the calling
    // VM. Used by timer + httpCall callbacks (25.2 territory) but
    // available at 25.1 for completeness.
    SetEffectiveContext(ctx context.Context, contextID uint32) WasmResult

    // Done signals the guest is done with the named context.
    Done(ctx context.Context, contextID uint32) WasmResult
}

// HeaderPair is a single (key, value) pair in a header-map serialization.
type HeaderPair struct {
    Key   string
    Value string
}
```

```go
// internal/wasm/abi/types.go — proxy-wasm v0.2.1 enum types
// (value-faithful encoding per AMEND-A7)

package abi

// WasmResult is the host→guest result code. 10 named values with value-gaps
// at positions 5, 9, 11 (per AMEND-A7 — the gaps MUST be preserved
// byte-faithfully because guest modules check specific integer values and
// remapping would break wire compatibility).
type WasmResult int32

const (
    WasmResultOk                   WasmResult = 0
    WasmResultNotFound             WasmResult = 1
    WasmResultBadArgument          WasmResult = 2
    WasmResultSerializationFailure WasmResult = 3
    WasmResultParseFailure         WasmResult = 4
    // gap at 5 — BadExpression in BRAINSTORM hypothesis; does NOT exist in v0.2.1
    WasmResultInvalidMemoryAccess  WasmResult = 6
    WasmResultEmpty                WasmResult = 7
    WasmResultCasMismatch          WasmResult = 8
    // gap at 9 — ResultMismatch in BRAINSTORM hypothesis; does NOT exist in v0.2.1
    WasmResultInternalFailure      WasmResult = 10
    // gap at 11 — BrokenConnection in BRAINSTORM hypothesis; does NOT exist in v0.2.1
    WasmResultUnimplemented        WasmResult = 12
)

// WasmBufferType identifies the buffer the guest is reading/writing.
// Value 8 = FOREIGN_FUNCTION_ARGUMENTS (NOT CallData as BRAINSTORM
// hypothesized; per AMEND-A7).
type WasmBufferType int32

const (
    WasmBufferTypeHttpRequestBody       WasmBufferType = 0
    WasmBufferTypeHttpResponseBody      WasmBufferType = 1
    WasmBufferTypeDownstreamData        WasmBufferType = 2
    WasmBufferTypeUpstreamData          WasmBufferType = 3
    WasmBufferTypeHttpCallResponseBody  WasmBufferType = 4
    WasmBufferTypeGrpcReceiveBuffer     WasmBufferType = 5
    WasmBufferTypeVmConfiguration       WasmBufferType = 6
    WasmBufferTypePluginConfiguration   WasmBufferType = 7
    WasmBufferTypeForeignFunctionArguments WasmBufferType = 8 // NOT CallData
)

// WasmHeaderMapType identifies the header-map the guest is reading/writing.
type WasmHeaderMapType int32

const (
    WasmHeaderMapTypeHttpRequestHeaders         WasmHeaderMapType = 0
    WasmHeaderMapTypeHttpRequestTrailers        WasmHeaderMapType = 1
    WasmHeaderMapTypeHttpResponseHeaders        WasmHeaderMapType = 2
    WasmHeaderMapTypeHttpResponseTrailers       WasmHeaderMapType = 3
    WasmHeaderMapTypeHttpCallResponseHeaders    WasmHeaderMapType = 4
    WasmHeaderMapTypeHttpCallResponseTrailers   WasmHeaderMapType = 5
    WasmHeaderMapTypeGrpcReceiveInitialMetadata WasmHeaderMapType = 6
    WasmHeaderMapTypeGrpcReceiveTrailingMetadata WasmHeaderMapType = 7
)

// LogLevel is the proxy_log severity.
type LogLevel int32

const (
    LogLevelTrace    LogLevel = 0
    LogLevelDebug    LogLevel = 1
    LogLevelInfo     LogLevel = 2
    LogLevelWarn     LogLevel = 3
    LogLevelError    LogLevel = 4
    LogLevelCritical LogLevel = 5
)

// ProxyAction is the guest→host action on proxy_on_request_headers /
// proxy_on_response_headers callbacks.
type ProxyAction int32

const (
    ProxyActionContinue ProxyAction = 0
    ProxyActionPause    ProxyAction = 1
)

// WasiErrno is the WASI errno return code for the 8 WASI shim hostcalls.
// Partial roster (only the values 25.1 surface actually uses); full
// roster lives in proxy-wasm spec v0.2.1.
type WasiErrno int32

const (
    WasiErrnoSuccess     WasiErrno = 0
    WasiErrnoBadf        WasiErrno = 8
    WasiErrnoInval       WasiErrno = 28
    WasiErrnoNotsup      WasiErrno = 58
    WasiErrnoNotcapable  WasiErrno = 76 // upstream's choice for capability denial; see D-P1
)
```

### 3.2 `internal/wasm/` file split

8 production files in the package root + 1 subdirectory `abi/` with 1 production file + 8 test files (larger than phase-22.1 `internal/lua/` because the proxy-wasm v0.2.1 host ABI surface is substantially larger):

```
internal/wasm/
  doc.go              # package overview + Q1-Q9 BRAINSTORM decision summary +
                      # AMEND-A1..A9 cross-references + API surface summary +
                      # wazero v1.10.1 + proxy-wasm v0.2.1 ABI pin
  vm.go               # VM type + NewVM + VMOption + State + Run + HasGlobalFunc +
                      # CallProxyOnContextCreate + CallProxyOnRequestHeaders +
                      # CallProxyOnResponseHeaders + CallProxyOnDone + CallProxyOnLog +
                      # CallProxyOnDelete + Close + panic-wrapper +
                      # WithSandboxConfig + WithPanicHandler + WithLogSink
  compile.go          # Module + CompileCache + NewCompileCache + CompileModule +
                      # AbiVersion enum + sha256-keyed caching + sync.RWMutex
                      # discipline + ErrUnsupportedAbiVersion sentinel
  sandbox.go          # SandboxConfig type + SanitizationConfig stub + IsAllowed +
                      # the ~80-key capability roster constants (per §3.3)
  registration.go     # ABICallbacks interface + HeaderPair + the host-module
                      # wiring (registers the 16 proxy_* env-namespace hostcalls +
                      # the 8 wasi_snapshot_preview1.* custom-shim hostcalls onto
                      # the VM's wazero.Runtime)
  bytecode_util.go    # ABI-version detection via byte-faithful reimplementation
                      # of proxy-wasm-cpp-host:bytecode_util.cc:32-97 (per AMEND-A6) —
                      # scans wasm module export section type 7 for the
                      # proxy_abi_version_0_2_1 sentinel
  pairs.go            # serialized pairs wire format byte-faithful reimplementation
                      # of proxy-wasm-cpp-host:pairs_util.h (per R3 +
                      # parent §13-R3) — u32 num_pairs / u32 key_len, u32 value_len /
                      # key_bytes NUL value_bytes NUL
  wasi.go             # custom 8-stub WASI implementation (per R4 + parent §13-R4) —
                      # fd_write routes to proxy_log; proc_exit traps; environ_*
                      # returns zeros at 25.1; args_* returns zeros; clock_time_get +
                      # random_get implemented at host accuracy
  abi/types.go        # WasmResult (10 named values with value-gaps at 5/9/11 per
                      # AMEND-A7) + WasmBufferType (9 values; value 8 =
                      # FOREIGN_FUNCTION_ARGUMENTS per AMEND-A7) + WasmHeaderMapType +
                      # LogLevel + ProxyAction + WasiErrno (subset roster)

  vm_test.go              # VM lifecycle + options + Run + per-callback methods + Close +
                          # panic-wrapper behavior tests
  compile_test.go         # NewCompileCache + CompileModule + cache-hit-on-same-content +
                          # cache-miss-on-different + concurrent read/add tests +
                          # AbiVersion detection tests (v0.2.1 + v0.1.0 + v0.2.0 +
                          # missing-sentinel) per AMEND-A6
  sandbox_test.go         # per-capability ALLOW/DENY exhaustive tests; verifies the
                          # default-deny posture per AMEND-A5; ~80-key roster coverage
  registration_test.go    # ABICallbacks interface invocation + host-module wiring
                          # round-trip tests (guest invokes hostcall → ABICallbacks
                          # method fires → result returns to guest)
  bytecode_util_test.go   # byte-faithful ABI-version detection tests against
                          # crafted-wasm-binary fixtures (~5-10 binaries: valid
                          # v0.2.1 + v0.1.0 + v0.2.0 + Unknown + malformed export
                          # section + truncated module)
  pairs_test.go           # pairs wire format byte-faithful serialization +
                          # deserialization round-trip tests + golden-bytes table
                          # (per R3 + the cpp-host pairs_util.cc round-trip oracle)
  wasi_test.go            # WASI 8-stub shim unit tests per R4 (fd_write →
                          # proxy_log routing; proc_exit traps; clock_time_get +
                          # random_get; environ_* + args_* zero returns; bad-fd
                          # WasiErrno::BADF returns)
  abi/types_test.go       # WasmResult / WasmBufferType / etc. value-faithful
                          # encoding tests per AMEND-A7 (value-gap preservation
                          # critical — guest modules check specific integer values)
```

### 3.3 Default-deny capability roster — materialized at sandbox.go per parent §4.3 + AMEND-A5 + ADR-0204

The 25.1 `SandboxConfig` zero-value posture is **StrictDefaultDeny** per AMEND-A5. The roster table per parent §4.3 STANDS UNCHANGED; reproduced here for in-place readability with the 25.1 implementation discipline:

**~80-capability roster** (37 core `proxy_*` hostcalls + 7 ABI-versioned hostcalls + 12-of-43 implemented WASI hostcalls + 24 module-getter exports). At 25.1, the materialized subset is the 24 hostcalls actually registered (per §5) + the 13 module-getter capability keys (gated at `getFunction` time per `proxy-wasm-cpp-host:wasm.h:274-282`). Operators may explicitly enable capabilities from the 25.1 surface via `PluginConfig.capability_restriction_config.allowed_capabilities`:

- **Headers-bridge family** (7 keys): `proxy_get_header_map_pairs`, `proxy_set_header_map_pairs`, `proxy_get_header_map_value`, `proxy_add_header_map_value`, `proxy_replace_header_map_value`, `proxy_remove_header_map_value`, `proxy_get_header_map_size`.
- **Local-response** (1 key): `proxy_send_local_response`.
- **Property** (2 keys): `proxy_get_property`, `proxy_set_property`.
- **Log** (2 keys): `proxy_log`, `proxy_get_log_level`.
- **Status** (1 key): `proxy_get_status`.
- **Time** (1 key): `proxy_get_current_time_nanoseconds`.
- **Context-lifecycle** (2 keys): `proxy_set_effective_context`, `proxy_done`.
- **WASI** (8 keys; bare names — no `proxy_` prefix): `fd_write`, `clock_time_get`, `random_get`, `environ_sizes_get`, `environ_get`, `args_sizes_get`, `args_get`, `proc_exit`.
- **Module-init / allocator** (5 keys; the gating disposition pending D-P2 resolution at IMPL Task 6): `_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate`.
- **Lifecycle + HTTP callback module-getters** (8 keys; gated at `getFunction` time): `proxy_on_context_create`, `proxy_on_vm_start`, `proxy_on_configure`, `proxy_on_done`, `proxy_on_delete`, `proxy_on_log`, `proxy_on_request_headers`, `proxy_on_response_headers`.

**Implementation discipline at IMPL Task 6:** `internal/wasm/sandbox.go` materializes the roster as a set of package-private string constants (`capability_proxy_log = "proxy_log"`, etc.) + a `SandboxConfig.IsAllowed(name)` method that inverts the upstream empty-map-allow-all semantic. The host-side hostcall dispatch reads `vm.sandbox.IsAllowed(capabilityName)` before invoking the ABICallbacks method; denied calls return `WasmResult::InternalFailure` (=10) + emit an integration error log + increment `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict counter per AMEND-A2). WASI denials use `WasiErrno::NOTSUP` (=58) OR `WasiErrno::ENOTCAPABLE` (=76) per D-P1 — see §12 D-P1 resolution at IMPL Task 2 first-action.

**`SanitizationConfig` accept-empty discipline** (per AMEND-A1 + parent §4.3.5): upstream's `SanitizationConfig` proto is EMPTY (no fields) + upstream marks it "currently unimplemented and ignored, and so should be left empty" (`source/extensions/common/wasm/plugin.cc:14`). envoy-go matches upstream byte-faithfully — accept empty `SanitizationConfig{}` values; ignore non-empty (parse-and-discard mirrors phase-24's `override_option` INERT acceptance per AMEND-4). NO PARSE-REJECT on non-empty SanitizationConfig; NO IMPL surface beyond the accept-empty discipline.

**envoy-go-strict departure record at IMPL Task 17** (per parent §13.5 edit #3): default-deny capability sandbox departure recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures section. Departure-record count 18 → 19 (or 18 → 22-23 with the consolidated 4-5-record bundle per parent §13.5 edit #5).

### 3.4 Per-stream `*wazero.Runtime` construction + per-module `*Module` compile cache (per parent §4.2 + §11.5)

Per parent §4.2 + §11.5 D4. The 25.1 WEAK-default: per-stream `*wazero.Runtime` construction with shared per-module `*wazero.CompiledModule` compile cache. Each per-stream invocation:

1. The filter's `*compiledConfig` (built at config-load via `buildCompiledConfig` per §4.2) carries the pre-compiled `*Module` (compiled once via `wasm.CompileModule(ctx, src, cfg.compileCache)`).
2. At `DecodeHeaders` entry, the filter calls `vm := wasm.NewVM(ctx, opts...)`, constructing a fresh `*wazero.Runtime` with the compiler backend (interpreter at 25.1 per parent §2.7; compiler opt-in deferred) + the 16-hostcall env-namespace host module + the 8-hostcall `wasi_snapshot_preview1`-namespace custom-shim host module + the sandbox capability roster applied.
3. The filter calls `vm.RegisterABICallbacks(cb)` registering the per-stream HTTP-filter context bundle.
4. The filter calls `vm.Run(ctx, cfg.module, rootContextID)` which: (a) instantiates the `*wazero.CompiledModule` onto the new `*wazero.Runtime` (cheap — modules are pre-compiled bytecode); (b) calls `_initialize` (the WASI-style module init) OR `_start` (alt module init) per guest's choice — guest exports exactly one of the two; (c) calls `proxy_on_vm_start(rootContextID, vmConfigurationSize)` so the guest can read `VmConfig.configuration` via `proxy_get_buffer_bytes(VM_CONFIGURATION, ...)` (deferred 25.2 — at 25.1 the hostcall stub-returns Unimplemented; guests using 25.1-restricted surface ignore VM-config bytes); (d) calls `proxy_on_configure(rootContextID, pluginConfigurationSize)` so the guest can read `PluginConfig.configuration` (same 25.1 caveat).
5. The filter calls `vm.CallProxyOnContextCreate(ctx, streamContextID, rootContextID)` constructing the per-stream context.
6. The filter calls `vm.CallProxyOnRequestHeaders(ctx, streamContextID, headerCount, endOfStream)` which returns `ProxyAction::CONTINUE` (=0) or `ProxyAction::PAUSE` (=1).
7. If `proxy_send_local_response` fired during the hook: filter returns `StopIteration` + `cb.SendLocalReply(captured_status, captured_body, captured_headers)`. Otherwise `CONTINUE` → `Continue`; `PAUSE` without local-response → integration error log + fall-through `Continue` (stream-control hostcalls deferred to 25.2).
8. `cfg.stats.executions++` after CallProxyOnRequestHeaders regardless of outcome (matches the AMEND-A2 envoy-go-strict counter discipline).
9. If CallProxyOnRequestHeaders returned an error (wazero trap or hostcall-denial chain): `cfg.stats.envoy_go.failures++` + log via the configured sink + continue (NO wire-side error surface — wazero traps don't terminate the stream at 25.1; the trap aborts the module dispatch + the filter falls through to `Continue`).
10. The encode-side mirrors decode for `proxy_on_response_headers` + per-stream context handling.
11. At `OnDestroy`: the filter calls `vm.CallProxyOnDone(streamContextID)` + `vm.CallProxyOnLog(streamContextID)` + `vm.CallProxyOnDelete(streamContextID)` + `vm.Close()` releasing the `*wazero.Runtime` (releases the per-stream runtime + linear memory + the compiled-module reference).

**wazero-VM-pool design (ESCAPE-VALVE-CANDIDATE per parent §1.2 + §13-R8):** if 25.1 IMPL benchmarks of per-stream construction cost surface unacceptable overhead (e.g., > 1ms per stream at the headers-only bridge surface), the ADR-0205 escape-valve slot anchors a "per-module wazero Runtime pool with pre-instantiated entries" decision. Until benchmarked, hold per-stream construction as the WEAK-default. The phase-22.1 `*LState`-pool benchmark observed 70µs per construction (well under threshold); wazero compiler-mode initialization is comparable or faster (per parent §1.2 hypothesis). See §13 R8 (RATIFIED-PENDING items).

### 3.5 `internal/filter/http/wasm/` file split (per parent §4.4 STANDS + 5-test-file extension)

Parent SPEC §4.4 file split STANDS UNCHANGED at this 25.1 SPEC; this SPEC extends with the 5-test-file companion per the phase-22.1 SPEC §3.5 precedent. 8 production files + 5 test files:

```
internal/filter/http/wasm/
  doc.go                  # package overview + Q1-Q9 BRAINSTORM decision summary +
                          # AMEND-A1..A9 cross-references + D-P1..D-P6 cross-refs +
                          # API surface summary (filterStats, compiledConfig, TypeURL)
  wasm.go                 # filter struct + factory (HTTPFilterFactory) + filterStats +
                          # TypeURL + filterName + per-route validator registration
  compiled_config.go      # config parse + 18-arm PARSE-REJECT roster per parent §6.2 +
                          # module-compile cache key generation + ABI-version detection
                          # gating (CONSUMED only proxy_abi_version_0_2_1 per AMEND-A6)
  datasource.go           # 4-arm AsyncDataSource.Local arm resolution (Filename +
                          # InlineBytes + InlineString + EnvironmentVariable) +
                          # WatchedDirectory PARSE-REJECT + Remote PARSE-REJECT +
                          # empty-oneof PARSE-REJECT
  abi_callbacks.go        # implements wasm.ABICallbacks for the per-stream HTTP-filter
                          # context — 7 header-map methods + GetProperty (minimal tree) +
                          # SetProperty + SendLocalResponse (consumes REUSE 5) +
                          # GetStatus (consumes ADR-0196 EncoderFilterCallbacks.ResponseStatus
                          # per D-P3 + R7) + Log (routes to filter log sink) + GetLogLevel +
                          # GetCurrentTimeNanoseconds + SetEffectiveContext + Done
  decode_headers.go       # DecodeHeaders implementation + vm.NewVM construction + Run +
                          # CallProxyOnContextCreate + CallProxyOnRequestHeaders +
                          # ProxyAction handling + captured-local-response handoff
  encode_headers.go       # EncodeHeaders implementation + CallProxyOnResponseHeaders +
                          # ProxyAction handling + captured-local-response handoff
  stats.go                # 5-counter stat surface per AMEND-A2 (Group B wasm.wazero.created +
                          # wasm.wazero.active gauge + 3 envoy-go-strict counters
                          # wasm.<plugin_name>.{executions,hostcall_denied,envoy_go.failures})
                          # — tri-group prefix structure; HCM-stats_prefix DROPPED

  wasm_test.go            # filter + factory + filterStats + decode/encode wiring +
                          # per-stream VM lifecycle integration (~1500-2000 LoC)
  compiled_config_test.go # 18-arm PARSE-REJECT table-driven tests per parent §6.2
  datasource_test.go      # 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT +
                          # empty-oneof PARSE-REJECT + file-read failure paths
  abi_callbacks_test.go   # all 13-callback subset round-trip tests + minimal property tree
                          # exhaustive coverage + GetStatus ADR-0196 first co-consumer
                          # round-trip + sandbox-deny dispatch for each capability key
  fuzz_test.go            # 34th project-wide fuzzer FuzzWasmConfigParse with ~30 corpus
                          # seeds covering all 18 PARSE-REJECT arms + valid configs +
                          # adversarial wasm bytecode (must-never-panic invariant)
```

### 3.6 Boot-registration alphabetical position (per ADR-0100 §2.2 + parent §4.4)

Per ADR-0100 §2.2 stylistic discipline. `cmd/envoy-go/main.go` currently registers 19 HTTP-filter entries before `httpReg.Freeze()` — at master tip the alphabetical roster is: `adaptive_concurrency`, `admissioncontrol`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `extauthz`, `extproc`, `fault`, `globalratelimit`, `header_mutation`, `jwtauthn`, `localratelimit`, `lua`, `oauth2`, `rbac`, `router`. The 20th entry `wasm.New` inserts alphabetically **between `router` and the freeze** (no entry after `router` at master tip; `wasm` sorts after `router` so it appends to the tail of the registration block). Insertion line is the line immediately before `httpReg.Freeze()`. Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only. **D-P-AUX**: verify the alphabetical roster at IMPL Task 14 first-action (grep `httpReg.Register` at `cmd/envoy-go/main.go`); if a successor row landed between this SPEC and IMPL, adjust the insertion line accordingly.

---

## 4. compiledConfig + code shapes

### 4.1 Public surface

```go
package wasm

import (
    "github.com/esalaine/envoy-go/internal/filter/http"  // for HTTPFilterFactory
)

// TypeURL is the canonical Envoy type-URL for the Wasm HTTP filter config.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"

// filterName is the canonical Envoy filter name.
const filterName = "envoy.filters.http.wasm"

// New is the HTTPFilterFactory registered at boot per ADR-0072.
func New(ctx http.FilterFactoryContext) (http.FilterInstanceFactory, error)
```

`New` parses + validates the `Wasm` proto into a `*compiledConfig`, allocates the `filterStats`, compiles the resolved AsyncDataSource.Local bytes via `wasm.CompileModule` (gating ABI version via `bytecode_util.go` byte-faithful detection per AMEND-A6), constructs the per-config `SandboxConfig` from `PluginConfig.capability_restriction_config`, registers the per-route validator per parent §6.2 arm 18, and returns a `FilterInstanceFactory` closure that produces per-stream `*filter` values. Mirrors the cors/.../lua/global_ratelimit factory shape.

### 4.2 `compiledConfig` + `filterStats` shape

```go
package wasm

import (
    "github.com/esalaine/envoy-go/internal/stats"
    "github.com/esalaine/envoy-go/internal/wasm"
)

// compiledConfig is the immutable post-parse listener-level config.
type compiledConfig struct {
    module       *wasm.Module          // pre-compiled wasm bytecode (single module at 25.1; multi-plugin lands at 25.3)
    compileCache *wasm.CompileCache    // module-cache holder (kept alive for compiledConfig lifetime; GC-driven eviction)
    sandbox      wasm.SandboxConfig    // from PluginConfig.capability_restriction_config; zero-value = StrictDefaultDeny per AMEND-A5
    pluginName   string                // from PluginConfig.name (Group-C stat-prefix discriminator per AMEND-A2)
    rootContextID uint32               // plugin-context discriminator from PluginConfig.root_id; allocated as a u32 counter at config-load (NOT the string root_id directly — the string is mapped to a u32 ID via a per-plugin counter; see §3.4 step 4)
    vmConfig     []byte                // from VmConfig.configuration (passed to proxy_on_vm_start); deferred-hostcall surface — 25.1 guests ignore
    pluginConfig []byte                // from PluginConfig.configuration (passed to proxy_on_configure); deferred-hostcall surface
    stats        *filterStats          // SHARED across listener; no per-route stat at 25.1 (no per-route at 25.1; per-route PARSE-REJECTs)
}

// filterStats — 5 counters per AMEND-A2 + parent §7.
// Tri-group prefix structure: Group B (upstream-parity) + envoy-go-strict extensions.
// HCM-injected stats_prefix is DROPPED per AMEND-A2 (the wasm filter row DIVERGES from
// the dominant §9 family-row pattern).
type filterStats struct {
    // Group B (wasm.<runtime>.* prefix; <runtime> = "wazero" uniformly):
    created       *stats.Counter // wasm.wazero.created — upstream-parity per AMEND-A2
    active        *stats.Gauge   // wasm.wazero.active — upstream-parity per AMEND-A2

    // envoy-go-strict extensions (wasm.<plugin_name>.* prefix):
    executions    *stats.Counter // wasm.<plugin_name>.executions — per proxy_on_request_headers invocation
    hostcallDenied *stats.Counter // wasm.<plugin_name>.hostcall_denied — per default-denied hostcall invocation
    envoyGoFailures *stats.Counter // wasm.<plugin_name>.envoy_go.failures — per VM-failure event
}
```

The `compiledConfig` is constructed at `New` and shared (read-only) across the listener's `*filter` per-stream instances. Each per-stream filter dispatch constructs a fresh `*wasm.VM` from `cfg.module` per §3.4. **No per-route stat at 25.1** (no per-route Wasm at 25.1; per-route TPFC PARSE-REJECTs per parent §6.2 arm 18).

### 4.3 Filter struct + per-stream dispatch shape

```go
package wasm

import (
    "context"

    "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/wasm"
)

// filter is the per-stream filter instance. NOT goroutine-safe.
type filter struct {
    cfg           *compiledConfig
    vm            *wasm.VM
    streamContextID uint32

    // Captured local-response state — populated by abi_callbacks.SendLocalResponse;
    // consumed by DecodeHeaders/EncodeHeaders after CallProxyOnRequestHeaders/Response.
    sentLocalResponse *capturedLocalResponse
}

// capturedLocalResponse holds the proxy_send_local_response payload until the
// host-side filter dispatch handoffs to FilterCallbacks.SendLocalReply.
type capturedLocalResponse struct {
    statusCode uint32
    statusMsg  string
    body       string
    additionalHeaders []wasm.HeaderPair
    grpcStatus int32
}

// Compile-time interface assertions.
var (
    _ http.StreamDecoderFilter = (*filter)(nil)
    _ http.StreamEncoderFilter = (*filter)(nil)
)
```

The dispatch shape per-stream lifecycle:
- **DecodeHeaders**: `vm.NewVM(ctx, withSandboxConfig(cfg.sandbox), withLogSink(filterLog))` → `vm.RegisterABICallbacks(&abiCallbacks{filter: f, ...})` → `vm.Run(ctx, cfg.module, cfg.rootContextID)` → `vm.CallProxyOnContextCreate(ctx, f.streamContextID, cfg.rootContextID)` → `cfg.stats.executions++` → `vm.CallProxyOnRequestHeaders(ctx, f.streamContextID, headerCount, endOfStream)`. If `f.sentLocalResponse != nil`: return `StopIteration` + `decoderCb.SendLocalReply(...)`. Else `ProxyAction::CONTINUE` → `Continue`; `ProxyAction::PAUSE` w/o local-response → log + `Continue` (stream-control deferred).
- **EncodeHeaders**: `vm.CallProxyOnResponseHeaders(ctx, f.streamContextID, headerCount, endOfStream)` → same disposition.
- **OnDestroy**: `vm.CallProxyOnDone(ctx, f.streamContextID)` → `vm.CallProxyOnLog(ctx, f.streamContextID)` → `vm.CallProxyOnDelete(ctx, f.streamContextID)` → `vm.Close()`.

---

## 5. Hostcall + Callback surface (24 hostcalls + 13 callbacks at 25.1)

Per parent SPEC §4.2 + AMEND-A4 + AMEND-A7. The 25.1 hostcall surface registered at `internal/wasm/registration.go` exposes 24 hostcalls + 13 guest-export callbacks. The full enumeration with file-of-implementation + capability key + deferred-25.2/25.3 stub disposition:

### 5.1 16 `proxy_*` env-namespace hostcalls (active at 25.1)

| # | Hostcall | Capability key | File | Notes |
|---|---|---|---|---|
| 1 | `proxy_log(level, msg_ptr, msg_size)` | `proxy_log` | registration.go | routes to `WithLogSink`; anchors `wasm.<plugin>.executions` counter on entry to a proxy_on_request_headers (not here) |
| 2 | `proxy_get_log_level(result_level_uint32_ptr)` | `proxy_get_log_level` | registration.go | v0.2.1-new; detection signal for v0.2.1-only modules |
| 3 | `proxy_send_local_response(status_code, status_msg_ptr, status_msg_size, body_ptr, body_size, additional_headers_ptr, additional_headers_size, grpc_status)` | `proxy_send_local_response` | registration.go | 8-argument hostcall; consumes REUSE 5 (`SendLocalReply` per parent §3.3) |
| 4 | `proxy_get_header_map_pairs(type, ptr_ptr, size_ptr)` | `proxy_get_header_map_pairs` | registration.go + pairs.go | byte-faithful pairs wire format per R3 |
| 5 | `proxy_set_header_map_pairs(type, ptr, size)` | `proxy_set_header_map_pairs` | registration.go + pairs.go | same wire format |
| 6 | `proxy_get_header_map_value(type, key_ptr, key_size, value_ptr_ptr, value_size_ptr)` | `proxy_get_header_map_value` | registration.go | |
| 7 | `proxy_add_header_map_value(type, key_ptr, key_size, value_ptr, value_size)` | `proxy_add_header_map_value` | registration.go | |
| 8 | `proxy_replace_header_map_value(type, key_ptr, key_size, value_ptr, value_size)` | `proxy_replace_header_map_value` | registration.go | |
| 9 | `proxy_remove_header_map_value(type, key_ptr, key_size)` | `proxy_remove_header_map_value` | registration.go | |
| 10 | `proxy_get_header_map_size(type, result_ptr)` | `proxy_get_header_map_size` | registration.go | |
| 11 | `proxy_get_property(path_ptr, path_size, value_ptr_ptr, value_size_ptr)` | `proxy_get_property` | registration.go | minimal property tree at 25.1 (`request.headers.*`, `response.headers.*`, `request.path`, `request.method`, `request.host`); full CEL surface 25.2 |
| 12 | `proxy_set_property(key_ptr, key_size, value_ptr, value_size)` | `proxy_set_property` | registration.go | |
| 13 | `proxy_get_status(code_ptr, value_ptr_ptr, value_size_ptr)` | `proxy_get_status` | registration.go + abi_callbacks.go | consumes ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` per D-P3 + R7 (first co-consumer of phase-23 primitive) |
| 14 | `proxy_set_effective_context(context_id)` | `proxy_set_effective_context` | registration.go | context-switch hostcall (used by timer + httpCall callbacks at 25.2) |
| 15 | `proxy_done()` | `proxy_done` | registration.go | guest-signals-end-of-context |
| 16 | `proxy_get_current_time_nanoseconds(result_uint64_ptr)` | `proxy_get_current_time_nanoseconds` | registration.go | DEPRECATED in v0.2.1; implementable for upstream byte-faithfulness |

### 5.2 8 `wasi_snapshot_preview1.*` namespace shims (active at 25.1; custom-stub implementation per R4)

Per R4 + parent §4.2: do NOT use wazero's built-in `imports/wasi_snapshot_preview1` package (which routes `fd_write` to host stdout/stderr; we need it routed through `proxy_log` per the proxy-wasm semantics).

| # | Hostcall | Capability key | File | Notes |
|---|---|---|---|---|
| 17 | `fd_write(fd, iovec, iovec_size, nwritten_ptr)` | `fd_write` | wasi.go | fd=1 → `proxy_log(INFO, ...)`; fd=2 → `proxy_log(ERROR, ...)`; other → `WasiErrno::BADF` (=8) |
| 18 | `clock_time_get(clock_id, precision, time_ptr)` | `clock_time_get` | wasi.go | wall (CLOCK_REALTIME=0) + monotonic (CLOCK_MONOTONIC=1); host-time accuracy at 25.1 (no fake-time seam) |
| 19 | `random_get(buffer, buffer_size)` | `random_get` | wasi.go | `crypto/rand.Read(...)` bytes |
| 20 | `environ_sizes_get(num_elements_ptr, buffer_size_ptr)` | `environ_sizes_get` | wasi.go | returns 0/0 at 25.1+25.2; 25.3 returns real values from `VmConfig.environment_variables` |
| 21 | `environ_get(argv_ptr, buffer_ptr)` | `environ_get` | wasi.go | paired with environ_sizes_get; writes nothing at 25.1+25.2 |
| 22 | `args_sizes_get(argc_ptr, argv_buf_size_ptr)` | `args_sizes_get` | wasi.go | always 0/0 (no command-line args ever passed to wasm plugin) |
| 23 | `args_get(argv_ptr, argv_buf_ptr)` | `args_get` | wasi.go | paired; writes nothing |
| 24 | `proc_exit(exit_code)` | `proc_exit` | wasi.go | well-behaved guests MUST NOT call; if invoked, terminates VM via wazero trap |

### 5.3 13 guest-export callbacks (host invokes these on the guest at 25.1)

The host invokes these via `wazero.Module.ExportedFunction("proxy_on_X").Call(ctx, args...)` lookups; the corresponding `VM` methods (per §3.1) wrap each call with the capability gate + the panic-wrapper.

| # | Callback | Caller (vm.go method) | Notes |
|---|---|---|---|
| C1 | `_initialize()` | Run (step b alternative) | required module init; mutually exclusive with `_start` |
| C2 | `_start()` | Run (step b alternative) | alt module init |
| C3 | `main()` | Run (step b followup) | called after `_initialize` if exported; discard return |
| C4 | `malloc(size_t)` | (host calls via ABI shim for buffer-return) | legacy guest allocator (v0.1.0-era); some guest stdlib paths call internally |
| C5 | `proxy_on_memory_allocate(size_t)` | (host calls via ABI shim for buffer-return) | v0.2.0+ preferred guest allocator |
| C6 | `proxy_on_context_create(context_id, parent_context_id)` | CallProxyOnContextCreate | host signals new context (root context: parent=0; stream context: parent=root) |
| C7 | `proxy_on_vm_start(root_context_id, vm_configuration_size)` | Run (step c) | host signals VM started |
| C8 | `proxy_on_configure(plugin_context_id, plugin_configuration_size) → bool` | Run (step d) | host signals plugin config |
| C9 | `proxy_on_done(context_id) → bool` | CallProxyOnDone | host signals end-of-context; returning false defers finalize |
| C10 | `proxy_on_log(context_id)` | CallProxyOnLog | final access-log-time callback before stream context destroyed |
| C11 | `proxy_on_delete(context_id)` | CallProxyOnDelete | final context destruction signal |
| C12 | `proxy_on_request_headers(stream_context_id, num_headers, end_of_stream) → ProxyAction` | CallProxyOnRequestHeaders | returns CONTINUE (=0) or PAUSE (=1) |
| C13 | `proxy_on_response_headers(stream_context_id, num_headers, end_of_stream) → ProxyAction` | CallProxyOnResponseHeaders | returns CONTINUE (=0) or PAUSE (=1) |

### 5.4 23 deferred-25.2/25.3 hostcalls (registered as stub-Unimplemented at 25.1)

Per parent §4.2 Option B: 25.1 registers ALL hostcalls (24 active + 23 deferred = 47 total) so module-instantiation succeeds for modules that import the deferred surface; the deferred stubs return `WasmResult::Unimplemented` (=12) when invoked. The 23 deferred hostcalls fall under: body+buffer (3), stream-control (2), timer (1), metric (4), shared-data (2), shared-queue (4), outbound HTTP (1), outbound gRPC (5), foreign-function (1). The full list lives in parent §4.2 + parent §3.1 surface-mapping table; 25.1 IMPL registers the stubs but does NOT enumerate them in this 25.1 SPEC (they belong to the 25.2 + 25.3 sub-phase SPECs).

---

## 6. Per-task structure (17 tasks; ~3,650-5,360 LIVE LoC envelope per parent §3.0)

Per phase-22.1 SPEC §6 precedent + parent §3.0 task/LoC envelope (25.1 ~17-19 tasks; ~3,650-5,360 LIVE LoC; task-arm gate fits under ADR-0045 ~25-task threshold). The 25.1 IMPL session's TDD/subagent-driven dispatch hits each Task in order. Per-task acceptance criteria + commit-message anticipation:

### Task 1 — NEW `internal/wasm/` package skeleton + `doc.go` + go.mod + ABI types
- **Files:** Create `internal/wasm/doc.go`, `internal/wasm/abi/types.go`. Modify `go.mod` (add `github.com/tetratelabs/wazero v1.10.1` direct dep per AMEND-A1). Modify `go.sum` (regenerated).
- **Acceptance:** wazero v1.10.1 added as direct dep; `go.mod` Go-floor STAYS at `go 1.23.0`; `abi/types.go` defines WasmResult (10 named values with value-gaps at 5/9/11), WasmBufferType (9 named values; value 8 = FOREIGN_FUNCTION_ARGUMENTS), WasmHeaderMapType (8 values), LogLevel (6 values), ProxyAction (2 values), WasiErrno (subset roster); doc.go covers Q1-Q9 BRAINSTORM decisions + AMEND-A1..A9 cross-refs + the API surface summary; build clean; `internal/wasm/abi/types_test.go` round-trip tests pass (value-gap preservation verified).
- **Commit:** `feat(internal/wasm): bootstrap framework primitive — doc.go + abi/types.go + wazero v1.10.1 dep`

### Task 2 — `internal/wasm/bytecode_util.go` byte-faithful ABI-version detection per AMEND-A6 + D-P1 first-action
- **Files:** Create `internal/wasm/bytecode_util.go` + `internal/wasm/bytecode_util_test.go`.
- **Acceptance:** `BytecodeUtil.GetAbiVersion(src []byte) (AbiVersion, error)` reimplements `proxy-wasm-cpp-host:bytecode_util.cc:32-97` byte-faithfully — scans wasm module export section (type 7) for function-kind exports named `proxy_abi_version_0_2_1` / `proxy_abi_version_0_2_0` / `proxy_abi_version_0_1_0`; returns the detected version OR `AbiVersionUnknown`. Test coverage: crafted-wasm-binary fixtures (~5-10 binaries: valid v0.2.1, valid v0.1.0, valid v0.2.0, missing-sentinel, malformed export section, truncated module). **D-P1 first-action**: scrape upstream `proxy_wasm_exports.h:232-249` against the WASI errno disposition; record the chosen errno (`NOTSUP=58` or `ENOTCAPABLE=76`) in this task's PROGRESS.md entry + use the chosen value in §3.3 sandbox.go WASI denial path.
- **Commit:** `feat(internal/wasm): byte-faithful bytecode_util ABI-version detection per AMEND-A6 + D-P1 errno pin`

### Task 3 — `internal/wasm/pairs.go` byte-faithful pairs wire format per R3
- **Files:** Create `internal/wasm/pairs.go` + `internal/wasm/pairs_test.go`.
- **Acceptance:** `EncodePairs([]HeaderPair) []byte` + `DecodePairs([]byte) ([]HeaderPair, error)` byte-faithful round-trip vs `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc`. Wire format: `u32 num_pairs / u32 key_len, u32 value_len (repeated num_pairs times) / key_bytes NUL value_bytes NUL (repeated num_pairs times)`. Test coverage: empty pairs, single pair, multi-pair, oversized pairs (boundary check), malformed input (truncated header, oversize length count). Golden-bytes table per the cpp-host oracle.
- **Commit:** `feat(internal/wasm): byte-faithful pairs wire format per R3`

### Task 4 — `internal/wasm/wasi.go` custom 8-stub WASI implementation per R4
- **Files:** Create `internal/wasm/wasi.go` + `internal/wasm/wasi_test.go`.
- **Acceptance:** 8 WASI shim functions implementing the proxy-wasm semantics: `fd_write` routes to `proxy_log` (fd=1 → INFO, fd=2 → ERROR, other → `WasiErrno::BADF`=8); `clock_time_get` at host accuracy (CLOCK_REALTIME=0 wall + CLOCK_MONOTONIC=1 monotonic); `random_get` via `crypto/rand`; `environ_sizes_get` + `environ_get` return 0/0 at 25.1+25.2 (deferred to 25.3); `args_sizes_get` + `args_get` return 0/0 always; `proc_exit` traps via `sys.NewExitError(exit_code)`. Do NOT use wazero's built-in `imports/wasi_snapshot_preview1`. Test coverage: each shim's golden semantics + bad-fd/bad-arg error paths.
- **Commit:** `feat(internal/wasm): custom 8-stub WASI implementation per R4`

### Task 5 — `internal/wasm/compile.go` Module + CompileCache + ABI-version gating
- **Files:** Create `internal/wasm/compile.go` + `internal/wasm/compile_test.go`.
- **Acceptance:** `Module` type wrapping `wazero.CompiledModule` + detected `AbiVersion` + sha256 hash. `CompileCache` content-addressed cache with sync.RWMutex; nil-tolerant per ADR-0085. `CompileModule(ctx, src, cache)` compiles via wazero's compile runtime + gates via `bytecode_util.GetAbiVersion` (`AbiVersionUnknown` or `AbiVersion_0_2_0` or `AbiVersion_0_1_0` → returns `ErrUnsupportedAbiVersion` per AMEND-A6); cache-hit-on-same-content-hash returns the cached module; concurrent read/add tested.
- **Commit:** `feat(internal/wasm): Module + CompileCache + ABI-version gating per AMEND-A6`

### Task 6 — `internal/wasm/sandbox.go` default-deny capability roster + D-P2 closure
- **Files:** Create `internal/wasm/sandbox.go` + `internal/wasm/sandbox_test.go`.
- **Acceptance:** `SandboxConfig` type with `AllowedCapabilities map[string]SanitizationConfig`; `SanitizationConfig` empty struct (accept-empty-as-no-op per AMEND-A1 §11.4 + parent §4.3.5); `IsAllowed(name string) bool` inverts upstream's empty-map-allow-all semantic (empty map → deny ALL). Package-private capability-key constants for the 25.1 surface (24 hostcalls + 13 module-getter/callback keys). Test coverage: empty map denies all keys; populated map allows only named keys; sanitization-config-non-empty parse-and-discard. **D-P2 closure first-action**: scrape `proxy-wasm-cpp-host:wasm.cc:298-302` getFunction discipline to confirm whether the 5 module-init/allocator callbacks (`_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate`) participate in capability gating; record the disposition in this task's PROGRESS.md entry (anticipated: ungated — they are required for instantiation).
- **Commit:** `feat(internal/wasm): default-deny capability sandbox per AMEND-A5 + D-P2 closure`

### Task 7 — `internal/wasm/vm.go` VM lifecycle + ABICallbacks interface + panic-wrapper
- **Files:** Create `internal/wasm/vm.go` + `internal/wasm/registration.go` + `internal/wasm/vm_test.go` + `internal/wasm/registration_test.go`.
- **Acceptance:** `VM` type per §3.1 production signatures + `VMOption` function-option pattern (`WithSandboxConfig`, `WithPanicHandler`, `WithLogSink`); `NewVM(ctx, opts...)` constructs the wazero.Runtime + registers the env-namespace + wasi_snapshot_preview1-namespace host modules (24 active hostcalls + 23 deferred stubs); `State`, `RegisterABICallbacks`, `Run` (module-init lifecycle), `HasGlobalFunc`, `CallProxyOnContextCreate`, `CallProxyOnRequestHeaders`, `CallProxyOnResponseHeaders`, `CallProxyOnDone`, `CallProxyOnLog`, `CallProxyOnDelete`, `Close`. Panic-wrapper invokes `WithPanicHandler` after `recover()`. `ABICallbacks` interface at registration.go carries the 13-method headers-bridge subset. Test coverage: per-stream construction round-trip; sandbox-deny dispatch for each capability key; panic-wrapper behavior tests; concurrent VMs share no state.
- **Commit:** `feat(internal/wasm): VM lifecycle + ABICallbacks interface + panic-wrapper`

### Task 8 — NEW `internal/filter/http/wasm/` package skeleton + `doc.go` + `wasm.go` + `stats.go`
- **Files:** Create `internal/filter/http/wasm/doc.go`, `internal/filter/http/wasm/wasm.go`, `internal/filter/http/wasm/stats.go`, `internal/filter/http/wasm/wasm_test.go`.
- **Acceptance:** Filter package skeleton with `TypeURL` + `filterName` constants; `New(ctx http.FilterFactoryContext) (http.FilterInstanceFactory, error)` factory stub returning a sentinel error (real parse lands at Task 9); `filterStats` with 5 counters per AMEND-A2 (`wasm.wazero.{created,active}` + `wasm.<plugin_name>.{executions,hostcall_denied,envoy_go.failures}`) registered against `internal/stats`; tri-group prefix structure with HCM-stats_prefix DROPPED. Test coverage: stat-name table-driven verification (114 → 119 project delta).
- **Commit:** `feat(internal/filter/http/wasm): package skeleton + 5-counter stat surface per AMEND-A2`

### Task 9 — `internal/filter/http/wasm/compiled_config.go` 18-arm PARSE-REJECT roster + D-P5 byte-stable wording
- **Files:** Create `internal/filter/http/wasm/compiled_config.go` + `internal/filter/http/wasm/compiled_config_test.go`.
- **Acceptance:** `compiledConfig` struct per §4.2; `buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error)` parses the `Wasm` proto + dispatches across the 18-arm PARSE-REJECT roster per parent §6.2 (full byte-stable wording per D-P5; package-private `parseReject*` consts; `TestParseRejectConstants_ByteStable` table-driven test verifies each constant string). **D-P5 closure at this task**: pin the exact byte-stable wording for all 18 arms; commit-time `TestParseRejectConstants_ByteStable` enforces. Test coverage: each PARSE-REJECT arm triggered + exact wording asserted; valid-config path produces non-nil compiledConfig + sha256-hash-keyed module cache lookup.
- **Commit:** `feat(internal/filter/http/wasm): 18-arm PARSE-REJECT roster + D-P5 byte-stable wording`

### Task 10 — `internal/filter/http/wasm/datasource.go` 4-arm AsyncDataSource.Local resolution
- **Files:** Create `internal/filter/http/wasm/datasource.go` + `internal/filter/http/wasm/datasource_test.go`.
- **Acceptance:** `resolveDataSource(local *corev3.DataSource) ([]byte, error)` dispatches across the 4-arm DataSource oneof (Filename → `os.ReadFile`; InlineBytes → verbatim; InlineString → byte-cast; EnvironmentVariable → `os.LookupEnv`); `WatchedDirectory` sibling field PARSE-REJECTs (arm 7); empty `specifier` oneof PARSE-REJECTs (arm 8). Test coverage: each arm + ENOENT / unset env var / empty inline-bytes failure paths.
- **Commit:** `feat(internal/filter/http/wasm): 4-arm AsyncDataSource.Local resolution`

### Task 11 — `internal/filter/http/wasm/abi_callbacks.go` ABICallbacks implementation + D-P3 closure (ADR-0196 first co-consumer)
- **Files:** Create `internal/filter/http/wasm/abi_callbacks.go` + `internal/filter/http/wasm/abi_callbacks_test.go`.
- **Acceptance:** `abiCallbacks` struct implementing `wasm.ABICallbacks` for the per-stream HTTP-filter context: 7 header-map methods, GetProperty (minimal property tree: `request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host`), SetProperty, SendLocalResponse (captures local-reply state on the filter struct; consumed in decode/encode handlers), GetStatus (RE-CONSUMES `EncoderFilterCallbacks.ResponseStatus()` per ADR-0196 + D-P3 + R7 — FIRST co-consumer of phase-23 primitive), Log (routes to filter log sink), GetLogLevel, GetCurrentTimeNanoseconds, SetEffectiveContext, Done. **D-P3 closure first-action**: scrape ADR-0196's accessor signature + the encoder-callback shape; confirm consumption discipline. Test coverage: each method round-trips correctly + minimal-property-tree exhaustive coverage + sandbox-deny path returns `WasmResult::InternalFailure`.
- **Commit:** `feat(internal/filter/http/wasm): ABICallbacks impl + D-P3 ADR-0196 first co-consumer`

### Task 12 — `internal/filter/http/wasm/decode_headers.go` + `encode_headers.go`
- **Files:** Create `internal/filter/http/wasm/decode_headers.go`, `internal/filter/http/wasm/encode_headers.go`.
- **Acceptance:** `DecodeHeaders` constructs per-stream VM, registers ABICallbacks, runs module-init lifecycle, dispatches `proxy_on_request_headers`, handles ProxyAction (CONTINUE → Continue; PAUSE w/o local-response → log + Continue; captured local-response → StopIteration + SendLocalReply). `EncodeHeaders` mirrors for `proxy_on_response_headers`. `executions` counter increments per dispatch. Test coverage: full lifecycle integration (per-stream VM created → dispatched → destroyed); panic-wrapper bumps `envoy_go.failures`; sandbox-deny bumps `hostcall_denied`.
- **Commit:** `feat(internal/filter/http/wasm): DecodeHeaders + EncodeHeaders dispatch shape`

### Task 13 — Boot-registration at `cmd/envoy-go/main.go` (alphabetical position per §3.6)
- **Files:** Modify `cmd/envoy-go/main.go`.
- **Acceptance:** `httpReg.Register(wasm.TypeURL, wasm.New)` insertion at the line immediately before `httpReg.Freeze()` per §3.6 (alphabetical position; `wasm` sorts after `router` → appends to tail of the 19-entry roster). 19 → 20 HTTP filters wired. Build clean. Test coverage: integration test verifies the filter is discoverable via the registry.
- **Commit:** `feat(cmd/envoy-go): wire envoy.filters.http.wasm at 20th HTTP filter`

### Task 14 — 34th project-wide fuzzer `FuzzWasmConfigParse`
- **Files:** Create `internal/filter/http/wasm/fuzz_test.go`.
- **Acceptance:** `FuzzWasmConfigParse(f *testing.F)` with ~30 corpus seeds covering all 18 PARSE-REJECT arms + valid configs + adversarial wasm bytecode (must-never-panic invariant). Coverage: wazero compile error path (arm 17 — adversarial bytecode must not crash the parser). `grep -c "^func Fuzz"` project-wide goes from 33 → 34. Test coverage: `go test -run=^$ -fuzz=FuzzWasmConfigParse -fuzztime=30s` clean.
- **Commit:** `test(internal/filter/http/wasm): 34th project-wide fuzzer FuzzWasmConfigParse`

### Task 15 — Differential fixture `0034-http-wasm-headers-bridge` (7 scenarios; full cross-side per parent §8.1 + §4.5 D6 guardrails) + NEW `BackendKind=HTTPWasm`
- **Files:** Create `test/fixtures/0034-http-wasm-headers-bridge/` directory tree per parent §8.1.3 (README.md, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go, scripts/{a-g}/Cargo.toml + src/lib.rs + scripts/README.md, bytecode/{a-g}.wasm vendored). Modify `test/differential/runner_test.go` (NEW `BackendKind=HTTPWasm` constant + blank-import of the fixture).
- **Acceptance:** 7 scenarios per parent §8.1.1 (add-fixed-header / replace-header / remove-header / respond-shortcircuit / log-only-passthrough / header-iteration-count / property-read-method) all full cross-side byte-exact via existing `CompareBytes` runner branch (per `reference_differential_asserter_dispatch` — `StatsAsserter.AssertStats` for subject-side stat assertions because this is a cross-side fixture, NOT `SubjectAsserter` which is dead on the cross-side path). Each scenario complies with §4.5 D6 guardrails (no memory traps; HTTP/1.1; no float-formatted logs; only 24-hostcall surface). Vendored Rust source per Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target). `BackendKind=HTTPWasm` added to runner switch-case (~20 LoC delta). 35 → 36 differential fixture dirs (+1; 0035 lands at Task 16 for 35 → 37).
- **Commit:** `test(test/fixtures/0034-http-wasm-headers-bridge): 7-scenario full cross-side byte-exact per parent §8.1`

### Task 16 — Differential fixture `0035-http-wasm-boot-reject` + D-P6 arm-finalization
- **Files:** Create `test/fixtures/0035-http-wasm-boot-reject/` directory tree (README.md, envoy.yaml, envoy-go.yaml, inputs/driver.go). Modify `test/differential/runner_test.go` (blank-import).
- **Acceptance:** Single-arm boot-reject parity per D-P6 — anticipated arm 5 (`vm-config-code-required`); the empirically-tested arm with the most distinctive substring + cleanest config shape. **D-P6 closure first-action**: empirically-test arm 5 + arms {3, 4, 8, 17} against upstream Envoy v1.37.2 boot stderr; pick the arm with the most distinctive substring + cleanest config shape; record disposition in this task's PROGRESS.md entry. 36 → 37 differential fixture dirs (+1). Cross-side common stderr substring per `runBootRejectFixture` discipline (per `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch).
- **Commit:** `test(test/fixtures/0035-http-wasm-boot-reject): single-arm boot-reject parity per D-P6`

### Task 17 — Benchmark + BEHAVIOR_CONTRACT.md 6-edit bundle + ADR-0202+0203+0204 §Decision+§Consequences body landing + STATE.md re-advance + R8 escape-valve gate
- **Files:** Create `internal/filter/http/wasm/wasm_bench_test.go` (BenchmarkPerStreamVM_Construction_Headers). Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md` (6-edit bundle per parent §13.5). Modify `docs/envoy-go/DECISIONS.md` (ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences body landings per ADR-0044 in-place edit discipline). Modify `docs/envoy-go/STATE.md` + `docs/envoy-go/ROADMAP.md`.
- **Acceptance:** **R8 escape-valve gate**: benchmark `BenchmarkPerStreamVM_Construction_Headers` measures per-stream wazero Runtime construction cost. If `ns/op < 1_000_000` (1ms): WEAK-default per-stream construction STANDS; ADR-0205 escape-valve STAYS UNCONSUMED. If exceeded: ADR-0205 escape-valve fires (§Context + §Decision + §Consequences all land at this same Task 17 commit; "per-module wazero Runtime pool with pre-instantiated entries" decision). 6-edit BEHAVIOR_CONTRACT.md bundle per parent §13.5: (1) NEW `### envoy.filters.http.wasm` subsection ~150-250 lines; (2) stat-table 114 → 119 extension; (3-5) 4-5 envoy-go-strict departure records consolidated; (6) NEW `### Phase 25.1 forward-pointer notes` subsection ~50-80 lines. ADR-0202 / -0203 / -0204 §Decision + §Consequences bodies per parent §10.1 + this 25.1 SPEC §3-§5 + §11.1 D-S1 + Task 1-16 cumulative evidence. STATE.md re-advance to `phase 25.1 IMPL done; awaiting 25.2 SPEC` + ROADMAP row 25.1 flipped `planned → done`. **17 → 24 envoy-go-strict departures** (or 18 → 24-25 per parent §13.5 edit #5).
- **Commit:** `feat(phase-25.1): BEHAVIOR_CONTRACT bundle + ADR-0202+0203+0204 bodies + STATE re-advance [R8 escape-valve gate]`

**Total: 17 tasks** — well within the parent §3.0 17-19 task envelope. Anticipated LIVE LoC: ~4,200-4,800 (within the ~3,650-5,360 envelope). Per-Task LoC distribution: Tasks 1-7 (`internal/wasm/` primitive) ~2,200-2,800 LoC; Tasks 8-13 (`internal/filter/http/wasm/` package + boot wiring) ~1,200-1,500 LoC; Tasks 14-17 (fuzzer + 2 fixtures + benchmark + atomic-landing bundle) ~800-1,000 LoC.

---

## 7. PARSE-REJECT roster (cross-reference parent §6 + D-P5 byte-stable wording finalization)

The parent SPEC §6.2 enumerates the 18-arm 25.1 PARSE-REJECT roster verbatim. This 25.1 SPEC INHERITS the 18-arm roster + commits to byte-stable wording finalization at Task 9 per D-P5. The arm-by-arm wording lives at parent §6.2; this SPEC's role is the IMPL-time wording-discipline commitment:

- **Wording discipline:** format `"wasm: <field_path>: <reason> [; <forward-pointer hint>]"` per parent §6.1. Filter-proto-name prefix `wasm:` invariant on every arm. Constants live as package-private `parseReject*` consts at `internal/filter/http/wasm/compiled_config.go`, returned via `errors.New(parseReject...)` for byte-stability. Kebab-case arm identifiers (used for SPEC cross-reference + test-name suffixes like `TestBuildCompiledConfig_PARSE_REJECT_remote_arm_deferred`).
- **Boot-reject parity arms:** arms 1, 2, 3, 4, 5, 8, 17 (typed_config + required-field-missing arms + compile-failure — upstream ALSO rejects).
- **envoy-go-strict-stricter arms:** arms 6 (Remote), 7 (WatchedDirectory), 9 (FAIL_RELOAD), 10 (fail_open), 11 (runtime discriminator), 12 (duplicate vm_id), 13 (environment_variables), 14 (allow_precompiled), 15 (nack_on_code_cache_miss), 16 (ABI v0.1.0/v0.2.0), 18 (per-route TPFC) — upstream ACCEPTS; envoy-go PARSE-REJECTs as defensive mirror or envoy-go-strict-stricter departure.
- **Cross-validation:** `TestParseRejectConstants_ByteStable` (ADR-0080) verifies each constant's byte content at IMPL Task 9.

### 7.2 25.2 + 25.3 PARSE-REJECT forward-pointers

Per parent §6.3. 25.2 anticipated +8-12 arms (body+buffer + capability_restriction_config arm structures); 25.3 anticipated +6-10 arms (per-route TPFC + multi-plugin VM-sharing + environment_variables activation). Authoritative roster lives in each sub-phase's own SPEC.

---

## 8. Stat surface (cross-reference parent §7)

The parent SPEC §7 enumerates the 5-counter 25.1 stat surface per AMEND-A2. This 25.1 SPEC INHERITS verbatim. Summary:

- `wasm.wazero.created` (counter; Group B upstream-parity per AMEND-A2)
- `wasm.wazero.active` (gauge; Group B upstream-parity per AMEND-A2)
- `wasm.<plugin_name>.executions` (counter; envoy-go-strict extension per AMEND-A2)
- `wasm.<plugin_name>.hostcall_denied` (counter; envoy-go-strict extension per AMEND-A2 + AMEND-A5)
- `wasm.<plugin_name>.envoy_go.failures` (counter; envoy-go-strict extension per AMEND-A2)

**Stat-prefix template DIVERGES from §9 family-row pattern** per AMEND-A2 — tri-group structure (`wasm.<runtime>.{created,active}` Group B + `wasm.<plugin_name>.{...}` Group C-and-envoy-go-strict) rather than HCM-rooted `http.<HCM_stat_prefix>.<filter>.*`. HCM-injected `stats_prefix` is DROPPED (`source/extensions/filters/http/wasm/config.h:51-53`). NOT an envoy-go-strict departure — upstream-parity preservation. Recorded at BEHAVIOR_CONTRACT.md stat-name mapping section as a special-case row.

Project stat count **114 → 119** at 25.1 phase-done (+5).

---

## 9. Differential fixture taxonomy (per cold-start refinements (g) + (h))

### 9.1 Fixture `0034-http-wasm-headers-bridge` (7 cross-side scenarios; full byte-exact per AMEND-A4)

Per parent §8.1 + cold-start refinement (g) + §4.5 D6 guardrails. The 7 scenarios per parent §8.1.1 table:

| # | Name | Plugin behavior | Wire-output assertion | Asserter |
|---|---|---|---|---|
| (a) | add-fixed-header | `proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-wasm-injected", "hello")` | Request header reflected | `CompareBytes` |
| (b) | replace-header | `proxy_replace_header_map_value(HTTP_REQUEST_HEADERS, "user-agent", "envoy-go-wasm/1.0")` | Reflected user-agent | `CompareBytes` |
| (c) | remove-header | `proxy_remove_header_map_value(HTTP_REQUEST_HEADERS, "x-blocked")` | Reflected request without x-blocked | `CompareBytes` |
| (d) | respond-shortcircuit | `proxy_send_local_response(403, "Forbidden", "denied", &[], 0)` | Full byte-pinned tuple at client | `CompareBytes` |
| (e) | log-only-passthrough | `proxy_log(INFO, "wasm hit")` | Reflected request unchanged + executions counter delta | `CompareBytes` + `StatsAsserter.AssertStats` (per `reference_differential_asserter_dispatch`) |
| (f) | header-iteration-count | `proxy_get_header_map_pairs` + count + add x-headers-count | Reflected x-headers-count: N (HTTP/1.1; no HPACK reorder per §4.5(b)) | `CompareBytes` |
| (g) | property-read-method | `proxy_get_property("request.method")` + add x-request-method | Reflected x-request-method: GET | `CompareBytes` |

**§4.5 guardrail compliance:** all 7 scenarios use only the 24-hostcall surface; HTTP/1.1 transport; no memory-trap probes; no float-formatted logs.

**Asserter dispatch discipline (per `reference_differential_asserter_dispatch`):** scenario (e)'s subject-side stat-counter assertion lives at `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)` (the cross-side runner branch invokes `StatsAsserter` per the live-asserter dispatch table); deliberately-break test verifies liveness (break the stat assertion → expect FAIL → restore). DO NOT place the assertion at `SubjectAsserter.AssertSubject` (that interface is dead on the cross-side path).

**Recommended directory structure** per parent §8.1.3:

```
test/fixtures/0034-http-wasm-headers-bridge/
  README.md             # ~150-250 lines: scope + 7-scenario table + topology + cross-refs
  envoy.yaml            # reference Envoy bootstrap
  envoy-go.yaml         # subject bootstrap; same topology
  expectations.yaml     # human-readable declarative scenario expectations (NOT consumed by runner)
  inputs/
    driver.go           # registered Driver impl (~400-600 LoC); per-scenario probes + classifyBody +
                        # StatsAsserter.AssertStats implementation for scenario (e)
  scripts/              # per-scenario Rust source + reproduction build script
    a_add_header/Cargo.toml + src/lib.rs
    b_replace_header/Cargo.toml + src/lib.rs
    c_remove_header/Cargo.toml + src/lib.rs
    d_respond/Cargo.toml + src/lib.rs
    e_log_only/Cargo.toml + src/lib.rs
    f_headers_count/Cargo.toml + src/lib.rs
    g_property_method/Cargo.toml + src/lib.rs
    README.md           # reproduction script + pinned rustup toolchain + cargo build invocation
  bytecode/             # vendored pre-built .wasm files (binary blobs committed to git)
    a_add_header.wasm
    b_replace_header.wasm
    c_remove_header.wasm
    d_respond.wasm
    e_log_only.wasm
    f_headers_count.wasm
    g_property_method.wasm
```

NEW `BackendKind=HTTPWasm` constant addition at `test/differential/runner_test.go` near the existing `HTTPLua`/`HTTPCsrf`/`HTTPCompressor` precedent — switch-case addition; ~20 LoC delta. Lands at Task 15.

### 9.2 Fixture `0035-http-wasm-boot-reject` (single-arm boot-reject parity per D-P6)

Per parent §8.2 + cold-start refinement (h). Single-arm boot-reject parity at arm 5 (`vm-config-code-required`) anticipated per D-P6 — empirically tested at Task 16 first-action against upstream Envoy v1.37.2 boot stderr; alternative candidates {3, 4, 5, 8, 17}. **Selection criterion**: most distinctive common stderr substring + cleanest config shape. Anticipated common substring for arm 5: `"required"` or `"vm_config.code"`.

Boot-reject infrastructure: existing `BootRejectFixture` runner discipline from phase-22.1; cross-side asserts both reference + subject fail to boot + common stderr substring present in both. Per `reference_differential_fixture_dispatch_constraint`, one fixture dir = one runner branch (this is the boot-reject branch).

### 9.3 Total fixture-dir count

35 → 37 at 25.1 phase-done (+2: `0034` + `0035`). 25.2 + 25.3 add +2 each per parent §8.3 + §8.5.

### 9.4 Listener topology

Single listener with a single HCM containing the wasm filter (alphabetical position) + router terminator. Mirrors the dominant §9 family-row topology.

---

## 10. Behavior-contract delta (cross-reference parent §9)

The parent SPEC §9 enumerates the 10 high-level semantic changes for phase 25. The 25.1 IMPL final-Task 6-edit BEHAVIOR_CONTRACT.md bundle (per parent §13.5 + §14 below) lands the 25.1-scope subset of those changes. This 25.1 SPEC INHERITS verbatim; the bundle materializes at Task 17.

---

## 11. SPEC-time empirical-pin block (cross-reference parent §11 + D-S1 sub-pin)

The parent SPEC §11 carries the full 9-pin empirical-pin block (D1-D9) resolved IN-SESSION at the parent SPEC drafting. This 25.1 SPEC does NOT re-execute those pins; they apply across all 3 sub-phases. The 25.1 SPEC ADDS one sub-pin:

### 11.1 D-S1 — 34th-fuzzer count VERIFIED at this SPEC commit (per parent §13-R5)

**Pin:** verify project-wide fuzzer count = 33 at master tip + the new `FuzzWasmConfigParse` is the 34th.

**Evidence (this SPEC session):**

```
$ cd /home/esa/git/envoy-go && grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not -path './.worktrees/*' -not -path './.claude/*') | wc -l
33
```

(Detailed scrape command + output captured in the 25.1 PROGRESS.md at IMPL Task 14 first-action; this SPEC anchors the SPEC-time disposition: **count CONFIRMED at 33 at master tip**; `FuzzWasmConfigParse` is the 34th.)

**Disposition:** CONFIRMED at SPEC commit. ADR-0203 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch pin to **34** at IMPL Task 17. R5 (parent §13.2) CLOSED at this 25.1 SPEC §11.1.

---

## 12. SPEC-time D-questions for IMPL-time resolution

Per parent §12 + phase-22.1 §11 precedent. The parent SPEC has CLOSED most D-question candidates at SPEC commit per AMEND-A1..A9. The 6 remaining D-questions (D-P1..D-P6) carry from parent §12; this 25.1 SPEC commits each to a specific IMPL Task for resolution + anticipates the answer.

| D# | Question (per parent §12) | Resolution Task | Anticipated answer |
|---|---|---|---|
| **D-P1** | WASI denial errno: `NOTSUP=58` OR `ENOTCAPABLE=76`? | Task 2 first-action scrape of `proxy_wasm_exports.h:232-249` | Mirror upstream `ENOTCAPABLE=76` for byte-faithfulness; if wazero's WASI semantics prevent the exact return code, fall back to `NOTSUP=58` + record a sub-pin envoy-go-strict departure. |
| **D-P2** | Module-init/allocator callbacks (5 keys: `_initialize`/`_start`/`main`/`malloc`/`proxy_on_memory_allocate`) participate in capability gating? | Task 6 first-action scrape of `proxy-wasm-cpp-host:wasm.cc:298-302` | Ungated — module-init callbacks are required for instantiation; gating them would break every module. |
| **D-P3** | `proxy_get_status` consumes ADR-0196 `EncoderFilterCallbacks.ResponseStatus()`? | Task 11 first-action scrape of ADR-0196 accessor signature + encoder-callback shape | Consume ADR-0196 (FIRST co-consumer of phase-23 primitive — RATIFIES the phase-23 extraction discipline analogous to phase-22.2's first co-consumer of phase-20 `internal/httpclient/`). |
| **D-P4** | `BenchmarkPerStreamVM_Construction_Headers` > 1ms? | Task 17 R8 escape-valve gate | Under threshold (phase-22.1 observed 70µs for gopher-lua; wazero compiler-mode initialization is comparable). If exceeded, ADR-0205 escape-valve consumes. |
| **D-P5** | Pin exact byte-stable wording for all 18 PARSE-REJECT arms. | Task 9 `TestParseRejectConstants_ByteStable` table | Wordings per parent §6.2 table; byte-stability enforced by table-driven test. |
| **D-P6** | Confirm arm 5 (`vm-config-code-required`) is cleanest boot-reject parity candidate OR pick alternative from {3, 4, 5, 8, 17}? | Task 16 first-action empirical test against upstream Envoy v1.37.2 boot stderr | Arm 5 — anticipated distinctive substring `"required"` reproduced by envoy-go's mirror wording. |

**D-question discipline:** each Task's PROGRESS.md entry quotes the scrape evidence + records the disposition; the relevant ADR §Decision body (ADR-0202 / -0203 / -0204) carries forward the disposition at Task 17 atomic landing.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + sub-phase-specific)

The parent SPEC §13 enumerates 8 RATIFIED-PENDING-IMPL items (R1-R8). This 25.1 SPEC INHERITS all 8 items + adds 25.1-SPEC sub-pins. Disposition table:

| Item | Parent §13 framing | 25.1 SPEC disposition |
|---|---|---|
| **R1** | 25.1 fixture-0034 cross-side byte-exact + §4.5 D6 guardrail compliance | STANDS. Lands at Task 15. |
| **R2** | ABI v0.1.0+v0.2.0 PARSE-REJECT byte-faithful detection point | STANDS. Lands at Task 2 (`internal/wasm/bytecode_util.go`). |
| **R3** | pairs wire format byte-faithful reimplementation | STANDS. Lands at Task 3 (`internal/wasm/pairs.go`). |
| **R4** | WASI shim custom 8-stub implementation | STANDS. Lands at Task 4 (`internal/wasm/wasi.go`). |
| **R5** | 34th project-wide fuzzer count verification | **CLOSED** at this 25.1 SPEC §11.1 D-S1 resolution: count CONFIRMED at 33 at master tip; `FuzzWasmConfigParse` is 34th. ADR-0203 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch pin to 34 at Task 17. |
| **R6** | ADR-0177 `internal/httpclient/` co-consumer validation at 25.2 | NOT a 25.1 item; settles at 25.2 IMPL (`proxy_http_call` task). |
| **R7** | ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` first co-consumer at 25.1 | STANDS per D-P3 anticipated answer. Lands at Task 11. RATIFIES the phase-23 framework-primitive extraction discipline. |
| **R8** | wazero per-stream Runtime construction benchmark + ADR-0205 escape-valve gate | STANDS. Lands at Task 17 first-action benchmark. If `ns/op > 1_000_000` (1ms): ADR-0205 escape-valve consumed for "per-module wazero Runtime pool with pre-instantiated entries" decision (§Context + §Decision + §Consequences body all land at the same Task 17 commit per ADR-0044). Until benchmarked, hold per-stream construction as the WEAK-default. |

**New 25.1-SPEC sub-pins:**

- **D-S1 (R5) closed at §11.1** — 33 → 34 fuzzer count CONFIRMED at this SPEC commit.
- **D-P1 closure at Task 2 first-action** — WASI denial errno disposition.
- **D-P2 closure at Task 6 first-action** — module-init callback gating disposition.
- **D-P3 closure at Task 11 first-action** — ADR-0196 first co-consumer signature confirmation.
- **D-P5 closure at Task 9** — 18-arm PARSE-REJECT byte-stable wording pinning.
- **D-P6 closure at Task 16 first-action** — boot-reject arm selection.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (cross-reference parent §13.5)

The parent SPEC §13.5 enumerates the 25.1 IMPL final-Task **6-edit bundle**. This 25.1 SPEC INHERITS the 6-edit bundle verbatim. The bundle lands at IMPL Task 17 per §6 atomic landing:

1. NEW `### envoy.filters.http.wasm` subsection (~150-250 lines) — headers-bridge-focused for 25.1; forward-pointers to 25.2 + 25.3.
2. Stat-table 114 → 119 extension under `## Stat surface` — 5 new rows under tri-group prefix structure per AMEND-A2; structural note documenting HCM-stats_prefix-DROPPED + tri-group divergence from §9 family-row pattern.
3. envoy-go-strict departure record #1: default-deny capability sandbox (per AMEND-A5 + ADR-0204).
4. envoy-go-strict departure record #2: ABI v0.1.0+v0.2.0 PARSE-REJECT (per AMEND-A6).
5. envoy-go-strict departure record #3 (consolidated 4-5-record group): `AsyncDataSource.Remote` PARSE-REJECT + runtime-name discriminator PARSE-REJECT + `executions` + `hostcall_denied` + `envoy_go.failures` envoy-go-strict counters + structural-note row for HCM-stats_prefix DROPPED. Departure-record count 18 → 23-24 (or 18 → 22 if some consolidate further at Task 17 authoring time).
6. NEW `### Phase 25.1 forward-pointer notes` subsection (~50-80 lines) — 25.2 anticipated additions (body+buffer; timer/metric/shared-data/httpCall; foreign-function-with-empty-default-registry per AMEND-A9) + 25.3 anticipated additions (per-route 5th-canonical REUSE-by-absence per AMEND-A3 — NO ADR-0125 amendment; multi-plugin VM-sharing; conformance harness seed at 62.5% threshold per AMEND-A8).

25.2 + 25.3 IMPL final-Task bundles anticipated per parent §13.5 — settled at 25.2 + 25.3 BRAINSTORM/SPEC.

---

## 15. Test surface + 25.1 IMPL acceptance checklist

### 15.1 Test surface (per parent SPEC §14)

The parent SPEC §14 enumerates the 6-layer 25.1 test surface verbatim:

- **Layer A: unit tests at `internal/filter/http/wasm/`** — `wasm_test.go` (~1500-2000 LoC); `compiled_config_test.go` (18-arm table-driven per parent §6.2); `datasource_test.go` (4-arm table-driven); `abi_callbacks_test.go` (13-callback subset + sandbox-deny dispatch).
- **Layer B: unit tests at `internal/wasm/`** — `vm_test.go` + `compile_test.go` + `sandbox_test.go` + `bytecode_util_test.go` + `pairs_test.go` + `wasi_test.go` + `registration_test.go` + `abi/types_test.go`.
- **Layer C: 34th project-wide fuzzer `FuzzWasmConfigParse`** at standard ADR-0018 baseline; ~30 corpus seeds covering all 18 PARSE-REJECT arms + valid + adversarial wasm bytecode.
- **Layer D: differential fixture `0034-http-wasm-headers-bridge`** per §9.1 — 7 scenarios full cross-side byte-exact via `CompareBytes`; subject-side stat assertion via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`.
- **Layer D′: boot-reject fixture `0035-http-wasm-boot-reject`** per §9.2 — single-arm boot-reject parity at arm 5 (anticipated per D-P6).
- **Layer E: race + concurrency tests** at `internal/wasm/` + `internal/filter/http/wasm/` — concurrent VM construction; compile-cache concurrent read/add; per-stream filter dispatch independence.

### 15.2 Six-gate checklist (per phase-22+24 precedent)

- **Gate A — build:** `go build ./...` clean (incl. `internal/wasm/` + `internal/filter/http/wasm/` + wazero direct go.mod dep).
- **Gate B — vet + lint:** `go vet ./...` + `golangci-lint run` clean; no new suppressions.
- **Gate C — race:** `go test -race ./...` clean.
- **Gate D — differential:** 37/37 fixtures GREEN at 25.1 phase-done (0000-0033 pre-existing + 0034 + 0035 new); cross-side byte-exact on `0034`; boot-reject substring on `0035`.
- **Gate E — fuzz:** `FuzzWasmConfigParse` clean at 30s/seed; no panics across the 34 project-wide fuzzers.
- **Gate F — h2spec:** 53/53 PASS at ADR-0051 v1.32.4 pin.

### 15.3 25.1 IMPL acceptance checklist (parent §15 + sub-phase-specific extensions)

The parent SPEC §15 enumerates 24 items. This 25.1 SPEC EXTENDS with 6 sub-phase-specific items per the next-prompt-permitted scope. The 25.1 IMPL Task 17 atomic landing MUST satisfy ALL of:

**Items 1-24 from parent SPEC §15 (verbatim — see parent SPEC §15):**

1. NEW `internal/wasm/` package created with API surface per §3.1 production refinements + parent §4.1.
2. NEW `internal/filter/http/wasm/` package created with files per §3.5 + parent §4.4.
3. wazero `v1.10.1` added as direct go.mod dependency per AMEND-A1.
4. `Wasm.config.vm_config.code` consumed (4-arm AsyncDataSource Local) per parent §5.4 + §6.2 arms 5-8; Remote + WatchedDirectory PARSE-REJECTed.
5. `PluginConfig.{name, root_id, vm_config, configuration, capability_restriction_config}` consumed; deferred fields PARSE-REJECTed per parent §6.2 arms 9, 10, 13.
6. `VmConfig.{vm_id (singleton), runtime ("" or wazero), code, configuration}` consumed; deferred fields PARSE-REJECTed per parent §6.2 arms 11, 12, 14, 15.
7. ABI version PARSE-REJECT per parent §6.2 arm 16 + AMEND-A6 byte-faithful `BytecodeUtil.GetAbiVersion` reimplementation at Task 2.
8. 25.1 hostcall surface = 24 hostcalls (16 `proxy_*` + 8 `wasi_*`) per §5; 23 deferred-25.2/25.3 hostcalls registered as stub-Unimplemented per parent §4.2 Option B.
9. 25.1 callback surface = 13 callbacks per §5.3 (5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks).
10. Default-deny capability sandbox per §3.3 + AMEND-A5 + envoy-go-strict departure record at BEHAVIOR_CONTRACT.md (per §14 edit #3).
11. Per-stream `*VM` construction + per-module `*Module` compile cache per §3.4.
12. 18-arm PARSE-REJECT roster per parent §6.2 (subject to D-P5 byte-stable wording finalization at Task 9 — `TestParseRejectConstants_ByteStable`).
13. 5-counter stat surface per parent §7 + this 25.1 SPEC §8; 114 → 119 BEHAVIOR_CONTRACT.md update per §14 edit #2.
14. 4-5 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md per §14 edits #3-#5 (default-deny + ABI v0.1.0/v0.2.0 PARSE-REJECT + Remote PARSE-REJECT + runtime-discriminator PARSE-REJECT + envoy-go-strict counters consolidated).
15. 34th project-wide fuzzer `FuzzWasmConfigParse` at standard ADR-0018 baseline; must-never-panic verified.
16. Differential fixture `0034-http-wasm-headers-bridge` GREEN — 7 scenarios full cross-side byte-exact per parent §8.1 + §4.5 D6 guardrails; vendored Rust-sourced `.wasm` bytecode under `bytecode/` per Q9 + AMEND-A1.
17. Differential fixture `0035-http-wasm-boot-reject` GREEN per parent §8.2 — single-arm boot-reject parity per D-P6.
18. NEW `BackendKind=HTTPWasm` constant added at `test/differential/runner_test.go`.
19. WASI shim custom 8-stub implementation per R4 + §5.2 (`fd_write` → `proxy_log`; `proc_exit` traps; NOT wazero's built-in WASI imports).
20. pairs wire format byte-faithful reimplementation per R3 at `internal/wasm/pairs.go`.
21. ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences bodies landed in DECISIONS.md per the §Context anchor at parent SPEC commit; ADR-0044 in-place edit discipline.
22. STATE.md re-advance to `phase 25.1 IMPL done; awaiting 25.2 SPEC` + ROADMAP row 25.1 flipped `planned → done` per ADR-0106.
23. 19 HTTP filters wired (per master tip) → 20 HTTP filters wired post-25.1 (`wasm.New` insertion at `cmd/envoy-go/main.go` per §3.6).
24. wazero-VM-pool benchmark task per R8 + D-P4; if `ns/op > 1_000_000` (1ms), ADR-0205 escape-valve fires.

**Items 25-30 — 25.1 SPEC-specific extensions:**

25. **D-S1 resolution recorded at §11.1** — 34th-fuzzer count CONFIRMED at this SPEC commit; ADR-0203 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch pin to 34 at IMPL Task 17.
26. **D-P1 closure at Task 2 first-action** — WASI denial errno disposition; PROGRESS.md entry quotes upstream evidence; sandbox.go uses the chosen errno.
27. **D-P2 closure at Task 6 first-action** — module-init/allocator callback gating disposition; PROGRESS.md entry quotes upstream `wasm.cc:298-302` evidence.
28. **D-P3 closure at Task 11 first-action** — ADR-0196 first co-consumer signature confirmation; PROGRESS.md entry quotes ADR-0196 + encoder-callback shape evidence; RATIFIES phase-23 framework-primitive extraction discipline.
29. **D-P5 closure at Task 9** — 18-arm PARSE-REJECT byte-stable wording pinning via `TestParseRejectConstants_ByteStable` table-driven test.
30. **D-P6 closure at Task 16 first-action** — boot-reject arm selection via empirical test against upstream Envoy v1.37.2 boot stderr.

Plus the standard per-task PROGRESS.md entry shape per phase-21+22.1+23+24 IMPL precedent (17 entries; each quotes command outputs per `superpowers:verification-before-completion`; each Task's acceptance criteria verified before PROGRESS.md entry) + REVIEW.md authored at 25.1 IMPL phase-done per `superpowers:requesting-code-review` (per-task review notes + cross-cutting + green-light evidence).

---

## Appendix A — Cross-references to parent SPEC

This 25.1 SPEC cross-references the parent SPEC at `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md` for the following content (inherited verbatim; NOT duplicated here):

- **Parent §1** (Mission) — envelope D + 3-way pre-split + 14-fact summary (FIRST §9 row to author in-house proxy-wasm v0.2.1 host ABI; FIRST §9 row to introduce Apache-2.0-licensed VM-class dep; FIRST §9 row to seed conformance harness; etc.).
- **Parent §1.1** (9-AMEND catalog) — all 9 AMENDs incorporated into this 25.1 SPEC's §3/§4/§5/§7/§8/§9/§10/§14.
- **Parent §1.2** (STRENGTHENED-or-revised D-hypothesis at SPEC commit) — STRENGTHENED-WEAK-HOLD-with-1-slot-buffer STANDS UNCHANGED; 25.1 IMPL escape-valve at ADR-0205 from `wazero-VM-pool` benchmark surface (§13-R8).
- **Parent §2** (Scope — non-purposes + REUSES-not-consumed) — full 29-item non-purposes catalog; this 25.1 SPEC §2 summarizes for 25.1 surface only.
- **Parent §3** (Sub-phase scope summary) — 3-way split LOCKED at BRAINSTORM Q1; STANDS unchanged at SPEC commit; per-sub-phase scope detail at each sub-phase's SPEC.
- **Parent §4** (Framework primitive) — `internal/wasm/` API + `internal/filter/http/wasm/` package + sandbox roster + D6 guardrails.
- **Parent §5** (Proto-field roster) — `Wasm` 1 field + `PluginConfig` 8 fields + `VmConfig` 7 fields + AsyncDataSource 2-arm + DataSource 4-arm + 1 binding-gap forward-pointer.
- **Parent §6** (PARSE-REJECT roster) — 18-arm 25.1 roster per AMEND-A1 + AMEND-A6.
- **Parent §7** (Stat surface) — 5 counters under tri-group prefix structure per AMEND-A2.
- **Parent §8** (Differential fixture taxonomy) — 7-scenario fixture-0034 + single-arm fixture-0035 + `BackendKind=HTTPWasm` + `bytecode/` + `scripts/` subdirectory layout.
- **Parent §9** (Behavior-contract delta) — 10 high-level semantic changes.
- **Parent §10** (ADR anchor map) — 3 NEW ADR §Context drafts at 25.1; +3-4 at 25.2; +3 at 25.3; ZERO IN-PLACE AMENDMENTs; ZERO ADR-0125 amendments per AMEND-A3.
- **Parent §11** (SPEC-time empirical-pin block) — all 9 pins resolved IN-SESSION at parent SPEC drafting; §11.1-§11.8 full scrape evidence.
- **Parent §12** (SPEC-time D-questions for IMPL-time resolution) — D-P1..D-P6 carry forward per this 25.1 SPEC §12.
- **Parent §13** (RATIFIED-PENDING-IMPL items) — 8 items; this 25.1 SPEC §13 disposition table maps each.
- **Parent §13.5** (BEHAVIOR_CONTRACT.md edit bundle anticipation) — 6-edit bundle at 25.1 IMPL final Task per ADR-0052.
- **Parent §14** (Test surface) — 6-layer test taxonomy.
- **Parent §15** (25.1 IMPL acceptance checklist) — 24 items; this 25.1 SPEC §15.3 extends with 6 sub-phase-specific items 25-30.

---

## Appendix B — Phase 25.1 ADR landings summary

At THIS 25.1 SPEC commit: **NO NEW ADRs consumed.** DECISIONS.md tail STAYS at ADR-0204. Next-free ADR STAYS at ADR-0205.

At 25.1 IMPL Task 17 atomic landing:

- **ADR-0202 §Decision + §Consequences body** — NEW `internal/wasm/` framework primitive. Per parent §4.1 sketch + this 25.1 SPEC §3.1 production signatures + §3.2 8-file split + §3.3 default-deny capability roster + §3.4 per-stream lifecycle + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per parent §4.1 + BRAINSTORM Q3 + Q4 + the broader §9 WASM host family explicit-future-consumer roster.
- **ADR-0203 §Decision + §Consequences body** — NEW `internal/filter/http/wasm/` package shape. Per parent §4.4 + this 25.1 SPEC §3.5 file split + §4 compiledConfig + filterStats + parent §6.2 18-arm PARSE-REJECT roster + parent §8.1 fixture-0034 + §11.1 D-S1 closure (34-fuzzer count CONFIRMED) + Task 2 D-P1 + Task 6 D-P2 + Task 9 D-P5 + Task 11 D-P3 + Task 16 D-P6 closures + R3 + R4 + R7 RATIFIED-PENDING items.
- **ADR-0204 §Decision + §Consequences body** — proxy-wasm capability-restriction default-deny + envoy-go-strict sandbox posture. Per parent §4.3 + AMEND-A5 + this 25.1 SPEC §3.3 + Task 6 D-P2 closure + capability key roster (~80 keys; 25.1 materializes the 24-hostcall subset + 13 module-getter callback subset) + denial semantic (`WasmResult::InternalFailure`=10 + integration error log + `hostcall_denied` counter) + `SanitizationConfig` accept-empty discipline.
- **ADR-0205 §Context + §Decision + §Consequences body (CONDITIONAL — only if R8 escape-valve fires)** — wazero-VM-pool design. Only consumes if Task 17 benchmark `BenchmarkPerStreamVM_Construction_Headers` surfaces > 1ms per-stream construction cost. If unconsumed: ADR-0205 carries forward to 25.2 BRAINSTORM as the 25.2 IMPL escape-valve slot per parent §1.2.

At 25.2 IMPL final Task: ADR-0205 .. ADR-0208 (anticipated 3-4 NEW ADRs per parent §10.2 — `internal/wasm/` 25.2 ABI extensions + `internal/wasm/foreign.go` foreign-function registration per AMEND-A9 + `internal/filter/http/wasm/` 25.2 package extensions + escape-valve reserve).

At 25.3 IMPL final Task: ADR-0210 (EXPLICIT-NO-NEW-CANONICAL per-route classification per AMEND-A3) + ADR-0211 (multi-plugin VM-sharing semantics) + ADR-0212 (conformance harness pin per AMEND-A8) + NO ADR-0125 amendment per AMEND-A3.

**End of phase 25.1 SPEC.**
