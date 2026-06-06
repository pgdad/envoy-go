# Fixture 0052 — mongo-fault-delay (cross-side; fault delay + close-direction destroy)

Cross-side differential fixture for the `envoy.filters.network.mongo_proxy`
L4 network filter (phase 29.3 / ADR-0226). It is the **cross-side proof of the
whole 29.3 phase**: the async **fault-delay** halt/resume seam (pre- and
post-handoff), the **`cx_destroy_*` close-direction VALUE parity** (D-P4 CLOSED),
and the **`cx_drain_close`** reply-completion drain stat (PRESENCE-downgraded —
see below). Two listeners on each side, filter chain `[mongo_proxy, tcp_proxy]`,
both fronting the same MongoDB-aware canned-RESPONSE backend
(BackendKind=`TCPMongoResponder`, reused — no new BackendKind). The responder
writes **correlated** OP_REPLY frames whose `responseTo` echoes the request
`requestID`, so both sides see byte-identical wire bytes
(`reference_wire_format_both_sides_see_same_bytes`).

## Status

LANDED (phase 29.3 IMPL Task 11). Cross-side PASS on BOTH runner paths (reference
Envoy v1.37.2 docker + envoy-go subprocess); `ref ≡ subj` on every keyed counter.
The R4 deliberate-break liveness proof is recorded below.

## Fixture type

Cross-side (`Driver` + `MultiListenerDriver` + `BackendKindAware` +
`StatsAsserter`). The runner spawns BOTH proxies, drives traffic against the two
listeners, diffs the side-independent verdict output, then runs `AssertStats`
(the asserter-dispatch-mandated cross-side assertion path — a `SubjectAsserter`
would NOT run on the cross-side path and would be a dead vacuous assertion, per
the `reference_differential_asserter_dispatch` project memory).

## Topology

Two listeners, two stat_prefixes, each filter chain `[mongo_proxy, tcp_proxy]`
routing to ONE cluster (`c_resp`) → ONE `TCPMongoResponder` backend:

| listener    | reference port | stat_prefix | delay block                       | backend             |
| ----------- | -------------- | ----------- | --------------------------------- | ------------------- |
| `l_delayed` | 19143          | `mongo_d`   | `fixed_delay: 0.100s` @ 100% HUNDRED | `TCPMongoResponder` |
| `l_nodelay` | 19144          | `mongo_nd`  | (none)                            | `TCPMongoResponder` |

The delay is **DETERMINISTIC** (100% probability, fixed 0.100s). The injected
latency is **invisible to correctness** — the fixture asserts only the
`delays_injected` VALUE and the traffic-completes verdict. **Timing/duration is
NEVER scraped or compared** on either side.

## The arms (SPEC §6.1)

