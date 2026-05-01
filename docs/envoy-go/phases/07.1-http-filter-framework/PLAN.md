# Phase 07.1 — HTTP filter framework (`internal/filter/http/` package, iteration protocol, registry, per-route config, `cors` + `envoy_go_test` filters, fixtures `0007a` + `0007b`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit and at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6.1 (split gate — ~1500 LoC AND <25 tasks), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 07.1); `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; ~965 lines, 16 sections, read in full); `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` (parent master SPEC — cross-cutting context for the 07.1 + 07.2 split); `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` (the brainstorm-close artefact at master `9c3752b` that the 07.1 SPEC distils from); `docs/envoy-go/phases/06.1-stats-prometheus/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` and `docs/envoy-go/phases/06.2-access-log/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 06.2 PLAN is the structural precedent — §-numbering, heredoc-style task headers, ADR-with-first-use-commit discipline, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections, TDD-step granularity); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0069 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-numbering rule, **ADR-0005** autonomous plan-review adaptation + subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0014** `Server: envoy` mirror, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0040** router-as-direct-call (totally superseded by ADR-0071), **ADR-0041** silent-ignore set + amendment policy (amended by ADR-0073 + ADR-0076), **ADR-0042** `[router]`-only chain shape (partially superseded by ADR-0071), **ADR-0044** HCM-network-filter introduction, **ADR-0045** planner-time-split discipline, **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0059** internal stats Registry architecture (the architectural-shape sibling 07.1's ADR-0072 mirrors), **ADR-0066** access-log architecture (the no-third-party-library sibling), **ADR-0068** three-tier equivalence matrix (the SPEC §1 #9 pattern this PLAN's fixture 0007a inherits at single-tier), **ADR-0069** is the verified DECISIONS.md tail; phase 07.1's seven anticipated ADRs land at ADR-0070..ADR-0076); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — adds a NEW `## HTTP filter chain` top-level section + amends `## HTTP/1.1` and `## HTTP/2` for the ADR-0040 / ADR-0042 supersession; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 07.1 — D-3.7 reserves pin bumps for dedicated phases).

