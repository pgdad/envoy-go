# Phase 96 — `listener-default-chain-tlsmode` — PROGRESS (IMPL)

**All 19 tasks of `PLAN.md` §5 landed, in order.** Lifecycle-state **3 -> DONE**; ROADMAP row 96
flipped `in-progress` -> `done` at Task 16.

⚠️ **Every figure below is a CAPTURE, not a forecast.** Quoting is not executing; each was produced by
running its command at this stage's own tip.

---

## 1. Task ledger

| task | what landed | commit |
|---|---|---|
| 1 | panic gate proven LIVE + un-fixed baseline | *no commit (evidence)* |
| 2 | shape-A live-handshake crash pin, RED at tip by process abort | `4595bfa7` |
| 3 | shape-B crash pin — `len(chains) == 0` is NOT the boundary | `797a4521` |
| 4 | arm (f), the QUIC registration pin that must `Start` | `7ad54349` |
| 5 | INVERT the dead assertion, DELETE its mirror, add arms (d) and (e) | `29dc6243` |
| 6 | **THE ROW** — the one-line `tlsMode` predicate | `9971ca63` |
| 7 | `.go` prose reconciliation, sites 5, 6, 7, 9 | `19cefc8d` |
| 8 | `DECISIONS.md` sites 1 and 2 + two stale cites | `00be6809` |
| 9 | fixture `0121` — directory, PKI, both bootstraps | `d4313ed9` |
| 10 | fixture `0121` driver — the MEASURED map at N=3 | `57778349` |
| 11 | the three registration gates, proven and NC'd | `4cc1add9` |
| 12 | `expectations.yaml` and `README.md` | `da6e4060` |
| 13 | the fixture NCs | *no commit (evidence)* |
| 14 | `BEHAVIOR_CONTRACT.md` site 3 on both axes + ledger entry | `0294e56a` |
| 15 | `ADR-0318` §Decision + §Consequences; house guard DISARMED | `68b289c1` |
| 16 | `ROADMAP.md` row 96 -> `done`, `+1 fixtures`, **SITE 10** | `57cb5482` |
| — | CORRECTION: the QUIC accept-loop claim (see §5) | `13fd60a8` |
| 17 | byte-untouched roster, asserted and PROVEN LIVE | *no commit (evidence)* |
| 18 | the six-gate sweep | *no commit (evidence)* |
| 19 | this file + `STATE.md` + `STATE_HISTORY.md` + `next-prompt.txt` | the close |

The task commits above are the pre-squash SHAs on the three stage branches
(`phase-96-impl`, `phase-96-fixture`, `phase-96-docs`); the close squashes them into one.

---

## 2. The un-fixed tip WAS spent first, and the evidence cannot be recovered afterwards

`PLAN.md` §5's ordering is load-bearing: Tasks 1-5 land and RUN at the un-fixed tip, which IS the
negative control for NC roster rows 1-3 and is consumed the moment Task 6 widens the predicate.

**Task 1 — baseline and gate liveness (roster row 8).**
```
go list ./internal/listener/...          -> 3 packages (selector resolves)
go test ./internal/listener/ -count=1 -v -> RC=0, `=== RUN` 157,
                                            anchored FAIL `^(FAIL|--- FAIL)|^ *--- FAIL` = 0,
                                            anchored gate `^panic:|DATA RACE|SIGSEGV`   = 0
TestListenerMetrics_GateMatchesInc       -> PASS, 3 arms (a)(b)(c)
```
⚠️ **A gate that reads 0 has not been shown to work.** With `panic("t1 gate liveness probe")` inserted
into a scratch test the same anchored command read **1**, first line
`panic: t1 gate liveness probe [recovered, repanicked]`; with the probe removed it read **0** again.
The zero above is therefore a measurement, not a broken command.

**Tasks 2 and 3 — the two crash pins, RED AT TIP BY PROCESS ABORT.** Each run alone:
```
RC=1 · `=== RUN` 1 (non-zero: not a `[no tests to run]` exit-0) · anchored gate 2
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x10]
  internal/stats.(*Counter).Inc(...)              internal/stats/counter.go:22
  internal/listener.(*listenerRuntime).serveConnection(...)
                                                  internal/listener/manager.go:1393
  created by ...acceptLoop                        internal/listener/manager.go:1268
```
⚠️ **The POINTER assertions were observed to fire BEFORE the abort** — `rt.tlsMode = false, want true`
then all five `ssl.*` pointers `= nil, want non-nil`. That is what distinguishes the CAUSE from the
EFFECT ([[reference_nil_stats_counter_inc_crashes_goroutine]]). Two assertions in the shared helper
correctly did NOT fire because they PASS at the un-fixed tip: `rt.defaultChain`/`.tlsCfg` are both
non-nil, and `len(rt.chainSpecs)` matched (0 for shape A, **1** for shape B). Shape B carrying a filter
chain and crashing identically is the whole point: **`len(chains) == 0` is not the boundary.**

