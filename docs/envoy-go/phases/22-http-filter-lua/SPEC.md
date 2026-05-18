# Phase 22 — `envoy.filters.http.lua` (parent master SPEC)

**Phase id:** `22`
**Slug:** `22-http-filter-lua`
**Status:** `in-progress` (SPEC stage; **3-way pre-split LOCKED at BRAINSTORM Q2** — sub-phases `22.1-http-filter-lua-vm-and-headers-bridge` + `22.2-http-filter-lua-full-bridge` + `22.3-http-filter-lua-multi-script-and-per-route`; no SPEC-time split re-evaluation per §3.0)
**Produced by:** `superpowers:brainstorming` SCOPED to SPEC authoring (lifecycle-state 1 → 2 per ADR-0005 §Decision 4; transcribes `docs/envoy-go/phases/22-http-filter-lua/BRAINSTORM.md` into formal SPEC shape, resolves the 7 §10 empirical pins IN-SESSION via parallel-subagent fan-out against reference Envoy v1.37.2 + v1.32.4 go-control-plane proto bindings + `github.com/yuin/gopher-lua` per ADR-0004, anchors the 2 NEW ADR §Context drafts (ADR-0188 + ADR-0189), reserves the ADR-0125 §(xiv) AMENDMENT slot for 22.3 IMPL, and points down to the per-sub-phase SPEC sessions for surface detail per the phase-18.1/18.2/19.1/19.2 precedent)
**Depends on:** phase 21 (`done` at master `473e970` — phase-21 squash-merge; SHA-fill follow-up `e63feab`; phase-22 BRAINSTORM at `615d22a`; BRAINSTORM SHA-fill `cdc8f95` = master tip + this SPEC session's base commit)
**Sub-phases:** `22.1-http-filter-lua-vm-and-headers-bridge`, `22.2-http-filter-lua-full-bridge`, `22.3-http-filter-lua-multi-script-and-per-route`
**Authored:** 2026-05-18
**Differential surface at end of phase:** ROADMAP rows `22.1`, `22.2`, and `22.3` all `done`; the parent row `22` flips `in-progress → done` AT THE SAME phase-done commit as `22.3`'s phase-done (mirroring the 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 + 07 / 07.1 / 07.2 + 08 / 08.1 / 08.2 + 18 / 18.1 / 18.2 + 19 / 19.1 / 19.2 closure pattern, extended to 3-way per Q2). Cumulatively across the three sub-phases: NEW `internal/lua/` framework primitive (ADR-0188); NEW `internal/filter/http/lua/` package (ADR-0189); NEW differential fixtures `0026-http-lua-headers-bridge` (22.1, full cross-side byte-exact for 6 wire-interactive scenarios + scenario (g) substring-match per AMEND-10 + §8.3), `0027-http-lua-full-bridge` (22.2, partial cross-side / REFERENCE-LESS fallback for non-deterministic scenarios), `0028-http-lua-multi-script-and-per-route` (22.3, cross-side byte-exact for deterministic scenarios) are all differentially green; pre-existing fixtures `0000`–`0025` remain green; the h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS; three new fuzzers (`FuzzLuaConfigParse` in 22.1; further fuzzers settled at 22.2 + 22.3 BRAINSTORMs).

---

## 1. Mission summary

Phase 22 lands `envoy.filters.http.lua` — Envoy's canonical HTTP Lua scripting filter, the FIRST §9 family-row whose configuration **delegates per-request behavior to operator-authored interpreted scripts** — as the FIFTEENTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), buffer (13), compressor (14), bandwidth_limit (15), rbac (16), jwt_authn (17), ext_authz (18 with its 18.1+18.2 split), ext_proc (19 with its 19.1+19.2 split), oauth2 (20), and adaptive_concurrency (21). Per the phase-22 BRAINSTORM's 12-question dialogue the MVP envelope is: **Envelope D — full upstream parity** (per Q1: full gopher-lua VM + full Envoy↔Lua bridge surface + named `SourceCodes` map + `LuaPerRoute` 3-arm oneof + ADR-0125 NEW 9th canonical at 22.3 amendment) delivered across THREE sub-phases (per Q2 3-way pre-split at BRAINSTORM time); **Lua VM library = `github.com/yuin/gopher-lua` v1.1.2** (per Q3 + AMEND-2 pin recommendation; pure-Go Lua 5.1; MIT-licensed; NO CGO; matches upstream's LuaJIT 5.1 dialect); **NEW `internal/lua/` framework primitive at 22.1 first-consumer** (per Q4 EXTRACT-NOW — ENDS phase-21 ZERO-NEW-framework-primitive streak; ADR-0188); **4-arm in-package DataSource resolution at 22.1** (per Q5: Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT — deferred to future Runtime/hot-reload phase); **Pragmatic-middle 22.1 bridge surface** (per Q6: `:headers()` + headers-object methods + `:logXxx()` + `:streamInfo()` subset + `request_handle:respond()`); **NEW 9th canonical per-route shape at 22.3** (per Q7: 3-arm hybrid `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`); **3-counter 22.1 stat surface** (per Q8: `errors` + `executions` + `respond_calls` under corrected template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per AMEND-2 + §11.5); **full cross-side byte-exact fixture-0026 for 6 wire-interactive scenarios + substring-match for scenario (g)** (per Q9 + AMEND-10 + §8.3 scope-narrow per option 2); **WEAK HOLD ADR-0044 escape-valve hypothesis** (per Q10: 2 anticipated ADRs ADR-0188 + ADR-0189; 0-1 escape-valve slot consumption at 22.1 IMPL — STRENGTHENED-VARIANT discussion at §1.2).

Phase 22 is also: (i) the FIRST §9 family-row whose configuration **delegates per-request behavior to operator-authored interpreted scripts** — every prior §9 row had a fully-declarative configuration surface (parse → compile → execute against a fixed algorithm); phase 22 introduces a NEW class of filter where the operator-provided proto-config carries arbitrary Turing-complete script source code that executes at every request. (ii) the FIRST §9 row to introduce a **third-party Lua VM dependency** (`github.com/yuin/gopher-lua` v1.1.2 per AMEND-2). (iii) the FIRST §9 row to **pre-split THREE-way at BRAINSTORM time** (prior 2-way splits at phases 05/06/07/08/18/19 all settled at SPEC or PLAN time per ADR-0045 §6 — phase-22 anchors the split at BRAINSTORM-time and the SPEC author does NOT re-decide). (iv) the FIRST §9 row since phase 17 (jwt_authn at `internal/jwks/` + `internal/jwt/`) to introduce a NEW framework primitive of substantial scope (ENDS the phase-21 ZERO-NEW-framework-primitive streak). (v) the FIRST §9 row to amend ADR-0125's canonical-pattern roster since phase 17 jwt_authn (ENDS the FOUR-CONSECUTIVE ADR-0125-skip streak across phases 18/19/20/21; roster grows 8 → 9 at 22.3 IMPL per Q7).

**This parent master SPEC carries the cross-cutting design that applies to ALL THREE sub-phases** — the envelope-D envelope concept, the shared `compiledConfig` shape with phased-PARSE-REJECT, the per-stream gopher-lua VM execution discipline, the `internal/lua/` framework primitive's public API surface, the full §11 empirical-pin block (all 7 §10 pins resolved IN-SESSION this session per ADR-0004 — they span all three sub-phases, so they live here once), the §1.1 empirical-finding-driven scope-revision (amendment) block, and the §9 BEHAVIOR_CONTRACT delta + §13 verbatim Markdown patch anticipation. It points down to each sub-phase's authoritative SPEC for the per-surface detail.

After phase 22, the project has proven its fifteenth §9 engineering claim: *envoy-go's HTTP filter framework hosts a fully-operational Lua-script-driven HTTP filter that compiles operator-supplied Lua source from any of 4 DataSource arms at config-load, dispatches per-stream into a fresh gopher-lua VM (sandboxed default-deny per §4.3 with an envoy-go-strict departure from upstream's bare `luaL_openlibs` posture per AMEND-1), invokes `envoy_on_request`/`envoy_on_response` hooks against bridge methods for headers manipulation + logging + `:streamInfo()` subset access + synchronous `:respond()` short-circuit (full bridge surface at 22.2 IMPL), supports named-script lookup via `Lua.SourceCodes` map + per-route 3-arm oneof override at 22.3 IMPL (NEW 9th canonical per ADR-0125 §(xiv)), publishes 3 counters under the HCM-rooted `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template (per AMEND-2 stat-prefix correction + AMEND-3 `executions` reclassification — 2 upstream-parity + 1 envoy-go-strict), and is OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on every axis except the documented divergence-windows (stdlib-sandbox-strict per AMEND-1; `respond_calls` envoy-go-strict counter per AMEND-3; gopher-lua-vs-LuaJIT `tostring(float)`/`string.format`/`pcall` error-string formatting divergences per AMEND-9 forward-pointer for 22.2; scenario (g) substring-match vs byte-exact per AMEND-10).*

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 7 §11 empirical pins (executed at this SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy + v1.32.4 go-control-plane bindings + gopher-lua source) generated the following **12 amendment-block entries** — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM:

- **AMEND-1 (stdlib sandbox — REFUTES BRAINSTORM §7.2 surface-1):** The BRAINSTORM hypothesized upstream Envoy's LuaJIT sandbox denies `os.*`/`io.*`/`debug.*`/`package.*`. The §11.3 empirical scrape against `source/extensions/filters/common/lua/lua.cc` (v1.37.2) REFUTES: upstream calls `luaL_openlibs(state.get())` — the full LuaJIT stdlib (base, package, io, os, debug, math, string, table) — **without subsequent neutering**. `wrappers.cc` registers per-method bridges but does NOT strip dangerous stdlib. envoy-go's 22.1 strict default-deny sandbox (per §4.3) is therefore an **envoy-go-strict DEPARTURE** (not parity preservation as the BRAINSTORM framed it); requires a BEHAVIOR_CONTRACT.md envoy-go-strict departure record at 22.1 IMPL final Task bundle alongside the §13.6 stat departure. The departure rationale: gopher-lua exposes `os.execute` (subprocess), `os.exit` (terminates host process), `io.popen` (shell-out), `package.*` (filesystem-search loader), `channel` (Go-native chan), `debug.getupvalue`/`setupvalue` (cross-closure tampering) — none of which are upstream-parity arguments and all of which are sandbox-breaking in envoy-go's per-stream goroutine dispatch model. The concrete sandbox roster lands as a table in §4.3.

- **AMEND-2 (stat-prefix template — REFUTES BRAINSTORM §5.2; mirrors phase-21 AMEND-3 C2):** The BRAINSTORM hypothesized template `lua.<config_stat_prefix>.<stat>`. The §11.5 empirical scrape against `source/extensions/filters/http/lua/lua_filter.h:343-347` REFUTES: upstream's `generateStats` constructs `absl::StrCat(prefix, "lua.", filter_stats_prefix)` where `prefix` is the **HCM-injected per-filter `stats_prefix`** (same as every other §9 filter — phase-09/12/14/16/17/18/19/20/21 — per ADR-0143 SN2-reuse convention). True template: **`http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>`** (HCM-rooted). The proto godoc shorthand `lua.<stat_prefix>.errors` at `lua.pb.go:77,81` is upstream's abbreviated form, NOT the literal full path. If `Lua.stat_prefix` is empty, names become `http.<HCM>.lua..errors` (literal consecutive dots — mirrors phase-14 compressor empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243). The 22.1 stat-table in §7 + §13.5 reflects this template correction. AMEND-2 is the **stat-prefix correction with direct phase-21-precedent**: every §9 row has surfaced the same HCM-injection correction at SPEC empirical-pin time.

- **AMEND-3 (`executions` reclassification — REFUTES BRAINSTORM §5.1 + §5.4):** The BRAINSTORM classified `executions` as an envoy-go-strict extension. The §11.5 empirical scrape against `source/extensions/filters/http/lua/lua_filter.h:23-24` REFUTES: the upstream `ALL_LUA_FILTER_STATS(COUNTER) COUNTER(errors) COUNTER(executions)` macro defines **2 upstream-parity counters**. `executions` is incremented at `lua_filter.cc:872` (`stats_.executions_.inc()`) per script invocation. The corrected envoy-go-strict bundle drops from 2 records (BRAINSTORM §5.4) to **1 record** for `respond_calls` only — the third-party-introduced counter that gives operators visibility into `:respond()` short-circuit frequency. The 22.1 stat roster in §7 reflects the reclassification: 2 upstream-parity (`errors` + `executions`) + 1 envoy-go-strict (`respond_calls`).

- **AMEND-4 (PARSE-REJECT roster — CONFIRMS-AND-TIGHTENS BRAINSTORM §9.4):** BRAINSTORM §9.4 anticipated "~12-18 arms" for 22.1. The §11.4 empirical scrape produces **18 arms exactly** (upper bound; tightens the range). The 18-arm roster lives in §6.2; the principal driver of the upper-bound tightening is AMEND-5 (DataSource arm expansion) + AMEND-6 (3 additional baseline arms BRAINSTORM omitted).

- **AMEND-5 (DataSource 10-arm refinement — REFINES BRAINSTORM §2.5):** BRAINSTORM §2.5 framed DataSource handling as "4 arms" (Filename / InlineBytes / InlineString / EnvironmentVariable; each with "PARSE-REJECT if empty/unreadable" collapsed). The §11.4 empirical scrape REFINES to **10 DataSource-related rejection leaves**: 4 oneof-arm cases × 2-3 rejection classes per arm (`Filename` splits into `{name-empty, ENOENT-or-other-OS-error, zero-byte-contents}`; `EnvironmentVariable` splits into `{name-empty, unset, empty-value}`) + `WatchedDirectory` + `empty-oneof` (specifier oneof unset). The framing in §2.5 is correct in shape but undercounts the rejection-arm leaves; the PARSE-REJECT roster in §6.2 enumerates all 10 explicitly.

- **AMEND-6 (3 additional baseline PARSE-REJECT arms — EXTENDS BRAINSTORM §9.4):** The §11.4 empirical scrape adds 3 PARSE-REJECT arms BRAINSTORM §9.4 omitted: (a) `typed-config-required` + `typed-config-unmarshal` (universal-to-every-filter baseline per ADR-0080 byte-stable wording — should be present even though BRAINSTORM didn't list); (b) `default-source-code-required` (post-inline-code-rejection; arm disposition depends on §12-D1 — see §12); (c) `script-missing-required-hooks` (phase-22-specific defensive check — gopher-lua compiles fine on a Lua chunk that defines neither `envoy_on_request` nor `envoy_on_response`, but the resulting filter is operationally a misconfig). Also REFUTES the BRAINSTORM §9.4 mention of "sandbox-violation" PARSE-REJECT — gopher-lua does NOT have a static-AST sandbox-scan; the deny-list discipline is enforced at VM-construction time (don't expose modules) rather than at compile-time PARSE-REJECT; no `sandbox-violation` arm in the roster.

- **AMEND-7 (`:respond()` byte-pin extension — EXTENDS BRAINSTORM §6.2 scenario (d)):** BRAINSTORM §6.2 scenario (d) asserts: *"Client receives 403 + body `denied` + no upstream request."* The §11.6 empirical scrape against `lua_filter.cc:559-583` + `utility.cc:1237-1284` (`prepareLocalReply`) EXTENDS to the full byte-pinned tuple `{status 403; content-length: 6 (auto-set from body length); content-type: text/plain (default reference content-type at utility.cc:1241,1273 because the Lua headers table did NOT supply content-type); body "denied" (6 bytes verbatim, no trailing newline, no JSON wrapping); no upstream request initiated}`. Mirror of phase-21 AMEND-6 precision. The wire-shape pin lands as §8.2.4 specification (and at ADR-0189 §Decision body at 22.1 IMPL).

- **AMEND-8 (`:respond()` from `envoy_on_response` runtime-reject — EXTENDS):** The §11.6 empirical scrape against `lua_filter.cc:1031-1034` shows `EncoderCallbacks::respond` raises `luaL_error(state, "respond not currently supported in the response path")`. envoy-go MUST mirror this with the byte-exact gopher-lua runtime-error string per upstream-parity discipline; the error is caught by the panic-recovery wrapper, increments `lua.errors`, and the script aborts. Also EXTENDS with the `:status` range validation: upstream rejects `:status` values outside `[200, 600)` with the byte-exact error string `":status must be between 200-599"` per `lua_filter.cc:~578-580`. envoy-go mirrors both runtime-rejection strings. (Not a PARSE-REJECT arm — runtime-only.)

- **AMEND-9 (gopher-lua-vs-LuaJIT divergences — EXTENDS BRAINSTORM §6.3 + §7.2 risk #3):** The §11.2 empirical scrape across gopher-lua's `value.go`, `stringlib.go`, `state.go`, and Envoy's `wrappers.cc` confirms three observable-output divergences that the 22.1 fixture-0026 scenarios (a)-(f) intentionally avoid: (a) `tostring(float)` uses Go's `strconv.FormatFloat(f, 'g', -1, 64)` (shortest round-trippable) while LuaJIT uses `LUAI_NUMFMT = "%.14g"` — diverges on values like `0.1 + 0.2`; (b) `string.format("%d", float)` produces `"%!d(float64=...)"` in gopher-lua (Go-fmt mismatch) while LuaJIT silently truncates; (c) `pcall` error-message prefix differs (gopher-lua `"[string \"chunk\"]:line: msg"` vs LuaJIT `"chunk:line: msg"`). EXTENDS BRAINSTORM §7.2 risk #3 with concrete divergence catalogue. RECOMMENDS a `lua.FormatNumber(v) string` helper on `internal/lua/` for 22.2 forward use. RECOMMENDS a BEHAVIOR_CONTRACT.md envoy-go-strict departure record for runtime-error log-message wording at 22.1 IMPL final Task bundle (parallel to the AMEND-1 stdlib-sandbox-strict departure + AMEND-3 `respond_calls` departure — total 3 envoy-go-strict departure records in the 22.1 IMPL bundle). Also CONFIRMS: header-iteration determinism is bridge-snapshot-driven (per §11.2.3 — `HeaderMapWrapper::luaPairs` snapshots into a Go slice walked by integer index), NOT Lua-5.1-spec-guaranteed; BRAINSTORM §2.9's claim that "`__pairs` iteration order in 5.1 is deterministic given the headers-map ordering" REWORDS per §11.2 → "header `__pairs` iteration is deterministic because the bridge `__pairs` metamethod snapshots into a Go slice and walks by integer index; pure-Lua table iteration order is unspecified per Lua-5.1 §2.5.7."

- **AMEND-10 (fixture-0026 scenario (g) scope-narrow — REFUTES-AND-NARROWS BRAINSTORM Q9 + §6.2):** BRAINSTORM Q9 + §6.2 scenario (g) claims "envoy-go subject + reference Envoy both fail to start with **byte-exact error logs**". The §11.7 empirical scrape EMPIRICALLY-REFUTES the byte-exact-stderr level via three independent divergence sources: (i) gopher-lua vs LuaJIT compile-error format differs (`<chunkname>:<line>: <msg>` vs `[string "..."]:<line>: <msg>`); (ii) envoy-go's `log.Fatalf("load config: %v", err)` at `cmd/envoy-go/main.go:66` prepends a timestamp prefix vs Envoy server log format; (iii) Envoy's `"error ``{exception}`` initializing config '{bootstrap_debug} {config_yaml} {config_path}'"` wrap includes the bootstrap proto dump + config YAML which trivially diverge. **NARROWS** the scenario (g) cross-side commitment from "byte-exact error logs" to **substring-match on `"script load error"`** (option 2 per §11.7.6): both sides' stderr MUST contain the literal substring `"script load error"`; envoy-go's bootstrap wraps gopher-lua's error with the same prefix at the boot layer for upstream parity. ~50 LoC IMPL delta at envoy-go bootstrap layer (wording-pinning discipline at `cmd/envoy-go/main.go` boot-reject path); NO new driver interface; NO ADR-0190 consumption. The §8.3 fixture taxonomy + §13.6 BEHAVIOR_CONTRACT.md edit bundle reflect the scope-narrow.

- **AMEND-11 (`BackendKind=HTTPLua` constant + fixture-0026 layout — EXTENDS BRAINSTORM §3.7 + §6):** The §11.7 empirical scrape EXTENDS: fixture-0026 needs a NEW `BackendKind` constant `HTTPLua` (mirrors `HTTPCsrf` / `HTTPCompressor` / `HTTPAdaptiveConcurrency` precedent at `test/differential/runner_test.go:547`) — purely a switch-case addition; ~20 LoC delta. Fixture layout uses a NEW `scripts/` subdirectory (per-scenario `.lua` files) to exploit the DataSource `Filename` arm naturally (vs all-inline-strings collapsed into the YAML) — adds DataSource-arm coverage for free + improves per-scenario readability. Concrete layout in §8.4.

- **AMEND-12 (v1.32.4 vs v1.37.2 binding-gap forward-pointers — EXTENDS):** The §11.1 empirical scrape against v1.32.4 go-control-plane Lua proto binding vs upstream v1.37.2 IDL surfaces two fields present in v1.37.2 but ABSENT from envoy-go's consumed v1.32.4 binding: (a) `Lua.clear_route_cache` (field 5, `google.protobuf.BoolValue`); (b) `LuaPerRoute.filter_context` (field 4, `google.protobuf.Struct` — sibling of, not arm of, the `override` oneof). Both are binding-gaps not behavioral PARSE-REJECTs (the v1.32.4 protobuf-go parser silently drops unknown fields). Forward-pointer disposition per the ADR-0008-style binding-gap convention: parent SPEC §3 catalogs the gap; a future `go-control-plane` upgrade phase activates the field surfaces with their own SPEC. Not blocking for 22.1.

### 1.2 ADR continuity + STRENGTHENED-or-revised D-hypothesis at SPEC commit

Phase 21 closed at ADR-0187. Phase 22 anticipates **2 NEW ADRs at 22.1 IMPL** (ADR-0188 + ADR-0189) + **1 IN-PLACE §Decision AMENDMENT to ADR-0125 at 22.3 IMPL** (the §(xiv) NEW 9th canonical per-route shape; AMENDMENT-anticipation paragraph at §6 of this SPEC). §Context drafts anchor at this SPEC commit; §Decision + §Consequences bodies land at each ADR's Lands-in-Task per ADR-0044. **Next-free ADR after phase 22 SPEC commit stays `ADR-0190`** (2 numbers consumed: ADR-0188..ADR-0189). ADR-0044 escape-valve held in reserve at ADR-0190 for ~0-1 impl-time-unanticipated ADRs at 22.1 IMPL.

**D-hypothesis at SPEC commit:** BRAINSTORM Q10 WEAK-HOLD predicted 0-1 escape-valve consumption at 22.1 IMPL (one-slot consumption from phase-21's STRENGTHENED two-slot buffer at ADR-0188 + ADR-0189). This SPEC's empirical-pin scrape produces **two surfaces that could plausibly consume the escape-valve slot** but neither is anticipated to escalate from the §Decision-body-of-existing-ADR scope:

1. **`*LState`-pool design (§11.3.4 escape-valve candidate)** — if 22.1 IMPL benchmarks of per-stream `*LState` construction cost surface unacceptable overhead (e.g., > 1ms per stream at the headers-only bridge surface), the ADR-0190 slot would anchor a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. Until benchmarked, the WEAK-default is per-stream construction (no pool); the per-stream discipline lands inside ADR-0188 §Decision body without a separate ADR.

2. **Scenario (g) `BootRejectFixture` interface (§11.7 escape-valve candidate originally)** — COLLAPSES per AMEND-10 + the user's option 2 choice (substring-match `"script load error"`). No new driver interface; no ADR-0190 consumption. The wording-pinning discipline lands as a 22.1 IMPL late-task edit at `cmd/envoy-go/main.go` boot-reject path without a separate ADR.

**SPEC-time disposition:** WEAK HOLD STANDS (UNCHANGED from BRAINSTORM Q10). 2 anticipated ADRs (ADR-0188 + ADR-0189) land cleanly; 0-1 escape-valve slot consumption from the `*LState`-pool benchmark surface (the only remaining escape-valve candidate post-§11.7 collapse). ZERO-slot buffer post-22.1-IMPL absorbs the consumption if it fires. The 22.2 BRAINSTORM re-evaluates buffer discipline + anticipates 22.2 IMPL ADR consumption (likely ~2-4 NEW ADRs for full-bridge-API + httpCall + body-buffering interaction with ADR-0128 + dynamic-metadata bridge deferral).

---

## 2. Scope — non-purposes + REUSES-not-consumed

Phase 22 is 3-way-split per Q2 + §3 (no SPEC-time split re-decision). It does NOT extend the framework or any other subsystem beyond the minimum needed to land `envoy.filters.http.lua` envelope D under the existing 07.1 framework + the 1 NEW `internal/lua/` framework primitive (ADR-0188) + the 1 NEW `internal/filter/http/lua/` package (ADR-0189) + the 1 IN-PLACE AMENDMENT to ADR-0125 at 22.3 (NEW 9th canonical §(xiv)).

- **2.1 `WatchedDirectory` DataSource arm OUT OF SCOPE + PARSE-REJECT** (per Q5 + AMEND-5). The hot-reload-of-scripts surface is deferred to a future Runtime / RTDS / hot-reload family phase. Mirrors phase-21 ADR-0187's `enabled.RuntimeFeatureFlag` PARSE-REJECT pattern + forward-pointer.

- **2.2 `Lua.InlineCode` deprecated field OUT OF SCOPE + PARSE-REJECT** (per envoy-go-strict deprecated-field-rejection discipline + AMEND-6). The field exists in v1.32.4 + v1.37.x proto bindings but is marked `[deprecated = true, (envoy.annotations.deprecated_at_minor_version) = "3.0"]` in the proto IDL. Upstream Envoy honors with a deprecation warn-log; envoy-go is stricter — PARSE-REJECT mirrors phase-20 §20.P6 deprecated-`ConfigSource.path`-field-1 PARSE-REJECT precedent.

- **2.3 Lua 5.2 / 5.3 / 5.4 dialect features OUT OF SCOPE.** gopher-lua is Lua 5.1 (matches upstream LuaJIT 5.1 dialect); scripts using 5.2+ features (`goto`, integer subtype, `bit32` / `utf8` stdlib) get Lua-compile-time errors at script-source compilation. Documented in `internal/lua/doc.go` + `internal/filter/http/lua/doc.go` at 22.1 IMPL.

- **2.4 Dynamic-metadata bridge surfaces OUT OF SCOPE at 22.1.** `:metadata()` + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()` deferred to 22.2 SPEC; likely PARSE-REJECT or partial-deferral per the project's cross-phase dynamic-metadata-deferral discipline (deferred at phases 16/17/18/19/20).

- **2.5 Body-access bridge methods OUT OF SCOPE at 22.1.** `:body()` + `:bodyChunks()` + `:trailers()` deferred to 22.2 (interact with phase-13 ADR-0128 decode-side body-buffering primitive). 22.1 scripts that call these methods get a Lua runtime error per Q6 Lua-idiomatic-error discipline (scripts can `pcall` to handle gracefully).

- **2.6 `:httpCall()` outbound HTTP call surface OUT OF SCOPE at 22.1.** Deferred to 22.2; reuses phase-20 `internal/httpclient/` framework primitive at first co-consumer (validates the phase-20 extraction).

- **2.7 `:connection()` SSL/TLS access OUT OF SCOPE at 22.1.** Deferred to 22.2; integrates with phase-03 TLS primitives.

- **2.8 Crypto / base64 / sha helpers OUT OF SCOPE at 22.1.** `:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()` deferred to 22.2; likely thin wrappers over Go's `crypto/*` + `encoding/base64`.

- **2.9 `:fileBytes()` file-read helper OUT OF SCOPE at 22.1.** Deferred to 22.2; security caveats apply (sandbox concern; 22.2 SPEC settles the security discipline).

- **2.10 `:timestamp()` time helper OUT OF SCOPE at 22.1.** Deferred to 22.2; non-deterministic, complicates cross-side byte-comparison for scenarios that include timestamps.

- **2.11 Full `:streamInfo()` surface OUT OF SCOPE at 22.1** (per Q6 pragmatic-middle). `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata()` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()` deferred to 22.2.

- **2.12 `Lua.SourceCodes` named-script map OUT OF SCOPE at 22.1+22.2; consumed at 22.3.** PARSE-REJECT at 22.1+22.2 per AMEND-4 + §6.2 arm 4.

- **2.13 `LuaPerRoute` OUT OF SCOPE at 22.1+22.2; consumed at 22.3.** PARSE-REJECT at 22.1+22.2 per AMEND-4 + §6.2 arm 18. The NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) AMENDMENT body land at 22.3 IMPL.

- **2.14 Async script execution coroutines NEVER-DEFERRED for 22.1.** Upstream Envoy's Lua filter supports per-script Lua coroutines for yielding during `:httpCall()` and `:body()` await. envoy-go's 22.2 implementation may or may not adopt coroutines (22.2 SPEC settles); alternative is goroutine-resume-on-completion (mirrors phase-09 fault async primitive + phase-18.2 ext_authz_grpc async-resume primitive). The `coroutine` stdlib library IS exposed in 22.1's sandbox (per AMEND-A4 + §4.3 — matches upstream's `luaL_openlibs` opening it) but no 22.1 bridge methods consume coroutines.

- **2.15 `Lua.clear_route_cache` (v1.37.2 field 5) NEVER-DEFERRED at 22.1.** Per AMEND-12 v1.32.4 binding-gap forward-pointer. The field is ABSENT from envoy-go's consumed v1.32.4 binding; activates when go-control-plane bumps to v1.37.x.

- **2.16 `LuaPerRoute.filter_context` (v1.37.2 field 4) NEVER-DEFERRED at 22.3.** Per AMEND-12 v1.32.4 binding-gap forward-pointer.

- **2.17 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed (no TLS-principal interaction at 22.1; 22.2's `:connection()` may consume). ADR-0150 jwks NOT consumed. ADR-0151 jwt verifier NOT consumed. ADR-0177 `internal/httpclient/` NOT consumed at 22.1 (22.2's `:httpCall()` consumes — first co-consumer validates the phase-20 extraction). ADR-0178 `internal/sdsfile/` NOT consumed. ADR-0158 `internal/grpcclient/` NOT consumed. ADR-0165 + ADR-0174 `DecoderFilterCallbacks` + `EncoderFilterCallbacks` cross-phase-reusable extensions NOT consumed (no TLS/principal-attribute envelope to populate; no body-buffering primitive needed at 22.1). ADR-0186 `Clock` seam NOT consumed (no timer-driven path in 22.1).

- **2.18 MVP confirmations (positive consumption assertions for 22.1).** `Lua.DefaultSourceCode` IN MVP (4-arm DataSource resolution per §6.2 arms 6-15 + Q5). `Lua.StatPrefix` IN MVP (qualifies the 3-counter stat namespace per AMEND-2 + §7). 22.1 bridge: `envoy_on_request` + `envoy_on_response` hooks IN MVP; `:headers()` + 7 headers-object methods IN MVP per Q6; 6 `:logXxx()` methods IN MVP; `:streamInfo()` 4-method subset IN MVP per Q6; `request_handle:respond(headers, body)` IN MVP with byte-pin per AMEND-7. Sandbox config IN MVP (default-deny per §4.3 + AMEND-1). 3-counter stat surface IN MVP per §7.

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q2; no SPEC-time re-decision per §1.4

Per BRAINSTORM Q2 + §1.4 the 3-way pre-split at BRAINSTORM time is the LOCKED disposition. The §11 empirical-pin scrape produced no structural reason to revisit the split (no surface that would re-collapse to single-row; no scope-creep that would warrant a 4-way split). LoC envelope re-estimated post-empirical-scrape at **~3500-5000 production across all 3 sub-phases** — the BRAINSTORM §1.4 estimate of ~3500-5000 STANDS unchanged. Per-sub-phase task counts estimated at 14-18 per sub-phase × 3 = 42-54 tasks total. Each sub-phase fits cleanly under the ADR-0045 split-gate (~25 tasks, ~1500 LoC) — no further split needed at any sub-phase level.

The SPEC author's call: **3-way split LOCKED at BRAINSTORM Q2 STANDS at SPEC commit**. No ADR-0045 §6 re-application; no NEW ADR for the split disposition (BRAINSTORM Q2 + ADR-0106 ROADMAP row registrations already cover the discipline). This mirrors the parent-row precedent set at phase-18 SPEC §3 + phase-19 SPEC §3 — the SPEC author confirms the BRAINSTORM split.

### 3.1 Split surface-mapping table (per phase-18+19 §11.1 precedent)

| Field / surface | 22.1 disposition | 22.2 disposition | 22.3 disposition |
|---|---|---|---|
| `Lua.DefaultSourceCode` (4-arm DataSource) | **CONSUMED** (4 arms; WatchedDirectory PARSE-REJECT) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `Lua.SourceCodes` map | **PARSE-REJECT** (deferred-to-22.3 wording) | PARSE-REJECT (unchanged) | **CONSUMED** (named-script lookup) |
| `Lua.InlineCode` (deprecated) | **PARSE-REJECT** (envoy-go-strict deprecated-field-rejection) | PARSE-REJECT (unchanged) | PARSE-REJECT (unchanged) |
| `Lua.StatPrefix` | **CONSUMED** (qualifies stat namespace) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `Lua.clear_route_cache` (v1.37.2 field 5; v1.32.4 binding-gap per AMEND-12) | binding-gap forward-pointer | binding-gap forward-pointer | binding-gap forward-pointer |
| `envoy_on_request` hook | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `envoy_on_response` hook | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `request_handle:headers()` + 7 methods (`:get` / `:getAtIndex` / `:getNumValues` / `:add` / `:append` / `:remove` / `:replace`) + `__pairs` metamethod | **CONSUMED** | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `:logTrace/:logDebug/:logInfo/:logWarn/:logErr/:logCritical` | **CONSUMED** (6 log methods) | CONSUMED (unchanged) | CONSUMED (unchanged) |
| `:streamInfo()` subset (`:protocol` / `:routeName` / `:downstreamLocalAddress` / `:downstreamDirectRemoteAddress`) | **CONSUMED** (4-method subset) | full `:streamInfo()` (deferred methods) | unchanged from 22.2 |
| `request_handle:respond(headers, body)` | **CONSUMED** (request-path only per AMEND-8) | unchanged | unchanged |
| `:body()` + `:bodyChunks()` | runtime-error (Lua-idiomatic) | **CONSUMED** (interacts with ADR-0128) | unchanged from 22.2 |
| `:trailers()` | runtime-error | **CONSUMED** | unchanged from 22.2 |
| `:metadata()` (dynamic-metadata bridge) | runtime-error | likely PARSE-REJECT or partial-deferral (22.2 SPEC settles) | unchanged from 22.2 |
| `:connection()` | runtime-error | **CONSUMED** | unchanged from 22.2 |
| `:httpCall()` (outbound HTTP — reuses `internal/httpclient/`) | runtime-error | **CONSUMED** | unchanged from 22.2 |
| Crypto / base64 / sha helpers | runtime-error | **CONSUMED** | unchanged from 22.2 |
| `:fileBytes()` | runtime-error | **CONSUMED** | unchanged from 22.2 |
| `:timestamp()` | runtime-error | **CONSUMED** | unchanged from 22.2 |
| `LuaPerRoute` (3-arm oneof) | **PARSE-REJECT** (deferred-to-22.3 wording) | PARSE-REJECT (unchanged) | **CONSUMED** (NEW 9th canonical per ADR-0125 §(xiv)) |
| `LuaPerRoute.filter_context` (v1.37.2 field 4; v1.32.4 binding-gap per AMEND-12) | binding-gap forward-pointer | binding-gap forward-pointer | binding-gap forward-pointer |
| Stat surface | **3 counters** (`errors` + `executions` + `respond_calls` per AMEND-3) | likely +2 httpCall counters (22.2 SPEC settles) | 0 net new (SHARED-vacuous per 9th canonical) |
| Differential fixture | **`0026-http-lua-headers-bridge`** (6 wire-interactive cross-side + scenario (g) substring per AMEND-10) | `0027-http-lua-full-bridge` (partial cross-side / REFERENCE-LESS fallback) | `0028-http-lua-multi-script-and-per-route` (cross-side byte-exact for deterministic) |
| Fuzzer | **`FuzzLuaConfigParse`** (28th — count verification per AMEND-A pending §11.4) | 22.2 fuzzer(s) settled at 22.2 BRAINSTORM | 22.3 fuzzer(s) settled at 22.3 BRAINSTORM |
| ADR anchors | **ADR-0188 §Decision + §Consequences body lands; ADR-0189 §Decision + §Consequences body lands** | settled at 22.2 BRAINSTORM (anticipated ~2-4 NEW ADRs) | NEW 9th canonical ADR + ADR-0125 §(xiv) AMENDMENT body land |
| BEHAVIOR_CONTRACT.md edit bundle | **7-edit bundle at 22.1 IMPL final Task per ADR-0052** (§13) | extends `### envoy.filters.http.lua` subsection with body-stage detail; stat-table +2 if httpCall counters land | extends with multi-script + per-route detail; ADR-0125 §(xiv) cross-reference |

### 3.2 Per-sub-phase scope detail

The authoritative scope detail lives in each sub-phase's SPEC, authored at the sub-phase's own dedicated session per the phase-18.1 / 18.2 / 19.1 / 19.2 precedent:

- `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/SPEC.md` — drafted at the dedicated 22.1 SPEC session (next-skill `superpowers:brainstorming` per phase-18.1/19.1 precedent). The per-sub-phase BRAINSTORM may not be needed (the parent BRAINSTORM settled enough decisions); the 22.1 SPEC session may proceed directly to SPEC authoring. Anticipated artefacts: SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md across 3-4 sessions.
- `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/SPEC.md` — drafted at the dedicated 22.2 SPEC session (next-session-after-22.1-IMPL-phase-done). 22.2 BRAINSTORM expected (more design decisions surface than 22.1 — full bridge API + body-buffering interaction with ADR-0128 + httpCall reuse of phase-20 `internal/httpclient/` + dynamic-metadata-bridge disposition).
- `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/SPEC.md` — drafted at the dedicated 22.3 SPEC session (next-session-after-22.2-IMPL-phase-done). 22.3 BRAINSTORM expected (per-route 3-arm oneof design + ADR-0125 §(xiv) AMENDMENT body authoring).

The 22.1 SPEC inherits this parent SPEC's §6 PARSE-REJECT roster + §7 stat surface + §8 fixture-0026 disposition + §11 empirical-pin block + §12 D-questions + §13 RATIFIED-PENDING-IMPL items + §13 BEHAVIOR_CONTRACT.md edit bundle. The per-sub-phase SPECs reference back to this parent SPEC for the cross-cutting decisions and detail only the sub-phase-specific extensions.

---

## 4. Framework primitive — NEW `internal/lua/` + NEW `internal/filter/http/lua/` (per Q4 EXTRACT-NOW + §11.3)

Per BRAINSTORM Q4 + §3.1 + §11.3 + AMEND-1 + AMEND-A2. Phase 22.1 introduces ONE NEW `internal/lua/` framework primitive at first-consumer (ENDS the phase-21 ZERO-NEW-framework-primitive streak; FIRST §9 row since phase 17 jwt_authn to introduce a NEW framework primitive of substantial scope) + ONE NEW `internal/filter/http/lua/` package. Phase 22.3 anchors the NEW 9th canonical per-route shape ADR + the ADR-0125 §(xiv) IN-PLACE AMENDMENT (roster 8 → 9). No framework deltas at 22.2 beyond consuming the existing `internal/httpclient/` (phase-20 ADR-0177) at first co-consumer for `:httpCall()`.

### 4.1 NEW `internal/lua/` framework primitive (ADR-0188; lands at 22.1 IMPL)

Package boundary: `internal/lua/` hosts the GENERIC VM lifecycle (init, sandbox config, panic-recovery, per-stream execution context, script-compilation cache, bridge-registration interface); `internal/filter/http/lua/` hosts the HTTP-FILTER-SPECIFIC bridge methods (`request_handle:headers()`, `response_handle:headers()`, etc.). The bridge-registration interface is the seam between the two: consumers register their Go callbacks as Lua-callable functions via the primitive's API; the primitive doesn't know about HTTP-specific concepts.

The primitive's API shape (provisional; settled at 22.1 SPEC; this parent SPEC anchors the shape per AMEND-A2 + §11.3.3 + §11.3.5):

```go
// VM is a per-stream gopher-lua execution context. NOT goroutine-safe.
type VM struct { /* unexported; *lua.LState + sandbox config + bridge registry */ }

// VMOption configures VM construction.
type VMOption interface{ /* unexported sealed */ }

// SandboxConfig governs which stdlib modules are exposed to scripts.
// Zero value = StrictUpstreamParity (DENY io/os/debug/package/channel) per AMEND-1.
type SandboxConfig struct {
    AllowBaseFull       bool        // exposes dofile/loadfile/loadstring/load/require/module/print
    AllowIO             bool
    AllowOS             bool        // os.execute/exit/remove/rename — never recommended
    AllowOSTimeHelpers  bool        // os.time/date/clock/difftime only (upstream-parity arm)
    AllowDebug          bool
    AllowPackage        bool
    AllowChannel        bool        // gopher-lua-specific; no upstream-parity argument
    AllowCoroutine      bool        // default-true per AMEND-A4 (matches upstream)
    BasePrintSink       io.Writer   // redirects base.print writes; nil = drop (no stdout leak)
}

func WithSandboxConfig(sb SandboxConfig) VMOption
func WithPanicRecovery(rcvr PanicRecoveryFn) VMOption
func WithStderrSink(sink io.Writer) VMOption

func NewVM(opts ...VMOption) *VM

// Chunk wraps a compiled *FunctionProto safe for cross-VM reuse.
type Chunk struct { /* unexported */ }

// CompileCache is a per-compiledConfig content-addressed compile cache,
// keyed by sha256(source bytes). Cache is owned by the *compiledConfig
// (filter-config-instance scope; GC-driven eviction).
type CompileCache struct { /* unexported */ }

func NewCompileCache() *CompileCache
func CompileScript(src []byte, cache *CompileCache) (*Chunk, error)

// HookFn is a hook the VM exposes to scripts (envoy_on_request / envoy_on_response).
type HookFn func(*VM) error

// RegisterBridgeMethod registers a Go function as a Lua-callable bridge method
// available under the named global (typically the request/response handle userdata).
func (vm *VM) RegisterBridgeMethod(name string, fn lua.LGFunction)

// Run loads the chunk's *FunctionProto onto this VM's *LState (cheap; no
// recompilation per stream) and calls it with the named hooks defined.
func (vm *VM) Run(chunk *Chunk, hooks ...HookFn) error

// Close releases the VM's *LState. Idempotent.
func (vm *VM) Close()
```

**API-REVISION ALLOWANCE clause per Q4** (anchored at ADR-0188 §Decision body at 22.1 IMPL Lands-in-Task): the primitive's API shape is provisional at consumer #1 (HTTP filter Lua); the second consumer (cluster specifier Lua at `envoy.router.cluster_specifiers.lua`, access logger Lua at `envoy.access_loggers.lua`, string matcher Lua at `envoy.string_matcher.lua` — whichever materializes first) may require API revision after empirical validation. ADR-0188's §Decision body carries an EXPLICIT API-REVISION ALLOWANCE clause referencing the future consumer-#2 phase as the validation event. Mirrors phase-20 ADR-0159 §Future Work CLOSURE-AT-PHASE-20 pattern but anticipates future revision rather than closing prior revision.

### 4.2 Per-stream `*LState` construction + per-script compile cache discipline (per §11.3.4 + §11.3.5)

Per §11.3.4 empirical evaluation against gopher-lua source + the canonical author-recommended `sync.Pool` pattern from gopher-lua README. **22.1 WEAK-default: per-stream `*LState` construction with shared per-script-source `*Chunk` cache.** Each per-stream invocation:

1. The filter's `*compiledConfig` carries the pre-compiled `*Chunk` (compiled once at config-load via `CompileScript(src, cfg.compileCache)`).
2. At `DecodeHeaders` entry, the filter calls `vm := lua.NewVM(opts...)`, constructing a fresh `*lua.LState` with the sandbox roster applied (cheap loading of the configured stdlib + nil-ing of denied functions per AMEND-1 §11.3.3).
3. The filter calls `vm.Run(cfg.chunk, hooks...)` which loads the `*FunctionProto` from `Chunk` onto the new `*LState` (cheap — chunks are bytecode-only, no LState-specific state) and calls the script's `envoy_on_request`/`envoy_on_response` hook.
4. At `OnDestroy` (or end-of-stream), the filter calls `vm.Close()` which calls `*lua.LState.Close()` and releases the registry/call-stack memory.

**`*LState`-pool design (ESCAPE-VALVE-CANDIDATE per §11.3.4 + §1.2):** if 22.1 IMPL benchmarks of per-stream construction cost surface unacceptable overhead (e.g., > 1ms per stream at the headers-only bridge surface), the ADR-0190 escape-valve slot anchors a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. Until benchmarked, hold per-stream construction as the WEAK-default. The pool design (per-script-source pool keying, chunk pre-loading, lifecycle management vs `sync.Pool`'s GC-driven eviction) is non-trivial enough to merit its own ADR if triggered.

### 4.3 Sandbox roster — default-deny per AMEND-1 + envoy-go-strict departure record at IMPL

Per §11.3.1 empirical scrape against gopher-lua + §11.3.2 vs upstream Envoy `luaL_openlibs` posture + AMEND-1 REFUTATION. The 22.1 `SandboxConfig` zero-value posture is STRICT-DEFAULT-DENY (sandbox-strict). The roster:

| gopher-lua module | Exposed by `OpenLibs()` | envoy-go 22.1 disposition | Rationale |
|---|---|---|---|
| `base` | yes (27 funcs) | **ALLOW core; DENY** `dofile`, `loadfile`, `loadstring`, `load`, `module`, `require`, `collectgarbage`, `getfenv`, `setfenv`; **REDIRECT** `print` (BasePrintSink; default = drop / no-stdout-leak) | sandbox-breaking arbitrary-code-load + cross-closure tampering |
| `package` | yes | **DENY wholesale** (do not call `OpenPackage`) | filesystem-search loader `loLoaderLua` reads `.lua` from disk |
| `table` | yes | **ALLOW** | safe |
| `io` | yes | **DENY wholesale** | `io.open`/`io.popen` are unsandboxed file/subprocess access |
| `os` | yes | **DENY** `execute`, `exit`, `remove`, `rename`, `tmpname`, `setlocale`, `getenv`; **ALLOW** `os.time`, `os.date`, `os.clock`, `os.difftime` (read-only time helpers) | subprocess + host-Go-process-exit hazards; preserve upstream-parity time arm |
| `string` | yes | **ALLOW** | safe |
| `math` | yes | **ALLOW** | safe |
| `debug` | yes | **DENY wholesale** | `getupvalue`/`setupvalue`/`setfenv` allow cross-closure tampering — sandbox-breaking. EXCEPTION: re-expose `debug.traceback` as an INTERNAL-only global for the panic-recovery wrapper's use (NOT in the script's namespace). |
| `channel` (gopher-lua-specific) | yes | **DENY wholesale** | No LuaJIT counterpart; exposes Go-native `chan` to Lua scripts — no upstream-parity argument |
| `coroutine` | yes | **ALLOW** per AMEND-A4 | Matches upstream `luaL_openlibs` opening it; 22.2's `:body()`/`:httpCall()` may consume internally |

Implementation: NOT `OpenLibs()` (which opens everything indiscriminately). Instead call the per-lib `OpenXxx` selectively, then for `AllowBaseFull == false`, walk the base-globals table and `LNil` out the denied function names. For `AllowOSTimeHelpers && !AllowOS`, call `OpenOs` and then nil out the disallowed `os.execute`/`exit`/`remove`/`rename`/`getenv`/`setlocale`/`tmpname` entries on the resulting module table.

**envoy-go-strict departure record at IMPL** (BEHAVIOR_CONTRACT.md final-Task 7-edit bundle per §13.6): the stdlib-sandbox-strict posture is documented as an envoy-go-strict departure from upstream's bare-`luaL_openlibs`-no-neutering posture. Three departure records total in the 22.1 IMPL bundle: (i) **stdlib-sandbox-strict** (this one per AMEND-1); (ii) **`respond_calls` envoy-go-strict counter** (per AMEND-3 + §7); (iii) **runtime-error log-message wording** (gopher-lua's `[string "chunk"]:line: msg` format vs LuaJIT's `chunk:line: msg`; per AMEND-9 RECOMMEND-DEPARTURE-RECORD).

### 4.4 NEW `internal/filter/http/lua/` package (ADR-0189; lands at 22.1 IMPL)

Package boundary: `internal/filter/http/lua/` hosts the HTTP-FILTER-SPECIFIC parse + bridge methods + filter callbacks. Per §11.4.2 empirical scrape vs prior-phase package shape. The package files (provisional; settled at 22.1 SPEC):

```
internal/filter/http/lua/
  doc.go               # package overview + Q1-Q12 BRAINSTORM decision summary + AMEND-1..AMEND-12 cross-references
  lua.go               # filter struct + factory (HTTPFilterFactory) + filterStats
  compiled_config.go   # config parse + 4-arm DataSource resolution + 18-arm PARSE-REJECT roster + script-compile cache key generation
  datasource.go        # DataSource arm resolution (Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT)
  bridge.go            # HTTP-filter-specific bridge methods (headers + log + streamInfo subset + respond)
  decode_headers.go    # envoy_on_request hook firing
  encode_headers.go    # envoy_on_response hook firing
  stats.go             # 3-counter stat surface
  lua_test.go          # unit tests (~1500-2000 LoC anticipated)
  compiled_config_test.go  # PARSE-REJECT roster table-driven tests (18 cases per §6.2)
  datasource_test.go   # DataSource resolution unit tests
  bridge_test.go       # bridge-method unit tests
  fuzz_test.go         # 28th project-wide fuzzer FuzzLuaConfigParse (count subject to §11.4 AMEND-D verification)
```

Boot-registration insertion at `cmd/envoy-go/main.go`: alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 stylistic discipline. 16 HTTP filters wired pre-phase-22.1; **17 post-phase-22.1**. The Go-package identifier is `lua` (single token; matches `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac` precedent — no underscore needed).

### 4.5 ADR-0125 IN-PLACE AMENDMENT-anticipation (per Q7 + §1.2 + lands at 22.3 IMPL)

Per Q7 + §1.2. ADR-0125's canonical-pattern roster grows from 8 → 9 at phase 22.3 IMPL via in-place §Decision §(xiv) AMENDMENT (mirrors the phase-13 + phase-14 + phase-15 + phase-16 + phase-17 in-place-amend-at-IMPL precedents at ADR-0125 §(viii)-(xiii)). The AMENDMENT body adds the 9th canonical's specification (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`) + the SHARED stat-discipline + the structural-distinctness rationale (combines the 5th canonical's `disabled-bool` + the 8th canonical's `string-reference-delegation` + a novel `DataSource-wholesale-override` arm in a single oneof; structurally distinct from all 8 prior canonicals).

**AMENDMENT-anticipation paragraph at this SPEC commit** (anchored at ADR-0125 in DECISIONS.md via the SPEC's commit to DECISIONS.md):

> **Amendment (per phase 22 parent SPEC §4.5; AMENDMENT body lands at phase 22.3 IMPL final Task per ADR-0044 in-place edit discipline)**. Phase 22.3 will be the FIRST row to use the NEW 9th canonical per-route pattern: a wrapper proto (`LuaPerRoute`) with a REQUIRED oneof `override` containing three arms — `disabled` (bool; PGV `const: true`; mirrors 5th canonical disable-bool); `name` (string; PGV `min_len: 1`; string-reference-delegation into the listener-level `Lua.SourceCodes` map; mirrors 8th canonical string-reference); `source_code` (`*core.DataSource`; wholesale-override using a `DataSource` type rather than a parent-config sub-message; novel — no prior canonical uses `DataSource`-typed wholesale-override). The structural distinctness vs all 8 prior canonicals: combines `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override` in a single oneof — no prior canonical has all three. The 9th canonical's stat-discipline is SHARED (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors` stat namespace; matches phase-17 jwt_authn 8th canonical SHARED per ADR-0154 + the SHARED-stats discipline applies because `LuaPerRoute` has no separate `stat_prefix` field). The AMENDMENT body — including the full §(xiv) clause, the updated 9-shape table, and the per-row first-use citation — lands at phase 22.3 IMPL final Task per ADR-0044.

---

## 5. Proto-field roster (per §11.1)

Per §11.1 empirical scrape against `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/http/lua/v3/lua.pb.go` + `lua.pb.validate.go` + upstream v1.37.2 IDL.

### 5.1 `Lua` message field roster (4 fields)

| # | Field name | Proto type | Go type | PGV | Dep? | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `inline_code` | string | string | none | **YES** (`[deprecated = true]` at pb.go:43; `(envoy.annotations.deprecated_at_minor_version) = "3.0"`) | 22.1 PARSE-REJECT (envoy-go-strict deprecated-field rejection per AMEND-6) |
| 2 | `source_codes` | `map<string, DataSource>` | `map[string]*v3.DataSource` | per-entry embedded DataSource validation; no map-key/size rules | no | 22.1 PARSE-REJECT (deferred-to-22.3 per AMEND-4 arm 4); 22.3 CONSUMED |
| 3 | `default_source_code` | `DataSource` | `*v3.DataSource` | embedded-message recursive validation only | no | 22.1 CONSUMED (4-arm + WatchedDirectory PARSE-REJECT per §5.3) |
| 4 | `stat_prefix` | string | string | none | no | 22.1 CONSUMED (qualifies `<config_stat_prefix>` slot per §7 + AMEND-2) |

### 5.2 `LuaPerRoute` message field roster (3-arm oneof)

| # | Field name | Proto type | Go type | PGV | Dep? | Sub-phase mapping |
|---|---|---|---|---|---|---|
| 1 | `disabled` (oneof `override`) | bool | bool | `(validate.rules).bool = {const: true}` (must equal `true`) | no | 22.1+22.2 PARSE-REJECT (entire message); 22.3 CONSUMED |
| 2 | `name` (oneof `override`) | string | string | `(validate.rules).string = {min_len: 1}` | no | 22.1+22.2 PARSE-REJECT; 22.3 CONSUMED |
| 3 | `source_code` (oneof `override`) | `DataSource` | `*v3.DataSource` | embedded recursive validation only | no | 22.1+22.2 PARSE-REJECT; 22.3 CONSUMED |
| — | `override` (oneof itself) | oneof wrapper | n/a | **`(validate.required) = true`** — exactly one arm required (validate.go:333-342) | n/a | 22.3 — PGV-required at consume time |

### 5.3 `DataSource` arm roster (`envoy.config.core.v3.DataSource`)

| # | Arm / Field | Proto type | PGV | Disposition |
|---|---|---|---|---|
| 1 | `filename` (oneof `specifier`) | string | `(validate.rules).string = {min_len: 1}` | 22.1 CONSUMED |
| 2 | `inline_bytes` (oneof `specifier`) | bytes | none | 22.1 CONSUMED |
| 3 | `inline_string` (oneof `specifier`) | string | none | 22.1 CONSUMED |
| 4 | `environment_variable` (oneof `specifier`) | string | `(validate.rules).string = {min_len: 1}` | 22.1 CONSUMED |
| 5 | `watched_directory` (NOT in oneof; sibling field 5) | `WatchedDirectory` | none | 22.1 PARSE-REJECT (per Q5 + §6.2 arm 7 + AMEND-5) |
| — | `specifier` oneof itself | oneof | `option (validate.required) = true` | 22.1 PARSE-REJECT empty-oneof case (§6.2 arm 6 + AMEND-5) |

Note: `watched_directory` is a sibling field, NOT an arm of the `specifier` oneof. Upstream docstring: *"This field only makes sense when the ``filename`` field is set."* envoy-go's presence-check at parse time (PARSE-REJECT on non-zero `watched_directory`) is the correct enforcement shape per Q5.

### 5.4 v1.32.4 vs v1.37.2 binding-gap forward-pointers (per AMEND-12)

Two fields present in upstream v1.37.2 IDL but ABSENT from envoy-go's consumed v1.32.4 binding:

- **`Lua.clear_route_cache` (field 5; type `google.protobuf.BoolValue`)** — Forward-pointer binding-gap. The v1.32.4 protobuf-go parser silently drops the unknown field; no envoy-go-side PARSE-REJECT applies. Activates when go-control-plane bumps to v1.37.x (future binding-bump phase).
- **`LuaPerRoute.filter_context` (field 4; type `google.protobuf.Struct`; sibling of `override` oneof — NOT an arm of)** — Forward-pointer binding-gap. Same disposition. Documented in §11.1.4.

---

## 6. PARSE-REJECT roster (per §11.4 — 18 arms at 22.1; 20+ forward-pointer arms at 22.3)

Per §11.4 empirical scrape + AMEND-4 (CONFIRMS-AND-TIGHTENS to 18 arms exactly) + AMEND-5 (10 DataSource-related arms) + AMEND-6 (3 additional baseline arms; sandbox-violation arm REMOVED). Byte-stable wording per ADR-0080 + the prior-phase precedents (phase-21 SPEC §5 + phase-20 SPEC §5).

### 6.1 Wording discipline + arm-name convention

Per phase-21 SPEC §5 precedent. Format: `"lua: <field_path>: <reason> [; <forward-pointer hint>]"`. Filter-proto-name prefix `lua:` invariant on every arm. Constants live as package-private `parseReject*` consts at `internal/filter/http/lua/compiled_config.go`, returned via `errors.New(parseReject...)` for byte-stability. Kebab-case arm identifiers (used for SPEC cross-reference + test-name suffixes like `TestBuildCompiledConfig_PARSE_REJECT_inline_code_deprecated`) follow `<field-path-with-dots-as-dashes>-<rejection-class>` per phase-21 + phase-20 + phase-19 + phase-18 precedent.

### 6.2 22.1 PARSE-REJECT roster (18 arms exactly per AMEND-4)

PGV-baseline note (per §11.4 + lua.pb.validate.go): the `Lua` message has **NO PGV rules** on InlineCode / SourceCodes / DefaultSourceCode / StatPrefix — every 22.1 arm below is envoy-go-strict-as-defensive-mirror (not PGV-mirror).

| # | arm-name (kebab-case) | trigger condition | byte-exact error wording |
|---|---|---|---|
| 1 | `typed-config-required` | `typedConfig == nil` at factory entry | `"lua: typed_config required"` |
| 2 | `typed-config-unmarshal` | `anypb.UnmarshalTo` fails into `*Lua` | `"lua: typed_config unmarshal: %w"` |
| 3 | `inline-code-deprecated-rejected` | `m.InlineCode != ""` (any setting) | `"lua: inline_code is deprecated; use default_source_code"` |
| 4 | `source-codes-deferred-to-22-3` | `len(m.SourceCodes) > 0` | `"lua: source_codes map is not yet supported (lands in phase 22.3)"` |
| 5 | `default-source-code-required` | `m.DefaultSourceCode == nil` AND no other source path (post-inline-code-rejection) | `"lua: default_source_code required"` (subject to §12-D1 disposition — see §12) |
| 6 | `data-source-specifier-required` | `ds.GetSpecifier() == nil` (no oneof arm set; bare `DataSource{}`) | `"lua: default_source_code: specifier oneof required"` |
| 7 | `data-source-watched-directory-deferred` | `ds.GetWatchedDirectory() != nil` | `"lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"` |
| 8 | `data-source-filename-empty` | `*DataSource_Filename` arm with `Filename == ""` | `"lua: default_source_code: filename empty"` |
| 9 | `data-source-filename-read-failed` | `os.ReadFile(Filename)` returns error (ENOENT / EACCES / EISDIR / etc.) | `"lua: default_source_code: read file %q: %w"` |
| 10 | `data-source-filename-empty-contents` | file readable but body length 0 | `"lua: default_source_code: file %q is empty"` |
| 11 | `data-source-inline-bytes-empty` | `*DataSource_InlineBytes` arm with `len(InlineBytes) == 0` | `"lua: default_source_code: inline_bytes empty"` |
| 12 | `data-source-inline-string-empty` | `*DataSource_InlineString` arm with `InlineString == ""` | `"lua: default_source_code: inline_string empty"` |
| 13 | `data-source-env-var-name-empty` | `*DataSource_EnvironmentVariable` arm with `EnvironmentVariable == ""` | `"lua: default_source_code: environment_variable name empty"` |
| 14 | `data-source-env-var-unset` | env var name set but `os.LookupEnv` returns `false` | `"lua: default_source_code: environment_variable %q not set"` |
| 15 | `data-source-env-var-empty-value` | env var name set, lookup returns true with empty string | `"lua: default_source_code: environment_variable %q is empty"` |
| 16 | `script-compile-failed` | resolved bytes fail gopher-lua `LState.LoadString` (syntax error) | `"lua: default_source_code: compile: %w"` |
| 17 | `script-missing-required-hooks` | compiled script defines NEITHER `envoy_on_request` NOR `envoy_on_response` (defensive — subject to §12-D1) | `"lua: default_source_code: script defines neither envoy_on_request nor envoy_on_response"` |
| 18 | `per-route-deferred-to-22-3` | any `typed_per_filter_config[envoy.filters.http.lua]` map entry at route/vhost level (via HCM `RegisterPerRouteValidator` hook per phase-20 §5.2 + ADR-0110 single-chokepoint) | `"lua: per-route configuration is not yet supported (lands in phase 22.3)"` |

Notes per arm:
- **Arm 3** (`inline-code-deprecated-rejected`): upstream Envoy honors `InlineCode` with a deprecation warn-log; envoy-go is stricter (mirrors phase-20 §20.P6 deprecated-`ConfigSource.path` PARSE-REJECT precedent). NOT one of the 3 envoy-go-strict BEHAVIOR_CONTRACT departure records — the deprecated-field-rejection discipline is project-wide and documented at ADR-0080-cluster references (DECISIONS.md:10254 et al.) not per-row.
- **Arm 5** (`default-source-code-required`): disposition subject to §12-D1 — if upstream Envoy treats absent-`default_source_code` + absent-`inline_code` as a no-op filter (silent pass-through), the disposition flips to **degraded no-op** rather than PARSE-REJECT. 22.1 IMPL anchors the empirical-pin verification at the IMPL-time first-task — see §12-D1.
- **Arm 9**: uses `%w` to wrap the `os.ReadFile` error (file-not-found / permission-denied / directory-instead-of-file all collapse into this arm). Mirrors jwtauthn's `data_source read file %q: %w` shape at `internal/filter/http/jwtauthn/provider.go:294`.
- **Arm 16**: wraps gopher-lua's `*lua.ApiError` compile error (variable bytes — file name + line number from gopher-lua's parser); structural prefix is byte-stable.
- **Arm 17**: phase-22-specific defensive check. Subject to §12-D1 (same empirical-pin verification — does upstream allow a hook-less script as a no-op filter?).
- **Arm 18**: uses the existing HCM `RegisterPerRouteValidator` hook per phase-20 §5.2 + ADR-0110 single-chokepoint; not a separate filter-package PARSE-REJECT.

