# Phase 50 Brainstorm — `dog_statsd max_bytes_per_datagram` batching (the SEVENTH row of the Observability family; a transport-layer-only change over the LANDED phase-49 `internal/statssink/dogstatsd.go` `DogStatsdSink` emitter; a SINGLE FLAT ROW, no new `Sink`, no new package)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, direct-on-master (the phase-44/45/46/47/48/49 family-row BRAINSTORM precedent). Row 50 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated).
>
> **The pick is already made** (`max_bytes_per_datagram` batching — a human decision via the loop re-open, the cheapest remaining deferred Observability candidate per the row-49 ROADMAP note). This BRAINSTORM settles the SCOPE, not the "which row" question.
>
> **User dialogue (4 questions, 2026-07-05):**
> - **Q1 — differential proof-shape → a NEW fixture, `0094-stats-sink-dogstatsd-batching`.** Not an in-place extension of `0093`. Keeps the no-batching baseline (`0093`) and the batching proof (`0094`) as independent, uncluttered fixtures.
> - **Q2 — oversized-single-line posture → MIRROR THE REFERENCE: send it alone, uncapped for that one line.** The `go-control-plane/envoy@v1.32.4` `stats.pb.go:707-713` doc-comment on `DogStatsdSink.max_bytes_per_datagram` is explicit: *"this value may not be respected if smaller than a single metric."* No drop, no truncation, no envoy-go-strict departure.
> - **Q3 — degenerate cap value (e.g. an explicit `0`) → ACCEPT IT, no new strict-reject.** Combined with Q2's oversized-line rule, an explicit `0` (or any cap smaller than any real line) self-degrades to "every line sent alone" — behaviorally IDENTICAL to phase 49's unconditional one-per-datagram. No special-cased validation needed.
> - **Q4 — envelope → SINGLE FLAT ROW.** A buffering/flush-on-overflow change inside the existing `Submit` loop + one new config field + one new differential fixture. No new `Sink`, no new package. ADR-0045 escape-valve UNCONSUMED.
>
> **Self-answered (verified against the proto, not assumed):** the unset/absent-field default is confirmed, from the SAME doc-comment, to be "Envoy will emit one metric per datagram" — i.e. phase 49's existing unconditional behavior is ALREADY reference-parity when the field is absent. This is NOT a live-probe finding (no Docker probe was run this session); it is read directly from `go-control-plane/envoy@v1.32.4` `config/metrics/v3/stats.pb.go:707-713`, which mirrors the upstream `.proto` API-doc comment. The SPEC still pins this live (D-DSDB-DEFAULT below) per this project's standing discipline that docs are not the wire truth (`reference_wire_format_both_sides_see_same_bytes`) — but no surprise is expected here.

---

## 1. Mission and scope confirmation (50 — a single flat row, transport-layer only)

### 1.1 What phase 50 delivers as a self-contained whole (real multi-metric datagram packing)
Lifts the phase-49 STRICT-REJECT at `internal/bootstrap/bootstrap.go:591` (`parseDogStatsdSinkConfig`: `if dsd.GetMaxBytesPerDatagram() != nil { return fmt.Errorf(...) }`) into a genuine accept-and-honor path: a new `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` field (`0` meaning "no cap — one metric per datagram," the phase-49 default, UNCHANGED), threaded into `DogStatsdSink`, which now accumulates newline-joined DogStatsd lines into a growing buffer per flush and writes ONE UDP datagram per buffer-full, instead of unconditionally one datagram per line.

### 1.2 What phase 50 does NOT deliver (forward to §8)
Any change to the line-formatting/tag-extraction/delta-state logic (batching is purely a TRANSPORT-layer concern applied AFTER line formatting — Q3 in the router's framing, self-answered YES, orthogonal); any change to `StatsdSink` (`StatsdSink` has no `max_bytes_per_datagram`-equivalent field — confirmed absent from `stats.pb.go`'s `StatsdSink` message); `graphite`/OTLP-metrics sinks; the plain-statsd `tcp_cluster_name` transport; the tap filter; tracing extras.

