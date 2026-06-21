# Phase 42.2a (per_try_timeout) IMPL — PROGRESS

Execution record for the 42.2a IMPL (the per-attempt deadline over the 42.1 retry loop). PLAN: `docs/envoy-go/phases/42.2-hedging/PLAN.md`. SPEC: `docs/envoy-go/phases/42.2-hedging/SPEC.md`.

**IMPL base commit:** `85299c83` (the 42.2a PLAN, docs-only). Worktree branch `phase-42.2a-impl` off master.

## Pre-IMPL baselines (Task 1)

Six-gate baseline on the IMPL base:
- `go build ./...` — **OK**
- `go vet ./...` — **OK**
- `gofmt -l internal/ test/` — **empty** (clean)
- `go test ./internal/...` — **PASS**
- `go test ./test/differential/ -count=1` — **77-dir GREEN** (the byte-stability anchor; see Task 1 record)
- next-free `refContainerListenerPort` = **19164** (0075) ⇒ 0076 uses **19165** (confirmed; matches the PLAN D-S422A-5)

**Counts at IMPL base:** stat surface **1173** / fixtures **77** / fuzzers **42** / BackendKind tail **36** (`BlockingHoldResponder`) / DECISIONS tail **ADR-0249** (next-free **ADR-0250**).

**Anticipated exit (PLAN §Exit deltas):** stat surface 1173 → **1181** (Task 5: +2 on 0075's two retry clusters ⇒ 1175; Task 8: +6 for 0076's new `c_ptt` cluster ⇒ 1181) · fixtures 77 → **78** (`0076-per-try-timeout`) · fuzzers **42** · BackendKind tail **36** (REUSE `BlockingHoldResponder`) · DECISIONS tail ADR-0249 → **ADR-0250** (next-free ADR-0251) · ZERO new packages/modules. Row 42 STAYS `in-progress` (42.2b remains).

## Task checklist (11-task TDD spine)

- [x] T1 — baselines + PROGRESS scaffold (`cffc8356`; 77-dir baseline GREEN)
- [x] T2 — split `reset` from `connect-failure` (`retryReset` 5th bit) + `perTryTimeoutRetriable()` (`deebd09a`)
- [x] T3 — `RetryPolicy.perTryTimeout` + widened `NewRetryPolicy` (6th param) + negative reject (`f515a8bb`)
- [x] T4 — the `per_try_timeout` parse in `parseRetryPolicy` (hcm) + route-scoped reject + byte-stability gate (`903b7f73`)
- [x] T5 — the `upstream_rq_per_try_timeout` counter (6th `retryStats` member via `EnsureRetryStats`) (`682b2b0c`)
- [x] T6 — `retryExecutorH1`: per-attempt ctx + discriminator + synthesized-504 (`4d94a2a0`)
- [x] T7 — `retryExecutorH2` + the `Status:0` client-cancel-not-retried invariant (`2c64cb3d`)
- [x] T8 — the `0076-per-try-timeout` cross-side fixture (`f797b80b`)
- [x] T9 — `0076` deliberate breaks (2) + 20-run flake (verification; no commit)
- [x] T10 — full 78-dir differential + six-gate (verification; no commit)
- [x] T11 — ADR-0250 body + BEHAVIOR_CONTRACT + completion bundle (this commit)

## Per-task execution record

Executed subagent-driven (fresh implementer per task + review). Each code task: failing test → run-fail → implement → run-pass → gofmt/golangci-lint → per-task byte-stability gate (the full differential) → local commit. Subagents staged only their own files; the controller squashes + pushes at stage-close.

**T6 IMPL-discovered race fix (an as-built refinement over the SPEC §3.1 discriminator).** The SPEC's `timedOut := ptt>0 && ctx.Err()==nil && errors.Is(attemptCtx.Err(), DeadlineExceeded)` was found RACY under `-race` (reproducible 502-leak + per_try_timeout undercount): the H1 driver propagates the child deadline onto the upstream socket, so the socket-I/O-timeout-broken read can return BEFORE context's timer goroutine stamps `attemptCtx.Err()`. The as-built three-signal discriminator (parent-alive + failure-manifestation-guard [`resp.localOrigin` H1 / `resp.localOrigin || resp.Status==0` H2] + wall-clock-fallback `!time.Now().Before(attemptDeadline)`, read before the explicit `cancel()`) is the corrected form — code-reviewer-verified against every edge case. Recorded in ADR-0250 §Decision/§Consequences.

**Deliberate breaks (T9, `-count=1` per `reference_differential_break_protocol_count1`):**
- Break A — neuter the H1 `per_try_timeout` threading (`if false && rp.perTryTimeout > 0`): `TestDifferential/0076` FAILED (`-timeout 40s`) — "the retry loop should return a 504 local reply, not a transport failure" (the held read hangs without a deadline). Restored clean.
- Break B — `IncUpstreamRqPerTryTimeout` no-op: `TestDifferential/0076` FAILED — "cluster.c_ptt.upstream_rq_per_try_timeout delta = 0, want 4". Restored clean.
- 20-run flake gate: **20/20 PASS** (the per-try-timeout firing is deterministic — held > T; counts are exact).

**Six-gate (T10, ADR-0052):** `go build ./...` ✓ · `go vet ./...` ✓ · `gofmt -l internal/ test/` empty ✓ · `golangci-lint run ./...` clean ✓ · `go test ./internal/... -count=1` PASS ✓ · `go test ./test/differential/ -count=1` **78-dir GREEN** ✓ (a transient `0024-http-oauth2` `subject ready: EOF` startup-race flake — `reference_differential_fullsuite_startup_flake` — was isolate-re-run GREEN [`ok` 1.1s] + the full suite re-run GREEN [237s]; NOT a regression — phase 42.2a touches only `retry.go`/`config.go`/`cluster.go` + the `0076` fixture, the upstream path is byte-identical when `per_try_timeout ≤ 0`).

## Exit counts (verified at the six-gate)

| Axis | At IMPL base | At 42.2a IMPL-done |
|------|--------------|--------------------|
| stat surface | 1173 | **1181** (+8: +2 on 0075's two retry clusters + +6 for 0076's `c_ptt`) |
| differential fixtures | 77 | **78** (`0076-per-try-timeout`) |
| fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 36 | 36 (REUSE `BlockingHoldResponder`) |
| DECISIONS tail | ADR-0249 | **ADR-0250** (next-free ADR-0251) |
| new packages / go.mod modules | — | ZERO / ZERO |
| ROADMAP row 42 | in-progress | **in-progress** (42.2b hedging remains) |
