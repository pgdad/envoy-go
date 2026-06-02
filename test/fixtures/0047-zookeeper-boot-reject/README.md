# Fixture 0047 — zookeeper-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.zookeeper_proxy` L4 network filter (phase 28.1b).
Asserts that BOTH reference Envoy v1.37.2 AND envoy-go fail-close at
config-load when the filter's PGV-required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the
`tryStart*` variants, asserts BOTH return a non-nil err (boot rejection
on both sides), and asserts BOTH stderr buffers contain the
case-sensitive substring `stat_prefix`.

## Boot-reject trigger

An `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`
config with `stat_prefix` OMITTED. The proto marks `stat_prefix`
PGV-required (`(validate.rules).string.min_len = 1`), so BOTH the C++
reference Envoy and envoy-go reject it at config-load — a genuine
PGV-mirror cross-side both-reject (SPEC §5.2 lists the
missing-`stat_prefix` case as boot-reject parity for zookeeper_proxy).

This fixture is the BOOT-REJECT branch of the fixture-dispatch pair:
per the project's fixture-dispatch constraint (one fixture dir = one
runner branch), this directory covers the symmetric boot-reject arm
only. The cross-side request arms live in
`0046-zookeeper-requests`.

## Relationship to 0044-network-rbac-boot-reject

This fixture mirrors `0044-network-rbac-boot-reject` exactly, replacing
the `rbac_network` filter with `zookeeper_proxy`. The
`BootRejectFixture` discipline, bootstrap structure, `c_unused` cluster
ordering sidestep, and `stat_prefix` substring choice are all identical
to the 0044 precedent.

## Substring discipline (honest asymmetry)

The two implementations surface the SAME violation with DIFFERENT
wordings (captured live, dockerized v1.37.2 at Task 8):

- reference Envoy (PascalCase field name):
  `Proto constraint validation failed (ZooKeeperProxyValidationError.StatPrefix: value length must be at least 1 characters)`
- envoy-go (snake_case field name):
  `zookeeper_proxy: stat_prefix is required`

The two ERROR wordings share NO distinctive case-sensitive token (ref
uses PascalCase `StatPrefix`; envoy-go uses snake_case `stat_prefix` —
exactly the same asymmetry as 0044). The substring assertion uses
`stat_prefix`:

- **subject side** (the side under test): `stat_prefix` is the envoy-go
  error wording itself; the subject stderr is JUST the error line (no
  YAML echo), so the match is fully load-bearing.
  No 0047-specific deliberate-break was run; the 0044 deliberate-break
  (its Task 14: PascalCase `StatPrefix` fails the subject-side match) is
  the cross-fixture proof that the subject-side substring mechanism is
  live, and the R4 deliberate-break run for `0046` (PROGRESS.md Task 7)
  proves the zookeeper stat surface's assertion liveness.
- **reference side**: reference Envoy echoes the offending bootstrap into
  its stderr, so `stat_prefix` (the tcp_proxy filter's required field in
  the rejected config) appears there; the genuine reference-reject
  assertion is the runner's separate `refErr != nil` gate (the runner
  FATALS if the reference boots cleanly), which the live run confirms
  fires on the `ZooKeeperProxyValidationError.StatPrefix` PGV violation.

Both sides rejecting is the cross-side parity claim; the per-side
substring pins the envoy-go wording.

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042/0044.

## Numeric PGV-mirror arms' disposition

The numeric PGV-mirror validation arms (e.g. `max_packet_bytes` bounds,
`default_latency_threshold` / `latency_threshold_overrides`) are
unit-test-only at phase 28.1. Their fixture disposition is deferred to
the 28.2 SPEC.
