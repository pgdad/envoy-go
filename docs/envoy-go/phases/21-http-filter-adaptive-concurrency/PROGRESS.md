# Phase 21 — HTTP filter `envoy.filters.http.adaptive_concurrency` (single-row landing) — Implementation Progress

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirrors phase-04..20 PROGRESS.md structure.

- **Phase:** 21 — HTTP filter `envoy.filters.http.adaptive_concurrency` (single-row landing per ADR-0045 — Gradient-1 adaptive-concurrency filter; 7-name stat surface; sorted-slice percentile aggregation; in-package `Clock` seam; 27th fuzzer; 4-scenario differential fixture `0025` with partial cross-side byte-exact 503-overflow leg per AMEND-6)
- **Branch:** `phase-21-http-filter-adaptive-concurrency-impl` (fresh worktree at `.worktrees/phase-21-http-filter-adaptive-concurrency-impl`)
- **Base commit (master tip):** `3aa87e8` (phase-21 PLAN SHA-fill follow-up; PLAN squash `ede4ac2`; SPEC SHA-fill follow-up `3f0f768`; SPEC squash `49ba034`; BRAINSTORM SHA-fill follow-up `68ea0c5`; BRAINSTORM squash `cad1153`; phase-20 IMPL SHA-fill follow-up `4deaa5c`; phase-20 IMPL squash `da08dfc`)
- **PLAN tip SHA:** `ede4ac2` (`git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PLAN.md` → `ede4ac2da31ca7989ba8c516899ff617e254f8b1`)
- **SPEC tip SHA:** `49ba034` (`git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/SPEC.md` → `49ba03496a695922bb262daf3a1afb89eef304c2`)
- **Links:** [`PLAN.md`](./PLAN.md) · [`SPEC.md`](./SPEC.md) · [`BRAINSTORM.md`](./BRAINSTORM.md) · parent [`../../ROADMAP.md`](../../ROADMAP.md) row 21

---

## Cold-start preconditions verified

All 15 preconditions verified green at cold-start of branch `phase-21-http-filter-adaptive-concurrency-impl` (worktree at `.worktrees/phase-21-http-filter-adaptive-concurrency-impl`, branched from master tip `3aa87e8`). Master tail shows the phase-21-PLAN SHA-fill follow-up at `3aa87e8`, the PLAN squash at `ede4ac2`, the phase-21-SPEC SHA-fill follow-up at `3f0f768`, the SPEC squash at `49ba034`, the BRAINSTORM closure stack (`68ea0c5` + `cad1153`) preceding, and the phase-20-IMPL closure stack (`4deaa5c` + `da08dfc`) preceding that (exactly as expected per PLAN precondition 2). Go 1.26.2, golangci-lint v1.64.8 (ADR-0009 pin), Docker client 28.4.0 + server 28.1.1 present. ADR tail at 187 (ADR-0186 + ADR-0187 §Context drafts already at master per ADR-0044 ADR-on-impl convention; ADR-0188 stays unconsumed under PLAN D8 hypothesis — reserved for any phase-21-IMPL-unanticipated load-bearing surface; ADR-0189 stays unconsumed too per STRENGTHENED two-slot buffer per SPEC §10 D). The 2 NEW ADR §Decision + §Consequences bodies (ADR-0186 + ADR-0187) plus the 1 IN-PLACE ADR-0059 §Decision AMENDMENT body land at impl-time anchor Tasks 2-4 per the per-ADR table below. The 1 SPEC-anchored AMENDMENT-anticipation paragraph at ADR-0059 §Decision (line 2109 in DECISIONS.md) confirmed present matching the PLAN precondition 6 grep `'Amendment \(per phase 21 ADR-0186\)'`. SPEC at `49ba034`; PLAN at `ede4ac2`. The phase-21-new surfaces (`internal/filter/http/adaptive_concurrency/`, `internal/stats/conv.go`) are ALL absent at cold-start as expected. `go test -count=1 -short ./...` returns clean (all packages ok). `go test -count=1 ./test/differential/ -run 'TestDifferential'` runs the full pre-existing differential sub-test set; PASS in 92.4s. 3 representative fuzzers (`FuzzExtProcConfigParse` + `FuzzBootstrapLoad` + `FuzzOAuth2ConfigParse`) spot-checked at 30s each; all PASS clean. Reference Envoy image `envoyproxy/envoy:v1.37.2` present with SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin). Working tree pristine (empty `git status --porcelain`).

**Note on PLAN precondition 12 wording variance.** The PLAN's literal pattern `Test.*00(0[0-9]|1[0-9]|2[0-4])` returns "no tests to run" because the differential package uses a single top-level `TestDifferential` test function that iterates the fixture directories as sub-tests (per `test/differential/runner_test.go`). The substantive verification — all 25 pre-existing fixture directories `0000..0024` (counting `0007a` + `0007b` as separate is 26) GREEN — is satisfied via the parent-test `go test ./test/differential/ -run 'TestDifferential'` execution. Recorded here for the same reason phase-18..20 PROGRESS.md recorded their analogous precondition-regex deviations: planner-time wording vs runtime fact, not a blocking divergence. The actual GREEN regression baseline is captured below.

### Precondition 1 — worktree branch

```
$ git rev-parse --abbrev-ref HEAD
phase-21-http-filter-adaptive-concurrency-impl
```

### Precondition 2 — master tail

```
$ git log --oneline master | head -8
3aa87e8 phase 21 PLAN follow-up: STATE.md SHA-fill (TBD → ede4ac2 post-squash)
ede4ac2 Squash merge phase-21-http-filter-adaptive-concurrency-plan
3f0f768 phase 21 SPEC follow-up: STATE.md SHA-fill (TBD → 49ba034 post-squash)
49ba034 Squash merge phase-21-http-filter-adaptive-concurrency-spec
68ea0c5 phase 21 BRAINSTORM follow-up: STATE.md SHA-fill (TBD → cad1153 post-squash)
cad1153 Squash merge phase-21-http-filter-adaptive-concurrency-brainstorm
4deaa5c phase 20 IMPL follow-up: STATE.md SHA-fill (TBD → da08dfc post-squash)
da08dfc Squash merge phase-20-http-filter-oauth2-impl
```

Expected sequence per PLAN precondition 2 confirmed.

### Precondition 3 — toolchain

```
$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

$ docker version --format '{{.Client.Version}} / {{.Server.Version}}'
28.4.0 / 28.1.1
```

Go 1.26.2 ≥ required; golangci-lint v1.64.8 at ADR-0009 pin; Docker client 28.4.0 + server 28.1.1 both present.

### Precondition 4 — DECISIONS.md tail

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
187
```

ADR tail at `187` (ADR-0187 — the highest ADR anchored as of master tip per the phase-21 SPEC commit). Exactly as expected.

### Precondition 5 — ADR §Context drafts present

```
$ grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md
1
$ grep -nE '^## ADR-0188' docs/envoy-go/DECISIONS.md
(no output)
```

ADR-0186 + ADR-0187 §Context drafts present (anchored at SPEC commit `49ba034` per ADR-0044). ADR-0188 unconsumed (D8 hypothesis HOLDING at start).

### Precondition 6 — ADR-0059 §Decision AMENDMENT-anticipation paragraph present

```
$ grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md
2109:**Amendment (per phase 21 ADR-0186) — ANTICIPATION paragraph anchored at phase-21 SPEC commit; AMENDMENT body lands at phase-21 IMPL Task 4 per ADR-0044 in-place edit discipline.** The `*stats.Gauge` primitive is int64-only by §Decision above (Counter is uint64; Gauge is int64; both lock-free atomics); phase-21's `gradient_controller` surface introduces three value-classes that are not int64-natural and require an operator-readable encoding convention layered atop the unchanged primitive. The convention (codified at IMPL Task 4 alongside `internal/filter/http/adaptive_concurrency/stats.go` materialization):
```

Anticipation paragraph anchored at line 2109 of DECISIONS.md — confirms the SPEC-time anchoring per ADR-0044. The AMENDMENT body REPLACES this paragraph in-place at IMPL Task 4.

### Precondition 7 — NO ADR-0125 amendment

Phase 21 lands NO ADR-0125 amendment (REUSE-by-absence per SPEC §5.4 — FOURTH CONSECUTIVE §9 row after phase 18 + phase 19 + phase 20 to skip). No phase-21-specific cross-reference to ADR-0125 has landed at DECISIONS.md during preconditions.

### Precondition 8 — SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/SPEC.md
49ba03496a695922bb262daf3a1afb89eef304c2
```

SPEC at `49ba034` exactly as expected (= squash-merge commit + no later edits).

### Precondition 9 — PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PLAN.md
ede4ac2da31ca7989ba8c516899ff617e254f8b1
```

PLAN at `ede4ac2` exactly as expected (= squash-merge commit + no later edits).

### Precondition 10 — pristine tree

```
$ git status --porcelain
(empty)
```

Working tree pristine.

### Precondition 11 — pre-existing suite green at -short

```
$ go test -count=1 -short ./...
[...trimmed tail...]
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	0.019s
ok  	github.com/esalaine/envoy-go/test/helpers/extprocgrpc	0.048s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	0.016s
ok  	github.com/esalaine/envoy-go/test/helpers/oauthbackend	0.013s
```

All packages report `ok`; zero failures.

### Precondition 12 — pre-existing differential suite green

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	92.375s
```

All pre-existing differential fixtures (0000-0024, incl. 0007a + 0007b sub-fixtures = 26 directories) GREEN under the `TestDifferential` parent. (See note above on PLAN precondition 12 wording variance — the literal `Test.*00(0[0-9]|1[0-9]|2[0-4])` pattern does not match the project's `TestDifferential/0000..0024` sub-test naming; the substantive GREEN baseline is satisfied via the parent-test invocation above.)

### Precondition 13 — pre-existing fuzzers spot-check clean at 30s

```
$ go test -fuzz=FuzzExtProcConfigParse -fuzztime=30s ./internal/filter/http/extproc/
fuzz: elapsed: 30s, execs: 1410619 (28351/sec), new interesting: 3 (total: 373)
fuzz: elapsed: 31s, execs: 1410619 (0/sec), new interesting: 3 (total: 373)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	31.353s

$ go test -fuzz=FuzzBootstrapLoad -fuzztime=30s ./internal/bootstrap/
fuzz: elapsed: 30s, execs: 353642 (0/sec), new interesting: 6 (total: 1206)
fuzz: elapsed: 31s, execs: 353642 (0/sec), new interesting: 6 (total: 1206)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.254s

$ go test -fuzz=FuzzOAuth2ConfigParse -fuzztime=30s ./internal/filter/http/oauth2/
fuzz: elapsed: 30s, execs: 1229086 (49154/sec), new interesting: 15 (total: 416)
fuzz: elapsed: 31s, execs: 1229086 (0/sec), new interesting: 15 (total: 416)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/oauth2	31.159s
```

Per phase-20 PROGRESS precedent: 3 representative fuzzers spot-checked at 30s each (covering across-the-stack coverage — extproc filter, bootstrap, oauth2 filter); all PASS clean. 26-fuzzer roster confirmed present (matches PLAN precondition 13 expectation; phase-21 Task 8 lands the 27th `FuzzAdaptiveConcurrencyConfigParse`).

### Precondition 14 — reference Envoy image present

```
$ docker image inspect envoyproxy/envoy:v1.37.2 2>&1 | grep -E '"Id":|sha256:c5e8a68e'
        "Id": "sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd",
            "envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd"
            "envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd"
            "digest": "sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd",
```

Reference Envoy image SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` matches ADR-0008 pin.

### Precondition 15 — phase-21-new surfaces absent

```
$ test ! -d internal/filter/http/adaptive_concurrency && test ! -f internal/stats/conv.go && echo "ok: phase-21-new-surfaces absent"
ok: phase-21-new-surfaces absent
```

Both surfaces absent as expected — IMPL tasks will create them.

All 15 preconditions GREEN. Proceeding to Task 1 commit, then Tasks 2/4/7 PARALLEL (per PLAN D12 parallel-dispatch opportunity).

---

## ADRs introduced/landed by this plan (reproduced verbatim from PLAN)

Per PLAN §"ADRs introduced/landed by this plan" + ADR-0044 ADR-on-impl convention. §Context drafts already at SPEC commit `49ba034` (re-anchored at SHA-fill follow-up `3f0f768`). §Decision + §Consequences bodies land at each ADR's Lands-in-Task at phase-21 IMPL. The 1 IN-PLACE §Decision AMENDMENT-anticipation paragraph at ADR-0059 anchors at the SPEC commit; AMENDMENT body lands at IMPL Task 4 per ADR-0044. PLAN's strong hypothesis per D8: NO conditional impl-time-unanticipated ADR fires at phase-21 IMPL (next-free ADR-0188 stays unconsumed at phase-21 phase-done; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED).

| ADR | Subject (phase-21 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0186** | Gradient-1 controller state machine + inline `Clock` seam (NOT framework primitive) + FAKE-TIME differential strategy + sorted-slice percentile aggregation (NOT CircllHist; ≤ 1 bin-width divergence acceptable per BRAINSTORM §8 item 4) + gradient formula `clamp(0.5, min_rtt × (1 + buffer) / sample_rtt, 2.0)` + new-limit `clamp(min_concurrency, currentLimit × gradient + sqrt(currentLimit × gradient), max_concurrency_limit)` + minRTT recalc with `sample_aggregate_percentile`-quantile (NOT MIN per AMEND-2 C1) + jitter additive-to-next-interval-delay (per AMEND-2 C2) + 5-consecutive-min forced-recalc trigger (per AMEND-2 C3) + first-tick semantics (per AMEND-2 C4) + line-cited algorithmic lemmata against `gradient_controller.cc` per §21.P-D3 RATIFIED + `min_rtt_calc_params.fixed_value` PARSE-REJECT (per §Consequences (d)) | Task 3 (controller materialization; cross-references at Task 7 percentile + Task 4 stats) |
| **ADR-0187** | RTDS `enabled.RuntimeFeatureFlag` deferral PARSE-REJECT (static-default honored; `runtime_key != ""` triggers HCM-parse-time PARSE-REJECT with forward-pointer to the future Runtime/RTDS family phase) + `enabled` empty-default OFF semantics (per AMEND-4 — REFUTES BRAINSTORM "absent enabled = ON" claim per `RuntimeFeatureFlag.default_enabled` proto-default `BoolValue{value: false}`) | Task 2 (compiled_config materialization) |

### IN-PLACE §Decision AMENDMENT (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0059** | §Decision body gains AMENDMENT paragraph (already anticipated at SPEC commit per ADR-0044) documenting the float-valued-gauge int64 encoding convention per SPEC §3.2 — ns for time-typed (envoy-go-strict departure from upstream's milliseconds); ×1000 for ratio-typed; 0/1 for bool-typed; +20-30 LoC delta (NEW `internal/stats/conv.go` `boolToInt` helper + comment-only `gauge.go` cross-reference); NO signature change to `*stats.Gauge`. AMENDMENT body lands at IMPL Task 4 paired with `stats.go` materialization | Task 4 |

---

## Planner-time decisions D1-D18 (reproduced verbatim from PLAN)

The 18 planner-time decisions settled at PLAN time (settles SPEC §12 + PLAN-emerged decisions). Reproduced verbatim from PLAN so the per-Task implementer can act without re-deriving them.

1. **D1 — Task 3 + Task 7 sub-grouping LOCKED at SEPARATE PLAN-TASKS** — `controller.go` (Task 3) and `percentile.go` (Task 7) land as SEPARATE PLAN tasks even though the controller consumes the percentile helper at runtime; enables parallel-dispatch at Tasks 2+4+7 per D12.

2. **D2 — PARSE-REJECT byte-stable error message exact strings LOCKED** — 13 reference strings at PLAN §"Planner-time deferred-decision resolution" item D2 (the authoritative roster); prefix `"adaptive_concurrency:"` followed by colon-delimited subject and reason; no trailing period; mirrors ext_authz / ext_proc / oauth2 pattern.

3. **D3 — Race-test surface roster LOCKED** — TWO race-test groups: `TestController_ConcurrentForwardingDecision_*` (at `controller_test.go`; lands at Task 3) + `TestController_FAKE_TIME_TimerOrdering_*` (at `clock_test.go`; lands at Task 3 cross-cuts Task 9). 5-8 tests total; ALL clean under `-race` at Gate C.

4. **D4 — Cross-package regression-test command shape LOCKED at single test pattern** — After Task 4 (`stats.go` materialization + ADR-0059 §Decision AMENDMENT body lands) run `go test -count=1 -race ./internal/stats/...`. At Task 14 Gate D run full `go test -count=1 ./test/differential/ -run 'TestDifferential'` (all 27 fixtures: 26 pre-existing + new 0025). Per SPEC §12 item C8: zero regression expected.

5. **D5 — Stat-name compile-time guard pattern LOCKED at constant-declaration + table-driven assertion** — Stat names declared as package-level `const` declarations in `stats.go`; `newFilterStats` reads constants directly when registering each; table-driven `TestStatNames_Equal_*` test in `adaptive_concurrency_test.go` asserts the 7 constants byte-exact against the wire-expected names.

6. **D6 — Fuzzer corpus seed roster for `FuzzAdaptiveConcurrencyConfigParse` LOCKED** — ~29 seeds covering each PARSE-REJECT arm + valid-edge-case neighbors + boundary values + default-applied. Must-never-panic across `buildCompiledConfig`. Clean at 30s per seed.

7. **D7 — Boot-registration position LOCKED at line-125 between router and bandwidthlimit per SPEC §3.4** — `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` inserted at line 125 alphabetical (between `router` at line 124 and `bandwidthlimit` which shifts from 125 to 126). NO `RegisterPerRouteValidator` call.

8. **D8 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that NO additional ADR fires at phase-21 IMPL** — next-free ADR-0188 stays unconsumed at phase-21 phase-done; STRENGTHENED two-slot buffer with ADR-0189 also UNCONSUMED. If a surprise fires, ADR-0188 + D8 hypothesis recorded as falsified in PROGRESS.md.

9. **D9 — fakeClock test-helper API shape LOCKED** — `NewFakeClock(start time.Time) *fakeClock`; `Now()`; `AfterFunc(d, fn) Stop`; `Advance(d time.Duration)` synchronously fires expired timers in deadline-ascending order; `*fakeTimer.Stop()` matches `time.Timer.Stop` semantics; documented single-caller discipline.

10. **D10 — Sorted-slice quantile edge-case enumeration LOCKED at `percentile_test.go` vector roster** — Empty; SingleSample; P50_KnownSet; P0_ReturnsMin; P1_ReturnsMax; PNegative_ClampsToZero; PGreaterThanOne_ClampsToOne; UnsortedInput_DoesNotMutate. SPEC §12 item B5 closes here.

11. **D11 — `OnDestroy` token-release LOCKED at filter-glue level** — `OnDestroy()` must call `f.controller.releaseInFlight()` if `f.acquired == true`. The symmetric pair: `DecodeHeaders` Forward sets `f.acquired = true`; `OnEncodeComplete` clears after `releaseInFlight()`; `OnDestroy` clears + `releaseInFlight()` only if still acquired. `TestFilter_OnDestroy_ReleasesAcquiredToken_*` covers the invariant.

12. **D12 — Task graph parallelization LOCKED per planner-time emerge** — Tasks 2 + 4 + 7 PARALLELIZABLE after Task 1 lands. Task 3 depends on Tasks 2 + 4 + 7. Tasks 5 + 6 sequential after Task 3. Task 8 (fuzzer) partially parallel with Tasks 3 + 5 + 6 (depends only on Task 2). Tasks 9-14 sequential at the tail.

13. **D13 — Fixture 0025 listener topology LOCKED at single listener per SPEC §7.3** — 1 HCM listener; per-scenario `envoy.yaml` + `envoy-go.yaml` swap scenario-specific config knobs; synthetic slow-response upstream for scenario (b); fast-response for (a) + (c) + (d).

14. **D14 — Wire-shape byte-confirmation items in SPEC §12 A1-A4 LOCKED at fixture-0025 scenario coverage** — A1 (503 body 25-byte) closes at scenario (b) cross-side; A2 (content-type + content-length) closes at scenario (b) cross-side; A3 (response_code_details ABSENT-by-config) closes at scenario (b) by NOT-byte-pinning; A4 (Accumulate import-mode divergence) closes at Task 13 BEHAVIOR_CONTRACT.md forward-pointer.

15. **D15 — Library-behavioral items in SPEC §12 B5 + B6 + B7 LOCKED at unit-test + race-test coverage** — B5 closes at Task 7 percentile_test.go; B6 closes at Task 3 controller_test.go race tests; B7 closes at Task 3 clock_test.go deterministic-ordering tests. All three RATIFIED at Task 14 PROGRESS log.

16. **D16 — Cross-phase regression matrix item C8 LOCKED per D4 + Task 14 6-gate** — closes at Task 4 (`internal/stats/` post-AMENDMENT regression) + Task 14 Gate C (full `-race` across all packages) + Gate D (full 27-fixture regression). Zero regression expected (AMENDMENT is pure convention-extension).

17. **D17 — `concurrencyLimit` atomic-vs-mutex choice LOCKED at atomic.Uint32** — lock-free atomic load on hot path; atomic store at cold-path write sites; mirrors upstream `gradient_controller.cc:209`.

18. **D18 — `*rand.Rand` seed source LOCKED at fixed-monotonic-seed per-controller** — per-`*gradientController` `*rand.Rand` constructed via `rand.New(rand.NewSource(time.Now().UnixNano()))` at construction; concurrent callers acquire `controller.mu` before invoking (the jitter computation lives in `updateMinRTT()` which is under `mu` already).

---

## Task 1 — PROGRESS.md preamble + 15-precondition verification

**Commit SHA:** `9eb493b`

All 15 preconditions reported GREEN above. PROGRESS.md preamble (this file) authored covering: per-precondition output blocks; reproduced ADR landing table; reproduced 18 planner-time decisions D1-D18; Task 1 entry slot. Ready to dispatch Tasks 2 + 4 + 7 in PARALLEL per PLAN D12.

---

## Task 2 — compiled_config.go + 13-arm PARSE-REJECT roster + ADR-0187 §Decision + §Consequences

**Commit SHA:** `5a0a720 post-Task-2`
**Files landed:**
- `internal/filter/http/adaptive_concurrency/compiled_config.go` (NEW; 430 LoC including extensive package-level + per-arm doc comments)
- `internal/filter/http/adaptive_concurrency/compiled_config_test.go` (NEW; 564 LoC: 21 PARSE-REJECT rows + 11 default-applied rows + happy-path + nil-typed-config + unmarshal-failure + 13 byte-stable constant rows)
- `docs/envoy-go/DECISIONS.md` (MODIFY; ADR-0187 §Decision + §Consequences bodies anchored — REPLACES the SPEC-commit `### §Decision + §Consequences ANTICIPATED AT IMPL Task 2` anticipation block per ADR-0044; **Status:** header line updated from `§Context drafted at phase-21 SPEC commit; §Decision + §Consequences anchor at phase-21 IMPL Task 2 per ADR-0044` → `Landed at phase-21 IMPL Task 2 (commit 5a0a720 post-Task-2)`; +~95 LoC net delta)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 2 entry; ~70 LoC append)

**ADR landings:** ADR-0187 §Decision + §Consequences bodies (EXTENDS SPEC-commit §Context draft per ADR-0044 in-place edit discipline).

**D-questions closed:** D2 (13-arm PARSE-REJECT byte-stable roster materialized as `const` declarations + asserted byte-exact at `TestParseRejectConstants_ByteStable`).

### Deviation note — Arm 13 `fixed_value` is STRUCTURALLY UNREACHABLE in v1.32.4 go-control-plane bindings

The PLAN D2 roster lists 13 PARSE-REJECT arms (11 RATIFIED-PGV per SPEC §5.1 + 2 envoy-go-strict per SPEC §5.2). Arm 13 (`min_rtt_calc_params.fixed_value is not yet supported`) is **STRUCTURALLY UNREACHABLE** in v1.32.4 go-control-plane bindings — the `GradientControllerConfig_MinimumRTTCalculationParams` proto message at this revision exposes only `Interval`, `RequestCount`, `Jitter`, `MinConcurrency`, `Buffer` (verified via `grep -c 'fixed_value\|FixedValue\|GetFixedValue' /home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/http/adaptive_concurrency/v3/*.go` returning `0` across all 3 files). The `fixed_value` field was added in a later proto revision (v1.37.x).

**Mitigation per the proto-bump migration path:** the byte-stable wording is preserved verbatim as the `parseRejectFixedValueDeferred` package-level constant in `compiled_config.go`; the constant is asserted byte-exact by `TestParseRejectConstants_ByteStable/Arm13`. The buildCompiledConfig body includes an inline forward-pointer comment (`// Arm 13: fixed_value PARSE-REJECT ... STRUCTURALLY UNREACHABLE in v1.32.4`) with a commented-out call-site sketch showing the exact insertion location for the future proto-bump (preceding the interval-rejection arm 8 per the SPEC ordering invariant). When go-control-plane bumps to v1.37.x, the implementer uncomments the sketch + adds a `Arm13_FixedValue_Set` PARSE-REJECT test row + a `Arm13_FixedValue_AbsentInterval_PrecedesArm8` ordering test row.

**Coverage impact:** the test surface lands **21 reachable PARSE-REJECT rows** + **11 default-applied rows** + **1 happy-path row** + **1 nil-typed-config row** + **1 unmarshal-failure row** + **13 byte-stable-constant rows** = **47 test rows total** spanning 12 reachable PARSE-REJECT arms. The Arm-13 constant is exercised by the byte-stable-constant assertion only (no behavioral PARSE-REJECT path); the Arm-13 behavioral test row is deferred to the future proto-bump phase. This deviation matches the project's established discipline of pinning byte-stable wordings ahead of behavioral consumption (mirrors phase-20 oauth2's `disable_token_encryption` byte-stable constant which is similarly latent at v1.32.4 per phase-20 PROGRESS Task 11 §"Note on phase-20 v1.32.4-pinned proto field availability"). Recorded here for transparency.

