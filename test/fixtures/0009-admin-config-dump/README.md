# Fixture 0009 — admin-config-dump differential

This fixture asserts per-endpoint equivalence between envoy-go's four 08.1
admin endpoints (`/config_dump`, `/clusters`, `/listeners`, `/server_info`)
and reference Envoy v1.37.2 under a 5-request defined load against a STATIC
cluster with 2 endpoints. See `expectations.yaml` for the full per-endpoint
allow-list rationale. See SPEC §7 + ADR-0086 + ADR-0087 + ADR-0088 for the
authoritative contract.

## 5-request workload

The driver issues 5 sequential `GET / HTTP/1.1` round-trips against the
listener (port 10000 on subject; in-container port allocated by the runner
on reference). The 5 requests round-robin across the 2 endpoints, populating
upstream connection counters on both sides. After the load, the driver
sleeps 200ms for stats to settle, then scrapes the four admin endpoints
from each proxy.

## Per-endpoint canonicalisation

The driver's `ProbeAdmin` applies per-endpoint canonicalisation BEFORE
returning the byte stream to the runner's CompareBytes pass:

- `/config_dump` + `/server_info`: JSON parse; recursively zero allow-
  listed paths (build metadata, timestamps, uptime, command-line options,
  node.user_agent_*, node.extensions); re-marshal with sorted keys + 1-
  space indent.
- `/clusters`: line-parse into (cluster, key, value) tuples; drop the 8
  per-endpoint cx_*/rq_* counter tuples per planner-time decision 8;
  sort tuples; emit.
- `/listeners`: byte-passthrough (the runner's existing dechunk
  preprocessor handles upstream's transfer-encoding: chunked).

## Planner-time decision 8 cross-reference

envoy-go does not track per-endpoint stats (per ADR-0063 deferral); the
`/clusters` per-endpoint cx_*/rq_* counter lines emit literal `0` on the
envoy-go side. Reference Envoy emits per-endpoint observed values. The
canonicalisation drops these 8 tuples on BOTH sides so the set-equality
comparison passes.

## SPEC + ADR cross-references

- SPEC §7 (differential fixture) + §11.1–§11.4 (verbatim Envoy scrapes)
- ADR-0086 (`/config_dump` body shape)
- ADR-0087 (`/clusters` + `/listeners` body shape)
- ADR-0088 (`/server_info` body shape + state-enum coverage)
- ADR-0089 (admin-endpoint deferral list)
- ADR-0090 (no-ACL admin security posture)
- planner-time decision 8 (per-endpoint counter `0` emission;
  `expectations.yaml` documents the allow-list extension)

## Backend kind

`HTTPHello` (existing helper from fixture 0007a-cors) — backends return a
fixed body; the differential cares about admin-endpoint output, not
backend-response equivalence.
