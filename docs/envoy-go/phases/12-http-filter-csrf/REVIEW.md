# Phase 12 — Code review (REVIEW.md)

**Phase id:** `12` (fifth §9 HTTP-filters family-row to land per ADR-0106; FIRST production filter to demonstrate "wholesale data-only override + shared per-route stats" — diverges from phase 11 ADR-0117 INDEPENDENT-stats precedent)
**Slug:** `12-http-filter-csrf`
**Branch under review:** `phase-12-http-filter-csrf-impl`
**Range:** `2706168` (branch tip; phase-done SHA-fill follow-up to `4f4ed39`) — 13 task commits + SHA-fill / PROGRESS-append follow-ups
**Parent ROADMAP row:** `12 http-filter-csrf` flipped `in-progress → done` at the Task 12 commit `4f4ed39` (already landed prior to this REVIEW; row 12's status field reads `done` on the impl branch at HEAD).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 13 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + phase-11 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0120..ADR-0124 + phase 11 REVIEW carry-forward items.
**Six-gate state at HEAD:** all green per Task 12's verification sweep — outputs reproduced verbatim in the Six-gate retrospective section below.

This review covers the full phase 12 surface: `internal/filter/http/csrf/` package (`doc.go` + `csrf.go` + `csrf_test.go` + `fuzz_test.go`), one load-bearing HCM framework fix (`internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` `:authority` pseudo-header injection — see headline retrospective below), `cmd/envoy-go/main.go` boot registration, differential fixture `0014-http-csrf` (6 scenarios, single-listener + two-route topology, reference Envoy v1.37.2 STRICT_DNS + envoy-go STATIC), `FuzzCsrfPolicyConfigParse` (sixteenth fuzzer in repo), BEHAVIOR_CONTRACT.md §13 four-edit bundle (NEW csrf subsection + 26→29 stat-name table extension + equivalence-matrix row + Phase 12 forward-pointer notes), the five ADRs ADR-0120..ADR-0124, and the ROADMAP row 12 status flip + STATE.md advance.

This REVIEW closes phase 12's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.**

All six phase-done gates are GREEN at HEAD `4f4ed39` per the Task 12 verification sweep (Six-gate retrospective section below). The implementation faithfully realizes the SPEC across all 13 PLAN tasks. The csrf filter is the FIFTH §9 HTTP-filters family-row to ship under ADR-0106 and the first to demonstrate the "wholesale data-only override + shared stats" pattern (per-route TPFC carries `[]compiledOrigin` only; `*filterStats` pointer is shared with the listener-level config) — diverging structurally from phase 11 ADR-0117's INDEPENDENT-stats discipline.

The architectural centerpiece is the §11.9 amendment + ADR-0124 design: csrf has no stateful per-route resources (no token-bucket analog), so per-route runtime can SHARE the `*filterStats` pointer. This makes csrf the precedent for any future data-only-override filter (e.g., header-injection variants, mutator-style filters). The empirical-pin discipline produced two MAJOR amendments at SPEC drafting (origin comparison shape + `filter_enabled` PGV-required) which both held under implementation without further adjustment.

The differential fixture `0014-http-csrf` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 6 scenarios on a single listener with two routes (default `/` + per-route `/route-only`), exercising same-origin allow (scenario 1), cross-origin reject byte-exact 403 + 14-byte `Invalid origin` body + 4-header lowercase wire-form (scenario 2), additional_origins exact-match (scenario 3), missing-source-origin reject (scenario 4), Referer fallback (scenario 5), and per-route wholesale override with SHARED stats (scenario 7a/7b). PASSES first-try after the Task 11 HCM `:authority` injection fix.

All five anticipated ADRs (ADR-0120..ADR-0124) landed at the correct tasks per the PLAN's "ADRs introduced by this plan" table.

---

## 2. N-1 carry-forward dispositions (from phase-11 REVIEW)

Phase-11's REVIEW §7 identified eight carry-forward items. Phase 12's disposition for each:

| # | Phase-11 item | Phase-12 disposition |
|---|---|---|
| **CF-1** | `Registry.NewCounterIfAbsent` reusability | **Not exercised.** csrf has SHARED per-route stats per §11.9 + ADR-0124, so no post-Freeze idempotent registration is needed; per-route runtimeConfig holds a pointer-copy of the listener-level `*filterStats`. The phase-11 primitive remains available; future stateful per-route filters reuse it. Ackowledged as available; not consumed. |
| **CF-2** | `tokenBucket` primitive reusability | **Not exercised.** csrf has no rate-limit-style state; primitive remains unexported in `localratelimit/bucket.go`. Future filters that need a token bucket should consider extraction (per phase 11 CF-2) at that point. |
| **CF-3** | Deferred field families to schedule | **Partially advanced; csrf adds its own deferrals.** Of the 8 deferred local_ratelimit clusters, the Runtime + hot restart family (cluster 2) is now ALSO blocking csrf's `filter_enabled` percentage-gating + `shadow_enabled` shadow-mode evaluation. Promoted in priority via §13.4 Phase 12 forward-pointer notes — the divergence-window now spans two filters (local_ratelimit + csrf). Other clusters remain on their phase-11-set timelines. |
| **CF-4** | `LocalRateLimitPerRoute` SPEC/PLAN errata pattern | **Discipline reused — no errata triggered for csrf.** Phase 12 SPEC §11 empirical-pin block ran cleanly; the `CsrfPolicy` proto used for both listener-level and per-route TPFC is the same upstream proto (no `CsrfPolicyPerRoute` non-existence question). The discipline-of-record (capture corrections in PROGRESS.md preamble; do NOT amend SPEC/PLAN) was available but unused. |
| **CF-5** | `singleflight` optimization for `resolvePerRouteConfig` | **Not applicable to csrf.** csrf's `buildPerRouteRuntime` is a pure-data transform (no allocation of stateful resources); under cold-start fan-out the duplicate allocations would only be `*runtimeConfig` slices, not buckets. Phase 11's TODO remains valid for future stateful filters; csrf neither extends nor closes it. |
| **CF-6** | `buildRuntimeConfigPerRoute` / `buildRuntimeConfig` duplication | **Not duplicated for csrf.** csrf consolidates parse-time validation in `New` (the listener-level path) only; per-route building is a runtime-time data-only transform (`buildPerRouteRuntime` at request time per planner-time decision 5), not a re-validation. The phase-11 KEEP-IN-SYNC pattern is not needed here. |
| **CF-7** | `flattenToProm` SN1/SN3/SN5 asymmetry | **Still open.** Phase 12 reuses the existing SN2 HCM-namespace tag-extractor per §11.6 confirmation — no new SN rule, no exercise of the SN1/SN3/SN5 dotted-rest path. Carried forward unchanged for a future stat-discipline phase. |
| **CF-8** | Tag-extraction collision quirk | **Still open; out of scope.** Fixture 0014's HCM `stat_prefix=ingress_csrf` is collision-safe; no exercise of the quirk. Forward-pointer note in BEHAVIOR_CONTRACT §13.4 (phase 11) remains valid. |

---

## 3. Per-task retrospective

The 13 tasks landed 13 task-commits + SHA-fill / PROGRESS-append follow-ups + 2 in-flight code-review fix-up commits (Task 2 follow-up `6bf381e`; Task 4 follow-up `c3cdd2c`). Tasks that deviated from PLAN verbatim are called out below.

**Task 1 (commit `3f34717`):** Execution-precondition check + PROGRESS.md preamble. All 16 preconditions satisfied at cold-start without any IMPL-N substitutions (contrast phase 11 IMPL-1). The PROGRESS.md preamble enumerates the 9 planner-time deferred decisions verbatim from the PLAN so the file is self-contained for any task-N reader. No deviations.

**Task 2 (commit `d127af4` + follow-up `6bf381e`):** `internal/filter/http/csrf/` package skeleton + `New` factory PGV-mirror + parse-time StringMatcher drop. Lands `doc.go` + `csrf.go` + `csrf_test.go` (Groups 1 + 2). **Two minor structural deviations from PLAN sketches:** (a) `filterStats` field type is `*stats.Counter` (not `*atomic.Int64` as the PLAN/SPEC §6.2 documented conceptually) — `*stats.Counter` itself wraps `atomic.Uint64` so the lock-free-Inc semantic is preserved (matches phase 11 ADR-0115's `filterStats` shape); (b) the PLAN's stub helpers (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`, `buildPerRouteRuntime`) + `_ = url.Parse` import-keepalive were OMITTED at this commit — golangci-lint's `unused` linter flagged them; Task 3 lands them with their bodies + the `net/url` import in lockstep.

**Task 2 follow-up (commit `6bf381e`):** Code-quality-reviewer Approved-with-comments fixups: (I-1) ADR-0121 §Decision (ii) prose polish — replaced a self-correcting mid-clause about phase-11 vs phase-12 wording-discipline inversion with crisp ADR prose stating the structural distinction (numeric-bound vs proto-shape PGV checks); (I-2) `newFilterStats` nil-guard relocation — moved the guard from inside `newFilterStats` to the call-site in `New`, mirroring phase 11 local_ratelimit's pattern; (M-1) corrected an inaccurate code comment about `filter_enabled.default_value` percentage inspection; (M-3) added a one-line forward-disclaimer in `doc.go` for ADR-0122/0123/0124 anchors landing in Tasks 3-4. Production-code surface unchanged by I-1/M-3; minor production-code edit by I-2/M-1.

**Task 3 (commit `b9f946b`):** `DecodeHeaders` body + 4-method gate + origin trichotomy + host:port-only equality + reject path. Fills the helper bodies omitted at Task 2 (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`, `buildPerRouteRuntime`). Lands ADR-0122 (algorithm) + ADR-0123 (rejection wire shape). **One PLAN-text deviation noted:** PLAN test-code lines 845-924 + impl-code lines 1118-1123 use `.Add(1)` on counters; impl chose `.Inc()` to match phase 11 `local_ratelimit.go:363-369` precedent (idiomatic Counter usage; `Add(delta)` reserved for non-unit increments). NO semantic change.

**Task 4 (commit `5b1b70e` + follow-up `c3cdd2c`):** `filterStats` wiring confirmation + per-route SHARED-stats unit tests + 3-counter stat-name discipline. **NO production code changes** — Task 3 already landed the per-route shared-stats wiring via `buildPerRouteRuntime` per planner-time decision 5; Task 4 is unit-test confirmation + ADR-0124 landing only. **Two PLAN-text deviations** (PLAN verbatim test code did not compile against actual Counter API; impl adapted; no semantic change): (a) `int64(N)` → `uint64(N)` in counter assertions (Counter.Load returns uint64); (b) `reg.Counter(name)` lookup → `reg.Walk(...)` set-membership check (Registry exposes no Counter(name); phase 11 precedent uses Walk).

**Task 4 follow-up (commit `c3cdd2c`):** Code-review Important + Minor fix-ups: (I) `TestStats_ThreeCountersUnderHCMStatPrefix` was MISSING-only-asymmetric (would silently pass on a 4th unexpected counter); added `len(registered) != 3` exact-count assertion ahead of the per-name loop, mirroring phase 11's `TestStatNames_FourCountersUnderStatPrefix` discipline. (M) Reworded a misleading "AGGREGATE" error message in the single-request `TestDecodeHeaders_PerRouteOverride_DataReplaced` to clarify the property under test is per-route-shared-with-listener (not multi-source aggregation, which is covered by the next test). Production code untouched.

**Task 5 (commit `d87afe4`):** `FuzzCsrfPolicyConfigParse` (sixteenth fuzzer). 30s budget: 4.77M execs clean. **One PLAN-text deviation:** renamed local variable `any` → `tc` to avoid shadowing Go 1.18+'s predeclared `any` (alias for `interface{}`); linters (`predeclared`) commonly flag this. NO semantic change.

**Task 6 (commit `130207f`):** `cmd/envoy-go/main.go` boot registration. Two-line registration delta inserted alphabetically between `cors` and `envoygotest`. Boot block now reads as a single sorted list of 7 filter factories (`router → cors → csrf → envoygotest → fault → header_mutation → localratelimit`). No deviations.

**Task 7 (commit `5d1902f`):** Fixture infrastructure — `HTTPCsrf BackendKind = 11` enum + runner spawn helper + driver stub for blank-import. Three-file delta wiring fixture 0014 into the differential runner ahead of fixture content. Pattern-match against phase-11 BackendKind=10 precedent. **One stylistic precedent applied:** the `startHTTPCsrfBackend` spawn helper wraps the underlying `cmd.Start()` error with `fmt.Errorf("start: %w", err)` — preferred over PLAN pseudocode that left the error unwrapped. Minor; matches the project's existing wrap discipline. No semantic change.

**Task 8 (commit `75543b3`):** Fixture 0014 `backends/backend.go`. Near-copy of phase-11 fault/local_ratelimit backend (~30 LoC). **Two lint-clean adjustments vs the PLAN-verbatim block** (both skin-deep, no behavior drift): (a) added a 2-line file-level `// Backend for fixture 0014-http-csrf...` package comment to satisfy `revive`'s `package-comments` rule (PLAN's verbatim block lacked one but the phase-11 precedent has one — phase-11-precedent intent wins per PLAN's "Mirrors exactly" wording); (b) wrapped the `fmt.Fprint(w, "backend\n")` write in `_, _ =` to satisfy `errcheck` (matches the precedent's existing discharge idiom). Both required to clear the project's existing golangci-lint baseline.

**Task 9 (commit `52c8f81`):** Fixture 0014 `envoy.yaml` + `envoy-go.yaml` bootstraps. Single-listener topology per planner-time decision 7 (saves driver complexity vs phase 11's 4-listener fan-out; csrf is data-only with no per-scenario timing variation). Both YAMLs explicitly set `filter_enabled.default_value: { numerator: 100, denominator: HUNDRED }` per SPEC §11.11 amendment. `dns_lookup_family: V4_ONLY` carried forward from phase-11 IMPL fix (Docker Desktop IPv6 root cause). Both YAMLs validated under reference Envoy v1.37.2 in `--mode validate`. No deviations.

**Task 10 (commit `adb1d82`):** Fixture 0014 `expectations.yaml` + `README.md` (narrative-only documentation per ADR-0019). Verbatim-faithful to PLAN §1832-1946. Both files cross-reference §11.8 operator footgun + §11.9 SHARED-stats divergence-from-phase-11 + §11.11 `filter_enabled` PGV discipline. No deviations.

**Task 11 (commit `3a2394f` — HEADLINE):** Fixture 0014 `driver/driver.go` (single-listener 6-scenario sequential orchestration). The MOST SIGNIFICANT deviation of the phase. Replaces the Task 7 driver stub with the full `fixture.Driver` + `fixture.BackendKindAware` + `fixture.StatsAsserter` implementation (~441 LoC). 7 sequential HTTP/1.1 POSTs against `l_main` (6 public scenarios + scenario 7 split into 7a + 7b sub-requests sharing the listener); deterministic per-probe encoding; date-header allow-listed; final stat assertion `request_valid=4 / request_invalid=2 / missing_source_origin=1`. **PRODUCTION-CODE LOAD-BEARING DEVIATION** — see headline retrospective below.

> ### HEADLINE: HCM `:authority` pseudo-header injection (Task 11 production-code prereq)
>
> **What landed:** 8 lines added to `internal/filter/hcm/connection.go` (H1 path, ~lines 277-290) + 8 lines to `internal/filter/hcm/h2dispatch.go` (H2 path, ~lines 218-228) — a symmetric `:authority` injection mirroring the existing Phase-07.1 Task-18 `:method` injection pattern in the same files.
>
> **Why it was needed:** Go's stdlib `http.ReadRequest` strips the `Host` header off `req.Header` and stores it on `req.Host` (per stdlib documentation). Chain-level filters calling `headers.Get("Host")` OR `headers.Get(":authority")` therefore saw `""` on the H1 path. csrf's `targetOriginValue()` always saw an empty target → same-origin scenario 1 + Referer-fallback scenario 5 incorrectly rejected as cross-origin (subject saw `request_invalid=4 / request_valid=2` instead of expected `request_invalid=2 / request_valid=4`).
>
> **Why this is a latent bug fix, not new architectural design:** the existing `:method` injection is a Phase-07.1 Task-18 framework primitive — it ALREADY mirrors HTTP/1.1's request-line method onto the H1 codec's headers map so chain-level filters observe a consistent pseudo-header signal across both H1 and H2. The `:authority` pseudo-header was the only request-line / pseudo-header field NOT yet mirrored (request-target's path lives at `req.URL.Path`, scheme is conceptually trivial). The H1 codec's omission was latent until phase 12 produced the first chain-level filter that needed `:authority`. The fix is symmetric: same wire-emit safety guarantee as `:method` (response-emit paths iterate `OrderedHeaders` not `req.Header`, so the colon-prefixed pseudo-header never leaks onto the wire); same gating (`if _, ok := ...; !ok && req.Host != ""`); same Phase-07.1-style block comment cross-referencing the H2-codec parsing site.
>
> **Why it landed in phase 12 rather than as a separate framework-fix phase:** PLAN line 2189 explicitly anticipated this failure mode (a) in the Task 11 fixture-orchestration acceptance text: "`Host` header missing on H1 path → target hostAndPort is empty". The fix is mechanically narrow (16 lines total across two files; mirror-symmetry preserved), gate-safely scoped (h2spec 53/53 unchanged; all prior differential fixtures green), and naturally co-located with the first consumer.
>
> **Carry-forward implication for phase 13+:** future filters that read `:authority` should rely on the framework injection. Re-deriving the value from `req.Host` at the filter layer is a smell — it duplicates the framework primitive, suggests a missing test, and risks divergence between the H1 and H2 paths if the framework is ever updated asymmetrically. The same applies to `:method`. Filters that read other pseudo-headers (`:path`, `:scheme`) should consult this same primitive's coverage before re-deriving from `req.URL` / TLS state.

The first DIFFERENTIAL run of fixture 0014 FAILED with `differential mismatch` (subject 403 same-origin where reference 200) + `subj envoy_http_csrf_request_invalid=4 want 2` + `subj envoy_http_csrf_request_valid=2 want 4` — confirming the `:authority` root-cause hypothesis. Post-fix: 0014 PASSES; regression-suite `0011-0014` 4-fixture sequence PASSES end-to-end.

**Task 12 (commit `4f4ed39` + SHA-fill `2706168`):** BEHAVIOR_CONTRACT.md 4-edit bundle + ROADMAP row 12 flip + STATE.md advance + 6-gate phase-done verification. All six gates green. The 4-edit bundle landed at the expected anchors (§13.1 csrf subsection at line 1093; §13.2 26→29 stat-name table extension at line 134; §13.3 Equivalence Matrix row at line 34; §13.4 Phase 12 forward-pointer notes at line 1539). STATE.md flipped to `awaiting next planning`. Phase-done commit message includes a "Notable production-code change during Task 11" paragraph documenting the HCM `:authority` injection. No deviations.

**Task 13 (THIS commit):** REVIEW.md per end-of-phase review discipline. This document. Closes phase 12 lifecycle (state 5 → 6).

---

## 4. Planner-time decisions retrospective

The nine planner-time deferred decisions reproduced from PROGRESS.md preamble, evaluated against implementation outcomes. Mark: **✓** validated by implementation; **△** validated with a minor adjustment; **✗** exposed a flaw.

**D1 (Filter-callback wiring hook = `SetDecoderCallbacks(cb)`; encode side ABSENT — decoder-only filter):** **✓ VALIDATED.** First §9 production filter to express decoder-only structurally via `HTTPFilter{Decoder: f, Encoder: nil}`. `OnNewStream` wiring sets only `dcb` per existing framework precedent; `ecb` field absent. Pattern-matches the BRAINSTORM hypothesis exactly.

**D2 (`HTTPFilter` value shape = `Decoder: f, Encoder: nil`):** **✓ VALIDATED.** PLAN-emerging clarification of D1; saves implementing the StreamEncoderFilter method set; makes decoder-only nature self-documenting. ADR-0120 §Decision records the structural distinction from cors/fault/header_mutation (which all set both Decoder + Encoder).

**D3 (URL-parse semantics for `hostAndPort()` = `net/url.Parse` + verbatim-string-on-parse-failure):** **✓ VALIDATED.** Mirrors Envoy's `Http::Utility::Url::initialize` for common cases; verified at unit-test Group 4/5 + the §11 empirical pins as regression baseline. Synthetic `http://` prefix discipline (per planner-time decision 8) ensures URL parser acceptance; scheme is then stripped by `hostAndPort()` so byte-equivalence with reference Envoy is preserved.

**D4 (Filter-internal validation error message wording = envoy-go's own clear-text wording):** **✓ VALIDATED.** Option (b); `csrf: filter_enabled is required` + `csrf: filter_enabled.default_value is required`. Phase 11 ADR-0115's option (a) verbatim-mirror discipline was DELIBERATELY NOT followed here because Envoy's PGV-template-generated messages for proto-shape requirements (`CsrfPolicyValidationError.FilterEnabled: value is required`) are not hand-written byte-equivalence targets — contrast phase 11's numeric-bound check on `fill_interval` which DOES have a canonical `server.cc:76` byte-equivalence target. ADR-0121 §Decision (ii) records this structural inversion.

**D5 (Per-route stats wiring mechanism = OPTION (b) per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper):** **✓ VALIDATED.** Per-route runtimeConfig SHARES the listener-level `*filterStats` pointer; no `NewCounterIfAbsent` re-registration; no caching for MVP. **First production filter to demonstrate this "wholesale data-only override + shared stats" pattern.** Diverges from phase 11 ADR-0117 INDEPENDENT-stats precedent. Acknowledged in ADR-0124 §Decision (iv) with the explicit phase-11 contrast paragraph + §Consequences canonical-reference declaration.

**PLAN-6 (File-split decision = SINGLE-FILE `csrf.go`):** **✓ VALIDATED.** No `origin.go` split; final csrf.go came in at 296 LoC — under the mental-model threshold. Helpers (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`, `buildPerRouteRuntime`) are unexported and not anticipated to be reused by future filters; package-internal cohesion preserved. Contrast phase 11's `bucket.go` split which was driven by token-bucket primitive isolation.

**PLAN-7 (Fixture topology = SINGLE LISTENER `l_main` with TWO ROUTES):** **✓ VALIDATED.** `/` default + `/route-only` per-route TPFC; fits existing `fixture.Driver` contract; saves driver complexity vs phase 11's 4-listener `MultiListenerDriver` topology. All 6 scenarios run as 7 sequential POSTs in one Drive call; no per-scenario teardown.

**PLAN-8 (`:scheme` synthesis for `targetOriginValue` = USE A SYNTHETIC `http://` PREFIX):** **✓ VALIDATED.** No framework extension; no `:scheme` injection callback (contrast the `:authority` injection that DID land — different category: `:authority` is a framework-level pseudo-header surfacing fix; `:scheme` synthesis is filter-internal URL-parser glue). The synthetic prefix is stripped via `hostAndPort()` per §11.3 amendment so byte-equivalence with reference Envoy is preserved.

**PLAN-9 (`BackendKind = HTTPCsrf BackendKind = 11`):** **✓ VALIDATED.** Trivial mechanical addition. `case fixture.HTTPCsrf:` block in `runner_test.go` fires correctly; fixture registered and run successfully.

**Score: 9/9 ✓.** No flaws exposed; one minor structural distinction (D4 vs phase 11) documented in ADR-0121.

---

## 5. ADR retrospective

Each of the five ADRs ADR-0120..ADR-0124, evaluated for whether the §Decision body held up under implementation + fixture exercise:

**ADR-0120** (`internal/filter/http/csrf/` package shape — single-token directory matching cors precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value with `Encoder: nil`): **VALIDATED.** Single-token directory `csrf/` aligns with `cors/` + `fault/` precedent (UNLIKE `local_ratelimit/`'s no-underscore-elide of `local_ratelimit`). Boot-registration ordering `router → cors → csrf → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze` is exactly as described; csrf does NOT call `RegisterPerRouteValidator` (data-only override; no validation-time analog needed). The decoder-only `HTTPFilter` value is the first §9 production filter to express this structurally.

**ADR-0121** (`runtimeConfig` shape + 1-consumed/1-PGV-validated-not-honored/1-deferred field decomposition + PGV-mirror filter-internal validation discipline + StringMatcher non-exact parse-time-drop): **VALIDATED with one prose fix-up at I-1.** The 1+1+1 decomposition held throughout all 13 tasks. The §Decision (ii) prose was rewritten at the Task 2 follow-up (commit `6bf381e`) — the original text contained a self-correcting mid-clause about phase-11 vs phase-12 wording-discipline inversion; the rewrite states the structural distinction crisply (numeric-bound PGV check has canonical byte-equivalence target → option (a); proto-shape PGV check has none → option (b)). §Decision and §Consequences are now mutually consistent. The `additional_origins[].StringMatcher.exact` parse-time-drop discipline (per ADR-0101 §3) executes cleanly against the 5-variant Group 2 unit-test matrix (prefix, suffix, contains, safe_regex, ignore_case_with_exact).

**ADR-0122** (Origin extraction trichotomy + host:port-only equality + canonical 4-method gate + `additional_origins[].exact` matched against host[:port] form + scheme-strip discipline via synthetic `http://` prefix): **VALIDATED.** All four interlocked algorithm decisions held: (1) 4-method gate `{POST, PUT, DELETE, PATCH}` per §11.1; (2) origin extraction trichotomy (`null` → empty / empty → Referer / unparseable → verbatim) per §11.2 — all three branches covered by Group 4 unit tests + scenario 4/5 in fixture 0014; (3) host:port-only equality + scheme-strip via `hostAndPort()` per §11.3+§11.7+§11.8 — exercised by Group 5 unit tests (no case folding, no default-port stripping, trailing-slash IS stripped) + fixture 0014 scenarios 1+2+3; (4) `additional_origins[].exact` matched against host[:port] form per §11.7+§11.8 — exercised by Group 5's `TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches` + fixture 0014 scenario 3. Synthetic `http://` prefix discipline (per planner-time decision 8) verified empirically against reference Envoy.

**ADR-0123** (Rejection-path wire shape — `SendLocalReply(403, "Invalid origin", {Content-Type: text/plain})` + body byte-exact `Invalid origin` + 4-header lowercase wire-form + 403 hardcoded status + `SendLocalReply` reuse from phase 09 fault precedent): **VALIDATED.** The 4-header wire shape (content-length / content-type / date / server) + body `Invalid origin` (14 bytes ASCII, no LF, MD5 `7433f3a046afcebee10e455dd26b0eb6`) + 403 default status is byte-equivalent to reference Envoy in fixture 0014 scenarios 2 + 4 + 7b. The `SendLocalReply` reuse from phase-09 fault is correct. Decision: body literal kept inline at the single call site (NOT promoted to package-level `const` — contrast phase 11's `rateLimitedBody` which IS `const` because of multi-call-site reference + `runtimeConfig.body` indirection; csrf has neither). Structurally consistent.

**ADR-0124** (`BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension + 3 csrf counters anchored at HCM stat_prefix + NO new SN flattening rule + drop `shadow_request_invalid` from MVP + per-route stats SHARED with listener-level): **VALIDATED — and is the architectural centerpiece of phase 12.** Five sub-decisions: (i) stat-name discipline + Rule SN2 reuse with NO new SN rule (CONTRAST ADR-0118's SN9 addition for local_ratelimit) — verified empirically against Prometheus output (`envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="ingress_csrf"}`); (ii) 26→29-name extension landing at Task 12 — Equivalence Matrix row + 29-name table both updated; (iii) `shadow_request_invalid` MVP scope-out aligned with §11.6 conclusion (e); (iv) per-route stats SHARED with listener-level — DIVERGENCE FROM PHASE 11 ADR-0117 INDEPENDENT-stats precedent — exercised by `TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute` + fixture 0014 scenario 7's split 7a/7b stats AGGREGATING into the same 3-counter family; (v) ADR-0073 wholesale-override applies AS-IS with NO amendment paragraph (phase 11's ADR-0117 amendment + phase 10's ADR-0110 amendment both stay landed and unused by phase 12). The `buildPerRouteRuntime` helper is the canonical reference for "data-only per-route override + shared stats" implementations going forward.

---

## 6. Six-gate retrospective

**Gate (a) build/vet/lint:** Clean at all task commits. Recurring `gofmt` whitespace + `revive` package-comment + `errcheck` discharge issues caught and fixed at Tasks 2/3/8 — same pattern as prior phases. The `behavioural→behavioral` misspell was caught at Task 11 driver review. All three (build/vet/golangci-lint) clean at HEAD.

**Gate (b) unit tests + race:** All packages PASS. `go test -race -count=1 ./...` clean across all 26 packages. csrf package adds 27 test leaves (Groups 1-6) — all PASS under `-race -count=1` in 1.04s. The differential suite ran in 45.6s under `-race` — no flake on Task 12's verification sweep.

**Gate (c) h2spec:** 53/53 at ADR-0051 pin unchanged. The Task 11 HCM `:authority` injection is response-emit-safe (response-emit paths iterate `OrderedHeaders` not `req.Header`) — does NOT regress h2 conformance. Verified at Task 12 gate (c) outputs.

**Gate (d) fuzzers:** 16 fuzzers total at HEAD. `FuzzCsrfPolicyConfigParse` re-validated at Task 12 at 30s budget (~2.2M execs / 30s clean). All 15 prior fuzzers run at 30s budget each — clean. Total wall ~8 minutes for the full sweep.

**Gate (e) differential:** All 16 sub-tests (0000–0014 with 0007a/0007b split; 15 fixture directories) PASS in 83.17s on Task 12's verification sweep — no flake. (One transient port-collision flake on a re-run during Task 11 development was harness-side, pre-existing on master, unrelated to phase 12; reproduced cleanly on retry.) Fixture 0014 scenarios 2's byte-exact 403 body + 4-header set assertion + scenario 7's per-route shared-stats AGGREGATION assertion are the primary non-vacuous claims. Fixture 0014 was the most complex orchestration (single listener, 7 sequential probes, deterministic encoding, date-allow-list) but landed first-try after the Task 11 HCM `:authority` fix.

**Gate (f) BEHAVIOR_CONTRACT.md alignment + ROADMAP row 12 status:** `grep -nE 'envoy.filters.http.csrf' BEHAVIOR_CONTRACT.md` returns 4 anchors (Equivalence Matrix line 34; HTTP filter chain subsection lines 1093/1095; Forward-pointer notes line 1541). All 5 ADRs (ADR-0120..ADR-0124) confirmed in DECISIONS.md. ROADMAP row 12 reads `done`.

### Six-gate verification appendix (verbatim from Task 12 commit `4f4ed39`)

```
$ go build ./...           # clean
$ go vet ./...             # clean
$ golangci-lint run ./...  # clean

$ go test -race -count=1 ./...
ok  github.com/esalaine/envoy-go/internal/filter/http/csrf  1.041s
[... 25 other packages PASS; differential suite 45.636s ...]

$ go test -count=1 -v ./test/conformance/h2spec/ -run TestH2Spec
53 tests, 53 passed, 0 skipped, 0 failed
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.297s

$ for fuzzer in ...16 fuzzers...; do go test -fuzztime=30s ...; done
=== [3/16] FuzzCsrfPolicyConfigParse  PASS  31.083s  (16th fuzzer; new in phase 12)
[... 15 other fuzzers PASS at 30s budget each ...]

$ go test -count=1 -timeout=600s -v ./test/differential/ -run 'TestDifferential'
--- PASS: TestDifferential (83.17s)
    [... 15 sub-tests including 0014-http-csrf 3.79s ...]
PASS

$ grep -nE 'envoy.filters.http.csrf' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -5
34:| HTTP filter `envoy.filters.http.csrf` | 0014-http-csrf: scenario1: same-origin POST → 200; ...
1093:### envoy.filters.http.csrf
1095:Phase 12 ships `envoy.filters.http.csrf` per the canonical Envoy v1.37.2 filter spec. ...
1541:**Deferred field families** ...

$ grep -nE '29-name table' docs/envoy-go/BEHAVIOR_CONTRACT.md
134:### 29-name table (introduced by phase 06.1; extended by phase 09; extended by phase 11; extended by phase 12)
```

Six-gate state: **ALL GREEN at HEAD `4f4ed39`.** Phase-done commit landed at Task 12; this REVIEW closes lifecycle-state 5 → 6.

---

## 7. §1.1 amendment + confirmation retrospective

The §11 empirical-pin block produced **four** BRAINSTORM amendments and **three** confirmations. All seven held under implementation + fixture exercise:

**Amendment §11.3+§11.7+§11.8 (origin comparison shape — MAJOR REVISION; collective into ADR-0122):** **HELD.** Scheme is NOT part of equality; NO case normalization; NO default-port stripping; trailing slash IS stripped (via URL-parser path-drop, not separate normalization); `additional_origins[].exact` matched against host[:port] form. All five sub-points exercised by Group 5 unit tests + fixture 0014 scenarios 1+2+3+7. Operator-footgun callout (full-URL form NEVER matches) landed at SPEC §6.4 + BEHAVIOR_CONTRACT §13.1 + §13.4. No surprises.

**Amendment §11.2 (origin parse-failure trichotomy — MINOR REVISION; into ADR-0122):** **HELD.** Three branches: `Origin: null` literal → empty NO Referer fallback; `Origin:` empty/absent → Referer fallback; `Origin:` non-empty unparseable → verbatim NO Referer fallback. All three covered by Group 4 unit tests (`TestDecodeHeaders_OriginNullLiteral_*` + `TestDecodeHeaders_OriginEmpty_RefererFallback` + `TestDecodeHeaders_OriginUnparseable_VerbatimUsed`) + fixture 0014 scenarios 4 + 5. The synthetic `http://` prefix per planner-time decision 8 is what makes the "verbatim" branch actually trigger consistently — without the prefix, an `Origin:` value like `notaurl` would parse as an opaque scheme-less string instead of failing; the trichotomy depends on the prefix discipline.

**Amendment §11.11 (`filter_enabled` is PGV-REQUIRED — MAJOR REVISION; into ADR-0121):** **HELD.** Both `filter_enabled` non-nil + inner `default_value` non-nil are validated at parse time per the new filter-internal validation discipline. envoy-go-own-wording per planner-time decision 4. `shadow_enabled` optional + silent-ignored. Fixture 0014 explicitly sets `default_value: { numerator: 100, denominator: HUNDRED }` on both sides. No surprise: the divergence-window the §11.11 probe identified (Envoy's PGV vs envoy-go's silent-ignore default) was closed by the explicit fixture config setting; the runtime behavior is byte-equivalent. The "UNSET trap" anticipated in BRAINSTORM is structurally non-applicable to csrf (PGV rejects at boot before reaching envoy-go's runtime).

**Amendment §11.9 (per-route stats SHARED with listener-level — MINOR REVISION; into ADR-0124):** **HELD.** Per-route runtimeConfig holds `[]compiledOrigin` only; the `*filterStats` pointer is the listener-level (HCM-scoped) one. Exercised by `TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute` + fixture 0014 scenario 7's 7a/7b split. ADR-0124 §Decision (iv) records the divergence-from-phase-11 contrast paragraph; the `buildPerRouteRuntime` helper is documented as canonical reference for future "wholesale data-only override + shared stats" filters.

**Confirmation §11.6 (no `shadow_request_invalid` counter):** **HELD.** Reference Envoy v1.37.2 emits 3 counters under all-defaults config; envoy-go matches. Stat surface stays at 3 names; 26→29-name table extension is correct.

**Confirmation §11.6 (Prometheus tag-extractor reuse):** **HELD.** csrf reuses the existing `envoy_http_conn_manager_prefix` extractor (SN2 rule from ADR-0061). NO new SN flattening rule. NO new tag-extractor pattern in `internal/admin/stats.go`. Phase 12 introduces zero framework-level stat-name plumbing — contrast phase 11's SN9 + new tag-extractor.

**Confirmation §11.10 (rejection wire shape):** **HELD.** 4-header lowercase wire-form (content-length: 14, content-type: text/plain, date: <RFC1123>, server: envoy); status 403; body `Invalid origin` (14 bytes, no LF). Verified byte-equivalent against reference Envoy in fixture 0014 scenarios 2 + 4 + 7b.

**One nuance during implementation:** the §11.3 amendment's reliance on the synthetic `http://` prefix discipline (planner-time decision 8) became visible in unit-test design — Group 4 + Group 5 tests had to construct request headers consistent with the prefix synthesis pathway, NOT the reference Envoy `:scheme` injection pathway. Documented in csrf.go's targetOriginValue helper comment + ADR-0122 §Decision (5).

---

## 8. Carry-forward findings for phase 13+

| # | Item | Disposition |
|---|---|---|
| **CF-1** | "Wholesale data-only override + shared stats" pattern (`buildPerRouteRuntime` per ADR-0124) | **AVAILABLE AS PRECEDENT.** Future data-only-override filters (e.g., header-injection variants, mutator-style filters; non-stateful per-route customization) reuse the pattern: per-route runtimeConfig holds the data slice; the `*filterStats` pointer is shared with the listener-level config. ADR-0124 §Consequences documents the canonical reference. CONTRAST phase 11 ADR-0117's INDEPENDENT-stats discipline for stateful per-route filters. |
| **CF-2** | Synthetic `http://` prefix idiom (planner-time decision 8) | **AVAILABLE AS PRECEDENT.** Filters that need URL parsing without TLS/scheme state — particularly decoder-only filters with no `DownstreamTLS()` callback — can use a synthetic-scheme prefix to make the URL parser happy, then strip the scheme via `hostAndPort()`-style helpers to preserve byte-equivalence. ADR-0122 records the discipline. |
| **CF-3** | HCM `:authority` injection (Task 11 prereq fix) | **LOAD-BEARING; CARRY FORWARD AS DISCIPLINE.** Future filters reading `:authority` should rely on the framework injection at `connection.go` + `h2dispatch.go`. Re-deriving from `req.Host` at the filter layer is a smell. Same applies to `:method` (the existing Phase-07.1 Task-18 injection). Filters reading other pseudo-headers (`:path`, `:scheme`) should consult the framework's coverage before re-deriving from `req.URL` / TLS state. |
| **CF-4** | Runtime + hot restart family (deferred; blocks csrf `filter_enabled` percentage-gating + `shadow_enabled` shadow-mode) | **DEFERRED — NOW DOUBLY-BLOCKING.** The cluster blocks both phase 11 local_ratelimit's `filter_enabled` + `filter_enforced` percentage-gating AND phase 12 csrf's `filter_enabled` percentage-gating + `shadow_enabled` shadow-mode evaluation. Promoted in priority via §13.4 Phase 12 forward-pointer notes — divergence-window now spans two filters. This is a candidate for the next infrastructure-phase brainstorm. |
| **CF-5** | Full StringMatcher engine (deferred; blocks non-exact `additional_origins` variants) | **DEFERRED.** csrf currently drops prefix/suffix/contains/safe_regex/ignore_case at parse time per ADR-0101 §3 + ADR-0121. A general StringMatcher engine would unblock not just csrf's non-exact variants but also other filters (header_mutation has analogous deferrals at phase 10). Candidate for a dedicated framework-primitive phase. |
| **CF-6** | M1 dead-code in `sourceOriginValue` (deferred from Task 3 review) | **DEFERRED.** A small dead-code path in the helper was flagged at Task 3 code-review but skipped per the brief's "stylistic; defer indefinitely" disposition. Could be cleaned in a future commit; no functional impact. Surface-area: ~3 LoC. |
| **CF-7** | `flattenToProm` SN1/SN3/SN5 asymmetry (carried from phase-10 + phase-11) | **Still open.** Phase 12 did not exercise the path (reuses SN2 HCM-namespace extractor). Carry forward to a future stat-discipline phase. |
| **CF-8** | Tag-extraction collision quirk (carried from phase-11) | **Still open; out of scope.** Forward-pointer note in BEHAVIOR_CONTRACT §13.4 (phase 11) remains valid. |

---

## 9. Phase metrics

PLAN estimate ~750-1100 LoC total. Approximate actuals from committed diffs:

| Surface | PLAN estimate | Approximate actual |
|---|---|---|
| Production (`csrf.go` + `doc.go`) | ~250-400 + ~25 LoC | 296 + 49 = 345 LoC |
| Tests (`csrf_test.go`) | ~150-200 LoC | 555 LoC |
| Fuzzer (`fuzz_test.go`) | ~40 LoC | 68 LoC |
| Framework deltas (Task 6 main.go + Task 11 HCM `:authority` injection) | ~20 LoC (zero new primitives) | 2 + 16 = 18 LoC |
| Fixture (backend + YAMLs + driver + expectations + README) | ~250-350 LoC | 30 + 85 + 74 + 441 + 64 + 44 = 738 LoC |
| Docs (5 ADRs + BEHAVIOR_CONTRACT + STATE.md + ROADMAP) | ~50 LoC for non-ADR | 5 ADRs + 4-edit BC bundle + STATE/ROADMAP advance — ~600+ LoC |

Production code came in roughly on estimate (345 vs 275-425 estimate). Test LoC ran ~3x estimate driven by the 6-test-group + multiple-helper structure (Groups 1-6 + 5 helpers `newPostHeaders` / `mustNewListenerFactory` / `freshFilter` / `localReplyArgs` / `fakeCallbacks`). Fixture ran ~2x estimate driven by the 441-LoC driver (the most complex 0014-driver to date; single-listener but 7-probe sequential orchestration + per-probe encoding + stat assertion + date-allow-list).

| Phase tally | Value |
|---|---|
| ADRs landed | 5 (ADR-0120..ADR-0124) |
| Tasks completed | 13/13 |
| Six gates green | YES (build/vet/lint + race tests + h2spec 53/53 + 16 fuzzers @ 30s + 16 differential subtests + BC alignment) |
| Code-review fix-up commits | 2 (Task 2 follow-up `6bf381e`; Task 4 follow-up `c3cdd2c`) |
| SHA-fill follow-up commits | 13 (one per task) |
| New fuzzers in repo | 1 (FuzzCsrfPolicyConfigParse — 16th overall) |
| New differential fixtures | 1 (0014-http-csrf — 15th fixture directory) |
| Production-code load-bearing deviation | 1 (HCM `:authority` injection — Task 11 prereq fix; 16 LoC across 2 files) |
| §1.1 amendments + confirmations | 4 + 3, all held |
| Planner-time decisions validated | 9/9 |

---

## 10. Acceptance against SPEC §15

Cross-referencing SPEC §15 acceptance checklist (abridged):

- [x] `internal/filter/http/csrf/` package lands with `doc.go` + `csrf.go` + `csrf_test.go` + `fuzz_test.go`. `New` factory implements SPEC §6 contract.
- [x] PGV-mirror filter-internal validation per ADR-0121: `filter_enabled` non-nil + inner `default_value` non-nil checked at parse time; envoy-go-own-wording errors; `shadow_enabled` optional silent-ignored.
- [x] StringMatcher non-exact parse-time-drop discipline per ADR-0101 §3 + ADR-0121: prefix/suffix/contains/safe_regex/ignore_case dropped at PARSE; empty-`exact` values dropped; verbatim host[:port] form preserved.
- [x] Origin extraction trichotomy per ADR-0122 §11.2: `null` literal → empty NO Referer fallback; empty/absent → Referer fallback; unparseable → verbatim NO Referer fallback. All three branches unit-tested + 2 exercised in fixture.
- [x] Method gate `{POST, PUT, DELETE, PATCH}` per ADR-0122 §11.1; non-modifying methods short-circuit before counter touch.
- [x] Host:port-only equality per ADR-0122 §11.3+§11.7: scheme stripped both sides; NO case folding; NO default-port stripping; trailing slash IS stripped.
- [x] `additional_origins[].exact` matched against host[:port] form per ADR-0122 §11.7+§11.8; operator-footgun callout in SPEC §6.4 + BEHAVIOR_CONTRACT §13.1 + §13.4.
- [x] Per-route stats SHARED with listener-level per ADR-0124 + §11.9 amendment: per-route runtimeConfig holds `[]compiledOrigin` only; `*filterStats` pointer shared. `TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute` exercises the AGGREGATION property; fixture 0014 scenario 7's 7a/7b split confirms in differential.
- [x] Rejection wire shape per ADR-0123: body `Invalid origin` (14 bytes, no LF) + 4-header lowercase wire-form + 403 hardcoded status; `SendLocalReply` reuse from phase 09.
- [x] 3 csrf counters under HCM `stat_prefix` per ADR-0124 + SPEC §13.2: `request_valid` / `request_invalid` / `missing_source_origin`. NO new SN flattening rule. Reuses SN2 HCM-namespace extractor.
- [x] `cmd/envoy-go/main.go` registers `csrf.New` under `csrf.TypeURL` alphabetically between `cors` and `envoygotest`.
- [x] `FuzzCsrfPolicyConfigParse` (16th fuzzer) per ADR-0018: 30s budget; 3-entry seed corpus; `(factory, nil) ∨ (nil, error)` invariant; 4.77M execs clean at landing + 2.2M execs / 30s clean at Task 12 sweep.
- [x] Fixture 0014-http-csrf green: 6 scenarios on single listener with two routes; `RequiresReference: true`; STATIC-subject + STRICT_DNS-reference; `dns_lookup_family: V4_ONLY`; `filter_enabled.default_value: { numerator: 100, denominator: HUNDRED }` explicit on both sides; `shadow_enabled` omitted on both sides per §11.11 probe #3 baseline.
- [x] HCM `:authority` injection at `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go`: 16 LoC mirror-symmetric across H1 + H2 paths; same wire-emit safety as existing `:method` injection; fixture 0014 PASSES post-fix.
- [x] `go test -race ./...` clean: verified by gate (b).
- [x] `FuzzCsrfPolicyConfigParse` runs clean at 30s: verified by gate (d).
- [x] h2spec 53/53 PASS: verified by gate (c). The HCM `:authority` injection does NOT regress h2 conformance.
- [x] Five new ADRs (ADR-0120..ADR-0124) in DECISIONS.md: verified by gate (f).
- [x] BEHAVIOR_CONTRACT.md §13 four-edit bundle: verified by gate (f).
- [x] ROADMAP row 12 flips `in-progress → done`: verified by gate (f) (Task 12 commit `4f4ed39`).
- [x] STATE.md `active-phase: <unset — next session resolves>` + `lifecycle-state: awaiting` + `next-skill: superpowers:brainstorming`: verified by Task 12's STATE rewrite + SHA-fill `2706168`.
- [x] REVIEW.md authored: THIS document.

All acceptance items checked. Phase-done. Phase 12 lifecycle (state 5 → 6) closes at the commit landing this REVIEW. Branch `phase-12-http-filter-csrf-impl` is ready for merge to master per the linear-history (fast-forward) precedent established by phases 00–11.
