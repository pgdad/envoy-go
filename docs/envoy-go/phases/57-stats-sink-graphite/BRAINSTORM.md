# Phase 57 Brainstorm — `graphite_statsd` stats sink (the TENTH row of the Observability family; the FOURTH `stats_sinks[]` consumer; a periodic UDP graphite-flavored statsd-line stats sink over the LANDED phase-47 `stats.Registry`-walking flush subsystem, `envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink` → graphite-tagged statsd line protocol over a UDP `address` — a SINGLE FLAT ROW, UDP-only, tags IN-SCOPE, `max_bytes_per_datagram` batching HONORED DAY 1)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes. Fresh worktree `.worktrees/phase-57-brainstorm`, branch `phase-57-stats-sink-graphite-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open:** phase 56 (`http-tap-filter`) landed COMPLETE (row 56 `done`). This session RE-OPENED the loop and, at the FIRST brainstorm decision, the human picked the `graphite` stats sink (the cheapest remaining Observability follow-on) over three declined alternatives (a tracing sub-feature, opening the xDS family, opening a different new family).
>
> **Baselines re-verified against master tip `91db1512` (the 56.2 IMPL squash):** stat surface **1201** · fixtures **102** (tail `0100-http-tap-bodies`) · fuzzers **53** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0274** (next-free **ADR-0275**) · new Go packages **0** · new go.mod modules **0**. Counts are UNCHANGED at a BRAINSTORM (docs-only).

---

## 1. Mission and scope confirmation (57 — a single flat row, UDP-only, tags in-scope, batching honored)

### 1.1 What phase 57 delivers as a self-contained whole (a periodic UDP graphite-tagged statsd-line stats sink)

A NEW `internal/statssink/graphite.go` `GraphiteStatsdSink` — a fourth `stats_sinks[]` consumer over the LANDED phase-47 `Flusher`/`Sink` seam — that on each flush walks the stats registry (already done by the `Flusher`), and for each `MetricFamily`:

- applies the sink-private `deltaState` (COUNTER family → per-flush DELTA `|c`; GAUGE family → ABSOLUTE `|g`) — reusing `delta.go` VERBATIM (a SECOND, independent instance, never shared with the statsd/dog_statsd sinks; the phase-49 posture);
- extracts the residual name + tags via `stats.ExtractTags` (the phase-47.2b/49 tag core, reused unchanged);
- formats a **graphite-flavored statsd line**: `<prefix>.<residual-name>;<key1>=<value1>;<key2>=<value2>:<value>|<type>` — the tags append to the METRIC NAME as graphite's native `;k=v` pairs (CONTRAST dog_statsd's trailing `|#k:v` suffix; CONTRAST plain statsd's tag-free full dotted name);
- accumulates each line into a per-flush buffer, flushing it as one or more UDP datagrams per the `max_bytes_per_datagram` cap (reusing the phase-50 `appendLine`/`flush` batching machinery);
- writes each datagram over a connectionless UDP writer (the phase-48 `net.DialUDP` shape).

The bootstrap side adds a `parseGraphiteStatsdSinkConfig` arm + a `GraphiteStatsdSinkConfig` struct + a new typed-extension dispatch case in `parseStatsSinks` (§4). `cmd/envoy-go/main.go` gains a fourth build loop constructing the sink after `Load` returns.

### 1.2 What phase 57 does NOT deliver (forward to §8)

Nothing beyond the `GraphiteStatsdSink` proto's own two knobs (`address` + `prefix` + `max_bytes_per_datagram`, ALL consumed). There is no `tcp_cluster_name` equivalent on this proto (§2.2), no histogram surface (envoy-go has none — ADR-0060). The remaining Observability deferred candidates (`OTLP-metrics` sink, tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace) are untouched and carry forward (§9).

### 1.3 Phase-done as the TENTH Observability-family row landing (family STAYS OPEN)

Row 57 is the TENTH Observability-family row and the FOURTH `stats_sinks[]` consumer (after `metrics_service`-47, `statsd`-48/55, `dog_statsd`-49/50). After phase 57 phase-done the family STAYS OPEN — the deferred candidates listed in §9 remain.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed) *(Q2)*

