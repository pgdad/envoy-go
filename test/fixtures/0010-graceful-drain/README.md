# Fixture 0010 — graceful-drain differential

This fixture asserts per-state-transition equivalence between envoy-go's
graceful-drain surface and reference Envoy v1.37.2 under a slow-streaming-
backend probe (5KB at 1KB/s = 5s in-flight window). Two driver paths in one
binary per SPEC §7 + BRAINSTORM Decision 12.

## Driver paths

1. **Admin-trigger path (against both proxies):** boot envoy-go + reference
   Envoy + slow-streaming Go HTTP backend on an allocated port; sanity-scrape
   /ready on each proxy (expect LIVE\n); start a long-lived `GET /slow`
   request on each listener; trigger drain (per-proxy script — see §11.2
   deviation note); poll /ready until DRAINING\n; scrape /server_info; attempt
   new conn (expect accept-then-FIN); wait for in-flight to complete; assert
   in-flight body byte-equal; cleanup via SIGKILL.
2. **SIGTERM-trigger path (envoy-go only):** deferred per PLAN gotcha 1 (the
   differential runner harness does not expose SIGTERM injection). Would boot
   envoy-go + backend; sanity-scrape; start in-flight; SIGTERM envoy-go; poll
   /ready until DRAINING; wait for in-flight to complete; wait for envoy-go to
   exit; assert exit status 0. Per §11.7 deviation — Envoy v1.37.2 SIGTERM is
   immediate-exit; only envoy-go has the drain-then-exit semantics.

## Per-proxy trigger script (per SPEC §7.2 + §11.2 deviation)

- envoy-go: `POST /drain_listeners` (single trigger; unifies listener drain
  and load-balancer-disposition flip per ADR-0091).
- reference Envoy: `POST /drain_listeners` + `POST /healthcheck/fail` (two
  triggers; Envoy separates listener drain from load-balancer-disposition
  flip per §11.2 finding).

## Backend shape

Minimal Go HTTP backend on a runner-allocated port. `/slow` streams 5KB at
1KB/s (5s total response time); `/` serves a fast 200 OK + `backend1\n` for
sanity. Per SPEC §7.5.

## Cross-references

- SPEC §7 (differential fixture); §11 (empirical pins)
- ADR-0091 (drain state machine); ADR-0093 (/drain_listeners contract);
  ADR-0094 (listener accept-then-FIN); ADR-0097 (/ready DRAINING);
  ADR-0098 (/server_info DRAINING)
- BEHAVIOR_CONTRACT.md ## Graceful drain (the contract umbrella the fixture
  exercises)
- planner-time decision 8 (framework reuse: shared boot helpers; per-state-
  transition byte-equality NOT shared with 0009's structural-projection)
