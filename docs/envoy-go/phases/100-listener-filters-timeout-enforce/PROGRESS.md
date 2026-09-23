# Phase 100 (listener-filters-timeout-enforce) — IMPL progress

Base SHA: `9ba23146b1bfa53aff2d3f6e2e0950ecb2f50115` (master). Worktree
`/home/esa/git/envoy-go-wt-p100impl`, branch `wt-phase-100-impl`.

## Task 1: Baseline, a proven-live panic gate, and PROGRESS.md

**Step 1 — worktree already created** (skipped per dispatch instructions). Confirmed:
`pwd` = `/home/esa/git/envoy-go-wt-p100impl`; `git rev-parse --abbrev-ref HEAD` = `wt-phase-100-impl`;
`git log -1` = `9ba23146 ... PLAN correction ...`; `git status --short` clean at start.

**Step 2 — selectors resolve.**

`go list ./internal/listener/... ./internal/stats/... | wc -l` → **5** (non-zero, no `[setup failed]`):
```
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
github.com/pgdad/envoy-go/internal/stats
github.com/pgdad/envoy-go/internal/stats/dynamic
```

`go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/... | wc -l`
→ **7** (non-zero, no `[setup failed]`):
```
github.com/pgdad/envoy-go/cmd/envoy-go
github.com/pgdad/envoy-go/internal/admin
github.com/pgdad/envoy-go/internal/boot
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
github.com/pgdad/envoy-go/validate
```

**Step 3 — bare-tip rosters.**

```
W=/home/esa/git/envoy-go-wt-p100impl; S=$W/.superpowers/sdd/PLAN/scratch
cd $W && out=$(go test -count=1 -v ./internal/listener/... 2>&1); rc=$?
echo "$out" | /usr/bin/grep -oE '^=== RUN   [^ ]+' | sort > $S/base-listener.txt
echo "rc=$rc RUN=$(wc -l < $S/base-listener.txt) FAIL=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')"
```
Output: `rc=0 RUN=266 FAIL=0` — matches the plan's expected `RUN=266`. Written to `$S/base-listener.txt`.

```
cd $W && out=$(go test -count=1 -v ./internal/stats/... 2>&1); rc=$?
echo "$out" | /usr/bin/grep -oE '^=== RUN   [^ ]+' | sort > $S/base-stats.txt
echo "rc=$rc RUN=$(wc -l < $S/base-stats.txt) FAIL=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')"
```
Output: `rc=0 RUN=196 FAIL=0` — matches the plan's expected `RUN=196`. Written to `$S/base-stats.txt`.

Five-selector suite, MEASURED (not inherited from phase 99):
```
cd $W && out=$(go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/... 2>&1); rc=$?
runcount=$(echo "$out" | /usr/bin/grep -cE '^=== RUN')
failcount=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')
echo "selector='./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...' rc=$rc RUN=$runcount FAIL=$failcount"
```
Output: `selector='./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...' rc=0 RUN=421 FAIL=0`.
The count is **421**, over the five selectors `./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`.
This happens to agree with the phase-99 IMPL's carried figure, but it is a fresh measurement at this
tip (`9ba23146`), not an inherited value.

**Step 4 — panic gate proven live.** Wrote a throwaway `internal/listener/zz_p100gate_test.go`:
```go
package listener

import "testing"

func TestP100Gate(t *testing.T) {
	panic("p100-gate")
}
```
Ran `go test -count=1 -run TestP100Gate ./internal/listener/`: `rc=1`, output includes
`panic: p100-gate [recovered, repanicked]` and `FAIL	github.com/pgdad/envoy-go/internal/listener	0.006s`.
`/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` on that output = **1** (≥ 1, gate proven live).
Deleted `zz_p100gate_test.go`. `git status --short` afterward printed **nothing** (clean).

**Step 5/6 — this file created and committed** (Task 1 commit, below).

---

## Task 2: The manager-level unit file — U1, U3, U5, U5f — RED at the un-fixed tip

**Step 1 — extraction.**
```
ext () { awk -v want="$1" '$0 ~ "^## Appendix " want " —" {f=1; next} f && /^```/ {c++; if (c==1) {o=1; next} if (c==2) exit} o' "$2"; }
ext "C.1" docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > internal/listener/listener_filters_timeout_test.go
wc -l internal/listener/listener_filters_timeout_test.go
```
`wc -l` = **338**, matching the plan's expected figure. File written verbatim by extraction, never
retyped.

**Step 2 — compiles and selector matches 4.**
`go vet ./internal/listener/` → rc **0**.
```
go test -count=1 -v -run 'TestListenerFilterTimeout|TestParseListenerFiltersTimeoutZeroDisables' ./internal/listener/ 2>&1 | /usr/bin/grep -c '^=== RUN'
```
→ **4**.

**Step 3 — run at the tip; ALL FOUR RED, each for its named reason (verbatim, `rc=1`,
`FAIL github.com/pgdad/envoy-go/internal/listener 3.009s`):**

- `TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients` (U1) — **FAIL** at **3.00s**:
  `listener_filters_timeout_test.go:109: want all 20 silent clients closed by the server before 2s
  (listener_filters_timeout 1s, continue=false); got map[open:20]`. Matches the plan's named reason
  (`got map[open:20]`) and its predicted `3.00 s` exactly.
- `TestParseListenerFiltersTimeoutZeroDisables` (U3) — **FAIL**:
  `listener_filters_timeout_test.go:133: lfTimeoutMs for an explicit 0s = 15000, want 0 (disabled)`.
  Matches the plan's named reason verbatim.
- `TestListenerFilterTimeoutPreCxTimeoutByValue` (U5) — **FAIL**:
  `listener_filters_timeout_test.go:197: name existence: counter
  "listener.127_0_0_1_39649.downstream_pre_cx_timeout" is not registered before traffic (a missing name
  is a failure, never a zero)`. Matches the plan's named reason (name-existence failure) verbatim.
- `TestListenerFilterTimeoutPreCxTimeoutOnAbort` (U5f) — **FAIL**:
  `listener_filters_timeout_test.go:321: name existence: counter
  "listener.127_0_0_1_41355.downstream_pre_cx_timeout" is not registered before traffic (a missing name
  is a failure, never a zero)`. Same name-existence failure as U5, matches the plan's named reason.

No finding: all four RED for exactly their predicted reasons, no other-reason RED observed.

**Step 4 — `gofmt -l internal/listener/` prints nothing** (verified, empty output).

**Step 5 — commit** (Task 2 commit, below).

---

## Task 3: The pipeline-level unit file — U4 (i)(ii)(iii)(iv) — (i) and (iv) RED at the tip

**Step 1 — extraction.**
```
ext C.2 docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > internal/listener/listenerfilter/pipeline_deadline_test.go
wc -l internal/listener/listenerfilter/pipeline_deadline_test.go
```
`wc -l` = **209**, matching the plan's expected figure.

**Step 2 — selector check.**
```
go test -count=1 -v -run 'TestPipelineRunDeadline|TestPipelineRunZeroTimeoutHoldsSilentPeek' ./internal/listener/listenerfilter/ 2>&1 | /usr/bin/grep -c '^=== RUN'
```
→ **4**.

**Step 3 — run at the tip (`rc=1`, `FAIL github.com/pgdad/envoy-go/internal/listener/listenerfilter 4.808s`):**

- `TestPipelineRunDeadlineInterruptsSilentPeek` (U4(i)) — **FAIL** at **3.00s**:
  `pipeline_deadline_test.go:111: deadline enforced: Run did not return within 3s of a 200 ms timeout on
  a silent peer (it returned only after the peer was closed, at 3.002307283s)` and
  `pipeline_deadline_test.go:117: deadline window: Run returned after 3.002307283s, want within [150ms,
  1s] of a 200 ms timeout`. Matches the plan's predicted "RED at ~3.0 s" exactly (its own 3s timer
  closes the peer; the test FAILS rather than hangs).
- `TestPipelineRunDeadlineClearedOnSuccess` (U4(ii)) — **PASS** (0.40s). GREEN, as the liveness pin
  predicted (§4.1; blind to NC4 by design).
- `TestPipelineRunDeadlineClearedAfterTimeout` (U4(iv)) — **FAIL** at **1.00s**:
  `pipeline_deadline_test.go:178: deadline enforced: Run did not return within 1s of a 200 ms timeout on
  a silent peer (returned after the peer sent, at 1.000269746s)`. Matches the plan's predicted "RED at
  ~1.0 s" exactly.
- `TestPipelineRunZeroTimeoutHoldsSilentPeek` (U4(iii)) — **PASS** (0.40s). GREEN, as the liveness pin
  predicted (§4.1).

No finding: all four arms landed exactly where the plan predicted (RED/RED/GREEN/GREEN with matching
timings).

**Step 4 — `gofmt -l internal/listener/listenerfilter/` prints nothing; `go vet
./internal/listener/listenerfilter/` → rc 0** (both verified).

**Step 5 — commit** (Task 3 commit, below).

---

## Task 4: U2 — the abort test strengthened in place

**Step 1 — saved OLD test body.** Copied the pre-change body of
`TestUnifiedDispatchListenerFilterTimeoutAbortsConnection` (`internal/listener/manager_test.go:4036-4090`
before this task's edit) into `.superpowers/sdd/PLAN/scratch/zz_u2old_test.go`, renamed
`TestU2OLDAbortsConnection`, package `listener`. This file is in the gitignored scratch directory and is
NOT part of any commit (verified: it is not in `git status --short` output below, and is not staged in
this task's commit). Kept for Task 12's side-by-side NC6 run.

**Step 2 — applied Appendix D at anchor A7.**
```
ext D docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > $S/appendix-D.diff
wc -l $S/appendix-D.diff   # 30
cd $W && git apply --check $S/appendix-D.diff   # rc=0
git apply $S/appendix-D.diff                    # rc=0
git diff --numstat -- internal/listener/manager_test.go
```
Output: `9	3	internal/listener/manager_test.go` — matches the plan's expected `9 3` exactly. No new
import was needed; `errors`, `io`, `syscall` were already imported by `manager_test.go` (confirmed at
lines 12, 14, 21). `go vet ./internal/listener/` → rc **0**.

**Step 3 — run at the tip.**
```
go test -count=1 -v -run '^TestUnifiedDispatchListenerFilterTimeoutAbortsConnection$' ./internal/listener/
```
Result: `--- PASS: TestUnifiedDispatchListenerFilterTimeoutAbortsConnection (1.00s)`, `rc=0`. Matches the
plan's predicted "GREEN at ~1.00 s (BY DESIGN: the stub is ctx-aware)" exactly. Recorded as a pre-fix
green whose falsifier is NC6 (Task 12).

No finding: numstat, compile, and runtime behavior all matched the plan's predictions exactly.

**Step 4 — commit** (Task 4 commit, below).

---

## Task 5: Fixture `0125` — the driver

**Step 1 — port re-census at the IMPL tip.**
```
for p in 15125 15228 15229 15230 15231; do
  echo "--- $p ---"; git -C $W grep -lw -- "$p" -- test/ internal/ cmd/
