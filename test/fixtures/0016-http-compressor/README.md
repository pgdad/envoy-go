# Fixture 0016 — `envoy.filters.http.compressor` differential equivalence

Six scenarios per phase 14 SPEC §7.1; sequential against a single listener `l_main`
with six routes (`/text-html-1024`, `/image-png-1024`, `/text-html-10`,
`/text-html-etag-strong`, `/per-route-disabled`, `/per-route-rmae`).
Reference Envoy v1.37.2 (STRICT_DNS) vs envoy-go (STATIC).

## Scenarios

1. **Allow-compress** — `GET /text-html-1024 AE: gzip` → 200 + `content-encoding: gzip` + `vary: Accept-Encoding`; decompressed body byte-exact 1024 bytes; `response_compressed +1`, `response_total_uncompressed_bytes +1024`, `response_total_compressed_bytes +<gzipped>`, `header_compressor_used +1`.
2. **Skip — content-type mismatch** — `GET /image-png-1024 AE: gzip` → 200; NO `content-encoding`/`vary`; `content-length: 1024`; identity body byte-exact; `response_not_compressed +1`, `header_compressor_used +1`. (`image/png` not in default 8-entry content-type list per §11.1.)
3. **Skip — below min_content_length** — `GET /text-html-10 AE: gzip` → 200; NO `content-encoding`/`vary`; `content-length: 10`; identity body byte-exact; `response_not_compressed +1`, `response_content_length_too_small +1`, `header_compressor_used +1`. (10 bytes < default `min_content_length=30` per §11.9.)
4. **Strong-ETag-strip + compressed-passthrough (mode-a)** — `GET /text-html-etag-strong AE: gzip` → 200 + `content-encoding: gzip` + `vary: Accept-Encoding` + NO `etag` header; decompressed body byte-exact 1024 bytes; `response_compressed +1`, totals, `header_compressor_used +1`. (`disable_on_etag_header=false` default → strip-but-still-compress per §1.1 amendment 6 mode-a.)
5. **Per-route disabled** — `GET /per-route-disabled AE: gzip` → 200; NO `content-encoding`/`vary`; `content-length: 1024`; identity body byte-exact; NO counter increments (filter wholly inactive — `header_compressor_used` does NOT fire on disabled route per bucket 1).
6. **Per-route remove_accept_encoding_header override** — `GET /per-route-rmae AE: gzip` → 200 + `content-encoding: gzip` + `vary: Accept-Encoding`; decompressed body is the echobackend JSON `{method, path, headers}`; driver asserts NO `"accept-encoding"` key in echoed map; `response_compressed +1`, totals, `header_compressor_used +1`.

(Total cross-scenario deltas: `response_compressed +3`, `response_not_compressed +2`, `response_content_length_too_small +1`, `header_compressor_used +5`, `not_compressed_etag +0`.)

## Single-listener bootstrap discipline (per planner-time decision 9)

All 6 scenarios run against the same listener `l_main` (single boot; no per-scenario
teardown). Driver issues all 6 requests sequentially in one `DriveReference` /
`DriveSubject` call. The compressor is purely synchronous — no timing tolerances.

## Topology

- **Listener `l_main`:** HTTP/1.1 plaintext; six routes; listener-level `Compressor` filter BEFORE `envoy.filters.http.router` in the HCM filter chain.
- **Listener-level Compressor:** `compressor_library{name: text_optimized, typed_config: ...Gzip}` + `response_direction_config: {}` (all defaults: `enabled=true`, `min_content_length=30`, `content_type=<8-entry default list>`, `disable_on_etag_header=false`, `remove_accept_encoding_header=false`, `uncompressible_response_codes=[]`).
- **Route `/text-html-1024`:** `direct_response` 200; 1024-byte body of "A"s; `content-type: text/html`.
- **Route `/image-png-1024`:** `direct_response` 200; 1024-byte body of "A"s; `content-type: image/png`.
- **Route `/text-html-10`:** `direct_response` 200; 10-byte body `"AAAAAAAAAA"`; `content-type: text/html`.
- **Route `/text-html-etag-strong`:** `direct_response` 200; 1024-byte body of "B"s; `content-type: text/html` + `etag: "abc"` (strong-form).
- **Route `/per-route-disabled`:** `direct_response` 200; 1024-byte body of "A"s; `content-type: text/html`; per-route TPFC `CompressorPerRoute{disabled: true}` (per-route 5th canonical disabled discipline per ADR-0125 amendment §(viii)).
- **Route `/per-route-rmae`:** `cluster: c_backend` (echobackend); per-route TPFC `CompressorPerRoute{overrides: {response_direction_config: {remove_accept_encoding_header: true}}}` (per-route 5th canonical override discipline per ADR-0125 amendment §(viii)).
- **Cluster `c_backend`:** HTTP/1.1 echo backend (new shared helper at `test/helpers/echobackend/`; planner-time decision 6) serving inbound request method + path + headers as a JSON object in the response body. Used by scenario 6 for the upstream-side Accept-Encoding-absence assertion.

