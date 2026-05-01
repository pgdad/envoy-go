# Phase 07.1 — HTTP filter framework (`internal/filter/http/` package, iteration protocol, extension registry, per-route config, `cors` + `envoy_go_test` filters, fixtures `0007a` + `0007b`)

**Phase id:** `07.1`
**Slug:** `07.1-http-filter-framework`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (autonomous mode per ADR-0004; transcribes the brainstorm-close BRAINSTORM.md into formal SPEC shape and pins the four §2.6 empirical obligations against reference Envoy v1.37.2 — server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`)
**Depends on:** phase 06 (done at master `2c65fcc`)
**Parent phase:** `07-filter-chain-framework` (in-progress; split into `07.1` + `07.2` per ADR-0045; `07.1` does NOT close the parent — `07.2` does on its phase-done commit, mirroring the 05/05.1/05.2 + 06/06.1/06.2 closure pattern)
**Master design document:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` (this commit) and `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` (brainstorm-close artifact, master `9c3752b` parent). The BRAINSTORM is the authoritative design source; this SPEC distills BRAINSTORM §§2–9 into formal contract language and pins the §2.6 empirical obligations.
**Differential surface at end of sub-phase:** TWO new fixtures land. (1) NEW differential fixture `test/differential/0007a-cors/` is differentially green (gate (a) non-vacuous): a 2-route + 4-request workload exercising preflight (allowed/disallowed origins) + actual-request (allowed/no-origin) at both proxies; per-request response status + response header set + body byte-equal across envoy-go and reference Envoy v1.37.2 (modulo the existing differential ignore-list in `BEHAVIOR_CONTRACT.md ## Header allow-list`). (2) NEW structural fixture `test/differential/0007b-iteration-probe/` is structurally green (gate (a) tier 2): an envoy-go-only 8-request workload exercising every iteration-protocol state branch (Continue / StopIteration / StopIterationAndBuffer / async-resume / SendLocalReply on decode-headers / SendLocalReply on decode-data / encode-side modify / decode-trailers stop), each request asserted against an embedded per-mode expectation table (status + headers + body). Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log` remain green (gate (b)). h2spec conformance gate (c) re-runs at the ADR-0051 pin and stays at 53/53 PASS (07.1 touches the HCM dispatch wiring on both H1 and H2 paths but does NOT change H2 wire-codec behavior). Fuzz (d) re-runs the existing 9 fuzzers AND adds a new `FuzzFilterChainParse` over the chain-config parser at the 30s ADR-0018 budget. Build/vet/lint/test (e) and REVIEW (f) apply normally. ROADMAP row `07.1` flips `planned → in-progress` at the SPEC commit and `→ done` at the phase-done commit; the parent row `07` stays `in-progress` (closes only at 07.2's phase-done per the precedent mirror).

---

## 1. Purpose

Phase 07.1 lands envoy-go's HTTP filter chain framework — a real iteration protocol with async-resume, an extension registry, a 3-tier per-route config model, the first real Envoy filter (`cors`), and a test-only probe filter (`envoy_go_test`) that covers every iteration-protocol state branch. Concretely:

1. **A new `internal/filter/http/` package** — `StreamDecoderFilter` + `StreamEncoderFilter` interfaces, status enums (`FilterHeadersStatus`, `FilterDataStatus`, `FilterTrailersStatus`), `DecoderFilterCallbacks` + `EncoderFilterCallbacks`, `HTTPRegistry` (Register / Lookup / Freeze), `FilterChain` per-stream state machine with decode/encode iteration + buffer management + async-resume signaling, `typed_per_filter_config` 3-tier merge (RouteConfiguration / VirtualHost / Route, most-specific-override). The package is the heart of the framework: ~800-1200 LoC of new machinery (per BRAINSTORM §3 + §4), with HCM `parseFilterWithCtx` taking a `*HTTPRegistry` parameter and walking `http_filters[]` in declaration order. Two-step factory pattern: step 1 (HCM-build time) parses + validates `typed_config`; step 2 (per-request) allocates a fresh filter instance.

2. **Two new HTTP filters under `internal/filter/http/`:**
   - **`cors/`** — real Envoy filter (`envoy.filters.http.cors`); decode-side detects preflight (`OPTIONS` + `Origin` + `Access-Control-Request-Method`) and synthesizes a `204 No Content` (or `200 OK` per Envoy) preflight response via `SendLocalReply`; encode-side appends `Access-Control-Allow-Origin` / `Access-Control-Expose-Headers` / `Access-Control-Allow-Credentials` per the resolved per-route config. ~150 LoC + ~200 LoC tests.
   - **`envoygotest/`** — test-only probe filter (`envoy.filters.http.envoy_go_test`); per-request mode-dispatched branching on `x-envoy-go-test-mode` header covering 8 iteration-state modes (continue, stop-and-resume-headers, stop-and-buffer-data, local-reply-decode, local-reply-decode-data, modify-encode-headers, modify-encode-data, stop-trailers); per-route `count` config echoed into a response header (`x-envoy-go-test-route-count: N`). Hand-rolled minimal proto schema lives at `internal/filter/http/envoygotest/proto/` (does NOT extend the upstream go-control-plane registry — envoy-go-only). ~250 LoC + ~400 LoC tests.

3. **Router extracted as a real terminal filter** — the largest refactor in 07.1. The existing `routerAction` + `routerActionH2` in `internal/filter/hcm/actions.go` move to a new `internal/filter/http/router/` package. The terminal filter's `decodeHeaders` dispatches the route action (cluster dial OR direct_response synthesize). ~600 LoC of action code + tests are migrated; tests are byte-preserved per BRAINSTORM §6.8 ("imports update; package names update; test bodies byte-preserved"). `directResponseAction` stays in `hcm/actions.go` as a *route-action shape* (decided at route-match time, synthesized by the router filter when its terminal step runs).

4. **Per-route config model** — `typed_per_filter_config` honored on Route, VirtualHost, and RouteConfiguration scopes. Build-time validation: keys MUST reference filter names present in the chain's `http_filters[]`; unknown filter names error at parse with `hcm: route_config: typed_per_filter_config: unknown filter name %q (chain has [...])`. Lookup API: `DecoderFilterCallbacks.RequestRouteConfig() proto.Message` returns the merged proto (Route > VirtualHost > RouteConfiguration; most-specific override, no field-merge). Lazy cache on first lookup per request.

5. **Extension registry** — `*filter.HTTPRegistry` constructed once at boot in `cmd/envoy-go/main.go`, threaded explicitly into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`. Freeze-after-boot invariant (mirrors `*stats.Registry` LBP-1 from 06.1): `Register` post-Freeze panics; `Freeze()` is idempotent and called from `cmd/envoy-go/main.go` after all `Register` calls. Three filters registered at boot: `router.New`, `cors.New`, `envoygotest.New`.

6. **HCM-side validation tightening** — phase-04 ADR-0042's "exactly `[router]`" rule becomes "non-empty; last entry must be router" (partial supersession). Empty `http_filters[]` errors at parse with `hcm: http_filters: must contain at least 1 entry (the router)`. Last entry not router errors with `hcm: http_filters: last entry must be %q (router)`. Duplicate filter names error with `hcm: http_filters: duplicate filter name %q`. Unknown `typed_config.type_url` errors with `hcm: http_filters[i]: unknown type_url %q (registry: known are [...])`.

7. **Body buffer cap** — `filterBufferLimitBytes = 1 << 20` (1 MiB) hardcoded constant matching Envoy's default. Decode-side overflow on `StopIterationAndBuffer` synthesizes a `413 Payload Too Large` local reply that flows through the encode chain. Encode-side overflow resets the connection (H1 close, H2 RST_STREAM). The configurable knobs (`per_connection_buffer_limit_bytes` on Listener, `per_request_buffer_limit_bytes` on Route) are silently ignored at parse-time per ADR-0041 amendment.

8. **`sendLocalReply` flow** — synthesized response enters the encode chain at filter[len-1] (reverse iteration entry); first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log line `hcm: filter %q called SendLocalReply after encode-side started; ignoring`. **Empirically pinned** at SPEC time per §11 #4 below: against reference Envoy v1.37.2 with chain `[lua_a, lua_b, lua_c, router]` where `lua_b` calls `respond` (Envoy's sendLocalReply), the encode order observed is `lua_c → lua_b → lua_a` — i.e., the synthesized response enters at filter[len-1] of the encode-side filter set (router has no encode-side log-emission in the probe; the visible entry is `lua_c` at index 2 = encode-len-1).

9. **Empirically pinned obligations.** Per BRAINSTORM §2.6, the SPEC executes four empirical scrapes against reference Envoy v1.37.2 (`envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) and pins the verbatim observed behavior in §11 below: (a) filter declaration order on encode side = reverse decode order; (b) cors filter response shape (preflight + actual-request + disallowed-origin); (c) 413 overflow response shape; (d) sendLocalReply encode-chain entry filter index. **All four pins resolved IN-SESSION** — none are carried forward to impl time.

10. **Anticipated ADRs:** seven ADRs per BRAINSTORM §8, numbered ADR-0070..ADR-0076 (next-free per the `DECISIONS.md` tail at master `2c65fcc` being ADR-0069). The planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule. Topics enumerated in §8 below.

After phase 07.1, the project has proven its eighth central engineering claim (HTTP-filter-framework half): *envoy-go runs a real multi-filter HTTP iteration chain — filters can stop, buffer, resume async, modify decode/encode headers and bodies, synthesize local replies that re-enter the encode chain — and produces behaviorally-equivalent CORS responses to upstream Envoy, while a test-only probe filter exercises every iteration-protocol state to structural correctness.* The listener-chain-completion half (07.2) is delivered later; the parent ROADMAP row `07` flips to `done` at 07.2's phase-done.

## 2. Non-purposes

Phase 07.1 does **not** do any of the following. Most are explicit non-goals from BRAINSTORM §§2.1, 5.6, 9; a few are scope-narrowings the SPEC introduces by consolidating BRAINSTORM's "deferred to phase X" annotations. Each non-purpose is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Iteration-protocol non-goals (per BRAINSTORM §2.1, §5.6)

- **High/low watermark backpressure events.** No `StopAllIterationAndWatermark`, no `onAboveWriteBufferHighWatermark` / `onBelowWriteBufferLowWatermark` callbacks. Deferred to first HTTP-filter-family phase that demands them (likely a streaming-body filter like compression or buffer where stream pacing matters). → BOOTSTRAP §9 HTTP-filters family.
- **`Encode1xxHeaders` callback.** Filters cannot intercept 1xx informational responses. Deferred to first filter touching 1xx (e.g., a future `expect-continue` filter or upstream 100-Continue handling). → family phase.
- **Metadata frames.** No `decodeMetadata` / `encodeMetadata` / `MetadataMap`. Deferred to xDS-metadata family or a filter that consumes per-request metadata (e.g., `header_to_metadata`'s metadata-write side fully wired). → xDS or family phase.
- **`ContinueAndDontEndStream` status.** Deferred to first filter that needs to extend a stream past its declared end (rare). → family phase.
- **`decoderBufferLimit()` / broader callback surface.** No `StreamInfo`, `connectionLocalAddress`, `route()`, `clusterInfo()`, `dispatchTimer`, etc. on the callback shape. The MVP callback set is minimal (`ContinueDecoding`, `ContinueEncoding`, `SendLocalReply`, `RequestRouteConfig`, plus the encoder-side `EncodeHeaders/Data/Trailers` injection methods). Each addition lands an ADR in a future family phase.
- **Per-filter goroutine isolation.** A filter that spawns its own goroutine for an async lookup is the filter's own responsibility; the framework recovers panics on the dispatch goroutine but does NOT police filter-spawned goroutines. → first family phase that introduces such a filter does its own audit.

### 2.2 Per-route-config non-goals (per BRAINSTORM §2.3, §5.6)

- **Field-level merge for `typed_per_filter_config`.** 07.1 implements most-specific-override (Route > VirtualHost > RouteConfiguration), NOT Envoy's optional field-level merge mode (the `disabled` flag + partial-override semantics). Deferred to first family phase where partial-override semantics are demanded by an Envoy-equivalent test. → family phase.
- **`filter.disabled` flag.** No skip-this-filter-on-this-route override. → family phase that needs it.
- **Filter chain hot-reload.** No xDS-LDS-driven dynamic chain updates. → xDS family.

### 2.3 Filter-set non-goals (per BRAINSTORM §2.4, §9)

- **HTTP-filter family beyond `cors` + `envoy_go_test`.** No `header_manipulation`, `fault`, `jwt_authn`, `ext_authz`, `oauth2`, `csrf`, `buffer`, `lua`, `wasm`, `local_ratelimit`, `rbac`, `compression`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`. Each is its own phase under the BOOTSTRAP §9 HTTP-filters family — incrementally landed.

### 2.4 Buffer-policy non-goals (per BRAINSTORM §2.1, §5.6)

- **Configurable `per_connection_buffer_limit_bytes` (Listener-scope).** Silently ignored at parse-time; extends ADR-0041 silent-ignore set. → dedicated buffer-policy phase or first family phase that needs tuning.
- **Configurable `per_request_buffer_limit_bytes` (Route-scope).** Same disposition. → same.

### 2.5 Router-proto non-goals (per BRAINSTORM §2.1, §9; inherited from ADR-0040)

- **Router proto fields silently ignored at phase 04 (`dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`).** Each promotion lands in the phase that consumes the field; 07.1 does NOT promote any of them. The router-as-terminal-filter refactor preserves the ADR-0040-inherited silent-ignore set verbatim.

### 2.6 Listener-side non-goals (07.2's scope)

- **`listener_filters` framework.** → 07.2.
- **`FilterChainMatch` fields beyond SNI** (destination_port, prefix_ranges, source_*, application_protocols/ALPN). → 07.2.
- **`Listener.default_filter_chain`.** → 07.2.

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 07.1)

Per doctrine `D-3.6`, phase 07.1 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 07.1 |
|---|---|
| (a) new/changed differential fixtures green | **Non-vacuous (two fixtures).** (1) Differential fixture `test/differential/0007a-cors/` passes: 2 routes (`/permissive`, `/strict`) with differing per-route cors policies; 4 sequential requests across both proxies (`OPTIONS /permissive` allowed-origin → preflight 200 + CORS headers verbatim per §11 #2; `OPTIONS /strict` disallowed-origin → 405 per §11 #2; `GET /permissive` allowed-origin → 200 + body + injected `Access-Control-Allow-Origin`; `GET /strict` no-origin → 200 + body + no CORS headers); per-request status + response header set + body byte-equal across envoy-go and reference Envoy (modulo the existing differential ignore-list — `Server`, `Date`, `Content-Length` per `BEHAVIOR_CONTRACT.md ## Header allow-list`). (2) Structural fixture `test/differential/0007b-iteration-probe/` passes: 8 sequential requests at envoy-go (no reference Envoy in this fixture), one per `x-envoy-go-test-mode` value; each response asserted against an embedded per-mode expectation table (status + headers + body). |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log` all pass without regression. The 07.1 changes are additive on the chain side (router moves from a direct-call to a terminal-filter, but the wire-level behavior on existing fixtures is unchanged — the dispatch path that produces the wire bytes is what changes); HCM `NewFilter*` signatures gain a `*filter.HTTPRegistry` parameter; pre-existing fixtures' bootstraps thread an empty (or default-three-filter) registry through the constructor change; no existing fixture's behavioral expectations change. |
| (c) conformance suites pass | `test/conformance/h2spec/` re-runs at the ADR-0051 pin (`summerwind/h2spec@sha256:...` per `CONFORMANCE_PINS.md`) and reports `failed == 0` over the unchanged threshold list (sections 3, 4, 5, 6 ex-6.6, 7, 8 — 53/53 PASS at the 05.1+05.2 baseline). 07.1 doesn't touch H2 wire code; the HCM dispatch wiring change is between codec-decoded request and the route action, which is below the h2spec conformance surface. Pin is NOT bumped (D-3.7). |
| (d) new/existing fuzzers run clean for CI short-budget | Existing 8 fuzzers (`internal/bootstrap.FuzzBootstrapLoad`, `internal/filter/tcpproxy.FuzzTcpProxyFilter`, `internal/tls.FuzzTLSContextParse`, `internal/filter/hcm.FuzzHCMConfigParse`, `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode`, `internal/stats.FuzzPromTextFormat`, plus 06.2's access-log-format fuzzer) run clean at the 30s ADR-0018 budget. **NEW:** `internal/filter/http.FuzzFilterChainParse` runs clean at the same budget. Total: 9 fuzzers post-07.1. |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for the new `internal/filter/http/` package (registry / chain / per-route / cors / envoygotest / router; race-clean; freeze-after-boot enforcement); extended tests for `internal/filter/hcm/` (chain-integration in connection.go + h2dispatch.go; modified actions.go; modified config.go for chain-config parse); `go test -race ./...` clean — N goroutines calling `ContinueDecoding`/`ContinueEncoding` on the same chain (idempotent + coalesced); concurrent `HTTPRegistry.Lookup` calls from N HCM constructors at boot; per-request chain teardown vs an in-flight `ContinueEncoding` from a slow filter; one filter's timer goroutine vs the dispatch goroutine racing on `cb.SendLocalReply`. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 07.1.

### 4.1 New production code (in 07.1)

- **`internal/filter/http/doc.go`** — package overview: framework architecture, the `StreamDecoderFilter` / `StreamEncoderFilter` interfaces, the registry contract, the per-stream `FilterChain` lifecycle, the buffer-limit constant, the freeze-after-boot invariant.
- **`internal/filter/http/types.go`** — `StreamDecoderFilter` interface (decodeHeaders/Data/Trailers + SetDecoderCallbacks + OnDestroy), `StreamEncoderFilter` interface (encodeHeaders/Data/Trailers + SetEncoderCallbacks + OnDestroy), status enums (`FilterHeadersStatus`: Continue, StopIteration; `FilterDataStatus`: Continue, StopIterationAndBuffer, StopIterationNoBuffer; `FilterTrailersStatus`: Continue, StopIteration), `HTTPFilter` tagged-union over decoder-only / encoder-only / both, `HTTPFilterFactory` + `FilterInstanceFactory` two-step factory pattern, `FactoryCtx` (carries the registry pointer + parsed proto-helpers needed by per-filter parsers).
- **`internal/filter/http/callbacks.go`** — `DecoderFilterCallbacks` (`ContinueDecoding`, `SendLocalReply`, `RequestRouteConfig`, plus encoder-style injection methods for filters that synthesize responses), `EncoderFilterCallbacks` (`ContinueEncoding`, plus encoder-side injection methods). The callback structs are concretes implemented by the `chain` package; the interface is what filters depend on.
- **`internal/filter/http/registry.go`** — `HTTPRegistry struct { mu sync.RWMutex; byTypeURL map[string]HTTPFilterFactory; frozen atomic.Bool }`. `NewHTTPRegistry()`, `Register(typeURL string, f HTTPFilterFactory)` (panics if frozen, panics on duplicate type_url), `Lookup(typeURL string) (HTTPFilterFactory, bool)`, `Freeze()` (idempotent). Mirrors `*stats.Registry` LBP-1 from 06.1.
- **`internal/filter/http/chain.go`** — `FilterChain` per-stream state machine. Allocated by HCM dispatch (connection.go for H1, h2dispatch.go for H2) at the start of each request. Owns: filter instances (allocated via per-request factories from the chainConfig), per-filter callbacks, decode buffer (capacity `filterBufferLimitBytes = 1 << 20`), encode buffer (same cap), merged per-route config map, decode iteration index, encode iteration index, async-resume signal channels (`decodeResumeCh`, `encodeResumeCh`; both `chan struct{}` capacity 1 — non-blocking sends, idempotent coalesce). Methods: `runDecodeHeaders(headers, endStream)`, `runDecodeData(data, endStream)`, `runDecodeTrailers(trailers)`, `runEncodeHeaders(headers, endStream)`, `runEncodeData(data, endStream)`, `runEncodeTrailers(trailers)`, `beginLocalReply(status, headers, body)`. Single-goroutine-per-request iteration invariant (per BRAINSTORM §5.1): the HCM dispatch goroutine is the only goroutine that drives chain iteration; filter callbacks called from filter-spawned goroutines are signal-only (channel send to wake the dispatch goroutine).
- **`internal/filter/http/perroute.go`** — `typed_per_filter_config` parser + 3-tier merge. `BuildPerRouteConfig(rc *route.RouteConfiguration, filterNames []string) (PerRouteConfig, error)`: parses `typed_per_filter_config` on each scope (RouteConfiguration / VirtualHost / Route); validates keys against filter names (errors on unknown). `(*PerRouteConfig) Resolve(filterName string, route, vhost, rc Scope) proto.Message`: merge order Route > VirtualHost > RouteConfiguration; most-specific override (no field-merge). Lazy cache on first lookup per request (the cache is per-stream, lives on `FilterChain`).
- **`internal/filter/http/registry_test.go`**, **`chain_test.go`**, **`perroute_test.go`**, **`callbacks_test.go`** — unit tests per BRAINSTORM §6.1 (full enumeration in §13.1 below).
- **`internal/filter/http/fuzz_test.go`** — `FuzzFilterChainParse` per BRAINSTORM §6.5. Fuzzes adversarial `http_filters[]` slices into `parseFilterWithCtx` (varied type_urls, malformed typed_configs, oversized counts).

- **`internal/filter/http/cors/cors.go`** — real Envoy filter `envoy.filters.http.cors` (~150 LoC). Decode-side: detect preflight (`OPTIONS` + `Origin` + `Access-Control-Request-Method`); if matched + origin is allowed by per-route policy, `SendLocalReply(200, "", corsHeaders)` synthesizing the preflight response with the verbatim header set pinned in §11 #2. If matched + origin NOT allowed, the cors filter does NOT inject the preflight response — the request proceeds (per the v1.37.2 empirical pin: a disallowed-origin preflight in a routed config produces `405 Method Not Allowed` from the upstream/router path, not a synthesized 4xx from cors). Encode-side: append `Access-Control-Allow-Origin` / `Access-Control-Allow-Credentials` / `Access-Control-Expose-Headers` per the resolved per-route config when the request had an `Origin` header that matched the allow-list; pass-through otherwise.
- **`internal/filter/http/cors/cors_test.go`** — preflight detection (OPTIONS + Origin + ACRM); preflight allowed origin → 200 + headers; disallowed origin → preflight passes through to router (which 405s); actual request → encodeHeaders adds CORS response headers; per-route override (different allowed origins on different routes); the type_url + factory (`cors.New`) round-trip through the registry.

- **`internal/filter/http/envoygotest/filter.go`** — test-only probe filter `envoy.filters.http.envoy_go_test` (~250 LoC). Per-request mode dispatch on `x-envoy-go-test-mode` request header. Modes:
  - `continue` — Continue on every callback.
  - `stop-and-resume-headers` — StopIteration on decodeHeaders; spawn a goroutine that calls `ContinueDecoding()` after a 10ms tick.
  - `stop-and-buffer-data` — decodeData StopIterationAndBuffer until end_stream; modify buffered body bytes (e.g., uppercase ASCII); Continue.
  - `local-reply-decode` — decodeHeaders SendLocalReply(418, body=`"i am a teapot\n"`).
  - `local-reply-decode-data` — decodeData SendLocalReply(418, body=`"teapot mid-body\n"`) on first DATA chunk.
  - `modify-encode-headers` — pass-through decode; on encodeHeaders, add `x-envoy-go-test-injected: 1`.
  - `modify-encode-data` — pass-through decode; on encodeData, prefix body bytes with `"PROBED:"`.
  - `stop-trailers` — StopIteration on decodeTrailers; resume after 10ms tick.

  Per-route config: a `count` field (int32). On encodeHeaders, the filter calls `RequestRouteConfig()` and echoes `count` into a response header (`x-envoy-go-test-route-count: N`).
- **`internal/filter/http/envoygotest/proto/envoygotest.pb.go`** — hand-rolled minimal proto Message (not in upstream go-control-plane). Two fields: `mode_default` (string; default test-mode if no header) + `count` (int32; per-route-config field).
- **`internal/filter/http/envoygotest/filter_test.go`** — each `x-envoy-go-test-mode` value's expected behavior; per-route `count` config → response header injection.

- **`internal/filter/http/router/router.go`** — terminal filter (`envoy.filters.http.router`); decodeHeaders dispatches the resolved route action (cluster dial OR direct_response synthesize). Migrated from `internal/filter/hcm/actions.go`'s `routerAction`; tests byte-preserved.
- **`internal/filter/http/router/router_h2.go`** — H2-specific dispatch (was `routerActionH2` in `hcm/actions.go`). Migrated; tests byte-preserved.
- **`internal/filter/http/router/router_test.go`**, **`router_h2_test.go`** — migrated tests; per BRAINSTORM §6.8, byte-preserved except for import paths + package names.

### 4.2 Changed production code (in 07.1)

- **`internal/filter/hcm/config.go`** — `parseFilterWithCtx(...)` signature gains a `*filter.HTTPRegistry` parameter. Walks `http_filters[]` in declaration order; for each entry, calls `registry.Lookup(entry.typed_config.type_url)`; constructs the chain config (filter name + per-instance factory closure). Validation: empty chain errors; last entry not router errors; duplicate filter names error; unknown type_url errors. After chain config built, parses `typed_per_filter_config` on RouteConfig / VirtualHost / Route via `perroute.Build` and validates keys ⊆ chain filter names.
- **`internal/filter/hcm/filter.go`** — `Filter` struct gains `chainConfig []chainEntry` (filter name + per-instance factory) and `perRouteConfig PerRouteConfig`. Constructor signature widens: all four constructor variants (`NewFilter`, `NewFilterWithCtx`, `NewFilterWithCtxAndSinks`, the new `NewFilterWithCtxAndSinksAndRegistry`) extend with one `*filter.HTTPRegistry` parameter. The `cmd/envoy-go/main.go` call site updates with one line. Legacy call sites in tests update mechanically (tests not exercising the registry pass an empty `*filter.HTTPRegistry` allocated via `filter.NewHTTPRegistry()` and immediately frozen).
- **`internal/filter/hcm/actions.go`** — `routerAction` + `routerActionH2` are DELETED (moved to `internal/filter/http/router/`). `directResponseAction` STAYS (route-action shape decided at route-match time, synthesized by the router filter when its terminal step runs). The `actions_test.go` tests for `directResponseAction` stay; the `routerAction` / `routerActionH2` tests move to `internal/filter/http/router/router_test.go` + `router_h2_test.go` byte-preserved.
- **`internal/filter/hcm/connection.go`** (H1 dispatch) — `dispatchRequest(ctx, req, w)` allocates a per-request `*FilterChain` from `f.chainConfig`, runs the filter chain (`chain.runDecodeHeaders` → ... → terminal router filter → ... → `chain.runEncodeHeaders` → ... → wire write). The pre-existing flow (request line + headers parsed via std `http.ReadRequest` → terminal action `entry.action.do`) becomes (parsed request → chain run → router filter is the terminal entry that performs the action). The wire-write path is unchanged.
- **`internal/filter/hcm/h2dispatch.go`** (H2 dispatch) — same shape change as connection.go for the H2 codec path.
- **`internal/filter/hcm/accesslog_emit.go`** — emit-deferral hook sites unchanged in code BUT the trigger point shifts: the access-log Sink.Emit hook now fires from the chain's terminal-completion path (encode chain finished writing) rather than from the action.do return. The emit-deferral logic itself is unchanged.
- **`internal/filter/doc.go`** — rewritten. Replaces the phase-00 placeholder ("real implementation lands in phase 07") with the actual architectural overview pointing to `internal/filter/http/` for the HTTP-side framework and `internal/filter/hcm/` for the HCM-internal dispatch wiring.
- **`cmd/envoy-go/main.go`** — at boot: `reg := filter.NewHTTPRegistry()`; `reg.Register(router.TypeURL, router.New)`; `reg.Register(cors.TypeURL, cors.New)`; `reg.Register(envoygotest.TypeURL, envoygotest.New)`; `reg.Freeze()`; threads `reg` into the HCM constructor chain via `listenerManager.New(...)` (which threads it into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`).
- **`internal/listener/manager.go`** — listener-manager's HCM-construction path threads the `*filter.HTTPRegistry` parameter through to `hcm.NewFilter*`.
- **`internal/bootstrap/bootstrap.go`** — adds blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"` (carries the `Cors` + `CorsPolicy` proto messages) so `protojson` can round-trip 07.1 fixture bootstraps. Per ADR-0016's amendment policy, this addition is documented in PROGRESS, not a new ADR.

### 4.3 New harness and fixture code (in 07.1)

- **`test/differential/0007a-cors/`** — new differential fixture directory. Contents:
  - **`envoy-go.yaml`** — subject bootstrap. 1 listener (`l_h1`) binding `127.0.0.1:0` plaintext. 1 filter chain with empty `filter_chain_match`. 1 HCM with `codec_type: HTTP1` and `stat_prefix: ingress_http`. 1 STATIC cluster `c0` with 1 endpoint pointing at the controlled backend. Route_config with two routes:
    - `/permissive` (vhost `vh_permissive`): per-route `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{allow_origin_string_match: [exact: "https://example.test", exact: "https://anywhere.test"], allow_methods: "GET, POST, OPTIONS", allow_headers: "*", expose_headers: "x-baz", allow_credentials: true, max_age: "600"}` → cluster `c0`.
    - `/strict` (vhost `vh_strict`): per-route `CorsPolicy{allow_origin_string_match: [exact: "https://example.test"], ...}` → cluster `c0`.
    - `http_filters: [envoy.filters.http.cors, envoy.filters.http.router]`.
  - **`envoy.yaml`** — reference bootstrap. Same listener / HCM / route_config shape. 1 STRICT_DNS cluster `c0` pointing at `host.docker.internal:<backend-port>` with `dns_lookup_family: V4_ONLY` per ADR-0010; same `http_filters` order. `--concurrency 1` per ADR-0028.
  - **`expectations.yaml`** — prose description of the 4-request workload + the per-request expectation table (status, response-header set, body equivalence flag) per §11 #2 below.
  - **`README.md`** — explains the fixture's purpose (differential per-request equivalence on cors filter), the STATIC-vs-STRICT_DNS divergence, the 4-request shape, the per-route config differential exercise, the cross-reference to BEHAVIOR_CONTRACT `## HTTP filter chain` (introduced at 07.1 phase-done).
  - **`driver/driver.go`** — `BackendCount() = 1`. `SubjectListenerName() = "l_h1"`. `ReferenceListenerPort() = 15007`. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)` issue 4 H1 requests (per the §11 #2 verbatim shapes) and capture status + response headers + body. The runner asserts per-request status + header set (modulo the existing differential ignore-list in `BEHAVIOR_CONTRACT.md ## Header allow-list`) + body byte-equality.
  - **`driver/driver_test.go`** — distribution-/expectation-assertion unit tests.
  - **`backends/main.go`** — small Go program that starts an HTTP/1.1 server on a configurable port; returns 200 OK with a static body (`"hello\n"`). Used by both proxies' upstream cluster.

- **`test/differential/0007b-iteration-probe/`** — new structural fixture directory. Contents:
  - **`envoy-go.yaml`** — subject bootstrap. 1 listener (`l_h1`) binding `127.0.0.1:0` plaintext. 1 HCM with `codec_type: HTTP1` and `stat_prefix: ingress_http`. 1 STATIC cluster `c0` with 1 endpoint. Route_config with one route (`/`) → cluster `c0`. `http_filters: [envoy.filters.http.envoy_go_test, envoy.filters.http.router]`. Per-route `typed_per_filter_config[envoy.filters.http.envoy_go_test] = {count: 7}`.
  - **`expectations.yaml`** — embedded per-mode expectation table covering 8 modes (continue, stop-and-resume-headers, stop-and-buffer-data, local-reply-decode, local-reply-decode-data, modify-encode-headers, modify-encode-data, stop-trailers); each row carries the expected status + response header set + body. Same shape as 0006-access-log's per-record matrix per ADR-0068's three-tier pattern, but here the tier is "structural assertion" only (no reference Envoy implements `envoy_go_test`).
  - **`README.md`** — explains: this is an envoy-go-only structural fixture (no reference Envoy in this fixture); the probe filter `envoy_go_test` covers every iteration-state branch; the per-mode workload + the expected-shape table; the `RequiresReference: false` registration in `runner.go`.
  - **`driver/driver.go`** — `BackendCount() = 1`. `SubjectListenerName() = "l_h1"`. `RequiresReference() = false`. `DriveSubject(ctx, addr)` issues 8 H1 requests, one per mode; collects status + response headers + body; the runner asserts each against the expectation table.
  - **`driver/driver_test.go`** — per-mode unit tests.
  - **`backends/main.go`** — H1 echo backend (returns the request body if non-empty; else returns `"backend\n"`).

- **`test/differential/runner.go`** (extended) — registration update: blank-import the new fixture-0007a + 0007b driver packages. The runner's per-fixture loop calls each driver's hooks per the in-band pattern. `0007a` registers as `RequiresReference: true`; `0007b` as `RequiresReference: false`.

### 4.4 Changed documentation and state (in 07.1)

- **`docs/envoy-go/ROADMAP.md`** — row `07.1`: `status: planned → in-progress` flipped at the SPEC commit (per the corrected pattern from phase 05/05.1/05.2 + 06.1/06.2's SPEC commits, recorded in `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); transitions to `done` at the 07.1 phase-done commit. Row `07` (parent): stays `in-progress` at the 07.1 phase-done commit (the parent only flips to `done` at 07.2's phase-done — see parent SPEC §5). Row `07.2`: stays `planned` until 07.2's SPEC drafts. The split landed in this commit's ROADMAP edit (per the SPEC-drafting subagent's deliverable list).
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → 07.2 entry at lifecycle-state 1).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (extended in-place per ADR-0052's authorization, mirroring the 06.1 / 06.2 in-place-edit pattern at phase-done) — adds a new top-level section `## HTTP filter chain` populated with the empirical-pin blocks from §11 below + the iteration-protocol shape rules + the buffer-overflow rules + the async-resume mechanics + the filter ordering rules + the equivalence-matrix new row. Amends the existing `## HTTP/1.1` and `## HTTP/2` subsections to update the "exactly `[router]`" rule references to "non-empty; last entry must be router" with a forward-pointer to `## HTTP filter chain`. **The in-place edit lands at the 07.1 phase-done commit, NOT at the SPEC commit** (mirrors the 06.1 / 06.2 in-place-edit timing per the same ADR-0052 discipline).
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — UNCHANGED in 07.1 (no pin bump; D-3.7 reserves pin bumps for dedicated phases).
- **`docs/envoy-go/DECISIONS.md`** — seven new ADRs introduced by phase 07.1, numbered ADR-0070..ADR-0076 (next-free per the `DECISIONS.md` tail at master `2c65fcc` being ADR-0069; the planner re-verifies next-free at write time per ADR-0004's autonomous-numbering rule). Topics enumerated in §8 below; the ADRs themselves are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).
- **`docs/envoy-go/phases/07-filter-chain-framework/SPEC.md`** — UNCHANGED in 07.1 (the parent master SPEC is read-only history once drafted at this commit).

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 07.1)

Phase 07.1 introduces one new package tree (`internal/filter/http/`), substantially refactors `internal/filter/hcm/` (constructor signatures widen + the HCM dispatch path gains a chain runner + actions.go's router actions move out), and threads a `*filter.HTTPRegistry` parameter through one constructor chain (`hcm.NewFilter*`) plus the listener-manager's HCM-construction path.

```
cmd/envoy-go/main.go                          (MODIFIED: alloc *filter.HTTPRegistry, register
                                               router/cors/envoygotest, Freeze; thread through
                                               listener-manager into HCM constructors)
cmd/envoy-go/main_test.go                     (MODIFIED: bootstrap variants thread a Registry)
internal/bootstrap/bootstrap.go               (MODIFIED: blank-import cors v3 proto)
internal/listener/manager.go                  (MODIFIED: NewManager threads *filter.HTTPRegistry
                                               into the HCM-construction closure)
internal/filter/doc.go                        (REWRITE: framework overview)
internal/filter/http/                         (NEW package tree)
   doc.go                                     (NEW)
   types.go                                   (NEW: StreamDecoderFilter, StreamEncoderFilter,
                                               status enums, factory pattern)
   callbacks.go                               (NEW: DecoderFilterCallbacks, EncoderFilterCallbacks)
   registry.go                                (NEW: HTTPRegistry, Register, Lookup, Freeze)
   chain.go                                   (NEW: FilterChain per-stream state machine)
   perroute.go                                (NEW: typed_per_filter_config 3-tier merge)
   types_test.go, callbacks_test.go,
     registry_test.go, chain_test.go,
     perroute_test.go                         (NEW)
   fuzz_test.go                               (NEW: FuzzFilterChainParse)

   cors/                                      (NEW: real Envoy filter, differential fixture A)
     cors.go                                  (~150 LoC)
     cors_test.go
     doc.go

   envoygotest/                               (NEW: test-only probe, structural fixture B)
     filter.go                                (~250 LoC; per-request mode-driven branching)
     filter_test.go
     proto/envoygotest.pb.go                  (hand-rolled minimal proto)
     doc.go

   router/                                    (NEW: router extracted as a real filter)
     router.go                                (terminal filter; H1)
     router_h2.go                             (terminal filter; H2)
     router_test.go, router_h2_test.go        (migrated; byte-preserved)
     doc.go

internal/filter/hcm/                          (existing; substantially refactored)
   config.go                                  (MODIFIED: parses http_filters[] via *HTTPRegistry;
                                               typed_per_filter_config plumbed via perroute.Build)
   filter.go                                  (MODIFIED: NewFilterWithCtxAndSinksAndRegistry(...)
                                               extends the constructor chain with *HTTPRegistry;
                                               Filter struct stores chainConfig + perRouteConfig)
   actions.go                                 (MODIFIED: routerAction + routerActionH2 DELETED;
                                               directResponseAction STAYS as a route-action shape)
   actions_test.go                            (MODIFIED: routerAction tests deleted; direct_response
                                               tests stay)
   connection.go                              (MODIFIED: H1 dispatch runs FilterChain before/after
                                               terminal router; access-log emit triggers from
                                               chain-completion)
   h2dispatch.go                              (MODIFIED: same change for H2 codec path)
   accesslog_emit.go                          (UNCHANGED in code; trigger-point semantics shift)
   bytecounter.go                             (UNCHANGED)
   h2/                                        (UNCHANGED)
   chain_integration_test.go                  (NEW: framework-runs-filters-then-router happy paths
                                               in both H1 and H2)

internal/filter/tcpproxy/                     (UNCHANGED in 07.1)

test/differential/runner.go                   (MODIFIED: blank-imports for 0007a + 0007b drivers;
                                               0007a registers as RequiresReference: true,
                                               0007b as RequiresReference: false)
test/differential/0007a-cors/                 (NEW differential fixture)
test/differential/0007b-iteration-probe/      (NEW structural fixture)

docs/envoy-go/BEHAVIOR_CONTRACT.md            (MODIFIED at phase-done commit, NOT SPEC commit:
                                               new ## HTTP filter chain section + amendments to
                                               ## HTTP/1.1 and ## HTTP/2 router-shape rules)
docs/envoy-go/CONFORMANCE_PINS.md             (UNCHANGED)
docs/envoy-go/DECISIONS.md                    (APPENDED at impl-time per ADR-by-ADR commits:
                                               ADR-0070..ADR-0076 — seven ADRs)
docs/envoy-go/ROADMAP.md                      (MODIFIED at SPEC commit: row 07.1 planned → in-progress;
                                               row 07.2 added; row 07 parent gains sub-phases column.
                                               At phase-done: row 07.1 → done; row 07 stays in-progress)
docs/envoy-go/STATE.md                        (MODIFIED at each lifecycle transition by parent session)
docs/envoy-go/phases/07-filter-chain-framework/SPEC.md  (UNCHANGED — parent master SPEC, read-only)
docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
docs/envoy-go/phases/07.2-listener-chain-completion/README.md  (UNCHANGED — sibling SPEC stub)
```

### 5.2 Iteration-protocol concurrency model (per BRAINSTORM §5.1)

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `HTTPRegistry.Register` | Once per filter, at process start | `Registry.mu` Lock; panics if frozen |
| Boot | `HTTPRegistry.Lookup` from `parseFilterWithCtx` | Once per HCM build per filter entry | `Registry.mu` RLock |
| Per-request | `chain.runDecode*` / `chain.runEncode*` | Per request | None — single goroutine per request iterates the chain |
| Per-request | `cb.ContinueDecoding()` / `cb.ContinueEncoding()` | Per async filter, per request | Send-on-buffered-chan (capacity 1) — non-blocking; idempotent |
| Per-request | `cb.SendLocalReply()` | At most once per request | `sync.Once` guards entry into beginLocalReply |
| Per-request | filter `OnDestroy` | At end of request | None — called from HCM dispatch goroutine after chain teardown |

**Single-goroutine-per-request iteration invariant:** the HCM dispatch goroutine (per H1 connection `runConnection` or H2 stream `onHeaders`) is the *only* goroutine that drives `chain.runDecode*` / `chain.runEncode*`. Filter callbacks called from other goroutines (e.g., a filter's async timer goroutine calling `ContinueDecoding`) are signal-only — they unblock the dispatch goroutine via channel send; they do NOT enter chain iteration themselves. This makes the chain's internal state (iteration index, per-filter buffer, perRouteConfigCache) lock-free.

### 5.3 The `*HTTPRegistry` freeze invariant (per BRAINSTORM §5.1)

Mirrors `*stats.Registry` LBP-1 from 06.1. After `*HTTPRegistry.Freeze()` is called from `cmd/envoy-go/main.go` after all `Register` calls, any subsequent `Register` panics with `filter: registry frozen: cannot register %q post-boot`. `Lookup` does not panic post-Freeze (it remains read-allowed). The boot-time order is:

```
cmd/envoy-go/main.go
   ↓
filter.NewHTTPRegistry()
   ↓ Register: router.TypeURL → router.New
   ↓ Register: cors.TypeURL   → cors.New
   ↓ Register: envoygotest.TypeURL → envoygotest.New
   ↓
HTTPRegistry.Freeze()
   ↓
bootstrap.Load(configPath) → *Bootstrap
   ↓
listenerManager.New(listeners, ..., *filter.HTTPRegistry, ...)
   ↓ on each listener.filter_chain[].filters[i] of type=HCM:
        hcm.NewFilterWithCtxAndSinksAndRegistry(...)
            ↓ parseFilterWithCtx walks msg.GetHttpFilters():
                 for each entry:
                    factory, ok := httpRegistry.Lookup(entry.typed_config.type_url)
                    if !ok → "hcm: http_filters[i]: unknown type_url ..."
                    instanceFactory, err := factory(entry.typed_config, factoryCtx)
                    chainConfig.append({name: entry.name, factory: instanceFactory})
            ↓ enforce: last entry MUST be router (terminal)
            ↓ enforce: at least 1 entry (the router)
            ↓ build typed_per_filter_config map per Route/VHost/RouteConfig
            ↓ validate typed_per_filter_config keys ⊆ chain filter names
            ↓ Filter struct stores: chainConfig []chainEntry, perRouteConfig map
   ↓
listenerManager.Run() — listener manager begins accepting connections
```

A unit test (`registry_test.go`) sets the frozen flag and panics on `Register` after.

### 5.4 Per-request decode-side flow (per BRAINSTORM §4.2)

```
H1: connection.go runConnection() — request line + headers parsed via std http.ReadRequest
H2: h2dispatch.go onHeaders()      — request headers via h2.StreamReader

[converged dispatch from here]
   ↓
hcm.Filter.dispatchRequest(ctx, req, w):
   chain := newFilterChain(f.chainConfig, f.perRouteConfig, req)   -- alloc per-request state
        ↓ for each chainEntry: instance := entry.factory()         -- fresh per-request filter
        ↓ chain.filters = []HTTPFilter{cors, envoygotest, router}
   chain.runDecodeHeaders(req.Header, !hasBody)
        ↓ for i in 0..len(filters)-1:
             status := filters[i].DecodeHeaders(headers, endStream)
             switch status:
               Continue: i++
               StopIteration: park on chain.decodeResumeCh; on resume i++
                 (sendLocalReply during pause: handled per §5.6)
   if request has body:
      chain.runDecodeData(body chunks)
        ↓ for each chunk: iterate decode-side; per-filter buffer if StopIterationAndBuffer
        ↓ overflow at filterBufferLimitBytes → synthesize 413 + enter encode chain
   if request has trailers:
      chain.runDecodeTrailers(trailers)
   ↓ terminal filter is router; its decodeHeaders/Data/Trailers initiates upstream dial OR
     synthesizes direct_response (depending on resolved route action)
```

### 5.5 Per-request encode-side flow (per BRAINSTORM §4.3)

```
[Trigger paths for entering encode chain]
  (a) Router filter received upstream response: chain.runEncodeHeaders(...)
  (b) sendLocalReply called from any filter: chain.beginLocalReply(status, hdrs, body)
                                              → chain.runEncodeHeaders(...)

chain.runEncodeHeaders(headers, endStream):
   ↓ for i in len(filters)-1 .. 0   (reverse order — see §11 #1 empirical pin)
        skip filter if it has no encoder side
        status := filters[i].EncodeHeaders(headers, endStream)
        switch status: Continue / StopIteration (resume on chain.encodeResumeCh)
   ↓ once iteration completes: write status line + headers to wire
       (H1: bufio.Writer; H2: streamWriter)
chain.runEncodeData(data chunks):
   ↓ same reverse iteration; same buffer mgmt; overflow on encode-side resets the connection
     (H1: close; H2: RST_STREAM — per §11 #3 empirical pin)
chain.runEncodeTrailers(trailers):
   ↓ same reverse iteration

After encode chain terminates:
   ↓ accesslog.Sink.Emit hook fires (existing path; trigger is now chain-completion, not
     direct action.do return — emit-deferral path adapts in connection.go / h2dispatch.go)
```

### 5.6 `sendLocalReply` flow (per BRAINSTORM §4.5; see §11 #4 empirical pin)

```
Filter[k] calls cb.SendLocalReply(status=403, body, headers) during decodeHeaders:
   ↓ chain marks decode-side aborted; cancels any pending resume
   ↓ chain.beginLocalReply(status, headers, body):
       synthesizedHeaders := mergeStandardLocalReplyHeaders(headers, status, len(body))
       chain.runEncodeHeaders(synthesizedHeaders, endStream=false)
       chain.runEncodeData([]byte(body), endStream=true)
       (no trailers)
   ↓ if any encode-side filter returns StopIteration during this: it parks normally
   ↓ written to wire after encode chain completes
```

Per the §11 #4 empirical pin, the synthesized response enters the encode chain at filter[len-1] of the encode-side filter set. All filters with encode side run regardless of where decode aborted (filter[k]'s encode runs even though it was the abort point on decode — the encode iteration is reverse-order and full).

If two decode-side filters call `SendLocalReply` concurrently (filter[0] starts iteration; filter[2] resumes from async and calls SendLocalReply before filter[1]'s pause completes): the first call wins; the second is a no-op. The chain's "first SendLocalReply wins" guard is a `sync.Once` + atomic flag.

### 5.7 Async-resume mechanics (per BRAINSTORM §4.4)

```
Filter calls cb.ContinueDecoding() from any goroutine:
   ↓ select { case ch.decodeResumeCh <- struct{}{}: default: }    -- non-blocking; idempotent
HCM dispatch goroutine, parked on StopIteration:
   ↓ select {
        case <-ch.decodeResumeCh: i++; continue iteration
        case <-ctx.Done():        abort iteration; close downstream
        case <-time.After(reqDeadline): abort with 504
     }
```

The resume channel is buffered with capacity 1, so duplicate `ContinueDecoding()` calls (filter-author bug or genuine race between two callbacks) are silently coalesced. `Filter.OnDestroy()` is called after iteration completes (success or abort) to let filters release resources. Per-stream goroutines are *not* spawned by the framework; if a filter spawns its own goroutine for an async lookup, that's the filter's responsibility (and OnDestroy is the filter's signal to cancel it).

**No goroutine bloat in the common case.** A request whose filters all return Continue runs entirely on the HCM dispatch goroutine — zero new goroutines. Async filters (test probe in `stop-and-resume-headers` mode, or future `jwt_authn`) spawn their own goroutines; that's their own design surface.

## 6. Iteration-protocol surface — interfaces, status enums, callbacks (per BRAINSTORM §2.1)

### 6.1 Filter interfaces

```go
type StreamDecoderFilter interface {
    DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
    DecodeData(data []byte, endStream bool)            FilterDataStatus
    DecodeTrailers(trailers http.Header)               FilterTrailersStatus
    SetDecoderCallbacks(cb DecoderFilterCallbacks)
    OnDestroy()
}
type StreamEncoderFilter interface {
    EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
    EncodeData(data []byte, endStream bool)            FilterDataStatus
    EncodeTrailers(trailers http.Header)               FilterTrailersStatus
    SetEncoderCallbacks(cb EncoderFilterCallbacks)
    OnDestroy()
}
```

A filter implements decode-only, encode-only, or both. The factory's return type signals which side(s) — see §6.4.

### 6.2 Status enums

- `FilterHeadersStatus`: `Continue`, `StopIteration`. Envoy's `ContinueAndDontEndStream` is OUT of MVP — single-knob state we don't need.
- `FilterDataStatus`: `Continue`, `StopIterationAndBuffer`, `StopIterationNoBuffer`. Watermark variants are OUT of MVP.
- `FilterTrailersStatus`: `Continue`, `StopIteration`.

### 6.3 Callbacks (filter ⇒ framework)

```go
type DecoderFilterCallbacks interface {
    ContinueDecoding()
    SendLocalReply(status int, body string, headers http.Header)
    EncodeHeaders(headers http.Header, endStream bool)  // for filters that synthesize responses
    EncodeData(data []byte, endStream bool)
    EncodeTrailers(trailers http.Header)
    RequestRouteConfig() proto.Message  // per-route config for the calling filter's name
}
type EncoderFilterCallbacks interface {
    ContinueEncoding()
    EncodeHeaders(headers http.Header, endStream bool)  // injection (rare)
    EncodeData(data []byte, endStream bool)
    EncodeTrailers(trailers http.Header)
}
```

`ContinueDecoding()` / `ContinueEncoding()` are safe from any goroutine. `SendLocalReply` synthesizes a response that enters the encode-side filter chain per §5.6 + §11 #4. `RequestRouteConfig()` returns the merged proto for the calling filter's name (nil if no per-route config applies; merge cached per-stream after first lookup).

### 6.4 Two-step factory pattern

```go
type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)
type FilterInstanceFactory func() HTTPFilter   // called once per request
type HTTPFilter struct {
    Decoder StreamDecoderFilter   // nil for encoder-only filters
    Encoder StreamEncoderFilter   // nil for decoder-only filters
}
```

Step 1 (HCM-build time): `HTTPFilterFactory` parses + validates `typed_config`, returns a `FilterInstanceFactory` closure. Step 2 (per-request): the closure allocates a fresh filter instance bound to the parsed config. Mirrors Envoy's `FilterFactoryFn`. Per-config validation cost paid once; per-request cost is one allocation.

### 6.5 Body buffering on `StopIterationAndBuffer`

The framework appends `decodeData` chunks into a per-stream buffer up to `const filterBufferLimitBytes = 1 << 20` (1 MiB; matches Envoy default per §11 #3 empirical pin). Overflow synthesizes a `413 Payload Too Large` local reply (verbatim shape per §11 #3), which then flows through the encode-side filter chain. Configurable knobs (`per_connection_buffer_limit_bytes` on Listener, `per_request_buffer_limit_bytes` on Route) are silently ignored at parse-time per ADR-0041 amendment.

### 6.6 Filter ordering

`http_filters[]` declaration order on decode-side; reverse declaration order on encode-side per §11 #1 empirical pin. The router stays the **terminal filter** — semantically must be last in `http_filters[]`. Phase-04's "exactly `[router]`" rule (ADR-0042) is replaced by "non-empty; last entry must be router" — partial supersession.

## 7. Differential fixture `0007a-cors` + structural fixture `0007b-iteration-probe` (per BRAINSTORM §2.5)

### 7.1 Equivalence claims

**`0007a-cors`** is differential (equivalence-with-Envoy): per-request status + response header set + body byte-equal across envoy-go and reference Envoy v1.37.2 (modulo the existing `BEHAVIOR_CONTRACT.md ## Header allow-list`).

**`0007b-iteration-probe`** is structural (envoy-go-only): each per-mode response asserted against an embedded expectation table. No reference Envoy in this fixture. Same shape as 0006-access-log's per-record matrix but with one tier (structural assertion) rather than three (the 06.2 three-tier matrix).

### 7.2 `0007a-cors` — driver outline (per §11 #2 empirical pin)

1. Boot envoy-go on P1 + reference Envoy on P2 with bootstraps that share the same HCM `http_filters: [envoy.filters.http.cors, envoy.filters.http.router]` order, two routes (`/permissive` allows `[https://example.test, https://anywhere.test]`, `/strict` allows `[https://example.test]`), each route's per-route `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{...}`.
2. Boot a controlled backend on P3 (returns 200 OK with body `"hello\n"`).
3. Issue 4 sequential requests at each proxy:

   | # | Request | Expected response (per §11 #2 verbatim pin) |
   |---|---|---|
   | 1 | `OPTIONS /permissive` `Origin: https://example.test` `Access-Control-Request-Method: GET` `Access-Control-Request-Headers: x-foo,x-bar` | `200 OK` + the 6 CORS preflight headers (`access-control-allow-origin`, `access-control-allow-credentials`, `access-control-allow-methods`, `access-control-allow-headers`, `access-control-max-age`, `access-control-expose-headers`) + empty body |
   | 2 | `OPTIONS /strict` `Origin: https://other.test` `Access-Control-Request-Method: GET` | `405 Method Not Allowed` (preflight passes through cors filter; reaches router which rejects the OPTIONS; matches Envoy's empirical behavior — see §11 #2) |
   | 3 | `GET /permissive` `Origin: https://example.test` | `200 OK` + body `"hello\n"` + injected `access-control-allow-origin: https://example.test` + `access-control-allow-credentials: true` + `access-control-expose-headers: x-baz` |
   | 4 | `GET /strict` (no Origin) | `200 OK` + body `"hello\n"` + NO CORS response headers |

4. Per-request equivalence: status + response header set (modulo allow-list) + body byte-equal across envoy-go and Envoy. The driver returns `(refResponses, subjResponses)` to the runner; the runner asserts.

### 7.3 `0007b-iteration-probe` — driver outline

envoy-go boots with `http_filters: [envoy.filters.http.envoy_go_test, envoy.filters.http.router]`, route `/` → cluster `c0` (a controlled echo backend), per-route `typed_per_filter_config[envoy.filters.http.envoy_go_test] = {count: 7}`. Driver issues 8 sequential `POST /` requests (or `GET /` for modes that don't need a body), each with a different `x-envoy-go-test-mode` header value:

| Mode | Body | Expected response |
|---|---|---|
| `continue` | `"abc"` | 200 + body from backend + `x-envoy-go-test-route-count: 7` |
| `stop-and-resume-headers` | `""` | 200 + body from backend + `x-envoy-go-test-route-count: 7` (resume after 10ms tick) |
| `stop-and-buffer-data` | `"abc"` | 200 + body from backend (modified by filter on decode) + `x-envoy-go-test-route-count: 7` |
| `local-reply-decode` | `""` | 418 + body `"i am a teapot\n"` (no upstream) |
| `local-reply-decode-data` | `"abc"` | 418 + body `"teapot mid-body\n"` (no upstream; mid-body abort) |
| `modify-encode-headers` | `""` | 200 + body from backend + `x-envoy-go-test-injected: 1` + `x-envoy-go-test-route-count: 7` |
| `modify-encode-data` | `""` | 200 + body `"PROBED:" + backend body` + `x-envoy-go-test-route-count: 7` |
| `stop-trailers` | `""` (with trailers) | 200 + body from backend + `x-envoy-go-test-route-count: 7` (resume on trailers after 10ms tick) |

Each per-mode response is asserted against the embedded expectation table in `expectations.yaml`. The runner registers the fixture with `RequiresReference: false`.

### 7.4 Differential gate scope clarification (per BRAINSTORM §6.7)

The differential gate (gate (a)) only asserts equivalence on `0007a` (`RequiresReference: true`); `0007b` is a structural-assertion gate (`RequiresReference: false`, matching the existing "envoy-go behaves as documented" pattern). The split is reflected in `test/differential/runner.go`'s fixture registration.

## 8. ADRs anticipated (per BRAINSTORM §8)

Seven ADRs anticipated for 07.1, numbered ADR-0070..ADR-0076 (next-free per the `DECISIONS.md` tail at master `2c65fcc` being ADR-0069). The planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule. The ADRs are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).

The numbering below is the expected mapping based on topical ordering; the planner may reorder commit-time landings if that reads more naturally in PLAN.md, in which case the actual ADR number assignments may permute (the four ADR-0066..ADR-0069 block in 06.2 used a non-monotonic commit-time ordering — this is permitted and recorded in the ADR's `Lands-in-task` field).

- **ADR-0070 — Phase-07 planner-time split (07.1 + 07.2).** Status: Accepted. Doctrine: D-3.5 + D-3.6. Decision: phase 07 splits into 07.1 (HTTP filter framework — under HCM) + 07.2 (listener-chain completion — under listener manager) at planner-time per ADR-0045's pattern. Rationale (per BRAINSTORM §1): disjoint code surfaces (`internal/filter/http/` + `internal/filter/hcm/` for 07.1 vs `internal/listener/` for 07.2), 07.1-first ordering rationale (07.1 unblocks BOOTSTRAP §9 HTTP-filters family; 07.2 has no §9 dependents). Mirrors ADR-0045 (phase 05 split) and the phase-06 split. Lands-in-task: SPEC drafting (this commit's ROADMAP edit).

- **ADR-0071 — HTTP filter iteration protocol shape.** Status: Accepted. Doctrine: D-3.2 + D-3.5. Decision: Envoy-faithful subset with async-resume; narrow method set (`DecodeHeaders/Data/Trailers`, `EncodeHeaders/Data/Trailers`); status enums settled (`Continue / StopIteration` for headers + trailers, `Continue / StopIterationAndBuffer / StopIterationNoBuffer` for data); explicitly out-of-MVP (`ContinueAndDontEndStream`, watermark variants, `Encode1xxHeaders`, metadata frames). Two-step factory pattern (config-time + per-request). Async-resume via per-stream buffered channel; single-goroutine-per-request iteration. **Supersedes ADR-0040 totally** (router-as-direct-call inside HCM connection loop is replaced by router-as-terminal-filter via the iteration protocol). Lands-in-task: 07.1 PLAN Task wherever `internal/filter/http/types.go` + `chain.go` first land.

- **ADR-0072 — `*HTTPRegistry` threaded constructor map, no package-global.** Status: Accepted. Doctrine: D-3.4. Decision: explicit threading discipline (mirrors `*stats.Registry` LBP-1 from 06.1 + `*cluster.Manager`); freeze-after-boot invariant; `Register` panic on post-Freeze; `Lookup` by `typed_config.type_url`. Rejection of `init()`-based global registration. Rationale: `init()`-based global registries make test isolation hard (each test wants its own filter set), tie filter-set composition to import-graph layout (a future build-tag-gated filter would flip imports unpredictably), and contradict the `*stats.Registry` LBP-1 precedent the project established in 06.1. Lands-in-task: 07.1 PLAN Task wherever `internal/filter/http/registry.go` lands.

- **ADR-0073 — `typed_per_filter_config` 3-tier merge model.** Status: Accepted. Doctrine: D-3.5. Decision: Route > VirtualHost > RouteConfiguration; most-specific-override (not field-level merge); lazy-cache on first `RequestRouteConfig()` call per request. Build-time validation: keys ⊆ chain filter names. **Honored at parse-time — partial supersession of ADR-0041's silent-ignore set:** `typed_per_filter_config` moves from silent-ignored to honored. Lands-in-task: 07.1 PLAN Task wherever `internal/filter/http/perroute.go` lands.

- **ADR-0074 — Trivial filter set: `cors` (real, differential) + `envoy_go_test` (test-only, structural).** Status: Accepted. Doctrine: D-3.3 + D-3.4. Decision: 07.1 ships `envoy.filters.http.cors` (real Envoy filter, used by differential fixture `0007a`) + `envoy.filters.http.envoy_go_test` (test-only probe filter, used by structural fixture `0007b`). The `envoygotest` proto schema is envoy-go-only (hand-rolled, not in upstream go-control-plane). Rationale: `cors` exercises the per-route config + the encode-side header injection + the SendLocalReply path on preflight (real-world filter behavior under differential equivalence); `envoy_go_test` covers iteration-protocol state branches that no single real filter covers (the 8-mode matrix of §7.3). Iteration-state coverage attribution table in the ADR enumerates which mode tests which protocol surface. Lands-in-task: 07.1 PLAN Tasks wherever `internal/filter/http/cors/` + `internal/filter/http/envoygotest/` land.

- **ADR-0075 — `sendLocalReply` encode-chain semantics.** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: synthesized response enters the encode chain at filter[len-1] (reverse iteration entry — see §11 #4 empirical pin); first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log; encode-side iteration is full (every encode-side filter runs). The empirical pin in §11 #4 is the durable evidence for the filter-index entry point (verified at SPEC time against reference Envoy v1.37.2 with chain `[lua_a, lua_b, lua_c, router]`; observed encode order `lua_c → lua_b → lua_a` after `lua_b` calls Envoy's `respond` API). Lands-in-task: 07.1 PLAN Task wherever `chain.beginLocalReply` lands.

- **ADR-0076 — Body buffer cap; 413 on decode overflow; reset on encode overflow.** Status: Accepted. Doctrine: D-3.5 + D-3.6. Decision: `filterBufferLimitBytes = 1 << 20` (1 MiB) hardcoded constant matching Envoy's default. Decode-side overflow synthesizes a `413 Payload Too Large` local reply (verbatim shape per §11 #3 empirical pin) → encode chain. Encode-side overflow → connection reset (H1 close, H2 RST_STREAM). The configurable knobs (`per_connection_buffer_limit_bytes`, `per_request_buffer_limit_bytes`) are silently ignored — extends ADR-0041 silent-ignore set (parallel to the 06.2 amendment for access_log fields and to the 06.1 add for `stats_config.stats_tags`). Lands-in-task: 07.1 PLAN Task wherever `chain.runDecodeData` + the 413 synthesis path lands.

**Inline supersessions** (recorded in the ADRs above, not as separate ADRs):

- **ADR-0040 totally superseded** by ADR-0071. Phase-04's "router invoked by direct function call inside HCM connection loop" is replaced by router-as-terminal-filter via the iteration protocol.
- **ADR-0042 partially superseded** by ADR-0071. Phase-04's "exactly `[router]`" rule becomes "non-empty; last entry must be router". Lower bound stays; upper bound lifted.
- **ADR-0041 amended** by ADR-0073 and ADR-0076. `typed_per_filter_config` moves from silent-ignored to honored (ADR-0073). `per_connection_buffer_limit_bytes` + `per_request_buffer_limit_bytes` are added to the silent-ignored set (ADR-0076).

(Phase 06.1 had 6 ADRs; 06.2 had 4. 7 sits at the high end — appropriate for a framework phase that supersedes prior ADRs and introduces a new protocol surface. The planning session may consolidate if any pair turns out to be the same decision in two clothes.)

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 07.1 silently ignores the following at parse time (no error, no honored behavior):

- Listener `per_connection_buffer_limit_bytes` (extends ADR-0041 silent-ignore set per ADR-0076).
- Route `per_request_buffer_limit_bytes` (same).
- `filter_chain_match.application_protocols[]` (still in the silent-skip set from 05.1; promoted to honored in 07.2).
- Listener `listener_filters[]` (still silently skipped from phase 03; promoted to honored in 07.2).
- `Listener.default_filter_chain` (still errors at parse from phase 03; promoted to honored in 07.2).
- `filter.disabled` flag on per-route config (no skip-this-filter-on-this-route override) — silently ignored at parse-time.
- Router proto fields silently ignored at phase 04 (`dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`) — unchanged from ADR-0040; 07.1 inherits the silent-ignore set verbatim.

The full silently-ignored set is the union of phases 04 / 05.1 / 05.2 / 06.1 / 06.2's silently-ignored sets plus 07.1's amendment above. The phase-04..06.2 ignored sets are NOT amended by this list — only extended. ADR-0041 (the original silent-ignore ADR) is amended (not superseded) to record the 07.1 additions; the amendment shape mirrors the 05.1 + 05.2 + 06.1 + 06.2 amendments.

## 10. Carry-forward dispositions (per BRAINSTORM §2.7)

**None expected.** Phase 06.2 REVIEW.md's 11 Minors are a separate post-phase-done batch (per STATE.md `next-skill-scope`) and do not interact with phase 07.1's scope. Phase 04's deferred items (M-X from 04 REVIEW) are H1-protocol-specific and unrelated to filter-chain framework. Phase 05's M-4 / M-10 / M-12 are H2-hardening concerns explicitly carry-forwarded to "dedicated H2-hardening sub-phase or upstream-robustness family" by 06.1's §13.2 disposition; they remain deferred from 07.1.

## 11. Empirical-pin block (per BRAINSTORM §2.6 — all four pins resolved IN-SESSION)

Mirrors 06.1's Rule SN4 empirical-pin block in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` (per ADR-0061) and 06.2's verbatim access-log pin (per ADR-0066). All four pin probes were executed against reference Envoy v1.37.2 at the `ENVOY_TARGET.md`-pinned image SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per the `/config_dump` `node.user_agent_build_version.metadata.revision.sha` field) on 2026-04-30 by this SPEC-drafting session. Each verbatim block below is paste-from-terminal output and is what `BEHAVIOR_CONTRACT.md ## HTTP filter chain` will carry verbatim at the 07.1 phase-done in-place edit (per ADR-0052; see §13).

### 11.1 Empirical pin #1 — Filter declaration order on encode side (reverse decode order)

**Probe configuration:** chain `[lua_a, lua_b, envoy.filters.http.router]` where `lua_a` and `lua_b` log on decode/encode entry via Envoy's Lua filter (`logCritical` writes to Envoy's stderr at `[critical]` level). Listener `127.0.0.1:10000`; route `/` → STRICT_DNS cluster `c0` reaching a host-side nginx backend.

**Probe request:** `GET / HTTP/1.1` (single sequential request).

**Verbatim Envoy stderr trace (decoded/encoded line emit order; timestamps preserved):**

```
[2026-05-01 01:10:55.841][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=A index=0
[2026-05-01 01:10:55.841][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=B index=1
[2026-05-01 01:10:55.842][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=B index=1
[2026-05-01 01:10:55.842][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=A index=0
```

**Conclusion (pinned):** decode-side iteration is in declaration order (`lua_a` index 0 → `lua_b` index 1 → router index 2 implicitly terminal). Encode-side iteration is in **reverse** declaration order (`lua_b` index 1 → `lua_a` index 0; router has no encode-side log emission in this probe but is the entry point into the encode chain since it produces the upstream response). This is the empirical evidence for the §6.6 + §5.5 reverse-encode-order rule. envoy-go's `chain.runEncodeHeaders` / `runEncodeData` / `runEncodeTrailers` MUST iterate from `len(filters)-1` down to `0`.

### 11.2 Empirical pin #2 — Cors filter response shape (preflight + actual-request + disallowed-origin)

**Probe configuration:** chain `[envoy.filters.http.cors, envoy.filters.http.router]`; one virtual_host with `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{allow_origin_string_match: [exact: "https://example.test"], allow_methods: "GET, POST, OPTIONS", allow_headers: "x-foo, x-bar", expose_headers: "x-baz", allow_credentials: true, max_age: "600"}`; one route `/` → STRICT_DNS cluster `c0` reaching a host-side nginx backend (which serves a 200 + HTML body on `GET /`).

**Probe (a) — Preflight, allowed origin:**

Request: `OPTIONS / HTTP/1.1` `Origin: https://example.test` `Access-Control-Request-Method: GET` `Access-Control-Request-Headers: x-foo,x-bar`

Verbatim response (header set in wire order, lowercase as emitted by Envoy):

```
< HTTP/1.1 200 OK
< access-control-allow-origin: https://example.test
< access-control-allow-credentials: true
< access-control-allow-methods: GET, POST, OPTIONS
< access-control-allow-headers: x-foo, x-bar
< access-control-max-age: 600
< access-control-expose-headers: x-baz
< date: Fri, 01 May 2026 01:09:51 GMT
< server: envoy
< content-length: 0
```

**Probe (b) — Preflight, disallowed origin:**

Request: `OPTIONS / HTTP/1.1` `Origin: https://other.test` `Access-Control-Request-Method: GET`

Verbatim response:

```
< HTTP/1.1 405 Method Not Allowed
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 157
< x-envoy-upstream-service-time: 0
```

**Probe (c) — Actual GET, allowed origin:**

Request: `GET / HTTP/1.1` `Origin: https://example.test`

Verbatim response (CORS-relevant headers shown in wire order; full body omitted — body is the upstream nginx default-page):

```
< HTTP/1.1 200 OK
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 896
< last-modified: Tue, 07 Apr 2026 12:09:53 GMT
< etag: "69d4f411-380"
< accept-ranges: bytes
< x-envoy-upstream-service-time: 0
< access-control-allow-origin: https://example.test
< access-control-allow-credentials: true
< access-control-expose-headers: x-baz
```

**Probe (d) — Actual GET, no Origin:**

Request: `GET / HTTP/1.1` (no Origin header)

Verbatim response (CORS headers absent — passthrough confirmed):

```
< HTTP/1.1 200 OK
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 896
< last-modified: Tue, 07 Apr 2026 12:09:53 GMT
< etag: "69d4f411-380"
< accept-ranges: bytes
< x-envoy-upstream-service-time: 0
```

**Conclusions (pinned):**

- **Preflight, allowed origin (probe a):** status `200 OK` (NOT `204 No Content` — BRAINSTORM §2.4 hypothesized 204; v1.37.2 emits 200). Six CORS response headers in this order: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-allow-methods`, `access-control-allow-headers`, `access-control-max-age`, `access-control-expose-headers`. Body length 0. envoy-go's cors filter MUST emit `200 OK` with empty body and the same six headers in the same order.
- **Preflight, disallowed origin (probe b):** the cors filter does NOT synthesize a 4xx local-reply for disallowed-origin preflights. Instead, the preflight passes through to the router, which sees an `OPTIONS /` and responds `405 Method Not Allowed` (since the route doesn't accept OPTIONS — which is the v1.37.2 default for routes without `route.connect_matcher` or explicit options handling). envoy-go's cors filter MUST replicate this passthrough (do NOT inject a 4xx; let the request flow to the router).
- **Actual request, allowed origin (probe c):** the cors filter's encodeHeaders adds three CORS response headers to the upstream response: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-expose-headers`. (NOT all six — `allow-methods`/`allow-headers`/`max-age` are preflight-only.)
- **Actual request, no Origin (probe d):** the cors filter is a no-op (no CORS response headers injected). Confirms the filter's gating discipline (no Origin → no encode-side action).

### 11.3 Empirical pin #3 — 413 overflow response shape

**Probe configuration:** chain `[envoy.filters.http.buffer, envoy.filters.http.router]` with `Buffer{max_request_bytes: 1024}`. Listener `per_connection_buffer_limit_bytes: 1024`. Route `/` → STRICT_DNS cluster `c0` (nginx backend).

**Probe request:** `POST / HTTP/1.1` `Content-Length: 2048` with a 2048-byte body of ASCII `'A'`.

**Verbatim response:**

```
< HTTP/1.1 413 Payload Too Large
< content-length: 17
< content-type: text/plain
< date: Fri, 01 May 2026 01:10:15 GMT
< server: envoy
< connection: close
```

**Body bytes (verbatim hex dump):**

```
00000000: 5061 796c 6f61 6420 546f 6f20 4c61 7267  Payload Too Larg
00000010: 65                                       e
```

**Conclusions (pinned):**

- Status: `413 Payload Too Large`.
- Body: 17 bytes, exact ASCII `Payload Too Large` (no trailing newline).
- Headers in wire order: `content-length: 17`, `content-type: text/plain`, `date: <stamp>`, `server: envoy`, `connection: close`.
- Connection is closed (note `connection: close`) — the 413 forces the H1 conn to terminate; envoy-go's encode-side overflow must mirror this discipline (H1: emit 413 then close conn; H2: RST_STREAM after the local-reply HEADERS+DATA frames). The `connection: close` header is what makes the 413 path's connection-reset semantically explicit on the wire.
- envoy-go's decode-side buffer overflow MUST synthesize this verbatim shape (status + body + headers — modulo `date` and `server` which are already in the differential allow-list per `BEHAVIOR_CONTRACT.md ## Header allow-list`).

### 11.4 Empirical pin #4 — `sendLocalReply` encode-chain entry filter index

**Probe configuration:** chain `[lua_a, lua_b, lua_c, envoy.filters.http.router]` where `lua_b` calls `respond` (Envoy's sendLocalReply API) with status 418 + a `x-from: filterB` header + body `"418 from filterB\n"`. `lua_a`, `lua_c` log on decode/encode entry. Route `/` → STRICT_DNS cluster `c0` (would route to nginx, but `lua_b`'s `respond` aborts decode mid-chain).

**Probe request:** `GET / HTTP/1.1`

**Verbatim Envoy stderr trace (timestamps preserved):**

```
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=A index=0
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=B index=1 (calling respond)
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=C index=2
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=B index=1
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=A index=0
```

**Verbatim response:**

```
< HTTP/1.1 418 Unknown
< x-from: filterB
< content-length: 17
< content-type: text/plain
< date: Fri, 01 May 2026 01:11:16 GMT
< server: envoy
```

**Conclusions (pinned):**

- Decode aborted at `lua_b` (index 1) when it called `respond`. `lua_c` (index 2) and router (index 3) NEVER ran on the decode side.
- Encode-side iteration entered at **`lua_c` (index 2)** — i.e., `filter[len-1]` of the encode-side filter set (router has no observable encode-side action in this probe but is at index 3; the encode-side iteration starts from the last filter that has an encode side, which in this chain is `lua_c` at index 2).
- ALL THREE Lua filters' encode sides ran (`lua_c` → `lua_b` → `lua_a`), even though only filter B's decode side reached its abort point — confirming that `sendLocalReply` runs the FULL encode chain in reverse order, not just from the abort point upward.
- Status `418 Unknown`: HTTP/1.1 status text "Unknown" because 418 is not a stdlib-known status code on Envoy's HTTP/1.1 codec; the payload includes the user-supplied `x-from: filterB` header alongside the framework-injected `content-length` / `content-type` / `date` / `server`.
- envoy-go's `chain.beginLocalReply` MUST: (a) abort decode-side iteration at the calling filter's index; (b) enter encode-side iteration at `filter[len-1]` of the encode-side set (NOT at the calling filter's index, NOT at index 0); (c) iterate the FULL encode chain in reverse order (every encode-side filter runs); (d) merge framework-injected standard headers (`content-length`, `content-type`, `date`, `server`) with the user-supplied headers (`x-from`).

### 11.5 Synchronization with BEHAVIOR_CONTRACT.md

The four blocks above are paste-from-terminal verbatim and are what `BEHAVIOR_CONTRACT.md ## HTTP filter chain` will carry verbatim at the 07.1 phase-done in-place edit (per §13 below). The §11 block + the §13 block are synchronized (no drift permitted; future image bumps per `ENVOY_TARGET.md`'s refresh procedure that alter any of the four shapes require updating both locations in the same commit, mirroring the 06.1 / 06.2 paste-verbatim discipline).

## 12. Deferred decisions (the planner / implementer settles these)

Items the SPEC names but does not finalize; the planner closes them in PLAN.md or the implementer closes them at task time per the SPEC's recommendation.

1. **HCM dispatch hook for chain-completion → access-log emit trigger.** The 06.2 access-log emit-deferral hook fires from `directResponseAction.do`, `routerAction.do`, `h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`. Post-07.1, the router-action path moves into the router filter; the emit trigger needs to fire from the chain's terminal-completion path. Two viable sites: (a) at the end of `chain.runEncodeData(endStream=true)` / `chain.runEncodeTrailers` — uniform across all four 06.2 sites; (b) at the end of `chain.beginLocalReply` AND at the end of the natural router-path's encode (two sites, mirroring the directResponse-vs-router split). **Recommendation: (a)** — uniform; one site; matches the 06.2 chain-completion semantic. Planner records in PLAN.md.

2. **`HTTPRegistry.Lookup` race-vs-iterate during boot.** Two HCM constructors may race on `Lookup` during `listenerManager.New`'s loop. Both are RLock-only (lock-free against `Register` only on the post-Freeze path; pre-Freeze, `Register` Lock-vs-Lookup-RLock is naturally sequenced by the registry's `RWMutex`). The freeze invariant + the listener-manager's strict ordering (Freeze runs *before* `listenerManager.New`) make this safe by construction. Planner verifies the ordering at PLAN write time and asserts no `Register` happens after `Freeze` via a unit test in `registry_test.go`.

3. **Per-route config cache placement.** The cache lives on the per-stream `FilterChain`. Two viable shapes: (a) `map[filterName]proto.Message` allocated lazily on first `RequestRouteConfig` lookup, populated incrementally; (b) `[]proto.Message` indexed by filter chain index, allocated empty at chain construction. **Recommendation: (a)** — minimal allocation in the common case (filters that don't call `RequestRouteConfig` pay zero cost); slight map-lookup cost in the hot path is irrelevant compared to the iteration overhead. Planner records in PLAN.md.

4. **Concrete ADR numbers for ADR-0070..ADR-0076.** Per `DECISIONS.md` tail at master `2c65fcc` being ADR-0069, the next-free is ADR-0070; 07.1's seven ADRs land at ADR-0070..ADR-0076. The planner re-verifies next-free at write time (per ADR-0004's autonomous-numbering rule) and assigns the seven anticipated topics to the seven numbers in the order they're authored in PLAN.md. The topical ordering above (split / iteration-protocol / registry / per-route / filter-set / sendLocalReply / buffer-cap) is the suggested authoring order; the planner may permute.

5. **`FilterChain` decode/encode iteration index types.** Two viable shapes: (a) two `int` cursors (decodeIdx, encodeIdx) on the chain struct; (b) a single `iterState struct { idx int; phase decodeOrEncode }` with helpers. **Recommendation: (a)** — simpler; the two cursors are independent surfaces; helpers add no benefit at this scale. Planner records in PLAN.md.

6. **Chain-config mutation discipline at `chain.beginLocalReply`.** When a filter calls SendLocalReply, the chain marks decode aborted and skips remaining decode-side filters. The encode-side iteration starts from `len(filters)-1` regardless. Two viable shapes: (a) the filter[k]'s decode side that called SendLocalReply has its `OnDestroy` called when chain teardown happens; the encode side of that same filter still runs; (b) the filter is skipped entirely on encode (since it produced the response, it doesn't need to encode it). Per §11 #4 empirical pin, Envoy uses (a) — the calling filter's encode side runs (`lua_b` index 1's encode log line emits in §11 #4's trace). envoy-go MUST replicate (a). Planner verifies at PLAN time that the chain's encode-iteration-index advancement does NOT skip the calling filter.

7. **Test-only probe filter mode-dispatch storage.** The `envoy_go_test` filter has an internal table mapping `x-envoy-go-test-mode` header values → mode-dispatch logic. Two viable shapes: (a) a `map[string]func(*filter)` table; (b) a `switch mode { case "continue": ... }` body. **Recommendation: (b)** — a switch is more debuggable, makes the mode set discoverable from the source, and matches the 06.2 `directResponseAction` / `routerAction` precedent of "explicit dispatch over a closed enum". Planner records in PLAN.md.

8. **Fixture-0007a + 0007b driver pattern: in-band assertions vs. generic Driver-interface extension.** BRAINSTORM §2.5 outlines per-fixture in-band patterns. Both 0006 and 0005 went in-band; 0007a + 0007b should too. **Recommendation: in-band** — matches the 05.2 + 06.1 + 06.2 in-band precedent. Planner records in PLAN.md.

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 New `## HTTP filter chain` section (full population)

A new top-level section is added to `docs/envoy-go/BEHAVIOR_CONTRACT.md` between the existing `## HTTP/2` and `## TCP proxy` sections (or at the structural-equivalent place; the planner picks at the in-place-edit commit). The section is populated with the per-BRAINSTORM §7 subsections + the four §11 empirical-pin blocks verbatim:

```
## HTTP filter chain

*Introduced by phase 07.1. Justified by ADR-0070 (planner-time split), ADR-0071
(iteration protocol shape; supersedes ADR-0040 totally; partially supersedes
ADR-0042), ADR-0072 (HTTPRegistry threading), ADR-0073 (typed_per_filter_config
3-tier merge; amends ADR-0041), ADR-0074 (cors + envoy_go_test filter set),
ADR-0075 (sendLocalReply encode-chain entry semantics), ADR-0076 (1 MiB buffer
cap; 413 on decode overflow; reset on encode overflow; amends ADR-0041).*

### Asserted equivalence
- cors filter preflight response shape (status, header set, header values)
  byte-equal to reference Envoy v1.37.2 — verbatim scrape pinned in
  ### Empirical evidence (cors preflight) below.
- cors filter actual-request response header injection byte-equal.
- Filter declaration order on decode side; reverse on encode side — verbatim
  scrape evidence pinned in ### Empirical evidence (filter ordering) below.
- 413 Payload Too Large response shape on decode-side buffer overflow —
  verbatim scrape evidence pinned in ### Empirical evidence (413 overflow) below.
- typed_per_filter_config 3-tier merge precedence (Route > VirtualHost >
  RouteConfiguration); most-specific override (no field-merge).
- sendLocalReply enters encode chain at filter[len-1] of the encode-side
  filter set (reverse iteration start); full encode chain runs — verbatim
  scrape evidence pinned in ### Empirical evidence (sendLocalReply entry) below.

### Not asserted
- Behavioral equivalence of the test-only `envoy.filters.http.envoy_go_test`
  probe filter — structural assertion only (no reference Envoy implements it).
- Watermark backpressure event timing (out of MVP scope).
- 1xx informational header processing (out of MVP scope).
- Metadata frame processing (out of MVP scope).
- ContinueAndDontEndStream status semantics (out of MVP scope).
- Per-route filter `disabled` flag honoring (out of MVP scope).

### Buffer overflow behavior
- decode-side: 413 local reply, hardcoded constant 1 MiB
  (filterBufferLimitBytes), enters encode chain.
- encode-side: connection reset (H1: close; H2: RST_STREAM).
- per_connection_buffer_limit_bytes / per_request_buffer_limit_bytes
  silently ignored — extends ADR-0041 silent-ignore set.

### Async resume mechanics
- StopIteration parks dispatch goroutine on a per-stream resume channel.
- ContinueDecoding / ContinueEncoding callbacks unblock the channel.
- Single-goroutine-per-request iteration; no per-filter goroutine spawned
  by the framework.
- ctx.Done() during pause aborts iteration; OnDestroy fires for cleanup.

### Filter ordering
- http_filters[] declaration order on decode-side.
- Reverse declaration order on encode-side (router last on decode →
  router first on encode).
- Last entry MUST be the router (terminal filter); errors at parse otherwise.

### Empirical evidence (filter ordering)
[verbatim block from §11.1 above — paste-from-terminal Envoy stderr trace]

### Empirical evidence (cors preflight)
[verbatim blocks from §11.2 above — four wire-order response captures]

### Empirical evidence (413 overflow)
[verbatim blocks from §11.3 above — wire-order response capture + body hex dump]

### Empirical evidence (sendLocalReply entry)
[verbatim blocks from §11.4 above — Envoy stderr trace + wire-order response]

### Applies to
- Phase 07.1 onward (HTTP filter framework).

### Does not yet apply to
- Network filter chain (still phase 02 minimal — TCP-proxy or HCM as single
  entry per ADR-0033).
- Listener filters (deferred to 07.2).
- FilterChainMatch beyond SNI (deferred to 07.2).
- Listener.default_filter_chain (deferred to 07.2).
- HTTP filter family implementations beyond cors + envoy_go_test
  (incrementally landed by §9 family phases).
```

### 13.2 New `## Equivalence Matrix` row (transcribed from BRAINSTORM §7.3)

The `## Equivalence Matrix` table at `BEHAVIOR_CONTRACT.md` lines 9–23 gains a new row:

```
| Dimension              | Equivalence claim                                        | Allow-list / tolerance                |
|------------------------|----------------------------------------------------------|---------------------------------------|
| HTTP filter chain      | Per-request equivalence on cors preflight + actual-     | Differential covers cors only.        |
|                        | request response shapes (status + header set + body)    | envoy.filters.http.envoy_go_test      |
|                        | between envoy-go and reference Envoy. Filter            | excluded (test-only). All other       |
|                        | iteration order, sendLocalReply encode-chain entry,     | filters in the §9 family are          |
|                        | and 413 overflow shape are verbatim-pinned at the       | future-phase scope.                   |
|                        | ENVOY_TARGET SHA.                                       |                                       |
```

### 13.3 ADR-0040 / ADR-0042 supersession-note amendments to existing subsections

The existing `## HTTP/1.1` and `## HTTP/2` subsections of `BEHAVIOR_CONTRACT.md` mention HTTP-filter chain shape (e.g., the "exactly `[router]`" rule from ADR-0042). Those references are updated to "non-empty; last entry is router" — partial supersession, with a forward pointer to `## HTTP filter chain` for the full discipline. The phase-04 BEHAVIOR_CONTRACT mention of router-as-direct-call is removed in favor of router-as-terminal-filter discipline. Existing per-filter reasoning in `## HTTP/1.1 ### Asserted equivalence` is preserved verbatim — the wire-level claims don't change; only the dispatch path that produces those wire bytes does.

## 14. Testing strategy (per BRAINSTORM §6)

### 14.1 Unit tests (`internal/filter/http/`)

- **`registry_test.go`** — Register / Lookup / duplicate-name panic / Freeze / post-Freeze panic; concurrent `Lookup` calls (race-clean under `-race`).
- **`chain_test.go`** — decode-side iteration with all status combinations (Continue / StopIteration / StopIterationAndBuffer / StopIterationNoBuffer); encode-side reverse iteration; StopIteration + async resume via callback channel; SendLocalReply during decode → flows through encode (verifies §11 #4 entry semantics on synthetic mode); SendLocalReply twice → second is no-op + log; buffer overflow on decode → 413 (verifies §11 #3 verbatim shape on synthetic mode); buffer overflow on encode → connection reset; filter panic recovery → 503 local reply; ctx cancel mid-iteration; concurrent ContinueDecoding (race-clean); per-route config merge (3-tier override).
- **`perroute_test.go`** — typed_per_filter_config parse + merge + lookup with unknown filter name + missing scope; lazy cache hit/miss; nil return when no per-route config applies.
- **`callbacks_test.go`** — `ContinueDecoding` / `ContinueEncoding` semantics (idempotent send on full channel); `SendLocalReply` first-call-wins via `sync.Once`; `RequestRouteConfig` returns the merged proto.

### 14.2 Unit tests (`internal/filter/http/cors/`)

- **`cors_test.go`** — preflight detection (OPTIONS + Origin + ACRM); preflight allowed origin → 200 + the six CORS headers in the §11 #2 verbatim order; preflight disallowed origin → passthrough (does NOT inject 4xx); actual request → encodeHeaders adds the three CORS response headers per §11 #2 (c); actual request without Origin → no-op; per-route override (different allowed origins on different routes); the type_url + factory (`cors.New`) round-trip through the registry.

### 14.3 Unit tests (`internal/filter/http/envoygotest/`)

- **`filter_test.go`** — each `x-envoy-go-test-mode` value's expected behavior (per §7.3 table); per-route `count` config → response header injection; the 8-mode dispatch table is exhaustive.

### 14.4 Unit tests (`internal/filter/http/router/`)

- **`router_test.go`** / **`router_h2_test.go`** — extracted from existing `hcm/actions_test.go`. Tests preserved per BRAINSTORM §6.8 (byte-preserved test bodies; only imports + package names update). Test count drops marginally as some duplicate cases consolidate.

### 14.5 Unit tests (`internal/filter/hcm/`)

- **`config_test.go`** — modified: parses http_filters[] via registry; rejects empty / non-router-terminal / duplicate names / unknown type_url.
- **`connection_test.go`** / **`h2dispatch_test.go`** — modified: dispatch invokes FilterChain before terminal router; existing assertions preserved.
- **`actions_test.go`** — modified: routerAction tests deleted (moved to router pkg); directResponseAction tests stay.
- **NEW: `chain_integration_test.go`** — framework-runs-filters-then-router happy paths in both H1 and H2.

### 14.6 Differential fixture `0007a-cors`

(See §7.2 above for matrix + equivalence claim.) Per-request equivalence: status + response header set + body byte-equal across envoy-go and Envoy (modulo the existing differential ignore-list). Driver in-band; runner registers `RequiresReference: true`.

### 14.7 Structural fixture `0007b-iteration-probe`

(See §7.3 above.) envoy-go-only; driver in-band; runner registers `RequiresReference: false`. The 8-mode workload is per-mode-asserted against the embedded expectation table.

### 14.8 h2spec re-run (gate (c))

Per BRAINSTORM §6.4, phase 07.1 touches HCM dispatch wiring (H1 and H2). The h2spec gate at 53/53 must remain green — the dispatch-path change is between codec-decoded request and the route action, which is below the h2spec conformance surface. Existing gate (c) re-runs unchanged at the ADR-0051 SHA pin.

### 14.9 Fuzzers (gate (d))

Existing 8 fuzzers re-run at the 30s ADR-0018 budget. **NEW: `internal/filter/http.FuzzFilterChainParse`** — fuzzes adversarial `http_filters[]` slices into `parseFilterWithCtx` (varied type_urls, malformed typed_configs, oversized counts). Cheap (~80 LoC); adversarial-config bugs are the most likely class of bug in the new chain parser. Total: 9 fuzzers post-07.1.

### 14.10 Race detector + lint (gate (e))

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean across (per BRAINSTORM §5.5):

- N goroutines calling `ContinueDecoding` on the same chain (idempotent, coalesced).
- One filter's timer goroutine vs the dispatch goroutine racing on `cb.SendLocalReply`.
- Concurrent `HTTPRegistry.Lookup` calls from N HCM constructors at boot.
- Per-request chain teardown vs an in-flight `cb.ContinueEncoding` from a slow filter (`OnDestroy` semantics).

Unit tests in `chain_test.go` exercise each. Differential fixture `0007a-cors` indirectly stresses end-to-end concurrency.

## 15. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 07.1's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) **non-vacuous** (fixture 0007a differential green; fixture 0007b structurally green).
- [ ] `internal/filter/http/` package exists; `StreamDecoderFilter` + `StreamEncoderFilter` interfaces + status enums + callbacks defined in `types.go` + `callbacks.go`; `HTTPRegistry` + `Register` + `Lookup` + `Freeze` in `registry.go`; `FilterChain` + per-stream state machine in `chain.go`; `typed_per_filter_config` 3-tier merge in `perroute.go`; per-package `doc.go`.
- [ ] `internal/filter/http/cors/` exists; `cors.New` factory registered at boot; preflight + actual-request semantics match §11 #2 empirical pin verbatim.
- [ ] `internal/filter/http/envoygotest/` exists; 8-mode dispatch table per §7.3; hand-rolled proto in `proto/envoygotest.pb.go`; per-route `count` config exercised.
- [ ] `internal/filter/http/router/` exists; `routerAction` + `routerActionH2` migrated from `hcm/actions.go`; tests byte-preserved per BRAINSTORM §6.8.
- [ ] `internal/filter/hcm/actions.go` no longer contains `routerAction` or `routerActionH2`; `directResponseAction` stays.
- [ ] HCM constructor chain widens: `hcm.NewFilterWithCtxAndSinksAndRegistry(...)` exists; pre-existing `NewFilter` / `NewFilterWithCtx` / `NewFilterWithCtxAndSinks` either remain (forwarding to the new constructor with a default-empty registry) or are deleted with all call sites updated; the `cmd/envoy-go/main.go` call site updates.
- [ ] `cmd/envoy-go/main.go` allocates `*filter.HTTPRegistry`, registers router/cors/envoygotest, calls `Freeze()` BEFORE `listenerManager.New(...)`.
- [ ] **`HTTPRegistry` freeze-after-boot invariant enforced**: post-Freeze `Register("X", ...)` panics with `filter: registry frozen: cannot register %q post-boot`. Unit test in `registry_test.go` asserts the panic.
- [ ] **All four §11 empirical pins verbatim-present** in §11 of THIS SPEC (the SPEC commit, not a follow-up). Reviewer at REVIEW time grep-checks: (a) §11.1's Envoy stderr trace block (lua_a/lua_b decode → reverse encode); (b) §11.2's four wire-order response blocks (preflight allowed/disallowed + actual-request allowed/no-origin); (c) §11.3's 413 wire-order response + body hex dump; (d) §11.4's Envoy stderr trace (3 Lua filters; B calls respond; encode order C → B → A) + wire-order response.
- [ ] **`BEHAVIOR_CONTRACT.md ## HTTP filter chain` section landed at phase-done commit (NOT SPEC commit)** with the four §11 empirical-pin blocks verbatim, plus the iteration-protocol / buffer-overflow / async-resume / filter-ordering subsections per §13.1 above. The §11 block + the §13 block are paste-verbatim-synchronized (no drift; future image bumps require updating both in the same commit).
- [ ] `BEHAVIOR_CONTRACT.md ## Equivalence Matrix` has the new "HTTP filter chain" row from §13.2 above.
- [ ] `BEHAVIOR_CONTRACT.md ## HTTP/1.1` and `## HTTP/2` subsections updated for the ADR-0040 / ADR-0042 supersession (router-shape rule "exactly `[router]`" → "non-empty; last entry must be router"; forward-pointer to `## HTTP filter chain`).
- [ ] All seven 07.1 ADRs (the planner-assigned ADR-0070..ADR-0076 mapping to split / iteration-protocol / registry / per-route-merge / filter-set / sendLocalReply / buffer-cap) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. Inline-supersession notes in ADR-0071 (totally supersedes ADR-0040; partially supersedes ADR-0042) and ADR-0073 + ADR-0076 (amends ADR-0041) are explicit. The ADR-numbering-shift discipline from ADR-0045 + ADR-0004 is honored (the planner verified next-free at write time and the seven numbers are contiguous; topical-vs-commit-order non-monotonicity is permitted and recorded in each ADR's `Lands-in-task` field per the 06.2 ADR-0066..ADR-0069 precedent).
- [ ] Fixture `0007a-cors/` is committed in full: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The 4-request workload + per-request equivalence assertion shape is implemented; runner registers as `RequiresReference: true`.
- [ ] Fixture `0007b-iteration-probe/` is committed in full: `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The 8-mode workload + per-mode embedded-expectation-table assertion is implemented; runner registers as `RequiresReference: false`.
- [ ] `test/conformance/h2spec/` is UNCHANGED; pin still at the ADR-0051 SHA; 53/53 PASS.
- [ ] No phase-04 / 05.1 / 05.2 / 06.1 / 06.2 fixture (`0000`/`0001`/`0002`/`0003`/`0004`/`0005`/`0006`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `STATE.md` is at lifecycle-state 6 for 07.1; `ROADMAP.md` row `07.1` is `done`; row `07` (parent) stays `in-progress`; row `07.2` is `planned`. The §5.1 / §5.3 phase-done commit's message names every ADR introduced or referenced.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the phase-04..06.2 verification protocol; SHA-fill for each task entry per the convention.
- [ ] **`FuzzFilterChainParse` is committed** under `internal/filter/http/fuzz_test.go`; runs clean at the 30s ADR-0018 budget; total fuzzer count post-07.1 is 9.
- [ ] No third-party filter-chain-engine / filter-iteration library is imported. The `internal/filter/http` package's external dependencies are limited to the Go standard library (`sync`, `sync/atomic`, `io`, `net/http`, `time`, `fmt`, `errors`) plus `google.golang.org/protobuf` (proto runtime) and the upstream `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3` for the `Cors` + `CorsPolicy` proto types only.

When all boxes above are checked, phase 07.1 is `done`, the parent row `07` stays `in-progress` (closes only at 07.2's phase-done), and the project advances to phase 07.2 (listener-chain-completion) at lifecycle-state 1.

## 16. References

- **BRAINSTORM:** `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` — the authoritative design source; this SPEC distills BRAINSTORM §§2–9 into formal SPEC shape and pins the §2.6 empirical obligations. Every decision in this SPEC traces back to BRAINSTORM.
- **Parent master SPEC:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` — phase-07 parent; carries the cross-cutting decisions that apply to BOTH 07.1 and 07.2.
- **Sibling SPEC stub:** `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` — placeholder for 07.2; will be superseded by the 07.2 SPEC at lifecycle-state 1 of that sub-phase.
- **Structural precedent (sub-phase SPEC shape):** `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` and `docs/envoy-go/phases/06.2-access-log/SPEC.md` — the §-numbering, header tone, acceptance-bullet format, and overall shape this SPEC mirrors. The empirical-pin block in §11 of this SPEC mirrors 06.1's Rule SN4 verbatim-pin block (in `BEHAVIOR_CONTRACT.md ## Stat-name mapping`) and 06.2's verbatim access-log pin (per ADR-0066).
- **Structural precedent (parent master SPEC shape):** `docs/envoy-go/phases/05-http-2/SPEC.md` and `docs/envoy-go/phases/06-observability-baseline/SPEC.md`.
- **BEHAVIOR_CONTRACT.md:** `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the contract this SPEC's §13 extensions land in (in-place edit at phase-done per ADR-0052).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Cited verbatim in each §11 empirical-pin sub-block. The four pin probes used the image at this SHA against bootstraps shaped to elicit each pinned shape (configs in `/tmp/envoy-07.1-empirical/` during the SPEC session; not committed to repo since they're SPEC-time scaffolding, not fixture artifacts).
- **Phase-04 SPEC partial-supersession references:** `docs/envoy-go/phases/04-http-1.1/SPEC.md` — the router-as-direct-call sections that ADR-0040 documents and ADR-0071 totally supersedes; the "exactly `[router]`" rule that ADR-0042 documents and ADR-0071 partially supersedes (lower bound stays, upper bound lifted).
- **DECISIONS.md:** `docs/envoy-go/DECISIONS.md` — ADR-0001 (template), ADR-0004 (autonomous-numbering rule + autonomous-brainstorming adaptation under which THIS SPEC was authored), ADR-0008 (Envoy pin, referenced via ENVOY_TARGET.md), ADR-0010 (`dns_lookup_family: V4_ONLY` for STRICT_DNS reference clusters), ADR-0017 (small-mechanical-fixes do not require ADRs), ADR-0018 (fuzzer 30s short-budget policy), ADR-0028 (`--concurrency 1` reference invocation), ADR-0040 (totally superseded by ADR-0071), ADR-0041 (amended by ADR-0073 and ADR-0076), ADR-0042 (partially superseded by ADR-0071), ADR-0045 (planner-time-split discipline; reused for the 07.1 + 07.2 split per ADR-0070), ADR-0051 (h2spec pin SHA), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization), ADR-0061 (Rule SN4 empirical-pin pattern this SPEC's §11 mirrors), ADR-0066 (06.2 access-log empirical-pin pattern this SPEC's §11 also mirrors), ADR-0069 (last extant ADR at master `2c65fcc`; the 07.1 ADRs start at ADR-0070).
- **BOOTSTRAP_PROMPT cross-references:**
  - **§5** (Phase Lifecycle State Machine) — the lifecycle states 1 (SPEC drafting; this commit's deliverable) → 6 (REVIEW approved + phase-done) that 07.1 traverses.
  - **§5.3** (Commit message format) — the phase-done commit message format `phase 07.1: http-filter-framework [ADR-0070, ADR-0071, ..., ADR-0076]` plus differential-surface + conformance summary.
  - **§6.2** (How to split — planner-time-split discipline) — the discipline ADR-0045 invokes for the 07.1 + 07.2 split; this SPEC honors §6.2 by being one of two sibling sub-phase SPECs under the parent.
  - **§7.5** (Phase-done gate — six-gate checklist) — the gate set §3 specializes for 07.1.
  - **§4.1** (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit) — the row-flip discipline §4.4 honors.
- **ROADMAP.md:** `docs/envoy-go/ROADMAP.md` — rows `07`, `07.1`, `07.2` per the split landed in this commit's ROADMAP edit.
- **PROGRESS-style precedents:** `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md`, `docs/envoy-go/phases/06.2-access-log/PROGRESS.md` — the SHA-fill convention 07.1's PROGRESS.md will mirror.
