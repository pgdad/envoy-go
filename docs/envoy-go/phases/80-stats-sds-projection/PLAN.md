# PLAN 80 — `sds.` label-hoisting + the boot-boundary reject: **the SPEC's own fixture verdict is RED ON ARRIVAL**, its LoC arithmetic mixes two bases, and the golden "fold" it offers is a FORCED MERGE

**Stage:** PLAN (lifecycle-state `2` → 3). **ROW 80 STAYS `in-progress`**; `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` are **BYTE-UNTOUCHED** at this stage. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master **`47b9b378`** — ⚠️ **NOT the `f2dd994a` the router names.** `47b9b378` is the router's own roll commit sitting on top of `f2dd994a`; branching off `f2dd994a` would silently discard it. `git merge-base --is-ancestor f2dd994a HEAD` ⇒ true, and `git diff --name-only f2dd994a..47b9b378` ⇒ **`next-prompt.txt` only**, so every doc count in the router survives. Worktree `/home/esa/git/envoy-go-wt/phase80-plan`, branch `phase80-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + **`STATE_HISTORY.md`** + `next-prompt.txt` — **five files**, matching the phase-79 PLAN precedent (which was itself one more than 76/77/78 for the same reason: §Recent is at its five-entry cap and the evicted bullet has never been archived).

**Method:** four probe agents on disjoint remits (P1 cost/LoC · P2 fixture `0110` executed against the live reference · P3 the validation leg end-to-end · P4 bookkeeping re-derivation), each in its own **detached** worktree with private scratch and a private port band in **42000-42299** — clear of **both** reserved bands (`20000-31007` subject blocks, `11000-14999` backends, new at `f2dd994a`) and the static fixture ports — plus controller re-derivation. **Zero commits, zero pushes by any agent**; every experimental edit reverted by explicit path and verified sha256 byte-identical (4/4 files, independently re-verified by the controller).

⚠️ **Every figure below was EXECUTED at this tip.** A SPEC cite is not evidence (`feedback_brief_citations_not_evidence`).

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

The lineage record is that **every stage finds a load-bearing error in its predecessor, and every one is found by EXECUTION rather than review.** The phase-80 SPEC refuted nine BRAINSTORM claims and four of its own probes. **This PLAN refutes or corrects nineteen SPEC claims** and adds five findings no phase-80 document carries. **Three change what the IMPL must build.**

### 1.1 ⚠️ HEADLINE ONE — SPEC §7.2's OWN VERDICT SHIPS A RED FIXTURE. Three of the five counters are ZERO on the subject

§7.2's VERDICT reads, verbatim: *"a cross-side assertion of exactly **5** names each carrying `envoy_xds_resource_name="<secret>"`, with values asserted **per-side only** (`>= 1`), not cross-side."*

**Executed against the live pinned reference and a real subject boot, 4/4 consecutive runs, identical every time:**

| name | ref | subj | blanket per-side `>= 1` |
|---|---|---|---|
| `update_attempt` | 3 | 1 | ok |
| `update_success` | 1 | 1 | ok |
| `update_failure` | 1 | **0** | **RED** |
| `update_rejected` | 0 | **0** | **RED** |
| `init_fetch_timeout` | 0 | **0** | **RED** |

⇒ **A planner implementing §7.2's sentence literally fails on three of five names.** This is precisely the failure mode §7.2 was written to prevent (*"BRAINSTORM §9 item 5 calls it a 'parity assertion expecting 5 names'. That phrasing would produce a RED fixture on arrival"*) — **reproduced one layer down, inside the correction.**

**The repair is a SPLIT ROSTER**, and T9 ships it as two named variables: `sdsProjectedNames` (**5**, name + hoisted label only) and `sdsMovedNames` (**2** — `update_attempt`, `update_success` — the only ones carrying a value floor). **Both derived from measurement, never from the suffix list.**

⚠️ **And §7.2's supporting enumeration is wrong in BOTH directions**: `update_rejected` is *also* safely `0 == 0` cross-side (unnamed by the SPEC), `update_success` does **not** carry `update_attempt`'s hazard (it is 1 == 1), and `update_failure` is a **genuine cross-side divergence (1 vs 0) the SPEC never names at all.** The conclusion (no value parity) survives; **the enumeration behind it must not be inherited.** Two further figures: the reference's `update_attempt` is **3, not 2**, and the `sds.*` roster is **14 flat / 12 prom, not "10 flat / 9 prom"** — the SPEC's own §7.2 list of nine out-of-scope extras plus the in-scope five *entails* 14, so the number contradicts the list printed beside it.

### 1.2 ⚠️ HEADLINE TWO — `0110`'s OWN DOCUMENTS FORBID WHAT THIS ROW MUST DO, and the prohibition is right about the mechanism and wrong about the consequence

`0110/expectations.yaml:210-215` and `0110/driver/driver.go:643-646` carry a standing prohibition the SPEC never mentions:

> *"The assertion is deliberately CONFINED to `listener.<addr>.ssl.*`. It must not reach into the `sds.*` or `cluster.sds_cluster.*` scopes: `DriveSubject` hard-stops both SDS receivers before step 10, so those scopes are reconnecting against a closed port while this runs and are **inherently unstable**."*

**Measured verdict: the mechanism is real, the consequence is misattributed.** The hard-stopped receiver *is* why the reference reads `update_attempt=3 / update_failure=1` instead of `1/0` — but those values were **stable 4/4 runs**. The instability lands on **values**, not on names or labels. ⇒ **§7.2's name+label-only posture is not merely preferable, it is what makes the assertion legal at all**, and this row owes a **prose reversal** in `0110/expectations.yaml:210`, `0110/README.md:178` and (if mirrored) `0111`'s parallel pair — **none machine-gated, so omitting them is SILENT** and the fixture would ship contradicting its own recorded boundary. §7.1 names none of them.

⚠️ **A latent flake, named now:** the reference's `3/1` depends on retry timing after receiver close. A slower machine could read `4/1` or `2/0`. **A value pin on the REFERENCE side is a flake by construction** — a third, independent argument for name+label-only.

### 1.3 ⚠️ HEADLINE THREE — §9.3's LoC ARITHMETIC MIXES TWO BASES, AND THE SPLIT DOES NOT FIRE

§9.3 computes *"855 × 1.75 ≈ 1496 and 855 × 2.04 ≈ 1744 ⇒ the row sits ON the ~1500 line at the central multiplier and OVER it at the ceiling."* **The multipliers and the estimate come from different bases.**

Measured, case-sensitive `LoC` (⚠️ case-**insensitive** matches `local`, `LocalAddress`, `clock`, `allocates` and returns 15/16 — a broken NC that reads as refuting the claim):

```
76 SPEC: 0   76 PLAN: 0
77 SPEC: 0   77 PLAN: 1     <- ~700 exists ONLY in the PLAN
78 SPEC: 0   78 PLAN: 1     <- ~330 exists ONLY in the PLAN
79 SPEC: 1   79 PLAN: 11
```

**Phases 77 and 78 carry NO SPEC LoC estimate at all**, so `2.040×` and `1.500×` are *realized ÷ **PLAN** estimate* and cannot be SPEC-basis ratios. Realized numerators re-derived from the IMPL squashes (`git show --numstat`, `.go` only): 77 `4d7f63c2` **1428**, 78 `3a4c8cfa` **495**, 79 `895f0be2` **1532** ⇒ 1428/700 = **2.0400**, 495/330 = **1.5000**, 1532/875 = **1.75086**. **The ratios reproduce to the byte — against PLAN estimates.** The only SPEC→realized datapoint in the window is phase 79: ~500 → 1532 = **×3.06**, n=1.

⇒ **§9.3 gates the wrong quantity.** `BOOTSTRAP_PROMPT.md` §6.1 gates *"`PLAN.md` **estimates** exceed ~1500 lines of code of **net change**"* — the number **this document writes down**, not an estimate multiplied by a realized ratio. Both nearest comparables realized *net* **1406** and **1446** — **under** the line, without splitting.

**VERDICT: the split does NOT fire.** §4 states the bottom-up: **~700 `.go` insertions (~640 net)**, 13 tasks ≪ ~25.

⚠️ **AND THE SPEC'S NOMINATED SEAM IS INCOHERENT.** §9.3 says *"split at the T4/T10 seam (goldens+fixture as leg b)"*. T4 is `internal/statssink`; T10 is `test/fixtures/0110`. **They share no file, no package and no dependency** — two unrelated leaves, not the *"coherent slice of the original"* §6.2 requires. Were a split ever needed the natural seam is the row's own §1 items 1 vs 2: **projection leg (T1-T4, T9) / validation leg (T5-T8)**, which are logically independent (the arm does not need the reject; the reject does not need the arm). **Recorded so no later stage re-derives it; not taken.**

⚠️ **The normative cite is confirmed exactly as the SPEC states it** — heading `### 6.1 When to split` at `:285`, *"~25 **numbered** tasks"* `:289`, *"~1500 **lines of code**"* `:290`, **`:291` BLANK**, the third (mid-execution, >~10 sub-steps) trigger at **`:292`**. One correction the SPEC does not make: **the file is at the REPO ROOT** (`/BOOTSTRAP_PROMPT.md`, 522 lines), not under `docs/envoy-go/`.

### 1.4 ⚠️ HEADLINE FOUR — "T5 folds into T4" IS NOT A FOLD. IT IS A FORCED ATOMIC MERGE, AND THE COUNT IS 13 NOT 14

§9's floor rests on *"T5 folds into T4"* and its ceiling on *"T4 splits."* **Both are refuted by the same experiment.** With an `sds.` arm + **one** `goldenRoster` entry + `goldenTaggedIdx[13]=true` applied:

```
--- FAIL: TestGolden_DogStatsd_ByteMirrorWire
    dogstatsd: line[13] = "envoy-go.sds.update_success:7|c|#envoy.xds_resource_name:server_cert",
                    want "envoy-go.sds.update_success:7|c|#envoy.cluster_name:backend"
--- FAIL: TestGolden_Graphite_ByteMirrorWire
--- FAIL: TestGolden_LabelMapper_ByteMirrorWire
--- FAIL: TestGolden_OTLP_ByteMirrorWire   (all four cells)
```

The tag literal is **hard-coded at four sites** (`golden_bytemirror_test.go:167`, `:208`, `:253`, `:354`) as `envoy.cluster_name`/`backend`, and `goldenName` is a **positional** struct literal, so adding a per-entry tag field **rewrites all 14 roster lines**. ⇒ **T5 was never independent** — the three non-OTLP goldens go red the instant T4's roster entry lands. **T4+T5 is one atomic task, so the enumeration is 13, and T4 must NOT split** (any sub-split leaves the tree red between legs).

⚠️ **The third §6.1 trigger IS live on the merged T4** (forced struct refactor + 14 rewritten roster lines + 4 literal sites + 4 re-measured `wantBytes` + the full 2×2 cross-product). That is a **mid-execution** trigger; it is named in T4's own gate so it cannot surprise.

### 1.5 ⚠️ THE REJECT'S SITE IS AMBIGUOUS BY ONE STATEMENT, AND THE TWO POSITIONS ARE OBSERVABLY DIFFERENT

§4 specifies *"immediately after `ParseSDSConfig` returns **and** before `RegisterSDSStats`."* Those two clauses span **`boot.go:196`** and **`:200`**, with **`grpcclient.NewSDSClient` at `:196-199` between them.** On a config with a hyphenated name **and** an unknown SDS cluster:

| placement | stderr | message |
|---|---|---|
| **before the dial** (`:196`) | **179 B** | `sds provider: xds: sds: invalid secret name: "server-cert" (…)` |
| after the dial (= baseline) | **128 B** | `sds provider: xds: sds: dial cluster "no_such_cluster": … unknown cluster` |

⇒ **the dial error MASKS the name reject**, and §2.5's own discriminating NC already showed an SDS-less boot dies at `dial tcp …: connection refused` (332 B). A reject satisfying the SPEC's literal wording could sit where **G4's positive arm would require a live SDS server to observe it.** **T5 pins the line: `boot.go:196`, BEFORE `NewSDSClient`.** Found independently by the controller (from the call order) and by P3 (by measurement).

### 1.6 ⚠️ §4.2's ADR-0065(b) TABLE CONTAINS A CELL THAT ASSERTS THE OPPOSITE OF WHAT IT PROVES

§4.2's table row reads `server-cert : all five suffixes agree = false`. **Executed: all five suffixes are `false`, therefore they AGREE — `agree = true`.** Exhaustive sweep: **0 disagreements over 95 single-byte and 9025 two-byte secret names.**