**RETRACTED: "sandbox-violation" PARSE-REJECT arm.** Per AMEND-6 + §11.4. gopher-lua does NOT have a static-AST sandbox-scan; the deny-list discipline (per §4.3) is enforced at VM-construction time (don't expose denied modules to the per-stream VM) rather than at compile-time PARSE-REJECT. A static-AST-scan-for-banned-call PARSE-REJECT is non-idiomatic in Lua (BRAINSTORM §2.6 already rejected static-analysis-for-deferred-methods on idiomatic grounds; same applies here). NO `sandbox-violation` arm in the 22.1 roster.

### 6.3 22.3 forward-pointer arms (per §11.4.3 — 20+ arms anticipated)

Forward-pointer arms that PARSE-REJECT at 22.1+22.2 (via arms 4 + 18 above) and become fully-resolved at 22.3 when `Lua.SourceCodes` + `LuaPerRoute` become consumed:

1. **`source-codes-each-value-data-source-resolution` (8 arms × per-map-entry)** — each `SourceCodes[name]` map value goes through the 8-arm DataSource gauntlet (arms 6-15 above); error wording prefix `"lua: source_codes[%q]: ..."`.
2. **`source-codes-key-empty`** — wording `"lua: source_codes: key must be non-empty"`.
3. **`source-codes-key-duplicate-of-default`** — collision check (if a reserved-name discipline lands at 22.3 BRAINSTORM); wording `"lua: source_codes[%q] conflicts with default_source_code reserved name"`.
4. **`per-route-override-oneof-required`** — `LuaPerRoute.Override == nil` (PGV-mirror per validate.go:333-342); wording `"lua: per-route: override oneof is required"`.
5. **`per-route-disabled-must-be-true`** — `LuaPerRoute_Disabled` arm with `false` (PGV-mirror per validate.go:253-262); wording `"lua: per-route: disabled must be true (PGV const:true violation; disabled:false is not meaningful)"`.
6. **`per-route-name-min-1-rune`** — `LuaPerRoute_Name` arm with `""` (PGV-mirror per validate.go:277-286); wording `"lua: per-route: name length must be at least 1 rune"`.
7. **`per-route-name-not-in-source-codes`** — Name references a key not present in the listener-level `Lua.SourceCodes` map (cross-listener-resolution check at HCM-build time after both listener-level + per-route are parsed; mirrors jwt_authn 8th-canonical `provider_name` dangling-reference check); wording `"lua: per-route: name %q is not defined in source_codes"`.
8. **`per-route-source-code-each-arm` (8 arms)** — `LuaPerRoute_SourceCode` arm's inline `*DataSource` goes through the 8-arm DataSource gauntlet; wording `"lua: per-route: source_code: ..."`.

Total 22.3 forward-pointer arms: **~20 arms** (8 SourceCodes-DataSource × value + 8 per-route SourceCode-DataSource + 4 PGV-mirror arms). Combined 22.1+22.3 PARSE-REJECT roster: **~38 arms**.

---

## 7. Stat surface (per §11.5 + AMEND-2 + AMEND-3)

Per §11.5 empirical scrape + AMEND-2 (stat-prefix template REFUTED — actual is HCM-rooted) + AMEND-3 (`executions` reclassification — upstream-parity NOT envoy-go-strict).

### 7.1 22.1 stat-surface roster (3 counters; project 99 → 102)

| # | Internal name | Type | Source | Description |
|---|---|---|---|---|
| 1 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.errors` | counter | filter | Script execution errors (gopher-lua runtime errors caught by the panic-recovery wrapper). Upstream-parity per `lua_filter.cc:811`. |
| 2 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.executions` | counter | filter | Script invocations (every `envoy_on_request` / `envoy_on_response` call; increments per invocation). Upstream-parity per AMEND-3 + `lua_filter.cc:872`. |
| 3 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.respond_calls` | counter | filter | `:respond()` short-circuit invocations. **envoy-go-strict extension per AMEND-3** (NOT in upstream surface; collision-free verified at §11.5.3). Operator-visibility for how often Lua short-circuits the upstream call. |

### 7.2 Stat-prefix template (per AMEND-2)

`http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` — HCM-injected per-filter `stats_prefix` prefix (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac, phase-17 jwt_authn, phase-18.1+18.2 ext_authz, phase-19.1+19.2 ext_proc, phase-20 oauth2, phase-21 adaptive_concurrency — the dominant §9 family-row pattern; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). Upstream's `generateStats` constructs the prefix via `absl::StrCat(prefix, "lua.", filter_stats_prefix)` at `lua_filter.h:343-347`. Per AMEND-2: if `Lua.stat_prefix` is empty, names become `http.<HCM>.lua..errors` (literal consecutive dots — mirrors phase-14 compressor empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243).

### 7.3 Project stat-count delta

**99 → 102 at 22.1 (+3)** per §11.5.4 + AMEND-3 (arithmetic unchanged from BRAINSTORM §5.3 even after the executions reclassification — still 3 NEW names). 22.2 anticipated additions: likely +2 httpCall counters (22.2 SPEC settles). 22.3 anticipated additions: 0 (SHARED-vacuous per the 9th canonical's SHARED-stats discipline).

### 7.4 envoy-go-strict departure rationale (per AMEND-3 corrected to 1 record)

Per AMEND-3 + §11.5.3. The 1 envoy-go-strict extension (`respond_calls`) is NOT in upstream's stat surface; it is envoy-go-only for operator-visibility into `:respond()` short-circuit rate (operationally useful for policy-enforcement audit). The departure rationale + the BEHAVIOR_CONTRACT departure-record discipline land at the 22.1 IMPL final Task's 7-edit bundle per §13.6 + ADR-0052 atomic landing. **One stat departure record** (corrected from BRAINSTORM §5.4's 2 records) for `respond_calls`; the `executions` counter is upstream-parity per AMEND-3 and does NOT need a departure record. Parallel to phase-21's 2 envoy-go-strict departure records (RTT-ns + sorted-slice-percentile) — phase 22 has 3 total departure records: stat-related (`respond_calls` per AMEND-3) + stdlib-sandbox-strict (per AMEND-1) + runtime-error log-message wording (per AMEND-9 RECOMMEND-DEPARTURE-RECORD).

---

## 8. Differential fixture taxonomy (per §11.6 + §11.7)

Per §11.6 (`:respond()` wire-shape) + §11.7 (cross-side test-infra validation) + AMEND-10 (scenario (g) scope-narrow option 2) + AMEND-11 (`BackendKind=HTTPLua` + scripts/ subdirectory).

### 8.1 22.1 fixture-0026: full cross-side byte-exact for 6 wire-interactive scenarios + substring-match for scenario (g)

Fixture `0026-http-lua-headers-bridge` lands as full cross-side byte-exact for the 6 wire-interactive scenarios (a)-(f) per BRAINSTORM Q9 + §6.2 + §11.7.2. Scenario (g) (script-compile-error) narrows to **substring-match `"script load error"`** per AMEND-10 + §11.7.6 option 2 + the user's SPEC-time choice. Fixture structure: single directory `test/fixtures/0026-http-lua-headers-bridge/` per §8.4 layout.

### 8.2 Scenario taxonomy (7 scenarios)

| # | Name | Lua script behavior | Wire-output assertion (cross-side) |
|---|---|---|---|
| (a) | add-fixed-header | `function envoy_on_request(rh) rh:headers():add("x-lua-injected", "hello") end` | Request header `x-lua-injected: hello` present at upstream echobackend (driver asserts substring in reflected JSON body) |
| (b) | replace-header | `function envoy_on_request(rh) rh:headers():replace("user-agent", "envoy-go-lua/1.0") end` | Reflected `user-agent: envoy-go-lua/1.0` |
| (c) | remove-header | `function envoy_on_request(rh) rh:headers():remove("x-blocked") end` | Reflected request without `x-blocked` header |
| (d) | respond-shortcircuit | `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end` | Client receives full byte-pinned tuple: status `403 Forbidden`; `content-length: 6` (auto-set per §11.6.4); `content-type: text/plain` (default per `Utility::prepareLocalReply` at utility.cc:1241,1273); body `denied` (6 bytes verbatim, no trailing newline); no upstream request initiated. Per AMEND-7 mirroring phase-21 AMEND-6 precision. |
| (e) | log-only-passthrough | `function envoy_on_request(rh) rh:logInfo("lua hit") end` | Reflected request unchanged at upstream; **stat-counter delta** `lua.<prefix>.executions` increments (per §12-D3 disposition — option (a) recommended: stat-counter IS the "Lua ran" assertion; literal log line is supplementary, NOT cross-side asserted). |
| (f) | headers-iteration | `function envoy_on_request(rh) local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n)) end` | Reflected header `x-headers-count: N` where N is computable from the probe's request-header count (deterministic per §11.2.3 bridge-snapshot iteration; subject to §13-R3 headers-map insertion-order sub-pin) |
| (g) | script-compile-error | Lua source with intentional syntax error (e.g., `function envoy_on_request(rh) end syntax-error-here`) | **Both sides exit non-zero at config-load + both sides' stderr contains literal substring `"script load error"`** (per AMEND-10 option 2 scope-narrow). envoy-go's bootstrap wraps gopher-lua's compile error with the same `"script load error"` prefix per §13-W wording-pinning discipline at `cmd/envoy-go/main.go`. NOT byte-exact stderr (per §11.7.5 + AMEND-10 REFUTATION of BRAINSTORM Q9 byte-exact claim). |

### 8.3 Scenario (g) substring-match discipline (per AMEND-10 + §11.7.6 option 2)

The §11.7 empirical scrape REFUTED BRAINSTORM Q9 + §6.2 scenario (g) "byte-exact error logs" via three divergence sources (gopher-lua-vs-LuaJIT format; timestamp prefix; bootstrap dump). Per AMEND-10 + the user's SPEC-time choice: scope-narrow to substring-match `"script load error"`. Implementation discipline:

1. **Reference Envoy side** (per §11.7.5): upstream raises `LuaException(fmt::format("script load error: {}", lua_tostring(state.get(), -1)))` at `source/extensions/filters/common/lua/lua.cc`; the Envoy server wraps as `"error ``script load error: ...`` initializing config '...'"` to stderr at non-zero exit. The substring `"script load error"` is present.
2. **envoy-go side** (NEW wording-pinning discipline at IMPL): envoy-go's `cmd/envoy-go/main.go:60-66` boot-reject path wraps gopher-lua's compile error with a sibling `"script load error: "` prefix at the boot-reject log site. The 22.1 IMPL late-task (anticipated Task 13-15 per the phase-21 Task-12-fixture-precedent + late-task wiring) lands the wrapping. ~50 LoC delta. NO new driver interface; NO new ADR (lives inside ADR-0189 §Decision body).
3. **Fixture runner side** (per §11.7.3): the runner currently does NOT support config-load-fail testing (`StartReferenceProxy`/`StartSubjectProxy` both `t.Fatalf` on startup failure). The 22.1 IMPL fixture task adds a NEW OPTIONAL `BootRejectFixture` driver interface + `tryStart` variants in `harness.go` (~80-130 LoC). The runner's NEW `runBootRejectFixture` branch parallels `runReferenceLessFixture` at `runner_test.go:1268`. Asserts both sides exit non-zero AND both sides' stderr contains `"script load error"` substring. **RATIFIED-PENDING-IMPL-TIME-RUNNER-EXTENSION** per §13-R1 — lives in the 22.1 IMPL bundle without a NEW ADR (lives inside ADR-0189 §Decision body); does NOT consume the ADR-0190 escape-valve.

### 8.4 Recommended fixture-0026 directory structure (per §11.7.4 + AMEND-11)

```
test/fixtures/0026-http-lua-headers-bridge/
  README.md             # ~150-250 lines: scope + 7-scenario table + topology + cross-refs to SPEC §8 + ADR-0188+0189
  envoy.yaml            # reference Envoy bootstrap; single listener + lua filter; templated {{.BackendPort}}
  envoy-go.yaml         # subject bootstrap; same topology; templated {{.AdminPort}} {{.ListenerPort}} {{.BackendPort}}
  expectations.yaml     # human-readable declarative scenario expectations (NOT consumed by runner)
  inputs/
    driver.go           # registered Driver impl (~400-600 LoC); per-scenario probes + classifyBody
  scripts/              # NEW per fixture-0026 per AMEND-11 — per-scenario Lua source files
    a_add_header.lua
    b_replace_header.lua
    c_remove_header.lua
    d_respond.lua
    e_log_only.lua
    f_headers_iter.lua
    g_compile_error.lua  # intentional syntax error
