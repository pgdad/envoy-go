# Fixture 0021 — `envoy.filters.http.ext_authz` gRPC-service mode differential equivalence

Eight scenarios per phase 18.2 SPEC §7.1; sequential against three plaintext
listeners (`l_test_a`, `l_test_b`, `l_test_c`) covering the ext_authz
gRPC-service mode across allow, deny, error, failure-mode-allow,
with_request_body, per-route disabled, per-route context_extensions, and
`OkHttpResponse` upstream-mutation paths. One echobackend cluster (`c_backend`
— reuses `test/helpers/echobackend/` from phase 14) for upstream-echo allow
paths; one live in-process gRPC `Authorization/Check` auth server
(`c_authz_grpc` — `test/helpers/extauthzgrpc/`, new in phase 18.2) for
scenarios that reach the auth service. Plaintext-only per SPEC §7.2 +
§11.P13 + planner-time decision D10 — no TLS in phase 18.2 fixtures
(TLS-plumbing closed in-session at SPEC §11.P13 RATIFICATION). Reference
Envoy v1.37.2 (STRICT_DNS via `host.docker.internal`) vs envoy-go (STATIC,
in-process).

## Three-listener topology (per SPEC §7.2 + D10 — mirrors 18.1's 0020 pattern)

`failure_mode_allow` is a TOP-LEVEL `ExtAuthz` field — it cannot be set via
`ExtAuthzPerRoute.check_settings` (per 18.1 SPEC §10 notable lesson). Scenarios
3 (`failure_mode_allow:false + status_on_error:503`) and 4
(`failure_mode_allow:true + failure_mode_allow_header_add:true`) must therefore
target DIFFERENT listener-level configs, requiring three separate listeners:

- **`l_test_a`** (`hcm_local_a`, ref port 10021) — `failure_mode_allow:false`
  (default) + live gRPC auth (`c_authz_grpc`). Scenarios 1+2+5+6+7+8.
  Listener-level `with_request_body{max_request_bytes:8192,
  allow_partial_message:true}` for scenario 5. Per-route overrides:
  - `/disabled` → `ExtAuthzPerRoute{disabled:true}` (scenario 6 — 5th-canonical
    `disabled` arm).
  - `/ctx` → `ExtAuthzPerRoute{check_settings:{context_extensions:{policy:scenario7}}}`
    (scenario 7 — 5th-canonical `check_settings` arm; gRPC-only
    `context_extensions` field — see SPEC §5 + §7.1).

- **`l_test_b`** (`hcm_local_b`, ref port 10022) — `failure_mode_allow:false` +
  `status_on_error: ServiceUnavailable` (503) + same `c_authz_grpc` cluster
  (auth server STOPPED by driver before scenario 3 → gRPC dial fails →
  dispError → 503 LocalReply). Scenario 3.

- **`l_test_c`** (`hcm_local_c`, ref port 10023) — `failure_mode_allow:true` +
  `failure_mode_allow_header_add:true` + same `c_authz_grpc` cluster (auth
  server stays STOPPED through scenario 4 → gRPC dial fails → request
  PROCEEDS to backend with `x-envoy-auth-failure-mode-allowed:true` injected
  upstream). Scenario 4.

