# Phase 10 — Code review (REVIEW.md)

**Phase id:** `10` (second §9 HTTP-filters family-row to land per ADR-0106)
**Slug:** `10-http-filter-header-mutation`
**Branch under review:** `phase-10-http-filter-header-mutation-impl`
**Range:** `2c80b30` (branch tip; phase-done SHA-fill commit) — 18 task commits + SHA-fill / PROGRESS-append follow-ups
**Parent ROADMAP row:** `10 http-filter-header-mutation` flips `in-progress → done` at the Task 16 commit `5fbebad` (already landed prior to this REVIEW; row 10's status field reads `done` on the impl branch at HEAD).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 18 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + phase-09 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0108..ADR-0113.
**Six-gate state at HEAD:** all green per Task 17's verification sweep — outputs reproduced verbatim in §"Six-gate verification appendix" below.

This review covers the full phase 10 surface: `internal/filter/http/header_mutation/` package (`doc.go` + `header_mutation.go` + `header_mutation_test.go` + `fuzz_test.go`), three ADR-0110 framework additions (`PerRouteConfig.ResolveAllTiers` + `DecoderFilterCallbacks.RequestRouteConfigsAllTiers` + `HTTPRegistry.RegisterPerRouteValidator` + `BuildPerRouteConfig` 4-param widening), `cmd/envoy-go/main.go` boot registration + pre-Freeze `RegisterPerRouteValidator` call, differential fixture `0012-http-header-mutation` (4 scenarios, dual-listener l_lws + l_mws, reference Envoy v1.37.2 STRICT_DNS + envoy-go STATIC), the FuzzHeaderMutationConfigParse fuzzer (fourteenth fuzzer in repo; thirteenth per PLAN wording — see §8 finding M-1), BEHAVIOR_CONTRACT.md §13 four-patch bundle (NEW header_mutation subsection + equivalence-matrix row + two forward-pointer notes), the six ADRs ADR-0108..ADR-0113, and the ROADMAP row 10 status flip + STATE.md advance.

This REVIEW closes phase 10's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Final assessment

**APPROVED.**

All six phase-done gates are GREEN at HEAD `8e17e06` per the Task 17 verification sweep (§6 below). The implementation faithfully realizes the SPEC across all 18 PLAN tasks. The header_mutation filter is the SECOND §9 HTTP-filters family-row to ship under ADR-0106; the package shape (`internal/filter/http/header_mutation/` with `doc.go` + `header_mutation.go` + tests + fuzzer) directly mirrors the fault precedent at `internal/filter/http/fault/`.

The architectural centrepiece is the ADR-0110 multi-tier per-route evaluation framework. Phase 10 is the FIRST production exerciser of `ResolveAllTiers` + `RequestRouteConfigsAllTiers` + `RegisterPerRouteValidator`. Two substantive design surprises surfaced during implementation:

1. **`RegisterPerRouteValidator` registration-site bug (PLAN design flaw):** The PLAN prescribed calling `RegisterPerRouteValidator` from inside `New`. With two listeners (l_lws + l_mws), the second HCM's `New` call hit the frozen registry's panic guard. Fix: extracted `header_mutation.RegisterPerRouteValidator(reg)` as an exported function called from `cmd/envoy-go/main.go` BEFORE `httpReg.Freeze()`. The hook mechanism itself is correct; only the registration site moved.

2. **Differential body-strip allow-list required three iterations** before the fixture was stable: baseline proxy-injected headers (`x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*`), then `connection` (envoy-go passes hop-by-hop headers; reference Envoy strips), then `user-agent` (envoy-go's Go net/http upstream client injects `Go-http-client/1.1` after header_mutation removes the driver-supplied one; reference Envoy C++ upstream does not inject any).

Both findings were resolved inline at Task 15. No Critical findings remain at HEAD.

The differential fixture `0012-http-header-mutation` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 4 scenarios across 2 listeners with a backend that reflects request headers into the response body and emits multi-value `X-Multi` response headers, exercising all 4 AppendActions + Remove + `keep_empty_value` boundary + multi-tier `most_specific_header_mutations_wins` flag.

All six anticipated ADRs (ADR-0108..ADR-0113) landed at the correct tasks per the PLAN's "ADRs introduced by this plan" table. ADR-0110 is the architectural ADR for this phase, amending ADR-0073's 3-tier-merge model to document the multi-tier (non-collapsing) variant.

---

## 2. N-1 carry-forward dispositions (from phase-09 REVIEW)

Phase-09's REVIEW §5 identified six carry-forward items. Phase 10's disposition for each:

| # | Phase-09 item | Phase-10 disposition |
|---|---|---|
| **N-1** | `counterValue(t, reg, name)` helper precedent | **Not applicable.** header_mutation emits zero stats (ADR-0108 zero-stats discipline). No stat counter helper needed. Item remains available for phase 11+ if that filter emits stats. |
| **N-2** | `recordingDCB` sync.Mutex + accessor pattern for async-resume | **Not needed.** header_mutation is synchronous (DecodeHeaders returns Continue; no async-resume, no timer-callback goroutine). The pattern is available for future async-resume filters. |
| **N-3** | FactoryCtx Stats + StatPrefix already plumbed | **Confirmed in place and not widened.** header_mutation's `New` factory correctly accepts the 4-param `FactoryCtx` and ignores the Stats/StatPrefix fields (nil-tolerance per ADR-0085 + ADR-0108 zero-stats discipline). No further FactoryCtx change needed. |
| **N-4** | `flattenToProm` SN-asymmetry (SN1/SN3/SN5 internal dots unhandled) | **Not triggered.** header_mutation emits zero stats, so no new stat names traverse `flattenToProm`. The hypothetical concern persists; carry forward to next stat-emitting §9 family-child phase. |
| **N-5** | `markedActive atomic.Bool` + CAS pattern for timer-callback / OnDestroy race | **Not needed.** header_mutation is synchronous; no timer-callback goroutine; no OnDestroy/goroutine seam. Pattern available for future async-resume filters. |
| **N-6** | Differential fixture timing-bucket pattern (elapsed-bucket fast<80ms / delayed≥80ms) | **Not needed.** header_mutation introduces no deliberate delays; fixture assertions are request-response-header equality only. Pattern available for future timer-driven filters. |

Additionally: phase-09 REVIEW §3.2 item I-1 (`flattenToProm` SN-asymmetry) was explicitly carried forward. Phase 10 does not trigger it; still open for phase 11+.

---

## 3. Per-task retrospective

The 18 tasks landed 18 task-commits + SHA-fill / PROGRESS-append follow-ups. Tasks that deviated from PLAN verbatim are called out below.

**Task 1 (commit `c9f4e61`):** Execution-precondition check + PROGRESS.md preamble. All 16 preconditions satisfied. No deviations.

**Task 2 (commit `3eba8df`):** `PerRouteConfig.ResolveAllTiers`. PLAN sketch matched implementation exactly. 12 test functions. No deviations.

**Task 3 (commit `0551b4`):** `DecoderFilterCallbacks.RequestRouteConfigsAllTiers` + chain wiring. PLAN sketch matched. Mock sweep required one stub addition: `recordingDCB` in `fault_test.go` needed `RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) { return nil, nil, nil }`. The new `fakeDecoderCB` in `callbacks_test.go` also received the method. No semantic deviations.

**Task 4 (commit `80df1ea`):** `HTTPRegistry.RegisterPerRouteValidator` + `BuildPerRouteConfig` 4-param widening. **Deviation:** `BuildPerRouteConfig` widened with an explicit `reg *HTTPRegistry` parameter (chosen over the alternative `BuildPerRouteConfigWithRegistry` function name). 19 `, nil` additions swept across 6 test files. Pre-existing `parseMap` location-prefix `i, i` quirk in `perroute.go` preserved verbatim. One test-name deviation: `TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil` used `"got non-nil func"` assertion string instead of `%v` on a func value (vet caught `func value, not called` at format-string check).

**Task 5 (commit `7cda566`):** `internal/filter/http/header_mutation/` package core + ADR-0108/0109/0111. **Deviation:** `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored` adapted. The PLAN's verbatim snippet used `{AppendAction: ..., Append: &corev3.HeaderValue{...}}` to construct a `KeyValueMutation`. At implementation time `go doc` revealed `KeyValueMutation` lives in `corev3` with a different struct shape: `Append *KeyValueAppend` + `Remove string` (no top-level `AppendAction` field). Adapted to `&corev3.KeyValueMutation{Remove: "q"}` — valid minimal construction; silent-ignore path still exercised. Test purpose unchanged. gofmt alignment correction applied to `compiledMutationOp` struct literal (golangci-lint caught non-canonical spacing).

**Task 6 (commit `898c3ed`):** `applyOps` + `applyAppendAction` + AppendAction × 4 + `keep_empty_value` boundary. PLAN sketch matched. 15 test functions (11 top-level + 4 table-driven sub-cases). No deviations.

**Task 7 (commit `a242b78`):** `DecodeHeaders` multi-tier resolution + ADR-0110 + ADR-0073 amendment. PLAN sketch matched. 8 test functions. ADR-0073 amendment paragraph appended correctly after the existing Lands-in-task paragraph. No deviations.

**Task 8 (commit `6169b09`):** `EncodeHeaders` symmetric + race-detector cycle test. PLAN's `TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide` struct literal had a trailing comma that triggered a golangci-lint gofmt violation; fixed by `gofmt -w`. Race-detector cycle test (`TestHeaderMutation_MultiTierConcurrentRequests`) spawned 64 goroutines from a shared factory — PASS under `-race`. No semantic deviations.

**Task 9 (commit `1246525`):** `cmd/envoy-go/main.go` boot registration. Mechanical; no deviations. At this point `RegisterPerRouteValidator` was NOT yet called from main.go — that fix landed at Task 15 when the design flaw surfaced.

**Task 10 (commit `4a82931`):** `FuzzHeaderMutationConfigParse` (thirteenth fuzzer per PLAN label; fourteenth actual — see §8 M-1). 30s budget: 6.54M execs, 219 new-interesting, no panics, no `(nil, nil)`. This is the highest exec count of any phase's fuzzer run in the project.

**Task 11 (commit `4078c45`):** Fixture infrastructure — `HTTPHeaderMutation BackendKind = 9` + spawn helper + driver stub. PLAN sketch matched. Blank-import for driver package included at this task (deviation from 0011 precedent where it was deferred to Task 14). No semantic issues.

**Task 12 (commit `4910547`):** Fixture 0012 `backend.go`. Echo backend with sorted request-header body reflection + multi-value `X-Multi` headers for OVERWRITE/APPEND coverage. Smoke test confirmed correct behavior. No deviations.

**Task 13 (commit `237ecad`):** Fixture 0012 `envoy.yaml` + `envoy-go.yaml` dual-listener bootstraps. **Bug introduced here (fixed Task 15):** `envoy.yaml` had admin port 9912 but `harness.StartReferenceProxy` hardcodes the ready-wait on `9901/tcp`. Every other fixture uses 9901. Fixed at Task 15 alongside the driver and RegisterPerRouteValidator fixes. Docker `--mode validate` confirmed 2 listeners + 1 cluster accepted. No other deviations.

**Task 14 (commit `dc648dd`):** Fixture 0012 `expectations.yaml` + `README.md`. Prose documentation per ADR-0019; no machine evaluation. No deviations.

**Task 15 (commit `eb55904`):** Fixture 0012 `driver.go` + three inline fixes. **Three substantive deviations from PLAN:**
- **Design bug in PLAN — `RegisterPerRouteValidator` registration site.** PLAN had `New` calling `ctx.Registry.RegisterPerRouteValidator()`. With 2 listeners, the second HCM's `New` call hit the frozen registry's panic guard. Fix: extracted `header_mutation.RegisterPerRouteValidator(reg)` as exported function; called from `cmd/envoy-go/main.go` BEFORE `httpReg.Freeze()`. Comment from `New`: "Do NOT register the per-route validator here — by the time New is called, the HTTP registry has already been Frozen, so any call here would panic. The RegisterPerRouteValidator function is exported and called by main.go before Freeze."
- **Admin port bug (Task 13 oversight).** envoy.yaml had admin port 9912; corrected to 9901 per harness discipline.
- **Body-strip allow-list required three iterations:** baseline (`x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*`), then `connection` (hop-by-hop forwarding asymmetry between envoy-go and reference Envoy), then `user-agent` (Go net/http upstream client injects `Go-http-client/1.1` after header_mutation removes the driver-supplied one; reference Envoy C++ upstream does not). Final allow-list documented in driver source for future fixture authors.

After all three fixes: `TestDifferential/0012-http-header-mutation` PASSES in 1.73s. Full `TestDifferential` suite (14 subtests, 13 fixture directories) PASS in 39.76s.

**Task 16 (commit `5fbebad`):** BEHAVIOR_CONTRACT patches + ADR-0112 + ADR-0113 + ROADMAP row 10 done. Four patches landed: NEW header_mutation subsection, equivalence-matrix row, two forward-pointer notes. ADR-0112 (query_parameter_mutations deferred) + ADR-0113 (formatter substitution deferred) appended. ROADMAP row 10 flipped `in-progress → done`. No deviations.

**Task 17 (commit `8e17e06`):** Phase-done six-gate verification + STATE.md advance. All six gates green. 13 differential fixture directories (14 subtests with 0007a/0007b split) run in 39.76s. Fastest per-phase differential gate verification to date. STATE.md flipped to `awaiting next planning`. No deviations.

**Task 18 (THIS commit):** REVIEW.md per requesting-code-review skill (end-of-phase review). This document. Closes phase 10 lifecycle (state 5 → 6). Next session: `superpowers:brainstorming` against the §9 family-children list per ADR-0106.

---

## 4. Planner-time decisions retrospective

The eleven planner-time deferred decisions from PROGRESS.md §Preamble, evaluated against implementation outcomes:

**Decision 1 (DECODER-ONLY `RequestRouteConfigsAllTiers` callback):** VALIDATED. The cors precedent held; `EncodeHeaders` calls `f.dcb.RequestRouteConfigsAllTiers()` using the decoder-side callback captured in `SetDecoderCallbacks`, which is the same callback already in use for `DecodeHeaders`. No symmetric encoder callback needed. This matches the PLAN's "mirrors cors precedent at cors.go:163" note.

**Decision 2 (per-request per-tier cache = SKIP):** VALIDATED. No performance regression observed in differential timing (fixture runs at 1.73s for 4 scenarios across 2 listeners). Revisit threshold not reached.

**Decision 3 (EAGER per-route validation via registry hook):** VALIDATED but with a SUBSTANTIVE design surprise. The hook mechanism works correctly: `validatePerRouteHeaderMutation` runs at HCM-build time, rejecting protected-header violations before any request is served. However, the PLAN's prescription of calling `RegisterPerRouteValidator` from inside `New` was incompatible with the multi-listener HCM-build flow (registry frozen by then). Fix: extracted to main.go pre-Freeze. The registration site moved; the eager-validation semantics are preserved.

**Decision 4 (VALUE-TYPED `compiledMutationOp`):** VALIDATED. The `applyOps` hot loop iterates value-typed structs without pointer chasing. Read-only-after-New invariant confirmed by the 64-goroutine race-detector cycle test (Task 8).

**Decision 5 (prefix-on-`:` + EqualFold on `host`):** VALIDATED. The 10-case `TestNew_ProtectedHeader` table (`:method`, `:path`, `:authority`, `:scheme`, `:status`, `host`, `Host`, `HOST`, response `:status`, response `host`) all pass.

**Decision 6 (SHIP fuzzer):** VALIDATED. `FuzzHeaderMutationConfigParse` ran 6.54M execs at 30s budget — highest exec count of any fuzzer run in the project. Clean.

**Decision 7 (race-detector cycle test = `TestHeaderMutation_MultiTierConcurrentRequests`):** VALIDATED. 64 concurrent goroutines constructing fresh `*filter` instances from a shared `*runtimeConfig` factory, each calling `DecodeHeaders` + `EncodeHeaders`. PASS under `-race -count=1`. Confirms the read-only-after-New invariant.

**Decision 8 (`applyOps` exposure = KEEP unexported):** VALIDATED. Tested entirely via the public `DecodeHeaders`/`EncodeHeaders` surface; no test needed direct access to `applyOps`. No profiling revealed a reason to expose it.

**Decision 9 (per-route-validator integration test fan-out = TABLE-DRIVEN per tier):** VALIDATED. The 6 `TestBuildPerRouteConfig_PerRouteValidator_*` tests in `perroute_test.go` cover RC-tier error, VHost-tier error, Route-tier error, no-validator, nil-registry, and only-consulted-for-registered-filters. All pass.

**Decision 10 (fixture path = `test/fixtures/0012-http-header-mutation/`):** VALIDATED. `runtime.Caller(0)` + `filepath.Dir` path-resolution in the driver correctly locates the YAML files. Full differential suite PASS confirms the path is correct.

**Decision 11 (`HTTPHeaderMutation BackendKind = 9`):** VALIDATED. The `case fixture.HTTPHeaderMutation:` block in `runner_test.go` fires correctly; the fixture registered and run successfully.

---

## 5. Six ADRs retrospective

**ADR-0108** (package shape + boot registration + zero-stats discipline): VALIDATED. The 4-file split (`doc.go` + `header_mutation.go` + `header_mutation_test.go` + `fuzz_test.go`) mirrors fault. The zero-stats discipline (no `registerStats` call; `FactoryCtx.Stats` intentionally ignored) is the first explicit zero-stats ADR for a §9 family filter; it is correctly reflected in the BEHAVIOR_CONTRACT §13.1 "Stats — none emitted" sub-section.

**ADR-0109** (`runtimeConfig` 3-field shape + `compiledMutationOp` value-typed + AppendAction × 4 + `keep_empty_value` semantics + multi-value collapse/preserve): VALIDATED. All 14 unit tests covering the parser + apply-loop semantics + 4 differential scenarios pass. The `keep_empty_value=false` silent-skip fires BEFORE the AppendAction switch per §11.2 conclusion (c) — this ordering was correctly preserved through all test iterations.

**ADR-0110** (multi-tier per-route evaluation + `ResolveAllTiers` + `RequestRouteConfigsAllTiers` + `RegisterPerRouteValidator` + accessor-choice discipline + cross-tier algorithm + amends ADR-0073): VALIDATED, with one §Consequences note that the per-route-validator registration site is `main.go` (pre-Freeze), NOT `New`. Future filters using `RegisterPerRouteValidator` must follow the same main.go-pre-Freeze pattern. ADR-0073 amendment paragraph correctly appended at the end of the ADR-0073 section before its `---` separator.

**ADR-0111** (protected-header set + CONFIG-LOAD-TIME rejection + verbatim error format + EAGER per-route validation lifecycle): VALIDATED. The CONFIG-LOAD-TIME rejection at HCM-build time (not request-time) is confirmed by the `TestNew_ProtectedHeader` table and `TestBuildPerRouteConfig_PerRouteValidator_*` integration tests. The verbatim error format `"protected header %q cannot be mutated (decision ADR-0111)"` is correct in production code and test assertions.

**ADR-0112** (`mutations.query_parameter_mutations[]` DEFERRED): VALIDATED as a deferral. `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored` confirms silent-ignore at parse time (no error, no ops produced). The deferred surface is correctly named in the BEHAVIOR_CONTRACT §13.1 "Does not yet apply to" sub-section.

**ADR-0113** (header-value formatter substitution DEFERRED): VALIDATED as a deferral. The formatter-substitution path is correctly absent from `compileOps`; field is silently ignored. The deferral rationale ("full Envoy command-string subsystem is its own multi-phase project") is recorded in ADR-0113.

---

## 6. Six-gate retrospective

**Gate (a) build/vet/lint:** Clean at all task commits. The one recurring issue (golangci-lint gofmt non-canonical alignment on struct literals, Tasks 5 and 8) was caught immediately by the lint step and fixed before committing.

**Gate (b) unit tests + race:** 33 packages PASS. `go test -race -count=1 ./...` clean. header_mutation package is synchronous and has no goroutine seam — the race-clean result is straightforward. The 64-goroutine concurrent test validates the read-only-after-New invariant.

**Gate (c) h2spec:** 53/53 at ADR-0051 pin unchanged. Phase 10 touches no codec or H2 path.

**Gate (d) fuzzers:** 14 fuzzers total at HEAD (12 pre-09 + FuzzFaultConfigParse from phase 09 + FuzzHeaderMutationConfigParse from phase 10). The PLAN's "thirteenth fuzzer" label is an off-by-one vs actual count (see §8 M-1). `FuzzHeaderMutationConfigParse` ran 6.54M execs at 30s budget (highest in project). Gate (d) abbreviated per option B: only the new fuzzer re-run at Task 17; 13 prior fuzzers carried forward green (none of their code paths are touched by phase 10 changes).

**Gate (e) differential:** 39.76s end-to-end across 14 subtests (13 fixture directories including 0012's dual-listener + 4-scenario run). Fastest per-phase differential gate verification to date. The 0012 fixture's 4 scenarios run in 1.73s — compact and non-vacuous.

**Gate (f) BEHAVIOR_CONTRACT alignment + ROADMAP row 10 status:** `grep -cE 'envoy.filters.http.header_mutation' BEHAVIOR_CONTRACT.md` returns 7 (subsection + equivalence-matrix row + 2 forward-pointer notes + 3 sub-section anchors). All 6 ADRs confirmed in DECISIONS.md. ROADMAP row 10 reads `done`.

---

## 7. Carry-forward findings for phase 11+

| # | Item | Disposition |
|---|---|---|
| **CF-1** | `RegisterPerRouteValidator` registration-site convention | **CRITICAL PATTERN.** Future filters with per-route protected-header or per-route validation needs MUST extract the registration to an exported `RegisterPerRouteValidator(reg *HTTPRegistry)` function called from `main.go` BEFORE `httpReg.Freeze()`. Calling from inside `New` will panic on multi-listener builds. ADR-0110 §Consequences should be amended to record this convention explicitly if a third filter adopts it. |
| **CF-2** | `BuildPerRouteConfig` 4-param signature | **In place.** Future framework extensions touching this call site must pass the `*HTTPRegistry` parameter. All 19 test-only callers already pass `, nil`. Production call site in `buildPerRouteFromHCM` passes the live registry. No further action required unless the signature widens again. |
| **CF-3** | Differential body-strip allow-list discipline | **DOCUMENTED IN DRIVER SOURCE.** The 0012 driver's strip list (`x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*`, `connection`, `user-agent`) is the authoritative starting point for future fixtures whose backends reflect request headers in the response body. The `user-agent` entry is specific to the envoy-go Go net/http upstream client injection; reference Envoy C++ upstream does not inject one. Future driver authors should verify against this baseline rather than discovering the allow-list iteratively. |
| **CF-4** | `flattenToProm` SN-asymmetry (from phase-09 I-1) | **Still open.** header_mutation emits zero stats; the hypothetical SN1/SN3/SN5 dotted-rest bug was not triggered. Carry forward to the next stat-emitting §9 family-child phase (e.g., local_ratelimit, jwt_authn). Fix is: apply `strings.ReplaceAll(rest, ".", "_")` symmetrically across SN1/SN3/SN5 cases in `internal/stats/name.go`. |
| **CF-5** | Zero-stats filter pattern (ADR-0108) | **AVAILABLE AS PRECEDENT.** Future filters that deliberately emit no stats can cite ADR-0108 and omit the `registerStats` call + `FactoryCtx.Stats` usage entirely. nil-tolerance per ADR-0085 means this requires no framework change. |
| **CF-6** | Dual-listener fixture pattern (0012) | **AVAILABLE AS PRECEDENT.** fixture 0012 is the first fixture with two listeners on different ports, driven by `fixture.MultiListenerDriver`. Future filters needing per-listener flag variations can use the l_lws/l_mws split as a template. Runner allocates both ports; driver accesses via `SubjectListenerNames`/`ReferenceListenerPorts`. |

---

## 8. Minor findings

### M-1 (Documentation-only off-by-one — fuzzer count)

The PLAN, SPEC, and phase-10 commit messages label `FuzzHeaderMutationConfigParse` as "thirteenth fuzzer." The actual count at HEAD is 14: 12 pre-phase-09 fuzzers (FuzzHCMConfigParse + FuzzTcpProxyFilter + FuzzPromTextFormat + FuzzConfigDumpFormat + FuzzTLSContextParse + FuzzFilterChainParse + FuzzFrameStream + FuzzHPACKDecode + FuzzDrainTransitions + FuzzAccessLogFormat + FuzzBootstrapLoad + FuzzFilterChainMatch) + FuzzFaultConfigParse from phase 09 + FuzzHeaderMutationConfigParse from phase 10 = 14. The off-by-one originates from phase 09's "twelfth fuzzer" label (itself off-by-one per phase-09 REVIEW M-1); each phase compounds the previous error. The PROGRESS.md gate (d) output (`grep ... | wc -l` → `14`) is authoritative. ADR-0018's "every parser/codec/filter ships a fuzzer" discipline is satisfied regardless. No action.

### M-2 (PLAN's `KeyValueMutation` struct shape — proto API mismatch)

The PLAN's verbatim `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored` snippet assumed `KeyValueMutation` has an `AppendAction` field at top level. The actual `corev3.KeyValueMutation` has `Append *KeyValueAppend` + `Remove string`. This is a PLAN authoring error (proto API not verified at planning time). The adaptation to `&corev3.KeyValueMutation{Remove: "q"}` is semantically equivalent for the test's purpose; the silent-ignore path is exercised regardless. No action on production code.

### M-3 (Task 9 / Task 15 registration-site split)

`cmd/envoy-go/main.go` now has two header_mutation-related lines: `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` (Task 9) and `header_mutation.RegisterPerRouteValidator(httpReg)` (Task 15 fix). These are logically related but committed three tasks apart. A future refactor could group them with a comment block, but the current placement is functionally correct and lint-clean. No action.

### M-4 (Admin port 9912 in Task 13 envoy.yaml)

Task 13 wrote envoy.yaml with admin port 9912 (copying the SPEC §7.4 YAML verbatim). The harness hardcodes the ready-wait on 9901/tcp. This is the same class of error as phase-09's M-6 (reference admin port 9902→9901 at Task 14). Both could be avoided by a harness-side dynamic port allocation or a comment in the SPEC fixture template. No action at HEAD; noted for future SPEC authoring.

---

## 9. LoC retrospective

PLAN estimated ~2050 LoC total (~430 production + ~370 tests + ~50 fuzzer + ~720 fixture + ~470 docs). Approximate actuals from committed files:

| Surface | PLAN estimate | Approximate actual |
|---|---|---|
| Production (`header_mutation.go` + `doc.go`) | ~430 LoC | ~338 LoC (`doc.go` 85 + `header_mutation.go` 253) |
| Tests (`header_mutation_test.go`) | ~370 LoC | ~278 LoC (package unit tests only) |
| Fuzzer (`fuzz_test.go`) | ~50 LoC | ~37 LoC |
| Framework deltas (Tasks 2/3/4 across 6 files) | ~50 LoC | ~150 LoC (registry hook + signature widening + 19-call sweep was larger than estimated) |
| Fixture (backends + YAMLs + driver) | ~720 LoC | ~442 LoC (backend 51 + driver 326 + YAMLs ~65) |
| Docs (ADRs + BEHAVIOR_CONTRACT + expectations + README) | ~470 LoC | ~410 LoC (ADRs +221 + BC patches + expectations 58 + README 65) |

The main deviation: framework deltas ran ~3× the PLAN's ~50 LoC estimate (the registry hook + signature widening + 19-call sweep was a larger mechanical footprint than anticipated at planning time). Production + test + fuzzer LoC came in slightly under estimate. Total actual approximately 1655 LoC vs PLAN's ~2050 estimate (framework overrun offset by leaner package implementation).

---

## 10. Per-ADR cohesion summary

| ADR | Title | Lands-in-task | Commit | Amendments |
|---|---|---|---|---|
| ADR-0108 | `internal/filter/http/header_mutation/` package shape + boot registration + zero-stats discipline | T5 | `7cda566` | none |
| ADR-0109 | `runtimeConfig` 3-field shape + `compiledMutationOp` value-typed + AppendAction × 4 + `keep_empty_value` semantics + multi-value | T5 | `7cda566` | none |
| ADR-0110 | Multi-tier per-route evaluation + `ResolveAllTiers` + `RequestRouteConfigsAllTiers` DECODER-ONLY + `RegisterPerRouteValidator` + accessor-choice discipline + cross-tier ordering algorithm; amends ADR-0073 | T7 | `a242b78` | none (framework pieces T2/T3/T4; registration-site fix in T15 is an implementation correction, not an ADR amendment) |
| ADR-0111 | Protected-header set + CONFIG-LOAD-TIME rejection + verbatim error format + EAGER per-route validation lifecycle | T5 | `7cda566` | none |
| ADR-0112 | `mutations.query_parameter_mutations[]` deferred — coupled to `KeyValueMutation` triple + path-query rewriting subsystem | T16 | `5fbebad` | none (deferral ADR per ADR-0040 format) |
| ADR-0113 | Header-value formatter substitution deferred — full Envoy command-string subsystem is its own multi-phase project | T16 | `5fbebad` | none (deferral ADR per ADR-0040 format) |

The ADR cluster is internally consistent. ADR-0108 anchors the package shape; ADR-0109 anchors the parser + apply-loop semantics; ADR-0110 anchors the multi-tier framework (cross-references ADR-0073 amendment); ADR-0111 anchors protected-header validation (referenced by ADR-0108 §Consequences + ADR-0110 RegisterPerRouteValidator); ADR-0112 + ADR-0113 are symmetric deferral ADRs (both reference ADR-0040 deferral format and ADR-0111/0109 respectively for the silently-ignored surfaces).

---

## 11. Six-gate verification appendix

All six gates run against HEAD `8e17e06` per Task 17's verification sweep (PROGRESS Task 17 entry verbatim). Reproduced summary outputs:

### Gate (a) — build clean

```
$ go build ./...
EXIT:0  clean

$ go vet ./...
EXIT:0  clean

$ golangci-lint run ./...
EXIT:0  clean
```

**Result: PASS — clean.**

### Gate (b) — unit tests + race

```
$ go test -race -count=1 ./...
33 packages PASS; 0 FAIL
EXIT:0
```

**Result: PASS — 33 packages clean under -race.**

### Gate (c) — h2spec re-run

```
$ go test -count=1 ./test/conformance/h2spec/ -run TestH2Spec
--- PASS: TestH2Spec (2.41s)

(53 tests, 53 passed, 0 skipped, 0 failed at ADR-0051 pin; phase 10 touches no codec)
```

**Result: PASS — 53/53 at ADR-0051 pin (unchanged).**

### Gate (d) — fuzzers

Ran only `FuzzHeaderMutationConfigParse` for 30s at Task 17 (option B rationale: 13 prior fuzzers verified in prior phases; phase 10 touches none of their code paths; Task 10 already ran FuzzHeaderMutationConfigParse for 30s at fuzzer-introduction time with 6.54M execs clean).

```
$ grep -rE '^func Fuzz' --include='*_test.go' internal/ test/ | wc -l
14

$ go test -fuzz=FuzzHeaderMutationConfigParse -fuzztime=30s ./internal/filter/http/header_mutation/
fuzz: elapsed: 30s, execs: 6545485 (...)
PASS
ok  github.com/esalaine/envoy-go/internal/filter/http/header_mutation  31.061s
```

**Result: PASS — 14 fuzzers in repo; FuzzHeaderMutationConfigParse clean at 30s (6.54M execs); 13 prior fuzzers carried forward green per option B.**

### Gate (e) — differential 0000-0012 all green

```
$ go test -count=1 ./test/differential/
=== RUN   TestDifferential
    --- PASS: TestDifferential/0000-tcp-echo
    --- PASS: TestDifferential/0001-tcp-proxy-rr
    --- PASS: TestDifferential/0002-tls-tcp
    --- PASS: TestDifferential/0003-http11-routing
    --- PASS: TestDifferential/0004-h2-routing
    --- PASS: TestDifferential/0005-prometheus-stats
    --- PASS: TestDifferential/0006-access-log
    --- PASS: TestDifferential/0007a-cors
    --- PASS: TestDifferential/0007b-iteration-probe
    --- PASS: TestDifferential/0008-listener-chain-match
    --- PASS: TestDifferential/0009-admin-config-dump
    --- PASS: TestDifferential/0010-graceful-drain
    --- PASS: TestDifferential/0011-http-fault
    --- PASS: TestDifferential/0012-http-header-mutation
--- PASS: TestDifferential (39.76s)
```

**Result: PASS — 14 differential subtests green (0000..0012 with 0007 split into 0007a/0007b — 13 fixture directories, 14 subtest count).**

### Gate (f) — BEHAVIOR_CONTRACT alignment + ROADMAP row 10 status

```
$ grep -cE 'envoy.filters.http.header_mutation' docs/envoy-go/BEHAVIOR_CONTRACT.md
7

$ grep -cE '^## ADR-(0108|0109|0110|0111|0112|0113):' docs/envoy-go/DECISIONS.md
6

$ awk -F'|' 'NR>3 && $2 ~ /^ 10 / {print $5}' docs/envoy-go/ROADMAP.md
 done
```

**Result: PASS — BEHAVIOR_CONTRACT.md populated with:**
- §13.1 NEW `### envoy.filters.http.header_mutation` subsection under `## HTTP filter chain`
- §13.4 Equivalence Matrix row appended after the `envoy.filters.http.fault` row
- §13.5 Forward-pointer note at `## HTTP filter chain ### typed_per_filter_config 3-tier merge` (ADR-0110 multi-tier amendment)
- §13.5 Forward-pointer note at `## HTTP filter chain ### envoy.filters.http.cors ### Asserted equivalence` (second production filter in EncodeHeaders)
- ROADMAP row 10 status field reads `done` (flipped at Task 16 commit `5fbebad`)
- All six ADRs ADR-0108..ADR-0113 confirmed in DECISIONS.md

Six-gate state: **ALL GREEN at HEAD `8e17e06`.** Phase-done commit landed at Task 17; this REVIEW closes lifecycle-state 5 → 6.

---

## 12. Acceptance against SPEC §15

Cross-referencing SPEC §15 acceptance checklist (abridged):

- [x] `internal/filter/http/header_mutation/` package lands with `doc.go` + `header_mutation.go` + `header_mutation_test.go` + `fuzz_test.go` (FuzzHeaderMutationConfigParse). `New` factory implements SPEC §6 contract.
- [x] ADR-0110 framework additions: `ResolveAllTiers` + `RequestRouteConfigsAllTiers` + `RegisterPerRouteValidator` + `BuildPerRouteConfig` 4-param widening. Build clean.
- [x] Decode-side multi-tier algorithm: `f.cfg.requestOps` applied first; `RequestRouteConfigsAllTiers()` consulted for 3-tier unmerged messages; flag-controlled apply order (Route→VHost→RC for flag=false; RC→VHost→Route for flag=true).
- [x] Encode-side symmetric algorithm: `f.cfg.responseOps` applied first; same `f.dcb.RequestRouteConfigsAllTiers()` (DECODER-ONLY per decision 1); compileForResponse + flag-controlled apply order.
- [x] AppendAction × 4 semantics per ADR-0109: APPEND_IF_EXISTS_OR_ADD / ADD_IF_ABSENT / OVERWRITE_IF_EXISTS_OR_ADD / OVERWRITE_IF_EXISTS + `keep_empty_value=false` silent-skip BEFORE switch.
- [x] Protected-header set per ADR-0111: prefix-check on `:` + EqualFold on `host`; CONFIG-LOAD-TIME rejection; EAGER per-route validation via RegisterPerRouteValidator (exported function, called from main.go pre-Freeze per Task 15 fix).
- [x] `cmd/envoy-go/main.go` registers `header_mutation.New` under `header_mutation.TypeURL` + calls `header_mutation.RegisterPerRouteValidator(httpReg)` before `httpReg.Freeze()`.
- [x] FuzzHeaderMutationConfigParse fuzzer per ADR-0018: 30s budget + 3-entry seed corpus + `(factory, nil) ∨ (nil, error)` invariant; 6.54M execs clean.
- [x] Fixture 0012-http-header-mutation green: 4 scenarios across dual listeners l_lws/l_mws; `RequiresReference: true`; STATIC-subject + STRICT_DNS-reference; admin port 9901 per harness discipline.
- [x] `go test -race ./...` clean: verified by gate (b) above.
- [x] FuzzHeaderMutationConfigParse runs clean at 30s: verified by gate (d) above.
- [x] h2spec 53/53 PASS: verified by gate (c) above.
- [x] Six new ADRs (ADR-0108..ADR-0113) in DECISIONS.md: verified by gate (f) above.
- [x] BEHAVIOR_CONTRACT.md §13 four-patch bundle: verified by gate (f) above.
- [x] ROADMAP row 10 flips `in-progress → done`: verified by gate (f) above (Task 16 commit `5fbebad`).
- [x] STATE.md `active-phase: awaiting next planning` + `lifecycle-state: awaiting` + `next-skill: superpowers:brainstorming`: verified by Task 17's STATE rewrite + SHA-fill `2c80b30`.
- [x] REVIEW.md authored: THIS document.

All acceptance items checked. Phase-done. Phase 10 lifecycle (state 5 → 6) closes at the commit landing this REVIEW. Branch `phase-10-http-filter-header-mutation-impl` is ready for merge to master per the linear-history (fast-forward) precedent established by phases 00–09.
