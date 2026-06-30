# Phase 48 Implementation Plan — the `statsd` UDP stats sink: LIFT the `envoy.config.metrics.v3.StatsdSink` TypeURL from the `bootstrap.go:430` sibling-reject into a TWO-arm dispatch + a `parseStatsdSinkConfig` arm → a `StatsdSinkConfig{UDPAddress, Prefix}` + a NEW `internal/statssink/statsd.go` `StatsdSink` (a `*net.UDPConn` writer + the `<prefix>.<name>:<value>|c`/`|g` statsd-line mapping, counters ALWAYS-delta over a sink-private `deltaState`, gauges absolute) over the LANDED phase-47 `Flusher`/`Sink` substrate + the `cmd/envoy-go/main.go` sink-slice fan-out + `FuzzStatsdSinkConfigParse` + a `differential.HostGatewayIP` harness helper + the driver-owned `test/helpers/statsdrecv` UDP receiver + the `0092-stats-sink-statsd` cross-side delta-SUM-with-stability-barrier differential — a SINGLE FLAT ROW, UDP-only; the FIFTH Observability-family row; ZERO new packages, ZERO new go.mod modules; ANCHORS ADR-0265; row 48 flips `done` at this IMPL six-gate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** When the bootstrap carries a `statsd` `stats_sinks[]` entry with a UDP `address`, envoy-go dials that UDP address once (`net.DialUDP` — the FIRST `*net.UDPConn` in the tree) and every `stats_flush_interval` (default 5s) snapshots the frozen process-global `stats.Registry` (the SAME `snapshot()` the metrics_service sink uses) and writes one statsd line per metric as a UDP datagram: `<prefix>.<name>:<delta>|c` for each COUNTER family (the per-flush delta over a sink-private always-on `deltaState`), `<prefix>.<name>:<abs>|g` for each GAUGE family (absolute), the full dotted internal name with tags inlined, ZERO labels, default prefix `envoy`. Proven cross-side on a deterministic COUNTER name-subset (the delta-SUM + a post-convergence stability barrier) + an absolute-gauge subset against `contrib-v1.37.2` by the `0092` differential through a driver-owned UDP receiver. **ANCHORS ADR-0265** (its §Decision/§Consequences body lands atomically here); ROADMAP row 48 (`stats-sink-statsd`) flips **`done`** at this IMPL six-gate (the sole leg — ADR-0106; NO parent rollup); the Observability family STAYS OPEN.

**Architecture:** ONE new `Sink` impl (`internal/statssink/statsd.go`) — a `*net.UDPConn` writer + the `Counter`/`Gauge` → statsd-line mapping over a sink-private always-on `deltaState` (reusing the landed `delta.go` `newDeltaState()`/`apply()` VERBATIM: counters→delta, gauges→absolute) — SYNCHRONOUS per-flush `Write` (no channel, no writer goroutine: a UDP `Write` never blocks on a peer and the `Flusher` calls `Submit` serially), with an idempotent `sync.Once`-guarded `Close`. PLUS a NEW bootstrap parse arm: the `statsdSinkTypeURL` constant (descriptor-derived), the `bootstrap.go:430` single-URL gate replaced by a TWO-arm dispatch, `parseStatsdSinkConfig` → a `StatsdSinkConfig{UDPAddress, Prefix}` appended to a NEW `result.StatsdSinkConfigs` slice, the strict-reject arms (`tcp_cluster_name` / sibling-TypeURL / missing-`statsd_specifier`); the `cmd/envoy-go/main.go` second build loop fanning statsd sinks into the SAME `statsSinks []statssink.Sink` slice + the flusher-gate generalization; `FuzzStatsdSinkConfigParse`; a `differential.HostGatewayIP` harness helper + the driver-owned `test/helpers/statsdrecv` UDP receiver; the `0092` differential. Byte-identical and stat-surface-identical when no statsd `stats_sinks[]` entry is configured (every non-sink path untouched; the full differential is the regression anchor; `StatsdSinkConfigs` stays empty, the flusher build gate unchanged).

**Tech Stack:** Go; the EXISTING `internal/statssink` package (one new file `statsd.go`; reuses `flusher.go`/`sink.go`/`mapping.go`/`delta.go` unchanged); `internal/bootstrap` (the `stats_sinks[]` two-arm dispatch + the statsd parse arm + the `config/metrics/v3` blank-import, ALREADY present); `cmd/envoy-go/main.go` (the second sink build loop); the driver-owned `test/helpers/statsdrecv` UDP receiver (the `test/helpers/metricsservice` analog, but UDP); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`, the `0090` two-per-side-receivers + hard-Close precedent). The statsd line protocol is hand-rolled `strconv`/`net` — NO client lib; the `StatsdSink` proto resolves at the already-blank-imported `go-control-plane/envoy v1.32.4` `config/metrics/v3`. **ZERO new go.mod modules, ZERO new packages.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1200** (H2 cluster; non-H2 **1196**) → **1200** (+0 — D-SD-STATS-FINAL); fixtures **93** → **94** (`0092`); fuzzers **50** → **51** (`FuzzStatsdSinkConfigParse`); BackendKind **38** → **38** (driver-owned UDP receiver is NOT a BackendKind); DECISIONS tail **ADR-0264** → **ADR-0265** (next-free ADR-0266); **+0 go.mod modules, +0 packages**.
- **Module path:** `github.com/esalaine/envoy-go`.
- **No new dependency:** `go mod tidy -diff` MUST be EMPTY at every task (the statsd line protocol is `strconv`/`net`; the `StatsdSink` proto resolves at the already-direct go-control-plane).
- **Process anchors:** ADR-0044 (ADR §Decision+§Consequences land at IMPL) · ADR-0045 (sub-split soft gate — escape-valve UNCONSUMED; re-checked at Task 1) · ADR-0080 (strict-reject anti-silent-divergence) · ADR-0064 (`stats_sinks[]` silent-ignore — now a SECOND consumer) · ADR-0060 (no histograms → no `|ms` timers) · ADR-0106 (per-leg rows; row 48 flips `done` here, no parent rollup) · ADR-0265 (this leg — ANCHORED here).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit, every task.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0092'`, NEVER bare `'0092'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): the synchronous `StatsdSink` adds NO background mutator, but the `Flusher` ticker goroutine remains one — the `-race` gate MUST run the FULL `internal/statssink` package.
- **Delta-sink stability barrier** (`reference_delta_sink_differential_stability_barrier`): a single first-reach-K cannot distinguish delta from absolute (the first flush's delta == the absolute value); assert the delta-SUM is STILL K after ≥2 further (zero-delta) flushes.
- **Two per-side receivers + hard Close** (`reference_periodic_sink_differential_two_receivers`): periodic flushes stream for the whole test; one shared receiver cross-contaminates. (UDP is connectionless — no GracefulStop deadlock; `Close()` the `*net.UDPConn`.)
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the UDP receiver is a `test/helpers/statsdrecv` server the proxy WRITES to — NOT a runner BackendKind (stays 38).
- **Docker bridge + literal IP** (`reference_docker_probe_bridge_network`): the statsd sink rejects hostnames (literal-IP-only); the reference reaches the host receiver at the bridge GATEWAY IP (a literal IP) via `HostGatewayIP`; verify decode RAN (receiver datagram count > 0) before trusting a green.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the statsd line `<prefix>.<name>:<value>|<type>` is shared — the §11 live probe is the wire truth.

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding envoy-go's SECOND stats-export sink and the FIRST consumer of the bootstrap `stats_sinks[]` `StatsdSink` TypeURL. The substrate is ALL built at phase 47 — the `stats.Registry.Walk`/`Freeze` contract, the `internal/statssink` `Flusher` (ticker → `snapshot()` → fan to `[]Sink`), the `Sink` interface, the cumulative/no-labels `snapshot()` mapping, the `deltaState` per-flush-delta transform, the `stats_sinks[]`/`stats_flush_interval` parse + `StatsSinkConfig`, and the `cmd/envoy-go/main.go` post-Freeze flusher build + LIFO-drain. Phase 48 adds a bounded delta: one new `Sink` impl (a UDP writer), one config-parse arm (lifting one TypeURL from a reject), one main second-loop, one fuzzer, one harness helper, one receiver helper, one differential. **NO new framework piece, NO new package, NO new module.**

**What ALREADY works (do NOT re-build) — verified at PLAN time (re-confirm line numbers before editing; files evolve):**

- **`internal/statssink/flusher.go`** — `type Flusher struct{ reg *stats.Registry; interval time.Duration; sinks []Sink; nowMs func() int64 }` (`:13`); `NewFlusher(reg, interval, sinks) *Flusher` (`:22`); `Start(ctx)` (`:33` — a `time.NewTicker(interval)` loop, `flushOnce()` per tick, stop on `ctx.Done()`); `flushOnce()` (`:46` — `snapshot(reg, nowMs())` → `for _, s := range f.sinks { s.Submit(batch) }`). **Sink-agnostic — it fans the SAME absolute batch slice to every sink, unchanged.**
- **`internal/statssink/sink.go`** — `type Sink interface{ Submit(batch []*dto.MetricFamily); Close() error }` (`:18`). `MetricsServiceSink` is one impl; the `StatsdSink` is the SECOND. Note the idioms to copy: `closeOnce sync.Once` + `closeErr error` for idempotent `Close` (`:75`/`:140`); `lastDropLog atomic.Int64` + the rate-limited drop-log (`:77`/`:124-132`, `dropLogIntervalNanos = int64(time.Second)` at `:47`).
- **`internal/statssink/delta.go`** — `type deltaState struct{ last map[string]uint64 }`; `newDeltaState() *deltaState` (`:20`); `apply(abs []*dto.MetricFamily) []*dto.MetricFamily` (`:37`) — returns a NEW batch where every COUNTER family carries the per-flush delta `(current − last[name])` (latching `last[name]=current`), GAUGE/other families shared by pointer untransformed. **MUST NOT mutate the input** (the Flusher fans the same slice to all sinks). An absent key reads 0 ⇒ first flush emits `current−0` = the absolute (no special branch). **This contract MATCHES the statsd `|c`-delta / `|g`-absolute split EXACTLY — reuse it VERBATIM, sink-private, ALWAYS-on (no knob; intrinsic to statsd).**
- **`internal/statssink/mapping.go`** — `snapshot(reg *stats.Registry, nowMs int64) []*dto.MetricFamily` — Counter→`COUNTER` absolute, Gauge→`GAUGE`, full dotted `Name()`, ZERO `LabelPair`, `TimestampMs` set, no `Help`. **Reused unchanged** (the `StatsdSink` consumes the SAME `[]*dto.MetricFamily` the `MetricsServiceSink` does).
- **`internal/bootstrap/bootstrap.go`** — the `config/metrics/v3` blank-import is ALREADY present as the named import `metricsconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/metrics/v3"` (`:161`); `metricsServiceTypeURL` is descriptor-derived (`:214`). `parseStatsSinks(bs, result)` (`:415`) sets `result.FlushInterval` (default 5s), rejects `stats_flush_on_admin` (`:422`), then for each `stats_sinks[]` entry: `nil` typed_config → reject (`:427`), `tc.GetTypeUrl() != metricsServiceTypeURL` → reject (`:430`), else `parseMetricsServiceConfig` (`:433`). `parseMetricsServiceConfig(tc *anypb.Any, idx int, result *Bootstrap) error` (`:449`) is the parse-arm shape to mirror. `StatsSinkConfig` struct (`:274`); `Bootstrap.StatsSinkConfigs []StatsSinkConfig` (`:348`) + `Bootstrap.FlushInterval time.Duration` (`:353`).
- **`cmd/envoy-go/main.go`** — the sink build block (`:189-202`): `var statsFlusher *statssink.Flusher` (`:189`); `var statsSinks []statssink.Sink` (`:190`); `flusherDone := make(chan struct{})` (`:191`); `if len(bs.StatsSinkConfigs) > 0 { node := ...; for _, cfg := range bs.StatsSinkConfigs { client, err := grpcclient.NewMetricsServiceClient(...); statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(...)) }; statsFlusher = statssink.NewFlusher(bs.Stats, bs.FlushInterval, statsSinks) }` (`:192-202`). The shutdown defer (`:212-217`) waits `<-flusherDone` then `for _, s := range statsSinks { _ = s.Close() }`. `bs.Stats.Freeze()` (`:365`); the post-Freeze `if statsFlusher != nil { go func(){ defer close(flusherDone); statsFlusher.Start(ctx) }() } else { close(flusherDone) }` (`:373-380`). **All sink-agnostic — generalize the build gate + add a second loop.**
- **`test/helpers/metricsservice/metricsservice.go`** (319 LoC) — the `test/helpers/statsdrecv` template (but gRPC, not UDP): `New(t)`/`NewAtAddr(addr)` (`:94`/`:114`), `newServer(addr)` binds a listener + spawns a reader, accumulators under an `sync.RWMutex`, `FamilySum(name)` running-sum (`:206`), `Messages()` total-messages (`:256`, the stability-barrier counter), `Reset()` (`:279`), `Addr()` (`:293`), `Close()` hard-stop (`:316`). **The statsd receiver mirrors the SHAPE but reads UDP datagrams, not a gRPC stream.**
- **`test/fixtures/0090-stats-sink-metrics-service-deltas/driver/driver.go`** (567 LoC) — **the `0092` driver template** (the delta-SUM + stability-barrier shape). Mirror: TWO per-side receivers on two allocated ports (`ensure`/`mustAllocatePort`/`mustStartReceiver`); `subsetNames` the 3-counter subset; `driveSide` fires K=7 GETs → `pollSubset` (FamilySum==K release barrier) → `awaitFurtherFlushes(srv, 2)` (the stability barrier) → snapshot; `DriveSubject` calls `closeServers()` (hard Close); `AssertStats`/`assertSide` assert delta-SUM==K + type==COUNTER; the `mustReadFixtureFile`/`mustRender` template helpers; the compile-time `fixture.{Driver,BackendKindAware,StatsAsserter}` assertions. **`0092` replaces the gRPC receiver with the UDP receiver, the metrics_service config with the statsd config, and uses `HostGatewayIP` for the reference's literal-IP statsd address.**
- **`test/differential/harness.go`** — `StartReferenceProxy(ctx, pin, bootstrap, listenerPorts...)` (`:105`) sets `hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}` (`:121`) so the reference container reaches the host. The container joins the default Docker `bridge` network. **`HostGatewayIP` is ADDED here.**