The user settled a SINGLE FLAT ROW. The substrate is ENTIRELY landed: the delta transform (phase 48), the UDP writer (phase 48), the tag-extraction core (phase 47.2b/49), and — decisively — the `max_bytes_per_datagram` batching machinery (phase 50). The ONLY genuinely-new code is a graphite tag-format function (~10 LoC) plus the typed-extension dispatch case. The concrete decomposition is anticipated at ~8–12 tasks, well under the ADR-0045 `~15` ceiling. A 57.1/57.2 by-concern escape valve was CONSIDERED and REJECTED at the brainstorm (Q2): the phase-49→phase-50 split existed ONLY because phase 49 shipped dog_statsd BEFORE any batching machinery existed; that machinery now EXISTS, so there is no second subsystem to strand. The escape valve is documented as unconsumed and re-armable if the SPEC's task count surprises upward.

### 1.5 Seed-stub alignment + package placement

The sink lives in the EXISTING `internal/statssink` package alongside `statsd.go`/`dogstatsd.go`/`statsd_tcp.go` — a NEW `graphite.go` + `graphite_test.go`, ZERO new packages. The bootstrap parse arm lives in the EXISTING `internal/bootstrap/bootstrap.go` alongside the other three sink parsers. The fuzzer lives in a new `internal/bootstrap/graphite_fuzz_test.go` (the `statsd_fuzz_test.go`/`dogstatsd_fuzz_test.go` precedent).

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for graphite (checked: `grep -rn -i graphite` over `internal/`/`cmd/`/`test/` returns NOTHING beyond the ROADMAP deferred-candidate mention).

### 1.7 Phase 57's relationship to the existing seams (a FOURTH Sink on the LANDED flusher + REUSED delta/UDP/tag/batching cores + a bootstrap parse extension)

The sink is a FOURTH `Sink` impl on the phase-47 `Flusher`/`Sink` seam — REUSED unchanged, no new framework piece. It REUSES four landed cores verbatim: `delta.go` (phase 48), the UDP writer shape (phase 48), `stats.ExtractTags` (phase 47.2b/49), and the phase-50 `appendLine`/`flush` batching loop. The ONE novel piece is the graphite line-format function. The dispatch extension is the SAME pattern as `metrics_service` (a typed-extension `Any` matched by type URL; §4).

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the graphite stats sink *(Q0 → phase 57 row registered)*

The FIRST brainstorm decision. Picked from the two live work sources (Observability deferred candidates vs. a never-opened family) as the cheapest remaining follow-on — it reuses the MOST landed substrate of any candidate. Row 57 registers `in-progress` AT this BRAINSTORM commit per the §Schema invariant.

### 2.2 Transport: UDP `address` only — not a real scope choice *(self-answered; the proto oneof has only `GraphiteStatsdSink_Address`)*

