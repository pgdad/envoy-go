# Phase 42.2 — hedging (the SECOND leg of the FOURTH Upstream-robustness-family row)

Row 42 (`retries-and-hedging`) is a **three-leg** row over the 42.1 retry substrate:

| Leg | Subject | Status | ADR |
|-----|---------|--------|-----|
| 42.1 | `retry_policy` — the sequential retry loop | **DONE** (`4a255b25`) | ADR-0249 |
| **42.2a** | **`per_try_timeout` — the per-attempt deadline** | **DONE** | **ADR-0250** |
| 42.2b | hedging — the concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` + the fan-out | pending | ADR-0251 (anticipated) |

Row 42 flips `done` only when ALL THREE legs land (ADR-0106 + `reference_roadmap_split_phase_row_done`).

## Documents

- `BRAINSTORM.md` — the hedging-leg charter: the `#not-implemented-hide:` finding (only `hedge_on_per_try_timeout` is wired in the reference; the `initial_requests`/`additional_request_chance` fan-out is no-op'd), the four settled forks, the pre-authorized 42.2a/42.2b sub-split.
- `SPEC.md` — the **42.2a** (`per_try_timeout`) charter: the §11 D-H-* live pins against `contrib-v1.37.2`, the AMEND-PT1..7 amendments, the §13 ADR-0250 §Context draft.
- `PLAN.md` — the 42.2a 11-task bite-sized TDD spine + the settled D-S422A-1..8 block.
- `PROGRESS.md` — the 42.2a IMPL execution record (the six-gate, the deliberate breaks + flake, the T6 race-fix, the exit counts).
- (42.2b gets its own `42.2b-…/` directory for its BRAINSTORM/SPEC/PLAN/PROGRESS.)

## 42.2a as-built (ADR-0250)

`RetryPolicy.per_try_timeout` (field 3) — a per-attempt deadline, the project's FIRST request-scoped deadline (envoy-go does not implement the global `RouteAction.timeout`). The 42.1 `retryExecutorH1`/`retryExecutorH2` wrap each `do{H1,H2}ClusterAction` call in a child `context.WithTimeout(ctx, perTryTimeout)`; on the child deadline expiring while the parent ctx is alive, the attempt is a per-try-timeout — a synthesized **504** (overriding the H1 502 / H2 `Status:0` sentinel), `upstream_rq_per_try_timeout`++, retried under {5xx, gateway-error, retriable-status-codes∋504, **reset**} (NOT connect-failure-alone, so the 42.1-fused `connect-failure`/`reset` bit SPLITS into `retryConnectFail` + `retryReset`). The discriminator is race-robust (a wall-clock-deadline-elapsed fallback immune to context's timer-goroutine lag, scoped to the driver's synthesized-failure manifestation, distinguishing a per-try-timeout from a client cancel — the 42.1 H2 `Status:0`-not-retried invariant preserved). Byte-identical when `per_try_timeout ≤ 0`. The cross-side `0076-per-try-timeout` fixture (a single held `BlockingHoldResponder` host → deterministic exhaustion to 504) proves it against the reference.

**Counts at 42.2a IMPL-done:** stat surface **1173 → 1181** (+1 `upstream_rq_per_try_timeout` per retry cluster) · fixtures **77 → 78** (`0076`) · fuzzers **42** · BackendKind tail **36** (REUSE `BlockingHoldResponder`) · DECISIONS tail **ADR-0249 → ADR-0250** (next-free ADR-0251) · ZERO new packages/modules. The full 78-dir differential + the six-gate GREEN. Row 42 STAYS `in-progress` (42.2b remains).

**Next:** the 42.2b (hedging) BRAINSTORM — the concurrent `hedgeExecutor` + `hedge_on_per_try_timeout` (leave-in-flight + concurrent hedge, first-acceptable-wins) + the `initial_requests`/`additional_request_chance` fan-out, over the 42.2a per-attempt deadline (ADR-0251).
