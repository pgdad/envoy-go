# Phase 42.2a SPEC — `per_try_timeout`: a per-attempt deadline over the 42.1 sequential retry loop — the FIRST sub-leg of the hedging leg (the SECOND leg of the FOURTH Upstream-robustness-family row)

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-42.2 (hedging) BRAINSTORM (`docs/envoy-go/phases/42.2-hedging/BRAINSTORM.md`, commit `3979881f`). This SPEC charters phase **42.2a** — the per-attempt timeout (`RetryPolicy.per_try_timeout`, field 3): a per-attempt `context` deadline derived around each `do{H1,H2}ClusterAction` call inside the 42.1 retry executor, the project's FIRST request-scoped deadline of any kind. On the deadline, the attempt fails as a synthesized **504** that the 42.1 sequential loop re-attempts (NO concurrency at 42.2a — the concurrent `hedgeExecutor` is 42.2b). The `upstream_rq_per_try_timeout` counter folds into the 42.1 `EnsureRetryStats` scoping. Counts at SPEC commit UNCHANGED (stat surface **1173** / fixtures **77** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0249**, next-free **ADR-0250**). The §11 D-H-* per-try-timeout empirical pins were EXECUTED IN-SESSION (2026-06-21) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land **per-attempt time-bounding** at the route boundary: a route whose `retry_policy` carries `per_try_timeout` bounds EACH upstream attempt by a deadline. The 42.1 retry executor already loops one `do{H1,H2}ClusterAction` call per attempt; 42.2a wraps each call in a per-attempt child `context` (`context.WithTimeout(reqCtx, perTryTimeout)`). When the child deadline expires while the upstream still holds, the attempt is abandoned as a synthesized **504 Gateway Timeout**, `upstream_rq_per_try_timeout`++, and — because a `per_try_timeout`-expiry is classified as a retriable 504 — the EXISTING 42.1 sequential loop re-attempts (under the same `num_retries` + `retry_budget` caps). This is the project's FIRST request-scoped deadline (envoy-go does NOT implement the global `RouteAction.timeout` — §11 D-H-TIMEOUT), and the substrate the 42.2b concurrent hedge will share. Byte-identical when no `per_try_timeout` (the nil/zero-guard — the executor derives no child ctx).

