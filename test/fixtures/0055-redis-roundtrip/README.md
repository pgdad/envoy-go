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

## Arms (phase 32.2 — the §8.1 command matrix, Task 10)

Each 32.2 arm opens a **fresh connection**, writes ONE request, reads ONE
single-frame reply, and emits the reply bytes (the cross-side byte-equivalence
signal — both sides MUST produce identical bytes). The expected replies are the
upstream-faithful local-reply / proxied-reply wordings `classify` produces.

| Arm | Request | Expected reply (byte-identical cross-side) | Upstream hit? | Stat effect |
|-----|---------|--------------------------------------------|---------------|-------------|
| `get-miss` | `respArray("GET","nope")` | `$-1\r\n` (null bulk) | Yes (proxied) | `command.get.total/success` +1 |
| `incr` | `respArray("INCR","ctr")` | `:1\r\n` | Yes (proxied) | `command.incr.total/success` +1 |
| `del` | `respArray("DEL","foo")` | `:1\r\n` | Yes (proxied) | `command.del.total/success` +1 |
| `echo` | `respArray("ECHO","hi")` | `$2\r\nhi\r\n` | No (local) | none |
| `echo-arity` | `respArray("ECHO")` (arity 1) | `-invalid request\r\n` | No (splitter reject) | `splitter.invalid_request` +1 |
| `quit` | `respArray("QUIT")` | `+OK\r\n` then conn CLOSE (`closeAfter`) | No (local) | none |
| `hello-3` | `respArray("HELLO","3")` | `-NOPROTO unsupported protocol version\r\n` | No (local error) | none |
| `hello-options` | `respArray("HELLO","2","AUTH","u","p")` (>2 args) | `-ERR HELLO options like AUTH and SETNAME are not supported\r\n` | No (local error) | none |
| `unknown` | `respArray("BOGUSCMD","x")` | `-ERR unknown command 'BOGUSCMD', with args beginning with: x\r\n` | No (splitter reject) | `splitter.unsupported_command` +1 |
| `ping-arg` | `respArray("PING","hello")` | `+PONG\r\n` | No (local) | none |
| `held-open` (Task 11) | `respArray("PING")` on a conn left OPEN across the scrape | `+PONG\r\n` (read, not closed) | No (local) | `downstream_cx_active` held at 1; `downstream_cx_total`/`downstream_rq_total` +1 (still cross-side equal) |

### Lifecycle gauge arm (phase 32.2 — Task 11, §8.2)

`downstream_cx_active` is a GAUGE reflecting LIVE downstream connections. All the
transient matrix arms open+close fresh conns, so after they finish each side's
`cx_active` returns toward 0. The **held-open arm** opens ONE more connection
LAST, sends `respArray("PING")` (a local reply — no upstream dial, no proxied
command), reads the `+PONG\r\n`, and does NOT close it. At scrape time exactly
ONE downstream connection is live per side → `downstream_cx_active == 1` on BOTH
sides (the mongo `op_query_active` 29.2 held-arm precedent). The held conns are
closed in `AssertStats` right after the gauge assertion (a `defer` guards a
mid-assertion fatal — `fixture.TB` has no `Cleanup`).

Asserted via `/stats/prometheus` (the `redis.` tag-extractor renders gauges with
a `# TYPE … gauge` line; `scrapeProm` reads gauge values identically to counters):

| Prom gauge | Asserted | Explanation |
|---|---|---|
| `envoy_redis_downstream_cx_active` | **== 1** BOTH sides | the held-open conn parked across the scrape |
| `envoy_redis_downstream_rq_active` | PRESENT **&& == 0** BOTH sides | quiesced post-workload (the held PING reply drained within `settleDelay`) |
| `envoy_redis_downstream_cx_rx_bytes_buffered` | **subject == 0** only | subject-side coverage boundary (see below) |
| `envoy_redis_downstream_cx_tx_bytes_buffered` | **subject == 0** only | subject-side coverage boundary (see below) |

**Buffered-gauge coverage boundary (Task 11).** The held-open arm leaves the
REFERENCE's `downstream_cx_rx_bytes_buffered == 14` (== `len(respArray("PING"))`,
the held conn's still-buffered request frame) — NOT 0. The subject never wires
the 2 buffered gauges (`filter.go` inc/decs only `cx_active` + `rq_active`), so
they pin at 0. Buffered is therefore NOT cross-side equal; the assertion pins the
SUBJECT `== 0` only (a `close_direction`-style framework coverage boundary —
`reference_close_direction_framework_gap`). The reference is intentionally not
asserted. The subject `== 0` pin is non-vacuous: it proves the subject renders
the gauge AND has not spuriously incremented it.

### Per-command / splitter accounting (cumulative with the 32.1 SET + GET arm)

Asserted cross-side **equal** via `/stats/prometheus` (the `redis.` tag-extractor
arm hoists `stat_prefix` to a single `envoy_redis_prefix` label and flattens the
command/splitter tail into the metric NAME):