**The statsd wire model (§11 D-SD-* — live-probed against `contrib-v1.37.2` 2026-06-30; all pinned in SPEC-48.md):**
- **The line** (D-SD-LINE): `<prefix>.<dotted-name>:<int>|<type>`, ONE statsd line per UDP datagram (657 datagrams, 0 multi-line, max 77 bytes — NOT newline-batched), integer value, no `@rate` sampling, no signed delta-gauge. `<type>` ∈ {`c` counter-delta, `g` gauge-absolute}; the reference also emits `|ms` for histograms — envoy-go has NONE (ADR-0060) ⇒ `|c`/`|g`-only.
- **The value** (D-SD-DELTA — LOAD-BEARING): COUNTER `|c` is the per-flush DELTA-since-last-flush (`upstream_rq_total` emitted `7,0,0,0` across four flushes — SUM==7==K, last-seen 0). GAUGE `|g` is ABSOLUTE (`membership_healthy:1|g`), no sign prefix. ⇒ the `StatsdSink` ALWAYS owns a sink-private `deltaState` (no knob).
- **The name** (D-SD-NAME): the full dotted internal name with tag VALUES inlined (`cluster.backend.upstream_rq_total`), ZERO labels; the prefix join is `prefix + "." + name`; default prefix `envoy`. Cross-side name-compatible.
- **The reject roster** (D-SD-REJECT, 4 variants): the reference REJECTS at load a missing `statsd_specifier` (PGV oneof-required) + a hostname `address` (literal-IP-only, no DNS); BOOTS a `tcp_cluster_name` (statsd-over-TCP — envoy-go STRICT-REJECTS, UDP-only) + a `socket_address` with `protocol:TCP` (and STILL emits UDP — `protocol` IGNORED). ⇒ envoy-go REFERENCE-PARITY on the missing-specifier reject; envoy-go-STRICT on `tcp_cluster_name`; `protocol` accepted-and-ignored.
- **+0 self-stats, no sink cluster** (D-SD-STATS): the reference registers no statsd-scoped self-stat; the UDP sink dials no cluster (no incidental `upstream_cx_*`). Surface delta +0.

### Proto facts (verified at PLAN time; `go-control-plane/envoy v1.32.4` already direct; NO new module)

- **`metricsconfigv3.StatsdSink`** (`config/metrics/v3/stats.pb.go:571`; the SAME package already named-imported in `bootstrap.go:161`). The TypeURL is `type.googleapis.com/envoy.config.metrics.v3.StatsdSink` (SPEC §11 live-verified; **DERIVE via the proto descriptor, NOT hard-code** — `reference_network_filter_typeurl_extensions`). Accessors: `GetAddress() *corev3.Address` (field 1, oneof `statsd_specifier`); `GetTcpClusterName() string` (field 2, oneof `statsd_specifier`); `GetPrefix() string` (field 3). `GetAddress()` returns nil for BOTH a missing oneof AND a `tcp_cluster_name` arm.
- **`corev3.Address` → `socket_address`**: `addr.GetSocketAddress() *corev3.SocketAddress` (nil unless the socket_address oneof arm is set); `sa.GetAddress() string` (the IP literal); `sa.GetPortValue() uint32`; `sa.GetProtocol() corev3.SocketAddress_Protocol` (accepted-and-IGNORED — dial UDP regardless).
- `anypb.Any.UnmarshalTo(&metricsconfigv3.StatsdSink{})` — the unmarshal (the `parseMetricsServiceConfig` precedent at `:451`).

---

## D-question resolutions (the SPEC §12 D-SD-* PLAN/IMPL pins — settled here)

**D-SD-SPLIT → NO sub-split (a SINGLE FLAT ROW, 10 tasks).** Anticipated ~200–250 prod LoC: one new `Sink` impl (`statsd.go`, ~80 LoC) + the parse arm (~40 LoC) + the main second-loop (~12 LoC) + the receiver helper (~110 LoC) + the harness helper (~20 LoC). All reuse landed templates (`delta.go`, `sink.go` idioms, the `0090` driver, the `metricsservice` receiver). Well under the ADR-0045 gate; the 48.1/48.2 escape-valve stays UNCONSUMED. Re-confirmed at Task 1 with the real baseline.

**D-SD-CONFIG-HOME → a parallel `StatsdSinkConfigs []StatsdSinkConfig` slice on `internal/bootstrap`** (alongside `StatsSinkConfigs` at `:348`), NOT a unified sink-config interface list. Rationale: the two sink kinds carry disjoint parse-time data (metrics_service: `ClusterName`/`ReportCountersAsDeltas`/`EmitTagsAsLabels`; statsd: `UDPAddress`/`Prefix`), and `main.go` already owns the "build each config kind into the one `statsSinks []Sink`" responsibility — a unified interface would force a premature abstraction over two members. `main.go` runs TWO build loops (metrics_service then statsd) appending into the SAME slice; the `Flusher` is sink-agnostic.

**D-SD-LIFECYCLE-SHAPE → SYNCHRONOUS per-flush `Write` (no channel, no writer goroutine).** A UDP `Write` is fire-and-forget — it never blocks on a peer (no handshake, no flow control), and the `Flusher` calls `Submit` serially from its single goroutine (one flush at a time; the sink contract forbids `Submit` after `Close`). So `Submit` applies the sink-private `deltaState` then writes one datagram per family inline. This is simpler than the `MetricsServiceSink`'s bounded-channel + writer-goroutine (which exists only to absorb a slow/blocking gRPC stream) AND adds NO background mutator (lighter `-race` story — `reference_full_suite_race_after_background_mutator`). A `Write` error is rate-limit-LOGGED-and-DROPPED (the `sink.go` `lastDropLog atomic.Int64` idiom; UDP is lossy by design; NOT counted — +0 self-stats).