42.2a is the FIRST sub-leg of the pre-authorized 42.2a/42.2b sub-split (the BRAINSTORM §1.4 / §2.2): the per-attempt deadline + the `upstream_rq_per_try_timeout` counter + the per-try-timeout-vs-client-cancel discriminator + the `0076` differential. The concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` + the `initial_requests`/`additional_request_chance` fan-out is 42.2b. Row 42 flips `done` only when ALL THREE legs (42.1 + 42.2a + 42.2b) land (ADR-0106 + `reference_roadmap_split_phase_row_done`).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments/refinements to the BRAINSTORM design.

- **AMEND-PT1 (per-try-timeout = a synthesized 504; the retriable classification splits `reset` from `connect-failure`).** Live finding (D-H-RETRY): a per-attempt-timeout expiry produces a synthesized **504 Gateway Timeout** (confirmed: every probe whose attempts all timed out returned final `504`; `cluster.<n>.upstream_rq_504` incremented per request, `http.<prefix>.downstream_rq_5xx` per request). Whether the loop RE-ATTEMPTS a per-try-timeout depends on `retry_on` — confirmed by probe over a single slow host (`per_try_timeout:0.5s`, backend delay 3s, `num_retries:2`): `retry_on:"5xx"` → **3 attempts** (504 ∈ 5xx, retried); `retry_on:"gateway-error"` → **3 attempts** (504 ∈ {502,503,504}); `retry_on:"reset"` → **3 attempts** (a per-try-timeout IS a reset); `retry_on:"connect-failure"` → **1 attempt** (NOT retried); `retry_on` OMITTED → **1 attempt** (NOT retried). So a per-try-timeout is retriable under `{5xx, gateway-error, retriable-status-codes (if 504∈list), reset}` but **NOT** `connect-failure`-alone. This DIVERGES from a genuine dial-refusal, which (confirmed by the dead-backend probe) retries under **BOTH** `reset` AND `connect-failure` (`c_dead.upstream_rq_retry` delta=2 for each, final 503). So envoy-go's 42.1 fusion (`connect-failure`==`reset`==`retryConnectFail`, retriable iff `localOrigin`) is FAITHFUL for dial-failures but cannot distinguish the per-try-timeout case. 42.2a SPLITS `reset` into its own bit `retryReset` (the `localOrigin` dial-failure path in `matches` retries under EITHER `retryConnectFail|retryReset`, preserving 42.1), and the per-try-timeout retriable predicate is `matches(504, localOrigin=false) || (on & retryReset != 0)` — i.e. {5xx, gateway-error, retriable-status-codes∋504, reset}, NOT `connect-failure`. §3.
- **AMEND-PT2 (stat surface — +1 counter, but folded into `EnsureRetryStats` ⇒ +1 PER retry cluster; 1173 → ~1181, NOT the BRAINSTORM's +1).** Live finding (D-H-STATS): the counter name is exactly `cluster.<n>.upstream_rq_per_try_timeout` (confirmed via `/stats` scrape), incrementing ONCE per timed-out ATTEMPT (delta `+3` for a 3-attempt request, `+1` for a 1-attempt request — per-attempt, not per-request). It is the natural sixth member of the 42.1 retry-counter block: 42.2a folds it into `retryStats` + `Cluster.EnsureRetryStats()` (`cluster.go:254`), so EVERY retry-policy cluster registers it (emit-0 on retry clusters with no `per_try_timeout`). The surface-registration consequence (the BRAINSTORM's "~1174" counted the new NAME, not the per-cluster registration model — `1163→1173` was `+5 × 2 clusters`): `0075`'s TWO existing retry clusters each gain `+1` (= +2), and the new `0076` retry cluster registers the full SIX-counter block (= +6). Anticipated surface **1173 → ~1181** (+8), FIRM once `0076`'s cluster topology is pinned at the PLAN (a single-held-host single retry cluster ⇒ +6; +2 from `0075`). The BEHAVIOR_CONTRACT doc-count advances accordingly. (Adding an emit-0 name to `0075`'s subject `/stats` does NOT break `0075` — it asserts NAMED deltas, and the reference emits `upstream_rq_per_try_timeout` always-on, so the name is present on both sides regardless.)
- **AMEND-PT3 (no global `RouteAction.timeout` in envoy-go — `per_try_timeout` is the FIRST request deadline; no `min(per_try, global)` clamp).** Live finding (D-H-TIMEOUT, code-confirmed): envoy-go does NOT parse or consume `RouteAction.timeout` (field 8) anywhere (`grep` of `internal/filter/hcm` + `internal/filter/http/router` finds only H2-framer socket-read deadlines — unrelated). The reference implements a global route timeout (default 15s — confirmed: a `per_try_timeout`-less route with `timeout:6s` over a 3s backend returned `200` at 3s, bounded only by the route timeout). envoy-go does NOT model it. CONSEQUENCE: `per_try_timeout` is envoy-go's FIRST request-scoped deadline of ANY kind; there is NO global deadline to clamp against, so the BRAINSTORM's `min(per_try_timeout, remaining global)` interaction is MOOT (the per-attempt child ctx derives from the request ctx, which carries no route-timeout deadline). The global `RouteAction.timeout` stays DEFERRED (§2). `per_try_timeout` unset or `0s` ⇒ NO per-attempt bound (confirmed: `0s` did not instantly fire — the request reached the backend; the standard Envoy "0 disables" semantic).
- **AMEND-PT4 (the per-try-timeout-vs-client-cancel discriminator — the child `attemptCtx` vs the parent; the H2 `Status:0` coexistence).** The load-bearing IMPL design (D-H-RETRY): the executor wraps each attempt with `attemptCtx, cancel := context.WithTimeout(parentCtx, perTryTimeout)` and passes `attemptCtx` to the driver. The H1 driver ALREADY propagates the ctx deadline to the upstream socket (`router.go:659-661` `upstream.SetDeadline(dl)`) so a held read fails at the deadline → its existing 502-`localOrigin` return; the H2 driver ALREADY treats ANY `ctx.Err()` deadline/cancel as the `Status:0` + `*h2.Error` CANCEL sentinel (`router_h2.go:111`). So AFTER the driver returns, the executor discriminates: if `parentCtx.Err() == nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)` ⇒ a **per-try-timeout** (override the driver's 502/`Status:0` to a synthesized **504**, `upstream_rq_per_try_timeout`++, classify retriable per AMEND-PT1); ELSE the existing 42.1 behavior holds (a parent-ctx client cancel still returns H2 `Status:0` un-retried — `matches(0,false)==false`; the 42.1 `TestRetryExecutorH2_CtxCancelNotRetried` invariant must STILL pass). The driver code is UNCHANGED (it already honors ctx deadlines); the discrimination + the 504-synthesis live in the executor. `defer cancel()` per attempt (no timer leak).
- **AMEND-PT5 (reject surface — a negative `per_try_timeout` is rejected by the reference; `0s`/unset = no bound; thin envoy-go guard; NO new fuzzer).** Live finding (D-H-PROTO): a negative `per_try_timeout` (`-1s`) is REJECTED at `--mode validate` with `Invalid duration: Expected positive duration` — a GENERIC protobuf duration-positivity check (not a `per_try_timeout`-specific PGV; the same check that guards every Duration). `0s` is a VALID config meaning "no per-attempt bound". envoy-go parses `per_try_timeout` via a guarded `.AsDuration()` and treats `≤ 0s` (incl. unset) as NO per-attempt bound (the executor derives no child ctx); a NEGATIVE value is a degenerate config the reference rejects — envoy-go MIRRORS with a thin reject (ADR-0080 byte-stable, house wording `route: %q: retry_policy: per_try_timeout must not be negative`) OR clamps-to-no-bound (a PLAN call, D-S422A-2). NO new fuzzer (config-parse, unit-tested — the 41/42.1 precedent); fuzzers STAY **42**.
- **AMEND-PT6 (differential — a single held host = a deterministic cross-side-EXACT exhaustion to 504; the `per_try_timeout` T delay is feature-inherent, not a flake-sleep).** Live finding (D-H-DIFFERENTIAL): a single held `BlockingHoldResponder` host (held PAST a small `per_try_timeout` T) makes the per-try-timeout fire DETERMINISTICALLY on EVERY attempt (the host never responds within T), so the request exhausts to a final **504** with cross-side-EXACT counts: `upstream_rq_per_try_timeout == num_retries+1` (every attempt timed out), `upstream_rq_retry == num_retries`, `upstream_rq_retry_limit_exceeded == 1`. SINGLE host ⇒ offset-irrelevant (no `reference_round_robin_offset_randomized` exposure). The T delay (e.g. 200–500ms) is the FEATURE's own timing — the test waits ≈`(num_retries+1)×T` for the request to complete — NOT a `time.Sleep`-to-guess-a-budget (`reference_concurrency_differential_release_barrier`: count-immune, no timing-margin assertion). The held attempts are freed via `/__release` at the END (drain — the held responder goroutines unblock on the gate). `0076-per-try-timeout`. NO new BackendKind (REUSE `BlockingHoldResponder` 36).
- **AMEND-PT7 (the global `upstream_rq_timeout` counter is DISTINCT + DEFERRED).** Live finding (D-H-STATS): the reference's per-try-timeout counter (`upstream_rq_per_try_timeout`) is DISTINCT from the global-request-timeout counter (`upstream_rq_timeout`, which stayed **0** throughout the per-try-timeout probes — it tracks `RouteAction.timeout` expiries, which envoy-go does not implement — AMEND-PT3). 42.2a registers ONLY `upstream_rq_per_try_timeout`; `upstream_rq_timeout` stays UNREGISTERED (deferred with the global route timeout). A recorded departure (the reference emits `upstream_rq_timeout` always-on).

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0250 (the per-attempt-timeout architecture — the per-attempt child `context` deadline derived in the retry executor + the per-try-timeout-vs-client-cancel discriminator + the synthesized-504 + the `reset`-vs-`connect-failure` split + the per-try-timeout retriable predicate + the `upstream_rq_per_try_timeout` counter folded into `EnsureRetryStats` + the no-global-`RouteAction.timeout` posture) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 42.2a IMPL per ADR-0044. DECISIONS tail STAYS ADR-0249 at this SPEC; next-free ADR-0250. The §10 BRAINSTORM D-H pins (the per-try-timeout subset) are RESOLVED in §11; the 42.2b hedging pins (esp. D-H-RACE — the concurrent collector) are the SEPARATE 42.2b SPEC's obligation. The PLAN/IMPL D-questions are §12.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **Hedging — the concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` + the `initial_requests`/`additional_request_chance` fan-out** — the 42.2b sub-leg (ADR-0251). 42.2a is STRICTLY sequential: a per-try-timeout ABANDONS the attempt and the 42.1 loop re-attempts; NO concurrent attempt, NO leave-in-flight, NO collector. The `RouteAction.hedge_policy` (field 27) parse is 42.2b's (a per_try_timeout config needs no `hedge_policy`).
- **The global `RouteAction.timeout` (field 8, the overall route deadline)** — NOT implemented in envoy-go (AMEND-PT3); 42.2a does not add it. `per_try_timeout` stands alone. A future row.
- **`per_try_idle_timeout` (RetryPolicy field 13)** — a per-attempt IDLE deadline (distinct from the per-attempt wall-clock `per_try_timeout`); parse-accept-but-defer.
- **The global `upstream_rq_timeout` counter** — distinct from `upstream_rq_per_try_timeout`; deferred with the global route timeout (AMEND-PT7).
- **`request_mirror_policies` (RouteAction field 30, shadow traffic)** — a future row.
- **The `RX`/`URX`/hedge access-log response flags** — blocked on the absent response-flags surface (the 42.1 AMEND-RT7 / phase-41 CB4 precedent); a recorded departure. The differential asserts STATS + the final status, never the access-log line.
- **Per-try-timeout on non-HTTP (TCP/network) upstreams.**