### 1.3 Phase-done as the SEVENTH Observability-family row landing (family STAYS OPEN)
Row 50 is the SEVENTH Observability-family row (after gRPC-ALS @ 44, OTLP-log @ 45, tracing @ 46, metrics_service @ 47, statsd @ 48, dog_statsd @ 49). The family STAYS OPEN — remaining deferred candidates: `graphite`/OTLP-metrics sinks, the plain-statsd `tcp_cluster_name` transport, tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace, the tap filter. NO parent rollup (ADR-0106); row 50 flips `done` at the phase-50 IMPL six-gate.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed)
The buffering change lives entirely inside the already-landed `DogStatsdSink.Submit` loop; no second subsystem, no new seam. A pre-authorized 50.1/50.2 escape-valve stays UNCONSUMED per ADR-0045 unless the SPEC surfaces unexpected size (anticipated smaller than phase 49 — no new tag/delta logic, just a buffer-accumulate-then-flush loop + one config field + one differential; ~80–150 prod LoC anticipated).

### 1.5 Seed-stub alignment + package placement
The batching logic lives in the EXISTING `internal/statssink/dogstatsd.go` (extending `DogStatsdSink.Submit`, or a small sibling helper in the same file). NO new package, NO new go.mod module.

### 1.6 No prebrainstorm-notes branch
No off-master prebrainstorm-notes branch exists for this row.

### 1.7 Phase 50's relationship to the existing seams (a transport-layer change over the LANDED phase-49 emitter)
REUSES: the `Flusher`/`Sink` seam (unchanged); the `DogStatsdSink` line-formatting + tag-extraction + sink-private `deltaState` (UNCHANGED — batching operates on the already-formatted line strings, never touching delta/tag computation); the `bootstrap.go` `parseDogStatsdSinkConfig` arm (RELAXED, not replaced); the `0093` differential harness shape (driver-owned UDP receiver, two-per-side-receivers + hard-`Close()`) as the template for a NEW `0094` fixture. NEW: `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` + a buffer-accumulate-then-flush-on-overflow loop replacing `Submit`'s current one-`Write`-per-line call + a `test/helpers/statsdrecv` receiver-side change to split a received datagram's payload on `\n` into individual lines before per-line ingest (a receiver-side change only — the wire format itself, one-or-more `\n`-joined DogStatsd lines per datagram, needs no new grammar on the line level, only a pre-split at the datagram-ingest boundary).

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with dog_statsd batching *(Q0 → phase 50 row registered)*
The loop was RE-OPENED; a human picked `max_bytes_per_datagram` batching from the deferred Observability candidates (the cheapest remaining follow-on — reuses the phase-49 emitter, bootstrap parse arm, and differential harness shape unchanged).

### 2.2 Config surface: `MaxBytesPerDatagram uint64`, no pointer needed *(self-answered, from Q3)*
The proto field is a `*wrapperspb.UInt64Value` (nilable — distinguishes "absent" from "explicitly zero"). Ordinarily this would suggest a `*uint64` on `DogStatsdSinkConfig` to preserve that distinction. But per Q3 ("accept it"), an explicit `0` and an absent field produce IDENTICAL runtime behavior under the Q2 oversized-line rule: a cap of `0` makes every line "larger than the cap" by definition, so every line is sent alone — the SAME as phase 49's unconditional one-per-datagram behavior. The distinction is therefore NOT load-bearing; `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (plain, no pointer) is sufficient, parsed via `dsd.GetMaxBytesPerDatagram().GetValue()` (which already returns `0` for a nil wrapper). This ELIMINATES a whole class of nil-vs-zero bugs before they can occur — a design simplification, not a corner cut.

