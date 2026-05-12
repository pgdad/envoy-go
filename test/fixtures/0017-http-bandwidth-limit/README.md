# Fixture 0017 — `envoy.filters.http.bandwidth_limit` differential equivalence

Six scenarios per phase 15 SPEC §7.1; sequential against listener `l_test_a`
(HCM chain `bandwidth_limit → router`) with six routes (`/echo-response`,
`/echo-request`, `/echo-both`, `/echo-tiny`, `/echo-disabled`,
`/echo-override`). Listener `l_test_b` + cluster `c_backend_b` provide the
echobackend pair (reuses `test/helpers/echobackend/` from phase 14) for the
two POST scenarios that exercise upstream-arrival timing. Reference Envoy
v1.37.2 (STRICT_DNS) vs envoy-go (STATIC).

Listener-level config (verbatim per SPEC §7.2):

```yaml
envoy.filters.http.bandwidth_limit:
  stat_prefix: default
  enable_mode: REQUEST_AND_RESPONSE
  limit_kbps: 10            # KiB/s — see "KiB/s units" note below
  fill_interval: 0.05s      # 50ms
```

## Scenarios

1. **Response-only throttle (default route)** — `GET /echo-response` → 200 + 10240-byte direct_response body; wall-clock ≈ 1000ms ±70ms (chunk_size=512; ticks=ceil(10240/512)=20; throttle=20×50ms=1000ms); `default.http_bandwidth_limit.response_enabled +1`, `response_enforced +20`, `response_incoming_total_size +10240`, `response_allowed_total_size +10240`.
2. **Request-only throttle (per-route `enable_mode: REQUEST`)** — `POST /echo-request` (10240-byte body) → 200 + echobackend JSON echo (body length per-side variable; expectBodyLen=-1 skips byte-exact length); upstream-arrival ≈ 1000ms ±70ms; `default.http_bandwidth_limit.request_enabled +1`, `request_enforced +20`, `request_incoming_total_size +10240`, `request_allowed_total_size +10240`. (Per-route override engages decode-side only; response-side wholly inactive.)
3. **REQUEST_AND_RESPONSE symmetric (inherits listener)** — `POST /echo-both` (5120-byte body) → 200 + echobackend JSON echo (expectBodyLen=-1); total wall-clock ≈ 1000ms ±70ms (decode 500ms + encode 500ms sequential; per-direction ticks=ceil(5120/512)=10); both `request_*` AND `response_*` counters bump: `_enabled +1` each direction, `_enforced +10` each direction, `_incoming_total_size / _allowed_total_size +5120` each direction.
4. **Tiny body within one-tick floor** — `GET /echo-tiny` → 200 + 100-byte direct_response body; wall-clock ≈ 50ms ±70ms (100 < chunk_size=512 → one-tick floor per §6.6; ticks=1); `default.http_bandwidth_limit.response_enabled +1`, `response_enforced +1`, `response_incoming_total_size +100`, `response_allowed_total_size +100`.
5. **Per-route disabled (per-route `enable_mode: DISABLED`)** — `GET /echo-disabled` → 200 + 10240-byte direct_response body; wall-clock < 70ms (no throttle on disabled route per §11.P12; upper-bound-only assertion via `assertWithinTolerance`'s `expectThrottle==0` branch); NO counter increments — filter wholly inactive (per SPEC §1.1 amendment 1 — `enable_mode: DISABLED` is the disable mechanism; there is NO `disabled` shortcut at proto level).
6. **Per-route INDEPENDENT-stats override (`stat_prefix: override`, `limit_kbps: 100`)** — `GET /echo-override` → 200 + 10240-byte direct_response body; wall-clock ≈ 100ms ±70ms (per-route chunk_size = 100 × 1024 × 0.050 = 5120; ticks=ceil(10240/5120)=2; throttle=2×50ms=100ms); counter deltas land UNDER `override.http_bandwidth_limit.*` per ADR-0139 + SPEC §11.P4 + §11.P14 INDEPENDENT-stats discipline; the listener-level `default.http_bandwidth_limit.*` namespace receives NO increments from this scenario.

(Total cross-scenario deltas — default namespace: `request_enabled +2` (scenarios 2, 3), `request_enforced +30`, `request_*_total_size +15360`; `response_enabled +3` (scenarios 1, 3, 4), `response_enforced +31`, `response_*_total_size +15460`. Override namespace: `response_enabled +1`, `response_enforced +2`, `response_*_total_size +10240` (scenario 6 only).)

## Wall-clock tolerance: ±70ms (per SPEC §11.P9 + §13.5)

Phase 15 establishes a **±70ms** per-scenario wall-clock tolerance — wider
than phase-09 fault's ±10ms and phase-11 local_ratelimit's ±10ms. The wider
envelope absorbs:

- **Initial-burst-capacity approximation variance.** Envoy's token bucket has
  an initial burst capacity (~`limit_kbps × 1024` bytes per probeA); bodies
  fitting within burst complete in 5-107ms regardless of throttle math. The
  envoy-go MVP's Path B-async (one-tick floor at `fill_interval`)
  approximates within ±70ms.
- **`time.AfterFunc` Linux granularity + CI scheduling jitter.** The
  silent-then-blast Path B-async emits the buffered body when the `AfterFunc`
  fires; OS scheduler granularity adds a few ms of jitter.
- **Wire-shape divergence.** envoy-go emits Path B-async (silent buffer +
  one-blast emit at throttle completion); reference Envoy emits Path A rate-
  paced chunks at exact `fill_interval` cadence (per §11.P8 + §11.P15). The
  two converge on the total-throttle-time axis within ±70ms; the chunk-
  arrival-time axis observably diverges.

Disabled-route scenario 5 uses upper-bound-only assertion (`got ≤ Tolerance`;
lower bound trivially 0). All other scenarios use the symmetric ±Tolerance
band.

## KiB/s units (per SPEC §1.1 amendment 6 — BRAINSTORM-time refutation)

The `limit_kbps` field is **kibibytes-per-second (KiB/s), NOT kilobits-per-
second**. The BRAINSTORM-time §1.1 item 3 + §2.3 hypothesis framed the field
as "throttle rate in kilobits-per-second" with steady-rate math
(`throttle_seconds = (body_size_bytes × 8) / (limit_kbps × 1000)`). §11.P15
empirically REFUTES this on TWO axes:

- **Units = KiB/s (1024 bytes/sec), not kbps (1000 bits/sec).** Per the proto
  comment at `bandwidth_limit.pb.go:95` ("The limit supplied in KiB/s") +
  the empirical chunk-size formula confirmed at probeL (51.2 bytes/tick at
  `limit_kbps=1, fill_interval=50ms`).
- **Throttle math = kbps-per-tick chunking, not steady-rate.** Envoy paces
  body bytes at `fill_interval` cadence, emitting `chunk_size` bytes per
  tick where:

  ```
  chunk_size = limit_kbps × 1024 × fill_interval_seconds (bytes/tick)
  throttle   = ceil(body_size / chunk_size) × fill_interval
  ```

For the fixture's listener-level config (`limit_kbps=10, fill_interval=50ms`):
`chunk_size = 10 × 1024 × 0.050 = 512 bytes/tick`. For scenario 1's
10240-byte body: ticks = `ceil(10240/512) = 20` → throttle = `1000ms`.
**NOT** the brainstorm's 8.192-second steady-rate estimate.

## Histograms allow-list (per SPEC §1.1 amendment 9 + BEHAVIOR_CONTRACT §242 twin-series-filter discipline)

Envoy emits **2 unconditional transfer-duration histograms** per active
`stat_prefix`:

- `envoy_<stat_prefix>_http_bandwidth_limit_request_transfer_duration_*`
- `envoy_<stat_prefix>_http_bandwidth_limit_response_transfer_duration_*`

(The `_*` family suffix covers the Prometheus histogram three-name convention:
`_bucket{le="..."}`, `_sum`, `_count`.) These fire UNCONDITIONALLY on Envoy
side (NOT gated by `enable_response_trailers` as BRAINSTORM §8.2 implicitly
assumed); §11.P3 ratified the unconditional-emission behavior.

The envoy-go MVP per phase-06.1 baseline ("counters + gauges only —
histograms deferred") **does not emit histograms**. The driver's
`parseBandwidthLimitPromBody` helper STRIPS the 2 transfer-duration families
from both pre/post scrapes before computing the per-counter delta map. The
substring match `_request_transfer_duration_` OR `_response_transfer_duration_`
catches all three histogram family suffixes (`_bucket` / `_sum` / `_count`)
in a single check. The divergence-window is absorbed by the twin-series
filter; per-counter delta byte-equivalence on the 14 active stats per
stat_prefix (8 counters + 6 gauges per ADR-0138) is preserved.

BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` subsection extends
at Task 15 with a phase-15 entry: 2 unconditional bandwidth_limit transfer-
duration histograms allow-listed pending a future histogram-emit-infra phase
(per SPEC §1.1 amendment 9 — re-activation closes the divergence-window by
extending `filterStats` with 2 histogram fields once `*stats.Registry.Histogram`
+ Prometheus `histogram_*` extractor land).

## Stat namespace (per SPEC §1.1 amendment 8 + §11.P10 + §11.P11)

Internal stat path: `<stat_prefix>.http_bandwidth_limit.<counter>` (single-
segment `http_bandwidth_limit` underscore infix; NOT HCM-rooted). Prometheus
rendering INLINES `stat_prefix` into the base name:
`envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` with NO labels (NO
tag-extractor; NO new SN10 flattening rule per §11.P10). The existing
`internal/stats/name.go` default-branch flatten handles this via the dot→
underscore substitution; ADR-0061 + ADR-0118 are NOT amended.

Scenario 6 introduces a SECOND active `stat_prefix` (`override`) per the
ADR-0139 INDEPENDENT-stats discipline. The `override.http_bandwidth_limit.*`
namespace tree is disjoint from `default.http_bandwidth_limit.*`; scenario 6's
counter bumps land EXCLUSIVELY under `override.*`.

## Envoy deviation

None — the bandwidth_limit filter is a normal HTTP filter. No SIGTERM/drain
divergence; no special HCM wiring. Per-route TPFC handling is the existing
3-tier `Resolve` per ADR-0073 (most-specific-override).

## Future-deferred work

- **Trailer emission** (SPEC §2.1.2 + §8.1): `enable_response_trailers` +
  `response_trailer_prefix` are silent-ignored on envoy-go MVP per the
  phase-06.1 baseline (no trailer-emission framework primitive yet). The
  fixture does NOT exercise the trailer-emission divergence-window; future
  trailer-emission framework phase will close this gap.
- **Histogram emission**: the 2 transfer-duration histograms land on Envoy
  but are stripped on envoy-go side via the twin-series-filter allow-list
  per amendment 9; future histogram-emit-infra phase closes this.

## Planner-time decisions cross-references

Phase 15 PLAN settles 6 decisions (per BRAINSTORM §6.2 + §7 + §9; refined
per SPEC §1.1 amendments). This fixture exercises:

- **D1 (Bare-`BandwidthLimit`-via-TPFC per-route)**: per SPEC §1.1 amendment 1
  + ADR-0139. Scenarios 5 + 6 exercise BOTH per-route mechanisms (DISABLED
  as disable; bare-proto override with own `stat_prefix` + `limit_kbps` +
  `enable_mode: RESPONSE` as INDEPENDENT-stats override).
- **D2 (Path B-async kbps-per-tick body algorithm)**: per ADR-0137 +
  SPEC §1.1 amendment 6 + §11.P15. All 6 scenarios use the chunk_size +
  ceil-ticks throttle formula.
- **D3 (14-active-stat surface)**: per ADR-0138 + SPEC §1.1 amendments 7 + 8.
  8 counters + 6 gauges per stat_prefix; 2 histograms allow-listed.
- **D4 (INDEPENDENT per-route stats)**: per ADR-0139 + SPEC §11.P4 + §11.P14.
  Scenario 6 exercises the second stat_prefix.
- **D5 (±70ms tolerance)**: per SPEC §11.P9 + §13.5. All 6 scenarios use
  the wider envelope absorbing initial-burst + AfterFunc-granularity + CI
  jitter on the Path B-async vs Path A rate-paced chunk-pattern divergence.
- **D6 (echobackend shared helper)**: scenarios 2 + 3 route to
  `test/helpers/echobackend/` (reused from phase 14 per ADR-0133 §Decision
  (ii) precedent on per-side body-length variance).