## Wire-shape divergence-window (per ADR-0131 §Decision (ii) + ADR-0133 §Decision (iv))

On compressed scenarios (1, 4, 6) envoy-go and reference Envoy emit different
wire shapes for the same compressed payload:

- **envoy-go:** identity `Transfer-Encoding` (omitted) + fixed `Content-Length: <gzipped-size>`. The gzip output length is known at `OverwriteBody` time per the body algorithm Path B (buffer-then-compress).
- **Reference Envoy:** `Transfer-Encoding: chunked` + NO `Content-Length`. Envoy's compressor filter emits compressed bytes incrementally via the streaming codec; the wire is chunked.

Both forms are wire-legal per RFC 7230 §3.3. The driver's cross-side header
comparison excludes `content-length` (value) and `transfer-encoding` (presence)
on those three scenarios per the per-fixture allow-list. On uncompressed
scenarios (2, 3, 5) both sides emit identity + fixed CL; no allow-list applies.
The BEHAVIOR_CONTRACT phase-14 forward-pointer notes (landed at Task 15) document
this divergence at the contract level.

## Decompress-and-compare body-assertion discipline (per ADR-0133)

Compressed-output bytes are STRUCTURALLY non-byte-exact across compressor
implementations: Go's `compress/gzip` and reference Envoy's libz produce
different headers/trailers + different deflate-stream byte sequences for the
same input plaintext (§11.14). The driver therefore asserts on the
**decompressed plaintext**, not the compressed bytes:

- **Uncompressed scenarios (2, 3, 5):** byte-exact body comparison (raw bytes).
- **Compressed scenarios (1, 4, 6):** decompress both sides via
  `compress/gzip.NewReader` + assert byte-exact equality of the resulting
  plaintexts. Optionally also assert plaintext equals the original
  pre-compression input (ADR-0133 §Decision (ii) optional invariant —
  applicable for scenarios 1 + 4 where the input is a fixed string; scenario 6's
  input is the echobackend's JSON output, which is asserted via the
  `assertNoAcceptEncodingInEchoedBody` parse).

The driver exports `assertBodyEquivalent(envoyGo, envoy *scenarioResult, originalPayload []byte) error`
+ `decompressGzip(body []byte) ([]byte, error)` per ADR-0133 §Decision (i)+(ii)
as reusable primitives for any future codec/transform fixture.

## Per-route disabled-OR-rmAE 5th canonical discipline (per SPEC §1.3 + ADR-0125 amendment §(viii))

Phase 14 introduces the 5th canonical per-route shape: `CompressorPerRoute` is
a separate proto from listener-level `Compressor`, carrying a `oneof override`
with two cases:

- **`disabled: true`** — filter wholly inactive; `EncodeHeaders` short-circuits
  with `HeadersContinue` per bucket 1; no skip-decision evaluation; no counter
  increments (NOT EVEN `header_compressor_used`); response body passes through
  untouched (scenario 5).
- **`overrides: { response_direction_config: { remove_accept_encoding_header: true } }`** —
  override (wholesale data-only); the listener-level `Compressor` proto is
  effectively replaced by the override's nested `response_direction_config` for
  the matched route. On scenario 6 this strips the inbound `Accept-Encoding`
  header from the upstream-forwarded request (DecodeHeaders path) BEFORE
  routing to `c_backend`; the echobackend echoes upstream-observed headers in
  its JSON body, so the driver can verify the strip succeeded.

## Per-route SHARED stats (per ADR-0125 amendment §(ix) + ADR-0132 §Decision (iv))

