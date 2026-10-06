# Phase 101 — `listener-filters-timeout-envelope-lift` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Stage:** PLAN (lifecycle **2 -> 3**). Worktree `/home/esa/git/envoy-go-wt-p101-plan` off master `7d18bcba`,
branch `phase-101-plan`. Written under the 2026-07-12 standing directive: **no human consulted.**
**Spec:** `docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/SPEC.md` (791 lines). The plan argues
from the spec, and executors read both. **§0 records thirteen findings against the SPEC and the measurements
that settle its two unmeasured cells. Where the two disagree, this PLAN wins, and §0 says why.**
**Evidence only:** `BRAINSTORM.md` (535 lines). Where it disagrees with `SPEC.md` §0, the SPEC governs; where
the SPEC disagrees with this PLAN's §0, this PLAN governs.

**Goal.** Accept every `listener_filters_timeout` the pinned reference accepts and refuse exactly what it
refuses. Today envoy-go boot-rejects anything outside its own `[1s, 60s]` envelope on every listener. After the
row, the value is a `uint64` of milliseconds computed from the Duration's fields. nil gives 15000. A negative
seconds or nanos field is rejected (`expected a positive duration`), and so is a seconds field above
`9223372035` (`duration out of range`). Everything else truncates to whole milliseconds, and 0 disables the
timeout. **+0 stat NAMES.**

**Architecture.** The SPEC's shape **A** is kept unchanged (Appendix A, `manager.go 8 9`, `pipeline.go 1 1`,
`pipeline_deadline_test.go 1 1`). `parseListenerFiltersTimeout` returns `uint64` and reads `GetSeconds()` /
`GetNanos()`, never the saturating `AsDuration()`. `listenerRuntime.lfTimeoutMs` and `Pipeline.Run`'s
`timeoutMs` widen to `uint64`, so the seven untyped `Run(…, N)` test callers keep their meaning
(milliseconds). The rest of the plan makes the edit falsifiable:
- 15 new unit arms, 2 envelope pins re-pointed and renamed, and 1 QUIC build arm;
- the six-listener differential fixture `0126`;
- five NC mutants scored per arm (NC5 recorded vacuous);
- a six-sub-gate layout gate over `pipeline.go`, each sub-gate shown to fire;
- the comment and doc reconciliation.

**Every code block in the appendices was built, run and reverted at this PLAN stage (§1.2, §4). None of it is
a sketch.** §10 also re-extracted every appendix from this file and applied it in IMPL order.

**Tech stack.** Go; `go-control-plane` v3 protos (`durationpb`); `internal/listener` and its `listenerfilter`
package; the differential harness against `envoyproxy/envoy:contrib-v1.37.2` by digest.

---

## Global Constraints

These apply implicitly to every task.

- **Reference pin:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`). **Run it BY DIGEST**, after verifying `docs/envoy-go/ENVOY_TARGET.md` lines 3-4.
- **`grep` is a `ugrep` shell function in EVERY shell here.** It honours `.gitignore`, and `next-prompt.txt`
  is gitignored yet tracked. Use `/usr/bin/grep`, `git grep`, or a direct path, and name the binary.
- **Use `git -C <abs-worktree>` for every git command.** The Bash tool's cwd silently resets, and shell
  variables do not survive between tool calls. Put `pwd` + `git rev-parse --abbrev-ref HEAD` before any
  commit.
- **`go test` rules:**
  - `-count=1` is not optional, and use `-v` whenever you count: `RUN=0` beside `RC=0` is a vacuous green.
  - Take rc from `PIPESTATUS[0]` or `out=$(…); rc=$?`, never from the end of a pipe.
  - The FAIL matcher is `^(FAIL|--- FAIL)|^ *--- FAIL`. The panic gate is `^panic:|DATA RACE|SIGSEGV`.
  - A `-run` selector that matches nothing prints `[no tests to run]` and EXITS 0.
- **Lint:** `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`. **misspell runs in US locale**, so no British
  spellings go into `.go` comments. Check `gofmt -l` by its OUTPUT; it never exits non-zero.
- **The appendices live in THIS file as fenced blocks — EXTRACT them, never re-derive them.** Extract one by
  its heading:
  ```bash
  ext () { awk -v want="$1" '$0 ~ "^## Appendix " want " —" {f=1; next} f && /^```/ {c++; if (c==1) {o=1; next} if (c==2) exit} o' "$2"; }
  P=docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/PLAN.md
  ext A $P > $S/A-code.diff
  ```
  **Always `git apply --check` first.** For a whole-file appendix (F.1-F.3, G), compare `wc -l` against §1.2's
  figure before trusting the extraction. §10 records the extraction, re-run at this PLAN's close.
- **Cost figures come from `git diff --numstat`, never `--stat`.**
- **Build with `-o <scratch>`.** A bare `go build ./cmd/envoy-go/` drops a binary in the worktree.
- **Ports:** the ephemeral range is `32768-60999`, and the harness reserves `20000..31007` and `11000..14999`.
  **`0126`'s reference ports `15126` and `15232`-`15236` were censused at this tip** (§2.4). Re-census them at
  the IMPL tip.
- **Never tear down a container this session did not create**, and then only BY NAME. A `reaper_*` container
  belongs to the differential itself.
- **Shape A is the decision. Do NOT "simplify" to `time.Duration`** (`SPEC.md` §0.1: every untyped
  `Run(…, 1000)` caller silently becomes 1000 ns, a FLAKE), **nor to a `uint32` clamp** (§0.6: two red value
  arms).
- **`pipeline.go` layout is GATED (§6).** Six sub-gates must pass:
  - `:33-37` md5-identical and `:43` byte-identical;
  - `:32` changes only `uint32` → `uint64`;
  - the import block is unchanged;
  - exactly one CODE `context.WithTimeout`, at `:43`;
  - no line-count-changing hunk at or above `:43`.
- **+0 stat NAMES.** The ledger entry is **delta-only `+0, UNCHANGED`, quoting NO absolute** (`SPEC.md` §8.2).
- **Do NOT add a QUIC differential arm, a 61 s timing arm, a 1 ms immediate-request arm, a reject-message pin,
  a close-KIND pin, or the `downstream_listener_filter_*` names**, and do not touch `stats_flush_interval`.
  Each is measured, named and banked (`SPEC.md` §1, §7.5-§7.7).

---

## Review Focus

These are inputs the SPEC implies but its own arm table did not exercise. Each line is pinned by a test in the
task named.

1. **A mixed-sign Duration built in Go** (`{Seconds: 1, Nanos: -5e8}`; protojson can never produce it, but
   envoy-go runs no PGV). It must be REJECTED, not read as 0.5 s. This is pinned implicitly by V2 and
   explicitly by the NegativeNanos arm `{0, -5e8}`. **Its falsifier is NC2** (Task 9).
2. **The largest accepted value, carried through `Pipeline.Run`.** It must not overflow `time.Duration` into a
   negative (fire-at-once) deadline. `TestListenerFilterTimeoutPipelineHoldsAtMax` (Task 2) holds it for 1 s,
   and it stays green under NC4 because 9223372036000 ms × 1e6 is still below the `int64` maximum (§0.3).
3. **A QUIC listener carrying an old-envelope-violating value.** It must BUILD, which is the reference's
   accept-and-ignore. `TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds` covers it (Task 3).
4. **A `true` listener whose deadline fires before the client speaks.** It must fall through to the DEFAULT
   chain, not drop. This was MEASURED on both sides (§0.11), and fixture `0126` T1 plus S `l_half_true` 1
   pin it (Task 4).
5. **A `uint32`-wrapping value (`4294968s`).** It must hold the connection open, not drop it at ~705 ms. Unit
   `…AboveUint32MsAccepted` + `…PipelineHoldsPastUint32Wrap` (Task 2) and fixture W1 pin it. **NC1 reddens
   all three** (§4).

---

## 0. What this PLAN refuted, by execution

Two measurement agents built every code block in throwaway detached worktrees off `7d18bcba`:
- The **subject** agent (Docker-free) built Appendix A and the comment pass, the final unit tests, the QUIC
  arm, the NC roster at unit level and the layout gate.
- The **fixture** agent (the only Docker user) built `0126` and ran it nine times, plus `0125` once under A.

The controller re-ran the load-bearing results first-hand in one more throwaway worktree: the tip RED set
(292 RUN, 16 top-level FAILs), the final green (292 RUN, 0 FAIL), the layout gate (0 failed sub-gates), and
NC4 with its marker asserted. It also re-read the fixture logs (R0's boot-reject line; R1-R3 PASS; NC1 W1 FIN
at 704 ms; NC4 M1). **Every worktree was created detached and removed; the only one left is the stage
worktree.**

### 🔴 0.1 — `SPEC.md` §6.2's "drop the `p101ToMs` shim" makes the tests UNCOMPILABLE at the tip, so §14 item 7's RED capture is impossible as written

Under A the parse returns `uint64`. At the tip it returns `uint32`. Without the shim, the comparison against a
`uint64` want fails to compile: `listener_filters_timeout_test.go:351: cannot use … uint32 as uint64`, and the
QUIC arm gives `quic_test.go:1881: mismatched types uint32 and uint64`. The package reads `[build failed]`,
rc 1, **RUN 0**. A compile failure names no arm, so "the 11 RED arms named" could never be recorded.

**The fix, built and run:** the parse helper returns `uint64(v)` (`v, err := parseListenerFiltersTimeout(…);
return uint64(v), err`), and the QUIC arm compares `uint64(rt.lfTimeoutMs)`. Both conversions are no-ops under
A. No enabled linter flags them (`unconvert` is not in `.golangci.yml`, checked). They are what let the tests
land and go RED **before** the production edit (Task 2 before Task 6). That ordering is load-bearing, because
nothing after Task 6 can recreate the un-fixed measurement.

### 🔴 0.2 — 13 of the 15 new arms are RED at the tip, not 11: U-negnanos and U-pmax were GREEN only because Appendix B asserts NO message for them

`SPEC.md` §0.4 and §6.2 say U-negnanos and U-pmax are "GREEN at the tip for the wrong reason". That is true of
Appendix B, which passes `""` as the expected phrase. The final arms assert the phrase:
- `NegativeNanosRejected` wants `expected a positive duration`;
- `ProtoMaxRejected` wants `duration out of range`.

At the tip both are RED on the MESSAGE (`reject message … does not contain …`), because the envelope's message
is different. **So 13 of 15 are RED, and only U-nil and U-zero stay green as regression guards.** The two
arms' falsifiers remain NC2 and NC3 (§4.1), but they now also carry a TDD red. Method note 7g: assert WHICH
error fired.

### ⚠️ 0.3 — `SPEC.md` §9 NC4 OMITS `MaxAccepted` from its must-redden set

Rounding (`+ 500000` before `/1e6`) turns `9223372035.999999999s` into **9223372036000**, not 9223372035999,
so `TestParseListenerFiltersTimeoutMaxAccepted` FAILS under NC4. The subject agent measured this and the
controller confirmed it first-hand: `value: got 9223372036000 ms, want 9223372035999 ms`. `PipelineHoldsAtMax`
still holds under NC4, because 9223372036000 ms × 1e6 = 9.223372036e18 ns is below the `int64` maximum. That
is the opposite of the overflow `SPEC.md` §2 warns of for `9223372036s`. **§7 corrects the row.**

### ⚠️ 0.4 — the occurrence set has a code-comment hit NEITHER matcher catches: `manager_test.go:4053`

`// 1s pipeline timeout (ADR-0082 floor); slowListenerFilter blocks 2s.` is a present-tense claim that a 1 s
floor exists. `SPEC.md` §5's table lacks it, and both §5 matchers miss it (no `60000`, no `envelope`, no
`[1s, 60s]`). It is reconciled inside Appendix D (the comment drops "floor"). **Method note 49 again: a matcher
reports only what it spells.**

### ⚠️ 0.5 — the QUIC build siblings live in `manager_test.go`, not `quic_test.go`

`SPEC.md` §4 and §6.3 say `quic_test.go` builds QUIC listener runtimes. The build-only QUIC tests are
`TestBuildListenerRuntime_QUIC*` in `manager_test.go`. Appendix E still puts the new arm in `quic_test.go`, on
that file's own `mkQUICListener` / `NewManager` pattern, because the arm drives a full `NewManager` build like
its neighbours there. The SPEC's placement is kept, and its premise is corrected.

### ⚠️ 0.6 — `pipeline.go:32` is NOT cited; `:43` is (eight times) and `:33-37` once

`SPEC.md` §4 says `:32`, `:33-37` and `:43` are "all cited". A literal census finds:
- **8** citations of `:43`: `manager.go:479`, `BEHAVIOR_CONTRACT.md:1969`, and six in `DECISIONS.md`;
- **one** of `:33-37`;
- **zero** of `:32`.

The gate keeps a `:32` sub-gate anyway (c), because A edits that exact line and the sub-gate proves the edit is
the widening and nothing else (§6).

### ⚠️ 0.7 — `SPEC.md` §5's docs table misses five hits; none of them needs an edit

The doc census (both matchers, unioned, each hit resolved by backward `^## ADR-` search) adds:

| hit | what it is | verdict |
|---|---|---|
| `BEHAVIOR_CONTRACT.md:5150` | the phase-100 ledger entry, which says the reference's QUIC registration is "NOT measured" (`SPEC.md` §0.2 has since measured it) | historical ledger; leave it, and have the phase-101 entry say it was measured |
| ADR-0081 (a) `:3171` | the O(N × D) complexity claim | true; leave it |
| ADR-0082 `:3048`, `:3054`, `:3056` | the pipeline intro, the `continue…` semantics, the shared budget | ADR-0082 is superseded clause by clause and never edited; these three clauses SURVIVE, so ADR-0323 names them as kept |

### 🔴 0.8 — the fixture floor "~+800-900" is refuted: `0126` is **+795**

That is `driver.go` 673 + `README.md` 92 + `expectations.yaml` 29 + `runner_test.go` 1, BELOW the stated floor.
Dropping `0125`'s 50-way concurrency arm saved more than the extra listener cost. A floor borrowed from a
sibling's SIZE is not a floor for a different SHAPE (method note 20).

### ⚠️ 0.9 — the drive length is set by the 1.5 s holds, not by H1's 2000 ms

`SPEC.md` §7.4 says the longest hold, 2000 ms, sets each side's drive. H1's 2000 ms is a CAP. The server
closes at ~500 ms, so the drive is bounded by W1/M1's 1500 ms holds: about 1.5 s per side, 5.3-5.5 s per run
including container start. The fixture agent's driver header repeated the SPEC's claim. **The controller
corrected that comment, line-count-neutral (`2 2` within the file, still 673 lines), after the runs.** The
change is comment-only and was re-checked with `gofmt` and `go vet` (§10).

### ⚠️ 0.10 — at the un-fixed tip the reference side is NEVER DRIVEN

The runner tries the subject boot three times (`listener: "l_half": listener_filters_timeout 500ms is outside
the supported [1s, 60s] envelope`). It then fails with `runner_test.go:1222: subj start (attempt 3): subject
ready: EOF` and terminates the reference container before any drive. So "reference green on every arm at the
tip" (the phase-100 PLAN's Task 7 wording) is **not observable** for `0126`. The tip record is the subject's
boot-reject MESSAGE (method note 51: proven by the message, not by the rc). The reference evidence comes from
the shape-A runs.

### ✅ 0.11 — OWED CELL 1, MEASURED: the subject's `0.5s`/`true` fall-through serves the DEFAULT chain

Both sides returned `"DEFAULT l_half_true\n"` and `"DEFAULT l_one_true\n"` in **every** run (R1-R3, the three
measurement runs, NC1, NC4: eight runs). This matches the reference's R2. T1/T2 pin the body as a
chain-selection guard. The discriminator stays S's `l_half_true` **1** beside the mirror `l_one_true` **0**
(method note 55).

### ✅ 0.12 — OWED CELL 2, MEASURED: the reference's `0.5s` drop through the host `-p` path does NOT smear

A measurement-only variant of the driver (H1 plus 30 concurrent silent replicates on `l_half`, `wantPreCx`
31, confirmed live at 31 = 31 on both sides, never committed) was run three times:

| side | n | min | max | mean | σ | (mean−350)/σ | (900−mean)/σ |
|---|---|---|---|---|---|---|---|
| reference (docker-proxy) | 93 | 500 | 508 | 502.89 | 2.54 | 60 | 156 |
| subject (shape A) | 93 | 500 | 501 | 500.95 | 0.23 | 669 | 1769 |

The plain runs R1-R3 read reference 501 / 501 / 502 and subject 501 / 500 / 501. **`[350, 900]` is KEPT.**
Against `0125`'s worst observed reference spread (σ 13.72) the margins are still 11σ low and 29σ high, well
past the 4-5σ rule (`reference_differential_band_sigma_margin`). The reference's close through docker-proxy
was FIN in 93 of 93 (recorded, never pinned).

### ✅ 0.13 — CONFIRMED, not refuted