done
ss -tan | /usr/bin/grep -E '15125|15228|15229|15230|15231'
ss -uan | /usr/bin/grep -E '15125|15228|15229|15230|15231'
```
Result: `15125` appears only in `test/fixtures/0123-listener-transport-protocol/README.md` and
`.../driver/driver.go` (the 0123 fixture's reserving prose); `15228`, `15229`, `15230`, `15231` appear in
zero files; both `ss -tan` and `ss -uan` produced no matching lines (no sockets bound on any of the five
ports). Matches the controller's pre-measured census exactly.

**Step 2 — extraction.**
```
ext () { awk -v want="$1" '$0 ~ "^## Appendix " want " —" {f=1; next} f && /^```/ {c++; if (c==1) {o=1; next} if (c==2) exit} o' "$2"; }
ext F.1 docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > test/fixtures/0125-listener-filters-timeout/driver/driver.go
wc -l test/fixtures/0125-listener-filters-timeout/driver/driver.go
```
`wc -l` = **727**, matching the plan's expected figure exactly (no nested-fence correction needed).

**Step 3 — compile and vet.**
```
go vet ./test/fixtures/0125-listener-filters-timeout/...   # rc 0
gofmt -l test/fixtures/0125-listener-filters-timeout/       # empty
```
Both clean.

No finding: extraction, vet, and gofmt all matched the plan's predictions exactly.

**Step 4 — commit** (Task 5 commit, below).

---

## Task 6: Fixture `0125` — `README.md`, `expectations.yaml`, and the FOUR registration gates

**Step 1 — extraction.**
```
ext F.2 docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > test/fixtures/0125-listener-filters-timeout/README.md
wc -l test/fixtures/0125-listener-filters-timeout/README.md   # 79
ext F.3 docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > test/fixtures/0125-listener-filters-timeout/expectations.yaml
wc -l test/fixtures/0125-listener-filters-timeout/expectations.yaml   # 30
```
Both match the plan's expected figures exactly (no nested-fence correction needed).

**Step 2 — the blank import.**
```
ext F.4 docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > $S/appendix-F4.diff
wc -l $S/appendix-F4.diff        # 12 (diff header + one hunk)
git apply --check $S/appendix-F4.diff   # rc 0
git apply $S/appendix-F4.diff           # rc 0
git diff --numstat -- test/differential/runner_test.go
```
Output: `1	0	test/differential/runner_test.go` — matches the plan's expected `1 0` exactly. The
import lands in sorted position, immediately after `0124-listener-sni-longest-suffix/driver` and before
the non-fixture `test/helpers` import.

**Step 3 — the four registration gates, by the SET.**
```
extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
extract $W/test/differential/runner_test.go > $S/imports.txt
ls -d $W/test/fixtures/*/ | xargs -n1 basename | sort > $S/dirs.txt
wc -l < $S/imports.txt; wc -l < $S/dirs.txt
comm -23 $S/imports.txt $S/dirs.txt; comm -13 $S/imports.txt $S/dirs.txt
/usr/bin/grep -c '^0125-listener-filters-timeout$' $S/imports.txt
/usr/bin/grep -c '^0125-listener-filters-timeout$' $S/dirs.txt
```
Result: **127** and **127**; both `comm` outputs empty; both grep counts = **1**. Matches the plan's
expected figures exactly. Gate 1 (`RegisterFixture` in `init()`, present in the extracted driver.go from
Task 5) and Gate 3 (name == directory basename `0125-listener-filters-timeout`, byte for byte) and Gate 4
(the `NNNN-` shape, satisfied by the directory name) are satisfied by construction; Gate 2 (the import) is
proven live by this SET check.

**Step 4 — NC the extractor**, in scratch copies of `runner_test.go` only (never the tracked file):
- **Rename** the import's directory token (`0125-listener-filters-timeout` → `0125-RENAMED`) in
  `$S/nc-rename-runner_test.go`: `comm -23` now shows `0125-RENAMED` (in imports, not dirs) and
  `comm -13` now shows `0125-listener-filters-timeout` (in dirs, not imports) — **BOTH** directions fire,
  as predicted.
- **Delete** the import line in `$S/nc-delete-runner_test.go`: import count drops to 126, `comm -23` is
  empty, `comm -13` shows `0125-listener-filters-timeout` — **only** `comm -13` fires, as predicted.

No finding: extraction, apply, gate check, and both NC arms matched the plan's predictions exactly.

**Step 5 — commit** (Task 6 commit, below).

---

## Task 7: RECORD the un-fixed tip — every falsifier RED, for its NAMED reason

**This record cannot be recreated after Task 8 (the production edit).**

### Step 1 — unit suite at the tip

```
out=$(go test -count=1 -v ./internal/listener/... ./internal/stats/... 2>&1); rc=$?
echo "$out" | /usr/bin/grep -c '^=== RUN'
echo "$out" | /usr/bin/grep -E '^--- FAIL'
```

Result: `rc=1`, `=== RUN` count = **470**, exactly the six top-level FAILs of §4.2:

```
--- FAIL: TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients (3.00s)
--- FAIL: TestParseListenerFiltersTimeoutZeroDisables (0.00s)
--- FAIL: TestListenerFilterTimeoutPreCxTimeoutByValue (0.00s)
--- FAIL: TestListenerFilterTimeoutPreCxTimeoutOnAbort (0.00s)
--- FAIL: TestPipelineRunDeadlineInterruptsSilentPeek (3.00s)
--- FAIL: TestPipelineRunDeadlineClearedAfterTimeout (1.00s)
```

**Roster diff against Task 1's baseline** (`base-listener.txt` 266 names + `base-stats.txt` 196 names =
462 unique base names, vs. 470 tip names):

```
comm -13 $S/t7-base-combined.txt $S/t7-tip-names.txt   # additions
comm -23 $S/t7-base-combined.txt $S/t7-tip-names.txt   # removals
```

Additions (exactly 8, matches 470 = 266 + 196 + 8 exactly):
```
TestListenerFilterTimeoutPreCxTimeoutByValue
TestListenerFilterTimeoutPreCxTimeoutOnAbort
TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients
TestParseListenerFiltersTimeoutZeroDisables
TestPipelineRunDeadlineClearedAfterTimeout
TestPipelineRunDeadlineClearedOnSuccess
TestPipelineRunDeadlineInterruptsSilentPeek
TestPipelineRunZeroTimeoutHoldsSilentPeek
```
Removals: **none** (empty output). Matches the plan's expectation exactly: eight new top-level names
added, nothing removed. Full output saved at `$S/t7-unit-tip.log`.

No finding: RUN count, rc, the six named FAILs, and the roster diff all matched the plan's predictions
exactly.

### Step 2 — fixture at the tip, ALONE

```
out=$(go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0125-listener-filters-timeout$' -timeout 30m 2>&1); rc=$?
```
Output saved to `$S/t7-fixture-tip.log`. `rc=1`.

Confirmed `=== RUN   TestDifferential/0125-listener-filters-timeout` present (line 2) and **zero** `SKIP`
lines. Zero panic-gate hits (`^panic:|DATA RACE|SIGSEGV`). Reference container pinned by digest
`envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`, confirmed in
the testcontainers log line. Only this session's ryuk/reference containers were created; they were
terminated by the test itself (`🚫 Container terminated: 1b64fc377b55`); no pre-existing container
(`cpj-p14-*`, `golink-ai`, any `reaper_*`) was touched.

Wall time: `--- FAIL: TestDifferential (35.70s)` — within the plan's 33.9-36.4 s window.

**Per-arm result, exactly as predicted by §4.3:**

- **Reference green on every arm.** `ref F1 closed=true ms=1001 kind=FIN`; all 50 `ref F2[NN]` closed=true
  at 1001-1002 ms, kind=FIN; `ref Z1 closed=false ms=16506 kind=open` (0s disables — Z1 is meant to hold);
  `ref N1 closed=false ms=2800 kind=open` (no listener_filters — nothing to time); `ref F3/T1/T2/T3` all
  `status=200` with the expected bodies. No reference-side failure line anywhere in the log.
- **F1 (`l_false` silent) — RED**, subject: `subj F1 closed=false ms=3000 bytes=0 kind=open`, then:
  `runner_test.go:1356: subj F1 l_false: silent client NOT closed by the server within 3000 ms (kind=open)
  — listener_filters_timeout 1s with continue:false must close it`. Matches "open at 3000 ms" exactly.
- **F2 (`l_false` 50 concurrent silent) — RED**, subject: all 50 `subj F2[NN] closed=false ms=3000
  kind=open`, then: `runner_test.go:1356: subj F2 l_false: 50 of 50 concurrent silent clients NOT closed
  within 3000 ms: #0(open@3000ms) ... #49(open@3000ms)`. Matches "50/50 open" exactly.
- **All five S rows — RED, series ABSENT** (subject side; reference side present with the right values):
  ```
  ref S l_false     envoy_listener_downstream_pre_cx_timeout{address="0.0.0.0_15125"}      = 51 (present=true)  want 51
  ref S l_true      envoy_listener_downstream_pre_cx_timeout{address="0.0.0.0_15228"}      = 1  (present=true)  want 1
  ref S l_true_tls  envoy_listener_downstream_pre_cx_timeout{address="0.0.0.0_15229"}      = 1  (present=true)  want 1
  ref S l_zero      envoy_listener_downstream_pre_cx_timeout{address="0.0.0.0_15230"}      = 0  (present=true)  want 0
  ref S l_nofilt    envoy_listener_downstream_pre_cx_timeout{address="0.0.0.0_15231"}      = 0  (present=true)  want 0

  subj S l_false     envoy_listener_downstream_pre_cx_timeout{address="127_0_0_1_20016"} = 0 (present=false) want 51
    runner_test.go:1356: subj S l_false: ... ABSENT (series present: []), want present and == 51
  subj S l_true      envoy_listener_downstream_pre_cx_timeout{address="127_0_0_1_20017"} = 0 (present=false) want 1
    runner_test.go:1356: subj S l_true: ... ABSENT (series present: []), want present and == 1
  subj S l_true_tls  envoy_listener_downstream_pre_cx_timeout{address="127_0_0_1_20018"} = 0 (present=false) want 1
    runner_test.go:1356: subj S l_true_tls: ... ABSENT (series present: []), want present and == 1
  subj S l_zero      envoy_listener_downstream_pre_cx_timeout{address="127_0_0_1_20019"} = 0 (present=false) want 0
    runner_test.go:1356: subj S l_zero: ... ABSENT (series present: []), want present and == 0
  subj S l_nofilt    envoy_listener_downstream_pre_cx_timeout{address="127_0_0_1_20020"} = 0 (present=false) want 0
    runner_test.go:1356: subj S l_nofilt: ... ABSENT (series present: []), want present and == 0
  ```
  The `downstream_pre_cx_timeout` counter does not exist yet on the subject side for any of the five
  listeners — matches "series ABSENT" exactly for all five rows, including the two zero-want rows
  (`l_zero`, `l_nofilt`), which are ABSENT rather than present-and-0 (the counter is simply never
  registered pre-fix).
- **CompareBytes — RED:**
  ```
  runner_test.go:1295: differential mismatch:
      first divergence at offset 18
      ref [2..34]:  ... l_false closed=true in_window=t...
      subj[2..34]:  ... l_false closed=false in_window=...
  ```
  Matches "CompareBytes RED" exactly.
- **Everything else green**, matching §4.3's g/g cells: F3 (`status=200 body="INDEXED l_false\n"`), T1/T2
  (`status=200 body="INDEXED l_true\n"`), T3 (`status=200 body="DEFAULT l_true_tls\n"`), N1 (both sides
  `closed=false ms=2800 kind=open`, matched, no failure line), Z1 (both sides `closed=false ms≈16506-16512
  kind=open`, matched, no failure line).

No finding: the subject was RED on exactly F1, F2, all five S rows, and CompareBytes, and green everywhere
else; the reference was green on every arm — matches §4.3's tip row exactly, for the named reasons.

### Step 3 — reference close spread and subject open count

**Reference close spread** (51 closes: F1's 1 + F2's 50), read directly from the log's own computed line:
```
ref F1+F2 SPREAD n=51 min=1001 max=1002 mean=1001.0 sd=0.19 F2 kinds FIN=50
```
51 closes, min 1001 ms, max 1002 ms, mean 1001.0 ms, sd 0.19 ms, all kind=FIN. This falls at the low edge
of but inside the plan's expected **1000-1004 ms** spread (§4.3, §0.13); the measured band (1001-1002 ms)
is narrower than but fully contained within the predicted envelope. Individual sample ms values above
range 1001-1002 (see the 51 `ref F1`/`ref F2[NN]` lines pasted in Step 2).

**Subject open count:** 51 (F1's 1 + F2's 50), all at `ms=3000` `kind=open`, `closed=false` — the
subject's own computed line confirms: `subj F1+F2 SPREAD n=0 (no close observed) F2 kinds open=50` (n=0
closes observed; the 50 F2 opens plus F1's 1 open = 51 total silent-client connections that reached the
3000 ms driver cutoff still open, none closed by the un-fixed subject).

No finding: both figures were read directly from the driver's own computed summary lines, not
re-derived, and both match the plan's per-arm predictions for the un-fixed tip.

### Step 4 — commit (Task 7 commit, below).

## Task 8: THE PRODUCTION EDIT — PB1, code hunks only

### Step 1 — extract and apply Appendix A

```
ext A docs/.../PLAN.md > $S/patch-PB1.diff
git -C $W apply --check $S/patch-PB1.diff   # CHECK_OK
git -C $W apply $S/patch-PB1.diff
git -C $W diff --numstat
```
Result:
```
15	0	internal/listener/listenerfilter/pipeline.go
14	6	internal/listener/manager.go
```
Matches the brief's `15 0` / `14 6` exactly. `errors` was already imported in `manager.go` (line 7), so
`errors.Is` required no new import.

### Step 2 — layout gate

```
bash $S/layout-gate.sh $W master
```
Result:
```
PASS (a) pipeline.go:33-37,43 byte-identical to master
PASS (b) pipeline.go import block unchanged
PASS (c) one code WithTimeout, at internal/listener/listenerfilter/pipeline.go:43
FAIL (d) tls_inspector.go numstat [] (want 3 3), non-comment changed lines = 0 (want 0)
PASS (e) no line-count-changing pipeline.go hunk at/above line 44
layout-gate: 1 failed sub-gate(s) (base master)
```
Exit status 1. Matches the predicted shape exactly: a, b, c, e PASS; d FAILS by design because
`tls_inspector.go` is untouched until Task 11 (its comment reconciliation lands in Appendix B).

### Step 3 — gofmt / vet

```
gofmt -l internal/listener/     # prints nothing
go vet ./internal/listener/...  # rc 0
```
Both clean.

### Step 4 — symbol assertions

```
git -C $W grep -c 'context.AfterFunc' -- internal/listener/listenerfilter/pipeline.go   # 1
git -C $W grep -c 'downstream_pre_cx_timeout' -- internal/listener/manager.go           # 1
git -C $W grep -c 'errors.Is(err, context.DeadlineExceeded)' -- internal/listener/manager.go  # 2
```
All ≥ 1. The `errors.Is(err, context.DeadlineExceeded)` count of 2 is not a finding: line 535 is a
pre-existing occurrence elsewhere in `manager.go`, and line 1356 is the new `serveConnection` step-(4)
Inc site added by this patch.

**Note on help text:** as flagged by the brief, `go test ./internal/stats/...` is RED after Task 8 alone
(`TestHelpText_KeySetExact` — the new stat name has no help-text entry yet); Task 9 (below) lands the pair
that turns it GREEN. Recorded honestly, not fixed early.

### Step 5 — commit

Committed `internal/listener/listenerfilter/pipeline.go` and `internal/listener/manager.go` only (pathspec
per the brief), message exactly as specified. Commit `17a1dda8`.

## Task 9: The help-text PAIR — ONE commit

### Step 1 — each half alone is RED

Applied Appendix E.2 (`helptext_test.go`, `1 0`) alone:
```
go test -count=1 -v ./internal/stats/
```
Result: `rc=1`, `--- FAIL: TestHelpText_KeySetExact (0.00s)` and `--- FAIL: TestHelpText_NoSelfEqualHelp (0.00s)`.
Both reddened, matching the brief exactly.

Reverted (`git checkout -- internal/stats/helptext_test.go`), applied Appendix E.1 (`name.go`, `2 0`)
alone:
```
go test -count=1 -v ./internal/stats/
```
Result: `rc=1`, `--- FAIL: TestHelpText_KeySetExact (0.00s)` only — `NoSelfEqualHelp` stayed GREEN.
Matches the brief exactly.

Reverted (`git checkout -- internal/stats/name.go`).

### Step 2 — apply both

```
git -C $W apply $S/patch-E1.diff
git -C $W apply $S/patch-E2.diff
git -C $W diff --numstat
```
Result: `1 0 internal/stats/helptext_test.go`, `2 0 internal/stats/name.go`.
```
go test -count=1 -v ./internal/stats/
```
Result: `rc=0`, `=== RUN` count **146**. Matches the brief's "RUN 146" exactly.

### Step 3 — commit

Committed `internal/stats/name.go` and `internal/stats/helptext_test.go` together, ONE commit, message
exactly as specified. Commit `a3d6524a`.

## Task 10: ALL ARMS GREEN — unit, race, fixture x3

### Step 1 — full unit suite

```
go test -count=1 -v ./internal/listener/... ./internal/stats/...
```
Result: `rc=0`, `=== RUN` count **470**, 0 `--- FAIL`, 0 `panic:`/`DATA RACE`/`SIGSEGV`. Matches "RUN 470,
0 FAIL, rc 0" exactly; roster unchanged from Task 7's (same names, outcome flipped to green).

### Step 2 — U1 x3 and U4 x3 under -race

```
go test -count=1 -race -v ./internal/listener/... -run 'TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients'
go test -count=1 -race -v ./internal/listener/... -run 'TestPipelineRun'
```
Each run x3:
```
U1 run1 RC=0 FAIL=0 RACE=0
U1 run2 RC=0 FAIL=0 RACE=0
U1 run3 RC=0 FAIL=0 RACE=0
U4 run1 RC=0 FAIL=0 RACE=0
U4 run2 RC=0 FAIL=0 RACE=0
U4 run3 RC=0 FAIL=0 RACE=0
```
`TestPipelineRun` selector matches 11 subtests each run (`TestPipelineRunDeadlineInterruptsSilentPeek`,
`TestPipelineRunDeadlineClearedOnSuccess`, `TestPipelineRunDeadlineClearedAfterTimeout`,
`TestPipelineRunZeroTimeoutHoldsSilentPeek`, `TestPipelineRunZeroFilters`, `TestPipelineRunContinuePath`,
`TestPipelineRunStopIterationPath`, `TestPipelineRunFilterError`, `TestPipelineRunTimeoutSharedAcrossFilters`,
`TestPipelineRunZeroTimeoutDisablesEnforcement`, `TestPipelineRunPropagatesError`). All green, 0 DATA RACE
across all 6 runs.

### Step 3 — full package under -race

```
go test -count=1 -race ./internal/listener/...
```
Result: `rc=0`:
```
ok  	github.com/pgdad/envoy-go/internal/listener	8.387s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	2.255s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	1.012s
```

### Step 4 — fixture ALONE x3

```
go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0125-listener-filters-timeout$' -timeout 30m
```
Run 1: `rc=0`, `--- PASS: TestDifferential (36.11s)`, `=== RUN   TestDifferential` +
`=== RUN   TestDifferential/0125-listener-filters-timeout` both present, 0 SKIP.
```
ref F1+F2 SPREAD n=51 min=1000 max=1002 mean=1001.1 sd=0.42 F2 kinds FIN=50
subj F1+F2 SPREAD n=51 min=1000 max=1014 mean=1000.8 sd=3.29 F2 kinds FIN=50
```

Run 2: `rc=0`, `--- PASS: TestDifferential (35.52s)`, both RUN lines present, 0 SKIP.
```
ref F1+F2 SPREAD n=51 min=1001 max=1030 mean=1015.3 sd=13.72 F2 kinds FIN=50
subj F1+F2 SPREAD n=51 min=1000 max=1021 mean=1002.1 sd=4.73 F2 kinds FIN=50
```
**Finding:** the reference's own close spread on this run (max 1030 ms, sd 13.72) reads wider than the
plan's predicted ~1000-1004 ms reference envelope (§0.13, §4.3). It remains comfortably inside the task's
`[700, 1800]` STOP window, so this is recorded as a finding, not a STOP: the reference side itself shows
more jitter run-to-run than the PLAN-stage measurement implied, independent of the subject fix.

Run 3: `rc=0`, `--- PASS: TestDifferential (35.43s)`, both RUN lines present, 0 SKIP.
```
ref F1+F2 SPREAD n=51 min=1001 max=1003 mean=1001.7 sd=0.61 F2 kinds FIN=50
subj F1+F2 SPREAD n=51 min=1000 max=1014 mean=1001.3 sd=3.72 F2 kinds FIN=50
```

All three runs PASS, 0 FAIL, 0 SKIP, all six close-spread bounds (three ref, three subj) fall inside
`[700, 1800]`. Subject spreads land inside the plan's predicted ~1000-1021 ms band on all three runs; the
reference matches the predicted ~1000-1004 ms band on runs 1 and 3 and is a wider (but still in-window)
finding on run 2. Full logs: `$S/t10-fixture-run1.log`, `$S/t10-fixture-run2.log`, `$S/t10-fixture-run3.log`.

### Step 5 — commit

This commit updates `PROGRESS.md` only (per the brief), and documents Tasks 8, 9, and 10 together, since
Tasks 8 and 9's own commits (`17a1dda8`, `a3d6524a`) carried only their code/test pathspecs per their
briefs' exact instructions.

## Task 11: The occurrence-set reconciliation in CODE — comments, under TWO gates

### Step 1 — extract and apply Appendix B

```
ext B docs/.../PLAN.md > $S/patch-COMMENTS.diff
git -C $W apply --check $S/patch-COMMENTS.diff   # CHECK_OK, applies on Appendix A
git -C $W apply $S/patch-COMMENTS.diff
git -C $W diff --numstat HEAD
```
Result:
```
2	2	internal/listener/listenerfilter/pipeline.go
3	3	internal/listener/listenerfilter/tls_inspector/tls_inspector.go
11	10	internal/listener/manager.go
```
Matches the brief's `2 2`, `3 3`, `11 10` exactly.

### Step 2 — gate the KIND

```
git -C $W diff -U0 HEAD | /usr/bin/grep -E '^[+-]' | /usr/bin/grep -vE '^(\+\+\+|---) ' \
  | sed -E 's/^[+-][[:space:]]*//' | /usr/bin/grep -vc '^//'
