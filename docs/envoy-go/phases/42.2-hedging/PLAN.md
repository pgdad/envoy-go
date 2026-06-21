# Per-try-timeout (`per_try_timeout` — the per-attempt deadline, phase 42.2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `RetryPolicy.per_try_timeout` — a per-attempt deadline that wraps each `do{H1,H2}ClusterAction` call inside the existing 42.1 retry executor in a child `context.WithTimeout`. On the child deadline (while the upstream still holds), the attempt is abandoned as a synthesized **504**, `upstream_rq_per_try_timeout`++, and — when `retry_on` matches the 504/`reset` class — the EXISTING 42.1 sequential loop re-attempts. NO concurrency (the concurrent `hedgeExecutor` is 42.2b). Byte-identical when `per_try_timeout ≤ 0`.

**Architecture:** A per-attempt `attemptCtx, cancel := context.WithTimeout(ctx, perTryTimeout)` derived around each driver call in `retryExecutorH1`/`retryExecutorH2` (`internal/filter/http/router/retry.go`). AFTER the driver returns, a discriminator — `ctx.Err()==nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)` — distinguishes a per-try-timeout (override the H1 502 / H2 `Status:0` to a synthesized 504, count, classify retriable) from a parent-ctx client cancel (the 42.1 H2 `Status:0`-not-retried path). The driver code is UNCHANGED (the H1 driver already `SetDeadline`s the socket; the H2 driver already returns `Status:0` on a ctx deadline). The 42.1 `retry_on` classifier SPLITS `reset` from `connect-failure` (a per-try-timeout retries under `reset`/5xx/gateway-error but NOT `connect-failure`-alone). `per_try_timeout` is parsed in `parseRetryPolicy` (hcm) onto `RetryPolicy`; the `upstream_rq_per_try_timeout` counter folds into the 42.1 `EnsureRetryStats`. ZERO new packages/modules.

**Tech Stack:** Go; `route.v3.RetryPolicy.per_try_timeout` (`*durationpb.Duration`, go-control-plane v1.37.0 — already vendored); `internal/stats`; the `test/differential` cross-side harness (reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge).

This PLAN implements `docs/envoy-go/phases/42.2-hedging/SPEC.md` (read it first). Counts at PLAN commit UNCHANGED (stat surface **1173** / fixtures **77** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0249**, next-free **ADR-0250**). The module path is `github.com/esalaine/envoy-go`. All anchors below are verified against the worktree at `3fca7aa4`.

---

## D-question resolutions (SPEC §12) — settled at PLAN

The implementer MUST follow these (baked into the tasks).

### D-S422A-1 — the executor loop-placement of the per-try-timeout test ★ load-bearing
The per-try-timeout discrimination slots BETWEEN the driver call and the existing 42.1 `matches`/`num_retries` branch, REPLACING the single `matches` test with a timeout-aware classify. A timed-out attempt counts toward `num_retries` like any retriable outcome (the reference confirms: `retry_on:5xx num_retries:2` → exactly 3 timed-out attempts) — so it falls through to the SAME `attempt >= numRetries` check. The increment ordering is preserved: `upstream_rq_retry` + `upstream_rq_retry_backoff_exponential` fire on the NEXT iteration's retry-dispatch (the 42.1 `attempt>0` block), `upstream_rq_per_try_timeout` fires when the timeout is detected (this iteration), `upstream_rq_retry_limit_exceeded` when a timed-out attempt is at the `num_retries` cap, `upstream_rq_retry_success` only on a NON-timeout NON-retriable recovery. The exact loop shape is in Task 6.

### D-S422A-2 — the negative-`per_try_timeout` disposition: REJECT (byte-stable, ADR-0080)
A NEGATIVE `per_try_timeout` is rejected by the reference at config-load (the generic protobuf duration-positivity check, `Invalid duration: Expected positive duration` — live-confirmed `--mode validate`). envoy-go MIRRORS with a thin route-scoped reject (ADR-0080). `per_try_timeout: 0s` / unset ⇒ NO per-attempt bound (a VALID config — NOT a reject; the executor derives no child ctx). Mechanism (D-S422A-4): `NewRetryPolicy` validates `perTryTimeout < 0 → error` via a NEW shared const `ErrMsgPerTryTimeoutNegative = "per_try_timeout must not be negative"` (mirroring the `ErrMsgMaxIntervalBelowBase` drift-proofing pattern at `retry.go:20`); `parseRetryPolicy` (hcm) re-emits it route-scoped: `route: %q: retry_policy: per_try_timeout must not be negative`. Unit-level (no boot-reject dir — the 41/42.1 precedent).

### D-S422A-3 — the 504-synthesis header set
REUSE the existing local-reply header builders: `localReplyHeaders(0)` (H1, `router.go:728`) and `h2LocalReplyHeaders()` (H2, `router_h2.go:156`), with an EMPTY body (`Body: nil`). The differential asserts the status + the stats, NOT the 504 body bytes, so the exact body is parity-immaterial (match the existing synthesized-5xx shape: empty body, the standard 3-header set). The synthesized `ActionResponse{Status: 504, Headers: localReplyHeaders(0), Body: nil}` (H1) / `{Status: 504, Headers: h2LocalReplyHeaders(), Body: nil}` (H2).

