# Phase 49 Brainstorm — `dog_statsd` stats sink (the SIXTH row of the Observability family; the THIRD `stats_sinks[]` consumer; a periodic UDP DogStatsd-line-protocol stats sink with TAG support over the LANDED phase-47 `stats.Registry`-walking flush subsystem, `envoy.config.metrics.v3.DogStatsdSink` → DogStatsd line protocol over a UDP `address` — a SINGLE FLAT ROW, UDP-only, tags IN-SCOPE, batching DEFERRED)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, direct-on-master (the phase-44/45/46/47/48 family-row BRAINSTORM precedent). Row 49 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated).
>
> **The pick is already made** (dog_statsd — a human decision via the loop re-open, the cheapest remaining deferred Observability candidate per the row-48 ROADMAP note). This BRAINSTORM settles the SCOPE, not the "which row" question.
>
> **User dialogue (3 questions, 2026-07-02):**
> - **Q1 — tag scope → tags IN-SCOPE, reusing `stats.ExtractTags`.** DogStatsd's defining feature over plain statsd is tag support (`name:value|c|#tag1:val1,tag2:val2`). Reuse the SAME SN1–SN9+SN4 extraction core the phase-47.2b `labelMapper` (`internal/statssink/label.go`) already calls, reformatted as an inline `|#` tag suffix instead of `metric[].label[]` `LabelPair`s. An MVP with zero tags (mirroring phase 48) was rejected as pointless — a user configuring dog_statsd almost always wants tags.
> - **Q2 — `max_bytes_per_datagram` batching → DEFERRED.** One-metric-per-datagram only this row (the phase-48 statsd precedent). If explicitly set, envoy-go-STRICT-REJECTS it at parse (mirrors the phase-48 `tcp_cluster_name`-set-when-unsupported reject posture) — SPEC probes whether the reference's own unset-default also emits one-per-datagram.
> - **Q3 — phasing → SINGLE FLAT ROW.** The flush substrate, delta state (`delta.go`), AND the tag-extraction core (`stats.ExtractTags`) are ALL already landed and proven; phase 49 only wires them into a new `Sink` + a DogStatsd line/tag formatter + a parse arm + one differential. No new subsystem to couple — same precedent as least_request-34/random-35/maglev-37/kafka-31/statsd-48.

---

## 1. Mission and scope confirmation (49 — a single flat row, UDP-only, tags in-scope)

### 1.1 What phase 49 delivers as a self-contained whole (a periodic UDP DogStatsd-line stats sink with tags)
A THIRD `stats_sinks[]` consumer: the bootstrap `envoy.config.metrics.v3.DogStatsdSink` (`address` UDP variant) driving a periodic DogStatsd line-protocol emitter over the SAME landed phase-47 flush subsystem. Each `stats_flush_interval` tick, the existing `Flusher` (`internal/statssink/flusher.go`) snapshots the frozen `stats.Registry` into one `[]*dto.MetricFamily` batch and fans it out to every configured `Sink`; phase 49 adds a NEW `Sink` — a `DogStatsdSink` — that walks that batch, extracts tags from each dotted name via `stats.ExtractTags` (the phase-47.2b `label.go` extraction core), and writes DogStatsd lines (`<prefix>.<residual-name>:<value>|c` or `|c|#tag1:val1,tag2:val2` for COUNTER families, `|g`/`|g|#...` for GAUGE) as UDP datagrams to the configured `address`. The metrics_service, statsd, and dog_statsd sinks coexist in the same `statsSinks` slice; the Flusher stays sink-agnostic.

### 1.2 What phase 49 does NOT deliver (forward to §8)
`max_bytes_per_datagram` batching (multiple metrics per datagram — envoy-go-STRICT-REJECTED if set this row); `graphite`/`open_telemetry`-metrics sinks (each its own future deferred Observability row); the plain-statsd `tcp_cluster_name` transport (a DIFFERENT sink, already deferred at phase 48); timers/`|ms` (envoy-go has no histograms — ADR-0060); any non-`socket_address` address shape.