```

The `scripts/` subdirectory exploits the DataSource `Filename` arm naturally (vs all-inline-strings collapsed into the YAML) — adds DataSource-arm coverage for free + improves per-scenario readability. Mirrors the fixture-0019 PKI-subdir pattern. Driver's `SubjectConfig` templates the path as `{{.FixtureDir}}/scripts/<scenario>.lua` per the fixture-0019 precedent.

### 8.5 Backend reuse (per AMEND-11)

NEW `BackendKind` constant `HTTPLua` (mirrors `HTTPCsrf` / `HTTPCompressor` / `HTTPAdaptiveConcurrency` precedent at `test/differential/runner_test.go:547`) — purely a switch-case addition in `runner_test.go` near line 547. ~20 LoC delta. REUSE shared `test/helpers/echobackend/cmd/echobackend/` (the shared 14+ helper) which reflects request headers as JSON body — needed for scenarios (a) + (b) + (c) + (f) to assert reflected headers.

### 8.6 22.2 + 22.3 fixtures (forward-pointer)

- **22.2 fixture-0027** `0027-http-lua-full-bridge`: PARTIAL cross-side. `:headers()` + `:streamInfo()` + `:respond()` scenarios continue cross-side byte-exact. `:body()` + `:trailers()` + `:metadata()` scenarios may be cross-side byte-exact (deterministic) or REFERENCE-LESS (if dynamic-metadata-bridge is deferred). `:httpCall()` + `:timestamp()` scenarios fall back to REFERENCE-LESS subject-only (non-deterministic timing + non-deterministic outbound call ordering). 22.2 BRAINSTORM settles the exact taxonomy.
- **22.3 fixture-0028** `0028-http-lua-multi-script-and-per-route`: cross-side byte-exact for the multi-script + per-route scenarios that produce deterministic wire output. Scenario taxonomy similar to 22.1 but exercises the `SourceCodes` named-script lookup + the `LuaPerRoute` 3-arm oneof per-route override resolution. 22.3 BRAINSTORM settles the exact scenario list.

---

## 9. Behavior-contract delta (semantic; per AMEND-1 + AMEND-3 + AMEND-9)

The phase-22 behavior-contract delta vs phase-21 baseline (high-level semantic changes; the verbatim Markdown patch lives at §13):

1. **Lua-script-driven HTTP filter semantics** — NEW class of filter that delegates per-request behavior to operator-authored interpreted scripts (FIRST §9 row per §1). Observable: operator-supplied Lua source compiled at config-load (PARSE-REJECT on compile failure per §6.2 arm 16); per-request invocation of `envoy_on_request`/`envoy_on_response` hooks against bridge methods. The behavior depends entirely on the operator's script — envoy-go's behavior-contract claim is **upstream-parity at the bridge-method API surface** (modulo the documented divergences per AMEND-9).

2. **Stdlib-sandbox-strict envoy-go-strict departure** (per AMEND-1). Documented as an envoy-go-strict departure from upstream's bare-`luaL_openlibs`-no-neutering posture. Rationale: gopher-lua exposes sandbox-breaking modules (`os.execute`/`os.exit`/`io.popen`/`package.*`/`channel`/`debug.*`) that upstream does NOT need to deny because LuaJIT's deployment model in upstream Envoy assumes operator trust + per-worker VM scoping; envoy-go's per-stream goroutine dispatch model cannot make the same assumption (any operator's Lua can compromise the entire envoy-go process). Recorded at BEHAVIOR_CONTRACT.md §13.6 row 1.

3. **`respond_calls` envoy-go-strict counter** (per AMEND-3 — corrected from BRAINSTORM §5.4 2-record bundle to 1-record bundle). Documented as an envoy-go-strict departure for operator-visibility into `:respond()` short-circuit frequency. Recorded at BEHAVIOR_CONTRACT.md §13.6 row 2.

4. **Runtime-error log-message wording divergence** (per AMEND-9 RECOMMEND-DEPARTURE-RECORD). gopher-lua's `pcall` error-message format `"[string \"chunk\"]:line: msg"` diverges from LuaJIT's `"chunk:line: msg"`. The wire NEVER carries the error string (per BRAINSTORM §2.9 — errors increment stat + log + continue without mutation); only the envoy log shows the divergent wording. Recorded at BEHAVIOR_CONTRACT.md §13.6 row 3 as the third envoy-go-strict departure.

5. **Scenario (g) substring-match cross-side claim** (per AMEND-10). The fixture-0026 differential gate weakens from BRAINSTORM Q9's "byte-exact error logs" to "both-sides-exit-non-zero + both-sides-stderr-contains-`"script load error"`". Recorded at BEHAVIOR_CONTRACT.md §13.7 as a phase-22-specific cross-side-equivalence carve-out.

6. **Header `__pairs` iteration determinism** (per AMEND-9 + §11.2.3 sub-pin). Bridge-snapshot-driven (HeaderMapWrapper::luaPairs in upstream + envoy-go's mirror at IMPL); pure-Lua table iteration unspecified per Lua-5.1 §2.5.7. Subject to §13-R3 sub-pin verification at 22.1 IMPL: envoy-go's underlying headers-map MUST be insertion-ordered for scenario (f) determinism.

---

## 10. Deferred items + forward-pointers (per BRAINSTORM §8 + §11 corrections)

The full envelope-D scope is delivered across 22.1 + 22.2 + 22.3. Items DEFERRED to future phases (cross-phase boundaries) + items FORWARD-POINTED for future SPEC / IMPL resolution. Sourced from BRAINSTORM §8 with §11 empirical corrections layered:

1. **`WatchedDirectory` DataSource arm hot-reload** (per Q5 + §2.1) — PARSE-REJECT at 22.1 (§6.2 arm 7); deferred to future Runtime / RTDS / hot-reload family phase.
2. **`Lua.InlineCode` deprecated field** (per envoy-go-strict + §2.2) — PARSE-REJECT at 22.1 (§6.2 arm 3); never re-enabled in envoy-go.
3. **Lua 5.2 / 5.3 / 5.4 dialect features** (per §2.3) — gopher-lua is Lua 5.1; documented in `internal/lua/doc.go` at 22.1 IMPL.
4. **Dynamic-metadata bridge surfaces** (`:metadata()` + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()`) — per §2.4; settled at 22.2 SPEC.
5. **Async script execution coroutines** (per §2.14) — settled at 22.2 SPEC; 22.1 sandbox exposes `coroutine` library per AMEND-A4 but no 22.1 bridge methods consume.
6. **`:httpCall()` outbound HTTP call** (per §2.6) — 22.2; reuses phase-20 `internal/httpclient/` at first co-consumer (validates the phase-20 extraction).
7. **Body-buffering interaction with ADR-0128** (22.2 `:body()` per §2.5) — 22.2 SPEC settles the exact interaction discipline.
8. **`:connection()` SSL/TLS access** (22.2 per §2.7) — integrates with phase-03 TLS primitives.
9. **Crypto / base64 / sha helpers** (22.2 per §2.8) — likely thin wrappers over Go's `crypto/*` + `encoding/base64`.
10. **`:fileBytes()` file-read helper** (22.2 per §2.9) — security caveats apply (sandbox concern).
11. **`:timestamp()` time helper** (22.2 per §2.10) — non-deterministic; fixture cross-side byte-exact challenges.
12. **Full `:streamInfo()` surface** (22.2 per §2.11) — `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata()` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()`.
13. **`Lua.SourceCodes` named-script map** (22.3 per §2.12 + 9th canonical Name arm) — multi-script lookup.
14. **`LuaPerRoute` 3-arm oneof** (22.3 per §2.13 + 9th canonical) — per-route override; NEW 9th canonical per Q7.
15. **Cluster specifier Lua + access logger Lua + string matcher Lua** (future cross-family phases) — consumers #2/3/4 for the `internal/lua/` framework primitive; each future phase BRAINSTORM revisits the API shape per Q4's API-REVISION ALLOWANCE clause.
16. **`Lua.clear_route_cache` (v1.37.2 field 5; v1.32.4 binding-gap)** (per AMEND-12 + §5.4) — activates when go-control-plane bumps to v1.37.x.
17. **`LuaPerRoute.filter_context` (v1.37.2 field 4; v1.32.4 binding-gap)** (per AMEND-12 + §5.4) — same disposition.
18. **`tostring(float)` + `string.format("%d", float)` gopher-lua-vs-LuaJIT divergences** (per AMEND-9) — forward-pointer for 22.2 + recommends a `lua.FormatNumber(v) string` helper on `internal/lua/` API surface for 22.2 use.
19. **`*LState`-pool design** (per §11.3.4 + §1.2 escape-valve candidate) — 22.1 IMPL benchmark gates whether the ADR-0190 escape-valve fires.

---

## 11. SPEC-time empirical-pin block — all 7 §10 pins resolved IN-SESSION (per ADR-0004)

This block contains the verbatim parallel-subagent-fan-out scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09–21 SPEC §11's structure. **Probe date: 2026-05-18.** The 7 pins span all three sub-phases, so they are resolved once, here, in the parent SPEC; each sub-phase SPEC references this block.

**Reference source corpus** (multi-axis verification per the phase-15/16/17/18/19/20/21 discipline):

1. **`go-control-plane v1.32.4` bindings** (the ADR-0008 proto pin) at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/`: `extensions/filters/http/lua/v3/{lua.pb.go, lua.pb.validate.go}`; `config/core/v3/base.pb.go` (DataSource).
2. **Upstream Envoy v1.37.2 source + IDL** via WebFetch against `github.com/envoyproxy/envoy` at tag v1.37.2: `source/extensions/filters/http/lua/{lua_filter.cc, lua_filter.h, wrappers.cc, wrappers.h, config.cc, config.h}`; `source/extensions/filters/common/lua/{lua.cc, lua.h}`; `source/common/http/utility.cc`; `api/envoy/extensions/filters/http/lua/v3/lua.proto`; `api/envoy/config/core/v3/base.proto`; `test/extensions/filters/http/lua/{lua_filter_test.cc, config_test.cc}`.
3. **`github.com/yuin/gopher-lua`** via WebFetch against `master` (snapshot at `v1.1.2` recommended pin per AMEND-2): `baselib.go`, `stringlib.go`, `iolib.go`, `oslib.go`, `debug_api.go`, `loadlib.go`, `channellib.go`, `state.go`, `auxlib.go`, `linit.go`, `value.go`, `table.go`, `README.md`.

