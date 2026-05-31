# Fixture 0044 — network-rbac-boot-reject

BOOT-REJECT differential fixture for the `envoy.filters.network.rbac`
L4 network filter (phase 26.3). Asserts that BOTH reference Envoy
v1.37.2 AND envoy-go fail-close at config-load when the filter's
PGV-required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the
`tryStart*` variants, asserts BOTH return a non-nil err (boot rejection
on both sides), and asserts BOTH stderr buffers contain the
case-sensitive substring `stat_prefix`.

## Boot-reject trigger

An `envoy.extensions.filters.network.rbac.v3.RBAC` config with
`stat_prefix` OMITTED. The proto marks `stat_prefix` PGV-required
(`(validate.rules).string.min_len = 1`, `rbac.pb.validate.go:178`), so
BOTH the C++ reference Envoy and envoy-go reject it at config-load — a
genuine PGV-mirror cross-side both-reject (SPEC §10 lists
`rbac-network-stat-prefix-required` as boot-reject parity).

This is distinct from the envoy-go-strict-only rejects (HTTP-only matcher
arm / `delay_deny` / invalid stat_prefix) which upstream silently ACCEPTS
(the unknown extension field is dropped by upstream's protobuf parser)
and which are therefore subject-side-only rejects covered by the
Task-8/Task-13 unit tests, NOT cross-side fixtures (per the
fixture-dispatch-constraint: one fixture dir = one runner branch).

## Substring discipline (honest asymmetry)

The two implementations surface the SAME violation with DIFFERENT
wordings (captured live, dockerized v1.37.2 at Task 14):

- reference Envoy (PascalCase field name):
  `Proto constraint validation failed (RBACValidationError.StatPrefix:
  value length must be at least 1 characters)`
- envoy-go (snake_case field name):
  `rbac_network: stat_prefix is required`

The two ERROR wordings share NO distinctive case-sensitive token (ref
uses PascalCase `StatPrefix`; envoy-go uses snake_case `stat_prefix`;
the longest common case-sensitive substring of the error lines is the
non-distinctive 5-char `refix`). The substring assertion uses
`stat_prefix`:

- **subject side** (the side under test): `stat_prefix` is the envoy-go
  error wording itself; the subject stderr is JUST the 126-byte error
  line (no YAML echo), so the match is fully load-bearing — a
  deliberate-break to PascalCase `StatPrefix` FAILS the subject (proven
  at Task 14).
- **reference side**: reference Envoy echoes the offending bootstrap into
  its stderr, so `stat_prefix` (the tcp_proxy filter's required field in
  the rejected config) appears there; the genuine reference-reject
  assertion is the runner's separate `refErr != nil` gate (the runner
  FATALS if the reference boots cleanly), which the live run confirms
  fires on the `RBACValidationError.StatPrefix` PGV violation.

Both sides rejecting is the cross-side parity claim; the per-side
substring pins the envoy-go wording.

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042.
