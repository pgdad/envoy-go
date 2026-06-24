# Phase 43.2b Progress — graceful HTTP/2 GOAWAY-driven upstream connection rotation

**Branch:** `phase-43.2b-h2-goaway-rotation`
**Plan:** `docs/envoy-go/phases/43.2-h2-connection-pool/PLAN-43.2b.md`
**Spec:** `docs/envoy-go/phases/43.2-h2-connection-pool/SPEC-43.2b.md`
**Goal:** Add codec-visible GOAWAY signalling (`GoneAway`/`GoneAwayCh`/`Done` accessors) + an admission-skip + generalized eviction (`Closed() || GoneAway()`) + a per-conn drain-watcher (graceful idle prompt-close) + lazy replacement via the MISS path + the FIRST codec→cluster stat wiring (`http2.rx_reset` + `http2.tx_reset` via `h2.WithResetHooks`) + cross-side differential `0080-h2-goaway-rotation`. Records ADR-0254; the SECOND-and-FINAL sub-leg of row 43 — at THIS leg's IMPL row 43 flips `done` and the Upstream-robustness family (39–43) CLOSES.

---

## Baselines (captured at Task 1, commit `<this commit>`)

Run from the worktree root `/home/esa/git/envoy-go/.worktrees/phase-43.2b-h2-goaway-rotation` on branch `phase-43.2b-h2-goaway-rotation`.

### Build / vet / fmt / lint

```
$ go build ./...
(no output)
EXIT: 0  — CLEAN

$ go vet ./...
(no output)
EXIT: 0  — CLEAN

$ gofmt -l internal/ test/
(no output)
EXIT: 0  — CLEAN

$ golangci-lint run ./internal/cluster/... ./internal/filter/... 2>&1 | tail -5
(no output)
EXIT: 0  — CLEAN     (golangci-lint at /home/esa/go/bin/golangci-lint — present)
```

### Unit tests

```
$ go test ./internal/cluster/... ./internal/filter/hcm/h2/... -count=1 2>&1 | tail -20
ok  	github.com/esalaine/envoy-go/internal/cluster	3.499s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.496s
EXIT: 0  — PASS
```

### Differential suite

Differential: green at the 43.2a IMPL tip (81/81 GREEN at ADR-0253 completion). NOT re-run at Task 1 (heavy — 81-dir suite). The new `0080-h2-goaway-rotation` fixture lands at Task 8.

---

## Count baselines

All figures verified by the commands below; the ACTUAL observed numbers are recorded. **No discrepancy** between observed and the Task-1 stated baseline.

| Counter | Stated baseline | Observed | Verification command |
|---------|-----------------|----------|----------------------|
| Stat surface (H2 cluster) | **1185** | 1185 (per ADR-0253 / BEHAVIOR_CONTRACT) | recorded in `docs/envoy-go/BEHAVIOR_CONTRACT.md` stat-surface block (43.2a advanced 1183 → 1185) |
| Stat surface (non-H2 cluster) | **1183** | 1183 | byte-stable for non-H2 (useH2-gated stats) |
| Fixtures | **81** (tail `0079`) | **81** (tail `0079-h2-multiplex-pool`) | `ls -d test/fixtures/00* \| wc -l` → 81; `ls -d test/fixtures/00* \| tail -1` → `test/fixtures/0079-h2-multiplex-pool` |
| Fuzzers (documented) | **42** | 42 (carried unchanged per `reference_fuzzer_count_docs_drift`) | documented running total |
| Fuzzers (actual `^func Fuzz`) | **43** (known doc drift) | **43** | `grep -c '^func Fuzz' $(git grep -l '^func Fuzz' -- '*_test.go') \| awk -F: '{s+=$2} END{print "fuzzers:", s}'` → `fuzzers: 43` |
| BackendKind tail | **37** (`H2HoldResponder`) | **37** (`H2HoldResponder`) | `grep -n 'BackendKind = 3[0-9]' test/differential/fixture/fixture.go \| tail -1` → `597:	H2HoldResponder BackendKind = 37` |
| DECISIONS tail | **ADR-0253** (next-free **ADR-0254**) | **ADR-0253** (next-free **ADR-0254**) | `grep -n '^## ADR-' docs/envoy-go/DECISIONS.md \| tail -1` → `15861:## ADR-0253 — the HTTP/2 upstream multiplex connection pool ...` |

