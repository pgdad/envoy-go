# PROGRESS 79 — stats-prometheus-projection

Companion to `PLAN.md`. The PLAN half below is written at the PLAN stage; the `# IMPL record` half is appended at the phase-79 IMPL.

---

# PLAN record (2026-07-28)

**Stage:** PLAN. Lifecycle-state **2 → 3**. **ROW 79 STAYS `in-progress`.**
**Base:** master `9b2ef891`. Worktree `/home/esa/git/envoy-go-wt-p79plan`, branch `phase79-plan`.
**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + **`STATE_HISTORY.md`** + `next-prompt.txt`.
⚠️ **FIVE files — ONE MORE than the phase-76/77/78 PLAN precedent, and the reason is a finding, not an accident.** Those three PLAN commits were each exactly FOUR files and **none** touched `STATE_HISTORY.md` (`git show --stat` on `9b0077b9`, `acedfd2b`, `62588ade`). This one is five because §Recent lineage is at its five-entry cap **and the bullet this stage evicts has never been archived** — see the archive-gap finding below.
**BYTE-UNTOUCHED at this stage:** `ROADMAP.md` · `BEHAVIOR_CONTRACT.md` · `DECISIONS.md` · **ZERO `.go` files.**

**Dispatch:** four investigation agents on disjoint remits, each with a **private detached worktree**, **private scratch** and a **banded port range** (A2 41000-41099, A4 41200-41299 — both **outside** the differential harness's 20000-31007 range), plus controller re-derivation. **No docker** — D-SPP-1 is discharged. All four worktrees finished `git status --porcelain` **EMPTY**; no commits, no pushes, no unscoped `git restore`.

## What was EXECUTED at this stage

| # | thing | outcome |
|---|---|---|
| 1 | The **mandatory §3.7 hoisting negative control**, real wire bytes, **two** injection sites, four consumers, OTLP 2×2, determinism proven first (two BEFORE runs, `diff` exit 0, 17 473 B) | **FIRES on all four consumers, both sites** — but **110** diff lines, not 152, and **one OTLP cell is INERT** |
| 2 | The whole suite **under** the hoisting arm | `ok internal/stats · internal/stats/dynamic · internal/statssink`, **INNER_EXIT=0** — nothing in the tree can tell byte-mirror from tag-hoisting today |
| 3 | The `WriteProm` counter **deadlock**, `go test -timeout 20s` | reproduced; goroutine trace names **both** lock sites |
| 4 | The **discriminating control** — the same counter pre-registered eagerly outside `Walk` | `--- PASS`, `INNER_EXIT=0` ⇒ mechanism confirmed **and** the escape hatch is sound |
| 5 | `TestNoNewStat*` blindness with a **failing baseline**, both arms | Arm A all five PASS **on a build that deadlocks**; Arm B all five FAIL |
| 6 | The `:350` string swapped to the nine-segment form, built and suite-run, reverted | build 0 · `./internal/stats/ -v` **87 PASS / 0 FAIL / 0 `no tests to run`** · `./internal/... ` **70 `ok`** · **sha256 byte-identical** after revert |
| 7 | The **four-arm `helpText` matrix**, roster **derived** from `normalizeAddr` | the decisive blind row reproduced: Guard A **PASS-BLIND**, Guard B **FAIL** |
| 8 | The **four-arm observability matrix**, `-race` | fired-leg **SETS** recorded; two SPEC attributions incomplete |
| 9 | The **48-token nine-segment acceptance probe** + a 46-line static audit | **NINE confirmed**, no tenth, none spurious |
| 10 | A **210-key `STATE_HISTORY` archive audit** over the whole `STATE.md` commit history | **40 evicted bullets in NEITHER file** |
| 11 | `ROADMAP.md:209` read with **character** offsets | six task bands; this row's banked figure is **`11-14`** at char 45763 |
| 12 | Every anchor and count in SPEC §10 and router items 7/13/16 | **seven drifts**, three propagated into three documents each |
| 13 | All three sentinel checks, with firing negative controls | does NOT fire; `stop` NOT created |

## THE HEADLINE: THE ROW'S OWN FIX IS INCOMPLETE IN EXACTLY THE WAY THE DEFECT IT REPAIRS IS

SPEC §1.3's point is that the BRAINSTORM's repair for the stale error string *"reproduces the same defect one generation on"* — it fixed the number without auditing where the number lives. **SPEC §10's edit-site roster commits the same error one level up.** It says **"Fixture (1 file)"**. The stale enumeration lives in **FOUR**: `driver/driver.go:154-156`, `expectations.yaml:43-46`, `envoy.yaml:16`, `README.md:32-35`.

And `driver.go:155-156` is wrong **twice** in a way no document names: it omits `wasm.` **and** `kafka.`, and it lists **`rbac.` as a top-level prefix** when `.rbac.` is an **INFIX** detector. Demonstrated live against a byte-identical copy — `ANYTHING_AT_ALL.rbac.allowed` parses clean, and `ANYTHING_AT_ALL` appears nowhere in the source.

⇒ **three mutually inconsistent wrong counts are live in-tree**: `name.go:350` says **four**, `BEHAVIOR_CONTRACT.md:5020` says **five**, `driver.go:155`/`expectations.yaml:44` say **four plus a wrong list**. The PLAN promotes the sweep to **its own task (T3)** so it cannot ride as a footnote.

## THE SECOND HEADLINE: A GATE CELL THAT CANNOT FAIL — BROKEN-GATE SHAPE **FIFTEEN**

The mandatory NC fires on all four sinks for both injection sites, with verbatim wire bytes reproducing the SPEC's claims (`|#envoy.tracer_name:opentelemetry`, `;envoy.tracer_name=opentelemetry`, `label:{name:"envoy.tracer_name"}`). **But inside OTLP it is cell-dependent:**

| cell `(useTagExtractedName, emitTagsAsAttributes)` | byte-mirror | under NC | Δ |
|---|---|---|---|
| **(F, F)** | 1144 | **1144** | **0 — CANNOT FAIL** |
| (F, T) | 1210 | 1348 | +138 |
| (T, F) | 1128 | 1086 | **−42 — SHRINKS** |
| (T, T) | 1194 | 1290 | +96 |

⇒ **a golden on the default OTLP knobs is VACUOUS** — green on the correct tree *and* on the wrong one — and a "bytes must grow" assertion goes RED for the wrong reason on `(T, F)`. **This is distinct from all fourteen prior broken-gate shapes: the command is right, the assertion is right, and the CELL is inert.** It was found only because the NC was run across the full cross-product rather than on the default configuration.

**And the result that makes T7 load-bearing:** with the hoisting arm applied, `go test ./internal/stats/... ./internal/statssink/... -count=1` is **INNER_EXIT=0**. Nothing in the tree today distinguishes a byte-mirror arm from a tag-hoisting one.

⚠️ **Structural corollary that explains why:** `label.go:39`'s guard is `if err != nil || len(labels) == 0`, and `dogstatsd.go:86` / `graphite.go:70` fall back to literally `residual, labels = fam.GetName(), nil` — **byte-identical to what a byte-mirror arm returns**. Three of four sinks are no-op **by construction**. **`otlp.go:189-197` is the only consumer reaching the new state by a different route** — and it is the one with the inert cell. The expensive consumer and the fragile gate are the same one.

## THE COST: **10–13, BUDGET 12** — and the LoC estimate is REFUTED

Floor and budget agree with SPEC §9. **The ceiling does not.** §9 prices the probability that any task splits at **zero** (*"ceiling 12 as enumerated"*), in a lineage where the immediately preceding row did exactly that.

**The LoC refutation is a measurement, and it is the load-bearing half.** SPEC §9 says ~500 LoC, *"≪ ~1500"*. Realized `.go` in the IMPL squashes (located by subject; all three PLAN commits carry 0 `.go` files, so the IMPL squash *is* the row's code):

| phase | that PLAN's own estimate | realized `.go` | ratio |
|---|---|---|---|
| 77 (`4d7f63c2`) | ~700 | **+1428 / −22** | **2.04×** |
| 78 (`3a4c8cfa`) | ~330 | **+495 / −24** | **1.50×** |

⇒ **~875**, converging with an independent bottom-up build-up. Corroborating: §9 budgets ~300–400 test lines **in total**, while T7 alone is ~350 against existing `internal/statssink` goldens of 419/430/471/542/773 lines. **One task plausibly consumes the whole stated test budget.** The gate still does not trip — but **"≪" becomes "inside, ~1.7× margin."**

⚠️ **The NORMATIVE split gate is not where §9 cites it.** `:225` and `:472` are flow reminders; **`BOOTSTRAP_PROMPT.md` §6.1 at `:285-290`** is normative, and the `25 tasks`/`1500 LoC` grep **misses it** (different numeral spelling). ⚠️ **§6.1 carries a THIRD trigger no prior stage discharges** — mid-execution split when a task's sub-steps exceed ~10 items. **LIVE on T7 and T8.** That is how the ceiling of 13 actually fires.

**Calibration re-derived first-hand from primary documents:** 76 SPEC 7–9 → PLAN **9** (**AT**) · 77 SPEC 11–13 → PLAN **12** (**INSIDE**) · 78 SPEC 7–9 → PLAN **10** (**ABOVE**). **Two-for-three at or above** — reproducing SPEC §1.4's R11 correction against the BRAINSTORM's *"three-for-three"* mislabel.

⚠️ **The band derivation ran behind an anchoring firewall and DISCLOSED TWO PARTIAL BREACHES rather than laundering a clean attestation** — the SPEC's §9 *heading* carries the band, so a section-map grep leaked `10-12`/`9-11`, and §14 leaked `11-13`. It therefore claims **no independence for the digits** and rests on what is code- and `numstat`-derived. **That is why the LoC result is the load-bearing half of the cost section and the task number is not.**

⚠️ **The banked `ROADMAP.md:209` figure is `11-14`**, read with **character** offsets (the line is 45 959 chars / 46 423 bytes, so `cut -c` truncates mid-word) — and it covers **four** arms including the deferred `sds.` one. **9-11 and 10-12 are both re-scopes downward; 10-13 sits between them.**

## THE ARCHIVE GAP — 40 LINEAGE BULLETS WERE DELETED, NOT ARCHIVED

ADR-0288 §Decision 3: *"the sixth is moved to `STATE_HISTORY.md`."* §Consequences predicts the failure: *"A new discipline a session can get wrong… not enforced by tooling."*

Audited mechanically — all 210 distinct `- **prior active-phase:**` keys ever **removed** from `STATE.md`, checked against today's `STATE_HISTORY.md` **and** today's `STATE.md`: **40 are in neither.** `STATE_HISTORY.md`'s archived phase range is `38 … 66, 76, 77, 78` — **phases 67 through 75 are wholly absent.** Negative-controlled both ways: every lost slug returns `HIST=0` under a *loose* slug grep, while `boot-panic-visibility` ⇒ 2 and `runtime-static-layer` ⇒ 1.

**Severity is bounded and stated:** ADR-0288 §Context establishes the content is triply redundant, so nothing is unrecoverable. **What is false is the archive's claim to be the complete record.**

⚠️ **It determined this stage's own file set.** The bullet this PLAN evicts is `STATE.md:54` (`phase 77 … SPEC done`), `grep -cF` in `STATE_HISTORY.md` ⇒ **0**. **So this PLAN archives it — hence five files.** The 40-bullet backfill is a **named deferral**, not fixed here.

## ANCHOR DRIFT — seven, three propagated into three documents each

| # | claimed | TRUE at `9b2ef891` |
|---|---|---|
| D1 | `scrapeProm` `:407-447` | **`:407-458`** — truncates 11 lines, cutting the timestamp strip, the `ParseFloat`/NaN guard **and `return out, nil`**. Wrong in `SPEC.md:157`, `SPEC.md:335`, `next-prompt.txt:107` |
| D2 | the both-forms parser at `driver.go:399-403` | **`:429-443`**. `:399-403` is **doc-comment text, zero executable lines** — SPEC §3.5's parity justification cites a comment |
| D3 | `manager.go:381` `normalizeAddr` | **defined `:352`** (doc `:331`); `:381` is its sole call site, first line of `registerListenerMetrics` (declared `:380`) |
| D4 | SPEC §10 "Fixture (1 file)" | **4** |
| D5 | `BC:5020` *"closes with …"* | that is the last **bolded** clause; two clauses follow |
| D6 | `STATE.md` §Project counts | **self-contradicts §Current inside the same file** — fixtures 119 vs 120, surface 1205 vs 1207, tail ADR-0298 vs ADR-0301 |
| D7 | `STATE.md:44` | says §Current carries the phase-79 **BRAINSTORM**; it carries the **SPEC**. **Repaired at this stage** |

**Not drifted** (re-derived, each confirmed): all 12 `name.go` anchors · the four `CutPrefix` cites `:286 :306 :323 :340` · all three `prom.go` anchors and the `continue`-not-early-exit correction · both `registry.go` lock anchors · all 11 `name_test.go` anchors and counts · all four `statssink` call sites · **the five-call-site denominator** · the five `TestNoNewStat*` guards as exhaustive · `admin/{stats,prometheus,admin}.go` · `tracing`, `listener`, `xds`, `boot` · `BC:5020`'s existence and its five-arm enumeration · every one of the 15 doc/count figures.

## FURTHER SPEC CORRECTIONS

- ⚠️ **§3.3's *"1 line / 1151 bytes"* is REFUTED AS STATED.** Under the SPEC's own message shape the 30-name roster emits **1180 B**. 1151 is reachable only with a bare-comma join; `", "` gives 1180 — **delta exactly 29, the separator count.** The SPEC never pins the format string or the separator, **so the figure is not reproducible from the document and a gate asserting it is unfalsifiable.** T4 pins both. The **1 line** half is confirmed.
- ⚠️ **§3.6's per-arm attribution is incomplete.** Fired-leg **SETS**: NC-1a → **leg 3 only** · NC-1b → **legs 1 AND 3** · NC-2 → **legs 1 AND 2**. ⇒ *"the two over-fire arms trip DIFFERENT legs"* is **partially refuted** — leg 3 fires in both. **But leg 3's justification gets SHARPER, not weaker: NC-1a fires leg 3 and nothing else, so without leg 3 it passes entirely.** The SPEC's conclusion — a positive-only test passes all three NCs — is **CONFIRMED**.
- ⚠️ **§3.7's PROM byte figures 304 → 2171 are NOT reproduced** — measured **380 → 2246** with a registration-derived roster. **Line counts 2 → 12 CONFIRMED.** Byte counts depend on the unspecified control roster and **must not be pinned**.
- ⚠️ **§3.7's OTLP `1154 → 1292` absolutes are not reproducible**; the **+138 delta is confirmed** and pins to exactly one cell.
- ⚠️ **§3.2's secret→fixture attribution is SWAPPED** — `edf_validation_ca` is **`0110`**, `rccf_validation_ca` is **`0111`**. The *set* is right, so the 30-name total and the 79.1 deferral are unaffected. Recorded so 79.1 does not inherit it.

## FINDINGS NO PHASE-79 DOCUMENT CARRIES

1. **The inert OTLP gate cell** (above) — broken-gate shape fifteen.
2. **The whole suite is green under the hoisting NC.**
3. **Three of four sinks are no-op by construction**; OTLP is the sole different-route consumer.
4. **`BOOTSTRAP_PROMPT.md` §6.1 is the normative split gate**, missed by the canonical grep, **and it carries a mid-execution trigger** live on T7/T8.
5. ⚠️ **`WriteProm` is called inside a FUZZ TARGET** — `internal/stats/fuzz_test.go:73`, inside `f.Fuzz(...)`. A skip-site `log.Printf` would fire **per iteration** under short-budget CI. The target synthesizes `listener.<addr>.downstream_cx_total`, which projects via SN3, **so it should not fire — but that is a PREDICTION and T5 executes it**, both directions. §3.3's *"only non-test **production** caller"* is true; the qualifier is load-bearing.
6. ⚠️ **The prometheus blast radius is ~30 fixture drivers, not one.** `setRuntimeStats` is called on **both** boot success paths including the `lr == nil` branch (`bootstrap.go:686`), so registration is unconditional and **every subject's `/stats/prometheus` gains 6 lines**. `grep -rln 'stats/prometheus' test/` ⇒ **62 files across ~30 fixtures**; no exhaustive-set or count assertion found, so probably benign — **but UNMEASURED, and only the full 120-fixture run discharges it.** This is why the full-differential mandate is right, **for a reason §9 does not state**.
7. **The 40-bullet `STATE_HISTORY` archive gap** (above) — outside the row's subject.

## THE NINE-SEGMENT CLAIM SURVIVES — but needs a SPECIES ADJUDICATION

Two methods with **different blind axes**: a static audit of all 46 lines consuming the input across `ExtractTags`' 314-line body (**9** root detectors, **4** infix, nothing else; NC on the pipeline exits 1) and a **48-token** live acceptance probe against a byte-identical copy (md5 verified both sides). Accepted set is exactly `cluster http kafka listener mongo redis server thrift wasm`. **No tenth. None spurious.**

⚠️ **But a name reaching `:350` has failed THIRTEEN detectors — nine top-level plus four mid-name** (`.http_local_rate_limit.` `:147`, `.http_bandwidth_limit.` `:185`, `.rbac.` `:236`, `.zookeeper.` `:265`). *"Nine recognized top-level segments"* is true about top-level segments and **incomplete about why the name was rejected**. **T2 must adjudicate this explicitly BEFORE drafting the byte-stable guard**, or the guard pins a **sixth** generation of an under-specified list — the exact failure mode §1.3 exists to prevent, one generation on.

## C7 NO LONGER RESTS ON THE SPEC'S DOCKER RUN ALONE

The reference zipkin family's lack of `spans_dropped` is corroborated in-repo by a **prior live probe two rows earlier**: `docs/envoy-go/phases/46-tracing/SPEC-46.2.md:127`/`:163` records `{reports_dropped, reports_failed, reports_sent, reports_skipped_no_cluster, spans_sent, timer_flushed}` — **verbatim the phase-79 SPEC's list**. `DECISIONS.md:16215` confirms `spans_dropped` is an envoy-go naming *decision*. `SPEC-46.1.md:19,203` records the reference **OTel** roster as `{spans_sent, spans_dropped, timer_flushed}`, so **`tracing.opentelemetry.spans_dropped` DOES have a counterpart** ⇒ **9 of 10 exactly right.** All `spans_dropped` hits under `test/` are `subjSpansDroppedStat` — **subject-side only** — so no unsatisfiable cross-side assertion exists today.

## The §3.7 denominator, re-derived with a MULTI-SPELLING search

**SET A** (`stats_sinks`) = **10** · **SET B** (tracers: `tracing:` / `http.tracers.` / `OpenTelemetryConfig` / `ZipkinConfig` / `tracing.v3` / `.trace.v3`) = **11** · **SET C** (gRPC/OTel access loggers: `access_loggers.{http_grpc,tcp_grpc,open_telemetry}` / `HttpGrpcAccessLogConfig` / `TcpGrpcAccessLogConfig` / `OpenTelemetryAccessLogConfig`) = **5**. **`A ∩ (B ∪ C)` = EMPTY**, denominator **120**. ⚠️ **B and C are both NON-EMPTY, and that is the negative control** — the greps demonstrably find these configs, so the **0** is real and not manufactured by one broken spelling.

`runtime.*` **IS** live-covered, by code path: `parseLayeredRuntime` calls `setRuntimeStats` on the `lr == nil` branch (`bootstrap.go:686`) as well as `:734`.

## Counts re-derived at this tip, each with a negative control

fixtures **120** (next-free `0119`; faithful predicate `^[0-9]{4}[a-z]?-` — a bare `^[0-9]{4}-` gives **118**, missing `0007a-cors` and `0007b-iteration-probe`) · blank imports **120** on the FULL prefix (naive `^\t_ ` ⇒ **126**) · fuzzers **55** (appended `func Fuzz` on a scratch copy ⇒ 56) · internal packages **73** (`find` and `go list` agree) · `DECISIONS.md` **17628**, tail **ADR-0301 PROPOSED** (`:17598`, STATUS `:17600`), `^## ADR-0302` ⇒ **0** · `ROADMAP.md` **227 / 111** · `BEHAVIOR_CONTRACT.md` **5762** · `STATE_HISTORY.md` **414 → 416** at this commit · `BC` `### Does not yet apply to` **14** (not 15) · `BC` ledger **27** rows + a 28th detached at `:805` · `non-H2 **N**` **11**, newest `:5010` · next-free reference port **10119** · non-test `ExtractTags` call sites **5** · stat surface **1207**, ⚠️ **DOCUMENTARY + CONFIG-CONDITIONAL — assert the DELTA** · BackendKind **tail 38** (a TAIL VALUE — 39 constants) · go.mod modules **2** (the phase-61.2 lineage figure, NOT a repo total — the single `go.mod` requires **67**).

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

- **(1)** `NOT DONE: row 79` — **no `GATE FAIL` at `want=111`.** NCs `want=109`/`110`/`112` each ⇒ `GATE FAIL: examined 111 data rows, expected <want>`.
- **(2)** **FIVE, UNCHANGED** — `:189 :199 :209 :215 :223`. **This row narrows NOTHING**, stated rather than forecast.
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NC: an invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`.
- Input measured **227 lines / 1 003 291 bytes / 13** bare `candidates:` hits vs the sentinel's narrower 5.

⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage**, so no matcher string can have leaked. **It re-arms at the IMPL** (PLAN Task 12.4).

## Hygiene

- Every git command used `git -C <abs-worktree-path>`. ⚠️ **The Bash cwd reset FIRED AGAIN — the EIGHTEENTH consecutive session** (`Shell cwd was reset to /home/esa/git/envoy-go`), observed live.
- Four agents ran concurrently with private scratch, private detached worktrees and banded ports **outside 20000-31007**; no collision. No docker containers created by any agent.
- Zero pushes by any agent. Zero commits in any agent worktree. No unscoped `git restore` anywhere. Every experimental edit reverted by **explicit path**; `name.go` verified **sha256 byte-identical** across the T2 edit/revert cycle.

---
