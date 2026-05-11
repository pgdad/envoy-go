# Phase 15 Brainstorm — `envoy.filters.http.bandwidth_limit`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 15 (`http-filter-bandwidth-limit`), the EIGHTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, and `compressor` at phase 14). The next session (lifecycle-state 1 → 2 for phase 15, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10/11/12/13/14 precedent) authors `docs/envoy-go/phases/15-http-filter-bandwidth-limit/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-15-http-filter-bandwidth-limit-brainstorm`, branch `phase-15-http-filter-bandwidth-limit-brainstorm`, branched from master tip `f4ce582` (the phase 14 lifecycle-state-6 STATE.md-advance commit `phase 14 lifecycle-state-6: STATE.md advance (awaiting next planning)`). The phase 14 squash-merge commit `9df9a29` and its earlier SHA-fill follow-up `a3895b1` precede `f4ce582`; phase 14's PLAN squash `bdcb7c1` + PLAN SHA-fill `3af5d3a` are earlier still in the trunk history. `f4ce582` is the most recent master tip.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via 5-question dialogue (Q1 §9 family-row pick — `bandwidth_limit` chosen from the 11-candidate remaining list `jwt_authn / rbac / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control / bandwidth_limit / global_ratelimit`; Q2 direction MVP — `BOTH (REQUEST + RESPONSE)` chosen from `request-only / response-only / both / DISABLED-only-and-parse-reject`; Q3 body algorithm — `Path B-async = buffer-then-delayed-emit` chosen from `Path B-async / Path A streaming / Path B-blast`; Q4 per-route discipline — `5th canonical disabled-OR-override` chosen from `disabled-OR-override / merge-semantic / refusing-per-route`; Q5 field envelope — `4 consumed + 3 silent-ignored + per-route` chosen from `4+3 / slim 2+5 / strict-runtime-rejected`). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0134, where ADR-0129-ADR-0134 landed in phase 14), and the just-shipped phase 14 + phase 13 + phase 12 + phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 + 12 + 13 + 14 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/14-http-filter-compressor/BRAINSTORM.md` and `docs/envoy-go/phases/13-http-filter-buffer/BRAINSTORM.md` section-for-section, reframed for the bandwidth_limit scope and adapted for its specific surface area. Phase 15 sits in a structurally important position relative to the §9 family: it is the FIRST §9 family-row since phase 12 csrf to introduce **ZERO framework deltas** — neither decode-side nor encode-side primitives are added. The filter wholly composes against (a) phase-09 fault's async-resume primitive (`time.AfterFunc` + `cb.ContinueDecoding()` / `cb.ContinueEncoding()`); (b) phase-13 ADR-0128's decode-side body-buffering + synthetic-empty-terminal `RunDecodeData` framework deltas; (c) phase-14 ADR-0131's encode-side `OverwriteBody` framework primitive. The phase-13 + phase-14 primitives are thereby demonstrated as reusable load-bearing infrastructure rather than one-off filter accommodations — this is a load-bearing observation about the §9 family's framework-delta accretion shape (per the Q1 family-child selection rationale). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 15 — this brainstorm cold-started fresh from the §9 heading + the phase 14 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-11.

---

## 1. Mission and scope confirmation (15 only)

ROADMAP row `15 | http-filter-bandwidth-limit | 14 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 15 is the EIGHTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 14 squash-merge commit `9df9a29` (with PLAN SHA-fill at `3af5d3a` and lifecycle-state-6 STATE.md-advance at `f4ce582`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 62: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114); `csrf` shipped in phase 12 (`internal/filter/http/csrf/` per ADR-0120); `buffer` shipped in phase 13 (`internal/filter/http/buffer/` per ADR-0125); `compressor` shipped in phase 14 (`internal/filter/http/compressor/` per ADR-0129–ADR-0133). Phase 15 ships **the symmetric request-and-response bandwidth throttle** as the EIGHTH real filter — the canonical Envoy-style "rate-limit body throughput in kilobits-per-second" filter. The chosen branch + directory + Go-package identifier are all `bandwidthlimit` (single-token; matching the Envoy filter type-URL `envoy.filters.http.bandwidth_limit` modulo Go's underscore-stripping convention).

Phase 15 is also: (i) the FIRST §9 family-row since phase-12 csrf to introduce **ZERO framework deltas** — a load-bearing observation per Q1 selection rationale, codifying phase-13 ADR-0128 (decode-side primitives) + phase-14 ADR-0131 (encode-side `OverwriteBody` primitive) as reusable infrastructure rather than one-off accommodations; (ii) the FIRST §9 family-row to **symmetrically exercise BOTH** decode-side body-buffering AND encode-side `OverwriteBody` within the same filter — a pairing that demonstrates the decode/encode surface symmetry promised by phase-13/14's framework-delta sequence; (iii) the FIRST §9 family-row to layer an async-resume body-throttle on top of phase-13's body-buffering — the algorithmic structure is phase-09 fault's `time.AfterFunc` + `ContinueDecoding/Encoding` adapted to a per-body-byte-count throttle duration computation (vs. phase-09 fault's static delay or random injection); (iv) the THIRD §9 family-row using the disabled-OR-override 5th canonical per-route discipline (codified at ADR-0125 by phase 13 buffer; reused with WHOLESALE-not-merge semantic by phase 14 compressor; phase 15 reuses the same shape — see §2.4); (v) the FIRST §9 family-row whose per-route stats hypothesis is INDEPENDENT (mirroring phase-11 local_ratelimit's ADR-0117 INDEPENDENT-stats discipline rather than phase-12/13/14's SHARED-stats discipline), driven by the per-route token-bucket statefulness (each per-route override owns a fresh token bucket; see §2.5 + §5).

### 1.1 What 15 delivers as a self-contained whole

Phase 15 lands `envoy.filters.http.bandwidth_limit` (the canonical Envoy bandwidth-limit filter, request + response, symmetric throttle, single token bucket per direction per scope) under the 07.1 framework. **Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):**

1. **New `internal/filter/http/bandwidthlimit/` package** owning the filter implementation. Package directory + Go package identifier are both `bandwidthlimit` (single token; mirrors the `localratelimit/` precedent from phase 11; the `_` in the Envoy filter name does NOT propagate into the Go package identifier per ADR-0114 §2.1). Files mirror the `internal/filter/http/localratelimit/` shape from phase 11 (the precedent for token-bucket-stateful filters): `bandwidthlimit.go` (filter type + factory + decode + encode methods + per-route helper + filterStats struct + compiledConfig + token-bucket helpers + timer-cleanup hook), `bandwidthlimit_test.go` (unit tests), `fuzz_test.go` (the 19th fuzzer in the repo — `FuzzBandwidthLimitConfigParse`), `doc.go` (package overview + 4-consumed/3-deferred decomposition + per-route disabled-OR-override summary + Path B-async body-algorithm summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 9 entries after phase 14: `router.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains a tenth `httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`. `bandwidthlimit` inserts between `router` and `buffer` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing of `envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit`,** the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 7 top-level fields; phase 15 consumes 4 and silent-ignores 3. **Consumed (4 fields; per §2.5):**

   - `stat_prefix` (string) — emission-scope tag for the 6-counter/gauge stat surface (see §2.7). Empty default is permitted (mirrors local_ratelimit per phase-11 ADR-0114).
   - `enable_mode` (`EnableMode` enum, all 4 values: `DISABLED`, `REQUEST`, `RESPONSE`, `REQUEST_AND_RESPONSE`) — honored at parse + runtime per §2.2. `DISABLED` is full passthrough (no throttle); `REQUEST` activates decode-side throttle only; `RESPONSE` activates encode-side throttle only; `REQUEST_AND_RESPONSE` activates BOTH (symmetric).
   - `limit_kbps` (`UInt64Value`, REQUIRED) — throttle rate in kilobits-per-second. Filter-internal range check: `limit_kbps > 0` (zero-throttle is parse-rejected; §11 pin §9.P2 confirms exact PGV).
   - `fill_interval` (Duration; default 50ms per Envoy v1.37.2) — token-bucket fill cadence. envoy-go-side filter-internal range check `fill_interval ∈ [20ms, 1s]` mirrors phase-11 local_ratelimit's `fill_interval` precedent per §11 pin §9.P5 confirms exact Envoy filter-internal bounds.

   **Silent-ignored at runtime (3 fields; behavior-divergence-windows documented per phase-12 csrf-style at BEHAVIOR_CONTRACT phase-15 forward-pointer notes):**

   - `runtime_enabled` (RuntimeFeatureFlag) — always-100%-active per phase-11/12/14 silent-ignore pattern (ADR-0117/ADR-0121/ADR-0130 silent-ignore-runtime-flag precedent).
   - `enable_response_trailers` (bool) — when true on Envoy, emits `x-envoy-bandwidth-rate-limit-latency-ms` + related trailers; envoy-go silent-ignores (always-no-trailers; trailer-emission primitive deferred to a future trailer-emission framework phase).
   - `response_trailer_prefix` (string) — couples to `enable_response_trailers`; silent-ignored.

4. **Per-route TPFC: `disabled` boolean OR `BandwidthLimitPerRoute` wholesale-override (5th canonical disabled-OR-override; ADR-0125 amendment §(xi)+).** Per the proto message `BandwidthLimitPerRoute` (§11 pin §9.P1 confirms exact proto shape — oneof of `disabled` + override sub-message is the brainstorm hypothesis; §9.P1 may amend), per-route entries carry a oneof with two cases: (a) `disabled: true` — the filter is wholly inactive on this route, no throttle, no counter increments, body forwards as-is to the wire; (b) `bandwidth_limit: <BandwidthLimit>` — a WHOLESALE override of the listener-level BandwidthLimit message (NOT a merge; mirrors phase-13 buffer + phase-14 compressor wholesale-override per ADR-0073 + ADR-0125). The per-route override carries its own `stat_prefix`, `enable_mode`, `limit_kbps`, `fill_interval`; if it owns its own `stat_prefix` it emits to an INDEPENDENT counter namespace (per §2.5 + §5 INDEPENDENT-stats hypothesis). Both shapes are honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (disabled OR override) drives the disposition. **Phase 15 is the THIRD row to use the disabled-OR-override discipline** codified by ADR-0125 (phase 13 buffer FIRST; phase 14 compressor SECOND). ADR-0125 gains an in-place amendment paragraph §(xi)+ confirming the pattern's third reuse + noting the WHOLESALE-not-merge override semantic for the entire BandwidthLimit message (the narrow-override-surface precedent stays bound to phase 14's `ResponseDirectionOverrides`). NO new per-route ADR for phase 15.