**Anticipated at 43.2b IMPL completion:** stat **1187** (+2: `http2.rx_reset` + `http2.tx_reset`, useH2-gated; non-H2 stays **1183**) / fixtures **82** (`0080`) / fuzzers **42** documented → reconciled to **43** at the completion task (D-H2B-FUZZER-RECONCILE) / BackendKind **38** (`H2GoawayResponder`) / DECISIONS **ADR-0254** (next-free **ADR-0255**).

---

## D-H2B-SPLIT-FINAL — the final ADR-0045 split re-check

**Decision:** 43.2b ships **flat** (ONE leg, ~10 tasks, ≈150–220 prod LoC, one differential `0080`), under the ADR-0045 soft gate (the 43.1 D-S431-7 / 43.2a precedent).

- The 10-task spine in `PLAN-43.2b.md` is the authoritative decomposition; no task-spine sub-split.
- **43.2b ships flat; no remaining deferral except the recorded hardening items: `goaway_sent` (reference-0-on-drain ⇒ would diverge — AMEND-H2B-3), `stream_refused_errors` (reference-dormant), `min(local,peer)` peer-min stream-cap enforcement, and `nextStreamID` 2^31 retirement.**

---

## ROADMAP posture

Row 43 (`connection-pooling`) flips **`done`** at **THIS leg's IMPL** (43.1 + 43.2a + 43.2b ALL landed) ⇒ the Upstream-robustness family (phases 39–43) **CLOSES** (ADR-0106 + `reference_roadmap_split_phase_row_done`).

- 43.1 done (ADR-0252)
- 43.2a done (ADR-0253)
- 43.2b **done** (this leg — GOAWAY rotation + reset stats — ADR-0254) — **row 43 FLIPS `done` at this Task-10 IMPL; the Upstream-robustness family (39–43) CLOSES.**

---

## Task log

| Task | Status | Description | Commit |
|------|--------|-------------|--------|
| T1 | **DONE** | PROGRESS scaffolding + green baselines + ADR-0045 split re-check (flat) | (this commit) |
| T2 | pending | `GoneAway()`/`GoneAwayCh()`/`Done()` codec accessors | |
| T3 | pending | Admission-skip + eviction generalization (`Closed() \|\| GoneAway()`) in `h2pool.go` | |
| T4 | pending | Per-conn drain-watcher goroutine (LOAD-BEARING) | |
| T5 | pending | Codec reset-hooks `WithResetHooks` + RST sites | |
| T6 | pending | Register `http2.rx_reset` + `tx_reset` + wire hooks at dial | |
| T7 | pending | `H2GoawayResponder` BackendKind 38 raw-framer backend | |
| T8 | pending | `0080-h2-goaway-rotation` differential fixture | |
| T9 | **DONE_WITH_CONCERNS** | `0080` deliberate-break proofs + flake gate + `-race` — 3/4 breaks bit the expected assertion; break (b) does NOT bite (finding, see below) | (this commit) |
| T10 | **DONE** | Completion bundle — ADR-0254 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 43 → done) + fuzzer reconcile (42→43) + full six-gate (82-dir differential GREEN) | (this commit) |

---

## Task 9 — 0080 deliberate-break proofs

Each break edited production code to disable ONE behavior, re-ran
`go test ./test/differential/ -run 'TestDifferential/0080' -count=1`, and was
reverted with `git restore` ONLY (branch + clean tree re-verified after each).
`-count=1` on every run (defeats go-test result caching). NO production edit
survives this task; the only committed change is this PROGRESS update.

### Break (a) — disable the admission-skip — BIT (expected assertion)

`internal/cluster/h2pool.go` `findStreamHitLocked`: dropped `&& !pc.cc.GoneAway()`
from the admission condition. Result: FAIL on **step 2 drain-in-flight**
(`upstream_cx_http2_total` stayed 1, ≠ 2):

```
subject: step 2 drain-in-flight: subject: stats did not converge to
map[cluster.c_h2gw.http2.streams_active:2 cluster.c_h2gw.upstream_cx_http2_total:2]
within 15s (last seen map[cluster.c_h2gw.http2.streams_active:2
cluster.c_h2gw.upstream_cx_http2_total:1]) (the GOAWAY'd conn should take NO new
stream — the 2nd request should MISS it and dial a REPLACEMENT, upstream_cx_http2_total→2)
```

Bit the EXPECTED `upstream_cx_http2_total==2` assertion. Restored clean.

### Break (b) — drop the watcher's idle-close — DID NOT BITE (FINDING)