| Prom metric | Value | Source arms |
|---|---|---|
| `envoy_redis_command_get_total` / `_success` | **2** | 32.1 `GET foo` (hit) + `get-miss` `GET nope` |
| `envoy_redis_command_set_total` / `_success` | 1 | 32.1 `SET foo bar` |
| `envoy_redis_command_incr_total` / `_success` | 1 | `incr` |
| `envoy_redis_command_del_total` / `_success` | 1 | `del` |
| `envoy_redis_splitter_invalid_request` | 1 | `echo-arity` |
| `envoy_redis_splitter_unsupported_command` | 1 | `unknown` |

(All seven prom metrics carry the single `{envoy_redis_prefix="redis_r"}` label
and are byte-identical cross-side — NO `name.go` reconciliation was needed; the
SPEC §5 design matched the reference Envoy verbatim.)

### D-S32.2-2 — confirmed UNKNOWN-arm reference bytes

The `unknown` arm reconciled the `-ERR unknown command ...` wording LIVE against
the contrib reference Envoy. **Confirmed reference bytes** (Go `%q`):

```
-ERR unknown command 'BOGUSCMD', with args beginning with: x\r\n
```

The reference appends the request ARGUMENTS (`args[1:]`, here the single `"x"`,
joined by `", "`) verbatim after `beginning with: ` — NOT an empty suffix.
`internal/filter/network/redisproxy/commands.go`'s `unknownCommandError` was
corrected in lockstep to join `args[1:]` with `", "` (and its unit-test
expectation in `commands_test.go` updated to match). The wire is shared, so the
subject now matches the reference byte-for-byte
(`reference_wire_format_both_sides_see_same_bytes`).

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

### R6 Deliberate-break liveness proof (phase 32.2 — Task 11)

The Task-11 prom/gauge assertions were each proven LIVE with `-count=1`:

| Break | Perturbation | FAIL output | Reverted → PASS |
|---|---|---|---|
| A | scraped `subjP[command_incr_total]=99999` before the promEqual loop | `cross-side mismatch envoy_redis_command_incr_total{envoy_redis_prefix="redis_r"}: ref=1 subj=99999` | ✓ |
| B | scraped `subjP[splitter_unsupported_command]=77777` before the loop | `cross-side mismatch envoy_redis_splitter_unsupported_command{envoy_redis_prefix="redis_r"}: ref=1 subj=77777` | ✓ |
| C | closed `d.subjHeld` just before the `refP` scrape | `subj: downstream_cx_active = 0, want 1 (held-open arm)` | ✓ |
| D | append `'!'` to subj `incr` reply bytes (new-arm CompareBytes prong) | `differential mismatch: first divergence at offset 37` | ✓ |
| E | subj `upstream_cx_total` per-side `want` 4→99 | `subj cluster.redis_cluster.upstream_cx_total = 4, want 99` | ✓ |

All five were DRIVER-only perturbations (no production file touched); each
reverted and the post-revert `0055` differential PASSES.

## RESP request builders (D-S32.1-4)

```go
// Shared with the 32.2 command-matrix arms (same package).
func respBulk(s string) []byte          // "$<len>\r\n<bytes>\r\n"
func respArray(parts ...string) []byte  // "*<n>\r\n" + respBulk per part
func inline(s string) []byte            // "<text>\r\n"
```

## Cross-side divergences found

**`cluster.redis_cluster.upstream_cx_total` — per-side pin (32.2, Task 10).** Once
the 32.2 command-matrix runs each proxied command on its OWN fresh downstream
connection, the upstream-connection counts diverge architecturally:

- **Reference** pools upstream connections at the cluster level → ONE reused
  upstream connection serves all 5 proxied requests → `upstream_cx_total == 1`.
- **Subject** uses a ONE-CONN-PER-DOWNSTREAM upstream seam (`filter.go`: each
  downstream connection lazily dials its own dedicated upstream, NO cross-conn
  pool) → the 4 distinct proxied-command downstream conns (SET arm-2, `get-miss`,
  `incr`, `del`) each dial 1 upstream → `upstream_cx_total == 4`.

This is DETERMINISTIC per side and is pinned with an EXACT per-side value (ref 1,
subj 4) — the 0053 abandon-at-close per-side-pin precedent
(`reference_close_direction_framework_gap`). The request COUNT
(`upstream_rq_total == 5`) is pooling-independent and stays cross-side equal.

**`unknown` arm wording (D-S32.2-2).** The reference echoes the request arguments
after `with args beginning with: ` (see the D-S32.2-2 block above);
`unknownCommandError` was corrected in lockstep. After this fix all byte-equivalence
and per-command/splitter prom assertions are equal cross-side.

Otherwise reference Envoy v1.37.2 `redis_proxy` TERMINAL and envoy-go `redis_proxy`
TERMINAL produce identical downstream RESP responses and identical `redis.*` +
`command.*` + `splitter.*` stat values.

Note: the subject admin exposes `/stats` (flat `name: value` text) via the
`internal/admin/stats.go` handler added at phase 32.1 (D-S32.1-5) — used for the
`redis.*` / `cluster.*` flat counters. As of 32.2 (Task 10) `AssertStats` ALSO
scrapes `/stats/prometheus` for the per-command / splitter cross-side equality
(the `redis.` Prometheus tag-extractor arm landed at 32.2, Task 7).