### D-S422A-4 — `per_try_timeout` threaded via a WIDENED `NewRetryPolicy` (6th param)
Widen `NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, baseInterval, maxInterval, perTryTimeout time.Duration) (*RetryPolicy, error)` — the SINGLE validated constructor (the negative reject lives here alongside the `max<base` arm). The churn is mechanical: the one production caller (`parseRetryPolicy`, `config.go:645`) gains the parsed `perTryTimeout`; the `retry_test.go` callers gain a trailing `0` (no bound). Chosen over a post-construction setter (which would split validation across two methods and leave `perTryTimeout` settable post-validate). `RetryPolicy.perTryTimeout time.Duration` is unexported (the router package owns it); the executors read `rp.perTryTimeout`.

### D-S422A-5 — `0076` constants single-sourced + the H1-only cross-side posture
One `const`/`var` block at the top of the `0076` driver (`reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort = 19165` (the NEXT-FREE — verify via `grep -rhoE 'refContainerListenerPort[[:space:]]*=[[:space:]]*[0-9]+' test/fixtures/ | grep -oE '[0-9]+' | sort -n | tail -1` ⇒ expect 19164 [0075], so 19165), `refAdminPort = 9901`, `clusterPtt = "c_ptt"`, `numRetries = 3` (the N), `perTryTimeoutMs = 250` (the T — small but reliably fires while the backend holds; keeps the test ≈`(N+1)×T` ≈ 1s), `holdReleasePath = "/__release"`. The cross-side fixture is **H1-only** (the reference downstream→upstream is HTTP/1.1 — the cross-side anchor); the H2 per-try-timeout discrimination + the `Status:0` client-cancel invariant are UNIT-tested (Task 7, deterministic, no Docker). Single-source `stat_prefix` (the HCM listener's).

### D-S422A-6 — the exact surface integer: 1173 → 1181
`upstream_rq_per_try_timeout` folds into `retryStats` + `EnsureRetryStats` (the 6th member), so EVERY retry-policy cluster registers it. Re-count predicate: `+6 per NEW retry cluster` (the full six-counter block) + `+1 per EXISTING retry cluster` (the new sixth counter on the already-5-counter clusters). `0075` has TWO existing retry clusters (`c_exhaust` + `c_recover`) ⇒ +2; `0076` adds ONE new retry cluster (`c_ptt`, single held host) ⇒ +6. **1173 + 2 + 6 = 1181.** (Adding an emit-0 name to `0075`'s subject `/stats` does NOT break it — `0075` asserts NAMED deltas, and the reference emits `upstream_rq_per_try_timeout` always-on, so the name is present on both sides regardless.)

### D-S422A-7 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~140–200 LoC** across ~4 prod files (`retry.go`, `config.go`, `cluster.go`, no `circuitbreaker.go` change) + ~1 harness file (`0076` driver); **11 tasks**. Both axes comfortably under the gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** of 42.2a — it ships as the first sub-leg; 42.2b (hedging) is the pre-authorized second sub-leg (a later session).

### D-S422A-8 — the per-try-timeout attempt's status-class/outcome accounting (H1 502 vs H2 none) — an ACCEPTED departure, IMPL-verified
The H1 driver, on a per-try-deadline read-break, runs `IncStatusClass(502)` + `RecordUpstreamResult(502, LocalOriginErr:true)` INTERNALLY before returning its 502 (which the executor then OVERRIDES to a synthesized 504). The H2 driver's ctx-deadline path returns `Status:0` with NO `IncStatusClass`/`RecordUpstreamResult` — so H1 and H2 differ in the per-attempt status-class/outlier accounting on a per-try-timeout. The `0076` differential asserts ONLY `upstream_rq_per_try_timeout`/`upstream_rq_retry`/`_retry_limit_exceeded`/`upstream_rq_total`/the final 504 — it does NOT assert `upstream_rq_502`/`upstream_rq_5xx`, so it is GREEN either way. The driver code stays UNCHANGED (the SPEC promise). DISPOSITION: accept the H1-records-a-local-origin-502-on-timeout behavior as a recorded departure (a timed-out host genuinely failed locally — feeding the outlier seam is defensible), noted in the ADR-0250 §Consequences. A cheap IMPL re-probe (scrape `upstream_rq_502`/`upstream_rq_5xx` on the per-try-timeout cluster — Task 6) confirms whether the reference also counts the timed-out attempts as 5xx; if it does, there is NO divergence to record. The `0076` assertions do not depend on the outcome.

---

## File structure

**Production (`internal/`):**
- `internal/filter/http/router/retry.go` (MODIFY) — split `reset` into a NEW `retryReset` bit (`parseRetryOn` + the `matches` `localOrigin` arm `retryConnectFail|retryReset`); the `perTryTimeoutRetriable()` predicate (`matches(504,false) || retryReset`); `RetryPolicy.perTryTimeout time.Duration` + the widened `NewRetryPolicy` (6th param + the negative reject via `ErrMsgPerTryTimeoutNegative`); the per-attempt `context.WithTimeout` + the discriminator + the synthesized-504 in `retryExecutorH1`/`retryExecutorH2`.
- `internal/filter/hcm/config.go` (MODIFY, `parseRetryPolicy` :615) — parse `eff.GetPerTryTimeout()` (guarded `.AsDuration()`); pass to the widened `NewRetryPolicy`; the negative-reject route-scoped wrap (mirroring the `max<base` wrap at :649).
- `internal/cluster/cluster.go` (MODIFY) — `retryStats.perTryTimeout *stats.Counter` (the 6th member); register it in `EnsureRetryStats()` (:254); `IncUpstreamRqPerTryTimeout()` (the Inc-method pattern at :270).
- (NO `circuitbreaker.go` change — `per_try_timeout` is route-level, not a budget; NO `router.go`/`router_h2.go` driver-body change — the drivers already honor ctx deadlines; the executor edits live in `retry.go`.)

**Test harness (`test/`):**
- `test/fixtures/0076-per-try-timeout/driver/driver.go` + `driver_test.go` + `expectations.yaml` + `README.md` (CREATE). NO `fixture.go`/`runner_test.go` BackendKind change (REUSE `BlockingHoldResponder` 36). Register the fixture via the blank-import in `test/differential/runner_test.go` (the 0075 precedent).

**Docs:** `DECISIONS.md` (ADR-0250 body), `BEHAVIOR_CONTRACT.md` (the per-try-timeout sub-block + stat 1173 → 1181), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:** Create `docs/envoy-go/phases/42.2-hedging/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run + record: `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... 2>&1 | tail -20`; `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **77**-dir suite — the byte-stability anchor). Stat surface: record the running total **1173** (SPEC §14). The 42.2a exit (1173 → **1181**, +8) lands in two checkpoints: Task 5 registers the 6th counter on 0075's two existing retry clusters (+2 ⇒ 1175); Task 8's `0076` adds the new six-counter `c_ptt` cluster (+6 ⇒ 1181). The Task 5 registration test verifies the per-cluster 6-counter registration (the +2); the +6 is the new-fixture delta at Task 8.
- [ ] **Step 2: Record baselines + the task checklist** (counts: stat 1173 / fixtures 77 / fuzzers 42 / BackendKind tail 36 / DECISIONS tail ADR-0249, next-free ADR-0250; the SPEC §14 exit deltas: stat → ~1181, fixtures → 78, DECISIONS → ADR-0250).
- [ ] **Step 3: Commit** (`phase 42.2a Task 1: PROGRESS scaffold + pre-IMPL baselines`).

---

## Task 2: Split `reset` from `connect-failure` + the `perTryTimeoutRetriable` predicate

**Files:** Modify `internal/filter/http/router/retry.go`; Test `internal/filter/http/router/retry_test.go`

The 42.1 `parseRetryOn` FUSES `connect-failure`+`reset` into one `retryConnectFail` bit (`retry.go:99`). Split `reset` into its own bit so a per-try-timeout (a reset, NOT a connect-failure — SPEC §11 D-H-RETRY) classifies faithfully, while PRESERVING 42.1 (a genuine dial-refusal retries under BOTH tokens — live-confirmed). `retryReset` is the 5th `retryOnBits` value (the `uint8` has room — NO type widening):
```go
const (
	retry5xx          retryOnBits = 1 << iota // any upstream 5xx
	retryGatewayError                         // {502,503,504}
	retryConnectFail                          // local-origin connect failure
	retryStatusCodes                          // status ∈ retriableCodes
	retryReset                                // local-origin reset (a per-try-timeout is a reset; ⊇ connect-failure + timeouts)
)
```
`parseRetryOn`: `case "connect-failure": b |= retryConnectFail` and `case "reset": b |= retryReset` (SEPARATE cases — no longer fused). `matches` `localOrigin` arm becomes `if rp.on&(retryConnectFail|retryReset) != 0 && localOrigin { return true }` (BOTH tokens still retry a dial-refusal — 42.1 preserved). The new predicate:
```go
// perTryTimeoutRetriable reports whether a per-try-timeout (a synthesized 504,
// treated as a reset) is retriable under this policy: any 504-status token
// (5xx, gateway-error, retriable-status-codes∋504) OR `reset`. NOT
// connect-failure-alone (a per-try-timeout is a reset, not a connect failure).
func (rp *RetryPolicy) perTryTimeoutRetriable() bool {
	return rp.matches(504, false) || rp.on&retryReset != 0
}
```

- [ ] **Step 1: Write failing tests** in `retry_test.go`: (a) `parseRetryOn("reset")` sets `retryReset` and NOT `retryConnectFail`; `parseRetryOn("connect-failure")` sets `retryConnectFail` and NOT `retryReset`; `parseRetryOn("connect-failure reset")` sets both. (b) `matches(503, localOrigin=true)` is TRUE for `retry_on:"connect-failure"` AND for `retry_on:"reset"` (a dial-refusal retries under both — 42.1 preserved). (c) `perTryTimeoutRetriable()`: TRUE for `5xx`, `gateway-error`, `reset`, `retriable-status-codes`+{504}; FALSE for `connect-failure`-alone, for empty, for `retriable-status-codes`+{500} (504 not listed).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the `retryReset` bit + the split cases + the `matches` arm update + `perTryTimeoutRetriable`.
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/http/router/ -run Retry -count=1`).
- [ ] **Step 5:** gofmt/vet/lint on the router pkg.
- [ ] **Step 6: Commit** (`phase 42.2a Task 2: split reset from connect-failure + perTryTimeoutRetriable predicate`).

---

## Task 3: `RetryPolicy.perTryTimeout` + the widened `NewRetryPolicy` + the negative reject

**Files:** Modify `internal/filter/http/router/retry.go`; Test `retry_test.go`

Add `perTryTimeout time.Duration` to `RetryPolicy` (the `≤0` ⇒ no bound; `<0` ⇒ a config error). Widen `NewRetryPolicy` (D-S422A-4) + add the shared error const (mirroring `ErrMsgMaxIntervalBelowBase` at :20):
```go
// ErrMsgPerTryTimeoutNegative is the retry_policy per_try_timeout reject suffix.
// The hcm parse layer re-emits it with a "route: %q: retry_policy: " prefix;
// keep both sites referencing this const so they cannot drift.
const ErrMsgPerTryTimeoutNegative = "per_try_timeout must not be negative"

func NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, baseInterval, maxInterval, perTryTimeout time.Duration) (*RetryPolicy, error) {
	// ... existing body (on/numRetries/retriableCodes/base/max + max<base reject) ...
	if perTryTimeout < 0 {
		return nil, errors.New(ErrMsgPerTryTimeoutNegative)
	}
	rp.perTryTimeout = perTryTimeout // 0 ⇒ no bound (executor skips the child ctx)
	return rp, nil
}
```
(`errors` is already imported in `retry.go`.) Update the existing `retry_test.go` `NewRetryPolicy(...)` callers to pass a trailing `0` (no bound).

- [ ] **Step 1: Write failing tests:** (a) `NewRetryPolicy("5xx", 3, nil, 0, 0, 250*time.Millisecond)` ⇒ `perTryTimeout==250ms`, no error; (b) `NewRetryPolicy("5xx", 1, nil, 0, 0, -1)` ⇒ error (`ErrMsgPerTryTimeoutNegative`); (c) `NewRetryPolicy("5xx", 1, nil, 0, 0, 0)` ⇒ `perTryTimeout==0`, no error (no bound); (d) the existing max<base reject still fires with the new arg present.
- [ ] **Step 2: Run → FAIL** (signature mismatch + missing field).
- [ ] **Step 3: Implement** the field + the widened signature + the negative reject const; fix the existing test call sites (trailing `0`).
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/http/router/ -run Retry -count=1`) + `go build ./...` (the `parseRetryPolicy` caller in hcm now mismatches — it is fixed in Task 4; if Task 4 is not yet landed, temporarily pass `0` at `config.go:645` to keep the build green, noted in PROGRESS, and Task 4 replaces it with the parsed value).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.2a Task 3: RetryPolicy.perTryTimeout + widened NewRetryPolicy + negative reject`).

> **Ordering note:** Task 3 widens `NewRetryPolicy`, whose sole production caller is `parseRetryPolicy` (Task 4). Either land Task 4's parse in the same breath, or pass a temporary `0` at `config.go:645` in Task 3 (build-green) and replace it in Task 4. The controller picks; PROGRESS records which.

---

## Task 4: The `per_try_timeout` parse in hcm + the negative route-scoped reject

**Files:** Modify `internal/filter/hcm/config.go` (`parseRetryPolicy` :615); Test `internal/filter/hcm/config_test.go`

In `parseRetryPolicy`, after the `num_retries`/backoff block and BEFORE the `NewRetryPolicy` call, parse `per_try_timeout` (guarded `.AsDuration()` — nil-safe):
```go
var ptt time.Duration
if d := eff.GetPerTryTimeout(); d != nil {
	ptt = d.AsDuration()
}
rp, err := router.NewRetryPolicy(eff.GetRetryOn(), v, eff.GetRetriableStatusCodes(), base, mx, ptt)
if err != nil {
	// NewRetryPolicy's error arms are max<base AND per_try_timeout<0; re-emit
	// route-scoped, SUFFIX byte-identical to the Task-2/3 strings.
	return nil, fmt.Errorf("route: %q: retry_policy: %s", name, err.Error())
}
```
(NOTE: the existing wrap at :649 hardcodes `router.ErrMsgMaxIntervalBelowBase`; generalize it to `err.Error()` so BOTH the max<base and the per_try_timeout-negative suffixes flow through unchanged — verify both suffixes stay byte-identical. `time` is already imported in hcm; `durationpb` is NOT needed — `eff.GetPerTryTimeout()` returns the `*durationpb.Duration` and `.AsDuration()` is a method call requiring no import.)

- [ ] **Step 1: Write failing tests** (`config_test.go`): (a) a route with `retry_policy{retry_on:"5xx", num_retries:2, per_try_timeout:250ms}` ⇒ the built `RetryPolicy` carries `perTryTimeout==250ms` (add an exported test accessor `(*RetryPolicy) PerTryTimeout() time.Duration` in `retry.go` if none exists — mirror `NumRetries()` at :73); (b) `per_try_timeout:-1s` ⇒ the reject `route: %q: retry_policy: per_try_timeout must not be negative`; (c) `per_try_timeout` unset ⇒ `perTryTimeout==0` (no bound); (d) the existing max<base reject still fires unchanged (byte-identical suffix).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the parse + the generalized reject wrap + the `PerTryTimeout()` accessor.
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/hcm/ -count=1`) + `go build ./...` + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (all **77** GREEN — no fixture sets `per_try_timeout`).
- [ ] **Step 5:** gofmt/vet/lint on `internal/`.
- [ ] **Step 6: Commit** (`phase 42.2a Task 4: per_try_timeout parse + negative route-scoped reject`).

