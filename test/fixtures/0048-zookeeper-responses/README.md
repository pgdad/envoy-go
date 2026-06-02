# Fixture 0048 — zookeeper-responses (cross-side; R4/R5 response-decoder proof)

Cross-side differential fixture for the `envoy.filters.network.zookeeper_proxy`
**response decoder** (phase 28.2 / ADR-0223). 0046-zookeeper-requests proved the
request side against a silent sink; this fixture is the first cross-side proof
of the 28.2 response side: each listener's filter chain is
`[zookeeper_proxy, tcp_proxy]`, so the read filter (zookeeper_proxy) decodes
ZooKeeper request frames AND — via `onWrite` (reference Envoy) / the 28.2
`OnWrite` glue (envoy-go) — decodes the correlated response frames the upstream
writes back. Both reference Envoy v1.37.2 (dockerized) and envoy-go boot the
same four-listener bootstrap; the driver asserts per-opcode response-counter +
latency-bucket + response-bytes parity across an eight-arm workload.

## Fixture type

Cross-side (`MultiListenerDriver` + `StatsAsserter`). Per
`reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE
runner branch) this fixture is exclusively cross-side — there are NO boot-reject
arms (those live in 0047). The asserter is `StatsAsserter` (NOT
`SubjectAsserter`, which only runs on the reference-less path and would be a dead
vacuous assertion on a cross-side fixture —
`reference_differential_asserter_dispatch`). The runner spawns BOTH proxies,
drives the same workload against both, diffs the side-independent verdict output,
then runs `AssertStats` ONCE with both admin addresses (the driver scrapes both
sides in-band).

## Topology: `[zookeeper_proxy, tcp_proxy]` → TCPZKResponder (not TCPSink)

The terminal `tcp_proxy` targets a **driver-controlled ZooKeeper-aware
canned-response** backend (`BackendKind=TCPZKResponder`; the runner's
`acceptZKResponder`), NOT a silent sink.

**Why TCPSink cannot serve this fixture.** 0046's `TCPSink` accepts + drains +
NEVER writes, so no response bytes ever traverse the filter chain and the
response decoder is never exercised — `*_resp` / latency / `response_bytes`
counters would stay at 0 on both sides (vacuously equal). To prove the response
decoder, the backend must write CORRELATED ZooKeeper response frames back
through the chain. `TCPZKResponder` is that backend: for every request frame it
reads it waits a FIXED 10 ms delay (`zkResponderDelay`), then writes a correlated
canned response.

A single `TCPZKResponder` backend (cluster `c_zk`) serves all four listeners.
`tcp_proxy` needs an upstream cluster and a zero-cluster boot is rejected by both
sides, so `c_zk` doubles as the boot-satisfying cluster AND the responder target.
The `zookeeper_proxy` `@type` URL carries the `extensions.` segment
(`reference_network_filter_typeurl_extensions`):
`type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`.

## Four listeners / four stat_prefixes

| Listener   | stat_prefix | config                                                                                                   | arms |
|------------|-------------|----------------------------------------------------------------------------------------------------------|------|
| `l_resp`   | `zk_resp`   | defaults (NO latency metrics, NO resp-bytes flag)                                                         | 1-3  |
| `l_fast`   | `zk_fast`   | `enable_latency_threshold_metrics` + `default_latency_threshold: 3600s`                                   | 4    |
| `l_slow`   | `zk_slow`   | `enable_latency_threshold_metrics` + `default_latency_threshold: 0.001s` + override `{GetData: 3600s}`    | 5-6  |
| `l_rflags` | `zk_rflags` | `enable_per_opcode_response_bytes_metrics: true`                                                          | 7    |

Reference in-container ports (Task 1 pin): `l_resp`=15050, `l_fast`=15051,
`l_slow`=15052, `l_rflags`=15053. Subject ports are `subjListenerPort` + 0/1/2/3.

The proto enum value name for the override opcode is **`GetData`** (the
`LatencyThresholdOverride_Opcode` value 4); reference Envoy v1.37.2 accepts this
YAML spelling on its first `--mode validate` boot — no spelling correction was
needed.

## Deterministic-threshold construction (D-P9)

The responder's fixed 10 ms pre-response delay means every measured latency on
BOTH sides is ≥ 10 ms. Therefore:

- `default_latency_threshold: 3600s` → EVERY response FAST (10 ms ≤ 3600 s; the
  comparison is inclusive). `l_fast` buckets all responses fast.
- `default_latency_threshold: 0.001s` (1 ms) → EVERY response SLOW (10 ms >
  1 ms). `l_slow` buckets all responses slow…
- …EXCEPT the `GetData` override `3600s`, which beats the 1 ms default → the
  override arm's `getdata_resp` buckets FAST.

The 1 ms slow-arm threshold and the 3600 s fast/override threshold straddle the
fixed 10 ms delay with 1000× and 360000× margins, so there is no cross-side
timing nondeterminism.

## Trigger-opcode encoding (D-S28.2-2)

The responder peeks the request opcode (frame bytes 4-8) for data requests:

- **getacl (wire op 6) → wrong xid.** The responder echoes `xid + 1000`, so the
  response is uncorrelated on both sides → `decoder_error` (and `getacl_resp`
  stays 0). The SAME connection then survives and a follow-up `sync` decodes
  normally — the abandon-no-resync recovery proof.
- **exists (wire op 3) → watch-event push.** The responder writes a NORMAL
  correlated response (→ `exists_resp`), THEN an unsolicited watch-event push.
  The push uses the FULL ReplyHeader format (**D-S28.2-1**):
  `xid(−1) | zxid(8) | error(4) | event_type(4) | client_state(4) | path-len(4) |
  path` = 37 bytes, ≥ the 28-byte upstream `parseWatchEvent` minimum. Both
  reference Envoy and envoy-go accept this format and tick `watch_event`. (The
  SPEC's original 16-byte watch-event pin was corrected to upstream's value at
  IMPL; the responder writes the corrected format.)

## Eight-arm taxonomy + expected counters

Arms run in declared order over the shared listeners, so `AssertStats` asserts
**cumulative** values per listener. Each arm WRITES a request frame then READS
the expected number of response frames before proceeding (the round-trip driving
discipline — deterministic cross-side decode ordering + natural backpressure),
and emits a side-independent verdict line so equivalent behavior yields
byte-identical drive output for the runner's `CompareBytes` gate.

- **Arm 1 (round-trips, `l_resp`)** — `connect` + `getdata`(xid 1) + `create`(xid
  2) + `ping` + `close`(xid 3), each answered by exactly 1 response frame
  (`[]int{1,1,1,1,1}`). → `connect_resp`=1, `getdata_resp`=1, `create_resp`=1,
  `ping_resp`=1, `close_resp`=1.
- **Arm 2 (watch event, `l_resp`)** — `exists`(xid 4) → 2 frames (correlated
  response + watch-event push). → `exists_resp`=1, `watch_event`=1.
- **Arm 3 (wrong xid + survival, `l_resp`)** — `getacl`(xid 5) [wrong-xid trigger]
  then `sync`(xid 6) on the SAME connection (`[]int{1,1}`). → `getacl_resp`=0,
  `decoder_error`=1, `sync_resp`=1 (abandon-no-resync recovery).
- **Arm 4 (all-fast, `l_fast`)** — `connect` + `getdata`(1) + `setdata`(2) → every
  response FAST. → `*_resp`=1 + `*_resp_fast`=1 + `*_resp_slow`=0 for each of
  connect/getdata/setdata.
- **Arm 5 (all-slow, `l_slow`)** — `connect` + `setdata`(1) + `delete`(2) → every
  response SLOW. → `*_resp_slow`=1 + `*_resp_fast`=0 for connect/setdata/delete.
- **Arm 6 (override, `l_slow`)** — `getdata`(3) → `getdata_resp_fast`=1 (the
  GetData override 3600 s beats the 1 ms default) while arm 5's ops were slow.
- **Arm 7 (flag-gated resp-bytes, `l_rflags`)** — `connect` + `getdata`(1) →
  `connect_resp`=1, `getdata_resp`=1, `connect_resp_bytes` > 0 (cross-side
  equality), and `getdata_resp_bytes` > 0 (cross-side equality). On `l_resp`
  (flag off) `getdata_resp_bytes` stays 0.
- **Arm 8 (deliberate-break)** — recorded procedure (no live traffic); see the R4
  record below.

The full expected cross-side column (asserted on BOTH sides):

```
l_resp (flags off):
  connect_rq/resp = 1/1   getdata_rq/resp = 1/1   create_rq/resp = 1/1
  ping_rq/resp = 1/1      close_rq/resp = 1/1     exists_rq/resp = 1/1
  watch_event = 1         getacl_rq = 1           getacl_resp = 0
  decoder_error = 1       sync_rq/resp = 1/1
  connect_resp_fast/slow = 0/0   getdata_resp_fast/slow = 0/0  (latency flag off)
  getdata_resp_bytes = 0  (resp-bytes flag off)
