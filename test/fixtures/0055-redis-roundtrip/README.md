# 0055-redis-roundtrip

Cross-side differential fixture for the `redis_proxy` TERMINAL network filter (phase 32.1).

## Scope

Both the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) and the
envoy-go subprocess run a `[redis_proxy]` TERMINAL listener (NO `tcp_proxy`
behind it). The driver acts as a RESP client and drives the same requests
against both sides; the runner asserts byte-equivalent responses AND stat parity
(`StatsAsserter`).

Per the cross-side-XOR-boot-reject constraint
(`reference_differential_fixture_dispatch_constraint`), ALL cross-side arms
share this ONE directory. Phase 32.1 lands arms 1–2 (PING local-reply + proxied
SET/GET); phase 32.2 extends with the full command matrix + splitter arms.

## Bootstrap (both sides)

```
listener l_redis
  filter_chain: [redis_proxy TERMINAL]
    stat_prefix: redis_r
    settings.op_timeout: 5s
    prefix_routes.catch_all_route.cluster: redis_cluster

cluster redis_cluster → TCPRedisResponder backend (runner-allocated)
```

Reference side uses `STRICT_DNS` + `host.docker.internal`; subject side uses
`STATIC` + `127.0.0.1`.

## Arms (phase 32.1)

| Arm | Description | Upstream hit? |
|-----|-------------|---------------|
| 1 — `ping` | `inline("PING")` + `respArray("PING")` on one conn; both replies must be `+PONG\r\n` byte-identical cross-side | No (local-reply, AMEND-R5) |
| 2 — `set-get` | `respArray("SET","foo","bar")` + `respArray("GET","foo")` on one conn; SET → `+OK\r\n`, GET → `$3\r\nbar\r\n` byte-identical cross-side | Yes (lazy dial on SET; 1 upstream cx, 2 upstream rq) |

## Assertion prongs (§8.1.1 — the §9 FIRST)

1. **Byte-equivalence prong** (`CompareBytes`): `DriveReference` and
   `DriveSubject` return the raw reply bytes from both arms. The runner's
   `CompareBytes` proves the proxy GENERATES identical downstream responses on
   both sides:
   - PING arm: `+PONG\r\n+PONG\r\n` (inline + array PING, two replies)
   - SET/GET arm: `+OK\r\n$3\r\nbar\r\n` (SET then GET replies)

2. **Stat parity prong** (`StatsAsserter`, Task 14): scrapes FLAT `/stats`
   (internal-name text, NOT `/stats/prometheus` — the `redis.` Prometheus
   tag-extractor arm is 32.2) from both admin endpoints and asserts the
   following counters are cross-side equal:

   | Counter name | Actual value | Explanation |
   |---|---|---|
   | `redis.redis_r.downstream_cx_total` | 2 | one conn per arm |
   | `redis.redis_r.downstream_rq_total` | **4** | inline+array PING + SET + GET |
   | `redis.redis_r.downstream_cx_rx_bytes_total` | (equal) | bytes from downstream |
   | `redis.redis_r.downstream_cx_tx_bytes_total` | (equal) | bytes to downstream |
   | `cluster.redis_cluster.upstream_cx_total` | **1** | one lazy dial (SET is first proxied) |
   | `cluster.redis_cluster.upstream_rq_total` | **2** | SET + GET forwarded upstream |

## R6 Deliberate-break liveness proof (PLAN Task 14 Step 4)

All assertions were verified LIVE by temporarily perturbing the driver (with
`-count=1` to defeat go-test caching;
`reference_differential_break_protocol_count1`):

| Break | Perturbation | FAIL output | Reverted → PASS |
|---|---|---|---|
| 1 | `want downstream_rq_total==99` | `R6-BREAK ref redis.redis_r.downstream_rq_total = 4, want 99` | ✓ |
| 2 | `want upstream_rq_total==99` | `R6-BREAK ref cluster.redis_cluster.upstream_rq_total = 2, want 99` | ✓ |
| 3 | Append `'!'` to subj SET/GET reply bytes | `differential mismatch: first divergence at offset 28` | ✓ |
| 4 | Append `'X'` to subj PING reply bytes | `differential mismatch: first divergence at offset 14` | ✓ |
| 5 | `want upstream_cx_total==99` | `R6-BREAK ref cluster.redis_cluster.upstream_cx_total = 1, want 99` | ✓ |

## RESP request builders (D-S32.1-4)

```go
// Shared with the 32.2 command-matrix arms (same package).
func respBulk(s string) []byte          // "$<len>\r\n<bytes>\r\n"
func respArray(parts ...string) []byte  // "*<n>\r\n" + respBulk per part
func inline(s string) []byte            // "<text>\r\n"
```

## Cross-side divergences found

None. All 6 asserted counters are equal cross-side (reference Envoy v1.37.2
`redis_proxy` TERMINAL and envoy-go `redis_proxy` TERMINAL produce identical
downstream RESP responses and identical `redis.*` + `cluster.*` stat values).

Note: the subject admin exposes `/stats` (flat `name: value` text) via the
`internal/admin/stats.go` handler added at phase 32.1 (D-S32.1-5). This is
the direct flat-stats scrape surface used by this fixture's `AssertStats`;
`/stats/prometheus` (which skips `redis.*` stats since the `redis.` Prometheus
tag-extractor arm lands at 32.2) is NOT used here.
