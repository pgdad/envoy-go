# Phase 49 Implementation Plan — the `dog_statsd` UDP stats sink WITH TAGS: LIFT the `envoy.config.metrics.v3.DogStatsdSink` TypeURL from the `bootstrap.go` two-arm `stats_sinks[]` dispatch into a THREE-arm dispatch + a `parseDogStatsdSinkConfig` arm → a `DogStatsdSinkConfig{UDPAddress, Prefix}` + a NEW `internal/statssink/dogstatsd.go` `DogStatsdSink` (a SECOND, independent `*net.UDPConn` writer + the `<prefix>.<residual-name>:<value>|c`/`|g[|#tags]` line-with-tags mapping over `stats.ExtractTags` in NATURAL/unsorted order, counters ALWAYS-delta over a sink-private `deltaState`, gauges absolute) over the LANDED phase-47/48 `Flusher`/`Sink` substrate + the `cmd/envoy-go/main.go` third build loop + `FuzzDogStatsdSinkConfigParse` + the extended `test/helpers/statsdrecv` UDP receiver (a `Tags()` accessor) + the `0093-stats-sink-dogstatsd` cross-side delta-SUM-with-stability-barrier + tag-set differential — a SINGLE FLAT ROW, UDP-only; the SIXTH Observability-family row; ZERO new packages, ZERO new go.mod modules; ANCHORS ADR-0266; row 49 flips `done` at this IMPL six-gate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** When the bootstrap carries a `dog_statsd` `stats_sinks[]` entry with a UDP `address`, envoy-go dials that UDP address once (`net.DialUDP` — a SECOND, independent `*net.UDPConn`, phase-48-shaped) and every `stats_flush_interval` (default 5s) snapshots the frozen process-global `stats.Registry` (the SAME `snapshot()` the metrics_service/statsd sinks use) and writes one DogStatsd line per metric as a UDP datagram: `<prefix>.<residual-name>:<delta>|c[|#tags]` for each COUNTER family (the per-flush delta over a sink-private always-on `deltaState`, a SECOND independent instance from `StatsdSink`'s), `<prefix>.<residual-name>:<abs>|g[|#tags]` for each GAUGE family (absolute), the residual name + tags from `stats.ExtractTags` (the SAME SN1–SN9+SN4 matcher `label.go`'s `labelMapper` calls) formatted `envoy.<key>:<value>` comma-joined in the SLICE'S NATURAL (unsorted) order — NO `|#` suffix at all when zero tags extract, default prefix `envoy`. Proven cross-side on a deterministic COUNTER name-subset (the delta-SUM + a post-convergence stability barrier + the extracted tag SET) + an absolute-gauge-plus-tag subset against `contrib-v1.37.2` by the `0093` differential through the EXTENDED driver-owned `statsdrecv` UDP receiver. **ANCHORS ADR-0266** (its §Decision/§Consequences body lands atomically here); ROADMAP row 49 (`stats-sink-dogstatsd`) flips **`done`** at this IMPL six-gate (the sole leg — ADR-0106; NO parent rollup); the Observability family STAYS OPEN.

