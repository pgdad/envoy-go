# Fixture 0018 — `envoy.filters.http.rbac` differential equivalence

Eight scenarios per phase 16 SPEC §7.1; sequential against three listeners
(`l_test_a` plaintext HCM `hcm_local_a` + `l_test_b` echobackend HCM +
`l_test_a_tls` mTLS-required HCM `hcm_local_a_tls`) with six routes on
`l_test_a` (`/`, `/public`, `/api`, `/protected`, `/per-route-disabled`,
`/per-route-override`) + one route on `l_test_a_tls` (`/admin`). Listener
`l_test_b` + cluster `c_backend_b` provide the echobackend pair (reuses
`test/helpers/echobackend/` from phase 14) for scenario 5's upstream-routed
`/protected` request. Reference Envoy v1.37.2 (STRICT_DNS + Docker bind-
mounted PKI) vs envoy-go (STATIC, in-process PKI).

Listener-level config (verbatim per SPEC §7.2):

```yaml
envoy.filters.http.rbac:
  rules_stat_prefix: default
  shadow_rules_stat_prefix: default
  rules:
    action: ALLOW
    policies:
      admin_users:           # header X-User=admin
      public_paths:           # url_path /public + any principal
      tenant_api_users:       # AND[url_path /api, header X-Tenant=acme]
      internal_or_protected:  # OR[url_path prefix /protected, header X-Internal=true]
      authenticated_admin:    # url_path /admin + Principal_Authenticated (l_test_a_tls only)
```

The scenario-8 per-route TPFC override carries
`rules_stat_prefix: override` + `shadow_rules_stat_prefix: override_shadow`
(both with a DENY action on `header X-User=guest`). The scenario-7 per-route
TPFC carries `RBACPerRoute{}` (rbac field absent) — the 7th canonical
absence-implies-disabled semantic per ADR-0125 §(xii).

## Scenarios

1. **Allow-by-header-match** — `GET /` with `X-User: admin` → 200 + 32-byte
   direct_response body `fixture-0018-direct-response-OK\n`. Policy
   `admin_users` matches (header-exact admin). ALLOW; counter
   `default.allowed` +1 (hcm_local_a).
2. **Deny-no-match** — `GET /` with `X-User: guest` → 403 + 19-byte
   `RBAC: access denied`. No policy matches; ALLOW + no-match → DENY.
   Counter `default.denied` +1 (hcm_local_a).
3. **Allow-by-url-path** — `GET /public` → 200 + 32-byte direct_response.
   Policy `public_paths` matches (url_path exact /public + any principal).
   ALLOW; `default.allowed` +1.
4. **Allow-by-AND-composite** — `GET /api` with `X-Tenant: acme` → 200 +
   32-byte direct_response. Policy `tenant_api_users` matches (AND[url_path
   exact /api, header X-Tenant exact acme]). ALLOW; `default.allowed` +1.
   *Task-14 fixture redesign: the BRAINSTORM scenario 4 used a
   destination_port permission, but envoy-go MVP stubs `DestinationPort()`
   to 0; the redesigned scenario exercises the same `Permission_AndRules`
   canonical evaluator using MVP-plumbed accessors. The destination_port
   canonical remains covered by unit tests at Group 3.*
5. **Allow-by-OR-composite** — `GET /protected` → 200 + echobackend JSON
   echo (per-side variable-length body per phase-14 ADR-0133 §(ii); driver
   asserts non-empty body, not byte-exact). Policy `internal_or_protected`
   matches (OR[url_path prefix /protected, header X-Internal=true] — first
   clause matches). ALLOW; routes through cluster `c_backend_b` to
   echobackend; `default.allowed` +1.
   *Task-14 fixture redesign: the BRAINSTORM scenario 5 used a
   direct_remote_ip principal, but envoy-go MVP stubs `DirectRemoteIP()`
   to nil; the redesigned scenario exercises `Permission_OrRules` using
   MVP-plumbed accessors. The direct_remote_ip canonical remains covered
   by unit tests at Group 4.*
6. **mTLS Allow-by-TLS-principal** — `GET /admin` over HTTP/1.1-over-mTLS
   to `l_test_a_tls`; client presents cert with URI SAN
   `spiffe://example.com/admin` signed by fixture-CA. Policy
   `authenticated_admin` matches (url_path exact /admin + Principal_Authenticated.
   principal_name StringMatcher exact matches the URI SAN per the
   priority-ordered candidate slice from ADR-0144's `DownstreamPrincipal()`
   accessor). 200 + 32-byte direct_response. ALLOW; `default.allowed`
   +1 on hcm_local_a_tls (NOT hcm_local_a).