---

## Task 5: The `upstream_rq_per_try_timeout` counter (the 6th `retryStats` member)

**Files:** Modify `internal/cluster/cluster.go`; Test `internal/cluster/cluster_test.go`

Add `perTryTimeout *stats.Counter` to `retryStats` (`cluster.go:156`); register it in `EnsureRetryStats()` (:254); add the Inc method (the :270 pattern):
```go
type retryStats struct {
	rq, success, limitExceeded, backoffExp, backoffRL, perTryTimeout *stats.Counter
}
// in EnsureRetryStats(), alongside the existing 5:
perTryTimeout: c.statsReg.NewCounter(p + "upstream_rq_per_try_timeout"),

func (c *Cluster) IncUpstreamRqPerTryTimeout() {
	if c.retry != nil {
		c.retry.perTryTimeout.Inc()
	}
}
```

- [ ] **Step 1: Write a failing test:** after `registerClusterMetrics` + `EnsureRetryStats()`, the cluster registers EXACTLY the 6 `upstream_rq_retry*`/`upstream_rq_per_try_timeout` counters (registry introspection — the 42.1 Task-5 pattern); `IncUpstreamRqPerTryTimeout()` increments it; a cluster WITHOUT `EnsureRetryStats()` has NONE of them; idempotent on a second `EnsureRetryStats()` call.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the field + the registration + the Inc method.
- [ ] **Step 4: Run → PASS** (`go test ./internal/cluster/ -count=1`) + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (all **77** GREEN — adding the emit-0 name to `0075`'s two retry clusters does NOT break their named-delta asserts; the reference emits it always-on). Record: `0075`'s two retry clusters now register 6 each (+2); the suite surface is **1173 → 1175** at THIS task (the +6 for `0076` lands at Task 8). 
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.2a Task 5: upstream_rq_per_try_timeout counter (6th retryStats member via EnsureRetryStats)`).

---

## Task 6: The `retryExecutorH1` per-attempt deadline + discriminator + synthesized-504

**Files:** Modify `internal/filter/http/router/retry.go` (`retryExecutorH1`); Test `retry_test.go`

Wrap the `doH1ClusterAction` call (`retry.go:170`) in the per-attempt child ctx + the discriminator. ★ Use an EXPLICIT `cancel()` (NOT `defer` in the loop — a defer accumulates timers until the executor returns; and a fired deadline's `DeadlineExceeded` survives an explicit `cancel()` because context's first-error-wins, so reading `attemptCtx.Err()` after `cancel()` is correct):
```go
// inside the for-loop, REPLACING the bare `resp, ep, err = doH1ClusterAction(ctx, a, req)`:
attemptCtx := ctx
var cancel context.CancelFunc
if rp.perTryTimeout > 0 {
	attemptCtx, cancel = context.WithTimeout(ctx, rp.perTryTimeout)
}
resp, ep, err = doH1ClusterAction(attemptCtx, a, req)
if cancel != nil {
	cancel()
}
if attempt > 0 {
	a.cluster.ReleaseRetry()
}
// per-try-timeout discrimination (AMEND-PT4): the CHILD attemptCtx deadline fired
// while the PARENT ctx is still alive ⇒ a per-try-timeout (NOT a client cancel).
timedOut := rp.perTryTimeout > 0 && ctx.Err() == nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
if timedOut {
	a.cluster.IncUpstreamRqPerTryTimeout()
	resp = ActionResponse{Status: 504, Headers: localReplyHeaders(0), Body: nil} // synthesized 504, override the driver's 502
	err = nil
	if !rp.perTryTimeoutRetriable() {
		return resp, ep, err
	}
} else if !rp.matches(resp.Status, resp.localOrigin) {
	if attempt > 0 {
		a.cluster.IncUpstreamRqRetrySuccess()
	}
	return resp, ep, err
}
if attempt >= rp.numRetries {
	a.cluster.IncUpstreamRqRetryLimitExceeded()
	return resp, ep, err
}
```
(NOTE: `context` is already imported in `retry.go`; the `ReleaseRetry`/`IncUpstreamRqRetrySuccess` placement preserves the 42.1 ledger. A timed-out attempt that IS retriable falls through to the `attempt >= numRetries` check — a per-try-timeout counts toward `num_retries` like any retriable outcome, D-S422A-1. A timed-out attempt that is NOT retriable returns the synthesized 504 directly — NO `retry_success` (it did not recover).)

- [ ] **Step 1: Write failing tests** (router pkg, over a controllable slow/held backend — a small `net.Listen` server that blocks before responding, OR the existing driver test harness with a hang-until-signal backend): (a) EXHAUSTION — a single always-blocking host + `retry_on:"5xx", num_retries:3, per_try_timeout:50ms` ⇒ final **504**, `upstream_rq_per_try_timeout==4`, `upstream_rq_retry==3`, `upstream_rq_retry_limit_exceeded==1`, `upstream_rq_total==4`; (b) NOT-RETRIABLE per-try-timeout — `retry_on:"connect-failure"` (only) + the blocking host ⇒ final 504, `upstream_rq_per_try_timeout==1`, `upstream_rq_retry==0` (NOT retried); (c) `retry_on:"reset"` (only) + the blocking host ⇒ retried (`upstream_rq_retry==3`); (d) BYTE-STABILITY — `per_try_timeout==0` + a fast 200 backend ⇒ no child ctx, behaves exactly as 42.1 (a `context`-deadline-free path; assert `upstream_rq_per_try_timeout==0`); (e) the explicit-cancel: a FAST 200 attempt under `per_try_timeout:5s` ⇒ `timedOut==false` (the cancel makes `attemptCtx.Err()==Canceled`, not DeadlineExceeded).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the per-attempt ctx + discriminator + synthesized-504 in `retryExecutorH1`.
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/http/router/ -race -count=1`) + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (all **77** GREEN — `per_try_timeout==0` everywhere). OPTIONAL IMPL re-probe (D-S422A-8): scrape `cluster.c_slow.upstream_rq_502`/`upstream_rq_5xx` on the SPEC's per-try-timeout probe to confirm whether the reference counts timed-out attempts as 5xx (informs the ADR §Consequences departure note; the `0076` asserts neither).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.2a Task 6: retryExecutorH1 per-attempt deadline + discriminator + synthesized 504`).