### Summary disposition table (7 pins → 12 AMENDs)

| Pin | Topic | Disposition | AMEND cross-ref |
|---|---|---|---|
| §11.1 | Proto-field roster (Lua + LuaPerRoute + DataSource against v1.32.4 + v1.37.2) | CONFIRMS 4 Lua fields + 3-arm LuaPerRoute oneof + 4-arm DataSource + 2 binding-gaps | AMEND-12 |
| §11.2 | gopher-lua-vs-LuaJIT observable-output divergence across 22.1 bridge methods | PARTIAL-REFUTE §2.9 wording; CONFIRMS 22.1 fixture scenarios safe; EXTENDS named risks | AMEND-9 (+ headers-map sub-pin §13-R3) |
| §11.3 | Sandbox config + per-worker VM scoping | REFUTES BRAINSTORM §7.2 surface-1 (upstream does NOT sandbox); EXTENDS API shape; flags ESCAPE-VALVE candidate | AMEND-1 + AMEND-A2 + AMEND-A4 |
| §11.4 | PARSE-REJECT arm roster build for 22.1 | CONFIRMS-AND-TIGHTENS to 18 arms; REFINES DataSource framing; EXTENDS with baseline arms; RETRACTS sandbox-violation arm | AMEND-4 + AMEND-5 + AMEND-6 |
| §11.5 | Stat-surface byte-exactness against upstream | REFUTES template; REFUTES executions classification | AMEND-2 + AMEND-3 |
| §11.6 | `:respond(headers, body)` wire-shape byte-exactness | CONFIRMS short-circuit; EXTENDS scenario (d) byte-pin + `:status` validation + `envoy_on_response` runtime-reject + sub-table multi-value | AMEND-7 + AMEND-8 |
| §11.7 | Cross-side fixture infra validation for fixture-0026 | CONFIRMS (a)-(f); REFUTES (g) byte-exact-stderr; option 2 chosen per user; EXTENDS with NEW BackendKind + scripts/ layout | AMEND-10 + AMEND-11 |

