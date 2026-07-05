# Phase 50 Implementation Plan — `dog_statsd max_bytes_per_datagram` real multi-metric datagram batching: DELETE the `bootstrap.go:591-593` strict-reject + add `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` + a buffer-accumulate-then-flush-on-overflow rewrite of `DogStatsdSink.Submit`'s per-line `write` call site (delta/tag/line-formatting UNCHANGED) + a minimal additive `test/helpers/statsdrecv` extension (two last-seen-by-name accessors; NO parser change) + the `0094-stats-sink-dogstatsd-batching` cross-side differential — a SINGLE FLAT ROW, transport-layer-only; the SEVENTH Observability-family row; ZERO new packages, ZERO new go.mod modules; ANCHORS ADR-0267; row 50 flips `done` at this IMPL six-gate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** When a bootstrap `dog_statsd` `stats_sinks[]` entry carries an EXPLICIT `max_bytes_per_datagram`, envoy-go now HONORS it: `DogStatsdSink.Submit` accumulates consecutive ALREADY-formatted DogStatsd lines (registry-walk emission order, unchanged) into a growing buffer and flushes the buffer as ONE UDP datagram whenever the NEXT line would make the buffer STRICTLY exceed the cap (a buffer landing EXACTLY at the cap after appending still fits — the live-proven boundary operator, SPEC-50 §1.1 AMEND-DSDB-BOUNDARY-CONFIRMED). A single line whose own formatted length exceeds the cap is sent alone in its own oversized datagram — no error, no drop, no truncation, and NO special-cased branch (it falls out of the SAME general algorithm). An ABSENT field (or an explicit `0`) continues to emit exactly one line per datagram, byte-identical to the landed phase-49 behavior. Proven cross-side by the `0094-stats-sink-dogstatsd-batching` differential (reusing `0093`'s already-proven delta-SUM/tag-set/gauge subset assertions verbatim, PLUS two new batching-specific proofs: at least one multi-line datagram was observed, and a deliberately oversized line stayed alone). **ANCHORS ADR-0267** (its §Decision/§Consequences body lands atomically here); ROADMAP row 50 (`stats-sink-dogstatsd-batching`) flips **`done`** at this IMPL six-gate (the sole leg — ADR-0106; NO parent rollup); the Observability family STAYS OPEN.

