# Phase 102 (stats-flush-interval-bounds) — IMPL progress

Base SHA: `d4c59f51` (master). Worktree
`/home/esa/git/envoy-go/.worktrees/wt-phase-102-impl`, branch `wt-phase-102-impl`.

## Task 1: Baseline, a proven-live panic gate, and PROGRESS.md

**Step 1** — worktree pre-existing (made by the controller). Branch `wt-phase-102-impl`.

**Step 2** — `go list $U | wc -l` -> **3** (non-zero, no `[setup failed]`).

**Step 3** — bare-tip roster, selector `$U` = `./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...`:
```
rc=0 RUN=391 FAIL=0
```
Roster saved (scratch) as `base-run.txt`.

**Step 4** — panic gate live: throwaway `internal/bootstrap/zz_gate_test.go` with `panic("p102-gate")`;
`go test -count=1 -run TestP102Gate ./internal/bootstrap/`; `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` = **1**.
File deleted; `git status --short` prints nothing.

**PRE-FLIP sentinel** (next-prompt.txt checks 1-3 against `docs/envoy-go/ROADMAP.md`, `/usr/bin/grep`):
```
(1) NOT DONE: row 102
(2) 212: 218: 224: 234: 240: (remaining deferred (not-yet-chartered) candidates:) and 248: (deferred candidates:)  -> six hits
(3) silent
NC-A (row 62 -> in-progress): NC LANDED? [ in-progress ] / NOT DONE: row 62 / NOT DONE: row 102
NC-B (want=133): NOT DONE: row 102 / GATE FAIL: examined 134 data rows, expected 133
```

## Task 2: The 42 unit leaves — 29 RED at the un-fixed tip

Appendices B and C extracted with `ext`. `wc -l`: bootstrap file **147**, cmd file **90**. Identifier-collision grep
hit only docs/ (PLAN/SPEC), no `.go` file. `go vet ./internal/bootstrap/ ./cmd/envoy-go/` rc 0; `gofmt -l` prints nothing.
`go test -count=1 -v $U`: `rc=1`, `=== RUN` **437**, FAIL matcher **37**, **29** RED leaves, panic gate 0.
Reasons (measured): 18 at `stats_flush_interval_test.go:78` and 9 at `:136`, all `got nil error, want one containing ...` (27 bootstrap leaves);
2 at `:83` `got err=<nil>, want *exec.ExitError with exit code 1` (binary `0s`, `-1s`).
13 GREEN-by-design leaves asserted `--- PASS`: ten `YAMLAccept/*`, GoBuilt `0s_1000000ns` and `299s_999999999ns`, binary `0.001s`.
RED list (`tip-red.txt`):
```
TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/0s
TestEnvoyGoBinary_ModeValidate_StatsFlushInterval/-1s
TestStatsFlushInterval_GoBuilt/0s_1000000000ns
TestStatsFlushInterval_GoBuilt/0s_999999ns
TestStatsFlushInterval_GoBuilt/-1s_0ns
TestStatsFlushInterval_GoBuilt/1s_-1ns
TestStatsFlushInterval_GoBuilt/1s_2147483647ns
TestStatsFlushInterval_GoBuilt/300s_0ns
TestStatsFlushInterval_GoBuilt/9223372035s_0ns
TestStatsFlushInterval_GoBuilt/9223372036s_0ns
TestStatsFlushInterval_GoBuilt/MaxInt64s_0ns
TestStatsFlushInterval_YAMLReject/0.0005s/B0_no_sink
TestStatsFlushInterval_YAMLReject/0.0005s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/0.000999999s/B0_no_sink
TestStatsFlushInterval_YAMLReject/0.000999999s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/-0.5s/B0_no_sink
TestStatsFlushInterval_YAMLReject/-0.5s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/0s/B0_no_sink
TestStatsFlushInterval_YAMLReject/0s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/1000s/B0_no_sink
TestStatsFlushInterval_YAMLReject/1000s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/-1s/B0_no_sink
TestStatsFlushInterval_YAMLReject/-1s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/300s/B0_no_sink
TestStatsFlushInterval_YAMLReject/300s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/315576000000s/B0_no_sink
TestStatsFlushInterval_YAMLReject/315576000000s/B1_statsd_sink
TestStatsFlushInterval_YAMLReject/9223372036s/B0_no_sink
TestStatsFlushInterval_YAMLReject/9223372036s/B1_statsd_sink
```

## Task 3: Fixtures 0127 and 0128 — drivers, READMEs, imports, four registration gates

Step 1: `git grep -c '\b15127\b'` / `15128` over `test/ internal/ cmd/` print nothing; `ss -tanH; ss -uanH` show no socket on either port.
Step 2: Appendices E.1-E.4 extracted; `wc -l` = **172 / 92 / 175 / 97**.
Step 3: Appendix E.5 `git apply --check` then applied; `git diff --numstat -- test/differential/runner_test.go` = `2	0`.
Step 4: `go vet` (both fixture dirs + `./test/differential/`) rc 0; `gofmt -l` prints nothing; `GOTOOLCHAIN=go1.26.2 golangci-lint run` (same three packages) rc 0.
Step 5: imports **130**, dirs **130**, `comm -23` and `comm -13` both empty; each new name reads 1 in each file.
Step 6 (NC, scratch copy of runner_test.go): renaming 0127's import token fired BOTH directions (`comm -23` listed `0127-stats-flush-interval-rangeX-boot-reject`, `comm -13` listed `0127-stats-flush-interval-range-boot-reject`); deleting the 0128 import line fired `comm -13` only (`0128-stats-flush-interval-negative-boot-reject`), `comm -23` empty.

