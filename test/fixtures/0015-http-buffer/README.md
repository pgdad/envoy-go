# Fixture 0015 — `envoy.filters.http.buffer` differential equivalence

Six scenarios per phase 13 SPEC §7.1; sequential against a single listener `l_main`
with three routes (`/` default + `/route-disabled` per-route TPFC + `/route-tighter`
per-route TPFC). Reference Envoy v1.37.2 (STRICT_DNS) vs envoy-go (STATIC).

## Scenarios

1. **Body fits listener cap** — `POST / body=1 KiB (CL-known)` → 200; `downstream_rq_2xx +1`.
2. **Streaming overflow CL-known** — `POST / body=2 MiB (CL-known); Expect: 100-continue` → 100-Continue (interim, stripped by client) + 413 `Payload Too Large` + 4-header set + `connection: close`; `downstream_rq_4xx +1`.
3. **Chunked overflow per-route tighter** — `POST /route-tighter Transfer-Encoding: chunked body~=200 KiB (exceeds 128 KiB override)` → 413 (NO 100-Continue with chunked); `downstream_rq_4xx +1`.
4. **Per-route disabled bypasses cap** — `POST /route-disabled body=2 MiB (exceeds listener 1 MiB)` → 200; filter inactive; `downstream_rq_2xx +1`.
5. **Per-route tighter override fires CL-known** — `POST /route-tighter body=200 KiB (CL-known; exceeds 128 KiB override)` → 100-Continue + 413; `downstream_rq_4xx +1`.
6. **Chunked-passthrough Content-Length injection** — `POST / Transfer-Encoding: chunked body=10 KiB` → 200; backend asserts inbound `Content-Length: 10240` (NOT chunked). Cross-cutting assertion for `maybeAddContentLength` mirror; `downstream_rq_2xx +1`.

(Total: `downstream_rq_total +6`, `downstream_rq_2xx +3`, `downstream_rq_4xx +3`.)

## Single-listener bootstrap discipline (per planner-time decision 6)

All scenarios run against the same listener (single boot; no per-scenario teardown).
Driver issues all 6 requests sequentially in one `DriveReference` / `DriveSubject` call.
Buffer is purely synchronous — no timing tolerances.

## Topology

- **Listener `l_main`:** HTTP/1.1 plaintext; three routes.
- **Route `/`:** default; inherits listener-level `Buffer` (1 MiB cap per ADR-0126).
- **Route `/route-disabled`:** per-route TPFC `disabled: true` (filter wholly inactive per ADR-0125).
- **Route `/route-tighter`:** per-route TPFC `buffer.max_request_bytes: 131072` (128 KiB override; tighter than listener).
- **Cluster `c_backend`:** HTTP/1.1 echo backend serving inbound request headers as JSON in response body (per planner-time decision 9; load-bearing for scenario 6 backend assertion per D4 settlement).

## `max_request_bytes ≤ 1 MiB` envoy-go-only parse-time validation (per ADR-0126)

Reference Envoy v1.37.2 accepts arbitrary `UInt32Value` up to ~4 GiB at parse time.
envoy-go rejects values ≤ 0 or > 1048576 (1 MiB) with envoy-go-own error wording
(`"buffer: max_request_bytes (%d) exceeds envoy-go cap of 1048576 bytes"`).
This fixture does NOT exercise the divergence window (`max_request_bytes > 1 MiB`);
all configured values are within the cap (listener 1 MiB; per-route 128 KiB).

## `maybeAddContentLength` chunked → fixed-CL conversion (per ADR-0127 v2 + SPEC §11.8-CL)

Scenario 6 is the cross-cutting assertion: inbound chunked `Transfer-Encoding: chunked`
request (no Content-Length header) is accumulated by the buffer filter, then `maybeAddContentLength`
is invoked on terminal `endStream=true` to inject `Content-Length: <accumulated>` and drop
`Transfer-Encoding: chunked`. The backend receives the fixed-CL form and echoes it in the
JSON response body; the driver asserts the inbound header equals `"10240"`. This mirrors
`buffer_filter.cc:91-97` (reference Envoy behavior verified empirically at SPEC §11.8-CL).

## Per-route disabled-OR-override 5th canonical discipline (per ADR-0125)

Phase 13 introduces the 5th canonical per-route shape: `BufferPerRoute` is a separate
proto from listener-level `Buffer`, carrying a `oneof override` with two cases:
- **`disabled: true`** — filter wholly inactive; `DecodeHeaders` returns `HeadersContinue` immediately;
  framework safety-net cap never engages; request body passes through untouched (scenario 4).
- **`buffer.max_request_bytes`** — override (wholesale data-only); accumulation cap fires at the override value
  instead of listener-level (scenarios 3 + 5). SHARED-vacuous stats (no filter-specific counters;
  HCM-level `downstream_rq_4xx` increments count both listener-level and per-route overflows).

## 100-Continue addendum (per SPEC §11.8 + planner-time decision 10)

Scenarios 2 and 5 (CL-known requests with body size > some threshold) emit `HTTP/1.1 100 Continue`
as an interim response before the eventual 413. This is HCM/H1-codec discipline, not buffer-filter
discipline (per SPEC §11.8 amendment). Go's `http.Client.Do` transparently strips 1xx interim
responses, so the driver's status assertion (`resp.StatusCode == 413`) compares the final response.
Scenarios 3 and 6 (chunked paths) do NOT emit 100-Continue (chunked bypasses `Expect:` handling).
The driver makes no explicit 100-Continue assertions; the wire-shape is allowed-but-not-required,
matching reference Envoy's HCM/H1-codec behavior.

## Planner-time decisions cross-references

Phase 13 PLAN § "Planner-time deferred-decision resolution" settles eleven decisions (D1–D4
from SPEC §12; seven PLAN-emerging D5–D11). This fixture exercises the following:

- **D1 (Filter-callback wiring):** Decoder-only `HTTPFilter` value; `SetDecoderCallbacks(cb)` per cors/fault/csrf precedent.
- **D3 (Error wording):** envoy-go's own clear-text validation messages (e.g., "buffer: max_request_bytes (%d) exceeds envoy-go cap").
- **D4 (Backend assertion):** JSON-echo backend serving inbound headers; driver parses and asserts scenario 6's `content-length` field.
- **D5 (`HTTPFilter` Encoder: nil):** Decoder-only per phase 12 csrf ADR-0120 precedent.
- **D6 (Single-listener topology):** Three routes on one listener; all 6 scenarios sequential without teardown.
- **D8 (Chunked body construction):** `req.TransferEncoding = []string{"chunked"}` + `bytes.NewReader(data)` Go stdlib idiom.
- **D9 (Backend JSON-echo):** Echoes inbound request method + path + headers as JSON object in response body.
- **D10 (100-Continue handling):** Go stdlib `http.Client.Do` strips 1xx; driver sees final response only (no explicit 100-Continue code).

## Envoy deviation

None — buffer is a normal HTTP filter. No SIGTERM/drain divergence; no special HCM wiring.
Per-route TPFC handling is the existing 3-tier `Resolve` per ADR-0073 (most-specific-override).

## Stat surface

No new buffer-filter-specific counters (per SPEC §1.1 amendment 5 — phase 13 contributes
ZERO entries to the 29-name stat-table). Buffer overflow is observable via the existing
in-table HCM counter `downstream_rq_4xx` (incremented by HCM framework after `SendLocalReply(413)`).
Envoy-only `downstream_rq_too_large` (+3) and `downstream_rq_completed` (+6) are filtered out
via the existing twin-series-discipline allow-list (per BEHAVIOR_CONTRACT.md twin-series note).
