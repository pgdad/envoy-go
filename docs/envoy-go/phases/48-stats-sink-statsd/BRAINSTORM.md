# Phase 48 Brainstorm — `statsd` stats sink (the FIFTH row of the Observability family; the SECOND `stats_sinks[]` consumer; a periodic UDP statsd-line-protocol stats sink over the LANDED phase-47 `stats.Registry`-walking flush subsystem, `envoy.config.metrics.v3.StatsdSink` → statsd line protocol over a UDP `address` — a SINGLE FLAT ROW, UDP-only)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, direct-on-master (the phase-44/45/46/47 family-row BRAINSTORM precedent). Row 48 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated).
>
> **The pick is already made** (statsd — a human decision via the loop re-open). This BRAINSTORM settles the SCOPE, not the "which row" question.
>
> **User dialogue (2 questions, 2026-06-30):**
> - **Q1 — transport scope → UDP `address` only.** The canonical statsd default. A brand-new UDP datagram seam (envoy-go's first `net.UDPConn`). `tcp_cluster_name` becomes a future deferred follow-on (its own later row/sub-leg). TCP-over-named-cluster is OUT of phase 48.
> - **Q2 — phasing → SINGLE FLAT ROW.** The phase-47 flush substrate already exists (no new framework piece, no second subsystem to couple) — the least_request-34 / random-35 / maglev-37 / kafka-31 precedent. A pre-authorized 48.1/48.2 escape-valve stays UNCONSUMED per ADR-0045 unless the SPEC surfaces unexpected size.

---

## 1. Mission and scope confirmation (48 — a single flat row, UDP-only)

### 1.1 What phase 48 delivers as a self-contained whole (a periodic UDP statsd-line stats sink)
A second `stats_sinks[]` consumer: the bootstrap `envoy.config.metrics.v3.StatsdSink` (`address` UDP variant) driving a periodic statsd line-protocol emitter over the SAME landed phase-47 flush subsystem. Each `stats_flush_interval` tick, the existing `Flusher` (`internal/statssink/flusher.go`) snapshots the frozen `stats.Registry` into one `[]*dto.MetricFamily` batch and fans it out to every configured `Sink`; phase 48 adds a NEW `Sink` — a `StatsdSink` — that walks that batch and writes statsd lines (`<prefix>.<name>:<value>|c` for COUNTER families, `<prefix>.<name>:<value>|g` for GAUGE) as UDP datagrams to the configured `address`. The metrics_service sink and the statsd sink coexist in the same `statsSinks` slice; the Flusher is sink-agnostic.

### 1.2 What phase 48 does NOT deliver (forward to §8)
`tcp_cluster_name` transport (statsd-over-named-cluster); `dog_statsd`/`graphite`/`open_telemetry`-metrics sinks (each its own future deferred Observability row); timers/`|ms` (envoy-go has no histograms — ADR-0060); tag-expanded metric names (the proto explicitly states the statsd sink "does not support tagged metrics"); any non-`socket_address` address shape.

### 1.3 Phase-done as the FIFTH Observability-family row landing (family STAYS OPEN)
Row 48 is the FIFTH Observability-family row (after gRPC-ALS @ 44, OTLP-log @ 45, tracing @ 46, metrics_service @ 47). The family STAYS OPEN — remaining deferred candidates: dog_statsd / graphite / OTLP-metrics sinks + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace + the tap filter. NO parent rollup (ADR-0106); row 48 flips `done` at the phase-48 IMPL six-gate.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed)
The flush substrate is LANDED; phase 48 adds only a new `Sink` + a UDP seam + a parse arm + a differential — no second subsystem to couple. SINGLE FLAT ROW (Q2). A pre-authorized 48.1 (UDP seam + emitter + parse + differential) / 48.2 (a deferred concern) escape-valve stays UNCONSUMED per ADR-0045 unless the SPEC surfaces unexpected size (~200–350 prod LoC / ~8–12 tasks anticipated, well under the gate).

