# Fixture 0042 — network-direct-response-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.direct_response` network filter (phase 26.1).
Asserts that BOTH reference Envoy v1.37.2 AND envoy-go fail-close at
config-load when the filter's `response` message is present but its
DataSource `specifier` oneof is UNSET (`response: {}`), per §6.1 +
D-P26.1-4.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain `"specifier"`
   (case-sensitive substring).

## Boot-reject trigger (reject arm)

`envoy.filters.network.direct_response` filter config with `response: {}`
— the `Config.response` DataSource message is PRESENT but its `specifier`
oneof (`inline_string` / `inline_bytes` / `filename` /
`environment_variable`) is UNSET.

This `response: {}` arm was chosen empirically over the alternative
`response` ABSENT arm because the absent arm does **not** reject on the
reference: upstream Envoy treats `Config.response` itself as optional and
boots successfully when the field is absent, validating the `specifier`
oneof only once a `response` message is present. envoy-go rejects both
arms identically (its `resolveDataSource` catches a nil DataSource and a
nil specifier with the same const), but only `response: {}` produces a
SYMMETRIC cross-side reject — the discipline this fixture requires.

## Stderr comparison (D-P26.1-4)

Captured empirically at Task 16 against the dockerized reference
`envoyproxy/envoy:v1.37.2` (the authoritative upstream binary the runner
launches via `ensureDocker`).

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `"Proto constraint validation failed (ConfigValidationError.Response: embedded message failed validation | caused by field: \"specifier\", reason: is required)"` (PGV `(validate.required) = true` on the DataSource `specifier` oneof) |
| envoy-go | `"listener manager: listener: \"l_dr\": filter_chains[0]: filters[0]: direct_response: response.specifier is required"` (`parseRejectResponseSpecifierRequired` constant per §6.1, Task-9 byte-stable) |

**Common substring:** `"specifier"` — the 9-character DataSource oneof
field name. Both implementations independently surface this exact token,
case-identical: upstream quotes the proto oneof field name
(`field: "specifier"`), and envoy-go names the same field in its
byte-stable const (`response.specifier is required`). Because the runner's
substring assertion is case-sensitive (`strings.Contains`), `specifier`
is a valid shared needle. (The fragment `is required` is ALSO shared
between the two wordings, but `specifier` is the more distinctive
load-bearing token — the oneof field name at the heart of the reject.)

## Bootstrap discipline

Self-contained inline bootstrap (Option B per fixture-0033 precedent).
The unset `response: {}` is embedded directly in the bootstrap rendered
by `renderBootRejectBootstrap()` (driver.go). No host-mount or file
reference is needed.

A minimal `c_unused` cluster (`127.0.0.1:1` — never dialed) is declared
so envoy-go's cluster manager (which runs before the listener manager)
does not fail with a zero-cluster error before the filter PARSE-REJECT
fires. Same ordering sidestep as fixture-0033 / 0041.

## This fixture vs fixture-0041

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch), the cross-side and boot-reject
surfaces are SEPARATE directories:
- **0041** — cross-side (direct_response with a valid `inline_string`
  response; CompareBytes on the static body)
- **0042** — boot-reject (BootRejectFixture; both sides fail to boot on
  the unset `response: {}` specifier)

## Cross-references

- parent SPEC §8.3 (boot-reject network fixture scope)
- parent SPEC §6.1 (direct_response specifier-required PARSE-REJECT)
- 26.1 PLAN Task 16 + D-P26.1-4 (boot-reject common stderr substring)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- fixture-0041 (sibling cross-side direct_response fixture)
- fixture-0033 (nearest BootRejectFixture precedent; http ratelimit)
