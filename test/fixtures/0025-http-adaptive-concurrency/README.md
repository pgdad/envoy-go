# Fixture 0025 — http-adaptive-concurrency

Differential subject-only structural fixture for the
`envoy.filters.http.adaptive_concurrency` HTTP filter (phase 21).
4 scenarios driving distinct controller behaviors per phase-21 SPEC
§7 + §11 §21.P1 RATIFIED + AMEND-6.

## Scope deviation from PLAN §10

PLAN §10 specified a 4-sub-directory structure
(`parse_ok/` + `overflow_503/` + `stat_surface/` +
`pass_through_when_disabled/`) AND a cross-side byte-exact promise for
scenario (b) overflow_503 per AMEND-6. After reviewing the established
fixture conventions in this repository (the phase-20 oauth2 fixture
0024 + the phase-07.1 iteration-probe fixture 0007b precedent), the
Task 10 IMPL landed a **single-directory + REFERENCE-LESS** fixture
mirroring the most-recent precedent:

- ONE `envoy.yaml` (inert skeleton for future cross-side extension) +
  ONE `envoy-go.yaml` (3 listeners, one per filter-config variant) +
  ONE `expectations.yaml` (REFERENCE-LESS structural expectations) +
  ONE `README.md` (this file) + ONE `inputs/driver.go`.
- All 4 scenarios REFERENCE-LESS — the driver implements
  `ReferenceLessFixture` per the 0007b + 0024 precedent. The runner
  short-circuits the reference-Envoy spawn + `DriveReference` +
  byte-stream `CompareBytes`; only `DriveSubject` + the in-band
  `SubjectAsserter` run.

**The AMEND-6 cross-side byte-exact promise for scenario (b) is
deferred to a future cross-side extension** — flagged as
RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION in the Task 10
PROGRESS.md entry. The envoy-go-side byte-exact pinning of the 503 +
25-byte "reached concurrency limit" body + `content-type: text/plain`
header still lands at scenario (b) per the AMEND-6 wire-shape
invariants — only the cross-side `CompareBytes` against reference
Envoy v1.32.4 is deferred. Rationale: the 4 scenarios collectively
exercise the controller-behavior axes the PLAN scoped (PARSE-OK
construction + Block-leg 503 + 7-name stat surface + AMEND-4
pass-through default), and the cross-side comparison adds testing
infrastructure complexity (reference-Envoy bootstrap synthesis +
admin-port allocation for a 3-listener differential + pre-amplified
backend response-delay coordination so both stacks observe the same
in-flight overflow trigger ordering) without changing the envoy-go-
side wire-shape coverage. The forward-pointer is well-defined: the
`envoy.yaml` skeleton at this fixture is the starting point for the
extension; the 3-listener layout mirrors `envoy-go.yaml` exactly.

## 4 scenarios

| # | Scenario | Listener | Filter config | Request | Expected outcome |
|---|---|---|---|---|---|
| (a) | parse_ok | `l_a_default` | default Gradient-1 (minConcurrency=3, maxConcurrencyLimit=1000, enabled=true) | single GET / | HTTP 200; rq_blocked counter remains 0 |
| (b) | overflow_503 | `l_b_overflow` | minConcurrency=1 + maxConcurrencyLimit=1 + concurrency_limit_exceeded_status code=ServiceUnavailable | 2 concurrent GET /slow (slow-stream backend; 5-second response) | first request → 200 OK; second request → 503 + body "reached concurrency limit" (25 bytes verbatim) + content-type: text/plain; rq_blocked counter == 1 |
| (c) | stat_surface | `l_a_default` | default Gradient-1 + admin /stats scrape | single GET / + scrape /stats?format=prometheus | admin /stats exposes the 7-name stat surface (rq_blocked + 6 gauges); concurrency_limit == 3 (minConcurrency default); min_rtt_calculation_active == 1 (initial window per AMEND-2 C4) |
| (d) | pass_through_when_disabled | `l_d_disabled` | `enabled` field ABSENT (filter OFF per AMEND-4 default) | single GET / | HTTP 200; no 503; rq_blocked counter remains 0; controller never consulted |

## Listener topology