### 1.5 Seed-stub alignment + package placement
The `StatsdSink` emitter lives in the EXISTING `internal/statssink` package (a sibling to `sink.go`'s `MetricsServiceSink` — likely `statsd.go`). NO new package. The UDP seam is local to `internal/statssink` (a `net.UDPConn` writer), NOT a new `internal/grpcclient`-style typed wrapper (statsd is connectionless line protocol, not gRPC).

### 1.6 No prebrainstorm-notes branch
No off-master prebrainstorm-notes branch exists for statsd (contrast phase-11 local_ratelimit).

### 1.7 Phase 48's relationship to the existing seams (a SECOND Sink on the LANDED flusher + a NEW UDP seam + a bootstrap parse extension)
REUSES: the `Flusher` (unchanged — fans `[]*dto.MetricFamily` to `[]Sink`); the `Sink` interface (`Submit`/`Close`); the `stats.Registry` `Walk`/`Freeze` snapshot (ADR-0059); the `snapshot()` cumulative/no-labels mapping (the statsd sink consumes the SAME default batch the metrics_service sink does — full dotted name, absolute counter values, no labels); the `stats_sinks[]`/`stats_flush_interval` bootstrap surface (ADR-0262); the `main.go:189-201` sink-slice + flusher build. NEW: the `StatsdSink` emitter + the UDP datagram seam + a `parseStatsdSinkConfig` arm lifting the StatsdSink TypeURL from the `bootstrap.go:430` sibling-reject.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the statsd stats sink *(Q0 → phase 48 row registered)*
The loop was RE-OPENED; a human picked statsd from the deferred Observability candidates. The cheapest follow-on to phase-47 metrics_service (reuses the flush subsystem + the `stats_sinks[]` slot + the `internal/statssink` package).

### 2.2 Transport: UDP `address` only *(Q1 → the transport)*
The canonical statsd default. A NEW UDP datagram seam (`net.DialUDP` / `*net.UDPConn`) — envoy-go's FIRST UDP socket. `tcp_cluster_name` DEFERRED (§8); a configured `tcp_cluster_name` is STRICT-REJECTED at parse (this row is UDP-only).

### 2.3 Envelope: a SINGLE FLAT ROW *(Q2 → ADR-0265)*
No split. The flush substrate exists; phase 48 is purely additive (a new Sink + UDP seam + parse arm + differential). ANCHORS ADR-0265.

### 2.4 The sink lifecycle: a connectionless UDP writer (no identifier, no stream, no node) *(self-answered; pinned at SPEC, D-SD-LIFECYCLE)*
statsd UDP is fire-and-forget datagrams — NO lazy-stream/identifier-once/reconnect machinery (contrast the metrics_service client-streaming RPC). `NewStatsdSink` dials the UDP address once; `Submit` writes datagram(s) per flush; `Close` closes the `*net.UDPConn`. A write error is logged-and-dropped (lossy is intrinsic to UDP statsd; the metrics_service drop-log idiom). The bounded-channel + writer-goroutine shape MAY be retained for Submit/Close symmetry with `MetricsServiceSink`, or simplified to a synchronous write (SPEC pins — D-SD-LIFECYCLE).

### 2.5 The line mapping: `MetricFamily` → statsd line *(self-answered; pinned at SPEC, D-SD-LINE + D-SD-NAME + D-SD-DELTA)*
Walk the `[]*dto.MetricFamily` batch: COUNTER → `<prefix>.<name>:<value>|c`, GAUGE → `<prefix>.<name>:<value>|g`. `<name>` = the family's `MetricFamily.Name` (the full dotted internal name with tags INLINED — `v.Name()`; the proto confirms "does not support tagged metrics", so ZERO SN-rule label machinery). `<prefix>` = `StatsdSink.prefix` (default `envoy`), joined `prefix + "." + name`. The exact prefix join, one-datagram-per-line vs newline-batched packing, and the value format (integer) are SPEC live-probe pins (D-SD-LINE/D-SD-NAME).

**D-SD-DELTA — the LOAD-BEARING probe.** statsd `|c` is canonically a delta-since-last-flush (an increment), `|g` an absolute. If the live reference latches counter deltas, the `StatsdSink` owns a sink-local last-value delta state (the 47.2a `internal/statssink/delta.go` `deltaState` pattern, sink-private) and the differential asserts the delta-SUM with a post-convergence stability barrier (`reference_delta_sink_differential_stability_barrier`); if absolute, it mirrors 0089's `value==K` last-seen. PROBE DECIDES (§10) — NOT pre-assumed here.

### 2.6 Deferred-policy posture: additive config; the StatsdSink TypeURL LIFTED from the `bootstrap.go:430` sibling-reject; the rest strict-rejected *(self-answered; pinned at SPEC, D-SD-REJECT)*
`bootstrap.go:430` currently rejects every non-`metrics_service` TypeURL. Lift the StatsdSink TypeURL into a NEW dispatch arm + `parseStatsdSinkConfig` → a NEW `StatsdSinkConfig{UDPAddress, Prefix}`. STRICT-REJECT (ADR-0080, mirroring the phase-47 reject discipline): `tcp_cluster_name` (UDP-only this row), a missing/non-`socket_address` address, a non-UDP `protocol` on the socket_address (SPEC pins whether to assert UDP). The reject text for an UNSUPPORTED sibling TypeURL now names BOTH supported sinks (metrics_service + statsd) (`reference_strict_reject_sibling_typeurl_gap`). `prefix` default `envoy`.

### 2.7 Stat surface hypothesis: zero statsd-sink self-stats *(self-answered; SPEC pins, D-SD-STATS)*
The metrics_service sink landed +0 self-stats (D-MS-STATS-FINAL; the reference emits no metrics_service self-stats). statsd anticipated identically +0 (PROBE — D-SD-STATS); a UDP write has no cluster, so no incidental sink-cluster upstream stats either (contrast the metrics_service gRPC cluster).

---

## 3. Framework-survey result — a SECOND Sink on the LANDED flusher + a NEW UDP seam; ZERO new packages, ZERO new go.mod modules (48 anticipated)

### 3.1 Framework: a NEW `StatsdSink` (in the EXISTING `internal/statssink`) over the LANDED `Flusher`/`Sink` seam *(per §1.7)*
No new flush loop, no new typed-client layer. The `StatsdSink` is a `Sink` impl + a `net.UDPConn` writer.

### 3.2 NEW packages: anticipated NONE (the `StatsdSink` lives in the existing `internal/statssink`; the UDP receiver under `test/helpers/`).

### 3.3 go.mod modules: anticipated NONE. The statsd line protocol is trivial (`fmt.Fprintf`-shaped line formatting + `net.DialUDP`); HAND-ROLLED, no client lib. The `StatsdSink` proto resolves at the already-present `go-control-plane/envoy` dep. `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES
The `Flusher`/`Sink` seam (`flusher.go`/`sink.go`); the `snapshot()` cumulative/no-labels mapping (`mapping.go`); the `stats.Registry` `Walk`/`Freeze`; the `stats_sinks[]`/`stats_flush_interval` parse (`bootstrap.go` `parseStatsSinks`); the `main.go` sink-slice + flusher build; the 47.2a `deltaState` PATTERN (if D-SD-DELTA confirms deltas — a sink-private copy, NOT a shared seam); the driver-owned two-per-side-receivers + hard-Close differential harness (`reference_periodic_sink_differential_two_receivers`).

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener)
Same surface as phase 47: top-level `Bootstrap.stats_sinks[]` + `stats_flush_interval`. Phase 48 adds a SECOND accepted TypeURL to the `parseStatsSinks` dispatch. A bootstrap MAY carry both a metrics_service and a statsd sink (the list is independently parsed per entry; the flusher fans out to all).

