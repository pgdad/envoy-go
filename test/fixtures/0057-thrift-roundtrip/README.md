# 0057-thrift-roundtrip

Cross-side differential fixture for the `thrift_proxy` TERMINAL network filter (phase 33).

## Scope

Both the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) and the
envoy-go subprocess run a `[thrift_proxy]` TERMINAL listener (NO `tcp_proxy`
behind it). The driver acts as a framed-binary Thrift client and drives the same
CALL frames against both sides; the runner asserts byte-equivalent downstream
reply frames AND stat parity (`StatsAsserter`).

Per the cross-side-XOR-boot-reject constraint
(`reference_differential_fixture_dispatch_constraint`), ALL cross-side arms share
this ONE directory. The boot-reject arm (missing `stat_prefix`) lives in the
separate `0058-thrift-boot-reject` dir (Task 13).

## Bootstrap (both sides) — SPEC §11.2 working YAML, verbatim

```
listener l_thrift
  filter_chain: [thrift_proxy TERMINAL]
    stat_prefix: thrift_r
    transport:   FRAMED
    protocol:    BINARY
    payload_passthrough: true
    route_config:
      routes:
        - { match: { method_name: "Ping" }, route: { cluster: thrift_cluster } }
        - { match: { method_name: "boom" }, route: { cluster: thrift_cluster } }

cluster thrift_cluster → TCPThriftResponder backend (runner-allocated, Task 11)
```

Reference side uses `STRICT_DNS` + `host.docker.internal`
(`reference_docker_probe_bridge_network`); subject side uses `STATIC` +
`127.0.0.1`.

### Route-table design (method-keyed, NO match-all)

The route table is DELIBERATELY method-keyed with NO match-all (`""`) route so
that exactly one method MISSES:

| Method | Routes? | Outcome |
|--------|---------|---------|
| `Ping` | HIT (→ `thrift_cluster`) | round-trip → backend REPLY void-success (arm 1) |
| `boom` | HIT (→ `thrift_cluster`) | round-trip → backend EXCEPTION reply (arm 3) |
| `Pong` | MISS (matches neither) | local `UnknownMethod` exception, NO dial (arm 2) |

`boom` MUST be a HIT route because the reply-EXCEPTION arm needs the request to
REACH the backend (the `TCPThriftResponder` answers an EXCEPTION only for the
in-band marker method name `"boom"`, Task 11 / `thriftMarkerException`). A
match-all route would make NOTHING miss, so the table is exact-match only.

## Arms

Each arm opens a **fresh connection**, writes ONE framed-binary CALL, reads ONE
complete framed-binary reply frame (4-byte BE length prefix + payload), and emits
the FULL reply frame bytes (the cross-side byte-equivalence signal — the framed
single-flight one-call-per-connection MVP contract, SPEC §8 caveat (iii)).

| Arm | Request | Downstream reply (byte-identical cross-side) | Upstream? |
|-----|---------|----------------------------------------------|-----------|
| 1 — `hit` | CALL `Ping` seq 1 | framed-binary REPLY (msgtype 2, method `Ping`, seq 1, single-STOP void body) | Yes (HIT round-trip) |
| 2 — `miss` | CALL `Pong` seq 2 | local `UnknownMethod` EXCEPTION (msgtype 3, AppException `{1: "no route for method 'Pong'", 2: i32 1}`) | No (local reply) |
| 3 — `reply-exception` | CALL `boom` seq 3 | framed-binary EXCEPTION (msgtype 3, AppException `{1: "backend exception", 2: i32 6}`, echoing `boom`/seq 3) | Yes (HIT → backend EXCEPTION) |

## Assertion prongs (the load-bearing two-pronged proof — SPEC §8)

1. **Byte-equivalence prong** (`CompareBytes`): `DriveReference` and `DriveSubject`
   return the concatenated downstream reply frames from all three arms. The
   runner's `CompareBytes` proves the proxy GENERATES byte-identical downstream
   Thrift responses on both sides — the void-success REPLY (arm 1), the local
   `UnknownMethod` EXCEPTION (arm 2), and the backend EXCEPTION (arm 3). All three
   passed byte-identical with NO `name.go`/encoder reconciliation needed (the
   SPEC §11.2/Appendix A wire layout matched the reference verbatim).

2. **Stat parity prong** (`StatsAsserter`): scrapes FLAT `/stats` from both admin
   endpoints and asserts the `thrift.thrift_r.*` + `cluster.thrift_cluster.*`
   roster, partitioned into cross-equal vs per-side below.

### Cross-side EQUAL counters

| Counter | Value | Source |
|---|---|---|
| `thrift.thrift_r.response` | 2 | Ping REPLY + boom EXCEPTION (the Pong miss has no upstream response) |
| `thrift.thrift_r.response_reply` | 1 | Ping REPLY (boom is an EXCEPTION) |
| `thrift.thrift_r.response_success` | 1 | Ping void-success |
| `thrift.thrift_r.response_passthrough` | 2 | Ping + boom upstream replies |
| `thrift.thrift_r.response_exception` | **2** | Pong LOCAL miss + boom BACKEND exception (the two distinct exception paths) |
| `thrift.thrift_r.route_missing` | 1 | Pong |
| `cluster.thrift_cluster.upstream_rq_total` | 2 | Ping + boom (request COUNT is pooling-independent) |
| `thrift.thrift_r.request_active` | 0 | quiesced post-workload (D-S33-3; PRESENT && == 0 both sides) |