---

## Task 7: The `retryExecutorH2` per-attempt deadline + the `Status:0` client-cancel invariant

**Files:** Modify `internal/filter/http/router/retry.go` (`retryExecutorH2`); Test `retry_test.go`

Mirror Task 6 in `retryExecutorH2` (`retry.go:215`), with `h2LocalReplyHeaders()` for the synthesized 504. The LOAD-BEARING H2 distinction (SPEC §3.2/AMEND-PT4): the H2 driver returns `Status:0` + an `*h2.Error` for BOTH a child per-try-timeout AND a parent client-cancel. The discriminator (`ctx.Err()==nil && attemptCtx.Err()==DeadlineExceeded`) catches the per-try-timeout (override to 504, count, classify) while a PARENT client-cancel (`ctx.Err()!=nil`) falls through to the EXISTING 42.1 path where `matches(0,false)==false` ⇒ returned `Status:0` un-retried. The 42.1 `TestRetryExecutorH2_CtxCancelNotRetried` invariant MUST still pass.
```go
// in retryExecutorH2, REPLACING the bare `resp, ep, err = doH2ClusterAction(ctx, a, req)`:
attemptCtx := ctx
var cancel context.CancelFunc
if rp.perTryTimeout > 0 {
	attemptCtx, cancel = context.WithTimeout(ctx, rp.perTryTimeout)
}
resp, ep, err = doH2ClusterAction(attemptCtx, a, req)
if cancel != nil {
	cancel()
}
if attempt > 0 {
	a.cluster.ReleaseRetry()
}
timedOut := rp.perTryTimeout > 0 && ctx.Err() == nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
if timedOut {
	a.cluster.IncUpstreamRqPerTryTimeout()
	resp = ActionResponse{Status: 504, Headers: h2LocalReplyHeaders(), Body: nil}
	err = nil
	if !rp.perTryTimeoutRetriable() {
		return resp, ep, err
	}
} else if !rp.matches(resp.Status, resp.localOrigin) {
	if attempt > 0 {
		a.cluster.IncUpstreamRqRetrySuccess()
	}
	return resp, ep, err
}
if attempt >= rp.numRetries {
	a.cluster.IncUpstreamRqRetryLimitExceeded()
	return resp, ep, err
}
```