## Task 4: The un-fixed tip recorded — every falsifier RED, for its named reason

Step 1 (unit suite, `$U`): `rc=1`, `=== RUN` **437**, FAIL matcher **37**, **29** RED leaves; `cmp` against Task 2's `tip-red.txt` identical.
Roster vs `base-run.txt`: **0** removed, **46** added = the 42 leaves + their 4 parents (`TestEnvoyGoBinary_ModeValidate_StatsFlushInterval`, `TestStatsFlushInterval_GoBuilt`, `_YAMLAccept`, `_YAMLReject`).
Step 2 (fixtures alone, `-run $FXSEL ./test/differential/`): `rc=1`; `grep -ac '^=== RUN   TestDifferential/'` = **2**; `grep -ac SKIP` = **0**. No `direct_response.body is required` failure. Verdict lines:
```
    runner_test.go:1151: BootRejectFixture: subject proxy started cleanly — expected boot rejection for broken script ""
    runner_test.go:1151: BootRejectFixture: subject proxy started cleanly — expected boot rejection for broken script ""
--- FAIL: TestDifferential (3.51s)
    --- FAIL: TestDifferential/0127-stats-flush-interval-range-boot-reject (2.01s)
    --- FAIL: TestDifferential/0128-stats-flush-interval-negative-boot-reject (1.50s)
```
Step 3 (cite gate, Appendix F, `cite-gate.sh $W master`): `cite-gate: 27 cited lines checked, 0 moved`, exit 0.

## Task 5: The production edit — Appendix A under the cite gate

Step 1: `ext A` extracted (64 lines), `git apply --check` then applied; `git diff --numstat -- internal/bootstrap/bootstrap.go` = `21	10`.
Step 2: anchor A2 `d.AsDuration(); v > 0` = 0 hits; each of `Expected positive duration`, `Duration out-of-range`, `value must be inside range [1ms, 5m0s)` = 1 hit; `GetSeconds(), d.GetNanos()` = 1.
Step 3: hunks `-226,2 +226,2`, `-534,4 +534,4`, `-789,2 +789,6`, `-800,2 +804,8`, `-802,0 +813`.
Step 4: `cite-gate.sh $W master` => `cite-gate: 27 cited lines checked, 0 moved`, exit 0.
Step 5: `gofmt -l internal/bootstrap` empty; `go vet` rc 0; `GOTOOLCHAIN=go1.26.2 golangci-lint run` rc 0; British-spelling sweep over added lines: no hits.

## Task 6: All arms green

Step 1 (`go test -count=1 -v $U`): rc 0, `=== RUN` **437**, FAIL matcher **0**, panic gate **0**; all **29** leaves of `tip-red.txt` read `--- PASS`.
Step 2 (`go test -count=1 -race $U`): rc 0, `DATA RACE` 0.
Step 3 (two new test files moved to scratch): rc 0, `=== RUN` **391**, FAIL **0**; files moved back, `git status --short` empty.
Step 4 (`$FXSEL` x3, solo): rc 0 each, 2 `=== RUN`, 0 SKIP, both `--- PASS`; wall 4s / 5s / 4s (test-reported 3.78s on run 3: 0127 2.18s, 0128 1.59s).
Step 5 (`TestDifferential/0092-stats-sink-statsd$`): rc 0, 1 RUN, 0 SKIP, `--- PASS` (5.77s).

## Task 7: The NC roster, scored per arm (unit and fixture)

Step 1: `ext D` -> `$S/nc.py` (27 lines); `cmp bootstrap.go bootstrap.final.go` identical before starting.
Step 2 (mechanisms, copied from PLAN §4.3 BEFORE running):

| NC | mutation | mechanism | expected RED leaves (parents) |
|---|---|---|---|
| NC1 | drop `\|\| (secs == 0 && nanos < 1000000)` | zero / sub-ms reach F5 | 8 (3) |
| NC2 | drop `\|\| nanos > 999999999` | Go-built nanos overflow passes F3 | 2 (1) |
| NC3 | delete the NEG case | `-1s` reaches F5; `-0.5s` falls to F4 (wrong class) | 7 (3) |
| NC4 | `secs >= 300` -> `secs > 300` | `300s` accepted | 3 (2) |
| NC5 | drop `secs > 9223372035 \|\|` | huge seconds fall to F4 | 6 (2) |
| NC6 | order F4, F2, F3 | `-0.5s` and huge seconds hit F4 first; `-1s` does not | 8 (2) |
| NC7 | gate the block on `len(bs.GetStatsSinks()) > 0` | no sink => field ignored => 5 s | 26 (5) |
| NC8 | `nanos < 1000000` -> `nanos <= 1000000` | 1 ms edge rejected | 4 (3) |
| NC9 | all three cases print the RANGE message | class collapse | 15 (3) |

Step 3 (unit; `$S/runnc.sh n`: apply, marker count, `go test -count=1 -v $U`, restore, marker count; `$U` = `./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...`):

