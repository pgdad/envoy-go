# Phase 18.1 — HTTP filter `envoy.filters.http.ext_authz` (HTTP service mode) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.ext_authz` in **HTTP service mode** — Envoy v1.37.2's canonical external-authorization filter delegating the allow/deny decision to an external HTTP service over an outbound POST — as the foundational half of the ELEVENTH §9 production HTTP filter, with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis except the documented divergence-windows.

**Architecture:** NEW `internal/filter/http/extauthz/` package — a DECODER-only `StreamDecoderFilter` (`Encoder: nil`; 5th §9 row to ship pure decode-side after csrf @ 12, buffer @ 13, rbac @ 16, jwt_authn @ 17). The `ExtAuthz.services` oneof is resolved at config-load time into a `checkFn` closure on a mode-agnostic `compiledConfig`; in 18.1 the `http_service` arm builds a thin ext_authz-local `httpAuthClient` (ADR-0159 disposition (b) — NOT a generalized `internal/httpclient/` package), and the `grpc_service` arm PARSE-REJECTs (activated by 18.2). `DecodeHeaders` resolves per-route (5th-canonical REUSE), optionally buffers the request body via the phase-13 ADR-0128 decode-side body-buffering primitive, builds the request-side-filtered `AuthorizationRequest`, and fires the outbound POST on the phase-09 fault async-resume primitive (`StopIteration` + goroutine + `cb.ContinueDecoding()`); on resume the mode-agnostic disposition-application logic converges on `{allow, deny, error}` — allow-path bidirectional header injection + optional `clear_route_cache`; deny-path `SendLocalReply` with the auth service's verbatim status/body/`allowed_client_headers`-filtered headers; error-path `failure_mode_allow` / `status_on_error` posture. Package split: `extauthz.go` (filter type + factory + decode methods + `filterStats` + `compiledConfig` + `compiledPerRoute` + per-route resolver), `check.go` (the HTTP-outbound auth-check — `httpAuthClient` + `buildHTTPCheckFn` + the HTTP-response → `checkDisposition` mapping + the `failure_mode_allow` / error-classification boundary), `attributes.go` (the HTTP-mode `AuthorizationRequest` builder + the request-side header filtering through the top-level `ExtAuthz.allowed_headers`/`disallowed_headers`), `extauthz_test.go` (unit tests), `fuzz_test.go` (the 22nd fuzzer `FuzzExtAuthzConfigParse`), `doc.go` (package overview). One NEW shared test-helper at `test/helpers/extauthzhttp/` (in-process scriptable HTTP auth server, spawn-per-fixture). Differential fixture `0020-http-ext-authz-http` with 7 HTTP-mode scenarios. The per-route surface is the **5th-canonical REUSE** (NO new ADR-0125 canonical — the FIRST §9 row to REUSE one) with SHARED-stats; the stat surface is 6 counters under `http.<HCM_stat_prefix>.ext_authz.*` (SN2-reuse). 18.1 introduces ONE new framework primitive (ADR-0159 — the HTTP-outbound auth-check, a thin local client) and REUSES four (phase-09 async-resume, phase-13 ADR-0128 body-buffering, phase-17 ADR-0150 outbound-HTTP structure composed-against, ADR-0085 `SendLocalReply`).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/ext_authz/v3` + `envoy/type/matcher/v3` + `envoy/config/core/v3` + `envoy/type/v3`); `protojson`/`anypb` for proto decoding; `net/http.Client` Go stdlib for the outbound auth POST; `context.Context` for per-request cancellable outbound calls; `time.AfterFunc`-free async-resume — the outbound call runs in a plain goroutine that calls `cb.ContinueDecoding()` (phase-09 fault precedent); `sync.Map` for the per-route lazy-cache (ADR-0117 + ADR-0125 §(v) precedent); `internal/matcher` for `StringMatcher`/`ListStringMatcher` compilation; `internal/stats` for the 6-counter `filterStats`; reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext fixture (no H2 differential coverage; no TLS, no PKI — phase 18.1 fixture is plaintext-only).

---

## Scope check — why phase 18.1 ships as one row (it already is the split half)

Phase 18 was SPLIT into `18.1-ext-authz-http` + `18.2-ext-authz-grpc` at the phase-18 SPEC commit (`308e9b6`) per ADR-0045 / ADR-0164 — the §5 13-pin empirical-pin resolution against reference Envoy v1.37.2 (28-field `ExtAuthz` proto surface; two structurally distinct transports; brand-new `internal/grpcclient/` gRPC infrastructure; ~2400–3600 LoC combined) CONFIRMED the BRAINSTORM §1.4 HIGH split-readiness leaning. This PLAN is for the 18.1 sub-phase ONLY (HTTP service mode + the filter scaffold); 18.2 (gRPC service mode + `internal/grpcclient/`) is a separate sub-phase with its own SPEC/PLAN/IMPL lifecycle.

Net change estimate for 18.1 (mirroring the phase-09..17 PLAN component-table convention):

