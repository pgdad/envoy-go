# Fixture 0046 — zookeeper-requests (cross-side; R8 read-seam proof)

Cross-side differential fixture for the `envoy.filters.network.zookeeper_proxy`
L4 network filter (phase 28.1 / ADR-0222). It is the **first cross-side fixture
for zookeeper_proxy** and — re-enabled at phase 28.1b — the **load-bearing proof
of the read-side seam (R8)**: each listener's filter chain is
`[zookeeper_proxy, tcp_proxy]`, so a read filter (zookeeper_proxy) decodes every
ZooKeeper request frame on the connection and ticks per-opcode counters, then
the terminal `tcp_proxy` drains the bytes to a silent TCP sink. Both reference
Envoy v1.37.2 (dockerized) and envoy-go boot the same two-listener bootstrap;
the driver asserts per-opcode counter parity + the cross-side `request_bytes`
wire-footprint equality across a seven-arm workload.

## Fixture type

Cross-side (`MultiListenerDriver` + `StatsAsserter`). The runner spawns BOTH
proxies, drives traffic against both listeners, diffs the side-independent
verdict output, then runs `AssertStats` (the asserter-dispatch-mandated
cross-side assertion path — a `SubjectAsserter` would NOT run on the
cross-side path and would be a dead vacuous assertion). No
`DistributionAsserter` is needed: both sides talk to the same sink backend
and per-backend accept counts carry no routing-proof signal.

## The read-seam dependency (R8 — why this fixture exists)

This fixture was committed-but-DISABLED at the 28.1a closure (the ADR-0045
28.1a/28.1b split). Its multi-frame arms (2, 3, 4) drive several ZooKeeper
frames on ONE connection as separate, time-spaced socket writes — so they
require **every socket read of the connection's lifetime to reach
`zookeeper_proxy.OnData`**.

- **Reference Envoy** re-iterates the read filter chain on every read for the
  connection's lifetime ("forever re-iteration"), so it decodes and counts every
  frame regardless of which read it arrives in.
- **envoy-go at 28.1a** exited the chain runtime's read loop permanently at the
  terminal handoff (`TerminalReady() → HandleTerminal() → return`), so a
  `[zookeeper_proxy, tcp_proxy]` chain delivered only the FIRST socket read's
  bytes to `zookeeper_proxy.OnData`. Multi-read connections undercounted on the
  subject side → cross-side stat divergence.

The 28.1b **read-side seam** (`readChainConn` + `chainRuntime.replayRead`;
28.1b SPEC §3, ADR-0221 §AMEND) re-feeds post-handoff socket reads back through
the read chain, restoring reference Envoy's forever-re-iteration parity. This
fixture is the cross-side proof that the seam closes the gap.

Arm-by-arm seam dependency:

| Arm | Workload                                   | Reads | Seam needed? | 28.1a status |
|-----|--------------------------------------------|-------|--------------|--------------|
| 1   | single `connect` frame                     | 1     | no           | green        |
| 2   | connect+ping+getdata+create+close (paced)  | many  | **yes (R8)** | RED (undercount) |
| 3   | create2+getchildren2+setwatches2 (paced)   | many  | **yes (R8)** | RED (undercount) |
| 4   | oversized garbage → pause → recovery getdata | 2+  | **yes (R8)** | RED (undercount) |
| 5   | single `getdata` on l_flags                | 1     | no           | green        |
| 6   | exists-at-zero (assertion-only, no traffic)| 0     | no           | green        |
| 7   | deliberate-break (recorded procedure)      | —     | —            | —            |

The 28.1a Task-16 BLOCKED divergence table is the authoritative pre-seam record;
arms 1/5/6 were already green at 28.1a, and arms 2/3/4 are the seam's proof.

## Topology: `[zookeeper_proxy, tcp_proxy]` → TCPSink (not echo)

The terminal `tcp_proxy` targets a **silent TCP sink** backend
(`BackendKind=TCPSink`; D-S28.1-5), NOT an echo backend. A TCPEcho backend would
push the echoed ZK request bytes back through reference Envoy's `zookeeper_proxy`
`onWrite` response decoder, ticking `*_resp` / `decoder_error` increments that
envoy-go's 28.1 `OnWrite` no-op stub never mirrors → cross-side divergence. A
silent sink drains reads without writing, so no response bytes traverse the
chain on either side, and the 28.1 scope stays request-only (SPEC §8.1.1).

## Two listeners / two stat_prefixes

