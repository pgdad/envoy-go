# Fixture 0032 — `envoy.filters.http.ratelimit` global rate-limit differential

Phase 24.1 scope per parent SPEC §7.1 + §7.3. Six scenarios — five
cross-side byte-exact (b/c/d-core/e and a structural via cross-side
status-line byte-stream comparison), one subject-only via
`StatsAsserter.AssertStats` (h). 24.2 ADDS scenarios (f) `vh_inclusion`
and (g) `x_ratelimit_headers` to this same directory + extends (d) with
the remaining five descriptor actions (per the 24.1/24.2 split per
ADR-0201).

## Fixture type

CROSS-SIDE (default `RequiresReference()=true`): the runner spawns
reference Envoy v1.37.2 + envoy-go, drives both via
`DriveReference`/`DriveSubject`, and `CompareBytes` is the differential
gate. The cross-side byte-exact guarantee relies on the shared in-process
fake `RateLimitService` (`test/helpers/ratelimitgrpc/`) being dialed by
both sides at the SAME `127.0.0.1:<port>` (allocated once per fixture run
+ shared via `host.docker.internal` (reference, dockerized) / `127.0.0.1`
(subject, host) templating per the `0021` `extauthzgrpc.NewAtAddr`
precedent). Per AMEND-6 the fake emits `RateLimitResponse` by proto field
NUMBER + omits unset optionals — load-bearing for the (c) `over_limit_429`
cross-side byte-exact wire-comparison.

## Single-listener topology (per parent SPEC §7.3)

ONE listener (`l_test_a`) carrying ONE HCM with the ratelimit filter (in
alphabetical position, with the router terminator) + the RLS cluster
(`c_ratelimit`) pointing at the fake gRPC server + a synthetic always-200
echobackend cluster (`c_backend`). No multi-listener topology — avoids
the `freeTCPPort` combined-run flake per 22.2 REVIEW §7.4 (the 24.1 PLAN
cites this exact constraint).

Scenarios that need DIFFERENT filter-level config (failure_mode_deny
toggle for fail-open vs fail-closed paths) are NOT exercised at 24.1 —
scenario (e) `failure_mode_open` uses the DEFAULT `failure_mode_deny:false`
(the AMEND-3 proto-zero default) so a single listener config suffices.
Per-route discrimination across scenarios (b)/(c)/(d) lives at the
route-table level via the `rate_limits[]` policy entries Task 5 plumbed
through (ADR-0198 DELTA-2). Each scenario path carries its own
descriptor-producing policy:

| Scenario | Path | Descriptors | Fake script (CanonicalKey) |
|---|---|---|---|
| (a) `parse_ok` | `/scenario_a` | none (route has NO `rate_limits`) | n/a (no RLS call fires) |
| (b) `ok_admit` | `/scenario_b` | `generic_key{key=scenario,value=b}` | `domain_b \| scenario=b` → OK |
| (c) `over_limit_429` | `/scenario_c` | `generic_key{key=scenario,value=c}` | `domain_b \| scenario=c` → OVER_LIMIT |
| (d) `descriptor_actions` | `/scenario_d` | 4-action chain (see below) | per-action key (see driver) → OK |
| (e) `failure_mode_open` | `/scenario_e` | `generic_key{key=scenario,value=e}` | RLS server stopped → transport error → fail-open admit |
| (h) `stat_surface` | n/a (observational) | reuses (b)/(c)/(d)/(e) deltas | `StatsAsserter.AssertStats` asserts counter deltas AFTER (b)+(c)+(d)+(e) |

The single shared `domain` (`domain_b`) appears in all script keys per
the filter's `compiledConfig.domain` (set on `envoy.yaml`/`envoy-go.yaml`
ratelimit filter config).

## 6-scenario matrix

### (a) parse_ok — subject-only structural via cross-side byte-stream

The driver issues one `GET /scenario_a` against `l_test_a`. The matched
route carries NO `rate_limits` policy → the descriptor engine produces
zero descriptors → DecodeHeaders short-circuits to Continue → echo backend
returns 200. The driver emits `scenario a status=200 body=ok` into the
cross-side byte stream; both sides produce identical bytes ⇒
`CompareBytes` passes. This is the byte-stream-confirmed "the ratelimit
filter parses, loads, and the zero-descriptor path is byte-equivalent
to reference Envoy" structural assertion.

### (b) ok_admit — cross-side byte-exact