`GraphiteStatsdSink.statsd_specifier` is a oneof with EXACTLY ONE arm, `*GraphiteStatsdSink_Address` (`address` = `core.v3.Address`, documented "The UDP address of a running Graphite-compliant listener"). There is NO `tcp_cluster_name` equivalent (CONTRAST plain `StatsdSink`, whose phase-55 TCP arm added the whole bounded-channel writer-goroutine machinery). So graphite is UDP-only by construction — the phase-48/49 connectionless-UDP shape. A missing `statsd_specifier` is a boot-reject (the PGV `oneof value cannot be a typed-nil` + envoy-go's own "specifier is required" tail, mirroring the statsd nil-address reject).

### 2.3 Tags: IN-SCOPE, reusing `stats.ExtractTags` *(self-answered; graphite's defining feature)*

Graphite tag support is the ENTIRE reason the `graphite_statsd` extension exists (a tag-free graphite sink would be byte-identical to plain statsd). So tags are IN-SCOPE, reusing `stats.ExtractTags` (the residual-name + `[]stats.Label` split, phase 47.2b/49). The NOVELTY is purely the FORMAT (§2.7): graphite appends `;k=v` pairs to the metric NAME, before the `:value|type`.

### 2.4 Batching (`max_bytes_per_datagram`): HONORED DAY 1, reusing phase-50 machinery *(Q1 → the real fork)*

The user chose to HONOR `max_bytes_per_datagram` from day 1 rather than defer+strict-reject it (the phase-49 dog_statsd posture). Rationale: the phase-50 batching machinery (`appendLine` flushing the buffer as its own datagram when the next line would STRICTLY exceed the cap; the oversized-single-line-sent-alone rule; `max_bytes_per_datagram == 0` ⇒ one-line-per-datagram) ALREADY EXISTS and is tested. Reusing it for graphite is nearly free and avoids a near-trivial future follow-on row that would only re-wire the same loop. The `>0` PGV validation on `max_bytes_per_datagram` (`value must be greater than 0`) is mirrored. The exact overflow comparison operator + the join separator are SPEC-time pins (D-GR-BATCH, cross-checked against phase-50's D-DSDB-BOUNDARY/D-DSDB-OVERSIZED answers — the same C++ `buildMessage`/datagram-packing code path, so the answers are expected to match, but re-probed to be safe).

### 2.5 Envelope: a SINGLE FLAT ROW *(Q2 → ADR-0275)*

Per §1.4. One row 57, one ADR-0275 (sink only, no seam ADR — the phase-48/49/50/55 single-ADR-on-reuse shape). The ADR-0045 escape valve is documented unconsumed.

### 2.6 The sink lifecycle: a connectionless UDP writer, phase-48-shaped *(self-answered; SPEC confirms, D-GR-LIFECYCLE)*

A `*net.UDPConn` opened once in `NewGraphiteStatsdSink` (resolving the `address` `SocketAddress`), each datagram written inline in `Submit`. A dial error at construction is fatal at `main.go` (the StatsdSink/DogStatsdSink `log.Fatalf` precedent). This is a SECOND, INDEPENDENT UDP conn from any other sink.

### 2.7 The line mapping: `MetricFamily` → graphite-tagged statsd line *(self-answered; SPEC pins, D-GR-TAGFORMAT + D-GR-DELTA + D-GR-PREFIX)*

The load-bearing NOVEL piece. The anticipated line shape is:

```
<prefix>.<residual-name>;<key1>=<value1>;<key2>=<value2>:<value>|c        (COUNTER, per-flush delta)
<prefix>.<residual-name>;<key1>=<value1>;<key2>=<value2>:<value>|g        (GAUGE, absolute)
<prefix>.<residual-name>:<value>|c                                        (no tags → no ; suffix at all)
```

Anticipated but SPEC-PINNED against the reference (`envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm, per `reference_docker_probe_bridge_network` + `reference_stats_sink_metrics_service_probe`):

- **D-GR-TAGFORMAT** — the exact graphite tag grammar: the `;` separator, the `=` key/value delimiter, whether the FIRST tag is separated from the name by `;` (anticipated yes), and the empty-tags line (anticipated: no `;` at all, name flows straight into `:value`, mirroring dog_statsd's no-`|#`-suffix rule).
- **D-GR-DELTA** — does graphite emit `|c` COUNTER deltas (anticipated yes, like statsd/dog_statsd — it is a statsd derivative) or absolute counters? Probed INDEPENDENTLY of phases 48/49 (do not assume — `reference_fixture_workload_constant_desync` teaches that a shared assumption can silently desync).
- **D-GR-PREFIX** — the default prefix when `prefix` is empty. Graphite's proto references `StatsdSink.prefix`, which defaults to `envoy`; anticipated `envoy`, SPEC-confirmed.
- **D-GR-TAGORDER** — the tag emission order: `ExtractTags`'s natural (SN-rule-prepended, unsorted) order (anticipated, the dog_statsd D-DSD-TAGS-ORDER precedent) vs. alphabetical. `reference_dogstatsd_tag_order_unsorted` warns explicitly against sorting a tag-bearing wire string without probing.

### 2.8 Deferred-policy posture: additive config; a NEW `GraphiteStatsdSink` TypeURL dispatch case; the sibling-reject message EXTENDED *(self-answered; pinned at SPEC, D-GR-REJECT)*

The `parseStatsSinks` switch gains a new `case graphiteStatsdSinkTypeURL:`. The `graphiteStatsdSinkTypeURL` is DERIVED from the proto descriptor (`(&graphitestatsdv3.GraphiteStatsdSink{}).ProtoReflect().Descriptor().FullName()`), NOT hand-typed — the `reference_network_filter_typeurl_extensions` discipline (this is a `envoy.extensions.stat_sinks.…` extension type URL, unlike the inline `envoy.config.metrics.v3.…` statsd/dog_statsd type URLs). The extensions proto package is BLANK-IMPORTED so its descriptor registers. The default-case sibling-reject message is EXTENDED to name graphite among the supported sinks (`reference_strict_reject_sibling_typeurl_gap` — lifting one type URL out of silent-ignore needs the reject message updated in lockstep). SPEC pins D-GR-REJECT: which fields, if any, beyond the oneof-required and `max_bytes_per_datagram>0` warrant a strict-reject.

### 2.9 Stat surface hypothesis: zero graphite-sink self-stats *(self-answered; SPEC pins, D-GR-STATS)*

A stats sink EMITS the registry; it does not (in envoy-go) register self-stats. Phases 48/49/50/55 were ALL +0 on the stat surface. Anticipated stat surface **1201 (+0)**. SPEC confirms via D-GR-STATS (the `reference_stats_sink_metrics_service_probe` name-subset method — the reference emits only USED stats; assert a named subset, not the whole registry).

---

## 3. Framework-survey result — a FOURTH Sink on the LANDED flusher + REUSED delta/UDP/tag/batching cores; ZERO new packages, ZERO new go.mod modules (57 anticipated)

### 3.1 Framework: a NEW `GraphiteStatsdSink` (in the EXISTING `internal/statssink`) over the LANDED `Flusher`/`Sink` seam *(per §1.7)*

No new framework piece. The `Sink` interface (`Submit([]*dto.MetricFamily)`) is implemented by a fourth type.

### 3.2 NEW packages: anticipated NONE

`graphite.go` lives in the existing `internal/statssink`; the differential receiver REUSES `test/helpers/statsdrecv` (the UDP receiver already consumed by the `0092`/`0093`/`0094` fixtures — graphite is UDP statsd-format, byte-parseable by the same receiver, or a thin graphite-aware assert layer over it — SPEC decides).

### 3.3 go.mod modules: anticipated NONE

The graphite line protocol is HAND-ROLLED (`strings.Builder` line formatting + `net.DialUDP` — the dog_statsd shape). The `GraphiteStatsdSink` proto resolves at the ALREADY-PRESENT `go-control-plane/envoy v1.32.4` dep (the extension lives in the CORE `/envoy` module — VERIFIED: `.../envoy@v1.32.4/extensions/stat_sinks/graphite_statsd/v3`, NOT `/contrib`). It boots on the contrib reference image because a core stat-sink extension is present in both standard and contrib builds (D-GR-IMAGE confirms). `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

- **phase-47** `Flusher`/`Sink` seam + the `stats.Registry`-walking flush loop (`internal/statssink/flusher.go`, `sink.go`, `mapping.go`).
- **phase-48** the `deltaState` transform (`delta.go`) + the connectionless UDP writer shape (`udp.go`, `NewStatsdSink`).
- **phase-47.2b/49** the `stats.ExtractTags` residual-name + `[]stats.Label` tag core.
- **phase-50** the `appendLine`/`flush` `max_bytes_per_datagram` datagram-packing loop (`dogstatsd.go`).
- **phase-48/49** the `parseUDPSinkAddressAndPrefix` helper + the `stats_sinks[]` typed_config dispatch pattern (`bootstrap.go`).
- **the `0093`/`0094` fixtures'** `test/helpers/statsdrecv` UDP receiver + `differential.HostGatewayIP` (`reference_host_gateway_ip_docker_desktop`).

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener)

The graphite sink is a BOOTSTRAP-level `stats_sinks[]` entry (the phase-47/48/49 surface), NOT a per-listener filter. `parseStatsSinks` reads each entry's `typed_config` type URL and dispatches; graphite adds a fourth `case`. The sink is constructed in `cmd/envoy-go/main.go` after `Load` returns, wired into the periodic `stats_flush_interval` `Flusher` loop alongside any other configured sinks.

---

## 5. Stat surface hypothesis — zero graphite-sink self-stats (57)

### 5.1 Stat names (SPEC pins, D-GR-STATS)

Anticipated: NONE. The sink emits the existing registry; it registers no counters/gauges of its own (the phase-48/49/50/55 posture).

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

Anticipated NONE beyond the standard deferred-policy strict-rejects (§2.8). The graphite tag-order and delta semantics are PARITY, not departures — SPEC confirms.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Differential fixture envelope — anticipated ONE directory `0101-stats-sink-graphite`

### 6.1 Fixtures

ONE new dir `0101-stats-sink-graphite`: a cross-side payload-parity differential — envoy-go and the reference each flush to a per-side driver-owned `statsdrecv` UDP receiver; the driver asserts the received graphite-format lines match cross-side (the payload aggregated across datagrams — `reference_streaming_sink_differential_framing`: assert the PAYLOAD MULTISET, not datagram framing) for a named subset of counter (`|c` delta) + gauge (`|g`) families WITH tags (proving the `;k=v` format). The delta-SUM-with-stability-barrier shape (`reference_delta_sink_differential_stability_barrier` + `reference_periodic_sink_differential_two_receivers`: TWO per-side receivers, hard `Close`, assert the SUM is STILL K after ≥2 further flushes) is REUSED from `0092`/`0093`. A `max_bytes_per_datagram` batching arm (multi-metric datagrams) may be a SECOND fixture `0102-stats-sink-graphite-batching` OR an arm within `0101` — SPEC decides (the phase-50 `0094` precedent was a separate dir; but here batching ships in the SAME row, so an arm is likely cleaner). PIN at SPEC (D-GR-FIXTURE).

### 6.2 Total

Fixtures **102 → 103** (anticipated; `0101-stats-sink-graphite`). If a separate batching dir is chosen, **102 → 104**.

### 6.3 New BackendKind: anticipated NONE

The differential receiver is a driver-owned `statsdrecv` UDP receiver under `test/helpers/` (NOT a runner `BackendKind` — `reference_differential_grpc_receiver_driver_owned` generalized to UDP receivers). BackendKind tail stays **38**.

### 6.4 New fuzzer: anticipated ONE — `FuzzGraphiteStatsdSinkConfigParse`

The `GraphiteStatsdSink` typed_config parse arm (a NEW accepted-config parse path) warrants its own no-panic fuzzer, per the `FuzzStatsdSinkConfigParse`/`FuzzDogStatsdSinkConfigParse` precedent. Fuzzers **53 → 54**. SPEC confirms (D-GR-FUZZER; note `reference_fuzzer_count_docs_drift` — reconcile the documented running total against actual `^func Fuzz` before adding).

---

## 7. Anticipated ADRs — 1 at the phase-57 IMPL: ADR-0275 (the graphite UDP stats sink)

ADR-0275 (the graphite-flavored statsd UDP stats sink — sink only, no seam ADR since the `Flusher`/`Sink`/`delta`/`ExtractTags`/batching substrate is all REUSED). §Context drafted at the SPEC, §Decision/§Consequences land at the IMPL per ADR-0044. Next-free after: ADR-0276.

---

## 8. Deferred items

- **`OTLP-metrics` stats sink** — an OTLP metrics `stats_sinks[]` consumer (`OpenTelemetry` metrics export); carries forward.
- **Tracing `custom_tags`** — per-span custom tags on the phase-46 tracing engine; carries forward.
- **Tracing `spawn_upstream_span`** — a distinct upstream span; carries forward.
- **Tracing `http_service`** — an HTTP tracing backend; carries forward.
- **Tracing force-trace** — `x-envoy-force-trace` header handling; carries forward.
- **`stats_flush_on_admin`** — still rejected (`bootstrap.go:498`); orthogonal, carries forward.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 57 PICKS UP the `graphite` sink from the Observability family's standing deferred-candidate list (the ROADMAP §Observability paragraph: "`graphite`/OTLP-metrics sinks + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace"). After phase 57 the remaining deferred candidates are: `OTLP-metrics` sink + the four tracing sub-features. The family STAYS OPEN.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network` + `reference_stats_sink_metrics_service_probe`)

- **D-GR-TAGFORMAT** — the exact graphite tag grammar (`;` separator, `=` delimiter, first-tag separation, empty-tags line). §2.7.
- **D-GR-DELTA** — `|c` COUNTER delta vs absolute, probed INDEPENDENTLY of phases 48/49. §2.7.
- **D-GR-PREFIX** — the default `prefix` (anticipated `envoy`). §2.7.
- **D-GR-TAGORDER** — `ExtractTags` natural order vs alphabetical (`reference_dogstatsd_tag_order_unsorted`). §2.7.
- **D-GR-BATCH** — the `max_bytes_per_datagram` overflow operator + join separator, cross-checked against phase-50's D-DSDB-* answers (same C++ path, but re-probed). §2.4.
- **D-GR-STATS** — zero graphite-sink self-stats hypothesis. §2.9/§5.
- **D-GR-REJECT** — the deferred-policy strict-reject surface + the sibling-reject message extension. §2.8.
- **D-GR-IMAGE** — does the contrib reference image boot a graphite sink (core extension, anticipated yes, low risk). §3.3.
- **D-GR-FIXTURE** — one dir with a batching arm vs a second batching dir. §6.1.
- **D-GR-FUZZER** — confirm `FuzzGraphiteStatsdSinkConfigParse` earns its place + reconcile the fuzzer running total. §6.4.
- **D-GR-LIFECYCLE** — the connectionless UDP writer lifecycle confirmation. §2.6.

---

## 11. Prior-phase lessons applied

- **`reference_network_filter_typeurl_extensions`** — the graphite type URL is an `extensions.*` extension; DERIVE it from the proto descriptor, BLANK-IMPORT the package, never hand-type the string. §2.8.
- **`reference_strict_reject_sibling_typeurl_gap`** — extend the default-case sibling-reject message to name graphite in lockstep with adding the accept case. §2.8.
- **`reference_dogstatsd_tag_order_unsorted`** — do NOT sort the tags without probing; the reference emits `ExtractTags`'s natural order. §2.7.
- **`reference_delta_sink_differential_stability_barrier`** + **`reference_periodic_sink_differential_two_receivers`** — the delta-SUM-with-stability-barrier + TWO per-side receivers + hard Close differential shape. §6.1.
- **`reference_streaming_sink_differential_framing`** — assert the PAYLOAD multiset aggregated across datagrams, not datagram framing. §6.1.
- **`reference_stats_sink_emits_used_only`** + **`reference_stats_sink_metrics_service_probe`** — the reference emits only USED stats; assert named subsets. §2.9.
- **`reference_host_gateway_ip_docker_desktop`** — a container reaches a host receiver at the host-gateway literal IP (`differential.HostGatewayIP`). §3.4.
- **`reference_fuzzer_count_docs_drift`** — reconcile the fuzzer running total before adding one. §6.4.
- **`feedback_brief_citations_not_evidence`** — every `file:line` in this BRAINSTORM (the `bootstrap.go` dispatch sites, the proto field paths) is to be RE-DERIVED from source at the SPEC, never verified against this document.
- **`reference_docker_probe_bridge_network`** + **`reference_probe_fresh_container_per_arm`** — SPEC probes run on a Docker bridge network with a FRESH container per arm (`docker logs` accumulates across restart).

---

## 12. Section closeout

**Settled:** subject (graphite stats sink, Q0); transport (UDP-only, self-answered by the proto); tags (IN-SCOPE, self-answered by graphite's purpose); batching (`max_bytes_per_datagram` HONORED DAY 1 reusing phase-50 machinery, Q1); envelope (SINGLE FLAT ROW, Q2 → ADR-0275). The ONE novel piece is the graphite tag-format function; all else reuses landed, tested substrate.

**Anticipated moves at the phase-57 IMPL (docs-only now):** a NEW `internal/statssink/graphite.go` `GraphiteStatsdSink` + `parseGraphiteStatsdSinkConfig` arm + `GraphiteStatsdSinkConfig` struct + a fourth `main.go` build loop + `FuzzGraphiteStatsdSinkConfigParse` + the `0101-stats-sink-graphite` cross-side differential (reusing `statsdrecv`). Counts: stat surface **1201 (+0)** · fixtures **102 → 103** (or → 104 if a separate batching dir) · fuzzers **53 → 54** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0275** (next-free **ADR-0276**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `91db1512`):** stat surface **1201** · fixtures **102** · fuzzers **53** · BackendKind **38** · DECISIONS tail **ADR-0274** (next-free **ADR-0275**). Row 57 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-57 SPEC** (live-probe D-GR-* against `envoyproxy/envoy:contrib-v1.37.2`).