---

## 3. The per-attempt deadline architecture (ADR-0250)

### 3.0 Split disposition — the FIRST sub-leg (42.2a) of the 42.2a/42.2b sub-split

42.2a = the per-attempt child ctx + the discriminator + the synthesized-504 + the `reset`-split classifier + the `upstream_rq_per_try_timeout` counter + the parse + `0076`. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~120–200 prod LoC / ~8–11 tasks — well under `> ~25 tasks OR > ~1500 LoC`; 42.2a stands alone as a usable per-attempt-timeout feature). 42.2b (hedging) is the pre-authorized second sub-leg; row 42 flips `done` only when all three legs land.

### 3.1 The per-attempt child context (in the 42.1 retry executor)

`RetryPolicy` (`internal/filter/http/router/retry.go`) gains a `perTryTimeout time.Duration` field (parsed from `RetryPolicy.per_try_timeout`; `≤0` ⇒ no bound). In BOTH `retryExecutorH1` and `retryExecutorH2`, each loop iteration wraps the single driver call:

```
attemptCtx := ctx
var cancel context.CancelFunc
if rp.perTryTimeout > 0 {
    attemptCtx, cancel = context.WithTimeout(ctx, rp.perTryTimeout)
}
resp, ep, err = do{H1,H2}ClusterAction(attemptCtx, a, req)   // driver already honors ctx deadlines
if cancel != nil { cancel() }                                 // no timer leak
// per-try-timeout discrimination (AMEND-PT4):
if rp.perTryTimeout > 0 && ctx.Err() == nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
    a.cluster.IncUpstreamRqPerTryTimeout()
    resp = ActionResponse{Status: 504, Headers: localReplyHeaders(0), Body: nil}  // synthesized 504 (H2 uses h2LocalReplyHeaders())
    err = nil
    if !rp.perTryTimeoutRetriable() { return resp, ep, err }   // not retriable → caller (final 504)
    // else fall through to the 42.1 retriable path (num_retries / retry_budget cap → re-attempt)
} else if !rp.matches(resp.Status, resp.localOrigin) {
    ... // existing 42.1 non-retriable return (incl. the H2 Status:0 client-cancel)
}
```