**Goal:** Land envoy-go's HTTP filter chain framework — a real iteration protocol with async-resume, an extension registry, a 3-tier per-route config model, the first real Envoy filter (`cors`), and a test-only probe filter (`envoy_go_test`) that covers every iteration-protocol state branch — and migrate the existing router-as-direct-call dispatch into a router-as-terminal-filter shape behind that protocol. Two new fixtures land: `test/differential/0007a-cors/` is differentially green (gate (a) non-vacuous; per-request status + response header set + body byte-equal across envoy-go and reference Envoy v1.37.2, modulo the existing differential ignore-list in `BEHAVIOR_CONTRACT.md ## Header allow-list`) on a 4-request workload exercising preflight (allowed/disallowed origins) + actual-request (allowed/no-origin) per the §11.2 empirical pin; `test/differential/0007b-iteration-probe/` is structurally green (`RequiresReference: false`) on an 8-request workload covering every iteration-protocol state branch (Continue / StopIteration / StopIterationAndBuffer / async-resume / SendLocalReply on decode-headers / SendLocalReply on decode-data / encode-side modify / decode-trailers stop). Concretely: a NEW `internal/filter/http/` package (`StreamDecoderFilter` + `StreamEncoderFilter` interfaces in `types.go`; status enums `FilterHeadersStatus` / `FilterDataStatus` / `FilterTrailersStatus`; `DecoderFilterCallbacks` + `EncoderFilterCallbacks` in `callbacks.go`; `HTTPRegistry` Register / Lookup / Freeze in `registry.go`; `FilterChain` per-stream state machine with decode/encode iteration + buffer management + async-resume signaling in `chain.go`; `typed_per_filter_config` 3-tier merge in `perroute.go`; per-package `doc.go`; ~800-1200 LoC of new machinery per BRAINSTORM §3 + §4); two new HTTP filters under `internal/filter/http/` (`cors/` real Envoy filter ~150 LoC + ~200 LoC tests; `envoygotest/` test-only probe ~250 LoC + ~400 LoC tests with hand-rolled minimal proto); router extracted as a real terminal filter (the largest refactor — `routerAction` + `routerActionH2` move from `internal/filter/hcm/actions.go` to `internal/filter/http/router/`; tests byte-preserved per BRAINSTORM §6.8; `directResponseAction` STAYS in `hcm/actions.go` as a route-action shape decided at route-match time); per-route config model (`typed_per_filter_config` honored on Route / VirtualHost / RouteConfiguration scopes; build-time validation; lookup via `DecoderFilterCallbacks.RequestRouteConfig() proto.Message`; lazy cache on first lookup per request); extension registry (`*filter.HTTPRegistry` constructed once at boot in `cmd/envoy-go/main.go`, threaded explicitly into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`; freeze-after-boot invariant mirrors `*stats.Registry` LBP-1 from 06.1; three filters registered at boot: `router.New`, `cors.New`, `envoygotest.New`); HCM-side validation tightening (phase-04 ADR-0042's "exactly `[router]`" rule becomes "non-empty; last entry must be router" — partial supersession; empty `http_filters[]` errors at parse; last entry not router errors; duplicate filter names error; unknown `typed_config.type_url` errors); body buffer cap (`filterBufferLimitBytes = 1 << 20` = 1 MiB hardcoded matching Envoy default; decode-side overflow synthesizes `413 Payload Too Large` per §11 #3 empirical pin verbatim; encode-side overflow resets the connection — H1 close, H2 RST_STREAM); `sendLocalReply` flow (synthesized response enters the encode chain at `filter[len-1]` per §11 #4 empirical pin; first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log; full encode chain runs in reverse order); seven new ADRs ADR-0070..ADR-0076 (re-verified at Task 1 step 1 against `DECISIONS.md` tail being ADR-0069); a NEW `internal/filter/http.FuzzFilterChainParse` fuzzer at the 30s ADR-0018 budget — fuzzes adversarial `http_filters[]` slices into `parseFilterWithCtx` (varied type_urls, malformed typed_configs, oversized counts); ninth fuzzer overall; a `BEHAVIOR_CONTRACT.md ## HTTP filter chain` in-place-edit population (NEW top-level section between `## HTTP/2` and `## TCP proxy` carrying the four §11 empirical-pin blocks verbatim per ADR-0052 + §13.1) + amendments to `## HTTP/1.1` and `## HTTP/2` for the ADR-0040 / ADR-0042 supersession; STATE.md / ROADMAP.md / PROGRESS.md updates with row 07.1 → `done` at the phase-done commit (parent row 07 stays `in-progress` — the parent only flips to `done` at 07.2's phase-done, mirroring the 05/05.1/05.2 + 06/06.1/06.2 closure pattern). After phase 07.1, the project has proven its eighth central engineering claim (HTTP-filter-framework half): *envoy-go runs a real multi-filter HTTP iteration chain — filters can stop, buffer, resume async, modify decode/encode headers and bodies, synthesize local replies that re-enter the encode chain — and produces behaviorally-equivalent CORS responses to upstream Envoy, while a test-only probe filter exercises every iteration-protocol state to structural correctness.* The listener-chain-completion half (07.2) is delivered later; the parent ROADMAP row `07` flips to `done` at 07.2's phase-done.

**Architecture:** The 07.1 surface is the additive introduction of one new package tree (`internal/filter/http/` with five sub-packages: root + `cors/` + `envoygotest/` + `envoygotest/proto/` + `router/`) plus substantial refactor of `internal/filter/hcm/` (constructor signatures widen + dispatch path gains a chain runner + `actions.go`'s router actions move out) plus the threading of a `*filter.HTTPRegistry` parameter through one constructor chain (`hcm.NewFilter*`) plus the listener-manager's HCM-construction path plus boot-wiring in `cmd/envoy-go/main.go` plus a single blank-import addition in `internal/bootstrap/bootstrap.go` for the cors v3 proto. The threading mirrors 06.1's `*stats.Registry` parameter-threading discipline (codified in 06.1 ADR-0059); SPEC §4.2's file inventory enumerates each constructor change explicitly. Concretely: `internal/filter/http/types.go` (NEW; ~80 LoC) defines `StreamDecoderFilter` + `StreamEncoderFilter` interfaces, status enums (`FilterHeadersStatus`: `Continue`, `StopIteration`; `FilterDataStatus`: `Continue`, `StopIterationAndBuffer`, `StopIterationNoBuffer`; `FilterTrailersStatus`: `Continue`, `StopIteration`), `HTTPFilter` tagged-union over decoder-only / encoder-only / both, `HTTPFilterFactory` + `FilterInstanceFactory` two-step factory pattern, `FactoryCtx` carrying registry pointer + parsed proto-helpers; `internal/filter/http/callbacks.go` (NEW; ~60 LoC) defines `DecoderFilterCallbacks` (`ContinueDecoding`, `SendLocalReply`, `RequestRouteConfig`, plus encoder-style injection methods for filters that synthesize responses) + `EncoderFilterCallbacks` (`ContinueEncoding`, plus encoder-side injection methods); the callback structs are concretes implemented by the `chain` package; the interface is what filters depend on; `internal/filter/http/registry.go` (NEW; ~80 LoC) defines `HTTPRegistry struct { mu sync.RWMutex; byTypeURL map[string]HTTPFilterFactory; frozen atomic.Bool }`, `NewHTTPRegistry()`, `Register(typeURL string, f HTTPFilterFactory)` (panics if frozen, panics on duplicate type_url), `Lookup(typeURL string) (HTTPFilterFactory, bool)`, `Freeze()` (idempotent); mirrors `*stats.Registry` LBP-1 from 06.1; `internal/filter/http/chain.go` (NEW; ~500 LoC) defines `FilterChain` per-stream state machine, allocated by HCM dispatch (connection.go for H1, h2dispatch.go for H2) at the start of each request; owns filter instances (allocated via per-request factories from the chainConfig), per-filter callbacks, decode buffer (capacity `filterBufferLimitBytes = 1 << 20`), encode buffer (same cap), merged per-route config map, decode iteration index, encode iteration index, async-resume signal channels (`decodeResumeCh`, `encodeResumeCh`; both `chan struct{}` capacity 1 — non-blocking sends, idempotent coalesce); methods `runDecodeHeaders`, `runDecodeData`, `runDecodeTrailers`, `runEncodeHeaders`, `runEncodeData`, `runEncodeTrailers`, `beginLocalReply(status, headers, body)`; single-goroutine-per-request iteration invariant (HCM dispatch goroutine is the only goroutine that drives chain iteration; filter callbacks called from filter-spawned goroutines are signal-only via channel send); `internal/filter/http/perroute.go` (NEW; ~120 LoC) defines `typed_per_filter_config` parser + 3-tier merge — `BuildPerRouteConfig(rc *route.RouteConfiguration, filterNames []string) (PerRouteConfig, error)` parses `typed_per_filter_config` on each scope and validates keys ⊆ chain filter names (errors on unknown), `(*PerRouteConfig) Resolve(filterName string, route, vhost, rc Scope) proto.Message` performs Route > VirtualHost > RouteConfiguration most-specific override (no field-merge), lazy cache `map[filterName]proto.Message` on first lookup per request lives on `FilterChain`; `internal/filter/http/doc.go` (NEW; ~50 LoC) — package overview; `internal/filter/http/fuzz_test.go` (NEW; ~80 LoC) — `FuzzFilterChainParse` fuzzes adversarial `http_filters[]` slices through `parseFilterWithCtx` (varied type_urls, malformed typed_configs, oversized counts); `internal/filter/http/cors/cors.go` (NEW; ~150 LoC) — real Envoy filter `envoy.filters.http.cors`; decode-side detects preflight (`OPTIONS` + `Origin` + `Access-Control-Request-Method`); allowed origin → `SendLocalReply(200, "", corsHeaders)` synthesizing the preflight response with the verbatim header set pinned in §11.2 (six headers in fixed order: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-allow-methods`, `access-control-allow-headers`, `access-control-max-age`, `access-control-expose-headers`); disallowed-origin preflight → cors filter does NOT inject a 4xx (per §11.2 empirical pin: passes through to router which 405s); encode-side appends three CORS response headers (`access-control-allow-origin`, `access-control-allow-credentials`, `access-control-expose-headers`) on the upstream response when the request had an `Origin` matching the allow-list; `internal/filter/http/envoygotest/filter.go` (NEW; ~250 LoC) — test-only probe filter `envoy.filters.http.envoy_go_test`; per-request mode dispatch on `x-envoy-go-test-mode` header covering 8 iteration-state modes (`continue`, `stop-and-resume-headers`, `stop-and-buffer-data`, `local-reply-decode`, `local-reply-decode-data`, `modify-encode-headers`, `modify-encode-data`, `stop-trailers`); per-route config `count` (int32) echoed into a response header (`x-envoy-go-test-route-count: N`) on encodeHeaders; explicit-switch dispatch (per Decision §3.7 — debuggable, discoverable, matches the `directResponseAction` / `routerAction` precedent); `internal/filter/http/envoygotest/proto/envoygotest.pb.go` (NEW; ~30 LoC) — hand-rolled minimal proto with two fields (`mode_default` string + `count` int32); does NOT extend the upstream go-control-plane registry; `internal/filter/http/router/router.go` (NEW; ~250 LoC; migrated from `internal/filter/hcm/actions.go routerAction`) + `router_h2.go` (NEW; ~250 LoC; migrated from `routerActionH2`) — terminal filter `envoy.filters.http.router`; decodeHeaders dispatches the resolved route action (cluster dial OR direct_response synthesize); tests byte-preserved per BRAINSTORM §6.8 (imports update; package names update; test bodies byte-preserved); `internal/filter/hcm/config.go` (MODIFIED) — `parseFilterWithCtx(...)` signature gains a `*filter.HTTPRegistry` parameter, walks `http_filters[]` in declaration order, validates (empty / non-router-terminal / duplicate / unknown type_url), parses `typed_per_filter_config` via `perroute.Build` and validates keys ⊆ chain filter names; `internal/filter/hcm/filter.go` (MODIFIED) — `Filter` struct gains `chainConfig []chainEntry` (filter name + per-instance factory) and `perRouteConfig PerRouteConfig`; constructor signature widens; new `NewFilterWithCtxAndSinksAndRegistry(...)` extends the constructor chain with one `*filter.HTTPRegistry` parameter; pre-existing `NewFilter` / `NewFilterWithCtx` / `NewFilterWithCtxAndSinks` either remain (forwarding to the new constructor with a default-empty registry) or are deleted with all call sites updated (Decision §3.4 settles this); `internal/filter/hcm/actions.go` (MODIFIED) — `routerAction` + `routerActionH2` are DELETED (moved to `internal/filter/http/router/`); `directResponseAction` STAYS as a route-action shape decided at route-match time, synthesized by the router filter when its terminal step runs; `internal/filter/hcm/connection.go` (MODIFIED, H1 dispatch) — `dispatchRequest(ctx, req, w)` allocates a per-request `*FilterChain` from `f.chainConfig`, runs the filter chain (`chain.runDecodeHeaders` → ... → terminal router filter → ... → `chain.runEncodeHeaders` → ... → wire write); access-log emit-deferral hook fires from the chain's terminal-completion path (end of `chain.runEncodeData(endStream=true)` / `chain.runEncodeTrailers`) per Decision §3.1 + SPEC §12 #1; `internal/filter/hcm/h2dispatch.go` (MODIFIED, H2 dispatch) — same shape change for the H2 codec path; `internal/filter/hcm/accesslog_emit.go` (UNCHANGED in code; trigger-point semantics shift); `internal/filter/doc.go` (REWRITE) — replaces the phase-00 placeholder ("real implementation lands in phase 07") with the actual architectural overview pointing to `internal/filter/http/` for the HTTP-side framework and `internal/filter/hcm/` for the HCM-internal dispatch wiring; `cmd/envoy-go/main.go` (MODIFIED) — at boot: `reg := filter.NewHTTPRegistry(); reg.Register(router.TypeURL, router.New); reg.Register(cors.TypeURL, cors.New); reg.Register(envoygotest.TypeURL, envoygotest.New); reg.Freeze()`; threads `reg` into the HCM constructor chain via `listenerManager.New(...)` (which threads it into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`); `internal/listener/manager.go` (MODIFIED) — listener-manager's HCM-construction path threads the `*filter.HTTPRegistry` parameter through to `hcm.NewFilter*`; `internal/bootstrap/bootstrap.go` (MODIFIED) — adds blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"` (carries the `Cors` + `CorsPolicy` proto messages) so `protojson` can round-trip 07.1 fixture bootstraps; per ADR-0016's amendment policy this addition is documented in PROGRESS, not a new ADR; `test/differential/0007a-cors/` (NEW directory) carries `envoy-go.yaml` + `envoy.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go` (small Go HTTP/1.1 server returning 200 OK with body `"hello\n"`); 4-request workload + per-request equivalence shape; runner registers as `RequiresReference: true`; `test/differential/0007b-iteration-probe/` (NEW directory) carries `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go` (H1 echo backend); 8-request per-mode workload + per-mode embedded-expectation-table assertion; runner registers as `RequiresReference: false`; `test/differential/runner.go` (MODIFIED) blank-imports the new fixture-0007a + 0007b driver packages; `BEHAVIOR_CONTRACT.md` is edited in place at the closing-sweep commit per ADR-0052 — adds NEW `## HTTP filter chain` top-level section between `## HTTP/2` and `## TCP proxy` populated with the four §11 empirical-pin blocks verbatim + iteration-protocol shape rules + buffer-overflow rules + async-resume mechanics + filter-ordering rules + `## Equivalence Matrix` row addition; amends the existing `## HTTP/1.1` and `## HTTP/2` subsections to update the "exactly `[router]`" rule references to "non-empty; last entry must be router" (with forward-pointer to `## HTTP filter chain`); the seven ADRs ADR-0070..ADR-0076 land at first-use-task ordering per the phase-04/05.1/05.2/06.1/06.2 precedent.

**Tech Stack:**
- Go 1.23 (unchanged from 06.2; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `sync`, `sync/atomic`, `io`, `net/http`, `time`, `fmt`, `errors`, `bufio`, `bytes`, `strings`, `context` — the exhaustive set the `internal/filter/http/` package and the modified `internal/filter/hcm/` files consume.
- `google.golang.org/protobuf` (proto runtime) — the `proto.Message` return type from `RequestRouteConfig` + the `anypb.Any` type the two-step factory parses; transitive from existing imports.
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3` — proto types only (`Cors` + `CorsPolicy`); blank-imported in `internal/bootstrap/bootstrap.go` so `protojson` can round-trip 07.1 fixture bootstraps containing `typed_per_filter_config[envoy.filters.http.cors]` entries. **No new go-control-plane runtime helpers** (D-3.2 forbids).
- **NEW: no third-party filter-chain-engine / filter-iteration library.** `go.mod` MUST NOT contain `github.com/justinas/alice`, `github.com/urfave/negroni`, `github.com/go-chi/chi/middleware`, or any other middleware/filter-chain library import. The acceptance check at Task 23 step 4 grep-verifies the absence (per ADR-0071 + SPEC §15 acceptance bullet "No third-party filter-chain-engine / filter-iteration library is imported").
- `internal/cluster` (existing) — consumed by `internal/filter/http/router/` (the migrated router action calls `Cluster.Dial` / `DialH2` for upstream dispatch).
- `internal/stats` (06.1's deliverable) — UNCHANGED in 07.1; framework allocates no new stats counters.
- `internal/accesslog` (06.2's deliverable) — UNCHANGED in package shape; the HCM emit-trigger point shifts from `routerAction.do` / `directResponseAction.do` / `routerActionH2.doH2` / `h2DirectResponseAdapter.WriteH2` (the four 06.2 sites) to a single chain-completion hook at the end of `chain.runEncodeData(endStream=true)` / `chain.runEncodeTrailers` per Decision §3.1 + SPEC §12 #1. `accesslog_emit.go` body is preserved; only the call sites move (from `actions.go` + `h2dispatch.go` to `chain.go`'s terminal-completion path).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged). Phase 07.1 reads HCM `http_filters[]` (proto type `envoy.config.filter.network.http_connection_manager.v3.HttpFilter`) + `typed_per_filter_config` (proto type `map<string, google.protobuf.Any>`) + `envoy.extensions.filters.http.cors.v3.{Cors, CorsPolicy}` typed-config; no proto version bump.
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0007a's reference (Envoy in a Docker container) — same harness as 06.2's fixture 0006 consumes; phase 07.1 does not modify `test/differential/harness.go`.
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0007a's reference image AND the source of the four §11 empirical pins (already executed at SPEC time and pinned verbatim in SPEC §11; not re-scraped at PLAN/impl time).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 07.1 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS; phase 07.1 touches HCM dispatch wiring (H1 and H2) but the dispatch-path change is between codec-decoded request and the route action, which is below the h2spec conformance surface.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- **Forbidden runtime imports (D-3.2 + ADR-0071):** `github.com/justinas/alice`, `github.com/urfave/negroni`, `github.com/go-chi/chi/middleware`, `github.com/gorilla/handlers`, ANY HTTP-middleware / filter-chain library. The boundary grep at Task 23 step 4 enforces. Test-side use is also forbidden.
- `internal/filter/http/` is a NEW package tree introduced in 07.1; no pre-existing imports of it exist.
- `internal/filter/hcm/` extends in place; no new imports outside the standard library + `github.com/esalaine/envoy-go/internal/filter/http` (the existing 06.1 `internal/stats` and 06.2 `internal/accesslog` imports are unchanged).
- `internal/listener/`, `cmd/envoy-go/`, `internal/bootstrap/` extensions add a single import path each: `github.com/esalaine/envoy-go/internal/filter/http` (and its sub-packages `cors`, `envoygotest`, `router` for the `cmd/envoy-go/main.go` registry-population). The package-import-graph stays acyclic (the boundary check is grep-verifiable: no `internal/filter/http` file imports `internal/filter/hcm`; the http package is a near-leaf — it imports stdlib + protobuf runtime + cors-v3-proto + `internal/cluster` (router sub-package only)).

---

## Scope check — why phase 07.1 ships as one sub-phase

Net change estimate (mirroring the 06.2 PLAN's component-table convention):

- `internal/filter/http/types.go` ~80 + `types_test.go` ~120 = ~200
- `internal/filter/http/callbacks.go` ~60 + `callbacks_test.go` ~150 = ~210
- `internal/filter/http/registry.go` ~80 + `registry_test.go` ~150 = ~230
- `internal/filter/http/chain.go` ~500 + `chain_test.go` ~600 = ~1100
- `internal/filter/http/perroute.go` ~120 + `perroute_test.go` ~200 = ~320
- `internal/filter/http/doc.go` ~50
- `internal/filter/http/fuzz_test.go` ~80
- `internal/filter/http/cors/cors.go` ~150 + `cors_test.go` ~200 + `doc.go` ~20 = ~370
- `internal/filter/http/envoygotest/filter.go` ~250 + `filter_test.go` ~400 + `proto/envoygotest.pb.go` ~30 + `doc.go` ~20 = ~700
- `internal/filter/http/router/router.go` ~250 + `router_h2.go` ~250 + `router_test.go` ~300 (byte-preserved) + `router_h2_test.go` ~300 (byte-preserved) + `doc.go` ~20 = ~1120 (most of this is migration, not new authoring — the byte-preserved tests sum ~600 of the ~1120)
- `internal/filter/hcm/config.go` extension (parseFilterWithCtx accepts *HTTPRegistry; chain config build) ~100 + `config_test.go` extension ~150 = ~250
- `internal/filter/hcm/filter.go` extension (struct field + constructor widen) ~30 + `filter_test.go` extension ~50 = ~80
- `internal/filter/hcm/actions.go` deletion (routerAction + routerActionH2 removed) -300 + `actions_test.go` deletion -300 = -600 net (offset by the router/ migration above)
- `internal/filter/hcm/connection.go` extension (H1 dispatch runs FilterChain) ~50 + `connection_test.go` extension ~80 = ~130
- `internal/filter/hcm/h2dispatch.go` extension (H2 dispatch runs FilterChain) ~50 + `h2dispatch_test.go` extension ~80 = ~130
- `internal/filter/hcm/chain_integration_test.go` (NEW) ~250
- `internal/filter/doc.go` rewrite ~30
- `internal/listener/manager.go` extension (Registry threading) ~10 + `manager_test.go` extension ~30 = ~40
- `internal/bootstrap/bootstrap.go` extension (cors v3 blank import) ~3 + `bootstrap_test.go` extension ~10 = ~13
- `cmd/envoy-go/main.go` extension (alloc + register + Freeze + thread) ~20 + `main_test.go` extension ~40 = ~60
- `test/differential/runner.go` extension (registration of 0007a + 0007b) ~6
- `test/differential/0007a-cors/` (envoy-go.yaml ~80 + envoy.yaml ~80 + expectations.yaml ~50 + README.md ~80 + driver/driver.go ~280 + driver/driver_test.go ~80 + backends/main.go ~50) = ~700
- `test/differential/0007b-iteration-probe/` (envoy-go.yaml ~80 + expectations.yaml ~80 + README.md ~80 + driver/driver.go ~350 + driver/driver_test.go ~100 + backends/main.go ~50) = ~740
- `docs/envoy-go/DECISIONS.md` (seven ADRs) ~500
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place edit + amendments) ~200
- `docs/envoy-go/ROADMAP.md` (row updates) ~3
- `docs/envoy-go/STATE.md` (lifecycle transitions) ~5
- `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` ~250

Total estimate: **~6500 LoC** with a net-after-deletions effective of **~5500 LoC** (the 600 LoC of router/router_h2 deletions in `hcm/actions.go` partially offset the 1120 LoC of `internal/filter/http/router/` additions; ~520 LoC of the router-pkg LoC is NEW production code the framework requires, not migration). Test-LoC is ~3000 of the total; production-LoC is ~3500. Task count is **23** — below the 25-task gate (`BOOTSTRAP_PROMPT.md` §6.1's primary signal). LoC estimate is well above the soft 1500-threshold OR-leg, comparable to 06.1 (~3300 actual) and 06.2 (~4000 actual; 06.2 PLAN's estimate was ~3000) which both shipped as one phase. The phase-04 / 05.1 / 05.2 / 06.1 / 06.2 precedent is that task-count-under-25 is the load-bearing signal; the LoC OR-leg has been exceeded in four of the five prior phases without splitting, when the surface is structurally atomic.

Phase 07.1 ships as **one** sub-phase (NOT split into 07.1.1 + 07.1.2 — even though the natural surface axis exists: 07.1.1 = framework + HCM dispatch wiring + router migration; 07.1.2 = filters + fixtures) for three reasons:

1. **The surface-axis split (07.1.1 framework + dispatch wiring; 07.1.2 filters + fixtures) creates vacuous gate (a) on 07.1.1.** Per BOOTSTRAP §6.3 ("do not ship incomplete stubs that conformance tests can't exercise"), a 07.1.1 carrying only the iteration protocol + registry + chain state machine + HCM dispatch wiring + router migration would have NO new differential fixture exercising the framework's iteration-protocol claim — fixture 0007a (cors differential) and fixture 0007b (iteration-probe structural) BOTH need filter implementations to exist (cors filter for 0007a; envoy_go_test for 0007b). The pre-existing fixtures 0003-http11-routing and 0004-h2-routing would still be green on 07.1.1 (the router still routes), but they only exercise the router-as-terminal-filter shape — they do not exercise StopIteration, async-resume, SendLocalReply, or buffer overflow. A 07.1.1 with the framework but no filters that exercise the framework is exactly the "incomplete stub" anti-pattern §6.3 targets. Splitting also leaves the `*HTTPRegistry` allocated-but-empty (or with only `router.New` registered) in 07.1.1's `cmd/envoy-go/main.go` until 07.1.2 wires up `cors.New` + `envoygotest.New` — dead infrastructure until 07.1.2.

2. **Task count is below the 25-task gate; LoC estimate is at the OR-leg with established phase-04 / 05.1 / 05.2 / 06.1 / 06.2 precedent.** Per phase-04 / 05.1 / 05.2 / 06.1 / 06.2 precedent, task-count-under-25 is the primary signal that one phase is the right shape. 07.1's 23 tasks fits with margin; the SPEC §1 estimated 12-16-task range was for the production-code surface only, and the +7 tasks for fixtures + ADRs + closing sweep are within the gate. The ~5500-LoC effective estimate is comparable to 06.1's ~3300 actual landed and 06.2's ~4000 actual; the OR-leg has been exceeded in four of five prior phases without splitting, and the structural-atomicity argument (#1 above) precludes splitting on the surface axis.

3. **The framework + cors filter + envoygotest filter + router migration + dispatch wiring + both fixtures form one atomic load-bearing claim.** Per BOOTSTRAP §6.3 + SPEC §1, the central engineering claim of 07.1 is "envoy-go runs a real multi-filter HTTP iteration chain — filters can stop, buffer, resume async, modify decode/encode headers and bodies, synthesize local replies that re-enter the encode chain — and produces behaviorally-equivalent CORS responses to upstream Envoy." Removing fixture 0007a (cors differential) reduces 07.1 to "framework exists but is not differentially equivalence-claimed" — fails the D-3.3 differential-correctness doctrine. Removing fixture 0007b (iteration-probe structural) reduces 07.1 to "differential gate green on cors, but iteration-protocol state branches not exercised under test" — fails BOOTSTRAP §6.3 because StopIteration / StopIterationAndBuffer / async-resume / SendLocalReply / buffer-overflow are then untested. Removing the router migration leaves the chain with a hard-coded terminal that bypasses iteration — not the architecture this phase ships. Removing the framework leaves cors/envoygotest with nothing to plug into. The five components (framework, router migration, dispatch wiring, cors+envoygotest filters, both fixtures) form a coherent atomic unit.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **9000** by the end of Task 20 (i.e., before fixture 0007a + 0007b's driver tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A ~64% miss on a carefully-bounded sub-phase is a signal the plan's shape is wrong, not just that the work is large. Mid-execution split valve: `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger (any single task's sub-steps blow up past ~10 items) stays active. The two tasks most likely to blow past 10 sub-steps are Task 5 (`chain.go` decode-side iteration — the largest single-file change after the fixture drivers, includes StopIteration parking + the resume-channel mechanics) and Task 21 (fixture 0007a driver — orchestrates 4-request workload + per-request equivalence + per-route config differential + STATIC-vs-STRICT_DNS divergence). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR — the natural axis remains 07.1.1 (framework + dispatch wiring + router migration) and 07.1.2 (filters + fixtures), with the caveat from #1 above that 07.1.1 has vacuous gate (a) and would need a placeholder probe filter to land non-vacuously.

---

## ADRs introduced by this plan

Seven ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0069** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` → `ADR-0069:` at the master-`f2dd659`-then-SPEC-commit baseline; the planner re-verified at PLAN-write time that ADR-0066..ADR-0069 all landed in the 06.2 phase-done + sha-fill commits per SPEC §8 anticipation; if a mid-PLAN-authoring ADR landed since the SPEC commit, re-number 07.1 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 2 — the executor checks at Task 1 step 1). Per SPEC §8, phase 07.1's seven ADRs land at ADR-0070..ADR-0076 in topical order. The topic-to-ADR-number map:

- **SPEC §8 ADR-0070 anticipation** (Phase-07 planner-time split into 07.1 + 07.2) → **ADR-0070** (lands Task 1, the PROGRESS preamble; first ADR of the seven; the split is a process decision that documents the SPEC-drafting session's already-landed ROADMAP edit at master `ee45aba`, so anchoring it at T1 — the implementation session's first commit — is the natural fit since this is the first opportunity to land the ADR after the SPEC commit's ROADMAP edit).
- **SPEC §8 ADR-0071 anticipation** (HTTP filter iteration protocol shape; supersedes ADR-0040 totally; partially supersedes ADR-0042) → **ADR-0071** (lands Task 2, the `internal/filter/http/types.go` + `callbacks.go` introduction; first use of the iteration-protocol shape in production code; the architectural shape applies to every subsequent task in the package).
- **SPEC §8 ADR-0072 anticipation** (`*HTTPRegistry` threaded constructor map, no package-global) → **ADR-0072** (lands Task 3, the `internal/filter/http/registry.go` introduction; first use of the threading discipline in production code; mirrors `*stats.Registry` LBP-1 from ADR-0059).
- **SPEC §8 ADR-0073 anticipation** (`typed_per_filter_config` 3-tier merge model; amends ADR-0041's silent-ignore set by promoting `typed_per_filter_config` from silent-ignored to honored) → **ADR-0073** (lands Task 4, the `internal/filter/http/perroute.go` introduction; first use of the merge logic in production code).
- **SPEC §8 ADR-0075 anticipation** (`sendLocalReply` encode-chain semantics; anchors §11 #4 empirical pin) → **ADR-0075** (lands Task 7, the `chain.go` `beginLocalReply` + first-call-wins implementation; first use of the encode-chain-entry-at-`filter[len-1]` semantics in production code).
- **SPEC §8 ADR-0076 anticipation** (Body buffer cap; 413 on decode overflow; reset on encode overflow; amends ADR-0041 silent-ignore set by adding `per_connection_buffer_limit_bytes` + `per_request_buffer_limit_bytes`) → **ADR-0076** (lands Task 9, the `chain.go` buffer-overflow path implementation; first use of the 413 verbatim shape + connection-reset discipline in production code).
- **SPEC §8 ADR-0074 anticipation** (Trivial filter set: `cors` real differential + `envoy_go_test` test-only structural) → **ADR-0074** (lands Task 18, the `internal/filter/http/cors/cors.go` introduction; first use of the filter-set composition in production code; the iteration-state coverage attribution table in the ADR enumerates which `envoy_go_test` mode tests which protocol surface).

Note: the FIRST-USE ORDERING is Tasks 1, 2, 3, 4, 7, 9, 18 — i.e. ADR-0070 first (T1), ADR-0071 second (T2), ADR-0072 third (T3), ADR-0073 fourth (T4), ADR-0075 fifth (T7), ADR-0076 sixth (T9), ADR-0074 seventh (T18). This produces an ADR-number-vs-commit-order sequence (0070, 0071, 0072, 0073, 0075, 0076, 0074) — non-monotonic at the last entry. Per SPEC §8's explicit permission ("the planner may permute commit-time landings if that reads more naturally in PLAN.md") and per the 05.2 ADR-0055..ADR-0058 + 06.1 ADR-0059..ADR-0064 + 06.2 ADR-0066..ADR-0069 precedents (all three used non-monotonic commit-time orderings), the non-monotonic mapping is correct here. The contiguous-block discipline (ADR-0070..ADR-0076 inclusive, no gaps) is preserved; topical coherence drives the in-task pairing (ADR-0074 lands at Task 18 because that's where the cors filter's TypeURL constant is first defined and its `New` factory is first registered into the chain via the `cmd/envoy-go/main.go` boot wiring at Task 20 — but the Task 18 commit is the production-code anchor; ADR-0075 lands at Task 7 because that's where `beginLocalReply` is defined; ADR-0076 lands at Task 9 because that's where the 413 synthesis path is implemented). The PLAN documents the mapping explicitly so the executor doesn't "fix" the ordering at execution time.

Summaries:

- **ADR-0070 — Phase-07 planner-time split (07.1 + 07.2).** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 + D-3.6. Decision: phase 07 (filter-chain framework) splits into 07.1 (HTTP filter framework — under HCM) + 07.2 (listener-chain completion — under listener manager) at planner-time per ADR-0045's pattern (which documented the 05.1 + 05.2 split). Rationale (per BRAINSTORM §1 + parent SPEC §3): the two halves have disjoint code surfaces (`internal/filter/http/` + `internal/filter/hcm/` for 07.1 vs `internal/listener/` for 07.2); 07.1-first ordering is correct because 07.1 unblocks the BOOTSTRAP §9 HTTP-filters family (every future HTTP filter — `header_manipulation`, `fault`, `jwt_authn`, `ext_authz`, etc. — depends on the iteration protocol + extension registry shipping in 07.1) while 07.2 has no §9 dependents. Mirrors ADR-0045 (phase 05 split: 05.1 downstream H2, 05.2 upstream H2) and the phase-06 split (06.1 stats, 06.2 access-log). Lands in Task 1 (the PROGRESS preamble — first commit of the implementation session; the ROADMAP edit that the split anchors already landed at master `ee45aba` per SPEC drafting). The parent row 07 closes only at 07.2's phase-done (mirroring 05/05.1/05.2 + 06/06.1/06.2 closure pattern). Supersedes nothing.

- **ADR-0071 — HTTP filter iteration protocol shape.** Status: Accepted. Date: task-execution date. Doctrine: D-3.2 (write from scratch — no third-party filter-chain library) + D-3.5 (record durable design rationale). Decision: Envoy-faithful subset with async-resume; narrow method set on filter interfaces (`DecodeHeaders/Data/Trailers`, `EncodeHeaders/Data/Trailers`, `SetDecoderCallbacks` / `SetEncoderCallbacks`, `OnDestroy`); status enums settled (`FilterHeadersStatus`: `Continue` / `StopIteration`; `FilterDataStatus`: `Continue` / `StopIterationAndBuffer` / `StopIterationNoBuffer`; `FilterTrailersStatus`: `Continue` / `StopIteration`); explicitly out-of-MVP (`ContinueAndDontEndStream`, watermark variants `StopAllIterationAndWatermark`, `Encode1xxHeaders`, metadata frames `decodeMetadata`/`encodeMetadata`). Two-step factory pattern: `HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` parses + validates `typed_config` once at HCM-build time; `FilterInstanceFactory func() HTTPFilter` allocates a fresh filter instance once per request. Async-resume via per-stream buffered channel (`chan struct{}` capacity 1; non-blocking sends; idempotent coalesce); single-goroutine-per-request iteration invariant — the HCM dispatch goroutine is the only goroutine that drives chain iteration; filter callbacks called from filter-spawned goroutines are signal-only (channel send to wake the dispatch goroutine) and do NOT enter chain iteration themselves. Rationale (per BRAINSTORM §2.1 + §5.1): MVP scoping that ships the iteration protocol's load-bearing surfaces while deferring rarely-used Envoy features (1xx, metadata, watermark) to first-use phases; single-goroutine iteration makes the chain's internal state lock-free; the two-step factory pattern mirrors Envoy's `FilterFactoryFn` and amortizes typed_config validation across requests. Alternatives considered: (A) Envoy's full method set — rejected for YAGNI (D-3.5; the methods we drop have no in-scope callers in 07.1's filter set, and any future filter that needs them would re-litigate via its own ADR); (B) per-filter goroutine — rejected because spawning a goroutine per filter per request is goroutine-bloat for the common-case (filters that all return Continue) and the framework's invariant is single-goroutine iteration (Envoy's discipline mirrored). Consequences: (a) the framework's external dependencies are limited to the Go stdlib + `google.golang.org/protobuf` + `internal/cluster` (router sub-package only) — no third-party filter-chain-engine; (b) the iteration-protocol shape documented in `internal/filter/http/doc.go` (the package overview); (c) future family phases that introduce additional iteration features (1xx, metadata, watermark) extend this package by adding to the `StreamDecoderFilter` / `StreamEncoderFilter` interfaces — each such addition lands its own ADR in the family phase that needs it. **Supersedes ADR-0040 totally** (router-as-direct-call inside HCM connection loop is replaced by router-as-terminal-filter via the iteration protocol). **Partially supersedes ADR-0042** (the "exactly `[router]`" rule's lower bound stays as "must contain router as last entry"; the upper bound "exactly `[router]`" is lifted to "non-empty; last entry must be router"). Lands in Task 2 (the iteration-protocol introduction).

- **ADR-0072 — `*HTTPRegistry` threaded constructor map, no package-global.** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale; the threading discipline is a contract that future package consumers MUST observe). Decision: the `*HTTPRegistry` is constructed once at boot in `cmd/envoy-go/main.go`, threaded explicitly into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)` via the listener-manager's HCM-construction path, NOT a package-global registered via `init()`. Freeze-after-boot invariant mirrors `*stats.Registry` LBP-1 from ADR-0059: `HTTPRegistry.Freeze()` is called from `cmd/envoy-go/main.go` after all `Register` calls; any subsequent `Register` panics with `filter: registry frozen: cannot register %q post-boot`; `Lookup` does not panic post-Freeze (read-allowed). Three filters registered at boot: `router.New` (`envoy.filters.http.router`), `cors.New` (`envoy.filters.http.cors`), `envoygotest.New` (`envoy.filters.http.envoy_go_test`). Rationale (per BRAINSTORM §5.1 + Decision §3.2): `init()`-based global registries make test isolation hard (each test wants its own filter set), tie filter-set composition to import-graph layout (a future build-tag-gated filter would flip imports unpredictably), and contradict the `*stats.Registry` LBP-1 precedent the project established in 06.1. Explicit threading is the same pattern 06.1 ADR-0059 invoked for stats. Alternatives considered: (A) `init()`-based global — rejected for the test-isolation + import-graph reasons above; (B) interface-injection without freeze (just a `Lookup` interface) — rejected because the freeze-after-boot invariant is the load-bearing test for "no late filter registration"; without it, a future bug that calls `Register` post-boot fails silently rather than loudly. Consequences: (a) all HCM constructors widen by one parameter (`*filter.HTTPRegistry`); pre-existing call sites in `cmd/envoy-go/main.go`, `internal/listener/manager.go`, and tests update mechanically (Decision §3.4 settles whether the legacy constructors are deleted or kept as forwarding shims); (b) the freeze-after-boot invariant is grep-verifiable in `registry_test.go` (post-Freeze `Register` panics); (c) future Observability-family / xDS-family / WASM-family phases that introduce additional filter types extend this registry by registering their factories at boot — no architectural churn needed. Lands in Task 3 (the `internal/filter/http/registry.go` introduction). Supersedes nothing; complements ADR-0059.

- **ADR-0073 — `typed_per_filter_config` 3-tier merge model.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5. Decision: `typed_per_filter_config` is honored at parse-time on Route, VirtualHost, and RouteConfiguration scopes; merge order is Route > VirtualHost > RouteConfiguration with most-specific-override (no field-merge); lazy cache `map[filterName]proto.Message` on first `RequestRouteConfig()` call per request, populated incrementally; build-time validation: keys MUST reference filter names present in the chain's `http_filters[]`; unknown filter names error at parse with `hcm: route_config: typed_per_filter_config: unknown filter name %q (chain has [...])`. **Honored at parse-time — partial supersession of ADR-0041's silent-ignore set:** `typed_per_filter_config` moves from silent-ignored (in phases 04/05.1/05.2) to honored. Rationale (per BRAINSTORM §2.3 + SPEC §12 #3): per-route-config is a load-bearing primitive for almost every real Envoy filter (cors policies vary per-route; jwt_authn provider IDs vary per-route; ratelimit descriptors vary per-route); shipping the framework without it would fail the cors differential at the first route-vs-route differential exercise. Most-specific-override (vs Envoy's optional field-level merge mode with `disabled` flag) is the simpler shape; field-level merge is deferred to first family phase that demands it via an Envoy-equivalent test (per SPEC §2.2). Lazy cache (vs eager pre-allocation) mirrors the lock-free hot-path discipline ADR-0066 established for access-log (filters that don't call `RequestRouteConfig` pay zero cost). Alternatives considered: (A) eager `[]proto.Message` indexed by filter chain index — rejected for allocation-in-common-case reason above; (B) field-level merge mode — rejected for YAGNI in 07.1 (no in-scope filter consumes the field-merge semantic; cors policy is most-specific-override regardless). Consequences: (a) the silently-ignored field set is amended (per ADR-0041's amendment shape, mirroring the 05.1 + 05.2 + 06.1 + 06.2 amendments) — `typed_per_filter_config` is REMOVED from the silent-ignore set on Route/VirtualHost/RouteConfiguration; (b) `filter.disabled` flag stays silent-ignored at parse-time (per SPEC §2.2 + §9; deferred to family phase that demands it); (c) future fixtures that exercise field-level merge will land their own ADR superseding the most-specific-override discipline if needed. Lands in Task 4 (the `internal/filter/http/perroute.go` introduction). Supersedes nothing; amends ADR-0041.

- **ADR-0074 — Trivial filter set: `cors` real differential + `envoy_go_test` test-only structural.** Status: Accepted. Date: task-execution date. Doctrine: D-3.3 (own the canonical observation surface; the differential equivalence claim lives here) + D-3.4 (record durable design rationale; the test-only filter's role is non-obvious without an ADR). Decision: phase 07.1 ships exactly two filters in addition to the migrated `router`: (a) `envoy.filters.http.cors` — real Envoy filter, used by differential fixture `0007a-cors`; (b) `envoy.filters.http.envoy_go_test` — test-only probe filter, used by structural fixture `0007b-iteration-probe`. The `envoygotest` proto schema is envoy-go-only (hand-rolled at `internal/filter/http/envoygotest/proto/envoygotest.pb.go`; not in upstream go-control-plane; does NOT extend the upstream registry). Iteration-state coverage attribution: each of `envoy_go_test`'s 8 modes targets a specific iteration-protocol surface (`continue`: baseline pass-through; `stop-and-resume-headers`: StopIteration on decode-headers + async-resume via channel; `stop-and-buffer-data`: StopIterationAndBuffer + body modification; `local-reply-decode`: SendLocalReply on decode-headers; `local-reply-decode-data`: SendLocalReply mid-decode-data; `modify-encode-headers`: encode-side header injection; `modify-encode-data`: encode-side body modification; `stop-trailers`: StopIteration on decode-trailers + async-resume). The 8-mode matrix is exhaustive over the framework's iteration-state branches per SPEC §7.3. Rationale (per BRAINSTORM §2.4 + §6.1): `cors` is the load-bearing differential filter — it exercises (a) per-route config (different `CorsPolicy` per route), (b) encode-side header injection on the upstream response, (c) `SendLocalReply` on preflight (synthesized response that re-enters the encode chain), (d) mode-dispatched decode-side branching (preflight vs actual-request); these four surfaces cover most of the framework's claim. `envoy_go_test` covers iteration-protocol state branches that no single real filter covers (the 8-mode matrix); shipping a test-only probe filter is the same pattern Envoy ships internally for its own iteration-protocol unit tests (`source/extensions/filters/http/dynamic_forward_proxy/`-style probes). Alternatives considered: (A) `cors` only (no `envoy_go_test`) — rejected because cors does not exercise StopIterationNoBuffer, async-resume on decode-trailers, or encode-side body modification; gate (a)'s structural-coverage claim would have holes; (B) more real filters (e.g., add `header_manipulation` for encode-side header modification coverage) — rejected for scope-creep; one real filter is enough to ship the differential claim, and `envoy_go_test`'s 8 modes provide structural coverage at lower implementation cost than three real filters; (C) `envoy_go_test` only (no `cors`) — rejected because gate (a) would be vacuous (no real-Envoy-equivalence claim); the cors filter is the sole differential-equivalence anchor. Consequences: (a) the registry is populated with three filters at boot (`router`, `cors`, `envoygotest`); (b) the `envoygotest` proto schema is in-tree (hand-rolled), and the SPEC §15 final acceptance bullet asserts no upstream-go-control-plane registry extension; (c) future HTTP-filter-family phases (BOOTSTRAP §9) extend the registered filter set; the `envoy_go_test` filter STAYS as the canonical iteration-protocol probe (it does not get removed by future phases — it is the project's iteration-protocol regression net). Lands in Task 18 (the `internal/filter/http/cors/cors.go` introduction; the cors+envoygotest filter-set composition is anchored at Task 18 because cors is the load-bearing differential filter; the envoygotest filter's introduction at Task 19 inherits the ADR's filter-set framing without re-anchoring). Supersedes nothing.

- **ADR-0075 — `sendLocalReply` encode-chain semantics.** Status: Accepted. Date: task-execution date. Doctrine: D-3.3 (the synthesized-response shape is differentially observable; the empirical pin is the durable evidence) + D-3.5 (record the rationale durably). Decision: when a filter calls `cb.SendLocalReply(status, body, headers)` from any callback (decode or encode side), the framework: (a) marks decode-side aborted at the calling filter's index (cancels any pending resume); (b) constructs the synthesized response (merging framework-injected standard headers — `content-length`, `content-type`, `date`, `server` — with the user-supplied headers); (c) enters the encode chain at `filter[len-1]` of the encode-side filter set (NOT at the calling filter's index, NOT at index 0); (d) iterates the FULL encode chain in reverse order (every encode-side filter runs, including the calling filter's own encode side); (e) first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log line `hcm: filter %q called SendLocalReply after encode-side started; ignoring`. The empirical pin in SPEC §11 #4 is the durable evidence (verified at SPEC time against reference Envoy v1.37.2 with chain `[lua_a, lua_b, lua_c, router]` where `lua_b` calls Envoy's `respond` API; observed encode order `lua_c → lua_b → lua_a` — i.e., entry at filter[len-1] of the encode-side set; ALL three Lua filters' encode sides ran). Rationale (per BRAINSTORM §4.5 + SPEC §11 #4): the filter[len-1] entry point is what Envoy does (verified empirically); the full-encode-chain-runs-in-reverse discipline preserves encode-side filter contracts (e.g., a header-injection filter's encoded headers are observable on the synthesized response too); first-call-wins prevents duplicate-response races (filter[0] starts iteration; filter[2] resumes from async and calls SendLocalReply before filter[1]'s pause completes — first wins). Alternatives considered: (A) entry at the calling filter's encode index (NOT filter[len-1]) — rejected because it diverges from Envoy and would break differential equivalence on the cors filter's preflight path (cors is at filter[0]; if it called SendLocalReply, an entry-at-calling-index discipline would skip the router's encode side, breaking the encode-chain contract); (B) skip the calling filter's own encode side (since it produced the response) — rejected per SPEC §12 #6 + §11 #4 empirical pin (Envoy uses (a): the calling filter's encode side runs); (C) parallel encode-side iteration on SendLocalReply (faster) — rejected because parallel iteration breaks the ordering contract (encode-side filters declare their order; a header-mutation filter at index 1 must observe and possibly modify what filter at index 2 emitted). Consequences: (a) the `chain.beginLocalReply` implementation in `chain.go` (Task 7) honors the four sub-decisions (a–e) verbatim; (b) the unit test `TestChain_SendLocalReply_EntersAtLenMinus1` in `chain_test.go` asserts the encode-iteration entry point on a synthetic 4-filter chain; (c) the BEHAVIOR_CONTRACT addition at Task 23 carries the §11 #4 empirical-pin block verbatim (no drift permitted; the §11 block + the §13 block are paste-verbatim-synchronized). Lands in Task 7 (the `chain.go` `beginLocalReply` implementation; first use of the encode-chain-entry-at-`filter[len-1]` semantics in production code). Supersedes nothing.

- **ADR-0076 — Body buffer cap; 413 on decode overflow; reset on encode overflow.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 (record durable design rationale) + D-3.6 (every phase is a green build; the 413 + reset disciplines are load-bearing for the overflow-safety claim). Decision: `filterBufferLimitBytes = 1 << 20` (1 MiB) is a hardcoded constant matching Envoy's default. Decode-side buffer overflow on `StopIterationAndBuffer` synthesizes a `413 Payload Too Large` local reply with verbatim shape per SPEC §11 #3 empirical pin: status `413 Payload Too Large`; body 17 bytes ASCII `Payload Too Large` (no trailing newline); response headers in wire order: `content-length: 17`, `content-type: text/plain`, `date: <stamp>`, `server: envoy`, `connection: close`. The 413 then flows through the encode-side filter chain (full encode chain runs per ADR-0075 semantics, since the 413 is a SendLocalReply-equivalent). Encode-side buffer overflow → connection reset (H1: emit no further bytes and close conn; H2: RST_STREAM after the local-reply HEADERS+DATA frames). The configurable knobs `per_connection_buffer_limit_bytes` (Listener-scope) and `per_request_buffer_limit_bytes` (Route-scope) are silently ignored at parse-time per ADR-0041 amendment. Rationale (per BRAINSTORM §2.1 + SPEC §11 #3): hardcoded matches Envoy's default; configurable knobs are deferred to a dedicated buffer-policy phase (or first family phase that needs tuning) — shipping the configurable knobs would require implementing per-listener + per-route per-stream buffer-pool plumbing that 07.1's framework does not need; 413 verbatim shape is what Envoy emits (verified empirically at SPEC time); encode-side reset (rather than 413) is necessary because once response headers have been written to the wire, a 413 cannot be synthesized — the only honest signal is connection termination; H1's `connection: close` discipline forces the conn to close after the 413 (matching Envoy). Alternatives considered: (A) configurable buffer cap from day 1 — rejected for YAGNI (no in-scope filter consumes the configurable knob; `cors` and `envoygotest` operate in well-bounded body-size regimes); (B) 413 on encode-side overflow (instead of reset) — rejected because it is impossible (response started); (C) unbounded buffering — rejected for OOM-on-overload reasons (matches access-log's drop-newest discipline from 06.1 ADR-0066, applied here as overflow-413 on the body-buffer surface). Consequences: (a) the silently-ignored field set is amended (per ADR-0041's amendment shape) — `per_connection_buffer_limit_bytes` (Listener) + `per_request_buffer_limit_bytes` (Route) are added to the silent-ignored set; (b) the 413 verbatim shape is grep-verifiable in `chain_test.go` (Task 9); (c) future buffer-policy phases that ship the configurable knobs supersede this ADR's silent-ignore disposition (the hardcoded constant + the 413 + reset disciplines stay; the silent-ignore is what gets superseded). Lands in Task 9 (the `chain.go` buffer-overflow path implementation; first use of the 413 verbatim shape + connection-reset discipline in production code). Supersedes nothing; amends ADR-0041.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0077+) in the same commit as the code it decides for. If such a decision would expand phase-07.1 scope beyond SPEC §1–§13, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6 — noting that 07.1 SPEC §1's anticipated 12-16-task production-code surface plus the +7 fixtures/ADRs/closing-sweep tasks brings 23 total; scope-expansion absorption preserves the task count by absorbing the new ADR's anchoring task into an existing task; defer if the absorption would push the absorbing task past the §6.1 secondary-trigger of ~10 sub-steps.

---

## Settled SPEC §12 deferred decisions

SPEC §12 leaves eight 07.1-scoped implementation-detail choices to the planner. This PLAN settles them so the executor does not re-litigate. Only decisions with cross-phase impact are also captured as ADRs.

### §3.1 HCM dispatch hook for chain-completion → access-log emit trigger

**Decision: Adopt SPEC §12 #1 option (a)** — uniform single trigger site at the end of `chain.runEncodeData(endStream=true)` / `chain.runEncodeTrailers`. The 06.2 access-log emit-deferral hook fires from four sites (`directResponseAction.do`, `routerAction.do`, `h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`); after 07.1, the router-action path moves into the router filter, so the four sites collapse into one chain-completion site that fires regardless of whether the response was synthesized by a SendLocalReply, returned from upstream, or generated by `directResponseAction`. The `accesslog_emit.go` body is preserved verbatim; only the call sites move. Rationale: option (a) is uniform (one site replaces four), matches the chain-completion semantic (the access log records the request's terminal state, which is unambiguously chain-encode-completion regardless of decode-side abort path), and naturally handles the SendLocalReply-path access-log emission (the synthesized response goes through the encode chain → triggers the chain-completion hook → emits the access-log record). Option (b) would split into two sites (one at SendLocalReply, one at natural router-encode), creating two divergent code paths to maintain. **Cross-phase impact: NO** — this is a 07.1 implementation detail; ADR-0066's access-log architecture is unchanged. **Code anchor:** Task 15 step 3 (H1 connection.go), Task 16 step 3 (H2 h2dispatch.go).

### §3.2 `HTTPRegistry.Lookup` race-vs-iterate during boot

**Decision: Already safe by construction.** Two HCM constructors may race on `Lookup` during `listenerManager.New`'s loop. Both are RLock-only (lock-free against `Register` only on the post-Freeze path; pre-Freeze, `Register` Lock-vs-Lookup-RLock is naturally sequenced by the registry's `RWMutex`). The freeze invariant + the listener-manager's strict ordering (Freeze runs *before* `listenerManager.New`) make this safe by construction. **Cross-phase impact: NO** — codified in ADR-0072. **Code anchor:** Task 3 step 6 — unit test `TestRegistry_PostFreezeRegisterPanics` asserts the panic; Task 20 step 4 — `cmd/envoy-go/main.go` calls `Freeze()` BEFORE `listenerManager.New(...)`; precondition #11 at Task 1 step 1 grep-verifies the strict ordering at execution-precondition check time.

### §3.3 Per-route config cache placement

**Decision: Adopt SPEC §12 #3 option (a)** — `map[filterName]proto.Message` allocated lazily on first `RequestRouteConfig` lookup, populated incrementally; cache lives on the per-stream `FilterChain`. Rationale: minimal allocation in the common case (filters that don't call `RequestRouteConfig` pay zero cost — no allocation, no map zero-value initialization); slight map-lookup cost in the hot path is irrelevant compared to the iteration overhead. Option (b) (`[]proto.Message` indexed by filter chain index) would pre-allocate one slot per filter even if no filter calls `RequestRouteConfig` — wasted allocation in the common case. **Cross-phase impact: NO** — codified in ADR-0073. **Code anchor:** Task 4 step 3 (`perroute.go` lazy-cache implementation), Task 5 step 4 (`chain.go` cache lifecycle on stream allocation).

### §3.4 HCM constructor signature widening: forward or delete

**Decision: DELETE the legacy constructors and update all call sites mechanically.** SPEC §4.2 enumerates the four legacy constructors (`NewFilter`, `NewFilterWithCtx`, `NewFilterWithCtxAndSinks`) and the new `NewFilterWithCtxAndSinksAndRegistry`. Per SPEC's "either remain (forwarding to the new constructor with a default-empty registry) or are deleted with all call sites updated" — this PLAN chooses **delete**. Rationale: the existing call sites are limited to `cmd/envoy-go/main.go` (one site), `internal/listener/manager.go` (one site), and HCM unit tests (~15 sites in `config_test.go` / `filter_test.go` / `connection_test.go` / `h2dispatch_test.go` / `actions_test.go` / `chain_integration_test.go` (NEW) / `accesslog_emit_test.go`); each site updates with one line (additional `*filter.HTTPRegistry` parameter); forwarding shims would carry no maintenance benefit (the legacy constructors have no external consumers; this is an internal package). Deletion also surfaces any forgotten call site at compile time (vs forwarding which would silently accept the old signature and pass through a default-empty registry — masking a missed update). The new constructor is the SOLE entry point: `func NewFilterWithCtxAndSinksAndRegistry(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx, registry *stats.Registry, accessLogSinks []accesslog.Sink, httpRegistry *filter.HTTPRegistry) (*Filter, error)`. The four `NewFilter*` symbols disappear from the package. **Cross-phase impact: NO** — internal-API-only. **Code anchor:** Task 14 step 2 (`filter.go` constructor widening; legacy deletion); Task 14 step 3 (call site sweep + compile verification across the package).

### §3.5 `FilterChain` decode/encode iteration index types

**Decision: Adopt SPEC §12 #5 option (a)** — two separate `int` cursors on the chain struct (`decodeIdx int`, `encodeIdx int`). Rationale: simpler; the two cursors are independent surfaces (decode iterates 0 → len-1; encode iterates len-1 → 0; the two are not related except via the SendLocalReply transition). Option (b) (a single `iterState struct { idx int; phase decodeOrEncode }` with helpers) adds no benefit at this scale — the two cursors carry independent invariants and the helpers would be 1-line wrappers. **Cross-phase impact: NO** — internal struct shape. **Code anchor:** Task 5 step 2 (`chain.go` struct shape), Task 6 step 2 (encode cursor introduction).

### §3.6 Chain-config mutation discipline at `chain.beginLocalReply`

**Decision: Pinned to SPEC §11 #4 empirical evidence — the calling filter's encode side runs.** When a filter calls SendLocalReply during decode-side iteration, the chain marks decode aborted at the calling filter's index and skips remaining decode-side filters; the encode-side iteration starts from `len(filters)-1` regardless and runs the FULL encode chain in reverse order (every encode-side filter runs, INCLUDING the calling filter's own encode side). Per SPEC §11 #4 empirical pin, Envoy uses this discipline (with chain `[lua_a, lua_b, lua_c, router]` and lua_b calling `respond`, the observed encode order is `lua_c → lua_b → lua_a` — `lua_b`'s encode side runs even though it was the SendLocalReply caller on the decode side). envoy-go MUST replicate this discipline. **Cross-phase impact: YES** — codified in ADR-0075. **Code anchor:** Task 7 step 4 — unit test `TestChain_SendLocalReply_CallingFilterEncodeRuns` on a synthetic 4-filter chain asserts the calling filter's encode side runs; Task 7 step 2 — `chain.beginLocalReply` does NOT skip the calling filter's encode side (the encode-iteration cursor advances normally from `len-1` down to `0`).

### §3.7 Test-only probe filter mode-dispatch storage

**Decision: Adopt SPEC §12 #7 option (b)** — a `switch mode { case "continue": ... case "stop-and-resume-headers": ... }` body. Rationale: a switch is more debuggable (the executor can step through the dispatch path with a stdlib debugger, observe each case's branch); makes the mode set discoverable from the source (a future reader sees the 8 cases enumerated in one place; no separate map literal to find); matches the 06.2 `directResponseAction` / `routerAction` precedent of "explicit dispatch over a closed enum"; avoids the function-pointer indirection that adds CPU-branch-prediction noise during bench-mark runs. Option (a) (a `map[string]func(*filter)` table) would scatter the dispatch logic across map-literal initialization + 8 separate functions — harder to read in source-order. **Cross-phase impact: NO** — internal to `envoygotest/filter.go`. **Code anchor:** Task 19 step 2 (`envoygotest/filter.go` mode-dispatch switch), Task 19 step 4 (per-mode unit test exercises each case).

### §3.8 Fixture 0007a + 0007b driver pattern: in-band assertions vs. generic Driver-interface extension

**Decision: Adopt SPEC §12 #8 in-band.** Both fixture 0006 (06.2) and fixture 0005 (06.1) went in-band; fixtures 0007a + 0007b should too. The driver interface stays minimal (`BackendCount()`, `SubjectListenerName()`, `ReferenceListenerPort()`, `RequiresReference()`, `DriveSubject(ctx, addr)`, `DriveReference(ctx, addr)`); per-fixture assertions live in the driver (e.g., `driver.go`'s `assertResponses(t, refResps, subjResps)` for 0007a; `driver.go`'s `assertModeMatrix(t, responses)` for 0007b). Rationale: matches the 05.2 + 06.1 + 06.2 in-band precedent; no generic `LogFileExpectations` / `IterationExpectations` interface extension is needed (the fixture-specific assertion shape lives in the fixture's own driver, not the runner). **Cross-phase impact: NO** — fixture-internal. **Code anchor:** Task 21 step 4 (0007a driver.go assertion logic), Task 22 step 4 (0007b driver.go assertion logic); Task 21+22 step 6 (runner registration).

---

## Carry-forward triage

Per SPEC §10, **no carry-forward dispositions are expected for 07.1** from prior phases:

- **Phase 06.2 REVIEW.md's 11 Minors** — separate post-phase-done batch per the prior STATE.md `next-skill-scope`; do NOT interact with 07.1's scope; can run in parallel as a separate branch.
- **Phase 04 deferred items (M-X from 04 REVIEW)** — H1-protocol-specific; unrelated to filter-chain framework.
- **Phase 05 M-4 / M-10 / M-12** — H2-hardening concerns explicitly carry-forwarded to "dedicated H2-hardening sub-phase or upstream-robustness family" per 06.1's §13.2 disposition; remain deferred from 07.1.
- **Phase 06.1 REVIEW M-8 (drain-loop polling vs arbitrary `time.Sleep`)** — already prophylactically adopted by 06.2 fixture 0006's driver; 07.1 fixtures 0007a + 0007b inherit the 25ms-poll/5s-deadline pattern by default in Tasks 21 + 22 (no separate disposition needed; the pattern is now project-default).

If a NEW carry-forward surfaces at execution time (e.g., a 06.2 Minor turns out to be load-bearing for 07.1's HCM dispatch wiring), the executor invokes `superpowers:systematic-debugging` and either (a) absorbs the fix into the affected 07.1 task with a PROGRESS cross-reference, or (b) blocks 07.1 on a follow-up commit per BOOTSTRAP §5 deviation discipline.

---

## Spec-review advisory responses

The 07.1 SPEC was self-reviewed inline (per ADR-0004 fallback when subagent dispatch is unavailable in nested-agent context; review applied the four-category rubric from `superpowers:brainstorming`'s `spec-document-reviewer-prompt.md` — Completeness / Consistency / Clarity / Scope / YAGNI; status: Approved with no Issues). **No spec-review advisories carry into this PLAN** — the review identified no Issues. The two follow-up commits since the SPEC commit (`9d1ec9c` STATE.md SHA-fill + `f2dd659` spec-reviewer fixes for cors 200 OK clarification + fuzzer-count typo) addressed grep-verifiable cosmetic issues that the spec-reviewer caught after the original SPEC commit; none of those changes affect this PLAN's task structure.

---

## Execution preconditions (the executor checks at Task 1)

Before starting Task 2 (the first code-changing task), the executor MUST verify each of the following preconditions on the implementation worktree. If any precondition fails, the executor STOPS and follows the precondition's "if fails" guidance.

1. **Worktree branch.** The current branch is `phase/07.1-http-filter-framework-impl` per ADR-0003 + the per-phase-worktree convention. Verify with `git rev-parse --abbrev-ref HEAD`. If on a different branch, the executor invoked the wrong worktree — exit and start a new session per BOOTSTRAP §1.

2. **Branch base.** The branch was created from master tip after the PLAN.md commit's SHA-fill (i.e., master HEAD is the SHA-fill follow-up commit's SHA, not the TBD-bearing commit). Verify with `git log --oneline master | head -3` — expect the top three to be the PLAN SHA-fill, the PLAN commit, and the SPEC-reviewer-fixes commit (`f2dd659`) in some order. If the branch base is older, the executor missed the PLAN commit's master fast-forward — exit and start a new session.

3. **PROGRESS.md absence.** `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` does NOT exist at branch creation. Task 1 creates it. If it exists, an earlier session left state behind — exit and invoke `superpowers:systematic-debugging`.

4. **Docker daemon.** `docker version` reports both client and server (the differential harness needs the daemon for fixture 0007a's reference container; fixture 0007b is envoy-go-only and does not need the daemon, but Tasks 11–17 incidentally exercise the H2 dispatch path via h2spec which DOES need the daemon at Task 23 step 2). If the daemon is unavailable, the executor still proceeds through Tasks 1–20 (which don't need the daemon), but Task 21's gate (a) sweep + Task 23's gate (c) re-run block until the daemon is up.

5. **Go toolchain.** `go version` reports `go1.23` or newer. The `go.mod` directive is `go 1.23.0`. If older, install Go 1.23+ before proceeding.

6. **golangci-lint.** `golangci-lint version` reports `1.64.8` (per ADR-0009). If older or newer, install the pinned version.

7. **Pre-existing fixtures green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006' -v` reports all PASS. If any fixture fails, a regression was introduced before 07.1 — exit and invoke `superpowers:systematic-debugging` on the regression.

8. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0069:`. If the tail is `> ADR-0069`, a mid-PLAN-authoring ADR landed; the executor re-numbers the seven 07.1 ADRs sequentially from `tail + 1` and updates every task's ADR reference before starting Task 2. If the tail is `< ADR-0069`, an ADR was lost — exit and invoke `superpowers:systematic-debugging`.

9. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` returns `f2dd659...` or its descendant (the spec-reviewer-fixes follow-up — the current SPEC HEAD). If a newer SPEC commit lands mid-PLAN-authoring, the executor reads the new SPEC and reconciles tasks.

10. **`internal/filter/http/` is absent.** `test ! -d internal/filter/http` returns success (the directory does not yet exist). If it exists, an earlier session left state behind — exit and invoke `superpowers:systematic-debugging`.

11. **`internal/filter/hcm/` carries the four current target sites for refactor.** `grep -nE 'func \(a \*directResponseAction\) do|func \(a \*routerAction\) do|func \(r \*routerActionH2\) doH2|func \(.* \*h2DirectResponseAdapter\) WriteH2' internal/filter/hcm/` returns at least four matches. The planner verified at PLAN-write time these are at `actions.go:102` (`directResponseAction.do`), `actions.go:142` (`routerAction.do`), `actions.go:270` (`routerActionH2.doH2`), `h2dispatch.go:?` (`h2DirectResponseAdapter.WriteH2` — actual line per current HEAD; verify at execution time). If any site is missing, the upstream code drifted — exit and invoke `superpowers:systematic-debugging`.

12. **`HTTPRegistry` symbol absent.** `grep -rn 'HTTPRegistry\|httpRegistry' internal/ cmd/` returns no matches (or only matches in this phase's PLAN.md/SPEC.md if they're staged). If matches exist in `internal/` or `cmd/`, an earlier session left partial state behind — exit and invoke `superpowers:systematic-debugging`.

13. **BEHAVIOR_CONTRACT placeholders present.** `grep -nE '^## HTTP/1\.1$|^## HTTP/2$' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns the two existing top-level subsection headings (at the lines verified at PLAN-write time: 410 and 452). The Task 23 step 1 in-place edit inserts the new `## HTTP filter chain` section between `## HTTP/2` and `## TCP proxy` (currently at line 329) — verify these anchor headings exist at execution-precondition time.

14. **Reference Envoy image pull.** `docker pull envoyproxy/envoy:v1.37.2` succeeds. The image is needed for fixture 0007a's reference container at Task 21. If the pull fails (network / Docker auth), Tasks 1–20 still proceed; Task 21 blocks on this.

If all 14 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble [ADR-0070]

**Files:**
- Create: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0070)

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. ADR-0070 (Phase-07 planner-time split) lands here as the first ADR of the seven, anchored at the PROGRESS preamble — the implementation session's first commit — since the ROADMAP edit that the split formalizes already landed at master `ee45aba` (the SPEC commit), and T1 is the first opportunity to land the ADR after the SPEC commit's ROADMAP edit.

**Precondition:** worktree exists at `phase/07.1-http-filter-framework-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up.
**Artifact:** `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` (new file); `docs/envoy-go/DECISIONS.md` (ADR-0070 appended).
**Acceptance:** all 14 preconditions report green; PROGRESS.md preamble entry committed; ADR-0070 appears in `DECISIONS.md` with full Context/Decision/Consequences sections per the ADR-0001 template.
**Verification command:** `git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` returns the Task 1 commit's SHA; `grep -nE '^## ADR-0070:' docs/envoy-go/DECISIONS.md` returns one match.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/07.1-http-filter-framework-impl
git log --oneline master | head -3                                    # expect: PLAN SHA-fill, PLAN commit, SPEC-reviewer-fixes commit
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006' -v
                                                                       # expect: every fixture PASS
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
                                                                       # expect: ADR-0069:
git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md
                                                                       # expect: f2dd659... or descendant
test ! -d internal/filter/http && echo OK                              # expect: OK
grep -nE 'func \(a \*directResponseAction\) do|func \(a \*routerAction\) do|func \(r \*routerActionH2\) doH2|h2DirectResponseAdapter\) WriteH2' internal/filter/hcm/
                                                                       # expect: at least four matches
grep -rn 'HTTPRegistry\|httpRegistry' internal/ cmd/ | grep -v '_test.go' | grep -v 'PLAN\|SPEC' || echo NONE
                                                                       # expect: NONE
grep -nE '^## HTTP/1\.1$|^## HTTP/2$' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: two matches (lines 410, 452)
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`**

```markdown
# Phase 07.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1/06.2 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 14 preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0070]

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 14 preconditions per PLAN §"Execution preconditions"; phase-06.2 close confirmed present in HEAD; SPEC at <SPEC SHA>; ADR tail at 0069 (next-free 0070); internal/filter/http/ absent (the package implementation lands at Task 2+); HTTPRegistry symbol absent. Landed ADR-0070 (phase-07 planner-time split per ADR-0045's pattern).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md
<verbatim>
$ test ! -d internal/filter/http && echo OK
OK
\`\`\`
```

- [ ] **Step 3: Append ADR-0070 to `docs/envoy-go/DECISIONS.md`**

Append to the file's tail (after the last `## ADR-NNNN:` block; preserve existing content verbatim):

```markdown
## ADR-0070: Phase-07 planner-time split (07.1 + 07.2)

**Status:** Accepted — <task-execution date YYYY-MM-DD>
**Doctrine:** D-3.5 (decisions are written), D-3.6 (every phase is a green build).
**Lands-in-task:** 07.1 PLAN Task 1 (PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at master `ee45aba` per SPEC drafting).

### Context

Phase 07 (filter-chain framework, BOOTSTRAP §8 row 07) covers two structurally distinct halves: (a) the HTTP filter framework — iteration protocol + extension registry + per-route config + the `cors` real filter + `envoy_go_test` test-only probe + the router-as-terminal-filter migration — anchored under the `internal/filter/hcm/` + `internal/filter/http/` package surface; and (b) the listener-chain completion — `listener_filters[]` framework, `FilterChainMatch` fields beyond SNI, `Listener.default_filter_chain` — anchored under the `internal/listener/` package surface. The two halves share no production-code surface; they share only the BOOTSTRAP §8 row identifier.

Per BRAINSTORM §1 + parent SPEC §3, the BRAINSTORM session split phase 07 along this surface axis at brainstorm-close. The split landed in the SPEC-drafting commit (master `ee45aba`) via:
- ROADMAP row `07` flipped `planned → in-progress` with sub-phases column `07.1, 07.2`.
- Row `07.1` added as `planned` with depends-on `06`.
- Row `07.2` added as `planned` with depends-on `07.1`.

This ADR formalizes the split decision durably; the ROADMAP edit is its concrete on-disk effect.

### Decision

Phase 07 is split into two sub-phases at planner-time per ADR-0045's discipline (which documented the 05.1 + 05.2 split and the 06.1 + 06.2 split):
- **07.1 — HTTP filter framework.** Surface: `internal/filter/http/` (NEW package tree) + `internal/filter/hcm/` (refactored). Differential surface at end: fixtures 0007a (cors differential) + 0007b (iteration-probe structural). Lands the iteration protocol that BOOTSTRAP §9 HTTP-filters family depends on.
- **07.2 — Listener-chain completion.** Surface: `internal/listener/`. Differential surface at end: TBD (07.2 SPEC drafts the fixtures). Lands the listener-side filter primitives.

Ordering is 07.1-first, 07.2-second because 07.1 unblocks the BOOTSTRAP §9 HTTP-filters family (every future HTTP filter — `header_manipulation`, `fault`, `jwt_authn`, `ext_authz`, etc. — depends on the iteration protocol + extension registry shipping in 07.1) while 07.2 has no §9 dependents.

The parent ROADMAP row `07` flips `planned → in-progress` at the SPEC-drafting commit (already landed at master `ee45aba`); transitions to `done` ONLY at 07.2's phase-done commit (NOT at 07.1's phase-done) — mirroring the 05/05.1/05.2 + 06/06.1/06.2 closure pattern.

### Alternatives considered

(A) Ship phase 07 as one sub-phase. Rejected: the cumulative LoC estimate (HTTP framework + listener chain + filter set + two fixtures) is ~12000 LoC, well above the §6.1 plan-size gate's ~1500-LoC OR-leg AND would push task count past the 25-task gate.

(B) Split along a different axis (e.g., filter-set first, then framework, then listener). Rejected: the iteration protocol + extension registry + chain state machine is the load-bearing primitive every filter depends on; splitting filter-set-first would require the framework's interfaces to be defined twice (placeholder in filter-set-first; real in framework-second) — wasted work.

(C) Defer the listener-chain to a feature-family phase post-08. Rejected: BOOTSTRAP §8 row 07's "filter chain framework" canonical title covers BOTH the HTTP framework AND the listener chain; deferring the listener chain would leave the BOOTSTRAP MVP trunk's row 07 incomplete on a load-bearing primitive (listener filters are needed for BOOTSTRAP §9's network-filters family).

### Consequences

(a) The phase 07 ROADMAP row carries a `sub-phases` column listing `07.1, 07.2`; status `in-progress` until BOTH sub-phases land done.

(b) 07.1's phase-done commit flips row `07.1 → done` AND leaves row `07` at `in-progress`; 07.2's phase-done commit flips BOTH rows `07.2 → done` AND `07 → done` AT THE SAME COMMIT.

(c) The 07.1 + 07.2 SPECs are siblings under a parent master SPEC at `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md`; the parent SPEC is read-only history once drafted (mirror of the 05 + 06 parent master SPECs).

(d) The seven 07.1 ADRs (ADR-0070..ADR-0076) are 07.1-scoped; 07.2 will introduce its own ADRs at its own SPEC + PLAN time.

(e) Total task count of phases 07.1 + 07.2 is bounded: 07.1 ships at 23 tasks (this PLAN); 07.2 will draft its own task count at its own PLAN time.
```

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md docs/envoy-go/DECISIONS.md
git commit -m "phase 07.1: PROGRESS.md preamble + precondition verification [ADR-0070]"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (per the phase-02/03/04/05.1/05.2/06.1/06.2 SHA-fill convention: a follow-up tiny commit `phase 07.1: PROGRESS SHA-fill for Task 1` lands the SHA).

*Anchored: SPEC §3, §4.4 (PROGRESS lifecycle), §15 (precondition acceptance bullet), §8 (ADR-0070 anticipation).*

---

## Task 2: `internal/filter/http/types.go` + `callbacks.go` — interfaces, status enums, two-step factory pattern, callback contracts [ADR-0071]

**Files:**
- Create: `internal/filter/http/doc.go`
- Create: `internal/filter/http/types.go`
- Create: `internal/filter/http/types_test.go`
- Create: `internal/filter/http/callbacks.go`
- Create: `internal/filter/http/callbacks_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0071)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` (append Task 2 entry)

The interfaces + status enums + factory pattern + callback contracts are the foundational primitives every subsequent task in the package consumes. ADR-0071 (HTTP filter iteration protocol shape) lands at this task per the topical-co-landing decision in `## ADRs introduced by this plan` above.

**Precondition:** Task 1 done; `internal/filter/http/` does not yet exist.
**Artifact:** `internal/filter/http/{doc,types,callbacks}.go` (new); `internal/filter/http/{types,callbacks}_test.go` (new); `docs/envoy-go/DECISIONS.md` (ADR-0071 appended).
**Acceptance:** `go test ./internal/filter/http/ -count=1 -v` passes (with no chain.go yet, only types/callbacks tests exercise — the test set is small but compiles); the four interfaces (`StreamDecoderFilter`, `StreamEncoderFilter`, `DecoderFilterCallbacks`, `EncoderFilterCallbacks`) + three status enums + two-step factory match SPEC §6.1–§6.4 contract.
**Verification command:** `go test ./internal/filter/http/ -count=1 -v && grep -nE '^## ADR-0071:' docs/envoy-go/DECISIONS.md`.

- [ ] **Step 1: Create `internal/filter/http/doc.go`**

```go
// Package http provides envoy-go's HTTP filter chain framework — the iteration
// protocol, extension registry, per-stream FilterChain state machine, and
// per-route config 3-tier merge.
//
// Architecture (per ADR-0071, ADR-0072, ADR-0073):
//
//   - Filter interfaces (StreamDecoderFilter / StreamEncoderFilter) live in
//     types.go; status enums (FilterHeadersStatus / FilterDataStatus /
//     FilterTrailersStatus) and the two-step HTTPFilterFactory /
//     FilterInstanceFactory pattern live alongside.
//   - Callback contracts (DecoderFilterCallbacks / EncoderFilterCallbacks)
//     live in callbacks.go.
//   - The extension registry HTTPRegistry (Register / Lookup / Freeze) lives
//     in registry.go; freeze-after-boot invariant mirrors *stats.Registry
//     LBP-1 from ADR-0059.
//   - Per-stream state in chain.go: FilterChain owns filter instances,
//     decode/encode iteration cursors, body buffers (capped at
//     filterBufferLimitBytes = 1 << 20), async-resume signal channels, and
//     the merged per-route config map (lazy-cached on first lookup).
//   - typed_per_filter_config 3-tier merge in perroute.go: most-specific
//     override (Route > VirtualHost > RouteConfiguration); no field-merge.
//
// Concurrency invariant (per ADR-0071): single-goroutine-per-request
// iteration. The HCM dispatch goroutine is the only goroutine that drives
// chain.runDecode* / chain.runEncode*. Filter callbacks called from
// filter-spawned goroutines are signal-only (channel send to wake the
// dispatch goroutine).
//
// External dependencies: Go stdlib + google.golang.org/protobuf +
// internal/cluster (router sub-package only). NO third-party
// filter-chain-engine / filter-iteration library.
package http
```

- [ ] **Step 2: Write the failing test for status enums + interface shapes (in `types_test.go`)**

```go
package http

import (
	"net/http"
	"testing"
)

func TestFilterHeadersStatus_Values(t *testing.T) {
	if Continue == StopIteration {
		t.Fatalf("Continue and StopIteration must differ")
	}
	if int(Continue) != 0 || int(StopIteration) != 1 {
		t.Fatalf("expected Continue=0, StopIteration=1; got Continue=%d, StopIteration=%d", Continue, StopIteration)
	}
}

func TestFilterDataStatus_Values(t *testing.T) {
	if DataContinue == DataStopIterationAndBuffer || DataStopIterationAndBuffer == DataStopIterationNoBuffer {
		t.Fatalf("data status enums must be distinct")
	}
}

func TestFilterTrailersStatus_Values(t *testing.T) {
	if TrailersContinue == TrailersStopIteration {
		t.Fatalf("trailers status enums must be distinct")
	}
}

// Compile-only assertion: a test-only fake filter implements both decoder and
// encoder interfaces with the expected method set. If any method is renamed,
// this test fails to compile.
type fakeFilter struct{}

func (fakeFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus    { return Continue }
func (fakeFilter) DecodeData([]byte, bool) FilterDataStatus               { return DataContinue }
func (fakeFilter) DecodeTrailers(http.Header) FilterTrailersStatus        { return TrailersContinue }
func (fakeFilter) SetDecoderCallbacks(DecoderFilterCallbacks)             {}
func (fakeFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus    { return Continue }
func (fakeFilter) EncodeData([]byte, bool) FilterDataStatus               { return DataContinue }
func (fakeFilter) EncodeTrailers(http.Header) FilterTrailersStatus        { return TrailersContinue }
func (fakeFilter) SetEncoderCallbacks(EncoderFilterCallbacks)             {}
func (fakeFilter) OnDestroy()                                             {}

func TestFilterInterfaces_Compile(t *testing.T) {
	var _ StreamDecoderFilter = fakeFilter{}
	var _ StreamEncoderFilter = fakeFilter{}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/filter/http/ -count=1 -v`
Expected: FAIL with "undefined: Continue", "undefined: StreamDecoderFilter", etc.

- [ ] **Step 4: Implement `internal/filter/http/types.go`**

```go
package http

import (
	"net/http"

	"google.golang.org/protobuf/types/known/anypb"
)

// FilterHeadersStatus is returned by DecodeHeaders / EncodeHeaders to signal
// iteration control. Per ADR-0071 (Envoy-faithful subset; ContinueAndDontEndStream
// out of MVP).
type FilterHeadersStatus int

const (
	Continue       FilterHeadersStatus = iota // proceed to the next filter
	StopIteration                             // park; resume via cb.ContinueDecoding/ContinueEncoding
)

// FilterDataStatus is returned by DecodeData / EncodeData to signal iteration
// control + per-filter buffering. Per ADR-0071 (watermark variants out of MVP).
type FilterDataStatus int

const (
	DataContinue              FilterDataStatus = iota // proceed
	DataStopIterationAndBuffer                        // park + accumulate body chunks until end_stream
	DataStopIterationNoBuffer                         // park; no body accumulation
)

// FilterTrailersStatus is returned by DecodeTrailers / EncodeTrailers.
type FilterTrailersStatus int

const (
	TrailersContinue       FilterTrailersStatus = iota
	TrailersStopIteration                       // park; resume via cb.Continue*
)

// StreamDecoderFilter is implemented by filters that participate in the
// downstream-to-upstream (decode) iteration. A filter implements decode-only,
// encode-only, or both; the factory's return type signals which side(s).
type StreamDecoderFilter interface {
	DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
	DecodeData(data []byte, endStream bool) FilterDataStatus
	DecodeTrailers(trailers http.Header) FilterTrailersStatus
	SetDecoderCallbacks(cb DecoderFilterCallbacks)
	OnDestroy()
}

// StreamEncoderFilter is implemented by filters that participate in the
// upstream-to-downstream (encode) iteration. Encode iteration is reverse of
// decode (per ADR-0071 + SPEC §11.1 empirical pin).
type StreamEncoderFilter interface {
	EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
	EncodeData(data []byte, endStream bool) FilterDataStatus
	EncodeTrailers(trailers http.Header) FilterTrailersStatus
	SetEncoderCallbacks(cb EncoderFilterCallbacks)
	OnDestroy()
}

// HTTPFilter is the tagged-union over decoder-only / encoder-only / both. The
// factory returns this; the chain dispatches per non-nil side.
type HTTPFilter struct {
	Name    string              // filter name from http_filters[].name
	Decoder StreamDecoderFilter // nil for encoder-only filters
	Encoder StreamEncoderFilter // nil for decoder-only filters
}

// HTTPFilterFactory parses + validates typed_config once at HCM-build time and
// returns a per-request FilterInstanceFactory closure. Per ADR-0071 two-step
// factory pattern.
type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh filter instance bound to the parsed
// config. Called once per request.
type FilterInstanceFactory func() HTTPFilter

// FactoryCtx carries the registry pointer + parsed proto-helpers needed by
// per-filter parsers.
type FactoryCtx struct {
	Registry *HTTPRegistry // optional reference for filter factories that need to look up sibling filters
	// Future extensions (cluster manager, stats registry, accesslog sinks) added
	// per-family-phase as filter implementations require them.
}
```

- [ ] **Step 5: Write the failing test for callback contract shape (in `callbacks_test.go`)**

```go
package http

import (
	"net/http"
	"testing"

	"google.golang.org/protobuf/proto"
)

type fakeDecoderCB struct {
	continueCalls int
	localReplies  int
	routeCfgCalls int
}

func (c *fakeDecoderCB) ContinueDecoding()                                          { c.continueCalls++ }
func (c *fakeDecoderCB) SendLocalReply(int, string, http.Header)                    { c.localReplies++ }
func (c *fakeDecoderCB) RequestRouteConfig() proto.Message                          { c.routeCfgCalls++; return nil }
func (c *fakeDecoderCB) EncodeHeaders(http.Header, bool)                            {}
func (c *fakeDecoderCB) EncodeData([]byte, bool)                                    {}
func (c *fakeDecoderCB) EncodeTrailers(http.Header)                                 {}

func TestDecoderFilterCallbacks_Compile(t *testing.T) {
	var _ DecoderFilterCallbacks = (*fakeDecoderCB)(nil)
}

type fakeEncoderCB struct {
	continueCalls int
}

func (c *fakeEncoderCB) ContinueEncoding()                {}
func (c *fakeEncoderCB) EncodeHeaders(http.Header, bool)  {}
func (c *fakeEncoderCB) EncodeData([]byte, bool)          {}
func (c *fakeEncoderCB) EncodeTrailers(http.Header)       {}

func TestEncoderFilterCallbacks_Compile(t *testing.T) {
	var _ EncoderFilterCallbacks = (*fakeEncoderCB)(nil)
}
```

- [ ] **Step 6: Implement `internal/filter/http/callbacks.go`**

```go
package http

import (
	"net/http"

	"google.golang.org/protobuf/proto"
)

// DecoderFilterCallbacks is the framework-supplied callback shape for
// decode-side filters. Every method except RequestRouteConfig is safe to call
// from any goroutine (per ADR-0071's async-resume mechanics).
type DecoderFilterCallbacks interface {
	// ContinueDecoding wakes the dispatch goroutine if it is parked on a
	// StopIteration return. Idempotent: duplicate calls are coalesced via the
	// chain's per-stream resume channel (capacity 1, non-blocking send).
	ContinueDecoding()

	// SendLocalReply synthesizes a response that enters the encode chain at
	// filter[len-1] (per ADR-0075 + SPEC §11 #4 empirical pin). First-call-wins
	// via sync.Once on the chain; second-call-after-encode-started is a no-op
	// + log line.
	SendLocalReply(status int, body string, headers http.Header)

	// RequestRouteConfig returns the merged proto.Message for the calling
	// filter's name (Route > VirtualHost > RouteConfiguration most-specific
	// override per ADR-0073). Nil if no per-route config applies. Lazy-cached
	// on first lookup per request.
	RequestRouteConfig() proto.Message

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods for filters that synthesize responses without using
	// SendLocalReply. Rare; intended for filters like header_manipulation
	// that need to inject encode-side material from a decode-side context.
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)
}

// EncoderFilterCallbacks is the framework-supplied callback shape for
// encode-side filters.
type EncoderFilterCallbacks interface {
	// ContinueEncoding wakes the dispatch goroutine if it is parked on an
	// encode-side StopIteration return. Same coalescing discipline as
	// ContinueDecoding.
	ContinueEncoding()

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods (rare).
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)
}

// Static-assertion helpers: the proto.Message return type must compile with
// google.golang.org/protobuf/proto.
var _ proto.Message = nil
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/filter/http/ -count=1 -v`
Expected: all tests PASS.

- [ ] **Step 8: Append ADR-0071 to `docs/envoy-go/DECISIONS.md`**

Append the full ADR-0071 block per the summary in `## ADRs introduced by this plan` above. Use the ADR-0001 template structure (Status / Doctrine / Lands-in-task / Context / Decision / Alternatives / Consequences). The Decision section MUST enumerate: the four filter-interface methods (DecodeHeaders/Data/Trailers + SetDecoderCallbacks + OnDestroy; same on encoder); the three status enums with their values; the two-step factory pattern; the async-resume buffered-channel discipline; the single-goroutine-per-request iteration invariant. The Consequences section MUST name: total supersession of ADR-0040 (router-as-direct-call replaced by router-as-terminal-filter); partial supersession of ADR-0042 (lower bound stays, upper bound lifted to "non-empty; last entry must be router"); the framework's external-dependency limit (stdlib + protobuf + cors-v3-proto + internal/cluster).

- [ ] **Step 9: Append Task 2 PROGRESS entry**

```markdown
## Task 2 — internal/filter/http/types.go + callbacks.go [ADR-0071]

**Commits:** TBD — this task's commit
**Notes:** Created internal/filter/http/{doc,types,callbacks}.go + test pairs. Defined StreamDecoderFilter + StreamEncoderFilter interfaces, three status enums (FilterHeadersStatus/FilterDataStatus/FilterTrailersStatus), DecoderFilterCallbacks + EncoderFilterCallbacks interfaces, two-step HTTPFilterFactory + FilterInstanceFactory pattern. Landed ADR-0071 (HTTP filter iteration protocol shape; supersedes ADR-0040 totally; partially supersedes ADR-0042). go test ./internal/filter/http/ green; package compiles standalone (registry.go + chain.go + perroute.go land in subsequent tasks).
**Outputs:**
\`\`\`
$ go test ./internal/filter/http/ -count=1 -v
<verbatim — TestFilterHeadersStatus_Values, TestFilterDataStatus_Values, TestFilterTrailersStatus_Values, TestFilterInterfaces_Compile, TestDecoderFilterCallbacks_Compile, TestEncoderFilterCallbacks_Compile all PASS>
$ grep -nE '^## ADR-0071:' docs/envoy-go/DECISIONS.md
<verbatim — one match>
\`\`\`
```

- [ ] **Step 10: Commit**

```bash
git add internal/filter/http/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: internal/filter/http types + callbacks [ADR-0071]"
```

SHA-fill follow-up after the commit (per Refinement convention).

*Anchored: SPEC §4.1 (file inventory), §6.1 + §6.2 + §6.3 + §6.4 (interfaces / status enums / callbacks / factory), §8 (ADR-0071 anticipation), §15 acceptance bullets 2 + 3.*

---

## Task 3: `internal/filter/http/registry.go` — HTTPRegistry Register/Lookup/Freeze [ADR-0072]

**Files:**
- Create: `internal/filter/http/registry.go`
- Create: `internal/filter/http/registry_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0072)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` (append Task 3 entry)

The extension registry — the boot-time wiring point for all HTTP filter factories. Mirrors `*stats.Registry` LBP-1 from 06.1 (ADR-0059); freeze-after-boot invariant prevents late-registration races.

**Precondition:** Task 2 done; `internal/filter/http/types.go` + `callbacks.go` compile.
**Artifact:** `internal/filter/http/{registry,registry_test}.go`; `docs/envoy-go/DECISIONS.md` (ADR-0072 appended).
**Acceptance:** `go test -race ./internal/filter/http/ -count=1 -v` passes (race-clean under concurrent Lookup); post-Freeze Register panics with the SPEC §5.3 message.
**Verification command:** `go test -race ./internal/filter/http/ -run TestRegistry -count=1 -v`.

- [ ] **Step 1: Write the failing test for Register / Lookup / Freeze (in `registry_test.go`)**

```go
package http

import (
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

func dummyFactory(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) {
	return func() HTTPFilter { return HTTPFilter{} }, nil
}

func TestRegistry_RegisterLookup(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo.Filter", dummyFactory)
	if _, ok := reg.Lookup("type.googleapis.com/foo.Filter"); !ok {
		t.Fatalf("expected Lookup to find registered type_url")
	}
	if _, ok := reg.Lookup("type.googleapis.com/missing"); ok {
		t.Fatalf("expected Lookup to miss unknown type_url")
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/dup", dummyFactory)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on duplicate Register")
		}
		if !strings.Contains(r.(string), "duplicate") {
			t.Fatalf("expected panic message to mention 'duplicate'; got %q", r)
		}
	}()
	reg.Register("type.googleapis.com/dup", dummyFactory)
}

func TestRegistry_PostFreezeRegisterPanics(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo", dummyFactory)
	reg.Freeze()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on post-Freeze Register")
		}
		if !strings.Contains(r.(string), "frozen") || !strings.Contains(r.(string), "post-boot") {
			t.Fatalf("expected panic message mentioning 'frozen' + 'post-boot'; got %q", r)
		}
	}()
	reg.Register("type.googleapis.com/late", dummyFactory)
}

func TestRegistry_FreezeIdempotent(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Freeze()
	reg.Freeze() // should not panic
}

func TestRegistry_LookupAfterFreezeOK(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo", dummyFactory)
	reg.Freeze()
	if _, ok := reg.Lookup("type.googleapis.com/foo"); !ok {
		t.Fatalf("Lookup must be allowed post-Freeze")
	}
}

func TestRegistry_ConcurrentLookup_RaceClean(t *testing.T) {
	reg := NewHTTPRegistry()
	for i := 0; i < 16; i++ {
		reg.Register("type.googleapis.com/f"+string(rune('a'+i)), dummyFactory)
	}
	reg.Freeze()
	var wg sync.WaitGroup
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = reg.Lookup("type.googleapis.com/fa")
			}
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/http/ -run TestRegistry -count=1 -v`
Expected: FAIL with "undefined: NewHTTPRegistry" and "undefined: HTTPRegistry".

- [ ] **Step 3: Implement `internal/filter/http/registry.go`**

```go
package http

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// HTTPRegistry is the boot-time-populated, freeze-after-boot extension
// registry mapping typed_config type_urls to HTTPFilterFactory functions.
// Per ADR-0072: explicit threaded constructor map (no package-global init);
// freeze-after-boot invariant mirrors *stats.Registry LBP-1 from ADR-0059.
type HTTPRegistry struct {
	mu        sync.RWMutex
	byTypeURL map[string]HTTPFilterFactory
	frozen    atomic.Bool
}

// NewHTTPRegistry allocates an empty registry. Call Register for each filter
// factory at boot, then Freeze before listenerManager.New.
func NewHTTPRegistry() *HTTPRegistry {
	return &HTTPRegistry{byTypeURL: make(map[string]HTTPFilterFactory)}
}

// Register adds a filter factory under typeURL. Panics if the registry is
// frozen or if typeURL is already registered (caller bug).
func (r *HTTPRegistry) Register(typeURL string, f HTTPFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("filter: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byTypeURL[typeURL]; exists {
		panic(fmt.Sprintf("filter: duplicate Register for type_url %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

// Lookup returns the registered factory for typeURL, or false if absent.
// Safe to call from any goroutine; lock-free post-Freeze (the RWMutex's
// RLock degenerates to read-only access on a frozen map).
func (r *HTTPRegistry) Lookup(typeURL string) (HTTPFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

// Freeze marks the registry sealed; subsequent Register panics. Idempotent.
// Called once from cmd/envoy-go/main.go after all Register calls and BEFORE
// listenerManager.New.
func (r *HTTPRegistry) Freeze() {
	r.frozen.Store(true)
}

// KnownTypeURLs returns a sorted slice of registered type_urls. Used by the
// HCM parser's error messages on unknown type_url to give actionable output.
func (r *HTTPRegistry) KnownTypeURLs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byTypeURL))
	for k := range r.byTypeURL {
		out = append(out, k)
	}
	// Sort the slice for deterministic error messages.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/filter/http/ -count=1 -v`
Expected: all tests PASS, including `TestRegistry_ConcurrentLookup_RaceClean` under `-race`.

- [ ] **Step 5: Append ADR-0072 to `docs/envoy-go/DECISIONS.md`**

Append per the summary in `## ADRs introduced by this plan` above. Decision section enumerates: explicit threading (vs `init()`-based global); freeze-after-boot panic with the verbatim message; `Lookup` lock-free post-Freeze; three filters registered at boot per phase 07.1; mirror of `*stats.Registry` LBP-1 from ADR-0059.

- [ ] **Step 6: Append Task 3 PROGRESS entry + commit**

PROGRESS entry per the Task 2 shape. Commit message:

```bash
git add internal/filter/http/registry.go internal/filter/http/registry_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: internal/filter/http registry [ADR-0072]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1, §5.3 (freeze invariant), §6 (registry), §8 (ADR-0072 anticipation), §15 acceptance bullet 2 + 8.*

---

## Task 4: `internal/filter/http/perroute.go` — typed_per_filter_config 3-tier merge [ADR-0073]

**Files:**
- Create: `internal/filter/http/perroute.go`
- Create: `internal/filter/http/perroute_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0073)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` (append Task 4 entry)

`typed_per_filter_config` parsed at HCM-build time, merged at request-time on RequestRouteConfig lookup. Most-specific override (Route > VirtualHost > RouteConfiguration); no field-merge; lazy cache on first lookup per request.

**Precondition:** Task 3 done; HTTPRegistry exists.
**Artifact:** `internal/filter/http/{perroute,perroute_test}.go`; `docs/envoy-go/DECISIONS.md` (ADR-0073 appended).
**Acceptance:** `go test ./internal/filter/http/ -run TestPerRoute -count=1 -v` passes; merge precedence + unknown-filter-name error + nil-on-no-config + lazy-cache hit/miss all green.
**Verification command:** `go test ./internal/filter/http/ -run TestPerRoute -count=1 -v`.

- [ ] **Step 1: Write the failing test for parse + merge (in `perroute_test.go`)**

```go
package http

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestPerRoute_BuildAndResolve_RouteWins(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.router"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("vh-level"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("route-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{vhost: vhCfg, route: rtCfg}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0)
	if got == nil {
		t.Fatalf("expected non-nil resolve")
	}
	gotS, ok := got.(*wrapperspb.StringValue)
	if !ok || gotS.GetValue() != "route-level" {
		t.Fatalf("expected route-level wins; got %v", got)
	}
}

func TestPerRoute_BuildAndResolve_VHostFallback(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("vh-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{vhost: vhCfg, route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0).(*wrapperspb.StringValue)
	if got.GetValue() != "vh-level" {
		t.Fatalf("expected vh-level; got %s", got.GetValue())
	}
}

func TestPerRoute_BuildAndResolve_RCFallback(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc-level"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{vhost: nil, route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	got := pr.Resolve("envoy.filters.http.cors", 0).(*wrapperspb.StringValue)
	if got.GetValue() != "rc-level" {
		t.Fatalf("expected rc-level; got %s", got.GetValue())
	}
}

func TestPerRoute_BuildAndResolve_NilOnAbsent(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	pr, err := BuildPerRouteConfig(nil, []routeScope{{vhost: nil, route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	if got := pr.Resolve("envoy.filters.http.cors", 0); got != nil {
		t.Fatalf("expected nil resolve when no scope carries a config; got %v", got)
	}
}

func TestPerRoute_BuildRejectsUnknownFilterName(t *testing.T) {
	chainNames := []string{"envoy.filters.http.router"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("oops"))}
	_, err := BuildPerRouteConfig(rcCfg, nil, chainNames)
	if err == nil {
		t.Fatalf("expected error on unknown filter name")
	}
	if !strings.Contains(err.Error(), "unknown filter name") || !strings.Contains(err.Error(), "envoy.filters.http.cors") {
		t.Fatalf("expected error to mention 'unknown filter name' + the filter name; got %q", err.Error())
	}
}

func TestPerRoute_LazyCacheHitMiss(t *testing.T) {
	chainNames := []string{"envoy.filters.http.cors"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("rc"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{vhost: nil, route: nil}}, chainNames)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	a := pr.Resolve("envoy.filters.http.cors", 0)
	b := pr.Resolve("envoy.filters.http.cors", 0)
	if a != b {
		t.Fatalf("expected lazy-cache to return same proto.Message pointer on repeated lookup")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/http/ -run TestPerRoute -count=1 -v`
Expected: FAIL with "undefined: BuildPerRouteConfig" and "undefined: routeScope".

- [ ] **Step 3: Implement `internal/filter/http/perroute.go`**

```go
package http

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// routeScope carries the typed_per_filter_config maps from a single Route +
// its containing VirtualHost. The PerRouteConfig holds one routeScope per
// matched route + the RouteConfiguration-level map.
type routeScope struct {
	vhost map[string]*anypb.Any
	route map[string]*anypb.Any
}

// PerRouteConfig is the parsed-and-validated per-route config tree, built
// once at HCM-build time. Resolve performs the merge + unmarshal at
// request-time with a lazy cache.
type PerRouteConfig struct {
	rc     map[string]proto.Message     // RouteConfiguration-scope, parsed
	scopes []scopeParsed                // one per route, parsed
	mu     sync.Mutex                   // guards cache
	cache  map[cacheKey]proto.Message   // (filterName, routeIdx) → resolved merge
}

type scopeParsed struct {
	vhost map[string]proto.Message
	route map[string]proto.Message
}

type cacheKey struct {
	filterName string
	routeIdx   int
}

// BuildPerRouteConfig parses each scope's typed_per_filter_config map (Anypb
// blobs) into proto.Message values, validating that all keys reference filter
// names present in chainNames. Returns an error with the SPEC §4.4 + §13.1
// canonical message on unknown filter names.
//
// Most-specific override on Resolve: Route > VirtualHost > RouteConfiguration.
// No field-merge per ADR-0073.
func BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string) (*PerRouteConfig, error) {
	chainSet := make(map[string]struct{}, len(chainNames))
	for _, n := range chainNames {
		chainSet[n] = struct{}{}
	}
	out := &PerRouteConfig{cache: make(map[cacheKey]proto.Message)}
	parseMap := func(in map[string]*anypb.Any, location string) (map[string]proto.Message, error) {
		if in == nil {
			return nil, nil
		}
		m := make(map[string]proto.Message, len(in))
		for k, a := range in {
			if _, ok := chainSet[k]; !ok {
				return nil, fmt.Errorf("hcm: %s: typed_per_filter_config: unknown filter name %q (chain has %v)", location, k, chainNames)
			}
			msg, err := a.UnmarshalNew()
			if err != nil {
				return nil, fmt.Errorf("hcm: %s: typed_per_filter_config[%q]: unmarshal: %w", location, k, err)
			}
			m[k] = msg
		}
		return m, nil
	}
	var err error
	out.rc, err = parseMap(rcCfg, "route_config")
	if err != nil {
		return nil, err
	}
	out.scopes = make([]scopeParsed, len(scopes))
	for i, s := range scopes {
		vh, err := parseMap(s.vhost, fmt.Sprintf("route_config.virtual_hosts[%d]", i))
		if err != nil {
			return nil, err
		}
		rt, err := parseMap(s.route, fmt.Sprintf("route_config.virtual_hosts[%d].routes[%d]", i, i))
		if err != nil {
			return nil, err
		}
		out.scopes[i] = scopeParsed{vhost: vh, route: rt}
	}
	return out, nil
}

// Resolve returns the merged proto.Message for filterName at routeIdx (the
// matched-route index in the chain's per-stream state). Cache-on-first-lookup;
// returns nil if no scope carries a config for filterName.
func (p *PerRouteConfig) Resolve(filterName string, routeIdx int) proto.Message {
	if p == nil {
		return nil
	}
	key := cacheKey{filterName: filterName, routeIdx: routeIdx}
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := p.cache[key]; ok {
		return m
	}
	var msg proto.Message
	if routeIdx >= 0 && routeIdx < len(p.scopes) {
		if m, ok := p.scopes[routeIdx].route[filterName]; ok {
			msg = m
		} else if m, ok := p.scopes[routeIdx].vhost[filterName]; ok {
			msg = m
		}
	}
	if msg == nil {
		if m, ok := p.rc[filterName]; ok {
			msg = m
		}
	}
	p.cache[key] = msg
	return msg
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/filter/http/ -count=1 -v`
Expected: all tests PASS, including the six TestPerRoute_* cases.

- [ ] **Step 5: Append ADR-0073 + commit**

ADR-0073 per the summary above; PROGRESS entry per Task 2 shape. Commit:

```bash
git add internal/filter/http/perroute.go internal/filter/http/perroute_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: internal/filter/http perroute 3-tier merge [ADR-0073]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1, §5.4 + §6.3 (per-route config), §8 (ADR-0073 anticipation), §15 acceptance bullet 2.*

---

## Task 5: `internal/filter/http/chain.go` — FilterChain struct + decode-side iteration + StopIteration parking

**Files:**
- Create: `internal/filter/http/chain.go`
- Create: `internal/filter/http/chain_test.go`
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

The chain state machine is the framework's load-bearing per-stream surface. Task 5 lands the struct + decode-side iteration; subsequent Tasks 6/7/8/9 land encode-side, beginLocalReply, async-resume mechanics, and buffer overflow respectively (all on the same `chain.go` file, with each task adding tests for its sub-surface).

**Precondition:** Tasks 2–4 done; types/callbacks/registry/perroute compile.
**Artifact:** `internal/filter/http/{chain,chain_test}.go` (initial decode-side surface).
**Acceptance:** `go test -race ./internal/filter/http/ -run TestChain_Decode -count=1 -v` passes — Continue chain runs in declaration order; StopIteration parks the dispatch goroutine; resume via `decodeResumeCh` advances the cursor; ctx-cancel during pause aborts and OnDestroy fires.

- [ ] **Step 1: Write the failing test for decode-side Continue + StopIteration parking**

```go
package http

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// recordingFilter logs each callback for assertion.
type recordingFilter struct {
	name           string
	headersStatus  FilterHeadersStatus
	dataStatus     FilterDataStatus
	trailersStatus FilterTrailersStatus
	decodeHeaders  atomic.Int32
	decodeData     atomic.Int32
	decodeTrailers atomic.Int32
	encodeHeaders  atomic.Int32
	encodeData     atomic.Int32
	encodeTrailers atomic.Int32
	destroyed      atomic.Int32
	dcb            DecoderFilterCallbacks
	ecb            EncoderFilterCallbacks
}

func (f *recordingFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.decodeHeaders.Add(1)
	return f.headersStatus
}
func (f *recordingFilter) DecodeData([]byte, bool) FilterDataStatus     { f.decodeData.Add(1); return f.dataStatus }
func (f *recordingFilter) DecodeTrailers(http.Header) FilterTrailersStatus { f.decodeTrailers.Add(1); return f.trailersStatus }
func (f *recordingFilter) SetDecoderCallbacks(cb DecoderFilterCallbacks) { f.dcb = cb }
func (f *recordingFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.encodeHeaders.Add(1)
	return Continue
}
func (f *recordingFilter) EncodeData([]byte, bool) FilterDataStatus    { f.encodeData.Add(1); return DataContinue }
func (f *recordingFilter) EncodeTrailers(http.Header) FilterTrailersStatus { f.encodeTrailers.Add(1); return TrailersContinue }
func (f *recordingFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks) { f.ecb = cb }
func (f *recordingFilter) OnDestroy()                                   { f.destroyed.Add(1) }

func newChainOf(filters ...*recordingFilter) (*FilterChain, []*recordingFilter) {
	hf := make([]HTTPFilter, len(filters))
	for i, f := range filters {
		hf[i] = HTTPFilter{Name: f.name, Decoder: f, Encoder: f}
	}
	return NewFilterChain(hf, nil), filters
}

func TestChain_Decode_AllContinue(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected RunDecodeHeaders to report iteration-complete (terminated=true)")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's DecodeHeaders called once; got a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_ResumeAdvances(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.dcb.ContinueDecoding()
	}()
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after async resume")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected b's DecodeHeaders to run after resume; a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_CtxCancelAborts(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration}
	b := &recordingFilter{name: "b", headersStatus: Continue}
	chain, _ := newChainOf(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	terminated, err := chain.RunDecodeHeaders(ctx, http.Header{}, true)
	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if terminated {
		t.Fatalf("expected aborted iteration; got terminated=true")
	}
	chain.Destroy()
	if a.destroyed.Load() == 0 || b.destroyed.Load() == 0 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy after ctx-cancel; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}
}
```

- [ ] **Step 2: Implement `internal/filter/http/chain.go` (decode side only; encode + beginLocalReply + buffer + async-resume mechanics added in Tasks 6–9)**

```go
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// filterBufferLimitBytes is the per-stream body-buffer cap matching Envoy's
// default. Per ADR-0076 (lands in Task 9). Defined here at chain.go's head so
// later tasks can reference it without re-declaration.
const filterBufferLimitBytes = 1 << 20 // 1 MiB

// FilterChain is the per-stream state machine that drives iteration of HTTP
// filters. Allocated by HCM dispatch (connection.go for H1, h2dispatch.go for
// H2) at the start of each request via NewFilterChain.
//
// Concurrency invariant (per ADR-0071): the HCM dispatch goroutine is the
// only goroutine that drives RunDecode* / RunEncode* methods. Filter callbacks
// (ContinueDecoding / ContinueEncoding) are signal-only — they unblock the
// dispatch goroutine via channel send.
type FilterChain struct {
	filters []HTTPFilter
	perRoute *PerRouteConfig // optional; nil if no per-route config

	// Iteration cursors (per Decision §3.5: two int cursors).
	decodeIdx int
	encodeIdx int

	// Async-resume signal channels (capacity 1; non-blocking sends; idempotent
	// coalesce). Written from any goroutine via callback methods; read only by
	// the dispatch goroutine.
	decodeResumeCh chan struct{}
	encodeResumeCh chan struct{}

	// Body buffers (decode-side; encode-side added in Task 6/Task 9 with the
	// 413/reset overflow paths).
	decodeBuf      []byte
	decodeBufOver  bool

	// SendLocalReply guard (Task 7).
	localReplyOnce sync.Once
	localReplyDone atomic.Bool

	// Encode-side started flag (Task 7) — second SendLocalReply after this is
	// a no-op + log line.
	encodeStarted atomic.Bool

	// Per-stream destroyed-once guard.
	destroyOnce sync.Once
}

// NewFilterChain allocates a per-stream chain. filters is the chain config
// expanded by per-request factory invocation (caller supplies fresh instances).
// perRoute may be nil.
func NewFilterChain(filters []HTTPFilter, perRoute *PerRouteConfig) *FilterChain {
	c := &FilterChain{
		filters:        filters,
		perRoute:       perRoute,
		decodeResumeCh: make(chan struct{}, 1),
		encodeResumeCh: make(chan struct{}, 1),
	}
	// Wire per-filter callback structs (concrete impl tied to this chain).
	for i := range filters {
		idx := i
		if d := filters[i].Decoder; d != nil {
			d.SetDecoderCallbacks(&decoderCB{c: c, idx: idx})
		}
		if e := filters[i].Encoder; e != nil {
			e.SetEncoderCallbacks(&encoderCB{c: c, idx: idx})
		}
	}
	return c
}

// RunDecodeHeaders iterates the decode-side filters in declaration order.
// Returns (terminated=true) if iteration completed; (terminated=false, err) if
// aborted by ctx-cancel or SendLocalReply.
func (c *FilterChain) RunDecodeHeaders(ctx context.Context, headers http.Header, endStream bool) (bool, error) {
	for c.decodeIdx < len(c.filters) {
		if c.localReplyDone.Load() {
			// SendLocalReply called from a previous filter; encode chain runs in Task 7.
			return false, nil
		}
		f := c.filters[c.decodeIdx].Decoder
		if f == nil {
			c.decodeIdx++
			continue
		}
		status := f.DecodeHeaders(headers, endStream)
		switch status {
		case Continue:
			c.decodeIdx++
		case StopIteration:
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterHeadersStatus %d", c.filters[c.decodeIdx].Name, status)
		}
	}
	return true, nil
}

// parkDecode waits on decodeResumeCh, ctx.Done, or returns the appropriate
// error. Single-goroutine invariant — only the dispatch goroutine calls this.
func (c *FilterChain) parkDecode(ctx context.Context) error {
	select {
	case <-c.decodeResumeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Destroy fires OnDestroy on every filter exactly once. Safe to call multiple
// times; idempotent.
func (c *FilterChain) Destroy() {
	c.destroyOnce.Do(func() {
		for _, f := range c.filters {
			if f.Decoder != nil {
				f.Decoder.OnDestroy()
			} else if f.Encoder != nil {
				f.Encoder.OnDestroy()
			}
		}
	})
}

// decoderCB is the framework's concrete impl of DecoderFilterCallbacks for a
// specific filter index. ContinueDecoding is non-blocking; channel send is
// coalesced via the buffered-1 channel.
type decoderCB struct {
	c   *FilterChain
	idx int
}

func (d *decoderCB) ContinueDecoding() {
	select {
	case d.c.decodeResumeCh <- struct{}{}:
	default:
	}
}

func (d *decoderCB) SendLocalReply(status int, body string, headers http.Header) {
	// Implementation lands in Task 7 (beginLocalReply + first-call-wins).
	// Stubbed for Task 5 to avoid linker-error cascade; Task 7 replaces.
	_ = d.c.localReplyOnce
	d.c.localReplyDone.Store(true)
	_ = status
	_ = body
	_ = headers
}

func (d *decoderCB) RequestRouteConfig() any { return nil } // cast-friendly until perroute lookup wires in Task 5 step 3 below
func (d *decoderCB) EncodeHeaders(http.Header, bool)        {}
func (d *decoderCB) EncodeData([]byte, bool)                {}
func (d *decoderCB) EncodeTrailers(http.Header)             {}

// encoderCB is the framework's concrete impl of EncoderFilterCallbacks. Same
// non-blocking send discipline.
type encoderCB struct {
	c   *FilterChain
	idx int
}

func (e *encoderCB) ContinueEncoding() {
	select {
	case e.c.encodeResumeCh <- struct{}{}:
	default:
	}
}

func (e *encoderCB) EncodeHeaders(http.Header, bool) {}
func (e *encoderCB) EncodeData([]byte, bool)         {}
func (e *encoderCB) EncodeTrailers(http.Header)      {}

// errCancelled is returned from RunDecode* / RunEncode* when ctx was cancelled
// mid-iteration. Sentinel used by chain_test.go assertions.
var errCancelled = errors.New("chain: iteration aborted by ctx cancellation")
```

Note: `RequestRouteConfig` returns `any` here as a placeholder; the proper `proto.Message` return type lands at Task 7 step 3 alongside the perRoute lookup wiring (the lookup needs the route-match index from the HCM dispatch path, which connects in Task 13). This is a deliberate temporary divergence from the `DecoderFilterCallbacks` interface declared in Task 2; Task 7 step 3 reconciles by adding the lookup + restoring the proto.Message return type in callbacks.go's interface declaration. The intermediate state compiles because no production caller invokes RequestRouteConfig until Task 18 (cors).

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test -race ./internal/filter/http/ -run TestChain_Decode -count=1 -v`
Expected: three TestChain_Decode_* cases PASS.

- [ ] **Step 4: PROGRESS entry + commit**

```bash
git add internal/filter/http/chain.go internal/filter/http/chain_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain.go decode-side iteration + StopIteration parking"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1, §5.2 (concurrency model), §5.4 (decode-side flow), §6.5 (buffering), §15 acceptance bullet 2.*

---

## Task 6: `chain.go` — Encode-side reverse iteration

**Files:**
- Modify: `internal/filter/http/chain.go` (add `RunEncodeHeaders` / `RunEncodeData` / `RunEncodeTrailers`)
- Modify: `internal/filter/http/chain_test.go` (add encode-side tests)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Encode-side iterates filter chain in REVERSE declaration order per SPEC §11.1 empirical pin. Per filter the framework calls EncodeHeaders/Data/Trailers; a returned StopIteration parks on `encodeResumeCh`. This task does NOT yet handle the SendLocalReply trigger (Task 7) or buffer overflow (Task 9).

**Precondition:** Task 5 done.
**Artifact:** Three new methods on `*FilterChain` + their unit tests.
**Acceptance:** `TestChain_Encode_ReverseOrder` confirms encode iterates `len-1 → 0`; `TestChain_Encode_StopIteration_ResumeAdvances` confirms parking + async resume.

- [ ] **Step 1: Add failing tests in `chain_test.go`**

```go
func TestChain_Encode_ReverseOrder(t *testing.T) {
	order := make([]string, 0, 3)
	var orderMu sync.Mutex
	mk := func(name string) *recordingFilter {
		f := &recordingFilter{name: name, headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
		// hook EncodeHeaders to record order
		// (the recordingFilter's existing EncodeHeaders only counts; we wrap via a helper)
		return f
	}
	a := mk("a"); b := mk("b"); c := mk("c")
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: encodeRecorder{f: a, order: &order, mu: &orderMu}},
		{Name: "b", Decoder: b, Encoder: encodeRecorder{f: b, order: &order, mu: &orderMu}},
		{Name: "c", Decoder: c, Encoder: encodeRecorder{f: c, order: &order, mu: &orderMu}},
	}
	chain := NewFilterChain(hf, nil)
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration complete")
	}
	want := []string{"c", "b", "a"}
	if !equalSlice(order, want) {
		t.Fatalf("expected encode order %v; got %v", want, order)
	}
}

type encodeRecorder struct {
	f     *recordingFilter
	order *[]string
	mu    *sync.Mutex
}

func (e encodeRecorder) EncodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	e.mu.Lock()
	*e.order = append(*e.order, e.f.name)
	e.mu.Unlock()
	return e.f.EncodeHeaders(h, end)
}
func (e encodeRecorder) EncodeData(d []byte, end bool) FilterDataStatus { return e.f.EncodeData(d, end) }
func (e encodeRecorder) EncodeTrailers(t http.Header) FilterTrailersStatus { return e.f.EncodeTrailers(t) }
func (e encodeRecorder) SetEncoderCallbacks(cb EncoderFilterCallbacks)     { e.f.SetEncoderCallbacks(cb) }
func (e encodeRecorder) OnDestroy()                                        { e.f.OnDestroy() }

func equalSlice(a, b []string) bool {
	if len(a) != len(b) { return false }
	for i := range a { if a[i] != b[i] { return false } }
	return true
}
```

- [ ] **Step 2: Implement encode-side iteration in `chain.go`**

```go
// RunEncodeHeaders iterates the encode-side filters in REVERSE declaration
// order (per SPEC §11.1 empirical pin). Returns (terminated=true) on full
// iteration; (false, err) on ctx-cancel.
func (c *FilterChain) RunEncodeHeaders(ctx context.Context, headers http.Header, endStream bool) (bool, error) {
	c.encodeStarted.Store(true)
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeHeaders(headers, endStream)
		switch status {
		case Continue:
			c.encodeIdx--
		case StopIteration:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterHeadersStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	return true, nil
}

// RunEncodeData iterates encode-side data callbacks in reverse. Buffer overflow
// handling lands in Task 9.
func (c *FilterChain) RunEncodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeData(data, endStream)
		switch status {
		case DataContinue:
			c.encodeIdx--
		case DataStopIterationAndBuffer, DataStopIterationNoBuffer:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterDataStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	return true, nil
}

// RunEncodeTrailers iterates encode-side trailer callbacks in reverse.
func (c *FilterChain) RunEncodeTrailers(ctx context.Context, trailers http.Header) (bool, error) {
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeTrailers(trailers)
		switch status {
		case TrailersContinue:
			c.encodeIdx--
		case TrailersStopIteration:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		}
	}
	return true, nil
}

func (c *FilterChain) parkEncode(ctx context.Context) error {
	select {
	case <-c.encodeResumeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

- [ ] **Step 3: Run tests + commit**

```bash
go test -race ./internal/filter/http/ -count=1 -v
git add internal/filter/http/chain.go internal/filter/http/chain_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain.go encode-side reverse iteration"
```

SHA-fill follow-up.

*Anchored: SPEC §5.5, §6.6, §11.1 empirical pin, §15 acceptance bullet 2.*

---

## Task 7: `chain.go` — `beginLocalReply` + first-call-wins via sync.Once + RequestRouteConfig wiring [ADR-0075]

**Files:**
- Modify: `internal/filter/http/chain.go` (add `beginLocalReply`; replace decoderCB.SendLocalReply stub; restore proto.Message return on RequestRouteConfig)
- Modify: `internal/filter/http/callbacks.go` (no shape change; the interface declared in Task 2 already returns `proto.Message` — Task 7 reconciles the chain.go stub)
- Modify: `internal/filter/http/chain_test.go` (add SendLocalReply tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0075)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

`beginLocalReply` enters the encode chain at `filter[len-1]` per SPEC §11 #4 empirical pin; the FULL encode chain runs in reverse order (every encode-side filter runs, INCLUDING the calling filter's own encode side). First-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + `log.Printf("hcm: filter %q called SendLocalReply after encode-side started; ignoring", filterName)`.

**Precondition:** Tasks 5 + 6 done.
**Artifact:** `chain.go` adds `beginLocalReply`; `decoderCB.SendLocalReply` is wired; `RequestRouteConfig` returns proto.Message via the perRoute lookup at the route-index supplied by the chain's per-stream state (chain stores routeIdx — set by HCM dispatch at Task 13).

**Acceptance:** `TestChain_SendLocalReply_EntersAtLenMinus1` confirms entry at `filter[len-1]` on a synthetic 4-filter chain `[a, b, c, router]` where `b` calls SendLocalReply (`b`'s decoder is the caller; observed encode order is `router → c → b → a`); `TestChain_SendLocalReply_FirstCallWins` confirms second concurrent call is no-op; `TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs` confirms the log line.

- [ ] **Step 1: Add failing tests in `chain_test.go`** (TestChain_SendLocalReply_EntersAtLenMinus1 + TestChain_SendLocalReply_FirstCallWins + TestChain_SendLocalReply_CallingFilterEncodeRuns + TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs).

The first test exercises a 4-filter chain (`a`, `b`, `c`, `router` placeholders); `b`'s `DecodeHeaders` calls `b.dcb.SendLocalReply(418, "i am teapot", nil)`; the test asserts that ALL FOUR encode sides ran in reverse order (`router → c → b → a`) and that NO decode side past `b` ran. This is the §11 #4 empirical-pin assertion in unit-test form.

- [ ] **Step 2: Implement `beginLocalReply` in `chain.go`**

```go
// beginLocalReply synthesizes a response from a filter's SendLocalReply call.
// Per ADR-0075 + SPEC §11 #4 empirical pin: enters encode chain at filter[len-1];
// runs FULL encode chain in reverse order; first-call-wins via sync.Once;
// second-call-after-encode-started is a no-op + log.
func (c *FilterChain) beginLocalReply(ctx context.Context, callerIdx int, status int, body string, headers http.Header) {
	if c.encodeStarted.Load() {
		// Encode chain already started; second SendLocalReply is a no-op + log.
		fmt.Fprintf(c.diagLogWriter(), "hcm: filter %q called SendLocalReply after encode-side started; ignoring\n", c.filters[callerIdx].Name)
		return
	}
	c.localReplyOnce.Do(func() {
		c.localReplyDone.Store(true)
		// Merge framework-injected standard headers with user-supplied headers.
		merged := make(http.Header, len(headers)+4)
		for k, v := range headers {
			merged[k] = v
		}
		merged.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		if merged.Get("Content-Type") == "" {
			merged.Set("Content-Type", "text/plain")
		}
		// Date and Server are filled by the HCM wire-write path, NOT here.
		// Run the encode chain.
		_, _ = c.RunEncodeHeaders(ctx, merged, len(body) == 0)
		if len(body) > 0 {
			_, _ = c.RunEncodeData(ctx, []byte(body), true)
		}
		// no trailers
	})
}

// diagLogWriter returns the destination for log messages. Default is stderr;
// tests can override via SetDiagLogWriter (test-only helper not in this task).
func (c *FilterChain) diagLogWriter() io.Writer { return os.Stderr }
```

(adds `io`, `os` imports)

- [ ] **Step 3: Wire `decoderCB.SendLocalReply` to call `beginLocalReply`; restore proto.Message return on `RequestRouteConfig`**

Replace the stub from Task 5:

```go
func (d *decoderCB) SendLocalReply(status int, body string, headers http.Header) {
	d.c.beginLocalReply(d.c.ambientCtx, d.idx, status, body, headers)
}

func (d *decoderCB) RequestRouteConfig() proto.Message {
	if d.c.perRoute == nil {
		return nil
	}
	return d.c.perRoute.Resolve(d.c.filters[d.idx].Name, d.c.routeIdx)
}
```

Add to `FilterChain` struct: `routeIdx int` (set by HCM dispatch at request start) + `ambientCtx context.Context` (the request ctx, set by HCM dispatch via `chain.SetRequestCtx(ctx, routeIdx)`).

- [ ] **Step 4: Run tests + ADR-0075 + commit**

```bash
go test -race ./internal/filter/http/ -count=1 -v
git add internal/filter/http/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain.go beginLocalReply + first-call-wins [ADR-0075]"
```

SHA-fill follow-up.

*Anchored: SPEC §5.6, §11 #4, §8 (ADR-0075 anticipation), §15 acceptance bullet 2.*

---

## Task 8: `chain.go` — Async-resume mechanics + concurrent ContinueDecoding/Encoding race-tests

**Files:**
- Modify: `internal/filter/http/chain_test.go` (add concurrent-callback race tests; existing channels are already buffered-1 + non-blocking from Task 5/6)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Async-resume mechanics already exist (Task 5/6 wired the buffered-1 + non-blocking-send pattern). Task 8 explicitly tests the concurrent-callback discipline + the per-stream-goroutine-model under `-race`: N goroutines calling `ContinueDecoding` on the same chain (idempotent + coalesced); one filter's timer goroutine vs the dispatch goroutine racing on `cb.SendLocalReply`; per-request chain teardown vs in-flight `ContinueEncoding` from a slow filter (`OnDestroy` semantics).

**Precondition:** Tasks 5–7 done.
**Artifact:** Three new race-tested cases in `chain_test.go`. NO production-code change (the discipline is already in `chain.go` from Task 5/6).
**Acceptance:** `go test -race ./internal/filter/http/ -count=10 -v` passes (10× repeat to surface flakes).

- [ ] **Step 1: Add `TestChain_ConcurrentContinueDecoding_Coalesced`** — 64 goroutines per chain hammer `ContinueDecoding` while the dispatch goroutine is parked; assert exactly one resume happens (the buffered-1 channel coalesces; no panic, no goroutine leak).

- [ ] **Step 2: Add `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply`** — a filter's `DecodeHeaders` spawns a goroutine that 5ms later calls `cb.SendLocalReply(403, "", nil)`; the dispatch goroutine is mid-iteration (filter[1]); assert first-call-wins + no race + the encode chain runs.

- [ ] **Step 3: Add `TestChain_DestroyVsInFlightContinueEncoding`** — a filter's encode side calls `ContinueEncoding` after the chain has been destroyed (simulating a slow filter that loses the race); assert no panic + the channel send is silently dropped.

- [ ] **Step 4: Run + commit**

```bash
go test -race ./internal/filter/http/ -count=10 -v
git add internal/filter/http/chain_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain.go async-resume + concurrent-callback race tests"
```

SHA-fill follow-up.

*Anchored: SPEC §5.7, §14.10 (race tests), §15 acceptance bullet 2.*

---

## Task 9: `chain.go` — Buffer overflow (decode 413, encode reset) [ADR-0076]

**Files:**
- Modify: `internal/filter/http/chain.go` (add `RunDecodeData` with buffer accounting + 413 path; add encode-overflow signal)
- Modify: `internal/filter/http/chain_test.go` (add overflow tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0076)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Decode-side buffer overflow on `StopIterationAndBuffer` synthesizes a `413 Payload Too Large` local reply with the SPEC §11 #3 verbatim shape: status `413 Payload Too Large`; body 17 bytes ASCII `Payload Too Large` (no trailing newline); headers in wire order: `content-length: 17`, `content-type: text/plain`, `date: <stamp>`, `server: envoy`, `connection: close`. The 413 then flows through encode chain via `beginLocalReply`. Encode-side overflow signals to the HCM dispatch goroutine which resets the connection (H1 close, H2 RST_STREAM) — the chain returns a `errEncodeBufferOverflow` sentinel from `RunEncodeData`; the dispatch path in connection.go / h2dispatch.go handles the reset (Tasks 15, 16).

**Precondition:** Tasks 5–8 done.
**Artifact:** `RunDecodeData` + buffer overflow path + encode overflow sentinel; ADR-0076 appended.
**Acceptance:** `TestChain_DecodeData_OverflowSynthesizes413` confirms verbatim shape; `TestChain_EncodeData_OverflowReturnsSentinel` confirms the sentinel.

- [ ] **Step 1: Add failing tests** (decode 413 byte-shape + encode overflow sentinel + body-cap-respected-on-non-overflow + `connection: close` header presence + body == 17-byte literal `Payload Too Large` no trailing newline).

- [ ] **Step 2: Implement `RunDecodeData` + overflow path in `chain.go`**

```go
const localReply413BodyBytes = "Payload Too Large" // 17 bytes; no newline; per §11 #3 empirical pin

// RunDecodeData iterates decode-side data callbacks in declaration order.
// On StopIterationAndBuffer, accumulates body bytes up to filterBufferLimitBytes;
// overflow synthesizes a 413 local reply and returns (false, nil) — the chain
// state machine is now in encode mode.
func (c *FilterChain) RunDecodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
	for c.decodeIdx < len(c.filters) {
		if c.localReplyDone.Load() {
			return false, nil
		}
		f := c.filters[c.decodeIdx].Decoder
		if f == nil {
			c.decodeIdx++
			continue
		}
		status := f.DecodeData(data, endStream)
		switch status {
		case DataContinue:
			c.decodeIdx++
		case DataStopIterationAndBuffer:
			if len(c.decodeBuf)+len(data) > filterBufferLimitBytes {
				// Overflow: synthesize 413 per SPEC §11 #3 empirical pin.
				c.decodeBufOver = true
				headers := http.Header{}
				headers.Set("Connection", "close")
				c.beginLocalReply(ctx, c.decodeIdx, 413, localReply413BodyBytes, headers)
				return false, nil
			}
			c.decodeBuf = append(c.decodeBuf, data...)
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		case DataStopIterationNoBuffer:
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		}
	}
	return true, nil
}

// errEncodeBufferOverflow is returned from RunEncodeData when the encode-side
// buffer cap is exceeded. The dispatch path resets the connection (H1 close,
// H2 RST_STREAM) — handled in Tasks 15 + 16.
var errEncodeBufferOverflow = errors.New("chain: encode-side buffer overflow; resetting connection")
```

Update `RunEncodeData` (from Task 6) to track encode-side buffer accumulation and return `errEncodeBufferOverflow` when `len(c.encodeBuf)+len(data) > filterBufferLimitBytes`.

- [ ] **Step 3: Implement HCM-side `Connection: close` injection on the wire path**

The `connection: close` header in the 413 response forces the H1 conn to terminate after the local reply emits. The HCM dispatch path in connection.go (Task 15) reads this header on the synthesized response and closes the conn after writing.

- [ ] **Step 4: Run + ADR-0076 + commit**

```bash
go test -race ./internal/filter/http/ -count=1 -v
git add internal/filter/http/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain.go buffer overflow [ADR-0076]"
```

SHA-fill follow-up.

*Anchored: SPEC §6.5 (buffer cap), §11 #3 (verbatim 413 shape), §8 (ADR-0076 anticipation), §15 acceptance bullet 2.*

---

## Task 10: `internal/filter/http/fuzz_test.go` — `FuzzFilterChainParse` (ninth fuzzer)

**Files:**
- Create: `internal/filter/http/fuzz_test.go`
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

`FuzzFilterChainParse` fuzzes adversarial `http_filters[]` slices into the chain-config parser shape (the `parseFilterWithCtx` surface that lands at Task 13). Pre-Task-13 the fuzzer targets `BuildPerRouteConfig` (Task 4) on adversarial typed_config maps; post-Task-13 the fuzzer extends to chain-shape adversarial inputs. This task lands the initial perroute-targeted fuzzer; Task 13 step 4 extends it to also fuzz the parseFilterWithCtx output. 30s ADR-0018 budget.

**Precondition:** Tasks 4 + 9 done.
**Artifact:** `internal/filter/http/fuzz_test.go`.
**Acceptance:** `go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/` runs clean; no crashes; total fuzzer count post-Task-10 is 9 (matches SPEC §1 + §14.9).

- [ ] **Step 1: Write the fuzzer**

```go
package http

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func FuzzFilterChainParse(f *testing.F) {
	f.Add([]byte("envoy.filters.http.cors"), []byte("rc-payload"), []byte("vh-payload"), []byte("rt-payload"))
	f.Add([]byte(""), []byte(""), []byte(""), []byte(""))
	f.Add([]byte("\x00\x01\x02"), []byte("\xff\xfe"), nil, nil)
	f.Fuzz(func(t *testing.T, filterName, rcVal, vhVal, rtVal []byte) {
		// Build adversarial typed_per_filter_config maps.
		mk := func(b []byte) map[string]*anypb.Any {
			if b == nil { return nil }
			a, err := anypb.New(wrapperspb.String(string(b)))
			if err != nil { return nil }
			return map[string]*anypb.Any{string(filterName): a}
		}
		rcCfg := mk(rcVal)
		vh := mk(vhVal); rt := mk(rtVal)
		// Validate either against an empty chain or a chain that matches.
		chains := [][]string{{}, {string(filterName)}, {string(filterName), "envoy.filters.http.router"}}
		for _, chain := range chains {
			_, _ = BuildPerRouteConfig(rcCfg, []routeScope{{vhost: vh, route: rt}}, chain)
		}
	})
}
```

- [ ] **Step 2: Run + verify clean for 30s + commit**

```bash
go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/
git add internal/filter/http/fuzz_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: FuzzFilterChainParse (ninth fuzzer)"
```

SHA-fill follow-up.

*Anchored: SPEC §14.9 + §15 fuzzer-count bullet.*

---

## Task 11: `internal/filter/http/router/` — extract router as a real terminal filter (byte-preserved tests)

**Files:**
- Create: `internal/filter/http/router/doc.go`
- Create: `internal/filter/http/router/router.go` (migrated from `internal/filter/hcm/actions.go routerAction`)
- Create: `internal/filter/http/router/router_h2.go` (migrated from `routerActionH2`)
- Create: `internal/filter/http/router/router_test.go` (byte-preserved from `actions_test.go`)
- Create: `internal/filter/http/router/router_h2_test.go` (byte-preserved)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

The router becomes a real terminal filter implementing `StreamDecoderFilter` (decode-side: dispatches the route action — cluster dial OR direct_response synthesize — and either initiates an upstream dial-and-roundtrip OR transitions to encode chain via SendLocalReply for direct_response). Tests are byte-preserved per BRAINSTORM §6.8 — only imports + package names update; test bodies (the `t.Fatalf` calls, the gomega-style assertions, etc.) are byte-identical.

**Precondition:** Tasks 5–9 done; the chain framework is functional.
**Artifact:** Four new files in `internal/filter/http/router/`.
**Acceptance:** `go test ./internal/filter/http/router/ -count=1 -v` passes — every test from `internal/filter/hcm/actions_test.go`'s router-related test set runs green in the new package; `go vet ./internal/filter/http/router/` clean.

- [ ] **Step 1: Create `internal/filter/http/router/doc.go`**

```go
// Package router provides envoy-go's terminal HTTP filter
// (envoy.filters.http.router). The filter dispatches the resolved route
// action — either dialing an upstream cluster and proxying the response, or
// synthesizing a direct_response when the route action is direct_response.
//
// Migrated from internal/filter/hcm/actions.go routerAction + routerActionH2
// at phase 07.1 (per ADR-0071's router-as-terminal-filter discipline). Tests
// are byte-preserved per BRAINSTORM §6.8 — imports + package names update;
// test bodies unchanged.
package router
```

- [ ] **Step 2: Migrate `routerAction` → `Filter` + `New` factory in `router.go`**

The existing `routerAction.do(ctx, req, bw)` body becomes `(*Filter).DecodeHeaders(...)` + `DecodeData(...)` + `DecodeTrailers(...)` (the action body splits across the iteration callbacks). The cluster.Dial / RoundTrip / Write-response sequence stays intact; the only structural change is splitting the synchronous action.do into the iteration-callback shape:

```go
package router

import (
	"context"
	"net/http"

	"google.golang.org/protobuf/types/known/anypb"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/cluster"
	// other imports preserved from hcm/actions.go
)

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"

// New is the HTTPFilterFactory exposed at boot via filter.HTTPRegistry.Register.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	// Parse + validate the Router proto from tc (preserves the existing
	// hcm/config.go validation logic — moved here intact).
	// Returns the per-request factory.
	return func() envoyhttp.HTTPFilter {
		f := &Filter{}
		return envoyhttp.HTTPFilter{Name: "envoy.filters.http.router", Decoder: f, Encoder: f}
	}, nil
}

type Filter struct {
	dcb     envoyhttp.DecoderFilterCallbacks
	ecb     envoyhttp.EncoderFilterCallbacks
	// ... action-shape state migrated from routerAction
}

func (f *Filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Migrated from routerAction.do — initiate cluster.Dial; for endStream
	// requests immediately roundtrip and call cb.EncodeHeaders/EncodeData.
	// For requests with body, return Continue and accumulate on DecodeData.
	// ...
	return envoyhttp.Continue
}
// ... DecodeData / DecodeTrailers / EncodeHeaders/Data/Trailers / SetCallbacks / OnDestroy
```

The full migration is mechanical; the executor copies the routerAction body verbatim, replaces references to `bw *bufio.Writer` with calls to `f.dcb.EncodeHeaders(...)` + `f.dcb.EncodeData(...)`, and deletes the now-unreachable code paths (the action.do return-tuple is replaced by the iteration-callback flow).

- [ ] **Step 3: Migrate `routerActionH2` → `FilterH2` (or share the same `Filter` struct with H2-codec branching) in `router_h2.go`**

Same shape as Step 2 for the H2 path. The existing `routerActionH2.doH2` body migrates to the H2-specific portions of the `Filter`'s callback methods.

- [ ] **Step 4: Byte-preserve test files**

Copy `internal/filter/hcm/actions_test.go`'s router-related tests verbatim to `internal/filter/http/router/router_test.go`; update only `package` line + import paths. Do NOT modify test bodies.

- [ ] **Step 5: Run tests + commit**

```bash
go test ./internal/filter/http/router/ -count=1 -v
git add internal/filter/http/router/ docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: extract router as terminal filter (byte-preserved tests)"
```

Note: at this point `internal/filter/hcm/actions.go` STILL contains `routerAction` + `routerActionH2`; their deletion happens in Task 12. The duplication is intentional — Task 11 lands the new code with passing tests; Task 12 deletes the old code in a clean separate commit.

SHA-fill follow-up.

*Anchored: SPEC §4.1, §4.2 (router migration), §15 acceptance bullets 5 + 6.*

---

## Task 12: `internal/filter/hcm/actions.go` — delete `routerAction` + `routerActionH2`; `directResponseAction` STAYS

**Files:**
- Modify: `internal/filter/hcm/actions.go` (DELETE routerAction + routerActionH2; directResponseAction stays)
- Modify: `internal/filter/hcm/actions_test.go` (DELETE router tests; directResponseAction tests stay)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

After Task 11, `internal/filter/http/router/` is the canonical home for router action logic. Task 12 deletes the now-unreferenced `routerAction` + `routerActionH2` symbols + their tests from `internal/filter/hcm/actions.go`. `directResponseAction` STAYS — it remains a route-action shape decided at route-match time, synthesized by the router filter when its terminal step runs (the SPEC §4.2 invariant).

**Precondition:** Task 11 done; `internal/filter/http/router/` tests pass.
**Artifact:** `actions.go` shrinks by ~500 LoC; `actions_test.go` shrinks by ~300 LoC; only the `directResponseAction` impl + its tests survive.
**Acceptance:** `grep -nE 'routerAction|routerActionH2' internal/filter/hcm/` returns zero matches; `go test ./internal/filter/hcm/ -run TestDirectResponseAction -count=1 -v` still passes.

- [ ] **Step 1: Delete `routerAction` struct + `routerAction.do` + `routerActionH2` struct + `routerActionH2.doH2` + `routerActionH2.write502` + `routerActionH2.do` from `actions.go`**

Mechanical deletion — preserve `directResponseAction.body` / `writeH1` / `writeH2` / `do` verbatim.

- [ ] **Step 2: Delete corresponding test functions from `actions_test.go`**

Preserve `TestDirectResponseAction*` tests verbatim.

- [ ] **Step 3: Verify the package still compiles**

`go build ./internal/filter/hcm/`. If a forgotten reference exists (e.g., `connection.go` still imports `routerAction`), the compile error surfaces — those references will be replaced in Task 15 (H1 dispatch refactor) and Task 16 (H2 dispatch refactor). Task 12 is permitted to leave the package not-yet-buildable IF AND ONLY IF the executor immediately moves to Task 13/14/15/16 to restore buildability — the deletion + replacement is a single logical step split across four tasks for review-granularity.

**Recommended approach:** the executor combines Tasks 12 + 13 + 14 into a single working branch that lands as ONE commit per task, with the package fully buildable after the LAST of the four (Task 16). Inter-task `go build` failures are acceptable for the inner three tasks (12/13/14/15); only the final task 16 commit must restore buildability across the package. The PROGRESS entries for Tasks 12–15 each note "package does not yet build; restored at Task 16" — this is a deliberate split-for-review pattern.

Alternatively, the executor may bundle Tasks 12 + 13 + 14 + 15 + 16 into a single commit and write five PROGRESS entries against that one commit (per the 06.2 PLAN's allowance for related multi-file refactors when each component is independently undebuggable).

- [ ] **Step 4: Commit**

```bash
git add internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: delete routerAction + routerActionH2 (moved to internal/filter/http/router)"
```

(Note: `git commit` may fail if pre-commit hooks require buildability; in that case, batch with Task 13–16 per the recommended approach above.)

SHA-fill follow-up.

*Anchored: SPEC §4.2 (deletion), §15 acceptance bullet 5.*

---

## Task 13: `internal/filter/hcm/config.go` — `parseFilterWithCtx` accepts `*HTTPRegistry`; chain config build; per-route plumbing

**Files:**
- Modify: `internal/filter/hcm/config.go` (parseFilterWithCtx widening + http_filters[] walk + per-route plumbing)
- Modify: `internal/filter/hcm/config_test.go` (extend tests for chain validation)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

The HCM config parser walks `http_filters[]` in declaration order, calls `*HTTPRegistry.Lookup(typed_config.type_url)` for each entry, accumulates a `[]chainEntry` (filter name + per-instance factory closure) into the `Filter` struct, and parses `typed_per_filter_config` via `perroute.BuildPerRouteConfig` validating keys ⊆ chain filter names.

Validation tightening (per SPEC §1 #6 + ADR-0071's partial supersession of ADR-0042):
- Empty `http_filters[]` → `hcm: http_filters: must contain at least 1 entry (the router)`.
- Last entry not router → `hcm: http_filters: last entry must be %q (router)`.
- Duplicate filter names → `hcm: http_filters: duplicate filter name %q`.
- Unknown `typed_config.type_url` → `hcm: http_filters[i]: unknown type_url %q (registry: known are [%s])`.

**Precondition:** Tasks 11–12 done.
**Artifact:** `parseFilterWithCtx` widens; `Filter` struct gains `chainConfig` + `perRouteConfig` fields (the struct extension lives in Task 14's `filter.go` change; Task 13 adds the parser side that populates them).
**Acceptance:** Existing chain validation tests pass with adapted error messages; new tests for the four error-class paths (empty / non-router-terminal / duplicate / unknown-type_url) pass.

- [ ] **Step 1: Add failing tests** (TestParseFilterWithCtx_RejectsEmptyChain, TestParseFilterWithCtx_RejectsNonRouterTerminal, TestParseFilterWithCtx_RejectsDuplicateFilterName, TestParseFilterWithCtx_RejectsUnknownTypeURL).

- [ ] **Step 2: Update `parseFilterWithCtx` signature**

```go
// before: parseFilterWithCtx(tc *anypb.Any, ...) (*Filter, error)
// after:  parseFilterWithCtx(tc *anypb.Any, ..., httpRegistry *filter_http.HTTPRegistry) (*Filter, error)
```

Walks `http_filters[]`; for each entry calls `httpRegistry.Lookup(entry.typed_config.type_url)`; accumulates into `[]chainEntry`; runs the four validations. The existing "exactly `[router]`" rule (per ADR-0042 at config.go:224) is REPLACED by:

```go
if len(filters) == 0 {
	return fmt.Errorf("hcm: http_filters: must contain at least 1 entry (the router)")
}
last := filters[len(filters)-1]
if last.GetName() != routerFilterName || lastTypeURL(last) != routerTypeURL {
	return fmt.Errorf("hcm: http_filters: last entry must be %q (router); got %q (%s)", routerFilterName, last.GetName(), lastTypeURL(last))
}
seen := make(map[string]struct{}, len(filters))
for i, f := range filters {
	if _, dup := seen[f.GetName()]; dup {
		return fmt.Errorf("hcm: http_filters: duplicate filter name %q", f.GetName())
	}
	seen[f.GetName()] = struct{}{}
	tu := f.GetTypedConfig().GetTypeUrl()
	factory, ok := httpRegistry.Lookup(tu)
	if !ok {
		return fmt.Errorf("hcm: http_filters[%d]: unknown type_url %q (registry: known are %v)", i, tu, httpRegistry.KnownTypeURLs())
	}
	instanceFactory, err := factory(f.GetTypedConfig(), filter_http.FactoryCtx{Registry: httpRegistry})
	if err != nil {
		return fmt.Errorf("hcm: http_filters[%d]: factory: %w", i, err)
	}
	chainConfig = append(chainConfig, chainEntry{name: f.GetName(), factory: instanceFactory})
}
```

- [ ] **Step 3: Wire perRoute build into the parse path**

After the chain config is built, parse the `typed_per_filter_config` maps from RouteConfiguration / VirtualHost / Route via `filter_http.BuildPerRouteConfig(rcMap, scopes, chainNames)`; store on `Filter.perRouteConfig`. On error, return at the parse boundary.

- [ ] **Step 4: Extend `internal/filter/http/fuzz_test.go` `FuzzFilterChainParse` to also fuzz the chain-shape parser**

Per Task 10's note, the initial fuzzer targets `BuildPerRouteConfig`; Task 13 extends it to fuzz `parseFilterWithCtx` directly via a thin shim. Add a second fuzz seed body that constructs adversarial `[]*envoy_extensions_filters_network_http_connection_manager_v3.HttpFilter` slices (varied type_urls, malformed `typed_config` Anypb blobs, oversized counts, non-ASCII filter names) and invokes the chain-config parser on them. The chain-config parser must not panic on any input — it returns a structured error per the four canonical error classes (empty / non-router-terminal / duplicate / unknown type_url). The fuzzer's invariant: `parseFilterWithCtx(...)` either returns a `*Filter` or an error; never panics.

```go
func FuzzFilterChainParse_ChainShape(f *testing.F) {
	f.Add([]byte("filter-a"), []byte("filter-b"), uint8(2))
	f.Fuzz(func(t *testing.T, name1, name2 []byte, count uint8) {
		// Build adversarial http_filters[] entries; invoke parseFilterWithCtx
		// via a test-only shim that constructs the minimal HCM proto wrapper.
		// Assert no panic; error is fine; *Filter is fine.
		_ = name1; _ = name2; _ = count
		// (full implementation: build []*HttpFilter with varied type_urls + nil typed_configs;
		// call parseFilterWithCtx via test-only shim; recover panic = test failure.)
	})
}
```

The two-fuzzer count is logically a single `FuzzFilterChainParse` target with two seed corpora; both run under the single 30s budget per ADR-0018 (the budget is per-target, not per-seed).

- [ ] **Step 5: Run tests + commit**

```bash
go test ./internal/filter/hcm/ -count=1 -v
go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/    # extended fuzzer also runs clean
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go internal/filter/http/fuzz_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: hcm/config parses http_filters[] via HTTPRegistry; FuzzFilterChainParse extended to chain shape"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (config.go modifications), §1 #6 (validation tightening), §14.9 (fuzzer extension), §15 acceptance bullets 6 + 7.*

---

## Task 14: `internal/filter/hcm/filter.go` — constructor signature widening + Filter struct extension

**Files:**
- Modify: `internal/filter/hcm/filter.go` (delete legacy constructors; introduce `NewFilterWithCtxAndSinksAndRegistry`; extend Filter struct with `chainConfig` + `perRouteConfig`)
- Modify: All call sites in `internal/filter/hcm/{config,filter,connection,h2dispatch}_test.go` + `internal/listener/manager.go` + `cmd/envoy-go/main.go` (mechanical update — one line per site)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Per Decision §3.4: DELETE the legacy `NewFilter` / `NewFilterWithCtx` / `NewFilterWithCtxAndSinks` constructors; the new `NewFilterWithCtxAndSinksAndRegistry` is the SOLE entry point. Pre-existing call sites update mechanically (one line each, adding the `*filter_http.HTTPRegistry` parameter). Update all test bootstraps that currently use the legacy constructors.

**Precondition:** Task 13 done.
**Artifact:** `filter.go` constructor signature change + struct extension; ~15 call site updates in tests.
**Acceptance:** `go build ./...` clean (assuming Tasks 15 + 16 also land — see Task 12 step 3); `go test ./internal/filter/hcm/ -count=1 -v` passes.

- [ ] **Step 1: Update `Filter` struct in `config.go` (NEW chainConfig + perRouteConfig fields)**

```go
type chainEntry struct {
	name    string
	factory filter_http.FilterInstanceFactory
}

type Filter struct {
	// ... existing fields preserved
	chainConfig    []chainEntry
	perRouteConfig *filter_http.PerRouteConfig
}
```

- [ ] **Step 2: Replace constructor in `filter.go`**

```go
// Sole constructor for HCM. All legacy NewFilter* are deleted.
func NewFilterWithCtxAndSinksAndRegistry(
	tc *anypb.Any,
	clusters *cluster.Manager,
	lc ListenerCtx,
	registry *stats.Registry,
	accessLogSinks []accesslog.Sink,
	httpRegistry *filter_http.HTTPRegistry,
) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc, registry, accessLogSinks, httpRegistry)
}
```

The four legacy constructors (`NewFilter`, `NewFilterWithCtx`, `NewFilterWithCtxAndSinks`) are REMOVED.

- [ ] **Step 3: Sweep call sites mechanically**

The list of call sites (verified at PLAN-write time):
- `cmd/envoy-go/main.go` — one site (the boot wiring); updated in Task 20 alongside the registry-population
- `internal/listener/manager.go` — one site (HCM-construction closure); updated in this task with the `*filter_http.HTTPRegistry` parameter threading
- `internal/filter/hcm/config_test.go` / `filter_test.go` / `connection_test.go` / `h2dispatch_test.go` / `actions_test.go` / `accesslog_emit_test.go` — ~15 test sites; each updates with `nil` or `filter_http.NewHTTPRegistry()` (immediately frozen) for the new parameter

For each test site that doesn't exercise the registry, pass an empty-frozen `*HTTPRegistry`:
```go
emptyReg := filter_http.NewHTTPRegistry()
// register the minimum (router) so the chain validates as non-empty terminal-router
emptyReg.Register(router.TypeURL, router.New)
emptyReg.Freeze()
```

- [ ] **Step 4: Run tests + commit**

```bash
go build ./...
go test ./internal/filter/hcm/ -count=1 -v
git add internal/filter/hcm/ internal/listener/manager.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: hcm constructor widens to take *HTTPRegistry; legacy constructors deleted"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (constructor changes + Filter struct), Decision §3.4, §15 acceptance bullet 6.*

---

## Task 15: `internal/filter/hcm/connection.go` — H1 dispatch runs FilterChain

**Files:**
- Modify: `internal/filter/hcm/connection.go` (alloc per-request `*FilterChain`; run decode chain; on terminal completion run encode chain; access-log emit triggers from chain-completion)
- Modify: `internal/filter/hcm/connection_test.go` (extend tests; preserve existing assertions)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

H1 dispatch path becomes: parse request → alloc `*FilterChain` from `f.chainConfig` (one fresh filter instance per chainEntry) → `chain.SetRequestCtx(ctx, routeIdx)` → `chain.RunDecodeHeaders` → `chain.RunDecodeData` (if body) → `chain.RunDecodeTrailers` (if trailers) → terminal router filter performs upstream dial / direct_response synthesize → `chain.RunEncodeHeaders` → `chain.RunEncodeData` → wire-write through bufio.Writer → access-log emit fires from chain-completion (Decision §3.1).

**Precondition:** Tasks 11–14 done.
**Artifact:** `dispatchRequest` is rewritten to drive the chain; access-log emit-deferral hook moves from `actions.go` action sites to chain-completion hook.
**Acceptance:** Pre-existing fixtures `0003-http11-routing` + `0006-access-log` remain green (gate (b)); new chain-mediated dispatch produces unchanged wire output.

- [ ] **Step 1: Add `Filter.dispatchRequest` rewrite**

Pseudocode:

```go
func (f *Filter) dispatchRequest(ctx context.Context, req *http.Request, w *bufio.Writer) error {
	// Match route (existing match logic from route.go).
	matched, routeIdx, err := f.matchRoute(req)
	if err != nil {
		return err
	}
	// Alloc per-request chain.
	chainHF := make([]filter_http.HTTPFilter, len(f.chainConfig))
	for i, e := range f.chainConfig {
		chainHF[i] = e.factory()
	}
	chain := filter_http.NewFilterChain(chainHF, f.perRouteConfig)
	chain.SetRequestCtx(ctx, routeIdx)
	defer chain.Destroy()
	// Plumb the resolved direct_response (if applicable) into the router filter via the per-request factory ctx.
	// (the router filter's per-request state holds the matched route's action — direct_response or cluster-dial.)
	_ = matched

	startTime := time.Now()
	// Decode side.
	if _, err := chain.RunDecodeHeaders(ctx, req.Header, req.Body == nil || req.Body == http.NoBody); err != nil {
		return err
	}
	if req.Body != nil && req.Body != http.NoBody {
		// Stream body chunks into chain.RunDecodeData.
		buf := make([]byte, 32*1024)
		for {
			n, rerr := req.Body.Read(buf)
			if n > 0 {
				if _, err := chain.RunDecodeData(ctx, buf[:n], rerr != nil); err != nil {
					return err
				}
			}
			if rerr != nil {
				break
			}
		}
	}
	if len(req.Trailer) > 0 {
		if _, err := chain.RunDecodeTrailers(ctx, req.Trailer); err != nil {
			return err
		}
	}
	// Encode side is driven by the router filter's terminal step (which calls
	// chain.RunEncodeHeaders / RunEncodeData / RunEncodeTrailers via its
	// EncoderFilterCallbacks). The chain emits to w via the wire-write
	// callback set on connection setup.
	// On chain-completion: emit access log per Decision §3.1.
	bytesSent := chain.WireBytesWritten()
	statusCode := chain.LastStatusCode()
	picked := chain.LastPickedEndpoint() // surfaced from router filter via callback
	f.emitAccessLog(req, statusCode, bytesSent, picked, startTime)
	// On encode-side overflow: chain returned errEncodeBufferOverflow; close conn.
	if chain.EncodeOverflowed() {
		// caller closes the connection
		return errConnReset
	}
	// On 413 with `Connection: close`: emit then close.
	if hdrs := chain.LastResponseHeaders(); hdrs.Get("Connection") == "close" {
		return errConnClose
	}
	return nil
}
```

The chain owns the wire-write callback; `connection.go`'s `runConnection` loop wires `w *bufio.Writer` into the chain at construction so encode-side `RunEncodeHeaders` / `RunEncodeData` flushes through.

- [ ] **Step 2: Add the access-log emit triggers from chain-completion (Decision §3.1)**

The previous emit-deferrals at `directResponseAction.do` + `routerAction.do` are removed from `actions.go` (the routerAction is gone; directResponseAction is now invoked from the router filter, NOT from actions.go directly). The single new emit site lives in `dispatchRequest` per the pseudocode above.

- [ ] **Step 3: Update `connection_test.go`** — preserve existing assertions; update bootstrap helpers to thread an empty-frozen `*HTTPRegistry`.

- [ ] **Step 4: Run tests + commit**

```bash
go test ./internal/filter/hcm/ -count=1 -v
go test ./test/differential/ -run 'Test.*0003|Test.*0006' -count=1 -v   # pre-existing fixtures still green
git add internal/filter/hcm/connection.go internal/filter/hcm/connection_test.go internal/filter/hcm/actions.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: H1 dispatch runs FilterChain; accesslog emit from chain-completion"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2, §5.4 (decode-side flow), Decision §3.1, §15 acceptance bullet 6.*

---

## Task 16: `internal/filter/hcm/h2dispatch.go` — H2 dispatch runs FilterChain

**Files:**
- Modify: `internal/filter/hcm/h2dispatch.go` (same shape change as Task 15 for H2 codec path)
- Modify: `internal/filter/hcm/h2dispatch_test.go`
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Same architecture as Task 15 but on the H2 codec path. The H2 stream's `onHeaders` allocates the chain, runs decode side, terminal router emits encode side via the `streamWriter`. Access-log emit fires from chain-completion. The H2 ctx-cancel sentinel discipline (06.2) is preserved: the chain's final-status method returns 0 on cancellation; `emitAccessLogH2` skips submission per the SPEC §2.1 last bullet (06.2 inheritance).

**Precondition:** Tasks 11–15 done.
**Artifact:** H2 dispatch path uses the chain.
**Acceptance:** Pre-existing fixtures `0004-h2-routing` (06.2's H2 fixture) + `0006-access-log` H2 unit tests remain green; h2spec at 53/53 PASS (Task 23 step 2 re-runs).

- [ ] **Step 1: Rewrite the H2 dispatch entry point** (the per-stream `onHeaders` handler) to allocate `*FilterChain` and drive iteration on the H2 stream's headers/data/trailers.

- [ ] **Step 2: Wire encode chain to the H2 streamWriter** — the encode-side iteration writes through the H2 codec instead of bufio.Writer.

- [ ] **Step 3: Preserve H2 ctx-cancel sentinel** — `chain.LastStatusCode` returns 0 on cancellation; `emitAccessLogH2` skips submission.

- [ ] **Step 4: Update tests + commit**

```bash
go test ./internal/filter/hcm/ -count=1 -v
go test ./test/differential/ -run 'Test.*0004' -count=1 -v
git add internal/filter/hcm/h2dispatch.go internal/filter/hcm/h2dispatch_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: H2 dispatch runs FilterChain"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2, §5.4 + §5.5 (decode + encode flows; H2 path), §15 acceptance bullet 6.*

---

## Task 17: `internal/filter/hcm/chain_integration_test.go` — H1 + H2 happy paths

**Files:**
- Create: `internal/filter/hcm/chain_integration_test.go`
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

End-to-end-but-in-process tests of "framework runs filters then router" on both H1 and H2. Two recording filters (`a`, `b` from Task 5's `recordingFilter` helper, exported here as a test fixture) wired ahead of the router; a route_config that direct_response synthesizes `200 OK\n`; assert `a.DecodeHeaders` + `b.DecodeHeaders` both run before the response is generated; assert `b.EncodeHeaders` + `a.EncodeHeaders` both run after (reverse order). Same for H2 path.

**Precondition:** Tasks 15 + 16 done.
**Artifact:** New integration test file.
**Acceptance:** `go test ./internal/filter/hcm/ -run TestChainIntegration -count=1 -v` passes for both H1 + H2 cases.

- [ ] **Step 1: Write the H1 happy path test**
- [ ] **Step 2: Write the H2 happy path test**
- [ ] **Step 3: Run + commit**

```bash
go test ./internal/filter/hcm/ -run TestChainIntegration -count=1 -v
git add internal/filter/hcm/chain_integration_test.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: chain_integration_test for H1 + H2 happy paths"
```

SHA-fill follow-up.

*Anchored: SPEC §14.5 + §15 acceptance bullet 6.*

---

## Task 18: `internal/filter/http/cors/` — real Envoy filter `envoy.filters.http.cors` [ADR-0074]

**Files:**
- Create: `internal/filter/http/cors/doc.go`
- Create: `internal/filter/http/cors/cors.go`
- Create: `internal/filter/http/cors/cors_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0074)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

The cors filter's decode-side detects preflight (`OPTIONS` + `Origin` + `Access-Control-Request-Method`); on allowed origin it `SendLocalReply(200, "", corsHeaders)` synthesizing the preflight response with the verbatim header set pinned in SPEC §11.2 (six headers in fixed order: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-allow-methods`, `access-control-allow-headers`, `access-control-max-age`, `access-control-expose-headers`); on disallowed origin the filter does NOT inject a 4xx (passes through to router which 405s). Encode-side adds three CORS response headers on the upstream response when the request had an `Origin` matching the allow-list (`access-control-allow-origin`, `access-control-allow-credentials`, `access-control-expose-headers`); pass-through otherwise.

**Precondition:** Tasks 11–17 done.
**Artifact:** `internal/filter/http/cors/{doc,cors,cors_test}.go`.
**Acceptance:** `go test ./internal/filter/http/cors/ -count=1 -v` passes — preflight allowed → 200 + six headers in the §11.2 verbatim order; preflight disallowed → passthrough (cors does NOT inject 4xx); actual request allowed-origin → encodeHeaders adds three headers; actual request no-origin → no-op; per-route override (different allowed origins on different routes); the type_url + factory round-trip through registry.

- [ ] **Step 1: Write the failing tests** (TestCors_Preflight_AllowedOriginEmits200WithSixHeaders, TestCors_Preflight_DisallowedOriginPassesThrough, TestCors_ActualRequest_AllowedOriginAddsThreeHeaders, TestCors_ActualRequest_NoOriginIsNoOp, TestCors_PerRouteOverride, TestCors_FactoryRoundTrip).

- [ ] **Step 2: Implement `cors.go`**

```go
package cors

import (
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/anypb"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"

// New is the HTTPFilterFactory exposed at boot.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	// The filter-level Cors message has no fields used in 07.1; everything
	// behavioral is per-route via CorsPolicy. Validate tc unmarshals to *Cors.
	if tc != nil {
		var c corsv3.Cors
		if err := tc.UnmarshalTo(&c); err != nil {
			return nil, err
		}
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{}
		return envoyhttp.HTTPFilter{Name: "envoy.filters.http.cors", Decoder: f, Encoder: f}
	}, nil
}

type filter struct {
	dcb              envoyhttp.DecoderFilterCallbacks
	ecb              envoyhttp.EncoderFilterCallbacks
	originAllowed    bool       // captured during DecodeHeaders for encode-side use
	matchedOrigin    string
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }
func (f *filter) OnDestroy()                                              {}

func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	policy := f.routePolicy()
	origin := headers.Get("Origin")
	method := strings.ToUpper(strings.TrimSpace(getMethod(headers)))
	if origin == "" {
		return envoyhttp.Continue // no-op for non-CORS requests
	}
	f.originAllowed = originAllowedByPolicy(origin, policy)
	if f.originAllowed {
		f.matchedOrigin = origin
	}
	isPreflight := method == "OPTIONS" && headers.Get("Access-Control-Request-Method") != ""
	if isPreflight {
		if !f.originAllowed {
			// Pass through to router per §11 #2 empirical pin (router 405s).
			return envoyhttp.Continue
		}
		// Allowed preflight: synthesize 200 + six CORS headers in §11.2 verbatim order.
		ph := http.Header{}
		ph.Set("Access-Control-Allow-Origin", origin)
		if policy.GetAllowCredentials().GetValue() {
			ph.Set("Access-Control-Allow-Credentials", "true")
		}
		if methods := policy.GetAllowMethods(); methods != "" {
			ph.Set("Access-Control-Allow-Methods", methods)
		}
		if hdrs := policy.GetAllowHeaders(); hdrs != "" {
			ph.Set("Access-Control-Allow-Headers", hdrs)
		}
		if maxAge := policy.GetMaxAge(); maxAge != "" {
			ph.Set("Access-Control-Max-Age", maxAge)
		}
		if expose := policy.GetExposeHeaders(); expose != "" {
			ph.Set("Access-Control-Expose-Headers", expose)
		}
		f.dcb.SendLocalReply(200, "", ph)
		return envoyhttp.StopIteration
	}
	return envoyhttp.Continue
}

func (f *filter) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	if !f.originAllowed {
		return envoyhttp.Continue // no-op
	}
	policy := f.routePolicy()
	headers.Set("Access-Control-Allow-Origin", f.matchedOrigin)
	if policy.GetAllowCredentials().GetValue() {
		headers.Set("Access-Control-Allow-Credentials", "true")
	}
	if expose := policy.GetExposeHeaders(); expose != "" {
		headers.Set("Access-Control-Expose-Headers", expose)
	}
	return envoyhttp.Continue
}

// DecodeData / DecodeTrailers / EncodeData / EncodeTrailers are pass-through.
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus      { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus { return envoyhttp.TrailersContinue }
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus      { return envoyhttp.DataContinue }
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus { return envoyhttp.TrailersContinue }

func (f *filter) routePolicy() *corsv3.CorsPolicy {
	cfg := f.dcb.RequestRouteConfig()
	if cfg == nil {
		return &corsv3.CorsPolicy{}
	}
	p, _ := cfg.(*corsv3.CorsPolicy)
	return p
}

func originAllowedByPolicy(origin string, p *corsv3.CorsPolicy) bool {
	for _, m := range p.GetAllowOriginStringMatch() {
		if m.GetExact() == origin {
			return true
		}
		if pre := m.GetPrefix(); pre != "" && strings.HasPrefix(origin, pre) {
			return true
		}
		if suf := m.GetSuffix(); suf != "" && strings.HasSuffix(origin, suf) {
			return true
		}
	}
	return false
}

func getMethod(headers http.Header) string {
	// HCM populates :method via the request line / H2 pseudo-header. For the
	// purposes of cors.DecodeHeaders, the dispatch path injects the method as
	// `:method` H2-style or via headers.Get("X-Method") on H1. Both H1 and H2
	// dispatch in HCM ensure the chain has access to the method via headers.
	if m := headers.Get(":method"); m != "" {
		return m
	}
	return headers.Get("X-Method") // H1 dispatch helper inserts :method-equivalent
}

var _ = strconv.Itoa // reserved for future use
```

Note: the `getMethod` helper above presumes the HCM H1 dispatch path injects the method into headers; this convention is documented in `internal/filter/hcm/connection.go`'s comments at Task 15.

- [ ] **Step 3: Run tests + ADR-0074 + commit**

```bash
go test ./internal/filter/http/cors/ -count=1 -v
git add internal/filter/http/cors/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: cors filter [ADR-0074]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1, §11.2 empirical pin (six-header verbatim order), §8 (ADR-0074 anticipation), §15 acceptance bullets 3 + 4.*

---

## Task 19: `internal/filter/http/envoygotest/` — test-only probe filter

**Files:**
- Create: `internal/filter/http/envoygotest/doc.go`
- Create: `internal/filter/http/envoygotest/filter.go`
- Create: `internal/filter/http/envoygotest/filter_test.go`
- Create: `internal/filter/http/envoygotest/proto/envoygotest.pb.go` (hand-rolled minimal proto)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

`envoy.filters.http.envoy_go_test` — test-only probe filter. Per-request mode dispatch on `x-envoy-go-test-mode` header covering 8 iteration-state modes. Per-route `count` config echoed into `x-envoy-go-test-route-count: N` response header on encodeHeaders. Hand-rolled minimal proto (envoy-go-only; not in upstream go-control-plane). Mode-dispatch via explicit-switch (Decision §3.7).

**Precondition:** Tasks 11–18 done.
**Artifact:** Four new files under `envoygotest/` + its `proto/` subdir.
**Acceptance:** `go test ./internal/filter/http/envoygotest/ -count=1 -v` passes — eight modes, per-mode behavior asserted; per-route count config → response header injection; type_url + factory round-trip through registry.

- [ ] **Step 1: Write `proto/envoygotest.pb.go`** (hand-rolled minimal proto Message; two fields: `mode_default` string + `count` int32; implement `proto.Message` interface manually OR use `protogen` to generate a minimal stub; the hand-rolled approach is preferred per SPEC §4.1 since the proto schema is envoy-go-only).

- [ ] **Step 2: Write the failing tests** for the 8-mode matrix (one test per mode, plus a TestEnvoyGoTest_PerRouteCountConfig + TestEnvoyGoTest_FactoryRoundTrip).

- [ ] **Step 3: Implement `filter.go` with explicit-switch mode dispatch** (per Decision §3.7). The 8 modes are `continue`, `stop-and-resume-headers`, `stop-and-buffer-data`, `local-reply-decode`, `local-reply-decode-data`, `modify-encode-headers`, `modify-encode-data`, `stop-trailers`.

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	mode := headers.Get("x-envoy-go-test-mode")
	if mode == "" {
		mode = f.modeDefault
	}
	switch mode {
	case "continue":
		return envoyhttp.Continue
	case "stop-and-resume-headers":
		go func() {
			time.Sleep(10 * time.Millisecond)
			f.dcb.ContinueDecoding()
		}()
		return envoyhttp.StopIteration
	case "stop-and-buffer-data":
		return envoyhttp.Continue // body handling on DecodeData
	case "local-reply-decode":
		f.dcb.SendLocalReply(418, "i am a teapot\n", nil)
		return envoyhttp.StopIteration
	case "local-reply-decode-data":
		return envoyhttp.Continue // body handling on DecodeData
	case "modify-encode-headers":
		return envoyhttp.Continue
	case "modify-encode-data":
		return envoyhttp.Continue
	case "stop-trailers":
		return envoyhttp.Continue // trailers handling on DecodeTrailers
	default:
		return envoyhttp.Continue
	}
}

// ... DecodeData / DecodeTrailers / EncodeHeaders / EncodeData with the corresponding mode-branch logic
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -race ./internal/filter/http/envoygotest/ -count=1 -v
git add internal/filter/http/envoygotest/ docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: envoygotest probe filter (8-mode iteration coverage)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 + §7.3 (8-mode matrix), Decision §3.7, §15 acceptance bullets 3 + 4.*

---

## Task 20: `cmd/envoy-go/main.go` + `internal/listener/manager.go` + `internal/bootstrap/bootstrap.go` — boot wiring + cors v3 blank import

**Files:**
- Modify: `cmd/envoy-go/main.go` (alloc *HTTPRegistry; Register router/cors/envoygotest; Freeze; thread into listenerManager.New)
- Modify: `internal/listener/manager.go` (already updated in Task 14 to thread the registry; verify the threading)
- Modify: `internal/bootstrap/bootstrap.go` (blank-import `cors/v3` proto so protojson round-trips fixture bootstraps)
- Modify: `internal/filter/doc.go` (REWRITE: framework overview pointing to internal/filter/http/ + internal/filter/hcm/)
- Modify: `cmd/envoy-go/main_test.go` + `internal/listener/manager_test.go` + `internal/bootstrap/bootstrap_test.go` (extend tests as needed)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

Boot wiring: at process start, allocate the registry, register the three filter factories, freeze, then thread through to `listenerManager.New(...)` which threads to `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`. The `cors/v3` blank import in `bootstrap.go` ensures `protojson` can round-trip 07.1 fixture bootstraps that contain `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{...}` entries.

**Precondition:** Tasks 11–19 done.
**Artifact:** Five files modified; one rewritten (`internal/filter/doc.go`).
**Acceptance:** `go build ./...` clean; `go test ./cmd/envoy-go/ ./internal/listener/ ./internal/bootstrap/ -count=1 -v` passes; pre-existing fixtures `0000-0006` remain green.

- [ ] **Step 1: Update `cmd/envoy-go/main.go` boot wiring**

```go
import (
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
)

func main() {
	// ... existing setup
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Register(cors.TypeURL, cors.New)
	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
	httpReg.Freeze()
	// thread httpReg into listenerManager.New(...)
	// ... existing main flow continues
}
```

- [ ] **Step 2: Verify `internal/listener/manager.go` threads the registry** (already done at Task 14 step 2; this step verifies the test bootstraps were correctly updated).

- [ ] **Step 3: Add cors/v3 blank import to `bootstrap.go`**

```go
import (
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
)
```

- [ ] **Step 4: Rewrite `internal/filter/doc.go`**

Replace the phase-00 placeholder with the actual architectural overview (per SPEC §4.2):

```go
// Package filter is the parent package for envoy-go's filter implementations.
// HTTP-side framework + filter implementations live under filter/http/; the
// HCM (HTTP connection manager network filter) lives under filter/hcm/; the
// TCP-proxy network filter lives under filter/tcpproxy/.
//
// The HTTP filter chain framework (introduced by phase 07.1 — ADR-0071)
// provides StreamDecoderFilter / StreamEncoderFilter interfaces, a
// freeze-after-boot extension registry (ADR-0072), a typed_per_filter_config
// 3-tier merge (ADR-0073), the per-stream FilterChain state machine with
// async-resume + body buffering (ADR-0076 buffer cap; ADR-0075 sendLocalReply
// semantics), and a starter filter set: cors (real Envoy filter; ADR-0074),
// envoygotest (test-only probe; ADR-0074), router (terminal filter; ADR-0071's
// total supersession of ADR-0040). See filter/http/doc.go for the package
// overview.
package filter
```

- [ ] **Step 5: Run tests + commit**

```bash
go build ./...
go test ./cmd/envoy-go/ ./internal/listener/ ./internal/bootstrap/ -count=1 -v
go test ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006' -count=1 -v
git add cmd/envoy-go/main.go internal/listener/manager.go internal/bootstrap/bootstrap.go internal/filter/doc.go ...
git commit -m "phase 07.1: boot wiring (HTTPRegistry alloc + freeze) + cors v3 blank-import"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (boot wiring), §5.3 (freeze invariant), §15 acceptance bullets 7 + 8.*

---

## Task 21: Differential fixture `test/differential/0007a-cors/`

**Files:**
- Create: `test/differential/0007a-cors/envoy-go.yaml`
- Create: `test/differential/0007a-cors/envoy.yaml`
- Create: `test/differential/0007a-cors/expectations.yaml`
- Create: `test/differential/0007a-cors/README.md`
- Create: `test/differential/0007a-cors/driver/driver.go`
- Create: `test/differential/0007a-cors/driver/driver_test.go`
- Create: `test/differential/0007a-cors/backends/main.go`
- Modify: `test/differential/runner.go` (blank-import the new driver; register `RequiresReference: true`)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

The cors differential fixture: 2 routes (`/permissive`, `/strict`) with differing per-route cors policies; 4 sequential requests across both proxies (`OPTIONS /permissive` allowed-origin → preflight 200 + 6 CORS headers verbatim per §11.2; `OPTIONS /strict` disallowed-origin → 405; `GET /permissive` allowed-origin → 200 + body + 3 injected CORS headers; `GET /strict` no-origin → 200 + body + no CORS headers); per-request status + response header set + body byte-equal across envoy-go and reference Envoy v1.37.2 (modulo allow-list).

**Precondition:** Tasks 18–20 done.
**Artifact:** Seven new fixture files.
**Acceptance:** `go test ./test/differential/ -run 'Test.*0007a' -count=1 -v` passes — per-request equivalence on the 4-request workload.

- [ ] **Step 1: Write `envoy-go.yaml`** (subject bootstrap; STATIC cluster; `http_filters: [envoy.filters.http.cors, envoy.filters.http.router]`; two routes with `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{...}`).

- [ ] **Step 2: Write `envoy.yaml`** (reference bootstrap; STRICT_DNS + `host.docker.internal:<backend-port>` per ADR-0010; `--concurrency 1` per ADR-0028).

- [ ] **Step 3: Write `backends/main.go`** (small Go HTTP/1.1 server returning 200 OK with body `"hello\n"`).

- [ ] **Step 4: Write `driver/driver.go`** (4 H1 requests per side; per-request status + header set + body byte-equality assertions; per-route config differential exercise).

- [ ] **Step 5: Write `expectations.yaml`** (prose description + 4-request expectation table per §11.2).

- [ ] **Step 6: Update `test/differential/runner.go`** (blank-import + register `RequiresReference: true`).

- [ ] **Step 7: Run + commit**

```bash
go test ./test/differential/ -run 'Test.*0007a' -count=1 -v
git add test/differential/0007a-cors/ test/differential/runner.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: fixture 0007a-cors (differential)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3, §7.2 (matrix), §11.2 (verbatim shape), §15 acceptance bullet 12.*

---

## Task 22: Structural fixture `test/differential/0007b-iteration-probe/`

**Files:**
- Create: `test/differential/0007b-iteration-probe/envoy-go.yaml`
- Create: `test/differential/0007b-iteration-probe/expectations.yaml`
- Create: `test/differential/0007b-iteration-probe/README.md`
- Create: `test/differential/0007b-iteration-probe/driver/driver.go`
- Create: `test/differential/0007b-iteration-probe/driver/driver_test.go`
- Create: `test/differential/0007b-iteration-probe/backends/main.go`
- Modify: `test/differential/runner.go` (blank-import the new driver; register `RequiresReference: false`)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md`

8-mode envoy-go-only structural fixture exercising every iteration-protocol state branch via `envoy.filters.http.envoy_go_test`. No `envoy.yaml` (no reference Envoy in this fixture). Per-mode embedded expectation table (status + headers + body) asserted against subject responses.

**Precondition:** Task 21 done.
**Artifact:** Six new fixture files.
**Acceptance:** `go test ./test/differential/ -run 'Test.*0007b' -count=1 -v` passes — all 8 modes' subject responses match the embedded expectation table.

- [ ] **Step 1: Write `envoy-go.yaml`** (`http_filters: [envoy.filters.http.envoy_go_test, envoy.filters.http.router]`; route `/` → STATIC cluster; per-route `typed_per_filter_config[envoy.filters.http.envoy_go_test] = {count: 7}`).

- [ ] **Step 2: Write `backends/main.go`** (H1 echo backend returning the request body if non-empty, else `"backend\n"`).

- [ ] **Step 3: Write `driver/driver.go`** (8 H1 requests with 8 distinct `x-envoy-go-test-mode` values per SPEC §7.3; per-mode embedded-expectation-table assertions; `RequiresReference: false`).

- [ ] **Step 4: Write `expectations.yaml`** (8-mode table per SPEC §7.3).

- [ ] **Step 5: Update `test/differential/runner.go`** (blank-import + register `RequiresReference: false`).

- [ ] **Step 6: Run + commit**

```bash
go test ./test/differential/ -run 'Test.*0007b' -count=1 -v
git add test/differential/0007b-iteration-probe/ test/differential/runner.go docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: fixture 0007b-iteration-probe (structural)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3, §7.3 (8-mode matrix), §15 acceptance bullet 13.*

---

## Task 23: BEHAVIOR_CONTRACT in-place edit + ROADMAP/STATE updates + closing six-gate sweep

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (insert NEW `## HTTP filter chain` top-level section between `## HTTP/2` and `## TCP proxy`; amend `## HTTP/1.1` and `## HTTP/2` for ADR-0040 / ADR-0042 supersession; add row to `## Equivalence Matrix`)
- Modify: `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` (closing entry quoting all six gates' command outputs verbatim)
- Modify: `docs/envoy-go/STATE.md` (NOT touched at this commit — see Refinement; advanced by the verification session)
- Modify: `docs/envoy-go/ROADMAP.md` (NOT touched at this commit — see Refinement; flipped by the REVIEW session's phase-done commit)

The BEHAVIOR_CONTRACT in-place edit lands at THIS commit (the implementation session's last commit) per the 06.1 / 06.2 in-place-edit-at-impl-session-last-commit timing convention (per Refinement). The four §11 empirical-pin blocks land verbatim in `## HTTP filter chain`'s `### Empirical evidence (filter ordering)` / `### Empirical evidence (cors preflight)` / `### Empirical evidence (413 overflow)` / `### Empirical evidence (sendLocalReply entry)` subsections per SPEC §13.1. The §11 block + the §13 block are paste-verbatim-synchronized.

**Precondition:** Tasks 1–22 done; pre-existing fixtures still green; new fixtures green.
**Artifact:** BEHAVIOR_CONTRACT extended in place; PROGRESS quotes all six gates.
**Acceptance:** all six phase-done gates (a–f) green per SPEC §3 (gate (f) defers to REVIEW session); the boundary grep at step 4 surfaces no third-party filter-chain library; the four-empirical-pin grep at step 5 confirms paste-verbatim-synchronization.

- [ ] **Step 1: Insert NEW `## HTTP filter chain` top-level section into BEHAVIOR_CONTRACT.md** (between `## HTTP/2` ending at line ~520 and `## TCP proxy` starting at line ~329 in the current file — actually between `## HTTP/2`'s end and the next existing section; verify the insertion point at execution time)

Use the SPEC §13.1 verbatim block (the one starting with `## HTTP filter chain` heading and ending with the `### Does not yet apply to` block). Paste the four §11 empirical-pin blocks verbatim into the four `### Empirical evidence (...)` subsections.

- [ ] **Step 2: Amend `## HTTP/1.1` and `## HTTP/2` for ADR-0040 / ADR-0042 supersession**

In `## HTTP/1.1` (line 410 in current file):
- Replace "The phase-04 HCM-filter chain shape `[router]` (ADR-0042)." → "The phase-04 HCM-filter chain shape requirement that the chain be non-empty with router as terminal entry; the original ADR-0042 'exactly `[router]`' rule was partially superseded by ADR-0071 in phase 07.1 (lower bound stays; upper bound lifted) — see `## HTTP filter chain` for the full discipline."
- Replace "HCM filter chain beyond `[router]` (phase 07's filter-chain framework)." → "The full HTTP-filter chain framework (iteration protocol, async-resume, sendLocalReply semantics, per-route config) — see `## HTTP filter chain`."

In `## HTTP/2` (line 452 in current file):
- Add a "see `## HTTP filter chain` for HCM filter-chain dispatch wiring" reference where ADR-0040 / ADR-0042 rules are referenced.

- [ ] **Step 3: Add new row to `## Equivalence Matrix` (per SPEC §13.2)**

The existing `## Equivalence Matrix` table at line 9 of BEHAVIOR_CONTRACT.md gains a "HTTP filter chain" row:

```
| HTTP filter chain      | Per-request equivalence on cors preflight + actual-     | Differential covers cors only.        |
|                        | request response shapes (status + header set + body)    | envoy.filters.http.envoy_go_test      |
|                        | between envoy-go and reference Envoy. Filter            | excluded (test-only). All other       |
|                        | iteration order, sendLocalReply encode-chain entry,     | filters in §9 family are              |
|                        | and 413 overflow shape are verbatim-pinned at the       | future-phase scope.                   |
|                        | ENVOY_TARGET SHA.                                       |                                       |
```

- [ ] **Step 4: Run gate (a) sweep — all differential fixtures green**

```bash
go test -count=1 ./test/differential/... -v 2>&1 | tee /tmp/07.1-gate-a.log
```

Expected: all 9 fixtures (`0000` through `0007b`) PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 5: Run gate (c) sweep — h2spec at 53/53 PASS at the ADR-0051 pin**

```bash
docker run --rm summerwind/h2spec@sha256:<pin from CONFORMANCE_PINS.md> -h 127.0.0.1 -p <subject port> -t -s 1.1 -e <sections per CONFORMANCE_PINS.md>
```

Expected: 53/53 PASS, 0 fail. Quote into PROGRESS.

- [ ] **Step 6: Run gate (d) sweep — all 9 fuzzers clean for 30s each**

```bash
for fuzz in $(go test -list '.*Fuzz.*' ./... | grep -E '^Fuzz'); do
	go test -fuzz=$fuzz -fuzztime=30s ./... # adapt per package per ADR-0018
done
```

Expected: 9 fuzzers (8 from prior phases + new FuzzFilterChainParse) clean.

- [ ] **Step 7: Run gate (e) sweep — vet + lint + test -race**

```bash
go vet ./...                                                   2>&1 | tee /tmp/07.1-vet.log
golangci-lint run ./...                                        2>&1 | tee /tmp/07.1-lint.log
go test -race -count=1 ./...                                   2>&1 | tee /tmp/07.1-race.log
```

Expected: all clean. Quote each.

- [ ] **Step 8: Boundary grep — no third-party filter-chain library**

```bash
grep -rE 'github.com/justinas/alice|github.com/urfave/negroni|github.com/go-chi/chi/middleware|github.com/gorilla/handlers' . --include='*.go' --include='go.mod' --include='go.sum'
```

Expected: zero matches (per SPEC §15 final acceptance bullet).

- [ ] **Step 9: Empirical-pin grep — confirm BEHAVIOR_CONTRACT carries verbatim §11 blocks**

```bash
grep -A5 '^### Empirical evidence (filter ordering)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (cors preflight)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (413 overflow)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (sendLocalReply entry)' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

Expected: each returns the corresponding §11 block's first 5 content lines verbatim (the `[2026-05-01 ...][critical][lua] ...` traces / `< HTTP/1.1 ...` wire captures / `00000000: 5061 796c ...` hex dumps).

- [ ] **Step 10: Append closing PROGRESS entry**

```markdown
## Task 23 — BEHAVIOR_CONTRACT in-place edit + closing six-gate sweep [ADR-0070, ADR-0071, ADR-0072, ADR-0073, ADR-0074, ADR-0075, ADR-0076]

**Commits:** TBD — this task's commit
**Notes:** Landed BEHAVIOR_CONTRACT.md ## HTTP filter chain section + four §11 empirical-pin blocks verbatim + ADR-0040 / ADR-0042 supersession amendments + Equivalence Matrix row addition. Six-gate sweep all green (gate (f) defers to REVIEW session). Total fuzzer count post-07.1 is 9. No third-party filter-chain library imported.
**Outputs (verbatim — gate (a) tail; gate (c) summary; gate (d) summary; gate (e) all clean; gate boundary grep zero):**
\`\`\`
$ go test -count=1 ./test/differential/... -v | tail -30
<verbatim>
$ docker run ... summerwind/h2spec ... | tail -10
<verbatim — 53/53 PASS>
$ go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/ | tail -5
<verbatim — clean>
$ go vet ./...
<verbatim — empty (clean)>
$ golangci-lint run ./...
<verbatim — empty (clean)>
$ go test -race -count=1 ./... | tail -20
<verbatim — all PASS>
$ grep -rE 'github.com/justinas/alice|github.com/urfave/negroni|...' . --include='*.go' --include='go.mod' --include='go.sum'
<verbatim — empty (zero matches)>
\`\`\`
```

- [ ] **Step 11: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md
git commit -m "phase 07.1: BEHAVIOR_CONTRACT.md ## HTTP filter chain + closing sweep [ADR-0070, ADR-0071, ADR-0072, ADR-0073, ADR-0074, ADR-0075, ADR-0076]"
```

SHA-fill follow-up.

- [ ] **Step 12: Confirm phase-07.1 readiness for state-5 transition (do NOT advance STATE — that's the verification session per BOOTSTRAP §5)**

The implementation session ends with Task 23 committed on `phase/07.1-http-filter-framework-impl`. STATE advancement through 4 → 5 → 6 is per-session work, not this task's responsibility. The Refinement section names what the verification + REVIEW sessions land on top of this commit.

*Anchored: SPEC §1 #11, §3 (six-gate phase-done), §4.4 (ROADMAP/STATE/PROGRESS lifecycle), §13 (BEHAVIOR_CONTRACT additions), §15 (full acceptance checklist), and BOOTSTRAP §5.3 (commit-message-completeness), §7.5 (six-gate sweep).*

---

## Refinement

This section absorbs the conventions that the 06.2 PLAN's Refinement section codified for execution-time consistency. Every item below applies to phase 07.1 unless explicitly noted.

**SHA-fill follow-up convention (per phase-02 / 03 / 04 / 05.1 / 05.2 / 06.1 / 06.2 precedent).** Every task's commit lands the task's main change; immediately after, a follow-up tiny commit `phase 07.1: PROGRESS SHA-fill for Task N` updates that task's PROGRESS.md `**Commits:**` line with the just-landed short SHA. The follow-up commit's body is empty; its title is the only line. Two commits per task; the executor MUST NOT skip the follow-up.

**BEHAVIOR_CONTRACT in-place edit lands at the Task 23 commit (per ADR-0052).** The `## HTTP filter chain` section addition + the `## HTTP/1.1` + `## HTTP/2` amendments + the `## Equivalence Matrix` row addition land at Task 23's commit, NOT at any earlier task's commit. Per ADR-0052 the in-place edit is authorised; per SPEC §4.4 the timing is "at the phase-done commit" — but per BOOTSTRAP §5 step 6 the phase-done commit is the REVIEW session's, NOT the implementation session's; the BEHAVIOR_CONTRACT edit anticipates the REVIEW session by landing at Task 23 (the implementation session's last commit) so the verification session can grep-check the edit before REVIEW runs. Mirrors the 06.1 / 06.2 PLAN's identical convention.

**Empirical-pin blocks land verbatim at Task 23 (NOT scraped at Task 23).** Per SPEC §11 + §13.1, the four empirical-pin blocks were already executed at SPEC time (against reference Envoy v1.37.2 SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) and pinned verbatim in SPEC §11. Task 23's job is to PASTE the §11 blocks into BEHAVIOR_CONTRACT.md `## HTTP filter chain` verbatim — NOT to re-scrape. The §11 block + the §13 block are paste-verbatim-synchronized; future image bumps per `ENVOY_TARGET.md`'s refresh procedure that alter any of the four shapes require updating BOTH locations in the same commit, mirroring the 06.1 / 06.2 paste-verbatim discipline.

**Multi-file refactor handling for Tasks 12–16 (delete-then-rewire).** Tasks 12 (delete `routerAction`) + 13 (config.go widening) + 14 (filter.go constructor widening) + 15 (H1 dispatch) + 16 (H2 dispatch) are a delete-then-rewire sequence. The package may be temporarily not-buildable between Tasks 12 and 14 (inclusive); buildability is restored at Task 14 (constructor widen finishes the surface) and the differential gates (b) re-runnable from Task 15 onward. Two execution patterns are permitted:
- **Pattern A (recommended for review-granularity):** five separate commits (one per task); inter-task `go build` failures are documented in each PROGRESS entry's "Notes" field as "package does not yet build; restored at Task <N>"; the executor confirms buildability is restored at the targeted task before SHA-fill follow-up.
- **Pattern B (recommended for atomic-refactor preference):** Tasks 12–16 batched as one commit with five PROGRESS entries against that one commit; the executor still SHA-fills against the single bundled commit per the 06.2 PLAN's allowance for related multi-file refactors when each component is independently undebuggable.

The PLAN does not prescribe which pattern; the executor picks based on review preference at execution time.

**ROADMAP row 07.1 → in-progress at the SPEC commit (already landed); → done at the phase-done commit (parent row 07 STAYS in-progress).** Per BOOTSTRAP §4.1 invariant 3: at the SPEC commit (already landed at master `ee45aba`, before this PLAN commit), row 07.1 flipped `planned → in-progress` — the SPEC-authoring session did this. Per SPEC §4.4: at the phase-done commit (the REVIEW session's lifecycle-state-6 commit, NOT Task 23), row 07.1 flips `in-progress → done`; row 07 (parent) STAYS `in-progress` (the parent only flips to `done` at 07.2's phase-done commit per the 05/05.1/05.2 + 06/06.1/06.2 closure pattern). Task 23's commit deliberately does NOT touch ROADMAP.md; the anticipated text is recorded in the PROGRESS Task 23 entry but lands at the REVIEW session's phase-done commit. The phase-done commit-message body explicitly names ROADMAP-row 07.1 transition + the explicit non-transition of row 07 per SPEC §3's commit-subject template.

**ADR-numbering monotonicity discipline (ADR-0070..ADR-0076 contiguous).** Per ADR-0004's autonomous-numbering rule, the planner verified at PLAN-write time that the DECISIONS.md tail is `ADR-0069`; phase 07.1's seven ADRs land at ADR-0070..ADR-0076 (contiguous block). Per `## ADRs introduced by this plan` above, the commit-time ordering (Task 1 / Task 2 / Task 3 / Task 4 / Task 7 / Task 9 / Task 18) produces non-monotonic ADR-number-vs-commit-order at the last entry (0070, 0071, 0072, 0073, 0075, 0076, 0074), permitted per SPEC §8 and the 05.2 + 06.1 + 06.2 precedents. The contiguous-block discipline is preserved (no gaps); each ADR's `Lands-in-task` field records the in-task anchoring. The Task 1 step 1 precondition re-verifies the tail; if ADR-0069 has been superseded by a mid-PLAN-authoring ADR, every task's ADR reference shifts uniformly.

**Commit-message-completeness check (per BOOTSTRAP §5.3).** Each task's commit message names the ADR(s) introduced in that task (in `[ADR-NNNN]` square-bracket form per the phase-04/05.1/05.2/06.1/06.2 convention). The Task 23 closing commit (per Step 11) names ALL SEVEN ADRs in the bracketed list — `[ADR-0070, ADR-0071, ADR-0072, ADR-0073, ADR-0074, ADR-0075, ADR-0076]` — so a `git log --grep='ADR-007[0-6]'` query surfaces every authoring task plus the closing task. The phase-done commit (REVIEW session's) carries the same bracketed list per SPEC §3.

**Six-gate local sweep at Task 23 (per BOOTSTRAP §7.5; SPEC §3).** Gates (a) / (b) / (c) / (d) / (e) all run at Task 23; gate (f) defers to REVIEW. The PROGRESS Task 23 entry quotes each gate's last-30-lines output verbatim. The Task 23 step 8 boundary grep + the step 9 four-empirical-pin grep are SPEC §15-anchored acceptance bullets that the verification session re-runs.

**No-third-party-filter-chain-library acceptance (per ADR-0071 + SPEC §15).** Task 23 step 8's grep is the gate; the executor CONFIRMS no `justinas/alice`/`urfave/negroni`/`go-chi/middleware`/`gorilla/handlers` import lands in any production-code path. Test-side use is also forbidden. The grep applies uniformly across `_test.go` and production code.

**Mid-execution split valve.** Per `## Scope check` triggering re-evaluation: if cumulative landed-LoC by Task 20 exceeds 9000, invoke `superpowers:systematic-debugging`. Per `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger, if any single task's sub-steps blow past 15 (vs the recommended 10 trigger; 15 reflects the framework's structural complexity), the executor splits per §6.2 with a new ADR. The natural axis remains 07.1.1 (framework + dispatch wiring + router migration; Tasks 1–17) and 07.1.2 (filters + boot wiring + fixtures + closing sweep; Tasks 18–23) — with the caveat from the Scope check argument #1 that 07.1.1 has vacuous gate (a) and would need a placeholder probe filter to land non-vacuously.

**Empirical-pin discipline (ENVOY_TARGET image bumps).** The four §11 empirical-pin blocks are verbatim-paste against reference Envoy v1.37.2 (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`). If a future phase bumps `ENVOY_TARGET.md`'s pin, ALL FOUR pin blocks in BOTH SPEC §11 AND `BEHAVIOR_CONTRACT.md ## HTTP filter chain` must be re-scraped + updated in the SAME commit (the pin-bump phase's commit). The §11 block + the §13 block are synchronized; no drift permitted.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 23 on `phase/07.1-http-filter-framework-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/07.1-http-filter-framework-impl
   ```
2. **The verification session** (next-fresh from the implementation session) re-runs all six gates per BOOTSTRAP §7.5 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. Verification commits `phase 07.1: STATE.md → lifecycle-state 5` on master.
3. **The REVIEW session** (next-fresh from verification) writes `docs/envoy-go/phases/07.1-http-filter-framework/REVIEW.md` per BOOTSTRAP §5 state 5 → 6. The REVIEW session's phase-done commit (per SPEC §3 commit-subject template):
   - Flips ROADMAP row 07.1 → `done`.
   - Leaves ROADMAP parent row 07 at `in-progress` (per the 05/05.1/05.2 + 06/06.1/06.2 closure pattern; row 07 closes only at 07.2's phase-done).
   - Lands the BEHAVIOR_CONTRACT verification block (a re-grep that the Task 23 edit landed correctly).
   - Advances STATE to phase 07.2 (`active-phase: 07.2-listener-chain-completion`; `lifecycle-state: 1` per the §5 state machine; `next-skill: superpowers:brainstorming`) at the SAME phase-done commit.
   - Commit message: `phase 07.1: phase-done — http-filter-framework lands; ROADMAP row 07.1 → done; row 07 stays in-progress [ADR-0070, ADR-0071, ADR-0072, ADR-0073, ADR-0074, ADR-0075, ADR-0076]`.

**No part of this section is done by Task 23.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/07.1-http-filter-framework-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` (per ADR-0005 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md), `next-skill-scope: <execute PLAN.md per the 23-task sequence; create worktree .worktrees/phase-07.1-http-filter-framework-impl per ADR-0003>`, `last-commit: TBD` (the SHA-fill follow-up commit lands the actual SHA per the phase-02..06.2 SHA-fill precedent).
3. Fast-forward `master` to `phase/07.1-http-filter-framework-plan` per ADR-0003 (parent session's responsibility).
4. Worktree for the next session: `.worktrees/phase-07.1-http-filter-framework-impl` on branch `phase/07.1-http-filter-framework-impl` (recommended per `## Execution preconditions` #1).
5. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on `phase/07.1-http-filter-framework-plan`). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (`internal/filter/http/{doc,types,callbacks,registry,chain,perroute,fuzz_test}.go` and the test pairs; `internal/filter/http/cors/{doc,cors}.go` + tests; `internal/filter/http/envoygotest/{doc,filter}.go` + proto + tests; `internal/filter/http/router/{doc,router,router_h2}.go` + byte-preserved tests; `internal/filter/hcm/{config,filter,actions,connection,h2dispatch,accesslog_emit,chain_integration_test}.go` modifications; `internal/filter/doc.go` rewrite; `internal/listener/manager.go`, `internal/bootstrap/bootstrap.go`, `cmd/envoy-go/main.go` boot wiring; differential fixture 0007a + structural fixture 0007b in full; runner registration; seven ADRs ADR-0070..ADR-0076; `BEHAVIOR_CONTRACT.md ## HTTP filter chain` in-place edit + amendments to `## HTTP/1.1` and `## HTTP/2` + Equivalence Matrix row.)
- Does the PLAN settle every 07.1-scoped SPEC §12 deferred decision? (8 items — see `## Settled SPEC §12 deferred decisions`.)
- Does the PLAN mitigate every SPEC §14 testing-strategy item with a task-level step? (14.1 unit tests for `internal/filter/http/` → Tasks 2–9; 14.2 `cors_test.go` → Task 18; 14.3 `envoygotest/filter_test.go` → Task 19; 14.4 `router_test.go` byte-preserved → Task 11; 14.5 `internal/filter/hcm/` test extensions → Tasks 13–17; 14.6 differential fixture 0007a → Task 21; 14.7 structural fixture 0007b → Task 22; 14.8 h2spec re-run → Task 23 step 5; 14.9 fuzzers → Task 10 + Task 23 step 6; 14.10 race detector + lint → Task 23 step 7.)
- Does the PLAN preserve the empirical-pin discipline? (SPEC §11 blocks land verbatim at Task 23 step 1; the §11 block + the §13 block are synchronized — no drift.)
- Does the PLAN preserve the H2 ctx-cancel zero-status sentinel from 06.2? (Task 16 step 3 — `chain.LastStatusCode` returns 0 on cancellation; `emitAccessLogH2` skips submission per the 06.2 inheritance.)
- Does the PLAN preserve the carry-forward triage from SPEC §10? ("None expected" — confirmed in `## Carry-forward triage`; 06.2 REVIEW Minors stay separate; 04 + 05.x deferred items unchanged in disposition.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 5 chain.go decode-side, Task 7 beginLocalReply, Task 11 router migration, Task 15+16 dispatch wiring, Task 21+22 fixtures, Task 23 closing sweep)?
- Does the ADR number sequence match the verified DECISIONS.md tail? (ADR-0069 → ADR-0070..0076; non-monotonic mapping by topic-vs-first-use-order documented above.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~5500 LoC effective, 23 tasks, no further coherent split axis exists without vacuous-gate-(a) anti-pattern; per phase-04 / 05.1 / 05.2 / 06.1 / 06.2 precedent, one-sub-phase shipment is correct.)
- Does the import topology stay clean? (`internal/filter/http/` is a near-leaf importing only stdlib + `google.golang.org/protobuf` + `cors/v3` proto + `internal/cluster` (router sub-package only); `internal/filter/hcm/`, `internal/listener/`, `internal/bootstrap/`, `cmd/envoy-go/` import `internal/filter/http/`; no third-party filter-chain library; the boundary grep at Task 23 step 8 enforces.)
- Are the seven ADRs internally consistent? (ADR-0070's split decision matches the SPEC commit's ROADMAP edit; ADR-0071's interface shape matches Task 2's types.go; ADR-0072's freeze invariant matches Task 3's registry_test.go; ADR-0073's 3-tier merge matches Task 4's perroute_test.go; ADR-0074's filter set matches Tasks 18+19's filter implementations; ADR-0075's encode-chain entry matches Task 7's beginLocalReply; ADR-0076's 1-MiB cap + 413 + reset matches Task 9's chain.go overflow path.)
- Are the empirical pins faithfully transcribed? (Task 23 step 9 grep-verifies; SPEC §11.1's lua trace, §11.2's four wire captures, §11.3's 17-byte body hex dump, §11.4's lua trace + wire capture all paste-verbatim into BEHAVIOR_CONTRACT.)