**D-SD-DELTA-REUSE → reuse the landed `delta.go` `newDeltaState()`/`apply()` VERBATIM (same package, unexported), sink-private, ALWAYS-on.** The `apply` contract (COUNTER→per-flush delta, GAUGE/other→absolute pass-through) MATCHES the statsd `|c`/`|g` split EXACTLY. No statsd-local copy, no config knob (UNLIKE the metrics_service `report_counters_as_deltas` knob — statsd's `|c`-is-delta is intrinsic). The `StatsdSink` holds `delta *deltaState` (always non-nil) and calls `s.delta.apply(batch)` first in `Submit`.

**D-SD-ADDR → `UDPAddress string` (a `host:port` literal), resolved at `NewStatsdSink` via `net.ResolveUDPAddr`.** `parseStatsdSinkConfig` builds the string `fmt.Sprintf("%s:%d", host, port)` (NO new bootstrap import — `fmt` is present); `NewStatsdSink` resolves it via `net.ResolveUDPAddr("udp", udpAddr)` + `net.DialUDP("udp", nil, raddr)`. envoy-go does NOT enforce literal-IP at parse (the reference's hostname-reject is a reference-internal PGV behavior, not cross-side-informative; `net.ResolveUDPAddr` accepts an IP literal and benignly resolves a hostname — the differential uses a literal IP either way, so the distinction is moot). A resolve/dial error surfaces at `NewStatsdSink` → `main.go` `log.Fatalf` (the metrics_service-client-error precedent).

**D-SD-RECEIVER-WIRING → a `differential.HostGatewayIP(ctx)` harness helper (Docker-inspect the `bridge` network gateway) gives the driver a LITERAL IP to bake as the reference's statsd address; the receiver binds `0.0.0.0:<port>`.** The statsd sink rejects hostnames, so the reference CANNOT use `host.docker.internal` for the statsd address (it CAN for the HTTP backend). But the reference container already routes to the host via `host.docker.internal:host-gateway` — and that gateway IS the host's IP on the Docker bridge (a literal IP, e.g. `172.17.0.1`). A host process bound `0.0.0.0:<port>` is reachable from the container at that gateway IP. `HostGatewayIP` inspects the `bridge` network's IPAM gateway and returns it; the driver bakes `<gatewayIP>:<refStatsdPort>` as the reference statsd address. The subject (in-process on the host) uses `127.0.0.1:<subjStatsdPort>`. TWO per-side receivers, each bound `0.0.0.0` on its own allocated port (the `0090` precedent). **The live reachability is PROVEN at Task 9 (decode-ran: receiver datagram count > 0 on each side); the controller runs the live reference.** Contingency (if the bridge-gateway IP proves unreachable at IMPL — e.g. testcontainers attaches to a non-`bridge` network): fall back to a user-defined bridge with a fixed subnet/gateway via `testcontainers/network.New` + `req.Networks` (the §11 shared-bridge shape) — flagged in Task 9, not the default.

**D-SD-FUZZER → land `FuzzStatsdSinkConfigParse` at 48 (fuzzers 50 → 51).** A no-panic fuzzer over the statsd parse arm (the `FuzzStatsSinkConfigParse` precedent). Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 51` at the completion task, reconciling the documented-vs-actual count (`reference_fuzzer_count_docs_drift` — **50 actual** at baseline, verified in-session).

**D-SD-STATS-FINAL → +0 (NO new statsd-scoped stat names; surface stays 1200 / non-H2 1196).** Matches the reference (+0 self-stats) AND sidesteps the self-referential export subtlety (a self-counter registered pre-Freeze would itself appear in the next flush's batch). `Write`-drops are rate-limit-LOGGED, NOT counted. A registration test (Task 8) asserts the surface is UNCHANGED.

**D-SD-GAUGE-SUBSET → YES, `0092` adds a cross-side absolute-gauge subset assertion (`cluster.<backend>.membership_healthy == 1|g`) alongside the counter delta-SUM.** This makes the `|g` formatting path live cross-side (otherwise only unit tests cover `|g`). `membership_healthy` is deterministically 1 (a single healthy backend host, no health-checking → defaults healthy; §11 confirmed `membership_healthy:1|g`) and cross-side name-equal. The receiver's `Gauge(name)` reads the last-seen `|g` value.

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/statssink/statsd.go` — `type StatsdSink struct{ conn *net.UDPConn; prefix string; delta *deltaState; closeOnce sync.Once; closeErr error; lastDropLog atomic.Int64 }`; `NewStatsdSink(udpAddr, prefix string) (*StatsdSink, error)`; `Submit(batch []*dto.MetricFamily)` (synchronous: `delta.apply` then one datagram per family); `Close() error` (idempotent). Satisfies `Sink`.

**Production (modified):**
- `internal/bootstrap/bootstrap.go` — the `statsdSinkTypeURL` const; the `StatsdSinkConfig` struct + `Bootstrap.StatsdSinkConfigs` field; the `parseStatsSinks` single-URL gate → a two-arm dispatch; `parseStatsdSinkConfig` + the strict-reject arms.
- `cmd/envoy-go/main.go` — the flusher-build gate generalization + the second (statsd) build loop.
- `test/differential/harness.go` — `HostGatewayIP(ctx) (string, error)`.

**Test (created):**
- `internal/statssink/statsd_test.go` (`-race`, table-driven over a fake UDP listener).
- `internal/bootstrap/statsd_fuzz_test.go` (`FuzzStatsdSinkConfigParse`).
- `test/helpers/statsdrecv/statsdrecv.go` (+ `statsdrecv_test.go`).
- `test/fixtures/0092-stats-sink-statsd/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.

**Test (modified):**
- `internal/bootstrap/bootstrap_test.go` (the statsd parse-accept + strict-reject arms).
- `internal/bootstrap/registration_test.go` (the +0 surface guard — confirm UNCHANGED).
- `test/differential/runner_test.go` (blank-import the `0092` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/48-stats-sink-statsd/PROGRESS-48.md`, `docs/envoy-go/DECISIONS.md` (ADR-0265 §Decision/§Consequences — ANCHORS the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 48 flips `done`**).

---

## Task 1: Phase scaffolding — PROGRESS-48.md + baselines + the final ADR-0045 split re-check (D-SD-SPLIT)

**Files:**
- Create: `docs/envoy-go/phases/48-stats-sink-statsd/PROGRESS-48.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-48.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                   # expect 93 (tail 0091-stats-sink-metrics-service-labels)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 50
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go # expect = the BackendKind tail (38)
go mod tidy -diff                                                # expect EMPTY (clean)
grep -rn 'StatsdSinkConfig\|statsdSinkTypeURL\|parseStatsdSinkConfig\|statssink/statsd\|statsdrecv\|HostGatewayIP' internal/ cmd/ test/ --include='*.go'  # expect: NONE (48 introduces them)
grep -c 'metricsServiceTypeURL' internal/bootstrap/bootstrap.go  # expect >=2 (the const + the gate)
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **93** / fuzzers **50** / BackendKind **38** / DECISIONS tail **ADR-0264** (next-free **ADR-0265**).

- [ ] **Step 2: Write the PROGRESS-48.md scaffold** — a header (phase 48 IMPL, the SPEC-48 reference + the "SECOND `stats_sinks[]` consumer + FIRST UDP datagram seam + FIFTH Observability-family row; ANCHORS ADR-0265; row 48 flips `done` at this IMPL" note, the worktree branch), a task checklist mirroring this plan, the baseline block, the **D-SD-SPLIT confirmation (NO sub-split — the escape-valve stays UNCONSUMED; the LoC estimate above)**, and the anticipated exit counts: stat **1200** (+0 — D-SD-STATS-FINAL) / fixtures **94** (`0092-stats-sink-statsd`) / fuzzers **51** (`FuzzStatsdSinkConfigParse`) / BackendKind **38** (driver-owned UDP receiver) / DECISIONS **ADR-0265** / **0 new packages, 0 new go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/48-stats-sink-statsd/PROGRESS-48.md
git commit -m "phase 48 Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (statsd UDP stats sink; ANCHORS ADR-0265; row 48 flips done at this IMPL)"
```

---

## Task 2: The statsd parse arm — `statsdSinkTypeURL` + `StatsdSinkConfig` + the two-arm dispatch + `parseStatsdSinkConfig` + the strict-reject arms (`internal/bootstrap/bootstrap.go`) [TDD]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Test: `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: the existing `parseStatsSinks` (`:415`) + `metricsServiceTypeURL` (`:214`) + `metricsconfigv3` import (`:161`); `anypb.Any` (already imported).
- Produces: `type StatsdSinkConfig struct{ UDPAddress string; Prefix string }`; `Bootstrap.StatsdSinkConfigs []StatsdSinkConfig`; `statsdSinkTypeURL` (var, descriptor-derived); `parseStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error`. (Task 5's `main.go` loop reads `bs.StatsdSinkConfigs`; Task 3's fuzzer drives this arm through `Load`.)

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (mirror the existing metrics_service parse tests — find them via `grep -n 'parseMetricsServiceConfig\|StatsSinkConfigs\|metrics_service' internal/bootstrap/bootstrap_test.go`; reuse the YAML-`Load` harness). Use a shared `head` document (a `node`, `admin`, empty `static_resources` — copy the `FuzzStatsSinkConfigParse` `head`). The statsd TypeURL string is `type.googleapis.com/envoy.config.metrics.v3.StatsdSink`.
  - **accept — UDP socket_address + prefix**:
    ```yaml
    stats_sinks:
      - name: envoy.stat_sinks.statsd
        typed_config:
          "@type": type.googleapis.com/envoy.config.metrics.v3.StatsdSink
          address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
          prefix: myprefix
    ```
    ⇒ `Load` succeeds; `len(bs.StatsdSinkConfigs) == 1`; `bs.StatsdSinkConfigs[0] == StatsdSinkConfig{UDPAddress: "127.0.0.1:8125", Prefix: "myprefix"}`; `len(bs.StatsSinkConfigs) == 0`.
  - **accept — default prefix `envoy`** (no `prefix`): same but omit `prefix` ⇒ `StatsdSinkConfigs[0].Prefix == "envoy"`.
  - **accept — `protocol: TCP` IGNORED** (`socket_address: { address: 127.0.0.1, port_value: 8125, protocol: TCP }`) ⇒ succeeds; `UDPAddress == "127.0.0.1:8125"` (protocol accepted-and-ignored; dial UDP regardless).
  - **reject — `tcp_cluster_name`** (`typed_config: { "@type": ...StatsdSink, tcp_cluster_name: statsd }`) ⇒ `Load` returns an error matching `tcp_cluster_name` + `UDP-only`.
  - **reject — missing `statsd_specifier`** (a `StatsdSink` with NEITHER `address` NOR `tcp_cluster_name`, e.g. only `prefix: x`) ⇒ error matching `socket_address` / `statsd_specifier`.
  - **reject — sibling/unknown TypeURL** (an entry whose `@type` is some OTHER metrics type, e.g. `type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink`) ⇒ error naming BOTH supported sinks (the message contains `metrics_service` AND `statsd`).
  - **typeurl-descriptor test**: assert `statsdSinkTypeURL == "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"` (mirror the existing `metricsServiceTypeURL` assertion test).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/bootstrap/ -run 'TestStatsd|TestParseStatsd|TestStatsdSinkTypeURL' -count=1` ⇒ FAIL (undefined `StatsdSinkConfig`/`statsdSinkTypeURL`/`parseStatsdSinkConfig`; the accepts error on the unsupported sibling sink). (Adjust `-run` to the actual test names you chose.)

- [ ] **Step 3: Implement** in `bootstrap.go`:
  - Add the descriptor-derived const beside `metricsServiceTypeURL` (`:214`):
```go
// statsdSinkTypeURL is the typed_config TypeURL for the statsd UDP stats sink
// (envoy.config.metrics.v3.StatsdSink). DERIVED from the proto descriptor (the
// metricsServiceTypeURL precedent — reference_network_filter_typeurl_extensions);
// the StatsdSink type resolves at the already-imported metricsconfigv3 package
// (config/metrics/v3, v1.32.4). A test asserts it equals the SPEC §11 string.
var statsdSinkTypeURL = "type.googleapis.com/" + string((&metricsconfigv3.StatsdSink{}).ProtoReflect().Descriptor().FullName())
```
  - Add the struct beside `StatsSinkConfig` (`:274`):
```go
// StatsdSinkConfig is the parsed statsd UDP stats-sink config from one top-level
// stats_sinks[] StatsdSink entry (ADR-0265). The sink (the StatsdSink + Flusher)
// is constructed in cmd/envoy-go/main.go after Load returns; this struct carries
// only the parse-time data. tcp_cluster_name is STRICT-REJECTED (UDP-only — the
// reference boots statsd-over-TCP; ADR-0080); a missing statsd_specifier / nil
// socket_address is a REFERENCE-PARITY reject; socket_address.protocol is
// accepted-and-IGNORED (dial UDP regardless — rejecting the proto-default TCP(0)
// would reject the omit case).
type StatsdSinkConfig struct {
	UDPAddress string // socket_address host:port (an IP literal:port; net.ResolveUDPAddr-resolvable)
	Prefix     string // StatsdSink.prefix, default "envoy" when empty
}
```
  - Add the field beside `StatsSinkConfigs` (`:348`): `StatsdSinkConfigs []StatsdSinkConfig`.
  - Replace the single-URL gate in `parseStatsSinks` (`:430-435`) with a two-arm dispatch:
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
		default:
			return fmt.Errorf("bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports the metrics_service sink %q and the statsd sink %q)", i, tc.GetTypeUrl(), metricsServiceTypeURL, statsdSinkTypeURL)
		}
```
  - Add `parseStatsdSinkConfig` (beside `parseMetricsServiceConfig`):
```go
// parseStatsdSinkConfig parses one statsd UDP stats sink typed_config and appends
// a StatsdSinkConfig to result.StatsdSinkConfigs (ADR-0265). It STRICT-REJECTS
// (ADR-0080): tcp_cluster_name (UDP-only this row — the reference boots statsd-
// over-TCP). A missing statsd_specifier / nil socket_address is a REFERENCE-PARITY
// reject (the reference PGV-rejects it). GetAddress() returns nil for BOTH a
// missing oneof AND a tcp_cluster_name arm, so the tcp_cluster_name check runs
// FIRST (distinct message). socket_address.protocol is accepted-and-IGNORED
// (envoy-go dials UDP regardless). prefix defaults to "envoy" when empty.
func parseStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var sd metricsconfigv3.StatsdSink
	if err := tc.UnmarshalTo(&sd); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd typed_config: %w", idx, err)
	}
	if sd.GetTcpClusterName() != "" {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd tcp_cluster_name is not supported (envoy-go is UDP-only; configure address.socket_address)", idx)
	}
	sa := sd.GetAddress().GetSocketAddress()
	if sa == nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd requires address.socket_address (statsd_specifier is required)", idx)
	}
	prefix := sd.GetPrefix()
	if prefix == "" {
		prefix = "envoy"
	}
	result.StatsdSinkConfigs = append(result.StatsdSinkConfigs, StatsdSinkConfig{
		UDPAddress: fmt.Sprintf("%s:%d", sa.GetAddress(), sa.GetPortValue()),
		Prefix:     prefix,
	})
	return nil
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/bootstrap/ -run 'TestStatsd|TestParseStatsd|TestStatsdSinkTypeURL' -count=1` ⇒ PASS; `go mod tidy -diff` ⇒ EMPTY.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 48 Task 2: statsd parse arm — statsdSinkTypeURL + StatsdSinkConfig + two-arm stats_sinks[] dispatch + parseStatsdSinkConfig + strict-reject (tcp_cluster_name / missing-specifier / sibling-TypeURL naming both sinks) (ADR-0265, ADR-0080)"
```

---

## Task 3: `FuzzStatsdSinkConfigParse` — the no-panic fuzzer over the statsd parse arm (`internal/bootstrap/statsd_fuzz_test.go`)

**Files:**
- Create: `internal/bootstrap/statsd_fuzz_test.go`

The statsd parse arm is an untrusted bootstrap-config boundary (the `FuzzStatsSinkConfigParse` precedent at `internal/bootstrap/statssink_fuzz_test.go`). Drive `Load` end-to-end; assert no-panic (an error return is fine).

- [ ] **Step 1: Write the fuzzer** in `statsd_fuzz_test.go` (mirror `statssink_fuzz_test.go` — the same `head` constant + the `f.Fuzz(func(t, data){ _, _ = Load(bytes.NewReader(data)) })` body). Seeds exercise: the valid accept (socket_address + prefix); the default-prefix accept (no prefix); `protocol: TCP` accepted; each reject arm (`tcp_cluster_name`; missing `statsd_specifier`; the sibling/unknown metrics TypeURL); a coexisting metrics_service + statsd pair; plus degenerate/garbage documents (`[]byte{}`, `"\x00\x00\x00"`, `stats_sinks: [{}]`, a garbage `@type`).
```go
package bootstrap

import (
	"bytes"
	"testing"
)

// FuzzStatsdSinkConfigParse exercises the statsd stats_sinks[] parse arm (phase 48
// Task 2) end-to-end through Load for arbitrary bootstrap document bytes. Load MUST
// NOT panic on any input; a returned error is fine (D-SD-FUZZER). The untrusted
// boundary is the bootstrap config carrying a statsd stats_sinks[] entry (the
// FuzzStatsSinkConfigParse precedent).
func FuzzStatsdSinkConfigParse(f *testing.F) {
	const head = `node: { id: sd-node, cluster: sd-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	const statsdType = "type.googleapis.com/envoy.config.metrics.v3.StatsdSink"
	const msType = "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"

	// valid accept (socket_address + prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      prefix: myprefix
`))
	// default prefix (no prefix)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`))
	// protocol: TCP accepted-and-ignored
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125, protocol: TCP } }
`))
	// tcp_cluster_name (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: statsd
`))
	// missing statsd_specifier (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      prefix: x