5. **Filter-callback shape: BOTH `StreamDecoderFilter` + `StreamEncoderFilter` on the SAME `*filter` instance.** The filter implements BOTH interfaces simultaneously (mirrors phase-14 ADR-0129 same-`*filter` precedent — compressor is `StreamEncoderFilter` primarily plus a minimal `StreamDecoderFilter` surface for `Accept-Encoding` stripping; phase 15 generalizes to symmetric BOTH). Static blank-identifier compile-time checks for both interfaces. The decode-side surface: `DecodeHeaders` resolves per-route + caches effective config + sets `f.requestActive` per `enable_mode`; `DecodeData` buffers + throttles per §2.3; `DecodeTrailers` pass-through. The encode-side surface: `EncodeHeaders` cache effective config + sets `f.responseActive`; `EncodeData` buffers + throttles per §2.3; `EncodeTrailers` pass-through. `OnDestroy` MUST cancel any pending `time.AfterFunc` timer (per §4 timer cleanup discipline).

6. **Body algorithm: Path B-async = buffer-then-delayed-emit (Decision #3 → ADR-0137).** Per Q3 = "Path B-async (buffer-then-delayed-emit)", the algorithm:

   - On the decode path (when `f.requestActive`): `DecodeHeaders` returns `HeaderContinue` (no header mutation; body throttle is body-only). `DecodeData(data, endStream)` BUFFERS `data` into `f.requestBody []byte`; if NOT `endStream`, returns `DataStopIterationAndBuffer` (no upstream-forward yet). On `endStream=true`, the filter computes `throttle_duration = (len(f.requestBody) * 8 / 1000) / limit_kbps seconds`, then arms `time.AfterFunc(throttle_duration, func() { cb.ContinueDecoding() })`. The framework's `RunDecodeData` returns; the request is paused at the filter-chain checkpoint. When the timer fires, `cb.ContinueDecoding()` resumes the chain, which forwards the buffered body upstream all-at-once.
   - On the encode path (when `f.responseActive`): symmetric — `EncodeHeaders` returns `HeaderContinue`. `EncodeData(data, endStream)` BUFFERS into `f.responseBody`; on `endStream=true`, computes throttle, arms `time.AfterFunc`, returns `DataStopIterationAndBuffer`; timer fires → `cb.ContinueEncoding()` resumes; framework writes the buffered body to wire. The body bytes themselves are forwarded via the existing `cb.OverwriteBody` primitive (phase-14 ADR-0131) IF needed; for the symmetric "buffer same bytes; emit later" pattern, the body slice is structurally unchanged — `cb.OverwriteBody` may or may not be invoked depending on whether the framework's buffer-and-forward shape requires the explicit replace. SPEC author confirms exact primitive use at framework-survey step (anticipated: zero invocation; the framework's `DataStopIterationAndBuffer` + `ContinueEncoding` pair already returns the buffered bytes; `OverwriteBody` is not required for the same-bytes case).
   - **NO new framework primitive.** The whole algorithm composes against (a) phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding/Encoding`; (b) phase-13 ADR-0128's decode-side body-buffering machinery (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation); (c) phase-14 ADR-0131's encode-side `OverwriteBody` IF needed (anticipated: not needed; body bytes are unchanged).

   **Wire-shape divergence from reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-15 forward-pointer notes): Envoy's bandwidth_limit emits chunks AT THE RATE-PACED CADENCE — small chunks per `fill_interval` tick, distributed across the throttle window. envoy-go's MVP emits all bytes in ONE blast at the end of the throttle window. Total-throttle-time is observably equivalent within ±10-50ms tolerance (the request takes the same wall-clock duration regardless of chunk-timing); chunk-timing observably diverges (Envoy: paced chunks during throttle window; envoy-go: silence then dump). For consumers that don't depend on intra-throttle chunk timing (typical HTTP clients), the behavior is byte-equivalent. §11 empirical pin §9.P8 confirms Envoy's exact wire shape under fixture conditions. ADR-0137 records the divergence + the explicit forward-pointer to a future streaming-framework phase that would land symmetric rate-paced chunk-emit primitives (mirroring phase-14 ADR-0131 §(vi) future encode-side streaming phase forward-pointer).

7. **Stat surface — 46→52-name extension (Decision #5 stat-surface hypothesis → ADR-0138).** **6 new counters + gauges** under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`, extending the phase-14 46-name table (phase 14 took the table from 29 → 46 via 17 new counters per ADR-0132 — and per phase-14 SPEC §1.1 amendment 3 did NOT introduce a new SN flattening rule, REUSING the existing SN2 HCM-stat-prefix rule). Hypothesized names (per Envoy v1.37.2 docs `configuration/http/http_filters/bandwidth_limit_filter`):

   - `request_enabled` — counter; increments once per stream-with-active-request-throttle.
   - `request_enforced` — counter; increments once per stream where the request-side throttle actually engaged (body large enough that throttle_duration > 0).
   - `request_pending` — gauge; tracks the count of streams currently waiting on a request-side timer.
   - `response_enabled` — counter; symmetric to request side.
   - `response_enforced` — counter; symmetric.
   - `response_pending` — gauge; symmetric.

   Histogram stats (`request_allowed_size`, `request_incoming_size`, `response_allowed_size`, `response_incoming_size`) DEFERRED per phase-06.1 ROADMAP-row "counters + gauges only — histograms deferred" baseline. Reference Envoy emits these four histograms; envoy-go MVP emits only the 6 counter/gauge stats (see §8.2 deferral).

   **Stat namespace + Prometheus tag-extractor:** §11 empirical pin §9.P10 confirms exact stat path + tag pattern. Hypothesis: `http.<HCM stat_prefix>.<filter stat_prefix>.<counter>` (same shape as phase-11 local_ratelimit per ADR-0117 + SN9 per ADR-0118). The SN-rule hypothesis is **SN2 reuse** (the existing HCM-stat-prefix rule). IF §11 pin §9.P10 finds a filter-specific tag-extractor is needed (e.g., to disambiguate request-vs-response counter axes via a `direction=request|response` tag), a new SN10 rule is introduced (next-after-SN9 per phase-11 ADR-0118; phase-14 declined to introduce SN10 per its §1.1 amendment 3, leaving the slot free for phase-15 or a later phase). Brainstorm hypothesis is SN2 reuse with the direction encoded in the counter-name suffix (no need for a separate tag); SPEC pin resolves.

8. **No new framework primitive on either side.** Phase 15 reuses (a) phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume pattern; (b) phase-13 ADR-0128's decode-side body-buffering + synthetic-empty-terminal `RunDecodeData` framework deltas; (c) phase-14 ADR-0131's encode-side `OverwriteBody` IF needed (anticipated: not needed); (d) the 3-tier `PerRouteConfig.Resolve` from phase 07.1; (e) the existing `SendLocalReply` from fault/local_ratelimit/csrf/buffer precedent (NOT used directly — bandwidth_limit never short-circuits to a local reply; this contrasts with phase 11's local_ratelimit which DOES short-circuit). Phase 15 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor, NO new HCM hook, NO new chain-iteration disposition. This is the FIRST §9 row since phase-12 csrf to introduce ZERO framework deltas — load-bearing observation about the §9 family's framework-delta accretion shape (see §3 below).

**Plus three artifact-level deliverables:**

9. **Differential fixture `0017-http-bandwidth-limit`** under `test/fixtures/0017-http-bandwidth-limit/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising six scenarios per §6 below. The fixture reuses `test/helpers/echobackend/` from phase 14 (the real-backend echo helper) for the request-side throttle scenarios (the response is echoed back, allowing the driver to assert both upstream-arrival-time AND downstream-response-time within tolerance). The fixture asserts response status, **body byte-equivalent** (bandwidth_limit does not transform bytes; only paces them — unlike phase-14 compressor's decompress-and-compare body-assertion per ADR-0133), counter deltas via `/stats/prometheus` scrape equivalence, per-route-tier independent disposition (both `disabled` and `bandwidth_limit` override shapes exercised), AND **total-throttle-time tolerance** assertions (±50ms wall-clock equivalence between Envoy and envoy-go for each throttle scenario; mirrors phase-11 local_ratelimit's `±10ms refill-after-fill_interval` tolerance discipline).

10. **`BEHAVIOR_CONTRACT.md` 4-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.cors`, `### envoy.filters.http.fault`, `### envoy.filters.http.header_mutation`, `### envoy.filters.http.local_ratelimit`, `### envoy.filters.http.csrf`, `### envoy.filters.http.buffer`, `### envoy.filters.http.compressor` subsections): a NEW `### envoy.filters.http.bandwidth_limit` subsection covering the 4-consumed / 3-ignored field map, the bidirectional throttle semantics (decode + encode), the Path B-async body algorithm + total-throttle-time-equivalent / chunk-timing-divergent wire-shape divergence-window from Envoy, the per-route disabled-OR-override semantics, the INDEPENDENT-stats hypothesis for per-route. Plus the 46→52-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0017 with per-scenario tolerance discipline (total-throttle-time ±50ms). Plus a NEW `### Phase 15 forward-pointer notes` subsection under `## Forward-pointer notes` covering the ~8-item deferral list (per §8 below).

11. **Anticipated 5 ADRs (ADR-0135 through ADR-0139)** per §7 below. ADR-0134 is the highest-numbered ADR landed in phase 14; ADR-0135 is the next-free.