A single TCPSink backend (`c_sink`) serves both listeners. `tcp_proxy` needs an
upstream cluster, and a zero-cluster boot is rejected by both sides, so `c_sink`
doubles as the boot-satisfying cluster AND the drain target.

- **l_plain** (`stat_prefix = zk_plain`) — flags off. Exercises the basic
  per-opcode request counters (`connect_rq`, `ping_rq`, `getdata_rq`,
  `create_rq`, `close_rq`, `create2_rq`, `getchildren2_rq`, `setwatches2_rq`,
  `decoder_error`) + the global `request_bytes`. The per-opcode
  `getdata_rq_bytes` is created (eager roster) but never incremented (flag off).
- **l_flags** (`stat_prefix = zk_flags`) — `enable_per_opcode_request_bytes_metrics`
  + `enable_per_opcode_decoder_error_metrics` ON. Exercises the flag-gated
  per-opcode `getdata_rq_bytes` (created eagerly AND incremented).

## Seven-arm taxonomy + expected counters

The arms run in declared order over the shared listeners, so the driver asserts
**cumulative** counter values. Each arm drives BOTH sides identically and emits
a side-independent verdict line, so equivalent behavior yields byte-identical
drive output for the runner's `CompareBytes` gate.

- **Arm 1 (connect)** — one `connect` frame on l_plain → `connect_rq` += 1.
- **Arm 2 (multi-opcode)** — one l_plain connection, five paced writes
  (`connect`, `ping`, `getdata` xid 1, `create` xid 2, `close` xid 3). The
  load-bearing `request_bytes` cross-side equality proof. Requires the read seam.
- **Arm 3 (digit-suffixed)** — one l_plain connection, three paced writes
  (`create2` xid 4, `getchildren2` xid 5, `setwatches2` xid 6 as a DATA request
  with wire op 105). Asserts `create2_rq`=1, `getchildren2_rq`=1,
  `setwatches2_rq`=1. Requires the read seam.
