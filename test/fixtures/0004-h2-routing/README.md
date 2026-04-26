# Fixture 0004 — HTTP/2 routing (HCM AUTO + ALPN h2 + upstream H/2 + TLS termination + TLS origination)

**Purpose:** end-to-end exercise the phase-05.2 dataplane (HCM(AUTO) + route match + router → upstream-H/2 client codec + upstream-TLS validation) and prove byte-equivalent decoded response bodies + per-side RR distribution + status-code equivalence between upstream Envoy and envoy-go on a 27-request workload, all over TLS-terminated downstream H/2 + TLS-originated upstream H/2.

**Differential surface:** concatenated decoded response bodies for the 9 `/health` direct_response requests (`"OK\n"` x 9) are byte-equivalent. The `/api/v1/<n>` router-action bodies are NOT concatenated into the diff stream (RR-pick ordering may diverge between STATIC and STRICT_DNS); routing correctness is covered by per-side `[3,3,3]` distribution + status-200. The 404 catch-all bodies are NOT compared (envoy-go: `not found\n`; Envoy: HTML/JSON local reply per its default config).

**Local-correctness surface:** each proxy's per-cluster accept counts over the 9 router-action requests must be exactly `[3, 3, 3]` (RR witness per ADR-0028's `--concurrency 1` reference pin). Counts are derived from parsing the response body's `"backend-<idx>:"` prefix — the H/2 backend is a subprocess, so the runner's in-process accept counters do NOT see these requests; the driver implements `DistributionAsserter` from the response bytes instead.

**Topology:**

```
client (test, H2RoundTrip helper, fresh dial per request)
    ──TLS+ALPN h2──> [subj listener 127.0.0.1:<subjPort>] ──HCM(AUTO)──> direct_response | router → STATIC c_h2_backend → TLS+h2 → 127.0.0.1:<bN>
    ──TLS+ALPN h2──> [container-mapped <hostPort>]        ──Envoy──>     HCM(AUTO) → direct_response | router → STRICT_DNS c_h2_backend → TLS+h2 → host.docker.internal:<bN>
```

The 3 backends are subprocesses spawned from `test/fixtures/0004-h2-routing/backends/main.go` (one per index 0/1/2; `BACKEND_IDX` env var supplies the idx that flows into response bodies). They listen on TLS with `NextProtos=["h2"]` + `http2.ConfigureServer` driver-side (D-3.2 governs envoy-go runtime, not test backends).

**Routes (declaration order; first-match-wins):**

1. `match.path: "/health"` → direct_response 200 `"OK\n"`
2. `match.prefix: "/api"` → router → cluster `c_h2_backend` (3 endpoints, RR)
3. `match.prefix: "/"` → direct_response 404 `"not found\n"` (explicit catch-all)

**Driver request schedule (27 requests per side):**

- 9 × `GET /health` → expect 200, body `"OK\n"` (concatenated into the byte stream)
- 9 × `GET /api/v1/<n>` for n=0..8 → expect 200, body `"backend-<idx>:v1/<n>"` (not concatenated; distribution counted)
- 9 × `GET /missing/<n>` for n=0..8 → expect 404 (body NOT concatenated)

**STATIC vs STRICT_DNS divergence (ADR-0027 inherited):** subject is host-side STATIC; reference is container-side STRICT_DNS with `dns_lookup_family: V4_ONLY` per ADR-0010.

**`--concurrency 1` reference pin (ADR-0028 inherited):** keeps RR distribution deterministic on the reference side.

**Per-request fresh dial (ADR-0039 inherited; settled SPEC §10 #13 H/2):** the driver's `H2RoundTrip` helper creates a fresh `*http2.Transport` + `*http2.ClientConn` per call (no caching), so each request consumes one RR slot end-to-end.

**ALPN-h2 e2e shape:** the driver advertises `NextProtos=["h2"]` only; the listener offers `["h2","http/1.1"]` (so HCM `codec_type: AUTO` selects H/2). Upstream cluster's `transport_socket.alpn_protocols=["h2"]` plus `typed_extension_protocol_options.HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` pins the upstream codec to H/2 (per ADR-0056 — `Cluster.UseH2()`).

**ADR-0057 closure of ADR-0035 H/2 leg:** ADR-0035 carved out the `phase-05` H/2 expansion. Phase 05.1 settled the downstream H/2 leg via the H/2 server codec + h2spec gate. This fixture (with its driver landing at Task 14) closes the upstream H/2 leg by exercising the full chain end-to-end on a non-trivial workload: HCM(AUTO) → router → upstream H/2 client codec → 3 TLS h2 backends. ADR-0057 lands at Task 14 alongside the driver registration.

**Header allow-list (ADR-0044 inherited):**

`date`, `server`, `content-type`, `content-length`, `transfer-encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id` — values not compared.

**Per-side `[3,3,3]` RR rule:** the 9 `/api/v1/<n>` requests on a 3-endpoint RR cluster must distribute exactly 3 to each backend (subject + reference, independently). The driver counts by parsing `"backend-<idx>:"` prefixes from decoded response bodies.

**PKI regeneration procedure:**

```bash
cd test/fixtures/0004-h2-routing
go run ./pki/gen
git diff --exit-code pki/ && echo ok   # expect: ok (deterministic)
```

The committed PEMs are the authoritative source. CI does NOT run the generator. Re-run only to rotate (and update the `notBefore` / `notAfter` constants in `pki/gen/main.go`).

**Run locally (will become available once Task 14 lands the driver):**

```bash
go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -v -timeout=10m
```

Until then, the fixture content (this directory) is committed; the runner does not yet pick it up because no driver package is blank-imported in `test/differential/runner_test.go`.

**Re-baseline:** per ADR-0008 §"refresh procedure". If upstream Envoy's pin bumps and the gate fails on the response-body concatenation, supersede the failing ADR if the bytes change.
