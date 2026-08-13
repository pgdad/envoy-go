# PLAN 87 — h2-double-slash-path-routing

**Stage:** PLAN (lifecycle-state 2 -> 3). **Date:** 2026-08-13.
**Base master:** `0f0156645027f49260a9ade7285d14cb08ca4732` (from `git rev-parse master`), branch `phase-87-plan`.
**Scope guard:** docs-only. ZERO production `.go`, ZERO test `.go`. `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, `DECISIONS.md` **BYTE-UNTOUCHED** (row 87 stays `in-progress` at `:149`; sentinel `want` stays **119**; the strict `PROPOSED` guard stays **ARMED at 1** — the phase-87 IMPL disarms it, not this stage).
**Method:** ⚠️ **NAMED DEPARTURE, THIRD CONSECUTIVE STAGE (BRAINSTORM-87 / SPEC-87 precedent): no investigation agents — every probe INLINE by the controller.** The subagent-driven style remains the standing preference and returns at the IMPL, where the work is parallel; a PLAN whose probe set is one throwaway unit table, one `go run`, and one locally-booted binary does not repay the coordination. Probes: a 13-row throwaway `buildRequest` table (`zz_probe87_test.go`, written into the worktree, run, **DELETED with sha256 + empty-porcelain proof**), a 17-case `net/url` `go run` in `/tmp`, the tip binary built with `-o` into scratch, an h2c probe config in scratch, and a throwaway drive probe in `test/helpers` driven through **the repo's own `helpers.H2CRoundTrip`** — the exact helper the `0004` driver uses. Ports **47430-47431**, bound transiently, released with `ss` proof. NO docker (the reference record is banked in SPEC §4 and this stage adds no reference claim). NO worktree survives this stage.

---

## 1. RED ANCHORS RE-PROVEN BY EXECUTION AT THIS PLAN TIP — NOT QUOTED

Every stage refutes its predecessor by execution. The SPEC's anchors were re-run at `0f015664`; **no SPEC claim moved**, and the run added three facts the SPEC did not have (§1.5).

### 1.1 The unit layer — 8 of 13 rows RED at the tip, failure lines read

The throwaway table is the SPEC §6 shape verbatim (nine accept rows + four reject rows) run against the **unmodified** `buildRequest`. Actual output, recorded not predicted:

| row | `:path` | tip result | verdict |
|---|---|---|---|
| accept | `/` | `Path="/"` | **GREEN at tip** — regression control |
| accept | `/foo` | `Path="/foo"` | **GREEN at tip** — regression control |
| accept | `/foo?a=b` | `Path="/foo"`, `RawQuery="a=b"` | **GREEN at tip** — regression control |
| accept | `/a//b` | `Path="/a//b"` | **GREEN at tip** — regression control |
| accept | `*` | `Path="*"` | **GREEN at tip** — regression control |
| accept | `//foo` | `URL.Path = "", want "//foo"` | **RED** |
| accept | `//` | `URL.Path = "", want "//"` | **RED** |
| accept | `//foo/bar` | `URL.Path = "/bar", want "//foo/bar"` | **RED — the silent mis-route** |
| accept | `//foo?x=1` | `URL.Path = "", want "//foo"` | **RED** |
| reject | `foo` | `error = nil, want "bad :path" (req.URL.Path="foo")` | **RED** |
| reject | `?a=b` | `error = nil, want "bad :path" (req.URL.Path="")` | **RED** |
| reject | `/foo#frag` | `error = nil, want "fragment in :path" (req.URL.Path="/foo")` | **RED** |
| reject | `/foo?a=b#frag` | `error = nil, want "fragment in :path" (req.URL.Path="/foo")` | **RED** |

⚠️ **The five GREEN-at-tip accept rows are the ROW'S REGRESSION CONTROLS and must be labeled as such in the landed table** — green-on-arrival rows are not RED anchors and must not be counted as proof of the fix (`reference_liveness_break_needs_failing_baseline`). The `RequestURI`-stays-literal assertion also passed on every accepted row at the tip, so **that pin is a control too**, not an anchor.

### 1.2 The 17-case `net/url` primitive differential, re-derived — SPEC §1.1 CONFIRMED, one detail refined