```
Result: `0`. Every changed line across all three files is a `//` comment.

### Step 3 — gate the SHAPE

```
bash $S/layout-gate.sh $W master
```
Result:
```
PASS (a) pipeline.go:33-37,43 byte-identical to master
PASS (b) pipeline.go import block unchanged
PASS (c) one code WithTimeout, at internal/listener/listenerfilter/pipeline.go:43
PASS (d) tls_inspector.go numstat 3 3, comment-only
PASS (e) no line-count-changing pipeline.go hunk at/above line 44
layout-gate: 0 failed sub-gate(s) (base master)
```
0 failed sub-gates, exit 0.

### Step 4 — show the gates fire (scratch copy)

Made a plain file copy of the worktree (`cp -r`, NOT `git worktree add` — this copy shares the SAME
`.git/worktrees/envoy-go-wt-p100impl` gitdir/index as `$W`, so every plant/revert in it used plain `cp`
of the known-good file from `$W`, never `git checkout`, to avoid touching the shared index; confirmed the
real `$W` was unaffected after each drill via `git -C $W diff --numstat HEAD` and a clean layout-gate run).

Plant 1 — a line-adding edit to the `Run` doc comment (added one bullet line after the OnDestroy bullet):
```
bash $S/layout-gate.sh <scratch-copy> master
```
Result:
```
FAIL (a) pipeline.go:33-37,43 differ from master
PASS (b) pipeline.go import block unchanged
FAIL (c) code WithTimeout hits = [.../pipeline.go:44] want [.../pipeline.go:43]
PASS (d) tls_inspector.go numstat 3 3, comment-only
FAIL (e) pipeline.go hunk(s) above line 45 change the line count: [-31,0 +32,1]
layout-gate: 3 failed sub-gate(s)
```
**(e) FAILS**, as required. (a) and (c) also fail as knock-on effects of the line shift pushing
`:33-37`/`:43` down by one — matches §6's "plant e: the Run doc grows a line" row (F P F P F, exit 3)
exactly. Reverted via `cp` from `$W`; a follow-up gate run confirmed 0 failed sub-gates again.

