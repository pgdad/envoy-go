# PLAN 79 — `/stats/prometheus` PROJECTION completeness: **the row's OWN fix is incomplete in exactly the way the defect it repairs is**, and the sink gate has a cell that CANNOT FAIL

**Stage:** PLAN (lifecycle-state `2` → 3). Row 79 stays `in-progress`; `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` are **BYTE-UNTOUCHED** at this stage. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master `9b2ef891`. Worktree `/home/esa/git/envoy-go-wt-p79plan`, branch `phase79-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + **`STATE_HISTORY.md`** + `next-prompt.txt`. ⚠️ **FIVE files, and that is ONE MORE than the phase-76/77/78 PLAN precedent** — each of those three was exactly four and **none** touched `STATE_HISTORY.md` (`git show --stat` on `9b0077b9`, `acedfd2b`, `62588ade`). The reason is §1.4: §Recent lineage is AT its five-entry cap and the bullet this stage evicts has never been archived. **Do not write "four files, matching the precedent" — that sentence would be falsified by the commit carrying it, which is exactly what happened at the phase-79 SPEC (§14, third self-correction).**

**Method:** four investigation agents on disjoint remits, each in its own detached worktree with private scratch and a private port band (A2 41000-41099, A4 41200-41299 — both **outside** the differential harness's 20000-31007 allocation range), plus controller re-derivation. **No docker at this stage** — D-SPP-1 is discharged and re-running it is not PLAN work. Every figure below was **executed at this tip (`9b2ef891`)**; a SPEC cite is not evidence (`feedback_brief_citations_not_evidence`).

---

## 1. PLAN re-derivation ledger — what this stage RE-DERIVED, REFUTED, and newly EXECUTED

The lineage record is that **every stage finds a load-bearing error in its predecessor, and every one is found by EXECUTION rather than review.** The phase-79 SPEC refuted nine BRAINSTORM claims and two of its own. This PLAN refutes or corrects **eleven** SPEC claims and adds **six** findings no phase-79 document carries. Two of them change what the IMPL must build.

### 1.1 ⚠️ HEADLINE ONE — SPEC §10's FIXTURE ROSTER IS **ONE** FILE. THE STALE ENUMERATION LIVES IN **FOUR**, AND THERE ARE **THREE** MUTUALLY INCONSISTENT WRONG COUNTS LIVE IN-TREE

SPEC §1.3's whole point is that the BRAINSTORM's repair for the stale error string *"reproduces the same defect one generation on"* — it fixed the number without auditing where the number lives. **§10's edit-site roster commits the same error one level up.**

§10 says, verbatim, **"Fixture (1 file): `test/fixtures/0118-runtime-static-layer/driver/driver.go`."** Audited at this tip, the stale top-level enumeration and/or the terminal-error quote appear in **FOUR** files in that fixture:

| file | what it carries |
|---|---|
| `driver/driver.go:154-156` | prose enumeration — **wrong twice** (below) |
| `expectations.yaml:43-46` | the four-form list + a `#`-comment quote of the terminal error |
| `envoy.yaml:16` | the four-form list |
| `README.md:32-35` | prose + a quote of the terminal error |

⚠️ **And `driver.go:155-156` is wrong in a way no document has named.** It omits **`wasm.`** and **`kafka.`** from the top-level set, *and* it lists **`rbac.`** as a top-level hoisting prefix. `.rbac.` is an **INFIX** detector (`strings.Index`, `name.go:235`), not a prefix — demonstrated live against a byte-identical copy of `name.go`:

```
ANYTHING_AT_ALL.rbac.allowed          -> err=<nil>  residual="rbac.allowed"
ANYTHING_AT_ALL.zookeeper.decoder_error -> err=<nil>
```

`ANYTHING_AT_ALL` appears nowhere in the source. A reader counting `rbac.`/`zookeeper.` as top-level reaches **eleven**, not nine.

⇒ **three mutually inconsistent wrong counts are live in-tree right now**: `name.go:350` says **four**, `BEHAVIOR_CONTRACT.md:5020` says **five**, `driver.go:155`/`expectations.yaml:44` say **four plus a wrong list**. They must move to **nine together**. Fixing one and shipping under a commit message announcing the fix is precisely the §1.3 failure mode, one generation on. **This PLAN promotes the sweep to its own task (T3) so it cannot ride as a footnote on T2.**

### 1.2 ⚠️ HEADLINE TWO — THE SINK GATE HAS A CELL THAT **CANNOT FAIL**, AND TODAY'S WHOLE SUITE IS **GREEN UNDER THE HOISTING NEGATIVE CONTROL**

The SPEC mandated the §3.7 hoisting negative control be run **at PLAN time**. It was, on real wire bytes, with determinism proven first (two identical BEFORE runs, `diff` exit 0, 17 473 B each) and inputs derived from real registration sites.

**The NC FIRES — on all four consumers, for BOTH injection sites.** Verbatim wire bytes reproduce the SPEC's claims exactly:

```
dogstatsd:  envoy-go.tracing.spans_sent:17|c|#envoy.tracer_name:opentelemetry
graphite:   envoy-go.tracing.spans_sent;envoy.tracer_name=opentelemetry:17|c
labelmap:   label: { name: "envoy.tracer_name" value: "opentelemetry" }   (x4)
```

Varying the injection site to `access_logs.` (`reference_break_arm_injection_site_is_a_claim`) produces **identical per-consumer diff-line counts**. **The gate is not site-dependent.**

⚠️ **But it IS cell-dependent inside OTLP, and one cell is inert:**

| OTLP cell `(useTagExtractedName, emitTagsAsAttributes)` | byte-mirror | under NC | Δ |
|---|---|---|---|
| **(F, F)** | 1144 | **1144** | **0 — DOES NOT FIRE** |
| (F, T) | 1210 | 1348 | **+138** |
| (T, F) | 1128 | 1086 | **−42 — SHRINKS** |
| (T, T) | 1194 | 1290 | +96 |

⇒ **an OTLP golden exercising only the default knobs is VACUOUS**, and a "bytes must grow" assertion goes RED for the wrong reason on `(T, F)`. **T7 must run at least `emitTagsAsAttributes=true` and must not assert monotonic growth.**

And the result that makes the gate load-bearing rather than decorative — **with the hoisting arm applied, today's entire suite is green**:

```
$ go test ./internal/stats/... ./internal/statssink/... -count=1     # tracing.-HOIST applied
ok internal/stats · ok internal/stats/dynamic · ok internal/statssink       INNER_EXIT=0
```

**Nothing in the tree today can distinguish a byte-mirror arm from a tag-hoisting arm.** That is the gap T7 closes.

⚠️ **A structural corollary, independently derived, that explains WHY three of the four sinks are cheap and OTLP is not.** For `label.go:39` the guard is `if err != nil || len(labels) == 0` — the pre-arm **error** path and the post-arm **zero-label** path funnel into the identical passthrough. For `dogstatsd.go:86` and `graphite.go:70` the error fallback is literally `residual, labels = fam.GetName(), nil`, **byte-identical to what a byte-mirror arm returns**. ⇒ those three are no-op **by construction**. **`otlp.go:189-197` is the only consumer that reaches the new state by a different route** (`base = residual` under `useTagExtractedName && err == nil`) — and it is the one carrying the 2×2 cross-product and the inert cell. **The expensive consumer and the fragile gate are the same one.**

### 1.3 ⚠️ HEADLINE THREE — THE BAND: **10–13, BUDGET 12.** THE FLOOR AND BUDGET AGREE WITH THE SPEC; THE CEILING DOES NOT — AND THE LoC ESTIMATE IS REFUTED BY THE LINEAGE'S OWN NUMBERS

**SPEC §9's ceiling of 12 is the budget with a different label.** It prices the probability that *any* task splits at zero — *"ceiling 12 as enumerated"* — in a lineage where the immediately preceding row did exactly that (78: SPEC 7–9 → PLAN **10**, one above the ceiling, for two reasons that were both *"this task splits"*).