7. **Per-route 7th-canonical disabled** — `GET /per-route-disabled` with
   `X-User: guest` (which WOULD deny at the listener level). Per-route
   TPFC carries `RBACPerRoute{}` (rbac field absent) → 7th canonical
   absence-implies-disabled → filter wholly bypassed for this route per
   ADR-0125 §(xii) case (a). 200 + 32-byte direct_response. NO counter
   increment anywhere (the per-route disable IS honored on BOTH sides per
   Task-14 empirical scrape).
8. **Per-route wholesale-override with INDEPENDENT-vs-SHARED stats
   divergence** — `GET /per-route-override` with `X-User: guest`. Per-route
   TPFC carries `RBACPerRoute{rbac: <override config DENY guests
   rules_stat_prefix:override shadow_rules_stat_prefix:override_shadow>}`.
   The override DENY policy matches → 403 + 19-byte `RBAC: access denied`.
   Counter impact DIVERGES BY SIDE per the INDEPENDENT-vs-SHARED stats
   discipline (documented divergence-window — see below):

   - **envoy-go (INDEPENDENT per ADR-0145):** `override.denied` +1 +
     `override_shadow.shadow_denied` +1. The listener-level `default.*`
     namespace is UNCHANGED.
   - **reference Envoy v1.37.2 (SHARED per Task-14 empirical):**
     `default.denied` +1 (stacks on top of scenario 2's +1) +
     `default.shadow_denied` +1. The per-route
     `rules_stat_prefix:override` / `shadow_rules_stat_prefix:override_shadow`
     are IGNORED.

   The driver's `AssertStats` enforces side-specific expectations AND a
   cross-side TOTAL-event-count equivalence assertion: the SUM of
   (`default.denied` + `override.denied`) matches between sides
   (both record the same number of DENY events; only the prefix-binding
   differs).

(Total cross-scenario observed counts — per-side; see expectations.yaml
for the full Counter-delta map and the 9-window divergence roster.)

## mTLS PKI notes (fresh-gen via `pki/` package init())

The fixture's mTLS PKI (fixture-CA + server cert/key + client cert/key) is
generated FRESH at fixture-load time by the `pki` package's `init()` (per
planner-time decision 11 — NOT pre-baked / committed). The driver's
`inputs/driver.go` carries a blank-import
`_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki"` that
triggers the init() chain BEFORE `ReferenceBootstrap` / `SubjectConfig` /
`runTLSScenario6` reference the cert paths. Go's package-init topology
guarantees ordering. See `pki/gen.go` for the full PKI hierarchy +
issuance rationale.

- **Fixture CA**: ecdsa.P256 self-signed; CN `fixture-0018-rbac-ca`; 24h
  validity.
- **Server cert**: ecdsa.P256 signed by CA; DNSNames=`[l_test_a_tls.fixture.test]`;
  CN `fixture-0018-rbac-server`; ExtKeyUsage=ServerAuth. The DNS SAN
  matches the SNI the driver presents in `runTLSScenario6`'s `tls.Config{ServerName: "l_test_a_tls.fixture.test"}`.
- **Client cert**: ecdsa.P256 signed by CA; URIs=`[spiffe://example.com/admin]`;
  CN `client.fixture.test`; ExtKeyUsage=ClientAuth. The URI SAN matches
  the listener-level `authenticated_admin` policy's
  `Principal_Authenticated.principal_name.exact:
  "spiffe://example.com/admin"`.

**Reference Envoy container PKI bind-mount mechanism** (Task-14 carry-
forward from Task 13): the reference Envoy runs inside a Docker container
+ cannot read the host-fixture's `pki/*.pem` files directly. The driver
implements `fixture.ReferenceLogMounter`'s `ReferenceHostMounts()` returning
three bind-mount descriptors (ca.pem → /pki/ca.pem, server.pem →
/pki/server.pem, server.key.pem → /pki/server.key.pem). The runner
pre-creates each host file with `O_CREATE|O_WRONLY` (no truncation; the
pki init() output survives) + chmods to 0o666 before passing to
`StartReferenceProxyWithMounts`. The reference Envoy YAML reads the
in-container `/pki/*` paths; the envoy-go YAML reads the host-side
`<fixtureDir>/pki/*.pem` paths directly.

## 7th canonical per-route discipline notes (ADR-0125 §(xii))