| NC | marker before/after | RUN | panic gate | RED total = leaves + parents | vs PLAN 4.3 |
|---|---|---|---|---|---|
| NC1 | 1 / 0 | 437 | 0 | 11 = 8 + 3 | match |
| NC2 | 1 / 0 | 437 | 0 | 3 = 2 + 1 | match |
| NC3 | 1 / 0 | 437 | 0 | 10 = 7 + 3 | match |
| NC4 | 1 / 0 | 437 | 0 | 5 = 3 + 2 | match |
| NC5 | 1 / 0 | 437 | 0 | 8 = 6 + 2 | match |
| NC6 | 1 / 0 | 437 | 0 | 10 = 8 + 2 | match |
| NC7 | 1 / 0 | 437 | 0 | 31 = 26 + 5 | match |
| NC8 | 1 / 0 | 437 | 0 | 7 = 4 + 3 | match |
| NC9 | 2 / 0 | 437 | 0 | 18 = 15 + 3 | match |

Sorted RED lists are in `$S/nc<n>.red`; the leaf NAMES were read against 4.3 row by row (e.g. NC6 leaves `-1s` GREEN, NC5 leaves `9223372035s_0ns` GREEN, NC7 = 9 B0 YAML rejects + 9 GoBuilt rejects + binary `0s`/`-1s` + 4 B0 accepts + 2 GoBuilt accepts + `TestStatsSink_InertFlushInterval`; NC9 = 7 NEG + 8 OOR). Every other leaf GREEN (RUN 437 minus the RED set). No mismatch in either direction.

Step 4 (fixtures, one `$FXSEL` run per NC; each 2 `=== RUN`, 0 SKIP via `grep -a`; marker after revert 0):

| NC | rc | 0127 | 0128 | vs 4.4 |
|---|---|---|---|---|
| NC1 | 1 | FAIL: `subject proxy started cleanly` | PASS | match |
| NC3 | 1 | PASS | FAIL: `subject stderr does NOT contain "Invalid duration: Expected positive duration"` | match |
| NC6 | 1 | PASS | FAIL: same | match |
| NC7 | 1 | FAIL: started cleanly | FAIL: started cleanly | match |
| NC9 | 1 | PASS | FAIL: same | match |
| NC2, NC4, NC5, NC8 | 0 each | PASS | PASS | match |

Step 5: `cmp bootstrap.go bootstrap.final.go` identical; `git status --short` shows only PROGRESS.md.

## Task 8: The occurrence set re-derived at the IMPL tip

