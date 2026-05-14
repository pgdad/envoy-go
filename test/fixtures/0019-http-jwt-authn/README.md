# Fixture 0019 — `envoy.filters.http.jwt_authn` differential equivalence

Eight scenarios per phase 17 SPEC §7.1; sequential against one plaintext
listener (`l_test_a` HCM `hcm_local_a` with filter chain `jwt_authn → router`)
with three routes (`/`, `/alt-req`, `/per-route-disabled`). Two clusters back
the listener: `c_backend` (echobackend subprocess — reuses
`test/helpers/echobackend/` from phase 14) for the upstream-echo allow paths,
and `c_jwks_backend` (in-process JWKS server — NEW `test/helpers/jwksbackend/`
in phase 17) serving the RemoteJwks JWK Sets. Plaintext-only per SPEC §7.4 —
no mTLS in phase 17. Reference Envoy v1.37.2 (STRICT_DNS via
`host.docker.internal`) vs envoy-go (STATIC, in-process).

## PKI

`pki/gen.go` generates fresh key material at fixture-load time (planner-time
decision 11 — no checked-in private keys):

- 3 RSA-2048 keypairs — `RS256Key1` (provider-rs256), `RS256Key3` (provider-alt),
  plus a spare; serialized as JWK Sets served by the in-process JWKS backend at
  `/.well-known/jwks-rs.json` + `/.well-known/jwks-alt.json`.
- 1 ECDSA-P256 keypair — `ES256Key` (provider-es256); serialized as a JWK Set
  embedded inline in the listener config as `local_jwks.inline_string`.
- 1 "tampered" RSA-2048 key — never published in any JWK Set; used to sign
  scenario 5's bad-signature token (carrying provider-rs256's `kid` so kid
  lookup succeeds but signature verification fails).

## Listener config (verbatim per SPEC §7.2)

```yaml
envoy.filters.http.jwt_authn:
  bypass_cors_preflight: true
  providers:
    provider-rs256:   # RemoteJwks → c_jwks_backend /.well-known/jwks-rs.json
    provider-es256:   # LocalJwks  → inline_string ES256 JWK Set
    provider-alt:     # RemoteJwks → c_jwks_backend /.well-known/jwks-alt.json
  rules:
    - match: { prefix: / }
      requires:
        requires_any: [provider-rs256, provider-es256]
  requirement_map:
    alt-req: { provider_name: provider-alt }
```

Route `/alt-req` carries per-route TPFC
`PerRouteConfig{requirement_name: "alt-req"}` — the 8th-canonical
string-reference-delegation per ADR-0125 §(xiii). Route `/per-route-disabled`
carries `PerRouteConfig{disabled: true}` — the 8th-canonical disable arm.

## Host-header pin

The differential harness reaches the reference proxy and the subject proxy at
different addresses, and reference Envoy's jwt_authn reflects the request's
full URL into the WWW-Authenticate `realm` (`<scheme>://<authority><path>`).
The driver pins `Host: jwt-authn.fixture.test` on every request so both
proxies observe the identical authority — the realm is then byte-equivalent
per-side.

## Scenarios

1. **valid-RS256-RemoteJwks-allow** — `GET /` with `Authorization: Bearer
   <RS256 token>` → 200 + echobackend echo. Listener rule `prefix /` →
   `requires_any` → provider-rs256 validates against the RemoteJwks fetched at
   filter load. ALLOW; `allowed` +1.
2. **valid-ES256-LocalJwks-allow** — `GET /` with `Authorization: Bearer
   <ES256 token>` → 200 + echobackend echo. provider-es256 validates via the
   inline LocalJwks (no JWKS fetch). ALLOW; `allowed` +1.
3. **missing-token-deny** — `GET /` with NO `Authorization` → 401 + body
   byte-exact `Jwt is missing` (14B) + WWW-Authenticate
   `Bearer realm="http://jwt-authn.fixture.test/"` (NO error param — JwtMissed
   case per §1.1 amendment 12). `denied` +1.
4. **expired-token-deny** — `GET /` with an RS256 token whose `exp` is in the
   past → 401 + body byte-exact `Jwt is expired` (14B) + WWW-Authenticate
   `Bearer realm="http://jwt-authn.fixture.test/", error="invalid_token"`.
   `denied` +1.
