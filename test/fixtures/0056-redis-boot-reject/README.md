# Fixture 0056 — redis-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.redis_proxy` L4 network filter (phase 32.1 SPEC
§8.1). Asserts that BOTH reference Envoy AND envoy-go fail-close at
config-load when the filter's required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`differential.BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the `tryStart*`
variants, asserts BOTH return a non-nil err (boot rejection on both
sides — this is the PRIMARY, load-bearing claim), and asserts BOTH
stderr buffers contain the case-sensitive substring `redis_proxy` (a
SECONDARY sanity check; see "Substring discipline" below for why
`redis_proxy` and not `stat_prefix`).

## Boot-reject trigger

A `redis_proxy.v3.RedisProxy` config with `stat_prefix` OMITTED. The
proto marks `stat_prefix` PGV-required (min 1 rune), so BOTH the C++
reference Envoy (PGV) and envoy-go (the `errStatPrefixRequired` const in
`config.go`) reject it at config-load — a genuine both-sides-reject
(SPEC §8.1).

This fixture is the BOOT-REJECT branch of the fixture-dispatch pair:
per the project's fixture-dispatch constraint (one fixture dir = one
runner branch), this directory covers the symmetric boot-reject arm
only. The cross-side request arms live in `0055-redis-roundtrip`.

## Relationship to 0054-kafka-boot-reject

This fixture mirrors `0054-kafka-boot-reject` (and `0050-mongo-boot-reject`)
exactly, replacing the `kafka_broker` filter with `redis_proxy`. The
`BootRejectFixture` discipline, bootstrap structure, and `c_unused`
cluster ordering sidestep are identical to the 0050/0054/0047 precedent.

NOTE: the substring choice DIFFERS from the kafka-0054 precedent.
kafka-0054 can assert `stat_prefix` because its echoed `tcp_proxy`
filter carries its own `stat_prefix` field, so the token appears in the
reference stderr naturally. `redis_proxy` is a TERMINAL network filter
(no `tcp_proxy` in the chain), so there is NO natural lowercase
`stat_prefix` token in the reference stderr — see "Substring
discipline" below.

## Other PGV reject arms stay UNIT-TEST-ONLY (D-P32-5)

The redis_proxy proto carries additional validation constraints beyond
`stat_prefix` — notably `settings.op_timeout` (required), the
`no_upstream_connections` / catch-all-cluster constraints, and related
nested-field rules. Per phase 32.1 D-P32-5, those further reject arms
are covered UNIT-TEST-ONLY by
`internal/filter/network/redisproxy/config_test.go` (the `parseConfig`
validation arms), NOT as additional differential boot-reject fixtures.
This fixture covers ONLY the `stat_prefix`-missing symmetric boot-reject.

## Substring discipline (honest cross-impl divergence)

The PRIMARY, load-bearing claim of this fixture is that BOTH sides FAIL
TO BOOT (the runner's `refErr != nil && subjErr != nil` gate). The
shared stderr substring is a SECONDARY sanity check.

The two implementations word the SAME violation DIFFERENTLY:

- subject (envoy-go): `redis_proxy: stat_prefix is required` —
  snake_case; both `redis_proxy` and `stat_prefix` are in the error
  line itself.
- reference (C++ Envoy): `RedisProxyValidationError.StatPrefix: value
  length must be at least 1 characters` — CamelCase `StatPrefix`, NOT
  lowercase `stat_prefix`.

So **lowercase `stat_prefix` does NOT appear in the reference's genuine
stderr.** This was verified directly: removing all driver-injected
tokens and capturing the raw reference-container stderr, the runner
reported `reference stderr does NOT contain "stat_prefix"`. The
reference's full genuine stderr (1565 bytes) is the CamelCase PGV
violation plus an echo of the rejected bootstrap (which contains the
filter `name: envoy.filters.network.redis_proxy` and the
`redis_proxy.v3.RedisProxy` @type, but NO lowercase `stat_prefix`).

An EARLIER version of this driver matched `stat_prefix` on the
reference side ONLY because the bootstrap carried a driver comment
(`# stat_prefix INTENTIONALLY OMITTED`) that Envoy echoed back — a
CIRCULAR match that would have held regardless of WHY boot failed. That
comment has been REMOVED.

The committed substring is therefore **`redis_proxy`** — the strongest
token that GENUINELY appears in BOTH real stderrs from a NON-circular
source:

- **subject side** (the side under test): `redis_proxy` is the leading
  token of the envoy-go error line itself (`redis_proxy: stat_prefix is
  required`); the subject stderr is JUST the error line (no YAML echo),
  so the match is fully load-bearing.
- **reference side**: `redis_proxy` appears in the echoed config's REAL
  filter `name: envoy.filters.network.redis_proxy` and the
  `redis_proxy.v3.RedisProxy` typed_config @type — load-bearing config
  tokens that SELECT this filter, NOT a comment injected to satisfy the
  assertion. The genuine reference-reject assertion remains the runner's
  separate `refErr != nil` gate.

Liveness was proven by an R6-style break: temporarily setting the
substring to `zzz_not_present` made the test FAIL with `reference
stderr does NOT contain "zzz_not_present"`, confirming the substring is
matched against real stderr; reverting to `redis_proxy` restored PASS.

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042/0044/0047/0050/0054.

## Cross-references

- phase 32.1 SPEC §8.1 (this fixture's scope)
- 32.1 PLAN Task 15 (this fixture)
- `harness.go` `BootRejectFixture` interface (`runBootRejectFixture` branch)
- fixture `0054-kafka-boot-reject` (the symmetric template this mirrors)
- fixture `0055-redis-roundtrip` (cross-side arms; the one-dir-one-branch
  companion for the RedisProxy filter)
