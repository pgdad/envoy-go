# PROGRESS 81 — stats-name-charset-guards

Stage-by-stage execution record. One section per lifecycle stage, appended at that stage's close.

---

# BRAINSTORM record (2026-07-31)

**Stage:** BRAINSTORM (lifecycle-state `DONE` → `1`). **Row 81 REGISTERED `in-progress`** at this commit. Sentinel `want` **112 → 113** in the same commit. Base master **`4f8e159c`** (phase-80 IMPL squash tip, located by SUBJECT). Worktree `/home/esa/git/envoy-go-wt-phase81`, branch `phase-81-brainstorm`. **SELF-PICKED** per the 2026-07-12 standing directive.

## What was EXECUTED at this stage

**Four costing agents in parallel**, all read-only in one shared worktree with private scratch dirs, none writing to the repo (`git status --porcelain` = 0 lines at every checkpoint). Controller re-derived every load-bearing claim itself rather than adopting a brief (`feedback_brief_citations_not_evidence`).

| agent | subject | outcome |
|---|---|---|
| A | dynamic-token charset exposure | **PICKED** — and found the request-time crash |
| B | `STATE_HISTORY.md` gap + `ROADMAP.md` malformed rows | rejected; 2 refutations + 2 new blockers |
| D | differential bind-retry + `--mode validate` | rejected; 5 refutations |
| E | gRPC + WASM family opens | rejected; 1 load-bearing refutation |

## ⚠️ THE HEADLINE: THE PICK RESTS ON A CRASH THIS STAGE PROVED, NOT INHERITED

`internal/rbac/perpolicy.go:25` lazily registers `<base>.policy.<policyName>.<suffix>` at **request time**, guarding `policyName` for emptiness only. `git grep -n 'IsValidName' -- internal/rbac/` returns **no output**. `internal/stats/registry.go:115-119` **panics** on an invalid name, and no `recover()` covers the request path.

Controller-run probe (written into `internal/rbac/`, executed, deleted; worktree verified clean afterwards):

```
NC OK: policy name "allow_admins" registered cleanly
CONFIRMED request-time PANIC: stats: invalid metric name:
  "http.myhcm.rbac.policy.allow-admins.allowed"
```

**The NC arm is what makes this a result** — the underscore spelling registers cleanly, so the probe discriminates on the hyphen alone.

## Refutation ledger — what EXECUTION found that `next-prompt.txt` got wrong

**Load-bearing (changed the pick or the scope):**

1. **`ROADMAP.md:76` does NOT decline a WASM row — a category error.** The quoted sentence is real (verified verbatim, and it occurs **exactly once** in the file) but says *"the FINAL §9 **HTTP-filters**-family row"*. `ROADMAP.md:218` is a **separate `### WASM host family` heading** (`:220` — *"Own multi-phase sub-project"*). A WASM-host row is not an HTTP-filters row. What `:76` correctly kills is the *rider*; a standalone row is untouched. ⇒ **WASM is live and defensible whenever it is smallest.**
2. **`--mode validate`'s defect runs the OPPOSITE way.** Not false-accept — **false-reject**, 3/3 against the pinned reference `envoyproxy/envoy:contrib-v1.37.2`. envoy-go `EXIT=1` on fixtures 0103/0108/0109; reference `configuration OK`, `EXIT=0`. Those three are live, passing differential fixtures.
3. **"The five existing rejects" is wrong by 3×** — there are **FIFTEEN** production `IsValidName` guard sites (controller-re-derived census in BRAINSTORM §2.2).
4. **The failure mode is a direct `panic`, not a nil counter.** `checkName` panics; registration never returns nil. `reference_nil_stats_counter_inc_crashes_goroutine` is the wrong mechanism for this surface — there is no nil to defend against.
5. **The bind-retry fix as prescribed cannot work.** All 26 backend arms spawn `go run`, so `cmd.Start()` returns `nil` on collision and the failure surfaces later at the child's `Listen`. A `startSubjectWithRetry` equivalent keys on a start error that never occurs.

**Bookkeeping / gate-shape (recorded, did not change the pick):**

6. **The archive gap is 57, not 58** (join key `(phase-id, STAGE)`, slug discarded). Two extractor traps cleared: slug drift (31 spurious misses) and **two distinct archive bullet shapes** (`STATE_HISTORY.md:428/:430` merge annotation and bullet on one line, inflating a naive count to 59). "36 for phases 67-75" **CONFIRMED exactly**. 57 is a **lower bound** — 87 pre-slug-convention subjects are invisible to the canonical extractor.
7. **The obvious archive guard SELF-CLEARS.** Without the `(slug)` parens, `phase 77 PLAN done` matches **`STATE.md`'s own confession prose** at `:18` and `:20`. `reference_sentinel_matcher_string_self_clears`, in a new file.
8. **CI is a shallow clone** — `actions/checkout@v6` with no `fetch-depth` ⇒ depth 1, killing any `git log`-derived expectation set. The git-free alternative was tested and **measurably fails** (380 stages from disk vs 209 from git).
9. **Naive `NF==8` also throws 15 FALSE POSITIVES**, not merely missing row 78. Unusable in both directions.
10. **`-count=6` is not a gate** — ~40 min to test for absence of a rare event; six clean runs occur with p ≈ 0.53 even if the bug is untouched.
11. **"close-then-rebind" is CURRENT, not stale** — the landed `f2dd994a` helper comment uses exactly that wording. What is stale is the ephemeral-range/loopback framing.
12. **`harness_test.go`'s "4 more" call sites contain ZERO backend arms.**
13. **A landed memory overgeneralized.** `reference_grep_c_zero_is_a_broken_gate` claimed GNU `grep -c` on a file list prints nothing. **False** — only `git grep -c` is silent; GNU/ugrep always print (`0`, or `path:0` per file). Both exit 1. **Memory corrected.**
14. **The router's `mongo collection names` hypothesis is REFUTED** (3 of 3 wire-derived tokens already guarded, denominator stated); its `wasm plugin names` hypothesis is **CONFIRMED**.

