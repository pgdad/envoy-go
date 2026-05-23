# Fixture 0032 — `envoy.filters.http.ratelimit` global rate-limit differential

Phase 24.1 scope per parent SPEC §7.1 + §7.3 (6 scenarios: a/b/c/d-core/e/h)
EXTENDED at phase 24.2 Task 6 with 3 ADDITIONS (per ADR-0201 24/24.1/24.2
split):
- **(f) `vh_inclusion`** — 3 sub-scenarios (OVERRIDE / INCLUDE / IGNORE)
  exercising the parent SPEC §4.3 Axis-B cross-tier composition table.
- **(g) `x_ratelimit_headers`** — multi-descriptor OVER_LIMIT with per-
  descriptor `current_limit`/`limit_remaining`/`duration_until_reset` →
  cross-side byte-pin on the `x-ratelimit-{limit,remaining,reset}` triple
  per AMEND-8 + §4.7 + the Task 5 follow-up wire-order discipline.
- **(d) `descriptor_actions` extension** — the 4-action 24.1 core chain
  GROWS to a 9-action chain (the 5 REMAINING actions from Task 1 added:
  `source_cluster` / `masked_remote_address` / `metadata` /
  `query_parameters` / `query_parameter_value_match`). `destination_cluster`
  remains INTENTIONALLY OMITTED per the framework-limitation note (no
  `MatchedClusterName()` accessor on DecoderFilterCallbacks at master tip).

The fixture count stays at 35 (no new fixture directory — 0032 absorbs the
24.2 additions per the PLAN Task 6 brief).

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
+ (g) `x_ratelimit_headers` cross-side byte-exact wire-comparison.

## Single-listener + single-vhost topology (per parent SPEC §7.3)

ONE listener (`l_test_a`) carrying ONE HCM with the ratelimit filter (in
alphabetical position, with the router terminator) + the RLS cluster
(`c_ratelimit`) pointing at the fake gRPC server + a synthetic always-200
echobackend cluster (`c_backend`). No multi-listener topology — avoids
the `freeTCPPort` combined-run flake per 22.2 REVIEW §7.4 (the 24.1 PLAN
cites this exact constraint).

Single virtual_host `vh_a` (`domains: ["*"]`) per envoy-go's HCM constraint
at `internal/filter/hcm/config.go` (single-vhost canonical per ADR-0019/
0072). The vhost carries a **vhost-level `rate_limits`** emitting
`generic_key{vh:vh_a}` — Phase 24.2 ADDITION; consumed by the (f) sub-
scenarios under `/scenario_f_*` to exercise the §4.3 Axis-B table. The
24.1 scenarios (b/c/d/e) + (g) all carry their own route-level
`rate_limits` ⇒ OVERRIDE-default vhost-SKIP arm fires ⇒ the vh-policy is
INVISIBLE on the 24.1 scenarios (zero byte-stream impact). Scenario (a)
is the EXCEPTION (NO route rate_limits → OVERRIDE-default would walk the
vh); (a) carries a `vh_rate_limits=IGNORE` TPFC to preserve the 24.1
zero-descriptor short-circuit semantics.

Scenarios that need DIFFERENT filter-level config (failure_mode_deny
toggle for fail-open vs fail-closed paths) are NOT exercised at 24.1/24.2
— scenario (e) `failure_mode_open` uses the DEFAULT `failure_mode_deny:false`
(the AMEND-3 proto-zero default) so a single listener config suffices.
Per-route discrimination across scenarios lives at the route-table level
via the `rate_limits[]` policy entries (ADR-0198 DELTA-2) + the per-route
`RateLimitPerRoute` TPFC (ADR-0199; 24.2). Each scenario path carries its
own descriptor-producing config:

