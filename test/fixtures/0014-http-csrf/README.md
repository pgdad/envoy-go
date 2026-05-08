# Fixture 0014 — `envoy.filters.http.csrf` differential equivalence

Six scenarios per phase 12 SPEC §7.1; sequential against a single listener `l_main`
with two routes (`/` default + `/route-only` per-route TPFC). Reference Envoy
v1.37.2 (STRICT_DNS) vs envoy-go (STATIC).

## Scenarios

1. **Same-origin POST allowed** — `POST / Origin: http://127.0.0.1:<port>` → 200; `request_valid +1`.
2. **Cross-origin POST rejected** — `POST / Origin: https://evil.test` → 403 + `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length`, `content-type`, `date`, `server: envoy`); `request_invalid +1`.
3. **additional_origins exact match** — `POST / Origin: https://app.example.test` → 200; `request_valid +1`. (Entry is `app.example.test` host:port form per §11.8 amendment.)
4. **No source-origin rejected** — `POST /` (no Origin, no Referer) → 403; `missing_source_origin +1`.
5. **Referer fallback** — `POST / Referer: http://127.0.0.1:<port>/somepage` (no Origin) → 200; `request_valid +1`.
7. **Per-route wholesale-override** — (a) `POST /route-only Origin: https://route-only.test` → 200; (b) `POST / Origin: https://route-only.test` → 403. Counter increments AGGREGATE with listener-level series (per §11.9 amendment — diverges from phase 11 ADR-0117 precedent which had INDEPENDENT per-route stats).

(Scenario 6 — GET passthrough — is unit-only per SPEC §2.4 + §14.1 group 3; not in the differential fixture.)

## Single-listener bootstrap discipline (per planner-time decision 7)

All scenarios run against the same listener (single boot; no per-scenario teardown). Driver issues all 7 requests sequentially in one `DriveReference` / `DriveSubject` call. csrf is purely synchronous — no timing tolerances.

## `filter_enabled` PGV discipline (per §11.11 amendment)

Both `envoy.yaml` and `envoy-go.yaml` set `filter_enabled.default_value: {numerator: 100, denominator: HUNDRED}` explicitly on both the listener-level and per-route CsrfPolicy entries. Reference Envoy v1.37.2 PGV-rejects boot if `filter_enabled` is absent OR if `filter_enabled.default_value` is absent — non-negotiable. envoy-go's `New` factory PGV-mirrors per ADR-0121 (validates non-nil presence; the percentage value is silent-ignored at runtime per §1.1 amendment 3).

`shadow_enabled` is OMITTED on both sides per §11.11 probe #3 baseline (Envoy permits omission; envoy-go also accepts; runtime is always-never-shadow on both).

## Operator footgun (per §11.8 amendment)

`additional_origins[].exact` matches the source's `host[:port]` form — NOT the full URL with scheme. Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header. Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port). envoy-go faithfully replicates Envoy's behavior; this is a known footgun in the upstream spec.

## Per-route stats SHARED with listener-level (per §11.9 amendment)

csrf is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern. Phase 11's local_ratelimit precedent (ADR-0117) had INDEPENDENT per-route stats; phase 12 is the inverse pattern — per-route data REPLACES listener data, but counter increments AGGREGATE under the SAME `*filterStats` (one counter series per HCM scope). ADR-0124 captures this.

## Envoy deviation

None — csrf is a normal HTTP filter; no SIGTERM/drain divergence. Per-route TPFC handling is the existing 3-tier `Resolve` per ADR-0073 (most-specific-override).

## Planner-time decisions cross-references

- Decision 5: per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper at request time; SHARES listener-level `*filterStats` pointer.
- Decision 7: single-listener topology fits existing `fixture.Driver` contract.
- Decision 8: synthetic `http://` prefix for target-URL parsing (no framework extension).