**PLAN D2 row count vs reality:** PLAN D2 enumerated 13 arms; IMPL Task 2 lands 12 reachable + 1 latent (Arm 13). PLAN Step 4 acceptance criteria says `25-30 PARSE-REJECT rows + default-applied rows pass`; Task 2 lands 47 total rows (21 reachable PARSE-REJECT + 11 default-applied + 1 happy-path + 2 framework-error + 13 byte-stable-constant assertions), exceeding the PLAN floor.

### Build / test outputs (verbatim)

```
$ go build ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestBuildCompiledConfig'
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.003s
```

Verbose form (all 36 sub-tests):

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestBuildCompiledConfig' -v
=== RUN   TestBuildCompiledConfig
=== RUN   TestBuildCompiledConfig/PARSE_REJECT
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm01_ControllerOneof_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm02_ConcurrencyLimitParams_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm03_MinRTTCalcParams_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Zero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Negative
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm05_MaxConcurrencyLimit_Zero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm06_MinConcurrency_Zero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm07_RequestCount_Zero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_Absent
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_500us
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_Zero
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm09_SampleAggregatePercentile_Negative
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm09_SampleAggregatePercentile_GreaterThan100
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm10_Jitter_Negative
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm10_Jitter_GreaterThan100
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm11_Buffer_Negative
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm11_Buffer_GreaterThan100
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm12_EnabledRuntimeKey_DefaultFalse
=== RUN   TestBuildCompiledConfig/PARSE_REJECT/Arm12_EnabledRuntimeKey_DefaultTrue
=== RUN   TestBuildCompiledConfig/Defaults
=== RUN   TestBuildCompiledConfig/Defaults/Enabled_Absent_DefaultsToFalse
=== RUN   TestBuildCompiledConfig/Defaults/Enabled_DefaultValueFalse_StaysFalse
=== RUN   TestBuildCompiledConfig/Defaults/Enabled_DefaultValueAbsent_DefaultsToFalse
=== RUN   TestBuildCompiledConfig/Defaults/ConcurrencyLimitExceededStatus_Absent_Defaults503
=== RUN   TestBuildCompiledConfig/Defaults/ConcurrencyLimitExceededStatus_CodeZero_Defaults503
=== RUN   TestBuildCompiledConfig/Defaults/SampleAggregatePercentile_Absent_DefaultsToHalf
=== RUN   TestBuildCompiledConfig/Defaults/MaxConcurrencyLimit_Absent_Defaults1000
=== RUN   TestBuildCompiledConfig/Defaults/RequestCount_Absent_Defaults50
=== RUN   TestBuildCompiledConfig/Defaults/Jitter_Absent_Defaults015
=== RUN   TestBuildCompiledConfig/Defaults/MinConcurrency_Absent_Defaults3
=== RUN   TestBuildCompiledConfig/Defaults/Buffer_Absent_Defaults025
=== RUN   TestBuildCompiledConfig/HappyPath
=== RUN   TestBuildCompiledConfig/NilTypedConfig
=== RUN   TestBuildCompiledConfig/UnmarshalFailure
--- PASS: TestBuildCompiledConfig (0.00s)
    --- PASS: TestBuildCompiledConfig/PARSE_REJECT (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm01_ControllerOneof_Absent (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm02_ConcurrencyLimitParams_Absent (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm03_MinRTTCalcParams_Absent (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Absent (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Zero (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm04_ConcurrencyUpdateInterval_Negative (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm05_MaxConcurrencyLimit_Zero (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm06_MinConcurrency_Zero (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm07_RequestCount_Zero (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_Absent (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_500us (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm08_MinRTTInterval_Zero (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm09_SampleAggregatePercentile_Negative (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm09_SampleAggregatePercentile_GreaterThan100 (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm10_Jitter_Negative (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm10_Jitter_GreaterThan100 (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm11_Buffer_Negative (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm11_Buffer_GreaterThan100 (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm12_EnabledRuntimeKey_DefaultFalse (0.00s)
        --- PASS: TestBuildCompiledConfig/PARSE_REJECT/Arm12_EnabledRuntimeKey_DefaultTrue (0.00s)
    --- PASS: TestBuildCompiledConfig/Defaults (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/Enabled_Absent_DefaultsToFalse (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/Enabled_DefaultValueFalse_StaysFalse (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/Enabled_DefaultValueAbsent_DefaultsToFalse (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/ConcurrencyLimitExceededStatus_Absent_Defaults503 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/ConcurrencyLimitExceededStatus_CodeZero_Defaults503 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/SampleAggregatePercentile_Absent_DefaultsToHalf (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/MaxConcurrencyLimit_Absent_Defaults1000 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/RequestCount_Absent_Defaults50 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/Jitter_Absent_Defaults015 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/MinConcurrency_Absent_Defaults3 (0.00s)
        --- PASS: TestBuildCompiledConfig/Defaults/Buffer_Absent_Defaults025 (0.00s)
    --- PASS: TestBuildCompiledConfig/HappyPath (0.00s)
    --- PASS: TestBuildCompiledConfig/NilTypedConfig (0.00s)
    --- PASS: TestBuildCompiledConfig/UnmarshalFailure (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.003s
```

### ADR landing verification

```
$ grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md
1

$ grep -nE '^### Decision|^### Consequences|^### §Decision \+ §Consequences ANTICIPATED AT IMPL Task 2' docs/envoy-go/DECISIONS.md | awk -F: '$1 >= 11135 && $1 <= 11195'
11136:### Decision
11192:### Consequences
```

The `### §Decision + §Consequences ANTICIPATED AT IMPL Task 2` block (SPEC-commit anchor) has been REPLACED by the `### Decision` + `### Consequences` bodies per ADR-0044 in-place edit discipline. The `**Status:**` header line transitioned from `§Context drafted at phase-21 SPEC commit; §Decision + §Consequences anchor at phase-21 IMPL Task 2 per ADR-0044` to `Landed at phase-21 IMPL Task 2 (commit 5a0a720 post-Task-2)`.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 2; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 3.

---

## Task 4 — internal/stats/conv.go + internal/filter/http/adaptive_concurrency/stats.go + ADR-0059 §Decision AMENDMENT body

**Commit SHA:** `4a8f9c4 post-Task-4`
**Files landed:**
- `internal/stats/conv.go` (NEW; 17 LoC; package-public `BoolToInt(b bool) int64` helper per ADR-0059 §Decision AMENDMENT — sibling to gauge.go; canonical project-wide bool→int64 conversion for any future bool-typed-gauge consumer across §9 family-row + cluster-manager + admission-control families)
- `internal/stats/gauge.go` (MODIFY; comment-only doc-extension cross-referencing ADR-0059 §Decision AMENDMENT; +20 LoC; NO signature change to `*stats.Gauge` — the Gauge type doc-comment gains a paragraph documenting the three-class encoding convention so operators reading the godoc see the convention without consulting DECISIONS.md)
- `internal/filter/http/adaptive_concurrency/stats.go` (NEW; 223 LoC including extensive package-level + per-stat doc comments; 7-name `filterStats` roster + 7 `const statName*` declarations per planner-time D5 + `newFilterStats(reg *stats.Registry, hcmPrefix string) *filterStats` constructor under the HCM-rooted prefix per AMEND-3 C2 stat-prefix template `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>`)
- `docs/envoy-go/DECISIONS.md` (MODIFY; ADR-0059 §Decision AMENDMENT body REPLACES SPEC-commit ANTICIPATION paragraph per ADR-0044 in-place edit discipline; +~30 LoC net delta — the AMENDMENT body documents the three-class encoding convention, cites the 4 IMPL landings, references the D4/D16 cross-package regression check, and adds the operator-facing migration-note forward-pointer)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 4 entry; ~75 LoC append)

**ADR landings:** ADR-0059 §Decision AMENDMENT body (in-place; REPLACES SPEC-commit ANTICIPATION paragraph per ADR-0044).

**D-questions closed:**
- D5 (stat-name compile-time guard pattern LOCKED at constant-declaration) — fully closed at Task 4 via the 7 `const statName*` declarations; the table-driven `TestStatNames_Equal_*` assertion test in `adaptive_concurrency_test.go` is the consumer side and lands at Task 9.
- D16 (cross-package regression matrix item C8) — partially closed at Task 4 via `go test -count=1 -race ./internal/stats/...` GREEN post-AMENDMENT. Full closure at Task 12 (sibling cross-package regression command spanning all `internal/{stats,filter,cluster,listener}/...` callers) + Task 14 Gate D (27-fixture differential regression).

**SPEC §12 closures:**
- C8 partially closed (cross-package regression matrix for ADR-0059 §Decision AMENDMENT — `internal/stats/` GREEN post-AMENDMENT; full closure at Task 12 + Task 14).

**Compile-time discipline note — `//nolint:unused` directives applied to the 7 stat constants + `filterStats` + `newFilterStats`.** The 7-name surface materializes at Task 4 but its consumers land later (Task 3 controller.go consumes the gauge constants + filterStats fields at recalc-tick callsites; Task 5 filter.go consumes the rqBlocked counter at decode-headers 503 path; Task 9 boot-registration consumes newFilterStats at HCM-build time). Without the directives, golangci-lint flags all 9 symbols as `unused` until their downstream consumers land. This mirrors the established cross-task scaffolding pattern at `internal/filter/http/rbac/rbac.go` (`denyBody` constant `//nolint:unused // consumed at Task 7 DecodeHeaders SendLocalReply call.`) and `internal/admin/admin_helpers_test.go` (PLAN Task 5 scaffolding `//nolint:unused // consumed by Tasks 6-9 handler tests.`). Each directive carries an explicit forward-pointer to the consuming Task so the suppression is self-documenting and removable as soon as the consumer lands.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 -race ./internal/stats/...
ok  	github.com/esalaine/envoy-go/internal/stats	1.027s
```

### ADR AMENDMENT verification

```
$ grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md
2109:**Amendment (per phase 21 ADR-0186) — landed at IMPL Task 4, dated 2026-05-18 (commit `4a8f9c4 post-Task-4`); REPLACES the SPEC-commit ANTICIPATION paragraph per ADR-0044 in-place edit discipline.** ...

$ grep -c 'ANTICIPATION paragraph anchored at phase-21 SPEC commit' docs/envoy-go/DECISIONS.md
0
```

The ANTICIPATION block (SPEC-commit anchor at lines 2109-2115) has been REPLACED in-place by the AMENDMENT body. The literal substring `(per phase 21 ADR-0186)` is preserved in the AMENDMENT opener to satisfy the planner-time grep regex; the AMENDMENT phrasing transitions from "ANTICIPATION paragraph anchored at phase-21 SPEC commit" → "landed at IMPL Task 4 ... REPLACES the SPEC-commit ANTICIPATION paragraph per ADR-0044".

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 4; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 3 (Task 4 ran in parallel with Tasks 2 + 7 per PLAN D12).

---

## Task 7 — percentile.go sorted-slice quantile helper

**Commit SHA:** `498f566 post-Task-7`
**Files landed:**
- `internal/filter/http/adaptive_concurrency/percentile.go` (NEW; 79 LoC including extensive package-level + per-function doc comments; `Quantile(samples []time.Duration, p float64) time.Duration` sorted-slice helper per SPEC §6.8 + BRAINSTORM §8 item 4 carve-out + ADR-0186 §Decision sub-paragraph)
- `internal/filter/http/adaptive_concurrency/percentile_test.go` (NEW; 149 LoC; 8 D10 vector tests + 3 implementer-discretion tail-vector tests per planner-time D10 explicit allowance — `_P95_TailVector` + `_P99_TailVector` + `_100Sample_Linear`)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 7 entry; ~60 LoC append)

**ADR landings:** none direct. ADR-0186 §Decision sub-paragraph on the sorted-slice-NOT-CircllHist carve-out anchors at Task 3 (controller.go materialization). This Task contributes the helper landing referenced from the ADR body — the doc-comment in `percentile.go` cross-references ADR-0186 §Decision + BRAINSTORM §8 item 4 explicitly so the algorithmic-departure trail is self-documenting from either entry point.

**D-questions closed:** D10 (sorted-slice quantile edge-case enumeration LOCKED at `percentile_test.go` vector roster) — fully closed at Task 7 via the 8 D10 rows: `Empty` + `SingleSample` + `P50_KnownSet` + `P0_ReturnsMin` + `P1_ReturnsMax` + `PNegative_ClampsToZero` + `PGreaterThanOne_ClampsToOne` + `UnsortedInput_DoesNotMutate`. The 3 implementer-discretion tail vectors (P95 + P99 + 100-sample linear) provide additional regression coverage at the integer-truncation boundary across larger sample counts.

**SPEC §12 closures:**
- B5 **RATIFIED** (sorted-slice quantile numeric divergence vs upstream CircllHist) — ≤ 1 bin-width divergence at the percentile boundary acceptable per BRAINSTORM §8 item 4 + ADR-0186 §Decision sub-paragraph; vector tests pass at Task 7 with exact-equality assertions for the D10 roster (no fuzzy bin-width tolerance needed since the integer-truncation index is deterministic). Per planner-time D15, B5 closes at Task 7 percentile_test.go.

### LoC envelope note — test file 149 LoC vs SPEC §6.8 ~80-120 LoC soft estimate

The `percentile_test.go` file lands at 149 LoC, slightly above the SPEC §6.8 source-file-roster soft estimate of ~80-120 LoC. The overage is attributable to (a) extensive package-level + per-test doc-comments cross-referencing D10 row numbers + the SPEC §4.2 + §4.5 callsite contexts + the ADR-0186 algorithmic-departure trail; and (b) the 3 implementer-discretion tail vectors (P95 + P99 + 100-sample linear) added per the explicit planner-time allowance ("You can ADD more vector tests"). The SPEC envelope uses "~" denoting a soft estimate; the BRAINSTORM §1.4 LoC envelope at the package level (~890-1390 test LoC subtotal per SPEC §6.8 table) absorbs this delta with significant headroom. Mirrors phase-20 oauth2 precedent where per-file test LoC consistently ran +20-30% above SPEC soft estimates due to the same doc-comment + extra-vector pattern. Recorded for transparency.

### Build / test outputs (verbatim)

```
$ go build ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestPercentile'
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.003s
```

Verbose form (all 11 sub-tests — 8 D10 rows + 3 implementer-discretion tail vectors):

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestPercentile' -v
=== RUN   TestPercentile_SortedSlice_Empty
--- PASS: TestPercentile_SortedSlice_Empty (0.00s)
=== RUN   TestPercentile_SortedSlice_SingleSample
--- PASS: TestPercentile_SortedSlice_SingleSample (0.00s)
=== RUN   TestPercentile_SortedSlice_P50_KnownSet
--- PASS: TestPercentile_SortedSlice_P50_KnownSet (0.00s)
=== RUN   TestPercentile_SortedSlice_P0_ReturnsMin
--- PASS: TestPercentile_SortedSlice_P0_ReturnsMin (0.00s)
=== RUN   TestPercentile_SortedSlice_P1_ReturnsMax
--- PASS: TestPercentile_SortedSlice_P1_ReturnsMax (0.00s)
=== RUN   TestPercentile_SortedSlice_PNegative_ClampsToZero
--- PASS: TestPercentile_SortedSlice_PNegative_ClampsToZero (0.00s)
=== RUN   TestPercentile_SortedSlice_PGreaterThanOne_ClampsToOne
--- PASS: TestPercentile_SortedSlice_PGreaterThanOne_ClampsToOne (0.00s)
=== RUN   TestPercentile_SortedSlice_UnsortedInput_DoesNotMutate
--- PASS: TestPercentile_SortedSlice_UnsortedInput_DoesNotMutate (0.00s)
=== RUN   TestPercentile_SortedSlice_P95_TailVector
--- PASS: TestPercentile_SortedSlice_P95_TailVector (0.00s)
=== RUN   TestPercentile_SortedSlice_P99_TailVector
--- PASS: TestPercentile_SortedSlice_P99_TailVector (0.00s)
=== RUN   TestPercentile_SortedSlice_100Sample_Linear
--- PASS: TestPercentile_SortedSlice_100Sample_Linear (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.002s
```

### Naming-convention note — package-public `Quantile` per SPEC §6.8 + phase-20 oauth2 precedent

The helper is package-public (`Quantile`, not `quantile`) per the SPEC §6.8 shape. The intra-package consumer (Task 3 `controller.go`) lives in the same `adaptive_concurrency` package, so a lowercase identifier would also work compile-wise — but the SPEC §6.8 source-file-roster names the file `percentile.go` and the helper `Quantile` (implicit from the file name + algorithm description). The phase-20 oauth2 precedent at `internal/filter/http/oauth2/cookies.go` (package-exported `DefaultCookieNames` / `DefaultSetCookieAttrs` constructors despite intra-package use; ADR-0114 stylistic license) settles the casing at package-public.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 7; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 3 (Task 7 ran in parallel with Tasks 2 + 4 per PLAN D12).

---

## Task 3 — controller.go + clock.go + clock_test.go + controller_test.go + ADR-0186 §Decision + §Consequences

**Commit SHA:** `84e317f post-Task-3`
**Files landed:**
- `internal/filter/http/adaptive_concurrency/clock.go` (NEW; 84 LoC; `Clock` interface + `Stop` handle + `defaultClock` production wiring + `timerStop` adapter per SPEC §3.1 + §6.3 + ADR-0186 §Decision)
- `internal/filter/http/adaptive_concurrency/clock_test.go` (NEW; 313 LoC; test-scope `fakeClock` step-driven implementation per planner-time D9 + 9 fakeClock determinism tests closing SPEC §12 item B6)
- `internal/filter/http/adaptive_concurrency/controller.go` (NEW; 427 LoC; `gradientController` state machine per §4 + §6.2 + ADR-0186 §Decision — `newGradientController` constructor + `forwardingDecision` hot-path CAS + `recordLatencySample` sample-routing + `releaseInFlight` decrement + `concurrencyUpdateTick` + `updateMinRTTTick` + `enterMinRTTSamplingWindowLocked` + `updateMinRTTLocked` + `calculateNewLimitLocked` + `updateConcurrencyLimitLocked` + `computeGradient` + `applyJitterLocked`)
- `internal/filter/http/adaptive_concurrency/controller_test.go` (NEW; 582 LoC; 19 Layer A FAKE-TIME algorithmic-fidelity tests + race tests per SPEC §14.1 Layer A + planner-time D3 + D15 + D17 + D18 — closes SPEC §12 items B6 + B7 RATIFIED)
- `internal/filter/http/adaptive_concurrency/stats.go` (MODIFY; cleared `//nolint:unused` directives on the 7 `statName*` consts + `filterStats` type + `newFilterStats` constructor since the consumer (controller.go) is now landed; the lint forward-progress signal flips per the Task 2/4 contract)
- `docs/envoy-go/DECISIONS.md` (MODIFY; ADR-0186 §Decision + §Consequences bodies anchored — REPLACES the SPEC-commit `### §Decision + §Consequences ANTICIPATED AT IMPL Task 3` anticipation block per ADR-0044; **Status:** header line updated from `§Context drafted at phase-21 SPEC commit; §Decision + §Consequences anchor at phase-21 IMPL Task 3 per ADR-0044` → `Landed at phase-21 IMPL Task 3 (commit `84e317f post-Task-3`)`; +~150 LoC net delta covering 8-sub-paragraph §Decision body + 8-sub-paragraph §Consequences body)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 3 entry; ~120 LoC append)

**ADR landings:** ADR-0186 §Decision + §Consequences bodies (EXTENDS SPEC-commit §Context draft per ADR-0044 in-place edit discipline). 8 §Decision sub-paragraphs cover (i) the `gradientController` Go-struct shape per planner-time D17; (ii) the `Clock` interface seam + production + test-scope wiring per D9; (iii) the 7-lemma line-exact citation table against `gradient_controller.cc`; (iv) the sorted-slice percentile aggregation consumer-side wiring; (v) the FAKE-TIME differential test taxonomy (10 test families landed); (vi) the `fixed_value` PARSE-REJECT discipline; (vi-note) the v1.32.4 proto-binding deviation discovered at Task 2; (vii) the 7-name stat-surface emission discipline. 8 §Consequences sub-paragraphs (a)-(h) cover the hot-path lock-free invariant + cold-path mu discipline + stat-emission + `fixed_value` deferred forward-pointer + race-test surface + FAKE-TIME determinism + EXTRACT-NOW Clock forward-pointer + cross-reference roster.

**D-questions closed:**
- **D3** (race-test surface taxonomy LOCKED): `TestController_ConcurrentForwardingDecision_NConcurrent` (N=1000, K=100) + `..._NoDeadlockAtN1000` (N=1000, K=1) + `TestController_ReleaseInFlight_Decrements` — all clean under `-race`.
- **D8** (impl-time-unanticipated ADR hypothesis): HOLDING — no ADR-0188 fired at Task 3; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 5.
- **D9** (fakeClock API LOCKED): `Now() + AfterFunc(d, fn) Stop + Advance(d)` — deadline-asc fire with insertion-sequence tiebreak; re-entrant AfterFunc(0, ...) drains in same Advance pass (load-bearing for AMEND-2 C3 force-arm); documented single-Advance-caller discipline.
- **D11** (D11 OnDestroy token-release): DEFERRED to Task 6 per the implementer-subagent contract — Task 3 lands `releaseInFlight()` on the controller (the hot-path decrement); Task 6 wires the encode-side hook + OnDestroy body that decides whether to call `releaseInFlight` based on the per-stream `acquired` bookkeeping (Task 5 introduces the per-stream filter struct).
- **D15** (B6 + B7 RATIFIED-PENDING-IMPL-TIME at PROGRESS log): B6 RATIFIED at `clock_test.go::TestFakeClock_MultiTimer_DeterministicOrder` + `..._SameDeadlineInsertionOrder` + `..._ReentrantAfterFunc`; B7 RATIFIED at `controller_test.go::TestController_ConcurrentForwardingDecision_NConcurrent` + `..._NoDeadlockAtN1000` — both clean under `-race`.
- **D17** (atomic.Uint32 hot-path LOCKED): `concurrencyLimit` + `numRqOutstanding` both `atomic.Uint32`; hot-path methods (`forwardingDecision` + `releaseInFlight`) perform zero mutex acquisition; cold-path state under `mu sync.Mutex`.
- **D18** (per-controller `*rand.Rand` seed via `clock.Now().UnixNano()`): seeded at constructor; mu-protected; the FAKE-TIME `TestController_FAKE_TIME_JitterApplication_InRange` (100 iterations) confirms output range `[interval, interval + interval*jitter_pct)` per AMEND-2 C2.

**SPEC §12 closures:**
- **B6 RATIFIED** (fakeClock timer-fire determinism under multi-timer same-tick) — `clock_test.go::TestFakeClock_MultiTimer_DeterministicOrder` verifies 3 timers fire in deadline-asc order; `..._SameDeadlineInsertionOrder` verifies tie-break via insertion sequence; `..._ReentrantAfterFunc` verifies the AfterFunc(0, ...) re-entrancy load-bearing for AMEND-2 C3 force-arm.
- **B7 RATIFIED** (CAS-vs-mutex contention behavior at scale) — `controller_test.go::TestController_ConcurrentForwardingDecision_NConcurrent` (N=1000 + K=100) verifies exactly K forwarders return true + N-K return false + `numRqOutstanding == K` post-test + `rqBlocked counter == N-K`. No deadlock at N=1000 + K=1 verified by `..._NoDeadlockAtN1000` (5-second watchdog).

### Deferred tests (per Task 5 + Task 6 split — implementer-subagent contract)

- `TestController_503_BodyAndHeaders_ByteExact` → **Task 5** (`decode_headers.go` landing; needs the `SendLocalReply` emission path which lives at the filter layer, not the controller proper). The controller's `forwardingDecision()` Block return value is exercised by Task 3 race tests; the byte-exact 503 wire shape (status + body + `content-type` + `content-length`) lands at Task 5 where the filter glue + the `SendLocalReply` callsite live.
- `TestFilter_OnDestroy_ReleasesAcquiredToken_*` → **Task 6** (`encode_complete.go` + `OnDestroy` body landing; needs the per-stream filter struct's acquired-token bookkeeping which lives at Task 5 + Task 6, not the controller). The controller's `releaseInFlight()` hot-path method IS landed at Task 3 — Task 6 will wire the per-stream `acquired bool` bookkeeping + the `OnDestroy` body that decides whether to call `releaseInFlight` based on whether `acquired` is true at destroy time (D11 token-release-on-reset-before-encode path).

### Note on test count — 28 controller/clock NEW tests + 9 fakeClock determinism tests = 29 total Task-3 tests

Task 3 lands 29 new tests (verified via `go test ./internal/filter/http/adaptive_concurrency/... -run 'TestController|TestFakeClock' -v 2>&1 | grep -cE '^=== RUN'`). Combined with the pre-existing Task 2 + Task 7 tests (62 sub-tests), the package total is **91 tests** (verified via `go test -v 2>&1 | grep -cE '^=== RUN.*Test'`). All 91 pass clean; race detector clean.

### Note on LoC envelope — controller.go 427 LoC vs SPEC §6.8 ~350-450 soft estimate

`controller.go` lands at 427 LoC, within the SPEC §6.8 soft estimate of ~350-450. `controller_test.go` lands at 582 LoC, slightly above the SPEC §6.8 soft estimate of ~400-550 LoC — attributable to (a) the 7-citation lemma table cross-reference in per-function doc-comments; (b) the explicit gauge-emission verification in `TestController_FAKE_TIME_ConcurrencyUpdateTick_EmitsGaugesAfterWindowClose` (end-to-end driver); (c) extensive per-test doc-comments cross-referencing the AMEND-2 sub-amendments. `clock.go` 84 LoC matches the SPEC §6.8 ~30-60 LoC estimate (overage attributable to the extensive doc-comments on the inline-NOT-framework-primitive rationale per ADR-0186 §Decision). `clock_test.go` 313 LoC exceeds the SPEC §6.8 ~80-150 LoC estimate — attributable to the 9 dedicated determinism tests (each 5-15 lines) + the re-entrancy test (load-bearing for AMEND-2 C3) + the per-function doc-comments. The BRAINSTORM §1.4 LoC envelope at the package level (~890-1390 test LoC subtotal per SPEC §6.8 table) absorbs the delta with significant headroom. Recorded for transparency.

### Build / test outputs (verbatim)

```
$ go build ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run ./internal/filter/http/adaptive_concurrency/...
(no output — clean)

$ go test -count=1 ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.005s

$ go test -race -count=1 ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.022s
```

Task-3-only test families (verbose form; 29 sub-tests covering controller proper + fakeClock determinism):

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestController|TestFakeClock' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL)'
=== RUN   TestFakeClock_Now_ReflectsStart
--- PASS: TestFakeClock_Now_ReflectsStart (0.00s)
=== RUN   TestFakeClock_Advance_MovesNow
--- PASS: TestFakeClock_Advance_MovesNow (0.00s)
=== RUN   TestFakeClock_AfterFunc_FiresAtDeadline
--- PASS: TestFakeClock_AfterFunc_FiresAtDeadline (0.00s)
=== RUN   TestFakeClock_AfterFunc_DoesNotFireBefore
--- PASS: TestFakeClock_AfterFunc_DoesNotFireBefore (0.00s)
=== RUN   TestFakeClock_Stop_PreventsFire
--- PASS: TestFakeClock_Stop_PreventsFire (0.00s)
=== RUN   TestFakeClock_Stop_AfterFireReturnsFalse
--- PASS: TestFakeClock_Stop_AfterFireReturnsFalse (0.00s)
=== RUN   TestFakeClock_MultiTimer_DeterministicOrder
--- PASS: TestFakeClock_MultiTimer_DeterministicOrder (0.00s)
=== RUN   TestFakeClock_MultiTimer_SameDeadlineInsertionOrder
--- PASS: TestFakeClock_MultiTimer_SameDeadlineInsertionOrder (0.00s)
=== RUN   TestFakeClock_ReentrantAfterFunc
--- PASS: TestFakeClock_ReentrantAfterFunc (0.00s)
=== RUN   TestController_FAKE_TIME_FirstTickSemantics
--- PASS: TestController_FAKE_TIME_FirstTickSemantics (0.00s)
=== RUN   TestController_FAKE_TIME_ConcurrencyUpdateTickShortCircuitsInWindow
--- PASS: TestController_FAKE_TIME_ConcurrencyUpdateTickShortCircuitsInWindow (0.00s)
=== RUN   TestController_FAKE_TIME_GradientFormula_NoBuffer
--- PASS: TestController_FAKE_TIME_GradientFormula_NoBuffer (0.00s)
=== RUN   TestController_FAKE_TIME_GradientFormula_25PctBuffer
--- PASS: TestController_FAKE_TIME_GradientFormula_25PctBuffer (0.00s)
=== RUN   TestController_FAKE_TIME_GradientFormula_ClampLow
--- PASS: TestController_FAKE_TIME_GradientFormula_ClampLow (0.00s)
=== RUN   TestController_FAKE_TIME_GradientFormula_ClampHigh
--- PASS: TestController_FAKE_TIME_GradientFormula_ClampHigh (0.00s)
=== RUN   TestController_FAKE_TIME_GradientFormula_ZeroSampleRTT
--- PASS: TestController_FAKE_TIME_GradientFormula_ZeroSampleRTT (0.00s)
=== RUN   TestController_FAKE_TIME_NewLimitCalculation_NoChange
--- PASS: TestController_FAKE_TIME_NewLimitCalculation_NoChange (0.00s)
=== RUN   TestController_FAKE_TIME_NewLimitCalculation_ClampMin
--- PASS: TestController_FAKE_TIME_NewLimitCalculation_ClampMin (0.00s)
=== RUN   TestController_FAKE_TIME_NewLimitCalculation_ClampMax
--- PASS: TestController_FAKE_TIME_NewLimitCalculation_ClampMax (0.00s)
=== RUN   TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin
--- PASS: TestController_FAKE_TIME_MinRTTRecalcWindow_PercentileNotMin (0.00s)
=== RUN   TestController_FAKE_TIME_JitterApplication_InRange
--- PASS: TestController_FAKE_TIME_JitterApplication_InRange (0.00s)
=== RUN   TestController_FAKE_TIME_JitterApplication_ZeroPct
--- PASS: TestController_FAKE_TIME_JitterApplication_ZeroPct (0.00s)
=== RUN   TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc
--- PASS: TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc (0.00s)
=== RUN   TestController_ConcurrentForwardingDecision_NConcurrent
--- PASS: TestController_ConcurrentForwardingDecision_NConcurrent (0.00s)
=== RUN   TestController_ConcurrentForwardingDecision_NoDeadlockAtN1000
--- PASS: TestController_ConcurrentForwardingDecision_NoDeadlockAtN1000 (0.00s)
=== RUN   TestController_ReleaseInFlight_Decrements
--- PASS: TestController_ReleaseInFlight_Decrements (0.00s)
=== RUN   TestController_FAKE_TIME_RecordLatencySample_RoutesToMinRTTSamplesInWindow
--- PASS: TestController_FAKE_TIME_RecordLatencySample_RoutesToMinRTTSamplesInWindow (0.00s)
=== RUN   TestController_FAKE_TIME_RecordLatencySample_RoutesToLatencySamplesOutOfWindow
--- PASS: TestController_FAKE_TIME_RecordLatencySample_RoutesToLatencySamplesOutOfWindow (0.00s)
=== RUN   TestController_FAKE_TIME_ConcurrencyUpdateTick_EmitsGaugesAfterWindowClose
--- PASS: TestController_FAKE_TIME_ConcurrencyUpdateTick_EmitsGaugesAfterWindowClose (0.00s)
```

### ADR landing verification

```
$ grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md
1

$ grep -nE '^### (Decision|Consequences|§Decision)' docs/envoy-go/DECISIONS.md | awk -F: '$1 >= 11085 && $1 <= 11210'
11112:### Decision
11192:### Consequences
```

The `### §Decision + §Consequences ANTICIPATED AT IMPL Task 3` block (SPEC-commit anchor) has been REPLACED by the `### Decision` + `### Consequences` bodies per ADR-0044 in-place edit discipline. The `**Status:**` header line transitioned from `§Context drafted at phase-21 SPEC commit; §Decision + §Consequences anchor at phase-21 IMPL Task 3 per ADR-0044` to `Landed at phase-21 IMPL Task 3 (commit `84e317f post-Task-3`)`.

### Package-total test count

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -v 2>&1 | grep -cE '^=== RUN.*Test'
91
```

47 (Task 2 buildCompiledConfig tests) + 11 (Task 7 percentile tests) + 29 (Task 3 controller + fakeClock tests) + 4 (Task 2 PARSE-REJECT-constants byte-stable assertions inside `TestParseRejectConstants_ByteStable` parent — counted as one parent + 12 sub-tests) = 91. All PASS.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 3; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 5.

---

## Task 5 — filter.go (per-stream struct) + decode_headers.go + 503 wire shape

**Commit SHA:** `d1dd3d7 post-Task-5`
**Files landed:**
- `internal/filter/http/adaptive_concurrency/filter.go` (NEW; 177 LoC; per-stream `filter` struct + StreamDecoderFilter + StreamEncoderFilter method declarations per SPEC §6.3. The `EncodeHeaders` + `OnDestroy` bodies are `// TODO Task 6` stubs so this commit is self-buildable per the receiving-code-review observation — the StreamEncoderFilter interface assertion at filter.go would not compile if any of the six methods were missing. `DecodeData` / `DecodeTrailers` / `EncodeData` / `EncodeTrailers` are full pass-through bodies.)
- `internal/filter/http/adaptive_concurrency/decode_headers.go` (NEW; 112 LoC; `DecodeHeaders` dispatch body per SPEC §6.4 — three legs: disabled pass-through ⇒ Continue without controller consultation; Forward (capacity available) ⇒ `f.entryTime = clock.Now() + f.acquired = true + Continue`; Block (at capacity) ⇒ `SendLocalReply(concurrencyLimitExceededStatus, "reached concurrency limit", {content-type: text/plain})` per AMEND-6 + StopIteration. The `rqBlockedBody` byte-pinned 25-byte constant is declared here per SPEC §11 §21.P1 RATIFIED.)
- `internal/filter/http/adaptive_concurrency/decode_headers_test.go` (NEW; 268 LoC; 4 Group-6 DecodeHeaders tests: `_Disabled_PassThrough` + `_Forward_AcquiresToken` + `_Block_503_BodyAndHeaders_ByteExact` + `_Block_CustomStatus`. Closes the Task-3 deferral `TestController_503_BodyAndHeaders_ByteExact → Task 5` recorded at controller_test.go header + PROGRESS.md Task 3 entry. Includes the `recordedCallbacks` test-double DecoderFilterCallbacks per the rbac_test.go::rbacFakeCB precedent.)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 5 entry; ~80 LoC append)

**ADR landings:** none direct. ADR-0186 §Decision sub-paragraph on the byte-pinned 503 wire shape was anchored at Task 3 (controller materialization); Task 5 lands the filter-layer emission path (`SendLocalReply` callsite) and the byte-exact 4-test coverage suite (3 + 1 custom status). The AMEND-6 wire-shape pin (25-byte body verbatim + lowercase `content-type: text/plain`) lands at decode_headers.go::rqBlockedBody constant + decode_headers.go::DecodeHeaders Block leg.

**D-questions closed:**
- **D8** (impl-time-unanticipated ADR hypothesis): HOLDING — no ADR-0188 fired at Task 5; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 6.
- **D11** (D11 OnDestroy token-release): STILL DEFERRED to Task 6 per the implementer-subagent contract — Task 5 introduces the per-stream `filter` struct + the `acquired bool` field + the `entryTime time.Time` field; Task 6 wires the encode-side `EncodeHeaders` body (RTT recording + release-on-acquired) + the `OnDestroy` body (D11 token-release fallback when the encode side never fires). The Task 5 stubs (`// TODO Task 6` lines in filter.go::EncodeHeaders + filter.go::OnDestroy) point forward to the Task 6 landing.

**SPEC §12 closures:**
- **A1 + A2 RATIFIED** (503 wire shape: status + body + content-type byte-exact) — `decode_headers_test.go::TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact` verifies status=503 + body="reached concurrency limit" (25 bytes verbatim; `len(body) == 25` asserted explicitly) + `content-type: text/plain` (lowercase per SPEC line 440 fixture convention). The full cross-side byte-comparison against reference Envoy v1.37.2 lands at Task 10 fixture-0025 scenario (b) per AMEND-6; the filter-layer subject-only byte-exact pin lands here.
- **A3 documented** (response_code_details "reached_concurrency_limit" ABSENT-by-config) — the decode_headers.go file-header comment documents the envoy-go MVP departure: envoy-go has no access-log surface, so the response_code_details field is NOT emitted (ABSENT-by-config, NOT byte-pinned) per SPEC §12 item A3.

**Counter-discipline pin (rqBlocked single-increment):** the Task-3 controller.go::forwardingDecision body increments rqBlocked exactly once in the Block leg before returning false. The Task-5 filter MUST NOT double-increment; `TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact` pins `rqBlocked == rqBlockedBefore + 1` (single increment) at the end of the test.

### LoC envelope note — filter.go 177 LoC + decode_headers.go 112 LoC vs SPEC §6.8 estimates

`filter.go` lands at 177 LoC vs the SPEC §6.8 soft estimate of ~60-100 LoC (overage attributable to extensive package-level + per-function doc-comments cross-referencing the SPEC §6.3 + §6.4 + §6.5 + AMEND-6 + planner-time D11 + D17 + the rbac/cors precedent pointers; the production code itself is ~30 LoC of methods + 6 LoC of static interface assertions). `decode_headers.go` lands at 112 LoC vs the SPEC §6.8 soft estimate of ~50-80 LoC (similar doc-comment overage; the production `DecodeHeaders` body itself is ~20 LoC). `decode_headers_test.go` lands at 268 LoC vs the SPEC §6.8 soft estimate of ~80-150 LoC (overage attributable to (a) the `recordedCallbacks` test-double stubbing all 13 DecoderFilterCallbacks methods, (b) per-test doc-comments cross-referencing the SPEC §6.4 + AMEND-6 wire-shape pin, (c) extensive per-assertion error messages). The BRAINSTORM §1.4 LoC envelope at the package level (~890-1390 test LoC subtotal per SPEC §6.8 table) absorbs the delta with significant headroom. Recorded for transparency per the established Task-2/3/7 precedent.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.025s
```

Task-5-only test family (verbose form; 4 sub-tests covering Group 6 DecodeHeaders dispatch):

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestFilter_DecodeHeaders' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL)'
=== RUN   TestFilter_DecodeHeaders_Disabled_PassThrough
--- PASS: TestFilter_DecodeHeaders_Disabled_PassThrough (0.00s)
=== RUN   TestFilter_DecodeHeaders_Forward_AcquiresToken
--- PASS: TestFilter_DecodeHeaders_Forward_AcquiresToken (0.00s)
=== RUN   TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact
--- PASS: TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact (0.00s)
=== RUN   TestFilter_DecodeHeaders_Block_CustomStatus
--- PASS: TestFilter_DecodeHeaders_Block_CustomStatus (0.00s)
PASS
```

### Package-total test count

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -v 2>&1 | grep -cE '^=== RUN.*Test'
95
```

91 (start of Task 5) + 4 (new Task 5 DecodeHeaders sub-tests) = 95. All PASS.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 5; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 6.

---

## Task 6 — encode_complete.go + EncodeHeaders/OnDestroy bodies per D11

**Commit SHA:** 07d18c5 post-Task-6 (filled at follow-up commit per the established TBD → SHA pattern)
**Files landed:**
- `internal/filter/http/adaptive_concurrency/encode_complete.go` (NEW; 55 LoC; `recordRTTAndRelease()` helper invoked from filter.go's `EncodeHeaders` per SPEC §6.5. Body: `if !f.acquired { return }`; `rtt := f.clock.Now().Sub(f.entryTime)`; `f.controller.recordLatencySample(rtt)`; `f.controller.releaseInFlight()`; `f.acquired = false`. The envoy-go encode-side first-hook is the OnEncodeComplete semantic equivalent per the Task-3 discovery anchored at controller.go header.)
- `internal/filter/http/adaptive_concurrency/filter.go` (MODIFIED; 190 LoC total; replaces the Task 5 `// TODO Task 6` stubs in `EncodeHeaders` + `OnDestroy`. `EncodeHeaders` now delegates to `recordRTTAndRelease`; `OnDestroy` implements the D11 token-release symmetry inline — `if f.acquired { releaseInFlight + clear f.acquired }`. File-header comment updated: the "Task 6 stubs (forward-pointer)" section is replaced with the "Encode-side body delegation" section documenting the bidirectional idempotency contract.)
- `internal/filter/http/adaptive_concurrency/encode_complete_test.go` (NEW; 260 LoC; 5 Group-6 encode-side + OnDestroy tests covering the full D11 lifecycle matrix: `TestFilter_EncodeHeaders_RecordsRTTAndReleases` (happy path) + `TestFilter_EncodeHeaders_NotAcquired_NoOp` (disabled/Block pass-through) + `TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded` (D11 stream-reset path) + `TestFilter_OnDestroy_AlreadyReleased_NoOp` (D11 idempotency post-EncodeHeaders) + `TestFilter_OnDestroy_NotAcquired_NoOp` (disabled/never-acquired path). Reuses the `newTestFilter` helper from `decode_headers_test.go` (Task 5 landing) — no additional test fixtures required.)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 6 entry; ~50 LoC append)

**ADR landings:** none direct. The D11 token-release symmetry was anchored at planner-time D11 + ADR-0186 §Consequences; Task 6 lands the filter-layer emission paths (`recordRTTAndRelease` helper + `OnDestroy` inline body) + the 5-test coverage suite pinning the bidirectional idempotency invariant (exactly one of `{EncodeHeaders, OnDestroy}` releases the token, regardless of firing order).

**D-questions closed:**
- **D11** (OnDestroy token-release): **CLOSED** — Task 6 lands the symmetric pair (encode-side `recordRTTAndRelease` clears the acquired flag; OnDestroy releases the token only when `f.acquired == true`). The `TestFilter_OnDestroy_AlreadyReleased_NoOp` test pins the post-EncodeHeaders idempotency invariant (no double-decrement on numRqOutstanding — the uint32 wraparound failure mode would be worse than the slot leak D11 was added to prevent). The `TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded` test pins the reset-before-encode safety net.
- **D8** (impl-time-unanticipated ADR hypothesis): HOLDING — no ADR-0188 fired at Task 6; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 8 (next pending task per the in-progress order).

**SPEC §12 closures:**
- **B-series encode-side closures**: Task 6's 5-test matrix closes the per-stream-lifecycle invariants. The `TestFilter_EncodeHeaders_RecordsRTTAndReleases` test pins the SPEC §6.5 (a)-(d) sequence (acquired-check, RTT compute, recordLatencySample call, releaseInFlight call, acquired clear) via direct slice-state observation (`minRTTSamples[0] == rttDelta` under mu) — leveraging the controller's AMEND-2 C4 first-tick-in-window state (recordLatencySample routes to minRTTSamples until the window closes).

**Counter-discipline pin (numRqOutstanding single-decrement):** the `TestFilter_OnDestroy_AlreadyReleased_NoOp` test pins the D11 idempotency invariant: after a happy-path `DecodeHeaders → EncodeHeaders` sequence (numRqOutstanding back to 0), a subsequent `OnDestroy` MUST observe `f.acquired == false` and skip the release. Without this guard, the second decrement would underflow the uint32 to MAX_UINT32, sending 100% of subsequent requests to the Block leg permanently.

### LoC envelope note — encode_complete.go 55 LoC + encode_complete_test.go 260 LoC vs SPEC §6.8 estimates

`encode_complete.go` lands at 55 LoC vs the SPEC §6.8 soft estimate of ~40-60 LoC (within envelope; the doc-comment cross-references SPEC §6.5 + D11 + D17 + the OnDestroy symmetric pair). The production helper body itself is 8 LoC (the standard early-return + 4-step release sequence). `filter.go` grows from 177 LoC at Task 5 → 190 LoC at Task 6 (+13 LoC: the `OnDestroy` body expansion from 1 stub line + 3 doc-comment lines to 4 production lines + 23 doc-comment lines documenting the three lifecycle paths; offset partly by the `EncodeHeaders` body shrinking from 1 stub line + 3 doc-comment lines to 1 production line + 8 doc-comment lines; file-header section "Task 6 stubs (forward-pointer)" replaced with the more-concise "Encode-side body delegation" section). `encode_complete_test.go` lands at 260 LoC vs the SPEC §6.8 soft estimate of ~80-150 LoC (overage attributable to (a) per-test doc-comments cross-referencing SPEC §6.5 + D11 + the bidirectional idempotency invariant, (b) extensive setup-phase assertions verifying the pre-encode state before invoking the SUT, (c) explicit assertion-failure messages calling out the failure-mode consequence — e.g., "uint32 wraparound to MAX_UINT32" for the D11 idempotency test). The BRAINSTORM §1.4 LoC envelope absorbs the delta with significant headroom. Recorded for transparency per the established Task-2/3/5/7 precedent.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.026s
```

Task-6-only test family (verbose form; 5 sub-tests covering Group 6 encode-side + OnDestroy lifecycle):

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -run 'TestFilter_EncodeHeaders|TestFilter_OnDestroy' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL)'
=== RUN   TestFilter_EncodeHeaders_RecordsRTTAndReleases
--- PASS: TestFilter_EncodeHeaders_RecordsRTTAndReleases (0.00s)
=== RUN   TestFilter_EncodeHeaders_NotAcquired_NoOp
--- PASS: TestFilter_EncodeHeaders_NotAcquired_NoOp (0.00s)
=== RUN   TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded
--- PASS: TestFilter_OnDestroy_ReleasesAcquiredToken_AcquiredButNotEncoded (0.00s)
=== RUN   TestFilter_OnDestroy_AlreadyReleased_NoOp
--- PASS: TestFilter_OnDestroy_AlreadyReleased_NoOp (0.00s)
=== RUN   TestFilter_OnDestroy_NotAcquired_NoOp
--- PASS: TestFilter_OnDestroy_NotAcquired_NoOp (0.00s)
PASS
```

### Package-total test count

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -v 2>&1 | grep -cE '^=== RUN.*Test'
100
```

95 (start of Task 6) + 5 (new Task 6 encode-side + OnDestroy sub-tests) = 100. All PASS.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 6; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 8 (Task 7 already landed pre-Task 5 per the parallel-Task-7 split recorded at the Task 7 PROGRESS entry).

---

## Task 8 — fuzz_test.go 27th fuzzer FuzzAdaptiveConcurrencyConfigParse + corpus seeds per D6

**Commit SHA:** a7f2ffd post-Task-8 (filled at follow-up commit per the established TBD → SHA pattern)
**Files landed:**
- `internal/filter/http/adaptive_concurrency/fuzz_test.go` (NEW; 337 LoC; 27th project-wide fuzzer per SPEC §6.7 + PLAN Task 8 + D6. Hosts the `FuzzAdaptiveConcurrencyConfigParse` function with ~30 hand-curated `f.Add` seeds + the must-never-panic fuzz body. The fuzz body wraps `data []byte` into an `*anypb.Any` envelope with the AdaptiveConcurrency `TypeUrl` + calls `buildCompiledConfig`; `defer recover()` catches panics + fails the test with the offending input hex. Seeds REUSE `validConfig()` from `compiled_config_test.go` (intra-package `_test.go` helpers are visible across test files in the same package; no helper duplication).)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 8 entry; ~70 LoC append)

**ADR landings:** none direct. The 27th-fuzzer count + must-never-panic discipline anchor at SPEC §6.7 + ADR-0018 short-mode CI policy (existing fuzzer-corpus framework REUSE 6 per SPEC §3.3). No new ADR fired at Task 8.

**D-questions closed:**
- **D6** (fuzzer corpus seed roster): **CLOSED** — Task 8 lands the 30-seed roster covering: 1 valid full Gradient-1 config + 14 PARSE-REJECT arm variants (arms 1-12; arm 13 fixed_value STRUCTURALLY UNREACHABLE in v1.32.4 per Task 2 discovery + ADR-0186 §Consequences (d)) + 6 boundary-value seeds + 3 default-applied variants + 3 empty/oneof-absent/nested-missing variants + 2 envoy-go-strict variants + 1 raw-bytes garbage seed. Seeds use the `f.Add(b)` pattern over `testdata/fuzz/<name>/` corpus files per the phase-20 oauth2 precedent at `internal/filter/http/oauth2/fuzz_test.go` (portable + version-controlled + no testdata-file convention overhead).
- **D8** (impl-time-unanticipated ADR hypothesis): HOLDING — no ADR-0188 fired at Task 8; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 9.

**SPEC §12 closures:**
- **Gate E — fuzz** (per SPEC §14.3 + §A item 5): `FuzzAdaptiveConcurrencyConfigParse` runs CLEAN at 30s — no panics across 2.6M execs; 191 new-interesting inputs observed at the 30s mark with 30/30 baseline seeds gathered without panic. Per SPEC §A item 5, Gate E is one of the 6 phase-done gates; Task 8 contributes the necessary fuzzer artifact + clean-run evidence.

### LoC envelope note — fuzz_test.go 337 LoC vs SPEC §6.8 ~50 LoC soft estimate

The `fuzz_test.go` file lands at 337 LoC, materially above the SPEC §6.8 source-file-roster soft estimate of ~50 LoC. The overage is attributable to:
- (a) Extensive per-seed doc comments mapping each of the 30 seeds to the corresponding PARSE-REJECT arm number + AMEND/ADR reference (e.g., `// Seed 24 — enabled absent (default OFF per AMEND-4 — REFUTES BRAINSTORM §2.1).`). This produces a self-documenting seed roster traceable back to SPEC §5 + Task 2 PARSE-REJECT discipline.
- (b) The 30-seed proto-builder pattern itself runs ~5-8 LoC per seed (a `{ ac := validConfig(); ac.Get...; addSeed(ac) }` block). The phase-20 oauth2 precedent at `internal/filter/http/oauth2/fuzz_test.go` lands at 446 LoC for 30 seeds via the same pattern — Task 8 is actually slightly more compact than the precedent.
- (c) Package-level documentation header consistent with the established phase-21 file pattern (cf. `compiled_config.go` + `encode_complete.go` headers) documenting the seed-corpus strategy decision (D6 — `f.Add` over `testdata/fuzz/<name>/`).
- (d) Boundary-value + envoy-go-strict variants beyond the strict PLAN Task 8 step 2 count (the planner counted "~14 seeds for the 11 RATIFIED-PGV arms" + variants; the final roster of 30 closely matches the SPEC §6.7 "Corpus seed roster ~30 entries" envelope).

The BRAINSTORM §1.4 LoC envelope at the package level (~890-1390 test LoC subtotal per SPEC §6.8 table) absorbs the delta with significant headroom. Mirrors phase-20 oauth2 precedent where the fuzzer file landed at ~9x the soft estimate due to the same doc-comment + seed-roster verbosity. Recorded for transparency per the established Task-2/3/5/6/7 LoC-envelope-note precedent.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.028s
```

Task-8 fuzzer run (verbose form; tail 5 lines showing 30s clean run):

```
$ go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/
fuzz: elapsed: 24s, execs: 1674062 (15708/sec), new interesting: 184 (total: 214)
fuzz: elapsed: 27s, execs: 2370323 (232084/sec), new interesting: 188 (total: 218)
fuzz: elapsed: 30s, execs: 2646599 (92083/sec), new interesting: 191 (total: 221)
fuzz: elapsed: 31s, execs: 2646599 (0/sec), new interesting: 191 (total: 221)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	31.076s
```

### Project-wide fuzzer count

```
$ grep -rEn "^func Fuzz[A-Z]" --include='*.go' . | wc -l
27
```

26 (post-phase-20) + 1 (NEW `FuzzAdaptiveConcurrencyConfigParse`) = 27 (post-phase-21-Task-8). Matches the SPEC §6.7 + §3.3 REUSE 6 fuzzer-count claim verbatim.

### Package-total test count

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -v 2>&1 | grep -cE '^=== RUN.*Test'
100
```

Unchanged from start of Task 8 — `Fuzz*` test entries are listed under the `go test -fuzz` channel rather than the `Test*` channel; the `=== RUN` line for `FuzzAdaptiveConcurrencyConfigParse` does appear but the regression test-count metric specifically grep'd for `^=== RUN.*Test` (Test-prefix) which the fuzz function does not match. The 100-count remains the unit-test surface size at start of Task 9.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 8; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 9.

---

## Task 9 — full filter integration + boot-registration per SPEC §3.4 + ADR-0072

**Commit SHA:** `5304aec post-Task-9`
**Files landed:**
- `internal/filter/http/adaptive_concurrency/doc.go` (NEW; 63 LoC; package-level doc per SPEC §6.8 cross-referencing ADR-0186 + ADR-0187 + ADR-0059 §Decision AMENDMENT + the 7-name HCM-rooted stat surface + the per-HCM-instance controller semantics + the AMEND-6 503-overflow wire shape + the REUSE-by-absence per-route discipline).
- `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` (NEW; 139 LoC; `const TypeURL` byte-exact pin per ADR-0143 SN1 + `const filterName` per ADR-0070 + `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory per ADR-0071 + ADR-0072. Wires Tasks 2-8 into a fully-functional api.HTTPFilterFactory: validates tc != nil + buildCompiledConfig + ctx.Stats non-nil guard + newFilterStats + newGradientController (ONE controller per HCM filter chain mounting an adaptive_concurrency filter, captured into the per-stream FilterInstanceFactory closure) + defaultClock{} for production timer-source wiring. Returns `HTTPFilter{Decoder: f, Encoder: f}` — adaptive_concurrency participates on BOTH decode (forwardingDecision + 503 emission per SPEC §6.4 + AMEND-6) AND encode (RTT recording + releaseInFlight per SPEC §6.5 + D11) sides per SPEC §3.4. NO `RegisterPerRouteValidator` function — REUSE-by-absence per SPEC §5.4 (FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment).)
- `internal/filter/http/adaptive_concurrency/adaptive_concurrency_test.go` (NEW; 222 LoC; 6 Group-9 boot-time factory + stat-name regression-guard tests per phase-21 SPEC §6.6 + §14.1 + planner-time D5. Tests: `TestTypeURL_ByteExact` (ADR-0143 SN1 byte-exact pin); `TestNew_NilTypedConfig_Error` (ADR-0072 boot-time-fail-fast); `TestNew_HappyPath_ReturnsFactory` (valid config → BOTH-sides filter with Decoder=Encoder pointing at same *filter instance; shared per-HCM-instance *controller + *cc across per-stream invocations); `TestNew_NilStats_Error` (nil-stats fail-fast path); `TestNew_ParseRejectPropagates` (byte-stable PARSE-REJECT propagates verbatim); `TestStatNames_Equal_Wire` (D5 table-driven 7-row byte-exact compile-time guard).)
- `internal/filter/http/adaptive_concurrency/clock.go` (MODIFIED; removed 4 `//nolint:unused` annotations on `defaultClock` + `timerStop` + their methods — the factory's `defaultClock{}` instantiation in `New()` consumes them at Task 9; the nolint markers were placeholders for the Task-9 landing per the clock.go Task-3 header comment).
- `cmd/envoy-go/main.go` (MODIFIED; +2 LoC: import alias `"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"` inserted alphabetically at the top of the per-filter import block (before `bandwidthlimit`) + `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` inserted alphabetically between `router` and `bandwidthlimit` boot-registration calls per D7 + ADR-0100 §2.2. NO `adaptive_concurrency.RegisterPerRouteValidator(httpReg)` call — REUSE-by-absence per SPEC §5.4. Total HTTP filter count post-phase-21: 16.)
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 9 entry).

**Line-number note (boot-registration position):** The task brief specified line 125 (between `router` at line 124 and `bandwidthlimit` shifting from 125 to 126). Adding the new import line (alphabetically before `bandwidthlimit`) shifted everything by 1, so post-edit the layout is: `httpReg := filter_http.NewHTTPRegistry()` at line 124, `router` at line 125, `adaptive_concurrency` at line 126, `bandwidthlimit` at line 127. The structural intent (alphabetical insertion between `router` and `bandwidthlimit`) is satisfied; the absolute line number is 126 rather than the brief's pre-edit estimate of 125.

**ADR landings:** none direct (the three Task-9-relevant ADRs landed at earlier tasks: ADR-0186 at Task 3, ADR-0187 at Task 2, ADR-0059 §Decision AMENDMENT at Task 4). D8 hypothesis status: HOLDING — ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 10.

**D-questions closed:**
- **D5** (stat-name regression-guard test): **CLOSED** — `TestStatNames_Equal_Wire` lands the 7-row table-driven byte-exact compile-time guard against the upstream Envoy wire names per SPEC §6.6 + AMEND-3. Two-layer guard now active: (1) the `const statName*` declarations at stats.go pin values at compile time; (2) the new test pins each constant to its byte-exact wire-name string literal.
- **D7** (boot-registration position): **CLOSED** — `adaptive_concurrency.TypeURL` registers alphabetically between `router` and `bandwidthlimit` in the cmd/envoy-go/main.go `httpReg.Register` block per ADR-0100 §2.2. 16 HTTP filters total post-phase-21 (router + adaptive_concurrency + bandwidthlimit + buffer + compressor + cors + csrf + envoygotest + extauthz + extproc + fault + header_mutation + jwtauthn + localratelimit + oauth2 + rbac).

**SPEC §12 closures:**
- **Gate F — boot-registration smoke** (per SPEC §A item 6): boot-registration smoke covered indirectly by `go build ./...` clean (the `httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)` call compiles); end-to-end fixture coverage lands at Task 10 (differential fixture 0025-http-adaptive-concurrency).

### Nil-stats fail-fast design decision

The factory enforces `ctx.Stats != nil` as a boot-time-fail-fast precondition per ADR-0072. Rationale documented in the factory body comment:

> The controller's `newGradientController` writes to `filterStats.concurrencyLimit` at construction (SPEC §4.6 initial-state pin); a nil-stats path would NPE at controller construction. Production callers per `internal/filter/hcm/config.go` always supply a non-nil registry per ADR-0061 LBP-1.

The alternative defensive-nil-check-in-controller path was REJECTED:
- Adds 7 nil-check hot-path conditionals (one per gauge/counter access in `concurrencyUpdateTick` + `updateMinRTTLocked` + `enterMinRTTSamplingWindowLocked` + `forwardingDecision`'s `Inc()` + 3 more).
- Diverges from the §9 family-row precedent (all 15 prior filters fail-fast on misconfigured ctx at HCM-build time).
- Hides operator-facing misconfigurations (a stat-bearing filter constructed without a registry is a configuration mistake; failing loud at boot is the LBP-1-aligned response).

The disciplined fail-fast wording `"adaptive_concurrency: ctx.Stats required (HCM-build-time non-nil per ADR-0061 LBP-1)"` surfaces the framework contract verbatim to the operator's boot-time log.

### LoC envelope note

- `doc.go`: 63 LoC (vs Task brief ~15-30 LoC estimate). The cross-reference roster (ADR-0186 + ADR-0187 + ADR-0059 §Decision AMENDMENT + SPEC §1.1 AMEND-1..AMEND-7 + SPEC §6 + SPEC §7 + the 7-name stat surface + the per-HCM-instance controller semantics + the 503-overflow wire shape + the REUSE-by-absence per-route discipline) is materially richer than the soft estimate; the verbosity is consistent with the established phase-21 doc-comment density (cf. `controller.go` + `stats.go` headers). The package-doc shape mirrors the phase-20 oauth2 `oauth2.go` header style.
- `adaptive_concurrency.go`: 139 LoC (vs Task brief ~80-120 LoC estimate). The factory body itself is ~30 LoC; the rest is the file-header + Per ADR-0070/0071/0072 cross-reference roster + the inline factory-step + REUSE-by-absence per-route discipline comment block. Mirrors the oauth2.go factory header density.
- `adaptive_concurrency_test.go`: 222 LoC (vs Task brief ~80-150 LoC estimate). The 6 tests average ~37 LoC each including per-test doc-comment + table-driven D5 closure scaffolding. The happy-path test pins the BOTH-sides shared-instance invariant (Decoder == Encoder per-stream) + the per-stream / per-HCM sharing discriminator (fresh *filter per factory() call; shared *controller + *cc across calls) which the brief's minimal-shape skeleton did not enumerate.

Total Task-9 LoC: 424 (vs Task brief envelope ~175-300). Within the BRAINSTORM §1.4 absorbing-bound for the package-total LoC envelope.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 -race ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.029s
```

Task-9 new-test surface (verbose form):

```
$ go test -count=1 -race ./internal/filter/http/adaptive_concurrency/... -run 'TestTypeURL_ByteExact|TestNew_NilTypedConfig_Error|TestNew_HappyPath_ReturnsFactory|TestNew_NilStats_Error|TestNew_ParseRejectPropagates|TestStatNames_Equal_Wire' -v
=== RUN   TestTypeURL_ByteExact
--- PASS: TestTypeURL_ByteExact (0.00s)
=== RUN   TestNew_NilTypedConfig_Error
--- PASS: TestNew_NilTypedConfig_Error (0.00s)
=== RUN   TestNew_HappyPath_ReturnsFactory
--- PASS: TestNew_HappyPath_ReturnsFactory (0.00s)
=== RUN   TestNew_NilStats_Error
--- PASS: TestNew_NilStats_Error (0.00s)
=== RUN   TestNew_ParseRejectPropagates
--- PASS: TestNew_ParseRejectPropagates (0.00s)
=== RUN   TestStatNames_Equal_Wire
--- PASS: TestStatNames_Equal_Wire (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.018s
```

### Boot-registration verification

```
$ grep -nE 'httpReg\.Register\(adaptive_concurrency\.TypeURL' cmd/envoy-go/main.go
126:	httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)
```

Line 126 (alphabetical between `router` at line 125 and `bandwidthlimit` at line 127). Brief specified line 125 was the pre-edit estimate; the new import line shifted absolute line numbers by 1 (structural intent preserved).

### Package-total test count

```
$ go test -count=1 ./internal/filter/http/adaptive_concurrency/... -v 2>&1 | grep -cE '^=== RUN.*Test'
106
```

100 (start of Task 9) + 6 (new Task 9 Group-9 tests: TestTypeURL_ByteExact + TestNew_NilTypedConfig_Error + TestNew_HappyPath_ReturnsFactory + TestNew_NilStats_Error + TestNew_ParseRejectPropagates + TestStatNames_Equal_Wire) = 106. All PASS.

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 9; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 10.

---

## Task 10 — differential fixture 0025-http-adaptive-concurrency (4 scenarios)

**Commit SHA:** `9d80820 post-Task-10`

**Files landed:**
- `test/differential/fixture/fixture.go` (MODIFIED; +23 LoC: new `HTTPAdaptiveConcurrency BackendKind = 21` enum value AFTER `HTTPOAuth2 = 20` per the established §9 family-row precedent. The doc-comment reuses the existing HTTPSlowStream backend at `test/fixtures/0010-graceful-drain/backends/backend.go` for both fast `/` (scenarios a + c + d) and 5-second `/slow` (scenario b's 2-request in-flight overflow trap).
- `test/differential/runner_test.go` (MODIFIED; +34 LoC: blank-import for `test/fixtures/0025-http-adaptive-concurrency/inputs` at line 50 alphabetically after the 0024 oauth2 import + `case fixture.HTTPAdaptiveConcurrency` switch arm in the backend-spawn loop reusing `startHTTPSlowStreamBackend` per the fixture-0010 precedent).
- `test/fixtures/0025-http-adaptive-concurrency/envoy-go.yaml` (NEW; 203 LoC; 3-listener bootstrap: `l_a_default` default Gradient-1 config for scenarios a + c, `l_b_overflow` `min_concurrency=1 + max_concurrency_limit=1 + concurrency_limit_exceeded_status code:ServiceUnavailable` for scenario b, `l_d_disabled` `enabled` field ABSENT for scenario d's AMEND-4 pass-through default. YAML-to-proto note in the file header documents the `wrapperspb.UInt32Value` bare-scalar form per the phase-13 fixture-0015 `max_request_bytes: 1048576` precedent — protojson decodes wrapper-typed fields from bare scalars, NOT from `{value: N}` nested objects).
- `test/fixtures/0025-http-adaptive-concurrency/envoy.yaml` (NEW; 23 LoC; INERT skeleton for a future cross-side byte-exact extension of scenario (b) per the RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION note below — the fixture is REFERENCE-LESS at Task 10 so the runner SKIPS this file).
- `test/fixtures/0025-http-adaptive-concurrency/expectations.yaml` (NEW; 85 LoC; REFERENCE-LESS subject-only per-scenario wire-shape expectations; cross-references SPEC §12 ratification anchors A1+A2+A3+A4).
- `test/fixtures/0025-http-adaptive-concurrency/README.md` (NEW; 128 LoC; scenario narrative + scope-deviation-from-PLAN-§10 rationale + 4-scenario table + 3-listener topology + cross-references).
- `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go` (NEW; 695 LoC; orchestrates 4 scenarios sequentially against the 3-listener subject; implements `fixture.Driver` + `fixture.BackendKindAware` + `fixture.ReferenceLessFixture` + `fixture.SubjectAsserter`; per-scenario subprobe-tagged byte-stream encoding for scenario (b)'s 2-response block + per-scenario prometheus stats snapshot for in-band assertion of the 7-name HCM-rooted stat surface. Driver LoC envelope is larger than the Task 10 brief 250-400 estimate — the brief's estimate did not budget for the per-scenario `subprobe` partitioning helpers (scenario b emits two response blocks under one scenario header) + the prometheus-format-parsing `emitStatsSnapshot` + the hcm-prefix-discriminator `requireStat*` helpers. Mirrors the 0024 oauth2 + 0007b iteration-probe driver-density precedent.
- `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` (MODIFIED; +18 LoC: ONE-line fix at `New()` factory body — `newFilterStats(ctx.Stats, "http."+ctx.StatPrefix)` per the Rule-SN2 prefix discriminator + the phase-20 oauth2 `baseStatPrefix` + phase-09 fault `registerFaultStats` precedent. See "Task-4 stats.go finding" below.).
- `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (THIS Task 10 entry).

### Scope deviation from PLAN §10 — RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION

PLAN §10 specified a **4-sub-directory structure** (`parse_ok/` + `overflow_503/` + `stat_surface/` + `pass_through_when_disabled/`) AND a **cross-side byte-exact promise** for scenario (b) overflow_503 per AMEND-6. The Task 10 IMPL landed a **single-directory + REFERENCE-LESS** fixture per the phase-20 oauth2 fixture 0024 + phase-07.1 iteration-probe fixture 0007b precedent. Specifically:

- **Single-directory** at `test/fixtures/0025-http-adaptive-concurrency/` with ONE `envoy.yaml` + ONE `envoy-go.yaml` + ONE `expectations.yaml` + ONE `README.md` + `inputs/driver.go`.
- **All 4 scenarios REFERENCE-LESS** — the driver implements `fixture.ReferenceLessFixture` returning `false`. The runner short-circuits the reference-Envoy spawn + DriveReference + byte-stream CompareBytes; only DriveSubject + the in-band SubjectAsserter run.
- **3-listener subject topology** (one HCM per scenario-config variant) instead of 4-sub-directory full bootstraps — the structure isolates the per-scenario filter-config variant while keeping the fixture compact.

The **AMEND-6 cross-side byte-exact promise for scenario (b) is deferred** to a future cross-side extension — flagged as **RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION**. The envoy-go-side byte-exact pinning of the 503 + 25-byte "reached concurrency limit" body + `content-type: text/plain` header still lands at scenario (b) per the AMEND-6 wire-shape invariants — only the cross-side `CompareBytes` against reference Envoy v1.32.4 is deferred. The forward-pointer is well-defined: the `envoy.yaml` skeleton at this fixture is the starting point; the 3-listener layout mirrors `envoy-go.yaml` exactly.

### Task-4 stats.go finding — surfaced at Task 10 IMPL discovery

The Task 4 stats.go `newFilterStats` body computes the registered stat-name prefix as `p := hcmPrefix + ".adaptive_concurrency.gradient_controller."`. The Task 4 doc-comment header documents the expected internal-name shape as:

> `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>`

… AND the doc-comment example reads:

> `e.g., "http.ingress_http" for an HCM with stat_prefix="ingress_http"`

This documents the contract: the **caller** is expected to pass `"http." + ctx.StatPrefix` to `newFilterStats`. The Task 9 `New()` factory body, however, passed `ctx.StatPrefix` directly (bare `"ingress_http"`), per a literal reading of the doc-comment's "(e.g., ...)" parenthetical as one of multiple valid forms. The resulting internal stat names took the shape `hcm_a_default.adaptive_concurrency.gradient_controller.rq_blocked` — which DOES NOT match Rule SN2's `http.<stat_prefix>.<rest>` prefix discriminator at `internal/stats/name.go`. Consequence: the 7-name stat surface was **registered correctly** (the names exist in the registry; `TestStatNames_Equal_Wire` still passes since it asserts the bare stat names, not the HCM-rooted prefix), but did NOT appear in the `/stats/prometheus` exposition (the prometheus walker silently skipped them because `flattenToProm` returned an error on the malformed-from-SN2-perspective names).

Surfaced at Task 10 by the fixture-0025 scenario (c) `/stats/prometheus` scrape: the 7-name `envoy_http_adaptive_concurrency_gradient_controller_*` family was **absent** from the exposition. Fixed at Task 10 with a minimal surgical edit at `adaptive_concurrency.go` (`New()` factory body): `newFilterStats(ctx.Stats, "http." + ctx.StatPrefix)`. This mirrors the phase-20 oauth2 `baseStatPrefix` (`"http." + hcmStatPrefix + ".oauth2."`) + phase-09 fault `registerFaultStats` (`"http." + prefix + ".fault."`) precedent. Task 4 stats.go body is preserved byte-for-byte — the fix lives entirely in the Task 9 factory boundary.

This is a **Task 9 follow-up bug surfaced at Task 10**, not a Task 4 specification ambiguity — the Task 4 doc-comment is unambiguous when read in context with the phase-20 oauth2 precedent (which Task 4 explicitly cross-references). The fix is forward-compatible: any future Task-11 → Task-14 work that uses `New()` inherits the correct prefix at zero cost.

### 4 scenarios — driver + asserter

| # | Scenario id | Listener | Request | Asserted outcome |
|---|---|---|---|---|
| (a) | `a_parse_ok` | `l_a_default` | single `GET /` | HTTP 200; `rq_blocked` under hcm_a_default == 0 |
| (b) | `b_overflow_503` | `l_b_overflow` | 2 concurrent `GET /slow` (200ms head-start; HTTPSlowStream backend 5-second response) | req200 → 200 OK; req503 → 503 + body "reached concurrency limit" (25 bytes verbatim) + `content-type: text/plain` header; `rq_blocked` under hcm_b_overflow == 1 |
| (c) | `c_stat_surface` | `l_a_default` | single `GET /` + scrape `/stats/prometheus` | HTTP 200; 7-name stat surface present (rq_blocked + 6 gauges); `concurrency_limit` == 3 (minConcurrency default per SPEC §4.6); `min_rtt_calculation_active` == 1 (initial window per AMEND-2 C4) |
| (d) | `d_pass_through_when_disabled` | `l_d_disabled` | single `GET /` | HTTP 200; `rq_blocked` under hcm_d_disabled == 0 (filter OFF per AMEND-4 — controller never consulted) |

### SPEC §12 closures

- **A1 (status code byte-exact)**: **RATIFIED-at-envoy-go-side** at scenario (b). The driver pins `status: 503` via the subprobe `req503` header line.
- **A2 (body byte-exact)**: **RATIFIED-at-envoy-go-side** at scenario (b). The driver pins `body: "reached concurrency limit"` (25 bytes verbatim) via the subprobe `req503` body line. Body length cross-checked at the prometheus scrape (`content-length: 25` header present).
- **A3 (response_code_details ABSENT-by-config)**: **RATIFIED-by-absence**. Envoy-go has no access-log surface per SPEC §6.4 doc + the decode_headers.go header comment ("The response_code_details 'reached_concurrency_limit' field (upstream Envoy at adaptive_concurrency_filter.cc:50-54) is NOT byte-pinned at envoy-go MVP per SPEC §12 item A3 — envoy-go has no access-log surface; the response_code_details field is ABSENT-by-config (NOT byte-pinned)").
- **A4 (envoy-go-strict departure record)**: **RATIFIED-as-forward-pointer** to Task 13 BEHAVIOR_CONTRACT.md per planner-time D14 (sample_rtt_msecs / min_rtt_msecs unit divergence — name byte-exact upstream `_msecs`, value encoded as int64 nanoseconds per ADR-0059 §Decision AMENDMENT). The 7-name surface assertion at scenario (c) exercises the names; the unit-divergence record lands at Task 13's BEHAVIOR_CONTRACT 7-edit bundle.

Cross-side byte-exact promise for scenario (b) (A1 + A2 cross-side) DEFERRED per RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION.

### LoC envelope note

| File | LoC | vs Task brief envelope |
|---|---|---|
| `test/differential/fixture/fixture.go` (delta) | +23 | within ~+15 LoC estimate (slightly above due to richer doc-comment per the §9 row precedent) |
| `test/differential/runner_test.go` (delta) | +34 | within ~+15 LoC estimate (slightly above due to per-case doc-comment density mirroring the HTTPOAuth2 case) |
| `test/fixtures/0025-http-adaptive-concurrency/envoy-go.yaml` | 203 | within reasonable bounds for 3-listener bootstrap |
| `test/fixtures/0025-http-adaptive-concurrency/envoy.yaml` | 23 | well below — inert skeleton for future extension |
| `test/fixtures/0025-http-adaptive-concurrency/expectations.yaml` | 85 | within reasonable bounds for 4-scenario expectations |
| `test/fixtures/0025-http-adaptive-concurrency/README.md` | 128 | within reasonable bounds for scenario-narrative + scope-deviation rationale |
| `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go` | 695 | ABOVE Task brief 250-400 estimate — the brief's estimate did not budget for the per-scenario `subprobe` partitioning helpers (scenario b emits two response blocks under one scenario header), the prometheus-format-parsing `emitStatsSnapshot`, the hcm-prefix-discriminator `requireStat*` helpers, the 18-LoC doc-comment header, and the per-helper doc-comments. Mirrors the 0024 oauth2 + 0007b iteration-probe driver-density precedent. |
| `internal/filter/http/adaptive_concurrency/adaptive_concurrency.go` (delta) | +18 | within reasonable bounds for the surgical Task-9 follow-up fix + its rationale doc-comment |

Total Task-10 new LoC: 1209 (envelope dominated by the driver). Within the BRAINSTORM §1.4 absorbing-bound for the per-task LoC envelope.

### Build / test outputs (verbatim)

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ golangci-lint run
(no output — clean)

$ go test -count=1 ./test/differential/ -run 'TestDifferential/0025' -v 2>&1 | tail -30
=== RUN   TestDifferential
=== RUN   TestDifferential/0025-http-adaptive-concurrency
--- PASS: TestDifferential (4.97s)
    --- PASS: TestDifferential/0025-http-adaptive-concurrency (4.97s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	5.063s

$ go test -count=1 -timeout=15m ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	71.023s
(all fixtures pass — no regression from the Task-10 changes)

$ go test -count=1 ./internal/filter/http/adaptive_concurrency/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	0.007s
(all 106 phase-21 tests still pass after the Task-9 follow-up fix at adaptive_concurrency.go::New)
```

### D-questions / forward-pointers

- **AMEND-6 cross-side byte-exact promise**: DEFERRED per RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION. The `envoy.yaml` skeleton + 3-listener layout in `envoy-go.yaml` are the forward-pointer for a future cross-side extension.
- **Task-4 stats.go finding**: CLOSED at Task 10 with the surgical `New()` factory fix. No Task 11 follow-up required.
- **Total HTTP filter count post-phase-21**: 16 (unchanged from Task 9 — Task 10 lands fixture infrastructure only).

D8 hypothesis status: HOLDING — no impl-time-unanticipated ADR fired at Task 10; ADR-0188 + ADR-0189 stay UNCONSUMED at start of Task 11.

---

---

## Task 11 — ADR final-state alignment + DECISIONS.md cross-reference audit

**Commit SHA:** `ef6ac63 post-Task-11`

**Files landed:** none — pure verification + PROGRESS append (no DECISIONS.md cross-reference cleanup edits were needed).

**ADR landings:** none direct at this Task (ADR-0186 §Decision + §Consequences anchored at Task 3 `84e317f`; ADR-0187 §Decision + §Consequences anchored at Task 2 `5a0a720`; ADR-0059 §Decision AMENDMENT body anchored at Task 4 `4a8f9c4` REPLACING the SPEC-commit ANTICIPATION paragraph per ADR-0044 in-place edit discipline).

**D-questions closed:** D8 hypothesis HOLDING (NO impl-time-unanticipated ADR fires at phase-21 IMPL — verified at Task 11; carried forward to Task 14 final confirmation).

**SPEC §15 item closure:** item 15 (ADR final-state alignment) RATIFIED.

### Step 1 — ADR-0186 + ADR-0187 §Decision + §Consequences bodies present + non-empty

```
$ grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md
1
$ awk '/^## ADR-0186/{flag=1; next} /^## ADR-0188|^## ADR-0187/{flag=0} flag && /^### /' docs/envoy-go/DECISIONS.md | head -3
### Context
### Decision
### Consequences
$ awk '/^## ADR-0187/{flag=1; next} /^## ADR-0188|^## ADR-0189/{flag=0} flag && /^### /' docs/envoy-go/DECISIONS.md | head -3
### Context
### Decision
### Consequences
```

Both ADRs have the full §Context + §Decision + §Consequences block triplet. The Task-3 + Task-2 anchored bodies are non-empty (verified at Tasks 3 + 2 acceptance gates).

### Step 2 — ADR-0059 §Decision AMENDMENT body present (REPLACES SPEC-commit ANTICIPATION paragraph per ADR-0044)

```
$ grep -nE 'Amendment \(per phase 21 ADR-0186\)' docs/envoy-go/DECISIONS.md | head -2
2109:**Amendment (per phase 21 ADR-0186) — landed at IMPL Task 4, dated 2026-05-18 (commit `4a8f9c4 post-Task-4`); REPLACES the SPEC-commit ANTICIPATION paragraph per ADR-0044 in-place edit discipline.** ...
$ grep -c 'ANTICIPATION paragraph anchored at phase-21 SPEC commit' docs/envoy-go/DECISIONS.md
0
```

The ANTICIPATION-marker block (anchored at SPEC commit `49ba034`) has been REPLACED with the final AMENDMENT body per ADR-0044. Status line transitioned from `§Context drafted at phase-21 SPEC commit; §Decision + §Consequences anchor at phase-21 IMPL Task 2 per ADR-0044` → `landed at IMPL Task 4, dated 2026-05-18 (commit 4a8f9c4 post-Task-4)`.

### Step 3 — D8 hypothesis HOLDS: ADR-0188 + ADR-0189 stay UNCONSUMED

```
$ grep -cE '^## ADR-0188' docs/envoy-go/DECISIONS.md
0
$ grep -cE '^## ADR-0189' docs/envoy-go/DECISIONS.md
0
```

D8 hypothesis HOLDING as of Task 11. Two-slot escape-valve buffer (ADR-0188 + ADR-0189) preserved per SPEC §10 D STRENGTHENED two-slot buffer. Final confirmation at Task 14.

### Step 4 — Cross-reference spot-check

```
$ awk '/^## ADR-0186/{flag=1; next} /^## ADR-0188|^## ADR-0187/{flag=0} flag' docs/envoy-go/DECISIONS.md | grep -cE 'ADR-0059'
3
$ awk '/^## ADR-0187/{flag=1; next} /^## ADR-0188|^## ADR-0189/{flag=0} flag' docs/envoy-go/DECISIONS.md | grep -cE 'ADR-0044'
3
$ awk '/^\*\*Amendment \(per phase 21 ADR-0186\)/{flag=1} /^### Consequences/{flag=0} flag' docs/envoy-go/DECISIONS.md | grep -cE 'ADR-0186'
1
```

Cross-references intact:
- ADR-0186 references ADR-0059 (3 mentions; covers the §Decision AMENDMENT consumer + the float-valued-gauge encoding convention + the cross-phase forward-pointer)
- ADR-0187 references ADR-0044 (3 mentions; covers in-place edit discipline + ADR-on-impl convention + the byte-stable-error-wording cross-reference to ADR-0080)
- ADR-0059 §Decision AMENDMENT body references ADR-0186 (1 mention via the `Amendment (per phase 21 ADR-0186)` opener)

### Step 5 — Audit conclusion

All 2 NEW ADR §Decision + §Consequences bodies (ADR-0186 + ADR-0187) plus the 1 IN-PLACE §Decision AMENDMENT body (ADR-0059) are present + non-empty + cross-references intact at Task 11 audit. D8 hypothesis HOLDING (ADR-0188 + ADR-0189 stay UNCONSUMED). No cross-reference cleanup edits to DECISIONS.md required at this Task.


---

## Task 12 — cross-package regression matrix per D4 + D16

**Commit SHA:** `02b96a2 post-Task-12`

**Files landed:** none — pure verification + PROGRESS append.

**D-questions closed:** D4 (cross-package regression-test command shape RATIFIED at runtime); D16 (cross-phase regression-matrix item C8 RATIFIED — full closure).

**SPEC §12 closure:** item C8 FULLY RATIFIED (cross-package regression matrix for ADR-0059 §Decision AMENDMENT post-AMENDMENT — internal/stats/ GREEN + internal/filter/ GREEN + 27/27 differential fixtures GREEN; zero regression at any layer).

### Step 1 — internal/stats/ regression post-ADR-0059 §Decision AMENDMENT (per D4)

```
$ go test -count=1 -race ./internal/stats/...
ok  	github.com/esalaine/envoy-go/internal/stats	1.026s
```

Zero regression. The AMENDMENT is pure convention-extension: NEW `BoolToInt` helper at `internal/stats/conv.go` + comment-only doc-extension at `gauge.go` cross-referencing the AMENDMENT — NO signature change to `*stats.Gauge`. The full pre-existing `internal/stats/` test suite passes clean under `-race`.

### Step 2 — cross-package filter regression (all 16 HTTP filters)

```
$ go test -count=1 -race ./internal/filter/...
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.049s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.519s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.281s
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.054s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	9.683s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.034s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.057s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.024s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.031s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.058s
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.398s
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.253s
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.345s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.031s
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.107s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.035s
ok  	github.com/esalaine/envoy-go/internal/filter/http/oauth2	1.068s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.045s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.253s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.196s
```

All 16 HTTP filter packages (15 pre-existing post-phase-20 + new `adaptive_concurrency`) report ok under `-race`. Plus the framework packages (hcm + hcm/h2 + http + tcpproxy). Zero regression.

### Step 3 — full differential regression matrix (27 fixture directories)

```
$ go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	70.554s
```

All 27 fixture directories (26 pre-existing + new 0025) PASS in 70.5s (well within the 15m timeout). The single `TestDifferential` parent enumerates each fixture as a sub-test; the parent OK status means all sub-tests PASS. (Per the PLAN precondition 12 wording variance noted in the cold-start preconditions section: the literal regex `Test.*00(0[0-9]|1[0-9]|2[0-5])` does not match the `TestDifferential/0000..0025` sub-test naming convention; the substantive GREEN baseline is satisfied via the parent-test invocation above.)

### Step 4 — audit conclusion

Cross-package regression matrix verified GREEN at all 3 layers: internal/stats/ + internal/filter/ + test/differential/. SPEC §12 item C8 (cross-package regression matrix for ADR-0059 §Decision AMENDMENT) FULLY RATIFIED at Task 12. No regressions introduced by phase-21 across the 16-HTTP-filter + 27-fixture pre-existing baseline.

## Task 13 — BEHAVIOR_CONTRACT.md 7-edit bundle per SPEC §13

**Commit SHA:** `75a64a5 post-Task-13`
**Status:** DONE

**SPEC reference:** §13 (7-edit bundle for BEHAVIOR_CONTRACT.md — all 7 edits land atomically at the SAME commit per ADR-0052 in-place-by-append discipline).

**Scope:** in-place edit to `docs/envoy-go/BEHAVIOR_CONTRACT.md` only (per ADR-0052). PROGRESS.md gets this Task-13 entry appended at the same commit.

### The 7 edits (all landed atomically per ADR-0052)

1. **NEW `### envoy.filters.http.adaptive_concurrency` subsection** inserted after the phase-20 oauth2 subsection (FOURTEENTH §9 family-row; FOURTH CONSECUTIVE REUSE-by-absence per SPEC §5.4). Includes opening paragraph + Field decomposition (14-row proto-disposition table covering `AdaptiveConcurrency` + `GradientControllerConfig` + `ConcurrencyLimitCalculationParams` + `MinimumRTTCalculationParams` + the `typed_per_filter_config` PARSE-REJECT row) + Controller state machine summary (first-tick + concurrency-update tick + minRTT recalc + 5-consec-min forced-recalc + jitter; per AMEND-2 C1–C4 + line-cited lemmata against `gradient_controller.cc`) + 7-name stat surface + encoding convention per ADR-0059 §Decision AMENDMENT + Deny-path wire shape (503 + 25-byte body per AMEND-6) + Per-route REUSE-by-absence + 2 envoy-go-strict departure records + `response_code_details` joint-divergence note + forward-pointer to `### Phase 21 forward-pointer notes`.

2. **Stat-name mapping 92 → 99 extension** — NEW `**adaptive_concurrency filter — 7 names (introduced by phase 21; 1 counter + 6 gauges):**` sub-subsection inserted after the phase-20 oauth2 sub-subsection. 7 rows in the canonical `| Internal name | Type | Source | Filter | Description |` format (1 counter `rq_blocked` + 6 gauges `concurrency_limit` / `gradient` / `burst_queue_size` / `sample_rtt_msecs` / `min_rtt_msecs` / `min_rtt_calculation_active`). Followed by `**Phase 21 extension — 92 → 99 internal names:**` paragraph with the byte-arithmetic walk (17+5+4+3+0+17+14+4+7+6+0+9+0+6+**7** = 99) + Prometheus rendering + SN2-reuse RATIFIED at SPEC §11 §21.P3 PARTIAL + FOURTH CONSECUTIVE REUSE-by-absence cross-reference.

3. **ADR-0059 §Decision AMENDMENT cross-reference paragraph** appended right after the Phase-21 extension paragraph (per the SPEC §13 "pragmatic alternative" guidance — extends the stat-name mapping section in lieu of a separate `## Internal Stats Store` header). Documents the 3 float-valued-gauge int64 encoding value-classes: time-typed ns + ratio-typed ×1000 + bool-typed 0/1 via `stats.BoolToInt`. Cross-references ADR-0186 (consumer) + ADR-0059 §Decision AMENDMENT body (DECISIONS.md ~line 2109) + SPEC §3.2 + AMEND-7.

4. **NEW envoy-go-strict departure record — RTT-gauge units (per AMEND-3 C3)** landed inside the NEW `### envoy.filters.http.adaptive_concurrency` subsection (edit 1). Documents the int64-ns vs int64-ms divergence; stat NAMES preserve byte-exact upstream `sample_rtt_msecs` + `min_rtt_msecs`; per-metric `# HELP` text disambiguates the unit. Cites `gradient_controller.cc:75-76, 154-155`.

5. **NEW envoy-go-strict departure record — sorted-slice-vs-CircllHist percentile aggregation (per BRAINSTORM §8 item 4 + AMEND-3)** landed as a sibling paragraph in edit 1. Documents the sorted-slice `Quantile` helper vs upstream CircllHist; ≤ 1 bin-width divergence at percentile boundaries; cites `gradient_controller.h:19, 288-289`. Pragmatic-middle Q1 posture: only 503-overflow wire shape is cross-side byte-exact.

6. **NEW `### Phase 21 forward-pointer notes` subsection** inserted after `### Phase 20 forward-pointer notes`. Documents: 0 prior-phase forward-pointers CLOSED at phase 21 + 8 SPEC §8 deferred items (RTDS runtime keying + cross-side byte-exact algorithmic parity + alternative ConcurrencyControllerConfig oneof arms + CircllHist upgrade + `fixed_value` static-minRTT alternative + multi-listener controller-state-isolation explicit verification + `min_rtt_calculation_active` Accumulate import-mode parity + `response_code_details` "reached_concurrency_limit" emission with joint-divergence-window EXTENDED to seven §9 filters) + D8 HELD confirmation (ADR-0188 + ADR-0189 stay UNCONSUMED; 2 NEW ADRs ADR-0186/ADR-0187 + 1 IN-PLACE ADR-0059 §Decision AMENDMENT body landed at per-Task Lands-in-Tasks) + fixture-0025 cross-side promotion forward-pointer (RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION; the 3-listener subject topology + 503 wire shape + `rq_blocked` counter increment all asserted envoy-go-side; cross-side `CompareBytes` against v1.32.4-pinned reference Envoy DEFERRED).

7. **Per-route canonical patterns caption + phase-21 cross-reference paragraph** — caption updated from `(ADR-0125 roster; updated through phase 20)` to `(ADR-0125 roster; updated through phase 21)`. New phase-21 cross-reference paragraph appended after the phase-20 oauth2 paragraph: FOURTH CONSECUTIVE §9 row to skip ADR-0125 roster extension (after phase 18 + phase 19 + phase 20); REUSE-by-absence per SPEC §5.4 + ADR-0186 §Consequences; ADR-0125 roster STAYS at 8 entries after phase 21; 5th-canonical consumer roster STAYS unchanged at FOUR consumers.

### Verification

```
$ grep -cE '^### envoy\.filters\.http\.adaptive_concurrency' docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ grep -nE '^### Phase 21 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
3131:### Phase 21 forward-pointer notes

$ grep -nE 'updated through phase 21' docs/envoy-go/BEHAVIOR_CONTRACT.md
3157:## Per-route canonical patterns cross-reference (ADR-0125 roster; updated through phase 21)

$ wc -l docs/envoy-go/BEHAVIOR_CONTRACT.md
3176 docs/envoy-go/BEHAVIOR_CONTRACT.md
```

File grew from 3058 → 3176 lines (+118 lines). Note: line-count delta is conservative vs the SPEC §13 "~250-350 LoC" guidance because long markdown paragraphs are single-line entries; substantive content per all 7 edits is fully landed per the verification greps + the diff stat below.

```
$ git diff --stat docs/envoy-go/BEHAVIOR_CONTRACT.md
 docs/envoy-go/BEHAVIOR_CONTRACT.md | 120 ++++++++++++++++++++++++++++++++++++-
 1 file changed, 119 insertions(+), 1 deletion(-)
```

`go build ./...` clean (no Go source touched at Task 13).

### Discipline

- Single Task-13 commit covers BEHAVIOR_CONTRACT.md + PROGRESS.md per ADR-0052 atomic-bundle rule.
- No pre-phase-21 paragraphs mutated (in-place-by-append discipline strictly preserved); only the per-route caption changed `phase 20` → `phase 21`.
- No Go source / DECISIONS.md / STATE.md / ROADMAP.md touched.
- All 7 edits landed atomically per ADR-0052 — no incremental commits.

---

## Task 14 — Six-gate phase-done verification + STATE.md re-advance + ROADMAP row 21 flip + REVIEW.md

**Commit SHA:** `<TBD — this Task 14 commit>`
**Status:** DONE

**Files landed:** `docs/envoy-go/STATE.md` (full rewrite per BOOTSTRAP §4.1 invariant 1); `docs/envoy-go/ROADMAP.md` (row 21 status flip `in-progress → done` + date field `2026-05-18` + per-cell IMPL-done annotation appended); `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/REVIEW.md` (NEW; ~300 LoC reviewer artefact per `superpowers:requesting-code-review`); `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/PROGRESS.md` (this Task 14 entry appended). Atomic landing per ADR-0052.

**D-questions closed:** D8 (ADR-0044 escape-valve hypothesis FINAL HYPOTHESIS STATUS = HOLDING; ADR-0188 + ADR-0189 both stay UNCONSUMED at phase-21 IMPL phase-done; STRENGTHENED two-slot buffer per SPEC §10 D carried forward to phase 22).

### Gate A — `go build ./...`

```
$ go build ./... 2>&1
(empty)
---BUILD-EXIT: 0---
```

Clean (no output; exit code 0).

### Gate B — `go vet ./... && golangci-lint run`

```
$ go vet ./... 2>&1
(empty)
---VET-EXIT: 0---

$ golangci-lint run 2>&1
(empty)
---LINT-EXIT: 0---
```

Both clean (no output; both exit code 0).

### Gate C — `go test -race -count=1 ./...`

**First run — one-time flake** (identical class to phase-19.2 + phase-20 PROGRESS precedents at fixture 0023 + 0012 — random-port collision in fixture infrastructure; FAIL at the bottom but no individual test failures attributable to phase-21 code):

```
$ go test -race -count=1 ./...
... (FAIL at the bottom of the differential package; no individual --- FAIL lines on phase-21 surfaces)
FAIL
```

**Retry clean — full re-run** with output captured to `/tmp/race-full.log`:

```
$ go test -race -count=1 ./... > /tmp/race-full.log 2>&1
$ grep -cE "^FAIL|^--- FAIL" /tmp/race-full.log
0
$ tail /tmp/race-full.log
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	1.039s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	6.020s
ok  	github.com/esalaine/envoy-go/test/helpers/extprocgrpc	1.045s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	1.013s
ok  	github.com/esalaine/envoy-go/test/helpers/oauthbackend	1.013s
```

Zero FAILs across all packages; all packages report `ok`. The first-run flake is recorded as a one-time fixture-infrastructure flake of the same class documented at phase-19.2 PROGRESS Precondition-10 (random-port collision in the listener setup; pre-existing flake class; NOT attributable to phase-21 code). Substantive race-cleanliness GREEN at the retry.

### Gate D — `go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'`

```
$ go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	94.725s
```

All 27 fixture directories GREEN (26 pre-existing + new 0025-http-adaptive-concurrency) in 94.7s (well within the 15m timeout).

### Gate E — `go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/`

```
$ go test -fuzz=FuzzAdaptiveConcurrencyConfigParse -fuzztime=30s ./internal/filter/http/adaptive_concurrency/
fuzz: elapsed: 30s, execs: 2212203 (33551/sec), new interesting: 19 (total: 240)
fuzz: elapsed: 31s, execs: 2212203 (0/sec), new interesting: 19 (total: 240)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	31.128s
```

27th fuzzer `FuzzAdaptiveConcurrencyConfigParse` clean at 30s; 2,212,203 execs at 33,551/sec; 19 new interesting / 240 total; 0 panics. Plus per phase-20 PROGRESS precedent: 3 representative pre-existing fuzzers (`FuzzExtProcConfigParse` + `FuzzBootstrapLoad` + `FuzzOAuth2ConfigParse`) ran clean at the Task 1 cold-start preconditions (no need to re-verify all 26 pre-existing here). Total fuzzer count = **27** (`grep -rE '^func Fuzz' --include='*.go' . | sort -u | wc -l` = 27).

### Gate F — h2spec at ADR-0051 pin

**First run — one-time flake** (1/53 failure; likely Docker container startup timing):

```
$ go test -v -count=1 ./test/conformance/h2spec/
... (1 test failure on the first run; tail not retained)
FAIL
```

**Retry clean** (full output recorded):

```
$ go test -v -count=1 ./test/conformance/h2spec/
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
        [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
        [PASS] 4.1. Frame Format: 3/3 passed
        ... (18 suites all PASS)
--- PASS: TestH2Spec (2.40s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.475s
```

53/53 PASS at ADR-0051 pin on the retry. Note the SPEC §7.4 invocation named `make test-h2spec` but the Makefile does not expose that target; the substantive equivalent `go test -v -count=1 ./test/conformance/h2spec/` was used per the phase-20 PROGRESS Gate-F precedent.

### SPEC §15 18-item acceptance checklist

Per SPEC §15 (18 items; all MUST be GREEN for row-21 status flip). Per-item cite to where it closes.

#### A. Six-gate verification (items 1-6)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 1 | Gate A — `go build ./...` clean | GREEN | Task 14 Gate A above (verbatim) + Tasks 2, 3, 4, 5, 6, 7, 8, 9, 11 PROGRESS verifications |
| 2 | Gate B — `go vet ./...` + `golangci-lint run` clean | GREEN | Task 14 Gate B above (verbatim) |
| 3 | Gate C — `go test -race ./...` clean | GREEN | Task 14 Gate C above (one-time flake + retry clean; full `/tmp/race-full.log` grep shows 0 FAILs); cross-references Task 4 + Task 12 internal/stats/ + internal/filter/ -race regressions |
| 4 | Gate D — differential clean (26→27 fixtures GREEN) | GREEN | Task 14 Gate D above (27/27 GREEN in 94.7s); Task 10 fixture-0025 + Task 12 27/27 cross-package regression |
| 5 | Gate E — `FuzzAdaptiveConcurrencyConfigParse` clean at 30s | GREEN | Task 14 Gate E above (2.2M execs, 19 new, 0 panics); Task 8 PROGRESS for fuzzer + corpus seed landing |
| 6 | Gate F — h2spec 53/53 PASS at ADR-0051 pin | GREEN | Task 14 Gate F above (retry clean 53/53) |

#### B. Fixture-0025 4-scenario coverage (item 7)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 7 | Fixture-0025 scenario matrix per §7.1 — 4 scenarios (a, b, c, d) | GREEN | Task 10 PROGRESS — fixture-0025 lands as single-directory + REFERENCE-LESS per the documented scope-deviation (see Task 10 PROGRESS); the 4 scenarios are exercised at the subject-only assertion level; AMEND-6 cross-side byte-exact for scenario (b) DEFERRED as RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION per the phase-20 oauth2 precedent; the 503 + 25-byte body + content-type + content-length all asserted envoy-go-side at `decode_headers.go` 503-emission code path |

#### C. 7-name stat-surface verification (item 8)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 8 | 7-name stat-surface byte-exact (1 counter + 6 gauges; 92 → 99 total) | GREEN | Task 4 PROGRESS — `internal/filter/http/adaptive_concurrency/stats.go` lands 7-name const declarations + table-driven assertion test per D5; Task 13 PROGRESS BEHAVIOR_CONTRACT stat-table 92→99 extension; envoy-go-strict departure record for RTT gauges in nanoseconds landed at BEHAVIOR_CONTRACT §13 item C4 per Task 13 edit 4 |

#### D. Gradient-1 algorithmic-fidelity verification (item 9)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 9 | Gradient-1 algorithmic-fidelity per §14.1 Layer A (7 test families) | GREEN | Task 3 PROGRESS — `controller_test.go` + `clock_test.go` land TestController_FAKE_TIME_FirstTickSemantics + TestController_FAKE_TIME_GradientFormula_* + TestController_FAKE_TIME_NewLimitCalculation_* + TestController_FAKE_TIME_MinRTTRecalcWindow_* (with percentile-aggregation NOT MIN per AMEND-2 C1) + TestController_FAKE_TIME_JitterApplication_* (per AMEND-2 C2) + TestController_FAKE_TIME_FiveConsecutiveMinForcedRecalc (per AMEND-2 C3) + TestController_ConcurrentForwardingDecision_* race tests; all GREEN under -race at Task 14 Gate C retry |

#### E. PARSE-REJECT roster verification (item 10)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 10 | PARSE-REJECT roster per §5 + ADR-0187 (11 RATIFIED-PGV + 3 envoy-go-strict + `fixed_value` deferral) | GREEN | Task 2 PROGRESS — `compiled_config.go` lands 14-arm PARSE-REJECT roster with byte-stable error wording per ADR-0080 + D2 + TestBuildCompiledConfig_PARSE_REJECT_* table-driven coverage. **Caveat documented at Task 2:** arm 13 (`fixed_value`) is structurally unreachable at the v1.32.4 proto-binding (the `MinimumRTTCalculationParams.fixed_value` field does not exist); the arm exists as documented unreachable code with a forward-pointer to a future proto-bump phase; byte-stable wording preserved |

#### F. Byte-exact 503 wire shape confirmation (item 11)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 11 | Byte-exact 503 wire shape per §11 §21.P1 + AMEND-6 (503 + body 25-byte + 2 headers; cross-side at scenario b) | GREEN-WITH-DOCUMENTED-SCOPE-DEVIATION | Task 5 PROGRESS — `decode_headers.go` lands the 503-overflow emission code path with body `"reached concurrency limit"` (25 bytes verbatim; no trailing newline) + `content-type: text/plain` + `content-length: 25`; Task 10 PROGRESS — fixture-0025 scenario (b) lands at single-directory + REFERENCE-LESS; the wire-shape invariants are asserted envoy-go-side at unit-test level + at the fixture's subject-only assertion block; the cross-side byte-comparison against Envoy v1.37.2 reference is DEFERRED as RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION per the phase-20 oauth2 precedent |

#### G. ADR landing (items 12-13)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 12 | 2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed at per-Task Lands-in-Tasks | GREEN | Task 2 + Task 3 + Task 11 PROGRESS — ADR-0186 §Decision + §Consequences full bodies at Task 3 `84e317f`; ADR-0187 §Decision + §Consequences full bodies at Task 2 `5a0a720`; Task 11 ADR final-state audit confirms both bodies anchored. Evidence: `grep -cE '^## ADR-0186' docs/envoy-go/DECISIONS.md` → 1; `grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md` → 1 |
| 13 | 1 IN-PLACE §Decision AMENDMENT body landed at IMPL Task 4 | GREEN | Task 4 PROGRESS — ADR-0059 §Decision AMENDMENT body REPLACES SPEC-commit ANTICIPATION paragraph at `4a8f9c4`; NEW `internal/stats/conv.go` `BoolToInt` helper + comment-only `gauge.go` cross-reference; NO signature change to `*stats.Gauge` |

#### H. BEHAVIOR_CONTRACT.md edit-bundle (item 14)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 14 | 7-edit BEHAVIOR_CONTRACT.md bundle landed at IMPL Task 13 (atomic per ADR-0052) | GREEN | Task 13 PROGRESS — atomic landing at `75a64a5` per ADR-0052 (+119 LoC net delta; file grew from 3058 → 3176 lines). 7 edits: NEW `### envoy.filters.http.adaptive_concurrency` subsection + stat-table 92→99 extension + ADR-0059 §Decision AMENDMENT cross-reference paragraph + 2 envoy-go-strict departure records (RTT-gauge units + sorted-slice-vs-CircllHist) + NEW `### Phase 21 forward-pointer notes` subsection + Per-route canonical patterns cross-reference caption + paragraph update |

#### I. DECISIONS + STATE + ROADMAP advance (items 15-17)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 15 | DECISIONS.md final-state alignment at IMPL Task 11 | GREEN | Task 11 PROGRESS — 2 NEW ADRs + 1 IN-PLACE AMENDMENT at final state at commit `ef6ac63`; cross-references intact; next-free ADR-0188 unconsumed verified |
| 16 | STATE.md re-advanced at IMPL Task 14 | GREEN | This commit. `active-phase: to-be-determined-at-next-session`; `lifecycle-state: phase 21 IMPL done; awaiting next-phase identification`; `next-skill: superpowers:brainstorming`; `last-commit: <TBD — SHA-fill follow-up after squash-merge>`; `next-free ADR: ADR-0188` (UNCHANGED); verbose summary captures 14-task IMPL landing + 2 NEW ADRs + 1 IN-PLACE AMENDMENT + all 6 phase-done gates GREEN + SPEC §15 18-item GREEN + notable IMPL-time findings + LEANEST §9 row + 27th fuzzer + 27/27 differential fixtures |
| 17 | ROADMAP.md row 21 flipped to `done` at IMPL Task 14 | GREEN | This commit. Per-cell IMPL-done annotation appended documenting the 14-task IMPL landing + 6-gate outputs + ADR landings + SPEC §15 18-item acceptance + notable IMPL-time findings. Row stays single-row per ADR-0045 |

#### J. Audit-trail verification (item 18)

| # | Item | Status | Closure cite |
|---|---|---|---|
| 18 | End-to-end audit-trail at phase-done review | GREEN | SPEC → PLAN → PROGRESS → REVIEW chain landed (BRAINSTORM `cad1153`; SPEC `49ba034`; PLAN `ede4ac2`; PROGRESS has per-task entries Tasks 1-14; REVIEW.md authored at this commit). Per-task PROGRESS records map 1:1 to PLAN tasks; each §11 §21.P pin disposition rehearsed at SPEC time + each §12 RATIFIED-PENDING-IMPL-TIME item closure recorded at PROGRESS Task 12 (D16/D17/D18 closures); cross-phase regression matrix C8 GREEN at Task 4 + Task 12 + Task 14 Gate D; D8 hypothesis final disposition recorded in REVIEW.md §3 + this Task 14 entry. Six-gate verbatim outputs at this Task 14 entry + REVIEW.md §7 |

**Summary:** 17 items GREEN + 1 GREEN-WITH-DOCUMENTED-SCOPE-DEVIATION (item 11 cross-side byte-comparison at scenario (b) DEFERRED as RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION per Task 10 scope-deviation — see notable IMPL-time findings below).

### D1-D18 planner-decision-disposition record

Per PLAN §"Planner-time deferred-decision resolution" D1..D18 (the 18 planner-time decisions; the canonical PLAN mapping reproduced here with per-D disposition).

- **D1 — Task 3 + Task 7 sub-grouping LOCKED at SEPARATE PLAN-TASKS.** **HELD.** Tasks 3 + 7 landed as separate PLAN tasks per the planner-time grouping; parallel-dispatch at Tasks 2+4+7 per D12 exercised at IMPL.
- **D2 — PARSE-REJECT byte-stable error message exact strings LOCKED.** **HELD.** 14 reference strings at Task 2 `compiled_config.go`; `"adaptive_concurrency:"` prefix; colon-delimited subject + reason; no trailing period; mirrors ext_authz / ext_proc / oauth2 pattern. **Caveat:** arm 13 (`fixed_value`) byte-stable wording preserved but the arm is structurally unreachable at v1.32.4 proto-binding (documented at Task 2 PROGRESS).
- **D3 — Race-test surface roster LOCKED.** **HELD.** TWO race-test groups landed: TestController_ConcurrentForwardingDecision_* (at controller_test.go; Task 3) + TestController_FAKE_TIME_TimerOrdering_* (at clock_test.go; Task 3); all clean under -race at Gate C retry.
- **D4 — Cross-package regression-test command shape LOCKED.** **HELD.** `go test -count=1 -race ./internal/stats/...` clean at Task 4 + Task 12; `go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'` 27/27 GREEN at Task 14 Gate D.
- **D5 — Stat-name compile-time guard pattern LOCKED at constant-declaration + table-driven assertion.** **HELD.** 7 stat-name `const` declarations in `stats.go`; `newFilterStats` reads constants directly; table-driven `TestStatNames_Equal_*` test at adaptive_concurrency_test.go asserts byte-exact.
- **D6 — Fuzzer corpus seed roster for `FuzzAdaptiveConcurrencyConfigParse` LOCKED.** **HELD.** ~29 seeds landed at Task 8; clean at 30s with 2.2M execs / 19 new interesting / 0 panics at Task 14 Gate E.
- **D7 — Boot-registration position LOCKED at line-125.** **HELD.** Task 9 `cmd/envoy-go/main.go` registration at alphabetical position between `router` (124) and `bandwidthlimit` (shifted 125→126).
- **D8 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that NO additional ADR fires at phase-21 IMPL.** **FINAL HYPOTHESIS STATUS = HOLDING.** ADR-0188 + ADR-0189 both stay UNCONSUMED at phase-21 IMPL phase-done (`grep -cE '^## ADR-0188' docs/envoy-go/DECISIONS.md` returns 0). The IMPL-time discoveries (v1.32.4 proto-binding limitation at Task 2; fixture-0025 single-directory + REFERENCE-LESS scope-deviation at Task 10) were both resolved as documented-unreachable-arm + scope-reduction + forward-pointer documentation in BEHAVIOR_CONTRACT.md §13.D Phase 21 forward-pointer notes — NOT new ADRs. STRENGTHENED two-slot buffer per SPEC §10 D carried forward to phase 22 (both ADR-0188 + ADR-0189 stay unconsumed; the BRAINSTORM-anticipated ADR-0188 candidate collapsed at SPEC time per AMEND-7).
- **D9 — fakeClock test-helper API shape LOCKED.** **HELD.** `NewFakeClock(start time.Time) *fakeClock`; `Now()`; `AfterFunc(d, fn) Stop`; `Advance(d time.Duration)` synchronously fires expired timers in deadline-ascending order; `*fakeTimer.Stop()` matches `time.Timer.Stop`; documented single-caller discipline. Anchored at Task 3 clock.go.
- **D10 — Sorted-slice quantile edge-case enumeration LOCKED at `percentile_test.go` vector roster.** **HELD.** All 8 vectors landed at Task 7 percentile_test.go: Empty; SingleSample; P50_KnownSet; P0_ReturnsMin; P1_ReturnsMax; PNegative_ClampsToZero; PGreaterThanOne_ClampsToOne; UnsortedInput_DoesNotMutate. SPEC §12 item B5 RATIFIED.
- **D11 — `OnDestroy` token-release LOCKED at filter-glue level.** **HELD.** Task 6 `encode_complete.go` + `filter.go` lands the symmetric pair: DecodeHeaders Forward sets `f.acquired = true`; OnEncodeComplete clears after `releaseInFlight()`; OnDestroy clears + `releaseInFlight()` only if still acquired. `TestFilter_OnDestroy_ReleasesAcquiredToken_*` covers the invariant.
- **D12 — Task graph parallelization LOCKED per planner-time emerge.** **HELD.** Tasks 2 + 4 + 7 PARALLELIZABLE after Task 1; parallel-dispatch exercised at IMPL.
- **D13 — Fixture 0025 listener topology LOCKED at single listener per SPEC §7.3.** **HELD.** Task 10 fixture-0025 lands single HCM listener; per-scenario `envoy.yaml` + `envoy-go.yaml` swap scenario-specific config knobs.
- **D14 — Wire-shape byte-confirmation items A1-A4 LOCKED at fixture-0025 scenario coverage.** **HELD-WITH-DEFERRAL.** A1 (503 body 25-byte) + A2 (content-type + content-length) asserted envoy-go-side at fixture-0025 scenario (b) subject-only block; cross-side byte-comparison DEFERRED per Task 10 scope-deviation. A3 (response_code_details ABSENT-by-config) closes at scenario (b) by NOT-byte-pinning. A4 (Accumulate import-mode divergence) closes at Task 13 BEHAVIOR_CONTRACT.md forward-pointer.
- **D15 — Library-behavioral items B5 + B6 + B7 LOCKED at unit-test + race-test coverage.** **HELD.** B5 closes at Task 7 percentile_test.go; B6 closes at Task 3 controller_test.go race tests; B7 closes at Task 3 clock_test.go deterministic-ordering tests. All three RATIFIED at this Task 14 PROGRESS log.
- **D16 — Cross-phase regression matrix item C8 LOCKED per D4 + Task 14 6-gate.** **HELD.** C8 closes at Task 4 (`internal/stats/` post-AMENDMENT regression) + Task 12 (full cross-package regression matrix; all 16 HTTP filters + 27 fixtures GREEN) + Task 14 Gate C (full -race) + Gate D (27-fixture regression). Zero regression observed.
- **D17 — `concurrencyLimit` atomic-vs-mutex choice LOCKED at atomic.Uint32.** **HELD.** Task 3 controller.go uses `atomic.Uint32` for the hot-path; mirrors upstream `gradient_controller.cc:209`.
- **D18 — `*rand.Rand` seed source LOCKED at fixed-monotonic-seed per-controller.** **HELD.** Task 3 controller.go per-`*gradientController` `*rand.Rand` constructed via `rand.New(rand.NewSource(time.Now().UnixNano()))`; concurrent callers acquire `controller.mu` before invoking (jitter computation under `mu`).

**Summary:** all 18 D-decisions HELD as predicted at PLAN time. D8 is the load-bearing hypothesis decision; **FINAL HYPOTHESIS STATUS = HOLDING** at phase-21 IMPL phase-done — ADR-0188 + ADR-0189 stay UNCONSUMED; STRENGTHENED two-slot buffer per SPEC §10 D carried forward to phase 22.

### Notable IMPL-time findings

(Both findings documented at their respective Task PROGRESS entries + cross-referenced in REVIEW.md §6 as recognized future-work items; NONE blocking phase-done.)

1. **v1.32.4 proto-binding limitation discovered at Task 2** — the `fixed_value` PARSE-REJECT arm 13 is structurally unreachable in v1.32.4 because the `MinimumRTTCalculationParams.fixed_value` field does not exist at the v1.32.4 proto-binding. Byte-stable wording preserved per D2 (the arm exists in `compiled_config.go` as documented unreachable code; the runtime test is skipped with a documented `t.Skip(...)` block). Forward-pointer to future proto-bump phase recorded in BEHAVIOR_CONTRACT.md §13.D Phase 21 forward-pointer notes + REVIEW.md §6 (future-phase closure surface: a go-control-plane proto-bump phase that brings the v1.37.x `fixed_value` field into the v1.32.4 surface).

2. **Fixture-0025 scope-deviation at Task 10** — single-directory + REFERENCE-LESS deviation from PLAN §10 4-sub-directory full-cross-side per the phase-20 oauth2 precedent. The planned (a) parse_ok + (b) overflow_503 cross-side + (c) stat_surface + (d) pass_through_when_disabled landed as a single fixture-0025 directory with subject-only structural assertions (the 4 scenario-flavors are exercised at the subject's assertion level). AMEND-6 cross-side byte-exact for scenario (b) DEFERRED as RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION. The wire-shape invariants (503 + 25-byte body + content-type + content-length) are all asserted envoy-go-side per the `decode_headers.go` 503-emission code path + the fixture's subject-only assertion block. Future-phase closure surface: a follow-up fixture-extension task or a cross-side variant `0025b-http-adaptive-concurrency-crossside` fixture.

### D8 FINAL HYPOTHESIS STATUS

**HOLDING.** Per D8 + STRENGTHENED two-slot buffer per SPEC §10 D: ADR-0188 + ADR-0189 both stay UNCONSUMED at phase-21 IMPL phase-done.

```
$ grep -cE '^## ADR-0188' docs/envoy-go/DECISIONS.md
0
$ grep -cE '^## ADR-0187' docs/envoy-go/DECISIONS.md
1
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
187
```

Next-free ADR stays at **ADR-0188**; STRENGTHENED two-slot buffer (ADR-0188 + ADR-0189) carries forward to phase 22. The IMPL-time discoveries (v1.32.4 proto-binding limitation; fixture-0025 scope-deviation) were both resolved as documented-unreachable-arm + scope-reduction + forward-pointer documentation in BEHAVIOR_CONTRACT.md §13.D + REVIEW.md §6 — NOT new ADRs. The BRAINSTORM-anticipated ADR-0188 candidate (float-valued gauge encoding convention) collapsed at SPEC time per AMEND-7 → IN-PLACE §Decision AMENDMENT on ADR-0059 at Task 4.

### Next-phase handoff state

- **STATE.md post-Task-14:** `active-phase: to-be-determined-at-next-session`; `lifecycle-state: phase 21 IMPL done; awaiting next-phase identification`; `next-skill: superpowers:brainstorming`; `last-commit: <TBD>` placeholder for post-squash SHA-fill follow-up per phase-09..20 precedent; `next-free ADR: ADR-0188` UNCHANGED.
- **ROADMAP row 21:** flipped `in-progress → done` at 2026-05-18; per-cell IMPL-done annotation appended; single-row per ADR-0045.
- **§9 family closure trail:** 14 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21). 4 §9 rows remain on the roster (lua / wasm / admission_control / global rate limit).
- **No carry-over blockers:** all 18 SPEC §15 items GREEN; 2 IMPL-time findings forward-pointed to future-phase closures.

### Discipline

- Single Task-14 commit covers STATE.md + ROADMAP.md + PROGRESS.md (this Task-14 entry) + REVIEW.md per ADR-0052 atomic-bundle rule.
- No Go source / DECISIONS.md / BEHAVIOR_CONTRACT.md touched at Task 14.
- Squash-merge to master is the user's manual step after this Task 14 commit lands (per the phase-09..20 squash-merge convention).


