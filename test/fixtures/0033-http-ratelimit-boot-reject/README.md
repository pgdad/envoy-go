# Fixture 0033 — http-ratelimit-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.http.ratelimit` HTTP filter (phase 24.1).
Asserts that BOTH reference Envoy v1.37.2 AND envoy-go fail-close at
config-load when the filter's `domain` field is empty (§5.1 arm 1).

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `"omain"` (case-sensitive substring).

## Boot-reject trigger

`envoy.filters.http.ratelimit` filter config with the `domain` field
omitted (empty string default per proto3), per §5.1 arm 1.

## Stderr comparison (D-RL4)

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `"Proto constraint validation failed (RateLimitValidationError.Domain: value length must be at least 1 characters)"` (PGV `validate.rules.string.min_len = 1` on the `domain` field) |
| envoy-go | `"listener manager: listener: 'l_test_a': filter_chains[0]: hcm: http_filters[0]: factory: ratelimit: domain is required"` (parseRejectDomainRequired constant per §5.1 row 1) |

**Common substring:** `"omain"` — a 5-character fragment of the field
name `domain`. Because the runner's substring assertion is
case-sensitive (`strings.Contains`), and upstream emits the proto
camel-case form `Domain` (capital D, from the Go-protoc PGV validation
error type) while envoy-go emits the wire-name lowercase form
`domain`, neither full-case spelling is shared. The fragment `omain`
is present in both `Domain` and `domain`, is distinctive enough to
not collide with unrelated tokens, and is the canonical D-RL4
substring finalized empirically at Task 11.

## Bootstrap discipline

Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031
precedent). The empty `domain` is embedded directly in the bootstrap
rendered by `renderBootRejectBootstrap()` (driver.go). No host-mount
or file reference is needed.

A minimal `c_unused` cluster (`127.0.0.1:1` — never dialed) is
declared so envoy-go's cluster manager (which runs before the listener
manager) does not fail with a zero-endpoint error before the filter
PARSE-REJECT fires. Same ordering sidestep as fixture-0026 / 0029 /
0031.

A minimal-but-syntactically-valid `c_ratelimit` cluster
(`127.0.0.1:1` with the mandatory `http2_protocol_options:{}`) is
declared and referenced from the filter's
`rate_limit_service.envoy_grpc.cluster_name`. This ensures the PGV /
envoy-go parse path proceeds PAST the rate_limit_service-shape arms;
the boot-reject is therefore unambiguously attributable to the empty
`domain` field. The cluster is never dialed because the boot-reject
fires at config-load, strictly before any listener binds.

## This fixture vs fixture-0032

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch), the cross-side and boot-reject
surfaces are SEPARATE directories:
- **0032** — cross-side (RequiresReference=true; CompareBytes on
  scenarios a/b/c/d-core/e + StatsAsserter on h)
- **0033** — boot-reject (BootRejectFixture; both sides fail to boot
  on empty `domain`)

## Cross-references

- parent SPEC §7.2 (boot-reject fixture scope)
- parent SPEC §5.1 row 1 (domain empty PARSE-REJECT; byte-stable wording)
- 24.1 PLAN Task 11 + D-RL4 (boot-reject common stderr substring)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- Phase 24.1 fixture 0032 (sibling cross-side fixture)
- fixture-0031 (nearest BootRejectFixture precedent; admission_control)
- fixture-0029 (lua source_codes BootRejectFixture precedent)
- fixture-0026 (original lua BootRejectFixture precedent)
