# 0080-h2-goaway-rotation — graceful GOAWAY-driven H2 upstream connection rotation (cross-side EXACT)

Phase 43.2b (`docs/envoy-go/phases/43.2-h2-connection-pool/`) — the BEHAVIORAL
proof of the HTTP/2 upstream multiplex pool's **graceful GOAWAY drain lifecycle**.
It validates, cross-side against reference Envoy `contrib-v1.37.2` over a Docker
bridge, that a GOAWAY'd pooled upstream connection is admission-skipped, drains,
and closes — with a lazy replacement dial — plus the two behavior-driven
`http2.*` reset counters.

## Purpose

When a pooled upstream H2 connection receives a peer `GOAWAY`, the pool marks it
**draining**: it takes NO new streams (admission-skip), drains its in-flight
streams, then closes — an idle draining conn is closed PROMPTLY by a per-conn
watcher, an in-flight one closes on its last stream release. The replacement opens
LAZILY on the next demand (`upstream_cx_http2_total` ++). Two reset counters are
also exercised: `http2.rx_reset` (a RST_STREAM the codec RECEIVES) and
`http2.tx_reset` (a RST_STREAM(CANCEL) the codec SENDS).

## Cross-side EXACT — why (`reference_h2_goaway_rotation_stats`)

The SPEC-43.2b §11 live probe (2026-06-24) confirmed the reference observes a peer
GOAWAY ENTIRELY via the `upstream_cx_*` lifecycle counters — **no `http2.*`
counter moves during a rotation**, and there is **no `goaway_received` counter**.
Only `http2.rx_reset` + `http2.tx_reset` of the `http2.*` family are live. Driven
as discrete poll-to-converge phases behind release barriers, the rotation conn
count + the reset counters are deterministic, so they are asserted **cross-side
EXACT on both sides** (AMEND-H2B-4):

```
cluster.c_h2gw.upstream_cx_http2_total == 2   (one rotation: orig + replacement)
cluster.c_h2gw.http2.streams_active    == 0   (at quiesce)
cluster.c_h2gw.http2.rx_reset          == 1   (one backend RST_STREAM)
cluster.c_h2gw.http2.tx_reset          == 1   (one upstream CANCEL)
```

### D-H2B-CXSTATS — assertion scope

Only the `upstream_cx_*` / `http2.*` counters **both** sides emit are
scraped/asserted: `upstream_cx_http2_total`, `upstream_cx_active`,
`http2.streams_active`, `http2.rx_reset`, `http2.tx_reset`. `cx_close_notify` /
`cx_destroy_local` are NOT asserted (envoy-go does not emit them). Confirmed
in-task: a useH2 cluster in envoy-go registers exactly `upstream_cx_total`,
`upstream_cx_active`, `upstream_cx_http2_total`, `http2.streams_active`,
`http2.rx_reset`, `http2.tx_reset` (`internal/cluster/manager.go`).

## The H2-downstream gate (`reference_h2_pool_downstream_codec_gate`)

The HCM selects the H1-vs-H2 router action by the **downstream** listener codec.
The H2 upstream multiplex pool — and therefore the GOAWAY drain lifecycle — engages
ONLY on an H2 (TLS+ALPN-h2) downstream listener (there is no H1-down→H2-up bridge
in envoy-go). So the `0080` listener is the `0004`/`0079` H2 PKI shape; an
H1-downstream variant would SILENTLY never exercise the pool. The Step-1
decode-ran guard (`upstream_cx_http2_total > 0`) catches a disengaged pool.

## Topology — one cluster, one backend

| Cluster   | upstream | `C` (max_concurrent_streams) | routes |
|-----------|----------|------------------------------|--------|
| `c_h2gw`  | h2c (no `transport_socket`) | 100 (never binds) | `/tx` (per_try_timeout 0.5s, num_retries 0) + `/*` (no timeout) |

`C=100` is HIGH so it never binds: a single held stream + the control request share
ONE upstream conn — the rotation is about conn **identity** (the GOAWAY drives the
multi-conn count), not the local stream cap. Subject is STATIC / `127.0.0.1`;
reference is STRICT_DNS / `host.docker.internal` (the shared-bridge shape,
`reference_docker_probe_bridge_network`).

The backend (`H2GoawayResponder`, BackendKind 38, Task 7) is a raw-framer h2c
responder advertising `SETTINGS_MAX_CONCURRENT_STREAMS=1000 >> C` that HOLDS each
request stream and, on control requests routed **through the proxy**:

- `/__release` — **BROADCAST**: 200 to every held stream on every live conn. A
  per-conn release could never reach a held stream on a DRAINING conn (the proxy
  never routes a new request to a draining conn — it is admission-skipped), so the
  release broadcasts (matching the SPEC live-probe backend + the kind-37
  `acceptH2Hold` process-global gate). This task added the broadcast to the
  landed Task-7 backend.
- `/__goaway` — a per-conn `GOAWAY(NO_ERROR)` naming the highest stream id (so no
  in-flight stream is abandoned); the conn STAYS open (graceful drain).
- `/__rst` — a per-conn `RST_STREAM(INTERNAL_ERROR)` on the lowest held stream;
  the control 200 is answered FIRST so the conn-eviction does not race it.

## The six-stage barrier drive (SLEEPLESS — poll-to-converge + release-barrier; sequential-per-side)

All work runs inside `AssertStats`, SUBJECT fully then REFERENCE
(`reference_concurrency_differential_release_barrier`). Synchronization is
poll-the-gauge until convergence + the backend's broadcast `/__release` — NEVER a
`time.Sleep` (the `per_try_timeout` in Step 5 IS the mechanism under test, not a
sync sleep).

1. **Establish** — fire 1 held `GET /` → poll `upstream_cx_http2_total == 1` AND
   `http2.streams_active == 1`.
2. **Drain (in-flight)** — `/__goaway` (rides the in-flight conn A → draining); a
   2nd held `GET` MISSes A → REPLACEMENT dial → poll `upstream_cx_http2_total == 2`
   (the headline rotation count, pinned from a 0 base); `/__release` (broadcast) →
   both held drain to 200 → A closes on its last release → poll
   `http2.streams_active == 0` AND `upstream_cx_active == 1`.
3. **Drain (idle)** — fire+release 1 `GET /` (conn idle), `/__goaway` → the
   per-conn watcher closes it PROMPTLY → poll `upstream_cx_active == 0`.
4. **rx_reset** — fire 1 held `GET /`, `/__rst` on it → poll `http2.rx_reset == 1`
   + the DOWNSTREAM request observes a **502** (assert the DOWNSTREAM class, not
   the upstream class — `reference_concurrent_attempt_downstream_class_assertion`).
5. **tx_reset** — fire 1 held `GET /tx/` (per_try_timeout route) → the per-try
   timeout cancels the upstream attempt ctx → the codec emits `RST_STREAM(CANCEL)`
   → poll `http2.tx_reset == 1` + a downstream **504**. (envoy-go has NO
   downstream-cancel→upstream-cancel propagation — the H2 server dispatches each
   stream on the connection-level ctx — so `per_try_timeout` is the faithful
   tx_reset trigger; the reference emits the same upstream CANCEL on a per-try
   timeout, so the count stays cross-side EXACT.)
6. **Quiesce** — `/__release` (broadcast) → remaining held drain → poll
   `http2.streams_active == 0`. (`upstream_cx_active` is NOT pinned here: the
   `/__release` control request leaves one idle pooled conn, so it settles to 1;
   the `cx_active == 0` close was observed at the idle-drain prong, Step 3.)

## Running

```bash
go test ./test/differential/ -run 'TestDifferential/0080' -count=1
```

(Requires Docker for the reference container. Use the
`-run 'TestDifferential/0080'` selector — a bare `-run '0080'` matches zero
subtests, `reference_differential_run_selector`. Always `-count=1`. A transient
`subject ready: EOF` is a subprocess startup race, NOT a regression —
isolate-re-run to tell it apart, `reference_differential_fullsuite_startup_flake`.)

## Config-as-Go-string

Per the differential-fixture convention there are NO standalone `envoy.yaml` /
`envoy-go.yaml` files — the two bootstraps are returned as Go string templates
from `driver.ReferenceBootstrap` / `driver.SubjectConfig` (the 0079 mechanism).
The single-sourced workload constants (`streamCap`, `cluster`,
`refContainerListenerPort`) live in `driver/driver.go` and are pinned by
`driver/driver_test.go::TestConstants` (`reference_fixture_workload_constant_desync`).
The fixture reuses the `0004`/`0079` H2 TLS+ALPN-h2 downstream PKI (`pki/`).