Plant 2 — a code change in `tls_inspector.go` at shape `3 3` (replaced the middle comment line of the
3-line block with `_ = ctx // planted CODE line for the layout gate drill (shape 3 3)`, keeping 3
removed / 3 added):
```
bash $S/layout-gate.sh <scratch-copy> master
```
Result:
```
PASS (a) ...
PASS (b) ...
PASS (c) ...
FAIL (d) tls_inspector.go numstat [3 3] (want 3 3), non-comment changed lines = 1 (want 0)
PASS (e) ...
layout-gate: 1 failed sub-gate(s)
```
**(d) FAILS**, as required, even though the numstat shape (`3 3`) is unchanged — the gate is catching the
COMMENT-ONLY invariant, not just the line count. Reverted via `cp` from `$W`; a follow-up gate run
confirmed 0 failed sub-gates again. Scratch copy removed after the drill.

### Step 5 — re-run the unit suite, gofmt, lint

```
go test -count=1 -v ./internal/listener/... ./internal/stats/...
```
Result: `rc=0`, `=== RUN` count **470**, 0 FAIL.
```
gofmt -l internal/listener/ internal/stats/     # prints nothing
GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/... ./internal/stats/...
```
Result: `rc=0`, no findings printed.

### Step 6 — commit

Committed `internal/listener/listenerfilter/pipeline.go`,
`internal/listener/listenerfilter/tls_inspector/tls_inspector.go`, `internal/listener/manager.go`, and
this `PROGRESS.md` update together.

## Task 12: The NC roster — eight mutants, each proven able to fire, scored PER ARM

**No production or test file in the real worktree is touched by this task.** All eight mutants (Appendix
H.1-H.8) are applied and reversed in a THROWAWAY detached worktree, `/home/esa/git/envoy-go-wt-p100nc`,
created at the Task 11 tip `d163a009`. The only real-worktree edit is this `PROGRESS.md`.

### Step 1 — throwaway worktree

```
git -C /home/esa/git/envoy-go-wt-p100impl worktree add --detach /home/esa/git/envoy-go-wt-p100nc d163a009
```
Result: `Preparing worktree (detached HEAD d163a009)`. `git worktree list` shows it as `(detached HEAD)`
alongside the unrelated `wt-phase-100-impl-docs` worktree (not touched — belongs to a different session).

Copied the Task-4 OLD-U2 body in, per the brief:
```
cp .superpowers/sdd/PLAN/scratch/zz_u2old_test.go /home/esa/git/envoy-go-wt-p100nc/internal/listener/
```
`git -C /home/esa/git/envoy-go-wt-p100nc status --short` → `?? internal/listener/zz_u2old_test.go` (the
ONLY entry) for the rest of this task; it is kept in place across every NC (it declares
`TestU2OLDAbortsConnection`, package `listener`, no import collisions) and removed only when the worktree
itself is removed at Step 7.

Extracted the eight appendices with the plan's `ext` awk function; each extracted to a clean diff whose
`NC<n>` marker count is exactly 1 pre-application (`/usr/bin/grep -c 'NC<n>'` on the extracted `.diff`).
`git apply --check` accepted all eight with **zero** complaints (no offset-hunk rejection). A
`patch -p1 --dry-run` cross-check on the same tree reports the offsets actually taken when applied for
real:

| Appendix | file | offset |
|---|---|---|
| H.1 | `internal/listener/manager.go` hunk | +1 line (1352→1353) |
| H.1 | `internal/listener/listenerfilter/pipeline.go` hunk | 0 |
| H.2 | `internal/listener/manager.go` | +1 line (1354→1355) |
| H.3 | `internal/listener/manager.go` | 0 |
| H.4 | `internal/listener/listenerfilter/pipeline.go` | 0 |
| H.5 | `internal/listener/listenerfilter/pipeline.go` | 0 |
| H.6 | `internal/listener/manager.go` | +1 line (1358→1359) |
| H.7 | `internal/listener/manager.go` | +1 line (1353→1354) |
| H.8 | `internal/listener/manager.go` | +1 line (1353→1354) |

The `manager.go` offset is the Task-11 comment reconciliation (Appendix B, one added prose line before the
`serveConnection` block) shifting every hunk below it down by exactly one line relative to PB1's own line
numbers; `pipeline.go` hunks land at their PB1 line numbers unchanged (Task 11's `pipeline.go` edit was
comment-only at `:3-3` inside `tls_inspector.go`, not `pipeline.go`). `patch -p1` (not `git apply`) is used
for every real application below, per the brief.

### Step 2 — mechanism, written BEFORE running (method note 7d), from §7

