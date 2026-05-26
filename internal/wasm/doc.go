// Package wasm is the envoy-go-OWNED proxy-wasm v0.2.1 host-side
// framework primitive at the NEW top-level package `internal/wasm/` per
// ADR-0202 (§Context anchored at phase 25 parent SPEC commit `2c1455d`;
// §Decision + §Consequences body lands at phase 25.1 IMPL Task 17 per
// ADR-0044 in-place edit discipline).
//
// Pure-Go implementation via github.com/tetratelabs/wazero v1.10.1
// (Apache-2.0 licensed per AMEND-A1; the BRAINSTORM 'MIT-licensed'
// characterization was a typo). Go-floor pinned at 1.23.0 to satisfy
// wazero's minimum.
//
// Strategically positioned for cross-phase reuse: the *VM lifecycle +
// *Module + content-addressed *CompileCache + capability-scoped
// *SandboxConfig + ABI v0.2.1 hostcall/callback surface compose against
// future consumers of the proxy-wasm runtime beyond the phase-25.1 first
// consumer `internal/filter/http/wasm/` (envoy.filters.http.wasm) — the
// future cluster-specifier-wasm + access-logger-wasm +
// network-filter-wasm + WasmService singleton per BRAINSTORM Q3 + Q4
// (consumers #2-#5; ADR-0202 carries the EXPLICIT API-REVISION
// ALLOWANCE clause that fires at consumer #2 landing — the API shape
// may be revised after empirical validation).
//
// # BRAINSTORM Q1-Q9 decisions (SETTLED)
//
//   - Q1: 3-way pre-split — phase 25 envelope splits into 25.1
//     (HTTP-filter + framework primitive + headers-bridge subset),
//     25.2 (body callbacks + AsyncDataSource.Remote), 25.3 (gRPC +
//     HTTP-call + shared-data + shared-queue + foreign-function +
//     metric); 25.1 is foundational and lands the primitive.
//   - Q2: wazero pin — github.com/tetratelabs/wazero v1.10.1 direct
//     dep (NOT Wasmer / NOT V8 / NOT WAMR / NOT WAVM); rationale
//     anchored in 25.1 SPEC §3 (pure-Go, no CGO, Apache-2.0).
//   - Q3: WASM host family scope — the 5 §9 production WASM filters
//     (HTTP, cluster-specifier, access-logger, network, WasmService)
//     co-consume this primitive; ADR-0202 EXPLICIT API-REVISION clause
//     applies at consumer #2.
//   - Q4: API-revision allowance at consumer #2 — the VM/Module/
//     CompileCache/SandboxConfig API shape may be revised when
//     consumer #2 (cluster-specifier-wasm) lands; the consumer #1
//     shape ships at 25.1.
//   - Q5: default-deny capability sandbox — see AMEND-A5; zero-value
//     SandboxConfig is StrictDefaultDeny.
//   - Q6: ABI v0.2.1-only — see AMEND-A6; proxy-wasm spec v0.2.1 is
//     the SOLE supported ABI; older guests REJECT at compile-gate.
//   - Q7: tri-group prefix structure — see AMEND-A2; hostcall + WASI +
//     callback identifiers carry distinct prefixes for clarity.
//   - Q8: 5th-canonical REUSE-by-absence — see AMEND-A3; ADR-0125
//     STAYS at 10 canonicals (no new canonical at 25.1; ADR-0125 NOT
//     amended; the absence of an entry IS the canonical bind).
//   - Q9: vendored Rust-sourced .wasm — fixture-0034's proxy-wasm
//     guest module is built from upstream proxy-wasm-rust-sdk source
//     and vendored as bytecode; reproduction script + checksum live
//     at `scripts/build_proxy_wasm_fixture_0034.sh` (lands at IMPL
//     Task 15).
//
// # AMEND-A1..A9 cross-references (one-line each)
//
//   - AMEND-A1: wazero is Apache-2.0 (corrects BRAINSTORM 'MIT'
//     typo); Go-floor STAYS at 1.23.0 per wazero requirement.
//   - AMEND-A2: tri-group prefix structure for hostcall + WASI +
//     callback identifiers (proxy_/wasi_snapshot_preview1_/proxy_on_).
//   - AMEND-A3: ADR-0125 STAYS at 10 canonicals; 5th-canonical
//     REUSE-by-absence (NO new canonical at phase 25).
//   - AMEND-A4: per-stream VM construction model (NOT per-config-shared
//     VM); each HTTP stream gets its own VM instance for isolation.
//   - AMEND-A5: default-deny capability sandbox (zero-value
//     SandboxConfig = StrictDefaultDeny; explicit opt-in required for
//     each capability).
//   - AMEND-A6: ABI v0.2.1-only support; older spec versions REJECT at
//     compile-gate with byte-stable error wording.
//   - AMEND-A7: WasmResult 10 named values with byte-faithful gaps at
//     positions 5/9/11 (BRAINSTORM's 13-contiguous hypothesis was
//     incorrect); WasmBufferType value 8 = ForeignFunctionArguments
//     (NOT CallData as BRAINSTORM hypothesized).
//   - AMEND-A8: panic-handler wrapper around every guest call; genuine
//     Go panics from bridge callbacks recover()'d + reported via
//     PanicHandlerFn (NOT propagated to Envoy filter chain).
//   - AMEND-A9: compile-cache content-addressed via SHA-256(.wasm
//     bytecode); nil-tolerant per ADR-0085 (cache absence != cache
//     miss; absent cache = compile-every-time).
//
// # API surface summary (lands progressively across Tasks 1-7)
//
// Task 1 (this task) lands ONLY the package skeleton + the
// `internal/wasm/abi/` enum types + the wazero direct dep. The named
// production types below are FORWARD-DECLARED here for cross-Task
// orientation; their bodies land at the indicated Task.
//
// Task 5 — internal/wasm/compile.go:
//
//   - Module — opaque content-addressed compiled-module representation;
//     SHA-256(.wasm)-keyed in the optional CompileCache.
//   - CompileCache + NewCompileCache + CompileModule([]byte,
//     *CompileCache) — content-addressed compile cache (nil-tolerant
//     per ADR-0085 / AMEND-A9).
//   - (*Module).AbiVersion() — returns the proxy-wasm spec version
//     detected at compile-gate (v0.2.1 ONLY per AMEND-A6).
//
// Task 6 — internal/wasm/sandbox.go:
//
//   - SandboxConfig — per-capability ALLOW/DENY posture; the zero value
//     is StrictDefaultDeny per AMEND-A5.
//   - SanitizationConfig — host-side input sanitization knobs for
//     header / pair / body buffer accesses from the guest.
//   - IsAllowed(*SandboxConfig, Capability) bool — capability check
//     used by every guarded hostcall.
//
// Task 7 — internal/wasm/vm.go + internal/wasm/registration.go:
//
//   - VM — opaque per-stream wazero execution context (NOT goroutine-safe);
//     constructed via NewVM(opts ...VMOption); released via Close at OnDestroy.
//   - VMOption — function-option configurator (mirrors the internal/lua,
//     internal/jwks, and internal/httpclient option-pattern precedent).
//   - PanicHandlerFn — invoked after recover() in the VM panic-wrapper for
//     genuine Go panics from bridge callbacks per AMEND-A8.
//   - (*VM).State() — escape-hatch for per-stream state in hostcall handlers.
//   - RegisterABICallbacks(*VM, ABICallbacks) — installs the consumer
//     callback table (decode/encode headers, log, done, delete).
//   - (*VM).Run(*Module) — instantiates the module + executes _start.
//   - (*VM).HasGlobalFunc(name) bool — supports Task 9 PARSE-REJECT.
//   - (*VM).CallProxyOnContextCreate / RequestHeaders / ResponseHeaders /
//     Done / Log / Delete — typed proxy-wasm v0.2.1 callback invokers used
//     by 25.1 (body / trailers / async-call land in 25.2 / 25.3).
//   - (*VM).Close() — releases the wazero module instance; idempotent.
//
// # Sibling subdirectory: internal/wasm/abi/
//
// The `abi/` subdirectory holds proxy-wasm v0.2.1 wire-protocol enum types
// (WasmResult / WasmBufferType / WasmHeaderMapType / LogLevel / ProxyAction
// / WasiErrno) — value-faithful per AMEND-A7. See internal/wasm/abi/types.go.
//
// # 25.2 BRAINSTORM Q1-Q11 + AMEND-B1..B5 cross-refs (lands at IMPL Tasks 1-22)
//
// Phase 25.2 evolves this package per ADR-0205+0206+0207+0208 + the
// BRAINSTORM Q1-Q11 + 5 AMEND-B entries empirical-pin block. The
// authoritative 25.2 SPEC anchor: `docs/envoy-go/phases/25.2-http-filter-
// wasm-body-and-advanced-bridge/SPEC.md` (§3.1 production signatures + §5
// hostcall/callback wire shapes).
//
//   - Q1: continued 3-way pre-split (25.1/25.2/25.3); 25.2 is body+advanced.
//
//   - Q2: wazero pin UNCHANGED at v1.10.1 (AMEND-A1 carries forward).
//
//   - Q3: root-VM lifecycle architecture — ONE long-lived `*RootVM` per
//     `*compiledConfig` (upstream-byte-faithful per cpp-host
//     `Wasm`/`Plugin` model); per-stream contexts as CHILDREN sharing the
//     wazero Runtime + Module. The 25.1 per-stream `*VM` (61µs/stream
//     wazero.Runtime construction) RETIRED per D-P-PLAN-6 — see
//     root_vm.go + stream_context.go (Task 1).
//
//   - Q4: API-revision allowance at consumer #2 STAYS at ADR-0202 envelope;
//     25.2's per-consumer-#1 evolution lands under NEW ADRs per phase-22.2
//     Q10 strict-scope precedent.
//
//   - Q5: tick goroutine per-`*RootVM` with envoy-go-strict 10ms period
//     floor + ADR-0186 Clock seam injection (first co-consumer beyond
//     phase-21 — RATIFIES extraction). Lands at tick.go (Task 5).
//
//   - Q6: shared-data map per-`*RootVM` with CAS semantic byte-exact from
//     cpp-host + envoy-go-strict caps (1 MiB per-value; 1024 entries).
//     Lands at shared_data.go (Task 6).
//
//   - Q7: NEW `internal/filterstate/` framework primitive at consumer #2
//     scope (ADR-0207); phase-22.2 lua MIGRATES non-breaking. Generic
//     per-stream Bucket + FilterStateObject interface (StateType
//     discriminator). Consumed via proxy_get_property "filter_state.*" +
//     "upstream_filter_state.*" paths per AMEND-B4.
//
//   - Q8: mixed-mode fixture-0036 single-listener (14 scenarios; 10
//     cross-side + 4 subject-only via StatsAsserter; deliberate-break
//     liveness MANDATORY) per ADR-0192 precedent.
//
//   - Q9: NEW `internal/stats/dynamic/` infrastructure (ADR-0208) +
//     dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2;
//     per-`*RootVM` `*Registry` wrap at dynamic_stats.go (Task 12).
//
//   - Q10: ADR-0202 §Consequences one-line in-place AMEND
//     acknowledgment per Q10 strict-scope precedent + ADR-0044 in-place
//     edit discipline (NO new ADR number for the AMEND).
//
//   - Q11: 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` (Task 19;
//     35-seed corpus across 10 dimensions per D-P-PLAN-10).
//
//   - AMEND-B1: buffer-clamp wire-contract — `proxy_get_buffer_bytes` host
//     shim clamps on `start + max_size > buffer.length` (returns Ok with
//     truncated length); i32-overflow returns BadArgument. Lands at the
//     body bridge (Task 4).
//
//   - AMEND-B2: metric signedness — increment uses SIGNED `int64` delta;
//     record uses UNSIGNED `uint64` value. envoy-go-strict 1024-entry cap
//     on dynamic namespace.
//
//   - AMEND-B3: httpCall + cancel-at-destruction — outstanding httpCalls
//     dispatched from a stream context are CANCELED at the
//     StreamContext.Close cancel-at-destruction step. envoy-go-strict
//     departure: `http_call_response_after_close` counter
//     (post-cancel arrivals are dropped, not delivered). Lands at
//     http_call.go (Task 8).
//
//   - AMEND-B4: NUL-delimited proxy_get_property/proxy_set_property path
//     parsing per upstream cpp-host wire format. Full ~70-path
//     property-tree mapping at property.go (Task 13).
//
//   - AMEND-B5: gate-at-registration host-module wiring — for each
//     capability in the 58-key 25.2 cumulative roster (37 from 25.1 + 21
//     NEW), if the per-`*RootVM` SandboxConfig.IsAllowed(key) returns
//     false, the corresponding host function is NOT registered on the
//     wazero Runtime. Matches upstream byte-faithfully + amplifies the
//     default-deny posture.
//
// # 25.2 ABI surface evolution summary
//
//   - RootVM lifecycle (Task 1) — root_vm.go + stream_context.go; 25.1
//     per-stream *VM RETIRED per D-P-PLAN-6; STUB state for tick + shared-
//     data + httpCalls + foreign-function + dynamic-stats (activation at
//     Tasks 5-8 + 12).
//   - SandboxConfig 37→58 keys (Task 2) — sandbox.go EXTEND; 21 NEW
//     capability keys per AMEND-B5 + SPEC §11.5.
//   - ABICallbacks 13→20 methods (Task 15) — abi_callbacks.go EXTEND;
//     7 NEW methods (OnRequestBody / OnResponseBody / OnRequestTrailers /
//     OnResponseTrailers / OnTick / OnHttpCallResponse / OnForeignFunction)
//     per ADR-0206.
//   - Hostcall surface: 24 active + 23 deferred at 25.1 → 38 active + 9
//     deferred at 25.2 (14 NEW activations; cumulative 47 total stays
//     UNCHANGED). Lands at registration.go EXTEND (Task 3).
//   - tick goroutine + shared-data + httpCall + foreign-function + property
//   - dynamic-stats wrap — see Q5/Q6/Q7/Q9 above.
package wasm
