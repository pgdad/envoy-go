# Phase 93 — `h2-local-reply-content-length` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Lifecycle state 2 -> 3.** Row 93 stays `in-progress`; `ROADMAP.md` is **BYTE-UNTOUCHED by this stage** and the sentinel `want` stays **125**. A PLAN adds no ADR: tail stays **ADR-0315**, next-free stays **ADR-0316**, `^---$` stays **216**, and the house `PROPOSED` guard stays **ARMED at 1**.

**Goal:** On the HTTP/2 leg, a locally generated reply emits a `Content-Length` header, always, valued at the real body length — closing an internal inconsistency in which `h2LocalReplyHeaders()` is the sole local-reply composer in the tree that omits the field.

**Architecture:** The fix lands in the **composer** `h2LocalReplyHeaders()`, which gains a `bodyLen int` parameter mirroring its H/1 sibling; it deliberately does **not** land in the wire writer `writeH2Reply`. Seven call sites pass a length — four pass `0`, three pass `len(bad502Body)`. The differential instrument is upgraded from a `content-length` **arity** pin (which goes vacuous once both sides read 1) to a **declared-vs-delivered** pin that records the claimed value and the summed DATA-frame body length per side.

**Tech Stack:** Go; `internal/filter/http/router` (composer + call sites), `internal/filter/hcm` (wire writer + the recompute pin), `test/differential` + `test/fixtures/0004-h2-routing` (the instrument), `docs/envoy-go/DECISIONS.md` (ADR-0315), `docs/envoy-go/BEHAVIOR_CONTRACT.md` (narrative rider).

**Spec:** `docs/envoy-go/phases/93-h2-local-reply-content-length/SPEC.md` (409 lines). The PLAN argues from the SPEC; executors read both.

**Predecessor status:** the SPEC **reversed the BRAINSTORM's central pick** (§4) and refuted eleven inherited claims. Read `BRAINSTORM.md` knowing its §3.2, §3.3 and §6.1 arm choice are refuted and its `ADR-0085` attribution is wrong.

---

## Global Constraints

Every task's requirements implicitly include this section. Values are copied verbatim from `SPEC.md` §12 and re-derived at this PLAN's tip (`ed4716fb`).

**G1 — `-count=1` on EVERY `test/differential` invocation, unconditionally.** The harness builds envoy-go as a **subprocess**, so a router edit is **not a compile-time input** to that test binary and Go's cache serves a stale PASS. A fix whose own gate cannot see it is the most dangerous form of this trap. The invocation is:
`go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v 2>&1`