| NC | mutation | mechanism to a failure |
|---|---|---|
| NC1 | PB1 → P0: the `context.AfterFunc` block removed from `pipeline.go`; a second, raw `SetReadDeadline` clock added around `p.Run` in `manager.go` | the socket deadline wins on its own clock, `tls_inspector` maps the resulting read error to `raw_buffer`/`Continue`, and `Run` returns `nil` — nothing downstream ever observes a `ctx` cancellation, so the counter and the abort path are never reached by the ONE-clock design |
| NC2 | `rt.downstreamPreCxTimeout.Inc()` deleted (kept referenced via `_ =` so it still compiles) | the counter object exists and is wired, but its value never moves off 0 |
| NC3 | the `0s`→disabled fold in `parseListenerFiltersTimeout` reverted to fall through to `defaultMs` | an explicit `0s` config is silently treated as "absent" (15000 ms), so the 15 s default deadline fires instead of "never" |
| NC4 | the deferred `ds.SetReadDeadline(time.Time{})` clear deleted from the `AfterFunc`'s cleanup | a deadline that fired stays set in the past on the raw connection, so any handoff after a timeout inherits an already-expired deadline and its next read fails spuriously |
| NC5 | the `if !stop() { <-fired }` wait replaced with `_ = stop(); _ = fired` (no wait) | `Run` can return and its cleanup can race the `AfterFunc` goroutine that is still calling `SetReadDeadline`; a narrow, non-deterministic window with no assertable, deterministic failure path (declared BLIND) |
| NC6 | `_ = pkConn.Close()` deleted from the `!continueOnLfTimeout` abort branch | a `false`-policy connection whose pipeline timed out is never closed — the client is left holding an open, silent socket forever, which is exactly the pre-fix (tip) behavior for that one branch |
| NC7 | `errors.Is(err, context.DeadlineExceeded)` replaced with `err != nil` | ANY pipeline error (including a manager-ctx `Canceled`, which is explicitly NOT a timeout per U5's cancel half) now books the counter, over-counting |
| NC8 | the `Inc()` guarded with `&& rt.continueOnLfTimeout` | a `continue_on_listener_filters_timeout: false` timeout (the abort path) never books the counter, even though the reference books it under BOTH continue values |

### Step 3-4 — unit-level NCs, apply / assert / run / reverse / assert

Per-NC procedure (repeated verbatim for every application): `patch -p1 < $S/nc/appendix-H.<n>.diff`;
`/usr/bin/grep -c 'NC<n>' <patched file(s)>` ≥ 1 asserted BEFORE reading any test output;
`go test -count=1 -v ./internal/listener/... ./internal/stats/... 2>&1 | tee $S/nc/logs/<label>.log`;
score every §4.1 arm from the saved log; `patch -R -p1 < $S/nc/appendix-H.<n>.diff`; `/usr/bin/grep -c
'NC<n>'` reads 0; `git -C /home/esa/git/envoy-go-wt-p100nc status --short` shows only
`?? internal/listener/zz_u2old_test.go`.

**Infrastructure defect found before NC1's first run:** the scratch `zz_u2old_test.go` has NO `import`
block (verified `cat -A` on the source copy — file starts `package listener` then falls straight into a
doc comment and `func TestU2OLDAbortsConnection`). Compiling it as-is fails the WHOLE `internal/listener`
package (`undefined: testing`, `listenerv3`, `corev3`, `durationpb`, `time`; confirmed by a first run:
`internal/listener [build failed]`, `RUN=286` instead of the expected 470+1, and the failure is silent —
no `--- FAIL` line, just a build error, so a naive `FAIL=0` read on that log would have been wrong in the
other direction). Fixed by adding the standard import block (`context`, `net`, `testing`, `time`, the two
go-control-plane v3 packages, `durationpb`, and `internal/stats`) to the THROWAWAY tree's copy only — the
real worktree's `.superpowers/sdd/PLAN/scratch/zz_u2old_test.go` is untouched. `gofmt -l` and `go vet
./internal/listener/` are silent on the corrected copy. All results below are from the corrected copy.

### Step 3-4 results — unit-level NCs vs §4.1

Every run below is `go test -count=1 -v ./internal/listener/... ./internal/stats/...`, base `RUN=470` (PB1)
+ 1 (`TestU2OLDAbortsConnection`, always resident) = **471** every time; no `[build failed]`, no `[no
tests to run]`, no panic/DATA RACE/SIGSEGV.

**NC1 (×3, U1/U5-cancel probabilistic per §7):**

| run | RUN | FAIL | U1 (of 20 silent) | U4(i) | U4(iv) | U5 cancel | everything else |
|---|---|---|---|---|---|---|---|
| 1 | 471 | 4 | **R** fellThrough 9 (closed 11) | **R** | **R** | **R** (acted at 1000ms) | G |
| 2 | 471 | 4 | **R** fellThrough 5 (closed 15) | **R** | **R** | **R** (acted at 1001ms) | G |
| 3 | 471 | 4 | **R** fellThrough 6 (closed 14) | **R** | **R** | **R** (acted at 1000ms) | G |

Same 4 tests reddened all 3/3 runs: `TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients` (U1),
`TestListenerFilterTimeoutPreCxTimeoutByValue` (U5, cancel half only — the other three halves passed all
3/3), `TestPipelineRunDeadlineInterruptsSilentPeek` (U4(i)), `TestPipelineRunDeadlineClearedAfterTimeout`
(U4(iv)). U2-OLD, U2-NEW, U3, U4(ii), U4(iii), U5 (name/immediate/silent-value/silent-liveness), U5f, U6
all GREEN 3/3 — matches §4.1's NC1 column and §7's "U1 (4/4, probabilistic), U4(i), U4(iv), U5 cancel
(4/4, deterministic) redden; U3, U6, U5 silent stay green" exactly. Marker asserted =1 before every run,
=0 after every reversal; `git status --short` clean (only the zz file) after each.

**NC2 (×1):** `RUN=471 FAIL=2` — `TestListenerFilterTimeoutPreCxTimeoutByValue` and
`TestListenerFilterTimeoutPreCxTimeoutOnAbort` (U5f). Inside U5: the **silent** half reddens (value stuck
at 0, want 1) — the direct hit — and the **cancel** half ALSO reddens as a stated knock-on (reads 0, want
1, because it asserts the CUMULATIVE value and the prior silent increment never landed): `cancel:
listener.….downstream_pre_cx_timeout = 0 after a manager-ctx cancel, want 1`. This matches §4.1's
footnote verbatim ("NC2 reddens U5's cancel half only as a knock-on... not an independent detection").
U1, F1/F2 (fixture, not applicable at unit level), CompareBytes (fixture) stay outside scope; U1 itself
stayed GREEN as predicted. Marker 1→0 confirmed.

**NC3 (×1):** `RUN=471 FAIL=1` — only `TestParseListenerFiltersTimeoutZeroDisables` (U3). Matches §4.1/§7
exactly (U1, U4, U5 all green). Marker 1→0 confirmed.

**NC4 (×1):** `RUN=471 FAIL=2` — `TestListenerFilterTimeoutPreCxTimeoutByValue` (U5, **silent liveness**
half only: `silent liveness: want the fall-through connection still open (client deadline), got n=0
err=EOF`) and `TestPipelineRunDeadlineClearedAfterTimeout` (U4(iv)). U4(ii) stayed GREEN (§0.2's liveness
pin, structurally blind to NC4 because the peer sends before the deadline fires). Matches §7's NC4 row
exactly. Marker 1→0 confirmed.

**NC5 (×5, declared BLIND, +1 `-race`):** all five full-suite runs: `RUN=471 FAIL=0 rc=0`. The dedicated
`-race` run on `./internal/listener/...` alone: `RUN=275 FAIL=0 rc=0`, **0 DATA RACE** lines. Recorded as
blind, not faked, matching §4.1's "The full ... ×5 read 470 RUN, 0 FAIL, rc 0; -race read 0 races" (our
count reads 471, +1 for the always-resident `TestU2OLDAbortsConnection`, which is itself unaffected by
NC5). Marker 1→0 confirmed after the sixth (final) run.

**NC6 (×1, OLD+NEW U2 side by side — `zz_u2old_test.go` is resident for every run in this task, so this
is simply the NC6 run read against both U2 bodies):** `RUN=471 FAIL=3`.
- `TestU2OLDAbortsConnection` (U2-OLD): **PASS (3.00s)** — the OLD test's own read deadline is 3s and its
  assertion accepts ANY non-nil read error including the CLIENT's own deadline expiry, so it passes even
  though the server never closed the connection. **This is the vacuity**, exactly as predicted ("PASS at
  3.00 s — VACUOUS").
- `TestUnifiedDispatchListenerFilterTimeoutAbortsConnection` (U2-NEW, Appendix D strengthened): **FAIL
  (3.00s)** — the strengthened assertion requires the SERVER to close before 2s (EOF/ECONNRESET), which
  under NC6 never happens.
- `TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients` (U1): **FAIL** — `got map[closed:4
  open:16]` (the abort branch's `false` connections are never closed under NC6).
- `TestListenerFilterTimeoutPreCxTimeoutOnAbort` (U5f): **FAIL** — `i/o timeout after 3.000931534s` (the
  client's own 3s read deadline fires first because the server never closes).
- U3, U5 (the other three halves) stayed GREEN.

Matches §7's NC6 row exactly: "must redden U1, U2-NEW, U5f; stays green U2-OLD (3.00 s — the vacuity),
U3, U5." Marker 1→0 confirmed.

**NC7 (×1):** `RUN=471 FAIL=1` — `TestListenerFilterTimeoutPreCxTimeoutByValue`, cancel half: `cancel:
listener.….downstream_pre_cx_timeout = 2 after a manager-ctx cancel, want 1` — reads exactly **2**, as
§7 names it ("U5 cancel (reads 2)"). U1 and F1 (fixture) unaffected. Marker 1→0 confirmed.

**NC8 (×1):** `RUN=471 FAIL=1` — only `TestListenerFilterTimeoutPreCxTimeoutOnAbort` (U5f). U5, the
T-arms (fixture) and CompareBytes (fixture) are out of unit scope / stayed green. Matches §7 exactly.
Marker 1→0 confirmed.

Every one of the eight unit-level NC columns above reproduces §4.1's / §7's declared roster with no
observed departure at the unit level (NC1's exact fell-through COUNT is inherently probabilistic and was
never pinned to a single number by the plan beyond "5-8 of 20"; two of our three runs (5, 6) land inside
that band and one (9) lands just outside it — recorded as a measured variance, not re-run to force a
match, per Step 6's rule).

### Step 5 — fixture NCs (`test/differential`, fixture `0125-listener-filters-timeout`, run ALONE)

Ports re-censused at the IMPL tip against `test/fixtures/0125-listener-filters-timeout/driver/driver.go`
lines 118-122: `l_false=15125 l_true=15228 l_true_tls=15229 l_zero=15230 l_nofilt=15231` — **unchanged**
from the PLAN-stage census in §2.4. `docs/envoy-go/ENVOY_TARGET.md` lines 3-4 confirm the same digest
pin (`envoyproxy/envoy@sha256:7edd5b0f...453be8`, `contrib-v1.37.2`).

Each run: `go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0125-listener-filters-timeout$'
-timeout 30m`, foreground. Every run showed `=== RUN   TestDifferential/0125-listener-filters-timeout`
and 0 `SKIP` before any result was read. No container outside this fixture's own (`ryuk`, one envoy
reference container, torn down by the harness itself each time) was touched; the `cpj-p14-*` / `golink-ai`
/ `reaper_*` containers from other sessions were left alone throughout (confirmed present, unaffected, via
`docker ps -a` before starting).

**NC1 (×2):**

| arm | run 1 | run 2 | §4.3 declared |
|---|---|---|---|
| F1 `l_false` | g / **R** open@3000 | g / **R** open@3000 | g / R open at 3000 |
| F2 50 concurrent | g / **R** 36 open | g / **R** 37 open | g / R 33 open, g / R 26 open |
| F3,T1,T2,T3,N1 | g/g | g/g | g/g |
| Z1 | g/g | g/g | g/g |
| S `l_false`=51 | g / **R** 14 | g / **R** 13 | g/R 17, g/R 24 |
| S `l_true`,`l_true_tls`=1 | g / **R** 0, 0 | g / **R** 0, 0 | g/g, g/R 0 (declared 1-of-2 coin flip) |
| S `l_zero`=0 | g/g | g/g | g/g |
| S `l_nofilt`=0 | g/g | g/g | g/g |
| CompareBytes | **R** (first divergence offset 18, `l_false closed=true/false`) | **R** (offset 18, same field) | R |
| verdict | FAIL(1) | FAIL(1) | FAIL ×2 |

**Finding:** `S l_true`/`l_true_tls` reddened in BOTH of our two runs (0 instead of 1), not a 1-of-2 coin
flip as §4.3 declares. Not re-run to force a match (Step 6's rule) — recorded as a measured departure.

**NC2 (×1):** F1/F2 close correctly (g/g, `subj F1 closed=true ms=1001 kind=FIN`, 50/50 FIN), F3/T-arms/N1/Z1
g/g, CompareBytes GREEN (no "first divergence" in the log — confirms §4.3's "NC2 ... invisible to
CompareBytes"), `S l_false`=0 want 51 **R**, `S l_true`=0 want 1 **R**, `S l_true_tls`=0 want 1 **R**,
`S l_zero`/`l_nofilt` g/g. verdict FAIL(1). Matches §4.3's NC2 row exactly.

**NC3 (×1):** F1/F2/F3/T-arms/N1 g/g, **Z1 R** (`subj Z1 l_zero: silent client ended at 15000 ms
(kind=FIN), want still open at 16500 ms`), `S l_zero`=1 want 0 **R**, `S l_false/l_true/l_true_tls/l_nofilt`
g/g, CompareBytes **R** (first divergence offset 332). verdict FAIL(1). Matches §4.3's NC3 row exactly.

**NC8 (×1):** F1/F2 close correctly (g/g — NC8 only guards the `Inc()`, not the `Close()`), F3/T-arms/N1/Z1
g/g, CompareBytes GREEN (no "first divergence" — confirms §4.3's "NC8 ... invisible to CompareBytes"),
`S l_false`=0 want 51 **R**, `S l_true`/`l_true_tls` g/g (1/1, correctly booked under
`continue_on_listener_filters_timeout: true`), `S l_zero`/`l_nofilt` g/g. verdict FAIL(1). Matches §4.3's
NC8 row exactly.

Every marker asserted ≥1 immediately after `patch -p1` and 0 immediately after `patch -R -p1`, for every
fixture NC above; `git -C /home/esa/git/envoy-go-wt-p100nc status --short` read only
`?? internal/listener/zz_u2old_test.go` after each reversal.

### Step 6 — every measured cell vs §4.1 / §4.3: findings

All eight NC columns of §4.1 (unit) and all four exercised rows of §4.3 (fixture: NC1, NC2, NC3, NC8)
reproduce the plan's declared roster **arm-for-arm** (which test reddens, which stays green, and — for
NC2/NC7 — the exact knock-on/over-count value). The only departures found, recorded and NOT re-run to
force a match:

1. **NC1 unit, U1 fell-through count:** §4.1/§7 declare "5-8 of 20"; our 3 runs measured 9, 5, 6 fell
   through (11, 15, 14 closed of 20). Run 1's 9 is outside the declared 5-8 band; runs 2-3 are inside it.
   Inherent OS-timing probabilism (§7 labels this row itself "probabilistic"); not a mechanism defect.
2. **NC1 fixture, `S l_true`/`l_true_tls` coin flip:** §4.3 declares this pair a "1 of 2" coin flip across
   its own two runs. Our two independent runs both landed on the SAME side (both reddened, 0 instead of
   1, both runs) — 2-of-2, not 1-of-2. Recorded as a measured departure from the declared distribution,
   not a correctness defect (the mechanism note for NC1 — the raw-deadline second clock racing the
   TLS-inspector read — plausibly biases toward failure once TLS negotiation itself is in the silent
   window, but this task does not re-run to test that hypothesis).
3. **NC1 fixture, exact `S l_false` and F2-open counts:** §4.3 declares 17/24 (its own two runs) and 33/26
   open; ours measured 14/13 and 36/37. Same qualitative direction (**R**, well below/above the reference
   value) in every run; the exact counts are declared-probabilistic (concurrent 50-way race against a
   1s pipeline deadline) and were never pinned to a single number.

No other cell — including every NC2, NC3, NC4, NC5 (×5 + race), NC6 (OLD/NEW U2 side-by-side vacuity),
NC7, and NC8 result, at both unit and fixture level — differed from §4.1/§4.3's declared value.

### Step 7 — remove the throwaway worktree

`git -C /home/esa/git/envoy-go-wt-p100impl status --short` in the throwaway tree read only `??
internal/listener/zz_u2old_test.go` immediately before removal (no residual mutant, no `.orig` backup
file). `git -C /home/esa/git/envoy-go-wt-p100impl worktree remove --force /home/esa/git/envoy-go-wt-p100nc`
removed it; `git -C /home/esa/git/envoy-go-wt-p100impl worktree list` afterward shows only
`/home/esa/git/envoy-go` (master), `/home/esa/git/envoy-go-wt-p100docs` (a DIFFERENT session's worktree,
untouched throughout), and `/home/esa/git/envoy-go-wt-p100impl` itself — `envoy-go-wt-p100nc` is gone from
both the listing and the filesystem.

### Step 8 — this commit

Only `docs/envoy-go/phases/100-listener-filters-timeout-enforce/PROGRESS.md` is touched in this real
worktree for Task 12; no production or test file here was ever modified (every mutant lived and died in
the now-removed throwaway worktree).

## Task 13: ADR-0322 §Decision + §Consequences, ACCEPTED

Landed in a separate worktree, `/home/esa/git/envoy-go-wt-p100docs` (branch `wt-phase-100-impl-docs`,
created off `d163a009`), then cherry-picked onto this branch. **On this branch (`wt-phase-100-impl`) the
commit is `eb83980e`**; the docs worktree's original commit was `1014a200` — same tree, different parent
chain. `--numstat`: `122 1 docs/envoy-go/DECISIONS.md`.

The status line flips `PROPOSED` → `ACCEPTED` using ADR-0321's exact wording pattern ("are APPENDED" →
"APPENDED", "footer below" → "footer", the RE-ARMS sentence replaced by "WAS RE-ARMED … AND IS DISARMED BY
THIS FLIP"). §Decision covers PB1 as built (`pipeline.go:43` and `:33-37` unmoved, cross-checked against
the live file), the counter (registered unconditionally at `manager.go:420`, `Inc`'d at `:1358` before the
`continueOnLfTimeout` branch), the `0s` split (`parseListenerFiltersTimeout` at `:953`), the ADR-0082
supersessions, and ADR-0296/ADR-0320. §Consequences (a)-(g) list the SPEC §4.3 behaviour rows (including
the shutdown row per PLAN §0.8), the evidence (6 unit RED at the tip, `0125` F1/F2/S/CompareBytes RED, 470
`=== RUN` / 0 FAIL, `-race` clean, `0125` ×3 PASS, reference 1000-1004 ms, one IMPL run at 1030 ms), the
QUIC registration gap (unmeasured on the reference, banked not pinned), the NC roster (NC5 BLIND, NC6's
OLD-U2 vacuity at 3.00 s — **quoting no Task-12 per-cell result**, pointing to this file instead), the +1
stat name, what was not bought, and the envelope-lift next row.

Guard verified DISARMED by LINE and by ADR (not by count alone): pre-edit, `wc -l` read 19509 and
`/usr/bin/grep -n '^> \*\*STATUS: PROPOSED'` hit at line 19491; post-edit the same grep is silent (rc=1),
`sed -n 19491p` reads `> **STATUS: ACCEPTED …`, and the ADR-0231 decoy at `DECISIONS.md:14866` is
byte-untouched (md5 `929719b67c87aa16ac1e406fed7eba6b` before and after). Sub-gate counts: `^---$` 216,
`^## ADR-` 321, bare `^## ` 329, tail `## ADR-0322` (next-free `ADR-0323` reads 0), `wc -l` 19630
(19509 → 19630, +121 = 122 added − 1 deleted).

## Task 14: contract — timeout enforced, 0s disables; ledger +1 (delta only)

**On this branch the commit is `7773b4a3`** (docs worktree original: `f2ebf5ce`). `--numstat`:
`3 1 docs/envoy-go/BEHAVIOR_CONTRACT.md`.

The `:4359` bullet is rewritten in place as one line: ENFORCED by one clock inside `Pipeline.Run`
(ADR-0322); absent → 15 s; explicit `0s` DISABLES; the `[1s, 60s]` envelope stays envoy-go's OWN
(ADR-0082, the reference accepts `0.5s`/`61s`, lift is the next row); `false` closes and `true` falls
through (stamped `raw_buffer`), both AT the deadline; each timeout books
`listener.<addr>.downstream_pre_cx_timeout` under both values; a shutdown cancel books nothing. The ledger
entry `**Phase 100 — +1 (`listener.<addr>.downstream_pre_cx_timeout`) — delta only:**` sits after the
`**Phase 99 — +0, UNCHANGED` paragraph, blank-line separated as every sibling entry is, names the one
`NewCounter` site, the unconditional registration, and QUIC carrying it (reference unmeasured), and spells
its own departure from the phase-94 absolute/`(+N)` form out in words (three mutually inconsistent
absolutes are live at one tip) so the entry carries no `→` at all. The "exactly one new `NewCounter` call
site" claim and the QUIC registration line (`internal/listener/quic.go:66`) were both verified directly
against the diff and the source.

⚠️ **Refutation carried forward:** `wc -l docs/envoy-go/BEHAVIOR_CONTRACT.md` reads **5998 → 6000**, not the
step's predicted 5999. The extra line is the blank paragraph separator every ledger entry carries (the
existing 5138/5140/…/5148 entries are blank-separated paragraphs; without the blank, the new entry would
merge into the phase-99 paragraph). PLAN §1.2's own file-cost row (`~+3 / −1`) already predicts net +2 =
6000, so the PLAN was internally inconsistent between its own §1.2 and the Task 14 step text, and the
measured `3 1` / `5998 → 6000` is the one that stands.