- **Arm 4 (garbage + survival)** — one l_plain connection: an oversized length
  prefix (2 MiB > the 1 MiB default `max_packet_bytes` → `decoder_error`), a
  ≥200 ms pause, then a valid recovery `getdata` (xid 8) ON THE SAME
  CONNECTION. The AMEND-A8 no-resync path abandons the read buffer but leaves
  the connection OPEN, and the post-garbage getdata (appended past the decoder
  high-water mark) still decodes → `decoder_error`=1, `getdata_rq`=2
  (cumulative with arm 2's getdata). Requires the read seam.
- **Arm 5 (flag-gated bytes)** — one `getdata` on l_flags →
  `zk_flags.zookeeper.getdata_rq_bytes` > 0 on both sides (cross-side equality);
  `zk_plain.zookeeper.getdata_rq_bytes` stays 0 (flag off on l_plain).
- **Arm 6 (exists-at-zero)** — no traffic (assertion-only, in `AssertStats`).
  The response-side counters (`getdata_resp`, `getdata_resp_fast`,
  `watch_event`, `response_bytes`) are PRESENT and 0 on both sides for both
  prefixes (eager roster creation; SPEC §4.3 exists-at-zero parity).
- **Arm 7 (deliberate-break)** — recorded procedure (no live traffic); see the
  R4 record below.

The full expected cross-side column (re-pinned at 28.1b Task 1):

```
zk_plain.zookeeper.connect_rq        = 2   (arm 1 + arm 2)
zk_plain.zookeeper.ping_rq           = 1   (arm 2)
zk_plain.zookeeper.getdata_rq        = 2   (arm 2 xid 1 + arm 4 recovery xid 8)
zk_plain.zookeeper.create_rq         = 1   (arm 2)
zk_plain.zookeeper.close_rq          = 1   (arm 2)
zk_plain.zookeeper.create2_rq        = 1   (arm 3)
zk_plain.zookeeper.getchildren2_rq   = 1   (arm 3)
zk_plain.zookeeper.setwatches2_rq    = 1   (arm 3)
zk_plain.zookeeper.decoder_error     = 1   (arm 4 oversized frame)
zk_plain.zookeeper.request_bytes     = 307 (cross-side equality; arm 2 load-bearing)
zk_plain.zookeeper.getdata_rq_bytes  = 0   (flag OFF on l_plain)
zk_flags.zookeeper.getdata_rq        = 1   (arm 5)
zk_flags.zookeeper.getdata_rq_bytes  > 0   (flag ON; cross-side equality)
+ exists-at-zero (arm 6): getdata_resp / getdata_resp_fast / watch_event / response_bytes = 0
```

## `request_bytes = 307` cross-side equality proof

`zk_plain.zookeeper.request_bytes` is the wire-footprint sum (length-prefix +
payload) of every frame the decoder accepts on l_plain — the
wire-footprint-as-bytes discipline (SPEC §4.5 item 4). The driver asserts the
two sides AGREE and are > 0 (not a hardcoded literal in the equality clause),
and the re-pinned reference column fixes the value at **307**. Arm 2 (five paced
frames on one connection) is the load-bearing contributor; without the read seam
the subject would only count arm 2's first frame and the equality would fail.
The assertion is `ref == subj && ref > 0`; the 307 value appears only in the
re-pinned reference column (PROGRESS.md Task 1 Step 4) — `AssertStats` checks
cross-side equality and positivity, never a hardcoded literal.

## StatsAsserter mechanics

`AssertStats` is the cross-side asserter (per the asserter-dispatch project
memory: `SubjectAsserter` only runs on the reference-less path and would be a
dead vacuous assertion on a cross-side fixture). The runner invokes it ONCE with
BOTH admin addresses; the driver scrapes `/stats/prometheus` from each
(`scrapeZKStats`), retaining lines whose name contains the `_zookeeper_` infix.
The zookeeper counters carry an EMPTY label set on both sides (AMEND-A4:
upstream applies no tag extraction to this filter; its exposition is flat), so
the driver keys by the bare flattened name (`envoy_<prefix>_zookeeper_<counter>`)
via `lookupZKCounter`. Present-vs-absent is reported DISTINCTLY from a
wrong-value: an ABSENT counter signals a name-shape / eager-creation failure
(this is exactly what R4 break (b) trips). envoy-go renders these names through
the `flattenToProm` `.zookeeper.` inline-prefix arm
(`internal/stats/name.go`, the `zkSegment` case).

## R4 deliberate-break record (both breaks LIVE, both reverted)

Per the project's differential-asserter discipline, the assertions were proven
non-vacuous against the green baseline:

**Break (a) — wrong expected value.** Edited the driver expectation
`{"zk_plain.zookeeper.getdata_rq", 2}` → `3` and ran the fixture. The test
FAILED on BOTH sides (proving the assertion runs against both ref and subj):

```
runner_test.go:1064: ref zk_plain.zookeeper.getdata_rq = 2, want 3
runner_test.go:1064: subj zk_plain.zookeeper.getdata_rq = 2, want 3
--- FAIL: TestDifferential/0046-zookeeper-requests
```

Reverted; `git diff test/fixtures/0046-zookeeper-requests/driver/` is banner-only.

**Break (b) — name-shape liveness.** Commented out the `.zookeeper.` `zkSegment`
arm in `internal/stats/name.go`. With this arm gone, `flattenToProm` errors on
every `<prefix>.zookeeper.<counter>` name and `WriteProm` drops it, so the
subject's `/stats/prometheus` no longer renders the zookeeper counters. The
test FAILED with the subject reporting every counter ABSENT while the reference
retained them:

```
runner_test.go:1064: subj: counter zk_plain.zookeeper.connect_rq ABSENT (creation parity / name-shape failure)
... (all 15 fixed-value counters ABSENT on subj) ...
runner_test.go:1064: cross-side zk_plain.zookeeper.request_bytes: ref=(307,true) subj=(0,false), want present, equal, and > 0
runner_test.go:1064: cross-side zk_flags.zookeeper.getdata_rq_bytes: ref=(25,true) subj=(0,false), want present, equal, and > 0
--- FAIL: TestDifferential/0046-zookeeper-requests
```

Reverted; `git diff internal/stats/` is empty.

Both breaks must be run with `go test -count=1` — the docker-driven differential
test is otherwise served from `go test`'s result cache and a stale PASS can mask
a live name.go break (observed during this task; recorded in PROGRESS.md Task 7).

## Cross-references

- phase 28.1b SPEC §3 (the read-side seam) + §5.1 (this fixture's gate)
- phase 28.1 SPEC §8.1 (cross-side zookeeper-requests fixture scope)
- ADR-0221 §AMEND (`readChainConn` + `replayRead`); ADR-0222 (zookeeper_proxy)
- 28.1a PROGRESS.md Task 16 (the BLOCKED divergence table — pre-seam evidence)
- fixture-0043-network-rbac (cross-side network filter + StatsAsserter template)
- project memory `reference_differential_asserter_dispatch` (StatsAsserter is
  load-bearing for cross-side; SubjectAsserter would be vacuous here)
