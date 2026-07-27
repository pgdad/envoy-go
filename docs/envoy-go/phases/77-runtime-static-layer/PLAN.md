# PLAN 77 — the bootstrap `layered_runtime` **static-layer** consumer

> **For agentic workers:** execute this plan task-by-task, red-first. Steps use checkbox (`- [ ]`) syntax. Each task re-derives its own anchors at the tip it is editing — **anchors drift within a phase's own tasks**, and *a drift CORRECTION is itself a claim* (`reference_a_drift_correction_is_itself_a_claim`).

**Goal:** lift envoy-go's wholesale boot-REJECT of `layered_runtime` **for the `static_layer` arm only**, land an `internal/runtime.Snapshot` (recursive `Struct` → dotted-path flattening with **TWO** termination branches, distinct-key UNION across layers), register `runtime.num_keys` + `runtime.num_layers`, and pin the semantics cross-side with one new differential fixture — while landing, **atomically**, a nine-arm reject roster that keeps the three sibling oneof arms closed.

**Architecture:** the reject is a raw-YAML generic-map pre-check (`bootstrap.go:568-570`) that fires on mere key presence, **before** `protojson.Unmarshal`. Lifting it exposes **four** oneof arms at once, so the lift and the roster are ONE change. `internal/runtime` takes `[]map[string]*structpb.Value` (one map per declared layer, precedence-ordered) and returns a `*Snapshot`; it never sees a bootstrap proto and imports `structpb` and nothing else. The oneof walk, the roster and the two `NewGauge` calls live in `internal/bootstrap`.

**Tech Stack:** Go 1.26.5 · `internal/bootstrap` · `internal/runtime` (the phase-00 placeholder directory) · `internal/stats` · `google.golang.org/protobuf/types/known/structpb` · the differential harness against `envoyproxy/envoy:contrib-v1.37.2`.

**STAGE:** PLAN (lifecycle-state **2 → 3**). **ROW 77 STAYS `in-progress`.** `ROADMAP.md` **BYTE-UNTOUCHED**; `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; `DECISIONS.md` **BYTE-UNTOUCHED** (ADR-0299 is PROPOSED and completes at the IMPL). File set: this `PLAN.md` + `PROGRESS.md` + `STATE.md` + `next-prompt.txt`.

**ADR-0045 split gate APPLIED and NOT TRIPPED.** The gate is *"> ~25 tasks OR > ~1500 LoC"*. This plan derives **12** tasks and ~700 production+test LoC. A single flat row stands; the escape valve at the *(snapshot + roster)* / *(stats + fixture)* seam stays **UNARMED**.

---

## 1. PLAN re-derivation ledger — what this stage RE-DERIVED, REFUTED, and newly EXECUTED

Every figure below was produced at this PLAN's own tip (`b2dba137`) by execution — one LIVE reference probe fleet (**43 fresh containers**, image id verified per boot), one Go-execution agent in a throwaway worktree, one read-only gate/anchor agent, each with **private scratch**, plus controller re-derivation. **None is carried from the SPEC.**

**Three of this controller's own predictions were REFUTED.** They are stated first, because a prediction a probe merely restates is worthless.

### 1.1 ⚠️ THE HEADLINE — THE SPEC'S OWN DISCRIMINATION INSTRUCTION IS UNACHIEVABLE AS STATED, AND THE FIXTURE CANNOT CARRY THE BREAK ROSTER

`STATE.md:17` and SPEC §8.3 both require: *"the four arms live in ONE bootstrap and all feed ONE gauge, so the PLAN must choose per-arm key counts such that **no two arms' breakage produces the same `num_keys` total**."*

**EXECUTED against a prototype flattener over the exact four-arm config. It cannot be done, and the reason is structural rather than arithmetic:**

```
[T2-ARM] BASELINE (spec, UNION)               total=6
[T2-ARM] A: SUM instead of UNION              total=7
[T2-ARM] B: no recursion (depth-1)            total=4  keys=[emp frac nest ov.key]
[T2-ARM] C: no numerator/denominator branch   total=8
[T2-ARM] D: no empty-struct branch            total=4  keys=[frac nest.mid.leaf1 nest.mid.leaf2 ov.key]
[T2-ARM] DISTINCTNESS: 3 distinct totals out of 4
[T2-ARM] *** COLLISION at total=4: [B: no recursion, D: no empty-struct branch]
```

**The controller's own tuned design — deltas `+1 / −1 / +2 / −2` — was REFUTED by this run.** It assumed each break perturbs exactly one arm. It does not: **a broken recursion collapses the nested arm AND the empty-struct arm simultaneously**, because both arms are reached by the same recursive descent. No choice of per-arm key counts separates them, because the two breaks are not independent.

**Worse, and found independently by the reference probe: two counterfactuals do not move the total AT ALL.** A case-INSENSITIVE termination test and a PAIR-matching test both leave `num_keys` at 6 on the shipped config and change only the key SET.

⇒ **`num_keys` alone is an insufficient break detector, and no re-tuning of the config fixes it.** The resolution is architectural, and it is the single most important decision in this plan:

- **The key SET is asserted at the UNIT level** (`internal/runtime/snapshot_test.go`, T1/T2), where it is fully available, deterministic, and free. **All six discriminating breaks live there.**
- **The FIXTURE asserts `num_keys` + `num_layers` cross-side only** (`6` / `2`), which is what it is uniquely able to prove — that envoy-go and the reference agree — and it carries **transposition** and **registration-absence** breaks, which the unit test cannot see.

**The break roster is therefore SPLIT BY LAYER, not concentrated in the fixture.** SPEC §8.3's four arms all stay in the fixture config — they earn their place as the cross-side agreement pins — but §8.3's instruction that a break must be identifiable *from the gauge* is withdrawn as impossible.

### 1.2 ⚠️ THE SECOND HEADLINE — SENTINEL CHECK (1) IS FAIL-**UNSAFE**, AND THIS IS THE FIRST STAGE TO PROVE THE DANGEROUS DIRECTION

Every document from phase 75 forward records check (1)'s blind spot as *"currently HARMLESS (all four missed rows are `done`)"*. **The blind spot was re-derived independently at this tip and CONFIRMED — 109 data rows / 105 matched / four misses at `:31`, `:35`, `:83`, `:84`** (em-dash "after" cell · dot in a slug · two letter-suffixed ids). That much the lineage had right.

**What no prior stage did was fire the unsafe arm.** EXECUTED on scratch copies:

| arm | check-(1) output |
|---|---|
| flip row 76 (ordinary shape) → `in-progress` | `NOT DONE: row 76` + `row 77` — RED ✅ |
| flip row `00` → `in-progress` | `NOT DONE: row 77` only — **flipped row INVISIBLE** |
| flip row `04` → `in-progress` | `NOT DONE: row 77` only — **INVISIBLE** |
| flip `28.1a` + `28.1b` → `in-progress` | `NOT DONE: row 77` only — **INVISIBLE** |
| **all four `in-progress` + row 77 `done`** | **(empty)** ⚠️ **`stop` WOULD BE CREATED with FOUR not-done rows on the books** |

⚠️ **Check (3) fails SAFE by construction; check (2)'s blind spot was found and fixed at the phase-77 BRAINSTORM. Check (1)'s is the one nobody re-derived, and it fails UNSAFE — the same defect class, in the check the lineage kept calling harmless.** A false "done" ends the project early.

**A fixed, field-parsed form was written and armed on all five arms** (§4, T12). It replaces pattern-matching with `-F'|'` field extraction and adds a **denominator assertion**, so deleting a data row is itself an error:

```sh
awk -F'|' -v want=109 '
  /^\| *[0-9]/ { n++; id=$2; st=$5
    gsub(/^[ \t]+|[ \t]+$/,"",id); gsub(/^[ \t]+|[ \t]+$/,"",st)
    if (st !~ /^done/) print "NOT DONE: row " id }
  END { if (n != want) print "GATE FAIL: examined " n " data rows, expected " want }' docs/envoy-go/ROADMAP.md
```
Arms observed: current tree ⇒ `NOT DONE: row 77` · row-00 arm ⇒ names row 00 · row-04 arm ⇒ names row 04 · 28.1a/b arm ⇒ names both · all-four arm ⇒ names all four · deleted row ⇒ `GATE FAIL: examined 108 data rows, expected 109`.

⚠️ **THE FIX IS INSTALLED AT THIS PLAN, NOT DEFERRED TO THE IMPL.** `next-prompt.txt` is where the sentinel lives, it is already in this stage's file set, and `ROADMAP.md` is untouched by the change (the fix is at the MATCHER). **Deferring a stop-condition fix to the very session where the stop condition first becomes reachable** — the IMPL, where row 77 flips `done` and check (1) goes silent — **is the failure mode this lineage keeps documenting.** **T12 VERIFIES the fixed form is present and re-runs it after the `ROADMAP.md` edit**; it does not install it.

⚠️ **`want=109` is a LIVE figure, not a constant.** A row-adding phase must bump it in the same commit, or every later session gets `GATE FAIL: examined 110 … expected 109`. That noise is deliberate — a silent wrong denominator is how check (1) failed in the first place.

### 1.3 ⚠️ THE REFERENCE-SIDE NUMBERS ARE NOW MEASURED, NOT EXTRAPOLATED

SPEC §1.3 lists, under **NOT EXECUTED**: *"The fixture `0118` does not exist. Its config pair, driver and cross-side numbers are specified in §8 and have **not** been run. The reference-side expected values are extrapolated from the probe arms, not read from a `0118` run."*

**That item is now DISCHARGED for the reference side.** The exact `layered_runtime` block the fixture will ship was booted on the pinned image, **fresh container per arm, three runs per arm**:

| arm | r1 | r2 | r3 | verdict |
|---|---|---|---|---|
| **COMBINED (the shipped config)** | **6 / 2** | **6 / 2** | **6 / 2** | `num_keys=6`, `num_layers=2` |
| A-only (2-layer overlap) | 1 / 2 | 1 / 2 | 1 / 2 | contributes **1** |
| B-only (nested, 2 leaves) | 2 / 1 | 2 / 1 | 2 / 1 | contributes **2** |
| C-only (`{numerator, foo, bar}`) | 1 / 1 | 1 / 1 | 1 / 1 | contributes **1** |
| D-only (`{e1:{}, e2:{}}`) | 2 / 1 | 2 / 1 | 2 / 1 | contributes **2** |
| C-control `{foo,bar,baz}` | 3 / 1 | 3 / 1 | 3 / 1 | **RECURSES** |
| C-capitalized `{Numerator,Denominator}` | 2 / 1 | 2 / 1 | 2 / 1 | **case-SENSITIVE** |
| C-denominator-alone `{denominator,foo}` | 1 / 1 | 1 / 1 | 1 / 1 | **either name alone** |
| D-control `emp2: {}` | 1 / 1 | 1 / 1 | 1 / 1 | empty struct is a **counted LEAF** |
| **P-single** *(positive control)* | 1 / 1 | 1 / 1 | 1 / 1 | gate is **not stuck** |
| **P-baseline** *(no `layered_runtime`)* | 0 / 0 | 0 / 0 | 0 / 0 | floor established |

**The isolation arms sum EXACTLY to the combined total** — `1 + 2 + 1 + 2 = 6` — so no arm's contribution is mis-attributed and there is no cross-arm interaction. The two controls bracket the readout, so a stuck-red *and* a stuck-constant gate would both have been visible. **Zero boot failures in 43 boots.**

**Verbatim prometheus line shape — NO labels, and each sample is preceded by its own `# TYPE` line:**
```
# TYPE envoy_runtime_num_keys gauge
envoy_runtime_num_keys{} 6
# TYPE envoy_runtime_num_layers gauge
envoy_runtime_num_layers{} 2
```
⚠️ A hand-rolled `grep -c` on either name returns **2** per scrape. `scrapeProm` skips `#` lines, so this bites only a shell gate.

⚠️ **`runtime.load_success` and `runtime.override_dir_not_exists` both read `1` in the P-baseline arm too** — i.e. with no `layered_runtime` block at all. **Neither is usable as a "a static layer actually loaded" precondition.** A diff of P-baseline against COMBINED over the whole `envoy_runtime_*` block shows **only** the two gauge lines differ. The fixture's only honest precondition is the **separate absent check** plus `num_layers == 2`.

### 1.4 ⚠️ `Snapshot` IS **NOT** COLLISION-FREE — the SPEC's §5 check missed it, and so did this controller

SPEC §5 reports *"`^type Snapshot` ⇒ **0** declarations"* and concludes the name is clear. That regex is anchored to a **type** declaration. An AST pass over `internal/`, `cmd/` and `validate/` finds:

```
Snapshot   1 decl
   -> internal/dynamicmetadata/dynamicmetadata.go:74 (method)
        func (b *Bucket) Snapshot() map[string]map[string]*structpb.Value
```

**The collision is benign by scope** — a method on `*Bucket` in a different package cannot conflict with a top-level `type Snapshot struct` in `internal/runtime`. **The name is KEPT.** It is recorded because a collision check that reports "clear" when a declaration exists is the failure the memory exists to prevent, and because a reader encountering both will reasonably wonder.

The same pass shows why the AST matters: `Snapshot` occurs as a bare word **46** times but declares **once**; `flatten` occurs **24** times (all prose) and declares **zero** times. **A grep-based roster would have cleared `Snapshot` and flagged `flatten`. Both answers would have been wrong.**

**All other drafted identifiers: 0 declarations.** `NewSnapshot` · `Flatten` · `flatten` · `layerKeys` · `parseLayeredRuntime` · the nine `parseReject*` names · `runtimeNumKeysStat` · `runtimeNumLayersStat`. `TestParseRejectConstants_ByteStable` declares **9** times repo-wide but **0** in `internal/bootstrap`; Go test names are package-scoped, so adopting the established name there is safe and is a feature.

### 1.5 ⚠️ THE ONEOF WRAPPER TYPE NAMES ARE ASYMMETRIC — a switch drafted from memory FAILS TO COMPILE

Found by compile error, not by reading. In the pinned `go-control-plane` `config/bootstrap/v3/bootstrap.pb.go`:

| line | type | role |
|---|---|---|
| 1345 | `RuntimeLayer_StaticLayer` | oneof wrapper — **no trailing `_`** |
| 1353 | `RuntimeLayer_DiskLayer_` | oneof wrapper — **trailing `_`** |
| 1357 | `RuntimeLayer_AdminLayer_` | oneof wrapper — **trailing `_`** |
| 1361 | `RuntimeLayer_RtdsLayer_` | oneof wrapper — **trailing `_`** |
| 2092 / 2168 / 2207 | `RuntimeLayer_DiskLayer` / `_AdminLayer` / `_RtdsLayer` | nested **messages** |

`static_layer` is a `google.protobuf.Struct`, so no nested message squats its name and its wrapper keeps the bare form; the other three are disambiguated. **The un-suffixed names are real types**, so `case *bootstrapv3.RuntimeLayer_DiskLayer:` is a legal-looking line that fails as `impossible type switch case`. It fails loudly, not silently — but it will burn a cycle. T4 carries the correct spellings verbatim.

### 1.6 ⚠️ AN UNSET ONEOF TAKES `default`, AND THE TWO EMPTY SPELLINGS ARE INDISTINGUISHABLE

EXECUTED with the pre-check temporarily lifted (vacuity control `total_nonsense: 1` ⇒ **ERROR**, so the lift did not disable strictness and every result below stands):

```
[e] spec == nil (interface comparison) ? true       %T of spec = <nil>
[e] bare switch            -> DEFAULT arm taken
[e] switch-with-case-nil   -> CASE NIL taken
[g1 layered_runtime: {}]           GetLayeredRuntime()==nil? false ; len(GetLayers())=0
[g2 layered_runtime: {layers: []}] GetLayeredRuntime()==nil? false ; len(GetLayers())=0
[h  absent]                        GetLayeredRuntime()==nil? true
```

⇒ **A `default:` arm returning "unknown layer type" will MISLABEL the unset case.** Arm 4 needs an explicit `case nil:`, and it does fire when present. ⇒ **arms A and C of SPEC §3.2 are indistinguishable after unmarshal**, so arm 9's predicate is exactly `bs.GetLayeredRuntime() != nil && len(...GetLayers()) == 0` and **one test covers both spellings** — CONFIRMED by execution rather than by inference.

Also EXECUTED: `disk_layer` / `admin_layer` / `rtds_layer` each unmarshal **cleanly** (`err = <nil>`); duplicate layer names unmarshal cleanly with **both layers retained**; `k.list` and `k.null` both yield a **non-nil** `*structpb.Value` (kinds `Value_ListValue` / `Value_NullValue`). ⇒ `reference_protojson_null_decodes_to_nil` does not apply; a `v == nil` guard **never fires** for a YAML `null` and the implementation must switch on `v.GetKind()`.

⚠️ **`ov.key` survives as a single literal map key containing a dot** — protojson does not split it, and the flattener must not either. Measured on both sides.

### 1.7 ⚠️ THE `countMetrics` STAT-GUARD IDIOM IS BLIND TO A RENAME

SPEC §10 T7 prescribes *"a `TestNoNewStat*`-class delta guard"*. The five landed guards (`internal/statssink/registration_test.go`) count via `countMetrics(reg)` over `(*stats.Registry).Walk`. **There is no `Names()` API** — `Walk` is the only introspection seam (re-confirmed; `grep -rn '\.Names()'` ⇒ zero hits).

