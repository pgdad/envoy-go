# Fixture 0058 — thrift-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.network.thrift_proxy` L4 network filter (phase 33 SPEC
§8.2 / §6.2). Asserts that BOTH the contrib reference Envoy
(`envoyproxy/envoy:contrib-v1.37.2`) AND envoy-go fail-close at
config-load when the filter's required `stat_prefix` field is MISSING.

## Fixture type

BOOT-REJECT (`differential.BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch starts both proxies via the `tryStart*`
variants, asserts BOTH return a non-nil err (boot rejection on both
sides — this is the PRIMARY, load-bearing claim), and asserts BOTH
stderr buffers contain the case-sensitive substring `thrift_proxy` (a
SECONDARY sanity check; see "Substring discipline" below for why
`thrift_proxy` and not `stat_prefix`).

## Boot-reject trigger

A `thrift_proxy.v3.ThriftProxy` config with `stat_prefix` OMITTED. The
proto marks `stat_prefix` PGV-required (min 1 rune), so BOTH the C++
reference Envoy (PGV) and envoy-go (the `errStatPrefixRequired` const in
`config.go` — the ADR-0080 byte-stable reject string) reject it at
config-load — a genuine both-sides-reject (SPEC §8.2).

This fixture is the BOOT-REJECT branch of the fixture-dispatch pair:
per the project's fixture-dispatch constraint (one fixture dir = one
runner branch), this directory covers the symmetric boot-reject arm
only. The cross-side request arms live in `0057-thrift-roundtrip`.

## Relationship to 0056-redis-boot-reject

This fixture mirrors `0056-redis-boot-reject` (and 0050-mongo / 0054-kafka)
exactly, replacing the `redis_proxy` filter with `thrift_proxy`. The
`BootRejectFixture` discipline, bootstrap structure, the `c_unused`
cluster ordering sidestep, AND the CamelCase-vs-snake_case substring
asymmetry (resolved with the `<filter>_proxy` substring) are identical
to the 0056 precedent.

## Other reject arms stay UNIT-TEST-ONLY

The thrift_proxy proto + envoy-go-strict validation carry additional
reject arms beyond `stat_prefix` — notably the route / route-action /
thrift-filter-name PGV arms and the un-chosen transport/protocol
DEPARTURE arms (envoy-go-strict rejects transport/protocol enum values
the reference parse-ACCEPTS — AMEND-T7, so those are NOT cross-side
boot-rejects). Per phase 33 PLAN Task 13, those arms are covered
UNIT-TEST-ONLY by
`internal/filter/network/thriftproxy/config_test.go` (the
`TestParseConfig` validation arms) and `route_test.go`
(`TestRouteParseRejects`), NOT as additional differential boot-reject
fixtures. This fixture covers ONLY the `stat_prefix`-missing symmetric
boot-reject — the one load-bearing FIXTURE arm.

## Substring discipline (honest cross-impl divergence)

The PRIMARY, load-bearing claim of this fixture is that BOTH sides FAIL
TO BOOT (the runner's `refErr != nil && subjErr != nil` gate). The
shared stderr substring is a SECONDARY sanity check.

The two implementations word the SAME violation DIFFERENTLY (captured
empirically against `envoyproxy/envoy:contrib-v1.37.2`):

- subject (envoy-go): `thrift_proxy: stat_prefix is required` —
  snake_case; both `thrift_proxy` and `stat_prefix` are in the error
  line itself
  (`listener manager: listener: "l_thrift": filter_chains[0]:
  filters[0]: thrift_proxy: stat_prefix is required`).
- reference (C++ Envoy): `Proto constraint validation failed
  (ThriftProxyValidationError.StatPrefix: value length must be at
  least 1 characters)` — CamelCase `StatPrefix`, NOT lowercase
  `stat_prefix`.

So **lowercase `stat_prefix` does NOT appear in the reference's genuine
stderr** — the field was OMITTED from the bootstrap, so there is no
`stat_prefix:` line to echo, and the violation is rendered in CamelCase.

The committed substring is therefore **`thrift_proxy`** — the strongest
token that GENUINELY appears in BOTH real stderrs from a NON-circular
source:

- **subject side** (the side under test): `thrift_proxy` is the leading
  token of the envoy-go error line itself (`thrift_proxy: stat_prefix
  is required`); the subject stderr is the error line, so the match is
  fully load-bearing.
- **reference side**: the harness boots the reference via
  `--config-yaml <bootstrap>`, which echoes the FULL offending
  bootstrap on rejection. `thrift_proxy` appears TWICE in that echo —
  the REAL filter `name: envoy.filters.network.thrift_proxy` and the
  `thrift_proxy.v3.ThriftProxy` typed_config @type — load-bearing
  config tokens that SELECT this filter, NOT a comment injected to
  satisfy the assertion. The genuine reference-reject assertion remains
  the runner's separate `refErr != nil` gate.

Empirical capture (the reference DID boot-reject — full raw stderr,
trimmed):

```
[critical][main] [source/server/server.cc:453] error initializing config:
... name: envoy.filters.network.thrift_proxy
    "@type": type.googleapis.com/...thrift_proxy.v3.ThriftProxy ...
: Proto constraint validation failed
  (ThriftProxyValidationError.StatPrefix: value length must be at least
   1 characters)
```

## Bootstrap discipline

Self-contained inline bootstrap. A minimal `c_unused` cluster
(127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
(which runs BEFORE the listener manager) does not fail with a
zero-cluster error before the listener config-load reject fires. Same
ordering sidestep as fixtures 0033/0041/0042/0044/0047/0050/0054/0056.

## Cross-references

- phase 33 SPEC §8.2 (this fixture's scope) + §6.2 (boot-stderr substring parity)
- 33 PLAN Task 13 (this fixture)
- `harness.go` `BootRejectFixture` interface (`runBootRejectFixture` branch)
- fixture `0056-redis-boot-reject` (the symmetric template this mirrors)
- fixture `0057-thrift-roundtrip` (cross-side arms; the one-dir-one-branch
  companion for the ThriftProxy filter)