One `GET /scenario_b` against `l_test_a`. The route carries a single
`generic_key{descriptor_key:scenario, descriptor_value:b}` policy → the
descriptor engine emits `[{scenario:b}]` → RLS fake matches
`CanonicalKey="domain_b|scenario=b"` → returns OK → filter Continue → echo
200. Cross-side `CompareBytes` on the per-scenario status line confirms
byte-equivalence; the subject's `cluster.c_ratelimit.ratelimit.ok`
counter increments (verified in scenario h's observational AssertStats
read of the accumulated deltas).

### (c) over_limit_429 — cross-side byte-exact

One `GET /scenario_c` against `l_test_a`. The route carries
`generic_key{scenario:c}` → fake matches `domain_b|scenario=c` → returns
OVER_LIMIT (proto-number-faithful; no `raw_body`, no `quota`, no
per-descriptor optionals — AMEND-6 / D-RL5) → filter emits the §4.7
byte-shape:
- status: 429 (the default `rateLimitedStatus`)
- header: `x-envoy-ratelimited: true` (AMEND-8 order [a])
- body: empty (no `RawBody` on the scripted response)

Cross-side `CompareBytes` on the status line + body classification asserts
byte-equivalence. The subject's `over_limit` counter increments.

### (d) descriptor_actions — cross-side byte-exact (4 core actions)

One `GET /scenario_d` against `l_test_a` with three discriminating
client-supplied request bits:
- `x-tenant: tenant-x` (read by the `request_headers` action)
- `x-canary: true` (read by the `header_value_match` action's headers
  matcher — `present_match:true` on `x-canary`)
- the loopback peer address (consumed by the `remote_address` action)

The route's `rate_limits[]` carries ONE policy with FOUR actions in this
order:
1. `generic_key{scenario:d}` → `scenario=d`
2. `request_headers{header_name:x-tenant, descriptor_key:tenant}` →
   `tenant=tenant-x`
3. `remote_address{}` → `remote_address=127.0.0.1`
4. `header_value_match{descriptor_value:canaried,
   headers:[{name:x-canary, present_match:true}]}` → `header_match=canaried`

The descriptor engine emits ONE descriptor with FOUR entries (AMEND-6
action-list order). The fake script keyed
`domain_b|scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried`
returns OK ⇒ filter Continue ⇒ echo 200. Cross-side `CompareBytes` on
the status line asserts byte-equivalence.

**NO `destination_cluster` action.** Per Task 7 framework-limitation note
(decode_headers.go file-header step 2) the DecoderFilterCallbacks surface
has NO `MatchedClusterName()` accessor at master tip → the
`destination_cluster` action's whole-descriptor-drop arm at
`descriptors.go::actionDestinationCluster` ALWAYS fires under empty
cluster-name input → a scenario exercising it would silently drop the
whole descriptor (per §4.5 behavior 1) ⇒ the RLS call would NOT fire ⇒
the fake's "default OK" arm would kick in, masking the assertion.
24.1 fixture 0032 therefore restricts scenario (d) to the FOUR
framework-reachable core actions per the PLAN Task 10 brief.

### (e) failure_mode_open — cross-side byte-exact

