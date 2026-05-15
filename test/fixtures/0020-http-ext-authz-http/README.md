# Fixture 0020 — `envoy.filters.http.ext_authz` HTTP-service mode differential equivalence

Seven scenarios per phase 18.1 SPEC §7.1; sequential against three plaintext
listeners (`l_test_a`, `l_test_b`, `l_test_c`) covering the ext_authz
HTTP-service mode across allow, deny, error, failure-mode-allow, body-buffering,
per-route disabled, and per-route check_settings paths. One echobackend cluster
(`c_backend` — reuses `test/helpers/echobackend/` from phase 14) for
upstream-echo allow paths; one live in-process HTTP auth server (`c_authz` —
`test/helpers/extauthzhttp/`, new in phase 18.1) for scenarios that reach the
auth service; one hardcoded-unreachable cluster (`c_authz_down`) for the error
and failure-mode-allow paths. Plaintext-only per SPEC §7.2 + planner-time
decision D12 — no TLS in phase 18.1. Reference Envoy v1.37.2 (STRICT_DNS via
`host.docker.internal`) vs envoy-go (STATIC, in-process).

## Three-listener topology (Task-11 review-fix)

`failure_mode_allow` is a TOP-LEVEL `ExtAuthz` field — it cannot be set via
`ExtAuthzPerRoute.check_settings`. Scenarios 3 (`failure_mode_allow:false +
status_on_error:503`) and 4 (`failure_mode_allow:true +
failure_mode_allow_header_add:true`) must therefore target DIFFERENT
listener-level configs, requiring three separate listeners:

- **`l_test_a`** (`hcm_local_a`, ref port 10020) — `failure_mode_allow:false`
  (default) + live auth server (`c_authz`). Scenarios 1+2+5+6+7.
  Listener-level `with_request_body` (max 4096B, `allow_partial_message:false`),
  `allowed_headers` (`authority` exact + `x-*` prefix),
  `authorization_request.headers_to_add` (`x-fixture-fingerprint: 0020`),
  `authorization_response.allowed_upstream_headers` (`x-authz-result`),
  `authorization_response.allowed_client_headers` (`x-authz-denied`).

- **`l_test_b`** (`hcm_local_b`, ref port 10021) — `failure_mode_allow:false` +
  `status_on_error: ServiceUnavailable` (503) + unreachable auth (`c_authz_down`).
  Scenario 3.

- **`l_test_c`** (`hcm_local_c`, ref port 10022) — `failure_mode_allow:true` +
  `failure_mode_allow_header_add:true` + unreachable auth (`c_authz_down`).
  Scenario 4.

## Auth-server lifecycle

The in-process HTTP auth server (`test/helpers/extauthzhttp/`) runs on a single
stable port allocated at driver-instantiation time. It is restarted between
scenario windows that require a different `Script`:

- **S1**: `FixedScript(200, nil, {"x-authz-result": "allowed"})`.
- **S2**: `FixedScript(403, []byte("access denied"), {"x-authz-denied": "true"})`.
- **S3+S4**: Server stays running from S2 (irrelevant — these listeners point at
  `c_authz_down`).
- **S5**: `InspectScript` — allows if POST body contains `"hello"`.
- **S6**: No restart (server stays from S5; filter bypassed entirely).
- **S7**: `FixedScript(200, nil, {"x-authz-result": "allowed"})`.
- Teardown: server stopped after S7.

## Scenarios

1. **HTTP allow** — `GET /scenarios/1-allow` with `x-client-id: scenario-1`
   against `l_test_a`. Auth returns 200 + `x-authz-result: allowed`. Both sides:
   200 + echobackend echo body; `x-authz-result: allowed` arrives upstream
   (allowed_upstream_headers passthrough). `ok` +1.

2. **HTTP deny** — `POST /scenarios/2-deny` with body `"request-body"` against
   `l_test_a`. Auth returns 403 + body `"access denied"` + `x-authz-denied:
   true`. Both sides: 403 + body byte-exact `"access denied"` (13B);
   `x-authz-denied: true` header present in response (allowed_client_headers
   passthrough). `denied` +1.
   *§18.P11 confirmed: decision header `x-authz-denied` precedes framework
   housekeeping on both sides — see §18.P11 note in `expectations.yaml`.*

3. **Error → status_on_error** — `GET /scenarios/3-error` against `l_test_b`.
   Auth cluster `c_authz_down` is unreachable; connection refused. Both sides:
   503 + empty body (`status_on_error: ServiceUnavailable`). `error` +1.

4. **failure_mode_allow** — `GET /scenarios/4-failure-allow` against `l_test_c`.
   Auth cluster `c_authz_down` is unreachable; connection refused.
   `failure_mode_allow:true + failure_mode_allow_header_add:true` → filter allows
   through with `x-envoy-auth-failure-mode-allowed: true` injected upstream. Both
   sides: 200 + echobackend echo body containing the marker header. `error` +1,
   `failure_mode_allowed` +1.

