# PROGRESS 80 — stats-sds-projection

*Stage-by-stage execution record. The PLAN half lands at the PLAN; the IMPL half is appended at the IMPL.*

---

# PLAN record (2026-07-30)

**Stage:** PLAN (lifecycle-state `2` → 3). **Row 80 STAYS `in-progress`**; `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED**; sentinel `want` STAYS **112**. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master **`47b9b378`** — ⚠️ **NOT the `f2dd994a` the router names.** `47b9b378` is the router's own roll commit on top of `f2dd994a`; branching off `f2dd994a` would have silently discarded it. Confirmed `git merge-base --is-ancestor f2dd994a HEAD` ⇒ true and `git diff --name-only f2dd994a..47b9b378` ⇒ **`next-prompt.txt` only**, so every doc count in the router survives. Worktree `/home/esa/git/envoy-go-wt/phase80-plan`, branch `phase80-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + **`STATE_HISTORY.md`** + `next-prompt.txt` — **five files**, matching the phase-79 PLAN precedent for the same reason (§Recent at its five-entry cap with an unarchived evictee).

## What was EXECUTED at this stage

Four probe agents on disjoint remits, each in its own **detached** worktree with private scratch and a private port band in **42000-42299** — clear of **both** reserved bands (`20000-31007`, and `11000-14999` new at `f2dd994a`) and the static fixture ports — plus controller re-derivation.

| probe | remit | headline |
|---|---|---|
| **P1** | cost / LoC / calibration | the `12-15` band and the `855 × 1.75` arithmetic |
| **P2** | fixture `0110`, **against the live reference container** | §7.2's own verdict is **RED on 3 of 5 names** |
| **P3** | the validation leg, **end-to-end with a real binary + SDS server** | the reject's site is ambiguous and the two positions differ observably |
| **P4** | bookkeeping re-derivation | §8's ADR block is stale by the SPEC's own commit |

**Zero commits, zero pushes by any agent.** Every experimental edit reverted **by explicit path**, sha256 byte-identity verified **4/4 files**, and independently re-confirmed by the controller (`git diff --stat HEAD` empty on all four worktrees). No unscoped `git restore`. P3 wrote **zero files into its worktree** — both probe programs injected via `go build -overlay`. No docker container was created by any probe outside P2's harness-owned testcontainers lifecycle; `docker ps -a --filter name=p80p2` empty; **no image-filter sweep was ever run.**

## ⚠️ THE HEADLINE: SPEC §7.2's OWN VERDICT SHIPS A RED FIXTURE

§7.2 exists to stop a planner reading "parity" as set- or value-equality. **Its replacement verdict reproduces the same failure one layer down.** It prescribes values *"per-side only (`>= 1`)"*. Measured against the live reference and a real subject boot, **4/4 consecutive runs, identical every time**:

| name | ref | subj | blanket `>= 1` |
|---|---|---|---|
| `update_attempt` | 3 | 1 | ok |
| `update_success` | 1 | 1 | ok |
| `update_failure` | 1 | **0** | **RED** |
| `update_rejected` | 0 | **0** | **RED** |
| `init_fetch_timeout` | 0 | **0** | **RED** |

⇒ **three of five names fail.** The repair is a **split roster**: `sdsProjectedNames` (5, name + hoisted label) and `sdsMovedNames` (2, value floor). Both **derived from measurement**, never from the suffix list.

**And §7.2's supporting enumeration is wrong in both directions:** `update_rejected` is *also* safely `0 == 0` (unnamed), `update_success` does **not** carry `update_attempt`'s hazard (1 == 1), and `update_failure` is a **genuine cross-side divergence (1 vs 0) the SPEC never names.** Reference `update_attempt` is **3, not 2**. The `sds.*` roster is **14 flat / 12 prom, not 10 / 9** — and the SPEC's own list of nine out-of-scope extras plus the in-scope five *entails* 14, so the figure contradicts the list printed beside it.

⚠️ **The design was proven live, red-on-arrival, and DISCRIMINATING:** the real T9 code, run at this stage with row 80 unlanded, produced **exactly 5 errors, all `subj:`, ZERO `ref:`**. A broken reference arm would have printed 10.

## ⚠️ THE SECOND HEADLINE: `0110`'s OWN DOCUMENTS FORBID WHAT THIS ROW MUST DO

`0110/expectations.yaml:210-215` and `driver/driver.go:643-646` carry a standing prohibition the SPEC never mentions — the assertion *"must not reach into the `sds.*` … scopes"* because `DriveSubject` hard-stops both SDS receivers, leaving them *"inherently unstable."*

**Measured verdict: the mechanism is real, the consequence is misattributed.** The hard-stopped receiver *is* why the reference reads `attempt=3 / failure=1` instead of `1/0` — but those values were **stable 4/4**. **The instability lands on VALUES, not on names or labels.** ⇒ the name+label-only posture is **what makes the assertion legal at all**, and the row owes a **prose reversal** in `0110/expectations.yaml:210`, `0110/README.md:178` and `0111`'s pair. ⚠️ **Nothing parses `0110`'s `expectations.yaml`, so omitting the flip is SILENT** and the fixture would ship contradicting its own recorded boundary.

⚠️ **A latent flake named now:** the reference's `3/1` depends on retry timing after receiver close; a slower machine could read `4/1` or `2/0`. **A value pin on the REFERENCE side is a flake by construction.**

## ⚠️ THE COST: **11-14, BUDGET 13** — and §9.3's LoC arithmetic MIXES TWO BASES