### 2.3 The packing algorithm: sequential accumulate-then-flush-on-overflow *(self-answered; SPEC pins the exact boundary rule, D-DSDB-BOUNDARY)*
Walk the batch's formatted lines (UNCHANGED from phase 49 — delta/tag computation happens first, exactly as today) in the SAME order the registry walk produces them (no sort, no reorder — Q3 in the router's original framing, self-answered YES, orthogonal to delta/tag computation per §1.7). Maintain a growing buffer:
- For each line, compute the prospective buffer size if the line were appended (accounting for the `\n` separator when the buffer is already non-empty).
- If the buffer is non-empty AND appending would exceed `MaxBytesPerDatagram`, flush the current buffer as one UDP datagram FIRST, then start a new buffer with this line.
- If the buffer is EMPTY and this single line already exceeds `MaxBytesPerDatagram` on its own, send it alone as its own (oversized) datagram immediately (Q2) and leave the buffer empty for the next line.
- After all lines are processed, flush any non-empty remaining buffer.
- `MaxBytesPerDatagram == 0` (unset or explicit degenerate zero, §2.2) is NOT a special-cased branch — it falls out of the general algorithm naturally, since every line is immediately "too large for an empty 0-byte cap" and gets the oversized-alone treatment, exactly reproducing phase 49's per-line-datagram behavior with NO conditional fork in the code.

The EXACT overflow comparison (does a buffer that lands EXACTLY at the cap after appending count as full, i.e. is the check `>` or `>=` against the cap) is a SPEC-time live pin (D-DSDB-BOUNDARY) — not assumed here.

### 2.4 Activation scope: no separate on/off switch — the general algorithm subsumes both states *(self-answered per §2.3; resolves the router's Q1)*
There is no branch that says "if `max_bytes_per_datagram` is set, batch; else don't." The SAME accumulate-then-flush loop runs unconditionally; the cap value alone determines whether it ever accumulates more than one line per datagram. This is simpler than a feature-flagged code path and cannot regress phase 49's behavior when the field is absent (§2.2 shows why `0` and absent coincide).

### 2.5 Oversized-single-line handling: send alone, no error, no drop *(Q2 → confirmed by the proto doc-comment)*
Already covered in §2.3 — the empty-buffer branch. No log line, no error, no envoy-go-strict departure: this is ordinary, expected, reference-documented behavior, not a degenerate case.

### 2.6 Config validation: no new strict-reject *(Q3)*
The existing `parseDogStatsdSinkConfig` strict-reject for `max_bytes_per_datagram != nil` is REMOVED entirely (not narrowed — there is no remaining condition under which this field warrants a reject). All other phase-49 strict-rejects (missing `address`/`socket_address`, unsupported sibling TypeURLs) are UNCHANGED.