### §11.1 Proto-field roster scrape

Per parallel-subagent §9.1 report. The full scrape evidence:

**§11.1.1 `Lua` message field roster** (4 fields per §5.1; v1.32.4 pb.go lines 27-83 enumeration). `inline_code` field 1 + `source_codes` field 2 + `default_source_code` field 3 + `stat_prefix` field 4. The pb.validate.go file shows NO PGV rules on any of these fields (lines 39-144 are all `// no validation rules for X`); the message-level oneof-required does NOT apply (Lua has no top-level oneof). Sub-phase mapping per §5.1 table.

**§11.1.2 `LuaPerRoute` message field roster** (3-arm `override` oneof per §5.2; v1.32.4 pb.go lines 146-244). The `override` oneof itself carries `(validate.required) = true` (validate.go:333-342) — PGV-required at 22.3 consume time. Per-arm PGV: `disabled` `bool {const: true}`; `name` `string {min_len: 1}`; `source_code` embedded recursive validation only.

**§11.1.3 `DataSource` arm roster** (4 oneof arms + WatchedDirectory sibling per §5.3; upstream v1.37.2 base.proto identical shape in v1.32.4 binding). `specifier` oneof itself `option (validate.required) = true`. `watched_directory` is a SIBLING field (not an arm) — upstream docstring constrains it to fire "only when ``filename`` field is set."