`))
	// coexisting metrics_service + statsd
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
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`))
	// degenerate / garbage
	f.Add([]byte{})
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("stats_sinks: [{}]\n"))
	f.Add([]byte(head + "stats_sinks: [{typed_config: {\"@type\": " + statsdType + "}}]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Load MUST NOT panic regardless of data content; an error return is fine.
		_, _ = Load(bytes.NewReader(data))
	})
}
```

- [ ] **Step 2: Run the fuzzer briefly** — `go test ./internal/bootstrap/ -run 'FuzzStatsdSinkConfigParse' -count=1` (seed-only) ⇒ PASS; then `go test ./internal/bootstrap/ -fuzz 'FuzzStatsdSinkConfigParse' -fuzztime 20s` ⇒ no crash, no panic. Confirm the running fuzzer count is now 51: `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **51**.

- [ ] **Step 3 (optional precision fold): refresh the now-stale seed comment** in the EXISTING `internal/bootstrap/statssink_fuzz_test.go:110` — its `// sibling StatsdSink TypeURL (reject)` seed (`tcp_cluster_name: statsd`) no longer rejects-as-sibling at phase 48 (StatsdSink is now SUPPORTED); it rejects in `parseStatsdSinkConfig` on `tcp_cluster_name` (UDP-only). The seed stays a valid no-panic input (the fuzzer stays green) — only the COMMENT is stale. Optionally update it to `// statsd tcp_cluster_name (UDP-only reject)`. If touched, include the file in the commit below.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/statsd_fuzz_test.go
git commit -m "phase 48 Task 3: FuzzStatsdSinkConfigParse — no-panic fuzzer over the statsd stats_sinks[] parse arm (fuzzers 50 -> 51; D-SD-FUZZER)"
```

---

## Task 4: The `StatsdSink` (`internal/statssink/statsd.go`) — a `*net.UDPConn` writer + the statsd-line mapping over a sink-private always-on `deltaState` [TDD, table-driven over a fake UDP listener; full-package `-race`]

**Files:**
- Create: `internal/statssink/statsd.go`, `internal/statssink/statsd_test.go`

**Interfaces:**
- Consumes: the existing `Sink` interface (`sink.go:18`); `newDeltaState()`/`deltaState.apply` (`delta.go`); `dto "github.com/prometheus/client_model/go"` (already a dep).
- Produces: `type StatsdSink struct{...}`; `NewStatsdSink(udpAddr string, prefix string) (*StatsdSink, error)`; `Submit(batch []*dto.MetricFamily)`; `Close() error`. (Task 5's `main.go` loop calls `NewStatsdSink`.)

The sink is SYNCHRONOUS (no channel, no writer goroutine — D-SD-LIFECYCLE-SHAPE): `Submit` applies the sink-private `deltaState` then writes one UDP datagram per family inline. Test against a REAL local UDP listener (`net.ListenUDP` on `127.0.0.1:0`) reading the datagrams — UDP is local and lossless on loopback, so a fake listener + a short read deadline is deterministic.

- [ ] **Step 1: Write the failing tests** in `statsd_test.go`. A helper spins a `127.0.0.1:0` UDP listener, returns its `addr` + a `read(n int) []string` that reads `n` datagrams (each one line) with a read deadline:
  - **counter→`|c` delta + gauge→`|g` absolute + prefix join**: build a batch via `snapshot()` over a registry with `c := reg.NewCounter("cluster.backend.upstream_rq_total"); c.Add(7)` and `g := reg.NewGauge("cluster.backend.membership_healthy"); g.Set(1)`; `s, _ := NewStatsdSink(addr, "myprefix")`; `s.Submit(batch)`; read 2 datagrams ⇒ the set equals `{"myprefix.cluster.backend.upstream_rq_total:7|c", "myprefix.cluster.backend.membership_healthy:1|g"}`.
  - **delta semantics across flushes**: `c.Add(7)` (cumulative 7) → `Submit(snapshot(...))` ⇒ `...:7|c`; then WITHOUT adding more, `Submit(snapshot(...))` ⇒ `...:0|c` (the second flush's delta is 0 — the canonical statsd increment; this is the assertion that proves the sink-private `deltaState` is live); then `c.Add(3)` (cumulative 10) → `Submit(snapshot(...))` ⇒ `...:3|c`.
  - **gauge stays absolute across flushes**: `g.Set(1)` → `Submit` ⇒ `...:1|g`; `g.Set(1)` again → `Submit` ⇒ `...:1|g` (absolute, NOT a 0 delta — proves gauges bypass the delta transform).
  - **negative gauge**: `g.Set(-5)` ⇒ `...:-5|g` (signed absolute; the only sign case).
  - **default prefix**: `NewStatsdSink(addr, "envoy")` ⇒ lines start `envoy.`.
  - **empty batch**: `s.Submit(nil)` ⇒ no datagram written (read deadline elapses with 0 reads), no panic.
  - **Close idempotent**: `s.Close()` twice ⇒ same (nil) error, no panic; a `Submit` is NOT made after `Close` (the contract; do not test send-after-close).
  - **resolve error**: `NewStatsdSink("not a valid addr", "p")` ⇒ `(nil, err)`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestStatsdSink' -count=1` ⇒ FAIL (`StatsdSink`/`NewStatsdSink` undefined).