### 2.7 Differential proof-shape: a NEW fixture `0094-stats-sink-dogstatsd-batching` *(Q1)*
Clones the `0093` two-per-side-UDP-receiver + hard-`Close()` shape (`reference_periodic_sink_differential_two_receivers`), configured with a deliberately small `max_bytes_per_datagram` (large enough to hold ≥2 typical lines, small enough to force at least one multi-line datagram under the fixture's request load). The driver-owned receiver (`test/helpers/statsdrecv`, extended — NOT a BackendKind per `reference_differential_grpc_receiver_driver_owned`) must split each received datagram's payload on `\n` BEFORE per-line ingest. Assertions (per the router's §6 Q6 sketch): (a) the emitted line SET is unchanged from an unbatched run (same subset-by-name/tags — batching must not alter WHAT is sent, only how it's grouped into datagrams); (b) at least one RECEIVED datagram actually contains more than one line (proof that batching occurred, not merely that the parser tolerates a multi-line payload it never actually receives); (c) no received datagram exceeds the configured cap, EXCEPT a deliberately-crafted oversized-single-line case which is asserted to be sent alone and exceed the cap on its own (proving §2.5, not violating it).

### 2.8 Receiver-parser risk: trace an example multi-line datagram before editing `statsdrecv` *(flagged; `reference_line_parser_extension_delimiter_reuse`)*
`test/helpers/statsdrecv`'s `ingest` already does a delimiter-based split (the phase-49-era first-`|`-then-colon split, per STATE.md). Introducing a NEW top-level `\n`-split stage BEFORE that per-line parsing must not collide with any `\n` that could already appear inside a line (none currently do — DogStatsd lines have no internal newlines) but the SPEC must trace one concrete multi-line datagram byte-for-byte before touching the parser, per the standing lesson from the phase-49 tag-suffix extension.

### 2.9 Stat surface hypothesis: zero new self-stats *(self-answered; SPEC pins, D-DSDB-STATS)*
A transport-layer buffering change carries no new counters/gauges — the metrics_service/statsd/dog_statsd +0 precedent continues. Anticipated stat surface UNCHANGED at 1200.

---

## 3. Framework-survey result — a buffering change inside the LANDED `DogStatsdSink`; ZERO new packages, ZERO new go.mod modules, ZERO new `Sink` impls (50 anticipated)

### 3.1 Framework: no new framework piece
No new `Sink`, no new flush loop, no new typed-client layer. `Flusher` (`flusher.go`) is untouched; `DogStatsdSink.Submit` (`internal/statssink/dogstatsd.go`) gains a buffering stage between line-formatting and `Write`.

### 3.2 NEW packages: anticipated NONE.

### 3.3 go.mod modules: anticipated NONE. Pure Go buffer/string manipulation; no client library. `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES
`DogStatsdSink`'s line formatting, tag extraction (`stats.ExtractTags`), and sink-private `deltaState` (ALL unchanged); the `Flusher`/`Sink` seam; the `bootstrap.go` `stats_sinks[]` dispatch (only `parseDogStatsdSinkConfig`'s body changes, not the dispatch arm itself); the `0093` differential harness shape (driver-owned two-per-side receivers + hard `Close()`) as the `0094` template; `test/helpers/statsdrecv` (extended in-place with a datagram-level `\n`-pre-split, per §2.8).

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener), same DogStatsdSink entry
No change to WHERE this config lives — still the top-level `Bootstrap.stats_sinks[]` `DogStatsdSink` entry, just one more field on it (`max_bytes_per_datagram`) now honored instead of rejected.

---

## 5. Stat surface hypothesis — zero new self-stats (50)

### 5.1 Stat names (SPEC pins, D-DSDB-STATS)
Anticipated NONE (the metrics_service/statsd/dog_statsd +0 precedent).

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)
NONE NEW. The phase-49 `max_bytes_per_datagram`-set strict-reject is REMOVED (§2.6), not replaced by a narrower one. All other phase-49 rejects (missing address/socket_address, unsupported sibling TypeURL) carry forward unchanged.

### 5.3 Anticipated surface arithmetic
Stat surface **1200 → 1200** (+0 anticipated; non-H2 1196). SPEC/IMPL pin.

---

## 6. Differential fixture envelope — anticipated ONE NEW directory `0094-stats-sink-dogstatsd-batching`

### 6.1 Fixtures
`0094-stats-sink-dogstatsd-batching`: clones the `0093` cross-side shape (driver-owned UDP receiver, NOT a BackendKind; two per-side receivers + hard `Close()`), configured with a small `max_bytes_per_datagram`. Extends `test/helpers/statsdrecv` with a datagram-level `\n`-pre-split (§2.8). Asserts: (a) the emitted line set is unchanged from an unbatched baseline; (b) at least one received datagram actually contains >1 line; (c) no datagram exceeds the cap except a deliberately-oversized single line, which is asserted to exceed it alone (§2.7). Differential discipline carries: `-run 'TestDifferential/0094'` NEVER bare (`reference_differential_run_selector`); `-count=1` on every break + `-race` (`reference_differential_break_protocol_count1`); the Docker bridge + a decode-ran proof (`reference_docker_probe_bridge_network`); the live reference is the wire truth (`reference_wire_format_both_sides_see_same_bytes`).

### 6.2 Total
fixtures **95 → 96** (`0094`).

### 6.3 New BackendKind: anticipated NONE (driver-owned UDP receiver, BackendKind stays 38).

