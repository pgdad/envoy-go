# Fixture 0051 — mongo-responses (cross-side; response-side decode + correlation)

Cross-side differential fixture for the `envoy.filters.network.mongo_proxy`
L4 network filter (phase 29.2 / ADR-0224). It is the **response-side sibling**
of `0049-mongo-requests`: a single listener (`l_resp`, `stat_prefix mongo_r`)
whose filter chain is `[mongo_proxy, tcp_proxy]` fronts a **MongoDB-aware
canned-RESPONSE backend** (BackendKind=`TCPMongoResponder`). The driver SENDS
requests; the responder writes **correlated** OP_REPLY / OP_COMMANDREPLY frames
back through the chain, so reference Envoy's `onWrite` response decoder (and
envoy-go's mirror) fires. The driver asserts response-side counter parity + the
`op_query_active` gauge quiesced-point + the `cx_destroy_*` presence boundary.

## Status

LANDED (phase 29.2 IMPL Task 11A). Cross-side PASS (`ref ≡ subj` on every keyed
counter; gauge == 0 at rest both sides). The R4 deliberate-break liveness proof
is recorded below.

## Fixture type

Cross-side (`Driver` + `BackendKindAware` + `StatsAsserter`; single-listener, so
NOT `MultiListenerDriver`). The runner spawns BOTH proxies, drives traffic
against the one listener, diffs the side-independent verdict output, then runs
`AssertStats` (the asserter-dispatch-mandated cross-side assertion path — a
`SubjectAsserter` would NOT run on the cross-side path and would be a dead
vacuous assertion, per the `reference_differential_asserter_dispatch` project
memory).

## Topology

One listener, one stat_prefix, filter chain `[mongo_proxy, tcp_proxy]` routing
to ONE cluster (`c_resp`) → ONE `TCPMongoResponder` backend:

| listener | reference port | stat_prefix | backend             |
| -------- | -------------- | ----------- | ------------------- |
| `l_resp` | 19142          | `mongo_r`   | `TCPMongoResponder` |

### Backend choice: TCPMongoResponder (not TCPSink)

29.1's `0049` used a silent `TCPSink` so NO response bytes traversed the chain
(request-only scope). 29.2 is response-side, so it NEEDS a backend that writes
**correlated** replies. `TCPMongoResponder` (fixture.go:520; the
`acceptMongoResponder` / `mongoRespondLoop` in runner_test.go) parses
`messageLength` + `requestID` + `opCode` only and writes a frame whose
`responseTo` echoes the request `requestID`. Marker `requestID`s select the
reply variant; the `dMarker*` driver-local consts MIRROR the responder's
`mongoMarker*` integer values (a different package) so both sides send
byte-identical frames. The dockerized reference reaches the host-side responder
via `host.docker.internal` (STRICT_DNS); the subject uses a STATIC `127.0.0.1`
cluster.

## The six arms (SPEC §6.2)

All on `l_resp` (`mongo_r`), in declared order, each on a FRESH connection that
is closed before the next (so the gauge quiesces between arms; `driveAndReadReply`
reads the reply so the response decode + correlation complete before close — D-P9):

1. **Plain reply round-trip** — OP_QUERY reqID 1 (plain non-marker) → empty
   OP_REPLY (responseTo 1) → `op_reply` +1; correlated → gauge Inc/Dec → 0.
2. **Reply-flag variants** (three fresh conns) — reqIDs 7001 / 7002 / 7003 →
   `op_reply_cursor_not_found` / `_query_failure` / `_valid_cursor` each +1; each
   correlated.
3. **OP_COMMAND round-trip** — OP_COMMAND reqID 20 → OP_COMMANDREPLY →
   `op_command_reply` +1; OP_COMMAND does NOT append an active-query entry, so the
   gauge is untouched.
4. **Unanswered-query / residual drain** — OP_QUERY reqID 7777; the responder
   WITHHOLDS the reply → the query stays in-flight while the conn is open
   (gauge == 1) → the bounded read times out → conn close → `onDestroy` drains the
   residual → gauge back to 0.
5. **Uncorrelated reply** — OP_QUERY reqID 7005 → reply with `responseTo`=reqID+50000
   matching NO sent query → `op_reply` +1, the correlation MISS leaves the gauge
   untouched; the 7005 query itself drains at this conn's close → gauge 0.
6. **Malformed-reply decoding_error** (fresh conn) — OP_QUERY reqID 7004 → a
   malformed OP_REPLY (`numberReturned`=1 with NO doc) → `decoding_error` +1 both
   sides (same bytes; `reference_wire_format_both_sides_see_same_bytes`). The
   request itself is a valid OP_QUERY (counted in `op_query`).

