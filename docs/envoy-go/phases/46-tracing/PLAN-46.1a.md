# Phase 46.1a Implementation Plan — the header-level request-tracing engine: a NEW `internal/tracing` package (the full-fidelity sampling/request-id decision + W3C `traceparent` extract/continue/inject + `tracestate` pass-through + the `x-request-id` UUID byte-14 trace-reason stamp) + the HCM-filter config-parse arm (LIFT `HttpConnectionManager.tracing` from the ADR-0041 silent-ignore set) + the 5 HCM-scoped `http.<stat_prefix>.tracing.*` counters + the `0086-tracing-request-id` cross-side differential — the FIRST sub-leg of the 46.1 (core+OTLP) by-exporter leg (span emission + the OTLP exporter follow at 46.1b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** When an HCM carries a `tracing` block with a recognized OpenTelemetry provider, envoy-go runs the full-fidelity sampling/request-id decision per request (the three `Percent` knobs + `x-client-trace-id` force + the incoming `traceparent` sampled-bit, in the §11 precedence), GENERATES (or preserves-and-stamps) the `x-request-id` UUID with the trace-reason encoded into its string-index-14 nibble (NoTrace=`4` / Sampled=`9` / Client=`b`), EXTRACTS an incoming W3C `traceparent` (continuing its trace-id) or starts a fresh trace, and INJECTS the new trace context (`traceparent` + `tracestate` pass-through) toward the upstream — proven cross-side EXACT (the injected `x-request-id` nibble + the `traceparent` continuation, via the `HTTPHeaderMutation` header-echo backend) + subject-side (the 5 HCM-scoped `tracing.*` counters) by the `0086-tracing-request-id` differential against `contrib-v1.37.2`. **No span is built and nothing is exported at 46.1a** — the per-request SERVER span, the `OTLPTracesClient`, the `OTLPExporter`, and the `0087-tracing-otlp` span differential are the 46.1b sub-leg.

**Architecture:** This is the project's FIRST request-tracing subsystem and its FIRST genuinely-new request-path Go package in a long stretch (ONE new package `internal/tracing`; ZERO new go.mod modules). It lifts the HCM `tracing` field (proto field 7) out of the ADR-0041 silent-ignore set — but the parse lands in the **HCM filter** (`internal/filter/hcm/config.go:parseFilterWithCtx`, where `stat_prefix` + the registry + the existing `http.<stat_prefix>.downstream_rq_*` counters already live and where the `stats.IsValidName` stat_prefix guard already runs), NOT in `bootstrap.go` (the SPEC §3.1 guess — refined here per D-TRACE-CONFIG-HOME). The decision engine is a pure function (`tracing.Decide(http.Header, *TracingConfig, RandSource) Decision`); the HCM filter calls it at request dispatch (`connection.go:448`), stamps the headers, and increments the matching HCM-scoped counter. Byte-identical when no HCM `tracing` provider is configured (every non-tracing path untouched — the full differential is the regression anchor). The differential reuses the existing `HTTPHeaderMutation` backend (Kind 9) to capture the upstream-forwarded headers — NO new BackendKind, NO receiver. ANCHORS the engine half of ADR-0260 (whose §Decision/§Consequences body lands at the 46.1b IMPL, the completing sub-leg); row 46 STAYS `in-progress`; the Observability family STAYS OPEN.

**Tech Stack:** Go; the NEW `internal/tracing` package; `internal/filter/hcm` (the `Filter` struct + `parseFilterWithCtx` config parse + the `connection.go` dispatch seam); `internal/bootstrap` (the ADR-0016 proto blank-import registry for the OTel tracer config proto); the resolved `go-control-plane/envoy v1.32.4` tracing config protos (`config/trace/v3.OpenTelemetryConfig` + the HCM `Tracing` message + `type.v3.Percent` + `type.tracing.v3.CustomTag` for reject-detect); `net/http` `Header` propagation; `crypto/rand` + `math/rand/v2` (behind the `RandSource` seam); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY — the trace config protos resolve at the already-present `go-control-plane/envoy` module).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. There is NO request-tracing today: the HTTP Connection Manager (HCM) silently ignores its `tracing` field (the ADR-0041 posture), and the router EXPLICITLY does NOT inject `x-request-id`/`x-envoy-*`/`x-forwarded-*` headers (`router.go:763`). You are adding the FIRST tracing subsystem — but only the **header-level engine** half: the sampling/request-id decision, the `x-request-id` UUID generation/stamping, and the W3C `traceparent`/`tracestate` extract-continue-inject. You build a NEW `internal/tracing` package (pure logic — no gRPC, no goroutine, no proto wire) and wire it into the HCM filter at request dispatch. You do NOT build a span, an exporter, or an OTLP client — those are the 46.1b sub-leg.

**Why the parse lives in the HCM filter, not bootstrap.** Envoy's `HttpConnectionManager` proto is unmarshalled TWICE in this codebase: once in `bootstrap.go` (to walk `access_log[]`) and once in `internal/filter/hcm/config.go:206` (`parseFilterWithCtx`, the REAL HCM filter build). The HCM-scoped stats (`http.<stat_prefix>.downstream_rq_total` …) — the exact shape the 5 new `tracing.*` counters mirror — are registered in `parseFilterWithCtx` (config.go:285–295), gated by an `IsValidName` stat_prefix check (config.go:235). The `tracing` field, the `stat_prefix`, the registry, and the per-request dispatch path are ALL in package `hcm`. So the tracing config parse + the counters + the decision-engine call all live in package `hcm`; `internal/tracing` is the reusable pure-logic engine they call.

**The decision engine in one breath (the §11 live-probe precedence, all pinned).** Per request, with the incoming `http.Header` + the parsed `*TracingConfig`:
1. An incoming **`traceparent`** (W3C `00-<32hex traceid>-<16hex parentid>-<2hex flags>`) ⇒ CONTINUE: keep its trace-id, set `ParentSpanID` = its parent-id, `Sample` = its sampled-bit — authoritative, NOT subject to the local `random_sampling`/`overall_sampling` caps. Class = `not_traceable`. The `x-request-id` nibble stays `4` (NoTrace — the reason reflects only LOCAL random/force decisions).
2. else **`x-client-trace-id`** present ⇒ FORCE candidate, gated by `client_sampling` (`client_sampling=0` drops the force). Class = `client_enabled`. Nibble = `b` (Client).
3. else **`random_sampling`** roll decides. Class = `random_sampling`. Nibble = `9` (Sampled) when it fires, else `4` (NoTrace).
4. THEN **`overall_sampling`** caps the LOCALLY-decided candidate (steps 2–3 only; NOT step 1; `overall_sampling=0` suppresses BOTH random AND client-force).

