# Fixture 0054 — kafka-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.kafka_broker` L4 network filter (phase 31 SPEC
§8.2). Asserts that BOTH reference Envoy AND envoy-go fail-close at
config-load when the filter's required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`differential.BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the `tryStart*`
variants, asserts BOTH return a non-nil err (boot rejection on both
sides), and asserts BOTH stderr buffers contain the case-sensitive
substring `stat_prefix`.

## Boot-reject trigger

A `kafka_broker.v3.KafkaBroker` config with `stat_prefix` OMITTED. The
proto marks `stat_prefix` PGV-required (min 1 rune), so BOTH the C++
reference Envoy (PGV) and envoy-go (the `errStatPrefixRequired` const in
`config.go`) reject it at config-load — a genuine both-sides-reject
(SPEC §8.2).

This fixture is the BOOT-REJECT branch of the fixture-dispatch pair:
per the project's fixture-dispatch constraint (one fixture dir = one
runner branch), this directory covers the symmetric boot-reject arm
only. The cross-side request arms live in `0053-kafka-requests`.

## Relationship to 0050-mongo-boot-reject

This fixture mirrors `0050-mongo-boot-reject` exactly, replacing the
`mongo_proxy` filter with `kafka_broker`. The `BootRejectFixture`
discipline, bootstrap structure, `c_unused` cluster ordering sidestep,
and `stat_prefix` substring choice are all identical to the 0050/0047
precedent.

## Other PGV reject arms stay UNIT-TEST-ONLY (D-P3)

The kafka_broker proto carries additional PGV constraints beyond
`stat_prefix` — notably the `api_keys` ID-range validation and the
nested broker-address-rewrite rule. Per phase 31 D-P3, those further
reject arms are covered UNIT-TEST-ONLY by
`internal/filter/network/kafkabroker/config_test.go` (the
`parseConfig` PGV/range arms), NOT as additional differential
boot-reject fixtures. This fixture covers ONLY the `stat_prefix`-missing
symmetric boot-reject.

## Substring discipline (honest asymmetry)

The two implementations surface the SAME violation with DIFFERENT
wordings:

- reference Envoy: a PGV violation naming the field (and echoing the
  offending bootstrap, in which `stat_prefix` appears on the tcp_proxy
  filter line).
- envoy-go: `kafka_broker: stat_prefix is required` — the snake_case
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
proof for the kafka stat surface is recorded against `0053`; no
`0054`-specific break is needed (the 0050/0047 boot-reject substring
mechanism is already proven live cross-fixture).

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042/0044/0047/0050.

## Cross-references

- phase 31 SPEC §8.2 (this fixture's scope)
- 31 PLAN Task 14 (this fixture)
- `harness.go` `BootRejectFixture` interface (`runBootRejectFixture` branch)
- fixture `0050-mongo-boot-reject` (the symmetric template this mirrors)
- fixture `0053-kafka-requests` (cross-side arms; the one-dir-one-branch
  companion for the KafkaBroker filter)