**Tasks 4 and 5 — arms (d), (e), (f) at the un-fixed tip.** `RC=1`, 7 `=== RUN`, **zero** `panic:` —
arms (d)/(e)/(f) are build/registration arms and do not dial. Arm (f) is the distinguishing signature:
**a REGISTRATION failure with NO crash**, six assertions on six distinct lines, while
`rt.kind == kindQUIC`, `rt.defaultChain != nil`, `.tlsCfg != nil` and `len(rt.chainSpecs) == 0` all
PASSED — red on the defect and nothing else. Arm (f) calls `mgr.Start(ctx)`; without it the arm would
have been red regardless of the fix (`PLAN.md` §0.9) and roster row 3 would have fired for the wrong
reason. The inverted arm-(c) assertion stayed GREEN and was shown non-vacuous by a scratch control that
negated its `want` expression (RED at `manager_test.go:2515`, restored green).

---

## 3. Task 6 — the row itself

`internal/listener/manager.go`, the sole `tlsMode:` write in the file, `+1 / -1`. Backward function
search resolves it to `func buildListenerRuntimeWithCtx(` at `:562`; the short form
`buildListenerRuntime(` reads **0** — it does not exist.

**GREEN AFTER:** `RC=0`, `=== RUN` **162** (157 + 5), anchored FAIL **0**, anchored gate **0**. All six
`TestListenerMetrics_GateMatchesInc` arms PASS and both crash pins PASS.

**NC roster rows 1, 2 and 3, re-run by REVERTING the predicate, one at a time:**

| row | arm | result |
|---|---|---|
| 1 | shape A | binary ABORTS · RC=1 · `=== RUN` 1 · anchored gate **2** |
| 2 | shape B | binary ABORTS · RC=1 · `=== RUN` 1 · anchored gate **2** |
| 3 | arm (f) | five pointers NIL · RC=1 · `=== RUN` 2 · anchored gate **0** — REGISTRATION failure, NO crash |

Predicate restored, `sha256sum -c` **OK**, re-run green at RUN 162 / FAIL 0 / gate 0 before committing.

⚠️ **THE CRASH LINE MOVED AND THE LITERAL IS THE IDENTITY.** `PLAN.md` §3.5 pins the crash at
`manager.go:1393`; under the reverting NC it fires at **`:1406`**, because Task 6's own Step-6 doc
rewrite inserted 13 lines above it. `:1406` IS `rt.sslHandshake.Inc()`, verified by direct read — the
SUCCESS-path Inc, not the `fail_verify_*` classifier sites, which sit at `:1389`/`:1391` (and
`:1398`/`:1411`). **A line-number pin would have read as a finding here; the literal does not**
([[reference_line_shift_after_insert_is_banded]]).

---

## 4. Task 13 — the fixture NCs, and 🔴 TWO ROSTER ROWS THAT CANNOT FIRE AS SPECIFIED

Fixture `0121` at the un-fixed tip was RED **one step earlier than the PLAN predicted**: the subject
process SIGSEGV'd at `manager.go:1393` on arm 1's handshake and arms 2-3 got connection-refused, so the
failing assertion was `runner_test.go:1278: subj drive`, **not** `AssertStats`. The absent names are the
cause; the crash is the symptom that arrives first. With Task 6 landed the fixture is GREEN.

| roster row | specified expectation | ACTUAL |
|---|---|---|
| 6 — delete `AssertStats` | "the fixture goes RED" | 🔴 **DID NOT FIRE — RC=0, GREEN** |
| 7 — drop one of the five names | "RED" | 🔴 **DID NOT FIRE — RC=0, GREEN** |
| 10 — `no_certificate` 3 -> 0 | RED on BOTH sides | ✅ **FIRED**, both sides |