- [ ] **Step 1: Write failing tests** (H2 driver harness): (a) H2 per-try-timeout exhaustion (a blocking H2 upstream + `retry_on:"5xx", num_retries:2, per_try_timeout:50ms`) ⇒ final 504, `upstream_rq_per_try_timeout==3`, `upstream_rq_retry==2`, `_retry_limit_exceeded==1`; (b) the PARENT client-cancel invariant — a caller-cancelled ctx (parent) + `per_try_timeout:5s` ⇒ `Status:0` returned un-retried, `upstream_rq_per_try_timeout==0`, `upstream_rq_retry==0` (the per-try child did NOT fire; the parent cancel is the cause — `TestRetryExecutorH2_CtxCancelNotRetried` parity); (c) `per_try_timeout==0` byte-stability (a fast H2 200 ⇒ no child ctx).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `retryExecutorH2`'s per-attempt ctx + discriminator + synthesized-504.
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/http/router/ -race -count=1`, incl. the preserved `TestRetryExecutorH2_CtxCancelNotRetried`) + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (77 GREEN).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.2a Task 7: retryExecutorH2 per-attempt deadline + Status:0 client-cancel invariant preserved`).

---

## Task 8: The `0076-per-try-timeout` cross-side fixture

**Files:** Create `test/fixtures/0076-per-try-timeout/driver/{driver.go,driver_test.go}` + `expectations.yaml` + `README.md`; Modify `test/differential/runner_test.go` (blank-import the new fixture)