**EXECUTED: a control registering `runtime.keys` / `runtime.layers` — both names WRONG, count right — PASSES the count-only idiom.**

```
--- PASS   ok  EXIT=0        <-- 2 in, 2 out, both names wrong
```

A **name-set** gate was written and armed on five red arms (`+1`, `+3`, one renamed, both renamed, zero registered) plus the shipped green arm; it also went red on a real typo (`runtime.numLayers`). **T5 ships the name-set form, not the count form.** The gate returns violations rather than calling `t.Errorf`, which is what lets it negative-control itself in-suite.

### 1.8 ⚠️ THE `go doc` FAIL-OPEN IS NARROWER AND WORSE THAN RECORDED — and a `+0 exported symbols` gate goes RED on a CORRECT tree

The lineage records *"`go doc -all <A> <B>` fails open with a `./` prefix"*. **Partly REFUTED, and sharpened:**

| spelling | result |
|---|---|
| `go doc -all ./internal/runtime ./internal/bootstrap` | **only `runtime`'s docs**, EXIT **0** ⚠️ |
| `go doc -all ./internal/runtime ./nonexistent-dir-xyz` | only `runtime`'s docs, EXIT **0** ⚠️ |
| `go doc -all ./nonexistent-dir-xyz` *(alone)* | `cannot find package "."`, EXIT **1** — **REFUTED**, does NOT fail open |
| `go doc -all internal/runtime internal/bootstrap` *(bare)* | `no symbol internal/bootstrap in package …`, EXIT **1** |

Byte-length proof that arg2 is discarded: `runtime alone = 285` · `runtime+bootstrap = 285` · `bootstrap alone = 19185`.

⇒ **the defect is not "`./` fails open"; it is that with a VALID arg1, ANY arg2 is silently swallowed.** A two-package gate written this way reports on one package and calls it clean. **One package per invocation, always.**

⚠️ **AND A NEW, ELEVENTH BROKEN-GATE SHAPE, found in this stage's own first draft.** `internal/runtime` currently has **zero** exported symbols. A naive `printf '%s\n' "$cur"` against a 0-byte baseline emits a stray newline and **the gate goes RED on an untouched tree**. A `+0 exported symbols` gate over an empty package must be built and green-armed **before** it is trusted. The shipped form writes the lister's stdout straight to a temp file. Baselines at this tip: `internal/runtime` **0** exported symbols · `internal/bootstrap` **11**.

### 1.9 ⚠️ `BEHAVIOR_CONTRACT.md:1857`'s TWO STALE `+20` CITES DO NOT EXIST

Router item 16 and SPEC §13 item 8 both schedule a fix for *"`BEHAVIOR_CONTRACT.md:1857`'s two stale `+20` cites, plus two more in the same file"*. **REFUTED by re-derivation.** Line `:1857` is the QUIC-parity paragraph and carries no `+20`. Repo-wide the file has exactly **two** `+20` occurrences, at `:5697` and `:5730`, and **both are phase enumerations** (`phases 18+19+20+21`) — i.e. `+20` meaning *"plus phase 20"*, not a delta anchor. **There is no `+20` stat/count anchor in the file at all.**

⇒ **that deferral is STRUCK, not carried forward.** §5 records the strike so the next stage does not re-inherit it.

### 1.10 Other corrections this PLAN owes forward

- **`BEHAVIOR_CONTRACT.md`'s ledger has TWO live discontinuities and the second is undocumented ANYWHERE.** The `1198 → 1200` step is explained at `:732` (two tracer-scoped counters) but **not in ledger form**, so a ledger-form sweep cannot see it. The `1200 → 1201` step has **no accounting line anywhere in the file** (`grep -n '1201'` ⇒ only `:5002`). ⇒ **assert the DELTA `+2`; the absolute `1205 → 1207` rides an unaudited gap and this plan does not pretend otherwise.**
- **`grep -n 'Phase 76'` over `BEHAVIOR_CONTRACT.md` returns ZERO hits** — stronger than the SPEC's *"no Phase 76 ledger entry"*. The Phase 77 line follows **Phase 75 at `:5004`** directly.
- **`:847`'s `115-dir` figure is stale by four** (actual **119**, going to **120**). It is the file's only such hit.
- **ADR-0278 (`DECISIONS.md:16672`, a `**§Context.**` BODY line — heading `:16670`) cites `bootstrap.go:499` for the pre-check; `ROADMAP.md:139` cites `:568-569`. The actual site is `:568-570`.** ADR-0278's cite is **stale**. Recorded; T10 disposes of it.
- **All three `BEHAVIOR_CONTRACT.md` "rejects still stand" sites CONFIRMED at `:906`, `:926`, `:968`** — but ⚠️ `:906` reads *"still stand"* while `:926` and `:968` read *"**STILL** stand"*. **A `grep 'still stand'` finds ONE of the three.** Case-fold or the sweep misses two.
- **`FuzzBootstrapLoad`'s missing invariant, EXECUTED.** The two-line upgrade was applied and run: seed corpus **PASSES**; `-fuzztime 30s` ⇒ **2,357,987 execs, 369 corpus entries, ZERO violations**, no crasher written. ⚠️ **A green fuzz run can also mean "did not run"**, so a negative control was fired: renaming one message's prefix to `NEGCTRL:` turned two seeds RED immediately. **The assertion is live and the 2.36M-exec PASS is meaningful.**
- **Anchors CONFIRMED at this tip:** ADR-0299 heading `DECISIONS.md:17464` (§Context **1** / §Decision **0** / §Consequences **0** / in-block `^---$` **0** / footer at EOF `:17494`) · last `^---$` `:17020` · ADR-0268 heading `:16418`, roster BODY line `:16430` · ADR-0016 `:387` · ADR-0045 `:1466` · ADR-0089 `:3514` with deferral rows `:3543` / `:3550` · ADR-0106 `:4788` · ADR-0187 `:11280` · ADR-0195 `:12413` · ADR-0288 `:16983` · `ROADMAP.md` row 77 at `:139`, Runtime candidates `:213`, Operational-tooling short form `:221` · `BEHAVIOR_CONTRACT.md` **5746** lines · `DECISIONS.md` EOF **17494**.
- **Counts re-derived MECHANICALLY at this tip:** fixtures **119** · fuzzers **55** · internal packages **73** · blank imports in `runner_test.go` **119** (equal to the fixture count) · inline `fmt.Errorf("bootstrap: …")` arms **47**, all in `bootstrap.go` · byte-gate roster **977** after the set-difference · reference port `10118` **free** (zero hits in `test/`, in any `.go`, in any `.yaml`).
- ⚠️ **The Bash cwd reset fired AGAIN at this stage — the TENTH consecutive session** (`Shell cwd was reset to /home/esa/git/envoy-go` immediately after a `cd` into scratch). Every git command in this session used `git -C <abs-worktree-path>`.

---

## 2. Global constraints

Every task's requirements implicitly include this section.

- **Envelope: +2 stats (1205 → 1207) · +1 fixture (`0118`, 119 → 120) · +0 packages (`internal/runtime` EXISTS) · +0 go.mod modules · +0 fuzzers (a seed is not a `func Fuzz`) · +0 BackendKinds (tail 38 / 39 declared) · +0 new PUBLIC surface.**
- **Assert the stat DELTA, never the total** (§1.10 — the ledger has an unaudited `1200 → 1201` step), and assert the **NAME SET**, never the count (§1.7).
- **`internal/bootstrap/bootstrap.go:565-567` (`dynamic_resources`) STAYS BYTE-UNTOUCHED.** Only the `layered_runtime` arm at `:568-570` moves.
- **`ADR-0089` stays BYTE-UNTOUCHED** — it defers `/runtime` and `POST /runtime_modify` to this family and row 77 lands **neither**.
- **`internal/runtime` imports `structpb` and NOTHING else.** It takes `[]map[string]*structpb.Value` and returns a `*Snapshot`; it never sees a bootstrap proto. `go list -deps ./internal/runtime` returns **one line** today; after T1 it must return `internal/runtime` + `structpb`'s closure and **no envoy-go package**.
- **The nine reject messages all carry the `"bootstrap: "` prefix**, land as unexported `const` strings, and are consumed with `errors.New` (no verbs) or `fmt.Errorf` (verbs) — the landed `wasm` precedent.
- **The lexical rule is LEXICAL.** Termination is `numerator` OR `denominator`, lowercase, exact, case-sensitive, **either alone**, at any depth, **values never inspected**. ⚠️ **An implementation that parses or validates a `FractionalPercent` here REJECTS configs the reference ACCEPTS.**
- **TWO termination branches**, not one: the lexical one and the **empty struct** (a counted LEAF, not zero keys).
- **Break protocol:** `-count=1` on every run (`reference_differential_break_protocol_count1`); run breaks **AFTER committing** (`reference_break_protocol_commit_first`); revert with `git restore`, **never** `git checkout <sha>` (detaches HEAD); **confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`).
- **Worktree discipline:** `git -C <abs-worktree-path>` for EVERY git command. The Bash cwd silently resets to the repo root — **it fired again during this PLAN, the tenth consecutive session**. Tripwire `pwd` + `git rev-parse --abbrev-ref HEAD` (must be the stage branch, **NEVER `master`**) + `git rev-list --count master..HEAD` before any commit or gate run.
- **Per-task hygiene:** `[ "$(gofmt -l <pkg> | wc -l)" -eq 0 ]` (gate on OUTPUT — `gofmt -l` NEVER exits non-zero, re-proved at this PLAN), `go vet ./<pkg>/...`, `golangci-lint run ./<pkg>/...`.
- **Identifier roster (AST-checked at this PLAN — 0 declarations each except as noted):** `NewSnapshot` · `Flatten` · `flatten` · `layerKeys` · `parseLayeredRuntime` · `parseRejectDiskLayer` · `parseRejectAdminLayer` · `parseRejectRtdsLayer` · `parseRejectLayerSpecifierUnset` · `parseRejectLayerNameEmpty` · `parseRejectDuplicateLayerName` · `parseRejectStaticLayerValueList` · `parseRejectStaticLayerValueNull` · `parseRejectLayeredRuntimeNoLayers` · `runtimeNumKeysStat` · `runtimeNumLayersStat` · `TestParseRejectConstants_ByteStable` (0 in `internal/bootstrap`). ⚠️ **`Snapshot` declares ONCE** (`internal/dynamicmetadata/dynamicmetadata.go:74`, a method) — **benign by scope, KEPT, recorded** (§1.4).
- **`validate/validate_test.go:6` imports stdlib `"runtime"`.** EXECUTED: same-file unaliased ⇒ `runtime redeclared in this block`; **separate file in the same package ⇒ no collision**; aliased ⇒ no collision. **T4's `validate` test change adds no import**, so this does not bite — but any later test needing `internal/runtime` there must use a separate file or an alias. Neither `internal/bootstrap/bootstrap.go` nor `cmd/envoy-go/main.go` imports stdlib `runtime`.

---

## 3. File structure

```
internal/runtime/snapshot.go                          [NEW]  T1,T2  Snapshot, flatten, NewSnapshot
internal/runtime/snapshot_test.go                     [NEW]  T1,T2  the KEY-SET assertions + all six unit breaks
internal/runtime/doc.go                               [EDIT] T1     replace the 241-byte phase-00 placeholder
internal/bootstrap/bootstrap.go                       [EDIT] T3,T4,T5
                                                             T3 the nine reject constants
                                                             T4 :568-570 REPLACED; :547-549 doc comment; parseLayeredRuntime after :584
                                                             T4 a `Runtime *runtime.Snapshot` field on the Bootstrap struct
                                                             T5 the two NewGauge calls
internal/bootstrap/bootstrap_test.go                  [EDIT] T3,T4,T5
                                                             T3 TestParseRejectConstants_ByteStable + roster-size guard
                                                             T4 TestLoad_RejectsLayeredRuntime REPLACED (:82-96)
                                                             T5 the name-set stat-delta guard