### 1.2 What 15 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12/13/14 precedent; deferrals are 8 items grouped by family-coupling, slightly more than phase 14's 8-item list per the bandwidth_limit's larger silent-ignored-field surface around trailers). Summary: `enable_response_trailers` + `response_trailer_prefix` trailer-emission + the underlying trailer-emission framework primitive; 4 histogram stats; `runtime_enabled` RuntimeFeatureFlag (always-100%-active); chunked-rate-paced wire-shape (Path A, future streaming-framework phase); per-route `stat_prefix` emission-scope (INDEPENDENT-vs-SHARED, depends on §11 pin); `fill_interval` values outside `[20ms, 1s]` envoy-go-only parse-rejection (filter-internal cap, future high-resolution-timer phase); multi-listener BandwidthLimit chaining (NOT anticipated by reference Envoy); `BandwidthLimitPerRoute` non-standard proto field shapes if §11 pin §9.P1 reveals them. None are blockers for closing row 15 phase-done.

### 1.3 Phase-done as a §9 family-row landing

Phase 15's phase-done commit closes ROADMAP row `15` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 15 is the EIGHTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, 12-csrf, 13-buffer, 14-compressor). The next §9 family-row will be numbered `16` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 15 is **single-row at brainstorm time** — a cohesive ~1100-1400 LoC implementation slice covering a symmetric request+response throttle — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split would be:

- **15.1 = response-side throttle MVP**: the filter type + factory + `EncodeHeaders` + `EncodeData` body-buffer-and-throttle (encode-side only) + `compiledConfig` parsing (full 4-consumed field envelope but `enable_mode` only honoring `DISABLED` + `RESPONSE` values, with `REQUEST` + `REQUEST_AND_RESPONSE` parse-rejected envoy-go-side until 15.2 lands) + 3-counter/gauge stats (response_enabled, response_enforced, response_pending) + per-route disabled-OR-override + ADR-0125 amendment paragraph. Differential fixture covers response-only scenarios (1, 4, 5, 6 from §6.2).
- **15.2 = request-side throttle + symmetric REQUEST_AND_RESPONSE**: per-route + listener-level `enable_mode` REQUEST + REQUEST_AND_RESPONSE activation; `DecodeData` body-buffer-and-throttle; 3 more counter/gauge stats (request_enabled, request_enforced, request_pending); fixture scenarios 2 + 3.

