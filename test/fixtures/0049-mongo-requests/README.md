# Fixture 0049 — mongo-requests (cross-side; request-side decode parity)

Cross-side differential fixture for the `envoy.filters.network.mongo_proxy`
L4 network filter (phase 29.1 / ADR-0224). It is the **first cross-side fixture
for mongo_proxy**: each listener's filter chain is `[mongo_proxy, tcp_proxy]`,
so a read filter (mongo_proxy) decodes every mongo wire request on the
connection — ticking the per-opcode / per-command / per-collection / per-callsite
counters and the `decoding_error` family — then the terminal `tcp_proxy` drains
the bytes to a **silent TCP sink** (BackendKind=TCPSink). Both reference Envoy
(dockerized) and envoy-go boot the same two-listener bootstrap; the driver
asserts per-stat counter parity across a nine-arm workload, generalizing the
`0046-zookeeper-requests` cross-side mechanics to mongo's label-bearing
Prometheus surface.

## Status

LANDED. The driver was built in three increments:

- **Task 14 (part 1):** the driver SKELETON — the self-contained little-endian
  mongo wire/BSON builders (D-S29.1-3, shared verbatim with the future 29.2
  `0051` driver), the two-listener bootstraps, the MultiListener plumbing, and
  the TCPSink wiring.
- **Task 15 (part 2):** the label-aware `StatsAsserter` (the Prometheus parse
  generalized to `(metric + sorted-label-set) → value`) + arms 1-5. Cross-side
  PASS.
- **Task 16 (part 3):** arms 6-9 + the sibling `0050-mongo-boot-reject` fixture.
  Cross-side PASS (`ref ≡ subj` on every keyed counter); the R4 deliberate-break
  liveness proof is recorded below.

The cross-side run drives both proxies and asserts per-stat counter parity over
the eight traffic/assertion arms (arm 9 is the recorded deliberate-break, not a
live arm).

## Fixture type

Cross-side (`MultiListenerDriver` + `StatsAsserter` + `BackendKindAware`). The
runner spawns BOTH proxies, drives traffic against both listeners, diffs the
side-independent verdict output, then runs `AssertStats` (the
asserter-dispatch-mandated cross-side assertion path — a `SubjectAsserter` would
NOT run on the cross-side path and would be a dead vacuous assertion, per the
`reference_differential_asserter_dispatch` project memory). No
`DistributionAsserter` is needed: both sides talk to the same sink backend and
per-backend accept counts carry no routing-proof signal.

## Topology (SPEC §8.1)

Two listeners, two stat_prefixes, both filter chains `[mongo_proxy, tcp_proxy]`,
both routing to ONE cluster (`c_sink`) → ONE silent TCPSink backend:

| listener     | reference port | stat_prefix | `commands`                       | arms          |
| ------------ | -------------- | ----------- | -------------------------------- | ------------- |
| `l_default`  | 19140          | `mongo_a`   | default (`{delete,insert,update}`) | 1,3,4,5,6,7,8,9 |
| `l_commands` | 19141          | `mongo_b`   | `["isMaster"]`                   | 2             |

### Backend choice: TCPSink (not TCPEcho)

A TCPEcho backend would push echoed mongo request bytes back through reference
Envoy's response path, and the 29.1 scope is **request-only** (SPEC §2 / §8.1).
A silent sink drains reads without writing, so no response bytes traverse the
filter chain on either side. Mirrors the `0046-zookeeper-requests` choice.

### Bootstrap discipline

The `tcp_proxy` terminal needs an upstream cluster — `c_sink` (the runner's
TCPSink backend) is both the proxy target AND the boot-satisfying cluster (a
zero-cluster boot is rejected by both sides). The `mongo_proxy` `@type` URL
carries the `extensions.` segment:
`type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`.

## The nine arms (SPEC §8.1.3 — as implemented in `driver/driver.go`)

The arm-to-listener dispatch below matches the driver source (the authoritative
record). The cumulative per-prefix `want` values live in the ARM-ACCOUNTING
TABLE comment above `driveProxy`.

1. **plain query** (`l_default`, fresh conn): OP_QUERY to `db.collection1` with
   query `{a: 1}` (no `_id`, no `maxTimeMS`) → `op_query` +1,
   `op_query_scatter_get` +1, `op_query_no_max_time` +1,
   `collection.collection1.query.total` +1, `collection.collection1.query.scatter_get` +1.
2. **$cmd + commands-list semantics** (`l_commands`, two fresh conns — the
   AMEND-B7 / D-P8 proof): (i) OP_QUERY to `admin.$cmd` with `{isMaster: 1}` →
   `cmd.isMaster.total` +1 (in the list); (ii) OP_QUERY to `admin.$cmd` with
   `{foo: 1}` → `cmd.unknown_command.total` +1 (not in the list). Both arms also
   `op_query` +1 each; NEITHER increments `op_query_no_max_time` (the $cmd
   exclusion).
3. **query-shape variants** (`l_default`, fresh conns): `{_id: 7}` scalar →
   PrimaryKey (only `collection.<c>.query.total`); `{_id: {$in-doc}}` (a
   Document-typed `_id`) → MultiGet (`op_query_multi_get` + `.query.multi_get`);
   a query with flag bits `0x02|0x10|0x20|0x40` → the four `op_query_*` flag
   counters +1 each.