**Enumerated 13, not 14.** §9's *"T5 folds into T4"* is **not a fold — it is a forced atomic merge.** With an `sds.` arm + **one** roster entry applied, **all four goldens fail**: the tag literal is hard-coded at **four** sites (`:167 :208 :253 :354`) and `goldenName` is a **positional** struct literal, so a per-entry tag field rewrites all 14 roster lines. **T5 was never independent, and T4 must NOT split** (any sub-split leaves the tree red between legs), which refutes §9's ceiling mechanism too. Ceiling **14** rests on **T9** splitting — which the SPEC never nominates.

**§9.3's multipliers are on a PLAN-estimate basis and are applied to a SPEC estimate.** Case-sensitive `LoC`: **phases 77 and 78 carry NO SPEC LoC estimate at all** (0 hits in both SPECs; the figures exist only in their PLANs). So `2.040×` and `1.500×` are *realized ÷ PLAN* — they **reproduce to the byte** on that basis (1428/700, 495/330, 1532/875 = 1.75086) and cannot be SPEC-basis ratios. The only SPEC→realized datapoint is phase 79: ~500 → 1532 = **×3.06**, n=1.

⇒ **§6.1 gates the PLAN's own estimate of net change, not estimate × realized-multiplier.** *"855 × 1.75 ≈ 1496 ⇒ ON the ~1500 line"* is arithmetic on mismatched bases. **Both nearest comparables realized net 1406 and 1446 — UNDER the line, without splitting.**

**LoC bottom-up: ~700 `.go` insertions (~640 net)** — production ~86 · test ~457 · fixture ~154. ⚠️ **The SPEC's ~855 total is roughly right and EVERY BUCKET IS WRONG; the errors cancel.** Its production **55** omits T5's reject entirely; its test **690** over-projects from a non-analogous base (phase 79's 1208 test lines were **89% NEW FILES**, whereas **phase 80 creates zero new test files**); its fixture **110** understates (phase 79 realized **195** for the same species of conversion, and P2's executed T9 measured **183 added / 132 code**).

**NO SPLIT: 13 ≪ ~25, ~640 net ≪ ~1500.** ⚠️ **And the SPEC's nominated T4/T10 seam is incoherent** — `internal/statssink` and `test/fixtures/0110` share no file, package or dependency, so it is two unrelated leaves rather than §6.2's *"coherent slice."* The natural seam is **projection leg (T1-T4, T9) / validation leg (T5-T8)**. ⚠️ **The third §6.1 trigger (>~10 sub-steps) IS live on the merged T4** and is written into T4's own gate.

**Calibration 4/4 confirmed, none interpolated:** 76 `~7-9` → **9** (AT) · 77 `11-13` → **12** (INSIDE) · 78 `7–9` → **10** (**ABOVE**) · 79 `10-12` → **12** (AT). Three of four at or above ceiling ⇒ read 13-in-11-14 as **"expect 14."** All four PLAN squashes carry **0 `.go` files** (4/4).

## ⚠️ THE REJECT: THE SITE IS AMBIGUOUS BY ONE STATEMENT, AND THE POSITIONS DIFFER OBSERVABLY

§4 says *"immediately after `ParseSDSConfig` returns **and** before `RegisterSDSStats`."* Those clauses span **`boot.go:196`** and **`:200`**, with **`grpcclient.NewSDSClient` at `:196-199` between them.**

| placement | stderr | message |
|---|---|---|
| **before the dial** (`:196`) | **179 B** | `xds: sds: invalid secret name: "server-cert" (…)` |
| after the dial (= baseline) | **128 B** | `dial cluster "no_such_cluster": … unknown cluster` |

⇒ **the dial error MASKS the name reject**, and §2.5's own NC already showed an SDS-less boot dying at `connection refused` (332 B). A reject satisfying the SPEC's literal wording could sit where **G4's positive arm would need a live SDS server to observe it.** **The line is PINNED at `boot.go:196`.** Found independently by the controller (from call order) and by P3 (by measurement).

**End-to-end, executed with a real binary and a real SDS server:** cert-SDS `server-cert` ⇒ exit **1**, **0 B** stdout, **179 B** stderr, exact string · segment `trailing_dot.` ⇒ **181 B** · **VC-SDS** `rccf-validation-ca` ⇒ **186 B** (⚠️ **the shape 4 of 5 fixtures use** — the SPEC only reasoned about cert-SDS) · READY control ⇒ **63 B** `envoy-go ready`, 0 B stderr, 5 counters · **failing baseline** (no reject) ⇒ 332 B **dial** error, **not rejected today**. Every arm discriminated on **OUTPUT**; `timeout 124` never occurred.

## ⚠️ FOUR MORE VALIDATION-LEG DEFECTS

1. **§4.2's ADR-0065(b) table contains a cell that asserts the OPPOSITE of what it proves.** It reads `server-cert : all five suffixes agree = false`. **All five are `false`, therefore they AGREE.** Exhaustive: **0 disagreements over 95 single-byte and 9025 two-byte names.** The conclusion is right and now exhaustively proven; the cited proof row states the contrary.
2. **§4.2's segment predicate is INCOMPLETE as worded** — *"no leading dot, no trailing dot, no `..`"* on the *secret* passes `""`, assembling to `sds..init_fetch_timeout`, which **`IsValidName` ACCEPTS**. Not exploitable (`ParseSDSConfig` rejects `""` first at `internal/xds/config.go:28-30`) **but a unit test of the predicate in isolation would certify an incomplete guard.** ⇒ guard the segments on the **ASSEMBLED** name.
3. **§4.1's package-edge NC does not exist.** `internal/conv` has **0 tracked files**; `go list` exits **1** with `directory not found`. **The sentence claiming the NC was re-run is where it fires** — twice in one paragraph. A real NC (`./internal/clock`: exit 0, 47 deps, 0 hits) discriminates.
4. **§4.1 checks the WRONG package's edge.** The reject lands in `internal/boot`, yet the justification cites `internal/xds`. Re-run correctly: `./internal/boot` ⇒ 554 deps, **1** `internal/stats` hit plus a **direct** import at `boot.go:33` ⇒ **zero import changes needed**, measured.

