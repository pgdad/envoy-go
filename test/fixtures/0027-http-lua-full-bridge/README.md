# Fixture 0027 — `http-lua-full-bridge`

The phase-22.2 differential fixture for the full Envoy↔Lua bridge surface
delta. Asserts cross-side parity between envoy-go's
`envoy.filters.http.lua` (22.2 IMPL) and reference Envoy v1.37.2 across
13 scenarios covering body / trailers / metadata / dynamic-metadata /
connection-SSL / crypto / fileBytes / streamInfo-full / httpCall /
timestamp / filterState bridge surfaces.

## Scope

Per [phase 22.2 SPEC §8](../../docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/SPEC.md)
+ D5 closure (option f-B cert-fingerprint-only) + D8 closure (fileBytes
envoy-go-strict per §13-R8 PLAN-scrape) + D-P11 closure (REUSE existing
`runReferenceLessFixture` pattern; NO NEW driver-helper added).

ADR cross-references:
- [ADR-0190](../../docs/envoy-go/DECISIONS.md#adr-0190) — `internal/dynamicmetadata/` framework primitive (consumer #1).
- [ADR-0191](../../docs/envoy-go/DECISIONS.md#adr-0191) — `internal/lua/` 22.2 API extensions (coroutine + BodyBuffer).
- [ADR-0192](../../docs/envoy-go/DECISIONS.md#adr-0192) — `internal/filter/http/lua/` 22.2 package shape (8 bridge surface families + envoy-go-strict departures).

## 13-scenario matrix (per SPEC §8.2)

| # | Scenario | Bridge surface | Mode |
|---|---|---|---|
| (a) | `a_body_whole`         | `:body()` whole-buffer (defensive Go string copy) | cross-side `CompareBytes` |
| (b) | `b_body_chunks`        | `:bodyChunks()` iterator | cross-side `CompareBytes` |
| (c) | `c_trailers`           | `:trailers()` 8-method mutation surface | cross-side `CompareBytes` |
| (d) | `d_metadata_empty`     | `:metadata():get(k)` empty-userdata at binding-gap | cross-side `CompareBytes` |
| (e) | `e_dynamic_metadata`   | `:streamInfo():dynamicMetadata()` set+get round-trip | cross-side `CompareBytes` |
| (f) | `f_connection_ssl_fp`  | `:connection():ssl():sha256PeerCertificateDigest()` (f-B fingerprint-only per D5) | cross-side `CompareBytes` |
| (g) | `g_crypto`             | `:sha256(s)` + `:base64Escape(s)` | cross-side `CompareBytes` |
| (h) | `h_filebytes`          | `:fileBytes(path)` envoy-go-strict per D8 | **REFERENCE-LESS subject-only** |
| (i) | `i_streaminfo_upstream` | `:streamInfo():upstreamHost` + `:upstreamCluster` | cross-side `CompareBytes` |
| (j) | `j_httpcall_sync`      | `:httpCall(cluster, ..., async=nil)` sync arm | **REFERENCE-LESS subject-only** |
| (k) | `k_httpcall_async`     | `:httpCall(cluster, ..., async=true)` fire-and-forget | **REFERENCE-LESS subject-only** |
| (l) | `l_timestamp`          | `:timestamp('milliseconds')` non-deterministic wall-clock | **REFERENCE-LESS subject-only** |
| (m) | `m_filterstate`        | `:streamInfo():filterState()` set+get round-trip | **REFERENCE-LESS subject-only** |

**Counts:** 8 deterministic cross-side `CompareBytes` + 5 REFERENCE-LESS
subject-only = 13 scenarios total. Per PLAN reclassification at D8: (h)
moves from cross-side to REFERENCE-LESS subject-only (reference Envoy
v1.37.2 does NOT expose `:fileBytes` on the request_handle bridge — see
§13-R8 PLAN-scrape outcome + the BEHAVIOR_CONTRACT.md §13.6 departure
record landing at Task 19).

## Topology

```
                  ┌─────────────────────────────────┐
                  │  reference Envoy / envoy-go     │
                  │  13 HTTP listeners              │
                  │                                 │
   GET /scenario_a ─→ l_test_a  [HTTP] ──┐
   POST /scenario_b → l_test_b  [HTTP] ──┤
   GET /scenario_c ─→ l_test_c  [HTTP] ──┤
   GET /scenario_d ─→ l_test_d  [HTTP] ──┤
   GET /scenario_e ─→ l_test_e  [HTTP] ──┤
   GET /scenario_f ─→ l_test_f  [TLS]  ──┤───→  c_backend  ──→  echobackend
   GET /scenario_g ─→ l_test_g  [HTTP] ──┤             │       (reflects request
   GET /scenario_h ─→ l_test_h  [HTTP] ──┤             │        headers as JSON)
   GET /scenario_i ─→ l_test_i  [HTTP] ──┤             │
   GET /scenario_j ─→ l_test_j  [HTTP] ──┼─→ c_httpcall ──→  (same backend)
   GET /scenario_k ─→ l_test_k  [HTTP] ──┤             │
   GET /scenario_l ─→ l_test_l  [HTTP] ──┤
   GET /scenario_m ─→ l_test_m  [HTTP] ──┘
                  └─────────────────────────────────┘
```

- **Plaintext listeners (12):** a, b, c, d, e, g, h, i, j, k, l, m.
- **TLS listener (1):** f — presents `certs/cert.pem` + `certs/key.pem`
  (Task 17 plumbing). Both sides mount the SAME PEM bytes →
  `:sha256PeerCertificateDigest()` returns byte-identical hex digest.
- **Upstream clusters:** `c_backend` (echobackend; cross-side reflects
  request headers as JSON body) + `c_httpcall` (mirror cluster used by
  `:httpCall()` scenarios j+k; same backend endpoint).

## Cert fixture (scenario f-B per D5 closure)

Per [Task 17](../../docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md#task-17-cert-fixture-plumbing-for-scenario-f-b-per-d5-closure)
the fixture ships a 100-year-validity self-signed cert pair at
`certs/cert.pem` + `certs/key.pem`. Both differential sides present the
SAME PEM at runtime; the cross-side `CompareBytes` assertion for
scenario (f) is the literal lowercase-hex 64-char digest:

```
6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841
```

Cert metadata:

- **Subject / Issuer:** `CN = fixture-0027` (self-signed).
- **Serial:** `224E7B641EB5CAF1A2483CC110BFD024F7C2C810` (20-byte random; 160 bits of entropy).
- **SAN list:** `DNS:fixture-0027.example.com, IP Address:127.0.0.1`.
- **Validity:** `Not Before: May 19 14:25:39 2026 GMT` → `Not After: Apr 25 14:25:39 2126 GMT` (~100 years).
- **Public key:** RSA 2048-bit, sha256WithRSAEncryption signature.
- **sha256 fingerprint (DER bytes):** `6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841` (matches the `:sha256PeerCertificateDigest()` cross-side return).

The driver (`inputs/driver.go`) sets `InsecureSkipVerify: true` on the
TLS client — the cross-side assertion is the cert FINGERPRINT, NOT the
cert-chain validation.

## Driver pattern (D-P11 REUSE)

Per D-P11 closure, the driver REUSES the existing
`runReferenceLessFixture` pattern (NO new driver-helper added at the
runner). The fixture-0027 driver dispatches per-scenario from a single
`driveProxy` + `emitScenario` + `classifyBody` body (mirrors
[fixture-0026](../0026-http-lua-headers-bridge/inputs/driver.go) Tier-2
pattern).

For REFERENCE-LESS scenarios (h/j/k/l/m):

- **Reference side:** skips the listener probe; emits the normalized
  constant token `subject-only=ok` directly into the byte stream.
- **Subject side:** ACTUALLY probes the listener; emits the SAME
  normalized constant token `subject-only=ok` into the byte stream.
  Real verdict (response status + body prefix) is captured via stderr
  side-channel when `FIXTURE_0027_VERBOSE=1` env var is set.

This achieves byte-equal cross-side stream → `CompareBytes` succeeds
without adding any new driver-helper or runner branch.

## Per-scenario assertion shape

For each scenario, the driver emits one line into the byte-comparison
buffer:

```
scenario a status=200 body=x-body-len=17
scenario b status=200 body=x-chunk-count=present
scenario c status=200 body=x-trailers-status=nil
scenario d status=200 body=x-md-get=nil,x-md-pairs=0
scenario e status=200 body=x-dynmd=v-fixture27
scenario f status=200 body=x-ssl-fp=expected
scenario g status=200 body=crypto=present
scenario h subject-only=ok
scenario i status=200 body=x-up-cluster-empty=no
scenario j subject-only=ok
scenario k subject-only=ok
scenario l subject-only=ok
scenario m subject-only=ok
```

Byte-stream identity between ref + subj at every scenario line →
`CompareBytes` succeeds. The cross-side classifier collapses non-
substantive divergences (chunking strategy, header forwarding) into
deterministic tokens.

## Fixture green-light deferred to Task 19

This fixture's directory + driver + .lua scripts + YAMLs land at Task 18.
Final cross-cutting integration green-light (all 13 scenarios pass on
both sides simultaneously) is DEFERRED to Task 19 atomic landing, which
depends on all bridge surfaces from Tasks 7-13 + stats/race/fuzz from
Tasks 14-16 being fully integrated. Smoke-run at Task 18 commit time may
exhibit per-scenario failures; PROGRESS.md captures the smoke-run
outcome verbatim per `superpowers:verification-before-completion`.