This split mirrors phase 10 + 11 + 12 + 13 + 14's anticipated-but-unused split. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the LoC estimate (~280-330 impl + ~500-700 production for the timer-wired body throttle on both sides + ~500-600 tests + ~80 fuzzer + ~150-200 fixture-Go-driver/backend + ~150 fixture-yaml/README = ~1100-1400 total when including yaml configs and README; ~500-700 if counting Go production code alone). Task count estimate: ~12-16 tasks. Both estimates remain comfortably under ADR-0045's 1500 LoC / 25 task split-trigger upstream of either accounting. Phase 15 is structurally smaller than phase 14 at the proto-surface level (4 consumed fields vs. 8) and smaller at the algorithmic level (token-bucket-via-throttle-duration arithmetic + `time.AfterFunc` vs. compressor's gzip-encoder + Vary/Content-Encoding header injection + skip-predicate matrix); the symmetric request+response duplication doubles the surface relative to the asymmetric phase-14 compressor's response-only.

### 1.5 Seed-stub alignment

Like phases 09, 10, 11, 12, 13, and 14, phase 15 has NO sibling SPEC stub — phase 15 enters fresh after the phase 14 close. The §9 family-children list at ROADMAP line 62 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`, plus the future `envoy.filters.http.decompressor` companion). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 15 has NO such branch. The brainstorm dialogue (Q1-Q5 over the user-Claude exchange) was sufficient to settle filter pick + direction mode + body algorithm + per-route discipline + field envelope without preliminary scoping notes. This matches the phase 09 / 10 / 12 / 13 / 14 cold-start precedent.

### 1.7 Phase 15's relationship to phase 13 ADR-0128 + phase 14 ADR-0131 framework deltas

Phase 13 introduced two framework primitives at `internal/filter/hcm/connection.go` per ADR-0128: synthetic empty-terminal `RunDecodeData` on chunked-body EOF + post-body Content-Length reconciliation propagating filter-set Content-Length into `req.ContentLength`. Phase 14 introduced the encode-side `OverwriteBody` primitive at `EncoderFilterCallbacks` per ADR-0131. Phase 15 **consumes BOTH** — symmetrically. The decode-side body-buffering machinery from ADR-0128 is what makes Path B-async feasible on the request side (the filter sees the accumulated `req.Body` at `DecodeData(endStream=true)` time, exactly as phase-13 buffer does); the encode-side `OverwriteBody` from ADR-0131 is what makes Path B-async feasible on the response side IF the body slice needs replacement (anticipated: not needed; the filter buffers the same bytes and re-emits them unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding`). This is the FIRST §9 row to consume BOTH framework-delta sets simultaneously — load-bearing demonstration that phase-13 + phase-14 framework deltas are not one-off filter accommodations but reusable infrastructure (per Q1 selection rationale).

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 5 decisions below are the phase-15-specific design choices reached during the Q-dialogue. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 Family-child selection: `bandwidth_limit` *(Decision #0 → ADR-0135 rationale)*

Per Q1 = "bandwidth_limit chosen from §9 remaining unbrainstormed list", the selection criteria per ADR-0106(e):

- **Coherence ★★★★★ with phase 14 just-shipped artefacts.** Phase 14 introduced the encode-side `OverwriteBody` primitive (ADR-0131). Phase 13 introduced the decode-side body-buffering primitives (ADR-0128). Phase 15 bandwidth_limit exercises BOTH simultaneously (symmetric request + response throttle). This is the FIRST §9 row to coherently consume the decode + encode framework-delta sequence.
- **Upstream-dependency readiness.** Pure-Go implementation. Reuses phase-11 Option-A token-bucket primitive (the rate-arithmetic underlying `throttle_duration` computation is a thin wrapper over `body_size / limit_kbps` — no new dependency on `golang.org/x/time/rate` or similar; phase-11's hand-rolled atomic-counter approach is the model).
- **Scope-fit.** Estimated ~1100-1400 LoC; under ADR-0045's 1500-LoC split-trigger. Symmetric request+response surface doubles the algorithmic surface relative to asymmetric phase-14, but the per-direction algorithm is simpler (no skip-predicate matrix, no codec dispatch, no header mutation).
- **Family-child rotation.** The remaining 11 §9 family-children (`jwt_authn / rbac / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control / bandwidth_limit / global_ratelimit`) span widely-varying surface complexities. `bandwidth_limit` sits in the medium-complexity middle band — large enough to demonstrate phase-13/14 framework primitives' reusability, small enough to fit a single-row release. By contrast, `jwt_authn` / `ext_authz` / `ext_proc` / `oauth2` are auth-family filters with their own MUCH larger sub-trees (cryptographic config, side-channel HTTP/gRPC calls to external services); `lua` / `wasm` are scripting-engine families requiring whole sub-systems; `adaptive_concurrency` / `admission_control` are control-loop families with their own statefulness models; `rbac` is policy-engine family; `global_ratelimit` requires gRPC-side-channel rate-limit-service integration. `bandwidth_limit` is the cleanest fit.

ADR-0135 (the anticipated layout ADR for phase 15) documents the selection rationale + the FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 framework deltas.

### 2.2 Direction MVP: BOTH (REQUEST + RESPONSE) *(Decision #2 → ADR-0135 consequence)*

Per Q2 = "BOTH (REQUEST + RESPONSE); honor `enable_mode` enum all 4 values", the direction scope:

- `enable_mode = DISABLED` → full passthrough (no throttle; counters do NOT increment per §11 pin §9.P12); stream-trace observable as filter-present-but-inactive.
- `enable_mode = REQUEST` → decode-side throttle ACTIVE; encode-side passthrough.
- `enable_mode = RESPONSE` → decode-side passthrough; encode-side throttle ACTIVE.
- `enable_mode = REQUEST_AND_RESPONSE` → BOTH sides ACTIVE; symmetric throttle. (NOTE: the throttle durations on each side are INDEPENDENT — a 5 KiB request takes `5 * 8 / limit_kbps` seconds upstream-direction; a 5 KiB response takes another `5 * 8 / limit_kbps` seconds downstream-direction; the user sees their total wall-clock as `(req_size + resp_size) * 8 / limit_kbps` modulo upstream-server processing time.)

This is the symmetric exercise of phase-13 + phase-14 framework primitives. ADR-0045 release valve splits naturally into 15.1 (response-side per §1.4) + 15.2 (request-side) if SPEC finds > 1500 LoC.

### 2.3 Body algorithm: Path B-async (buffer-then-delayed-emit) *(Decision #3 → ADR-0137)*

Per Q3 = "Path B-async = buffer-then-delayed-emit", the algorithm:

**Decode path (when `f.requestActive`):**
1. `DecodeHeaders(headers, endStream)`: resolve per-route → cache `*compiledPerRoute` on filter state; determine `f.requestActive` per effective `enable_mode`; `Continue` (no header mutation).
2. `DecodeData(data, endStream)`:
   - Append `data` to `f.requestBody` buffer.
   - If NOT `endStream`: return `DataStopIterationAndBuffer` (the framework continues to accumulate further chunks; per phase-13 ADR-0128, the synthetic empty-terminal call eventually arrives even on chunked-body inputs).
   - If `endStream=true` AND `f.requestActive`: compute `throttle_duration_ns = (len(f.requestBody) * 8 * 1e9) / (limit_kbps * 1000)` nanoseconds. If `throttle_duration_ns < 1ms` (fast-passthrough threshold; see §6.2 scenario 4): emit no timer; just `Continue` (and increment `request_enabled +1` but NOT `request_enforced`). Otherwise: increment `request_enabled +1`, `request_enforced +1`, `request_pending +1`; arm `f.requestTimer = time.AfterFunc(throttle_duration, func() { f.requestPending.Dec(); cb.ContinueDecoding() })`; return `DataStopIterationAndBuffer`. The framework holds the request at the filter checkpoint until `ContinueDecoding` fires.
3. `DecodeTrailers`: pass-through (request trailers, if any, are pass-through; they queue behind the buffered body and resume with `ContinueDecoding`).

**Encode path (when `f.responseActive`):** symmetric — replace `Decode` → `Encode`, `request` → `response`. The encode-side timer reuses `time.AfterFunc` with `cb.ContinueEncoding()` as the callback.

**Throttle arithmetic:** `throttle_duration_seconds = (body_size_bytes * 8) / (limit_kbps * 1000)`. With `limit_kbps = 10` (10 kbps = 10000 bits/sec = 1250 bytes/sec) and `body_size = 10240` bytes: `throttle_duration = 10240 / 1250 = 8.192 seconds`. With `body_size = 100` and `limit_kbps = 10`: `throttle_duration = 100 / 1250 = 0.08 seconds = 80ms` (above fast-passthrough threshold; throttle arms). With `body_size = 10` and `limit_kbps = 10`: `throttle_duration = 10 / 1250 = 8ms` (below fast-passthrough threshold; throttle does NOT arm).

**Wire-shape divergence summary** (ADR-0137 records):
- envoy-go: `time.AfterFunc` delay then **single blast** of buffered body bytes at timer-fire. Chunk timing on the wire: 0 chunks during throttle window, 1 chunk at fire.
- Envoy: rate-paced chunks during throttle window — small chunks emitted at `fill_interval` cadence (e.g., 50ms × N chunks for an N×50ms throttle).
- Total-throttle-time on the wire: observably equivalent within ±10-50ms tolerance (the request body arrives upstream — or the response body arrives downstream — within the same wall-clock window).
- Body bytes: byte-equivalent (bandwidth_limit does NOT transform bytes; it only paces them).
- §11 empirical pin §9.P8 confirms Envoy's exact chunk-pacing behavior under fixture conditions.

**Forward-pointer to future streaming-framework phase** (per phase-14 ADR-0131 §(vi) pattern): when envoy-go grows symmetric encode-side + decode-side streaming primitives (`EmitChunk` / `ConsumeChunk`-style APIs that allow filters to emit partial body bytes interleaved with timer waits), phase 15's Path B-async naturally upgrades to Path A streaming. The forward-pointer phase ALSO lands the trailer-emission primitive needed for `enable_response_trailers` per §8.1 deferral.

### 2.4 Per-route discipline: 5th canonical disabled-OR-override *(Decision #4 → reuses ADR-0125; amendment paragraph §(xi)+)*

Per Q4 = "5th canonical disabled-OR-override per ADR-0125 (3rd usage after phase-13 buffer + phase-14 compressor)", the per-route TPFC shape:

```proto
message BandwidthLimitPerRoute {
  oneof override {
    bool disabled = 1;
    BandwidthLimit bandwidth_limit = 2;
  }
}
```

(§11 pin §9.P1 confirms exact proto shape; brainstorm hypothesis is the oneof-disabled-OR-override pattern.)

Two cases:
- **(a) `disabled: true`** — the filter is wholly inactive on this route. No throttle. No counter increments (the listener-level filter's counter scope is NOT affected by skipped per-route-disabled streams — they simply don't pass through the active-counter-emit path). Body bytes pass through to wire unchanged.
- **(b) `bandwidth_limit: <BandwidthLimit>`** — WHOLESALE override of the entire listener-level BandwidthLimit message (NOT a merge; the override's `stat_prefix`, `enable_mode`, `limit_kbps`, `fill_interval` REPLACE the listener-level values entirely). Mirrors phase-13 buffer's `BufferPerRoute.buffer` wholesale-override + phase-14 compressor's `CompressorPerRoute.overrides.response_direction_config` wholesale-override. The narrow-override-surface precedent stays bound to phase 14's `ResponseDirectionOverrides` (where the override could in principle have had fewer fields than the listener-level message); phase 15's BandwidthLimit override is the entire message, mirroring phase-13.

`parsePerRoute` flow:
1. If `disabled: true` → produce `*compiledPerRoute{disabled: true}`.
2. If `bandwidth_limit: { … }` → recursively run `buildCompiledConfig(bandwidth_limit)` → produce `*compiledPerRoute{disabled: false, overrideConfig: <built>}`.
3. Empty/oneof-not-set → reject at parse with envoy-go-own error `bandwidth_limit: per-route entry has no override field set; expected one of {disabled, bandwidth_limit}`.

Resolution flow at request time (mirrors phase-13/14):
1. `PerRouteConfig.Resolve(ctx)` → most-specific `*compiledPerRoute` for this route.
2. If `disabled=true` → set `f.passthrough=true`; `DecodeHeaders` / `DecodeData` / `EncodeHeaders` / `EncodeData` short-circuit pass-through.
3. If `disabled=false` AND `overrideConfig != nil` → use `overrideConfig` for the throttle (including its own `stat_prefix`, which drives INDEPENDENT stat namespace per §2.5 + §5); otherwise use the listener-level config.

**ADR-0125 in-place amendment paragraph §(xi)+** (NOT a new ADR; in-place per phase-13 ADR-0127-v2 + phase-14 ADR-0125 amendment precedent): noting phase 15 bandwidth_limit as the THIRD row to use disabled-OR-override + the WHOLESALE-not-merge semantic for the entire BandwidthLimit message (narrow-override-surface precedent stays bound to phase 14's `ResponseDirectionOverrides`). Authored at phase 15 SPEC drafting time per the ADR-0125 in-place-update precedent.

### 2.5 Field envelope: 4 consumed + 3 silent-ignored + per-route *(Decision #5 → ADR-0136)*

Per Q5 = "4 consumed + 3 silent-ignored", the field decomposition is:

**CONSUMED at runtime (4 fields):**
1. `BandwidthLimit.stat_prefix` (string) — emission-scope tag for the 6-counter/gauge stat surface; empty default permitted.
2. `BandwidthLimit.enable_mode` (`EnableMode` enum) — all 4 values honored (DISABLED / REQUEST / RESPONSE / REQUEST_AND_RESPONSE) per §2.2.
3. `BandwidthLimit.limit_kbps` (UInt64Value, REQUIRED) — throttle rate kilobits-per-second; filter-internal validation `limit_kbps > 0` (§11 pin §9.P2 confirms exact PGV).
4. `BandwidthLimit.fill_interval` (Duration; default 50ms) — token-bucket fill cadence; envoy-go-side filter-internal range check `fill_interval ∈ [20ms, 1s]` mirrors phase-11 local_ratelimit's precedent (§11 pin §9.P5 confirms exact Envoy bounds).

**SILENT-IGNORED at runtime (3 fields; behavior-divergence-windows documented per phase-12/14 csrf-style at BEHAVIOR_CONTRACT phase-15 forward-pointer notes):**
1. `runtime_enabled` (RuntimeFeatureFlag) — always-100%-active per phase-11/12/14 silent-ignore pattern. Divergence-window if Envoy-side enabled at < 100%. Fixture configs MUST set explicitly to 100%/HUNDRED on Envoy side (§11 pin §9.P6 confirms parse-time PGV requirement).
2. `enable_response_trailers` (bool) — always-no-trailers in envoy-go MVP; trailer-emission primitive deferred per §8.1. Operator divergence-window: configs setting `enable_response_trailers: true` see Envoy emit `x-envoy-bandwidth-rate-limit-*` trailers; envoy-go emits no trailers.
3. `response_trailer_prefix` (string) — couples to `enable_response_trailers`; silent-ignored.

**Per-route disabled+override per Decision #4** (§2.4 above).

**PARSE-REJECTED (envoy-go-only validation):** `limit_kbps == 0` or unset (REQUIRED; mirrors phase-13's `max_request_bytes > 1 MiB` envoy-go-only validation per ADR-0126); `fill_interval` outside `[20ms, 1s]` (envoy-go-only filter-internal cap; §8.6 deferral re-activation in future high-resolution-timer phase). Error wording: `bandwidth_limit: limit_kbps required and must be > 0` + `bandwidth_limit: fill_interval %v outside supported range [20ms, 1s]; phase-15 MVP enforces this filter-internal bound`.

### 2.6 Stat surface — 46→52-name extension *(Decision implicit in stat-surface hypothesis → ADR-0138)*

Per the stat-surface hypothesis from the brainstorm context: 6 counters+gauges per active stat_prefix:

- `request_enabled` (counter), `request_enforced` (counter), `request_pending` (gauge)
- `response_enabled` (counter), `response_enforced` (counter), `response_pending` (gauge)

Histogram stats (`request_allowed_size`, `request_incoming_size`, `response_allowed_size`, `response_incoming_size`) DEFERRED per phase-06.1 ROADMAP-row "counters + gauges only — histograms deferred" baseline (see §8.2 deferral). Reference Envoy emits these four histograms; envoy-go MVP emits only the 6 counter/gauge stats.

**Stat namespace + Prometheus tag-extractor:** §11 empirical pin §9.P10 confirms exact stat path + tag pattern. Hypothesis: `http.<HCM stat_prefix>.<filter stat_prefix>.<counter>` (same shape as phase-11 local_ratelimit per ADR-0117 + SN9 per ADR-0118). The SN-rule hypothesis is **SN2 reuse** — the existing HCM-stat-prefix tag-extractor handles this without amendment.

IF §11 pin §9.P10 finds a filter-specific tag-extractor is needed (e.g., a `direction=request|response` tag separated from the counter suffix), a new **SN10 rule** is introduced (next-after-SN9 per phase-11 ADR-0118; phase-14 declined to introduce a new SN rule per its §1.1 amendment 3, leaving SN10 as the next-free slot). Brainstorm hypothesis is SN2 reuse with direction encoded in the counter-name suffix.

**Per-route stats discipline: INDEPENDENT hypothesis** (§11 pin §9.P4 confirms). Rationale: per-route owns a fresh stateful token bucket + its own `stat_prefix` as own emission scope (mirrors phase-11 local_ratelimit's INDEPENDENT-stats per ADR-0117; DIVERGES from phase-12 csrf + phase-13 buffer + phase-14 compressor's SHARED-stats per ADR-0124/ADR-0125/ADR-0132). The stateful token-bucket-per-route precedent is the load-bearing motivator: each per-route override OWNS its own throttle state (different `limit_kbps`, different `fill_interval`, different pending-stream-count), so its stats MUST be tagged with its own `stat_prefix` to be observably distinct.

If §11 pin §9.P4 finds Envoy emits SHARED stats (per-route routed into listener-level counter namespace), envoy-go SPEC author updates ADR-0139 accordingly + amends BEHAVIOR_CONTRACT.

**Stat surface count summary:**
- Phase 11 (local_ratelimit): 22 → 26 names (4 new counters; SN9 introduced per ADR-0118).
- Phase 12 (csrf): 26 → 29 names (3 new counters; reuses HCM-stat-prefix tag-extractor per SN2).
- Phase 13 (buffer): 29 → 29 names (0 new counters; vacuous; reuses HCM-stat-prefix per ADR-0125).
- Phase 14 (compressor): 29 → 46 names (17 new counters per ADR-0132; NO new SN rule — REUSES SN2 per phase-14 SPEC §1.1 amendment 3; the BRAINSTORM-hypothesized SN10 was refuted at SPEC time).
- Phase 15 (bandwidth_limit): 46 → **52 names** (6 new counter+gauge mix; SN2 reuse hypothesis; SN10 introduced only if §11 pin §9.P10 demands).

---

## 3. Framework survey — ZERO framework deltas

Phase 15 is the FIRST §9 row since phase 12 csrf to introduce ZERO framework deltas. This is a load-bearing observation about the §9 family's framework-delta accretion shape (per Q1 selection rationale):

- Phase 07.1 cors: NEW framework (the entire HTTP-filter framework). N/A baseline.
- Phase 09 fault: introduced `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume primitives (the FIRST async-resume pattern in the framework).
- Phase 10 header_mutation: ZERO framework deltas (the FIRST zero-delta §9 row).
- Phase 11 local_ratelimit: ZERO framework deltas (reused phase-09 fault's async-resume primitives; introduced filterStats discipline + INDEPENDENT-per-route-stats per ADR-0117).
- Phase 12 csrf: ZERO framework deltas.
- Phase 13 buffer: TWO framework deltas (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation per ADR-0128). FIRST §9 row to break the zero-delta invariant.
- Phase 14 compressor: ONE framework delta (`EncoderFilterCallbacks.OverwriteBody` per ADR-0131). The encode-side mirror of ADR-0128.
- **Phase 15 bandwidth_limit: ZERO framework deltas.** Reuses phase-09 fault's async-resume + phase-13 ADR-0128's decode-side body-buffering + phase-14 ADR-0131's `OverwriteBody` (if needed; anticipated: not needed). The FIRST §9 row since phase 12 csrf to introduce no new primitives.

**What Path A streaming throttle would have required if Path A had been chosen** (deferral context for §8.4):

If the user had picked Path A (true rate-paced chunk-emit, byte-by-byte equivalence with Envoy's wire shape), envoy-go would have needed:

- A new `EncoderFilterCallbacks.EmitChunk(b []byte)` API allowing filters to emit partial body bytes interleaved with timer waits. Mirrors phase-14 ADR-0131 §(vi) forward-pointer.
- A new `DecoderFilterCallbacks.ConsumeChunk(b []byte)` symmetric primitive on the request side.
- HCM machinery to invoke `RunDecodeData` / `RunEncodeData` chunk-by-chunk rather than once with the full body. Currently `connection.go:467-475` invokes `RunEncodeData` ONCE with the full body; chunk-by-chunk would require a new iteration disposition.
- `writeH1Reply` chunked-output mode (the same delta phase-14 ADR-0131 §(vi) forward-points to).
- An estimated ~150-200 LoC framework delta — large enough to constitute its own phase per ADR-0045 split-trigger.

The user picked Path B-async (Q3) precisely to AVOID this large framework delta and keep phase 15 a focused single-row filter landing. The forward-pointer is recorded in ADR-0137; the actual primitives land in a future streaming-framework phase.

**What is reused** (already-on-disk primitives the filter composes against):
- `time.AfterFunc(d, func() {...})` standard library — used by phase-09 fault for `delay.fixed_delay`; phase-15 bandwidth_limit uses identically with a computed `d` rather than a config-static `d`.
- `cb.ContinueDecoding()` / `cb.ContinueEncoding()` callbacks — used by phase-09 fault to resume after the delay timer; phase-15 uses identically.
- Decode-side body-buffering machinery (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation) per ADR-0128 — used by phase-13 buffer to count bytes against `max_request_bytes`; phase-15 uses to accumulate the full request body before computing throttle.
- Encode-side `OverwriteBody` per ADR-0131 — used by phase-14 compressor to replace `resp.Body` with compressed bytes; phase-15 uses IF a body-slice-replacement is needed (anticipated: not needed; the buffered bytes ARE the original bytes, re-emitted unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding`).
- 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) per phase 07.1.

**No filter-chain ordering surgery.** Phase 15 bandwidth_limit's filter-chain position is up to the operator (matches phase-11 local_ratelimit's flexibility). Suggested ordering: bandwidth_limit BEFORE cors (so the throttle applies to the cors-mutated headers' body bytes) and BEFORE compressor (so the throttle applies to the compressed body bytes — when both filters are present, the smaller compressed body has a shorter throttle).

---

## 4. Per-stream timer + cleanup discipline

Phase 15 introduces per-stream `*time.Timer` lifecycle management. The discipline:

**Timer lifecycle:**
- `time.AfterFunc(d, fn)` returns a `*time.Timer`. The filter stores it at `f.requestTimer` (decode side) and `f.responseTimer` (encode side) per-filter-instance (per-stream).
- Timer fires → `fn()` runs in a goroutine. `fn()` invokes `cb.ContinueDecoding()` (or `ContinueEncoding`) which resumes the chain.

**Cleanup on `OnDestroy`:**
- The HTTPFilter interface includes `OnDestroy()` called when the stream is being torn down (connection close, request abort, timeout). Phase 15's `OnDestroy` MUST call `f.requestTimer.Stop()` and `f.responseTimer.Stop()` if non-nil — preventing the timer goroutine from invoking `cb.ContinueDecoding/Encoding` on a destroyed callback handle.
- `Stop()` returns a bool indicating whether the timer was active when stopped. The filter does NOT need to consult this bool; the cleanup is idempotent (Stop on an already-fired or already-stopped timer is a no-op).

**Race-safety considerations:**
- Timer goroutine vs. OnDestroy race: the timer may fire JUST before OnDestroy is called. If the timer-fire callback has already invoked `cb.ContinueDecoding/Encoding` and that has progressed into framework code that holds the callback handle valid, OnDestroy completes cleanly. If the timer-fire callback is mid-flight when OnDestroy is invoked, the framework's callback-handle invalidation must be safe (the chain.go RunDecodeData / RunEncodeData wrapper is the dispatch point — SPEC author confirms at framework-survey step).
- Phase-09 fault's existing timer pattern is the precedent (fault arms `time.AfterFunc` for `delay.fixed_delay` injection; phase-09's cleanup pattern is the reference). Brainstorm hypothesis: phase-15 reuses phase-09's exact pattern verbatim — including the OnDestroy timer.Stop() call.

**Goroutine-leak prevention:**
- `time.AfterFunc` is documented to NOT leak goroutines on Stop(); the timer's goroutine exits cleanly after Stop().
- The fundamental requirement is that EVERY `time.AfterFunc(...)` call paired with a stored timer reference MUST have a corresponding `Stop()` call on a teardown path. Phase 15's `OnDestroy` is that path.
- Test-discipline: `go test -race` on the bandwidth_limit package + an `OnDestroy`-fires-mid-throttle integration test scenario validates no goroutine leak.

**No new framework primitive for timer management.** Phase-09 fault already validated the `time.AfterFunc` + `OnDestroy.Stop()` pattern; phase-15 reuses without amendment.

---

## 5. Per-route discipline — INDEPENDENT-stats hypothesis

Per §2.5 + §2.6, per-route stats are hypothesized INDEPENDENT (each per-route override emits to its own `stat_prefix` counter namespace, NOT the listener-level scope). Rationale:

- **Stateful per-route token bucket.** Each per-route override OWNS its own throttle state — a fresh effective `limit_kbps`, `fill_interval`, and pending-stream-count. The listener-level token bucket and the per-route token bucket are LITERALLY DIFFERENT bucket-state objects (independent `request_pending` gauges; independent throttle-arming decisions). Mirrors phase-11 local_ratelimit per ADR-0117 (each per-route override = fresh token bucket = own stat namespace).
- **Own stat_prefix.** The override's `BandwidthLimit.stat_prefix` field is a first-class config knob — the operator can choose to set it to anything (including the same string as the listener-level for SHARED-emission). Brainstorm hypothesis: the override `stat_prefix` is honored as a NEW emission-scope tag.
- **Counter-arithmetic observability.** A 100-stream load against a route with a per-route override should produce 100 increments on the override's `request_enabled` counter, NOT 100 increments on the listener-level's counter. This is what an observability dashboard operator expects (per-route counters distinguish routes).

**Divergence from phase-12/13/14:** Those filters' per-route stats SHARED with listener-level because their per-route overrides are STATELESS (compressor's per-route override just changes effective config knobs; no fresh state object). Phase-15 bandwidth_limit's stateful-override matches phase-11 local_ratelimit's stateful-override; both INDEPENDENT.

**§11 empirical pin §9.P4 confirms or refutes** the INDEPENDENT hypothesis. If Envoy v1.37.2 emits SHARED stats (per-route routes into listener-level counter namespace), envoy-go SPEC author either (a) matches Envoy (SHARED) and amends ADR-0139 accordingly + amends BEHAVIOR_CONTRACT to note the per-route stat_prefix is honored at parse but ignored at emission; or (b) elects to DIVERGE from Envoy on this axis (INDEPENDENT — the more useful behavior) and documents the divergence-window per phase-12/14 style. Brainstorm position: INDEPENDENT is the operationally-correct shape regardless of Envoy's choice; the SPEC author's pin is to ratify or document the divergence.

**ADR-0139** (anticipated): codifies the INDEPENDENT-vs-SHARED resolution per the empirical pin.

---

## 6. Differential fixture (`0017-http-bandwidth-limit`)

### 6.1 Topology

Two listeners + two clusters (matches phase 11/12/13/14 fixture topology):

- **Listener `l_test_a`** (TCP plaintext on port `<envoy-go-test-port>` for envoy-go side; matching port on Envoy side per the `0016` template). Hosts an HCM with one filter-chain; the chain has filters: `bandwidth_limit → router`. Listener-level config is `BandwidthLimit{stat_prefix: "default", enable_mode: REQUEST_AND_RESPONSE, limit_kbps: 10, fill_interval: 50ms}`. Routes:
  - Route `/echo-response`: `direct_response` with body `<10 KiB deterministic ASCII payload>` and `content-type: application/octet-stream`. Default-route; exercises response-side throttle in scenario 1.
  - Route `/echo-request`: routes to backend cluster `c_backend_b` via real-backend echo (reuses `test/helpers/echobackend/` from phase 14). Exercises request-side throttle in scenario 2.
  - Route `/echo-both`: routes to backend echo with request body 5 KiB; backend echoes back 5 KiB. Exercises symmetric REQUEST_AND_RESPONSE in scenario 3.
  - Route `/echo-tiny`: `direct_response` with body `<100 bytes>`. Exercises fast-passthrough (throttle_duration < 1ms threshold) in scenario 4.
  - Route `/echo-disabled`: per-route TPFC `BandwidthLimitPerRoute{disabled: true}`. Exercises per-route disabled in scenario 5.
  - Route `/echo-override`: per-route TPFC `BandwidthLimitPerRoute{bandwidth_limit: {stat_prefix: "override", enable_mode: RESPONSE, limit_kbps: 100, fill_interval: 50ms}}`. Body `<10 KiB>` direct_response. Exercises per-route override with own stat_prefix + tighter throttle in scenario 6.

- **Listener `l_test_b`** + cluster `c_backend_b`: echo-backend cluster pair (real upstream-echo backend from phase 14's `test/helpers/echobackend/`). Used as the routing target for routes `/echo-request` and `/echo-both`.

### 6.2 6 scenarios

Per 6 routes above, the differential fixture exercises 6 scenarios:

1. **Response-only throttle (default route):** request `GET /echo-response`. Expected response: status 200, `content-length: 10240`, body byte-equivalent (10 KiB ASCII), total-time ≈ `(10240 * 8) / (10 * 1000) = 8.192` seconds (within ±50ms tolerance).
2. **Request-only throttle:** request `POST /echo-request` with body `<10 KiB ASCII>` and `enable_mode: REQUEST` (a sub-route override of the listener-level REQUEST_AND_RESPONSE). Expected: status 200, upstream-arrival-time (asserted via echo-backend timestamping) ≈ 8.192 seconds (within ±50ms); body byte-equivalent.
3. **REQUEST_AND_RESPONSE symmetric:** request `POST /echo-both` with body `<5 KiB>`. Expected: status 200, total wall-clock ≈ `2 * (5120 * 8) / (10 * 1000) = 8.192` seconds (within ±50ms tolerance); body byte-equivalent.
4. **Tiny-body fast-passthrough:** request `GET /echo-tiny` with `<100 byte>` direct_response body and a sub-route effective `limit_kbps: 1000` (per-route override picking up at this row's fixture entry; throttle_duration = `(100 × 8) / (1000 × 1000) = 0.8ms` < 1ms fast-passthrough threshold). Expected: status 200, total wall-clock < 20ms (no throttle armed; counter `response_enabled +1` but NOT `response_enforced`; gauge `response_pending` unmoved). Asserts the fast-passthrough threshold short-circuits the timer arm. SPEC author may refine the body-size / limit_kbps parameters per §11.4 if the threshold value lands at a different millisecond floor (5ms or 10ms per `time.AfterFunc` Linux granularity — see §11 pin §9.P9).
5. **Per-route disabled:** request `GET /echo-disabled` with body `<10 KiB>`. Expected: status 200, total wall-clock < 100ms (no throttle on this route — `disabled: true` wholesale opt-out), body byte-equivalent.
6. **Per-route override with own stat_prefix:** request `GET /echo-override`. Listener throttle would have been 8.192s; per-route override is `limit_kbps: 100` (10× higher), so override throttle is `(10240 * 8) / (100 * 1000) = 0.8192` seconds. Expected: status 200, total wall-clock ≈ 0.82 seconds (within ±50ms tolerance); body byte-equivalent; `/stats/prometheus` shows the `override` stat_prefix counter namespace has +1 `response_enabled` + +1 `response_enforced` while the `default` namespace counters did NOT increment on this stream (assuming INDEPENDENT-stats hypothesis ratified by §11 pin §9.P4).

### 6.3 Asserted equivalence

**Per-scenario assertions** (mirrors phase 11/12/13/14 scenario-by-scenario equivalence; see SPEC §3 acceptance review at SPEC drafting):

- **Status code:** byte-exact.
- **Headers:** lowercase wire-form byte-exact on ALL response headers (no header mutation; bandwidth_limit is a body-pacing filter, not a header-mutation filter).
- **Body:** byte-exact (bandwidth_limit does NOT transform bytes; only paces them; both Envoy and envoy-go emit the same bytes — the only difference is timing). Mirrors phase-11/12/13's byte-exact body discipline; DIVERGES from phase-14 compressor's decompress-and-compare per ADR-0133.
- **Total wall-clock time:** within ±50ms tolerance per scenario (PathB-async vs. Envoy's rate-paced chunks observably converge on the total-throttle-time axis). Smaller bodies (scenario 4: <20ms) tighter; larger throttles (scenario 1: 8.192s) looser.
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the 6 phase-15 counter/gauge stats per scenario.
- **Per-route fixture-config disposition:** scenarios 5 + 6 exercise BOTH per-route shapes (`disabled` + `bandwidth_limit` override); scenario 6 ALSO exercises INDEPENDENT-vs-SHARED stat namespace per §11 pin §9.P4.

### 6.4 Driver shape

`inputs/driver.go` mirrors the `0016` driver shape:
- 6 scenarios, each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Wall-clock timing helper `measureRequestDuration(ctx, req) (resp, duration, error)` using `time.Now()` at request-issue and response-completion.
- Per-scenario assertion helper that asserts both byte-exact body AND wall-clock within tolerance.
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.
- For scenarios 2 + 3 (request-side throttle), the echo-backend's `X-Echo-Received-At` timestamp header is asserted against `time.Now()` at request-issue to measure upstream-arrival time independent of response-side throttle.

---

## 7. Anticipated ADRs (ADR-0135 through ADR-0139)

5 anticipated ADRs (consistent with phase 11/12/14's 5-ADR rosters; one more than phase 13's 4-ADR roster). Phase 15 next-free is ADR-0135 (ADR-0134 was the highest landed in phase 14).

- **ADR-0135: `bandwidthlimit` package shape + boot registration + 4-file split + filterStats struct + DECODER+ENCODER SAME `*filter` instance.** Mirrors phase-14 ADR-0129 same-`*filter` precedent + phase-13 ADR-0125 + phase-12 ADR-0120 + phase-11 ADR-0114 + phase-10 ADR-0108 layout ADRs. Documents the package directory + extension-registry registration position (`bandwidthlimit` between `router` and `buffer` alphabetical-after-router) + the BOTH-encoder-and-decoder nature (symmetric request + response throttle). Codifies the FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 framework deltas.

- **ADR-0136: `compiledConfig` + 4-consumed/3-silent-ignored field decomposition + envoy-go-side parse validation.** Documents the 4 consumed fields + 3 silent-ignored fields + parse-rejection on `limit_kbps == 0` and `fill_interval` outside `[20ms, 1s]`. Cross-references phase-13 ADR-0126's envoy-go-only-check precedent + phase-11 ADR-0115's `fill_interval` precedent. Includes the throttle-duration arithmetic formula (body_size × 8 / (limit_kbps × 1000)).

- **ADR-0137: Body algorithm Path B-async (buffer-then-delayed-emit) + wire-shape divergence + forward-pointer to future streaming-framework phase.** Documents Path B-async as the chosen algorithm + the chunk-timing-divergent / total-throttle-time-equivalent wire-shape divergence-window from Envoy. Records the forward-pointer to a future streaming-framework phase that lands symmetric `EmitChunk` / `ConsumeChunk` primitives (mirrors phase-14 ADR-0131 §(vi) pattern). Cross-references phase-09 fault's `time.AfterFunc` precedent + phase-13 ADR-0128 + phase-14 ADR-0131 as the composed framework primitives. NO new framework primitive introduced by phase 15.

- **ADR-0138: 6-counter/gauge stat surface + namespace + SN-rule (SN2 reuse hypothesis; SN10 introduced only if §11 pin §9.P10 demands).** Documents the 6 counters + gauges + their hypothesized scope (`http.<HCM>.<filter_stat_prefix>.<counter>`) + SN2-reuse flattening rule (no new tag-extractor required under the brainstorm hypothesis; SN10 introduced if pin amends). Cross-references phase-11 ADR-0118 SN9 + phase-14 ADR-0132 NO-new-SN-rule disposition as the precedent (phase-14 ratified SN2 reuse for an 11→17-counter expansion, leaving SN10 as the next-free SN-rule slot for phase-15 or a successor phase).

- **ADR-0139: Per-route INDEPENDENT-stats hypothesis ratification OR refutation.** Documents the per-route disposition: INDEPENDENT (per-route override owns its own `stat_prefix` emission scope) is the brainstorm hypothesis; SPEC author resolves §11 pin §9.P4 IN-SESSION against Envoy v1.37.2. ADR ratifies the hypothesis OR documents the SHARED amendment (whichever Envoy emits) OR documents an envoy-go DIVERGENCE (if envoy-go elects INDEPENDENT despite Envoy emitting SHARED, with divergence-window text per phase-12/14 style). Cross-references phase-11 ADR-0117 INDEPENDENT-stats precedent + phase-12/13/14 SHARED-stats precedents.

**Plus an ADR-0125 in-place amendment paragraph §(xi)+** (NOT a new ADR): noting phase 15 bandwidth_limit as the THIRD row to use the disabled-OR-override 5th canonical per-route discipline + the WHOLESALE-not-merge semantic for the entire BandwidthLimit message (narrow-override-surface precedent stays bound to phase 14's ResponseDirectionOverrides). Authored at phase 15 SPEC drafting time per the ADR-0125 in-place-update precedent (mirrors phase-13 ADR-0127 v2 + phase-14 ADR-0125 amendment).

SPEC-time may revise the 5-ADR count per ADR-0044 SPEC-time-anticipation discipline (e.g., if framework-survey at SPEC time reveals a needed primitive previously unrecognized, ADR-0140 lands as the 6th).

---

## 8. Deferral list

Per phase 11/12/13/14 inline-deferral discipline (no omnibus ADR), the deferrals are 8 family-coupled items:

### 8.1 `enable_response_trailers` + `response_trailer_prefix` + trailer-emission primitive

`BandwidthLimit.enable_response_trailers` + `BandwidthLimit.response_trailer_prefix` are silent-ignored at runtime (always-no-trailers). Couples to a future trailer-emission framework phase that lands the underlying `EncoderFilterCallbacks.EmitTrailers(map[string]string)` primitive. Re-activation enables the trailer-emission for `x-envoy-bandwidth-rate-limit-latency-ms` + related trailers. Operator divergence-window: configs setting `enable_response_trailers: true` see Envoy emit `x-envoy-bandwidth-rate-limit-*` trailers; envoy-go emits no trailers.

### 8.2 4 histogram stats

`request_allowed_size`, `request_incoming_size`, `response_allowed_size`, `response_incoming_size` histograms DEFERRED per phase-06.1 ROADMAP-row "counters + gauges only — histograms deferred" baseline. Reference Envoy emits these four histograms; envoy-go MVP emits only the 6 counter/gauge stats. Couples to a future histogram-emit infrastructure phase that lands `*stats.Registry.Histogram` + Prometheus `histogram_*` extractor.

### 8.3 `runtime_enabled` RuntimeFeatureFlag

Silent-ignored at runtime; envoy-go always-100%-active when the filter is configured. Mirrors phase-11/12/14 silent-ignore-runtime-flag pattern (ADR-0117/ADR-0121/ADR-0130). Couples to Runtime + hot restart family. Re-activation lands when the Runtime family phase brings RTDS / Runtime-layer support.

### 8.4 Future streaming-framework phase (Path A chunk-timing-equivalent throttle)

The future streaming-framework phase lands `EmitChunk` / `ConsumeChunk` primitives that allow filters to emit body bytes interleaved with timer waits — enabling Path A (rate-paced chunk-emit; byte-by-byte wire-shape equivalence with Envoy). The phase ALSO lands the trailer-emission primitive (§8.1) and amends phase-14 compressor's wire-shape divergence (per ADR-0131 §(vi)). Forward-pointer recorded at ADR-0137. Re-activation: phase-15 bandwidth_limit's Path B-async upgrades to Path A; wire-shape divergence-window closes.

### 8.5 Per-route override `stat_prefix` emission scope (INDEPENDENT-vs-SHARED)

Depends on §11 pin §9.P4 resolution at SPEC time. Brainstorm hypothesis: INDEPENDENT (per-route emits to own counter namespace). SPEC author ratifies OR amends. ADR-0139 codifies. Coupling: future per-route stat-emission infrastructure phase (e.g., if a `cluster_manager`-level family adds per-route stat-aggregation across the proxy, the per-route INDEPENDENT-stats discipline composes naturally with that infrastructure).

### 8.6 `fill_interval` values outside `[20ms, 1s]` envoy-go-only parse-rejection

envoy-go-side filter-internal cap on `fill_interval`. If Envoy v1.37.2's PGV permits values outside `[20ms, 1s]` (e.g., 1ms, 10s), envoy-go's parse-rejection is a divergence-window. Couples to a future high-resolution-timer phase (sub-20ms `fill_interval` requires high-resolution timer support beyond `time.AfterFunc`'s default ~5-10ms resolution) AND a future long-duration-timer phase (>1s `fill_interval` is unbounded but doesn't materially differ from `1s + N×fill_interval` for downstream observability). §11 pin §9.P5 confirms exact Envoy filter-internal bounds.

### 8.7 Multi-listener BandwidthLimit chaining

A configuration with multiple `bandwidth_limit` filters in the same chain (e.g., listener-level + virtualhost-level + route-level) NOT anticipated by reference Envoy v1.37.2. Envoy's documented behavior is: the first `bandwidth_limit` filter in the chain owns the throttle; subsequent filters become no-ops. envoy-go follows. Documented here for forward-completeness — if Envoy ever amends to support chain-composition (e.g., "compose throttles by multiplication"), envoy-go would follow.

### 8.8 `BandwidthLimitPerRoute` non-standard proto field shapes

If §11 pin §9.P1 reveals the per-route proto shape is NOT a oneof of `disabled` + override but some other shape (e.g., per-route `disabled` boolean side-by-side with per-route `override` BandwidthLimit at the top level, without oneof), envoy-go SPEC author amends the per-route parser + ADR-0125 amendment paragraph wording. Brainstorm hypothesis is the oneof shape; placeholder for SPEC-time refinement.

---

## 9. Empirical pins for SPEC §11

The SPEC author (lifecycle-state 1 → 2) executes these pins IN-SESSION against reference Envoy v1.37.2 per ADR-0004. Each pin either RATIFIES the brainstorm hypothesis (→ no SPEC §11 amendment) or AMENDS it (→ SPEC §11 amendment-block + possibly a §12 brainstorm-amendment cycle if the empirical re-frame is too large for the §11 amendment-block channel — phase 13 precedent).

**P1 — Exact `BandwidthLimitPerRoute` proto shape.** CRITICAL pin. Confirm: oneof of `disabled` boolean + override sub-message (brainstorm hypothesis, mirrors phase-13 buffer + phase-14 compressor)? Or top-level `disabled` boolean + override field side-by-side (no oneof)? Or some other shape entirely? Determines the per-route parser shape + ADR-0125 amendment paragraph wording + §8.8 deferral relevance.

**P2 — PGV requirements on each consumed field.** Is `limit_kbps` REQUIRED at parse-time (brainstorm hypothesis: yes)? Is `fill_interval` REQUIRED or DEFAULTED (brainstorm hypothesis: defaulted to 50ms)? Is `enable_mode` REQUIRED at parse-time or DEFAULTED to DISABLED (brainstorm hypothesis: defaulted to DISABLED)? Is `stat_prefix` REQUIRED or OPTIONAL?

**P3 — Exact stat names + counter/gauge/histogram disposition.** Does Envoy v1.37.2 emit `request_enabled` as counter (brainstorm hypothesis)? Does it emit `request_pending` as gauge (brainstorm hypothesis)? Does it emit `request_allowed_total` as a separate counter in addition to the histogram (brainstorm hypothesis: no — the histogram-emit replaces the counter)? Confirm exact name list, scope, and counter-vs-gauge-vs-histogram disposition.

**P4 — Per-route stat SHARED-vs-INDEPENDENT.** CRITICAL pin (mirrors phase-11/13/14 question). Does Envoy emit per-route counters into the listener-level counter namespace (SHARED) or into a per-route namespace tagged by the override's `stat_prefix` (INDEPENDENT)? Brainstorm hypothesis: INDEPENDENT. ADR-0139 codifies the resolution.

**P5 — `fill_interval` filter-internal min/max bounds.** Hypothesis: `[20ms, 1s]` mirrors phase-11 local_ratelimit. Confirm Envoy's exact filter-internal bounds. Are they PGV-enforced at parse time or filter-internal? If PGV-enforced, envoy-go's parse-rejection is byte-equivalent; if filter-internal-only, envoy-go's parse-rejection is a divergence-window (per §8.6).

**P6 — `runtime_enabled` type.** RuntimeFeatureFlag (BoolValue default) vs RuntimeFractionalPercent (fractional percent default). Brainstorm hypothesis: RuntimeFeatureFlag with default-on. Determines fixture-config setup (whether Envoy-side `runtime_enabled.default_value` is bool or fractional).

**P7 — `response_trailer_prefix` default value when `enable_response_trailers=false`.** When trailers are disabled, does the prefix still take effect (e.g., if enabled later via runtime override)? Or is it wholly ignored? Brainstorm position: silent-ignored regardless. Pin confirms.

**P8 — Wire-shape on response throttle path.** CRITICAL pin. Send a 10 KiB body via `direct_response` with `limit_kbps: 10` and `fill_interval: 50ms` (so ~164 ticks at 50ms each = 8.2 seconds throttle). Does Envoy emit small chunks at 50ms cadence (164 chunks ≈ 62 bytes each)? Or does it emit larger chunks at coarser cadence? Or does it buffer-and-blast like envoy-go? §11 pin determines the §6.3 wall-clock tolerance window.

**P9 — Throttle-timing tolerance window vs Envoy under defined load.** Under fixture conditions (single-stream, no concurrency), what is the observed tolerance on total-throttle-time between Envoy and envoy-go? Brainstorm hypothesis: ±50ms for small throttles, ±100ms for large throttles. Pin measures empirically and sets per-scenario tolerance per §6.3.

**P10 — Prometheus tag-extractor name.** SN2 reuse vs new SN10 rule. Hypothesis: SN2 reuse (HCM-stat-prefix tag is sufficient; direction encoded in counter name suffix). Pin confirms; if amendment needed, SN10 lands (next-after-SN9 per phase-11 ADR-0118; phase-14 declined to introduce SN10 per its §1.1 amendment 3, leaving the slot free for phase-15 or a later phase).

**P11 — Namespace flattening behavior.** Is the stat path `http.<HCM>.bandwidth_limit.<stat_prefix>.<counter>` (with `bandwidth_limit` infix) or `http.<HCM>.<stat_prefix>.<counter>` (no infix, mirrors local_ratelimit)? Or some other shape? Brainstorm hypothesis: no `bandwidth_limit` infix (mirrors local_ratelimit per ADR-0117/ADR-0118). Pin confirms.

**P12 — `enable_mode=DISABLED` runtime evaluation.** Full passthrough? Stats still increment (`request_enabled`/`response_enabled` would semantically NOT be `enabled` if DISABLED, but Envoy might still increment a "filter-evaluated" gauge)? Hypothesis: full passthrough; stats do NOT increment.

**P13 — Per-stream pending-gauge lifecycle.** When does `request_pending` increment? At start of throttle (when `DecodeData(endStream=true)` arms the timer) or on buffer-cap engagement (when `len(req.Body)` exceeds a configured threshold)? Brainstorm hypothesis: at start of throttle (when timer arms). Pin confirms — determines exact gauge-bump location in the filter code.

**P14 — Per-route override `stat_prefix` emission scope.** Sub-pin of P4. If P4 is INDEPENDENT, does the per-route's `stat_prefix` field drive a wholly-own counter namespace (e.g., `http.<HCM>.override.request_enabled`), OR does it share-with-listener-level under a single namespace? Brainstorm hypothesis: wholly-own counter namespace.

**P15 — `fill_interval` interaction with `limit_kbps` for throttle math.** Is the throttle implementation kbps-per-fill_interval-tick (tokens added per tick = `limit_kbps × fill_interval / 8 / 1000` bytes) or steady-rate (math computes via `body_size / limit_kbps` without considering fill_interval, with fill_interval only governing timer-granularity)? Brainstorm hypothesis: steady-rate (the math uses `body_size × 8 / (limit_kbps × 1000)`; `fill_interval` only governs how finely the timer ticks). Pin confirms; impacts the §2.3 throttle-arithmetic.

---

## 10. ROADMAP delta

### 10.1 New row added by this brainstorm

A single ROW is added to `ROADMAP.md` (the table after row 14 per phase-09-onward flat-row convention; per ADR-0106 the §9 family-rows are flat top-level rows):

| id | title | depends-on | status | sub-phases | summary |
|---|---|---|---|---|---|
| 15 | http-filter-bandwidth-limit | 14 | planned |  | New `internal/filter/http/bandwidthlimit/` package implementing `envoy.filters.http.bandwidth_limit` (Envoy v1.37.2 canonical symmetric request+response throttle filter) under the 07.1 framework. EIGHTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14). MVP envelope: 4 fields consumed (`stat_prefix`, `enable_mode` all 4 values, `limit_kbps` UInt64Value REQUIRED, `fill_interval` Duration default 50ms; envoy-go-side filter-internal [20ms, 1s] range check per §11 pin); 3 fields silent-ignored (`runtime_enabled` always-100%; `enable_response_trailers` always-no-trailers per future trailer-emission framework phase; `response_trailer_prefix` couples). `limit_kbps == 0` PARSE-rejected per ADR-0136. Body algorithm Path B-async (buffer-then-delayed-emit via `time.AfterFunc` + `ContinueDecoding/Encoding`) per ADR-0137; chunk-timing-divergent / total-throttle-time-equivalent wire-shape divergence (±10-50ms tolerance) from reference Envoy's rate-paced chunks documented with forward-pointer to future streaming-framework phase. **ZERO framework deltas** — FIRST §9 row since phase-12 csrf to introduce no new primitives; composes against phase-09 fault async-resume + phase-13 ADR-0128 decode-side body-buffering + phase-14 ADR-0131 encode-side `OverwriteBody` (anticipated: not needed) — load-bearing demonstration of phase-13/14 framework primitives' reusability. Per-route TPFC `disabled`-OR-`bandwidth_limit` wholesale-override shape (THIRD row using ADR-0125 5th canonical disabled-OR-override; WHOLESALE-not-merge override semantic for the entire BandwidthLimit message). Stat surface 46→52 names (6 new counter+gauge: `request_enabled`, `request_enforced`, `request_pending`, `response_enabled`, `response_enforced`, `response_pending`; histograms deferred per phase-06.1 baseline; SN2 reuse hypothesis with SN10 introduced only if §11 pin §9.P10 demands). Per-route stats INDEPENDENT hypothesis per ADR-0139 (mirrors phase-11 local_ratelimit ADR-0117 stateful-override + own-stat_prefix). Differential fixture `0017-http-bandwidth-limit` (6 scenarios: response-only throttle, request-only throttle, REQUEST_AND_RESPONSE symmetric, tiny-body fast-passthrough, per-route disabled, per-route override with own stat_prefix). 19th fuzzer `FuzzBandwidthLimitConfigParse`. Anticipated 5 ADRs (ADR-0135 through ADR-0139) + ADR-0125 in-place amendment paragraph §(xi)+. Per ADR-0106, §9 family-rows are flat top-level rows; phase 15 lands as row `15`. ADR-0045 surface-split release valve stays available if PLAN finds > ~1500 LoC / > ~25 tasks; brainstorm's position is single-row at ~1100-1400 LoC / ~12-16 tasks.

### 10.2 §9 family heading at ROADMAP line 56 stays unchanged

Per ADR-0106(c). The line `### HTTP filters family` and the family-children enumeration at ROADMAP line 62 are unchanged across this brainstorm + the eventual phase-done landing.

### 10.3 No-sibling-stub discipline (per ADR-0106(b))

This brainstorm authors NO sibling stubs in ROADMAP for the 10 not-yet-brainstormed §9 family-children (`jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`) plus the future `envoy.filters.http.decompressor` companion to compressor. Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 11. Open structural questions for SPEC author (carry-forwards)

Items that the SPEC author resolves at lifecycle-state 1 → 2 transition. Each is decision-bearing — neither the brainstorm hypothesis nor the empirical pin alone resolves it; both inputs + the SPEC author's judgment are required.

### 11.1 Should phase 15 PRE-COMMIT to the 15.1 + 15.2 split, or land as a single row?

Brainstorm position: single row (per §1.4). LoC estimate ~1100-1400, task estimate ~12-16 tasks; under ADR-0045 1500-LoC / 25-task split-trigger. The split-by-direction (response-side first; request-side second) is mechanically clean BUT the symmetric algorithmic structure is the load-bearing pedagogical value — splitting halves it. SPEC author re-evaluates at SPEC time with the empirical pin resolutions in hand (some pins might surface significantly more surface — e.g., if §9.P3 reveals a richer-than-expected stat surface, or §9.P1 reveals a more-complex per-route shape).

### 11.2 Should the `OverwriteBody` primitive be invoked on the encode side?

Brainstorm hypothesis: not needed (the buffered bytes ARE the original bytes; `DataStopIterationAndBuffer` + `ContinueEncoding` returns them unchanged through the framework). SPEC author at framework-survey step confirms by tracing the encode-chain code path — specifically `internal/filter/hcm/connection.go:467-475` + `chain.go:336` — to verify the buffered-data return-path does NOT require explicit `cb.OverwriteBody` invocation. If it DOES require, then phase-15 has ONE framework-delta REUSE (not introduction; ADR-0131 OverwriteBody already exists from phase 14) and the "ZERO framework deltas" framing in §3 amends to "ZERO new framework primitives; reuses existing ADR-0131 primitive."

### 11.3 INDEPENDENT-vs-SHARED stats — divergence-window or match Envoy?

Per §5 + §9.P4. If Envoy emits SHARED stats, envoy-go has two paths: (a) match Envoy (SHARED; per-route override `stat_prefix` honored at parse but routed into listener-level counter namespace), or (b) DIVERGE (INDEPENDENT; per-route `stat_prefix` drives own counter namespace; divergence-window documented). The operationally-correct shape is INDEPENDENT (per-route counters distinguish routes in dashboards); the byte-equivalent shape is whatever Envoy emits. SPEC author + user decide.

### 11.4 Fast-passthrough threshold value (sub-1ms or sub-20ms?)

Brainstorm hypothesis: throttle_duration < 1ms skips the timer arm. SPEC author may refine — e.g., if `time.AfterFunc` minimum granularity on Linux is empirically ~5ms, the threshold should be 5ms or even 10ms to avoid the timer-overhead-eats-throttle-budget anti-pattern. SPEC author measures empirically at SPEC time.

### 11.5 Should phase 15 carry forward a phase-13-style §12 amendment cycle?

Phase 13 buffer had a post-landing §12 amendment cycle (see phase-13 BRAINSTORM line 554 onward). Phase 14 compressor did NOT (the empirical pins resolved cleanly at SPEC time). Phase 15's amendment-cycle posture: NOT anticipated at brainstorm time; the 15 empirical pins are well-scoped and the framework-deltas are ZERO. The SPEC author retains the §12 channel as a release valve if a pin resolution unexpectedly reframes a brainstorm decision.

### 11.6 Filter-chain ordering with respect to compressor

When both `bandwidth_limit` and `compressor` are in the same chain, ordering matters: bandwidth_limit BEFORE compressor means the throttle paces the uncompressed body bytes (more bytes through the throttle); bandwidth_limit AFTER compressor means the throttle paces the compressed bytes (fewer bytes; tighter effective throughput). Both orderings are valid; the SPEC author documents the recommended ordering in BEHAVIOR_CONTRACT phase-15 forward-pointer notes and ensures the fixture's filter-chain ordering is explicit.

---

## End of phase 15 brainstorm

Authored 2026-05-11 against master tip `f4ce582`. Lifecycle-state-1 exit. Next session: SPEC drafting (skill `superpowers:writing-plans` routed through SPEC-authoring step per ADR-0005). SPEC author resolves §9 empirical pins IN-SESSION against reference Envoy v1.37.2.