Census (SPEC 4, verbatim): `git grep -n -i -E 'non-positive|parsed-and-inert|stats_flush_interval|D-MS-FLUSH' <rev> -- docs/ internal/ cmd/ test/ ':!docs/envoy-go/STATE_HISTORY.md' ':!docs/envoy-go/phases/'`.
Counts: `44b2de38` = **168**; `d4c59f51` = **167**; HEAD (IMPL tip) = **195**.
Base: 44b2de38 (the PLAN's stated base) and d4c59f51 differ by one hit, both in docs/envoy-go/STATE.md (3 -> 2: the PLAN commit moved the "active-phase" line to "prior active-phase" so it no longer matches). Per-file diff 44b2de38 -> HEAD is used (it contains that -1); growth is +27 (168 -> 195) = 28 new hits - 1 STATE.md.
Attribution of the 28 new hits (all expected files; none in any other file):
- cmd/envoy-go/stats_flush_interval_test.go +2; internal/bootstrap/stats_flush_interval_test.go +6 (the two new test files)
- test/fixtures/0127.../driver/driver.go +6, README.md +4; 0128.../driver/driver.go +5, README.md +3 (fixture dirs)
- internal/bootstrap/bootstrap.go 11 -> 13 (+2; rewritten comments at 226-227, 534-536, 788-791 and the three error strings)
- STATE.md 3 -> 2 (-1) is the PLAN-commit change above, not IMPL growth.
Tasks 9-10 (ADR-0324 text, contract ledger) have NOT landed, so no DECISIONS.md / BEHAVIOR_CONTRACT.md growth is expected or seen.
Step 3: `non-positive` in internal/bootstrap/bootstrap.go = **0** (was 2 comment hits at the base: 227, 534-535). No `parsed-and-inert` remains in bootstrap.go (old "Parsed-and-stored" comment replaced by "An in-range ... ALWAYS stored"). The one surviving `parsed-and-inert` hit census-wide is docs/envoy-go/BEHAVIOR_CONTRACT.md:845 ("a bare `stats_flush_interval` is parsed-and-inert"), which does NOT carry the in-range qualifier: recorded as a finding for Task 10 (ledger/contract text), not edited here.

## Task 9: ADR-0324 completed in place, ACCEPTED

Step 1: `ext G.1` / `ext G.2` -> `wc -l` **1** / **95**.
Step 3 (figures re-derived at this commit before installing; G.2 corrected in a scratch copy `G2c.md`, 96 lines):
- `git diff --numstat d4c59f51 HEAD -- internal/bootstrap/bootstrap.go` = `21	10` (matches). Hunks: code `-800,2 +804,8` + `-802,0 +813` = `9 2` (matches); comment hunks at 226, 534, 789 = **three**, not four. **CORRECTED** "plus four comment sites" -> "plus three comment hunks" (the PLAN's §1.2/Appendix A label said four; three measured).
- Leaves (`go test -count=1 -v -run 'TestStatsFlushInterval_|TestEnvoyGoBinary_ModeValidate_StatsFlushInterval' ./internal/bootstrap/ ./cmd/envoy-go/`, rc 0): YAMLReject 18, YAMLAccept 10, GoBuilt 11, binary 3 = 42 total; the bootstrap file has **39** leaves (9x2 + 5x2 + 11), not 41. **CORRECTED** "41 leaves" -> "39 leaves". 42 / 29 RED / 13 GREEN match Tasks 2/4.
- Cite gate re-run (`cite-gate.sh $W master`): `cite-gate: 27 cited lines checked, 0 moved`, exit 0; max cited line 756 < `func parseStatsSinks` at 801. The 22-moved figure is PLAN §6 (P3), not re-run at IMPL: **CORRECTED** the sentence to attribute it to the PLAN.
- NC counts 8, 2, 7, 3, 6, 8, 26 (+ `TestStatsSink_InertFlushInterval`), 4, 15 match Task 7 (`nc7.red`: 26 leaves; the 5 non-leaf REDs = 4 parents + `TestStatsSink_InertFlushInterval`).
Step 2: status line 19825 replaced by G1; G2c appended after the footer (last line). No `**Status:**`, no renumber, no `---`.
Step 4: `^> \*\*STATUS: PROPOSED` = **0**; `sed -n 19825p` reads `> **STATUS: ACCEPTED — §Context drafted at the phase-102 ...`, backward heading = `## ADR-0324`. Decoy `^\*\*Status:\*\* PROPOSED` hits `:14866` -> `## ADR-0231`, md5 `929719b67c87aa16ac1e406fed7eba6b`. `^---$` **216**, `^## ADR-` **323**, `^## ` **331**. Tail ADR `ADR-0324`; next free **ADR-0325**.
Step 5: `git diff --numstat -- docs/envoy-go/DECISIONS.md` = **`97	1`** (19839 -> **19935**), one line more than the PLAN's `96 1` because the cite-gate sentence correction wraps to one extra line.

## Task 10: BEHAVIOR_CONTRACT.md — the inert phrase and the +0 ledger entry

Step 1: `:845` literal `(a bare `stats_flush_interval` is parsed-and-inert)` -> `(an in-range bare `stats_flush_interval` is parsed-and-inert; an out-of-range one is a boot reject, ADR-0324)`; rest of the line byte-untouched.
Step 2: `ext H` (1 line) appended as its own paragraph one blank line after the `**Phase 101 — +0, UNCHANGED (` entry (now `:5154`).
Step 3: `/usr/bin/grep -nE '→|->'` over the new entry: no hit (rc 1). `git grep -c 'a bare `stats_flush_interval` is parsed-and-inert'` prints nothing; the new phrase reads **1**. `wc -l` **6002 -> 6004**; `git diff --numstat` = **`3	1`**; `git diff --check` clean.

## Task 11: ROADMAP row 102 -> done, under the field-count gate

Sentinel commands copied verbatim from `next-prompt.txt` lines 12-91 into `$S/sentinel.sh` (only `grep` -> `/usr/bin/grep`), run against `docs/envoy-go/ROADMAP.md` on both sides of the flip.

Step 1: cell paragraph `**IMPL done 2026-10-17** — ...` written to `$S/cell102.txt`, spliced into a scratch copy with field 5 -> `done`.
Step 2 (scratch copy): naive `awk -F'|' '/^\| *102 /{print NF}'` = **8**; escape-aware `sed 's/\\|//g' F | awk -F'|' '/^\| *102 /{print NF}'` = **8**; field 5 reads `[ done ]`; scratch 252 lines; every other line identical.
Step 3 (row 102 line of the scratch copy): `|` in the new paragraph **0**; `deferred candidates:` / `remaining deferred (not-yet-chartered) candidates:` **0**; `deferred` (case-insensitive) **0**; `-family row` (with `--`) **0**.
Step 4: installed; `git diff --numstat -- docs/envoy-go/ROADMAP.md` = `1	1`; `git diff --check` clean.

| measure | BEFORE the flip | AFTER the flip |
|---|---|---|
| check (1) | ONE: `NOT DONE: row 102` | **SILENT** |
| check (2) | SIX at :212 :218 :224 :234 :240 :248 | SIX at :212 :218 :224 :234 :240 :248 |
| check (3) | SILENT | SILENT |
| NC-A (row 62 -> in-progress; `NC LANDED? [ in-progress ]` inspected) | TWO: row 62, row 102 | **ONE**: `NOT DONE: row 62` |
| NC-B (want=133) | TWO: `NOT DONE: row 102`, `GATE FAIL: examined 134 data rows, expected 133` | **ONE**: `GATE FAIL: examined 134 data rows, expected 133` |
| NC-C | residual 0, FIRED | residual 0, FIRED |
| check-(2) positive control | residual 0, 6 substitutions | residual 0, 6 substitutions |
| NC-D `-family row` under `--` | 96 / 68 | 96 / 68 |
| escape-aware malformed set | {57 (line 119, NF 9), 69 (line 131, NF 10)} | same |
| row 102 NF naive / escape-aware | 8 / 8 | 8 / 8 |
| six-window md5 (trailing newline included, first 12 hex) | 212 10d7807bf02d, 218 4a92f7e62fc6, 224 2a7eb298b9fd, 234 242e53c6f7a3, 240 b2680e6f4fbf, 248 6caa1c3ce0e7 | identical |
| `want` / `ROADMAP.md` lines | 134 / 252 | 134 / 252 |

The sentinel does not fire (check (2) reads six); `stop` absent at the git root and in the worktree, not created.
PROPOSED guard at this tip: house form `^> \*\*STATUS: PROPOSED` 0 hits; `:19825` reads `> **STATUS: ACCEPTED —`, backward heading `19823 ## ADR-0324`; decoy `^\*\*Status:\*\* PROPOSED` at `:14866` -> `14864 ## ADR-0231`, md5 `929719b67c87`.

## Task 12: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

Preconditions: branch `wt-phase-102-impl`, HEAD `04249e85`, `git status --short` clean; `master` = `d4c59f51` (unmoved). Logs in `.superpowers/sdd/PLAN/scratch/` (`gate-a-run1.log`, `gate-a.log`, `gate-b.log`, `gate-b2.log`, `gate-c.log`, `fz1.log`, `fz2.log`, `lint*.log`).

### Step 1 - byte-untouched roster and edit roster

`git diff d4c59f51 --numstat` was EMPTY for: `cmd/envoy-go/main.go`, every non-test file under `cmd/`, `internal/statssink/`, `internal/stats/`, `internal/bootstrap/bootstrap_test.go`, `internal/bootstrap/statssink_fuzz_test.go`, `cmd/envoy-go/main_test.go`, every `test/fixtures/*` dir except `0127`/`0128`, `REVIEW_FINDINGS.md`, `go.mod`, `go.sum`, `.github/`.

Branch diff (13 paths), added/removed: bootstrap.go `21 10`; bootstrap/stats_flush_interval_test.go `147 0`; cmd/envoy-go/stats_flush_interval_test.go `90 0`; 0127 driver `172 0`, README `92 0`; 0128 driver `175 0`, README `97 0`; runner_test.go `2 0`; DECISIONS.md `97 1`; BEHAVIOR_CONTRACT.md `3 1`; ROADMAP.md `1 1`; phase PROGRESS.md `213 0` (before this append). All match PLAN 1.2 except **DECISIONS.md `97 1` vs the plan's `96 1`** (+1 line; the plan said "measured on THIS text", the ADR was completed in place at Task 9) - FINDING, recorded, not forced. The only path on neither roster is this phase's PROGRESS.md (expected).

### Step 2 - ARM roster

`go test -count=1 -v ./internal/bootstrap/... ./cmd/envoy-go/... ./internal/statssink/...`: rc 0, **437** sorted `=== RUN` lines vs `base-run.txt` **391**. `comm -13`: **46** gained = 42 leaves + 4 new parents (`TestEnvoyGoBinary_ModeValidate_StatsFlushInterval`, `TestStatsFlushInterval_GoBuilt`, `TestStatsFlushInterval_YAMLAccept`, `TestStatsFlushInterval_YAMLReject`). `comm -23`: **0** lost. 391 + 46 = 437.

### Step 3 - Gate (a): full differential (`-count=1 -v -timeout 40m`)

- Run 1: rc=1 after 258 s. **Port-race abort (MASKING)**: `panic: driver: start OTLP receiver on 0.0.0.0:33605 ... bind: address already in use` in `0084-otlp-access-log` driver (`ensureServer`, driver.go:169) - the known driver-receiver port race. 85 PASS / 1 FAIL (0084) / 0 SKIP before the abort, 86 fixtures started; the rest (including 0127/0128) were masked. Kept as `gate-a-run1.log`.
- Run 2 (rerun, recorded; clears nothing about run 1): rc=0, `--- PASS: TestDifferential (447.69s)`, `ok 451.749s`. **PASS 130 / FAIL 0 / SKIP 0**. Fixture set by name vs `ls -d test/fixtures/*/`: **130 = 130**, `comm -23` empty, `comm -13` empty. `0127` PASS (1.66s), `0128` PASS (1.53s).

### Step 4 - Gate (b): non-Docker sweep

`go list ./...` minus differential and h2spec: **245** packages (243 + the two new `driver` packages, as predicted).
- Sweep 1: rc=1; 124 `ok` + 1 `FAIL` + 120 `[no test files]` = 245. The FAIL is the known flake `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server...` (`internal/boot`): `boot error = "... DeadlineExceeded ..."`, want it to mention the initial-fetch timeout. Unrelated to this row (no stats-flush path).
- Isolation: 3x `-run TestSDSEndToEnd_FetchFailure_BootFailsClosed ./internal/boot/` rc 0 each; whole package rc 0. (A green rerun clears nothing; the flake fired once in sweep 1.)
- Sweep 2: rc=0; 125 `ok` + 0 `FAIL` + 120 `[no test files]` = **245**.

### Step 5 - Gate (c): h2spec

`go test -count=1 -v ./test/conformance/h2spec/`: rc=0, `95 tests, 94 passed, 1 skipped, 0 failed`; 0 `[FAIL]` lines.

### Step 6 - Gate (d): fuzzers

`git grep -c '^func Fuzz' -- '*.go'`: **56 targets across 48 files**. The row adds none. `FuzzBootstrapLoad` 20 s: rc 0 (497840 execs); `FuzzStatsSinkConfigParse` 20 s: rc 0 (536355 execs); both in `./internal/bootstrap/`. No `testdata/fuzz` files created; `git status --short` empty.

### Step 7 - Gate (e): panic gate, lint, planted control

Anchored `^panic:|DATA RACE|SIGSEGV`: gate-a run 2 **0**, gate-b **0**, gate-b2 **0**, gate-c **0**, both fuzz logs **0**; gate-a run 1 = **1** (the OTLP port-race panic above). `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`: rc=0, no output.
Planted control in `internal/stats/zz_lintplant.go` (`go build ./internal/stats/` rc=0 first): the brief's one-liner `func zzPlant() int { os.Remove("/x"); x := 1; x = 2; return x }` fires `errcheck` ONLY - **`ineffassign` did NOT fire on that one-liner** (FINDING: the brief's example does not exercise ineffassign). A multi-line `y := 1; y = 2; y = 3; return y` function in the same file fired `ineffassign` (2 hits, by name) alongside `errcheck`; lint rc=1. File removed; `git status --short` empty.

### Step 8 - Gate (f)

No `REVIEW.md` for phase 102 (nor 93-101). This is the one standing departure; compliance is not claimed.

### Summary - all six gates

| gate | result |
|---|---|
| (a) differential | run 1 ABORTED by the 0084 OTLP port race (85 PASS / 1 FAIL, masking); run 2 rc=0, 130 PASS / 0 FAIL / 0 SKIP, set 130 = 130 both `comm` directions |
| (b) sweep | 245 pkgs; sweep 1 rc=1 (known SDS flake, 124 ok + 1 FAIL + 120 none); sweep 2 rc=0 (125 ok + 0 FAIL + 120 none) |
| (c) h2spec | `95 tests, 94 passed, 1 skipped, 0 failed` |
| (d) fuzzers | 56 targets / 48 files; two 20 s runs rc 0, tree clean |
| (e) panic + lint | 0 on every green log (1 on aborted run 1); lint rc=0; control fired errcheck + ineffassign (multi-line form), removed, tree clean |
| (f) REVIEW.md | absent (standing departure) |

## Final-review fix

Two rulings from the final whole-branch review (minors 1 and 3) were fixed; no other file changed.

- **Ruling 1 (minor 1).** `assertFlushReject` in `internal/bootstrap/stats_flush_interval_test.go` now pins the full
  `bootstrap: stats_flush_interval: ` prefix (HasPrefix and message). File stays 147 lines (`git diff` is 2 lines changed).
- **Mutant evidence.** Mutant: `stats_flush_interval: ` removed from the three reject messages in `parseStatsSinks`
  (`// NCP` marker; `grep -c NCP` = 1 before running, 0 after restore). `go test -count=1 -v -run TestStatsFlushInterval`:
  NEW assertion RED (rc 1, 27 failing leaf `--- FAIL` lines plus 2 parent lines); OLD assertion (scratch copy of the test
  file) GREEN (rc 0, 0 FAIL, 42 `=== RUN`), so the old assertion was blind. Both files restored; `cmp bootstrap.go
  bootstrap.final.go` identical.
- **Run.** `go test -count=1 -v $U`: 437 `=== RUN`, 0 FAIL lines, 0 panic/race lines, rc 0. gofmt and vet clean;
  `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/bootstrap/` rc 0.
- **Ruling 2 (minor 3 plus the deferred Layout minor).** ADR-0324 Decision now reads "the `9 2` code change ... plus three
  comment sites, in five `@@` hunks"; the Layout paragraph names the third comment site (doc comment at :789, +4 lines,
  below the last cited line :756). The ROADMAP row-102 cell carried the same loose "three comment hunks" and was reworded
  to "three comment sites, in five hunks" (NF 8 naive and escape-aware, no `|`/`deferred`/`-family row`, 252 lines).