**§11.1.4 v1.32.4 vs v1.37.2 delta** — Two binding-gaps per AMEND-12 + §5.4:
- `Lua.clear_route_cache` (field 5; type `google.protobuf.BoolValue`) — Present in v1.37.2 IDL; ABSENT from v1.32.4 pb.go. Silent-drop on parse; no PARSE-REJECT applies.
- `LuaPerRoute.filter_context` (field 4; type `google.protobuf.Struct`; sibling of `override` oneof) — Same disposition.

No fields present in v1.32.4 but absent from v1.37.2 (no forward-removal).

**§11.1.5 D-question candidate (anchored at §12-D1)** — PGV behavior when `Lua.default_source_code` is unset AND `Lua.source_codes` is empty AND `Lua.inline_code` is empty. v1.32.4 pb.validate.go has NO top-level required-anyOf constraint (no oneof on Lua). Upstream Envoy behavior: at least one source-of-code must be present (otherwise filter has nothing to execute). The parent SPEC's §6.2 arm 5 PARSE-REJECT disposition is subject to verification at IMPL — if upstream allows the absent-all case as a no-op (degraded), envoy-go's discipline flips from PARSE-REJECT to silent no-op (cf. ADR-0125 6th canonical "Bare-message-via-TPFC + code-level-required" precedent at phase 15 bandwidth_limit). See §12-D1.

### §11.2 gopher-lua vs LuaJIT observable-output divergence

Per parallel-subagent §9.2 report. Methodology: byte-comparison across all 22.1 bridge methods (headers, `:logXxx()`, `:streamInfo()` subset, `:respond()`); empirical evidence from gopher-lua `value.go`/`stringlib.go`/`state.go` + Envoy `wrappers.cc`/`lua.cc`.

**§11.2.1 Scope of comparison** — Per AMEND-9 + §1.1 surfaces.

**§11.2.2 Per-method / per-stdlib divergence assessment**:
- `tostring(integer)` — CONFIRMED-IDENTICAL (gopher-lua `value.go::LNumber.String()` `fmt.Sprint(int64(nm))` vs LuaJIT `%.14g` produce same bytes for int64-magnitude values < 1e15).
- `tostring(float)` — CONFIRMED-DIVERGENT for non-integer-valued floats (gopher-lua's Go-shortest vs LuaJIT's `%.14g`). The 22.1 fixture scenarios (a)-(g) never exercise `tostring(float)` on the wire (scenario (f) uses integer count). Forward-pointer for 22.2 per AMEND-9.
- `tostring(nil)` / `tostring(true/false)` — CONFIRMED-IDENTICAL.
- `string.format("%d", n)` — CONFIRMED-DIVERGENT (gopher-lua's `fmt.Sprintf` mismatch produces `"%!d(float64=...)"` vs LuaJIT's silent truncation). 22.1 fixture never calls. Forward-pointer for 22.2.
- `__pairs` iteration order — CONFIRMED-DIVERGENT for pure-Lua tables; NOT-APPLICABLE to 22.1 headers (bridge-snapshot-driven per §11.2.3).
- `pcall` error-message format — CONFIRMED-DIVERGENT (gopher-lua `[string "chunk"]:line: msg` vs LuaJIT `chunk:line: msg`). Out of fixture path (errors go to log + stat, not wire per BRAINSTORM §2.9). RECOMMEND-DEPARTURE-RECORD per AMEND-9.

