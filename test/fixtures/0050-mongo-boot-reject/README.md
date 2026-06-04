# Fixture 0050 — mongo-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.mongo_proxy` L4 network filter (phase 29.1 /
ADR-0224). Asserts that BOTH reference Envoy AND envoy-go fail-close at
config-load when the filter's required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`differential.BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the `tryStart*`
variants, asserts BOTH return a non-nil err (boot rejection on both
sides), and asserts BOTH stderr buffers contain the case-sensitive
substring `stat_prefix`.

## Boot-reject trigger

A `mongo_proxy.v3.MongoProxy` config with `stat_prefix` OMITTED. The
proto marks `stat_prefix` required, so BOTH the C++ reference Envoy
(PGV) and envoy-go (the `errStatPrefixRequired` const in `config.go`)
reject it at config-load — a genuine both-sides-reject (SPEC §8.1).

This fixture is the BOOT-REJECT branch of the fixture-dispatch pair:
per the project's fixture-dispatch constraint (one fixture dir = one
runner branch), this directory covers the symmetric boot-reject arm
only. The cross-side request arms live in `0049-mongo-requests`.

## Relationship to 0047-zookeeper-boot-reject

This fixture mirrors `0047-zookeeper-boot-reject` exactly, replacing the
`zookeeper_proxy` filter with `mongo_proxy`. The `BootRejectFixture`
discipline, bootstrap structure, `c_unused` cluster ordering sidestep,
and `stat_prefix` substring choice are all identical to the 0047/0044
precedent.

## Substring discipline (honest asymmetry)

The two implementations surface the SAME violation with DIFFERENT
wordings:

- reference Envoy: a PGV violation naming the field (and echoing the
  offending bootstrap, in which `stat_prefix` appears on the tcp_proxy
  filter line).
- envoy-go: `mongo_proxy: stat_prefix is required` — the snake_case
  token `stat_prefix` is in the error line itself.

The substring assertion uses `stat_prefix`:

- **subject side** (the side under test): `stat_prefix` is the envoy-go
  error wording itself; the subject stderr is JUST the error line (no
  YAML echo), so the match is fully load-bearing.
- **reference side**: reference Envoy echoes the offending bootstrap
  into its stderr, so `stat_prefix` (the tcp_proxy filter's required
  field in the rejected config) appears there; the genuine
  reference-reject assertion is the runner's separate `refErr != nil`
  gate (the runner FATALS if the reference boots cleanly).

Both sides rejecting is the cross-side parity claim; the per-side
substring pins the envoy-go wording. The R4 deliberate-break liveness
proof for the mongo stat surface is recorded against `0049`
(PROGRESS.md Task 16); no `0050`-specific break is needed (the 0047/0044
boot-reject substring mechanism is already proven live cross-fixture).

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042/0044/0047.

## Cross-references

- phase 29.1 SPEC §8.1 (this fixture's scope)
- 29.1 PLAN Task 16 (this fixture)
- `harness.go` `BootRejectFixture` interface (`runBootRejectFixture` branch)
- fixture `0047-zookeeper-boot-reject` (the symmetric template this mirrors)
- fixture `0049-mongo-requests` (cross-side arms; the one-dir-one-branch
  companion for the MongoProxy filter)