The SPEC's *conclusion* ("validating one assembled name suffices") is **right and now exhaustively proven**; the row it cites as proof states the contrary. `reference_verification_table_launders_wrong_cites` — a "HOLDS" row gets trusted. ⚠️ Note ADR-0065(b)'s own stated *reason* (*"differ only in the suffix's last 4 chars"*) does **not** transfer to the sds suffix set (they differ in length and prefix), so the SPEC was right to re-test rather than inherit — it just recorded the result inverted.

### 1.7 ⚠️ §4.2's SEGMENT PREDICATE IS INCOMPLETE AS LITERALLY WORDED — it passes the EMPTY string

*"no leading dot, no trailing dot, no `..`"* applied **to the secret name** passes `""`, which assembles to `sds..init_fetch_timeout` — an interior empty segment that **`IsValidName` ACCEPTS** (measured: `IsValidName=true`, `guard(secret)=PASS`, `guard(assembled)=REJECT`). **Not exploitable at the chosen site**, because `ParseSDSConfig` rejects `name == ""` at `internal/xds/config.go:28-30` and runs first — **but a unit test of the predicate in isolation would certify an incomplete guard.** ⇒ **T5 checks segments on the ASSEMBLED name**, which is correct under both readings, and T6 pins the `""` cell explicitly.

### 1.8 ⚠️ THE SPEC'S PACKAGE-EDGE NC DOES NOT EXIST — both attempts were vacuous

§4.1 says *"verified `go list -deps ./internal/xds` ⇒ hit, NC `./internal/conv` ⇒ 0 — the first NC I ran was vacuous and was re-run."* **`internal/conv` DOES NOT EXIST**: `ls` ⇒ No such file, `git ls-files internal/conv` ⇒ **0**, `go list` exits **1** with `directory not found`. The replacement NC is vacuous **for a different reason than the original**, and the sentence claiming the fix is where it fires. `reference_gate_command_negative_control`.

⚠️ **And §4.1 checks the WRONG PACKAGE'S EDGE.** The reject lands in `internal/boot`, yet the justification cites `internal/xds` — residue from the pre-§4.1 draft. Re-run correctly, `go list -deps` with **no `...`**:

| package | exit | deps | `internal/stats` |
|---|---|---|---|
| `./internal/xds` | 0 | 317 | 1 |
| **`./internal/boot`** ← the edge that matters | 0 | 554 | **1** (plus a **direct** import at `boot.go:33`) |
| `./internal/conv` (the SPEC's NC) | **1** | **0** | 0 — **for the wrong reason** |
| `./internal/clock` (a REAL NC) | 0 | 47 | **0** |

**The positive claim holds and holds better than stated: `internal/stats` AND `strings` are already imported, so T5 needs ZERO import changes** (measured — the experimental edit compiled with the import block untouched).

### 1.9 ⚠️ SPEC §8 item 2 NAMES THREE PHRASES FOR FOUR OCCURRENCES — the fourth is UNNAMED

§8 item 2 correctly says the live-normative `79.1` set is *"4 occurrences on 3 lines, all in `BEHAVIOR_CONTRACT.md`"* and names three phrases. **The three anchors cover only `:5078`×2 and `:5080`.** The four occurrences sit on lines **`:158`, `:5078`(×2), `:5080`** — and the fourth is:

> `:158` — *"That asymmetry, not scope, is **why the `sds.` family is deferred to row 79.1**"*

**A phrase-anchored sweep that trusts the SPEC's list leaves `:158` stale.** Independently confirmed by the controller (`grep -nF '79.1'` ⇒ exactly those three line numbers, 4 occurrences). **This is the single most actionable defect this stage found.**

### 1.10 ⚠️ TWO CELLS THE SPEC MARKS LIVE-EDITABLE ARE HISTORICAL AND MUST NOT BE TOUCHED

| SPEC §6.1 item | site | why it must NOT change |
|---|---|---|
| item 9 | `internal/stats/segmentcount_test.go:21` | *"the message … enumerated NINE top-level segments **when twelve were live**"* — **past tense about phase 79.** Editing it falsifies the record. ⚠️ The file hard-codes **no numeral at all**; this is its only `twelve` and it is historical. |
| item 13 | `docs/envoy-go/STATE.md:48` | a §Recent lineage narrative about the phase-79 IMPL. §Recent entries migrate **VERBATIM** to `STATE_HISTORY.md` at eviction; editing breaks that invariant. |

### 1.11 ⚠️ TWO TRAPS THAT WOULD SILENTLY DEFEAT THE SWEEP

1. **CASE.** **Four of the six live `0118` hits are UPPERCASE `TWELVE`.** `git grep -cE '\btwelve\b'` **without `-i`** returns `README.md` only — **2 of 6.** ⇒ **the sweep MUST be `-i`.**
2. **ONE LINE, TWO OPPOSITE VERDICTS.** `BEHAVIOR_CONTRACT.md:167` carries **two** `twelve` occurrences: *"enumerated the **pre-79 nine** while **twelve** were live"* (**historical — must not change**) and *"The message now enumerates **twelve** top-level segments"* (**live — must change**). **A line-level substitution corrupts one.** Verified by the controller: `awk 'NR==167' | grep -oi twelve | wc -l` ⇒ **2**.

### 1.12 ⚠️ A WRONG FIGURE IS ALREADY BAKED INTO A LANDED ADR — and it is landable BECAUSE the ADR is still PROPOSED

The spelled-needle noise figure is **43 files** (`git grep -liE '\btwelve\b'`; **42** under the wrapped grep, and **42** even at the SPEC's own base). **It is never 45.** The wrong figure is live at three sites: **`DECISIONS.md:17686` (ADR-0302 §Context ¶11)**, `80/SPEC.md:384`, `next-prompt.txt:114`. ⇒ **the PLAN must not re-cite 45**, and because ADR-0302's STATUS is still `PROPOSED` and T13 already edits it, **the correction lands there** rather than becoming a permanent discrepancy.

### 1.13 ⚠️ THE SWEEP AND ROSTER FIGURES ARE STALE BY THE SPEC'S OWN COMMIT — three of them

`reference_branchpoint_roster_stale_midrow`, three times in one document:

| figure | SPEC says | @ `53855de0` (SPEC base) | @ `47b9b378` (THIS tip) |
|---|---|---|---|
| numeral needle `one of the 12` | 4 hits / 4 files | **4 / 4** ✅ exact | **6 / 6** (+`80/SPEC.md:383`, +**`next-prompt.txt:114`**) |
| scoped spelled needle | 26 lines / **20 files** | **26 lines / 18 files** — ⚠️ **the FILE count was wrong even at its own base** | **33 lines / 19 files** |
| `79.1` roster | 50 / 9 files | **50 / 9** ✅ exact | **61 / 10 files** |
| ADR block (§8 item 7) | 300 headings, ids 0001-**0301**, tail **ADR-0301**, `^## ADR-0302`⇒0 | — | **301 headings, ids 0001-0302, tail ADR-0302 PROPOSED, `^## ADR-0303`⇒0, NC `^## ADR-0302`⇒1** |

⚠️ **`7d014546` — the SPEC's own commit — added ADR-0302 (34 lines).** §8 item 7's entire block is stale by the commit carrying it, exactly as §2.9 is. **The IMPL re-derives at ITS tip and inherits none of these.** The gap/dup structure is unchanged and confirmed: **exactly one gap at ADR-0209, zero duplicates.**

⚠️ **§2.9's *"~11 occurrences in this row's OWN live documents"* is now 29** (`ROADMAP.md:142`×2 + `STATE.md`×4 + `80/BRAINSTORM.md`×8 + `80/SPEC.md`×8 + `next-prompt.txt`×7).

### 1.14 ⚠️ §6.1's "LIVE-EDITABLE (8 files / 16 lines)" DOES NOT DECOMPOSE — and §9/§11 CITE A PHANTOM ITEM

Measured live-normative set: **16 occurrences across 10 files** (§3). **§6.1's own items 8-14 sum to 13 lines across 9 files** — the figure is internally inconsistent with the enumeration printed beneath it, on either reading.

⚠️ **And §9's T2 row cites *"§6.1 items 8-15"* while §11 cites *"§6.1 item 15"*. §6.1 enumerates items 1-14.** The highest is 14 (`next-prompt.txt`). **A phantom item, cited twice.**

### 1.15 ⚠️ SPEC §4.3's SKIP-UNREACHABILITY IS CONFIRMED, BUT ITS BLAST-RADIUS FIGURE IS A DIFFERENT MEASUREMENT

§5.4's *"the whole-tree blast radius is ONE failing test"* is confirmed **for the projection arm over two package trees** (`./internal/stats/... ./internal/statssink/...`: baseline exit 0; with the arm, exit 1 and **exactly** `TestTerminalError_TopLevelCountMatchesCode`, `internal/statssink` **ok**). ⚠️ **The REJECT's own radius was never measured.** Measured now: **D-SDS-2 in isolation has ZERO whole-tree blast radius** — `go test -count=1` over **124 packages** (all but `test/differential`) ⇒ exit **0**, zero `FAIL`. **Two different numbers for two different changes; do not conflate them.**

Skip-branch unreachability **confirmed with new evidence**: over 95 bytes, guard-reject ⊇ skipAll with exactly **1** mismatch (`"."`, where the guard is *stricter*), and structurally guard-pass ⇒ all five register. **New: the reject covers the VC-SDS shape used by 4 of the 5 fixtures, not just cert-SDS.**

### 1.16 ⚠️ THE "NO-OP ON THE CORPUS" CLAIM SURVIVES — from a WRONG DENOMINATOR, and §2.8 hides the fixture count

**Denominators actually scanned** (`git grep`/`find`, names printed): **120** fixture dirs · **688** tracked files under `test/fixtures` · **249** yaml/json · **179** driver `.go` · `test/conformance` **40** files with **0** `sds` hits · **7** `internal/**/testdata` dirs with 0 secret names · **0** `secrets:` static-SDS blocks in `test/` · **0** hyphenated `name:` in the five SDS fixtures.

**FIVE fixtures carry `sds_secret_config`, not four** — `0108` and `0109` **share** `validation_ca`. Four distinct *names* is right; **the corpus footprint is 5 fixtures × 5 counters = 25 registrations, not 20.** All four names pass both legs ⇒ **the reject is a genuine no-op on the corpus**, now established by enumeration rather than by four hand-picked names. Predicate negative-controlled **8/8**: `server-cert`⇒REJECT(charset), `trailing_dot.`/`.lead`/`a..b`⇒REJECT(segment), all four corpus names ⇒ PASS; `my.dotted.cert`/`1leading_digit`/`UPPER` PASS, `a/b` REJECT. **Neither all-accept nor all-reject.**

⚠️ **`0024`'s exclusion is structurally stronger than §2.8 argues** and does not depend on reading YAML: `RegisterSDSStats` has **exactly one** non-test call site (`boot.go:201`) on the xDS/ADS branch, so `path_config_source` secrets **cannot reach it at all** (and both are valid anyway).

### 1.17 ⚠️ TWO FREE ITEMS THE SPEC PRICES AS WORK, AND ONE STALE COUNT NEITHER DOCUMENT FLAGS

- **§5.2's "no-self-equal leg extended to cover them" costs ZERO lines.** `TestHelpText_NoSelfEqualHelp` (`helptext_test.go:147`) already iterates `helpTextRoster` and drives the real `WriteProm`, so adding roster entries extends it automatically. **One of T3's three named components is free.**
- **T10's import delta is +0.** `bytes`, `fmt`, `math`, `net/http`, `strconv`, `strings` are all already imported by the `0110` driver — which is why P2's fragment was `gofmt`/`vet`/`golangci-lint` clean on first pass.
- ⚠️ **NEW, flagged by neither the SPEC nor any probe's roster: `internal/stats/name.go:490` reads *"Of the 25 entries, the first 10 cover the 13 unique Prometheus names"*.** `helpText` has **25** entries today and goes to **30** at T3, so **`:490`'s "25" is a live stale count T3 must move.** (P4 saw `:490` only as a grep-collision hazard on the numeral `13`, which it also is — a pre-existing, unrelated `13` that a post-move `grep '13' name.go` will hit.)

### 1.18 Confirmed unchanged, so the IMPL can rely on them

The `0110`→`rccf_validation_ca` / `0111`→`edf_validation_ca` attribution (**re-derived independently by the controller**; BRAINSTORM §4.3 is right and the phase-79 PLAN's *"SWAPPED"* correction is the false claim — `reference_a_drift_correction_is_itself_a_claim`) · §7.1's five-row fixture table, **all five rows** · `0110`'s blank import present at `runner_test.go:137`, **zero registration gates owed** · §2.8's four corpus secrets **all dot-free**, so the fixture **cannot** discriminate the first-dot fork ⇒ **G6 is mandatory** · §5.1's *"blind in all four cells"*, and **extending the roster un-blinds all four** (measured, not computed: F_F **1134→1212**, F_T **1200→1320**, T_F **1118→1184**, T_T **1184→1292**) · §2.7's `denominator=95 allFive=64 allNil=31 partial=0` and the exact 31-byte rejected set · §2.5 arm A2 (exit 1, 0 B stdout, **115 B** stderr) · §2.6's inversion, reproduced · `RegisterSDSStats` **1** non-test call site / 25 `_test.go`; `ParseSDSConfig` **4** non-test call sites with the three `internal/tls` ones wrapping `tls: downstream: %w` · ADR-0302's footer **byte-exact** (`od`-verified U+00A7; all 8 footers normalize to ONE string; ADR-0294…0300 = **SEVEN**, ADR-0301 = **0**, the recorded departure) · ADR-0299's stale `PROPOSED` · ADR-0301's seven-vs-six miscount · the eviction target and the phase-77 PLAN archive gap, both with firing NCs.

---

## 2. Global constraints

1. **One stage per session.** This is the PLAN. `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` stay **BYTE-UNTOUCHED**; row 80 stays `in-progress`; sentinel `want` stays **112**. The row flips to `done` at the IMPL only (T13).
2. **TDD spine.** Every behavioral task is RED-first with the RED **observed and recorded**, never inferred. A green that could also mean "did not run" is not a result (`reference_liveness_break_needs_failing_baseline`).
3. **One `t.Errorf` per property.** `t.Fatalf` makes later assertions dead code (`reference_fatalf_makes_assertions_unreachable`). The single `t.Fatalf` in T9 is deliberate and scoped to the *scraper-broken* diagnosis; every per-name assertion uses `Errorf`.
4. **Assert the SET, not a count** (`reference_stat_count_guard_blind_to_rename`). Report missing/extra separately.
5. **`-count=1` always.** A cached PASS is not a run.
6. **`gofmt -l` never exits non-zero** — gate on **OUTPUT**.
7. **`golangci-lint` runs `misspell` with `locale: US`** — ⚠️ **LIVE this row**: it edits Go comments **and** an error string. **Do not paste PLAN or SPEC prose into `.go` files.**
8. **Capture `INNER_EXIT`** — a harness's exit code is not the command's. ⚠️ **It FIRED AGAIN in the CI-repair session** (`golangci-lint … | tail` reported `LINT_EXIT=0`, which was `tail`'s status) **and once in this stage's own controller probes** (`go list … | grep -c; echo $?` reported `grep`'s status). Capture the tool's own exit code **and** its output byte count.
9. **`go build ./cmd/envoy-go/` drops an untracked binary in the worktree root** — build with `-o` into scratch.
10. **`git grep` for EVERY repo-wide sweep, and `-i`.** A recursive `grep` here is a shell function execing ugrep with `--ignore-files`, which honours `.gitignore`; `next-prompt.txt` is gitignored-but-**TRACKED** and therefore INVISIBLE to it (re-verified twice this stage: 9 vs 10 files, delta exactly that file; and 42 vs 43 on the `twelve` needle). **`git check-ignore` reassures you WRONGLY.** **Print FILE NAMES, never a count.**
11. **Git hygiene.** `git -C <abs-worktree-path>` for every git command; the Bash cwd reset is assumed live (**twenty-third consecutive session**). Subagents commit **locally only**, never push; controller squash-pushes at close. Parallel agents get private scratch, private **detached** worktrees, and port bands clear of **BOTH** `20000-31007` **and** `11000-14999` plus the static fixture ports; docker containers torn down **BY NAME**, never by an `ancestor=`/image filter.
12. **Breaks run AFTER committing** (`reference_break_protocol_commit_first`) with **`-count=1`**. Never an unscoped `git restore`.
13. **Every count re-derived at the IMPL's own tip.** Three of this SPEC's rosters were stale by the SPEC's own commit (§1.13). **Inherit no figure from this PLAN either.**

---

## 3. File structure — the IMPL's edit surface, RE-DERIVED

**Production (3 files):**
1. `internal/stats/name.go` — the `sds.` arm in the `switch` opened at **`:81`**, inserted after the `tracing.` arm (`:136-138`) and before `wasm.` (`:139`) · the const **`noRecognizedSegmentErrFmt` `:43-46`** (`12`→`13`, `sds.` into the pipe list; **the mid-name `4` is UNCHANGED** — `sds.` is a ROOT) · the doc block **`:24-46`** (`:30` numeral, `:34` spelled) · the SN-rule doc comment **`:59-75`** · **`helpText` `:513-548`** (25→30) and its doc-block count at **`:490`** (25→30).
2. `internal/boot/boot.go` — the reject at **`:196`**, after `ParseSDSConfig` (`:192-195`) and **BEFORE `grpcclient.NewSDSClient` (`:196-199`)**. **Zero import changes.**
3. `internal/admin/stats.go:20` — *"not one of the twelve top-level segments ExtractTags recognizes"*.

**Test (6 files):** `internal/stats/name_test.go` — `wantNoRecognizedSegmentErrFmt` **`:1066`**, its prose **`:1052`**, the arm's table rows, the G6 dotted-name fork test · `internal/stats/helptext_test.go` — `helpTextRoster` **`:40-68`** (25→30); `TestHelpText_NoSelfEqualHelp` **`:147`** extends free · `internal/stats/promskip_test.go:16` — enumerates all twelve, so **`sds.` joins the list** · `internal/statssink/golden_bytemirror_test.go` — `goldenRoster` **`:56-70`**, `goldenTaggedIdx` **`:76`**, `goldenRegistry`'s *"13-entry"* comment **`:86`**, the four hard-coded tag literals **`:167 :208 :253 :354`**, `goldenOTLPCells` **`:300-305`** · `internal/boot/boot_test.go` — G4 · `internal/xds/stats_test.go` — the skip-unreachable test.

**Fixture (3 files, ONE directory):** `test/fixtures/0110-tls-require-client-cert-false/` → `driver/driver.go` (`scrapeProm` **`:774-833`** retained unchanged; the new `scrapePromLabeled` + assertion; `AssertStats` **`:655`**; the prohibition prose **`:643-646`**) · `expectations.yaml:210` · `README.md:178`. **Plus `0111`'s parallel pair if mirrored.**

**Stale-`TWELVE` prose, LIVE-NORMATIVE — 16 occurrences / 10 files** (`git grep -i`, three-way classified):

| file | occ | note |
|---|---|---|
| `internal/stats/name.go` | 3 | `:30` numeral, `:34` spelled, `:44` in the const |
| `internal/stats/name_test.go` | 2 | `:1052` prose, `:1066` the hand-written twin |
| `internal/stats/promskip_test.go` | 1 | `:16` — **enumerates the roster**, so `sds.` is added too |
| `internal/admin/stats.go` | 1 | `:20` present tense |
| `test/fixtures/0118-runtime-static-layer/README.md` | 2 | `:32`, `:51` |
| `…/0118/driver/driver.go` | 2 | `:156`, `:177` — ⚠️ **UPPERCASE** |
| `…/0118/expectations.yaml` | 2 | `:44`, `:60` — ⚠️ **UPPERCASE** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | 2 | `:162` live; **`:167` is 1-of-2 — the live half only** (§1.11) |
| `next-prompt.txt` | 1 | `:150` — **`git grep`-only** |

⚠️ **The `0118` SIX are confirmed exactly** (README ×2, driver ×2, expectations ×2; `envoy.yaml` ⇒ **0**). **The fixture is NOT de-fanged** — all six enumerate the roster and assert *"the top-level answer is TWELVE"*.

**MUST NOT TOUCH (historical / invariant-blocked):** `internal/stats/segmentcount_test.go:21` and `docs/envoy-go/STATE.md:48` (§1.10) · `phases/{04,06.1,56,79,80}/…` · `STATE_HISTORY.md` · **`ROADMAP.md` `:118`×2 `:134`×1 `:141`×2 = FIVE occurrences** — §Schema-blocked, and ⚠️ **four of the five are pure FALSE POSITIVES** (tap-filter *"charter on TWELVE counts"*, *"Twelve controller-reperformed deliberate breaks"*, *"TWELVE MINOR"*, *"TWELVE tasks executed"*) with the fifth historically true, so **nothing there needs a `12→13` move on the merits** · **`DECISIONS.md` FOUR scoped hits, not three** — `:16546` FP, `:17646` historical, **`:17634` (ADR-0301 §Decision (b), *"has failed twelve top-level detectors"*) is PRESENT TENSE inside immutable ADR text and goes factually WRONG the moment row 80 lands** ⇒ record as a named permanent discrepancy alongside the ROADMAP five; `:17686` is ADR-0302 ¶11, editable at T13 (§1.12).

**Docs:** `DECISIONS.md` (ADR-0302 §Decision + §Consequences + STATUS flip; ADR-0299 rider; ¶11's `45`→43) · `BEHAVIOR_CONTRACT.md` · `ROADMAP.md` (row 80 → `done`, **IMPL only**) · `STATE.md` · `STATE_HISTORY.md` · `next-prompt.txt`.

---

## Task 1 — the `sds.` arm, the `12 → 13` constant, both doc blocks, the hand-written twin, and the DOTTED-NAME fork test

⚠️ **ONE TASK — items 1-4 of §6.1 plus G6 cannot split.** The AST guard compares the **live constant** against `ExtractTags`' **own AST**, so arm-without-string and string-without-arm each go red. And G6 belongs here because **the dotted-name behaviour IS this task's design decision** — §2.8 proves the fixture cannot discriminate it, so shipping the arm without G6 ships the row's central decision unguarded.

**RED-FIRST.** Add the table rows and the G6 fork test to `name_test.go` **before** the arm; observe RED; then green.

**The arm** — `internal/stats/name.go`, inserted after `:138` (`tracing.`) and before `:139` (`wasm.`), structurally cloned from the `cluster.` SN1 arm at `:82-91`:

```go
case strings.HasPrefix(internal, "sds."):
	// Rule SN1 shape (phase 80): sds.<secret>.<rest> hoists the
	// operator-supplied secret name into envoy_xds_resource_name.
	// FIRST-dot, single-segment, non-greedy -- byte-identical to the
	// reference's own tag extractor, recovered from the pinned binary:
	//   ^sds\.((<TAG_VALUE>)\.).+   with <TAG_VALUE> = [^\.]+
	// Last-dot would need a strings.LastIndex that appears NOWHERE in this
	// function: all four dynamic-token arms use first-dot. No dot->underscore
	// pre-transform belongs here; flattenToProm applies it at projection time.
	tail := strings.TrimPrefix(internal, "sds.")
	dot := strings.Index(tail, ".")
	if dot < 0 {
		return "", nil, fmt.Errorf("stats: name %q matches sds.* but has no <rest> segment", internal)
	}
	labels = append(labels, Label{Key: "envoy_xds_resource_name", Value: tail[:dot]})
	rest = tail[dot+1:]
	residual = "sds." + rest
```

**The constant** — `:43-46`, two edits on one line, **`12`→`13` and `sds.` into the pipe list at its source-order position** (after `tracing.`, before `wasm.`). ⚠️ **The mid-name `4` is UNCHANGED.** ⚠️ **`misspell` locale US is live**; the replacement carries identifier tokens only.

```go
const noRecognizedSegmentErrFmt = "stats: name %q has no recognized top-level segment " +
	"(want one of the 13: cluster.|http.|listener.|server.|runtime.|access_logs.|tracing.|sds.|wasm.|mongo.|kafka.|redis.|thrift.) " +
	"and no recognized mid-name segment " +
	"(want one of the 4: .http_local_rate_limit.|.http_bandwidth_limit.|.rbac.|.zookeeper.)"
```

**The doc blocks** — `:30` (`12 TOP-LEVEL segment detectors` → 13), `:34` (`the top-level twelve` → thirteen), and the SN-rule comment `:59-75` gains an SN1-shaped `sds.` line naming the first-dot decision.

**The hand-written twin** — `name_test.go:1066` `wantNoRecognizedSegmentErrFmt`, byte-identical to the new constant, plus its prose at `:1052`.
⚠️ **THIS TWIN IS WHY PHASE 79 SHIPPED A WRONG NUMBER THROUGH A BYTE-STABILITY GUARD.** *"A guard whose expectation is authored by hand shares the author's mistake."* **The protection is `segmentcount_test.go`, which hard-codes nothing** — it AST-derives one side and parses the count **and the set** out of the live constant via `claimedRe` (`(want one of the (\d+): ([^)]*)\)`), comparing **sets order-independently** and reporting missing/extra separately. **Do not hand-verify the number; let the guard derive it.**

**G6 — the dotted-name fork test** (the fixture cannot do this — all four corpus secrets are dot-free):

```go
// FIRST-dot, single-segment, non-greedy. Reference-pinned; last-dot would
// yield envoy_xds_resource_name="my.dotted.cert" and base envoy_sds_update_success.
{in: "sds.my.dotted.cert.update_success", wantResidual: "sds.dotted.cert.update_success",
	wantLabels: []Label{{Key: "envoy_xds_resource_name", Value: "my"}}},
{in: "sds.alpha.beta.gamma.update_success", wantResidual: "sds.beta.gamma.update_success",
	wantLabels: []Label{{Key: "envoy_xds_resource_name", Value: "alpha"}}},
// the arm's own dot<0 reject, mirroring SN1/SN2/SN3
{in: "sds.nodots", wantErr: true},
```

**Stacked controls in the same test:** `cluster.backend.upstream_rq_total` must **keep** its hoisted label; `filesystem.flushed_by_timer` must **stay** rejected.

**No collision:** `git grep -n '"sds\.' internal/stats/` ⇒ **0** before the edit (verify; an existing negative would silently invert).

**Gate:** RED observed first · `go test ./internal/stats/ -count=1 -v` INNER_EXIT 0 with the `--- PASS` census recorded (a count proves it ran, not that it was skipped) · `TestTerminalError_TopLevelCountMatchesCode`, `TestExtractTagsTerminalError_ByteStable`, `TestTerminalError_NamedRootsAreAccepted` all green · `gofmt -l internal/stats/` **output** empty · `golangci-lint run ./internal/stats/...` with its **own** exit code and byte count.

---

## Task 2 — the stale-`TWELVE` sweep: **16 occurrences / 10 files**, `git grep -i`, three-way classified

⚠️ **This is a TASK, not a footnote.** Phase 79's headline was that fixing the number without auditing where it lives *"reproduces the same defect one generation on"* — **and phase 79's own IMPL then did exactly that.**

**Sweep BOTH needles; scope the spelled one; use `git grep`; use `-i`.**

```sh
git grep -niE '\bone of the 12\b'                                    # numeral: 6 hits/6 files at this tip
git grep -niE '\btwelve\b' -- ':!docs/envoy-go/phases' ':!docs/envoy-go/STATE_HISTORY.md'
git grep -niE '\btwelve\b' | grep -iE 'segment|detector|ExtractTags|top-level|prefix'   # scoped
```

**Edit the 16/10 roster in §3. Do NOT edit the MUST-NOT-TOUCH set.** Three specific hazards:
- ⚠️ **`-i` is mandatory** — 4 of the 6 `0118` hits are UPPERCASE (§1.11).
- ⚠️ **`BEHAVIOR_CONTRACT.md:167` needs a phrase-scoped edit, not a line-level one** — two occurrences, opposite verdicts (§1.11).
- ⚠️ **`promskip_test.go:16` enumerates the roster**, so `sds.` joins the list. Its sentinel `promSkipUnprojectableName = "filesystem.flushed_by_timer"` **stays valid** (not sds-rooted) — **re-check it against `flattenToProm`, not against prose.**

**The two numeral false positives are CONFIRMED and must NOT be edited:** `phases/09-http-filter-fault/PROGRESS.md:1070` and `REVIEW.md:296`, both *"the 12 existing fuzzers"*.

**Gate — per-site, with a stated denominator, negative-controlled.** ⚠️ **`grep` cannot tell a mention from a use** (`reference_sentinel_matcher_string_self_clears`). The after-state check is *"the stale form appears **0** times in these 10 files"* **AND** *"the new form appears **16** times"*, each run against a **deliberately doctored scratch copy** to prove it fires. **Fix by PATTERN, not by line** — the enumeration recurs by inheritance, and every by-line cite written during phase 79 went stale inside phase 79.

⚠️ **Collision:** `name.go:490` already contains an unrelated `13` (*"the 13 unique Prometheus names"*). A post-move `grep '13' name.go` hits it. **Anchor on the phrase, not the numeral.**

---

## Task 3 — `helpText` 25 → 30, `helpTextRoster` 25 → 30, and the `:490` doc count

⚠️ **MANDATORY, NOT COSMETIC.** Under the arm, `WriteProm` emits the five families with **degraded HELP** — `# HELP envoy_sds_update_success envoy_sds_update_success` — because `prom.go`'s `if help == "" { help = g.name }` fires. **`TestHelpText_NoSelfEqualHelp` does not catch it today** only because the roster has no sds entries.

**`helpText`** (`name.go:513-548`), five entries, one line each:

```go
"envoy_sds_update_success":     "Total successful SDS secret updates delivered by the management server.",
"envoy_sds_update_failure":     "Total SDS secret updates that failed to apply.",
"envoy_sds_update_rejected":    "Total SDS secret updates rejected as invalid.",
"envoy_sds_update_attempt":     "Total SDS secret update attempts initiated.",
"envoy_sds_init_fetch_timeout": "Total SDS initial-fetch timeouts.",
```

⚠️ **DERIVE the five keys by running the REAL `flattenToProm` over the five registered internal names — never hand-type them.** A hand-typed key that disagrees with the projection passes the key-set guard *and* the no-self-equal guard if the golden is typed to match (phase 79's arm-4a result).

**`helpTextRoster`** (`helptext_test.go:40-68`), five entries. ⚠️ **Two-segment shape mandatory** — a single-segment `sds.x` hits T1's `dot < 0` reject:

```go
{internal: "sds.server_cert.update_success"},
{internal: "sds.server_cert.update_failure"},
{internal: "sds.server_cert.update_rejected"},
{internal: "sds.server_cert.update_attempt"},
{internal: "sds.server_cert.init_fetch_timeout"},
```

**`name.go:490`** — *"Of the **25** entries"* → **30** (§1.17), and extend the trailing narrative to name phase 80's `sds.` root.

⚠️ **`TestHelpText_NoSelfEqualHelp` needs NO edit** — it already iterates the roster (§1.17). **Do not price it as work; do confirm it now covers the five.**

⚠️ **THIS IS INTERNAL-CONSISTENCY WORK, NOT CONFORMANCE WORK.** The reference emits **ZERO `# HELP` lines** (`# TYPE` ×330, `# HELP` ×0). Extending `helpText` **widens** a pre-existing block-level departure rather than closing one — already stated in the map's own doc comment. **Do not describe it as parity work.**

**Gate:** RED observed on both `TestHelpText_KeySetExact` and `TestHelpText_NoSelfEqualHelp` before the `helpText` entries land; green after · `go test ./internal/stats/ -count=1`.

---

## Task 4 — the sink goldens: roster + tagged index + the FOUR hard-coded tag literals + four RE-MEASURED OTLP pins

⚠️ **FORCED ATOMIC — this is §1.4 and it is the largest task in the row.** All four goldens fail off one roster entry, so T4 **cannot** split and the SPEC's "T5" does not exist as a separate task.

⚠️ **THE THIRD §6.1 TRIGGER IS LIVE HERE** (>~10 sub-steps): positional-struct refactor · 14 rewritten roster lines · 4 literal sites · 4 re-measured `wantBytes` · the full 2×2 cross-product. **If it blows past ~10 sub-steps in execution, record it — do not silently absorb it.**

**Sub-steps:**
1. `goldenRoster` (`:56-70`) gains one `sds.*` entry. **Append at index 13** (minimal churn; the roster is in **registration == emission** order, so position determines bytes):
   ```go
   {"sds.server_cert.update_success", "sds.update_success", true, 7},
   ```
2. `goldenTaggedIdx` (`:76`) → `map[int]bool{0: true, 1: true, 13: true}`.
3. `goldenRegistry`'s doc comment (`:86`) — *"the 13-entry registry"* → **14**.
4. **The four hard-coded tag literals** (`:167`, `:208`, `:253`, `:354`) currently assume `envoy.cluster_name`/`backend` for every tagged index. Add a **per-entry tag field** to `goldenName` — ⚠️ **it is a POSITIONAL struct literal, so all 14 roster lines are rewritten.**
5. **RE-MEASURE all four `goldenOTLPCells.wantBytes`. Do NOT compute them.** Measured at this stage under a real arm: **1134→1212 · 1200→1320 · 1118→1184 · 1184→1292.** ⚠️ **These are this stage's numbers on this stage's experimental entry; the IMPL's entry may differ. Re-measure.** ⚠️ Phase 79 recorded that OTLP absolutes carry an unnamed 15-char version string (size = `base + len(version)`), so **absolutes are tree-local while deltas reproduce** — pin what you measure, in-tree.
6. Run the **full 2×2 cross-product**, not the default cell (`reference_probe_must_discriminate`).

⚠️ **THE HAZARD THIS TASK EXISTS TO REMOVE.** Before the roster grows, **all four cells are byte-identical under a real `sds.` arm** — `F_F_default_INERT_UNDER_HOIST` 1134/1134, `F_T` 1200/1200, `T_F` 1118/1118, `T_T` 1184/1184. **The command, the assertion and the cross-product are all correct; the INPUT ROSTER is the defect** (`reference_golden_roster_omits_family_under_test`, broken-gate shape **SEVENTEEN**). **Extending the roster is what un-blinds all four** — and running the cross-product without extending it first proves nothing.

⚠️ The sink attribute key is **`envoy.xds_resource_name`** (dotted); the Prometheus label key is **`envoy_xds_resource_name`** (underscored). **Do not conflate them.** ⚠️ **DogStatsd tag order stays UNSORTED** — `formatTagSuffix` has no `sort.Slice`. With one hoisted label there is no order to observe; **do not sort** (`reference_dogstatsd_tag_order_unsorted`).

**Gate:** RED on all four goldens observed before the re-measure; green after · `go test ./internal/statssink/ -count=1 -v` with the `--- PASS` census · the three non-OTLP goldens named individually in the record.

---

## Task 5 — the boot-boundary reject, PINNED at `boot.go:196`, BEFORE the dial

⚠️ **THE LINE IS PINNED, NOT A RANGE** (§1.5). After `ParseSDSConfig` (`:192-195`), **before `grpcclient.NewSDSClient` (`:196-199`)**. Placed after the dial, an unreachable-cluster or absent-SDS-server config fails at `dial` first (**128 B** / **332 B**) and **masks** the name reject (**179 B**).

```go
secretName, clusterName, timeout, err := xds.ParseSDSConfig(found)
if err != nil {
	return nil, err
}
if err := validateSDSSecretName(secretName); err != nil {
	return nil, err
}
```

**The predicate — guard the SEGMENTS on the ASSEMBLED name** (§1.7; `reference_dynamic_stat_name_charset_guard`: *"guard on the segments, not only on the assembled name, when empties are reachable"*):

```go
// validateSDSSecretName rejects a secret name that cannot form a clean metric
// name. Two legs: the stats charset, and segment well-formedness -- IsValidName
// is a CHARSET guard only and ACCEPTS an interior empty segment, so
// "trailing_dot." assembles to sds.trailing_dot..init_fetch_timeout, registers
// cleanly, and would project to envoy_sds__update_success where the reference
// serves envoy_sds_update_success{envoy_xds_resource_name="trail"}.
//
// Validating the LONGEST suffix suffices: all five suffixes agree for every
// secret name (exhaustively verified over 95 single-byte and 9025 two-byte
// names, zero disagreements), per ADR-0065 Consequences (b).
//
// ADR-0065 Consequences (e) mandates boundary validation for metrics derived
// from user input. Sanitising is FORECLOSED by that ADR's own Context: two
// names differing only in invalid characters would collapse to one label value.
func validateSDSSecretName(name string) error {
	assembled := "sds." + name + ".init_fetch_timeout"
	for _, seg := range strings.Split(assembled, ".") {
		if seg == "" {
			return fmt.Errorf(invalidSecretNameErrFmt, name)
		}
	}
	if !stats.IsValidName(assembled) {
		return fmt.Errorf(invalidSecretNameErrFmt, name)
	}
	return nil
}
```

```go
const invalidSecretNameErrFmt = "xds: sds: invalid secret name: %q " +
	"(must contain only ASCII letters, digits, underscore, or dot, " +
	"and form a valid metric-name segment)"
```

⚠️ **ZERO import changes** — `internal/stats` and `strings` are already imported (`boot.go:33`); measured (§1.8).

**What STAYS:** the `RegisterSDSStats` guard **and** `incNil`. Two-layer defence is the established pattern; `incNil`'s deletion is a proven boot-path SIGSEGV (`reference_nil_stats_counter_inc_crashes_goroutine`). **T7 converts the skip from live logic to defence-in-depth by proving it unreachable.**

⚠️ **THIS IS A DOCUMENTED envoy-go-STRICT DEPARTURE, NOT A FIX.** The reference **accepts** `server-cert`, boots green, and serves `envoy_sds_update_success{envoy_xds_resource_name="server-cert"} 1` with the hyphen `od`-verified verbatim in the label value. A hyphenated secret boots green there and will **boot-FAIL** here. T10 records it as a departure with the reference behaviour quoted.

⚠️ **Both deadlock-prone designs stay EXCLUDED** (`reference_registry_walk_lock_inversion`): no counter registered from inside the projection path, no lazy registration at first increment. **A boot-boundary reject registers nothing and walks nothing**, and it **moots ADR-0300 §Consequences (iii)** entirely — it returns an error into `main.go:156`'s existing `log.Fatalf` and never panics. ⚠️ **`--mode validate` is FORECLOSED as a home**: `validate.Bootstrap` passes a nil provider, so `NewSDSProvider` is never reached.

**Gate:** `go test ./internal/boot/ -count=1 -v` with the **`RUN`/`PASS`/`SKIP`/`FAIL` census recorded** — ⚠️ this package passed in **0.228 s** with the reject applied, implausibly fast for SDS e2e; the census (`RUN=21 PASS=21 SKIP=0 FAIL=0`, including `TestSDSEndToEnd_ServerCertViaSDS_HandshakeServesDeliveredLeaf`) is what makes the green falsifiable · `go build ./cmd/envoy-go/ -o "$SCRATCH/envoy-go"`.

---

## Task 6 — G4: the reject's POSITIVE, SEGMENT and NEGATIVE arms, plus the predicate's own NC

⚠️ **A guard that rejects everything passes a positive-only gate.** All three arms are required.

| arm | input | must produce |
|---|---|---|
| **positive** | `server-cert` | the exact `xds: sds: invalid secret name: "server-cert"` string |
| **segment** | `trailing_dot.`, `.lead`, `a..b`, **`""`** | the same string — ⚠️ `""` is the §1.7 cell; it is caught by `ParseSDSConfig` first in production, so assert the **predicate** directly |
| **negative** | `server_cert`, `validation_ca`, `rccf_validation_ca`, `edf_validation_ca` | **accept** |
| **fork-adjacent** | `my.dotted.cert`, `1leading_digit`, `UPPER` | **accept** (dots are legal; `sds.` supplies the leading char, the fixed suffix the trailing one) |
| **charset** | `a/b` | reject |

⚠️ **DERIVE every expectation from `IsValidName`, never by hand.** The SPEC's own probe hand-asserted `1leading_digit` and `trailing_dot.` as invalid; **both are valid** as bare names (`reference_probe_input_is_a_claim`).

**End-to-end arms, measured at this stage and reproducible:**

| arm | exit | stdout | stderr |
|---|---|---|---|
| cert-SDS, `server-cert`, live server | **1** | **0 B** | **179 B**, the exact string |
| segment, `trailing_dot.`, no server | **1** | **0 B** | **181 B** |
| **VC-SDS**, `rccf-validation-ca`, live | **1** | **0 B** | **186 B** — ⚠️ **the shape 4 of 5 fixtures use** |
| READY control, `server_cert`, live | — | **63 B** `envoy-go ready` | **0 B**, 5 counters on `/stats` |
| **failing baseline** (no reject), `server-cert` | 1 | 0 B | 332 B **dial** error — **NOT rejected today** |

⚠️ **The failing baseline is what makes this gate non-vacuous** (`reference_liveness_break_needs_failing_baseline`). ⚠️ **Discriminate hang-vs-exit by OUTPUT, never by status** — `timeout` exit 124 is shared by a healthy server and a hung boot.

**Gate:** each arm asserts the **STRING**, not the byte volume (`reference_output_volume_is_not_output_content`) · valid-arm stderr **byte-identical to baseline** (timestamp-stripped `diff` empty).

---

## Task 7 — the `RegisterSDSStats` skip branch is UNREACHABLE from production config

Converts the retained guard from live logic to documented defence-in-depth. **Evidence to reproduce, not to inherit:** over 95 printable bytes, **guard-reject ⊇ skipAll with exactly ONE mismatch** (`"."`, where T5's guard is *stricter*), and structurally guard-pass ⇒ `IsValidName(longest)` ⇒ (by the exhaustively verified agreement property, §1.6) **all five register**. `incNil` audit: `denominator=95 allFive=64 allNil=31 partial=0`, the 31 rejected bytes being `[space ! " # $ % & ' ( ) * + , - / : ; < = > ? @ [ \ ] ^ \` { | } ~]`.

**The test asserts:** for every byte that T5 accepts, `RegisterSDSStats` returns five **non-nil** counter pointers (⚠️ **assert the POINTERS** — a nil `*stats.Counter` `Inc` is a **process crash** with no `recover()`); and `partial == 0` across the sweep.

⚠️ **`internal/xds/stats_test.go:81` already carries a `bad name!` case that tests the retained guard. It STAYS** — the guard is not being deleted.

**Gate:** `go test ./internal/xds/ -count=1 -v`, census recorded.

---

## Task 8 — G5: the STACKED skip-line invariant

⚠️ **"Zero skips" alone is satisfiable by "nothing registered"** — that is exactly today's ambiguity (§2.6: on `/stats/prometheus` a working registration and a silently-skipped one are **byte-identical, same sha256, 4768 B**). **Stack it:**

1. after a clean SDS boot the aggregated skip line names **ZERO** `sds.` entries, **AND**
2. the **five** names are **PRESENT** on `/stats/prometheus`.

Either leg alone is vacuous; together they pin the invariant. **Non-vacuity of the matcher is provable:** the same file yields **12** `sds`-containing lines, all `envoy_cluster_*{envoy_cluster_name="sds_cluster"}` — the matcher works, the family is absent (`reference_empty_output_is_not_a_zero_result`). ⚠️ **Exclude `sds_cluster` and `ssl_context_update_by_sds` from the matcher** — a loose `sds` match over-counts (measured: 15 flat instead of 14).

**Gate:** the stacked assertion red on a deliberately broken arm (T12 arm ζ) and green on the correct tree.

---

## Task 9 — fixture `0110`: the LABEL-AWARE scrape, the SPLIT-ROSTER assertion, and the PROSE REVERSAL

⚠️ **THE SPLIT ROSTER IS §1.1 AND IT IS THE DIFFERENCE BETWEEN GREEN AND RED ON ARRIVAL.** Five names for name+label; **two** for a value floor.

⚠️ **NOT FREE REUSE.** `0110`'s `scrapeProm` (`:774-833`, 60 lines) keys by metric **NAME** and — per its own doc comment — strips the label set *"ENTIRELY"*. It **structurally cannot** assert `envoy_xds_resource_name`. **It is RETAINED UNCHANGED** for the `ssl.*` leg (which deliberately ignores the address label) and a **sibling** is added. ⚠️ **No reusable label-aware helper exists**: the only one in the harness is `parseMetricLine`/`parseLabels` in `0005-prometheus-stats/driver`, **unexported in a different package**. `0111`'s cross-side prom assertion is a precedent for **the metric NAME being cross-side stable under hoisting** — **not** for asserting a label value; **no in-tree fixture has ever asserted a hoisted label VALUE.** T9 is the first.

**The code — WRITTEN, COMPILED, LINTED AND RUN at this stage** (`gofmt` clean, `go vet` clean, `golangci-lint` exit 0 / 0 bytes, negative-controlled: an injected unused func + a British spelling gave exit 1 with both `unused` and `misspell`). **`git diff --numstat` ⇒ `183 0`; 132 non-blank non-comment.** ⚠️ **The SPEC's ~110 estimate is LOW by 20-65%.** **Import delta +0.** Preserved verbatim at `scratchpad/p2/t10.go.frag`; the load-bearing parts:

```go
// sdsSecretLabel is the label the prometheus exposition hoists the SDS secret
// name into on BOTH sides. MEASURED against the live reference at the phase-80
// PLAN: `envoy_sds_update_success{envoy_xds_resource_name="rccf_validation_ca"} 1`.
const sdsSecretLabel = "envoy_xds_resource_name"

// sdsProjectedNames is the FIVE-name subset envoy-go registers and (post row 80)
// projects. A strict SUBSET of the reference's twelve sds prom families -- the
// other seven are OUT OF SCOPE and must NOT be set-equality asserted.
var sdsProjectedNames = []string{
	"envoy_sds_update_success", "envoy_sds_update_failure", "envoy_sds_update_rejected",
	"envoy_sds_update_attempt", "envoy_sds_init_fetch_timeout",
}

// sdsMovedNames is the sub-subset whose value is >= 1 on BOTH sides. MEASURED
// 4/4 runs: ref attempt=3 success=1 failure=1, subj attempt=1 success=1 failure=0.
// So failure, rejected and init_fetch_timeout are ZERO on the SUBJECT and a
// blanket per-side `>= 1` would be RED ON ARRIVAL.
var sdsMovedNames = []string{"envoy_sds_update_attempt", "envoy_sds_update_success"}

type promSample struct {
	labels map[string]string
	value  float64
}
```

The assertion iterates both sides; per name it requires **presence**, then a sample carrying `sdsSecretLabel == secretName`, then a `>= 1` floor **only** for `sdsMovedNames`. ⚠️ **Values are NEVER compared cross-side** — envoy-go is **initial-fetch-only** and does not hold the stream open, while the reference maintains a long-lived subscription that **re-attempts after `DriveSubject` hard-stops both receivers**. The one `t.Fatalf` fires only when the label-aware scrape returns **zero families**, separating *"the projection did not land"* from *"the scraper broke"*; every per-name assertion is `Errorf`.

⚠️ **THE PROSE REVERSAL IS MANDATORY AND UNGATED** (§1.2). `0110/expectations.yaml:210` and `0110/README.md:178` **declare these counters NOT asserted** and forbid reaching into the `sds.*` scope as *"inherently unstable"*. **Nothing parses `0110`'s `expectations.yaml`, so omitting the flip is SILENT** and the fixture ships contradicting its own recorded boundary. Rewrite both to state that **names and labels are stable while VALUES are not**, and mirror to `0111`'s pair (`expectations.yaml:223`, `README.md:184`) if the assertion is mirrored there.

**RED-first, PROVEN at this stage** — with row 80 unlanded:
```
runner_test.go:1349: subj: envoy_sds_update_success ABSENT from /stats/prometheus (the sds.* projection did not land)
… ×5, all `subj:`, ZERO `ref:` …
--- FAIL: TestDifferential/0110-tls-require-client-cert-false (1.94s)
```
⚠️ **Exactly 5 errors, all `subj:`, zero `ref:` — that DISCRIMINATES.** The reference arm ran and passed all five name+label+floor checks; a broken reference arm would have printed 10.

**Zero registration gates owed** — dir, runner branch and the blank import at `runner_test.go:137` all present, `var _ fixture.StatsAsserter` present.

**Gate:** `go test ./test/differential/ -run 'TestDifferential/0110' -count=1 -v` — ⚠️ **confirm the selector MATCHED** (a no-match prints `[no tests to run]` and **exits 0**) · then the same for `0111` · `gofmt -l` output empty · `golangci-lint` with its own exit code.

---

## Task 10 — `BEHAVIOR_CONTRACT.md`: close the narrowed departure, record the NEW one, and repair FOUR `79.1` references

1. **Close the narrowed departure** at the *"THE DEPARTURE IS NARROWED, NOT ELIMINATED"* paragraph — 20 names go from dropped to projected. The four-family decomposition **20 + 4 + 4 + 2 = 30** is the contract's own and is correct. ⚠️ **Do NOT inherit *"20 of 30"* from `ROADMAP.md`'s row-79 cell**, which says *"THIRTY names across SIX families"* then enumerates **four** summing to **26**.
2. **Repair the `79.1` forward references — FOUR occurrences on THREE lines** (§1.9), **anchored on the PHRASE, never the line**:
   - `:5078` *"DEFERS the `sds.` twenty to row 79.1"*
   - `:5078` *"Until 79.1 lands, treat `sds.*` as flat-`/stats`-only"*
   - `:5080` *"79.1 takes it to 0 lines"*
   - ⚠️ **`:158` *"That asymmetry, not scope, is why the `sds.` family is deferred to row 79.1"* — UNNAMED BY THE SPEC.**
   ⚠️ `ROADMAP.md` row 79's three occurrences are **INVARIANT-BLOCKED** and named as a permanent discrepancy; the phase-79 phase documents are **historical and must NOT be rewritten**.
3. **Rewrite the five-counter subset paragraph**, which says the secret-name segment *"is guarded by `stats.IsValidName` before registration"* — **true today, false after this row.**
4. **Record the NEW DEPARTURE** (§1.5 / T5): envoy-go **boot-fails** where **the reference boots green and serves the counters with the name hoisted verbatim into the label value**. **Quote the reference behaviour. State it as a departure, not a fix.**
5. **`TWELVE` → `THIRTEEN`** at `:162` and **the live half of `:167` only** (§1.11).
6. ⚠️ **`:501` calls the `wasm.` arm a *"NEW SN9 flattening rule"*, which COLLIDES with ADR-0118's actual SN9.** Pre-existing; **named, not fixed.**

⚠️ **Pre-79 line cites are stale by `+54` only up to old line ~5023; a second hunk adds six more, so cites at or beyond old `:5024` are stale by `+60`.** A flat `+54` lands six lines short in the tail. **Prefer symbol anchors to any offset.**

**Gate:** the four `79.1` phrases ⇒ **0** post-edit, each negative-controlled on a doctored copy · `git grep -c '79\.1' -- docs/envoy-go/BEHAVIOR_CONTRACT.md` ⇒ **0**.

---

## Task 11 — the break roster, each arm proven to fire its OWN assertion

⚠️ **The BRAINSTORM omits this entirely; all four calibration PLANs carry one (4/4)** — 76 T8 · 77 T8 · 78 T6 (eight arms) · 79 T10. ⚠️ **Breaks run AFTER committing**, with **`-count=1`**. ⚠️ **PLAN break instructions don't compile** — substitute an equivalent and **record the substitution**.

⚠️ **THIS ROW'S BREAKS ARE UNUSUALLY HAZARDOUS BECAUSE THE TREE IS NEARLY BLIND TO THE ARM** (§1.15: the projection arm fails **exactly one** existing test). **A break arm that "fires" may be firing an unrelated assertion. Each arm must name WHICH assertion fired.**

| arm | edit | must fire |
|---|---|---|
| α | delete the `sds.` arm | T1 rows + G6; **T4 all four goldens**; T9 `0110` |
| β | `sds.` arm uses **last-dot** (`strings.LastIndex`) | ⚠️ **G6 ONLY** — the decisive arm. T9 stays GREEN (all four corpus secrets are dot-free), which is §2.8 demonstrated rather than argued |
| γ | bump the constant to `13` with **no** arm | `TestTerminalError_TopLevelCountMatchesCode` (`claims 13 … has 12`, naming `[sds.]`) **AND** `TestExtractTagsTerminalError_ByteStable` **AND** `TestTerminalError_NamedRootsAreAccepted` — **three tests** |
| δ | add the arm, leave the constant at `12` | `TestTerminalError_TopLevelCountMatchesCode` only (`claims 12 … has 13`) |
| ε | delete one `helpText` entry | T3 `TestHelpText_KeySetExact` **AND** `TestHelpText_NoSelfEqualHelp` |
| ζ | typo a `helpText` **value** to equal its key | ⚠️ **`NoSelfEqualHelp` ONLY — `KeySetExact` is BLIND.** (Phase 79 proved the *key*-typo variant is caught by both, refuting its own "decisive row"; the **value** defect is the one that discriminates) |
| η | move T5's reject **after** the dial | ⚠️ **G4's positive arm ONLY**, and it fires with the **128 B dial** message instead of the 179 B name message — §1.5 as an executable arm |
| θ | drop T5's **segment** leg, keep the charset leg | G4's segment arm only (`trailing_dot.`, `.lead`, `a..b`) |
| ι | `goldenRoster` entry added, `goldenTaggedIdx` **not** updated | T4 — names which of the four goldens fires |
| κ | run T4's OTLP golden on the **`(F,F)` cell only**, roster **un-extended** | ⚠️ **NOTHING fires — the shape-SEVENTEEN vacuity demonstration.** Record it whether or not it ships |
| λ | T9 asserts a blanket `>= 1` on all five names | ⚠️ **T9 reds on THREE names on the SUBJECT side** — §1.1 as an executable arm |

⚠️ **Arms κ and λ are NEGATIVE results and the two most valuable lines here** — κ shows a natural gate configuration that **cannot fail**; λ shows the SPEC's own verdict failing. ⚠️ **Vary the injection site** where an arm admits more than one (`reference_break_arm_injection_site_is_a_claim`), and beware **ordered legs tripping an EARLIER leg** (`reference_vacuous_break_modes`).

**Gate:** each arm red on its **named** assertions (**confirm WHICH fired**), restore green.

---

## Task 12 — the gates

⚠️ **THE FULL 120-FIXTURE DIFFERENTIAL IS MANDATORY** for any row touching `internal/stats` — it links into `cmd/envoy-go` at `test/differential/harness.go:240`, `:594` **and** `test/conformance/h2spec/h2spec_test.go:210`. ⚠️ **h2spec is a FOURTH consumer NOT covered by `./test/differential/`** — run it **explicitly**. Budget **~400-430 s** per green attempt (measured at `f2dd994a`: **403.2 s** locally, **427.8 s** in CI); **`-race` is a SECOND run, not a substitute.**

```sh
( go test ./test/differential/ -count=1 -v > "$SCRATCH/full.log" 2>&1; echo "INNER_EXIT=$?" )
grep -cE '^    --- PASS: TestDifferential/' "$SCRATCH/full.log"          # want 120
grep -E  '^    --- (FAIL|SKIP): TestDifferential/' "$SCRATCH/full.log"   # want EMPTY
grep -c  'no driver registered for fixture' "$SCRATCH/full.log"          # want 0
grep -o  'TestDifferential/[^ ]*' "$SCRATCH/full.log" | sed 's|TestDifferential/||' | sort -u \
  | comm -3 - <(ls -1 test/fixtures/ | grep -E '^[0-9]{4}[a-z]?-' | sort)  # want EMPTY
```

**Every clause is grounded in a recorded failure mode:** `-count=1` · `-v` (**without it a green log cannot be distinguished from a suite that ran nothing**) · the **INNER** exit code · the tally scoped to `TestDifferential/` so the bare parent line is excluded · **`comm -3` is the load-bearing gate, not the raw count** (120 with one fixture renamed and another skipped still reads 120) · `SKIP` asserted empty, since `t.Skipf` on an unregistered driver is the silent-green path. ⚠️ The faithful dir predicate is **`^[0-9]{4}[a-z]?-`**; a bare `^[0-9]{4}-` gives **118**. ⚠️ **`go test ./test/differential/...` (with `...`) matches TWO packages and BUFFERS `-v` output per package** — a run that looks hung for 7 minutes with an empty log is normal.

⚠️ **CI now runs this job at `-timeout 20m` with job `timeout-minutes: 30`.** If a local run approaches either, **that is a finding, not a budget to raise reflexively.**

**Plus:** `gofmt -l` **output** empty · `golangci-lint run` on the **six** touched packages (`internal/stats`, `internal/statssink`, `internal/boot`, `internal/xds`, `internal/admin`, the `0110` driver) with its **own** exit code and byte count · `go test ./internal/... -count=1` · `go test ./internal/stats/... ./internal/statssink/... ./internal/boot/... ./internal/xds/... -count=1 -race` · `go vet` · `git diff go.mod go.sum` **0 bytes**.

**Stat surface: assert the DELTA, expected +0.** The row **projects** existing names; it registers none. ⚠️ **1207 is DOCUMENTARY and CONFIG-CONDITIONAL — never assert the absolute.** ⚠️ **And the `TestNoNewStat*` guards are PROVEN BLIND to `internal/stats`** (all five live in `internal/statssink/registration_test.go` and none reaches it): **enumerate the diff's registration call sites and show the set is EMPTY.** Adding a `switch` arm is a **PROJECTION** change, not a **REGISTRATION** change.

⚠️ **Known live hazards — never reflex-classify any as a regression; isolate-re-run, then state the classification AND its evidence.**
- **`reference_sds_init_fetch_timeout_dial_budget_flake` — LIVE for this row's subject, in TWO packages.** It did **not** fire at the SPEC or at this stage.
- the pre-existing `internal/cluster` `-race` outlier (`TestOutlierDetector_ConcurrentEjectExactlyOnce`) · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`.
- ⚠️ **TWO former flakes were FIXED at `f46ba419`/`f2dd994a` and a recurrence of either is now a FINDING, not a re-run:** `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery` (was a whole-loop read deadline acting as a throughput assertion) and **the full-suite `bind: address already in use` BACKEND half** (now banded `11000..14999` + wildcard probe). **An in-band backend bind failure is a real finding.** The `subject ready: EOF` half was hardened at `0e9cc680` and likewise warrants suspicion, not acceptance.
- ⚠️ **`0061-lb-ring-hash` is NOT a live flake — a spread failure there is a FINDING.**

⚠️ **Gate hygiene — the lineage's broken-gate count is EIGHTEEN** (§8).

---

## Task 13 — ADR-0302, the ADR-0299 rider, row 80 → `done`, the sentinel, and the stage close

1. **ADR-0302 §Decision + §Consequences appended IN PLACE** after the retained footer (**ADR-0044-as-used**; ⚠️ ADR-0044 does not itself contain that discipline). **No renumber, no `---` separator.** **AND FLIP ADR-0302's STATUS `PROPOSED` → `COMPLETE`.**
   ⚠️ **RETAIN the italic footer** — `*(§Decision + §Consequences land at the phase-80 IMPL.)*`, byte-verified at `DECISIONS.md:17692`, the last line of §Context; `od`-confirmed U+00A7 (`302 247`), no `**`, newline-terminated, and **all 8 footers in the file normalize to ONE string**. ADR-0301 omitted it and forced phase 79 into a recorded departure.
   ⚠️ **The footer-carrying block is `ADR-0294 … ADR-0300` — SEVEN** (per-block scan: 0293 = 0, 0294-0300 = 1 each, **0301 = 0**, 0302 = 1). **ADR-0301's own STATUS says *"seven blocks"* then names the SIX-block range *"ADR-0295 through ADR-0300"*. Copy the FORM, not the miscount.**
   ⚠️ **Carry NO whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297.
   ⚠️ **CORRECT ADR-0302 §Context ¶11's *"forty-five files"* to 43** (§1.12) — landable precisely because the STATUS is still `PROPOSED`.
2. **The ADR-0299 rider:** STATUS `PROPOSED` → `COMPLETE` (`:17466`); its §Decision/§Consequences landed at the phase-77 IMPL. **The single word is the entire defect.** Guard shape: *"a `PROPOSED` STATUS must have no §Decision."* ⚠️ **The guard must tolerate the in-flight case or run AFTER both flips** — ADR-0302 is legitimately `PROPOSED` until step 1 completes.
3. **Row 80 → `done`** in `ROADMAP.md`. **`want` STAYS 112** — this row adds no row.
4. ⚠️ **THE LEAK CHECK RE-ARMS HERE.** The new cell must contain **ZERO** occurrences of the two deferred-candidate phrases and **no unregistered `<Family>-family row` slug**, each grep negative-controlled against a **deliberately doctored copy**. ⚠️ **`grep` cannot tell a mention from a use** — the leak is **proven to silence check (3) BY MENTION** (doctoring a cell to say `gRPC-family row` made `gRPC` stop printing while `WASM` kept printing).
   ```sh
   awk -F'|' '/^\| *80 /' docs/envoy-go/ROADMAP.md > "$SCRATCH/row80.txt"
   grep -coE 'deferred candidates:|remaining deferred \(not-yet-chartered\) candidates:' "$SCRATCH/row80.txt"  # want 0
   grep -oiE '(HTTP-filters|Network-filters|Load-balancing|Upstream-robustness|Observability|Operational-tooling|HTTP/3|gRPC|xDS|Runtime|WASM)-family row' "$SCRATCH/row80.txt" | sort -u
   ```
   ⚠️ **Row 80's cell must also be WELL-FORMED** — rows 57, 69 and 78 all lose summary content in GFM render, and row 78's `NF==8` **passes** because an unescaped inner `|` and a missing trailing `|` **cancel** (broken-gate shape sixteen). **Escape inner pipes and terminate with `|`.**
5. **Sentinel re-run AFTER the ROADMAP edit**, all three checks, with **firing** negative controls. ⚠️ **This row narrows NOTHING** — check (2) has **never** gone down across ~26 phases. **State it; do not forecast a decrease** (the phase-73 error).
6. **`STATE.md` §Current rolled IN PLACE** (ADR-0288 — never prepend). ⚠️ **§Recent is at its five-entry cap: evict the OLDEST to `STATE_HISTORY.md` VERBATIM.** At this tip the oldest is **`phase 79 (stats-prometheus-projection) BRAINSTORM done`** (`STATE.md:54`, 1722 bytes) — **verified ABSENT from the archive (0) with a FIRING NC on the phase-78 BRAINSTORM sibling (1)**. Roll `next-prompt.txt`. ⚠️ **Anchor §Current greps on `^- **<field>`**; all seven fields verified singleton, NC both ways (`- **zzz-nonexistent:` ⇒ 0; `- **prior active-phase:` ⇒ 5). ⚠️ **§Project counts SELF-CONTRADICTS §Current and is FROZEN at the phase-76 IMPL close** (`:31` 119 vs **120**, `:33` 1205 vs **1207**, `:35` ADR-0298 vs **ADR-0302**) — **anchor on §Current, which IS live.**
7. **The six-gate:** `BOOTSTRAP_PROMPT.md` **§7.5** — heading **`:357`**, gates (a)-(f) **`:360-365`**, closing sentence **`:367`** — at the **repo root**, with the `.md` extension. ⚠️ **`ADR-0045` QUOTES the split gate rather than stating it; `ADR-0106` defines nothing about phase-done gates.** ⚠️ **The normative split statement is §6.1, heading anchor `### 6.1 When to split`** (`:285`), **not `:287-291`** — `:291` is blank and the third trigger is at `:292`.

---

## 4. Band — **11-14, budget 13**, and the LoC estimate is REFUTED on its BASIS, not its magnitude

**Enumerated: 13 tasks** (§1.4 — the SPEC's T4/T5 is one forced-atomic task, so its 14 double-counts).

**FLOOR 11 — what collapses:** T7 folds into T6 (both are reject gates; the fold requires a deliberate cross-package placement decision — T6's arms live in `internal/boot/boot_test.go`, T7's subject in `internal/xds` with 25 `_test.go` occurrences — so it is a decision, not a merge of neighbours) and T8 folds into T12. **Below 11 is indefensible:** T12 and T13 cannot merge (the ROADMAP flip **re-arms** `reference_sentinel_matcher_string_self_clears`, making the sentinel a strictly-ordered *second* gate pass), and T1/T2/T3/T4/T5/T9/T10/T11 are eight independent seams in eight file sets with eight different gate commands.

**CEILING 14 — one mechanism, and it is NOT the SPEC's:** **T9 splits** (the 183-line label-aware scraper as leg a; the split-roster assertion plus the three-document prose reversal as leg b). ⚠️ **T4 must NOT split** (§1.4) — every sub-split leaves the tree red between legs, which is why the SPEC's ceiling mechanism is refuted rather than adopted.

**Calibration, re-derived from primary documents — 4/4 cells confirmed, none interpolated:**

| phase | SPEC band, quoted from its own heading | PLAN count (`grep -c '^## Task [0-9]'`) | position |
|---|---|---|---|
| 76 | `**~7-9 tasks**` (`SPEC.md:355`) | **9** | AT ceiling |
| 77 | `**11-13 tasks; a SINGLE FLAT ROW**` (`:467`) | **12** | INSIDE |
| 78 | `**7–9 tasks; a SINGLE FLAT ROW**` (`:307`) | **10** | **ABOVE** |
| 79 | `**THE BAND IS 10-12, BUDGET 12**` (`:279`) | **12** | AT ceiling |

**Three of four at or above the ceiling, none below the floor** ⇒ a budget of 13 in an 11-14 band reads as **"expect 14"**. All four PLAN squashes carry **0 `.go` files** (4/4 verified), so **the IMPL squash IS the row's code**.

**LoC — bottom-up, on the calibration numerator (`.go` insertions): ~700 (~640 net)** = production **~86** · test **~457** · fixture **~154** (P2's executed `183`/132 lands inside this), plus ~350 non-`.go` the ratio numerator excludes.

⚠️ **The SPEC's ~855 total is roughly right and EVERY BUCKET IS WRONG; the errors cancel.** Its production **55** omits T5's reject entirely (~35 lines) and should be ~90-130 (phase 79 realized **129** across three production files for a comparable surface). Its test **690** over-projects from a non-analogous base: phase 79's 1208 test lines were **89% NEW FILES** (segmentcount 348 + golden 435 + helptext 188 + promskip 110 = 1081/1208), whereas **phase 80 creates ZERO new test files** — every gate extends an existing one; ~450 is right. Its fixture **110** should be ~155-195 (phase 79 realized **195** for the same species of conversion on `0118`). ⇒ ~1.6× low, ~1.5× high, ~1.8× low. **A total-only sanity check passes while no bucket is defensible.**

**ADR-0045 / §6.1 do NOT trip:** 13 ≪ ~25 (~1.9× margin) · **~640 net `.go` < ~1500 (~2.3× margin)**. ⚠️ **The THIRD trigger (mid-execution, >~10 sub-steps) IS live on T4** and is written into T4's own gate. ⚠️ **§9.3's "ON the ~1500 line" is arithmetic on mismatched bases (§1.3), not a derived result** — and the two nearest comparables realized net **1406** and **1446** without splitting.

⚠️ **NO SPLIT. And the SPEC's T4/T10 seam is incoherent** (no shared file, package or dependency — two unrelated leaves, not §6.2's *"coherent slice"*). Were one ever needed the seam is **projection leg (T1-T4, T9) / validation leg (T5-T8)**.

---

## 5. Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

`ls stop` ⇒ `No such file or directory`. **It must not be created.**

| check | ACTUAL output at `47b9b378` | negative control, observed FIRING |
|---|---|---|
| **(1)** `want=112` | **`NOT DONE: row 80`** — correct, row 80 is `in-progress` | `want=111` ⇒ `GATE FAIL: examined 112 data rows, expected 111`; row **76** doctored to `in-progress` on a scratch copy ⇒ `NC NOT DONE: row 76` **alongside** row 80 (the script self-reported `rows doctored: 1`) |
| **(2)** | **FIVE — `:190 :200 :210 :216 :224`** (long form ×4, Operational-tooling short form at `:224`) | union **5** vs one-arm-stripped **4** — ⚠️ **5 → 4, NOT 5 → 0**; independently confirmed that the long form does **not** contain the short substring (`printf … \| grep -c 'deferred candidates:'` ⇒ **0**) |
| **(3)** | **`NEVER OPENED: gRPC`, `NEVER OPENED: WASM`** | invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`, while the REGISTERED slug `Observability` correctly printed **nothing** ⇒ the loop discriminates, it does not merely print |

Input measured at **228 lines / 112 data rows / 13** bare `candidates:` hits (against the sentinel's narrower 5), so an empty result could not have read as a zero result.

⚠️ **CHECK (2) IS UNCHANGED AND THIS ROW NARROWS NOTHING — STATED, NOT FORECAST. The twenty-sixth consecutive phase at which it did not go down.** This row's subject is the residue of a closed row's departure and is drawn from no candidate paragraph.

⚠️ **`want` STAYS 112.** ⚠️ **The LEAK CHECK is NOT live this stage** — `ROADMAP.md` is byte-untouched. **It re-arms at the IMPL** (T13.4).

---

## 6. Counts at this tip — each verified, mismatches flagged

| axis | value | note |
|---|---|---|
| fixtures | **120** (next-free `0119`) | ⚠️ predicate `^[0-9]{4}[a-z]?-`; bare `^[0-9]{4}-` ⇒ **118** (`0007a`, `0007b`) |
| fuzzers | **55** | ⚠️ `internal/`-scoped and repo-wide **both 55** — NOT discriminating; zero `^func Fuzz` outside `internal/` |
| internal packages | **73** | `go list` and `find` agree |
| `runner_test.go` blank imports | **120** | ⚠️ FULL prefix `^\t_ "github.com/pgdad/envoy-go/test/fixtures/`; naive `^\t_ ` ⇒ **126** (2 filter imports + 4 `_ = …`). ⚠️ **`\t` in GNU ERE is a literal `t`** — use `-P` |
| `BackendKind` tail | **38** | ⚠️ a TAIL VALUE — **39** declarations, values 0-38 contiguous, **no `iota`**; do NOT "fix" to 39 |
| `DECISIONS.md` | **17692** lines, **301** `^## ADR-` headings, tail **ADR-0302 PROPOSED**, next-free **ADR-0303** | ⚠️ **SPEC §8 says 300 / tail ADR-0301 — stale by the SPEC's own commit.** `^## ADR-0303` ⇒ 0, NC `^## ADR-0302` ⇒ 1. ids 0001-0302, **exactly one gap at ADR-0209**, zero duplicates |
| `ROADMAP.md` | **228** lines / **112** data rows | |
| `BEHAVIOR_CONTRACT.md` | **5822** | |
| `STATE_HISTORY.md` | **426** | 161 `prior active-phase` bullets |
| next-free reference port | **10119** | `10118` ⇒ 3 files in `0118`; `10119` ⇒ EMPTY |
| top-level detectors | **12** code / **12** error string — **AGREE** | `HasPrefix` ×8 + `CutPrefix` ×4; **row 80 ⇒ 13, and the error string moves in the SAME task** |
| mid-name detectors | **4** / **4** — AGREE | **UNCHANGED by this row** |
| `helpText` / `helpTextRoster` | **25** / **25** | both ⇒ 30; ⚠️ `name.go:490` says *"Of the 25 entries"* — **a live stale count T3 must move** |
| `goldenRoster` | **13** entries, **ZERO** `sds.*` (whole-file `sds` ⇒ 0) | `goldenTaggedIdx = {0,1}`; `goldenRegistry` comment says *"13-entry"* |
| `goldenOTLPCells` | **1134 / 1200 / 1118 / 1184** | ⚠️ **RE-MEASURE; do not compute** |
| stat surface | **1207** | ⚠️ DOCUMENTARY + CONFIG-CONDITIONAL — **assert the DELTA**, expected +0 |
| go.mod modules | **2** (phase-61.2 lineage figure) | ⚠️ NOT a repo total — the single `go.mod` requires **67**; do NOT "fix" 2 to 67 |
| `79.1` occurrences | **61 / 10 files** | ⚠️ SPEC says 50/9 — exact at ITS tip, stale here. **live-normative 4 on 3 lines** (§1.9) |
| stale-`TWELVE` live set | **16 occurrences / 10 files** | ⚠️ SPEC says "16 lines / 8 files"; its own items 8-14 sum to 13/9 |

---

## 7. Deferred — named so no later stage re-derives them

1. **Full hyphen fidelity.** The reference serves `envoy_sds_update_success{envoy_xds_resource_name="server-cert"}`; envoy-go will boot-fail. Needs either relaxing `NamePattern` (a `checkName`-panics invariant, repo-wide) or carrying raw operator bytes alongside the sanitized registry key through the `Registry`/`ExtractTags` seam. **Chartered, not taken.**
2. **The general dynamic-token charset exposure.** Six families assemble stat names from operator input; five REJECT and only `sds.` skipped. ⚠️ **This row does NOT audit whether the five existing rejects are complete**, nor whether wire-derived segments (mongo collection names, wasm plugin names) are reachable with an invalid name.
3. **The `# HELP` format departure** — the reference emits **zero**; envoy-go emits them per family. Pre-existing, independent of this row.
4. **`--mode validate` cannot validate ANY SDS bootstrap** — `validate.Bootstrap` passes a nil provider, so `NewSDSProvider` is never reached.
5. **The `listener.`/`stat_prefix` sanitization inconsistency** — ADR-0065 §Context rejected sanitizing a hoisted label value as *"a silent data-loss bug"*, yet `normalizeAddr` sanitizes the `listener.` address, which SN3 hoists. **The precedent set is internally inconsistent.**
6. ⚠️ **`DECISIONS.md:17634` (ADR-0301 §Decision (b))** — *"has failed twelve top-level detectors"*, **present tense inside immutable ADR text**, goes factually wrong the moment row 80 lands. **A named permanent discrepancy**, alongside `ROADMAP.md`'s five (four of which are false positives anyway).
7. **The `STATE_HISTORY.md` archive gap** — 36 bullets for phases 67-75, 58 overall. ⚠️ **`phase 77 PLAN done` is in NEITHER file**, re-confirmed with a firing NC (IMPL/SPEC/BRAINSTORM siblings all match 1 under the identical matcher; PLAN matches 0), and the archive demonstrably carries PLAN entries for 78, 63, 62, 60.1, 56.1, 46.2, 42.2b, 42.2a, 42.1. **4-6 tasks, defensible ONLY with a `comm -13` set-difference recurrence guard.**
8. **`ROADMAP.md` malformed rows 57/69/78** — invariant-blocked ⇒ **defensible as a GUARD, not a fix**, and the gate must be a **DISJUNCTION** (escape-aware `NF!=8` finds 57/69; escape-aware trailing-piece finds 78; **`NF==8` PASSES row 78**).
9. **`STATE.md` §Project** — stale on three axes, frozen at the phase-76 IMPL close. **Anchor on §Current.** Repairing a count by editing the sentence that states it is how the ADR-0296/0297 species starts.
10. ⚠️ **SYMMETRIC BIND HARDENING — the ALLOCATOR half LANDED at `f2dd994a`** (band `11000..14999` + wildcard probe + a pinning test with a firing NC). ⚠️ **The old "8-12 tasks" figure and the "close-then-rebind" characterization are STALE — do not inherit either.** What REMAINS is the **retry** gap: a window persists between the probe's `Close` and the child's bind, and the child is `go run`, so the gap spans a **BUILD**. **26 of the 29 `freeTCPPort(t)` call sites in `runner_test.go` are backend-startup arms with NO retry loop** (`harness_test.go` holds 4 more). ⚠️ **Still NOT verifiable by a single green suite run** — a prior instance needed `-count=6`.
11. **Opening the gRPC family — HARD-BLOCKED, evidence-backed:** `\.RunEncodeTrailers(` ⇒ **0** non-test / 1 test; `\.RunDecodeTrailers(` ⇒ **0** / 1; both declared, neither production-reachable. NC: `\.RunEncodeHeaders(` non-test ⇒ **4**. The **16-22+** band is **NOT re-derivable** — flagged, not inherited.
12. **The WASM row-summary rider stays DECLINED ON THE MERITS** — `ROADMAP.md:76` says *"phase 25 is the FINAL §9 HTTP-filters-family row"*, **and writing the marker would silence check (3) BY MENTION** (proven by NC).
13. **The PUBLIC IMPORT PATH defect — NOT DEFENSIBLE AS A PHASE: there is no green.** `DECISIONS.md:142` is an immutable ADR that must *name* the wrong path; 7 hits sit in CLOSED rows' summary cells; the root `PROGRESS.md` hits are pasted `go test` output; any `count==0` guard is unsatisfiable by construction. **0** occurrences in compiled Go.
14. **A mechanical stat-surface recount** — 8-11 tasks, defensible only as an **EXECUTABLE enumerator** (dump `Registry` keys after a boot). A grep-based recount covers ~0.8 % of the surface and stays rejected.
15. **Normalizing the Operational-tooling short-form deferred-candidate paragraph** to the long form.
16. **`BEHAVIOR_CONTRACT.md:501`'s SN9 collision** with ADR-0118's actual SN9.
17. **`golangci-lint-action@v6.5.2`'s Node-20 deprecation warning.** Deliberately NOT bumped: v7+ requires golangci-lint **v2** while the repo pins **v1.64.8**, so a bump forces a config migration. **A warning, not a failure.**
18. ⚠️ **`ROADMAP.md` §Schema `:18` (*"Sub-phases get their own rows"*) vs BRAINSTORM §1.1 claim 3** (the legs live in the parent's `sub-phases` PROSE). **Practice diverging from written doctrine.** The row-id adjudication (80, not `79.1`) survives on claims 1 and 2, which are independent and both hold. **Record the tension; do NOT relitigate the id.**

---

## 8. Gate hygiene — the lineage's broken-gate count is **EIGHTEEN**

The seventeen carried forward: two defects that **CANCEL** in the gate metric (`NF==8` passes the malformed row 78) · an **inert gate cell** · a full-suite recipe **without `-v`** is VACUOUS · a sha256 roster **desynced against a DELETED file** · **`gofmt -l` NEVER exits non-zero** (gate on OUTPUT) · `go doc -all <A> <B>` **swallows arg2** · a **`+0 exported symbols`** gate over an EMPTY package **reds on a CORRECT tree** · a **RANGE** gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` **exits 0 on a DELETED file** · a **count-only** stat guard **PASSES a build with BOTH names wrong** · a **`-run` no-match exits 0** with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests **exceeds the fixture denominator** · a stat-delta claim **cannot be discharged by guards scoped to another package** · a **stderr-VOLUME** assertion **passes on the hang** · **`golangci-lint` runs `misspell` with `locale: US`** · **a harness's exit code is not the command's** · **a GOLDEN ROSTER that omits the family under test, making EVERY cell of an otherwise-correct cross-product vacuous** (shape seventeen — live on T4).

⚠️ **AND AN EIGHTEENTH, FOUND AT THIS STAGE: A NEGATIVE CONTROL POINTED AT A TARGET THAT DOES NOT EXIST.** `go list -deps ./internal/conv` **exits 1** with `directory not found` and prints nothing; the resulting **`0` hits reads exactly like "the NC discriminates."** `internal/conv` has **0 tracked files**. **This is distinct from every prior entry — the command is right, the assertion is right, and the NC's SUBJECT is a phantom.** ⚠️ **It fired TWICE inside SPEC §4.1** (the original NC was vacuous, and the sentence announcing the re-run introduced a second vacuous one), and a probe agent nearly re-inherited it at this stage. **Remedy: an NC must assert its own target EXISTS and its command EXITED ZERO before its empty output is read as a result** — `reference_empty_output_is_not_a_zero_result` applied to the control rather than the measurement. A real NC (`./internal/clock`: exit 0, 47 deps, 0 hits) discriminates.

---

## 9. Self-review against the SPEC

**Adopted unchanged:** D-SDS-1 in full (SN1-shaped, **first-dot**, single-segment, non-greedy, `envoy_xds_resource_name`, no pre-transform in the arm, placed before `wasm.`) and its reference pinning · D-SDS-2's **boot-boundary reject** and the ADR-0065 §Consequences (e) grounding · the rejection of sanitization (foreclosed by ADR-0065 §Context) · **both deadlock-prone designs excluded**, and the skip-counter rejected on the additional circularity ground · `RegisterSDSStats`'s guard and `incNil` **STAY** · the mandatory `helpText`+`helpTextRoster` extension · the golden roster extension with **re-measured** pins · the name+label-shape **subset** posture (never set-equality, never value-parity) · **G6 as mandatory** because the fixture cannot discriminate the fork · the full-differential mandate · the §6.1 heading anchor and the `:287-292` correction · the ADR-0294…0300 SEVEN-block count.

**Refuted or corrected (nineteen):** §7.2's `>= 1` verdict → **RED on 3 of 5** (§1.1) · §7.2's cross-side hazard enumeration, wrong in both directions · §7.2's reference `update_attempt` **2 → 3** · §7.2's roster **10 flat / 9 prom → 14 / 12** · §7.1's silence on `0110`'s own **prohibition** and its three unflipped doc sites (§1.2) · §7.1's `0111` "label-bearing precedent" → **label-DISCARDING** · §9's *"T5 folds into T4"* → **a forced atomic merge, count 13 not 14** (§1.4) · §9's *"ceiling 15 if T4 splits"* → **T4 must not split** · §9.3's multiplier **basis mismatch** and its *"ON the ~1500 line"* conclusion (§1.3) · §9.3's implied split → **does not fire**, and its **T4/T10 seam is incoherent** · §9/§11's **phantom "§6.1 item 15"** · §4's site → **pinned before the dial** (§1.5) · §4.2's ADR-0065(b) table cell → **inverted** (§1.6) · §4.2's segment predicate → **incomplete, passes `""`** (§1.7) · §4.1's package-edge NC → **nonexistent target, twice**, and the **wrong package** checked (§1.8) · §8 item 2 → **three phrases for four occurrences** (§1.9) · §6.1 items 9 and 13 → **HISTORICAL, must not be edited** (§1.10) · §6.1's *"8 files / 16 lines"* → **16 / 10**, and its needle figures stale or wrong at their own base (§1.13/§1.14) · §8 item 7's entire **ADR block**, stale by the SPEC's own commit (§1.13) · the fixture-corpus denominator (**5 fixtures / 25 registrations**, not 4/20) (§1.16). **Plus §5.2's no-self-equal leg and T10's import delta, both FREE** (§1.17).

**Added, that no phase-80 document carries (five):** the reject's **own** blast radius (**124 packages green** — a different measurement from §5.4's "one failing test") · the reject's coverage of the **VC-SDS** shape used by 4 of 5 fixtures · **`name.go:490`'s stale "25 entries"** and its `13` grep-collision (§1.17) · the **uppercase-`TWELVE`** and **two-verdicts-on-one-line** sweep traps (§1.11) · **broken-gate shape EIGHTEEN** (§8). **Plus one outside the row's subject:** the *"forty-five files"* figure already baked into ADR-0302 ¶11, and landable there (§1.12).

**Spec requirements with no task — none.** §1 items 1-8 map to T1/T5 · T2 · T3 · T4 · T1 · T9 · T10 · T13.

---

## 10. Operative memories

`feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `feedback_push_to_origin` · `feedback_pertask_gofmt_lint` · `feedback_subagent_autocommit_claudemd` · `feedback_brief_citations_not_evidence` · `feedback_parallel_stream_mints_fresh_drift` · `reference_bash_cwd_reset_commits_to_main` · `reference_parallel_subagents_private_scratch` · `reference_parallel_agents_shared_machine_namespaces` · `reference_break_protocol_commit_first` · `reference_differential_break_protocol_count1` · `reference_deliberate_break_wrong_assertion` · `reference_break_arm_injection_site_is_a_claim` · `reference_plan_break_instructions_dont_compile` · `reference_vacuous_break_modes` · `reference_liveness_break_needs_failing_baseline` · `reference_positive_arm_cannot_catch_overfiring` · `reference_probe_must_discriminate` · `reference_probe_input_is_a_claim` · `reference_independent_probes_can_share_a_blind_axis` · `reference_empty_output_is_not_a_zero_result` · `reference_output_volume_is_not_output_content` · `reference_sample_is_not_an_audit` · `reference_stat_count_guard_blind_to_rename` · `reference_gate_command_negative_control` · `reference_fatalf_makes_assertions_unreachable` · `reference_a_drift_correction_is_itself_a_claim` · `reference_refutation_must_answer_the_claim_as_stated` · `reference_stale_cite_recurs_fix_by_pattern` · `reference_branchpoint_roster_stale_midrow` · `reference_handwritten_golden_shares_author_mistake` · `reference_golden_roster_omits_family_under_test` · `reference_verification_table_launders_wrong_cites` · `reference_document_hygiene_claim_not_evidence` · `reference_sentinel_matcher_string_self_clears` · `reference_compensating_defects_cancel_in_the_gate_metric` · `reference_deferred_candidate_cost_restale` · `reference_golangci_misspell_locale_us` · `reference_registry_walk_lock_inversion` · `reference_nil_stats_counter_inc_crashes_goroutine` · `reference_dynamic_stat_name_charset_guard` · `reference_differential_run_selector` · `reference_differential_fullsuite_startup_flake` · `reference_differential_fixture_three_registration_gates` · `reference_listener_stat_scope_cross_side_divergence` · `reference_dogstatsd_tag_order_unsorted` · `reference_stats_sink_emits_used_only` · `reference_sds_init_fetch_timeout_dial_budget_flake` · `reference_sds_fetchfail_posture_init_hold` · `reference_harness_exit_code_is_not_command_exit_code` · `reference_timeout_exit_124_shared_by_healthy_and_hung` · `reference_recursive_grep_blind_to_gitignored_tracked_file` · `reference_next_prompt_tracked_despite_gitignore` · `reference_pgv_forecloses_go_hazard` · `reference_roadmap_split_phase_row_done`