- **Re-derived figures.** `git diff d4c59f51 --numstat` DECISIONS.md is now `98 1` (was `97 1` above: +1 line from the
  Layout sentence wrapping), ROADMAP.md `1 1`. `bootstrap.go` stays `21 10`. No claim that the test file is
  cmp-identical to Appendix B exists in DECISIONS, ROADMAP or the BEHAVIOR_CONTRACT entry; the test file now differs from
  the PLAN appendix by those two lines, which is intended.

## Close-out (Task 13)

Tasks 1-13 landed in order on `wt-phase-102-impl` (base `d4c59f51`); Task 14 (squash, merge, push, worktree
remove) is the controller's. Row 102 reads `done` at `ROADMAP.md:164`; lifecycle-state **3 -> DONE**.

### Per-task summary

| task | what landed (measured) |
|---|---|
| 1 | bare roster 391 / 0 on `$U`; panic gate proven live (1 hit on a planted `panic`, file removed); pre-flip sentinel recorded |
| 2 | Appendices B-C: 437 `=== RUN`, 37 FAIL lines, **29** RED leaves by name, 13 GREEN by design |
| 3 | fixtures `0127` / `0128` + two imports (`2 0`); 130 = 130 both `comm` directions; rename / delete NCs fired as predicted |
| 4 | un-fixed tip: same 29 RED (`cmp` identical), 46 roster lines added, 0 removed; both fixtures RED with `subject proxy started cleanly`; cite gate 27 / 0 |
| 5 | Appendix A: `bootstrap.go` `21 10`; anchors asserted; cite gate 27 / 0; gofmt, vet, lint, misspell clean |
| 6 | 437 / 0, `-race` clean, 391 / 0 with the new test files moved out, `0127` / `0128` 3/3, `0092` green |
| 7 | nine NCs, each marker asserted before (1; NC9 2) and after (0); every unit and fixture cell equal to PLAN §4.3 / §4.4 |
| 8 | occurrence set: 168 (`44b2de38`) -> 195 at the IMPL tip, all 28 new hits in expected files; `non-positive` in `bootstrap.go` 0 |
| 9 | ADR-0324 §Decision + §Consequences appended, `ACCEPTED`; house guard EMPTY, proven by reading `:19825`; `97 1` at that commit |
| 10 | contract `:845` phrase rewritten + delta-only `+0` ledger entry (`:5154`); `3 1`, 6002 -> 6004 |
| 11 | row 102 flipped under the field-count gate (NF 8 / 8, no `\|`, no `deferred`, no `-family row`); sentinel on both sides |
| 12 | byte-untouched and ARM rosters (391 + 46 = 437); all six gates (below); one-line ineffassign plant FINDING |
| final-review fix | full `bootstrap: stats_flush_interval: ` prefix pinned (prefix-drop mutant: new assertion 27 leaves RED, old GREEN); ADR-0324 hunk wording; row-102 cell wording; `DECISIONS.md` `98 1` |
| 13 | this close-out; ADR-0324's Tests paragraph re-worded to name the full prefix (line-count-neutral); `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt` rolled |