Phase 14 stats are SHARED across the listener-level `Compressor` and any per-route
`CompressorPerRoute` overrides on the same listener: a single stat tree rooted at
`compressor.<library_name>.<codec>.[response.]<counter>` per ADR-0132 namespace
(here `compressor.text_optimized.gzip.response.<counter>`). Per-route overrides
increment the SHARED counters; there is no per-route stat namespace duplication.
Counter assertions in this fixture are therefore against the single
listener-level stat tree.

## New shared `test/helpers/echobackend/` helper (per planner-time decision 6)

Scenario 6 routes to a real cluster (not a direct_response) so the per-route
`remove_accept_encoding_header` override can be observed at the upstream-side
request boundary. The shared HTTP/1.1 echo backend at
`test/helpers/echobackend/cmd/echobackend/main.go` (introduced at Task 10)
serves a JSON object `{method, path, headers}` on every request — the driver
parses this JSON and asserts `"accept-encoding"` is absent from the `headers`
map. The helper is intentionally SHARED (not per-fixture-local) because it is
reusable across future codec/transform fixtures (planner-time decision 6
departs from phase-13 buffer's per-fixture `backends/backend.go` precedent).

## `compressor_library.name: text_optimized` load-bearing for stat-namespace identity (per §11.5 + ADR-0132 §Decision (v))

The 17 emitted counters carry `<library_name>` in their flattened Prometheus
namespace (`envoy_http_compressor_text_optimized_gzip_<counter>` — Rule SN2 reuse
per ADR-0132 §Decision (i)+(iv)). Divergent library names between envoy-go and
reference Envoy would diverge the namespace and break per-counter delta
equivalence. Both sides therefore land identical `compressor_library.name:
text_optimized` per §11.5 + ADR-0132 §Decision (v). The `text_optimized` name
is illustrative-and-arbitrary at SPEC time; the actual constraint is
cross-side-byte-equality (any value works as long as both sides match).

## `max_request_bytes`-style envoy-go-only parse-time validation (per ADR-0130)

Reference Envoy v1.37.2 accepts arbitrary Gzip codec parameters; envoy-go
applies envoy-go-own clear-text validation messages per ADR-0130 §Decision (vi)
on a 4-field subset (memory_level, window_bits, compression_strategy,
compression_level). This fixture does NOT exercise the divergence window — all
codec fields are at their default values via `response_direction_config: {}` +
typed-config Gzip-default unmarshal.

## Envoy deviation

None — the compressor is a normal HTTP filter. No SIGTERM/drain divergence;
no special HCM wiring. Per-route TPFC handling is the existing 3-tier `Resolve`
per ADR-0073 (most-specific-override).

## Stat surface

17 compressor-specific counters per ADR-0132 §Decision (i)+(iv) (new entries to
the stat-table — phase 14 contributes 17 entries, the largest single-phase stat
delta in the project). All 17 are byte-exact cross-side EXCEPT
`response_total_compressed_bytes` which is boundary-only `0 < value <
sum(uncompressed input bytes)` per planner-time decision 2 settling SPEC §12
deferred 2 (compressed output diverges between Go gzip and libz).

## Planner-time decisions cross-references

Phase 14 PLAN § "Planner-time deferred-decision resolution" settles ten decisions
(D1–D4 from SPEC §12; six PLAN-emerging D5–D10). This fixture exercises:

- **D1 (Body algorithm):** Path B buffer-then-compress per ADR-0131 §Decision (i); enables `OverwriteBody` invocation with full compressed plaintext at `EncodeData(endStream=true)`.
- **D2 (Counter precision):** Boundary-only assertion on `response_total_compressed_bytes` (`0 < value < uncompressed_input_bytes`); byte-exact on the other 16 counters.
- **D6 (echobackend shared helper):** Scenario 6 routes to the shared `test/helpers/echobackend/` helper for upstream-side AE-absence assertion.
- **D7 (echobackend JSON-echo):** Backend echoes inbound request `{method, path, headers}` as JSON; driver parses + asserts `"accept-encoding"` absent.
- **D9 (Single-listener topology):** Six routes on one listener; all 6 scenarios sequential without teardown.
- **D11 (Driver shape):** Single-listener `fixture.Driver` per the buffer / fault / cors / header_mutation / csrf precedent — NOT `MultiListenerDriver` (phase 07.2).