## Findings no phase-80 document carries

- **Production-dead trailer code** in four filters, incl. `admission_control`'s entire deferred gRPC-status state machine.
- **A vacuous differential arm** — `0036`'s `e_trailers_read` asserts `x-trailer-count=0` on a trailer-less GET.
- **Two stale non-test comments** claiming `RunDecodeTrailers` doesn't exist; it does, at `internal/filter/http/chain.go:455`.
- **`DECISIONS.md:13179`** enumerates 24 wasm module-function keys, **8 of which do not exist in `internal/`**.
- **Two sibling `freeTCPPort` helpers** still carry both pre-`f2dd994a` defects (`cmd/envoy-go/main_test.go:180`, `test/conformance/h2spec/h2spec_test.go:219`).
- **There is no doc-invariant test anywhere in the repo** — the 5 `_test.go` files naming these docs mention them in comments only.
- **A doctrinal contradiction to reconcile at the SPEC**: phase 80 says sanitizing is *"FORECLOSED by ADR-0065"*, yet `internal/listener/manager.go:352` `normalizeAddr` sanitizes.

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire

| check | ACTUAL output | NC, observed FIRING |
|---|---|---|
| **(1)** `want=112` | **NOTHING** | row **62** doctored ⇒ `NC NOT DONE: row 62` (`rows doctored: 1`); `want=111` ⇒ `NC GATE FAIL: examined 112 data rows, expected 111` |
| **(2)** | **FIVE** — `:190 :200 :210 :216 :224` (PRE-EDIT; ⚠️ **this row's own insertion shifted them to `:191 :201 :211 :217 :225` — the line-anchor-drift hazard firing on this stage's own document, caught only by re-running the gate on the FINAL tree**) | one-arm strip ⇒ **5 → 4, not 5 → 0** |
| **(3)** | `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` | invented slug fires; `Observability` correctly silent |

Input **228 lines / 112 data rows / 13** bare `candidates:` hits. **(2) and (3) print ⇒ the sentinel does NOT fire.** `ls stop` ⇒ `No such file or directory`; **NOT created**.

⚠️ **Check (1) is silent, so only a doctored-copy NC distinguishes it from a broken check. One was run and it fired.**
⚠️ **CHECK (2) UNCHANGED AT FIVE — THIS ROW NARROWS NOTHING, STATED NOT FORECAST.** Twenty-eighth consecutive phase.

**Leak check (row 81's cell only): PASSED.** 0 deferred-candidate phrases; one family slug, `Observability-family row`, already registered **52×** elsewhere ⇒ a use, not a mention.

**Row-81 well-formedness: the DISJUNCTION was required.** Over the live 112 rows, ARM-A flags **57, 69 only**; ARM-B flags **78 only** — independently reproduced by the controller. On a synthetic row 81 given **both** defects, **ARM-B fires, ARM-A stays silent, and naive `NF==8` PASSES** — `reference_compensating_defects_cancel_in_the_gate_metric`, on a row this stage authored.

## Counts re-derived at this tip

`ROADMAP.md` **228 → 229** lines / **112 → 113** data rows · `DECISIONS.md` **17724**, **301** ADR headings, tail **ADR-0302 COMPLETE**, next-free **ADR-0303** · `BEHAVIOR_CONTRACT.md` **5868** (untouched) · `STATE_HISTORY.md` **430 → 432** · fixtures **120** (naive predicate gives 118; next-free **0119**) · fuzzers **55** · internal packages **73** · blank imports **120** · **production `IsValidName` guard sites 15** · production stat-registration call sites **210 of 508** tree-wide.

**Normative cites re-verified EXACT**: `/BOOTSTRAP_PROMPT.md` §6.1 `:285` (`:289`/`:290` triggers, `:291` blank, `:292` third trigger); §7.5 `:357`, `(a)-(f)` `:360-365`, close `:367`.

## Six-gate (§7.5) — posture for a docs-only BRAINSTORM

(a)/(b)/(c)/(e) **not owed** — no `.go` and no fixture touched · (d) **VACUOUS**, said to be vacuous rather than green (55 fuzzers repo-wide, **0** added) · (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`; none since 25.3, 84 of 121 phase dirs carry none.** Recorded as a departure, not compliance.

## Hygiene

Docs-only: **ZERO production `.go`, ZERO test `.go`**. `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED** (`git diff --stat` shows neither). The §1 probe was deleted and the worktree verified clean before any commit.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED, OBSERVED THREE TIMES** (`Shell cwd was reset to /home/esa/git/envoy-go`). Twenty-fifth consecutive session. All git commands used `git -C <abs-path>`; branch tripwired as `phase-81-brainstorm`, never `master`.

**Broken-gate count stays EIGHTEEN** — no nineteenth shape. Two priors fired live: the compensating-defect cancellation (on this stage's own synthetic row) and the self-clearing matcher (in `STATE.md`, a new file for that shape).

## Next

**→ the phase-81 SPEC.** It must dispose **D-81-HELPER** first: a shared `internal/stats` helper makes the full 120-fixture differential **and** an explicit h2spec run mandatory for every later stage of this row; per-package inlining does not.