5. **with_request_body** — `POST /scenarios/5-body` with body `"hello world"`
   against `l_test_a`. Listener-level `with_request_body` gates the auth check
   on full body arrival (max 4096B). Auth uses `InspectScript` — reads the POST
   body, allows if it contains `"hello"`, returns 200 + `x-authz-result:
   allowed`. Both sides: 200 + echobackend echo body with injected header. `ok`
   +1.

6. **per-route disabled** — `GET /scenarios/6-disabled` against `l_test_a`.
   Route carries `ExtAuthzPerRoute{disabled: true}` — ext_authz filter is
   bypassed for this route. Both sides: 200 + echobackend echo body (NO auth
   check). NO counter increments per §6 amendment 7.

7. **per-route check_settings** — `POST /scenarios/7-check-settings` with body
   `"per-route-body-check"` against `l_test_a`. Route carries
   `ExtAuthzPerRoute{check_settings{disable_request_body_buffering: true}}` —
   overrides the listener-level `with_request_body` and disables body buffering
   for this route. Auth receives an empty body and allows (FixedScript 200 +
   `x-authz-result: allowed`). Both sides: 200 + echobackend echo body. `ok`
   +1.

## Counter expectations (per SPEC §7.5 + Task-13 empirical scrape)

All counters HCM-rooted under `http.<hcm>.ext_authz.<counter>` (Prometheus
`envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<hcm>"}`) per
ADR-0163 SHARED-stats. `lookupExtAuthzCounter` sums across all three
`hcm_local_a`, `hcm_local_b`, `hcm_local_c` label values.

| counter | envoy-go | reference | asserted |
|---|---|---|---|
| `ok` | +3 | +3 | cross-side equivalence |
| `denied` | +1 | +1 | cross-side equivalence |
| `error` | +2 | +2 | cross-side equivalence |
| `failure_mode_allowed` | +1 | +1 | cross-side equivalence |
| `invalid` | +0 | +0 | cross-side equivalence |
| `disabled` | +0 | +0 | not asserted (divergence-window 1) |

## Divergence-windows

1. **`disabled` counter structurally unreachable.** envoy-go MVP does not
   implement the `filter_enabled` gate (§6 amendment 7 — deferred); the
   `disabled` counter is registered in `filterStats` (6-counter completeness)
   but NEVER incremented. Scenario 6's `ExtAuthzPerRoute{disabled: true}`
   bypasses the filter with NO counter increments. Reference Envoy also
   publishes `disabled=0` for this fixture (the config does not set
   `filter_enabled`). NOT asserted.

2. **Reference Envoy extra stat names.** Reference Envoy v1.37.2 publishes
   additional counters not in the envoy-go 6-counter MVP surface
   (`request_header_limits_reached`, `response_header_limits_reached`,
   `omitted_response_headers`, `ignored_dynamic_metadata`,
   `filter_state_name_collision`). The driver's `AssertStats` only queries the
   5 cross-side-equivalent counters above.

3. **Dynamic metadata / filter_state silent-ignored.** envoy-go MVP
   silent-ignores `dynamic_metadata_keys`, `filter_state_rules`, and the
   dynamic-metadata propagation family. Fixture 0020's config does NOT set
   these fields.

4. **gRPC mode (phase 18.2 scope).** envoy-go MVP implements HTTP-mode
   ext_authz only (`http_service` arm). gRPC mode (`grpc_service`) is deferred
   to phase 18.2. Fixture 0020 uses `http_service` exclusively.

5. **Response-code-details on deny/error.** envoy-go silent; Envoy emits
   `ext_authz_error` / `ext_authz_denied` response_code_details. Not asserted
   by this fixture.

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS clusters via
  `host.docker.internal`; three listeners on ports 10020/10021/10022).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC clusters via `127.0.0.1`;
  runner-allocated ports substituted via Go template).
- `inputs/driver.go` — the 7-scenario multi-listener driver; registers the
  fixture with the differential runner, drives both proxies, and asserts
  byte-stream + counter equivalence.
- `expectations.yaml` — prose expectations + counter-delta map +
  divergence-window roster + §18.P11 closure note (ADR-0019 — the driver is
  the enforcer).

Cross-refs: SPEC §7.1 + §7.2 + §4 + §5.P11 + §6 amendment 7 + §7.3 + §7.4 +
§7.5 + ADR-0156 + ADR-0157 + ADR-0159 + ADR-0160 + ADR-0161 + ADR-0162 +
ADR-0163 + BEHAVIOR_CONTRACT §13.5 + planner-time decisions D2 + D12.