l_fast (3600s default → all fast):
  connect_resp = 1, connect_resp_fast/slow = 1/0
  getdata_resp = 1, getdata_resp_fast/slow = 1/0
  setdata_resp = 1, setdata_resp_fast/slow = 1/0
l_slow (1ms default + 10ms delay → all slow; GetData override → fast):
  connect_resp = 1,   connect_resp_slow/fast = 1/0
  setdata_resp = 1,   setdata_resp_slow/fast = 1/0
  delete_resp  = 1,   delete_resp_slow/fast  = 1/0
  getdata_resp = 1,   getdata_resp_fast/slow = 1/0   (the override)
l_rflags (resp-bytes flag on):
  connect_resp = 1   getdata_resp = 1
+ cross-side equality (present + equal + > 0 on both sides):
  zk_resp.request_bytes     zk_resp.response_bytes
  zk_fast.request_bytes     zk_fast.response_bytes
  zk_rflags.connect_resp_bytes   zk_rflags.getdata_resp_bytes
```

## Cross-side equality assertions

Six counters are asserted `ref == subj && ref > 0` (no hardcoded literal): the
`l_resp` `request_bytes` + `response_bytes` wire-footprint sums (arms 1-3 load),
the `l_fast` `request_bytes` + `response_bytes` sums (arm 4 load), and the flag-ON
`zk_rflags.connect_resp_bytes` + `zk_rflags.getdata_resp_bytes` (arm 7). These
prove the byte-accounting agrees cross-side without pinning brittle literals.

The `l_fast` byte-counter entries and the `l_rflags.connect_resp_bytes` entry were
added in the Task 9 review fix (coverage gap: both were exercised but unasserted
in the initial submission).

## StatsAsserter mechanics

`AssertStats` scrapes `/stats/prometheus` from both admin endpoints, retaining
lines whose name contains the `_zookeeper_` infix. The zookeeper counters carry
an EMPTY label set on both sides (no tag extraction), so the driver keys by the
bare flattened name (`envoy_<prefix>_zookeeper_<counter>`) via `lookupZKCounter`.
Present-vs-absent is reported DISTINCTLY from a wrong value: an ABSENT counter
signals a name-shape / eager-creation failure.

## R4 deliberate-break record (both breaks LIVE, both reverted; `-count=1`)

The assertions were proven non-vacuous against the green baseline. BOTH runs used
`go test -count=1` (result caching otherwise serves a stale PASS after a
deliberate break — `differential_break_protocol_count1`).

**Break (a) — wrong expected value.** Edited `{"zk_resp.zookeeper.getdata_resp",
1}` → `2`. The test FAILED on BOTH sides (proving the fixed-value assertion runs
against ref and subject):

```
runner_test.go:1080: ref zk_resp.zookeeper.getdata_resp = 1, want 2
runner_test.go:1080: subj zk_resp.zookeeper.getdata_resp = 1, want 2
--- FAIL: TestDifferential/0048-zookeeper-responses
```

Reverted; the value returned to 1.

**Break (b) — production-side liveness.** Commented out the
`d.countResponse("connect", frame)` call in `onConnectResponse`
(`internal/filter/network/zookeeperproxy/decoder.go`). The SUBJECT side stopped
ticking `connect_resp` while the reference still reported it — a cross-side
divergence proving the SUBJECT assertions are live:

```
runner_test.go:1080: subj zk_resp.zookeeper.connect_resp = 0, want 1
runner_test.go:1080: subj zk_fast.zookeeper.connect_resp = 0, want 1
--- FAIL: TestDifferential/0048-zookeeper-responses
```

(No `ref` errors printed — the reference still counted connect_resp.) Reverted;
`git diff internal/` is empty.

After both reverts the baseline returns GREEN (re-run with `-count=1`).

## R5 ratification

The correlation structures (`requestsByXid` / `controlRequestsByXid`) written at
28.1 are CONSUMED by the 28.2 response decoder; this fixture is the cross-side
proof that the consumption + the latency math + the byte accounting match
reference Envoy v1.37.2 across all four flag configurations. R5 (the 28.1
correlation-write commitment) is ratified by this fixture's GREEN status.

## Cross-references

- phase 28.2 SPEC §5.2 (this fixture's scope) + §3 (the response decoder)
- 28.2 PLAN Task 9 (this file + AssertStats + 8 arms)
- ADR-0223 (zookeeper_proxy response decoder); D-S28.2-1 (watch-push format);
  D-S28.2-2 (trigger encoding); D-P9 (deterministic-threshold construction)
- fixture-0046-zookeeper-requests (the request-side template + TCPSink rationale)
- project memory `reference_differential_asserter_dispatch` /
  `reference_differential_fixture_dispatch_constraint` /
  `differential_break_protocol_count1`