Three listeners on consecutive ports (subjListenerPort, +1, +2). The
runner allocates `subjListenerPort` via `freeTCPPort(t)`; the driver's
`SubjectConfig` interpolates `+1` and `+2` directly per the phase-20
oauth2 precedent (`LCTestPort = LATestPort + 1`). The 3-listener
structure isolates the per-scenario filter-config variant so each
scenario observes a clean controller initial state.

- **`l_a_default`** — default Gradient-1 config. Exercises scenarios
  (a) parse_ok + (c) stat_surface. Scenario (a) fires first against
  this listener; scenario (c) then scrapes /stats. The single fast
  request in (a) does NOT close the minRTT sampling window (the
  window needs 50 samples to close per request_count default), so (c)
  observes `min_rtt_calculation_active = 1` per AMEND-2 C4 first-tick
  semantics.
- **`l_b_overflow`** — `min_concurrency = 1 + max_concurrency_limit = 1 +
  concurrency_limit_exceeded_status code: ServiceUnavailable`. Initial
  `concurrency_limit = minConcurrency = 1` per SPEC §4.6 +
  `enterMinRTTSamplingWindowLocked`. Exercises scenario (b)
  overflow_503 — 2 concurrent GET /slow requests cause the second to
  hit `forwardingDecision`'s Block leg (current=1 >= limit=1), which
  increments `rq_blocked` and returns the AMEND-6 byte-pinned 503 +
  25-byte "reached concurrency limit" body + `content-type:
  text/plain` per `decode_headers.go` leg-3.
- **`l_d_disabled`** — `enabled` field ABSENT (filter OFF per AMEND-4
  REFUTATION default). Exercises scenario (d) — `decode_headers.go`
  leg-1 pass-through fires; the controller is NEVER consulted.

## Cross-references

- SPEC §7.1 — 4-scenario differential matrix.
- SPEC §7.3 — REFERENCE-LESS scope deviation (single-directory
  precedent).
- SPEC §11 §21.P1 RATIFIED — 503-overflow byte-pinned wire shape.
- AMEND-2 C4 — first-tick semantics (minRTT window enters at
  construction).
- AMEND-3 — 7-name HCM-rooted stat surface (`rq_blocked` +
  `concurrency_limit` + `gradient` + `burst_queue_size` +
  `sample_rtt_msecs` + `min_rtt_msecs` + `min_rtt_calculation_active`).
- AMEND-4 — `enabled` absent ⇒ filter OFF (REFUTES BRAINSTORM §2.1).
- AMEND-6 — 503 + body + content-type wire-shape invariants (envoy-go-
  side byte-exact at scenario (b); cross-side deferred per the Task 10
  RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION note).
- ADR-0059 §Decision AMENDMENT — float-valued-gauge int64 encoding
  (nanoseconds for time-typed; ×1000 for ratio-typed; 0/1 for
  bool-typed).
- ADR-0072 — boot-time-fail-fast.
- ADR-0186 — Gradient-1 controller state machine.
- ADR-0187 — RTDS `enabled.runtime_key` PARSE-REJECT deferral.
- Phase-20 oauth2 fixture 0024 — single-directory REFERENCE-LESS
  precedent (Task 12).
- Phase-07.1 iteration-probe fixture 0007b — REFERENCE-LESS precedent
  (Task 22).
- Phase-08.2 graceful-drain fixture 0010 — HTTPSlowStream backend
  precedent (the slow-stream `/slow` 5-second response leveraged by
  scenario (b)).

## Runtime invariants observed by the driver

The driver's `DriveSubject` emits a deterministic byte stream of
per-scenario probe blocks. The `SubjectAsserter` then matches
substrings per the expectation table above. Per the phase-20 oauth2
precedent the byte stream uses Go's `%q` quoting for body content so
multi-line / binary bodies remain debuggable in `-v` test output.

The driver does NOT race the per-scenario probes — scenarios fire
sequentially (a, then b, then c, then d) so the per-listener
controller state evolves predictably between scenarios. Scenario (b)
is the only scenario that issues concurrent requests; its 2-request
race against the in-flight=1 slot is the entire point of the
overflow_503 trap.