## Task 15: REVIEW_FINDINGS timeout clause annotated fixed

**On this branch the commit is `6f5c08ae`** (docs worktree original: `97b673c3`). `--numstat`:
`2 1 REVIEW_FINDINGS.md`.

A11 (`REVIEW_FINDINGS.md:185`) had no prior "fixed" annotation convention, so a bracketed, clause-scoped
tag was minted. Lines 185-189 now read the `listener_filters_timeout` clause with
`[FIXED at phase 100, ADR-0322 — this timeout clause only]` inserted inside that clause alone; the
`continue_on_listener_filters_timeout` non-timeout-gating clause (still true of the code, SPEC §10) and the
SNI case-sensitivity clause (still true) carry no annotation.

## Task 16: row 100 done

**On this branch the commit is `56137eac`** (docs worktree original: `beeabf47`). `--numstat`:
`1 1 docs/envoy-go/ROADMAP.md`.

The row-100 cell flips `in-progress` → `done` in the form of rows 98 and 99 (dates, what landed,
`--numstat`, tip RED → GREEN, the ADR, the contract and ledger, what was not bought). The cell cites no
Task-12 (NC) result of its own — same posture as ADR-0322 §Consequences (d): it states only the PLAN's §7
roster (NC5 BLIND, NC6's OLD-U2 vacuity) and points to this PROGRESS.md file. The sentinel from
`next-prompt.txt` (checks 1-3, NC-A/B/C/D, the check-(2) positive control, six md5s, and the escape-aware
malformed set) does NOT fire on the installed row — check (2) still prints six windows unchanged, all six
md5s unchanged, `want` stays 132, `ROADMAP.md` stays 250 lines, and no `stop` file is created. `git diff
--numstat` for the install: `1 1 docs/envoy-go/ROADMAP.md`, hunk `@@ -162 +162 @@` only — a flip, not an
add.

## Review-fix wave

A code review (opus) of Tasks 8-11 returned: **Spec ✅, Approved, 0 Critical, 0 Important.** Two minors were
named:
1. The `listenerRuntime` metric-block comment (`internal/listener/manager.go`, the block above
   `downstreamCxTotal`) claimed both cx metrics are "Inc/Dec'd from the per-connection path", which reads
   as one path for both directions. **Fixed here** (below).
2. A ~110-char comment line at the listener-filter registration site. **Left as-is** — cosmetic only, no
   correctness or accuracy defect.

**Fix 1 — the metric-block comment (`internal/listener/manager.go`).** Verified by `/usr/bin/grep -n`
before editing: `downstreamCxTotal.Inc()` and `downstreamCxActive.Inc()` are both in `acceptLoop`
(`manager.go:1285-1286`); `downstreamCxActive.Dec()` is deferred at the top of `serveConnection`
(`:1321`) and `downstreamPreCxTimeout.Inc()` fires later in the same function (`:1358`). The six-line
comment was rewritten, comment-only, to name `acceptLoop` for the two `Inc()`s and `serveConnection` for
the `Dec()` plus the new `downstreamPreCxTimeout.Inc()`, and to disambiguate the trailing "all five
pointers stay NIL" sentence to "all five ssl.* pointers stay NIL" (the five `ssl*` fields, not all eight
metric fields). `git -C W diff --numstat -- internal/listener/manager.go` read `5 5` (equal), and every
changed line starts with `//` after leading whitespace:
```
5	5	internal/listener/manager.go
```
`gofmt -l internal/listener/` printed nothing; `go vet ./internal/listener/` rc 0; `go test -count=1
./internal/listener/` → `ok github.com/pgdad/envoy-go/internal/listener 7.160s`; the layout gate
(`.superpowers/sdd/PLAN/scratch/layout-gate.sh $W master`) read:
```
PASS (a) pipeline.go:33-37,43 byte-identical to master
PASS (b) pipeline.go import block unchanged
PASS (c) one code WithTimeout, at internal/listener/listenerfilter/pipeline.go:43
PASS (d) tls_inspector.go numstat 3 3, comment-only
PASS (e) no line-count-changing pipeline.go hunk at/above line 44
layout-gate: 0 failed sub-gate(s) (base master)
```
`GOTOOLCHAIN=go1.26.2 golangci-lint run --disable-all --enable misspell ./internal/listener/...` produced
no findings (US spelling clean). Commit `a1de8a67`.

**Fix 2 — ADR-0322 §Consequences (`docs/envoy-go/DECISIONS.md`).** Two changes:
(a) A new §Consequences (a) bullet states the shutdown-hold consequence the review identified: PB1's
`context.AfterFunc` is armed only inside `if timeoutMs > 0`, so an explicit `0s` listener never arms it; a
manager-context shutdown cancel therefore does not interrupt a silent client's peek, and the
`serveConnection` goroutine is held until the client acts on its own or the process exits (`acceptLoop`
does not track it). The bullet is explicit that this is **derived from the mechanism** (§Context ¶3-4,
Decision 1) and **not a reference measurement** of shutdown behaviour.
(b) The NC roster was reconciled against Task 12's measured results (this file's Task 12 section, and
`.superpowers/sdd/PLAN/task-12-report.md`): measured departures from the plan were NC1 unit U1 fall-through
9/5/6 of 20 (plan declared 5-8), NC1 fixture `S l_true`/`l_true_tls` red in 2 of 2 runs (plan declared 1 of
2), and NC1 fixture `S l_false` 14/13 with F2-open 36/37 (plan declared 17/24 and 33/26); NC5 was blind
confirmed (×5 + `-race`) and NC6 confirmed OLD U2 PASS at 3.00 s vs NEW U2 FAIL. **ADR-0322 §Consequences
(d) quotes no NC1 per-cell figure and no plan figure that Task 12 contradicts** — it names only NC5 (BLIND)
and NC6 (OLD-U2 vacuity at 3.00 s, matching Task 12 exactly) and otherwise points to this PROGRESS.md file,
so no correction to quoted numbers was needed there; only the new shutdown-hold bullet was added.

Verification re-run after editing:
```
$ /usr/bin/grep -c '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md
0
$ /usr/bin/grep -c '^---$' docs/envoy-go/DECISIONS.md
216
$ /usr/bin/grep -c '^## ADR-' docs/envoy-go/DECISIONS.md
321
$ /usr/bin/grep -c '^## ' docs/envoy-go/DECISIONS.md
329
$ /usr/bin/grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0322
$ wc -l docs/envoy-go/DECISIONS.md
19637 docs/envoy-go/DECISIONS.md
```
`git diff --numstat -- docs/envoy-go/DECISIONS.md` read `7 0` (a pure insertion, no line touched or
removed; the status blockquote, `---` count, and `## ` headings are all unchanged). Commit `5d8f7476`.

## Task 17: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

Scratch: `$S = .superpowers/sdd/PLAN/scratch/gates` (created this task). Preconditions confirmed:
`git -C $W rev-parse --abbrev-ref HEAD` = `wt-phase-100-impl`; HEAD = `e5d20b7c` at task start;
`git -C $W status --short` clean at start and after the lint-control probe (below).

### Step 1 — byte-untouched roster and the edit roster's path set

`git -C $W diff master --numstat -- <path>` read EMPTY for every byte-untouched-roster path:
`internal/listener/quic.go`; `internal/listener/listenerfilter/{callbacks,types,chainmatch,doc}.go`;
`internal/listener/listenerfilter/tls_inspector/` except `tls_inspector.go`; `internal/stats/` except
`name.go` and `helptext_test.go`; every `test/fixtures/*` directory except `0125-listener-filters-timeout`;
`go.mod`, `go.sum`, `.github/**`. All empty.