Model on `test/fixtures/0074-circuit-breaker-max-requests/driver/driver.go` (the `BlockingHoldResponder` + `/__release` cross-side precedent) + `0075-retry-loop` (the retry-cluster `StatsAsserter` shape). Topology: **1 backend** (`BackendKindAt: 0 ⇒ BlockingHoldResponder (36)`, `BackendCount()==1`); **1 cluster** `c_ptt` = `[backendPorts[0]]` on both bootstraps; **1 route** `/ptt → c_ptt {retry_policy:{retry_on:"5xx", num_retries:3, per_try_timeout:250ms}}`. `--concurrency 1` (the harness default). Constants single-sourced (D-S422A-5); the cross-side fixture is **H1-only**.

`AssertStats(t, refAdminAddr, subjAdminAddr)` — cross-side via `StatsAsserter` (NOT `SubjectAsserter` — `reference_differential_asserter_dispatch`); sequential-per-side (the shared in-process held backend idle between sides — `reference_concurrency_differential_release_barrier`):
```
For each side (addr=listener, adminAddr):
  base := scrapeStats(adminAddr)
  // ONE request: the held host never responds within per_try_timeout, so every
  // attempt (1 + numRetries) times out → final 504. The held responder accumulates
  // one parked goroutine per timed-out attempt (it blocks on the gate, not the
  // conn) — drained by /__release after the assert.
  resp := GET addr/ptt   → assert resp.StatusCode == 504
  fin := scrapeStats(adminAddr)
  // cross-side EXACT (single held host — offset-irrelevant):
  assertDelta(c_ptt.upstream_rq_per_try_timeout, numRetries+1)   // ==4 (every attempt timed out)
  assertDelta(c_ptt.upstream_rq_retry, numRetries)               // ==3
  assertDelta(c_ptt.upstream_rq_retry_limit_exceeded, 1)
  assertDelta(c_ptt.upstream_rq_retry_success, 0)
  assertDelta(c_ptt.upstream_rq_total, numRetries+1)             // ==4
  assert(c_ptt.upstream_rq_total > 0)                            // decode-ran guard (ref)
  GET 127.0.0.1:<backendPort>/__release                          // drain the parked held attempts
```
The `per_try_timeout` T (250ms) is the FEATURE's own timing — the request completes in ≈`(numRetries+1)×T` ≈ 1s; NOT a `time.Sleep`-margin assertion (AMEND-PT6). NO new BackendKind (REUSE `BlockingHoldResponder` 36 — the same `/__release` re-armable gate the driver caches `<backendPort>` for on `127.0.0.1`).

