# Fixture 0011 — http-fault

Differential gate for envoy-go's `envoy.filters.http.fault` HTTP filter against
reference Envoy v1.37.2 per phase 09 SPEC §7.

## Equivalence claims (4 scenarios per SPEC §7.1)

1. **scenario1** (`/scenario1`) — listener-level delay-only inheritance:
   200 OK + body `backend\n`, time_total ≈ 100ms, stat `delays_injected += 1`.
2. **scenario2** (`/scenario2`) — combined delay+abort per-route override:
   503 + body `fault filter abort` (18 bytes, no newline) + 4-header set,
   time_total ≈ 100ms, stats `delays_injected += 1`, `aborts_injected += 1`.
3. **scenario3** — per-route wholesale-override demo:
   - **3a** (`/scenario3-wholesale`) — abort 418 wholesale-replaces listener delay:
     418 + body `fault filter abort`, time_total < 50ms (NO inherited delay).
   - **3b** (`/scenario3-baseline`) — no per-route override, inherits listener:
     200 + body `backend\n`, time_total ≈ 100ms.
4. **scenario4** (`/scenario4`) — headers-field exact-match gate:
   4 sub-probes a/b/c/d testing case-insensitive header NAME + case-sensitive
   header VALUE per §11.8.

## Bootstrap discipline

- Reference: Envoy v1.37.2 in Docker; admin :9902, listener :10001 (in-container;
  published to runner-allocated host ports).
- Subject: envoy-go on the host; admin + listener on runner-allocated ports.
- Backend: `test/fixtures/0011-http-fault/backends/backend.go` (Go HTTP/1.1)
  bound to a runner-allocated port; serves `200 OK` + body `backend\n` on `/`.
- Cluster: reference Envoy uses STRICT_DNS pointing at `host.docker.internal`
  (per ADR-0010); envoy-go subject uses STATIC pointing at `127.0.0.1`
  (envoy-go's cluster manager only supports STATIC, per the Task 12 follow-up
  amendment to planner-time decision 8).

## Status-text allow-list (planner-time decision 7)

For the non-stdlib status code 418 (scenario 3a + scenario 4 if extended):
Envoy emits `HTTP/1.1 418 Unknown` (no built-in status-text table for non-RFC
codes); envoy-go's `net/http` stdlib emits `HTTP/1.1 418 I'm a teapot`. The
differential equivalence is on STATUS CODE only for non-stdlib codes; status
TEXT is allow-listed. Standard codes (200, 503, 404, 405) compare byte-equal
on both code AND text.

## Twin-stat-series allow-list

`fault.response_rl_injected` is emitted as a permanently-zero counter on
both proxies (per ADR-0107 route A). The differential diff sees `0 == 0` for
this counter on every probe; allow-listed only in the documentation sense.

## SIGTERM behavior

Phase 09 introduces no SIGTERM-related divergence; envoy-go's drain
discipline from phase 08.2 is unchanged. The fixture does NOT exercise the
drain path.

## Cross-references

- SPEC §7.1 (per-scenario equivalence claims)
- SPEC §11.1 (PGV abort.http_status validation), §11.2 (delay timing samples),
  §11.3 (4-header-set + body byte-exact), §11.5 (header-driven path deferred
  per ADR-0104), §11.6 (5-stat verification), §11.7 (wholesale-override),
  §11.8 (headers-field semantics)
- ADR-0103 (abort terminal-replace), ADR-0102 (delay async-resume),
  ADR-0107 (5-stat extension), ADR-0073 (3-tier merge), ADR-0010 (host
  networking discipline)

### Cluster type (Task 12 follow-up amendment)

The original PLAN's planner-time decision 8 claimed STRICT_DNS would work for
both the reference container and the envoy-go subject. The Task 12 smoke test
revealed that envoy-go's cluster manager only supports STATIC clusters. The
fixture's `envoy-go.yaml` was amended to `type: STATIC` with literal
`127.0.0.1`; the reference `envoy.yaml` retains STRICT_DNS +
`host.docker.internal` for Docker-network resolution. The differential parity
is preserved because both proxies dial the same backend port; only the
resolution path differs.