### 6.4 New fuzzer: anticipated NONE, but NOT YET SETTLED (D-DSDB-FUZZER, SPEC pins). `max_bytes_per_datagram` is an EXISTING field on an EXISTING parse arm (`parseDogStatsdSinkConfig`) — the EXISTING `FuzzDogStatsdSinkConfigParse` already exercises arbitrary bytes through this same message, including this field, once the strict-reject is removed. UNLIKE phase 48/49's fuzzers (each guarding a brand-NEW dispatch arm for a brand-new sink TypeURL), this row adds no new dispatch arm. A SEPARATE consideration: the packing/buffering ALGORITHM itself (§2.3) is pure Go logic over a `[]string` of lines and a `uint64` cap — not a protobuf-parse boundary, so it does not fit this project's "one fuzzer per new accepted-config parse path" convention. SPEC decides whether a dedicated algorithmic fuzzer (e.g., panic-safety over adversarial cap/line-length combinations) is warranted anyway. Fuzzers anticipated to STAY AT 52 unless SPEC finds reason otherwise.

---

## 7. Anticipated ADRs — 1 at the phase-50 IMPL: ADR-0267 (the dog_statsd batching transport change)
ADR-0267 (ACCEPTED — real multi-metric datagram packing lifted from the phase-49 strict-reject; §Context drafted at the SPEC, §Decision/§Consequences landed in-place at the IMPL per ADR-0044). NO seam ADR (the `Flusher`/`Sink` seam and the `DogStatsdSink` line/tag/delta logic are all reused unchanged — this ADR is scoped to the packing algorithm alone). next-free after 50: ADR-0268.

---

## 8. Deferred items
- `graphite` / `open_telemetry`-metrics sinks (each its own future deferred Observability row).
- the plain-statsd `tcp_cluster_name` transport (a DIFFERENT sink, already deferred at phase 48 — not re-litigated here).
- timers/`|ms` (gated by the absent histogram boundary — ADR-0060, unchanged from prior rows).
- the tap filter + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace (unrelated Observability-family candidates, unchanged from the phase-49 deferred list).
- ANY change to `StatsdSink` (plain statsd) — confirmed to have no `max_bytes_per_datagram`-equivalent field; not a candidate for this pattern.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup
Picks up the phase-49 ROADMAP row's "`max_bytes_per_datagram` batching" deferred candidate (charted as phase 50 on 2026-07-05). The remaining deferred candidates (`graphite`/OTLP-metrics/plain-statsd-`tcp_cluster_name`/tracing-extras/tap) carry forward UNbrainstormed.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network`)
- **D-DSDB-DEFAULT:** live-confirm (not just doc-confirm) that an absent `max_bytes_per_datagram` emits one metric per datagram against `contrib-v1.37.2` — low-risk given the explicit proto doc-comment, but the project's standing discipline is the wire is the truth, not the docs (`reference_wire_format_both_sides_see_same_bytes`).
- **D-DSDB-BOUNDARY (LOAD-BEARING):** the exact overflow comparison — does a buffer that would land EXACTLY at the cap after appending the next line count as "still fits" (`<=`) or "already full" (`<`)? Confirm via a live probe with a deliberately tiny cap sized to land exactly on a line boundary.
- **D-DSDB-OVERSIZED:** confirm the reference truly sends an over-cap single line alone with no error/drop/truncation (per the proto doc-comment, §2.5) — and confirm it does NOT, e.g., split a single line's tag suffix across two datagrams (expected: a line is always atomic; the reference has no mechanism to split a line, only to decide whether to co-locate it with others).
- **D-DSDB-JOIN-ORDER:** confirm the reference does not reorder/sort/dedupe lines when packing multiple into one datagram (expected: pure sequential accumulation in emission order, orthogonal to delta/tag computation, per §2.3/§1.7).
- **D-DSDB-FUZZER:** does the packing algorithm warrant its own dedicated fuzzer, or does reuse of the existing `FuzzDogStatsdSinkConfigParse` (now exercising the un-rejected field) suffice? Anticipated: no new fuzzer (§6.4).

