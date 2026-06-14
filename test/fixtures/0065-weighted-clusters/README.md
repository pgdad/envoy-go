# 0065-weighted-clusters

Cross-side `[http_connection_manager + router]` differential over **three clusters**
via `RouteAction.weighted_clusters` — `c_a` (weight 50), `c_b` (weight 30), `c_sub`
(weight 20) — on BOTH sides (the 0003/0064 HTTP shape: reference STRICT_DNS /
`host.docker.internal`, subject STATIC / `127.0.0.1`). `c_sub` is a
`lb_subset_config` cluster with `fallback_policy: ANY_ENDPOINT` +
`subset_selectors: [{keys: ["version"]}]` over two endpoints: `backend2`
(`version: v1`) and `backend3` (`version: v2`). The `ClusterWeight.metadata_match
{version: v1}` at the routing plane constrains every `/w` request routed to `c_sub`
to ONLY the v1 subset — proving **RouteAction.weighted_clusters →
WeightedCluster.ClusterWeight.metadata_match → SubsetMatch composition**.

Phase 38.2 SPEC D-WC-IMPL-3 / PLAN Task 8.

## Topology: 4 backends / 3 clusters

| backend | cluster  | lb subset | effective weight |
|---------|----------|-----------|-----------------|
| 0       | `c_a`    | (none)    | 50 / 100 = 50%  |
| 1       | `c_b`    | (none)    | 30 / 100 = 30%  |
| 2       | `c_sub`  | `v1`      | 20 / 100 = 20%  |
| 3       | `c_sub`  | `v2`      | **0%** — excluded by `ClusterWeight.metadata_match{version:v1}` |

## The KEY assertion: composition affinity

`c_sub`'s `ClusterWeight.metadata_match {version: v1}` is merged with `c_sub`'s
`lb_subset_config` at the routing plane. The merged match `{version: v1}` selects
ONLY backend2 from the `c_sub` cluster — backend3 (`v2`) is excluded. This is the
**composition invariant**: `backend[3] == 0` after 500 `/w` requests. Any breakage
of the routing-plane → subset-plane merge pipeline surfaces as `backend[3] > 0`.

## The workload (per side)

`n=500` `GET /w` (each request is routed to a backend by the weighted RNG) +
`healthReqs=8` `GET /health` (a `direct_response "OK\n"` — the byte-equiv stream).
Each request is a fresh dial (`HTTPRoundTrip` sets `Connection: close`), so each
routed request is one upstream connection → the `HTTPEcho` backend's accept counter
increments once per request.

`/health` is a `direct_response` (`inline_string: "OK\n"`) served by the listener —
it does NOT touch any backend (no accept, no `upstream_cx`), and its body is
**address-independent** → byte-equal across both proxies. That `"OK\n" × 8` stream
is the runner's `CompareBytes` input (the 0003 byte-equiv precedent). The `/w`
bodies are NOT concatenated into the compared stream: the weighted-random per-request
order differs cross-side even though the PER-SIDE aggregate distribution matches the
weights.

## The distribution arm (`AssertDistribution`, PER-SIDE band)

The runner snapshots per-backend accept TOTALS after Drive. `AssertDistribution`
applies a **PER-SIDE σ-band** (both sides run independent RNG streams — cross-side
per-request equality is infeasible for RNG-based routing;
`reference_differential_hash_key_cross_side_infeasible`). Bands at ~4.5σ margin
(`reference_differential_band_sigma_margin`) for flake-free runs:

| backend | cluster / subset | mean (n=500) | σ     | band     |
|---------|-----------------|--------------|-------|----------|
| 0       | `c_a` (p=0.50)  | 250          | 11.18 | [200, 300] |
| 1       | `c_b` (p=0.30)  | 150          | 10.25 | [104, 196] |
| 2       | `c_sub/v1` (p=0.20) | 100     | 8.94  | [60, 140]  |
| 3       | `c_sub/v2`      | 0            | —     | **== 0** (composition affinity) |

Plus **conservation**: `Σ counts == 500` on each side (`/health` is `direct_response`
and NEVER reaches a backend — excluded from the routed sum;
`reference_fixture_workload_constant_desync`).

## The stats prong (`StatsAsserter`, post-drive)

The stats prong:

1. **"decode ran" guard** (`reference_docker_probe_bridge_network`): verifies
   `Σ cluster.{c_a,c_b,c_sub}.upstream_rq_total > 0` on the reference side before
   trusting the readout.

2. **Conservation Σ** on each side: `Σ cluster.{c_a,c_b,c_sub}.upstream_rq_total == 500`
   (all 500 `/w` requests were routed to a backend by HCM router).

3. **Per-cluster quiesce**: each cluster's `upstream_cx_active == 0` on both sides
   (`Connection: close` → each request is a fresh dial that completes before the next).

The per-cluster `upstream_rq_total` split (`c_a` vs `c_b` vs `c_sub`) is **NOT
cross-equaled** — the RNG picks differ per-side.

## Deliberate-break liveness (Task 9 — `-count=1`)

Each break was applied ONE AT A TIME, run with `-count=1` to defeat go-test caching
(`reference_differential_break_protocol_count1`), confirmed `--- FAIL`, then
`git restore`d. Selector: `-run 'TestDifferential/0065'`
(`reference_differential_run_selector`).

| # | edit | prong proven | observed `--- FAIL` |
|---|------|--------------|---------------------|
| (a) | **Swap `c_a`/`c_sub` weights** in `routeTable`: `c_a→20`, `c_sub→50` (keep `c_b→30`) | band: backend[0] drops to ~100, outside [200,300] | `reference: backend[0]=95 outside weighted band [200,300] (swapped weights? dropped cluster?)` |
| (b) | **Drop the `c_b` entry** (weight 30) from the `/w` weighted_clusters list | band: backend[0] rises to ~340 (total weight now 70, c_a dominates), outside [200,300] | `reference: backend[0]=340 outside weighted band [200,300] (swapped weights? dropped cluster?)` |
| (c) | **Remove `metadata_match: {filter_metadata: {"envoy.lb": {version: "v1"}}}` from the `c_sub` ClusterWeight** (keep `c_sub`'s cluster-level `fallback_policy: ANY_ENDPOINT`) | composition: no per-entry match → ANY_ENDPOINT fallback spreads across both v1 (backend2) and v2 (backend3); backend2 drops below b2Lo=60 | `reference: backend[2]=47 outside weighted band [60,140] (swapped weights? dropped cluster?)` |

All three breaks were reverted with `git restore`; `git diff` confirmed empty after each revert.

## Flake check (Task 9)

20 consecutive runs after all reverts:

```
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0065' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FLAKE"; done
```

Result: **20/20 PASS** — no flakes.

## Non-additions

- **NO new BackendKind** — reuses `HTTPEcho` (the 0003/0064 backend); the backend
  tail STAYS at 33.
- **NO new fuzzer** — the weighted_clusters config is static YAML; the weight
  randomization/Pick property tests are UNIT-level (covered by Task 7 accept-case tests).
- **NO boot-reject dir** — the `weighted_clusters` config-reject arms land UNIT-LEVEL.