A fresh trace generates a 16-byte trace-id + an 8-byte span-id (the span-id goes into the upstream `traceparent` and — at 46.1b — becomes the SERVER span's `span_id`). The `x-request-id` is GENERATED when absent (a UUIDv4 with string-index-14 set to the reason nibble) or PRESERVED-verbatim-and-stamped when present (overwrite ONLY string index 14). The `x-request-id` is generated + injected upstream REGARDLESS of the sampling decision (a not-sampled request still gets a fresh `x-request-id` + an upstream `traceparent` with flags `00`).

**Knob defaults:** absent `client_sampling`/`random_sampling`/`overall_sampling` ⇒ 100.0 (the reference default). The `value` is a `float64` percentage in `[0,100]`.

The differential test harness boots BOTH the real reference Envoy (Docker, `contrib-v1.37.2`) AND the in-process subject (envoy-go) against equivalent bootstraps, drives the same traffic, and asserts equivalence. For `0086-tracing-request-id`, BOTH sides route to the existing `HTTPHeaderMutation` backend (Kind 9), which reflects every received request header into its response body (sorted). The driver fires N requests, reads each side's echoed body, extracts the upstream-forwarded `x-request-id` + `traceparent`, and asserts the trace-reason nibble + the trace-id continuation match cross-side (the VALUES vary — only structure is asserted), plus the subject-side `http.<prefix>.tracing.*` counter values via `/stats`.

### Key source seams (verified at PLAN time against master `48a6a3ac`; re-confirm line numbers before editing — files evolve)

- **`internal/filter/hcm/config.go`** — `type Filter struct` (`:91` — re-confirmed at PLAN review; ~10-line drift off the earlier `:81` note) with `statPrefix string` + the 5 HCM-scoped counters `downstreamRqTotal`/`downstreamRq2xx`/`…Rq3xx`/`…Rq4xx`/`…Rq5xx *stats.Counter` (`:101`–`:105`). `parseFilterWithCtx(...)` (`:202`–`:313`) is the HCM config parser: it `proto.Unmarshal`s the `HttpConnectionManager` (`:206`), validates the stat_prefix (`if !stats.IsValidName("http." + statPrefix + ".downstream_rq_total")` `:235`), and constructs the `Filter` with `prefix := "http." + statPrefix + "."` + `registry.NewCounter(prefix + "downstream_rq_total")` ×5 (`:285`–`:295`). **The `tracing` field is NOT read today** (`hcm.GetTracing()` appears nowhere). **This is the home for: the `*TracingConfig` Filter field, the `tracing` parse arm + the 8 STRICT-REJECT arms, the 5 `tracing.*` counters (registered alongside `:291`–`:295`, prefix ALREADY validated at `:235`), and the decision-engine-config the dispatch path reads.**
- **`internal/filter/hcm/filter.go`** — `NewFilterWithCtxAndSinksAndRegistry(tc *anypb.Any, clusters, lc ListenerCtx, registry *stats.Registry, accessLogSinks, httpRegistry, dm)` (`:88`) is the Filter constructor (calls `parseFilterWithCtx`); `NewNetworkFactory(...)` (`:36`) is the boot wrapper closed over the boot singletons. **No new constructor param needed** — the registry + the parsed `tracing` config are both already in scope inside `parseFilterWithCtx`.
- **`internal/filter/network/builtins/builtins.go:55`** — `reg.Register(hcm.TypeURL, hcm.NewNetworkFactory(deps.ClusterManager, deps.StatsRegistry, …))` — the HCM boot registration. **UNCHANGED at 46.1a** (no new dependency threads through; the engine is constructed inside `parseFilterWithCtx`).
- **`internal/filter/hcm/connection.go`** — `func (f *Filter) dispatchRequest(ctx context.Context, downstream net.Conn, req *http.Request, bw *bufio.Writer) (int, error)` (`:311`); `startTime := time.Now()` (`:448`) inside `dispatchRequest`, with `req *http.Request` (the full request headers, mutable via `req.Header`), `ctx`, `downstream` (the source conn) all in scope, just after the router-filter action is set and the pseudo-headers (`:method`/`:authority`/`:path`) are seeded. **This is the span-start / decision point** — when `f.tracingConfig != nil`, run `tracing.Decide(req.Header, f.tracingConfig, f.rng)`, stamp `x-request-id` + inject `traceparent`/`tracestate` onto `req.Header`, and increment the matching `f.tracing*` counter. (The H2 dispatch path has an analogous seam — confirm + wire both; the request headers there are `h2.H2Request` fields.)
- **`internal/filter/hcm/accesslog_emit.go`** — `emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders …)` (`:19`, H1) / `emitAccessLogH2(req h2.H2Request, …)` (`:45`, H2). **NOT TOUCHED at 46.1a** (this is the SPAN-END seam — a 46.1b concern; 46.1a's decision is dispatch-only and is not carried here).
- **`internal/filter/http/router/router.go:763`** — the comment + guarantee that the router does NOT inject `x-request-id`/`x-envoy-*`/`x-forwarded-*`. **This invariant is REVISED — but ONLY under a configured HCM `tracing` provider** (the injection happens at the HCM dispatch seam, not the router; the router stays untouched; no provider ⇒ the path stays byte-identical). Update the comment to cross-reference the HCM tracing engine.
- **`internal/grpcclient/grpcclient.go:319`–`375`** — the `OTLPLogsClient` UNARY typed wrapper (57 LoC: struct + `NewOTLPLogsClient` + `Export` + `Close`). **NOT TOUCHED at 46.1a** — it is the template for the 46.1b `OTLPTracesClient`. (`coltracepb`/`collector/trace/v1` is NOT imported anywhere yet — a 46.1b import.)
- **`internal/accesslog/otlpsink.go`** (216 LoC) — the bounded-channel + writer-goroutine + size/interval/close buffer sink. **NOT TOUCHED at 46.1a** — the template for the 46.1b `OTLPExporter`.
- **`internal/stats/registry.go`** — `NewCounter(name) *Counter` (panics on invalid name — boot-time, ADR-0059), `NewCounterIfAbsent(name)` (post-Freeze, ADR-0117), `IsValidName(name) bool` (`:55`, the user-input-boundary guard), `Freeze()`. The tracing counters are allocated PRE-Freeze in `parseFilterWithCtx` via `NewCounter`.
- **`test/differential/fixture/fixture.go`** — the `BackendKind` enum, tail `H2GoawayResponder = 38`. **`HTTPHeaderMutation = 9`** (`:192`–`:202`): "Reflects received request headers into the response body, sorted for determinism." **UNCHANGED in 46.1a** (the differential reuses Kind 9).
- **`test/fixtures/0012-http-header-mutation/backends/backend.go:21`–`36`** — the `HTTPHeaderMutation` backend: it sorts `r.Header` keys and writes `"%s: %s\n"` per header/value into the response body. **This is how the `0086` driver reads the upstream-forwarded `x-request-id`/`traceparent`** (parse the echoed body lines).
- **`test/helpers/http_diff.go`** — the cross-side header-diff allowlist EXCLUDES `x-request-id` (reference Envoy injects it by default + the value varies). **The `0086` driver does NOT use the generic byte-compare for these headers** — it uses a CUSTOM asserter (parse the echoed body → extract `x-request-id`/`traceparent` → assert the nibble + continuation structurally), like the `0084` `AssertStats` shape.
- **`test/fixtures/0084-otlp-access-log/driver/driver.go`** — the driver-owned-assertion fixture shape (`func init() { fixture.RegisterFixture(...) }`, `BackendKind()`, `ReferenceBootstrap`/`SubjectConfig` templating, `DriveReference`/`DriveSubject` firing N requests + snapshotting, `AssertStats` cross-side + subject-`/stats`). **COPY the driver skeleton** (swap the OTLP receiver for the header-echo parse).
- **`test/differential/runner_test.go:111`–`112`** — the fixture-driver blank-import block (`_ ".../0084-otlp-access-log/driver"` / `_ ".../0085-otlp-access-log-operators/driver"`). Add the `0086` driver import here.

### Proto facts (verified at PLAN time against `go-control-plane/envoy@v1.32.4`; re-confirm at IMPL)

The HCM `Tracing` message lives at `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3` (the same package as `HttpConnectionManager` — already imported as `hcmv3` in config.go). Fields by tag (SPEC §5, re-verify):
- `HttpConnectionManager.Tracing *HttpConnectionManager_Tracing` (field **7** of the HCM); getter `.GetTracing()`.
- `HttpConnectionManager_Tracing`: `client_sampling *typev3.Percent` (3) / `random_sampling *typev3.Percent` (4) / `overall_sampling *typev3.Percent` (5) **CONSUMED**; `verbose bool` (6) **REJECT-if-true**; `max_path_tag_length *wrapperspb.UInt32Value` (7) **REJECT-if-present**; `custom_tags []*typetracingv3.CustomTag` (8) **REJECT-if-non-empty**; `provider *Tracing_Http` (9) **CONSUMED** (the OTel arm); `spawn_upstream_span *wrapperspb.BoolValue` (10) **REJECT-if-present** (D-TRACE-SPAWN-UPSTREAM-SPAN below).
- `Tracing_Http` (the provider wrapper, from `config/trace/v3`): `name string` (1) + `typed_config *anypb.Any` (oneof `config_type`) **CONSUMED** — the OTel provider only.
- `config/trace/v3.OpenTelemetryConfig` (TypeURL `type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig`): `grpc_service *corev3.GrpcService` (1) **CONSUMED** (`envoy_grpc.cluster_name`; `google_grpc` reject); `service_name string` (2) **CONSUMED** (stored for the 46.1b `Resource.service.name`; unused at 46.1a); `http_service` (3) / `resource_detectors` (4) / `sampler` (5) **REJECT-if-present**.
- `type/v3.Percent`: `.GetValue() float64`. `type/tracing/v3.CustomTag`: reject-detect only (non-empty list). `core/v3.GrpcService.GetEnvoyGrpc().GetClusterName()`; `ApiVersion_V2 = 1` (the deprecated-transport reject, the 44.x-parity arm).
- **Blank-import** `github.com/envoyproxy/go-control-plane/envoy/config/trace/v3` (ADR-0016) so the nested `provider.typed_config` Any (`OpenTelemetryConfig`) round-trips through protojson at bootstrap unmarshal. The HCM `tracing` message types (`Percent`, `CustomTag`) resolve transitively. `go mod tidy -diff` shows NO require change (all at the already-present `go-control-plane/envoy` module).

### W3C `traceparent` wire format (the `reference_wire_format_both_sides_see_same_bytes` discipline — adopt the reference framing verbatim, §11 D-TRACE-PROPAGATION)

`traceparent: 00-<32 lowercase-hex trace-id>-<16 lowercase-hex parent-id>-<2 hex flags>`. `version` = `00`. `flags` bit 0 = sampled (`01` sampled / `00` not). A trace-id of all-zero or a parent-id of all-zero is INVALID (treat as no-incoming-context). `tracestate` is an opaque comma-list — passed through verbatim (no vendor-entry mutation). On a fresh trace: generate a random 16-byte trace-id + 8-byte span-id; the upstream `traceparent` = `00-<new-trace-id>-<new-span-id>-<flags from the sample decision>`. On a continued trace: upstream `traceparent` = `00-<incoming-trace-id>-<new-span-id>-<propagated flags>`; the incoming parent-id is recorded as `ParentSpanID` (for the 46.1b span).

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. A leaked gofmt drift bit 26.3 — do NOT skip.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): subagents write to the WORKTREE path; the controller verifies `git -C <main-checkout> status` stays clean after each task and that the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only** (`feedback_subagents_no_push`): subagents NEVER push; the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0086'`, NEVER bare `'0086'` (which matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): 46.1a adds NO background goroutine (the decision engine is synchronous, called inline at dispatch), so the `-race` gate is the normal full-package run, not a background-mutator concern — still run the full `internal/filter/hcm` + `internal/tracing` packages `-race`.
- **Startup flake** (`reference_differential_fullsuite_startup_flake`): a `subject ready: EOF` in the full suite is a transient startup race on an UNRELATED fixture — isolate-re-run to distinguish from a regression.
- **Dynamic stat-name guard** (`reference_dynamic_stat_name_charset_guard`): the `http.<stat_prefix>.tracing.*` names carry the DYNAMIC `stat_prefix` segment — but the existing `config.go:235` `IsValidName` guard ALREADY validates the shared `http.<stat_prefix>.` prefix before any counter is registered, so the static `tracing.<class>` suffixes need NO second guard. Register the tracing counters in the same `parseFilterWithCtx` block, AFTER the `:235` guard.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): adopt the W3C `traceparent`/UUID framing VERBATIM — "our format" is never a valid deviation (the wire/header is shared; the differential's cross-side echo proves it).

---

## D-question resolutions (the SPEC §12 D-TRACE-* PLAN pins — settled here)

**D-TRACE-SPLIT → SUB-SPLIT (this PLAN is 46.1a; 46.1b follows under the same SPEC-46.1.md).** The real-LoC re-check (the as-built `otlpsink.go` 216 LoC + `OTLPLogsClient` 57 LoC + the engine pieces) put the full 46.1 leg at **~840 prod LoC** — ~2.7× the ~305-LoC one-leg 45.1 precedent (which sat "right at the ADR-0045 soft gate"). Past the gate ⇒ sub-split BY-CONCERN (the SPEC §3.0 pre-authorized contingency). The boundary, refined from the SPEC sketch to put ALL header propagation in 46.1a (the cleaner, more-testable cut):
- **46.1a (this PLAN, ~505 prod LoC) — the header-level tracing engine:** the `internal/tracing` package (decision + propagation extract/inject + request-id + RandSource + the HCM-counter helper) + the HCM-filter `tracing` parse arm + the 8 STRICT-REJECT arms + the 5 HCM-scoped `tracing.*` counters + the dispatch wiring (decision + `x-request-id` stamp + `traceparent` inject) + `FuzzExtractTraceparent` + `FuzzStampRequestID`. Subject-observable (the injected `x-request-id` nibble + the `traceparent` + the counters), proven by `0086-tracing-request-id` (the header-echo backend; NO span, NO exporter, NO receiver).
- **46.1b (next PLAN, ~480 prod LoC) — span emission + OTLP export:** the `span.go` model + the 16-attr roster + the `OTLPTracesClient` + the `OTLPExporter` + the span-lifecycle wiring (carry the `Decision` to the `accesslog_emit.go` end-seam) + the tracer-scoped `tracing.opentelemetry.{spans_sent,spans_dropped}` counters + the `log.Fatalf` collector-cluster gate + the `test/helpers/otlptrace` receiver + the `0087-tracing-otlp` span differential. CLOSES ADR-0260.

LoC breakdown (46.1a): `internal/tracing` decision.go ~90 / propagation.go ~95 (extract+inject+tracestate) / requestid.go ~55 / rand.go ~20 / stats.go (HCMCounters) ~30 = ~290; HCM parse arm + 8 rejects + blank-import ~120; dispatch wiring (H1+H2) ~70; counter registration in parseFilterWithCtx ~15; main/boot ~0 (no new wiring). **≈ 495–505 prod LoC** — a sensible single sub-leg (above the soft gate but coherent + independently testable; further splitting would fragment the engine).

**D-TRACE-CONFIG-HOME → `TracingConfig` lives in `internal/tracing`; PARSED in the HCM filter (`internal/filter/hcm/config.go:parseFilterWithCtx`), NOT `bootstrap.go`.** The SPEC §3.1 guessed `bootstrap.go:354`, but that unmarshal only walks `access_log[]`; the REAL HCM filter build (`config.go:parseFilterWithCtx`) is where `stat_prefix` + the registry + the `downstream_rq_*` counters + the `IsValidName` guard already live. So: the `tracing.TracingConfig` struct lives in `internal/tracing` (the engine package that consumes it — `Decide` takes `*TracingConfig`); package `hcm` imports `internal/tracing` to build a `*TracingConfig` from `hcm.GetTracing()` and to hold it + the `*tracing.HCMCounters` on the `Filter`. Acyclic: `hcm → tracing → stats` (tracing imports `stats`, `net/http`, `crypto/rand`, `math/rand/v2`, the go-control-plane trace protos; tracing imports NOTHING from `hcm`/`bootstrap`). The blank-import (`config/trace/v3`) lands in `internal/bootstrap` alongside the existing access-logger blank-imports (the registry-population home — ADR-0016) so the protojson round-trip at bootstrap unmarshal resolves the nested `OpenTelemetryConfig` Any.

```go
// internal/tracing/config.go
type TracingConfig struct {
    // sampling (the three Percent knobs; absent ⇒ 100.0 per the reference default)
    ClientSampling  float64 // client_sampling.value
    RandomSampling  float64 // random_sampling.value
    OverallSampling float64 // overall_sampling.value
    // OTel provider (46.1a accept set; ServiceName/ClusterName stored, UNUSED at 46.1a — 46.1b dials/labels)
    ServiceName string // OpenTelemetryConfig.service_name → the 46.1b Resource service.name
    ClusterName string // OpenTelemetryConfig.grpc_service.envoy_grpc.cluster_name → the 46.1b exporter dial
}
```
The cluster-exists / non-H2 gate + the `log.Fatalf` are 46.1b (no exporter dials the cluster at 46.1a) — so 46.1a accepts a `ClusterName` that does not yet resolve to a built cluster (stored, unused). The `0086` fixture defines a dummy collector cluster anyway (so the config is 46.1b-ready and the reference's data plane is satisfied).

**D-TRACE-RNG-SEAM → `tracing.RandSource interface { Float64() float64; Read([]byte) (int, error) }`; production = crypto/rand for `Read` (ids) + math/rand/v2 for `Float64` (the sampling roll); unit tests inject a deterministic fake.** `Float64()` drives the `random_sampling`/`overall_sampling`/`client_sampling` cap comparisons (`roll := rng.Float64()*100; sampled := roll < pct`); `Read(p)` fills the 16-byte trace-id + 8-byte span-id + the random UUID bytes. The differential forces `random_sampling=100%` (any `Float64` < 100 ⇒ always samples — the roll is irrelevant) and never asserts the id VALUES (only the nibble + continuation), so production crypto/rand needs no seeding. The unit tests inject a fake whose `Float64` returns a programmed sequence (to force sample/not-sample at a cap boundary — e.g. `random_sampling=50`, roll 0.4 ⇒ sample, 0.6 ⇒ not) and whose `Read` fills deterministic bytes (to assert a known trace-id/span-id is emitted). The `Filter` holds one `rng tracing.RandSource` (a process-shared `tracing.CryptoRand{}` in production), set at `parseFilterWithCtx`.

**D-TRACE-RECEIVER-WIRING → at 46.1a there is NO receiver; the `0086-tracing-request-id` differential uses the existing `HTTPHeaderMutation` backend (Kind 9) to capture the upstream-forwarded headers.** The receiver (`test/helpers/otlptrace`) + the shared-bridge OTLP reachability are a 46.1b concern (the span exporter). At 46.1a the only observable is the request headers the proxy forwards upstream — captured by the `HTTPHeaderMutation` backend's echo-into-body. The driver parses the echoed body for `x-request-id` + `traceparent`. **BackendKind STAYS 38; no new helper.** The continuation prong (an incoming `traceparent` with a fixed trace-id) is asserted from the SAME echoed body (the upstream `traceparent`'s trace-id == the incoming trace-id). One fixture dir, one cross-side assertion branch (`reference_differential_fixture_dispatch_constraint` satisfied — the boot-reject prong is a 46.1b `log.Fatalf` concern, deferred with the exporter).

**D-TRACE-STATS-FINAL (46.1a slice) → the 5 HCM-scoped `http.<stat_prefix>.tracing.{client_enabled,health_check,not_traceable,random_sampling,service_forced}` counters; surface 1191 → 1196 (+5).** Registered in `parseFilterWithCtx` alongside the `downstream_rq_*` block (prefix already `IsValidName`-validated at `:235`). `client_enabled` / `not_traceable` / `random_sampling` are LIVE (the §11 decision classes); `health_check` (health-check requests) + `service_forced` (the `x-envoy-force-trace` path, SPEC §2) register but stay 0 at 46.1a (parse-inert — no health-check detection / force-header honoring in scope). The tracer-scoped `tracing.opentelemetry.{spans_sent,spans_dropped}` (+2 → 1198) are 46.1b. Confirm the +5 DELTA via a registration test under a tracing config (assert the delta, not the brittle absolute). Counters register ONLY when a tracing HCM is configured (DYNAMIC — they do not exist in a no-tracing boot).

**D-TRACE-FUZZER → land `FuzzExtractTraceparent` + `FuzzStampRequestID` at 46.1a (fuzzers 46 → 48).** `FuzzExtractTraceparent` (the wire-derived `traceparent`/`tracestate` parse is an untrusted-input boundary — no-panic over arbitrary header strings) + `FuzzStampRequestID` (the index-14 overwrite over arbitrary inbound `x-request-id` strings — no-panic + the output stays UUID-shaped / index-14 == the reason nibble). The existing `FuzzHCMConfigParse` now ALSO exercises the new `tracing` parse arm (broader coverage, NO new fuzzer for the parse). The actual `^func Fuzz` count is **46** (verified at PLAN time) + the documented running total is **46** (no drift) ⇒ the two new fuzzers advance both to **48**. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 48 at the completion task (`reference_fuzzer_count_docs_drift`).

**D-TRACE-SPANEND-PLUMBING → DEFERRED to 46.1b (no span at 46.1a).** The `Decision` is computed at dispatch, applied to the headers + counters there, and discarded — it is NOT carried to the `accesslog_emit.go` end-seam (that plumbing arrives with the span at 46.1b, where the `Decision`'s `TraceID`/`SpanID`/`ParentSpanID` build the SERVER span). 46.1a touches `connection.go` (dispatch) only, NOT `accesslog_emit.go`.

**D-TRACE-SPAWN-UPSTREAM-SPAN (the SPEC §3.1 NIT — present-vs-`true` reject semantics) → STRICT-REJECT when the field is PRESENT (non-nil), regardless of value.** `spawn_upstream_span` is a `*wrapperspb.BoolValue`; the reference treats `true` as "add a 2nd CLIENT span" (out of scope) and absent/`false` as the single-SERVER-span default. envoy-go-strict rejects the field whenever it is PRESENT (non-nil) — even `spawn_upstream_span: false` — because honoring a present-but-false as accept-inert would silently diverge from "we emit exactly the single SERVER span and recognize no upstream-span knob." A user who wants the default simply omits the field. (This matches the `custom_tags`/`max_path_tag_length` present-rejects: present ⇒ reject; absent ⇒ default.)

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/tracing/config.go` — `TracingConfig` struct + the knob defaults + `BuildConfig(t *hcmtracing.HttpConnectionManager_Tracing) (*TracingConfig, error)` (the proto→struct + the 8 STRICT-REJECT arms) — OR the rejects live in `hcm/config.go` calling a thinner `tracing.NewConfig` (locked at Task 5; default: the reject arms live in `internal/tracing` so the engine owns its config validation + the parse fuzzer targets one function).
- `internal/tracing/rand.go` — `RandSource` interface + `CryptoRand` production impl.
- `internal/tracing/decision.go` — `TraceReason` + `SampleClass` + `Decision` struct + `Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision`.
- `internal/tracing/requestid.go` — `GenerateRequestID(reason TraceReason, rng RandSource) string` + `StampRequestID(existing string, reason TraceReason) string` (the index-14 overwrite).
- `internal/tracing/propagation.go` — `TraceContext` + `ExtractTraceparent(h http.Header) (TraceContext, bool)` + `InjectTraceparent(h http.Header, traceID [16]byte, spanID [8]byte, sampled bool, tracestate string)`.
- `internal/tracing/stats.go` — `HCMCounters` struct (the 5 counters) + `RegisterHCMCounters(reg *stats.Registry, statPrefix string) (*HCMCounters, error)` + `(*HCMCounters).Record(class SampleClass)`.
- `internal/bootstrap/bootstrap.go` — MODIFY: the `config/trace/v3` blank-import (ADR-0016).
- `internal/filter/hcm/config.go` — MODIFY: the `*tracing.TracingConfig` + `*tracing.HCMCounters` + `rng tracing.RandSource` Filter fields; the `tracing` parse arm in `parseFilterWithCtx` (call `tracing.NewConfig(hcm.GetTracing())` → store; register the 5 counters; set `rng`).
- `internal/filter/hcm/connection.go` — MODIFY: the dispatch-seam decision call (H1 at `:448` + the H2 analogue) — `Decide` → stamp `x-request-id` + inject `traceparent`/`tracestate` + `counters.Record(decision.Class)`.
- `internal/filter/http/router/router.go` — MODIFY (comment only): cross-reference the HCM tracing engine on the `:763` no-inject comment.

**Test (created):**
- `internal/tracing/decision_test.go` — the table-driven precedence/classification/nibble tests (against a fake `RandSource`).
- `internal/tracing/requestid_test.go` — generate/preserve/stamp + `FuzzStampRequestID`.
- `internal/tracing/propagation_test.go` — extract/inject/tracestate + `FuzzExtractTraceparent`.
- `internal/tracing/config_test.go` — the parse-accept + the 8 STRICT-REJECT arm tests.
- `internal/tracing/stats_test.go` — the `RegisterHCMCounters` +5 registration test + `Record` dispatch.
- `internal/filter/hcm/config_test.go` — MODIFY: the HCM-level tracing parse-accept + reject + the byte-stable (no-tracing ⇒ no counters) test.
- `internal/filter/hcm/connection_test.go` (or the existing HCM filter test) — MODIFY: the dispatch decision-applies-headers + counter test (no-tracing ⇒ untouched).
- `test/fixtures/0086-tracing-request-id/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}` — CREATE.
- `test/differential/runner_test.go` — MODIFY: blank-import the `0086` driver.

**Docs (completion task):**
- `docs/envoy-go/phases/46-tracing/PROGRESS-46.1a.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (ADR-0260 body stays deferred to 46.1b).

---

## Task 1: Phase scaffolding — PROGRESS-46.1a.md + baselines + the D-TRACE-SPLIT re-check record

**Files:**
- Create: `docs/envoy-go/phases/46-tracing/PROGRESS-46.1a.md`

- [ ] **Step 1: Record the baseline counts**

Run and record the verbatim outputs in PROGRESS-46.1a.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                 # expect 87 (tail 0085-otlp-access-log-operators)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                  # expect 46
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go  # expect = 38 (the BackendKind tail)
grep -rn 'GetTracing\|traceparent\|x-request-id' internal/ --include='*.go' | grep -v _test  # expect: router.go:763 (the no-inject comment) + the extauthz x-request-id reader (extauthz.go:~1033, UNRELATED — an ext_authz request-header pass-through, not tracing); NO production tracing engine + NO hcm.GetTracing() call
```
Baseline: stat surface **1191** (H2 cluster; non-H2 **1187**) / fixtures **87** / fuzzers **46** / BackendKind **38** / DECISIONS tail **ADR-0259** (next-free **ADR-0260**).

- [ ] **Step 2: Write the PROGRESS-46.1a.md scaffold** — a header (phase 46.1a IMPL, the SPEC-46.1 reference + the "46.1a sub-leg of the 46.1 by-exporter leg" note, the worktree branch), a task checklist mirroring this plan, the baseline-counts block, and the anticipated exit counts: stat **1196** (+5 — `http.<stat_prefix>.tracing.{client_enabled,health_check,not_traceable,random_sampling,service_forced}`) / fixtures **88** (`0086-tracing-request-id`) / fuzzers **48** (`FuzzExtractTraceparent` + `FuzzStampRequestID`) / BackendKind **38** (UNCHANGED) / DECISIONS **ADR-0259** (ADR-0260 body lands at 46.1b) / **0 new go.mod modules**.

- [ ] **Step 3: Record the D-TRACE-SPLIT re-check** — note the ~840-LoC full-leg estimate, the ~505-LoC 46.1a slice (the breakdown above), the 46.1a/46.1b boundary table, and that ADR-0260 closes at the 46.1b IMPL. (Bookkeeping re-check, not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/46-tracing/PROGRESS-46.1a.md
git commit -m "phase 46.1a Task 1: PROGRESS scaffold + baselines + the D-TRACE-SPLIT re-check (46.1a/46.1b boundary)"
```

---

## Task 2: The `internal/tracing` package skeleton — `TracingConfig` + the `RandSource` seam [TDD]

**Files:**
- Create: `internal/tracing/config.go`, `internal/tracing/rand.go`
- Test: `internal/tracing/config_test.go` (the defaults), `internal/tracing/rand_test.go`

These are the foundation types — no proto, no http yet (the proto→config parse is Task 5; this task establishes the struct + the RNG seam the decision engine consumes).

- [ ] **Step 1: Write the failing tests**
  - `rand_test.go`: a `fakeRand{floats []float64; bytes []byte}` implementing `RandSource` (the test seam) returns programmed `Float64()` values + fills `Read(p)` from `bytes`; assert `CryptoRand{}.Read(make([]byte,16))` returns `(16, nil)` + two successive reads differ (non-deterministic) + `CryptoRand{}.Float64()` is in `[0,1)`.
  - `config_test.go` (defaults only here): a helper `defaultConfig()` returning `&TracingConfig{ClientSampling:100, RandomSampling:100, OverallSampling:100}` round-trips (placeholder until Task 5 adds the proto parse); assert the zero-knob handling is documented (a value of 0.0 is a VALID 0% sample, distinct from absent⇒100 — that distinction is applied at parse time, Task 5).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -count=1` ⇒ FAIL (package/symbols undefined).

- [ ] **Step 3: Implement** `rand.go`:
```go
package tracing

import (
    crand "crypto/rand"
    mrand "math/rand/v2"
)

// RandSource is the injectable randomness seam (D-TRACE-RNG-SEAM). Float64 drives
// the sampling-cap comparisons; Read fills the trace-id / span-id / UUID bytes.
// Production uses CryptoRand; unit tests inject a deterministic fake to force each
// sampling branch + a known id.
type RandSource interface {
    Float64() float64
    Read(p []byte) (int, error)
}

// CryptoRand is the production RandSource: crypto/rand for id bytes (unguessable),
// math/rand/v2 for the sampling roll (statistical, no crypto requirement). The
// differential never asserts id VALUES, so no seeding is needed.
type CryptoRand struct{}

func (CryptoRand) Float64() float64        { return mrand.Float64() }
func (CryptoRand) Read(p []byte) (int, error) { return crand.Read(p) }
```
And `config.go` (the struct + defaults; the proto parse is Task 5):
```go
package tracing

// TracingConfig is the parsed HCM tracing config (D-TRACE-CONFIG-HOME — lives here,
// built from the HCM proto by NewConfig in Task 5, consumed by Decide). The three
// Percent knobs default to 100.0 when ABSENT (the reference default); an EXPLICIT 0
// is a valid 0% and is preserved by the parse. ServiceName/ClusterName are stored
// for the 46.1b exporter/Resource and are UNUSED at 46.1a.
type TracingConfig struct {
    ClientSampling  float64
    RandomSampling  float64
    OverallSampling float64
    ServiceName     string
    ClusterName     string
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/config.go internal/tracing/rand.go internal/tracing/rand_test.go internal/tracing/config_test.go
git commit -m "phase 46.1a Task 2: internal/tracing skeleton — TracingConfig + the RandSource seam (CryptoRand prod impl) (D-TRACE-RNG-SEAM)"
```

---

## Task 3: The request-id engine — generate / preserve / byte-14 stamp (`requestid.go`) [TDD] + `FuzzStampRequestID`

**Files:**
- Create: `internal/tracing/requestid.go`, `internal/tracing/requestid_test.go`

The UUID + the trace-reason nibble (§11 D-TRACE-REQUESTID). The `TraceReason` enum is the shared vocabulary (also used by `decision.go`); define it here.

- [ ] **Step 1: Write the failing tests** in `requestid_test.go`:
  - `TraceReason` constants map to the nibble hex chars: `NoTrace`→`'4'`, `Sampled`→`'9'`, `Client`→`'b'` (a `reasonNibble(r) byte` helper).
  - `GenerateRequestID(Sampled, fake)` (fake `Read` fills 16 known bytes) ⇒ a 36-char UUIDv4-shaped string (`8-4-4-4-12`, hyphens at 8/13/18/23), with `out[14] == '9'` and a valid variant nibble at index 19 (`8`/`9`/`a`/`b`).
  - `GenerateRequestID(NoTrace, fake)` ⇒ `out[14] == '4'`; `GenerateRequestID(Client, fake)` ⇒ `out[14] == 'b'`.
  - `StampRequestID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Sampled)` ⇒ `"aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee"` (ONLY index 14 overwritten, the rest verbatim — §11 probe `…-cccc-…`→`…-9ccc-…`).
  - `StampRequestID(<malformed: too short / no hyphens>, Sampled)` ⇒ returns the input UNCHANGED when index 14 is out of range or not a hyphen-group position (defensive — never panic; the fuzzer covers the rest).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestRequestID -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `requestid.go`:
  - `TraceReason` (`int`/`byte` enum: `NoTrace`/`Sampled`/`Client`) + `reasonNibble(TraceReason) byte` (`'4'`/`'9'`/`'b'`).
  - `GenerateRequestID(reason, rng)`: read 16 bytes; set the UUIDv4 version nibble (byte 6 high-nibble = 4) + variant (byte 8 high bits = 10); format as canonical hex-with-hyphens; then overwrite STRING index 14 with `reasonNibble(reason)` (index 14 is the version nibble's hex char — the canonical layout puts byte-6-high-nibble at string index 14). 
  - `StampRequestID(existing, reason)`: if `len(existing) < 15` ⇒ return `existing` unchanged; else `[]byte(existing)`, set `[14] = reasonNibble(reason)`, return as string. (Preserve everything else verbatim — the reference does NOT re-validate the inbound id.)

> **Nibble-position note:** the canonical UUID string `xxxxxxxx-xxxx-Vxxx-...` puts the version char `V` at string index 14 (8 hex + 1 hyphen + 4 hex + 1 hyphen = index 14). Confirm with a literal in the test (`"........-....-4..."` ⇒ index 14 is the char after the 2nd hyphen). The §11 probe pinned `…-cccc-…`→`…-9ccc-…`, i.e. the char right after the 2nd hyphen — index 14. ✓

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestRequestID -count=1` ⇒ PASS.

- [ ] **Step 5: Add `FuzzStampRequestID`** (no-panic + invariant): for any input string `s`, `StampRequestID(s, Sampled)` never panics; if `len(s) >= 15` the output equals `s` with index 14 == `'9'` and all other indices unchanged; if `len(s) < 15` the output == `s`.
```go
func FuzzStampRequestID(f *testing.F) {
    f.Add("")
    f.Add("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
    f.Add("short")
    f.Fuzz(func(t *testing.T, s string) {
        out := StampRequestID(s, Sampled) // must not panic
        if len(s) >= 15 {
            if out[14] != '9' { t.Fatalf("index 14 = %q, want '9'", out[14]) }
            if out[:14] != s[:14] || out[15:] != s[15:] { t.Fatalf("stamp mutated bytes outside index 14") }
        } else if out != s {
            t.Fatalf("short input mutated: %q -> %q", s, out)
        }
    })
}
```
Run: `go test ./internal/tracing/ -run FuzzStampRequestID -count=1` then `-fuzz FuzzStampRequestID -fuzztime 20s` ⇒ no crashers.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go build ./...
git add internal/tracing/requestid.go internal/tracing/requestid_test.go
git commit -m "phase 46.1a Task 3: x-request-id generate/preserve/byte-14 stamp (NoTrace=4/Sampled=9/Client=b) + FuzzStampRequestID (D-TRACE-REQUESTID; fuzzers 46→47)"
```

---

## Task 4: W3C `traceparent`/`tracestate` extract + inject (`propagation.go`) [TDD] + `FuzzExtractTraceparent`

**Files:**
- Create: `internal/tracing/propagation.go`, `internal/tracing/propagation_test.go`

The header propagation (§11 D-TRACE-PROPAGATION). `reference_wire_format_both_sides_see_same_bytes` — the W3C framing verbatim.

- [ ] **Step 1: Write the failing tests** in `propagation_test.go`:
  - `ExtractTraceparent` of `{"Traceparent": ["00-0102...0f10-1112131415161718-01"]}` (a valid 32-hex trace-id + 16-hex parent-id + flags `01`) ⇒ `(TraceContext{TraceID:[16]{..}, ParentID:[8]{..}, Sampled:true}, true)`.
  - flags `00` ⇒ `Sampled:false`.
  - all-zero trace-id (`00-000…0-…-01`) ⇒ `(_, false)` (invalid); all-zero parent-id ⇒ `(_, false)`.
  - malformed (wrong field count / non-hex / wrong version `99-…` / short) ⇒ `(_, false)`.
  - missing header ⇒ `(_, false)`.
  - case-insensitive header key (`traceparent` vs `Traceparent`) ⇒ found (use `http.Header.Get`, which canonicalizes).
  - `InjectTraceparent(h, traceID, spanID, true, "")` ⇒ `h.Get("Traceparent") == "00-<32hex traceID>-<16hex spanID>-01"` (lowercase hex); `sampled=false` ⇒ flags `00`. With `tracestate="vendor=abc"` ⇒ `h.Get("Tracestate") == "vendor=abc"`; empty tracestate ⇒ NO `Tracestate` header written.
  - round-trip: inject then extract ⇒ the same trace-id + (spanID as parentID) + sampled-bit.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestPropagation -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `propagation.go`:
  - `type TraceContext struct { TraceID [16]byte; ParentID [8]byte; Sampled bool; TraceState string }`.
  - `ExtractTraceparent(h http.Header) (TraceContext, bool)`: `v := h.Get("Traceparent")`; split on `-` into 4 parts; require `parts[0]=="00"`, `len(parts[1])==32`, `len(parts[2])==16`, `len(parts[3])==2`, all hex; decode; reject all-zero trace-id / all-zero parent-id; `Sampled = parts[3] bit0`; `TraceState = h.Get("Tracestate")`; return `(ctx, true)`. Any failure ⇒ `(TraceContext{}, false)`.
  - `InjectTraceparent(h http.Header, traceID [16]byte, spanID [8]byte, sampled bool, tracestate string)`: `flags := "00"; if sampled { flags = "01" }`; `h.Set("Traceparent", "00-"+hex(traceID)+"-"+hex(spanID)+"-"+flags)`; `if tracestate != "" { h.Set("Tracestate", tracestate) }`.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestPropagation -count=1` ⇒ PASS.

- [ ] **Step 5: Add `FuzzExtractTraceparent`** (no-panic over arbitrary header values):
```go
func FuzzExtractTraceparent(f *testing.F) {
    f.Add("00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01")
    f.Add("")
    f.Add("garbage-not-a-traceparent")
    f.Fuzz(func(t *testing.T, v string) {
        h := http.Header{}
        h.Set("Traceparent", v)
        ctx, ok := ExtractTraceparent(h) // must not panic
        if ok {
            // a valid parse never yields an all-zero trace-id / parent-id
            if ctx.TraceID == ([16]byte{}) || ctx.ParentID == ([8]byte{}) {
                t.Fatalf("ok parse with zero id: %q", v)
            }
        }
    })
}
```
Run: `go test ./internal/tracing/ -run FuzzExtractTraceparent -count=1` then `-fuzz FuzzExtractTraceparent -fuzztime 20s` ⇒ no crashers.

- [ ] **Step 6: Confirm the fuzzer count advanced** — `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **48** (was 46; +`FuzzStampRequestID` Task 3, +`FuzzExtractTraceparent` here). Record in PROGRESS-46.1a.md.

- [ ] **Step 7: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go build ./...
git add internal/tracing/propagation.go internal/tracing/propagation_test.go
git commit -m "phase 46.1a Task 4: W3C traceparent extract/inject + tracestate pass-through + FuzzExtractTraceparent (D-TRACE-PROPAGATION; fuzzers 47→48)"
```

---

## Task 5: The proto→`TracingConfig` parse + the 8 STRICT-REJECT arms (`tracing.NewConfig`) [TDD, table-driven]

**Files:**
- Modify: `internal/tracing/config.go`
- Test: `internal/tracing/config_test.go`

The reject arms live in `internal/tracing` (the engine owns its config validation; one parse fuzzer target). `NewConfig` takes the typed HCM `Tracing` message + the resolved provider Any.

- [ ] **Step 1: Write the failing table tests** in `config_test.go` (build `*hcmtracing.HttpConnectionManager_Tracing` proto literals — import the go-control-plane HCM package; for the provider, marshal an `OpenTelemetryConfig` into the `provider.typed_config` Any):
  - **accept-minimal**: a `provider` with an `OpenTelemetryConfig{ grpc_service: envoy_grpc{ cluster_name: "c" }, service_name: "svc" }`, no sampling knobs ⇒ `NewConfig` returns `&TracingConfig{ClientSampling:100, RandomSampling:100, OverallSampling:100, ServiceName:"svc", ClusterName:"c"}`.
  - **accept-knobs**: `random_sampling{value:50}`, `client_sampling{value:0}` (explicit 0), `overall_sampling{value:100}` ⇒ `RandomSampling:50, ClientSampling:0, OverallSampling:100` (explicit 0 PRESERVED, not coerced to 100).
  - **reject-non-otel-provider**: a `provider.typed_config` of a NON-OTel type (e.g. a Zipkin `ZipkinConfig` Any) ⇒ error naming the provider type (`reference_strict_reject_sibling_typeurl_gap` — an explicit per-provider reject).
  - **reject-empty-cluster**: OTel `grpc_service.envoy_grpc.cluster_name == ""` ⇒ error.
  - **reject-google_grpc**: `grpc_service.google_grpc{...}` ⇒ error naming `google_grpc`.
  - **reject-custom_tags**: a non-empty `custom_tags` ⇒ error.
  - **reject-verbose**: `verbose: true` ⇒ error. (`verbose:false`/absent ⇒ accept.)
  - **reject-max_path_tag_length**: a present `max_path_tag_length` ⇒ error.
  - **reject-spawn_upstream_span**: a PRESENT `spawn_upstream_span` (even `false`) ⇒ error (D-TRACE-SPAWN-UPSTREAM-SPAN).
  - **reject-http_service / reject-resource_detectors / reject-sampler**: each present OTel sub-field ⇒ its OWN error arm.
  - **nil-tracing**: `NewConfig(nil)` ⇒ `(nil, nil)` (no tracing configured — the byte-stable path).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestNewConfig -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `NewConfig(t *hcmtracing.HttpConnectionManager_Tracing) (*TracingConfig, error)` in `config.go`:
  - `if t == nil { return nil, nil }`.
  - reject `t.GetVerbose()`, a present `t.GetMaxPathTagLength()`, a non-empty `t.GetCustomTags()`, a present `t.GetSpawnUpstreamSpan()` — each its own arm with distinct `tracing:`-prefixed wording.
  - `p := t.GetProvider()`; require `p != nil`; unmarshal `p.GetTypedConfig()` — require the OTel `OpenTelemetryConfig` TypeURL (reject any other / nil with a per-type message).
  - reject the OTel `http_service`/`resource_detectors`/`sampler` (present); reject `google_grpc`; require non-empty `envoy_grpc.cluster_name`.
  - `pct := func(p *typev3.Percent, def float64) float64 { if p == nil { return def }; return p.GetValue() }` ⇒ map the three knobs (default 100).
  - return `&TracingConfig{...}`.

> The blank-import of `config/trace/v3` (so `provider.typed_config` resolves) is added in Task 6 (bootstrap) — but `NewConfig` here unmarshals via the typed `OpenTelemetryConfig{}` + `proto.Unmarshal(p.GetTypedConfig().GetValue(), &cfg)`, which works without the registry. The registry blank-import is for the protojson bootstrap round-trip.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestNewConfig -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/config.go internal/tracing/config_test.go
git commit -m "phase 46.1a Task 5: tracing.NewConfig — proto→TracingConfig + 8 STRICT-REJECT arms (non-OTel provider/google_grpc/empty-cluster/custom_tags/verbose/max_path_tag_length/spawn_upstream_span/http_service+resource_detectors+sampler) (ADR-0080)"
```

---

## Task 6: The decision engine `Decide` (sampling precedence + classification + nibble) [TDD, table-driven]

**Files:**
- Create: `internal/tracing/decision.go`
- Test: `internal/tracing/decision_test.go`
- Modify: `internal/bootstrap/bootstrap.go` (the `config/trace/v3` blank-import — ADR-0016)

The heart of the engine (§11 D-TRACE-SAMPLING precedence). `Decide` is pure (header + config + rng → Decision).

- [ ] **Step 1: Write the failing table tests** in `decision_test.go` (against a `fakeRand` forcing `Float64` + `Read`):
  - **fresh-random-sampled**: no `traceparent`, no `x-client-trace-id`, `RandomSampling:100`, fake `Float64`→0.0 ⇒ `Decision{Sample:true, Reason:Sampled, Class:RandomSampling, Continued:false, TraceID:<from rng>, SpanID:<from rng>, ParentSpanID:zero, RequestID:<nibble 9>}`.
  - **fresh-random-not-sampled**: `RandomSampling:0` (or fake `Float64`→0.99 at `RandomSampling:50`) ⇒ `Sample:false, Reason:NoTrace, Class:RandomSampling, RequestID:<nibble 4>` (still a fresh id + fresh trace/span ids — generated regardless of sampling).
  - **continued**: incoming `traceparent` `00-<fixed>-<fixedparent>-01` ⇒ `Sample:true, Reason:NoTrace (nibble 4), Class:NotTraceable, Continued:true, TraceID:<incoming>, ParentSpanID:<incoming parent>, SpanID:<fresh>`; AND `RandomSampling:0`+`OverallSampling:0` STILL `Sample:true` (continued bypasses local caps).
  - **continued-not-sampled**: incoming flags `00` ⇒ `Sample:false, Continued:true, Class:NotTraceable, nibble 4`.
  - **client-force**: no traceparent, `x-client-trace-id: abc`, `ClientSampling:100` ⇒ `Sample:true, Reason:Client (nibble b), Class:ClientEnabled`.
  - **client-force-suppressed**: `x-client-trace-id: abc`, `ClientSampling:0` ⇒ the force is SUPPRESSED and the request FALLS THROUGH to the `random_sampling` roll (§11: `client_sampling=0` drops `client_enabled` to 0). With `RandomSampling:100` ⇒ `Sample:true, Reason:Sampled (nibble 9), Class:RandomSampling`; with `RandomSampling:0` ⇒ `Sample:false, Class:RandomSampling`. (The Step-3 sketch's case-2 arm is authoritative for this path.) **RE-PROBE caveat:** §11 directly pinned only `client_enabled→0`; the fall-through-to-random is the read of the precedence, not a direct probe. If cheap, re-probe the reference (`client_sampling=0` + `x-client-trace-id` + `random_sampling=100` — does it still sample?) before pinning this test; else treat it as a working hypothesis and MATCH §11 if a probe contradicts.
  - **overall-cap**: no traceparent, `RandomSampling:100` but `OverallSampling:0` ⇒ `Sample:false` (overall caps the local sample); `x-client-trace-id`+`ClientSampling:100`+`OverallSampling:0` ⇒ `Sample:false` (overall suppresses client-force too — §11).
  - **preserve-existing-request-id**: an inbound `x-request-id: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` + a Sampled decision ⇒ `RequestID == "aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee"` (preserve+stamp, not regenerate).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestDecide -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `decision.go`:
```go
type SampleClass int
const ( ClientEnabled SampleClass = iota; HealthCheck; NotTraceable; RandomSampling; ServiceForced; NoClass )

type Decision struct {
    Sample       bool
    Reason       TraceReason   // NoTrace / Sampled / Client (the x-request-id nibble)
    Class        SampleClass   // the HCM-counter class (NoClass ⇒ increment none)
    Continued    bool
    TraceID      [16]byte
    SpanID       [8]byte       // fresh; the upstream traceparent span-id (+ the 46.1b span_id)
    ParentSpanID [8]byte       // incoming parent (continued) or zero
    TraceState   string        // pass-through
    RequestID    string        // the generated/stamped x-request-id
}

func Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision {
    var d Decision
    _, _ = rng.Read(d.SpanID[:]) // a fresh span-id always (upstream traceparent + 46.1b span)
    if ic, ok := ExtractTraceparent(h); ok {
        d.Continued = true; d.TraceID = ic.TraceID; d.ParentSpanID = ic.ParentID
        d.Sample = ic.Sampled; d.Reason = NoTrace; d.Class = NotTraceable; d.TraceState = ic.TraceState
    } else {
        _, _ = rng.Read(d.TraceID[:]) // fresh trace
        switch {
        case h.Get("X-Client-Trace-Id") != "" && rng.Float64()*100 < cfg.ClientSampling:
            d.Sample = true; d.Reason = Client; d.Class = ClientEnabled
        case h.Get("X-Client-Trace-Id") != "":
            // client_sampling suppressed the force ⇒ fall through to random (§11)
            d.Sample = rng.Float64()*100 < cfg.RandomSampling
            if d.Sample { d.Reason = Sampled }; d.Class = RandomSampling
        default:
            d.Sample = rng.Float64()*100 < cfg.RandomSampling
            if d.Sample { d.Reason = Sampled }; d.Class = RandomSampling
        }
        // overall_sampling caps the LOCALLY-decided sample (NOT a continued trace)
        if d.Sample && rng.Float64()*100 >= cfg.OverallSampling { d.Sample = false; d.Reason = NoTrace }
    }
    if existing := h.Get("X-Request-Id"); existing != "" {
        d.RequestID = StampRequestID(existing, d.Reason)
    } else {
        d.RequestID = GenerateRequestID(d.Reason, rng)
    }
    return d
}
```
> **The exact `client_sampling=0`-fallthrough + the `overall_sampling` second-roll semantics are the §11 reading — pin them with the table tests above; if a unit test contradicts the §11 probe notes, MATCH §11 (the empirical pin wins).** The `OverallSampling` cap as a SECOND `Float64` roll vs a deterministic gate: model it as the reference does — `overall_sampling` is itself a percentage cap (a second independent roll) per Envoy's `Sampler`; `overall=0` ⇒ `Float64*100 >= 0` always true ⇒ suppressed; `overall=100` ⇒ `>= 100` never ⇒ pass. ✓

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestDecide -count=1` ⇒ PASS.

- [ ] **Step 5: Add the bootstrap blank-import** (ADR-0016) in `internal/bootstrap/bootstrap.go` alongside the existing access-logger blank-imports:
```go
// Phase 46.1a registers the OpenTelemetry tracer config proto so protojson
// round-trips bootstraps carrying an HCM tracing.provider OpenTelemetryConfig
// typed_config (ADR-0260; lifts HttpConnectionManager.tracing from the ADR-0041
// silent-ignore set).
_ "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
```
Run: `go build ./... && go mod tidy -diff` ⇒ build OK, diff EMPTY (no new module — `config/trace/v3` is in the already-present go-control-plane module).

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ internal/bootstrap/ && golangci-lint run ./internal/tracing/... ./internal/bootstrap/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/decision.go internal/tracing/decision_test.go internal/bootstrap/bootstrap.go
git commit -m "phase 46.1a Task 6: tracing.Decide — full sampling precedence (continued bypasses caps / client-force / random / overall-cap) + classification + nibble + the config/trace/v3 blank-import (D-TRACE-SAMPLING; ADR-0016)"
```

---

## Task 7: The 5 HCM-scoped `tracing.*` counters (`tracing/stats.go` + `HCMCounters`) [TDD, +5 registration]

**Files:**
- Create: `internal/tracing/stats.go`, `internal/tracing/stats_test.go`

- [ ] **Step 1: Write the failing registration test** — `RegisterHCMCounters(reg, "ingress_http")` returns a non-nil `*HCMCounters` with 5 distinct non-nil counters; the registry gains EXACTLY 5 counters named `http.ingress_http.tracing.{client_enabled,health_check,not_traceable,random_sampling,service_forced}` (assert the count DELTA == 5, robust to the absolute). An INVALID `statPrefix` (e.g. `"bad name!"`) ⇒ `(nil, error)` (the IsValidName guard). `(*HCMCounters).Record(ClientEnabled)` increments `client_enabled`; `Record(RandomSampling)` increments `random_sampling`; `Record(NotTraceable)` increments `not_traceable`; `Record(HealthCheck)`/`Record(ServiceForced)` increment theirs; `Record(NoClass)` increments none.

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/tracing/ -run TestHCMCounters -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `stats.go`:
```go
type HCMCounters struct {
    clientEnabled, healthCheck, notTraceable, randomSampling, serviceForced *stats.Counter
}

// RegisterHCMCounters allocates the 5 HCM-scoped tracing decision counters under
// http.<statPrefix>.tracing.* (D-TRACE-STATS). The prefix is re-validated via
// IsValidName (defense-in-depth; the hcm config.go:235 guard already validates it
// for the shared http.<prefix>. namespace). health_check + service_forced register
// but stay 0 at 46.1a (no health-check detection / x-envoy-force-trace honoring).
func RegisterHCMCounters(reg *stats.Registry, statPrefix string) (*HCMCounters, error) {
    base := "http." + statPrefix + ".tracing."
    if !stats.IsValidName(base + "random_sampling") {
        return nil, fmt.Errorf("tracing: invalid stat_prefix %q", statPrefix)
    }
    return &HCMCounters{
        clientEnabled:  reg.NewCounter(base + "client_enabled"),
        healthCheck:    reg.NewCounter(base + "health_check"),
        notTraceable:   reg.NewCounter(base + "not_traceable"),
        randomSampling: reg.NewCounter(base + "random_sampling"),
        serviceForced:  reg.NewCounter(base + "service_forced"),
    }, nil
}

func (c *HCMCounters) Record(class SampleClass) {
    switch class {
    case ClientEnabled:  c.clientEnabled.Inc()
    case HealthCheck:    c.healthCheck.Inc()
    case NotTraceable:   c.notTraceable.Inc()
    case RandomSampling: c.randomSampling.Inc()
    case ServiceForced:  c.serviceForced.Inc()
    }
}
```

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/tracing/ -run TestHCMCounters -count=1` ⇒ PASS. Record the +5 surface delta (1191 → 1196) in PROGRESS-46.1a.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go build ./...
git add internal/tracing/stats.go internal/tracing/stats_test.go
git commit -m "phase 46.1a Task 7: RegisterHCMCounters — 5 HCM-scoped http.<prefix>.tracing.* counters (IsValidName-guarded) + Record dispatch (+5 → 1196; D-TRACE-STATS)"
```

---

## Task 8: HCM config-parse wiring — read `hcm.GetTracing()` in `parseFilterWithCtx` + the Filter fields + register the counters + the byte-stability guard [TDD]

**Files:**
- Modify: `internal/filter/hcm/config.go`
- Test: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing tests** in `config_test.go` (parse a full HCM `anypb.Any` via the existing HCM-filter parse test shape):
  - **accept**: an HCM with a `tracing` block (OTel provider, `random_sampling:100`, cluster_name `c`, service_name `svc`) ⇒ the parsed `Filter` has a non-nil `tracingConfig` (`RandomSampling==100`, `ClusterName=="c"`, `ServiceName=="svc"`), a non-nil `tracingCounters`, and the registry gained the 5 `http.<prefix>.tracing.*` counters.
  - **reject**: an HCM with a `tracing` block carrying `verbose:true` (or `spawn_upstream_span`) ⇒ `parseFilterWithCtx` returns an error (the `tracing.NewConfig` reject bubbled up as an `hcm:`-prefixed parse error).
  - **byte-stable (no tracing)**: an HCM with NO `tracing` block ⇒ `tracingConfig == nil`, `tracingCounters == nil`, and the registry has NO `tracing.*` counters (the +5 register ONLY under a configured provider — the byte-stable regression guard).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/filter/hcm/ -run TestParseFilter -count=1` (use the existing HCM parse test name) ⇒ FAIL.

- [ ] **Step 3: Implement** in `config.go`:
  - Add Filter fields: `tracingConfig *tracing.TracingConfig`, `tracingCounters *tracing.HCMCounters`, `rng tracing.RandSource`.
  - In `parseFilterWithCtx`, after the stat_prefix `IsValidName` guard (`:235`) and near the counter block (`:285`–`:295`):
    ```go
    tcfg, err := tracing.NewConfig(hcm.GetTracing())
    if err != nil {
        return nil, fmt.Errorf("hcm: tracing config: %w", err)
    }
    var tcounters *tracing.HCMCounters
    if tcfg != nil {
        tcounters, err = tracing.RegisterHCMCounters(registry, statPrefix)
        if err != nil {
            return nil, fmt.Errorf("hcm: tracing counters: %w", err)
        }
    }
    ```
  - Set `tracingConfig: tcfg, tracingCounters: tcounters, rng: tracing.CryptoRand{}` on the `Filter` literal. (Import `internal/tracing`.)

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/filter/hcm/ -run TestParseFilter -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go
git commit -m "phase 46.1a Task 8: HCM parse — lift hcm.GetTracing() from silent-ignore → tracing.NewConfig + RegisterHCMCounters in parseFilterWithCtx; no-tracing ⇒ no counters (byte-stable) (D-TRACE-CONFIG-HOME)"
```

---

## Task 9: HCM dispatch wiring — decision + `x-request-id` stamp + `traceparent` inject + counter at `connection.go` (H1+H2) [TDD]

**Files:**
- Modify: `internal/filter/hcm/connection.go`
- Modify: `internal/filter/http/router/router.go` (comment cross-reference only)
- Test: `internal/filter/hcm/connection_test.go` (or the existing HCM filter integration test)

- [ ] **Step 1: Write the failing tests** — drive a request through the HCM filter with a tracing config (a fake `RandSource` forcing a Sampled decision) and assert the headers the BACKEND/upstream sees:
  - **sampled-injects**: a request with no incoming trace headers ⇒ the upstream `req.Header` carries `X-Request-Id` (36-char, index-14 `'9'`) + `Traceparent` (`00-<32hex>-<16hex>-01`); the `http.<prefix>.tracing.random_sampling` counter == 1.
  - **continued**: a request with `Traceparent: 00-<fixed>-<fixedparent>-01` ⇒ upstream `Traceparent` trace-id == `<fixed>`, the `X-Request-Id` index-14 `'4'`; `tracing.not_traceable` == 1.
  - **preserve-request-id**: an inbound `X-Request-Id: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee` ⇒ upstream `X-Request-Id == aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee`.
  - **byte-stable (no tracing)**: the SAME request through a filter with `tracingConfig==nil` ⇒ NO `X-Request-Id`/`Traceparent` added (the headers pass through untouched — the regression guard); no counter movement.
  - Cover BOTH the H1 (`connection.go:448` path) and the H2 dispatch path.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/filter/hcm/ -run TestDispatchTracing -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `connection.go` at the dispatch seam (`:448` H1 + the H2 analogue), guarded by `if f.tracingConfig != nil`:
```go
if f.tracingConfig != nil {
    d := tracing.Decide(req.Header, f.tracingConfig, f.rng)
    req.Header.Set("X-Request-Id", d.RequestID)
    tracing.InjectTraceparent(req.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
    f.tracingCounters.Record(d.Class)
}
```
(For H2, apply the same to the `h2.H2Request` header set at the `h2dispatch.go` `WriteH2` seam (`startTime := time.Now()` ~`:282`) — confirm the H2 request-header mutation API + the inject site at IMPL; the decision call is identical. **CAUTION (PLAN-review note):** `WriteH2` takes `h2req h2.H2Request` BY VALUE; after mutating its headers you MUST re-`SetH2Request(h2req)` (or mutate the underlying header map in place) so the injected `traceparent`/`x-request-id` reach the upstream — mutating a discarded value-copy is a silent no-op. The `0086` differential exercises H1 only; H2 is unit-test-covered (Task 9 Step 1 covers both paths), so prove the H2 inject with the unit test.) Update `router.go:763`'s comment to note that `x-request-id`/`traceparent` ARE injected by the HCM tracing engine when an HCM `tracing` provider is configured (the router itself still injects nothing).

- [ ] **Step 4: Run to verify they pass + the full-package `-race`** — `go test ./internal/filter/hcm/ -run TestDispatchTracing -count=1` ⇒ PASS; then `go test ./internal/filter/hcm/ ./internal/tracing/ -race -count=1` ⇒ PASS (no race — the engine is synchronous, but the Filter is shared across connections; confirm the `rng`/counters are concurrency-safe — `stats.Counter` is atomic, `CryptoRand` is stateless, `Decide` allocates a fresh `Decision`).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ internal/filter/http/router/ && golangci-lint run ./internal/filter/hcm/... ./internal/filter/http/router/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/connection.go internal/filter/http/router/router.go internal/filter/hcm/connection_test.go
git commit -m "phase 46.1a Task 9: HCM dispatch wiring — Decide + x-request-id stamp + traceparent inject + counter at connection.go (H1+H2); no-tracing ⇒ byte-stable (revises router.go:763 invariant under a provider)"
```

---

## Task 10: The `0086-tracing-request-id` differential fixture (header-echo cross-side) + the sampling-decision subject unit coverage

**Files:**
- Create: `test/fixtures/0086-tracing-request-id/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0086` driver)

Model the directory on `test/fixtures/0084-otlp-access-log/` (the driver-owned-assertion shape) but use the existing `HTTPHeaderMutation` backend (Kind 9) — NO receiver. The continuation prong + the sampling-class assertion ride the same dir (one cross-side assertion branch — `reference_differential_fixture_dispatch_constraint`; the boot-reject prong is a 46.1b `log.Fatalf` concern).

- [ ] **Step 1: Author the bootstraps.** Both `envoy.yaml` (reference) + `envoy-go.yaml` (subject): an **H1** downstream listener → a route to the `HTTPHeaderMutation` backend; the HCM carries a `tracing` block: `provider: { name: envoy.tracers.opentelemetry, typed_config: OpenTelemetryConfig { grpc_service: { envoy_grpc: { cluster_name: "otlp_collector" } }, service_name: "0086" } }` + `random_sampling: { value: 100 }` (deterministic sampling — the differential never exercises partial sampling). Define a DUMMY `otlp_collector` cluster (STATIC, h2c, an unused endpoint — the reference boots permissively + silently fails export; envoy-go at 46.1a parses but never dials it; the cluster keeps the config 46.1b-ready). Use a FIXED `Host` + a query-less path. (Copy the listener/admin/backend templating from `0084`; SWAP the access-logger block for the `tracing` block + the dummy cluster.)

- [ ] **Step 2: Author `driver/driver.go`.** Copy `0084/driver/driver.go`, adapt: `package driver`, `fixtureName = "0086-tracing-request-id"`, `func init() { fixture.RegisterFixture(fixtureName, &tracingDriver{}) }`; `BackendKind() == fixture.HTTPHeaderMutation`; NO OTLP port/server. Drive: fire **N** (e.g. 8) plain requests (fixed `Host`/`User-Agent`, query-less path) + **M** (e.g. 4) requests carrying `Traceparent: 00-<FIXED 32hex>-<FIXED 16hex>-01`, against each side. The `HTTPHeaderMutation` backend echoes the forwarded headers into the response body; the driver parses each response body (the `"name: value\n"` lines) to extract the upstream `x-request-id` + `traceparent`. Snapshot per side.

- [ ] **Step 3: Author the assertions (`AssertStats`).** Cross-side EXACT on the STABLE structure (the VALUES vary — `reference_wire_format_both_sides_see_same_bytes` for the framing, structural assert for the random ids):
  - Both sides, the N plain requests: each echoed `x-request-id` is 36-char UUID-shaped with index-14 == `'9'` (Sampled); each echoed `traceparent` is `00-<32hex>-<16hex>-01` (sampled flag). (A zero-header "pass" is vacuous — assert the headers are PRESENT on both sides, proving injection ran.)
  - Both sides, the M continuation requests: each echoed `traceparent` trace-id == the FIXED incoming trace-id (continued); each echoed `x-request-id` index-14 == `'4'` (NoTrace — continued keeps the local-reason nibble).
  - Subject `/stats` (scrape via the `0084` helper): `http.<prefix>.tracing.random_sampling == N`, `http.<prefix>.tracing.not_traceable == M` (the continuation prong), `tracing.client_enabled == 0`, `tracing.health_check == 0`, `tracing.service_forced == 0`.
  - **UNasserted:** the `x-request-id`/`traceparent` VALUES (except the continuation trace-id) — random per request; the span-id; the reference's default-injected extras (`x-envoy-*`/`x-forwarded-*` — excluded, the `http_diff.go` allowlist precedent).
  - Author `expectations.yaml` minimal (this is a driver-`AssertStats` fixture, the `0084` shape).

- [ ] **Step 4: Author `README.md`** — the fixture's purpose (the header-level tracing engine; NO span/export at 46.1a), the `HTTPHeaderMutation`-echo capture mechanism, the random_sampling=100% determinism note, the continuation prong, the framing-not-asserted note, the dummy-collector-cluster (46.1b-ready) note, and the one-dir-one-branch note (`reference_differential_fixture_dispatch_constraint`).

- [ ] **Step 5: Register + run the fixture isolated.** Add `_ "github.com/esalaine/envoy-go/test/fixtures/0086-tracing-request-id/driver"` to `runner_test.go`'s blank-imports (alongside `:111`–`:112`).
Run (`reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0086' -count=1` ⇒ PASS (both sides inject the nibble-9 `x-request-id` + the `01` traceparent on the N plain requests + continue the fixed trace-id on the M continuation requests; subject counters match).
Confirm: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **88**.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0086-tracing-request-id/ test/differential/runner_test.go
git commit -m "phase 46.1a Task 10: 0086-tracing-request-id differential — cross-side x-request-id nibble + traceparent continuation via HTTPHeaderMutation echo + subject tracing.* counters, poll-free fixed traffic (fixtures 87 → 88)"
```

---

## Task 11: `0086` deliberate-break proofs + flake gate + the full-package `-race`

**Files:** (no production change — verification only; revert every break)

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0086` FAILS (the assertion is live), then `git restore` it:
  - (a) Break `GenerateRequestID`/`StampRequestID` to NOT stamp index 14 (leave the version `'4'`) ⇒ the nibble-`'9'` assertion must FAIL on the plain-request prong.
  - (b) Break `Decide` to ignore the incoming `traceparent` (always fresh trace) ⇒ the continuation trace-id assertion must FAIL on the M-prong.
  - (c) Break the `connection.go` inject site (skip `InjectTraceparent`) ⇒ the `traceparent`-present assertion must FAIL.
  - (d) Break `HCMCounters.Record`/the counter site (skip it) ⇒ the subject `tracing.random_sampling == N` assertion must FAIL.
  - (e) Break the byte-stability guard (run the decision even when `tracingConfig==nil`) — verify an UNRELATED non-tracing fixture is unaffected (it has no tracing config ⇒ no path change); confirm the guard via the Task-9 unit test instead (a differential can't easily prove a negative-injection). Note this in PROGRESS.
  Run each: `go test ./test/differential/ -run 'TestDifferential/0086' -count=1` ⇒ FAIL, then restore ⇒ PASS. Record each in PROGRESS-46.1a.md.

- [ ] **Step 2: Flake gate** — 20 consecutive green runs:
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0086' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. (A transient `subject ready: EOF` is the startup-race flake — `reference_differential_fullsuite_startup_flake` — isolate-re-run; NOT a 0086 regression.)

- [ ] **Step 3: Full `internal/tracing` + `internal/filter/hcm` package `-race`**:
```bash
go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1
```
Expected: PASS, no race (the Filter is shared across connections; the engine is stateless/atomic).

- [ ] **Step 4: Commit the PROGRESS update**
```bash
git add docs/envoy-go/phases/46-tracing/PROGRESS-46.1a.md
git commit -m "phase 46.1a Task 11: 0086 deliberate-break proofs (4 live differential assertions + the unit byte-stability guard) + 20/20 flake + full-package -race"
```

---

## Task 12: Full 88-dir differential + six-gate + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile (row 46 STAYS in-progress; ADR-0260 deferred to 46.1b)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/46-tracing/PROGRESS-46.1a.md`

- [ ] **Step 1: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                  # clean
go build ./...                                # ok
go test ./... -count=1                        # full unit + the 88-dir differential
go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1
```
Expected: all green. (The full differential is the byte-stability regression anchor — NO non-tracing fixture should move; the tracing engine is dormant unless a `tracing` block is configured.) Confirm `go mod tidy -diff` is EMPTY (ZERO new modules).

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — add a `### Request tracing — sampling/request-id engine (header propagation)` subsection: an HCM `tracing` block with an OTel provider runs the full-fidelity sampling/request-id decision (the three Percent knobs + `x-client-trace-id` force gated by `client_sampling` + the incoming `traceparent` sampled-bit, the §11 precedence: continued bypasses local caps; `overall_sampling` caps the local sample), generates/preserves `x-request-id` with the trace-reason in its byte-14 nibble (NoTrace=`4`/Sampled=`9`/Client=`b`), continues an incoming `traceparent` trace-id (or starts fresh), and injects the new trace context (`traceparent` + `tracestate` pass-through) upstream. STRICT-REJECT the non-OTel provider / `custom_tags` / `verbose` / `max_path_tag_length` / `spawn_upstream_span` (present) / OTel `http_service`/`resource_detectors`/`sampler` / `google_grpc` / empty cluster_name. 5 HCM-scoped `http.<stat_prefix>.tracing.*` counters (`client_enabled`/`not_traceable`/`random_sampling` live; `health_check`/`service_forced` register-but-0). **The per-request SERVER span + the OTLP export are 46.1b** (note the forward reference). Advance the stat-surface block 1191 → 1196.

- [ ] **Step 3: STATE.md + ROADMAP.md** — STATE active-phase → `phase 46.1a IMPL done`; counts → stat **1196** / fixtures **88** / fuzzers **48** / BackendKind **38** / DECISIONS **ADR-0259** (ADR-0260 body deferred to the 46.1b IMPL). ROADMAP row 46 STAYS **`in-progress`** (per-leg ADR-0106 + `reference_roadmap_split_phase_row_done`; row 46 flips `done` at the 46.2 IMPL; the Observability family STAYS OPEN); stage-note "46.1a (header engine) landed; 46.1b (span+OTLP export) next." Set the next action → the **46.1b PLAN**.

- [ ] **Step 4: Fuzzer-count reconcile** (`reference_fuzzer_count_docs_drift`) — verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == **48** and advance the documented running total 46 → 48 across STATE.md / BEHAVIOR_CONTRACT.md / ROADMAP.md / PROGRESS-46.1a.md consistently.

- [ ] **Step 5: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 46.1a (header tracing engine) IMPL: BEHAVIOR_CONTRACT + STATE/ROADMAP (row 46 STAYS in-progress; ADR-0260 body deferred to 46.1b; Observability family STAYS OPEN); stat 1196 / fixtures 88 / fuzzers 48 / BackendKind 38 / 0 new go.mod modules"
```

---

## Final review + handoff

- [ ] **Controller squashes the worktree branch** into ONE atomic commit (the house stage-close shape) with a subject `phase 46.1a (tracing header engine) IMPL: the sampling/request-id decision engine + traceparent propagation + x-request-id generation + the 5 HCM-scoped tracing.* counters — …`, verifies `git -C <main-checkout> status` is clean, then **pushes to origin** (`feedback_push_to_origin`) and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 46.1a IMPL squash and route the next session to the **46.1b PLAN** (the span model + `OTLPTracesClient` + `OTLPExporter` + `test/helpers/otlptrace` + `0087-tracing-otlp` + the ADR-0260 §Decision/§Consequences body that CLOSES the leg; row 46 STAYS in-progress until 46.2).
- [ ] **Counts at IMPL-done (the exit invariant):** stat surface **1196** (H2 cluster; non-H2 **1192** — the +5 tracing counters register on any tracing-HCM, cluster-independent) / fixtures **88** (tail `0086-tracing-request-id`) / fuzzers **48** (`FuzzStampRequestID` + `FuzzExtractTraceparent`) / BackendKind **38** (UNCHANGED — header-echo reuse) / DECISIONS **ADR-0259** (ADR-0260 body lands at 46.1b). ONE new Go package (`internal/tracing`); ZERO new go.mod modules (`go mod tidy -diff` EMPTY).

> **NOTE on the surface figure:** the +5 tracing counters are DYNAMIC (register only when an HCM `tracing` block is configured). Assert the +5 DELTA via the Task-7 registration test + the Task-12 six-gate, NOT a brittle absolute. The tracer-scoped `tracing.opentelemetry.{spans_sent,spans_dropped}` (+2 → 1198) land at 46.1b.
