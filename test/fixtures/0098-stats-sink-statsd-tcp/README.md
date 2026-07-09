# 0098-stats-sink-statsd-tcp

Differential for the phase 55 statsd **TCP** line-protocol stats sink
(ADR-0272). Same statsd wire format as the phase 48 UDP sink (`0092`), but
carried over a long-lived **TCP connection** the proxy opens by dialing a named
cluster (`StatsdSink.statsd_specifier.tcp_cluster_name`) instead of a
connectionless UDP socket. Subject envoy-go (in-process) vs reference Envoy
(`contrib-v1.37.2`, Docker).

## What it asserts

### Cross-side parity (both sides)

Identical to `0092`. A name-SUBSET, NOT the whole line set (the reference emits
only USED stats — many more lines including `|ms` timer histograms envoy-go
lacks — so the sets differ structurally; assert NAMED SUBSETS only,
`reference_stats_sink_emits_used_only`). Names are **prefix-joined**: the
receiver keys on the full `<prefix>.<name>` line.

**Counter subset** — on BOTH sides, for each of:

- `sdpfx.cluster.c_backend.upstream_rq_total`
- `sdpfx.http.hcm_local.downstream_rq_total`
- `sdpfx.http.hcm_local.downstream_rq_2xx`

the name is present (decode ran — a zero-line pass is structurally impossible)
and its per-flush `|c` delta-**SUM** `== 7 == K`, and is **still** `7` after
`>= 2` further flushes (the stability barrier — see below).

**Absolute gauge subset** — on BOTH sides:

- `sdpfx.cluster.c_backend.membership_total == 1`

`membership_total`, not `membership_healthy` — envoy-go registers the latter
only on clusters WITH `health_checks`, and `c_backend` has none
(`reference_membership_total_vs_healthy_gauge`).

### Subject-exact transport (three assertions the UDP fixture cannot make)

The TCP receiver observes the connection, so `0098` adds three assertions on the
**subject only**. The reference's values are **RECORDED** (via `log.Printf` —
`fixture.TB` has no `Logf`), never asserted.

- **`ConnCount == 1`** — envoy-go opens exactly ONE long-lived connection (no
  per-flush redial). Cross-side equality is infeasible **by the histogram
  boundary** (`AMEND-TCP-CONNCOUNT`): the reference opens a SECOND, `|ms`-only
  worker-timer connection that envoy-go (no histograms, ADR-0060) can never
  open, so the reference's `ConnCount` is 2. Not uncertainty — a structural gap.
- **`UnparsedCount == 0`** — envoy-go emits only `|c` and `|g` lines, every one
  `\n`-**TERMINATED**. A non-zero count is the signature of `\n`-**SEPARATED**
  (not terminated) framing concatenating two lines across a flush boundary. This
  is the **liveness signal** for the controller's later deliberate framing
  break. The reference legitimately emits ~35 `|ms` lines, so its unparsed count
  is recorded, not asserted (`reference_framing_break_needs_unparsed_counter`).
- **`sdpfx.cluster.c_statsd.upstream_cx_total == 0`** — `DialSink` took the
  **UNACCOUNTED** dial path (`AMEND-TCP-CXSTATS`): no `max_connections` permit,
  no `upstream_cx_*` accounting (`reference_cluster_sink_dial_unaccounted`).
  Subject-only: the reference never emits this line at all (`AMEND-TCP-USEDONLY`
  — it omits never-incremented counters), while envoy-go registers
  `upstream_cx_total` unconditionally for every cluster including `c_statsd`, so
  the line MUST be present with value `0`. Its **absence** is a real failure
  (the `cxTotalOK` guard distinguishes absent-from-zero: `statsdrecv` creates
  the map entry even for a `|c` line of value 0, so `DeltaSum` returns
  `(0, true)` when the line was emitted).

## Delta-SUM model + stability barrier

Each statsd flush emits `<prefix>.<name>:<delta>|c` — the per-flush increment,
NOT the cumulative absolute. A single-window burst makes the first flush's
`delta == absolute == K`, indistinguishable without post-convergence
observation. `awaitFurtherFlushes` observes `>= 2` further flushes after the
delta-SUM converges to `K`: under correct deltas the idle counters emit `0` each
flush so the SUM stays `K` (PASS); an absolute sink re-adds the cumulative so
the SUM overshoots (FAIL) — `reference_delta_sink_differential_stability_barrier`.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`, 17-byte
body), with a **bootstrap-level** `stats_sinks[]` `statsd` entry on BOTH sides:

- `@type: type.googleapis.com/envoy.config.metrics.v3.StatsdSink`
- `tcp_cluster_name: c_statsd` — the sink dials this STATIC cluster; its single
  endpoint is the driver-owned in-process `statsdrecv` **TCP** receiver.
- `prefix: sdpfx` — baked **identically** on both sides.
- `stats_flush_interval: 0.5s`.

`node: { id, cluster }` is **REQUIRED on BOTH sides** (`AMEND-TCP-NODE`) — the
reference REFUSES TO BOOT a `tcp_cluster_name` statsd sink without both. This is
TCP-specific: the UDP sink (`0092`) boots with no `node` at all.

## Reachability: LITERAL host-gateway IP for the reference

The reference container reaches the host `c_statsd` receiver at the **host-
gateway literal IP** (`reference_host_gateway_ip_docker_desktop`) — e.g.
`192.168.65.2` on Docker Desktop, not `127.0.0.1` and not the bridge IPAM
gateway. The driver resolves it via a throwaway `getent hosts
host.docker.internal` container (an inlined `hostGatewayIP`, to avoid an import
cycle with the `differential` package). The subject uses `127.0.0.1`. The
receiver binds `0.0.0.0:<port>` so the container can connect.

## Two private per-side receivers + hard `Close()`

The statsd sink flushes **periodically** and holds its TCP connection open for
the process lifetime, so the reference keeps sending during the subject's drive
window. The driver starts **two private TCP receivers** — one per side — bound
on `0.0.0.0:<port>` BEFORE either proxy starts, giving each side an
uncontaminated accumulator (`reference_periodic_sink_differential_two_receivers`).
After the subject snapshot BOTH are hard-stopped via `Close()` — for a TCP
receiver this hard-closes the listener and every accepted connection. NEVER a
graceful stop (the sink never closes its side).

## UNasserted

The whole line set (`AMEND-TCP-USEDONLY` — used-only, so structurally different);
`|ms` timer histograms (envoy-go has none); non-deterministic gauges
(`server.uptime`, `*_active`, connection churn); flush cadence; per-flush write
granularity (not observable to a line-parsing stream receiver — the assertion is
on aggregated line content, framing-agnostic).