- Appendix A applies to `7d18bcba` with `git apply --check` and reproduces `8 9` / `1 1` / `1 1` exactly.
- `pipeline.go` needs NO comment edit: `Run`'s doc carries no width or envelope claim.
- The comment pass is `manager.go 7 6` (three sites), so A+B is `15 15` and every line after `:953` returns to
  its tip number. **`manager.go:1356` (`p.Run(…, rt.lfTimeoutMs)`) is UNMOVED.**
- NC1 leaves `PipelineHoldsAtMax` GREEN (wraps to ~24 days) and reddens `PipelineHoldsPastUint32Wrap` (704.15
  ms), as `SPEC.md` §0.5 says.
- NC6 leaves the reject arms GREEN with the right phrase (V2/V3 fire first).
- `0125` still PASSES under A (reference closes 1001-1002 ms, n=51; subject 1000-1009 ms).
- `0126` PASSES 3/3 under A, and every S cell matches `SPEC.md` §7.4 on both sides.
- No new stat name.

---

## 1. Stage scope, MEASURED

### 1.1 What THIS PLAN commit touches — FOUR files

Precedents, by `git show --numstat --format=`: phase-100 PLAN `8d6422f3` and phase-99 PLAN `a62f32a0`. Each
touches `STATE.md`, `STATE_HISTORY.md` `2 0`, the new `PLAN.md` and `next-prompt.txt`, and nothing else. (The
phase-100 PLAN correction `9ba23146` touched three of those four.) **No `DECISIONS.md`**, so the `PROPOSED`
guard stays ARMED at `## ADR-0323`. **No `ROADMAP.md`**, so `want` stays **133** and the file stays **251**
lines.

### 1.2 What the IMPL will touch

| path | measured basis | added / removed |
|---|---|---|
| `internal/listener/listenerfilter/pipeline.go` | Appendix A | **1 / 1** |
| `internal/listener/listenerfilter/pipeline_deadline_test.go` | Appendix A (the one typed caller) | **1 / 1** |
| `internal/listener/manager.go` | Appendix A (`8 9`) + Appendix B (comments, `7 6`) | **15 / 15** |
| `internal/listener/listener_filters_timeout_test.go` | Appendix C: 13 parse arms + 2 pipeline-hold arms (file 338 → 544) | **206 / 0** |
| `internal/listener/manager_test.go` | Appendix D: two pins re-pointed and renamed, plus the `:4053` comment | **19 / 19** |
| `internal/listener/quic_test.go` | Appendix E: the QUIC build arm (two subtests) | **35 / 0** |
| `test/fixtures/0126-listener-filters-timeout-envelope/**` (3 files) | Appendix F.1-F.3 (673 + 92 + 29), built and run 9× | **794 / 0** |
| `test/differential/runner_test.go` | Appendix F.4, the blank import | **1 / 0** |
| `docs/envoy-go/DECISIONS.md` | ADR-0323 §Decision + §Consequences + status flip (precedent: the phase-100 IMPL's ADR-0322, `136 1`) | ~**+100 / −1** (estimate) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | `:4359` bullet + `:4338` gloss rewritten in place, `+0` ledger entry after `:5150` | ~**+1 / 0** net lines (estimate) |
| `docs/envoy-go/ROADMAP.md` | row 101 flip | **1 / 1** |
| `PROGRESS.md`, `STATE*.md`, `next-prompt.txt` | the close | not LoC |

**Byte-untouched roster.** The IMPL asserts each of these is EMPTY under `git diff master --numstat`:

- `internal/listener/quic.go`
- `internal/listener/listenerfilter/` except `pipeline.go` and `pipeline_deadline_test.go`
- `internal/stats/**` (+0 names: `name.go` and `helptext_test.go` must not move)
- `cmd/**`
- every fixture directory except `0126` (**`0125` especially**: `SPEC.md` §7.1)
- `REVIEW_FINDINGS.md` (`:185` names the timeout, not the envelope: `SPEC.md` §5)
- `go.mod`, `go.sum`, `.github/**`

⚠️ **The byte-untouched roster and the edit roster are not a partition** (method note 62). `STATE*.md`,
`PROGRESS.md` and `next-prompt.txt` are on neither list, and are named here for that reason. Set-difference:
neither roster contains ADR-0082's, ADR-0081's, ADR-0078's or ADR-0322's text. ADR-0323 supersedes or notes
them, and they are never edited. Neither roster contains `ROADMAP.md`'s six sentinel windows, which stay
byte-identical by md5 (§2.1).

### 1.3 The split gate — EVALUATED WITH A COMMAND, NOT SPLIT

`BOOTSTRAP_PROMPT.md` §6.1 sets the thresholds at **~25 numbered tasks** or **~1500 LoC**, and a per-task
sub-step maximum of ~10.

| accounting | this row (MEASURED at this PLAN) | phase-100 PLAN, for comparison |
|---|---|---|
| production `.go` (code + comments) | **1 + 15 = 16** added | 47 |
| + unit tests | **+ 1 + 206 + 19 + 35 = 261** → **277** | 604 |
| + fixture `driver.go` + `runner_test.go` (`.go` only) | **+ 673 + 1** → **951** | 1332 |
| + fixture `README.md` / `expectations.yaml` | **+ 92 + 29** → **1072** | 1441 |
| + docs (ADR ~100, contract ~3, row 1) | **≈ 1176** | ≈ 1547 |

**DECISION: DO NOT SPLIT.** The row is under ~1500 on every accounting, and the task count is 16 (§10). The
only clean seam is {unit layer} / {fixture}. Splitting there would ship the width change gated by only one
surface: only the fixture sees the reference, and only the unit layer sees NC2, NC3 and the pipeline-hold
arms. **Every `.go` row is MEASURED; the doc rows are estimates.**

---

## 2. Sentinel and baselines — RUN AT THIS STAGE's TIP

### 2.1 The three checks, the four NCs, and the check-(2) positive control

Run at the stage worktree tip (`7d18bcba`) before any edit, with `/usr/bin/grep`, verbatim from
`next-prompt.txt`, and **re-run in the publishing tree with identical output** (a PLAN touches no
`ROADMAP.md` byte):

- (1) **ONE**: `NOT DONE: row 101`.
- (2) **SIX** at `:211 :217 :223 :233 :239 :247`.
- (3) **SILENT**.
- NC-A: the substitution was inspected first (`NC LANDED? [ in-progress ]`), then **TWO** lines (`NOT DONE:
  row 62`, `NOT DONE: row 101`).
- NC-B at `want=132`: **TWO** lines (`NOT DONE: row 101`, `GATE FAIL: examined 133 data rows, expected 132`).
- NC-C: **FIRED** (residual 0).
- NC-D: **96 / 68** under `--`.
- Check-(2) positive control: **6 substitutions ASSERTED, residual 0**.
- Escape-aware malformed set: **{57, 69}**, at `:119` (NF 9) and `:131` (NF 10).
- Row 101 NF: **8** under BOTH forms.
- Per-line md5 with the **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex):
  `211 10d7807bf02d` · `217 4a92f7e62fc6` · `223 2a7eb298b9fd` · `233 242e53c6f7a3` · `239 b2680e6f4fbf` ·
  `247 6caa1c3ce0e7`.

⇒ **The sentinel does NOT fire, and `stop` was NOT created.**

### 2.2 The `PROPOSED` guard — ARMED, and this stage leaves it ARMED

`^> \*\*STATUS: PROPOSED` hits `:19648`, and a backward `^## ADR-` search resolves it to **`## ADR-0323`**
(`:19646`). The ADR-0231 decoy (`^\*\*Status:\*\* PROPOSED`) still hits `:14866` and resolves to
`## ADR-0231` (`:14864`); that line's md5 is `929719b67c87…`, unchanged. `DECISIONS.md` reads:
- **19662** lines, `^---$` **216**, `^## ADR-` **322**, bare `^## ` **330**;
- tail `ADR-0323`, next-free **ADR-0324** (`grep -c '^## ADR-0324'` ⇒ 0).

All are UNMOVED by this stage. **The IMPL disarms the guard at Task 11.**

### 2.3 The fixture set

The blank-import extractor reads **127 = 127** at this tip. With `0126` registered it read **128 = 128** on the
prototype (**104 `driver/` + 24 `inputs/`**), with `comm -23` and `comm -13` both EMPTY under the **(imports,
dirs)** argument order. The registered name equals the directory name byte for byte.

### 2.4 Ports

`git grep -lw -- <port> -- test/ internal/ cmd/` at `7d18bcba`:

- `15126`, `15232`, `15233`, `15234`, `15235`, `15236` (and `15237`) hit **zero** files.
- `ss -tan` / `ss -uan` show no socket on any of them, in any state.

The ad-hoc band this stage reserved, `16900-16999`, was censused with `git grep -In '169[0-9][0-9]' -- test/
internal/ cmd/`. Every hit was a substring of a hex span id or a Unix timestamp; there is no port. **The band
was not used**: the fixture runs went through the harness's own allocation, and the unit arms used port 0.
⚠️ **This paragraph spells both the band and the fixture ports, so it is a hit in the next census.**

---

## 3. STABLE ANCHORS — use these, never line numbers

- **A1** `func (p *Pipeline) Run(` (pipeline.go): the signature line, `:32` at the tip.
- **A2** `func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error)` (manager.go;
  `(uint64, error)` after Task 6).
- **A3** `	lfTimeoutMs             uint32` (the `listenerRuntime` field).
- **A4** `	lfTimeoutMs, err := parseListenerFiltersTimeout(name, l.GetListenerFiltersTimeout())` (the one caller,
  inside `buildListenerRuntimeWithCtx`).
- **A5** `func TestParseListenerFiltersTimeoutBelowFloorErrors(t *testing.T) {` and
  `func TestParseListenerFiltersTimeoutAboveCapErrors(t *testing.T) {` (manager_test.go).
- **A6** `		// 1s pipeline timeout (ADR-0082 floor); slowListenerFilter blocks 2s.` (manager_test.go).
- **A7** `- Per-pipeline timeout (`Listener.listener_filters_timeout`)` and
  `ADR-0082 (listener_filters_timeout [1s,60s] envelope)` and `**Phase 100 — +1 (` in `BEHAVIOR_CONTRACT.md`.
- **A8** `*§Decision and §Consequences follow at the phase-101 IMPL.*` in `DECISIONS.md` (the retained footer).
- **A9** the row whose first field is `101` in `ROADMAP.md`.

---

## 4. Test design — MEASURED per arm, not predicted

Scored at this PLAN stage in throwaway worktrees. Each NC is applied on top of A + B + the tests. P = PASS,
F = FAIL.

### 4.1 Unit arms (`go test -count=1 -v`; NC selector `-run 'TestParseListenerFiltersTimeout|TestListenerFilterTimeoutPipelineHolds|TestQUICListenerFiltersTimeout' ./internal/listener/` ⇒ **RUN 23**)

| arm (SPEC name → test) | TIP | A | NC1 | NC2 | NC3 | NC4 | NC6 |
|---|---|---|---|---|---|---|---|
| U-wrap `…AboveUint32MsAccepted` | F | P | **F** (704) | P | P | P | **F** |
| U-neg `…NegativeSecondsRejected` | F (msg) | P | P | **F** | P | P | P |
| U-negnanos `…NegativeNanosRejected` | **F (msg)** (§0.2) | P | P | **F** | P | P | P |
| U-oor `…AboveMaxSecondsRejected` | F (msg) | P | P | P | **F** | P | P |
| U-pmax `…ProtoMaxRejected` | **F (msg)** (§0.2) | P | P | P | **F** | P | P |
| U-max `…MaxAccepted` | F | P | **F** (2077251487) | P | P | **F** (9223372036000, §0.3) | **F** |
| U-subms `…SubMillisecondTruncatesToZero` | F | P | P | P | P | **F** (1) | **F** |
| U-1ms9 `…TruncatesNotRounds` | F | P | P | P | P | **F** (2) | **F** |
| U-500ms / U-90s / U-61s `…HalfSecond` / `…NinetySeconds` / `…SixtyOneSeconds` | F | P | P | P | P | P | **F** |
| U-zero / U-nil `…ZeroParsesToZero` / `…NilParsesToDefault` | P | P | P | P | P | P | P |
| Pipe-max `TestListenerFilterTimeoutPipelineHoldsAtMax` | F (parse) | P | **P** (wraps ~24 d) | P | P | P | **F** |
| Pipe-wrap `…PipelineHoldsPastUint32Wrap` | F (parse) | P | **F** (704.15 ms) | P | P | P | **F** |
| re-pointed `…SubSecondAccepted` / `…AboveOldCapAccepted` | F | P | P | P | P | P | **F** |
| QUIC `TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds` (0.5s, 120s) | F | P | P | P | P | P | **F** |
| existing `…InRange` / `…Default` / `…ZeroDisables` | P | P | P | P | P | P | P |

**Every tip failure reason is the envelope message** (`… is outside the supported [1s, 60s] envelope`), read
from each test's own output:
- the four reject arms fail with `reject message … does not contain …`;
- the accept arms, the two pins and the QUIC arm fail with `accept: got error …` or `rejected`;
- the two pipeline arms fail with `parse rejected`, because they never reach `Run`.

**Pre-fix GREEN arms, stated per arm (method note 61):** U-zero and U-nil are regression guards, green under
every column. `…InRange`, `…Default` and `…ZeroDisables` are the same. Their falsifiability is not this row's
business: row 100's NC3 owns `…ZeroDisables`.

### 4.2 Suite counts (`go test -count=1 -v ./internal/listener/...`)

| tree | RUN | top-level FAIL | rc |
|---|---|---|---|
| bare tip | **274** | 0 | 0 |
| tip + Appendices C, D, E | **292** | **16** (below) | 1 |
| A + B + C + D + E (final) | **292** | 0 | 0 |
| final, `-race` | 292 | 0 (0 `DATA RACE`) | 0 |

292 = 274 + 15 new top-level tests + the QUIC parent and its two subtests. The two re-points are renames, not
additions. **The 16 tip-RED top-level tests, re-run first-hand by the controller:**

```
TestListenerFilterTimeoutPipelineHoldsAtMax
TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap
TestParseListenerFiltersTimeoutAboveMaxSecondsRejected
TestParseListenerFiltersTimeoutAboveOldCapAccepted
TestParseListenerFiltersTimeoutAboveUint32MsAccepted
TestParseListenerFiltersTimeoutHalfSecond
TestParseListenerFiltersTimeoutMaxAccepted
TestParseListenerFiltersTimeoutNegativeNanosRejected
TestParseListenerFiltersTimeoutNegativeSecondsRejected
TestParseListenerFiltersTimeoutNinetySeconds
TestParseListenerFiltersTimeoutProtoMaxRejected
TestParseListenerFiltersTimeoutSixtyOneSeconds
TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero
TestParseListenerFiltersTimeoutSubSecondAccepted
TestParseListenerFiltersTimeoutTruncatesNotRounds
TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds
```

That is 11 parse arms, 2 pipeline arms, 2 re-pointed pins and 1 QUIC parent (its two subtests fail under it).