| Scenario | Path | TPFC | Descriptors | Fake script (CanonicalKey) |
|---|---|---|---|---|
| (a) `parse_ok` | `/scenario_a` | `vh_rate_limits: IGNORE` | none (route has NO `rate_limits` + IGNORE TPFC skips vh) | n/a (no RLS call fires) |
| (b) `ok_admit` | `/scenario_b` | — | `generic_key{key=scenario,value=b}` (OVERRIDE-default → vh skipped) | `domain_b \| scenario=b` → OK |
| (c) `over_limit_429` | `/scenario_c` | — | `generic_key{key=scenario,value=c}` | `domain_b \| scenario=c` → OVER_LIMIT |
| (d) `descriptor_actions` | `/scenario_d?region=us-east&plan=premium` | — | 9-action chain (see below) | per-action key (see driver) → OK |
| (e) `failure_mode_open` | `/scenario_e` | — | `generic_key{key=scenario,value=e}` | RLS server stopped → transport error → fail-open admit |
| (f1) `vh_override` | `/scenario_f_override` | — (proto-zero OVERRIDE) | `generic_key{scenario:f_override}` (route only — vh skipped under OVERRIDE since route non-empty) | `domain_b \| scenario=f_override` → OK |
| (f2) `vh_include` | `/scenario_f_include` | `vh_rate_limits: INCLUDE` | `[route, vhost]` 2 descriptors (route first per AMEND-6) | `domain_b \| scenario=f_include \| vh=vh_a` → OK |
| (f3) `vh_ignore` | `/scenario_f_ignore` | `vh_rate_limits: IGNORE` | `generic_key{scenario:f_ignore}` (route only — IGNORE unconditional vh-skip) | `domain_b \| scenario=f_ignore` → OK |
| (g) `x_ratelimit_headers` | `/scenario_g` | — | 2 single-action policies → 2 descriptors | `domain_b \| tier=bronze \| scope=burst` → OVER_LIMIT + per-descriptor statuses |
| (h) `stat_surface` | n/a (observational) | n/a | reuses preceding deltas | `StatsAsserter.AssertStats` asserts counter deltas AFTER all probes |

The single shared `domain` (`domain_b`) appears in all script keys per
the filter's `compiledConfig.domain` (set on `envoy.yaml`/`envoy-go.yaml`
ratelimit filter config). The bootstrap `node.cluster: rls_test_cluster`
seeds the (d) extension `source_cluster` action's value.

## 9-scenario matrix

### (a) parse_ok — subject-only structural via cross-side byte-stream

The driver issues one `GET /scenario_a` against `l_test_a`. The matched
route carries NO `rate_limits` policy AND a `vh_rate_limits=IGNORE` TPFC
so the vhost-level `rate_limits` is SKIPPED → the descriptor engine
produces zero descriptors → DecodeHeaders short-circuits to Continue →
echo backend returns 200. The driver emits `scenario a status=200 body=ok`
into the cross-side byte stream; both sides produce identical bytes ⇒
`CompareBytes` passes. This is the byte-stream-confirmed "the ratelimit
filter parses, loads, and the zero-descriptor path is byte-equivalent
to reference Envoy" structural assertion.

### (b) ok_admit — cross-side byte-exact

One `GET /scenario_b` against `l_test_a`. The route carries a single
`generic_key{descriptor_key:scenario, descriptor_value:b}` policy → the
descriptor engine emits `[{scenario:b}]` (OVERRIDE-default → vh skipped
since route is non-empty) → RLS fake matches
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
- header: `x-from-config: filtercfg` (filter-config response_headers_to_add slot [c])
- body: empty (no `RawBody` on the scripted response)

24.2 NOTE: the filter-config carries `response_headers_to_add:
[{key: x-from-config, value: filtercfg}]` (added at 24.2 for scenario (g)'s
wire-order test). On (c) the response includes that header too — the byte
stream does NOT capture per-hop headers, so this addition has zero
byte-stream impact. Cross-side `CompareBytes` on the status line + body
classification asserts byte-equivalence. The subject's `over_limit`
counter increments.

### (d) descriptor_actions — cross-side byte-exact (9 actions; phase 24.2 EXTENSION)

One `GET /scenario_d?region=us-east&plan=premium` against `l_test_a` with
three discriminating client-supplied request bits:
- `x-tenant: tenant-x` (read by the `request_headers` action)
- `x-canary: true` (read by the `header_value_match` action's headers
  matcher — `present_match:true` on `x-canary`)
- the loopback peer address (consumed by the `remote_address` AND
  `masked_remote_address` actions)
- the query string `?region=us-east&plan=premium` (consumed by
  `query_parameters` + `query_parameter_value_match`)

