# Fixture 0005-prometheus-stats

## Purpose

This is the project's **first observability-surface differential** and the **first non-vacuous gate-(a)** on the observability surface (SPEC §3 gate (a) non-vacuous constraint).

The fixture exercises the behavioral-delta equivalence between envoy-go and reference Envoy v1.37.2 on the 17 stat names emitted by phase 06.1 (SPEC §6). It drives a 5-request defined-load workload at both proxies, scrapes `/stats/prometheus` before and after on both sides, and asserts:

- **Per-counter delta-equality**: `delta_envoy_go == delta_envoy` on each of the 12 counter names.
- **Per-gauge snapshot-equality**: `after_envoy_go == after_envoy` on each of the 5 gauge names, observed after the driver waits for active connections to drain to zero.

HELP-text values are ignored per Rule SN6. Non-listed metric names in either side's `/stats/prometheus` output are ignored by the allow-list discipline (see `expectations.yaml`).

## STATIC vs STRICT_DNS divergence

Mirrors the pattern from fixtures 0001/0002/0003/0004 (ADR-0027):

- **Subject (envoy-go):** cluster `c0` uses `type: STATIC` pointing at `127.0.0.1:<backend-port>` (host-side, directly reachable from the test process).
- **Reference (Envoy):** cluster `c0` uses `type: STRICT_DNS` with `dns_lookup_family: V4_ONLY` pointing at `host.docker.internal:<backend-port>` (container-side reach to the host per ADR-0010).

The `--concurrency 1` flag (ADR-0028) is set unconditionally by the runner's testcontainers `Cmd` to keep Envoy's per-worker connection state deterministic.

## 5-request defined-load shape

The driver sends exactly 5 sequential HTTP/1.1 requests with target statuses `[200, 200, 404, 200, 502]`:

| Request | Path | Status | Reaches cluster? |
|---------|------|--------|-----------------|
| 1 | `GET /` | 200 | Yes |
| 2 | `GET /` | 200 | Yes |
| 3 | `GET /missing` | 404 | No — HCM direct_response |
| 4 | `GET /` | 200 | Yes |
| 5 | `GET /` with `X-Backend-Status: 502` | 502 | Yes (backend returns 502 explicitly) |

The 404 is served by the HCM's `direct_response` and never reaches cluster `c0`; hence `cluster.c0.upstream_rq_total` delta is 4 (not 5). The 502 is returned explicitly by the controlled backend (not a dial-failure) — this decouples the differential from any envoy-go-vs-Envoy semantic gap in dial-error→status-code mapping.

## Per-counter delta-equality + per-gauge snapshot-equality

Per ADR-0062 (Differential equivalence shape for stats output):

- **Counter assertion:** `(after_envoy_go - before_envoy_go) == (after_envoy - before_envoy)` for each counter name.
- **Gauge assertion:** `after_envoy_go == after_envoy` for each gauge name (snapshot after the driver waits for `cx_active` gauges to reach 0).
- `listener.<addr>.downstream_cx_total` and `cluster.c0.upstream_cx_total` use `delta_min >= 1` because HTTP keepalive may collapse multiple requests onto fewer TCP connections — the exact count is non-deterministic but the direction (at least 1) is guaranteed.

## 17-name allow-list and stat-name mapping

The 17 internal stat names mapped by this fixture are enumerated in `expectations.yaml`. The Prometheus-name flattening rules (SN1–SN8) that govern the `internal → Prometheus` name mapping are documented in `BEHAVIOR_CONTRACT.md ## Stat-name mapping`.

**Note:** At the time of this fixture's implementation (Task 14), `BEHAVIOR_CONTRACT.md ## Stat-name mapping` is populated at Task 15 (the phase-done commit). Cross-reference `docs/envoy-go/DECISIONS.md` ADR-0061 for the full SN1–SN8 rules with empirical evidence.

## Reference

- ADR-0062: Differential equivalence shape for stats output (this fixture's assertion contract)
- ADR-0061: Stat-name → Prometheus-name flattening rules SN1–SN8
- ADR-0063: Per-endpoint cluster stats not emitted in 06.1
- ADR-0010: STRICT_DNS dns_lookup_family V4_ONLY
- ADR-0027: STATIC vs STRICT_DNS divergence
- ADR-0028: --concurrency 1 reference invocation
