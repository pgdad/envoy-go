# Fixture 0003 — HTTP/1.1 routing (HCM + route + router + direct_response)

**Purpose:** end-to-end exercise the phase-04 dataplane (HCM + route match + router + direct_response over HTTP/1.1) and prove byte-equivalent decoded response bodies + RR distribution + status-code equivalence between upstream Envoy and envoy-go on a 27-request workload.

**Differential surface:** concatenated decoded response bodies for the 18 non-404 requests (9 × `/health` direct_response + 9 × `/api/v1/<n>` router-action) are byte-equivalent. The 404 catch-all bodies are NOT compared (envoy-go: `not found\n`; Envoy: HTML/JSON local reply per its default config).

**Local-correctness surface:** each proxy's per-backend accept counts over the 9 router-action requests must be exactly `[3, 3, 3]` on the subject side (RR witness per ADR-0028's `--concurrency 1` reference pin). Reference-side counts are not asserted exactly because `host.docker.internal` DNS may rotate endpoints differently than STATIC.

**Topology:**

```
client (test) ──> [subj listener 127.0.0.1:<subjPort>] ──HCM──> direct_response | router → STATIC → 127.0.0.1:<bN>
client (test) ──> [container-mapped <hostPort>] ──Envoy──> HCM → direct_response | router → STRICT_DNS → host.docker.internal:<bN>
```

Same client driver targets both proxies; the host-side HTTP-echo backends (per Task 14's BackendKind) serve both runs with per-side counter snapshots between drives.

**Routes (declaration order; first-match-wins):**

1. `match.path: "/health"` → direct_response 200 `"OK\n"`
2. `match.prefix: "/api"` → router → cluster `c_backend` (3 endpoints, RR)
3. `match.prefix: "/"` → direct_response 404 `"not found\n"` (explicit catch-all per SPEC §10 #5 settled choice)

**Driver request schedule (27 requests per side):**

- 9 × `GET /health` → expect 200, body `"OK\n"` (concatenated into the byte stream)
- 9 × `GET /api/v1/<n>` for n=0..8 → expect 200, body `"backend-<idx>:v1/<n>"` (concatenated; `<idx>` is whichever backend served)
- 9 × `GET /missing/<n>` for n=0..8 → expect 404; body NOT concatenated (Envoy/envoy-go local-reply bodies diverge)

**STATIC vs STRICT_DNS divergence (ADR-0027 inherited):** subject is host-side STATIC; reference is container-side STRICT_DNS with `dns_lookup_family: V4_ONLY` per ADR-0010.

**`--concurrency 1` reference pin (ADR-0028 inherited):** keeps RR distribution deterministic on the reference side.

**Per-request fresh dial (ADR-0039):** subject's router action does not pool upstream connections. The driver's `Connection: close` per request preserves the per-request endpoint-pick semantic (each request takes one RR slot).

**Header allow-list (ADR-0044):**

`date`, `server`, `content-length`, `transfer-encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id` — values not compared.

**Run locally:**

```bash
go test ./test/differential/ -run 'TestDifferential/0003-http11-routing' -v -timeout=10m
```

**Re-baseline:** per ADR-0008 §"refresh procedure". If upstream Envoy's pin bumps and the gate fails on the response-body concatenation, supersede the failing ADR if the bytes change.