Plus the route-level `metadata.filter_metadata.envoy.filters.http.ratelimit.tier:
gold` consumed by the `metadata` action (`ROUTE_ENTRY` source); plus the
bootstrap-level `node.cluster: rls_test_cluster` consumed by the
`source_cluster` action.

The route's `rate_limits[]` carries ONE policy with NINE actions in this
order (AMEND-6 entries-in-action-list):
1. `generic_key{scenario:d}` → `scenario=d`
2. `request_headers{header_name:x-tenant, descriptor_key:tenant}` →
   `tenant=tenant-x`
3. `remote_address{}` → `remote_address=127.0.0.1`
4. `header_value_match{descriptor_value:canaried,
   headers:[{name:x-canary, present_match:true}]}` → `header_match=canaried`
5. **24.2** `source_cluster{}` → `source_cluster=rls_test_cluster`
6. **24.2** `masked_remote_address{v4_prefix_mask_len:24}` →
   `masked_remote_address=127.0.0.0/24`
7. **24.2** `metadata{descriptor_key:tier, metadata_key:..., source:ROUTE_ENTRY}` →
   `tier=gold`
8. **24.2** `query_parameters{query_parameter_name:region, descriptor_key:region}` →
   `region=us-east`
9. **24.2** `query_parameter_value_match{descriptor_value:premium_plan,
   query_parameters:[{name:plan, string_match:{exact:premium}}]}` →
   `query_match=premium_plan` (default `descriptor_key` per AMEND-11)

The descriptor engine emits ONE descriptor with NINE entries. The fake
script keyed
`domain_b|scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried;source_cluster=rls_test_cluster;masked_remote_address=127.0.0.0/24;tier=gold;region=us-east;query_match=premium_plan`
returns OK ⇒ filter Continue ⇒ echo 200. Cross-side `CompareBytes` on
the status line asserts byte-equivalence.

**NO `destination_cluster` action.** Per the 24.1 framework-limitation
note (decode_headers.go file-header step 2) the DecoderFilterCallbacks
surface has NO `MatchedClusterName()` accessor at master tip → the
`destination_cluster` action's whole-descriptor-drop arm at
`descriptors.go::actionDestinationCluster` ALWAYS fires under empty
cluster-name input → a scenario exercising it would silently drop the
whole descriptor (per §4.5 behavior 1) ⇒ the RLS call would NOT fire ⇒
the fake's "default OK" arm would kick in, masking the assertion.
24.2 fixture 0032 therefore restricts scenario (d) to the NINE framework-
reachable actions per the PLAN Task 6 brief.

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
`applyError` fail-open arm). Per the 24.2 probe-order change, (e) is the
LAST probe in the sequence — the fake-stop happens after every other
scenario (including the 24.2 additions f1/f2/f3/g).

### (f) vh_inclusion — cross-side byte-exact (3 sub-scenarios; phase 24.2)

Phase 24.2 Task 6 addition. Three sub-scenarios exercising the parent
SPEC §4.3 Axis-B cross-tier composition table. The vhost `vh_a` carries
a vhost-level `rate_limits` emitting `generic_key{vh:vh_a}`. The route-
level `rate_limits` emits a different per-scenario `generic_key{scenario:f_*}`.
The cross-tier composition table:

| Sub | `vh_rate_limits` | route `rate_limits` | vh policy walked? | route policy walked? | descriptor count |
|---|---|---|---|---|---|
| (f1) override | OVERRIDE (proto-zero default — NO TPFC) | non-empty | NO (route non-empty wins under OVERRIDE) | YES | 1 (route only) |
| (f2) include | INCLUDE (TPFC RateLimitPerRoute.vh_rate_limits=INCLUDE) | non-empty | YES | YES | 2 (route first, vhost second per AMEND-6) |
| (f3) ignore | IGNORE (TPFC RateLimitPerRoute.vh_rate_limits=IGNORE) | non-empty | NO (IGNORE unconditional) | YES | 1 (route only) |

All three sub-scenarios return OK from the fake (no OVER_LIMIT) → echo
200. The fake CanonicalKey lookups discriminate on the descriptor set:
- (f1): `domain_b|scenario=f_override` (1-descriptor)
- (f2): `domain_b|scenario=f_include|vh=vh_a` (2-descriptor)
- (f3): `domain_b|scenario=f_ignore` (1-descriptor)

Cross-side `CompareBytes` on the status line asserts byte-equivalence.