The driver code is UNCHANGED. The discrimination + the 504-synthesis + the per-try-timeout retriable test live in the executor. The exact loop-structure (where the per-try-timeout test slots relative to the 42.1 `matches`/`num_retries`/`tryAcquireRetry` ledger — and whether a timed-out attempt counts toward `num_retries` like any other retriable outcome) is a PLAN pin (D-S422A-1); the reference treats it as an ordinary retriable outcome (the `num_retries` cap bounds it — confirmed: `retry_on:5xx num_retries:2` → exactly 3 timed-out attempts).

### 3.2 The per-try-timeout-vs-client-cancel discriminator (AMEND-PT4 — the load-bearing distinction)

A per-attempt-deadline expiry fires the CHILD `attemptCtx`; a client cancel/downstream-deadline fires the PARENT `ctx`. The executor tells them apart by checking, after the driver returns, whether the PARENT is still alive (`ctx.Err() == nil`) while the CHILD's error is `context.DeadlineExceeded`. This is critical for H2: the driver's `Status:0` + `*h2.Error` CANCEL sentinel (`router_h2.go:111`) is emitted for BOTH a child per-try-timeout AND a parent client-cancel — the executor must NOT let a per-try-timeout reach the downstream as a RST_STREAM(CANCEL) (it must re-attempt or synthesize a 504), while a genuine parent client-cancel must STILL return `Status:0` un-retried (the 42.1 `matches(0,false)==false` path — `TestRetryExecutorH2_CtxCancelNotRetried` must keep passing). For H1, the per-try deadline breaks the held `http.ReadResponse` into the existing 502-`localOrigin` return, which the executor likewise overrides to a 504 when the child deadline is the cause.

### 3.3 The retriable classification: splitting `reset` from `connect-failure` (AMEND-PT1)

The 42.1 `parseRetryOn` fuses `connect-failure` and `reset` into a single `retryConnectFail` bit (both retry the `localOrigin` dial/reset failure). 42.2a SPLITS them: `connect-failure` → `retryConnectFail`; `reset` → a NEW `retryReset` bit. The 42.1 `matches` `localOrigin` arm becomes `rp.on&(retryConnectFail|retryReset) != 0 && localOrigin` — PRESERVING 42.1 (a genuine dial-refusal still retries under either token, confirmed faithful by D-H-RETRY). A new predicate classifies the per-try-timeout:

```
func (rp *RetryPolicy) perTryTimeoutRetriable() bool {
    // 504-status tokens (5xx, gateway-error, retriable-status-codes∋504) OR `reset`;
    // NOT connect-failure-alone (a per-try-timeout is a reset, not a connect failure).
    return rp.matches(504, false) || rp.on&retryReset != 0
}
```

`matches(504, false)` covers `5xx` (504∈[500,599]), `gateway-error` (504∈{502,503,504}), and `retriable-status-codes` (if 504 ∈ `retriable_status_codes[]`). The `|| retryReset` covers `retry_on:"reset"`. `connect-failure`-alone yields neither ⇒ not retriable (faithful). This is the ONLY surgery to the 42.1 classifier; the `retryConnectFail` semantics for dial-failures are unchanged. `retryReset` is the 5th `retryOnBits` value — the existing `uint8` (4 bits used) has ample room, NO type widening (a PLAN note).

### 3.4 Byte-stability

When `perTryTimeout == 0` (unset or `0s`), the executor derives no child ctx and runs exactly the 42.1 loop ⇒ byte-identical (every existing fixture, incl. `0075`, stays green; the full 77-dir byte-stability gate must hold). The `reset`-split is behavior-preserving for dial-failures (D-H-RETRY confirms both tokens retry a dial-refusal). The `upstream_rq_per_try_timeout` counter registers on retry clusters (emit-0 where no `per_try_timeout`) — present on both subject and reference (always-on), so no named-assertion breakage.

---

## 4. Framework primitives — the child-ctx deadline + the discriminator over the 42.1 substrate + 0 new packages + 0 new go.mod deps