### 1.3 Phase-done as the SIXTH Observability-family row landing (family STAYS OPEN)
Row 49 is the SIXTH Observability-family row (after gRPC-ALS @ 44, OTLP-log @ 45, tracing @ 46, metrics_service @ 47, statsd @ 48). The family STAYS OPEN — remaining deferred candidates: statsd `tcp_cluster_name` transport / graphite / OTLP-metrics sinks / dog_statsd `max_bytes_per_datagram` batching + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace + the tap filter. NO parent rollup (ADR-0106); row 49 flips `done` at the phase-49 IMPL six-gate.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed)
The flush substrate, the delta-state pattern, AND the tag-extraction core are ALL landed; phase 49 adds only a new `Sink` + a DogStatsd line/tag formatter + a parse arm + a differential — no second subsystem to couple. SINGLE FLAT ROW (Q3). A pre-authorized 49.1/49.2 escape-valve stays UNCONSUMED per ADR-0045 unless the SPEC surfaces unexpected size (~250–350 prod LoC / ~9–13 tasks anticipated — slightly larger than phase 48 owing to the tag-formatting path, still well under the gate).

### 1.5 Seed-stub alignment + package placement
The `DogStatsdSink` emitter lives in the EXISTING `internal/statssink` package (a sibling to `sink.go`'s `MetricsServiceSink` and `statsd.go`'s `StatsdSink` — `dogstatsd.go`). NO new package. The UDP seam reuses the phase-48 `net.DialUDP`/`*net.UDPConn` writer shape (a second, independent `*net.UDPConn`, not shared with `StatsdSink`).

### 1.6 No prebrainstorm-notes branch
No off-master prebrainstorm-notes branch exists for dog_statsd (contrast phase-11 local_ratelimit).

### 1.7 Phase 49's relationship to the existing seams (a THIRD Sink on the LANDED flusher + a REUSED UDP-writer shape + a REUSED tag-extraction core + a bootstrap parse extension)
REUSES: the `Flusher` (unchanged — fans `[]*dto.MetricFamily` to `[]Sink`); the `Sink` interface (`Submit`/`Close`); the `stats.Registry` `Walk`/`Freeze` snapshot (ADR-0059); the `stats_sinks[]`/`stats_flush_interval` bootstrap surface (ADR-0262); the `main.go` sink-slice + flusher build (now a THIRD build loop); the phase-48 synchronous-`Submit`/idempotent-`Close`/rate-limit-drop-log UDP writer SHAPE (a sink-private copy, not a shared writer type — `StatsdSink` and `DogStatsdSink` stay independent `Sink` impls per the existing `MetricsServiceSink`/`StatsdSink` precedent); the 47.2a `deltaState` PATTERN (`delta.go`, sink-private, IF the D-DSD-DELTA probe confirms deltas — NOT pre-assumed, may differ from statsd's answer); `stats.ExtractTags` (`internal/stats`, the SAME SN1–SN9+SN4 matcher `label.go`'s `labelMapper` calls — consumed DIRECTLY by the new DogStatsd tag formatter, not via `labelMapper`/`LabelPair`, since the target shape is an inline `|#` string suffix, not a `metric[].label[]` structure); the driver-owned two-per-side-receivers + hard-Close differential harness (`reference_periodic_sink_differential_two_receivers`). NEW: the `DogStatsdSink` emitter + a DogStatsd line/tag-suffix formatter + a `parseDogStatsdSinkConfig` arm lifting the DogStatsdSink TypeURL from the `bootstrap.go:430` sibling-reject (now naming ALL THREE supported sinks).

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the dog_statsd stats sink *(Q0 → phase 49 row registered)*
The loop was RE-OPENED; a human picked dog_statsd from the deferred Observability candidates (the cheapest remaining follow-on — reuses the flush subsystem + the `stats_sinks[]` slot + the `internal/statssink` package + the phase-47.2b tag-extraction core).

### 2.2 Transport: UDP `address` only — not a real scope choice *(self-answered; the proto has no `tcp_cluster_name`-equivalent oneof member)*
Unlike `StatsdSink`, `DogStatsdSink`'s `dog_statsd_specifier` oneof has ONLY `address` (`go-control-plane/envoy@v1.32.4` `config/metrics/v3/stats.pb.go:776-786`) — there is no TCP-over-cluster variant to defer. DogStatsd is inherently UDP-only.