🔴 **BOTH NON-FIRINGS ARE FLAWS IN THE CONTROLS, NOT DEFECTS IN THE FIXTURE — and the PLAN contradicts
itself on row 6.** `PLAN.md` Task 10 Step 1 states that the runner dispatches step 10 *"via a SILENT
type assertion with no `else` branch"*; verified at tip, `test/differential/runner_test.go:1351` reads
`if sa, ok := d.(fixture.StatsAsserter); ok { sa.AssertStats(...) }`. **Deleting the method therefore
makes `ok == false` and the stats leg is skipped — green is the STRUCTURALLY GUARANTEED outcome.** Row 6
as written asserts the opposite of what Task 10 Step 1 proves. Row 7 fails for a simpler reason:
**dropping an assertion that PASSES can never redden a run.** Removing coverage is invisible by
construction — which is precisely the `+0/+0`-control hazard
([[reference_deleted_zero_delta_control_is_invisible]]) appearing inside the NC roster itself.

**BOTH CONTROLS WERE REPAIRED AND RE-RUN, AND BOTH REPAIRED FORMS FIRED:**

- **row 6 repaired** — KEEP the method, add an unconditional `t.Errorf`. Result: `RC=1`, anchored FAIL 5,
  message `runner_test.go:1352: NC row 6 REPAIRED: AssertStats WAS dispatched by the runner`.
  ⇒ dispatch is LIVE; the compile-time `_ fixture.StatsAsserter` assertion is doing its job.
- **row 7 repaired** — keep the name, corrupt its EXPECTED value (`wantConnectionError` 0 -> 1). Result:
  `RC=1`, anchored FAIL 5, and it fired on **BOTH** sides:
  `reference: envoy_listener_ssl_connection_error = 0, want 1` ·
  `subject: envoy_listener_ssl_connection_error = 0, want 1`.
  ⇒ the NEGATIVE half is asserted, per side, not merely scraped.
- **row 10** — `wantNoCertificate` -> 0, i.e. the `{handshake: N, rest: 0}` map `SPEC.md` §14 item 5
  warns about. Fired on BOTH sides:
  `reference: envoy_listener_ssl_no_certificate = 3, want 0` ·
  `subject: envoy_listener_ssl_no_certificate = 3, want 0`.
  ⇒ **the warning is now evidence**: that map fails against CORRECT code, on both sides, exactly as
  `PLAN.md` §3.1 measured.

Every mutation was applied to a working copy and reverted under `sha256sum -c` (**OK** each time);
`git status --porcelain --untracked-files=all` EMPTY in the worktree and in the canonical root.

**Task 11's roster rows 4 and 5** (run by the fixture agent, reported and accepted):
row 5 (rename in a scratch copy) — count stayed **123**, split stayed 99+24, **both** `comm` directions
fired. Row 4 (delete the new import) — imports 123 -> **122**, driver split 99 -> 98, `comm -23` fired
with `0121-listener-default-chain-tls`. ⚠️ **`comm -13` read EMPTY, not fired** — and that is a third
mis-specified expectation: **a pure deletion cannot manufacture an import-with-no-directory**, so
"both directions" belongs to the rename control alone. The gate still fires.

---

## 5. What this IMPL REFUTED, by execution

Method note 2: every stage's job is to refute its predecessor. The 96 PLAN refuted fifteen; this IMPL
refutes **six**, four of them inside the PLAN's own task steps and one against its split-gate verdict (§9).

1. 🔴 **`Manager.Start` DOES launch an accept loop for `kindQUIC`.** The PLAN (Task 4 Step 6, Task 14
   Step 1) and the SPEC both say it does not. Measured: `Manager.Start`'s FIRST range loop (`:1148`)
   dispatches `kindQUIC` to `startQUIC`, which runs `go rt.quicAcceptLoop(ctx, ql)` (`quic.go:88`) ->
   `go rt.serveQUICConnection(...)` (`:104`). What is skipped is the **TCP** accept loop: the SECOND
   range loop (`:1181`) `continue`s on `rt.kind == kindQUIC`, and `serveConnection` — the sole Inc site
   for this family — has **exactly one** call site in the package, `manager.go:1268`, on that TCP path.
   The CONCLUSION (permanently zero, PARITY) survives; only the mechanism was wrong. ⚠️ Note that
   `BEHAVIOR_CONTRACT.md:1973`'s surrounding paragraph ALREADY stated the symbol-anchored version
   correctly and warned that these anchors must be found by SHAPE — the loose paraphrase was introduced
   beside a correct statement and contradicted it. Corrected at three sites (`13fd60a8`).
