# Fixture 0013-http-local-ratelimit

Differential fixture for phase 11's `envoy.filters.http.local_ratelimit` filter
landing per phase 11 SPEC.md §7. Validates byte-equivalent behavior across
reference Envoy v1.37.2 (STRICT_DNS) and envoy-go (STATIC) under 4 scenarios
covering basic-allow / basic-rate-limited / refill-after-fill_interval /
per-route-override.

## Scenarios

1. **scenario_1_basic_allow** — Listener `l_s1` with `max_tokens=10,
   tokens_per_fill=10, fill_interval=1s, stat_prefix=foo`. Sends 5 sequential
   GETs; expects 5×HTTP 200 + counter deltas
   `foo.http_local_rate_limit.{enabled=5, ok=5, rate_limited=0, enforced=0}`.

2. **scenario_2_basic_rate_limited** — Listener `l_s2` with `max_tokens=2,
   tokens_per_fill=2, fill_interval=60s, stat_prefix=bar`.
   Sends 5 sequential GETs; expects first 2×HTTP 200 + last 3×HTTP 429 with
   byte-exact body `local_rate_limited` (18 bytes, no LF) + 4-header set
   `content-length: 18, content-type: text/plain, date: <RFC1123>, server: envoy`
   + counter deltas `bar.http_local_rate_limit.{enabled=5, ok=2, rate_limited=3,
   enforced=3}` (lockstep MVP invariant per ADR-0118).

3. **scenario_3_refill_after_fill_interval** — Listener `l_s3` with
   `max_tokens=1, tokens_per_fill=1, fill_interval=200ms, stat_prefix=baz`.
   Sends 3 GETs at t=0/10ms/250ms; expects t=0→200, t=10ms→429,
   t=250ms→200 (refill via lazy access at the 200ms boundary); ±10ms wallclock
   tolerance per ADR-0116 + SPEC §11.7 empirical pin.

4. **scenario_4_per_route_override** — Listener `l_per_route` with listener-level
   `qux` config + per-route `/strict` TPFC override + no-override `/loose` route.
   Sends 6 interleaved GETs; expects `/strict` to rate-limit independently
   (independent bucket per ADR-0117 = ADR-0073 amendment) + listener-level
   counters NOT incremented for `/strict` reqs (wholesale-override per SPEC §11.6).

## 4-listener pre-configured bootstrap

Both `envoy.yaml` (reference) and `envoy-go.yaml` (subject) carry FOUR listeners
per PLAN planner-time decision 8 (which DIVERGES from SPEC §7.1's two-listener
layout to avoid a per-scenario-teardown framework extension to the existing
differential-fixture harness; the harness's `fixture.Driver` interface defines
one Drive call per fixture and does not support per-scenario teardown):
- `l_s1` — scenario 1 bucket params (cap=10, fill=10, interval=1s, stat_prefix=foo)
- `l_s2` — scenario 2 bucket params (cap=2,  fill=2,  interval=60s, stat_prefix=bar)
- `l_s3` — scenario 3 bucket params (cap=1,  fill=1,  interval=200ms, stat_prefix=baz)
- `l_per_route` — scenario 4 (listener qux + per-route /strict + /loose)

Each listener binds its own port (allocated by the runner). The driver dials
each listener in turn within a SINGLE `DriveSubject`/`DriveReference`
invocation; per-listener bucket state is naturally isolated by listener-
distinct factories. All 4 listeners explicitly set `filter_enabled` +
`filter_enforced` to 100% per SPEC §1.1 amendment (RuntimeFractionalPercent
default is 0% — omitting these would silently disable rate-limiting in
reference Envoy, breaking differential equivalence). envoy-go silent-ignores
the fields (per SPEC §2.1 cluster 2); the field presence is for byte-
equivalent config-load behavior.

## No per-scenario teardown

Bucket-state isolation is achieved at boot time via the 4-listener topology;
no per-scenario teardown is required. This avoids the ~50 LoC framework
extension that adding per-scenario teardown to `fixture.Driver` would entail.

## Envoy deviation

NONE. local_ratelimit is a normal HTTP filter; no SIGTERM/drain divergence; no
wire-protocol divergence beyond the documented `server: envoy` value (per SPEC
§1.1 amendment which corrected BRAINSTORM's `envoy-go` hypothesis).

## IMPL-1 substitution applied

Per [`docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md`](../../../docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md)
preamble, the per-route TPFC entry on `/strict` uses
`@type: ...LocalRateLimit` (the SAME proto as the listener-level config);
upstream Envoy v1.37.2 has NO separate `LocalRateLimitPerRoute` message. The
SPEC.md §7 + PLAN.md sketches that wrap fields under `rate_limit:` are
historical artefacts; the actual envoy.yaml + envoy-go.yaml in this directory
place the fields directly under the message.

## ADR cross-references

- ADR-0114: package shape (`localratelimit/` no-underscore)
- ADR-0115: runtimeConfig + PGV + filter-internal `fill_interval >= 50ms` validation
- ADR-0116: tokenBucket primitive (lazy refill on access; ±10ms refill tolerance)
- ADR-0117: per-route bucket isolation (ADR-0073 amendment)
- ADR-0118: 22→26 stat-table + Rule SN9 Prometheus tag-extractor
- ADR-0119: rate-limited response wire shape

## Planner-time decisions cross-references

- D1: Tag-extractor registration site = `internal/stats/name.go` SN9
- D2: Filter-callback wiring = `SetDecoderCallbacks` + `SetEncoderCallbacks`
- D3: PGV plumbing = explicit checks in New
- D4: Scenario 3 tolerance = ±10ms simple time.Sleep (retry-with-deadline reserved)
- D5: Test-only clock injection = SKIP
- 6 (PLAN): file split = bucket.go + local_ratelimit.go
- 7 (PLAN): race-detector test = TestTokenBucket_ConcurrentTryConsume
- 8 (PLAN): fixture topology = 4 pre-configured listeners (l_s1, l_s2, l_s3, l_per_route); diverges from SPEC §7.1
- 9 (PLAN): BackendKind = HTTPLocalRateLimit BackendKind = 10
