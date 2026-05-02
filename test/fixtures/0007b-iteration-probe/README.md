# Fixture 0007b — iteration-probe (HCM + envoy_go_test + router)

**Purpose:** envoy-go-only structural fixture exercising every iteration-protocol state branch via the test-only `envoy.filters.http.envoy_go_test` probe filter (Task 19; ADR-0074). Eight sequential H1 requests, one per `x-envoy-go-test-mode` value, asserted against an embedded per-mode expectation table per SPEC §7.3.

**Why no reference Envoy:** the `envoy.filters.http.envoy_go_test` type URL is hand-rolled (`internal/filter/http/envoygotest/proto/envoygotest.pb.go`) and does NOT exist in upstream `envoyproxy/go-control-plane` or upstream Envoy. Reference Envoy would fail at typed_config parse. SPEC §7.4 codifies the split: gate (a) (differential) covers `0007a-cors`; gate (a) tier 2 (structural) covers `0007b`.

**Driver registration:** `iterationProbeDriver{}` registers via `init()` with `fixtureName = "0007b-iteration-probe"`. The driver implements `fixture.ReferenceLessFixture` returning `false`; the runner's `runReferenceLessFixture` branch spawns ONLY the subject, drives subject, and invokes `fixture.SubjectAsserter` for the in-band per-mode assertion.

**Topology:**

```
client (test) ──> [subj listener 127.0.0.1:<subjPort>]
                  ──HCM[envoy_go_test, router]──>
                  STATIC → 127.0.0.1:<backendPort>
```

Single backend instance. No reference Envoy. No Docker. No `--concurrency 1` pin (no reference to align against).

**Filter chain (declaration order):**

1. `envoy.filters.http.envoy_go_test` — test-only probe (ADR-0074; Task 19)
2. `envoy.filters.http.router` — terminal (ADR-0071; Task 11 migration)

**Per-route config (typed_per_filter_config):**

```yaml
envoy.filters.http.envoy_go_test:
  "@type": type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTestPerRoute
  count: 7
```

The probe's `EncodeHeaders` echoes `x-envoy-go-test-route-count: 7` on every response (universal encode-side branch in `internal/filter/http/envoygotest/filter.go`).

**8-mode workload (SPEC §7.3):**

| # | Mode | Method+Body | Expected wire shape |
|---|---|---|---|
| 1 | `continue` | GET (no body) | 200 + `"backend\n"` + route-count |
| 2 | `stop-and-resume-headers` | GET | 200 + `"backend\n"` + route-count (10ms async resume) |
| 3 | `stop-and-buffer-data` | POST `"payload"` | 200 + `"payload"` (echoed) + route-count (10ms async resume on data) |
| 4 | `local-reply-decode` | GET | 418 + `"i am a teapot\n"` + route-count (SendLocalReply on DecodeHeaders) |
| 5 | `local-reply-decode-data` | POST `"payload"` | 418 + `"i am a teapot\n"` + route-count (SendLocalReply on DecodeData) |
| 6 | `modify-encode-headers` | GET | 200 + `"backend\n"` + route-count + `x-envoy-go-test-encoded: yes` |
| 7 | `modify-encode-data` | GET | 200 + `"MODIFIED"` + route-count (8-byte slice; copy-truncate "MODIFIED\n" → "MODIFIED") |
| 8 | `stop-trailers` | GET | 200 + `"backend\n"` + route-count (see disposition note below) |

**Iteration-protocol state coverage attribution:**

- `Continue` headers status — modes 1, 6, 7 (decode-side pass-through)
- `StopIteration` headers status + async-resume — mode 2 (parkDecode loop yields after ContinueDecoding signal)
- `DataStopIterationAndBuffer` + async-resume — mode 3
- `SendLocalReply` from `DecodeHeaders` — mode 4 (entry into encode chain at `filter[len-1]` per ADR-0075 + SPEC §11 #4 empirical pin)
- `SendLocalReply` from `DecodeData` — mode 5
- Encode-side header set — mode 6 (`headers.Set` mutates the in-flight slice)
- Encode-side data mutation — mode 7 (`copy(data, "MODIFIED\n")` short-slice-aware)
- Per-route config 3-tier merge + lookup (`RequestRouteConfig` lazy cache) — every mode (universal route-count echo)

**Mode 8 disposition (honest):**

The probe filter's `DecodeTrailers` branch returns `TrailersStopIteration` and spawns an async resumer (filter.go lines 164-175); this is exercised at unit-test scope by `internal/filter/http/envoygotest/filter_test.go::TestEnvoyGoTest_ModeStopTrailers` which directly drives `chain.RunDecodeTrailers`. **However**, H1 HCM dispatch (`internal/filter/hcm/connection.go`) does NOT currently invoke `chain.RunDecodeTrailers` — the H1 chunked-T-E trailer parsing was deferred at Task 15 close-out (per PROGRESS Task 15 notes; H2 observe-and-discard per ADR-0058). Mode 8's end-to-end wire shape on this fixture is therefore identical to mode 1 (`continue`); the probe's stop-trailers branch never fires on H1 traffic. This is documented honestly in `expectations.yaml`, `driver.go`'s `modeExpectations`, and the `doc.go` package comment so a future maintainer adding H1-chunked-T-E trailer parsing will rebaseline mode 8's expected behavior to a delayed-resume shape.

**Backend (`backends/main.go`):**

- `GET /` → 200 + body `"backend\n"` (8 bytes, fixed)
- `POST /` (non-empty body) → 200 + body equal to request body verbatim
- `Connection: close` set so envoy-go's keepalive upstream pool retires after each response

The 8-byte fixed body is intentionally chosen so mode 7's `copy("MODIFIED\n", "backend\n")` truncates to `"MODIFIED"` (8 bytes; the trailing `\n` of `"MODIFIED\n"` is dropped). Pinned by `internal/filter/http/envoygotest/filter.go::EncodeData`'s short-slice-aware semantics.

**Run locally:**

```bash
go test ./test/differential/ -run 'TestDifferential/0007b' -v -timeout=5m
```

Or the driver-internal unit tests:

```bash
go test ./test/fixtures/0007b-iteration-probe/...
```

**Assertion model:** in-band per SPEC §12 #8 (mirrors 05.2 / 06.1 / 06.2 / 0007a precedent). The driver's `AssertSubject` callback inspects the captured per-mode byte stream for `(status, header-presence, body)` per the embedded `modeExpectations` table; no generic data-structure extension to `fixture.Driver`. The `SubjectAsserter` interface is the new shape for the reference-less assertion path.