The stronger detection of "descriptor set divergence between sides" lives
in the Task 4 unit tests (descriptors_test.go + decode_headers_test.go);
this fixture is the integration smoke that confirms both sides walk the
table end-to-end without diverging in the AMEND-5 legacy-bool-trumps-enum
+ AMEND-4 Axis-A early-return precedence wiring.

### (g) x_ratelimit_headers — cross-side byte-exact + X-RateLimit triple byte-pin (phase 24.2)

Phase 24.2 Task 6 addition. One `GET /scenario_g` against `l_test_a`.
The route carries TWO `rate_limits[]` policies, each with ONE
`generic_key` action → the descriptor engine emits TWO descriptors:
- `[{tier=bronze}]`
- `[{scope=burst}]`

The fake matches `domain_b|tier=bronze|scope=burst` and returns
OVER_LIMIT with per-descriptor statuses populated:

| Index | code | requests_per_unit | unit | limit_remaining | duration_until_reset.seconds |
|---|---|---|---|---|---|
| 0 | OVER_LIMIT | 10 | SECOND | 2 | 1 |
| 1 | OVER_LIMIT | 100 | MINUTE | 7 | 60 |

Filter-level `enable_x_ratelimit_headers: DRAFT_VERSION_03` is set ⇒ the
X-RateLimit emission gate fires. `buildXRateLimitHeaders` per
`headers.go`:
- MIN selection: strict-`<` ⇒ status[0] (LimitRemaining=2 < 7) wins.
- `x-ratelimit-limit`: `10, 10;w=1, 100;w=60` (MIN's rpu + quota-policy
  segments for BOTH descriptors per the upstream iterate-all pattern;
  no `;name=` clause since no `name` field is set).
- `x-ratelimit-remaining`: `2` (MIN.LimitRemaining).
- `x-ratelimit-reset`: `1` (MIN.DurationUntilReset.Seconds; nanos
  IGNORED per upstream).

Wire order on OVER_LIMIT per §4.7 line 214 + the Task 5 follow-up
discipline:
1. `x-envoy-ratelimited: true` (slot [a]; AMEND-8)
2. (none — no RLS response_headers_to_add scripted)
3. **X-RateLimit triple** (slot [c-pre]; inlined at `applyOverLimit`)
4. `x-from-config: filtercfg` (slot [c]; filter-config `response_headers_to_add`)

The driver byte-pins the X-RateLimit triple values into the cross-side
byte stream (3 extra `scenario g <header>=<value>` lines after the
status/body verdict). Both sides MUST emit identical values; CompareBytes
detects any divergence on MIN-selection / unit→seconds map / quota-policy
suffix construction.

The subject's `over_limit` counter increments (per dispositions
`applyOverLimit` arm).

### (h) stat_surface — subject-only via `StatsAsserter.AssertStats`

OBSERVATIONAL — no additional probe burst; (h) asserts the counter
deltas accumulated by the preceding probes. `StatsAsserter.AssertStats`
scrapes the SUBJECT admin `/stats/prometheus` and asserts the four
cluster-scoped counters at the deterministic deltas:

| Counter (Prometheus form) | Expected value | Source |
|---|---|---|
| `envoy_cluster_ratelimit_ok{envoy_cluster_name="c_ratelimit"}` | 5 | b + d + f1 + f2 + f3 (admit) |
| `envoy_cluster_ratelimit_over_limit{envoy_cluster_name="c_ratelimit"}` | 2 | c + g (OVER_LIMIT) |
| `envoy_cluster_ratelimit_error{envoy_cluster_name="c_ratelimit"}` | 1 | e (RLS unreachable) |
| `envoy_cluster_ratelimit_failure_mode_allowed{envoy_cluster_name="c_ratelimit"}` | 1 | e (failure_mode_deny:false) |