- [ ] **Step 1:** Write `driver_test.go` (the per-fixture helper table test — the 0074/0075 precedent) → run → FAIL.
- [ ] **Step 2:** Write `driver.go` (`Name`, `BackendCount`, `BackendKindAt`, `SubjectListenerName`, `ReferenceBootstrap`, `SubjectConfig`, `ReferenceListenerPort`, `DriveReference`/`DriveSubject`, `ProbeAdmin`, `AssertStats` + the `scrapeStats`/`assertDelta`/`releaseHeld` helpers copied from 0074/0075) + `expectations.yaml` + `README.md`. Single-source the constants (D-S422A-5). Blank-import in `runner_test.go`.
- [ ] **Step 3:** `go test ./test/fixtures/0076-per-try-timeout/driver/ -count=1` (the unit test) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0076' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix is REQUIRED). Expected PASS: both sides exhaust `/ptt` to 504 with `upstream_rq_per_try_timeout==4`, `upstream_rq_retry==3`.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **77 → 78**, stat surface → **1181** (the +6 for `c_ptt` joins the +2 from `0075` recorded at Task 5).
- [ ] **Step 6: Commit** (`phase 42.2a Task 8: 0076 cross-side per-try-timeout fixture (held-host exhaustion-exact)`).

---

## Task 9: `0076` deliberate breaks + 20-run flake

**Files:** none committed (verification; SPEC §8 break protocol).

★ `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/0076` selector.