1. **fault-delay round-trip (pre + post handoff)** — ONE connection on
   `l_delayed`: OP_QUERY reqID 1 → read the correlated OP_REPLY; OP_QUERY reqID 2
   on the SAME connection → read its reply. Delay 1 fires **PRE-handoff** (in the
   read-loop's `runData` hold), delay 2 fires **POST-handoff** (in `replayRead`),
   exercising both halt sites of the async seam. `delays_injected{mongo_d} == 2`
   BOTH sides; both replies received (the passthrough is never broken).

2. **seam non-perturbation (no-delay)** — plain OP_QUERY → reply on `l_nodelay`.
   `delays_injected{mongo_nd} == 0` BOTH sides (R1 live equivalence — a no-delay
   mongo filter is non-haltable, so the chain takes the byte-identical pre-29.3
   path); the reply is received exactly as on the delayed listener but with no halt.

3. **`cx_drain_close`** — PRESENCE-DOWNGRADED (D-S29.3-8; see the disposition
   section below). Asserted present + `== 0` on both sides; no drain is driven.

4. **`cx_destroy_*` VALUE parity (D-P4 CLOSED)** — on `l_nodelay`:
   - **(i) LOCAL**: OP_QUERY with the withhold marker (reqID 7777) — the responder
     reads but withholds the reply and keeps its conn OPEN; the bounded read times
     out; the **DRIVER** then closes its end (downstream EOF first) with the query
     in-flight → `cx_destroy_local_with_active_rq{mongo_nd} == 1` BOTH sides.
   - **(ii) REMOTE**: OP_QUERY with the remote-close marker (reqID 7006) — the
     responder reads then **closes its backend conn WITHOUT replying** (upstream
     EOF first); the query is in-flight at the upstream close →
     `cx_destroy_remote_with_active_rq{mongo_nd} == 1` BOTH sides.
   - **(iii) all-answered**: plain OP_QUERY → reply (the active-query list empties
     on the correlated reply) → close → the list is EMPTY at close → **NEITHER**
     `cx_destroy` increments (the control case).

5. **all-quiesced roster** — after the arms: `op_query_active == 0` on BOTH
   prefixes, BOTH sides (the 29.2 gauge re-proven under fault load — every answered
   query Dec'd it; every in-flight residual drained at its connection close), with
   the `# TYPE … gauge` line asserted per side. The asserted counters at their values.

### The close-direction mechanics (arm 4)

The close DIRECTION is recorded by `tcp_proxy`: the downstream→upstream pump
returns on downstream (LOCAL) EOF, the upstream→downstream pump on upstream
(REMOTE) EOF; the first to return wins (first-wins CAS on the chain). Arm 4(i)
keeps the backend conn open and closes the DRIVER end → LOCAL; arm 4(ii) closes
the BACKEND end → REMOTE. The `mongoMarkerRemoteClose` (reqID 7006) was added to
`mongoRespondLoop` for arm 4(ii); the existing `mongoMarkerWithhold` (7777) serves
arm 4(i) (it withholds the reply but keeps the conn open, so the LOCAL close wins).

## Arm-accounting table (asserted in AssertStats)

| counter (per prefix)                  | `mongo_d` | `mongo_nd` |
| ------------------------------------- | --------- | ---------- |
| `delays_injected`                     | **2**     | **0**      |
| `cx_destroy_local_with_active_rq`     | 0         | **1**      |
| `cx_destroy_remote_with_active_rq`    | 0         | **1**      |
| `cx_drain_close`                      | 0 (presence) | 0 (presence) |
| `op_query_active` (gauge, @rest)      | 0         | 0          |

Live dump (`FIXTURE_0052_DUMP_STATS=1`) — `ref` and `subj` BYTE-IDENTICAL on all
of the above.

## `cx_drain_close` differential disposition — PRESENCE-DOWNGRADED (D-S29.3-8)

The differential `cx_drain_close` arm is **DOWNGRADED to a PRESENCE +
exists-at-zero assertion** on both sides. The stat fires only when a correlated
reply EMPTIES the active-query list WHILE the connection's callbacks report
`Draining() == true` (`filter.go` `OnWrite`). Driving that cross-side would
require (a) an admin `POST /drain_listeners` and (b) a reply-completion landing
inside the narrow draining window — but the driver's **drive phase has no admin
address** (the runner passes admin addrs only to `AssertStats`, not to
`DriveSubjectMulti`), and the reply-vs-drain ordering is **not deterministically
reproducible** across the docker reference + the subprocess subject.

Per D-S29.3-8 this is a **sanctioned downgrade**. The load-bearing ratification is
the **Task-9 UNIT value proof** (deterministic):
`TestFilter_DrainCloseOnEmptyListWhenDraining` (== 1) +
`TestFilter_NoDrainCloseWhenNotDraining` (== 0). The phase is NOT blocked on a
flaky differential drain arm.

## NO access-log fixture surface (AMEND-B10)

The 29.3 mongo access log is **timing-bearing** and therefore
**differential-INVISIBLE** (AMEND-B10 / D-P7): the JSON log line carries a
duration that legitimately differs cross-side, so it is NOT asserted by any
fixture. Its proof is the unit goldens (Task 7/8). This fixture does NOT configure
`access_log` and asserts no log surface.

## R4 deliberate-break liveness proof (`-count=1`)

Each new assertion proven LIVE by a temporary deliberate break + revert, run with
`-count=1` (Go caches test results — without it a stale PASS would hide the break,
per `reference_differential_break_protocol_count1`). Reverts via `git restore`.

**(a) `delays_injected` expected 3 (when 2 armed)** —
`go test ./test/differential/ -run 'TestDifferential/0052' -count=1`:
```
ref  envoy_mongo_delays_injected{envoy_mongo_prefix="mongo_d"} = 2, want 3
subj envoy_mongo_delays_injected{envoy_mongo_prefix="mongo_d"} = 2, want 3
--- FAIL: TestDifferential/0052-mongo-fault-delay
```
→ FAIL BOTH paths. REVERTED (expected 2) → `--- PASS: TestDifferential/0052-mongo-fault-delay`.

**(b) Task-10 `cx_destroy_*` switch commented out** (filter.go `OnDestroy`) —
same command:
```
subj envoy_mongo_cx_destroy_local_with_active_rq{envoy_mongo_prefix="mongo_nd"} = 0, want 1
subj envoy_mongo_cx_destroy_remote_with_active_rq{envoy_mongo_prefix="mongo_nd"} = 0, want 1
--- FAIL: TestDifferential/0052-mongo-fault-delay
```
→ FAIL **subject-side only** (the reference stays correct — a true cross-side proof).
REVERTED (`git restore`) → PASS.

**(c) Task-9 `cx_drain_close` increment commented out** (PRESENCE-downgraded → the
UNIT test is the proof per the protocol) —
`go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_DrainClose|TestFilter_NoDrainClose' -count=1`:
```
TestFilter_DrainCloseOnEmptyListWhenDraining: cx_drain_close = 0, want 1
--- FAIL: TestFilter_DrainCloseOnEmptyListWhenDraining
```
(`TestFilter_NoDrainCloseWhenNotDraining` correctly stays PASS — it asserts 0.)
REVERTED (`git restore`) → `ok`.

## Cross-references

- phase 29.3 SPEC §6.1 (the cross-side arms) + §3.2 (fault delay) / §3.4
  (`cx_drain_close`) / §3.5 (close direction).
- 29.3 IMPL Task 11 (this fixture) + D-S29.3-8 (the `cx_drain_close` downgrade).
- `0049-mongo-requests` (the `MultiListenerDriver` + two-listener bootstrap template).
- `0051-mongo-responses` (the `TCPMongoResponder` + correlated-reply read template).
- ADR-0226 (the 29.3 fault/log/drain/close-direction architecture).
- project memory `reference_close_direction_framework_gap` (cx_destroy_* keyed by
  close direction — VALUE parity at 29.3, D-P4 CLOSED).