**The "no-op on the corpus" claim SURVIVES — from a wrong denominator.** Denominators actually scanned: **120** fixture dirs · **688** tracked files · **249** yaml/json · **179** driver `.go` · `test/conformance` **40** files with **0** `sds` hits · **7** `testdata` dirs with 0 secret names · **0** static `secrets:` blocks. ⚠️ **FIVE fixtures carry `sds_secret_config`, not four** (`0108` and `0109` **share** `validation_ca`) ⇒ the footprint is **5 × 5 = 25 registrations, not 20.** All four names pass both legs. Predicate NC **8/8**, neither all-accept nor all-reject.

**The reject's OWN blast radius, never measured before: ZERO.** `go test -count=1` over **124 packages** ⇒ exit **0**. ⚠️ **§5.4's "ONE failing test" is the PROJECTION arm over two package trees — a different measurement. Do not conflate them.**

## ⚠️ THE `79.1` REPAIR: SPEC §8 NAMES THREE PHRASES FOR FOUR OCCURRENCES

The count *"4 occurrences on 3 lines, all in `BEHAVIOR_CONTRACT.md`"* is **right**; the three anchors cover only `:5078`×2 and `:5080`. The lines are **`:158`, `:5078`(×2), `:5080`**, and the fourth is **unnamed**:

> `:158` — *"That asymmetry, not scope, is **why the `sds.` family is deferred to row 79.1**"*

**A phrase-anchored sweep trusting the SPEC's list leaves `:158` stale.** Confirmed independently by the controller. **The most actionable defect this stage found.**

## ⚠️ TWO SWEEP CELLS ARE HISTORICAL, AND TWO TRAPS WOULD SILENTLY DEFEAT THE REST

- **§6.1 item 9 (`segmentcount_test.go:21`) and item 13 (`STATE.md:48`) are HISTORICAL and must NOT be edited.** Both are past-tense statements that twelve *were* live during phase 79; editing either falsifies the record, and `STATE.md:48` sits in a §Recent entry that migrates **VERBATIM** at eviction. ⚠️ `segmentcount_test.go` hard-codes **no numeral at all**.
- ⚠️ **CASE: four of the six live `0118` hits are UPPERCASE `TWELVE`.** A case-sensitive sweep finds **2 of 6**. **The sweep MUST be `-i`.**
- ⚠️ **`BEHAVIOR_CONTRACT.md:167` carries TWO `twelve` with OPPOSITE verdicts** (one historical, one live). **A line-level substitution corrupts one.** Verified: `grep -oi twelve | wc -l` ⇒ **2**.

**The live-normative set is 16 occurrences / 10 files**, not §6.1's *"16 lines / 8 files"* — whose own items 8-14 sum to **13 lines / 9 files**, inconsistent with itself either way. ⚠️ **The `0118` SIX are confirmed exactly** (README ×2, driver ×2, expectations ×2; `envoy.yaml` ⇒ **0**) — **the fixture is NOT de-fanged.** ⚠️ **§9 and §11 both cite "§6.1 item 15"; §6.1 enumerates items 1-14.** A phantom item, cited twice.

## ⚠️ THREE ROSTERS STALE BY THE SPEC'S OWN COMMIT — and a wrong figure already inside a landed ADR

| figure | SPEC | @ `53855de0` | @ `47b9b378` |
|---|---|---|---|
| numeral needle | 4 / 4 | **4 / 4** ✅ | **6 / 6** |
| scoped spelled needle | 26 lines / **20 files** | **26 / 18** — ⚠️ **file count wrong at its own base** | **33 / 19** |
| `79.1` roster | 50 / 9 | **50 / 9** ✅ | **61 / 10** |
| ADR block (§8 item 7) | 300, tail **ADR-0301**, `^## ADR-0302`⇒0 | — | **301, ids 0001-0302, tail ADR-0302 PROPOSED, `^## ADR-0303`⇒0, NC `^## ADR-0302`⇒1** |

**`7d014546` — the SPEC's own commit — added ADR-0302.** §8 item 7 is stale by the commit carrying it, exactly as §2.9 is. ⚠️ **§2.9's *"~11 in this row's own documents"* is now 29.**