2. 🔴 **NC roster row 6 cannot fire as specified** — see §4. The PLAN's own Task 10 Step 1 proves it.
3. 🔴 **NC roster row 7 cannot fire as specified** — dropping a passing assertion cannot redden.
4. 🔴 **Task 11 Step 4's "both `comm` directions" is impossible for a pure deletion** — see §4.
5. ⚠️ **`go list ./...` is 239, not 237** (**237** non-Docker, not 235). The delta is exactly this row's
   two new packages, named: `test/fixtures/0121-listener-default-chain-tls/driver` and
   `.../pki/gen`. `PLAN.md` §7's 237/235 was correct AT ITS OWN TIP and is stale by construction the
   moment the fixture lands — the §0.1 trap in miniature.

Also recorded: **`PLAN.md` §3.5's `manager.go:1393` crash-line pin is stale after Task 6's own Step 6**
(now `:1406`, same literal) — see §3; and the fixture's pre-fix red arrives at `subj drive`, not at
`AssertStats` — see §4.

---

## 6. Task 17 — the byte-untouched roster

**The three safe members — whole-file `sha256sum`, correct because no `SPEC.md` §11 edit-roster row
names any of them:**
```
071cb7a536872da1605f3ef213187141b7aa19029ffd001569e4bda7845b0edb  internal/listener/quic.go
internal/tls   —  8 files
internal/stats — 23 files
find internal/tls internal/stats -type f | sort | xargs sha256sum, master vs tip:
  ALL BYTE-IDENTICAL — 0 mismatches
```

**🔴 `internal/listener/manager.go` is on BOTH rosters, and `PLAN.md` §0.13's prediction that BOTH
obvious guards fail against correct code is CONFIRMED BY EXECUTION:**
```
whole-file digest   master 7718e175fb514829 -> tip 746e27f494cf3202   CHANGED, as it MUST
sed -n '746,747p'   master 9154e453c99515d9 -> tip 2f5a5475a17f62b8   CHANGED, for the WRONG reason
                    (the D4 block now sits at :759 — Task 6 Step 6 inserted 13 lines above it)
```
**The two-part literal-anchored guard, which is invariant under that shift:**
```
grep -c -F 'Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture' …  -> 1
grep -A1 -F '…' … | sha256sum
  -> 9154e453c99515d96a5d4bd9aea17fcd9b70c39682bf4418f40e0136a8ec0817
```
**NC roster row 11 — the guard PROVEN LIVE:** on a scratch copy, mutating ONE character on the block's
second line moved the digest to `7f5950ce39836c3335885266ca34825acedd7373`. A matching digest is
therefore evidence, not a resting state.

**Step 4 — the SET DIFFERENCE, computed mechanically with `comm -12`:**
**INTERSECTION = `internal/listener/manager.go`, AND IT ALONE.**

**NC roster row 12 — DIFF THE ARM ROSTER, not the counters:**
`(a)(b)(c)` -> `(a)(b)(c)(d)(e)(f)` — **3 -> 6, and `comm -23` on the arm names is EMPTY: none removed.**

---

## 7. Task 18 — the six gates. DEPARTURES ARE NAMED; COMPLIANCE IS NOT CLAIMED.

**(a) DIFFERENTIAL — `123/123`.** `go test ./test/differential/ -count=1 -v`:
`RC=0`, 140 `=== RUN`, anchored FAIL **0**, anchored gate **0**, `ok … 407.357s`;
**123 distinct fixtures PASSED**. Fixture set asserted BY NAME, BOTH DIRECTIONS, on the anchored
blank-import extractor: **dirs 123 = imports 123**, `comm -23` EMPTY, `comm -13` EMPTY, split
**99 `driver/` + 24 `inputs/`**. ⚠️ `-race` on this suite would be VACUOUS — the subject is an unraced
subprocess — so it was not run here.

**(b) NON-DOCKER SWEEP — 237 packages, gated on `PIPESTATUS[0]` plus a SET RECONCILIATION.**
`go list ./...` reads **239**; the two Docker drivers excluded by name are
`test/differential` and `test/conformance/h2spec`. `PIPESTATUS[0]=0`, anchored FAIL **0**, anchored
gate **0**. Reconciliation: **declared 237 = reported 237**, `comm -23` EMPTY, `comm -13` EMPTY, status
tokens `125 ok` + `112 ?` (no test files). ⚠️ **237/239, not the PLAN's 235/237** — the delta is this
row's two new packages, named in §5.
⚠️ **The FULL `internal/listener` package under `-race`**: `RC=0`, `=== RUN` 162, FAIL 0, gate 0.
⚠️ **The first reconciliation extractor read 0 and was WRONG, not the run** — `[ \t]` inside a GNU ERE
bracket expression is a literal backslash and `t`, not a tab. Re-run with a correct extractor.
**A silent 0 from a reconciliation is indistinguishable from a broken matcher; both were checked.**