**Architecture:** ONE new `Sink` impl (`internal/statssink/dogstatsd.go`) — a SECOND, independent `*net.UDPConn` writer + the `Counter`/`Gauge` → DogStatsd-line-with-tags mapping over a SECOND, independent sink-private always-on `deltaState` (reusing the landed `delta.go` `newDeltaState()`/`apply()` VERBATIM: counters→delta, gauges→absolute) + a per-family `stats.ExtractTags` call building an inline `|#` tag suffix in NATURAL (unsorted) order — SYNCHRONOUS per-flush `Write` (the phase-48 `StatsdSink` shape, confirmed unchanged at SPEC), with an idempotent `sync.Once`-guarded `Close`. PLUS a NEW bootstrap parse arm: the `dogStatsdSinkTypeURL` constant (descriptor-derived), the EXISTING two-arm dispatch extended to THREE arms, `parseDogStatsdSinkConfig` → a `DogStatsdSinkConfig{UDPAddress, Prefix}` appended to a NEW `result.DogStatsdSinkConfigs` slice, the strict-reject arms (an EXPLICIT `max_bytes_per_datagram` / a missing `dog_statsd_specifier`); the `cmd/envoy-go/main.go` THIRD build loop fanning dog_statsd sinks into the SAME `statsSinks []statssink.Sink` slice + the flusher-gate generalization (a three-way OR); `FuzzDogStatsdSinkConfigParse`; an EXTENSION of `test/helpers/statsdrecv` (a revised colon/pipe split + a NEW `Tags()` accessor — backward-compatible with `0092`'s existing tagless usage); the `0093` differential (REUSING `differential.HostGatewayIP`'s PATTERN via a LOCAL, package-private duplicate in the driver — the `0092` driver's own precedent, to avoid an import cycle with `test/differential`, NOT a new harness function). Byte-identical and stat-surface-identical when no dog_statsd `stats_sinks[]` entry is configured (every non-sink path untouched; the full differential is the regression anchor; `DogStatsdSinkConfigs` stays empty, the flusher build gate unchanged).

**Tech Stack:** Go; the EXISTING `internal/statssink` package (one new file `dogstatsd.go`; reuses `flusher.go`/`sink.go`/`mapping.go`/`delta.go` unchanged; consumes `internal/stats.ExtractTags` DIRECTLY — a NEW cross-package dependency inside `internal/statssink`, but `internal/statssink` already depends on `internal/stats` transitively via `stats.Registry`/`stats.NewRegistry`, so this is NOT a new import edge in the dependency graph, just a new SYMBOL consumed); `internal/bootstrap` (the `stats_sinks[]` three-arm dispatch + the dog_statsd parse arm + the `config/metrics/v3` import, ALREADY present); `cmd/envoy-go/main.go` (the third sink build loop); the driver-owned `test/helpers/statsdrecv` UDP receiver (EXTENDED, not replaced); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`, the `0092` two-per-side-receivers + hard-Close + `HostGatewayIP`-PATTERN precedent). The DogStatsd line protocol is hand-rolled `strconv`/`strings`/`net` — NO client lib; the `DogStatsdSink` proto resolves at the already-imported `go-control-plane/envoy v1.32.4` `config/metrics/v3`. **ZERO new go.mod modules, ZERO new packages.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1200** (H2 cluster; non-H2 **1196**) → **1200** (+0 — D-DSD-STATS-FINAL); fixtures **94** → **95** (`0093`); fuzzers **51** → **52** (`FuzzDogStatsdSinkConfigParse`); BackendKind **38** → **38** (the extended driver-owned UDP receiver is NOT a new BackendKind); DECISIONS tail **ADR-0265** → **ADR-0266** (next-free ADR-0267); **+0 go.mod modules, +0 packages**.
- **Module path:** `github.com/esalaine/envoy-go`.
- **No new dependency:** `go mod tidy -diff` MUST be EMPTY at every task (the DogStatsd line protocol is `strconv`/`strings`/`net`; the `DogStatsdSink` proto resolves at the already-direct go-control-plane).
- **Process anchors:** ADR-0044 (ADR §Decision+§Consequences land at IMPL) · ADR-0045 (sub-split soft gate — escape-valve UNCONSUMED; re-checked at Task 1) · ADR-0080 (strict-reject anti-silent-divergence) · ADR-0064 (`stats_sinks[]` silent-ignore — now a THIRD consumer) · ADR-0060 (no histograms → no `|ms` timers) · ADR-0106 (per-leg rows; row 49 flips `done` here, no parent rollup) · ADR-0266 (this leg — ANCHORED here).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit, every task.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0093'`, NEVER bare `'0093'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the synchronous `DogStatsdSink` adds NO background mutator, but the `Flusher` ticker remains one (now feeding a SECOND periodic UDP sink) — the `-race` gate MUST run the FULL `internal/statssink` package.
- **Delta-sink stability barrier** (`reference_delta_sink_differential_stability_barrier`): a single first-reach-K cannot distinguish delta from absolute; assert the delta-SUM is STILL K after ≥2 further (zero-delta) flushes.
- **Two per-side receivers + hard Close** (`reference_periodic_sink_differential_two_receivers`): periodic flushes stream for the whole test; one shared receiver cross-contaminates.
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the UDP receiver is the EXTENDED `test/helpers/statsdrecv` server the proxy WRITES to — NOT a runner BackendKind (stays 38).
- **Docker bridge + literal IP** (`reference_docker_probe_bridge_network` · `reference_host_gateway_ip_docker_desktop`): the dog_statsd sink rejects hostnames (literal-IP-only, mirroring statsd); the reference reaches the host receiver at the host-gateway literal IP. **IMPORTANT CORRECTION to the naive reading of `reference_host_gateway_ip_docker_desktop`:** the EXPORTED `differential.HostGatewayIP` (`test/differential/harness.go:503`) exists and is exercised by its OWN test (`harness_test.go:174`), but the `0092` driver does **NOT** import it — `runner_test.go` blank-imports the driver FROM WITHIN `package differential`, so a driver package importing `differential` (non-test) risks/creates a cycle with the test-augmented `differential` build. The `0092` driver's actual, LANDED solution (verified by reading `test/fixtures/0092-stats-sink-statsd/driver/driver.go:508-589`) is a LOCAL, unexported `hostGatewayIP` function DUPLICATING `HostGatewayIP`'s body inside the driver package itself. **The `0093` driver MUST do the SAME** (duplicate the helper locally, do NOT attempt `differential.HostGatewayIP(...)` from the driver) — Task 7 below copies the `0092` driver's exact local helper verbatim.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the DogStatsd line `<prefix>.<residual-name>:<value>|<type>[|#tags]` is shared — the SPEC §11 live probe is the wire truth.
- **SN4 wire-name gotcha (caught during this PLAN's own design, folded into SPEC-49 by a follow-up correction commit):** the `_2xx`-class counter's WIRE name after `stats.ExtractTags` is `http.downstream_rq_xx` (the digit is HOISTED INTO A TAG, `envoy.response_code_class`), **NOT** `http.downstream_rq_2xx` (the phase-48 statsd sink's wire name for the "same" stat, since statsd applies NO tag extraction — a genuinely different mapping between the two sinks for the identical underlying counter). Task 7's `subsetNames`/`subsetTags` below use the CORRECT post-extraction wire name.

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding envoy-go's THIRD stats-export sink and the THIRD consumer of the bootstrap `stats_sinks[]` `DogStatsdSink` TypeURL. The substrate is ALL built at phases 47/48 — the `stats.Registry.Walk`/`Freeze` contract, the `internal/statssink` `Flusher` (ticker → `snapshot()` → fan to `[]Sink`), the `Sink` interface, the cumulative/no-labels `snapshot()` mapping, the `deltaState` per-flush-delta transform, the `stats_sinks[]`/`stats_flush_interval` parse, the `cmd/envoy-go/main.go` post-Freeze flusher build + LIFO-drain, AND (unlike at phase 48) the `stats.ExtractTags` tag-extraction core (landed at 47.2b for the metrics_service `emit_tags_as_labels` knob) + the `differential.HostGatewayIP` harness helper (landed at phase 48). Phase 49 adds a bounded delta: one new `Sink` impl (a SECOND UDP writer, this one formatting tags), one config-parse arm (a THIRD dispatch arm), one main third-loop, one fuzzer, an EXTENSION to the existing UDP receiver helper, one differential. **NO new framework piece, NO new package, NO new module.**

**What ALREADY works (do NOT re-build) — verified at PLAN time (2026-07-02; re-confirm line numbers before editing — files evolve):**

- **`internal/statssink/flusher.go`** — `Flusher` (ticker → `snapshot()` → fan `[]*dto.MetricFamily` to `[]Sink`, sink-agnostic, UNCHANGED). `NewFlusher(reg, interval, sinks) *Flusher`; `Start(ctx)`; `flushOnce()`.
- **`internal/statssink/sink.go:18`** — `type Sink interface{ Submit(batch []*dto.MetricFamily); Close() error }`. `MetricsServiceSink` (47.1) and `StatsdSink` (48) are the two existing impls; `DogStatsdSink` is the THIRD. Idioms to copy: `closeOnce sync.Once` + `closeErr error` (idempotent `Close`); `lastDropLog atomic.Int64` + `dropLogIntervalNanos = int64(time.Second)` (`sink.go:47` — reuse, do NOT redeclare) for the rate-limited drop-log.
- **`internal/statssink/delta.go`** — `newDeltaState() *deltaState` / `(*deltaState).apply(abs []*dto.MetricFamily) []*dto.MetricFamily`: COUNTER→per-flush-delta (rebuilt), GAUGE/other→shared-by-pointer absolute. MUST NOT mutate the input (the Flusher fans the SAME slice to every sink — `StatsdSink` and `DogStatsdSink` EACH own a PRIVATE `deltaState`, never shared). An absent key reads 0 ⇒ first flush emits the absolute (no special branch). **This contract MATCHES the DogStatsd `|c`-delta/`|g`-absolute split EXACTLY (D-DSD-DELTA CONFIRMED at SPEC) — reuse it VERBATIM, sink-private, ALWAYS-on, as a SECOND independent instance.**
- **`internal/statssink/mapping.go:22`** — `snapshot(reg, nowMs) []*dto.MetricFamily` — Counter→`COUNTER` absolute, Gauge→`GAUGE`, full dotted `Name()`, ZERO `LabelPair`, no `Help`. Reused unchanged — `DogStatsdSink` consumes the SAME `[]*dto.MetricFamily` the other two sinks do.
- **`internal/statssink/label.go`** — the `labelMapper` (metrics_service `emit_tags_as_labels`, 47.2b): `apply` calls `stats.ExtractTags(fam.GetName())`; on `err != nil || len(labels)==0` it shares the family UNCHANGED (full name, no labels) — the SAME fallback `DogStatsdSink` uses (defensive; can't happen for a registered name). It SORTS its `LabelPair`s (`sort.Slice`, `label.go:50`) because `LabelPair` order is immaterial to a structured Prometheus label — **`DogStatsdSink` must NOT do this** (D-DSD-TAGS-ORDER — the reference's own wire order is `stats.ExtractTags`'s NATURAL, unsorted return order; sorting would DIVERGE from the reference for every `_Nxx`-tagged stat).
- **`internal/statssink/statsd.go`** (107 LoC, phase 48) — the EXACT shape to mirror for the UDP-writer/`Sink` skeleton (construction, `Submit`, `write`, `Close`); `DogStatsdSink` differs ONLY in (a) its OWN independent `*net.UDPConn` + `deltaState`, (b) the per-family `stats.ExtractTags` call + tag-suffix formatting.
- **`internal/stats/name.go:47`** — `func ExtractTags(internal string) (string, []Label, error)` — the residual dotted name (tag-value segments removed) + the extracted `[]Label{Key, Value}` (keys in the `envoy_`-prefixed Prometheus underscore form, e.g. `envoy_cluster_name`) + `nil` on success; `"", nil, error` on a name matching no top-level rule. **`name.go:354-357`** — Rule SN4 PREPENDS the `envoy_response_code_class` label to the FRONT of the slice (`labels = append([]Label{{...}}, labels...)`) — the load-bearing evidence for the natural-order-matches-reference finding (SPEC §1.1 AMEND-DSD-TAGS-ORDER).
- **`internal/bootstrap/bootstrap.go`** — `metricsServiceTypeURL` (`:214`, descriptor-derived); `statsdSinkTypeURL` (`:221`, descriptor-derived, phase 48); `StatsdSinkConfig` struct (`:280-283`); `Bootstrap.StatsSinkConfigs` (`:368`) / `Bootstrap.StatsdSinkConfigs` (`:377`) / `Bootstrap.FlushInterval` fields. `parseStatsSinks(bs, result)` (`:444-473`): sets `result.FlushInterval` (default 5s, `:445-450`), rejects `stats_flush_on_admin` (`:451-453`), then for each `stats_sinks[]` entry (`:454`) a `nil` typed_config → reject (`:456-458`), then the TWO-arm `switch tc.GetTypeUrl()` (`:459-470`): `metricsServiceTypeURL` → `parseMetricsServiceConfig` (`:460-463`); `statsdSinkTypeURL` → `parseStatsdSinkConfig` (`:464-467`); `default` → the sibling-reject naming BOTH sinks (`:468-469`). `parseStatsdSinkConfig` (`:518-539`) is the parse-arm shape to mirror EXACTLY (its reject-ordering comment at `:510-517` explains why `tcp_cluster_name` is checked before the nil-`socket_address` check — `DogStatsdSink` has NO such sibling oneof member, so `parseDogStatsdSinkConfig` is SIMPLER: just the nil-`socket_address` check, no ordering subtlety).
- **`cmd/envoy-go/main.go`** — the sink build block (`:180-231`): the comment (`:180-189`); `var statsFlusher *statssink.Flusher` (`:190`); `var statsSinks []statssink.Sink` (`:191`); `flusherDone := make(chan struct{})` (`:192`); the gate `if len(bs.StatsSinkConfigs) > 0 || len(bs.StatsdSinkConfigs) > 0 {` (`:193`) wrapping the metrics_service loop (`:194-203`) then the statsd loop (`:208-214`) then `statsFlusher = statssink.NewFlusher(...)` (`:215`); the LIFO-drain defer (`:226-231`). **All sink-agnostic — generalize the gate to a three-way OR + append a third loop.**
- **`test/helpers/statsdrecv/statsdrecv.go`** (159 LoC, phase 48) — the driver-owned UDP receiver: `NewAtAddr(addr)`, `DeltaSum(name)`, `Gauge(name)`, `SeenCount(name)`, `Reset()`, `Addr()`, `Close()`. The `ingest` function (`:79-111`) currently splits `name`/`value` via `colon := strings.LastIndexByte(line, ':')` (`:87`) — **this BREAKS on a tagged DogStatsd line** (the `|#` tag suffix itself contains `key:value` pairs, so the LAST colon in a tagged line falls INSIDE the tag suffix, not at the name/value boundary). Task 6 below REVISES this to a first-`|`-then-colon split (SPEC §8.2's fold).
- **`test/differential/harness.go:503`** — `func HostGatewayIP(ctx context.Context) (string, error)` ALREADY EXISTS (landed at phase 48 Task 7) — but per the Global Constraints note above, the `0093` driver does NOT import it; it DUPLICATES the `0092` driver's own local `hostGatewayIP` helper (`test/fixtures/0092-stats-sink-statsd/driver/driver.go:508-589`) verbatim into the `0093` driver package.
- **`test/fixtures/0092-stats-sink-statsd/driver/driver.go`** (628 LoC) — **the `0093` driver TEMPLATE** (the delta-SUM + stability-barrier + gauge-subset shape, NOW EXTENDED with tag-set assertions). Task 7 clones this file and adapts: the DogStatsdSink typed_config; the WIRE (post-`ExtractTags`) subset names (NOT the raw dotted names `0092` uses — SN4 rewrites `_2xx`→`_xx`); a `subsetTags`/`gaugeTags` expected-tag-set map; `statsdrecv.Server.Tags(name)` assertions; the SAME local `hostGatewayIP` duplicate; the SAME two-receiver + hard-Close + poll-to-converge + stability-barrier shape.

**The DogStatsd wire model (SPEC-49.md §11 D-DSD-* — live-probed against `contrib-v1.37.2` 2026-07-02; all pinned):**
- **The line** (D-DSD-LINE): `<prefix>.<residual-name>:<int>|<type>[|#tag1:val1,...]`, ONE line per UDP datagram BY DEFAULT (`max_bytes_per_datagram` unset — confirmed: 100% single-line datagrams in the base/no-prefix/protocol-TCP probe runs). Integer value, no `@rate`, no sign (except negative gauges). `<type>` ∈ {`c` counter-delta, `g` gauge-absolute}; envoy-go has NO histograms (ADR-0060) ⇒ `|c`/`|g`-only.
- **The value** (D-DSD-DELTA — LOAD-BEARING, independently confirmed): COUNTER `|c` is the per-flush DELTA-since-last-flush (`cluster.upstream_rq_total` emitted `6,1,0,0`/`7,0,0` across flushes — SUM==7==K). GAUGE `|g` is ABSOLUTE, no sign prefix (except a genuinely negative gauge). ⇒ `DogStatsdSink` ALWAYS owns its OWN sink-private `deltaState` (no knob), a SECOND instance from `StatsdSink`'s.
- **The name + tags** (D-DSD-NAME + D-DSD-TAGS — the SECOND LOAD-BEARING finding): the residual is BYTE-IDENTICAL to `stats.ExtractTags`'s projection (`cluster.backend.upstream_rq_total` → residual `cluster.upstream_rq_total` + tag `envoy.cluster_name:backend`); the prefix join is `prefix + "." + residual`; default prefix `envoy`. Tag delimiter `,` between pairs, `:` within a pair; tag-KEY format `envoy.<key>` IDENTICAL to the `labelMapper` convention (`"envoy."+TrimPrefix(l.Key,"envoy_")`); untagged names emit with NO `|#` suffix. **Tag ORDER is NOT sorted** — the reference emits `stats.ExtractTags`'s own natural (SN4-prepended) order (`envoy.response_code_class:2,envoy.cluster_name:backend` — reverse-alpha); the production formatter must iterate the `[]Label` slice AS-IS.
- **The reject roster** (D-DSD-REJECT): the reference REJECTS at load a missing `dog_statsd_specifier` (PGV oneof-required, wording-identical shape to the statsd finding); BOOTS and DEMONSTRABLY HONORS an EXPLICIT `max_bytes_per_datagram` (multi-metric newline-batched datagrams up to the byte cap — a REAL feature, not a no-op) ⇒ envoy-go-STRICT-REJECTS it explicitly; BOOTS a `socket_address` with `protocol: TCP` and STILL emits UDP (the field is IGNORED, identical to statsd).
- **+0 self-stats, no sink cluster** (D-DSD-STATS): the reference registers no dog_statsd-scoped self-stat; the UDP sink dials no cluster. Surface delta +0.

### Proto facts (verified at PLAN time; `go-control-plane/envoy v1.32.4` already direct; NO new module)

- **`metricsconfigv3.DogStatsdSink`** (`config/metrics/v3/stats.pb.go:695`; the SAME package already named-imported in `bootstrap.go` as `metricsconfigv3`, a normal named import — NOT blank). The TypeURL is `type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink` (SPEC §11 live-verified; **DERIVE via the proto descriptor, NOT hard-code**). Accessors: `GetAddress() *corev3.Address` (field 1, oneof `dog_statsd_specifier` — the **ONLY** member; unlike `StatsdSink`, there is NO `tcp_cluster_name`-equivalent sibling arm); `GetPrefix() string` (field 3); `GetMaxBytesPerDatagram() *wrapperspb.UInt64Value` (field 4 — a WRAPPER type, so `nil` distinguishes "unset" from an explicit `0`).
- **`corev3.Address` → `socket_address`**: SAME as the `StatsdSink` consumption — `sa.GetAddress() string` (IP literal); `sa.GetPortValue() uint32`; `sa.GetProtocol()` (accepted-and-IGNORED).
- `anypb.Any.UnmarshalTo(&metricsconfigv3.DogStatsdSink{})` — the unmarshal (the `parseStatsdSinkConfig` precedent).

---

## D-question resolutions (the SPEC §12 D-DSD-* PLAN/IMPL pins — settled here)

**D-DSD-SPLIT → NO sub-split (a SINGLE FLAT ROW, 9 tasks).** Anticipated ~250–320 prod LoC: one new `Sink` impl (`dogstatsd.go`, ~100–120 LoC — slightly larger than `statsd.go`'s 108 owing to the tag-formatting path) + the parse arm (~45 LoC) + the main third-loop (~10 LoC) + the `statsdrecv` extension (~40 LoC delta) + the driver (~650 LoC, but almost entirely a clone of the landed `0092` driver). Well under the ADR-0045 gate; the 49.1/49.2 escape-valve stays UNCONSUMED. Re-confirmed at Task 1 with the real baseline. **9 tasks (not 10 like phase 48)** — no separate harness-helper task is needed since `HostGatewayIP`'s PATTERN is duplicated locally in the driver (the `0092` precedent), not built fresh.

**D-DSD-CONFIG-HOME → a parallel `DogStatsdSinkConfigs []DogStatsdSinkConfig` slice on `internal/bootstrap`** (alongside `StatsSinkConfigs`/`StatsdSinkConfigs`), NOT a unified sink-config interface list — the SAME rationale as `StatsdSinkConfig`: disjoint parse-time data per sink kind, and `main.go` already owns "build each config kind into the one `statsSinks []Sink`." `main.go` runs THREE build loops (metrics_service, then statsd, then dog_statsd) appending into the SAME slice.

**D-DSD-TAG-HELPER → duplicate the 1-line `"envoy."+strings.TrimPrefix(key,"envoy_")` transform inline in `dogstatsd.go`, NO shared helper.** The transform is a single expression; `labelMapper` builds a `dto.LabelPair` (a structured field), `DogStatsdSink` builds a literal wire-string segment — the two call sites have different SHAPES around the one-line transform (a `proto.String(...)` field assignment vs a `strings.Builder` append), so factoring a shared 1-line helper would be a premature abstraction over a trivial expression, not a genuine dedup. (Per the project's own no-premature-abstraction discipline — three similar lines beat an introduced indirection.)

**D-DSD-DELTA-REUSE → reuse the landed `delta.go` `newDeltaState()`/`apply()` VERBATIM (same package, unexported), as a SECOND sink-private, ALWAYS-on instance — INDEPENDENT of `StatsdSink`'s own instance.** The `apply` contract MATCHES the DogStatsd `|c`/`|g` split EXACTLY (D-DSD-DELTA CONFIRMED, matching phase 48's `|c` finding independently). `DogStatsdSink` holds its OWN `delta *deltaState` field (always non-nil); `NewDogStatsdSink` calls `newDeltaState()` fresh — NEVER passed in from `StatsdSink`.

**D-DSD-ADDR → `UDPAddress string` (a `host:port` literal), resolved at `NewDogStatsdSink` via `net.ResolveUDPAddr`.** Identical shape to `StatsdSinkConfig`/`NewStatsdSink` — `parseDogStatsdSinkConfig` builds `fmt.Sprintf("%s:%d", host, port)`; `NewDogStatsdSink` resolves + dials. A resolve/dial error surfaces at `NewDogStatsdSink` → `main.go` `log.Fatalf` (the `StatsdSink`-error precedent).

**D-DSD-RECEIVER-WIRING → the `0093` driver DUPLICATES the `0092` driver's LOCAL `hostGatewayIP` helper verbatim (NOT `differential.HostGatewayIP`) — see the Global Constraints note above for WHY (the import-cycle avoidance the `0092` driver already solved).** The `statsdrecv.Server` receiver TYPE is EXTENDED (Task 6), not replaced; `NewAtAddr`/`Addr`/`Close`/`DeltaSum`/`Gauge`/`SeenCount` are REUSED unchanged; the NEW `Tags(name)` accessor is ADDED. TWO per-side receivers, each bound `0.0.0.0` (the `0092` precedent) — reachability PROVEN at Task 7 (decode-ran: receiver datagram count > 0, PLUS the delta-SUM converge-poll structurally requires decode).

**D-DSD-FUZZER → land `FuzzDogStatsdSinkConfigParse` at 49 (fuzzers 51 → 52).** A no-panic fuzzer over the dog_statsd parse arm (the `FuzzStatsdSinkConfigParse` precedent). Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 52` at the completion task (`reference_fuzzer_count_docs_drift` — **51 actual** at baseline, verified in-session).

**D-DSD-STATS-FINAL → +0 (NO new dog_statsd-scoped stat name; surface stays 1200 / non-H2 1196).** Matches the reference (+0 self-stats). `Write`-drops are rate-limit-LOGGED, NOT counted. A registration test (Task 8) asserts the surface is UNCHANGED.

**D-DSD-GAUGE-SUBSET → YES, `0093` adds a cross-side absolute-gauge-PLUS-tag subset assertion (`cluster.membership_total == 1|g` tagged `{"envoy.cluster_name": <backend>}`) alongside the counter delta-SUM-plus-tags.** Mirrors the `0092` `D-SD-GAUGE-SUBSET` choice exactly (`membership_total`, NOT `membership_healthy` — `reference_membership_total_vs_healthy_gauge`: envoy-go registers `membership_healthy` only on health-checked clusters, and the `0093` topology — cloned from `0092` — carries none).

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/statssink/dogstatsd.go` — `type DogStatsdSink struct{ conn *net.UDPConn; prefix string; delta *deltaState; closeOnce sync.Once; closeErr error; lastDropLog atomic.Int64 }`; `NewDogStatsdSink(udpAddr, prefix string) (*DogStatsdSink, error)`; `Submit(batch []*dto.MetricFamily)` (delta.apply → per-family `stats.ExtractTags` → tag-suffix build → one datagram); `Close() error`. Satisfies `Sink`.

**Production (modified):**
- `internal/bootstrap/bootstrap.go` — the `dogStatsdSinkTypeURL` const; the `DogStatsdSinkConfig` struct + `Bootstrap.DogStatsdSinkConfigs` field; the two-arm `parseStatsSinks` dispatch → THREE arms; `parseDogStatsdSinkConfig` + the strict-reject arms.
- `cmd/envoy-go/main.go` — the flusher-build gate generalization (three-way OR) + the THIRD (dog_statsd) build loop.
- `test/helpers/statsdrecv/statsdrecv.go` — the `ingest` colon/pipe-split revision + a NEW `tags map[string]map[string]string` field + `Tags(name)` accessor + `Reset()` clearing it.

**Test (created):**
- `internal/statssink/dogstatsd_test.go` (`-race`, table-driven over the EXISTING `udpListener`/`sameSet` helpers in `statsd_test.go` — SAME package, reused directly, no redefinition).
- `internal/bootstrap/dogstatsd_fuzz_test.go` (`FuzzDogStatsdSinkConfigParse`).
- `test/fixtures/0093-stats-sink-dogstatsd/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.

**Test (modified):**
- `internal/bootstrap/bootstrap_test.go` (the dog_statsd parse-accept + strict-reject arms).
- `internal/statssink/registration_test.go` (`TestNoNewStat_StatsdRegistrationGuard` — the +0 surface guard, confirm UNCHANGED).
- `test/helpers/statsdrecv/statsdrecv_test.go` (the NEW `Tags()` tests + a backward-compatibility regression test proving tagless `0092`-style lines still parse identically).
- `test/differential/runner_test.go` (blank-import the `0093` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/49-stats-sink-dogstatsd/PROGRESS-49.md`, `docs/envoy-go/DECISIONS.md` (ADR-0266 §Decision/§Consequences — ANCHORS the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 49 flips `done`**).

---

## Task 1: Phase scaffolding — PROGRESS-49.md + baselines + the final ADR-0045 split re-check (D-DSD-SPLIT)

**Files:**
- Create: `docs/envoy-go/phases/49-stats-sink-dogstatsd/PROGRESS-49.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-49.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                   # expect 94 (tail 0092-stats-sink-statsd)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 51
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go # expect = the BackendKind tail (38)
go mod tidy -diff                                                # expect EMPTY (clean)
grep -rn 'DogStatsdSinkConfig\|dogStatsdSinkTypeURL\|parseDogStatsdSinkConfig\|statssink/dogstatsd\|\.Tags(' internal/ cmd/ test/ --include='*.go'  # expect: NONE (49 introduces them)
grep -c 'statsdSinkTypeURL' internal/bootstrap/bootstrap.go      # expect >=2 (the const + the gate)
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **94** / fuzzers **51** / BackendKind **38** / DECISIONS tail **ADR-0265** (next-free **ADR-0266**).

- [ ] **Step 2: Write the PROGRESS-49.md scaffold** — a header (phase 49 IMPL, the SPEC-49 reference + the "THIRD `stats_sinks[]` consumer + SECOND UDP datagram seam + SIXTH Observability-family row + tags via `stats.ExtractTags` in natural order; ANCHORS ADR-0266; row 49 flips `done` at this IMPL" note, the worktree branch), a task checklist mirroring this plan, the baseline block, the **D-DSD-SPLIT confirmation (NO sub-split — the escape-valve stays UNCONSUMED; the LoC estimate above; 9 tasks, one fewer than phase 48 since `HostGatewayIP`'s pattern is duplicated, not built)**, and the anticipated exit counts: stat **1200** (+0 — D-DSD-STATS-FINAL) / fixtures **95** (`0093-stats-sink-dogstatsd`) / fuzzers **52** (`FuzzDogStatsdSinkConfigParse`) / BackendKind **38** / DECISIONS **ADR-0266** / **0 new packages, 0 new go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/49-stats-sink-dogstatsd/PROGRESS-49.md
git commit -m "phase 49 Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (dog_statsd UDP stats sink with tags; ANCHORS ADR-0266; row 49 flips done at this IMPL)"
```

---

## Task 2: The dog_statsd parse arm — `dogStatsdSinkTypeURL` + `DogStatsdSinkConfig` + the three-arm dispatch + `parseDogStatsdSinkConfig` + the strict-reject arms (`internal/bootstrap/bootstrap.go`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: the existing `parseStatsSinks` (`:444`) + `metricsServiceTypeURL`/`statsdSinkTypeURL` (`:214`/`:221`) + `metricsconfigv3` import (`:161`); `anypb.Any` (already imported).
- Produces: `type DogStatsdSinkConfig struct{ UDPAddress string; Prefix string }`; `Bootstrap.DogStatsdSinkConfigs []DogStatsdSinkConfig`; `dogStatsdSinkTypeURL` (var, descriptor-derived); `parseDogStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error`. (Task 5's `main.go` loop reads `bs.DogStatsdSinkConfigs`; Task 3's fuzzer drives this arm through `Load`.)

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (mirror the existing statsd parse tests — find them via `grep -n 'parseStatsdSinkConfig\|StatsdSinkConfigs\|statsd' internal/bootstrap/bootstrap_test.go`; reuse the YAML-`Load` harness). The dog_statsd TypeURL string is `type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink`.
  - **accept — UDP socket_address + prefix**:
    ```yaml
    stats_sinks:
      - name: envoy.stat_sinks.dog_statsd
        typed_config:
          "@type": type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink
          address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
          prefix: myprefix
    ```
    ⇒ `Load` succeeds; `len(bs.DogStatsdSinkConfigs) == 1`; `bs.DogStatsdSinkConfigs[0] == DogStatsdSinkConfig{UDPAddress: "127.0.0.1:8125", Prefix: "myprefix"}`; `len(bs.StatsSinkConfigs) == 0`; `len(bs.StatsdSinkConfigs) == 0`.
  - **accept — default prefix `envoy`** (no `prefix`): same but omit `prefix` ⇒ `DogStatsdSinkConfigs[0].Prefix == "envoy"`.
  - **accept — `protocol: TCP` IGNORED** (`socket_address: { address: 127.0.0.1, port_value: 8125, protocol: TCP }`) ⇒ succeeds; `UDPAddress == "127.0.0.1:8125"`.
  - **reject — explicit `max_bytes_per_datagram`** (`typed_config: { ...DogStatsdSink, address: {...}, max_bytes_per_datagram: 512 }`) ⇒ `Load` returns an error matching `max_bytes_per_datagram`.
  - **reject — missing `dog_statsd_specifier`** (a `DogStatsdSink` with no `address`, e.g. only `prefix: x`) ⇒ error matching `socket_address` / `dog_statsd_specifier`.
  - **reject — sibling/unknown TypeURL** (an entry whose `@type` is some OTHER metrics type not in {metrics_service, statsd, dog_statsd}) ⇒ error naming ALL THREE supported sinks (the message contains `metrics_service`, `statsd`, AND `dog_statsd`).
  - **accept — coexisting metrics_service + statsd + dog_statsd** (all three in one `stats_sinks[]` list) ⇒ each populates its OWN config slice; no cross-contamination.
  - **typeurl-descriptor test**: assert `dogStatsdSinkTypeURL == "type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink"` (mirror the existing `statsdSinkTypeURL` assertion test).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/bootstrap/ -run 'TestDogStatsd|TestParseDogStatsd|TestDogStatsdSinkTypeURL' -count=1` ⇒ FAIL (undefined `DogStatsdSinkConfig`/`dogStatsdSinkTypeURL`/`parseDogStatsdSinkConfig`; the accepts error on the unsupported sibling). (Adjust `-run` to the actual test names chosen.)

- [ ] **Step 3: Implement** in `bootstrap.go`:
  - Add the descriptor-derived const beside `statsdSinkTypeURL` (`:221`):
```go
// dogStatsdSinkTypeURL is the typed_config TypeURL for the dog_statsd UDP stats
// sink with tags (envoy.config.metrics.v3.DogStatsdSink). DERIVED from the proto
// descriptor (the metricsServiceTypeURL/statsdSinkTypeURL precedent —
// reference_network_filter_typeurl_extensions); the DogStatsdSink type resolves
// at the already-imported metricsconfigv3 package (config/metrics/v3, v1.32.4).
// A test asserts it equals the SPEC §11 string.
var dogStatsdSinkTypeURL = "type.googleapis.com/" + string((&metricsconfigv3.DogStatsdSink{}).ProtoReflect().Descriptor().FullName())
```
  - Add the struct beside `StatsdSinkConfig` (`:280-283`):
```go
// DogStatsdSinkConfig is the parsed dog_statsd UDP stats-sink config from one
// top-level stats_sinks[] DogStatsdSink entry (ADR-0266). The sink (the
// DogStatsdSink + Flusher) is constructed in cmd/envoy-go/main.go after Load
// returns; this struct carries only the parse-time data. An EXPLICITLY set
// max_bytes_per_datagram is STRICT-REJECTED (the reference HONORS it with real
// multi-metric batching; envoy-go emits one metric per datagram this row); a
// missing dog_statsd_specifier / nil socket_address is a REFERENCE-PARITY
// reject; socket_address.protocol is accepted-and-IGNORED (dial UDP regardless).
type DogStatsdSinkConfig struct {
	UDPAddress string // socket_address host:port (an IP literal:port; net.ResolveUDPAddr-resolvable)
	Prefix     string // DogStatsdSink.prefix, default "envoy" when empty
}
```
  - Add the field beside `StatsdSinkConfigs` (`:377`): `DogStatsdSinkConfigs []DogStatsdSinkConfig`.
  - Replace the two-arm dispatch in `parseStatsSinks` (`:459-470`) with a three-arm dispatch:
```go
		switch tc.GetTypeUrl() {
		case metricsServiceTypeURL:
			if err := parseMetricsServiceConfig(tc, i, result); err != nil {
				return err
			}
		case statsdSinkTypeURL:
			if err := parseStatsdSinkConfig(tc, i, result); err != nil {
				return err
			}
		case dogStatsdSinkTypeURL:
			if err := parseDogStatsdSinkConfig(tc, i, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports the metrics_service sink %q, the statsd sink %q, and the dog_statsd sink %q)", i, tc.GetTypeUrl(), metricsServiceTypeURL, statsdSinkTypeURL, dogStatsdSinkTypeURL)
		}
```
  - Add `parseDogStatsdSinkConfig` (beside `parseStatsdSinkConfig`):
```go
// parseDogStatsdSinkConfig parses one dog_statsd UDP stats sink typed_config and
// appends a DogStatsdSinkConfig to result.DogStatsdSinkConfigs (ADR-0266). It
// STRICT-REJECTS (ADR-0080): an EXPLICITLY set max_bytes_per_datagram (the
// reference HONORS it with real multi-metric newline-batched datagrams — a
// genuine, deferred feature, not a no-op; envoy-go is one-metric-per-datagram
// only this row). A missing dog_statsd_specifier / nil socket_address is a
// REFERENCE-PARITY reject (the reference PGV-rejects it). UNLIKE StatsdSink,
// DogStatsdSink's oneof has ONLY the address member — no tcp_cluster_name-shaped
// sibling arm to check first (simpler ordering than parseStatsdSinkConfig's).
// socket_address.protocol is accepted-and-IGNORED. prefix defaults to "envoy"
// when empty.
func parseDogStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var dsd metricsconfigv3.DogStatsdSink
	if err := tc.UnmarshalTo(&dsd); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd typed_config: %w", idx, err)
	}
	if dsd.GetMaxBytesPerDatagram() != nil {
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

- [ ] **Step 4: Run to verify they pass — AND fix two PRE-EXISTING tests that will flip from reject to accept.** `go test ./internal/bootstrap/ -run 'TestDogStatsd|TestParseDogStatsd|TestDogStatsdSinkTypeURL' -count=1` ⇒ PASS; `go mod tidy -diff` ⇒ EMPTY. **Load-bearing (caught by the plan-document-reviewer pass — do NOT rely on a text-match grep here, it returns zero hits and is a red herring):** `bootstrap_test.go` already has TWO existing sibling/unknown-TypeURL reject table rows that use a syntactically-valid `dog_statsd`/`DogStatsdSink` config as their "currently unsupported sink" example — `TestStatsSink_Rejects/sibling_unknown_sink` (~line 1862) and `TestStatsdSink_Rejects/sibling_unknown_typeurl` (~line 2092), both asserting `if err == nil { t.Fatalf(...) }`. Once this task's three-arm dispatch ACCEPTS `dogStatsdSinkTypeURL`, `Load` SUCCEEDS on both inputs and both tests FAIL (a semantic accept/reject flip on the table row, not a stale error-string — `grep -rn 'supports the metrics_service sink' internal/bootstrap/*_test.go` finds NOTHING because neither test string-matches that wording). Locate both cases (`grep -n 'sibling_unknown_sink\|sibling_unknown_typeurl' internal/bootstrap/bootstrap_test.go`) and swap their example TypeURL from `DogStatsdSink` to a genuinely-still-unsupported one (e.g. a stub `envoy.config.metrics.v3.GraphiteStatsSink`-shaped TypeURL, or any TypeURL string not in `{metricsServiceTypeURL, statsdSinkTypeURL, dogStatsdSinkTypeURL}`). Then run the FULL package: `go test ./internal/bootstrap/ -count=1` ⇒ ALL PASS (this is the real regression gate — a narrow `-run` filter would NOT catch this).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 49 Task 2: dog_statsd parse arm — dogStatsdSinkTypeURL + DogStatsdSinkConfig + three-arm stats_sinks[] dispatch + parseDogStatsdSinkConfig + strict-reject (max_bytes_per_datagram-set / missing-specifier / sibling-TypeURL naming all three sinks) (ADR-0266, ADR-0080)"
```

---

## Task 3: `FuzzDogStatsdSinkConfigParse` — the no-panic fuzzer over the dog_statsd parse arm (`internal/bootstrap/dogstatsd_fuzz_test.go`)

**Files:**
- Create: `internal/bootstrap/dogstatsd_fuzz_test.go`

The dog_statsd parse arm is an untrusted bootstrap-config boundary (the `FuzzStatsdSinkConfigParse` precedent at `internal/bootstrap/statsd_fuzz_test.go`). Drive `Load` end-to-end; assert no-panic.

- [ ] **Step 1: Write the fuzzer** in `dogstatsd_fuzz_test.go` (mirror `statsd_fuzz_test.go` — the same `head` constant + `f.Fuzz(func(t, data){ _, _ = Load(bytes.NewReader(data)) })` body). Seeds exercise: the valid accept (socket_address + prefix); the default-prefix accept; `protocol: TCP` accepted; each reject arm (`max_bytes_per_datagram` set; missing `dog_statsd_specifier`; the sibling/unknown metrics TypeURL); a coexisting metrics_service + statsd + dog_statsd triple; plus degenerate/garbage documents.
```go
package bootstrap

import (
	"bytes"
	"testing"
)

// FuzzDogStatsdSinkConfigParse exercises the dog_statsd stats_sinks[] parse arm
// (phase 49 Task 2) end-to-end through Load for arbitrary bootstrap document
// bytes. Load MUST NOT panic on any input; a returned error is fine (D-DSD-FUZZER).
func FuzzDogStatsdSinkConfigParse(f *testing.F) {
	const head = `node: { id: dsd-node, cluster: dsd-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	const dogStatsdType = "type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink"
	const statsdType = "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"
	const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"

	// valid accept (socket_address + prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      prefix: myprefix
`))
	// default prefix (no prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`))
	// protocol: TCP accepted-and-ignored
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125, protocol: TCP } }
`))
	// max_bytes_per_datagram set (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      max_bytes_per_datagram: 512
`))
	// missing dog_statsd_specifier (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      prefix: x
`))
	// coexisting metrics_service + statsd + dog_statsd
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.metrics_service
    typed_config:
      "@type": ` + msType + `
      transport_api_version: V3
      grpc_service:
        envoy_grpc:
          cluster_name: mc
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8126 } }
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`))
	// degenerate / garbage
	f.Add([]byte{})
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("stats_sinks: [{}]\n"))
	f.Add([]byte(head + "stats_sinks: [{typed_config: {\"@type\": " + dogStatsdType + "}}]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic regardless of data content; an error return is fine.
		_, _ = Load(bytes.NewReader(data))
	})
}
```

- [ ] **Step 2: Run the fuzzer briefly** — `go test ./internal/bootstrap/ -run 'FuzzDogStatsdSinkConfigParse' -count=1` (seed-only) ⇒ PASS; then `go test ./internal/bootstrap/ -fuzz 'FuzzDogStatsdSinkConfigParse' -fuzztime 20s` ⇒ no crash, no panic. Confirm the running fuzzer count is now 52: `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **52**.

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/dogstatsd_fuzz_test.go
git commit -m "phase 49 Task 3: FuzzDogStatsdSinkConfigParse — no-panic fuzzer over the dog_statsd stats_sinks[] parse arm (fuzzers 51 -> 52; D-DSD-FUZZER)"
```

---

## Task 4: The `DogStatsdSink` (`internal/statssink/dogstatsd.go`) — a SECOND `*net.UDPConn` writer + the DogStatsd-line-with-tags mapping over a SECOND sink-private `deltaState` + `stats.ExtractTags` in NATURAL order [TDD, table-driven; full-package `-race`]

**Files:**
- Create: `internal/statssink/dogstatsd.go`, `internal/statssink/dogstatsd_test.go`

**Interfaces:**
- Consumes: the existing `Sink` interface (`sink.go:18`); `newDeltaState()`/`deltaState.apply` (`delta.go`); `stats.ExtractTags` (`internal/stats/name.go:47` — a NEW import inside `internal/statssink`, but `internal/statssink` already imports `internal/stats` for `*stats.Registry`, so this is not a new package-level dependency edge); `dto "github.com/prometheus/client_model/go"` (already a dep); the EXISTING `udpListener`/`sameSet` test helpers in `statsd_test.go` (SAME package `statssink` — reused directly, no redefinition).
- Produces: `type DogStatsdSink struct{...}`; `NewDogStatsdSink(udpAddr string, prefix string) (*DogStatsdSink, error)`; `Submit(batch []*dto.MetricFamily)`; `Close() error`. (Task 5's `main.go` loop calls `NewDogStatsdSink`.)

- [ ] **Step 1: Write the failing tests** in `dogstatsd_test.go` (reuse `udpListener(t)` / `sameSet(t, got, want)` from `statsd_test.go` — same package, do NOT redefine):
  - **counter→`|c` delta + gauge→`|g` absolute + prefix join + a two-tag counter, NATURAL (unsorted) order**: build a batch via `snapshot()` over a registry with `c := reg.NewCounter("cluster.backend.upstream_rq_total"); c.Add(7)` and `g := reg.NewGauge("cluster.backend.membership_total"); g.Set(1)`; `s, _ := NewDogStatsdSink(addr, "dsdpfx")`; `s.Submit(batch)`; read 2 datagrams ⇒ the set equals `{"dsdpfx.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend", "dsdpfx.cluster.membership_total:1|g|#envoy.cluster_name:backend"}` (note the RESIDUAL name — `cluster.upstream_rq_total`, NOT the raw `cluster.backend.upstream_rq_total` — the `backend` segment is HOISTED into the tag).
  - **the SN4 status-class collapse + NATURAL (unsorted) two-tag order**: `rq2xx := reg.NewCounter("http.hcm_local.downstream_rq_2xx"); rq2xx.Add(5)`; `Submit` ⇒ the line is `"dsdpfx.http.downstream_rq_xx:5|c|#envoy.response_code_class:2,envoy.http_conn_manager_prefix:hcm_local"` — assert the EXACT literal string (order matters here: `response_code_class` BEFORE `http_conn_manager_prefix`, the SN4-prepend order — a `sort.Slice` bug would emit `http_conn_manager_prefix` first and this assertion would catch it).
  - **untagged name emits NO `|#` suffix**: `u := reg.NewCounter("server.dynamic_unknown_fields"); u.Add(0)`; `Submit` ⇒ the line is exactly `"dsdpfx.server.dynamic_unknown_fields:0|c"` (no trailing `|#`, no trailing empty segment).
  - **delta semantics across flushes**: `c.Add(7)` (cumulative 7) → `Submit` ⇒ `...upstream_rq_total:7|c|#...`; then WITHOUT adding more, `Submit` again ⇒ `...upstream_rq_total:0|c|#...` (the second flush's delta is 0 — proves the sink-private `deltaState` is live); then `c.Add(3)` (cumulative 10) → `Submit` ⇒ `...upstream_rq_total:3|c|#...`.
  - **independence from a co-existing `StatsdSink`'s `deltaState`**: construct BOTH a `NewStatsdSink` and a `NewDogStatsdSink` against TWO SEPARATE `udpListener`s, `Submit` the SAME batch to both; assert each sink's OWN delta sequence is independent (e.g. flush the `StatsdSink` an EXTRA time in between — the `DogStatsdSink`'s next delta must be UNAFFECTED, proving no shared state).
  - **gauge stays absolute across flushes**: `g.Set(1)` → `Submit` ⇒ `...:1|g|#...`; `g.Set(1)` again → `Submit` ⇒ `...:1|g|#...` (NOT a 0 delta).
  - **negative gauge**: `g.Set(-5)` ⇒ `...:-5|g|#...` (the only sign case).
  - **default prefix**: `NewDogStatsdSink(addr, "envoy")` ⇒ lines start `envoy.`.
  - **empty batch**: `s.Submit(nil)` ⇒ no datagram written, no panic.
  - **Close idempotent**: `s.Close()` twice ⇒ same (nil) error, no panic.
  - **resolve error**: `NewDogStatsdSink("not a valid addr", "p")` ⇒ `(nil, err)`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestDogStatsdSink' -count=1` ⇒ FAIL (`DogStatsdSink`/`NewDogStatsdSink` undefined).

- [ ] **Step 3: Implement** `dogstatsd.go`:
```go
package statssink

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/esalaine/envoy-go/internal/stats"
)

// DogStatsdSink writes the frozen registry snapshot to a DogStatsd server as UDP
// datagrams every flush (ADR-0266): one DogStatsd line per metric family,
// <prefix>.<residual-name>:<value>|<type>[|#tag1:val1,...]. COUNTER families
// carry the per-flush DELTA (|c) over a SECOND, INDEPENDENT sink-private
// always-on deltaState (D-DSD-DELTA — reusing the landed delta.go transform
// VERBATIM, no knob, NEVER shared with StatsdSink's own instance); GAUGE
// families carry the ABSOLUTE value (|g). The name is stats.ExtractTags's
// residual dotted name (the SAME SN1-SN9+SN4 matcher label.go's labelMapper
// calls, consumed DIRECTLY here — not via labelMapper/LabelPair, since the
// target shape is an inline wire-string suffix, not a structured field); the
// extracted tags are formatted envoy.<key>:<value>, comma-joined, in the
// SLICE'S NATURAL (unsorted) order (D-DSD-TAGS-ORDER — the reference does NOT
// alphabetically sort; ExtractTags's own SN4-prepended order already matches
// it. CONTRAST labelMapper, which sorts because LabelPair order is immaterial
// to a structured Prometheus label — a DogStatsd tag suffix is a literal wire
// string where order is part of the byte-format). A name with zero extracted
// tags (or an ExtractTags error — defensive, can't happen for a registered
// name) emits with NO |# suffix at all. envoy-go has no histograms (ADR-0060),
// so only |c/|g lines are produced.
//
// Writer shape (D-DSD-LIFECYCLE): SYNCHRONOUS, identical to StatsdSink — a UDP
// Write is fire-and-forget and the Flusher calls Submit serially, so Submit
// writes each datagram inline. This is a SECOND, INDEPENDENT *net.UDPConn from
// StatsdSink's (never shared).
type DogStatsdSink struct {
	conn   *net.UDPConn
	prefix string
	delta  *deltaState // always non-nil — a SECOND, independent instance from StatsdSink's

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// NewDogStatsdSink resolves udpAddr (a host:port literal), dials a connected UDP
// socket (a SECOND *net.UDPConn in the tree, independent of any StatsdSink's),
// and returns a ready sink. A resolve/dial error is returned verbatim (->
// main.go log.Fatalf, the StatsdSink-error precedent).
func NewDogStatsdSink(udpAddr string, prefix string) (*DogStatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve dog_statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial dog_statsd udp %q: %w", udpAddr, err)
	}
	return &DogStatsdSink{conn: conn, prefix: prefix, delta: newDeltaState()}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then, per family, extracts the residual name + tags via
// stats.ExtractTags and writes ONE UDP datagram. Called serially by the Flusher.
func (s *DogStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	for _, fam := range batch {
		var suffix string
		switch fam.GetType() {
		case dto.MetricType_COUNTER:
			suffix = "|c"
		case dto.MetricType_GAUGE:
			suffix = "|g"
		default:
			continue // no other family type exists (no histograms — ADR-0060)
		}
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			// Defensive: can't happen for a registered name (the label.go labelMapper
			// precedent) — fall back to the full untransformed name, no tags.
			residual, labels = fam.GetName(), nil
		}
		name := s.prefix + "." + residual
		tagSuffix := formatTagSuffix(labels)
		for _, m := range fam.GetMetric() {
			var v float64
			if fam.GetType() == dto.MetricType_GAUGE {
				v = m.GetGauge().GetValue()
			} else {
				v = m.GetCounter().GetValue()
			}
			line := name + ":" + strconv.FormatInt(int64(v), 10) + suffix + tagSuffix
			s.write(line)
		}
	}
}

// formatTagSuffix builds the inline "|#tag1:val1,tag2:val2" suffix from labels
// IN THEIR NATURAL (unsorted) ORDER — no sort.Slice (D-DSD-TAGS-ORDER; CONTRAST
// labelMapper.apply in label.go, which sorts because LabelPair order is
// immaterial to a structured Prometheus label, unlike this literal wire string).
// Returns "" when labels is empty (no |# suffix at all — AMEND-DSD-NAME-CONFIRMED).
func formatTagSuffix(labels []stats.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("|#")
	for i, l := range labels {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("envoy.")
		b.WriteString(strings.TrimPrefix(l.Key, "envoy_"))
		b.WriteByte(':')
		b.WriteString(l.Value)
	}
	return b.String()
}

// write sends one DogStatsd line as one UDP datagram. A Write error is
// rate-limit-logged (at most once per second — the accesslog lastDropLog idiom)
// and dropped.
func (s *DogStatsdSink) write(line string) {
	if _, err := s.conn.Write([]byte(line)); err != nil {
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: dog_statsd udp write failed, dropping line: %v", err)
		}
	}
}

// Close closes the UDP socket. Idempotent via sync.Once.
func (s *DogStatsdSink) Close() error {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			s.closeErr = s.conn.Close()
		}
	})
	return s.closeErr
}
```
NOTE: `dropLogIntervalNanos` is already defined in `sink.go:47` (same package) — reuse it, do NOT redeclare. `stats.Label` (the type `internal/stats/name.go` exports — `type Label struct{ Key, Value string }`) is the parameter type of `formatTagSuffix`; confirm the exact exported name via `grep -n 'type Label' internal/stats/name.go` (it is `Label`, unqualified, per the file already read at PLAN time).

- [ ] **Step 4: Run to verify they pass + full-package race** — `go test ./internal/statssink/ -run 'TestDogStatsdSink' -count=1` ⇒ PASS; then `go test ./internal/statssink/ -race -count=1` (FULL package — the `Flusher` ticker remains a background mutator, now feeding a SECOND periodic UDP sink) ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/dogstatsd.go internal/statssink/dogstatsd_test.go
git commit -m "phase 49 Task 4: DogStatsdSink — a SECOND *net.UDPConn writer + <prefix>.<residual>:<v>|c/|g[|#tags] mapping over a SECOND sink-private always-on deltaState + stats.ExtractTags in natural/unsorted order (synchronous Submit, idempotent Close; ADR-0266, D-DSD-DELTA/NAME/TAGS-ORDER/LINE/LIFECYCLE)"
```

---

## Task 5: Boot wiring — the dog_statsd build loop + the flusher-gate generalization (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `bs.DogStatsdSinkConfigs` (Task 2); `statssink.NewDogStatsdSink` (Task 4); the existing `statsSinks []statssink.Sink` (`:191`), `statsFlusher` (`:190`), the flusher build (`:215`), the shutdown defer (`:226-231`).
- Produces: dog_statsd sinks appended to the SAME `statsSinks` slice; the flusher built when ANY of the three sink kinds exists.

- [ ] **Step 1: Generalize the build gate + add the dog_statsd loop.** Change the gate at `:193` and append a third loop after the statsd loop (`:208-214`), before the `NewFlusher` call (`:215`):
```go
	if len(bs.StatsSinkConfigs) > 0 || len(bs.StatsdSinkConfigs) > 0 || len(bs.DogStatsdSinkConfigs) > 0 {
		if len(bs.StatsSinkConfigs) > 0 {
			node := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
			for _, cfg := range bs.StatsSinkConfigs {
				client, err := grpcclient.NewMetricsServiceClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("statssink: metrics_service client for cluster %q: %v", cfg.ClusterName, err)
				}
				statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, node, cfg.ReportCountersAsDeltas, cfg.EmitTagsAsLabels))
			}
		}
		for _, cfg := range bs.StatsdSinkConfigs {
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		// Phase 49 (ADR-0266): the dog_statsd UDP stats sink with tags.
		// NewDogStatsdSink dials a SECOND, independent connected UDP socket; a
		// resolve/dial error is a fatal boot failure (the StatsdSink precedent).
		// Synchronous (no goroutine), so it adds no background mutator to the
		// shutdown drain.
		for _, cfg := range bs.DogStatsdSinkConfigs {
			sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: dog_statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		statsFlusher = statssink.NewFlusher(bs.Stats, bs.FlushInterval, statsSinks)
	}
```
Update the comment block at `:180-189` to note ALL THREE sink kinds now feed `statsSinks`. The shutdown defer (`:226-231`), the post-Freeze `statsFlusher.Start` (`:384-393` — verified at PLAN time: the comment at `:384-386`, `if statsFlusher != nil {` at `:387`, the `go func(){ defer close(flusherDone); statsFlusher.Start(ctx) }()` at `:391`, the `else { close(flusherDone) }` at `:392-393`; this has drifted ~11 lines since the phase-47/48 citation — re-confirm once more at edit time since Tasks 2-4 may shift it further), and the `flusherDone` plumbing are UNCHANGED (sink-agnostic).

- [ ] **Step 2: Verify byte-stability + build.** No dog_statsd unit test in `main.go` (exercised end-to-end by `0093`). Confirm the no-sink path is untouched:
```bash
go build ./... && echo BUILD_OK
go vet ./cmd/... && gofmt -l cmd/envoy-go/main.go   # gofmt empty
golangci-lint run ./cmd/...
```
The byte-stability regression anchor is the FULL differential (Task 8) — when `DogStatsdSinkConfigs` is empty (every existing fixture), the gate is unchanged and no second UDP dial/sink happens.

- [ ] **Step 3: Commit**
```bash
git add cmd/envoy-go/main.go
git commit -m "phase 49 Task 5: main.go — dog_statsd sink build loop into the shared statsSinks slice + flusher-gate generalization (three-way OR; ADR-0266)"
```

---

## Task 6: Extend `test/helpers/statsdrecv` — the colon/pipe-split revision + a `Tags()` accessor [TDD, backward-compat regression]

**Files:**
- Modify: `test/helpers/statsdrecv/statsdrecv.go`, `test/helpers/statsdrecv/statsdrecv_test.go`

**Interfaces:**
- Consumes: nothing new (still `net`/`strconv`/`strings`/`sync`).
- Produces: a NEW `func (s *Server) Tags(name string) (map[string]string, bool)`. `DeltaSum`/`Gauge`/`SeenCount`/`Reset`/`Addr`/`Close`/`NewAtAddr` are UNCHANGED signatures. (Task 7's `0093` driver consumes `Tags`.)

The CURRENT `ingest` (`statsdrecv.go:79-111`) locates the name/value boundary via `colon := strings.LastIndexByte(line, ':')` (`:87`) — correct ONLY because a tagless statsd line has exactly one colon. A DogStatsd line's `|#` tag suffix ITSELF contains `key:value` pairs, so on a tagged line the LAST colon falls INSIDE the tag suffix, silently misparsing (or dropping) every tagged line. The fix locates the FIRST `|` (neither the name nor the value contains one), splits `name:value` unambiguously from `line[:pipe1]`, then splits `line[pipe1+1:]` on a SECOND `|` (if present) into the `c`/`g` type token and the optional `#tag1:val1,tag2:val2` tag suffix.

- [ ] **Step 1: Write the failing tests** in `statsdrecv_test.go` (ADD to the existing file; do NOT remove the existing tagless test — it is the backward-compatibility regression guard):
  - **regression: existing tagless behavior UNCHANGED** — re-run (or confirm still passing) the EXISTING test that writes `"p.cluster.x.rq_total:7|c"`/`"p.cluster.x.rq_total:0|c"`/`"p.cluster.x.healthy:1|g"` and asserts `DeltaSum`/`Gauge`/`SeenCount`; ADD an assertion that `Tags("p.cluster.x.rq_total")` returns `(nil, false)` (a tagless line never populates a tag set).
  - **a single-tag line**: write `"dsdpfx.cluster.upstream_rq_total:6|c|#envoy.cluster_name:backend"` then `"dsdpfx.cluster.upstream_rq_total:1|c|#envoy.cluster_name:backend"`; poll until `SeenCount(...) == 2`; assert `DeltaSum(...) == 7`; assert `Tags("dsdpfx.cluster.upstream_rq_total") == map[string]string{"envoy.cluster_name": "backend"}` with `ok == true`.
  - **a two-tag line (comma-joined, order as received)**: write `"dsdpfx.http.downstream_rq_xx:5|c|#envoy.response_code_class:2,envoy.http_conn_manager_prefix:hcm_local"`; assert `Tags(...) == map[string]string{"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": "hcm_local"}` (map equality — order-independent by construction).
  - **a tagged gauge**: write `"dsdpfx.cluster.membership_total:1|g|#envoy.cluster_name:backend"`; assert `Gauge(...) == 1` with `ok`, AND `Tags(...) == map[string]string{"envoy.cluster_name": "backend"}`.
  - **an absent name** ⇒ `Tags(...)` returns `(nil, false)`.
  - **`Reset()` clears tags too** — after `Reset()`, `Tags(name)` for a previously-tagged name returns `(nil, false)`.
  - **malformed tag pair is skipped, not fatal** — a datagram with `"...:1|c|#malformed"` (no colon in the tag segment) does not panic; `Tags(name)` returns an EMPTY (not nil) map for that entry, or `(nil,false)` — either is acceptable; assert no panic and the counter's `DeltaSum`/`SeenCount` still update normally.

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ FAIL (`Tags` undefined; the tagged-line tests likely mis-parse under the current `ingest`).

- [ ] **Step 3: Implement** the revision in `statsdrecv.go`:
  - Add a `tags map[string]map[string]string` field to `Server` (beside `deltaSums`/`gauges`/`seen`), initialize it in `NewAtAddr`, clear it in `Reset`.
  - Replace `ingest` (`:79-111`):
```go
// ingest parses each newline-delimited DogStatsd/statsd line in one datagram
// (<name>:<value>|<type>[|#tag1:val1,...]) and updates the accumulators.
// Malformed lines/segments are skipped, never fatal. REVISED (phase 49) from a
// last-colon split (correct only for a tagless statsd line) to a first-pipe-
// then-colon split: neither name nor value contains '|', so the FIRST '|'
// unambiguously separates "name:value" from "type[|#tags]" — a tagged line's
// tag suffix contains its OWN colons, which the old last-colon split mis-took
// for the name/value boundary. This degenerates to the EXACT prior behavior on
// a tagless line (line-parser-extension delimiter-reuse gotcha).
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pipe1 := strings.IndexByte(line, '|')
		if pipe1 < 0 {
			continue
		}
		head := line[:pipe1] // "name:value" — no '|' precedes here
		colon := strings.LastIndexByte(head, ':')
		if colon < 0 {
			continue
		}
		name := head[:colon]
		val, err := strconv.ParseFloat(head[colon+1:], 64)
		if err != nil {
			continue
		}
		rest := line[pipe1+1:] // "type[|#tag1:val1,...]"
		typ := rest
		var lineTags map[string]string
		if pipe2 := strings.IndexByte(rest, '|'); pipe2 >= 0 {
			typ = rest[:pipe2]
			tagPart := strings.TrimPrefix(rest[pipe2+1:], "#")
			lineTags = make(map[string]string)
			for _, pair := range strings.Split(tagPart, ",") {
				if c := strings.IndexByte(pair, ':'); c >= 0 {
					lineTags[pair[:c]] = pair[c+1:]
				}
			}
		}
		switch typ {
		case "c":
			s.deltaSums[name] += val
			s.seen[name]++
		case "g":
			s.gauges[name] = val
			s.seen[name]++
		default:
			continue
		}
		if lineTags != nil {
			s.tags[name] = lineTags
		}
	}
}

// Tags returns the last-seen tag set for name (from its most recent |# suffix),
// and ok=false if name was never seen with a tag suffix (a tagless line, or no
// datagram for name at all).
func (s *Server) Tags(name string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tags[name]
	return t, ok
}
```
  Update `NewAtAddr` to initialize `tags: make(map[string]map[string]string)`, and `Reset` to reassign `s.tags = make(map[string]map[string]string)`.

- [ ] **Step 4: Run to verify they pass + race** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ PASS (INCLUDING the pre-existing tagless test — the regression guard); `go test ./test/helpers/statsdrecv/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/statsdrecv/ && golangci-lint run ./test/helpers/statsdrecv/... && go vet ./test/helpers/statsdrecv/... && go build ./...
git add test/helpers/statsdrecv/
git commit -m "phase 49 Task 6: test/helpers/statsdrecv — revise ingest to a first-pipe-then-colon split (a last-colon split mis-parses a tagged line) + a NEW Tags(name) accessor; backward-compatible with 0092's tagless usage (reference_line_parser_extension_delimiter_reuse, D-DSD-RECEIVER-WIRING)"
```

---

## Task 7: The `0093-stats-sink-dogstatsd` differential fixture (driver + YAMLs + expectations + README) + register in the runner

**Files:**
- Create: `test/fixtures/0093-stats-sink-dogstatsd/driver/driver.go`, `.../envoy.yaml`, `.../envoy-go.yaml`, `.../expectations.yaml`, `.../README.md`
- Modify: `test/differential/runner_test.go` (blank-import the `0093` driver)

**Interfaces:**
- Consumes: `statsdrecv.{NewAtAddr,DeltaSum,Gauge,SeenCount,Tags,Addr,Close}` (Task 6); the dog_statsd parse arm + sink + main wiring (Tasks 2/4/5); the `fixture.{Driver,BackendKindAware,StatsAsserter}` interfaces + `fixture.RegisterFixture` (the `0092` driver shows the exact surface, `test/fixtures/0092-stats-sink-statsd/driver/driver.go`, 628 LoC — READ IT FIRST, this task clones it).

Clone `test/fixtures/0092-stats-sink-statsd/driver/driver.go` verbatim, then apply these diffs:

- [ ] **Step 1: `driver.go` constants + subset (the WIRE-name correction is the load-bearing diff vs `0092`):**
```go
const (
	fixtureName = "0093-stats-sink-dogstatsd"

	refAdminPort    = 9901
	refListenerPort = 10093 // fixture 0093 takes 10093 per the "100NN" convention

	numReq = 7

	probePath = "/probe"
	probeHost = "dogstatsd.example"
	probeUA   = "dogstatsd-probe/1"

	statPrefix  = "hcm_local"
	backendName = "c_backend"

	// The dog_statsd metric prefix baked identically on both sides. DISTINCT
	// from the 0092 statsd fixture's "sdpfx" (a different sink, coexistence not
	// tested here, but the prefixes must not collide if both were ever combined).
	prefix = "dsdpfx"

	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// subsetNames is the deterministic COUNTER WIRE (post-stats.ExtractTags) name
// subset — NOT the raw pre-extraction dotted names 0092 uses. dog_statsd applies
// ExtractTags (0092's plain statsd sink does not), so the residual + prefix join
// differs from 0092's for the "same" underlying stat:
//   cluster.<backendName>.upstream_rq_total  -> wire "cluster.upstream_rq_total"   (backendName HOISTED to a tag)
//   http.<statPrefix>.downstream_rq_total    -> wire "http.downstream_rq_total"    (statPrefix HOISTED to a tag)
//   http.<statPrefix>.downstream_rq_2xx      -> wire "http.downstream_rq_xx"       (SN4 REWRITES _2xx->_xx + hoists the digit)
// CONFIRMED at the SPEC-49 §11 live probe: "probepfx.http.downstream_rq_xx:...|c|#envoy.response_code_class:2,...".
var subsetNames = []string{
	prefix + ".cluster.upstream_rq_total",
	prefix + ".http.downstream_rq_total",
	prefix + ".http.downstream_rq_xx",
}

// subsetTags is the expected extracted tag SET (order-independent map equality
// — the differential does NOT depend on the wire's tag ORDER even though the
// production formatter now matches the reference's natural order) per subset name.
var subsetTags = map[string]map[string]string{
	prefix + ".cluster.upstream_rq_total": {"envoy.cluster_name": backendName},
	prefix + ".http.downstream_rq_total":  {"envoy.http_conn_manager_prefix": statPrefix},
	prefix + ".http.downstream_rq_xx":     {"envoy.response_code_class": "2", "envoy.http_conn_manager_prefix": statPrefix},
}

// gaugeName + gaugeTags: the absolute |g subset (D-DSD-GAUGE-SUBSET), mirroring
// 0092's membership_total-not-membership_healthy precedent
// (reference_membership_total_vs_healthy_gauge — the 0093 cluster, cloned from
// 0092, carries no health_checks).
var gaugeName = prefix + ".cluster.membership_total"
var gaugeTags = map[string]string{"envoy.cluster_name": backendName}
```

- [ ] **Step 2: `sideSnapshot` + `driveSide` diffs (add tag capture):**
```go
type sideSnapshot struct {
	sums      map[string]float64
	tags      map[string]map[string]string
	gaugeVal  float64
	gaugeOK   bool
	gaugeTags map[string]string
}
```
In `driveSide`, after the stability barrier (`awaitFurtherFlushes(ctx, srv, subsetNames[0], 2)`, UNCHANGED):
```go
	snap := sideSnapshot{
		sums: make(map[string]float64, len(subsetNames)),
		tags: make(map[string]map[string]string, len(subsetNames)),
	}
	for _, name := range subsetNames {
		sum, _ := srv.DeltaSum(name)
		snap.sums[name] = sum
		tags, _ := srv.Tags(name)
		snap.tags[name] = tags
	}
	snap.gaugeVal, snap.gaugeOK = srv.Gauge(gaugeName)
	snap.gaugeTags, _ = srv.Tags(gaugeName)
	return b.Bytes(), snap, nil
```
`pollSubset`/`subsetConverged`/`describeSubset`/`awaitFurtherFlushes`/`fireProbe`/`mustAllocateUDPPort`/`mustStartReceiver`/`ensure`/`closeServers`/`ReferenceListenerPort`/`SubjectListenerName`/`BackendCount`/`BackendKind`/`ProbeAdmin`/`fixtureDir`/`mustReadFixtureFile`/`mustRender` are UNCHANGED from `0092` (copy verbatim, only the receiver type stays `*statsdrecv.Server` — SAME package, extended in Task 6).

- [ ] **Step 3: `ReferenceBootstrap`/`SubjectConfig` diffs (rename template keys, swap the sink TypeURL — the rendered YAML keys are otherwise IDENTICAL to 0092's):**
```go
func (d *statsdDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	gwIP, err := hostGatewayIP(context.Background())
	if err != nil {
		panic(fmt.Sprintf("driver: hostGatewayIP: %v", err))
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"DogStatsdHost": gwIP,
		"DogStatsdPort": d.refStatsdPort,
		"Prefix":       prefix,
		"StatPrefix":   statPrefix,
		"BackendName":  backendName,
	})
}

func (d *statsdDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     subjAdminPort,
		"ListenerPort":  subjListenerPort,
		"BackendPort":   backendPorts[0],
		"DogStatsdHost": "127.0.0.1",
		"DogStatsdPort": d.subjStatsdPort,
		"Prefix":        prefix,
		"StatPrefix":    statPrefix,
		"BackendName":   backendName,
	})
}
```
(Struct field names `refStatsdPort`/`subjStatsdPort`/`refSrv`/`subjSrv` on `statsdDriver` are UNCHANGED from `0092` — reusing the exact receiver-lifecycle plumbing; only the TEMPLATE KEYS are renamed `StatsdHost/Port` → `DogStatsdHost/Port` for readability, not required for correctness.)

- [ ] **Step 4: the `hostGatewayIP` local duplicate (Global Constraints note — copy `0092`'s exactly, unmodified):** copy `test/fixtures/0092-stats-sink-statsd/driver/driver.go:508-589` (the FULL `hostGatewayIP` function — doc comment through the closing brace and final `return ip, nil`; a truncated copy stopping at `:586` cuts off the return statement), AND its full import list — `context`, `bytes`, `io`, `net`, `strings`, `dockertypes "github.com/docker/docker/api/types"`, `"github.com/docker/docker/api/types/container"`, `"github.com/testcontainers/testcontainers-go"`) VERBATIM into the `0093` driver. Do NOT attempt to import `test/differential` and call `differential.HostGatewayIP` — this is the exact cycle the `0092` driver already avoided (`runner_test.go` blank-imports the driver FROM WITHIN `package differential`).

- [ ] **Step 5: `AssertStats`/`assertSide` diffs (add tag-set assertions):**
```go
func assertSide(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()

	if len(snap.sums) == 0 {
		t.Fatalf("%s: no metric counters captured (decode did not run)", side)
	}
	for _, name := range subsetNames {
		sum, present := snap.sums[name]
		if !present {
			t.Fatalf("%s: COUNTER subset %q absent (decode did not run for it)", side, name)
		}
		if sum != float64(numReq) {
			t.Fatalf("%s: counter %q delta-sum = %v, want %d (== K)", side, name, sum, numReq)
		}
		wantTags := subsetTags[name]
		gotTags := snap.tags[name]
		if !maps.Equal(gotTags, wantTags) {
			t.Fatalf("%s: counter %q tags = %v, want %v", side, name, gotTags, wantTags)
		}
	}

	if !snap.gaugeOK {
		t.Fatalf("%s: gauge %q absent (expected membership_total|g)", side, gaugeName)
	}
	if snap.gaugeVal != 1 {
		t.Fatalf("%s: gauge %q = %v, want 1", side, gaugeName, snap.gaugeVal)
	}
	if !maps.Equal(snap.gaugeTags, gaugeTags) {
		t.Fatalf("%s: gauge %q tags = %v, want %v", side, gaugeName, snap.gaugeTags, gaugeTags)
	}
}
```
Add `"maps"` to the import list (Go 1.23 stdlib `maps.Equal` — the module is `go 1.23.0`, confirmed at PLAN time).

- [ ] **Step 6: Write `envoy.yaml`** (clone `0092/envoy.yaml`; swap the sink block + rename the templated `Statsd*` keys to `DogStatsd*`):
```yaml
stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink
      address:
        socket_address: { protocol: UDP, address: {{.DogStatsdHost}}, port_value: {{.DogStatsdPort}} }
      prefix: {{.Prefix}}
stats_flush_interval: 0.5s
```
The listener/route/cluster blocks are IDENTICAL to `0092`'s (H1 single-listener → `HTTPFixedBody` backend, `stat_prefix {{.StatPrefix}}`, cluster `{{.BackendName}}`, `STRICT_DNS`/`host.docker.internal`). Update the file's header comment to describe the dog_statsd sink + the tag-bearing wire format + the SN4 wire-name gotcha (so a future reader doesn't repeat this PLAN's own caught mistake).

- [ ] **Step 7: Write `envoy-go.yaml`** (the subject template — SAME shape, `{{.DogStatsdHost}}` = `127.0.0.1`, `{{.DogStatsdPort}}` = subjStatsdPort, cluster `STATIC`/`127.0.0.1`).

- [ ] **Step 8: Register the driver** in `runner_test.go` (after the `0092` blank-import):
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0093-stats-sink-dogstatsd/driver"
```

- [ ] **Step 9: Run the differential** (live Docker — the controller's host). `reference_differential_run_selector` — NEVER bare `0093`:
```bash
go test ./test/differential/ -run 'TestDifferential/0093' -count=1 -v
```
Expected: PASS; the `-v` output shows the per-side delta-SUMs == 7, the tag sets matching `subsetTags`, and the gauge == 1 with `gaugeTags`. Confirm decode ran (each side's `DeltaSum(subset) == 7` is structurally required to converge; a missing/wrong tag set FAILS the assertion, not just a silent miss).

- [ ] **Step 10: Deliberate breaks** (`reference_differential_break_protocol_count1` — `-count=1` EVERY break; revert after each; verify the main repo is clean + branch undetached — `feedback_subagent_worktree_detach`):
  - **(a) absolute-not-delta** — in `dogstatsd.go` `Submit`, temporarily skip `s.delta.apply` (emit the RAW absolute batch). The stability barrier MUST FAIL: after convergence the next flushes re-add the cumulative, so `DeltaSum` overshoots 7. Run `-run 'TestDifferential/0093' -count=1` ⇒ FAIL. Proves the delta assertion is live. REVERT.
  - **(b) dropped/wrong tags** — temporarily make `formatTagSuffix` return `""` unconditionally (drop ALL tags) OR swap the tag-key prefix (emit `"foo."` instead of `"envoy."`). The `assertSide` tag-set check FAILS (`gotTags` is `nil`/wrong vs `wantTags`). Proves the tag-set assertion is live. REVERT.
  - **(c) barrier-masks-(a)** — re-apply break (a), AND temporarily drop the `awaitFurtherFlushes` call in the driver (snapshot right after `pollSubset`). Verify the differential now PASSES (the first flush's delta == the absolute == 7, invisible without the barrier) — demonstrating the barrier is load-bearing. REVERT both.

- [ ] **Step 11: Flake-stability + full-package race**:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0093' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FAIL"; done   # expect 20/20 PASS
go test ./internal/statssink/ -race -count=1   # full package; the Flusher ticker + a SECOND periodic sink
```
(If a run shows `subject ready: EOF`, isolate-re-run per `reference_differential_fullsuite_startup_flake`.)

- [ ] **Step 12: Write `expectations.yaml` + `README.md`** (clone `0092`'s, adapt the prose to the DogStatsd line-with-tags protocol + the delta-SUM-with-stability-barrier + the tag-SET assertions + the SN4 wire-name gotcha + the UNasserted set: the whole line set / family count, non-deterministic gauges, per-datagram framing, `|ms` timers, literal tag ORDER).

- [ ] **Step 13: Per-task gates + commit**
```bash
gofmt -l test/fixtures/0093-stats-sink-dogstatsd/ test/differential/runner_test.go && golangci-lint run ./test/... && go vet ./test/... && go build ./...
git add test/fixtures/0093-stats-sink-dogstatsd/ test/differential/runner_test.go
git commit -m "phase 49 Task 7: 0093-stats-sink-dogstatsd differential — cross-side delta-SUM-with-stability-barrier + tag-SET assertions (3-counter subset, WIRE post-ExtractTags names) + absolute-gauge-plus-tag subset over the dog_statsd UDP sink; two per-side extended statsdrecv receivers + a local hostGatewayIP duplicate (0092 precedent); breaks (a)(b)(c) + 20/20 flake + full-package race (ADR-0266, D-DSD-DELTA/TAGS/GAUGE-SUBSET)"
```

---

## Task 8: The +0 stat-surface guard (D-DSD-STATS-FINAL) + the full differential + the six-gate

**Files:**
- Modify (only if the registration test needs a dog_statsd note): `internal/statssink/registration_test.go` (`TestNoNewStat_StatsdRegistrationGuard` — or the equivalent surface-count test, re-confirm via `grep -rln 'func Test.*[Rr]egistration\|1196\|1200' internal/`)

**Interfaces:** none new — this task PROVES the surface is unchanged + the suite is green.

- [ ] **Step 1: Confirm the +0 surface.** Locate the stat-surface registration test. Run it: it MUST PASS UNCHANGED (the dog_statsd sink registers NO self-stat; the UDP sink dials no cluster — D-DSD-STATS-FINAL).
```bash
go test ./internal/bootstrap/ ./internal/stats/ -count=1   # surface tests PASS, count unchanged 1200 / non-H2 1196
```

- [ ] **Step 2: The full differential** (live Docker; 95 dirs — `reference_differential_fullsuite_startup_flake`: a transient `subject ready: EOF` on an UNRELATED dir is a startup race — isolate-re-run that dir, then re-run full):
```bash
go test ./test/differential/ -count=1 2>&1 | tail -30   # expect ok (all 95 fixtures incl. 0093)
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

- [ ] **Step 4: Commit** (only if Step 1 touched the registration test; otherwise verification-only, folded into Task 9's commit):
```bash
git add internal/statssink/registration_test.go 2>/dev/null || true
git commit -m "phase 49 Task 8: confirm +0 stat surface (1200 / non-H2 1196 unchanged) — no dog_statsd self-stat, no sink cluster (D-DSD-STATS-FINAL)" 2>/dev/null || echo "no registration change — verification-only"
```

---

## Task 9: ADR-0266 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + the fuzzer-count reconcile

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0266 §Decision + §Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/49-stats-sink-dogstatsd/PROGRESS-49.md`

- [ ] **Step 1: ADR-0266 body** — append §Decision + §Consequences to the ADR-0266 §Context already drafted in SPEC-49 §13 (copy the §Context into DECISIONS.md if not already there, then add): §Decision — lift the `DogStatsdSink` TypeURL into a three-arm `stats_sinks[]` dispatch + `parseDogStatsdSinkConfig` → `DogStatsdSinkConfig{UDPAddress, Prefix}`; the `internal/statssink/dogstatsd.go` `DogStatsdSink` (a SECOND independent `*net.UDPConn` writer + the `<prefix>.<residual>:<v>|c`/`|g[|#tags]` mapping over a SECOND sink-private always-on `deltaState`, tags via `stats.ExtractTags` in natural/unsorted order, synchronous `Submit`, idempotent `Close`); the `main.go` third build loop; STRICT-REJECT an explicit `max_bytes_per_datagram` (the reference honors it with real batching) + sibling TypeURLs (naming all three sinks) + REFERENCE-PARITY reject a missing `dog_statsd_specifier`; the extended `test/helpers/statsdrecv` (`Tags()` + the colon/pipe-split fix) + `0093` (delta-SUM + stability barrier + tag-set + absolute-gauge-plus-tag subset); +0 stat surface; ZERO new packages/modules. §Consequences — the THIRD `stats_sinks[]` consumer + SECOND UDP datagram seam; the always-on delta (no knob, independent instance per sink); the natural/unsorted tag order (a departure from the `labelMapper`'s sorted convention, justified by reference-parity); `max_bytes_per_datagram`/graphite/OTLP-metrics + `|ms` timers deferred; the family STAYS OPEN.

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — add the `### Stats sinks — the dog_statsd UDP sink with tags` subsection (under the phase-48 statsd section) per SPEC §9: a bootstrap `stats_sinks[]` `dog_statsd` entry with a UDP `address` → dial a SECOND UDP socket + flush one DogStatsd line per metric per UDP datagram (`<prefix>.<residual>:<delta>|c[|#tags]` counters / `<prefix>.<residual>:<abs>|g[|#tags]` gauges, tags via `stats.ExtractTags` — the SAME core the metrics_service `emit_tags_as_labels` knob uses — formatted `envoy.<key>:<value>` comma-joined in natural/unsorted order, no `|#` suffix when untagged, default prefix `envoy`); STRICT-REJECT an explicit `max_bytes_per_datagram` + sibling/unknown sink TypeURL; REFERENCE-PARITY reject a missing `dog_statsd_specifier`; `log.Fatalf` on an unresolvable UDP address; byte-identical when no dog_statsd sink is configured; stat surface stays 1200 (+0).

- [ ] **Step 3: STATE.md** — roll the active-phase header to `phase 49 (stats-sink-dogstatsd) IMPL done` (row 49 `done`); update the counts to stat **1200** / fixtures **95** / fuzzers **52** / BackendKind **38** / DECISIONS **ADR-0266** (next-free ADR-0267); set NEXT to "none chartered" (the loop re-opens via the router / next BRAINSTORM).

- [ ] **Step 4: ROADMAP.md** — flip row 49 (`stats-sink-dogstatsd`) to **`done`** (the sole leg — ADR-0106; no parent rollup); keep the Observability family note OPEN (remaining deferred candidates: `max_bytes_per_datagram` batching / `graphite` / OTLP-metrics sinks / the plain-statsd `tcp_cluster_name` transport / tracing extras / the tap filter).

- [ ] **Step 5: PROGRESS-49.md** — mark all tasks complete; record the FINAL counts (re-run the Task-1 baseline commands and paste): fixtures **95**, fuzzers **52** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **52** — reconciling 51 → 52), stat **1200**, BackendKind **38**, `go mod tidy -diff` EMPTY.

- [ ] **Step 6: Final full suite on the frozen HEAD + commit**:
```bash
go build ./... && go test ./... -count=1 && grep -rh '^func Fuzz' --include='*.go' . | wc -l   # ALL PASS; fuzzers == 52
git add docs/envoy-go/
git commit -m "phase 49 Task 9: ADR-0266 §Decision/§Consequences + BEHAVIOR_CONTRACT dog_statsd subsection + STATE/ROADMAP (row 49 done) + PROGRESS close + fuzzer-count reconcile (51 -> 52) — dog_statsd UDP stats sink with tags COMPLETE"
```

---

## Self-Review (run before declaring the plan ready)

- **Spec coverage:** SPEC §3 (parse arm + struct + dispatch + main wiring) → Tasks 2/5; §3.3 (DogStatsdSink + delta + tag formatting + synchronous Submit + idempotent Close) → Task 4; §5 (proto roster) → Task 2; §6 (reject arms + fuzzer) → Tasks 2/3; §7 (+0 surface) → Task 8; §8 (0093 + breaks + receiver extension + BackendKind 38) → Tasks 6/7; §9 (behavior contract) → Task 9; §11 D-DSD-* pins → honored in Tasks 2/4/7; §12 D-DSD-* PLAN questions → resolved in the D-question block above; §13 (ADR-0266) → Task 9. All covered.
- **The SN4 wire-name gotcha:** caught DURING this PLAN's own authoring (before any test code was written against the wrong name) — a follow-up correction commit fixed SPEC-49.md's §8.1/§11 D-DSD-SUBJECT text FIRST, then this PLAN's Task 7 `subsetNames`/`subsetTags` were written against the CORRECTED wire name (`http.downstream_rq_xx`, not `http.downstream_rq_2xx`). Flagged explicitly in the Global Constraints + Task 7 Step 1 comment so a PLAN reviewer / IMPL executor doesn't silently regress it.
- **The `hostGatewayIP` import-cycle gotcha:** the SPEC's D-DSD-RECEIVER-WIRING said "reuse `differential.HostGatewayIP` verbatim" — this PLAN CORRECTS that to "duplicate the `0092` driver's LOCAL copy," having verified by reading the ACTUAL landed `0092` driver code (not just the SPEC's prose) that `differential.HostGatewayIP` is exported-but-unused-by-the-driver, precisely BECAUSE of the blank-import direction (`runner_test.go`, `package differential`, blank-imports the driver — so the driver importing `differential` back would risk a cycle). Task 7 Step 4 makes this explicit.
- **Placeholder scan:** no vague placeholders — the `0092` driver (628 LoC, fully read) is the concrete template for every Task-7 diff; the exact registration-test name (Task 8) is located via `grep` rather than assumed.
- **Type consistency:** `DogStatsdSinkConfig{UDPAddress, Prefix}` (Task 2) ↔ `NewDogStatsdSink(udpAddr, prefix)` (Task 4) ↔ `main.go` loop (Task 5); `statsdrecv.{DeltaSum,Gauge,SeenCount,Tags}` (Task 6) ↔ the `0093` driver (Task 7); the `Sink` interface satisfied by `DogStatsdSink` (Submit/Close). Consistent.

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/49-stats-sink-dogstatsd/PLAN-49.md`.** Per the router (and `feedback_execution_style` + `feedback_git_worktrees`), the phase-49 IMPL is subagent-driven in a FRESH worktree off master; subagents commit locally only; the controller verifies each commit + re-runs the full suite on the frozen HEAD + does the deliberate-break verification ITSELF + squashes + pushes at stage-close. The next router stage (after this PLAN lands + a `plan-document-reviewer` pass) is the phase-49 IMPL.