Phase 16 is the FIRST §9 row to use the **absent-implies-disabled-OR-
wholesale-override** per-route discipline (the 7th canonical pattern per
ADR-0125 §(xii) in-place amendment). Distinct from BOTH the 5th canonical
(`disabled` bool in oneof; phase-13 buffer + phase-14 compressor) AND the
6th canonical (bare-message-via-TPFC + code-level-required-field;
phase-15 bandwidth_limit). The 7th canonical's defining feature:
`RBACPerRoute` carries a SINGLE optional `rbac` sub-message (field 2;
field 1 reserved); absence-of-the-sub-message-field implies
disabled-via-proto-comment; presence-of-the-sub-message-field implies
wholesale-override of the listener-level config.

Fixture 0018 exercises both cases in adjacent scenarios:
- **Scenario 7** (case (a) absent → disabled): `RBACPerRoute{rbac: nil}`
  → filter wholly bypassed; NO counter emission.
- **Scenario 8** (case (b) present → wholesale-override): `RBACPerRoute{rbac:
  <override>}` → listener-level config IGNORED for this route; the
  override config evaluates standalone with its own rules_stat_prefix /
  shadow_rules_stat_prefix.

## INDEPENDENT-stats per-route discipline notes (ADR-0145)

Phase 16 inherits the INDEPENDENT-stats per-route discipline introduced at
ADR-0117 (phase-11 local_ratelimit) + extended at ADR-0139 (phase-15
bandwidth_limit). The per-route override's `rules_stat_prefix` +
`shadow_rules_stat_prefix` create FRESH stat namespaces disjoint from the
listener-level prefixes. envoy-go enforces INDEPENDENT-stats verbatim per
ADR-0145; reference Envoy v1.37.2's behavior is SHARED-stats (the per-route
stat_prefix settings are ignored; per-route counters fold into the
listener-level namespace). This is the **INDEPENDENT-vs-SHARED-per-route-
stats divergence-window** documented at expectations.yaml + accommodated
by the driver's side-specific assertion table + cross-side total-event-
count equivalence assertion.

Future stat-surface-refinement phase MAY introduce an envoy-go SHARED-stats
opt-in (e.g., a `share_stats_with_listener: bool` field on RBACPerRoute);
deferred per ADR-0145 §Future-work.

## Counter-delta assertion discipline notes

The driver's `AssertStats` scrapes `/stats/prometheus` from both admin
endpoints + builds a per-side stats map keyed by the full Prometheus line
(name + labels). `lookupRBACCounter` normalizes across TWO observed
Prometheus naming conventions (Task-14 empirical):

- **Form A (reference Envoy label form):** base
  `envoy_http_rbac_<suffix>{envoy_rbac_http_prefix="<namespace>",envoy_http_conn_manager_prefix="<HCM>"}`.
  Reference Envoy ships a tag-extractor that promotes
  `rules_stat_prefix` to the `envoy_rbac_http_prefix` label.
- **Form B (envoy-go inline form):** base
  `envoy_http_rbac_<namespace>_<suffix>{envoy_http_conn_manager_prefix="<HCM>"}`.
  envoy-go's MVP uses the SN2-reuse hypothesis from ADR-0145 with NO new
  SN10 flatten rule (or its equivalent for rbac) — the namespace is
  inlined into the base name via SN2's dot→underscore substitution.

`lookupRBACCounter` sums across all HCM-prefix label permutations for the
given (namespace, suffix) pair. Both conventions are handled with absent-
as-zero fallback. The **Prometheus tag-extractor surface divergence**
window is absorbed at the driver level; future enhancement may add a
phase-16-stat-extractor in `internal/stats/name.go` to close it
structurally.

## Divergence-window summary

Fixture 0018 surfaces TWO empirically-observed divergence-windows + carries
SEVEN documented-but-not-surfaced windows. Per expectations.yaml the 9
windows are:

1. **Prometheus tag-extractor surface divergence** (Task-14 empirical;
   driver-normalized).
2. **INDEPENDENT-vs-SHARED per-route stats divergence** (ADR-0145 +
   Task-14 empirical; side-specific assertion with cross-side total-
   equivalence check).
3. LOG-action access_log_hint dynamic-metadata divergence (not surfaced;
   no LOG-action scenarios in fixture 0018).
4. response_code_details on DENY divergence (not surfaced; driver does
   not assert).
5. CEL three-field condition evaluation divergence (not surfaced;
   policies do not use the CEL condition field).
6. Shadow access-log integration divergence (not surfaced; no current
   observable divergence at fixture 0018 wallclock).
7. sourced_metadata + filter_state Principal always-no-match (not
   surfaced; not exercised).
8. Principal_Authenticated canonical 3-cert-field scope (not surfaced;
   client cert URI SAN is the canonical accessor).