- NEW: `RetryPolicy.perTryTimeout` + the `retryReset` bit + the `perTryTimeoutRetriable()` predicate + the per-attempt `context.WithTimeout` + the per-try-timeout discrimination/504-synthesis in `retryExecutorH1`/`retryExecutorH2` (`internal/filter/http/router/retry.go`); the `per_try_timeout` parse in `buildRouterActionWithVH` (`internal/filter/hcm/config.go`) threaded onto `RetryPolicy` (via `NewRetryPolicy` or a setter); the `upstream_rq_per_try_timeout` registration (the 6th `retryStats` member) + `IncUpstreamRqPerTryTimeout()` in `internal/cluster/cluster.go`.
- REUSED: the 42.1 `retryExecutorH1`/`retryExecutorH2` loop + the `matches`/`backoff`/`NewRetryPolicy` (ADR-0249); the single-attempt drivers `do{H1,H2}ClusterAction` + their ALREADY-ctx-deadline-honoring socket paths (`router.go:659-661`, `router_h2.go:111`); the buffered `ActionResponse` + `localReplyHeaders`/`h2LocalReplyHeaders` (the 504 synthesis source); `Cluster.EnsureRetryStats()` scoping + the per-attempt `TryAcquireRequest`/`RecordUpstreamResult`/`TryAcquireRetry` seams (ADR-0248/0245/0249); the held-backend `/__release` differential gate (`BlockingHoldResponder` 36); the `internal/admin` `/stats` endpoint + the `reference_docker_probe_bridge_network` differential pattern.
- ZERO new Go packages. ZERO new go.mod modules (`RetryPolicy.per_try_timeout` is `*durationpb.Duration` in the existing go-control-plane v1.37.0 dep; `go mod tidy -diff` EMPTY — §11 D-H-PROTO, confirmed).

---

## 5. Proto-field roster (per §11 D-H-PROTO)

`RetryPolicy.per_try_timeout` = `route.v3.RetryPolicy` field **3** (`*durationpb.Duration`); the 42.1-confirmed neighbors: `per_try_idle_timeout` field 13 (DEFER), `retry_on` field 1, `num_retries` field 2, `retriable_status_codes` field 7, `retry_back_off` field 8. The hedging carriers (42.2b): `RouteAction.hedge_policy` field 27, `VirtualHost.hedge_policy` field 17, `route.v3.HedgePolicy{initial_requests=1 [UInt32Value, PGV gte:1, #not-implemented-hide:], additional_request_chance=2 [FractionalPercent, #not-implemented-hide:], hedge_on_per_try_timeout=3 [bool]}`.

`per_try_timeout` PGV: NONE specific — only the GENERIC protobuf duration-positivity check (a NEGATIVE duration is rejected at config-load with `Invalid duration: Expected positive duration`; AMEND-PT5). `0s` is valid (no per-attempt bound). The global `RouteAction.timeout` = field 8 (NOT implemented in envoy-go — AMEND-PT3). `go mod tidy -diff` EMPTY → ZERO new module (route.v3 already imported).

---

## 6. PARSE-REJECT roster (per §11 D-H-PROTO + ADR-0080)

A THIN surface — one optional arm. House wording `route: %q: retry_policy: <reason>`:
- `per_try_timeout` NEGATIVE (`< 0s`) — the reference rejects it (generic duration positivity, AMEND-PT5). envoy-go MIRRORS with a thin reject (`per_try_timeout must not be negative`) OR clamps `≤0` to no-bound — a PLAN call (D-S422A-2). `per_try_timeout: 0s` / unset ⇒ NO per-attempt bound (NOT a reject — a valid config).

NOT rejected: a positive `per_try_timeout` of any magnitude (no upper bound); the deferred `per_try_idle_timeout`. The reject arm (if added) is unit-level (no boot-reject dir — the 41/42.1 precedent). NO new fuzzer (AMEND-PT5).

---

## 7. Stat surface — +1 new counter (`upstream_rq_per_try_timeout`), folded into `EnsureRetryStats` ⇒ +1 per retry cluster (per §11 D-H-STATS + AMEND-PT2)

The +1 NEW cluster counter `cluster.<n>.upstream_rq_per_try_timeout` — a counter, ++ once per ATTEMPT that hits its `per_try_timeout` (per-attempt, NOT per-request — confirmed delta `+3` for a 3-attempt request). Registered as the SIXTH member of `retryStats` via `Cluster.EnsureRetryStats()` (the 42.1 scoping) — so it lands on EVERY retry-policy cluster (emit-0 where no `per_try_timeout`), keeping the registration model uniform with the existing five retry counters.

Surface consequence (AMEND-PT2): `0075`'s TWO retry clusters each gain `+1` (=+2); the new `0076` retry cluster registers the full SIX-counter block (=+6). Anticipated **1173 → ~1181** (+8) — FIRM once `0076`'s topology is pinned at the PLAN (single held host = one retry cluster). The global `upstream_rq_timeout` is NOT registered (deferred — AMEND-PT7).