---

## 5. Stat surface hypothesis — zero statsd-sink self-stats (48)

### 5.1 Stat names (SPEC pins, D-SD-STATS)
Anticipated NONE (the metrics_service +0 precedent).

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)
The strict rejects: `tcp_cluster_name`, missing/non-`socket_address` address, unsupported sibling TypeURLs. All reference-divergence-or-parity TBD at the SPEC live probe.

### 5.3 Anticipated surface arithmetic
Stat surface **1200 → 1200** (+0 anticipated; non-H2 1196). SPEC/IMPL pin.

---

## 6. Differential fixture envelope — anticipated ONE directory `0092-stats-sink-statsd`

### 6.1 Fixtures
`0092-stats-sink-statsd`: a cross-side differential with a DRIVER-OWNED UDP statsd-line receiver (NOT a BackendKind — `reference_differential_grpc_receiver_driver_owned`), TWO per-side receivers + a hard `Close()` teardown (`reference_periodic_sink_differential_two_receivers` — the periodic flush means the reference streams datagrams for the whole test). Assert the emitted statsd-line subset by `<prefix>.<name>` over a deterministic counter/gauge subset under a defined request load: the delta-SUM with a post-convergence stability barrier IF D-SD-DELTA confirms deltas, else the absolute `value==K` last-seen. Differential discipline carries: `-run 'TestDifferential/0092'` NEVER bare (`reference_differential_run_selector`); `-count=1` on every break + `-race` (`reference_differential_break_protocol_count1`); the Docker bridge + a decode-ran proof (`reference_docker_probe_bridge_network`); the live reference is the wire truth (`reference_wire_format_both_sides_see_same_bytes`).

### 6.2 Total
fixtures **93 → 94** (`0092`).

### 6.3 New BackendKind: anticipated NONE (driver-owned UDP receiver, BackendKind stays 38).

### 6.4 New fuzzer: anticipated ONE — `FuzzStatsdSinkConfigParse` (the StatsdSink typed_config parse arm; fuzzers **50 → 51**). SPEC confirms (a new accepted-config parse path warrants its own no-panic fuzzer, the `FuzzStatsSinkConfigParse` precedent).

---

## 7. Anticipated ADRs — 1 at the phase-48 IMPL: ADR-0265 (the statsd UDP stats sink)
ADR-0265 (ACCEPTED — the statsd line-protocol UDP sink; §Context drafted at the SPEC, §Decision/§Consequences landed in-place at the IMPL per ADR-0044). NO seam ADR (the `Flusher`/`Sink` seam is reused unchanged). next-free after 48: ADR-0266.

---

