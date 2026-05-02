# Fixture 0007a — cors filter (HCM + cors + router)

**Purpose:** end-to-end exercise the phase-07.1 HTTP filter chain framework's first real filter (`envoy.filters.http.cors`) and prove byte-equivalent CORS response shape between upstream Envoy v1.37.2 and envoy-go on a 4-request workload covering the §11.2 empirical-pin matrix (preflight allowed/disallowed + actual-request allowed/no-origin). This is the project's first differential anchor for the cors filter and the `typed_per_filter_config` 3-tier per-route config model (ADR-0071/0073).

**Differential surface:** per-request status + body + alphabetically-sorted CORS-header set. Bodies are %q-quoted into the Drive byte stream so trailing-newline / non-printable divergences surface as wire-equality failures rather than silent string-strip mismatches. Non-CORS response headers (Date, Server, Content-Length, Content-Type, x-envoy-*, x-request-id) are omitted from the byte stream and silently allow-listed; the runner's `helpers.PhaseFourHTTPAllowList` already covers them.

**Filter chain:** `[envoy.filters.http.cors, envoy.filters.http.router]` on both sides; the cors filter is the project's first non-terminal HTTP filter and the router is the migrated terminal filter from phase-04 (`internal/filter/http/router/`). The cors filter operates on both decode (preflight detection + SendLocalReply on allowed-origin preflight) and encode (3-header injection on allowed-origin actual requests) sides.

**Topology:**

```
client (test) ──> [subj listener 127.0.0.1:<subjPort>] ──HCM[cors,router]──> STATIC → 127.0.0.1:<backendPort>
client (test) ──> [container-mapped <hostPort>] ──Envoy──> HCM[cors,router] → STRICT_DNS → host.docker.internal:<backendPort>
```

Single backend instance (no RR exercised); body is fixed `"hello\n"` (6 bytes).

**Routes (declaration order; first-match-wins):**

1. `match.path: "/permissive"` → `route { cluster: c_backend }` with `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{allow_origin_string_match: [exact: "https://example.test"], allow_methods: "GET, POST, OPTIONS", allow_headers: "x-foo, x-bar", max_age: "600", expose_headers: "x-baz", allow_credentials: true}`.
2. `match.path: "/strict"` → `direct_response 405 "method not allowed\n"` with `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{allow_origin_string_match: [exact: "https://only.test"]}` (very restrictive; no test origin matches).

**Driver request schedule (4 requests per side; SPEC §11.2 probes a/b/c/d):**

| # | Request | Origin | ACR-Method | Expected behavior |
|---|---|---|---|---|
| 1 | `OPTIONS /permissive` | `https://example.test` | `POST` | cors filter SendLocalReply 200 + 6 access-control-* preflight headers (per §11.2 probe a) |
| 2 | `OPTIONS /strict` | `https://other.test` | `POST` | cors no-op (origin doesn't match `only.test`); falls through to direct_response 405 |
| 3 | `GET /permissive` | `https://example.test` | (n/a) | backend 200 + body `"hello\n"`; cors encode-side injects 3 access-control-* headers (per §11.2 probe c) |
| 4 | `GET /permissive` | (none) | (n/a) | backend 200 + body `"hello\n"`; cors no-op (no Origin); zero CORS headers (per §11.2 probe d) |

**Per-route cors config differential exercise:** the 4-request workload exercises all three cors decode-side branches:

- `originAllowed=true, isPreflight=true`  → SendLocalReply path (request 1).
- `originAllowed=false, isPreflight=true` → passthrough to router/direct_response (request 2).
- `originAllowed=true, isPreflight=false` → encode-side header injection (request 3).
- no Origin                                → cors filter is fully no-op (request 4).

**STATIC vs STRICT_DNS divergence (ADR-0027 inherited):** subject is host-side STATIC; reference is container-side STRICT_DNS with `dns_lookup_family: V4_ONLY` per ADR-0010.

**`--concurrency 1` reference pin (ADR-0028 inherited):** keeps response shape deterministic on the reference side.

**CORS header set-equality (not byte-equality on order):** the actual-request 3-header path (request 3) on the envoy-go subject side emits the 3 headers AFTER the original upstream carrier in alphabetical order (per `internal/filter/http/types.go ReconcileOrderedHeaders`); reference Envoy v1.37.2 emits them in source-order (allow-origin, allow-credentials, expose-headers). Byte-equality on the wire is therefore NOT achievable for the actual-request path; **set-equality on header NAMES + per-name value byte-equality IS achievable and is what this fixture asserts** (the driver sorts the cors-* headers alphabetically before encoding into the Drive byte stream). This is the (b) fallback per Task 21 prompt: ADR-0071's filter API stability is preserved over actual-request byte-equality. The trade-off is documented in PROGRESS Task 19 close-out + Task 21 entry.

**Header allow-list (ADR-0044):**

`date`, `server`, `content-length`, `transfer-encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id` — these are omitted from the Drive byte stream entirely (only `access-control-*` headers are emitted), so they are silently allow-listed by construction.

**PLAN deviation rationale:**

- **Request 4 path: `/permissive` (not `/strict`).** The PLAN brief says request 4 is `GET /strict` no-Origin → 200 + body. With `/strict` being direct_response 405 (necessary for request 2's 405 — see next bullet), `GET /strict` would produce 405 not 200. We use `GET /permissive` no-Origin instead, which (a) preserves the 4-request matrix, (b) exercises the same cors no-op-on-no-Origin branch, and (c) directly mirrors SPEC §11.2 probe (d) which uses the same route as probes (a) and (c).
- **`/strict` 405 via direct_response (not router fallthrough).** The PLAN brief assumes envoy-go's router 405s OPTIONS by default the way reference Envoy v1.37.2 does empirically in §11.2 probe (b). envoy-go's router (`internal/filter/http/router/router.go`) does not implement this behavior — phase-04's `matchPath` / `matchPrefix` vocabulary (`internal/filter/hcm/route.go`) doesn't include method-restricted routes. Using `direct_response: 405` on the `/strict` route makes both proxies 405 OPTIONS /strict deterministically without depending on undocumented router-side OPTIONS handling. The cors filter's behavior under test (passthrough on disallowed-origin preflight) is preserved.

**Run locally:**

```bash
go test ./test/differential/ -run 'TestDifferential/0007a-cors' -v -timeout=10m
```

**Re-baseline:** per ADR-0008 §"refresh procedure". If upstream Envoy's pin bumps and the gate fails on cors-header values, supersede the failing ADR (likely ADR-0074) if the bytes change.