`upstream_rq_504` / `http.<prefix>.downstream_rq_5xx` already exist (no new stat — a per-try-timeout exhaustion's final 504 increments the existing status-class counters). `upstream_rq_total` counts every attempt (already LIVE).

---

## 8. Differential fixture taxonomy (+1: `0076` cross-side per-try-timeout — held-host exhaustion-exact)

### 8.1 `0076-per-try-timeout` (cross-side; NO new BackendKind)

An HTTP listener on BOTH the subject and the reference (`contrib-v1.37.2`), with ONE retry cluster (REUSING `BlockingHoldResponder` 36):

**EXHAUSTION arm (cross-side EXACT — the deterministic core).** Cluster `c_ptt` = {1 `BlockingHoldResponder` host}, route `/ptt` with `retry_policy{retry_on:"5xx", num_retries:N, per_try_timeout:T}` (T small — e.g. 200–500ms — but generous enough to fire reliably while the backend holds; a PLAN/IMPL pin). Drive 1 request: the held host accepts but never responds within T, so EVERY attempt (1 + N) times out at T; the request exhausts to a final **504**. Assert on BOTH sides: `cluster.c_ptt.upstream_rq_per_try_timeout == N+1`, `upstream_rq_retry == N`, `upstream_rq_retry_limit_exceeded == 1`, `upstream_rq_retry_success == 0`, `upstream_rq_total == N+1`, final downstream status **504**. Fully deterministic (single held host; offset-irrelevant — no `reference_round_robin_offset_randomized` exposure). After the assertion, `/__release` the held attempts (drain the responder goroutines). Decode-ran guard: `ref[upstream_rq_total] > 0`.

The held backend serves BOTH sides SEQUENTIALLY-PER-SIDE (subject fully — drive, assert, release, drain — THEN reference), per `reference_concurrency_differential_release_barrier` (the shared in-process backend idle between sides). SLEEPLESS in the budget-guessing sense: the per-try-timeout firing is DETERMINISTIC (held > T), and the test waits only the FEATURE's own `≈(N+1)×T` completion time — NOT a `time.Sleep`-margin assertion (AMEND-PT6; `reference_differential_band_sigma_margin`). The `-count=1` + `TestDifferential/0076` selector discipline (`reference_differential_break_protocol_count1` + `reference_differential_run_selector`). 2 deliberate breaks: (A) `per_try_timeout` not threaded (no child ctx) ⇒ the attempt blocks indefinitely on the held backend ⇒ the test hangs/fails (no 504); (B) the `upstream_rq_per_try_timeout` counter not wired ⇒ the cross-side delta assert fails. Constants (N / T / cluster topology / `refContainerListenerPort`) single-sourced (`reference_fixture_workload_constant_desync`). Fixtures **77 → 78**.

### 8.2 The per-try-timeout-vs-client-cancel discriminator + the `reset`-split — UNIT-tested

The H2 `Status:0` client-cancel-vs-per-try-timeout discrimination (AMEND-PT4) + the `reset`/`connect-failure` split (AMEND-PT1) + the `perTryTimeoutRetriable()` predicate are UNIT-tested in the router package (deterministic, no Docker): a per-try-timeout under each `retry_on` token ({5xx, gateway-error, reset} retry; {connect-failure, none} do not), the synthesized-504, the H2 `Status:0` parent-client-cancel-still-not-retried invariant (the 42.1 `TestRetryExecutorH2_CtxCancelNotRetried` parity), and the `≤0` no-bound byte-stability. The cross-side `0076` is the per-try-timeout integration anchor; the token-classification matrix is unit-level (the reference's per-token behavior is the §11 pin).

### 8.3 New fuzzer: NONE

`per_try_timeout` is config-parse (no new wire decoder); the parse/reject is unit-tested (the 41/42.1 precedent). Fuzzers STAY **42** (AMEND-PT5).

---

## 9. Behavior-contract delta (the 42.2a bundle; ADR-0052 atomic landing)

Extend the `### Route — request recovery (retries)` subsection in BEHAVIOR_CONTRACT.md (or a `#### Per-attempt timeout` sub-block): the `per_try_timeout` per-attempt child-`context` deadline (the project's FIRST request-scoped deadline; envoy-go does NOT implement the global `RouteAction.timeout`); a per-attempt-deadline expiry ⇒ a synthesized **504** + `upstream_rq_per_try_timeout`++ (per attempt) + (where `retry_on` matches the 504-or-`reset` class) a 42.1 sequential re-attempt under the same caps; the per-try-timeout-vs-client-cancel discriminator (the child `attemptCtx` vs the parent — the H2 `Status:0` client-cancel stays un-retried); the `reset`-vs-`connect-failure` split (a per-try-timeout retries under `reset`/5xx/gateway-error/retriable-status-codes∋504, NOT `connect-failure`-alone; a dial-refusal still retries under both); `per_try_timeout ≤ 0s` ⇒ no bound (byte-identical); the `upstream_rq_per_try_timeout` counter scoped via `EnsureRetryStats` (surface 1173 → ~1181); the deferred global `upstream_rq_timeout`/`RouteAction.timeout` departures. The stat-surface block advances 1173 → ~1181 (PLAN-firmed).

---

## 10. Per-task structure (~8–11 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) the `retryReset` bit split in `parseRetryOn` + the `matches` `localOrigin` arm update + the `perTryTimeoutRetriable()` predicate + unit tests (the token matrix; dial-refusal-under-both preserved); (3) the `RetryPolicy.perTryTimeout` field + the `per_try_timeout` parse in `buildRouterActionWithVH` + the `≤0` no-bound + the negative-reject arm (D-S422A-2) + unit tests; (4) the per-attempt `context.WithTimeout` + the per-try-timeout discriminator + the synthesized-504 in `retryExecutorH1` + unit tests (incl. the `≤0` byte-stability); (5) the same in `retryExecutorH2` + the H2 `Status:0` client-cancel-still-not-retried invariant + unit tests; (6) the `upstream_rq_per_try_timeout` registration (6th `retryStats` member) + `IncUpstreamRqPerTryTimeout()` + the surface re-count (1173 → ~1181) + the full-77-dir byte-stability gate; (7) the `0076` cross-side fixture (held-host exhaustion); (8) `0076` deliberate breaks (2) + 20-run flake; (9) full 78-dir differential + six-gate; (10) ADR-0250 body + BEHAVIOR_CONTRACT; (11) completion bundle + ROADMAP row 42 SPEC→note (row STAYS `in-progress` — 42.2b remains). The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO further split of 42.2a).

---

## 11. SPEC-time empirical-pin block (D-H-* — executed IN-SESSION 2026-06-21)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge network `ptprobe`; a STRICT_DNS `mccutchen/go-httpbin` backend with `/delay/N` [slow] + `/get` [fast] + a dead `:9999` host; a gateway Envoy with per-try-timeout routes varying `retry_on`; single-shot probes with `/stats` deltas; `--mode validate` reject probes; `--concurrency 1`; request path verified `upstream_rq_total>0`) + the go-control-plane v1.37.0 module cache.

| Pin | Disposition |
|-----|-------------|
| **D-H-PROTO** | CONFIRMED. `RetryPolicy.per_try_timeout=3` (Duration), `per_try_idle_timeout=13`; `RouteAction.hedge_policy=27` / `timeout=8`; `VirtualHost.hedge_policy=17`; `HedgePolicy{initial_requests=1 [gte:1, #not-implemented-hide:], additional_request_chance=2 [#not-implemented-hide:], hedge_on_per_try_timeout=3}`. `per_try_timeout` has NO specific PGV; a NEGATIVE duration is rejected by the generic protobuf positivity check (`Invalid duration: Expected positive duration`). `0s`/unset ⇒ no bound. `go mod tidy -diff` EMPTY → ZERO new module. §5/AMEND-PT5. |
| **D-H-TIMEOUT** | PINNED. envoy-go does NOT implement `RouteAction.timeout` (field 8 — code-confirmed: zero consumption in hcm/router). The reference DOES (default 15s; a `per_try_timeout`-less route with `timeout:6s` over a 3s backend → 200 at 3s). So `per_try_timeout` is envoy-go's FIRST request deadline; NO `min(per_try, global)` clamp (no global to clamp); the global route timeout stays DEFERRED. `per_try_timeout:0s` ⇒ no per-attempt bound (the request reached the backend, no instant fire). §AMEND-PT3. |
| **D-H-RETRY** | PINNED (the load-bearing finding). A per-try-timeout ⇒ a synthesized **504**; retried iff `retry_on` ∈ {5xx, gateway-error, retriable-status-codes∋504, **reset**} — NOT `connect-failure`-alone, NOT empty (single-slow-host probe, `per_try_timeout:0.5s`, `num_retries:2`: 5xx/gateway-error/reset → 3 attempts; connect-failure/none → 1 attempt). A genuine dial-refusal retries under BOTH `reset` AND `connect-failure` (dead-backend probe: delta=2 each, 503). ⇒ split `reset` from `connect-failure`; the per-try-timeout predicate = `matches(504,false) || retryReset`. The child-`attemptCtx`-vs-parent-`ctx` discriminator coexists with the 42.1 H2 `Status:0` client-cancel-not-retried path. §3/AMEND-PT1/PT4. |
| **D-H-STATS** | CONFIRMED. The counter is `cluster.<n>.upstream_rq_per_try_timeout` (++ per timed-out ATTEMPT — delta +3 for 3 attempts, +1 for 1). DISTINCT from the global `upstream_rq_timeout` (stayed 0 — tracks `RouteAction.timeout`, deferred). Final downstream status 504 (`downstream_rq_5xx`++ per request; `upstream_rq_504`++ per request). 42.2a folds `upstream_rq_per_try_timeout` into `EnsureRetryStats` ⇒ +1 per retry cluster; surface 1173 → ~1181. §7/AMEND-PT2/PT7. |
| **D-H-DIFFERENTIAL** | PINNED. A SINGLE held `BlockingHoldResponder` host (held > T) ⇒ deterministic exhaustion to a final 504 with cross-side-EXACT counts (`upstream_rq_per_try_timeout==N+1`, `upstream_rq_retry==N`, `upstream_rq_retry_limit_exceeded==1`, `upstream_rq_total==N+1`). Offset-irrelevant (single host). The T delay is feature-inherent, not a flake-sleep; `/__release` drains at the end. `0076-per-try-timeout`; NO new BackendKind (reuse `BlockingHoldResponder` 36). 2 deliberate breaks. §8/AMEND-PT6. |

(The 42.2b hedging pins — D-H-RACE [the concurrent collector], the `hedge_on_per_try_timeout` leave-in-flight behavior, the fan-out — are the SEPARATE 42.2b SPEC's obligation, a later sub-leg.)

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S422A-1** the exact executor loop-placement of the per-try-timeout test relative to the 42.1 `matches`/`num_retries`/`tryAcquireRetry` ledger (does a timed-out attempt count toward `num_retries` like any retriable outcome — the reference says YES; the increment ordering vs `upstream_rq_retry`/`retry_backoff_exponential`).
- **D-S422A-2** the negative-`per_try_timeout` disposition — a thin byte-stable reject (`route: %q: retry_policy: per_try_timeout must not be negative`) vs clamp-`≤0`-to-no-bound (default: REJECT, mirroring the reference; or clamp if simpler — the PLAN decides).
- **D-S422A-3** the 504-synthesis header set — REUSE `localReplyHeaders(0)` (H1) / `h2LocalReplyHeaders()` (H2)? whether the body is empty (the reference's 504 body) — a parity detail (the differential asserts status + stats, not the 504 body bytes).
- **D-S422A-4** whether `per_try_timeout` is threaded onto `RetryPolicy` via a widened `NewRetryPolicy` signature vs a post-construction setter (signature-churn vs a setter — the 42.1 `NewRetryPolicy` is exported + test-consumed).
- **D-S422A-5** `0076` constants (N / T / cluster topology / `refContainerListenerPort` next-free after `0075`'s 19164) single-sourced; the exact `T` that fires reliably while held without lengthening the suite (200–500ms candidate); whether the fixture is H1-only (the reference downstream is H1 — the cross-side anchor) with the H2 discriminator unit-tested.
- **D-S422A-6** the exact surface integer (1173 → ?) once `0076`'s retry-cluster count is fixed — the re-count predicate is now `+6 per NEW retry cluster` (the full six-counter block) and `+1 per EXISTING retry cluster` (0075's two, which gain the new sixth counter); ⇒ 1173 + 2 + 6 = ~1181 for a single-retry-cluster `0076`.
- **D-S422A-7** the ADR-0045 final split-gate re-check (anticipated NO further split — ~120–200 LoC / ~8–11 tasks).

---

## 13. ADR continuity — the ADR-0250 §Context DRAFT (anchored here; full entry lands at the 42.2a IMPL)

**ADR-0250 §Context (draft).** Phases 39–42.1 established the cluster-runtime substrate for upstream robustness: active health checking + outlier detection route AROUND unhealthy hosts (ADR-0242/0243/0245/0246/0247), circuit breaking SHEDS load (ADR-0248), and the 42.1 retry loop RECOVERS a single request by re-attempting it against an LB-re-picked host (ADR-0249) — a `retryExecutorH1`/`retryExecutorH2` wrapping the single-attempt `do{H1,H2}ClusterAction` driver, classifying the buffered `ActionResponse.Status` (+ the `localOrigin` transport signal) against a parsed `retry_on` bitset, replaying the buffered body, and re-attempting under `num_retries` + the activated `retry_budget`. The per-attempt timeout (`RetryPolicy.per_try_timeout`) is the next dimension — it bounds EACH attempt by a deadline, the project's FIRST request-scoped deadline of any kind (envoy-go does not implement the global `RouteAction.timeout`). The phase-42.2 BRAINSTORM settled a per-attempt child `context` shared by the sequential loop (42.2a) and the future concurrent hedge (42.2b); the §11 live pins (D-H-PROTO/TIMEOUT/RETRY/STATS/DIFFERENTIAL, executed in-session 2026-06-21 against `contrib-v1.37.2` per `reference_docker_probe_bridge_network`) then REFINED the design: (AMEND-PT1) a per-try-timeout produces a synthesized 504 retried under {5xx, gateway-error, retriable-status-codes∋504, reset} but NOT connect-failure-alone — so the 42.1 `connect-failure`/`reset` fusion must SPLIT (a dial-refusal still retries under both, but a per-try-timeout is a reset, not a connect failure); (AMEND-PT3) envoy-go has NO global `RouteAction.timeout`, so `per_try_timeout` stands alone with no `min(per_try, global)` clamp; (AMEND-PT4) the executor discriminates a child-`attemptCtx` per-try-timeout from a parent-`ctx` client cancel (the 42.1 H2 `Status:0` CANCEL sentinel is emitted for both — only the parent-cancel stays un-retried); (AMEND-PT2/PT7) the `upstream_rq_per_try_timeout` counter folds into `EnsureRetryStats` (+1 per retry cluster; surface 1173 → ~1181), distinct from the deferred global `upstream_rq_timeout`. The design: a per-attempt `context.WithTimeout(reqCtx, perTryTimeout)` around each driver call in the 42.1 executor; a post-driver per-try-timeout discriminator (`parentCtx.Err()==nil && attemptCtx.Err()==DeadlineExceeded`); a synthesized 504 + `IncUpstreamRqPerTryTimeout()`; a `perTryTimeoutRetriable()` predicate (`matches(504,false) || retryReset`); byte-identical when `per_try_timeout ≤ 0`. NO concurrency (42.2b layers the concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` + the fan-out on top — ADR-0251). §Decision + §Consequences land at the 42.2a IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1173** / fixtures **77** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0249** (next-free **ADR-0250**). ROADMAP row 42 STAYS `in-progress` (the SPEC note appended). Anticipated at the 42.2a IMPL: fixtures 77 → **78** (`0076-per-try-timeout`), BackendKind tail **36** UNCHANGED (REUSE `BlockingHoldResponder`), DECISIONS tail ADR-0249 → **ADR-0250** (next-free ADR-0251), stat surface 1173 → **~1181** (+1 `upstream_rq_per_try_timeout` per retry cluster — PLAN-firmed), fuzzers **42** (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Row 42 flips `done` only when ALL THREE legs (42.1 + 42.2a + 42.2b) land. Next → the phase-42.2a PLAN (`superpowers:writing-plans`), then the 42.2a IMPL; then the **42.2b (hedging) BRAINSTORM** (the concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` + the fan-out over the 42.2a deadline — ADR-0251).