---

## 11. Prior-phase lessons applied
- `reference_periodic_sink_differential_two_receivers` — TWO per-side receivers + hard `Close()` for a periodic sink (the `0094` template, cloned from `0093`).
- `reference_differential_grpc_receiver_driver_owned` — the UDP receiver is a driver-owned `test/helpers` server, NOT a BackendKind.
- `reference_differential_run_selector` / `reference_differential_break_protocol_count1` — `-run 'TestDifferential/0094'` never bare; `-count=1` on every break + `-race`.
- `reference_line_parser_extension_delimiter_reuse` — extending `statsdrecv` with a datagram-level `\n`-pre-split needs an example multi-line datagram traced first, not just described (§2.8).
- `reference_dogstatsd_tag_order_unsorted` — unrelated to this row (batching doesn't touch tag formatting), but noted since `0094` reuses the SAME extended `statsdrecv` receiver that carries this precedent.
- `reference_docker_probe_bridge_network` / `reference_wire_format_both_sides_see_same_bytes` — the SPEC probe runs on a shared Docker bridge with a decode-ran proof; the wire, not the proto doc-comment, is the final authority (§10 D-DSDB-DEFAULT).
- `reference_host_gateway_ip_docker_desktop` — reuse `differential.HostGatewayIP`/the `0093` driver's local `hostGatewayIP` helper verbatim if `0094` needs the same literal-IP reference→host UDP reachability shape.
- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; controller squashes + pushes at stage-close.

---

## 12. Section closeout
- **Subject:** the `DogStatsdSink.max_bytes_per_datagram` field — real multi-metric newline-batched UDP datagram packing, lifted from the phase-49 strict-reject, over the LANDED `internal/statssink/dogstatsd.go` `DogStatsdSink` emitter.
- **Q1 fixture strategy:** a NEW dedicated fixture `0094-stats-sink-dogstatsd-batching` (not an in-place `0093` extension).
- **Q2 oversized-single-line:** mirror the reference — send alone, uncapped for that one line, no drop/truncation (confirmed by the `stats.pb.go` doc-comment).
- **Q3 degenerate cap value:** accept any explicit value including `0`; no new strict-reject (§2.2/§2.6 show `0` and absent are behaviorally identical under the packing algorithm).
- **Q4 envelope:** SINGLE FLAT ROW (escape-valve unconsumed under ADR-0045).
- **Scope:** remove the `bootstrap.go:591` `max_bytes_per_datagram`-set strict-reject + add `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (plain, no pointer) + a buffer-accumulate-then-flush-on-overflow rewrite of `DogStatsdSink.Submit`'s per-line `Write` loop (delta/tag computation UNCHANGED) + the `test/helpers/statsdrecv` datagram-level `\n`-pre-split + the `0094-stats-sink-dogstatsd-batching` cross-side differential.
- **Anticipated counts:** stat **1200** (+0) / fixtures **96** (`0094`) / fuzzers **52** (anticipated UNCHANGED, D-DSDB-FUZZER pins) / BackendKind **38** / DECISIONS **ADR-0267** (next-free ADR-0268); ZERO new packages, ZERO new go.mod modules.
- **Load-bearing SPEC probes:** D-DSDB-BOUNDARY (the exact overflow comparison operator) + D-DSDB-DEFAULT (live-confirm the absent-field default) + D-DSDB-OVERSIZED (confirm no splitting/dropping of an over-cap line) + D-DSDB-JOIN-ORDER (confirm no reordering).
- **Row 50** registers `in-progress` at this BRAINSTORM commit; flips `done` at the phase-50 IMPL six-gate (NO parent rollup — ADR-0106). The Observability FAMILY STAYS OPEN.
- **Next → the phase-50 SPEC** (`SPEC-50.md` — execute the §10 D-DSDB-* live pins against `contrib-v1.37.2`; anchor the ADR-0267 §Context draft; docs-only direct-on-master).