4. **other request opcodes** (`l_default`, one conn): OP_INSERT → `op_insert` +1;
   OP_GET_MORE → `op_get_more` +1; OP_KILL_CURSORS → `op_kill_cursors` +1;
   OP_COMMAND → `op_command` +1.
5. **$comment callsite** (`l_default`, fresh conn): OP_QUERY to `db.collection1`
   with `{a: 1, $comment: "{\"callingFunction\": \"fixtureFn\"}"}` →
   `collection.collection1.callsite.fixtureFn.query.total` +1 AND
   `collection.collection1.query.total` +1 (AMEND-C3 double-count).
6. **unsupported-opcode + sniffing-off** (`l_default`, FRESH conn): an OP_MSG
   (2013) frame is an unsupported opcode → `decoding_error` +1 both sides; the
   bytes still pass through to the backend. The first decode error turns sniffing
   OFF for the connection lifetime (AMEND-B6), so a follow-up VALID OP_QUERY on
   the SAME conn increments **NOTHING** — this is why `op_query{mongo_a}` stays at
   **5, not 6** (the dropped-query proof). The arm-6 follow-up contributes ZERO to
   every per-prefix query counter.
7. **garbage-BSON arm** (`l_default`, FRESH conn — separate from arm 6): a
   well-framed OP_QUERY whose BSON document is malformed (bad element type
   `0x13`) → the BSON walk fails BEFORE any `op_query`/collection counter
   increments → `decoding_error` +1 both sides (a second independent +1; the
   throw-parity proof). Contributes ZERO to every query counter.
8. **exists-at-zero / gauge-TYPE parity** (assertion-only, no traffic): the
   response-side counters (`op_reply*`, `op_command_reply`, `delays_injected`,
   `cx_drain_close`) present-and-0 on both sides + both prefixes; the
   `op_query_active` gauge's `# TYPE … gauge` line present on both sides;
   `cx_destroy_*_with_active_rq` PRESENT both sides (value NOT compared — AMEND-C2,
   the 29.2 increment). Creation-parity proof — the increments are 29.2's.
9. **deliberate-break** (recorded R4 procedure — see below): flips an expected
   counter value / removes the name.go tag-extractor arm and confirms the test
   FAILS, proving `AssertStats` is not vacuous.

Because arms 6 and 7 each open a FRESH connection and `decoding_error` is
at-most-once per connection lifetime (D-S29.1-6), the cumulative
`decoding_error{mongo_a}` is **2** (a6 + a7). Neither error frame advances any
`op_query`/`collection.*.query.*` counter, so those cumulative `want` values are
unchanged from the arms 1-5 totals.

## R4 deliberate-break liveness proof (arm 9)

Two temporary breaks were run cross-side with `-count=1` (defeating Go's test
result cache per `reference_differential_break_protocol_count1`), then reverted.
Both breaks made the cross-side fixture FAIL, proving the assertions are LIVE
(not vacuous) per `reference_differential_asserter_dispatch`.

**Break 1 — flip one `want`** (`op_query{mongo_a}` `5 → 99`): cross-side FAILS on
both runner paths. BOTH sides report the true value `5`; the wrong `want 99` is
caught:

```
runner_test.go:1082: ref envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} = 5, want 99
runner_test.go:1082: subj envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} = 5, want 99
--- FAIL: TestDifferential/0049-mongo-requests (4.15s)
```

**Break 2 — disable the §7.4 `name.go` mongo tag-extractor arm** (comment out the
`mongo.` arm in `internal/stats/name.go`): EVERY `envoy_mongo_*` label-keyed
lookup goes ABSENT on the subject side, so every keyed assertion fails — proving
the §7.4 arm is load-bearing:

```
runner_test.go:1082: subj: counter envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} ABSENT (creation / name-shape / label-extraction failure)
runner_test.go:1082: subj: counter envoy_mongo_op_query_scatter_get{envoy_mongo_prefix="mongo_a"} ABSENT (creation / name-shape / label-extraction failure)
... (every envoy_mongo_* keyed lookup ABSENT — op_query family, decoding_error,
    collection.*, cmd.*, the exists-at-zero response-side counters, and the
    op_query_active gauge) ...
runner_test.go:1082: subj: gauge envoy_mongo_op_query_active{envoy_mongo_prefix="mongo_a"} ABSENT (creation failure)
--- FAIL: TestDifferential/0049-mongo-requests
```

After BOTH reverts, the cross-side run is GREEN again and `internal/stats/name.go`
is unmodified.

## Cross-references

- phase 29.1 SPEC §8.1 (this fixture's scope) + §7.4 (the Prometheus label table)
- 29.1 PLAN Task 14 (driver part 1) + Task 15/16 (StatsAsserter + arms)
- fixture `0046-zookeeper-requests` (cross-side MultiListener + StatsAsserter +
  TCPSink structural precedent)
- ADR-0224 (mongo_proxy filter architecture)
- project memory `reference_differential_asserter_dispatch` (StatsAsserter is
  load-bearing for cross-side; SubjectAsserter would be vacuous here)