`git -C $W diff master --numstat` (full branch diff, 17 files):
```
2	1	REVIEW_FINDINGS.md
3	1	docs/envoy-go/BEHAVIOR_CONTRACT.md
129	1	docs/envoy-go/DECISIONS.md
1	1	docs/envoy-go/ROADMAP.md
1112	0	docs/envoy-go/phases/100-listener-filters-timeout-enforce/PROGRESS.md
338	0	internal/listener/listener_filters_timeout_test.go
17	2	internal/listener/listenerfilter/pipeline.go
209	0	internal/listener/listenerfilter/pipeline_deadline_test.go
3	3	internal/listener/listenerfilter/tls_inspector/tls_inspector.go
28	19	internal/listener/manager.go
9	3	internal/listener/manager_test.go
1	0	internal/stats/helptext_test.go
2	0	internal/stats/name.go
1	0	test/differential/runner_test.go
79	0	test/fixtures/0125-listener-filters-timeout/README.md
727	0	test/fixtures/0125-listener-filters-timeout/driver/driver.go
30	0	test/fixtures/0125-listener-filters-timeout/expectations.yaml
```
Every path except one is named in PLAN §1.2's edit-roster table (the three `0125/**` files sum to
`836 0`, matching §1.2's combined figure exactly). **The one path on neither roster is
`docs/envoy-go/phases/100-listener-filters-timeout-enforce/PROGRESS.md`** — expected, per §1.2's own
note that `PROGRESS.md`, `STATE*.md` and `next-prompt.txt` are on neither list "by design." No other
extra path exists; `next-prompt.txt` and no `STATE*.md` file exist in this worktree, and
`git -C $W diff master --numstat -- next-prompt.txt` is empty (byte-untouched though not required to be).

Two rows measure differently from §1.2's predicted per-file split (recorded as a departure, not a defect —
§1.2's own figures are pre-execution estimates for some rows, `~` on two of them):
- `internal/listener/manager.go`: predicted `25 16` (Appendix A `14 6` + Appendix B `11 10`); measured
  **`28 19`**.
- `docs/envoy-go/DECISIONS.md`: predicted `~+100 / 0`; measured **`129 1`** (one line touched, not zero).
The other rows (`pipeline.go` `17 2`, `tls_inspector.go` `3 3`, `name.go` `2 0`, `helptext_test.go` `1 0`,
`listener_filters_timeout_test.go` `338 0`, `pipeline_deadline_test.go` `209 0`, `manager_test.go` `9 3`,
`runner_test.go` `1 0`, `0125/**` combined `836 0`, `BEHAVIOR_CONTRACT.md` `3 1`, `REVIEW_FINDINGS.md` `2 1`,
`ROADMAP.md` `1 1`) match §1.2 exactly.

**File-scope measurement against precedents** (`git -C $W show --numstat --format= <sha> | wc -l` for the
prior IMPL commits; `git -C $W diff --numstat master | wc -l` for this branch):
```
d5bc9153 (phase-99 IMPL)  : 20 files
c7bd2880 (phase 96 IMPL)  : 20 files
0a985a35 (phase 94 IMPL)  : 28 files
this branch vs master    : 17 files
```
This row's file scope (17) sits below all three measured precedents (20, 20, 28) — narrower, not wider.

### Step 2 — ARM roster (method note 28)

Base rosters (Task 1, `$W/.superpowers/sdd/PLAN/scratch/{base-listener,base-stats}.txt`): 266 + 196 = 462
sorted `=== RUN` lines, confirmed byte-identical (after stripping the `=== RUN   ` prefix) to
`.superpowers/sdd/PLAN/scratch/t7-base-combined.txt` (the Task 7 base roster already on disk).

New roster: `go test -count=1 -v ./internal/listener/...` and `./internal/stats/...`, `=== RUN` lines
combined and sorted: **470** lines (rc=0 both packages).

`comm -13 base new` (names only in NEW): exactly the eight —
`TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients`, `TestParseListenerFiltersTimeoutZeroDisables`,
`TestListenerFilterTimeoutPreCxTimeoutByValue`, `TestListenerFilterTimeoutPreCxTimeoutOnAbort`,
`TestPipelineRunDeadlineInterruptsSilentPeek`, `TestPipelineRunDeadlineClearedOnSuccess`,
`TestPipelineRunDeadlineClearedAfterTimeout`, `TestPipelineRunZeroTimeoutHoldsSilentPeek`.
`comm -23 base new` (names only in BASE, i.e. lost): **empty**. 462 + 8 = 470, exactly.

Each of the eight verified by `git -C $W grep -n '^func <name>('` against source, all found:
`internal/listener/listener_filters_timeout_test.go:36,123,149,274` (the first four) and
`internal/listener/listenerfilter/pipeline_deadline_test.go:100,124,160,189` (the last four).

### Step 3 — Gate (a): the full differential, `-count=1`

`go test -count=1 -v ./test/differential/ -timeout 60m` (foreground, `$S/gate-a.log`): **`rc=0`**,
`--- PASS: TestDifferential (472.66s)`, package line `ok  	.../test/differential	477.179s`. No rerun was
needed — no port-race abort text appeared and the panic gate (below) is 0.

Fixture-set reconciliation BY NAME, both `comm` directions, against `ls -d test/fixtures/*/` (127 dirs):
extracted 127 `--- PASS: TestDifferential/<name>` lines (0 `FAIL`, 0 `SKIP` lines of any form); sorted
fixture names vs. the 127-dir list: `comm -23` (dirs missing from the run) empty, `comm -13` (run names not
a real dir) empty. **127 = 127, PASS 127 / FAIL 0 / SKIP 0**, exactly the expected count.

### Step 4 — Gate (b): the non-Docker sweep

`go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'` → **242** packages
(`$S/gate-b-pkgs.txt`). The new package is confirmed present: `test/fixtures/0125-listener-filters-timeout/
driver`. Phase-99's prior figure was 241; this row's own new fixture package makes 242 — measured, not
predicted.

`go test -count=1 $(pkgs) 2>&1 | tee $S/gate-b.log; echo rc=${PIPESTATUS[0]}`: **`rc=0`**. Set
reconciliation: `ok` lines = 125, `FAIL`/`--- FAIL` lines = 0, `[no test files]` lines = 117.
**125 + 0 + 117 = 242 = package count**, exact. No FAIL of any kind, so no known flake fired this run
(`TestSDSEndToEnd_FetchFailure_BootFailsClosed`, the SDS dial-budget flake, `internal/httpclient` zero-value,
`TestOutlierDetector_ConcurrentEjectExactlyOnce`, `TestFramer_ReaderGoroutineDoesNotLeak`,
`TestEnvoyGoBinary_TwoListenerCutover` port flake — none recurred).

### Step 5 — Gate (c): h2spec (Docker)

`go test -count=1 -v ./test/conformance/h2spec/ -timeout 30m` (`$S/gate-c.log`): summary line at
`h2spec_test.go:288`: **`95 tests, 94 passed, 1 skipped, 0 failed`**. `--- PASS: TestH2Spec (2.70s)`,
package `ok ... 2.772s`, **`rc=0`**. 0 `[SKIP]`-tagged sub-lines beyond the one counted in the summary; 0
`[FAIL]` lines. The container it started (`5da62b4497fc`) was terminated by the harness itself
(`🚫 Container terminated`), not by this session directly.

### Step 6 — Gate (d): fuzzers

`git -C $W grep -c '^func Fuzz' -- '*.go'` → **48 files**; `awk -F: '{sum+=$2}'` over the same → **56
targets**. Matches the expected 48/56 exactly; this row adds no fuzzer.

### Step 7 — Gate (e): the anchored panic gate, lint, and the planted control

Anchored panic gate `^panic:|DATA RACE|SIGSEGV` over `$S/gate-a.log`, `$S/gate-b.log`, `$S/gate-c.log`:
**0** hits in all three (proven live at Task 1).

`GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`: **`rc=0`**, 0 lines of output (clean).

Planted control, `internal/listener/zz_lintplant.go` (new, non-test file, deleted after the probe):
```go
package listener

import "os"

func LintPlantExported() {
	os.Open("/tmp/zz-lintplant-does-not-exist")
	x := 1
	x = 2
	_ = x
}
```
`go build -o /tmp/zz-lintplant-build ./internal/listener/` → **`rc=0`** (compiles cleanly; binary
discarded). Re-running `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` with the file present: **`rc=1`**,
exactly three findings, each linter firing BY NAME:
```
internal/listener/zz_lintplant.go:6:9: Error return value of `os.Open` is not checked (errcheck)
internal/listener/zz_lintplant.go:5:1: exported: exported function LintPlantExported should have comment or be unexported (revive)
internal/listener/zz_lintplant.go:7:2: ineffectual assignment to x (ineffassign)
```
`errcheck`, `revive`, and `ineffassign` each fired exactly as designed. The control file was then deleted
(`rm internal/listener/zz_lintplant.go`); `git -C $W status --short` printed nothing — clean.

### Step 8 — Gate (f): no REVIEW.md

`find docs/envoy-go/phases -maxdepth 2 -iname REVIEW.md` lists `REVIEW.md` under phases 00 through 25.x but
under none of 93, 94, 95, 96, 97, 98, 99, or 100. **This phase carries no `REVIEW.md`** — named here as the
one standing departure, matching the precedent set by phases 93-99 (none of which has one either). No
compliance is claimed for a `REVIEW.md` gate; there is none to run.

### Summary — all six gates

| gate | result |
|---|---|
| (a) differential | rc=0, 127 PASS / 0 FAIL / 0 SKIP, fixture set 127=127 both `comm` directions, no port-race abort, single run |
| (b) non-Docker sweep | rc=0, 242 packages, 125 ok + 0 FAIL + 117 `[no test files]` = 242, no flake fired |
| (c) h2spec | rc=0, `95 tests, 94 passed, 1 skipped, 0 failed` |
| (d) fuzzers | 48 files / 56 targets |
| (e) panic gate + lint | 0 panic/race/segv hits over a/b/c; lint rc=0 clean; planted control fired errcheck+revive+ineffassign by name, then removed (`git status` clean) |
| (f) REVIEW.md | absent, as it is for phases 93-99; named as the standing departure |

No subagents were used for this task. Commit: `PROGRESS.md` only.

## Final-review fix (after Task 17)

A whole-branch review (opus) of `9ba23146..e5d20b7c` returned **Ready with fixes**: one Important and four
Minors. The controller ruled Important 1 plus Minors 1 and 4 into one commit, and parked Minor 2 (the
`listenerRuntime` metric-block comment describes the TCP path only; it is accurate for the struct it
documents, though QUIC listeners register the same name) and Minor 3 (the U5/U5f helper duplication, a
PLAN-built, byte-identical appendix). Commit `67dcb18f`:
- **Important 1 — the row-100 cell's `manager.go` figure was false at the branch tip.** It quoted `25 / 16`;
  `git diff --numstat master -- internal/listener/manager.go` read `28 19`, because the review-wave comment
  fix `a1de8a67` landed AFTER Task 16 wrote the cell. Corrected to `28 / 19`. The cell's other numstat
  figures (`pipeline.go` `17 / 2`, `tls_inspector.go` `3 / 3`, `name.go` `2 / 0`) were re-measured and
  are unchanged; the cell quotes no `DECISIONS.md` figure. Gated in a scratch copy first: NF **8** under the
  naive and the escape-aware forms, neither sentinel phrase and no bare `deferred` in the line, and the other
  249 lines byte-identical. The full sentinel re-ran afterwards and read exactly the Task 16 post-flip
  shapes (recorded under Close-out, below).
