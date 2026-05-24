# Phase 25 — `envoy.extensions.filters.http.wasm.v3.Wasm` (parent master SPEC)

**Phase id:** `25`
**Slug:** `25-http-filter-wasm`
**Status:** `in-progress` (SPEC stage; **3-way pre-split LOCKED at BRAINSTORM Q1** — sub-phases `25.1-http-filter-wasm-runtime-and-headers-bridge` + `25.2-http-filter-wasm-body-and-advanced-bridge` + `25.3-http-filter-wasm-perroute-and-conformance`; no SPEC-time split re-evaluation per §3.0)
**Produced by:** `superpowers:writing-plans` SCOPED to parent SPEC authoring (lifecycle-state 1 → 2 per ADR-0005 §Decision 4; transcribes `docs/envoy-go/phases/25-http-filter-wasm/BRAINSTORM.md` into formal SPEC shape, resolves the 9 §10 D1–D9 empirical pins IN-SESSION via parallel-subagent fan-out against reference Envoy v1.37.2 + v1.32.4 go-control-plane proto bindings + proxy-wasm-cpp-host@`da3ce05d` + the proxy-wasm v0.2.1 spec README + wazero `main` + proxy-wasm-rust-sdk v0.2.4 per ADR-0004, anchors the 3 NEW ADR §Context drafts (ADR-0202 + ADR-0203 + ADR-0204), and points down to the per-sub-phase SPEC sessions for surface detail per the phase-22.1/22.2/22.3 precedent)
**Depends on:** phase 24 (`done` at master `94c2b78` — phase-24.2 IMPL squash that closed the parent row 24 per the 18/19/22 ROLLUP precedent; SHA-fill follow-up `ec0ee80`; cold-start rewrite `35f4c73`; tip repoint `1ab2014`; phase-25 BRAINSTORM at `c17cbf0`; BRAINSTORM SHA-fill `29aa1db`; tip repoint `1f773cf` = master tip + this SPEC session's base commit)
**Sub-phases:** `25.1-http-filter-wasm-runtime-and-headers-bridge`, `25.2-http-filter-wasm-body-and-advanced-bridge`, `25.3-http-filter-wasm-perroute-and-conformance`
**Authored:** 2026-05-24
**Differential surface at end of phase:** ROADMAP rows `25.1`, `25.2`, and `25.3` all `done`; the parent row `25` flips `in-progress → done` AT THE SAME phase-done commit as `25.3`'s phase-done (mirroring the 05/05.1/05.2 + 06/06.1/06.2 + 07/07.1/07.2 + 08/08.1/08.2 + 18/18.1/18.2 + 19/19.1/19.2 + 22/22.1/22.2/22.3 + 24/24.1/24.2 closure pattern). Cumulatively across the three sub-phases: NEW `internal/wasm/` framework primitive (ADR-0202); NEW `internal/filter/http/wasm/` package (ADR-0203); NEW differential fixtures `0034-http-wasm-headers-bridge` (25.1, full cross-side byte-exact for headers-bridge scenarios), `0036-http-wasm-body-and-advanced` (25.2, partial cross-side / mixed-mode for non-deterministic legs per ADR-0192 precedent), `0038-http-wasm-perroute-and-multi-plugin` (25.3, cross-side byte-exact for deterministic per-route + multi-plugin scenarios), plus their boot-reject siblings `0035-http-wasm-boot-reject` + `0037-http-wasm-body-and-advanced-boot-reject` + `0039-http-wasm-perroute-boot-reject` are all differentially green; pre-existing fixtures `0000`–`0033` remain green; the h2spec conformance gate is unchanged at 53/53 PASS at the ADR-0051 pin; three new fuzzers (`FuzzWasmConfigParse` at 25.1; `FuzzWasmHostcallEnvelope` at 25.2; `FuzzWasmPerRouteConfig` at 25.3); NEW conformance harness `test/conformance/proxy-wasm/` at 25.3 (10/16 cpp-host test families ported = 62.5% starting pass-threshold per ADR-0212).

---

## 1. Mission summary

Phase 25 lands `envoy.extensions.filters.http.wasm.v3.Wasm` — Envoy's canonical HTTP WebAssembly filter, the FINAL §9 family-row and the SECOND §9 row whose configuration **delegates per-request behavior to operator-authored Turing-complete bytecode** (phase-22 lua introduced the first such filter; phase-25 introduces the second via WebAssembly) — as the EIGHTEENTH and FINAL production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), buffer (13), compressor (14), bandwidth_limit (15), rbac (16), jwt_authn (17), ext_authz (18 with 18.1+18.2), ext_proc (19 with 19.1+19.2), oauth2 (20), adaptive_concurrency (21), lua (22 with 22.1+22.2+22.3), admission_control (23), and global_ratelimit (24 with 24.1+24.2). Per the phase-25 BRAINSTORM's 9-question dialogue plus 3 post-synthesis confirmations the MVP envelope is: **Envelope D — full upstream parity** (per Q1: full wazero VM + full proxy-wasm v0.2.1 HTTP-filter host ABI + named `vm_id`-keyed VM sharing + `PluginConfig.capability_restriction_config` + per-route `typed_per_filter_config` wholesale-override) delivered across THREE sub-phases (per Q1 + Q7 3-way pre-split at BRAINSTORM time); **WASM runtime = `github.com/tetratelabs/wazero` v1.10.1** (per Q2 + parent SPEC AMEND-A1 pin recommendation; pure-Go WebAssembly 1.0+2.0 conformant; Apache-2.0-licensed; NO CGO; floor Go 1.23.0); **NEW `internal/wasm/` framework primitive at 25.1 first-consumer** (per Q3 + Q4 EXTRACT-NOW — ENDS phase-24 ZERO-NEW-package-level-primitive disposition; SECOND occurrence of EXTRACT-NOW-at-first-consumer after phase-22.1 `internal/lua/`; ADR-0202); **in-house proxy-wasm v0.2.1 host ABI at `internal/wasm/abi/`** (per Q3: 47 hostcalls — 40 `proxy_*` + 7 mandatory `wasi_snapshot_preview1.*` shims — plus 30 guest-exported callbacks; NO third-party `proxy-wasm-go-host` dependency); **envoy-go-strict default-deny capability roster** (per Q4: empty `allowed_capabilities` → DENY-ALL in envoy-go vs UPSTREAM's ALLOW-ALL — departure from upstream behavior; ADR-0204); **5th-canonical wholesale-override per-route REUSE-by-absence** (per Q5 + parent SPEC AMEND-A3 CONFIRMS: NO `WasmPerRoute` proto exists in v1.32.4 binding OR v1.37.2 IDL; per-route override is wholesale-override via TPFC of the `Wasm` message itself; ADR-0125 STAYS at 10 canonicals + NO §(xvi) amendment); **Pragmatic-middle 25.1 hostcall surface** (per Q7 FEATURE-PROGRESSIVE: 24 hostcalls land at 25.1 — see §4.2); **7-counter 25.1 stat surface** (per Q8 + parent SPEC AMEND-A2: REFUTES the BRAINSTORM-hypothesized flat `wasm.<vm_id>.<stat>` template — upstream uses tri-group structure with `wasm.<runtime>.{created,active}` + `wasm.<plugin_name>.{vm_reload*}` + flat `wasm.remote_load_*`); **full cross-side byte-exact fixture-0034 for headers-bridge** (per Q6 + parent SPEC AMEND-A4 CONFIRMS: wazero compiler-mode is WebAssembly Core 1.0+2.0 spectest-green; proxy-wasm ABI is wire-defined + host-owned formatting; 25.1 hostcall subset is BYTE-EXACT-SAFE with §4.5 guardrails); **Rust-vendored pre-built `.wasm` test bytecode under `test/fixtures/wasm-plugins/`** (per Q9: proxy-wasm-rust-sdk v0.2.4 + `wasm32-wasip1` target per Rust 1.78+ rename — NOT the BRAINSTORM-quoted `wasm32-wasi`).

Phase 25 is also: (i) the FINAL §9 family-row — closes the §9 HTTP-filters family roster after 18 row-landings spread across phases 07.1 → 25.3; subsequent expansion lives in the broader `WASM host family` per `BOOTSTRAP_PROMPT.md §9` line 116 (cluster-specifier-wasm at `envoy.router.cluster_specifiers.wasm`; access-logger-wasm at `envoy.access_loggers.wasm`; network-filter-wasm at `envoy.filters.network.wasm`; WasmService singleton plugins) which CONSUMES the `internal/wasm/` primitive landed here at multi-consumer scope; (ii) the SECOND §9 row to introduce a **third-party VM-class dependency** (`github.com/tetratelabs/wazero` v1.10.1 per AMEND-A1; first was phase-22's gopher-lua v1.1.2 per ADR-0188); (iii) the SECOND §9 row to **pre-split THREE-way at BRAINSTORM time** (phase-22 lua was first; ADR-0162 precedent); (iv) the FIRST §9 row to **author a complete in-house proxy-wasm v0.2.1 host ABI implementation** — 47 hostcalls across header-map / buffer / shared-data / log / metric / dispatch / property / context-lifecycle / foreign-function / stream-info / WASI families plus 30 guest-exported callbacks; (v) the FIRST §9 row whose underlying runtime LICENSE is **Apache-2.0** (wazero) rather than the MIT-licensed gopher-lua precedent (parent SPEC AMEND-A1 corrects the BRAINSTORM §2.3 "MIT-licensed" typo); (vi) the FIRST §9 row to seed `test/conformance/proxy-wasm/` per `BOOTSTRAP_PROMPT.md §7.3` anticipation — at 25.3 with a 62.5% starting pass-threshold against proxy-wasm-cpp-host@`da3ce05d` (parent SPEC AMEND-A8 REFUTES the BRAINSTORM-implicit assumption that the conformance suite lives in `proxy-wasm/spec` — that repo ships NO test suite).

This parent master SPEC carries the cross-cutting design that applies to ALL THREE sub-phases — the envelope-D scope concept, the shared `compiledConfig` shape with phased-PARSE-REJECT, the per-stream wazero VM execution discipline, the `internal/wasm/` framework primitive's public API surface, the full §11 empirical-pin block (all 9 §10 D1–D9 pins resolved IN-SESSION this session per ADR-0004 — they span all three sub-phases, so they live here once), the §1.1 empirical-finding-driven scope-revision (AMEND) block, the §9 BEHAVIOR_CONTRACT delta, and the §13 verbatim Markdown patch anticipation. It points down to each sub-phase's authoritative SPEC for the per-surface detail.

After phase 25, the project has proven its eighteenth and final §9 engineering claim: *envoy-go's HTTP filter framework hosts a fully-operational WebAssembly-bytecode-driven HTTP filter that compiles operator-supplied `.wasm` modules from the 4-arm AsyncDataSource Local envelope at config-load (Remote arm + WatchedDirectory sibling field PARSE-REJECT per §6.2; runtime discriminators `envoy.wasm.runtime.{v8,wamr,wasmtime,null}` PARSE-REJECT per envoy-go-strict departure; proxy-wasm ABI v0.1.0 + v0.2.0 PARSE-REJECT per §6.2 envoy-go-strict-stricter-than-upstream departure per AMEND-A6), dispatches per-stream into a fresh wazero VM (per-stream `*wazero.Runtime` construction with shared module compile cache; sandboxed default-deny per §4.3 with an envoy-go-strict departure from upstream's default-allow per AMEND-A5), invokes the 30 proxy-wasm v0.2.1 guest-exported callbacks against the 47-hostcall in-house host ABI for headers-bridge manipulation + body/buffer access + timer + metrics + shared-data + outbound HTTP dispatch + property access + log + send-local-response (full bridge surface at 25.2 IMPL), supports per-route override via 5th-canonical wholesale-override TPFC (REUSE-by-absence per Q5 + AMEND-A3 — no NEW canonical) + multi-plugin VM-sharing via `vm_id` discriminator at 25.3 IMPL, publishes 7 counters under tri-group prefix structure (`wasm.<runtime>.{created,active}` + `wasm.<plugin_name>.{vm_reload,vm_reload_backoff,vm_reload_success,vm_reload_failure}` per AMEND-A2 + 2 envoy-go-strict extensions `executions` + `hostcall_denied`), and is OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on every axis except the documented divergence-windows (default-deny capability posture per AMEND-A5; v0.1.0/v0.2.0 ABI PARSE-REJECT per AMEND-A6; `executions` + `hostcall_denied` envoy-go-strict counters per AMEND-A2; 0-vs-10 foreign-function default registry per AMEND-A7).*

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 9 §10 D1–D9 empirical pins (executed at this SPEC session via parallel-subagent fan-out against `envoyproxy/envoy@v1.37.2` C++ source + the `go-control-plane@v1.32.4` proto bindings in the local module cache + `proxy-wasm/proxy-wasm-cpp-host@da3ce05d` + the `proxy-wasm/spec@main` ABI v0.2.1 README + `wazero/wazero@main` README + CI surface + `proxy-wasm/proxy-wasm-rust-sdk@v0.2.4`) generated the following **9 AMEND-block entries** — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM. **Six are substantive REFUTATIONS** (AMEND-A1 wazero pin + license correction; AMEND-A2 stat-roster structural REFUTATION; AMEND-A3 WasmPerRoute-absence CONFIRMED with EXPLICIT-NO-CANONICAL classification; AMEND-A6 ABI-version PARSE-REJECT is envoy-go-strict-stricter departure not parity; AMEND-A7 WasmResult/WasmBufferType enum value REFUTATIONS; AMEND-A8 conformance-suite source REFUTATION); two are CONFIRMS-with-refinement (A4 + A5); one is RATIFIES-with-extensions (A9).

- **AMEND-A1 (wazero version pin + license correction — REFUTES BRAINSTORM §2.3 "MIT-licensed" + REFINES the version pin; driven by §11.5 D5):** The BRAINSTORM §2.3 narrative claims `github.com/tetratelabs/wazero` is "MIT-licensed pure-Go WebAssembly 1.0 (MVP) runtime". The §11.5 D5 scrape REFUTES on the license: per `https://raw.githubusercontent.com/wazero/wazero/main/LICENSE` (the repo also moved from `tetratelabs/wazero` → `wazero/wazero`; the `tetratelabs` org redirects), wazero is **Apache-2.0-licensed**, not MIT. The §1 narrative reflects the correction. REFINES the version pin: the latest stable `v1.11.0` (2025-12-19) bumps the floor Go version to **1.24.0** and adds `golang.org/x/sys` as a direct dependency — incompatible with envoy-go's `go 1.23.0` floor (per `/home/esa/git/envoy-go/.worktrees/phase-25-http-filter-wasm-spec/go.mod` line 3). **Pin: `github.com/tetratelabs/wazero v1.10.1`** (2025-11-11; Go 1.23.0 floor; tag commit `ee3f9d9c5c6689bbf30824aec049371aaa239f4c`). Carries the v1.10.0-line features (concurrent compilation #2381; tail-call proposal opt-in #2403; W^X security default #2429) without bumping envoy-go's Go floor. Fallback ordering: `v1.9.0` (2025-02-19) → `v1.8.2` (2024-11-25). DO NOT fall forward to `v1.11.0` without a separate envoy-go Go-floor-bump phase. Also REFINES the wasm target identifier — Rust renamed `wasm32-wasi` → `wasm32-wasip1` in Rust 1.78 (April 2024); proxy-wasm-rust-sdk examples use the post-rename name; the BRAINSTORM §2.9 + §10 D5 quoted-text `wasm32-wasi` is updated to `wasm32-wasip1` at this SPEC commit. proxy-wasm-rust-sdk pin: **`=0.2.4`** (2025-10-01; ABI v0.2.1 export-symbol verified at `src/lib.rs:proxy_abi_version_0_2_1`); v0.2.5 is 4 days fresh at SPEC-authoring date and fails the ADR-0008-equivalent soak threshold.

- **AMEND-A2 (stat-surface REFUTES BRAINSTORM §5.1 + §8 + §10 D2 — structural REFUTATION; driven by §11.2 D2):** The BRAINSTORM §5.1 hypothesized 5 upstream-parity counters (`vm_created`, `vm_failed`, `plugin_created`, `plugin_failed`, `plugin_runtime_error`) + 2 envoy-go-strict extensions (`executions`, `hostcall_denied`) under a flat `wasm.<vm_id>.<stat>` prefix template. The §11.2 D2 empirical scrape against `envoyproxy/envoy@v1.37.2:source/extensions/common/wasm/stats_handler.h` REFUTES the entire roster + the prefix template. The actual upstream surface is **TRI-GROUP**:
  - **Group A (`CreateWasmStats`; flat `wasm.` prefix; process-global remote-fetch cache):** 5 counters `remote_load_cache_hits` + `remote_load_cache_negative_hits` + `remote_load_cache_misses` + `remote_load_fetch_successes` + `remote_load_fetch_failures` + 1 gauge `remote_load_cache_entries` (`stats_handler.h:22-28`). **DEFERRED** at envoy-go phase 25 because `AsyncDataSource.Remote` is PARSE-REJECT'd per §6.2 — no remote bytecode fetch path.
  - **Group B (`LifecycleStats`; `wasm.<runtime>.` prefix; per-VM-runtime):** 1 counter `created` + 1 gauge `active` (`stats_handler.h:34-36`). Fires `VmCreated`/`VmShutDown` events at `wasm.cc:81,98,142`. envoy-go's `<runtime>` is uniformly `"wazero"` because no other runtime is exposed (the upstream-distinct runtime discriminators are PARSE-REJECT'd per §6.2).
  - **Group C (`WasmStats`; `wasm.<plugin_name>.` prefix; per-PluginConfig):** 4 counters `vm_reload` + `vm_reload_backoff` + `vm_reload_success` + `vm_reload_failure` (`stats_handler.h:109-113`). Fire sites at `stats_handler.cc:91,94,97` via `wasm.cc:514,525,527,531`. Note `vm_reload` is declared-but-never-incremented in v1.37.2 (orphan counter). **DEFERRED** at envoy-go phase 25.1: the VM-reload machinery requires `PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig` — neither lands until 25.3 (per §3.1 surface-mapping). 25.3 BRAINSTORM/SPEC re-evaluates whether to land Group C or skip.
  - **NO `vm_id` discriminator anywhere.** The BRAINSTORM-hypothesized `wasm.<vm_id>.<stat>` template does NOT exist in upstream Envoy v1.37.2.
  - **HCM-injected `stats_prefix` is DROPPED.** Per `source/extensions/filters/http/wasm/config.h:51-53` (the typed-factory signature: `(... const std::string&, FactoryContext&)` with the second parameter UNNAMED), the HCM-rooted `http.<HCM_stat_prefix>.wasm....` template does NOT apply. wasm stats anchor at `FactoryContext::scope()` root — NOT at the per-HCM-listener filter prefix that every other §9 filter uses. This is a unique structural property of the wasm filter row.
  - **No counter named `vm_failed`/`plugin_created`/`plugin_failed`/`plugin_runtime_error` exists.** The BRAINSTORM-hypothesized roster is a fabrication; the closest upstream analogue for "VM failure" is the `FailState` enum (set on the `Wasm` object) which is reachable via lifecycle event dispatch but does NOT increment a counter.

  **Net 25.1 stat surface (REFINED per AMEND-A2):** 5 counters — `wasm.wazero.created` (Group B upstream-parity; renamed via the runtime discriminator) + `wasm.wazero.active` gauge (Group B) + `wasm.<plugin_name>.executions` (envoy-go-strict extension; counter per `proxy_on_request_headers` invocation) + `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict extension; counter per default-denied hostcall invocation per AMEND-A5 + ADR-0204) + `wasm.<plugin_name>.envoy_go.failures` (envoy-go-strict replacement for the upstream `FailState`-via-event surface — gives operators a single observable counter for "the VM died" without recreating the `vm_reload_*` mechanism deferred to 25.3). Project stat count **114 → 119** at 25.1 (+5, NOT +7 as the BRAINSTORM hypothesized). The 25.2 + 25.3 BRAINSTORMs/SPECs may add additional envoy-go-strict counters (`tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied` were the BRAINSTORM §5.2 forward-pointers). The 3 envoy-go-strict counters (`executions` + `hostcall_denied` + `envoy_go.failures`) require BEHAVIOR_CONTRACT.md envoy-go-strict departure records at the 25.1 IMPL final Task bundle (parallel to phase-22's `respond_calls` + stdlib-sandbox-strict + runtime-error-wording departures — phase-25 has 5-6 total departure records anticipated at 25.1 IMPL per §13).

- **AMEND-A3 (`WasmPerRoute` absence CONFIRMED — EXPLICIT-NO-CANONICAL classification per Q5; driven by §11.1 D3):** BRAINSTORM §2.6 + §4 + §10 D3 hypothesized that the per-route Wasm override is the 5th-canonical wholesale-override pattern via TPFC because no dedicated `WasmPerRoute` message was visible at brainstorm-time proto inspection. The §11.1 D3 empirical scrape **CONFIRMS DEFINITIVELY**: there is NO `WasmPerRoute` message in `envoy.extensions.filters.http.wasm.v3` in v1.32.4 binding (`FILTER_PB:153-156` declares `NumMessages: 1`; the GoTypes vector at `FILTER_PB:117-118` lists exactly `(*Wasm)(nil)` + the imported `(*v3.PluginConfig)(nil)`) AND no `WasmPerRoute` in the v1.37.2 IDL (verified via WebFetch against `api/envoy/extensions/filters/http/wasm/v3/wasm.proto`). Per-route Wasm override is wholesale-override of the parent `Wasm` message itself via `typed_per_filter_config`. **Classification: 5th-canonical REUSE-by-absence (EXPLICIT-NO-NEW-CANONICAL per ADR-0173 / ADR-0180 precedent).** ADR-0125 roster STAYS at 10 canonicals — **NO §(xvi) amendment at phase 25.3 IMPL**. The escape-valve hypothesis "ADR-0125 amendment 10 → 11" from BRAINSTORM §10 D3 + §7.3 is RETIRED at this SPEC commit. 25.3 anchors the EXPLICIT-NO-NEW-CANONICAL decision via a new ADR analogous to ADR-0180 (phase-20 oauth2's no-amendment classification ADR) — provisionally **ADR-0210** per §10 anchor map.

- **AMEND-A4 (wazero-vs-V8 byte-exact CONFIRMS BRAINSTORM §2.7 hypothesis with guardrails — driven by §11.6 D6):** BRAINSTORM §2.7 + §10 D6 committed to full cross-side byte-exact differential between envoy-go (wazero) and reference Envoy v1.37.2 (V8) on the headers-bridge hostcall subset. The §11.6 D6 empirical scrape CONFIRMS the hypothesis with three structural reasons: (i) wazero's compiler-mode `internal/integration_test/spectest/v{1,2}/` passes both WebAssembly Core 1.0 + 2.0 spec tests (1000+ testdata files on amd64/arm64; CI-gated); (ii) proxy-wasm ABI v0.2.1 is FINAL + wire-defined (host owns ALL serialization — pairs encoding, WasmResult enum, log line formatting); (iii) zero open wazero bugs on compiler-mode WebAssembly 1.0 semantics (the only bug-labeled issue affecting wasm-1.0 correctness, #2371 `i32.ne` for host-function negative numbers, is INTERPRETER-ONLY and we default to compiler mode). The 25.1 headers-bridge surface (proxy_log, proxy_get/set/add/replace/remove_header_map_pairs/value, proxy_send_local_response, proxy_get/set_property, proxy_get_status, plus the 6 lifecycle hooks + 2 HTTP hooks) is **BYTE-EXACT-SAFE** modulo the §4.5 guardrails. **Guardrail bit at §4.5:** fixture-0034 scenarios MUST NOT (a) trigger linear-memory traps (avoid OOM probes — defer to 25.2), (b) depend on HTTP/2 header iteration order (use HTTP/1.1 OR sort-on-assertion), (c) emit log lines containing float-formatted numbers (no floats on the 25.1 wire anyway), (d) call any hostcall outside the headers-bridge subset. The 25.2 fixture-0036 expects mixed-mode partial-cross-side per the phase-22.2 ADR-0192 precedent because `proxy_dispatch_http_call` + `proxy_on_tick` + `proxy_define_metric` introduce non-determinism that complicates cross-side byte-comparison.

- **AMEND-A5 (default-deny capability roster CONFIRMS BRAINSTORM §2.5 envoy-go-strict departure — driven by §11.4 D7):** BRAINSTORM §2.5 + §10 D7 committed envoy-go-strict to a default-DENY capability roster, departing from upstream Envoy's default-ALLOW posture. The §11.4 D7 empirical scrape against `proxy-wasm/proxy-wasm-cpp-host@main:include/proxy-wasm/wasm.h:103-106` CONFIRMS the upstream gate function `capabilityAllowed`: `return allowed_capabilities_.empty() || allowed_capabilities_.find(capability_name) != allowed_capabilities_.end();` — when the map is empty, ALL hostcalls allowed. envoy-go's strict default-deny gate inverts the empty-map semantic. The complete capability key roster is ~80 entries (37 core `proxy_*` hostcalls + 7 ABI-versioned hostcalls + 12-of-43 implemented WASI hostcalls + 24 module-getter exports — full enumeration in §4.3.4). Denied invocations return `WasmResult::InternalFailure` (=10) + emit an integration error log + increment `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict counter per AMEND-A2). **envoy-go-strict departure record at IMPL** (per §13): the default-deny posture is documented as an envoy-go-strict departure from upstream's bare-empty-map-allow-all posture; rationale: WASM has a substantially larger and riskier hostcall surface than Lua (proxy_call_foreign_function for arbitrary host-side dispatch; proxy_dispatch_http_call for outbound network; proxy_set_shared_data for cross-stream state; proxy_define_metric for unbounded dynamic-stat namespace creation); upstream Envoy v1.37.2 marks its 3 sandbox runtimes (V8, WAMR, Wasmtime) as `status: alpha` + `security_posture: unknown` (per `source/extensions/extensions_metadata.yaml:1631-1635`) — the alpha-status posture is incompatible with envoy-go's safe-by-default discipline.

