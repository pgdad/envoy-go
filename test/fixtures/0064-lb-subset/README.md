# 0064-lb-subset

Cross-side `[http_connection_manager + router]` differential over ONE 4-endpoint
cluster with `lb_policy: ROUND_ROBIN` + `lb_subset_config: {fallback_policy:
ANY_ENDPOINT, subset_selectors: [{keys: ["version"]}]}` + 4 endpoints tagged with
`envoy.lb` metadata (`version: v1` ×2 — endpoints idx 0,1; `version: v2` ×2 —
endpoints idx 2,3) on BOTH sides (the 0003 HTTP shape: reference STRICT_DNS /
`host.docker.internal`, subject STATIC / `127.0.0.1`). This is the **end-to-end
proof of the subset plane** — the HTTP route `RouteAction.metadata_match["envoy.lb"]`
producer → `cluster.SubsetMatch` threaded onto `ctx` at dispatch → `subsetLB.Pick` →
the matching version subset's `ROUND_ROBIN` leaf → within-subset affinity — that
envoy-go's subset load balancer routes a request to the correct version subset with
the SAME SET-membership shape and moves the SAME request-counted subset stats
(cross-equal `lb_subsets_selected` / `lb_subsets_fallback`) as the reference Envoy
`contrib-v1.37.2`.

Phase 38.1 SPEC §8.1 / 38.1 PLAN Task 11.

## The KEY insight vs maglev/ring_hash (0061/0062/0063): NAT-transparent affinity

The subset key is **STATIC ROUTE CONFIG** (`RouteAction.metadata_match`), IDENTICAL
on both sides — it is NOT a wire-derived hash key. So it is NAT-transparent in the
STRONGEST sense: not only does the key survive the Docker hop, the **version→idx MAP
is the SAME on both sides**. The driver BUILDS both bootstraps, tagging endpoints
idx 0,1 = `v1` and idx 2,3 = `v2` in the SAME order the runner spawns the backends,
and the `HTTPEcho` backend embeds its OWN idx in the body (`backend-<idx>:<seg>`).
So **SET-MEMBERSHIP affinity holds TRUE on BOTH sides AND is host-ATTRIBUTABLE per
side** — strictly cleaner than the per-side modular invariant maglev/ring_hash
needed. `reference_differential_hash_key_cross_side_infeasible` is **INVERTED** here:
host identity IS feasible because the map is static config, not an address-keyed
hash table.

## The workload (per side)

For each of the two subset routes (`/v1`, `/v2`) the driver sends **K=16** `GET`
requests, then **K=16** `GET /none` (the ANY_ENDPOINT fallback arm — `version: v9`
matches NO subset), then **8** `GET /health`. The routed total is **3 × 16 = 48**
(`totalReqs`, DERIVED from `routes * perRoute` — never a literal,
`reference_fixture_workload_constant_desync`). Each request is a fresh dial
(`HTTPRoundTrip` sets `Connection: close`), so each routed request is one upstream
connection → the `HTTPEcho` backend's accept counter increments once per request.

`/health` is a `direct_response` (`inline_string: "OK\n"`) served by the listener —
it does NOT touch the backend (no accept, no `upstream_cx`), and its body is
**address-independent** → byte-equal across both proxies. That `OK\n`×8 stream is
the runner's `CompareBytes` input (the 0003 byte-equiv precedent). The ROUTED bodies
are NOT concatenated into the compared stream: `ROUND_ROBIN` alternates within a
subset, so the per-request order of idx 0 vs 1 (and the fallback order over all 4)
may differ cross-side even though the SET is identical.

## The routing arm — per-route SET-membership (asserted INSIDE `drive()`)

The runner's aggregate `AssertDistribution(refCounts, subjCounts)` channel only sees
per-backend **TOTALS** across the whole workload, so it cannot attribute by route.
The per-route SET-membership / within-subset spread / fallback spread are therefore
asserted **INSIDE `drive()`** by parsing each routed response body's embedded backend
idx (`backend-<idx>:<seg>`). A violation fails the drive (→ fails the test). The
asserted behaviors (BOTH sides — the map is NAT-transparent):

