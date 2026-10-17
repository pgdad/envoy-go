# Phase 102 — `stats-flush-interval-bounds` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Stage:** PLAN (lifecycle **2 -> 3**). Worktree `/home/esa/git/envoy-go/.worktrees/wt-phase-102-plan` off
master `44b2de38`, branch `wt-phase-102-plan`. Written under the 2026-07-12 standing directive: **no human
consulted.**
**Spec:** `docs/envoy-go/phases/102-stats-flush-interval-bounds/SPEC.md` (886 lines). The plan argues from the
spec, and executors read both. **§0 records this stage's findings against the SPEC, and the measurements of
every cell the SPEC left PREDICTED. Where the two disagree, this PLAN wins, and §0 says why.**
**Evidence only:** `BRAINSTORM.md` (613 lines). Where it disagrees with `SPEC.md` §0, the SPEC governs; where
the SPEC disagrees with this PLAN's §0, this PLAN governs.

**Goal.** Accept exactly the `stats_flush_interval` values the pinned reference accepts, `[1ms, 5m)`, and refuse
every other present value at `Load` in the reference's three phrases and in its order, with or without a stats
sink. Absent still means 5 s. **+0 stat NAMES.**

**Architecture.** `parseStatsSinks` (`internal/bootstrap/bootstrap.go`) reads `GetSeconds()` / `GetNanos()`,
never the saturating `AsDuration()`, and applies the SPEC's rule F1-F5 (`SPEC.md` §2) unchanged:
- a negative field gives `Invalid duration: Expected positive duration`;
- seconds above `9223372035` or nanos above `999999999` give `Invalid duration: Duration out-of-range`;
- a total outside `[1ms, 300s)` gives `value must be inside range [1ms, 5m0s)`.

Each message is prefixed `bootstrap: stats_flush_interval: `. The code is the SPEC's P2 byte for byte. This
PLAN adds the two half-falsified comment edits the SPEC left unbuilt, and makes both early comment hunks
**line-count-neutral**, so none of the 27 cited `bootstrap.go` lines moves (§0.2, §6). Production is
`bootstrap.go` **`21 10`** (Appendix A). The rest of the plan makes the edit falsifiable:
- 42 new test leaves in two new files (Appendices B-C), 29 RED at the tip;
- two sink-free boot-reject fixtures, `0127` (`0s`) and `0128` (`-0.5s`) (Appendix E);
- nine NC mutants scored per arm, at the unit level and through both fixtures (Appendix D, §4);
- a cite gate shown to fire (Appendix F);
- the doc reconciliation, written out and applied once (Appendices G-H).

**Every code and doc block in the appendices was built, applied and run at this PLAN stage (§4, §10). None of
it is a sketch.**

**Tech stack.** Go (module `go 1.23.0`); `go-control-plane` v3 protos (`durationpb`, `bootstrapv3`);
`internal/bootstrap`, `cmd/envoy-go`; the differential harness against `envoyproxy/envoy:contrib-v1.37.2` by
digest.

---

## Global Constraints

These apply implicitly to every task.

- **Reference pin:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`). **Run it BY DIGEST**, after verifying `docs/envoy-go/ENVOY_TARGET.md` lines 3-4.
- **`grep` is a `ugrep` shell function in EVERY shell here.** It honours `.gitignore`, and `next-prompt.txt`
  is gitignored yet tracked. Use `/usr/bin/grep`, `git grep`, or a direct path, and name the binary.
- **Differential logs contain binary bytes.** `/usr/bin/grep` on them prints `binary file matches` and NO
  matching lines, so a verdict grep reads EMPTY (§0.7). **Pass `-a` to every grep over a differential log.**
- **Use `git -C <abs-worktree>` for every git command.** The Bash tool's cwd silently resets, and shell
  variables do not survive between tool calls. Put `pwd` + `git rev-parse --abbrev-ref HEAD` before any
  commit.
- **`go test` rules:**
  - `-count=1` is not optional, and use `-v` whenever you count: `RUN=0` beside `RC=0` is a vacuous green.
  - Take rc from `PIPESTATUS[0]` or `out=$(…); rc=$?`, never from the end of a pipe.
  - The FAIL matcher is `^(FAIL|--- FAIL)|^ *--- FAIL`. The panic gate is `^panic:|DATA RACE|SIGSEGV`.
  - A `-run` selector that matches nothing prints `[no tests to run]` and EXITS 0.
- **The unit selector** is `./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...` (call it
  `$U`). Name it with every figure: **391** `=== RUN` at the tip, **437** with the new leaves.
- **The fixture selector** is
  `-run 'TestDifferential/(0127-stats-flush-interval-range-boot-reject|0128-stats-flush-interval-negative-boot-reject)$' ./test/differential/`
  (call it `$FXSEL`). It must print exactly **2** `=== RUN   TestDifferential/` lines and **0** `SKIP`.
- **Lint:** `GOTOOLCHAIN=go1.26.2 golangci-lint run …`. **misspell runs in US locale**, so no British
  spellings go into `.go` comments (`normalize`, `behavior`, never `normalise`, `behaviour`). Check `gofmt -l`
  by its OUTPUT; it never exits non-zero. A lint control must COMPILE (§0.9).
- **The appendices live in THIS file as fenced blocks — EXTRACT them, never re-derive them.** Extract one by
  its heading; the closing fence is the opening fence's backticks, so a block may contain shorter fences:
  ```bash
  ext () { awk -v want="$1" '$0 ~ "^## Appendix " want " —" {f=1; next} f && !o && /^(```|~~~)/ {fence=$0; sub(/[a-z]+$/,"",fence); o=1; next} o && $0==fence {exit} o' "$2"; }
  P=docs/envoy-go/phases/102-stats-flush-interval-bounds/PLAN.md
  ext A $W/$P > $S/A.diff
  ```
  **Always `git apply --check` first.** For a whole-file appendix compare `wc -l` against §1.2's figure before
  trusting the extraction. §10 records the extraction, run at this PLAN's close in a fresh worktree.
- **Cost figures come from `git diff --numstat`, never `--stat`.**
- **Build with `-o <scratch>`.** A bare `go build ./cmd/envoy-go/` drops a binary in the worktree.
- **Ports:** the ephemeral range is `32768-60999`, and the harness reserves `20000..31007` and `11000..14999`.
  **`0127` / `0128` take `15127` / `15128`** (the `15000 + <index>` convention). Both read 0 files at this tip
  (§2.4). Re-census them at the IMPL tip.
- **Never tear down a container this session did not create**, and then only BY NAME. A `reaper_*` container
  belongs to the differential itself; a sibling session's containers (seen at this stage: `p116-c1c-*`,
  `busy_brattain`, `golink-ai`) are not yours.
- **The rule is the SPEC's F1-F5, in that ORDER.** Do NOT reorder the cases (NC6 is the falsifier), merge the
  messages (NC9), gate the block on sinks (NC7), or "simplify" to `AsDuration()` (it saturates; that is the
  tip).
- **`bootstrap.go` layout is GATED (§6).** Every `bootstrap.go:<line>` citation outside
  `docs/envoy-go/phases/` must point at a byte-identical line before and after (Appendix F). The two comment
  hunks above `parseStatsSinks` are line-count-neutral by construction.
- **Both fixtures stay SINK-FREE.** A sink-ful boot-reject fixture under a mutant that prints `ready` and then
  panics is read by the harness as "subject proxy started cleanly", with no panic text in its output (§0.4).
- **+0 stat NAMES.** The ledger entry is **delta-only `+0, UNCHANGED`, quoting NO absolute** (`SPEC.md` §8.2).
- **Do NOT add** an out-of-range fixture directory, a nanos-overflow reference arm, a cadence arm, a
  whole-message byte pin, or any `stats_flush_on_admin` / `Pipeline.Run` change. Each is a decided exclusion
  (`SPEC.md` §1, §6.1).
- **Do NOT tidy the in-range test and fixture literals** (`0.5s`, `2s`, `3s`, `0.1s`): all sit inside
  `[1ms, 5m)` (`SPEC.md` §4).

---

## Review Focus

These are inputs the SPEC implies but no arm of its own fixture table exercises. Each line is pinned by a test
in the task named.

1. **A Go-built mixed-sign Duration** (`{Seconds: 1, Nanos: -1}`; protojson cannot produce it, and envoy-go runs
   no PGV). It must be REJECTED with the negative phrase, not read as `999.999999ms`. Pinned by
   `TestStatsFlushInterval_GoBuilt/1s_-1ns` (Task 2). **Its falsifier is NC3** (§4.3).
2. **A Go-built nanos overflow** (`{0, 1000000000}`, `{1, 2147483647}`). It must be REJECTED out-of-range, not
   accepted as 1 s or ~3.1 s. Pinned by the two GoBuilt arms (Task 2). **NC2 reddens exactly those two**, and
   no YAML arm can (protojson normalises them).
3. **An in-range value without a sink.** It must still be stored and stay inert (no flusher goroutine).
   Pinned by the four non-absent `YAMLAccept/*/B0_no_sink` arms (Task 2) and the EXISTING
   `TestStatsSink_InertFlushInterval`. **NC7 reddens all five.**
4. **The exact edges**: `0.001s` and `299.999999999s` accept with EXACT values; `0.000999999s` and `300s`
   reject. Pinned by YAML and GoBuilt arms on both sides of each edge (Task 2). **NC4 and NC8 are the
   falsifiers.**
5. **A zero or negative interval WITH a sink.** Without the row, or under NC1 / NC3, such a config prints
   `envoy-go ready` and then panics `non-positive interval for NewTicker` (§0.5). The differential harness
   cannot see the panic (§0.4), so the unit layer must: the `*/B1_statsd_sink` reject arms for `0s`, `-1s`
   and `-0.5s` (Task 2) fail fast at `Load`, before any ticker exists.

---

## 0. What this PLAN refuted or settled, by execution

Two measurement streams built every block in throwaway detached worktrees off `44b2de38`:
- The **subject** stream (the controller, Docker-free) installed Appendices B-C, captured the tip, applied P2 and
  this PLAN's comment edits, ran the nine NCs at the unit level twice (on P3 and on the final P4), built and
  fired the cite gate, applied Appendices G-H to the real `DECISIONS.md` / `BEHAVIOR_CONTRACT.md`, and drove
  the binary at a real boot.
- The **fixture** agent (the only Docker user) built `0127` / `0128` and ran them under TIP, P2 ×3 and all nine
  NCs, plus a temporary sink-ful probe fixture under P2 and NC1. The controller then re-ran both fixtures on
  P4 and on TIP itself.

Every worktree was created detached and removed; the only one left is the stage worktree.

### 🔴 0.1 — `SPEC.md` §6.2's fixture shape boot-rejects envoy-go FOR AN UNRELATED REASON

The SPEC specifies one listener with "an HCM `direct_response` `222`" and no body. Built that way, the
subject refuses it at the tip:
`listener manager: listener: "l_a": filter_chains[0]: filters[0]: hcm: route 0: direct_response.body is required`.
That is method note 51, an unrelated reject masking the site under test. The tip cell would read FAIL with "subject
stderr does NOT contain …" instead of "subject started cleanly". The sink-gated and lower-bound mutants would
read RED for the same wrong reason. **Both fixtures carry `body: { inline_string: l_a }`.** Controls: with
`stats_flush_interval: 1s` the reference validates the same bootstrap (`--mode validate`, rc 0); at the tip
the subject BOOTS both fixtures.

### 🔴 0.2 — the SPEC's comment hunks would move 22 of the 27 cited `bootstrap.go` lines

`git grep` finds **27** distinct `bootstrap.go:<line>` citations in `internal/`, `cmd/`, `test/`,
`BEHAVIOR_CONTRACT.md` and `DECISIONS.md`, from `:68` to `:756`. P2's first two hunks (`:226`, `:534`) each
ADD a line, so every cite from `:250` down (22 of them) shifts by 1 or 2. Under P2 plus this PLAN's first comment draft
(P3), the cite gate (Appendix F) read **22 moved**. **This PLAN rewrites both hunks line-count-neutral**
(`@@ -223,8 +223,8`, `@@ -531,10 +531,10`); everything that grows sits at `:786` and below, past every
cite. Under the final shape (P4) the gate reads **0 moved**. That is method note 94: a cited line is part of
the decision.

### ⚠️ 0.3 — the floor moved again: `14 5` → `21 10`

The SPEC's `14 5` omitted the two half-falsified comment sites its own §4 names: `FlushInterval`'s second
sentence and `parseStatsSinks`'s doc comment. Built here: `bootstrap.go` **`21 10`** (code `9 2` unchanged;
the doc comment grows by four lines and names the three classes, their order and ADR-0324). This is
`reference_measured_prototype_is_a_lower_bound`'s twenty-fourth consecutive row, and this time it fired on
what the SPEC knowingly left unbuilt, not on a mis-estimate.

### ✅ 0.4 — OWED, MEASURED: the harness reads "ready, then panic" as a CLEAN BOOT

A temporary sink-ful copy of `0127` (`0s` plus one statsd sink, port `15129`, deleted afterwards) under NC1
reads **FAIL `BootRejectFixture: subject proxy started cleanly — expected boot rejection`**, 3 of 3 runs. The
panic text appears in **0** lines of the logs: the harness stops the subject as soon as it prints `envoy-go
ready`. Run alone, the same NC1 binary printed `ready` and died `panic: non-positive interval for NewTicker`
(`flusher.go:34`, launched from `main.go:361`), exit 2, about 9 ms after start. Under P2 the probe PASSES.
**So a sink-ful fixture would be RED under NC1 for a misleading reason and with no panic evidence. The
fixtures stay sink-free, now on a measurement rather than a caution.**

### ✅ 0.5 — OWED, MEASURED: NC3's `-1s` at a real boot

NC3 binary, the Appendix C config plus `stats_flush_interval: -1s`:
- with a statsd sink (`127.0.0.1:17201`, unread): prints `envoy-go ready`, then
  `panic: non-positive interval for NewTicker`, rc **2**, in **11 ms**;
- without a sink: boots and serves; `timeout 4` ends it (rc 124 at 4003 ms, `ready` printed, no panic);
- `--mode validate` with the sink: `configuration OK`, rc 0.

The P4 binary on the same sink config exits rc 1 in 8 ms: `load config: bootstrap: stats_flush_interval:
Invalid duration: Expected positive duration`. `0s` exits rc 1 in 7 ms with the range phrase, and `0.001s`
boots (`ready`, alive at 4 s). **NC3 has the same sink-split observable as NC1** (`SPEC.md` §0.4).

### ✅ 0.6 — OWED, MEASURED: every `0127` / `0128` cell of `SPEC.md` §6.4

All thirteen predicted cells hold (§4.4). One extra arm turns a reasoned claim into a measured one: with
`0128` temporarily set to `-1s`, NC6 **PASSES** the fixture. The mutant escapes, exactly as `SPEC.md` §0.3
reasoned, so `-0.5s` is the right arm.

### ⚠️ 0.7 — a differential log is BINARY to `grep`

Both the controller's P4 and TIP fixture logs make `/usr/bin/grep` print `binary file matches` and nothing
else. A verdict count taken that way reads empty, which looks like "zero PASS" or "zero FAIL" depending on
what you expected. **`-a` is mandatory** (Global Constraints).

### ⚠️ 0.8 — the redundant loop copies are DROPPED, and the matrix was re-run

The module is `go 1.23.0` (per-iteration loop variables since 1.22), so `b, c := b, c` and `c := c` are no-ops.
Removing the four lines takes Appendix B from 150 to **147** lines and Appendix C from 91 to **90**. Re-run at
the tip: **437 RUN / 37 FAIL lines / rc 1**, the same 29 RED leaves. Under P4: **437 / 0 / rc 0**.

### ⚠️ 0.9 — method note 88 fired on THIS stage's first lint plant too

The controller's first planted control (`x := 1; x = 2`, never read) did not compile and fired only
`typecheck`. The second plant (`return x` added) compiles, and fired `errcheck` ×1 and `ineffassign` ×1,
rc 1, before it was deleted. Lint on P4 then read rc 0, now proven live. **Two consecutive stages wrote a
non-compiling first plant.** Step 7 of Task 13 prescribes the compiling form.

### ⚠️ 0.10 — the occurrence census grew by SIX, all of them this phase's own documents

`SPEC.md` §4's command reads **168** at `44b2de38` against the SPEC's **162** at `fbd1b2a3`. Re-running it at
both commits and diffing per file localises the growth: `DECISIONS.md` +5 (the SPEC's own ADR-0324 draft) and
`STATE.md` +1. **No new code, test or fixture site.** Under `docs/envoy-go/phases/` it reads 212 hits in 39 files
against 156 / 38; the difference is `SPEC.md` itself. The table of `SPEC.md` §4 stands. The IMPL re-runs the
census for GROWTH only (Task 8).

### ⚠️ 0.11 — the fixture floor `~580` is an overstatement: `0127` + `0128` + import = **538**

`git diff --numstat`: `0127` `driver.go` 172 + `README.md` 92, `0128` 175 + 97, `runner_test.go` `2 0`. The
SPEC's figure was a file size of `0042` doubled. (A first draft of the READMEs read 91 / 96. Each then gained
one line when its reference-pin wording was corrected from `envoyproxy/envoy:v1.37.2` to the `contrib` tag by
digest.)

### ⚠️ 0.12 — the doc costs, MEASURED by applying them

Appendix G applied to the real `DECISIONS.md` reads **`96 1`** (19839 → **19934**). The status line flips in
place (`1 1`) and §Decision + §Consequences add 95 lines. After it, the house guard reads EMPTY, and line `:19825`
READS `> **STATUS: ACCEPTED`; the decoy `:14866` md5 `929719b67c87` is unmoved, as are `^---$` 216 and
`^## ADR-` 323. Appendix H applied to `BEHAVIOR_CONTRACT.md` reads **`3 1`** (6002 → **6004**): the inert
phrase is rewritten in place, and the ledger entry plus its blank line follow the phase-101 entry. **The IMPL re-measures
both at its last commit (method note 97); these are measurements of THIS text, not of whatever text the
IMPL lands.**