9. **mTLS require_client_certificate via ADR-0147 unanticipated lift**
   (Task-14 unanticipated; phase-03 blanket rejection lifted SCOPED to
   well-formed mTLS configs).

## Envoy deviation

Two unanticipated envoy-go changes landed at Task 14:

- **ADR-0147 (Task-14 unanticipated):** phase-03 TLS layer's blanket
  rejection of `require_client_certificate=true` is lifted SCOPED to
  well-formed mTLS configurations (validation_context.trusted_ca PEM
  provided). The lift maps Envoy's `require_client_certificate=true` onto
  stdlib `crypto/tls.RequireAndVerifyClientCert` mode + `ClientCAs` pool
  populated from the fixture-CA. Previously parse-rejected surfaces
  (SDS-bound secrets, custom_validator_config, match_typed_san,
  verify_certificate_hash/spki) remain rejected.
- **HCM `:path` pseudo-header injection (Task-14 framework delta):** the
  HCM decode-side dispatch now injects `:path` onto `req.Header` symmetric
  with the pre-existing `:method` + `:authority` injection (per Task 11/12
  precedent). The rbac filter (and future filters consuming `:path` via
  the `evalContext.URLPath()` accessor) observe the request path
  consistently across H1 + H2 dispatch paths. Per `connection.go` +
  `h2dispatch.go`.

## Future-deferred work

- **DestinationPort / DestinationIP / DirectRemoteIP / RemoteIP /
  RequestedServerName framework primitives:** envoy-go MVP stubs these
  accessors to zero values; the corresponding Permission/Principal
  canonicals (destination_port, destination_ip, direct_remote_ip,
  remote_ip, requested_server_name) evaluate to FALSE on envoy-go at
  fixture 0018 wallclock. The canonicals are covered by unit tests at
  Group 3 + Group 4 (stub-evalContext pre-populated). Future framework-
  primitive phase plumbs the accessors through from connection state.
- **CEL three-field condition + check_condition:** envoy-go silent-
  ignored at parse; full CEL evaluator deferred to a future CEL-engine
  phase.
- **sourced_metadata + filter_state Principal:** always-FALSE on envoy-go;
  full evaluation deferred to a future dynamic-metadata + filter-state
  family phase.
- **Principal_Authenticated extended cert fields (Issuer DN, Serial,
  fingerprints):** deferred per ADR-0144 §Decision canonical-3-field
  scope.
- **Per-route SHARED-stats opt-in:** future stat-surface-refinement may
  add `share_stats_with_listener: bool` on RBACPerRoute to close the
  INDEPENDENT-vs-SHARED divergence-window structurally.

## Planner-time decisions cross-references

Phase 16 PLAN settles 14 decisions (per BRAINSTORM §6 + §7 + §9; refined
per SPEC §1.1 12 amendments). This fixture exercises:

- **D1 (4-counter base surface)** per ADR-0140 + amendments 5 + 8 + 9.
- **D2 (dual-engine dispatch: rules-engine + matcher-engine)** per
  ADR-0141. Fixture uses rules-engine on both sides (matcher-engine
  unit-tested at Group 5).
- **D3 (Permission + Principal Large 11 evaluator surface)** per ADR-0143;
  scenarios exercise Permission_AndRules + Permission_OrRules +
  Permission_UrlPath + Permission_Header + Principal_Any +
  Principal_Authenticated canonicals.
- **D4 (TLS-principal accessor framework primitive)** per ADR-0144;
  scenario 6's mTLS path exercises the `DownstreamPrincipal()` accessor.
- **D5 (stat surface finalization + per-route INDEPENDENT-stats)** per
  ADR-0145; scenario 8 exercises the INDEPENDENT-stats discipline (with
  the documented divergence-window vs reference's SHARED-stats).
- **D6 (shadow + LOG-partial + track_per_rule_stats per-policy emission)**
  per ADR-0146; scenario 8's shadow_rules emit the shadow_denied counter
  on both sides (with the same divergence-window).
- **D7 (7th canonical absent-implies-disabled-OR-wholesale-override)**
  per ADR-0125 §(xii); scenarios 7 + 8 exercise both cases.
- **D8 (echobackend shared helper)** per ADR-0133 §(ii); scenario 5
  routes through the shared echobackend.
- **D9 (mTLS PKI fresh-gen via package init)** per planner-time decision
  11; pki/gen.go's init() generates the 5 PEMs at fixture-load time.
- **ADR-0147 (Task-14 unanticipated; mTLS-lift):** phase-03 TLS layer
  blanket-rejection of `require_client_certificate=true` lifted SCOPED
  to well-formed mTLS configs.