**On the final tree:**
- `gofmt -l internal/listener/` prints nothing, and `go vet ./internal/listener/...` is clean.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/...` returns rc 0. A planted file fired `unused`
  and `misspell` first, so the linters run.
- `go test -count=1 ./internal/listener/... ./cmd/envoy-go/... ./internal/admin/...` returns rc 0.
- ⚠️ The phase-100 IMPL's **470** is a DIFFERENT selector (`./internal/listener/... ./internal/stats/...`). The
  IMPL measures it at Task 1 and names the selector with the number.

### 4.3 Fixture `0126`, per arm — 9 runs plus 3 measurement runs, none discarded (reference / subject)

| arm | R0 tip | A ×3 (R1-R3) | NC1 | NC4 |
|---|---|---|---|---|
| H1 `l_half` closes in `[350, 900]`, 0 bytes | not driven / **boot reject** | g/g ×3 (501-502 / 500-501 ms) | g/g | g/g |
| W1 `l_wrap` open at 1500 ms | not driven / boot reject | g/g ×3 | g / **R** (FIN at 704 ms) | g/g |
| M1 `l_subms` open at 1500 ms | not driven / boot reject | g/g ×3 | g/g | g / **R** (FIN at 1 ms) |
| T1, T2 (`DEFAULT …` body), N1 | not driven / boot reject | g/g ×3 | g/g | g/g |
| S `l_half` 1, `l_half_true` 1, others 0 | not driven / boot reject | g/g ×3, all six, every series present | g / **R** (`l_wrap` 1) | g / **R** (`l_subms` 1) |
| CompareBytes | — | equal ×3 | **R** at `W1 … open_at_1500ms=false` | **R** at `M1 … open_at_1500ms=false` |
| verdict | FAIL | PASS ×3 | FAIL | FAIL |

- **R0's failure lines:** `listener manager: listener: "l_half": listener_filters_timeout 500ms is outside
  the supported [1s, 60s] envelope` (three start attempts), then `runner_test.go:1222: subj start (attempt
  3): subject ready: EOF` (§0.10).
- **NC markers:** each was counted at 1 before its run and 0 after reverting.
- **Wall time:** ~6-7 s per `0126` run including container start (5.3-5.5 s of test time). `0125` under A: one
  PASS, 35.4 s.
- NC2, NC3 and NC6 were not run at fixture level. NC2 and NC3 have no fixture arm (no reject arm can boot).
  NC6 is the tip's boot reject again.

---

## 5. Tasks

⚠️ **Order is load-bearing.** Tasks 1-5 land every falsifier and record the un-fixed tip **before** Task 6
changes production code. Nothing after Task 6 can recreate that measurement. ⚠️ **Evaluate the context budget
BEFORE starting each task** (`checking-context-budget`). Commit after every task and keep `PROGRESS.md`
current. If the session stops mid-spine, say which task it reached and leave row 101 `in-progress` (method
note 47).

Throughout: `W=/home/esa/git/envoy-go-wt-p101impl`, `S=<your scratch dir>`,
`P=$W/docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/PLAN.md`, and `ext` as defined in
Global Constraints. Re-export them in every Bash call.

---

### Task 1: Baseline, a proven-live panic gate, and `PROGRESS.md`

**Files:**
- Create: `docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/PROGRESS.md`

**Interfaces:**
- Produces: `base-listener.txt` (the sorted `=== RUN` roster at the un-fixed tip). Tasks 5 and 14 diff against
  it. Also the phase-100 two-package count, measured, with its selector named.

- [ ] **Step 1: Create the IMPL worktree off the CURRENT master tip.**
  `git -C /home/esa/git/envoy-go worktree add -b wt-phase-101-impl $W master`. Then check `pwd` and
  `git -C $W rev-parse --abbrev-ref HEAD` ⇒ `wt-phase-101-impl`.
- [ ] **Step 2: Resolve the selectors before believing any FAIL.** A nonexistent package prints
  `[setup failed]` and exits 1. `go list ./internal/listener/... | wc -l` must be non-zero.
- [ ] **Step 3: Record the bare-tip roster.**
  ```bash
  cd $W && out=$(go test -count=1 -v ./internal/listener/... 2>&1); rc=$?
  echo "$out" | /usr/bin/grep -oE '^=== RUN   [^ ]+' | sort > $S/base-listener.txt
  echo "rc=$rc RUN=$(wc -l < $S/base-listener.txt) FAIL=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')"
  ```
  Expected: `rc=0 RUN=274 FAIL=0`. Then run `./internal/listener/... ./internal/stats/...` and RECORD its count
  with the selector named (the phase-100 IMPL read 470; measure it, do not inherit it).
- [ ] **Step 4: Prove the panic gate live.** Put a `panic("p101-gate")` in a throwaway `_test.go` in
  `internal/listener`. Run `go test -count=1 -run TestP101Gate ./internal/listener/`, and confirm
  `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` reads ≥ 1. Delete the file and confirm `git -C $W status
  --short` prints nothing.
- [ ] **Step 5: Write `PROGRESS.md`** with a header (phase, worktree, base SHA) and a Task 1 entry quoting the
  commands and outputs of Steps 2-4.
- [ ] **Step 6: Commit.**
  `git -C $W add docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/PROGRESS.md && git -C $W commit -m "phase 101 (listener-filters-timeout-envelope-lift) IMPL task 1: baseline roster and a proven-live panic gate"`.

---

### Task 2: The parse and pipeline-hold arms — 13 of 15 RED at the un-fixed tip

**Files:**
- Modify: `internal/listener/listener_filters_timeout_test.go` (Appendix C, `206 0`, 338 → 544 lines)

**Interfaces:**
- Consumes: `parseListenerFiltersTimeout` (returns `uint32` at the tip and `uint64` after Task 6; the helper
  `lftParse` converts with `uint64(v)` so the file compiles under BOTH, per §0.1),
  `listenerfilter.NewPeekerConn`, `listenerfilter.AsPeeker`, `listenerfilter.Pipeline.Run`.
- Produces: 15 top-level tests, 13 `TestParseListenerFiltersTimeout…` and 2
  `TestListenerFilterTimeoutPipelineHolds…`, with helpers `lftParse`, `lftAccept`, `lftReject`,
  `lftLoopbackPair`, `lftPipeHold` (none collides with an existing package-`listener` identifier: checked with
  `git grep`).

- [ ] **Step 1: Apply Appendix C.** `ext C $P > $S/C.diff; git -C $W apply --check $S/C.diff && git -C $W apply $S/C.diff`.
  `git -C $W diff --numstat` ⇒ `206	0	internal/listener/listener_filters_timeout_test.go`; `wc -l` ⇒ 544.
- [ ] **Step 2: Confirm it compiles and the selector matches.** `go vet ./internal/listener/` rc 0.
  `go test -count=1 -v -run 'TestParseListenerFiltersTimeout(AboveUint32|Negative|AboveMax|ProtoMax|MaxAccepted|SubMillisecond|TruncatesNot|HalfSecond|Ninety|SixtyOne|ZeroParses|NilParses)|TestListenerFilterTimeoutPipelineHolds' ./internal/listener/ 2>&1 | /usr/bin/grep -c '^=== RUN'` ⇒ **15**.
- [ ] **Step 3: Run it at the tip.** Expect **13 RED, each for its NAMED reason** (§4.1):
  - the four reject arms on `reject message … does not contain …`;
  - the seven accept arms on `accept: got error … outside the supported [1s, 60s] envelope`;
  - the two pipeline arms on `parse rejected`.
  Expect `…ZeroParsesToZero` and `…NilParsesToDefault` GREEN. Record every failure line in `PROGRESS.md`.
  **A RED for any other reason is a finding. Stop and diagnose.**
- [ ] **Step 4: `gofmt -l internal/listener/` prints nothing.**
- [ ] **Step 5: Commit** (pathspec: the test file + `PROGRESS.md`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 2: 15 parse/pipeline arms, 13 RED at the un-fixed tip`.

---

### Task 3: The two envelope pins re-pointed and renamed, the `:4053` comment, and the QUIC build arm

**Files:**
- Modify: `internal/listener/manager_test.go` at anchors **A5** and **A6** (Appendix D, `19 19`)
- Modify: `internal/listener/quic_test.go` (Appendix E, `35 0`)

**Interfaces:**
- Consumes: `mkQUICListener`, `NewManager` (the `quic_test.go` pattern); `mgr.runtimes[0].lfTimeoutMs`,
  compared as `uint64(…)` so it compiles at the tip (§0.1).
- Produces: `TestParseListenerFiltersTimeoutSubSecondAccepted` (500) and
  `TestParseListenerFiltersTimeoutAboveOldCapAccepted` (90000), replacing `…BelowFloorErrors` and
  `…AboveCapErrors`; and `TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds` with subtests `0.5s` and
  `120s` (asserting build success, `kind == kindQUIC`, and the stored value).

- [ ] **Step 1: READ the two pins' inputs before applying** (method note 93):
  `durationpb.New(500*time.Millisecond)` and `durationpb.New(90*time.Second)`, both VALID Durations that the
  reference accepts.
- [ ] **Step 2: Apply Appendices D and E** (`ext D`, `ext E`, each `git apply --check` first).
  `git -C $W diff --numstat` ⇒ `19	19	…manager_test.go` and `35	0	…quic_test.go`.
- [ ] **Step 3: Assert the old names are GONE and the new ones present.** `git -C $W grep -c 'BelowFloorErrors\|AboveCapErrors' -- internal/listener/` reads nothing (capture it; `grep -c` on zero exits 1).
  `git -C $W grep -c 'ADR-0082 floor' -- internal/listener/manager_test.go` likewise reads 0.
- [ ] **Step 4: Run them at the tip.** `go test -count=1 -v -run 'SubSecondAccepted|AboveOldCapAccepted|TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds' ./internal/listener/`.
  Expect RUN **5** (two pins, the QUIC parent and its two subtests) and **all RED on the envelope message**.
  Record them.
