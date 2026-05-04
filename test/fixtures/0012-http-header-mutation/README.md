# Fixture 0012 — `envoy.filters.http.header_mutation`

Phase 10 differential fixture exercising envoy-go's `envoy.filters.http.header_mutation`
filter against reference Envoy v1.37.2 across four scenarios per SPEC §7.1:

1. **Listener-only mutations** (`/listener-only/anything` → `l_lws:10012`): all
   4 AppendActions + Remove + `keep_empty_value` boundary on the listener-level
   filter config; no per-route config on the matched route.
2. **Per-route override + listener interaction** (`/route-override/anything` → `l_lws:10012`):
   listener applied first, then Route tier adds an additional mutation.
3. **Multi-tier evaluation, flag=false, least-specific wins** (`/multi-tier/anything` → `l_lws:10012`):
   per-route configs at all 3 tiers (RC + VirtualHost + Route) all OVERWRITE the
   same header `x-test`; least-specific (RouteConfiguration) wins per SPEC §11.5.
4. **Multi-tier evaluation, flag=true, most-specific wins** (`/multi-tier/anything` → `l_mws:10013`):
   SAME per-route configs as scenario 3, but listener-level
   `most_specific_header_mutations_wins=true`; most-specific (Route) wins.

## Bootstrap shape

Each proxy boots TWO listeners with IDENTICAL per-route tier configurations;
only the listener-level `most_specific_header_mutations_wins` flag differs:

- `l_lws` (LWS = least-specific wins; flag=false): port 10012 (ref) / 10011 (subj)
- `l_mws` (MWS = most-specific wins; flag=true): port 10013 (ref) / 10012 (subj)

The dual-listener pattern is the project's preferred shape for testing
flag-controlled cross-tier ordering (TWO listeners with identical per-route tiers
and the flag as the distinguishing variable).

## Backend

Single Go HTTP backend (`backends/backend.go`) on port 18012 (ref-side) /
runner-allocated (subj-side). Reflects received request headers into the
response body (sorted for determinism); emits `X-Resp-Test: backend-original`
single-value + `X-Multi: alpha, beta` multi-value response headers for
OVERWRITE / APPEND multi-value testing per SPEC §11.4.

## What this fixture does NOT test

- **Stats:** header_mutation emits ZERO stats per SPEC §11.3; the driver does
  NOT scrape stats endpoints or assert stat deltas.
- **Timing:** synchronous filter; no time-bounded assertions.
- **Protected-header rejection:** CONFIG-LOAD-TIME per ADR-0111 — covered by
  unit tests at `internal/filter/http/header_mutation/header_mutation_test.go`,
  NOT by differential fixture (a config attempting protected-header mutation
  would refuse to boot — both reference + subject would refuse).
- **H2 differential:** fixture is HTTP/1.1-only per SPEC §2.3.
- **Cross-filter interaction** (header_mutation × cors / × fault): fixture is
  header_mutation + router only.
- **`mutations.query_parameter_mutations`** (deferred per ADR-0112).
- **Header-value formatter substitution syntax** (deferred per ADR-0113).

## Planner-time decision cross-references

- Fixture path corrected to `test/fixtures/0012-http-header-mutation/` (NOT
  `test/differential/0012-http-header-mutation/` per SPEC §4.3 erratum) per
  PLAN planner-time decision 10.
- New BackendKind enum value `HTTPHeaderMutation BackendKind = 9` per PLAN
  planner-time decision 11.

## Cross-references

- SPEC: `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` §7
- BEHAVIOR_CONTRACT: `docs/envoy-go/BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation`
- ADRs: ADR-0108 (package), ADR-0109 (parser + apply-loop), ADR-0110 (multi-tier framework), ADR-0111 (protected headers)