## Cumulative arm-accounting table (asserted in AssertStats)

| counter                       | want | source arms                            |
| ----------------------------- | ---- | -------------------------------------- |
| `op_query`                    | 7    | arms 1, 2×3, 4, 5, 6 (arm 3 is OP_COMMAND) |
| `op_command`                  | 1    | arm 3                                  |
| `op_reply`                    | 5    | arm 1 + arm 2×3 + arm 5                 |
| `op_reply_cursor_not_found`   | 1    | arm 2 (7001)                           |
| `op_reply_query_failure`      | 1    | arm 2 (7002)                           |
| `op_reply_valid_cursor`       | 1    | arm 2 (7003)                           |
| `op_command_reply`            | 1    | arm 3                                  |
| `decoding_error`              | 1    | arm 6 (malformed reply)                |

**`op_query` == 7, not 6.** The Task-11A PLAN's estimate said 6 (counting only
"arms 1, 2×3, 4ii, 5"); it OMITTED arm 6's OP_QUERY. Arm 6's *request* is a
valid OP_QUERY — only the *response* is malformed — so it DOES tick `op_query`.
The **LIVE reference is ground truth** and confirms `op_query` == 7 (both sides);
the table was recounted to match. `op_reply` == 5 because the malformed reply
(arm 6) fails to decode before the `op_reply` inc (`decodeReply`'s `parseDocument`
loop returns `d.fail()` first) and the withhold arm (4) sends no reply.

## The gauge quiesced-point design

The load-bearing **cross-side** gauge proof is **ANSWERED → 0**: every correlated
reply Decs the gauge (first-match-erase by `responseTo`), and at AssertStats time
(all connections closed) the gauge is **0 on both sides** — answered queries Dec'd
it, unanswered/uncorrelated residuals drained at connection close (`onDestroy`).
The `# TYPE envoy_mongo_op_query_active gauge` line is asserted on both sides.

### Unanswered-gauge approach: (B) — unit-covered while-open

The `op_query_active == 1` WHILE-OPEN assertion requires scraping while the
withhold connection is still open, but the runner calls `AssertStats` ONCE after
`DriveReference`/`DriveSubject` return (all connections closed). The Drive*
methods receive only the proxy `addr`, NOT the admin addr, so an inline `== 1`
scrape (approach A) is not cleanly achievable. This fixture uses **approach (B)**
(PLAN-sanctioned default): the cross-side gauge arms are the ANSWERED `== 0` proof
plus the post-close `== 0` residual-drain proof. The unanswered `== 1` while-open
point is **already proven at the unit level** (Task 6
`TestOnDestroy_DrainsResidualGauge` + the lifecycle tests assert the
open-connection gauge==1 and the drain→0 at the decoder level). Arm 4 still drives
the withhold reqID (7777) to exercise the residual-drain-at-close path cross-side.

## `cx_destroy_*` presence-only (D-P4)

`cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq` are asserted
**PRESENT** on both sides but their VALUES are NOT compared. The network framework
records close TYPE not DIRECTION (local/remote), so envoy-go cannot key these
counters until the framework-surgery sub-phase (29.3); the reference increments one
per query-bearing connection close (live: `local`=3) while envoy-go increments
neither (live: 0). This is the close-direction coverage boundary
(`reference_close_direction_framework_gap`). `delays_injected` + `cx_drain_close`
are asserted present + == 0 on both sides.

## NO dynamic-metadata fixture surface

The mongo_proxy dynamic-metadata emission is **proven by unit tests only** (§3.7);
there is no dynamic-metadata cross-side fixture surface here.

## R4 deliberate-break liveness proof (`-count=1`)

Three breaks, each reverted (`reference_differential_break_protocol_count1` — the
`-count=1` flag defeats `go test` result caching so a temporarily-broken
production path actually re-runs):

| break | change                                                              | result            |
| ----- | ------------------------------------------------------------------- | ----------------- |
| (a)   | AssertStats `op_reply` want 6 (received 5)                          | FAIL → revert GREEN |
| (b)   | comment `d.stats.opQueryActive.Dec()` in `decodeReply` correlation  | FAIL: subj gauge = 4, want 0 → revert GREEN |
| (c)   | comment `d.stats.opQueryActive.Inc()` in `appendQuery`              | FAIL: subj gauge = -7, want 0 → revert GREEN |

After all reverts, `codec.go` is byte-identical (0 diff lines) and the fixture is
GREEN. Each break failed a DISTINCT assertion (the table count for (a); the
answered-gauge-quiesced arm for (b)/(c)), proving each assertion is live.