**§11.2.3 Header-iteration determinism** — CONFIRMED-IDENTICAL semantics with caveat. Upstream `HeaderMapWrapper::luaPairs` (per `wrappers.cc`) snapshots header pointers via `parent_.headers_.iterate([&]...{ entries_.push_back(&header); })`; Envoy's `HeaderMap` iteration is insertion-ordered. Lua `__pairs` metamethod walks `entries_` by integer index — neither gopher-lua's nor LuaJIT's table-hash-order behavior is in play; order fixed by C++/Go iteration over the snapshot vector. For envoy-go's `internal/filter/http/lua/bridge.go` `__pairs` metamethod: MUST snapshot headers into a Go slice at `__pairs` invocation + iterate by index. **Sub-pin §13-R3:** verify envoy-go's underlying headers-map iteration is insertion-ordered (load-bearing for scenario (f) cross-side + subject-side determinism). If the headers-map is a Go `map[string][]string` directly (unordered), iteration order will be random per-run.

**§11.2.4 gopher-lua version pin recommendation** — **`github.com/yuin/gopher-lua v1.1.2`** (recommended pin per AMEND-2; suitable for `internal/lua/doc.go` per ADR-0008-equivalent discipline; pair with SHA in `go.mod` for human discoverability).

### §11.3 Sandbox config + per-worker VM scoping

Per parallel-subagent §9.3 report.

**§11.3.1 gopher-lua stdlib roster** — Per AMEND-1 + §4.3 table. 10 stdlib modules exposed by `OpenLibs()` iterating the `luaLibs` slice at `linit.go`.

**§11.3.2 Upstream Envoy LuaJIT sandbox shape** — REFUTES BRAINSTORM §7.2 surface-1. `source/extensions/filters/common/lua/lua.cc` calls `luaL_openlibs(state.get())` — full LuaJIT stdlib — **without subsequent neutering**. `wrappers.cc` registers per-method bridges but does NOT strip `os.*`/`io.*`/`debug.*`/`package.*`. envoy-go's 22.1 strict default-deny is an envoy-go-strict DEPARTURE not parity preservation. The departure rationale: gopher-lua's stdlib has sandbox-breaking arms that upstream LuaJIT in upstream Envoy's per-worker deployment model assumes operator-trust + per-worker VM scoping bounds — envoy-go's per-stream goroutine dispatch model cannot make the same assumption.

**§11.3.3 envoy-go sandbox API design** — Per AMEND-A2 + §4.1 + §4.3. Concrete `SandboxConfig` struct shape recommended for `internal/lua/` primitive. Default-deny zero-value matches upstream-parity-strict posture; explicit allow-toggles compose with the per-script `compiledConfig` so consumer #2/3/4 phases can each pick a posture without API revision.

**§11.3.4 Per-`*LState` construction cost + recommendation** — Per §4.2 + §1.2. Per-stream `*LState` construction with shared per-script-source `*Chunk` cache is the 22.1 WEAK-default. gopher-lua's `state.go::NewState` allocates registry + call-stack + runs `OpenLibs()` (10 module registrations); gopher-lua README explicitly recommends `sync.Pool` for per-thread reuse: *"To create per-thread LState instances, You can use the `sync.Pool` like mechanism … LState pool addresses the non-goroutine-safe nature of individual LState instances."* **ESCAPE-VALVE-CANDIDATE per §1.2:** 22.1 IMPL benchmark gates whether ADR-0190 escape-valve fires for the `*LState`-pool design.

**§11.3.5 Compile-cache discipline** — Per §4.1 API. Chunks (`*FunctionProto`) safe for cross-LState reuse; cache owned by `*compiledConfig` (filter-config-instance scope; GC-driven eviction). API: `NewCompileCache` + `CompileScript(src, cache) (*Chunk, error)` + `VM.Run(chunk, hooks...)`.

### §11.4 PARSE-REJECT roster build

Per parallel-subagent §9.4 report.

**§11.4.1 Prior-phase wording precedent** — Per §6.1. Format `"<filter_proto_name>: <field_path>: <reason> [; <hint>]"`. Phase-21 SPEC §5 is the cleanest single-table precedent; phase-20 + phase-19 + phase-18 SPECs carry similar.

**§11.4.2 22.1 PARSE-REJECT roster** — Per §6.2 (18 arms exactly per AMEND-4).

**§11.4.3 22.3 forward-pointer arms** — Per §6.3 (~20 arms).

**§11.4.4 AMEND recommendations** — Per AMEND-4 + AMEND-5 + AMEND-6.

**§11.4.5 Fuzzer-count claim verification** (per AMEND-A4 — pending verification per §13-R4): BRAINSTORM §3.7 claims `FuzzLuaConfigParse` is the 28th project-wide fuzzer. Current `internal/filter/http/` fuzzer count is **16** (per `grep -c "^func Fuzz" internal/filter/http/**/fuzz_test.go`). The 28th claim assumes additional fuzzers outside `internal/filter/http/` are counted. RECOMMEND the 22.1 IMPL first-task scrape verify the 28th-fuzzer claim by grepping `^func Fuzz` across the whole project tree before pinning the number in the §13.4 BEHAVIOR_CONTRACT.md patch. Anchored at §13-R4 RATIFIED-PENDING-IMPL-TIME item.

**§11.4.6 sandbox-violation arm RETRACTED** — Per AMEND-6 + §6.2 note. gopher-lua does NOT have a static-AST sandbox-scan; deny-list enforced at VM-construction time. No PARSE-REJECT arm.

### §11.5 Stat-surface byte-exactness

Per parallel-subagent §9.5 report.

**§11.5.1 Upstream stat-name roster** — `source/extensions/filters/http/lua/lua_filter.h:23-24` (v1.37.2):
```cpp
#define ALL_LUA_FILTER_STATS(COUNTER) COUNTER(errors) COUNTER(executions)
```
Exactly 2 upstream counters; both bumped at `lua_filter.cc:811` (`errors`) and `:872` (`executions`). No gauges, no histograms. The 22.1 MVP roster is upstream-complete for the 2 names + 1 envoy-go-strict (`respond_calls`).

**§11.5.2 Stat-prefix template empirical correction** — Per AMEND-2 + §7.2. `lua_filter.h:343-347` constructs `absl::StrCat(prefix, "lua.", filter_stats_prefix)` where `prefix` is HCM-injected. True template: `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>`. The proto godoc shorthand at `lua.pb.go:77,81` (`lua.<stat_prefix>.errors`) is upstream's abbreviated form, NOT the literal full path. Empty `Lua.stat_prefix` produces literal consecutive-dot names (`http.<HCM>.lua..errors`) — mirrors phase-14 compressor empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243.

**§11.5.3 envoy-go-strict extensions confirmation** — `executions` is upstream-parity NOT envoy-go-strict (per AMEND-3 + the `ALL_LUA_FILTER_STATS` macro). `respond_calls` is collision-free vs upstream (no upstream stat with `respond` substring; no collision with any existing 99 project stat names). Departure-record bundle drops from 2 → **1** for `respond_calls`.

**§11.5.4 99-stat math verification** — BEHAVIOR_CONTRACT.md line 367: "Phase 21 total: 92 → 99 internal names" with breakdown `17 + 5 + 4 + 3 + 0 + 17 + 14 + 4 + 7 + 6 + 0 + 9 + 0 + 6 + 7 = 99` CONFIRMED. Phase 22.1 adds 3 (`errors` + `executions` + `respond_calls`) → **99 → 102 (+3)**. Arithmetic UNCHANGED from BRAINSTORM §5.3.

### §11.6 `:respond()` wire-shape byte-exactness

Per parallel-subagent §9.6 report.

**§11.6.1 Argument shape** — Per AMEND-7 + §11.6.2-§11.6.7. Stack: `[req_handle, headers_table, body_string_optional]`. `luaL_checktype(state, 2, LUA_TTABLE)` enforces headers table presence; `luaL_optlstring(state, 3, nullptr, &body_size)` reads body as raw `(const char*, size_t)` — binary-safe.

**§11.6.2 Status code handling** — `lua_filter.cc:~578-580`: `absl::SimpleAtoi(headers->getStatusValue(), &status)`; range-check `status < 200 || status >= 600` raises `luaL_error(state, ":status must be between 200-599")`. Test `ImmediateResponseBadStatus` confirms the byte-exact error string. envoy-go MUST mirror per AMEND-8.

**§11.6.3 Body bytes** — `lua_filter.cc:~571-574`: `Buffer::OwnedImpl(raw_body, body_size)`; `headers->setContentLength(body_size)`. NO transformation — verbatim byte pass-through. Test `ImmediateResponse` confirms `EXPECT_EQ("nope", body)` for `:respond({[":status"]="503"}, "nope")` — 4 bytes verbatim.

**§11.6.4 Auto-set headers** — `source/common/http/utility.cc:1237-1284` `Utility::prepareLocalReply`:
- `content_type` defaults to `Headers::get().ContentTypeValues.Text` (= `"text/plain"`) at line 1241; applied at line 1273 IFF `modify_headers_` did not supply one.
- `content-length` auto-set to `body_text.size()` at line 1270 when body non-empty; removed when body empty.
- Status line set from `Code` enum (NOT from the `:status` string the Lua passed — values match because `luaRespond` validated/extracted `:status` and passed through as `Http::Code`).

**§11.6.5 Header propagation** — `buildHeadersFromTable` (lua_filter.cc:122-145) iterates Lua-table keys + `addCopy`s under `LowerCaseString(key)`. NO filtering of `:` pseudo-headers (`:status` IS added, then re-set from Code enum). Multi-value: Lua sub-table value produces one header line per inner string.

**§11.6.6 Multiple-respond + cross-hook-respond semantics** — `:respond()` from `envoy_on_response` raises `luaL_error(state, "respond not currently supported in the response path")` per `lua_filter.cc:1031-1034`. Test `RespondInResponsePath` confirms. `:respond()` after headers continued raises `luaL_error(state, "respond() cannot be called if headers have been continued")` per `lua_filter.cc:562`. After successful `:respond()`, state → `Responded` + `lua_yield(state, 0)` — no further script code runs in that hook. envoy-go MUST mirror both runtime-error strings per AMEND-8.

**§11.6.7 envoy-go byte-exact wire shape (SPEC pin)** — Per §8.2 scenario (d) AMEND-7 + AMEND-8. For `:respond({[":status"]="403"}, "denied")` from `envoy_on_request`:
- Status line: `HTTP/1.1 403 Forbidden`
- `content-length: 6` (auto-set per utility.cc:1270)
- `content-type: text/plain` (default per utility.cc:1241,1273; applied because Lua headers did NOT supply content-type)
- Body: `denied` (6 bytes verbatim; no trailing newline; no JSON wrapping)
- `response_code_details`: `"lua_response"` (lua_filter.cc:1027; access-log surface only, not on wire)
- No upstream request initiated

### §11.7 Cross-side comparison test infrastructure for fixture-0026

Per parallel-subagent §9.7 report.

**§11.7.1 Existing runner capabilities** — `test/differential/` (1903 LoC total). `runFixture` 12-step orchestration at `runner_test.go:83`. Driver contract is `Driver` interface (8 mandatory methods + 10 OPTIONAL interfaces selected via type-assertion). Comparison surfaces: HTTP-response BYTE-EXACT (`CompareBytes` step 7); per-request round-trip (status + body + header set-equal); stats scrape diff; access-log byte diff.

**§11.7.2 Scenario (a)-(f) HTTP response byte-compare** — Existing runner SUFFICES with ZERO infrastructure additions. fixture-0023 precedent (phase-19.2 ext_proc body — most-recent full-cross-side 6+ scenario fixture) demonstrates exact pattern: `DriveReference` + `DriveSubject` both call shared `driveProxy` accumulating per-scenario `scenarioResult` records into `bytes.Buffer` via `emitScenario`; per-scenario wire shape encoded with deterministic `scenario <id> status=%d body=%s\n` lines via `classifyBody` (insulates from non-substantive divergences like upstream's `date:` header).

**§11.7.3 Scenario (g) envoy-log byte-compare** — REFUTES BRAINSTORM Q9. Runner does NOT support envoy-log byte-comparison today; does NOT support config-load PARSE-REJECT testing (both `StartReferenceProxy` and `StartSubjectProxy` `t.Fatalf` on startup failure). gopher-lua compile error format diverges from LuaJIT's. Three IMPL-time options per §11.7.6; SPEC's choice is option 2 (substring-match `"script load error"`).

**§11.7.4 Recommended fixture-0026 layout** — Per §8.4.

**§11.7.5 Reference Envoy script-compile-error wire shape** — `source/extensions/filters/common/lua/lua.cc` (v1.37.2): `throw LuaException(fmt::format("script load error: {}", lua_tostring(state.get(), -1)))`. Substring `"script load error"` present in upstream's stderr at non-zero exit.

**§11.7.6 AMEND recommendations** — Per AMEND-10 (option 2 chosen) + AMEND-11.

**§11.7.7 D-question candidates** — Per §12-D2 (scenario (g) option already chosen — option 2; D-question CLOSED at SPEC commit), §12-D3 (scenario (e) `:logInfo()` cross-side assertion shape), §12-D4 (scripts/ subdirectory vs inline DataSource.InlineString — chosen scripts/ per §8.4 + AMEND-11; D-question CLOSED at SPEC commit).

---

## 12. SPEC-time D-questions for PLAN-time resolution

Per phase-21 SPEC §12 + phase-18+19+20 D-question precedent. SPEC-time D-questions surface unresolved decisions that the parent SPEC author anchors for PLAN-time resolution (the 22.1 PLAN session that follows the 22.1 SPEC). The parent SPEC has already CLOSED 2 of the §11.7 D-question candidates at SPEC commit (§11.7.7 D2 + D4); the remaining open D-questions:

### D1 (per §11.1.5 + §6.2 arm 5 + arm 17): `default_source_code` absent vs no-op disposition

**Question:** When `Lua.DefaultSourceCode` is unset AND `Lua.SourceCodes` is empty AND `Lua.InlineCode` is empty (post-`InlineCode` PARSE-REJECT), what is upstream Envoy's behavior — PARSE-REJECT (envoy-go's §6.2 arm 5 disposition) or degraded no-op (silent pass-through)? Same question applies to `script-missing-required-hooks` (§6.2 arm 17) — does upstream allow a compile-clean hook-less script as a no-op?