5. **bad-signature-deny** — `GET /` with a token signed by the tampered RSA
   key (carrying provider-rs256's `kid`) → 401 + body byte-exact
   `Jwt verification fails` (22B) + WWW-Authenticate with `error="invalid_token"`.
   `denied` +1.
6. **bypass-cors-preflight** — `OPTIONS /` with `Origin` +
   `Access-Control-Request-Method`, NO `Authorization`. `bypass_cors_preflight:
   true` → the preflight predicate (`:method == OPTIONS && origin != "" &&
   access-control-request-method != ""`, §11.P1 verbatim) fires → 200 +
   echobackend echo (validation bypassed). `cors_preflight_bypassed` +1.
   *Reference Envoy also increments `allowed` here — see Divergence-windows 1.*
7. **per-route requirement_name delegation** — `GET /alt-req` with an RS256
   token valid for provider-alt → 200 + echobackend echo. The per-route TPFC
   `PerRouteConfig{requirement_name: "alt-req"}` resolves at request time
   against `requirement_map["alt-req"]` = provider-alt. ALLOW; `allowed` +1.
8. **per-route disabled** — `GET /per-route-disabled` with NO `Authorization`.
   The per-route TPFC `PerRouteConfig{disabled: true}` bypasses jwt_authn
   wholly for this route → 200 + echobackend echo. NO counter increment on the
   envoy-go side per §1.1 amendment 5.
   *Reference Envoy increments `allowed` here — see Divergence-windows 1.*

## Counter expectations (per SPEC §7.5 + Task-13 empirical scrape)

All counters HCM-rooted under `http.hcm_local_a.jwt_authn.<counter>`
(Prometheus `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="hcm_local_a"}`
— identical form on both sides; §11.P7 SN2-reuse RATIFIED at Task 8).

| counter | envoy-go | reference | asserted |
|---|---|---|---|
| `allowed` | +3 | +5 | per-side (divergence-window 1) |
| `denied` | +3 | +3 | cross-side equivalence |
| `cors_preflight_bypassed` | +1 | +1 | cross-side equivalence |
| `jwks_fetch_success` | +2 | +2 | cross-side equivalence |
| `jwks_fetch_failed` | +0 | +0 | cross-side equivalence |
| `jwt_cache_hit` | +0 | +0 | not asserted (divergence-window 2) |
| `jwt_cache_miss` | +0 | +9 | not asserted (divergence-window 2) |

## Divergence-windows

1. **`allowed` counter on bypass paths.** Reference Envoy increments `allowed`
   on EVERY request that clears the filter gate, including CORS-preflight
   bypass (scenario 6) and per-route `disabled: true` passthrough (scenario 8).
   envoy-go MVP increments `allowed` ONLY on an active-engine ALLOWED result
   per SPEC §3 + §1.1 amendment 5 ("PerRouteConfig{disabled: true} → no counter
   increments"). SPEC-mandated envoy-go behaviour, not a bug — asserted
   per-side.
2. **`jwt_cache_hit` / `jwt_cache_miss` structurally unreachable.** envoy-go
   MVP silent-ignores `jwt_cache_config` (§8 deferral 8); the validated-JWT LRU
   cache is never constructed. The two counters are registered (71-name
   stat-table completeness) but never incremented. Reference Envoy's cache is
   always active.
3. **`response_code_details` on DENY** (§1.1 amendment 11 + §8 deferral 13) —
   envoy-go silent; Envoy emits `jwt_authn_access_denied{<reason>}`. Not
   asserted by this fixture.
4. **`filter_state_rules`** (§1.1 amendment 1 + §8 deferral 12), **the
   dynamic-metadata family** (§8 deferrals 1-4), **the v1.37.x claim-coverage
   extensions** `subjects` / `require_expiration` / `max_lifetime` (§1.1
   amendments 2-3 + §8 deferrals 15-17), and **`clear_route_cache`
   implicit-on-side-effect** (§8 deferral 18) are all silent-ignored by
   envoy-go MVP. Fixture 0019's config does not set any of these fields, so
   these divergence-windows are not surfaced empirically.

See `expectations.yaml` for the full per-side counter-delta map and
divergence-window roster.

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS clusters via
  `host.docker.internal`).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC clusters via `127.0.0.1`).
- `pki/gen.go` — fresh RSA + ECDSA keypair + JWK Set generation at
  fixture-load time.
- `inputs/driver.go` — the 8-scenario driver; registers the fixture with the
  differential runner, drives both proxies, and asserts byte-stream + counter
  equivalence.
- `expectations.yaml` — prose expectations + divergence-window roster
  (ADR-0019 — the driver is the enforcer).

Cross-refs: SPEC §7 + §1.1 amendments 1-3, 5, 8-12 + §11.P1-P2, P6-P7 +
ADR-0148, ADR-0150, ADR-0152, ADR-0153, ADR-0154, ADR-0155 + ADR-0125 §(xiii).