- **SET-membership affinity:** every backend serving a `/v1` request ∈ `{0,1}`;
  every `/v2` request ∈ `{2,3}`. 100% DETERMINISTIC under `metadata_match` — a leak
  across the subset boundary (a `/v1` request landing on a v2 host) is a hard fail.
- **within-subset spread:** both members of each 2-host subset are hit across K=16
  (`ROUND_ROBIN` alternates → ≥1 each member).
- **fallback spread:** `/none` (`ANY_ENDPOINT` over all 4 hosts) hits ≥2 of the 4
  backends (the fallback path is not collapsed).

## The conservation arm (`AssertDistribution`, the runner's aggregate channel)

The runner snapshots per-backend accept TOTALS after Drive (each accept on the
`HTTPEcho` backend counts one routed request). `AssertDistribution` carries:

- **conservation:** each side's per-backend counts sum to the ROUTED total
  (`3*perRoute = 48`). `/health` is a `direct_response` that NEVER reaches a backend
  (no accept, no `upstream_cx`), so it is EXCLUDED from the routed sum — accounted
  for explicitly (the 8 health round-trips contribute 0 accepts).
- **full-roster coverage:** all 4 backends are nonzero — `/v1` hits `{0,1}`, `/v2`
  hits `{2,3}`, so the routed union touches EVERY endpoint.

DETERMINISTIC / EXACT — NOT a σ-band (`reference_differential_band_sigma_margin`
governs RNG-distributed bands; the subset key is static config, not RNG).

## The stats prong (`StatsAsserter`, post-drive) — SPEC §8.1