⚠️ **The spelled-needle noise figure is 43 files (42 wrapped; 42 even at the SPEC's base) — NEVER 45 — and the wrong figure is already at `DECISIONS.md:17686`, ADR-0302 §Context ¶11.** It is **landable at the IMPL** precisely because that ADR's STATUS is still `PROPOSED`.

## FINDINGS NO PHASE-80 DOCUMENT CARRIES

1. **`internal/stats/name.go:490` reads *"Of the 25 entries"*** — `helpText` goes 25 → 30 at T3, so **this is a live stale count**, flagged by neither the SPEC nor any probe roster. It also carries a pre-existing unrelated **`13`** that a post-move numeral grep will collide with.
2. **§5.2's no-self-equal extension costs ZERO lines** — `TestHelpText_NoSelfEqualHelp` already iterates `helpTextRoster` and drives the real `WriteProm`. One of T3's three named components is free.
3. **T9's import delta is +0** — `bytes`, `fmt`, `math`, `net/http`, `strconv`, `strings` are all already imported by the `0110` driver.
4. **`0111` is precedent for the metric NAME being cross-side stable under hoisting, NOT for asserting a label VALUE** — its `scrapeProm` discards labels. **No in-tree fixture has ever asserted a hoisted label value; T9 is the first.** The only label-aware parser in the harness (`0005-prometheus-stats/driver`) is **unexported in another package**.
5. **Broken-gate shape EIGHTEEN** (below).

## Broken-gate shape **EIGHTEEN** — a negative control pointed at a target that DOES NOT EXIST

`go list -deps ./internal/conv` **exits 1** with `directory not found` and prints nothing — and the resulting **`0` hits reads exactly like "the NC discriminates."** **Distinct from all seventeen priors: the command is right, the assertion is right, and the NC's SUBJECT is a phantom.** ⚠️ **It fired TWICE inside SPEC §4.1** — the original NC was vacuous, and the sentence announcing the re-run introduced a second vacuous one — and a probe agent nearly re-inherited it here. **Remedy: an NC must assert its own target EXISTS and its command EXITED ZERO before its empty output is read as a result.** `reference_empty_output_is_not_a_zero_result`, applied to the control rather than the measurement.

## Confirmed, so the IMPL can rely on it

D-SDS-1 in full and its reference pinning · the `0110`→`rccf_validation_ca` / `0111`→`edf_validation_ca` attribution (**re-derived independently by the controller** — BRAINSTORM §4.3 is right and the phase-79 PLAN's *"SWAPPED"* correction is the false claim) · §7.1's fixture table, **all five rows**; `0110`'s blank import present, **zero registration gates owed** · all four corpus secrets **dot-free** ⇒ **G6 mandatory** · §5.1 blind in **all four** cells, and extending the roster **un-blinds all four** (measured 1134→1212 · 1200→1320 · 1118→1184 · 1184→1292) · §5.4's one-failing-test result for the projection arm (`TestTerminalError_TopLevelCountMatchesCode`; `internal/statssink` **ok**) · §2.7's `95/64/31/partial=0` and the exact 31-byte rejected set, derived from `IsValidName` and never hand-asserted · §2.5 arm A2 (**115 B**) · §2.6's inversion · `RegisterSDSStats` **1** non-test call site, `ParseSDSConfig` **4** · ADR-0302's footer **byte-exact** (`od`-verified U+00A7; all 8 footers normalize to ONE string; **ADR-0294…0300 = SEVEN**, ADR-0301 = **0**) · ADR-0299's stale `PROPOSED` · ADR-0301's seven-vs-six miscount · every count in the router's roster.

## Counts re-derived at this tip, with negative controls

fixtures **120** (bare predicate **118**) · fuzzers **55** (⚠️ both scopes — NOT discriminating) · internal packages **73** · blank imports **120** (naive **126**; ⚠️ **`\t` in GNU ERE is a literal `t`** — use `-P`) · `BackendKind` tail **38** over **39** declarations, no `iota` · `DECISIONS.md` **17692** / **301** headings / tail **ADR-0302 PROPOSED** / next-free **ADR-0303**, ids 0001-0302, **one gap at ADR-0209**, zero duplicates · `ROADMAP.md` **228 / 112** · `BEHAVIOR_CONTRACT.md` **5822** · `STATE_HISTORY.md` **426** · port **10119** · detectors **12 code / 12 string, AGREE**; mid-name **4 / 4** · `helpText` **25** / roster **25** · `goldenRoster` **13** with **ZERO** `sds.` · OTLP **1134/1200/1118/1184** · stat surface **1207** (⚠️ DOCUMENTARY, assert the DELTA) · go.mod modules **2** (⚠️ the single `go.mod` requires **67**).

**`STATE.md` audit:** all seven §Current fields **singleton**, NC both ways (`- **zzz-nonexistent:` ⇒ 0; `- **prior active-phase:` ⇒ 5). **§Project SELF-CONTRADICTS §Current on three axes** (`:31` 119 vs 120, `:33` 1205 vs 1207, `:35` ADR-0298 vs ADR-0302) and is **frozen at the phase-76 IMPL close** — anchor on §Current. **Eviction target confirmed:** `phase 79 … BRAINSTORM done` (`:54`, 1722 B), **ABSENT from the archive (0) with a FIRING NC on the phase-78 sibling (1)**. **The phase-77 PLAN archive gap reproduces** (IMPL/SPEC/BRAINSTORM = 1 each, **PLAN = 0 in BOTH files**), non-vacuously — the archive carries PLAN entries for 78, 63, 62, 60.1, 56.1, 46.2, 42.2b, 42.2a, 42.1.

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

`ls stop` ⇒ `No such file or directory`. **It must not be created.**

- **(1)** `NOT DONE: row 80` at `want=112`. NCs, both fired: `want=111` ⇒ `GATE FAIL: examined 112 data rows, expected 111`; row **76** doctored ⇒ `NC NOT DONE: row 76` **alongside** row 80 (`rows doctored: 1`).
- **(2)** **FIVE — `:190 :200 :210 :216 :224`** — **UNCHANGED. The twenty-sixth consecutive phase at which it did not go down. STATED, not forecast.** ⚠️ One-arm strip moves **5 → 4, not 5 → 0**; independently re-confirmed that the long form does **not** contain the short substring (⇒ 0).
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NCs: invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`; the REGISTERED slug `Observability` printed **nothing**.
- Input measured **228 lines / 112 data rows / 13** bare `candidates:` hits (vs the sentinel's narrower 5).

⚠️ **`want` STAYS 112. The leak check is DORMANT this stage and re-arms at the IMPL** (T13.4).

## Hygiene

⚠️ **THE BASH CWD RESET IS ASSUMED LIVE — the twenty-third consecutive session.** Every git command used `git -C <abs-worktree-path>`; branch and commit-count tripwires run before any write.

⚠️ **A CONTROLLER PROBE DEFECT, CAUGHT AND RECORDED.** An early `ls -la "$S/p2/" | head` truncated a **12-file** listing at 8 entries, and I read the absence of `t10.go.frag` as the artifact not existing — i.e. **I nearly reported a probe's artifact claim as false on the strength of a truncated listing.** Caught by re-running with an explicit `find` and a stated denominator. **`head` is not a denominator** (`reference_empty_output_is_not_a_zero_result`). Separately, a `go list … | grep -c; echo $?` in my own hand reported **`grep`'s** status, not `go list`'s — `reference_harness_exit_code_is_not_command_exit_code` firing on the controller inside the stage that lists it as constraint 8.

**Probe self-reported defects, recorded rather than laundered:** P1's phase-76 NC was **case-insensitive** and matched `local`/`clock`/`allocates`, returning 15/16 and reading as a refutation until re-run case-sensitively (0/0) · P1 first assumed T5 was free because the goldens build `want` programmatically — true, but the tag literal is hard-coded alongside, and **only running the experiment surfaced it** · P2 nearly reported §7.2's verdict as sound from source reading alone, and only per-side value dumps exposed the three zeros · P2's first `sds` line filter would have over-counted (`sds_cluster`, `ssl_context_update_by_sds`) · P3's first boot arm used a nonexistent flag and returned exit **2** with a usage dump, caught by reading stderr **content** · P3 flagged a 0.228 s green as implausible and made it falsifiable with a `RUN/PASS/SKIP/FAIL` census · P4's `^\t_ ` under `-E` returned **0**, impossible against a 3473-line file, and its blindness probe's "blind" arm was **`command grep`** (i.e. not the blind tool), which **cannot fire** — caught by set-differencing the rosters.

**Not run at this stage, flagged rather than silently dropped:** the full 120-fixture differential and `-race` (**the PLAN owes none of it; T12 runs it at the IMPL**) · h2spec · any two-secret shape (unconstructible — §2.5's hard boot reject) · the subject side's **hoisted label key** post-landing (asserted against the **reference** only; if envoy-go hoists under a different key, T9 fails on the label check, not the name check) · a flake study of the reference's `3/1` values (4 runs is not one).

**No flake fired at this stage.**

---

# IMPL record (2026-07-30)

**Stage:** IMPL (lifecycle-state `3` → DONE). **Row 80 flips `in-progress` → `done`.** Sentinel `want` STAYS **112** — this row adds no row. Base master **`28aa342d`** (the PLAN squash tip, located by SUBJECT via `git log --grep 'phase 80'`, never by position). Worktree `/home/esa/git/envoy-go-wt/phase80-impl`, branch `phase80-impl`.

**Eleven task commits**, squashed at close: T1 `16cf67e9` · T3 `3117c3ef` · T5 `0352b45e` · T4 `b30f933c` · T6 `c40198bc` · T7 `21598400` · T8 `de4f7566` · T2 `a412ab38` · T9 `7016bf65` · T10 `c38baa3b` · T13 `4f5b7944`. Seven subagents, all committing **locally only**; controller squash-pushed. Two ran in parallel on disjoint packages (`internal/stats` vs `internal/boot`+`internal/xds`), two more in parallel on disjoint trees (the `TWELVE` sweep vs fixture `0110`/`0111`), and the break agent ran alongside the doc agent — every one staging by **explicit path**, none ever running an unscoped `git restore`.

## ⚠️ THE HEADLINE: THE PLAN'S OWN SPLIT ARITHMETIC IS THE THING THIS STAGE REFUTED

PLAN §4 states **`~640 net .go < ~1500 (~2.3× margin)`**. **Realized: 1701 insertions / 65 deletions = 1636 net `.go`** — **over the ~1500 trigger by 136 lines.** Unlike the SPEC's estimate (which PLAN §1.3 correctly diagnosed as wrong per-bucket with *cancelling* errors), here **every bucket over-ran and the errors COMPOUNDED**:

| bucket | PLAN estimate (ins) | realized ins | ratio |
|---|---|---|---|
| production | ~86 | **135** | 1.6× |
| test | ~457 | **935** | 2.0× |
| fixture driver | ~154 | **631** | **4.1×** |
| **`.go` total** | **~700 (~640 net)** | **1701 (1636 net)** | **2.4× / 2.6×** |

**The dominant miss is the fixture bucket**, and its cause is a conditional the PLAN priced at zero: *"plus `0111`'s parallel pair **if mirrored**"*. It **was** mirrored — at **320 insertions, more than `0110`'s 302** — and mirroring turned out to be load-bearing, not optional (see the counter-classification finding below).

⚠️ **THE SPLIT DID NOT RETROACTIVELY FIRE, AND THAT IS THE CORRECT READING.** `BOOTSTRAP_PROMPT.md` §6.1 (`:290`) gates *"`PLAN.md` **estimates** exceed ~1500 lines of code of net change"* — the number **the PLAN writes down**. The PLAN estimated ~640 across **13 tasks** (≪ ~25), so neither trigger was live at authoring time. **But 1636 is the lineage's first realized crossing** (comparables: 1406, 1446, phase 79's 1532), and it is recorded rather than absorbed.

⚠️ **THE THIRD §6.1 TRIGGER DID FIRE, ON T4, EXACTLY WHERE THE PLAN SAID TO WATCH FOR IT.** T4 enumerated **6** sub-steps against a ~10 threshold and executed **17** — ~2.8× the enumeration, ~1.7× the threshold. It stayed atomic (one commit, the tree never red between legs), which is why it was right not to split it, but the estimate was low.

## Refutation ledger — what EXECUTION found that the PLAN got wrong

**Load-bearing (changed what shipped):**

1. **⚠️ A SECOND CROSS-SIDE DEPARTURE, NAMED BY NO PHASE-80 DOCUMENT.** Fixture `0111` serves an **EMPTY** `validation_context`. **The reference ACKs it and books `sds.<secret>.update_success`; envoy-go REJECTS it and books `update_rejected`**, then falls back to the inline default exactly as the byte observable requires. **The two sides book the same event under OPPOSITE counter names** (ref `3/1/1/0/0` vs subj `1/0/0/1/0`). ⇒ **mirroring `0110`'s two-name floor set into `0111` is RED ON ARRIVAL**; `0111` floors `update_attempt` **alone**. This is `reference_sds_empty_ack_narrow_classifier` observed for the first time **through the stats surface**. Recorded as a named, unasserted divergence in `BEHAVIOR_CONTRACT.md` — and it is a **fourth independent argument** against cross-side value assertions.
2. **The G6 code block does not compile.** The PLAN prints keyed rows including `wantErr: true` for `TestExtractTags`' table — which is a **positional** three-field literal with **no `wantErr` field**. Two compile errors at once. G6 was given its own function with its own table type. `reference_plan_break_instructions_dont_compile`, fired on a *task* block rather than a break block.
3. **`t.Fatalf` was already live in the table the PLAN sends rows to.** Appending under it would have made the trailing negative assertion dead code — the exact hazard §2 constraint 3 names. Converted to `Errorf` + `continue`. **The PLAN's instruction, followed literally, would have SHIPPED the anti-pattern it forbids.**
4. **The predicate's fork-adjacent rationale is wrong in both cells.** PLAN T6: *"`1leading_digit` and `trailing_dot.` … **both are valid** as bare names."* Measured: `IsValidName("1leading_digit") = false` **and** `IsValidName("trailing_dot.") = false` — **both INVALID**. The SPEC's verdict was corrected in the *wrong direction*. The *expectations* are unaffected (`sds.` supplies the leading char; the segment leg catches the trailing dot) but the stated reason is false. Each case now pins `wantBareValid` so this cannot recur silently. `reference_probe_input_is_a_claim`.

**Bookkeeping / gate-shape (recorded, did not change what shipped):**

5. **⚠️ *"It is never 45"* IS ITSELF FALSE.** PLAN §1.12 asserts the spelled-needle file figure is **43** and *"never 45"*. Measured across the lineage: **42** @`53855de0` · **43** @`7d014546` and @`47b9b378` · **45** @`28aa342d` — **the PLAN's own squash added `PLAN.md`+`PROGRESS.md` and moved 43→45**, so ADR-0302 ¶11's *"forty-five"* was **correct at the tip the PLAN was measuring from**. It is **38** after T2's sweep and **39** at HEAD. ADR-0302 ¶11 now carries **39 with its tip named**, recorded as a **moving measurement, not a constant.** `reference_a_drift_correction_is_itself_a_claim` — a drift correction is itself a claim, and this one was wrong.
6. **Break arm γ's "three tests" is false, for an instructive reason.** With the arm deleted and the constant at 13, `TestExtractTagsTerminalError_ByteStable` **PASSES** — its golden is **hand-written and was bumped in lockstep with the constant**. It only fires once the two desync. **`reference_handwritten_golden_shares_author_mistake`, firing on the very guard whose own file header describes that hazard.** What actually caught the defect is `segmentcount_test.go`, which hard-codes nothing. Arm δ has the mirror-image error: reverting the constant alone fires **two** tests, not one.
7. **Break arm λ is understated.** PLAN §1.1 and the λ row predict *"reds on THREE names on the SUBJECT side."* Executed, stable over 3 runs: **5 errors — 3 subject AND 2 reference.** `update_rejected` and `init_fetch_timeout` are **0 on BOTH sides**, which §1.1's own table shows in its ref column while the prose marks those rows RED only in the subject column.
8. **⚠️ A SECOND κ DATAPOINT, FOUND BY ACCIDENT AND WORSE THAN THE FIRST.** Broken-gate shape **seventeen** was confirmed live (roster un-extended ⇒ **1134 bytes in both worlds**, arm present or deleted, while the NC `1134→1135` fires — command, assertion and cross-product all correct, the *input roster* the entire defect). **But under arm α, with the roster FULLY EXTENDED, the `F_F` cell STILL passed** while the other three failed. ⇒ **extending the roster is necessary but not sufficient — a `(F,F)`-only cell restriction remains fatal even after the roster fix.** The κ hazard survives its own remedy.
9. **Break arm η's injection site is NOT a live variable.** Both post-dial positions (after `NewSDSClient`, and after `RegisterSDSStats`) produce **byte-identical** failures. §1.5's masking claim is confirmed; the sub-choice is unobservable. Note that at site 2 the invalid name reaches `RegisterSDSStats` and **nothing in `internal/xds` fires** — the skip-branch layer is blind to it, as §4.3's unreachability argument implies.
10. **Arms α, θ and ι each fire MORE than the PLAN names** — α adds `TestHelpText_KeySetExact` and both terminal-error guards (5 tests in `internal/stats`, not 3); θ adds `TestValidateSDSSecretName_PrintableByteSweep` (`.` crosses from the rejected to the accepted byte set — a genuinely independent second catcher); ι adds `TestGolden_RosterTagDeclarationsAgree` **first and most diagnostically**, and within OTLP only the two `emitTagsAsAttributes=true` cells fire.
11. **Line anchors drifted mid-row, as predicted and then observed.** `golden_bytemirror_test.go:354` is a closing brace (the literal is at `:351-352`). `name.go:490` is **`:517`** after T1 landed above it (`helpText` `:540`, not `:513`). The SN9-collision cite `:501` is at **`:555`** — and `:501` at this tip *is* a real `wasm.` line, so a line-anchored reader lands on a **plausible-looking wrong target**. `STATE.md:48` is `:50`. `next-prompt.txt:150` is a **phantom cite** (the file carries eight `twelve`, none on `:150`). **Every phrase anchor held; every line anchor that mattered had moved.**
12. **`git grep -c … ⇒ 0` is a broken gate as literally specified.** With zero matching lines `git grep -c` **prints nothing and exits 1** — it never prints `0`. And `-c` counts **LINES, not occurrences**: pre-edit it reads **3** against **4** live `79.1` occurrences, so it structurally cannot distinguish "3 of 4 repaired" from "4 of 4". The discriminating measure is `grep -o … | wc -l` plus per-phrase probes.
13. **The `internal/boot` census is 23, not the PLAN's 21** — 23 at base, before any edit; 30 after T6+T8. **The reject added zero tests, so 21 was never right at this tip.**
14. **PLAN T10 says "the NEW DEPARTURE", singular** (there are two), **misses a paragraph that goes FALSE** the moment the `sds.`-deferral sentence is closed (the phase-79 resolution paragraph two lines above still read *"STILL OPEN, NARROWED, for `sds.`"*), and **omits the `# HELP` section** the row moves 25→30.
15. **T12's touched-package roster says six; it is EIGHT** — T2's sweep modified `0118`'s driver and T9 mirrored to `0111`'s. Both were linted.
16. **Two claims held exactly and are recorded so no later stage re-derives them:** T7's sweep reproduced **95 / 64 / 31 / 0** and the 31-byte rejected set **byte-identical**, with guard-reject ⊇ skipAll at exactly one mismatch (`"."`, guard stricter); and the four OTLP `wantBytes` re-measured to **1212 / 1320 / 1184 / 1292**, matching the PLAN — **measured, not inherited**, and they coincide only because the roster entry is byte-identical and `goldenOTLPVersion` is unchanged. **The PLAN's caution that they are tree-local remains correct in principle; it simply did not bite here.**

## Gates — ACTUAL output

| gate | result |
|---|---|
| **full 120-fixture differential** `-count=1 -v` | **INNER_EXIT=0**, **120 `--- PASS`**, FAIL/SKIP **EMPTY**, `no driver registered` **0**, **`comm -3` EMPTY** (ran=120, dirs=120), **402 s** — 33 % of CI's `-timeout 20m` |
| **h2spec** (the FOURTH `cmd/envoy-go` consumer, run explicitly) | INNER_EXIT=0, `53 tests, 53 passed, 0 skipped, 0 failed` across 18 sections |
| `go test ./internal/... -count=1` | INNER_EXIT=0, 70 `ok`, 0 FAIL |
| `-race` over the four touched trees (a SECOND run) | INNER_EXIT=0, **`DATA RACE` count 0** |
| `go vet ./...` | 0, **0 bytes** |
| `gofmt -l` over **eight** packages | **OUTPUT EMPTY** (gated on output — it never exits non-zero; NC on a doctored file **printed the name AND still exited 0**) |
| `golangci-lint run` over eight packages | **own exit 0**, 0 bytes; NC with `-E godot` ⇒ exit 1 with a real finding, **proving the loader reaches the packages** |
| `git diff … -- go.mod go.sum` | **0 bytes** |
| **stat-surface delta** | **+0, PROVEN BY ENUMERATION.** Added registration call sites in the diff: **2, both in a `_test.go`**. **Production set = ∅.** Matcher non-vacuity: the same regex tree-wide = **508 call sites / 84 files**. The five `TestNoNewStat*` guards were re-confirmed **blind** (all in `internal/statssink/registration_test.go`) and **did not discharge this** |

**Zero hazards fired.** `address already in use` = **0** in the 161 KB differential log (**both** bands clean — the `f2dd994a` backend banding held), `subject ready: EOF` = **0**, `0061-lb-ring-hash` PASS, `internal/cluster` / `internal/httpclient` / `internal/filter/hcm/h2` all `ok` plain and under `-race`. **`reference_sds_init_fetch_timeout_dial_budget_flake` did not fire in any run** despite being live for this row's subject. **No isolate-re-run was needed.**

## Sentinel — re-run MECHANICALLY after the ROADMAP flip. It does NOT fire

| check | ACTUAL output | NC, observed FIRING |
|---|---|---|
| **(1)** `want=112` | **NOTHING** — row 80 is now `done` | `want=111` ⇒ `GATE FAIL: examined 112 data rows, expected 111`; row 76 doctored ⇒ `NC NOT DONE: row 76` |
| **(2)** | **FIVE** — `:190 :200 :210 :216 :224` | union **5**, long-arm-only **4**, short-arm-only **1** ⇒ **5→4, NOT 5→0** |
| **(3)** | **`NEVER OPENED: gRPC`, `NEVER OPENED: WASM`** | invented slug prints; registered `Observability` correctly silent |

Input measured at **228 lines / 112 data rows / 13** bare `candidates:` hits. **(2) and (3) still print ⇒ the sentinel does NOT fire. `ls stop` ⇒ `No such file or directory`; it was NOT created.**
⚠️ **CHECK (2) IS UNCHANGED. THIS ROW NARROWS NOTHING — STATED, NOT FORECAST. The twenty-seventh consecutive phase at which it did not go down.**

**The leak check RE-ARMED and passed.** Row 80's cell: deferred-candidate phrases **0**; slugs present = **`Observability-family row` only**, registered **51×** elsewhere. ⚠️ **The by-mention silencing was reproduced LIVE**: on a doctored copy `gRPC` **stopped printing** while `WASM` kept printing, and check (2) went 5→6. **`grep` cannot tell a mention from a use.**
**Row-80 well-formedness**: the **disjunction** was required — over all 112 rows, ARM-A (`NF!=8`, escape-aware) catches 57/69 only and ARM-B (trailing-piece) catches 78 only. The **compensating-defect NC** (inject an inner pipe + strip the trailing one) makes naive `NF==8` **pass a malformed row**, exactly as on real row 78; ARM-B catches it.

## Bookkeeping

**ADR-0302** §Decision + §Consequences appended **IN PLACE** (ADR-0044-as-used, no renumber, no `---`), STATUS `PROPOSED` → `COMPLETE`, **footer RETAINED** at `:17692` (`od`-verified U+00A7). Per-block footer scan re-derived: **ADR-0294…0300 = SEVEN**, ADR-0301 = 0 (the recorded departure), ADR-0302 = 1 — **ADR-0301's own "seven blocks / ADR-0295 through ADR-0300" miscount was NOT copied.** No whole-file grep count carried into any ADR.
**The ADR-0299 rider landed**: STATUS `PROPOSED` → `COMPLETE`. The guard *"a `PROPOSED` STATUS must carry no §Decision"* over all 301 blocks named **ADR-0299 and no other** pre-edit and is **silent** post-edit, NC'd both ways. STATUS census **13 COMPLETE / 2 PROPOSED → 15 / 0**.
**`STATE.md`** §Current rolled **IN PLACE**; §Project **left alone** (frozen at the phase-76 IMPL close, self-contradicting §Current — repairing a count by editing the sentence that states it is how the ADR-0296/0297 species starts). All seven §Current fields verified singleton, NC both ways.
**Eviction**: `phase 79 (stats-prometheus-projection) SPEC done` — **re-derived as oldest at this tip**, verified **ABSENT** from the archive (0 hits / 428 lines) with a **firing NC** on the phase-79 BRAINSTORM sibling (1). Body migrated **VERBATIM**, `cmp` exit 0, with a firing `cmp` NC against a sibling body. **`STATE_HISTORY.md` 428 → 430.** The known `phase 77 PLAN done` gap re-checked and **not widened**.

## Six-gate (§7.5, `/BOOTSTRAP_PROMPT.md` at the REPO ROOT — `:357`, `:360-365`, `:367` re-verified exact)

(a) differential **GREEN, 120/120** · (b) corpus **120** dirs, same run · (c) h2spec **53/53**; proxy-wasm **INHERITED, not re-run — said so rather than claimed** · (d) fuzzers **VACUOUS AND RECORDED AS VACUOUS** (55 repo-wide, **0** added; matcher NC: 11 `+func Test` added) · (e) `gofmt`/`vet`/`golangci-lint`/`-race` all clean · (f) ⚠️ **FINDING — no `REVIEW.md` for phase 80, and none since 25.3; 84 of 121 phase dirs carry none. A standing lineage departure from §7.5(f), recorded as such rather than as compliance.**

## Broken-gate count stays **EIGHTEEN** — no new shape, but two priors fired live

No nineteenth shape was found. **Shape seventeen (a golden roster omitting the family under test) was reproduced live and then shown to survive its own remedy** (finding 8). **The hand-written-golden shape fired on a break arm's prediction** (finding 6). **`gofmt -l` printing while exiting 0** was observed directly in an NC. **A harness's exit code is not the command's** was defended against throughout by `INNER_EXIT` capture and did not fire.

## Carried forward, deliberately NOT fixed

`DECISIONS.md` ADR-0301 §Decision (b)'s *"has failed twelve top-level detectors"* — **present tense inside immutable ADR text, factually wrong as of `16cf67e9`; a named permanent discrepancy** · `ROADMAP.md`'s five `twelve` (§Schema-invariant-blocked; **four are pure false positives** and the fifth is historically true, so **nothing is owed there on the merits**) · the `wasm.` **SN9 collision** with ADR-0118 (which is why the `sds.` arm claims **no SN number at all**) · `BEHAVIOR_CONTRACT.md`'s `# HELP` departure, **widened** 25→30 and explicitly **not** parity work · full hyphen fidelity · the general dynamic-token charset exposure across six families (**this row does NOT audit whether the five existing rejects are complete**) · **`--mode validate` cannot validate ANY SDS bootstrap** (`validate.Bootstrap` passes a nil provider) · the `listener.`/`stat_prefix` sanitization inconsistency · the `STATE_HISTORY.md` archive gap · `ROADMAP.md`'s malformed rows 57/69/78 · the differential bind **retry** gap (the allocator half landed at `f2dd994a`) · the two `//nolint:gosec` directives this row added, **inert** because `gosec` is not among the 9 linters `.golangci.yml` enables.
⚠️ **A NEW NEEDLE COLLISION, named now:** `0110`'s driver comment *"envoy_sds_* metric names (twelve families…)"* is **accurate** (the reference exposes **14 distinct names / 12 families** — the `update_duration` histogram contributes `_sum`/`_bucket`/`_count`) but is a **false positive for any future `\btwelve\b` top-level-detector sweep.**
