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
