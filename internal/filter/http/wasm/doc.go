// Package wasm implements the envoy.filters.http.wasm HTTP filter under the
// 07.1 HTTP filter framework. Phase 25.1: VM runtime + headers-bridge mode
// (foundational third of the SIXTEENTH §9 production HTTP filter; phase 25.2 +
// 25.3 deliver the full envelope D — body / trailers / full property tree /
// outbound HTTP / outbound gRPC / shared-data / shared-queue / timer / metric
// / foreign-function + the 5th-canonical WasmPerRoute wholesale-override +
// multi-plugin VM-sharing + conformance harness seed).
//
// Anchored at ADR-0203 (§Context drafted at phase 25 parent SPEC; §Decision +
// §Consequences body lands at 25.1 IMPL Task 17 per ADR-0044 in-place edit
// discipline). Consumes the NEW internal/wasm/ framework primitive at first
// consumer per ADR-0202 + BRAINSTORM Q4 EXTRACT-NOW.
//
// # API surface (per 25.1 SPEC §4.1)
//
//   - TypeURL — byte-exact wire URL
//     "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"
//     per ADR-0143 SN1. Pinned at wasm_test.go::TestTypeURL.
//   - filterName — "envoy.filters.http.wasm" per ADR-0070; identifies the
//     filter on the listener config http_filters[].name + the HCM chain
//     dispatch identifier. Pinned at wasm_test.go::TestFilterName.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx)
//     (envoyhttp.FilterInstanceFactory, error) — the HTTPFilterFactory
//     registered at boot (Task 13 wires
//     `httpReg.Register(wasm.TypeURL, wasm.New)` alphabetically after the
//     router terminal-filter slot). TASK 8 SKELETON: returns
//     (nil, errFactorySkeleton); FULL IMPL body lands at Task 9 (parse) +
//     Task 11 (abi_callbacks) + Task 12 (decode/encode dispatch).
//   - RegisterPerRouteValidator — exported function called from
//     cmd/envoy-go/main.go BEFORE httpReg.Freeze() per the
//     header_mutation + oauth2 + lua precedent (the registry rejects post-
//     Freeze registrations; New runs during listener construction AFTER
//     Freeze, so it cannot self-register the validator). At 25.1 + 25.2 the
//     validator one-liner returns the arm-18 PARSE-REJECT
//     "wasm: per-route configuration is not yet supported (lands in
//     phase 25.3)" per ADR-0110 single-chokepoint + parent §6.2 arm 18 +
//     AMEND-A3 5th-canonical REUSE-by-absence. 25.3 IMPL replaces the body
//     with the NEW 5th-canonical WasmPerRoute wholesale-override validator.
//
// # BRAINSTORM Q1-Q9 decision summary (per parent BRAINSTORM)
//
//   - Q1 PARENT SCOPE (envelope D): 25.1 lands VM runtime + headers-bridge
//     subset; 25.2 lands full envelope D + advanced bridge delta + outbound
//     HTTP/gRPC; 25.3 lands 5th-canonical WasmPerRoute + multi-plugin VM-
//     sharing + conformance harness seed. PARENT 3-way pre-split LOCKED at
//     parent BRAINSTORM Q1.
//   - Q2 WAZERO DEP: github.com/tetratelabs/wazero v1.10.1 pinned per
//     AMEND-A1; pure-Go WebAssembly runtime; no CGO; Apache-2.0 license.
//     `wasm32-wasip1` rustc target + proxy-wasm-rust-sdk = 0.2.4 for the
//     fixture-0034 vendored bytecode source.
//   - Q3 ENVELOPE-D BRIDGE: pragmatic-middle headers + log + minimal property
//     tree + send_local_response at 25.1; defer body / trailers / full property
//     tree / outbound HTTP / outbound gRPC / shared-data / shared-queue /
//     timer / metric / foreign-function to 25.2; per-route + multi-plugin +
//     conformance harness to 25.3.
//   - Q4 EXTRACT-NOW vs DEFER framework primitive: EXTRACT-NOW per ADR-0202.
//     NEW `internal/wasm/` framework primitive at first-consumer scope; future
//     consumers (cluster-specifier-wasm; access-logger-wasm; network-filter-
//     wasm) reuse the same primitive at API-revision-allowed scope per the
//     phase-22.1 EXPLICIT API-REVISION ALLOWANCE clause precedent.
//   - Q5 DEFAULT-DENY CAPABILITY SANDBOX: SandboxConfig zero-value posture is
//     StrictDefaultDeny per AMEND-A5 (envoy-go-strict DEPARTURE recorded at
//     BEHAVIOR_CONTRACT.md final-Task 6-edit bundle per 25.1 SPEC §13.5
//     edit #3). Rationale: WASM has a substantially larger and riskier
//     hostcall surface than Lua; upstream Envoy v1.37.2 marks its 3 sandbox
//     runtimes (V8, WAMR, Wasmtime) as `status: alpha` + `security_posture:
//     unknown` — the alpha-status posture is incompatible with envoy-go's
//     safe-by-default discipline.
//   - Q6 4-ARM AsyncDataSource.Local: PARSE-REJECT envoy-go-strict for
//     AsyncDataSource.Remote (upstream supports remote fetching;
//     envoy-go-strict requires local-only at 25.1). The 4 arms are
//     (a) inline-bytes (b) inline-string (c) filename (d) environment-
//     variable. Lands at Task 10.
//   - Q7 5TH-CANONICAL REUSE-by-absence: WasmPerRoute absence CONFIRMED at
//     25.1 (no per-route Wasm); ADR-0125 STAYS at 10 canonicals; NO §(xvi)
//     amendment per AMEND-A3. 25.3 activates the per-route 5th-canonical
//     wholesale-override.
//   - Q8 CROSS-SIDE FIXTURE: fixture-0034-http-wasm-headers-bridge with 7
//     wire-interactive scenarios — full cross-side byte-exact via existing
//     CompareBytes; fixture-0035-http-wasm-boot-reject for the
//     PARSE-REJECT arm coverage. NEW BackendKind=HTTPWasm.
//   - Q9 RUST-SOURCED VENDORED BYTECODE: fixture-0034 ships precompiled
//     `wasm32-wasip1` bytecode binaries sourced from a small Rust SDK
//     scaffold (proxy-wasm-rust-sdk = 0.2.4); the Rust source is vendored
//     under testdata/rust-src/ for reproducibility, the compiled .wasm
//     binaries land under testdata/wasm/ for fixture consumption.
//
// # AMEND-A1..A9 cross-references (per parent SPEC §1.1)
//
//   - AMEND-A1: wazero v1.10.1 pin + Apache-2.0 license correction +
//     `wasm32-wasip1` target + proxy-wasm-rust-sdk = 0.2.4 +
//     SanitizationConfig accept-empty discipline + allow_precompiled /
//     nack_on_code_cache_miss PARSE-REJECT additions.
//   - AMEND-A2: stat-roster STRUCTURAL REFUTATION → 5-counter tri-group
//     structure; HCM-stats_prefix DROPPED; no vm_id discriminator.
//     Anchors §stats.go at Task 8.
//   - AMEND-A3: WasmPerRoute absence CONFIRMED; ADR-0125 STAYS at 10;
//     NO §(xvi) amendment. Anchors §validatePerRouteWasm at Task 8.
//   - AMEND-A4: wazero-vs-V8 byte-exact CONFIRMS with §4.5 D6 guardrails;
//     directly informs fixture-0034 7-scenario authoring at Task 15.
//   - AMEND-A5: default-deny capability roster CONFIRMS envoy-go-strict
//     departure; directly informs §3.3 sandbox roster at internal/wasm/
//     sandbox.go (Task 6) + ADR-0204.
//   - AMEND-A6: proxy-wasm v0.1.0 + v0.2.0 PARSE-REJECT envoy-go-strict-
//     stricter departure; directly informs §3.4 hostcall registration +
//     bytecode_util.go byte-faithful detection at Task 2 + parent §6.2
//     arm 16.
//   - AMEND-A7: WasmResult 10-value with value-gaps + WasmBufferType
//     FOREIGN_FUNCTION_ARGUMENTS=8; directly informs §3.2 `abi/types.go`
//     value-faithful encoding at Task 1.
//   - AMEND-A8: conformance source REFINED — 25.3 territory, NOT 25.1
//     surface.
//   - AMEND-A9: foreign-function disposition RATIFIES option (b) — 25.2
//     territory, NOT 25.1 surface.
//
// # D-P1..D-P6 cross-references (per 25.1 SPEC §11 + §12)
//
//   - D-P1 — WASI denial errno disposition (NOTSUP=58 vs ENOTCAPABLE=76):
//     closed at IMPL Task 2 first-action (upstream `proxy_wasm_exports.h:
//     232-249` scrape).
//   - D-P2 — module-init/allocator callback capability-gating disposition
//     (5 callbacks ungated): closed at IMPL Task 6 first-action (upstream
//     `proxy-wasm-cpp-host:wasm.cc:298-302` getFunction scrape).
//   - D-P3 — ADR-0196 ResponseStatus first co-consumer: closes at IMPL
//     Task 11 (abi_callbacks.go `proxy_get_status` impl).
//   - D-P4 — reserved.
//   - D-P5 — 18-arm PARSE-REJECT byte-stable wording: closes at IMPL Task 9
//     (compiled_config.go).
//   - D-P6 — fixture-0035 PARSE-REJECT scenario disposition: closes at
//     IMPL Task 16.
//
// # File split (per 25.1 SPEC §3.5)
//
// 7 production files + N test files. At Task 8 only doc.go + wasm.go +
// stats.go + wasm_test.go land. Subsequent tasks add:
//   - Task 9:  compiled_config.go + compiled_config_test.go (18-arm
//     PARSE-REJECT roster + buildCompiledConfig + valid-config rows).
//   - Task 10: datasource.go + datasource_test.go (4-arm AsyncDataSource.Local
//     resolution + Remote PARSE-REJECT + file-read failure paths).
//   - Task 11: abi_callbacks.go + abi_callbacks_test.go (ABICallbacks impl +
//     headers-bridge subset + send_local_response capture +
//     `proxy_get_status` ADR-0196 first co-consumer).
//   - Task 12: decode_headers.go + encode_headers.go (proxy_on_request_headers
//   - proxy_on_response_headers dispatch + ProxyAction handling +
//     OnDestroy lifecycle).
//   - Task 14: fuzz_test.go (34th project-wide fuzzer
//     FuzzWasmConfigParse).
//
// # Per-route discipline (PARSE-REJECT at 25.1 + 25.2; 5th canonical at 25.3)
//
// WasmPerRoute PARSE-REJECTs at any tier (Route / VirtualHost /
// RouteConfiguration / listener-typed_per_filter_config) via
// RegisterPerRouteValidator per ADR-0110 single-chokepoint + parent §6.2
// arm 18 + AMEND-A3. Wording: "wasm: per-route configuration is not yet
// supported (lands in phase 25.3)". 25.3 IMPL replaces the body with the
// NEW 5th-canonical wholesale-override validator (WasmPerRoute proto
// wholesale-overrides the listener-config; ADR-0125 STAYS at 10 canonicals
// — NO §(xvi) amendment per AMEND-A3 REUSE-by-absence).
//
// # 5-counter stat surface (per AMEND-A2 + parent §7)
//
// Tri-group prefix structure DIVERGES from the dominant §9 family-row pattern:
//
//   - Group B (`wasm.<runtime>.*`; `<runtime>` = `"wazero"` uniformly):
//     `wasm.wazero.created` (counter) + `wasm.wazero.active` (gauge).
//     Upstream-parity.
//
//   - envoy-go-strict extensions (`wasm.<plugin_name>.*`):
//     `wasm.<plugin_name>.executions` (counter; per
//     proxy_on_request_headers invocation) + `wasm.<plugin_name>.
//     hostcall_denied` (counter; per default-denied hostcall) +
//     `wasm.<plugin_name>.envoy_go.failures` (counter; per VM-failure event).
//     Each requires a BEHAVIOR_CONTRACT.md final-Task departure record.
//
// HCM-injected `stats_prefix` is DROPPED per AMEND-A2 (upstream
// `source/extensions/filters/http/wasm/config.h:51-53` does NOT inject the
// HCM `stats_prefix`; the wasm filter row DIVERGES from the dominant §9
// family-row pattern but NOT from upstream). Structural-note row at
// BEHAVIOR_CONTRACT.md final-Task captures the family-row divergence.
//
// Project-wide stat-count delta: 114 → 119 per parent §7. Verified at the
// `+5 per-call delta` test in wasm_test.go (TestNewFilterStats_ProjectStatCountDelta).
//
// # Cross-references
//
//   - ADR-0202 (NEW internal/wasm/ framework primitive; bodies land at
//     Task 17).
//   - ADR-0203 (NEW internal/filter/http/wasm/ package shape; bodies land
//     at Task 17).
//   - ADR-0204 (proxy-wasm default-deny capability sandbox + envoy-go-strict
//     posture; bodies land at Task 17).
//   - ADR-0070 (filter-registration convention; filterName).
//   - ADR-0071 (two-step factory; HTTPFilterFactory return signature).
//   - ADR-0072 (boot-time-fail-fast).
//   - ADR-0085 (nil-tolerance; newFilterStats(nil, ...) returns nil).
//   - ADR-0110 (per-route validator single-chokepoint).
//   - ADR-0117 (NewCounterIfAbsent / NewGaugeIfAbsent for Group-B shared
//     namespace stats).
//   - ADR-0125 (per-route canonical-shape roster; STAYS at 10 per AMEND-A3).
//   - ADR-0143 SN1 + SN2 (byte-exact TypeURL pin + stat-name SN2-reuse).
//   - 25 parent SPEC + 25.1 SPEC §3.5 + §4 + §5 + §7.
package wasm