- [ ] **Step 3: Implement** `statsd.go`:
```go
package statssink

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// StatsdSink writes the frozen registry snapshot to a statsd server as UDP
// datagrams every flush (ADR-0265): one statsd line per metric family,
// <prefix>.<dotted-name>:<value>|<type>. COUNTER families carry the per-flush
// DELTA (|c) over a sink-private always-on deltaState (the canonical statsd
// increment — D-SD-DELTA; reusing the landed delta.go transform VERBATIM, no knob);
// GAUGE families carry the ABSOLUTE value (|g). The name is the full dotted
// internal name with tags inlined and ZERO labels (the StatsdSink proto does not
// support tagged metrics). envoy-go has no histograms (ADR-0060), so only |c/|g
// lines are produced (the reference's |ms timers have no analog).
//
// Writer shape (D-SD-LIFECYCLE-SHAPE): SYNCHRONOUS. A UDP Write is fire-and-forget
// (never blocks on a peer), and the Flusher calls Submit serially from its single
// goroutine, so Submit writes each datagram inline — no bounded channel, no writer
// goroutine (contrast MetricsServiceSink, whose channel absorbs a slow gRPC
// stream). This adds NO background mutator. A Write error is rate-limit-LOGGED and
// DROPPED (UDP is lossy by design; NOT counted — +0 self-stats, D-SD-STATS-FINAL).
type StatsdSink struct {
	conn   *net.UDPConn
	prefix string
	delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta (no knob)

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// NewStatsdSink resolves udpAddr (a host:port literal), dials a connected UDP
// socket (the FIRST *net.UDPConn in the tree), and returns a ready sink. A
// resolve/dial error is returned verbatim (-> main.go log.Fatalf, the
// metrics_service-client-error precedent).
func NewStatsdSink(udpAddr string, prefix string) (*StatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial statsd udp %q: %w", udpAddr, err)
	}
	return &StatsdSink{conn: conn, prefix: prefix, delta: newDeltaState()}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then writes one UDP datagram per family. Called serially
// by the Flusher.
func (s *StatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	for _, fam := range batch {
		name := s.prefix + "." + fam.GetName()
		var suffix string
		switch fam.GetType() {
		case dto.MetricType_COUNTER:
			suffix = "|c"
		case dto.MetricType_GAUGE:
			suffix = "|g"
		default:
			continue // no other family type exists (no histograms — ADR-0060)
		}
		for _, m := range fam.GetMetric() {
			var v float64
			if fam.GetType() == dto.MetricType_GAUGE {
				v = m.GetGauge().GetValue()
			} else {
				v = m.GetCounter().GetValue()
			}
			line := name + ":" + strconv.FormatInt(int64(v), 10) + suffix
			s.write(line)
		}
	}
}

// write sends one statsd line as one UDP datagram. A Write error is rate-limit-
// logged (at most once per second — the accesslog lastDropLog idiom) and dropped.
func (s *StatsdSink) write(line string) {
	if _, err := s.conn.Write([]byte(line)); err != nil {
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: statsd udp write failed, dropping line: %v", err)
		}
	}
}

// Close closes the UDP socket. Idempotent via sync.Once.
func (s *StatsdSink) Close() error {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			s.closeErr = s.conn.Close()
		}
	})
	return s.closeErr
}
```
NOTE: `dropLogIntervalNanos` is already defined in `sink.go:47` (same package) — reuse it, do NOT redeclare. The counter value is `float64(delta)` (a uint64 < 2^53) and the gauge value is `float64(Load())` (an int64) — both exact integers; `strconv.FormatInt(int64(v), 10)` round-trips them (counters are non-negative ⇒ no sign; gauges may be negative ⇒ the only `-` case, AMEND-SD-LINE).

- [ ] **Step 4: Run to verify they pass + full-package race** — `go test ./internal/statssink/ -run 'TestStatsdSink' -count=1` ⇒ PASS; then the FULL package race (the `Flusher` ticker remains a background mutator — `reference_full_suite_race_after_background_mutator`): `go test ./internal/statssink/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/statsd.go internal/statssink/statsd_test.go
git commit -m "phase 48 Task 4: StatsdSink — *net.UDPConn writer + <prefix>.<name>:<v>|c/|g line mapping over a sink-private always-on deltaState (synchronous Submit, idempotent Close; ADR-0265, D-SD-DELTA/LINE/LIFECYCLE)"
```

---

## Task 5: Boot wiring — the statsd build loop + the flusher-gate generalization (`cmd/envoy-go/main.go`)

**Files:**
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `bs.StatsdSinkConfigs` (Task 2); `statssink.NewStatsdSink` (Task 4); the existing `statsSinks []statssink.Sink` (`:190`), `statsFlusher` (`:189`), the flusher build (`:201`), the shutdown defer (`:212`).
- Produces: statsd sinks appended to the SAME `statsSinks` slice; the flusher built when EITHER sink kind exists. (No new exported surface.)

- [ ] **Step 1: Generalize the build gate + add the statsd loop.** Change the gate at `:192` and add a second loop. The node-build stays scoped to the metrics_service branch (statsd needs no node). The result:
```go
	if len(bs.StatsSinkConfigs) > 0 || len(bs.StatsdSinkConfigs) > 0 {
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
		// Phase 48 (ADR-0265): the statsd UDP stats sink. NewStatsdSink dials a
		// connected UDP socket; a resolve/dial error is a fatal boot failure (the
		// metrics_service-client precedent). Synchronous (no goroutine), so it adds
		// no background mutator to the shutdown drain.
		for _, cfg := range bs.StatsdSinkConfigs {
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		statsFlusher = statssink.NewFlusher(bs.Stats, bs.FlushInterval, statsSinks)
	}
```
Update the comment block at `:180-188` to note BOTH sink kinds now feed `statsSinks`. The shutdown defer (`:212-217`), the post-Freeze `statsFlusher.Start` (`:373-380`), and the `flusherDone` plumbing are UNCHANGED (sink-agnostic).

- [ ] **Step 2: Verify byte-stability + build.** No statsd unit test in `main.go` (it is exercised end-to-end by `0092`). Confirm the no-sink path is untouched:
```bash
go build ./... && echo BUILD_OK
go vet ./cmd/... && gofmt -l cmd/envoy-go/main.go   # gofmt empty
golangci-lint run ./cmd/...
```
The byte-stability regression anchor is the FULL differential (Task 9) — when `StatsdSinkConfigs` is empty (every existing fixture), the gate is unchanged and no UDP dial/sink happens.

- [ ] **Step 3: Commit**
```bash
git add cmd/envoy-go/main.go
git commit -m "phase 48 Task 5: main.go — statsd sink build loop into the shared statsSinks slice + flusher-gate generalization (build when EITHER sink kind exists; ADR-0265)"
```

---

## Task 6: The driver-owned UDP receiver (`test/helpers/statsdrecv/statsdrecv.go`) [+ unit test]