- **AMEND-A6 (proxy-wasm v0.1.0 + v0.2.0 PARSE-REJECT is envoy-go-strict-stricter departure — REFUTES BRAINSTORM §2.4 "DEPRECATED upstream" framing; driven by §11.5 D4):** BRAINSTORM §2.4 + §8 item 3 framed the proxy-wasm v0.1.0 ABI as "DEPRECATED upstream — PARSE-REJECT". The §11.5 D4 empirical scrape against `proxy-wasm/proxy-wasm-cpp-host@main:src/wasm.cc:244-249` REFUTES the "DEPRECATED upstream" framing: upstream Envoy v1.37.2 ACCEPTS all three ABI versions (v0.1.0, v0.2.0, v0.2.1) and version-dispatches the registered hostcall + callback sets via `registerCallbacks` (`wasm.cc:286-293`) + `getFunctions` (`wasm.cc:298-302`). Only `AbiVersion::Unknown` (no sentinel export found in the module's export section type 7) is rejected upstream at `wasm.cc:248-249` with `"Missing or unknown Proxy-Wasm ABI version"`. envoy-go-strict's PARSE-REJECT of BOTH v0.1.0 AND v0.2.0 is therefore an **envoy-go-strict-stricter departure** from upstream behavior, not a parity behavior. Rationale: maintaining version-dispatch for v0.1.0's distinct hostcall names (`proxy_get_configuration`, `proxy_continue_request`, `proxy_continue_response`, `proxy_clear_route_cache`) + v0.2.0's distinct semantics doubles the in-house ABI surface; the envoy-go-strict posture targets v0.2.1 exclusively (the FINAL, widely-implemented ABI per the v0.2.1 spec README at `proxy-wasm/spec@main:abi-versions/v0.2.1/README.md`). **Detection point at envoy-go:** reimplement `BytecodeUtil::getAbiVersion` (`bytecode_util.cc:32-97`) byte-faithfully — scan the wasm module's export section (section type 7) for a function-kind export named `proxy_abi_version_0_2_1` (24 UTF-8 ASCII bytes); PARSE-REJECT on any other sentinel value OR on absent sentinel. BEHAVIOR_CONTRACT departure record at 25.1 IMPL final Task per §13. The BRAINSTORM §8 item 3 wording is updated from "DEPRECATED upstream — PARSE-REJECT" to "envoy-go-strict-stricter departure — PARSE-REJECT v0.1.0 + v0.2.0".

- **AMEND-A7 (`WasmResult` + `WasmBufferType` enum value REFUTATIONS; driven by §11.5 D4):** BRAINSTORM §10 D4 placeholder cited a 13-value `WasmResult` enum (`Ok=0`, `NotFound=1`, `BadArgument=2`, `SerializationFailure=3`, `ParseFailure=4`, `BadExpression=5`, `InvalidMemoryAccess=6`, `Empty=7`, `CasMismatch=8`, `ResultMismatch=9`, `InternalFailure=10`, `BrokenConnection=11`, `Unimplemented=12`). The §11.5 D4 empirical scrape against the proxy-wasm v0.2.1 spec README REFUTES the 13-value count: the actual enum is **10 named values with VALUE GAPS at positions 5, 9, 11** (per `proxy-wasm/spec@main:abi-versions/v0.2.1/README.md` and cross-verified against `proxy-wasm-rust-sdk@v0.2.4:src/types.rs::Status` enum). Authoritative roster: `Ok=0`, `NotFound=1`, `BadArgument=2`, `SerializationFailure=3`, `ParseFailure=4`, (gap 5), `InvalidMemoryAccess=6`, `Empty=7`, `CasMismatch=8`, (gap 9), `InternalFailure=10`, (gap 11), `Unimplemented=12`. The BRAINSTORM-hypothesized `BadExpression=5`, `ResultMismatch=9`, `BrokenConnection=11` do NOT exist in v0.2.1. The value gaps MUST be preserved byte-faithfully — `Unimplemented` MUST encode as integer 12, not as a tightly-packed 0..9; guest modules check specific integer values and remapping would break wire compatibility. Also REFUTES `WasmBufferType` value 8 — BRAINSTORM hypothesis was `CallData=8`; actual name per the spec README is `FOREIGN_FUNCTION_ARGUMENTS=8`. `WasmHeaderMapType` 8-value roster CONFIRMS unchanged. The §4 framework primitive's `internal/wasm/abi/types.go` file MUST transcribe the 10-named-value enum + value-gap-preserving encoding + the renamed `FOREIGN_FUNCTION_ARGUMENTS` BufferType.

- **AMEND-A8 (conformance-suite source REFUTES BRAINSTORM §6.5 implicit `proxy-wasm/spec` assumption — driven by §11.7 D9):** BRAINSTORM §6.5 + §10 D9 anticipated seeding `test/conformance/proxy-wasm/` from "the proxy-wasm spec v0.2.1 conformance bytecode" (BRAINSTORM §1.1 sub-phase 25.3.g). The §11.7 D9 empirical scrape REFUTES the source: `proxy-wasm/spec@main:abi-versions/v0.2.1/` ships ONE file (a 63KB `README.md`) and nothing else — NO `.wasm` bytecode, NO test driver, NO YAML. The spec repo is a wire-spec document, NOT a test suite. **The actual canonical test surface is `proxy-wasm/proxy-wasm-cpp-host@da3ce05d:test/`** (14 vendored `.wasm` bytecode binaries compiled from Rust + C++ sources + 16 driver `.cc` files + CI matrix across V8/WAMR/Wasmtime/WasmEdge/NullVM). Bytecode SHA-pinned to `proxy-wasm-cpp-host@da3ce05d8d59ebccbfcad434bb4784c98a4ece6a` (`main` HEAD at scrape date 2026-05-20). **Pass-threshold disposition at 25.3 phase-done: 10 of 16 test families ported = 62.5% PASS** (the 6 deferred families: `null_vm_test` C++-only; `security_test` + `signature_util_test` signed-bytecode out-of-row; `shared_data_test` + `shared_queue_test` 25.2-territory IF those hostcalls land at 25.2 ELSE 25.4-or-WASM-host-family-follow-up; `fuzz/` out-of-row). Phase-05 h2spec (53/53 PASS at the ADR-0051 pin) is the closest precedent for the conformance-pinning ADR shape; phase-25.3 ADR (provisionally ADR-0212) follows the format with the scoped-subset threshold rather than the 100%-pass-on-pinned-commit precedent (the phase-22 lua mixed-mode fixture is the closer precedent for scoped-subset disposition).

- **AMEND-A9 (foreign-function disposition RATIFIES-with-extensions BRAINSTORM §2.4 + §10 D8 — driven by §11.8 D8):** BRAINSTORM §2.4 + §10 D8 left the foreign-function disposition open between (a) full registration interface with starter set, (b) registration interface + PARSE-REJECT-by-default, (c) PARSE-REJECT entirely. The §11.8 D8 empirical scrape against `envoyproxy/envoy@v1.37.2:source/extensions/common/wasm/foreign.cc` (10 registered foreign functions: `verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`) + `proxy-wasm-cpp-host@da3ce05d:src/exports.cc:147-184` (the host dispatch returning `WasmResult::NotFound` for unregistered names) **RATIFIES option (b) with extensions**: `internal/wasm/foreign.go` ships at 25.2 IMPL with a `ForeignFunctionRegistry` API (`Register(name, fn) error` + `Get(name) (ForeignFunction, bool)`); the default registry is EMPTY (zero foreign functions registered at envoy-go phase 25); the host-side `proxy_call_foreign_function` ABI shim returns `WasmResult::NotFound` (=1) for unregistered names — matches upstream wire semantic byte-exact (NOT `WasmResult::Unimplemented`=12); the capability roster default-deny per AMEND-A5 additionally denies the entire `proxy_call_foreign_function` capability at the gate layer. envoy-go-strict departure record at 25.2 BEHAVIOR_CONTRACT.md (upstream registers 10 foreign functions by default; envoy-go registers ZERO — operators must explicitly enable the `proxy_call_foreign_function` capability + the WASM host family phases register specific foreign functions at multi-consumer scope). The registration interface lands NOW (at 25.2) rather than deferring to the WASM host family per the BRAINSTORM API-REVISION ALLOWANCE clause — extracting the small (~100 LIVE LoC) interface at 25.2 frees the future cluster-specifier-wasm + access-logger-wasm + network-filter-wasm consumers from re-litigating the framework primitive's API at consumer #2.

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

Phase 24 closed at ADR-0201 (the split-application ADR for phase 24). Phase 25 anticipates **3 NEW ADRs at 25.1 IMPL** (ADR-0202 + ADR-0203 + ADR-0204) anchored §Context here + §Decision + §Consequences body lands at IMPL Lands-in-Task per ADR-0044; 25.2 + 25.3 IMPL anchor additional NEW ADRs (provisionally ADR-0205 .. ADR-0212 per §10 anchor map). **ZERO IN-PLACE §Decision AMENDMENTs** at this parent SPEC commit (ADR-0125's canonical roster STAYS at 10 entries per AMEND-A3; ADR-0188's `internal/lua/` API-REVISION ALLOWANCE clause is NOT triggered here — wasm is a separate primitive at `internal/wasm/`). **Next-free ADR after phase 25 SPEC commit advances `ADR-0202` → `ADR-0205`** (3 numbers consumed: ADR-0202..ADR-0204). ADR-0044 escape-valve reserve at ADR-0205+ for any impl-time-unanticipated ADRs at 25.1 IMPL.

**D-hypothesis at SPEC commit:** BRAINSTORM Q10 + §7.4 hypothesized WEAK-HOLD-with-2-slot-buffer at 25.1 IMPL phase-done — ADR-0202 + ADR-0203 + ADR-0204 land cleanly; 0-2 unanticipated ADRs fire from the escape-valve. This SPEC's empirical-pin scrape **STRENGTHENS the disposition to WEAK-HOLD-with-1-slot-buffer** because the AMEND-A1 .. AMEND-A9 amendments resolve six SPEC-time-surface-risks at SPEC time rather than at IMPL time:
- wazero-vs-V8 divergence risk (BRAINSTORM-flagged escape-valve candidate) — CLOSED at AMEND-A4 with §4.5 guardrails. Zero IMPL-time scope-creep anticipated.
- proxy-wasm ABI v0.1.0/v0.2.0 PARSE-REJECT semantics — CLOSED at AMEND-A6 with the byte-faithful detection-point pin (`bytecode_util.cc:32-97` transcription target).
- WasmResult / WasmBufferType enum encoding — CLOSED at AMEND-A7 with the value-gap-preservation discipline.
- Stat-surface roster + prefix template — CLOSED at AMEND-A2 (5-counter REFINED roster + tri-group structural CONFIRMS).
- WasmPerRoute escape-valve to NEW 11th canonical — RETIRED at AMEND-A3 (5th-canonical REUSE-by-absence definitive).
- Foreign-function disposition — CLOSED at AMEND-A9 (option (b) registration interface + empty default registry + WasmResult.NotFound).

The 1-slot residual buffer at 25.1 IMPL covers (a) the wazero VM-pool design escape-valve (if 25.1 IMPL benchmarks of per-stream construction cost surface > 1ms unacceptable overhead, analogous to phase-22.1's ADR-0190 `*LState`-pool escape-valve which DID NOT FIRE per phase-22.1 §13-R6 disposition observed at 70µs) and (b) any wazero/V8 sub-pin edge case the §4.5 guardrails missed. ADR-0205 reserve slot covers either contingency. 25.2 + 25.3 IMPL ADR consumption is anchored at the respective sub-phase BRAINSTORMs.

**SPEC-time disposition:** STRENGTHENED-WEAK-HOLD with 1-slot-buffer (vs BRAINSTORM's 2-slot). 3 anticipated ADRs (ADR-0202 + ADR-0203 + ADR-0204) land cleanly; 0-1 escape-valve slot consumption from the wazero-VM-pool benchmark surface (the only remaining escape-valve candidate post-AMEND-A1..A9 closure).

---

## 2. Scope — non-purposes + REUSES-not-consumed

Phase 25 is 3-way-split per Q1+Q7 + §3 (no SPEC-time split re-decision per §3.0). It does NOT extend the framework or any other subsystem beyond the minimum needed to land `envoy.extensions.filters.http.wasm.v3.Wasm` envelope D under the existing 07.1 framework + the 1 NEW `internal/wasm/` framework primitive (ADR-0202) + the 1 NEW `internal/filter/http/wasm/` package (ADR-0203) + the 1 NEW default-deny capability ADR (ADR-0204) + the 5th-canonical REUSE-by-absence per-route classification (no ADR-0125 amendment per AMEND-A3).

- **2.1 `AsyncDataSource.Remote` arm OUT OF SCOPE + PARSE-REJECT** (per BRAINSTORM §1.2 + §8 item 1 + AMEND-A8 corollary). The hot-fetch-from-xDS/HTTP-source surface is deferred to a future Runtime / RTDS / hot-reload family phase. Mirrors phase-21 ADR-0187's `enabled.RuntimeFeatureFlag` PARSE-REJECT pattern + phase-22's WatchedDirectory PARSE-REJECT precedent. envoy-go-strict departure record (upstream supports remote fetch via the Group-A remote-load stat surface per AMEND-A2; envoy-go has no runtime layer).

- **2.2 `DataSource.watched_directory` sibling field OUT OF SCOPE + PARSE-REJECT** (per BRAINSTORM §2.1(d) + phase-22 lua precedent). Deferred to future Runtime/RTDS/hot-reload family phase.

- **2.3 Upstream-distinct runtime name discriminators OUT OF SCOPE + PARSE-REJECT.** `VmConfig.runtime ∈ {"envoy.wasm.runtime.v8", "envoy.wasm.runtime.wamr", "envoy.wasm.runtime.wasmtime", "envoy.wasm.runtime.null"}` triggers HCM-parse-time PARSE-REJECT. envoy-go accepts ONLY the empty string `""` (defaults to wazero) OR an explicit `"envoy.wasm.runtime.wazero"` (envoy-go-strict extension). Cross-side fixtures use the empty-string default to stay byte-exact with upstream's v8 default. envoy-go-strict departure record (upstream v1.37.2 supports 4 sandbox runtimes + the null VM, all marked `status: alpha` + `security_posture: unknown` per `source/extensions/extensions_metadata.yaml:1631-1635`).

- **2.4 proxy-wasm v0.1.0 + v0.2.0 ABI versions OUT OF SCOPE + PARSE-REJECT** (per AMEND-A6). envoy-go-strict targets v0.2.1 exclusively. Detection point: scan the wasm module's export section (section type 7) for a function-kind export named `proxy_abi_version_0_2_1`; PARSE-REJECT on any other sentinel value OR on absent sentinel. **envoy-go-strict-stricter departure** from upstream (which version-dispatches all 3 ABI versions); rationale: maintaining v0.1.0 + v0.2.0 dispatch doubles the in-house ABI surface for marginal operator value.

- **2.5 `WasmService` singleton plugin loaders OUT OF SCOPE.** `envoy.extensions.wasm.v3.WasmService` is a separate top-level config (NOT an HTTP filter). Lives in the broader §9 WASM host family beyond phase 25.

- **2.6 Cluster-specifier-wasm / access-logger-wasm / network-filter-wasm OUT OF SCOPE.** Separate filter / extension hosts; consume `internal/wasm/` at consumer #2+ scope. Each future phase BRAINSTORM revisits the API shape per ADR-0202's EXPLICIT API-REVISION ALLOWANCE clause. WASM host family phases.

- **2.7 wazero JIT/AOT compiler backend OUT OF SCOPE (opt-in deferred).** wazero ships an interpreter backend (default; pure-Go; supported on every Go GOOS/GOARCH) + an optional optimizing-compiler backend (amd64 + arm64; JIT-compiles wasm to native; opt-in via `wazero.NewRuntimeConfigCompiler()`). Phase 25 uses the interpreter default; the compiler opt-in is a future ops-tuning phase. NOT in scope at any sub-phase of phase 25.

- **2.8 wazero W^X security default + tail-call proposal OUT OF SCOPE.** v1.10.0-line features (W^X for compiled modules; tail-call proposal opt-in) are wazero-runtime-level — envoy-go uses the safe defaults; no operator-visible exposure at any sub-phase of phase 25.

- **2.9 Memory-trap fixture scenarios OUT OF SCOPE at 25.1 + 25.2** (per §4.5 D6 guardrails). Linear-memory limit violations + OOM probes are deferred — wazero traps with different error strings than V8, breaking byte-exact divergence. Memory-error scenarios may land at 25.3-or-later with a mixed-mode discipline.

- **2.10 HTTP/2 header iteration order fixture dependence OUT OF SCOPE at 25.1.** Fixture-0034 scenarios use HTTP/1.1 OR sort-on-assertion to avoid HPACK reorder divergence (per §4.5 D6 guardrails).

- **2.11 Body-access + trailer-access hostcalls OUT OF SCOPE at 25.1.** `proxy_get_buffer_bytes` + `proxy_set_buffer_bytes` for `HTTP_REQUEST_BODY` / `HTTP_RESPONSE_BODY` + `proxy_on_request_body` + `proxy_on_response_body` + `proxy_on_request_trailers` + `proxy_on_response_trailers` deferred to 25.2. 25.1 modules that import these hostcalls succeed at module-instantiation (stub-returns-Unimplemented per Option B in §4.2) but receive `WasmResult::Unimplemented` (=12) when invoked.

- **2.12 Timer dispatch hostcall OUT OF SCOPE at 25.1.** `proxy_set_tick_period_milliseconds` + `proxy_on_tick` callback deferred to 25.2.

- **2.13 Metric hostcalls OUT OF SCOPE at 25.1.** `proxy_define_metric` + `proxy_increment_metric` + `proxy_record_metric` + `proxy_get_metric` + the plugin-defined dynamic-stats namespace (`wasmcustom.<user-defined-name>` per `source/extensions/common/wasm/stats_handler.h:20`) deferred to 25.2.

- **2.14 Shared-data hostcalls OUT OF SCOPE at 25.1.** `proxy_get_shared_data` + `proxy_set_shared_data` (CAS-protected cross-stream + cross-plugin KV within the same `vm_id`) deferred to 25.2.

- **2.15 Shared-queue hostcalls + `proxy_on_queue_ready` callback OUT OF SCOPE at 25.1 + 25.2.** The 4 shared-queue hostcalls (`proxy_register_shared_queue` + `proxy_resolve_shared_queue` + `proxy_enqueue_shared_queue` + `proxy_dequeue_shared_queue`) require cross-VM (cross-`vm_id`) coordination — structurally similar to phase-22 lua's coro discipline but at WasmService scope. **Recommendation:** defer to WASM host family per BRAINSTORM §8 item 5. 25.2 BRAINSTORM/SPEC may re-evaluate.

- **2.16 Outbound HTTP dispatch OUT OF SCOPE at 25.1.** `proxy_http_call` + `proxy_on_http_call_response` deferred to 25.2; REUSES phase-20 `internal/httpclient/` framework primitive at first-or-later co-consumer (per ADR-0177; validates the phase-20 extraction per ADR-0177 §Consequences forward-pointer).

- **2.17 Outbound gRPC dispatch OUT OF SCOPE at all sub-phases of phase 25.** The 5 gRPC hostcalls (`proxy_grpc_call` + `proxy_grpc_stream` + `proxy_grpc_send` + `proxy_grpc_cancel` + `proxy_grpc_close`) + 4 gRPC callbacks (`proxy_on_grpc_receive_initial_metadata` + `proxy_on_grpc_receive` + `proxy_on_grpc_receive_trailing_metadata` + `proxy_on_grpc_close`) deferred to WASM host family. Rationale (per §11.8 D8 scrape): the gRPC surface is large (5 hostcalls + 4 callbacks) + intersects with envoy-go's gRPC client primitives (`internal/grpcclient/`) at multiple integration points; BRAINSTORM §1.1 sub-phase 25.2 does NOT explicitly enumerate gRPC hostcalls.

- **2.18 Foreign-function default registry OUT OF SCOPE at all sub-phases.** Per AMEND-A9 + ADR-0204: `internal/wasm/foreign.go` registration interface lands at 25.2 with an EMPTY default registry (zero foreign functions registered). The upstream 10 foreign functions (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`) are NOT ported at phase 25. envoy-go-strict departure record at 25.2 BEHAVIOR_CONTRACT.md.

- **2.19 Per-route `Wasm` 5th-canonical wholesale-override via TPFC OUT OF SCOPE at 25.1 + 25.2; CONSUMED at 25.3.** PARSE-REJECT at 25.1+25.2 per §6.2 arm 18 (via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint). 25.3 activates the wholesale-override resolver (REUSES existing TPFC mechanism per §3.3 REUSE 4). NO new canonical per AMEND-A3 + ADR-0210.

- **2.20 Multi-plugin VM-sharing OUT OF SCOPE at 25.1; CONSUMED at 25.3.** At 25.1 envoy-go accepts `vm_id` as a singleton VM key — duplicate `vm_id` across two `PluginConfig` entries PARSE-REJECTs per §6.2 arm 12. At 25.3 the multi-plugin path lands (multiple `PluginConfig` entries with the same `vm_id` share a single VM instance, instantiating distinct plugin contexts under it).

- **2.21 `VmConfig.environment_variables` OUT OF SCOPE at 25.1 + 25.2; CONSUMED at 25.3.** PARSE-REJECT at 25.1+25.2 per §6.2 arm 13. The WASI `environ_sizes_get` + `environ_get` shims return zeros at 25.1; 25.3 feeds them from the `EnvironmentVariables.host_env_keys` + `key_values` fields.

- **2.22 `PluginConfig.failure_policy = FAIL_RELOAD` + `ReloadConfig` OUT OF SCOPE at 25.1 + 25.2; CONSUMED at 25.3.** PARSE-REJECT `failure_policy = FAIL_RELOAD` + non-nil `reload_config` at 25.1+25.2 per §6.2 arm 9. 25.3 activates with the `wasm.<plugin_name>.{vm_reload,vm_reload_backoff,vm_reload_success,vm_reload_failure}` Group-C counter surface per AMEND-A2.

- **2.23 `PluginConfig.fail_open` deprecated field OUT OF SCOPE at 25.1 + 25.2; CONSUMED at 25.3.** Per AMEND-A2 + the AMEND-A1 v1.32.4 binding scrape (`CENTRAL_PB:476`): `fail_open` is the deprecated predecessor to `failure_policy = FAIL_OPEN`. 25.3 honors the deprecated field via the `failure_policy` ladder; 25.1 + 25.2 PARSE-REJECT it per §6.2 arm 10.

- **2.24 `VmConfig.allow_precompiled` + `VmConfig.nack_on_code_cache_miss` OUT OF SCOPE + PARSE-REJECT** (per AMEND-A1 binding scrape — both added to §6.2 PARSE-REJECT roster per §11.1 D1). `allow_precompiled` is V8/wasmtime AOT-only (incompatible with wazero's interpreter-default); `nack_on_code_cache_miss` is paired with `AsyncDataSource.Remote` (PARSE-REJECT'd per §2.1).

- **2.25 `SanitizationConfig` per-capability sanitization rules OUT OF SCOPE (accept-empty-as-no-op).** Per AMEND-A1 + §11.4: upstream's `SanitizationConfig` proto is EMPTY (no fields) and the upstream wasm host comment at `plugin.cc:14` marks the sanitization layer as "currently unimplemented and ignored, and so should be left empty". envoy-go matches upstream byte-faithfully — accept empty `SanitizationConfig{}` values; ignore non-empty (parse-and-discard mirrors phase-24's `override_option` INERT acceptance per AMEND-4).

- **2.26 v1.37.2 binding-gap forward-pointers NEVER-DEFERRED at 25.x.** Per AMEND-A1 + §11.1.5 D1: `PluginConfig.allow_on_headers_stop_iteration` (v1.37.2 field 9; `google.protobuf.BoolValue`) is ABSENT from envoy-go's consumed v1.32.4 binding (next-free-field = 9 vs upstream's 10). Forward-pointer binding-gap: the v1.32.4 protobuf-go parser silently drops the unknown field; no envoy-go-side PARSE-REJECT applies. Activates when go-control-plane bumps to v1.37.x.

- **2.27 NEVER-DEFERRED — Runtime feature-flag layer.** envoy-go has no runtime-features layer (phase-20 S2 settled). The `AsyncDataSource.Remote` PARSE-REJECT per §2.1 + the runtime-name discriminator PARSE-REJECT per §2.3 are consumed at their static defaults; PARSE-REJECT on any operator-visible attempt to use them.

- **2.28 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed (no TLS-principal interaction at 25.1+25.2; 25.3 may consume via `proxy_get_property` of `downstream.tls.peer_certificate`). ADR-0150 jwks NOT consumed. ADR-0151 jwt verifier NOT consumed. ADR-0177 `internal/httpclient/` NOT consumed at 25.1 (25.2 consumes for `proxy_http_call` per §2.16). ADR-0178 `internal/sdsfile/` NOT consumed. ADR-0158 `internal/grpcclient/` NOT consumed (gRPC hostcalls OUT OF SCOPE per §2.17). ADR-0165 + ADR-0174 `DecoderFilterCallbacks` + `EncoderFilterCallbacks` cross-phase-reusable extensions NOT consumed at 25.1 (25.2 may consume for the `proxy_get_property` full stream-info surface; 25.3 may consume for per-route plugin-context isolation). ADR-0186 `Clock` seam NOT consumed at 25.1 (25.2 consumes for `proxy_on_tick`). ADR-0188 `internal/lua/` NOT consumed (independent VM-class primitive). ADR-0190 `internal/dynamicmetadata/` may be RE-CONSUMED at 25.2 IF the `proxy_get_property` `metadata.*` path maps cleanly to the dynamicmetadata accessor (25.2 SPEC settles). ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` may be consumed at 25.1 IF the `proxy_get_status` hostcall consumes it (25.1 SPEC settles).

- **2.29 MVP confirmations (positive consumption assertions for 25.1).** `Wasm.config` IN MVP (sole top-level field per AMEND-A1 + §11.1.1). `PluginConfig.name` + `PluginConfig.root_id` + `PluginConfig.vm_config` + `PluginConfig.configuration` + `PluginConfig.capability_restriction_config` IN MVP. `VmConfig.vm_id` + `VmConfig.runtime` (default-only) + `VmConfig.code.local` (4-arm DataSource) + `VmConfig.configuration` IN MVP. 25.1 hostcall surface = 24 hostcalls (per §4.2 — 16 `proxy_*` + 7 `wasi_snapshot_preview1.*` + 1 noted-elsewhere — see §4.2 detail). 25.1 callback surface = 13 callbacks (per §4.2 — 5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks).

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q1; no SPEC-time re-decision per §1.4

Per BRAINSTORM Q1 + §1.4 the 3-way pre-split at BRAINSTORM time is the LOCKED disposition. The §11.8 D8 + §11.1 D1 empirical-pin scrapes produced no structural reason to revisit the split (no surface that would re-collapse to single-row; no scope-creep that would warrant a 4-way split). LoC envelope re-estimated post-empirical-scrape at **~7,680-11,960 LIVE production LoC across all 3 sub-phases** — substantially LARGER than the BRAINSTORM §1.4 estimate of "~5000-7000 LoC". Per-sub-phase task counts estimated at 17-19 (25.1) + 20-24 (25.2) + 9-12 (25.3) = **46-55 tasks total** — matches BRAINSTORM §1.4's 42-54 range. Each sub-phase fits cleanly under the ADR-0045 task-arm split-gate (~25 tasks); the LoC-arm gate fires at 25.1 + 25.2 but this is the expected pattern for NEW-framework-primitive sub-phases + advanced-hostcall-bundle sub-phases (phase-22.1 + phase-22.2 similarly exceeded the LoC-arm half of the gate at landing time; no further split was triggered).

The SPEC author's call: **3-way split LOCKED at BRAINSTORM Q1 STANDS at SPEC commit**. No ADR-0045 §6 re-application; no NEW ADR for the split disposition (BRAINSTORM Q1 + ADR-0106 ROADMAP row registrations already cover the discipline). Mirrors the parent-row precedent set at phase-18 SPEC §3 + phase-19 SPEC §3 + phase-22 SPEC §3 — the SPEC author confirms the BRAINSTORM split.

### 3.1 Split surface-mapping table (per phase-22 §3.1 precedent)

The mapping below uses **CONSUMED** (field/surface is fully implemented at this sub-phase), **PARSE-REJECT** (field/surface generates a config-load error with byte-stable wording at this sub-phase; lifts at the listed sub-phase), **stub-Unimplemented** (hostcall is registered but returns `WasmResult::Unimplemented`=12 at runtime per §4.2 Option B), **binding-gap** (field absent from envoy-go's consumed v1.32.4 binding per AMEND-A1; forward-pointer).

| Field / surface | 25.1 disposition | 25.2 disposition | 25.3 disposition |
|---|---|---|---|
| `Wasm.config` (top-level wrapper field; sole field per AMEND-A1) | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `PluginConfig.name` | **CONSUMED** (key for Group-C stat prefix per AMEND-A2) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `PluginConfig.root_id` | **CONSUMED** (plugin-context discriminator) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `PluginConfig.vm_config` (oneof `vm`) | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `PluginConfig.configuration` (passed to `proxy_on_configure`) | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `PluginConfig.capability_restriction_config.allowed_capabilities` | **CONSUMED** (default-deny posture per AMEND-A5 + ADR-0204; 25.1 capability roster covers headers-bridge hostcalls only) | CONSUMED (capability roster extends as 25.2 hostcalls land — body/buffer/timer/metric/shared-data/httpCall/foreign-function) | CONSUMED (capability roster extends as 25.3 surfaces land — per-route + multi-plugin) |
| `PluginConfig.capability_restriction_config.allowed_capabilities[*].sanitization_config` | **accept-empty-as-no-op** per AMEND-A1 §11.4 (empty proto; upstream-unimplemented) | accept-empty-as-no-op (unchanged) | accept-empty-as-no-op (unchanged) |
| `PluginConfig.fail_open` (deprecated bool) | **PARSE-REJECT** (deferred-to-25.3 per §2.23 + §6.2 arm 10) | PARSE-REJECT (unchanged) | **CONSUMED** (mapped onto `failure_policy = FAIL_OPEN` via the AMEND-A1 ladder) |
| `PluginConfig.failure_policy` (enum FAIL_RELOAD/FAIL_CLOSED/FAIL_OPEN/UNSPECIFIED) | **CONSUMED for UNSPECIFIED + FAIL_CLOSED** (default + closed); **PARSE-REJECT** for FAIL_RELOAD + FAIL_OPEN (deferred to 25.3 per §2.22 + §6.2 arm 9) | unchanged | **CONSUMED full ladder** |
| `PluginConfig.reload_config` (BackoffStrategy wrapper) | **PARSE-REJECT** if non-nil (deferred to 25.3) | PARSE-REJECT (unchanged) | **CONSUMED** (paired with `failure_policy = FAIL_RELOAD`) |
| `PluginConfig.allow_on_headers_stop_iteration` (v1.37.2 field 9) | binding-gap forward-pointer (absent from v1.32.4 binding per AMEND-A1) | binding-gap forward-pointer | binding-gap forward-pointer |
| `VmConfig.vm_id` | **CONSUMED as singleton** (duplicate-vm_id PARSE-REJECT per §6.2 arm 12) | CONSUMED as singleton (unchanged) | **CONSUMED with multi-plugin sharing** (duplicate `vm_id` admits with distinct plugin contexts) |
| `VmConfig.runtime` | `""` only (defaults wazero) OR `"envoy.wasm.runtime.wazero"` envoy-go-strict accept; **PARSE-REJECT** all other discriminator strings per §6.2 arm 11 | unchanged | unchanged |
| `VmConfig.code` (AsyncDataSource) `local` arm (4-sub-arm DataSource) | **CONSUMED** (Filename + InlineBytes + InlineString + EnvironmentVariable per §5.3) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `VmConfig.code` (AsyncDataSource) `remote` arm | **PARSE-REJECT** (RTDS/Runtime family deferral per §2.1 + §6.2 arm 6 + envoy-go-strict departure record) | PARSE-REJECT (unchanged) | PARSE-REJECT (unchanged) |
| `DataSource.watched_directory` sibling field | **PARSE-REJECT** (RTDS/hot-reload deferral per §2.2 + §6.2 arm 7) | PARSE-REJECT (unchanged) | PARSE-REJECT (unchanged) |
| `VmConfig.configuration` (passed to `proxy_on_vm_start`) | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `VmConfig.allow_precompiled` | **PARSE-REJECT** if true (V8/wasmtime AOT-only per §2.24 + §6.2 arm 14; envoy-go-strict departure record) | PARSE-REJECT (unchanged) | PARSE-REJECT (unchanged) |
| `VmConfig.nack_on_code_cache_miss` | **PARSE-REJECT** if true (paired with Remote arm per §2.24 + §6.2 arm 15) | PARSE-REJECT (unchanged) | PARSE-REJECT (unchanged) |
| `VmConfig.environment_variables` (`EnvironmentVariables` wrapper) | **PARSE-REJECT** if non-nil (deferred to 25.3 per §2.21 + §6.2 arm 13) | PARSE-REJECT (unchanged) | **CONSUMED** (feeds WASI `environ_*` shims with real data) |
| `EnvironmentVariables.host_env_keys` | (deferred — parent field PARSE-REJECT) | (deferred) | **CONSUMED** at 25.3 (operator-specified host env var pass-through) |
| `EnvironmentVariables.key_values` | (deferred — parent field PARSE-REJECT) | (deferred) | **CONSUMED** at 25.3 (operator-specified literal env values) |
| proxy-wasm ABI version sentinel export | **CONSUMED for `proxy_abi_version_0_2_1`**; PARSE-REJECT for `proxy_abi_version_0_1_0` + `proxy_abi_version_0_2_0` + absent sentinel per §2.4 + §6.2 arm 16 + AMEND-A6 envoy-go-strict departure | unchanged | unchanged |
| Headers-bridge hostcalls (`proxy_get_header_map_*` ×7 + `proxy_send_local_response` + `proxy_log` + `proxy_get_log_level` + `proxy_get/set_property` + `proxy_get_status` + `proxy_get_current_time_nanoseconds` + `proxy_set_effective_context` + `proxy_done`) | **CONSUMED** (16 `proxy_*` hostcalls per §4.2 + AMEND-A2 detail) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| WASI shim hostcalls (`fd_write` + `clock_time_get` + `random_get` + `environ_sizes_get` + `environ_get` + `args_sizes_get` + `args_get` + `proc_exit`) | **CONSUMED** (8 `wasi_snapshot_preview1.*` stubs per §4.2 — custom 7+1 implementation; do NOT use wazero's built-in WASI imports per AMEND-A4 + §11.5 D4 wazero-divergence guardrail) | CONSUMED (unchanged at hostcall level; `environ_*` returns zeros at 25.1+25.2 stubs) | CONSUMED (`environ_*` returns real values from `VmConfig.environment_variables`) |
| Body + buffer hostcalls (`proxy_get_buffer_bytes` + `proxy_set_buffer_bytes` + `proxy_get_buffer_status`) | stub-Unimplemented (registered but returns WasmResult::Unimplemented=12) | **CONSUMED** | CONSUMED (unchanged) |
| Stream-control hostcalls (`proxy_continue_stream` + `proxy_close_stream`) | stub-Unimplemented | **CONSUMED** | CONSUMED (unchanged) |
| Timer hostcall (`proxy_set_tick_period_milliseconds`) | stub-Unimplemented | **CONSUMED** | CONSUMED (unchanged) |
| Metric hostcalls (`proxy_define_metric` + `proxy_increment_metric` + `proxy_record_metric` + `proxy_get_metric`) + `wasmcustom.<name>` dynamic-stats namespace | stub-Unimplemented | **CONSUMED** | CONSUMED (unchanged) |
| Shared-data hostcalls (`proxy_get_shared_data` + `proxy_set_shared_data`) | stub-Unimplemented | **CONSUMED** | CONSUMED (unchanged) |
| Outbound HTTP dispatch hostcalls (`proxy_http_call` + `proxy_on_http_call_response`) | stub-Unimplemented | **CONSUMED** (RE-CONSUMES phase-20 `internal/httpclient/` per ADR-0177) | CONSUMED (unchanged) |
| Foreign-function hostcall (`proxy_call_foreign_function`) + `proxy_on_foreign_function` callback | stub-Unimplemented | **CONSUMED with empty default registry** per AMEND-A9 (returns WasmResult::NotFound=1; capability-roster default-deny per AMEND-A5) | CONSUMED (unchanged) |
| Full stream-info property surface (`proxy_get_property` of `upstream.*`, `downstream.tls.*`, `connection.*`, `filter_state.*`, etc.) | minimal property tree (`request.headers.*` + `response.headers.*` + `request.path` + `request.method` + `request.host` only) | **CONSUMED full surface** | CONSUMED (unchanged) |
| Shared-queue hostcalls (4) + `proxy_on_queue_ready` callback | stub-Unimplemented | (decision deferred to 25.2 BRAINSTORM — recommend DEFER per §2.15) | (recommend defer to WASM host family per §2.15) |
| Outbound gRPC hostcalls (5) + 4 gRPC callbacks | stub-Unimplemented | stub-Unimplemented | stub-Unimplemented (DEFER to WASM host family per §2.17) |
| TCP/network-filter hostcalls + callbacks (`proxy_on_new_connection`, `proxy_on_downstream_data`, etc.) | NOT REGISTERED (network-filter-wasm row out-of-scope per §2.6) | NOT REGISTERED | NOT REGISTERED |
| Per-route `Wasm` via TPFC (5th-canonical wholesale-override per AMEND-A3) | **PARSE-REJECT** (deferred-to-25.3 per §2.19 + §6.2 arm 18) | PARSE-REJECT (unchanged) | **CONSUMED** (5th-canonical REUSE-by-absence; NO ADR-0125 amendment; ADR-0210 EXPLICIT-NO-NEW-CANONICAL ADR) |
| Multi-plugin VM-sharing via `vm_id` (multiple PluginConfig with same `vm_id` share VM) | **PARSE-REJECT** (deferred-to-25.3 per §2.20 + §6.2 arm 12) | PARSE-REJECT (unchanged) | **CONSUMED** (ADR-0211 multi-plugin VM-sharing semantics + plugin-context isolation discipline) |
| `test/conformance/proxy-wasm/` harness seed | not present | not present | **SEEDED** (10/16 cpp-host test families ported; 62.5% pass-threshold; pinned to `proxy-wasm-cpp-host@da3ce05d` per AMEND-A8 + ADR-0212) |
| Stat surface | **5 counters** per AMEND-A2: `wasm.wazero.created` (Group B) + `wasm.wazero.active` gauge (Group B) + `wasm.<plugin_name>.executions` (envoy-go-strict) + `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict) + `wasm.<plugin_name>.envoy_go.failures` (envoy-go-strict). Project stat count 114 → 119 (+5) | likely +N envoy-go-strict counters (25.2 BRAINSTORM/SPEC settles — anticipated `tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied`); plus open-ended plugin-defined dynamic-stats namespace `wasmcustom.<custom_name>` (NOT counted in static total) | likely +Group-C 4 counters if `failure_policy = FAIL_RELOAD` lands (per AMEND-A2 + §2.22) |
| Differential fixture | **`0034-http-wasm-headers-bridge`** (full cross-side byte-exact for headers-bridge scenarios per AMEND-A4 with §4.5 guardrails) + **`0035-http-wasm-boot-reject`** (PGV-mirror + envoy-go-strict-stricter boot-rejects per §6.1 + §6.2) | `0036-http-wasm-body-and-advanced` (partial cross-side per ADR-0192 precedent) + `0037-http-wasm-body-and-advanced-boot-reject` | `0038-http-wasm-perroute-and-multi-plugin` (cross-side byte-exact for deterministic per-route + multi-plugin) + `0039-http-wasm-perroute-boot-reject` |
| Fuzzer | **`FuzzWasmConfigParse`** (34th project-wide fuzzer; 33 → 34 at master tip) | `FuzzWasmHostcallEnvelope` (35th) | `FuzzWasmPerRouteConfig` (36th) |
| HTTP filter wiring at `cmd/envoy-go/main.go` | **+1 entry** `wasm.New` alphabetically between `router` and any subsequent — 19 → 20 HTTP filters wired | unchanged at 20 | unchanged at 20 |
| ADR anchors | **ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences bodies land** | ADR-0205 .. ADR-0208 settled at 25.2 BRAINSTORM/SPEC (anticipated ~3-4 NEW ADRs for body+buffer + timer/metric/shared-data + httpCall + foreign-function-with-empty-registry-per-AMEND-A9) | ADR-0210 (EXPLICIT-NO-NEW-CANONICAL per-route per AMEND-A3) + ADR-0211 (multi-plugin VM-sharing) + ADR-0212 (conformance harness pin) + (no ADR-0125 amendment per AMEND-A3) |
| BEHAVIOR_CONTRACT.md edit bundle | **6-edit bundle at 25.1 IMPL final Task per ADR-0052** (§13) | extends `### envoy.filters.http.wasm` subsection with body-stage detail; stat-table +N; additional envoy-go-strict departure records if foreign-function-empty-registry lands per AMEND-A9 | extends with per-route + multi-plugin + conformance-harness detail; 5th-canonical REUSE-by-absence caption note (NO §(xvi) amendment per AMEND-A3) |

### 3.2 Per-sub-phase scope detail

The authoritative scope detail lives in each sub-phase's SPEC, authored at the sub-phase's own dedicated session per the phase-22.1 / 22.2 / 22.3 precedent:

- `docs/envoy-go/phases/25.1-http-filter-wasm-runtime-and-headers-bridge/SPEC.md` — drafted at the dedicated 25.1 SPEC session (next-skill `superpowers:writing-plans` scoped to 25.1 SPEC authoring per phase-22.1 precedent). Per-sub-phase BRAINSTORM may not be needed (the parent BRAINSTORM + this parent SPEC settled enough decisions); the 25.1 SPEC session may proceed directly to SPEC authoring. Anticipated artefacts: SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md across 3-4 sessions.
- `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-bridge/SPEC.md` — drafted at the dedicated 25.2 SPEC session (next-session-after-25.1-IMPL-phase-done). 25.2 BRAINSTORM expected (more design decisions surface than 25.1 — body+buffer + timer + metrics + shared-data + httpCall + foreign-function-default-registry-disposition per AMEND-A9 + dynamic-metadata-bridge disposition + full stream-info surface).
- `docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/SPEC.md` — drafted at the dedicated 25.3 SPEC session (next-session-after-25.2-IMPL-phase-done). 25.3 BRAINSTORM expected (per-route resolver design + multi-plugin VM-sharing semantics + conformance harness pass-threshold final verification + `environment_variables` activation + `failure_policy` ladder activation).

The 25.1 SPEC inherits this parent SPEC's §6 PARSE-REJECT roster + §7 stat surface + §8 fixture-0034+0035 disposition + §11 empirical-pin block + §12 D-questions + §13 RATIFIED-PENDING-IMPL items + §14 BEHAVIOR_CONTRACT.md edit bundle. The per-sub-phase SPECs reference back to this parent SPEC for the cross-cutting decisions and detail only the sub-phase-specific extensions.

---

## 4. Framework primitive — NEW `internal/wasm/` + NEW `internal/filter/http/wasm/` (per Q3 + Q4 EXTRACT-NOW + §11.5 D4)

Per BRAINSTORM Q3 + Q4 + §3.1 + §11.4 + §11.5 + AMEND-A4 + AMEND-A5. Phase 25.1 introduces ONE NEW `internal/wasm/` framework primitive at first-consumer (ENDS the phase-24 ZERO-NEW-package-level-primitive disposition; SECOND occurrence of EXTRACT-NOW-at-first-consumer after phase-22.1 `internal/lua/`) + ONE NEW `internal/filter/http/wasm/` package. Phase 25.3 anchors the EXPLICIT-NO-NEW-CANONICAL per-route classification (per AMEND-A3 — NO ADR-0125 amendment). No framework deltas at 25.2 beyond consuming the existing `internal/httpclient/` (phase-20 ADR-0177) at second co-consumer for `proxy_http_call` + the foreign-function registration interface lands inside `internal/wasm/foreign.go` per AMEND-A9.

### 4.1 NEW `internal/wasm/` framework primitive (ADR-0202; lands at 25.1 IMPL)

Package boundary: `internal/wasm/` hosts the GENERIC wazero VM lifecycle (init, sandbox config, panic-recovery, per-stream execution context, module-compilation cache, ABI-registration interface, the in-house proxy-wasm v0.2.1 host ABI implementation for the 47-hostcall surface, the 30-callback guest-export interface) + the in-house ABI types (`WasmResult` enum with value-gap-preserving encoding per AMEND-A7, `WasmBufferType`, `WasmHeaderMapType`, `LogLevel`, `StreamType`, `MetricType`, `ProxyAction`, `WasiErrno`); `internal/filter/http/wasm/` hosts the HTTP-FILTER-SPECIFIC config parse + filter callbacks + per-stream HTTP-context shape. The ABI-registration interface is the seam between the two: consumers register their Go callbacks as wasm-host-side functions via the primitive's API; the primitive doesn't know about HTTP-specific concepts.

The primitive's API shape (provisional; settled at 25.1 SPEC; this parent SPEC anchors the shape per the phase-22.1 ADR-0188 precedent):

```go
// VM is a per-stream wazero execution context. NOT goroutine-safe.
// Each per-stream filter dispatch constructs a fresh VM via NewVM;
// OnDestroy releases via Close.
type VM struct { /* unexported; wazero Runtime + Module + sandbox config + capability registry */ }

// VMOption configures VM construction. Function-option pattern (matches
// internal/lua/'s phase-22.1 precedent).
type VMOption func(*VM)

// WithSandboxConfig sets the per-capability ALLOW/DENY posture.
// Zero value = StrictDefaultDeny (DENY all 80 capabilities per AMEND-A5).
func WithSandboxConfig(sb SandboxConfig) VMOption

// WithPanicHandler registers a panic-recovery callback invoked after recover().
// Same shape as internal/lua/'s WithPanicHandler.
func WithPanicHandler(fn PanicHandlerFn) VMOption

// WithLogSink redirects proxy_log output. nil = drop (no stdout leak).
func WithLogSink(w io.Writer) VMOption

func NewVM(opts ...VMOption) *VM

// Module wraps a compiled wazero CompiledModule safe for cross-VM reuse.
// Holds the wasm bytecode + the detected ABI version.
type Module struct { /* unexported */ }

// CompileCache is a per-compiledConfig content-addressed compile cache,
// keyed by sha256(wasm bytes). Cache is owned by *compiledConfig
// (filter-config-instance scope; GC-driven eviction).
type CompileCache struct { /* unexported */ }

func NewCompileCache() *CompileCache
func CompileModule(ctx context.Context, src []byte, cache *CompileCache) (*Module, error)

// ABI-registration interface — consumer registers per-context callbacks via
// the primitive's API; the primitive doesn't know about HTTP-specific concepts.
type ABICallbacks interface {
    OnContextCreate(contextID, parentContextID uint32)
    OnVMStart(rootContextID uint32, vmConfigurationSize uint32) bool
    OnConfigure(pluginContextID uint32, pluginConfigurationSize uint32) bool
    OnDone(contextID uint32) bool
    OnDelete(contextID uint32)
    OnLog(contextID uint32)
    OnRequestHeaders(streamContextID uint32, numHeaders uint32, endOfStream bool) ProxyAction
    OnResponseHeaders(streamContextID uint32, numHeaders uint32, endOfStream bool) ProxyAction
    // ... extends at 25.2 + 25.3 ...
}

// VM.RegisterABICallbacks registers the consumer's callback bundle.
// Lands at 25.1 with the headers-bridge subset; extends at 25.2 + 25.3.
func (vm *VM) RegisterABICallbacks(cb ABICallbacks)

// Run loads the module's CompiledModule onto this VM's wazero Runtime
// (cheap; no re-compilation per stream) and calls _initialize + proxy_on_vm_start.
func (vm *VM) Run(ctx context.Context, module *Module, pluginContextID uint32) error

// CallProxyOnRequestHeaders invokes the guest's proxy_on_request_headers callback.
// (One per guest-callable function in the 30-callback roster; the function
//  names are mechanical — matches the proxy-wasm v0.2.1 spec verbatim.)
func (vm *VM) CallProxyOnRequestHeaders(ctx context.Context, streamContextID, numHeaders uint32, endOfStream bool) (ProxyAction, error)
// ... 13-callback subset at 25.1 per §4.2 ...

// ForeignFunctionRegistry (lands at 25.2 per AMEND-A9; AT 25.1 the type exists but
// no registrations occur — empty default registry returns NotFound).
type ForeignFunctionRegistry struct { /* unexported; sync.RWMutex + map */ }

func NewForeignFunctionRegistry() *ForeignFunctionRegistry
func (r *ForeignFunctionRegistry) Register(name string, fn ForeignFunction) error
func (r *ForeignFunctionRegistry) Get(name string) (ForeignFunction, bool)

// Close releases the VM's wazero Runtime. Idempotent.
func (vm *VM) Close()
```

**EXPLICIT API-REVISION ALLOWANCE clause** (anchored at ADR-0202 §Decision body at 25.1 IMPL Lands-in-Task): the primitive's API shape is provisional at consumer #1 (HTTP filter Wasm); the second consumer (cluster specifier Wasm at `envoy.router.cluster_specifiers.wasm`, access logger Wasm at `envoy.access_loggers.wasm`, network filter Wasm at `envoy.filters.network.wasm`, WasmService singleton plugin loaders — whichever materializes first in the broader §9 WASM host family) may require API revision after empirical validation. ADR-0202's §Decision body carries an EXPLICIT API-REVISION ALLOWANCE clause referencing the future-consumer phases. The future-consumer roster is **structurally MORE committed than phase-22.1 lua's speculative future-consumer roster** because the broader §9 WASM host family is explicitly listed at `BOOTSTRAP_PROMPT.md §9` line 116 with 4 anticipated consumers — the API revision risk at consumer #2 is reduced by the explicit family-level roadmap commitment.

### 4.2 Per-stream wazero VM construction + per-module compile cache discipline + 25.1 hostcall surface (per §11.5 D4 + AMEND-A1 + AMEND-A7)

Per §11.5 D4 empirical evaluation against `proxy-wasm-cpp-host@da3ce05d` + wazero's `wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())` API. **25.1 WEAK-default: per-stream wazero Runtime construction with shared per-module `*wazero.CompiledModule` cache.** Each per-stream invocation:

1. The filter's `*compiledConfig` carries the pre-compiled `*Module` (compiled once at config-load via `CompileModule(ctx, src, cfg.compileCache)`).
2. At `DecodeHeaders` entry, the filter calls `vm := wasm.NewVM(opts...)`, constructing a fresh `*wazero.Runtime` with the compiler backend + the sandbox capability roster applied (default-deny per AMEND-A5; the configured `capability_restriction_config.allowed_capabilities` map widens the roster).
3. The filter registers the 47-hostcall env-namespace + the 8-hostcall `wasi_snapshot_preview1`-namespace host modules onto the wazero Runtime via `RegisterABICallbacks(cb)` (Option B per §11.5 D4 — register-all-stub-deferred; deferred hostcalls return `WasmResult::Unimplemented`=12).
4. The filter calls `vm.Run(ctx, cfg.module, rootContextID)` which: (a) instantiates the `*wazero.Module` onto the new `*wazero.Runtime` (cheap — modules are pre-compiled bytecode); (b) calls `_initialize` (the WASI-style module init); (c) calls `proxy_on_vm_start(rootContextID, vmConfigurationSize)` so the guest module can read `VmConfig.configuration` via `proxy_get_buffer_bytes(VM_CONFIGURATION, ...)`; (d) calls `proxy_on_configure(pluginContextID, pluginConfigurationSize)` so the guest can read `PluginConfig.configuration`.
5. The filter calls `vm.CallProxyOnRequestHeaders(...)` per the request lifecycle; the guest may invoke any of the 24 registered hostcalls (16 `proxy_*` + 8 `wasi_*`); deferred hostcalls stub-return Unimplemented.
6. At `OnDestroy` (or end-of-stream), the filter calls `vm.Close()` which calls `*wazero.Runtime.Close()` and releases the linear memory + the compiled-module reference.

**25.1 host-side hostcall registration: 24 hostcalls (16 `proxy_*` + 8 `wasi_*`).** Per AMEND-A4 + §11.5 D4 + the §11.7 D9 spec README. The 16 `proxy_*` env-namespace hostcalls at 25.1:

1. `proxy_log(level, msg_ptr, msg_size)` — header-bridge log emission (anchors `wasm.<plugin_name>.executions` counter increment per AMEND-A2).
2. `proxy_get_log_level(result_level_uint32_ptr)` — v0.2.1-new; v0.1.0/v0.2.0 do NOT export. Detection signal for v0.2.1-only modules.
3. `proxy_send_local_response(status_code, status_msg_ptr, status_msg_size, body_ptr, body_size, additional_headers_ptr, additional_headers_size, grpc_status)` — 8-argument hostcall (largest in v0.2.1 surface); reuses `SendLocalReply` per §3.3 REUSE 5.
4. `proxy_get_header_map_pairs(type, ptr_ptr, size_ptr)` — serialized pairs format per AMEND-A4 (reimplemented byte-faithfully from `pairs_util.h`/`.cc`).
5. `proxy_set_header_map_pairs(type, ptr, size)` — same wire format.
6. `proxy_get_header_map_value(type, key_ptr, key_size, value_ptr_ptr, value_size_ptr)`.
7. `proxy_add_header_map_value(type, key_ptr, key_size, value_ptr, value_size)`.
8. `proxy_replace_header_map_value(type, key_ptr, key_size, value_ptr, value_size)`.
9. `proxy_remove_header_map_value(type, key_ptr, key_size)`.
10. `proxy_get_header_map_size(type, result_ptr)`.
11. `proxy_get_property(path_ptr, path_size, value_ptr_ptr, value_size_ptr)` — minimal property tree (`request.headers.*`, `response.headers.*`, `request.path`, `request.method`, `request.host`); full CEL stream-info surface deferred to 25.2.
12. `proxy_set_property(key_ptr, key_size, value_ptr, value_size)`.
13. `proxy_get_status(code_ptr, value_ptr_ptr, value_size_ptr)` — HTTP/gRPC status read; consumed inside `proxy_on_response_headers`.
14. `proxy_set_effective_context(context_id)` — context-switch hostcall (used by timer + httpCall callbacks; available at 25.1 for completeness).
15. `proxy_done()` — guest-signals-end-of-context hostcall.
16. `proxy_get_current_time_nanoseconds(result_uint64_ptr)` — DEPRECATED in v0.2.1 (per the spec README; guests are encouraged to use `wasi_snapshot_preview1.clock_time_get`) but trivially implementable; available at 25.1 for upstream byte-faithfulness.

The 8 `wasi_snapshot_preview1.*` namespace shims at 25.1 (custom 8-stub implementation per AMEND-A4 D6 guardrail — do NOT use wazero's built-in `imports/wasi_snapshot_preview1` package, which routes `fd_write` to host stdout/stderr rather than through `proxy_log`):

17. `fd_write(fd, iovec, iovec_size, nwritten_ptr)` — fd=1 (stdout) → routes to `proxy_log(LogLevel::INFO, ...)`; fd=2 (stderr) → routes to `proxy_log(LogLevel::ERROR, ...)`; other fds → returns `WasiErrno::BADF` (=8).
18. `clock_time_get(clock_id, precision, time_ptr)` — wall clock (`CLOCK_REALTIME` = 0) + monotonic clock (`CLOCK_MONOTONIC` = 1); 25.1 honors both at host-time accuracy (no fake-time seam at 25.1).
19. `random_get(buffer, buffer_size)` — fills buffer with `crypto/rand.Read(...)` bytes.
20. `environ_sizes_get(num_elements_ptr, buffer_size_ptr)` — at 25.1 returns 0/0 (no env vars exposed); at 25.3 returns real values from `VmConfig.environment_variables`.
21. `environ_get(argv_ptr, buffer_ptr)` — paired with environ_sizes_get; at 25.1 writes nothing.
22. `args_sizes_get(argc_ptr, argv_buf_size_ptr)` — always returns 0/0 (no command-line args ever passed to a wasm plugin).
23. `args_get(argv_ptr, argv_buf_ptr)` — paired with args_sizes_get; writes nothing.
24. `proc_exit(exit_code)` — MUST NOT be called by well-behaved guests; if invoked, terminates the VM via wazero trap.

**25.1 guest-callback surface: 13 callbacks (per §11.5 D4 + AMEND-A4).** The host invokes these on the guest via `wazero.Module.ExportedFunction("proxy_on_X")` lookups + `.Call(ctx, args...)`:

C1. `_initialize` — required module init; called once at `vm.Run` entry. C2. `_start` — alt module init (mutually exclusive with `_initialize`); guest exports one or the other. C3. `main` — called after `_initialize` if exported; discard return value. C4. `malloc(size_t)` — legacy guest allocator (v0.1.0-era); required because some guest stdlib paths call malloc internally. C5. `proxy_on_memory_allocate(size_t)` — v0.2.0+ preferred guest allocator; host uses this for buffer-write hostcall return paths. C6. `proxy_on_context_create(context_id, parent_context_id)` — host signals new context (root context: parent=0; stream context: parent=root_context_id). C7. `proxy_on_vm_start(root_context_id, vm_configuration_size)` — host signals VM started; guest reads VmConfig.configuration via `proxy_get_buffer_bytes`. C8. `proxy_on_configure(plugin_context_id, plugin_configuration_size)` — host signals plugin config; guest reads PluginConfig.configuration. C9. `proxy_on_done(context_id) → bool` — host signals end-of-context; returning false defers finalize. C10. `proxy_on_log(context_id)` — final access-log-time callback before stream context destroyed. C11. `proxy_on_delete(context_id)` — final context destruction signal. C12. `proxy_on_request_headers(stream_context_id, num_headers, end_of_stream) → ProxyAction` — returns CONTINUE (=0) or PAUSE (=1). C13. `proxy_on_response_headers(stream_context_id, num_headers, end_of_stream) → ProxyAction`.

### 4.3 Default-deny capability roster (per AMEND-A5 + ADR-0204 + §11.4 D7)

Per §11.4 D7 empirical scrape against `proxy-wasm-cpp-host:include/proxy-wasm/wasm.h:103-106` (`capabilityAllowed` gate function) + AMEND-A5 envoy-go-strict departure. The 25.1 `SandboxConfig` zero-value posture is **StrictDefaultDeny** — empty `allowed_capabilities` map ⇒ DENY ALL hostcalls. Operators MUST explicitly enable each capability via `PluginConfig.capability_restriction_config.allowed_capabilities[<capability_name>] = SanitizationConfig{}`.

#### 4.3.1 Capability key format

Per §11.4 D7: the gate map key is the bare hostcall name string. For `proxy_*` env-namespace hostcalls, the key is `proxy_<base>` (e.g. `proxy_get_header_map_pairs`). For `wasi_snapshot_preview1.*` hostcalls, the key is the bare WASI function name with NO `proxy_` prefix (e.g. `fd_write`). For guest-exported callbacks the host invokes (e.g. `proxy_on_request_headers`), the key is `proxy_<base>` — these participate in the same capability gate at the `getFunction` lookup side per `proxy-wasm-cpp-host:wasm.h:274-282`.

#### 4.3.2 Denial semantic

When a hostcall is denied (`allowed_capabilities` non-empty AND key absent), envoy-go returns `WasmResult::InternalFailure` (=10) + emits an integration error log (matches upstream `proxy_wasm_exports.h:217-226` byte-faithfully) + increments `wasm.<plugin_name>.hostcall_denied` (envoy-go-strict counter per AMEND-A2). WASI denials use a separate stub returning `WasiErrno::NOTSUP` (=58, NOT upstream's `ENOTCAPABLE` = 76 — TBD-25.1-IMPL: settle the WASI denial errno against the empirical scrape at IMPL first-task).

#### 4.3.3 Default-deny baseline at 25.1 (empty `allowed_capabilities`)

At 25.1 IMPL with the StrictDefaultDeny zero-value `SandboxConfig` posture, ALL 24 registered hostcalls are gated. The capability roster grows organically as 25.2 + 25.3 hostcalls register.

#### 4.3.4 Operator-opt-in 25.1 capability roster (when `allowed_capabilities` is non-empty)

Operators may explicitly enable hostcalls from the 25.1 surface by adding keys to `allowed_capabilities`:
- Headers-bridge family (7 keys): `proxy_get_header_map_pairs`, `proxy_set_header_map_pairs`, `proxy_get_header_map_value`, `proxy_add_header_map_value`, `proxy_replace_header_map_value`, `proxy_remove_header_map_value`, `proxy_get_header_map_size`.
- Local-response (1 key): `proxy_send_local_response`.
- Property (2 keys): `proxy_get_property`, `proxy_set_property`.
- Log (2 keys): `proxy_log`, `proxy_get_log_level`.
- Status (1 key): `proxy_get_status`.
- Time (1 key): `proxy_get_current_time_nanoseconds`.
- Context-lifecycle (2 keys): `proxy_set_effective_context`, `proxy_done`.
- WASI (8 keys; bare names): `fd_write`, `clock_time_get`, `random_get`, `environ_sizes_get`, `environ_get`, `args_sizes_get`, `args_get`, `proc_exit`.

Plus the 13 module-function-getter capability keys (gated at `getFunction` time per `proxy-wasm-cpp-host:wasm.h:274-282`): `proxy_on_context_create`, `proxy_on_vm_start`, `proxy_on_configure`, `proxy_on_done`, `proxy_on_delete`, `proxy_on_log`, `proxy_on_request_headers`, `proxy_on_response_headers`, plus the 5 module-init (`_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate`) — these are required-for-instantiation and TBD-25.1-IMPL: settle whether they participate in capability gating or are unconditionally ungated at the runtime entry-point.

The 25.2 + 25.3 capability roster extensions land at the respective sub-phase SPECs.

#### 4.3.5 SanitizationConfig disposition

Per AMEND-A1 + §11.4: upstream's `SanitizationConfig` proto is EMPTY (no fields) + upstream marks it "currently unimplemented and ignored, and so should be left empty" (`source/extensions/common/wasm/plugin.cc:14`). envoy-go matches upstream byte-faithfully — accept empty `SanitizationConfig{}` values; ignore non-empty (parse-and-discard mirrors phase-24's `override_option` INERT acceptance per AMEND-4). NO PARSE-REJECT on non-empty SanitizationConfig; NO IMPL surface beyond the accept-empty discipline.

### 4.4 NEW `internal/filter/http/wasm/` package (ADR-0203; lands at 25.1 IMPL)

Package boundary: `internal/filter/http/wasm/` hosts the HTTP-FILTER-SPECIFIC parse + filter callbacks + per-stream HTTP-context shape. Per §11.1 D1 + §11.4 D7 + the phase-22.1 ADR-0189 precedent. The package files (provisional; settled at 25.1 SPEC):

```
internal/filter/http/wasm/
  doc.go               # package overview + Q1-Q9 BRAINSTORM decision summary + AMEND-A1..A9 cross-references
  wasm.go              # filter struct + factory (HTTPFilterFactory) + filterStats
  compiled_config.go   # config parse + 4-arm AsyncDataSource resolution + 18-arm PARSE-REJECT roster + module-compile cache key generation
  datasource.go        # AsyncDataSource.Local arm resolution (4-sub-arm DataSource: Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory + Remote PARSE-REJECT)
  abi_callbacks.go     # implements wasm.ABICallbacks for the HTTP-filter context (headers-bridge methods + lifecycle hooks)
  decode_headers.go    # proxy_on_request_headers hook firing + ProxyAction handling
  encode_headers.go    # proxy_on_response_headers hook firing + ProxyAction handling
  stats.go             # 5-counter stat surface per AMEND-A2 (Group B `created` + `active` gauge + 3 envoy-go-strict counters)
  wasm_test.go         # unit tests (~1500-2000 LoC anticipated)
  compiled_config_test.go  # 18-arm PARSE-REJECT roster table-driven tests
  datasource_test.go   # AsyncDataSource resolution unit tests
  abi_callbacks_test.go # HTTP-filter ABI callback unit tests
  fuzz_test.go         # 34th project-wide fuzzer FuzzWasmConfigParse
```

Boot-registration insertion at `cmd/envoy-go/main.go`: alphabetical between any subsequent filters — per ADR-0100 §2.2 stylistic discipline. 19 HTTP filters wired pre-phase-25.1 (per master tip); **20 post-phase-25.1**. The Go-package identifier is `wasm` (single token; matches `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac`/`lua` precedent — no underscore needed).

### 4.5 D6 guardrail bit (per AMEND-A4 + §11.6)

Per AMEND-A4 + §11.6 D6 + the wazero-vs-V8 byte-exact CONFIRMS verdict. Phase 25.1 fixture-0034 authors MUST NOT include scenarios that:

(a) **Exceed wazero's default linear-memory limit** — wazero traps with the Go error string `"out of bounds memory access"`; V8 traps with a different string. Memory-OOM probes are DEFERRED to 25.2 (where the body-buffer surface is fully landed). 25.1 fixtures avoid memory-trap scenarios entirely.

(b) **Depend on HTTP/2 header iteration order** — HPACK may reorder headers between wazero (consumed via Go's `net/http` headers) and V8 (consumed via Envoy's HeaderMap). Use HTTP/1.1 fixtures OR sort headers in the cross-side assertion. The fixture-0034 scenarios use HTTP/1.1 by default per the phase-22 lua precedent.

(c) **Emit log lines containing float-formatted numbers** — wazero and V8 may diverge on `tostring(float)` / `string.format("%f")` byte output (analogous to gopher-lua-vs-LuaJIT divergence per phase-22 AMEND-9). NOT a 25.1 concern (no floats on the headers-bridge wire); flagged for 25.2 fixture scenarios.

(d) **Call any hostcall outside the 24-hostcall headers-bridge subset listed in §4.2.** A scenario that invokes `proxy_get_buffer_bytes` (deferred 25.2) at 25.1 fires the stub-returns-Unimplemented path — the cross-side assertion would diverge on `WasmResult` propagation. 25.1 fixture-0034 scenarios use only the 24-hostcall surface.

Any scenario violating (a)-(d) is either BOOT-REJECTED at the fixture-author lint (the 25.1 IMPL fixture authoring task includes a scenario-roster review pass) OR DEFERRED to 25.2/25.3.

---

## 5. Proto-field roster (per §11.1 D1)

Per §11.1 D1 empirical scrape against `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/http/wasm/v3/wasm.pb.go` + `.../extensions/wasm/v3/wasm.pb.go` + `.../config/core/v3/base.pb.go` (the AsyncDataSource + DataSource + RemoteDataSource + WatchedDirectory envelope) + upstream v1.37.2 IDL via WebFetch.

### 5.1 `Wasm` HTTP-filter wrapper message field roster (1 field)

The `envoy.extensions.filters.http.wasm.v3.Wasm` wrapper has EXACTLY ONE field:

| # | Field name | Proto type | Go type | PGV | Dep? | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `config` | `envoy.extensions.wasm.v3.PluginConfig` | `*v3.PluginConfig` | embedded-message-validate dispatch only (`FILTER_VAL:79-87`) — no `required` constraint | no | 25.1 CONSUMED |

**No other fields.** Per `FILTER_PB:153-156` the wrapper declares `NumMessages: 1`. The GoTypes vector at `FILTER_PB:117-118` lists exactly `(*Wasm)(nil)` + the imported `(*v3.PluginConfig)(nil)`. **No `stat_prefix` field on the wrapper** — the stat namespace anchors at `VmConfig.runtime` + `PluginConfig.name` per AMEND-A2.

### 5.2 `PluginConfig` message field roster (8 fields per v1.32.4 binding)

`envoy.extensions.wasm.v3.PluginConfig` — declared `[#next-free-field: 9]` per `CENTRAL_PB:442` (matches 8 used fields; v1.37.2 IDL is at `[#next-free-field: 10]` per AMEND-A1 binding-gap forward-pointer for `allow_on_headers_stop_iteration`).

| # | Field name | Proto type | Go type | PGV | Default | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `name` | string | string | `no validation rules for Name` (`CENTRAL_VAL:738`) | `""` | 25.1 CONSUMED |
| 2 | `root_id` | string | string | `no validation rules for RootId` (`CENTRAL_VAL:740`) | `""` | 25.1 CONSUMED |
| 3 | `vm_config` (in `oneof vm`) | `VmConfig` | `*VmConfig` via `PluginConfig_VmConfig` (`CENTRAL_PB:585-589`) | embedded validate + oneof typed-nil reject (`CENTRAL_VAL:833-877`) | nil | 25.1 CONSUMED |
| 4 | `configuration` | `google.protobuf.Any` | `*anypb.Any` | embedded validate dispatch (`CENTRAL_VAL:742-769`) | nil | 25.1 CONSUMED |
| 5 | `fail_open` (deprecated bool) | bool | bool | `no validation rules for FailOpen` (`CENTRAL_VAL:771`) | false | 25.1+25.2 PARSE-REJECT (§6.2 arm 10); 25.3 CONSUMED |
| 6 | `capability_restriction_config` | `CapabilityRestrictionConfig` | `*CapabilityRestrictionConfig` | embedded validate dispatch (`CENTRAL_VAL:804-831`) | nil | 25.1 CONSUMED (default-deny posture per AMEND-A5) |
| 7 | `failure_policy` (enum) | `FailurePolicy` | `FailurePolicy` int32 | `no validation rules for FailurePolicy` (`CENTRAL_VAL:773`) | `UNSPECIFIED` (=0) | 25.1+25.2 PARSE-REJECT for `FAIL_RELOAD`+`FAIL_OPEN` (§6.2 arm 9); 25.3 CONSUMED full ladder |
| 8 | `reload_config` | `ReloadConfig` | `*ReloadConfig` | embedded validate dispatch (`CENTRAL_VAL:775-802`) | nil | 25.1+25.2 PARSE-REJECT if non-nil (§6.2 arm 9); 25.3 CONSUMED |

**FailurePolicy enum values** (`CENTRAL_PB:32-44`): `UNSPECIFIED = 0` (default; falls back to `FAIL_CLOSED`); `FAIL_RELOAD = 1`; `FAIL_CLOSED = 2`; `FAIL_OPEN = 3`.

**ReloadConfig** (`CENTRAL_PB:90-137`): single field `backoff` (`envoy.config.core.v3.BackoffStrategy`, field 1). Only consumed when `failure_policy = FAIL_RELOAD`.

**v1.37.2 binding-gap (AMEND-A1):** `PluginConfig.allow_on_headers_stop_iteration` (field 9; `google.protobuf.BoolValue`) is ABSENT from v1.32.4 binding. Forward-pointer note in §10. NOT a PARSE-REJECT (v1.32.4 protobuf-go parser silently drops unknown fields).

### 5.3 `VmConfig` message field roster (7 fields per v1.32.4 binding)

`envoy.extensions.wasm.v3.VmConfig` — declared `[#next-free-field: 8]` per `CENTRAL_PB:240` (7 used fields; matches v1.37.2 IDL exactly — no binding-gap).

| # | Field name | Proto type | Go type | PGV | Default | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `vm_id` | string | string | `no validation rules for VmId` (`CENTRAL_VAL:440`) | `""` | 25.1 CONSUMED as singleton (duplicate-vm_id PARSE-REJECT per §6.2 arm 12); 25.3 multi-plugin sharing |
| 2 | `runtime` | string | string | `no validation rules for Runtime` (`CENTRAL_VAL:442`) | `""` (defaults wazero) | 25.1 CONSUMED `""` only; PARSE-REJECT discriminators per §6.2 arm 11 |
| 3 | `code` | `envoy.config.core.v3.AsyncDataSource` | `*v3.AsyncDataSource` | embedded validate dispatch (`CENTRAL_VAL:444-471`) | nil (PGV requires `specifier` oneof per §5.4) | 25.1 CONSUMED (Local arm 4-sub-arm); Remote PARSE-REJECT per §6.2 arm 6 |
| 4 | `configuration` | `google.protobuf.Any` | `*anypb.Any` | embedded validate dispatch (`CENTRAL_VAL:473-500`) | nil | 25.1 CONSUMED (on_vm_start init payload) |
| 5 | `allow_precompiled` | bool | bool | `no validation rules for AllowPrecompiled` (`CENTRAL_VAL:502`) | false | PARSE-REJECT if true per §6.2 arm 14 (V8/wasmtime AOT-only; incompatible with wazero) |
| 6 | `nack_on_code_cache_miss` | bool | bool | `no validation rules for NackOnCodeCacheMiss` (`CENTRAL_VAL:504`) | false | PARSE-REJECT if true per §6.2 arm 15 (paired with Remote arm) |
| 7 | `environment_variables` | `EnvironmentVariables` | `*EnvironmentVariables` | embedded validate dispatch (`CENTRAL_VAL:506-533`) | nil | 25.1+25.2 PARSE-REJECT if non-nil per §6.2 arm 13; 25.3 CONSUMED |

### 5.4 `AsyncDataSource` + `DataSource` + `RemoteDataSource` + `WatchedDirectory` envelope

`envoy.config.core.v3.AsyncDataSource` (`CORE_PB:1999-2079`) — 2-arm oneof `specifier`:
- `local` (oneof field 1; type `DataSource`) — 25.1 CONSUMED
- `remote` (oneof field 2; type `RemoteDataSource`) — 25.1 PARSE-REJECT per §6.2 arm 6

**PGV:** `(validate.required) = true` on the `specifier` oneof per `CORE_VAL:3497-3506` — an empty `AsyncDataSource{}` fails PGV with `"value is required"`.

`envoy.config.core.v3.DataSource` (`CORE_PB:1693-1831`) — 4-arm oneof `specifier` + 1 sibling field:

| Arm | Proto type | PGV | Sub-phase mapping |
|---|---|---|---|
| `filename` (oneof field 1) | string | `value length must be at least 1 runes` (`CORE_VAL:2830-2839`) | 25.1 CONSUMED |
| `inline_bytes` (oneof field 2) | bytes | `no validation rules` (`CORE_VAL:2853`) | 25.1 CONSUMED |
| `inline_string` (oneof field 3) | string | `no validation rules` (`CORE_VAL:2866`) | 25.1 CONSUMED |
| `environment_variable` (oneof field 4) | string | `value length must be at least 1 runes` (`CORE_VAL:2880-2889`) | 25.1 CONSUMED |
| `watched_directory` (sibling field 5; NOT in oneof) | `WatchedDirectory` | embedded validate dispatch (`CORE_VAL:2786-2813`) | 25.1 PARSE-REJECT per §6.2 arm 7 |

**PGV on DataSource:** `(validate.required) = true` on the `specifier` oneof per `CORE_VAL:2894-2903`. An empty `DataSource{}` fails PGV with `"value is required"`.

`envoy.config.core.v3.RemoteDataSource` (`CORE_PB:1932-1996`) — 3 fields; entire arm PARSE-REJECT at envoy-go phase 25.

`envoy.config.core.v3.WatchedDirectory` (`CORE_PB:1645-1691`) — single field `path` (string; PGV `min_len: 1`). Sibling to the DataSource oneof, NOT part of it. Semantics: only meaningful when paired with `filename` arm.

### 5.5 `CapabilityRestrictionConfig` + `SanitizationConfig`

`envoy.extensions.wasm.v3.CapabilityRestrictionConfig` (`CENTRAL_PB:139-196`):

| # | Field | Proto type | Go type | PGV | Default | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `allowed_capabilities` | `map<string, SanitizationConfig>` | `map[string]*SanitizationConfig` | `no validation rules for AllowedCapabilities[key]` + per-value embedded validate (`CENTRAL_VAL:189-233`) | nil (empty map) | 25.1 CONSUMED (default-deny posture per AMEND-A5) |

`envoy.extensions.wasm.v3.SanitizationConfig` (`CENTRAL_PB:198-237`) — **EMPTY message** (no fields). Upstream comment at `CENTRAL_PB:198-200`: *"NOTE: This is currently unimplemented."* envoy-go matches upstream byte-faithfully — accept empty `SanitizationConfig{}` values (per §4.3.5).

### 5.6 `EnvironmentVariables`

`envoy.extensions.wasm.v3.EnvironmentVariables` (`CENTRAL_PB:560-625`):

| # | Field | Proto type | Go type | PGV | Default | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `host_env_keys` | repeated string | `[]string` | (no rule emitted; `CENTRAL_VAL:627-641`) | nil | 25.3 CONSUMED (operator-specified host env var pass-through) |
| 2 | `key_values` | `map<string, string>` | `map[string]string` | `no validation rules for KeyValues` (`CENTRAL_VAL:634`) | nil | 25.3 CONSUMED (operator-specified literal env values) |

### 5.7 v1.32.4 vs v1.37.2 binding-gap forward-pointers (per AMEND-A1)

One binding-gap identified:
- **`PluginConfig.allow_on_headers_stop_iteration`** (v1.37.2 field 9; type `google.protobuf.BoolValue`) — Forward-pointer binding-gap. The v1.32.4 protobuf-go parser silently drops the unknown field; no envoy-go-side PARSE-REJECT applies. Activates when go-control-plane bumps to v1.37.x. Semantics: gates whether `proxy_on_request_headers` / `proxy_on_response_headers` may return `ProxyAction::PAUSE` (suspend iteration); the SPEC anticipates this surface lands at 25.2 (paired with the stream-control hostcalls `proxy_continue_stream` + `proxy_close_stream`).

No other v1.32.4 vs v1.37.2 binding-gaps detected in the wasm-relevant proto surface. The HTTP-filter wrapper (`Wasm`), central `PluginConfig` (modulo the one gap), `VmConfig`, `EnvironmentVariables`, `CapabilityRestrictionConfig`, `SanitizationConfig`, `ReloadConfig`, `FailurePolicy` enum, and the `AsyncDataSource` / `DataSource` / `RemoteDataSource` / `WatchedDirectory` envelope all match v1.37.2 modulo `allow_on_headers_stop_iteration`.

---

## 6. PARSE-REJECT roster (per §11.1 D1 — 18 arms at 25.1)

Per §11.1 D1 empirical scrape + AMEND-A1 binding refinements + AMEND-A6 envoy-go-strict-stricter ABI-version PARSE-REJECT. Byte-stable wording per ADR-0080 + the prior-phase precedents (phase-22 SPEC §6 + phase-24 SPEC §5).

### 6.1 Wording discipline + arm-name convention

Per phase-22 SPEC §6.1 + phase-21 SPEC §5 precedent. Format: `"wasm: <field_path>: <reason> [; <forward-pointer hint>]"`. Filter-proto-name prefix `wasm:` invariant on every arm. Constants live as package-private `parseReject*` consts at `internal/filter/http/wasm/compiled_config.go`, returned via `errors.New(parseReject...)` for byte-stability. Kebab-case arm identifiers (used for SPEC cross-reference + test-name suffixes like `TestBuildCompiledConfig_PARSE_REJECT_remote_arm_deferred`) follow `<field-path-with-dots-as-dashes>-<rejection-class>`.

### 6.2 25.1 PARSE-REJECT roster (18 arms)

PGV-baseline note (per §11.1 + `wasm.pb.validate.go`): the `Wasm` wrapper has NO PGV rules on `config`; the `PluginConfig` has NO PGV rules on `name` / `root_id` / `fail_open` / `failure_policy` / `reload_config` / `capability_restriction_config` / `configuration`; the `VmConfig` has NO PGV rules on any field. PGV does require `AsyncDataSource.specifier` + `DataSource.specifier` oneofs (per §5.4). Most 25.1 PARSE-REJECT arms below are envoy-go-strict-as-defensive-mirror (NOT PGV-mirror).

| # | arm-name (kebab-case) | trigger condition | byte-exact error wording |
|---|---|---|---|
| 1 | `typed-config-required` | `typedConfig == nil` at factory entry | `"wasm: typed_config required"` |
| 2 | `typed-config-unmarshal` | `anypb.UnmarshalTo` fails into `*Wasm` | `"wasm: typed_config unmarshal: %w"` |
| 3 | `config-required` | `wasm.Config == nil` (the top-level PluginConfig) | `"wasm: config (PluginConfig) is required"` |
| 4 | `vm-config-required` | `wasm.Config.Vm == nil` OR no `*PluginConfig_VmConfig` arm | `"wasm: config.vm_config is required"` |
| 5 | `vm-config-code-required` | `vm.Code == nil` | `"wasm: config.vm_config.code is required"` |
| 6 | `vm-config-code-remote-deferred` | `vm.Code.GetRemote() != nil` | `"wasm: config.vm_config.code.remote is not yet supported (lands in a future Runtime/RTDS family phase)"` |
| 7 | `data-source-watched-directory-deferred` | `vm.Code.GetLocal() != nil && local.WatchedDirectory != nil` | `"wasm: config.vm_config.code.local.watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"` |
| 8 | `data-source-specifier-required` | `local.Specifier == nil` (PGV-mirror) | `"wasm: config.vm_config.code.local: specifier oneof required"` |
| 9 | `plugin-failure-policy-fail-reload-deferred` | `pc.FailurePolicy == FAIL_RELOAD` OR `pc.ReloadConfig != nil` | `"wasm: config.failure_policy = FAIL_RELOAD (or reload_config set) is not yet supported (lands in phase 25.3)"` |
| 10 | `plugin-fail-open-deferred` | `pc.FailOpen == true` (deprecated field) | `"wasm: config.fail_open is not yet supported (deprecated upstream; lands in phase 25.3 via failure_policy = FAIL_OPEN)"` |
| 11 | `vm-config-runtime-discriminator-deferred` | `vm.Runtime ∉ {"", "envoy.wasm.runtime.wazero"}` | `"wasm: config.vm_config.runtime %q is not supported (envoy-go uses wazero exclusively; envoy-go-strict departure)"` |
| 12 | `vm-config-vm-id-duplicate` | two `PluginConfig` entries within the same listener carry the same non-empty `vm_id` | `"wasm: config.vm_config.vm_id %q is duplicated across PluginConfig entries (multi-plugin VM-sharing lands in phase 25.3)"` |
| 13 | `vm-config-environment-variables-deferred` | `vm.EnvironmentVariables != nil` | `"wasm: config.vm_config.environment_variables is not yet supported (lands in phase 25.3)"` |
| 14 | `vm-config-allow-precompiled-rejected` | `vm.AllowPrecompiled == true` | `"wasm: config.vm_config.allow_precompiled is not supported (incompatible with wazero interpreter-default; envoy-go-strict departure)"` |
| 15 | `vm-config-nack-on-code-cache-miss-rejected` | `vm.NackOnCodeCacheMiss == true` | `"wasm: config.vm_config.nack_on_code_cache_miss is not supported (paired with code.remote; envoy-go-strict departure)"` |
| 16 | `module-abi-version-rejected` | module-instantiation: sentinel export ∉ {`proxy_abi_version_0_2_1`} OR missing sentinel | `"wasm: module: required proxy_abi_version_0_2_1 export not found (envoy-go-strict targets ABI v0.2.1 only; v0.1.0 + v0.2.0 + missing sentinel rejected)"` |
| 17 | `module-compile-failed` | resolved bytes fail wazero `CompileModule` | `"wasm: config.vm_config.code: compile: %w"` |
| 18 | `per-route-deferred-to-25-3` | any `typed_per_filter_config[envoy.filters.http.wasm]` map entry at route/vhost level (via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint) | `"wasm: per-route configuration is not yet supported (lands in phase 25.3)"` |

Notes per arm:
- **Arms 6 + 7** (Remote arm + WatchedDirectory): envoy-go-strict departures (upstream supports both via Runtime/RTDS); BEHAVIOR_CONTRACT departure records at 25.1 IMPL final Task bundle per §13.
- **Arm 11** (runtime discriminator): envoy-go-strict departure per AMEND-A6 + §2.3. BEHAVIOR_CONTRACT departure record.
- **Arms 12 + 13 + 9 + 10**: deferred-to-25.3 wording; upstream Envoy ACCEPTS these (no boot-reject fixture parity); unit-tested + BEHAVIOR_CONTRACT-recorded.
- **Arms 14 + 15**: envoy-go-strict-stricter departures; upstream silently accepts both (allow_precompiled is V8/wasmtime opt-in; nack_on_code_cache_miss pairs with Remote).
- **Arm 16** (ABI-version PARSE-REJECT): envoy-go-strict-stricter departure per AMEND-A6 (upstream version-dispatches all 3 ABI versions; envoy-go rejects v0.1.0 + v0.2.0). BEHAVIOR_CONTRACT departure record. Detection point: `internal/wasm/bytecode_util.go` reimplements `BytecodeUtil::getAbiVersion` byte-faithfully (scans wasm module export section type 7 for function-kind export named `proxy_abi_version_0_2_1`; 24 UTF-8 ASCII bytes).
- **Arm 17** (compile-failure): wraps wazero's compile error via `%w`; structural prefix `"wasm: config.vm_config.code: compile: "` is byte-stable; the wrapped error contents are variable (wazero parser error + line number).
- **Arm 18**: uses the existing HCM `RegisterPerRouteValidator` hook per phase-20 §5.2 + ADR-0110 single-chokepoint; NOT a separate filter-package PARSE-REJECT.

**Boot-reject differential fixture `0035-http-wasm-boot-reject` candidates** (per §8.2): the cleanest boot-reject-by-upstream-too arms are arms 1, 2, 3, 4, 5, 8 (typed_config + required-field-missing arms — upstream ALSO rejects). Arms 6 + 7 + 9-15 + 18 are envoy-go-strict-stricter departures (upstream ACCEPTS) — NOT boot-reject candidates; unit-tested + BEHAVIOR_CONTRACT-recorded. Arm 16 (ABI-version) is a special case: a v0.1.0 module is BOOT-REJECTED by upstream's `Unknown` arm only — but a v0.1.0 module is NOT boot-rejected by upstream (upstream version-dispatches); thus arm 16 is also envoy-go-strict-stricter (NOT a boot-reject-parity candidate). Arm 17 (compile-failure) IS a boot-reject-parity candidate (both upstream and envoy-go fail to instantiate malformed bytecode).

### 6.3 25.2 + 25.3 PARSE-REJECT roster forward-pointers

The 25.2 + 25.3 sub-phase SPECs introduce additional PARSE-REJECT arms covering the body+buffer hostcall capability roster + per-route TPFC PARSE-REJECT arms + multi-plugin VM-sharing semantic arms. Anticipated count: 8-12 additional arms at 25.2 + 6-10 additional arms at 25.3. Authoritative roster lives in each sub-phase's SPEC §6.

---

## 7. Stat surface (per §11.2 D2 + AMEND-A2)

Per §11.2 D2 empirical scrape + AMEND-A2 structural REFUTATION. The BRAINSTORM §5.1 hypothesized 5+2-counter flat `wasm.<vm_id>.<stat>` template is REFUTED; the actual upstream surface is tri-group + the HCM-injected stats_prefix is DROPPED.

### 7.1 25.1 stat-surface roster (5 counters; project 114 → 119)

| # | Internal name | Type | Source | Description |
|---|---|---|---|---|
| 1 | `wasm.wazero.created` | counter | upstream-parity (Group B per AMEND-A2) | Counter increments per VM construction. Per `wasm.cc:81,98` (base-VM ctor + thread-local clone). envoy-go's `<runtime>` is uniformly `"wazero"` because no other runtime is exposed (PARSE-REJECT per §6.2 arm 11). |
| 2 | `wasm.wazero.active` | gauge | upstream-parity (Group B per AMEND-A2) | Live count of `*VM` instances. Backed by atomic counter at the framework level. |
| 3 | `wasm.<plugin_name>.executions` | counter | envoy-go-strict extension (AMEND-A2) | Counter increments per `proxy_on_request_headers` invocation regardless of outcome. Operator-visibility for per-stream-invocation rate. |
| 4 | `wasm.<plugin_name>.hostcall_denied` | counter | envoy-go-strict extension (AMEND-A2 + AMEND-A5) | Counter increments per default-denied hostcall invocation. Operator-visibility for sandbox enforcement. |
| 5 | `wasm.<plugin_name>.envoy_go.failures` | counter | envoy-go-strict extension (AMEND-A2) | Counter increments per VM-failure event (replaces the upstream `FailState`-via-event surface — gives operators a single observable counter for "the VM died" without recreating the `vm_reload_*` mechanism deferred to 25.3). |

**Group A `wasm.remote_load_*` counters (5 counters + 1 gauge per AMEND-A2) are DEFERRED at envoy-go phase 25** because `AsyncDataSource.Remote` is PARSE-REJECT'd per §6.2 arm 6 — no remote bytecode fetch path.

**Group C `wasm.<plugin_name>.vm_reload*` counters (4 counters per AMEND-A2) are DEFERRED at envoy-go phase 25.1 + 25.2** because `failure_policy = FAIL_RELOAD` + `ReloadConfig` are PARSE-REJECT'd per §6.2 arm 9. 25.3 may activate Group C if `failure_policy = FAIL_RELOAD` lands.

### 7.2 Stat-prefix template (per AMEND-A2)

**`wasm.<discriminator>.<stat>`** where `<discriminator>` is:
- For Group B (`created` + `active`): `<runtime>` (= `"wazero"` for envoy-go).
- For Group C (`vm_reload*` deferred to 25.3) + envoy-go-strict counters: `<plugin_name>` (from `PluginConfig.name`).

**HCM-injected `stats_prefix` is DROPPED** per AMEND-A2 (`source/extensions/filters/http/wasm/config.h:51-53` — typed-factory signature drops the stats_prefix string). The wasm filter row DIVERGES from the dominant §9 family-row pattern (every other §9 row uses HCM-rooted SN2-reuse `http.<HCM_stat_prefix>.<filter>.*` per ADR-0143). This divergence is a property of upstream Envoy v1.37.2 + reflected at envoy-go for byte-faithfulness; NOT an envoy-go-strict departure.

**Empty `PluginConfig.name`** produces names with literal empty segment (e.g. `wasm..executions`) — mirrors phase-14 compressor empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243.

### 7.3 Project stat-count delta

**114 → 119 at 25.1 (+5)** per AMEND-A2. 25.2 anticipated additions: likely +4 envoy-go-strict counters (`tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied`) — 25.2 SPEC settles + may consolidate. 25.3 anticipated additions: 0-4 (Group C `vm_reload*` IF `failure_policy = FAIL_RELOAD` activates; 0 otherwise). The plugin-defined dynamic-stats namespace `wasmcustom.<custom_name>` (via `proxy_define_metric` at 25.2) is operator-extensible at runtime — NOT counted in the static stat name total.

### 7.4 envoy-go-strict departure rationale

Per AMEND-A2. The 3 envoy-go-strict extensions (`executions`, `hostcall_denied`, `envoy_go.failures`) are NOT in upstream's stat surface; they are envoy-go-only for operator-visibility into per-stream invocation rate + sandbox enforcement rate + VM-failure rate. The departure rationale + the BEHAVIOR_CONTRACT departure-record discipline land at the 25.1 IMPL final Task's edit bundle per §13 + ADR-0052 atomic landing. **Three stat departure records** at 25.1 (plus the AMEND-A5 default-deny + AMEND-A6 ABI-version-strict + AMEND-A8 conformance-source-departure records — total ~6 envoy-go-strict departure records at 25.1 IMPL bundle).

---

## 8. Differential fixture taxonomy (per §11.6 D6 + §11.7 D9 + AMEND-A4 + AMEND-A8)

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side + boot-reject surfaces are SEPARATE directories from the start. Fixture directory count **35 → 37 at 25.1; 37 → 39 at 25.2; 39 → 41 at 25.3** — total +6 dirs across the family.

### 8.1 25.1 fixture-0034 (cross-side headers-bridge; full byte-exact per AMEND-A4)

Fixture `0034-http-wasm-headers-bridge` lands as full cross-side byte-exact per BRAINSTORM Q6 + AMEND-A4 D6 CONFIRMS + §4.5 guardrails. A vendored Rust-sourced `.wasm` plugin (per Q9: `proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target) drives the headers-bridge scenarios.

#### 8.1.1 Scenario taxonomy (7 scenarios)

| # | Name | Plugin behavior | Wire-output assertion (cross-side) |
|---|---|---|---|
| (a) | add-fixed-header | `proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-wasm-injected", "hello")` in `proxy_on_request_headers` | Request header `x-wasm-injected: hello` present at upstream echobackend |
| (b) | replace-header | `proxy_replace_header_map_value(HTTP_REQUEST_HEADERS, "user-agent", "envoy-go-wasm/1.0")` | Reflected `user-agent: envoy-go-wasm/1.0` |
| (c) | remove-header | `proxy_remove_header_map_value(HTTP_REQUEST_HEADERS, "x-blocked")` | Reflected request without `x-blocked` header |
| (d) | respond-shortcircuit | `proxy_send_local_response(403, "Forbidden", "denied", &[], 0)` | Client receives full byte-pinned tuple: status `403 Forbidden`; `content-length: 6`; body `denied` (6 bytes verbatim); no upstream request initiated |
| (e) | log-only-passthrough | `proxy_log(LogLevel::INFO, "wasm hit")` | Reflected request unchanged at upstream; **stat-counter delta** `wasm.<plugin_name>.executions` increments per probe (subject-side assertion via `StatsAsserter.AssertStats` per `reference_differential_asserter_dispatch`) |
| (f) | header-iteration-count | `proxy_get_header_map_pairs(HTTP_REQUEST_HEADERS, ...)` + count + `proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-headers-count", &n.to_string())` | Reflected header `x-headers-count: N` where N is the count of request headers (deterministic for HTTP/1.1 per §4.5(b) guardrail — counts ignore iteration order) |
| (g) | property-read-method | `proxy_get_property("request.method") → bytes` + `proxy_add_header_map_value("x-request-method", method)` | Reflected `x-request-method: GET` (or similar) — exercises the minimal property tree at 25.1 |

#### 8.1.2 §4.5 guardrail compliance

All 7 scenarios comply with §4.5 D6 guardrails: (a) no memory-trap probes; (b) HTTP/1.1 transport (no HPACK reorder); (c) no float-formatted log lines; (d) all hostcalls within the 24-hostcall 25.1 surface.

#### 8.1.3 Recommended fixture-0034 directory structure

```
test/fixtures/0034-http-wasm-headers-bridge/
  README.md             # ~150-250 lines: scope + 7-scenario table + topology + cross-refs to SPEC §8 + ADR-0202+0203+0204
  envoy.yaml            # reference Envoy bootstrap; single listener + wasm filter; templated {{.BackendPort}}
  envoy-go.yaml         # subject bootstrap; same topology
  expectations.yaml     # human-readable declarative scenario expectations (NOT consumed by runner)
  inputs/
    driver.go           # registered Driver impl (~400-600 LoC); per-scenario probes + classifyBody
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

NEW `BackendKind=HTTPWasm` constant added at `test/differential/runner_test.go` near the existing `HTTPLua`/`HTTPCsrf`/`HTTPCompressor` precedent — purely a switch-case addition; ~20 LoC delta.

### 8.2 25.1 fixture-0035 (boot-reject)

`0035-http-wasm-boot-reject`: PGV-mirror reject where upstream Envoy ALSO rejects at boot. Cleanest single-arm: **missing `vm_config.code`** (arm 5 `vm-config-code-required`) — upstream PGV rejects via the `AsyncDataSource.specifier required` rule (`CORE_VAL:3497-3506`); envoy-go's PGV-mirror rejects with the §6.2 arm 5 wording. The fixture pins the common distinctive stderr substring (e.g. `"is required"` or `"vm_config.code"`).

Per phase-22 + phase-23 + phase-24 boot-reject precedent, the 0035 fixture exercises ONE arm at boot-reject parity; the other envoy-go-strict-stricter arms (6, 7, 9-15, 16, 18) are unit-tested + BEHAVIOR_CONTRACT-recorded.

### 8.3 25.2 + 25.3 fixtures (forward-pointer)

- **25.2 fixture-0036** `0036-http-wasm-body-and-advanced`: PARTIAL cross-side per ADR-0192 precedent. Deterministic legs (body-read-only / body-mutation / shared-data-read-after-write) cross-side byte-exact; non-deterministic legs (tick timing / httpCall response sequencing / metric emission timing) fall back to subject-only assertions via the multi-listener mixed-mode pattern. 25.2 BRAINSTORM/SPEC settles taxonomy.
- **25.2 fixture-0037** `0037-http-wasm-body-and-advanced-boot-reject`: PGV-mirror reject for malformed advanced-hostcall config (e.g. invalid capability_restriction_config arm structure).
- **25.3 fixture-0038** `0038-http-wasm-perroute-and-multi-plugin`: cross-side byte-exact for deterministic per-route override scenarios (per-route applies / per-route disabled / listener-default applies) + multi-plugin scenarios (multiple PluginConfig entries with same vm_id share VM; distinct plugin contexts).
- **25.3 fixture-0039** `0039-http-wasm-perroute-boot-reject`: PGV-mirror reject for dangling vm_id reference or malformed per-route TPFC.

### 8.4 Conformance harness seed at 25.3 (per AMEND-A8 + §11.7 D9)

`test/conformance/proxy-wasm/` seeds at 25.3 IMPL per `BOOTSTRAP_PROMPT.md §7.3` anticipation + AMEND-A8. The source: **`proxy-wasm/proxy-wasm-cpp-host@da3ce05d` test/ directory** (NOT `proxy-wasm/spec` — that repo ships NO conformance suite per AMEND-A8 REFUTATION).

**Pin target:** `proxy-wasm-cpp-host@da3ce05d8d59ebccbfcad434bb4784c98a4ece6a` (`main` HEAD at scrape date 2026-05-20; message "Implement execution limits for wasmtime (#510)"). This repo has NO tags; SHA pinning is the only option.

**Vendored bytecode roster** (10 of 14 cpp-host test_data binaries; the 4 deferred are signed-variants + null-vm-only fixtures):
- `abi_export.wasm`, `callback.wasm`, `clock.wasm`, `endianness.wasm`, `env.wasm`, `random.wasm`, `canary_check.wasm`, `http_logging.wasm`, `stop_iteration.wasm`, `bad_malloc.wasm`, `resource_limits.wasm`, `trap.wasm`.

**Pass-threshold disposition at 25.3 phase-done: 10/16 test families ported = 62.5% PASS.**

**Deferred (6 families, with rationale):**
- `null_vm_test` — C++-only null VM, not portable to wazero.
- `security_test` — signed-bytecode verification, out of phase-25 scope.
- `signature_util_test` — wasm-module signature handling, same.
- `shared_data_test` — requires shared-data hostcalls (25.2 territory; deferred to 25.4-or-WASM-host-family follow-up if not landed at 25.2).
- `shared_queue_test` — requires shared-queue hostcalls (deferred per §2.15).
- `fuzz/` — fuzz targets, out of phase-25 scope.

Phase-05 h2spec (53/53 PASS at the ADR-0051 pin) is the closest precedent for the conformance-pinning ADR shape; phase-25.3 follows the format with the scoped-subset threshold per AMEND-A8.

### 8.5 Total fixture-dir count

35 → 37 at 25.1 (+2: `0034` + `0035`); 37 → 39 at 25.2 (+2: `0036` + `0037`); 39 → 41 at 25.3 (+2: `0038` + `0039`). **Total +6 across the family.** Plus 1 new conformance harness directory `test/conformance/proxy-wasm/` at 25.3 (NOT counted in the fixture-dir total per `reference_differential_fixture_dispatch_constraint`; conformance lives in a separate harness scope per phase-05 + grpc-conformance precedent).

### 8.6 Listener topology

Single listener with a single HCM containing the wasm filter (alphabetical position) + router terminator. SPEC confirms whether a second listener is needed for any 25.2 scenario (e.g. `proxy_http_call` to a second cluster — avoid the `freeTCPPort` combined-run flake surface per phase-22.2 REVIEW §7.4 where possible).

---

## 9. Behavior-contract delta (semantic; per AMEND-A2 + A5 + A6 + A8 + A9)

The phase-25 behavior-contract delta vs phase-24 baseline (high-level semantic changes; the verbatim Markdown patch lives at §13):

1. **WebAssembly-bytecode-driven HTTP filter semantics** — SECOND class of filter that delegates per-request behavior to operator-authored Turing-complete bytecode (phase-22 lua introduced the first; phase-25 introduces the second via WebAssembly). Observable: operator-supplied `.wasm` bytecode compiled at config-load (PARSE-REJECT on compile failure per §6.2 arm 17; PARSE-REJECT on ABI version ≠ v0.2.1 per arm 16); per-request invocation of the 30 `proxy_on_*` guest-exported callbacks against the in-house 47-hostcall proxy-wasm v0.2.1 host ABI. The behavior depends entirely on the operator's bytecode — envoy-go's behavior-contract claim is **upstream-parity at the hostcall ABI wire level** (modulo the documented divergences listed below).

2. **Default-deny capability sandbox envoy-go-strict departure** (per AMEND-A5 + ADR-0204). Documented as an envoy-go-strict departure from upstream's bare-empty-map-allow-all posture. Rationale: WASM has a substantially larger and riskier hostcall surface than Lua (proxy_call_foreign_function for arbitrary host-side dispatch; proxy_dispatch_http_call for outbound network; proxy_set_shared_data for cross-stream state; proxy_define_metric for unbounded dynamic-stat namespace creation); upstream Envoy v1.37.2 marks its 3 sandbox runtimes (V8, WAMR, Wasmtime) as `status: alpha` + `security_posture: unknown` — the alpha-status posture is incompatible with envoy-go's safe-by-default discipline. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table.

3. **proxy-wasm ABI v0.1.0 + v0.2.0 PARSE-REJECT envoy-go-strict-stricter departure** (per AMEND-A6 + ADR-0203). Documented as an envoy-go-strict-stricter departure from upstream's accept-all-3-ABI-versions behavior. Detection point: `BytecodeUtil::getAbiVersion` byte-faithful reimplementation; require `proxy_abi_version_0_2_1` sentinel export. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table.

4. **`AsyncDataSource.Remote` PARSE-REJECT envoy-go-strict departure** (per §2.1 + §6.2 arm 6). Upstream supports remote bytecode fetch via the Group-A remote-load stat surface; envoy-go PARSE-REJECTs pending the Runtime/RTDS family phase. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table.

5. **Runtime-name discriminator PARSE-REJECT envoy-go-strict departure** (per §2.3 + §6.2 arm 11). Upstream accepts `envoy.wasm.runtime.{v8,wamr,wasmtime,null}` via 4 separate wasm-runtime extensions; envoy-go accepts only the empty-string default (→ wazero) and `"envoy.wasm.runtime.wazero"`. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table.

6. **`executions` + `hostcall_denied` + `envoy_go.failures` envoy-go-strict counters** (per AMEND-A2 + ADR-0203). Documented as 3 envoy-go-strict additions for operator-visibility into per-stream invocation rate + sandbox enforcement rate + VM-failure rate. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table.

7. **Stat-prefix template DIVERGES from §9 family-row pattern** (per AMEND-A2). The wasm filter row uses tri-group prefix structure (`wasm.<runtime>.{created,active}` Group B + `wasm.<plugin_name>.{...}` Group C + the envoy-go-strict counters) rather than the dominant HCM-rooted SN2-reuse `http.<HCM_stat_prefix>.<filter>.*` per ADR-0143. This divergence is a property of upstream Envoy v1.37.2 (`source/extensions/filters/http/wasm/config.h:51-53` drops the HCM-injected stats_prefix); NOT an envoy-go-strict departure — upstream-parity preservation. Recorded at BEHAVIOR_CONTRACT.md stat-name mapping section as a special-case row with cross-reference.

8. **Foreign-function 0-vs-10 default registry envoy-go-strict departure** (per AMEND-A9 + ADR-0203 at 25.2). Upstream registers 10 foreign functions by default; envoy-go registers ZERO. Operators must explicitly enable the `proxy_call_foreign_function` capability AND register specific foreign functions at multi-consumer scope. Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures table at 25.2.

9. **Conformance-suite source REFINES from BRAINSTORM hypothesis** (per AMEND-A8 + ADR-0212 at 25.3). `proxy-wasm/spec` ships NO conformance suite; `test/conformance/proxy-wasm/` seeds from `proxy-wasm-cpp-host@da3ce05d` test/ with a 62.5% starting pass-threshold. Recorded at BEHAVIOR_CONTRACT.md conformance-harness section at 25.3.

10. **wazero-vs-V8 cross-side risk envelope** (per AMEND-A4 + §4.5 D6 guardrails). The 25.1 fixture-0034 cross-side byte-exact claim depends on the §4.5 (a)-(d) guardrails. Recorded at BEHAVIOR_CONTRACT.md cross-side-equivalence carve-out at 25.1.

---

## 10. ADR anchor map (3 NEW §Context drafts at 25.1; +N at 25.2; +3 at 25.3; ZERO IN-PLACE AMENDMENTs; ZERO ADR-0125 amendments per AMEND-A3)

Per ADR-0044: the ADR-0202..ADR-0204 §Context drafts anchor at this SPEC commit (appended to `DECISIONS.md`); §Decision + §Consequences bodies land at each ADR's Lands-in-Task at 25.1 IMPL.

### 10.1 25.1 ADRs (ADR-0202 + ADR-0203 + ADR-0204)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0202** | NEW `internal/wasm/` framework primitive — wazero VM lifecycle + per-stream `*Runtime` construction + per-module `*Module` compile cache + in-house proxy-wasm v0.2.1 host ABI surface (47 hostcalls: 40 `proxy_*` + 7 mandatory WASI shims; 30 guest-exported callbacks) + WasmResult/WasmBufferType/WasmHeaderMapType/LogLevel/StreamType/MetricType/ProxyAction/WasiErrno type definitions (value-gap-preserving per AMEND-A7) + ABI-registration interface + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (WASM host family phases) + wazero v1.10.1 go.mod direct dependency pin per AMEND-A1 + proxy-wasm-rust-sdk =0.2.4 test-bytecode pin + `wasm32-wasip1` target | §1; §4.1; §4.2; §11.5 D4; §1.1 AMEND-A1 + A4 + A6 + A7 | 25.1 IMPL first task that materializes `internal/wasm/` (anticipated Task 1-3 per the phase-22.1 + phase-17 framework-primitive-first-task precedent) |
| **ADR-0203** | NEW `internal/filter/http/wasm/` package shape — HTTP-filter-specific config parse + 4-arm AsyncDataSource Local resolution + 18-arm PARSE-REJECT roster + 5-counter envoy-go-strict stat surface per AMEND-A2 (tri-group prefix structure; HCM-stats_prefix DROPPED) + filter-callback wiring + fixture-0034 full-cross-side discipline with §4.5 D6 guardrails + envoy-go-strict-stricter ABI v0.1.0+v0.2.0 PARSE-REJECT per AMEND-A6 + 4 envoy-go-strict departure records | §1; §4.4; §5; §6; §7; §8; §1.1 AMEND-A1 + A2 + A4 + A6 | 25.1 IMPL first task that materializes `internal/filter/http/wasm/` (anticipated Task 1-3) |
| **ADR-0204** | proxy-wasm capability-restriction default-deny + envoy-go-strict sandbox posture — empty `allowed_capabilities` ⇒ DENY-ALL (departure from upstream allow-all per AMEND-A5) + capability key roster (~80 keys; 25.1 covers 24 hostcall keys + 13 module-getter callbacks) + denial semantic (`WasmResult::InternalFailure` + integration error log + `hostcall_denied` counter increment) + accept-empty-SanitizationConfig discipline per AMEND-A1 §11.4 + BEHAVIOR_CONTRACT departure record | §2.5; §4.3; §1.1 AMEND-A5 | 25.1 IMPL Lands-in-Task at sandbox-config materialization |

### 10.2 25.2 ADRs (anticipated ~3-4 ADRs; provisional ADR-0205 .. ADR-0208)

Per BRAINSTORM §7.2 + AMEND-A9 closure. 25.2 BRAINSTORM/SPEC settles exact ADR roster.

| ADR | Anticipated subject |
|---|---|
| **ADR-0205** | `internal/wasm/` 25.2 ABI extensions — body + buffer + trailers + timer + metrics + shared-data + httpCall (RE-CONSUMES phase-20 `internal/httpclient/` per ADR-0177) + full stream-info property surface. Capability roster extension. |
| **ADR-0206** | `internal/wasm/foreign.go` foreign-function registration interface per AMEND-A9 — registration interface + EMPTY default registry + `WasmResult::NotFound` (=1) for unregistered names + capability-roster default-deny via `proxy_call_foreign_function` key. envoy-go-strict departure record (0-vs-10 default foreign-function registry). |
| **ADR-0207** | `internal/filter/http/wasm/` 25.2 package shape extensions — full hostcall set wiring + envoy-go-strict counter extensions (`tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied`) + dynamic-stats namespace `wasmcustom.<custom_name>` admin-discoverability discipline + fixture-0036 mixed-mode discipline. |
| **ADR-0208** | (Escape-valve reserve at 25.2 — likely candidates: dynamic-metadata bridge disposition; shared-queue defer disposition; gRPC-hostcall-defer-to-WASM-host-family ADR.) |

### 10.3 25.3 ADRs (anticipated 3 ADRs; provisional ADR-0210 + ADR-0211 + ADR-0212)

| ADR | Anticipated subject |
|---|---|
| **ADR-0210** | Per-route Wasm 5th-canonical REUSE-by-absence EXPLICIT-NO-NEW-CANONICAL classification per AMEND-A3 (analogous to ADR-0173 / ADR-0180 — no `WasmPerRoute` proto exists in v1.32.4 binding OR v1.37.2 IDL; per-route override is wholesale-override via TPFC of the `Wasm` message itself; ADR-0125 STAYS at 10 canonicals + NO §(xvi) amendment). |
| **ADR-0211** | Multi-plugin VM-sharing semantics — `vm_id`-keyed VM reuse + plugin-context isolation discipline + cross-plugin shared-data scoping (if shared-data lands at 25.2). |
| **ADR-0212** | `test/conformance/proxy-wasm/` conformance harness seed + pin SHA `proxy-wasm-cpp-host@da3ce05d` per AMEND-A8 + 10-of-16 test family port + 62.5% starting pass-threshold + 6-family deferral with rationale (analogous to ADR-0051 h2spec discipline but with scoped-subset disposition rather than 100%-pass-on-pinned-commit). |

### 10.4 ZERO IN-PLACE §Decision AMENDMENTs + ZERO ADR-0125 amendments

Per AMEND-A3: ADR-0125's canonical roster STAYS at 10 entries — **NO §(xvi) amendment** at phase 25.3 IMPL. The BRAINSTORM-anticipated escape-valve "ADR-0125 amendment 10 → 11" is RETIRED at this SPEC commit. No other in-place ADR §Decision body amendments anticipated.

### 10.5 ADR-0044 escape-valve reserve + STRENGTHENED-WEAK-HOLD D-hypothesis

Per §1.2: STRENGTHENED-WEAK-HOLD with **1-slot-buffer** at 25.1 IMPL (vs BRAINSTORM Q10's 2-slot-buffer). The 1 residual buffer covers (a) the wazero-VM-pool design escape-valve + (b) any wazero/V8 sub-pin edge case the §4.5 guardrails missed. Provisional ADR-0205 reserve slot.

**D-style hypothesis at SPEC commit:** ADR-0205 may consume at 25.1 IMPL (most-likely surface: wazero-VM-pool ADR if per-stream construction cost exceeds 1ms). The probability is LOW (phase-22.1 `*LState`-pool benchmark gate observed 70µs per construction, well under threshold; wazero compiler-mode initialization is comparable or faster); WEAK-HOLD stands.

### 10.6 Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts at 25.1 SPEC commit | 3 | ADR-0202; ADR-0203; ADR-0204 |
| Anticipated NEW ADRs at 25.2 | 3-4 | ADR-0205; ADR-0206; ADR-0207; ADR-0208 (reserve) |
| Anticipated NEW ADRs at 25.3 | 3 | ADR-0210; ADR-0211; ADR-0212 |
| IN-PLACE §Decision AMENDMENT-anticipation | 0 | NONE |
| ADR-0125 amendments | 0 (RETIRED per AMEND-A3) | NONE |
| ADR-0044 escape-valve reserve | 0-1 at 25.1 IMPL | ADR-0205 reserved if VM-pool fires |

**Next-free ADR post-25-SPEC commit: `ADR-0205`** (3 NEW consumed: ADR-0202..ADR-0204). Anticipated next-free after 25.3 phase-done: **`ADR-0213`** (matching BRAINSTORM §7.4 estimate of "ADR-0202..ADR-0213 anticipated across the family").

---

## 11. Empirical-pin block (D1–D9 resolved at this SPEC session)

This block contains the parallel-subagent-fan-out scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09–24 SPEC §11's structure. **Probe date: 2026-05-24.** The 9 pins span all three sub-phases, so they are resolved once, here, in the parent SPEC; each sub-phase SPEC references this block.

**Reference source corpus** (multi-axis verification per the phase-15..24 discipline):

1. **`go-control-plane v1.32.4` bindings** (the ADR-0008 proto pin) at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/`: `extensions/filters/http/wasm/v3/{wasm.pb.go, wasm.pb.validate.go}`; `extensions/wasm/v3/{wasm.pb.go, wasm.pb.validate.go}` (the central PluginConfig + VmConfig + CapabilityRestrictionConfig + SanitizationConfig + ReloadConfig + EnvironmentVariables + FailurePolicy enum + WasmService); `config/core/v3/{base.pb.go, base.pb.validate.go}` (AsyncDataSource + DataSource + RemoteDataSource + WatchedDirectory).
2. **Upstream Envoy v1.37.2 source + IDL** via WebFetch against `github.com/envoyproxy/envoy` at tag v1.37.2: `source/extensions/filters/http/wasm/{wasm_filter.h, wasm_filter.cc, config.h, config.cc}`; `source/extensions/common/wasm/{wasm.h, wasm.cc, context.h, context.cc, foreign.cc, plugin.h, plugin.cc, stats_handler.h, stats_handler.cc, remote_async_datasource.h, wasm_runtime_factory.h}`; `source/extensions/wasm_runtime/v8/config.cc`; `source/extensions/extensions_metadata.yaml:1631-1635`; `api/envoy/extensions/wasm/v3/wasm.proto`; `api/envoy/extensions/filters/http/wasm/v3/wasm.proto`.
3. **`proxy-wasm/proxy-wasm-cpp-host@main` (de-facto reference host)** via WebFetch: `include/proxy-wasm/{wasm.h, wasm_vm.h, bytecode_util.h, exports.h}`; `src/{wasm.cc, exports.cc, bytecode_util.cc}`. Pin SHA: `da3ce05d8d59ebccbfcad434bb4784c98a4ece6a` (`main` HEAD at scrape date 2026-05-20).
4. **`proxy-wasm/spec@main`** via WebFetch: `abi-versions/v0.2.1/README.md` (the 63KB ABI v0.2.1 specification document).
5. **`wazero/wazero@main`** via WebFetch + GitHub API: `README.md` Conformance section; `internal/integration_test/spectest/v{1,2}/`; `go.mod`; open-issue list filtered by `state:open label:bug` (7 results at scrape date).
6. **`proxy-wasm/proxy-wasm-rust-sdk@v0.2.4`** via WebFetch: `src/lib.rs` (verify `proxy_abi_version_0_2_1` sentinel symbol); `Cargo.toml` (license + dependencies); `examples/{http_headers,http_body}/README.md` (the `wasm32-wasip1` target invocation).

### Summary disposition table (9 pins → 9 AMENDs)

| Pin | Topic | Disposition | AMEND cross-ref |
|---|---|---|---|
| §11.1 | D1 — Filter-config field roster (Wasm + PluginConfig + VmConfig + CapabilityRestrictionConfig + AsyncDataSource/DataSource against v1.32.4 + v1.37.2) | CONFIRMS + REFINES (PluginConfig roster extended with `failure_policy` enum + `reload_config` + `allow_precompiled` + `nack_on_code_cache_miss`; SanitizationConfig empty-as-no-op; 1 binding-gap forward-pointer for `allow_on_headers_stop_iteration`) | AMEND-A1 |
| §11.2 | D2 — Stat roster + prefix template | REFUTES (BRAINSTORM 5+2 flat `wasm.<vm_id>.<stat>` template wrong; actual tri-group `wasm.<runtime>.*` + `wasm.<plugin>.*` + `wasm.remote_load_*`; HCM-stats_prefix DROPPED; no `vm_id` discriminator) | AMEND-A2 |
| §11.3 | D3 — `WasmPerRoute` shape | CONFIRMS no such message exists (5th-canonical REUSE-by-absence; ADR-0125 STAYS at 10) | AMEND-A3 |
| §11.4 | D7 — Default-deny capability roster | CONFIRMS departure (upstream `capabilityAllowed` gate empty-map-allow-all; envoy-go inverts to deny-all); EXTENDS with ~80-key capability roster + denial semantic (`WasmResult::InternalFailure`) + SanitizationConfig accept-empty discipline | AMEND-A5 |
| §11.5 | D4 + AMEND-A6 — proxy-wasm v0.2.1 host ABI surface + ABI-version PARSE-REJECT | CONFIRMS 47-hostcall + 30-callback rosters; REFUTES BRAINSTORM-hypothesized `WasmResult` enum (10 values + value-gaps at 5/9/11; NOT 13); REFUTES `WasmBufferType` value 8 name (FOREIGN_FUNCTION_ARGUMENTS, NOT CallData); REFINES 25.1 hostcall surface from ~17 to ~24 (7 mandatory WASI shims + `proxy_get_log_level` + `proxy_get_current_time_nanoseconds`); REFINES BRAINSTORM "DEPRECATED upstream" v0.1.0 framing to "envoy-go-strict-stricter departure" (upstream accepts all 3 ABI versions and version-dispatches) | AMEND-A6 + A7 |
| §11.5 | D5 — wazero + proxy-wasm-rust-sdk version pins | REFINES (pin: wazero `v1.10.1` Go 1.23.0 floor; proxy-wasm-rust-sdk `=0.2.4`; `wasm32-wasip1` target — NOT BRAINSTORM-quoted `wasm32-wasi`); REFUTES BRAINSTORM "MIT-licensed" wazero claim (actual Apache-2.0) | AMEND-A1 |
| §11.6 | D6 — wazero-vs-V8 observable-output divergence | CONFIRMS BRAINSTORM §2.7 hypothesis (wazero compiler-mode is WebAssembly Core 1.0+2.0 spectest-green; proxy-wasm ABI is wire-defined + host-owned; zero open wazero bugs on compiler-mode wasm-1.0 semantics) WITH §4.5 guardrails (no memory-trap probes; no HTTP/2 header-iteration-order dependence; no float-formatted log lines; no out-of-25.1-subset hostcalls) | AMEND-A4 |
| §11.7 | D9 — proxy-wasm conformance bytecode pinning + pass-threshold | REFUTES BRAINSTORM-implicit source (`proxy-wasm/spec` ships NO test suite); REFINES source to `proxy-wasm-cpp-host@da3ce05d` test/ (14 bytecode + 16 driver .cc files); REFINES pass-threshold from 100% to 62.5% starting target at 25.3 phase-done (10/16 families ported; 6 deferred with rationale) | AMEND-A8 |
| §11.8 | D8 — foreign-function disposition + sub-phase LoC/task estimate | RATIFIES option (b) registration interface + empty default registry + `WasmResult::NotFound` (=1) for unregistered names per AMEND-A9; REFINES BRAINSTORM §1.4 sub-phase LoC/task estimates (25.1 ~17-19 tasks ~3,650-5,360 LIVE; 25.2 ~20-24 tasks ~2,730-4,250 LIVE; 25.3 ~9-12 tasks ~1,300-2,350 LIVE; family-final ~46-55 tasks ~7,680-11,960 LIVE — within BRAINSTORM 42-54 task range; LoC above BRAINSTORM ~5000-7000 estimate but task-arm gate fits all 3 sub-phases) | AMEND-A9 |

The detailed scrape evidence per pin (line-numbered citations from `wasm.pb.go`, `stats_handler.h`, `wasm.cc`, `bytecode_util.cc`, the proxy-wasm v0.2.1 spec README, wazero CI surface, etc.) is captured in the AMEND-A1..A9 block at §1.1 + the per-§§ narrative; the parent SPEC author treats §1.1 + §§4-8 as the load-bearing transcription of the scrape evidence. Sub-phase SPECs do not re-execute these pins.

---

## 12. SPEC-time D-questions for PLAN-time resolution

Per phase-22 SPEC §12 + phase-21 SPEC §12 + phase-18+19+20 D-question precedent. SPEC-time D-questions surface unresolved decisions that the parent SPEC author anchors for PLAN-time resolution (the 25.1 PLAN session that follows the 25.1 SPEC). The parent SPEC has CLOSED most D-question candidates at SPEC commit per AMEND-A1..A9; the remaining open D-questions:

### D-P1: WASI denial errno (per §4.3.2)

**Question:** When a WASI hostcall is denied via the capability gate, should envoy-go return `WasiErrno::NOTSUP` (=58) OR `WasiErrno::ENOTCAPABLE` (=76, upstream's choice per `proxy_wasm_exports.h:232-249`)?

**Resolution at:** 25.1 IMPL first-task scrape against the upstream stub at `proxy_wasm_exports.h:232-249` + cross-check the 7-WASI-shim implementation's behavior under wazero. **Anticipated answer:** mirror upstream (`ENOTCAPABLE = 76`) for byte-faithfulness; if wazero's WASI semantics prevent the exact return code, fall back to `NOTSUP = 58` + record a sub-pin envoy-go-strict departure.

### D-P2: Module-init callback capability-gating (per §4.3.4)

**Question:** Do the 5 module-init/allocator callbacks (`_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate`) participate in capability gating at the `getFunction` lookup side, OR are they unconditionally ungated at the runtime entry-point?

**Resolution at:** 25.1 IMPL first-task scrape against the upstream `getFunction` discipline at `proxy-wasm-cpp-host:wasm.cc:298-302`. **Anticipated answer:** ungated (the module-init callbacks are required for instantiation; gating them would break every module).

### D-P3: `proxy_get_status` consumer of ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` (per §2.28)

**Question:** Should the 25.1 `proxy_get_status` hostcall implementation consume the ADR-0196 `EncoderFilterCallbacks.ResponseStatus() int` accessor (introduced at phase-23 admission_control), OR resolve the status code via a different path?

**Resolution at:** 25.1 IMPL first-task scrape against the encoder-callback shape + ADR-0196's accessor signature. **Anticipated answer:** consume ADR-0196 (FIRST co-consumer of the phase-23 primitive — RATIFIES the phase-23 extraction discipline analogous to phase-22.2's first co-consumer of phase-20 `internal/httpclient/`).

### D-P4: wazero-VM-pool benchmark gate (per §1.2 escape-valve)

**Question:** Does 25.1 IMPL `BenchmarkPerStreamVM_Construction_Headers` measure per-stream wazero Runtime construction cost > 1ms?

**Resolution at:** 25.1 IMPL benchmark task (analogous to phase-22.1 Task 12 `BenchmarkPerStreamLState_Construction_Headers`). **Anticipated answer:** under threshold (phase-22.1 observed 70µs for gopher-lua; wazero compiler-mode initialization is comparable). If exceeded, the ADR-0205 escape-valve slot anchors a "per-module wazero Runtime pool with pre-instantiated entries" decision.

### D-P5: 25.1 PARSE-REJECT byte-stable wording finalization (per §6.1)

**Question:** Pin the exact byte-stable wording for the 18-arm 25.1 PARSE-REJECT roster (the §6.2 table is provisional; IMPL anchors via `TestParseRejectConstants_ByteStable` table per ADR-0080).

**Resolution at:** 25.1 IMPL compiled_config.go authoring + the byte-stable test landing.

### D-P6: 25.1 boot-reject fixture arm finalization (per §8.2)

**Question:** Confirm arm 5 `vm-config-code-required` is the cleanest boot-reject parity candidate for fixture-0035, OR pick an alternative from arms {3, 4, 5, 8, 17}.

**Resolution at:** 25.1 IMPL fixture-0035 task — empirical-test the candidate arms against upstream Envoy v1.37.2 boot stderr; pick the arm with the most distinctive substring + cleanest config shape. **Anticipated answer:** arm 5 (`vm-config-code-required`) — the `AsyncDataSource.specifier required` PGV rule produces an upstream stderr substring containing `"required"` that envoy-go's mirror wording reproduces.

---

## 13. RATIFIED-PENDING-IMPL items + BEHAVIOR_CONTRACT.md edit bundle

Per phase-22 SPEC §13 + §14 + phase-21 SPEC §13. Items the SPEC anchors as RATIFIED at SPEC commit but pending IMPL-time confirmation against the actual envoy-go codebase state.

### 13.1 Wire-shape byte-confirmations

- **R1: 25.1 fixture-0034 cross-side byte-exact + §4.5 D6 guardrail compliance.** 25.1 IMPL fixture-authoring task verifies each scenario complies with §4.5 (a)-(d); lint-pass + run against reference Envoy v1.37.2 byte-for-byte.

- **R2: ABI v0.1.0 + v0.2.0 PARSE-REJECT byte-faithful detection point.** 25.1 IMPL `internal/wasm/bytecode_util.go` reimplements `BytecodeUtil::getAbiVersion` byte-faithfully per AMEND-A6 + §11.5 D4 + `proxy-wasm-cpp-host:bytecode_util.cc:32-97` transcription target.

- **R3: pairs wire format byte-faithful reimplementation.** 25.1 IMPL `internal/wasm/pairs.go` reimplements the `u32 num_pairs / u32 key_len, u32 value_len / key_bytes NUL value_bytes NUL` serialization byte-faithfully from `proxy-wasm-cpp-host:pairs_util.h` + `pairs_util.cc` transcription target. Load-bearing for all header-map hostcalls cross-side parity.

### 13.2 Library-behavioral

- **R4: WASI shim custom 8-stub implementation.** 25.1 IMPL `internal/wasm/wasi.go` implements the 8 WASI hostcalls with proxy-wasm semantics (NOT wazero's built-in `imports/wasi_snapshot_preview1` package); `fd_write` routes to `proxy_log`; `proc_exit` traps. Per AMEND-A4 + §11.5 D4 + §4.2.

- **R5: 34th project-wide fuzzer count verification.** 25.1 IMPL first-task scrape `grep -c "^func Fuzz" $(find /home/esa/git/envoy-go -name 'fuzz_test.go')` confirms project-wide fuzzer count = 33 at master tip; the new `FuzzWasmConfigParse` is the 34th.

### 13.3 Cross-phase regression

- **R6: ADR-0177 `internal/httpclient/` co-consumer validation at 25.2.** The 25.2 IMPL `proxy_http_call` task lands the next co-consumer of phase-20's `internal/httpclient/` primitive after phase-22.2 (`:httpCall()` was the first). RATIFIES the phase-20 framework-primitive extraction discipline per ADR-0177 §Consequences forward-pointer.

- **R7: ADR-0196 `EncoderFilterCallbacks.ResponseStatus()` first co-consumer at 25.1.** Per D-P3 anticipated answer — the `proxy_get_status` hostcall consumes ADR-0196's accessor at 25.1 IMPL. RATIFIES the phase-23 framework-primitive extraction discipline.

### 13.4 Sandbox + perf

- **R8: wazero per-stream Runtime construction benchmark.** Per D-P4 + §1.2 escape-valve candidate. 25.1 IMPL benchmark task gates whether ADR-0205 escape-valve fires.

### 13.5 BEHAVIOR_CONTRACT.md edit bundle anticipation (per ADR-0052 atomic landing)

Per ADR-0052 in-place-edit authorization + phase-22 SPEC §14 + phase-24 SPEC §13 precedent. The BEHAVIOR_CONTRACT.md gains its phase-25 content in three passes across the 3 sub-phases (one bundle per sub-phase IMPL final Task). The 25.1 IMPL final-Task **6-edit bundle**:

1. **NEW `### envoy.filters.http.wasm` subsection** under the §9 family filter documentation. Headers-bridge-focused for 25.1; carries forward-pointers to 25.2 (body+advanced) + 25.3 (per-route + multi-plugin + conformance). ~150-250 lines. References the 24-hostcall 25.1 surface + the 13-callback 25.1 guest-export surface + the 5-counter stat roster.

2. **Stat-table 114 → 119 extension** under BEHAVIOR_CONTRACT.md `## Stat surface`. 5 new rows under tri-group prefix structure per AMEND-A2:
   - `wasm.wazero.created` (counter; upstream-parity; Group B per AMEND-A2)
   - `wasm.wazero.active` (gauge; upstream-parity; Group B)
   - `wasm.<plugin_name>.executions` (counter; envoy-go-strict extension per AMEND-A2)
   - `wasm.<plugin_name>.hostcall_denied` (counter; envoy-go-strict extension per AMEND-A2 + AMEND-A5)
   - `wasm.<plugin_name>.envoy_go.failures` (counter; envoy-go-strict extension per AMEND-A2)
   Plus a structural note documenting the HCM-stats_prefix-DROPPED + tri-group divergence from §9 family-row pattern per AMEND-A2.

3. **envoy-go-strict departure record #1: default-deny capability sandbox** (per AMEND-A5). NEW row at BEHAVIOR_CONTRACT.md envoy-go-strict departures table. Departure-record count 18 → 19.

4. **envoy-go-strict departure record #2: ABI v0.1.0 + v0.2.0 PARSE-REJECT** (per AMEND-A6). NEW row. Departure-record count 19 → 20.

5. **envoy-go-strict departure record #3: `AsyncDataSource.Remote` PARSE-REJECT + runtime-name discriminator PARSE-REJECT + `executions` + `hostcall_denied` + `envoy_go.failures` envoy-go-strict counters + 0-vs-10-foreign-function (when 25.2 lands; not at 25.1 bundle)** — consolidated into ~4-5 envoy-go-strict departure records at 25.1 bundle. Departure-record count 20 → 24-25.

6. **NEW `### Phase 25.1 forward-pointer notes` subsection.** Documents 25.2-anticipated additions (body+buffer hostcalls; timer/metric/shared-data/httpCall; foreign-function-with-empty-default-registry per AMEND-A9; dynamic-metadata-bridge disposition) + 25.3-anticipated additions (per-route 5th-canonical REUSE-by-absence per AMEND-A3 — NO ADR-0125 amendment; multi-plugin VM-sharing; conformance harness seed at 62.5% threshold per AMEND-A8). ~50-80 lines.

25.2 + 25.3 IMPL final-Task bundles anticipated (settled at 25.2 + 25.3 BRAINSTORM/SPEC):

- **25.2 bundle**: extends `### envoy.filters.http.wasm` with body-stage detail; stat-table +N if envoy-go-strict counters land; additional envoy-go-strict departure records (foreign-function 0-vs-10 default registry per AMEND-A9 + ADR-0206; tick/httpCall/metric counters).
- **25.3 bundle**: extends with per-route + multi-plugin + conformance detail; 5th-canonical REUSE-by-absence caption note (NO §(xvi) amendment per AMEND-A3); ADR-0212 conformance harness pin + 62.5% threshold cross-reference; potentially Group-C `vm_reload*` counters if `failure_policy = FAIL_RELOAD` lands.

---

## 14. Test surface

### 14.1 Layer A: unit tests at `internal/filter/http/wasm/` (25.1 IMPL)

- `wasm_test.go` (~1500-2000 LoC anticipated): filter factory + filter struct + filterStats + `New` table-driven tests.
- `compiled_config_test.go`: 18-arm PARSE-REJECT roster table-driven tests per §6.2.
- `datasource_test.go`: 4-arm DataSource resolution unit tests + WatchedDirectory PARSE-REJECT + Remote PARSE-REJECT + empty-oneof PARSE-REJECT + file-read failure paths.
- `abi_callbacks_test.go`: HTTP-filter ABI callback unit tests for the 13-callback subset + the 24-hostcall capability-gating dispatch.

### 14.2 Layer B: unit tests at `internal/wasm/` (25.1 IMPL)

- `vm_test.go`: per-stream `*VM` construction + `RegisterABICallbacks` + `Run` + `CallProxyOnRequestHeaders` + ... + `Close` table-driven tests; sandbox-config table-driven tests covering each ALLOW/DENY toggle per §4.3.
- `compile_test.go`: `NewCompileCache` + `CompileModule` + cache-hit-on-same-content-hash + cache-miss-on-different-source table-driven tests.
- `sandbox_test.go`: per-capability ALLOW/DENY exhaustive tests; verifies the default-deny posture per AMEND-A5.
- `bytecode_util_test.go`: ABI-version detection unit tests (v0.1.0 + v0.2.0 + v0.2.1 + missing-sentinel cases per AMEND-A6 + §6.2 arm 16).
- `pairs_test.go`: pairs wire format serialization byte-faithful unit tests per R3.
- `wasi_test.go`: WASI shim unit tests per R4 (fd_write → proxy_log; clock_time_get; random_get; environ_*; args_*; proc_exit traps).
- `abi_test.go`: WasmResult/WasmBufferType/WasmHeaderMapType/LogLevel/StreamType/MetricType/ProxyAction/WasiErrno value-faithful encoding tests per AMEND-A7.

### 14.3 Layer C: 34th project-wide fuzzer `FuzzWasmConfigParse` (25.1 IMPL)

`fuzz_test.go` at `internal/filter/http/wasm/`. Corpus seeds covering all 18 PARSE-REJECT arms per §6.2 + valid-config seeds. ~30 corpus seeds total at standard ADR-0018 baseline. Must-never-panic invariant covers wazero compile error path (arm 17 — adversarial wasm bytecode must not crash the parser).

### 14.4 Layer D: differential fixture `0034-http-wasm-headers-bridge` (25.1 IMPL)

Per §8.1. 7 scenarios; full cross-side byte-exact via existing `CompareBytes` per AMEND-A4 + §4.5 D6 guardrails. NEW `BackendKind=HTTPWasm` constant addition at `test/differential/runner_test.go`. NEW `bytecode/` subdirectory layout with vendored pre-built `.wasm` files per Q9.

### 14.5 Layer E: race + concurrency tests (25.1 IMPL)

`controller_test.go` equivalent at `internal/wasm/` + `internal/filter/http/wasm/`. Per-stream `*VM` construction + concurrent-fire-and-forget tests to verify no cross-stream state leak; sandbox-config thread-safety; compile-cache concurrent-read concurrent-add tests (sync.RWMutex discipline analogous to phase-22.1 `internal/lua/` precedent).

### 14.6 Six-gate checklist (per phase-22+24 precedent)

- **Gate A — build**: `go build ./...` clean (incl. `internal/wasm/` + `internal/filter/http/wasm/` + wazero direct go.mod dep).
- **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new suppressions.
- **Gate C — race**: `go test -race ./...` clean (incl. the new packages + per-stream `*VM` construction).
- **Gate D — differential**: 37/37 fixtures GREEN at 25.1 phase-done (0000-0033 pre-existing + 0034 + 0035 new); cross-side byte-exact on `0034`; boot-reject substring on `0035`.
- **Gate E — fuzz**: `FuzzWasmConfigParse` clean at 30s/seed; no panics across the 34 project-wide fuzzers.
- **Gate F — h2spec**: 53/53 PASS at ADR-0051 v1.32.4 pin.

---

## 15. 25.1 IMPL acceptance checklist (per phase-22.1 §16 precedent)

The 25.1 IMPL Task that lands the packages + tests + fixture + ADR landings + STATE.md re-advance MUST satisfy ALL of:

1. NEW `internal/wasm/` package created with the API surface per §4.1 (VM + VMOption + Module + CompileCache + ABICallbacks + ForeignFunctionRegistry + WasmResult/WasmBufferType/WasmHeaderMapType/LogLevel/StreamType/MetricType/ProxyAction/WasiErrno types).
2. NEW `internal/filter/http/wasm/` package created with files per §4.4.
3. wazero `v1.10.1` added as direct go.mod dependency per AMEND-A1.
4. `Wasm.config.vm_config.code` consumed (4-arm AsyncDataSource Local) per §5.4 + §6.2 arms 5-8; Remote + WatchedDirectory PARSE-REJECTed per §6.2 arms 6-7.
5. `PluginConfig.{name, root_id, vm_config, configuration, capability_restriction_config}` consumed per §5.2; `failure_policy = FAIL_RELOAD` + `reload_config` + `fail_open` + `environment_variables` PARSE-REJECTed per §6.2 arms 9, 10, 13.
6. `VmConfig.{vm_id (singleton), runtime ("" or "envoy.wasm.runtime.wazero"), code, configuration}` consumed per §5.3; `runtime` discriminators + `allow_precompiled` + `nack_on_code_cache_miss` + multi-plugin duplicate-vm_id PARSE-REJECTed per §6.2 arms 11, 12, 14, 15.
7. ABI version PARSE-REJECT per §6.2 arm 16 + AMEND-A6 byte-faithful `BytecodeUtil::getAbiVersion` reimplementation at `internal/wasm/bytecode_util.go`.
8. 25.1 hostcall surface = 24 hostcalls (16 `proxy_*` + 8 `wasi_*`) per §4.2; 23 deferred-25.2/25.3 hostcalls registered as stub-Unimplemented per Option B in §4.2.
9. 25.1 callback surface = 13 callbacks per §4.2 (5 module-init/allocator + 6 lifecycle hooks + 2 HTTP hooks).
10. Default-deny capability sandbox per §4.3 + AMEND-A5 + envoy-go-strict departure record at BEHAVIOR_CONTRACT.md (per §13 edit #3).
11. Per-stream `*VM` construction + per-module `*Module` compile cache per §4.2.
12. 18-arm PARSE-REJECT roster per §6.2.
13. 5-counter stat surface per §7 (`wasm.wazero.{created,active}` + `wasm.<plugin>.{executions,hostcall_denied,envoy_go.failures}`) per AMEND-A2; 114 → 119 BEHAVIOR_CONTRACT.md update per §13 edit #2.
14. 4-5 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md per §13 edits #3-#5 (default-deny + ABI v0.1.0/v0.2.0 PARSE-REJECT + Remote PARSE-REJECT + runtime-discriminator PARSE-REJECT + envoy-go-strict counters consolidated).
15. 34th project-wide fuzzer `FuzzWasmConfigParse` at standard ADR-0018 baseline; must-never-panic verified.
16. Differential fixture `0034-http-wasm-headers-bridge` GREEN — 7 scenarios full cross-side byte-exact per §8.1 + §4.5 D6 guardrails; vendored Rust-sourced `.wasm` bytecode under `bytecode/` per Q9 + AMEND-A1 (`proxy-wasm-rust-sdk =0.2.4` + `wasm32-wasip1` target).
17. Differential fixture `0035-http-wasm-boot-reject` GREEN per §8.2 — single-arm boot-reject parity at arm 5 (`vm-config-code-required`).
18. NEW `BackendKind=HTTPWasm` constant added at `test/differential/runner_test.go` per §8.1.3.
19. WASI shim custom 8-stub implementation per R4 + §4.2 (`fd_write` → `proxy_log`; `proc_exit` traps; NOT wazero's built-in WASI imports).
20. pairs wire format byte-faithful reimplementation per R3 at `internal/wasm/pairs.go`.
21. ADR-0202 + ADR-0203 + ADR-0204 §Decision + §Consequences bodies landed in DECISIONS.md per the §Context anchor at this SPEC commit; ADR-0044 in-place edit discipline.
22. STATE.md re-advance to `phase 25.1 IMPL done; awaiting 25.2 SPEC` + ROADMAP row 25.1 flipped `planned → done` per ADR-0106 per-cell IMPL-done annotation.
23. 19 HTTP filters wired (per master tip) → 20 HTTP filters wired post-25.1 (`wasm.New` alphabetical insertion at `cmd/envoy-go/main.go`).
24. wazero-VM-pool benchmark task per R8 + D-P4; if `ns/op > 1_000_000` (1ms), ADR-0205 escape-valve fires.

25.2 + 25.3 IMPL acceptance checklists settle at each sub-phase's own SPEC.

---

**End of phase 25 parent master SPEC.**