**The LoC estimate is refuted by measurement, not judgement.** SPEC §9 states *"~40 production + ~300-400 test + ~60 fixture ≈ **~500 LoC** ≪ ~1500."* Realized `.go` in the IMPL squashes (located by `git log --grep`, never by position; all three PLAN commits carry **0 `.go` files**, so the IMPL squash *is* the row's code):

| phase | that PLAN's own LoC estimate | realized `.go` | ratio |
|---|---|---|---|
| 76 (`191e72e6`) | (none stated) | +300 / −29 | — |
| 77 (`4d7f63c2`) | *"~700 production+test LoC"* | **+1428 / −22** | **2.04×** |
| 78 (`3a4c8cfa`) | *"~330 production+test LoC"* | **+495 / −24** | **1.50×** |

Applying 1.5–2.0× to ~500 gives **750–1020**; an independent bottom-up build-up gives **~875**. Two methods, no shared input, converging. **Corroborating denominator:** SPEC §9 budgets ~300–400 test lines *in total*, while T7 alone is ~350 — against existing goldens in `internal/statssink` of 419 / 430 / 471 / 542 / 773 lines. **One task plausibly consumes the entire stated test budget.**

⇒ **the PLAN carries `~875 LoC`, and replaces `≪ ~1500` with "inside, with ~1.7× margin."** The gate still does not trip. The word "≪" was doing work the number cannot support.

⚠️ **AND THE NORMATIVE SPLIT GATE IS NOT WHERE §9 CITES IT.** SPEC §9 says `BOOTSTRAP_PROMPT.md` *"carries the string twice (`:225`, `:472`)"*. True — and **incomplete about the gate**. Both cited lines sit in flow reminders (`:225` under §5 *Phase Lifecycle State Machine*; `:472` under §11 *Skill Routing Appendix*). **The normative statement is `### 6.1 When to split` at `:285-290`**, which the `25 tasks` / `1500 LoC` grep **misses** because it spells the numerals differently:

```
290:- `PLAN.md` estimates exceed **~1500 lines of code** of net change.
```

⚠️ **§6.1 carries a THIRD trigger no prior stage has discharged:** splitting is triggered **mid-execution** *"if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity."* **That trigger is LIVE on T7 and T8** (four consumers + a 2×2 cross-product + two stacked controls + `unix_nano` normalization; a 52-line parser rewrite plus a parity conversion plus a RED-first liveness cycle). **This is the mechanism by which the ceiling of 13 actually fires, and it is written into the gate itself.**

**Calibration, re-derived first-hand from the primary documents** (a prior document's summary of this record is not evidence — one is known to have mislabeled it):

| phase | SPEC band | PLAN band | IMPL executed | outcome |
|---|---|---|---|---|
| 76 | 7–9 (`SPEC.md:355`) | **9** (`grep -cE '^#+ *Task [0-9]+' PLAN.md`) | 9 | **AT** the ceiling |
| 77 | 11–13 (`SPEC.md:467`) | **12** (PLAN header + `grep -c`) | 12 | **INSIDE**, one below |
| 78 | 7–9 (`SPEC.md:307`) | **10** (PLAN header) | 10 | **ABOVE**, +1 |

⇒ **76 AT · 77 INSIDE · 78 ABOVE — two-for-three at or above.** This independently reproduces SPEC §1.4's R11 correction against the BRAINSTORM's *"three-for-three"* mislabel.

⚠️ **THE BANKED ROADMAP FIGURE, LOCATED POSITIONALLY.** `ROADMAP.md:209` is a **45 959-character single line carrying SIX task bands** (`5-7` @16227, `9–13` @17909, `10–14` @21896, `7–11` @26189, `8–12` @30880, **`11-14` @45763**). Read with **character** offsets, not `cut -c` (byte-based; the line is multibyte UTF-8 — 46 423 bytes vs 45 959 chars — and a byte slice truncates mid-word and reads as a corrupted file). This row's banked figure is **`11-14 tasks`**, and its context shows it covers **FOUR** arms including the `sds.` hoisting arm now deferred to 79.1. **So 9-11 and 10-12 are both re-scopes DOWNWARD from a banked 11-14, and 10-13 sits between them.**

### 1.4 ⚠️ A FINDING OUTSIDE THE ROW'S SUBJECT THAT THIS STAGE'S OWN FILE SET FORCED IT TO MAKE: **40 LINEAGE BULLETS WERE DELETED, NOT ARCHIVED**

ADR-0288 §Decision 3 states: *"The recent-lineage list is capped at five; **the sixth is moved to `STATE_HISTORY.md`**."* ADR-0288 §Consequences even predicts the failure: *"A new discipline a session can get wrong… not enforced by tooling."*

Audited mechanically — every `- **prior active-phase:**` line ever **removed** from `STATE.md` (210 distinct keys, harvested from the file's whole commit history), checked against today's `STATE_HISTORY.md` **and** today's `STATE.md`:

```
LOST (in neither file): 40
```

`STATE_HISTORY.md`'s archived phase range is `38 … 66, 76, 77, 78` — **phases 67 through 75 are wholly absent**, and 76 is partial. Negative-controlled in both directions: the loose slug grep returns `HIST=0` for every lost slug and `HIST=2` / `HIST=1` for `boot-panic-visibility` / `runtime-static-layer`, which *are* archived.

**Severity is bounded and stated:** ADR-0288 §Context establishes the content is triply redundant (commit message, phase docs, ROADMAP row), so nothing is unrecoverable. **What is false is the archive's own claim to be the complete record.**

⚠️ **This is not a documentary curiosity for this stage — it determines this stage's file set.** The bullet this PLAN evicts is `STATE.md:54`, `phase 77 (runtime-static-layer) SPEC done`, and `grep -cF` in `STATE_HISTORY.md` returns **0**. **So this PLAN archives it, and the file set is five.** **NOT FIXED here:** the 40-bullet backfill is a named deferral (§7) — a docs-only PLAN stage on a projection row is the wrong place to rewrite ten phases of archive, and doing it silently under a phase-79 commit message would be its own species of the §1.1 defect.

### 1.5 Anchor drift — SEVEN corrections, three of which propagated into three documents each

| # | cite | claimed | TRUE at `9b2ef891` | severity |
|---|---|---|---|---|
| **D1** | `0118/driver/driver.go` `scrapeProm` | `:407-447` | **`:407-458`** — the claim truncates 11 lines, cutting the timestamp strip, the `ParseFloat`/NaN guard **and `return out, nil`**. Wrong in `SPEC.md:157`, `SPEC.md:335` (the edit-site roster) **and** `next-prompt.txt:107` | **HIGH** — an IMPL editing "`:407-447`" edits a function that does not end there |
| **D2** | `driver.go` "both-forms parser" | `:399-403`, *"which handles both forms"* | **`:429-443`**. `:399-403` is **doc-comment text only** — an example block plus the opening of a sentence; **zero executable lines** | **HIGH** — SPEC §3.5's parity-flip justification rests on this anchor, so the justification currently points at prose |
| **D3** | `manager.go:381` `normalizeAddr` | the function | **`normalizeAddr` is DEFINED at `:352`** (doc `:331`); **`:381` is its sole call site**, the first line of `registerListenerMetrics` (declared `:380`). Cite `:352` for the function, `:381` for the use | **MEDIUM** — T6's roster derivation is dispatched against this cite |
| **D4** | SPEC §10 "Fixture (**1** file)" | 1 | **4** — see §1.1 | **HIGH** |
| **D5** | `BEHAVIOR_CONTRACT.md:5020` | *"closes with `The day internal/stats learns runtime.`…"* | that is the last **bolded** clause, not the closing sentence; two clauses follow (`…naming the follow-up. A prose-only deferral would have rotted; this one cannot.`) | LOW — a byte-anchored edit keyed on "closes with" will miss |
| **D6** | `STATE.md` §Project counts | — | **self-contradicts §Current inside the same file**: `:31` fixtures **119** (true 120) · `:33` stat surface **1205** (lineage **1207**) · `:35` DECISIONS tail **ADR-0298** / next-free **ADR-0299**, while `:21` of the same file says tail **ADR-0301** / next-free **ADR-0302**. `:29` labels the block a **phase-76 IMPL** snapshot | MEDIUM — **NOT repaired** (§7) |
| **D7** | `STATE.md:44` | — | says *"§Current carries the newest stage (**the phase-79 BRAINSTORM**…)"*; §Current `:15` is the phase-79 **SPEC** | LOW — **repaired at this stage**, since this PLAN rewrites that preamble anyway |

**Explicitly NOT drifted** (re-derived, each confirmed): all 12 `name.go` anchors (`ExtractTags` `:47`, `switch {` **`:50`**, arms `:51 :61 :83 :93 :97`, `default:` `:132`, terminal error `:350`, `flattenToProm` `:370` / call `:371`, `helpText` `:458-476` = **15** entries counted mechanically, prose `:448`, SN-rule block `:24-46`) · the four `CutPrefix` cites `:286 :306 :323 :340` · all three `prom.go` anchors **and the SPEC's `continue`-not-early-exit correction** · both `registry.go` lock anchors · all 11 `name_test.go` anchors and counts (959 lines, 55 `^func Test`, 56 top-level funcs, 0 `^func Fuzz`) · all four `statssink` call sites · **the five-call-site denominator** · the five `TestNoNewStat*` guards as exhaustive · `admin/{stats.go,prometheus.go,admin.go}` · `tracing`, `listener`, `xds`, `boot` · `BC:5020`'s existence and its five-arm enumeration · every one of the 15 doc/count figures.

### 1.6 ⚠️ THE NINE-SEGMENT CLAIM **SURVIVES** — but it needs a SPECIES ADJUDICATION the SPEC does not make

Two independent methods with **different blind axes** (`reference_independent_probes_can_share_a_blind_axis`):

1. **Static audit, not a sample.** All 46 lines consuming the input parameter across `ExtractTags`' 314-line body: **9** root detectors (`HasPrefix` `:51 :61 :83 :93 :97`; `CutPrefix` `:286 :306 :323 :340`), **4** infix detectors (`strings.Index` `:147 :185 :236 :265`), nothing else. No regexp, map lookup, segment-split or helper adds an acceptor. NC on the pipeline: `strings.(HasPrefix|CutPrefix)\(internalZZZ` → exit 1.
2. **Live acceptance probe**, 48 candidate tokens (the 9 claimed + every top-level segment harvested from real `NewCounter`/`NewGauge` literals + the 0118 trio + invented controls), each as `<tok>.aaa.bbb` and `<tok>.aaa`, against a byte-identical copy (md5 verified both sides):

```
ACCEPTED (9): cluster http kafka listener mongo redis server thrift wasm
REJECTED (39): access_logs admin ... rbac ... runtime ... tracing ... zookeeper zzzz_nonexistent
```

**No tenth. None spurious.** ⇒ **NINE is CONFIRMED.**

⚠️ **THE ADJUDICATION T2 MUST MAKE BEFORE IT DRAFTS A BYTE-STABLE GUARD.** A name reaching `:350` has failed **thirteen** detectors — nine top-level **plus four mid-name**. *"Nine recognized top-level segments"* is a true statement about top-level segments; it is **not** a complete statement about why the name was rejected. **T2 decides explicitly what the string claims, and the byte-stable guard pins that decision.** Without this, the guard pins a **sixth** generation of an under-specified list — the exact failure mode §1.3 exists to prevent, one generation on.

### 1.7 Corrections to the SPEC's executed matrices — the results hold, the per-arm attribution does not

⚠️ **§3.6's matrix must be restated as fired-leg SETS, not single legs** (`reference_deliberate_break_wrong_assertion` cuts both ways — it is as wrong to under-report a firing as to mis-attribute one). Re-executed, all four arms:

| arm | `INNER_EXIT` | legs fired | SPEC said |
|---|---|---|---|
| positive (aggregate log) | **0 PASS** | none — captured **108 B**: `stats: WriteProm skipped 1 registered metric name(s) with no recognized top-level segment: runtime.num_keys` | ✅ |
| NC-1a over-fire, aggregate over every name | **1** | **LEG 3 ONLY** — *"OVER-FIRES: it names `server.live`, which PROJECTED successfully"* | leg 3 ✅ |
| NC-1b over-fire, per-name (2 lines) | **1** | **LEGS 1 AND 3** — *"fired 2 time(s), want EXACTLY 1"* | leg 1 ⚠️ **incomplete** |
| NC-2 never fire (the shipped tip) | **1** | **LEGS 1 AND 2** — *"fired 0 time(s)"*, `captured log (0 bytes, 0 non-empty line(s))` | leg 1 ⚠️ **incomplete** |

⇒ ⚠️ **the SPEC's *"the two over-fire arms trip DIFFERENT legs"* is PARTIALLY REFUTED** — they are distinguishable by *set*, but **leg 3 fires in both**. **The justification for leg 3 is now sharper than the SPEC's, not weaker:** NC-1a fires leg 3 **and nothing else**, so without leg 3 NC-1a passes **entirely**. The SPEC's conclusion — *a positive-only test would have passed all three negative controls* — is **CONFIRMED**.

⚠️ **§3.3's *"1 line / 1151 bytes"* is REFUTED AS STATED.** With the SPEC's **own** message shape (§3.6's captured line), the 30-name roster emits **1180 bytes**. 1151 is reachable **only** under a bare-comma `strings.Join(names, ",")`; the natural `", "` join gives 1180. **Delta = exactly 29 = the separator count.** The SPEC never pins the format string or the separator, so the figure **is not reproducible from the document** and a gate asserting it is unfalsifiable. **T4 PINS the format string and the separator, and the PLAN carries 1180.** The **1 line** half is CONFIRMED.

⚠️ **§3.2's secret→fixture attribution is SWAPPED**: `edf_validation_ca` is **`0110`**, `rccf_validation_ca` is **`0111`** — the SPEC has it the other way. The *set* is right, so the 30-name total and the 79.1 deferral are unaffected. Recorded so 79.1 does not inherit it.

### 1.8 Two blast-radius findings no phase-79 document carries

1. ⚠️ **`WriteProm` is called inside a FUZZ TARGET.** §3.3's *"only non-test **production** caller"* is true and the qualifier is load-bearing — but `internal/stats/fuzz_test.go:73` calls `WriteProm` **inside `f.Fuzz(...)`**. A skip-site `log.Printf` would fire **per fuzz iteration** under the short-budget CI run. The target synthesizes `listener.<addr>.downstream_cx_total`, which projects via SN3, **so the line should not fire — but that is a PREDICTION and it is added to T5's gate as an execution**, not carried as reasoning. Three further test callers exist (`prom_test.go`, `name_test.go:913`, `bandwidthlimit_test.go:1978`).
2. ⚠️ **The prometheus blast radius is ~30 fixture drivers, not one.** `setRuntimeStats` (`bootstrap.go:751`) is called on **both** boot success paths, including the `lr == nil` branch at `:686` — so registration is **unconditional**, and once the `runtime.` arm lands **every subject's `/stats/prometheus` gains 6 lines** (2 `# HELP` + 2 `# TYPE` + 2 samples). `grep -rln 'stats/prometheus' test/` ⇒ **62 files across ~30 fixtures**. The parse shapes were audited: `0005` parses into a named-field `Snapshot` (`driver.go:484`), every other scraper is a per-line parse into a map; **no exhaustive-set or count assertion was found**, so the radius is *probably* benign. ⚠️ **It is UNMEASURED, and only the full 120-fixture run discharges it.** This is why §9's *"the FULL differential is mandatory"* is right — **for a reason §9 does not state.**

### 1.9 What this PLAN EXECUTED, and what it deliberately did not

**EXECUTED at this tip:** the mandatory §3.7 hoisting NC on real wire bytes, **two injection sites**, four consumers, OTLP 2×2, determinism proven first · the whole-suite green-under-NC result · the `WriteProm` **deadlock** with a goroutine trace naming both lock sites · the **discriminating control** (eager pre-registration outside `Walk` ⇒ PASS) · the `TestNoNewStat*` blindness **including the composite claim** (all five green against a build that deadlocks) with a failing baseline · the terminal-error string swap, built and suite-run, then reverted **sha256 byte-identical** · the **four-arm `helpText` matrix** including the decisive blind row · the **four-arm observability matrix** with fired-leg sets · the 48-token nine-segment acceptance probe · the 210-key `STATE_HISTORY` archive audit · all three sentinel checks with firing negative controls · every anchor and count in SPEC §10 and router items 7/13/16.

**NOT executed — carried as claims:** the absolute stat surface **1207** (documentary; **assert the DELTA**) · the full 120-fixture differential (deliberately not run at a docs-only stage; **mandatory at the IMPL**) · h2spec · the reference-side zipkin roster (see below).

⚠️ **C7 no longer rests on the SPEC's docker run alone, and that is a strengthening.** The claim that the reference zipkin family has **no `spans_dropped`** is independently corroborated in-repo by a **prior live probe two rows earlier**: `docs/envoy-go/phases/46-tracing/SPEC-46.2.md:127`/`:163` records the reference roster as `{reports_dropped, reports_failed, reports_sent, reports_skipped_no_cluster, spans_sent, timer_flushed}` — **verbatim the phase-79 SPEC's list**. `DECISIONS.md:16215` confirms `spans_dropped` is an envoy-go naming *decision*. And `SPEC-46.1.md:19,203` records the reference **OTel** roster as `{spans_sent, spans_dropped, timer_flushed}` — so **`tracing.opentelemetry.spans_dropped` DOES have a counterpart**, making **9 of 10** exactly right. All `spans_dropped` hits under `test/` are `subjSpansDroppedStat` — **subject-side only** — so no unsatisfiable cross-side assertion exists today.

---

## 2. Global constraints

1. **One stage per session.** This is the PLAN. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` stay **BYTE-UNTOUCHED**; row 79 stays `in-progress`. The row flips to `done` at the IMPL only.
2. **TDD spine.** Every task that adds behavior is RED-first with the RED **observed and recorded**, never inferred. A green that could also mean "did not run" is not a result (`reference_liveness_break_needs_failing_baseline`).
3. **One `t.Errorf` per property.** `t.Fatalf` makes later assertions dead code (`reference_fatalf_makes_assertions_unreachable`).
4. **Assert the SET, not a count.** A count-only guard passes a build with **both** names wrong (`reference_stat_count_guard_blind_to_rename`).
5. **`-count=1` always.** A cached PASS is not a run.
6. **`gofmt -l` never exits non-zero** — gate on **OUTPUT**.
7. **`golangci-lint` runs `misspell` with `locale: US`** — ⚠️ **LIVE this row: it edits Go comments AND an error string.** It fired at the SPEC on *"signalled"*. **Do not paste SPEC or PLAN prose into `.go` files.**
8. **Capture `INNER_EXIT`** — a harness's exit code is not the command's.
9. **`go build ./cmd/envoy-go/` drops an untracked binary in the worktree root** — build with `-o` into scratch.
10. **Git hygiene.** `git -C <abs-worktree-path>` for every git command; the Bash cwd reset is assumed live (it **fired again this session**, the eighteenth consecutive). Subagents commit **locally only**, never push; controller squash-pushes at close. Parallel agents get private scratch, private detached worktrees and port bands **outside 20000-31007**; docker containers torn down **BY NAME**.
11. **Breaks run AFTER committing** — `git restore` wipes uncommitted work. Never an unscoped `git restore`.

---

## 3. File structure — the IMPL's edit surface, RE-DERIVED

**Production (2 files):**
1. `internal/stats/name.go` — three arms in the `switch` at **`:50`** (⚠️ **NOT `~:100-110`**, which is inside the `wasm.` arm's comment block `:98-125`); SN-rule doc block **`:24-46`**; terminal error **`:350`**; `helpText` **`:458-476`** (15 entries) and its prose count at **`:448`**.
2. `internal/stats/prom.go` — the skip block **`:39-41`** (bare `return` **`:40`**, a per-metric `continue` inside the `Walk` closure opened `:37` / closed `:53`) and the self-contradicting comment **`:18-22`**.

**Test (3 files):** `internal/stats/name_test.go` (959 lines) — arm guards, the byte-stable string guard, both `helpText` guards · a NEW observability test in `internal/stats` · a NEW sink-identity golden in `internal/statssink`.

**Fixture (⚠️ FOUR files, not one — §1.1):** `test/fixtures/0118-runtime-static-layer/` → `driver/driver.go` (pin blocks **`:142-162`** and **`:164-169`**, call site `:266`, function `:269-315`, `scrapeProm` **`:407-458`**, the real both-forms parser **`:429-443`**, the wrong prose at **`:154-156`**) · `expectations.yaml:43-46` · `envoy.yaml:16` · `README.md:32-35`.

**Also:** `internal/admin/stats.go:20` (the stale `(the redis. Prometheus tag-extractor arm is 32.2)` parenthetical — that arm landed at `name.go:323`).

**Docs:** `DECISIONS.md` (ADR-0301 §Decision + §Consequences) · `BEHAVIOR_CONTRACT.md` (§7) · `ROADMAP.md` (row 79 → `done`, **IMPL only**) · `STATE.md` · `STATE_HISTORY.md` · `next-prompt.txt`.

---

## Task 1 — the three byte-mirror arms

**Edit:** `internal/stats/name.go`, insert after the `server.` SN5 arm (`:93-96`), before the `wasm.` arm (`:97`):

```go
case strings.HasPrefix(internal, "runtime."):
	residual = internal
case strings.HasPrefix(internal, "access_logs."):
	residual = internal
case strings.HasPrefix(internal, "tracing."):
	residual = internal
```

`labels` stays nil — the exact SN5 shape. **No dot→underscore pre-transform belongs here**; `flattenToProm` (`:370`) already does that at projection time. Update the SN-rule doc block `:24-46` to name the three roots.

⚠️ **RED-first.** Add the table rows to `name_test.go` asserting `residual == input && labels == nil` for all ten names **before** the arms, observe RED, then green. Ten names, derived from registration sites, denominator stated:

| name | site |
|---|---|
| `runtime.num_keys`, `runtime.num_layers` | `internal/bootstrap/bootstrap.go:621-622` (gauges) |
| `access_logs.grpc_access_log.logs_{written,dropped}` | `internal/accesslog/stats.go:24-25` |
| `access_logs.open_telemetry_access_log.logs_{written,dropped}` | `internal/accesslog/stats.go:34-35` |
| `tracing.opentelemetry.spans_{sent,dropped}` | `internal/tracing/stats.go:53-54` |
| `tracing.zipkin.spans_{sent,dropped}` | `internal/tracing/stats.go:70-71` |

**Stacked controls in the same test:** `cluster.backend.upstream_rq_total` must **keep** its hoisted label; `listener_manager.listener_create_success` must **stay** rejected. ⚠️ **Carry this caveat:** `listener_manager.*` has **no non-test registration site anywhere in envoy-go** — it is a synthetic control inherited from `name_test.go:956`, valid as an over-fire control but **not evidence about a shipped metric**.

**Gate:** `go test ./internal/stats/ -count=1` (RED observed first, then INNER_EXIT 0) · `gofmt -l internal/stats/` **output** empty · `golangci-lint run ./internal/stats/...`.

**No collision:** `grep -nE '"(runtime|access_logs|tracing)\.' name_test.go` ⇒ **0 hits** across 959 lines, so the existing negatives at `:141` and `:956` are unaffected.

---

## Task 2 — the `:350` error string → the NINE-segment enumeration, **plus the species adjudication**, plus a byte-stable guard

**Current, verbatim:**
```go
return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment (want cluster.|http.|listener.|server.)", internal)
```

**⚠️ ADJUDICATE FIRST (§1.6), then write.** The nine top-level segments are `cluster. http. listener. server. wasm. mongo. kafka. redis. thrift.`. A name reaching this line has **also** failed four mid-name detectors (`.http_local_rate_limit.` `:147`, `.http_bandwidth_limit.` `:185`, `.rbac.` `:236`, `.zookeeper.` `:265`). **T2 decides, in the commit, whether the message enumerates the nine only (and says "top-level" precisely) or names both species.** Record the decision in the PLAN's PROGRESS entry. **Do not draft the guard before the decision.**

**Then a BYTE-STABLE guard** on the landed string (the phase-77 `TestParseRejectConstants_ByteStable` precedent). ⚠️ **Without it, this row's own fix is exactly as unguarded as the defect it repairs.**

⚠️ **`misspell` locale US is live on this edit.** The replacement contains identifier tokens only. **Do not paste PLAN prose into the string.**

**Proven safe at PLAN time:** with the nine-segment form in place — `go build ./...` **0** · `go test ./internal/stats/ -count=1 -v` **0** with **87 `--- PASS`, 0 FAIL, 0 `no tests to run`** (proving it ran, not that it was skipped) · `go test ./internal/... -count=1` **0**, **70 `ok`**. Reverted; `name.go` **sha256 byte-identical** across the cycle.

**Denominator for "no test asserts the string":** `has no recognized top-level segment` ⇒ **12** repo-wide hits — **1** `.go` (`name.go:350`), **2** in-fixture non-executing (`0118/expectations.yaml:46` a `#` comment, `0118/README.md:35` prose), **9** in `docs/`. Neither fixture hit carries `(want `. Of the 5 `(want ` co-occurrences, **4 are docs and none executes.**

**Gate:** red-then-green on the byte-stable guard; full `./internal/...` suite green.

---

## Task 3 — the stale-enumeration sweep: **FOUR fixture files + two more**, promoted out of T2

⚠️ **This is §1.1 and it is a task, not a footnote.** Fixing `name.go:350` alone ships the §1.3 defect one generation on.

| site | defect | after |
|---|---|---|
| `0118/driver/driver.go:154-156` | four-form list, **omits `wasm.` and `kafka.`**, **lists `rbac.` as top-level** when `.rbac.` is INFIX | nine top-level, `rbac.`/`zookeeper.` correctly named as mid-name detectors |
| `0118/expectations.yaml:43-46` | four-form list + terminal-error quote | nine |
| `0118/envoy.yaml:16` | four-form list | nine |
| `0118/README.md:32-35` | prose + terminal-error quote | nine |
| `internal/admin/stats.go:20` | *"(the redis. Prometheus tag-extractor arm is 32.2)"* — **that arm LANDED** (`name.go:323`) | stale parenthetical removed/corrected |
| `BEHAVIOR_CONTRACT.md:5020` | enumerates **five** arms | handled in T9 (same file, one edit pass) |

**Gate — grep-verified per site with a stated denominator, negative-controlled.** ⚠️ **`grep` cannot tell a mention from a use** (`reference_sentinel_matcher_string_self_clears`): the after-state check must be *"the four-form list appears **0** times in these five files"* **and** *"the nine-form list appears **5** times"*, each run against a deliberately doctored scratch copy to prove it fires. **Fix by PATTERN, not by line** (`reference_stale_cite_recurs_fix_by_pattern`) — the four-form list recurs by inheritance.

---

## Task 4 — `WriteProm` observability: the aggregated log, with the format string **PINNED**

**DECISION (inherited from SPEC §3.3, which resolved it by execution): an aggregated `log.Printf`, ONE line per `WriteProm` call, names sorted and joined.** Stat surface stays **+0**.

⚠️ **The counter form is DISQUALIFIED because it DEADLOCKS, and this PLAN reproduced it.** `Registry.Walk` (`registry.go:138-142`) holds `r.mu.RLock()` across every `fn` call; `getOrRegister` (`:179`) takes `r.mu.Lock()`; Go's `RWMutex` is not reentrant. Injecting `r.NewCounterIfAbsent("server.prom_name_skipped").Inc()` at `prom.go:40` under `go test -timeout 20s`:

```
panic: test timed out after 20s
getOrRegister registry.go:179 (RWMutex.Lock) <- NewCounterIfAbsent :162
  <- WriteProm.func1 prom.go:40 <- Walk registry.go:142 (under the :139 RLock) <- WriteProm :37
```

**Discriminating control executed** (`reference_probe_must_discriminate`): the same counter **pre-registered eagerly outside `Walk`** and only `.Inc()`'d inside ⇒ `--- PASS`, `INNER_EXIT=0`. ⇒ the mechanism is exactly as stated **and the SPEC's escape hatch is sound** — it is simply a different leg shape, not the one this row ships.

⚠️ **`sync.Once` stays REJECTED on executed grounds** — it makes the signal order-dependent and untestable beyond one test.

⚠️ **PIN THE FORMAT STRING AND THE SEPARATOR (§1.7).** The SPEC's 1151 B is reachable only under a bare-comma join; the natural `", "` join gives **1180 B** on the real 30-name roster — delta exactly 29, the separator count. **T4 lands the format string verbatim in the PLAN's PROGRESS record and the byte figure alongside it, or the gate is unfalsifiable.** Shape (US-spelled, identifier tokens only):

```
stats: WriteProm skipped N registered metric name(s) with no recognized top-level segment: <sorted, joined>
```

**Also rewrite `prom.go:18-22`.** It contradicts itself inside one sentence — *"silently skipped … log+ignore"* — and there is **no log**: `prom.go` imports only `fmt io sort strings`. Leaving it after adding the log ships a **third** generation of the inconsistency.

**Frequency, denominator stated:** **12** repo-wide `WriteProm(` hits excluding the definition — **10** in `_test.go`, **1** a `doc.go:24` comment (not a call), **exactly 1** real non-test call site: `internal/admin/prometheus.go:23` (`handlePrometheus`, registered `admin.go:94`). Nothing polls it. ⚠️ **The noise is self-extinguishing: this row takes 30 → 20 names, and 79.1 takes it to 0 ⇒ 0 lines.**

**Envelope, measured:** `go build ./...` **0** · `git diff go.mod go.sum` **0 bytes** · `golangci-lint run ./internal/stats/...` **0** · `gofmt -l` empty · only the stdlib `log` import added. `reference_new_subpackage_pulls_transitive_module` does not fire.

**Gate:** builds; lint clean; `git diff go.mod go.sum` 0 bytes.

---

## Task 5 — the observability stacked control, its **three** negative controls, and the fuzz-flood check

⚠️ **A positive assertion cannot catch an over-firing signal** (`reference_positive_arm_cannot_catch_overfiring`). Stacked control: one projecting name and one dropped name in the **same** registry.

- **Capture:** `log.SetOutput(&buf)` + `log.SetFlags(0)`, restored via `t.Cleanup`. **Confirmed `-race` clean** (`go test ./internal/stats/... -count=1 -race` INNER_EXIT 0, **0** `WARNING: DATA RACE`). **NOT `t.Parallel()`**; `internal/stats` has no other `log` user (grep-confirmed on the clean tip).
- **Registry:** leg A `server.live` (live SN5 arm) must **NOT** be signaled; leg B `runtime.num_keys` (no arm at this tip) must be.
- **Four assertions, each its own `t.Errorf`:** (1) **exactly one** non-empty log line · (2) the line **names** `runtime.num_keys` · (3) **negative leg** — it must **NOT** name `server.live` · (4) liveness cross-check that `envoy_server_live 1` really is in the exposition, so leg 3 is not vacuous.

**The matrix, RE-EXECUTED at this tip and stated as fired-leg SETS (§1.7):**

| arm | `INNER_EXIT` | legs fired |
|---|---|---|
| positive | **0 PASS** | none — captured **108 B** |
| **NC-1a** over-fire, aggregate over every name | **1** | **LEG 3 ONLY** |
| **NC-1b** over-fire, per-name (2 lines) | **1** | **LEGS 1 AND 3** |
| **NC-2** never fire — the shipped tip | **1** | **LEGS 1 AND 2** |

⚠️ **Leg 3's justification, corrected and sharper:** NC-1a fires leg 3 **and nothing else**, so **without leg 3, NC-1a passes entirely.** NC-2 doubles as the required failing baseline — the assertion is proven RED on today's tree before anything is fixed.

⚠️ **NEW GATE LEG — the fuzz-flood check (§1.8).** `internal/stats/fuzz_test.go:73` calls `WriteProm` **inside `f.Fuzz(...)`**. Prediction: the synthesized `listener.<addr>.downstream_cx_total` projects via SN3 and the line does not fire. **This is a prediction and T5 EXECUTES it** — run the fuzz target short-budget with the log live and assert the captured output is empty, with a negative control that feeds a `runtime.`-shaped seed and *does* fire. **A prediction recorded as a result is how the phase-79 SPEC's own probe-input defect happened.**

**Gate:** each NC red on its named legs, positive green, `-race` clean, fuzz-flood check executed both directions.

---

## Task 6 — `helpText` ×10, the `:448` prose count, and **BOTH** reverse guards

`helpText` goes 15 → **25** entries. Prose at `:448` (*"Of the 15 entries"*) updates.

⚠️ **TWO guards, because the four-arm matrix proves one is insufficient** — re-executed at this tip:

| arm | Guard A (`TestHelpText_KeySetExact`) | Guard B (`TestHelpText_NoSelfEqualHelp`) |
|---|---|---|
| clean tree | PASS | PASS (15 `# HELP` lines parsed) |
| extra unlisted entry | **FAIL** — `extra: [envoy_zz_a4_extra]` | PASS (n/a) |
| entry deleted | **FAIL** — `missing: [envoy_cluster_membership_total]` | **FAIL** — self-equal line |
| ⚠️ **key TYPO'd *and the typo copy-pasted into the golden list*** | ⚠️ **PASS — BLIND** | **FAIL** — `"# HELP envoy_cluster_membership_total envoy_cluster_membership_total"` |

**The last row is decisive:** the golden-set guard alone is defeated by the single most likely authoring mistake. **Only the projection-driven guard catches it.**

- **Guard A** — set **equality**, reporting `missing` and `extra` **separately**, never a count.
- **Guard B** — drives the **real projection**, parses every `# HELP <name> <help>` line, asserts none is **self-equal** (`prom.go:59-61`'s degradation signature).

⚠️ **THE ROSTER MUST BE DERIVED, NOT HAND-WRITTEN** (`reference_probe_input_is_a_claim`, which fired in practice at the SPEC — a hand-written roster produced a **false positive on 6 names**). Derive from **`normalizeAddr` at `internal/listener/manager.go:352`** (⚠️ **NOT `:381` — that is the call site inside `registerListenerMetrics`, declared `:380`**; §1.5 D3) and the landed golden at `name_test.go:10`. `normalizeAddr` = `":"→"_"`, `"."→"_"`, `"["`/`"]"`→`""`, so the real shape is `listener.0_0_0_0_10000.…`, not `listener.0.0.0.0_10000.…`.

**Anti-false-positive step, executed BEFORE the matrix:** `roster=15 distinct_bases=15 helpText=15`, zero missing, zero extra — **no repeat of the SPEC's 6-name false positive.**

**"No reverse direction" re-confirmed on today's tree:** a deliberately wrong 16th entry (`envoy_zz_bogus_typod_key`) leaves `go build ./...` **0** and `go test ./internal/stats/... -count=1` **INNER_EXIT 0**.

⚠️ **THE JUSTIFICATION IS NOT PARITY.** The reference emits **ZERO** `# HELP` lines (`grep -c '^# HELP'` = 0 on all six reference dumps). envoy-go emits one per group. **This leg is an envoy-go-internal quality choice that WIDENS an existing block-level departure** — T9 states it as a departure, never as parity.

**Gate:** the four-arm matrix, all four arms re-run, each fired guard named.

---

## Task 7 — the `internal/statssink` golden **byte** gate

⚠️ **THE GATE MUST BE A UNIT TEST, NOT A FIXTURE GATE**, and the denominator is now stated with a multi-spelling search (`reference_probe_must_discriminate`):

- **SET A** — fixtures configuring `stats_sinks`: **10** (`0089 0090 0091 0092 0093 0094 0098 0101 0112 0113`)
- **SET B** — tracers, searched as `tracing:` / `http.tracers.` / `OpenTelemetryConfig` / `ZipkinConfig` / `tracing.v3` / `.trace.v3`: **11** (`0086 0087 0088 0102 0105 0106 0107 0114 0115 0116 0117`)
- **SET C** — gRPC/OTel access loggers, searched as `access_loggers.{http_grpc,tcp_grpc,open_telemetry}` / `HttpGrpcAccessLogConfig` / `TcpGrpcAccessLogConfig` / `OpenTelemetryAccessLogConfig`: **5** (`0081 0082 0083 0084 0085`)
- **`A ∩ (B ∪ C)` = EMPTY**, denominator **120**.

⚠️ **B and C are both NON-EMPTY, and that is the negative control** — the greps demonstrably find these configs, so the **0** is real and not manufactured by a single broken spelling (`reference_empty_output_is_not_a_zero_result`).

**`runtime.*` IS live-covered, by code path:** `parseLayeredRuntime` calls `setRuntimeStats` on the **`lr == nil`** branch (`bootstrap.go:686`) as well as `:734`, so registration is unconditional — all 10 stats-sink fixtures configure zero `layered_runtime` yet still carry the gauges, and `label.go:38-42`'s `err != nil` fallback puts the full dotted name on the wire.

**The golden, specified precisely** (registration order **is** emission order — `Walk` is registration-ordered). Assert the **ordered slice**, never `len(lines) == 13` and never per-name presence:

```
Registry (13 entries: 10 byte-mirror + 3 controls)
  cluster.backend.upstream_rq_total          counter 7   <- stacked control: label MUST survive
  cluster.backend.membership_total           gauge   3   <- stacked control
  listener_manager.listener_create_success   counter 1   <- over-fire control: MUST stay untransformed
  runtime.num_keys / num_layers              gauge  5 / 2
  access_logs.grpc_access_log.logs_written / logs_dropped              counter 11 / 0
  access_logs.open_telemetry_access_log.logs_written / logs_dropped    counter 13 / 0
  tracing.opentelemetry.spans_sent / spans_dropped                     counter 17 / 0
  tracing.zipkin.spans_sent / spans_dropped                            counter 19 / 0

GOLDEN 1 dogstatsd   "envoy-go.<FULL DOTTED NAME>:<v>|<c|g>"   and NO "|#" suffix
  ... except the two controls:  "envoy-go.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend"
GOLDEN 2 graphite    "envoy-go.<FULL DOTTED NAME>:<v>|<c|g>"   and NO ";" anywhere
  ... except:        "envoy-go.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c"
GOLDEN 3 labelMapper out[i].GetName() == <FULL DOTTED NAME>  AND  len(Metric[0].Label) == 0
  ... except:        out[0].GetName() == "cluster.upstream_rq_total", Label[0] == {envoy.cluster_name, backend}
GOLDEN 4 OTLP        m.GetName() == "envoy-go." + <FULL DOTTED NAME>  AND  len(dp.Attributes) == 0
```

⚠️ **GOLDEN 4 MUST run at least `emitTagsAsAttributes=true`.** The `(useTagExtractedName=F, emitTagsAsAttributes=F)` cell is **PROVEN INSENSITIVE** — NC delta **0** (1144 → 1144). A golden on the default knobs alone is **vacuous**. ⚠️ **And do NOT assert monotonic byte growth** — the `(T, F)` cell **shrinks** (1128 → 1086) under the NC.

Optional byte pin (safe: `*_unix_nano` are proto `fixed64`, so byte counts are timestamp-value-independent), prefix `envoy-go`, the 13-family roster above: `(F,F) 1144 · (F,T) 1210 · (T,F) 1128 · (T,T) 1194`. ⚠️ **Normalize `*_unix_nano` with an in-test guard that `t.Fatalf`s if the normalization regex never fires**, so the OTLP comparison cannot be silently vacuous.

⚠️ **BEFORE/AFTER at PLAN time:** all four consumers **ZERO diff lines** under the byte-mirror arms; `cluster.*` kept its hoisted label (no collateral); `listener_manager.*` stayed dropped (no over-fire), **including at the Prometheus layer** (still absent from the exposition). `EXTRACT` 20 diff lines = 10 removed + 10 added. `PROM` **2 → 12 metric lines** (line counts CONFIRMED). ⚠️ **The SPEC's PROM byte figures 304 → 2171 are NOT reproduced** — measured **380 → 2246** with a registration-derived roster. **Byte counts depend on the unspecified control roster and must NOT be pinned.**

⚠️ **The NC total is 110 diff lines across the four sinks, NOT the SPEC's 152.**

**Hosting cost measured:** `git diff go.mod go.sum` **0 bytes**; `protobuf/{proto,encoding/prototext}` and `commonpb` are already in the graph; the sinks' constructors, `snapshot()`, `newLabelMapper()`, `otlpKV()` and `toExportRequest` are package-local.

**Gate:** golden green on the byte-mirror tree; **hoisting NC RED across all four consumers**, re-run at the IMPL tip.

⚠️ **§6.1 mid-execution split trigger is LIVE on this task** (§1.3). If the sub-steps exceed ~10, split OTLP into its own task — that is the named ceiling mechanism (a), not a surprise.

---

## Task 8 — `0118`: pin → parity, **label-aware**, liveness RED-first

**DECISION: CONVERT, do not delete.** *"Delete it in favour of the generic prometheus comparison"* is **REFUTED — no such comparison exists.** `0118`'s `ProbeAdmin` returns `/ready` only; there is no generic prometheus differential anywhere in the runner. Deleting leaves that surface with **zero** assertions on **either** side, silently, while reading as cleanup.

**Liveness was PROVEN by the full red-then-green cycle BEFORE any edit** (`reference_liveness_break_needs_failing_baseline`): baseline `INNER_EXIT=0` with `--- PASS: TestDifferential/0118-runtime-static-layer` and `subj_num_keys_present=false`, **no `[no tests to run]`** · break (inject the `runtime.` arm) `INNER_EXIT=1` with **exactly** the two subject-absence `t.Errorf`s firing and nothing else · revert `INNER_EXIT=0`. **Subject emitted 6 and 2, identical to the reference — a parity assertion will pass.**

**The conversion:** rename to `assertPrometheusExpositionParity`; keep the reference loop unchanged; replace the subject-absence loop with a present-and-equal check, **absence-check separate from value-check** and `continue`-ing (mirroring the `:225-232` vacuity guard). **Keep the flat-`/stats` legs as a second seam** — they are the only thing distinguishing "gauge wrong" from "renderer wrong".

⚠️ **ANCHOR CORRECTIONS THIS TASK DEPENDS ON (§1.5):**
- **`scrapeProm` is `:407-458`, NOT `:407-447`.** The short form cuts the timestamp strip, the `ParseFloat`/NaN guard **and `return out, nil`**.
- **The both-forms parser is `:429-443`, NOT `:399-403`** — `:399-403` is doc-comment text with **zero executable lines**. SPEC §3.5's justification cites the comment.

⚠️ **`scrapeProm` IS LABEL-BLIND** — it splits at `{` (`:429-435`), so a parity assertion built on it **passes silently against a future hoisting arm**. Given the **zero-label brace divergence** (reference writes `envoy_runtime_num_keys{} 6`; envoy-go **omits `{}`**), the flip is safe only *through* the both-forms parser, **never** a raw-line comparison. **T8 makes the scrape label-AWARE or states the limitation explicitly.**

⚠️ **FAILURE-ATTRIBUTION:** `0118` break failures report at **`runner_test.go:1349`**, not `driver.go:308` — `t.Helper()` plus the `fixture.TB` indirection collapses the driver frames. **A gate grepping a `driver.go` line number will not match.**

**Gate:** `-run 'TestDifferential/0118-runtime-static-layer' -count=1`, RED-first, with the **`-run` no-match footgun negative-controlled** — a no-match exits **0** printing `[no tests to run]`, so confirm the subtest ran **by name**.

⚠️ **§6.1 mid-execution split trigger is LIVE on this task too** — ceiling mechanism (b).

---

## Task 9 — `BEHAVIOR_CONTRACT.md`

⚠️ **This section exists because the BRAINSTORM's edit-site table had NO `BEHAVIOR_CONTRACT.md` row at all**, and `:5020` is **mandatory on the `+0` path**.

1. **`:5020` — MANDATORY.** The phase-77 departure note says the two `runtime.*` gauges are *"SILENTLY ABSENT from `/stats/prometheus`"* and that *"the day `internal/stats` learns `runtime.`, fixture 0118 goes RED on purpose, naming the follow-up. A prose-only deferral would have rotted; this one cannot."* ⚠️ **Anchor on the WHOLE sentence, not on "closes with" — two clauses follow the bolded one (§1.5 D5).** **Phase 79 is that day.** Record the departure **CLOSED for `runtime.`, `access_logs.` and `tracing.`** and **PERSISTING for `sds.`** pending 79.1. ⚠️ It also independently enumerates **five** arms — supersede that to **nine** (§1.1).
2. **A new stat-name-mapping statement** for the three arms, as the SN5-shaped byte-mirror rule, naming the **nine** recognized top-level segments — **and the four mid-name detectors, per T2's adjudication.**
3. **The `# HELP` departure stated AS A DEPARTURE** (§Task 6), not as parity.
4. ⚠️ **`tracing.zipkin.spans_dropped` named as envoy-go-only** so no later row writes an unsatisfiable cross-side assertion. **Nine of ten have a reference counterpart; one does not.** ⚠️ **Any text saying "these ten now match the reference" is wrong.** `tracing.opentelemetry.spans_dropped` **does** have a counterpart (`SPEC-46.1.md:19,203`).
5. **NO ledger row** — the surface is **+0**. Per SPEC §7.5, the non-H2 parallel total was **abandoned three rows ago** (newest `non-H2` figure is `:5010`, phase 47.1; tail rows `:5014`/`:5016`/`:5018` carry none), so a future `+1` row appends a single-absolute row and need not touch a non-H2 total.
6. ⚠️ The three tail ledger rows each carry a `[LEDGER GAPS — RECORDED, NOT RESOLVED]` block naming two unattributed steps (`1198→1200`, `1200→1201`) and each says the absolute *"must be re-derived MECHANICALLY"*. **Any future row that moves the ledger inherits that warning verbatim.**

**Gate:** grep-verified per site, each with a firing negative control against a doctored scratch copy.

---

## Task 10 — the break roster, each arm proven to fire its OWN assertion

⚠️ **Breaks run AFTER committing** (`reference_break_protocol_commit_first`) and with **`-count=1`** (`reference_differential_break_protocol_count1`). ⚠️ **PLAN break INSTRUCTIONS don't compile** (`reference_plan_break_instructions_dont_compile`) — where an arm below does not compile as written, substitute an equivalent and **record the substitution**. ⚠️ **A break arm's INJECTION SITE is itself a claim** — vary it (done for the sink NC: two sites, identical results).

| arm | edit | must fire |
|---|---|---|
| α | delete the `runtime.` arm | T1 rows for `runtime.*`; **T8** `0118` parity |
| β | delete the `tracing.` arm | T1 rows for `tracing.*`; **T7 GOLDEN 1-4** |
| γ | **tag-hoisting** `tracing.` arm instead of byte-mirror | **T7 across all four consumers** (110 diff lines, proven at PLAN time) |
| γ′ | tag-hoisting on **`access_logs.`** (site varied) | same — proven **not** site-dependent |
| δ | revert the `:350` string | **T2** byte-stable guard only |
| ε | delete the `WriteProm` log | **T5 legs 1 AND 2** (proven: NC-2) |
| ζ | log every name, aggregate | **T5 leg 3 ONLY** (proven: NC-1a) |
| η | log per-name (2 lines) | **T5 legs 1 AND 3** (proven: NC-1b) |
| θ | delete one `helpText` entry | **T6 Guard A AND Guard B** |
| ι | typo a `helpText` key **and copy the typo into the golden** | ⚠️ **T6 Guard B ONLY — Guard A is BLIND** (the decisive row) |
| κ | run GOLDEN 4 on the `(F,F)` cell only, under arm γ | ⚠️ **NOTHING fires — the vacuity demonstration** (§1.2) |

⚠️ **Arm κ is a NEGATIVE result and the most valuable line in the roster** — it demonstrates that a plausible, natural gate configuration **cannot fail**. Record it whether or not it is shipped as a test.

**Gate:** each arm red on its **named** assertions (confirm WHICH fired — `reference_deliberate_break_wrong_assertion`), restore green.

---

## Task 11 — the gates

⚠️ **THE FULL 120-FIXTURE DIFFERENTIAL IS MANDATORY** — `internal/stats` links into `cmd/envoy-go`, built at **THREE** sites: `test/differential/harness.go:240`, `:594`, and `test/conformance/h2spec/h2spec_test.go:210` (`TestH2Spec` entry `:30`). ⚠️ **h2spec is a FOURTH consumer of the same binary and is NOT covered by `./test/differential/`** — run it **explicitly** rather than excluding it silently. ⚠️ **And §1.8's ~30-fixture prometheus blast radius is the substantive reason**, not just linkage: every subject's `/stats/prometheus` gains 6 lines. Budget **~400-420 s** per green attempt; **`-race` is a SECOND run, not a substitute.**

```sh
( go test ./test/differential/ -count=1 -v > "$SCRATCH/full.log" 2>&1; echo "INNER_EXIT=$?" )
grep -cE '^    --- PASS: TestDifferential/' "$SCRATCH/full.log"          # want 120
grep -E  '^    --- (FAIL|SKIP): TestDifferential/' "$SCRATCH/full.log"   # want EMPTY
grep -c  'no driver registered for fixture' "$SCRATCH/full.log"          # want 0
grep -o  'TestDifferential/[^ ]*' "$SCRATCH/full.log" | sed 's|TestDifferential/||' | sort -u \
  | comm -3 - <(ls -1 test/fixtures/ | grep -E '^[0-9]{4}[a-z]?-' | sort)  # want EMPTY
```

**Each clause is grounded in a recorded failure mode:** `-count=1` (a cached PASS is not a run) · `-v` (**without it a green log is indistinguishable from a suite that ran nothing**) · **capture the INNER exit code** · tally scoped to `TestDifferential/` so the bare parent line is excluded · **the `comm -3` cross-check is the load-bearing gate, not the raw count** (120 with one fixture renamed and another skipped still reads 120) · assert `SKIP` empty explicitly, since `t.Skipf` on an unregistered driver is the silent-green path. **Verified at this tip: fixture dirs 120, blank imports 120 on the FULL `^\t_ "github.com/pgdad/envoy-go/test/fixtures/` prefix (naive `^\t_ ` ⇒ 126), `comm -3` EMPTY.** ⚠️ The faithful dir predicate is `^[0-9]{4}[a-z]?-` — a bare `^[0-9]{4}-` gives **118** (`0007a-cors`, `0007b-iteration-probe`).

**Plus:** `gofmt -l` **output** empty and `golangci-lint run` on **four** touched packages (`internal/stats`, `internal/statssink`, `internal/admin`, the `0118` driver) · `go test ./internal/... -count=1` · `go test ./internal/stats/... -count=1 -race`.

⚠️ **THE `+0` STAT-SURFACE ARGUMENT MUST BE STRUCTURAL, AND THE `TestNoNewStat*` BLINDNESS IS PROVEN.** All five guards live in `internal/statssink/registration_test.go` (`:26 :53 :81 :109 :137`, `package statssink`, the exhaustive repo-wide set) and none reaches `internal/stats`. Re-executed with a failing baseline: **Arm A** — a genuine new registration injected inside `internal/stats` ⇒ all five `--- PASS`, `GUARD_EXIT=0`, ⚠️ **and the same build gave `DEADLOCK_PROBE_EXIT=1`** — they are blind not only to a stat-surface regression but to a **process hang in the code they nominally cover**. **Arm B** — the same registration moved to `NewRegistry()` ⇒ all five `--- FAIL`, `GUARD_EXIT=1`. ⇒ Arm A's green is **genuine blindness, not a no-run**, and **the blindness cuts both ways**. **Enumerate the diff's registration call sites and show the set is empty.** Adding a `switch` arm is a **PROJECTION** change, not a **REGISTRATION** change.

**Stat surface 1207 is DOCUMENTARY and CONFIG-CONDITIONAL — assert the DELTA, never the absolute.**

⚠️ **Known live hazards — never reflex-classify any as a regression.** The full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **before any assertion**, the latter a **PANIC that can abort the whole binary**, firing more readily under `-race` and as the fixture count grows — now **120**; hardened at `0e9cc680` **for the SUBJECT PROXY ONLY**) · `reference_sds_init_fetch_timeout_dial_budget_flake` (**TWO** packages) · the pre-existing `internal/cluster` `-race` outlier flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce`) · `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery` (**FIRED at the SPEC**; `go list -deps` shows that package has **zero** dependency on `internal/stats`; isolated re-run green) · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`. ⚠️ **A stage brief's flake list is not the index.** Isolate-re-run, then state the classification **and its evidence**. ⚠️ **`0061-lb-ring-hash` is NOT a live flake — a spread failure there is a FINDING.**

⚠️ **Gate hygiene — the lineage's broken-gate count is FOURTEEN**, enumerated in §8.

---

## Task 12 — ADR-0301, row 79 → `done`, and the stage close

1. **ADR-0301 §Decision + §Consequences appended IN PLACE** after the retained footer (**ADR-0044-as-used**; ⚠️ **ADR-0044 does not itself contain that discipline**). **No renumber, no `---` separator.** §Context is landed (`DECISIONS.md` **17628** lines, tail **ADR-0301 PROPOSED**, `^## ADR-0302` ⇒ **0**). Next-free stays **ADR-0302**. ⚠️ **ADR-0301 carries NO whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297.
2. ⚠️ **The ADR range-extraction hazard is REFUTED at this tip** — `^## ADR-0107` matches **1** line (`:4304`); `:4858` begins `##` at byte 0; **zero** lines begin space-then-`#`. **`^## ADR-` is a safe anchor.** *(ADR-0209 has no heading by design; `## ADR-0127 v2:` at `:5973` is the sole heading matching neither form.)*
3. **Row 79 → `done`** in `ROADMAP.md`. Row 79's shape is canonical and re-verified at this tip: **7 pipes, `NF=8`, field 8 EMPTY, pipe-terminated.** `want` **STAYS 111** — this row adds no row.
4. ⚠️ **THE LEAK-CHECK RE-ARMS AT THE IMPL.** The new cell text must contain **ZERO** occurrences of the two deferred-candidate phrases and no unregistered `<Family>-family row` slug, **each grep negative-controlled against a deliberately doctored copy.** ⚠️ **`grep` cannot tell a mention from a use** — writing a sentinel matcher string into the file the sentinel greps silences the check **BY MENTION**.
5. **Sentinel re-run AFTER the ROADMAP edit**, all three checks, with firing negative controls. ⚠️ **This row narrows NOTHING** — check (2) has **never** gone down across ~22 phases; the `:209` candidate sentence keeps its text until the row closes. **State it, do not forecast it.**
6. **`STATE.md` §Current rolled IN PLACE** (ADR-0288). ⚠️ **§Recent lineage is at its five-entry cap — a close that adds a bullet MUST evict the oldest to `STATE_HISTORY.md`** (§1.4). Roll `next-prompt.txt`.
7. **The six-gate:** `BOOTSTRAP_PROMPT.md` **§7.5**, heading **`:357`**, gates (a)-(f) at **`:360-365`**, closing sentence **`:367`** — at the **repo root**, with the `.md` extension. ⚠️ **ADR-0106 does NOT define the six-gate** (it governs the SOLE-leg property). ⚠️ **ADR-0045 (`DECISIONS.md:1466`) does not STATE the split gate — it QUOTES it at `:1475`**; ⚠️ **and the NORMATIVE statement is `BOOTSTRAP_PROMPT.md` §6.1 at `:285-290`, which the `25 tasks`/`1500 LoC` grep misses** (§1.3). **Citing the figures to ADR-0045 alone is a laundered cite.**

---

## 4. Band — **10–13, budget 12** — and the disagreement IS the headline

**FLOOR 10 — what collapses:** T3 folds into T2 (same defect species, one commit); T6's two guards fold into one task **if** the derived roster lands clean first try (not free — the SPEC records an invented-input attempt that already produced a false positive on 6 names). **Below 10 is indefensible:** T11 and T12 cannot merge (phase 76 merged them, but row 79 **re-arms** `reference_sentinel_matcher_string_self_clears` at the ROADMAP flip, making the sentinel a strictly-ordered *second* gate pass), and T1/T2/T4/T5/T7/T9/T10 are seven independent seams in seven files with seven different gate commands.

**CEILING 13 — three named, mutually independent mechanisms, each +1:**
- **(a) T7 splits OTLP off.** Three of four sinks are no-op **by construction** (§1.2); OTLP is the only consumer reaching the new state by a different route, and it carries the 2×2 cross-product **and the inert cell**. A separate OTLP golden is a real +1.
- **(b) T8 splits.** Label-aware `scrapeProm` is a rewrite of a **52-line** parser (`:407-458`) that five other assertions in the same driver consume; if any existing key moves, the flat-`/stats` legs and the reference-side legs must be re-proven.
- **(c) T11 becomes two** if the full suite or `-race` reds and the classification needs its own evidence pass. Phase 76's IMPL recorded *"G7 — `-race`: a REAL failure on the first attempt"*; phase 77's IMPL took the same shape.

⚠️ **AND `BOOTSTRAP_PROMPT.md` §6.1's THIRD trigger — mid-execution split when a task's sub-steps exceed ~10 — is LIVE on T7 and T8.** That is the mechanism by which 13 actually fires, and it is written into the gate itself. **A split at 13 is a planned outcome here, not a phase-78-style surprise.**

**LoC: ~875, not ~500** (§1.3). **ADR-0045 does NOT trip**: 12–13 ≪ ~25 (~2× margin), ~875 < ~1500 (**~1.7× margin — "inside", not "≪"**).

⚠️ **AGREEMENT WAS NOT RELIED ON.** The band derivation ran behind an **anchoring firewall** and the agent **disclosed two partial breaches** rather than laundering a clean attestation: the SPEC's §9 *heading* carries the band, so a section-map grep leaked `10-12`/`9-11`, and §14 leaked `11-13`. It therefore **claims no independence for the digits** and rests on what is code- and `numstat`-derived: the 12-task decomposition, the three ceiling mechanisms, the no-op-by-construction finding, the §6.1 cite, and the LoC refutation. **That disclosure is why the LoC result is the load-bearing half of this section and the task number is not.**

---

## 5. Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

- **(1)** `NOT DONE: row 79` — **no `GATE FAIL` at `want=111`.** NCs: `want=109`, `110`, `112` each ⇒ `GATE FAIL: examined 111 data rows, expected <want>`.
- **(2)** **FIVE**, at `:189 :199 :209 :215 :223` — **UNCHANGED. This row narrows NOTHING**, stated rather than forecast.
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NC: an invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`.
- Input measured: **227 lines / 1 003 291 bytes / 13** bare `candidates:` hits (vs the sentinel's narrower 5) — so an empty result could not read as a zero result.

⚠️ **`want` STAYS 111. The PLAN adds no row and does not touch it.** ⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage**, so the matcher-leak hazard is dormant for one stage; **it re-arms at the IMPL** (Task 12.4).

---

## 6. Counts at this tip — each with a firing negative control

| axis | value | negative control observed |
|---|---|---|
| differential fixture dirs | **120** (next-free `0119`) | probe dir ⇒ 121 |
| `runner_test.go` blank imports (FULL prefix) | **120** | delete one ⇒ 119; naive `^\t_ ` ⇒ **126** |
| fuzzers | **55** | appended `func FuzzNegativeControl` ⇒ 56 (on a scratch copy) |
| internal packages | **73** (`find` and `go list` agree) | +1 pkg dir ⇒ 74; a `.go`-free dir ⇒ stays |
| `DECISIONS.md` | **17628**, tail **ADR-0301 PROPOSED** (`:17598`, STATUS `:17600`) | `^## ADR-0302` ⇒ 0; `^## ADR-0301` ⇒ 1 |
| `ROADMAP.md` | **227 lines / 111 data rows** | +1 row ⇒ 228 / 112 |
| `BEHAVIOR_CONTRACT.md` | **5762** | — |
| `STATE_HISTORY.md` | **414** → **416** at this commit | — |
| `BC` `### Does not yet apply to` | **14** (not 15) | 18 total mentions; the 4 non-headings enumerated |
| `BC` ledger | **27** rows `:4966`-`:5018` under `:4962`, + a 28th detached at `:805` | 74 file-wide `^\*\*Phase ` |
| `BC` `non-H2 \*\*[0-9]+\*\*` | **11**, newest `:5010` | `non-H3` ⇒ 0 |
| stat surface | **1207** — ⚠️ **DOCUMENTARY + CONFIG-CONDITIONAL; assert the DELTA** | no mechanical command exists |
| BackendKind | **tail 38** | ⚠️ a TAIL VALUE — 39 constants, `TCPEcho = 0`; do NOT "fix" to 39 |
| go.mod modules | **2** (phase-61.2 lineage figure) | ⚠️ NOT a repo total — the single `go.mod` requires **67**; do NOT "fix" 2 to 67 |
| next-free reference port | **10119** | `10118` ⇒ 5 hits; `10119` ⇒ 0 |
| non-test `ExtractTags` call sites | **5** | `ExtractTagsZZZ(` ⇒ exit 1; `flattenToProm(` ⇒ 64 |

---

## 7. Deferred — named so no later stage re-derives them

1. **Row 79.1** — the `sds.` label-hoisting arm (**20** of the 30 names) + registration-time validation (~7-9). ⚠️ The `sds.` arm is the one that **BREAKS** §3.7's no-op result, and that is now **executed** rather than argued: the hoisting NC moved all four sinks. ⚠️ Per ADR-0300 §Consequences (ii), any injected-defer probing of the validation leg **MUST VARY THE INSERTION POINT** across the pre- and post-anchor windows. ⚠️ **§3.2's secret attribution is SWAPPED — `edf_validation_ca` is `0110`, `rccf_validation_ca` is `0111`** (§1.7). Do not inherit the SPEC's order.
2. **The `# HELP` block-level departure** — the reference emits none; envoy-go emits one per group. Named, not closed.
3. ⚠️ **NEW: the `STATE_HISTORY.md` archive gap — 40 evicted lineage bullets were deleted, not archived** (§1.4). Phases **67–75 wholly absent**; 76 partial. ADR-0288 §Decision 3 mandates the move. **NOT FIXED here** — a docs-only PLAN on a projection row is the wrong place, and doing it under a phase-79 commit message would be its own §1.1 species. **A future maintenance row should backfill from `git log` and state the denominator.**
4. ⚠️ **`STATE.md` §Project counts is STALE ON FOUR AXES and self-contradicts §Current inside the same file** (§1.5 D6). **Deliberately NOT repaired — repairing a count by editing the sentence that states it is how the ADR-0296/0297 species starts. Anchor on §Current, which IS live.**
5. **The documentary defects, unchanged:** the non-existent public import path (**36 live occurrences across SEVEN files**; ⚠️ all 8 root `PROGRESS.md` hits are **pasted `go test` output**, so rewriting them is a different doctrinal act from correcting a statement; `DECISIONS.md:142` is an ADR that *decides* the wrong path and was never superseded; ⚠️ fix by PATTERN `esalaine/envoy-go` **bare**, not the `/validate` form) · a mechanical stat-surface count (8-11 tasks) · the unresolved half of the `BEHAVIOR_CONTRACT` stale-cite claim (`internal/tls/config.go:272`, `internal/filter/http/chain.go:19` flagged, **not audited**) · `BEHAVIOR_CONTRACT.md:501`'s SN9 collision with ADR-0118's actual SN9 · **`ADR-0299`'s STATUS line still reads `PROPOSED`** although its §Decision and §Consequences landed at the phase-77 IMPL.
6. ⚠️ **`ROADMAP.md`'s row 78 is the ONLY MALFORMED ROW of 111** — `NF=8` with 2 877 characters of IMPL summary in field 8 and **no trailing `|`**, so GFM drops the entire phase-78 IMPL summary from the rendered table. **NOT FIXED**: the §Schema invariant at `:18` forbids editing a closed row's `summary` cell.
7. **Normalizing the Operational-tooling short-form deferred-candidate paragraph** to the long form — a named, deliberately-untaken follow-up.
8. **The WASM row-summary rider stays DECLINED on the merits** — `ROADMAP.md:76` declares phase 25 the FINAL §9 HTTP-filters-family row and rows 25.x use "family" 23 times to declare it CLOSED, so registering WASM is a **doctrine adjudication against a landed closure statement**, and writing the marker would silence check (3) **BY MENTION**. **Do not re-adopt it as cheap.**
9. **Symmetric bind hardening** — `mustAllocatePort()` in **10** drivers; the racy helper copy-pasted under FOUR names, ~15 definitions. *"Fixture BACKENDS have no retry"* is OVERBROAD: only `fixture.HTTPSH2` (`runner_test.go:288-293`) can close-then-rebind race. **Still rejected: it CANNOT be verified by a green suite run** (`0e9cc680` needed `-count=6`).
10. **Opening the gRPC family — HARD-BLOCKED.** `\.RunEncodeTrailers(` ⇒ **0** non-test, 1 test; ⚠️ the DECODE side is equally dead. **The entire trailers pair is unreachable from production code.** Both charter-satisfying candidates sit behind it: **16-22+ tasks.**

---

## 8. Gate hygiene — the lineage's broken-gate count is **FOURTEEN**

A full-suite recipe **without `-v` is VACUOUS** · a sha256 roster built from one tree is **desynced BY CONSTRUCTION against a DELETED file** · **`gofmt -l` NEVER exits non-zero** (gate on OUTPUT) · `go doc -all <A> <B>` **silently swallows arg2** · a **`+0 exported symbols`** gate over an EMPTY package goes RED on a CORRECT tree · a **RANGE** gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` **exits 0 on a DELETED file** · a **count-only** stat guard **PASSES a build with BOTH names wrong** · a **`-run` no-match exits 0** with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests **exceeds the fixture denominator** · a stat-delta claim **cannot be discharged by guards scoped to another package** · a **stderr-VOLUME** assertion **passes on the hang** · **`golangci-lint` runs `misspell` with `locale: US`** (⚠️ **LIVE this row**) · **a harness's exit code is not the command's** — capture `INNER_EXIT`.

⚠️ **AND A FIFTEENTH, FOUND AT THIS STAGE: A GATE CELL THAT CANNOT FAIL.** T7's OTLP `(useTagExtractedName=F, emitTagsAsAttributes=F)` cell is **byte-identical under the hoisting negative control** (§1.2). A golden exercising only the default knobs is **vacuous** — it is green on the correct tree **and** green on the wrong one. **This is distinct from every prior entry: the command is right, the assertion is right, and the *cell* is inert.** ⚠️ **The only way this was found is that the negative control was run across the full cross-product rather than on the default configuration** (`reference_probe_must_discriminate`).

---

## 9. Self-review against the SPEC

**Adopted unchanged:** the three-arm byte-mirror decision (D-SPP-1, closed against a live reference) · D-SPP-3's **LOG** resolution and the `sync.Once` rejection · D-SPP-5's **CONVERT-not-delete** · the two `helpText` guards and the decisive blind row · the stacked-control design and its capture mechanism · `+0` unconditional and the structural argument for it · the mandatory `BEHAVIOR_CONTRACT.md:5020` edit · the nine-segment enumeration · the full-differential mandate.

**Refuted or corrected (eleven):** §10's *"Fixture (1 file)"* → **four** (§1.1) · §3.7's NC total **152** → **110** (§1.2) · §3.7's PROM bytes **304→2171** → **380→2246**, not pinnable · §3.7's OTLP **1154→1292** → absolutes not reproducible, **+138 confirmed at one cell only** · §3.3's **1151 B** → **1180 B**, and the format string is unpinned (§1.7) · §3.6's per-arm leg attribution → **fired-leg SETS**, and *"different legs"* partially refuted · §3.8's `manager.go:381 normalizeAddr` → **`:352`** (§1.5 D3) · §3.5 / §10 / router's `scrapeProm :407-447` → **`:407-458`** (D1) · §3.1's *"the existing parser `:399-403`"* → **`:429-443`**; the cited range is a comment (D2) · §3.2's secret→fixture attribution **swapped** · §9's **~500 LoC** → **~875**, and *"≪ ~1500"* → *"inside, ~1.7× margin"* (§1.3).

**Added, that no phase-79 document carries (six):** the inert OTLP gate cell · the whole-suite-green-under-NC result · the no-op-by-construction structure of three sinks · `BOOTSTRAP_PROMPT.md` §6.1 as the normative gate **plus its mid-execution trigger** · the `WriteProm` fuzz-target caller · the ~30-fixture prometheus blast radius. **Plus one outside the row's subject:** the 40-bullet `STATE_HISTORY` archive gap (§1.4).

---

## 10. Operative memories

`feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `feedback_push_to_origin` · `feedback_pertask_gofmt_lint` · `reference_bash_cwd_reset_commits_to_main` · `reference_parallel_subagents_private_scratch` · `reference_parallel_agents_shared_machine_namespaces` · `reference_break_protocol_commit_first` · `reference_differential_break_protocol_count1` · `reference_deliberate_break_wrong_assertion` · `reference_break_arm_injection_site_is_a_claim` · `reference_plan_break_instructions_dont_compile` · `reference_positive_arm_cannot_catch_overfiring` · `reference_liveness_break_needs_failing_baseline` · `reference_probe_must_discriminate` · `reference_probe_input_is_a_claim` · `reference_independent_probes_can_share_a_blind_axis` · `reference_empty_output_is_not_a_zero_result` · `reference_output_volume_is_not_output_content` · `reference_sample_is_not_an_audit` · `reference_stat_count_guard_blind_to_rename` · `reference_gate_command_negative_control` · `reference_fatalf_makes_assertions_unreachable` · `reference_a_drift_correction_is_itself_a_claim` · `reference_stale_cite_recurs_fix_by_pattern` · `reference_document_hygiene_claim_not_evidence` · `reference_sentinel_matcher_string_self_clears` · `reference_deferred_candidate_cost_restale` · `reference_golangci_misspell_locale_us` · `reference_registry_walk_lock_inversion` · `reference_nil_stats_counter_inc_crashes_goroutine` · `reference_differential_run_selector` · `reference_differential_fullsuite_startup_flake` · `feedback_brief_citations_not_evidence` · `reference_next_prompt_tracked_despite_gitignore`