**Files:**
- Create: `test/helpers/statsdrecv/statsdrecv.go`, `test/helpers/statsdrecv/statsdrecv_test.go`

**Interfaces:**
- Produces: `type Server struct{...}`; `NewAtAddr(addr string) (*Server, error)`; `DeltaSum(name string) (sum float64, ok bool)`; `Gauge(name string) (value float64, ok bool)`; `SeenCount(name string) int`; `Reset()`; `Addr() string`; `Close()`. (Task 7's driver consumes all of these.)

The UDP analog of `test/helpers/metricsservice` (driver-owned; NOT a BackendKind — `reference_differential_grpc_receiver_driver_owned`). Each flush emits one datagram per metric NAME, so `SeenCount(name)` (datagrams-per-name) IS the per-name flush counter — the stability-barrier signal (a `|c` line with delta 0 still increments `SeenCount`). `DeltaSum(name)` sums `|c` values (the delta-SUM == K invariant); `Gauge(name)` is the last-seen `|g` value.

- [ ] **Step 1: Write the failing test** in `statsdrecv_test.go`:
  - `srv, _ := NewAtAddr("127.0.0.1:0")`; `defer srv.Close()`; dial `net.Dial("udp", srv.Addr())`.
  - Write 3 datagrams: `"p.cluster.x.rq_total:7|c"`, `"p.cluster.x.rq_total:0|c"`, `"p.cluster.x.healthy:1|g"`. Poll (≤2s) until `srv.SeenCount("p.cluster.x.rq_total") == 2`.
  - assert `DeltaSum("p.cluster.x.rq_total") == 7` (7+0) with ok; `Gauge("p.cluster.x.healthy") == 1` with ok; `SeenCount("p.cluster.x.rq_total") == 2`; an absent name ⇒ `ok == false`.
  - `Reset()` ⇒ `DeltaSum`/`Gauge`/`SeenCount` all zero/absent.

- [ ] **Step 2: Run to verify it fails** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ FAIL (package undefined).

- [ ] **Step 3: Implement** `statsdrecv.go`:
```go
// Package statsdrecv provides a minimal in-process statsd UDP receiver for tests
// and differential fixtures. It is the statsd counterpart of the metricsservice
// helper (the gRPC MetricsService receiver): a driver-owned receiver that the
// proxy WRITES UDP datagrams to, so per project convention it is a test helper
// rather than a runner BackendKind (reference_differential_grpc_receiver_driver_owned;
// BackendKind STAYS 38).
package statsdrecv

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Server reads statsd line-protocol UDP datagrams (<prefix>.<name>:<value>|<type>)
// and accumulates, per full dotted name: the running SUM of every |c (counter)
// value (DeltaSum — under the always-delta statsd counter model the per-flush
// deltas sum to the cumulative total, == K after K requests), the last-seen |g
// (gauge) absolute value (Gauge), and the datagram COUNT (SeenCount). Because the
// sink emits exactly one datagram per metric name per flush, SeenCount(name) is
// the number of flushes that included name — the delta stability-barrier signal
// (an idle counter's 0-delta |c line still increments SeenCount). Goroutine-safe
// via an RWMutex (the reader goroutine writes; the poll/assert surface reads)
// under the -race detector.
type Server struct {
	conn *net.UDPConn

	mu        sync.RWMutex
	deltaSums map[string]float64 // |c running sum per name
	gauges    map[string]float64 // |g last-seen per name
	seen      map[string]int     // datagram count per name

	closeOnce sync.Once
}

// NewAtAddr binds a UDP listener on the caller-supplied host:port (e.g.
// "0.0.0.0:<port>" so a Docker reference-Envoy can write to the host, or
// "127.0.0.1:0" for an ephemeral loopback port) and starts a reader goroutine.
// Lifecycle is the caller's responsibility via Close (the metricsservice.NewAtAddr
// precedent — no t.Cleanup).
func NewAtAddr(addr string) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: resolve %q: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: listen %q: %w", addr, err)
	}
	s := &Server{
		conn:      conn,
		deltaSums: make(map[string]float64),
		gauges:    make(map[string]float64),
		seen:      make(map[string]int),
	}
	go s.readLoop()
	return s, nil
}

// readLoop reads datagrams until the conn is closed (ReadFromUDP returns an error),
// ingesting each. A 64 KiB buffer comfortably holds one statsd line (max ~77 bytes
// observed; §11).
func (s *Server) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return // conn closed
		}
		s.ingest(buf[:n])
	}
}

// ingest parses each newline-delimited statsd line in one datagram (the reference
// emits one line per datagram — §11 — but split-on-newline is robust to batching)
// and updates the accumulators. Malformed lines are skipped.
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.LastIndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := line[:colon]
		rest := line[colon+1:]
		pipe := strings.IndexByte(rest, '|')
		if pipe < 0 {
			continue
		}
		val, err := strconv.ParseFloat(rest[:pipe], 64)
		if err != nil {
			continue
		}
		typ := rest[pipe+1:]
		switch typ {
		case "c":
			s.deltaSums[name] += val
			s.seen[name]++
		case "g":
			s.gauges[name] = val
			s.seen[name]++
		}
	}
}

// DeltaSum returns the running SUM of every |c value received for name, and
// ok=false if none was received.
func (s *Server) DeltaSum(name string) (sum float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum, ok = s.deltaSums[name]
	return
}

// Gauge returns the last-seen |g absolute value for name, and ok=false if none.
func (s *Server) Gauge(name string) (value float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok = s.gauges[name]
	return
}

// SeenCount returns the number of datagrams received for name (== flushes that
// included it). The delta stability-barrier signal.
func (s *Server) SeenCount(name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seen[name]
}

// Reset drops all accumulators (per-side separation, the metricsservice.Reset
// precedent). Call only when no datagram is in flight.
func (s *Server) Reset() {
	s.mu.Lock()
	s.deltaSums = make(map[string]float64)
	s.gauges = make(map[string]float64)
	s.seen = make(map[string]int)
	s.mu.Unlock()
}

// Addr returns the bound host:port (load-bearing when NewAtAddr allocated an
// ephemeral port).
func (s *Server) Addr() string {
	return s.conn.LocalAddr().String()
}

// Close closes the UDP socket (unblocking the reader goroutine). Idempotent via
// sync.Once. UDP is connectionless — there is no GracefulStop-vs-hard-stop
// distinction (contrast the metricsservice gRPC receiver).
func (s *Server) Close() {
	s.closeOnce.Do(func() { _ = s.conn.Close() })
}
```

- [ ] **Step 4: Run to verify it passes + race** — `go test ./test/helpers/statsdrecv/ -count=1` ⇒ PASS; `go test ./test/helpers/statsdrecv/ -race -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/statsdrecv/ && golangci-lint run ./test/helpers/statsdrecv/... && go vet ./test/helpers/statsdrecv/... && go build ./...
git add test/helpers/statsdrecv/
git commit -m "phase 48 Task 6: test/helpers/statsdrecv — driver-owned UDP statsd receiver (DeltaSum/Gauge/SeenCount poll surface; the metricsservice analog; BackendKind stays 38; D-SD-RECEIVER-WIRING)"
```

---

## Task 7: The harness `HostGatewayIP` helper (`test/differential/harness.go`)

**Files:**
- Modify: `test/differential/harness.go`
- Test: `test/differential/harness_test.go` (add an opportunistic test)

**Interfaces:**
- Produces: `func HostGatewayIP(ctx context.Context) (string, error)` — the Docker `bridge` network's IPAM gateway IP (a literal IP the reference container reaches the host at; the statsd sink rejects hostnames). (Task 8's driver bakes it as the reference statsd address.)

The reference container reaches the host via `host.docker.internal:host-gateway` = the host's IP on the Docker bridge. `HostGatewayIP` returns that literal IP by inspecting the `bridge` network (the default network testcontainers attaches to). A host process bound `0.0.0.0:<port>` is reachable from the container at this IP.

- [ ] **Step 1: Write the test** in `harness_test.go` (gated on Docker availability, mirroring the existing `docker unavailable` skip at `:67-78`):
  - `ip, err := HostGatewayIP(context.Background())`; require `err == nil`; assert `net.ParseIP(ip) != nil` (a valid literal IP) and `ip` is non-empty. Skip with `t.Skip` if Docker is unavailable (reuse the existing socket-probe skip).

- [ ] **Step 2: Run to verify it fails** — `go test ./test/differential/ -run 'TestHostGatewayIP' -count=1` ⇒ FAIL (undefined).

- [ ] **Step 3: Implement** `HostGatewayIP` in `harness.go`. Acquire a Docker client via testcontainers (`testcontainers.NewDockerClientWithOpts(ctx)` — the v0.27.0 helper) and inspect the `bridge` network:
```go
// HostGatewayIP returns the Docker bridge network's IPAM gateway IP — the host's
// address on the bridge that a reference container reaches the host at (the same
// endpoint host.docker.internal:host-gateway resolves to). It is a LITERAL IP, so
// it can be baked into config that rejects hostnames (the statsd UDP sink —
// AMEND-SD-REJECT; reference_docker_probe_bridge_network). A host process bound
// 0.0.0.0:<port> is reachable from the reference container at this IP:port.
func HostGatewayIP(ctx context.Context) (string, error) {
	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return "", fmt.Errorf("host gateway ip: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	nw, err := cli.NetworkInspect(ctx, "bridge", dockertypes.NetworkInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("host gateway ip: inspect bridge network: %w", err)
	}
	for _, cfg := range nw.IPAM.Config {
		if cfg.Gateway != "" {
			return cfg.Gateway, nil
		}
	}
	return "", fmt.Errorf("host gateway ip: bridge network has no IPAM gateway")
}
```
NOTE: the import for `NetworkInspectOptions` is `github.com/docker/docker/api/types` (alias it `dockertypes`) — confirmed against the vendored `docker/docker v24.0.7+incompatible`: `(*client.Client).NetworkInspect(ctx, id string, types.NetworkInspectOptions) (types.NetworkResource, error)`, `NetworkResource.IPAM.Config []IPAMConfig`, `IPAMConfig.Gateway string` (the `IPAMConfig` type is reached through the returned value — no separate import). `testcontainers.NewDockerClientWithOpts(ctx, ...)` returns `(*testcontainers.DockerClient, error)` in v0.27.0 — that type EMBEDS `*client.Client`, so `cli.NetworkInspect(...)` and `cli.Close()` are promoted (the code above is correct as written). Re-run `go doc` if the vendored version has bumped.