The observational shape (vs a separate burst phase) is INTENTIONAL — the
shared in-process fake stops cleanly but the subject's gRPC sub-channel
manages reconnect state per ADR-0158, and a stop-then-restart-on-the-same-port
within a single fixture run is NOT a pattern the existing fake-toggle
fixtures exercise (0021 puts the STOPPED state at the END of the live
batch; never a mid-stream restart). Observational (h) avoids the
reconnect edge AND keeps the per-scenario counter deltas pinned 1:1 to
the probe sequence — no burst-replay accounting drift. Scenario (e) is
the LAST probe in the sequence per the 24.2 probe-order change (so the
fake-stop happens after every other scenario including the 24.2
additions).

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
publishes the same names + values; (h) asserts the SUBJECT counters —
the cross-side equivalence on these counters is ratified at 24.2 if
needed (24.1/24.2's primary cross-side gate is the byte-stream
`CompareBytes` on the per-scenario lines).

## Scripting discipline (per Task 9 advisory)

The shared fake's `ShouldRateLimit` returns a default OK on no-match
(unlike `extauthzgrpc` which returns `codes.Unavailable`). To avoid
silent test passes from forgotten scripts every expected canonical key
is EXPLICITLY scripted in `driver.go::setupRLS` (which starts the fake +
pre-populates every per-scenario script). The driver also
logs the scripted-key set under `FIXTURE_0032_DUMP_BYTES=1` for
debug visibility.

The phase 24.2 Task 6 additions extend the script map with 5 new keys:
- `scenario=f_override` → OK (1-descriptor)
- `scenario=f_include|vh=vh_a` → OK (2-descriptor)
- `scenario=f_ignore` → OK (1-descriptor)
- `tier=bronze|scope=burst` → OVER_LIMIT with per-descriptor
  CurrentLimit / LimitRemaining / DurationUntilReset populated
- The (d) script's key was REWRITTEN to carry 9 entries (was 4 at 24.1).

## Reference container ports

| Listener / endpoint | In-container port |
|---|---|
| `l_test_a` | 10032 |
| admin | 9901 |

(`c_ratelimit` upstream cluster endpoint port is runner-allocated +
shared per the `0021` pre-allocated-port + `host.docker.internal`
templating pattern.)

## Cross-references

- parent SPEC §7.1 (8-scenario matrix — 24.1 scope a/b/c/d-core/e/h; 24.2 ADDS f/g)
- parent SPEC §7.3 (single-listener topology)
- parent SPEC §4.3 (Axis-B vh_rate_limits cross-tier composition table — scenario f)
- parent SPEC §4.7 line 214 (OVER_LIMIT wire-order; AMEND-8 + Task 5 follow-up)
- 24.1 SPEC §7 (24.1 scenario scoping)
- 24.2 SPEC §3 (24.2 scenario additions)
- AMEND-1 (cluster-scoped stat surface)
- AMEND-3 (filter-config 13-field roster + defaults/clamps)
- AMEND-4 (Axis-A embedded-config precedence — per-route `rate_limits[]` early-return)
- AMEND-5 (`vh_rate_limits` composition + legacy `include_vh_rate_limits` force-include)
- AMEND-6 (proto-number-faithful fake encoding — load-bearing for byte-exact)
- AMEND-8 (OVER_LIMIT header order; X-RateLimit slot between x-envoy-ratelimited and filter-config)
- AMEND-10 (cross-namespace cluster-stat charging)
- AMEND-11 (per-action key defaults — `query_match` default for `query_parameter_value_match`)
- ADR-0010 (`host.docker.internal` reference-container loopback alias)
- ADR-0166 (H2-without-TLS upstream — plaintext h2c rls cluster)
- ADR-0197 (filter shape + dispositions + cross-namespace stat charge — CORE slice + X-RateLimit amend at 24.2 Task 5)
- ADR-0198 (DELTA-2 route-table accessor pair — 24.1 Task 5)
- ADR-0199 (RateLimitPerRoute NEW 10th canonical — 24.2 Task 3)
- ADR-0200 (route-level PARSE-REJECTs — 24.1 Task 3)
- ADR-0201 (24/24.1/24.2 split)
- `reference_differential_asserter_dispatch` (subject assertions in `StatsAsserter`, not `SubjectAsserter`)
- `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch; cross-side scenarios share the cross-side branch; (h) StatsAsserter dispatch unchanged per 24.1 precedent)
- 18.2 fixture-0021 (template precedent — fixed pre-allocated port + `host.docker.internal`/`127.0.0.1` templating + `extauthzgrpc.NewAtAddr` analogue)
- 23 fixture-0030 (StatsAsserter precedent + single-bootstrap two-listener pattern; 0032 takes the single-listener path)