**Architecture:** ZERO new `Sink` impl, ZERO new framework piece. A NEW `uint64` field threaded through THREE call sites (`bootstrap.go`'s config struct + parse arm, `dogstatsd.go`'s constructor, `main.go`'s build loop); a rewrite of ONE method (`DogStatsdSink.Submit`'s per-line `s.write(line)` call site becomes a buffer-accumulate-then-flush-on-overflow pair, `appendLine`/`flush`) — the delta-state transform, the `stats.ExtractTags` tag extraction, and the DogStatsd line-formatting (`formatTagSuffix`, the `name+":"+value+suffix+tagSuffix` construction) are BYTE-FOR-BYTE UNCHANGED; a small additive pair of last-seen-by-name accessors on the driver-owned `test/helpers/statsdrecv.Server` (mirroring the file's EXISTING `tags`/`Tags(name)` idiom — NOT a parser change, the datagram-level `\n`-split at `statsdrecv.go:131` already exists and needs zero modification, SPEC-50 §1.1 AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED); one new differential fixture cloning `0093`'s driver almost verbatim. Byte-identical and stat-surface-identical when no `dog_statsd` sink is configured, or one is configured with NO `max_bytes_per_datagram` (or an explicit `0`) — the regression anchor is the full differential (re-running `0093` unaffected, plus the new `0094`).

**Tech Stack:** Go; the EXISTING `internal/statssink` package (`dogstatsd.go` modified, no new file); `internal/bootstrap` (the `DogStatsdSinkConfig` struct + `parseDogStatsdSinkConfig` modified, the strict-reject DELETED); `cmd/envoy-go/main.go` (the `NewDogStatsdSink` call site's third argument); the driver-owned `test/helpers/statsdrecv` (two additive accessors); the Docker-bridge differential harness. Pure `strings.Builder`/`net` — NO new client lib, NO new go.mod module. **ZERO new go.mod modules, ZERO new packages.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1200** (H2 cluster; non-H2 **1196**) → **1200** (+0, transport-layer only); fixtures **95** → **96** (`0094`); fuzzers **52** → **52** (UNCHANGED — D-DSDB-FUZZER resolved NO new fuzzer at SPEC); BackendKind **38** → **38** (the extended driver-owned UDP receiver is NOT a new BackendKind); DECISIONS tail **ADR-0266** → **ADR-0267** (next-free ADR-0268); **+0 go.mod modules, +0 packages**.
- **Module path:** `github.com/esalaine/envoy-go`. Go **1.23.0** (`maps.Equal` available, the `0093` precedent).
- **No new dependency:** `go mod tidy -diff` MUST be EMPTY at every task.
- **Process anchors:** ADR-0044 (ADR §Decision+§Consequences land at IMPL) · ADR-0045 (sub-split soft gate — escape-valve UNCONSUMED; re-checked at Task 1) · ADR-0080 (strict-reject anti-silent-divergence — **NOTE this row REMOVES a prior strict-reject, the reverse direction**: lifting an ALREADY-strict-rejected field into an honored one is not itself an ADR-0080 concern, since the field is now genuinely implemented, not silently ignored) · ADR-0106 (per-leg rows; row 50 flips `done` here, no parent rollup) · ADR-0266 (the phase-49 dog_statsd sink this row extends) · ADR-0267 (this leg — ANCHORED here).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit, every task.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0094'`, NEVER bare `'0094'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the `DogStatsdSink` remains synchronous (no new goroutine — buffering happens entirely within one `Submit` call), but the `Flusher` ticker remains a background mutator — the `-race` gate MUST run the FULL `internal/statssink` package.
- **Boundary operator (LOAD-BEARING, live-proven at SPEC):** the overflow comparison is `prospective > cap` (STRICT) — a buffer landing EXACTLY at the cap after appending STILL FITS. Do NOT implement `>=` (that would split an exact-fit pair the reference co-locates — SPEC-50 §1.1 AMEND-DSDB-BOUNDARY-CONFIRMED, live-proven both directions at cap=75/74).
- **Two per-side receivers + hard Close** (`reference_periodic_sink_differential_two_receivers`): periodic flushes stream for the whole test; one shared receiver cross-contaminates.
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the UDP receiver is the EXTENDED `test/helpers/statsdrecv` server — NOT a runner BackendKind (stays 38).
- **Docker bridge + literal IP** (`reference_docker_probe_bridge_network` · `reference_host_gateway_ip_docker_desktop`): reuse the `0093` driver's LOCAL `hostGatewayIP` helper VERBATIM — do NOT attempt `differential.HostGatewayIP` from the driver (the SAME import-cycle avoidance `0093`/`0092` already solved: `runner_test.go` blank-imports the driver from within `package differential`).
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the DogStatsd line + datagram-packing behavior is shared — the SPEC §10 live probe is the wire truth.
- **DogStatsd tag order stays unsorted** (`reference_dogstatsd_tag_order_unsorted`): unaffected by this row (batching never touches tag formatting) — carried forward because `0094` reuses the SAME `formatTagSuffix`/subset-tag assertions as `0093`.
- **Admin-interface wire-name collision** (`reference_admin_interface_wire_name_collision`): `0094` reuses `DeltaSumTagged` (the `0093` fix), unaffected by batching — the admin interface's own HCM stats still collapse to the SAME residual wire names, differing only by tag value.

---

## Orientation — read before Task 1 (the zero-context brief)

You are lifting ONE bootstrap strict-reject into an honored config field, and rewriting ONE method's final "write a datagram" step into a two-step buffer-accumulate-then-flush pair. The delta-state transform, `stats.ExtractTags`, and the entire DogStatsd line-formatting pipeline (residual name, prefix join, tag-suffix construction, natural/unsorted tag order) are landed at phase 49 and are COMPLETELY UNTOUCHED by this row — batching is a pure transport-layer concern applied strictly AFTER a line string already exists. **No new framework piece, no new package, no new module, no new fuzzer.**

**What ALREADY works (do NOT re-build) — verified at PLAN time (2026-07-05; re-confirm line numbers before editing — files evolve):**

- **`internal/statssink/dogstatsd.go`** (149 LoC) — `DogStatsdSink{conn *net.UDPConn; prefix string; delta *deltaState; closeOnce sync.Once; closeErr error; lastDropLog atomic.Int64}`; `NewDogStatsdSink(udpAddr, prefix string) (*DogStatsdSink, error)` (`:56`); `Submit(batch []*dto.MetricFamily)` (`:72-103`) — applies `s.delta.apply(batch)`, then per family computes `suffix`/`residual`/`labels`/`name`/`tagSuffix` (UNCHANGED by this row), then per metric builds `line := name + ":" + strconv.FormatInt(...) + suffix + tagSuffix` and calls `s.write(line)` (`:100` — **THIS is the ONLY call site this row's rewrite touches**); `formatTagSuffix(labels []stats.Label) string` (`:110-126`, UNCHANGED — natural/unsorted order, `strings.Builder`); `write(line string)` (`:131-139`, UNCHANGED — rate-limit-logged-and-dropped on error, the `sink.go` `lastDropLog`/`dropLogIntervalNanos` idiom); `Close() error` (`:142-149`, UNCHANGED — idempotent `sync.Once`).
- **`internal/bootstrap/bootstrap.go`** — `DogStatsdSinkConfig` struct (`:301-304`: `UDPAddress string`; `Prefix string`); `parseDogStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error` (`:586-607`): unmarshal → **the strict-reject `if dsd.GetMaxBytesPerDatagram() != nil { return ... }` at `:591-593`, TO BE DELETED ENTIRELY** → the nil-`socket_address` reject (`:594-597`) → `prefix` default (`:598-601`) → append to `result.DogStatsdSinkConfigs` (`:602-605`). The three-arm `stats_sinks[]` dispatch (`parseStatsSinks`, unchanged elsewhere) already routes `dogStatsdSinkTypeURL` here.
- **`cmd/envoy-go/main.go`** — the sink build block: the three-way OR gate (`:194`, unchanged), the metrics_service loop, the statsd loop, THEN the dog_statsd loop (`:221-227`): `sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix)` (`:222` — **the ONLY `main.go` call site this row's Task 4 touches**, gaining a third argument), `statsFlusher = statssink.NewFlusher(...)` (`:228`). The comment block above the gate (`:180-189`) already names all three sink kinds.
- **`test/helpers/statsdrecv/statsdrecv.go`** (269 LoC) — the driver-owned UDP receiver, LANDED at phase 49 with `DeltaSum`/`DeltaSumTagged`/`Gauge`/`SeenCount`/`Tags`/`Reset`/`Addr`/`Close`. **`ingest` (`:128-184`) ALREADY splits a received datagram's payload on `\n` BEFORE per-line parsing** (`:131`: `for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n")`), confirmed by a byte-for-byte trace this SPEC (§1.1 AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED) — **do NOT touch this function.** The last-seen-per-name idiom to mirror for the NEW accessors: `tags map[string]map[string]string` field (`:49`) + `Tags(name string) (map[string]string, bool)` accessor (`:189-194`).
- **`internal/statssink/registration_test.go:79-99`** — `TestNoNewStat_DogStatsdRegistrationGuard` calls `NewDogStatsdSink("127.0.0.1:65535", "envoy")` (TWO-arg, the phase-49 signature). **This call site BREAKS once Task 3 adds a third parameter — it must be updated in the SAME task** (a compile-time break, not a semantic one; passes `0` for "no cap").
- **`internal/statssink/dogstatsd_test.go`** (290 LoC, phase 49) — ELEVEN existing `NewDogStatsdSink(addr, prefix)` call sites (lines `21,43,68,88,130,178,209,230,249,266,282`, verified via `grep -n 'NewDogStatsdSink(' internal/statssink/dogstatsd_test.go` at PLAN time) exercising delta/tag/lifecycle behavior UNRELATED to batching. **ALL ELEVEN break at Task 3 and must be updated to pass a third argument (`0` — preserving their EXACT existing "always one-per-datagram" semantics unchanged).** The file also defines `read(n int) []string` (via the shared `udpListener` helper in `statsd_test.go`) which reads exactly `n` DATAGRAMS (not lines) as raw strings — a multi-line datagram comes back as ONE element containing embedded `\n`s; Task 3's NEW batching tests must `strings.Split` that element on `\n` to inspect individual lines.
- **`test/fixtures/0093-stats-sink-dogstatsd/driver/driver.go`** (713 LoC) — **the `0094` driver TEMPLATE.** Clone verbatim then adapt: a distinct `prefix` constant; a deliberately LONG `backendName` constant (D-DSDB-CAP below); a `maxBytesPerDatagram` bootstrap field threaded into BOTH `envoy.yaml`/`envoy-go.yaml` templates; reuse `subsetNames`/`subsetTags`/`gaugeName`/`gaugeTags`/`sideSnapshot`/`driveSide`/`pollSubset`/`subsetConverged`/`awaitFurtherFlushes`/`fireProbe`/`mustAllocateUDPPort`/`mustStartReceiver`/`ensure`/`closeServers`/the LOCAL `hostGatewayIP` duplicate/`ProbeAdmin`/`fixtureDir`/`mustReadFixtureFile`/`mustRender` VERBATIM (unchanged); ADD two batching-specific assertions to `AssertStats`/`assertSide` (`MaxLinesInAnyDatagram() > 1`, `LinesInDatagram(<cluster-tagged-name>) == 1`).

**The packing algorithm (SPEC-50 §3.3 — live-pinned, PLAN adopts verbatim):**

```go
// DogStatsdSink gains ONE new field:
type DogStatsdSink struct {
	conn                *net.UDPConn
	prefix              string
	delta               *deltaState
	maxBytesPerDatagram uint64 // NEW (ADR-0267): 0 means "one metric per datagram" (phase-49 behavior)

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

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
// if appending would make it STRICTLY exceed maxBytesPerDatagram (the
// comparison is `>`, NOT `>=` — a buffer that lands EXACTLY at the cap after
// appending still fits, live-proven AMEND-DSDB-BOUNDARY-CONFIRMED). When buf is
// EMPTY, the line is ALWAYS accepted unconditionally — even if the line alone
// exceeds the cap — which is what makes an oversized single line "sent alone"
// fall out of the SAME general algorithm with NO special-cased branch: on the
// NEXT call, buf.Len() already exceeds the cap, so ANY next line's prospective
// size trivially exceeds the cap too, forcing a flush of the oversized line
// alone before the next line is added; if the oversized line is the LAST in
// the batch, Submit's trailing flush sends it. maxBytesPerDatagram == 0
// (absent field or an explicit degenerate zero) needs NO special case either:
// an empty buf always accepts the first line unconditionally, then the NEXT
// append's prospective size (>= 1) is never <= 0, so every line flushes alone
// before the next is added — reproducing phase 49's one-line-per-datagram
// behavior exactly.
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

---

## D-question resolutions (the SPEC §11 D-DSDB-* PLAN/IMPL pins — settled here)

**D-DSDB-SPLIT → NO sub-split (a SINGLE FLAT ROW, 8 tasks — FEWER than phase 49's 9).** Anticipated ~80–150 prod LoC: the `appendLine`/`flush` rewrite (~30 LoC net), the config field + parse-arm edit (~5 LoC), the `main.go` third-argument thread (~1 LoC), the `statsdrecv` accessor pair (~15–20 LoC), the driver (~700 LoC, but almost entirely a clone of the landed `0093` driver — same shape as `0093`'s own relationship to `0092`). Well under the ADR-0045 gate; the 50.1/50.2 escape-valve stays UNCONSUMED. Re-confirmed at Task 1 with the real baseline. **8 tasks (not 9 like phase 49)** — no new fuzzer task, no new bootstrap-dispatch-arm task (this row edits an EXISTING arm's body, not adds a new one).

**D-DSDB-CAP → cap = `200`; `backendName` = a ~160-character literal constant; `statPrefix` stays SHORT (`"hcm_local"`, the `0093` value, unchanged).** Reasoning (byte-math sketch, confirmed EXACTLY at Task 6 via a quick `len()` check before locking the fixture): the HCM-tagged subset lines (`http.downstream_rq_total`/`http.downstream_rq_xx`, tagged with the SHORT `statPrefix`) come out to roughly 70–110 bytes each — comfortably UNDER 200, so at least two of the three subset lines (plus the many other always-present short envoy-go self-stat lines) naturally co-batch under a 200-byte cap. The cluster-tagged subset lines (`cluster.upstream_rq_total`/`cluster.membership_total`, tagged with the LONG `backendName`) come out to roughly `59 + len(backendName)` bytes (the fixed prefix+residual+colon+value+type+tag-key-prefix overhead is ~59 bytes) — a 160-character `backendName` pushes this to ~219 bytes, comfortably OVER the 200-byte cap, guaranteeing these specific lines are ALWAYS sent alone. **Task 6 Step 1 computes the EXACT byte lengths from the ACTUAL rendered lines (not this hand-estimate) before finalizing the fixture** — if the estimate is off, adjust `backendName`'s length (not the cap) to preserve the "HCM lines batch, cluster lines don't" design intent.

**D-DSDB-STATS-FINAL → +0, confirmed via the EXISTING `TestNoNewStat_DogStatsdRegistrationGuard` (`registration_test.go:79-99`), NOT a new test.** This test's `NewDogStatsdSink` call site needs its THIRD argument added (Task 3, mechanical) — once fixed, it continues to assert the surface is unchanged (a transport-layer change registers no new stat). No new registration test is warranted.

**D-DSDB-FUZZER-SEED → NO extra seed added.** The existing `FuzzDogStatsdSinkConfigParse` seed at `max_bytes_per_datagram: 512` (`dogstatsd_fuzz_test.go:46-53`) already exercises this field through the parse arm; once the strict-reject is removed this seed simply exercises the ACCEPT path instead of the reject path under fuzzing mutation — the SAME untrusted-config-parse boundary. Task 2 confirms this seed still runs clean (a nicety check, not a new seed).

**D-DSDB-BUFFER-TYPE → `strings.Builder`** (SPEC §3.3's sketch, adopted verbatim) — matches `formatTagSuffix`'s EXISTING use of `strings.Builder` (`dogstatsd.go:114`) for stylistic consistency within the same file; no behavioral difference vs a `[]byte`/`bytes.Buffer` alternative.

---

## File structure (decomposition locked here)

**Production (modified only — ZERO new production files):**
- `internal/bootstrap/bootstrap.go` — delete the `:591-593` strict-reject; add `MaxBytesPerDatagram uint64` to `DogStatsdSinkConfig` (`:301-304`); thread `dsd.GetMaxBytesPerDatagram().GetValue()` into the `DogStatsdSinkConfig{...}` literal (`:602-605`).
- `internal/statssink/dogstatsd.go` — add `maxBytesPerDatagram uint64` field; add the third `NewDogStatsdSink` parameter; replace the `Submit` per-line `s.write(line)` call site with `s.appendLine(&buf, line)` + a trailing `s.flush(&buf)`; add `appendLine`/`flush`.
- `cmd/envoy-go/main.go` — thread `cfg.MaxBytesPerDatagram` into the `NewDogStatsdSink` call (`:222`).
- `test/helpers/statsdrecv/statsdrecv.go` — add `maxLinesInDatagram int` + `linesInDatagram map[string]int` fields; update `ingest` to populate them (NO change to the existing split/parse logic); add `MaxLinesInAnyDatagram() int` + `LinesInDatagram(name string) (int, bool)` accessors.

**Test (modified):**
- `internal/statssink/dogstatsd_test.go` — update ALL ELEVEN existing `NewDogStatsdSink` call sites (third arg `0`); ADD the new batching test cases.
- `internal/statssink/registration_test.go` — update the ONE `NewDogStatsdSink` call site (third arg `0`).
- `internal/bootstrap/bootstrap_test.go` — flip the `max_bytes_per_datagram`-set reject test to an ACCEPT test (asserting `DogStatsdSinkConfigs[0].MaxBytesPerDatagram == <value>`).
- `internal/bootstrap/dogstatsd_fuzz_test.go` — no structural change (the existing seed now exercises accept, not reject); a brief re-run confirms no panic.
- `test/helpers/statsdrecv/statsdrecv_test.go` — ADD tests for the two new accessors.

**Test (created):**
- `test/fixtures/0094-stats-sink-dogstatsd-batching/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.

**Test (modified):**
- `test/differential/runner_test.go` (blank-import the `0094` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/PROGRESS-50.md`, `docs/envoy-go/DECISIONS.md` (ADR-0267 §Decision/§Consequences — ANCHORS the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 50 flips `done`**).

---

## Task 1: Phase scaffolding — PROGRESS-50.md + baselines + the final ADR-0045 split re-check (D-DSDB-SPLIT)

**Files:**
- Create: `docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/PROGRESS-50.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-50.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                   # expect 95 (tail 0093-stats-sink-dogstatsd)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 52
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go # expect = the BackendKind tail (38)
go mod tidy -diff                                                # expect EMPTY (clean)
grep -n 'NewDogStatsdSink(' internal/statssink/dogstatsd_test.go internal/statssink/registration_test.go cmd/envoy-go/main.go  # expect exactly 13 call sites (11+1+1), all TWO-arg
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **95** / fuzzers **52** / BackendKind **38** / DECISIONS tail **ADR-0266** (next-free **ADR-0267**).

- [ ] **Step 2: Write the PROGRESS-50.md scaffold** — a header (phase 50 IMPL, the SPEC-50 reference + the "SEVENTH Observability-family row, transport-layer-only batching over the LANDED phase-49 DogStatsdSink; ANCHORS ADR-0267; row 50 flips `done` at this IMPL" note, the worktree branch), a task checklist mirroring this plan, the baseline block, the **D-DSDB-SPLIT confirmation (NO sub-split — the escape-valve stays UNCONSUMED; the LoC estimate above; 8 tasks, one fewer than phase 49 since no new fuzzer / no new dispatch arm are needed)**, and the anticipated exit counts: stat **1200** (+0) / fixtures **96** (`0094-stats-sink-dogstatsd-batching`) / fuzzers **52** (UNCHANGED) / BackendKind **38** / DECISIONS **ADR-0267** / **0 new packages, 0 new go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/PROGRESS-50.md
git commit -m "phase 50 Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (dog_statsd max_bytes_per_datagram batching; ANCHORS ADR-0267; row 50 flips done at this IMPL)"
```

---

## Task 2: Lift the `max_bytes_per_datagram` strict-reject — `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` + the `parseDogStatsdSinkConfig` edit (`internal/bootstrap/bootstrap.go`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Produces: `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (Task 3's `main.go` — actually Task 4's — reads it; the packing algorithm's cap).

- [ ] **Step 1: Locate and flip the existing reject test.** `grep -n 'max_bytes_per_datagram' internal/bootstrap/bootstrap_test.go` finds the phase-49 test asserting `Load` REJECTS an explicit `max_bytes_per_datagram` (mirroring the `dogstatsd_fuzz_test.go` seed's reject-arm comment). Rewrite it as an ACCEPT test:
```yaml
stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      max_bytes_per_datagram: 512
```
  ⇒ `Load` SUCCEEDS; `bs.DogStatsdSinkConfigs[0].MaxBytesPerDatagram == 512`.
  ADD a companion test: NO `max_bytes_per_datagram` field at all ⇒ `MaxBytesPerDatagram == 0` (the absent-field default, `GetValue()`'s nil-safe zero).
  ADD a companion test: an EXPLICIT `max_bytes_per_datagram: 0` ⇒ ALSO `MaxBytesPerDatagram == 0` (identical to absent — no special-cased validation, per the BRAINSTORM Q3/SPEC §2.2 design).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/bootstrap/ -run 'TestDogStatsd.*MaxBytes|TestParseDogStatsd' -count=1` ⇒ FAIL (the OLD reject-arm test still expects an error; the field doesn't exist on the struct yet).

- [ ] **Step 3: Implement.** In `bootstrap.go`:
  - Add the field to `DogStatsdSinkConfig` (`:301-304`):
```go
type DogStatsdSinkConfig struct {
	UDPAddress          string // socket_address host:port (an IP literal:port; net.ResolveUDPAddr-resolvable)
	Prefix              string // DogStatsdSink.prefix, default "envoy" when empty
	MaxBytesPerDatagram uint64 // NEW (ADR-0267): 0 (absent or explicit) means "one metric per datagram" (phase-49 behavior, UNCHANGED); >0 batches consecutive lines up to the cap
}
```
  - DELETE the strict-reject block (`:591-593`) entirely:
```go
	if dsd.GetMaxBytesPerDatagram() != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram is not supported (envoy-go emits one metric per datagram)", idx)
	}
```
  - Add the field to the struct literal (`:602-605`):
```go
	result.DogStatsdSinkConfigs = append(result.DogStatsdSinkConfigs, DogStatsdSinkConfig{
		UDPAddress:          fmt.Sprintf("%s:%d", sa.GetAddress(), sa.GetPortValue()),
		Prefix:              prefix,
		MaxBytesPerDatagram: dsd.GetMaxBytesPerDatagram().GetValue(),
	})
```
  - Update the doc-comments above `DogStatsdSinkConfig` and `parseDogStatsdSinkConfig` to remove the now-stale "STRICT-REJECTS ... max_bytes_per_datagram" language and describe the new honored field instead (ADR-0267).

- [ ] **Step 4: Run to verify they pass.** `go test ./internal/bootstrap/ -run 'TestDogStatsd|TestParseDogStatsd' -count=1` ⇒ PASS. Then the FULL package: `go test ./internal/bootstrap/ -count=1` ⇒ ALL PASS (confirm no OTHER test relied on the reject — e.g. a sibling-reject table row using `max_bytes_per_datagram` as its "currently unsupported" example, mirroring the Task-2-Step-4 gotcha PLAN-49 caught; `grep -n 'max_bytes_per_datagram' internal/bootstrap/bootstrap_test.go` to be sure only the ONE test targeted in Step 1 mentions it). `go mod tidy -diff` ⇒ EMPTY.

- [ ] **Step 5: Confirm the fuzzer seed still runs clean (D-DSDB-FUZZER-SEED).** `go test ./internal/bootstrap/ -run 'FuzzDogStatsdSinkConfigParse' -count=1` (seed-only, now exercising the ACCEPT path for the `max_bytes_per_datagram: 512` seed) ⇒ PASS, no panic. Confirm the running fuzzer count is STILL 52: `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **52** (UNCHANGED — no new fuzzer this row).

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 50 Task 2: lift the max_bytes_per_datagram strict-reject into DogStatsdSinkConfig.MaxBytesPerDatagram uint64 (absent/explicit-0 both parse to 0 -- no pointer needed; ADR-0267)"
```

---

## Task 3: The `appendLine`/`flush` buffer-accumulate-then-flush-on-overflow rewrite of `DogStatsdSink.Submit` (`internal/statssink/dogstatsd.go`) [TDD, table-driven; full-package `-race`]

**Files:**
- Modify: `internal/statssink/dogstatsd.go`, `internal/statssink/dogstatsd_test.go`, `internal/statssink/registration_test.go`

**Interfaces:**
- Consumes: `strings.Builder` (already imported for `formatTagSuffix`); the EXISTING `udpListener`/`sameSet` helpers in `statsd_test.go` (same package, reused).
- Produces: `NewDogStatsdSink(udpAddr, prefix string, maxBytesPerDatagram uint64) (*DogStatsdSink, error)` — a BREAKING signature change (Task 4's `main.go` call site depends on it).

- [ ] **Step 1: Update ALL THIRTEEN existing `NewDogStatsdSink` call sites to compile FIRST (mechanical, no semantic change)** — add a third argument `0` to every call in `dogstatsd_test.go` (11 sites: lines `21,43,68,88,130,178,209,230,249,266,282` — re-verify via `grep -n 'NewDogStatsdSink(' internal/statssink/dogstatsd_test.go`) and in `registration_test.go` (`:87`). `0` preserves each test's EXISTING "always one-per-datagram" semantics exactly (cap absent/zero degenerates to the phase-49 behavior — no test's assertions change).

- [ ] **Step 2: Write the NEW failing tests** in `dogstatsd_test.go` (ADD; do not remove the existing tests):
  - **exact-boundary co-locate (LOAD-BEARING — mirrors the SPEC's live D-DSDB-BOUNDARY probe):** construct two untagged counters whose formatted lines have KNOWN, computed byte lengths — e.g. `reg.NewCounter("x").Add(1)` with prefix `"p"` gives line `"p.x:1|c"` (7 bytes: verify via `len()` in the test, do not hardcode a possibly-wrong number in prose); `reg.NewCounter("yy").Add(2)` gives `"p.yy:2|c"` (8 bytes). Compute `capExact := len(line1) + 1 + len(line2)` (the exact combined length INCLUDING the `\n` separator) at test-write time. `NewDogStatsdSink(addr, "p", uint64(capExact))`; `Submit`; `read(1)` ⇒ ONE datagram whose `strings.Split(got[0], "\n")` yields BOTH lines (order as emitted). This proves the boundary is INCLUSIVE.
  - **exact-boundary-minus-one → split:** the SAME two lines, `NewDogStatsdSink(addr, "p", uint64(capExact-1))`; `Submit`; `read(2)` ⇒ TWO separate single-line datagrams. This proves `capExact` was the TRUE boundary (not merely "large enough for both regardless").
  - **oversized-single-line-alone:** a counter with a long name (e.g. `strings.Repeat("z", 40)`) whose line comfortably exceeds a small cap (e.g. cap `10`), PLUS a normal short second counter. `Submit`; `read(2)` ⇒ the oversized line arrives ALONE in its own datagram (`nlines` == 1 for it, i.e. no embedded `\n`), the short line arrives in its own (possibly separate) datagram — confirm the oversized line's FULL content is present (no truncation: `len(gotOversizedDatagram) == len(expectedOversizedLine)` exactly).
  - **multi-line preserves order across repeated flushes:** THREE untagged counters with a cap generous enough to hold all three in one datagram (e.g. `100`); `Submit` (first flush) → `read(1)` → split on `\n` → capture the observed line ORDER; `Submit` again (second flush, same registry, different or same values) → `read(1)` → split → assert the SAME relative order recurs. This proves no accidental reordering (mirrors AMEND-DSDB-JOIN-ORDER-CONFIRMED's live-probe methodology: order STABILITY across repeated flushes, not a hardcoded expected order — the registry's own walk order is whatever it is, the algorithm must not perturb it).
  - **cap = 0 (explicit) behaves identically to cap absent:** re-run the EXISTING `TestDogStatsdSink_CounterAndGaugePrefixJoin`-style scenario with `NewDogStatsdSink(addr, "dsdpfx", 0)` ⇒ `read(2)` returns TWO single-line datagrams (unchanged from the phase-49 assertion) — a regression guard that the degenerate zero-cap case needs no special code path.
  - **empty batch with a cap set:** `s.Submit(nil)` with `maxBytesPerDatagram > 0` ⇒ no datagram written, no panic (the trailing `flush` on an empty buffer is a no-op).

- [ ] **Step 3: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestDogStatsdSink' -count=1` ⇒ FAIL (the new tests fail on the current unconditional-one-line-per-datagram `Submit`; the 13 call-site updates make the package COMPILE but the OLD behavior doesn't batch).

- [ ] **Step 4: Implement** the rewrite in `dogstatsd.go` (per the Orientation section's pinned code block above — `maxBytesPerDatagram uint64` field; the third `NewDogStatsdSink` parameter; `Submit`'s per-line `s.write(line)` call site replaced by `s.appendLine(&buf, line)` inside a `var buf strings.Builder` declared once at the top of `Submit`, with a trailing `s.flush(&buf)` after the family loop; the NEW `appendLine`/`flush` methods). Update the file's package-level doc-comment (`:18-41`) to describe the batching behavior (ADR-0267) alongside the UNCHANGED delta/tag/line-formatting description.

- [ ] **Step 5: Run to verify they pass + full-package race** — `go test ./internal/statssink/ -run 'TestDogStatsdSink' -count=1` ⇒ PASS; then `go test ./internal/statssink/ -race -count=1` (FULL package — the `Flusher` ticker remains a background mutator) ⇒ PASS.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/dogstatsd.go internal/statssink/dogstatsd_test.go internal/statssink/registration_test.go
git commit -m "phase 50 Task 3: DogStatsdSink.appendLine/flush -- buffer-accumulate-then-flush-on-overflow rewrite of Submit's per-line write call (STRICT > comparison, live-proven inclusive boundary; oversized-alone + absent-cap fall out with zero special-casing; ADR-0267)"
```

---

## Task 4: Boot wiring — thread `MaxBytesPerDatagram` into the `NewDogStatsdSink` call (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `bs.DogStatsdSinkConfigs[i].MaxBytesPerDatagram` (Task 2); the Task-3 THREE-arg `NewDogStatsdSink`.

- [ ] **Step 1: Thread the third argument** at the dog_statsd build loop (`:221-227`):
```go
		for _, cfg := range bs.DogStatsdSinkConfigs {
			sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram)
			if err != nil {
				log.Fatalf("statssink: dog_statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
```
Update the preceding comment (`:217-220`) to mention the batching cap is now threaded through.

- [ ] **Step 2: Verify byte-stability + build.** No dog_statsd-batching unit test in `main.go` (exercised end-to-end by `0094`). Confirm the no-cap / no-sink paths are untouched:
```bash
go build ./... && echo BUILD_OK
go vet ./cmd/... && gofmt -l cmd/envoy-go/main.go
golangci-lint run ./cmd/...
```

- [ ] **Step 3: Commit**
```bash
git add cmd/envoy-go/main.go
git commit -m "phase 50 Task 4: main.go -- thread DogStatsdSinkConfig.MaxBytesPerDatagram into the NewDogStatsdSink call (ADR-0267)"
```

---

## Task 5: `test/helpers/statsdrecv` additive accessors — `MaxLinesInAnyDatagram()` + `LinesInDatagram(name)` [TDD; NO parser change]

**Files:**
- Modify: `test/helpers/statsdrecv/statsdrecv.go`, `test/helpers/statsdrecv/statsdrecv_test.go`

**Interfaces:**
- Produces: `func (s *Server) MaxLinesInAnyDatagram() int`; `func (s *Server) LinesInDatagram(name string) (int, bool)`. (Task 6's `0094` driver consumes both.)

Per SPEC §1.1 AMEND-DSDB-RECEIVER-NO-CHANGE-NEEDED, `ingest`'s EXISTING `strings.Split(strings.TrimSpace(string(b)), "\n")` (`:131`) already correctly ingests multi-line datagrams — **do NOT touch that line or the per-line parsing logic that follows it.** This task ONLY adds two small last-seen/running-max accumulators, mirroring the file's EXISTING `tags map[string]map[string]string` / `Tags(name)` idiom (`:49`/`:189-194`).

- [ ] **Step 1: Write the failing tests** in `statsdrecv_test.go` (ADD; do not remove existing tests):
  - **`MaxLinesInAnyDatagram` starts at 0** on a fresh `Server` (no datagram ingested yet).
  - **a single-line datagram leaves `MaxLinesInAnyDatagram() == 1`** after one `"p.a:1|c"` datagram.
  - **a multi-line datagram (send `"p.a:1|c\np.b:2|c"` as ONE UDP write) updates `MaxLinesInAnyDatagram() == 2`**, and does NOT regress if a subsequent SINGLE-line datagram arrives (the max is a running max, never decreases).
  - **`LinesInDatagram(name)` reflects the datagram that MOST RECENTLY carried name** — send a single-line datagram for `"p.a"`, assert `LinesInDatagram("p.a") == (1, true)`; then send a TWO-line datagram containing `"p.a"` and `"p.b"` together, assert `LinesInDatagram("p.a") == (2, true)` (updated to the latest datagram's line count) and `LinesInDatagram("p.b") == (2, true)`.
  - **an absent name** ⇒ `LinesInDatagram(...)` returns `(0, false)`.
  - **`Reset()` clears both** — after `Reset()`, `MaxLinesInAnyDatagram() == 0` and `LinesInDatagram(name)` for any previously-seen name returns `(0, false)`.
  - **regression: all EXISTING tests (DeltaSum/Gauge/SeenCount/Tags/DeltaSumTagged) still pass unchanged** — confirm by running the full existing test file, not just the new cases.

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ FAIL (`MaxLinesInAnyDatagram`/`LinesInDatagram` undefined).

- [ ] **Step 3: Implement** in `statsdrecv.go`:
  - Add two fields to `Server` (beside `deltaSums`/`sumsByTags`/`gauges`/`seen`/`tags`):
```go
	maxLinesInDatagram int            // Server-WIDE: the largest nlines observed in any ingested datagram
	linesInDatagram    map[string]int // last-seen per name: how many total lines were in the datagram that most recently carried this name
```
  - Initialize `linesInDatagram: make(map[string]int)` in `NewAtAddr`; reassign it (and reset `maxLinesInDatagram = 0`) in `Reset`.
  - In `ingest`, AFTER computing `lines := strings.Split(strings.TrimSpace(string(b)), "\n")` (UNCHANGED) and inside the existing per-line loop (still under the existing mutex), for each successfully-parsed line's `name`:
```go
		if n := len(lines); n > s.maxLinesInDatagram {
			s.maxLinesInDatagram = n
		}
		s.linesInDatagram[name] = len(lines) // overwritten each time — last-seen, the Tags(name) precedent
```
  (Place this update wherever `name` becomes available in the existing loop body — alongside the existing `s.tags[name] = lineTags` assignment is the natural spot; it applies to BOTH counter and gauge lines, unconditionally, unlike the tag update which is conditional on `lineTags != nil`.)
  - Add the accessors (mirroring `Tags`/`SeenCount`'s shape):
```go
// MaxLinesInAnyDatagram returns the largest number of lines ever observed in a
// single ingested datagram (Server-wide, not per-name) — the batching-occurred
// proof for a max_bytes_per_datagram differential (phase 50, ADR-0267).
func (s *Server) MaxLinesInAnyDatagram() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxLinesInDatagram
}

// LinesInDatagram returns how many total lines were in the datagram that MOST
// RECENTLY carried name, and ok=false if name was never seen. Lets a
// differential assert a specific line stayed ALONE (== 1) even when other
// lines in the same flush co-batched (phase 50, ADR-0267).
func (s *Server) LinesInDatagram(name string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.linesInDatagram[name]
	return n, ok
}
```

- [ ] **Step 4: Run to verify they pass + race** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ PASS (INCLUDING every pre-existing test); `go test ./test/helpers/statsdrecv/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/statsdrecv/ && golangci-lint run ./test/helpers/statsdrecv/... && go vet ./test/helpers/statsdrecv/... && go build ./...
git add test/helpers/statsdrecv/
git commit -m "phase 50 Task 5: test/helpers/statsdrecv -- additive MaxLinesInAnyDatagram/LinesInDatagram accessors (NO parser change -- ingest's existing datagram-level \n-split already handles multi-line datagrams; ADR-0267, D-DSDB-RECEIVER-NO-CHANGE-NEEDED)"
```

---

## Task 6: The `0094-stats-sink-dogstatsd-batching` differential fixture (driver + YAMLs + expectations + README) + register in the runner

**Files:**
- Create: `test/fixtures/0094-stats-sink-dogstatsd-batching/driver/driver.go`, `.../envoy.yaml`, `.../envoy-go.yaml`, `.../expectations.yaml`, `.../README.md`
- Modify: `test/differential/runner_test.go` (blank-import the `0094` driver)

**Interfaces:**
- Consumes: `statsdrecv.{NewAtAddr,DeltaSumTagged,Gauge,Tags,MaxLinesInAnyDatagram,LinesInDatagram,Addr,Close}` (Task 5); the batching-aware `NewDogStatsdSink`/parse arm (Tasks 2-4); the `fixture.{Driver,BackendKindAware,StatsAsserter}` interfaces (the `0093` driver shows the exact surface, `test/fixtures/0093-stats-sink-dogstatsd/driver/driver.go`, 713 LoC — READ IT FIRST, this task clones it).

Clone `test/fixtures/0093-stats-sink-dogstatsd/driver/driver.go` verbatim, then apply these diffs:

- [ ] **Step 1: `driver.go` constants — compute the EXACT byte math for D-DSDB-CAP before locking values in:**
```go
const (
	fixtureName = "0094-stats-sink-dogstatsd-batching"

	refAdminPort    = 9901
	refListenerPort = 10094 // fixture 0094 takes 10094 per the "100NN" convention

	numReq = 7

	probePath = "/probe"
	probeHost = "dogstatsd.example"
	probeUA   = "dogstatsd-probe/1"

	statPrefix = "hcm_local" // SHORT — the 0093 value, unchanged, so the HCM-tagged lines stay small enough to co-batch

	// backendName is DELIBERATELY LONG (~160 chars) so the envoy.cluster_name-tagged
	// lines (cluster.upstream_rq_total, cluster.membership_total) exceed the
	// configured cap ALONE, while the envoy.http_conn_manager_prefix-tagged lines
	// (tagged with the SHORT statPrefix) stay small and naturally co-batch with
	// each other and the many other short envoy-go/reference self-stat lines.
	// D-DSDB-CAP (SPEC-50 §8.1): compute the ACTUAL rendered line lengths (e.g. a
	// throwaway t.Logf(len(...)) during this step) before finalizing — if the
	// hand-estimate is off, adjust backendName's length, NOT the cap, to preserve
	// the "HCM lines batch, cluster lines don't" design intent.
	backendName = "very_long_backend_cluster_name_deliberately_chosen_to_force_the_envoy_cluster_name_tagged_dogstatsd_lines_past_the_configured_max_bytes_per_datagram_cap_for_this_fixture_xx"

	// The dog_statsd metric prefix baked identically on both sides. DISTINCT from
	// 0093's "dsdpfx".
	prefix = "dsdbpfx"

	// maxBytesPerDatagram: the configured cap (D-DSDB-CAP). Large enough to batch
	// short HCM-tagged lines together; comfortably smaller than the long-cluster-
	// name-tagged lines, which are therefore ALWAYS sent alone.
	maxBytesPerDatagram = 200

	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)
```
`subsetNames`/`subsetTags`/`gaugeName`/`gaugeTags` are IDENTICAL in SHAPE to `0093`'s (same three counter names + the gauge, re-keyed to the NEW `prefix`/`backendName`/`statPrefix` constants) — copy verbatim with the constant substitution.

- [ ] **Step 2: `sideSnapshot`/`driveSide`/`pollSubset`/`subsetConverged`/`awaitFurtherFlushes`/`describeSubset`/`fireProbe`/`mustAllocateUDPPort`/`mustStartReceiver`/`ensure`/`closeServers`/`ReferenceListenerPort`/`SubjectListenerName`/`BackendCount`/`BackendKind`/`ProbeAdmin`/`fixtureDir`/`mustReadFixtureFile`/`mustRender` — copy VERBATIM from `0093`, unchanged (same receiver type, same lifecycle, same poll/barrier discipline; only the constant VALUES differ, from Step 1).

- [ ] **Step 3: `ReferenceBootstrap`/`SubjectConfig` — add the `MaxBytesPerDatagram` template key:**
```go
func (d *statsdDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	gwIP, err := hostGatewayIP(context.Background())
	if err != nil {
		panic(fmt.Sprintf("driver: hostGatewayIP: %v", err))
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":           refAdminPort,
		"ListenerPort":        refListenerPort,
		"BackendHost":         "host.docker.internal",
		"BackendPort":         backendPorts[0],
		"DogStatsdHost":       gwIP,
		"DogStatsdPort":       d.refStatsdPort,
		"Prefix":              prefix,
		"StatPrefix":          statPrefix,
		"BackendName":         backendName,
		"MaxBytesPerDatagram": maxBytesPerDatagram,
	})
}

func (d *statsdDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":           subjAdminPort,
		"ListenerPort":        subjListenerPort,
		"BackendPort":         backendPorts[0],
		"DogStatsdHost":       "127.0.0.1",
		"DogStatsdPort":       d.subjStatsdPort,
		"Prefix":              prefix,
		"StatPrefix":          statPrefix,
		"BackendName":         backendName,
		"MaxBytesPerDatagram": maxBytesPerDatagram,
	})
}
```
(The `hostGatewayIP` LOCAL duplicate — copy `0093`'s exact function VERBATIM, including its full import list, per the Global Constraints note. Do NOT attempt `differential.HostGatewayIP`.)

- [ ] **Step 4: `AssertStats`/`assertSide` — reuse `0093`'s value/tag-correctness checks verbatim, ADD the two batching-specific proofs:**
```go
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	// ... UNCHANGED from 0093: the subsetNames delta-SUM + tag-set loop, the
	// gauge == 1 + tag-set check ...

	// NEW (phase 50, ADR-0267): batching-specific proofs.
	if snap.maxLinesInDatagram <= 1 {
		t.Fatalf("%s: MaxLinesInAnyDatagram() = %d, want > 1 (no batching observed)", side, snap.maxLinesInDatagram)
	}
	clusterTaggedName := prefix + ".cluster.upstream_rq_total"
	if n, ok := snap.linesInDatagram[clusterTaggedName]; !ok || n != 1 {
		t.Fatalf("%s: LinesInDatagram(%q) = (%d, %v), want (1, true) (the long-cluster-name-tagged line must stay alone)", side, clusterTaggedName, n, ok)
	}
}
```
Extend `sideSnapshot` with `maxLinesInDatagram int` and `linesInDatagram map[string]int` (populated in `driveSide` from `srv.MaxLinesInAnyDatagram()` and `srv.LinesInDatagram(clusterTaggedName)` after the stability barrier, alongside the existing `sums`/`tags`/`gaugeVal` capture).

- [ ] **Step 5: Write `envoy.yaml`** (clone `0093`'s; add `max_bytes_per_datagram: {{.MaxBytesPerDatagram}}` to the `dog_statsd` `typed_config` block, alongside `prefix: {{.Prefix}}`). Update the header comment to describe the batching behavior + the design (long `backendName` → guaranteed-oversized cluster-tagged lines; short `statPrefix` → co-batching HCM-tagged lines).

- [ ] **Step 6: Write `envoy-go.yaml`** (the subject template — SAME shape, `{{.MaxBytesPerDatagram}}` templated identically).

- [ ] **Step 7: Register the driver** in `runner_test.go` (after the `0093` blank-import):
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0094-stats-sink-dogstatsd-batching/driver"
```

- [ ] **Step 8: Run the differential** (live Docker — the controller's host). `reference_differential_run_selector` — NEVER bare `0094`:
```bash
go test ./test/differential/ -run 'TestDifferential/0094' -count=1 -v
```
Expected: PASS; the `-v` output shows the per-side delta-SUMs == 7, the tag sets matching `subsetTags`, the gauge == 1, `MaxLinesInAnyDatagram() > 1`, and `LinesInDatagram(<cluster-tagged-name>) == 1` on BOTH sides.

- [ ] **Step 9: Deliberate breaks** (`reference_differential_break_protocol_count1` — `-count=1` EVERY break; revert after each; verify the main repo is clean + branch undetached):
  - **(a) force one-line-per-datagram regardless of cap** — in `dogstatsd.go`'s `appendLine`, temporarily change the body to unconditionally `s.flush(buf); buf.WriteString(line); return` (skip the size check entirely — every line always flushes alone). Run `-run 'TestDifferential/0094' -count=1` ⇒ FAIL (`MaxLinesInAnyDatagram()` never exceeds 1). Proves assertion (b) is live. REVERT.
  - **(b) silently drop an overflow line instead of flushing-then-appending** — temporarily change the overflow branch of `appendLine` to `s.flush(buf); return` (drop `line` entirely, never write it into the fresh buffer). Run `-run 'TestDifferential/0094' -count=1` ⇒ FAIL (the subset counter whose line happens to land on an overflow boundary undercounts below K — the delta-SUM assertion fails). Proves the value-correctness assertion is live under the NEW batching code path specifically. REVERT.

- [ ] **Step 10: Flake-stability + full-package race**:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0094' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FAIL"; done   # expect 20/20 PASS
go test ./internal/statssink/ -race -count=1   # full package
```
(If a run shows `subject ready: EOF`, isolate-re-run per `reference_differential_fullsuite_startup_flake`.)

- [ ] **Step 11: Write `expectations.yaml` + `README.md`** (clone `0093`'s, adapt the prose to the batching design: the long-`backendName` technique, the `maxBytesPerDatagram` cap, the two batching-specific proofs, the reused value/tag-correctness proofs, the UNasserted set carried from `0093` — the whole line set / family count, non-deterministic gauges, per-flush datagram COUNT beyond "at least one multi-line," literal tag order, `|ms` timers).

- [ ] **Step 12: Per-task gates + commit**
```bash
gofmt -l test/fixtures/0094-stats-sink-dogstatsd-batching/ test/differential/runner_test.go && golangci-lint run ./test/... && go vet ./test/... && go build ./...
git add test/fixtures/0094-stats-sink-dogstatsd-batching/ test/differential/runner_test.go
git commit -m "phase 50 Task 6: 0094-stats-sink-dogstatsd-batching differential -- clones 0093's delta-SUM/tag-set/gauge subset verbatim + a long-backendName-forced oversized-cluster-line proof + a co-batched-HCM-line proof; breaks (a)(b) + 20/20 flake + full-package race (ADR-0267, D-DSDB-CAP)"
```

---

## Task 7: The +0 stat-surface guard (D-DSDB-STATS-FINAL) + the full differential + the six-gate

**Files:** none new — this task PROVES the surface is unchanged + the suite is green (the `NewDogStatsdSink` call-site fix already landed at Task 3).

- [ ] **Step 1: Confirm the +0 surface.** `TestNoNewStat_DogStatsdRegistrationGuard` (fixed at Task 3) MUST PASS UNCHANGED (a transport-layer change registers no new stat):
```bash
go test ./internal/statssink/ -run 'TestNoNewStat_DogStatsdRegistrationGuard' -count=1
go test ./internal/bootstrap/ ./internal/stats/ -count=1   # surface tests PASS, count unchanged 1200 / non-H2 1196
```

- [ ] **Step 2: The full differential** (live Docker; 96 dirs — `reference_differential_fullsuite_startup_flake`: a transient `subject ready: EOF` on an UNRELATED dir is a startup race — isolate-re-run that dir, then re-run full):
```bash
go test ./test/differential/ -count=1 2>&1 | tail -30   # expect ok (all 96 fixtures incl. 0093 UNAFFECTED + 0094)
```

- [ ] **Step 3: The six-gate** (the project's full pre-merge suite):
```bash
gofmt -l $(git diff --name-only master -- '*.go')     # empty
golangci-lint run ./...                               # clean
go vet ./...                                          # clean
go build ./...                                        # BUILD_OK
go test ./... -count=1                                # ALL PASS
go mod tidy -diff                                     # EMPTY (no new module)
```

- [ ] **Step 4: Commit** (verification-only unless Step 1 needed an unexpected fix):
```bash
git commit --allow-empty -m "phase 50 Task 7: confirm +0 stat surface (1200 / non-H2 1196 unchanged) -- a transport-layer change registers no new stat (D-DSDB-STATS-FINAL); full 96-dir differential + six-gate green"
```

---

## Task 8: ADR-0267 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + the fuzzer-count reconcile

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0267 §Decision + §Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/PROGRESS-50.md`

- [ ] **Step 1: ADR-0267 body** — append §Decision + §Consequences to the ADR-0267 §Context already drafted in SPEC-50 §12 (copy the §Context into DECISIONS.md if not already there, then add): §Decision — delete the `bootstrap.go:591-593` strict-reject + add `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` (absent/explicit-0 both parse to 0, no pointer); the `internal/statssink/dogstatsd.go` `appendLine`/`flush` buffer-accumulate-then-flush-on-overflow rewrite of `Submit`'s per-line write call (STRICT `>` boundary, live-proven inclusive at the cap; oversized-alone + absent-cap require zero special-casing); the `main.go` third-argument thread; the additive `test/helpers/statsdrecv` `MaxLinesInAnyDatagram`/`LinesInDatagram` accessors (NO parser change — the datagram-level `\n`-split already existed); `0094` (reuses `0093`'s value/tag-correctness subset verbatim + two new batching proofs, via a deliberately long `backendName`). §Consequences — a transport-layer-only change, +0 stat surface, ZERO new packages/modules, NO new fuzzer (the existing `FuzzDogStatsdSinkConfigParse` seed already covers the field); the family STAYS OPEN (`graphite`/OTLP-metrics/plain-statsd `tcp_cluster_name`/tracing extras/tap filter remain deferred).

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — extend the `### Stats sinks — the dog_statsd UDP sink with tags` subsection (landed at phase 49) per SPEC §9: an EXPLICIT `max_bytes_per_datagram` is now HONORED — envoy-go accumulates consecutive formatted DogStatsd lines (registry-walk order, never reordered/deduped) into a growing buffer and flushes it as one UDP datagram whenever the NEXT line would make the buffer STRICTLY exceed the cap (a buffer landing EXACTLY at the cap still fits); a line whose own length exceeds the cap is sent alone, no error/drop/truncation. An ABSENT field (or explicit `0`) continues to emit exactly one line per datagram, byte-identical to phase 49. Stat surface stays 1200 (+0).

- [ ] **Step 3: STATE.md** — roll the active-phase header to `phase 50 (stats-sink-dogstatsd-batching) IMPL done` (row 50 `done`); update the counts to stat **1200** / fixtures **96** / fuzzers **52** / BackendKind **38** / DECISIONS **ADR-0267** (next-free ADR-0268); set NEXT to "none chartered" (the loop re-opens via the router / next BRAINSTORM, OR the termination sentinel if this was the LAST chartered work — re-verify against the FULL ROADMAP.md before choosing).

- [ ] **Step 4: ROADMAP.md** — flip row 50 (`stats-sink-dogstatsd-batching`) to **`done`** (the sole leg — ADR-0106; no parent rollup); keep the Observability family note OPEN (remaining deferred candidates: `graphite`/OTLP-metrics sinks/the plain-statsd `tcp_cluster_name` transport/tracing extras/the tap filter).

- [ ] **Step 5: PROGRESS-50.md** — mark all tasks complete; record the FINAL counts (re-run the Task-1 baseline commands and paste): fixtures **96**, fuzzers **52** (UNCHANGED — confirm via `grep -rh '^func Fuzz' --include='*.go' . | wc -l`), stat **1200**, BackendKind **38**, `go mod tidy -diff` EMPTY.

- [ ] **Step 6: Final full suite on the frozen HEAD + commit**:
```bash
go build ./... && go test ./... -count=1 && grep -rh '^func Fuzz' --include='*.go' . | wc -l   # ALL PASS; fuzzers == 52
git add docs/envoy-go/
git commit -m "phase 50 Task 8: ADR-0267 §Decision/§Consequences + BEHAVIOR_CONTRACT dog_statsd-batching extension + STATE/ROADMAP (row 50 done) + PROGRESS close + fuzzer-count reconcile (52 unchanged) -- dog_statsd max_bytes_per_datagram batching COMPLETE"
```

- [ ] **Step 7: Check whether ALL chartered ROADMAP work is now complete.** Read the FULL `docs/envoy-go/ROADMAP.md` — if EVERY row is `done` and no new phase is chartered, the router's next-prompt.txt should roll to the TERMINATION SENTINEL shape (mirroring the `418ccf0e` precedent — "roll the router to TERMINAL") rather than a "next BRAINSTORM" placeholder; if the Observability family (or any other) has an obvious cheap next candidate a human should pick, the router stays in the "none chartered, awaiting a human pick" shape instead (the phase-49→50 precedent, `7e593d84`/`418ccf0e`). Do NOT invent a new phase to charter — that decision is the router's job at the NEXT session, not this IMPL's.

---

## Self-Review (run before declaring the plan ready)

- **Spec coverage:** SPEC §3 (parse-arm relaxation + config field + Submit rewrite + main wiring) → Tasks 2/3/4; §3.3 (the exact `appendLine`/`flush` algorithm) → Task 3; §5 (proto roster, unchanged) → Task 2; §6 (reject roster + fuzzer, D-DSDB-FUZZER-SEED) → Task 2 Step 5; §7 (+0 surface) → Task 7; §8 (`0094` + breaks + `statsdrecv` extension + BackendKind 38) → Tasks 5/6; §9 (behavior contract) → Task 8; §10 D-DSDB-* pins → honored verbatim in Task 3 (the boundary operator) and Task 6 (the oversized-line/join-order design); §11 D-DSDB-* PLAN questions → resolved in the D-question block above; §12 (ADR-0267) → Task 8. All covered.
- **The 13-call-site breaking-change gotcha:** caught during this PLAN's own authoring (before any test code was written against a stale two-arg signature) — Task 3 Step 1 explicitly enumerates and fixes ALL 13 existing `NewDogStatsdSink` call sites (11 in `dogstatsd_test.go`, 1 in `registration_test.go`, 1 in `main.go` deferred to Task 4) BEFORE any new batching test is written, so the package always compiles at each intermediate step.
- **The `hostGatewayIP` import-cycle gotcha (carried forward, not re-litigated):** Task 6 reuses the `0093` driver's LOCAL duplicate verbatim — the SAME avoidance `0092`/`0093` already solved.
- **Placeholder scan:** no vague placeholders — the `0093` driver (713 LoC, fully read) is the concrete template for every Task-6 diff; the exact registration-test name (Task 7) is the SAME test phase 49 already wrote, located and cited by name, not assumed.
- **Type consistency:** `DogStatsdSinkConfig{UDPAddress, Prefix, MaxBytesPerDatagram}` (Task 2) ↔ `NewDogStatsdSink(udpAddr, prefix, maxBytesPerDatagram)` (Task 3) ↔ `main.go` call (Task 4); `statsdrecv.{MaxLinesInAnyDatagram, LinesInDatagram}` (Task 5) ↔ the `0094` driver (Task 6). Consistent.
- **D-DSDB-CAP is a COMPUTED value, not an assumed one:** Task 6 Step 1 explicitly instructs computing the ACTUAL rendered line lengths before finalizing `backendName`'s exact length — the PLAN's ~160-char estimate is a starting point, not a hardcoded requirement, avoiding the risk of a subtly-wrong hand-computed byte count silently producing a fixture that doesn't actually exercise the oversized-line path.

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/50-stats-sink-dogstatsd-batching/PLAN-50.md`.** Per the router (and `feedback_execution_style` + `feedback_git_worktrees`), the phase-50 IMPL is subagent-driven in a FRESH worktree off master; subagents commit locally only; the controller verifies each commit + re-runs the full suite on the frozen HEAD + does the deliberate-break verification ITSELF + squashes + pushes at stage-close. The next router stage (after this PLAN lands + a `plan-document-reviewer` pass) is the phase-50 IMPL.