- [ ] **Step 4: Run to verify it passes** — `go test ./test/differential/ -run 'TestHostGatewayIP' -count=1` ⇒ PASS (or SKIP if Docker is unavailable in the task sandbox — the controller re-runs it on the live-Docker host). `go build ./... && go vet ./test/differential/...`.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/differential/harness.go && golangci-lint run ./test/differential/... && go vet ./test/differential/... && go build ./...
git add test/differential/harness.go test/differential/harness_test.go
git commit -m "phase 48 Task 7: harness HostGatewayIP — the Docker bridge gateway literal IP for the reference's statsd UDP address (statsd rejects hostnames; D-SD-RECEIVER-WIRING)"
```

---

## Task 8: The `0092-stats-sink-statsd` differential fixture (driver + YAMLs + expectations + README) + register in the runner

**Files:**
- Create: `test/fixtures/0092-stats-sink-statsd/driver/driver.go`, `.../envoy.yaml`, `.../envoy-go.yaml`, `.../expectations.yaml`, `.../README.md`
- Modify: `test/differential/runner_test.go` (blank-import the `0092` driver)

**Interfaces:**
- Consumes: `statsdrecv.{NewAtAddr,DeltaSum,Gauge,SeenCount,Addr,Close}` (Task 6); `differential.HostGatewayIP` (Task 7); the statsd parse arm + sink + main wiring (Tasks 2/4/5); the `fixture.{Driver,BackendKindAware,StatsAsserter}` interfaces + `fixture.RegisterFixture` (the `0090` driver shows the exact surface).

Clone the `0090` driver and adapt: TWO per-side `statsdrecv` receivers (not gRPC); the statsd bootstrap config (not metrics_service); the reference statsd address = `<HostGatewayIP>:<refStatsdPort>` (literal IP); the subject statsd address = `127.0.0.1:<subjStatsdPort>`; the delta-SUM + stability barrier on the 3-counter subset; PLUS the absolute-gauge subset (`cluster.<backend>.membership_healthy == 1`). The prefix is baked IDENTICALLY on both sides.

- [ ] **Step 1: Write `envoy.yaml`** (the reference bootstrap template — clone `0090/envoy.yaml`, swap the `stats_sinks[]` block). Templated keys: `{{.AdminPort}}` (9901), `{{.ListenerPort}}` (10092), `{{.BackendHost}}` (`host.docker.internal`), `{{.BackendPort}}`, `{{.StatsdHost}}` (the HostGatewayIP literal), `{{.StatsdPort}}` (refStatsdPort), `{{.Prefix}}`, `{{.StatPrefix}}`, `{{.BackendName}}`. The sink block:
```yaml
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.StatsdSink
      address:
        socket_address: { protocol: UDP, address: {{.StatsdHost}}, port_value: {{.StatsdPort}} }
      prefix: {{.Prefix}}
