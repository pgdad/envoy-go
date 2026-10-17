# Fixture 0128 — stats-flush-interval-negative-boot-reject

BOOT-REJECT differential fixture for the bootstrap's top-level
`stats_flush_interval` (phase 102). Asserts that BOTH reference Envoy
v1.37.2 AND envoy-go fail-close at config-load when the interval is
negative (`-0.5s`), with NO `stats_sinks` configured, and that both name
the negative-duration class rather than the range class.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `Invalid duration: Expected positive duration` (case-sensitive
   substring).

## Boot-reject trigger (reject arm)

`stats_flush_interval: -0.5s` at the top level of the bootstrap: seconds
`0`, nanos `-500000000`.

`-0.5s` (negative NANOS) was chosen over `-1s`. The reference refuses a
negative Duration before its range check. A value with seconds `0` also
falls under the range check's lower bound, so an envoy-go that checks the
range first (NC6) or prints one message for every class (NC9) prints the
range phrase and fails this fixture. `-1s` has seconds `-1`: not `0` and
below 300, so it skips the range clause, and an envoy-go that checks the
range first still prints the negative phrase. Measured: with `-1s` this
fixture PASSES under NC6. The bootstrap has no `stats_sinks`, so the claim
is sink-independent.

## Stderr comparison

Measured against the dockerized reference `envoyproxy/envoy:contrib-v1.37.2`,
run by digest (`docs/envoy-go/ENVOY_TARGET.md`).

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `Invalid duration: Expected positive duration: goo.gle/debugstr ... nanos: -500000000` (a `critical` `error ... initializing config` line) |
| envoy-go | `load config: bootstrap: stats_flush_interval: Invalid duration: Expected positive duration` |

**Common substring:** `Invalid duration: Expected positive duration`.
envoy-go mirrors the reference's phrase. The needle stops before the
reference's `: goo.gle/debug...` token and does not contain `bootstrap:`,
which the reference never prints.

## Bootstrap discipline

Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
with an HCM `direct_response` (status `222`, inline body) and a minimal
`c_unused` STATIC cluster (`127.0.0.1:1`, never dialed). The body is
required: envoy-go refuses a `direct_response` without one
(`hcm: route 0: direct_response.body is required`), so a body-less route
would reject the subject for an unrelated reason. With a valid `1s`
interval the same bootstrap validates on the reference
(`--mode validate`: `OK`), and at the pre-phase-102 tip the subject boots
it with `-0.5s`.

## Measured verdicts

Shapes are the phase 102 production change (P2, whose code the PLAN's
Appendix A carries unchanged) and its negative controls
(SPEC §7), each run through this fixture alone with `-count=1`.

| shape | verdict | what the subject did |
|---|---|---|
| tip (no production change) | FAIL | booted (`-0.5s` folds to 5 s) |
| P2 | PASS (3/3 runs) | rejected with the negative phrase |
| NC1 (no lower bound) | PASS | rejected with the negative phrase |
| NC3 (NEG case deleted) | FAIL | rejected with the range phrase |
| NC6 (range checked first) | FAIL | rejected with the range phrase |
| NC7 (block gated on a sink) | FAIL | booted |
| NC9 (one message for all classes) | FAIL | rejected with the range phrase |
| NC2, NC4, NC5, NC8 | PASS | rejected with the negative phrase |

Each run of this fixture (reference container start and exit, then the
subject build and exit) took 1.58-1.95 s across 13 runs.

## This fixture vs fixture-0127

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch, one substring), the range and the
negative-duration classes are SEPARATE directories:
- **0127** — range class (`0s`, `value must be inside range [1ms, 5m0s)`)
- **0128** — negative class (`-0.5s`, `Invalid duration: Expected positive duration`)

## Cross-references

- phase 102 SPEC §6 (two boot-reject directories) and §7 (NC roster)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- fixture-0127 (sibling range boot-reject)
- fixture-0042 (the BootRejectFixture shape this copies)