## 8. Deferred items
- `tcp_cluster_name` transport (statsd-over-named-cluster — its own future sub-leg/row; reuses the cluster/dispatch seam).
- `dog_statsd` / `graphite` / `open_telemetry`-metrics sinks (each its own future deferred Observability row).
- timers/`|ms` (gated by the absent histogram boundary — ADR-0060).
- tag-expanded metric names (the proto does not support tagged metrics; moot).
- newline-batched-packet packing optimization IF the SPEC probe shows the reference packs (carry as an IMPL micro-decision, not a deferred feature).

---

## 9. Cross-references against prior phases' deferred-items lists — pickup
Picks up the phase-47 ROADMAP family note's "statsd" deferred candidate (charted as phase 48 on 2026-06-30). The remaining deferred candidates (dog_statsd/graphite/OTLP-metrics/tracing-extras/tap) carry forward UNbrainstormed.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network` + `reference_stats_sink_metrics_service_probe`)
- **D-SD-DELTA (LOAD-BEARING):** does the reference statsd sink emit counter `|c` as a delta-since-last-flush or absolute? (Decides whether the `StatsdSink` owns a sink-local delta state and whether 0092 uses the delta-SUM stability barrier or the absolute `value==K`.)
- **D-SD-NAME:** the exact metric-name projection — confirm it is the full dotted internal name with tags inlined (NOT tag-expanded), and the exact `prefix` join (`prefix + "." + name`). The load-bearing differential surface.
- **D-SD-LINE:** the exact line format — value as integer, the `|c`/`|g` type suffixes, one-datagram-per-line vs newline-batched packing, gauge sign/delta-gauge (`+`/`-`) conventions.
- **D-SD-LIFECYCLE:** synchronous UDP write vs a bounded-channel writer-goroutine (the metrics_service shape); the write-error drop-log posture.
- **D-SD-STATS:** confirm +0 statsd-sink self-stats (the metrics_service precedent).
- **D-SD-REJECT:** does the reference accept/reject `tcp_cluster_name` standalone, a missing address, a non-UDP socket protocol? (envoy-go-strict rejects vs reference-parity.)

---

## 11. Prior-phase lessons applied
- `reference_stats_sink_metrics_service_probe` — name-subset (not whole-set) differential; the reference is stricter on config; no self-stats.
- `reference_periodic_sink_differential_two_receivers` — TWO per-side receivers + hard `Close()` (NOT GracefulStop / a single shared receiver) for a periodic sink.
- `reference_delta_sink_differential_stability_barrier` — if deltas, assert the SUM is STILL K after ≥2 further flushes (a flush-count barrier).
- `reference_differential_grpc_receiver_driver_owned` — the UDP receiver is a driver-owned `test/helpers` server, NOT a BackendKind.
- `reference_strict_reject_sibling_typeurl_gap` — lifting one TypeURL from the silent-reject set needs the dispatch to name ALL supported siblings.
- `reference_docker_probe_bridge_network` — the SPEC probe runs on a shared Docker bridge with a decode-ran proof.
- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; controller squashes + pushes at stage-close.

---

## 12. Section closeout
- **Subject:** the bootstrap `stats_sinks[]` `envoy.config.metrics.v3.StatsdSink` (UDP `address` variant) — a periodic UDP statsd-line-protocol stats sink over the LANDED phase-47 flush subsystem.
- **Q1 transport:** UDP `address` only (TCP-over-cluster deferred).
- **Q2 phasing:** SINGLE FLAT ROW (escape-valve unconsumed under ADR-0045).
- **Scope:** lift the StatsdSink TypeURL from the `bootstrap.go:430` sibling-reject + a `StatsdSinkConfig{UDPAddress, Prefix}` + a NEW `internal/statssink` `StatsdSink` emitter (UDP seam + line mapping) + main.go wiring + `FuzzStatsdSinkConfigParse` + the `0092-stats-sink-statsd` cross-side differential (driver-owned UDP receiver).
- **Anticipated counts:** stat **1200** (+0) / fixtures **94** (`0092`) / fuzzers **51** (`FuzzStatsdSinkConfigParse`) / BackendKind **38** / DECISIONS **ADR-0265** (next-free ADR-0266); ZERO new packages, ZERO new go.mod modules.
- **Load-bearing SPEC probe:** D-SD-DELTA (counter delta-vs-absolute) + D-SD-NAME (the name projection + prefix join).
- **Row 48** registers `in-progress` at this BRAINSTORM commit; flips `done` at the phase-48 IMPL six-gate (NO parent rollup — ADR-0106). The Observability FAMILY STAYS OPEN.
- **Next → the phase-48 SPEC** (`SPEC-48.md` — execute the §10 D-SD-* live pins against `contrib-v1.37.2`; anchor the ADR-0265 §Context draft; docs-only direct-on-master).
