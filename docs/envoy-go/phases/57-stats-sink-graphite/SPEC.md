# SPEC 57 — `graphite_statsd` stats sink (`envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink` → a periodic UDP graphite-flavored statsd-line sink over the LANDED phase-47 `Flusher`/`Sink` seam; the FOURTH `stats_sinks[]` consumer; tags appended to the metric NAME as `;k=v` pairs; `max_bytes_per_datagram` batching HONORED DAY 1 via the phase-50 machinery; a SINGLE FLAT ROW — ADR-0275)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; no production `.go` changes. Fresh worktree `.worktrees/phase-57-spec`, branch `phase-57-stats-sink-graphite-spec`, per `feedback_git_worktrees`.
>
> **Baselines re-verified against the master tip `136d6a00` (the post-BRAINSTORM router refresh; the BRAINSTORM squash `d2fe54df` sits two test-only parallel-workstream commits + one router commit below):** stat surface **1201** · fixtures **102** (tail `0100-http-tap-bodies`) · fuzzers **53** (`grep -rc '^func Fuzz'` reconciled) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0274** (next-free **ADR-0275**) · new Go packages **0** · new go.mod modules **0**. Counts are UNCHANGED at a SPEC (docs-only).

---

## 1. Purpose / Mission

Pin the behavior of a NEW `internal/statssink/graphite.go` `GraphiteStatsdSink` — the FOURTH `stats_sinks[]` consumer over the landed phase-47 `Flusher`/`Sink` seam — that on each flush applies a sink-private per-flush counter-delta transform, extracts residual-name + tags via `stats.ExtractTags`, formats **graphite-flavored statsd lines** (`<prefix>.<residual>[;key=value;…]:<value>|<c|g>` — tags embedded in the metric NAME, CONTRAST dog_statsd's trailing `|#k:v` suffix), packs lines into datagrams under the `max_bytes_per_datagram` cap via the phase-50 `appendLine`/`flush` machinery, and writes each datagram over a connectionless UDP socket. Plus: a `parseGraphiteStatsdSinkConfig` typed-extension dispatch arm in `internal/bootstrap/bootstrap.go`, a fourth `cmd/envoy-go/main.go` build loop, `FuzzGraphiteStatsdSinkConfigParse`, and the `0101-stats-sink-graphite` cross-side differential.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

All D-GR-* pins were executed live IN-SESSION 2026-07-11 against `envoyproxy/envoy:contrib-v1.37.2` (§11). Findings that CONFIRM or RESHAPE the BRAINSTORM's anticipated design:

- **AMEND-GR-TAGFORMAT-CONFIRMED (D-GR-TAGFORMAT).** The graphite tag grammar is exactly the BRAINSTORM's anticipated shape: each tag appends to the metric NAME as `;<key>=<value>` (the FIRST tag too — `;` separates it from the name), keys carry the dotted `envoy.` form, and a tag-free line carries NO `;` at all (name flows straight into `:value`). Live: `envoy.cluster.external.upstream_rq_xx;envoy.response_code_class=2;envoy.cluster_name=c1tag:3|c` (two tags) and `envoy.listener.admin.downstream_cx_total:1|c` (no tags). **Consequence:** the ONE novel piece is a ~10-LoC `graphiteTagSuffix` that folds tags INTO the name argument of the reused `emitStatsdLines` skeleton; the trailing-suffix argument stays `""` (§3.3).
- **AMEND-GR-DELTA-CONFIRMED (D-GR-DELTA, probed INDEPENDENTLY of phases 48/49).** Counter `|c` is a per-flush DELTA: `envoy.cluster.upstream_rq_total;envoy.cluster_name=c1tag` emitted `3|c` after the first 3 requests, `0|c` on quiet flushes, `2|c` after 2 more, then `0|c` — SUM 5 == K == the admin ground truth `cluster.c1tag.upstream_rq_total: 5`. Zero-delta counters ARE re-emitted every flush (`0|c`), and GAUGE `|g` is ABSOLUTE (`envoy.server.memory_allocated:6797120|g` then `6797392|g`…). **Consequence:** reuse `delta.go` VERBATIM as a FOURTH sink-private `newDeltaState()` instance; the `0101` differential uses the delta-SUM + stability-barrier shape.
- **AMEND-GR-PREFIX-CONFIRMED (D-GR-PREFIX).** Default prefix is `envoy` (every no-`prefix` line starts `envoy.`); an explicit `prefix: probepfx57` prefixed 604/604 captured lines. **Consequence:** reuse `parseUDPSinkAddressAndPrefix`'s `""→"envoy"` defaulting verbatim (§3.1).
- **AMEND-GR-TAGORDER-CONFIRMED (D-GR-TAGORDER; `reference_dogstatsd_tag_order_unsorted` honored — probed, not sorted).** The reference emits tags in its extractor's NATURAL order, NOT alphabetical: `…upstream_rq_xx;envoy.response_code_class=2;envoy.cluster_name=c1tag` puts `response_code_class` FIRST even though `cluster_name` appears first in the dotted name. **NEW cross-side trap:** envoy-go's `stats.ExtractTags` appends `envoy_cluster_name` FIRST (SN1, `internal/stats/name.go:51-60`) then the status-class label — the REVERSE of the reference's wire order on two-tag names. **Consequence:** envoy-go emits ITS OWN `ExtractTags` natural order unsorted (the dog_statsd `formatTagSuffix` precedent, `internal/statssink/dogstatsd.go:151-167`); the `0101` differential asserts tags as a SET (`Tags()`/`DeltaSumTagged`, the `0093` `maps.Equal` shape), NEVER as an ordered string. Per-side tag ORDER is a pinned COVERAGE BOUNDARY (unasserted cross-side).
- **AMEND-GR-BATCH-CONFIRMED (D-GR-BATCH, re-probed; matches phase-50's D-DSDB-* answers — same C++ datagram-packing path).** At cap=160: 216 multi-line datagrams, lines joined by a single `\n`, **12 datagrams at EXACTLY len=160** and ZERO above it ⇒ an exact-fit is NOT an overflow ⇒ the comparison is strict `>`; NO datagram ends in `\n` (`\n`-separated, not `\n`-terminated). At cap=120 with a 130-char cluster name: 147 over-cap datagrams (max 223 bytes), **100% single-line** (oversized-line-sent-alone, never co-batched, zero truncation) while 75 under-cap datagrams still multi-line. With the field ABSENT: 1373/1373 datagrams single-line (one-line-per-datagram). **Consequence:** reuse the phase-50 `appendLine`/`flush` machinery VERBATIM (strict-`>` prospective check incl. the `+1` separator byte, empty-buffer-always-accepts, per-call `strings.Builder`; `internal/statssink/dogstatsd.go:122-144`).
- **AMEND-GR-REJECT-CONFIRMED + ONE NEW ARM (D-GR-REJECT).** The reference boot-rejects: (i) a missing `statsd_specifier` — `Proto constraint validation failed (field: "statsd_specifier", reason: is required)`; (ii) an EXPLICIT `max_bytes_per_datagram: 0` — `Proto constraint validation failed (GraphiteStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`; (iii) a hostname `address` — `malformed IP address: host.docker.internal` (the statsd AMEND-SD-REJECT posture). **Consequence + a caught internal contrast:** envoy-go mirrors (i) via `parseUDPSinkAddressAndPrefix` and adds a NEW explicit-zero reject arm for (ii) — possible because the proto field is a `*wrapperspb.UInt64Value` (nil ⇔ absent is distinguishable from an explicit 0). NOTE the landed dog_statsd parse does NOT have this arm (`bootstrap.go:678` consumes `.GetValue()` unchecked) even though `DogStatsdSink` carries the IDENTICAL PGV `gt: 0` rule (re-derived from `stats.pb.validate.go:1144-1150`) — a pre-existing phase-50 parity gap, OUT OF SCOPE here, recorded as a deferred candidate (§2). For (iii) envoy-go keeps the inherited `net.ResolveUDPAddr` behavior (accepts hostnames) — the DOCUMENTED phase-48/49 DEPARTURE, unchanged.
- **AMEND-GR-NO-SELF-STATS-CONFIRMED (D-GR-STATS).** `admin /stats | grep -iE 'statsd|graphite'` is EMPTY on a flushing reference — the graphite sink registers ZERO self-stats. Stat surface **1201 (+0)**.
- **AMEND-GR-IMAGE-CONFIRMED (D-GR-IMAGE).** The contrib image boots + emits (arm A1: 1373 datagrams). The STANDARD `envoyproxy/envoy:v1.37.2` image ALSO boots + emits (281 datagrams) — graphite is a CORE extension, consistent with the proto living in the core `go-control-plane/envoy@v1.32.4` module (re-derived: `…/envoy@v1.32.4/extensions/stat_sinks/graphite_statsd/v3/graphite_statsd.pb.go`, NOT `/contrib`; ZERO new go.mod modules).
- **AMEND-GR-EXACTCODE-SUBSET (NEW, from the A1 capture).** The reference collapses EXACT-code counters by stripping the code entirely: `cluster.c1tag.external.upstream_rq_200` → wire `envoy.cluster.external.upstream_rq;envoy.response_code=200;envoy.cluster_name=c1tag`. envoy-go's `ExtractTags` has no exact-code rule — so the `0101` subset must avoid `_NNN` exact-code names (the `0093` precedent already avoided them; the class-collapsed `_xx` names ARE cross-side-compatible).

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0275 §Context DRAFTS here (§13); §Decision/§Consequences land at the phase-57 IMPL per ADR-0044. DECISIONS tail STAYS **ADR-0274** at this SPEC. All eleven BRAINSTORM §10 D-GR-* questions are DISPOSED: nine PINNED empirically (§11), D-GR-FIXTURE decided (ONE dir, batching folded in — §8.1), D-GR-FUZZER confirmed (§6). Two D-questions carry to PLAN/IMPL (§12).

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

No `tcp_cluster_name` (the proto oneof has no such arm — UDP-only by construction). No histogram surface (envoy-go has none — ADR-0060; the reference's per-observation `|ms` timer lines are UNasserted, §8.1). No tag-value escaping/sanitization beyond the existing sink posture (config-derived values; the `stats.IsValidName` charset excludes `;`/`=`, so envoy-go names cannot collide with the tag grammar). Untouched Observability deferred candidates: OTLP-metrics sink + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace. **NEW deferred candidate (recorded by this SPEC):** the dog_statsd explicit-`max_bytes_per_datagram: 0` parse arm — the reference PGV-rejects it (`gt: 0` on `DogStatsdSink`, identical to graphite's) while envoy-go's phase-50 parse consumes it as one-line-per-datagram; a one-line parity fix candidate for a future robustness sweep, NOT this row. `stats_flush_on_admin` stays rejected (`bootstrap.go:497-499`); orthogonal.

## 3. The sink — a NEW `internal/statssink/graphite.go` `GraphiteStatsdSink` + a `parseGraphiteStatsdSinkConfig` arm + a `GraphiteStatsdSinkConfig` + a `cmd/envoy-go/main.go` fan-out (ADR-0275)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED (re-checked at PLAN with real LoC)

Q2 settled a SINGLE FLAT ROW. The task envelope re-confirms at ~9–12 tasks (§10), well under the `~15` ceiling — every substrate piece (delta, UDP writer, tags, batching, receiver, differential shape) is landed and tested; the novel surface is one ~10-LoC format function + one dispatch case + one merged differential. D-GR-SPLIT re-checks at PLAN.

### 3.1 Config parse — a NEW typed-extension dispatch case + `parseGraphiteStatsdSinkConfig` + `GraphiteStatsdSinkConfig` (`internal/bootstrap/bootstrap.go`)

- **Type URL var, descriptor-derived** (the `metricsServiceTypeURL` precedent, `bootstrap.go:223`; `reference_network_filter_typeurl_extensions` — never hand-type):
  `var graphiteStatsdSinkTypeURL = "type.googleapis.com/" + string((&graphitestatsdv3.GraphiteStatsdSink{}).ProtoReflect().Descriptor().FullName())`
  where `graphitestatsdv3` is a REAL (not blank) import of `github.com/envoyproxy/go-control-plane/envoy/extensions/stat_sinks/graphite_statsd/v3` — the parse arm unmarshals into the typed message, which registers the descriptor. Re-derived FullName: `envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink` (the FIRST `envoy.extensions.…` stat-sink type URL; statsd/dog_statsd are inline `envoy.config.metrics.v3.…` types).
- **Dispatch:** `parseStatsSinks`'s switch (`bootstrap.go:505-520`) gains a fourth `case graphiteStatsdSinkTypeURL:` calling `parseGraphiteStatsdSinkConfig(tc, i, result)`.
- **Sibling-reject EXTENDED in lockstep** (`reference_strict_reject_sibling_typeurl_gap`): the default arm's message (`bootstrap.go:518-519`) is rewritten to name FOUR sinks: `bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports the metrics_service sink %q, the statsd sink %q, the dog_statsd sink %q, and the graphite_statsd sink %q)`.
- **`parseGraphiteStatsdSinkConfig`:** unmarshal the `Any`; extract `GetAddress().GetSocketAddress()` (the oneof's SOLE arm — `GraphiteStatsdSink_Address`, re-derived from `graphite_statsd.pb.go:112-118`; `prefix` is field 3 `string`; `max_bytes_per_datagram` is field 4 `*wrapperspb.UInt64Value`); reuse `parseUDPSinkAddressAndPrefix(sa, prefix, "graphite_statsd", "statsd_specifier", i)` (`bootstrap.go:645-654` — nil-`socket_address` reject + `""→"envoy"` prefix default + `net.JoinHostPort`); then the NEW explicit-zero arm: `if w := g.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 { return fmt.Errorf("bootstrap: stats_sinks[%d]: graphite_statsd max_bytes_per_datagram must be greater than 0", i) }` (PARITY with the probed reference reject, §11 A4b).
- **Config struct** (the `DogStatsdSinkConfig` shape, `bootstrap.go:314-318`): `type GraphiteStatsdSinkConfig struct { UDPAddress string; Prefix string; MaxBytesPerDatagram uint64 }` — populated via `.GetValue()` (nil wrapper → 0 → one-line-per-datagram, identical semantics to absent).

### 3.2 A THIRD independent connectionless-UDP seam instance — no new framework piece

`GraphiteStatsdSink` embeds the shared `udpWriter` (`internal/statssink/udp.go:18-25`, `sinkLabel: "graphite_statsd"`) — a THIRD independent `*net.UDPConn` (after statsd's and dog_statsd's), NEVER shared. Construction = the dog_statsd shape verbatim (`dogstatsd.go:67-80`): `net.ResolveUDPAddr("udp", udpAddr)` + `net.DialUDP("udp", nil, raddr)`; a dial error is fatal at `main.go` (§3.5). Idempotent `Close` via the embedded writer. (D-GR-LIFECYCLE confirmed by source re-derivation — no probe needed; envoy-go-side design.)

### 3.3 The `GraphiteStatsdSink` — a `Sink` over the reused delta/tags/batching cores + the ONE novel format function

```go
type GraphiteStatsdSink struct {
	udpWriter
	prefix              string
	delta               *deltaState // a FOURTH sink-private instance — never shared
	maxBytesPerDatagram uint64      // 0 (absent or the parse-rejected explicit 0 never reaches here) = one line per datagram
}

func (s *GraphiteStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	var buf strings.Builder // per-call — batching never spans flushes
	emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			residual, labels = fam.GetName(), nil
		}
		return s.prefix + "." + residual + graphiteTagSuffix(labels), ""
	}, func(line string) { s.appendLine(&buf, line) })
	s.flush(&buf)
}
```

The load-bearing insight (AMEND-GR-TAGFORMAT): `emitStatsdLines` (`udp.go:58-79`) builds `name + ":" + value + "|c"/"|g" + tagSuffix` — graphite's tags precede the `:` so they fold into the NAME return and the trailing `tagSuffix` return stays `""`. `emitStatsdLines`, `deltaState.apply` (`delta.go:39-58` — COUNTER families rebuilt with per-flush deltas keyed by full dotted name, GAUGE passed through absolute, input never mutated — required since `flushOnce` fans ONE batch slice to every sink, `flusher.go:46-51`), and the batching pair `appendLine`/`flush` are REUSED VERBATIM — the batching pair either generalized to a shared helper or duplicated with the same strict-`>` semantics (D-GR-BATCHSHARE, a PLAN choice, §12).

**`graphiteTagSuffix` (the ~10-LoC novel piece):** empty labels → `""`; else for each `stats.Label` in `ExtractTags`'s natural order append `";" + "envoy." + strings.TrimPrefix(l.Key, "envoy_") + "=" + l.Value` (the dog_statsd key-form precedent — keys emit dotted `envoy.cluster_name`, matching the probed reference wire).

### 3.4 The exported graphite-line shape (pinned §11)

```
<prefix>.<residual>;<key1>=<val1>;<key2>=<val2>:<delta>|c    (COUNTER — per-flush delta; zero-delta lines EMITTED)
<prefix>.<residual>;<key1>=<val1>:<abs>|g                    (GAUGE — absolute)
<prefix>.<residual>:<value>|c                                (tag-free — NO ';' anywhere)
```

Datagram packing (cap = `max_bytes_per_datagram`): lines `\n`-SEPARATED (never `\n`-terminated); a line is added iff `buf.Len() + 1 + len(line) > cap` is FALSE (strict `>` — exact-fit co-batches); an empty buffer always accepts (oversized-line-sent-alone, untruncated); cap 0/absent ⇒ one line per datagram. envoy-go emits its WHOLE registry each flush; the reference emits only USED stats (`reference_stats_sink_emits_used_only`) — assert named subsets. envoy-go emits NO `|ms` lines (no histogram surface).

### 3.5 Boot wiring (`cmd/envoy-go/main.go`)

A FOURTH build loop after `Load` (the dog_statsd loop shape, `main.go:246-252`): `for _, cfg := range bs.GraphiteStatsdSinkConfigs { sink, err := statssink.NewGraphiteStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram); if err != nil { log.Fatalf(...) }; statsSinks = append(statsSinks, sink) }`, plus a fourth clause in the `len(...) > 0` gate (`main.go:197`). The flusher wiring (`NewFlusher(bs.Stats, bs.FlushInterval, statsSinks)`, `main.go:253`; default interval 5s, `bootstrap.go:212-214`) is untouched.

### 3.6 Byte-stability

The line format is fully deterministic per side: `strconv.FormatInt` values, `ExtractTags` natural order, fixed separators. No map iteration anywhere in the emit path (labels are a slice). The per-call `strings.Builder` keeps datagram packing deterministic given a frozen registry walk order.

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

`graphite.go`/`graphite_test.go` join the existing `internal/statssink`; the parse arm joins `internal/bootstrap`; the fuzzer joins `internal/bootstrap` (`graphite_fuzz_test.go`). The proto resolves at the ALREADY-PRESENT core `go-control-plane/envoy v1.32.4` module (§1.1 AMEND-GR-IMAGE). `go mod tidy -diff` anticipated EMPTY.

## 5. Proto-field roster (consumed at 57)

| Field | Type (re-derived from `graphite_statsd.pb.go`) | Disposition |
|---|---|---|
| `statsd_specifier.address` (oneof, field 1) | `*corev3.Address` — the oneof's ONLY arm | CONSUMED — UDP endpoint via `parseUDPSinkAddressAndPrefix`; missing ⇒ reject (PARITY §11 A4a) |
| `prefix` (field 3) | `string` | CONSUMED — `""→"envoy"` default (PARITY §11 A1/A2) |
| `max_bytes_per_datagram` (field 4) | `*wrapperspb.UInt64Value` | CONSUMED — batching cap; explicit 0 ⇒ reject (PARITY §11 A4b); nil ⇒ one-line-per-datagram |

ALL fields consumed — no accepted-and-ignored surface. (`address.socket_address.protocol` is not read, the statsd/dog_statsd posture; the reference ignores it too on a UDP sink.)

## 6. PARSE-REJECT roster + fuzzer

- **PARSE-REJECT arms (ADR-0080, each with a DISTINCT message substring):** (i) missing `typed_config` (existing, `bootstrap.go:502-504`); (ii) unknown/sibling type URL — the EXTENDED four-sink default-arm message (§3.1; `reference_strict_reject_sibling_typeurl_gap`); (iii) missing `statsd_specifier`/nil `socket_address` — via `parseUDPSinkAddressAndPrefix` with `("graphite_statsd", "statsd_specifier")` labels (REFERENCE-PARITY, §11 A4a); (iv) **NEW** explicit `max_bytes_per_datagram: 0` — non-nil wrapper with value 0 (REFERENCE-PARITY, §11 A4b; the wrapper type makes absent-vs-explicit-0 distinguishable). NOT reject arms: `protocol` (unread); a hostname `address` (envoy-go's `ResolveUDPAddr` accepts hostnames — the inherited, documented phase-48/49 DEPARTURE from the reference's `malformed IP address` reject, §11 A4c).
- **Fuzzer (D-GR-FUZZER — CONFIRMED):** `FuzzGraphiteStatsdSinkConfigParse` in `internal/bootstrap/graphite_fuzz_test.go` (the `statsd_fuzz_test.go`/`dogstatsd_fuzz_test.go` precedent) — a no-panic fuzz over the typed-config parse arm. Running total reconciled per `reference_fuzzer_count_docs_drift`: actual `^func Fuzz` count is **53** == the documented 53; phase 57 lands **53 → 54**.

## 7. Stat surface — +0 (1201 → 1201)

AMEND-GR-NO-SELF-STATS-CONFIRMED (§11 A1): the reference registers zero graphite self-stats; envoy-go registers none (the phase-48/49/50/55 posture). No new departure flags.

## 8. Differential fixture taxonomy (+1: `0101` cross-side delta-SUM + tag-set + batching signals, MERGED into one dir)

### 8.1 `0101-stats-sink-graphite` (cross-side; D-GR-FIXTURE DECIDED: ONE dir, batching folded in)

Batching ships in the SAME row, so `0101` merges the `0093` (tags) and `0094` (batching) shapes into ONE fixture — one dir, one runner branch (cross-side; `reference_differential_fixture_dispatch_constraint`), fixtures **102 → 103** (NOT → 104). Topology (the `0093`/`0094` template): H1 downstream `l_test` → `HTTPFixedBody` backend (`BackendCount()==1`, kind UNCHANGED); TWO per-side driver-owned `statsdrecv` UDP receivers bound `0.0.0.0` and live before either proxy boots, hard `Close()` at teardown (`reference_periodic_sink_differential_two_receivers`); the reference reaches its receiver at the inlined-`hostGatewayIP` literal (the driver-local copy — the `differential` package is import-cycle-unreachable from drivers, `0092 driver.go:508-589`), the subject at `127.0.0.1`; `stats_flush_interval: 0.5s`; prefix baked identically both sides; **`max_bytes_per_datagram: 200` on BOTH sides + a deliberately long (~172-char) backend cluster name** (the `0094` oversized-proof constants); K = 7 requests per side.

- **Assertions (cross-side):** for a named subset of graphite WIRE names — `<pfx>.cluster.upstream_rq_total` (tags `{envoy.cluster_name: <backend>}`), `<pfx>.http.downstream_rq_total` (tags `{envoy.http_conn_manager_prefix: <sp>}`), `<pfx>.http.downstream_rq_xx` (tags `{envoy.response_code_class: 2, envoy.http_conn_manager_prefix: <sp>}`) — `DeltaSumTagged(name, wantTags) == K` per side after the stability barrier (poll until SUM==K, then `awaitFurtherFlushes ≥ 2`, then assert STILL K — `reference_delta_sink_differential_stability_barrier`); tag-SET equality via `Tags()` + `maps.Equal` (NEVER order — AMEND-GR-TAGORDER; the tag-matched lookup also disambiguates the admin-HCM wire-name collision, `reference_admin_interface_wire_name_collision`); gauge subset `<pfx>.cluster.membership_total == 1` (no-HC cluster — `reference_membership_total_vs_healthy_gauge`); **batching proofs:** `MaxLinesInAnyDatagram() > 1` AND the oversized cluster-tagged line always alone (`LinesInDatagram(name) == 1`); subject-side `UnparsedCount() == 0` (`reference_framing_break_needs_unparsed_counter`). The payload is aggregated ACROSS datagrams/flushes — framing never asserted cross-side (`reference_streaming_sink_differential_framing`).
- **UNasserted (coverage boundaries):** per-side tag ORDER (reversed cross-side, §1.1); the reference's `|ms` timer lines (envoy-go has no histograms; reference-side receiver `unparsed` is NOT asserted); exact-code `_NNN` names (AMEND-GR-EXACTCODE-SUBSET — extraction rules differ cross-side); whole-registry vs used-only emission breadth (`reference_stats_sink_emits_used_only`); datagram COUNT/framing.
- **Deliberate breaks (`reference_differential_break_protocol_count1`, each firing on the SUBJECT side, `-count=1`, each isolating ONE assertion per `reference_deliberate_break_wrong_assertion`):** (a) delta-break — absolute counters (skip `delta.apply`) ⇒ the post-barrier SUM overshoots K; (b) tag-format break — emit dog_statsd's `|#k:v` suffix instead of `;k=v` ⇒ the tagged lookup misses AND subject `UnparsedCount`/name-miss fires (PLAN engineers WHICH fires and adds an isolating break if masked); (c) batching break — ignore the cap (never mid-flush flush) ⇒ the oversized-alone assertion fails via co-batching, or cap-exceeding datagrams appear; (d) tag-order-robustness NON-break sanity: reversing envoy-go's tag order must NOT fail the suite (proving set-assertion liveness the other way). Plus the 20/20 flake gate + full-package `-race` at stage-close (`reference_full_suite_race_after_background_mutator`).
- **Receiver extension (`test/helpers/statsdrecv`, ADDITIVE):** `ingestLine` currently splits first-`|`-then-last-`:` — a graphite line parses with the `;k=v` pairs STUCK IN THE NAME. Extend: after the name split, `if i := strings.IndexByte(name, ';'); i >= 0` → parse `;key=value` pairs into the SAME tag/`byTag` machinery the `|#` path feeds (`Tags()`/`DeltaSumTagged` work unchanged); a pair missing `=` counts unparsed. Additive — statsd/dog_statsd lines have no `;` in names (`stats.IsValidName` excludes it), so `0092`/`0093`/`0094` are unaffected. The extension gets its own liveness proof (a unit test + break (b) above), per `reference_differential_asserter_dispatch`'s prove-the-new-assertion-live discipline.

### 8.2 New BackendKind: NONE

`HTTPFixedBody` reused; the UDP receiver stays a driver-owned `test/helpers` server (`reference_differential_grpc_receiver_driver_owned`, generalized). BackendKind tail stays **38**.

## 9. Behavior-contract delta (the phase-57 bundle; ADR-0275 atomic landing)

BEHAVIOR_CONTRACT gains the graphite_statsd sink subsection at the IMPL: the line mapping (§3.4), the delta/gauge semantics, the tag grammar + natural-order + key-form rules, the batching rules (strict-`>`, `\n`-separated, oversized-alone, 0/absent = one-per-datagram), the reject roster (§6), and the two documented DEPARTURES (hostname-accepting `address`; whole-registry emission) — all landing atomically with ADR-0275.

## 10. Per-task structure (~9–12 tasks; PLAN decomposes)

Anticipated spine: T1 baselines · T2 parse arm + config struct + sibling-reject extension + rejects (TDD) · T3 `graphiteTagSuffix` + line mapping unit tests · T4 `GraphiteStatsdSink` Submit (delta + batching reuse) unit tests · T5 `main.go` wiring · T6 fuzzer · T7 `statsdrecv` graphite-tag extension + unit tests · T8 `0101` fixture + driver · T9 deliberate breaks + flake gate · T10 ADR-0052 docs bundle. Margin ≥ 3 under the `~15` ADR-0045 ceiling (§3.0).

## 11. SPEC-time empirical-pin block (D-GR-* — executed IN-SESSION 2026-07-11)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2`, a FRESH container per arm (`reference_probe_fresh_container_per_arm`), reaching a host-bound (0.0.0.0) datagram-boundary-preserving Python UDP receiver at the host-gateway literal `192.168.65.2` (`reference_host_gateway_ip_docker_desktop`; resolved via a throwaway `alpine:3` `getent` container — the `HostGatewayIP` method); backend = a host `python3 -m http.server` at `192.168.65.2:8111`; H1 listener `:10000` `stat_prefix=ingresstag` → STATIC cluster `c1tag`; `stats_flush_interval: 1s`; K=5 requests (3 then 2, with quiet flushes between and after). Eight arms: A1 base/default · A2 `prefix: probepfx57` · A3 `max_bytes_per_datagram: 160` · A3b cap=120 + 130-char cluster name · A4a no-`address` · A4b `max_bytes_per_datagram: 0` · A4c hostname `address` · A5 the STANDARD (non-contrib) `v1.37.2` image. Decode-ran proof: A1 alone produced **1373 datagrams = 536 `|c` + 779 `|g` + 58 `|ms` lines**.

| Pin | Result |
|---|---|
| **D-GR-TAGFORMAT** (LOAD-BEARING) | PINNED (AMEND-GR-TAGFORMAT-CONFIRMED). Two-tag: `envoy.cluster.external.upstream_rq_xx;envoy.response_code_class=2;envoy.cluster_name=c1tag:3\|c`; one-tag gauge: `envoy.cluster.upstream_rq_active;envoy.cluster_name=c1tag:0\|g`; tag-free: `envoy.listener.admin.downstream_cx_total:1\|c` (NO `;`). Grammar: `;` separates name→first-tag AND tag→tag; `=` key/value; keys dotted `envoy.<tag>`. ⇒ tags fold into `emitStatsdLines`'s NAME argument; trailing suffix `""` (§3.3). |
| **D-GR-DELTA** (LOAD-BEARING; probed independently of 48/49) | PINNED (AMEND-GR-DELTA-CONFIRMED). `…upstream_rq_total;envoy.cluster_name=c1tag` emitted `3\|c, 0\|c, 2\|c, 0\|c…` — SUM 5 == K == admin `cluster.c1tag.upstream_rq_total: 5`; zero-delta counters RE-EMITTED every flush; gauges ABSOLUTE (`server.memory_allocated` monotone snapshots). ⇒ fourth sink-private `deltaState`; delta-SUM + stability-barrier differential. |
| **D-GR-PREFIX** | PINNED (AMEND-GR-PREFIX-CONFIRMED). No-`prefix` ⇒ every line `envoy.`-prefixed; `prefix: probepfx57` ⇒ 604/604 lines `probepfx57.`-prefixed. ⇒ reuse the `""→"envoy"` default in `parseUDPSinkAddressAndPrefix`. |
| **D-GR-TAGORDER** (LOAD-BEARING) | PINNED (AMEND-GR-TAGORDER-CONFIRMED). Natural extractor order, NOT alphabetical: `;envoy.response_code_class=2;envoy.cluster_name=c1tag` (r-c-c FIRST despite `cluster.` leading the dotted name) — the REVERSE of envoy-go `ExtractTags`'s SN1-first append order. ⇒ per-side natural order unsorted; cross-side tag assertions are SET-based only. |
| **D-GR-BATCH** (LOAD-BEARING) | PINNED (AMEND-GR-BATCH-CONFIRMED; matches phase-50 D-DSDB-BOUNDARY/OVERSIZED/DEFAULT on the shared C++ path). cap=160: 216 multi-line datagrams, `\n`-joined, **12 at EXACTLY len=160, zero above** ⇒ strict `>`; **0 datagrams `\n`-terminated**. cap=120 + long name: **147 over-cap datagrams (max 223B), 100% single-line**, 75 under-cap multi-line ⇒ oversized-sent-alone, untruncated. absent: 1373/1373 single-line ⇒ one-line-per-datagram. |
| **D-GR-STATS** | PINNED (AMEND-GR-NO-SELF-STATS-CONFIRMED). Flushing reference: `admin /stats \| grep -iE 'statsd\|graphite'` EMPTY. ⇒ stat surface 1201 (+0). |
| **D-GR-REJECT** | PINNED (AMEND-GR-REJECT-CONFIRMED). A4a no-`address`: `Proto constraint validation failed (field: "statsd_specifier", reason: is required)`, boot abort. A4b explicit `0`: `Proto constraint validation failed (GraphiteStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)` — dog_statsd carries the IDENTICAL PGV rule that envoy-go's landed parse does NOT enforce (§2 deferred). A4c hostname: `malformed IP address: host.docker.internal`. ⇒ §6 roster: mirror (i)+(ii); (iii) stays the inherited hostname-accepting DEPARTURE. |
| **D-GR-IMAGE** | PINNED (AMEND-GR-IMAGE-CONFIRMED). Contrib image boots + emits (A1); STANDARD `v1.37.2` image ALSO emits (281 datagrams) — a CORE extension; proto re-derived in the core `envoy@v1.32.4` module. ⇒ zero new go.mod deps; the differential keeps the contrib reference image. |
| **D-GR-FIXTURE** | DECIDED (design, informed by A3/A3b): ONE dir `0101-stats-sink-graphite` with `max_bytes_per_datagram: 200` + the long-cluster-name oversized proof folded in (the merged `0093`+`0094` shape) — fixtures 102 → **103**. |
| **D-GR-FUZZER** | CONFIRMED. Actual `^func Fuzz` == documented == **53** (reconciled per `reference_fuzzer_count_docs_drift`); `FuzzGraphiteStatsdSinkConfigParse` lands 53 → **54**. |
| **D-GR-LIFECYCLE** | CONFIRMED from source (no probe — envoy-go-side design): `ResolveUDPAddr`+`DialUDP` at construction (`dogstatsd.go:67-80` shape), `log.Fatalf` on dial error at `main.go`, idempotent `Close` via the shared `udpWriter`. A THIRD independent UDP conn. |

Every BRAINSTORM `file:line` was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`); one stale claim caught: the `bootstrap.go:417-419` struct doc comment still says an explicit dog_statsd `max_bytes_per_datagram` "is rejected at parse time" — outdated phase-49 wording; the phase-50 code consumes it (`bootstrap.go:678`). The IMPL's docs task corrects that comment in passing.

## 12. PLAN / IMPL D-questions (not empirical pins)

- **D-GR-BATCHSHARE** — reuse `appendLine`/`flush` by GENERALIZING to a shared helper (both sinks call it) vs duplicating the ~20 LoC in `graphite.go` with the same strict-`>` semantics. PLAN decides (bias: generalize — one tested implementation; but only if the dog_statsd tests stay untouched).
- **D-GR-SPLIT** — re-check the ADR-0045 gate with real task LoC at PLAN (anticipated ~9–12 tasks, §10).
- **Break-(b) firing precision** — engineer WHICH assertion the tag-format break fires (tagged-lookup miss vs subject `UnparsedCount`) and add an isolating break if the first masks the second (`reference_deliberate_break_wrong_assertion`).

## 13. ADR continuity — the ADR-0275 §Context DRAFT (anchored here; full entry lands at the phase-57 IMPL)

**ADR-0275 §Context (draft):** Phases 47–55 landed the whole periodic stats-sink substrate: the `Flusher`/`Sink` seam walking the frozen `stats.Registry` (ADR-0257 family), the always-on per-flush counter-delta `deltaState` (ADR-0265 — statsd `|c` IS a delta by protocol definition, no knob), the connectionless-UDP `udpWriter`, the `stats.ExtractTags` residual+labels core consumed by dog_statsd's tag suffix (ADR-0266), the `max_bytes_per_datagram` `appendLine`/`flush` datagram-packing loop (ADR-0267 — strict-`>` prospective overflow, `\n`-separated never terminated, oversized-sent-alone, 0 ⇒ one-line-per-datagram), and the TCP statsd transport (ADR-0272). Phase 57 opens the TENTH Observability-family row: the `graphite_statsd` sink — the FOURTH `stats_sinks[]` consumer and the FIRST `envoy.extensions.…` (typed-extension) stat-sink type URL, descriptor-derived and dispatched alongside the three inline `envoy.config.metrics.v3` siblings, with the default-arm reject message extended to name all four. Live probes against `envoyproxy/envoy:contrib-v1.37.2` (SPEC-57 §11, eight fresh-container arms) pinned: the graphite tag grammar (`<prefix>.<residual>;k=v;k2=v2:<value>|<type>` — tags in the NAME, `=`-delimited, `;`-separated including the first, tag-free lines `';'`-free), per-flush `|c` deltas with zero-delta re-emission and absolute `|g` gauges, default prefix `envoy`, NATURAL (unsorted) tag order whose two-tag wire order is REVERSED vs envoy-go's `ExtractTags` append order (cross-side tag assertions therefore SET-based), batching semantics byte-identical to phase-50's dog_statsd pins (exact-fit co-batches at the cap, oversized single lines sent alone untruncated), zero sink self-stats, and three boot-rejects (missing `statsd_specifier`; explicit `max_bytes_per_datagram: 0` per the PGV `gt: 0` rule — which dog_statsd's proto ALSO carries but envoy-go's landed phase-50 parse does not enforce, recorded as a deferred parity candidate; hostname addresses — where envoy-go keeps its documented hostname-accepting departure). The sink reuses `deltaState`, `udpWriter`, `emitStatsdLines`, `ExtractTags`, and the phase-50 batching machinery verbatim; the sole novel code is the ~10-LoC `graphiteTagSuffix` folding tags into the name argument. The `0101-stats-sink-graphite` differential merges the `0093` tag-set/delta-SUM-stability-barrier shape with the `0094` batching proofs in ONE dir (batching ships day-1 in the same row), over an additively-extended `statsdrecv` that parses name-embedded `;k=v` tags into the existing tag machinery. A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); §Decision/§Consequences land at the phase-57 IMPL. ANCHORS ADR-0275.

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat surface **1201** · fixtures **102** · fuzzers **53** · BackendKind **38** · DECISIONS tail **ADR-0274** (next-free **ADR-0275**). Anticipated at the phase-57 IMPL: stat surface **1201 (+0)** · fixtures **102 → 103** (`0101-stats-sink-graphite`) · fuzzers **53 → 54** (`FuzzGraphiteStatsdSinkConfigParse`) · BackendKind **38 (+0)** · DECISIONS tail **ADR-0275** (next-free **ADR-0276**) · **+0 go.mod modules**, **+0 packages**. Row 57 stays `in-progress`; it flips `done` at the phase-57 IMPL six-gate (the SOLE leg — no parent rollup, ADR-0106). The Observability family STAYS OPEN (remaining deferred candidates: OTLP-metrics sink + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace; plus the NEW dog_statsd explicit-zero parity candidate, §2). Next → the phase-57 PLAN (`superpowers:writing-plans`), then the phase-57 IMPL (subagent-driven per `feedback_execution_style`).