- `internal/filter/http/extauthz/doc.go` ~35
- `internal/filter/http/extauthz/extauthz.go` ~420–560 (filter + factory + `compiledConfig` + `compiledPerRoute` + `filterStats` + `checkDisposition` + `checkFn` typedef + `buildCompiledConfig` + `parsePerRoute` + `resolvePerRouteConfig` + `newFilterStats` + `DecodeHeaders`/`DecodeData`/`DecodeTrailers`/`OnDestroy`/`SetDecoderCallbacks` + `New` factory + the async-resume leg + the disposition-application logic)
- `internal/filter/http/extauthz/check.go` ~260–400 (`httpAuthClient` + `buildHTTPCheckFn` + the POST construction + the HTTP-response → `checkDisposition` mapping + the §5.P10 error-classification boundary + `path_prefix` prepend)
- `internal/filter/http/extauthz/attributes.go` ~200–340 (the `AuthorizationRequest` builder + `compileStringMatcherList` + request-side header filtering through `allowed_headers`/`disallowed_headers` + `headers_to_add` + the deprecated `AuthorizationRequest.allowed_headers` honored-if-present disposition + the response-side `AuthorizationResponse` matcher compilation + `validate_mutations` validation)
- `internal/filter/http/extauthz/extauthz_test.go` ~1100–1800 (9 unit-test groups per SPEC §14.1)
- `internal/filter/http/extauthz/fuzz_test.go` ~85 (22nd fuzzer `FuzzExtAuthzConfigParse`)
- `cmd/envoy-go/main.go` ~+3 (1 import + 1 register line)
- `test/helpers/extauthzhttp/doc.go` ~25 + `test/helpers/extauthzhttp/extauthzhttp.go` ~110–150 + `test/helpers/extauthzhttp/extauthzhttp_test.go` ~85 (NEW shared test-helper)
- `test/differential/fixture/fixture.go` ~+15 (`HTTPExtAuthzHTTP BackendKind = 17`)
- `test/differential/runner_test.go` ~+12 (blank import + switch-case)
- `test/fixtures/0020-http-ext-authz-http/` (NEW DIRECTORY) — `envoy.yaml` ~150 + `envoy-go.yaml` ~150 + `expectations.yaml` ~70 + `README.md` ~110 + `inputs/driver.go` ~300 = ~780
- `docs/envoy-go/DECISIONS.md` — 7 ADRs (ADR-0156/0157/0159/0160/0161/0162/0163) §Decision + §Consequences bodies authored at impl-time per ADR-0044 (§Context drafts already landed at SPEC commit `308e9b6`); ~+260 LoC
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+220 (§13 6-patch bundle)
- `docs/envoy-go/ROADMAP.md` ~+1 net; `docs/envoy-go/STATE.md` rewrite-in-place
- `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (NEW) ~600 + `REVIEW.md` (NEW) ~240

**Production code: ~915–1335 LoC** (filter impl `extauthz.go` + `check.go` + `attributes.go` + `doc.go`) **+ ~3 LoC main.go + ~135–175 LoC test-helper = ~1053–1513 LoC production** + ~1185–1885 LoC tests + ~780 LoC fixture + ~980 LoC docs ≈ **~4000–5160 LoC total**. Task count below is **15** — comfortably under the ADR-0045 25-task split-gate. The production-LoC high-end (~1513) brushes the ~1500-LoC soft threshold, but per the phase-13/14/15/16/17 LoC-borderline precedent (task count is the load-bearing split trigger, not LoC) and the fact that 18.1 IS ALREADY a sub-phase row, **18.1 ships as the single row it is** — no further split.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/extauthz/doc.go` | NEW | Package doc enumerating: (a) the dual-mode service envelope (`http_service` consumed in 18.1; `grpc_service` PARSE-REJECTs — "lands in 18.2"); (b) the 18.1-consumed `ExtAuthz` field set per parent SPEC §5.P1 (`http_service`, `transport_api_version` V3-only, `with_request_body`, `failure_mode_allow`, `failure_mode_allow_header_add`, `clear_route_cache`, `status_on_error`, `validate_mutations`, `allowed_headers`, `disallowed_headers`, `stat_prefix` + `HttpService.{server_uri, path_prefix, authorization_request, authorization_response}`) + the ~13 silent-ignored fields per SPEC §2.2/§8; (c) the DECODER-only filter shape (5th §9 row pure decode-side); (d) the per-route 5th-canonical REUSE (`oneof override{disabled(bool, const true) \| check_settings}`; SHARED-stats; NO ADR-0125 amendment); (e) the 6-counter `filterStats` (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`; `disabled` STRUCTURALLY UNREACHABLE under MVP); (f) the async-resume outbound-call leg + per-request cancellable `context.Context` cancelled at `OnDestroy`; (g) the deny/error wire shapes; (h) public API surface (`TypeURL` const, `New` factory); (i) the divergence-windows (`allowed_client_headers_on_success` DEFERRED — decode-side-only shape; `response_code_details` not emitted; dynamic-metadata family silent-ignored; cluster-scoped `cluster.<upstream>.ext_authz.*` triple DEFERRED); (j) ADR anchors (ADR-0156/0157/0159/0160/0161/0162/0163 + ADR-0125 §(v) 5th-canonical REUSE). Mirrors `rbac/doc.go` + `jwtauthn/doc.go` shape. Per SPEC §1 + §13. |
| `internal/filter/http/extauthz/extauthz.go` | NEW | Main filter file. **Public surface** (per SPEC §6.1): `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"`; `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory (mirrors the cors/.../jwtauthn `HTTPFilterFactory` shape per `internal/filter/http/types.go:243`). **Internal const:** `filterName = "envoy.filters.http.ext_authz"`. **Types** (per SPEC §6.2): `compiledConfig` (`checkFn` + `withRequestBody *bufferSettings` + `failureModeAllow`/`failureModeAllowHeaderAdd`/`clearRouteCache` bools + `statusOnError uint32` default 403 + `validateMutations` bool + `allowedHeaders`/`disallowedHeaders *stringMatcherList` + `stats *filterStats` — **mode-agnostic, field-final at 18.1**, 18.2 adds no field); `bufferSettings` (`maxRequestBytes uint32` + `allowPartialMessage` + `packAsBytes` bools); `checkDisposition` (`class dispositionClass` enum allow\|deny\|error + `upstreamSet`/`upstreamApp []headerKV` + `denyStatus uint32` + `denyBody []byte` + `denyHeaders []headerKV`); `dispositionClass` enum const block (`dispAllow`/`dispDeny`/`dispError`); `headerKV` (`name`/`value string`); `checkFn` typedef (`func(ctx context.Context, req *authRequest) (checkDisposition, error)`); `authRequest` (the request-side-filtered POST inputs — method, path-with-`path_prefix`, filtered headers, body); `compiledPerRoute` (`disabled bool` + `*compiledCheckSettings`); `compiledCheckSettings` (`contextExtensions map[string]string` — parsed, no-op in HTTP-mode + `withRequestBody *bufferSettings` + `disableRequestBodyBuffering bool`); `factoryState` (`listenerRC *compiledConfig` + `perRoute sync.Map` keyed by `*ExtAuthzPerRoute` proto pointer); `filter` (per-stream: `state *factoryState` + `dcb envoyhttp.DecoderFilterCallbacks` + `activeRC *compiledConfig` + `perRoute *compiledPerRoute` + `awaitingBody bool` + buffered-body accumulator + `callCtx context.Context` + `callCancel context.CancelFunc` + the async-resume guard `mu sync.Mutex` + `done bool`); `filterStats` (6 `*stats.Counter`: `ok`/`denied`/`errored`/`disabled`/`failureModeAllowed`/`invalid` — `errored` avoids the Go `error` keyword; `disabled` registered but STRUCTURALLY UNREACHABLE under MVP). **Helpers:** `buildCompiledConfig(ctx, raw *ext_authzv3.ExtAuthz) (*compiledConfig, error)` (per SPEC §6.4: `services` oneof dispatch — `nil` → PARSE-REJECT `ext_authz: services oneof must be set`, `*ExtAuthz_GrpcService` → PARSE-REJECT `ext_authz: grpc_service mode not yet supported (lands in phase 18.2)`, `*ExtAuthz_HttpService` → `buildHTTPCheckFn` from check.go; non-V3 `transport_api_version` → PARSE-REJECT per ADR-0008; `with_request_body` → validate `max_request_bytes > 0` → `*bufferSettings`; `status_on_error` default 403; `allowed_headers`/`disallowed_headers` → `compileStringMatcherList` from attributes.go; `filterStats` allocated guarded `if ctx.Stats != nil`); `parsePerRoute(raw *ext_authzv3.ExtAuthzPerRoute) (*compiledPerRoute, error)` (validate `override` oneof PGV-required; `disabled` arm → validate `const: true`, PARSE-REJECT `disabled: false`; `check_settings` arm → compile `context_extensions` + `disable_request_body_buffering` XOR `with_request_body`); `(s *factoryState) resolvePerRouteConfig(msg)` (3-tier TPFC resolution Route > VirtualHost > RouteConfiguration > listener-fallback via the existing machinery; `sync.Map` `LoadOrStore` lazy-cache keyed by proto pointer per ADR-0117 + ADR-0125 §(v)); `newFilterStats(reg, baseStatPrefix) *filterStats` (6 counters registered unconditionally; nil-tolerant per ADR-0085). **`DecodeHeaders` body** (per SPEC §6.3): resolve per-route → cache `*compiledPerRoute`; `perRoute.disabled` → `HeaderContinue` (NO auth call, NO counters); effective `withRequestBody` = per-route override OR listener-level; if set AND `!endStream` → `awaitingBody = true` + `HeaderStopIteration` (DecodeData accumulates; over-limit + `allow_partial_message:false` → `SendLocalReply(413, "Payload Too Large", {connection: close})` BEFORE the check, NO counters); else build `authRequest` (attributes.go) → fire the async outbound check (`HeaderStopIteration`; goroutine runs `cc.checkFn(callCtx, authReq)`; on completion the resume path applies the disposition under the `mu`/`done` guard). **Disposition-application** (mode-agnostic; on the resume path): `allow` → apply `upstreamSet` (overwrite) + `upstreamApp` (append) to the upstream request headers; `clear_route_cache` → `cb.ClearRouteCache()`; `cc.stats.ok++`; `ContinueDecoding`. `deny` → `cb.SendLocalReply(denyStatus, denyBody, denyHeaders)`; `cc.stats.denied++`. `error` → `failure_mode_allow:false` → `cb.SendLocalReply(statusOnError, "", {})` + `cc.stats.errored++`; `failure_mode_allow:true` → if `failure_mode_allow_header_add` add `x-envoy-auth-failure-mode-allowed: true` upstream; `cc.stats.errored++` + `cc.stats.failureModeAllowed++`; `ContinueDecoding`. `invalid` (validate_mutations rejection) → `cc.stats.invalid++`; treat as error posture. **`DecodeData`** participates in the ADR-0128 body-buffering interaction when `awaitingBody` (else pass-through). **`DecodeTrailers`** pass-through. **`OnDestroy`** calls `callCancel()` + sets `done` under `mu` (the async-resume-after-OnDestroy race guard — planner-time decision D4). **`SetDecoderCallbacks`** stores `dcb`. `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` compile-time assertion. ~420–560 LoC. |
| `internal/filter/http/extauthz/check.go` | NEW | The HTTP-outbound auth-check primitive (ADR-0159 disposition (b)). **`httpAuthClient`** — a thin ext_authz-local type wrapping `*http.Client` (`&http.Client{Timeout: hs.server_uri.timeout}`) + the parsed `path_prefix` + the compiled `authorization_request`/`authorization_response` matcher sets. Structurally mirrors the phase-17 `internal/jwks/Fetcher`'s `http.Client`/timeout discipline WITHOUT the cache/async-refresh machinery (per SPEC §3.1 — the two consumers have structurally different lifecycles; generalizing now is premature; the oauth2 phase is the natural generalize-to-`internal/httpclient/` trigger). **`buildHTTPCheckFn(hs *ext_authzv3.HttpService, ar *authRequestCfg, validateMutations bool) (checkFn, error)`** (per SPEC §6.5): validate `hs.server_uri` set + non-empty `uri` (PGV-mirror — `HttpService.server_uri` is NOT PGV-required; the factory rejects an empty one); construct the `httpAuthClient`; compile `hs.authorization_response.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` → `*stringMatcherList` triple (via `compileStringMatcherList` from attributes.go); return a `checkFn` closure. **The `checkFn` closure:** builds the outbound POST (`path_prefix` prepended to the path; the request-side-filtered headers from `authRequest`; the body when `with_request_body` is set); `client.Do(req.WithContext(ctx))`; maps the HTTP response → `checkDisposition` per the **§5.P10 error-classification boundary**: status `200` → **allow** (extract `allowed_upstream_headers` → `upstreamSet`, `allowed_upstream_headers_to_append` → `upstreamApp`); a recognized deny status (`401`/`403` per §5.P10) → **deny** (status + verbatim body + `allowed_client_headers`-filtered headers, `content-type` `text/plain` fallback, decision-headers-first ordering per §4); connect failure / timeout / context-cancelled / unrecognized status → **error** (the closure returns `(checkDisposition{class: dispError}, err)`); a `validate_mutations` rejection on the extracted header set → **invalid** disposition. ADR-0159 anchors the primitive + the (a)-vs-(b) record + the oauth2-generalization forward-pointer; ADR-0157 anchors the error-classification boundary. ~260–400 LoC. |
| `internal/filter/http/extauthz/attributes.go` | NEW | The HTTP-mode `AuthorizationRequest` builder + the `StringMatcher` machinery (ADR-0160 HTTP-mode portion). **`compileStringMatcherList(lsm *matcherv3.ListStringMatcher) (*stringMatcherList, error)`** — compiles a `ListStringMatcher` into the internal matcher set: exact/prefix/suffix/contains + `ignore_case` honored proto-faithful via `internal/matcher`; `safe_regex` reuses the phase-09/12 RegexMatcher-subset discipline (planner-time decision D5 — `google_re2` arm honored over Go `regexp` RE2; other engine arms PARSE-REJECT); `custom` PARSE-REJECTs envoy-go-strict (a `TypedExtensionConfig` plugin point — no envoy-go string-matcher extension registry). **`stringMatcherList`** internal type with a `matchAny(name string) bool` method. **`buildAuthRequest(f *filter, headers, body) *authRequest`** (per SPEC §1 item 6): filter the client request headers through `cc.allowedHeaders` (allow-list; `nil` = all pass), then REMOVE any header matching `cc.disallowedHeaders` (overrides `allowed_headers` per the proto doc), then append `hs.authorization_request.headers_to_add` static headers (+ the deprecated `AuthorizationRequest.allowed_headers` honored-if-present per planner-time decision D6); the path carries `path_prefix` prepended (done in check.go's closure, sourced here); include the body when `with_request_body` is set. **`validateMutationHeaders(hdrs []headerKV) error`** — the `validate_mutations` rule set (planner-time decision D7 — mirrors the phase-10 header_mutation protected-header discipline: reject `:`-prefixed pseudo-headers + invalid header-name characters + invalid header-value characters); a rejection drives the `invalid` disposition + counter. ~200–340 LoC. |
| `internal/filter/http/extauthz/extauthz_test.go` | NEW | Unit tests per SPEC §14.1 — single file for 18.1 (planner-time decision D3; revisit a `check_test.go` split only if it exceeds ~1800 LoC at impl time). **Group 1 — `ExtAuthz` parse:** `http_service` valid; `grpc_service` PARSE-REJECT (`grpc_service mode not yet supported (lands in phase 18.2)`); empty `services` oneof PARSE-REJECT; non-V3 `transport_api_version` PARSE-REJECT; `with_request_body` `max_request_bytes` 0 PARSE-REJECT; `HttpService.server_uri` unset / empty-`uri` PARSE-REJECT; `status_on_error` default 403; `stat_prefix` consumed. **Group 2 — `compiledConfig` shape:** 6-counter `filterStats` allocation; nil-`Stats` tolerance; `compiledConfig` field-finality (mode-agnostic; no transport-specific state). **Group 3 — `allowed_headers`/`disallowed_headers` matcher compilation:** exact/prefix/suffix/contains/`ignore_case`/`safe_regex`; `custom` PARSE-REJECT; `disallowed_headers` overrides `allowed_headers`. **Group 4 — HTTP-mode `checkFn`:** allow (200 + `allowed_upstream_headers`); deny (401/403 + body + `allowed_client_headers`); error (connect failure / timeout / unrecognized status); `path_prefix` prepend; `headers_to_add` appended; deprecated `AuthorizationRequest.allowed_headers` honored-if-present. **Group 5 — `DecodeHeaders` dispatch:** per-route `disabled` short-circuit (NO counters); async-resume allow/deny/error; `clear_route_cache` → `cb.ClearRouteCache()`; `failure_mode_allow` true/false posture; `failure_mode_allow_header_add`. **Group 6 — `with_request_body`:** body materialization via ADR-0128; over-limit + `allow_partial_message:false` → `SendLocalReply(413, "Payload Too Large", {connection: close})` + NO counters; `allow_partial_message:true`; `pack_as_bytes`. **Group 7 — per-route:** `parsePerRoute` PGV-mirror (`disabled: false` PARSE-REJECT, empty-`override` PARSE-REJECT); 3-tier resolution; `check_settings` merge (`disable_request_body_buffering` XOR `with_request_body`); SHARED-stats (no per-route `*filterStats`); `sync.Map` lazy-cache identity. **Group 8 — header mutation:** allow-path `upstreamSet`/`upstreamApp` upstream injection; deny-path `allowed_client_headers` filtering + `text/plain` fallback + decision-headers-first ordering; `validate_mutations` rejection → `invalid` counter. **Group 9 — `OnDestroy` cancellation:** in-flight `context.Context` cancelled; resume-after-`OnDestroy` guarded (no panic, no callback touch after `done`). Test helpers `mustAny(t, msg)` + `freshFactoryCtx()` + `freshFactoryCtxWithRegistry()` + `freshFilter(t, cfg)` + `scriptableAuthServer(t, script)` mirror phase-16/17 precedents. ~1100–1800 LoC. |
| `internal/filter/http/extauthz/fuzz_test.go` | NEW | `FuzzExtAuthzConfigParse` — fuzzes arbitrary bytes as the `tc *anypb.Any` to `New`. Asserts: `New` returns `(factory, nil)` OR `(nil, error)`; never panics; never `(nil, nil)`. Corpus seeds per SPEC §7.3: `http_service` valid / empty-`uri`; `grpc_service` (PARSE-REJECT path); empty `services` oneof (PARSE-REJECT); `with_request_body` `max_request_bytes` 0 / positive; per-route both arms (`disabled: true` / `disabled: false` / `check_settings`); `allowed_headers`/`disallowed_headers` matcher variants (incl. `custom` PARSE-REJECT); error-posture field combinations (`failure_mode_allow` × `failure_mode_allow_header_add` × `status_on_error`). 30s ADR-0018 budget; **22nd fuzzer overall**. ~85 LoC. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(extauthz.TypeURL, extauthz.New)` inserted between the existing `httpReg.Register(envoygotest.TypeURL, envoygotest.New)` (line ~126) and `httpReg.Register(fault.TypeURL, fault.New)` (line ~127) per ADR-0100 §2.2 alphabetical-after-router ordering (`extauthz` sorts between `envoygotest` and `fault`); plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/extauthz"` alphabetically among the filter-package imports. The resulting `httpReg.Register` block reads `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → extauthz → fault → header_mutation → jwtauthn → localratelimit → rbac → Freeze`. ~+3 LoC. Per SPEC §1 item 2 + ADR-0072 (registration order does not affect runtime behavior; stylistic discipline only). |
| `test/helpers/extauthzhttp/doc.go` | NEW | Package doc — `// Package extauthzhttp implements a minimal in-process scriptable HTTP authorization server for differential fixtures whose driver needs to wire an ext_authz http_service endpoint into both envoy.yaml and envoy-go.yaml. Used by phase 18.1 fixture 0020-http-ext-authz-http. Lifecycle: spawn-per-fixture; the runner allocates a free TCP port, starts the auth server via New(), wires the http_service.server_uri to that port in both yaml configs, runs the scenarios, then stops via Stop(). Scenario 3+4 (auth-server-unreachable) stop the server before the request.` ~25 LoC. Planner-time decision D1. |
| `test/helpers/extauthzhttp/extauthzhttp.go` | NEW | In-process scriptable HTTP auth server (planner-time decision D1 — mirrors `test/helpers/jwksbackend/` structure). **Public API:** `Server` type carrying `Listener net.Listener` + opaque state; `New(ctx context.Context, addr string, script Script) (*Server, error)` starts the server (`addr` e.g. `"127.0.0.1:0"` for ephemeral port); `Script` configures the per-request response — fixed status + body + headers, OR a per-path/per-method map, OR a body-inspecting predicate (for scenario 5 `with_request_body`); the HTTP handler returns the scripted status/body/headers; `(s *Server) Stop()` terminates cleanly via `srv.Shutdown(ctx)`; `(s *Server) Addr() string` returns the listener's actual address. **Library:** Go stdlib `net.Listen("tcp", addr)` + `http.Server.Serve` + `srv.Shutdown(ctx)`. ~110–150 LoC. Per SPEC §7.2. |
| `test/helpers/extauthzhttp/extauthzhttp_test.go` | NEW | Unit tests: `TestNew_StartsServerOnConfiguredAddr`; `TestServer_FixedScript_ReturnsStatusBodyHeaders`; `TestServer_PathMethodMap_Dispatch`; `TestServer_BodyInspectingScript`; `TestServer_Stop_ClosesListener`; `TestServer_ConcurrentClient_NoRace`. ~85 LoC. |
| `test/differential/fixture/fixture.go` | MODIFIED | NEW `BackendKind` enum value `HTTPExtAuthzHTTP BackendKind = 17` after the existing `HTTPJwtAuthn BackendKind = 16`. Doc-comment: "HTTPExtAuthzHTTP reuses the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go` for the upstream route + the NEW extauthzhttp helper at `test/helpers/extauthzhttp/` for the in-process HTTP auth server. 2-cluster topology (one HCM listener `l_test_a` plaintext with `ext_authz → router` filter chain + cluster `c_backend` → echobackend subprocess + cluster `c_authz` → extauthzhttp subprocess). No mTLS, no TLS, no PKI — phase 18.1 fixture is HTTP/1.1 plaintext-only." ~+15 LoC. |
| `test/differential/runner_test.go` | MODIFIED | NEW blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0020-http-ext-authz-http/inputs"` (alphabetical-after `0019`). NEW switch-case in the `BackendKind` dispatch for `HTTPExtAuthzHTTP` reusing the existing `startEchoBackend` helper + spawning an `extauthzhttp.New` instance per-test for the in-process auth server (scenarios 3+4 stop it before the request). ~+12 LoC. |
| `test/fixtures/0020-http-ext-authz-http/` | NEW DIRECTORY | Differential fixture with 7 scenarios per SPEC §7. Plaintext-only topology: 1 echo-backend cluster + 1 auth-server cluster + 1 listener `l_test_a` HCM with `ext_authz → router` filter chain. |
| `test/fixtures/0020-http-ext-authz-http/envoy.yaml` | NEW | Reference Envoy bootstrap. Listener `l_test_a` (TCP plaintext; HCM chain `ext_authz → router`) with listener-level `ExtAuthz` config (HTTP-mode: `http_service.server_uri` → cluster `c_authz`; `allowed_headers`/`disallowed_headers`; `with_request_body` for scenario 5; `failure_mode_allow` + `failure_mode_allow_header_add` + `status_on_error` for scenarios 3+4 — distinct per-listener or per-route as scenario topology requires). Routes: `/` → `c_backend` (scenarios 1–5); `/per-route-disabled` with per-route TPFC `ExtAuthzPerRoute{disabled: true}` (scenario 6); `/per-route-check-settings` with per-route TPFC `ExtAuthzPerRoute{check_settings{disable_request_body_buffering: true}}` (scenario 7). Cluster `c_backend` STRICT_DNS → echobackend subprocess. Cluster `c_authz` STRICT_DNS → extauthzhttp subprocess. ~150 LoC. Per SPEC §7.2. NOTE: scenarios 3+4 may need a separate listener OR a per-route ext_authz override pointing at an unreachable URI — the IMPL session settles the exact topology shape (likely a second listener `l_test_unreachable` or a `c_authz_down` cluster pointing at a closed port). |
| `test/fixtures/0020-http-ext-authz-http/envoy-go.yaml` | NEW | Equivalent envoy-go bootstrap. Same listener + routes + per-route map; cluster type STATIC. ~150 LoC. Per SPEC §7.2. |
| `test/fixtures/0020-http-ext-authz-http/inputs/driver.go` | NEW | Go driver issuing the 7 scenarios per SPEC §7.1 mirroring the phase-16/17 driver shape. Functions `runScenario1..runScenario7(ctx, baseURL, authBaseURL) error`. Per-scenario assertion: byte-exact body (allow paths backend-echo verbatim; deny paths the auth server's verbatim body) + response status equivalence + `/stats/prometheus` counter-delta equivalence on the 5 reachable counters (`ok`/`denied`/`error`/`failure_mode_allowed`/`invalid`) + backend-arrival header assertions (request-side filtering + allow-path upstream injection + the `x-envoy-auth-failure-mode-allowed` header). **extauthzhttp lifecycle helper** `setupAuthServer(t, ctx, port, script)`; teardown via `srv.Stop()`. **Counter-delta helper** `scrapeStats(t, baseURL)` + `assertCounterDelta(...)` mirrors phase-16/17. ~300 LoC. Per SPEC §7.2. |
| `test/fixtures/0020-http-ext-authz-http/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7. Documents the 7-scenario equivalence claim + the per-route 5th-canonical scenarios 6+7 + the divergence-window allow-list (`response_code_details` field ABSENT on the envoy-go side; `disabled` counter STRUCTURALLY UNREACHABLE — NOT asserted; cluster-scoped `cluster.*.ext_authz.*` triple not exercised). ~70 LoC. Per SPEC §7. |
| `test/fixtures/0020-http-ext-authz-http/README.md` | NEW | Fixture overview + 7-scenario list + reference-config citations + extauthzhttp in-process server lifecycle notes + per-route 5th-canonical-REUSE discipline note (NO ADR-0125 amendment; ADR-0163) + SHARED-stats discipline note + counter-delta assertion discipline + divergence-window note (`allowed_client_headers_on_success` DEFERRED; `response_code_details` not emitted; dynamic-metadata family silent-ignored; cluster-scoped triple DEFERRED). ~110 LoC. Per SPEC §7.2. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | **7 ADRs** (ADR-0156/0157/0159/0160/0161/0162/0163) — §Context drafts ALREADY landed at SPEC commit `308e9b6` per ADR-0044; §Decision + §Consequences bodies authored at impl-time per the Lands-in-task table: ADR-0156 + ADR-0157 at Task 2; ADR-0159 at Task 3; ADR-0160 at Task 4; ADR-0161 at Task 5; ADR-0162 at Task 6; ADR-0163 at Task 7. **NO ADR-0125 amendment paragraph** — ADR-0163's §Context draft already records the explicit no-amendment 5th-canonical-REUSE classification; verify NO `§(xiv)` is needed (the FIRST §9 row to REUSE an ADR-0125 canonical). ADR-0164 (ADR-0045 split-application) landed IN FULL at the SPEC commit — UNCHANGED by 18.1 IMPL. ADR-0044 escape-valve held in reserve for ~1 impl-time-unanticipated ADR (most-likely the async-resume-after-`OnDestroy` race guard → ADR-0165 at Task 9 if it needs an ADR-lift). ~+260 LoC across the 7 ADR §Decision + §Consequences bodies. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 — 6-patch bundle: §13.1 NEW `### envoy.filters.http.ext_authz` subsection inserted alphabetical-after `### envoy.filters.http.csrf` per ADR-0100 §2.2 (HTTP-mode-focused + "gRPC mode — see phase 18.2" forward-pointer) ~130 LoC; §13.2 stat-table 71 → 77 names (6 new counter rows; `disabled` noted STRUCTURALLY UNREACHABLE under MVP) ~35 LoC; §13.3 NEW Equivalence-Matrix row for fixture `0020-http-ext-authz-http` ~5 LoC; §13.4 NEW `### Phase 18.1 forward-pointer notes` subsection (the SPEC §8 ~11-item deferral list + the `response_code_details` joint divergence-window) ~40 LoC; §13.6 ADR-0125 §(v) 5th-canonical-REUSE cross-reference (ext_authz is the FIRST §9 row to REUSE the 5th canonical; NO new amendment paragraph) ~5 LoC; §13.7 the HTTP-outbound auth-check note — per ADR-0159 disposition (b), a thin cross-reference under the phase-17 `## JWKS framework primitive` umbrella ("see also: ext_authz HTTP-mode thin outbound client") — the lighter-touch shape ~5 LoC. §13.5 `## HTTPFilterCallbacks` — NO extensions (18.1 reuses the phase-09 async-resume + ADR-0128 body-buffering + ADR-0085 `SendLocalReply` primitives as-is). Total ~+220 LoC. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `18.1` status `in-progress → done` flip + summary sharpening (post-impl counts: PLAN-confirmed 15-task + ~1053–1513 LoC production estimate + final 7-ADR roster anchored). Rows `18` + `18.2` UNCHANGED at the 18.1 phase-done commit (parent stays `in-progress`; `18.2` stays `planned`) per parent SPEC §8. ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state: `active-phase: 18.2-ext-authz-grpc`; `lifecycle-state: phase 18.1 done; phase 18.2 SPEC pending` (the next session is the phase-18.2 SPEC session — `superpowers:writing-plans` is NOT next; 18.2 needs its full `SPEC.md` authored first, so `next-skill` is the SPEC-authoring skill per BOOTSTRAP §5, NOT the PLAN skill); `next-free ADR`: `ADR-0165` (or `ADR-0166` if the ADR-0044 escape-valve fired at Task 9). The exact `next-skill` string is settled at Task 14 Step 8 against the BOOTSTRAP §5 lifecycle-state machine. |
| `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror the phase-09..17 PROGRESS.md structure. ~600 LoC across 15 task entries. |
| `docs/envoy-go/phases/18.1-ext-authz-http/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~240 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's seven deferred decisions before implementation; this PLAN settles all seven plus five that emerged at PLAN-drafting time (items 8–12 below). The twelve resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each task can act without re-deriving them:

1. **D1 — `test/helpers/extauthzhttp/` directory name + helper shape LOCKED per SPEC §12.1.** The in-process HTTP auth server lands at the top-level shared-helper location `test/helpers/extauthzhttp/` (NOT per-fixture) — mirrors the phase-17 `test/helpers/jwksbackend/` precedent, anticipating that the 18.2 gRPC fixture will want a sibling `test/helpers/extauthzgrpc/`. Package name `extauthzhttp`. Helper shape: a `Server` type + `New(ctx, addr, script) (*Server, error)` + `Stop()` + `Addr()`; `Script` supports fixed-response, per-path/method map, and body-inspecting predicate modes. *Anchored: SPEC §12.1 + §7.2.*

2. **D2 — `httpAuthClient` retry discipline LOCKED at ZERO retry per SPEC §12.2.** The thin ext_authz-local HTTP client does NO retry — single-attempt-then-error. Rationale: `HttpService` has no retry-policy field in the proto (unlike the JWKS fetcher's `RetryPolicy`); a connect failure / timeout maps straight to the **error** disposition per §5.P10. The 18.1 IMPL at Task 3 confirms against the §5.P10 error-classification boundary. Impl-time alternative (a fixed single-retry) NOT selected — MVP single-attempt. *Anchored: SPEC §12.2 + parent SPEC §5.P10.*

3. **D3 — `extauthz_test.go` single-file LOCKED per SPEC §12.3.** All 9 unit-test groups stay in one `extauthz_test.go` for 18.1 (mirrors `rbac/rbac_test.go`). Impl-time MAY split `check_test.go` if the file exceeds ~1800 LoC — an IMPL-cohesion call recorded at Task 3/Task 4 if it triggers. *Anchored: SPEC §12.3.*

4. **D4 — Async-resume-after-`OnDestroy` race guard LOCKED at the phase-09-fault pattern + an explicit `mu`/`done` guard + a cancellable `context.Context` per SPEC §12.4.** The outbound check runs in a plain goroutine (phase-09 fault precedent: `StopIteration` returned synchronously; the goroutine performs the cancellable call; `cb.ContinueDecoding()` or the deny `SendLocalReply` path on completion). The per-filter `mu sync.Mutex` + `done bool` guard: `OnDestroy` sets `done = true` under `mu` and calls `callCancel()`; the resume goroutine acquires `mu`, checks `done`, and aborts the callback touch if the stream is gone. The per-request `context.Context` (cancelled at `OnDestroy`) makes the in-flight `client.Do` return promptly. **This is the most-likely ADR-0044 escape-valve surface** (parent SPEC §7 + 18.1 SPEC §10) — if the `mu`/`done` guard proves insufficient or the HCM-dispatch interaction needs a framework primitive, it lands as **ADR-0165** at Task 9. Lands at Task 9. *Anchored: SPEC §12.4 + §6.3 + parent SPEC §7.*

5. **D5 — `safe_regex` RegexMatcher engine subset LOCKED at the phase-09/12 RE2 subset per SPEC §12.5.** The `allowed_headers`/`disallowed_headers` (and the response-side matcher fields) `safe_regex` arm reuses the phase-09/12 RegexMatcher-subset discipline — the `google_re2` engine arm is honored (Go `regexp`, RE2-compatible); other `RegexMatcher` engine arms PARSE-REJECT envoy-go-strict. The 18.1 IMPL at Task 4 confirms the exact subset against reference Envoy v1.37.2. *Anchored: SPEC §12.5 + parent SPEC §5.P8.*

6. **D6 — Deprecated `AuthorizationRequest.allowed_headers` disposition LOCKED at honored-if-present per SPEC §12.6.** When the deprecated `AuthorizationRequest.allowed_headers` (#1) IS present in a config, envoy-go honors it proto-faithful for backward-compat (mirrors the phase-17 amendment-4 "deprecated-but-honored" disposition). The top-level `ExtAuthz.allowed_headers` (#17) is the primary path. The 18.1 IMPL at Task 4 confirms against v1.37.2 whether it still honors the deprecated field or PARSE-IGNOREs it; if v1.37.2 PARSE-IGNOREs, the IMPL flips to silent-ignore + records the flip in PROGRESS.md + ADR-0160 §Decision. *Anchored: SPEC §12.6 + parent SPEC §6 amendment 2.*

7. **D7 — `validate_mutations` validation rule set LOCKED at the phase-10 header_mutation protected-header discipline per SPEC §12.7.** When `validate_mutations: true`, the auth service's allow-path upstream-injection headers + the deny-path client headers are validated: `:`-prefixed pseudo-headers REJECTED; invalid header-name characters REJECTED; invalid header-value characters REJECTED. A rejection drives the **invalid** disposition + the `invalid` counter (treated as the error posture per SPEC §6.3). The 18.1 IMPL at Task 5 pins the exact rule set against v1.37.2 `validate_mutations` behavior. *Anchored: SPEC §12.7 + the phase-10 header_mutation precedent.*

8. **D8 — RATIFIED-PENDING-IMPL-TIME pin closures assigned to concrete tasks (NEW; surfaces at PLAN-time per SPEC §11).** The three 18.1 RATIFIED-PENDING-IMPL-TIME pins close as follows: **§18.P6** (the 6-counter stat surface) + **§18.P7** (the Prometheus tag-extractor SN2-reuse) close at **Task 8** via an empirical scrape of reference Envoy v1.37.2's `/stats/prometheus` output for fixture 0020's listener config (the canonical RATIFIED-PENDING closure step per phase-16 §10 lesson (c) + phase-17 §11.P7 precedent); if divergent, amend ADR-0163 §Decision in-place at Task 8. The **§18.P11 deny-path header-ordering byte-shape** closes at **Task 13** via the fixture-harness differential diff (the auth-service-supplied decision headers first, framework housekeeping `content-length`/`date`/`server: envoy` after). *Anchored: SPEC §11 + parent SPEC §5.P6/P7/P11.*

9. **D9 — Counter-delta byte-equivalence assertion convention carried forward per planner-time decision (NEW; surfaces at PLAN-time).** Fixture 0020's driver scrapes `/stats/prometheus` before + after each scenario; asserts byte-equivalence against reference Envoy's expected delta in `expectations.yaml`. The 5 reachable counters (`ok`/`denied`/`error`/`failure_mode_allowed`/`invalid`) are asserted; the `disabled` counter is NOT asserted (STRUCTURALLY UNREACHABLE under MVP per parent SPEC §6 amendment 7 — it publishes 0 always). The per-route `disabled` scenario (scenario 6) asserts NO `ext_authz` counter increments at all. *Anchored: SPEC §7 + parent SPEC §6 amendment 7 + the phase-16/17 ADR-0145 precedent.*

10. **D10 — BEHAVIOR_CONTRACT §13.1 insertion at alphabetical-after-`csrf` position per SPEC §13.1 + ADR-0100 §2.2 (NEW; surfaces at PLAN-time).** The `### envoy.filters.http.ext_authz` subsection inserts alphabetically between `### envoy.filters.http.csrf` and the next subsection. The IMPL at Task 14 verifies the current BEHAVIOR_CONTRACT.md subsection ordering and, if it is landing-chronological rather than alphabetical, falls back to the observed convention + records the fallback in PROGRESS.md. *Anchored: SPEC §13.1 + ADR-0100 §2.2.*

11. **D11 — ADR-0044 escape-valve disposition: NO pre-reserved task slot (NEW; surfaces at PLAN-time).** Per the phase-13/14/15/16/17 precedent (the impl-time-unanticipated ADR landed at a follow-up task or folded into a main task), this PLAN does NOT pre-reserve an explicit task slot. The most-likely surface is the async-resume-after-`OnDestroy` race guard (D4) → if it needs an ADR-lift it lands as **ADR-0165** at Task 9 or as a follow-up commit between Task 13 and Task 14. *Anchored: SPEC §10 + parent SPEC §7.*

12. **D12 — Fixture 0020 is plaintext-only; NO PKI, NO TLS (NEW; surfaces at PLAN-time).** Unlike the phase-16 rbac mTLS fixture or the phase-17 jwt_authn RSA/ECDSA PKI fixture, fixture 0020 wires a plain HTTP/1.1 listener + a plain-HTTP auth server. No `pki/gen.go`. The TLS-to-auth-service plumbing is an 18.2 concern (parent SPEC §5.P13 RATIFIED-PENDING-IMPL-TIME). *Anchored: SPEC §7.2.*

---

## ADRs introduced by this plan

The seven 18.1-landing ADRs anticipated by SPEC §10 (ADR-0156/0157/0159/0160/0161/0162/0163). **§Context drafts ALREADY landed at SPEC commit `308e9b6`** per ADR-0044 ADR-on-impl convention. **§Decision + §Consequences bodies authored at IMPL-time** per the Lands-in-task table below. **ADR-0164** (the ADR-0045 split-application ADR) landed IN FULL at the SPEC commit — UNCHANGED by 18.1 IMPL. ADR-0158 (the `internal/grpcclient/` primitive) is an 18.2 ADR — NOT touched by 18.1.

| ADR | Subject (18.1 portion) | Lands-in-task |
|---|---|---|
| **ADR-0156** | `internal/filter/http/extauthz/` package shape — single-token directory (underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/`) + DECODER-only `HTTPFilter` (`Encoder: nil`; 5th §9 row pure decode-side) + 6-base-counter `filterStats` (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`; `disabled` STRUCTURALLY UNREACHABLE under MVP; unconditional allocation at `New()` time) + boot-registration alphabetical between `envoygotest` and `fault` + the deny-path `SendLocalReply` mechanism (status/body/headers from the auth response or `status_on_error`; `content-length` synthesized per ADR-0085) | Task 2 |
| **ADR-0157** | `compiledConfig` shape + the `services`-oneof dual-mode dispatch (a `checkFn` closure; `grpc_service` arm PARSE-REJECTs in 18.1, §Decision amended at 18.2 IMPL) + consumed-vs-deferred field discipline + the error-posture fields (`failure_mode_allow` / `failure_mode_allow_header_add` / `status_on_error` / `validate_mutations`) + `transport_api_version` V3-only PARSE-REJECT + the empty-`services`-oneof factory rejection + the §5.P10 error-classification boundary in `check.go` | Task 2 |
| **ADR-0159** | HTTP-outbound auth-check framework primitive — the thin ext_authz-local client (disposition (b) per SPEC §3.1); `httpAuthClient` wrapping `*http.Client` + the configured `HttpService.server_uri.timeout` + `path_prefix`; composes-against (does NOT reuse) the phase-17 `internal/jwks/Fetcher` outbound-HTTP structure; the (a)-vs-(b) record + the oauth2-triggers-`internal/httpclient/`-generalization forward-pointer | Task 3 |
| **ADR-0160** | `AuthorizationRequest` builder (HTTP-mode portion) — `headers_to_add` + `path_prefix` prepend + the top-level `ExtAuthz.allowed_headers`/`disallowed_headers` request-side filtering (`ListStringMatcher` → exact/prefix/suffix/contains/`ignore_case`/`safe_regex`; `custom` PARSE-REJECT) + the deprecated-`AuthorizationRequest.allowed_headers` honored-if-present disposition | Task 4 |
| **ADR-0161** | Bidirectional header-mutation discipline (HTTP-mode portion) — `AuthorizationResponse.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` compilation + allow-path upstream injection (set vs append) + deny-path downstream `allowed_client_headers`-filtered emission + `validate_mutations` gating → `invalid` counter + the deny-path header-set construction (`text/plain` fallback, decision-headers-first ordering) + the `allowed_client_headers_on_success` deferral + the stash-for-HCM revisit note | Task 5 |
| **ADR-0162** | Request-body inclusion — `with_request_body{max_request_bytes, allow_partial_message, pack_as_bytes}` + the phase-13 ADR-0128 decode-side body-buffering reuse + the `allow_partial_message:false` over-limit → `SendLocalReply(413, "Payload Too Large", {connection: close})` edge case (auth NOT called, NO counters) + the `DecodeHeaders`-StopIteration / `DecodeData`-resume interaction | Task 6 |
| **ADR-0163** | Per-route 5th-canonical REUSE classification (explicit no-new-canonical decision; **NO ADR-0125 amendment paragraph** — the FIRST §9 row to REUSE an ADR-0125 canonical) + SHARED-stats discipline + the `CheckSettings` narrower-override surface + the 6-counter stat surface (`http.<HCM_stat_prefix>.ext_authz.*`; HCM-rooted SN2-reuse; RATIFIED-PENDING-IMPL-TIME §18.P6 + §18.P7 closed at Task 8) + the PGV wrinkles (`disabled` `const: true`; `override` oneof PGV-required) | Task 7 |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (in the slot of the existing §Context-draft for that ADR), sets `Status: Accepted` + `Date: <impl-date>` + `Lands-in: Task N`, includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning 1 match.

**NO in-place ADR edits required by phase 18.1.** ADR-0125 gains NO amendment paragraph (ADR-0163's §Context draft already records the explicit no-amendment 5th-canonical-REUSE classification — verify at Task 7 cold-start that NO `§(xiv)` is needed). ADR-0157's §Decision is amended at 18.2 IMPL (NOT 18.1) to activate the `grpc_service` arm. Cross-references to ADR-0072 (extension-registry registration) + ADR-0085 (`SendLocalReply`) + ADR-0100 §2.2 (boot-registration ordering) + ADR-0114 (single-token directory) + ADR-0117 (per-route `sync.Map` lazy-cache) + ADR-0125 §(v) (5th canonical) + ADR-0128 (decode-side body-buffering) + ADR-0150 (`internal/jwks/Fetcher` — composed-against) land in the relevant ADR §Consequences sections at their anchor tasks. No in-place edits.

**ADR-0044 escape-valve** held in reserve for ~1 impl-time-unanticipated ADR. Most-likely surface (per SPEC §10 + parent SPEC §7): the async-resume-after-`OnDestroy` race guard (planner-time decision D4) — if the `mu`/`done` + cancellable-context guard proves insufficient or the HCM-dispatch interaction needs a framework primitive, it lands as **ADR-0165** at Task 9 or as a follow-up commit between Task 13 and Task 14. NO pre-reserved task slot (planner-time decision D11).

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-18.1-ext-authz-http-impl \
                 -b phase-18.1-ext-authz-http-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-18.1-ext-authz-http-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-18.1-ext-authz-http-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -6` shows the PLAN.md squash commit + its SHA-fill follow-up at the head, with the SPEC.md squash commit `308e9b6` + its SHA-fill follow-up `312beec` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `164` (ADR-0164 — the highest §Context-draft / full ADR anchored at the SPEC commit). If higher, another phase landed concurrently — re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-015[6-9]|^## ADR-016[0-4]' docs/envoy-go/DECISIONS.md` returns `9` (ADR-0156..ADR-0164). `grep -cE '^Lands-in: Task [0-9]+ of phase-18.1' docs/envoy-go/DECISIONS.md` returns `≥6` (the 18.1-landing ADRs' §Context-draft Lands-in fields).
6. **NO ADR-0125 §(xiv) amendment.** `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0 matches — phase 18 lands NO ADR-0125 amendment (ADR-0163 records the explicit no-amendment decision). If `(xiv)` returns ≥1 match, investigate before proceeding.
7. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md` returns `308e9b6` (or descendant). If different, re-read SPEC + re-verify the parent SPEC §5 empirical pins.
8. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
9. **Pristine tree.** `git status --porcelain` returns empty.
10. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
11. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9])'` returns every fixture 0000–0019 PASS — the 20 pre-existing fixtures are the regression baseline.
12. **Pre-existing fuzzers run clean at 30s.** The 21 fuzzers from phases 02–17 run clean. Phase 18.1 adds the 22nd (`FuzzExtAuthzConfigParse` per Task 10).
13. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
14. **`envoy.extensions.filters.http.ext_authz.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3 ExtAuthz | head -5` returns the `ExtAuthz` proto type's exported fields without an `import path failed` error. If it fails, `go mod download`.
15. **Pre-existing `internal/filter/http/extauthz/` directory does NOT exist.** `test ! -d internal/filter/http/extauthz && echo "ok: extauthz absent"` returns success.
16. **Pre-existing `test/helpers/extauthzhttp/` directory does NOT exist.** `test ! -d test/helpers/extauthzhttp && echo "ok: extauthzhttp absent"` returns success.
17. **`cmd/envoy-go/main.go` registers exactly the expected filters at master tip.** `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `12` (`router` + 11 filters: `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `jwtauthn`, `localratelimit`, `rbac`). If 13+, another filter landed concurrently — re-verify the registration ordering before adding the `extauthz` line.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, the 7 ADRs (ADR-0156/0157/0159/0160/0161/0162/0163) have §Context drafts at the SPEC commit; each ADR's §Decision + §Consequences body is authored at its impl-anchor task. The PROGRESS preamble ANTICIPATES the 7 ADRs (each with its Lands-in-task anchor reproduced from this PLAN's per-ADR table) and records the 12 planner-time decisions.

**Precondition:** worktree exists at `phase-18.1-ext-authz-http-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions report green.
**Artifact:** `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` with: (a) Preamble summarizing the 17-precondition verification (verbatim command outputs captured); (b) the 7-ADR table from `## ADRs introduced by this plan` reproduced verbatim; (c) the 12 planner-time decisions reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 1: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: `internal/filter/http/extauthz/` package skeleton — `doc.go` + `extauthz.go` (TypeURL + types + `compiledConfig` + `filterStats` + `buildCompiledConfig` + `services`-oneof dispatch + `parsePerRoute` + `resolvePerRouteConfig` + `newFilterStats` + `New` factory + decode-method skeletons) + `extauthz_test.go` Groups 1 + 2 + 7 [ADR-0156, ADR-0157]

**Files:**
- Create: `internal/filter/http/extauthz/doc.go`
- Create: `internal/filter/http/extauthz/extauthz.go`
- Create: `internal/filter/http/extauthz/extauthz_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (fill in §Decision + §Consequences for ADR-0156 + ADR-0157)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 2 entry)

Establishes the extauthz package skeleton: the `compiledConfig`/`compiledPerRoute`/`compiledCheckSettings`/`checkDisposition`/`authRequest` shapes, the 6-counter `filterStats` struct, the `New` factory, `buildCompiledConfig` with the `services`-oneof dispatch (`http_service` calls a STUBBED `buildHTTPCheckFn` returning a sentinel error — the real impl lands at Task 3; `grpc_service` PARSE-REJECTs; empty `services` PARSE-REJECTs; non-V3 `transport_api_version` PARSE-REJECTs), `parsePerRoute` + `resolvePerRouteConfig` (the per-route scaffolding — full coverage at Task 7), `newFilterStats`, and the decode-method skeletons (`DecodeHeaders`/`DecodeData`/`DecodeTrailers`/`OnDestroy`/`SetDecoderCallbacks` — the real `DecodeHeaders` dispatch body lands at Task 9). Task 2 lands ADR-0156 (package shape + decoder-only `HTTPFilter` + 6-base-counter `filterStats` + unconditional allocation + deny-path `SendLocalReply` mechanism + boot-registration ordering) + ADR-0157 (`compiledConfig` + `services`-oneof dispatch + error-posture fields + V3-only PARSE-REJECT + empty-`services` factory rejection + the error-classification boundary spec). The `compileStringMatcherList` call in `buildCompiledConfig` (for `allowed_headers`/`disallowed_headers`) is STUBBED in Task 2 (returns a placeholder); the real impl lands at Task 4.

**Precondition:** Task 1 acceptance green.
**Artifact:** new package directory with `doc.go` + `extauthz.go` + `extauthz_test.go`; ADR-0156 + ADR-0157 in DECISIONS.md with `Lands-in: Task 2` + filled §Decision + §Consequences bodies.
**Acceptance:** `extauthz_test.go` Groups 1 + 2 + 7 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet ./internal/filter/http/extauthz/...` exit 0; `grep -nE '^## ADR-0156|^## ADR-0157' docs/envoy-go/DECISIONS.md` returns 2 matches; `grep -cE '^Lands-in: Task 2' docs/envoy-go/DECISIONS.md` returns 2.

- [ ] **Step 1: Write the Group 1 + Group 2 + Group 7 failing tests first** (per `superpowers:test-driven-development`). Implement the test cases enumerated in the File structure table for `extauthz_test.go` Group 1 (`ExtAuthz` parse — `http_service` valid, `grpc_service` PARSE-REJECT, empty `services` PARSE-REJECT, non-V3 PARSE-REJECT, `with_request_body` `max_request_bytes` 0 PARSE-REJECT, `server_uri` unset/empty PARSE-REJECT, `status_on_error` default 403, `stat_prefix` consumed), Group 2 (`compiledConfig` shape — 6-counter `filterStats` allocation, nil-`Stats` tolerance, field-finality), Group 7 (per-route — `parsePerRoute` PGV-mirror `disabled: false` PARSE-REJECT + empty-`override` PARSE-REJECT, 3-tier resolution, `check_settings` merge, `sync.Map` lazy-cache identity). Use `mustAny`, `freshFactoryCtx`, `freshFactoryCtxWithRegistry` helpers mirroring phase-16 `rbac_test.go` + phase-17 `jwtauthn_test.go`.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestNew_|TestBuildCompiled|TestParsePerRoute|TestResolvePerRoute' -v` — expect BUILD FAIL, package does not exist.

- [ ] **Step 3: Author `doc.go`** — package overview per the File structure table responsibility for `doc.go`.

- [ ] **Step 4: Author `extauthz.go`** — types + factory + helpers per the File structure table responsibility for `extauthz.go`. Pay specific attention to: (a) the mode-agnostic `compiledConfig` shape (field-final at 18.1; no transport-specific state); (b) the `services` oneof dispatch — `nil` → PARSE-REJECT `ext_authz: services oneof must be set`, `*ExtAuthz_GrpcService` → PARSE-REJECT `ext_authz: grpc_service mode not yet supported (lands in phase 18.2)`, `*ExtAuthz_HttpService` → STUBBED `buildHTTPCheckFn` sentinel error; (c) `transport_api_version` non-V3 → PARSE-REJECT per ADR-0008; (d) `with_request_body` → validate `max_request_bytes > 0`; (e) `status_on_error` default 403; (f) the 6-counter `newFilterStats` registered unconditionally (`disabled` registered but STRUCTURALLY UNREACHABLE), guarded `if ctx.Stats != nil` per ADR-0085 nil-tolerance; (g) `parsePerRoute` PGV-mirror — `override` oneof required, `disabled` arm `const: true` (PARSE-REJECT `disabled: false`); (h) `resolvePerRouteConfig` 3-tier TPFC resolution + `sync.Map` `LoadOrStore` keyed by proto pointer per ADR-0117 + ADR-0125 §(v); (i) the decode-method skeletons (`DecodeHeaders` returns `HeaderContinue` placeholder for now — the real dispatch body lands at Task 9; `SetDecoderCallbacks` stores `dcb`); (j) the `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` compile-time assertion.

- [ ] **Step 5: Run tests to verify they PASS** — `go test ./internal/filter/http/extauthz/ -run 'TestNew_|TestBuildCompiled|TestParsePerRoute|TestResolvePerRoute' -v` — expect Groups 1 + 2 + 7 PASS.

- [ ] **Step 6: Fill in ADR-0156 + ADR-0157 §Decision + §Consequences bodies** in DECISIONS.md — the §Context drafts already landed at the SPEC commit; the IMPL task fills the remaining sections. `Status: Accepted`, `Date: <impl-date>`, `Lands-in: Task 2`. Body content per the ADR table at `## ADRs introduced by this plan`.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 2: extauthz package skeleton + compiledConfig + filterStats + Groups 1+2+7 tests [ADR-0156, ADR-0157]"
grep -nE '^## ADR-0156|^## ADR-0157' docs/envoy-go/DECISIONS.md
# expect: 2 matches
```

---

## Task 3: `check.go` — the HTTP-outbound auth-check primitive (`httpAuthClient` + `buildHTTPCheckFn` + the POST construction + the HTTP-response → `checkDisposition` mapping + the §5.P10 error-classification boundary) + `extauthz_test.go` Group 4 [ADR-0159]

**Files:**
- Create: `internal/filter/http/extauthz/check.go`
- Modify: `internal/filter/http/extauthz/extauthz.go` (wire `buildCompiledConfig`'s `http_service` arm to the real `buildHTTPCheckFn`)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 4 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0159 §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 3 entry)

Lands the HTTP-outbound auth-check primitive per ADR-0159 disposition (b) — the thin ext_authz-local `httpAuthClient` (NOT a generalized `internal/httpclient/` package). `buildHTTPCheckFn` validates `HttpService.server_uri` (PGV-mirror — `server_uri` is NOT PGV-required; the factory rejects an empty one + an empty `uri`), constructs the `httpAuthClient` (`&http.Client{Timeout: hs.server_uri.timeout}`; ZERO retry per planner-time decision D2), and returns a `checkFn` closure. The closure builds the outbound POST (`path_prefix` prepended; the request-side-filtered headers — wired from the STUBBED `buildAuthRequest` at this task, real impl at Task 4; the body when `with_request_body` is set), calls `client.Do(req.WithContext(ctx))`, and maps the HTTP response → `checkDisposition` per the **§5.P10 error-classification boundary**: status `200` → **allow**; recognized deny status (`401`/`403`) → **deny**; connect failure / timeout / context-cancelled / unrecognized status → **error**. The `allowed_upstream_headers`/`allowed_client_headers` extraction is STUBBED in Task 3 (the disposition's header fields are populated minimally); the real extraction + `validate_mutations` gating lands at Task 5. ADR-0159 lands here (the primitive + the (a)-vs-(b) record + the oauth2-generalization forward-pointer).

**Precondition:** Task 2 acceptance green.
**Artifact:** `check.go` with `httpAuthClient` + `buildHTTPCheckFn` + the `checkFn` closure; ADR-0159 in DECISIONS.md with `Lands-in: Task 3`.
**Acceptance:** `extauthz_test.go` Group 4 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `grep -nE '^## ADR-0159' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write the Group 4 failing tests first** — HTTP-mode `checkFn` allow (200 + headers) / deny (401 + body, 403 + body) / error (connect failure via a closed port, timeout via a slow `httptest` server, unrecognized status `555`); `path_prefix` prepend; `headers_to_add` appended; deprecated `AuthorizationRequest.allowed_headers` honored-if-present. Use an `httptest.NewServer`-based scriptable auth server helper (or the `scriptableAuthServer` test helper).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestCheckFn|TestBuildHTTPCheckFn' -v` — expect FAIL.

- [ ] **Step 3: Author `check.go`** — `httpAuthClient` + `buildHTTPCheckFn` + the `checkFn` closure per the File structure table responsibility for `check.go`. Pay specific attention to: (a) the `server_uri` PGV-mirror rejection (unset / empty `uri` → PARSE-REJECT); (b) the `&http.Client{Timeout: hs.server_uri.timeout}` construction (ZERO retry per D2); (c) the §5.P10 error-classification boundary — the closure returns `(checkDisposition{class: dispError}, err)` on connect failure / timeout / `ctx.Err() != nil` / unrecognized status; (d) `path_prefix` prepended to the path; (e) the closure threads `req.WithContext(ctx)` so `OnDestroy`'s `callCancel()` aborts the in-flight call.

- [ ] **Step 4: Wire `buildCompiledConfig`'s `http_service` arm** to call the real `buildHTTPCheckFn` (replace the Task 2 sentinel-error stub).

- [ ] **Step 5: Run tests to verify they PASS** — `go test ./internal/filter/http/extauthz/ -run 'TestCheckFn|TestBuildHTTPCheckFn' -v` + re-run Groups 1+2+7.

- [ ] **Step 6: Fill in ADR-0159 §Decision + §Consequences** — `Status: Accepted`, `Date: <impl-date>`, `Lands-in: Task 3`. Record the (a)-vs-(b) disposition + the oauth2-triggers-generalization forward-pointer.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 3: check.go HTTP-outbound auth-check primitive + Group 4 tests [ADR-0159]"
```

---

## Task 4: `attributes.go` — the `AuthorizationRequest` builder + `compileStringMatcherList` + request-side header filtering + `extauthz_test.go` Group 3 [ADR-0160]

**Files:**
- Create: `internal/filter/http/extauthz/attributes.go`
- Modify: `internal/filter/http/extauthz/extauthz.go` (wire `buildCompiledConfig`'s `allowed_headers`/`disallowed_headers` to the real `compileStringMatcherList`)
- Modify: `internal/filter/http/extauthz/check.go` (wire the `checkFn` closure to the real `buildAuthRequest`)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 3 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0160 §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 4 entry)

Lands the HTTP-mode `AuthorizationRequest` builder per ADR-0160 (HTTP-mode portion). `compileStringMatcherList` compiles a `ListStringMatcher` into the internal `stringMatcherList` type — exact/prefix/suffix/contains + `ignore_case` honored proto-faithful via `internal/matcher`; `safe_regex` reuses the phase-09/12 RegexMatcher-subset discipline (planner-time decision D5 — `google_re2` arm honored; other engine arms PARSE-REJECT); `custom` PARSE-REJECTs. `buildAuthRequest` filters the client request headers through `cc.allowedHeaders` (allow-list; `nil` = all), removes any matching `cc.disallowedHeaders` (overrides `allowed_headers`), appends `authorization_request.headers_to_add` static headers (+ the deprecated `AuthorizationRequest.allowed_headers` honored-if-present per planner-time decision D6). At this task the IMPL confirms the D5 regex-subset + the D6 deprecated-field disposition against reference Envoy v1.37.2 (records any divergence in PROGRESS.md + ADR-0160 §Decision). `validateMutationHeaders` is authored here (per planner-time decision D7) but consumed at Task 5.

**Precondition:** Task 3 acceptance green.
**Artifact:** `attributes.go` with `compileStringMatcherList` + `buildAuthRequest` + `validateMutationHeaders`; ADR-0160 in DECISIONS.md with `Lands-in: Task 4`.
**Acceptance:** `extauthz_test.go` Group 3 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `grep -nE '^## ADR-0160' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write the Group 3 failing tests first** — `allowed_headers`/`disallowed_headers` matcher compilation: exact/prefix/suffix/contains/`ignore_case`/`safe_regex`; `custom` PARSE-REJECT; `disallowed_headers` overrides `allowed_headers`; `nil` `allowed_headers` = all pass; `headers_to_add` appended; deprecated `AuthorizationRequest.allowed_headers` honored-if-present.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestCompileStringMatcherList|TestBuildAuthRequest' -v`.

- [ ] **Step 3: Author `attributes.go`** — `compileStringMatcherList` + `stringMatcherList` + `buildAuthRequest` + `validateMutationHeaders` per the File structure table responsibility. Confirm the D5 `safe_regex` subset + the D6 deprecated-field disposition against reference Envoy v1.37.2; record findings in PROGRESS.md.

- [ ] **Step 4: Wire the real `compileStringMatcherList`** into `buildCompiledConfig` (replace the Task 2 stub) and the real `buildAuthRequest` into `check.go`'s `checkFn` closure (replace the Task 3 stub).

- [ ] **Step 5: Run tests to verify they PASS** — Group 3 + re-run Groups 1+2+4+7.

- [ ] **Step 6: Fill in ADR-0160 §Decision + §Consequences** — `Lands-in: Task 4`.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 4: attributes.go AuthorizationRequest builder + request-side header filtering + Group 3 tests [ADR-0160]"
```

---

## Task 5: Bidirectional header-mutation discipline — `AuthorizationResponse` matcher extraction + allow-path upstream injection + deny-path header-set construction + `validate_mutations` gating + `extauthz_test.go` Group 8 [ADR-0161]

**Files:**
- Modify: `internal/filter/http/extauthz/check.go` (the `checkFn` closure — real `AuthorizationResponse` matcher extraction into `checkDisposition`)
- Modify: `internal/filter/http/extauthz/attributes.go` (wire `validateMutationHeaders` into the disposition path)
- Modify: `internal/filter/http/extauthz/extauthz.go` (the disposition-application logic — allow-path upstream injection; deny-path header-set)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 8 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0161 §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 5 entry)

Lands the bidirectional header-mutation discipline per ADR-0161 (HTTP-mode portion). `buildHTTPCheckFn` now compiles `hs.authorization_response.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` → `*stringMatcherList` triple (replaces the Task 3 stub). The `checkFn` closure populates `checkDisposition`: on **allow** — `allowed_upstream_headers`-filtered auth-response headers → `upstreamSet` (overwrite), `allowed_upstream_headers_to_append`-filtered → `upstreamApp` (append); on **deny** — the auth response's verbatim body → `denyBody`, the `allowed_client_headers`-filtered headers → `denyHeaders` (with `content-type` `text/plain` fallback if the auth service supplied none in the allowed set; decision-headers-first ordering per SPEC §4). `validate_mutations: true` runs `validateMutationHeaders` over the extracted header sets; a rejection drives the **invalid** disposition + the `invalid` counter (treated as the error posture per SPEC §6.3). The disposition-application logic in `extauthz.go` applies `upstreamSet`/`upstreamApp` to the upstream request headers on the allow path (the deny-path `SendLocalReply` + error-path posture are exercised by the Task 9 `DecodeHeaders` dispatch tests). ADR-0161 §Consequences records the `allowed_client_headers_on_success` deferral (decode-side-only filter shape) + the stash-for-HCM revisit note.

**Precondition:** Task 4 acceptance green.
**Artifact:** `check.go` + `extauthz.go` + `attributes.go` updated; ADR-0161 in DECISIONS.md with `Lands-in: Task 5`.
**Acceptance:** `extauthz_test.go` Group 8 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `grep -nE '^## ADR-0161' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write the Group 8 failing tests first** — allow-path `upstreamSet`/`upstreamApp` extraction (overwrite vs append; `allowed_upstream_headers` filtering); deny-path `allowed_client_headers` filtering + `text/plain` fallback + decision-headers-first ordering; `validate_mutations` rejection of `:`-prefixed pseudo-headers / invalid name chars / invalid value chars → `invalid` disposition.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestHeaderMutation|TestValidateMutations|TestDenyHeaderSet' -v`.

- [ ] **Step 3: Implement the response-side extraction** in `check.go` (replace the Task 3 stub) + the disposition-application logic in `extauthz.go` + wire `validateMutationHeaders`. Confirm the D7 `validate_mutations` rule set against reference Envoy v1.37.2; record findings in PROGRESS.md.

- [ ] **Step 4: Run tests to verify they PASS** — Group 8 + re-run Groups 1–4 + 7.

- [ ] **Step 5: Fill in ADR-0161 §Decision + §Consequences** — `Lands-in: Task 5`. Record the `allowed_client_headers_on_success` deferral + the stash-for-HCM revisit note.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 5: bidirectional header-mutation discipline + validate_mutations gating + Group 8 tests [ADR-0161]"
```

---

## Task 6: `with_request_body` — the ADR-0128 decode-side body-buffering reuse + the over-limit 413 edge case + the `DecodeData`-resume interaction + `extauthz_test.go` Group 6 [ADR-0162]

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (`DecodeHeaders` body-buffering branch + `DecodeData` accumulation + the over-limit local-reply)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 6 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0162 §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 6 entry)

Lands the request-body inclusion per ADR-0162. `DecodeHeaders` computes the effective `withRequestBody` (per-route override OR listener-level); when set AND `!endStream`, sets `awaitingBody = true` and returns `HeaderStopIteration`. `DecodeData` accumulates the body via the phase-13 ADR-0128 decode-side body-buffering primitive (mirror the `buffer`/`bandwidthlimit` consumers). On body-complete, proceed to build the `authRequest` with the body included. On over-limit + `allow_partial_message: false`, emit `SendLocalReply(413, "Payload Too Large", {connection: close})` BEFORE the outbound check fires — the auth service is NEVER contacted, NO `ext_authz` counter increments (per parent SPEC §5.P5 + §6 amendment 6). `allow_partial_message: true` → the truncated `max_request_bytes`-byte prefix is included. `pack_as_bytes` is honored (the body bytes shape — for HTTP-mode this is the POST body verbatim). ADR-0162 anchors the buffering machinery + the over-limit edge case + the `DecodeHeaders`-StopIteration / `DecodeData`-resume interaction.

**Precondition:** Task 5 acceptance green.
**Artifact:** `extauthz.go` body-buffering path implemented; ADR-0162 in DECISIONS.md with `Lands-in: Task 6`.
**Acceptance:** `extauthz_test.go` Group 6 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `grep -nE '^## ADR-0162' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write the Group 6 failing tests first** — body materialization via ADR-0128 (body included in the `authRequest`); over-limit + `allow_partial_message:false` → `SendLocalReply(413, "Payload Too Large", {connection: close})` + NO counters + auth not called; `allow_partial_message:true` → truncated prefix included; `pack_as_bytes` honored; per-route `disable_request_body_buffering` overrides listener-level `with_request_body` to OFF.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestWithRequestBody|TestDecodeData' -v`.

- [ ] **Step 3: Implement the body-buffering path** in `extauthz.go` — the `DecodeHeaders` StopIteration branch + `DecodeData` accumulation per the ADR-0128 primitive + the over-limit local-reply. Reference the `internal/filter/http/buffer` + `internal/filter/http/bandwidthlimit` ADR-0128 consumers for the exact primitive shape.

- [ ] **Step 4: Run tests to verify they PASS** — Group 6 + re-run Groups 1–5 + 7 + 8.

- [ ] **Step 5: Fill in ADR-0162 §Decision + §Consequences** — `Lands-in: Task 6`.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 6: with_request_body ADR-0128 reuse + over-limit 413 edge case + Group 6 tests [ADR-0162]"
```

---

## Task 7: Per-route 5th-canonical REUSE — `parsePerRoute` PGV-mirror finalization + `resolvePerRouteConfig` 3-tier + `check_settings` merge + SHARED-stats + `extauthz_test.go` Group 7 finalization [ADR-0163]

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (`parsePerRoute` + `resolvePerRouteConfig` + `compiledCheckSettings` merge finalization)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (extend Group 7 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0163 §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 7 entry)

Finalizes the per-route discipline per ADR-0163 — the 5th-canonical REUSE classification (NO ADR-0125 amendment paragraph — the FIRST §9 row to REUSE an existing ADR-0125 canonical; ADR-0163's §Context draft already records the explicit no-amendment decision — Task 7 verifies NO `§(xiv)` is needed). The `ExtAuthzPerRoute.override` oneof has two arms: `disabled` (bool, PGV `const: true` — PARSE-REJECT `disabled: false`) and `check_settings` (`*CheckSettings` carrying `context_extensions` — parsed but NO-OP in HTTP-mode per SPEC §8 item 8 + the proto doc-note; `disable_request_body_buffering` — overrides listener-level `with_request_body` to OFF; `with_request_body` — a per-route body-buffering override, mutually exclusive with `disable_request_body_buffering`). `resolvePerRouteConfig` uses the existing 3-tier TPFC resolution (Route > VirtualHost > RouteConfiguration > listener-fallback) + the `sync.Map` `LoadOrStore` lazy-cache. **SHARED-stats** — the per-route override spawns no new policy-evaluation surface; it carries NO `*filterStats` (mirrors phase-12/13/14/17). ADR-0163 also anchors the 6-counter stat surface (the §18.P6 + §18.P7 RATIFIED-PENDING-IMPL-TIME closures are deferred to Task 8 — ADR-0163 §Decision notes the closure-at-Task-8 disposition).

**Precondition:** Task 6 acceptance green.
**Artifact:** `extauthz.go` per-route path finalized; ADR-0163 in DECISIONS.md with `Lands-in: Task 7`.
**Acceptance:** `extauthz_test.go` Group 7 (extended) tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `grep -nE '^## ADR-0163' docs/envoy-go/DECISIONS.md` returns 1 match; `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0 matches (NO ADR-0125 amendment).

- [ ] **Step 1: Extend the Group 7 failing tests** — `check_settings` merge (`disable_request_body_buffering` XOR `with_request_body`; mutual exclusivity PARSE-REJECT); `context_extensions` parsed but no-op in HTTP-mode; 3-tier resolution most-specific-wins; SHARED-stats (per-route resolution does NOT allocate a fresh `*filterStats`); `sync.Map` lazy-cache identity (concurrent `resolvePerRouteConfig` produces ONE `*compiledPerRoute` per proto pointer).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestParsePerRoute|TestResolvePerRoute|TestCheckSettings' -v`.

- [ ] **Step 3: Finalize `parsePerRoute` + `resolvePerRouteConfig` + the `compiledCheckSettings` merge** in `extauthz.go`.

- [ ] **Step 4: Run tests to verify they PASS** — Group 7 (extended) + re-run Groups 1–6 + 8.

- [ ] **Step 5: Fill in ADR-0163 §Decision + §Consequences** — `Lands-in: Task 7`. Record the explicit no-ADR-0125-amendment 5th-canonical-REUSE classification + the SHARED-stats discipline + the §18.P6/§18.P7 RATIFIED-PENDING-IMPL-TIME closure-at-Task-8 disposition. Verify `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 7: per-route 5th-canonical REUSE + SHARED-stats + Group 7 finalization [ADR-0163]"
```

---

## Task 8: Stat surface finalization — `newFilterStats` 6-counter registration + the §18.P6 + §18.P7 RATIFIED-PENDING-IMPL-TIME empirical-scrape closures + `extauthz_test.go` Group 2 stats sub-group

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (`newFilterStats` finalization — 6-counter registration under the SN2-reuse namespace)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (extend Group 2 with a stats-namespace integration sub-group)
- Modify: `docs/envoy-go/DECISIONS.md` (in-place amend ADR-0163 §Decision ONLY IF the empirical scrape REFUTES the SN2-reuse hypothesis)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 8 entry + the empirical-scrape evidence)

Finalizes the stat surface and closes the two Task-8 RATIFIED-PENDING-IMPL-TIME pins. `newFilterStats` registers the 6 counters (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`) unconditionally at `New()` time under the HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.ext_authz.<counter>` (`disabled` registers but is STRUCTURALLY UNREACHABLE under MVP — publishes 0 always). **§18.P6 + §18.P7 closure** (planner-time decision D8 + per phase-16 §10 lesson (c) + phase-17 §11.P7 precedent): run an empirical scrape of reference Envoy v1.37.2's `/stats/prometheus` output for a config equivalent to fixture 0020's listener — confirm (a) the 6 counter names match the SPEC's 6-counter table, (b) the Prometheus tag-extractor renders them as `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<stat_prefix>"}` (SN2-reuse — NO new SN-flattening rule). If the scrape DIVERGES, amend ADR-0163 §Decision in-place at this task (per the phase-13 ADR-0127-v2 in-place-amendment precedent) + record the divergence in PROGRESS.md.

**Precondition:** Task 7 acceptance green.
**Artifact:** `newFilterStats` finalized; the §18.P6 + §18.P7 empirical-scrape evidence in PROGRESS.md.
**Acceptance:** `extauthz_test.go` Group 2 stats sub-group tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; the empirical scrape confirms (or the documented amendment lands); the 6 counters render under `http.<HCM_stat_prefix>.ext_authz.*`.

- [ ] **Step 1: Write the Group 2 stats sub-group failing tests** — all 6 counters registered at `New()` time (unconditional, non-lazy); SN2-reuse (HCM-rooted `http.<HCM>.ext_authz.<counter>` path); nil-`Stats` tolerance; `disabled` registered but never incremented under MVP.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestFilterStats|TestNewFilterStats' -v`.

- [ ] **Step 3: Finalize `newFilterStats`** — 6-counter registration under the SN2-reuse namespace; reference `internal/stats/name.go` + the phase-16/17 `newFilterStats` precedent.

- [ ] **Step 4: Run the §18.P6 + §18.P7 empirical scrape** — exercise reference Envoy v1.37.2 (Docker, at the ENVOY_TARGET.md SHA) with a config equivalent to fixture 0020's listener; scrape `/stats/prometheus`; confirm the 6 counter names + the tag-extractor shape. Capture the verbatim scrape output into PROGRESS.md. If divergent, amend ADR-0163 §Decision in-place.

- [ ] **Step 5: Run tests to verify they PASS** — Group 2 stats sub-group + re-run all prior groups.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 8: stat surface finalization + §18.P6 + §18.P7 RATIFIED-PENDING empirical-scrape closures"
```

---

## Task 9: `DecodeHeaders` body — top-level dispatch + the async-resume outbound-call leg + `OnDestroy` cancellation + the disposition-application logic + `extauthz_test.go` Groups 5 + 9

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (`DecodeHeaders` dispatch body + the async-resume leg + `OnDestroy` cancellation + the `mu`/`done` guard)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Groups 5 + 9 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (ONLY IF the ADR-0044 escape-valve fires — author ADR-0165 for the async-resume-after-`OnDestroy` race guard)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 9 entry)

Lands the `DecodeHeaders` top-level dispatch body + the async-resume outbound-call leg per SPEC §6.3. `DecodeHeaders` resolves per-route → caches `*compiledPerRoute`; `perRoute.disabled` → `HeaderContinue` (NO auth call, NO counters); the body-buffering branch (Task 6) interposes when `withRequestBody` is set; otherwise builds the `authRequest` and fires the async outbound check — returns `HeaderStopIteration` synchronously; a goroutine runs `cc.checkFn(callCtx, authReq)`; on completion the resume path applies the disposition (allow → `upstreamSet`/`upstreamApp` injection + optional `cb.ClearRouteCache()` + `cb.ContinueDecoding()`; deny → `cb.SendLocalReply(...)`; error → the `failure_mode_allow` / `status_on_error` posture; invalid → `invalid` counter + error posture). The async-resume leg mirrors the phase-09 fault primitive exactly. `OnDestroy` calls `callCancel()` + sets `done = true` under `mu` (planner-time decision D4 — the async-resume-after-`OnDestroy` race guard: the resume goroutine acquires `mu`, checks `done`, and aborts the callback touch if the stream is gone). **If the `mu`/`done` + cancellable-context guard proves insufficient or the HCM-dispatch interaction needs a framework primitive, the ADR-0044 escape-valve fires → author ADR-0165 at this task** (planner-time decision D11).

**Precondition:** Task 8 acceptance green.
**Artifact:** `extauthz.go` `DecodeHeaders` dispatch + async-resume leg + `OnDestroy` cancellation implemented; ADR-0165 IF the escape-valve fired.
**Acceptance:** `extauthz_test.go` Groups 5 + 9 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0 (the `-race` run is load-bearing for the async-resume race guard); `go vet` exit 0.

- [ ] **Step 1: Write the Groups 5 + 9 failing tests first** — Group 5: per-route `disabled` short-circuit (NO counters); async-resume allow (upstream injection + `clear_route_cache` → `cb.ClearRouteCache()`); async-resume deny (`SendLocalReply` with the auth status/body/headers); async-resume error (`failure_mode_allow:false` → `status_on_error` + empty body; `failure_mode_allow:true` → `HeaderContinue` + `x-envoy-auth-failure-mode-allowed` if `failure_mode_allow_header_add` + both `error` AND `failure_mode_allowed` increment); invalid → `invalid` counter + error posture. Group 9: `OnDestroy` cancels the in-flight `context.Context`; resume-after-`OnDestroy` is guarded (no panic, no callback touch after `done`) — run these under `-race`.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test -race ./internal/filter/http/extauthz/ -run 'TestDecodeHeaders|TestAsyncResume|TestOnDestroy' -v`.

- [ ] **Step 3: Implement the `DecodeHeaders` dispatch body + the async-resume leg + `OnDestroy`** in `extauthz.go`. Reference `internal/filter/http/fault/fault.go` (the `StopIteration` + goroutine + `cb.ContinueDecoding()` async-resume primitive). Implement the `mu`/`done` guard + the cancellable `context.Context` per planner-time decision D4.

- [ ] **Step 4: Run tests to verify they PASS** — `go test -race ./internal/filter/http/extauthz/...` — Groups 5 + 9 + re-run all prior groups; the `-race` run MUST be clean.

- [ ] **Step 5: ADR-0044 escape-valve check** — if the race guard needed a framework primitive or a non-trivial synchronization lift, author ADR-0165 (`Status: Accepted`, `Date: <impl-date>`, `Lands-in: Task 9`) + record the escape-valve firing in PROGRESS.md. If the `mu`/`done` guard sufficed, record "ADR-0044 escape-valve NOT fired" in PROGRESS.md.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 9: DecodeHeaders dispatch + async-resume outbound-call leg + OnDestroy cancellation + Groups 5+9 tests"
```

---

## Task 10: `cmd/envoy-go/main.go` boot-registration + fixture infrastructure (`BackendKind` enum + runner switch-case) + `FuzzExtAuthzConfigParse` 22nd fuzzer + NEW `test/helpers/extauthzhttp/` test-helper

**Files:**
- Modify: `cmd/envoy-go/main.go` (register `extauthz.New` under `extauthz.TypeURL`)
- Create: `internal/filter/http/extauthz/fuzz_test.go`
- Create: `test/helpers/extauthzhttp/doc.go`
- Create: `test/helpers/extauthzhttp/extauthzhttp.go`
- Create: `test/helpers/extauthzhttp/extauthzhttp_test.go`
- Modify: `test/differential/fixture/fixture.go` (`HTTPExtAuthzHTTP BackendKind = 17`)
- Modify: `test/differential/runner_test.go` (blank import + switch-case)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 10 entry)

Wires the filter into the boot path + lands the fixture infrastructure. `cmd/envoy-go/main.go` gains the `httpReg.Register(extauthz.TypeURL, extauthz.New)` line inserted between `envoygotest` and `fault` per ADR-0100 §2.2 alphabetical-after-router ordering + the matching import. `fuzz_test.go` is the 22nd fuzzer (`FuzzExtAuthzConfigParse`) per SPEC §7.3. `test/helpers/extauthzhttp/` is the NEW shared in-process scriptable HTTP auth server (planner-time decision D1). `test/differential/fixture/fixture.go` gains the `HTTPExtAuthzHTTP` `BackendKind`; `runner_test.go` gains the blank import + the dispatch switch-case.

**Precondition:** Task 9 acceptance green.
**Artifact:** `main.go` registers extauthz; `fuzz_test.go` + `test/helpers/extauthzhttp/` created; fixture infrastructure wired.
**Acceptance:** `go build ./...` exit 0; `go vet ./...` exit 0; `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `13`; `go test -race -count=1 ./test/helpers/extauthzhttp/...` exit 0; `FuzzExtAuthzConfigParse` runs clean for 30s (`go test -run '^$' -fuzz 'FuzzExtAuthzConfigParse' -fuzztime 30s ./internal/filter/http/extauthz/`).

- [ ] **Step 1: Write the `test/helpers/extauthzhttp/extauthzhttp_test.go` failing tests first** — server starts on the configured addr; fixed-script returns status/body/headers; path/method-map dispatch; body-inspecting script; `Stop()` closes the listener; concurrent-client race safety.

- [ ] **Step 2: Author `test/helpers/extauthzhttp/{doc.go, extauthzhttp.go}`** — per the File structure table responsibility; mirror `test/helpers/jwksbackend/` structure.

- [ ] **Step 3: Run the test-helper tests to verify they PASS** — `go test -race ./test/helpers/extauthzhttp/...`.

- [ ] **Step 4: Author `fuzz_test.go`** — `FuzzExtAuthzConfigParse` per SPEC §7.3; corpus seeds per the File structure table.

- [ ] **Step 5: Register the filter in `cmd/envoy-go/main.go`** — insert the import + the `httpReg.Register(extauthz.TypeURL, extauthz.New)` line between `envoygotest` and `fault`.

- [ ] **Step 6: Wire the fixture infrastructure** — `HTTPExtAuthzHTTP BackendKind = 17` in `fixture.go`; the blank import + dispatch switch-case in `runner_test.go`.

- [ ] **Step 7: Verify** — `go build ./...` + `go vet ./...` + the 30s fuzz run + `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 13.

- [ ] **Step 8: Commit**

```bash
git add cmd/envoy-go/main.go internal/filter/http/extauthz/fuzz_test.go test/helpers/extauthzhttp/ test/differential/ docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 10: boot-registration + FuzzExtAuthzConfigParse 22nd fuzzer + test/helpers/extauthzhttp/ + fixture infrastructure"
```

---

## Task 11: Fixture `0020-http-ext-authz-http` — `inputs/driver.go` (7-scenario driver + extauthzhttp lifecycle + counter-delta scrape)

**Files:**
- Create: `test/fixtures/0020-http-ext-authz-http/inputs/driver.go`
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 11 entry)

Lands the 7-scenario differential driver per SPEC §7.1. Functions `runScenario1..runScenario7(ctx, baseURL, authBaseURL) error` covering: (1) HTTP allow — 200 + `allowed_upstream_headers` injected upstream → `ok=1`; (2) HTTP deny — 403 + body byte-exact + `allowed_client_headers`-filtered headers → `denied=1`; (3) error → `status_on_error` — auth server unreachable, `failure_mode_allow:false`, `status_on_error:503` → 503 + empty body → `error=1`; (4) `failure_mode_allow` — auth unreachable, `failure_mode_allow:true` + `failure_mode_allow_header_add:true` → 200 backend-echo + `x-envoy-auth-failure-mode-allowed` upstream → `error=1` + `failure_mode_allowed=1`; (5) `with_request_body` — auth inspects the POST body, allows → 200 backend-echo → `ok=1`; (6) per-route `disabled` — `ExtAuthzPerRoute{disabled: true}` → 200 backend-echo → NO `ext_authz` counter increments; (7) per-route `check_settings` — `ExtAuthzPerRoute{check_settings{disable_request_body_buffering: true}}` overriding listener-level `with_request_body` → 200 backend-echo, body NOT buffered → `ok=1` (SHARED stats). Per-scenario assertion: byte-exact body + status equivalence + `/stats/prometheus` counter-delta on the 5 reachable counters + backend-arrival header assertions. Includes the `setupAuthServer` lifecycle helper (scenarios 3+4 stop it before the request) + the `scrapeStats`/`assertCounterDelta` helpers.

**Precondition:** Task 10 acceptance green.
**Artifact:** `test/fixtures/0020-http-ext-authz-http/inputs/driver.go`.
**Acceptance:** `go build ./test/fixtures/0020-http-ext-authz-http/...` exit 0; `go vet` exit 0 (the driver compiles; the end-to-end differential run is Task 13).

- [ ] **Step 1: Author `inputs/driver.go`** — the 7-scenario driver + the extauthzhttp lifecycle helper + the counter-delta helpers, per the File structure table responsibility + the SPEC §7.1 per-request matrix.

- [ ] **Step 2: Verify it compiles** — `go build ./test/fixtures/0020-http-ext-authz-http/... && go vet ./test/fixtures/0020-http-ext-authz-http/...`.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0020-http-ext-authz-http/inputs/ docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 11: fixture 0020 driver.go — 7-scenario differential driver"
```

---

## Task 12: Fixture `0020-http-ext-authz-http` — `envoy.yaml` + `envoy-go.yaml` bootstraps

**Files:**
- Create: `test/fixtures/0020-http-ext-authz-http/envoy.yaml`
- Create: `test/fixtures/0020-http-ext-authz-http/envoy-go.yaml`
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 12 entry)

Lands the two bootstrap configs per SPEC §7.2. Both wire an HCM listener `l_test_a` (TCP plaintext; filter chain `ext_authz → router`) with listener-level `ExtAuthz` config (HTTP-mode: `http_service.server_uri` → cluster `c_authz`; `allowed_headers`/`disallowed_headers`; `with_request_body` for scenario 5; the error-posture fields for scenarios 3+4), routes `/` → `c_backend` + `/per-route-disabled` (TPFC `ExtAuthzPerRoute{disabled: true}`) + `/per-route-check-settings` (TPFC `ExtAuthzPerRoute{check_settings{disable_request_body_buffering: true}}`), cluster `c_backend` → echobackend, cluster `c_authz` → extauthzhttp. `envoy.yaml` uses STRICT_DNS; `envoy-go.yaml` uses STATIC. The IMPL settles the exact scenario-3+4 topology (a second listener / a `c_authz_down` cluster pointing at a closed port) — record the choice in PROGRESS.md.

**Precondition:** Task 11 acceptance green.
**Artifact:** `envoy.yaml` + `envoy-go.yaml`.
**Acceptance:** both YAML files parse (`go run ./cmd/envoy-go --config-validate test/fixtures/0020-http-ext-authz-http/envoy-go.yaml` or the established config-validation entry-point exits 0; the reference `envoy.yaml` validates against the v1.37.2 image).

- [ ] **Step 1: Author `envoy.yaml`** — the reference Envoy bootstrap per the File structure table responsibility.

- [ ] **Step 2: Author `envoy-go.yaml`** — the equivalent envoy-go bootstrap (STATIC clusters).

- [ ] **Step 3: Validate both configs** — envoy-go config-validation entry-point on `envoy-go.yaml`; the v1.37.2 image config-check on `envoy.yaml`.

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0020-http-ext-authz-http/envoy.yaml test/fixtures/0020-http-ext-authz-http/envoy-go.yaml docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 12: fixture 0020 envoy.yaml + envoy-go.yaml bootstraps"
```

---

## Task 13: Fixture `0020-http-ext-authz-http` — `expectations.yaml` + `README.md` + end-to-end differential pass (all 7 scenarios + all 21 fixtures) + the §18.P11 deny-path header-ordering RATIFIED-PENDING closure

**Files:**
- Create: `test/fixtures/0020-http-ext-authz-http/expectations.yaml`
- Create: `test/fixtures/0020-http-ext-authz-http/README.md`
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 13 entry + the §18.P11 closure evidence)

Lands `expectations.yaml` + `README.md` and runs the end-to-end differential to green. **§18.P11 closure** (planner-time decision D8): the deny-path header-ordering byte-shape (auth-supplied decision headers first, framework housekeeping `content-length`/`date`/`server: envoy` after) is RATIFIED-PENDING-IMPL-TIME — it closes here, at the fixture-harness differential diff, by confirming the envoy-go deny-path wire bytes match reference Envoy v1.37.2 byte-for-byte. If the ordering diverges, fix `check.go`'s deny-path header-set construction (Task 5's `denyHeaders` ordering) + record the closure evidence in PROGRESS.md.

**Precondition:** Task 12 acceptance green.
**Artifact:** `expectations.yaml` + `README.md`; the differential suite green.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0020'` PASS (all 7 scenarios); `go test -count=1 ./test/differential/` PASS (all 21 fixtures 0000–0020); the §18.P11 deny-path header-ordering confirmed byte-equivalent (evidence in PROGRESS.md).

- [ ] **Step 1: Author `expectations.yaml`** — per-scenario allow-list + counter-delta map per the File structure table responsibility.

- [ ] **Step 2: Author `README.md`** — fixture overview + 7-scenario list + divergence-window note.

- [ ] **Step 3: Run the fixture 0020 differential** — `go test -count=1 ./test/differential/ -run 'Test.*0020'`; iterate on the driver / configs / filter code until all 7 scenarios PASS.

- [ ] **Step 4: Close §18.P11** — confirm the deny-path header-ordering byte-shape against reference Envoy v1.37.2 via the fixture-harness diff; capture the verbatim deny-path wire bytes (both sides) into PROGRESS.md.

- [ ] **Step 5: Run the full differential suite** — `go test -count=1 ./test/differential/` — all 21 fixtures (0000–0020) PASS.

- [ ] **Step 6: Commit**

```bash
git add test/fixtures/0020-http-ext-authz-http/expectations.yaml test/fixtures/0020-http-ext-authz-http/README.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 13: fixture 0020 expectations + README + end-to-end differential pass + §18.P11 closure"
```

---

## Task 14: BEHAVIOR_CONTRACT.md 6-edit bundle + ROADMAP row 18.1 in-progress→done + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§13.1 + §13.2 + §13.3 + §13.4 + §13.6 + §13.7 — 6 patches per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (row 18.1 `in-progress → done`)
- Modify: `docs/envoy-go/STATE.md` (advance per BOOTSTRAP_PROMPT.md §5)
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 14 entry + the 6-gate report)

Lands the BEHAVIOR_CONTRACT.md 6-patch bundle per SPEC §13, flips ROADMAP row 18.1, advances STATE.md, and runs the 6 phase-done gates per BOOTSTRAP_PROMPT.md §7.5.

**Precondition:** Task 13 acceptance green.
**Artifact:** BEHAVIOR_CONTRACT.md + ROADMAP.md + STATE.md updated; the 6-gate report appended to PROGRESS.md.
**Acceptance:** All 6 phase-done gates green per SPEC §14.5:
- **Gate A** (build + vet + lint): `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0.
- **Gate B** (race tests): `go test -race -count=1 ./...` exit 0 repo-wide including the new `extauthz` + `extauthzhttp` packages.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin (no H2 wire-shape change).
- **Gate D** (fuzzers): 22 fuzzers green at 30s each.
- **Gate E** (differential): 21 differential fixtures (0000–0020) PASS.
- **Gate F** (BEHAVIOR_CONTRACT): the §13 6-patch bundle landed.

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md §13.1 patch** — NEW `### envoy.filters.http.ext_authz` subsection inserted alphabetical-after `### envoy.filters.http.csrf` per planner-time decision D10 (verify the current subsection ordering; if landing-chronological, fall back + record the fallback in PROGRESS.md).

- [ ] **Step 2: Apply §13.2 patch** — stat-table 71 → 77 names (6 new counter rows; `disabled` noted STRUCTURALLY UNREACHABLE under MVP).

- [ ] **Step 3: Apply §13.3 patch** — Equivalence-Matrix new row for fixture `0020-http-ext-authz-http`.

- [ ] **Step 4: Apply §13.4 patch** — NEW `### Phase 18.1 forward-pointer notes` subsection (the SPEC §8 ~11-item deferral list + the `response_code_details` joint divergence-window).

- [ ] **Step 5: Apply §13.6 patch** — ADR-0125 §(v) 5th-canonical-REUSE cross-reference (ext_authz is the FIRST §9 row to REUSE the 5th canonical; NO new amendment paragraph).

- [ ] **Step 6: Apply §13.7 patch** — the HTTP-outbound auth-check note (a thin cross-reference under the phase-17 `## JWKS framework primitive` umbrella per ADR-0159 disposition (b) — the lighter-touch shape).

- [ ] **Step 7: Flip ROADMAP row 18.1** to `in-progress → done` + sharpen the summary with post-impl counts (15-task + final 7-ADR roster + the ADR-0044 escape-valve disposition). Rows `18` + `18.2` UNCHANGED.

- [ ] **Step 8: Advance STATE.md** — `active-phase: 18.2-ext-authz-grpc`; `lifecycle-state: phase 18.1 done; phase 18.2 SPEC pending` (the next session is the 18.2 SPEC session per the parent SPEC §8 + the 05.1/05.2 lifecycle pattern); `next-skill:` the SPEC-authoring skill for 18.2; `last-commit: <Task 14 squash>`; `next-free ADR: ADR-0165` (or `ADR-0166` if the ADR-0044 escape-valve fired at Task 9); `last-updated: <impl-date>`.

- [ ] **Step 9: Run the 6-gate phase-done verification** — execute all 6 gate commands; capture verbatim outputs into the PROGRESS.md Task 14 entry; all green.

- [ ] **Step 10: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 14: BEHAVIOR_CONTRACT 6-edit bundle + ROADMAP row 18.1 done + STATE advance + 6-gate phase-done verification"
```

---

## Task 15: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/18.1-ext-authz-http/REVIEW.md`
- Modify: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (append Task 15 entry)

Lands the end-of-phase review document per `superpowers:requesting-code-review`. Covers: the 18.1 deliverables + the final ADR roster (7 anchored §Decision + §Consequences bodies — ADR-0156/0157/0159/0160/0161/0162/0163 — plus the ADR-0044 escape-valve disposition: whether ADR-0165 landed at Task 9); the SPEC §15 15-claim acceptance checklist verification; the 18.1-load-bearing §11 empirical-pin dispositions (the §18.P1/P2/P5/P6/P7/P8/P9/P10/P11/P12 reflections + the 3 RATIFIED-PENDING-IMPL-TIME closures: §18.P6 + §18.P7 at Task 8, §18.P11 at Task 13); the framework-delta impact (ONE new primitive — ADR-0159 the thin HTTP-outbound auth-check — + four REUSES: phase-09 async-resume, phase-13 ADR-0128 body-buffering, phase-17 ADR-0150 composed-against, ADR-0085 `SendLocalReply`); the divergence-window enumeration (`allowed_client_headers_on_success` DEFERRED; `response_code_details` not emitted; dynamic-metadata family silent-ignored; cluster-scoped `cluster.*.ext_authz.*` triple DEFERRED; `disabled` counter STRUCTURALLY UNREACHABLE under MVP); the parent-rollup note (the parent row `18` stays `in-progress` — it closes only when 18.2 is `done`).

**Precondition:** Task 14 acceptance green.
**Artifact:** new REVIEW.md file.
**Acceptance:** REVIEW.md committed; the 18.1 end-state captured.

- [ ] **Step 1: Author REVIEW.md** — structure per the `superpowers:requesting-code-review` skill output template + the phase-13..17 REVIEW.md precedent. ~240 LoC.

- [ ] **Step 2: Commit**

```bash
git add docs/envoy-go/phases/18.1-ext-authz-http/REVIEW.md docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md
git commit -m "phase 18.1 Task 15: REVIEW.md — end-of-phase review"
```

---

## End of phase 18.1 implementation plan