- [ ] **Step 1: Break (A) — `per_try_timeout` not threaded.** Temporarily make `retryExecutorH1` ignore `rp.perTryTimeout` (skip the `context.WithTimeout`, always pass the parent `ctx`). Run `go test ./test/differential/ -run 'TestDifferential/0076' -count=1` → MUST FAIL: the attempt blocks indefinitely on the held backend (no timeout) ⇒ the test hangs to its deadline / the final status is not 504 + `upstream_rq_per_try_timeout==0`. Restore. (★ guard the run with a generous test timeout so a genuine hang surfaces as a failure, not a stuck CI — `go test -timeout 60s`.)
- [ ] **Step 2: Break (B) — the per-try-timeout counter not wired.** Temporarily make `IncUpstreamRqPerTryTimeout` a no-op. Run → MUST FAIL (the cross-side `c_ptt.upstream_rq_per_try_timeout` delta asserts 0 ≠ 4). Restore.
- [ ] **Step 3: Confirm both restored** (`git diff` clean; the fixture PASSES).
- [ ] **Step 4: 20-run flake gate:** `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0076' -count=1 -timeout 90s || echo "FAIL $i"; done` → 20/20 PASS (the per-try-timeout firing is deterministic [held > T]; the counts are exact; if any flake, it is the unrelated startup race `reference_differential_fullsuite_startup_flake`, NOT a count issue — isolate-re-run; NEVER add a fixed sleep or widen T to "fix" a flake).
- [ ] **Step 5:** Record the break + flake results in PROGRESS. (No commit.)

---

## Task 10: Full 78-dir differential + six-gate

**Files:** none (verification); update PROGRESS.

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL **78** GREEN). Note the unrelated `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the named dir + a full re-run to distinguish from a regression.
- [ ] **Step 2: Counts → stat surface 1181; fixtures 78; fuzzers 42; BackendKind tail 36.** Record in PROGRESS.
- [ ] **Step 3:** If any gate fails, fix + re-run before proceeding.

---

## Task 11: ADR-0250 body + BEHAVIOR_CONTRACT + completion bundle

**Files:** Modify `docs/envoy-go/DECISIONS.md` (ADR-0250 full entry — §Decision + §Consequences; promote/refine the §Context from SPEC §13; DECISIONS tail ADR-0249 → **ADR-0250**, next-free ADR-0251); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the per-try-timeout sub-block in `### Route — request recovery (retries)`; advance the stat-surface block **1173 → 1181**); `docs/envoy-go/phases/42.2-hedging/PROGRESS.md` + CREATE `README.md`; `docs/envoy-go/STATE.md`; `docs/envoy-go/ROADMAP.md`; `next-prompt.txt`.

- [ ] **Step 1:** Write the ADR-0250 body. §Decision: the per-attempt `context.WithTimeout` in the 42.1 executor; the explicit-cancel-not-defer + the discriminator (`parentCtx.Err()==nil && attemptCtx.Err()==DeadlineExceeded`); the synthesized-504; the `reset`/`connect-failure` split + the `perTryTimeoutRetriable()` predicate; the `upstream_rq_per_try_timeout` counter via `EnsureRetryStats`; the no-global-`RouteAction.timeout` posture. §Consequences: byte-identical when `per_try_timeout ≤ 0`; the H1-records-a-local-origin-502-on-timeout departure (D-S422A-8); the deferred `per_try_idle_timeout`/global `RouteAction.timeout`/global `upstream_rq_timeout` (AMEND-PT3/PT7); the negative-reject (AMEND-PT5); NO concurrency (42.2b layers it — ADR-0251).
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT per-try-timeout sub-block + the stat-count bump 1173 → 1181 (the `Phase 42.2a` running-total entry).
- [ ] **Step 3:** Update PROGRESS (the 11-task record + the six-gate evidence + the break/flake results + the exit-delta table); write the phase README; update STATE (active-phase → `phase 42.2a (per_try_timeout) IMPL done`; counts → 1181 / 78 / 42 / 36 / ADR-0250); ROADMAP (row 42 STAYS `in-progress` — 42.2b hedging remains; append the 42.2a IMPL note); roll `next-prompt.txt` to the **42.2b (hedging) BRAINSTORM**.
- [ ] **Step 4: Final six-gate re-confirm** + record all exit counts.
- [ ] **Step 5: Commit** (`phase 42.2a Task 11: ADR-0250 body + BEHAVIOR_CONTRACT per-try-timeout + completion bundle (row 42 stays in-progress for 42.2b)`).
- [ ] **Step 6:** The controller squashes the 11 task commits + pushes to origin/master (`feedback_subagents_no_push` — subagents commit locally only; the controller squashes at stage-close + pushes per `feedback_push_to_origin`).

---

## Exit deltas (SPEC §14)

| Axis | At PLAN | At 42.2a IMPL |
|------|---------|---------------|
| stat surface | 1173 | **1181** (+1 `upstream_rq_per_try_timeout` per retry cluster: +2 on 0075's two, +6 for 0076's new six-counter cluster) |
| differential fixtures | 77 | **78** (`0076-per-try-timeout`) |
| fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 36 | 36 (REUSE `BlockingHoldResponder` 36) |
| DECISIONS tail | ADR-0249 | **ADR-0250** (next-free ADR-0251) |
| new packages / go.mod modules | — | ZERO / ZERO |
| ROADMAP row 42 | in-progress | **in-progress** (42.2b hedging remains; flips `done` only when ALL THREE legs land) |

Next → the phase-42.2a IMPL (`superpowers:subagent-driven-development` — fresh subagent per task + two-stage review); then the **42.2b (hedging) BRAINSTORM**.