The driver STOPS the RLS fake before issuing `GET /scenario_e` so the
gRPC dial fails fast (loopback connection-refused). The route carries
`generic_key{scenario:e}` → engine emits one descriptor → filter dispatches
ShouldRateLimit → transport error → `applyError` arm with
`failure_mode_deny:false` (the AMEND-3 proto-zero default; default on
this fixture's filter config) → admit-on-error (fail-open) → echo 200.

Both sides see the same transport-error → fail-open admit → echo 200 (the
echo backend stays up; only the RLS fake is stopped). Cross-side
`CompareBytes` asserts byte-equivalence. The subject's `error` AND
`failure_mode_allowed` counters increment (per the dispositions
`applyError` fail-open arm). The driver does NOT restart the fake after
scenario (e) — scenario (h) is OBSERVATIONAL (no additional probes), so
the fake stays stopped through teardown.

### (h) stat_surface — subject-only via `StatsAsserter.AssertStats`

OBSERVATIONAL — no additional probe burst; (h) asserts the counter
deltas accumulated by the preceding b/c/d/e probes. `StatsAsserter.AssertStats`
scrapes the SUBJECT admin `/stats/prometheus` and asserts the four
cluster-scoped counters at the deterministic deltas:

| Counter (Prometheus form) | Expected value | Source |
|---|---|---|
| `envoy_cluster_ratelimit_ok{envoy_cluster_name="c_ratelimit"}` | 2 | b admit + d admit |
| `envoy_cluster_ratelimit_over_limit{envoy_cluster_name="c_ratelimit"}` | 1 | c |
| `envoy_cluster_ratelimit_error{envoy_cluster_name="c_ratelimit"}` | 1 | e (RLS unreachable) |
| `envoy_cluster_ratelimit_failure_mode_allowed{envoy_cluster_name="c_ratelimit"}` | 1 | e (failure_mode_deny:false) |

The observational shape (vs a separate burst phase) is INTENTIONAL — the
shared in-process fake stops cleanly but the subject's gRPC sub-channel
manages reconnect state per ADR-0158, and a stop-then-restart-on-the-same-port
within a single fixture run is NOT a pattern the existing fake-toggle
fixtures exercise (0021 puts the STOPPED state at the END of the live
batch; never a mid-stream restart). Observational (h) avoids the
reconnect edge AND keeps the per-scenario counter deltas pinned 1:1 to
scenarios (b)/(c)/(d)/(e) — no burst-replay accounting drift.

Per `reference_differential_asserter_dispatch` (the runner's `runFixture`
dispatch only invokes `SubjectAsserter` on the reference-less path; this
fixture is cross-side) the subject-side counter assertions live in
`StatsAsserter.AssertStats`, NOT `SubjectAsserter`. AssertStats is
PROVEN LIVE via the deliberate-break recipe documented in the Task 10
PROGRESS entry: temporarily flip the expected `ok` value to a wrong
number, re-run, observe FAIL, then revert and observe GREEN.

The `cluster.<rls>.ratelimit.*` cross-namespace stat surface is THIS
filter's novelty per AMEND-1 + AMEND-10 (first cross-namespace cluster-
stat charge in the codebase per ADR-0197). Reference Envoy v1.37.2 also
publishes the same names + values; (h) asserts the SUBJECT counters
(scope: 24.1) — the cross-side equivalence on these counters is
ratified at 24.2 if needed (24.1's primary cross-side gate is the
byte-stream `CompareBytes` on b/c/d/e).

## Scripting discipline (per Task 9 advisory)

The shared fake's `ShouldRateLimit` returns a default OK on no-match
(unlike `extauthzgrpc` which returns `codes.Unavailable`). To avoid
silent test passes from forgotten scripts every expected canonical key
is EXPLICITLY scripted in `driver.go::setupRLS` (which starts the fake +
pre-populates every per-scenario script). The driver also
logs the scripted-key set under `FIXTURE_0032_DUMP_BYTES=1` for
debug visibility.

## Reference container ports

| Listener / endpoint | In-container port |
|---|---|
| `l_test_a` | 10032 |
| admin | 9901 |

(`c_ratelimit` upstream cluster endpoint port is runner-allocated +
shared per the `0021` pre-allocated-port + `host.docker.internal`
templating pattern.)

## Cross-references

- parent SPEC §7.1 (8-scenario matrix; 24.1 scope = a/b/c/d-core/e/h)
- parent SPEC §7.3 (single-listener topology)
- 24.1 SPEC §7 (24.1 scenario scoping)
- AMEND-1 (cluster-scoped stat surface)
- AMEND-3 (filter-config 13-field roster + defaults/clamps)
- AMEND-6 (proto-number-faithful fake encoding — load-bearing for byte-exact)
- AMEND-8 (OVER_LIMIT header order)
- AMEND-10 (cross-namespace cluster-stat charging)
- AMEND-11 (per-action key defaults)
- ADR-0010 (`host.docker.internal` reference-container loopback alias)
- ADR-0166 (H2-without-TLS upstream — plaintext h2c rls cluster)
- ADR-0197 (filter shape + dispositions + cross-namespace stat charge — CORE slice at 24.1)
- ADR-0198 (DELTA-2 route-table accessor pair — Task 5)
- ADR-0200 (route-level PARSE-REJECTs — Task 3)
- ADR-0201 (24/24.1/24.2 split)
- `reference_differential_asserter_dispatch` (subject assertions in `StatsAsserter`, not `SubjectAsserter`)
- `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch; cross-side vs boot-reject are different dirs — `0033` is the sibling)
- 18.2 fixture-0021 (template precedent — fixed pre-allocated port + `host.docker.internal`/`127.0.0.1` templating + `extauthzgrpc.NewAtAddr` analogue)
- 23 fixture-0030 (StatsAsserter precedent + single-bootstrap two-listener pattern; 0032 takes the single-listener path)