`internal/cluster/h2pool.go` `watchDrain`: made the `<-cc.GoneAwayCh()` case a
no-op (commented the `evictH2ConnLocked` + `h2PromoteLocked` body). Result:
**PASS** (re-confirmed across 3 runs, each `-count=1`). The step-3 idle-drain
assertion (`upstream_cx_active==0`) did NOT bite.

**Root cause (proven by a diagnostic double-break):** with the watcher idle-close
AND the `makeRelease` `GoneAway()` eager-close BOTH disabled, the suite fails — but
at **step 2 drain-settle** (`upstream_cx_active==1`), not step 3. This proves the
`makeRelease` generalized eager-close (Task 3) carries BOTH the step-2 in-flight
drain-close AND the step-3 idle drain-close: in step 3 the `/__goaway` control
request is itself a transient in-flight stream on the (now-draining) idle conn, so
when that control stream releases, `makeRelease`'s `(Closed() || GoneAway()) &&
inFlight==0` eager-close evicts the conn — beating the watcher every time. The
per-conn drain-watcher (Task 4) is therefore **redundant in the 0080 drive
sequence** and has NO live differential coverage here; its sole exercise must be
the Task-4 unit tests. The 0080 fixture as written does NOT prove the watcher.

A faithful watcher-proving drive would need the GOAWAY to land on the idle conn
WITHOUT routing a fresh control stream onto that same conn (e.g. a sideband GOAWAY
trigger that is not a proxied request) so no release-driven eager-close can mask
the watcher. That is a fixture-design gap, recorded for Task 10 / future hardening.
Diagnostic edits were reverted with `git restore`; branch + clean tree re-verified.

### Break (c) — drop the rx_reset hook increment — BIT (expected assertion)

`internal/filter/hcm/h2/client.go`: commented `cc.onRxReset()` at the RST_STREAM rx
site. Result: FAIL on **step 4 rx_reset** (`http2.rx_reset` stayed 0, ≠ 1):

```
subject: step 4 rx_reset: subject: stats did not converge to
map[cluster.c_h2gw.http2.rx_reset:1] within 15s (last seen
map[cluster.c_h2gw.http2.rx_reset:0]) (a backend RST_STREAM should bump
cluster.c_h2gw.http2.rx_reset to 1)
```

Bit the EXPECTED `http2.rx_reset==1` assertion. Restored clean.

### Break (d) — drop the tx_reset hook increment — BIT (expected assertion)

`internal/filter/hcm/h2/client.go`: commented `cc.onTxReset()` at the RoundTrip
CANCEL tx site. Result: FAIL on **step 5 tx_reset** (`http2.tx_reset` stayed 0, ≠ 1):

```
subject: step 5 tx_reset: subject: stats did not converge to
map[cluster.c_h2gw.http2.tx_reset:1] within 15s (last seen
map[cluster.c_h2gw.http2.tx_reset:0]) (the per_try_timeout should make the
upstream codec emit a CANCEL → cluster.c_h2gw.http2.tx_reset==1)
```

Bit the EXPECTED `http2.tx_reset==1` assertion. Restored clean.

### Flake gate + -race

- **Flake gate: 20/20 PASS** (`-count=1` each), 0 `subject ready: EOF` transients, 0 other failures.
- **`-race`: clean** (`ok ... 4.576s`, no data races).

### Concern summary

3 of 4 breaks bit their expected assertion (a→cx_http2_total, c→rx_reset,
d→tx_reset — all LIVE). Break (b) does NOT bite: the watcher's idle-close is
masked by `makeRelease`'s eager-close because the `/__goaway` control request rides
the idle conn as an in-flight stream. The 0080 fixture does NOT prove the per-conn
drain-watcher; that behavior's only proof is the Task-4 unit tests. Flagged for Task 10.

---

## Task 10 — completion bundle (ADR-0254 + docs + six-gate)

The durable record. Edited (docs-only; NO production code touched):

- **`docs/envoy-go/DECISIONS.md`** — **ADR-0254** authored (§Decision + §Consequences;
  §Context promoted from SPEC §13). Records: the codec-visible GOAWAY signal
  (`GoneAway`/`GoneAwayCh`/`Done`); admission-skip + `(Closed()||GoneAway())` eviction;
  the per-conn drain-watcher AND its documented COVERAGE BOUNDARY (unit-proven;
  differential-masked by `makeRelease`'s eager-close); lazy replacement via the MISS
  path; the FIRST codec→cluster stat wiring (`http2.rx_reset`/`http2.tx_reset` via
  `WithResetHooks`); the rotation observed via `upstream_cx_*` (NO new GOAWAY stat —
  AMEND-H2B-1); the two `0080` fixture-drive departures (backend-global `/__release`
  broadcast; `per_try_timeout`-driven `tx_reset`); the DEFERRED hardening items
  (`goaway_sent`, `stream_refused_errors`, `min(local,peer)`, `nextStreamID` 2^31,
  + a watcher-isolating differential). DECISIONS tail ADR-0253 → **ADR-0254**;
  next-free **ADR-0255**. NEITHER supersedes nor is superseded.
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — extended the `### Cluster — HTTP/2 upstream
  multiplex connection pool` subsection with the drain-lifecycle block (admission-skip,
  in-flight drain-close-on-last-stream, idle prompt-close watcher, lazy replacement) +
  the 2 reset counters + the `0080` differential + the watcher coverage boundary; advanced
  the stat-surface running tally with the **Phase 43.2b — 1185 → 1187 (+2)** entry.
- **`docs/envoy-go/STATE.md`** — active-phase → `phase 43.2b (h2-connection-pool) IMPL done`;
  prior PLAN-done line demoted. Counts: stat **1187** (H2; non-H2 **1183**) / fixtures **82**
  (`0080`) / fuzzers **43** (RECONCILED) / BackendKind **38** (`H2GoawayResponder`) /
  DECISIONS **ADR-0254** (next-free **ADR-0255**).
- **`docs/envoy-go/ROADMAP.md`** — leg 43.2b → done; **row 43 (`connection-pooling`) FLIPS
  `in-progress → done`** (ALL legs 43.1 + 43.2a + 43.2b landed) ⇒ the **Upstream-robustness
  family (phases 39–43) CLOSES** (the rows-36/39/42 split-phase precedent; NO parent rollup).
- **`docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS-43.2b.md`** — this final state.

### Fuzzer reconcile (D-H2B-FUZZER-RECONCILE)

Verified actual `^func Fuzz` count = **43**
(`grep -c '^func Fuzz' $(git grep -l '^func Fuzz' -- '*_test.go') | awk -F: '{s+=$2} END{print "fuzzers:", s}'`
→ `fuzzers: 43`). The documented running total was advanced **42 → 43** to match (43.2b
adds NO fuzzer — a figure correction at the family-closing leg, `reference_fuzzer_count_docs_drift`).
The project-memory note `~/.claude/projects/-home-esa-git-envoy-go/memory/reference_fuzzer_count_docs_drift.md`
was updated to record the reconcile (a separate file, NOT part of this repo / commit).

### The full six-gate (LANDING gate — all GREEN)

```
$ go build ./...                                   → exit 0, no output  (CLEAN)
$ go vet ./...                                     → exit 0, no output  (CLEAN)
$ gofmt -l internal/ test/                         → exit 0, no output  (CLEAN)
$ golangci-lint run ./...                          → exit 0, no output  (CLEAN)
$ go test ./... -count=1                           → non-differential packages GREEN
    ok  github.com/esalaine/envoy-go/internal/cluster            3.798s
    ok  github.com/esalaine/envoy-go/internal/filter/hcm/h2      2.487s