The `c_authz_grpc` cluster carries the mandatory
`typed_extension_protocol_options.HttpProtocolOptions.explicit_http_config.http2_protocol_options:{}`
block for gRPC framing per SPEC §11.P13 + §6.5 (UseH2() == true gate). Plaintext
h2c — no TLS — permitted per ADR-0166 (the H2-without-TLS relaxation landed at
Task 11.5 to support fixture 0021's in-process gRPC auth server).

## Per-route 5th-canonical REUSE discipline (per ADR-0163; NO ADR-0125 amendment)

Both per-route overrides exercise the 5th-canonical `ExtAuthzPerRoute` shape
that 18.1 ratified (`disabled: const:true` OR `check_settings`). Phase 18.2
exercises a SECOND `check_settings` field (`context_extensions`) but does NOT
amend the canonical pattern roster — ADR-0163 confirmed the 5th-canonical-REUSE
classification at 18.1. NO ADR-0125 §(xiv) amendment paragraph at 18.2.

## SHARED-stats discipline (per ADR-0163)

All three listeners share the SAME ext_authz stat namespace because the
per-listener stats are registered under the HCM stat_prefix and the driver's
`lookupExtAuthzCounter` sums across all three (`hcm_local_a/b/c`). Per-route
overrides DO NOT carry their own stat scope under MVP — they share the
listener's stats (per ADR-0163 SHARED-stats §Decision).

## Auth-server lifecycle (per the extauthzgrpc helper + SPEC §7.4)

The in-process gRPC auth server (`test/helpers/extauthzgrpc/Server`) is started
fresh at the beginning of each `driveProxy` run on a single STABLE port
(allocated lazily at driver instantiation via `extauthzgrpc.NewAtAddr(addr)`).
It is pre-populated with 5 scripts keyed by `:path` discriminator (per SPEC
§7.4):

- `/scenario1` → `CheckResponse{status:0, OkResponse: OkHttpResponse{}}`.
- `/scenario2` → `CheckResponse{status:7 (PERMISSION_DENIED), DeniedResponse:
  DeniedHttpResponse{status:403, body:"access denied",
  headers:[{key:x-authz-denied-reason, value:scenario2}]}}`.
- `/scenario5` → `CheckResponse{status:0, OkResponse: OkHttpResponse{}}` (the
  auth server sees the buffered body in `AttributeContext.request.http.body`).
- `/ctx` → `CheckResponse{status:0, OkResponse: OkHttpResponse{
  headers:[{key:x-authz-policy, value:scenario7, OVERWRITE_IF_EXISTS_OR_ADD}]}}`
  (coarse confirmation that the auth received
  `AttributeContext.context_extensions[policy]=scenario7`).
- `/scenario8` → `CheckResponse{status:0, OkResponse: OkHttpResponse{
  headers:[<4-arm append_action injection>],
  headers_to_remove:[x-fixture-supplied-removable]}}`.

Scenarios 3+4 do NOT register a script — the driver STOPS the server before
scenarios 3+4 so the gRPC dial fails. Scenario 6 (per-route disabled) bypasses
the filter entirely; its `:path` (`/disabled`) is never reached by the auth
side.

Lifecycle sequence inside `driveProxy`:

1. Start auth server + register all 5 scripts.
2. Scenarios 1, 2, 5 (sequential on `l_test_a`).
3. STOP auth server.
4. Scenario 3 (`l_test_b`) — gRPC dial fails → 503.
5. Scenario 4 (`l_test_c`) — gRPC dial fails → 200 + failure_mode_allow header.
6. RESTART auth server (re-register the 5 scripts).
7. Scenarios 6, 7, 8 (sequential on `l_test_a`).
8. Stop auth server at teardown.

## Divergence-window (phase-18.2-specific; per SPEC §8 + §13.4 + driver Task 12 empirical)

1. **`OkHttpResponse.response_headers_to_add` DEFERRED (SPEC §8 item 5).** The
   field PARSES; envoy-go does not inject the headers into the downstream
   allow-path response (decode-side-only — same family as
   `allowed_client_headers_on_success` deferred at 18.1). Fixture 0021 does NOT
   exercise this field; the divergence-window is documented but not
   surfaced empirically.

2. **`header_map` arm CONDITIONALLY DEFERRED (SPEC §8 item 8).**
   `encode_raw_headers: true` would populate `request.http.header_map` (ordered
   alternative to the legacy `headers` map). envoy-go defaults to `false`;
   reference Envoy default behavior matches. Fixture 0021 does NOT set the
   flag — the legacy `headers` map is exercised, byte-equivalent.

3. **OkResponse + non-zero-status + DeniedResponse + zero-status → dispError
   (SPEC §6.7 envoy-go-strict).** Reference Envoy v1.37.2 leniently accepts
   these structurally-inconsistent oneof combinations; envoy-go-strict treats
   both as dispError to surface auth-server bugs. Fixture 0021 does NOT
   exercise these directly — the 23rd fuzzer `FuzzCheckResponseMapping` (SPEC
   §7.3) is the primary coverage. Documented in BEHAVIOR_CONTRACT §13.4.

4. **OVERWRITE_IF_EXISTS no-op-when-absent (D5 4-arm dispatch).** Reference
   Envoy v1.37.2 collapses `OVERWRITE_IF_EXISTS` toward
   `OVERWRITE_IF_EXISTS_OR_ADD` semantics when the upstream is absent
   (observed empirically at Task 12 scenario 8: the auth-injected header
   `x-overwrite-only-if-exists:v3` arrives upstream on the reference side even
   though the client never sent the key). envoy-go-strict per ADR-0161 +
   SPEC §6.7 treats the arm as a NO-OP when the key is absent. The driver
   does NOT assert on this arm (only the 3 reachable arms).

5. **User-Agent default re-injection (Go net/http quirk).** Go's
   `net/http.Request.Write` re-injects `User-Agent: Go-http-client/1.1` when
   the request header is absent at upstream-write time — masking the filter's
   `headers.Del("user-agent")` from the headers_to_remove path. This is NOT
   an ext_authz divergence; it's a Go stdlib behavior in the router's upstream
   write step. Fixture 0021 works around it by using
   `x-fixture-supplied-removable` (an arbitrary header with no special
   stdlib handling) as the headers_to_remove test target.

6. **`gRPC core.GrpcService.{initial_metadata, retry_policy}` SILENT-IGNORED
   (SPEC §8 items 2+3).** Fields PARSE but have no effect. Fixture 0021 does
   NOT set them.

7. **`OkHttpResponse.{query_parameters_*, dynamic_metadata}` +
   `CheckResponse.dynamic_metadata` DEFERRED (SPEC §8 items 6+7).** Fixture
   0021 does NOT exercise them.

8. **Carry-forward 18.1 divergence-windows (SPEC §8 item 1).** All 11 phase-18.1
   carry-forwards apply unchanged: `*metadata_context_namespaces`,
   `filter_enabled` family, `enable_dynamic_metadata_ingestion`,
   `filter_metadata`, `charge_cluster_response_stats` + cluster-scoped stat
   triple (`disabled` counter structurally unreachable),
   `emit_filter_state_stats`, `bootstrap_metadata_labels_key`,
   `decoder_header_mutation_rules`, `allowed_client_headers_on_success`,
   `response_code_details` emission, access-log integration.

## Scenarios

1. **gRPC allow** — `GET /scenario1` with `x-client-id: scenario-1` against
   `l_test_a`. Auth returns `OK + OkHttpResponse{}`. Both sides: 200 +
   echobackend echo body. `ok` +1.

2. **gRPC deny** — `POST /scenario2` with body `"request-body"` against
   `l_test_a`. Auth returns `PERMISSION_DENIED + DeniedResponse{403,
   "access denied", x-authz-denied-reason:scenario2}`. Both sides: 403 + body
   byte-exact `"access denied"` (13B); `x-authz-denied-reason: scenario2`
   header present in response (VERBATIM pass-through per parent §5.P11 — UNLIKE
   HTTP-mode's matcher-filtered headers). `denied` +1.

3. **Error → status_on_error** — `GET /scenario3` against `l_test_b`. Auth
   server STOPPED → gRPC dial fails fast (connection refused). Both sides: 503
   + empty body (`status_on_error: ServiceUnavailable`). `error` +1.

4. **failure_mode_allow** — `GET /scenario4` against `l_test_c`. Auth server
   STOPPED → gRPC dial fails. `failure_mode_allow:true +
   failure_mode_allow_header_add:true` → filter allows through with
   `x-envoy-auth-failure-mode-allowed: true` injected upstream. Both sides:
   200 + echobackend echo body with the marker header. `error` +1,
   `failure_mode_allowed` +1.

5. **with_request_body (gRPC)** — `POST /scenario5` with body `"hello world"`
   against `l_test_a`. Listener-level `with_request_body` gates the auth check
   on full body arrival (max 8192B, allow_partial_message:true). Auth returns
   `OK + OkHttpResponse{}`; the buffered body populates
   `AttributeContext.request.http.body` (per ADR-0128 + SPEC §6.6 step 3).
   Both sides: 200 + echobackend echo body. `ok` +1.

6. **per-route disabled** — `GET /disabled` against `l_test_a`. Route carries
   `ExtAuthzPerRoute{disabled:true}` — ext_authz filter is bypassed for this
   route. Both sides: 200 + echobackend echo body (NO auth check). NO counter
   increments per parent §6 amendment 7.

7. **per-route context_extensions** — `GET /ctx` against `l_test_a`. Route
   carries `ExtAuthzPerRoute{check_settings:{context_extensions:{policy:scenario7}}}` —
   populates `AttributeContext.context_extensions["policy"]="scenario7"` (per
   SPEC §5 + §6.6 step 8). Auth's `/ctx` script echoes
   `x-authz-policy:scenario7` upstream for coarse confirmation. Both sides:
   200 + echobackend echo body containing `x-authz-policy=scenario7`. `ok`
   +1 (SHARED stats).

8. **OkHttpResponse upstream mutation** — `GET /scenario8` with a
   client-supplied `x-fixture-supplied-removable` header against `l_test_a`.
   Auth returns `OK + OkHttpResponse{headers:[<4-arm append_action injection>],
   headers_to_remove:[x-fixture-supplied-removable]}` (one entry per
   `append_action` arm + headers_to_remove). Both sides: 200 + echobackend
   echo body. Upstream-arrival assertions:
   - `x-injected-by-authz=scenario8` PRESENT (OVERWRITE_IF_EXISTS_OR_ADD).
   - `x-also-appended=append1` PRESENT (APPEND_IF_EXISTS_OR_ADD).
   - `x-add-if-absent=v4` PRESENT (ADD_IF_ABSENT).
   - `x-overwrite-only-if-exists` NOT asserted (OVERWRITE_IF_EXISTS no-op
     under envoy-go-strict; ref Envoy emits it anyway — see divergence-window
     #4 above).
   - `x-fixture-supplied-removable` ABSENT (headers_to_remove strips it).
   `ok` +1.

## Counter expectations (per SPEC §7.5 + Task-12 empirical scrape)

All counters HCM-rooted under `http.<hcm>.ext_authz.<counter>` (Prometheus
`envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<hcm>"}`) per
ADR-0163 SHARED-stats. `lookupExtAuthzCounter` sums across all three
`hcm_local_a`, `hcm_local_b`, `hcm_local_c` label values.

| counter | envoy-go | reference | asserted |
|---|---|---|---|
| `ok` | +4 | +4 | cross-side equivalence (scenarios 1, 5, 7, 8) |
| `denied` | +1 | +1 | cross-side equivalence (scenario 2) |
| `error` | +2 | +2 | cross-side equivalence (scenarios 3+4) |
| `failure_mode_allowed` | +1 | +1 | cross-side equivalence (scenario 4) |
| `invalid` | +0 | +0 | cross-side equivalence |
| `disabled` | +0 | +0 | not asserted (structurally unreachable) |

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS clusters via
  `host.docker.internal`; three listeners on ports 10021/10022/10023; the
  `c_authz_grpc` cluster carries the mandatory `http2_protocol_options:{}`
  + plaintext h2c).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC clusters via `127.0.0.1`;
  runner-allocated ports substituted via Go template; H2-without-TLS upstream
  permitted per ADR-0166).
- `inputs/driver.go` — the 8-scenario multi-listener driver; registers the
  fixture with the differential runner, manages the auth-server lifecycle
  (start → stop before S3+4 → restart before S6+7+8), drives both proxies,
  and asserts byte-stream + counter equivalence.
- `expectations.yaml` — prose expectations + counter-delta map +
  divergence-window roster + §18.P4/§18.P11/§18.P13 closure notes (ADR-0019 —
  the driver is the enforcer; this file is documentation).

Cross-refs: SPEC §7.1 + §7.2 + §7.5 + §5 + §6 amendment 7 + §6.5 + §6.6 + §6.7 +
§8 + §11.P4 + §11.P11 + §11.P13 + ADR-0156 + ADR-0157 + ADR-0158 + ADR-0160 +
ADR-0161 + ADR-0162 + ADR-0163 + ADR-0165 + ADR-0166 + BEHAVIOR_CONTRACT
§13.4 phase-18.2 forward-pointer subsection + planner-time decisions D5 + D10
+ D11.
