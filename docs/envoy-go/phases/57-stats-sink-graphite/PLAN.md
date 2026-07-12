# Phase 57 Implementation Plan — `graphite_statsd` stats sink: a NEW `internal/statssink/graphite.go` `GraphiteStatsdSink` (the FOURTH `stats_sinks[]` consumer; tags folded into the metric NAME as `;k=v` via the ~10-LoC `graphiteTagSuffix`; delta/UDP/batching cores REUSED — the batching pair GENERALIZED to shared `appendBatchLine`/`flushBatch` free functions, D-GR-BATCHSHARE) + a `parseGraphiteStatsdSinkConfig` dispatch arm (the FIRST `envoy.extensions.…` stat-sink type URL; sibling-reject extended to FOUR sinks; NEW explicit-`max_bytes_per_datagram: 0` reject) + `FuzzGraphiteStatsdSinkConfigParse` (53 → 54) + the additive `statsdrecv` `;k=v`-in-name extension + the `0101-stats-sink-graphite` merged tags+batching differential (fixtures 102 → 103) — a SINGLE FLAT ROW; ANCHORS ADR-0275; row 57 flips `done` at this IMPL six-gate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended, per `feedback_execution_style`) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Execution lessons: the global CLAUDE.md makes dispatched subagents AUTO-COMMIT (`feedback_subagent_autocommit_claudemd`) — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF (Task 9), and squashes + pushes at stage-close. Every task brief must pin the CANONICAL WORKTREE ROOT + worktree-relative paths (`feedback_subagent_worktree_path_targeting`) and carry the GIT-HYGIENE block (`git restore` only — no `checkout <sha>`, no `commit --amend`; re-verify the branch is undetached after each task, `feedback_subagent_worktree_detach`).