$ go test ./test/differential/ -count=1            → ok ... 245.340s  (82/82 GREEN)
```

**Transient-flake note (`reference_differential_fullsuite_startup_flake`).** When the
differential runs CONCURRENTLY with the full unit suite (the combined `go test ./...`),
the heavy ~80-subprocess fixture load occasionally triggers a `subject ready: EOF`
subprocess-startup race (3-fresh-port-retries-exhausted on ONE fixture) and/or a
`TestListen_BindsRequestedPort` ephemeral-port-bind race in the echobackend test helper.
Both are subprocess startup/bind races, NOT assertion mismatches (no
`--- FAIL: TestDifferential/<fixture>` per-fixture line ever appeared). Isolate-re-run
confirms: the DEDICATED `go test ./test/differential/` passed **6/6** clean full runs
(`245.340s` final), `0080` isolated GREEN (`4.137s`), and
`TestListen_BindsRequestedPort` GREEN 3/3 in isolation. Since Task 10 is DOCS-ONLY (no
production code touched), a code regression is impossible by construction — the combined-run
failures are the documented load-dependent startup flake, not a regression.

### Result

All six gates GREEN. Row 43 → done; the Upstream-robustness family (39–43) CLOSES.
Counts: stat **1187** (H2) / **1183** (non-H2) / fixtures **82** / fuzzers **43** /
BackendKind **38** / DECISIONS **ADR-0254** (next-free **ADR-0255**).