Re-run at this tip with both primitives side by side. Every SPEC §1.1 row reproduces byte-for-byte, including the escape semantics (`/foo%2Fbar` → `Path="/foo/bar"` + `RawPath="/foo%2Fbar"` on BOTH), asterisk-form, absolute-form, and the four repairs. Two refinements to record:

1. **`ParseRequestURI("/foo#frag")` also populates `RawPath="/foo#frag"`** — the SPEC table said only "frag kept IN Path". Immaterial to the decision (the fragment guard fires before the parse) but the table should not be re-quoted as complete.
2. **The stdlib error text is `parse "foo": invalid URI for request`** — a Go-version-dependent string. This decides the frozen assertion form in §3.

### 1.3 The differential layer, driven through the repo's OWN helper — both arms RED

Tip binary built with `-o` into scratch, booted on an h2c probe config carrying the **post-extension** `0004` route table (`path:/health` → `OK\n`, `prefix://edge` → `edge-ok`, `prefix:/` → `not found\n`), driven by `helpers.H2CRoundTrip` — the same x/net path `H2RoundTrip` uses in the driver. Actual output:

| arm | tip result | verdict |
|---|---|---|
| `GET /health` | `status=200 body="OK\n"` | control GREEN |
| `GET /missing/0` | `status=404 body="not found\n"` | control GREEN |
| `GET //edge` | `status=404 body=""` | **RED** |
| `GET //edge/health` | `status=200 body="OK\n"` | **RED — the silent mis-route, caught ONLY by body** |

### 1.4 The GREEN counter-proof, and the ONE claim no prior stage measured

The SPEC's `stream.go` edit was re-applied as a throwaway patch (`strings.IndexByte(path,'#')` guard + `url.ParseRequestURI`), rebuilt, and both layers re-run:

- unit table: **PASS** (all 13 rows).
- drive probe: `//edge` → **`200 edge-ok`**; `//edge/health` → **`200 edge-ok`**; both controls unchanged.
- `go test ./internal/filter/hcm/... -count=1`: **ok** (`hcm` 0.015s, `hcm/h2` 1.204s).

⚠️ **`prefix: "//edge"` MATCHING `//edge/health` IS A ROUTE-MATCHER CLAIM THAT NO PRIOR STAGE MEASURED** — the SPEC asserted the recipe from the parse behavior alone. It is now measured true end-to-end against envoy-go's own router. Had the matcher normalized or rejected the `//`-prefixed *config* value, D-87-DIFF would have needed reshaping at the IMPL; it does not.

The patch was reverted with `git checkout -- internal/filter/hcm/h2/stream.go`, verified by **`sha256sum -c` against the pre-patch capture (OK)**, both probe files deleted, and `git status --porcelain` + `git diff --stat master` both **EMPTY**.

### 1.5 Findings this stage adds

- **(a) THE `//edge` PRE-FIX 404 IS A ROUTE-MISS, NOT THE CATCH-ALL.** `//edge` returns **404 with an EMPTY body**, while `/missing/0` returns 404 with `not found\n`. `url.Parse("//edge")` yields `Path=""`, and `""` does not carry the `prefix: "/"` catch-all's prefix — so **no route matches at all**. The SPEC's "`//edge` → 404 (drive errors)" is directionally right but the IMPL's arm must assert **status**, not the catch-all body; a body-only assertion on this arm would compare `""` against `""` on a future regression and read green. Assert `status == 200 && body == "edge-ok"`.
- **(b) THE CONTRACT ALREADY CARRIES A SENTENCE THIS ROW REFUTES** — `BEHAVIOR_CONTRACT.md:2034`: *"the path is the bytes of the `:path` pseudo-header — there's no stdlib net/http parsing to inject normalisations"*. `url.Parse` IS stdlib parsing and it injected a corruption. The IMPL's contract edit is therefore a **RIDER on an existing false sentence** (the ADR-0307/`:2058` form), not merely a new bullet. Drafted in §5.1.
- **(c) OPERATIONAL:** the binary's flags are **`-c`** and **`-allow-h2c`** — NOT `--config-path`; and a bootstrap with zero clusters is rejected at boot (`cluster manager: cluster: zero clusters in bootstrap`), so an h2c probe config needs one unused STATIC cluster. Both cost a failed launch at this stage. ⚠️ Both failures were caught only because the launch OUTPUT was read — a `curl /ready` that returns empty looks identical to a slow boot (`reference_timeout_exit_124_shared_by_healthy_and_hung`).