### 2.3 Tags: IN-SCOPE, reusing `stats.ExtractTags` *(Q1 → the defining DogStatsd feature)*
Each `MetricFamily.Name` is run through `stats.ExtractTags` (the residual dotted name + extracted `[]Label{Key,Value}` pairs, identical extraction to the 47.2b `labelMapper`). The residual name is line-prefixed as usual (`<prefix>.<residual-name>`); the extracted tags are SORTED and formatted as a `|#tag1:val1,tag2:val2` suffix appended after the `|c`/`|g` type marker. A name with no extracted tags (ExtractTags error, or zero tags) emits with NO `|#` suffix at all — not an empty one. The exact tag-key format (raw `envoy_foo` vs `envoy.foo` vs bare `foo`) and delimiter/escaping rules are a SPEC live-probe pin (D-DSD-TAGS).

### 2.4 Batching (`max_bytes_per_datagram`): DEFERRED, STRICT-REJECTED if set *(Q2)*
One-metric-per-datagram only, mirroring phase 48's default posture. If a bootstrap sets `max_bytes_per_datagram` explicitly, envoy-go-STRICT-REJECTS the sink config at parse (an unimplemented-but-consequential axis, mirroring the phase-48 `tcp_cluster_name`-set reject). SPEC confirms the reference's own unset-default is also one-per-datagram (so envoy-go's unconditional one-per-datagram behavior is reference-parity when the field is absent).

### 2.5 Envelope: a SINGLE FLAT ROW *(Q3 → ADR-0266)*
No split. The flush substrate, delta-state pattern, and tag-extraction core all exist; phase 49 is purely additive (a new Sink + UDP seam + line/tag formatter + parse arm + differential). ANCHORS ADR-0266.

### 2.6 The sink lifecycle: a connectionless UDP writer, phase-48-shaped *(self-answered; SPEC confirms, D-DSD-LIFECYCLE)*
Same shape as `StatsdSink`: `NewDogStatsdSink` dials the UDP address once via `net.ResolveUDPAddr`+`net.DialUDP`; `Submit` writes datagram(s) synchronously per flush (no channel/writer-goroutine — UDP never blocks + serial `Submit` ⇒ no background mutator); `Close` is idempotent (`sync.Once`); a `Write` error is rate-limit-logged-and-dropped via the `sink.go` `lastDropLog` idiom.

### 2.7 The line mapping: `MetricFamily` → DogStatsd line + tag suffix *(self-answered; SPEC pins, D-DSD-LINE + D-DSD-TAGS + D-DSD-DELTA)*
Walk the `[]*dto.MetricFamily` batch: COUNTER → `<prefix>.<residual-name>:<value>|c[|#tags]`, GAUGE → `<prefix>.<residual-name>:<value>|g[|#tags]`. `<prefix>` = `DogStatsdSink.prefix` (default `envoy`, SAME default as statsd), joined `prefix + "." + residual-name`.

**D-DSD-DELTA — the LOAD-BEARING probe (independent of phase 48's D-SD-DELTA answer).** DogStatsd `|c` is canonically a delta-since-last-flush; CONFIRM against the live reference, do not assume it matches plain statsd's answer. If deltas, `DogStatsdSink` owns its OWN sink-private `deltaState` (a second, independent instance — NOT shared with `StatsdSink`) and the differential asserts the delta-SUM with a post-convergence stability barrier (`reference_delta_sink_differential_stability_barrier`); if absolute, it mirrors 0089's `value==K` last-seen.

**D-DSD-TAGS — the SECOND load-bearing probe.** The exact `|#` tag-suffix grammar: delimiter (`,` between pairs, `:` within a pair — PGV/statsd-client convention, confirm), tag-key format (does the reference emit the raw internal segment, an `envoy.`-prefixed form, or something else — CONTRAST the 47.2b `LabelPair` `"envoy."+TrimPrefix(...)` convention, which was envoy-go's OWN choice, not necessarily the wire truth here), tag ordering (sorted vs insertion-order — envoy-go picks sorted for determinism regardless, but confirm the reference doesn't do something incompatible with a subset-assertion differential).

### 2.8 Deferred-policy posture: additive config; the DogStatsdSink TypeURL LIFTED from the `bootstrap.go:430` sibling-reject; the rest strict-rejected *(self-answered; pinned at SPEC, D-DSD-REJECT)*
`bootstrap.go:430` currently rejects every TypeURL outside `{metrics_service, statsd}`. Lift the DogStatsdSink TypeURL into a NEW dispatch arm + `parseDogStatsdSinkConfig` → a NEW `DogStatsdSinkConfig{UDPAddress, Prefix}`. STRICT-REJECT (ADR-0080, mirroring the phase-47/48 reject discipline): `max_bytes_per_datagram` set (§2.4), a missing/non-`socket_address` address, a non-UDP `protocol` on the socket_address (SPEC pins whether to assert UDP, mirroring phase 48's accepted-and-ignored posture on the same field). The reject text for an UNSUPPORTED sibling TypeURL now names ALL THREE supported sinks (metrics_service + statsd + dog_statsd) (`reference_strict_reject_sibling_typeurl_gap`). `prefix` default `envoy`.

### 2.9 Stat surface hypothesis: zero dog_statsd-sink self-stats *(self-answered; SPEC pins, D-DSD-STATS)*
Both the metrics_service (D-MS-STATS-FINAL) and statsd (D-SD-STATS-FINAL) sinks landed +0 self-stats. dog_statsd anticipated identically +0 (PROBE — D-DSD-STATS); a UDP write has no cluster, so no incidental sink-cluster upstream stats either.

---

## 3. Framework-survey result — a THIRD Sink on the LANDED flusher + a REUSED UDP-writer shape + a REUSED tag-extraction core; ZERO new packages, ZERO new go.mod modules (49 anticipated)

### 3.1 Framework: a NEW `DogStatsdSink` (in the EXISTING `internal/statssink`) over the LANDED `Flusher`/`Sink` seam *(per §1.7)*
No new flush loop, no new typed-client layer. The `DogStatsdSink` is a `Sink` impl + a `net.UDPConn` writer (phase-48-shaped) + a tag-suffix formatter (new, over the existing `stats.ExtractTags`).

### 3.2 NEW packages: anticipated NONE (the `DogStatsdSink` lives in the existing `internal/statssink`; the differential receiver extends/reuses `test/helpers/statsdrecv` under `test/helpers/`).

### 3.3 go.mod modules: anticipated NONE. DogStatsd line protocol is trivial (`fmt.Fprintf`-shaped line formatting + `net.DialUDP`); HAND-ROLLED, no client lib. The `DogStatsdSink` proto resolves at the already-present `go-control-plane/envoy` dep. `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES
The `Flusher`/`Sink` seam (`flusher.go`/`sink.go`); the `stats.Registry` `Walk`/`Freeze`; the `stats_sinks[]`/`stats_flush_interval` parse (`bootstrap.go` `parseStatsSinks`); the `main.go` sink-slice + flusher build; the phase-48 `StatsdSink` UDP-writer SHAPE (a sink-private copy — synchronous `Submit`, idempotent `Close`, drop-log `Write` errors); the `deltaState` PATTERN (`delta.go`, IF D-DSD-DELTA confirms deltas — a sink-private copy, independent of `StatsdSink`'s own instance); `stats.ExtractTags` (`internal/stats`, the SAME extraction core `label.go` calls — consumed directly, not via `labelMapper`); the driver-owned two-per-side-receivers + hard-Close differential harness (`reference_periodic_sink_differential_two_receivers`); `differential.HostGatewayIP` IF the differential needs a literal-IP reference→host receiver address (the phase-48 `0092` precedent — reuse verbatim if `0093` needs the same shape).

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener)
Same surface as phases 47/48: top-level `Bootstrap.stats_sinks[]` + `stats_flush_interval`. Phase 49 adds a THIRD accepted TypeURL to the `parseStatsSinks` dispatch. A bootstrap MAY carry any combination of metrics_service, statsd, and dog_statsd sinks (the list is independently parsed per entry; the flusher fans out to all).

---

## 5. Stat surface hypothesis — zero dog_statsd-sink self-stats (49)

### 5.1 Stat names (SPEC pins, D-DSD-STATS)
Anticipated NONE (the metrics_service/statsd +0 precedent).

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)
The strict rejects: `max_bytes_per_datagram` set, missing/non-`socket_address` address, unsupported sibling TypeURLs. All reference-divergence-or-parity TBD at the SPEC live probe.

### 5.3 Anticipated surface arithmetic
Stat surface **1200 → 1200** (+0 anticipated; non-H2 1196). SPEC/IMPL pin.

---

## 6. Differential fixture envelope — anticipated ONE directory `0093-stats-sink-dogstatsd`

### 6.1 Fixtures
`0093-stats-sink-dogstatsd`: a cross-side differential with a DRIVER-OWNED UDP DogStatsd-line receiver (NOT a BackendKind — `reference_differential_grpc_receiver_driver_owned`; extend `test/helpers/statsdrecv` to parse the optional `|#tag:value` suffix if the line grammar is a strict superset of plain statsd, else a small sibling receiver), TWO per-side receivers + a hard `Close()` teardown (`reference_periodic_sink_differential_two_receivers`). Assert the emitted line subset by `<prefix>.<residual-name>` + a sorted tag-set over a deterministic counter/gauge subset under a defined request load: the delta-SUM with a post-convergence stability barrier IF D-DSD-DELTA confirms deltas, else the absolute `value==K` last-seen. Differential discipline carries: `-run 'TestDifferential/0093'` NEVER bare (`reference_differential_run_selector`); `-count=1` on every break + `-race` (`reference_differential_break_protocol_count1`); the Docker bridge + a decode-ran proof (`reference_docker_probe_bridge_network`); the live reference is the wire truth (`reference_wire_format_both_sides_see_same_bytes`).

### 6.2 Total
fixtures **94 → 95** (`0093`).

### 6.3 New BackendKind: anticipated NONE (driver-owned UDP receiver, BackendKind stays 38).

### 6.4 New fuzzer: anticipated ONE — `FuzzDogStatsdSinkConfigParse` (the DogStatsdSink typed_config parse arm; fuzzers **51 → 52**). SPEC confirms (the `FuzzStatsSinkConfigParse`/`FuzzStatsdSinkConfigParse` precedent — a new accepted-config parse path warrants its own no-panic fuzzer).

---

## 7. Anticipated ADRs — 1 at the phase-49 IMPL: ADR-0266 (the dog_statsd UDP stats sink)
ADR-0266 (ACCEPTED — the DogStatsd line-protocol UDP sink with tags; §Context drafted at the SPEC, §Decision/§Consequences landed in-place at the IMPL per ADR-0044). NO seam ADR (the `Flusher`/`Sink` seam is reused unchanged). next-free after 49: ADR-0267.

---

## 8. Deferred items
- `max_bytes_per_datagram` batching (multiple metrics per datagram — its own future sub-leg/row if ever needed; strict-rejected if set this row).
- `graphite` / `open_telemetry`-metrics sinks (each its own future deferred Observability row).
- the plain-statsd `tcp_cluster_name` transport (a DIFFERENT sink, already deferred at phase 48 — not re-litigated here).
- timers/`|ms` (gated by the absent histogram boundary — ADR-0060).
- the tap filter + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace (unrelated Observability-family candidates, unchanged from the phase-48 deferred list).

---

## 9. Cross-references against prior phases' deferred-items lists — pickup
Picks up the phase-48 ROADMAP row's "dog_statsd" deferred candidate (charted as phase 49 on 2026-07-02). The remaining deferred candidates (statsd `tcp_cluster_name`/graphite/OTLP-metrics/tracing-extras/tap/dog_statsd-batching) carry forward UNbrainstormed.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network` + `reference_stats_sink_metrics_service_probe`)
- **D-DSD-DELTA (LOAD-BEARING):** does the reference dog_statsd sink emit counter `|c` as a delta-since-last-flush or absolute? (Independent of phase 48's plain-statsd answer — confirm, don't assume identical. Decides whether `DogStatsdSink` owns a sink-local delta state and whether `0093` uses the delta-SUM stability barrier or the absolute `value==K`.)
- **D-DSD-TAGS (LOAD-BEARING):** the exact `|#tag1:val1,tag2:val2` grammar — delimiter, tag-key format (raw internal segment vs `envoy.`-prefixed vs other), presence/absence of the suffix on untagged names, tag ordering.
- **D-DSD-NAME:** confirm the residual-name projection (tag-value segments removed, dots kept) matches the 47.2b `ExtractTags` residual exactly, and the exact `prefix` join (`prefix + "." + residual-name`).
- **D-DSD-LINE:** the exact line format — value as integer, the `|c`/`|g` type suffixes, one-datagram-per-line (the reference's own `max_bytes_per_datagram`-unset default).
- **D-DSD-LIFECYCLE:** confirm the phase-48 synchronous-write shape carries unchanged (no reason to expect otherwise, but SPEC confirms per-phase per project discipline).
- **D-DSD-STATS:** confirm +0 dog_statsd-sink self-stats (the metrics_service/statsd precedent).
- **D-DSD-REJECT:** does the reference accept/reject a missing address, a non-UDP socket protocol, an explicit `max_bytes_per_datagram`? (envoy-go-strict rejects vs reference-parity.)

---

## 11. Prior-phase lessons applied
- `reference_stats_sink_metrics_service_probe` — name-subset (not whole-set) differential; the reference is stricter on config; no self-stats.
- `reference_periodic_sink_differential_two_receivers` — TWO per-side receivers + hard `Close()` (NOT GracefulStop / a single shared receiver) for a periodic sink.
- `reference_delta_sink_differential_stability_barrier` — if deltas, assert the SUM is STILL K after ≥2 further flushes (a flush-count barrier).
- `reference_differential_grpc_receiver_driver_owned` — the UDP receiver is a driver-owned `test/helpers` server, NOT a BackendKind.
- `reference_strict_reject_sibling_typeurl_gap` — lifting one TypeURL from the silent-reject set needs the dispatch to name ALL supported siblings (now three).
- `reference_docker_probe_bridge_network` — the SPEC probe runs on a shared Docker bridge with a decode-ran proof.
- `reference_host_gateway_ip_docker_desktop` — reuse `differential.HostGatewayIP` verbatim if `0093` needs a literal-IP reference→host receiver address (the phase-48 `0092` precedent).
- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; controller squashes + pushes at stage-close.

---

## 12. Section closeout
- **Subject:** the bootstrap `stats_sinks[]` `envoy.config.metrics.v3.DogStatsdSink` (UDP `address` variant) — a periodic UDP DogStatsd-line-protocol stats sink WITH TAGS over the LANDED phase-47 flush subsystem.
- **Q1 tags:** IN-SCOPE, reusing `stats.ExtractTags` (the 47.2b `label.go` extraction core) reformatted as an inline `|#tag1:val1,...` suffix.
- **Q2 batching:** `max_bytes_per_datagram` DEFERRED, envoy-go-STRICT-REJECTED if set this row.
- **Q3 phasing:** SINGLE FLAT ROW (escape-valve unconsumed under ADR-0045).
- **Scope:** lift the DogStatsdSink TypeURL from the `bootstrap.go:430` sibling-reject (now naming all three sinks) + a `DogStatsdSinkConfig{UDPAddress, Prefix}` + a NEW `internal/statssink` `DogStatsdSink` emitter (phase-48-shaped UDP seam + a new tag-suffix line formatter over `stats.ExtractTags`) + main.go's third build loop + `FuzzDogStatsdSinkConfigParse` + the `0093-stats-sink-dogstatsd` cross-side differential (driver-owned UDP receiver, extending `test/helpers/statsdrecv`).
- **Anticipated counts:** stat **1200** (+0) / fixtures **95** (`0093`) / fuzzers **52** (`FuzzDogStatsdSinkConfigParse`) / BackendKind **38** / DECISIONS **ADR-0266** (next-free ADR-0267); ZERO new packages, ZERO new go.mod modules.
- **Load-bearing SPEC probes:** D-DSD-DELTA (counter delta-vs-absolute, independent of phase 48's answer) + D-DSD-TAGS (the `|#` tag-suffix grammar).
- **Row 49** registers `in-progress` at this BRAINSTORM commit; flips `done` at the phase-49 IMPL six-gate (NO parent rollup — ADR-0106). The Observability FAMILY STAYS OPEN.
- **Next → the phase-49 SPEC** (`SPEC-49.md` — execute the §10 D-DSD-* live pins against `contrib-v1.37.2`; anchor the ADR-0266 §Context draft; docs-only direct-on-master).
