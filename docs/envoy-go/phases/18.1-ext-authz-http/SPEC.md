# Phase 18.1 SPEC — `envoy.filters.http.ext_authz` (filter scaffold + HTTP service mode)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `18.1` added `in-progress` at this SPEC commit (parent row `18` flips `planned → in-progress`; row `18.2` added `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–17 precedent. This SPEC is the authoritative input to the 18.1 PLAN.

**Parent:** `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4, the **full §5 13-pin empirical-pin block** resolved IN-SESSION, the §6 empirical-finding amendment block, the §7 9-ADR anchor map, and the §3 ADR-0045 split rationale). This sub-phase SPEC details the 18.1 surface only; it REFERENCES the parent's §5/§6/§7 rather than repeating them.

**Predecessors:** `docs/envoy-go/phases/18-http-filter-ext-authz/BRAINSTORM.md` (the §10 empirical pins are resolved in the parent SPEC §5). NO off-master prebrainstorm-notes branch.

**Sub-phase scope (per parent SPEC §2):** 18.1 lands the foundational filter — the NEW `internal/filter/http/extauthz/` package, the `ExtAuthz` proto parsing, the dual-mode `compiledConfig` envelope (with the `grpc_service` arm PARSE-REJECTing — 18.2 activates it), the `http_service` arm + the HTTP-outbound auth-check, the request-side header filtering, the HTTP-mode `AuthorizationResponse` allow/deny header extraction, `with_request_body` via the phase-13 ADR-0128 reuse, the per-route 5th-canonical REUSE + SHARED-stats + the 6-counter filter stat surface, the `failure_mode_allow` / `status_on_error` error posture, the async-resume outbound-call leg, boot-registration, the DECODER-only filter shape + `filterStats` + the deny-path `SendLocalReply` mechanism. **18.2 (the gRPC service mode + the `internal/grpcclient/` primitive) is OUT OF SCOPE for 18.1.**

**ADR continuity:** Phase 17 closed at ADR-0155. Phase 18 anticipates ADR-0156..ADR-0164 (9 ADRs per parent SPEC §7). The 18.1-landing ADRs are **ADR-0156, ADR-0157 (§Decision; amended at 18.2), ADR-0159, ADR-0160 (HTTP-mode portion), ADR-0161 (HTTP-mode portion), ADR-0162, ADR-0163** — anchored with §Context drafts at the parent SPEC commit; §Decision + §Consequences bodies LAND at each ADR's Lands-in-Task per ADR-0044. **ADR-0164** (the ADR-0045 split-application ADR) landed IN FULL at the parent SPEC commit. ADR-0158 (the `internal/grpcclient/` primitive) lands in 18.2. Next-free ADR after phase 18 is ADR-0165.

**Authored:** 2026-05-14.

---

## 1. Purpose

Phase 18.1 lands `envoy.filters.http.ext_authz` in **HTTP service mode** — the canonical Envoy external-authorization filter delegating the allow/deny decision to an external HTTP service over an outbound POST — as the foundational half of the ELEVENTH §9 production HTTP filter. It establishes the entire `internal/filter/http/extauthz/` package, the dual-mode `compiledConfig` envelope, and the mode-agnostic disposition-application logic; the `grpc_service` arm PARSE-REJECTs in 18.1 and is activated by 18.2. The seven architectural primitives:

1. **New `internal/filter/http/extauthz/` package** owning the filter implementation. Package directory + Go-package identifier are both `extauthz` (single token underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/`). Files mirror the phase-16 + phase-17 multi-file split: `extauthz.go` (filter type + factory + decode methods + `filterStats` struct + `compiledConfig` + per-route helper), `check.go` (the check dispatcher — in 18.1 the HTTP-outbound auth-check POST path + the HTTP-response → `checkDisposition` mapping + the `failure_mode_allow` / `status_on_error` error-classification; the `grpc_service` dispatch arm is a PARSE-REJECT stub in 18.1, filled in 18.2), `attributes.go` (the HTTP-mode `AuthorizationRequest` builder + the request-side header filtering through the top-level `ExtAuthz.allowed_headers`/`disallowed_headers`; the gRPC-mode `AttributeContext` builder is added in 18.2), `extauthz_test.go` (unit tests; anticipated 1100–1800 LoC given the dual-mode envelope + HTTP-mode check + header-mutation + body-inclusion subsurface), `fuzz_test.go` (the 22nd fuzzer — `FuzzExtAuthzConfigParse`), `doc.go` (package overview + the 6-decision summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"`) + `New` (the `HTTPFilterFactory`) per the cors/fault/.../jwtauthn precedent. ADR-0156 codifies.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 13 entries after phase 17: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `jwtauthn.New`, `localratelimit.New`, `rbac.New` before `httpReg.Freeze()`) gains a fourteenth `httpReg.Register(extauthz.TypeURL, extauthz.New)` call before the freeze. Insertion alphabetical-after-router per ADR-0100 §2.2: `extauthz` inserts between `envoygotest` and `fault`. Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only.

3. **`ExtAuthz` proto parsing — the dual-mode `compiledConfig` envelope.** Per parent SPEC §4.1 + §4.2 + ADR-0157: the `services` oneof is resolved at config-load time → produces a `checkFn` closure. In 18.1 the `http_service` arm builds the HTTP-outbound auth-check `checkFn`; the **`grpc_service` arm PARSE-REJECTs** with an envoy-go-strict error (`ext_authz: grpc_service mode not yet supported (lands in phase 18.2)`). An empty `services` oneof PARSE-REJECTs (the oneof is NOT PGV-required per parent §5.P1 — the factory rejects it). The 18.1 MVP consumes (per parent §5.P1): `http_service` (#3 → `HttpService`), `transport_api_version` (#12, V3-only PARSE-REJECT), `with_request_body` (#5 → `BufferSettings`), `failure_mode_allow` (#2), `failure_mode_allow_header_add` (#19), `clear_route_cache` (#6), `status_on_error` (#7 → `*type.v3.HttpStatus`), `validate_mutations` (#24), `allowed_headers` (#17 → `ListStringMatcher`), `disallowed_headers` (#25 → `ListStringMatcher`), `stat_prefix` (#13), and `HttpService.{server_uri, path_prefix, authorization_request, authorization_response}`. DEFERRED per parent §4.4 + §8 below. ADR-0157 codifies the envelope; ADR-0156 codifies the package + `filterStats` shape.

4. **HTTP-outbound auth-check (ADR-0159).** The `http_service` arm POSTs the filtered request to `HttpService.server_uri.uri` (+ `path_prefix` prepended to the path) with the request-side-filtered client headers (filtered through the top-level `ExtAuthz.allowed_headers` allow-list minus `disallowed_headers`, then `AuthorizationRequest.headers_to_add` static additions appended) + the request body when `with_request_body` is set. It parses the HTTP response status/body/headers into a `checkDisposition`: status `200` → **allow** (extract `AuthorizationResponse.allowed_upstream_headers` / `allowed_upstream_headers_to_append` into the upstream request); a recognized deny status (`401`/`403` per parent §5.P10) → **deny** (status + body + `allowed_client_headers`-filtered headers emitted via `SendLocalReply`); any other condition (connect failure, timeout, unrecognized status) → **error** (the `failure_mode_allow` / `status_on_error` posture applies per parent §5.P10). The outbound call is **async** — the decode dispatch goroutine parks on the per-stream resume channel (mirroring the phase-09 fault async-resume primitive: `StopIteration` + a goroutine performing the call + `cb.ContinueDecoding()` on completion) so the HCM dispatch goroutine is not blocked on network I/O. `OnDestroy` cancels the in-flight call's `context.Context` (the FIRST §9 row with a per-request cancellable outbound call). ADR-0159 anchors the HTTP-outbound auth-check + records the SPEC author's (a)-vs-(b) disposition (see §3.1).

5. **Filter-callback shape: `StreamDecoderFilter` ONLY** (`Encoder: nil`). ext_authz is a decode-side request-gate — evaluated at `DecodeHeaders` time, with the disposition computed BEFORE the request body is forwarded upstream. 5th §9 row to ship pure decode-side (after phase-12 csrf, phase-13 buffer, phase-16 rbac, phase-17 jwt_authn). Static blank-identifier compile-time check `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)`. The decode-side surface: `DecodeHeaders(headers, endStream)` resolves per-route → caches `*compiledPerRoute` on filter state → if the per-route arm is `disabled` returns `HeaderContinue` immediately (no auth call, no counter increments — per parent §5.P6 + §6 amendment 7); else if `with_request_body` is set returns `HeaderStopIteration` and waits for the body via the ADR-0128 decode-side body-buffering primitive; otherwise builds the HTTP `AuthorizationRequest`, fires the async outbound check, and on resume applies the disposition. `DecodeData` participates in the ADR-0128 body-buffering interaction when `with_request_body` is set (otherwise pass-through). `DecodeTrailers` pass-through. ADR-0156 codifies.

6. **HTTP-mode `AuthorizationRequest` builder + request-side header filtering (ADR-0160 HTTP-mode portion).** The builder constructs the outbound POST: filters client headers through the top-level `ExtAuthz.allowed_headers` (a `ListStringMatcher` — exact/prefix/suffix/contains + `ignore_case` honored proto-faithful; `safe_regex` reuses the phase-09/12 RegexMatcher-subset discipline; `custom` PARSE-REJECTs per parent §6 amendment 2), then removes any header matching `ExtAuthz.disallowed_headers` (which overrides `allowed_headers` per the proto doc), then appends `AuthorizationRequest.headers_to_add` static headers; prepends `path_prefix` to the path; includes the body when `with_request_body` is set. The deprecated `AuthorizationRequest.allowed_headers` (#1) is honored proto-faithful for backward-compat IF present (per parent §6 amendment 2 — the 18.1 §11-reference resolves whether v1.37.2 still honors it). ADR-0160 anchors the builder; the gRPC-mode `AttributeContext` builder is 18.2's portion.

7. **HTTP-mode header-mutation discipline (ADR-0161 HTTP-mode portion).** On **allow** (HTTP 200 from the auth service): `AuthorizationResponse.allowed_upstream_headers` + `allowed_upstream_headers_to_append` filter which of the auth service's HTTP response headers are injected into the *upstream* request (allowed_upstream_headers = overwrite/set; allowed_upstream_headers_to_append = append). On **deny** (recognized deny status): the auth service's HTTP response status + body + the `allowed_client_headers`-filtered headers are emitted *downstream* via `SendLocalReply` (per parent §5.P11 — body verbatim, `content-length` synthesized, `text/plain` fallback content-type). `validate_mutations` gates header-name/value safety validation (mirrors the phase-10 header_mutation protected-header discipline — protected pseudo-headers rejected). On **allow with `failure_mode_allow_header_add`** AND an error that `failure_mode_allow` let through: an `x-envoy-auth-failure-mode-allowed: true` header is added to the upstream request. DEFERRED: `allowed_client_headers_on_success` (per parent §5.P9 + §6 amendment 9 — decode-side-only filter shape); `query_parameters_to_set/remove` (path-query subsystem, ADR-0112); `dynamic_metadata_from_headers` (dynamic-metadata family). ADR-0161 anchors the HTTP-mode bidirectional emit-order + per-direction filtering rules.

Plus the **request-body inclusion (ADR-0162)**, the **per-route 5th-canonical REUSE + 6-counter stat surface (ADR-0163)**, and the **HTTP-outbound auth-check framework primitive (ADR-0159)** — detailed in §3, §5, §6 below.

After phase 18.1, the project has the foundational ext_authz filter: a decode-side gate that ships request attributes to an external HTTP auth service, parks on an async-resume leg while the POST is in flight, converges on a `{allow, deny, error}` disposition, applies bidirectional header mutation on allow + emits the auth service's status/body/headers verbatim on deny, and honors the `failure_mode_allow` / `status_on_error` posture on error — observable-outcomes byte-equivalent to reference Envoy v1.37.2 HTTP-mode ext_authz on every axis except the documented divergence-windows. Phase 18.2 then activates the `grpc_service` arm + the `internal/grpcclient/` primitive against the same package surface.

### 1.1 Empirical-finding-driven scope (per parent SPEC §6)

The 12 §6 amendments in the parent SPEC are the empirical-finding-driven scope revisions for phase 18. The amendments load-bearing for 18.1: **amendment 1** (`services` oneof not PGV-required — the factory rejects empty), **amendment 2** (`allowed_headers`/`disallowed_headers` are top-level, both modes; `AuthorizationRequest.allowed_headers` deprecated-but-honored), **amendment 3** (5th-canonical-REUSE confirmed; PGV wrinkles), **amendment 6** (`with_request_body` over-limit → local 413 + `connection: close`, auth skipped), **amendment 7** (`disabled` counter STRUCTURALLY UNREACHABLE under MVP; fixture scenario 7 counter assertion corrected; no explicit `filter_enabled` settings needed), **amendment 8** (6-counter stat surface; 71 → 77 names; no `cx_*`; cluster-scoped triple deferred), **amendment 9** (`allowed_client_headers_on_success` DEFERRED), **amendment 10** (error-classification boundary), **amendment 11** (deny-path wire shape). Amendments 4, 5, 12 are gRPC-mode (18.2). This 18.1 SPEC's §4/§5/§6/§7 incorporate amendments 1/2/3/6/7/8/9/10/11 into the formal SPEC shape.

---

## 2. Non-purposes

Phase 18.1 is a single-sub-phase slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land HTTP-mode ext_authz under the existing 07.1 framework + the ONE new framework primitive anchored at ADR-0159 (which IS part of 18.1's deliverable).

- **2.1 gRPC service mode is OUT OF SCOPE.** The `grpc_service` arm PARSE-REJECTs in 18.1. The `internal/grpcclient/` primitive (ADR-0158), the gRPC-mode `AttributeContext` builder (ADR-0160 gRPC-mode portion), the `CheckResponse` → disposition mapping + `OkHttpResponse`/`DeniedHttpResponse` header mutation (ADR-0161 gRPC-mode portion), the `encode_raw_headers` discipline, and the gRPC auth-server test-helper all land in 18.2.
- **2.2 Deferred `ExtAuthz` fields** (per parent §4.4 + §8 below): the four `*metadata_context_namespaces` fields, `filter_enabled` / `filter_enabled_metadata` / `deny_at_disable`, `enable_dynamic_metadata_ingestion`, `filter_metadata`, `charge_cluster_response_stats`, `bootstrap_metadata_labels_key`, `emit_filter_state_stats`, `decoder_header_mutation_rules` — silent-ignore under the inline-deferral discipline.
- **2.3 `allowed_client_headers_on_success`** — DEFERRED per parent §5.P9 + §6 amendment 9 (decode-side-only filter shape has no encode leg).
- **2.4 `query_parameters_to_set` / `query_parameters_to_remove`** — DEFERRED (path-query rewriting subsystem, ADR-0112).
- **2.5 The cluster-scoped `cluster.<upstream>.ext_authz.*` stat triple** — DEFERRED per parent §6 amendment 8 (a NEW stat-namespace pattern; couples to charging-into-the-cluster-stat-tree).
- **2.6 gRPC `GoogleGrpc` arm + gRPC retry-policy customization** — N/A in 18.1 (gRPC mode is 18.2; the `GoogleGrpc` PARSE-REJECT is an 18.2 concern).
- **2.7 No filter-chain ordering surgery.** ext_authz registers as one more entry in the existing extension registry; the HCM filter-chain iteration protocol is unchanged.
- **2.8 No `response_code_details` emission** — envoy-go's HCM does not surface `response_code_details` to local-reply callers (phase-04 scope); ext_authz's deny-path `response_code_details` (`ext_authz_denied`) is a documented divergence-window, joint with the phase-16 rbac + phase-17 jwt_authn `response_code_details` forward-pointer (§9).

---

## 3. Framework survey result (ONE new framework primitive in 18.1)

The framework survey evaluated reuse of phase-09-through-17 primitives BEFORE proposing new (per the phase-16 §10 lesson (a) + phase-17 §3 discipline). Findings for 18.1:

- **Phase-09 `time.AfterFunc` + `cb.ContinueDecoding` async-resume primitives**: **REUSED** — the HTTP-outbound check is async; the filter parks the decode dispatch goroutine and resumes on POST completion. Same async-resume primitive fault introduced; ext_authz is its load-bearing re-consumer for a per-request *outbound* call.
- **Phase-13 ADR-0128 decode-side body-buffering primitives**: **REUSED** — `with_request_body` materializes the request body via ADR-0128 (§6.4 + ADR-0162). Load-bearing reusability demonstration (SECOND consumer of ADR-0128 after phase-15 bandwidth_limit; FIRST to consume it for *outbound transmission* of the body).
- **Phase-17 ADR-0150 `internal/jwks/Fetcher` outbound-HTTP structure**: **COMPOSED-AGAINST** — the HTTP-outbound auth-check (§3.1 + ADR-0159) reuses the `http.Client` + timeout + (optional) retry structure the JWKS fetcher established.
- **ADR-0085 `SendLocalReply` framework primitive**: **REUSED** — the deny-path emission (§4). `content-length` synthesized + the standard header set (`server: envoy`, `date`) per ADR-0085.
- **ADR-0144 `DownstreamPrincipal()`**: NOT reused in 18.1 (the gRPC `AttributeContext.source.principal` is an 18.2 concern; HTTP-mode does not convey a principal field).
- **ADR-0125 8 canonical per-route patterns**: NO NEW canonical — the **5th canonical is REUSED** (§5 + ADR-0163; the FIRST §9 row to REUSE rather than add).

**Zero-delta is NOT feasible** for 18.1 — the `http_service` arm requires the HTTP-outbound auth-check. It is a clean cross-phase-reusable lift.

### 3.1 HTTP-outbound auth-check primitive — ext_authz-local thin client (ADR-0159; disposition: (b))

The HTTP-mode auth-check POSTs the request-side-filtered request to `HttpService.server_uri.uri` and parses the HTTP response into a `checkDisposition`. The BRAINSTORM §3.2 posed an (a)-vs-(b) disposition: (a) generalize the phase-17 outbound-HTTP structure into a shared `internal/httpclient/` package consumed by both `internal/jwks/` and ext_authz; (b) keep a thin ext_authz-local HTTP client that mirrors the JWKS fetcher's structure without a shared package.

**SPEC author's disposition: (b) — a thin ext_authz-local HTTP client.** Rationale: the two consumers have structurally different shapes — `internal/jwks/Fetcher` is a *cached, async-refreshing, scheduled-refetch* fetcher of a long-lived key set; ext_authz's HTTP-mode auth-check is a *synchronous-per-request, cancellable, no-cache* POST-and-parse. Generalizing now would mean designing a shared abstraction against only two consumers whose lifecycles barely overlap — a premature abstraction. The thin local client lives in `check.go` (an `httpAuthClient` type wrapping an `*http.Client` + the configured `HttpService.server_uri.timeout` + `path_prefix`), structurally mirroring the JWKS fetcher's `http.Client`/timeout discipline but without the cache/async-refresh machinery. **Forward-pointer:** the natural trigger to generalize into `internal/httpclient/` is the THIRD outbound-HTTP consumer — a future `oauth2` phase needs an outbound token-endpoint POST that IS synchronous-per-request like ext_authz's; when oauth2 brainstorms, the `internal/httpclient/` generalization should be reconsidered with three consumers in view. ADR-0159 records this disposition + the forward-pointer; the BEHAVIOR_CONTRACT either extends the phase-17 `## JWKS framework primitive` umbrella with a "see also ext_authz HTTP-mode" cross-reference OR anchors a thin `## HTTP outbound auth-check` note (§13.7 — the 18.1 IMPL chooses the lighter-touch BEHAVIOR_CONTRACT shape).

### 3.2 No filter-chain ordering surgery

Per §2.7. ext_authz registers as one more extension-registry entry; the HCM filter-chain iteration protocol, the per-route TPFC resolution, and the async-resume primitive are all consumed as-is.

---

## 4. Deny-path wire shape (deny + error dispositions)

Per parent SPEC §5.P11 RATIFIED + §6 amendment 11. On a **deny** disposition, the filter emits `SendLocalReply(status, body, headers)`:

- **status** — the auth service's HTTP response status (a recognized deny status, `401`/`403`; per parent §5.P10). NOT fixed by the filter — ext_authz is the FIRST §9 row whose deny-path status code comes from the auth service (unlike fault's `[200,600)`-constrained abort, csrf/rbac's fixed 403, local_ratelimit's 429, jwt_authn's 401-or-403).
- **body** — the auth service's HTTP response body, reproduced **verbatim** (per parent §5.P11). `content-length` is synthesized by the `SendLocalReply` framework primitive (ADR-0085).
- **headers** — the auth service's HTTP response headers, filtered through `AuthorizationResponse.allowed_client_headers` (a `ListStringMatcher`). Headers not in the allow-list are dropped. `content-type` is synthesized as `text/plain` if the auth service did not supply one in the allowed set (per parent §5.P11). The framework's standard header set (`server: envoy`, `date`) is preserved. NO `x-envoy-*` header is added on deny. Header ordering: decision headers first, framework housekeeping (`content-length`, `date`, `server: envoy`) after.

On an **error** disposition (connect failure, timeout, unrecognized auth status — per parent §5.P10):

- `failure_mode_allow: false` (proto default) → `SendLocalReply(status_on_error, "", {})` — the `status_on_error.code` (default `403` if `status_on_error` is unset; `503` is the common operator setting per the §5.P10 scrape) is emitted with an empty body. The `error` counter increments.
- `failure_mode_allow: true` → the request is allowed through (`HeaderContinue`); if `failure_mode_allow_header_add: true`, an `x-envoy-auth-failure-mode-allowed: true` header is added to the upstream request so the backend can observe the bypass. Both the `error` AND `failure_mode_allowed` counters increment (per parent §5.P10).

The `with_request_body` over-limit local-reply (per parent §5.P5 + §6 amendment 6) is distinct: `allow_partial_message: false` + an over-`max_request_bytes` body → `SendLocalReply(413, "Payload Too Large", {connection: close})` emitted BEFORE the outbound check fires; the auth service is never contacted; NO `ext_authz` counter increments (the request never reached a disposition). ADR-0156 anchors the deny-path + error-path `SendLocalReply` mechanism; ADR-0162 anchors the over-limit local-reply.

---

## 5. Per-route discipline — 5th canonical REUSE (NO new canonical) + SHARED-stats

Per parent SPEC §5.P2 RATIFIED + §6 amendment 3 + ADR-0163. `ExtAuthzPerRoute` carries one PGV-required oneof `override` with two arms:

- **`disabled` (bool, PGV `const: true`)** — `ExtAuthzPerRoute{disabled: true}` wholly deactivates the filter on the route: `DecodeHeaders` returns `HeaderContinue` immediately, no auth check, no counter increments, request forwards as-is. envoy-go PARSE-REJECTs `disabled: false` (the PGV `const: true` constraint — distinct from the buffer/compressor 5th canonical's unconstrained disabled-bool; a minor wrinkle recorded at parent §6 amendment 3, NOT a new canonical).
- **`check_settings` (`*CheckSettings`, PGV `required` within the arm)** — a NARROWER per-route override carrying `context_extensions` (`map[string]string`; gRPC-mode-only per its proto doc-note — in 18.1 HTTP-mode, `context_extensions` PARSES but has no HTTP-mode effect, documented as a no-op-in-HTTP-mode at §8), `disable_request_body_buffering` (`bool` — overrides the listener-level `with_request_body` to OFF on this route), and `with_request_body` (`*BufferSettings` — a per-route body-buffering override; mutually exclusive with `disable_request_body_buffering` per the proto doc). Exactly analogous to compressor's per-route `ResponseDirectionOverrides` being narrower than its listener-level config.

This maps cleanly onto **ADR-0125's existing 5th canonical** (disabled-bool arm + a NARROWER override sub-message arm in a oneof; the pattern phase-13 buffer + phase-14 compressor already use). **Phase 18 lands NO ADR-0125 amendment paragraph** — the FIRST §9 family-row since phase 13 to REUSE an existing canonical rather than extend the roster. ADR-0163 records the explicit no-amendment 5th-canonical-REUSE classification (the absence of a §(xiv) amendment is itself a recorded decision — the SPEC confirms the 5th-canonical fit holds against the reference-Envoy `ExtAuthzPerRoute`/`CheckSettings` semantics per parent §5.P2).

**Per-route stats SHARED with listener-level** (per parent §6 amendment 8 + ADR-0163): the per-route override adjusts `context_extensions`/buffering but still calls the same auth service — it spawns no new stateful policy-evaluation surface (unlike phase-11/15/16's INDEPENDENT-stats stateful filters). SHARED-stats; MIRRORS phase-12/13/14/17. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request via the existing TPFC resolution machinery; that entry's shape (`disabled` OR `check_settings`-merged) drives the disposition.

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

```go
package extauthz

// TypeURL is the canonical Envoy type-URL for the ext_authz HTTP filter config.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"

// filterName is the canonical Envoy filter name (underscore preserved).
const filterName = "envoy.filters.http.ext_authz"

// New is the HTTPFilterFactory registered at boot per ADR-0072.
func New(ctx envoyhttp.FilterFactoryContext) (envoyhttp.FilterInstanceFactory, error)
```

`New` parses + validates the `ExtAuthz` proto into a `*compiledConfig`, allocates the `filterStats`, and returns a `FilterInstanceFactory` closure that produces per-stream `*filter` values. Mirrors the cors/.../jwtauthn factory shape.

### 6.2 `compiledConfig` + `filterStats` shape

```go
// compiledConfig is the immutable post-parse listener-level config.
type compiledConfig struct {
	checkFn        checkFn          // the resolved transport closure (HTTP-mode in 18.1)
	withRequestBody *bufferSettings // nil if with_request_body unset
	failureModeAllow         bool
	failureModeAllowHeaderAdd bool
	statusOnError            uint32  // default 403
	validateMutations        bool
	clearRouteCache          bool
	allowedHeaders    *stringMatcherList // top-level request-side allow-list; nil = all
	disallowedHeaders *stringMatcherList // top-level request-side deny-list; nil = none
	stats             *filterStats       // SHARED across listener + all per-route
}

// checkDisposition is the mode-agnostic convergence value (parent §4.1).
type checkDisposition struct {
	class        dispositionClass // allow | deny | error
	upstreamSet  []headerMutation // allow-path: headers to inject upstream
	upstreamApp  []headerMutation // allow-path: headers to append upstream
	denyStatus   uint32           // deny-path: from the auth response
	denyBody     []byte           // deny-path: verbatim
	denyHeaders  []headerKV       // deny-path: allowed_client_headers-filtered
}

// checkFn performs the outbound auth check and returns the disposition.
// In 18.1 the only implementation is httpCheck (the http_service arm).
type checkFn func(ctx context.Context, req *authRequest) (checkDisposition, error)

// filterStats — 6 counters per parent §6 amendment 8 + ADR-0163.
// All COUNTERS; namespace http.<HCM_stat_prefix>.ext_authz.<counter> (SN2-reuse).
type filterStats struct {
	ok                 *stats.Counter
	denied             *stats.Counter
	errored            *stats.Counter // "error" — Go keyword avoidance
	disabled           *stats.Counter // STRUCTURALLY UNREACHABLE under MVP (parent §6 amendment 7)
	failureModeAllowed *stats.Counter
	invalid            *stats.Counter // validate_mutations rejection
}
```

The `compiledConfig` struct shape is **mode-agnostic and field-final at 18.1** — 18.2 adds NO field to it; 18.2 only supplies a second `checkFn` constructor (the gRPC arm) and amends ADR-0157's §Decision to touch `buildCompiledConfig`'s `services`-oneof dispatch arm only. The `checkFn` field's only 18.1 implementation is the HTTP-mode closure; the struct itself carries no transport-specific state. The 6 counters allocate **unconditionally** at `New()` time via `newFilterStats(reg, baseStatPrefix(ctx.StatPrefix))` (mirrors phase-17 jwt_authn's unconditional-allocation discipline; ADR-0156). `disabled` registers for scrape-stability but publishes 0 for the listener's lifetime under MVP (it increments only via the deferred runtime `filter_enabled` gate — parent §6 amendment 7). Per ADR-0085 nil-tolerance: `buildCompiledConfig` guards `if ctx.Stats != nil` before `newFilterStats`. The `compiledPerRoute` value (parsed from `ExtAuthzPerRoute`) carries `disabled bool` + a merged `*compiledCheckSettings` (`contextExtensions`, `withRequestBody`-or-disable); SHARED-stats means per-route carries NO `*filterStats` (mirrors phase-17's factoryState simplification).

### 6.3 `DecodeHeaders` body — top-level dispatch

```
DecodeHeaders(headers, endStream):
  1. resolve per-route → cache *compiledPerRoute on filter state
  2. if perRoute.disabled: return HeaderContinue  (no auth call, no counters — parent §6 amendment 7)
  3. effective withRequestBody = perRoute override OR listener-level
  4. if effective withRequestBody != nil AND !endStream:
       set filter.awaitingBody = true; return HeaderStopIteration
       (DecodeData accumulates via the ADR-0128 primitive; on body-complete OR
        over-limit, proceed to step 5 — over-limit + allow_partial_message:false
        emits SendLocalReply(413, "Payload Too Large", {connection: close}) and STOPS)
  5. build the authRequest (request-side-filtered headers + path_prefix + body)
  6. fire the async outbound check:
       return HeaderStopIteration
       go func(){ disp, err := cc.checkFn(ctx, authReq); resume with disp/err }
  7. on resume (cb.ContinueDecoding path):
       - allow: apply upstreamSet/upstreamApp header mutations; if clearRouteCache:
                cb.ClearRouteCache(); cc.stats.ok++; return (continue iteration)
       - deny:  cb.SendLocalReply(disp.denyStatus, disp.denyBody, disp.denyHeaders)
                cc.stats.denied++
       - error: if failureModeAllow: if failureModeAllowHeaderAdd: add
                  x-envoy-auth-failure-mode-allowed:true upstream;
                  cc.stats.errored++; cc.stats.failureModeAllowed++; continue
                else: cb.SendLocalReply(statusOnError, "", {}); cc.stats.errored++
       - invalid (validate_mutations rejection): cc.stats.invalid++; treat as error posture
```

`DecodeData` participates in the ADR-0128 body-buffering interaction when `awaitingBody` (otherwise pass-through). `DecodeTrailers` pass-through. `OnDestroy` cancels the in-flight outbound call's `context.Context`. `SetDecoderCallbacks` stores `cb`. The async-resume leg mirrors phase-09 fault exactly: `StopIteration` returned synchronously; a goroutine performs the (cancellable) outbound call; `cb.ContinueDecoding()` (or the deny `SendLocalReply` path) on completion. The per-request cancellable `context.Context` (cancelled at `OnDestroy`) is the FIRST §9 row's per-request outbound-call cancellation discipline — the 18.1 IMPL session verifies the resume-after-OnDestroy race is guarded (a likely ADR-0044 surface — parent §7).

### 6.4 `buildCompiledConfig` — parse + validate `ExtAuthz` (HTTP-mode)

`buildCompiledConfig(ctx, raw *ext_authzv3.ExtAuthz) (*compiledConfig, error)`:
1. **`services` oneof dispatch:** `nil` → PARSE-REJECT (`ext_authz: services oneof must be set`); `*ExtAuthz_GrpcService` → PARSE-REJECT (`ext_authz: grpc_service mode not yet supported (lands in phase 18.2)`); `*ExtAuthz_HttpService` → build the HTTP-mode `checkFn` (§6.5).
2. **`transport_api_version`:** non-V3 → PARSE-REJECT (ADR-0008).
3. **`with_request_body`:** if set, validate `max_request_bytes > 0` (PGV-mirror); build `*bufferSettings`.
4. **`status_on_error`:** default `403` if unset; else `status_on_error.code`.
5. **`allowed_headers` / `disallowed_headers`:** compile each `ListStringMatcher` → `*stringMatcherList` (exact/prefix/suffix/contains + `ignore_case`; `safe_regex` per the §11-reference RegexMatcher-subset; `custom` PARSE-REJECT).
6. allocate `filterStats` (guarded `if ctx.Stats != nil`).

### 6.5 HTTP-mode `checkFn` construction (`buildHTTPCheckFn`)

`buildHTTPCheckFn(hs *ext_authzv3.HttpService) (checkFn, error)`:
1. validate `hs.server_uri` is set + has a non-empty `uri` (PGV-mirror — `HttpService.server_uri` is not PGV-required per parent §5.P1, the factory rejects an empty one).
2. construct the `httpAuthClient`: `&http.Client{Timeout: hs.server_uri.timeout}` (the thin ext_authz-local client per §3.1).
3. compile `hs.authorization_request.{allowed_headers (deprecated — honored if present), headers_to_add}` + `hs.path_prefix`.
4. compile `hs.authorization_response.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` → `*stringMatcherList` triples.
5. return a `checkFn` closure: build the POST (`path_prefix` + filtered headers + body), `client.Do(req.WithContext(ctx))`, map the HTTP response → `checkDisposition` per parent §5.P10/§5.P11.

### 6.6 `parsePerRoute` + `resolvePerRouteConfig`

`parsePerRoute(raw *ext_authzv3.ExtAuthzPerRoute) (*compiledPerRoute, error)`: validate the `override` oneof is set (PGV-required); `disabled` arm → validate `const: true` (PARSE-REJECT `disabled: false`); `check_settings` arm → compile `context_extensions` + `disable_request_body_buffering` XOR `with_request_body`. `resolvePerRouteConfig` uses the existing 3-tier TPFC resolution (Route > VirtualHost > RouteConfiguration > listener fallback); cached on filter state via the existing pointer-identity `sync.Map` pattern (ADR-0117 + ADR-0125 §(v) 5th-canonical resolution).

---

## 7. Differential fixture `0020-http-ext-authz-http`

Per parent SPEC §2 + BRAINSTORM §6 (split-half). ~7 scenarios; equivalence pattern mirrors phase-13/14/15/16/17. HTTP-mode only (gRPC-mode scenarios land in 18.2's fixture `0021`).

### 7.1 Per-request matrix

| # | Scenario | Auth-server script | Expected disposition | Counter delta assertion |
|---|---|---|---|---|
| 1 | HTTP allow | HTTP 200 + `allowed_upstream_headers` set | 200 backend echo; injected header arrives upstream | `ok=1` |
| 2 | HTTP deny | HTTP 403 + body + `allowed_client_headers` | 403 + body byte-exact + filtered headers | `denied=1` |
| 3 | error → `status_on_error` | auth server unreachable; `failure_mode_allow:false`; `status_on_error:503` | 503 + empty body | `error=1` |
| 4 | `failure_mode_allow` | auth server unreachable; `failure_mode_allow:true` + `failure_mode_allow_header_add:true` | 200 backend echo + `x-envoy-auth-failure-mode-allowed` arrives upstream | `error=1` + `failure_mode_allowed=1` |
| 5 | `with_request_body` | auth server inspects the POST body; allows | 200 backend echo (body materialized via ADR-0128) | `ok=1` |
| 6 | per-route `disabled` | per-route `disabled: true` (no auth call made) | 200 backend echo | **NO `ext_authz` counter increments** (per parent §6 amendment 7 — NOT `disabled=1`) |
| 7 | per-route `check_settings` | per-route `check_settings{disable_request_body_buffering: true}` overriding listener-level `with_request_body`; auth allows | 200 backend echo; body NOT buffered | `ok=1` (SHARED stats) |

Optional 8th scenario at IMPL reshape time: `with_request_body` over-limit + `allow_partial_message:false` → 413 + `connection: close`, auth not called, NO counter increments (parent §5.P5).

### 7.2 Topology + test-helper

`envoy.yaml` + `envoy-go.yaml` each wire an HCM listener with the ext_authz filter (HTTP-mode) + a router, an echo upstream cluster (reuses `test/helpers/echobackend/`), and the ext_authz `http_service.server_uri` pointing at a NEW test-helper: an **in-process HTTP auth server** under `test/helpers/extauthzhttp/` (or analog — IMPL chooses the dir name), returning scriptable status/body/headers, spawned-per-fixture (mirrors the phase-17 `test/helpers/jwksbackend/` spawn-per-fixture lifecycle). Both `envoy.yaml` and `envoy-go.yaml` wire to the same helper URI. Scenario 3+4 (unreachable) stop the helper before the request. The driver in `inputs/driver.go` exercises the 7 scenarios; the harness asserts response status + body byte-equivalence on allow AND deny paths, `/stats/prometheus` counter-delta equivalence on the reachable counters (`ok`/`denied`/`error`/`failure_mode_allowed`/`invalid`), and backend-arrival header assertions (request-side filtering + allow-path upstream injection).

### 7.3 22nd fuzzer

`FuzzExtAuthzConfigParse` at `internal/filter/http/extauthz/fuzz_test.go`. Corpus seeds: each-decision + boundary cases — `http_service` valid/empty-uri, `grpc_service` (PARSE-REJECT path), empty `services` oneof (PARSE-REJECT), `with_request_body` `max_request_bytes` 0/positive, per-route both arms (`disabled: true`/`false`/`check_settings`), `allowed_headers`/`disallowed_headers` matcher variants, error-posture field combinations. 30s/seed under the ADR-0018 budget.

---

## 8. Deferred items (18.1 slice; per parent SPEC §4.4 + §8)

For future-phase consideration (none are blockers for closing row 18.1; all auditable in the ADR-0040 deferral trail):

1. **gRPC service mode** (`grpc_service` arm + the `internal/grpcclient/` primitive + the gRPC-mode `AttributeContext` builder + `CheckResponse` mapping) — lands in **18.2** (the explicit next sub-phase, not a deferral so much as a sequenced split).
2. **The four `*metadata_context_namespaces` fields** + `AuthorizationResponse.dynamic_metadata_from_headers` + `enable_dynamic_metadata_ingestion` + `filter_metadata` — DEFERRED: dynamic-metadata family (joint with phase-16 rbac + phase-17 jwt_authn — ext_authz is the THIRD §9 filter blocked on this family).
3. **`OkHttpResponse.query_parameters_to_set` / `query_parameters_to_remove`** — DEFERRED: path-query rewriting subsystem (ADR-0112; joint with phase-10 header_mutation's `query_parameter_mutations`). Note: `OkHttpResponse` is gRPC-mode; the analogous HTTP-mode concern does not arise in 18.1.
4. **`filter_enabled` (`RuntimeFractionalPercent`) + `filter_enabled_metadata` (`MetadataMatcher`) + `deny_at_disable` (`RuntimeFeatureFlag`)** — DEFERRED: Runtime family + matcher/metadata family. Per parent §5.P12 all three default to no-op when unset, so 18.1 fixture configs need NO explicit settings. Consequence: the `disabled` counter is STRUCTURALLY UNREACHABLE under MVP (parent §6 amendment 7).
5. **`allowed_client_headers_on_success`** — DEFERRED per parent §5.P9 + §6 amendment 9 (decode-side-only filter shape; copying auth-response headers to the downstream RESPONSE on the allow path requires an encode-side leg). Documented as a divergence-window. The IMPL session MAY revisit a stash-for-HCM mechanism (ADR-0161 §Consequences).
6. **`charge_cluster_response_stats` + the cluster-scoped `cluster.<upstream>.ext_authz.{ok,denied,error}` stat triple** — DEFERRED per parent §6 amendment 8 (a NEW stat-namespace pattern; couples to charging-into-the-cluster-stat-tree).
7. **`emit_filter_state_stats` + `bootstrap_metadata_labels_key` + `decoder_header_mutation_rules`** — DEFERRED: `emit_filter_state_stats` couples to the filter-state/access-log family; `bootstrap_metadata_labels_key` couples to node-metadata-labels; `decoder_header_mutation_rules` is the per-rule mutation-rejection surface (distinct from the MVP `validate_mutations` correctness checks — `validate_mutations` IS consumed).
8. **`CheckSettings.context_extensions` in HTTP-mode** — PARSES but is a no-op in 18.1 HTTP-mode (the proto doc-note explicitly says `context_extensions` is "only applied to a filter configured with a `grpc_service`"). 18.2 consumes it for the gRPC `AttributeContext.context_extensions`.
9. **gRPC retry/timeout-policy customization, `GoogleGrpc` arm** — N/A in 18.1; an 18.2 concern.
10. **`response_code_details` emission** (`ext_authz_denied`) — DEFERRED: phase-04 HCM does not surface `response_code_details` to local-reply callers; joint divergence-window with phase-16 rbac + phase-17 jwt_authn (§9).
11. **Access-log integration for ext_authz decision fields** (`%EXT_AUTHZ_*%`-style formatters) — DEFERRED: access-log-extension framework (joint with phase-16/17).

---

## 9. Cross-references against phase-17 deferred-items list — forward-pointer pickup

- **Phase-17 item 3 — `response_code_details` framework primitive**: NO PICKUP at MVP — ext_authz's deny-path `response_code_details` would be `ext_authz_denied`; ext_authz ADDS to the joint-closure forward-pointer (now phases 16 + 17 + 18). Documented at §13.4 phase-18.1 forward-pointer notes.
- **Phase-17 item 4 — dynamic-metadata family**: NO PICKUP — ext_authz's `*metadata_context_namespaces` + dynamic-metadata fields (§8 item 2) EXTEND the dynamic-metadata deferred-cluster; ext_authz is the THIRD §9 filter blocked on it.
- **Phase-17 item 9 — `clear_route_cache` implicit-on-side-effect trigger**: PARTIAL RELEVANCE — ext_authz has its OWN explicit `clear_route_cache` bool (§1 item 3; honored via the phase-10/17 `cb.ClearRouteCache()` primitive) but does NOT pick up the implicit-on-header-mutation trigger jwt_authn deferred.
- **Phase-17 items 1, 2, 5, 6, 7, 8, 10, 11, 12**: NO PICKUP (jwt_authn-specific concerns — none structurally close in phase 18.1).

**Forward-pointer net change for phase 18.1**: 0 closures. Phase 18.1 adds ~11 new deferred items (§8) + EXTENDS the dynamic-metadata-family + Runtime-family + `response_code_details` deferred-clusters.

---

## 10. ADR anchor map (18.1 subset; full 9-ADR map in parent SPEC §7)

The 18.1-landing ADRs, with §Context drafts anchored at the parent SPEC commit; §Decision + §Consequences LAND at the Lands-in-Task per ADR-0044:

| ADR | Subject (18.1 portion) | Lands-in-Task (hypothesis) |
|---|---|---|
| **ADR-0156** | `internal/filter/http/extauthz/` package shape + DECODER-only `HTTPFilter` + 6-base-counter `filterStats` + boot-registration + deny-path `SendLocalReply` mechanism | Task 2 |
| **ADR-0157** | `compiledConfig` shape + `services`-oneof dispatch (gRPC arm PARSE-REJECT in 18.1) + error-posture fields + V3-only PARSE-REJECT + empty-`services` factory rejection + the error-classification boundary in `check.go` | Task 2 |
| **ADR-0159** | HTTP-outbound auth-check primitive — the thin ext_authz-local client (disposition (b) per §3.1); `httpAuthClient` wrapping `*http.Client` + timeout + `path_prefix`; the (a)-vs-(b) record + the oauth2-triggers-generalization forward-pointer | Task 3 |
| **ADR-0160** | `AuthorizationRequest` builder (HTTP-mode portion) — `headers_to_add` + `path_prefix` prepend + the top-level `allowed_headers`/`disallowed_headers` request-side filtering + the deprecated-`AuthorizationRequest.allowed_headers` honored-if-present disposition | Task 4 |
| **ADR-0161** | Bidirectional header-mutation discipline (HTTP-mode portion) — `AuthorizationResponse.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` + `validate_mutations` gating + the deny-path header-set construction + the `allowed_client_headers_on_success` deferral + the stash-for-HCM revisit note | Task 5 |
| **ADR-0162** | Request-body inclusion — `with_request_body{max_request_bytes, allow_partial_message, pack_as_bytes}` + the ADR-0128 reuse + the `allow_partial_message:false` over-limit → 413 + `connection: close` edge case + the `DecodeHeaders`-StopIteration / `DecodeData`-resume interaction | Task 6 |
| **ADR-0163** | Per-route 5th-canonical REUSE classification (NO ADR-0125 amendment) + SHARED-stats + the `CheckSettings` narrower-override + the 6-counter stat surface (SN2-reuse; RATIFIED-PENDING-IMPL-TIME) + the PGV wrinkles | Task 7 |

**Lands-in-Task hypotheses are refined by the 18.1 PLAN.** ADR-0157's §Decision is amended at 18.2 IMPL to activate the `grpc_service` arm. ADR-0044 escape-valve: most-likely 18.1 surface is the async-resume-after-`OnDestroy` race guard (§6.3) — if it needs an ADR-lift, that surfaces at the 18.1 IMPL task.

---

## 11. Empirical-pin block — see parent SPEC §5

> **§11 is intentionally a reference section, not a self-contained pin block** — it diverges deliberately from the phase-17 SPEC §11 structure. The 13 §10 empirical pins span BOTH sub-phases (18.1 + 18.2), so per the parent SPEC §3 split rationale ("the parent master SPEC carries the cross-cutting design ... the full §5 13-pin empirical-pin block") they are resolved ONCE, in the parent master SPEC §5, rather than duplicated across the two sub-phase SPECs. A phase-done reviewer diffing this SPEC against phase-17's reads the pin evidence at `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` §5.

All 13 §10 empirical pins were resolved IN-SESSION per ADR-0004 at the **parent SPEC §5** (probe date 2026-05-14; reference Envoy v1.37.2 at the `ENVOY_TARGET.md` SHA + go-control-plane v1.32.4). The 18.1-load-bearing pins: §18.P1 (proto roster — RATIFIED-AND-EXTENDED), §18.P2 (`ExtAuthzPerRoute`/`CheckSettings` — RATIFIED), §18.P5 (`with_request_body` over-limit — RATIFIED), §18.P6 (stat surface — REFINED), §18.P7 (Prometheus tag-extractor — RATIFIED), §18.P8 (`allowed_headers` matcher subset — RATIFIED-AND-EXTENDED), §18.P9 (`allowed_client_headers_on_success` feasibility — PARTIAL→DEFER), §18.P10 (error-classification boundary — RATIFIED), §18.P11 (deny-path wire shape — RATIFIED), §18.P12 (`filter_enabled` family defaults — RATIFIED). §18.P3/P4/P13 are gRPC-mode (18.2). The §6 amendment block (12 amendments) is the empirical-finding-driven scope revision; amendments 1/2/3/6/7/8/9/10/11 are 18.1-load-bearing (§1.1).

**RATIFIED-PENDING-IMPL-TIME pins for 18.1** (per phase-16 §10 lesson (c) + phase-17 §11.P7 precedent): §18.P6 (the 6-counter stat surface — confirmed at the fixture-harness empirical scrape at the 18.1 stat-surface task) + §18.P7 (the Prometheus tag-extractor SN2-reuse — confirmed at the same task) + the §18.P11 deny-path header-ordering byte-shape (confirmed at the fixture-harness diff). The 18.1 PLAN assigns these RATIFIED-PENDING closures to the relevant impl tasks.

---

## 12. Deferred decisions (the planner / implementer settles these)

1. **`test/helpers/extauthzhttp/` directory name + helper shape** — the in-process HTTP auth server's package name + whether it shares structure with `test/helpers/jwksbackend/`. The 18.1 PLAN/IMPL chooses.
2. **`httpAuthClient` retry discipline** — whether the thin ext_authz-local HTTP client does any retry (the JWKS fetcher has a `RetryPolicy`; `HttpService` has no retry-policy field — MVP likely does ZERO retry, single-attempt-then-error). The 18.1 IMPL confirms against the §5.P10 error-classification boundary.
3. **`extauthz_test.go` vs split test files** — whether the unit tests stay in one file or split (`check_test.go`, `attributes_test.go`) as the surface grows. IMPL-cohesion call.
4. **The async-resume-after-`OnDestroy` race guard** (§6.3) — the exact synchronization primitive (the phase-09 fault pattern vs a fresh guard). Possible ADR-0044 surface.
5. **`safe_regex` RegexMatcher engine** — which regex-engine subset the `allowed_headers` `safe_regex` arm honors (reuses the phase-09/12 RegexMatcher-subset discipline — the 18.1 IMPL confirms the exact subset against v1.37.2).
6. **Deprecated `AuthorizationRequest.allowed_headers` disposition** — whether v1.37.2 still honors it or PARSE-IGNOREs it (parent §6 amendment 2 — the 18.1 IMPL confirms; MVP hypothesis is honored-if-present).
7. **`validate_mutations` validation rule set** — the exact header-name/value safety checks (mirrors the phase-10 header_mutation protected-header discipline; the 18.1 IMPL pins the exact rule set against v1.37.2).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at 18.1 phase-done)

1. **§13.1** — `## HTTP filter chain` → NEW `### envoy.filters.http.ext_authz` subsection (insertion alphabetical-after-`envoy.filters.http.csrf` per ADR-0100 §2.2): the dual-mode service envelope (HTTP-mode detailed; "gRPC mode — see phase 18.2" forward-pointer), the consumed-vs-deferred field map, the request-side header filtering (`allowed_headers`/`disallowed_headers`), the HTTP-mode bidirectional header-mutation discipline, the request-body-inclusion semantics + the over-limit 413 edge case, the `failure_mode_allow` / `status_on_error` error posture, the 5th-canonical-REUSE per-route (SHARED-stats), the deny-path `SendLocalReply` wire shape.
2. **§13.2** — `## Stat-name mapping` → 71 → 77-name table extension (6 new counters: `ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid` under `http.<HCM_stat_prefix>.ext_authz.*`; `disabled` noted STRUCTURALLY UNREACHABLE under MVP).
3. **§13.3** — `## Equivalence Matrix` → NEW row pointing at fixture `0020-http-ext-authz-http` with byte-exact body + status + header-set discipline.
4. **§13.4** — NEW `### Phase 18.1 forward-pointer notes` subsection (under `## Forward-pointer notes`) covering the §8 deferral list + the `response_code_details` joint divergence-window.
5. **§13.5** — `## HTTPFilterCallbacks` — NO extensions (18.1 reuses the phase-09 async-resume + ADR-0128 body-buffering + ADR-0085 `SendLocalReply` primitives as-is).
6. **§13.6** — `## Per-route canonical patterns` — ADR-0125 §(v) 5th-canonical cross-reference noting ext_authz as the FIRST §9 row to REUSE the 5th canonical (NO new amendment paragraph).
7. **§13.7** — the HTTP-outbound auth-check note — per ADR-0159 disposition (b), a thin cross-reference under the phase-17 `## JWKS framework primitive` umbrella ("see also: ext_authz HTTP-mode thin outbound client") OR a short `## HTTP outbound auth-check` note; the 18.1 IMPL chooses the lighter-touch shape.

---

## 14. Testing strategy

### 14.1 Unit tests (`extauthz_test.go`)

Test groups: (1) `ExtAuthz` parse — `http_service` valid, `grpc_service` PARSE-REJECT, empty `services` PARSE-REJECT, non-V3 PARSE-REJECT, `with_request_body` `max_request_bytes` 0 PARSE-REJECT; (2) `compiledConfig` shape — 6-counter `filterStats` allocation, nil-`Stats` tolerance, `status_on_error` default 403; (3) `allowed_headers`/`disallowed_headers` matcher compilation — exact/prefix/suffix/contains/`ignore_case`/`safe_regex`/`custom`-PARSE-REJECT; (4) HTTP-mode `checkFn` — allow/deny/error mapping per parent §5.P10, `path_prefix` prepend, `headers_to_add`; (5) `DecodeHeaders` dispatch — per-route `disabled` short-circuit, async-resume allow/deny/error, `clear_route_cache`; (6) `with_request_body` — body materialization via ADR-0128, over-limit 413 + `connection: close`, `allow_partial_message` true/false; (7) per-route — `parsePerRoute` PGV-mirror (`disabled: false` PARSE-REJECT, empty-`override` PARSE-REJECT), 3-tier resolution, `check_settings` merge; (8) header mutation — allow-path upstream injection, deny-path `allowed_client_headers` filtering, `validate_mutations` rejection → `invalid` counter; (9) `OnDestroy` cancellation — in-flight context cancelled, resume-after-OnDestroy guarded.

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/extauthz/...` + repo-wide race clean. Build/vet/lint clean.

### 14.3 Fuzzer

22nd fuzzer `FuzzExtAuthzConfigParse` (§7.3); 30s ADR-0018 budget. Existing 21 fuzzers re-run clean.

### 14.4 h2spec + differential

h2spec 53/53 PASS at the ADR-0051 pin (no H2 wire-shape change). 21 differential fixtures green at 18.1 phase-done (0000–0020; 0020 NEW — phase 17 ended at 0019, so 0000–0019 is 20 pre-existing + the new 0020 = 21).

### 14.5 Six-gate checklist (A/B/C/D/E/F per BOOTSTRAP_PROMPT.md §7.5)

- **Gate A** (build + vet + lint): green; new `extauthz` package compiles clean.
- **Gate B** (race tests): green; `go test -race ./internal/filter/http/extauthz/...` + repo-wide.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin.
- **Gate D** (fuzzers): 22 fuzzers green at 30s each.
- **Gate E** (differential): 21/21 fixtures green (0000–0020).
- **Gate F** (BEHAVIOR_CONTRACT): the §13 edit bundle landed; `tools/check_behavior_contract.sh` (or analog) green.

---

## 15. Acceptance checklist (for the reviewer)

The 18.1 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts:

1. **Package shape per ADR-0156:** `internal/filter/http/extauthz/{extauthz.go, check.go, attributes.go, extauthz_test.go, fuzz_test.go, doc.go}` with `Decoder: f, Encoder: nil` (decoder-only — 5th §9 row); 6-base-counter `filterStats` (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`) registered unconditionally at `New()` time; `disabled` STRUCTURALLY UNREACHABLE under MVP (publishes 0); compile-time `var _ envoyhttp.StreamDecoderFilter` assertion.
2. **Dual-mode envelope per ADR-0157:** the `services` oneof dispatch — `http_service` builds the HTTP-mode `checkFn`; `grpc_service` PARSE-REJECTs (`grpc_service mode not yet supported (lands in phase 18.2)`); empty `services` PARSE-REJECTs; non-V3 `transport_api_version` PARSE-REJECTs; the error-classification boundary per parent §5.P10.
3. **HTTP-outbound auth-check per ADR-0159:** the thin ext_authz-local `httpAuthClient` (disposition (b)); async outbound POST parking the decode dispatch goroutine on the phase-09 async-resume primitive; `OnDestroy` cancels the in-flight `context.Context`; the (a)-vs-(b) record + oauth2-generalization forward-pointer in ADR-0159 §Decision.
4. **`AuthorizationRequest` builder per ADR-0160:** request-side header filtering through the top-level `ExtAuthz.allowed_headers` minus `disallowed_headers`; `headers_to_add` appended; `path_prefix` prepended; deprecated `AuthorizationRequest.allowed_headers` honored-if-present.
5. **Header-mutation discipline per ADR-0161:** allow-path `allowed_upstream_headers` (set) + `allowed_upstream_headers_to_append` (append) upstream injection; deny-path `allowed_client_headers`-filtered downstream emission; `validate_mutations` gating → `invalid` counter; `allowed_client_headers_on_success` DEFERRED (divergence-window documented).
6. **Request-body inclusion per ADR-0162:** `with_request_body` materialized via the phase-13 ADR-0128 decode-side body-buffering reuse; `allow_partial_message:false` over-limit → `SendLocalReply(413, "Payload Too Large", {connection: close})`, auth not called, NO counter increments; `pack_as_bytes` honored.
7. **Per-route per ADR-0163:** 5th-canonical REUSE (NO ADR-0125 amendment paragraph — the FIRST §9 row to REUSE); `disabled` arm PGV `const: true` (PARSE-REJECT `disabled: false`); `override` oneof PGV-required (PARSE-REJECT empty); `check_settings` narrower-override merge; SHARED-stats (no per-route `*filterStats`); per-route `disabled` → NO counter increments (per parent §6 amendment 7).
8. **Deny + error wire shape per §4 + parent §5.P10/§5.P11:** deny → `SendLocalReply` with the auth service's status + verbatim body + `allowed_client_headers`-filtered headers, `content-length` synthesized, `text/plain` fallback; error + `failure_mode_allow:false` → `status_on_error` (default 403) + empty body; error + `failure_mode_allow:true` → `HeaderContinue` + `x-envoy-auth-failure-mode-allowed` (if `failure_mode_allow_header_add`) + both `error` AND `failure_mode_allowed` increment.
9. **§11 empirical pins:** all 13 §10 pins resolved IN-SESSION at the parent SPEC §5 per ADR-0004; the 18.1-load-bearing pins (§18.P1/P2/P5/P6/P7/P8/P9/P10/P11/P12) reflected in this SPEC's §4/§5/§6/§7; the RATIFIED-PENDING-IMPL-TIME pins (§18.P6 stat surface + §18.P7 tag-extractor + §18.P11 header-ordering) closed at the relevant 18.1 impl tasks.
10. **Differential fixture per §7:** `0020-http-ext-authz-http`, ~7 scenarios; byte-exact body assertion (allow paths backend-echo + deny paths verbatim auth body); per-counter delta equivalence on the 5 reachable counters; per-route 5th-canonical exercised on both arms (scenarios 6 + 7); 1 NEW test-helper (in-process HTTP auth server).
11. **BEHAVIOR_CONTRACT.md populated** per Gate F: §13.1 new `### envoy.filters.http.ext_authz` subsection (HTTP-mode-focused + gRPC forward-pointer); §13.2 stat-table 71 → 77; §13.3 equivalence-matrix row for 0020; §13.4 `### Phase 18.1 forward-pointer notes`; §13.6 ADR-0125 §(v) 5th-canonical-REUSE cross-reference; §13.7 the HTTP-outbound auth-check note.
12. **DECISIONS.md populated** per ADR-on-impl convention: ADR-0156/0157/0159/0160/0161/0162/0163 §Context drafts anchored at the parent SPEC commit; §Decision + §Consequences bodies LAND at each ADR's Lands-in-Task per ADR-0044. ADR-0164 (split-application) landed IN FULL at the parent SPEC commit. NO ADR-0125 amendment paragraph (ADR-0163 records the explicit no-amendment 5th-canonical-REUSE decision).
13. **ROADMAP.md** row `18.1` flips `in-progress → done` at the 18.1 phase-done commit; rows `18` + `18.2` unchanged at that commit (parent stays `in-progress`; `18.2` stays `planned`).
14. **All six phase-done gates green** at the 18.1 phase-done commit: build/vet/lint clean; race-test clean repo-wide; h2spec 53/53 PASS; 22 fuzzers green at 30s; 21 differential fixtures green (0000–0020); BEHAVIOR_CONTRACT.md populated.
15. **No master mutation outside the 18.1 squash-merge commit** — all work landed on the 18.1 worktree branches per ADR-0005 §Decision 4; master tip advances only at the squash-merge commit + SHA-fill follow-up.

End of phase 18.1 SPEC.