---

## 2. THE ORDERED IMPL TASKS UNDER D-87-SEQ — ONE LEG, ONE ATOMIC COMMIT, TDD HELD INSIDE

D-87-SEQ is unchanged: the swap, the guard, both test layers, the docs and the row flip land as **ONE squashed commit**. The task order below is the TDD order *inside* that commit; each task's RED must be observed and its failure line recorded before the next runs.

**T0 — RED census, unit layer.** Land the path-forms table in `internal/filter/hcm/h2/stream_test.go` beside `TestBuildRequest_ConnectionSpecificFields` (the Table-B idiom anchor at `:548`). Nine accept rows + four reject rows per SPEC §6, **with the five green-on-arrival rows labeled REGRESSION CONTROL in a comment** (§1.1). Run it; record the eight RED lines. **Do not proceed until the census matches §1.1's table** — a different RED set at the IMPL tip is a finding, not a rounding error.

**T1 — RED census, differential layer.** Land the `0004` extension per the §4 recipe (both YAMLs + the appended drive loop + prose). Run `TestDifferential/0004-h2-routing` against the unmodified tree. Expect the drive to fail on the `//edge` **status** arm (§1.5(a)). Record the failure line. ⚠️ **`-run` selector footgun**: confirm the named subtest actually printed; `[no tests to run]` EXITS 0 (`reference_differential_run_selector`).

**T2 — the production edit.** `internal/filter/hcm/h2/stream.go`, `buildRequest` (**cite by symbol** — the line drifted `:440` → `:465` across the lineage and sits at `:465` at THIS tip): the `strings.IndexByte(path, '#') >= 0` guard immediately before the parse, and `url.Parse` → `url.ParseRequestURI`. `strings` is already imported (`:11`); `net/url` is already imported. Comments name RFC 9113 §8.3.1 and the network-path-reference hazard. **Measured shape: 11 insertions / 1 deletion.**

**T3 — GREEN both layers, then the regression sweep.** Unit table green (13/13); `TestDifferential/0004-h2-routing` green with the named subtest printed; `go test ./internal/filter/hcm/... -count=1` green (measured green against the throwaway patch at this stage — a floor, not a guarantee, since T0/T1 add test files the probe did not have).

**T4 — docs.** ADR-0309 §Decision + §Consequences appended in place after the retained italic footer, **guard 1 → 0** (§5.2); the `BEHAVIOR_CONTRACT.md ## HTTP/2` rider + bullet (§5.1) riding ADR-0309 per ADR-0052; `PROGRESS.md`; ROADMAP row 87 → `done` (`numstat 1 1`, `want` STAYS 119, check (1) goes **SILENT** — the fourth silent reading in project history).

**T5 — gates LAST** (§8). Gates are this commit's evidence and run against the final tree, not an intermediate one.

⚠️ **T0 and T1 are BOTH prerequisites of T2.** The temptation to land T2 first (it is ten lines and already measured) would convert both RED censuses into unfalsifiable green-on-arrival claims.

---

## 3. THE ERROR-STRING CONSTRAINT SET — FROZEN

**ZERO landed reject strings change.** The row adds exactly ONE new string.

| form | `Error.Msg` | `Underlying` | assertion the table makes |
|---|---|---|---|
| `foo` (rootless) | `bad :path` — **REUSED**, byte-identical to the landed string at `stream.go:467` | non-nil (`parse "foo": invalid URI for request`) | `Msg` **exact equality** + `Underlying != nil` |
| `?a=b` (bare query) | `bad :path` — REUSED | non-nil | `Msg` exact equality + `Underlying != nil` |
| `/foo#frag` | **`fragment in :path` — THE ROW'S ONLY NEW STRING** | nil | `Msg` exact equality + `Underlying == nil` |
| `/foo?a=b#frag` | `fragment in :path` | nil | `Msg` exact equality + `Underlying == nil` |