internal/bootstrap/fuzz_test.go                       [EDIT] T6     :66-75 seed + :78-81 the prefix invariant
validate/validate_test.go                             [EDIT] T4     TestBootstrap_ReusesLoad_RejectsLayeredRuntime REPLACED (:65-79)
test/fixtures/0118-runtime-static-layer/**            [NEW]  T7     envoy.yaml, envoy-go.yaml, README.md,
                                                                    expectations.yaml, driver/driver.go
test/differential/runner_test.go                      [EDIT] T7     the blank import, a NEW line after :144
docs/envoy-go/BEHAVIOR_CONTRACT.md                    [EDIT] T9     :906 :926 :968 amendments + a Phase 77 ledger line
                                                                    after :5004 + :831/:847 1205 -> 1207 (+ the 115-dir figure)
docs/envoy-go/DECISIONS.md                            [EDIT] T10,T12 T10 ADR-0268 :16430 roster + ADR-0278 :16672 stale cite
                                                                     T12 ADR-0299 PROPOSED -> COMPLETE
docs/envoy-go/ROADMAP.md                              [EDIT] T12    row 77 (:139) -> done
docs/envoy-go/STATE.md, next-prompt.txt               [EDIT] T12    stage close + the fixed sentinel check (1)

BYTE-UNTOUCHED (sha256 roster verified at T11 — 977 files):
  every tracked *.go MINUS the 13-file edit roster
  ⚠️ internal/bootstrap/** (10 files), internal/runtime/doc.go, validate/*.go are
     REMOVED from the roster by set-difference; universe 990 − edit 13 = 977.
  ⚠️ The roster needs a MISSING leg as well as a MISMATCH leg — a DELETION reads
     as "no mismatch". Proved at this PLAN: the naive `[ -f ] || continue` idiom
     exits 0 on a deleted file. All four legs armed in §4.
```

---
## Task 1 — `internal/runtime`: the flattener, with BOTH termination branches

**Files:** Create `internal/runtime/snapshot.go`, `internal/runtime/snapshot_test.go`; Modify `internal/runtime/doc.go` (replace the 5-line phase-00 placeholder)

**Interfaces:**
- Consumes: nothing from earlier tasks. `google.golang.org/protobuf/types/known/structpb` only.
- Produces: `func flatten(prefix string, s *structpb.Struct, emit func(string))` — unexported, used by T2's `NewSnapshot`.

⚠️ **THE RULE IS LEXICAL, NOT SEMANTIC.** Termination is a test on the literal field NAME. Do not parse a `FractionalPercent`, do not validate the value, do not check the field count. `{numerator: "notanumber", denominator: NOTANENUM}` boots cleanly on the reference; an implementation that validates here **rejects configs the reference accepts**.

- [ ] **Step 1: Write the failing test**

Create `internal/runtime/snapshot_test.go`:

```go
package runtime

import (
	"sort"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

// mustStruct builds a *structpb.Struct from a Go map, failing the test on error.
func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	return s
}

// keysOf runs flatten over s and returns the emitted keys, sorted.
func keysOf(t *testing.T, s *structpb.Struct) []string {
	t.Helper()
	var got []string
	flatten("", s, func(k string) { got = append(got, k) })
	sort.Strings(got)
	return got
}

func TestFlatten_TerminationAndRecursion(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want []string
	}{
		// The four fixture arms, per-layer. MEASURED on the reference at the
		// phase-77 PLAN (3 fresh boots each); see PLAN §1.3.
		{"ArmA_FlatKeyWithLiteralDot", map[string]any{"ov.key": "from_L1"}, []string{"ov.key"}},
		{"ArmB_NestedTwoLeaves", map[string]any{
			"nest": map[string]any{"mid": map[string]any{"leaf1": 1, "leaf2": 2}},
		}, []string{"nest.mid.leaf1", "nest.mid.leaf2"}},
		{"ArmC_NumeratorTerminates", map[string]any{
			"frac": map[string]any{"numerator": 25, "foo": 2, "bar": 3},
		}, []string{"frac"}},
		{"ArmD_EmptyStructsAreLeaves", map[string]any{
			"emp": map[string]any{"e1": map[string]any{}, "e2": map[string]any{}},
		}, []string{"emp.e1", "emp.e2"}},

		// Discriminators. Each kills a plausible-but-wrong rule.
		{"NoTerminatorRecurses", map[string]any{
			"frac2": map[string]any{"foo": 2, "bar": 3, "baz": 4},
		}, []string{"frac2.bar", "frac2.baz", "frac2.foo"}},
		{"CaseSensitive_CapitalizedRecurses", map[string]any{
			"frac3": map[string]any{"Numerator": 25, "Denominator": "HUNDRED"},
		}, []string{"frac3.Denominator", "frac3.Numerator"}},
		{"DenominatorAloneTerminates", map[string]any{
			"frac4": map[string]any{"denominator": "HUNDRED", "foo": 1},
		}, []string{"frac4"}},
		{"TopLevelEmptyStructIsALeaf", map[string]any{
			"emp2": map[string]any{},
		}, []string{"emp2"}},
		// Values are NEVER inspected: this must TERMINATE, not error.
		{"InvalidValuesStillTerminate", map[string]any{
			"frac5": map[string]any{"numerator": "notanumber", "denominator": "NOTANENUM"},
		}, []string{"frac5"}},
		// One-field structs prove field count is irrelevant.
		{"OneFieldNonTerminatorRecurses", map[string]any{
			"k": map[string]any{"foo": 1},
		}, []string{"k.foo"}},
		{"OneFieldNumeratorTerminates", map[string]any{
			"k": map[string]any{"numerator": 25},
		}, []string{"k"}},
		// Unbounded depth; scalars and nests coexist.
		{"UnboundedDepth", map[string]any{
			"deep": map[string]any{"l2": map[string]any{"l3": map[string]any{"l4": 5}}},
		}, []string{"deep.l2.l3.l4"}},
		{"ScalarAndNestCoexist", map[string]any{
			"m": map[string]any{"n": 1}, "m2": 7,
		}, []string{"m.n", "m2"}},
		// A terminated struct nested under a recursing one.
		{"OuterRecursesInnerTerminates", map[string]any{
			"outer": map[string]any{"inner": map[string]any{"numerator": 1, "denominator": "HUNDRED"}},
		}, []string{"outer.inner"}},
	}
	if len(cases) != 14 {
		t.Fatalf("flatten roster: expected 14 rows (4 fixture arms + 10 discriminators); got %d", len(cases))
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := keysOf(t, mustStruct(t, tc.in))
			if len(got) != len(tc.want) {
				t.Errorf("%s: got %d keys %v, want %d keys %v", tc.name, len(got), got, len(tc.want), tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%s: key[%d] = %q, want %q (full: got %v want %v)", tc.name, i, got[i], tc.want[i], got, tc.want)
				}
			}
		})
	}
}
```

⚠️ **Every case asserts the KEY SET, not a count.** §1.1 measured that counts cannot separate a broken recursion from a broken empty-struct branch. The `len` check runs first only to make the per-index diff readable; the per-index loop is the assertion.

⚠️ **`t.Errorf` per property, `return` after the length mismatch** — a `t.Fatalf` here would make every later subtest unreachable (`reference_fatalf_makes_assertions_unreachable`). The roster-size guard IS a `t.Fatalf` because a short roster invalidates the whole test.

- [ ] **Step 2: Run it to verify it fails**

```sh
go test ./internal/runtime/ -run TestFlatten -count=1
```
Expected: **FAIL to COMPILE** — `undefined: flatten`. A compile failure is the correct red here; there is no `flatten` yet.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/runtime/snapshot.go`:

```go
// Package runtime materializes the bootstrap's layered_runtime static layers
// into a flat, precedence-collapsed key space.
package runtime

import "google.golang.org/protobuf/types/known/structpb"

// Termination field names. The reference's flattener stops descending at a
// Struct carrying EITHER of these, matched LEXICALLY and CASE-SENSITIVELY.
// The VALUES are never inspected: {numerator: "notanumber"} terminates and
// boots cleanly on the reference, so parsing a FractionalPercent here would
// REJECT configs the reference ACCEPTS. Measured over 15 arms, 3x each, at
// the phase-77 SPEC (§3.3.2) and re-measured at the phase-77 PLAN (§1.3).
const (
	terminatorNumerator   = "numerator"
	terminatorDenominator = "denominator"
)

// flatten walks s and calls emit once per leaf key, joining path segments with
// '.'. prefix is the accumulated path ("" at the root, so root fields emit
// bare). There are exactly TWO termination branches:
//
//  1. LEXICAL — the Struct carries a field literally named "numerator" or
//     "denominator". Either alone suffices; additional fields are irrelevant;
//     field count is irrelevant ({foo: 1} recurses while {numerator: 25}
//     terminates).
//  2. EMPTY — the Struct has zero fields. An empty Struct is a COUNTED LEAF,
//     not zero keys: `e: {f: {}}` yields the single key `e.f`. This branch is
//     recorded by no document before the phase-77 SPEC (§3.3.3), and the
//     inherited three-arm pin set could not have detected its absence.
//
// A field name containing a literal '.' is NOT re-split: `ov.key` emits
// `ov.key` verbatim. Measured cross-side.
func flatten(prefix string, s *structpb.Struct, emit func(string)) {
	fields := s.GetFields()
	if _, ok := fields[terminatorNumerator]; ok {
		emit(prefix)
		return
	}
	if _, ok := fields[terminatorDenominator]; ok {
		emit(prefix)
		return
	}
	if len(fields) == 0 {
		emit(prefix)
		return
	}
	for name, v := range fields {
		child := name
		if prefix != "" {
			child = prefix + "." + name
		}
		if sv, ok := v.GetKind().(*structpb.Value_StructValue); ok {
			flatten(child, sv.StructValue, emit)
			continue
		}
		emit(child)
	}
}
```

⚠️ **The root call passes `prefix == ""`.** A root Struct that is itself empty, or that itself carries `numerator`, would emit the empty string — a degenerate config no reference arm produces. T2's `NewSnapshot` drops empty keys and Step 5 asserts it.

⚠️ **Switch on `GetKind()`, never on `v == nil`.** A YAML `null` yields a **non-nil** `*structpb.Value` whose kind is `*structpb.Value_NullValue` (EXECUTED, §1.6). A nil guard never fires. `Value_NullValue` and `Value_ListValue` fall to `emit` here; T3/T4 reject them **before** the snapshot is built.

- [ ] **Step 4: Run it to verify it passes**

```sh
go test ./internal/runtime/ -run TestFlatten -count=1 -v
```
Expected: **PASS**, 14 subtests.

- [ ] **Step 5: Add the degenerate-root guard test, red-then-green**

Append to `snapshot_test.go`:

```go
func TestFlatten_EmptyRootEmitsEmptyKey(t *testing.T) {
	// A degenerate case no reference arm produces, pinned so NewSnapshot's
	// drop-empty-keys behaviour (Task 2) has something to stand on.
	got := keysOf(t, mustStruct(t, map[string]any{}))
	if len(got) != 1 || got[0] != "" {
		t.Errorf("empty root: got %v, want exactly one empty-string key", got)
	}
}
```
Run it (PASS), then confirm it is not vacuous by temporarily returning early from the `len(fields) == 0` branch without emitting — the test must go RED — and restore.

- [ ] **Step 6: Replace the `doc.go` placeholder**

`internal/runtime/doc.go` currently reads, in full:
```go
// Package runtime is a phase-00 placeholder. The real implementation
// lands in the runtime family (phases 09+). See docs/envoy-go/ROADMAP.md
// for the family expansion once phases under that heading enter
// in-progress.
package runtime
```
The package clause now lives on `snapshot.go`. **Delete `doc.go` entirely** rather than leaving a second package clause with a stale comment — `go list ./internal/...` stays **73** either way (the directory is unchanged), and a placeholder that says *"the real implementation lands in phases 09+"* sitting beside the real implementation is worse than no file.

- [ ] **Step 7: Hygiene + commit**

```sh
[ "$(gofmt -l internal/runtime | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./internal/runtime/...
golangci-lint run ./internal/runtime/...
go list -deps ./internal/runtime | grep -c 'pgdad/envoy-go' ; echo "EXPECT 1 (itself only)"
git -C <worktree> add internal/runtime/
git -C <worktree> commit -m "phase 77 T1: internal/runtime flatten with BOTH termination branches"
```
⚠️ **The `go list -deps` line is the structural cycle guard** (`reference_xds_config_seam_transitive_cycle_guard`): `internal/runtime` must depend on **no** envoy-go package. `grep -c 'pgdad/envoy-go'` must be exactly **1** — itself.

---

## Task 2 — `Snapshot`: UNION across layers, precedence-collapsed

**Files:** Modify `internal/runtime/snapshot.go`, `internal/runtime/snapshot_test.go`

**Interfaces:**
- Consumes: `flatten` (T1).
- Produces: `type Snapshot struct{...}` with `func NewSnapshot(layers []map[string]*structpb.Value) *Snapshot`, `func (s *Snapshot) NumKeys() int`, `func (s *Snapshot) NumLayers() int`, `func (s *Snapshot) Keys() []string`. **T4 calls `NewSnapshot`; T5 calls `NumKeys`/`NumLayers`.**

⚠️ **`Snapshot` declares once elsewhere** — `internal/dynamicmetadata/dynamicmetadata.go:74`, a method on `*Bucket`. **Benign by scope; the name is kept** (§1.4).

- [ ] **Step 1: Write the failing test**

Append to `internal/runtime/snapshot_test.go`:

```go
// combinedLayers is the EXACT two-layer shape fixture 0118 ships. MEASURED on
// envoyproxy/envoy:contrib-v1.37.2 at the phase-77 PLAN over 3 fresh boots:
// runtime.num_keys = 6, runtime.num_layers = 2, and the isolation arms sum
// exactly (1 + 2 + 1 + 2 = 6). See PLAN §1.3.
func combinedLayers(t *testing.T) []map[string]*structpb.Value {
	t.Helper()
	l1 := mustStruct(t, map[string]any{
		"ov.key": "from_L1",
		"nest":   map[string]any{"mid": map[string]any{"leaf1": 1, "leaf2": 2}},
		"frac":   map[string]any{"numerator": 25, "foo": 2, "bar": 3},
		"emp":    map[string]any{"e1": map[string]any{}, "e2": map[string]any{}},
	})
	l2 := mustStruct(t, map[string]any{"ov.key": "from_L2"})
	return []map[string]*structpb.Value{l1.GetFields(), l2.GetFields()}
}

func TestSnapshot_UnionAcrossLayers(t *testing.T) {
	s := NewSnapshot(combinedLayers(t))

	if got, want := s.NumLayers(), 2; got != want {
		t.Errorf("NumLayers() = %d, want %d", got, want)
	}
	if got, want := s.NumKeys(), 6; got != want {
		t.Errorf("NumKeys() = %d, want %d (reference-measured)", got, want)
	}
	want := []string{"emp.e1", "emp.e2", "frac", "nest.mid.leaf1", "nest.mid.leaf2", "ov.key"}
	got := s.Keys()
	if len(got) != len(want) {
		t.Errorf("Keys() = %v (%d), want %v (%d)", got, len(got), want, len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSnapshot_OverlapCountsOnce(t *testing.T) {
	// The UNION-vs-per-layer-SUM discriminator. Reference-measured A-only arm:
	// num_keys = 1, num_layers = 2.
	// ⚠️ THE OVERLAP IS LOAD-BEARING. If a future edit removes `ov.key` from
	// L2, SUM == UNION and this test goes VACUOUS while still passing.
	l1 := mustStruct(t, map[string]any{"ov.key": "from_L1"})
	l2 := mustStruct(t, map[string]any{"ov.key": "from_L2"})
	s := NewSnapshot([]map[string]*structpb.Value{l1.GetFields(), l2.GetFields()})
	if got := s.NumKeys(); got != 1 {
		t.Errorf("overlap NumKeys() = %d, want 1 (UNION, not per-layer SUM)", got)
	}
	if got := s.NumLayers(); got != 2 {
		t.Errorf("overlap NumLayers() = %d, want 2", got)
	}
	// A per-layer SUM implementation gives 2 here and 7 on combinedLayers.
	// Both numbers are asserted, so neither can drift silently.
}

func TestSnapshot_Degenerate(t *testing.T) {
	if got := NewSnapshot(nil).NumKeys(); got != 0 {
		t.Errorf("nil layers: NumKeys() = %d, want 0", got)
	}
	if got := NewSnapshot(nil).NumLayers(); got != 0 {
		t.Errorf("nil layers: NumLayers() = %d, want 0", got)
	}
	// An empty layer contributes a layer but no keys, and MUST NOT contribute
	// the empty-string key that flatten emits for an empty root.
	empty := mustStruct(t, map[string]any{})
	s := NewSnapshot([]map[string]*structpb.Value{empty.GetFields()})
	if got := s.NumLayers(); got != 1 {
		t.Errorf("empty layer: NumLayers() = %d, want 1", got)
	}
	if got := s.NumKeys(); got != 0 {
		t.Errorf("empty layer: NumKeys() = %d, want 0 (the empty root key is dropped)", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```sh
go test ./internal/runtime/ -run TestSnapshot -count=1
```
Expected: **FAIL to COMPILE** — `undefined: NewSnapshot`.

- [ ] **Step 3: Write the minimal implementation**

Append to `internal/runtime/snapshot.go`:

```go
import "sort"   // add to the existing import block

// Snapshot is the precedence-collapsed key space of a bootstrap's declared
// layered_runtime static layers. It is built once, at Load time, and is
// immutable thereafter.
//
// ⚠️ envoy-go's snapshot is BOOT-FIXED where the reference's is LIVE: the
// reference's runtime.num_keys moves when an admin layer is written through
// POST /runtime_modify. envoy-go ships no write path (row 77 lands neither
// /runtime nor /runtime_modify), so the two agree — see PLAN §5.
type Snapshot struct {
	keys      []string // sorted, distinct
	numLayers int
}

// NewSnapshot flattens each layer and unions the resulting key spaces. layers
// is one field-map per DECLARED layer, in precedence order (later layers
// override earlier ones). The override VALUE is not retained: this row serves
// no /runtime endpoint, and the reference's within-layer collision winner is
// NON-DETERMINISTIC across process starts (~40/60 over 18 fresh processes,
// phase-77 SPEC §3.3.1), so a value is not a thing envoy-go can agree with
// cross-side. The distinct-key COUNT and the key SET both are.
func NewSnapshot(layers []map[string]*structpb.Value) *Snapshot {
	seen := make(map[string]struct{})
	for _, fields := range layers {
		flatten("", &structpb.Struct{Fields: fields}, func(k string) {
			if k == "" {
				return // the degenerate empty-root key; never a real runtime key
			}
			seen[k] = struct{}{}
		})
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &Snapshot{keys: keys, numLayers: len(layers)}
}

// NumKeys is the distinct-key count across all layers (the UNION, not the
// per-layer sum). Published as runtime.num_keys.
func (s *Snapshot) NumKeys() int { return len(s.keys) }

// NumLayers is the number of DECLARED layers. Published as runtime.num_layers.
func (s *Snapshot) NumLayers() int { return s.numLayers }

// Keys returns the sorted distinct key set. The slice is freshly allocated per
// call, so a caller cannot mutate the Snapshot.
func (s *Snapshot) Keys() []string {
	out := make([]string, len(s.keys))
	copy(out, s.keys)
	return out
}
```

⚠️ **`sort.Strings` is what makes the key set assertable.** Go map iteration is randomized; without the sort every `Keys()` assertion would flake. This also makes envoy-go **more defined than the reference**, which is acceptable and unassertable cross-side (SPEC §3.3.1).

- [ ] **Step 4: Run it to verify it passes**

```sh
go test ./internal/runtime/ -count=1 -v
```
Expected: **PASS**, all subtests.

- [ ] **Step 5: Prove `Keys()` cannot be mutated through**

```go
func TestSnapshot_KeysIsACopy(t *testing.T) {
	s := NewSnapshot(combinedLayers(t))
	k := s.Keys()
	k[0] = "MUTATED"
	if s.Keys()[0] == "MUTATED" {
		t.Error("Keys() returned an aliased slice; a caller can corrupt the Snapshot")
	}
}
```
Run it (PASS), then temporarily `return s.keys` directly — the test must go RED — and restore.

- [ ] **Step 6: Hygiene + commit**

```sh
[ "$(gofmt -l internal/runtime | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./internal/runtime/... && golangci-lint run ./internal/runtime/...
go test ./internal/runtime/ -count=1
go list -deps ./internal/runtime | grep -c 'pgdad/envoy-go'   # MUST be 1
git -C <worktree> add internal/runtime/
git -C <worktree> commit -m "phase 77 T2: Snapshot UNION across layers, reference-measured at 6 keys / 2 layers"
```

---

## Task 3 — the NINE reject constants + `TestParseRejectConstants_ByteStable`

**Files:** Modify `internal/bootstrap/bootstrap.go` (a new `const` block), `internal/bootstrap/bootstrap_test.go` (the roster test)

**Interfaces:**
- Consumes: nothing.
- Produces: nine unexported `const` strings, consumed by T4's `parseLayeredRuntime`.

⚠️ **This task changes NO behavior.** It lands constants and their pin. The tree stays green throughout; the constants are unused until T4. `golangci-lint` may flag unused constants — if it does, land T3 and T4 as one commit rather than suppressing the linter, and say so in `PROGRESS.md`.

⚠️ **`internal/bootstrap` has ZERO named reject constants and ZERO `ByteStable` tests today** — all **47** arms are inline `fmt.Errorf`, all in `bootstrap.go` (re-derived at this PLAN). This row **introduces** the discipline **scoped to its own nine arms only**; it does **not** convert the other 46.

⚠️ **Copy the `wasm` variant, not `admission_control`.** `wasm/compiled_config_test.go:159-161` guards `len(cases)` so silently deleting a row FAILS; `admission_control` has no such guard — re-confirmed at this PLAN (`grep -n 'len(cases)'` over that file returns nothing, and repo-wide only **wasm** among the nine `TestParseRejectConstants_ByteStable` packages has it).

- [ ] **Step 1: Write the failing test**

Append to `internal/bootstrap/bootstrap_test.go` (package `bootstrap`, so the constants are reachable directly):

```go
// -----------------------------------------------------------------------------
// TestParseRejectConstants_ByteStable pins the byte-exact wording for each of
// the NINE phase-77 layered_runtime PARSE-REJECT arms (SPEC §6).
// Any drift requires a lockstep SPEC §6 + ADR-0299 edit per the ADR-0044
// atomic-edit discipline.
//
// ⚠️ THREE OF THE NINE ARE DELIBERATE DEPARTURES, not parity: the reference
// ACCEPTS disk_layer, admin_layer and rtds_layer (measured, phase-77 SPEC
// §1.1). They are rejected because silently ignoring rtds_layer means a config
// asking for DYNAMIC runtime quietly gets STATIC values.
//
// ⚠️ NO CROSS-SIDE WORDING ASSERTION IS POSSIBLE OR ATTEMPTED. The reference's
// PGV messages carry a proto DebugString whose redaction marker ROTATES across
// process starts (8 distinct strings measured in 13 fresh processes at the
// phase-77 PLAN), and its unknown-field message varies in whitespace AND in its
// near-L:C offsets across runs of the SAME file. envoy-go pins its OWN wording
// internally and never compares wording cross-side.
// -----------------------------------------------------------------------------
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		// Arms 1-3: the sibling oneof arms. DEPARTURES — the reference accepts these.
		{"Arm01_DiskLayer", parseRejectDiskLayer,
			"bootstrap: layered_runtime.layers.disk_layer is not supported; use layered_runtime.layers.static_layer"},
		{"Arm02_AdminLayer", parseRejectAdminLayer,
			"bootstrap: layered_runtime.layers.admin_layer is not supported; use layered_runtime.layers.static_layer"},
		{"Arm03_RtdsLayer", parseRejectRtdsLayer,
			"bootstrap: layered_runtime.layers.rtds_layer is not supported; use layered_runtime.layers.static_layer"},
		// Arms 4-6: parity. The reference rejects these too (4-5 via PGV, 6 via a
		// hand-written loader check).
		{"Arm04_LayerSpecifierUnset", parseRejectLayerSpecifierUnset,
			"bootstrap: layered_runtime.layers.layer_specifier is required"},
		{"Arm05_LayerNameEmpty", parseRejectLayerNameEmpty,
			"bootstrap: layered_runtime.layers.name is required"},
		{"Arm06_DuplicateLayerName", parseRejectDuplicateLayerName,
			"bootstrap: layered_runtime.layers.name %q is duplicated"},
		// Arms 7-8: parity. The reference's loader rejects both with
		// "Invalid runtime entry value for <key>".
		{"Arm07_StaticLayerValueList", parseRejectStaticLayerValueList,
			"bootstrap: layered_runtime.layers.static_layer: value for key %q is a list; runtime values must be scalar or a nested map"},
		{"Arm08_StaticLayerValueNull", parseRejectStaticLayerValueNull,
			"bootstrap: layered_runtime.layers.static_layer: value for key %q is null; runtime values must be scalar or a nested map"},
		// Arm 9: DEPARTURE. The reference ACCEPTS this and synthesizes a
		// writable admin layer; envoy-go ships no write path, so a gauge
		// counting an unreachable layer would be a false stat.
		{"Arm09_NoLayers", parseRejectLayeredRuntimeNoLayers,
			"bootstrap: layered_runtime.layers is empty; zero declared layers requests an implicit admin layer, which is not supported; use layered_runtime.layers with a static_layer"},
	}

	// Roster size: 9. The TENTH candidate arm (more than one admin_layer) is
	// DROPPED AS UNREACHABLE — envoy-go rejects admin_layer outright at arm 2,
	// so a second one can never be reached and the row would be untestable.
	// ⚠️ This guard is the wasm variant (compiled_config_test.go:159-161);
	// admission_control has none, so deleting a row there is silent.
	if len(cases) != 9 {
		t.Fatalf("TestParseRejectConstants_ByteStable: expected 9 rows (10 candidate arms − 1 DROPPED as unreachable); got %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestParseRejectConstants_AllCarryPrefix is the invariant Load's doc comment
// states and that only ONE of the four pre-existing reject tests checked.
func TestParseRejectConstants_AllCarryPrefix(t *testing.T) {
	all := []string{
		parseRejectDiskLayer, parseRejectAdminLayer, parseRejectRtdsLayer,
		parseRejectLayerSpecifierUnset, parseRejectLayerNameEmpty,
		parseRejectDuplicateLayerName, parseRejectStaticLayerValueList,
		parseRejectStaticLayerValueNull, parseRejectLayeredRuntimeNoLayers,
	}
	if len(all) != 9 {
		t.Fatalf("prefix roster: expected 9 constants, got %d", len(all))
	}
	for _, s := range all {
		if !strings.HasPrefix(s, "bootstrap: ") {
			t.Errorf("reject constant lacks the %q prefix: %q", "bootstrap: ", s)
		}
	}
}
```

`strings` is already imported by `bootstrap_test.go:5`; no import change.

- [ ] **Step 2: Run it to verify it fails**

```sh
go test ./internal/bootstrap/ -run TestParseRejectConstants -count=1
```
Expected: **FAIL to COMPILE** — nine `undefined:` errors.

- [ ] **Step 3: Write the minimal implementation**

Insert into `internal/bootstrap/bootstrap.go`, immediately **above** `// Load parses r as YAML` (i.e. before `:545`):

```go
// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings for the phase-77 layered_runtime arms
// (SPEC §6 + ADR-0299). Pinned byte-exact at bootstrap_test.go::
// TestParseRejectConstants_ByteStable, whose roster-size guard makes a silent
// deletion fail. Every prefix is "bootstrap: " per the Load doc contract.
//
// ⚠️ SCOPE: this block is the FIRST named-reject-constant discipline in
// internal/bootstrap. The package's other 47 inline fmt.Errorf("bootstrap: …")
// arms are NOT converted by this row.
//
// ⚠️ Arms 1, 2, 3 and 9 are envoy-go DEPARTURES: the reference ACCEPTS those
// configs (measured against contrib-v1.37.2, phase-77 SPEC §1.1 / §3.2).
// -----------------------------------------------------------------------------
const (
	// Arm 1: layers[].disk_layer set. DEPARTURE — the reference accepts it and
	// loads keys from the directory.
	parseRejectDiskLayer = "bootstrap: layered_runtime.layers.disk_layer is not supported; use layered_runtime.layers.static_layer"

	// Arm 2: layers[].admin_layer set. DEPARTURE — the reference accepts it.
	// envoy-go ships no POST /runtime_modify, so the layer could never gain a key.
	parseRejectAdminLayer = "bootstrap: layered_runtime.layers.admin_layer is not supported; use layered_runtime.layers.static_layer"

	// Arm 3: layers[].rtds_layer set. DEPARTURE, and the most important of the
	// three: silently ignoring it means a config asking for DYNAMIC runtime
	// quietly gets STATIC values — a wrong answer rather than a loud failure.
	parseRejectRtdsLayer = "bootstrap: layered_runtime.layers.rtds_layer is not supported; use layered_runtime.layers.static_layer"

	// Arm 4: layers[].layer_specifier unset. Parity (the reference rejects via
	// PGV). ⚠️ GetLayerSpecifier() returns a nil INTERFACE, so a bare type
	// switch takes `default`; parseLayeredRuntime uses an explicit `case nil`.
	parseRejectLayerSpecifierUnset = "bootstrap: layered_runtime.layers.layer_specifier is required"

	// Arm 5: layers[].name empty. Parity (PGV: value length must be >= 1).
	parseRejectLayerNameEmpty = "bootstrap: layered_runtime.layers.name is required"

	// Arm 6: two layers with the same name. Parity — but the reference's check
	// is a HAND-WRITTEN loader check, not PGV ("Duplicate layer name: L1").
	// protojson accepts duplicates and retains both layers (EXECUTED).
	parseRejectDuplicateLayerName = "bootstrap: layered_runtime.layers.name %q is duplicated"

	// Arm 7: a static_layer value that is a LIST. Parity — the reference's
	// loader rejects with "Invalid runtime entry value for <key>".
	parseRejectStaticLayerValueList = "bootstrap: layered_runtime.layers.static_layer: value for key %q is a list; runtime values must be scalar or a nested map"

	// Arm 8: a static_layer value that is NULL (null / ~ / bare-empty — one
	// case). Parity. ⚠️ The *structpb.Value is NON-NIL with kind
	// *structpb.Value_NullValue; a `v == nil` guard never fires.
	parseRejectStaticLayerValueNull = "bootstrap: layered_runtime.layers.static_layer: value for key %q is null; runtime values must be scalar or a nested map"

	// Arm 9: layered_runtime present with ZERO declared layers. DEPARTURE.
	// The reference synthesizes a genuinely writable admin layer (num_keys
	// 0 -> 1 -> 3 across two runtime_modify writes, measured). envoy-go
	// rejects the EXPLICIT admin_layer at arm 2; accepting the IMPLICIT form
	// would be incoherent, and reporting num_layers: 1 for a layer that can
	// never gain a key would be a false stat.
	// ⚠️ `layered_runtime: {}` and `layered_runtime: {layers: []}` are
	// INDISTINGUISHABLE after unmarshal (EXECUTED), so one predicate covers both.
	parseRejectLayeredRuntimeNoLayers = "bootstrap: layered_runtime.layers is empty; zero declared layers requests an implicit admin layer, which is not supported; use layered_runtime.layers with a static_layer"
)
```

- [ ] **Step 4: Run it to verify it passes**

```sh
go test ./internal/bootstrap/ -run TestParseRejectConstants -count=1 -v
```
Expected: **PASS**, 9 subtests + the prefix test.

- [ ] **Step 5: Prove the roster-size guard FIRES** *(this is the whole reason to copy the wasm variant)*

Delete one row from the `cases` slice, re-run:
```sh
go test ./internal/bootstrap/ -run TestParseRejectConstants_ByteStable -count=1
```
Expected: **FAIL** — `expected 9 rows (10 candidate arms − 1 DROPPED as unreachable); got 8`. Restore with `git restore`.

⚠️ **Confirm WHICH assertion fired.** The failure must be the `len(cases)` `Fatalf`, not a wording mismatch. If it is a wording mismatch you deleted the wrong thing.

- [ ] **Step 6: Prove the prefix test fires**

Temporarily strip `bootstrap: ` from one constant; `TestParseRejectConstants_AllCarryPrefix` must go RED **and** `TestParseRejectConstants_ByteStable` must ALSO go red (the `want` string still carries it). Both firing is correct — they are independent properties over the same bytes. Restore.

- [ ] **Step 7: Hygiene + commit**

```sh
[ "$(gofmt -l internal/bootstrap | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./internal/bootstrap/... && golangci-lint run ./internal/bootstrap/...
go test ./internal/bootstrap/ -count=1
git -C <worktree> add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git -C <worktree> commit -m "phase 77 T3: nine layered_runtime reject constants + the wasm-style roster-size guard"
```

---
## Task 4 — the LIFT + `parseLayeredRuntime` + BOTH test-site replacements — **ONE COMMIT, ATOMICALLY**

**Files:** Modify `internal/bootstrap/bootstrap.go` (`:547-549` doc comment, `:568-570` replaced, the `Bootstrap` struct, a new `parseLayeredRuntime` after `:584`), `internal/bootstrap/bootstrap_test.go` (`:82-96` replaced + the nine arm tests), `validate/validate_test.go` (`:65-79` replaced)

**Interfaces:**
- Consumes: `runtime.NewSnapshot` (T2), the nine constants (T3).
- Produces: `Bootstrap.Runtime *runtime.Snapshot`, read by T5's gauge registration.

### ⚠️ WHY THIS IS ONE TASK AND MUST NOT BE SPLIT

`reference_lifted_reject_hidden_enforcement`. **The pre-check is a SINGLE guard standing in front of FOUR oneof arms.** The instant `:568-570` is removed, `disk_layer`, `admin_layer` and `rtds_layer` go from *unreachable* to *silently accepted* — EXECUTED at this PLAN: all three unmarshal with `err = <nil>`. A task boundary with a green gate between the lift and the roster would ship, however briefly, a binary that accepts a config asking for **dynamic** runtime and serves it **static** values.

**And the two test replacements cannot be deferred either.** `internal/bootstrap/bootstrap_test.go:82-96` and `validate/validate_test.go:65-79` **both** use the fixture `name: static_layer` + `static_layer: {}` — exactly the arm this row legalizes — so **both flip to `err == nil` and die at `t.Fatal`** the moment the lift lands. A task that leaves either package red is not a task.

⚠️ **`static_layer: {}` is an EMPTY struct, and arm 9 is about zero LAYERS, not zero keys.** The replacement fixture declares **one** layer whose `static_layer` is empty ⇒ `num_layers = 1`, `num_keys = 1` (the empty struct is a counted leaf, key `static_layer`… no — the empty *root* key is dropped by `NewSnapshot`, so `num_keys = 0`). **Step 1 asserts this explicitly** because it is exactly the kind of off-by-one a reader will assume wrong.

- [ ] **Step 1: Write the failing tests — all nine arms plus the positive path**

Replace `internal/bootstrap/bootstrap_test.go:82-96` (the whole of `TestLoad_RejectsLayeredRuntime`) with:

```go
// TestLoad_AcceptsStaticLayer is the phase-77 lift. Before this row Load
// rejected any bootstrap containing the key layered_runtime; it now accepts
// the static_layer arm and builds a Snapshot.
//
// ⚠️ This REPLACES TestLoad_RejectsLayeredRuntime, whose fixture (name:
// static_layer + static_layer: {}) is exactly the arm being legalized.
func TestLoad_AcceptsStaticLayer(t *testing.T) {
	yaml := sampleBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: want nil error for a static_layer bootstrap, got %v", err)
	}
	if bs.Runtime == nil {
		t.Fatal("Load: Runtime snapshot is nil for a static_layer bootstrap")
	}
	if got := bs.Runtime.NumLayers(); got != 1 {
		t.Errorf("NumLayers() = %d, want 1", got)
	}
	// An EMPTY static_layer declares a layer with no keys. flatten emits the
	// degenerate empty-root key; NewSnapshot drops it. So 0, not 1.
	if got := bs.Runtime.NumKeys(); got != 0 {
		t.Errorf("NumKeys() = %d, want 0 (an empty static_layer has no keys)", got)
	}
}

func TestLoad_AcceptsStaticLayer_Populated(t *testing.T) {
	// The four-arm shape fixture 0118 ships. Reference-measured 6 / 2.
	yaml := sampleBootstrap + `
layered_runtime:
  layers:
    - name: L1
      static_layer:
        ov.key: "from_L1"
        nest: {mid: {leaf1: 1, leaf2: 2}}
        frac: {numerator: 25, foo: 2, bar: 3}
        emp: {e1: {}, e2: {}}
    - name: L2
      static_layer:
        ov.key: "from_L2"
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := bs.Runtime.NumLayers(); got != 2 {
		t.Errorf("NumLayers() = %d, want 2", got)
	}
	if got := bs.Runtime.NumKeys(); got != 6 {
		t.Errorf("NumKeys() = %d, want 6 (reference-measured over 3 fresh boots)", got)
	}
}

// TestLoad_LayeredRuntimeRejectArms covers all NINE arms of the roster. Each
// arm asserts the "bootstrap: " prefix AND a naming substring — the asymmetry
// R9 found (only one of the four pre-existing guards checked the prefix) is
// closed here for every new arm.
//
// ⚠️ Substring matching only, never == on the whole message: envoy-go's own
// protojson error carries a `line L:C` derived from the MARSHALED JSON, whose
// keys json.Marshal SORTS, so that column shifts whenever any other key in the
// document changes (measured: 1:32 / 1:21 / 1:74 / 1:2 for the same unknown key).
func TestLoad_LayeredRuntimeRejectArms(t *testing.T) {
	cases := []struct {
		name     string
		tail     string
		contains string
	}{
		{"Arm01_DiskLayer", `
layered_runtime:
  layers:
    - name: L1
      disk_layer: {symlink_root: /srv/runtime, subdirectory: current}
`, "disk_layer"},
		{"Arm02_AdminLayer", `
layered_runtime:
  layers:
    - name: L1
      admin_layer: {}
`, "admin_layer"},
		{"Arm03_RtdsLayer", `
layered_runtime:
  layers:
    - name: L1
      rtds_layer: {name: rtds, rtds_config: {resource_api_version: V3}}
`, "rtds_layer"},
		{"Arm04_LayerSpecifierUnset", `
layered_runtime:
  layers:
    - name: L1
`, "layer_specifier"},
		{"Arm05_LayerNameEmpty", `
layered_runtime:
  layers:
    - name: ""
      static_layer: {k: 1}
`, "name"},
		{"Arm06_DuplicateLayerName", `
layered_runtime:
  layers:
    - name: L1
      static_layer: {a: 1}
    - name: L1
      static_layer: {b: 2}
`, "duplicated"},
		{"Arm07_ValueIsList", `
layered_runtime:
  layers:
    - name: L1
      static_layer: {k.list: [1, 2, 3]}
`, "is a list"},
		{"Arm08_ValueIsNull", `
layered_runtime:
  layers:
    - name: L1
      static_layer: {k.null: null}
`, "is null"},
		// Arm 9 has TWO spellings that are indistinguishable after unmarshal.
		// Both must reject; both are listed so a predicate that only covers one
		// is caught.
		{"Arm09_NoLayersField", `
layered_runtime: {}
`, "is empty"},
		{"Arm09b_EmptyLayersList", `
layered_runtime:
  layers: []
`, "is empty"},
	}
	if len(cases) != 10 {
		t.Fatalf("reject-arm roster: expected 10 rows (9 arms, arm 9 spelled twice); got %d", len(cases))
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(sampleBootstrap + tc.tail))
			if err == nil {
				t.Fatalf("%s: Load returned nil error; want a reject", tc.name)
			}
			if !strings.HasPrefix(err.Error(), "bootstrap: ") {
				t.Errorf("%s: error prefix: got %q, want to start with %q", tc.name, err.Error(), "bootstrap: ")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("%s: error should contain %q: %q", tc.name, tc.contains, err.Error())
			}
		})
	}
}
```

And replace `validate/validate_test.go:65-79` (`TestBootstrap_ReusesLoad_RejectsLayeredRuntime`) with:

```go
func TestBootstrap_ReusesLoad_AcceptsStaticLayer(t *testing.T) {
	// ⚠️ REPLACES TestBootstrap_ReusesLoad_RejectsLayeredRuntime. Its fixture
	// (name: static_layer + static_layer: {}) is exactly the arm phase 77
	// legalizes, so the old test would have died at t.Fatal.
	yaml := sampleValidBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	if err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap: got %v, want nil for a static_layer bootstrap", err)
	}
}

func TestBootstrap_ReusesLoad_RejectsRtdsLayer(t *testing.T) {
	// The public-package sibling of the roster. rtds_layer is chosen because it
	// is the arm whose silent acceptance would be WORST — a config asking for
	// DYNAMIC runtime served STATIC values.
	// ⚠️ This asserts the "bootstrap: " prefix, which NEITHER pre-existing
	// validate/ reject test did (R9).
	yaml := sampleValidBootstrap + `
layered_runtime:
  layers:
    - name: L1
      rtds_layer: {name: rtds, rtds_config: {resource_api_version: V3}}
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for rtds_layer, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("error prefix: got %q, want to start with %q", err.Error(), "bootstrap: ")
	}
	if !strings.Contains(err.Error(), "rtds_layer") {
		t.Errorf("error should name rtds_layer: %q", err.Error())
	}
}
```

⚠️ **No import change in `validate/validate_test.go`.** It already imports `strings` (`:7`). It also imports stdlib `"runtime"` (`:6`) — **do not add `internal/runtime` to this file**; nothing here needs it, and same-file unaliased would be a hard compile error (EXECUTED, §1.6).

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/bootstrap/ -run 'TestLoad_AcceptsStaticLayer|TestLoad_LayeredRuntimeRejectArms' -count=1
go test ./validate/ -run 'TestBootstrap_ReusesLoad' -count=1
```
Expected: **FAIL to COMPILE** in `internal/bootstrap` (`bs.Runtime` undefined), and in `validate` the accept test FAILS with the old reject message. Both reds are the correct starting point.

- [ ] **Step 3: Write the minimal implementation — the lift, the field, the parser**

**(a) The struct field.** Add to the `Bootstrap` struct, after `ConfigPath string` (`:542`):
```go
	// Runtime is the flattened, precedence-collapsed static-layer key space
	// parsed from layered_runtime (phase 77, ADR-0299). Non-nil for EVERY
	// successfully-loaded bootstrap, including one with no layered_runtime
	// block at all — in that case it is a zero Snapshot (0 keys, 0 layers), so
	// the two gauges register and publish 0 unconditionally, matching the
	// reference, which emits both names at 0 in the absent arm (measured).
	Runtime *runtime.Snapshot
```

**(b) The import.** Add to the envoy-go group at `:181-182`:
```go
	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/runtime"
	"github.com/pgdad/envoy-go/internal/stats"
```
⚠️ **Unaliased is correct here** — `internal/bootstrap/bootstrap.go` does NOT import stdlib `runtime` (EXECUTED, §1.6).

**(c) The doc comment.** Replace `:547-549`'s clause. Current:
```go
// cause an error (ADR-0016). The phase-01 unsupported surfaces
// dynamic_resources and layered_runtime cause an error even though the proto
// itself defines them. The returned *Bootstrap also carries a freshly
```
becomes:
```go
// cause an error (ADR-0016). The phase-01 unsupported surface
// dynamic_resources causes an error even though the proto itself defines it;
// layered_runtime is accepted for its static_layer arm only (phase 77,
// ADR-0299) and rejected arm-by-arm otherwise. The returned *Bootstrap also
// carries a freshly
```

**(d) The lift.** Replace `:568-570` — the three lines
```go
	if _, ok := generic["layered_runtime"]; ok {
		return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
	}
```
— with **nothing**. `:565-567` (`dynamic_resources`) stays **byte-untouched**.

**(e) The parse hook.** Insert after `:584-586`'s `parseStatsSinks` block, before `return result, nil`:
```go
	if err := parseLayeredRuntime(bs, result); err != nil {
		return nil, err
	}
```
⚠️ **UNCONDITIONAL, not gated on presence.** The reference emits both gauge names at **0** in the absent arm (measured), and the fixture asserts the name set unconditionally.

**(f) The parser.** Add after `parseStatsSinks`:
```go
// parseLayeredRuntime walks bootstrap.layered_runtime, rejects every arm the
// nine-constant roster covers, and builds result.Runtime from the accepted
// static layers. It is called UNCONDITIONALLY: with no layered_runtime block
// it stores a zero Snapshot so the two gauges publish 0, matching the
// reference's absent arm.
//
// ⚠️ THE ROSTER AND THE LIFT ARE ONE CHANGE (reference_lifted_reject_hidden_
// enforcement). The wholesale pre-check this replaced stood in front of FOUR
// oneof arms; three of them unmarshal cleanly and would be SILENTLY ACCEPTED
// without the switch below.
func parseLayeredRuntime(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
	lr := bs.GetLayeredRuntime()
	if lr == nil {
		result.Runtime = runtime.NewSnapshot(nil)
		return nil
	}
	layers := lr.GetLayers()
	if len(layers) == 0 {
		// Arm 9. Covers BOTH `layered_runtime: {}` and `layers: []` — they are
		// indistinguishable after unmarshal (EXECUTED).
		return errors.New(parseRejectLayeredRuntimeNoLayers)
	}

	seenNames := make(map[string]struct{}, len(layers))
	fieldMaps := make([]map[string]*structpb.Value, 0, len(layers))
	for _, l := range layers {
		name := l.GetName()
		if name == "" {
			return errors.New(parseRejectLayerNameEmpty) // Arm 5
		}
		if _, dup := seenNames[name]; dup {
			return fmt.Errorf(parseRejectDuplicateLayerName, name) // Arm 6
		}
		seenNames[name] = struct{}{}

		// ⚠️ An UNSET oneof yields a nil INTERFACE, so a bare switch takes
		// `default` and would mislabel it (EXECUTED). `case nil` is explicit.
		// ⚠️ The wrapper type names are ASYMMETRIC: StaticLayer has no trailing
		// underscore; the other three DO. The un-suffixed forms are the nested
		// MESSAGE types, so getting this wrong is an "impossible type switch
		// case" compile error, not a silent miss.
		switch spec := l.GetLayerSpecifier().(type) {
		case nil:
			return errors.New(parseRejectLayerSpecifierUnset) // Arm 4
		case *bootstrapv3.RuntimeLayer_StaticLayer:
			fields := spec.StaticLayer.GetFields()
			if err := checkStaticLayerValues(fields); err != nil {
				return err // Arms 7, 8
			}
			fieldMaps = append(fieldMaps, fields)
		case *bootstrapv3.RuntimeLayer_DiskLayer_:
			return errors.New(parseRejectDiskLayer) // Arm 1
		case *bootstrapv3.RuntimeLayer_AdminLayer_:
			return errors.New(parseRejectAdminLayer) // Arm 2
		case *bootstrapv3.RuntimeLayer_RtdsLayer_:
			return errors.New(parseRejectRtdsLayer) // Arm 3
		default:
			return errors.New(parseRejectLayerSpecifierUnset)
		}
	}
	result.Runtime = runtime.NewSnapshot(fieldMaps)
	return nil
}

// checkStaticLayerValues rejects list and null leaf values at any depth (arms
// 7 and 8). The reference's loader rejects both with "Invalid runtime entry
// value for <key>"; the wording is not comparable cross-side (its debug marker
// rotates per process), so envoy-go pins its own.
//
// ⚠️ Recursion here mirrors flatten's TERMINATION rules exactly: a Struct that
// terminates flattening is a LEAF and its interior is never inspected, because
// the reference performs ZERO validation there — {numerator: "notanumber",
// denominator: NOTANENUM} boots cleanly. Validating inside a terminated struct
// would REJECT configs the reference ACCEPTS.
func checkStaticLayerValues(fields map[string]*structpb.Value) error {
	if _, ok := fields["numerator"]; ok {
		return nil
	}
	if _, ok := fields["denominator"]; ok {
		return nil
	}
	for name, v := range fields {
		switch kind := v.GetKind().(type) {
		case *structpb.Value_ListValue:
			return fmt.Errorf(parseRejectStaticLayerValueList, name)
		case *structpb.Value_NullValue:
			return fmt.Errorf(parseRejectStaticLayerValueNull, name)
		case *structpb.Value_StructValue:
			if err := checkStaticLayerValues(kind.StructValue.GetFields()); err != nil {
				return err
			}
		}
	}
	return nil
}
```
Add `"errors"` and `structpb "google.golang.org/protobuf/types/known/structpb"` to the import block.

⚠️ **The `default:` arm is unreachable today** (the oneof has exactly four wrappers plus nil) but is kept: a future go-control-plane bump that adds a fifth arm must fail CLOSED, not fall through to acceptance.

- [ ] **Step 4: Run to verify they pass**

```sh
go test ./internal/bootstrap/ -count=1
go test ./validate/ -count=1
```
Expected: **PASS** in both. ⚠️ **Run the whole package, not `-run`** — `reference_change_set_measure_not_build_measure`: a file-scoped run misses a sibling `_test.go` this change breaks.

- [ ] **Step 5: Prove each of the ten reject rows fires ITS OWN arm**

For each row in `TestLoad_LayeredRuntimeRejectArms`, temporarily neutralize **only that arm** in `parseLayeredRuntime` (e.g. change `return errors.New(parseRejectDiskLayer)` to `continue`) and confirm:
1. that row goes **RED**, and
2. **the other nine stay GREEN**.

```sh
go test ./internal/bootstrap/ -run TestLoad_LayeredRuntimeRejectArms -count=1 -v
```
Record which subtest failed for each neutralization. ⚠️ **`reference_ordered_assertion_legs_vacuous_on_constant_change`: a row that trips an EARLIER arm passes while testing nothing.** Arm 5 (empty name) and arm 4 (unset specifier) are the pair most at risk — arm 5's fixture has a specifier and arm 4's has a name, deliberately, so neither can short-circuit the other. Confirm that by neutralizing arm 5 and checking arm 4 stays green.

- [ ] **Step 6: Prove the LIFT itself is live**

Restore `:568-570`'s reject temporarily. `TestLoad_AcceptsStaticLayer` must go **RED**. Remove it again; green. This is the one break that proves the row does what it says.

- [ ] **Step 7: Hygiene + commit — ONE COMMIT**

```sh
[ "$(gofmt -l internal/bootstrap validate | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./internal/bootstrap/... ./validate/...
golangci-lint run ./internal/bootstrap/... ./validate/...
go test ./internal/bootstrap/ ./validate/ ./internal/runtime/ -count=1
git -C <worktree> diff --stat go.mod go.sum   # MUST be empty
git -C <worktree> add internal/bootstrap/ validate/
git -C <worktree> commit -m "phase 77 T4: lift the layered_runtime reject for static_layer ONLY, with the nine-arm sibling roster and both test sites, atomically"
```
⚠️ **`git diff go.mod` re-check** — `internal/runtime` is a NEW import edge even though it is not a new package (`reference_new_subpackage_pulls_transitive_module`). `structpb` is already a production dependency, so the expected diff is **empty**.

---

## Task 5 — the two gauges + a NAME-SET stat-delta guard

**Files:** Modify `internal/bootstrap/bootstrap.go` (two `NewGauge` calls in `parseLayeredRuntime`), `internal/bootstrap/bootstrap_test.go` (the guard)

**Interfaces:**
- Consumes: `Bootstrap.Runtime` (T4), `(*stats.Registry).NewGauge`, `(*stats.Gauge).Set`.
- Produces: the stat names `runtime.num_keys`, `runtime.num_layers`.

⚠️ **SHIP THE NAME-SET FORM, NOT THE COUNT FORM.** EXECUTED at this PLAN: the landed `countMetrics(reg)` idiom **PASSES** a control that registers `runtime.keys` / `runtime.layers` — two in, two out, **both names wrong** (§1.7). A count guard cannot see a rename.

- [ ] **Step 1: Write the failing test**

Append to `internal/bootstrap/bootstrap_test.go`:

```go
// registeredStatNames walks r and returns every registered metric name, sorted.
// ⚠️ (*stats.Registry) has NO Names() method — Walk is the only introspection
// seam (re-confirmed at the phase-77 PLAN).
func registeredStatNames(r *stats.Registry) []string {
	var out []string
	r.Walk(func(m stats.Metric) { out = append(out, m.Name()) })
	sort.Strings(out)
	return out
}

// TestStatDelta_LayeredRuntimeRegistersExactlyTwo pins the phase-77 stat
// envelope: EXACTLY two new names, and EXACTLY these two.
//
// ⚠️ ASSERT THE DELTA, NEVER THE TOTAL. BEHAVIOR_CONTRACT's ledger chain has
// TWO discontinuities (1198→1200, documented only in prose; and 1200→1201,
// documented NOWHERE), so the absolute 1205 → 1207 rides an unaudited gap. The
// +2 is what this row can prove.
//
// ⚠️ ASSERT THE NAME SET, NEVER THE COUNT. A count-only guard passes a build
// that registers two stats with both names WRONG (EXECUTED at the phase-77 PLAN).
func TestStatDelta_LayeredRuntimeRegistersExactlyTwo(t *testing.T) {
	base, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load (no layered_runtime): %v", err)
	}
	withLR, err := Load(strings.NewReader(sampleBootstrap + `
layered_runtime:
  layers:
    - name: L1
      static_layer: {a: 1}
`))
	if err != nil {
		t.Fatalf("Load (with layered_runtime): %v", err)
	}

	// The gauges register UNCONDITIONALLY, so both registries carry them and
	// the name sets are IDENTICAL. That is the property: presence does not
	// depend on the config.
	baseNames := registeredStatNames(base.Stats)
	lrNames := registeredStatNames(withLR.Stats)
	if len(baseNames) != len(lrNames) {
		t.Errorf("name-set size differs with/without layered_runtime: %d vs %d", len(baseNames), len(lrNames))
	}

	want := []string{"runtime.num_keys", "runtime.num_layers"}
	for _, n := range want {
		if !containsName(baseNames, n) {
			t.Errorf("%q ABSENT from a bootstrap with NO layered_runtime; the gauges must register unconditionally", n)
		}
		if !containsName(lrNames, n) {
			t.Errorf("%q ABSENT from a bootstrap WITH layered_runtime", n)
		}
	}

	// The DELTA: exactly two names beginning with "runtime.", no more.
	var got []string
	for _, n := range lrNames {
		if strings.HasPrefix(n, "runtime.") {
			got = append(got, n)
		}
	}
	if len(got) != len(want) {
		t.Errorf("runtime.* name set = %v (%d), want %v (%d)", got, len(got), want, len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("runtime.* name[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestStatDelta_GaugeValues(t *testing.T) {
	cases := []struct {
		name              string
		tail              string
		wantKeys, wantLay int64
	}{
		{"absent", ``, 0, 0},
		{"one_layer_one_key", `
layered_runtime:
  layers:
    - name: L1
      static_layer: {a: 1}
`, 1, 1},
		{"the_0118_shape", `
layered_runtime:
  layers:
    - name: L1
      static_layer:
        ov.key: "from_L1"
        nest: {mid: {leaf1: 1, leaf2: 2}}
        frac: {numerator: 25, foo: 2, bar: 3}
        emp: {e1: {}, e2: {}}
    - name: L2
      static_layer:
        ov.key: "from_L2"
`, 6, 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bs, err := Load(strings.NewReader(sampleBootstrap + tc.tail))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			// ⚠️ Look the gauges up by NAME through Walk, so a renamed stat is
			// caught here too rather than reading a Go field that still exists.
			gotKeys, okKeys := gaugeValueByName(bs.Stats, "runtime.num_keys")
			gotLay, okLay := gaugeValueByName(bs.Stats, "runtime.num_layers")
			if !okKeys {
				t.Errorf("runtime.num_keys not registered")
			} else if gotKeys != tc.wantKeys {
				t.Errorf("runtime.num_keys = %d, want %d", gotKeys, tc.wantKeys)
			}
			if !okLay {
				t.Errorf("runtime.num_layers not registered")
			} else if gotLay != tc.wantLay {
				t.Errorf("runtime.num_layers = %d, want %d", gotLay, tc.wantLay)
			}
		})
	}
}
```
Plus two small helpers (`containsName`, `gaugeValueByName` — the latter walks for the name and type-asserts `*stats.Gauge`, returning `(g.Load(), true)`), and `"sort"` added to the test import block alongside the existing `"reflect"`, `"strings"`, `"testing"`, `"time"`, and `"github.com/pgdad/envoy-go/internal/stats"`.

⚠️ **The absent-check is SEPARATE from the value check and the value check is in an `else`.** An unregistered gauge would otherwise read as `0 == 0` and pass vacuously on the `absent` row — the exact vacuity `0110`'s asserter guards against.

- [ ] **Step 2: Run to verify it fails** — `runtime.num_keys not registered` on every row.

- [ ] **Step 3: Write the minimal implementation**

At the end of `parseLayeredRuntime`, after `result.Runtime` is set on **every** return path, register and set:
```go
	// Registered UNCONDITIONALLY and pre-Freeze. Load allocates a FRESH
	// stats.NewRegistry() per call (:580), so no two Load calls share a
	// registry and double-registration is impossible by construction;
	// bs.Stats.Freeze() happens later, in cmd/envoy-go/main.go.
	result.Stats.NewGauge(runtimeNumKeysStat).Set(int64(result.Runtime.NumKeys()))
	result.Stats.NewGauge(runtimeNumLayersStat).Set(int64(result.Runtime.NumLayers()))
```
with, beside the reject constants:
```go
// The two phase-77 runtime gauges. ⚠️ Both are BOOT-FIXED where the reference's
// are LIVE (its num_keys moves under POST /runtime_modify); row 77 ships no
// write path, so the two agree. Both pass stats.NamePattern — EXECUTED with
// negative controls (`runtime.` and `runtime.num-keys` both rejected).
const (
	runtimeNumKeysStat   = "runtime.num_keys"
	runtimeNumLayersStat = "runtime.num_layers"
)
```

⚠️ **Restructure so the registration runs on the accept path only after `result.Runtime` is set, and on the `lr == nil` early return too.** The simplest correct shape is a small `setRuntimeStats(result)` helper called at both success returns; a `defer` would also fire on the reject paths and register gauges on a bootstrap that is about to be discarded — harmless but untidy, and it would make the `TestStatDelta` name-set assertion depend on reject ordering.

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/bootstrap/ -count=1`.

- [ ] **Step 5: Prove the guard goes RED on each of five arms** *(the count-only idiom passes three of these)*

| arm | edit | must fire |
|---|---|---|
| **S1** | register only `num_keys` | name-set size `1 != 2` |
| **S2** | register a third `runtime.num_overrides` | name-set size `3 != 2` |
| **S3** | rename to `runtime.numLayers` | ⚠️ **name mismatch — a COUNT guard PASSES this** |
| **S4** | rename BOTH to `runtime.keys`/`runtime.layers` | ⚠️ **name mismatch — a COUNT guard PASSES this** |
| **S5** | gate registration on `lr != nil` | `ABSENT from a bootstrap with NO layered_runtime` |

Run each, record the exact failure line, `git restore` between arms.

- [ ] **Step 6: Hygiene + commit** — as T4, plus `go test ./internal/... -count=1` to catch any package that walks a registry.

---

## Task 6 — the fuzz seed + the invariant `FuzzBootstrapLoad` never asserted

**Files:** Modify `internal/bootstrap/fuzz_test.go` (`:66-75` seeds, `:78-81` the body)

⚠️ **Fuzz delta is +0.** A corpus seed is not a `func Fuzz` (`reference_fuzzer_count_docs_drift`). The count stays **55**; `internal/bootstrap/testdata` does not exist.

⚠️ **`FuzzBootstrapLoad` does not assert the invariant its own comment states.** `:78-81` is `_, _ = Load(...)` — both returns discarded — under a comment claiming *"either `(*Bootstrap, nil)` or `(nil, err starting with "bootstrap: ")"*. **It is a panic-only guard and must not be cited as a prefix gate.** This two-line change closes that and gives the nine new arms fuzz coverage at once.

- [ ] **Step 1: Add the seed and the assertion**

After `:75`'s `f.Add(nested)`:
```go
	// Phase 77: the layered_runtime static-layer arm, carrying all four
	// flattening shapes so mutation explores the reject roster and both
	// termination branches.
	f.Add([]byte(sampleBootstrap + `
layered_runtime:
  layers:
    - name: L1
      static_layer:
        ov.key: "from_L1"
        nest: {mid: {leaf1: 1, leaf2: 2}}
        frac: {numerator: 25, foo: 2, bar: 3}
        emp: {e1: {}, e2: {}}
    - name: L2
      static_layer:
        ov.key: "from_L2"
`))
```
and replace the body at `:77-81`:
```go
	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic. Any input returns either (*Bootstrap, nil) or
		// (nil, err starting with "bootstrap: ").
		//
		// ⚠️ Before phase 77 this discarded both returns, so the stated
		// invariant was never checked — a panic-only guard under a comment
		// claiming more. The assertion below is what makes the comment true.
		_, err := Load(bytes.NewReader(data))
		if err != nil && !strings.HasPrefix(err.Error(), "bootstrap: ") {
			t.Fatalf("error lacks the %q prefix: %v", "bootstrap: ", err)
		}
	})
```
Add `"strings"` to the file's import block (currently `"bytes"`, `"testing"`).

- [ ] **Step 2: Run the seed corpus, then a short fuzz budget**

```sh
go test ./internal/bootstrap/ -run FuzzBootstrapLoad -count=1
go test ./internal/bootstrap/ -run FuzzBootstrapLoad -fuzz FuzzBootstrapLoad -fuzztime 30s
```
**EXECUTED at this PLAN against the pre-T4 tree:** seeds PASS; 30 s ⇒ **2,357,987 execs, 369 corpus entries, ZERO violations**, no crasher written. Re-run **after** T4 — the nine new arms are new error paths and the invariant is the thing that could break.

⚠️ **If a crasher appears under `testdata/fuzz/`, that is a REAL FINDING and a blocking one** — it means a new reject path returns an error without the prefix. Fix the arm, keep the crasher as a corpus file.

- [ ] **Step 3: Negative-control the assertion** *(a green fuzz run can also mean "did not run" — `reference_liveness_break_needs_failing_baseline`)*

Rename one message's prefix to `NEGCTRL:` and re-run the **seed corpus only**. **EXECUTED at this PLAN:**
```
--- FAIL: FuzzBootstrapLoad/seed#2  error lacks "bootstrap: " prefix: NEGCTRL: empty document
--- FAIL: FuzzBootstrapLoad/seed#3  error lacks "bootstrap: " prefix: NEGCTRL: empty document
```
Restore and confirm `grep -c NEGCTRL` ⇒ **0**.

- [ ] **Step 4: Confirm the fuzzer count did NOT move**

```sh
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l   # MUST be 55
```
Negative control: append a throwaway `func FuzzProbeOnly` ⇒ 56, then remove.

- [ ] **Step 5: Hygiene + commit**

```sh
[ "$(gofmt -l internal/bootstrap | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./internal/bootstrap/... && golangci-lint run ./internal/bootstrap/...
git -C <worktree> add internal/bootstrap/fuzz_test.go
git -C <worktree> commit -m "phase 77 T6: a layered_runtime fuzz seed, and the prefix invariant FuzzBootstrapLoad only claimed"
```

---
## Task 7 — differential fixture `0118-runtime-static-layer`, port `10118`

**Files:** Create `test/fixtures/0118-runtime-static-layer/{envoy.yaml,envoy-go.yaml,README.md,expectations.yaml,driver/driver.go}`; Modify `test/differential/runner_test.go` (the blank import)

**Interfaces:**
- Consumes: `fixture.Driver`, `fixture.StatsAsserter`, `fixture.TB`, `fixture.RegisterFixture`.
- Produces: nothing consumed by later tasks.

### ⚠️ THREE registration gates, all required — the third is the one that fails SILENTLY GREEN

1. The directory `test/fixtures/0118-runtime-static-layer/` — matches `discoverFixtures`'s bare-4-digit branch (`name[4] == '-'`), verified.
2. `fixture.RegisterFixture(fixtureName, …)` from `init()`, with `fixtureName` **exactly equal to the directory name**. A mismatch misses the registry lookup at `runner_test.go:192` and the runner **`t.Skipf`s — silently green**.
3. **A BLANK IMPORT in `runner_test.go`.** Currently **119**, contiguous at `:26-144`. Insert a NEW line **after `:144`** and before `:145` (`"github.com/pgdad/envoy-go/test/helpers"`) — the block is sorted and `test/fixtures/…` sorts before `test/helpers`, so gofmt keeps it there. Without it `init()` never runs.

⚠️ **`go test ./test/differential/ -run 'TestDifferential/0118' -count=1` on a tree where 0118 does not exist prints `[no tests to run]` and EXITS 0** — EXECUTED at this PLAN. **The `-run` selector cannot tell you the fixture ran.** Every invocation in this task uses the guarded form (§4, G5).

⚠️ **STATS ONLY — four independent reasons, the fourth strongest.** `compareAdminResponses` compares the body **byte-exact**, and the reference `/runtime` body cannot survive it: (1) JSON key order randomized **per request** (three consecutive GETs, three md5s, one sort-keys md5); (2) the Struct debug-string marker randomized **per process** — **8 distinct strings measured in 13 fresh processes at this PLAN**; (3) an empty-map value renders as a leaked non-deterministic DebugString *and still counts*; (4) **the within-layer collision winner is non-deterministic**, so any fixture asserting a `final_value` is unrunnable. All four contaminate the **body only**; the gauges are immune.

- [ ] **Step 1: Create the config pair**

`envoy.yaml` — reference side. Admin on 9901 (the harness hard-wires and maps `9901/tcp`), listener on `{{.ListenerPort}}` = 10118, one `STRICT_DNS` cluster at `host.docker.internal`, plus the `layered_runtime` block **verbatim as measured**:
```yaml
layered_runtime:
  layers:
    - name: L1
      static_layer:
        # Arm A — the UNION-vs-per-layer-SUM discriminator. ⚠️ THE OVERLAP IS
        # LOAD-BEARING: L2 re-declares this key. Remove it and SUM == UNION and
        # arm A goes VACUOUS while still passing.
        # ⚠️ The literal '.' is NOT re-split by either side (measured).
        ov.key: "from_L1"
        # Arm B — unbounded-depth leaf flattening. 2 keys.
        nest:
          mid:
            leaf1: 1
            leaf2: 2
        # Arm C — the LEXICAL termination rule. A SINGLE lowercase `numerator`
        # alongside unrelated siblings: a pair-matching implementation gives 3
        # here, the real rule gives 1. Spelling the full {numerator,denominator}
        # pair would pass against BOTH and discriminate nothing.
        # ⚠️ The VALUES are never inspected — do not add validation here.
        frac:
          numerator: 25
          foo: 2
          bar: 3
        # Arm D — the SECOND termination branch. An empty Struct is a COUNTED
        # LEAF: 2 keys, not 0. No prior document recorded this branch, and the
        # inherited three-arm pin set could not have detected its absence.
        emp:
          e1: {}
          e2: {}
    - name: L2
      static_layer:
        ov.key: "from_L2"
```
`envoy-go.yaml` — byte-identical `layered_runtime` block; the only deltas are the project's standing ones: `type: STATIC` instead of `STRICT_DNS`, `127.0.0.1` instead of `host.docker.internal`, no `dns_lookup_family`, and runner-allocated `{{.AdminPort}}` / `{{.ListenerPort}}`.

⚠️ **Small integers and short strings only.** The reference renders in-int32-range integers through a **6-significant-digit `%g`** (`2147483647` → `2.14748e+09`, and `1000000`/`1000001` **both** → `1e+06`), out-of-int32 integers and all floats verbatim, and the YAML and JSON front-ends **disagree** (`4.0` → `"4.0"` vs `"4"`). This fixture asserts no values — but a config using large or float literals would make any future value assertion unreproducible.

- [ ] **Step 2: Create `driver/driver.go`**

Clone the `0117` chassis (5-file shape, `refAdminPort = 9901` const + `{{.AdminPort}}`) and `0110`'s `scrapeProm` + `AssertStats` structure:

```go
// Package driver is the differential fixture driver for
// 0118-runtime-static-layer: the phase-77 layered_runtime static-layer
// consumer, asserted STATS-ONLY on runtime.num_keys / runtime.num_layers.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0118-runtime-static-layer"

	// In-container reference Envoy ports. Convention "10<fixture index>" —
	// 0114→10114, 0115→10115, 0116→10116, 0117→10117, so 0118→10118.
	// ⚠️ NOT 10450: that is the TLS/SDS band (0108-0113), and this is not a
	// TLS fixture. 10118 RE-DERIVED FREE at the phase-77 PLAN: zero hits in
	// test/, in any *.go, in any *.yaml.
	refAdminPort    = 9901
	refListenerPort = 10118

	// The reference-MEASURED expectations (contrib-v1.37.2, 3 fresh boots,
	// with 4 isolation arms summing exactly 1+2+1+2=6). See PLAN §1.3.
	wantNumKeys   = 6
	wantNumLayers = 2
)

type runtimeStaticLayerDriver struct{}

func init() { fixture.RegisterFixture(fixtureName, &runtimeStaticLayerDriver{}) }

// --- fixture.Driver (required) ---

// BackendCount stays 1: this fixture drives NO backend traffic, but the runner
// rejects 0 (runner_test.go:240-243 t.Fatalf) —
// reference_differential_backendcount_min_one. The default TCPEcho kind is the
// minimum viable shape (0110's posture). +0 BackendKinds.
func (*runtimeStaticLayerDriver) BackendCount() int           { return 1 }
func (*runtimeStaticLayerDriver) SubjectListenerName() string { return "l_test" }
func (*runtimeStaticLayerDriver) ReferenceListenerPort() int  { return refListenerPort }

func (d *runtimeStaticLayerDriver) ReferenceBootstrap(backendPorts []int) string {
	return mustRender(mustReadFixtureFile("envoy.yaml"), map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
	})
}

func (d *runtimeStaticLayerDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return mustRender(mustReadFixtureFile("envoy-go.yaml"), map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference / DriveSubject open and immediately close one TCP connection
// so the runner's CompareBytes step has a defined (empty) result on both sides.
// The row's whole observable is the gauge pair; there is no request semantics
// to compare.
func (d *runtimeStaticLayerDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}
func (d *runtimeStaticLayerDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

func (*runtimeStaticLayerDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}
```

and the asserter:

```go
// --- fixture.StatsAsserter ---

// AssertStats is the runner's step-10 stats leg (ADR-0062). It scrapes
// /stats/prometheus on BOTH sides and pins the two phase-77 runtime gauges.
//
// ⚠️ THE DISPATCH IS A SILENT TYPE ASSERTION with no else, no log and no skip
// (runner_test.go:1347-1349, FIRST addr = REFERENCE). A signature typo makes
// ok == false and this whole leg vanishes GREEN while the compiler, go vet and
// golangci-lint all stay quiet. The compile-time assertion below is the
// tripwire; break F5 proves the leg RUNS.
//
// ⚠️ NO PRECONDITION IS AVAILABLE FROM THE OTHER runtime.* NAMES. Measured at
// the phase-77 PLAN: runtime.load_success and runtime.override_dir_not_exists
// both read 1 on the reference even with NO layered_runtime block at all, so
// neither is a "a static layer loaded" guard. The absent-check plus a non-zero
// num_layers is the honest substitute.
//
// ⚠️ envoy-go publishes 2 runtime.* names where the reference publishes 9. The
// project asserts NAMED SUBSETS cross-side, never full-set equality
// (reference_stats_sink_emits_used_only), so this creates no divergence here —
// but a future row asserting full runtime.* name-set equality WILL fail,
// correctly. Recorded as a deferral in PLAN §5.
func (d *runtimeStaticLayerDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	// Scrapes are PRECONDITIONS, not properties -> Fatalf
	// (reference_fatalf_makes_assertions_unreachable).
	ref, err := scrapeProm(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus: %v", err)
	}
	subj, err := scrapeProm(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus: %v", err)
	}

	// fixture.TB has EXACTLY Errorf/Fatalf/Helper — no Logf
	// (reference_fixture_tb_has_no_logf). Diagnostics go through log.Printf.
	// This line is also the evidence that the leg RAN at all.
	log.Printf("0118 AssertStats: ref num_keys=%d num_layers=%d | subj num_keys=%d num_layers=%d",
		ref["envoy_runtime_num_keys"], ref["envoy_runtime_num_layers"],
		subj["envoy_runtime_num_keys"], subj["envoy_runtime_num_layers"])

	want := map[string]uint64{
		"envoy_runtime_num_keys":   wantNumKeys,   // 1(A) + 2(B) + 1(C) + 2(D), UNION
		"envoy_runtime_num_layers": wantNumLayers, // L1 + L2
	}
	for _, n := range []string{"envoy_runtime_num_keys", "envoy_runtime_num_layers"} {
		refVal, refOK := ref[n]
		subjVal, subjOK := subj[n]
		// ⚠️ THE ABSENT CHECK IS SEPARATE FROM THE VALUE CHECK, and it
		// `continue`s. A gauge that fails to REGISTER reads as 0 == 0 through a
		// single-value lookup and would pass VACUOUSLY.
		if !refOK {
			t.Errorf("ref: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if !subjOK {
			t.Errorf("subj: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if refVal != want[n] {
			t.Errorf("ref %s = %d, want %d", n, refVal, want[n])
		}
		if subjVal != want[n] {
			t.Errorf("subj %s = %d, want %d", n, subjVal, want[n])
		}
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", n, refVal, subjVal)
		}
	}

	// ⚠️ THE TRANSPOSITION CHECK, and it is only possible because 6 != 2.
	// A build that wires num_keys to the layer count and num_layers to the key
	// count passes every per-name value check above ONLY if the two wants are
	// equal. They are not, so the checks above already catch it — this is the
	// named diagnosis rather than two cryptic mismatches.
	if ref["envoy_runtime_num_keys"] == want["envoy_runtime_num_layers"] &&
		ref["envoy_runtime_num_layers"] == want["envoy_runtime_num_keys"] {
		t.Errorf("ref: the two gauges look TRANSPOSED (num_keys=%d num_layers=%d)",
			ref["envoy_runtime_num_keys"], ref["envoy_runtime_num_layers"])
	}
	if subj["envoy_runtime_num_keys"] == want["envoy_runtime_num_layers"] &&
		subj["envoy_runtime_num_layers"] == want["envoy_runtime_num_keys"] {
		t.Errorf("subj: the two gauges look TRANSPOSED (num_keys=%d num_layers=%d)",
			subj["envoy_runtime_num_keys"], subj["envoy_runtime_num_layers"])
	}
}

// Compile-time interface assertions. ⚠️ The StatsAsserter one is MANDATORY:
// runner_test.go:1347 dispatches via a SILENT type assertion, so a signature
// typo makes ok == false and the whole assertion NEVER RUNS while every tool
// stays quiet.
var (
	_ fixture.Driver        = (*runtimeStaticLayerDriver)(nil)
	_ fixture.StatsAsserter = (*runtimeStaticLayerDriver)(nil)
)
```
Plus `scrapeProm`, `fixtureDir`, `mustReadFixtureFile`, `mustRender` — cloned verbatim from `0110/driver.go:774-833` and `:570-614`. `scrapeProm` keys on the metric NAME with the label set **stripped entirely** and collides by **summing**; the two gauges carry an **empty label set** (`envoy_runtime_num_keys{} 6`), which that parser handles via its brace branch.

- [ ] **Step 3: Create `README.md` + `expectations.yaml`**

⚠️ **`expectations.yaml` is 100% comment prose read by ZERO Go code** (ADR-0019: *"the driver is the enforcer; this file is documentation"*). Include it for consistency with the other fixtures; **do not encode behavior in it.** Both files must state: the four arms and what each discriminates; that the L1/L2 overlap is **load-bearing**; that the row rejects three sibling arms the reference **ACCEPTS** (departures, not parity); and that envoy-go publishes **2** `runtime.*` names to the reference's **9**.

- [ ] **Step 4: Add the blank import**

Insert after `runner_test.go:144`:
```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0118-runtime-static-layer/driver"
```
Verify: `grep -c '^\s*_ "github.com/pgdad/envoy-go/test/fixtures/' test/differential/runner_test.go` ⇒ **120**, equal to the new fixture count.

- [ ] **Step 5: Run the fixture — with the GUARDED selector**

```sh
out=$(go test ./test/differential/ -run 'TestDifferential/0118-runtime-static-layer' -count=1 -v 2>&1); ec=$?
printf '%s' "$out" | grep -q 'no tests to run' && { echo "GATE FAIL: selector matched NOTHING"; ec=1; }
printf '%s' "$out" | grep -q '0118 AssertStats:' || { echo "GATE FAIL: AssertStats did NOT run"; ec=1; }
echo "EXIT=$ec"
```
⚠️ **Both guards are load-bearing.** The first catches a missing blank import (bare `-run` exits 0 with `[no tests to run]` — EXECUTED). The second catches the silent `StatsAsserter` dispatch — the `log.Printf` line is the only proof the leg executed.

- [ ] **Step 6: Confirm the fixture count and the port**

```sh
ls -d test/fixtures/[0-9]*/ | wc -l                 # 120
ls -d test/fixtures/*/ | grep -cE '[0-9]{4}[a-z]?-' # the faithful discoverFixtures predicate
grep -rn '10118' --include='*.go' . | grep -v 0118-runtime-static-layer | wc -l   # 0
```

- [ ] **Step 7: Hygiene + commit**

```sh
[ "$(gofmt -l test/fixtures/0118-runtime-static-layer test/differential | wc -l)" -eq 0 ] && echo GOFMT_OK
go vet ./test/... && golangci-lint run ./test/fixtures/0118-runtime-static-layer/...
git -C <worktree> add test/fixtures/0118-runtime-static-layer/ test/differential/runner_test.go
git -C <worktree> commit -m "phase 77 T7: fixture 0118-runtime-static-layer on port 10118, four arms, stats-only"
```

---

## Task 8 — the break roster

**Files:** none permanently. Every break is applied, observed, and reverted with `git restore`.

⚠️ **Run breaks AFTER committing** (`reference_break_protocol_commit_first` — `git restore` wipes uncommitted work). ⚠️ **`-count=1` on every run** (`reference_differential_break_protocol_count1`). ⚠️ **Confirm WHICH assertion fired**, never just that something went red (`reference_deliberate_break_wrong_assertion`).

### The roster is SPLIT BY LAYER, and §1.1 is why

`num_keys` **cannot** identify which arm broke: B and D both yield 4, and two further counterfactuals do not move it at all. So the six discriminating breaks live where the key SET is available — the unit test — and the fixture carries the three breaks the unit test cannot see.

| # | layer | edit | MUST fire | status |
|---|---|---|---|---|
| **U1** | T1 `flatten` | delete the `numerator`/`denominator` branch | `ArmC_NumeratorTerminates: key[0] = "frac.bar", want "frac"` — total 6→**8** | **[RUN — prototype]** |
| **U2** | T1 `flatten` | delete the `len(fields)==0` branch | `ArmD_EmptyStructsAreLeaves: got 0 keys` — total 6→**4** | **[RUN — prototype]** |
| **U3** | T1 `flatten` | emit at depth 1 (no recursion) | `ArmB_NestedTwoLeaves: got 1 key [nest]` — total 6→**4** ⚠️ **SAME TOTAL AS U2; the key set is what separates them** | **[RUN — prototype]** |
| **U4** | T2 `NewSnapshot` | per-layer SUM instead of UNION | `TestSnapshot_OverlapCountsOnce: NumKeys() = 2, want 1`; combined 6→**7** | **[RUN — prototype]** |
| **U5** | T1 `flatten` | case-INSENSITIVE name match | `CaseSensitive_CapitalizedRecurses: got 1 key [frac3]` ⚠️ **combined total STAYS 6** | **[IMPL]** |
| **U6** | T1 `flatten` | require BOTH names (pair match) | `DenominatorAloneTerminates: got 2 keys` ⚠️ **combined total STAYS 6** | **[IMPL]** |
| **R1-R10** | T4 | neutralize each reject arm in turn | that row RED, the other nine GREEN | **[IMPL]** |
| **R-lift** | T4 | restore the `:568-570` pre-check | `TestLoad_AcceptsStaticLayer` RED | **[IMPL]** |
| **S1-S5** | T5 | the five stat-guard arms | per the T5 table ⚠️ **S3/S4 PASS the count-only idiom** | **[RUN — S3/S4 on a scratch harness]** |
| **F1** | T7 | delete the `num_keys` registration | `subj: envoy_runtime_num_keys ABSENT` — the **absent** branch, NOT a value mismatch | **[IMPL]** |
| **F2** | T7 | transpose the two gauge `Set` calls | `subj … = 2, want 6` **and** `subj: the two gauges look TRANSPOSED` | **[IMPL]** |
| **F3** | T7 | remove the blank import | guarded `-run` ⇒ `GATE FAIL: selector matched NOTHING` ⚠️ **bare `-run` EXITS 0** | **[IMPL]** |
| **F4** | T7 | rename `AssertStats` → `AssertStatsX` | guarded run ⇒ `GATE FAIL: AssertStats did NOT run` ⚠️ **the suite otherwise stays GREEN** | **[IMPL]** |
| **F5** | T7 | change `wantNumKeys` 6 → 5 | `ref … = 6, want 5` **and** `subj … = 6, want 5` — proves the leg is live on BOTH sides | **[IMPL]** |
| **G1-G11** | §4 | the gate negative controls | per §4 | **[ALL RUN at this PLAN]** |

**Declared MUST-NOT-FIRE: none.** Every row above is expected to go red.

**Declared MUST-STAY-GREEN under U5 and U6: the FIXTURE.** Both leave `num_keys` at 6 on the shipped config. **If the fixture goes red under U5 or U6, something else broke** — investigate rather than celebrate. This is the clearest statement of §1.1's finding: the fixture is a cross-side agreement pin, not a break detector.

⚠️ **F1 is the only break that exercises the absent branch.** `num_layers` is `Set` to a non-zero value on this fixture, so deleting ITS registration would nil-pointer the subject process and the run would die before `AssertStats` — the absent branch would never fire. **Break `num_keys`'s registration only, and confirm the ABSENT message, not a value mismatch.**

---

## Task 9 — `BEHAVIOR_CONTRACT.md` sweep

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md` (5746 lines at this tip)

- [ ] **Step 1: Amend the three "rejects still stand" sites**

⚠️ **`:906` reads *"still stand"*; `:926` and `:968` read *"**STILL** stand"*. A `grep 'still stand'` finds ONE of the three.** Use `grep -in 'still stand'` and re-derive all three line numbers before editing.

Each gains: *the `layered_runtime` reject is **partially lifted at phase 77** — the `static_layer` arm only, with the three sibling oneof arms and the zero-declared-layers case rejected by hand as **deliberate departures** (the reference accepts all four); `dynamic_resources` is **unchanged**.*

- [ ] **Step 2: Add the Phase 77 ledger line after `:5004`**

```
**Phase 77 — 1205 → 1207 (+2)** — `runtime.num_keys` + `runtime.num_layers`, …
```
⚠️ **It follows Phase 75 DIRECTLY.** `grep -n 'Phase 76'` over this file returns **ZERO hits** — there is no Phase 76 entry to follow.

⚠️ **RECORD BOTH LEDGER GAPS; BACK-FILL NEITHER.** `1198 → 1200` is explained at `:732` but not in ledger form; `1200 → 1201` is explained **nowhere in the file**. Fabricating a line would be inventing a record. The new entry names both gaps and states that the row asserts the **delta**.

- [ ] **Step 3: `1205` → `1207` at `:831` and `:847`**

`grep -n '1205'` must return exactly **3** before the edit (`:831`, `:847`, `:5004`) — re-derive. ⚠️ **`:847` also carries a stale `115-dir` figure against an actual 119, going to 120.** Fix it to `120-dir` in the same edit, or leave it and say why in `PROGRESS.md` — **do not silently carry it**.

- [ ] **Step 4: DO NOT look for `+20` cites near `:1857`**

**REFUTED at this PLAN (§1.9).** `:1857` is the QUIC-parity paragraph. The file's only two `+20` occurrences are at `:5697` and `:5730` and both mean *"plus phase 20"*. **There is no `+20` anchor to fix.** This step exists so the next reader does not re-inherit the instruction.

- [ ] **Step 5: Verify + commit**

```sh
grep -c '1207' docs/envoy-go/BEHAVIOR_CONTRACT.md    # 3
grep -c '1205' docs/envoy-go/BEHAVIOR_CONTRACT.md    # 1 (the Phase 77 ledger line's "from" side)
grep -in 'still stand' docs/envoy-go/BEHAVIOR_CONTRACT.md | wc -l   # 3
wc -l docs/envoy-go/BEHAVIOR_CONTRACT.md             # 5746 + exactly the lines added
```
⚠️ **Report the line-count delta explicitly.** Every by-line citation into this file from other documents shifts by it.

---

## Task 10 — `DECISIONS.md` prose amendments (NOT ADR-0299)

**Files:** Modify `docs/envoy-go/DECISIONS.md`

- [ ] **Step 1: ADR-0268's stale test roster — BODY line `:16430`** (heading `:16418`; both CONFIRMED at this PLAN)

It records that `validate/validate_test.go` carries *"4 reused `internal/bootstrap` reject-arm cases (dynamic_resources, **layered_runtime**, YAML syntax error, empty document)"*. **T4 falsifies it** — the `layered_runtime` case became an ACCEPT plus a new `rtds_layer` reject. Amend in place, **leading with what survives** (the other three cases are untouched).

- [ ] **Step 2: ADR-0278's stale code cite — BODY line `:16672`** (heading `:16670`)

It cites `bootstrap.go:499` for the pre-check. **The actual site is `:568-570`** and `ROADMAP.md:139` cites `:568-569`. Replace the line number with a **symbol anchor** (`Load`'s raw-YAML generic-map pre-check) so it cannot drift again, and note that phase 77 lifts its `layered_runtime` arm.

- [ ] **Step 3: `ADR-0089` stays BYTE-UNTOUCHED** — deliberate. It defers `/runtime` (`:3550`) and `POST /runtime_modify` (`:3543`) to this family; row 77 lands **neither**, so the un-defer is owed only by whichever later row does. Verify with `sha256sum` before and after this task.

- [ ] **Step 4: Verify + commit** — re-derive every touched line number after the edit; a line-adding edit in this 17494-line file shifts every ADR heading below it.

---

## Task 11 — the gates

**Files:** none permanently.

Run every gate in §4, each with its negative control **observed at this tip, not cited from this PLAN**. ⚠️ **A green gate you have not seen go red is not evidence** — and this lineage's broken-gate count is now **eleven**.

- [ ] **Step 1:** G1-G11 per §4, recording actual output.
- [ ] **Step 2: the FULL differential suite**, not just 0118:
```sh
go test ./test/differential/ -count=1 -timeout 60m > /tmp/full.log 2>&1; echo "EXIT=$?" >> /tmp/full.log
grep -c -- '--- PASS' /tmp/full.log; grep -c -- '--- FAIL' /tmp/full.log; tail -3 /tmp/full.log
```
⚠️ **A HARNESS'S EXIT CODE IS NOT THE COMMAND'S.** At the phase-76 IMPL a notification reported *"exit code 0"* for a run that had **PANICKED at 84 of 119**; that was the wrapper shell's status. **Capture the inner status explicitly and derive the tally from the log.**
- [ ] **Step 3:** `go test ./internal/... -count=1` and `-race` on the touched packages.
- [ ] **Step 4:** classify any failure against §6's hazard list, **and state the evidence for the classification** — never the classification alone.

---

## Task 12 — ADR-0299 completion, row 77 → `done`, stage close

**Files:** Modify `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/ROADMAP.md` (`:139`), `docs/envoy-go/STATE.md`, `next-prompt.txt`, `docs/envoy-go/phases/77-runtime-static-layer/PROGRESS.md`

- [ ] **Step 1: ADR-0299 §Decision + §Consequences, IN PLACE**

Append **after the RETAINED italic footer** (`:17494`, EOF at this tip), mirroring ADR-0295/0296/0297/0298. **No renumber. NO `---` separator** — the convention was abandoned and the file's last `^---$` is `:17020`, ~440 lines before ADR-0298 begins. Verify after:
```sh
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Context'       # 1
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'      # 1
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Consequences'  # 1
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^\*(§Decision'      # 1 (RETAINED)
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^---$'              # 0
```
⚠️ **Negative-control the `awk` range** by running the same block over ADR-0298 — it must give `### Decision` ⇒ 1, proving the range discriminates.

⚠️ **ADR-0299 carries NO whole-file grep count.** That species self-falsified in ADR-0296 ¶3 and ADR-0297 ¶7 **and** ¶9. Every count is line-scoped or stated with no numeral.

⚠️ **§Consequences must record the three §1.1 findings**: that the SPEC's own per-arm-count discrimination instruction is **unachievable**; that the roster is therefore split by layer; and that the fixture is an agreement pin, not a break detector.

- [ ] **Step 2: `ROADMAP.md` row 77 (`:139`) → `done`.** ⚠️ **This is the ONLY task that touches `ROADMAP.md`.**

⚠️ **NEVER WRITE A SENTINEL MATCHER STRING INTO `ROADMAP.md`.** The sentinel greps that file; `grep` cannot tell a mention from a use. This fired LIVE at the phase-76 BRAINSTORM, twice in one commit. **Row 77's summary must not contain the phrases the check-(2) matcher looks for.**

⚠️ **This row narrows NOTHING.** The Runtime candidates sentence (`:213`) lists RTDS, the two admin endpoints, disk layer, admin layer, the six `runtime_key` arms, the silent-IGNORE knobs, hot restart, graceful drain. Row 77 delivers **none** of them — it delivers the static layer, which the sentence does not name. **Check (2) stays 4 (old matcher) / 5 (broadened).**

- [ ] **Step 3: RE-RUN ALL THREE SENTINEL CHECKS AFTER the `ROADMAP.md` edit lands.** Expected: **(1) SILENT** (row 77 done — check (1) closes for the fourth time in project history) · **(2)** 5 broadened / 4 old · **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. **The sentinel does not fire; `stop` is NOT created.**

- [ ] **Step 4: VERIFY the fixed check (1) is present and RE-RUN it after the `ROADMAP.md` edit.** The field-parsed form was **installed at the phase-77 PLAN**, not here (§1.2). Confirm `next-prompt.txt` still carries it — a session that "restored" the regex form would silently re-open a fail-unsafe stop condition.

⚠️ **`want=109` MUST still equal the live data-row count.** This row adds no ROADMAP row, so 109 stands — but re-derive it rather than assuming, and bump it in this same commit if it moved.

⚠️ **Writing the matcher strings into `next-prompt.txt` is SAFE** — the sentinel greps `ROADMAP.md`, not this file.

- [ ] **Step 5: counts + `STATE.md` + `PROGRESS.md` + `next-prompt.txt` roll**, per §4's exit table. Roll the next-free ADR to **ADR-0300** and the next-free fixture index to **0119**.

- [ ] **Step 6: the six-gate (ADR-0106)** — §4 G1-G11, all green, each with its control observed.

---
## 4. Gates — every one NEGATIVE-CONTROLLED at this PLAN

**Legend:** **[RUN]** = executed at this PLAN, outcome recorded. **[IMPL]** = to be executed at the phase-77 IMPL.

### G1 — gofmt, on OUTPUT not exit code **[RUN]**
```sh
[ "$(gofmt -l internal/runtime internal/bootstrap validate test/fixtures/0118-runtime-static-layer | wc -l)" -eq 0 ]
```
**Control observed:** with a deliberately misformatted file, `gofmt -l <file>`, `gofmt -l <dir>` and `gofmt -l .` **all exit 0**, and `gofmt -l . && echo CHAINED_RHS_RAN` **printed `CHAINED_RHS_RAN`** — the `&&` form is inert. The output-based form gave EXIT **1** dirty / **0** clean. ⚠️ **Never chain `gofmt -l` with `&&`.**

### G2 — exported-symbol drift, ONE PACKAGE PER INVOCATION **[RUN]**
`go doc` is unusable for this. **Sharpened finding (§1.8):** `./nonexistent` **alone** exits 1 (the recorded "fails open with a `./` prefix" is REFUTED for that spelling); the real defect is that with a **valid arg1**, any arg2 is **silently swallowed** — byte-length proof `runtime alone = 285`, `runtime+bootstrap = 285`, `bootstrap alone = 19185`.

Use a `go/parser` AST lister, one package per invocation, exiting 2 on a bad directory. Baselines at this tip: `internal/runtime` **0** exported symbols · `internal/bootstrap` **11**.

**Controls observed:** `+type Loader` in `internal/runtime` ⇒ RED · `+func NegControlExported` in `internal/bootstrap` ⇒ RED · an **unexported** addition ⇒ GREEN (no false positive) · a **doc-comment-only** edit ⇒ symgate GREEN while raw `go doc -all` diffing goes **RED** (claim (a) demonstrated head-on).

⚠️ **ELEVENTH BROKEN GATE, found in this stage's own first draft:** against a package with **zero** exported symbols, `printf '%s\n' "$cur"` emits a stray newline versus a 0-byte baseline and **the gate goes RED on an untouched tree**. Write the lister's stdout straight to a temp file. **A `+0 exported symbols` gate over an empty package must be green-armed before it is trusted.**

⚠️ **Expected at the IMPL:** `internal/runtime` goes **0 → 4** exported symbols (`Snapshot`, `NewSnapshot`, `NumKeys`/`NumLayers`/`Keys` as methods). That is the row's intended surface and the gate should be re-baselined, not silenced. **`+0 new PUBLIC surface` in the envelope means the `validate/` public package**, which gains nothing.

### G3 — sha256 byte-untouched roster, FOUR legs **[RUN]**
```sh
git ls-files '*.go' | sort > universe.txt                       # 990
git ls-files 'internal/bootstrap/*' 'internal/runtime/*' 'validate/*' \
  | grep -E '\.go$' | sort > editroster.txt                     # 13
comm -23 universe.txt editroster.txt > byte_gated.txt           # 977
grep -cE '^(internal/bootstrap/|internal/runtime/|validate/)' byte_gated.txt   # 0
```
**Controls observed:** unchanged ⇒ `ok=977 missing=0 mismatch=0 roster-desync=0`, EXIT 0 · modified ⇒ `[MISMATCH]` with both hashes, EXIT 1 · **DELETED ⇒ `[MISSING]`, EXIT 1** · roster desync ⇒ `[ROSTER-DESYNC]`, EXIT 1.

⚠️ **The naive `[ -f ... ] || continue` idiom exits 0 on the same deletion** — EXECUTED. **A roster without a MISSING leg reads a deletion as "no mismatch".**

⚠️ `test/fixtures/0118-…/driver/driver.go` and `test/differential/runner_test.go` are NEW/EDITED at T7 — set-difference them out too, or the roster is desynced by construction.

### G4 — the stat delta, by NAME SET not count **[RUN]**
See T5. **Controls observed on a scratch harness:** `+1` ⇒ RED · `+3` ⇒ RED · one renamed ⇒ RED · **both renamed with the right count ⇒ RED** · zero registered ⇒ RED · the shipped green arm ⇒ PASS.
⚠️ **The landed `countMetrics` idiom PASSES the both-renamed arm** (`ok EXIT=0`). **Do not copy it.**

### G5 — the guarded `-run` selector **[RUN]**
```sh
out=$(go test ./test/differential/ -run "TestDifferential/$N" -count=1 2>&1); ec=$?
printf '%s' "$out" | grep -q 'no tests to run' && { echo "GATE FAIL: selector matched NOTHING"; ec=1; }
```
**Controls observed:** nonexistent `0118` ⇒ bare form `ok … [no tests to run]` **EXIT 0**, guarded form **EXIT 1** · real `0117` ⇒ banner absent, `ok … 7.274s`, EXIT 0.

### G6 — `internal/runtime` is a leaf **[IMPL]**
`go list -deps ./internal/runtime | grep -c 'pgdad/envoy-go'` ⇒ **1** (itself). At this tip the whole `-deps` output is one line.

### G7 — `go.mod` untouched **[IMPL]**
`go mod tidy -diff` EMPTY and `git diff master -- go.mod go.sum` EMPTY. Control: a fake `require` ⇒ 67 → 68.

### G8 — fixtures 119 → **120**, fuzzers **55**, internal packages **73**, BackendKind tail **38**, blank imports **120** **[IMPL]**
Each with the control observed (`mkdir …/0119-fake` ⇒ 121; a throwaway `func FuzzProbeOnly` ⇒ 56; a throwaway `ZZProbeKind = 39` ⇒ tail 39).

### G9 — the full differential suite **[IMPL]** — see T11 Step 2. **Capture the inner exit status; derive the tally from the log.**

### G10 — the three sentinel checks, re-run AFTER the `ROADMAP.md` edit **[IMPL]** — plus the **fixed** check (1) (§1.2), armed on all five arms **[RUN]**.

### G11 — ADR-0299 block shape, with the `awk`-range negative control over ADR-0298 **[IMPL]** — see T12 Step 1.

---

## 5. Deferred — named so a future sweep finds them

1. **The `runtime.*` name-set divergence** — envoy-go publishes **2** names where the reference publishes **9**, in every arm. A future row asserting full-set equality will fail, correctly.
2. **`NamePattern` accepts an empty INTERIOR segment** (`a..b`) while rejecting a trailing one. This row is not exposed (it derives no stat name from a runtime key), but runtime keys are arbitrary operator-chosen dotted strings. ⚠️ **The validator catches a hyphen and does NOT catch `a..b`.**
3. **`""` means "stored, counted, loses resolution" in a static layer and "DELETE" through `/runtime_modify`.** Same character, same subsystem, opposite effect by write path.
4. **`load_success` / `override_dir_not_exists` are per-load counters, not boot-once**, and both read **1** even with no `layered_runtime` block. Neither is a load-ran guard.
5. **The `/runtime` body cannot discriminate absent from empty** — `layer_values` renders both as `""`.
6. **The reference's empty-map-value wart** — a garbage rotating DebugString as `final_value`, still counted.
7. **envoy-go's `num_keys` is BOOT-FIXED where the reference's is LIVE** — inherited by the `/runtime_modify` row.
8. **Within-layer collision determinism** — envoy-go picks an explicit rule (sorted keys) and is *more* defined than the reference. Visible only once a row serves `/runtime`.
9. **The `%g` rendering divergence** — the highest-value single arm is `n: 2147483647` ⇒ reference `2.14748e+09` vs a Go `strconv` default `2147483647`. ⚠️ **NON-INJECTIVE**, so that row cannot round-trip a value ≥ 10⁶ to verify what it set.
10. **The documented PUBLIC IMPORT PATH does not exist.** `head -1 go.mod` is `github.com/pgdad/envoy-go`; the docs say `github.com/esalaine/envoy-go/validate` on 20 lines / 24 occurrences across four docs. ⚠️ **`DECISIONS.md:142` is `## ADR-0006: module path github.com/esalaine/envoy-go` — an ADR that DECIDES the wrong path, never superseded**, and phase 77's own BRAINSTORM propagates it. A session copying it writes code that does not compile. **Beyond this row's chartered edit set.**
11. **Normalising the Operational-tooling paragraph (`ROADMAP.md:221`) to the long form** so both check-(2) matchers agree.
12. **A mechanical stat-surface count** to replace the documentary 1205 — ⚠️ **and now more clearly owed**: `BEHAVIOR_CONTRACT.md`'s ledger has a `1200 → 1201` step documented **nowhere in the file** (§1.10). Only *partially* constructible (non-literal names, `statroster` fan-out, registration functions passed as method values). **8-11 tasks**, not the small row it sounds like.
13. **`internal/bootstrap`'s other 47 inline `fmt.Errorf("bootstrap: …")` arms** — this row introduces the named-constant discipline for its own nine only.
14. **`internal/statssink`'s four *"stays 1200 / 1196"* prose sites**, stale since phase 49.

### ⚠️ STRUCK, not deferred
**`BEHAVIOR_CONTRACT.md:1857`'s "two stale `+20` cites, plus two more in the same file"** — **REFUTED at this PLAN (§1.9). No such anchor exists anywhere in the file.** Router item 16 and SPEC §13 item 8 both carry it; **both are wrong and it must not be re-inherited.**

### The banked next subject — re-cost before adopting
**`fault.abort.grpc_status`, ~7-9 tasks / +0 on every envelope axis.** `abortEnabled` is set only inside the `HttpStatus` type switch, so a `grpc_status` variant fires **no abort at all**. A 4-arm probe found the reference branches on the request **`content-type`**, returns HTTP **200** in every arm, and emits `grpc-status`/`grpc-message` as response **HEADERS not trailers** — which retires the framework blocker. ⚠️ **It is an HTTP-filters-family row and does NOT open the gRPC family**; the contrary claim's provenance is a **ROUTING LABEL** in row 09's summary, and **reading a filing label as a charter is the error.**

⚠️ **`reference_deferred_candidate_cost_restale`: re-derive at the tip that adopts it.** ⚠️ **AGREEMENT ACROSS DOCUMENTS IS NOT EVIDENCE — a claim that has survived several stages is MORE suspect than one written yesterday, not less.** This stage refuted three of its own controller's predictions and one instruction carried by four documents.

---

## 6. Known live hazards — never reflex-classify any of these as a regression

The full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **BEFORE any assertion**, the latter as a **PANIC that aborts the whole binary**, firing **more readily under `-race`**) · `reference_sds_init_fetch_timeout_dial_budget_flake` (TWO packages) · the PRE-EXISTING `internal/cluster` `-race` outlier flake · two still-**UNINDEXED** load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`).

⚠️ **`0061-lb-ring-hash` is NO LONGER a live flake** — resolved at phase 76 (`sourceIPs` 4 → 16). **A spread failure there is now a FINDING.**

⚠️ **A stage brief's flake list is not the index — the SIXTH consecutive stage at which that holds.** Isolate-re-run, then state the classification **AND its evidence**.

⚠️ **New at this row:** the reference's `/runtime` body is non-deterministic in **two** independent ways (per-request JSON member order; per-process debug marker — 8 distinct strings in 13 processes). **Nothing in this row reads that body.** If a future diff of it appears to flake, it is not flaking — it is correct.

⚠️ **Unmeasured until T11:** the full-suite effect of a 120th fixture on runtime and on the `bind: address already in use` startup flake, which fires more readily as the fixture count grows.

---

## 7. Self-review against the SPEC

| SPEC section | covered by | note |
|---|---|---|
| §1.1 the refuted founding premise | T3, T4, T7 prose | departures recorded AS departures in the constants, the tests and both fixture docs |
| §3.2 D-RSL-EMPTY (parse-reject) | T3 arm 9, T4 | ⚠️ EXECUTED: both spellings indistinguishable after unmarshal ⇒ **one predicate, two test rows** |
| §3.3.1 collision | T2 | value NOT retained — the reference's winner is non-deterministic |
| §3.3.2 the LEXICAL rule | T1 | 14-row roster incl. case-sensitivity, either-name-alone, invalid-values-still-terminate |
| §3.3.3 the EMPTY-STRUCT branch | T1 | the second termination branch, + the degenerate-root guard |
| §3.3.4 the inverted precision hazard | T7 Step 1 | small integers / short strings only; recorded in §5 for the `/runtime` row |
| §3.4 empty-string semantics | not exercised | ⚠️ the fixture uses no empty-string value; §5 item 3 carries it |
| §3.6 stats +2 | T5 | ⚠️ **name-set, not count** — the SPEC's `TestNoNewStat*` class is blind to a rename |
| §3.8 `internal/runtime` imports `structpb` alone | T1, T2, G6 | structural, not asserted |
| §5 identifier hygiene | §1.4 | ⚠️ **`Snapshot` DECLARES ONCE — the SPEC's `^type Snapshot` regex missed a method** |
| §6 the nine-arm roster | T3, T4 | wasm-style roster-size guard; arm 10 stays DROPPED |
| §7 `NamePattern` | T5 | both names valid; `a..b` deferred |
| §8.1 stats-only | T7 | four reasons, all re-confirmed |
| §8.2 asserter shape | T7 | `Fatalf` preconditions, `Errorf` properties, **separate absent check with `continue`** |
| §8.3 the four arms | T7 | ⚠️ **the arms SHIP; the instruction that a break be identifiable from the gauge is WITHDRAWN as impossible (§1.1)** |
| §8.3 the fifth (`%g`) arm | **not scheduled** | as the SPEC recommends — recorded in §5 item 9 |
| §8.4 construction constraints | T7 | `BackendCount()` = 1, three registration gates, prose-only expectations |
| §9 behavior-contract map | T9 | ⚠️ **the `+20` site does NOT exist** |
| §10 the task spine | T1-T12 | 12 tasks (T2 folded into T1; T10/T11 split out) |
| §11 edit roster | §3 | + `internal/runtime/doc.go` **deleted** rather than left stale |
| §12 sentinel | T12 | narrows nothing; **and the shipped check (1) is replaced — §1.2** |
| §13 deferrals | §5 | 14 carried, **1 STRUCK** |
| §14 ADR-0299 | T12 | §Consequences must record §1.1 |
| §15 exit counts | §4 G8 | |

**Gaps found and closed:** the SPEC's §8.3 discrimination instruction (impossible — §1.1) · its §5 collision check (missed a method — §1.4) · its §10 T7 guard class (blind to renames — §1.7) · its §13 item 8 (the target does not exist — §1.9).

**Gap found and NOT closed:** SPEC §3.4's empty-string semantics are settled but **unexercised by anything this row ships**. Adding an empty-string key to the fixture would add one key to `num_keys` and change every measured number for a property no cross-side assertion can see (`layer_values` renders present-but-empty byte-identically to absent). **Deliberately not added**; §5 item 3 carries it.

---

## 8. Operative memories

`reference_lifted_reject_hidden_enforcement` · `reference_probe_must_discriminate` · `reference_gate_command_negative_control` · `reference_ordered_assertion_legs_vacuous_on_constant_change` · `reference_fatalf_makes_assertions_unreachable` · `reference_fixture_tb_has_no_logf` · `reference_differential_run_selector` · `reference_spec_drafted_identifier_collision_check` · `reference_differential_fixture_three_registration_gates` · `reference_differential_fixture_port_convention` · `reference_differential_backendcount_min_one` · `reference_differential_asserter_dispatch` · `reference_stats_sink_emits_used_only` · `reference_protojson_null_decodes_to_nil` · `reference_xds_config_seam_transitive_cycle_guard` · `reference_new_subpackage_pulls_transitive_module` · `reference_fuzzer_count_docs_drift` · `reference_liveness_break_needs_failing_baseline` · `reference_differential_break_protocol_count1` · `reference_break_protocol_commit_first` · `reference_deliberate_break_wrong_assertion` · `reference_change_set_measure_not_build_measure` · `reference_harness_exit_code_is_not_command_exit_code` · `reference_quoting_is_not_executing` · `feedback_brief_citations_not_evidence` · `reference_a_drift_correction_is_itself_a_claim` · `reference_document_hygiene_claim_not_evidence` · `reference_sentinel_matcher_string_self_clears` · `reference_code_comment_not_evidence` · `reference_verification_table_launders_wrong_cites` · `reference_deferred_candidate_cost_restale` · `reference_next_prompt_tracked_despite_gitignore` · `reference_full_suite_race_after_background_mutator` · `reference_differential_fullsuite_startup_flake` · `reference_cluster_race_outlier_flake` · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `feedback_subagent_worktree_detach` · `feedback_subagent_worktree_path_targeting` · `reference_parallel_subagents_private_scratch` · `feedback_pertask_gofmt_lint` · **`reference_bash_cwd_reset_commits_to_main`**.
