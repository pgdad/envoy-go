# Fixture 0006 — Access-log differential

**Purpose:** Differential equivalence of per-request access-log records between
envoy-go and reference Envoy v1.37.2 under the 5-request workload from SPEC §7.2.
This is the project's second observability-surface differential (after fixture
0005-prometheus-stats) and the first asserting per-record field-by-field
equivalence between per-side access-log files.

## Workload

5 sequential HTTP/1.1 GETs per side:

| # | Path | Action | Status | BYTES_SENT |
|---|---|---|---|---|
| 1 | `/health` | direct_response 200 | 200 | 3 (`OK\n`) |
| 2 | `/api/v1/foo` | routed → c_backend | 200 | 17 (`backend:v1/fixed\n`) |
| 3 | `/api/v1/bar` | routed → c_backend | 200 | 17 |
| 4 | `/api/v1/baz` | routed → c_backend | 200 | 17 |
| 5 | `/notfound` | direct_response 404 | 404 | 10 (`not found\n`) |

The 17-byte body is byte-identical across all 3 backends so that `BYTES_SENT`
Tier-E equality holds regardless of which RR endpoint serves each request.

## STATIC vs. STRICT_DNS divergence

Subject (envoy-go) uses `type: STATIC` with 3 endpoints at `127.0.0.1:<port>`.
Reference (Envoy container) uses `type: STRICT_DNS` with `dns_lookup_family:
V4_ONLY` and 3 endpoints at `host.docker.internal:<port>` (per ADR-0010).

## Log-file mounting convention

Subject writes directly to `<t.TempDir()>/subject.log` (host path templated via
`SubjectConfig`). Reference writes to `/tmp/envoy-access.log` inside the
container; the harness bind-mounts `<t.TempDir()>/reference.log` to
`/tmp/envoy-access.log` via `testcontainers-go` `Mounts` so the driver can read
it as a host file.

## Drain discipline (per 06.1 REVIEW M-8 prophylactic adoption)

After the 5th response is received per side, the driver polls the log file at
25 ms intervals until each side's file has ≥ 5 lines, OR a 5 s hard deadline
trips. On deadline-trip the driver fails with a diagnostic showing the side name
and observed line count. No `time.Sleep(arbitrary)` — this is the pattern
Decision G mandates for new fixtures.

## Three-tier equivalence matrix

Records are paired by index. Per ADR-0068:

- **Tier E** (byte-equal cross-side, 8 operators):
  `:METHOD`, `:PATH`, `PROTOCOL`, `RESPONSE_CODE`, `BYTES_SENT`,
  `RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)` (both `-`), `USER-AGENT`, `:AUTHORITY`
- **Tier F** (format-only, 3 operators):
  `START_TIME` (RFC3339 ms-precision UTC), `DURATION` (int ms ≥ 0),
  `UPSTREAM_HOST` (`host:port` for routed; `-` for direct_response)
- **Tier S** (subject emits `-`, reference unconstrained, 4 operators):
  `RESPONSE_FLAGS`, `BYTES_RECEIVED`, `X-FORWARDED-FOR`, `X-REQUEST-ID`

Cross-reference: `docs/envoy-go/BEHAVIOR_CONTRACT.md ## Access log field mapping`
(populated at phase 06.2 phase-done commit per ADR-0052 / ADR-0068).