- [ ] **Step 5: `gofmt -l` prints nothing and `go vet ./internal/listener/` returns rc 0.**
- [ ] **Step 6: Commit** (pathspec: both test files + `PROGRESS.md`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 3: envelope pins re-pointed and renamed, QUIC build arm, all RED at the tip`.

---

### Task 4: Fixture `0126` — driver, README, expectations, and the FOUR registration gates

**Files:**
- Create: `test/fixtures/0126-listener-filters-timeout-envelope/driver/driver.go` (Appendix F.1, 673 lines)
- Create: `test/fixtures/0126-listener-filters-timeout-envelope/README.md` (Appendix F.2, 92 lines)
- Create: `test/fixtures/0126-listener-filters-timeout-envelope/expectations.yaml` (Appendix F.3, 29 lines)
- Modify: `test/differential/runner_test.go` (Appendix F.4, the blank import, `1 0`)

**Interfaces:**
- Consumes: `fixture.RegisterFixture`, `fixture.MultiListenerDriver`, `fixture.StatsAsserter` (the `0125`
  pattern).
- Produces: fixture name `0126-listener-filters-timeout-envelope`, byte-identical to the directory. Its six
  listeners are `l_half`, `l_half_true`, `l_one_true`, `l_wrap`, `l_subms` and `l_nofilt_120`, on reference
  ports `15126 15232 15233 15234 15235 15236`.

- [ ] **Step 1: Re-census the six ports at the IMPL tip.** For each, run
  `git -C $W grep -lw -- <port> -- test/ internal/ cmd/` and `ss -tan` / `ss -uan` in all states. Expected:
  zero files and no sockets. (Prose in `docs/` spells them too; that is not a bind.)
- [ ] **Step 2: Write the three files** from Appendices F.1-F.3 (`ext F.1 $P > …/driver/driver.go`, and so
  on). `wc -l` must read 673 / 92 / 29.
- [ ] **Step 3: Add the blank import** (Appendix F.4, after the `0125` line).
  `git -C $W diff --numstat -- test/differential/runner_test.go` ⇒ `1	0`.
- [ ] **Step 4: Compile and vet.** `go vet ./test/fixtures/0126-listener-filters-timeout-envelope/... ./test/differential/`
  returns rc 0, and `gofmt -l test/fixtures/0126-listener-filters-timeout-envelope/` prints nothing.
- [ ] **Step 5: Check the four gates by the SET, never by the exit code.** Gate 1 is `RegisterFixture` in
  `init()`, gate 2 the import, gate 3 name == directory byte for byte, gate 4 the `NNNN-` shape. Run the
  extractor with arguments in the order **(imports, dirs)**:
  ```bash
  extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
  extract $W/test/differential/runner_test.go > $S/imports.txt
  ls -d $W/test/fixtures/*/ | xargs -n1 basename | sort > $S/dirs.txt
  wc -l < $S/imports.txt; wc -l < $S/dirs.txt; comm -23 $S/imports.txt $S/dirs.txt; comm -13 $S/imports.txt $S/dirs.txt
  ```
  Expected: **128**, **128**, and both `comm` outputs empty. `/usr/bin/grep -c '^0126-listener-filters-timeout-envelope$'`
  must read 1 in each file.
- [ ] **Step 6: NC the extractor** in a SCRATCH COPY. Renaming the import's directory token must fire BOTH
  `comm` directions. Deleting the import line fires `comm -13` only, under the (imports, dirs) order.
- [ ] **Step 7: Commit** (pathspec: the three fixture files + `runner_test.go` + `PROGRESS.md`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 4: fixture 0126 and its four registration gates`.

---

### Task 5: RECORD the un-fixed tip — every falsifier RED, for its NAMED reason

**Files:**
- Modify: `PROGRESS.md` only

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: the pre-fix record that Task 8 compares against. **It cannot be recreated after Task 6.**

- [ ] **Step 1: Unit suite at the tip:** `go test -count=1 -v ./internal/listener/...`. Expected **RUN 292,
  rc 1, exactly the 16 top-level FAILs of §4.2**, each on the envelope message. Diff the sorted `=== RUN`
  roster against Task 1's. The additions must be exactly the 15 new names, the QUIC parent and its two
  subtests, and the two renamed pins. The only removals may be the two old pin names.
- [ ] **Step 2: Fixture `0126` at the tip, ALONE:**
  `go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0126-listener-filters-timeout-envelope$' -timeout 30m`.
  First confirm the `=== RUN   TestDifferential/0126-listener-filters-timeout-envelope` line and **zero `SKIP`**
  lines. Expected: FAIL at subject BOOT with `listener_filters_timeout 500ms is outside the supported [1s,
  60s] envelope`. **The reference is never driven** (§0.10). Do not record "reference green".
- [ ] **Step 3: Fixture `0125` at the tip, ALONE**, by the same command with its own name. Expected: PASS. It is
  byte-untouched and must stay green through the row.
- [ ] **Step 4: Paste the failure lines into `PROGRESS.md` and commit:**
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 5: the un-fixed tip recorded — 16 unit tests RED, 0126 RED at subject boot, 0125 green`.

---

### Task 6: THE PRODUCTION EDIT — shape A, code hunks only

**Files:**
- Modify: `internal/listener/manager.go` (Appendix A, `8 9`)
- Modify: `internal/listener/listenerfilter/pipeline.go` (Appendix A, `1 1`)
- Modify: `internal/listener/listenerfilter/pipeline_deadline_test.go` (Appendix A, `1 1`)

**Interfaces:**
- Produces:
  - `func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, error)`: V1 nil → 15000;
    V2 `s < 0 || n < 0` → `expected a positive duration`; V3 `s > 9223372035` → `duration out of range`;
    V4 `uint64(s)*1000 + uint64(n)/1e6`;
  - `listenerRuntime.lfTimeoutMs uint64`;
  - `func (p *Pipeline) Run(…, timeoutMs uint64) (retErr error)`;
  - `runPeekPipeline(server net.Conn, timeoutMs uint64)`.

- [ ] **Step 1: Extract and apply Appendix A** (`ext A $P > $S/A.diff`, `git -C $W apply --check`, then
  apply). `git -C $W diff --numstat HEAD` ⇒ `8 9 manager.go`, `1 1 pipeline.go`, `1 1 pipeline_deadline_test.go`.
- [ ] **Step 2: Assert the symbols landed, not only that the build passed** (method note 7). Each must read
  ≥ 1, scoped by pathspec:
  - `git -C $W grep -c 'timeoutMs uint64' -- internal/listener/listenerfilter/pipeline.go`
  - `git -C $W grep -c 'lfTimeoutMs             uint64' -- internal/listener/manager.go`
  - `git -C $W grep -c 's > 9223372035' -- internal/listener/manager.go`
  - `git -C $W grep -c 'expected a positive duration' -- internal/listener/manager.go`

  And this must read **0**: `git -C $W grep -c 'AsDuration' -- internal/listener/manager.go` (method note 100).
- [ ] **Step 3: Run the layout gate** (§6: `ext G $P > $S/layout-gate.sh; bash $S/layout-gate.sh $W master`).
  Expected: **0 failed sub-gates**.
- [ ] **Step 4: `gofmt -l internal/listener/` prints nothing; `go vet ./internal/listener/...` rc 0.**
- [ ] **Step 5: Commit** (pathspec: the three files):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 6: listener_filters_timeout is a uint64 of ms from the Duration fields; the [1s, 60s] envelope is gone`.

---

### Task 7: The comment reconciliation in CODE — comment-only, under TWO gates

**Files:**
- Modify: `internal/listener/manager.go` (Appendix B, `7 6`: the field doc at A3, the build-site comment at
  A4, the function doc at A2)

- [ ] **Step 1: Extract and apply Appendix B** on top of Task 6 (`ext B`, `--check`, apply).
  `git -C $W diff --numstat HEAD` ⇒ `7	6	internal/listener/manager.go`.
- [ ] **Step 2: Gate the KIND.** Every changed line must be a comment:
  `git -C $W diff -U0 HEAD | /usr/bin/grep -E '^[+-]' | /usr/bin/grep -vE '^(\+\+\+|---) ' | sed -E 's/^[+-][[:space:]]*//' | /usr/bin/grep -vc '^//'`
  must read **0**. ⚠️ `grep -c` on zero matches prints `0` and EXITS 1: capture the output, never chain on it.
- [ ] **Step 3: Gate the SHAPE.** `bash $S/layout-gate.sh $W master` must still read **0 failed sub-gates**. Then
  `git -C $W grep -n 'p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs)' -- internal/listener/manager.go`
  must print line **1356**, UNMOVED, because A+B is `15 15`.
- [ ] **Step 4: Show the KIND gate fires.** In a scratch copy, plant a code change inside one hunk (for
  example `const defaultMs = 15000` → `15001`). The Step 2 command must read ≥ 1. Revert it.
- [ ] **Step 5: Commit** (pathspec: `manager.go`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 7: comment reconciliation, comment-only and layout-gated`.

---

### Task 8: ALL ARMS GREEN — unit, race, fixture ×3, `0125` ×1

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Unit suite.** `go test -count=1 -v ./internal/listener/...` must read **RUN 292, 0 FAIL, rc 0**.
  The roster must equal Task 5's, with only the outcome changed.
- [ ] **Step 2: `go test -count=1 -race ./internal/listener/...` returns rc 0** with 0 `DATA RACE`. This is the
  full package, not the differential: the differential's subject is an unraced subprocess.
- [ ] **Step 3: Lint, with a live control.** Plant a COMPILING violation (for example `behaviour` in a test
  comment). `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/...` must fire `misspell`, rc 1.
  Remove the plant, and it must then return rc 0.
- [ ] **Step 4: Fixture `0126` ALONE ×3.** Every run must PASS, with the `=== RUN` line present and 0 `SKIP`.
  Record each side's H1 close (ms) per run, and T1/T2 bodies. Expected: reference ~501-508 ms, subject
  ~500-501 ms, bodies `DEFAULT l_half_true` / `DEFAULT l_one_true` on both sides. **If any H1 close falls
  outside `[350, 900]`, STOP.** Re-derive the window from the pooled spreads with the σ-margin method (§0.12),
  and never drop the arm.
- [ ] **Step 5: Fixture `0125` ALONE ×1** under the final tree must PASS.
- [ ] **Step 6: Commit** `PROGRESS.md`:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 8: all arms green — unit 292/0, -race clean, 0126 3/3, 0125 green`.

---

### Task 9: The NC roster — five mutants, each proven able to fire, scored PER ARM

**Files:**
- Modify: `PROGRESS.md` only. **Every mutant is applied in a THROWAWAY worktree and removed.**

**Interfaces:**
- Consumes: Appendix H's five patches (`ext H.1` … `ext H.5`), each relative to A+B. They apply with `patch
  -p1` on the Task 7 tree.

- [ ] **Step 1: Create a throwaway detached worktree** at the Task 7 commit
  (`git -C /home/esa/git/envoy-go worktree add --detach <path> <sha>`).
- [ ] **Step 2: Write each NC's mechanism BEFORE running it** into `PROGRESS.md`, as §7's "mechanism" column
  does. A row that cannot name one is vacuous (method note 7d). **NC5 is recorded VACUOUS and not run.**
- [ ] **Step 3: For each of NC1, NC2, NC3, NC4 and NC6:**
  - apply the patch, and ASSERT ITS MARKER LINE IS PRESENT: `/usr/bin/grep -c 'NC<n>'` on `manager.go` must
    read ≥ 1 **before** reading any result (method note 96);
  - run the §4.1 selector with `-v`, and expect RUN **23**;
  - record every arm of §4.1's column;
  - reverse the patch, and confirm the marker count is back to 0.
- [ ] **Step 4: Fixture NCs, serialized on Docker, each ALONE:** NC1 (W1 subject FIN at ~704 ms, S `l_wrap`
  1, CompareBytes) and NC4 (M1 subject FIN at ~1 ms, S `l_subms` 1, CompareBytes). Score per row against
  §4.3.
- [ ] **Step 5: Compare every cell with §4.1 / §4.3.** A cell that differs is a finding. Record it, and NEVER
  re-run until it matches.
- [ ] **Step 6: Remove the throwaway worktree**, and confirm with `git worktree list`.
- [ ] **Step 7: Commit** `PROGRESS.md`:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 9: NC roster scored per arm (NC5 vacuous, NC4 also reddens MaxAccepted)`.

---

### Task 10: The occurrence set, re-derived at the IMPL tip — GROWTH only

**Files:**
- Modify: `PROGRESS.md` only (an edit any new hit forces belongs to the task owning that file)

- [ ] **Step 1: Run BOTH matchers, case-insensitively, and UNION them** over `internal/ cmd/
  docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md`:
  `git -C $W grep -niE '60000|\[1s, ?60s\]|1s lower bound|envelope|uint32'` and
  `git -C $W grep -niE 'listener_filters_timeout|lfTimeoutMs|timeoutMs'`. Also run
  `git -C $W grep -niE 'ADR-0082 (floor|envelope)|ADR-0082\)' -- internal/` (the `:4053` class, §0.4).
- [ ] **Step 2: INHERIT `SPEC.md` §5's table plus §0.4 and §0.7 of this PLAN.** Use the matchers only to look
  for GROWTH (method note 77). Resolve each new markdown hit to its ADR by backward `^## ADR-` search, and
  mark it P or H.
- [ ] **Step 3: Assert no present-tense code or comment still claims the envelope or a `uint32` width** under
  `internal/listener/` non-test. The new tests' narration ("the old `[1s, 60s]` envelope", "a uint32 carrier
  wraps") is historical; name it as such.
- [ ] **Step 4: Commit** `PROGRESS.md`:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 10: occurrence set re-derived, growth adjudicated`.

---

### Task 11: `ADR-0323` completed IN PLACE — §Decision + §Consequences, ACCEPTED

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, after anchor **A8** (the retained italic footer)

- [ ] **Step 1: Append `### Decision (phase-101 IMPL)`** AFTER the RETAINED footer. Add no `**Status:**`
  line, no renumber and no `---`. It covers:
  - (a) the rule as built (V1-V5 of `SPEC.md` §2), read from the fields, never `AsDuration`;
  - (b) the width, `uint64` end to end (parse, field, `Run`), and why the seven untyped callers keep their
    meaning; B and C rejected on mechanism (`SPEC.md` §0.1, §0.6);
  - (c) `pipeline.go:43` and `:33-37` unmoved by the layout gate, with `:32` widened only;
  - (d) the SUPERSESSIONS of ADR-0082: the heading clause `in [1s, 60s]`, §Decision ¶1's envelope and
    message, §Consequences (a)'s message (the `listener: %q:` prefix SURVIVES), (b)'s `uint32` (`uint64` now;
    milliseconds and `0 = no-op` SURVIVE) and (c) in full. Also what ADR-0082 KEEPS: `:3048`, `:3054`,
    `:3056` (the pipeline, the `continue…` semantics and the shared per-pipeline budget, §0.7);
  - (e) the NOTES on ADR-0081 (c), whose latency claim survives without its yardstick, and on ADR-0078
    `:3221`'s historical field list; the SUPERSESSION of ADR-0322 (g)'s "envoy-go still rejects them".
- [ ] **Step 2: Append `### Consequences (phase-101 IMPL)`.** It covers:
  - QUIC: REJECTED → ACCEPTED-AND-IGNORED, the reference's measured behaviour;
  - the two arms that moved from the SPEC's prediction (§0.2: 13 RED, not 11; §0.3: NC4 reddens
    `MaxAccepted`);
  - what was NOT bought (`SPEC.md` §1);
  - the NC roster outcome (NC5 vacuous);
  - `stats_flush_interval` as the banked next candidate.
- [ ] **Step 3: Flip the status blockquote** from `PROPOSED` to `ACCEPTED`, in the house form. The phase-100
  IMPL's ADR-0322 (`:19491`) is the precedent: read it and copy the SHAPE.
- [ ] **Step 4: The guard is now DISARMED; prove it by LINE and by ADR.** `^> \*\*STATUS: PROPOSED` must
  read zero hits, and READ the ADR-0323 status line itself, because an empty result also means a broken
  matcher. The ADR-0231 decoy must still hit `:14866` → `## ADR-0231`, md5 `929719b67c87…`. `^---$` must read
  216, `^## ADR-` 322 and bare `^## ` 330 (no new heading). Next-free is ADR-0324, TAIL-derived. Quote the new
  `wc -l`.
- [ ] **Step 5: Commit** (pathspec: `DECISIONS.md`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 11: ADR-0323 §Decision + §Consequences, ACCEPTED`.

---

### Task 12: `BEHAVIOR_CONTRACT.md` — the bullet, the `:4338` gloss, and the `+0` ledger entry

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` at anchors **A7**

- [ ] **Step 1: Rewrite the `Per-pipeline timeout` bullet IN PLACE (one line)**, anchored on the LITERAL. Its
  new text, verbatim:
  ```
  - Per-pipeline timeout (`Listener.listener_filters_timeout`): ENFORCED by one clock inside `Pipeline.Run` (phase 100, ADR-0322) — a `context.AfterFunc` on the pipeline's own context cuts a blocked peek, so the deadline fires whether or not the client acts, and the read deadline is cleared before `Run` returns; an ABSENT field means the 15s default, and an explicit `0s` DISABLES the timeout; since phase 101 (ADR-0323) every value the reference accepts is accepted, carried as a `uint64` of whole milliseconds computed from the Duration's fields and TRUNCATED (so a sub-millisecond value also disables it), and only a negative seconds or nanos field (`expected a positive duration`) or seconds above `9223372035` (`duration out of range`) is rejected, on every listener, TCP or QUIC (a QUIC listener accepts and ignores the field, as the reference does); `continue_on_listener_filters_timeout: false` closes the connection AT the deadline, and `true` falls through to chain selection (stamped `raw_buffer`) AT the deadline; each timeout books `listener.<addr>.downstream_pre_cx_timeout` under both values, while a shutdown cancel of the manager context books nothing.
  ```
- [ ] **Step 2: Rewrite the `:4338` gloss in place.** Replace the literal
  `ADR-0082 (listener_filters_timeout [1s,60s] envelope)` with
  `ADR-0082 (listener_filters_timeout; its [1s,60s] envelope superseded by ADR-0323)`. Assert with `git -C $W
  grep -c` that the old literal reads 0 and the new one 1.
- [ ] **Step 3: Append the ledger entry** as its own paragraph directly after the `**Phase 100 — +1 (` entry.
  Its text, verbatim:
  ```
  **Phase 101 — +0, UNCHANGED (no new stat NAME; the row changes which `listener_filters_timeout` values boot, not which counters exist) — delta only:** phase 101 (`listener-filters-timeout-envelope-lift`) adds no stat name. `listener.<normalized-addr>.downstream_pre_cx_timeout` (phase 100) is unchanged. Its registration on a QUIC listener, recorded as NOT measured on the reference in the phase-100 entry above, was MEASURED at the phase-101 SPEC: the reference registers it on a QUIC listener at value 0 under every accepted value, as envoy-go does (ADR-0323). Like the phase-96 through phase-100 entries, this one quotes NO absolute, because three mutually inconsistent absolutes are live at one tip.
  ```
- [ ] **Step 4: Check the shape.** `/usr/bin/grep -nE '→|->'` over the new entry must find no `A → B` absolute.
  Expect `wc -l` **6000 → 6002**: +1 entry line plus its blank separator, with the bullet and gloss rewritten
  in place. MEASURE it, and quote the measured figure.
- [ ] **Step 5: Commit** (pathspec: the contract):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 12: contract — the envelope is gone, a uint64 of truncated ms; ledger +0 (delta only)`.

---

### Task 13: `ROADMAP.md` — row 101 → `done`, under the FIELD-COUNT gate

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md`, row **A9** only

- [ ] **Step 1: Write the new cell in a SCRATCH copy first.** It says `done`, names ADR-0323 `ACCEPTED`,
  shape A's numstat, fixture `0126` (+794 plus the import), the 16 tip-RED tests, and the NC outcome. It
  mentions **no Go `||`** and no other pipe; reword any pipe AWAY rather than escaping it.
- [ ] **Step 2: Count its fields under BOTH forms.** Naive: `awk -F'|' '/^\| *101 /{print NF}'`.
  Escape-aware: `sed 's/\\|//g' F | awk -F'|' '/^\| *101 /{print NF}'`, with **no file argument to awk**.
  Both must read **8**.
- [ ] **Step 3: Assert the cell spells NEITHER sentinel match phrase** (`deferred candidates:` or
  `remaining deferred (not-yet-chartered) candidates:`). Also check the bare word `deferred`
  case-insensitively, and `-family row` (with `--`). All must read 0 in the cell.
- [ ] **Step 4: Install it and re-run the full sentinel** (§2.1's commands). Expected, measured on both sides:
  - check (1) goes ONE → **SILENT**, and NC-A and NC-B go TWO → **ONE**;
  - `want` stays **133** and `ROADMAP.md` stays **251**;
  - the six windows keep their md5s;
  - the malformed set stays {57, 69}, and NC-D stays 96 / 68.
- [ ] **Step 5: Commit** (pathspec: `ROADMAP.md`):
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 13: row 101 done`.

---

### Task 14: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Byte-untouched roster (§1.2).** `git -C $W diff master --numstat -- <each path>` must be EMPTY
  for every path. The edit roster's paths must be exactly §1.2's; list any path that is on neither.
- [ ] **Step 2: ARM roster (method note 28).** Diff the sorted `=== RUN` roster against Task 1's. It must gain
  exactly the 15 new names, the QUIC parent and its two subtests, and the two renamed pins, and it must lose
  only the two old pin names.
- [ ] **Step 3: Gate (a), the full differential**, with `-count=1`:
  - assert the fixture set BY NAME in BOTH `comm` directions (**128 = 128**), and count PASS/FAIL/SKIP;
  - never trust the exit code alone;
  - **a port-race abort is MASKING**: record it and rerun, but a green rerun clears nothing.
- [ ] **Step 4: Gate (b), the non-Docker sweep:**
  `go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'`, gated on `PIPESTATUS[0]`
  plus a SET RECONCILIATION of `ok` / `FAIL` / `[no test files]` against the package count. The count rises by
  `0126/driver`, one package (242 → 243 expected); measure it rather than predict it. Keep every sweep on the
  record.
- [ ] **Step 5: Gate (c)**, h2spec, run verbose. Expected: `95 tests, 94 passed, 1 skipped, 0 failed`.
- [ ] **Step 6: Gate (d)**, the fuzzers: `git grep -c '^func Fuzz' -- '*.go'`. The row adds none, so expect 56
  targets across 48 FILES. Name both units.
- [ ] **Step 7: Gate (e)**, the anchored panic gate: **0**, proven live at Task 1. Then run
  `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` (rc 0), with a COMPILING planted control that fires
  `errcheck`, `revive` and `ineffassign`, and remove the control.
- [ ] **Step 8: Gate (f): no `REVIEW.md`.** That is the ONE standing departure (none of 93-100 has one). Name it;
  do not claim compliance.
- [ ] **Step 9: Commit** `PROGRESS.md`:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 14: roster, arm roster and six-gate sweep recorded`.

---

### Task 15: `PROGRESS.md` close-out and the router/state roll

**Files:**
- Modify: `PROGRESS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`

- [ ] **Step 1: Close `PROGRESS.md`:** a per-task summary, and every refutation this IMPL made (method note
  2). **Re-derive every figure that a governing document quotes, at this LAST commit** (method note 97).
- [ ] **Step 2: Roll `STATE.md` IN PLACE:** the §Current pointer, lifecycle-state → DONE, next-free ADR-0324.
  Evict the oldest §Recent entry:
  - **MEASURE the pre-roll date histogram, INCLUDING the entry this close promotes;**
  - verify the eviction with the LABEL-BOUND PAIR on BOTH files;
  - add a fabricated-label NC and a positive control on an ARCHIVED label.
- [ ] **Step 3: Archive the evictee as ONE inline parenthetical line.** Expected: strict guard DELTA 0 and raw
  delta **+2**, naming the LABEL and no figure.
- [ ] **Step 4: Roll `next-prompt.txt`.** The sentinel re-evaluates. With row 101 `done`, check (1) is SILENT
  and check (2) still SIX, so the loop SELF-PICKS. `stats_flush_interval` (banked, both sides measured) is the
  first candidate.
  - Grep the result for `YOUR STAGE`, for `IMPL`, and for every figure this row moved: `want`, the fixture set
    128, gate (b)'s package count, the guard polarity (DISARMED) and the ledger chain (method note 68).
  - Stage it with `git add -f next-prompt.txt`.
- [ ] **Step 5: Commit** the four files:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL task 15: close-out, state and router rolled`.

---

### Task 16: Squash, merge, push, and remove the worktree

- [ ] **Step 1: Squash the branch to ONE commit.** Its subject carries the FULL SLUG and the stage word:
  `phase 101 (listener-filters-timeout-envelope-lift) IMPL: …`.
- [ ] **Step 2: Merge to master fast-forward.** Re-run the sentinel in the MERGED tree; it must match Task 13.
- [ ] **Step 3: Push** (`feedback_push_to_origin`), using the repo's configured email.
- [ ] **Step 4: `git worktree remove` the IMPL worktree.** `git worktree list` must then read master only.
  Check the repo root with `git status --short --untracked-files=all`, and ignore the pre-existing `.claude/`.

---

## 6. The layout gate — BUILT, AND SHOWN TO FIRE

`SPEC.md` §4's layout constraint is mechanized as `layout-gate.sh <worktree> [base]` (Appendix G). It is
adapted from `100/PLAN.md` Appendix G, with the phase-100-specific `tls_inspector` sub-gate dropped and a `:32`
widening sub-gate added. It prints one PASS/FAIL line per sub-gate, and its exit status is the number of
failed sub-gates.

| sub-gate | asserts |
|---|---|
| (a) | `pipeline.go:33-37` md5-identical to base |
| (b) | `:43` byte-identical (the cited `context.WithTimeout` line) |
| (c) | `:32` is the base line, or the base line with only `uint32` → `uint64` |
| (d) | the import block is unchanged |
| (e) | exactly one CODE `context.WithTimeout` under `internal/listener/` non-test, at `pipeline.go:43` (`//` lines dropped) |
| (f) | no line-count-changing `pipeline.go` hunk whose first old line is at or above 43 (an insertion AFTER `:43` is legal) |

Measured at this PLAN:

| input | a | b | c | d | e | f | exit |
|---|---|---|---|---|---|---|---|
| **FINAL** (A+B) | P | P | P | P | P | P | **0** |
| TIP | P | P | P | P | P | P | 0 |
| `// planted` appended to `:35` | **F** | P | P | P | P | P | 1 |
| `// planted` appended to `:43` | P | **F** | P | P | P | P | 1 |
| `// planted` appended to `:32` | P | P | **F** | P | P | P | 1 |
| `:32` widened to `int64` instead | P | P | **F** | P | P | P | 1 |
| `"time"` → `"os"` | P | P | P | **F** | P | P | 1 |
| a second CODE `WithTimeout` in `manager.go` | P | P | P | P | **F** | P | 1 |
| +1 line after `:9`, −1 at `:20` (net 0) | P | P | P | P | P | **F** | 1 |
| `// planted` inserted after `:8` | F | F | F | P | F | F | 5 |
| a line inserted after `:43` (legal) | P | P | P | P | P | P | 0 |

**Every sub-gate fires on its own planted input, alone.** The controller re-ran FINAL first-hand (0 failed).
The `int64` plant shows that (c) is not a mere "line changed" check: it admits exactly the one widening.

---

## 7. The negative-control roster — CORRECTED against `SPEC.md` §9

| NC | mutation (compiles; Appendix H) | mechanism to a failure | must redden (MEASURED) | stays green (MEASURED) |
|---|---|---|---|---|
| NC1 | the return narrowed through `uint32` | 4294968000 and 9223372035999 truncate mod 2³² | `…AboveUint32MsAccepted` (704), `…MaxAccepted`, `…PipelineHoldsPastUint32Wrap` (704.15 ms); fixture **W1, S `l_wrap` 1, CompareBytes** | `…PipelineHoldsAtMax` (wraps to ~24 d), every reject arm, H1, M1 |
| NC2 | V2 neutralised (`&& false`) | a negative wraps to a huge `uint64` and is accepted | `…NegativeSecondsRejected`, `…NegativeNanosRejected` | every other arm |
| NC3 | V3 neutralised (`&& false`) | `9223372036s` / `315576000000s` accepted | `…AboveMaxSecondsRejected`, `…ProtoMaxRejected` | `…MaxAccepted`, every other arm |
| NC4 | round instead of truncate (`+ 500000`) | `0.0005s` → 1, `0.0019s` → 2, **and the max → 9223372036000** | `…SubMillisecondTruncatesToZero`, `…TruncatesNotRounds`, **`…MaxAccepted` (§0.3, NEW vs the SPEC)**; fixture **M1, S `l_subms` 1, CompareBytes** | `…PipelineHoldsAtMax` (9.223372036e18 ns < `int64` max), H1, W1, U-500ms, U-61s |
| NC5 | compute from `AsDuration()` | saturation only above ~292 years, which V3 already rejects | **NONE — VACUOUS by construction. Recorded, NOT run** (`reference_nc_roster_rows_can_be_structurally_vacuous`) | all |
| NC6 | the old envelope re-inserted for any non-exactly-zero value, after V2/V3 | the old `[1000, 60000]` check | every accept arm outside it (7), both pipeline holds, both re-pointed pins, the QUIC arm; at fixture level the tip's boot reject | the four reject arms (V2/V3 fire first, with the right phrase), U-zero, U-nil, `…InRange`, `…Default`, `…ZeroDisables` |

**Scored per arm, never per run** (method note 61).

---

## 8. Counts — this PLAN's own tip

- **Moved by this stage:**
  - `PLAN.md` is new (its line count is re-derived by `wc -l` in the publishing commit and quoted in
    `STATE.md`);
  - `STATE.md` rolled IN PLACE;
  - `STATE_HISTORY.md` **598 → 600** (`2 0`);
  - `next-prompt.txt` rolled.
- **NOT moved:**
  - `ROADMAP.md` **251** / **133** rows, row 101 `in-progress` at `:163`;
  - `DECISIONS.md` **19662**, `^---$` 216, `^## ADR-` 322, bare `^## ` 330, tail ADR-0323 (`PROPOSED`),
    next-free ADR-0324;
  - `BEHAVIOR_CONTRACT.md` **6000**;
  - every `.go` file;
  - fixtures **127 = 127**;
  - phase dirs **142**.
- **None of the six gates was run — a PLAN's scope, not an omission.** Building and running the appendices in
  throwaway worktrees is MEASUREMENT. The gate figures are carried, labelled, as the phase-100 IMPL's:
  differential 127/0/0, gate (b) 242 packages, h2spec `95 tests, 94 passed, 1 skipped, 0 failed`, fuzzers 56
  targets / 48 files, anchored panic gate 0, lint rc 0. The one standing departure is no `REVIEW.md` (none of
  93-100).

---

## 9. Deferred — carried, none chartered

- **Carried unchanged from `SPEC.md` §1 / §13:**
  - `stats_flush_interval` (the banked next candidate, both sides measured);
  - the close KIND under `false`;
  - `downstream_cx_total` post-filter accounting;
  - the `downstream_listener_filter_{remote_close,error}` names;
  - the `0s` shutdown hold (ADR-0322 (a));
  - the subject's 35-60 ms lateness at 61 s;
  - a TCP listener filter on a QUIC listener (subject UNMEASURED).
- **NEW at this PLAN — the reference's `0.5s` close through docker-proxy is a FIN, 93 of 93** (§0.12).
  Recorded with the close-KIND item; never pinned.
- **NEW at this PLAN — `TestBuildListenerRuntime_QUIC*` in `manager_test.go` versus the QUIC arms in
  `quic_test.go`** (§0.5): two homes for QUIC build tests. A test-organisation note, not a row.

---

## 10. `SPEC.md` §14 coverage, and self-review

| # | `SPEC.md` §14 owed | discharged at |
|---|---|---|
| 1 | build and run every embedded block: Appendix A at this tip, the §6 unit arms (shim dropped, pins renamed), §6.3's QUIC arm, the `0126` driver | §4 (every cell measured); Appendices A-H are the built artifacts. §0.1 shows the shim could not simply be dropped, §0.2 that 13 arms are RED, §0.5 where the QUIC arm lives |
| 2 | measure the two unmeasured `0126` cells before writing expectations | §0.11 (T1/T2 `DEFAULT` on both sides, 8 runs); §0.12 (reference 500-508 ms, σ 2.54, n=93; `[350, 900]` KEPT with its σ arithmetic) |
| 3 | re-census `0126`'s ports; re-derive the occurrence set with both matchers | §2.4 (all zero); §0.4 and §0.7 (growth: one code comment, five doc hits); Task 10 at the IMPL tip |
| 4 | the NC roster as appendix patches with `NC<n>` markers, NC5 recorded vacuous | §7, Appendix H.1-H.5, §4.1 / §4.3 (NC4's extra arm, §0.3) |
| 5 | the `pipeline.go` layout gate, reusing `100/PLAN.md` §6 | §6, Appendix G: six sub-gates, each fired by its own plant |
| 6 | the doc edits planned | Tasks 11 (ADR-0323), 12 (bullet, `:4338` gloss, ledger text VERBATIM), 13 (row flip under the field-count gate) |
| 7 | record the un-fixed tip before the first production edit | Task 5 (16 named unit REDs, `0126` boot-reject message, `0125` green) before Task 6 |

**Self-review, by command (re-run in the publishing tree):**

- **Tasks:** 16, measured with `/usr/bin/grep -c '^### Task ' PLAN.md`.
- **Sub-steps per task:** measured with `awk` over `^- \[ \] ` per `^### Task ` heading. The maximum is
  quoted in `STATE.md` at this close, and must be ≤ 10 (§6.1).
- **Placeholder scan:** `/usr/bin/grep -nE 'TBD|TODO|implement later|fill in' PLAN.md` must read only this
  line's own mention of those words.
- **Type consistency:** checked by `git grep` against the built files. The names `parseListenerFiltersTimeout`
  `(uint64, error)`, `lfTimeoutMs uint64`, `timeoutMs uint64`, the 18 test names, the fixture name and the
  six listener names are spelled identically in §4, §5, §7 and the appendices.
- **Appendix extraction:** every appendix was extracted from THIS file with `ext` and applied to a fresh
  detached worktree at `7d18bcba` in IMPL order: C, D, E, F.1-F.4 on the tip, then A, then B. The tip run
  reproduced 292 RUN / 16 FAIL, and the final run 292 RUN / 0 FAIL. Each H patch applied on A+B with its
  marker present. `go vet` was clean on the fixture package. The figures are recorded in `STATE.md` at this
  close.

---

# Appendices — the MEASURED artifacts (built, run and reverted at this PLAN stage)

Each appendix is the byte-exact artifact the measurement ran. The one exception is F.1's two-line header
comment, corrected after the runs (§0.9). The diffs are against `7d18bcba`. Appendices B, C, D and E are
relative to A+B; C, D and E touch only test files A does not, so they apply on the tip unchanged. Appendix H's
NC patches are relative to A+B.

## Appendix A — shape A, the production CODE hunks (`manager.go` `8 9`, `pipeline.go` `1 1`, `pipeline_deadline_test.go` `1 1`). Task 6

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index d6aed03e..8beb1958 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -29,7 +29,7 @@ type Pipeline struct{}
 //   - OnDestroy is called on every filter (in declaration order) after the
 //     loop ends, regardless of how the loop exited (Continue/StopIteration/
 //     error/timeout).
-func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) (retErr error) {
+func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint64) (retErr error) {
 	defer func() {
 		for _, f := range filters {
 			f.OnDestroy()
diff --git a/internal/listener/listenerfilter/pipeline_deadline_test.go b/internal/listener/listenerfilter/pipeline_deadline_test.go
index fdab4564..b234700f 100644
--- a/internal/listener/listenerfilter/pipeline_deadline_test.go
+++ b/internal/listener/listenerfilter/pipeline_deadline_test.go
@@ -56,7 +56,7 @@ type runResult struct {
 
 // runPeekPipeline starts Pipeline.Run over a real peekerConn wrapping server
 // with one peekOnlyFilter{5}; the result arrives on the returned channel.
-func runPeekPipeline(server net.Conn, timeoutMs uint32) (net.Conn, <-chan runResult) {
+func runPeekPipeline(server net.Conn, timeoutMs uint64) (net.Conn, <-chan runResult) {
 	pc := NewPeekerConn(server)
 	ch := make(chan runResult, 1)
 	go func() {
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index d969313c..4e3bc708 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -166,7 +166,7 @@ type listenerRuntime struct {
 	// per-conn pipeline. lfTimeoutMs is 0 (an explicit 0s: disabled, ADR-0322)
 	// or in [1000, 60000] (ADR-0082); default (nil) 15000.
 	listenerFilterFactories []listenerfilter.FilterInstanceFactory
-	lfTimeoutMs             uint32
+	lfTimeoutMs             uint64
 	continueOnLfTimeout     bool
 	// lfPeekBufSize is the LARGEST listenerfilter.InitialReadBufferSizer hint
 	// across the listener's listener_filters[] (tls_inspector's parsed
@@ -950,20 +950,19 @@ func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netRe
 // parseListenerFiltersTimeout parses Listener.listener_filters_timeout per
 // ADR-0082/ADR-0322: nil defaults to 15000ms; an explicit zero is 0 (disabled,
 // as on the reference); other values outside [1000, 60000]ms error.
-func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error) {
+func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, error) {
 	const defaultMs = 15000
 	if d == nil {
 		return defaultMs, nil
 	}
-	if d.GetSeconds() == 0 && d.GetNanos() == 0 {
-		return 0, nil
+	s, n := d.GetSeconds(), d.GetNanos()
+	if s < 0 || n < 0 {
+		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: expected a positive duration: %ds %dns", name, s, n)
 	}
-	total := d.AsDuration()
-	ms := total / time.Millisecond
-	if ms < 1000 || ms > 60000 {
-		return 0, fmt.Errorf("listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope", name, total)
+	if s > 9223372035 {
+		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
-	return uint32(ms), nil
+	return uint64(s)*1000 + uint64(n)/1e6, nil
 }
 
 // parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
```

## Appendix B — the comment reconciliation, comment-only (`manager.go` `7 6`), relative to A. Task 7

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index 4e3bc708..b4451346 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -163,8 +163,8 @@ type listenerRuntime struct {
 	chainByName  map[string]*chainInfo
 	// 07.2 (Task 9, ADR-0079) listener-filter pipeline plumbing. Populated
 	// from listener_filters[] at build time; consumed by serveConnection's
-	// per-conn pipeline. lfTimeoutMs is 0 (an explicit 0s: disabled, ADR-0322)
-	// or in [1000, 60000] (ADR-0082); default (nil) 15000.
+	// per-conn pipeline. lfTimeoutMs is any whole-millisecond value (0 =
+	// disabled, ADR-0322); default (nil) 15000 (ADR-0323).
 	listenerFilterFactories []listenerfilter.FilterInstanceFactory
 	lfTimeoutMs             uint64
 	continueOnLfTimeout     bool
@@ -828,8 +828,8 @@ func buildListenerRuntimeWithCtx(l *listenerv3.Listener, idx int, cm *cluster.Ma
 		sample.OnDestroy()
 	}
 
-	// ADR-0082: parse listener_filters_timeout (default 15000ms; envelope
-	// [1000, 60000]).
+	// ADR-0323: parse listener_filters_timeout (default 15000ms; any whole-ms
+	// value, rejecting only a negative or out-of-range Duration).
 	lfTimeoutMs, err := parseListenerFiltersTimeout(name, l.GetListenerFiltersTimeout())
 	if err != nil {
 		return nil, err
@@ -948,8 +948,9 @@ func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netRe
 }
 
 // parseListenerFiltersTimeout parses Listener.listener_filters_timeout per
-// ADR-0082/ADR-0322: nil defaults to 15000ms; an explicit zero is 0 (disabled,
-// as on the reference); other values outside [1000, 60000]ms error.
+// ADR-0323 from the Duration's fields (never AsDuration, which saturates):
+// nil defaults to 15000ms; negative seconds or nanos error; seconds above
+// 9223372035 error; otherwise whole milliseconds, truncated (0 = disabled).
 func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, error) {
 	const defaultMs = 15000
 	if d == nil {
```

## Appendix C — `internal/listener/listener_filters_timeout_test.go` (`206 0`, 338 → 544 lines: 13 parse arms + 2 pipeline-hold arms). Task 2

```diff
diff --git a/internal/listener/listener_filters_timeout_test.go b/internal/listener/listener_filters_timeout_test.go
index 2e3795fb..f0867d06 100644
--- a/internal/listener/listener_filters_timeout_test.go
+++ b/internal/listener/listener_filters_timeout_test.go
@@ -16,6 +16,7 @@ import (
 	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
 	"google.golang.org/protobuf/types/known/durationpb"
 
+	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
 	"github.com/pgdad/envoy-go/internal/stats"
 )
 
@@ -336,3 +337,208 @@ func TestListenerFilterTimeoutPreCxTimeoutOnAbort(t *testing.T) {
 		t.Errorf("abort: %s = %d after one timed-out, closed inspection (continue=false), want 1", name, v)
 	}
 }
+
+// ---- phase 101 (ADR-0323): the validity rule, one arm per row ----
+//
+// parseListenerFiltersTimeout computes whole milliseconds from the Duration's
+// FIELDS: nil gives 15000; a negative seconds OR nanos field is rejected; seconds
+// above 9223372035 are rejected; anything else is seconds*1000 + nanos/1e6,
+// truncated, so 0s and every sub-millisecond value give 0 (disabled). Each arm
+// below is single-cause and names the property it fails on.
+
+// lftParse parses {s, n} through the production parser for listener "l_lft".
+// The uint64 conversion is a no-op on the uint64 parser; it keeps this file
+// compiling against a narrower carrier, so a width regression reads as a
+// per-arm value failure instead of a package build failure.
+func lftParse(s int64, n int32) (uint64, error) {
+	v, err := parseListenerFiltersTimeout("l_lft", &durationpb.Duration{Seconds: s, Nanos: n})
+	return uint64(v), err
+}
+
+// lftAccept requires {s, n} to parse without error to exactly wantMs.
+func lftAccept(t *testing.T, label string, s int64, n int32, wantMs uint64) {
+	t.Helper()
+	ms, err := lftParse(s, n)
+	if err != nil {
+		t.Errorf("%s: accept: got error %v, want accepted", label, err)
+		return
+	}
+	if ms != wantMs {
+		t.Errorf("%s: value: got %d ms, want %d ms", label, ms, wantMs)
+	}
+}
+
+// lftReject requires {s, n} to be rejected; a non-empty sub must appear in the
+// error (the phrase is pinned, never the whole string).
+func lftReject(t *testing.T, label string, s int64, n int32, sub string) {
+	t.Helper()
+	ms, err := lftParse(s, n)
+	if err == nil {
+		t.Errorf("%s: reject: accepted with %d ms, want an error", label, ms)
+		return
+	}
+	if sub != "" && !strings.Contains(err.Error(), sub) {
+		t.Errorf("%s: reject message: %q does not contain %q", label, err.Error(), sub)
+	}
+}
+
+// TestParseListenerFiltersTimeoutAboveUint32MsAccepted: 4294968s is 4294968000
+// ms, 704 ms past 2^32 ms; a uint32 carrier wraps it to 704.
+func TestParseListenerFiltersTimeoutAboveUint32MsAccepted(t *testing.T) {
+	lftAccept(t, "4294968s", 4294968, 0, 4294968000)
+}
+
+// TestParseListenerFiltersTimeoutNegativeSecondsRejected: -1s is rejected as
+// the reference rejects it (Expected positive duration).
+func TestParseListenerFiltersTimeoutNegativeSecondsRejected(t *testing.T) {
+	lftReject(t, "-1s", -1, 0, "expected a positive duration")
+}
+
+// TestParseListenerFiltersTimeoutNegativeNanosRejected: -0.5s arrives as
+// {0, -5e8}; a seconds-only sign check would accept it.
+func TestParseListenerFiltersTimeoutNegativeNanosRejected(t *testing.T) {
+	lftReject(t, "-0.5s", 0, -500000000, "expected a positive duration")
+}
+
+// TestParseListenerFiltersTimeoutAboveMaxSecondsRejected: 9223372036s is one
+// second past the reference's measured ceiling (Duration out-of-range).
+func TestParseListenerFiltersTimeoutAboveMaxSecondsRejected(t *testing.T) {
+	lftReject(t, "9223372036s", 9223372036, 0, "duration out of range")
+}
+
+// TestParseListenerFiltersTimeoutProtoMaxRejected: the protobuf Duration
+// maximum, 315576000000s, is past the ceiling too.
+func TestParseListenerFiltersTimeoutProtoMaxRejected(t *testing.T) {
+	lftReject(t, "315576000000s", 315576000000, 0, "duration out of range")
+}
+
+// TestParseListenerFiltersTimeoutMaxAccepted: the largest accepted input,
+// 9223372035.999999999s, is 9223372035999 ms.
+func TestParseListenerFiltersTimeoutMaxAccepted(t *testing.T) {
+	lftAccept(t, "9223372035.999999999s", 9223372035, 999999999, 9223372035999)
+}
+
+// TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero: 0.0005s
+// truncates to 0, which disables the timeout.
+func TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero(t *testing.T) {
+	lftAccept(t, "0.0005s", 0, 500000, 0)
+}
+
+// TestParseListenerFiltersTimeoutTruncatesNotRounds: 0.0019s is 1 ms, not 2.
+func TestParseListenerFiltersTimeoutTruncatesNotRounds(t *testing.T) {
+	lftAccept(t, "0.0019s", 0, 1900000, 1)
+}
+
+// TestParseListenerFiltersTimeoutHalfSecond: 0.5s, below the old 1s floor.
+func TestParseListenerFiltersTimeoutHalfSecond(t *testing.T) {
+	lftAccept(t, "0.5s", 0, 500000000, 500)
+}
+
+// TestParseListenerFiltersTimeoutNinetySeconds: 90s, above the old 60s cap.
+func TestParseListenerFiltersTimeoutNinetySeconds(t *testing.T) {
+	lftAccept(t, "90s", 90, 0, 90000)
+}
+
+// TestParseListenerFiltersTimeoutSixtyOneSeconds: 61s parses to 61000, not a
+// value clamped to 60000.
+func TestParseListenerFiltersTimeoutSixtyOneSeconds(t *testing.T) {
+	lftAccept(t, "61s", 61, 0, 61000)
+}
+
+// TestParseListenerFiltersTimeoutZeroParsesToZero: an explicit 0s is 0.
+func TestParseListenerFiltersTimeoutZeroParsesToZero(t *testing.T) {
+	lftAccept(t, "0s", 0, 0, 0)
+}
+
+// TestParseListenerFiltersTimeoutNilParsesToDefault: an absent field is 15000.
+func TestParseListenerFiltersTimeoutNilParsesToDefault(t *testing.T) {
+	ms, err := parseListenerFiltersTimeout("l_lft", nil)
+	if err != nil {
+		t.Errorf("nil: accept: got error %v, want accepted", err)
+		return
+	}
+	if ms != 15000 {
+		t.Errorf("nil: value: got %d ms, want 15000 ms", ms)
+	}
+}
+
+// lftPeekFilter blocks in a 5-byte Peek, then continues.
+type lftPeekFilter struct{}
+
+func (lftPeekFilter) Inspect(_ context.Context, p listenerfilter.Peeker, _ *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
+	_, _ = p.Peek(5)
+	return listenerfilter.Continue, nil
+}
+
+func (lftPeekFilter) OnDestroy() {}
+
+// lftLoopbackPair returns both ends of one real loopback TCP connection.
+func lftLoopbackPair(t *testing.T) (peer, server net.Conn) {
+	t.Helper()
+	ln, err := net.Listen("tcp", "127.0.0.1:0")
+	if err != nil {
+		t.Fatalf("listen: %v", err)
+	}
+	defer func() { _ = ln.Close() }()
+	ch := make(chan net.Conn, 1)
+	go func() {
+		c, _ := ln.Accept()
+		ch <- c
+	}()
+	peer, err = net.Dial("tcp", ln.Addr().String())
+	if err != nil {
+		t.Fatalf("dial: %v", err)
+	}
+	return peer, <-ch
+}
+
+// lftPipeHold parses {s, n}, runs Pipeline.Run with the PARSED value against a
+// silent loopback peer, and requires Run to still be blocked after hold: the
+// parsed value must not collapse to a short or already-expired deadline.
+func lftPipeHold(t *testing.T, label string, s int64, n int32, hold time.Duration) {
+	t.Helper()
+	v, err := parseListenerFiltersTimeout("l_lft", &durationpb.Duration{Seconds: s, Nanos: n})
+	if err != nil {
+		t.Errorf("%s: pipeline: parse rejected: %v", label, err)
+		return
+	}
+	peer, server := lftLoopbackPair(t)
+	pc := listenerfilter.NewPeekerConn(server)
+	type res struct {
+		err error
+		el  time.Duration
+	}
+	ch := make(chan res, 1)
+	start := time.Now()
+	go func() {
+		var p listenerfilter.Pipeline
+		err := p.Run(context.Background(), []listenerfilter.ListenerFilter{lftPeekFilter{}}, listenerfilter.AsPeeker(pc), &listenerfilter.ChainMatchInputs{}, v)
+		ch <- res{err, time.Since(start)}
+	}()
+	returned := false
+	select {
+	case r := <-ch:
+		returned = true
+		t.Errorf("%s: pipeline hold: Run returned after %v (err %v), want still blocked at %v", label, r.el, r.err, hold)
+	case <-time.After(hold):
+	}
+	_ = peer.Close()
+	_ = pc.Close()
+	if !returned {
+		<-ch
+	}
+}
+
+// TestListenerFilterTimeoutPipelineHoldsAtMax drives Run with the parsed
+// maximum. A uint32 carrier wraps it to ~24 days, which still holds, so this
+// arm guards the overflow edge, not the width.
+func TestListenerFilterTimeoutPipelineHoldsAtMax(t *testing.T) {
+	lftPipeHold(t, "9223372035.999999999s", 9223372035, 999999999, 1000*time.Millisecond)
+}
+
+// TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap drives Run with the
+// parsed 4294968s. A uint32 carrier wraps it to 704 ms, inside the hold: this
+// is the behavioral arm that discriminates the width.
+func TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap(t *testing.T) {
+	lftPipeHold(t, "4294968s", 4294968, 0, 1000*time.Millisecond)
+}
```

## Appendix D — `internal/listener/manager_test.go` (`19 19`: the two pins re-pointed and renamed, the `:4053` comment). Task 3

```diff
diff --git a/internal/listener/manager_test.go b/internal/listener/manager_test.go
index 532a4f5f..4a3f93ea 100644
--- a/internal/listener/manager_test.go
+++ b/internal/listener/manager_test.go
@@ -3336,9 +3336,10 @@ func TestParseListenerFiltersTimeoutDefault(t *testing.T) {
 	}
 }
 
-// TestParseListenerFiltersTimeoutBelowFloorErrors verifies that a 500ms
-// listener_filters_timeout errors with the [1s, 60s] envelope message.
-func TestParseListenerFiltersTimeoutBelowFloorErrors(t *testing.T) {
+// TestParseListenerFiltersTimeoutSubSecondAccepted verifies that a 500ms
+// listener_filters_timeout, once refused by envoy-go's own 1s floor, builds
+// and parses to lfTimeoutMs=500 (ADR-0323).
+func TestParseListenerFiltersTimeoutSubSecondAccepted(t *testing.T) {
 	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
 	filter := mkTcpProxyFilter(t, "c_echo")
 	l := &listenerv3.Listener{
@@ -3353,19 +3354,19 @@ func TestParseListenerFiltersTimeoutBelowFloorErrors(t *testing.T) {
 		ListenerFiltersTimeout: durationpb.New(500 * time.Millisecond),
 	}
 	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
-	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
-	if err == nil {
-		t.Fatal("expected error for listener_filters_timeout=500ms, got nil")
+	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
+	if err != nil {
+		t.Fatalf("NewManager: listener_filters_timeout=500ms rejected: %v", err)
 	}
-	want := `listener: "name": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope`
-	if !strings.Contains(err.Error(), want) {
-		t.Errorf("error %q does not contain %q", err.Error(), want)
+	if got := mgr.runtimes[0].lfTimeoutMs; got != 500 {
+		t.Errorf("lfTimeoutMs = %d, want 500", got)
 	}
 }
 
-// TestParseListenerFiltersTimeoutAboveCapErrors verifies that a 90s
-// listener_filters_timeout errors with the same envelope-violation format.
-func TestParseListenerFiltersTimeoutAboveCapErrors(t *testing.T) {
+// TestParseListenerFiltersTimeoutAboveOldCapAccepted verifies that a 90s
+// listener_filters_timeout, once refused by envoy-go's own 60s cap, builds and
+// parses to lfTimeoutMs=90000 (ADR-0323).
+func TestParseListenerFiltersTimeoutAboveOldCapAccepted(t *testing.T) {
 	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
 	filter := mkTcpProxyFilter(t, "c_echo")
 	l := &listenerv3.Listener{
@@ -3380,13 +3381,12 @@ func TestParseListenerFiltersTimeoutAboveCapErrors(t *testing.T) {
 		ListenerFiltersTimeout: durationpb.New(90 * time.Second),
 	}
 	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
-	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
-	if err == nil {
-		t.Fatal("expected error for listener_filters_timeout=90s, got nil")
+	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
+	if err != nil {
+		t.Fatalf("NewManager: listener_filters_timeout=90s rejected: %v", err)
 	}
-	want := `listener: "name": listener_filters_timeout 1m30s is outside the supported [1s, 60s] envelope`
-	if !strings.Contains(err.Error(), want) {
-		t.Errorf("error %q does not contain %q", err.Error(), want)
+	if got := mgr.runtimes[0].lfTimeoutMs; got != 90000 {
+		t.Errorf("lfTimeoutMs = %d, want 90000", got)
 	}
 }
 
@@ -4050,7 +4050,7 @@ func TestUnifiedDispatchListenerFilterTimeoutAbortsConnection(t *testing.T) {
 		FilterChains: []*listenerv3.FilterChain{
 			{Filters: []*listenerv3.Filter{filter}},
 		},
-		// 1s pipeline timeout (ADR-0082 floor); slowListenerFilter blocks 2s.
+		// 1s pipeline timeout; slowListenerFilter blocks 2s.
 		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
 		// continue_on_listener_filters_timeout defaults to false (zero value).
 	}
```

## Appendix E — `internal/listener/quic_test.go` (`35 0`: the QUIC build arm). Task 3

```diff
diff --git a/internal/listener/quic_test.go b/internal/listener/quic_test.go
index 183af340..acc55f88 100644
--- a/internal/listener/quic_test.go
+++ b/internal/listener/quic_test.go
@@ -16,6 +16,7 @@ import (
 	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
 	quic "github.com/quic-go/quic-go"
 	http3 "github.com/quic-go/quic-go/http3"
+	"google.golang.org/protobuf/types/known/durationpb"
 	"google.golang.org/protobuf/types/known/wrapperspb"
 
 	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
@@ -1849,3 +1850,37 @@ func TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins(t *testing.T) {
 		})
 	}
 }
+
+// TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds: a QUIC listener
+// carrying listener_filters_timeout 0.5s or 120s (both outside envoy-go's old
+// [1s, 60s] envelope) builds without error, as the reference accepts both on a
+// QUIC listener (ADR-0323). The value is stored and never read: no QUIC path
+// runs a listener-filter pipeline.
+func TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds(t *testing.T) {
+	for _, tc := range []struct {
+		name   string
+		d      time.Duration
+		wantMs uint64
+	}{
+		{"0.5s", 500 * time.Millisecond, 500},
+		{"120s", 120 * time.Second, 120000},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
+			l := mkQUICListener(t, "c_echo", testAlphaCertPEM, testAlphaKeyPEM, []string{"h3"})
+			l.ListenerFiltersTimeout = durationpb.New(tc.d)
+			boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
+			mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
+			if err != nil {
+				t.Fatalf("build: QUIC listener with listener_filters_timeout %s rejected: %v", tc.name, err)
+			}
+			rt := mgr.runtimes[0]
+			if rt.kind != kindQUIC {
+				t.Errorf("kind: runtime kind = %v, want kindQUIC", rt.kind)
+			}
+			if uint64(rt.lfTimeoutMs) != tc.wantMs {
+				t.Errorf("value: lfTimeoutMs = %d, want %d", rt.lfTimeoutMs, tc.wantMs)
+			}
+		})
+	}
+}
```

## Appendix F.1 — `test/fixtures/0126-listener-filters-timeout-envelope/driver/driver.go` (673 lines). Task 4

```go
// Package driver registers the 0126-listener-filters-timeout-envelope fixture
// with the differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 101, SPEC §7): reference Envoy accepts
// Listener.listener_filters_timeout outside envoy-go's old [1s, 60s] envelope
// and enforces it as a millisecond count truncated from the Duration: 0.5s
// closes a silent client at ~500 ms, 4294968s does NOT wrap (a uint32 of ms
// would wrap it to ~705 ms), and 0.0005s truncates to 0 = disabled. The
// un-fixed envoy-go boot-rejects every one of these values.
//
// Six plaintext TCP listeners, each a ONE-LINE diff from one base
// (tls_inspector, fc_indexed matching transport_protocol tls, a last-resort
// default_filter_chain):
//
//	listener      delta                                 pre_cx (both sides)
//	l_half        0.5s, continue: false                 1 (H1)
//	l_half_true   0.5s, continue: true                  1 (T1 falls through)
//	l_one_true    1s, continue: true (T1's MIRROR)      0 (T2's GET beats it)
//	l_wrap        4294968s, continue: false             0 (W1: no wrap)
//	l_subms       0.0005s, continue: false              0 (M1: truncates to 0)
//	l_nofilt_120  no listener_filters, 120s, false      0 (N1: nothing to time)
//
// Every listener's fc_indexed matches "tls" and every client is plaintext, so
// every served body is the DEFAULT chain's: on a `true` listener the body
// cannot tell a fall-through from an early GET. The discriminator is the
// counter (S): l_half_true 1 beside its mirror l_one_true 0.
//
// ⚠️ WHAT THIS FIXTURE DELIBERATELY DOES NOT PIN (SPEC §7.7): any reject
// message, a close KIND (recorded, never asserted), an exact millisecond
// (windows only), downstream_cx_total on a drop listener, any
// downstream_listener_filter_* name, and the 1 ms immediate-request outcome.
//
// Every arm on a side runs CONCURRENTLY, so a side's drive costs max(arm),
// ~1.5 s (W1/M1's holds; H1 closes at ~0.5 s), not the sum of the arms.
package driver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0126-listener-filters-timeout-envelope"

// refAdminPort is the in-container reference admin port, fixed at 9901 by the
// harness.
const refAdminPort = 9901

// drivenPath is the path every GET arm requests. Both chains of every listener
// route "/" to a direct_response; the BODY names the chain.
const drivenPath = "/lft"

// wantStatus is the direct_response status on every chain.
const wantStatus = 200

// The H1 close window, in milliseconds from the client's dial returning.
//
// ⚠️ SET FROM MEASUREMENT, NOT FROM THE CONFIG. The configured deadline is
// 500 ms on both sides; README §Window records the per-side spreads and the
// σ-margin arithmetic. The window excludes an immediate close and a mis-parse
// to the 1 s mirror or the 15 s default. Widen it with a new measurement,
// never drop the arm.
const (
	windowLoMs = 350
	windowHiMs = 900
)

// Arm timing.
const (
	h1Hold     = 2000 * time.Millisecond // H1: hold a silent client this long
	openHold   = 1500 * time.Millisecond // W1/M1: still open at this point
	tDelay     = 800 * time.Millisecond  // T1/T2: silent this long, then GET
	getTimeout = 5 * time.Second         // a GET arm's read budget after its write
)

// Body tokens. The served body is "<token> <listener>\n", so one observed body
// names both the chain and the listener that produced it.
const (
	chainIndexed = "INDEXED"
	chainDefault = "DEFAULT"
)

// indexedMatch is fc_indexed's filter_chain_match.transport_protocol on EVERY
// listener: no plaintext client ever matches it.
const indexedMatch = "tls"

// lspec is one listener. Order is the index-wise zip the runner performs
// between SubjectListenerNames() and ReferenceListenerPorts().
type lspec struct {
	name    string
	refPort int // in-container reference port — CENSUSED (SPEC §7.3)
	// filters: whether the listener carries the tls_inspector listener filter.
	filters bool
	// timeout is the listener_filters_timeout literal — spelled in seconds,
	// never "500ms" (protojson).
	timeout string
	// cont is continue_on_listener_filters_timeout.
	cont bool
	// wantPreCx is downstream_pre_cx_timeout after the drive, BY VALUE.
	wantPreCx uint64
}

var listeners = []lspec{
	{name: "l_half", refPort: 15126, filters: true, timeout: "0.5s", cont: false, wantPreCx: 1},
	{name: "l_half_true", refPort: 15232, filters: true, timeout: "0.5s", cont: true, wantPreCx: 1},
	{name: "l_one_true", refPort: 15233, filters: true, timeout: "1s", cont: true, wantPreCx: 0},
	{name: "l_wrap", refPort: 15234, filters: true, timeout: "4294968s", cont: false, wantPreCx: 0},
	{name: "l_subms", refPort: 15235, filters: true, timeout: "0.0005s", cont: false, wantPreCx: 0},
	{name: "l_nofilt_120", refPort: 15236, filters: false, timeout: "120s", cont: false, wantPreCx: 0},
}

func init() { fixture.RegisterFixture(fixtureName, &lfeDriver{}) }

// lfeDriver is STATEFUL: each side's Drive records its observations so
// AssertStats can assert every arm absolutely, per side, one Errorf per
// property. A Drive-time error would become t.Fatalf and MASK every later arm.
type lfeDriver struct {
	mu    sync.Mutex
	obs   map[string]*sideObs // side -> observations
	addrs map[string]map[string]string
}

// closeObs is one silent client's outcome.
type closeObs struct {
	closed bool   // the server ended the connection before the hold elapsed
	ms     int64  // ms from dial-return to the close (or to the hold's end)
	bytes  int    // bytes the server sent before closing
	kind   string // FIN / RST / other:<err> / open / dial:<err> — RECORDED, not pinned
}

// getObs is one GET arm's outcome.
type getObs struct {
	status int
	body   string
	err    string
}

type sideObs struct {
	h1, w1, m1 closeObs
	get        map[string]getObs // arm -> outcome (T1, T2, N1)
}

// openArm is one silent client that must still be open at openHold.
type openArm struct {
	id       string
	listener string
	why      string
}

var openArms = []openArm{
	{id: "W1", listener: "l_wrap", why: "4294968s is 4294968000 ms; a uint32 of ms wraps it to ~705 ms"},
	{id: "M1", listener: "l_subms", why: "0.0005s truncates to 0 ms, which disables the deadline"},
}

// getArm is one timed GET: which listener, how long to stay silent first, and
// the chain whose body must answer.
type getArm struct {
	id        string
	listener  string
	delay     time.Duration
	wantChain string
}

var getArms = []getArm{
	{id: "T1", listener: "l_half_true", delay: tDelay, wantChain: chainDefault},
	{id: "T2", listener: "l_one_true", delay: tDelay, wantChain: chainDefault},
	{id: "N1", listener: "l_nofilt_120", delay: 0, wantChain: chainDefault},
}

var (
	_ fixture.Driver              = (*lfeDriver)(nil)
	_ fixture.MultiListenerDriver = (*lfeDriver)(nil)
	_ fixture.StatsAsserter       = (*lfeDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: a never-dialed placeholder (the runner rejects 0 and
// envoy-go boot-rejects an absent static_resources.clusters key).
func (*lfeDriver) BackendCount() int { return 1 }

func (*lfeDriver) SubjectListenerName() string { return listeners[0].name }

func (*lfeDriver) ReferenceListenerPort() int { return listeners[0].refPort }

func (*lfeDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig binds the six subject listeners at subjListenerPort+0..+5,
// inside the runner's probed 16-port block.
func (*lfeDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject are UNREACHABLE while MultiListenerDriver is
// implemented; they refuse rather than derive sibling addresses.
func (*lfeDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, errors.New("0126 drives only through DriveReferenceMulti")
}

func (*lfeDriver) DriveSubject(context.Context, string) ([]byte, error) {
	return nil, errors.New("0126 drives only through DriveSubjectMulti")
}

func (*lfeDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

func (*lfeDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*lfeDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *lfeDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "ref", addrs)
}

func (d *lfeDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "subj", addrs)
}

// drive runs EVERY arm on one side concurrently and emits a side-independent,
// timing-free verdict stream for the runner's CompareBytes. Only a missing
// address returns an error; every per-arm failure is recorded and asserted in
// AssertStats.
func (d *lfeDriver) drive(ctx context.Context, side string, addrs map[string]string) ([]byte, error) {
	for _, l := range listeners {
		if addrs[l.name] == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
	}
	o := &sideObs{get: map[string]getObs{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	start := time.Now()

	wg.Add(3)
	go func() { defer wg.Done(); o.h1 = silentProbe(ctx, addrs["l_half"], h1Hold) }()
	go func() { defer wg.Done(); o.w1 = silentProbe(ctx, addrs["l_wrap"], openHold) }()
	go func() { defer wg.Done(); o.m1 = silentProbe(ctx, addrs["l_subms"], openHold) }()
	for _, a := range getArms {
		wg.Add(1)
		go func(a getArm) {
			defer wg.Done()
			g := getProbe(ctx, addrs[a.listener], a.delay)
			mu.Lock()
			o.get[a.id] = g
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	log.Printf("%s: %s drive wall time %d ms", fixtureName, side, time.Since(start).Milliseconds())

	d.mu.Lock()
	if d.obs == nil {
		d.obs = map[string]*sideObs{}
		d.addrs = map[string]map[string]string{}
	}
	d.obs[side] = o
	d.addrs[side] = addrs
	d.mu.Unlock()

	logTimings(side, o)

	var b bytes.Buffer
	fmt.Fprintf(&b, "H1 l_half closed=%t in_window=%t bytes=%d\n", o.h1.closed, o.h1.closed && inWindow(o.h1.ms), o.h1.bytes)
	fmt.Fprintf(&b, "W1 l_wrap open_at_%dms=%t\n", openHold.Milliseconds(), isOpen(o.w1))
	fmt.Fprintf(&b, "M1 l_subms open_at_%dms=%t\n", openHold.Milliseconds(), isOpen(o.m1))
	for _, a := range getArms {
		g := o.get[a.id]
		fmt.Fprintf(&b, "%s %s status=%d body=%q err=%q\n", a.id, a.listener, g.status, g.body, g.err)
	}
	return b.Bytes(), nil
}

func inWindow(ms int64) bool { return ms >= windowLoMs && ms <= windowHiMs }

func isOpen(c closeObs) bool { return !c.closed && c.kind == "open" }

func (o *sideObs) open(id string) closeObs {
	if id == "W1" {
		return o.w1
	}
	return o.m1
}

// silentProbe dials addr, sends NOTHING, and reads until the server closes or
// hold elapses. The clock starts when the dial returns. The close kind is
// RECORDED (FIN = io.EOF, RST = ECONNRESET) and never asserted.
func silentProbe(ctx context.Context, addr string, hold time.Duration) closeObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return closeObs{kind: "dial:" + err.Error()}
	}
	t0 := time.Now()
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(t0.Add(hold))
	buf := make([]byte, 4096)
	n := 0
	for {
		k, rerr := c.Read(buf)
		n += k
		if rerr == nil {
			continue
		}
		ms := time.Since(t0).Milliseconds()
		var ne net.Error
		switch {
		case errors.As(rerr, &ne) && ne.Timeout():
			return closeObs{closed: false, ms: ms, bytes: n, kind: "open"}
		case errors.Is(rerr, io.EOF):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "FIN"}
		case errors.Is(rerr, syscall.ECONNRESET):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "RST"}
		default:
			return closeObs{closed: true, ms: ms, bytes: n, kind: "other:" + rerr.Error()}
		}
	}
}

// getProbe dials addr, stays silent for delay, then sends one HTTP/1.1 GET and
// reads the response. A connection the server closed during the silence
// surfaces as a recorded error, not a returned one.
func getProbe(ctx context.Context, addr string, delay time.Duration) getObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return getObs{err: "dial: " + err.Error()}
	}
	defer func() { _ = c.Close() }()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return getObs{err: "ctx: " + ctx.Err().Error()}
		}
	}
	_ = c.SetDeadline(time.Now().Add(getTimeout))
	req := "GET " + drivenPath + " HTTP/1.1\r\nHost: lfe\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(c, req); err != nil {
		return getObs{err: "write: " + err.Error()}
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return getObs{err: "read: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return getObs{status: resp.StatusCode, body: string(body), err: "body: " + err.Error()}
	}
	return getObs{status: resp.StatusCode, body: string(body)}
}

// logTimings RECORDS every silent connection's raw outcome and every GET's
// observed body, pass or fail, so a green run still leaves the measurement the
// window is set from. fixture.TB has no Logf.
func logTimings(side string, o *sideObs) {
	for _, s := range []struct {
		id, listener string
		c            closeObs
	}{{"H1", "l_half", o.h1}, {"W1", "l_wrap", o.w1}, {"M1", "l_subms", o.m1}} {
		log.Printf("%s: %s %s %s closed=%t ms=%d bytes=%d kind=%s",
			fixtureName, side, s.id, s.listener, s.c.closed, s.c.ms, s.c.bytes, s.c.kind)
	}
	for _, a := range getArms {
		g := o.get[a.id]
		log.Printf("%s: %s %s %s status=%d body=%q err=%q", fixtureName, side, a.id, a.listener, g.status, g.body, g.err)
	}
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, every arm absolutely and the per-listener
// downstream_pre_cx_timeout BY VALUE, one Errorf per property. Fatalf is
// reserved for a broken precondition (a failed scrape, an unrecorded drive).
func (d *lfeDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	d.mu.Lock()
	obs, addrs := d.obs, d.addrs
	d.mu.Unlock()
	for _, side := range []struct{ name, admin string }{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		o := obs[side.name]
		if o == nil {
			t.Fatalf("%s: no drive observations recorded — every assertion would be vacuous", side.name)
			return
		}
		assertArms(t, side.name, o)
		prom, err := scrapePromByAddr(side.admin)
		if err != nil {
			t.Fatalf("%s: scrape /stats/prometheus: %v", side.name, err)
			return
		}
		for _, l := range listeners {
			assertPreCx(t, side.name, l, listenerLabel(side.name, l, addrs[side.name][l.name]), prom)
		}
	}
}

func assertArms(t fixture.TB, side string, o *sideObs) {
	t.Helper()
	// H1 — one silent client on l_half (0.5s, false): closed, in window, 0 bytes.
	if !o.h1.closed {
		t.Errorf("%s H1 l_half: silent client NOT closed by the server within %d ms (kind=%s) — "+
			"listener_filters_timeout 0.5s with continue:false must close it", side, h1Hold.Milliseconds(), o.h1.kind)
	} else {
		if !inWindow(o.h1.ms) {
			t.Errorf("%s H1 l_half: closed at %d ms, want within [%d, %d] ms", side, o.h1.ms, windowLoMs, windowHiMs)
		}
		if o.h1.bytes != 0 {
			t.Errorf("%s H1 l_half: server sent %d bytes before closing, want 0", side, o.h1.bytes)
		}
	}
	// W1 / M1 — still open at openHold.
	for _, a := range openArms {
		if c := o.open(a.id); !isOpen(c) {
			t.Errorf("%s %s %s: silent client ended at %d ms (kind=%s), want still open at %d ms — %s",
				side, a.id, a.listener, c.ms, c.kind, openHold.Milliseconds(), a.why)
		}
	}
	// T1 / T2 / N1 — the served chain, by body.
	for _, a := range getArms {
		g, ok := o.get[a.id]
		if !ok {
			t.Errorf("%s %s %s: no observation recorded", side, a.id, a.listener)
			continue
		}
		if g.err != "" {
			t.Errorf("%s %s %s: GET after %d ms failed: %s", side, a.id, a.listener, a.delay.Milliseconds(), g.err)
			continue
		}
		if g.status != wantStatus {
			t.Errorf("%s %s %s: status %d, want %d", side, a.id, a.listener, g.status, wantStatus)
		}
		if want := bodyFor(a.wantChain, a.listener); g.body != want {
			t.Errorf("%s %s %s: body %q, want %q", side, a.id, a.listener, g.body, want)
		}
	}
}

// assertPreCx pins downstream_pre_cx_timeout BY VALUE on this listener's own
// address label. A MISSING series is a hard failure, never read as 0.
func assertPreCx(t fixture.TB, side string, l lspec, label string, prom map[string]map[string]uint64) {
	t.Helper()
	const metric = "envoy_listener_downstream_pre_cx_timeout"
	series, ok := prom[metric][label]
	log.Printf("%s: %s S %s %s{envoy_listener_address=%q} = %d (present=%t) want %d",
		fixtureName, side, l.name, metric, label, series, ok, l.wantPreCx)
	if !ok {
		have := make([]string, 0, len(prom[metric]))
		for k := range prom[metric] {
			have = append(have, k)
		}
		sort.Strings(have)
		t.Errorf("%s S %s: %s{envoy_listener_address=%q} ABSENT (series present: %v), want present and == %d",
			side, l.name, metric, label, have, l.wantPreCx)
		return
	}
	if series != l.wantPreCx {
		t.Errorf("%s S %s: %s = %d, want %d", side, l.name, metric, series, l.wantPreCx)
	}
}

// listenerLabel is each side's OWN envoy_listener_address spelling: the
// reference renders "0.0.0.0_<in-container port>"; envoy-go renders its
// configured bind address with ':' and '.' folded to '_'.
func listenerLabel(side string, l lspec, addr string) string {
	if side == "ref" {
		return "0.0.0.0_" + strconv.Itoa(l.refPort)
	}
	return strings.NewReplacer(":", "_", ".", "_").Replace(addr)
}

func bodyFor(chain, listener string) string { return chain + " " + listener + "\n" }

// scrapePromByAddr fetches /stats/prometheus and returns
// metric -> envoy_listener_address -> value, keeping ONLY series that carry an
// envoy_listener_address label.
func scrapePromByAddr(adminAddr string) (map[string]map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	const key = `envoy_listener_address="`
	out := map[string]map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		open := strings.IndexByte(line, '{')
		closeIdx := strings.LastIndexByte(line, '}')
		if strings.HasPrefix(line, "#") || open < 0 || closeIdx < open {
			continue
		}
		labels := line[open+1 : closeIdx]
		ki := strings.Index(labels, key)
		if ki < 0 {
			continue
		}
		val := labels[ki+len(key):]
		end := strings.IndexByte(val, '"')
		if end < 0 {
			continue
		}
		rest := strings.Fields(line[closeIdx+1:])
		if len(rest) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(rest[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		name := line[:open]
		if out[name] == nil {
			out[name] = map[string]uint64{}
		}
		out[name][val[:end]] += uint64(v)
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap renders BOTH sides from one template, differing only in bind
// address, admin port and listener ports.
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bootstrapHeadTmpl, bindAddr, adminPort)
	for i, l := range listeners {
		lf := ""
		if l.filters {
			lf = listenerFiltersBlock
		}
		fmt.Fprintf(&b, listenerTmpl,
			l.name,       // 1
			bindAddr,     // 2
			portFor(i),   // 3
			l.timeout,    // 4
			l.cont,       // 5
			lf,           // 6
			indexedMatch, // 7
			strconv.Quote(bodyFor(chainIndexed, l.name)), // 8
			strconv.Quote(bodyFor(chainDefault, l.name)), // 9
			strings.TrimPrefix(l.name, "l_"),             // 10 stat_prefix stem
		)
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

const bootstrapHeadTmpl = `admin:
  address:
    socket_address: { address: %s, port_value: %d }
static_resources:
  listeners:
`

const listenerFiltersBlock = `      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
`

// listenerTmpl: stat_prefixes are <stem>_indexed / <stem>_default — ALL
// TWELVE DISTINCT (two HCMs sharing a stat_prefix panic envoy-go at boot).
const listenerTmpl = `    - name: %[1]s
      address:
        socket_address: { address: %[2]s, port_value: %[3]d }
      listener_filters_timeout: %[4]s
      continue_on_listener_filters_timeout: %[5]t
%[6]s      filter_chains:
        - name: fc_indexed
          filter_chain_match:
            transport_protocol: %[7]s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %[10]s_indexed
                route_config:
                  name: rc_%[10]s_indexed
                  virtual_hosts:
                    - name: vh_%[10]s_indexed
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: %[8]s }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: %[10]s_default
              route_config:
                name: rc_%[10]s_default
                virtual_hosts:
                  - name: vh_%[10]s_default
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 200
                          body: { inline_string: %[9]s }
              http_filters:
                - name: envoy.filters.http.router
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

const clustersTmpl = `  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
```

## Appendix F.2 — `test/fixtures/0126-listener-filters-timeout-envelope/README.md` (92 lines). Task 4

```markdown
# 0126-listener-filters-timeout-envelope

Cross-side differential for phase 101 (`listener-filters-timeout-envelope-lift`):
reference Envoy **accepts** `Listener.listener_filters_timeout` outside envoy-go's
old `[1s, 60s]` envelope and enforces it as a millisecond count **truncated** from
the Duration. `0.5s` closes a silent client at ~500 ms; `4294968s` does **not**
wrap (a `uint32` of milliseconds would wrap it to ~705 ms); `0.0005s` truncates to
0, which **disables** the deadline. The un-fixed envoy-go boot-rejects every one of
these values (`listener_filters_timeout 500ms is outside the supported [1s, 60s]
envelope`), so at the tip the whole fixture is RED at subject boot.

`0125` is left byte-untouched: putting these values in its bootstrap would mask
every landed `0125` arm behind the same boot reject (SPEC §7.1).

## Shape

Cloned from `0125`: no YAML, no PKI, no `inputs/`. `renderBootstrap` in
`driver/driver.go` builds **both** sides from one template; only the bind address,
the admin port and the six listener ports differ cross-side.

Every listener is a **one-line diff** from one base: `tls_inspector`,
`fc_indexed` matching `transport_protocol: tls` (body `INDEXED <listener>`), and a
last-resort `default_filter_chain` (body `DEFAULT <listener>`). Every client is
plaintext, so `fc_indexed` never matches and every served body is `DEFAULT`.
All twelve HCM stat prefixes are distinct (a shared prefix panics envoy-go at boot).
Durations are spelled in seconds (`0.5s`), never `500ms` (protojson).

| listener | delta | reference port | `downstream_pre_cx_timeout` |
|---|---|---|---|
| `l_half` | `0.5s`, `continue…: false` | 15126 | **1** (H1) |
| `l_half_true` | `0.5s`, `true` | 15232 | **1** (T1 falls through) |
| `l_one_true` | `1s`, `true` — the MIRROR of `l_half_true` | 15233 | **0** (T2's GET beats it) |
| `l_wrap` | `4294968s`, `false` | 15234 | **0** |
| `l_subms` | `0.0005s`, `false` | 15235 | **0** |
| `l_nofilt_120` | no `listener_filters`, `120s`, `false` | 15236 | **0** |

`BackendCount() == 1`: a `c_unused` STATIC cluster no route dials (the runner
rejects 0; envoy-go boot-rejects an absent `clusters` key).

## Arms (every arm on a side runs concurrently — one ~1.5 s drive per side)

| arm | listener | client | pin |
|---|---|---|---|
| H1 | `l_half` | silent, hold 2000 ms | server closed in [350, 900] ms, 0 bytes |
| W1 | `l_wrap` | silent, hold 1500 ms | still open at 1500 ms |
| M1 | `l_subms` | silent, hold 1500 ms | still open at 1500 ms |
| T1 | `l_half_true` | silent 800 ms, then GET | 200, `DEFAULT l_half_true` |
| T2 | `l_one_true` | silent 800 ms, then GET | 200, `DEFAULT l_one_true` |
| N1 | `l_nofilt_120` | GET at 0 | 200, `DEFAULT l_nofilt_120` |
| S | all six | — | `downstream_pre_cx_timeout` BY VALUE on each side's own `envoy_listener_address` label on `/stats/prometheus`; a **missing** series is a hard failure |

The reference label is `0.0.0.0_<in-container port>`; the subject label is its
bind address with `:` and `.` folded to `_` (`127_0_0_1_<port>`).

**T1/T2 pin the counter, not only the body.** Under `true` the body cannot tell
"fell through at 0.5 s" from "the GET arrived first": both are `DEFAULT`. The body
pin only guards chain selection; the discriminator is S's `l_half_true` **1**
beside the mirror `l_one_true` **0**. Both sides served `DEFAULT l_half_true` and
`DEFAULT l_one_true` on every run (PLAN measurement: R1-R3, MEAS1-3, NC1, NC4).

## Window — MEASURED through the harness's host `-p` path

The clock starts when the client's dial returns. Pooled over three runs of a
measurement variant of this driver (H1 plus 30 concurrent replicates on
`l_half`, `wantPreCx` 31; never committed), shape A applied:

| side | n | min | max | mean | σ | (mean-350)/σ | (900-mean)/σ |
|---|---|---|---|---|---|---|---|
| reference (via docker-proxy) | 93 | 500 | 508 | 502.89 | 2.54 | 60 | 156 |
| subject (shape A) | 93 | 500 | 501 | 500.95 | 0.23 | 669 | 1769 |

The three plain runs (R1-R3, one H1 per side each) read reference 501 / 501 / 502
ms and subject 501 / 500 / 501 ms. `[350, 900]` holds with far more than the 4-5σ
the band rule asks for, and stays clear of an immediate close and of a mis-parse to
the 1 s mirror (≥ 1000 ms on both sides in `0125`). Against `0125`'s worst observed
reference spread (σ 13.72, max 1030 ms at 1 s), the margins are still 11σ low and
29σ high. The reference's close through docker-proxy was **FIN in 93 of 93**
(recorded, never pinned).

## Not pinned (SPEC §7.7)

Any reject message; a close KIND; an exact millisecond; `downstream_cx_total` on
a drop listener (the reference counts post-filter); any
`downstream_listener_filter_*` name; the 1 ms immediate-request outcome.

## Negative controls (measured on shape A, one run each)

| NC | mutation in `parseListenerFiltersTimeout` | red arms | green arms |
|---|---|---|---|
| tip | none (no shape A) | subject boot reject (`500ms is outside the supported [1s, 60s] envelope`, 3 attempts); no arm driven on either side | — |
| NC1 | narrow the return to `uint32` | W1 (subject FIN at 704 ms), S `l_wrap` (subject 1), CompareBytes (`W1 … open_at_1500ms=false`) | H1, M1, T1, T2, N1, every other S cell |
| NC4 | round instead of truncate (`+ 500000` before `/1e6`) | M1 (subject FIN at 1 ms), S `l_subms` (subject 1), CompareBytes (`M1 … open_at_1500ms=false`) | H1, W1, T1, T2, N1, every other S cell |
```

## Appendix F.3 — `test/fixtures/0126-listener-filters-timeout-envelope/expectations.yaml` (29 lines). Task 4

```yaml
# Phase 101 fixture 0126-listener-filters-timeout-envelope expectations
# (ADR-0019 — prose; the enforcers are driver/driver.go's per-side AssertStats
# arms, the runner's cross-side CompareBytes of the timing-free verdict stream,
# and the runner's admin probe. Nothing reads this file.)
#
# Reference: envoyproxy/envoy by digest (docs/envoy-go/ENVOY_TARGET.md);
# listeners 0.0.0.0:15126, :15232, :15233, :15234, :15235, :15236.
# Subject: envoy-go, listeners 127.0.0.1:<subjListenerPort>+0..+5.

window_ms: { lo: 350, hi: 900 }   # H1, from dial-return; measured 500-508 ref, 500-501 subj

listeners:
  l_half:       { timeout: 0.5s,     continue: false, filters: [tls_inspector], fc_indexed: tls, pre_cx_timeout: 1 }
  l_half_true:  { timeout: 0.5s,     continue: true,  filters: [tls_inspector], fc_indexed: tls, pre_cx_timeout: 1 }
  l_one_true:   { timeout: 1s,       continue: true,  filters: [tls_inspector], fc_indexed: tls, pre_cx_timeout: 0 }
  l_wrap:       { timeout: 4294968s, continue: false, filters: [tls_inspector], fc_indexed: tls, pre_cx_timeout: 0 }
  l_subms:      { timeout: 0.0005s,  continue: false, filters: [tls_inspector], fc_indexed: tls, pre_cx_timeout: 0 }
  l_nofilt_120: { timeout: 120s,     continue: false, filters: [],              fc_indexed: tls, pre_cx_timeout: 0 }

arms:
  H1: { listener: l_half,       client: silent hold 2000ms,    expect: closed in window, 0 bytes }
  W1: { listener: l_wrap,       client: silent hold 1500ms,    expect: open at 1500ms }
  M1: { listener: l_subms,      client: silent hold 1500ms,    expect: open at 1500ms }
  T1: { listener: l_half_true,  client: silent 800ms then GET, expect: 200 "DEFAULT l_half_true\n" }
  T2: { listener: l_one_true,   client: silent 800ms then GET, expect: 200 "DEFAULT l_one_true\n" }
  N1: { listener: l_nofilt_120, client: GET at 0,              expect: 200 "DEFAULT l_nofilt_120\n" }
  S:  { metric: envoy_listener_downstream_pre_cx_timeout, keyed_by: envoy_listener_address, missing: hard failure }

not_pinned: [reject message, close kind, exact ms, downstream_cx_total, downstream_listener_filter_*, 1ms immediate-request outcome]
```

## Appendix F.4 — `test/differential/runner_test.go`, the blank import (`1 0`). Task 4

```diff
diff --git a/test/differential/runner_test.go b/test/differential/runner_test.go
index 69934bf9..b59e46d4 100644
--- a/test/differential/runner_test.go
+++ b/test/differential/runner_test.go
@@ -150,6 +150,7 @@ import (
 	_ "github.com/pgdad/envoy-go/test/fixtures/0123-listener-transport-protocol/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0124-listener-sni-longest-suffix/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0125-listener-filters-timeout/driver"
+	_ "github.com/pgdad/envoy-go/test/fixtures/0126-listener-filters-timeout-envelope/driver"
 	"github.com/pgdad/envoy-go/test/helpers"
 
 	// Blank-imported so the lua filter's init() boot-registration fires for
```

## Appendix G — `layout-gate.sh <worktree> [base]` (six sub-gates; exit = failed count). Tasks 6, 7

```bash
#!/bin/bash
# Phase-101 layout gate (SPEC §4). usage: layout-gate.sh <worktree> [base-ref]
# Compares the WORKING TREE of <worktree> against base-ref (default: merge-base HEAD master).
# Prints one PASS/FAIL line per sub-gate; exit status = number of failed sub-gates.
W=${1:?worktree}
BASE=${2:-$(git -C "$W" merge-base HEAD master)}
P=internal/listener/listenerfilter/pipeline.go
fail=0
ok()  { echo "PASS ($1) $2"; }
bad() { echo "FAIL ($1) $2"; fail=$((fail+1)); }
base() { git -C "$W" show "$BASE:$P"; }

# (a) the OnDestroy defer, pipeline.go:33-37, is md5-identical to base (cited by ADR-0322).
a_base=$(base | sed -n '33,37p' | md5sum)
a_now=$(sed -n '33,37p' "$W/$P" | md5sum)
if [ "$a_base" = "$a_now" ]; then ok a "pipeline.go:33-37 md5-identical to $BASE"; else bad a "pipeline.go:33-37 differ from $BASE"; diff <(base | sed -n '33,37p') <(sed -n '33,37p' "$W/$P") | sed 's/^/    /'; fi

# (b) the WithTimeout line, pipeline.go:43, is byte-identical to base (cited 8 times).
if [ "$(base | sed -n '43p')" = "$(sed -n '43p' "$W/$P")" ] && [ -n "$(sed -n '43p' "$W/$P")" ]; then ok b "pipeline.go:43 byte-identical to $BASE"; else bad b "pipeline.go:43 differs from $BASE"; diff <(base | sed -n '43p') <(sed -n '43p' "$W/$P") | sed 's/^/    /'; fi

# (c) pipeline.go:32 is still the Run signature: byte-identical to base, OR base with exactly
#     the one legal edit "timeoutMs uint32" -> "timeoutMs uint64" (the row's width change).
c_base=$(base | sed -n '32p')
c_wide=$(printf '%s\n' "$c_base" | sed 's/timeoutMs uint32)/timeoutMs uint64)/')
c_now=$(sed -n '32p' "$W/$P")
if [ "$c_now" = "$c_base" ] || [ "$c_now" = "$c_wide" ]; then ok c "pipeline.go:32 is the Run signature (base or the uint32->uint64 widening only)"; else bad c "pipeline.go:32 is not the Run signature as allowed"; echo "    now: $c_now"; fi

# (d) the import block of pipeline.go is unchanged (no new import).
imp() { awk '/^import \(/{f=1} f{print} f&&/^\)/{exit}'; }
d_base=$(base | imp)
d_now=$(imp < "$W/$P")
if [ -n "$d_now" ] && [ "$d_base" = "$d_now" ]; then ok d "pipeline.go import block unchanged"; else bad d "pipeline.go import block changed"; diff <(echo "$d_base") <(echo "$d_now") | sed 's/^/    /'; fi

# (e) exactly ONE non-comment context.WithTimeout under internal/listener/ (non-test), at pipeline.go:43.
#     Comment hits are dropped (the raw grep reads three lines: pipeline.go:21 and manager.go:478 are prose).
e_hits=$(git -C "$W" grep -n 'context.WithTimeout' -- internal/listener/ ':!*_test.go' | awk -F: '{l=$0; sub(/^[^:]*:[^:]*:/,"",l); sub(/^[ \t]+/,"",l); if (l !~ /^\/\//) print $1":"$2}')
if [ "$e_hits" = "$P:43" ]; then ok e "one code WithTimeout, at $P:43"; else bad e "code WithTimeout hits = [$(echo $e_hits)] want [$P:43]"; fi

# (f) pipeline.go -U0 hunks: any hunk whose first affected OLD line is at/above line 43 must be
#     line-count-neutral (N N); a line-count change there moves a cited line (32, 33-37, 43).
#     A pure insertion "-a,0" affects line a+1; an insertion after line 43 ("-43,0") is legal.
f_bad=$(git -C "$W" diff -U0 "$BASE" -- "$P" | /usr/bin/grep -E '^@@ ' | awk '{
  split(substr($2,2),o,","); split(substr($3,2),n,",");
  a=o[1]+0; b=(2 in o)?o[2]+0:1; c=n[1]+0; d=(2 in n)?n[2]+0:1;
  s=(b==0)?a+1:a;
  if (s<=43 && b!=d) printf "[-%d,%d +%d,%d] ", a, b, c, d }')
if [ -z "$f_bad" ]; then ok f "no line-count-changing pipeline.go hunk at/above line 43"; else bad f "pipeline.go hunk(s) at/above line 43 change the line count: $f_bad"; fi

echo "layout-gate: $fail failed sub-gate(s) (base $BASE)"
exit $fail
```

## Appendix H.1 — NC1, the return narrowed through `uint32` (relative to A+B). Task 9

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b4451346..b2ba4925 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -963,7 +963,7 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, e
 	if s > 9223372035 {
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
-	return uint64(s)*1000 + uint64(n)/1e6, nil
+	return uint64(uint32(uint64(s)*1000 + uint64(n)/1e6)), nil // NC1
 }
 
 // parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
```

## Appendix H.2 — NC2, V2 neutralised (relative to A+B). Task 9

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b4451346..b6a26d40 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -957,7 +957,7 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, e
 		return defaultMs, nil
 	}
 	s, n := d.GetSeconds(), d.GetNanos()
-	if s < 0 || n < 0 {
+	if (s < 0 || n < 0) && false { // NC2
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: expected a positive duration: %ds %dns", name, s, n)
 	}
 	if s > 9223372035 {
```

## Appendix H.3 — NC3, V3 neutralised (relative to A+B). Task 9

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b4451346..4689670e 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -960,7 +960,7 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, e
 	if s < 0 || n < 0 {
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: expected a positive duration: %ds %dns", name, s, n)
 	}
-	if s > 9223372035 {
+	if s > 9223372035 && false { // NC3
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
 	return uint64(s)*1000 + uint64(n)/1e6, nil
```

## Appendix H.4 — NC4, rounding instead of truncation (relative to A+B). Task 9

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b4451346..589c5252 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -963,7 +963,7 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, e
 	if s > 9223372035 {
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
-	return uint64(s)*1000 + uint64(n)/1e6, nil
+	return uint64(s)*1000 + (uint64(n)+500000)/1e6, nil // NC4
 }
 
 // parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
```

## Appendix H.5 — NC6, the old envelope re-inserted (relative to A+B; the H.5 slot carries SPEC row NC6, since NC5 is vacuous and has no patch). Task 9

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b4451346..3ed0fdfb 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -963,7 +963,11 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, e
 	if s > 9223372035 {
 		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
-	return uint64(s)*1000 + uint64(n)/1e6, nil
+	ms := uint64(s)*1000 + uint64(n)/1e6
+	if !(s == 0 && n == 0) && (ms < 1000 || ms > 60000) { // NC6
+		return 0, fmt.Errorf("listener: %q: listener_filters_timeout is outside the supported [1s, 60s] envelope", name)
+	}
+	return ms, nil
 }
 
 // parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
```
