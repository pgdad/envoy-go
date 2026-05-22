# Fixture 0031 — http-admission-control-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.http.admission_control` HTTP filter (phase 23).
Asserts that BOTH reference Envoy v1.37.2 AND envoy-go fail-close at
config-load when `sr_threshold.default_value < 1.0%`.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `"cannot be less than 1.0%"` (case-sensitive substring).

## Boot-reject trigger

`sr_threshold.default_value.value = 0.5` (0.5%, which is < the 1.0%
minimum per SPEC §5.1 arm 2).

## Stderr comparison

| Side | Stderr excerpt |
|---|---|
| reference Envoy v1.37.2 | `"Success rate threshold cannot be less than 1.0%."` (config.cc:25-27 per AMEND-8) |
| envoy-go | `"admission_control: sr_threshold cannot be less than 1.0%"` (parseRejectSrThresholdTooLow constant) |

**Common substring:** `"cannot be less than 1.0%"` (present in both)

## Bootstrap discipline

Self-contained inline bootstrap (Option B2 per fixture-0026 + 0029
precedent). The `sr_threshold.default_value` is embedded directly in
the bootstrap rendered by `renderBootRejectBootstrap()` (driver.go).
No host-mount or file reference is needed.

A minimal `c_unused` cluster (`127.0.0.1:1` — never dialed) is
declared so envoy-go's cluster manager (which runs before the listener
manager) does not fail with a zero-endpoint error before the
admission_control PARSE-REJECT fires. Same ordering sidestep as
fixture-0026 + 0029.

## This fixture vs fixture-0030

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch), the cross-side and boot-reject
surfaces are SEPARATE directories:
- **0030** — cross-side (RequiresReference=true; CompareBytes on all-admit leg)
- **0031** — boot-reject (BootRejectFixture; both sides fail to boot)

## Cross-references

- SPEC §7.2 (boot-reject fixture scope)
- SPEC §5.1 row 2 (sr_threshold < 1.0% reject; byte-stable wording)
- AMEND-8 (boot-reject roster empirical derivation; config.cc:25-27)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- Phase-23 fixture 0030 (sibling cross-side fixture)
- fixture-0029 (nearest BootRejectFixture precedent)
- fixture-0026 (original BootRejectFixture precedent)