stats_flush_interval: 0.5s
```
The listener/route/cluster blocks are the `0090` H1 single-listener → `HTTPFixedBody` backend shape (stat_prefix `{{.StatPrefix}}`, cluster `{{.BackendName}}`). NO `node` is needed (statsd carries no identifier — contrast metrics_service). A 0.5s flush interval gives fast deterministic convergence.

- [ ] **Step 2: Write `envoy-go.yaml`** (the subject template — the same shape, `{{.StatsdHost}}` = `127.0.0.1`, `{{.StatsdPort}}` = subjStatsdPort, the runner-allocated admin/listener/backend ports). Same statsd sink block + `stats_flush_interval: 0.5s` + same `prefix`/`stat_prefix`/cluster name.

- [ ] **Step 3: Write the driver** `driver/driver.go` (clone `0090/driver/driver.go`; the diffs):
  - `fixtureName = "0092-stats-sink-statsd"`; `refListenerPort = 10092`; `numReq = 7`; `statPrefix = "hcm_local"`; `backendName = "c_backend"`; `prefix = "sdpfx"` (baked both sides); REMOVE the `wantNodeID`/`wantNodeCluster` (statsd has no identifier).
  - `subsetNames` — the 3 counters, **prefix-joined** (the receiver keys on `<prefix>.<name>`):
    ```go
    var subsetNames = []string{
        prefix + ".cluster." + backendName + ".upstream_rq_total",
        prefix + ".http." + statPrefix + ".downstream_rq_total",
        prefix + ".http." + statPrefix + ".downstream_rq_2xx",
    }
    var gaugeName = prefix + ".cluster." + backendName + ".membership_healthy" // absolute |g == 1
    ```
  - The receiver type is `*statsdrecv.Server` (not `metricsservice.Server`); `mustStartReceiver` calls `statsdrecv.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", port))`.
  - `ReferenceBootstrap`: call `differential.HostGatewayIP(context.Background())` (panic on error — the §11 wiring), bake `StatsdHost` = that IP, `StatsdPort` = `d.refStatsdPort`, `BackendHost` = `host.docker.internal`. (Rename the `MetricsHost`/`MetricsPort` keys to `StatsdHost`/`StatsdPort`.) **Driver is in package `driver` under `test/fixtures/...`; import the harness as `differential "github.com/esalaine/envoy-go/test/differential"` — verify no import cycle (the driver is imported BY the runner via blank-import; the runner is `package differential`. A `driver` package importing `differential` while `differential`'s test blank-imports `driver` is the EXISTING pattern? CHECK: if it cycles, instead expose `HostGatewayIP` so the driver can call it without a cycle — the driver package is separate from the `differential` test package, so `driver` importing `differential` (non-test code) is acyclic as long as `differential`'s non-test code does not import `driver`. The blank-import lives in `runner_test.go` (test code), so no cycle. Confirm with `go build`.)**
  - `SubjectConfig`: `StatsdHost` = `127.0.0.1`, `StatsdPort` = `d.subjStatsdPort`.
  - `driveSide`: fire `numReq` GETs → `pollSubset` (poll until each subset name's `DeltaSum == numReq`) → `awaitFurtherFlushes(srv, marker, 2)` (the stability barrier — wait until `srv.SeenCount(marker) >= base+2` for a marker subset counter) → snapshot the 3 `DeltaSum`s + the `Gauge(gaugeName)`. The stability-barrier helper keyed on `SeenCount` (one datagram per name per flush):
    ```go
    func awaitFurtherFlushes(ctx context.Context, srv *statsdrecv.Server, marker string, extra int) error {
        base := srv.SeenCount(marker)
        deadline := time.Now().Add(pollDeadline)
        for srv.SeenCount(marker) < base+extra {
            if time.Now().After(deadline) { return fmt.Errorf("statsd receiver: timed out waiting for %d further flushes (seen=%d base=%d)", extra, srv.SeenCount(marker), base) }
            select {
            case <-ctx.Done(): return fmt.Errorf("statsd receiver: context done waiting for %d further flushes: %w", extra, ctx.Err())
            case <-time.After(pollInterval):
            }
        }
        return nil
    }
    ```
    Use `subsetNames[0]` as the marker.
  - `subsetConverged`: every subset name has `DeltaSum == numReq` (the `0090` shape, swapping `FamilySum` → `DeltaSum`).
  - `sideSnapshot`: `{ sums map[string]float64; gaugeVal float64; gaugeOK bool }` (drop the node fields).
  - `AssertStats`/`assertSide`: assert each subset name's `DeltaSum == numReq`; assert `gaugeOK && gaugeVal == 1` (the absolute `|g` subset — D-SD-GAUGE-SUBSET); the decode-ran proof is the converge poll (a zero-datagram pass is structurally impossible). REMOVE the node id/cluster assertions.
  - `DriveSubject` calls `closeServers()` (hard `Close()` on both UDP receivers — connectionless, so a plain `Close` suffices; the `0090` precedent).
  - Keep `BackendCount()==1`, `BackendKind()==fixture.HTTPFixedBody`, `SubjectListenerName()=="l_test"`, `ReferenceListenerPort()==refListenerPort`, `ProbeAdmin` (GET /ready both sides), the `mustReadFixtureFile`/`mustRender`/`fixtureDir` helpers, the compile-time `fixture.{Driver,BackendKindAware,StatsAsserter}` assertions.

- [ ] **Step 4: Write `expectations.yaml` + `README.md`** (clone `0090`'s, adapt the prose to the statsd line protocol + the delta-SUM-with-stability-barrier + the absolute-gauge subset + the literal-IP `HostGatewayIP` wiring + the UNasserted set: the whole line set / family count [surfaces differ — the reference emits `|ms` + many gauges envoy-go lacks], non-deterministic gauges, the per-datagram framing, the `|ms` timers).

- [ ] **Step 5: Register the driver** in `runner_test.go` (after the `0091` blank-import at `:118`):
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0092-stats-sink-statsd/driver"
```

- [ ] **Step 6: Run the differential** (live Docker — the controller's host). `reference_differential_run_selector` — NEVER bare `0092`:
```bash
go test ./test/differential/ -run 'TestDifferential/0092' -count=1 -v
```
Expected: PASS; the `-v` output shows the per-side delta-SUMs == 7 and the gauge == 1. Confirm decode ran (each side's `DeltaSum(subset) == 7` is structurally required to converge).

- [ ] **Step 7: Deliberate breaks** (`reference_differential_break_protocol_count1` — `-count=1` EVERY break; revert after each; verify the main repo is clean + branch undetached after reverting — `feedback_subagent_worktree_detach`):
  - **(a) absolute-not-delta** — in `statsd.go` `Submit`, temporarily emit the absolute counter value (skip `s.delta.apply`, or emit `m.GetCounter().GetValue()` from the un-applied batch). The stability barrier MUST FAIL: after convergence the next flushes re-add the cumulative, so `DeltaSum` overshoots 7. Run `go test ./test/differential/ -run 'TestDifferential/0092' -count=1` ⇒ FAIL on a subset `DeltaSum != 7`. This proves the delta assertion is live. REVERT.
  - **(b) wrong prefix / `|g` for a counter** — temporarily join the wrong prefix (e.g. `"wrong." + fam.GetName()`) OR emit `|g` for a counter. The subset `DeltaSum` lookup misses ⇒ FAIL (timeout: subset never converges, or the gauge/counter type flips). This proves the name/type assertion is live. REVERT.
  - **(c) barrier-masks-(a)** — re-apply break (a), AND temporarily drop the `awaitFurtherFlushes` call in the driver (snapshot right after `pollSubset`). Verify the differential now PASSES (the first flush's delta == the absolute == 7, so without the barrier the absolute-emit break is INVISIBLE) — demonstrating the barrier is load-bearing. REVERT both.

- [ ] **Step 8: Flake-stability + full-package race**:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0092' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FAIL"; done   # expect 20/20 PASS
go test ./internal/statssink/ -race -count=1   # full package; the Flusher ticker is a background mutator
```
(If a run shows `subject ready: EOF`, isolate-re-run per `reference_differential_fullsuite_startup_flake` — a startup race, not a regression.)

- [ ] **Step 9: Per-task gates + commit**
```bash
gofmt -l test/fixtures/0092-stats-sink-statsd/ test/differential/runner_test.go && golangci-lint run ./test/... && go vet ./test/... && go build ./...
git add test/fixtures/0092-stats-sink-statsd/ test/differential/runner_test.go
git commit -m "phase 48 Task 8: 0092-stats-sink-statsd differential — cross-side delta-SUM-with-stability-barrier (3-counter subset) + absolute-gauge subset over the statsd UDP sink; two per-side UDP receivers + HostGatewayIP literal-IP wiring; breaks (a)(b)(c) + 20/20 flake + full-package race (ADR-0265, D-SD-DELTA/GAUGE-SUBSET)"
```

---

## Task 9: The +0 stat-surface guard (D-SD-STATS-FINAL) + the full differential + the six-gate

**Files:**
- Modify (only if the registration test needs a statsd note): `internal/bootstrap/registration_test.go` (or the equivalent surface-count test — `grep -rln 'func Test.*[Rr]egistration\|1196\|1200' internal/`)

**Interfaces:** none new — this task PROVES the surface is unchanged + the suite is green.

- [ ] **Step 1: Confirm the +0 surface.** Locate the stat-surface registration test (`grep -rn '1196\|1200\|RegisteredNames\|registration' internal/bootstrap/*_test.go internal/stats/*_test.go`). Run it: it MUST PASS UNCHANGED (the statsd sink registers NO self-stat; the UDP sink dials no cluster — D-SD-STATS-FINAL). If the test enumerates names, confirm no `statsd`/`stat_sink`-scoped name appears. No code change is expected; if the test needs a comment noting the statsd sink is +0, add it.
```bash
go test ./internal/bootstrap/ ./internal/stats/ -count=1   # surface tests PASS, count unchanged 1200 / non-H2 1196
```

- [ ] **Step 2: The full differential** (live Docker; 94 dirs — `reference_differential_fullsuite_startup_flake`: a transient `subject ready: EOF` on an UNRELATED dir is a startup race — isolate-re-run that dir, then re-run full):
```bash
go test ./test/differential/ -count=1 2>&1 | tail -30   # expect ok (all 94 fixtures incl. 0092)
```

- [ ] **Step 3: The six-gate** (the project's full pre-merge suite):
```bash
gofmt -l $(git diff --name-only master -- '*.go')     # empty
golangci-lint run ./...                               # clean
go vet ./...                                          # clean
go build ./...                                        # BUILD_OK
go test ./... -count=1                                # ALL PASS (unit + helpers + bootstrap)
go mod tidy -diff                                     # EMPTY (no new module)
```

- [ ] **Step 4: Commit** (only if Step 1 touched the registration test; otherwise this is a verification-only step folded into Task 10's commit):
```bash
git add internal/bootstrap/registration_test.go 2>/dev/null || true
git commit -m "phase 48 Task 9: confirm +0 stat surface (1200 / non-H2 1196 unchanged) — no statsd self-stat, no sink cluster (D-SD-STATS-FINAL)" 2>/dev/null || echo "no registration change — verification-only"
```

---

## Task 10: ADR-0265 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + the fuzzer-count reconcile

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0265 §Decision + §Consequences), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/48-stats-sink-statsd/PROGRESS-48.md`

- [ ] **Step 1: ADR-0265 body** — append §Decision + §Consequences to the ADR-0265 §Context already drafted in SPEC-48 §13 (copy the §Context into DECISIONS.md if not already there, then add): §Decision — lift the `StatsdSink` TypeURL into a two-arm `stats_sinks[]` dispatch + `parseStatsdSinkConfig` → `StatsdSinkConfig{UDPAddress, Prefix}`; the `internal/statssink/statsd.go` `StatsdSink` (a `*net.UDPConn` writer + the `<prefix>.<name>:<v>|c`/`|g` line mapping over a sink-private always-on `deltaState`, synchronous `Submit`, idempotent `Close`); the `main.go` second build loop; STRICT-REJECT `tcp_cluster_name` (UDP-only) + sibling TypeURLs (naming both sinks) + REFERENCE-PARITY reject a missing `statsd_specifier`; `HostGatewayIP` + `test/helpers/statsdrecv` + `0092` (delta-SUM + stability barrier + absolute-gauge subset); +0 stat surface; ZERO new packages/modules. §Consequences — the SECOND `stats_sinks[]` consumer + FIRST UDP datagram seam; the always-on delta (no knob — intrinsic to statsd); `tcp_cluster_name`/dog_statsd/graphite/OTLP-metrics + `|ms` timers deferred; the family STAYS OPEN.

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — add the `### Stats sinks — the statsd UDP sink` subsection (under the phase-47 metrics_service section) per SPEC §9: a bootstrap `stats_sinks[]` `statsd` entry with a UDP `address` → dial + flush one statsd line per metric per UDP datagram (`<prefix>.<name>:<delta>|c` counters / `<prefix>.<name>:<abs>|g` gauges, full dotted name, no labels, default prefix `envoy`); STRICT-REJECT `tcp_cluster_name` + sibling/unknown sink TypeURL; REFERENCE-PARITY reject a missing `statsd_specifier`; `log.Fatalf` on an unresolvable UDP address; byte-identical when no statsd sink is configured; stat surface stays 1200 (+0).

- [ ] **Step 3: STATE.md** — roll the active-phase header to `phase 48 (stats-sink-statsd) IMPL done` (row 48 `done`); update the counts to stat **1200** / fixtures **94** / fuzzers **51** / BackendKind **38** / DECISIONS **ADR-0265** (next-free ADR-0266); set NEXT to "none chartered" (the loop re-opens via the router / next BRAINSTORM).

- [ ] **Step 4: ROADMAP.md** — flip row 48 (`stats-sink-statsd`) to **`done`** (the sole leg — ADR-0106; no parent rollup); keep the Observability family note OPEN.

- [ ] **Step 5: PROGRESS-48.md** — mark all tasks complete; record the FINAL counts (re-run the Task-1 baseline commands and paste): fixtures **94**, fuzzers **51** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **51** — the `reference_fuzzer_count_docs_drift` reconcile: 50 → 51), stat **1200**, BackendKind **38**, `go mod tidy -diff` EMPTY.

- [ ] **Step 6: Final full suite on the frozen HEAD + commit**:
```bash
go build ./... && go test ./... -count=1 && grep -rh '^func Fuzz' --include='*.go' . | wc -l   # ALL PASS; fuzzers == 51
git add docs/envoy-go/
git commit -m "phase 48 Task 10: ADR-0265 §Decision/§Consequences + BEHAVIOR_CONTRACT statsd subsection + STATE/ROADMAP (row 48 done) + PROGRESS close + fuzzer-count reconcile (50 -> 51) — statsd UDP stats sink COMPLETE"
```

---

## Self-Review (run before declaring the plan ready)

- **Spec coverage:** SPEC §3 (parse arm + struct + dispatch + main wiring) → Tasks 2/5; §3.3 (StatsdSink + delta + line mapping + synchronous Submit + idempotent Close) → Task 4; §5 (proto roster) → Task 2; §6 (reject arms + fuzzer) → Tasks 2/3; §7 (+0 surface) → Task 9; §8 (0092 + breaks + receiver + BackendKind 38) → Tasks 6/7/8; §9 (behavior contract) → Task 10; §11 D-SD-* pins → honored in Tasks 2/4/8; §12 D-SD-* PLAN questions → resolved in the D-question block above; §13 (ADR-0265) → Task 10. All covered.
- **Placeholder scan:** the one genuine live-iteration risk is the Docker-networking glue (Task 7 `NetworkInspectOptions` import + Task 8 reachability) — provided as concrete code with a flagged IMPL-time confirmation + a named fallback (user-defined bridge), NOT a vague placeholder. The exact registration-test name (Task 9) is located via `grep` rather than assumed.
- **Type consistency:** `StatsdSinkConfig{UDPAddress, Prefix}` (Task 2) ↔ `NewStatsdSink(udpAddr, prefix)` (Task 4) ↔ `main.go` loop (Task 5); `statsdrecv.{DeltaSum,Gauge,SeenCount}` (Task 6) ↔ the `0092` driver (Task 8); `HostGatewayIP(ctx)` (Task 7) ↔ the driver's `ReferenceBootstrap` (Task 8); the `Sink` interface satisfied by `StatsdSink` (Submit/Close). Consistent.

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/48-stats-sink-statsd/PLAN-48.md`.** Per the router (and `feedback_execution_style` + `feedback_git_worktrees`), the phase-48 IMPL is subagent-driven in a FRESH worktree off master; subagents commit locally only; the controller verifies each commit + re-runs the full suite on the frozen HEAD + does the deliberate-break verification ITSELF + squashes + pushes at stage-close. The next router stage (after this PLAN lands + a `plan-document-reviewer` pass) is the phase-48 IMPL.