**Resolution at:** 22.1 IMPL first-task scrape against `source/extensions/filters/http/lua/config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor at v1.37.2. If upstream PARSE-REJECTs both cases (which is the expected disposition — envoy-go matches), §6.2 arms 5 + 17 STAND. If upstream allows the absent-all case as a no-op (degraded), §6.2 arms 5 + 17 flip to silent no-op (cf. ADR-0125 6th canonical "Bare-message-via-TPFC + code-level-required" precedent at phase 15 bandwidth_limit).

**Anticipated answer (BRAINSTORM hypothesis):** PARSE-REJECT both (upstream cannot operate a filter with no script source). 22.1 IMPL first-task closes the empirical pin.

### D3 (per §11.7.7 D2): scenario (e) `:logInfo()` cross-side assertion shape

**Question:** BRAINSTORM §6.2 wire-output column for scenario (e) states "Request unchanged at upstream + envoy log message". The runner cannot byte-diff log output today (no `LogAsserter` interface beyond `AccessLogAsserter` for access-log files). Three options:
- (a) Drop the "envoy log message" assertion from cross-side scope; rely on `lua.<prefix>.executions` stat counter (per §7) to confirm the script ran.
- (b) Add `:logInfo()` calls that ALSO bump a counter the driver can scrape (artificial — pollutes the script).
- (c) Introduce a NEW `LogAsserter` interface paralleling `AccessLogAsserter` (heavier infra delta).

**Resolution at:** 22.1 PLAN session.

**Anticipated answer (RECOMMENDED per §11.7.7):** option (a). The stat-counter delta IS the "Lua ran" assertion; the literal log line is supplementary. Scenario (e)'s wire-output column should read "Request unchanged at upstream; `lua.<prefix>.executions` counter delta = 1 per probe."

### D5 (per §11.4 + AMEND-A4 + §11.4.5): 28th-fuzzer claim verification

**Question:** BRAINSTORM §3.7 claims `FuzzLuaConfigParse` is the 28th project-wide fuzzer. Current `internal/filter/http/` count is 16. The 28th claim assumes additional fuzzers outside `internal/filter/http/` are counted (likely fuzzers in `internal/lua/` itself or elsewhere). What is the actual project-wide fuzzer count post-phase-21?

**Resolution at:** 22.1 IMPL first-task scrape. `grep -c "^func Fuzz" $(find /home/esa/git/envoy-go -name 'fuzz_test.go')`. Pin the actual number in §13.4 BEHAVIOR_CONTRACT.md patch + ADR-0189 §Decision body.

### D7 (per §11.2.3 + sub-pin §13-R3): envoy-go headers-map insertion-order verification

**Question:** Per §11.2.3 + AMEND-9. Header `__pairs` iteration determinism is bridge-snapshot-driven AND relies on envoy-go's underlying headers-map having insertion-ordered iteration. Verify envoy-go's headers-map type:
- If insertion-ordered slice-backed (e.g., `[]struct{k, v string}` or similar), scenario (f) cross-side AND subject-side determinism HOLDS.
- If Go `map[string][]string` directly (unordered), scenario (f) order is per-run random — breaks determinism.

**Resolution at:** 22.1 IMPL first-task scrape against `internal/filter/http/header_mutation/` (the canonical headers-map consumer) + the HCM headers-map type. If insertion-ordered: scenario (f) STANDS. If unordered: scenario (f) flips to a "sorted-list" assertion (snapshot + sort before iterate) OR re-frames as a "count-only" assertion (just N, not the specific order).

**Anticipated answer (BRAINSTORM hypothesis):** insertion-ordered (most Envoy-style headers-maps are insertion-ordered slice-backed for performance + HTTP/2 HPACK compatibility). 22.1 IMPL first-task closes the empirical pin.

---

## 13. RATIFIED-PENDING-IMPL items

Per phase-21 SPEC §12 + phase-20 + phase-19 + phase-18 SPEC RATIFIED-PENDING-IMPL precedent. Items the SPEC anchors as RATIFIED at SPEC commit but pending IMPL-time confirmation against the actual envoy-go codebase state. Each item lists the SPEC-time hypothesis + the IMPL Lands-in-Task that closes the pin.

### Wire-shape byte-confirmations

- **R1: scenario (g) substring-match `"script load error"` infrastructure** (per AMEND-10 + §8.3). 22.1 IMPL late-task (anticipated Task 13-15 per the phase-21 Task-12-fixture-precedent) lands the NEW OPTIONAL `BootRejectFixture` driver interface + `tryStart` variants in `harness.go` + `runBootRejectFixture` runner branch + envoy-go-side `"script load error: "` wrapping at `cmd/envoy-go/main.go:60-66` boot-reject path. ~80-130 LoC `harness.go` + `runner_test.go` delta + ~50 LoC `main.go` wrapping. NO new ADR (lives inside ADR-0189 §Decision body).

- **R2: scenario (d) `:respond()` full byte-pin** (per AMEND-7 + §8.2). 22.1 IMPL fixture-0026 task (anticipated Task 13-15) lands the driver-side assertion of `{status 403; content-length: 6; content-type: text/plain; body "denied"}` against both reference + subject sides via `classifyBody` or sibling probe-emit shape.

### Library-behavioral

- **R3: envoy-go headers-map insertion-order verification** (per §11.2.3 + §12-D7). 22.1 IMPL first-task scrape verifies the headers-map iteration order. If insertion-ordered (anticipated): scenario (f) cross-side determinism STANDS. If unordered: scenario (f) re-frames per §12-D7 resolution.

- **R4: 28th-fuzzer count verification** (per §11.4.5 + §12-D5). 22.1 IMPL first-task scrape confirms project-wide fuzzer count + pins the actual number in §13.4 BEHAVIOR_CONTRACT.md patch + ADR-0189 §Decision body.

### Cross-phase regression

- **R5: ADR-0177 `internal/httpclient/` first co-consumer validation** (per §2.6 + §2.17). The 22.2 IMPL `:httpCall()` task lands the first co-consumer of phase-20's `internal/httpclient/` primitive. RATIFIES the phase-20 framework-primitive extraction discipline (per ADR-0177 §Consequences forward-pointer). NOT a 22.1 item; settles at 22.2 IMPL.

### Sandbox + perf

- **R6: `*LState`-pool benchmark** (per §1.2 escape-valve candidate + §11.3.4). 22.1 IMPL benchmark task measures per-stream `*LState` construction cost at the headers-only bridge surface. If < 1ms: WEAK-default per-stream construction STANDS; no ADR-0190 fires. If > 1ms: ADR-0190 escape-valve consumed for `*LState` pool design.

### Wording-discipline

- **W (wording-pinning at envoy-go boot-reject)** (per §8.3 + R1). 22.1 IMPL late-task wraps gopher-lua's compile error with `"script load error: "` prefix at `cmd/envoy-go/main.go` boot-reject path. ~50 LoC delta. Constrains future PARSE-REJECT error wording at the bootstrap layer (forward-pointer for future filter PARSE-REJECT-at-config-load disciplines).

---

## 14. BEHAVIOR_CONTRACT.md edit bundle anticipation (per §9 delta + ADR-0052 atomic landing)

Per ADR-0052 in-place-edit authorization + phase-21 SPEC §13 + phase-18+19+20 precedent. The BEHAVIOR_CONTRACT.md gains its phase-22 content in three passes across the 3 sub-phases (one bundle per sub-phase IMPL final Task). The 22.1 IMPL final-Task **7-edit bundle** (per BRAINSTORM §1.1(j) + §5.4 + AMEND-3 corrected to 1-record stat-departure + AMEND-1 + AMEND-9 added departure records — net 7 edits unchanged from BRAINSTORM count):

1. **NEW `### envoy.filters.http.lua` subsection** under the §9 family filter documentation. Headers-bridge-focused for 22.1; carries forward-pointers to 22.2 (full bridge) + 22.3 (multi-script + per-route). ~80-120 lines.
2. **Stat-table 99 → 102 extension** under BEHAVIOR_CONTRACT.md `## Stat surface` (the table at line ~340-367). 3 new rows under `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template — `errors` (counter; upstream-parity); `executions` (counter; upstream-parity per AMEND-3); `respond_calls` (counter; envoy-go-strict extension per AMEND-3). ~3-line table-row insertion + extension summary paragraph mirroring phase-21's `## Phase 21 extension — 92 → 99 internal names` paragraph at line 367.
3. **envoy-go-strict departure record #1: stdlib-sandbox-strict** (per AMEND-1). NEW row at BEHAVIOR_CONTRACT.md §13.x "envoy-go-strict departures" table (or equivalent — phase-21 carries 2 such records at the time of writing). Records that envoy-go's strict default-deny sandbox departs from upstream's bare-`luaL_openlibs`-no-neutering posture; rationale: per-stream goroutine dispatch model cannot make the per-worker-VM-scoping assumption upstream relies on.
4. **envoy-go-strict departure record #2: `respond_calls` counter** (per AMEND-3 corrected from BRAINSTORM 2-record bundle). NEW row in same departures section. Operator-visibility rationale.
5. **envoy-go-strict departure record #3: runtime-error log-message wording** (per AMEND-9). NEW row in same departures section. gopher-lua's `[string "chunk"]:line: msg` format diverges from LuaJIT's `chunk:line: msg`; only the envoy log shows the divergent wording (wire never carries error strings).
6. **NEW `### Phase 22.1 forward-pointer notes` subsection**. Documents 22.2-anticipated additions (httpCall counters, body-bridge methods, dynamic-metadata-bridge disposition forward-pointer) + 22.3-anticipated additions (NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) AMENDMENT body + `Lua.SourceCodes` map activation). ~30-50 lines.
7. **Per-route-canonical cross-reference caption update** at the per-route canonical paragraph (BEHAVIOR_CONTRACT.md §x). Caption update only — the AMENDMENT body lands at 22.3 IMPL final-Task per ADR-0125 §(xiv). 1-line edit.

22.2 + 22.3 IMPL final-Task bundles anticipated (settled at 22.2 + 22.3 BRAINSTORM/SPEC):
- **22.2 bundle**: extends `### envoy.filters.http.lua` with body-stage detail; stat-table +N if httpCall counters land; ADR-0128 body-buffering cross-reference; additional envoy-go-strict departure records if dynamic-metadata-bridge defers structurally.
- **22.3 bundle**: extends with multi-script + per-route detail; ADR-0125 §(xiv) AMENDMENT body cross-reference; per-route 3-tier dispatch documentation; NEW 9th canonical per-route shape ADR cross-reference.

---

## 15. Test surface

### 15.1 Layer A: unit tests at `internal/filter/http/lua/` (22.1 IMPL)

- `lua_test.go` (~1500-2000 LoC anticipated): filter factory + filter struct + filterStats + `New` table-driven tests.
- `compiled_config_test.go`: 18-arm PARSE-REJECT roster table-driven tests per §6.2.
- `datasource_test.go`: 4-arm DataSource resolution unit tests (Filename / InlineBytes / InlineString / EnvironmentVariable) + WatchedDirectory PARSE-REJECT + empty-oneof PARSE-REJECT + file-read failure paths.
- `bridge_test.go`: bridge-method unit tests for the 7 headers-object methods + 6 `:logXxx` methods + 4-method `:streamInfo()` subset + `:respond()` with byte-pin assertions per §11.6.7.

### 15.2 Layer B: unit tests at `internal/lua/` (22.1 IMPL)

- `vm_test.go`: per-stream `*LState` construction + `RegisterBridgeMethod` + `Run` + `Close` table-driven tests; sandbox-config table-driven tests covering each ALLOW/DENY toggle per §4.3.
- `compile_test.go`: `NewCompileCache` + `CompileScript` + cache-hit-on-same-content-hash + cache-miss-on-different-source table-driven tests.
- `sandbox_test.go`: per-stdlib-module ALLOW/DENY exhaustive tests; verifies `dofile`/`loadfile`/`loadstring`/`require`/`io.open`/`io.popen`/`os.execute`/`os.exit`/`debug.getupvalue`/`channel.make`/`package.path` are nil-or-error post-sandbox-strict construction.

### 15.3 Layer C: 28th project-wide fuzzer `FuzzLuaConfigParse` (22.1 IMPL; count subject to §13-R4)

`fuzz_test.go` at `internal/filter/http/lua/`. Corpus seeds covering all 18 PARSE-REJECT arms per §6.2 + valid-config seeds. ~30 corpus seeds total at standard ADR-0018 baseline. Must-never-panic invariant covers gopher-lua compile error path (Arm 16 — adversarial Lua syntax must not crash the parser).

### 15.4 Layer D: differential fixture `0026-http-lua-headers-bridge` (22.1 IMPL)

Per §8.1-§8.5. 7 scenarios (a)-(g); scenarios (a)-(f) full cross-side byte-exact via existing `CompareBytes`; scenario (g) substring-match `"script load error"` via NEW OPTIONAL `BootRejectFixture` interface (per §13-R1).

### 15.5 Layer E: race + concurrency tests (22.1 IMPL)

`controller_test.go` equivalent at `internal/lua/` + `internal/filter/http/lua/`. Per-stream `*LState` construction + concurrent-fire-and-forget tests to verify no cross-stream state leak; sandbox-config thread-safety; compile-cache concurrent-read concurrent-add tests (sync.RWMutex discipline).

---

## 16. 22.1 IMPL acceptance checklist (18 items mirroring phase-21 §15)

Per phase-21 SPEC §15 18-item acceptance checklist precedent. The 22.1 IMPL Task that lands the package + tests + fixture + ADR landings + STATE.md re-advance MUST satisfy ALL of:

1. NEW `internal/lua/` package created with the API surface per §4.1 (VM + Chunk + CompileCache + SandboxConfig + VMOption types + WithSandboxConfig/WithPanicRecovery/WithStderrSink + NewVM + NewCompileCache + CompileScript + VM.RegisterBridgeMethod + VM.Run + VM.Close).
2. NEW `internal/filter/http/lua/` package created with files per §4.4.
3. `Lua.DefaultSourceCode` consumed; `Lua.SourceCodes` + `Lua.InlineCode` + `LuaPerRoute` PARSE-REJECTed per §6.2 arms 3 + 4 + 18.
4. 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT per §5.3 + §6.2 arms 6-15.
5. Pragmatic-middle bridge surface per Q6: `envoy_on_request` + `envoy_on_response` hooks + `:headers()` + 7 headers-object methods + `__pairs` metamethod + 6 `:logXxx()` methods + 4-method `:streamInfo()` subset + `request_handle:respond()` with §11.6.7 byte-pin.
6. Stdlib-sandbox-strict default-deny per §4.3 + AMEND-1 + envoy-go-strict departure record at BEHAVIOR_CONTRACT.md (per §14 edit #3).
7. Per-stream `*LState` construction + per-script-source `*Chunk` cache per §4.2 + §11.3.4.
8. 18-arm PARSE-REJECT roster per §6.2 (subject to §12-D1 disposition for arms 5 + 17).
9. 3-counter stat surface per §7 (`errors` + `executions` + `respond_calls`) under HCM-rooted template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per AMEND-2; 99 → 102 BEHAVIOR_CONTRACT.md update per §14 edit #2.
10. `respond_calls` envoy-go-strict counter departure record at BEHAVIOR_CONTRACT.md per §14 edit #4 + AMEND-3 (single record — corrected from BRAINSTORM 2-record bundle).
11. Runtime-error log-message wording envoy-go-strict departure record at BEHAVIOR_CONTRACT.md per §14 edit #5 + AMEND-9.
12. 28th project-wide fuzzer `FuzzLuaConfigParse` (count subject to §13-R4 verification) at standard ADR-0018 baseline; must-never-panic verified.
13. Differential fixture `0026-http-lua-headers-bridge` GREEN — 6 wire-interactive scenarios (a)-(f) full cross-side byte-exact via `CompareBytes`; scenario (g) substring-match `"script load error"` via NEW `BootRejectFixture` interface per §13-R1 + §8.3.
14. NEW `BackendKind=HTTPLua` constant added at `test/differential/runner_test.go:547` per AMEND-11; `internal/filter/http/lua/scripts/` subdirectory + 7 per-scenario `.lua` files per §8.4.
15. envoy-go-side `"script load error: "` wrapping at `cmd/envoy-go/main.go:60-66` boot-reject path per §13-W.
16. ADR-0188 §Decision + §Consequences body landed in DECISIONS.md (per the §Context anchor at this SPEC commit; ADR-0044 in-place edit discipline).
17. ADR-0189 §Decision + §Consequences body landed in DECISIONS.md (per the §Context anchor at this SPEC commit; includes all departure records + the §11.6.7 wire-shape + the §8.3 substring-match discipline + the §13-R1 BootRejectFixture interface).
18. STATE.md re-advance to `phase 22.1 IMPL done; awaiting 22.2 SPEC` + ROADMAP row 22.1 flipped `planned → done` per ADR-0106 per-cell IMPL-done annotation.

22.2 + 22.3 IMPL acceptance checklists settle at each sub-phase's own SPEC.

---

**End of phase 22 parent SPEC.**