**G2 — `go test ./...` drives Docker in TWO packages, not one.** Exclude both:
`go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`
(The BRAINSTORM's recipe named only the first.)

**G3 — assert the denominator.** `go test` without `-v` prints ZERO `=== RUN`; `RUN=0` beside `RC=0` is a **vacuous green**.

**G4 — assert the selector matched.** A `-run` selector matching nothing prints `[no tests to run]` and **exits 0**.

**G5 — anchored FAIL grep only.** Unanchored `grep -c 'FAIL'` reads nonzero on a fully green tree. Use `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`.

**G6 — gate on OUTPUT, not exit code.** `gofmt -l` never exits non-zero. `golangci-lint` does exit non-zero here, but still gate on output. Its **misspell runs in locale US** and fired on the SPEC's own prototype (`favour` -> `favor`): sweep British spellings in `.go` comments before the gate. Markdown prose may use them freely.

**G7 — reconcile the fixture set BY NAME, both directions.** The runner `t.Skipf`s an unregistered fixture and **no fixture-count gate exists anywhere in the tree**. Assert `--- PASS: TestDifferential/0004-h2-routing` explicitly. Fixtures at this tip: **121** (`ls -d test/fixtures/*/ | wc -l`; tail `0119-grpc-unary-trailers`, `0120` FREE). ⚠️ the plausible `grep -cE '^[0-9]{4}-'` form reads **119** — it drops `0007a`/`0007b`.

**G8 — the anchored panic gate `^panic:|DATA RACE|SIGSEGV` on every differential launch.** `-race` on the differential suite is **vacuous** — the subject is an unraced subprocess.

**G9 — NC every new assertion, and NC the fixes too.** Neutralise rather than build-break, so the package still compiles. A NC that is a build break proves nothing.

**G10 — `grep -c` on zero matches prints `0` AND exits 1.** Capture with `v=$(… || true)`; never `$(… || echo 0)`, which emits TWO zeros.

**G11 — `rc=$?` after a pipe returns the LAST command's status.** Use `out=$(…); rc=$?` or `PIPESTATUS[0]`. There is no `INNER_EXIT` in this repo.

**G12 — worktree discipline.** The Bash tool's cwd silently resets to `/home/esa/git/envoy-go` (observed again at this PLAN). Use `git -C <abs-worktree-path>` for every git command. Shell variables do not survive between Bash calls — re-export. Use `<<'EOF'`, never `<<EOF`, or backticked text is command-substituted. Build with `-o` into scratch: `go build ./cmd/envoy-go/` drops an untracked binary in the worktree root.

**G13 — cost figures come from `--numstat`, never `--stat`** (which is a SUM, not additions).

**G14 — port bands.** `net.ipv4.ip_local_port_range = 32768 60999`; the differential harness reserves `20000..31007`. Ad-hoc probes use **12000-19000**. Check with `ss -tan` (ALL states), never `ss -ltn`.

**G15 — never tear down a container this session did not create.** Kill only BY NAME, with a distinctive per-stage prefix. The foreign roster is **not closed and changed again at this PLAN** (`curl-world-httpd-1` is new since the SPEC).

---

## 0. What this stage refuted, by execution

Every stage's job is to refute its predecessor by execution. The BRAINSTORM refuted eleven; the SPEC refuted eleven, two of them the controller's own hypotheses and one reversing the BRAINSTORM's central pick. **This PLAN refutes seven.** Five are the SPEC's, one is its own measurement agent's, and one is a cite defect the SPEC introduced while correcting a different one.

1. ⚠️ **THE SPEC'S "18-ROW TABLE" IS NOT REPRODUCIBLE AT THIS TIP.** SPEC §6 calls `test/fixtures/0004-h2-routing/driver`'s table *"its 18-row table"* while arguing it is synthetic. **Measured at `ed4716fb`: the `TestH2Driver_AssertDistribution` table is TEN rows** (`driver_test.go:73-78` six distribution branches + `:82-85` four p92 arity pins). Candidate totals in that file are 10, 7 (`TestParseBackendIdx`), 17 (both tables) and 21 (`=== RUN` lines) — **none is 18.** SPEC §8's *"table 10 -> 18 rows"* is fine: **10 is the correct BASELINE**, and 18 is the POST-fix figure. §6's NC was evidently measured on a tree that ALREADY carried the §5 instrument, then restated as a fact about the current tree — the `reference_cost_figure_measured_at_publishing_commit` species. ⇒ **Per `reference_a_drift_correction_is_itself_a_claim`, quote NO number for "the driver's table" except the measured 10.** ⚠️ **The SPEC's underlying CLAIM survives intact** — the driver's unit table stays `ok` with the fix reverted, so it is not a gate on production behaviour. Only the number is wrong.

2. ⚠️ **TWO OF THE SPEC'S "DRIFT-PROOF" ANCHORS ARE NOT DRIFT-PROOF.** §2 row 4's anchor `val = strconv.Itoa(len(body))` matches **TWO** production lines in `internal/` — `hcm/h2dispatch.go:1015` (H/2, guard `if ln == "content-length"`, lowercase) and `hcm/codec.go:88` (H/1 `writeH1Reply`, guard `if canon == "Content-Length"`, canonical-MIME). §2 row 6's `chain.RunEncodeHeaders` matches **THREE** — `connection.go:738` (H/1), `h2dispatch.go:650` (H/2), `h3dispatch.go:366` (H/3). ⇒ **cite the guard+assign PAIR, or pathspec-scope.** This is `reference_symbol_assertion_needs_qualified_name` firing on an anchor the SPEC certified as drift-proof: **a "drift-proof anchor" is itself a claim, and it must be checked for UNIQUENESS, not just for existence.**

3. ⚠️ **`directResponseAction.body()` IS CITED WITHOUT ITS PACKAGE, AND THE PACKAGE IS THE SURPRISING ONE.** SPEC §10.1 row 6 and §3.2 both give bare `actions.go:91`. Measured: `git ls-files -- '*actions.go'` returns **exactly one** file, **`internal/filter/hcm/actions.go`** — the **hcm** package, NOT `internal/filter/http/router/`. This is the mirror image of the SPEC's own `internal/filter/hcm/chain.go` warning (that file does not exist; it is `internal/filter/http/chain.go`), running in the OPPOSITE direction. ⚠️ **The PLAN must not "fix" it by moving it to the router package.**

4. ⚠️ **THE CLAIM THE FIX FALSIFIES LIVES IN MORE FILES THAN ANYONE HAS COUNTED — AND THE COUNT ROSE TWICE DURING THIS STAGE.** SPEC §14 item 5 names **one** file (`README.md:147`). The PLAN's measurement agent found **two** for `ARITY, NEVER VALUE` (`README.md:153` + `driver.go:1377-1380`). **The controller re-derived it and found THREE** — the agent omitted `BRAINSTORM.md`. ⇒ **`reference_measured_prototype_is_a_lower_bound` fires a SEVENTH consecutive row, and it fired a second time INSIDE this stage, on its own agent's enumeration.** The "SEVEN call sites" claim likewise lives in **two** shipped files (`README.md:147`, `driver.go:1369`), not one. ⚠️ **Enumerate by `git grep -l` on the LITERAL CLAIM TEXT, never by naming the file you happen to be editing.**

5. ⚠️ **THE FOUR BODY-LESS CALL SITES ARE NOT HOMOGENEOUS, AND A PATCH KEYED ON `Body: nil` REACHES ONLY HALF OF THEM.** `retry.go:374` and `router_h2.go:80` write `Body: nil` **explicitly**; `router_h2.go:128` and `:138` **omit the `Body` field entirely**. ⇒ key the edit on `h2LocalReplyHeaders()`, never on `Body: nil`. ⚠️ **And `:128`/`:138` are BYTE-IDENTICAL** (`return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}, picked, nil`, three tabs of indent) — they must be disambiguated by their PRECEDING COMMENT and enclosing guard, never by line number.

6. ⚠️ **SPEC §9.1's ADR-0315 §Context OUTLINE IS STALE AGAINST THE ADR IT DESCRIBES.** §9.1 lists **NINE** paragraphs; the landed draft at `DECISIONS.md:18648` carries **TEN**, and the mapping differs (the landed ¶2 is the citation correction, ¶8 is the unit-coverage paragraph §9.1 omits entirely, ¶9 is the H/3 composition). ⇒ **the IMPL must extend the ADR from the LANDED TEXT, not from §9.1's outline**, or it will renumber paragraphs that are already correct.

7. ⚠️ **THE ARCHIVE EVICTION HAS NO UNIQUE OLDEST AT THIS CLOSE — THE COINCIDENCE THE ROUTER WARNED ABOUT HAS BROKEN.** The router notes the last TWO closes each had a unique oldest that ALSO sat last, "and that coincidence is exactly why position-by-convention must NOT be adopted." **Measured by direct date read at this tip: lines 52 and 54 BOTH read `2026-08-25` — a TIE.** The date read alone does **not** resolve the evictee. It is resolved by **lifecycle precedence** (phase 92 BRAINSTORM precedes phase 92 SPEC), giving `phase 92 (h2-response-header-validation) BRAINSTORM done`. ⚠️ **This happens to be the last line again — which is exactly why the tie-break must be STATED: a session reading position would get the right answer here for the wrong reason, for the third close running.**

8. ⚠️ **SPEC §6.1's `~+60` RECOMPUTE-PIN ESTIMATE IS REFUTED — THE REAL COST IS 147 LINES, ~2.45×.** Measured from a compiling, running, both-directions-NC'd file (`internal/filter/hcm/h2dispatch_recompute_cl_test.go`, `--numstat 147 0`; 92 lines even excluding blanks and comments). The estimate was right about **feasibility** and wrong about **price**. ⚠️ **This is the row's single largest unpriced item and SPEC §14 item 4 called it out precisely — the answer is simply 2.45× worse than guessed.**

9. ⚠️ **SPEC §8's ESTIMATED COMBINED PRODUCTION PATCH `~+12 / -10` IS REFUTED ON THE ADD SIDE, IN EVERY DE-ROT SCOPE.** MEASURED: the fix **alone** is `+10 / -8` across two files — **the SPEC's measured figure is CONFIRMED BYTE-EXACT** (`1 1` retry.go, `9 7` router_h2.go, the `9` including the `strconv` import). But the **cheapest honest de-rot** — a pure word swap, `three-`→`four-` and `(Content-Type, Date, Server)`→`(Content-Type, Content-Length, Date, Server)` — is **`+13 / -10`**, because the corrected clause no longer fits the two original comment lines and the reflow costs `+3/-2`, not `+2/-2`. The variant this PLAN ships, which also documents the new parameter rather than leaving it undocumented, is **`+15 / -10`**.

10. ⚠️ **SPEC §4's `internal/filter/hcm/` BASELINE OF `323 === RUN` IS REFUTED — IT IS `322`.** Measured three times: twice by the prototype agent with the fix applied, and once by the controller on the **clean tip with no fix at all**. Both read **322**, which is itself the discriminating observation: **the fix touches no `hcm` test, so the figure is 322 on both sides of it.** The router figure of **129** is CONFIRMED exactly, likewise on both sides. ⇒ post-fix denominators are **router 133** (+4: parent + 3 arms) and **hcm 327** (+5: parent + 4 rows).

11. ⚠️ **SPEC §5.1 UNDERSTATES WHAT HAPPENS TO THE ARITY PIN, AND OMITS A WHOLE RE-BASELINE FILE.** §5.1 says only that once the fix lands "arity reads **1 vs 1** on every arm and the pin stops discriminating." **Measured: the pin does not merely go quiet — it goes RED**, and two gate files must be re-baselined:
    - `driver.go` **`+1 / -1`** — `p92WantSubjCLFields = 0` becomes `1`.
    - `driver_test.go` **`+8 / -8`** — eight table rows carry `p92CLObs(0)` as `subjCL`. ⚠️ **`driver_test.go` IS NOT NAMED ANYWHERE IN THE SPEC as a re-baseline**; §8 lists it only for the table's growth.
    ⚠️ **AND ONE ROW IS A NEGATIVE CONTROL THAT MUST INVERT, NOT SHIFT.** Row `"p92 subj value: 1 where 0 expected"` (`driver_test.go:85`) passes `p92CLObs(1), p92CLObs(1)` with `wantErr: true`. Once the want is `1`, the observation **matches** and the row raises no error — so a `wantErr: true` row **FAILS**. It must become `"0 where 1 expected"` with `p92CLObs(0)`, **or it silently stops discriminating.** This is `reference_passing_test_is_not_a_guard` and `reference_break_roster_goes_stale_within_its_own_row` firing together.

12. ⚠️ **THE EMPTY-VS-ZERO TRAP IS LIVE ON THIS FIXTURE, AND IT IS THE ROW'S OWN SUBJECT.** Measured pre-fix, all five arms: the subject's declared value is **`<absent>`, NOT `0`** (`clArity=0`). A naive integer `declared == delivered` equality therefore has **no pre-fix subject value to compare at all**. §5.1 names the trap in the abstract; this is the measurement showing it fires on the very fixture the instrument is being built into. ⇒ **`declared` must be a three-state observation (absent / duplicated / a parsed integer), never an `int` with `0` overloaded to mean "missing".**

### What this stage CONFIRMED, by execution — recorded so the IMPL does not re-litigate it

- ⚠️ **`p93WantRefBodyLen = 87` IS CONFIRMED, AND IT WAS A CLAIM UNTIL THIS STAGE.** Measured live over Docker on **all five arms**, uniformly: reference `status=502 clArity=1 declared=87 bodyLen=87`. **`p93WantSubjBodyLen = 12` is likewise CONFIRMED by measurement**, not merely derived from `len(bad502Body)`: subject `bodyLen=12` on all five arms. §14 item 3 is discharged.
- **Bonus the SPEC did not have:** the reference's **declared** value also reads **87**, so §5.1's `declared == delivered` invariant **already holds on the reference side today**. The invariant is therefore not a new reference-side risk — it is a new *subject-side* gate.
- ⚠️ **THE CROSS-SIDE BYTE TRANSCRIPT HAS ZERO CHURN, PROVEN BY sha256, NOT BY READING.** Ref and subject streams dumped pre-fix and post-fix: all four are **420 bytes** with digest `c9739f62…`; `diff` empty both directions. §5.1's "no `Fprintf(&out` line moves" is CONFIRMED. §14 item 2 is discharged — **and the structural reason is stronger than the measurement**: there is **no stored baseline anywhere in `test/`** to re-baseline (`git ls-files -- test/ | grep -icE 'golden|baseline|snapshot'` reads **0**), and `expectations.yaml` is **prose, not machine-evaluated** (ADR-0019). `content-length` appears there once, inside the *header allow-list — values not compared*.
- **§5.2's blind-spot claim REPRODUCES EXACTLY.** `git grep -nE '\b(503|504)\b'` over `driver.go` returns **exactly one line**, `:1369`, **and it is a comment**. The instrument is structurally incapable of seeing the body-less sites. **Do not claim that gap closed.**
- **All six §10.1 anchors are still correct at this tip**, and `:1408` is still `bad = append(bad, …)` inside a loop. The SPEC's corrections hold; nothing drifted between the SPEC and this PLAN.
- **The seven call sites and the 4/3 split are CONFIRMED**, and README `:147`'s parenthetical breakdown (`retry.go` 504; `router_h2.go` 503 ×3 and 502 ×3) is **exactly right**. Only its consequence clause is at issue.

---

## File Structure

Ten files, in four groups. ⚠️ **`reference_measured_prototype_is_a_lower_bound` has now fired SEVEN consecutive rows, always by under-enumerating FILES — and it fired TWICE INSIDE THIS STAGE.** This roster is therefore stated as a **floor with its enumeration method named**, not as a closed set: it was built by `git grep -l` on the LITERAL CLAIM TEXT the fix falsifies, not by listing the files being edited.

### Group 1 — the production fix (2 files, MEASURED `+15 / -10`)
- **Modify `internal/filter/http/router/router_h2.go`** — `h2LocalReplyHeaders()` gains `bodyLen int` and emits `Content-Length` after `Content-Type`; six of the seven call sites updated; `strconv` imported; the doc comment at `:289-293` de-rotted (its "three-header insertion order (Content-Type, Date, Server)" clause is falsified BY THIS FIX).
- **Modify `internal/filter/http/router/retry.go`** — the seventh call site (`:374`, the 504) passes `0`.

### Group 2 — unit coverage the row BRINGS (2 files, both NEW, MEASURED)
- **Create `internal/filter/http/router/router_h2_local_reply_cl_test.go`** (**104 lines**) — `TestH2LocalReplyContentLengthAlwaysEmitted`, three arms, the H/1 sibling as a control that must stay green in BOTH directions.
- **Create `internal/filter/hcm/h2dispatch_recompute_cl_test.go`** (**147 lines**) — `TestWriteH2Reply_ContentLengthRecompute`, four rows, pinning the recompute the row's whole mechanism rests on and which NOTHING in the tree pins today.

### Group 3 — the differential instrument (2 files)
- **Modify `test/fixtures/0004-h2-routing/driver/driver.go`** — `p92DriveArm` records the summed DATA-frame body length beside the declared `content-length`; four new pins in `AssertDistribution` at `:1519-1520` (after the p92 pins at `:1515`/`:1518`, BELOW `CompareBytes`, joined by the existing `errors.Join` at `:1521`); a roster barrier read from the LIVE `p92Arms()`. ⚠️ **PLUS a 26-line comment de-rot at `:1355-1380` that nobody priced** — see Task 5.
- **Modify `test/fixtures/0004-h2-routing/driver/driver_test.go`** — the `TestH2Driver_AssertDistribution` table grows from its **MEASURED 10 rows** (`:73-78` + `:82-85`).

### Group 4 — the record (4 files)
- **Modify `test/fixtures/0004-h2-routing/README.md`** — `:147`'s consequence clause corrected; `:137`, `:139`, `:141`, `:143`, `:145`, `:153` and `:40` re-stated against the post-fix behaviour.
- **Modify `docs/envoy-go/DECISIONS.md`** — ADR-0315 §Decision + §Consequences appended IN PLACE after the RETAINED italic footer at the file tail. ⚠️ **Extend from the LANDED ten-paragraph §Context, not from SPEC §9.1's stale nine-item outline.**
- **Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md`** — a `## HTTP/2 local-reply Content-Length (phase 93)` narrative rider at the file tail, on the `## HTTP/2 response trailer forwarding (phase 84.1)` precedent (`:5913`). **No ledger line, no absolute.**
- **Modify `docs/envoy-go/ROADMAP.md`** — row 93 flips `in-progress` -> `done` **at the IMPL, not at this PLAN**, and its `ADR-0085` attribution is corrected to ADR-0155.

### ⚠️ NOT edited, deliberately
- **`docs/envoy-go/phases/93-h2-local-reply-content-length/BRAINSTORM.md`** carries the falsified `ARITY, NEVER VALUE` claim and the wrong `ADR-0085` attribution. **It is a historical phase document and is NOT retro-edited** — it records what was believed at the time. Its errors are corrected in the SPEC, in this PLAN and in ADR-0315, never in the BRAINSTORM itself.
- **`internal/filter/hcm/h3dispatch.go`** — the misleading Rule B comment (§7). **BANKED, see Task 8's decision.**

### Measured cost roll-up

| # | file | +add / -del | status |
|---|---|---|---|
| 1 | `internal/filter/http/router/router_h2.go` + `retry.go` | **`+15 / -10`** (2 files, incl. de-rot) | **MEASURED** — fix alone `+10 / -8` |
| 2 | `internal/filter/hcm/h2dispatch_recompute_cl_test.go` (NEW) | **`+147 / -0`** | **MEASURED** — SPEC estimated ~+60 |
| 3 | `internal/filter/http/router/router_h2_local_reply_cl_test.go` (NEW) | **`+104 / -0`** | **MEASURED** — SPEC estimated ~+117 |
| 4 | `test/fixtures/0004-h2-routing/driver/driver.go` — re-baseline only | **`+1 / -1`** | **MEASURED** |
| 5 | `test/fixtures/0004-h2-routing/driver/driver_test.go` — re-baseline only | **`+8 / -8`** | **MEASURED** |
| 6 | `driver.go` — the §5 instrument | ~`+300 / -44` | **SPEC-MEASURED**, not re-measured here |
| 7 | `driver_test.go` — table 10 -> 18 rows | ~`+57 / -10` | **SPEC-MEASURED**, not re-measured here |
| 8 | `driver.go` — the `:1355-1380` comment de-rot | **UNPRICED** | ⚠️ see Task 5 |
| 9 | `test/fixtures/0004-h2-routing/README.md` | ~`+18 / -8` | **ESTIMATED** (SPEC) |
| 10 | `docs/envoy-go/DECISIONS.md` — ADR-0315 §Decision + §Consequences | ~`+12 / -0` | **ESTIMATED** |
| 11 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` — narrative rider | ~`+3 / -0` | **ESTIMATED** |

**The minimum green change-set — fix + both re-baselines, no instrument, no docs — is MEASURED at 4 files, `+19 / -17`, and it passes the differential and the full non-Docker sweep.** That is the floor, not the deliverable.

---

## Task Sequence

Nine tasks. Each ends with an independently testable deliverable and its own commit. **Tasks 1-3 are the row's correctness spine and must land in order** — Task 2 pins the behaviour Task 1's correctness rests on, and Task 3 proves Task 1 both directions. Tasks 4-5 are the instrument. Tasks 6-9 are the record.

---

### Task 1: The production fix — `h2LocalReplyHeaders(bodyLen int)`

**Files:**
- Modify: `internal/filter/http/router/router_h2.go` (helper at `:294`, doc comment `:289-293`, six call sites)
- Modify: `internal/filter/http/router/retry.go:374` (the seventh call site, the 504)

**Interfaces:**
- Produces: `func h2LocalReplyHeaders(bodyLen int) envoyhttp.OrderedHeaders` — Tasks 3 and 4 depend on this exact signature. It mirrors the H/1 sibling `localReplyHeaders(bodyLen int)` (`router.go:675`).
- Consumes: `const bad502Body = "bad gateway\n"` (`router.go:31`, 12 bytes).

⚠️ **THIS IS THE SPEC §4 REVERSAL. DO NOT SILENTLY RE-ADOPT THE BRAINSTORM'S PARAMETERLESS PLACEHOLDER.** The BRAINSTORM called the parameter *"strictly dominated … inert"* on **cost alone** and never priced correctness. To disagree you must refute SPEC §4.2's access-log measurement **by execution** — a placeholder ships `content-length: 0` beside `BytesSent=12` on all three 502 sites unconditionally, because `chain.RunEncodeHeaders` (`h2dispatch.go:650`) reads the carrier ~26 lines before `writeH2Reply` (`:677`) corrects it, and `writeH2Reply` builds its own field slice and **never mutates the carrier**.

- [ ] **Step 1: Add the `strconv` import and change the helper signature**

In `internal/filter/http/router/router_h2.go`, add `"strconv"` to the import block, then change the helper. The `Content-Length` field goes **after `Content-Type`**, mirroring the H/1 sibling's slot and preserving the existing Date-before-Server relative order:

```go
func h2LocalReplyHeaders(bodyLen int) envoyhttp.OrderedHeaders {
	return envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Content-Length", Value: strconv.Itoa(bodyLen)},
		{Name: "Date", Value: httpDate()},
		{Name: "Server", Value: serverHeader()},
	}
}
```
⚠️ **Match the surrounding file's existing field-construction idiom exactly** — read the current body at `:294-299` first and keep its literal style; the block above shows the shape, not a licence to restyle.

- [ ] **Step 2: De-rot the helper's doc comment**

The comment at `:289-293` claims the helper *"preserves the three-header insertion order (Content-Type, Date, Server)"*. **The fix falsifies that.** Correct it to a four-header order naming `Content-Length`, and document the new parameter. ⚠️ **The corrected clause does not fit the two original lines** — the reflow is `+3 / -2`, which is exactly why the honest patch cannot be `-0`.

- [ ] **Step 3: Update all seven call sites**

⚠️ **KEY THE EDIT ON `h2LocalReplyHeaders()`, NEVER ON `Body: nil`** — the four body-less sites are **not homogeneous**: `retry.go:374` and `router_h2.go:80` write `Body: nil` explicitly, while `:128` and `:138` **omit the `Body` field entirely**. A patch keyed on `Body: nil` reaches only two of the four.

| site | status | pass |
|---|---|---|
| `retry.go:374` | 504 | `0` |
| `router_h2.go:80` | 503 | `0` |
| `router_h2.go:128` | 503 | `0` |
| `router_h2.go:138` | 503 | `0` |
| `router_h2.go:148` | 502 | `len(bad502Body)` |
| `router_h2.go:231` | 502 | `len(bad502Body)` |
| `router_h2.go:250` | 502 | `len(bad502Body)` |

⚠️ **`:128` AND `:138` ARE BYTE-IDENTICAL** — both read `return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}, picked, nil` at three tabs of indent. Disambiguate by the enclosing guard, never by line number: `:128` sits under `if attempt >= h2GrantRaceMaxRetries {` (`:124`); `:138` sits under `if cluster.IsConnPoolOverflow(err) {` (`:132`).

- [ ] **Step 4: Verify the edit landed — assert the SYMBOL, not the build**

A build is not evidence an edit landed.
```bash
cd /path/to/worktree
/usr/bin/grep -c 'func h2LocalReplyHeaders(bodyLen int)' internal/filter/http/router/router_h2.go   # want 1
n=$(git grep -c 'h2LocalReplyHeaders(' -- internal/filter/http/router/ | awk -F: '{s+=$2} END{print s}')
echo "call sites + def + doc: $n"   # want 9 (7 calls + 1 def + 1 doc-comment mention)
git grep -n 'h2LocalReplyHeaders()' -- internal/ ; echo "^ MUST BE EMPTY (no parameterless call survives)"
```

- [ ] **Step 5: Measure the cost with `--numstat`, never `--stat`**

```bash
git -C /path/to/worktree diff --numstat -- internal/filter/http/router/
```
Expected: `1 1 …/retry.go` and `14 9 …/router_h2.go` -> **`+15 / -10`**. ⚠️ **`git diff --stat` is a SUM, not additions** — it cannot produce this figure.

- [ ] **Step 6: Gate on OUTPUT**

```bash
gofmt -l internal/filter/http/router/            # MUST print nothing; gofmt NEVER exits non-zero
golangci-lint run ./internal/filter/http/router/...   # gate on OUTPUT, not RC
```
⚠️ **Sweep British spellings in `.go` comments first** — misspell runs in locale **US** here and fired on both the SPEC's prototype (`favour`) and this PLAN's (`behaviour` ×2).

- [ ] **Step 7: Run the package and ASSERT THE DENOMINATOR**

```bash
out=$(go test ./internal/filter/http/router/ -v -count=1 2>&1); rc=$?
echo "RC=$rc"
echo -n "RUN=";  echo "$out" | grep -c '^=== RUN' || true
echo -n "FAIL="; echo "$out" | grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' || true
```
Expected **RC=0, RUN=129, FAIL=0** at this step (Task 3 raises RUN to 133). ⚠️ **`RUN=0` beside `RC=0` is a VACUOUS GREEN.**

- [ ] **Step 8: Commit**

```bash
git -C /path/to/worktree add internal/filter/http/router/router_h2.go internal/filter/http/router/retry.go
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: h2LocalReplyHeaders takes bodyLen and always emits Content-Length"
```
⚠️ **Use EXPLICIT PATHSPECS.** ⚠️ **Never `-c user.email`** — the commit email must be the repo's configured one.

---

### Task 2: The `writeH2Reply` recompute pin — the behaviour NOTHING in the tree pins

**Files:**
- Create: `internal/filter/hcm/h2dispatch_recompute_cl_test.go`
- Read-only reference: `internal/filter/hcm/h2dispatch.go:1004` (`writeH2Reply`), `:1014-1016` (the recompute)

**Interfaces:**
- Consumes: `writeH2Reply(sw h2.StreamWriter, status int, headers filter_http.OrderedHeaders, body []byte, trailers []hpack.HeaderField) error` and the existing `captureH2Writer` test fake.
- Produces: nothing other tasks consume. This task is a **guard**, not a dependency.

⚠️ **THE ROW'S ENTIRE CORRECTNESS RESTS ON A BEHAVIOUR NOTHING PINS.** Task 1 is safe only because `writeH2Reply` recomputes an already-present `content-length` from `len(body)` before the wire. SPEC §6.1: the only evidence is a throwaway probe. **Land the pin.**

⚠️ **COST CORRECTION: SPEC §6.1 ESTIMATED `~+60` LINES. MEASURED: `147`, ~2.45×.** Do not treat 60 as a budget to squeeze into.

⚠️ **REUSE THE ESTABLISHED PATTERN, DO NOT INVENT ONE.** `TestWriteH2Reply_FrameSequence` (`h2dispatch_test.go:591`) already drives `writeH2Reply` through `captureH2Writer`. Follow it: no new fake, no new helper struct.

- [ ] **Step 1: Write the failing test**

Four rows. ⚠️ **The fourth exists because rows (a)-(c) alone CANNOT discriminate a writer that only rewrites when `len(body) > 0`** — that arm was added beyond the SPEC's three and it earns its place.

| row | carrier | body | wire expectation | role |
|---|---|---|---|---|
| `present_wrong_value_is_recomputed` | `content-length: 999` | 12 B | `"12"` | the primary pin |
| `absent_stays_absent` | no field | 12 B | **arity 0** | ⚠️ the NEGATIVE CONTROL — proves the writer *synthesizes nothing* |
| `present_correct_value_stays_correct` | `content-length: 12` | 12 B | `"12"` | a control; **cannot discriminate by design** |
| `present_wrong_value_bodyless_recomputes_to_zero` | `content-length: 999` | 0 B | `"0"` | discriminates a `len(body) > 0`-guarded rewrite |

Each row also asserts **carrier immutability** — that `writeH2Reply` does not mutate the `OrderedHeaders` it was handed. ⚠️ **This pins SPEC §4.1's premise instead of assuming it**, and that premise is the whole reason the fix must live in the composer rather than the writer.

The measured, running file is reproduced verbatim below.

```go
package hcm

import (
	"strconv"
	"strings"
	"testing"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// h2WireContentLength returns every content-length value the writer put on the
// response HEADERS block (w.headers[0]), in wire order. A slice, not a count,
// so a failure can name the offending values — and so the ABSENT case is
// distinguishable from a present "0" (reference_probe_discipline: empty is not
// zero).
func h2WireContentLength(t *testing.T, w *captureH2Writer) []string {
	t.Helper()
	if len(w.headers) == 0 {
		t.Fatalf("writer recorded no HEADERS frame")
	}
	var got []string
	for _, h := range w.headers[0] {
		if strings.EqualFold(h.Name, "content-length") {
			got = append(got, h.Value)
		}
	}
	return got
}

// TestWriteH2Reply_ContentLengthRecompute pins the ONE writeH2Reply behavior
// that phase 93's local-reply Content-Length rests on, and that NOTHING in the
// tree pinned before this test.
//
// The mechanism (h2dispatch.go writeH2Reply): the carrier is iterated as a
// slice and a field whose lowercased name is "content-length" has its value
// REPLACED by strconv.Itoa(len(body)). The field is never SYNTHESIZED — an
// absent content-length stays absent, unlike date/server which are appended as
// defaults.
//
// ⚠️ THE ABSENT ROW IS THE NEGATIVE CONTROL AND IT IS WHAT DISCRIMINATES.
// A rewrite-everything writer, a synthesize-always writer, and today's
// rewrite-if-present writer all agree on the two PRESENT rows. Only the absent
// row separates them, and only it can catch a change that starts stamping a
// content-length onto carriers that deliberately omit one.
//
// ⚠️ WHY THIS MATTERS TO THE ROUTER. Phase 93 makes h2LocalReplyHeaders emit a
// Content-Length carrying len(body). The wire value is correct under ANY
// composer value BECAUSE of the rewrite pinned here — so if this behavior
// ever changes, the router's composer becomes the sole source of wire truth
// and a wrong bodyLen would ship. This test is the tripwire for that.
//
// Errorf, not Fatalf, on every property: a row must report the arity fault and
// the value fault in the same run.
func TestWriteH2Reply_ContentLengthRecompute(t *testing.T) {
	const body = "bad gateway\n" // 12 bytes — the router's bad502Body shape.

	cases := []struct {
		name string
		// carrier is the pre-write header set handed to writeH2Reply.
		carrier filter_http.OrderedHeaders
		body    []byte
		// wantArity is how many content-length fields must reach the wire.
		wantArity int
		// wantValue is checked only when wantArity == 1.
		wantValue string
	}{
		{
			// The load-bearing row for phase 93: a PRESENT but WRONG value is
			// corrected from len(body). "999" cannot be produced by any
			// len(body) in this table, so a writer that merely echoed the
			// carrier would be caught here.
			name: "present_wrong_value_is_recomputed",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: "999"},
			},
			body:      []byte(body),
			wantArity: 1,
			wantValue: "12",
		},
		{
			// ⚠️ NEGATIVE CONTROL. A pristine carrier must come out with the
			// field ABSENT — arity 0, not "0". writeH2Reply appends date and
			// server defaults but must NOT append a content-length.
			name: "absent_stays_absent",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
			},
			body:      []byte(body),
			wantArity: 0,
		},
		{
			// A present-and-correct value survives the rewrite unchanged.
			name: "present_correct_value_stays_correct",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: strconv.Itoa(len(body))},
			},
			body:      []byte(body),
			wantArity: 1,
			wantValue: "12",
		},
		{
			// Bodyless: the recompute must read len(body), not the carrier.
			// Discriminates a writer that only rewrites when len(body) > 0.
			name: "present_wrong_value_bodyless_recomputes_to_zero",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: "999"},
			},
			body:      nil,
			wantArity: 1,
			wantValue: "0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Snapshot the carrier BEFORE the call. writeH2Reply builds its
			// own hf slice and must never mutate resp.Headers — SPEC 93 4.1's
			// access-log / encode-chain argument rests on that separation, so
			// it is pinned here rather than assumed.
			before := make([]string, len(tc.carrier))
			for i, h := range tc.carrier {
				before[i] = h.Name + ":" + h.Value
			}

			w := &captureH2Writer{}
			if err := writeH2Reply(w, 502, tc.carrier, tc.body, nil); err != nil {
				t.Fatalf("writeH2Reply: %v", err)
			}

			got := h2WireContentLength(t, w)
			if len(got) != tc.wantArity {
				t.Errorf("wire content-length arity = %d %v, want %d", len(got), got, tc.wantArity)
			} else if tc.wantArity == 1 && got[0] != tc.wantValue {
				t.Errorf("wire content-length = %q, want %q (= len(body) = %d)", got[0], tc.wantValue, len(tc.body))
			}

			for i, h := range tc.carrier {
				if now := h.Name + ":" + h.Value; now != before[i] {
					t.Errorf("carrier field %d mutated by writeH2Reply: %q, was %q", i, now, before[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run it and assert the selector matched**

```bash
out=$(go test ./internal/filter/hcm/ -run TestWriteH2Reply_ContentLengthRecompute -v -count=1 2>&1); rc=$?
echo "RC=$rc"
echo -n "RUN=";      echo "$out" | grep -c '^=== RUN' || true
echo -n "FAIL=";     echo "$out" | grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' || true
echo -n "NOTESTS=";  echo "$out" | grep -c 'no tests to run' || true
```
Expected **RC=0 · RUN=5 (parent + 4 rows) · FAIL=0 · NOTESTS=0**. ⚠️ **A `-run` selector matching nothing prints `[no tests to run]` and EXITS 0** — `NOTESTS` must read 0 or the green is fake.

- [ ] **Step 3: NC-1 — neutralise the recompute and confirm the pin reddens**

⚠️ **NEUTRALISE, DO NOT BUILD-BREAK.** A NC that fails to compile proves nothing. In `internal/filter/hcm/h2dispatch.go:1015`, change `val = strconv.Itoa(len(body))` to `_ = strconv.Itoa(len(body))` — the package still compiles.

Re-run Step 2. Expected **RC=1 · RUN=5 · FAIL=6**, with these exact failures:
```
h2dispatch_recompute_cl_test.go:137: wire content-length = "999", want "12" (= len(body) = 12)
h2dispatch_recompute_cl_test.go:137: wire content-length = "999", want "0" (= len(body) = 0)
--- FAIL: TestWriteH2Reply_ContentLengthRecompute/present_wrong_value_is_recomputed
--- PASS: TestWriteH2Reply_ContentLengthRecompute/absent_stays_absent
--- PASS: TestWriteH2Reply_ContentLengthRecompute/present_correct_value_stays_correct
--- FAIL: TestWriteH2Reply_ContentLengthRecompute/present_wrong_value_bodyless_recomputes_to_zero
```
⚠️ **CONFIRM *WHICH* ASSERTION FIRED, not merely that the run went red** — a break can fire the wrong assertion. The two present-and-wrong rows fire; the correct-value control cannot discriminate (by design); the negative control stays green.

- [ ] **Step 4: NC-2b — the ISOLATING NC that proves the negative control is not vacuous**

⚠️ **Step 3 leaves `absent_stays_absent` green, so Step 3 alone is NOT evidence that row does any work.** Restore `:1015`, then make `writeH2Reply` *synthesize* a `content-length` when the field is absent. Re-run.

Expected **RC=1 · RUN=5 · FAIL=5**, with **exactly ONE row red, and it is the negative control**:
```
h2dispatch_recompute_cl_test.go:135: wire content-length arity = 1 [12], want 0
--- PASS: .../present_wrong_value_is_recomputed
--- FAIL: .../absent_stays_absent
--- PASS: .../present_correct_value_stays_correct
--- PASS: .../present_wrong_value_bodyless_recomputes_to_zero
```
⚠️ **A coarser synthesize-always NC reddens all four rows (`RC=1, FAIL=8`) and is NOT the evidence you want** — it cannot show the negative control is independently load-bearing. **NC-2b is the one that carries the proof.**

- [ ] **Step 5: Restore and verify by digest**

```bash
git -C /path/to/worktree checkout -- internal/filter/hcm/h2dispatch.go
sha256sum -c /path/to/scratch/baseline.sha256   # captured BEFORE any NC
```
⚠️ **COMMIT TASK 1 FIRST** — `git checkout --` restores from HEAD and wipes uncommitted work.

- [ ] **Step 6: Full package, denominator asserted**

```bash
out=$(go test ./internal/filter/hcm/ -v -count=1 2>&1); rc=$?
echo "RC=$rc"; echo -n "RUN="; echo "$out" | grep -c '^=== RUN' || true
echo -n "FAIL="; echo "$out" | grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' || true
```
Expected **RC=0 · RUN=327 · FAIL=0**. ⚠️ **The baseline is 322, NOT the SPEC's 323** — measured on the clean tip and with the fix applied, both 322. The delta is +5 (parent + 4 rows).

- [ ] **Step 7: gofmt + lint, gated on output**

```bash
gofmt -l internal/filter/hcm/
golangci-lint run ./internal/filter/hcm/...
```
⚠️ **`behaviour` -> `behavior` — misspell FIRED on this exact file during the PLAN's measurement.**

- [ ] **Step 8: Commit**

```bash
git -C /path/to/worktree add internal/filter/hcm/h2dispatch_recompute_cl_test.go
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: pin writeH2Reply's content-length recompute, with an isolating negative control"
```

---

### Task 3: Unit coverage the row BRINGS — proven in BOTH directions

**Files:**
- Create: `internal/filter/http/router/router_h2_local_reply_cl_test.go`

**Interfaces:**
- Consumes: `h2LocalReplyHeaders(bodyLen int)` (Task 1), `localReplyHeaders(bodyLen int)` (`router.go:675`), `doH2ClusterAction`, and the package's existing `mkH2BackendPKI` / `h2EndpointCluster` / `h2RequestForTest` helpers.

⚠️ **THE ROW INHERITS ZERO UNIT COVERAGE.** With the fix applied, no existing Go test anywhere reddens. ⚠️ **AND THE DRIVER'S OWN TABLE IS NOT A SUBSTITUTE** — measured tree-wide with the fix reverted, **exactly ONE package reddens** and `test/fixtures/0004-h2-routing/driver` stays `ok`. That table is synthetic. **The differential is the only production gate on the driver side.**

⚠️ **NO LITERAL VALUE IS PINNED beyond "parses as a non-negative integer."** Pinning the composer's literal would pin a value `writeH2Reply` overwrites. What this row needs is that the field is **PRESENT at the encode seam**; its wire value is pinned separately, by Task 2.

⚠️ **ARITY EXACTLY ONE, NOT "AT LEAST ONE".** A duplicate `content-length` is itself a fault (RFC 9110 §8.6), so "at least one" would pass a set that must not ship.

The measured, running file (**104 lines**, SPEC estimated ~117):

```go
package router

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// contentLengthArity reports every value carried under a case-insensitive
// content-length field name, in carrier order. Returned as a slice (not a
// count) so a failure message can name the offending values.
func contentLengthArity(hdrs envoyhttp.OrderedHeaders) []string {
	var got []string
	for _, h := range hdrs {
		if strings.EqualFold(h.Name, "content-length") {
			got = append(got, h.Value)
		}
	}
	return got
}

// assertExactlyOneParseableContentLength is the ONE assertion every arm of
// TestH2LocalReplyContentLengthAlwaysEmitted runs.
//
// ⚠️ ARITY EXACTLY ONE, NOT "AT LEAST ONE". A duplicate content-length on an
// H/2 carrier is itself a protocol fault (RFC 9110 8.6), so "at least one"
// would pass a set that must not ship.
//
// ⚠️ NO LITERAL VALUE IS PINNED beyond "parses as a non-negative integer".
// writeH2Reply (h2dispatch.go) rewrites a present content-length from
// len(body) before the field reaches the wire, so pinning the composer's
// literal here would pin a value the writer overwrites. What this row needs
// is that the field is PRESENT at the encode seam; its wire value is pinned
// separately, in internal/filter/hcm.
//
// Errorf, never Fatalf: an arm must report BOTH the arity fault and any value
// fault in one run, and a Fatalf here would make the sibling arms' output
// depend on this one.
func assertExactlyOneParseableContentLength(t *testing.T, label string, hdrs envoyhttp.OrderedHeaders) {
	t.Helper()
	got := contentLengthArity(hdrs)
	if len(got) != 1 {
		t.Errorf("%s: content-length field arity = %d %v, want exactly 1", label, len(got), got)
		return
	}
	n, err := strconv.Atoi(got[0])
	if err != nil {
		t.Errorf("%s: content-length = %q does not parse as an integer: %v", label, got[0], err)
		return
	}
	if n < 0 {
		t.Errorf("%s: content-length = %d, want a non-negative integer", label, n)
	}
}

// TestH2LocalReplyContentLengthAlwaysEmitted pins the phase-93 charter: on the
// HTTP/2 leg a locally generated reply carries a Content-Length, ALWAYS.
//
// Three arms, and the middle one is a NEGATIVE CONTROL on the assertion
// itself:
//
//   - helper — the composer h2LocalReplyHeaders in isolation.
//   - h1_sibling_control — the same assertion over the H/1 sibling
//     localReplyHeaders(0), which has emitted the field since phase 04.
//     ⚠️ THIS ARM MUST BE GREEN IN BOTH DIRECTIONS. It is what makes a red
//     `helper` arm a statement about H/2 rather than about a broken helper.
//   - live_502_dial_failure — drives the REAL doH2ClusterAction against a
//     closed port, so the assertion runs over a header set an actual
//     production path composed rather than over a hand-built one.
func TestH2LocalReplyContentLengthAlwaysEmitted(t *testing.T) {
	t.Run("helper", func(t *testing.T) {
		assertExactlyOneParseableContentLength(t, "h2LocalReplyHeaders()", h2LocalReplyHeaders(len(bad502Body)))
	})

	t.Run("h1_sibling_control", func(t *testing.T) {
		assertExactlyOneParseableContentLength(t, "localReplyHeaders(0)", localReplyHeaders(0))
	})

	t.Run("live_502_dial_failure", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		// Port 1 is always refused, so the pool's dial fails and
		// doH2ClusterAction takes the synthesized-502 arm (router_h2.go).
		c := h2EndpointCluster(t, "127.0.0.1:1", pki)
		a := &routerActionH2{cluster: c}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Fatalf("doH2ClusterAction: %v", err)
		}
		// Guard the precondition: if this stopped being the 502 local-reply
		// arm, the content-length assertion below would be measuring some
		// other header set and would pass or fail for the wrong reason.
		if resp.Status != 502 {
			t.Fatalf("precondition: status = %d, want 502 (dial-failure local reply)", resp.Status)
		}
		assertExactlyOneParseableContentLength(t, "doH2ClusterAction dial-failure 502", resp.Headers)
	})
}
```

- [ ] **Step 1: Write the file above, then run it with the fix in place**

```bash
out=$(go test ./internal/filter/http/router/ -run TestH2LocalReplyContentLengthAlwaysEmitted -v -count=1 2>&1); rc=$?
echo "RC=$rc"
echo -n "RUN=";     echo "$out" | grep -c '^=== RUN' || true
echo -n "FAIL=";    echo "$out" | grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' || true
echo -n "NOTESTS="; echo "$out" | grep -c 'no tests to run' || true
```
Expected **RC=0 · RUN=4 · FAIL=0 · NOTESTS=0**, with all three arms PASS.

- [ ] **Step 2: Prove it the OTHER direction — and NEUTRALISE, do not revert**

⚠️ **A LITERAL REVERT OF TASK 1 BUILD-BREAKS THIS TEST FILE, WHICH IS NOT EVIDENCE.** Reverting the signature makes `h2LocalReplyHeaders(len(bad502Body))` uncompilable. Instead drop **only the emitted `Content-Length` field**, keeping `bodyLen int` in the signature and adding `_ = strconv.Itoa(bodyLen)` so `strconv` stays used.

Re-run. Expected **RC=1 · RUN=4 · FAIL=6**:
```
router_h2_local_reply_cl_test.go:76:  h2LocalReplyHeaders(): content-length field arity = 0 [], want exactly 1
router_h2_local_reply_cl_test.go:102: doH2ClusterAction dial-failure 502: content-length field arity = 0 [], want exactly 1
--- FAIL: TestH2LocalReplyContentLengthAlwaysEmitted/helper (0.00s)
--- PASS: TestH2LocalReplyContentLengthAlwaysEmitted/h1_sibling_control (0.00s)
--- FAIL: TestH2LocalReplyContentLengthAlwaysEmitted/live_502_dial_failure (0.00s)
```
⚠️ **EXACTLY TWO ARMS RED, H/1 CONTROL GREEN, BOTH FAILURES IN ONE RUN.** The H/1 arm staying green is what makes a red `helper` arm a statement about H/2 rather than about a broken assertion helper. **Both failures appearing together confirms non-fail-fast** — `t.Errorf`, never `t.Fatalf`, or later assertions become dead code.

⚠️ **The shared assertion uses `t.Helper()`, so the two failures carry DISTINCT call-site line numbers** (`:76` and `:102`). That is what keeps the table from being vacuated by its own shared assertion — `reference_shared_assertion_vacuates_unit_table`.

- [ ] **Step 3: Restore, then run the full package**

```bash
git -C /path/to/worktree checkout -- internal/filter/http/router/router_h2.go
out=$(go test ./internal/filter/http/router/ -v -count=1 2>&1); rc=$?
echo "RC=$rc"; echo -n "RUN="; echo "$out" | grep -c '^=== RUN' || true
```
Expected **RC=0 · RUN=133** (129 baseline + 4).

- [ ] **Step 4: gofmt + lint, gated on output; then commit**

```bash
gofmt -l internal/filter/http/router/
golangci-lint run ./internal/filter/http/router/...
git -C /path/to/worktree add internal/filter/http/router/router_h2_local_reply_cl_test.go
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: unit coverage for the always-emitted H/2 local-reply Content-Length, with the H/1 sibling as control"
```

---

### Task 4: Re-baseline the two gates the fix reddens — MEASURED, and one is a control that INVERTS

**Files:**
- Modify: `test/fixtures/0004-h2-routing/driver/driver.go:1383` (**`+1 / -1`**)
- Modify: `test/fixtures/0004-h2-routing/driver/driver_test.go` (**`+8 / -8`**)

⚠️ **THIS TASK IS NOT IN THE SPEC.** SPEC §5.1 says only that the arity pin "stops discriminating" once the fix lands. **Measured: it goes RED**, and the differential fails with exactly one error:
```
runner_test.go:1295: distribution: p92 subj content-length fields: want 0 on every arm,
                     got keepalive=1,upgrade=1,proxyconn=1,te-gzip=1,te-empty=1 (5 of 5 arms)
```
That single error is itself evidence the byte compare at step 7 **passed** and only the step-8 pin fired. ⚠️ **`driver_test.go` is named nowhere in the SPEC as a re-baseline.**

- [ ] **Step 1: Flip the subject-side want**

`driver.go:1383`: `p92WantSubjCLFields = 0` becomes `= 1`. ⚠️ **Anchor on the literal `p92WantSubjCLFields = 0`, not the line number** — and note `p92WantRefCLFields = 1` at `:1382` is **unchanged**, so the two constants now read the same value and a careless edit can hit the wrong one.

- [ ] **Step 2: Flip the seven ordinary table rows**

In `driver_test.go`, seven rows pass `p92CLObs(0)` as the `subjCL` column and must become `p92CLObs(1)` — the six distribution rows (`:73-78`) plus `"p92 ref value: 0 where 1 expected"` (`:84`).

- [ ] **Step 3: ⚠️ INVERT the eighth row — it is a NEGATIVE CONTROL, and shifting it silently disarms it**

Row `:85` currently reads:
```go
{"p92 subj value: 1 where 0 expected", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(1), true, "p92 subj content-length fields"},
```
Its `wantErr` is `true` **because** an observation of 1 contradicts a want of 0. **Once the want is 1, the observation MATCHES, no error is raised, and a `wantErr: true` row FAILS.** It must become:
```go
{"p92 subj value: 0 where 1 expected", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), true, "p92 subj content-length fields"},
```
⚠️ **Rename the row too.** A row named *"1 where 0 expected"* that actually tests the opposite is a lie the next reader will trust — `reference_break_roster_goes_stale_within_its_own_row`.

- [ ] **Step 4: Confirm the roster helper still reads LIVE, not a literal**

`p92CLObs` (`driver_test.go:30`) builds its slice from `len(p92Arms())`. ⚠️ **Do not replace it with a literal length.** Its own doc comment records that these rows went vacuous once already.

- [ ] **Step 5: Run the non-Docker sweep — it is what FOUND this task**

```bash
pkgs=$(go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$')
out=$(go test $pkgs -count=1 2>&1); rc=$?
echo "RC=$rc"; echo -n "FAIL="; echo "$out" | grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' || true
```
Expected **RC=0 · FAIL=0 · 125 `ok` + 109 no-test-files**. ⚠️ **Before the re-baseline the same sweep reads FAIL=6 — that delta IS this task.** ⚠️ **BOTH Docker drivers must be excluded; the BRAINSTORM's recipe named only the first.**

- [ ] **Step 6: Run the differential with `-count=1` and assert the PASS explicitly**

```bash
out=$(go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v 2>&1); rc=$?
echo "RC=$rc"
echo "$out" | grep -E '^\s*--- (PASS|FAIL): TestDifferential/0004-h2-routing'
echo -n "panic gate: "; echo "$out" | grep -cE '^panic:|DATA RACE|SIGSEGV' || true
```
⚠️ **`-count=1` IS NOT OPTIONAL** — the harness builds envoy-go as a **subprocess**, so a router edit is not a compile-time input to this binary and the cache serves a stale PASS. **A fix whose own gate cannot see it is the most dangerous form of this trap.** ⚠️ **Assert `--- PASS: TestDifferential/0004-h2-routing` EXPLICITLY** — an unregistered fixture is `t.Skipf`'d and no fixture-count gate exists anywhere in the tree. ⚠️ **`-race` here is VACUOUS** — the subject is an unraced subprocess.

- [ ] **Step 7: Commit**

```bash
git -C /path/to/worktree add test/fixtures/0004-h2-routing/driver/driver.go test/fixtures/0004-h2-routing/driver/driver_test.go
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: re-baseline the p92 subject arity pin 0->1 and INVERT its negative-control row"
```

---

### Task 5: The instrument — record `declared` AND `delivered`, per side, below the byte compare

**Files:**
- Modify: `test/fixtures/0004-h2-routing/driver/driver.go` — `p92DriveArm` (`:1194`), `AssertDistribution` (`:1493`), new `p93Want*` constants, and the `:1355-1380` comment de-rot
- Modify: `test/fixtures/0004-h2-routing/driver/driver_test.go` — table 10 -> 18 rows

**Interfaces:**
- Consumes: `p92Arms() []p92Arm` (`:1155`, five arms: `keepalive`, `upgrade`, `proxyconn`, `te-gzip`, `te-empty`), `p92DriveArm(...) ([]hpack.HeaderField, int, string)` (`:1194`), `errors.Join` (already used at `:1521`).
- Produces: `p93WantRefBodyLen = 87`, `p93WantSubjBodyLen = 12` — **both CONFIRMED by live measurement at this PLAN**, not claims.

**Why the pair.** `declared` alone is blind to a 0-byte body under a `content-length: 12` header; `delivered` alone is blind to a header that disagrees with its body. Together they support **one genuinely side-independent invariant plus two per-side departure pins**.

- [ ] **Step 1: Extend `p92DriveArm` to sum the delivered body**

Accumulate `len(f.Data())` across every DATA frame on the p92 stream and return it alongside the existing `(fields, status, failure)` triple.

⚠️ **The existing comment at `:1284-1285` says *"The body is NOT recorded"* — it justifies not recording the TEXT** (a forwarded 200 `p92-ok` and a local 502 carry different, side-specific text). **It does not justify not recording the LENGTH**, and the length is what an instrument needs. **Re-word that comment; do not delete it.**

- [ ] **Step 2: Make `declared` a THREE-STATE observation, never an `int`**

⚠️ **THE EMPTY-VS-ZERO TRAP IS LIVE ON THIS FIXTURE AND IT IS THIS ROW'S OWN SUBJECT.** Measured pre-fix on all five arms, the subject reads `clArity=0 declared=<absent>` — **absent, not `0`**. Model `declared` as **absent / duplicated / a parsed integer**. ⚠️ **A DUPLICATED `content-length` MUST YIELD `declaredOK = false`, NEVER "the first value"** — a duplicate is RFC 9113 §8.1.1 malformed and its value is meaningless; returning the first would launder a real defect into a plausible number.

- [ ] **Step 3: Add the four assertions in `AssertDistribution`, BELOW the byte compare**

Insert at `:1519-1520`, after the two p92 pins (`:1515`, `:1518`) and before `return errors.Join(errs...)` (`:1521`).

1. **`declared == delivered`, asserted PER SIDE as plain equality** (RFC 9110 §8.6). ⚠️ **This is the ONE property that is not a departure — it must hold on both sides independently and is NOT relaxed.** It is also the only assertion that can see a `content-length` lying about its own body, since arity now reads 1 on both sides. *(Measured: it already holds on the reference side at 87.)*
2. **Per-side body-length pins** at the measured values `p93WantRefBodyLen = 87` / `p93WantSubjBodyLen = 12`, recording the departure in both directions.

⚠️ **PLACEMENT IS LOAD-BEARING — NEVER IN `DriveReference`/`DriveSubject`.** Phase 92 MEASURED that a `Drive*` placement `t.Fatalf`s at step 5/6, **before** `CompareBytes` at step 7, and **masked the very regression it sat beside**. `AssertDistribution` is only `t.Errorf`'d, at step 8. ⚠️ **Non-fail-fast: append into `errs` so EVERY mismatching arm is named.** The function already has no `Fatalf`-equivalent — keep it that way.

- [ ] **Step 4: ⚠️ ADD THE ROSTER BARRIER — without it, zero observations satisfies every pin**

Every per-arm assertion is a range loop, so an empty slice passes all four vacuously. **Measured without a barrier: `got 0 observations, want 5` on all four pins, with one `wantErr` row passing for the wrong reason.**

Assert `len(got) == len(p92Arms())` **and** per-index arm-name identity, **read from the LIVE `p92Arms()`, never a literal.**

- [ ] **Step 5: ⚠️ DE-ROT THE 26-LINE COMMENT BLOCK AT `:1355-1380` — UNPRICED BY ANYONE**

⚠️ **NOBODY PRICED THIS.** SPEC §14 item 5 names only `README.md`. That block is falsified by this fix in **five** places:

| line | falsified claim |
|---|---|
| `:1361-1362` | *"the SUBJECT emits NONE -> arity 0"* — becomes 1 |
| `:1364-1367` | *"returns Content-Type / Date / Server and NO Content-Length"* and *"even takes the bodyLen the H/2 version lacks"* |
| `:1369` | *"so closing it is its own behavior-contract row: it is BANKED, not fixed here"* — **this row IS that row** |
| `:1372-1375` | *"if envoy-go later gains a Content-Length on H/2 local replies … THESE PINS REDDEN and a human must consciously re-derive them"* |
| `:1377-1380` | *"⚠️ ARITY, NEVER VALUE"* — the row now pins a VALUE (via `declared == delivered`) |

⚠️ **`:1372-1375` IS A PREDICTION THAT CAME TRUE AND THE DE-ROT MUST SAY SO.** Phase 92 wrote that these pins would redden and a human would have to consciously re-derive them. **That is exactly what happened.** Record it as the pin working as designed — do not quietly overwrite it.

- [ ] **Step 6: Grow the `driver_test.go` table from its MEASURED 10 rows to 18**

⚠️ **THE BASELINE IS 10, NOT 18.** SPEC §6 calls it *"its 18-row table"*; **measured at `ed4716fb` the `TestH2Driver_AssertDistribution` table is TEN rows** (`:73-78` + `:82-85`). SPEC §8's `10 -> 18` is the correct reading. **Quote no other number for this table.**

- [ ] **Step 7: NC every new assertion — all four, individually**

⚠️ **`reference_review_mandated_guard_is_untested`:** a guard the plan mandated is not thereby tested. For **each** of the four new pins, neutralise it alone and confirm **that** pin reddens and names the right arms. ⚠️ **Also NC the ROSTER BARRIER itself** by feeding an empty observation slice — it must fire, and the four pins must NOT be what reports the problem.

- [ ] **Step 8: Full gates, then commit**

```bash
gofmt -l test/fixtures/0004-h2-routing/driver/
golangci-lint run ./test/fixtures/0004-h2-routing/driver/...
out=$(go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -count=1 -v 2>&1)
echo "$out" | grep -E '^\s*--- (PASS|FAIL): TestDifferential/0004-h2-routing'
echo "$out" | grep -cE '^panic:|DATA RACE|SIGSEGV' || true
git -C /path/to/worktree add test/fixtures/0004-h2-routing/driver/
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: record declared AND delivered body length per side, with a roster barrier"
```

---

### Task 6: Correct README `:147` on its CONSEQUENCE — and the other seven lines

**Files:**
- Modify: `test/fixtures/0004-h2-routing/README.md` (189 lines; lines `:40, :137, :139, :141, :143, :145, :147, :153` — all eight line numbers CONFIRMED live at this tip)

⚠️ **CORRECT THE CLAUSE PRECISELY, OR SHIP A NEW FALSEHOOD IN THE COMMIT THAT FIXES THE OLD ONE.**

- [ ] **Step 1: Fix `:147` — the count is RIGHT, the consequence is WRONG**

The line reads, verbatim:
> **Why it is BANKED, not fixed here.** `h2LocalReplyHeaders()` has **seven** call sites across the 502/503/504 H/2 paths (`retry.go` 504; `router_h2.go` 503 x3 and 502 x3). Giving it a body length and emitting `Content-Length` changes every one of them and needs its own behavior-contract treatment with its own arms; it is unchartered and unpriced for this row.

⚠️ **THE `seven` IS CORRECT AND SO IS ITS PARENTHETICAL BREAKDOWN** — independently re-derived at this tip: `retry.go:374`=504, `router_h2.go:{80,128,138}`=503 ×3, `router_h2.go:{148,231,250}`=502 ×3. **Do not "fix" the count.**

What is false is *"changes every one of them"* — ⚠️ **and under this row's pick it is HALF-TRUE, which is the trap.** All seven call sites **do** change: each gains an argument. But not for the reason the sentence gives — it implies each site needed bespoke per-site treatment, when in fact four pass `0` and three pass `len(bad502Body)`, and the wire bytes are unchanged because `writeH2Reply` recomputes either way. **State that precisely.** The clause *"it is unchartered and unpriced for this row"* is simply superseded — this row charters and prices it.

- [ ] **Step 2: Re-state `:137`, `:141`, `:143`, `:145`, `:153` against post-fix behaviour**

- `:137` — *"reference 1, subject 0, on all five arms"* becomes **1 vs 1**; the departure it documents moves from **arity** to **body length (87 vs 12)**.
- `:141` — the heading *"H/2 local replies carry no `content-length`"* is now false as a present-tense claim; re-frame it as the phase-92-era departure that phase 93 closed.
- `:143` — the root-cause paragraph correctly describes the **pre-fix** asymmetry, including *"even takes a `bodyLen` the H/2 version does not have."* **Keep it as history, mark it closed.**
- `:145` — the compensating-defect-unmasking narrative is **still true and still valuable**; it explains why phase 92 exposed this. Leave the history, add the resolution.
- `:153` — *"⚠️ **ARITY, NEVER VALUE.**"* ⚠️ **This rule is now WRONG for this fixture**: the row pins `declared == delivered`, which IS a value assertion. The reasoning behind it — that the two 502 **bodies** differ by construction, so the body *bytes* are not a contract — **remains correct**, and the contract's ratified relaxation still stands (`BEHAVIOR_CONTRACT.md:1993`: *"Status is asserted; body is relaxed"*). **Narrow the rule rather than deleting it: body BYTES are still not compared cross-side; the LENGTH now is, per side.**
- `:139` — the placement rule (assert in `AssertDistribution`, below the byte compare, never in `Drive*`) is **unchanged and still load-bearing**. **Do not touch it.**
- `:40` — the cross-side transcript line correctly says `content-length` arity is *"deliberately NOT in the cross-side line — it is pinned per side instead."* That stays true; only the pinned values change.

- [ ] **Step 3: ⚠️ ENUMERATE THE OTHER CARRIERS — the README is NOT the only file**

```bash
git grep -ln 'ARITY, NEVER VALUE'
git grep -ln 'SEVEN local-reply sites'
```
⚠️ **`reference_measured_prototype_is_a_lower_bound` HAS FIRED SEVEN CONSECUTIVE ROWS AND FIRED TWICE INSIDE THIS STAGE.** SPEC §14 named **one** file; the PLAN's agent found **two**; the controller found **three**. The shipped carriers are `README.md:153` **and** `driver.go:1377-1380` (Task 5). ⚠️ **The third is `BRAINSTORM.md` — a HISTORICAL PHASE DOCUMENT that is NOT retro-edited.** It records what was believed at the time; its errors are corrected in the SPEC, this PLAN and ADR-0315, never in the BRAINSTORM itself.

- [ ] **Step 4: Verify no stale claim survives, then commit**

```bash
/usr/bin/grep -n 'reference 1, subject 0\|BANKED, not fixed' test/fixtures/0004-h2-routing/README.md
echo "^ must be empty or explicitly re-framed as history"
git -C /path/to/worktree add test/fixtures/0004-h2-routing/README.md
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: correct README :147 on its consequence (the seven is right) and re-state the phase-92 departure as closed"
```

---

### Task 7: Complete ADR-0315 — §Decision + §Consequences, IN PLACE

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0315 at `:18648`, currently **27 lines**, file tail

⚠️ **EXTEND FROM THE LANDED TEXT, NOT FROM SPEC §9.1's OUTLINE.** §9.1 lists **NINE** §Context paragraphs; the landed draft carries **TEN**, and the mapping differs — the landed ¶2 is the citation correction, ¶8 is a unit-coverage paragraph §9.1 omits entirely, ¶9 is the H/3 layer composition. **Renumbering to match §9.1 would corrupt paragraphs that are already correct.**

- [ ] **Step 1: Append §Decision and §Consequences after the RETAINED italic footer**

The footer *"§Decision and §Consequences follow at the phase-93 IMPL."* is **RETAINED**, per the ADR-0044 in-place discipline and the ADR-0294-0314 shared block form. ⚠️ **NO renumber. NO `---` separator.** ⇒ `^---$` **STAYS 216**; `^## ADR-` **STAYS 314**; bare `^## ` **STAYS 322**; tail **STAYS ADR-0315**; next-free **STAYS ADR-0316**.

- [ ] **Step 2: Record the citation correction as a first-class decision**

⚠️ **`ADR-0085` IS A MIS-ATTRIBUTION AND IT IS THE CHARTER'S LOAD-BEARING RULE A.** Independently re-verified at this PLAN's tip: `## ADR-0085` is at `:3282`, its block spans `:3282-:3327`, it reads *"Admin-mux reuse + LBP-1 third application"*, and it contains **ZERO** matches for `SendLocalReply|content-length|local reply|local-reply`. Both quoted sentences live under **`## ADR-0155`** (`:8187-:8381`) — `:8260` and `:8326`, both inside that block — and ADR-0155 merely **cites** "per ADR-0085" as authority.

⇒ the correct form is ***"recorded and ratified in ADR-0155, attributed there to ADR-0085."*** ⚠️ **Asserting the doctrine is IN ADR-0085 is refutable by opening `:3282`. Do not restate it.**

- [ ] **Step 3: Record the measured cost honestly, including where the SPEC was wrong**

State the MEASURED figures, not the estimates: production `+15 / -10`; recompute pin **147** lines (SPEC estimated ~60 — **a 2.45× miss**); unit test **104** (SPEC ~117); the two re-baselines `+1/-1` and `+8/-8` **that the SPEC did not name**. ⚠️ **`reference_measured_prototype_is_a_lower_bound` fires a SEVENTH consecutive row, and fired TWICE inside this stage.**

- [ ] **Step 4: DISARM the `PROPOSED` guard — and verify BY LINE AND BY ADR**

The house form `^> \*\*STATUS: PROPOSED` goes **1 -> 0**.
```bash
/usr/bin/grep -c '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md     # want 0 after
/usr/bin/grep -n '^\*\*Status:\*\* PROPOSED' docs/envoy-go/DECISIONS.md    # the DECOY: STAYS 1, at :14866 under ## ADR-0231
```
⚠️ **VERIFY BY LINE AND BY ADR, NEVER BY THE COUNT ALONE.** A session gating on the decoy reads `1` after a correct disarm and concludes it failed. ⚠️ **NEVER gate on the unanchored form (90 lines / 101 occurrences) nor the middle-ground `^\*\*Status:\*\*.*PROPOSED` (23).** This is `ADR-0312 §Consequences (x)` firing a third time — the SPEC hit it in the ARM direction, this task hits it in the DISARM direction.

- [ ] **Step 5: Commit**

```bash
git -C /path/to/worktree add docs/envoy-go/DECISIONS.md
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: complete ADR-0315 with the corrected ADR-0155 citation and the measured costs"
```

---

### Task 8: The BEHAVIOR_CONTRACT rider — and the H/3 disposition (§14 item 6)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (5966 lines) — a new `## HTTP/2 local-reply Content-Length (phase 93)` section at the **file tail**, on the `## HTTP/2 response trailer forwarding (phase 84.1)` precedent (`:5913`)

- [ ] **Step 1: Write the rider**

It states: the H/2 local-reply composer emits `Content-Length` unconditionally, valued at the body length; the value is observed **at the encode seam and by the access log** before the wire writer recomputes it; local-reply **body bytes** remain the ratified relaxation they already are (`:1993` — *"Status is asserted; body is relaxed"*).

⚠️ **NO LEDGER LINE AND NO ABSOLUTE.** The row registers no counter, so the stat surface moves **+0**, and that claim is **STRUCTURAL, not arithmetic**. ⚠️ **Three different stat-surface absolutes are live in this tree at one tip, and the contract warns of itself that a re-derivation *"should expect the re-derived figure to disagree."*** Per `reference_a_drift_correction_is_itself_a_claim`, **on a contested count: NO NUMBER.** ⚠️ **AND DO NOT SPELL THE REFUTED FIGURE IN EITHER ITS BARE OR ITS ARROW FORM** — the SPEC wrote a prohibition that quoted the token and thereby falsified itself on its own grep. Enforcement stays the per-phase `TestNoNewStat*` delta guards.

- [ ] **Step 2: ⚠️ DECISION — the H/3 comment de-rot is BANKED, not ridden (SPEC §14 item 6)**

**Decision: BANKED.** The SPEC's default stands, and here is the reasoning the IMPL should not re-litigate.

The misleading comment is at `internal/filter/hcm/h3dispatch.go` (`writeH3Reply` at `:33`, the Rule B synthesis at `:55`): *"Content-Length is synthesized ONLY when the body is non-empty (an empty-body response gets no Content-Length, never Content-Length: 0)."*

⚠️ **THE BRAINSTORM'S "TWO CONTRADICTORY RATIFIED RULES" CLAIM IS FALSE AS STATED AND INVERTS WHICH RULE IS OPERATIVE.** Re-verified at this PLAN's tip: `grep -c 'SetH2Action' internal/filter/hcm/h3dispatch.go` reads **0**, so **no H/2 carrier can reach `writeH3Reply` at all** and this row's fix cannot touch the H/3 leg. Rule A (carrier construction, `chain.go:1313`) and Rule B (synthesize-if-absent) govern different layers and compose without conflict — measured end-to-end, an empty-bodied `SendLocalReply` reaches the H/3 wire carrying `content-length: 0`, supplied by Rule A one layer up.

⇒ **the comment is a DEFECT TO REWORD, not a counter-rule to reconcile.**

**Why BANKED rather than ridden:** it changes **no behaviour**; it is wrong **today, independently of this row** — this row does not make it wrong — so the *"don't ship a new falsehood in the commit that fixes the old one"* rule that binds README `:147` does **not** bind it; and H/3 is an explicit charter non-goal. ⚠️ **BUT ADR-0315 MUST NAME IT** (its ¶9 already does), and it must carry a banked-candidate line, so a defect this row *diagnosed* is not lost by being out of scope.

⚠️ **AND `TestWriteH3Reply_EmptyBody` PASSES A NIL CARRIER**, so it pins only the synthesis arm: it neither contradicts Rule A nor detects it. **It is not a guard on the live path at all.** Do not cite it as coverage.

- [ ] **Step 3: Commit**

```bash
git -C /path/to/worktree add docs/envoy-go/BEHAVIOR_CONTRACT.md
git -C /path/to/worktree commit -m "phase 93 (h2-local-reply-content-length) IMPL: behavior-contract rider for the always-emitted H/2 local-reply Content-Length"
```

---

### Task 9: Close the row — the six-gate posture, then the lifecycle files

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md` (row 93 only), `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`

- [ ] **Step 1: Run the full six-gate posture — NAME DEPARTURES, DO NOT CLAIM COMPLIANCE**

(a)/(b) the differential **121 fixtures** + full `go test ./...` **excluding BOTH Docker drivers**, gated on `PIPESTATUS[0]` and a **SET RECONCILIATION** (⚠️ **not `INNER_EXIT`, which does not exist in this repo**); (c) h2spec cited **only from your own run** (⚠️ h2spec is measured **blind to burst-drain ordering**), grpc-conformance deferred in writing, proxy-wasm **10 families measured** (**not** "10/16"); (d) fuzzers **56 targets / 48 files** — ⚠️ **reconcile against `^func Fuzz` before quoting, the figure went stale INSIDE row 92's own commit**; (e) the anchored panic gate on every differential launch; (f) **no REVIEW.md — standing departure, named not claimed.**

- [ ] **Step 2: Flip row 93 and correct its attribution**

`ROADMAP.md` row 93: `in-progress` -> `done`, plus the date. ⚠️ **CORRECT ITS `ADR-0085` ATTRIBUTION** to *"ratified in ADR-0155, attributed there to ADR-0085"* — `:155` still carries the wrong form. ⚠️ **COUNT THE ROW'S FIELDS BEFORE AND AFTER: row 93 must stay `NF=8` under BOTH the escape-aware and the naive form.** ⚠️ **An unescaped `|` in a row PASSES the sentinel and breaks the field parse** — strip `\|` before splitting.

⚠️ **THE MOMENT ROW 93 FLIPS `done`, CHECK (2) IS AGAIN THE ONLY THING BLOCKING TERMINATION.** Check (1) goes silent. **Do NOT "tidy" a deferred-candidate line — deleting the last one ENDS THE PROJECT.** Re-run all three checks and all four NCs **after** the flip; **NC-A and NC-B both collapse from two-line to ONE-line shapes.** Re-measure, never inherit.

- [ ] **Step 3: Bank the candidates this row diagnosed but did not fix**

- **The `Content-Encoding: gzip` over an UNCOMPRESSED body corruption** — H/2-only, live on the tip. ⚠️ **EITHER phase-93 design incidentally fixes it, because both make the field present. DO NOT CLAIM CREDIT** for a fix the row did not design; the reference-side comparison is unmeasured.
- **The `h3dispatch.go` Rule B comment de-rot** (Task 8) — changes no behaviour.
- **The empty-body defect, BOTH legs** — ten sites; reference bodies MEASURED 87 / 167 / 19 / 24 B. ⚠️ **The contract ALREADY RELAXES local-reply body bytes** — a ratified relaxation, not an unnamed divergence. Adding a body emits a DATA frame where END_STREAM used to ride HEADERS.
- **New arms driving the 503/504 classes** — ⚠️ **the instrument is STRUCTURALLY INCAPABLE of seeing the body-less sites** because fixture `0004` never drives them (all five arms end on the 502 path; the driver's only `503`/`504` mention is a comment at `:1369`). **Do not claim that gap closed.**
- Carried forward unchanged: the pooled-upstream-lifetime defect · `ssl.connection_error` · 1xx interim responses · the H/1 no-`Host` divergence · `allocateOTLPPort` · the stat surface count (BLOCKED).

- [ ] **Step 4: Roll `STATE.md` IN PLACE, evicting by DIRECT DATE READ**

⚠️ **AT THIS TIP THERE IS NO UNIQUE OLDEST — THE COINCIDENCE HAS BROKEN.** Measured: `STATE.md:52` and `:54` **both read `2026-08-25`**. The date read alone does **not** resolve the evictee; it is resolved by **lifecycle precedence** (a phase's BRAINSTORM precedes its SPEC), giving `phase 92 (h2-response-header-validation) BRAINSTORM done`. ⚠️ **It sits last AGAIN — for the third close running — which is exactly why the tie-break must be STATED. A session reading position gets the right answer for the wrong reason.** Re-read the dates yourself; do not inherit this.

⚠️ **ROLL THE §Recent PREAMBLE SENTENCE TOO** — `^- \*\*prior active-phase:\*\*` cannot match a line beginning `*(`, so the guard is **structurally blind** to it and it fossilised for eight closes.

- [ ] **Step 5: Archive to `STATE_HISTORY.md` as ONE INLINE LINE**

Its label must **NOT** match `^- \*\*prior active-phase:\*\*`, so the strict guard stays at **163, DELTA 0**. ⚠️ **THE STRICT 163 IS NOT THE ENTRY COUNT** — measured, bullet-anchored **217** = strict **163** + parenthetical **54**, exactly. It is sound as a **delta-0 shape check**; any prose calling 163 "the entry count" is wrong, and **any gate must name WHICH form it uses** (strict 163 / colon 165 / loose 217; the colon form carries two false positives).

⚠️ **DO NOT NAME A POSITIVE CONTROL IN THE ARCHIVE LINE.** The archive's controls are **self-incrementing**: a control recorded in the file it measures is invalidated by the act of recording it (the phase-93 SPEC's two controls went 8->9 and 1->2 by its own line). **The simplest correct move is to name none.** If you name one anyway, you MUST disclose the +1-per-use.

⚠️ **The eviction check has the same trap:** the §Recent preamble NAMES the evictee, so a naive "is it gone?" grep on `STATE.md` reads **1**. **The discriminating form is the strict `^- \*\*prior active-phase:\*\* \`<label>\`` form** — verified **0** for this evictee at this PLAN, by both the strict and the naive form.

- [ ] **Step 6: Roll `next-prompt.txt` and land ONE squashed commit**

```bash
git -C /path/to/worktree add -f next-prompt.txt    # TRACKED but gitignored — plain `add` silently skips it
```
⚠️ **Put the FULL SLUG in the subject.** Subagents commit locally on the one stage branch; **the controller squashes, merges and pushes** — subagents never push. ⚠️ **If a late measurement invalidates something already pushed, land a CLEARLY-LABELLED correction commit with the full slug in its subject** — never force-push, never leave it standing.

---

## Sentinel — RUN MECHANICALLY AT THIS PLAN'S OWN TIP (`ed4716fb`), ACTUAL OUTPUT

⚠️ **The SPEC's re-confirmation does NOT carry forward.** Every figure below was re-measured by this stage at its own tip, not inherited.

- **(1)** `NOT DONE: row 93` — **ONE** line at `want=125`. ⇒ correct while row 93 is open.
- **(2)** **SIX** windows, at `:203 :209 :215 :225 :231 :239`. Per-line md5 baselined: `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 4ad940205410 b2680e6f4fbf 6caa1c3ce0e7`. ⚠️ **A doctoring NC on `:225` moved ONLY that digest (`4ad940205410 -> e0dc6ee1c42a`), proving the comparator discriminates.**
- **(3)** SILENT.

**All four mandated NCs run, ACTUAL output:**
- **NC-A** (doctor row 62): **TWO** lines — `NOT DONE: row 62` then `NOT DONE: row 93`.
- **NC-B** (`want=124` on the real file): **TWO** lines — `NOT DONE: row 93` + `GATE FAIL: examined 125 data rows, expected 124`.
- **NC-C** (check-3 doctored): **FIRED** — `NEVER OPENED: gRPC`; WASM control **2**; doctored-token count **0**.
- **NC-D**: long **5** / short **1** / union **6**.

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent at the git root and in all four stage worktrees.

⚠️ **THE PLAN CHANGES NO ROW, SO THE `ROADMAP.md` NO-OP IS ITSELF THE EVIDENCE** — sha256 `82baac19fa2c1c13…`, byte-identical across this stage, and a `--numstat` of nothing. ⚠️ **The margin is two ONLY while row 93 is open.**

**Counts re-derived at this tip, each with its named hazard:**

| axis | value | hazard |
|---|---|---|
| `ROADMAP.md` | **243** lines / **125** rows / tail **93** | row 93 is `NF=8` under BOTH the escape-aware and naive forms |
| malformed rows | **2** (ids 57 `NF=9`, 69 `NF=10`) | ⚠️ **a naive `awk -F'|'` reads SEVENTEEN** — strip `\|` before splitting; the forms DISAGREE on 57, AGREE on 69 |
| `DECISIONS.md` | **18674** lines, `^---$` **216**, `^## ADR-` **314**, bare `^## ` **322** (= 314 + 8 `## Amendment`) | tail **ADR-0315**, next-free **ADR-0316** — ⚠️ **TAIL-derived; headings+1 reads 315, the TAIL ITSELF, a TAKEN id** (the id space is sparse, one gap at `0209`) |
| `BEHAVIOR_CONTRACT.md` | **5966** | byte-untouched by this stage |
| `STATE.md` / `STATE_HISTORY.md` | **63** / **534** | strict guard **163**, parenthetical **54**, loose **217** = 163 + 54 exactly |
| phase dirs / fixtures | **134** / **121** (tail `0119`, **`0120` FREE**) | ⚠️ `grep -cE '^[0-9]{4}-'` reads **119** — it drops `0007a`/`0007b`; use `ls -d test/fixtures/*/ \| wc -l` |
| fuzzers | **56** targets / **48** files | reconcile against `^func Fuzz` before quoting |
| BackendKind | tail **38** (**39** declared, values 0-38) | ⚠️ a TAIL VALUE, not a count — do NOT "fix" it to 39 |
| `go.mod` | **67** require entries (18 direct + 49 indirect), **76** lines | ⚠️ **67 is the REQUIRE count, not the line count** — `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` reads **62**, missing uppercase/underscore paths |
| `-family row` | **95** occurrences / **67** lines | ⚠️ **always pass `--` before the pattern**; `grep -c` counts LINES, not occurrences |
| `PROPOSED` guard | house form **1** at `:18650` under `## ADR-0315`; decoy **1** at `:14866` under `## ADR-0231` | ⚠️ **VERIFY BY LINE AND BY ADR** |
| stat surface | **+0, and NO ABSOLUTE IS QUOTED** | three different absolutes are live at one tip; on a contested count: NO NUMBER |

⚠️ **`reference_a_drift_correction_is_itself_a_claim` — every number above is re-derived, including the ones this stage inherited and confirmed.**

---

## Self-review

**Spec coverage — SPEC §14's eight items, each mapped:**

| # | what the PLAN owed | where | status |
|---|---|---|---|
| 1 | Land the §4 reversal explicitly | Task 1 preamble | **DONE** — carried with its refutation, not silently |
| 2 | Measure the differential golden churn | §0 Confirmations | **DISCHARGED** — **ZERO**, proven by sha256 (420 B, `c9739f62…`, both sides, both directions), and structurally: **no stored baseline exists in `test/`** |
| 3 | Confirm `p93WantRefBodyLen` on a live `-count=1` run | §0 Confirmations, Task 5 | **DISCHARGED** — **87 CONFIRMED** on all five arms; `p93WantSubjBodyLen = 12` also **measured**, not merely derived |
| 4 | Price and land the `writeH2Reply` recompute pin | Task 2 | **DONE** — **147 lines MEASURED**, SPEC's ~60 **refuted** at 2.45× |
| 5 | Correct README `:147` on its CONSEQUENCE | Task 6 | **DONE** — plus **two further carriers** the SPEC did not name |
| 6 | Decide the H/3 comment de-rot | Task 8 Step 2 | **DECIDED: BANKED**, with reasoning, and ADR-0315 must name it |
| 7 | Re-derive every §10.1 anchor at the PLAN's tip | §0 Confirmations | **DONE** — all six still correct; **two SPEC "drift-proof" anchors found NOT UNIQUE**, one cited without its package |
| 8 | Carry §12's nine gate rules verbatim | Global Constraints G1-G15 | **DONE** — carried and extended to 15 |

**Placeholder scan:** no `TBD`, no "add appropriate error handling", no "similar to Task N". Both new test files are embedded **verbatim from a compiling, running, both-directions-NC'd prototype**. Two items are labelled **UNPRICED** (the `driver.go:1355-1380` comment de-rot) and **SPEC-MEASURED, not re-measured here** (the `+300/-44` instrument) — ⚠️ **these are honest labels on measurements this stage did not make, not placeholders**; the cost table marks every row MEASURED / SPEC-MEASURED / ESTIMATED / UNPRICED.

**Type consistency:** `h2LocalReplyHeaders(bodyLen int)` is used identically in Tasks 1, 3 and 6. `p92CLObs(int) []int`, `p92Arms() []p92Arm`, `p92DriveArm(...) ([]hpack.HeaderField, int, string)` and `p92WantSubjCLFields` match the live tree (verified `:1155`, `:1194`, `:1383`). `p93WantRefBodyLen` / `p93WantSubjBodyLen` are introduced once, in Task 5, and referenced nowhere earlier.

**Known gap, named not hidden:** Task 5's instrument (`~+300/-44`) and Task 6's README rewrite (`~+18/-8`) are the two largest un-re-measured items. Both were measured by the SPEC or estimated there; **this stage measured the fix, both test files, and both re-baselines instead**, on the judgment that the row's *correctness* spine mattered more than re-pricing its documentation. ⚠️ **The IMPL should expect the instrument's real cost to exceed `+300/-44`, because `reference_measured_prototype_is_a_lower_bound` has now fired seven consecutive rows and the `:1355-1380` de-rot is on top of it.**