The arm-3 `response_exception == 2` is the load-bearing D-S33-2 result: it proves
the BACKEND-reply exception path (arm 3 `boom`) increments `response_exception`
distinctly from the LOCAL route-miss exception (arm 2 `Pong`) — together they sum
to 2 on both sides.

### Per-side divergences (NOT cross-equal)

| Counter | ref | subj | Why |
|---|---|---|---|
| `thrift.thrift_r.request` / `request_call` / `request_passthrough` | 2 | 3 | the reference does NOT count `request*` on a routing MISS (SPEC §7.3 / D-T5 — the miss is accounted only via `route_missing`+`response_exception`); the subject's pump increments `request*` at the TOP of `serveRequest`, BEFORE the route match (PLAN Task 8 "decode → count request → match route"), so it counts all three calls. A documented per-side behavioral divergence, NOT a subject bug. |
| `cluster.thrift_cluster.upstream_cx_total` | 1 | 2 | the reference POOLS upstream connections at the cluster level → one reused conn serves Ping + boom → 1. The subject uses the one-conn-per-downstream upstream seam → the 2 distinct HIT downstream conns each dial 1 upstream → 2. The redis D-P32-9 / D-T9b pooling precedent. (`upstream_rq_total == 2` is pooling-independent and stays cross-equal above.) |
| `thrift.thrift_r.cx_destroy_local_with_active_rq` | 1 | 0 | the miss-arm close-direction boundary (below) |
| `thrift.thrift_r.downstream_response_drain_close` | 1 | 0 | the miss-arm close-direction boundary (below) |

#### Per-side close-direction coverage boundary (arm 2 — `reference_close_direction_framework_gap` / AMEND-T6 / D-T8)

On the MISS arm the REFERENCE moves `cx_destroy_local_with_active_rq` +
`downstream_response_drain_close` (it `FlushWrite`-closes after the local
exception with the rq active). The SUBJECT keeps its local-reply connection OPEN
(no drain-close — the network framework records close TYPE not DIRECTION), so both
stay 0. These are asserted **subject == 0 only** (the reference legitimately moves
them; NOT cross-equal). The subject `== 0` pin is non-vacuous: the counters are
eager-created (present-at-0) so the pin proves the subject renders them AND never
spuriously incremented them.

### Decode-ran witness (SPEC §8 caveat (i))

`thrift_proxy` emits NO listener `downstream_cx_rx_bytes_total` (that is an HCM
stat). The round-trip-ran witnesses:

- **Reference:** `cluster.thrift_cluster.upstream_cx_rx_bytes_total > 0`
  (observed 73 — the cluster received reply bytes from the backend). The
  **subject's** cluster package does NOT emit `upstream_cx_rx_bytes_total` (SPEC
  §7.5 — the subject reuses only `upstream_cx_total`/`upstream_rq_total`), so this
  byte witness is reference-side only.
- **Both sides:** `thrift.thrift_r.request_call > 0` (a CALL was decoded +
  serviced).

## Frame builders (Appendix A)

```go
// DUPLICATED here, NOT imported from internal/filter/network/thriftproxy
// (the 0055 self-contained redis-builder precedent + the runner's thriftReqFrame).
func thriftCallFrame(method string, seqID int32) []byte // framed CALL: 4-byte BE len + 0x8001 00 01 + i32 namelen + name + i32 seq + STOP
```

The reply frames (void-success REPLY, backend EXCEPTION) are built by the
`TCPThriftResponder` backend in the runner; the local `UnknownMethod` EXCEPTION is
built by the subject's `encodeUnknownMethod` (and the reference's `sendLocalReply`)
— the driver only reads/compares those reply frames.

## Deliberate-break liveness proof (PLAN Task 12 Step 4)

Per `reference_differential_break_protocol_count1` (go-test caching defeated with
`-count=1`).

| Break | Perturbation | FAIL output | Restore → PASS |
|---|---|---|---|
| Production — `response_success` | commented out `f.st.inc("response_success")` in `internal/filter/network/thriftproxy/filter.go` (the HIT-path void-success inc) | `cross-side mismatch thrift.thrift_r.response_success: ref=1 subj=0` + `subj thrift.thrift_r.response_success = 0, want 1` | ✓ |

This was a PRODUCTION break (not a driver perturbation), proving the cross-side
`StatsAsserter` is wired to the REAL subject counter (`filter.go`), not a vacuous
driver-side echo. After restoring the production line, `git diff` showed
`internal/filter/network/thriftproxy/filter.go` UNCHANGED and the `0057`
differential PASSES.

## Cross-side divergences found (summary)

Reference Envoy v1.37.2 `thrift_proxy` TERMINAL and envoy-go `thrift_proxy`
TERMINAL produce **byte-identical downstream Thrift responses** for all three arms
(void-success REPLY, local `UnknownMethod` EXCEPTION, backend EXCEPTION) with NO
encoder reconciliation. The stat divergences are the documented per-side
boundaries:

1. **`request*` miss-counting** (ref 2 / subj 3): the reference does not count a
   miss as a serviced request; the subject's count-before-match pump does.
2. **`upstream_cx_total` pooling** (ref 1 / subj 2): the reference pools upstream
   conns; the subject is one-conn-per-downstream (D-T9b / redis D-P32-9). The
   `upstream_rq_total` request count stays cross-equal.
3. **The 2 close-direction counters** (ref 1 / subj 0 on the miss arm): the
   framework-gap coverage boundary (AMEND-T6 / D-T8).

No real subject bug was found.

Note: the subject admin exposes `/stats` (flat `name: value` text) via the
`internal/admin/stats.go` handler; the gauge `thrift.thrift_r.request_active`
renders flat (`request_active: 0`) and is asserted quiesced.