**Goal:** A bootstrap `stats_sinks[]` entry carrying an `envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink` typed_config boots a `GraphiteStatsdSink`: every `stats_flush_interval` it walks the frozen-registry snapshot, applies a FOURTH sink-private per-flush counter-delta transform, formats graphite-flavored statsd lines `<prefix>.<residual>[;key=value;…]:<value>|<c|g>` (tags embedded in the metric NAME — CONTRAST dog_statsd's trailing `|#k:v` suffix; keys dotted `envoy.<tag>`, `ExtractTags` natural order, tag-free lines carry NO `;`), packs lines into datagrams under `max_bytes_per_datagram` with the phase-50 semantics (strict-`>` prospective overflow, `\n`-separated never terminated, oversized-sent-alone untruncated, absent ⇒ one line per datagram), and writes each datagram over a THIRD independent connected UDP socket. Parse rejects: missing `statsd_specifier`, explicit `max_bytes_per_datagram: 0` (BOTH reference-parity, SPEC-57 §11 A4a/A4b), plus the sibling/unknown-type default arm now naming FOUR sinks. Proven cross-side by `0101-stats-sink-graphite` (the merged `0093` tags + `0094` batching shape over an additively-extended `statsdrecv`). **ANCHORS ADR-0275** (§Decision/§Consequences land atomically here per ADR-0044); ROADMAP row 57 flips **`done`** at this IMPL six-gate (the SOLE leg — ADR-0106, no parent rollup); the Observability family STAYS OPEN.

**Architecture:** ONE new production file (`internal/statssink/graphite.go`, ~60 LoC) whose only novel logic is the ~10-LoC `graphiteTagSuffix`; everything else is the landed substrate consumed verbatim: `deltaState` (delta.go), `udpWriter` + `emitStatsdLines` (udp.go), `stats.ExtractTags`, and the phase-50 batching pair — which Task 3 GENERALIZES from `DogStatsdSink` methods into package-level `appendBatchLine`/`flushBatch` free functions in `udp.go` (D-GR-BATCHSHARE: one tested implementation; the dog_statsd behavioral tests exercise batching only through `Submit`, so they stay BYTE-UNTOUCHED — the bias condition holds, verified at PLAN time via `grep -n 'appendLine' internal/statssink/*_test.go` → zero call sites). Bootstrap gains a descriptor-derived type-URL var, a fourth dispatch case, `parseGraphiteStatsdSinkConfig` (reusing `parseUDPSinkAddressAndPrefix`), and `GraphiteStatsdSinkConfig`; `main.go` gains a fourth build loop + gate clause. The `statsdrecv` receiver gains an ADDITIVE `;k=v`-in-name parse block feeding the SAME tag machinery the `|#` path feeds (`stats.IsValidName`'s charset `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` excludes `;`/`=`, so pre-57 line shapes are unaffected — verified at `internal/stats/registry.go:49`). Byte-identical when no graphite sink is configured; regression anchor = the full 103-dir differential (0092/0093/0094/0098 unaffected).

**Tech Stack:** Go 1.23.0, module `github.com/pgdad/envoy-go` (NOTE: PLAN-50's "esalaine" module-path line was stale even then — re-derived from go.mod this PLAN). Proto: `github.com/envoyproxy/go-control-plane/envoy/extensions/stat_sinks/graphite_statsd/v3` — resolves at the ALREADY-PRESENT core `envoy@v1.32.4` module (AMEND-GR-IMAGE; verified in the module cache at PLAN time: `GetAddress() *v3.Address` / `GetPrefix() string` / `GetMaxBytesPerDatagram() *wrapperspb.UInt64Value`, oneof `statsd_specifier` with the SOLE `GraphiteStatsdSink_Address` arm at `graphite_statsd.pb.go:112-115`). **ZERO new go.mod modules, ZERO new packages.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1201 → 1201** (+0, D-GR-STATS); fixtures **102 → 103** (`0101-stats-sink-graphite`; tail was `0100-http-tap-bodies`); fuzzers **53 → 54** (`FuzzGraphiteStatsdSinkConfigParse`; reconcile per `reference_fuzzer_count_docs_drift`); BackendKind **38 → 38** (`H2GoawayResponder` stays the tail — the UDP receiver is driver-owned, `reference_differential_grpc_receiver_driver_owned`); DECISIONS tail **ADR-0274 → ADR-0275** (next-free ADR-0276); **+0 go.mod modules, +0 packages** (`go mod tidy -diff` EMPTY at every task).
- **Process anchors:** ADR-0044 (ADR §Decision/§Consequences at IMPL) · ADR-0045 (escape-valve UNCONSUMED — D-GR-SPLIT re-checked below: 11 tasks, margin 4) · ADR-0080 (strict-reject anti-silent-divergence) · ADR-0106 (row 57 flips `done` here, sole leg) · ADR-0265/0266/0267/0272 (the sink substrate this row consumes) · ADR-0275 (this row — ANCHORED at Task 11).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit, every code task.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on touched packages + `go vet` + `go build ./...`.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0101'`, NEVER bare `'0101'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break run AND every `-race` run uses `-count=1`; after each break, CONFIRM WHICH assertion fired from the failure output (`reference_deliberate_break_wrong_assertion`); `Errorf` per independent property in unit tests (`reference_fatalf_makes_assertions_unreachable` — `Fatalf` only for broken preconditions).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): `GraphiteStatsdSink` is synchronous (no new goroutine), but the `Flusher` ticker and the phase-55 TCP sink's writer goroutine are background mutators — the `-race` gate MUST run the FULL `internal/statssink` package.
- **Boundary operator (LOAD-BEARING):** the overflow comparison is `prospective > cap` (STRICT — exact-fit co-batches; live-proven for graphite at SPEC-57 §11 A3: 12 datagrams at EXACTLY len=160, zero above). Never `>=`.
- **Tag order:** per-side NATURAL `ExtractTags` order, UNSORTED (`reference_dogstatsd_tag_order_unsorted`); cross-side tag assertions SET-based ONLY (`maps.Equal`) — the reference's two-tag wire order is the REVERSE of envoy-go's (AMEND-GR-TAGORDER). Never assert an ordered tag string cross-side.
- **Receivers:** TWO per-side driver-owned `statsdrecv` receivers, live before either proxy boots, hard `Close()` at teardown (`reference_periodic_sink_differential_two_receivers`); the reference reaches its receiver at the LITERAL host-gateway IP via the driver-LOCAL `hostGatewayIP` duplicate (`reference_host_gateway_ip_docker_desktop`; import-cycle: `runner_test.go` blank-imports drivers from within `package differential` — do NOT import `differential.HostGatewayIP`).
- **Stability barrier:** poll `DeltaSumTagged == K`, then `awaitFurtherFlushes ≥ 2`, then assert STILL K (`reference_delta_sink_differential_stability_barrier`); named-subset assertions only (`reference_stats_sink_emits_used_only`); `DeltaSumTagged`/tag-matched lookups against the admin-HCM wire-name collision (`reference_admin_interface_wire_name_collision`); gauge = `membership_total`, NOT `membership_healthy` (no-HC cluster, `reference_membership_total_vs_healthy_gauge`); payload aggregated across datagrams — framing never asserted cross-side (`reference_streaming_sink_differential_framing`).
- **Coupled fixture constants** (`reference_fixture_workload_constant_desync`): `numReq = 7`, the ~172-char `backendName`, and `maxBytesPerDatagram = 200` are COUPLED — named `const`s in ONE block, byte-math re-computed from REAL production code at Task 8 (never hand-trusted).
- **Exact-code names excluded** (AMEND-GR-EXACTCODE-SUBSET): the differential subset must avoid `_NNN` exact-code counter names (the reference strips the code into a tag; envoy-go has no such rule) — the `_xx` class-collapsed names ARE cross-side-compatible.

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding the FOURTH stats sink over a fully-landed substrate. The ONLY novel production logic is `graphiteTagSuffix` (~10 LoC): graphite folds tags INTO the metric name (`name;k=v;k2=v2:value|c`), so the tags ride `emitStatsdLines`'s NAME argument and the trailing-suffix argument stays `""`. Everything else — delta transform, UDP writer, line emission skeleton, batching, receiver, differential shape — exists and is tested. **No new package, no new module, no new BackendKind.**

**What ALREADY works (verified at PLAN time 2026-07-11 against master `f0eb23a6`; re-confirm line numbers before editing — files evolve):**

- **`internal/statssink/udp.go`** — `udpWriter` (`:18-25`: connected `*net.UDPConn` + `sinkLabel` + rate-limited `write` + idempotent `Close`); `emitStatsdLines(batch, nameAndTags func(fam) (name, tagSuffix string), emit func(line string))` (`:58-80`) — builds `name + ":" + strconv.FormatInt(int64(v), 10) + suffix + tagSuffix` per metric, `|c` COUNTER / `|g` GAUGE, skips other types (no histograms, ADR-0060).
- **`internal/statssink/delta.go`** — `newDeltaState()` / `(*deltaState).apply` (`:39-60`): rebuilds COUNTER families with per-flush deltas keyed by full dotted name, latches `last`, passes GAUGE through by pointer, NEVER mutates the input (the Flusher fans ONE batch slice to every sink — `flusher.go:46-51`).
- **`internal/statssink/dogstatsd.go`** — the batching pair to GENERALIZE at Task 3: `appendLine` (`:122-135` — empty-buffer-always-accepts; `prospective := uint64(buf.Len()) + 1 + uint64(len(line))`; strict `>`; flush-then-accept on overflow) + `flush` (`:138-144`). `formatTagSuffix` (`:151-167`) is the key-form precedent: `"envoy." + strings.TrimPrefix(l.Key, "envoy_")`. Constructor shape at `:66-81` (`ResolveUDPAddr` + `DialUDP`, wrapped errors → `main.go` `log.Fatalf`). **No test file calls `appendLine`/`flush` directly** (verified: `grep -n 'appendLine\|\.flush(' internal/statssink/*_test.go` → only a prose comment at `dogstatsd_test.go:367` and the TCP sink's own unrelated `flush` at `statsd_tcp_test.go:737`) — the generalization leaves every dog_statsd test byte-untouched.
- **`internal/stats/name.go`** — `ExtractTags(internal) (residual string, labels []Label, err error)`; SN1 appends `envoy_cluster_name` FIRST (`:51-60`), SN4 prepends the status-class label — the natural order the sink emits. `internal/stats/registry.go:49` — `NamePattern = ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (no `;`, `=`, `:`, `|` — the receiver-extension additivity guarantee).
- **`internal/bootstrap/bootstrap.go`** — type-URL precedent `:217-238` (`metricsServiceTypeURL`/`statsdSinkTypeURL`/`dogStatsdSinkTypeURL`, each descriptor-derived + a `Test*_TypeURLConstant` equality test); `parseStatsSinks` dispatch switch `:505-520` with the THREE-sink default-arm message at `:518-519` (to be rewritten naming FOUR); `parseUDPSinkAddressAndPrefix` `:645-654` (nil-`socket_address` reject interpolating `(sinkLabel, specifierLabel)`, `""→"envoy"` prefix default, `net.JoinHostPort`); `parseDogStatsdSinkConfig` `:666-681` (the parse-arm shape; NOTE `:678` consumes `GetMaxBytesPerDatagram().GetValue()` UNCHECKED — the dog_statsd explicit-0 parity gap, OUT OF SCOPE, deferred per SPEC §2); `DogStatsdSinkConfig` `:314-318` (the config-struct shape); the STALE doc comment at `:415-423` ("an EXPLICITLY set max_bytes_per_datagram … rejected at parse time" — outdated phase-49 wording, corrected in passing at Task 2).
- **`cmd/envoy-go/main.go`** — the three-clause sink gate `:197`; the dog_statsd build loop `:246-252` (the fourth loop's template); `NewFlusher` wiring `:253` (UNTOUCHED); the sink-kinds comment `:183-186` (extend to name four).
- **`test/helpers/statsdrecv/statsdrecv.go`** — `ingestLine` (`:257-316`): first-`|`-then-last-`:` split; `head := line[:pipe1]`; `name := head[:colon]`; the `|#` tag block at `:281-291` feeding `lineTags`; the `c`/`g` switch feeding `deltaSums`/`sumsByTags[tagSignature]`/`gauges`/`seen`; unknown type → `unparsed++`. `ingest` (`:322-336`) splits datagrams on `\n` and records `maxLinesInDatagram`/`linesInDatagram`. Accessors: `DeltaSumTagged`/`Tags`/`Gauge`/`SeenCount`/`MaxLinesInAnyDatagram`/`LinesInDatagram`/`UnparsedCount`/`Reset`/`Close`.
- **`test/fixtures/0094-stats-sink-dogstatsd-batching/driver/driver.go`** (727 LoC) — **the `0101` driver TEMPLATE, READ IT FIRST.** It already merges the `0093` tags shape with the batching proofs: two private receivers + `ensure()` + `mustAllocateUDPPort`/`mustStartReceiver`, `driveSide` (K requests → `pollSubset` → `awaitFurtherFlushes(2)` → snapshot), `DeltaSumTagged` subset + `maps.Equal` tag sets + `membership_total` gauge + `MaxLinesInAnyDatagram() > 1` + `LinesInDatagram(clusterTaggedName) == 1`, the LOCAL `hostGatewayIP`, `ProbeAdmin`, template render helpers. `0101` re-keys constants, swaps the YAML sink block to graphite, and ADDS the subject-side `UnparsedCount() == 0` assertion.
- **`test/differential/runner_test.go:120-121`** — the blank-import registry (0093/0094 entries; 0101's goes after the current tail).
- **`internal/statssink/registration_test.go:73-99`** — `TestNoNewStat_DogStatsdRegistrationGuard`, the +0-surface guard template.
- **`internal/bootstrap/dogstatsd_fuzz_test.go`** — the fuzzer template (seed corpus through `Load`, no-panic contract).
- **`internal/bootstrap/bootstrap_test.go`** — sibling-reject table rows `sibling_unknown_sink` (`:1878`) and `sibling_unknown_typeurl` (`:2101`): both use a REAL-but-unhandled `envoy.config.metrics.v3.HystrixSink` (`reference_sibling_reject_test_needs_real_typeurl` — a fabricated URL fails at protojson Any-resolution BEFORE `parseStatsSinks`) with `errSubs: ["bootstrap:", "metrics_service", "statsd", "dog_statsd"]`. The message extension does NOT break them (substring match) but Task 2 EXTENDS both to also assert `"graphite_statsd"` and updates their "ALL THREE" comments. `TestDogStatsdSink_TypeURLConstant` (`:2326`) is the type-URL equality-test template.

**The graphite line shape (SPEC-57 §3.4, live-pinned §11 — PLAN adopts verbatim):**

```
<prefix>.<residual>;<key1>=<val1>;<key2>=<val2>:<delta>|c    (COUNTER — per-flush delta; zero-delta lines EMITTED every flush)
<prefix>.<residual>;<key1>=<val1>:<abs>|g                    (GAUGE — absolute)
<prefix>.<residual>:<value>|c                                (tag-free — NO ';' anywhere)
```

---

## D-question resolutions (SPEC-57 §12 — settled here)

**D-GR-BATCHSHARE → GENERALIZE.** Hoist the phase-50 batching pair out of `DogStatsdSink` methods into two package-level free functions in `udp.go` (the shared-helpers home, beside `emitStatsdLines`): `appendBatchLine(buf *strings.Builder, line string, maxBytesPerDatagram uint64, write func(string))` + `flushBatch(buf *strings.Builder, write func(string))`. `DogStatsdSink.Submit`'s two call sites delegate (`appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)` / `flushBatch(&buf, s.write)`); the `appendLine`/`flush` METHODS are deleted with their doc comments moved to the free functions. The SPEC's bias condition — "generalize IF the dog_statsd tests stay untouched" — HOLDS: no test calls the methods directly (verified at PLAN time, see Orientation); the six phase-50 batching tests exercise the algorithm through `Submit` and stay byte-untouched, doubling as the refactor's regression proof. One tested implementation; graphite calls the same functions (Task 4). Rejected alternative: duplicating ~22 LoC in `graphite.go` — two copies of a strict-`>` boundary invite silent divergence.

**D-GR-SPLIT → NO sub-split (a SINGLE FLAT ROW, 11 tasks).** Real-LoC estimate: `graphite.go` ~60 prod LoC (mostly the constructor + Submit shells; novel logic ~10), parse arm ~45, batching hoist net ~0 (moved, not added), `main.go` ~10, `statsdrecv` extension ~20, fuzzer ~85 (test), `graphite_test.go` ~450 (test), driver ~730 (a `0094` re-key). 11 tasks (SPEC §10 anticipated ~9–12) — margin 4 under the ADR-0045 `~15` ceiling; the escape-valve stays UNCONSUMED. Re-confirmed at Task 1 against the real baseline.

**Break-(b) firing precision → the SPEC's break-(b) shape is VACUOUS; REDESIGNED (a genuine PLAN-time catch per `feedback_brief_citations_not_evidence`).** SPEC §8.1 sketched break (b) as "emit dog_statsd's `|#k:v` suffix instead of `;k=v` ⇒ the tagged lookup misses AND subject UnparsedCount/name-miss fires." Re-derived against `statsdrecv.ingestLine` (`:257-316`): a dog_statsd-formatted line parses CLEANLY through the EXISTING `|#` path into the SAME `(name, tags)` buckets — `grpfx.cluster.upstream_rq_total:7|c|#envoy.cluster_name:X` yields the identical stripped name, identical tag map, identical `DeltaSumTagged` bucket as the graphite form. NO assertion fires; the break would prove nothing (worse: a green "break" run would be misread as assertion liveness). Redesigned roster (Task 9), each break isolating ONE assertion:
- **(b1) tag-drop break:** `graphiteTagSuffix` temporarily returns `""` ⇒ every subset line lands in the TAGLESS `tagSignature("")` bucket ⇒ `DeltaSumTagged(name, wantTags)` never converges ⇒ `pollSubset` timeout whose `describeSubset` diagnostic shows `ok=false` for all three names. Proves the tag-embedded lookup (and thus the `;k=v` wire format the receiver extension parses) is load-bearing.
- **(b2) UnparsedCount isolating break:** the graphite `Submit` additionally emits ONE well-formed-name/unknown-type line per flush (`s.prefix + ".break57:1|q"`) ⇒ the subset still converges, the barrier passes, every value/tag/gauge/batching assertion passes, and the SUBJECT-side `UnparsedCount() == 0` assertion fires ALONE — the isolating break `reference_deliberate_break_wrong_assertion` demands (in break (b1) the poll timeout masks it).
The receiver extension's missing-`=`-pair unparsed arm is proven live by its own unit test (Task 7), not by a differential break.

**Stale-comment placement (SPEC §11 said "the IMPL's docs task"):** the stale `bootstrap.go:415-423` dog_statsd doc comment is corrected at **Task 2** (the task already editing `bootstrap.go` — atomic with the surrounding change) rather than the Task-11 docs bundle; a doc-comment is code-file content, and a docs task editing production `.go` files would violate the task-boundary discipline. Recorded as a deliberate micro-deviation from SPEC §11's placement prose (the SPEC's intent — the comment gets fixed this row — is honored).

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/statssink/graphite.go` — `GraphiteStatsdSink` + `NewGraphiteStatsdSink` + `Submit` + `graphiteTagSuffix`.

**Production (modified):**
- `internal/statssink/udp.go` — add `appendBatchLine`/`flushBatch` free functions (+ `strings` import).
- `internal/statssink/dogstatsd.go` — `Submit` delegates to the free functions; the `appendLine`/`flush` methods deleted (doc comments move with the code).
- `internal/bootstrap/bootstrap.go` — `graphiteStatsdSinkTypeURL` var + import; `GraphiteStatsdSinkConfig` struct; `GraphiteStatsdSinkConfigs` field on `Bootstrap`; fourth dispatch case; `parseGraphiteStatsdSinkConfig`; the FOUR-sink default-arm message; the stale `:415-423` comment fix.
- `cmd/envoy-go/main.go` — fourth gate clause + fourth build loop.
- `test/helpers/statsdrecv/statsdrecv.go` — the additive `;k=v`-in-name block in `ingestLine`.

**Test (created):**
- `internal/statssink/graphite_test.go`; `internal/bootstrap/graphite_fuzz_test.go`; `test/fixtures/0101-stats-sink-graphite/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.

**Test (modified):**
- `internal/bootstrap/bootstrap_test.go` (graphite parse tests + the two sibling-reject rows extended); `internal/statssink/registration_test.go` (`TestNoNewStat_GraphiteRegistrationGuard`); `test/helpers/statsdrecv/statsdrecv_test.go` (extension tests); `test/differential/runner_test.go` (blank-import).

**Docs (Task 11):**
- `docs/envoy-go/DECISIONS.md` (ADR-0275 full entry), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (row 57 `done`; family deferred-list edit), `docs/envoy-go/phases/57-stats-sink-graphite/PROGRESS.md` (scaffolded at the PLAN session — Task 1 fills baselines, Task 11 closes it), `next-prompt.txt` (the router roll, controller-owned).

---

## Task 1: Baselines into the existing PROGRESS.md + the final ADR-0045 re-check

**Files:**
- Modify: `docs/envoy-go/phases/57-stats-sink-graphite/PROGRESS.md` (scaffold already committed by the PLAN session)

- [ ] **Step 1: Record the baseline counts** (verbatim outputs pasted into PROGRESS.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                    # expect 102 (tail 0100-http-tap-bodies)
grep -rn '^func Fuzz' --include='*.go' . | wc -l                  # expect 53
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go  # the BackendKind tail (38)
go mod tidy -diff                                                 # expect EMPTY
grep -c 'GraphiteStatsdSink' internal/bootstrap/bootstrap.go      # expect 0 (nothing landed yet)
```
Baseline: stat surface **1201** / fixtures **102** / fuzzers **53** / BackendKind **38** / DECISIONS tail **ADR-0274** (next-free **ADR-0275**).

- [ ] **Step 2: Confirm D-GR-SPLIT** in PROGRESS.md (NO sub-split; 11 tasks; the LoC table from the D-question block above; escape-valve UNCONSUMED) and the anticipated exit counts (stat **1201** +0 / fixtures **103** / fuzzers **54** / BackendKind **38** / DECISIONS **ADR-0275**).

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/57-stats-sink-graphite/PROGRESS.md
git commit -m "phase 57 Task 1: baselines into PROGRESS + ADR-0045 NO-sub-split re-check (graphite_statsd sink; ANCHORS ADR-0275; row 57 flips done at this IMPL)"
```

---

## Task 2: The graphite parse arm — `graphiteStatsdSinkTypeURL` + dispatch case + `parseGraphiteStatsdSinkConfig` + `GraphiteStatsdSinkConfig` + the FOUR-sink sibling-reject + the explicit-zero reject [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`, `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Produces: `GraphiteStatsdSinkConfig{UDPAddress string; Prefix string; MaxBytesPerDatagram uint64}`; `Bootstrap.GraphiteStatsdSinkConfigs []GraphiteStatsdSinkConfig` (Task 5's `main.go` loop reads it); the exported-to-tests type URL `type.googleapis.com/envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink`.

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (mirror the dog_statsd test group's shape; define a local `graphiteStatsdSinkType = "type.googleapis.com/envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink"` const beside the existing `metricsServiceType`-style consts):
  - **`TestGraphiteStatsdSink_TypeURLConstant`** (the `:2326` template): `graphiteStatsdSinkTypeURL == graphiteStatsdSinkType` (descriptor-derived matches the SPEC-57 §11 live-verified literal).
  - **accept, full config:** `address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }` + `prefix: gpfx` + `max_bytes_per_datagram: 512` ⇒ `Load` succeeds; `bs.GraphiteStatsdSinkConfigs[0] == {UDPAddress: "127.0.0.1:8125", Prefix: "gpfx", MaxBytesPerDatagram: 512}`.
  - **accept, defaults:** no `prefix`, no `max_bytes_per_datagram` ⇒ `{Prefix: "envoy", MaxBytesPerDatagram: 0}` (absent-cap = one line per datagram — NOT rejected; only an EXPLICIT 0 is).
  - **accept, `protocol: TCP` ignored** (the dog_statsd posture — dial UDP regardless).
  - **accept, hostname address** (`address: localhost`) — the DOCUMENTED phase-48/49 DEPARTURE from the reference's `malformed IP address` boot-reject (SPEC-57 §11 A4c); assert `UDPAddress == "localhost:8125"`.
  - **accept, IPv6 bracketed** (the `TestDogStatsdSink_IPv6AddressBracketed` `:2706` sibling): `address: "::1"` ⇒ `UDPAddress == "[::1]:8125"` (`net.JoinHostPort`).
  - **reject, missing `statsd_specifier`:** typed_config with only `prefix: x` ⇒ error contains `"bootstrap:"`, `"graphite_statsd"`, `"statsd_specifier"` (REFERENCE-PARITY, §11 A4a — via `parseUDPSinkAddressAndPrefix`'s interpolated labels).
  - **reject, explicit `max_bytes_per_datagram: 0`:** ⇒ error contains `"graphite_statsd max_bytes_per_datagram must be greater than 0"` (REFERENCE-PARITY, §11 A4b — the `*wrapperspb.UInt64Value` makes absent-vs-explicit-0 distinguishable; a DISTINCT substring per ADR-0080).
  - **EXTEND the two sibling-reject rows** (`sibling_unknown_sink` `:1878`, `sibling_unknown_typeurl` `:2101`): add `"graphite_statsd"` to both `errSubs` slices; update both "ALL THREE" comments to "ALL FOUR". `HystrixSink` remains a valid unknown sibling (still no dispatch arm).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/bootstrap/ -run 'TestGraphiteStatsdSink|TestParseStatsSinks|TestStatsSinks' -count=1` ⇒ FAIL (type/var/arm undefined; sibling rows fail on the missing `graphite_statsd` substring).

- [ ] **Step 3: Implement** in `bootstrap.go`:
  - Import: `graphitestatsdv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/stat_sinks/graphite_statsd/v3"` (a REAL import — the parse arm unmarshals into the typed message, registering the descriptor).
  - The type-URL var (after `dogStatsdSinkTypeURL` `:238`):
```go
// graphiteStatsdSinkTypeURL is the typed_config TypeURL for the graphite_statsd
// UDP stats sink (envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink)
// — the FIRST `envoy.extensions.…` typed-extension stat-sink type URL (its three
// siblings are inline envoy.config.metrics.v3 types). DERIVED from the proto
// descriptor (the metricsServiceTypeURL precedent —
// reference_network_filter_typeurl_extensions). The message resolves at the CORE
// go-control-plane/envoy module (graphite is a CORE extension — SPEC-57
// AMEND-GR-IMAGE; zero new go.mod modules). A test asserts it equals the
// SPEC-57 §11 live-verified string.
var graphiteStatsdSinkTypeURL = "type.googleapis.com/" + string((&graphitestatsdv3.GraphiteStatsdSink{}).ProtoReflect().Descriptor().FullName())
```
  - The config struct (after `DogStatsdSinkConfig` `:318`):
```go
// GraphiteStatsdSinkConfig is the parsed graphite_statsd UDP stats-sink config
// from one top-level stats_sinks[] GraphiteStatsdSink entry (ADR-0275). The sink
// is constructed in cmd/envoy-go/main.go after Load returns. Tags are folded
// into the metric NAME as ;k=v pairs at emit time (the sink's concern, not
// parse's). A missing statsd_specifier / nil socket_address is a
// REFERENCE-PARITY reject; an EXPLICIT max_bytes_per_datagram: 0 is a
// REFERENCE-PARITY reject (the PGV gt:0 rule — the wrapper type distinguishes
// absent from explicit 0); an ABSENT max_bytes_per_datagram parses to 0 =
// one line per datagram. socket_address.protocol is accepted-and-IGNORED;
// prefix defaults to "envoy" when empty.
type GraphiteStatsdSinkConfig struct {
	UDPAddress          string // socket_address host:port (net.ResolveUDPAddr-resolvable)
	Prefix              string // GraphiteStatsdSink.prefix, default "envoy" when empty
	MaxBytesPerDatagram uint64 // 0 (absent only — explicit 0 is parse-rejected) = one line per datagram; >0 batches up to the cap
}
```
  - `Bootstrap` gains `GraphiteStatsdSinkConfigs []GraphiteStatsdSinkConfig` (after `DogStatsdSinkConfigs` `:423`, with a matching doc comment: parsed graphite entries in declaration order; built in `main.go` only when non-empty).
  - The dispatch case (in the `:505-520` switch, after the `dogStatsdSinkTypeURL` case) + the FOUR-sink default arm:
```go
		case graphiteStatsdSinkTypeURL:
			if err := parseGraphiteStatsdSinkConfig(tc, i, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports the metrics_service sink %q, the statsd sink %q, the dog_statsd sink %q, and the graphite_statsd sink %q)", i, tc.GetTypeUrl(), metricsServiceTypeURL, statsdSinkTypeURL, dogStatsdSinkTypeURL, graphiteStatsdSinkTypeURL)
```
  - The parse function (after `parseDogStatsdSinkConfig` `:681`):
```go
// parseGraphiteStatsdSinkConfig parses one graphite_statsd UDP stats sink
// typed_config and appends a GraphiteStatsdSinkConfig (ADR-0275). Like
// DogStatsdSink, the statsd_specifier oneof has ONLY the address member
// (graphite_statsd.pb.go: GraphiteStatsdSink_Address is the sole arm — no
// tcp_cluster_name sibling, UDP-only by construction). Rejects (ADR-0080,
// both REFERENCE-PARITY, SPEC-57 §11 A4a/A4b): a missing statsd_specifier /
// nil socket_address (via parseUDPSinkAddressAndPrefix); an EXPLICIT
// max_bytes_per_datagram: 0 (the PGV gt:0 rule — non-nil wrapper with value
// 0; an ABSENT wrapper parses to 0 = one-line-per-datagram, NOT rejected).
// NOTE the landed dog_statsd parse does NOT enforce its identical PGV rule
// (bootstrap.go parseDogStatsdSinkConfig consumes GetValue() unchecked) —
// a pre-existing phase-50 parity gap, deferred (SPEC-57 §2), NOT fixed here.
func parseGraphiteStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var g graphitestatsdv3.GraphiteStatsdSink
	if err := tc.UnmarshalTo(&g); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: graphite_statsd typed_config: %w", idx, err)
	}
	addr, prefix, err := parseUDPSinkAddressAndPrefix(g.GetAddress().GetSocketAddress(), g.GetPrefix(), "graphite_statsd", "statsd_specifier", idx)
	if err != nil {
		return err
	}
	if w := g.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: graphite_statsd max_bytes_per_datagram must be greater than 0", idx)
	}
	result.GraphiteStatsdSinkConfigs = append(result.GraphiteStatsdSinkConfigs, GraphiteStatsdSinkConfig{
		UDPAddress:          addr,
		Prefix:              prefix,
		MaxBytesPerDatagram: g.GetMaxBytesPerDatagram().GetValue(),
	})
	return nil
}
```
  - Update `parseStatsSinks`'s doc comment (`:483-489`) to name four sinks; fix the STALE `DogStatsdSinkConfigs` field comment (`:415-423`): replace "an EXPLICITLY set max_bytes_per_datagram and a missing dog_statsd_specifier/socket_address are rejected at parse time" with "a missing dog_statsd_specifier/socket_address is rejected at parse time; max_bytes_per_datagram is HONORED (ADR-0267; 0/absent = one metric per datagram)".

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/bootstrap/ -count=1` ⇒ ALL PASS (full package — proves no other test asserted the three-sink message). `go mod tidy -diff` ⇒ EMPTY (the proto is in the already-present core module).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 57 Task 2: graphite_statsd parse arm -- descriptor-derived envoy.extensions type URL (the FIRST typed-extension stat sink), parseUDPSinkAddressAndPrefix reuse, NEW explicit-zero max_bytes_per_datagram reject (PGV parity), FOUR-sink sibling-reject, stale dog_statsd comment fixed (ADR-0275)"
```

---

## Task 3: D-GR-BATCHSHARE — hoist `appendLine`/`flush` into shared `appendBatchLine`/`flushBatch` (`internal/statssink/udp.go`); dog_statsd tests stay BYTE-UNTOUCHED [refactor; proof = the untouched phase-50 tests]

**Files:**
- Modify: `internal/statssink/udp.go`, `internal/statssink/dogstatsd.go`

**Interfaces:**
- Produces: `func appendBatchLine(buf *strings.Builder, line string, maxBytesPerDatagram uint64, write func(string))`; `func flushBatch(buf *strings.Builder, write func(string))` (Task 4's graphite `Submit` consumes both).

- [ ] **Step 1: Pre-refactor proof** — `go test ./internal/statssink/ -run 'TestDogStatsdSink' -count=1` ⇒ PASS (the baseline the refactor must preserve). Confirm zero direct test call sites: `grep -n 'appendLine\|\.flush(&' internal/statssink/*_test.go` ⇒ no hits.

- [ ] **Step 2: Implement the hoist.** In `udp.go` (add `"strings"` to the imports), after `emitStatsdLines`:
```go
// appendBatchLine accumulates line into buf, flushing buf as its own datagram
// FIRST (via flushBatch → write) if appending would make it STRICTLY exceed
// maxBytesPerDatagram (the comparison is `>`, NOT `>=` — a buffer that lands
// EXACTLY at the cap after appending still fits, live-proven for dog_statsd at
// AMEND-DSDB-BOUNDARY and re-proven for graphite at SPEC-57 §11 A3). When buf
// is EMPTY, the line is ALWAYS accepted unconditionally — even if the line
// alone exceeds the cap — which is what makes an oversized single line "sent
// alone" fall out of the SAME general algorithm with NO special-cased branch:
// on the NEXT call, buf.Len() already exceeds the cap, so ANY next line's
// prospective size trivially exceeds it too, forcing a flush of the oversized
// line alone; if the oversized line is the LAST in the batch, Submit's
// trailing flushBatch sends it. maxBytesPerDatagram == 0 (absent field) needs
// NO special case either: every append past the first exceeds a cap of 0, so
// every line flushes alone — the one-line-per-datagram behavior exactly.
// Hoisted VERBATIM from the phase-50 DogStatsdSink methods (ADR-0267) so
// DogStatsdSink and GraphiteStatsdSink share ONE tested implementation
// (D-GR-BATCHSHARE, ADR-0275); the phase-50 dog_statsd batching tests are the
// regression anchor, byte-untouched by the hoist.
func appendBatchLine(buf *strings.Builder, line string, maxBytesPerDatagram uint64, write func(string)) {
	if buf.Len() == 0 {
		buf.WriteString(line)
		return
	}
	prospective := uint64(buf.Len()) + 1 + uint64(len(line)) // +1 for the "\n" separator
	if prospective > maxBytesPerDatagram {
		flushBatch(buf, write)
		buf.WriteString(line)
		return
	}
	buf.WriteByte('\n')
	buf.WriteString(line)
}

// flushBatch writes buf's contents as ONE datagram via write (if non-empty)
// and resets it.
func flushBatch(buf *strings.Builder, write func(string)) {
	if buf.Len() == 0 {
		return
	}
	write(buf.String()) // the udpWriter write — rate-limit-logged-and-dropped on error
	buf.Reset()
}
```
  In `dogstatsd.go`: DELETE the `appendLine` (`:122-135`) and `flush` (`:138-144`) methods; rewrite `Submit`'s two call sites:
```go
	}, func(line string) {
		appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)
	})
	flushBatch(&buf, s.write)
```
  Trim the now-method-specific parts of the `DogStatsdSink` package doc (the batching paragraph now points at `appendBatchLine`'s doc).

- [ ] **Step 3: Run to verify nothing changed** — `go test ./internal/statssink/ -count=1` ⇒ ALL PASS with `dogstatsd_test.go`/`statsd_test.go`/`registration_test.go` BYTE-UNTOUCHED (`git status` must show ONLY `udp.go` + `dogstatsd.go` modified — the D-GR-BATCHSHARE bias condition, verified mechanically). Then `go test ./internal/statssink/ -race -count=1` (full package) ⇒ PASS.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/udp.go internal/statssink/dogstatsd.go
git commit -m "phase 57 Task 3: hoist the phase-50 batching pair into shared appendBatchLine/flushBatch free functions in udp.go (D-GR-BATCHSHARE: one tested strict-> implementation; dog_statsd tests byte-untouched as the regression proof; ADR-0275)"
```

---

## Task 4: `internal/statssink/graphite.go` — `GraphiteStatsdSink` + `graphiteTagSuffix` + unit tests + the +0 registration guard [TDD]

**Files:**
- Create: `internal/statssink/graphite.go`, `internal/statssink/graphite_test.go`
- Modify: `internal/statssink/registration_test.go`

**Interfaces:**
- Consumes: `udpWriter`/`emitStatsdLines` (udp.go), `newDeltaState` (delta.go), `appendBatchLine`/`flushBatch` (Task 3), `stats.ExtractTags`/`stats.Label`; the shared test helpers `udpListener`/`sameSet` (statsd_test.go) + `snapshot(reg, 0)` (mapping.go).
- Produces: `NewGraphiteStatsdSink(udpAddr string, prefix string, maxBytesPerDatagram uint64) (*GraphiteStatsdSink, error)` (Task 5's `main.go` loop calls it); the wire format Task 8's fixture asserts.

- [ ] **Step 1: Write the failing tests** in `graphite_test.go` (the `dogstatsd_test.go` harness shape — `udpListener(t)`, `snapshot(reg, 0)`, `sameSet`; `Errorf` per independent property):
  - **`TestGraphiteStatsdSink_CounterAndGaugeTagsInName`:** counter `cluster.backend.upstream_rq_total`+=7 and gauge `cluster.backend.membership_total`=1, prefix `grpfx`, cap 0 ⇒ two datagrams, set-equal to `{"grpfx.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c", "grpfx.cluster.membership_total;envoy.cluster_name=backend:1|g"}` (tags IN the name, NO trailing suffix — the load-bearing AMEND-GR-TAGFORMAT literal).
  - **`TestGraphiteStatsdSink_TwoTagNaturalOrder`:** counter `http.hcm_local.downstream_rq_2xx`+=5 ⇒ EXACT line `grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:5|c` (the SN4-prepend natural order — an exact-literal per-side pin; the CROSS-side assertion stays set-based, Task 8).
  - **`TestGraphiteStatsdSink_UntaggedNoSemicolon`:** counter `server.dynamic_unknown_fields`+=0 ⇒ `grpfx.server.dynamic_unknown_fields:0|c` and `!strings.Contains(line, ";")`.
  - **`TestGraphiteStatsdSink_DeltaSemanticsAcrossFlushes`:** +7 → flush ⇒ `…:7|c`; idle flush ⇒ `…:0|c` (zero-delta RE-EMITTED — AMEND-GR-DELTA); +3 → flush ⇒ `…:3|c`.
  - **`TestGraphiteStatsdSink_IndependentDelta`:** one registry, a `GraphiteStatsdSink` AND a `DogStatsdSink` each `Submit`ting the same snapshot twice ⇒ each sink's second flush shows delta 0 independently (a FOURTH sink-private `deltaState`, never shared).
  - **`TestGraphiteStatsdSink_GaugeAbsoluteAcrossFlushes`** + **`TestGraphiteStatsdSink_NegativeGauge`** (the dog_statsd siblings, graphite framing).
  - **Batching through the graphite path** (the shared functions get graphite-side liveness proof): **exact-boundary co-locates** (two short untagged counters, `capExact = len(line1)+1+len(line2)` ⇒ ONE `\n`-joined datagram), **capExact-1 splits** (⇒ TWO single-line datagrams), **oversized-alone untruncated** (a 40-char name at cap 10 + a short sibling ⇒ the oversized line arrives alone, `len` exact), **cap 0 = one line per datagram**.
  - **`TestGraphiteStatsdSink_EmptyBatch`** (`Submit(nil)` with cap set — no datagram, no panic), **`TestGraphiteStatsdSink_CloseIdempotent`**, **`TestGraphiteStatsdSink_ResolveError`** (`"not-an-addr"` ⇒ error mentions `graphite_statsd`).
  - In `registration_test.go`: **`TestNoNewStat_GraphiteRegistrationGuard`** (the `:73-99` dog_statsd template verbatim — `NewGraphiteStatsdSink("127.0.0.1:65535", "envoy", 0)` + a Flusher over a fresh registry ⇒ `countMetrics` stays 0; pins D-GR-STATS +0, surface stays 1201).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestGraphiteStatsdSink|TestNoNewStat_Graphite' -count=1` ⇒ FAIL (type undefined).

- [ ] **Step 3: Implement `graphite.go`** (SPEC-57 §3.2/§3.3 adopted verbatim; the dog_statsd file's doc-comment density):
```go
package statssink

import (
	"fmt"
	"net"
	"strings"

	dto "github.com/prometheus/client_model/go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// GraphiteStatsdSink writes the frozen registry snapshot as graphite-flavored
// statsd UDP datagrams every flush (ADR-0275): one line per metric family,
// <prefix>.<residual>[;key=value;…]:<value>|<c|g> — tags are folded INTO the
// metric NAME as ;k=v pairs (CONTRAST DogStatsdSink's trailing |#k:v suffix;
// live-pinned AMEND-GR-TAGFORMAT, SPEC-57 §11). COUNTER families carry the
// per-flush DELTA over a FOURTH sink-private always-on deltaState (zero-delta
// lines re-emitted every flush — AMEND-GR-DELTA); GAUGE families carry the
// ABSOLUTE value. Keys are the dotted envoy.<tag> form in ExtractTags's
// NATURAL (unsorted) slice order (AMEND-GR-TAGORDER — the reference does not
// sort either; its two-tag order is the REVERSE of ours, a documented
// cross-side coverage boundary). A tag-free name carries NO ';' at all.
// Batching reuses the shared appendBatchLine/flushBatch pair (ADR-0267
// semantics verbatim: strict-> prospective overflow incl. the +1 separator
// byte, \n-separated never terminated, oversized-sent-alone untruncated,
// 0/absent = one line per datagram; an EXPLICIT 0 never reaches here — the
// parse arm rejects it, PGV parity). SYNCHRONOUS (no goroutine): a THIRD
// independent connected *net.UDPConn via the shared udpWriter; envoy-go has
// no histograms (ADR-0060), so only |c/|g lines are produced.
type GraphiteStatsdSink struct {
	udpWriter
	prefix              string
	delta               *deltaState // always non-nil — a FOURTH sink-private instance, never shared
	maxBytesPerDatagram uint64      // 0 = one line per datagram (explicit 0 is parse-rejected upstream)
}

// NewGraphiteStatsdSink resolves udpAddr, dials a connected UDP socket (a THIRD
// independent *net.UDPConn), and returns a ready sink. A resolve/dial error is
// returned verbatim (-> main.go log.Fatalf, the statsd/dog_statsd precedent).
func NewGraphiteStatsdSink(udpAddr string, prefix string, maxBytesPerDatagram uint64) (*GraphiteStatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve graphite_statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial graphite_statsd udp %q: %w", udpAddr, err)
	}
	return &GraphiteStatsdSink{
		udpWriter:           udpWriter{conn: conn, sinkLabel: "graphite_statsd"},
		prefix:              prefix,
		delta:               newDeltaState(),
		maxBytesPerDatagram: maxBytesPerDatagram,
	}, nil
}

// Submit applies the sink-private deltaState, then per family folds the
// ExtractTags residual + graphiteTagSuffix INTO the emitted NAME (the
// tag-suffix return stays "" — graphite's tags precede the ':'), accumulating
// lines into a per-call buffer via the shared batching pair. Called serially
// by the Flusher.
func (s *GraphiteStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	var buf strings.Builder // a PER-CALL buffer — batching never spans across flushes
	emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			// Defensive: can't happen for a registered name — full name, no tags.
			residual, labels = fam.GetName(), nil
		}
		return s.prefix + "." + residual + graphiteTagSuffix(labels), ""
	}, func(line string) {
		appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)
	})
	flushBatch(&buf, s.write)
}

// graphiteTagSuffix renders labels as the graphite name-embedded tag suffix
// ";envoy.k1=v1;envoy.k2=v2" in the slice's NATURAL (unsorted) order — the ';'
// separates the name from the FIRST tag too, and an empty label set returns ""
// (no ';' anywhere — AMEND-GR-TAGFORMAT). Keys take the dotted envoy. form
// (the formatTagSuffix precedent). THE one novel piece of phase 57.
func graphiteTagSuffix(labels []stats.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range labels {
		b.WriteByte(';')
		b.WriteString("envoy.")
		b.WriteString(strings.TrimPrefix(l.Key, "envoy_"))
		b.WriteByte('=')
		b.WriteString(l.Value)
	}
	return b.String()
}
```

- [ ] **Step 4: Run to verify they pass + full-package race** — `go test ./internal/statssink/ -count=1` ⇒ ALL PASS; `go test ./internal/statssink/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/graphite.go internal/statssink/graphite_test.go internal/statssink/registration_test.go
git commit -m "phase 57 Task 4: GraphiteStatsdSink -- tags folded into the metric name via the ~10-LoC graphiteTagSuffix (;k=v, natural order, dotted keys); FOURTH sink-private deltaState; shared batching pair; +0 registration guard (ADR-0275)"
```

---

## Task 5: Boot wiring — the fourth `main.go` build loop + gate clause

**Files:**
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `bs.GraphiteStatsdSinkConfigs` (Task 2); `statssink.NewGraphiteStatsdSink` (Task 4).

- [ ] **Step 1: Implement.** Extend the gate (`:197`) with `|| len(bs.GraphiteStatsdSinkConfigs) > 0`; add the fourth loop after the dog_statsd loop (`:246-252`), before `NewFlusher` (`:253`, untouched):
```go
		// Phase 57 (ADR-0275): the graphite_statsd UDP stats sink — tags folded
		// into the metric NAME as ;k=v pairs; batching per max_bytes_per_datagram
		// (the shared phase-50 machinery). NewGraphiteStatsdSink dials a THIRD
		// independent connected UDP socket; a resolve/dial error is a fatal boot
		// failure (the statsd/dog_statsd precedent). Synchronous (no goroutine),
		// so it adds no background mutator to the shutdown drain.
		for _, cfg := range bs.GraphiteStatsdSinkConfigs {
			sink, err := statssink.NewGraphiteStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram)
			if err != nil {
				log.Fatalf("statssink: graphite_statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
```
Update the sink-kinds comment (`:183-186`) to name all four (metrics_service, statsd, dog_statsd, graphite_statsd).

- [ ] **Step 2: Gates** (no `main.go` unit test — exercised end-to-end by `0101`):
```bash
go build ./... && echo BUILD_OK
go vet ./cmd/... && gofmt -l cmd/envoy-go/main.go && golangci-lint run ./cmd/...
```

- [ ] **Step 3: Commit**
```bash
git add cmd/envoy-go/main.go
git commit -m "phase 57 Task 5: main.go -- fourth stats-sink build loop + gate clause for graphite_statsd (ADR-0275)"
```

---

## Task 6: `FuzzGraphiteStatsdSinkConfigParse` — fuzzers 53 → 54

**Files:**
- Create: `internal/bootstrap/graphite_fuzz_test.go`

- [ ] **Step 1: Write the fuzzer** (the `dogstatsd_fuzz_test.go` template: seed corpus through `Load`, no-panic contract). Seeds: valid accept (address+prefix+`max_bytes_per_datagram: 512`); default prefix; `protocol: TCP` (accepted-ignored); explicit `max_bytes_per_datagram: 0` (reject seed); missing `statsd_specifier` (reject seed); hostname address (accept — the departure); coexisting FOUR sinks (metrics_service + statsd + dog_statsd + graphite_statsd); the degenerate/garbage quartet (`{}`, `\x00\x00\x00`, `stats_sinks: [{}]`, bare-typed_config). The fuzz body: `_, _ = Load(bytes.NewReader(data))`.

- [ ] **Step 2: Run** — `go test ./internal/bootstrap/ -run 'FuzzGraphiteStatsdSinkConfigParse' -count=1` ⇒ PASS (seed-only), then a brief live fuzz `go test ./internal/bootstrap/ -run '^$' -fuzz 'FuzzGraphiteStatsdSinkConfigParse' -fuzztime 30s` ⇒ no crash. Reconcile the count (`reference_fuzzer_count_docs_drift`): `grep -rn '^func Fuzz' --include='*.go' . | wc -l` ⇒ **54**.

- [ ] **Step 3: Gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/graphite_fuzz_test.go
git commit -m "phase 57 Task 6: FuzzGraphiteStatsdSinkConfigParse -- no-panic fuzz over the graphite parse arm incl. explicit-zero + missing-specifier reject seeds (fuzzers 53 -> 54; ADR-0275)"
```

---

## Task 7: `statsdrecv` — the ADDITIVE `;k=v`-in-name graphite-tag extension [TDD]

**Files:**
- Modify: `test/helpers/statsdrecv/statsdrecv.go`, `test/helpers/statsdrecv/statsdrecv_test.go`

**Interfaces:**
- Produces: graphite lines now populate the SAME `Tags()`/`DeltaSumTagged()`/`Gauge()`/`sumsByTags` machinery the `|#` path feeds, keyed by the TAG-STRIPPED name (Task 8's driver consumes them unchanged).

- [ ] **Step 1: Write the failing tests** (ADD; never remove existing tests):
  - **two-tag counter:** ingest `grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm:5|c` ⇒ `DeltaSumTagged("grpfx.http.downstream_rq_xx", {"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": "hcm"}) == (5, true)`; `Tags` on the STRIPPED name returns that map; the UNstripped `;`-bearing name is NOT a key (`DeltaSum(fullLine-name)` ⇒ ok=false).
  - **one-tag gauge:** `grpfx.cluster.membership_total;envoy.cluster_name=b:1|g` ⇒ `Gauge("grpfx.cluster.membership_total") == (1, true)` + `Tags` match.
  - **tag-free graphite line unchanged:** `grpfx.server.uptime:3|g` parses exactly as before (regression).
  - **missing `=` in a pair:** `grpfx.x;badpair:1|c` ⇒ NOT accounted; `UnparsedCount() == 1`; no `deltaSums`/`tags` entry.
  - **delta accumulation across graphite lines:** two ingests of `…;envoy.cluster_name=b:3|c` then `…:2|c` (same tags) ⇒ `DeltaSumTagged == 5`.
  - **dog_statsd regression:** a `|#`-tagged line and a graphite line for DIFFERENT names coexist; both bucket correctly (the two grammars share the machinery).
  - **multi-line datagram with graphite tags:** one two-line datagram ⇒ `LinesInDatagram(<stripped name>) == 2` (batching signals key on the stripped name).
  - **`Reset()` clears** the new state (it lives in the existing maps — this is a confirm-test, not new code).

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ FAIL (the `;`-bearing name is treated as an opaque name today: the tagged lookups miss and `UnparsedCount` stays 0 where the test wants 1).

- [ ] **Step 3: Implement** in `ingestLine` — insert AFTER the `val` parse succeeds (`:274-278`) and BEFORE `rest := line[pipe1+1:]` (`:279`); HOIST the `var lineTags map[string]string` declaration here (deleting the later one at `:281`) and nil-guard the `|#` branch's `make`:
```go
	// Graphite name-embedded tags (phase 57, ADR-0275): "<name>;k1=v1;k2=v2".
	// ADDITIVE by construction: statsd/dog_statsd wire names cannot contain ';'
	// (stats.IsValidName's charset — registry.go NamePattern), so this block is
	// a no-op for every pre-57 line shape (0092/0093/0094/0098 unaffected). The
	// parsed pairs feed the SAME lineTags the '|#' path feeds, so Tags()/
	// DeltaSumTagged()/tagSignature work unchanged on the TAG-STRIPPED name.
	var lineTags map[string]string
	if semi := strings.IndexByte(name, ';'); semi >= 0 {
		tagPart := name[semi+1:]
		name = name[:semi]
		lineTags = make(map[string]string, 2)
		for _, pair := range strings.Split(tagPart, ";") {
			eq := strings.IndexByte(pair, '=')
			if eq < 0 {
				s.unparsed++ // a ';'-bearing name whose pair lacks '=' is unaccountable
				return "", false
			}
			lineTags[pair[:eq]] = pair[eq+1:]
		}
	}
```
  In the `|#` branch (`:282-291`): replace `lineTags = make(map[string]string)` with `if lineTags == nil { lineTags = make(map[string]string) }` (the two grammars never co-occur on a real line, but the parser must not clobber). Update the `Server`/`ingestLine` doc comments to name the graphite grammar (the `reference_line_parser_extension_delimiter_reuse` trace: the FIRST-`|` split is untouched — graphite tags precede the `:`, so `head` = `name;tags:value` and the LAST-`:` split still finds the value colon because tag values never contain `:` — `stats.IsValidName` excludes it envoy-go-side and the reference's tag values are code-classes/cluster names).

- [ ] **Step 4: Run to verify they pass + race** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ ALL PASS (including every pre-existing test); `go test ./test/helpers/statsdrecv/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Gates + commit**
```bash
gofmt -l test/helpers/statsdrecv/ && golangci-lint run ./test/helpers/statsdrecv/... && go vet ./test/helpers/statsdrecv/... && go build ./...
git add test/helpers/statsdrecv/
git commit -m "phase 57 Task 7: statsdrecv -- additive graphite ;k=v name-embedded tag parsing into the existing tag machinery (stripped-name keying; missing-= counts unparsed; statsd/dog_statsd lines structurally unaffected -- IsValidName excludes ';'; ADR-0275)"
```

---

## Task 8: The `0101-stats-sink-graphite` differential (driver + YAMLs + expectations + README) + runner registration + live run

**Files:**
- Create: `test/fixtures/0101-stats-sink-graphite/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go`

**Interfaces:**
- Consumes: the Task-7 receiver; Tasks 2/4/5 production code; `fixture.{Driver,BackendKindAware,StatsAsserter}` — cross-side fixtures dispatch through **StatsAsserter** (`reference_differential_asserter_dispatch`); ONE runner branch per dir (`reference_differential_fixture_dispatch_constraint`); `BackendCount() == 1` (`reference_differential_backendcount_min_one`).

Clone `test/fixtures/0094-stats-sink-dogstatsd-batching/driver/driver.go` (727 LoC — READ IT FIRST) and apply these diffs:

- [ ] **Step 1: Constants + byte math (the COUPLED block — `reference_fixture_workload_constant_desync`):**
```go
const (
	fixtureName = "0101-stats-sink-graphite"

	refAdminPort    = 9901
	refListenerPort = 10101 // the "100NN" convention extended: fixture 0101 → 10101

	numReq = 7 // K — the subset delta-SUMs converge to == 7 per side

	probePath = "/probe"
	probeHost = "graphite.example"
	probeUA   = "graphite-probe/1"

	statPrefix = "hcm_local" // SHORT — keeps the HCM-tagged lines co-batchable under the cap

	// backendName is DELIBERATELY LONG so the envoy.cluster_name-tagged lines
	// exceed maxBytesPerDatagram ALONE (the 0094 technique, graphite framing).
	// COUPLED with numReq and maxBytesPerDatagram — never change one alone.
	backendName = "very_long_backend_cluster_name_deliberately_chosen_to_force_the_envoy_cluster_name_tagged_graphite_lines_past_the_configured_max_bytes_per_datagram_cap_for_this_fixture_gr"

	prefix = "grpfx" // DISTINCT from 0092/0093/0094/0098's prefixes

	maxBytesPerDatagram = 200

	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)
```
  **Compute the EXACT rendered line lengths against the REAL production code** (a throwaway `t.Logf(len(...))` through `graphiteTagSuffix` + `stats.ExtractTags` — never the hand-estimate) before locking `backendName`. PLAN-time estimate (graphite framing, `;envoy.cluster_name=` = 20 bytes of tag overhead): `grpfx.cluster.upstream_rq_total;envoy.cluster_name=<~171 chars>:7|c` ≈ 226 > 200 and `grpfx.cluster.membership_total;…:1|g` ≈ 225 > 200 (always alone); `grpfx.http.downstream_rq_total;envoy.http_conn_manager_prefix=hcm_local:7|c` ≈ 75 and `grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:7|c` ≈ 100 (75+1+100 = 176 ≤ 200 — co-batchable). If the exact math differs, adjust `backendName`'s LENGTH, not the cap.

- [ ] **Step 2: The subset (WIRE names, tag SETS)** — identical in shape to 0094's, graphite keys unchanged (the receiver strips tags back out of the name, so lookup keys are the STRIPPED `<prefix>.<residual>` names):
```go
var subsetNames = []string{
	prefix + ".cluster.upstream_rq_total",
	prefix + ".http.downstream_rq_total",
	prefix + ".http.downstream_rq_xx",
}
var subsetTags = map[string]map[string]string{
	prefix + ".cluster.upstream_rq_total": {"envoy.cluster_name": backendName},
	prefix + ".http.downstream_rq_total":  {"envoy.http_conn_manager_prefix": statPrefix},
	prefix + ".http.downstream_rq_xx":     {"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": statPrefix},
}
var gaugeName = prefix + ".cluster.membership_total"
var gaugeTags = map[string]string{"envoy.cluster_name": backendName}

const clusterTaggedName = prefix + ".cluster.upstream_rq_total"
```
  NO `_NNN` exact-code names anywhere (AMEND-GR-EXACTCODE-SUBSET); `_xx` class names only. `maps.Equal` tag-SET assertions only (AMEND-GR-TAGORDER).

- [ ] **Step 3: Copy VERBATIM from 0094** (rename the driver type `statsdDriver` → `graphiteDriver`): `ensure`/`mustAllocateUDPPort`/`mustStartReceiver`, `BackendCount`(1)/`BackendKind`(`fixture.HTTPFixedBody`)/`SubjectListenerName`("l_test")/`ReferenceListenerPort`, `DriveReference`/`DriveSubject`/`closeServers`, `driveSide` (K probes → `pollSubset` → `awaitFurtherFlushes(…, 2)` → snapshot), `fireProbe`, `pollSubset`/`subsetConverged`/`describeSubset`/`awaitFurtherFlushes`, `ProbeAdmin`, the LOCAL `hostGatewayIP` (full function + imports, verbatim — the import-cycle rule), `fixtureDir`/`mustReadFixtureFile`/`mustRender`, the compile-time interface assertions. Template keys renamed `DogStatsdHost/Port` → `GraphiteHost/Port`.

- [ ] **Step 4: The ONE addition — subject-side `UnparsedCount`:** extend `sideSnapshot` with `unparsed int`; in `driveSide`, after the barrier: `snap.unparsed = srv.UnparsedCount()`. In `AssertStats`, after the two `assertSide` calls:
```go
	// SUBJECT-EXACT (never cross-side): envoy-go emits only |c/|g graphite
	// lines, so a correct subject leaves the receiver with ZERO unaccountable
	// lines. The reference legitimately produces unparsed lines (|ms timers) —
	// its count is NOT asserted (reference_framing_break_needs_unparsed_counter;
	// SPEC-57 §8.1).
	if d.subj.unparsed != 0 {
		t.Errorf("subject: UnparsedCount() = %d, want 0 (every subject graphite line must parse)", d.subj.unparsed)
	}
```
  (`Errorf`, not `Fatalf` — an independent property, `reference_fatalf_makes_assertions_unreachable`.) `assertSide` itself is the 0094 body unchanged: subset delta-SUM == 7 + `maps.Equal` tag sets, gauge == 1 + tag set, `maxLinesInDatagram > 1`, `linesInDatagram[clusterTaggedName] == 1`.

- [ ] **Step 5: `envoy.yaml` / `envoy-go.yaml`** — clone 0094's pair; swap the sink block (both sides; reference uses STRICT_DNS + `host.docker.internal`, subject STATIC + 127.0.0.1, ports templated as in 0094):
```yaml
stats_sinks:
  - name: envoy.stat_sinks.graphite_statsd
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink
      address:
        socket_address: { protocol: UDP, address: {{.GraphiteHost}}, port_value: {{.GraphitePort}} }
      prefix: {{.Prefix}}
      max_bytes_per_datagram: {{.MaxBytesPerDatagram}}
stats_flush_interval: 0.5s
```
  Header comments describe the graphite tag-in-name grammar + the batching design (the 0094 prose re-keyed). NO node field (graphite carries no proxy identifier).

- [ ] **Step 6: Register** in `runner_test.go` (after the current last fixture import):
```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0101-stats-sink-graphite/driver"
```

- [ ] **Step 7: Live run** (Docker; the correct selector — `reference_differential_run_selector`):
```bash
go test ./test/differential/ -run 'TestDifferential/0101' -count=1 -v
```
Expected: PASS — both sides converge to delta-SUM 7 with matching tag SETS, gauge 1, `MaxLinesInAnyDatagram > 1`, the cluster-tagged line alone, subject `UnparsedCount == 0`. If the REFERENCE side fails to parse, check the reference reaches the receiver (fresh container per any probe-arm rerun — `reference_probe_fresh_container_per_arm`).

- [ ] **Step 8: `expectations.yaml` + `README.md`** — clone 0094's prose shape; document: the merged 0093+0094 design (D-GR-FIXTURE: ONE dir, batching folded in — fixtures 102 → 103, NOT → 104); the exact byte-math table from Step 1 (as computed, not estimated); the asserted set (subset delta-SUMs + tag SETS + gauge + two batching proofs + subject-only UnparsedCount); the UNasserted coverage boundaries (per-side tag ORDER — reversed cross-side; the reference's `|ms` lines and its unparsed count; `_NNN` exact-code names; whole-registry-vs-used-only breadth; datagram COUNT/framing — `reference_streaming_sink_differential_framing`).

- [ ] **Step 9: Gates + commit**
```bash
gofmt -l test/fixtures/0101-stats-sink-graphite/ test/differential/runner_test.go && golangci-lint run ./test/... && go vet ./test/... && go build ./...
git add test/fixtures/0101-stats-sink-graphite/ test/differential/runner_test.go
git commit -m "phase 57 Task 8: 0101-stats-sink-graphite differential -- merged 0093-tags + 0094-batching shape over the graphite ;k=v wire grammar; delta-SUM stability barrier, SET-based tags, oversized-alone, subject UnparsedCount==0 (fixtures 102 -> 103; ADR-0275)"
```

---

## Task 9: Deliberate breaks + flake gate + full-package race [CONTROLLER-EXECUTED — never delegated]

**Files:** temporary edits to `internal/statssink/graphite.go` only — every break REVERTED before the next; `git status` clean + branch undetached verified after each.

Every run: `go test ./test/differential/ -run 'TestDifferential/0101' -count=1` (`reference_differential_break_protocol_count1`). After each break, READ the failure output and CONFIRM the named assertion fired (`reference_deliberate_break_wrong_assertion`).

- [ ] **Break (a) — delta:** in `Submit`, comment out `batch = s.delta.apply(batch)` ⇒ counters emit ABSOLUTE values ⇒ FAIL. Expected firing: either `pollSubset` timeout with `describeSubset` showing sums OVERSHOOTING 7 (the poll may never sample the transient ==7), or `assertSide`'s "delta-sum = X, want 7" with X > 7 — BOTH are the delta-family assertion (`reference_delta_sink_differential_stability_barrier`); confirm the output shows an overshot sum, not an unrelated error. REVERT.
- [ ] **Break (b1) — tag-drop:** `graphiteTagSuffix` returns `""` unconditionally ⇒ every line lands in the tagless `tagSignature("")` bucket ⇒ FAIL via `pollSubset` timeout, `describeSubset` showing `ok=false` on all three subset names. Proves the `;k=v` embedding + the Task-7 receiver parsing are load-bearing for the tagged lookups. REVERT.
- [ ] **Break (b2) — UnparsedCount isolation:** in `Submit`, after `flushBatch`, add `s.write(s.prefix + ".break57:1|q")` (a well-formed name, UNKNOWN type) ⇒ the subset converges, the barrier passes, EVERY value/tag/gauge/batching assertion passes, and the run FAILS ONLY on `subject: UnparsedCount() = N, want 0`. This is the isolating break the SPEC §12 asked for — in (b1) the poll timeout masks this assertion. Confirm the failure output names UnparsedCount and NOTHING else. REVERT.
  - **Record the vacuity note** in PROGRESS.md: the SPEC §8.1 break-(b) shape (`|#` suffix instead of `;k=v`) was NOT run — re-derivation at PLAN time proved `statsdrecv` parses a dog_statsd-formatted line into the identical buckets, so that break fires NOTHING (a green run would falsely suggest liveness). (b1)+(b2) replace it.
- [ ] **Break (c1) — never-batch:** in `Submit`'s emit closure, replace `appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)` with `s.write(line)` (one line per datagram regardless of cap) ⇒ FAIL on `MaxLinesInAnyDatagram() = 1, want > 1` (the oversized-alone check still PASSES — isolates the multi-line proof). REVERT.
- [ ] **Break (c2) — infinite cap:** pass `^uint64(0)` as the cap in the `appendBatchLine` call ⇒ every flush packs ALL lines into ONE datagram ⇒ FAIL on `LinesInDatagram(<clusterTaggedName>) = (N, true), want (1, true)` with N > 1 (the multi-line proof still PASSES — isolates the oversized-alone check). REVERT.
- [ ] **Non-break (d) — tag-order robustness:** reverse `graphiteTagSuffix`'s iteration (`for i := len(labels) - 1; i >= 0; i--`) ⇒ the differential MUST STILL PASS (set-based tag assertions are order-insensitive — proving liveness the OTHER way; per-side unit test `TestGraphiteStatsdSink_TwoTagNaturalOrder` FAILS under the same edit, which is fine — run ONLY the 0101 selector for this check). REVERT; re-run `go test ./internal/statssink/ -count=1` to confirm the revert restored the exact-literal unit test.
- [ ] **Flake gate + race:**
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0101' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FAIL"; done   # expect 20/20
go test ./internal/statssink/ -race -count=1      # FULL package (reference_full_suite_race_after_background_mutator)
go test ./test/helpers/statsdrecv/ -race -count=1
```
(A `subject ready: EOF` on an unrelated run is a startup race — isolate-re-run per `reference_differential_fullsuite_startup_flake`.)
- [ ] **Record every break's observed failure line** in PROGRESS.md, then commit:
```bash
git status --porcelain   # MUST be clean of production edits (all breaks reverted)
git add docs/envoy-go/phases/57-stats-sink-graphite/PROGRESS.md
git commit -m "phase 57 Task 9: deliberate breaks (a delta, b1 tag-drop, b2 unparsed-isolating, c1 never-batch, c2 infinite-cap) each confirmed firing its OWN assertion + non-break (d) order-reversal green + 20/20 flake + full-package race; SPEC break-(b) shape recorded VACUOUS"
```

---

## Task 10: +0 stat surface + the full 103-dir differential + the six-gate

**Files:** none new — verification only.

- [ ] **Step 1: Surface** — `go test ./internal/statssink/ -run 'TestNoNewStat' -count=1` (all four guards PASS); `go test ./internal/bootstrap/ ./internal/stats/ -count=1` ⇒ PASS, surface unchanged **1201**.
- [ ] **Step 2: Full differential** (live Docker, 103 dirs — 0092/0093/0094/0098 prove the receiver extension + batching hoist regressed nothing):
```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
- [ ] **Step 3: The six-gate:**
```bash
gofmt -l $(git diff --name-only master -- '*.go')   # empty
golangci-lint run ./...                             # clean
go vet ./...                                        # clean
go build ./...                                      # BUILD_OK
go test ./... -count=1                              # ALL PASS
go mod tidy -diff                                   # EMPTY
```
- [ ] **Step 4: Commit**
```bash
git commit --allow-empty -m "phase 57 Task 10: +0 stat surface (1201) confirmed via four registration guards; full 103-dir differential + six-gate green"
```

---

## Task 11: ADR-0275 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 57 `done`) + PROGRESS close + the router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/57-stats-sink-graphite/PROGRESS.md`, `next-prompt.txt`

- [ ] **Step 1: ADR-0275** — copy the §Context draft from SPEC-57 §13 into DECISIONS.md, then append: **§Decision** — `internal/statssink/graphite.go` `GraphiteStatsdSink` over `udpWriter`/`deltaState`/`emitStatsdLines` with the ~10-LoC `graphiteTagSuffix` folding tags into the NAME argument; the phase-50 batching pair GENERALIZED to shared `appendBatchLine`/`flushBatch` free functions in `udp.go` (D-GR-BATCHSHARE — one tested strict-`>` implementation, dog_statsd tests byte-untouched); `parseGraphiteStatsdSinkConfig` + the descriptor-derived FIRST `envoy.extensions.…` stat-sink type URL + the FOUR-sink sibling-reject + the NEW explicit-`max_bytes_per_datagram: 0` reject (PGV parity; dog_statsd's identical unenforced rule recorded as a deferred parity candidate); the fourth `main.go` loop; the additive `statsdrecv` `;k=v` extension; `0101` (merged 0093+0094 shape; the SPEC's break-(b) found VACUOUS at PLAN and replaced by isolating breaks). **§Consequences** — +0 stat surface, +0 packages/modules, fuzzers 54, fixtures 103; per-side tag ORDER a documented cross-side coverage boundary; the hostname-accepting `address` departure inherited unchanged; the family STAYS OPEN. DECISIONS tail → **ADR-0275** (next-free ADR-0276).
- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — the graphite_statsd subsection per SPEC-57 §9: the line mapping, delta/gauge semantics, tag grammar (natural order, dotted keys, tag-free = no `;`), batching rules, the reject roster (§6), the two departures (hostname-accepting address; whole-registry emission).
- [ ] **Step 3: STATE.md** — active-phase → `phase 57 (stats-sink-graphite) IMPL done` (demote the PLAN line to prior); counts: stat **1201** / fixtures **103** (tail `0101-stats-sink-graphite`) / fuzzers **54** / BackendKind **38** / DECISIONS **ADR-0275** (next-free ADR-0276).
- [ ] **Step 4: ROADMAP.md** — row 57 → **`done`** (sole leg, ADR-0106 — `reference_roadmap_split_phase_row_done` does not apply, this is not a split row); update the Observability family's deferred-candidates sentence: REMOVE graphite, ADD the dog_statsd explicit-`max_bytes_per_datagram: 0` parity candidate (keeping OTLP-metrics + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace) — the sentinel's check (2) keeps matching.
- [ ] **Step 5: PROGRESS.md** — all tasks checked; FINAL counts pasted from re-run baseline commands (fixtures 103, fuzzers **54** via `grep -rn '^func Fuzz' --include='*.go' . | wc -l`, `go mod tidy -diff` EMPTY); the break log complete.
- [ ] **Step 6: Roll `next-prompt.txt`** (TRACKED — edit in the worktree, fold into the squash; locate prior squashes by SUBJECT only, `reference_next_prompt_tracked_despite_gitignore`): the next stage is the next router decision (the Observability family and Operational-tooling remain open; five families never opened — the sentinel does NOT fire; do NOT create `stop`).
- [ ] **Step 7: Final suite on the frozen HEAD + commit**
```bash
go build ./... && go test ./... -count=1 && grep -rn '^func Fuzz' --include='*.go' . | wc -l   # ALL PASS; 54
git add docs/envoy-go/ next-prompt.txt
git commit -m "phase 57 Task 11: ADR-0275 full entry + BEHAVIOR_CONTRACT graphite subsection + STATE/ROADMAP (row 57 done; family deferred-list rolls graphite out, dog_statsd explicit-zero parity in) + PROGRESS close + router roll -- graphite_statsd sink COMPLETE"
```

---

## Self-Review (run at PLAN authoring — issues found were fixed inline)

- **Spec coverage:** §3.1 (parse arm + type URL + sibling extension + config struct) → Task 2; §3.2/§3.3 (sink + the novel `graphiteTagSuffix` + batching reuse) → Tasks 3/4; §3.5 (boot wiring) → Task 5; §3.4/§3.6 (line shape/byte-stability) → Task 4 tests; §5 (proto roster — all three fields consumed) → Task 2; §6 (reject roster + fuzzer) → Tasks 2/6; §7 (+0 surface) → Tasks 4 (guard) + 10; §8 (0101 + receiver extension + breaks + BackendKind 38) → Tasks 7/8/9; §9 (behavior contract) → Task 11; §11 pins → honored verbatim (strict-`>`, natural order, delta re-emission, explicit-zero reject, hostname departure); §12 D-questions → the D-question block (BATCHSHARE generalize; SPLIT no; break-(b) redesigned); §13 (ADR-0275) → Task 11. No gaps.
- **Every SPEC citation RE-DERIVED from source this session** (`feedback_brief_citations_not_evidence`): all `file:line` cites verified against master `f0eb23a6`; TWO stale-claim catches recorded — (1) PLAN-50's module-path line said `esalaine` where go.mod says `pgdad` (quoted nowhere here except as the warning); (2) the SPEC §8.1 break-(b) shape is VACUOUS against the real `ingestLine` (redesigned, documented in the D-question block and Task 9).
- **Placeholder scan:** every code step carries the actual code; the 0094 driver/YAML/expectations (fully read at PLAN time) are the concrete clone base for every Task-8 diff; test names, error substrings, and commands are literal.
- **Type consistency:** `GraphiteStatsdSinkConfig{UDPAddress, Prefix, MaxBytesPerDatagram}` (T2) ↔ `NewGraphiteStatsdSink(udpAddr, prefix, maxBytesPerDatagram)` (T4) ↔ the T5 loop; `appendBatchLine(buf, line, maxBytesPerDatagram, write)`/`flushBatch(buf, write)` (T3) ↔ both sinks' `Submit` (T3/T4); `statsdrecv` stripped-name keying (T7) ↔ the T8 subset lookups. Consistent.
- **Break isolation:** five breaks + one non-break, each mapped to EXACTLY the assertion it fires, with the masking analysis done up front (b2 exists BECAUSE b1's poll timeout masks UnparsedCount; c1/c2 isolate the two batching proofs from each other).
- **Fixture-constant coupling:** `numReq`/`backendName`/`maxBytesPerDatagram` declared in one const block with the coupling comment; byte math recomputed from production code at Task 8, estimate-only here.

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/57-stats-sink-graphite/PLAN.md`.** Per the router + `feedback_execution_style`, the phase-57 IMPL is **subagent-driven** (superpowers:subagent-driven-development) in a FRESH worktree off master (e.g. `.worktrees/phase-57-impl`, branch `phase-57-stats-sink-graphite-impl`); subagents commit locally only; the controller verifies each commit, executes Task 9's breaks ITSELF, re-runs the full suite on the frozen HEAD, and squashes + pushes at stage-close. The next router stage after this PLAN lands is the phase-57 IMPL.