### ✅ 0.13 — CONFIRMED, not refuted

- The tip with the new leaves: **437 RUN, 37 FAIL lines, rc 1; 29 RED leaves / 13 GREEN** (32 `--- FAIL`
  entries minus 3 parents; 14 `--- PASS` minus 1 parent). Every RED name matches `SPEC.md` §3.2.
- The existing suite under P4: **391 / 0** (the two new files moved aside).
- **All nine NC RED sets reproduce `SPEC.md` §7 EXACTLY**, on P3 and again on P4, the two sorted FAIL lists
  `cmp`-identical: NC1 8, NC2 2, NC3 7, NC4 3, NC5 6, NC6 8, NC7 26 plus `TestStatsSink_InertFlushInterval`,
  NC8 4, NC9 15. Every marker read ≥ 1 before (NC9: 2, two mutated lines) and 0 after.
- P4: `go vet` rc 0, `gofmt -l` empty, lint rc 0 (proven live, §0.9), panic gate 0.
- Ports `15127` / `15128`: 0 files each; no socket in any state.
- The sentinel shapes and every baseline figure the router predicted for a PLAN (§2).

---

## 1. Stage scope, MEASURED

### 1.1 What THIS PLAN commit touches — FOUR files

Precedents, by `git show --numstat --format=`: phase-101 PLAN `09b612f9` and phase-100 PLAN `8d6422f3`. Each
touches `STATE.md`, `STATE_HISTORY.md` `2 0`, the new `PLAN.md` and `next-prompt.txt`, and nothing else (§8
records this commit's own). **No `DECISIONS.md`**, so the `PROPOSED` guard stays ARMED at `## ADR-0324`. **No
`ROADMAP.md`**, so `want` stays **134** and the file stays **252** lines.

### 1.2 What the IMPL will touch

| path | measured basis | added / removed |
|---|---|---|
| `internal/bootstrap/bootstrap.go` | Appendix A (P2's code `9 2` + four comment sites) | **21 / 10** |
| `internal/bootstrap/stats_flush_interval_test.go` | Appendix B, new (41 leaves, 3 parents) | **147 / 0** |
| `cmd/envoy-go/stats_flush_interval_test.go` | Appendix C, new (3 leaves, 1 parent) | **90 / 0** |
| `test/fixtures/0127-stats-flush-interval-range-boot-reject/driver/driver.go` | Appendix E.1 | **172 / 0** |
| `test/fixtures/0127-stats-flush-interval-range-boot-reject/README.md` | Appendix E.2 | **92 / 0** |
| `test/fixtures/0128-stats-flush-interval-negative-boot-reject/driver/driver.go` | Appendix E.3 | **175 / 0** |
| `test/fixtures/0128-stats-flush-interval-negative-boot-reject/README.md` | Appendix E.4 | **97 / 0** |
| `test/differential/runner_test.go` | Appendix E.5, two blank imports after `0126` | **2 / 0** |
| `docs/envoy-go/DECISIONS.md` | Appendix G, applied once at this stage | **96 / 1** (measured on THIS text) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Appendix H, applied once at this stage | **3 / 1** (measured on THIS text) |
| `docs/envoy-go/ROADMAP.md` | row 102 flip | **1 / 1** |
| `PROGRESS.md`, `STATE*.md`, `next-prompt.txt` | the close | not LoC |

Code, tests and fixtures together measure **794 / 10 across 8 paths**. `git add -N` plus `git diff --numstat`
in the subject worktree read this before the README wording fix (+2), so the figure for this PLAN's appendices
is 796 / 10.

**Byte-untouched roster.** The IMPL asserts each of these is EMPTY under `git diff master --numstat`:

- `cmd/envoy-go/main.go` and every other non-test file in `cmd/` (D-MS-FLUSH-INERT survives there unchanged)
- `internal/statssink/**` (the flusher's `time.NewTicker` is untouched; the bound makes it unreachable with
  a non-positive interval)
- `internal/stats/**` (+0 names)
- the EXISTING tests `internal/bootstrap/bootstrap_test.go`, `internal/bootstrap/statssink_fuzz_test.go`,
  `cmd/envoy-go/main_test.go` (`SPEC.md` §5: no edit to any existing test)
- every fixture directory except `0127` / `0128`, **the sink fixtures `0089`-`0094`, `0098`, `0101`, `0112`,
  `0113` especially** (their `0.5s` / `0.1s` literals are in range and must not be tidied)
- `REVIEW_FINDINGS.md`, `go.mod`, `go.sum`, `.github/**`

⚠️ **The byte-untouched roster and the edit roster are not a partition** (method note 62). `STATE*.md`,
`PROGRESS.md` and `next-prompt.txt` are on neither list, and are named here for that reason. Set-difference:
neither roster contains ADR-0262's text (ADR-0324 AMENDS it; it is never edited), nor `BEHAVIOR_CONTRACT.md`'s
stale `91-dir` figure on the same line as the inert phrase. That figure is a separate claim this row does not
touch: **record it, do not tidy it**. Neither roster contains `ROADMAP.md`'s six sentinel windows, which stay
byte-identical by md5 (§2.1).

### 1.3 The split gate — EVALUATED WITH A COMMAND, NOT SPLIT

`BOOTSTRAP_PROMPT.md` §6.1 sets the thresholds at **~25 numbered tasks** or **~1500 LoC**, and a per-task
sub-step maximum of ~10.

| accounting | this row (MEASURED at this PLAN) | phase-101 PLAN, for comparison |
|---|---|---|
| production `.go` (code + comments) | **21** added | 16 |
| + unit tests | **+ 147 + 90** → **258** | 277 |
| + fixture `driver.go` + `runner_test.go` (`.go` only) | **+ 172 + 175 + 2** → **607** | 951 |
| + fixture `README.md` | **+ 92 + 97** → **796** | 1072 |
| + docs (ADR 96, contract 3, row 1) | **≈ 896** | ≈ 1176 |

**DECISION: DO NOT SPLIT.** The row is under ~1500 on every accounting, there are 14 tasks (§5), and no task
has more than 9 sub-steps (counted by command, §10). The only seam is {unit layer} / {fixtures}. Splitting there
would ship the rule gated by only one surface: only the fixtures see the reference, and only the unit layer
sees NC2, the Go-built arms and the sink path. **Every row above is MEASURED, the doc rows included (§0.12).**

---

## 2. Sentinel and baselines — RUN AT THIS STAGE's TIP

### 2.1 The three checks, the four NCs, and the check-(2) positive control

Run at the stage worktree tip (`44b2de38`) before any edit, with `/usr/bin/grep`, verbatim from
`next-prompt.txt`. They are **re-run in the publishing tree with identical output** (a PLAN touches no
`ROADMAP.md` byte):

- (1) **ONE**: `NOT DONE: row 102` at `want=134`.
- (2) **SIX** at `:212 :218 :224 :234 :240 :248`.
- (3) **SILENT**.
- NC-A: the substitution was inspected first (`NC LANDED? [ in-progress ]`), then **TWO** lines (`NOT DONE:
  row 62`, `NOT DONE: row 102`).
- NC-B at `want=133`: **TWO** lines (`NOT DONE: row 102`, `GATE FAIL: examined 134 data rows, expected 133`).
- NC-C: **FIRED** (residual 0, `NEVER OPENED: gRPC   <- NC FIRED`).
- NC-D: **96 / 68** under `--`.
- Check-(2) positive control: **6 substitutions ASSERTED, residual 0**.
- Escape-aware malformed set: **{57, 69}**, at `:119` (NF 9) and `:131` (NF 10).
- Row 102 NF: **8** under BOTH forms, at `:164`.
- `ROADMAP.md` **252** lines.
- Per-line md5 with the **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex):
  `212 10d7807bf02d` · `218 4a92f7e62fc6` · `224 2a7eb298b9fd` · `234 242e53c6f7a3` · `240 b2680e6f4fbf` ·
  `248 6caa1c3ce0e7`.

⇒ **The sentinel does NOT fire, and `stop` was NOT created** (absent at the git root).

### 2.2 The `PROPOSED` guard — ARMED, and this stage leaves it ARMED

`^> \*\*STATUS: PROPOSED` hits `:19825`, and a backward `^## ADR-` search resolves it to **`## ADR-0324`**
(`:19823`). The ADR-0231 decoy (`^\*\*Status:\*\* PROPOSED`) still hits `:14866` and resolves to
`## ADR-0231` (`:14864`); that line's md5 is `929719b67c87…`, unchanged. `DECISIONS.md` reads:
- **19839** lines, `^---$` **216**, `^## ADR-` **323**, bare `^## ` **331**;
- tail `ADR-0324`, next-free **ADR-0325** (`grep -c '^## ADR-0325'` ⇒ 0).

All are UNMOVED by this stage. **The IMPL disarms the guard at Task 9.**

### 2.3 The fixture set

The blank-import extractor reads **128 = 128** at this tip, with both `comm` directions EMPTY. With `0127` and
`0128` registered it read **130 = 130** on the prototype (**106 `driver/` + 24 `inputs/`**), both `comm`
directions EMPTY under the **(imports, dirs)** order. Each registered name equals its directory name byte for
byte.

### 2.4 Ports

- `git grep -c '\b15127\b' -- test/ internal/ cmd/` and the same for `15128` read **0 files** at `44b2de38`;
  `ss -tanH` / `ss -uanH` show no socket on either. (The probe port `15129` also read 0 files; the two
  `0129` substring hits sit in fixture `0016-http-compressor` and are not ports.)
- The ad-hoc band this stage took, **`17200-17299`**, was censused first: `git grep -In '172[0-9][0-9]' --
  test/ internal/ cmd/` read **0** lines, the word-bounded form **0**, and `ss` showed nothing in the band. One
  port was used: **`17201`**, an unread statsd target in §0.5's boot probe (listener and admin on port 0).
  ⚠️ **This paragraph spells the band and the fixture ports, so it is a hit in the next census.**

---

## 3. STABLE ANCHORS — use these, never line numbers

- **A1** `func parseStatsSinks(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {` (bootstrap.go).
- **A2** `		if v := d.AsDuration(); v > 0 {` (the line P2 replaces; absent after Task 5).
- **A3** `	// statsFlushIntervalDefault is the stats_flush_interval default (5s) applied` (the const comment).
- **A4** `	// FlushInterval is the stats_flush_interval (default 5s when absent/non-positive` (the field comment).
- **A5** `// stats_sinks[] entries (ADR-0262). stats_flush_interval is ALWAYS stored on` (the doc comment).
- **A6** `	_ "github.com/pgdad/envoy-go/test/fixtures/0126-listener-filters-timeout-envelope/driver"`
  (runner_test.go; the two new imports follow it).
- **A7** `*§Decision and §Consequences follow at the phase-102 IMPL.*` (DECISIONS.md, the retained footer and,
  at this tip, the file's LAST line) and `> **STATUS: PROPOSED — §Context drafted at the phase-102 SPEC`
  (its status line).
- **A8** `` (a bare `stats_flush_interval` is parsed-and-inert) `` and `**Phase 101 — +0, UNCHANGED (` in
  `BEHAVIOR_CONTRACT.md`.
- **A9** the row whose first field is `102` in `ROADMAP.md`.

---

## 4. Test design — MEASURED per arm, not predicted

### 4.1 Unit leaves (`go test -count=1 -v $U`)

| group | leaves | TIP | P4 |
|---|---|---|---|
| `TestStatsFlushInterval_YAMLReject/<v>/<B0_no_sink\|B1_statsd_sink>`: `0s`, `0.0005s`, `0.000999999s`, `300s`, `1000s`, `-1s`, `-0.5s`, `9223372036s`, `315576000000s` | 18 | **18 RED** (`got nil error`) | 18 GREEN |
| `TestStatsFlushInterval_YAMLAccept/<v>/<B0\|B1>`: `absent`, `0.001s`, `0.0015s`, `0.5s`, `299.999999999s` | 10 | 10 GREEN (by design) | 10 GREEN |
| `TestStatsFlushInterval_GoBuilt` rejects: `0s_999999ns`, `300s_0ns`, `1s_-1ns`, `-1s_0ns`, `0s_1000000000ns`, `1s_2147483647ns`, `MaxInt64s_0ns`, `9223372035s_0ns`, `9223372036s_0ns` | 9 | **9 RED** | 9 GREEN |
| `TestStatsFlushInterval_GoBuilt` accepts: `0s_1000000ns`, `299s_999999999ns` | 2 | 2 GREEN | 2 GREEN |
| `TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/{0s,-1s}` | 2 | **2 RED** | 2 GREEN |
| `TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/0.001s` | 1 | 1 GREEN | 1 GREEN |
| **total** | **42** | **29 RED / 13 GREEN** | **42 GREEN** |

### 4.2 Suite counts (`go test -count=1 -v $U`, rc from `PIPESTATUS[0]`)

| tree | `=== RUN` | FAIL lines | rc |
|---|---|---|---|
| TIP, existing suite | **391** | 0 | 0 |
| TIP + Appendices B-C | **437** | **37** | 1 |
| P4 + Appendices B-C | **437** | **0** | 0 |
| P4, existing suite only | **391** | 0 | 0 |

At the tip the package lines read `FAIL internal/bootstrap`, `FAIL cmd/envoy-go`, `ok internal/statssink`.

### 4.3 The NC roster, scored PER ARM (unit; Appendix D on P4; `$U`; RUN 437 every time; panic gate 0 every time)

| NC | mutation | mechanism | RED leaves (MEASURED) | parents RED |
|---|---|---|---|---|
| NC1 | drop `\|\| (secs == 0 && nanos < 1000000)` | zero / sub-ms reach F5 | **8**: YAML `0s`, `0.0005s`, `0.000999999s` × B0/B1; GoBuilt `0s_999999ns`; binary `0s` | 3 |
| NC2 | drop `\|\| nanos > 999999999` | Go-built nanos overflow passes F3 | **2**: GoBuilt `0s_1000000000ns`, `1s_2147483647ns` | 1 |
| NC3 | delete the NEG case | `-1s` reaches F5; `-0.5s` falls to F4 (wrong class) | **7**: YAML `-1s`, `-0.5s` × 2; GoBuilt `1s_-1ns`, `-1s_0ns`; binary `-1s` | 3 |
| NC4 | `secs >= 300` → `secs > 300` | `300s` accepted | **3**: YAML `300s` × 2; GoBuilt `300s_0ns` | 2 |
| NC5 | drop `secs > 9223372035 \|\|` | huge seconds fall to F4 | **6**: YAML `9223372036s`, `315576000000s` × 2; GoBuilt `MaxInt64s_0ns`, `9223372036s_0ns` | 2 |
| NC6 | order F4, F2, F3 | `-0.5s` and huge seconds hit F4 first; `-1s` does not | **8**: YAML `-0.5s`, `9223372036s`, `315576000000s` × 2; GoBuilt `MaxInt64s_0ns`, `9223372036s_0ns` | 2 |
| NC7 | gate the block on `len(bs.GetStatsSinks()) > 0` | no sink ⇒ field ignored ⇒ 5 s | **26**: 9 B0 YAML rejects, 9 GoBuilt rejects, binary `0s` / `-1s`, 4 B0 non-absent accepts, 2 GoBuilt accepts | 5, incl. the EXISTING `TestStatsSink_InertFlushInterval` |
| NC8 | `nanos < 1000000` → `nanos <= 1000000` | 1 ms edge rejected | **4**: YAML `0.001s` × 2; GoBuilt `0s_1000000ns`; binary `0.001s` | 3 |
| NC9 | all three cases print the RANGE message | class collapse | **15**: 7 NEG + 8 OOR leaves | 3 |

**None is vacuous.** "Must NOT redden" was read off the same runs: every leaf outside each set was GREEN. For
example, NC6 leaves YAML `-1s` × 2, binary `-1s`, GoBuilt `-1s_0ns` and `1s_-1ns` GREEN; NC5 leaves
GoBuilt `9223372035s_0ns` GREEN. The full per-NC sorted FAIL lists were `cmp`-identical between P3 and P4.

### 4.4 Fixtures `0127` / `0128`, per shape — MEASURED (`$FXSEL`; 2 `=== RUN` and 0 SKIP in every run)

| shape | rc | `0127` (`0s`) | `0128` (`-0.5s`) | SPEC §6.4 prediction |
|---|---|---|---|---|
| TIP (agent; controller re-run) | 1 | FAIL: `subject proxy started cleanly` | FAIL: `subject proxy started cleanly` | ✅ |
| P2 ×3 (agent), P4 ×1 (controller) | 0 | PASS | PASS | ✅ |
| NC1 | 1 | FAIL: started cleanly | PASS | ✅ |
| NC3 | 1 | PASS | FAIL: subject stderr lacks `Invalid duration: Expected positive duration` (printed the range phrase) | ✅ |
| NC6 | 1 | PASS | FAIL: same, range phrase | ✅ |
| NC7 | 1 | FAIL: started cleanly | FAIL: started cleanly | ✅ |
| NC9 | 1 | PASS | FAIL: same, range phrase | ✅ |
| NC2, NC4, NC5, NC8 | 0 each | PASS | PASS | ✅ |
| NC6 with `0128` temporarily at `-1s` | 0 | — | **PASS** (the mutant escapes) | §0.6 |

Subtest wall time across 13 agent runs: `0127` 2.18-2.71 s, `0128` 1.58-1.95 s; a whole `$FXSEL` run takes
~4-6 s. Reference stderr, measured: `0s` → `Proto constraint validation failed (BootstrapValidationError.StatsFlushInterval:
value must be inside range [1ms, 5m0s))`; `-0.5s` → `` error `Invalid duration: Expected positive duration:
goo.gle/debugstr … nanos: -500000000` initializing config ``. Neither needle reaches the token.

### 4.5 The real binary — boots (P4 and NC3; §0.5)

| binary | config | rc | time | observable |
|---|---|---|---|---|
| P4 | `-1s` + statsd sink | 1 | 8 ms | `load config: bootstrap: stats_flush_interval: Invalid duration: Expected positive duration` |
| P4 | `0s` + statsd sink | 1 | 7 ms | `… value must be inside range [1ms, 5m0s)` |
| P4 | `0.001s` + statsd sink | 124 (alive) | 4003 ms | `envoy-go ready`, no panic |
| NC3 | `-1s` + statsd sink | **2** | 11 ms | `ready`, then `panic: non-positive interval for NewTicker` |
| NC3 | `-1s`, no sink | 124 (alive) | 4003 ms | `ready`, no panic |
| NC3 | `-1s` + sink, `--mode validate` | 0 | — | `configuration OK` |

---

## 5. Tasks

⚠️ **Order is load-bearing.** Tasks 1-4 land every falsifier and record the un-fixed tip **before** Task 5
changes production code. Nothing after Task 5 can recreate that measurement. ⚠️ **Evaluate the context budget
BEFORE starting each task** (`checking-context-budget`). Commit after every task and keep `PROGRESS.md`
current. If the session stops mid-spine, say which task it reached and leave row 102 `in-progress` (method
note 47).

Throughout, re-export these in every Bash call:
```bash
W=/home/esa/git/envoy-go/.worktrees/wt-phase-102-impl
S=<your scratch dir>
P=docs/envoy-go/phases/102-stats-flush-interval-bounds/PLAN.md
U='./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...'
FXSEL='TestDifferential/(0127-stats-flush-interval-range-boot-reject|0128-stats-flush-interval-negative-boot-reject)$'
# plus `ext` from Global Constraints
```

---

### Task 1: Baseline, a proven-live panic gate, and `PROGRESS.md`

**Files:**
- Create: `docs/envoy-go/phases/102-stats-flush-interval-bounds/PROGRESS.md`

**Interfaces:**
- Produces: `$S/base-run.txt` (the sorted `=== RUN` roster at the un-fixed tip, selector `$U`). Tasks 4 and 12
  diff against it.

- [ ] **Step 1: Create the IMPL worktree off the CURRENT master tip.**
  `git -C /home/esa/git/envoy-go worktree add -b wt-phase-102-impl $W master`. Then check `pwd` and
  `git -C $W rev-parse --abbrev-ref HEAD` ⇒ `wt-phase-102-impl`.
- [ ] **Step 2: Resolve the selectors before believing any FAIL.** A nonexistent package prints
  `[setup failed]` and exits 1. `cd $W && go list $U | wc -l` must be non-zero.
- [ ] **Step 3: Record the bare-tip roster.**
  ```bash
  cd $W && out=$(go test -count=1 -v $U 2>&1); rc=$?
  echo "$out" | /usr/bin/grep -oE '^=== RUN   [^ ]+' | sort > $S/base-run.txt
  echo "rc=$rc RUN=$(wc -l < $S/base-run.txt) FAIL=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')"
  ```
  Expected: `rc=0 RUN=391 FAIL=0`.
- [ ] **Step 4: Prove the panic gate live.** Put `func TestP102Gate(t *testing.T) { panic("p102-gate") }` in a
  throwaway `internal/bootstrap/zz_gate_test.go`. Run `go test -count=1 -run TestP102Gate ./internal/bootstrap/`
  and confirm `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` on its output reads ≥ 1. Delete the file and
  confirm `git -C $W status --short` prints nothing.
- [ ] **Step 5: Write `PROGRESS.md`** with a header (phase, worktree, base SHA) and a Task 1 entry quoting the
  commands and outputs of Steps 2-4.
- [ ] **Step 6: Commit.**
  `git -C $W add docs/envoy-go/phases/102-stats-flush-interval-bounds/PROGRESS.md && git -C $W commit -m "phase 102 (stats-flush-interval-bounds) IMPL task 1: baseline roster and a proven-live panic gate"`.

---

### Task 2: The 42 unit leaves — 29 RED at the un-fixed tip

**Files:**
- Create: `internal/bootstrap/stats_flush_interval_test.go` (Appendix B, 147 lines)
- Create: `cmd/envoy-go/stats_flush_interval_test.go` (Appendix C, 90 lines)

**Interfaces:**
- Consumes: `bootstrap.Load(io.Reader)`, the unexported `parseStatsSinks(*bootstrapv3.Bootstrap, *Bootstrap)
  error`, the existing test helper `statsBootstrap(string) string` and the const `statsdSinkType` (both in
  `bootstrap_test.go`); in `cmd/envoy-go`, `go build` of the package itself.
- Produces: top-level tests `TestStatsFlushInterval_YAMLReject`, `TestStatsFlushInterval_YAMLAccept`,
  `TestStatsFlushInterval_GoBuilt` (package `bootstrap`) and
  `TestEnvoyGoBinary_ModeValidate_StatsFlushInterval` (package `main`); helpers `assertFlushReject`,
  `flushBootstraps`, `flushPhrases`, consts `flushNegPhrase`, `flushOORPhrase`, `flushRangePhrase`,
  `flushStatsdSink`, `flushValidateYAML`. None collides with an existing identifier: check with
  `git -C $W grep -nE '\b(assertFlushReject|flushBootstraps|flushPhrases|flushNegPhrase|flushOORPhrase|flushRangePhrase|flushStatsdSink|flushValidateYAML)\b'`
  BEFORE Step 1, expecting nothing.

- [ ] **Step 1: Write both files from the appendices.**
  `ext B $W/$P > $W/internal/bootstrap/stats_flush_interval_test.go; ext C $W/$P > $W/cmd/envoy-go/stats_flush_interval_test.go`.
  `wc -l` ⇒ **147** and **90**.
- [ ] **Step 2: Compile and format.** `go vet ./internal/bootstrap/ ./cmd/envoy-go/` rc 0; `gofmt -l
  internal/bootstrap cmd/envoy-go` prints nothing.
- [ ] **Step 3: Run at the tip.**
  ```bash
  cd $W && go test -count=1 -v $U > $S/t2.log 2>&1; echo rc=$?
  /usr/bin/grep -c '^=== RUN' $S/t2.log; /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' $S/t2.log
  /usr/bin/grep -E '^ *--- FAIL: ' $S/t2.log | sed -E 's/^ *--- FAIL: //; s/ \(.*//' | /usr/bin/grep / | sort > $S/tip-red.txt; wc -l < $S/tip-red.txt
  ```
  Expected: `rc=1`, **437**, **37**, **29** RED leaves, exactly §4.1's RED set. The 27 bootstrap leaves fail
  on `got nil error, want one containing …`, and the binary's `0s` and `-1s` on `got err=<nil>, want
  *exec.ExitError with exit code 1`. **A RED for any other reason is a finding. Stop and diagnose.**
- [ ] **Step 4: Assert the 13 GREEN-by-design leaves** by name: `--- PASS` for all ten `YAMLAccept/*`, GoBuilt
  `0s_1000000ns` and `299s_999999999ns`, and binary `0.001s`. These are the regression gate; their falsifiers
  are NC7 and NC8 (Task 7).
- [ ] **Step 5: Commit** (pathspec: the two test files + `PROGRESS.md`, with the RED list pasted):
  `phase 102 (stats-flush-interval-bounds) IMPL task 2: 42 flush-interval leaves, 29 RED at the un-fixed tip`.

---

### Task 3: Fixtures `0127` and `0128` — drivers, READMEs, imports, and the FOUR registration gates

**Files:**
- Create: `test/fixtures/0127-stats-flush-interval-range-boot-reject/driver/driver.go` (Appendix E.1, 172 lines)
- Create: `test/fixtures/0127-stats-flush-interval-range-boot-reject/README.md` (Appendix E.2, 92 lines)
- Create: `test/fixtures/0128-stats-flush-interval-negative-boot-reject/driver/driver.go` (Appendix E.3, 175 lines)
- Create: `test/fixtures/0128-stats-flush-interval-negative-boot-reject/README.md` (Appendix E.4, 97 lines)
- Modify: `test/differential/runner_test.go` (Appendix E.5, `2 0`, after anchor **A6**)

**Interfaces:**
- Consumes: `fixture.RegisterFixture`, `fixture.Driver`, the harness's `BootRejectFixture`
  (`BootRejectScript() string`, `ExpectedBootErrorSubstring() string`), `helpers.HTTPGetReadyRaw` (the `0042`
  pattern).
- Produces: fixture names `0127-stats-flush-interval-range-boot-reject` (`0s`, needle `value must be inside
  range [1ms, 5m0s)`, ref port `15127`) and `0128-stats-flush-interval-negative-boot-reject` (`-0.5s`, needle
  `Invalid duration: Expected positive duration`, ref port `15128`), each byte-identical to its directory.
  Each has one listener `l_a` with a `direct_response` `222` **and an inline body** (§0.1), a `c_unused` STATIC
  cluster at `127.0.0.1:1`, and **no `stats_sinks`**.

- [ ] **Step 1: Re-census both ports at the IMPL tip.** `git -C $W grep -c '\b15127\b' -- test/ internal/ cmd/`
  and the same for `15128`: expect no output (zero files; capture it, `grep -c` on zero exits 1). Then
  `ss -tanH; ss -uanH` show no socket on either, in any state.
- [ ] **Step 2: Write the four files** from Appendices E.1-E.4 (`mkdir -p` both `driver/` dirs; `ext E.1 $W/$P >
  …/driver/driver.go`, and so on). `wc -l` must read 172 / 92 / 175 / 97.
- [ ] **Step 3: Add the two blank imports.** `ext E.5 $W/$P > $S/E5.diff; git -C $W apply --check $S/E5.diff &&
  git -C $W apply $S/E5.diff`. `git -C $W diff --numstat -- test/differential/runner_test.go` ⇒ `2	0`.
- [ ] **Step 4: Compile, vet, lint.** `go vet ./test/fixtures/0127-stats-flush-interval-range-boot-reject/...
  ./test/fixtures/0128-stats-flush-interval-negative-boot-reject/... ./test/differential/` rc 0; `gofmt -l` on
  both fixture dirs prints nothing; `GOTOOLCHAIN=go1.26.2 golangci-lint run` on the same three packages rc 0.
- [ ] **Step 5: Check the four gates by the SET, never by the exit code.** Gate 1 is `RegisterFixture` in
  `init()`, gate 2 the import, gate 3 name == directory byte for byte, gate 4 the `NNNN-` shape. Run the
  extractor with arguments in the order **(imports, dirs)**:
  ```bash
  extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
  extract $W/test/differential/runner_test.go > $S/imports.txt
  ls -d $W/test/fixtures/*/ | xargs -n1 basename | sort > $S/dirs.txt
  wc -l < $S/imports.txt; wc -l < $S/dirs.txt; comm -23 $S/imports.txt $S/dirs.txt; comm -13 $S/imports.txt $S/dirs.txt
  ```
  Expected: **130**, **130**, and both `comm` outputs empty. Each of the two new names must read 1 in each
  file (`/usr/bin/grep -c '^0127-stats-flush-interval-range-boot-reject$'`, and the same for `0128`).
- [ ] **Step 6: NC the extractor** in a SCRATCH COPY of `runner_test.go`. Renaming `0127`'s import token must
  fire BOTH `comm` directions. Deleting the `0128` import line fires `comm -13` only, under the (imports, dirs)
  order.
- [ ] **Step 7: Commit** (pathspec: the four fixture files + `runner_test.go` + `PROGRESS.md`):
  `phase 102 (stats-flush-interval-bounds) IMPL task 3: fixtures 0127 and 0128 and their four registration gates`.

---

### Task 4: RECORD the un-fixed tip — every falsifier RED, for its NAMED reason

**Files:**
- Modify: `PROGRESS.md` only

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: the pre-fix record that Task 6 compares against. **It cannot be recreated after Task 5.**

- [ ] **Step 1: Unit suite at the tip.** Task 2 Step 3's commands again. Expected **437 / 37 / rc 1**, the
  same 29 RED leaves (`cmp` against `$S/tip-red.txt`). Diff the sorted `=== RUN` roster against
  `$S/base-run.txt`: the additions must be exactly the 42 leaves and their 4 parents, with NO removal.
- [ ] **Step 2: Both fixtures at the tip, alone.**
  `cd $W && go test -count=1 -v -run "$FXSEL" ./test/differential/ > $S/fx-tip.log 2>&1; echo rc=${PIPESTATUS[0]}`.
  First assert `/usr/bin/grep -ac '^=== RUN   TestDifferential/' $S/fx-tip.log` ⇒ **2** and `-ac SKIP` ⇒ **0**
  (**`-a` is mandatory**, §0.7). Expected: rc 1, both `--- FAIL`, each with
  `BootRejectFixture: subject proxy started cleanly — expected boot rejection`. **A FAIL naming
  `direct_response.body is required` means the fixture shape regressed (§0.1). Stop.**
- [ ] **Step 3: The cite gate at the tip.** `ext F $W/$P > $S/cite-gate.sh; chmod +x $S/cite-gate.sh;
  $S/cite-gate.sh $W master` ⇒ `cite-gate: 27 cited lines checked, 0 moved`, exit 0. Record the count of
  cited lines (it is a census: if a later commit added a cite, the number moves; re-derive, do not inherit).
- [ ] **Step 4: Paste the failure lines into `PROGRESS.md` and commit:**
  `phase 102 (stats-flush-interval-bounds) IMPL task 4: the un-fixed tip recorded — 29 leaves RED, 0127 and 0128 RED at subject boot`.

---

### Task 5: THE PRODUCTION EDIT — Appendix A, under the cite gate

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` at anchors **A1-A5** (Appendix A, `21 10`)

**Interfaces:**
- Consumes: `bs.GetStatsFlushInterval()` (`*durationpb.Duration`), `GetSeconds() int64`, `GetNanos() int32`.
- Produces: the rule F1-F5; `Bootstrap.FlushInterval` is `5s` when absent and EXACTLY `secs·1s + nanos`
  otherwise, never ≤ 0. The three messages are the literals Appendix B's consts name, after the
  `bootstrap: stats_flush_interval: ` prefix.

- [ ] **Step 1: Apply.** `ext A $W/$P > $S/A.diff; git -C $W apply --check $S/A.diff && git -C $W apply $S/A.diff`.
  `git -C $W diff --numstat -- internal/bootstrap/bootstrap.go` ⇒ `21	10`.
- [ ] **Step 2: Assert the symbols, not the build** (method note 7). Anchor **A2** must read 0 hits
  (`git -C $W grep -c 'd.AsDuration(); v > 0' -- internal/bootstrap/bootstrap.go`, capture it). The three
  phrases must each read exactly 1 hit in `bootstrap.go`. `GetSeconds(), d.GetNanos()` must read 1.
- [ ] **Step 3: The hunk shape.** `git -C $W diff -U0 -- internal/bootstrap/bootstrap.go | /usr/bin/grep '^@@'`
  must show `-226,2 +226,2` and `-534,4 +534,4` (line-count-NEUTRAL), then `-789,2 +789,6`, `-800,2 +804,8`
  and `-802,0 +813`, all below the last cite (`:756`). Measured on P4 at `44b2de38`.
- [ ] **Step 4: The cite gate.** `$S/cite-gate.sh $W master` ⇒ `0 moved`, exit 0.
- [ ] **Step 5: Comments are clean.** `gofmt -l internal/bootstrap` prints nothing; `go vet ./internal/bootstrap/
  ./cmd/envoy-go/` rc 0; `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/bootstrap/ ./cmd/envoy-go/` rc 0.
  Sweep the new comments for British spellings: `git -C $W diff -- internal/bootstrap/bootstrap.go |
  /usr/bin/grep -iE '^\+.*(ise|isation|our\b|tre\b)'` — read every hit (`otherwise` is not one).
- [ ] **Step 6: Commit** (pathspec: `bootstrap.go` + `PROGRESS.md`):
  `phase 102 (stats-flush-interval-bounds) IMPL task 5: stats_flush_interval bounded to [1ms, 5m) in the reference's three phrases and order`.

---

### Task 6: ALL ARMS GREEN — unit, race, both fixtures ×3, one sink fixture

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Unit.** `go test -count=1 -v $U` ⇒ **437 RUN, 0 FAIL lines, rc 0**, panic gate 0. Every leaf
  of `$S/tip-red.txt` now reads `--- PASS`.
- [ ] **Step 2: Race.** `go test -count=1 -race $U` ⇒ rc 0, `DATA RACE` 0.
- [ ] **Step 3: The existing suite alone.** Move the two new test files to `$S`, run `go test -count=1 -v $U` ⇒
  **391 / 0**, and move them back; `git -C $W status --short` must show only the expected paths.
- [ ] **Step 4: Both fixtures, three solo runs.** `$FXSEL` three times, each asserting 2 `=== RUN` and 0 SKIP
  with `grep -a`, each rc 0 with both `--- PASS`. Record the wall times.
- [ ] **Step 5: A sink fixture still passes.** Run `TestDifferential/0092-stats-sink-statsd$` once (an in-range
  `0.5s` with a real sink; byte-untouched). Expected PASS. **A FAIL here is a regression of the in-range path.**
- [ ] **Step 6: Commit `PROGRESS.md`:**
  `phase 102 (stats-flush-interval-bounds) IMPL task 6: 437/0, race clean, 0127 and 0128 pass 3/3, 0092 green`.

---

### Task 7: The NC roster — nine mutants, each proven in place, scored PER ARM, unit AND fixture

**Files:**
- Modify: `PROGRESS.md` only (every mutant is reverted)

- [ ] **Step 1: Extract the applier and save the shipped file.** `ext D $W/$P > $S/nc.py; cp
  $W/internal/bootstrap/bootstrap.go $S/bootstrap.final.go`.
- [ ] **Step 2: Name each mechanism BEFORE running** (method note 7d): copy §4.3's mechanism column into
  `PROGRESS.md` first.
- [ ] **Step 3: Unit, per NC.** For `n` in 1-9:
  ```bash
  cp $S/bootstrap.final.go $W/internal/bootstrap/bootstrap.go; python3 $S/nc.py $n $W/internal/bootstrap/bootstrap.go
  m=$(/usr/bin/grep -c "NC$n" $W/internal/bootstrap/bootstrap.go)     # must be >= 1 (NC9: 2)
  cd $W && go test -count=1 -v $U > $S/nc$n.log 2>&1; rc=$?
  /usr/bin/grep -E '^ *--- FAIL: ' $S/nc$n.log | sed -E 's/^ *--- FAIL: //; s/ \(.*//' | sort > $S/nc$n.red
  cp $S/bootstrap.final.go $W/internal/bootstrap/bootstrap.go; /usr/bin/grep -c "NC$n" $W/internal/bootstrap/bootstrap.go   # must print 0
  ```
  **Assert the marker BEFORE reading the result** (method note 96). Each leaf set must equal §4.3's row
  exactly; every other leaf GREEN. NC4's mutant is not gofmt-clean; that is cosmetic.
- [ ] **Step 4: Fixture cells, per NC.** For `n` in 1, 3, 6, 7, 9 (and 2, 4, 5, 8 as the GREEN controls),
  apply as in Step 3 and run `$FXSEL` once. Each verdict must equal §4.4's row. Revert and re-assert 0 markers
  after each.
- [ ] **Step 5: Restore and prove it.** `cmp $W/internal/bootstrap/bootstrap.go $S/bootstrap.final.go` and
  `git -C $W status --short` ⇒ only `PROGRESS.md` modified.
- [ ] **Step 6: Commit `PROGRESS.md`:**
  `phase 102 (stats-flush-interval-bounds) IMPL task 7: nine NCs, each marker asserted, scored per arm at unit and fixture level`.

---

### Task 8: The occurrence set, re-derived at the IMPL tip — GROWTH only

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Run `SPEC.md` §4's census verbatim:**
  `git -C $W grep -n -i -E 'non-positive|parsed-and-inert|stats_flush_interval|D-MS-FLUSH' -- docs/ internal/ cmd/ test/ ':!docs/envoy-go/STATE_HISTORY.md' ':!docs/envoy-go/phases/'`.
  Diff its per-file counts against the same command at `44b2de38` (this PLAN's base; **168** there).
- [ ] **Step 2: Attribute every new hit.** Expected growth: the two new test files, the two fixture dirs, the
  rewritten `bootstrap.go` comments, ADR-0324's new text and the new ledger entry. **Any hit in any OTHER file is
  a finding.** Inherit `SPEC.md` §4's TABLE; use the census only to look for growth (method note 77).
- [ ] **Step 3: Confirm the falsified phrases are gone.** `non-positive` must read 0 in
  `internal/bootstrap/bootstrap.go` (capture it). Every surviving `parsed-and-inert` hit must carry the
  in-range qualifier.
- [ ] **Step 4: Commit `PROGRESS.md`:**
  `phase 102 (stats-flush-interval-bounds) IMPL task 8: occurrence set re-derived, growth attributed`.

---

### Task 9: `ADR-0324` completed IN PLACE — §Decision + §Consequences, ACCEPTED

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` at anchor **A7**

- [ ] **Step 1: Extract.** `ext G.1 $W/$P > $S/G1.md; ext G.2 $W/$P > $S/G2.md`; `wc -l` ⇒ 1 and 95.
- [ ] **Step 2: Flip the status line in place, and append after the retained footer.** Replace the ONE line
  starting `> **STATUS: PROPOSED — §Context drafted at the phase-102 SPEC` with `G1.md`'s line, then append
  `G2.md` after the footer line (A7), which is the file's last line. Add no `**Status:**` line, no renumber and
  no `---`. (The phase-101 IMPL's ADR-0323, `:19648`, is the shape precedent.)
- [ ] **Step 3: Re-derive every figure the text quotes, at this commit.** The `21 10`, `9 2`, 41 / 42 / 29 / 13
  leaves, the 22-vs-0 cite-gate figures and the NC counts (8, 2, 7, 3, 6, 8, 26, 4, 15) must match Tasks 2-7's
  MEASURED values. **Correct the text if they moved; do not keep a figure because this PLAN wrote it** (method
  note 97).
- [ ] **Step 4: The guard is now DISARMED; prove it by LINE and by ADR.** `^> \*\*STATUS: PROPOSED` reads
  zero hits, AND `sed -n` the ADR-0324 status line and READ `> **STATUS: ACCEPTED`, because an empty result
  also means a broken matcher. The ADR-0231 decoy still hits `:14866` → `## ADR-0231`, md5 `929719b67c87…`.
  `^---$` 216, `^## ADR-` 323, bare `^## ` 331 (the new `###` headings are not `^## `). Tail `ADR-0324`,
  next-free **ADR-0325**, TAIL-derived.
- [ ] **Step 5: Measure.** `git -C $W diff --numstat -- docs/envoy-go/DECISIONS.md` ⇒ `96	1` if the text is
  unchanged (19839 → 19934); quote what you MEASURE.
- [ ] **Step 6: Commit** (pathspec: `DECISIONS.md`):
  `phase 102 (stats-flush-interval-bounds) IMPL task 9: ADR-0324 §Decision + §Consequences, ACCEPTED`.

---

### Task 10: `BEHAVIOR_CONTRACT.md` — the inert phrase and the `+0` ledger entry

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` at anchors **A8**

- [ ] **Step 1: Rewrite the half-falsified phrase IN PLACE, leading with what survives** (method note 63).
  Replace the literal `` (a bare `stats_flush_interval` is parsed-and-inert) `` with
  `` (an in-range bare `stats_flush_interval` is parsed-and-inert; an out-of-range one is a boot reject, ADR-0324) ``.
  The rest of the line (including its stale `91-dir` figure, §1.2) is byte-untouched.
- [ ] **Step 2: Append the ledger entry** as its own paragraph, one blank line after the
  `**Phase 101 — +0, UNCHANGED (` entry: `ext H $W/$P` gives it verbatim (one line).
- [ ] **Step 3: Check the shape.** `/usr/bin/grep -nE '→|->'` over the new entry finds no `A → B` absolute.
  `git -C $W grep -c 'a bare `stats_flush_interval` is parsed-and-inert' -- docs/envoy-go/BEHAVIOR_CONTRACT.md`
  reads nothing, the new phrase reads 1. Expect `wc -l` **6002 → 6004** and numstat `3	1`; quote what you
  MEASURE.
- [ ] **Step 4: Commit** (pathspec: the contract):
  `phase 102 (stats-flush-interval-bounds) IMPL task 10: contract — an out-of-range bare stats_flush_interval is a boot reject; ledger +0 (delta only)`.

---

### Task 11: `ROADMAP.md` — row 102 → `done`, under the FIELD-COUNT gate

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md`, row **A9** only

- [ ] **Step 1: Write the new cell in a SCRATCH copy first.** Append to the existing cell an
  `**IMPL done <date>**` paragraph in the row-101 form: `done`, ADR-0324 `ACCEPTED`, `bootstrap.go`'s
  numstat, the two new test files, fixtures `0127` / `0128` (+ the import), the 29 tip-RED leaves and both
  fixtures RED at subject boot, the NC outcome, the `+0` ledger, and what was not bought. **No Go `||` and no
  other pipe**: write "or" for the rule's disjunctions; reword any pipe AWAY rather than escaping it.
- [ ] **Step 2: Count its fields under BOTH forms.** Naive: `awk -F'|' '/^\| *102 /{print NF}'`.
  Escape-aware: `sed 's/\\|//g' F | awk -F'|' '/^\| *102 /{print NF}'`, with **no file argument to awk**.
  Both must read **8**. Flip field 5 to `done`.
- [ ] **Step 3: Assert the cell spells NEITHER sentinel match phrase** (`deferred candidates:` or
  `remaining deferred (not-yet-chartered) candidates:`), nor the bare word `deferred` case-insensitively,
  nor `-family row` (with `--`). All must read 0 in the cell.
- [ ] **Step 4: Install it and re-run the full sentinel** (§2.1's commands, verbatim from `next-prompt.txt`).
  Expected, measured on both sides of the flip:
  - check (1) goes ONE → **SILENT**, and NC-A and NC-B go TWO → **ONE**;
  - `want` stays **134** and `ROADMAP.md` stays **252**;
  - the six windows keep their md5s;
  - the malformed set stays {57, 69}, and NC-D stays 96 / 68.
- [ ] **Step 5: Commit** (pathspec: `ROADMAP.md`): `phase 102 (stats-flush-interval-bounds) IMPL task 11: row 102 done`.

---

### Task 12: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Byte-untouched roster (§1.2).** `git -C $W diff master --numstat -- <each path>` must be EMPTY
  for every path. The edit roster's paths must be exactly §1.2's; list any path on neither.
- [ ] **Step 2: ARM roster (method note 28).** Diff the sorted `=== RUN` roster of `$U` against
  `$S/base-run.txt`. It gains exactly the 42 leaves and 4 parents and loses NOTHING.
- [ ] **Step 3: Gate (a), the full differential**, with `-count=1`: assert the fixture set BY NAME in BOTH
  `comm` directions (**130 = 130**), count PASS / FAIL / SKIP with `grep -a`, never trust the exit code alone.
  **A port-race abort is MASKING**: record it and rerun, but a green rerun clears nothing.
- [ ] **Step 4: Gate (b), the non-Docker sweep:**
  `go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'`, gated on `PIPESTATUS[0]`
  plus a SET RECONCILIATION of `ok` / `FAIL` / `[no test files]` against the package count. The count rises by
  the two fixture `driver` packages (243 → 245 expected); measure it rather than predict it. Keep every sweep
  on the record.
- [ ] **Step 5: Gate (c)**, h2spec, run verbose. Expected: `95 tests, 94 passed, 1 skipped, 0 failed`.
- [ ] **Step 6: Gate (d)**, the fuzzers: `git grep -c '^func Fuzz' -- '*.go'`. The row adds none, so expect 56
  targets across 48 FILES; name both units. Run `FuzzBootstrapLoad` and `FuzzStatsSinkConfigParse` for 20 s
  each (the two that reach `parseStatsSinks`).
- [ ] **Step 7: Gate (e)**, the anchored panic gate: **0**, proven live at Task 1. Then
  `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` (rc 0), with a COMPILING planted control (§0.9), e.g.
  `func zzPlant() int { os.Remove("/x"); x := 1; x = 2; return x }` plus `var _ = zzPlant`, which must fire
  `errcheck` and `ineffassign`; `go build` it first to prove it compiles; remove it after.
- [ ] **Step 8: Gate (f): no `REVIEW.md`.** That is the ONE standing departure (none of 93-101 has one). Name it;
  do not claim compliance.
- [ ] **Step 9: Commit `PROGRESS.md`:**
  `phase 102 (stats-flush-interval-bounds) IMPL task 12: roster, arm roster and six-gate sweep recorded`.

---

### Task 13: `PROGRESS.md` close-out and the router/state roll

**Files:**
- Modify: `PROGRESS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`

- [ ] **Step 1: Close `PROGRESS.md`:** a per-task summary, and every refutation this IMPL made (method note 2).
  **Re-derive every figure that a governing document quotes, at this LAST commit** (method note 97): the
  ADR-0324 text, the contract entry and the row-102 cell included.
- [ ] **Step 2: Roll `STATE.md` IN PLACE:** the §Current pointer, lifecycle-state → DONE, next-free ADR-0325.
  Evict the oldest §Recent entry:
  - **MEASURE the pre-roll date histogram, INCLUDING the entry this close promotes;**
  - verify the eviction with the LABEL-BOUND PAIR on BOTH files;
  - add a fabricated-label NC and a positive control on an ARCHIVED label.
- [ ] **Step 3: Archive the evictee as ONE inline parenthetical line.** Expected: strict guard DELTA 0 and raw
  delta **+2**, naming the LABEL and no figure.
- [ ] **Step 4: Roll `next-prompt.txt`.** The sentinel re-evaluates: with row 102 `done`, check (1) is SILENT
  and check (2) still SIX, so the loop SELF-PICKS. Grep the result for `YOUR STAGE`, for `IMPL`, and for every
  figure this row moved (`want`, the fixture set 130, gate (b)'s package count, the guard polarity
  (DISARMED), the ledger chain, the port bands `15127` / `15128` now TAKEN) (method note 68). Stage it with
  `git add -f next-prompt.txt`.
- [ ] **Step 5: Commit** the four files:
  `phase 102 (stats-flush-interval-bounds) IMPL task 13: close-out, state and router rolled`.

---

### Task 14: Squash, merge, push, and remove the worktree

- [ ] **Step 1: Squash the branch to ONE commit.** Its subject carries the FULL SLUG and the stage word:
  `phase 102 (stats-flush-interval-bounds) IMPL: …`.
- [ ] **Step 2: Merge to master fast-forward.** Re-run the sentinel in the MERGED tree; it must match Task 11.
- [ ] **Step 3: Push** (`feedback_push_to_origin`), with the repo's configured email.
- [ ] **Step 4: `git worktree remove` the IMPL worktree.** `git worktree list` must then read master only.
  Check the repo root with `git status --short --untracked-files=all`, and ignore the pre-existing `.claude/`.

---

## 6. The cite gate — BUILT, AND SHOWN TO FIRE

`SPEC.md` placed no constraint on `bootstrap.go`'s layout. This PLAN adds one (§0.2), because 27 distinct
`bootstrap.go:<line>` citations live in `internal/`, `cmd/`, `test/`, `BEHAVIOR_CONTRACT.md` and
`DECISIONS.md`: `:68 :74 :134 :156 :164`, then 22 from `:250` to `:756`. `cite-gate.sh <worktree> [base]` (Appendix F) reads every such cite at
the BASE revision, then compares each cited line's bytes at the base against the worktree. Its exit status is
the number of moved lines.

| tree | output | exit |
|---|---|---|
| TIP (base `44b2de38`) | `27 cited lines checked, 0 moved` | 0 |
| P3 (P2 + line-ADDING comment edits) | `27 cited lines checked, 22 moved` (every cite from `:250` to `:756`) | 22 |
| **P4 (Appendix A)** | `27 cited lines checked, 0 moved` | **0** |

**The gate fires** (P3), and the chosen shape passes it. It covers only the cite set at the base: a cite added
by the IMPL itself is outside it, and Appendix G names no line number for that reason. ⚠️ **`docs/envoy-go/phases/`
is excluded by design** (closed documents, evidence; their cites were true when written).

---

## 7. The negative-control roster — `SPEC.md` §7, CONFIRMED with one hygiene change

`SPEC.md` §7's nine rows are inherited unchanged; §4.3 is the measured unit table, and §4.4 the fixture table.
The applier (Appendix D) is byte-identical to `SPEC.md` Appendix D. Its patterns touch code lines only, so it
applies to P4 as it did to P2. The one change is procedural: **NC9's marker count is 2** (two mutated lines),
so the gate is "≥ 1", never "== 1". No row is vacuous. The roster has no `AsDuration` row: restoring
`AsDuration` IS the tip, which §4.1's TIP column already scores (29 RED).

---

## 8. Counts — this PLAN's own tip

Each figure was produced by running its command in the tree that ships this file, after the last edit (method
note 97).

- `DECISIONS.md` **19839**, `^---$` **216**, `^## ADR-` **323**, bare `^## ` **331**, tail ADR-0324, next-free
  ADR-0325, house guard ARMED at `:19825` → `## ADR-0324`: all **UNMOVED** (a PLAN does not touch it).
- `ROADMAP.md` **252** / `want` **134** / row 102 `in-progress` at `:164`: **UNMOVED**.
- `BEHAVIOR_CONTRACT.md` **6002**: **UNMOVED**.
- Fixture set **128 = 128**: **UNMOVED** (`0127` / `0128` land at the IMPL).
- **Gate figures: NOT RUN (a PLAN's scope, not an omission).** Carried from the phase-101 IMPL: differential
  128/0/0, gate (b) 243 packages, h2spec `95 tests, 94 passed, 1 skipped, 0 failed`, fuzzers 56 targets / 48
  files, anchored panic gate 0, `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` rc 0.
- **File scope: FOUR paths** — this `PLAN.md`, `STATE.md`, `STATE_HISTORY.md` `2 0` and `next-prompt.txt`
  (§1.1; the commit's own `--numstat` is in its close-out record).

---

## 9. Deferred — carried, none chartered

- **NC1's and NC3's sink-split observables** (§0.4, §0.5): with a sink, `ready` then a `NewTicker` panic, which
  the harness reads as a clean boot; without one, a silent boot. Moot once the row lands, because the bound
  makes both unreachable. Recorded so that a future change near `Flusher.Start` or the harness's readiness
  probe knows that a post-`ready` panic is invisible to `runBootRejectFixture`.
- **The harness itself**: `tryStartSubjectProxy` returning on `ready` means NO boot-reject fixture can catch a
  late crash. A harness item, not a row.
- **Everything `SPEC.md` §1 excludes** (whole-message parity, sub-ms cadence, the OOR directory, the nanos
  reference arm, `stats_flush_on_admin`, `Pipeline.Run`).
- **`BEHAVIOR_CONTRACT.md`'s `91-dir` figure** on the inert-phrase line (§1.2): stale, not this row's claim.

---

## 10. `SPEC.md` §12 coverage, and self-review

### 10.1 Coverage of the seven owed items

| `SPEC.md` §12 item | discharged at |
|---|---|
| 1 build and run every embedded block, incl. the two half-falsified comment edits; re-measure the numstat | §0.2, §0.3, §4, Appendices A-F; `21 10` |
| 2 measure every §6.4 cell (tip, P2, NC1/3/6/7/9) with the fixture set asserted by name | §0.6, §4.4 (all nine NCs, not five), §2.3 |
| 3 re-census `15127` / `15128`; re-derive the occurrence set | §2.4; §0.10 (growth = the SPEC's own ADR draft + `STATE.md`) |
| 4 the NC roster as patches or Appendix D, each marker asserted | §4.3, §7, Appendix D, Task 7 |
| 5 the doc edits written out | Appendices G-H, applied once and measured (§0.12), Tasks 9-10 |
| 6 the un-fixed-tip capture as the IMPL's first tasks | Tasks 2-4 (29 RED named, 13 GREEN by design, both fixtures RED at subject boot) |
| 7 misspell in locale US | Global Constraints, Task 5 Step 5, Task 12 Step 7; Appendices A-C, E.1, E.3 read clean under lint |

The router's three "still unmeasured" items: every `0127` / `0128` cell (§4.4), how the harness treats
`ready`-then-panic (§0.4), and NC3's `-1s` panic at a real boot (§0.5). All three are MEASURED. The fourth,
the reference's nanos-overflow handling, stays SOURCE-READ: it is unreachable through config (`SPEC.md` §1).

### 10.2 The appendices, re-extracted and applied in IMPL order

At this PLAN's close, every appendix was extracted from THIS file with Global Constraints' `ext` into a fresh
detached worktree off `44b2de38`, and applied in task order (B, C, E.1-E.5, then A, then G, then H). Results:

- **Extraction:** all 13 blocks (A, B, C, D, E.1-E.5, F, G.1, G.2, H) extracted `cmp`-identical to the files
  that ran, at 64 / 147 / 90 / 27 / 172 / 92 / 175 / 97 / 13 / 13 / 1 / 95 / 1 lines.
- **Tests + fixtures before A** (the un-fixed tip): `$U` read **437 RUN / 37 FAIL lines / rc 1**; the cite gate
  read `27 cited lines checked, 0 moved`.
- **A applied** (`git apply --check` clean): `21	10`, `cmp`-identical to P4; cite gate **0 moved**; `$U` read
  **437 / 0 / rc 0**, panic gate 0.
- **`$FXSEL` on that tree:** 2 `=== RUN`, 0 SKIP, rc 0, both PASS (`0127` 2.34 s, `0128` 1.66 s). Under NC6
  (marker 1, applied with D): rc 1, `0127` PASS, `0128` FAIL; reverted to A, marker 0, `cmp` clean.
- **G and H applied as Tasks 9-10 prescribe:** `DECISIONS.md` `96	1`, `BEHAVIOR_CONTRACT.md` `3	1`; the house
  guard read 0 and `:19825` read `> **STATUS: ACCEPTED`.
- **The whole IMPL diff, minus `ROADMAP.md` and the close-out files:** **895 / 12 across 10 paths**
  (`git add -N` + `git diff --numstat 44b2de38`).

The worktree was then removed.

### 10.3 Self-review

- **Sub-step counts, by command** (`awk` over `^- \[ \] ` per `^### Task `): Task 1 6 · 2 5 · 3 7 · 4 4 · 5 6 · 6 6 · 7 6 ·
  8 4 · 9 6 · 10 4 · 11 5 · 12 9 · 13 5 · 14 4. The largest is 9 (Task 12), under §6.1's ~10 line. 14 tasks.
- **Placeholder scan:** `/usr/bin/grep -nE 'TBD|TODO|implement later|fill in details|Similar to Task|appropriate error handling'`
  over this file reads exactly ONE line, this sentence itself (it spells the pattern; method note 72).
- **Type and name consistency:** the Interfaces blocks name `parseStatsSinks(*bootstrapv3.Bootstrap,
  *Bootstrap) error`, `Bootstrap.FlushInterval time.Duration`, the three phrase consts and the two fixture
  names; each is spelled identically in Appendices A-E (checked by `git grep` in the §10.2 worktree).
- **Review Focus:** each of its five lines names the test that pins it and the NC that falsifies it.

---

# Appendices — the MEASURED artifacts (built, run and reverted at this PLAN stage)

Every block below is byte-identical (`cmp`) to the file that ran in §4 and §0.12. Line counts are in each
heading; check them after extraction.

## Appendix A — `internal/bootstrap/bootstrap.go` (`21 10`: P2's code `9 2` byte-identical, plus four comment sites; the two above `parseStatsSinks` line-count-neutral). Task 5

```diff
diff --git a/internal/bootstrap/bootstrap.go b/internal/bootstrap/bootstrap.go
index 2ea7189a..36319353 100644
--- a/internal/bootstrap/bootstrap.go
+++ b/internal/bootstrap/bootstrap.go
@@ -223,8 +223,8 @@ const (
 	// as flush-every-entry and is NOT coerced to this default.
 	alsDefaultBufferSizeBytes uint32 = 16384
 
-	// statsFlushIntervalDefault is the stats_flush_interval default (5s) applied
-	// when the field is absent or non-positive (ADR-0262 / D-MS-FLUSH).
+	// statsFlushIntervalDefault is the stats_flush_interval applied when the field
+	// is absent (ADR-0262 / D-MS-FLUSH); a present value is bounded (ADR-0324).
 	statsFlushIntervalDefault = 5 * time.Second
 )
 
@@ -531,10 +531,10 @@ type Bootstrap struct {
 	// OTLPMetricsSink is built in cmd/envoy-go/main.go after Load returns (and
 	// ONLY when this slice is non-empty).
 	OTLPSinkConfigs []OTLPSinkConfig
-	// FlushInterval is the stats_flush_interval (default 5s when absent/non-positive
-	// — D-MS-FLUSH). Parsed-and-stored even when no stats_sinks[] entry exists
-	// (inert in that case — D-MS-FLUSH-INERT); the Flusher uses it only when
-	// StatsSinkConfigs is non-empty.
+	// FlushInterval is the stats_flush_interval (default 5s when absent — D-MS-FLUSH;
+	// a present value outside [1ms, 5m) is rejected at Load, ADR-0324). An in-range
+	// value is stored even with no stats_sinks[] entry (inert — D-MS-FLUSH-INERT);
+	// the Flusher uses it only when StatsSinkConfigs is non-empty.
 	FlushInterval time.Duration
 	// ConfigPath is the file path the bootstrap was loaded from. Set by the
 	// caller (cmd/envoy-go/main.go) post-Load via bs.ConfigPath = *cfgPath;
@@ -786,8 +786,12 @@ func checkStaticLayerValues(fields map[string]*structpb.Value) error {
 }
 
 // parseStatsSinks reads the top-level stats_flush_interval (default 5s) and the
-// stats_sinks[] entries (ADR-0262). stats_flush_interval is ALWAYS stored on
-// result.FlushInterval, even with no stats_sinks[] (inert — D-MS-FLUSH-INERT).
+// stats_sinks[] entries (ADR-0262). An in-range stats_flush_interval is ALWAYS
+// stored on result.FlushInterval, even with no stats_sinks[] (inert —
+// D-MS-FLUSH-INERT); any other present value is rejected, with or without a
+// sink, in the reference's three phrases and in its order: a negative field,
+// then seconds above 9223372035 or nanos above 999999999, then a total outside
+// [1ms, 5m) (ADR-0324).
 // stats_flush_on_admin is STRICT-REJECTED (ADR-0080; envoy-go ships only the
 // periodic stats_flush_interval loop). Each stats_sinks[] entry must carry one
 // of the five supported typed_config types (metrics_service, statsd,
@@ -797,9 +801,16 @@ func checkStaticLayerValues(fields map[string]*structpb.Value) error {
 func parseStatsSinks(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
 	result.FlushInterval = statsFlushIntervalDefault
 	if d := bs.GetStatsFlushInterval(); d != nil {
-		if v := d.AsDuration(); v > 0 {
-			result.FlushInterval = v
+		secs, nanos := d.GetSeconds(), d.GetNanos()
+		switch {
+		case secs < 0 || nanos < 0:
+			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Expected positive duration")
+		case secs > 9223372035 || nanos > 999999999:
+			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Duration out-of-range")
+		case secs >= 300 || (secs == 0 && nanos < 1000000):
+			return fmt.Errorf("bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)")
 		}
+		result.FlushInterval = time.Duration(secs)*time.Second + time.Duration(nanos)
 	}
 	if bs.GetStatsFlushOnAdmin() {
 		return fmt.Errorf("bootstrap: stats_flush_on_admin is not supported (envoy-go ships only the periodic stats_flush_interval sink loop)")
```

## Appendix B — `internal/bootstrap/stats_flush_interval_test.go` (147 lines, new: 41 leaves, 3 parents; `SPEC.md` Appendix B minus three redundant loop copies). Task 2

```go
package bootstrap

import (
	"math"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// The three reference rejection classes for stats_flush_interval (phase 102).
const (
	flushNegPhrase   = "Invalid duration: Expected positive duration"
	flushOORPhrase   = "Invalid duration: Duration out-of-range"
	flushRangePhrase = "value must be inside range [1ms, 5m0s)"
)

var flushPhrases = []string{flushNegPhrase, flushOORPhrase, flushRangePhrase}

// flushStatsdSink is one statsd stats_sinks[] entry (bootstrap B1).
const flushStatsdSink = `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
`

// flushBootstraps are B0 (no sink) and B1 (one statsd sink): the reference
// refuses an out-of-range value with or without a sink.
var flushBootstraps = []struct{ name, sinks string }{
	{"B0_no_sink", ""},
	{"B1_statsd_sink", flushStatsdSink},
}

// assertFlushReject asserts err carries exactly one class phrase (want) and
// the "bootstrap: " prefix FuzzBootstrapLoad requires.
func assertFlushReject(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one containing %q", want)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "bootstrap: ") {
		t.Errorf("error %q lacks the %q prefix", msg, "bootstrap: ")
	}
	if !strings.Contains(msg, want) {
		t.Errorf("error %q does not contain %q", msg, want)
	}
	for _, other := range flushPhrases {
		if other != want && strings.Contains(msg, other) {
			t.Errorf("error %q also contains %q (want a single cause)", msg, other)
		}
	}
}

func TestStatsFlushInterval_YAMLReject(t *testing.T) {
	cases := []struct {
		value, want string
	}{
		{"0s", flushRangePhrase},
		{"0.0005s", flushRangePhrase},
		{"0.000999999s", flushRangePhrase},
		{"300s", flushRangePhrase},
		{"1000s", flushRangePhrase},
		{"-1s", flushNegPhrase},
		{"-0.5s", flushNegPhrase},
		{"9223372036s", flushOORPhrase},
		{"315576000000s", flushOORPhrase},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			t.Run(c.value+"/"+b.name, func(t *testing.T) {
				src := statsBootstrap("stats_flush_interval: " + c.value + "\n" + b.sinks)
				_, err := Load(strings.NewReader(src))
				assertFlushReject(t, err, c.want)
			})
		}
	}
}

func TestStatsFlushInterval_YAMLAccept(t *testing.T) {
	cases := []struct {
		name, line string
		want       time.Duration
	}{
		{"absent", "", 5 * time.Second},
		{"0.001s", "stats_flush_interval: 0.001s\n", time.Millisecond},
		{"0.0015s", "stats_flush_interval: 0.0015s\n", 1500 * time.Microsecond},
		{"0.5s", "stats_flush_interval: 0.5s\n", 500 * time.Millisecond},
		{"299.999999999s", "stats_flush_interval: 299.999999999s\n", 299*time.Second + 999999999},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			t.Run(c.name+"/"+b.name, func(t *testing.T) {
				bs, err := Load(strings.NewReader(statsBootstrap(c.line + b.sinks)))
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if bs.FlushInterval != c.want {
					t.Errorf("FlushInterval: got %v, want %v", bs.FlushInterval, c.want)
				}
			})
		}
	}
}

func TestStatsFlushInterval_GoBuilt(t *testing.T) {
	cases := []struct {
		name   string
		secs   int64
		nanos  int32
		reject string        // class phrase; "" means accept
		want   time.Duration // FlushInterval on accept
	}{
		{"0s_999999ns", 0, 999999, flushRangePhrase, 0},
		{"0s_1000000ns", 0, 1000000, "", time.Millisecond},
		{"299s_999999999ns", 299, 999999999, "", 299*time.Second + 999999999},
		{"300s_0ns", 300, 0, flushRangePhrase, 0},
		{"1s_-1ns", 1, -1, flushNegPhrase, 0},
		{"-1s_0ns", -1, 0, flushNegPhrase, 0},
		{"0s_1000000000ns", 0, 1000000000, flushOORPhrase, 0},
		{"1s_2147483647ns", 1, 2147483647, flushOORPhrase, 0},
		{"MaxInt64s_0ns", math.MaxInt64, 0, flushOORPhrase, 0},
		{"9223372035s_0ns", 9223372035, 0, flushRangePhrase, 0},
		{"9223372036s_0ns", 9223372036, 0, flushOORPhrase, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bs := &bootstrapv3.Bootstrap{StatsFlushInterval: &durationpb.Duration{Seconds: c.secs, Nanos: c.nanos}}
			result := &Bootstrap{}
			err := parseStatsSinks(bs, result)
			if c.reject != "" {
				assertFlushReject(t, err, c.reject)
				return
			}
			if err != nil {
				t.Fatalf("parseStatsSinks: %v", err)
			}
			if result.FlushInterval != c.want {
				t.Errorf("FlushInterval: got %v, want %v", result.FlushInterval, c.want)
			}
		})
	}
}
```

## Appendix C — `cmd/envoy-go/stats_flush_interval_test.go` (90 lines, new: 3 leaves, 1 parent; `SPEC.md` Appendix C minus one redundant loop copy). Task 2

```go
package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// flushValidateYAML is the TestEnvoyGoBinary_ModeValidate good config, with a
// stats_flush_interval line appended (phase 102).
const flushValidateYAML = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

// TestEnvoyGoBinary_ModeValidate_StatsFlushInterval runs --mode validate on
// the built binary: 0s and -1s exit 1 naming their reference class; 0.001s
// (the lower bound) validates.
func TestEnvoyGoBinary_ModeValidate_StatsFlushInterval(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cases := []struct {
		value, wantErr string // wantErr "" means exit 0 + "configuration OK"
	}{
		{"0s", "value must be inside range [1ms, 5m0s)"},
		{"-1s", "Invalid duration: Expected positive duration"},
		{"0.001s", ""},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := os.WriteFile(cfg, []byte(flushValidateYAML+"stats_flush_interval: "+c.value+"\n"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, bin, "-c", cfg, "--mode", "validate").CombinedOutput()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("--mode validate failed: %v\n%s", err, out)
				}
				if !strings.Contains(string(out), "configuration OK") {
					t.Errorf("stdout = %q, want it to contain %q", out, "configuration OK")
				}
				return
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("got err=%v, want *exec.ExitError with exit code 1 (out=%s)", err, out)
			}
			if !strings.Contains(string(out), c.wantErr) {
				t.Errorf("output = %q, want it to contain %q", out, c.wantErr)
			}
		})
	}
}
```

## Appendix D — the NC applier, `python3 nc.py <n> internal/bootstrap/bootstrap.go` (27 lines; byte-identical to `SPEC.md` Appendix D; each mutant carries `// NC<n>`). Task 7

```python
import sys
n=sys.argv[1]; p=sys.argv[2]
s=open(p).read()
NEG='''		case secs < 0 || nanos < 0:
			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Expected positive duration")
'''
OOR='''		case secs > 9223372035 || nanos > 999999999:
			return fmt.Errorf("bootstrap: stats_flush_interval: Invalid duration: Duration out-of-range")
'''
RNG='''		case secs >= 300 || (secs == 0 && nanos < 1000000):
			return fmt.Errorf("bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)")
'''
M={
'1':[('		case secs >= 300 || (secs == 0 && nanos < 1000000):\n','		case secs >= 300: // NC1\n')],
'2':[('		case secs > 9223372035 || nanos > 999999999:\n','		case secs > 9223372035: // NC2\n')],
'3':[(NEG,'		// NC3 NEG case deleted\n')],
'4':[('case secs >= 300 ||','case secs > 300 || // NC4\n\t\t\t')],
'5':[('		case secs > 9223372035 || nanos > 999999999:\n','		case nanos > 999999999: // NC5\n')],
'6':[(NEG+OOR+RNG, RNG.replace(':\n',': // NC6\n',1)+NEG+OOR)],
'7':[('	if d := bs.GetStatsFlushInterval(); d != nil {\n','	if d := bs.GetStatsFlushInterval(); d != nil && len(bs.GetStatsSinks()) > 0 { // NC7\n')],
'8':[('(secs == 0 && nanos < 1000000):\n','(secs == 0 && nanos <= 1000000): // NC8\n')],
'9':[('Invalid duration: Expected positive duration")\n','value must be inside range [1ms, 5m0s)") // NC9\n'),('Invalid duration: Duration out-of-range")\n','value must be inside range [1ms, 5m0s)") // NC9\n')],
}
for a,b in M[n]:
    assert s.count(a)==1,(n,a)
    s=s.replace(a,b)
open(p,'w').write(s)
```

## Appendix E.1 — `test/fixtures/0127-stats-flush-interval-range-boot-reject/driver/driver.go` (172 lines). Task 3

```go
// Package driver registers the 0127-stats-flush-interval-range-boot-reject
// BOOT-REJECT differential fixture with the runner per phase 102 SPEC §6. It
// is a SYMMETRIC boot-reject fixture: a top-level bootstrap
// `stats_flush_interval: 0s` MUST fail-closed at config-load on BOTH reference
// Envoy v1.37.2 AND envoy-go, and BOTH stderr buffers must contain the
// reference's range phrase.
//
// # Reject arm
//
// `0s` is below the reference's `[1ms, 5m)` range on the field
// (`(validate.rules).duration = {lt: 300s, gte: 1ms}`). It is the
// operator-visible silent fold: before phase 102 envoy-go folded every
// non-positive value to the 5 s default and booted. The bootstrap carries NO
// `stats_sinks`, so the claim is sink-independent: an envoy-go that only
// checks the interval when a sink exists still boots and fails this fixture.
//
// # Common boot-reject substring
//
//   - reference Envoy v1.37.2 stderr: "... StatsFlushInterval: value must be
//     inside range [1ms, 5m0s) ..."
//   - envoy-go stderr: "load config: bootstrap: stats_flush_interval: value
//     must be inside range [1ms, 5m0s)"
//
// The needle stops before the reference's `: goo.gle/debug...` token and does
// not contain `bootstrap:`, which the reference never prints.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
// with an HCM direct_response (status 222, inline body: envoy-go refuses a
// direct_response without a body, so a body-less route would boot-reject the
// subject for an unrelated reason) and a minimal c_unused cluster
// (127.0.0.1:1, never dialed). Neither is reached: the reject fires while the
// bootstrap is loaded, before any listener binds.
//
// # Cross-references
//
//   - phase 102 SPEC §6 (two boot-reject directories) and §6.4 (NC cells)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0128 (the negative-duration sibling)
//   - fixture-0042 (the BootRejectFixture shape this copies)
package driver

import (
	"context"
	"fmt"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0127-stats-flush-interval-range-boot-reject"

	refAdminPort = 9901
	refPort      = 15127 // l_a — the single boot-reject listener (ref container port).

	// flushInterval is the rejected top-level stats_flush_interval value.
	flushInterval = "0s"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr.
	expectedBootErrorSubstr = "value must be inside range [1ms, 5m0s)"
)

func init() {
	fixture.RegisterFixture(fixtureName, &flushRangeBootRejectDriver{})
}

type flushRangeBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*flushRangeBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; the backend is unused by the boot-reject path
func (*flushRangeBootRejectDriver) SubjectListenerName() string { return "l_a" }
func (*flushRangeBootRejectDriver) ReferenceListenerPort() int  { return refPort }

// ReferenceBootstrap renders the self-contained boot-reject bootstrap.
func (*flushRangeBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*flushRangeBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch.

func (*flushRangeBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushRangeBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushRangeBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err := helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- harness.BootRejectFixture ---

// BootRejectScript returns "": the trigger is inline in the bootstrap.
func (*flushRangeBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the substring asserted in BOTH stderrs.
func (*flushRangeBootRejectDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// renderBootRejectBootstrap returns the bootstrap BOTH proxies consume. The
// top-level stats_flush_interval is the only rejected field; there is no
// stats_sinks entry.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

stats_flush_interval: %s

static_resources:
  listeners:
    - name: l_a
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: l_a
                route_config:
                  name: r_a
                  virtual_hosts:
                    - name: vh_a
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 222
                            body: { inline_string: l_a }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, flushInterval, listenerPort)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver = (*flushRangeBootRejectDriver)(nil)
)
```

## Appendix E.2 — `test/fixtures/0127-stats-flush-interval-range-boot-reject/README.md` (92 lines). Task 3

```markdown
# Fixture 0127 — stats-flush-interval-range-boot-reject

BOOT-REJECT differential fixture for the bootstrap's top-level
`stats_flush_interval` (phase 102). Asserts that BOTH reference Envoy
v1.37.2 AND envoy-go fail-close at config-load when the interval is `0s`,
outside the reference's `[1ms, 5m)` range on the field, with NO
`stats_sinks` configured.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `value must be inside range [1ms, 5m0s)` (case-sensitive substring).

## Boot-reject trigger (reject arm)

`stats_flush_interval: 0s` at the top level of the bootstrap.

`0s` was chosen because it is the operator-visible silent fold: before
phase 102 envoy-go folded every non-positive value to the 5 s default and
booted, so an operator who wrote `0s` got a 5 s flush and no error, where
the reference refuses to start. The bootstrap has no `stats_sinks`, which
makes the claim sink-independent: an envoy-go that only checked the
interval when a sink exists would still boot here and fail this fixture.

## Stderr comparison

Measured against the dockerized reference `envoyproxy/envoy:contrib-v1.37.2`,
run by digest (`docs/envoy-go/ENVOY_TARGET.md`).

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `Proto constraint validation failed (BootstrapValidationError.StatsFlushInterval: value must be inside range [1ms, 5m0s))` |
| envoy-go | `load config: bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)` |

**Common substring:** `value must be inside range [1ms, 5m0s)`. envoy-go
mirrors the reference's range phrase. The needle stops before the
reference's `: goo.gle/debug...` token and does not contain `bootstrap:`,
which the reference never prints.

## Bootstrap discipline

Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
with an HCM `direct_response` (status `222`, inline body) and a minimal
`c_unused` STATIC cluster (`127.0.0.1:1`, never dialed). The body is
required: envoy-go refuses a `direct_response` without one
(`hcm: route 0: direct_response.body is required`), so a body-less route
would reject the subject for an unrelated reason. With a valid `1s`
interval the same bootstrap validates on the reference
(`--mode validate`: `OK`), and at the pre-phase-102 tip the subject boots
it with `0s`.

## Measured verdicts

Shapes are the phase 102 production change (P2, whose code the PLAN's
Appendix A carries unchanged) and its negative controls
(SPEC §7), each run through this fixture alone with `-count=1`.

| shape | verdict | what the subject did |
|---|---|---|
| tip (no production change) | FAIL | booted (`0s` folds to 5 s) |
| P2 | PASS (3/3 runs) | rejected with the range phrase |
| NC1 (no lower bound) | FAIL | booted (no sink, so no ticker) |
| NC3 (NEG case deleted) | PASS | rejected with the range phrase |
| NC6 (range checked first) | PASS | rejected with the range phrase |
| NC7 (block gated on a sink) | FAIL | booted |
| NC9 (one message for all classes) | PASS | rejected with the range phrase |
| NC2, NC4, NC5, NC8 | PASS | rejected with the range phrase |

Each run of this fixture (reference container start and exit, then the
subject build and exit) took 2.18-2.71 s across 13 runs.

## This fixture vs fixture-0128

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch, one substring), the range and the
negative-duration classes are SEPARATE directories:
- **0127** — range class (`0s`, `value must be inside range [1ms, 5m0s)`)
- **0128** — negative class (`-0.5s`, `Invalid duration: Expected positive duration`)

## Cross-references

- phase 102 SPEC §6 (two boot-reject directories) and §7 (NC roster)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- fixture-0128 (sibling negative-duration boot-reject)
- fixture-0042 (the BootRejectFixture shape this copies)
```

## Appendix E.3 — `test/fixtures/0128-stats-flush-interval-negative-boot-reject/driver/driver.go` (175 lines). Task 3

```go
// Package driver registers the 0128-stats-flush-interval-negative-boot-reject
// BOOT-REJECT differential fixture with the runner per phase 102 SPEC §6. It
// is a SYMMETRIC boot-reject fixture: a top-level bootstrap
// `stats_flush_interval: -0.5s` MUST fail-closed at config-load on BOTH
// reference Envoy v1.37.2 AND envoy-go, and BOTH stderr buffers must contain
// the reference's negative-duration phrase.
//
// # Reject arm
//
// `-0.5s` is a negative NANOS value (seconds 0, nanos -500000000). The
// reference refuses a negative Duration before its range check, with the
// `Invalid duration` class, not the range class. The value is chosen over
// `-1s` because seconds 0 is the case that still reaches the lower range bound:
// an envoy-go that checked the range first (or collapsed the classes into one
// message) would print the range phrase for `-0.5s` and fail this fixture,
// while `-1s` (seconds -1, below 300 and not 0) would skip the lower bound and
// let that mutant pass. The bootstrap carries NO `stats_sinks`, so the claim is
// sink-independent.
//
// # Common boot-reject substring
//
//   - reference Envoy v1.37.2 stderr: "... Invalid duration: Expected
//     positive duration ..."
//   - envoy-go stderr: "load config: bootstrap: stats_flush_interval: Invalid
//     duration: Expected positive duration"
//
// The needle stops before the reference's `: goo.gle/debug...` token and does
// not contain `bootstrap:`, which the reference never prints.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
// with an HCM direct_response (status 222, inline body: envoy-go refuses a
// direct_response without a body, so a body-less route would boot-reject the
// subject for an unrelated reason) and a minimal c_unused cluster
// (127.0.0.1:1, never dialed). Neither is reached: the reject fires while the
// bootstrap is loaded, before any listener binds.
//
// # Cross-references
//
//   - phase 102 SPEC §6 (two boot-reject directories) and §6.4 (NC cells)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0127 (the range sibling)
//   - fixture-0042 (the BootRejectFixture shape this copies)
package driver

import (
	"context"
	"fmt"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0128-stats-flush-interval-negative-boot-reject"

	refAdminPort = 9901
	refPort      = 15128 // l_a — the single boot-reject listener (ref container port).

	// flushInterval is the rejected top-level stats_flush_interval value.
	flushInterval = "-0.5s"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr.
	expectedBootErrorSubstr = "Invalid duration: Expected positive duration"
)

func init() {
	fixture.RegisterFixture(fixtureName, &flushNegativeBootRejectDriver{})
}

type flushNegativeBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*flushNegativeBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; the backend is unused by the boot-reject path
func (*flushNegativeBootRejectDriver) SubjectListenerName() string { return "l_a" }
func (*flushNegativeBootRejectDriver) ReferenceListenerPort() int  { return refPort }

// ReferenceBootstrap renders the self-contained boot-reject bootstrap.
func (*flushNegativeBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*flushNegativeBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch.

func (*flushNegativeBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushNegativeBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushNegativeBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err := helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- harness.BootRejectFixture ---

// BootRejectScript returns "": the trigger is inline in the bootstrap.
func (*flushNegativeBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the substring asserted in BOTH stderrs.
func (*flushNegativeBootRejectDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// renderBootRejectBootstrap returns the bootstrap BOTH proxies consume. The
// top-level stats_flush_interval is the only rejected field; there is no
// stats_sinks entry.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

stats_flush_interval: %s

static_resources:
  listeners:
    - name: l_a
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: l_a
                route_config:
                  name: r_a
                  virtual_hosts:
                    - name: vh_a
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 222
                            body: { inline_string: l_a }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, flushInterval, listenerPort)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver = (*flushNegativeBootRejectDriver)(nil)
)
```

## Appendix E.4 — `test/fixtures/0128-stats-flush-interval-negative-boot-reject/README.md` (97 lines). Task 3

```markdown
# Fixture 0128 — stats-flush-interval-negative-boot-reject

BOOT-REJECT differential fixture for the bootstrap's top-level
`stats_flush_interval` (phase 102). Asserts that BOTH reference Envoy
v1.37.2 AND envoy-go fail-close at config-load when the interval is
negative (`-0.5s`), with NO `stats_sinks` configured, and that both name
the negative-duration class rather than the range class.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `Invalid duration: Expected positive duration` (case-sensitive
   substring).

## Boot-reject trigger (reject arm)

`stats_flush_interval: -0.5s` at the top level of the bootstrap: seconds
`0`, nanos `-500000000`.

`-0.5s` (negative NANOS) was chosen over `-1s`. The reference refuses a
negative Duration before its range check. A value with seconds `0` also
falls under the range check's lower bound, so an envoy-go that checks the
range first (NC6) or prints one message for every class (NC9) prints the
range phrase and fails this fixture. `-1s` has seconds `-1`: not `0` and
below 300, so it skips the range clause, and an envoy-go that checks the
range first still prints the negative phrase. Measured: with `-1s` this
fixture PASSES under NC6. The bootstrap has no `stats_sinks`, so the claim
is sink-independent.

## Stderr comparison

Measured against the dockerized reference `envoyproxy/envoy:contrib-v1.37.2`,
run by digest (`docs/envoy-go/ENVOY_TARGET.md`).

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `Invalid duration: Expected positive duration: goo.gle/debugstr ... nanos: -500000000` (a `critical` `error ... initializing config` line) |
| envoy-go | `load config: bootstrap: stats_flush_interval: Invalid duration: Expected positive duration` |

**Common substring:** `Invalid duration: Expected positive duration`.
envoy-go mirrors the reference's phrase. The needle stops before the
reference's `: goo.gle/debug...` token and does not contain `bootstrap:`,
which the reference never prints.

## Bootstrap discipline

Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
with an HCM `direct_response` (status `222`, inline body) and a minimal
`c_unused` STATIC cluster (`127.0.0.1:1`, never dialed). The body is
required: envoy-go refuses a `direct_response` without one
(`hcm: route 0: direct_response.body is required`), so a body-less route
would reject the subject for an unrelated reason. With a valid `1s`
interval the same bootstrap validates on the reference
(`--mode validate`: `OK`), and at the pre-phase-102 tip the subject boots
it with `-0.5s`.

## Measured verdicts

Shapes are the phase 102 production change (P2, whose code the PLAN's
Appendix A carries unchanged) and its negative controls
(SPEC §7), each run through this fixture alone with `-count=1`.

| shape | verdict | what the subject did |
|---|---|---|
| tip (no production change) | FAIL | booted (`-0.5s` folds to 5 s) |
| P2 | PASS (3/3 runs) | rejected with the negative phrase |
| NC1 (no lower bound) | PASS | rejected with the negative phrase |
| NC3 (NEG case deleted) | FAIL | rejected with the range phrase |
| NC6 (range checked first) | FAIL | rejected with the range phrase |
| NC7 (block gated on a sink) | FAIL | booted |
| NC9 (one message for all classes) | FAIL | rejected with the range phrase |
| NC2, NC4, NC5, NC8 | PASS | rejected with the negative phrase |

Each run of this fixture (reference container start and exit, then the
subject build and exit) took 1.58-1.95 s across 13 runs.

## This fixture vs fixture-0127

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch, one substring), the range and the
negative-duration classes are SEPARATE directories:
- **0127** — range class (`0s`, `value must be inside range [1ms, 5m0s)`)
- **0128** — negative class (`-0.5s`, `Invalid duration: Expected positive duration`)

## Cross-references

- phase 102 SPEC §6 (two boot-reject directories) and §7 (NC roster)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- fixture-0127 (sibling range boot-reject)
- fixture-0042 (the BootRejectFixture shape this copies)
```

## Appendix E.5 — `test/differential/runner_test.go`, the two blank imports (`2 0`). Task 3

```diff
diff --git a/test/differential/runner_test.go b/test/differential/runner_test.go
index b59e46d4..d7610ced 100644
--- a/test/differential/runner_test.go
+++ b/test/differential/runner_test.go
@@ -151,6 +151,8 @@ import (
 	_ "github.com/pgdad/envoy-go/test/fixtures/0124-listener-sni-longest-suffix/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0125-listener-filters-timeout/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0126-listener-filters-timeout-envelope/driver"
+	_ "github.com/pgdad/envoy-go/test/fixtures/0127-stats-flush-interval-range-boot-reject/driver"
+	_ "github.com/pgdad/envoy-go/test/fixtures/0128-stats-flush-interval-negative-boot-reject/driver"
 	"github.com/pgdad/envoy-go/test/helpers"
 
 	// Blank-imported so the lua filter's init() boot-registration fires for
```

## Appendix F — `cite-gate.sh <worktree> [base-ref]` (13 lines; exit = moved cited lines). Tasks 4, 5

```bash
#!/usr/bin/env bash
# Phase-102 cite gate. usage: cite-gate.sh <worktree> [base-ref]
# Every `bootstrap.go:<N>` / `:<N>-<M>` citation outside docs/envoy-go/phases/ must
# point at a line whose bytes are identical in the base and in the worktree.
# Exit status = number of moved cited lines.
W=$1; BASE=${2:-master}; F=internal/bootstrap/bootstrap.go; bad=0; n=0
for N in $(git -C "$W" grep -h -oE 'bootstrap\.go:[0-9]+(-[0-9]+)?' "$BASE" -- internal/ cmd/ test/ docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md \
            | sed -E 's/.*://; s/-/ /' | tr ' ' '\n' | sort -nu); do
  n=$((n+1))
  a=$(git -C "$W" show "$BASE:$F" | sed -n "${N}p"); b=$(sed -n "${N}p" "$W/$F")
  [ "$a" == "$b" ] || { echo "MOVED: $F:$N"; bad=$((bad+1)); }
done
echo "cite-gate: $n cited lines checked, $bad moved"; exit $bad
```

## Appendix G.1 — the ADR-0324 status line, ACCEPTED (1 line; REPLACES the `> **STATUS: PROPOSED — §Context drafted at the phase-102 SPEC` line in place). Task 9

```markdown
> **STATUS: ACCEPTED — §Context drafted at the phase-102 SPEC; §Decision + §Consequences APPENDED IN PLACE at the phase-102 IMPL, after the RETAINED italic footer, with no renumber and no `---` separator (the ADR-0294-0323 shared block form).** ⚠️ **THE HOUSE `PROPOSED` GUARD WAS RE-ARMED BY THIS BLOCK AT THE SPEC AND IS DISARMED BY THIS FLIP — AND NO COUNT OF EITHER MATCHER IS WRITTEN ANYWHERE IN IT, DELIBERATELY**: while it read PROPOSED this line was itself a hit of the strict form, so any figure it named would have been falsified by its own landing. **Verify by LINE and by ADR** (backward `^## ADR-` heading search), **never by the count alone**, and never conflate the strict house form with the unrelated **ADR-0231 decoy** at `DECISIONS.md:14866`, which is **BYTE-UNTOUCHED** by this row. **ROW 102 STAYED `in-progress` THROUGH THE SPEC AND THE PLAN** and flips to `done` at this IMPL. An **Observability / bootstrap MAINTENANCE row claiming NO family ordinal**. It **AMENDS** ADR-0262's D-MS-FLUSH parse arm (*"`stats_flush_interval` defaults to 5s"* SURVIVES for an absent field; the bounds and the three rejects are new) and **NARROWS** D-MS-FLUSH-INERT to in-range values; it **NOTES** ADR-0323 §Consequences (g), which named this row next, and ADR-0323 V3, whose `9223372035` seconds ceiling is the same reference bound measured on a different field; it **SUPERSEDES** nothing.
```

## Appendix G.2 — ADR-0324 §Decision + §Consequences (95 lines, beginning with a blank line; APPENDED after the retained footer, the file's last line). Task 9

```markdown

### Decision (landed at the phase-102 IMPL)

**`parseStatsSinks` reads a present `stats_flush_interval` from the Duration's `seconds` and `nanos` fields,
never through `AsDuration`, and applies the reference's three checks in the reference's order, before any
`stats_sinks[]` entry is looked at.** Against the pre-row tip, `git diff --numstat` reads `21 10` for
`internal/bootstrap/bootstrap.go`: the `9 2` code hunk of the SPEC's P2, plus four comment sites. No other
production file changes.

- **F1.** Absent ⇒ `FlushInterval = 5s` (ADR-0262's default, unchanged).
- **F2.** `seconds < 0` or `nanos < 0` ⇒ `bootstrap: stats_flush_interval: Invalid duration: Expected positive
  duration`.
- **F3.** `seconds > 9223372035` or `nanos > 999999999` ⇒ `bootstrap: stats_flush_interval: Invalid duration:
  Duration out-of-range`.
- **F4.** `seconds >= 300`, or `seconds == 0` and `nanos < 1000000` ⇒ `bootstrap: stats_flush_interval: value
  must be inside range [1ms, 5m0s)`.
- **F5.** Otherwise `FlushInterval = seconds·1s + nanos`, exactly; nothing truncates or rounds.

Each message is the `bootstrap: stats_flush_interval: ` prefix followed by the reference's phrase byte for
byte, so a boot-reject fixture can assert one substring on both sides. The reference's run-varying
`goo.gle/debug…` token, its trailing spaces, its proto dump and its `seconds:` / `nanos:` echo are NOT
mirrored. Every field the arithmetic reads is bounded by F2-F4, so F5 stays below 300 s and cannot overflow.
F4 also rejects `{0, 0}`, so the flusher can never be built with a non-positive interval: the old `> 0` guard
is subsumed, not lost. The check is unconditional on sinks.

**Layout.** The two comment hunks above `parseStatsSinks` (the `statsFlushIntervalDefault` comment and the
`FlushInterval` field comment) are line-count-neutral, so none of the `bootstrap.go:<line>` citations in
`internal/`, `cmd/`, `test/`, `BEHAVIOR_CONTRACT.md` or this file moves. Every cited line sits above
`parseStatsSinks`, and a cite gate that compares each cited line's bytes at the base and at the tip read 0
moved. Under the SPEC's line-adding comment shape it read 22 moved.

**Tests.** `internal/bootstrap/stats_flush_interval_test.go` (41 leaves: nine YAML reject values and five YAML
accept values, each on a bootstrap without a sink and with one statsd sink, plus eleven Go-built Durations
called through `parseStatsSinks`), and `cmd/envoy-go/stats_flush_interval_test.go` (`--mode validate` on the
built binary: `0s` and `-1s` exit 1 naming their class, `0.001s` validates). Reject arms assert exactly one
class phrase, the absence of the other two, and the `bootstrap: ` prefix. At the un-fixed tip 29 of the 42
leaves were RED; the 13 accept leaves were GREEN by design.

**Fixtures.** `0127-stats-flush-interval-range-boot-reject` (`0s`, needle `value must be inside range [1ms,
5m0s)`) and `0128-stats-flush-interval-negative-boot-reject` (`-0.5s`, needle `Invalid duration: Expected
positive duration`), both symmetric `BootRejectFixture`s without a stats sink, each with an inline
`direct_response` body (envoy-go refuses a body-less `direct_response`, which would reject the subject for an
unrelated reason). Both were RED at the un-fixed tip (`subject proxy started cleanly`) and are GREEN after.
`-0.5s` rather than `-1s` is the negative arm because a negative-seconds value never reaches F4's lower bound:
with `-1s`, `0128` passed under the reordering mutant.

*Amends ADR-0262 (D-MS-FLUSH parse arm; D-MS-FLUSH-INERT narrowed to in-range values).*

### Consequences (landed at the phase-102 IMPL)

**(a) THE DECLARED BEHAVIOUR CHANGES.**
- **Absent, or a present value inside `[1ms, 5m)`**: unchanged. An in-range value without a sink is still
  parsed and inert (D-MS-FLUSH-INERT, narrowed).
- **`0s` or any negative value**: before, folded silently to the 5 s default and booted; now refused at Load,
  at `--mode validate` and at boot (rc 1), with or without a sink.
- **A sub-millisecond value** (`0.0005s`, `0.000999999s`): before, accepted and flushed about every
  millisecond; now refused with the range phrase.
- **`300s` and above, up to `9223372035.999999999s`**: before, accepted (no flush in a 300 s window); now
  refused with the range phrase.
- **`9223372036s` up to the protobuf maximum**: before, saturated by `AsDuration` to the Go `time.Duration`
  maximum and never flushed; now refused with the out-of-range phrase. `315576000001s` is still refused by
  the protojson decoder first, on both sides.

**(b) THE CLASS ORDER IS LOAD-BEARING.** The reference validates the Duration before PGV, so `-0.5s` (seconds 0)
and `9223372036s` never read the range phrase. A subject that checked the range first, or that printed one
message for every class, agrees with the reference on rc and disagrees on the phrase; the two fixtures and
the unit arms are what pin the phrase.

**(c) THE PANIC HAZARD THE LOWER BOUND REMOVES IS SINK-PATH ONLY.** With the lower bound deleted, `0s`
validates OK; at a real boot it prints `envoy-go ready` and then panics `non-positive interval for NewTicker`
when a sink is configured, and boots silently when none is. A negative value behaves the same way when the
negative case is deleted (`-1s` with a statsd sink: `ready`, then the same panic, exit 2, in about 11 ms;
without a sink it boots and serves). The differential harness kills the subject as soon as it prints `ready`,
so it classes such a subject as booted, with no panic text in its output; that is why both fixtures carry no
sink.

**(d) THE NEGATIVE-CONTROL ROSTER.** Nine mutants of the shipped shape, each marked and asserted in place before
it ran, each reddening exactly the predicted unit leaves: the lower bound (8), the nanos clause of F3 (2, Go-built
arms only, because protojson normalises YAML), the negative case (7), the 300 s edge (3), the seconds ceiling
(6), the class order (8), a sink-gated check (26 new leaves plus the existing `TestStatsSink_InertFlushInterval`),
the 1 ms edge (4) and a single message for all classes (15). At the fixture level, the lower-bound mutant
reddens `0127`; the negative-case, class-order and single-message mutants redden `0128`; the sink-gated mutant
reddens both; the other four redden neither.

**(e) WHAT THIS ROW DID NOT BUY.**
- whole-message byte parity (token, trailing spaces, proto dump, field echo);
- sub-millisecond cadence parity among accepted values, which is unobservable on the reference in this setup,
  and any explanation of the pre-row `0.0005s` cadence (moot now that it is refused);
- a reference arm for `nanos > 999999999`, which YAML and protojson cannot carry; that half of F3 is source-read
  parity, as in ADR-0323 (h);
- a third fixture for the out-of-range class: its mechanism is the one `0128` exercises, and its unit arms pin
  the phrase;
- any change to `stats_flush_on_admin` (still strict-rejected), to the exported `Pipeline.Run` residual
  (ADR-0323 (h)), or to any listener-side candidate;
- any stat name: the stat surface moves by `+0`.
```

## Appendix H — the `BEHAVIOR_CONTRACT.md` ledger entry (1 line; its own paragraph, one blank line after the phase-101 entry). Task 10

```markdown
**Phase 102 — +0, UNCHANGED (no new stat NAME; the row changes which `stats_flush_interval` values load, not which counters exist) — delta only:** phase 102 (`stats-flush-interval-bounds`) adds no stat name. The reference has no flush-count stat, and envoy-go's new rejects fire inside `Load`, before any stat is registered, so neither side's surface moves. An in-range `stats_flush_interval` without a stats sink is still parsed-and-inert; every other present value is now a boot reject (ADR-0324). Like the phase-96 through phase-101 entries, this one quotes NO absolute, because three mutually inconsistent absolutes are live at one tip.
```