- **Minor 1 — `0125/README.md` carried PLAN-time figures only.** IMPL observations were ADDED beside them,
  not in place of them: Task 10's one reference run with max **1030 ms** (σ **13.72**, still inside
  `[700, 1800]`), and Task 12's NC1 fixture cells (F2 open **36 / 37**, S `l_false` **14 / 13**, S
  `l_true` / `l_true_tls` red in **2 of 2**). `go vet ./test/fixtures/0125-listener-filters-timeout/...`
  rc 0 (the README is not compiled, so the fixture was not re-run).
- **Minor 4 — ADR-0322 §Decision named two production files and the help-text pair, and omitted the
  comment-only `tls_inspector.go` edit.** The paragraph now names it (`3 / 3`, comment-only) and the
  comment-only reconciliation in `pipeline.go` and `manager.go`, and says the layout gate covers
  `tls_inspector.go` (kind and shape) and `pipeline.go` (cited lines) but not `manager.go`. House guards
  after the edit: `^> \*\*STATUS: PROPOSED` **0** (captured; the tail status line `:19491` reads
  `ACCEPTED`), `^---$` **216**, `^## ADR-` **321**, bare `^## ` **329**, tail **ADR-0322**, `^## ADR-0323`
  **0**; the ADR-0231 decoy `:14866` still resolves to `## ADR-0231` (`:14864`), md5 `929719b67c87`.
  `DECISIONS.md` is now **19644** lines, `136 1` against master (Task 17 measured `129 1` before this fix).

## Close-out (Task 18)

### Per-task summary

| task | commit | result |
|---|---|---|
| 1 | `b866e904` | baseline rosters: listener 266 + stats 196 `=== RUN`, five-selector roster 421; the anchored panic gate proven live |
| 2 | `2143a3fa` | U1, U3, U5, U5f RED at the un-fixed tip, each for its named reason |
| 3 | `ce846450` | U4 (i)-(iv); (i) and (iv) RED at the tip, (ii) and (iii) GREEN |
| 4 | `b8d3a4d0` | U2 strengthened in place (`9 3`); PASS at 1.00 s; the OLD U2 saved to scratch |
| 5 | `d857bb52` | fixture `0125` driver, byte-identical to Appendix F.1 (727 lines); ports censused |
| 6 | `3c4527f3` | README (79), expectations (30), the registration import; fixture set 127 = 127; extractor NCs fired |
| 7 | `8bb5cb56` | the un-fixed tip recorded: unit 470 RUN, rc 1, six named FAILs; fixture subject RED on F1, F2, S x5 and CompareBytes, reference green |
| 8 | `17a1dda8` | PB1 (Appendix A); layout gate a, b, c, e PASS and d FAIL, by design |
| 9 | `a3d6524a` | the help-text pair in one commit, each half RED alone first |
| 10 | `5c6d481c` | unit 470 / 0 FAIL, `-race` clean, fixture GREEN 3/3 (one reference run reached 1030 ms) |
| 11 | `d163a009` | comment reconciliation (Appendix B); layout gate 0 failed, both plants fired |
| 12 | `3de5ab23` | NC1-NC8 scored per arm with markers asserted; NC5 BLIND, NC6 OLD-vs-NEW reproduced; NC1 probabilistic cells differ (below) |
| 13 | `eb83980e` | ADR-0322 §Decision + §Consequences, `ACCEPTED`; the house guard disarmed |
| 14 | `7773b4a3` | `BEHAVIOR_CONTRACT.md` bullet rewritten and the delta-only `+1` ledger entry (5998 -> 6000) |
| 15 | `6f5c08ae` | `REVIEW_FINDINGS.md` timeout clause annotated fixed (`2 1`) |
| 16 | `56137eac` | row 100 `done`; sentinel measured on both sides of the flip |
| review wave | `a1de8a67`, `5d8f7476`, `e5d20b7c` | the metric-block comment fixed (comment-only, `5 5`); ADR-0322 §Consequences gained the `0s` shutdown-hold line; PROGRESS for Tasks 13-16 |
| 17 | `f80e8fbc` | byte-untouched roster empty; arm roster 462 + 8 = 470; all six gates (a 127/0/0, b 242 rc 0, c 95/94/1/0, d 56/48, e 0 + lint rc 0 with a live plant, f no `REVIEW.md`) |
| final-review fix | `67dcb18f` | row-100 numstat, 0125 README, ADR-0322 §Decision (above) |
| 18 | this commit | close-out: `STATE.md` rolled, the archive +1, the router rolled to a phase-101 self-pick BRAINSTORM |
| 19 | pending | squash, merge, push and worktree removal — the controller's, not recorded here |

### Every refutation this IMPL made (method note 2) — FOURTEEN

Against the PLAN:
1. **`BEHAVIOR_CONTRACT.md` went 5998 -> 6000, not the 5999 Task 14 Step 3 predicted.** The ledger's
   entries are blank-separated paragraphs, so the entry costs a separator line too; PLAN §1.2's own
   `~+3 / -1` agrees with the measured `3 1`, and only the Task 14 figure was wrong.
2. **`manager.go` measures `28 19`, not PLAN §1.2's `25 16`.** Appendices A + B are `14 6` + `11 10` = `25 16`
   exactly as predicted; the review-wave comment fix (`a1de8a67`) moved it.
3. **`DECISIONS.md` measures `129 1` at Task 17 (`136 1` after the final-review fix), not `~+100 / 0`.**
   One line (ADR-0322's status line) is rewritten, not only appended.
4. **One reference close spread reached 1030 ms (σ 13.72, n=51, Task 10 run 2)** against the PLAN's
   1000-1004 ms (n=459). The other two runs read max 1002 and 1003 ms; the window `[700, 1800]` held.
5. **NC1 unit, U1 fell through 9, 5 and 6 times of 20** against the declared 5-8; run 1 sits outside the band.
6. **NC1 fixture, S `l_true` / `l_true_tls` red in 2 of 2 runs**, against the declared 1 of 2.
7. **NC1 fixture counts: S `l_false` 14 / 13 and F2 open 36 / 37**, against 17 / 24 and 33 / 26. Same
   direction, declared probabilistic, never pinned.
8. **Appendix B's metric-block comment claimed both cx metrics are Inc/Dec'd "from the per-connection
   path"** — false (both `Inc()`s are in `acceptLoop`). Found by the Task 8-11 review and fixed comment-only;
   accurate prose was ruled over byte-fidelity to the appendix.
9. **The Task 19 brief's extraction swallowed every appendix (2017 lines)** — found at preflight; benign,
   because implementers extracted each appendix from `PLAN.md` itself.

Against the IMPL's own work:
10. **The Task 4 scratch copy of the OLD U2 (`zz_u2old_test.go`) had no `import` block** and broke the
    package build when Task 12 used it; fixed in the throwaway worktree only.
11. **The row-100 cell's `manager.go 25 / 16` became false inside the stage** (final review, Important 1):
    Task 16 wrote a true figure, and a later commit of the same stage falsified it.
12. **ADR-0322 §Decision omitted the comment-only `tls_inspector.go` edit** (final review, Minor 4).
13. **`0125/README.md` carried only PLAN-time window and NC figures** (final review, Minor 1).
14. **Task 17's "17 files, narrower than all three precedents" compared unlike things.** `d5bc9153`,
    `c7bd2880` and `0a985a35` are squashed IMPL commits that already carry the close-out files
    (`STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`). At this close the branch touches **20** paths
    (`git diff --numstat master | wc -l`), the same as phases 99 and 96 (20) and below phase 94 (28).

Confirmed, not refuted: gate (b) **242** packages (the router carried 241, labelled unmeasured, and predicted
`+1` for `0125/driver`); the fixture set **127 = 127**, now **103 `driver/` + 24 `inputs/`**; every PB1,
test and fixture figure the PLAN built (byte-identical appendices, 470 RUN, six named tip REDs, the
`836 0` fixture); the sentinel shapes the router predicted for the flip; the §Recent tie the router
projected (below). Items 11-13 came from the stage's own final review — method note 2's "expect to be
refuted by your own review seam" held for a fourth row.

### Sentinel at this close

Commands verbatim from `next-prompt.txt` lines 12-72, `/usr/bin/grep`, in the publishing tree, after the
final-review row edit (ACTUAL output):
```
== (1)
== (2)
210:remaining deferred (not-yet-chartered) candidates:
216:remaining deferred (not-yet-chartered) candidates:
222:remaining deferred (not-yet-chartered) candidates:
232:remaining deferred (not-yet-chartered) candidates:
238:remaining deferred (not-yet-chartered) candidates:
246:deferred candidates:
== (3)
== NC-A
NC LANDED? [ in-progress ]
NOT DONE: row 62
== NC-B want=131
GATE FAIL: examined 132 data rows, expected 131
== NC-C
0
NEVER OPENED: gRPC   <- NC FIRED
== NC-D
96
68
== PC
0
6
== malformed (escape-aware)
119: row 57  NF=9
131: row 69  NF=10
== row100 NF naive/esc
162 8
162 8
== md5 (trailing newline included)
210 10d7807bf02d
216 4a92f7e62fc6
222 2a7eb298b9fd
232 242e53c6f7a3
238 b2680e6f4fbf
246 6caa1c3ce0e7
== wc
250
== stop
ls: cannot access '/home/esa/git/envoy-go/stop': No such file or directory
ls: cannot access '/home/esa/git/envoy-go-wt-p100impl/stop': No such file or directory
```
⇒ the sentinel does NOT fire; `stop` NOT created.

### Eviction and archive

Pre-roll histogram of §Current + §Recent (`grep -oE` over the `active-phase` / `prior active-phase`
labels, `uniq -c` on the dates), INCLUDING the entry this close promoted (the phase-100 PLAN):
**2 x `2026-09-22`, 4 x `2026-09-21`** — a FOUR-WIDE `09-21` tie at the tail, exactly as the router
projected, broken by LIST POSITION: the evictee is the phase-99 SPEC entry. LABEL-BOUND PAIR
(`/usr/bin/grep -cF -- '<backticked label>'`):

| label | `STATE.md` before -> after | `STATE_HISTORY.md` before -> after |
|---|---|---|
| evictee (phase-99 SPEC) | 1 -> 0 | 0 -> 1 |
| fabricated (`… SPECTRE done`) NC | 0 -> 0 | 0 -> 0 |
| positive control (phase-99 BRAINSTORM, archived) | 0 -> 0 | present -> present |

Archive guard (house anchored forms): strict `163 -> 163` (DELTA 0), parenthetical `83 -> 84`, loose
`246 -> 247`; `STATE_HISTORY.md` `592 -> 594` (`wc -l`), `2 0`. `STATE.md` `66 -> 66`, `10 10`.

### Router roll

`next-prompt.txt` now points at a phase-101 self-pick BRAINSTORM (state DONE -> 1), with the `[1s, 60s]`
envelope lift unblocked and adjudicated next in line. Figures moved: check (1) SILENT, NC-A / NC-B ONE,
fixtures 127 (103 + 24), gate (b) 242, the house guard DISARMED, the ledger chain ending in the phase-100
`+1` entry, the archive triple, the §Recent projection, the port census (`0125` now holds `15125` and
`15228`-`15231`). Method note 2 reads `100 IMPL fourteen`; two notes were added (97, 98) for items 11 and
14. The spent-IMPL imperatives found by grep, and the memory-slug audit, are listed in the task report.

**Line counts at this commit** (`wc -l`): `ROADMAP.md` 250, `DECISIONS.md` 19644, `BEHAVIOR_CONTRACT.md`
6000, `STATE.md` 66, `STATE_HISTORY.md` 594, `100/BRAINSTORM.md` 514, `SPEC.md` 853, `PLAN.md` 2938, this
file 1461. Phase dirs 141; fixtures 127 = 127.

## Task 19: squash, merge, push, worktree removal — done by the controller, not recorded here.
