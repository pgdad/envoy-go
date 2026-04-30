# Phase 07 Brainstorm — Filter Chain Framework

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-1 brainstorm session for phase 07 (`filter-chain-framework`). The next session (lifecycle-state 2, skill `superpowers:writing-plans`) authors `SPEC.md` for phase **07.1** based on this brainstorm. Phase 07.2 receives a sibling SPEC stub at the same time.

**Brainstorm session:** worktree `.worktrees/phase-07-filter-chain-framework-brainstorm`, branch `phase/07-filter-chain-framework-brainstorm`, branched from master tip `97ab12a` (phase-06.2 phase-done SHA-fill commit).

---

## 1. Scope decision — planner-time split

ROADMAP row 07 bundles four deliverables: **HTTP filter iteration protocol + per-route config + extension registry + a trivial pluggable filter that covers all iteration states.** ADR-0033 (phase 03) also pre-deferred a separate set of items to phase 07: full `FilterChainMatch` (destination_port, prefix_ranges, source_*, application_protocols/ALPN), `listener_filters`, and `Listener.default_filter_chain`. These two surfaces are architecturally disjoint — the first lives inside HCM (`internal/filter/hcm/` and a new `internal/filter/http/`); the second lives in `internal/listener/`. Per ADR-0045 (planner-time split) and the phase-06 → 06.1/06.2 precedent, phase 07 splits at brainstorm time into two sub-phases:

| ID | Title | Scope | Differential surface |
|---|---|---|---|
| **07.1** | `http-filter-framework` | HTTP filter iteration protocol + extension registry + per-route config + trivial real filter (`cors`) + test-only probe filter (`envoy_go_test`); supersedes ADR-0040, partially supersedes ADR-0042; amends ADR-0041 silent-ignore set | fixture `0007a-cors` (differential, equivalence-with-Envoy) + fixture `0007b-iteration-probe` (envoy-go-only, structural assertion) |
| **07.2** | `listener-chain-completion` | `listener_filters` framework + `FilterChainMatch` fields beyond SNI (destination_port, prefix_ranges, source_*, application_protocols/ALPN) + `Listener.default_filter_chain`; supersedes parts of ADR-0033 | fixture(s) TBD when 07.2 brainstorms |

**Rationale for split:**

- 07.1 lives entirely under `internal/filter/` and `internal/filter/hcm/`. 07.2 lives entirely under `internal/listener/`. Zero shared code surface.
- 07.1 is the larger / more novel build (iteration protocol is brand-new machinery; ~800–1200 LOC plus filter implementations). 07.2 is enumerative ADR-0033 follow-ups (~300–500 LOC).
- The HTTP-filters family (BOOTSTRAP §9) becomes unblocked the moment 07.1 lands — that's the deliverable that releases the phase-09+ planning chain. Bundling 07.2 into 07.1 delays the family unblock for surface that has no §9 dependents.
- Bundling them into one phase risks the same SPEC-bloat that drove the 06.1/06.2 split.