**(c) h2spec — `95 tests, 94 passed, 1 skipped, 0 failed`** (the skip is 6.9.2/2, invariant). `RC=0`.

**(d) FUZZERS — `56 / 48`**, reconciled against the matcher before quoting:
`grep -rhE '^func Fuzz' --include='*.go' .` -> **56** targets; `grep -rlE …` -> **48** files.

**(e) THE ANCHORED PANIC GATE — `^panic:|DATA RACE|SIGSEGV`, `0`, AND PROVEN LIVE AT THE POST-FIX TIP.**
Resting **0** on every gate log of this stage (differential, non-Docker sweep, h2spec, `-race`, unit).
Re-proven live after the fix — because the fix is precisely what removes the abort: a scratch
`panic("t18 post-fix gate liveness probe")` made the same command read **1**. Probe removed, tree clean.

**(f) NO `REVIEW.md` — the STANDING DEPARTURE, NAMED.** `docs/envoy-go/phases/96-…/` contains
`BRAINSTORM.md`, `SPEC.md`, `PLAN.md` and this file. Other phases carry one; this row does not, and that
is a departure rather than compliance.

**Repo hygiene:** `gofmt -l .` output EMPTY (gated on output — it never exits non-zero) ·
`go vet` rc 0 · `golangci-lint run` rc 0 on both changed package trees (US-locale misspell clean) ·
`go mod tidy -diff` EMPTY · `git diff master -- go.mod go.sum` EMPTY, require entries **67** under the
structural `awk` extractor · fixtures **123**.

**Step 7 — the flake register.** Every live member was searched for in this stage's own gate logs:
`TestSDSEndToEnd_FetchFailure_BootFailsClosed`, `TestOutlierDetector_ConcurrentEjectExactlyOnce`,
`TestFramer_ReaderGoroutineDoesNotLeak`,
`TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure`, `TestServerConn_TinyWindowDelivery` —
**0 FAIL occurrences each**; `0061-lb-ring-hash` and every other fixture covered by gate (a) at
123/123. ⚠️ **A GREEN RERUN CLEARS NOTHING** — no registration is retired here.

---

## 8. Sentinel and guards at this close

Run mechanically BEFORE and AFTER the `ROADMAP.md` edit, because **the shapes move inside this stage**:

| | before Task 16 | after Task 16 |
|---|---|---|
| check (1), `want=128` | `NOT DONE: row 96` — one line | **SILENT** |
| check (2) | SIX at `:206 :212 :218 :228 :234 :242` | **SIX**, same anchors |
| check (3) | SILENT | SILENT |
| NC-A | TWO lines | **ONE** (`NOT DONE: row 62`) |
| NC-B | TWO lines | **ONE** (`GATE FAIL: examined 128 data rows, expected 127`) |
| NC-C | fired, residual 0 | fired, residual 0 |
| NC-D | 96 occurrences / 68 lines | 96 / 68 |
| check-(2) positive control | 6 -> 0, 6 substitutions asserted | same |

Per-line md5 of the six sentinel windows, **trailing newline INCLUDED**, BYTE-IDENTICAL before and
after: `206 10d7807bf02d` · `212 4a92f7e62fc6` · `218 2a7eb298b9fd` · `228 242e53c6f7a3` ·
`234 b2680e6f4fbf` · `242 6caa1c3ce0e7`.

⇒ **THE SENTINEL DOES NOT FIRE** — check (2) still reads SIX. **`stop` WAS EVALUATED AND DELIBERATELY
NOT CREATED**, verified absent at the git root and in every stage worktree. No deferred-candidate line
was tidied.

🔴 **A FINDING AGAINST THIS STAGE'S OWN AUTHOR.** The first draft of row 96's IMPL-done narrative wrote
the Go predicate with a literal `||`. Measured immediately, before committing: row 96 went from NF 8/8
to naive **10** / escape-aware **10**, and the escape-aware malformed set grew from `{57, 69}` to
`{57, 69, 96}` — **while check (1) stayed SILENT throughout**, exactly as method note 24 says it would.
Repaired by REWORDING THE PIPE AWAY, not by escaping it. Post-repair: row 96 back to **8/8**, row 74
naive 10 / escape-aware **8**, malformed set exactly `{57, 69}`. `ROADMAP.md` STAYS **246** lines and
`want` STAYS **128** — a FLIP, not an ADD.