**DECIDED AT THIS PLAN (the SPEC left the form open):** the table asserts `herr.Msg` by **exact equality** — the package's Table-B idiom, which `stream_test.go:596-604` explicitly justifies — and asserts `Underlying` only for **nil-ness**. It does **NOT** assert the wrapped stdlib text. Rationale, measured at §1.2: `invalid URI for request` is `net/url`'s wording, not this project's; pinning it would make the suite a hostage of the Go toolchain version for zero discriminating power, since `Msg` + `Code` + nil-ness already separate all four reject rows from each other and from every landed reject. The `Underlying` nil-ness assertion is what keeps the two reject *families* distinguishable — without it, a future edit routing the fragment case through the parse would still read green.

Also frozen: `Code` is `ErrProtocolError` on all four rows (`buildRequest`'s existing idiom for every pseudo-header violation, D-87-REJECT-SHAPE); the landed `empty :path` / `missing :path` / `duplicate :path` strings and their gates are untouched.

---

## 4. THE `0004-h2-routing` EXTENSION RECIPE — PINNED

**Surface:** `test/fixtures/0004-h2-routing/{envoy.yaml, envoy-go.yaml, expectations.yaml, README.md, driver/driver.go}`. `doc.go` needs **no** edit (it documents PKI generation only — verified by reading it at this tip). `test/helpers/h2.go` needs **no** edit (§1.3 drove the arms through the unmodified helper).

**(a) Both route tables.** Insert ONE route **immediately before the `prefix: "/"` catch-all** — at `envoy.yaml:62` and `envoy-go.yaml:62` at this tip (re-derive; cite by the catch-all's `match: { prefix: "/" }` line, not by number):

```yaml
                        - match: { prefix: "//edge" }
                          direct_response:
                            status: 200
                            body: { inline_string: "edge-ok" }
```

Placement before the catch-all (rather than at the top) keeps the existing three routes in their existing relative order, so the documented first-match-wins table in `expectations.yaml` gains a row instead of renumbering three. **Measured working at this stage in exactly this position** (§1.3/§1.4).

**(b) The rendered-bootstrap path is UNAFFECTED — verified by reading, not assumed.** `renderBootstrap` substitutes three `{{...}}` PEM placeholders and then replaces `port_value: 0` **in order, one at a time**. A `direct_response` route introduces **no `port_value` line and no `{{...}}` token**, so neither the ordered port substitution nor `substitutePEM`'s first-occurrence rule shifts. `readYAML`'s leading-comment strip is likewise unaffected (the insertion is far below the header block). ⚠️ If the IMPL ever adds a route that names a *cluster*, this paragraph stops holding.

**(c) `drive()` — APPEND a fourth loop after the existing three.** The existing 27-request schedule and its concatenated transcript prefix stay **byte-identical**; the new arms append to the same `out` builder or assert in-band. Per §1.5(a):

- `GET //edge` → assert **`status == 200`** and `body == "edge-ok"`.
- `GET //edge/health` → assert `status == 200` and **`body == "edge-ok"`** (the mis-route is 200-with-`OK\n` pre-fix, so **the body assertion is the load-bearing one on this arm and the status assertion is the load-bearing one on the previous arm** — neither arm alone catches both failure modes).
- Error strings follow the loop idiom already in `drive()` (`return nil, fmt.Errorf("//edge: status=%d, want 200", status)`).

**(d) Counts untouched, each with its reason:** fixtures **121 +0** (extension, no new dir ⇒ `reference_differential_fixture_three_registration_gates` does not arise) · BackendKind **+0** · ports **+0** · `BackendCount()` stays 3 (`reference_differential_backendcount_min_one` satisfied) · `AssertDistribution` untouched — the new arms are `direct_response`, they touch no backend, so `[3,3,3]` is unchanged by construction.

**(e) Prose riders:** `expectations.yaml` — add route-table row `3. match.prefix: "//edge" -> direct_response 200 "edge-ok"` (renumbering the catch-all to 4) and extend the driver-schedule block to 29 requests/side with the two new arms and their assertions named. `README.md` — same two facts. Both are prose (ADR-0019: not machine-evaluated), so they are documentation obligations, not gates.

**(f) The reference side needs no new evidence.** SPEC §4 measured the reference answering the full path on `//foo/bar` with `contrib-v1.37.2`; the `//edge` arms are the same behavior under a different route name. **No docker at the IMPL for this row's reference claim** — the differential run itself re-exercises it per side.

---

## 5. DOCS — TEXT DRAFTED AT THIS PLAN

### 5.1 `BEHAVIOR_CONTRACT.md ## HTTP/2` (section at `:2019` at this tip — re-derive)

Two edits, both riding ADR-0309 per ADR-0052. **`BEHAVIOR_CONTRACT.md` is BYTE-UNTOUCHED until the IMPL.**

**Edit 1 — a RIDER appended to the existing `:2034` bullet** (the false-sentence form of ADR-0307/`:2058`; the original sentence is RETAINED, never rewritten):

> ⚠️ **RIDER (ADR-0309, phase 87; the mandated vehicle per ADR-0052): the *"no stdlib net/http parsing to inject normalisations"* half of the preceding sentence was FALSE ON THE DOWNSTREAM SIDE from phase 05.1 until phase 87.** The bullet is true of the *forwarding* path it describes, but the downstream codec did parse the `:path` before routing on it: `buildRequest` used `url.Parse`, the full RFC-3986 grammar, in which a leading `//` opens a network-path reference — so an origin-form `:path` of `//foo` had its authority peeled into `u.Host` and routed as an EMPTY path (404), and `//foo/bar` was **silently mis-routed to `/bar`**. H1 and H3 were unaffected (both parse the request-target themselves). From ADR-0309 forward the codec parses with `url.ParseRequestURI`, and the bullet's claim holds as written.

**Edit 2 — a new bullet in `### Asserted equivalence (05.1 + 05.2 scope)`, after the `:2035` route-match bullet:**

> - **NEW (phase 87, ADR-0309): origin-form `:path` parity on H2.** The downstream H2 codec routes on the request-target as written: a leading `//` is path bytes, not an authority (`//foo` → the `//foo` route; `//foo/bar` → the FULL path, not `/bar`), mid-path `//` is unchanged, percent-escape semantics (`Path`/`RawPath`) and the query split are unchanged, asterisk-form `*` still parses to `{Path:"*"}`, and `RequestURI` stays the literal `:path` bytes. Asserted cross-side by fixture `0004-h2-routing`'s `//edge` and `//edge/health` arms and per-form by the `buildRequest` path-forms table. ⚠️ **A `:path` that is not a valid request-target is now REJECTED rather than absorbed** — rootless (`foo`), bare-query (`?a=b`) and fragment-bearing (`/foo#frag`) targets, all of which previously produced a silent 404 or a silently-stripped fragment, are `PROTOCOL_ERROR`. ⚠️ **NAMED WIRE-SHAPE DIVERGENCE (D-87-REJECT-SHAPE, NOT asserted cross-side):** envoy-go rejects at STREAM level (`RST_STREAM PROTOCOL_ERROR`, the codec's existing pseudo-header idiom); the reference answers **400** to a fragment and **tears down the connection** on rootless/bare-query targets. Direction parity, differing wire shape; asserted at UNIT level only, consistent with every other pseudo-header reject in this codec.

### 5.2 `DECISIONS.md` — ADR-0309 completion plan

**Form (ADR-0294→0308 shared block, verified against ADR-0308 at this tip):** append `### Decision (landed at the phase-87 IMPL)` and `### Consequences (landed at the phase-87 IMPL)` **IN PLACE after the RETAINED italic footer** `*§Decision and §Consequences follow at the phase-87 IMPL.*`. **No renumber. No new `---`** (`^---$` STAYS **216**). Headings stay **308**, tail stays ADR-0309, next-free stays **ADR-0310** (TAIL-derived — headings+1 collides at the ADR-0209 gap). STATUS census stays **22**.

**The guard flip:** the block's `> **STATUS: PROPOSED …**` line becomes `> **STATUS: COMPLETE …**`, taking the strict `^> \*\*STATUS: PROPOSED` guard **1 → 0**. ⚠️ **This is the ONE non-append edit `DECISIONS.md` takes this row** — so the IMPL's append-only proof is `numstat N 1` (not `N 0`) and the byte-exact-prefix `cmp` will NOT hold; verify instead that the ONLY deleted line is the STATUS line. ⚠️ The completed block will itself NARRATE the word "PROPOSED", joining the **loose** matcher's standing false-positive set — carry **no whole-file count of the loose form** (the ADR-0305/0306/0307/0308 lesson, four firings).

**§Decision — the D-ledger** (one lettered paragraph each): **(a) D-87-PRIM** the primitive swap and why manual `&url.URL{}` was rejected; **(b) D-87-FRAG** the guard, decided by measured reference behavior (400) not taste; **(c) D-87-REJECT-SHAPE** stream-level reject vs the reference's 400/teardown, direction parity, unit-level only; **(d) D-87-DIFF** the `0004` extension and why no new fixture; **(e) D-87-SEQ** one leg, one atomic commit.

**§Consequences — at least these, i…n:** (i) the two new rejects are **behavior changes toward the reference on previously-silent inputs**, named as such; (ii) the `:2034` contract sentence was FALSE for the whole 05.1→87 window, recorded not fixed (append-only), ADR-0309 the correction; (iii) realized cost vs the §6 floor — **an overrun is a RECORDABLE finding here**, the eleventh consecutive `reference_measured_prototype_is_a_lower_bound` firing if it fires; (iv) the five green-on-arrival unit rows are controls, not anchors; (v) the `//edge` 404 is a route-miss with an EMPTY body — the status assertion is load-bearing (§1.5(a)); (vi) `prefix: "//edge"` matching `//edge/health` was measured at the PLAN, not inferred; (vii) any flake observed at the IMPL's gates, with identity captured.

---

## 6. BUDGET PER TASK — ON TOP OF THE SPEC FLOOR, AND ITSELF A FLOOR

| task | surface | net `.go` | basis |
|---|---|---|---|
| T0 | `stream_test.go` path-forms table | **~100-175** | SPEC §6 band (~100-170) + ~5 for the regression-control labeling §1.1 mandates |
| T1 | `driver/driver.go` fourth loop | **~25-45** | SPEC §8 band, unchanged (the loop shape is the existing three loops') |
| T2 | `stream.go` | **+10 net** measured (`11 ins / 1 del`); band **10-16** | measured twice — SPEC prototype and this PLAN's throwaway patch, identical shape |
| T4 | non-`.go` | 2 YAML routes (~4 lines each) · `expectations.yaml` + `README.md` ~20-30 · ADR-0309 §Decision+§Consequences ~12-20 lines · contract ~2 lines · ROADMAP `1 1` | — |
| **total `.go`** | | **~135-236 net** | SPEC floor was ~130-225 |

⚠️ **QUOTED AS A FLOOR, NOT A CENTRAL ESTIMATE.** Ten consecutive `reference_measured_prototype_is_a_lower_bound` firings, cause UNDER-ENUMERATION every time. Named un-enumerated classes at this stage: per-arm exact-message pins if the IMPL adopts Table-B's full comment idiom; reviewer-mandated arms; **and one this PLAN adds — the regression-control labeling and the `Underlying` nil-ness assertions of §3, neither of which the SPEC's per-file band contemplated.** An overrun past 236 is not a failure; it is a §Consequences (iii) entry.

---

## 7. COUNTS — EVERY ONE RE-DERIVED MECHANICALLY AT THIS TIP (`0f015664`)

- `ROADMAP.md` **237 lines / 119 data rows**, row 87 `in-progress` at `:149` · check-(2) anchors `:197 :203 :209 :219 :225 :233` · `-family row` **95 occurrences / 67 LINES** (⚠️ pass `--` before the pattern; the two forms are both correct and neither is "the" count) · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.
- `DECISIONS.md` **18114** · `^## ADR-` **308** · tail **ADR-0309 PROPOSED** · next-free **ADR-0310** (TAIL-derived) · strict `PROPOSED` guard **1 — ARMED, NOT disarmed at this PLAN** · STATUS census **22** · `^---$` **216**.
- `BEHAVIOR_CONTRACT.md` **5957** (byte-untouched) · `## HTTP/2` at **`:2019`**, the false sentence at **`:2034`**, the route-match bullet at **`:2035`** · `STATE.md` **64** · `STATE_HISTORY.md` **486** · `BOOTSTRAP_PROMPT.md` **522** (repo ROOT, not `docs/envoy-go/`) · phase dirs **128** · `REVIEW.md` **37** (standing departure, named not claimed).
- fixtures **121**, tail `0119-grpc-unary-trailers`, `0120` stays FREE · fuzzers **55 / 48 files** · BackendKind tail **38** (`H2GoawayResponder BackendKind = 38`, `fixture.go:614`).
- **Stat surface: the PLAN edits zero `.go`, so the DELTA is 0 by construction.** ⚠️ The census form matters and this stage's form is NOT the lineage's: `command grep -rno 'NewCounter(' --include='*.go' .` reads **327** and `NewGauge(` reads **79** at this tip — **occurrences over all `.go` including tests**, whereas the 86/87 lineage's carried `145/21` is a different form. **No absolute is corrected and none is carried**; the IMPL asserts the DELTA by running the SAME command on both sides and stating which form it ran (`reference_a_drift_correction_is_itself_a_claim`).
- phase-87 surface: defect site `buildRequest` in `internal/filter/hcm/h2/stream.go`, the `url.Parse(path)` call at **`:465` at this tip** (cite by SYMBOL — it drifted `:440`→`:465` already) · exactly **ONE** non-test `url.Parse` under `internal/filter/hcm/` (re-grepped) · Table-B idiom anchor `stream_test.go:548` · `test/fixtures/0004-h2-routing/` per §4 · H1 (`net/http`) and H3 (`h3dispatch.go`) correct — **DO NOT TOUCH**.
- ⚠️ **STILL CONTESTED, NO NUMBER CARRIED:** the `STATE_HISTORY.md` archive-gap total · the loose `PROPOSED` matcher's whole-file count · the stat-surface absolute · the zero-test `generic/*` suite count.

---

## 8. THE IMPL'S GATES (T5, run LAST, against the final tree)

Full differential **121 fixtures** with `INNER_EXIT` captured and the **ANCHORED** panic gate (`^panic:|DATA RACE|SIGSEGV` — the unanchored form false-fires 14× on a green log) · `go test ./...` · `-race` on `internal/filter/hcm/h2` · `gofmt -l` (gate on OUTPUT — it never exits non-zero) + `golangci-lint` (⚠️ misspell locale **US**) on the touched packages · **h2spec cited ONLY from the IMPL's own run** (`95 tests, 94 passed, 1 skipped, 0 failed`; skip invariantly `6.9.2/2`) · every count above re-run with its NC · the sentinel BOTH SIDES of the row flip.

**Six-gate posture — name departures, do not claim compliance:** grpc-conformance deferred in writing; proxy-wasm 10/16; no `REVIEW.md` (standing departure since phase 25.3).

**Flake watch, with identity capture BEFORE re-running:** the SDS dial-budget class now has a named member — `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server…` (`internal/boot/boot_sds_e2e_test.go:551`), which fires under full-suite load with a gRPC `DeadlineExceeded` recv error. Also live: `internal/cluster` `-race` outlier · `internal/httpclient` zero-value · the driver-receiver port race (`0081` binds `0.0.0.0:42039`). **Capture `-v` output BEFORE any re-run**, then clear scoped.

## 9. SENTINEL

See `PROGRESS.md` §Sentinel(PLAN) for this stage's recorded output — **ONE side** (a PLAN does not touch `ROADMAP.md`; byte-untouched verified by empty diff against master).

## 10. NEXT

**IMPL** — the single leg of D-87-SEQ: T0→T5 above, one atomic commit, row 87 flipped `done` with check (1) going SILENT, ADR-0309 completed with the strict guard 1 → 0, and the realized cost recorded against §6's floor.