### Refutations this IMPL made (method note 2) — ten

1. **PLAN §1.2 / Appendix A's "four comment sites" in `bootstrap.go`** — three comment sites in five `@@` hunks
   (two pure-comment hunks above `parseStatsSinks`, the `:789` doc comment, and the two code hunks). Corrected in
   ADR-0324 at Task 9 ("three comment hunks") and again at the final-review fix ("three comment sites, in five
   `@@` hunks").
2. **PLAN §1.2 / Appendix B's "41 leaves" in `internal/bootstrap/stats_flush_interval_test.go`** — **39** (YAMLReject
   18 + YAMLAccept 10 + GoBuilt 11); **42** in all with the binary's 3. Re-measured at this close:
   `go test -count=1 -v $U` 437 / 0 FAIL / 0 panic, the four parents' leaf counts 18 / 10 / 11 / 3.
3. **PLAN's `DECISIONS.md` `96 1`** — measured `97 1` at Task 9 (the cite-gate sentence correction wrapped one extra
   line), then **`98 1`** after the final-review fix (the Layout sentence wrapped one more): 19839 -> **19936**
   (re-measured at this close; Task 13's own ADR edit is line-count-neutral).
4. **The 22-vs-0 cite-gate firing figure** is a PLAN measurement (PLAN §6, the SPEC's line-adding comment shape),
   NOT re-run at the IMPL; the IMPL measured **27 checked / 0 moved** at Tasks 4, 5 and 9 and again at this close.
   ADR-0324's Layout paragraph attributes the 22 to the PLAN.
5. **The PLAN's one-line lint plant (`x := 1; x = 2; return x`) does NOT fire `ineffassign`** — it fired `errcheck`
   only; a multi-line variant fired `ineffassign` (2 hits, by name) beside `errcheck` (Task 12).
6. **The PLAN's `assertFlushReject` (Appendix B) was blind to dropping `stats_flush_interval: ` from the messages**
   (final review minor 1) — it pinned only `bootstrap: `. Fixed to `HasPrefix(msg, "bootstrap: stats_flush_interval: ")`;
   proven by a marked prefix-drop mutant (`NCP`, 1 before / 0 after): the new assertion reddens 27 leaves, the old one
   reads 42 RUN / 0 FAIL. The test file now departs from Appendix B by that one assertion (intended).
7. **Task 8's census base** — the PLAN's stated base `44b2de38` reads **168**; the IMPL base `d4c59f51` reads **167**.
   The one-hit difference is `STATE.md` (the PLAN commit demoted its active-phase line); growth is attributed against
   `44b2de38`.
8. **Gate (a) run 1 ABORTED** on the driver-owned receiver port race at fixture `0084` (OTLP, `0.0.0.0:33605`,
   `bind: address already in use`) after 85 PASS / 1 FAIL — a RECURRENCE, and MASKING (the rest, `0127` / `0128`
   included, unmeasured). Run 2: **130 / 0 / 0**, set 130 = 130 by name. Both kept side by side; a green rerun
   clears nothing.
9. **Gate (b) sweep 1 RED** on the SDS FetchFailure flake (`TestSDSEndToEnd_FetchFailure_BootFailsClosed`,
   `DeadlineExceeded`) — a RECURRENCE; 3/3 green in isolation; sweep 2 **245** packages rc 0. Both kept side by side.
10. **ADR-0324's Tests paragraph understated the shipped assertion after the final-review fix** (it said "the
    `bootstrap: ` prefix") — method note 97 firing a THIRD row running: a figure written true at Task 9 was made
    incomplete by a later commit of the same stage. Re-worded at this close to name the full prefix and the
    mutant (line-count-neutral; `98 1` unchanged).

### Ruled NOT fixed and banked

- **The fuzz target does not assert `FlushInterval` ∈ `[1ms, 300s)`** (final review minor 2) —
  `internal/bootstrap/statssink_fuzz_test.go` is on the PLAN's byte-untouched roster. Banked in `next-prompt.txt`.
- **`fmt.Errorf` with no format verbs** at the three reject returns (final review minor 4) — house style and
  PLAN-mandated byte-identical code. No action.
- **PLAN.md's stale "four comment sites" / "41 leaves"** — a closed stage's governing document is evidence; recorded
  here, not edited.

### CONFIRMED, not refuted

Every appendix applied as built (A, C, D, E.1-E.5, F, G, H; B except the one-assertion final-review fix); the tip
shape (391 bare; 437 / 37 FAIL lines / **29** RED by name, `cmp`-identical at Tasks 2 and 4); both fixtures RED at
subject boot with `subject proxy started cleanly` (not `direct_response.body is required`); every NC unit set and
every fixture cell of PLAN §4.3 / §4.4; **437 / 0**, `-race` clean; fixtures 3/3; **130 = 130** (106 `driver/` + 24
`inputs/`); gate (b) **245** as predicted (243 + the two fixture `driver` packages); h2spec `95 tests, 94 passed,
1 skipped, 0 failed`; fuzzers 56 targets / 48 files; the flip's sentinel shapes (check (1) ONE -> SILENT, NC-A /
NC-B TWO -> ONE, `want` 134, `ROADMAP.md` 252, six md5s unchanged); the projected §Recent tail (a UNIQUE `09-30`).

### Figures governing documents quote — re-derived at this LAST commit (method note 97)

| quoted in | figure | re-derived |
|---|---|---|
| ADR-0324, row 102 | `bootstrap.go` `21 10`, code `9 2`, three comment sites in five hunks | `git diff --numstat d4c59f51` `21 10`; hunks `-226,2 +226,2`, `-534,4 +534,4`, `-789,2 +789,6`, `-800,2 +804,8`, `-802,0 +813` |
| ADR-0324, row 102 | cite gate 27 checked / 0 moved; doc comment at `:789` +4 below last cite `:756` | `cite-gate.sh $W master` -> `27 cited lines checked, 0 moved`; `func parseStatsSinks` at `:801` |
| ADR-0324, row 102 | 39 + 3 = 42 leaves; 29 RED / 13 GREEN at the tip | 18 / 10 / 11 / 3 at this close; tip list in Task 2 |
| ADR-0324, row 102 | NC 8, 2, 7, 3, 6, 8, 26, 4, 15 | measured at Task 7, BEFORE the final-review assertion change; NOT re-run at this close |
| row 102 | `+147` / `+90`, drivers 172 / 175, READMEs 92 / 97, imports `2 / 0` | `git diff --numstat d4c59f51` identical |
| row 102 | 437 `=== RUN` / 0 FAIL after; `want` 134; `ROADMAP.md` 252 | 437 / 0 / 0 panic; 134 data rows; 252 lines |
| contract entry | delta-only `+0`, NO absolute | `:5154`, no `→` / `->`, no number |
| ADR-0324 | `DECISIONS.md` (none quoted inside the ADR) | 19936 lines, `^---$` 216, `^## ADR-` 323, bare `^## ` 331, tail ADR-0324 ACCEPTED, `^## ADR-0325` 0 |

One false-by-omission figure was found and fixed (refutation 10). The row-102 cell and the contract entry were true.

### Rulings (controller ledger, each with its cost-if-wrong)

1. Batch the 14 tasks into 6 dispatches (1-4, 5-6, 7-8, 9-11, 12, 13) — cost if wrong: coarser review granularity.
2. Task 14 (squash / merge / push / worktree remove) done by the controller — cost if wrong: none named (subagents must not push).
3. Accept PROGRESS.md Task 5/6 as the evidence record in place of a missing task-5-6 report — cost if wrong: thinner audit trail for Task 6 timings.
4. Leave PLAN.md's stale "four comment sites" / "41 leaves" unedited and record them here — cost if wrong: a future reader quoting PLAN figures.
5. Fix final-review minor 1 (pin the full prefix, proven by a prefix-drop mutant) — cost if wrong: the test file departs from Appendix B by one line.
6. Fix final-review minor 3 (ADR hunk wording and the Layout paragraph's missing `:789` hunk) — cost if wrong: none.
7. Do NOT fix final-review minor 2 (fuzz `FlushInterval` bound) — byte-untouched roster; banked — cost if wrong: an unpinned invariant under fuzz.
8. Do NOT fix final-review minor 4 (`fmt.Errorf` without args) — cost if wrong: none.

### Scope

`git diff --numstat master | wc -l` at this close: **15** paths (12 before Task 13, plus `STATE.md`,
`STATE_HISTORY.md` and `next-prompt.txt`), like for like with the squashed phase-101 IMPL `33f24624` (**17**) and
phase-100 `1ae29a1b` (**20**).

### Sentinel at this close

Re-run in full at the Task 13 tree (recorded in `next-prompt.txt`): check (1) **SILENT**, check (2) **SIX**, check (3)
**SILENT** ⇒ it does NOT fire; `stop` NOT created. The next session SELF-PICKS phase 103.