**`DECISIONS.md`.** Tasks 8 and 15 add no ADR and no separator: `^---$` STAYS **216**, `^## ADR-` STAYS
**317**, bare `^## ` STAYS **325**, tail STAYS **ADR-0318**, `^## ADR-0319` STAYS **0**; next-free
**ADR-0319**, TAIL-derived. File 18972 -> **19050**.
**The house `PROPOSED` guard is DISARMED** — ADR-0318 flipped in place, `PROPOSED` -> `ACCEPTED`,
verified BY LINE AND BY ADR (every remaining hit resolved backward to its enclosing `## ADR-` heading),
never by the count alone. The `^**Status:** PROPOSED` decoy still resolves to `## ADR-0231` at `:14864`
and its line's sha256 is byte-identical to master's. **NC roster row 9 FIRED**: on a scratch copy an
appended house-form line moved the count by exactly one. ⚠️ A zero on that guard is the RESTING STATE.

**`BEHAVIOR_CONTRACT.md`** 5989 -> **5991**. Deliberately-LEFT siblings `:1042` and `:1967` asserted
BYTE-IDENTICAL to master by per-line sha256. `406` / `406 -> 407` restated nowhere (0 additions match).

**The ten-site occurrence set is fully reconciled.** Every surviving hit of the false invariant under
`internal/` is a QUOTE-TO-REPEAL inside a correction block. Sites deliberately LEFT are enumerated in
the Task 7 commit with their reasons. Historical copies under `docs/envoy-go/phases/74-*, 92-*, 95-*`
stay byte-untouched.

---

## 9. Envelope

**+1 fixtures** (`0121-listener-default-chain-tls`, 122 -> **123**) · **+0 stat NAMES** (all five
`ssl.*` already existed; what changed is WHICH shapes register them — a `+0, UNCHANGED` ledger chain
entry, quoted as a DELTA with NO absolute) · **+0 BackendKinds** · **+0 fuzzers** (56 / 48) ·
**+0 go.mod modules** · **+2 Go packages** (the fixture's `driver` and `pki/gen`) · ONE ADR
(**ADR-0318**, COMPLETED IN PLACE).

**Net change vs master, measured by `--numstat` — never `--stat`, which is a SUM:**

| accounting | PLAN §1.2 estimate | **MEASURED at this close** | phase-94 precedent `0a985a35` |
|---|---|---|---|
| `.go` only, additions | ≈ +942 | **+1285** (net +1254) | +1143 |
| code + YAML + PEM + `expectations.yaml` | ≈ +1350 | **+1725** | +1635 |
| everything, additions | — | **+2069** | — |

🔴 **THE PLAN'S FIRST AND HEAVIEST SPLIT-GATE GROUND IS REFUTED BY THIS MEASUREMENT.** `PLAN.md` §1.2
decided DO NOT SPLIT on four grounds, the first being *"on the SAME accounting as the measured
structural precedent, this row is SMALLER than one that was not split"* — ≈+1350 against `0a985a35`'s
+1635. **Measured, it is LARGER: +1725 against +1635, and +1285 `.go` against +1143.** The row exceeds
the precedent on BOTH readings, so that ground does not hold and is withdrawn rather than reinterpreted.
[[reference_measured_prototype_is_a_lower_bound]] fires for the FIFTEENTH consecutive row, and this time
it inverted a comparison rather than merely widening a range.

**What survives, and why no retroactive split was taken.** BOOTSTRAP §6.1's numeric trigger is *~1500
lines of code*; on the `.go` accounting this row reads **1285 additions / 1254 net** and does not trip
it. §6.1's MID-EXECUTION trigger — *any single task's sub-steps blowing past ~10* — did not fire either:
no task exceeded nine, including Tasks 2 and 10, which §1.2 named as the likeliest. Grounds 2, 3 and 4
are untouched by this measurement: the production edit really is ONE LINE (`+1 / -1`, measured); the
only clean seam would have shipped a remotely-triggerable process-crash fix with NO cross-side evidence;
and 19 tasks is under ~25. **The verdict stands on those three, not on ground 1** — stated so a reviewer
can weigh the real basis rather than the one the PLAN led with.