**07.1 ships first.** 07.2 is independent (no dependency on 07.1's HTTP-filter machinery) and can be brainstormed any time after 07.1 phase-done; it sits as `planned` in ROADMAP after 07.1.

The phase-07 parent directory (`docs/envoy-go/phases/07-filter-chain-framework/`) carries this BRAINSTORM.md plus a master SPEC.md (eventually) summarizing both sub-phases. Mirrors `docs/envoy-go/phases/06-observability-baseline/`.

---

## 2. Phase 07.1 — design decisions

### 2.1 Iteration protocol surface *(Q3 + Q6 + Q7 outcomes → ADRs)*

**Decision:** Envoy-faithful protocol shape with **async-resume**, narrow method set, full state machine.

**Filter interfaces** (Go-idiomatic mirror of Envoy's `StreamDecoderFilter` + `StreamEncoderFilter`):

```go
type StreamDecoderFilter interface {
    DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
    DecodeData(data []byte, endStream bool)            FilterDataStatus
    DecodeTrailers(trailers http.Header)               FilterTrailersStatus
    SetDecoderCallbacks(cb DecoderFilterCallbacks)
    OnDestroy()
}
type StreamEncoderFilter interface {
    EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
    EncodeData(data []byte, endStream bool)            FilterDataStatus
    EncodeTrailers(trailers http.Header)               FilterTrailersStatus
    SetEncoderCallbacks(cb EncoderFilterCallbacks)
    OnDestroy()
}
```

A filter implements decode-only, encode-only, or both. The factory's return type signals which side(s).

**Status enums:**

- `FilterHeadersStatus`: `Continue`, `StopIteration`. (Envoy's `ContinueAndDontEndStream` is **out of MVP** — single-knob state we don't need.)
- `FilterDataStatus`: `Continue`, `StopIterationAndBuffer`, `StopIterationNoBuffer`. (Watermark variants are **out of MVP**.)
- `FilterTrailersStatus`: `Continue`, `StopIteration`.

**Callbacks** (filter ⇒ framework):

- `ContinueDecoding()` / `ContinueEncoding()` — resume from a paused StopIteration. Safe from any goroutine.
- `SendLocalReply(status int, body string, headers http.Header)` — synthesizes a response that **enters the encode-side filter chain** (Q6 = A; see §2.5 + §4.5).
- `EncodeHeaders` / `EncodeData` / `EncodeTrailers` (encoder-only filters) — inject response data without a sendLocalReply (rare; mostly used when the filter is the response source).
- `RequestRouteConfig() proto.Message` — the resolved per-route config for this filter (§2.3).

**Body buffering on `StopIterationAndBuffer`** (Q7 = A): the framework appends `decodeData` chunks into a per-stream buffer up to a hardcoded `const filterBufferLimitBytes = 1 << 20` (1 MiB; matches Envoy default). Overflow synthesizes a `413 Payload Too Large` local reply, which then flows through the encode-side filter chain. The configurable knobs (`per_connection_buffer_limit_bytes` on Listener, `per_request_buffer_limit_bytes` on Route) are **silently ignored at parse-time** per ADR-0041 pattern; promotion-to-honored is a future ADR.

**Filter ordering:** `http_filters[]` declaration order on decode-side; reverse declaration order on encode-side. Empirical-pin obligation at SPEC time (§2.6).

**Out-of-protocol-MVP** (deferred to first-filter-that-needs-it phase): high/low watermark events, `Encode1xxHeaders`, metadata frames, `ContinueAndDontEndStream`, `decoderBufferLimit()` query API, broader callback surface (`StreamInfo`, `connectionLocalAddress`, `route()`, `clusterInfo()`, etc.).

### 2.2 Extension registry *(Q4 outcome → ADR)*

**Decision:** Threaded constructor map. `*filter.HTTPRegistry` constructed in `cmd/envoy-go/main.go`, threaded into `hcm.NewFilterWithCtxAndSinksAndRegistry(...)`.

```go
package filter

type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)
type FilterInstanceFactory func() HTTPFilter // called once per request

type HTTPRegistry struct { /* keyed by typed_config.type_url */ }

func NewHTTPRegistry() *HTTPRegistry
func (r *HTTPRegistry) Register(typeURL string, f HTTPFilterFactory)
func (r *HTTPRegistry) Lookup(typeURL string) (HTTPFilterFactory, bool)
func (r *HTTPRegistry) Freeze()
```

`HTTPFilter` is a tagged-union over decoder-only / encoder-only / both. Registration happens once at boot in `cmd/envoy-go/main.go`:

```go
reg := filter.NewHTTPRegistry()
reg.Register(router.TypeURL,        router.New)
reg.Register(cors.TypeURL,          cors.New)
reg.Register(envoygotest.TypeURL,   envoygotest.New)
// after all registrations:
reg.Freeze()
```

**Two-step factory pattern.** Step 1 (HCM-build time): `HTTPFilterFactory` parses + validates `typed_config`, returns a `FilterInstanceFactory` closure. Step 2 (per-request): the closure allocates a fresh filter instance bound to the parsed config. Mirrors Envoy's `FilterFactoryFn`. Per-config validation is paid once; per-request cost is one allocation.

**Boot-freeze invariant.** Like `*stats.Registry` (06.1 LBP-1), `*HTTPRegistry` is mutable only during boot. `Freeze()` is idempotent and called from `cmd/envoy-go/main.go` after all `Register` calls. `Register` post-Freeze panics. A unit test sets the frozen flag and panics on `Register` after.

HCM `parseFilterWithCtx` takes `*HTTPRegistry` as a new parameter, walks `http_filters[]` in declaration order, and looks up each entry's factory by `typed_config.type_url`. Unknown type_url errors at config-parse time with `hcm: http_filters[i]: unknown type_url %q (registry: known are [...])`.

The router stays the **terminal filter** — semantically must be last in `http_filters[]`. Phase-04's "exactly `[router]`" rule (ADR-0042) is replaced by "non-empty; last entry must be router" — partial supersession.

### 2.3 Per-route config model *(Q2 outcome → ADR)*

**Decision:** `typed_per_filter_config` is honored on Route, VirtualHost, and RouteConfiguration scopes. Parsed at HCM-build time; merged on first lookup during iteration.

**Storage:** each Route/VirtualHost/RouteConfig that matched a registered filter's name carries a `map[string]proto.Message` (filter-name → typed proto). Build-time validation: `typed_per_filter_config` keys must reference names present in the chain's `http_filters[]`; unknown filter names error at parse.

**Merge order** (Envoy parity): `RouteConfiguration.typed_per_filter_config[name]` < `VirtualHost.typed_per_filter_config[name]` < `Route.typed_per_filter_config[name]`. The most-specific scope wins by **override**, not field-merge (this is Envoy's default; field-level merge via the `disabled` flag is **out of MVP**).

**Lookup API:** `DecoderFilterCallbacks.RequestRouteConfig() proto.Message` returns the merged proto for the calling filter's name. Returns nil if no per-route config applies. The merge is computed once, lazily, on first lookup per request, and cached on the per-stream filter context.

### 2.4 Trivial filter set *(Q5 outcome → 2 filters)*

**Filter A — `envoy.filters.http.cors`** (real Envoy filter, used by differential fixture `0007a`):

- `decodeHeaders`: detect preflight (`OPTIONS` + `Origin` + `Access-Control-Request-Method`); if matched, `sendLocalReply(204, "", corsHeaders)` synthesizing the preflight response. Non-preflight: pass-through.
- `encodeHeaders`: append `Access-Control-Allow-Origin` / `Access-Control-Expose-Headers` / `Access-Control-Allow-Credentials` per the resolved per-route config.
- Per-route config: `envoy.extensions.filters.http.cors.v3.CorsPolicy` proto. Demonstrates the per-route override path (different allowed origins per route).
- Implementation surface: ~150 LOC + ~200 LOC tests.

**Filter B — `envoy.filters.http.envoy_go_test`** (test-only, used by structural fixture `0007b`):

- Per-request branching driven by `x-envoy-go-test-mode` header. Modes:
  - `continue` — Continue on every callback.
  - `stop-and-resume-headers` — StopIteration on decodeHeaders; spawn a goroutine that calls `ContinueDecoding()` after a 10ms tick.
  - `stop-and-buffer-data` — decodeData StopIterationAndBuffer until end_stream; modify buffer; Continue.
  - `local-reply-decode` — decodeHeaders sendLocalReply(418).
  - `local-reply-decode-data` — decodeData sendLocalReply(418) mid-body.
  - `modify-encode-headers` — pass-through decode; on encodeHeaders, add `x-envoy-go-test-injected: 1`.
  - `modify-encode-data` — pass-through decode; on encodeData, prefix body bytes.
  - `stop-trailers` — StopIteration on decodeTrailers; resume after tick.
- Per-route config: a `count` field that the filter reads via `RequestRouteConfig()` and echoes into a response header (`x-envoy-go-test-route-count: N`). Demonstrates per-route lookup.
- Lives in `internal/filter/http/envoygotest/` — proto schema is **envoy-go-only**, defined in `internal/filter/http/envoygotest/proto/` as a hand-rolled minimal proto Message (does **not** extend the upstream proto registry).
- Implementation surface: ~250 LOC + ~400 LOC tests.

### 2.5 Differential / structural fixture shape

**Fixture `0007a-cors`** (differential, equivalence-with-Envoy):

- 1 listener, 1 cluster, 1 endpoint. HCM `http_filters: [cors, router]`.
- 2 routes: `/permissive` (cors per-route allows `*`) and `/strict` (cors per-route allows only `https://example.test`). The `typed_per_filter_config` plumbing is exercised here.
- Driver issues four request shapes:
  - `OPTIONS /permissive` with `Origin: https://example.test` + `Access-Control-Request-Method: GET` → expect 204 + `Access-Control-Allow-*` headers.
  - `OPTIONS /strict` with `Origin: https://other.test` → expect preflight rejected per Envoy semantics (verify shape at SPEC time).
  - `GET /permissive` with `Origin: https://example.test` → expect 200 + body + `Access-Control-Allow-Origin: https://example.test` injected on response.
  - `GET /strict` (no `Origin` header) → expect 200 + body + no CORS headers.
- Equivalence claim: per-request status + response header set + body byte-equal across envoy-go and reference Envoy v1.37.2 (ignoring `Server`, `Date`, `Content-Length` per the existing differential ignore-list in BEHAVIOR_CONTRACT).

**Fixture `0007b-iteration-probe`** (envoy-go-only, structural assertion):

- envoy-go boots with the test-only probe filter chained before router. No reference Envoy in this fixture.
- Driver issues a request matrix covering each `x-envoy-go-test-mode` value (~8 modes per §2.4 Filter B).
- Each request's response is asserted against a per-mode expectation table embedded in `expectations.yaml` (status, response headers, response body). Same shape as 0006-access-log's per-record matrix (per ADR-0068's three-tier pattern, but here the only tier is "structural assertion" since reference Envoy is absent).
- The probe filter acts as filter[0] in the chain `[envoygotest, router]` — no real proxying happens for `local-reply-*` modes; the others proxy through to a backend on P3 to exercise the encode-side path.

### 2.6 Empirical-pin obligations *(SPEC-author work)*

The brainstorm explicitly does NOT settle these; they require an empirical scrape against Envoy v1.37.2 at SPEC-drafting time:

1. **Filter ordering on encode side:** verify that Envoy invokes encode filters in *reverse* declaration order (the assumption in §2.1) — boot Envoy with `http_filters: [a, b, router]` where `a` and `b` log on decode/encode entry, observe encode order is `[router, b, a]`, pin verbatim in BEHAVIOR_CONTRACT.
2. **Cors filter response shape:** the exact `Access-Control-*` header set produced by Envoy's cors filter (which headers, in what order, with what casing) on preflight + actual-request responses. Pin verbatim.
3. **413 overflow response shape:** the exact body, headers, and status returned by Envoy when `per_connection_buffer_limit_bytes` is exceeded. Pin verbatim.
4. **Local-reply encode-chain entry filter index:** Envoy enters at filter[len-1] of encode-chain by default per the source; verify the actual on-wire behavior and pin (some flag combinations may behave differently).

### 2.7 Carry-forward dispositions

None expected. Phase 06.2 REVIEW.md's 11 Minors are a separate post-phase-done batch (per STATE.md `next-skill-scope`) and do not interact with phase 07's scope. Phase 04's deferred items (M-X from 04 REVIEW) are H1-protocol-specific and unrelated to filter-chain framework.

---

## 3. Architecture & package layout

```
internal/filter/                          -- existing; expanded
  doc.go                                  -- rewrite: framework overview
  http/                                   -- new: HTTP filter framework
    types.go                              -- StreamDecoderFilter, StreamEncoderFilter,
                                          --   FilterHeadersStatus / FilterDataStatus /
                                          --   FilterTrailersStatus, FilterFactory
    callbacks.go                          -- DecoderFilterCallbacks, EncoderFilterCallbacks
    registry.go                           -- HTTPRegistry, Lookup, Register, Freeze
    chain.go                              -- per-stream FilterChain state machine: decode/encode
                                          --   iteration, buffer mgmt, async-resume signaling
    perroute.go                           -- typed_per_filter_config parser + 3-tier merge
    doc.go
    *_test.go                             -- unit tests

    cors/                                 -- new: real Envoy filter (differential fixture A)
      cors.go                             -- ~150 LOC
      cors_test.go
      doc.go

    envoygotest/                          -- new: test-only probe filter (structural fixture B)
      filter.go                           -- ~250 LOC; per-request mode-driven branching
      filter_test.go
      proto/
        envoygotest.pb.go                 -- hand-rolled minimal proto (NOT in upstream go-control-plane)
      doc.go

    router/                               -- new: router extracted as a real filter
      router.go                           -- terminal filter; decodeHeaders dispatches route
                                          --   action (cluster dial OR direct_response synthesize)
      router_h2.go                        -- H2-specific dispatch (was routerActionH2 in hcm/actions.go)
      router_test.go
      doc.go

  hcm/                                    -- existing; substantially refactored
    config.go                             -- parses http_filters[] via *HTTPRegistry; per-route
                                          --   config plumbed via perroute.Build; routes carry
                                          --   the merged map
    filter.go                             -- NewFilter*WithRegistry(...) extends the constructor
                                          --   chain with *http.HTTPRegistry
    actions.go                            -- directResponseAction stays (for direct_response routes
                                          --   without filter-chain interaction). routerAction +
                                          --   routerActionH2 are DELETED — moved into router filter
    connection.go                         -- runs the FilterChain state machine before/after
                                          --   the terminal router filter
    h2dispatch.go                         -- same, H2 codec
    accesslog_emit.go                     -- existing emit-deferral hooks; the filter chain's
                                          --   completion path triggers emit
    bytecounter.go                        -- existing
    h2/                                   -- existing
  tcpproxy/                               -- existing; untouched in 07.1

cmd/envoy-go/main.go                      -- constructs *http.HTTPRegistry, registers router/cors/
                                          --   envoygotest, threads into hcm.NewFilter*

test/differential/0007a-cors/             -- new: differential fixture
  README.md, expectations.yaml,
  envoy.yaml, envoy-go.yaml, driver.go
test/differential/0007b-iteration-probe/  -- new: structural fixture (envoy-go-only)
  README.md, expectations.yaml,
  envoy-go.yaml, driver.go
test/differential/runner.go               -- registration update for fixtures 0007a + 0007b
```

**Key shape notes:**

1. **Two-step factory pattern.** `HTTPFilterFactory` is `func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)`. Step 1 (HCM-build time): parses + validates typed_config, returns a closure. Step 2 (per-request): closure allocates a fresh filter instance. Mirrors Envoy's `FilterFactoryFn`. Per-request cost is one allocation; per-config validation cost paid once.
2. **`internal/filter/http/router/` is the new router home.** The existing `routerAction` + `routerActionH2` in `hcm/actions.go` are moved into this package. `directResponseAction` stays in `hcm/actions.go` as a *route-action shape* (decided at route-match time, synthesized by the router filter when its terminal step runs). This is the largest refactor in 07.1: ~600 LOC moved; tests follow with one-line import-path edits.
3. **`*HTTPRegistry` is constructed once at boot and threaded explicitly.** Allocated in `bootstrap.Load()` or `cmd/envoy-go/main.go`, passed to `hcm.NewFilterWithRegistry(...)`. No package-global. Mirrors `*stats.Registry` (06.1 LBP-1) and `*cluster.Manager`. Frozen-after-boot invariant enforced by a panic in `Register` post-Freeze.
4. **Per-request state lives on a `FilterChain` instance.** Allocated by HCM dispatch (`connection.go` for H1, `h2dispatch.go` for H2) at the start of each request. Owns: filter instances (allocated via per-request factories), per-filter callbacks, decode/encode buffers, merged per-route config map, decode iteration index, encode iteration index, async-resume signal channel. Garbage-collected at end of request.
5. **Async-resume is channel-based.** `DecoderFilterCallbacks.ContinueDecoding()` sends on a `chan struct{}` owned by the `FilterChain`; the HCM dispatch goroutine selects on it (with `ctx.Done()`). No per-filter goroutine in the common case (filter returns Continue inline; channel never used). When a filter returns StopIteration, the HCM goroutine parks on the channel until either resume or ctx-cancel.
6. **`internal/accesslog/` is not touched in 07.1.** Existing emit-deferral hook sites in `accesslog_emit.go` continue to fire from the same code paths — the filter chain's terminal-completion path is the new emit trigger, but the emit logic itself is unchanged.
7. **Existing `NewFilterWithCtxAndSinks` signature widens to `NewFilterWithCtxAndSinksAndRegistry(...)`.** All four constructor variants in `hcm/filter.go` + `hcm/config.go` grow one parameter. The `cmd/envoy-go/main.go` call site updates with one line. Legacy call sites in tests update mechanically (tests not exercising the registry pass an empty `*HTTPRegistry`).
8. **`internal/filter/doc.go` rewrite.** Replaces the phase-00 placeholder ("real implementation lands in phase 07") with the actual architectural overview.

---

## 4. Data flow & wiring

### 4.1 Boot wiring (one-time, at process start)

```
cmd/envoy-go/main.go
   ↓
filter.NewHTTPRegistry()
   ↓ Register: router.TypeURL → router.New
   ↓ Register: cors.TypeURL   → cors.New
   ↓ Register: envoygotest.TypeURL → envoygotest.New
   ↓
bootstrap.Load(configPath) → *Bootstrap        [unchanged]
   ↓
cluster.NewManager(...)                        [unchanged]
   ↓
admin.New(ready, *stats.Registry)              [unchanged]
   ↓
listenerManager.New(listeners, *cluster.Manager, *stats.Registry, *filter.HTTPRegistry, accessLogSinks)
   ↓ on each listener.filter_chain[].filters[i] of type=HCM:
        hcm.NewFilterWithCtxAndSinksAndRegistry(tc, clusters, lc, registry, sinks, *filter.HTTPRegistry)
            ↓ parseFilterWithCtx walks msg.GetHttpFilters():
                 for each entry:
                    factory, ok := httpRegistry.Lookup(entry.typed_config.type_url)
                    if !ok → "hcm: http_filters[i]: unknown type_url ..."
                    instanceFactory, err := factory(entry.typed_config, factoryCtx)
                    chainConfig.append({name: entry.name, factory: instanceFactory})
            ↓ enforce: last entry MUST be router (terminal)
            ↓ enforce: at least 1 entry (the router)
            ↓ build typed_per_filter_config map per Route/VHost/RouteConfig
            ↓ validate typed_per_filter_config keys ⊆ chain filter names
            ↓ Filter struct stores: chainConfig []chainEntry, perRouteConfig map
   ↓
HTTPRegistry.Freeze()    -- panic on subsequent Register
```

### 4.2 Per-request decode-side flow

```
H1: connection.go runConnection() — request line + headers parsed via std http.ReadRequest
H2: h2dispatch.go onHeaders()      — request headers via h2.StreamReader

[converged dispatch from here]
   ↓
hcm.Filter.dispatchRequest(ctx, req, w):
   chain := newFilterChain(f.chainConfig, f.perRouteConfig, req)   -- allocates per-request state
        ↓ for each chainEntry: instance := entry.factory()         -- fresh per-request filter
        ↓ chain.filters = []HTTPFilter{cors, envoygotest, router}
   chain.runDecodeHeaders(req.Header, !hasBody)
        ↓ for i in 0..len(filters)-1:
             status := filters[i].DecodeHeaders(headers, endStream)
             switch status:
               Continue: i++
               StopIteration: park on chain.resumeCh; on resume i++
                 (sendLocalReply during pause: handled per §4.5)
   if request has body:
      chain.runDecodeData(body chunks)
        ↓ for each chunk: iterate decode-side; per-filter buffer if StopIterationAndBuffer
        ↓ overflow at filterBufferLimitBytes → synthesize 413 + enter encode chain
   if request has trailers:
      chain.runDecodeTrailers(trailers)
   ↓ terminal filter is router; its decodeHeaders/Data/Trailers initiates upstream dial OR
     synthesizes direct_response (depending on resolved route action)
```

### 4.3 Per-request encode-side flow

```
[Trigger paths for entering encode chain]
  (a) Router filter received upstream response: chain.runEncodeHeaders(...)
  (b) sendLocalReply called from any filter: chain.beginLocalReply(status, hdrs, body)
                                              → chain.runEncodeHeaders(...)

chain.runEncodeHeaders(headers, endStream):
   ↓ for i in len(filters)-1 .. 0   (reverse order)
        skip filter if it has no encoder side
        status := filters[i].EncodeHeaders(headers, endStream)
        switch status: Continue / StopIteration (resume on chain.encodeResumeCh)
   ↓ once iteration completes: write status line + headers to wire
       (H1: bufio.Writer; H2: streamWriter)
chain.runEncodeData(data chunks):
   ↓ same reverse iteration; same buffer mgmt; overflow on encode-side resets the connection
     (Envoy parity — empirically pinned at SPEC time)
chain.runEncodeTrailers(trailers):
   ↓ same reverse iteration

After encode chain terminates:
   ↓ accesslog.Sink.Emit hook fires (existing path; trigger is now chain-completion, not
     direct action.do return — emit-deferral path adapts)
```

### 4.4 Async-resume mechanics

```
Filter calls cb.ContinueDecoding() from any goroutine:
   ↓ select { case ch.resumeCh <- struct{}{}: default: }    -- non-blocking; idempotent
HCM dispatch goroutine, parked on StopIteration:
   ↓ select {
        case <-ch.resumeCh: i++; continue iteration
        case <-ctx.Done():  abort iteration; close downstream
        case <-time.After(reqDeadline): abort with 504
     }
```

The resume channel is buffered with capacity 1, so duplicate `ContinueDecoding()` calls (filter-author bug) are silently coalesced. `Filter.OnDestroy()` is called after iteration completes (success or abort) to let filters release resources. Per-stream goroutines are *not* spawned by the framework; if a filter spawns its own goroutine for an async lookup, that's the filter's responsibility (and OnDestroy is the filter's signal to cancel it).

**No goroutine bloat in the common case.** A request whose filters all return Continue runs entirely on the HCM dispatch goroutine — zero new goroutines. Async filters (test probe in `stop-and-resume-headers` mode, or future `jwt_authn`) spawn their own goroutines; that's their own design surface.

### 4.5 `sendLocalReply` flow

```
Filter[k] calls cb.SendLocalReply(status=403, body, headers) during decodeHeaders:
   ↓ chain marks decode-side aborted; cancels any pending resume
   ↓ chain.beginLocalReply(status, headers, body):
       synthesizedHeaders := mergeStandardLocalReplyHeaders(headers, status, len(body))
       chain.runEncodeHeaders(synthesizedHeaders, endStream=false)
       chain.runEncodeData([]byte(body), endStream=true)
       (no trailers)
   ↓ if any encode-side filter returns StopIteration during this: it parks normally
   ↓ written to wire after encode chain completes
```

The synthesized response enters the encode chain at filter[len-1] (encode iteration is reverse). All filters with encode side run, regardless of whether they were on the decode-side path before the abort. This is Envoy's default behavior (Q6 = A) and is empirically pinned at SPEC time (§2.6 #4) to confirm the filter-index entry point.

If two decode-side filters call `SendLocalReply` concurrently (filter[0] starts iteration; filter[2] resumes from async and calls SendLocalReply before filter[1]'s pause completes): the first call wins; the second is a no-op. The chain's "first SendLocalReply wins" guard is a `sync.Once` + atomic flag.

---

## 5. Error handling, edge cases, concurrency

### 5.1 Concurrency model

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `HTTPRegistry.Register` | Once per filter, at process start | `Registry.mu` Lock; panics if frozen |
| Boot | `HTTPRegistry.Lookup` from `parseFilterWithCtx` | Once per HCM build per filter entry | `Registry.mu` RLock |
| Per-request | `chain.runDecode*` / `chain.runEncode*` | Per request | None — single goroutine per request iterates the chain |
| Per-request | `cb.ContinueDecoding()` / `cb.ContinueEncoding()` | Per async filter, per request | Send-on-buffered-chan (capacity 1) — non-blocking; idempotent |
| Per-request | `cb.SendLocalReply()` | At most once per request | `sync.Once` guards entry into beginLocalReply |
| Per-request | filter `OnDestroy` | At end of request | None — called from HCM dispatch goroutine after chain teardown |

**Key invariant — single-goroutine-per-request iteration:** the HCM dispatch goroutine (per H1 connection `runConnection` or H2 stream `onHeaders`) is the *only* goroutine that drives `chain.runDecode*` / `chain.runEncode*`. Filter callbacks called from other goroutines (e.g., a filter's async timer goroutine calling `ContinueDecoding`) are signal-only — they unblock the dispatch goroutine via channel send; they do NOT enter chain iteration themselves. This makes the chain's internal state (iteration index, per-filter buffer, perRouteConfigCache) lock-free.

**HTTPRegistry freeze invariant** (mirrors 06.1 LBP-1): `Register` panics post-Freeze; `Freeze()` is idempotent and called from `cmd/envoy-go/main.go` after all `Register` calls. A unit test sets the frozen flag and panics on `Register` after.

### 5.2 Edge cases

- **Empty `http_filters[]`:** error at parse — `hcm: http_filters: must contain at least 1 entry (the router)`. (Phase-04 ADR-0042's "exactly [router]" rule is partially superseded — the lower bound stays.)
- **Last entry not router:** error at parse — `hcm: http_filters: last entry must be %q (router)`. The router is the terminal filter; non-terminal placement breaks the dispatch model.
- **Two filters with same name:** error at parse — `hcm: http_filters: duplicate filter name %q`. Matches Envoy.
- **Unknown `typed_config.type_url`:** error at parse — `hcm: http_filters[i]: unknown type_url %q (registry: known are [...])`. Lists known type_urls in the error for debuggability.
- **`typed_per_filter_config` referencing unknown filter name:** error at parse — `hcm: route_config: typed_per_filter_config: unknown filter name %q (chain has [...])`.
- **Filter returns invalid status enum value:** the framework treats it as Continue and logs a `hcm: filter %q returned invalid status %d` warning. Defensive — bad filter implementations don't deadlock the proxy.
- **`SendLocalReply` after encode chain has begun:** ignored; second-write-after-begin is a filter bug. Logged as `hcm: filter %q called SendLocalReply after encode-side started; ignoring`.
- **`StopIteration` returned from final encode-side filter (filter[0]):** parks the dispatch goroutine. Resume comes from `cb.ContinueEncoding()` on filter[0]'s callbacks. Same mechanics as decode-side.
- **Filter panics in DecodeHeaders / EncodeHeaders / etc.:** recovered by the chain dispatcher; synthesizes a 503 local reply (does NOT re-enter the encode chain through the panicking filter — beginLocalReply skips filters whose state is "panicked"). Logged with stack.
- **Body buffer overflow on decode-side:** synthesize 413; flow through encode chain per §4.5.
- **Body buffer overflow on encode-side:** reset connection (H1: close conn; H2: RST_STREAM). Empirically pinned at SPEC time.
- **Zero-length body chunks:** delivered to filter as `data=[]byte{}, endStream=true|false`. Framework does not skip; filters that don't care can return Continue immediately.
- **Connection close mid-iteration (downstream gone):** ctx cancels; chain dispatch unparks; aborts iteration; filter `OnDestroy` runs for cleanup. The dispatch goroutine returns.
- **`ContinueDecoding` called before iteration paused:** non-blocking send on a capacity-1 buffered channel — silently coalesced. The next `StopIteration` immediately resumes.
- **Filter calls `RequestRouteConfig()` before route is matched:** returns nil. Route matching happens in the router filter's decodeHeaders, which is the last-decode-side step; earlier filters that need per-route config get nil. (Real Envoy-divergence consideration but matches the simplest sane semantics for our route-match-late ordering.)
- **Per-route config: route has no `typed_per_filter_config[name]`, vhost does, route_config does:** vhost wins (most-specific). Lookup is constant-time after first cache.

### 5.3 Error paths

| Source | Failure mode | Behavior |
|---|---|---|
| `parseFilterWithCtx` errors | Bad config | Process exits at boot per existing pattern (`bootstrap.Load` → `main.go` fatal) |
| Factory parse errors | Bad typed_config | Wrapped as `hcm: http_filters[i]: %w` and reported at boot |
| `Lookup` miss at boot | Unknown type_url | Same — boot-time fatal |
| Per-request filter panic | Programming bug | Recovered → 503 local reply → log |
| Buffer overflow decode-side | Adversarial | 413 local reply via encode chain |
| Buffer overflow encode-side | Adversarial | Connection reset; log |
| Async resume after request done | Filter bug | Channel send is non-blocking; `OnDestroy` already fired; signal is dropped |
| `ctx.Done()` mid-iteration | Downstream gone | Abort iteration; `OnDestroy` runs; dispatch goroutine returns |

### 5.4 Persistence

**None.** Filter chain state is per-request, allocated on entry, garbage-collected on exit. No on-disk persistence. Matches Envoy.

### 5.5 Race-detector contract

`go test -race ./...` clean across:

- N goroutines calling `ContinueDecoding` on the same chain (idempotent, coalesced)
- One filter's timer goroutine vs the dispatch goroutine racing on `cb.SendLocalReply`
- Concurrent `HTTPRegistry.Lookup` calls from N HCM constructors at boot
- Per-request chain teardown vs an in-flight `cb.ContinueEncoding` from a slow filter (`OnDestroy` semantics)

Unit tests in `chain_test.go` exercise each. Differential fixture `0007a-cors` indirectly stresses end-to-end concurrency under load.

### 5.6 Things 07.1 does NOT handle

- High/low watermark backpressure events (deferred to first-filter-that-needs-it phase)
- 1xx informational headers (`Encode1xxHeaders`) — deferred
- Metadata frames — deferred
- `ContinueAndDontEndStream` status — deferred
- Configurable `per_connection_buffer_limit_bytes` — silently ignored (silent-ignore set extended; ADR)
- Configurable `per_request_buffer_limit_bytes` — silently ignored
- Per-filter goroutine isolation (filter panics recover but a runaway goroutine spawned by a filter is the filter's own responsibility)
- `filter.disabled` flag on per-route config (skip-this-filter-on-this-route override) — deferred
- Filter chain hot-reload — xDS family
- Router filter typed config knobs (`dynamic_stats`, `start_child_span`, etc. — phase-04 ADR-0040 ignored set inherits)

---

## 6. Testing strategy

### 6.1 Unit tests (per-package)

`internal/filter/http/`:

- `registry_test.go` — Register / Lookup / duplicate-name panic / Freeze / post-Freeze panic
- `chain_test.go` — decode-side iteration with all status combinations; encode-side reverse iteration; StopIteration + async resume via callback channel; SendLocalReply during decode → flows through encode; SendLocalReply twice → second is no-op; buffer overflow → 413; filter panic recovery → 503; ctx cancel mid-iteration; concurrent ContinueDecoding (race-clean); per-route config merge (3-tier override)
- `perroute_test.go` — typed_per_filter_config parse + merge + lookup with unknown filter name + missing scope

`internal/filter/http/cors/`:

- `cors_test.go` — preflight detection (OPTIONS + Origin + ACRM); preflight allowed origin → 204 + headers; disallowed origin → preflight rejected (Envoy-equivalent shape); actual request → encodeHeaders adds CORS response headers; per-route override (different allowed origins on different routes)

`internal/filter/http/envoygotest/`:

- `filter_test.go` — each `x-envoy-go-test-mode` value: continue / stop-and-resume-headers / stop-and-buffer-data / local-reply-decode / local-reply-decode-data / modify-encode-headers / modify-encode-data / stop-trailers. Per-route `count` config → response header injection.

`internal/filter/http/router/`:

- `router_test.go` — extracted from existing `hcm/actions_test.go`. Tests preserved: route match → cluster dial; direct_response synthesize; H1 502 + 503 paths; H2 502 + 503 paths; access-log emit-deferral. Test count drops marginally as some duplicate cases consolidate.

`internal/filter/hcm/`:

- `config_test.go` — modified: parses http_filters[] via registry; rejects empty / non-router-terminal / duplicate names / unknown type_url
- `connection_test.go` / `h2dispatch_test.go` — modified: dispatch invokes FilterChain before terminal router; existing assertions preserved
- `actions_test.go` — modified: routerAction tests deleted (moved to router pkg); directResponseAction tests stay
- New: chain-integration tests under `hcm/chain_integration_test.go` for the framework-runs-filters-then-router happy paths in both H1 and H2

### 6.2 Differential fixture `0007a-cors`

(See §2.5 above for matrix + equivalence claim.)

**Driver outline:**

1. Boot envoy-go on P1 + reference Envoy on P2 with identical bootstrap (1 listener, 1 cluster `c0` with 1 endpoint, HCM `http_filters: [cors, router]`, 2 routes `/permissive` + `/strict` with per-route cors policies differing).
2. Boot a controlled backend on P3 (returns 200 OK with a static body).
3. Send 4 request shapes:
   - `OPTIONS /permissive` with `Origin: https://example.test` + `Access-Control-Request-Method: GET` → expect 204 + `Access-Control-Allow-*` headers
   - `OPTIONS /strict` with `Origin: https://other.test` → expect preflight rejected (Envoy-equivalent shape — verify at SPEC time)
   - `GET /permissive` with `Origin: https://example.test` → expect 200 + body + `Access-Control-Allow-Origin: https://example.test` injected on response
   - `GET /strict` (no `Origin` header) → expect 200 + body + no CORS headers
4. Per-request equivalence: status + response header set + body byte-equal across envoy-go and Envoy (ignoring `Server`, `Date`, `Content-Length` per the existing differential ignore-list in BEHAVIOR_CONTRACT).

### 6.3 Structural fixture `0007b-iteration-probe`

envoy-go-only. Driver issues 8 requests, one per `x-envoy-go-test-mode` branch (§2.4 Filter B). Each request's response is asserted against an embedded expectation table (status, headers, body). Same shape as 0006-access-log's per-record matrix (per ADR-0068's three-tier pattern, but here the only tier is "structural assertion" since reference Envoy is absent).

The probe filter acts as filter[0] in the chain `[envoygotest, router]` — no real proxying happens for `local-reply-*` modes; the others proxy through to a backend on P3 to exercise the encode-side path.

### 6.4 h2spec re-run

Phase 07.1 touches HCM dispatch wiring (H1 and H2). The h2spec gate at 53/53 must remain green — if filter-chain integration regresses h2spec, that's a fail. Existing gate (c) re-runs unchanged at the ADR-0051 SHA pin.

### 6.5 Fuzzers

Existing 8 fuzzers re-run at 30s budget. **New: `FuzzFilterChainParse`** — fuzzes adversarial `http_filters[]` slices into `parseFilterWithCtx` (varied type_urls, malformed typed_configs, oversized counts). Cheap (~80 LOC); adversarial-config bugs are the most likely class of bug in the new chain parser. Total: 9 fuzzers post-07.1.

### 6.6 Race detector + lint

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean (gate (e)).

### 6.7 Differential gate scope clarification

Two fixtures land in 07.1 (`0007a-cors` differential + `0007b-iteration-probe` structural). The differential gate only asserts equivalence on `0007a`; `0007b` is a structural-assertion gate (matches the existing `0001-tcp-proxy-rr`-style "envoy-go behaves as documented" pattern, not an "equivalent to Envoy" pattern). The split is reflected in `test/differential/runner.go`'s fixture registration: `0007a` registers as `RequiresReference: true`, `0007b` as `RequiresReference: false`.

### 6.8 Pre-existing test surface migration

Moving `routerAction` + `routerActionH2` from `hcm/actions.go` to `internal/filter/http/router/` requires migrating ~600 LOC of action tests. To minimize regression risk, the migration is mechanical: imports update, package names update, but test bodies are byte-preserved. Any deviation from byte-preserve flags as an ADR-worthy decision in the SPEC. The refactor is large but bounded — no behavior change to the router itself, only the dispatch path that invokes it.

---

## 7. BEHAVIOR_CONTRACT.md additions

Phase 07.1 introduces a new top-level section `## HTTP filter chain` plus updates the Equivalence Matrix. The structure mirrors the existing `## HTTP/1.1`, `## HTTP/2`, `## TCP proxy` pattern.

### 7.1 New section `## HTTP filter chain`

Subsections (filled at SPEC time per §2.6 empirical-pin obligations):

```
### Asserted equivalence
- cors filter preflight response shape (status, header set, header values) byte-equal
  to reference Envoy v1.37.2 — verbatim scrape pinned at SPEC time (§2.6 #2)
- cors filter actual-request response header injection byte-equal
- Filter declaration order on decode side; reverse on encode side — verbatim
  scrape evidence pinned at SPEC time (§2.6 #1)
- 413 Payload Too Large response shape on decode-side buffer overflow —
  verbatim scrape evidence pinned at SPEC time (§2.6 #3)
- typed_per_filter_config 3-tier merge precedence (Route > VirtualHost >
  RouteConfiguration); most-specific override (no field-merge)
- sendLocalReply enters encode chain at filter[len-1] (reverse iteration
  start) — verbatim scrape evidence at SPEC time (§2.6 #4)

### Not asserted
- Behavioral equivalence of the test-only `envoy.filters.http.envoy_go_test`
  probe filter — structural assertion only (no reference Envoy implements it)
- Watermark backpressure event timing (out of MVP scope)
- 1xx informational header processing (out of MVP scope)
- Metadata frame processing (out of MVP scope)
- ContinueAndDontEndStream status semantics (out of MVP scope)
- Per-route filter `disabled` flag honoring (out of MVP scope)

### Buffer overflow behavior
- decode-side: 413 local reply, hardcoded constant 1 MiB
  (filterBufferLimitBytes), enters encode chain
- encode-side: connection reset (H1: close; H2: RST_STREAM)
- per_connection_buffer_limit_bytes / per_request_buffer_limit_bytes
  silently ignored — extends ADR-0041 silent-ignore set

### Async resume mechanics
- StopIteration parks dispatch goroutine on a per-stream resume channel
- ContinueDecoding/ContinueEncoding callbacks unblock the channel
- Single-goroutine-per-request iteration; no per-filter goroutine spawned
  by the framework
- ctx.Done() during pause aborts iteration; OnDestroy fires for cleanup

### Filter ordering
- http_filters[] declaration order on decode-side
- Reverse declaration order on encode-side (router last on decode →
  router first on encode)
- Last entry MUST be the router (terminal filter); errors at parse otherwise

### Applies to
- Phase 07.1 onward (HTTP filter framework)

### Does not yet apply to
- Network filter chain (still phase 02 minimal — TCP-proxy or HCM as single
  entry per ADR-0033)
- Listener filters (deferred to 07.2)
- FilterChainMatch beyond SNI (deferred to 07.2)
- Listener.default_filter_chain (deferred to 07.2)
- HTTP filter family implementations beyond cors + envoy_go_test
  (incrementally landed by §9 family phases)
```

### 7.2 Empirical-pin verbatim subsections

Following the 06.1 / 06.2 pattern, four short verbatim-evidence subsections live as `### Empirical evidence (cors preflight)`, `### Empirical evidence (filter ordering)`, `### Empirical evidence (413 overflow)`, `### Empirical evidence (sendLocalReply entry)`. Each is a 5–15-line Envoy-scrape excerpt scraped at SPEC time and pinned at the ENVOY_TARGET.md SHA.

### 7.3 New equivalence-matrix row

Appended to the `## Equivalence Matrix` table near line 9:

```
| Dimension              | Equivalence claim                                        | Allow-list / tolerance                |
|------------------------|----------------------------------------------------------|---------------------------------------|
| HTTP filter chain      | Per-request equivalence on cors preflight + actual-     | Differential covers cors only.        |
|                        | request response shapes (status + header set + body)    | envoy.filters.http.envoy_go_test      |
|                        | between envoy-go and reference Envoy. Filter            | excluded (test-only). All other       |
|                        | iteration order, sendLocalReply encode-chain entry,     | filters in the §9 family are          |
|                        | and 413 overflow shape are verbatim-pinned at the       | future-phase scope.                   |
|                        | ENVOY_TARGET SHA.                                       |                                       |
```

### 7.4 ADR-0040, ADR-0042 supersession notes

The existing `## HTTP/1.1` and `## HTTP/2` sections of BEHAVIOR_CONTRACT.md mention HTTP-filter chain shape (e.g., the "exactly [router]" rule from ADR-0042). Those references are updated to "non-empty; last entry is router" — partial supersession, with a forward pointer to `## HTTP filter chain` for the full discipline.

The phase-04 BEHAVIOR_CONTRACT mention of router-as-direct-call is removed in favor of router-as-terminal-filter discipline. Existing per-filter reasoning in `## HTTP/1.1` `### Asserted equivalence` is preserved verbatim — the wire-level claims don't change; only the dispatch path that produces those wire bytes does.

---

## 8. ADRs anticipated

The planning session (`superpowers:writing-plans` for SPEC.md, then PLAN.md) finalizes count + numbering. **Seven ADRs anticipated for 07.1**:

1. **Phase-07 planner-time split (07.1 + 07.2)** — mirrors ADR-0045 (phase 05 split) and the phase-06 split. Documents the disjoint-scope rationale (HCM-internal vs listener-scope) + 07.1-first ordering rationale (07.1 unblocks §9 family phases). (Q1 outcome.)
2. **HTTP filter iteration protocol shape** — Envoy-faithful subset with async-resume; narrow method set (`DecodeHeaders/Data/Trailers`, `EncodeHeaders/Data/Trailers`); status enums settled (`Continue / StopIteration` for headers+trailers, `Continue / StopIterationAndBuffer / StopIterationNoBuffer` for data); explicitly out-of-MVP (`ContinueAndDontEndStream`, watermark variants, `Encode1xxHeaders`, metadata frames). Two-step factory pattern (config-time + per-request). Async-resume via per-stream buffered channel; single-goroutine-per-request iteration. (Q3 outcome.) **Supersedes ADR-0040 totally.**
3. **HTTPRegistry — threaded constructor map, no package-global** — explicit threading discipline (mirrors `*stats.Registry` LBP-1 + `*cluster.Manager`); freeze-after-boot invariant; `Register` panic on post-Freeze; `Lookup` by `typed_config.type_url`. Rejection of `init()`-based global registration. (Q4 outcome.)
4. **typed_per_filter_config 3-tier merge model** — Route > VirtualHost > RouteConfiguration; most-specific-override (not field-level merge); lazy-cache on first `RequestRouteConfig()` call per request. Build-time validation: keys ⊆ chain filter names. Honored at parse-time (partial supersession of ADR-0041's "all silently-ignored fields" list — `typed_per_filter_config` moves to honored). (Q2 outcome.)
5. **Trivial filter set: cors (real, differential) + envoy_go_test (test-only, structural)** — split rationale (differential equivalence vs full state coverage); iteration-state coverage attribution table. The `envoygotest` proto schema is envoy-go-only (hand-rolled, not in upstream go-control-plane). (Q5 outcome.)
6. **sendLocalReply encode-chain semantics** — synthesized response enters encode chain at filter[len-1] (reverse iteration entry); first-call-wins via `sync.Once`; second-call-after-encode-started is a no-op + log; encode-side iteration is full (every encode-side filter runs). Empirical-pin obligation on the actual filter-index entry point at SPEC time. (Q6 outcome.)
7. **Body buffer cap — hardcoded 1 MiB; 413 on decode overflow; reset on encode overflow** — `filterBufferLimitBytes = 1 << 20` constant; decode-side overflow synthesizes 413 → encode chain; encode-side overflow → connection reset (H1 close, H2 RST_STREAM). The configurable knobs (`per_connection_buffer_limit_bytes`, `per_request_buffer_limit_bytes`) **silently ignored** — extends ADR-0041 silent-ignore set (parallel to the 06.2 amendment for access_log fields). (Q7 outcome.)

**Inline supersessions** (recorded in the new ADRs above, not as separate ADRs):

- **ADR-0040 totally superseded** by ADR (2) above. Phase-04's "router invoked by direct function call inside HCM connection loop" is replaced by router-as-terminal-filter via the iteration protocol.
- **ADR-0042 partially superseded** by ADR (2). Phase-04's "exactly [router]" rule becomes "non-empty; last entry must be router". Lower bound stays; upper bound lifted.
- **ADR-0041 amended** by ADR (4) and ADR (7). `typed_per_filter_config` moves from silent-ignored to honored. `per_connection_buffer_limit_bytes` + `per_request_buffer_limit_bytes` are added to the silent-ignored set.

(Phase 06.1 had 6 ADRs; 06.2 had 4. 7 sits at the high end — appropriate for a framework phase that supersedes prior ADRs and introduces a new protocol surface. The planning session may consolidate if any pair turns out to be the same decision in two clothes.)

---

## 9. Out-of-scope items deferred to later phases

| Item | Deferred to |
|---|---|
| Listener filters framework | **07.2** |
| `FilterChainMatch` fields beyond SNI (destination_port, prefix_ranges, source_*, ALPN/application_protocols) | **07.2** |
| `Listener.default_filter_chain` | **07.2** |
| Watermark backpressure events (`StopAllIterationAndWatermark`, etc.) | First HTTP-filter-family phase that demands them (likely a streaming-body filter like compression or buffer) |
| `ContinueAndDontEndStream` status | First filter that needs to extend a stream past its declared end (rare; family phase) |
| `Encode1xxHeaders` callback | First filter touching 1xx informational headers (e.g., a future `expect-continue` filter or upstream 100-Continue handling) |
| Metadata frames (`decodeMetadata` / `encodeMetadata` / `MetadataMap`) | xDS-metadata family or a filter that consumes per-request metadata (e.g., `header_to_metadata`'s metadata-write side fully wired) |
| Per-route filter `disabled` flag | First family phase where a route-scoped opt-out is needed |
| `per_connection_buffer_limit_bytes` config knob (Listener-scope) | A dedicated buffer-policy phase or first family phase that needs tuning |
| `per_request_buffer_limit_bytes` config knob (Route-scope) | Same |
| Filter chain hot-reload | xDS family |
| Network filter chain expansion (multi-entry network chain, network filter iteration) | Network-filters family phase (per ADR-0033 disjoint-protocol-layer disposition) |
| Router proto fields silently ignored at phase 04 (`dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`) | Each promotion lands in the phase that consumes the field |
| HTTP-filters family implementations beyond cors + envoy_go_test (header_manipulation, fault, jwt_authn, ext_authz, oauth2, csrf, buffer, lua, wasm, local_ratelimit, rbac, etc.) | BOOTSTRAP §9 HTTP-filters family — each filter brainstormed as its own phase |
| Field-level merge (vs override) for `typed_per_filter_config` | First family phase where partial-override semantics are demanded by an Envoy-equivalent test |
| Filter callback API extensions (`StreamInfo`, `connectionLocalAddress`, `route()`, `clusterInfo()`, `dispatchTimer`, etc. — Envoy's broader callback surface beyond the MVP set) | Incrementally grown by family phases; each addition lands an ADR |

---

## 10. Hand-off to writing-plans

Next session (lifecycle-state 1 → 2, skill `superpowers:writing-plans`) authors:

- `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` — master design summarizing 07.1 + 07.2 scope (mirrors `docs/envoy-go/phases/06-observability-baseline/SPEC.md` once it's authored, and the existing `phases/05-http-2/SPEC.md`).
- `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` — sub-phase SPEC for 07.1 derived from §§2-9 of this document, including the §2.6 empirical-pin obligations executed at SPEC-drafting time (verbatim Envoy v1.37.2 scrape evidence pinned for cors preflight, filter ordering, 413 overflow shape, and sendLocalReply entry filter index).
- `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` — sibling SPEC stub citing the master SPEC + this BRAINSTORM § "07.2 sub-phase" forward-looking notes (the `## 1. Scope decision` table is sufficient for the stub).
- **ROADMAP.md split:** row `07 | filter-chain-framework | 06 | planned` becomes parent `07 | filter-chain-framework | 06 | in-progress | 07.1, 07.2 | ...` with rows `07.1 | http-filter-framework | 06 | planned | | ...` and `07.2 | listener-chain-completion | 07.1 | planned | | ...`.
- After SPEC, lifecycle-state 1 → 2 with `next-skill: superpowers:writing-plans` (PLAN.md authoring) and `active-phase: 07.1-http-filter-framework`.

This BRAINSTORM.md is committed as the brainstorm-close artifact and is read-only history once the next session starts. Future sessions consult it as the authoritative record of the design decisions made here.
