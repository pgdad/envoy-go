# Phase 50 SPEC — `dog_statsd max_bytes_per_datagram` real multi-metric datagram batching: lift the `bootstrap.go:591` strict-reject into an HONORED `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` + a buffer-accumulate-then-flush-on-overflow rewrite of `DogStatsdSink.Submit` (delta/tag/line-formatting UNCHANGED) + a MINIMAL additive `test/helpers/statsdrecv` observability extension (the datagram-level `\n`-split ALREADY WORKS, no parser change needed) + the `0094-stats-sink-dogstatsd-batching` cross-side differential — a SINGLE FLAT ROW, transport-layer-only; the SEVENTH Observability-family row; ZERO new packages, ZERO new go.mod modules (ANCHORS ADR-0267)

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-50 BRAINSTORM (`docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/BRAINSTORM.md`, landing commit `5ac18f58`). This SPEC charters phase **50** as a single flat row (BRAINSTORM Q4; the ADR-0045 50.1/50.2 escape-valve stays UNCONSUMED): a transport-layer-only batching change over the LANDED phase-49 `DogStatsdSink` (`internal/statssink/dogstatsd.go`). Lift the `bootstrap.go:591` `max_bytes_per_datagram`-set strict-reject into a genuine accept-and-honor `parseDogStatsdSinkConfig` path → a NEW `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` field; rewrite `DogStatsdSink.Submit`'s per-line `s.write(line)` call into a buffer-accumulate-then-flush-on-overflow loop over the ALREADY-formatted lines (delta/tag/line-formatting computation is completely UNCHANGED — batching is purely a transport-layer concern applied strictly AFTER line formatting); extend `cmd/envoy-go/main.go`'s dog_statsd build loop to thread the new field into `NewDogStatsdSink`; extend `test/helpers/statsdrecv` with two small additive last-seen-by-name accessors (NOT a parser rewrite — §1.1 AMEND-DSDB-RECEIVER, the SPEC's key correction to the BRAINSTORM's §2.8 assumption); prove it with the `0094-stats-sink-dogstatsd-batching` cross-side differential. Counts at SPEC commit UNCHANGED (stat surface **1200** [H2 cluster; non-H2 **1196**] / fixtures **95** / fuzzers **52** / BackendKind **38** / DECISIONS tail **ADR-0266**, next-free **ADR-0267**). The §10 D-DSDB-* empirical pins were EXECUTED IN-SESSION (2026-07-05) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Deliver real multi-metric UDP datagram packing for the `dog_statsd` sink: when a bootstrap `dog_statsd` `stats_sinks[]` entry carries an EXPLICIT `max_bytes_per_datagram`, envoy-go now HONORS it — accumulating consecutive formatted DogStatsd lines (in the SAME registry-walk emission order the phase-49 `Submit` loop already produces) into a growing buffer and flushing the buffer as ONE UDP datagram whenever appending the next line would exceed the cap, instead of unconditionally writing one datagram per line. When the field is ABSENT (or explicitly `0`), the SAME general algorithm degrades to exactly phase 49's existing one-line-per-datagram behavior with NO conditional fork (§3.3) — reference-parity for the common/default case is preserved without a special case. This is a TRANSPORT-layer-only change: the delta-state transform, the `stats.ExtractTags` tag extraction, and the DogStatsd line-formatting (`<prefix>.<residual>:<value>|<type>[|#tags]`) are byte-for-byte UNCHANGED from the landed phase-49 code — only the final "how many lines share one `Write` call" decision changes. ZERO new Go packages, ZERO new go.mod modules (pure `strings`/`net` buffer manipulation, no new dependency). Byte-identical and stat-surface-identical when no `dog_statsd` sink is configured OR when one is configured with no `max_bytes_per_datagram` (every non-batching path untouched — the full differential is the regression anchor). It ANCHORS ADR-0267; row 50 flips `done` at the phase-50 IMPL six-gate (NO parent rollup — ADR-0106); the Observability family STAYS OPEN.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §10 live probe (2026-07-05, contrib-v1.37.2 — an H1 downstream :10000 `stat_prefix=ingress_http` → an `hashicorp/http-echo` backend, a bootstrap `dog_statsd` `stats_sinks[]` entry with a driver-owned Python UDP receiver at the Docker-Desktop host-gateway literal IP `192.168.65.2` [`reference_host_gateway_ip_docker_desktop`, re-confirmed this session], `stats_flush_interval=1s`, on a dedicated Docker bridge `dsdb-net` per `reference_docker_probe_bridge_network`; four config variants: no-cap/default, `max_bytes_per_datagram: 75` [an EXACT boundary probe], `max_bytes_per_datagram: 74` [one byte under], `max_bytes_per_datagram: 120` with a deliberately long cluster name [an oversized-single-line probe]) CONFIRMED the BRAINSTORM's core hypotheses and RESOLVED the LOAD-BEARING D-DSDB-BOUNDARY question. The findings that RESHAPE the design vs the BRAINSTORM's proposed/self-answered defaults:

- **AMEND-DSDB-DEFAULT-CONFIRMED (an absent `max_bytes_per_datagram` emits EXACTLY one line per datagram — LIVE-confirmed, not merely doc-confirmed).** Live finding (D-DSDB-DEFAULT): a no-cap probe run captured **1942 datagrams / 1942 with `nlines=1`** (100%) — zero multi-line datagrams. This is IDENTICAL to the phase-49 SPEC's own D-DSD-LINE finding; the field's absence is reference-parity with the ALREADY-landed phase-49 behavior, confirmed fresh rather than assumed from the proto doc-comment alone (`reference_wire_format_both_sides_see_same_bytes`).
- **AMEND-DSDB-BOUNDARY-CONFIRMED (the FIRST LOAD-BEARING finding — the overflow comparison is INCLUSIVE: a buffer that lands EXACTLY at the cap after appending the next line still "fits"; only a STRICT excess (`> cap`) triggers a flush-before-append).** Live finding (D-DSDB-BOUNDARY): two adjacent, byte-stable lines were identified from a live capture — `probepfx.dns.cares.not_found:<0|1>|c` (32 bytes) immediately followed in registry-walk order by `probepfx.server.hot_restart_generation:1|g` / `probepfx.cluster_manager.cluster_added:<0|1>|c` (42 bytes) — whose combined length with a `\n` separator is EXACTLY 32+1+42 = **75 bytes**. At `max_bytes_per_datagram: 75`, these two lines were observed **co-located in ONE 75-byte datagram** (`nlines=2`) on every occurrence across a 10-second capture. At `max_bytes_per_datagram: 74` (one byte under), the SAME two lines were observed **split into two separate datagrams** (`len=32,nlines=1` then `len=42,nlines=1`) on every occurrence. This is airtight: a combined size EQUAL to the cap fits; one byte OVER the cap does not. ⇒ the production comparison MUST be `prospective > cap` (flush-before-append), NOT `prospective >= cap` — the BRAINSTORM's §2.3 phrasing ("If the buffer is non-empty AND appending would exceed `MaxBytesPerDatagram`") is confirmed to mean STRICT excess, and this SPEC pins the exact operator that a naive reading could get backwards.
- **AMEND-DSDB-OVERSIZED-CONFIRMED (a single line whose formatted length ALONE exceeds the cap is sent as its own oversized datagram — no error, no drop, no truncation, no splitting of the line's own bytes across two datagrams).** Live finding (D-DSDB-OVERSIZED): with `max_bytes_per_datagram: 120` and a backend cluster name engineered to be ~130 characters (forcing the `envoy.cluster_name`-tagged `cluster.*` lines for that cluster to exceed 120 bytes on their own), **715 datagrams exceeding the 120-byte cap were observed** (up to 218 bytes), **100% of them `nlines=1`** (never co-batched with anything else), and **zero truncation** — every captured oversized line's reported string length exactly matched its expected full content (verified programmatically: no oversized line was ever missing bytes or cut short). This directly confirms Q2/§2.5 (mirror the reference: send alone, uncapped for that one line) and rules out any silent data loss.
- **AMEND-DSDB-JOIN-ORDER-CONFIRMED (multi-line datagrams preserve pure sequential accumulation order — no reordering, no sorting, no deduplication).** Live finding (D-DSDB-JOIN-ORDER): across every multi-line datagram captured in the cap=120 and cap=75 runs, (a) **zero datagrams contained a repeated metric name** (no dedup violation), and (b) for every pair of names ever observed adjacent within a datagram, **the relative order was 100% consistent** across every occurrence (zero instances of the same pair appearing in both orders) — proving the registry-walk emission order is stable and the packing loop never reorders it.
- **AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED (the SPEC's key correction to the BRAINSTORM's §2.8/§6.1 "flag a delimiter-collision risk, trace an example before editing the parser" — re-reading the ACTUAL landed phase-49 `test/helpers/statsdrecv/statsdrecv.go` `ingest` function shows it ALREADY splits a received datagram's payload on `\n` BEFORE per-line parsing, at `statsdrecv.go:131`: `for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n")`).** This is NOT a live-probe finding — it is a code-reading finding, per this project's standing discipline to re-read landed code fresh rather than trust a prior phase's doc citations (§Read-first invariant). Tracing one concrete multi-line datagram byte-for-byte through the CURRENT function (`reference_line_parser_extension_delimiter_reuse`): payload `"probepfx.dns.cares.not_found:0|c\nprobepfx.server.hot_restart_generation:1|g"` → `strings.TrimSpace` (no-op, no leading/trailing whitespace) → `strings.Split(..., "\n")` → `["probepfx.dns.cares.not_found:0|c", "probepfx.server.hot_restart_generation:1|g"]` → each element then flows through the EXISTING first-`|`-then-colon per-line parse UNCHANGED, exactly as if it had arrived alone in its own datagram. **No collision, no parser change of any kind is required to correctly ingest a multi-line datagram.** ⇒ phase 50 needs ZERO changes to `ingest`'s parsing/delimiter logic — the BRAINSTORM's anticipated "receiver-parser risk" (§2.8) does not exist in the landed code (it may have been added defensively at phase 49, or inherited from a shared idiom — regardless, it already does the right thing). The ONLY `statsdrecv` change phase 50 needs is a small ADDITIVE pair of observability accessors (§8.2) to let the differential PROVE batching occurred and PROVE the oversized line stayed alone — not to make ingestion correct (it already is).
- **D-DSDB-FUZZER (resolved as a code-reading/design question, per the BRAINSTORM's own framing — not a live probe).** Re-reading `internal/bootstrap/dogstatsd_fuzz_test.go` (landed at phase 49): its seed corpus ALREADY includes a `max_bytes_per_datagram: 512` entry (line 46-53) that exercises this exact field through the full `bootstrap.Load` parse path; once the strict-reject is lifted, this SAME seed continues to exercise the field's ACCEPT path under fuzzing mutation, at the SAME untrusted-config-boundary the fuzzer already targets. The packing algorithm itself (§3.3) consumes ALREADY-VALIDATED, ALREADY-formatted `[]string` lines and a parsed `uint64` cap — not raw untrusted bytes — so it does not fit this project's "one fuzzer per new accepted-config parse path" convention (the convention targets the wire/config parse boundary, not arbitrary in-process pure-Go logic operating on trusted inputs). ⇒ **NO new fuzzer.** Fuzzers stay **52** (§6).

Additional confirmed pins (no amendment; carried from phase 49, re-verified fresh): the `DogStatsdSink` proto resolves at the pinned `go-control-plane/envoy v1.32.4` `config/metrics/v3` package, already imported in `bootstrap.go` as `metricsconfigv3` (§11 of SPEC-49, re-verified §5 below); `max_bytes_per_datagram`(4) is a `*wrapperspb.UInt64Value` whose doc-comment ("By default Envoy will emit one metric per datagram... this value may not be respected if smaller than a single metric") is now LIVE-CONFIRMED rather than merely read (AMEND-DSDB-DEFAULT-CONFIRMED + AMEND-DSDB-OVERSIZED-CONFIRMED); the phase-48/49 synchronous-write UDP lifecycle shape carries unchanged (no new writer-goroutine, no channel — the buffering happens entirely within one synchronous `Submit` call).

### 1.2 ADR continuity + D-disposition at SPEC commit

- **ADR-0267** (next-free) — the dog_statsd `max_bytes_per_datagram` batching transport change; §Context drafted here (§12), §Decision/§Consequences land at the phase-50 IMPL (ADR-0044). The SEVENTH Observability-family row's sole leg; NO seam ADR (the `Flusher`/`Sink`/`deltaState`/`stats.ExtractTags` seams are all reused unchanged — this ADR is scoped to the packing algorithm alone, per BRAINSTORM §7).
- D-DSDB-DEFAULT / D-DSDB-BOUNDARY / D-DSDB-OVERSIZED / D-DSDB-JOIN-ORDER / D-DSDB-FUZZER: PINNED at this SPEC (§10/§1.1). The PLAN/IMPL D-questions (§11): resolved at PLAN/IMPL.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

- **Any change to the line-formatting / tag-extraction / delta-state logic.** Batching is purely a TRANSPORT-layer concern applied strictly AFTER line formatting (§1.7 of the BRAINSTORM, self-answered YES, orthogonal — confirmed by this SPEC's design in §3.3, which never touches the `deltaState.apply`/`stats.ExtractTags`/line-formatting code paths).
- **Any change to `StatsdSink` (plain statsd)** — `StatsdSink` has no `max_bytes_per_datagram`-equivalent field (confirmed absent from `stats.pb.go`'s `StatsdSink` message at phase 48; not re-litigated here).
- **`graphite` / `open_telemetry`-metrics sinks** — each its own future deferred Observability row.
- **The plain-statsd `tcp_cluster_name` transport** — a DIFFERENT sink, already deferred at phase 48.
- **Timers / `|ms` lines** — envoy-go has ZERO histograms (ADR-0060), unchanged from every prior Observability row.
- **The tap filter + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace** — unrelated Observability-family candidates, unchanged from the phase-49 deferred list.
- **A dedicated packing-algorithm fuzzer** — D-DSDB-FUZZER resolves NO new fuzzer is warranted (§1.1); the existing `FuzzDogStatsdSinkConfigParse` already covers the untrusted-config boundary this field crosses.

---

## 3. The packing change — `bootstrap.go` relaxation + `DogStatsdSinkConfig.MaxBytesPerDatagram` + `DogStatsdSink.Submit` rewrite (ADR-0267)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED (re-checked at PLAN with real LoC)

Phase 50 is chartered here as a single flat row (BRAINSTORM Q4). The flush substrate, the delta-state pattern, the tag-extraction core, AND the DogStatsd line-formatting are ALL landed and UNTOUCHED; phase 50 adds only: one config field, one bootstrap-parse relaxation (a strict-reject DELETION, not a narrowing), a buffer-accumulate rewrite of one existing method, two small additive `statsdrecv` accessors, and one new differential fixture (anticipated ~80–150 prod LoC — SMALLER than phase 49's ~250–350, per the BRAINSTORM §1.4 estimate, since NO new `Sink`, NO new tag/delta logic, and NO parser rewrite are needed — well under the ADR-0045 gate). A 50.1/50.2 sub-split is NOT anticipated; the FINAL gate is re-checked at the PLAN with real LoC. **Row 50 flips `done` at the phase-50 IMPL six-gate** (no parent rollup — ADR-0106); the Observability family STAYS OPEN.

### 3.1 Config parse — lift the `bootstrap.go:591` strict-reject; add `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (`internal/bootstrap/bootstrap.go`)

The CURRENT `parseDogStatsdSinkConfig` (`bootstrap.go:586-607`, re-read fresh this session):

```go
func parseDogStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var dsd metricsconfigv3.DogStatsdSink
	if err := tc.UnmarshalTo(&dsd); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd typed_config: %w", idx, err)
	}
	if dsd.GetMaxBytesPerDatagram() != nil {                                    // bootstrap.go:591-593 — DELETED ENTIRELY
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram is not supported (envoy-go emits one metric per datagram)", idx)
	}
	sa := dsd.GetAddress().GetSocketAddress()
	if sa == nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd requires address.socket_address (dog_statsd_specifier is required)", idx)
	}
	prefix := dsd.GetPrefix()
	if prefix == "" {
		prefix = "envoy"
	}
	result.DogStatsdSinkConfigs = append(result.DogStatsdSinkConfigs, DogStatsdSinkConfig{
		UDPAddress: fmt.Sprintf("%s:%d", sa.GetAddress(), sa.GetPortValue()),
		Prefix:     prefix,
	})
	return nil
}
```

Phase 50:

- **Deletes the `bootstrap.go:591-593` strict-reject block ENTIRELY** (per Q3/§2.6 of the BRAINSTORM — "no new condition under which this field warrants a reject"; not narrowed, not replaced by any other check).
- **Adds `MaxBytesPerDatagram: dsd.GetMaxBytesPerDatagram().GetValue()`** to the `DogStatsdSinkConfig{...}` literal at `bootstrap.go:602-605`. `GetValue()` on a nil `*wrapperspb.UInt64Value` already returns `0` (protobuf-generated getter nil-safety) — so an ABSENT field and an EXPLICIT `0` both parse to `MaxBytesPerDatagram: 0` with NO extra nil-check needed (§2.2 of the BRAINSTORM's self-answered design, confirmed correct — a plain `uint64`, no pointer, since the BRAINSTORM already established `0`-and-absent are behaviorally identical under the packing algorithm, §3.3 below).
- **No other validation is added.** All other phase-49 strict-rejects (missing `dog_statsd_specifier`/nil `socket_address`, unsupported sibling TypeURLs at the `parseStatsSinks` dispatch) are UNCHANGED.

The `DogStatsdSinkConfig` struct (`bootstrap.go:301-304`) gains one field:

```go
type DogStatsdSinkConfig struct {
	UDPAddress          string // socket_address host:port (an IP literal:port; net.ResolveUDPAddr-resolvable)
	Prefix              string // DogStatsdSink.prefix, default "envoy" when empty
	MaxBytesPerDatagram uint64 // NEW (ADR-0267): 0 (absent or explicit) means "one metric per datagram" (phase-49 behavior, UNCHANGED); >0 batches consecutive lines up to the cap
}
```

### 3.2 Boot wiring (`cmd/envoy-go/main.go:222`)

The THIRD build loop's `NewDogStatsdSink` call threads the new field:

```go
sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram)  // was: (cfg.UDPAddress, cfg.Prefix)
```

No other change to `main.go`: the flusher-build gate (`main.go:194`, the three-way OR over `StatsSinkConfigs`/`StatsdSinkConfigs`/`DogStatsdSinkConfigs`), the `Flusher` build, and the LIFO-drain `Close()` are all sink-agnostic and UNCHANGED.

### 3.3 The packing algorithm — `DogStatsdSink.Submit` (`internal/statssink/dogstatsd.go:72-103`, re-read fresh this session)

The CURRENT `Submit` calls `s.write(line)` (a direct one-`Write`-per-line call, `dogstatsd.go:100`) inside the per-metric loop. Phase 50 replaces this ONE call site with a buffer-accumulate-then-flush-on-overflow stage; the line-BUILDING code above it (`residual, labels, err := stats.ExtractTags(...)`, `formatTagSuffix`, the `line := name + ":" + ... + tagSuffix` construction) is COMPLETELY UNCHANGED:

```go
// DogStatsdSink gains ONE new field, set at construction:
type DogStatsdSink struct {
	conn                *net.UDPConn
	prefix              string
	delta               *deltaState
	maxBytesPerDatagram uint64 // NEW (ADR-0267): 0 means "one metric per datagram" (phase-49 behavior)

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// NewDogStatsdSink gains one new parameter (main.go:222 threads it):
func NewDogStatsdSink(udpAddr string, prefix string, maxBytesPerDatagram uint64) (*DogStatsdSink, error) {
	// ... unchanged resolve/dial ...
	return &DogStatsdSink{conn: conn, prefix: prefix, delta: newDeltaState(), maxBytesPerDatagram: maxBytesPerDatagram}, nil
}

func (s *DogStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	var buf strings.Builder // a PER-CALL buffer — batching never spans across flushes
	for _, fam := range batch {
		// ... UNCHANGED: suffix/residual/labels/name/tagSuffix computation ...
		for _, m := range fam.GetMetric() {
			// ... UNCHANGED: v computation ...
			line := name + ":" + strconv.FormatInt(int64(v), 10) + suffix + tagSuffix
			s.appendLine(&buf, line) // REPLACES the phase-49 s.write(line) call site
		}
	}
	s.flush(&buf) // flush any remaining partial buffer at the end of the batch
}

// appendLine accumulates line into buf, flushing buf as its own datagram FIRST
// if appending would make it exceed maxBytesPerDatagram (D-DSDB-BOUNDARY: the
// comparison is STRICT — a buffer that lands EXACTLY at the cap after
// appending still fits; only landing over the cap triggers a flush-before-
// append, live-confirmed §1.1). When buf is EMPTY, the line is ALWAYS
// accepted unconditionally (even if the line alone exceeds the cap) — this is
// what makes an oversized single line "sent alone" fall out of the SAME
// general algorithm with NO special-cased branch (D-DSDB-OVERSIZED, §2.3/§2.5
// of the BRAINSTORM): on the NEXT call, buf.Len() already exceeds the cap, so
// ANY next line's prospective size trivially exceeds the cap too, forcing a
// flush of the oversized line alone before the next line is added; if the
// oversized line is the LAST in the batch, Submit's trailing flush sends it.
// maxBytesPerDatagram == 0 (absent field OR an explicit degenerate zero, §3.1)
// needs NO special case either: an empty buf always accepts the first line
// unconditionally, then the NEXT append's prospective size (>= 1) is never
// <= 0, so every line flushes alone before the next is added — reproducing
// phase 49's one-line-per-datagram behavior exactly (AMEND-DSDB-DEFAULT-CONFIRMED).
func (s *DogStatsdSink) appendLine(buf *strings.Builder, line string) {
	if buf.Len() == 0 {
		buf.WriteString(line)
		return
	}
	prospective := uint64(buf.Len()) + 1 + uint64(len(line)) // +1 for the "\n" separator
	if prospective > s.maxBytesPerDatagram {
		s.flush(buf)
		buf.WriteString(line)
		return
	}
	buf.WriteByte('\n')
	buf.WriteString(line)
}

// flush writes buf's contents as ONE UDP datagram (if non-empty) and resets it.
func (s *DogStatsdSink) flush(buf *strings.Builder) {
	if buf.Len() == 0 {
		return
	}
	s.write(buf.String()) // the EXISTING write() — rate-limit-logged-and-dropped on error, UNCHANGED
	buf.Reset()
}
```

This is the "SINGLE unconditional accumulate-then-flush-on-overflow loop" the BRAINSTORM §2.3/§2.4 anticipated, with the EXACT boundary operator now pinned (`>`, strict) and the oversized-alone / zero-cap degenerate cases confirmed to require ZERO conditional branching beyond the two comparisons already shown — a SIMPLER shape than the BRAINSTORM's slightly more eager phrasing ("if the buffer is empty and this single line already exceeds the cap, send it alone IMMEDIATELY") suggested: the oversized line does not need to be flushed synchronously at the moment it's added — it naturally gets flushed by the NEXT `appendLine` call's overflow check (or by `Submit`'s own trailing `flush` call if it happens to be the batch's last line), with no observable difference in emitted datagrams either way.

### 3.4 Byte-stability

Byte-identical when no `dog_statsd` `stats_sinks[]` entry is configured (unaffected code path). Byte-identical DATAGRAM FRAMING when a `dog_statsd` sink IS configured with NO `max_bytes_per_datagram` (or an explicit `0`) — every line still flushes alone, per §3.3's degenerate-case analysis; the ONLY observable change from phase 49 is that lines now pass through `appendLine`/`flush` instead of a direct `write` call, with byte-for-byte identical wire output. The full differential (95-dir today → 96-dir with `0094`) is the regression anchor, including re-running `0093` (unaffected — its bootstrap carries no `max_bytes_per_datagram`) as part of the full suite.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

- REUSED (byte-for-byte, no behavior change): the `internal/statssink` `Flusher`/`Sink` interface; the `snapshot()` cumulative mapping; the sink-private `deltaState` (`delta.go`); `stats.ExtractTags` (`internal/stats/name.go:47`); `formatTagSuffix`'s natural/unsorted tag-order formatting (`dogstatsd.go:110-126`); the rate-limited drop-log `write` method (`dogstatsd.go:131-139`); the `sync.Once` idempotent `Close`; the `bootstrap.go` `parseStatsSinks` three-arm dispatch (unchanged — only `parseDogStatsdSinkConfig`'s BODY changes); the `main.go` post-Freeze flusher build + LIFO-drain; the driver-owned two-per-side-receivers + hard-`Close()` differential harness shape (`reference_periodic_sink_differential_two_receivers`); `differential.HostGatewayIP`-equivalent literal-IP reachability (the `0093` driver's local `hostGatewayIP` helper, duplicated again per `reference_host_gateway_ip_docker_desktop` + `D-DSD-RECEIVER-WIRING`'s import-cycle rationale, unchanged for `0094`).
- NEW: `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (§3.1); the `bootstrap.go:591-593` strict-reject DELETION; `DogStatsdSink.maxBytesPerDatagram` + `appendLine`/`flush` (§3.3); the `main.go:222` third parameter thread; two small additive last-seen-by-name accessors on `test/helpers/statsdrecv.Server` (§8.2 — NOT a parser change, §1.1 AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED); the `0094-stats-sink-dogstatsd-batching` fixture.
- ZERO new Go packages (the change lives entirely in the existing `internal/statssink` and `internal/bootstrap` packages; the receiver extension lives in the existing `test/helpers/statsdrecv`). ZERO new go.mod modules (`strings.Builder`/`net` only; `go mod tidy -diff` anticipated EMPTY).

---

## 5. Proto-field roster (consumed at 50)

From `DogStatsdSink` (`config/metrics/v3/stats.pb.go:695-714`, v1.32.4 — re-verified this session): `max_bytes_per_datagram`(4, `*wrapperspb.UInt64Value`) — LIFTED from strict-reject to CONSUMED (`GetMaxBytesPerDatagram().GetValue()`, §3.1). `address`(1, oneof `dog_statsd_specifier`) and `prefix`(3) are UNCHANGED consumption from phase 49. No other `DogStatsdSink` field exists (`stats.pb.go:695-714` verified in full — the message has exactly these three members). `go mod tidy` adds NOTHING (no new module).

## 6. PARSE-REJECT roster + fuzzer

- **PARSE-REJECT arms (ADR-0080):** UNCHANGED from phase 49 EXCEPT the `max_bytes_per_datagram`-set reject is REMOVED (§3.1). Remaining: a sibling/unknown `stats_sinks[]` TypeURL (the three-arm dispatch's else branch, unchanged); a missing `dog_statsd_specifier`/nil `socket_address` (REFERENCE-PARITY, unchanged). `socket_address.protocol` remains accepted-and-ignored (unchanged).
- **Fuzzer (D-DSDB-FUZZER, resolved §1.1):** NO new fuzzer. `FuzzDogStatsdSinkConfigParse` (`internal/bootstrap/dogstatsd_fuzz_test.go`) already seeds a `max_bytes_per_datagram: 512` case (lines 46-53) that will now exercise the ACCEPT path instead of the reject path — the SAME untrusted-config-parse boundary, no new seed needed (though the PLAN may add a seed exercising a LARGE or boundary-adjacent value for extra mutation coverage — a PLAN-level nicety, not a SPEC requirement). Fuzzers stay **52** (verified `^func Fuzz` == 52 this session, matching the documented count with no drift — `reference_fuzzer_count_docs_drift` does not apply this row).

## 7. Stat surface — +0 (1200 → 1200) (per BRAINSTORM §2.9/§5, no new probe needed — a transport-only change)

No new self-stat, no new sink cluster (the sink dials no cluster, unchanged from phase 49). Surface **1200 → 1200** (non-H2 **1196 → 1196**); pinned at the IMPL via the existing `registration_test.go` guard (mirroring `TestNoNewStat_DogStatsdRegistrationGuard`, e.g. a `TestNoNewStat_DogStatsdBatchingRegistrationGuard` or an extension of the existing test — PLAN decides the exact test name/shape).

## 8. Differential fixture taxonomy (+1: `0094` cross-side proof of batching + value-correctness under batching)

### 8.1 `0094-stats-sink-dogstatsd-batching` (cross-side — reuses the `0093` delta-SUM/tag-set/gauge subset UNCHANGED + two NEW batching-specific proofs)

Clones the `0093-stats-sink-dogstatsd` driver almost verbatim (same H1 downstream → HTTPFixedBody backend topology; same two-per-side-UDP-receiver + hard-`Close()` shape, `reference_periodic_sink_differential_two_receivers`; same `hostGatewayIP`-local-helper duplication, `reference_host_gateway_ip_docker_desktop` + `D-DSD-RECEIVER-WIRING`'s import-cycle rationale carried forward unchanged) with THREE deliberate deltas from `0093`:

1. **A distinct metric prefix** (e.g. `dsdbpfx`, distinct from `0093`'s `dsdpfx` — no coexistence collision risk, though not tested here).
2. **A bootstrap-level `max_bytes_per_datagram` set to the SAME numeric value on BOTH sides** (e.g. `200` — comfortably larger than a typical short untagged/lightly-tagged line [~30-90 bytes observed at §10] so 2-3 ordinary lines naturally co-batch, yet comfortably smaller than the deliberately oversized line below).
3. **A deliberately LONG `backendName` constant** (e.g. ~150-200 characters, vs `0093`'s short `"c_backend"`) — reusing the EXACT SAME subset names/tags `0093` already asserts (`cluster.upstream_rq_total`+`envoy.cluster_name`, `http.downstream_rq_total`+`envoy.http_conn_manager_prefix`, `http.downstream_rq_xx`+`envoy.response_code_class`/`envoy.http_conn_manager_prefix`, `cluster.membership_total`+`envoy.cluster_name`), but because `backendName` is now long, the `envoy.cluster_name`-tagged lines (`cluster.upstream_rq_total`, `cluster.membership_total`) become NATURALLY oversized (their formatted line, `<prefix>.cluster.upstream_rq_total:<delta>|c|#envoy.cluster_name:<~150-char name>`, comfortably exceeds the 200-byte cap on its own — PLAN/IMPL empirically confirms the exact byte math, D-DSDB-CAP §11), while the `envoy.http_conn_manager_prefix`-tagged lines (`http.downstream_rq_total`, `http.downstream_rq_xx`, tagged with a SHORT `statPrefix` unchanged from `0093`'s `"hcm_local"`) stay short and naturally co-batch with each other and with the many other always-present short envoy-go/reference self-stat lines under the 200-byte cap. This ONE change (a longer `backendName`) elegantly produces BOTH the "ordinary co-batching" case AND the "oversized-alone" case from the SAME already-proven `0093` subset, with no separate/parallel stat needed.

- **Assertions (cross-side, per side):**
  - **(a) Value/tag correctness under batching (reused verbatim from `0093`, §8.1 of SPEC-49):** the SAME 3-counter delta-SUM-with-stability-barrier + tag-set subset, PLUS the absolute-gauge-plus-tag subset (`cluster.membership_total == 1`), all == the SAME expected values as `0093` — proving batching does NOT alter WHAT is sent or its extracted tags, only how lines are grouped into datagrams. This is the "emitted line SET unchanged from an unbatched baseline" proof (BRAINSTORM §2.7(a)): `0093`'s already-landed, already-passing assertions on the IDENTICAL subset ARE the unbatched baseline: this fixture proves the SAME values hold when batching is active.
  - **(b) Batching actually occurred (NEW, §8.2):** `srv.MaxLinesInAnyDatagram() > 1` on EACH side's receiver — a datagram containing MORE than one line was actually observed (not merely that the parser tolerates one it never receives), proving real co-location happened.
  - **(c) The oversized line stayed alone and exceeded the cap (NEW, §8.2):** `srv.LinesInDatagram("<prefix>.cluster.upstream_rq_total") == 1` on each side (the long-cluster-name-tagged line was NEVER co-batched with anything, per D-DSDB-OVERSIZED) — proving the "send alone when oversized" behavior is exercised by THIS differential, not merely unit-tested in isolation.
- **UNasserted:** the whole datagram/line set (surfaces differ cross-side, unchanged from `0093`); non-deterministic gauges; the exact per-flush datagram COUNT (only that at least one multi-line datagram exists, and the oversized line's datagram-membership); the literal tag order (asserted as a set, `reference_dogstatsd_tag_order_unsorted`, unchanged from `0093`).
- **Deliberate breaks (`reference_differential_break_protocol_count1` — `-count=1` on EVERY break; `reference_differential_run_selector` — `-run 'TestDifferential/0094'` NEVER bare):**
  - (a) Force one-line-per-datagram regardless of `maxBytesPerDatagram` (e.g. call `s.flush(buf)` unconditionally after every `appendLine`) — `MaxLinesInAnyDatagram() > 1` must FAIL, proving assertion (b) is live.
  - (b) Silently DROP a line that would overflow the current buffer instead of flushing-then-starting-a-new-buffer (a plausible off-by-one bug shape) — the affected counter's delta-SUM must undercount below K, proving assertion (a)'s value-correctness check is live under the NEW batching code path specifically (not merely inherited from `0093`'s unbatched proof).
  - Plus the 20/20 flake-stability run and the FULL-package `-race` on `internal/statssink` AND `internal/stats` (unchanged discipline from `0093`).

### 8.2 `test/helpers/statsdrecv` extension — two additive last-seen-by-name accessors (NOT a parser change, §1.1)

Per AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED, `ingest`'s EXISTING datagram-level `\n`-split (`statsdrecv.go:131`) already correctly ingests multi-line datagrams — NO change to that logic. The ONLY addition is two small maps + accessors, following the file's EXISTING "last-seen per name" idiom (identical in shape to the landed `tags map[string]map[string]string` / `Tags(name)` pair, `statsdrecv.go:49`/`189-194`):

```go
// Server gains two new fields (alongside the existing deltaSums/gauges/seen/tags):
maxLinesInDatagram int            // Server-WIDE: the largest nlines observed in any ingested datagram
linesInDatagram    map[string]int // last-seen per name: how many total lines were in the datagram that most recently carried this name

// In ingest, after computing `lines := strings.Split(...)` (unchanged) and for
// each successfully-parsed line's `name` (inside the existing per-line loop,
// still under the existing mutex):
if n := len(lines); n > s.maxLinesInDatagram {
    s.maxLinesInDatagram = n
}
s.linesInDatagram[name] = len(lines) // overwritten each time — last-seen, the Tags(name) precedent

// New accessors (mirroring the existing Tags/Gauge/SeenCount shape):
func (s *Server) MaxLinesInAnyDatagram() int { ... }                          // Server-wide max
func (s *Server) LinesInDatagram(name string) (int, bool) { ... }             // last-seen per name
```

This is a ~15-20 LoC additive change with NO modification to the existing `deltaSums`/`sumsByTags`/`gauges`/`seen`/`tags` accumulation or the `ingest` delimiter logic — `0093` (which never calls the two new accessors) is completely unaffected. New BackendKind: **NONE** (the receiver EXTENDS the driver-owned `test/helpers/statsdrecv`, unchanged from the phase-49 precedent — `reference_differential_grpc_receiver_driver_owned`; BackendKind tail STAYS **38**).

---

## 9. Behavior-contract delta (the phase-50 bundle; ADR-0267 atomic landing)

Extend the `### Stats sinks — the dog_statsd UDP sink with tags` subsection (landed at phase 49) with a new paragraph: an EXPLICIT `max_bytes_per_datagram` on a bootstrap `dog_statsd` `stats_sinks[]` entry is now HONORED — envoy-go accumulates consecutive formatted DogStatsd lines (in registry-walk order, never reordered/deduped) into a growing buffer and flushes it as one UDP datagram whenever the NEXT line would make the buffer exceed the cap (a buffer landing EXACTLY at the cap still fits); a single line whose own formatted length exceeds the cap is sent alone in its own oversized datagram with no error/drop/truncation. An ABSENT field (or an explicit `0`) continues to emit exactly one line per datagram (byte-identical to phase 49). The stat-surface block stays 1200 (+0). ADR-0267 lands atomically with this contract delta at the phase-50 IMPL.

## 10. SPEC-time empirical-pin block (D-DSDB-* — executed IN-SESSION 2026-07-05)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a dedicated bridge `dsdb-net` `172.31.99.0/24`; an `hashicorp/http-echo` backend; a driver-owned Python UDP dump receiver bound `0.0.0.0:18125` on the host, reached by the reference container at the Docker-Desktop host-gateway literal IP `192.168.65.2` per `reference_host_gateway_ip_docker_desktop`; an H1 downstream :10000 `stat_prefix=ingress_http` → cluster `backend`, a bootstrap `dog_statsd` `stats_sinks[]` → the UDP receiver, `prefix: probepfx`, `stats_flush_interval=1s`; 4 config variants: no-cap/default, `max_bytes_per_datagram: 75`, `max_bytes_per_datagram: 74`, `max_bytes_per_datagram: 120` with a ~130-char cluster name).

| Pin | Result |
|-----|--------|
| **D-DSDB-DEFAULT** | PINNED (AMEND-DSDB-DEFAULT-CONFIRMED). No-cap run: 1942/1942 datagrams `nlines=1` (100%). Absent `max_bytes_per_datagram` is LIVE-confirmed reference-parity with phase 49's existing behavior. |
| **D-DSDB-BOUNDARY** (LOAD-BEARING) | PINNED (AMEND-DSDB-BOUNDARY-CONFIRMED). At cap=75, two adjacent lines totaling EXACTLY 75 bytes (32+1+42) co-located in ONE datagram, every occurrence. At cap=74 (one byte under), the SAME two lines split into two separate datagrams, every occurrence. ⇒ the comparison is `prospective > cap` (STRICT; an exact-fit is NOT an overflow). |
| **D-DSDB-OVERSIZED** | PINNED (AMEND-DSDB-OVERSIZED-CONFIRMED). At cap=120 with a ~130-char cluster name: 715 datagrams exceeded the cap (up to 218 bytes), 100% `nlines=1` (never co-batched), zero truncation (every oversized line's reported length exactly matched its expected full content). |
| **D-DSDB-JOIN-ORDER** | PINNED (AMEND-DSDB-JOIN-ORDER-CONFIRMED). Zero datagrams with a repeated metric name (no dedup); zero instances of any adjacent-pair ordering being observed in both directions across all captured multi-line datagrams (no reordering). |
| **D-DSDB-FUZZER** | RESOLVED (code-reading, not a live probe). `FuzzDogStatsdSinkConfigParse`'s existing seed corpus already exercises `max_bytes_per_datagram` through the FULL `bootstrap.Load` parse path; the packing algorithm itself operates on trusted, already-validated inputs and does not fit the "new parse-path fuzzer" convention. NO new fuzzer; stays 52. |
| **AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED** | RESOLVED (code-reading, not a live probe). `test/helpers/statsdrecv.go:131`'s `ingest` ALREADY splits a datagram payload on `\n` before per-line parsing — traced byte-for-byte with a concrete two-line example, confirming zero collision and zero parser change needed. |

## 11. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-DSDB-SPLIT** — the FINAL ADR-0045 re-check at PLAN with real LoC (a single flat row is anticipated, smaller than phase 49; §3.0).
- **D-DSDB-CAP** — the EXACT `0094` fixture cap value + `backendName` length that (a) comfortably batches the short HCM-tagged subset lines together and (b) comfortably makes the cluster-tagged subset lines exceed the cap alone — confirmed empirically at PLAN/IMPL against the ACTUAL envoy-go + reference line formats (this SPEC proposes cap=200 / backendName ~150-200 chars as a starting point, §8.1).
- **D-DSDB-STATS-FINAL** — the exact name/shape of the +0 registration guard test extension (§7).
- **D-DSDB-FUZZER-SEED** — whether the PLAN adds an extra fuzz seed (e.g. a boundary-adjacent or very-large `max_bytes_per_datagram` value) for additional mutation coverage, beyond the already-sufficient existing seed (§6, a nicety not a requirement).
- **D-DSDB-BUFFER-TYPE** — whether `appendLine`/`flush` use `strings.Builder` (as sketched in §3.3) or a `[]byte`/`bytes.Buffer` — a PLAN-level implementation-ergonomics choice with no behavioral difference; `strings.Builder` is proposed as the default (matches `formatTagSuffix`'s existing use of `strings.Builder`, `dogstatsd.go:114`).

## 12. ADR continuity — the ADR-0267 §Context DRAFT (anchored here; full entry lands at the phase-50 IMPL)

**ADR-0267 §Context (draft):** Phase 49 (ADR-0266) landed the `dog_statsd` UDP line-protocol stats sink WITH TAGS as envoy-go's THIRD `stats_sinks[]` consumer, STRICT-REJECTING an explicit `max_bytes_per_datagram` because the reference was confirmed (at the phase-49 SPEC live probe) to genuinely batch multiple metrics per UDP datagram when the field is set — a real, deferred feature, not a no-op. Phase 50 opens the SEVENTH Observability row by lifting that strict-reject into a genuine accept-and-honor path: a new `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` field (parsed via `dsd.GetMaxBytesPerDatagram().GetValue()`, which already returns `0` for an absent field — no pointer needed, since an absent field and an explicit `0` are BEHAVIORALLY IDENTICAL under the packing algorithm below), threaded into a rewritten `DogStatsdSink.Submit` that accumulates the ALREADY-formatted DogStatsd lines (delta/tag computation, and the line-formatting itself, are completely UNCHANGED from phase 49) into a growing buffer, flushing it as one UDP datagram whenever the NEXT line would push the buffer's size (plus a `\n` separator) STRICTLY over the configured cap. The 2026-07-05 live probe (D-DSDB-DEFAULT/BOUNDARY/OVERSIZED/JOIN-ORDER, contrib-v1.37.2) pinned the EXACT comparison operator (a buffer landing exactly at the cap after appending still fits — only strict excess triggers a flush-before-append, confirmed both directions with a byte-exact two-line test at cap=75 vs cap=74) and confirmed a line exceeding the cap on its own is sent alone with no error/drop/truncation (up to 218 bytes observed against a 120-byte cap, zero truncation across 715 oversized datagrams), and that multi-line datagrams preserve strict sequential emission order with no reordering or deduplication. A separate, non-probe finding (re-reading the ACTUAL landed phase-49 `test/helpers/statsdrecv` code, not the phase-49 BRAINSTORM's prose description of it) showed the receiver's datagram-level `\n`-split ALREADY existed and needed no change — the ONLY receiver-side addition is two small additive last-seen-by-name accessors (`MaxLinesInAnyDatagram`/`LinesInDatagram`) enabling the `0094` differential to prove batching occurred and prove an oversized line stayed alone, without touching the (already-correct) ingestion logic. The packing algorithm requires NO conditional special-casing for the absent/zero-cap case OR the oversized-single-line case — both fall out of the SAME two-comparison general loop. Phase 50 adds the `internal/statssink/dogstatsd.go` `maxBytesPerDatagram` field + `appendLine`/`flush` (replacing the phase-49 per-line `write` call site), the `bootstrap.go` field addition + strict-reject deletion, the `main.go:222` parameter thread, the `test/helpers/statsdrecv` accessor pair, and the `0094-stats-sink-dogstatsd-batching` cross-side differential (reusing `0093`'s already-proven value/tag-correctness subset verbatim, plus two new batching-specific proofs). ZERO new packages, ZERO new go.mod modules. `graphite`/OTLP-metrics sinks, the plain-statsd `tcp_cluster_name` transport, and tracing/tap-filter candidates remain deferred. §Decision/§Consequences land at the phase-50 IMPL. ANCHORS ADR-0267.

## 13. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat **1200** (H2 cluster; non-H2 **1196**) / fixtures **95** / fuzzers **52** / BackendKind **38** / DECISIONS **ADR-0266** (next-free **ADR-0267**). Anticipated at the phase-50 IMPL: stat **1200** (+0 — transport-layer only, no new self-stats) / fixtures **96** (`0094-stats-sink-dogstatsd-batching`) / fuzzers **52** (UNCHANGED — D-DSDB-FUZZER resolved NO new fuzzer) / BackendKind **38** (extended driver-owned UDP receiver, no new kind) / DECISIONS **ADR-0267** (next-free ADR-0268) / **+0 go.mod modules**, **+0 packages**. ROADMAP row 50 flips **`done`** at the phase-50 IMPL six-gate (the sole leg — ADR-0106; the Observability family STAYS OPEN). Next → the phase-50 PLAN (`superpowers:writing-plans` — the §11 D-DSDB-* PLAN questions, esp. D-DSDB-CAP + D-DSDB-STATS-FINAL + D-DSDB-BUFFER-TYPE; a fresh worktree off master per `feedback_git_worktrees`), then the phase-50 IMPL (subagent-driven per `feedback_execution_style`).