The **request-counted** stats cross-EQUAL on both sides; the build-time
`active`/`created` counters do NOT (the reference contrib build's accounting differs
from envoy-go's ×1-per-distinct-subset count), so `lb_subsets_active` is UNIT-asserted
on the SUBJECT side only.

| stat                                       | reference | subject | disposition                                   |
|--------------------------------------------|-----------|---------|-----------------------------------------------|
| `cluster.c_echo.upstream_cx_total`         | 48        | 48      | **cross-equal** == 48 (= `3*perRoute`)        |
| `cluster.c_echo.upstream_rq_total`         | 48        | 48      | **cross-equal** == 48                         |
| `cluster.c_echo.membership_total`          | 4         | 4       | **cross-equal** == 4                          |
| `cluster.c_echo.upstream_cx_active`        | 0         | 0       | **cross-equal** == 0 (quiesced)               |
| `cluster.c_echo.lb_subsets_selected`       | 32        | 32      | **cross-equal** == 32 (= `/v1`+`/v2`)         |
| `cluster.c_echo.lb_subsets_fallback`       | 16        | 16      | **cross-equal** == 16 (= `/none`)             |
| `cluster.c_echo.lb_subsets_active`         | (ref ×N)  | 2       | **subject-only** unit-assert == 2             |
| `cluster.c_echo.lb_subsets_created`        | (ref ×N)  | —       | NOT asserted (build-time accounting differs)  |

`lb_subsets_selected` counts a request routed to a matched subset (`/v1`, `/v2` →
32) and `lb_subsets_fallback` counts a request that fell through to the fallback
policy (`/none` → 16) — both are **request-counted** so they are IDENTICAL on both
sides. The subject's `lb_subsets_active == 2` is the count of distinct version
subsets enumerated (`v1`, `v2`).

The cross-equal `upstream_cx_total == 48 > 0` also proves the reference actually
**decoded** the subset config (the `reference_docker_probe_bridge_network` "decode
ran" signal — the `StatsAsserter` bites explicitly with a `did NOT decode` Fatalf if
`upstream_cx_total == 0`): had the reference container failed to reach the backends,
the prong would fire.

## Deliberate-break liveness (Task 12 — `-count=1`)

Task 12 applies each prong's break ONE AT A TIME to the fixture's shared config
(the `routeTable` `metadata_match`) or the driver's expected stat, run with
`-count=1` to defeat go-test caching (`reference_differential_break_protocol_count1`),
the named prong observed to `--- FAIL`, then `git restore`d. Selector:
`-run 'TestDifferential/0064'` (prefix-matches `TestDifferential/0064-lb-subset`;
`reference_differential_run_selector` — NEVER `-run '0064'`, which matches ZERO
subtests). Each break is TEMPORARY — only this break table + the PROGRESS.md
evidence are committed (no production change).

| # | break (the exact edit) | prong proven | observed `--- FAIL` (key line, `-count=1`) | restored |
|---|------------------------|--------------|--------------------------------------------|----------|
| 1 | **drop the `/v1` `metadata_match`** — remove the `metadata_match {envoy.lb: {version: v1}}` block from the `/v1` route in `routeTable` (so `/v1` is a plain `cluster: c_echo` route → no subset match → ANY_ENDPOINT fallback → spreads over ALL 4 backends) | SET-membership affinity (`assertSubsetMembership` for `/v1`) | `ref drive: /v1 affinity LEAK: backend[2] served a /v1 request but is not in the subset [0 1] (subset boundary breached)` | `git restore` → re-PASS |
| 2 | **misroute the fallback** — change the `/none` route's `metadata_match` from `version: "v9"` (matches NO subset) to `version: "v1"` (so `/none` lands on the v1 subset instead of taking the ANY_ENDPOINT fallback) | cross-side stats prong (`lb_subsets_selected` / `lb_subsets_fallback`) | `ref/subj cluster.c_echo.lb_subsets_selected = 48, want 32` AND `ref/subj cluster.c_echo.lb_subsets_fallback = 0, want 16` (the misrouted `/none` became a `selected`, not a `fallback`) | `git restore` → re-PASS |
| 3 | **perturb a stats expectation** — change the expected `lb_subsets_selected` want in `AssertStats` from `selectedReqs` (32) to a literal `99` | cross-side stats prong | `ref/subj cluster.c_echo.lb_subsets_selected = 32, want 99` (live observed 32 vs the perturbed expectation 99) | `git restore` → re-PASS |

Note on break 2: the `/none` fallback-spread `drive()` leg does NOT fire (the v1
subset `{0,1}` still hits ≥2 backends), but the stats prong catches the misroute
PRECISELY — both `lb_subsets_selected` and `lb_subsets_fallback` are live.

### Flake check

`go test ./test/differential/ -run 'TestDifferential/0064' -count=20` → **20/20
PASS** (35.9s). Plus `go test ./test/fixtures/0064-lb-subset/driver/ -count=20` →
**20/20 PASS** (the unit asserter logic). The SET-membership + within-subset-spread
legs are DETERMINISTIC (static `metadata_match` + `ROUND_ROBIN` over a 2-host subset
over 16 requests always alternates to hit both); the fallback-spread leg (`>= 2` of
4) is overwhelmingly stable over `ROUND_ROBIN` across 16 requests. `totalReqs` /
`selectedReqs` / `fallbackReqs` are DERIVED from named constants (`routes*perRoute`,
`2*perRoute`, `perRoute`) and `AssertDistribution` sums the per-backend counts
dynamically — NO hand-rolled literal count slice that could desync
(`reference_fixture_workload_constant_desync`).

## Firsts / non-additions

- **FIRST cross-side subset-routing proof** — the static-config `metadata_match` key
  is NAT-transparent in the STRONGEST sense (the version→idx map is identical both
  sides), so SET-membership affinity is host-ATTRIBUTABLE per side (the inverse of
  the maglev/ring_hash modular invariant).
- **FIRST request-counted subset-stat cross-equality** — `lb_subsets_selected` (32)
  + `lb_subsets_fallback` (16) cross-equal; `lb_subsets_active` (subject 2) is
  unit-asserted (the build-time `active`/`created` accounting differs cross-side).
- **NO new BackendKind** — reuses `HTTPEcho` (the 0003 backend); the backend tail
  STAYS 33. An LB phase exercises WHERE requests land, not what the backend speaks.
- **NO new fuzzer** — the subset key derives from static route config, not an
  untrusted wire frame; the subset enumeration / `Pick` property tests are UNIT-level.
- **NO boot-reject dir** — the `lb_subset_config` config-reject arms land UNIT-LEVEL.
