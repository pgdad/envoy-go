# Phase 15 — HTTP filter `envoy.filters.http.bandwidth_limit` (`internal/filter/http/bandwidthlimit/`, differential fixture `0017-http-bandwidth-limit`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.bandwidth_limit` extension + `## Stat-name mapping` 46→60 extension + `### Twin-series filter discipline` extension + `## Timing tolerances` extension, ZERO framework deltas) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory user preference) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.bandwidth_limit` — Envoy v1.37.2's canonical "rate-limit body throughput in **KiB/s** (kibibytes-per-second per proto comment line 95 + SPEC §1.1 amendment 6; NOT kilobits-per-second as BRAINSTORM hypothesized)" filter, BOTH-direction (symmetric request + response) MVP — as the EIGHTH production HTTP filter in envoy-go, with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis EXCEPT the deliberately allow-listed intra-throttle chunk-arrival-time axis (envoy-go: Path B-async silent-then-blast; Envoy: Path A rate-paced chunks at exact `fill_interval` cadence with `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes per tick; total-throttle-time observably equivalent within **±70ms** tolerance per §11.P9) and the 2 unconditional Envoy histograms (`request_transfer_duration`, `response_transfer_duration`) allow-listed via twin-series-filter divergence-window (phase-06.1 "counters + gauges only" baseline; SPEC §1.1 amendment 9) and the trailer-emission axis (always-no-trailers in envoy-go; deferred to future trailer-emission framework phase per §8.1), under the 07.1 framework, with **ZERO framework deltas** (FIRST §9 row since phase-12 csrf to introduce no new primitives; composes against phase-09 fault async-resume + phase-13 ADR-0128 decode-side body-buffering + phase-14 ADR-0131 encode-side `OverwriteBody` — anticipated NOT invoked since the buffered-return path returns bytes unchanged per §3 framework-survey).

**Architecture:** New `internal/filter/http/bandwidthlimit/` package owning the filter implementation; ENCODER+DECODER `HTTPFilter` value with SAME `*filter` instance servicing both sides (mirrors phase-14 compressor ADR-0129 same-`*filter` precedent, generalized to symmetric BOTH-direction throttle); body algorithm Path B-async (buffer-then-delayed-emit) per ADR-0137 — `DecodeData`/`EncodeData` on `*Active=true + endStream=true` buffers the body, computes `throttle_duration` via the kbps-per-tick formula (`chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes per tick; `ticks = ceil(body_size / chunk_size)`; `throttle = ticks × fill_interval`) per SPEC §6.6 + §1.1 amendment 6 + §11.P15, arms `f.requestTimer = time.AfterFunc(throttle, ...)` (decode-side; `f.responseTimer` symmetric on encode-side), and returns `DataStopIterationAndBuffer`; timer-fire callback increments `*_enforced` by `ticks` (per-tick cumulative match per §11.P3) + `*_allowed_total_size` by `bodyLen` + decrements `*_pending` + invokes `cb.ContinueDecoding/Encoding` to resume the chain; the framework's buffered-return path emits the buffered body bytes unchanged downstream (NO `cb.OverwriteBody` invocation needed since bytes are unchanged); `DecodeHeaders` resolves per-route TPFC via `dcb.RequestRouteConfig()` → `state.resolvePerRouteConfig(msg)` → caches effective `*compiledConfig` on `f.requestRC` + sets `f.requestActive` per `enable_mode` + cascades same RC to `f.responseRC` + `f.responseActive` so encode-side reads what decode-side resolved (per-stream symmetric semantic); `OnDestroy` stops both timers with the Stop-races-Fire pending-gauge discipline (`Stop()==true → Dec here; ==false → trust the callback's Dec`) per SPEC §4 + §6.9; per-route `BandwidthLimit` proto is REUSED directly via TPFC (NO `BandwidthLimitPerRoute` wrapper exists in Envoy v1.37.2 per §11.P1 + §1.1 amendments 1 + 2; phase-15 introduces a NEW 6th canonical per-route pattern — bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route — adjacent to ADR-0117's 4th canonical, documented at ADR-0125 §(xi) amendment paragraph already LANDED at SPEC commit `49e0361`); per-route stats INDEPENDENT per ADR-0139 + §11.P4 + §11.P14 (mirrors phase-11 local_ratelimit per ADR-0117 verbatim; SECOND row using stateful-override-with-INDEPENDENT-stats); 14-counter+gauge `filterStats` struct registered at `New` factory time per stat_prefix (8 counters: `request_enabled`, `request_enforced`, `request_incoming_total_size`, `request_allowed_total_size` + 4 response-symmetric; 6 gauges: `request_pending`, `request_incoming_size`, `request_allowed_size` + 3 response-symmetric) per ADR-0138 + §1.1 amendment 7 under namespace `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore-infix; NOT HCM-rooted per §11.P11) → Prometheus `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` via existing `internal/stats/name.go` default-branch flatten (NO new SN10 rule per §1.1 amendment 8 + §11.P10); 2 unconditional Envoy histograms (`request_transfer_duration`, `response_transfer_duration`) allow-listed via twin-series-filter divergence-window per §1.1 amendment 9 + BEHAVIOR_CONTRACT §242 extension; differential fixture 0017 two-listener topology with cluster `c_backend_b` reusing the existing `test/helpers/echobackend/` from phase-14, 6 scenarios per SPEC §7.1 covering response-only / request-only / REQUEST_AND_RESPONSE symmetric / tiny-body within initial burst / per-route DISABLED via `enable_mode: DISABLED` / per-route override with own `stat_prefix`, byte-exact body assertion (bandwidth_limit does NOT transform bytes; only paces them) + per-counter delta byte-equivalence on the 14 active stats + ±70ms wall-clock tolerance per scenario.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008); `protojson.Unmarshal` for proto decoding; `time.AfterFunc` Go stdlib for async-resume timer (reuse from phase-09 fault precedent); `sync.Map` for per-route lazy-cache (reuse from phase-11 ADR-0117 precedent); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per ADR-0008 + ENVOY_TARGET.md); golangci-lint 1.64.8 (ADR-0009 pin); Docker for differential harness; HTTP/1.1 plaintext fixture (no H2 differential coverage per SPEC §7.4).

---

## Scope check — why phase 15 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 / 14 PLAN's component-table convention):

- `internal/filter/http/bandwidthlimit/doc.go` ~30
- `internal/filter/http/bandwidthlimit/bandwidthlimit.go` ~320–380 (filter + factory + types + DecodeHeaders + DecodeData + DecodeTrailers + EncodeHeaders + EncodeData + EncodeTrailers + OnDestroy + Set{Decoder,Encoder}Callbacks + compiledConfig + factoryState + parsePerRoute + resolvePerRouteConfig + buildCompiledConfig + buildCompiledConfigPerRoute + filterStats + newFilterStats + newFilterStatsIfAbsent)
- `internal/filter/http/bandwidthlimit/bucket.go` ~60–90 (the kbps-per-tick `throttleDuration(bodySize int, limitKbps uint64, fillInterval time.Duration) (duration time.Duration, ticks uint64)` helper + GoDoc explaining the formula + the foot-gun branch per §1.1 amendment 10 + the one-tick floor)
- `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` ~700–900 (7 unit-test groups per SPEC §14.1 + planner-time-emerging Group 8 stats-namespace integration sub-group surfaced at planner-time per planner-time decision 9; ~50–65 test cases total)
- `internal/filter/http/bandwidthlimit/fuzz_test.go` ~80 (19th fuzzer in repo: `FuzzBandwidthLimitConfigParse`; mirrors phase-14 `FuzzCompressorConfigParse` shape extended for the 7-field BandwidthLimit proto)
- `cmd/envoy-go/main.go` +1 import line + 1 register line ~+3 (`httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)` inserted alphabetical-after-router per ADR-0100 §2.2 + ADR-0114 + ADR-0120 + ADR-0125 + ADR-0129 + ADR-0135 convention)
- `test/fixtures/0017-http-bandwidth-limit/` (NEW DIRECTORY) — `envoy.yaml` ~110 + `envoy-go.yaml` ~110 + `expectations.yaml` ~55 + `README.md` ~90 + `inputs/driver.go` ~220 = ~585
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPBandwidthLimit BackendKind = 14`) + doc-comment ~+15
- `test/differential/runner_test.go` blank-import addition + new `startEchoBackend` spawn helper reuse (echobackend helper from phase-14 already exists; just add the switch-case for `HTTPBandwidthLimit`) ~+15
- `docs/envoy-go/DECISIONS.md` 5 ADRs (ADR-0135 + ADR-0136 + ADR-0137 + ADR-0138 + ADR-0139) authored at impl-time per ADR-0044 ADR-on-impl convention; **NOT pre-landed at SPEC commit** per phase-13 buffer ADR-on-impl precedent (unlike phase-14 compressor's pre-landing) — phase-15 SPEC §8 roster anticipates the 5 ADRs but the §Decision bodies are authored at the impl-task that anchors each ADR per ADR-0044. Per-ADR Lands-in-task: ADR-0135 at Task 2; ADR-0136 at Task 2; ADR-0137 at Task 3; ADR-0138 at Task 8; ADR-0139 at Task 7. ~+400 LoC across the 5 ADRs.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.bandwidth_limit` subsection ~150 + §13.2 stat-table 46→60 names extension (14 new active rows + 2 deferred-histograms allow-list rows) ~25 + §13.3 equivalence-matrix row ~3 + §13.4 `### Phase 15 forward-pointer notes` subsection ~80 + §13.5 `## Timing tolerances` extension ~12 + `### Twin-series filter discipline` extension ~5 = ~+275
- `docs/envoy-go/ROADMAP.md` row `15` `in-progress → done` flip + summary sharpening ~+1 net
- `docs/envoy-go/STATE.md` advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (NEW; lifecycle artefact) ~650
- `docs/envoy-go/phases/15-http-filter-bandwidth-limit/REVIEW.md` (NEW; lifecycle artefact) ~200

**Production code: ~410–500 LoC (filter impl in `bandwidthlimit.go` + `bucket.go`) + 0 LoC framework deltas (ZERO framework deltas per §3 framework-survey) + ~3 LoC main.go + 0 LoC echobackend helper (reused from phase-14) = ~413–503 LoC production + ~780 LoC tests (~700-900 unit + 80 fuzzer) + ~585 LoC fixture YAML/Go + ~875 LoC docs (5 ADRs + BEHAVIOR_CONTRACT + ROADMAP + STATE + PROGRESS + REVIEW) ≈ ~2650–2750 LoC total** (production-only ~413–503 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~413–503 LoC; task count below is **16**, comfortably under the 25 limit). The 5 anticipated ADRs (ADR-0135..ADR-0139) authored at impl-time per ADR-0044; ADR-0125 amendment paragraph §(xi) **ALREADY LANDED at SPEC commit `49e0361`** per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) in-place-update precedent (no PLAN-time re-anchor needed). SPEC §1.3 (per BRAINSTORM Decisions 9 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 15 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~14–18 tasks; this PLAN lands at **16 tasks** (mid-bound — symmetric BOTH-direction throttle doubles the algorithmic surface relative to phase-11 local_ratelimit's request-only at ~393 LoC, but the per-direction algorithm is simpler (no token-bucket-state-machine, no SendLocalReply short-circuit, just throttle-duration arithmetic + `time.AfterFunc` arming); the 14-stat `filterStats` is larger than phase-11's 4 + phase-12's 3 + phase-13's 0 but smaller than phase-14's 17; differential fixture is 6 scenarios identical-count to phase-14).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 / SPEC §1.4 would be `15.1 = response-side throttle MVP (Tasks 1-5 + fixture scenarios 1, 4, 5, 6)` and `15.2 = request-side throttle + symmetric REQUEST_AND_RESPONSE (Tasks 6-7 incremental + scenarios 2, 3)`; SPEC §1.4 explicitly rejects the split since both halves stay well under the LoC threshold and the request-side mirror is small ~50-70 LoC additional production. PLAN concurs and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/bandwidthlimit/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`BandwidthLimit` proto with **4 listener-level fields actively consumed** per SPEC §1 + §1.1 amendments 3 + 4 + 5 — `stat_prefix` (string; PGV `min_len=1` per §11.P2 + amendment 3) + `enable_mode` (4-value enum: DISABLED / REQUEST / RESPONSE / REQUEST_AND_RESPONSE; PGV `defined_only=true`; default DISABLED=0) + `limit_kbps` (UInt64Value; **KiB/s units per proto comment + amendment 6**; OPTIONAL at listener-level with foot-gun semantic per amendment 10; CODE-LEVEL REQUIRED at per-route per filter source `"limit must be set for per route filter config"` mirrored as `"bandwidth_limit: per-route entry requires limit_kbps to be set"` per ADR-0136; PGV `gte=1` when wrapper present) + `fill_interval` (Duration; default 50ms; PGV `gte=20ms, lte=1s` per amendment 5 + §11.P5); **3 listener-level fields silent-ignored** — `runtime_enabled` (RuntimeFeatureFlag; always-100%-active per ADR-0117/ADR-0121/ADR-0130 precedent) + `enable_response_trailers` (bool; always-no-trailers in envoy-go MVP; trailer-emission primitive deferred per §8.1) + `response_trailer_prefix` (string; PGV pattern `^[^\x00\n\r]*$`; couples to `enable_response_trailers`); **operational foot-gun** at runtime when listener-level `limit_kbps` unset + active `enable_mode` (matches Envoy byte-equivalent per amendment 10 + probeJ)); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (DECODER-side: `DecodeHeaders` resolves per-route + caches effective `*compiledConfig` on `f.requestRC` + sets `f.requestActive` per `enable_mode` + cascades RC to encode-side `f.responseRC` + `f.responseActive`; `DecodeData` buffers + on `endStream=true` arms decode-side timer; `DecodeTrailers` pass-through; ENCODER-side: `EncodeHeaders` no-op (RC was cached at DecodeHeaders); `EncodeData` buffers + on `endStream=true` arms encode-side timer; `EncodeTrailers` pass-through; ENCODER+DECODER `HTTPFilter` value sets `Decoder: f, Encoder: f` SAME instance per planner-time decision 9 + ADR-0135 — mirrors phase-14 ADR-0129 same-`*filter` precedent generalized to symmetric BOTH-direction); (d) the per-route discipline (**6th canonical bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route** per ADR-0125 §(xi) amendment + ADR-0139 — distinct from ADR-0117 4th canonical (no code-level extra check) AND ADR-0125 5th canonical disabled-OR-override-sum-type (uses wrapper proto); same `BandwidthLimit` proto reused via TPFC by pointer-identity key into `factoryState.perRoute sync.Map` lazy-cache); per-route stats INDEPENDENT (mirrors phase-11 ADR-0117); (e) the body algorithm — **Path B-async (buffer-then-delayed-emit) with kbps-per-tick throttle math** per ADR-0137 + §6.6 + §1.1 amendment 6 — `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes per tick; `ticks = ceil(body_size / chunk_size)`; `throttle = ticks × fill_interval`; `time.AfterFunc(throttle, ...)` async-resume reuse from phase-09 fault precedent at `internal/filter/http/fault/fault.go:319,335`; **ZERO framework deltas** per §3 — composes against phase-09 + phase-13 ADR-0128 + phase-14 ADR-0131 framework primitives without amendment; `cb.OverwriteBody` anticipated NOT invoked (the buffered-return path returns bytes unchanged); (f) the wire-shape divergence-window from reference Envoy (envoy-go: silent-then-blast one-shot at timer-fire; Envoy: Path A rate-paced chunks at exact `fill_interval` cadence with `chunk_size` bytes per tick — 77 chunks at 51-byte cadence for `body=4000, kbps=1, fill=50ms` per probeL; total wall-clock throttle-time equivalent within ±70ms per §11.P9; chunk-arrival-time observably diverges) + the trailer-emission divergence-window (envoy-go: always-no-trailers; Envoy: 4 trailers when `enable_response_trailers: true`) + the histograms divergence-window (envoy-go: NO histograms per phase-06.1 baseline; Envoy: 2 unconditional `*_transfer_duration` histograms allow-listed via twin-series-filter per amendment 9); (g) the cross-cutting ADR anchors (ADR-0125 amendment §(xi) (LANDED at SPEC commit) / ADR-0135 / ADR-0136 / ADR-0137 / ADR-0138 / ADR-0139). Mirrors `internal/filter/http/compressor/doc.go`-style structure (~30 LoC precedent extended to ~30-35 LoC). Per SPEC §1 + §6.1. |
| `internal/filter/http/bandwidthlimit/bandwidthlimit.go` | NEW | Filter implementation — main file per planner-time decision 1 (TWO-WAY split: `bandwidthlimit.go` + `bucket.go`; mirrors phase-11 `local_ratelimit.go` + `bucket.go` precedent; SPEC §15 acceptance #1 enumerates this exact file set verbatim). **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Internal package consts:** `filterName = "envoy.filters.http.bandwidth_limit"`; `defaultFillInterval = 50 * time.Millisecond` (per Envoy filter source). **Unexported types (per SPEC §6.2 + §6.3):** `compiledConfig` struct (5 fields per §6.2: `statPrefix string` (PGV non-empty per §11.P2 + amendment 3) + `enableMode bandwidthlimitv3.BandwidthLimit_EnableMode` (4 values; default DISABLED) + `limitKbps uint64` (KiB/s per amendment 6; OPTIONAL at listener with foot-gun per amendment 10; FILTER-INTERNAL REQUIRED at per-route per amendment 4) + `fillInterval time.Duration` (PGV [20ms, 1s] when set; default 50ms per amendment 5) + `stats *filterStats` (14 active stats; nil when ctx.Stats is nil — test path per ADR-0085 nil-tolerance)); `factoryState` struct (3 fields per SPEC §6.3 + ADR-0117 + IMPL-1: `listenerRC *compiledConfig` + `perRoute sync.Map` (map[*bandwidthlimitv3.BandwidthLimit]*compiledConfig — per-route lazy-cache keyed by pointer-identity per ADR-0117 IMPL-1) + `reg *stats.Registry`); `filter` struct (per SPEC §6.3 — 11 fields: `state *factoryState` + `dcb envoyhttp.DecoderFilterCallbacks` + `ecb envoyhttp.EncoderFilterCallbacks` + decode-side per-stream state: `requestRC *compiledConfig`, `requestActive bool`, `requestBody []byte`, `requestTimer *time.Timer` + encode-side per-stream state: `responseRC *compiledConfig`, `responseActive bool`, `responseBody []byte`, `responseTimer *time.Timer`); `filterStats` struct (per SPEC §6.2 + §1.1 amendment 7 + ADR-0138 — **14 active fields**: 8 counters (`requestEnabled`, `requestEnforced`, `requestIncomingTotalSize`, `requestAllowedTotalSize`, `responseEnabled`, `responseEnforced`, `responseIncomingTotalSize`, `responseAllowedTotalSize`) + 6 gauges (`requestPending`, `requestIncomingSize`, `requestAllowedSize`, `responsePending`, `responseIncomingSize`, `responseAllowedSize`); the 2 transfer-duration histograms are NOT registered in MVP per phase-06.1 baseline + amendment 9). **Helpers (per SPEC §6.5):** `buildCompiledConfig(c *bandwidthlimitv3.BandwidthLimit, ctx envoyhttp.FactoryCtx, isPerRoute bool) (*compiledConfig, error)` (PGV-mirror at parse-time: stat_prefix REQUIRED (`"bandwidth_limit: stat_prefix required"` envoy-go-own wording); limit_kbps validation (>=1 when set; FILTER-INTERNAL REQUIRED at per-route when isPerRoute=true with envoy-go-own wording `"bandwidth_limit: per-route entry requires limit_kbps to be set"`); fill_interval bounds check (`gte=20ms, lte=1s`); allocates `*filterStats` via `newFilterStats` (listener) or `newFilterStatsIfAbsent` (per-route) when ctx.Stats != nil); `parsePerRoute(any *anypb.Any) (proto.Message, error)` (unmarshals to `*bandwidthlimitv3.BandwidthLimit`; defensive PGV-mirror partial; returns the proto for the registry's per-route map per phase-11 IMPL-1 pattern); `(s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledConfig` (mirrors phase-11 `resolvePerRouteConfig` at `local_ratelimit.go:305-337` verbatim with `*LocalRateLimit` → `*BandwidthLimit` and `buildRuntimeConfigPerRoute` → `buildCompiledConfigPerRoute`; nil msg → listener fallback; type-asserts to `*BandwidthLimit`; consults `sync.Map` lazy-cache; lazy-constructs via `buildCompiledConfigPerRoute` on first resolve via `LoadOrStore`); `newFilterStats(reg *stats.Registry, statPrefix string) *filterStats` (registers 14 counters under `<statPrefix>.http_bandwidth_limit.<counter>` path; nil-registry tolerance per ADR-0085); `newFilterStatsIfAbsent(reg *stats.Registry, statPrefix string) *filterStats` (post-Freeze idempotent via NewCounterIfAbsent + NewGaugeIfAbsent per ADR-0117). **DecodeHeaders body** (per SPEC §6.7): resolve per-route via `f.dcb.RequestRouteConfig()` → `state.resolvePerRouteConfig(perRouteMsg)` → cache on `f.requestRC` + cascade RC to `f.responseRC` (same RC under symmetric semantic since per-stream both directions share the resolved entry); set `f.requestActive = (em == REQUEST || em == REQUEST_AND_RESPONSE)`; set `f.responseActive = (em == RESPONSE || em == REQUEST_AND_RESPONSE)`; return `HeaderContinue` (no header mutation; bandwidth_limit is body-only). **DecodeData body** (per SPEC §6.7): if `!f.requestActive` → `DataContinue` (pure passthrough); append `data` to `f.requestBody`; if `!endStream` → `DataStopIterationAndBuffer`; on `endStream=true`: increment `*_enabled` + `*_incoming_total_size` + `*_incoming_size`; compute throttle via `throttleDuration(len(f.requestBody), f.requestRC.limitKbps, f.requestRC.fillInterval)` (from `bucket.go`); if `throttle == 0` → increment `*_allowed_total_size` + `*_allowed_size` + return `DataContinue`; else increment `*_pending`, arm `f.requestTimer = time.AfterFunc(throttle, func() { ...stats then f.dcb.ContinueDecoding() })`; timer-callback increments `*_enforced += ticks` (per-tick cumulative match per §11.P3 + §6.7) + `*_allowed_total_size += bodyLen` + `*_allowed_size.Set(bodyLen)` + `*_pending.Dec()` + `f.dcb.ContinueDecoding()`; return `DataStopIterationAndBuffer`. **EncodeHeaders body** (per SPEC §6.8): no-op (`HeaderContinue`); `responseRC` + `responseActive` were already cached at DecodeHeaders. **EncodeData body** (per SPEC §6.8): SYMMETRIC to DecodeData with `f.responseActive`, `f.responseBody`, `f.responseTimer`, response-* stats fields, `f.ecb.ContinueEncoding()`. **OnDestroy body** (per SPEC §6.9 + §4 Stop-races-Fire discipline): for each direction: `if f.<dir>Timer != nil && f.<dir>Timer.Stop() && f.<dir>RC != nil && f.<dir>RC.stats != nil { f.<dir>RC.stats.<dir>Pending.Dec() }` — if `Stop()` returns true (callback prevented) Dec the pending gauge here; if false (callback already ran or about to run) trust the callback's Dec; no double-decrement either way. **SetDecoderCallbacks** stores `f.dcb = cb`; **SetEncoderCallbacks** stores `f.ecb = cb` (BOTH per planner-time decision 10; SAME *filter services both). **DecodeTrailers** + **EncodeTrailers** pass-through (`TrailersContinue`). ~320–380 LoC. |
| `internal/filter/http/bandwidthlimit/bucket.go` | NEW | The per-stream throttle helper — the kbps-per-tick `throttleDuration(bodySize int, limitKbps uint64, fillInterval time.Duration) (duration time.Duration, ticks uint64)` function per SPEC §6.6 + §1.1 amendment 6 + §11.P15. **Algorithm:** if `bodySize == 0` → return `(0, 0)` (no throttle); if `limitKbps == 0` → return `(24 * time.Hour, 1)` per §1.1 amendment 10 foot-gun match (arbitrarily-large throttle to mirror Envoy's runtime-hang behavior on listener-level missing `limit_kbps` + active `enable_mode`); compute `fillSec := fillInterval.Seconds()`; compute `chunkSize := uint64(float64(limitKbps) * 1024 * fillSec)` bytes per tick; defensive `if chunkSize == 0 { chunkSize = 1 }` (structurally unreachable given fill_interval >= 20ms PGV; safe-fallback); `ticks = (uint64(bodySize) + chunkSize - 1) / chunkSize` (ceil division); `duration = time.Duration(ticks) * fillInterval`; return `(duration, ticks)`. **GoDoc:** the formula `chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds` (KiB/s × seconds = KiB → ×1024 = bytes); `throttle_duration = ceil(body_size / chunk_size_per_tick) × fill_interval`; the function is pure (no side effects; no allocations); the `ticks` return value is consumed by the caller to increment `*_enforced` by `ticks` at timer-fire per §11.P3 + §6.7 + §1.1 amendment 7 (per-tick cumulative match with Envoy). Empirical verification matrix from SPEC §6.6 reproduced inline (the 5-row table showing predicted vs observed across body sizes 100/1024/4000 and limit_kbps 1/5/10) so the impl-task author has the conformance evidence at hand. **No exported names** (function is package-local). ~60–90 LoC including GoDoc + the empirical-verification-matrix comment block. Mirrors phase-11 `bucket.go` precedent shape (which implements the `tokenBucket` lazy-refill primitive; phase-15 `bucket.go` is structurally simpler — pure-function throttle-duration arithmetic with no state). |
| `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` | NEW | Unit tests per SPEC §14.1 (7 SPEC-named groups + 1 stats-namespace integration sub-group surfaced at planner-time per planner-time decision 9). **Group 1 — Config parse + buildCompiledConfig PGV-mirror (per SPEC §14.1 #1 + §6.5 + §1.1 amendments 3 + 4 + 5):** `TestNew_NilTC` (nil typed_config → rejected with `"bandwidth_limit: typed_config required"`), `TestNew_MalformedTC` (random bytes → `"bandwidth_limit: unmarshal: ..."`), `TestBuildCompiledConfig_StatPrefixEmpty_Rejected` (empty `stat_prefix` → `"bandwidth_limit: stat_prefix required"` per §11.P2 + amendment 3), `TestBuildCompiledConfig_AllEnableModeValuesAccepted` (parametrized DISABLED/REQUEST/RESPONSE/REQUEST_AND_RESPONSE; all 4 accepted), `TestBuildCompiledConfig_LimitKbpsUnset_AcceptedAtListener` (foot-gun acceptance per amendment 10), `TestBuildCompiledConfig_LimitKbpsZero_Rejected` (`>=1` PGV-mirror defensive — value 0 → `"bandwidth_limit: limit_kbps must be >= 1"`), `TestBuildCompiledConfig_LimitKbpsExplicit` (parametrized 1/10/100/1000; all accepted; stored verbatim), `TestBuildCompiledConfig_FillIntervalDefault_50ms` (when wrapper absent → `compiledConfig.fillInterval == 50ms`), `TestBuildCompiledConfig_FillIntervalExplicit` (parametrized 20ms/50ms/100ms/500ms/1s; all accepted), `TestBuildCompiledConfig_FillIntervalBelowMin_Rejected` (10ms → `"bandwidth_limit: fill_interval 10ms outside supported range [20ms, 1s]"` per amendment 5), `TestBuildCompiledConfig_FillIntervalAboveMax_Rejected` (2s → analogous), `TestBuildCompiledConfig_RuntimeEnabled_SilentIgnored` (field present with `default_value: false` → parses cleanly; `compiledConfig` has no runtime-enabled field; runtime is always-100%-active per planner-time decision 7), `TestBuildCompiledConfig_EnableResponseTrailers_SilentIgnored` (field present with `enable_response_trailers: true` → parses cleanly; no trailers emitted at runtime per planner-time decision 8), `TestBuildCompiledConfig_ResponseTrailerPrefix_SilentIgnored` (field present → parses cleanly; couples to enable_response_trailers; not stored). **Group 2 — buildCompiledConfigPerRoute + parsePerRoute PGV-mirror discipline (per SPEC §14.1 #2 + §6.11 + §11.P1):** `TestBuildCompiledConfigPerRoute_LimitKbpsUnset_Rejected` (per-route entry without `limit_kbps` → `"bandwidth_limit: per-route entry requires limit_kbps to be set"` per amendment 4 + §11.P1 verbatim CODE-LEVEL extra check), `TestBuildCompiledConfigPerRoute_LimitKbpsSet_Accepted` (with `limit_kbps: 5` → accepted; INDEPENDENT stats allocated), `TestBuildCompiledConfigPerRoute_StatPrefixEmpty_Rejected` (same as listener-level; PGV mirrored), `TestParsePerRoute_ValidProto_Parses` (single `BandwidthLimit` unmarshals cleanly), `TestParsePerRoute_MalformedAny_Rejected` (bad bytes → unmarshal error), `TestParsePerRoute_RuntimeEnabledOverride_SilentIgnored` (per-route `runtime_enabled: { default_value: false }` → parses; not honored at runtime per planner-time decision 7 + §11.P6). **Group 3 — `throttleDuration` kbps-per-tick arithmetic (per SPEC §14.1 #3 + §6.6 + §1.1 amendment 6):** `TestThrottleDuration_EmptyBody_ReturnsZero` (bodySize=0 → (0, 0)), `TestThrottleDuration_LimitKbpsZero_ReturnsFootGun` (limitKbps=0 → (24h, 1) per amendment 10), `TestThrottleDuration_OneTickFloor` (parametrized small bodies fitting in one chunk_size → ticks=1, duration=fillInterval), `TestThrottleDuration_KbpsPerTickMatrix` (parametrized 5-row matrix from SPEC §6.6: body=100 kbps=10 fill=50ms → ticks=1 (one-tick floor); body=1024 kbps=10 fill=50ms → ticks=2 → 100ms; body=4000 kbps=10 fill=50ms → ticks=8 → 400ms; body=4000 kbps=5 fill=50ms → ticks=16 → 800ms; body=4000 kbps=1 fill=50ms → ticks=79 → 3950ms — all assert duration matches `time.Duration(ticks) * fillInterval` exactly), `TestThrottleDuration_FillIntervalGranularity` (parametrized fill_interval 20ms/50ms/100ms; verifies chunk_size = limit_kbps × 1024 × fillSec scales linearly), `TestThrottleDuration_LargeBody` (body=51200 kbps=10 fill=50ms → ticks=100 → duration=5s; verifies no overflow). **Group 4 — DecodeHeaders + DecodeData throttle (decode-side; per SPEC §14.1 #3 + §6.7):** `TestDecodeHeaders_EnableModeRequest_RequestActiveTrue` (mode=REQUEST → f.requestActive=true; f.responseActive=false), `TestDecodeHeaders_EnableModeResponse_ResponseActiveTrue` (mode=RESPONSE → f.requestActive=false; f.responseActive=true), `TestDecodeHeaders_EnableModeBoth_BothActive` (mode=REQUEST_AND_RESPONSE → both true), `TestDecodeHeaders_EnableModeDisabled_BothFalse` (mode=DISABLED → both false), `TestDecodeHeaders_PerRouteResolution_CachesRC` (per-route TPFC msg returned by dcb.RequestRouteConfig → f.requestRC points to per-route compiledConfig), `TestDecodeData_PassthroughWhenInactive_DataContinue` (requestActive=false → DataContinue regardless of body), `TestDecodeData_BufferedAccumulation_PreEndStream` (multi-chunk body; pre-endStream chunks return DataStopIterationAndBuffer; f.requestBody accumulates), `TestDecodeData_EndStream_ZeroBody_FastPath` (empty body → throttle=0 → DataContinue + stats incremented but no timer arm), `TestDecodeData_EndStream_SmallBody_OneTickFloor` (100-byte body @ kbps=10 fill=50ms → one-tick floor → timer arms for 50ms + DataStopIterationAndBuffer), `TestDecodeData_EndStream_LargeBody_MultiTick` (4000-byte body @ kbps=10 fill=50ms → 8 ticks → 400ms throttle), `TestDecodeData_TimerFire_IncrementEnforcedByTicks` (verify *_enforced bumped by exact ticks count at timer-fire per §11.P3 cumulative-match discipline), `TestDecodeData_TimerFire_ContinueDecodingInvoked` (verify dcb.ContinueDecoding called from timer callback). **Group 5 — EncodeHeaders + EncodeData throttle (encode-side; per SPEC §14.1 #4 + §6.8):** symmetric mirror of Group 4 — `TestEncodeHeaders_NoOp` (returns HeaderContinue; responseRC was cached at DecodeHeaders), `TestEncodeData_PassthroughWhenInactive_DataContinue`, `TestEncodeData_BufferedAccumulation_PreEndStream`, `TestEncodeData_EndStream_ZeroBody_FastPath`, `TestEncodeData_EndStream_SmallBody_OneTickFloor`, `TestEncodeData_EndStream_LargeBody_MultiTick`, `TestEncodeData_TimerFire_IncrementEnforcedByTicks`, `TestEncodeData_TimerFire_ContinueEncodingInvoked` (verify ecb.ContinueEncoding called from timer callback). **Group 6 — OnDestroy + timer cleanup + Stop-races-Fire (per SPEC §14.1 #5 + §4 + §6.9 + planner-time decision 3):** `TestOnDestroy_NoTimer_NoOp` (both timers nil → OnDestroy is no-op), `TestOnDestroy_TimerActive_StopReturnsTrue_DecPending` (arm timer with long throttle; OnDestroy before fire → Stop returns true → *_pending decremented here; no double-Dec), `TestOnDestroy_TimerFired_StopReturnsFalse_TrustCallback` (arm timer with short throttle; sleep past fire; OnDestroy → Stop returns false → trust callback's already-issued Dec; no double-Dec), `TestOnDestroy_RaceConcurrent_NoDoubleDecrement` (goroutine-driven concurrent OnDestroy + timer-fire race-test; `go test -race`; assert *_pending final value = 0 across N iterations; no panic / no negative gauge), `TestOnDestroy_BothDirectionsActive_BothCleanedUp` (REQUEST_AND_RESPONSE mode with both timers armed; OnDestroy stops both; both pending gauges balanced). **Group 7 — Per-route INDEPENDENT-stats wiring (per SPEC §14.1 #6 + §5 + §11.P4 + planner-time decision 5):** `TestPerRoute_IndependentStats_Allocated` (per-route override with own stat_prefix → newFilterStatsIfAbsent allocates wholly-own counter set), `TestPerRoute_IndependentStats_ListenerUnaffected` (load against per-route route → listener-level counters stay at 0; per-route counters increment), `TestPerRoute_DisableViaEnableModeDISABLED_NoCounterIncrements` (per-route `enable_mode: DISABLED` → wholly inactive; namespace registered but counters stay 0 per §11.P12), `TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute` (listener-level `enable_mode: DISABLED` produces identical wire output + counter footprint as per-route `enable_mode: DISABLED` per planner-time decision 6 + §12 deferred #5), `TestPerRoute_LazyCache_SyncMapKey` (multi-request load against same per-route entry → single allocation via LoadOrStore; verified by counting `*compiledConfig` allocations). **Group 8 — Stats namespace integration (per planner-time decision 9 + ADR-0138 + §11.P10 + §11.P11):** `TestStatsNamespace_AllFourteenActiveStatsRegistered` (8 counters + 6 gauges per stat_prefix), `TestStatsNamespace_UnderscoreInfix_NotHCMRooted` (verify internal path `<stat_prefix>.http_bandwidth_limit.<counter>` per §11.P11 — no `http.<HCM>.` prefix), `TestStatsNamespace_PromInlineFlatten_NoSN10` (verify Prometheus rendering `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` via existing default-branch flatten — NO label / NO tag-extractor / NO new SN10 rule per §11.P10 + amendment 8), `TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent` (multi-call `newFilterStatsIfAbsent` returns pointer-identity-equivalent stats per ADR-0117 post-Freeze idempotency). Test helpers `mustAny(t, msg proto.Message) *anypb.Any` + `freshFactoryCtx() envoyhttp.FactoryCtx` + `freshFactoryCtxWithRegistry() envoyhttp.FactoryCtx` mirror phase-13/14 precedents. Reference: phase-11 `local_ratelimit_test.go` (Group 1 + 2 + 3 + 4 for parse + bucket + DecodeHeaders + per-route stats) + phase-09 `fault_test.go` for the OnDestroy + timer + race-test discipline. ~700-900 LoC total covering ~50-65 test cases. |
| `internal/filter/http/bandwidthlimit/fuzz_test.go` | NEW | `FuzzBandwidthLimitConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. The fuzzer's interesting axes: random bytes vs. partial proto-shaped bytes vs. valid proto with random field values (notably `limit_kbps` and `fill_interval` extremes). Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the bandwidth_limit `New` factory is a parse-rejection surface (7 top-level fields + PGV-mirror validation per amendments 3 + 4 + 5). ~80 LoC; 30s budget per ADR-0018; **nineteenth fuzzer overall** (post-14's eighteenth `FuzzCompressorConfigParse`). Seed corpus: 6 valid-config seeds (default-everything with limit_kbps=10; explicit fill_interval=20ms; explicit limit_kbps=1000; enable_mode=REQUEST_AND_RESPONSE; runtime_enabled with default_value=false; response_trailer_prefix set) + 4 invalid-config seeds (nil typed_config; empty stat_prefix; limit_kbps=0; fill_interval=10ms). |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)` registration inserted IMMEDIATELY AFTER the existing `httpReg.Register(router.TypeURL, router.New)` line and BEFORE the existing `httpReg.Register(buffer.TypeURL, buffer.New)` line per ADR-0100 §2.2 router-first-then-alphabetical stylistic discipline (codified at phase 9 brainstorm time + reaffirmed at phases 10-14). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"` alphabetically among the existing filter-package imports (currently `bandwidthlimit` slots between `buffer` and `compressor`? — actually alphabetically `b` < `bu` so `bandwidthlimit` comes before `buffer`). The resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New); httpReg.Register(buffer.TypeURL, buffer.New); httpReg.Register(compressor.TypeURL, compressor.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(csrf.TypeURL, csrf.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Register(localratelimit.TypeURL, localratelimit.New); header_mutation.RegisterPerRouteValidator(httpReg); httpReg.Freeze()`. **No other wiring changes** — bandwidth_limit is HTTP-only, no listener/cluster/drain manager threading; no per-route-validator registration call (bandwidth_limit's per-route TPFC parsing happens at HCM-build via `BuildPerRouteConfig`'s generic `UnmarshalNew` since per-route uses the same `BandwidthLimit` proto — same discipline as phase-11 local_ratelimit per ADR-0117 IMPL-1; NOT phase-10 header_mutation's typed validator pattern). ~+3 LoC delta (1 import line + 1 register line). Per SPEC §1 item 2 + ADR-0135. |
| `test/helpers/echobackend/` | UNCHANGED | Already exists from phase 14 (`test/helpers/echobackend/echobackend.go` + `doc.go` + `echobackend_test.go` + `cmd/echobackend/main.go`). Phase 15 fixture 0017 reuses this helper for scenarios 2 + 3 (request-side + REQUEST_AND_RESPONSE) which need an echo-backend cluster `c_backend_b` to assert upstream-arrival time independent of response-side throttle. No new helper authored at phase 15. |
| `test/differential/fixture/fixture.go` | MODIFIED | NEW `HTTPBandwidthLimit BackendKind = 14` enum value + doc-comment matching the format used for `HTTPCompressor BackendKind = 13` mentioning the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go`. ~+15 LoC delta. Per planner-time decision 12. |
| `test/differential/runner_test.go` | MODIFIED | NEW blank-import addition for the fixture driver pkg + new switch-case in the BackendKind dispatch logic for `HTTPBandwidthLimit` to spawn the echobackend (reuses the existing `startEchoBackend` helper introduced at phase-14 Task 10). ~+15 LoC delta. |
| `test/fixtures/0017-http-bandwidth-limit/` | NEW DIRECTORY | Differential fixture with 6 scenarios per SPEC §7. Contents below. |
| `test/fixtures/0017-http-bandwidth-limit/envoy.yaml` | NEW | Reference Envoy bootstrap with TWO listeners (l_test_a + l_test_b) + cluster c_backend_b per SPEC §7.2. Listener `l_test_a` (TCP plaintext; HCM filter chain `bandwidth_limit → router`) with listener-level config `stat_prefix: default, enable_mode: REQUEST_AND_RESPONSE, limit_kbps: 10, fill_interval: 50ms` per SPEC §7.2 + 6 routes (`/echo-response` direct_response 10 KiB; `/echo-request` cluster c_backend_b with per-route `enable_mode: REQUEST`; `/echo-both` cluster c_backend_b inheriting listener REQUEST_AND_RESPONSE; `/echo-tiny` direct_response 100 bytes inheriting listener; `/echo-disabled` direct_response 10 KiB with per-route `enable_mode: DISABLED` per §1.1 amendment 1 disable mechanism — still requires `limit_kbps` to be set per amendment 4 + §11.P1; `/echo-override` direct_response 10 KiB with per-route `stat_prefix: override, enable_mode: RESPONSE, limit_kbps: 100, fill_interval: 50ms` per scenario 6). Listener `l_test_b` is the upstream echo-backend listener; cluster `c_backend_b` connects to it. **No Vary / No Content-Encoding / No trailer-emission** (bandwidth_limit does NOT mutate headers). ~110 LoC. |
| `test/fixtures/0017-http-bandwidth-limit/envoy-go.yaml` | NEW | Equivalent envoy-go config; same two-listener topology; same route+per-route map. ~110 LoC. |
| `test/fixtures/0017-http-bandwidth-limit/inputs/driver.go` | NEW | Go driver issuing the 6 scenarios per SPEC §7.4 mirroring phase 11/13/14 driver shape. Functions `runScenario1..runScenario6(ctx, baseURL) error`. Wall-clock timing helper `measureRequestDuration(ctx, req) (resp, duration, error)`. Per-scenario assertion helper for byte-exact body (10 KiB scenarios) + wall-clock within **±70ms** tolerance per SPEC §11.P9 + §7.3 + §13.5. Stats scrape per scenario; counter-delta computation against pre-scrape baseline asserting the 14 active stats per stat_prefix (8 counters + 6 gauges). Histograms allow-list filter (strip `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_*` lines from scrape before delta comparison per amendment 9 + §242 twin-series-filter extension). For scenarios 2 + 3 (request-side throttle), the echo-backend's `X-Echo-Received-At` timestamp header is asserted against `time.Now()` at request-issue to measure upstream-arrival time independent of response-side throttle. Total estimated driver size: ~220 LoC. |
| `test/fixtures/0017-http-bandwidth-limit/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7.3. **Twin-series filter** allow-listing the 2 unconditional Envoy histograms `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_*` per amendment 9. ~55 LoC. |
| `test/fixtures/0017-http-bandwidth-limit/README.md` | NEW | Fixture overview + scenario list (6 scenarios; per-scenario timing predictions from SPEC §7.1) + reference config citations + **±70ms tolerance discipline** (per §11.P9) + **KiB/s units note** (per §1.1 amendment 6) + **histograms allow-list note** (per amendment 9 + §242 twin-series-filter discipline). ~90 LoC. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | 5 NEW ADRs (ADR-0135 + ADR-0136 + ADR-0137 + ADR-0138 + ADR-0139) authored at impl-time per ADR-0044 ADR-on-impl convention — Lands-in-task: ADR-0135 at Task 2; ADR-0136 at Task 2; ADR-0137 at Task 3 (kbps-per-tick bucket helper landing); ADR-0138 at Task 8; ADR-0139 at Task 7. Plus ADR-0125 §(xi) amendment paragraph **ALREADY LANDED** at SPEC commit `49e0361` per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) in-place-update precedent; PLAN does not re-anchor it. ~+400 LoC across the 5 new ADRs. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 patches — §13.1 `### envoy.filters.http.bandwidth_limit` subsection inserted **AFTER `### envoy.filters.http.compressor` at line 1302** (landing-chronological per phase-13/14 precedent, NOT alphabetical-canonical as SPEC §13.1 stub-text claims — see planner-time decision 16 for the disposition); §13.2 stat-table 46→60 names extension (14 new active rows + 2 deferred-histogram rows in allow-list note); §13.3 NEW equivalence-matrix row pointing at fixture 0017 with ±70ms tolerance + histograms allow-list note; §13.4 NEW `### Phase 15 forward-pointer notes` subsection (~80 lines covering the ~8-item deferral list + foot-gun + histogram divergence + chunk-pattern divergence + KiB/s units note + ZERO framework deltas claim + no `BandwidthLimitPerRoute` proto note); §13.5 `## Timing tolerances` extension with ±70ms entry; `### Twin-series filter discipline` extension with the phase-15 histogram allow-list entry. ~+275 LoC total. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row 15 status `in-progress → done` flip + summary sharpening (post-amendment counts; PLAN-confirmed 16-task + ~413-503 LoC production estimate) ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state: lifecycle-state `phase 15 done; awaiting next planning`; next-skill (none — phase complete); next-active-phase TBD by ROADMAP. |
| `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror phase 04-14 PROGRESS.md structure. ~650 LoC across 16 task entries. |
| `docs/envoy-go/phases/15-http-filter-bandwidth-limit/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~200 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's eight deferred decisions before implementation; this PLAN settles all eight plus eight that emerged at PLAN-drafting time (items 9–16 below). The sixteen resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **D1 — `bandwidthlimit.go` file split = TWO-WAY SPLIT: `bandwidthlimit.go` (filter + factory + types + helpers + Encode/Decode methods + perroute resolver + filterStats) + `bucket.go` (the kbps-per-tick `throttleDuration` helper).** Per SPEC §12 #1 + §6 PLAN-author option + SPEC §15 acceptance #1 verbatim file enumeration `{bandwidthlimit.go, bucket.go, bandwidthlimit_test.go, fuzz_test.go, doc.go}`. The total filter surface estimate of ~390-470 LoC stays under the project's general 200-300 LoC mental-model threshold when split (main file ~320-380 + bucket.go ~60-90). The natural split shape per SPEC §6 places the throttle-duration arithmetic (a pure function with a closed-form formula + empirical-verification-matrix comment block) in its own file matching phase-11 `bucket.go`'s structural separation. The 2-way split keeps `bandwidthlimit.go` at acceptable mental-model load while extracting the throttle math + the foot-gun branch + the one-tick floor into `bucket.go` (tight, focused, easy to unit-test in Group 3 in isolation). Mirrors phase-11 `local_ratelimit.go` + `bucket.go` split rationale (the `tokenBucket` was a separable primitive; phase-15's `throttleDuration` is similarly separable, though pure-function with no state vs phase-11's stateful token-bucket). DIVERGES from phase-13 buffer + phase-12 csrf single-file precedents because phase-15's algorithmic core has a closed-form formula that benefits from isolation. *Anchored: SPEC §12 #1; SPEC §15 acceptance #1 verbatim file list; SPEC §6 file-split shape; phase-11 `local_ratelimit.go` + `bucket.go` precedent; project mental-model threshold.*

2. **D2 — Fast-passthrough threshold = RESOLVED AT SPEC TIME via the one-tick `fill_interval` floor.** Per SPEC §12 #2 + §6.6. The BRAINSTORM-hypothesized `1ms` fast-passthrough threshold is REPLACED by the natural one-tick floor (bodies fitting within `chunk_size_per_tick` still wait one `fill_interval` to match Envoy's per-tick behavior within ±70ms tolerance per §11.P9). PLAN adds only the sub-tick short-circuit `throttle = 0` for `bodySize == 0` (empty body; trivially no throttle). No further fast-passthrough refinement anticipated. The `TestThrottleDuration_OneTickFloor` test in Group 3 verifies. *Anchored: SPEC §12 #2 + §6.6; phase-09 fault's `time.AfterFunc` precedent's lack of fast-passthrough.*

3. **D3 — Pending-gauge Inc/Dec under Stop-races-Fire window = `Stop() returns true → Dec here; ==false → trust the callback` PER SPEC §6.9; PLAN-time race-test in Group 6 `TestOnDestroy_RaceConcurrent_NoDoubleDecrement` validates no double-decrement under aggressive concurrent OnDestroy + timer-fire scenarios.** Per SPEC §12 #3 + §4 + §6.9. The PLAN-time race-test design: goroutine-driven concurrent OnDestroy + timer-fire race (e.g., 100 concurrent iterations spawning a stream + arming a 1ms timer + invoking OnDestroy from a parallel goroutine while the timer is about to fire); `go test -race` MUST stay clean; the final pending-gauge value MUST be 0 across all iterations (no negative gauge, no orphan +1, no panic). If the test surfaces flakiness, PLAN's fallback is to add an explicit `markedActive atomic.Bool` field on `*filter` per phase-09 fault precedent at `fault.go:480-500` — the markedActive.CompareAndSwap(true, false) gate provides race-clean exactly-once semantics; the impl-task author adapts. SPEC's position: simpler `Stop()` bool discriminator suffices because (a) the timer's Stop() return value is documented atomic; (b) the callback's pending.Dec() runs from the timer goroutine; (c) the OnDestroy path's pending.Dec() (when Stop()==true) runs from the dispatch goroutine — these are mutually exclusive by the Stop() contract, so no double-Dec under any goroutine interleaving. PLAN concurs with SPEC's simpler shape + adds the race-test as the load-bearing validation. *Anchored: SPEC §12 #3 + §4 + §6.9; phase-09 fault `markedActive` race-clean precedent at `fault.go:441-465`; ADR-0105 race-clean discipline.*

4. **D4 — Per-route stat-counter cardinality bound = SILENT-ALLOW IN MVP; no explicit cap.** Per SPEC §12 #4 + ADR-0117 + §5 + §1.1 amendment 7. Each per-route TPFC entry allocates 14 active stats (8 counters + 6 gauges) at first-resolve; N per-route entries → 14N stats post-Freeze. PLAN settles silent-allow (matches phase-11 local_ratelimit's analogous discipline at 4 stats × N entries; no cap was added then either). Future stats-cardinality-governance phase MAY introduce a cap (operator-facing knob); SPEC's position: silent-allow in MVP. Documented at §13.4 phase-15 forward-pointer notes. *Anchored: SPEC §12 #4 + §1.1 amendment 7; phase-11 ADR-0117 silent-allow precedent; ADR-0040 silent-ignore-extension discipline.*

5. **D5 — `enable_mode: DISABLED` at listener-level vs per-route observable parity = UNIT-TEST PARITY ASSERTION in Group 7 (`TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute`).** Per SPEC §12 #5 + §11.P12. PLAN settles: verify both shapes (listener-level `enable_mode: DISABLED` AND per-route `enable_mode: DISABLED`) produce identical wire output (full passthrough; body bytes flow immediately; sub-50ms wall-clock for 10 KiB body) AND identical counter-delta footprint (all 14 stat names registered at 0 — namespace allocated but no increments per §11.P12). The Group 7 test exercises both code paths in the same test fixture; differential fixture scenario 5 covers the per-route shape; an equivalent listener-level scenario is unit-test-only (not exercised in differential fixture 0017 since the listener-level config has `enable_mode: REQUEST_AND_RESPONSE` per SPEC §7.2). *Anchored: SPEC §12 #5 + §11.P12; phase-11 + phase-14 unit-test parity discipline.*

6. **D6 — `fill_interval` granularity in throttle math = RESOLVED AT SPEC TIME via kbps-per-tick formula.** Per SPEC §12 #6 + §6.6 + §11.P15. The kbps-per-tick chunking discipline (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`; `throttle = ceil(body/chunk_size) × fill_interval`) is the SPEC-authored disposition. PLAN concurs and implements verbatim per §6.6. The `*_enforced` increment-by-`ticks` discipline at timer-fire (per SPEC §6.7 + §11.P3 + amendment 7) is the cumulative-counter byte-equivalence semantic; per-counter delta assertions in fixture 0017 use this convention. PLAN settles increment-by-ticks (NOT once-per-stream). The Group 3 test matrix verifies the formula against SPEC §6.6's 5-row empirical-verification table; the Group 4 + 5 tests verify the `*_enforced += ticks` semantic at timer-fire. *Anchored: SPEC §12 #6 + §6.6 + §6.7 + §11.P3 + §1.1 amendment 7.*

7. **D7 — Per-route `runtime_enabled` field interaction = SILENT-IGNORE AT PARSE + RUNTIME with Group 2 unit test asserting field is parsed but not honored at per-route position.** Per SPEC §12 #7 + §1.1 amendment via §11.P6. Both listener-level and per-route TPFC silent-ignore `runtime_enabled` (always-100%-active). PLAN settles: Group 2's `TestParsePerRoute_RuntimeEnabledOverride_SilentIgnored` test asserts the field is parsed without error AND not stored on `compiledConfig` (no runtime-enabled field exists on the struct); the filter is always-active regardless. Operator divergence-window if `default_value: false` set on either side; documented at §13.4 forward-pointer notes. *Anchored: SPEC §12 #7 + §11.P6 + ADR-0117/ADR-0121/ADR-0130 silent-ignore-runtime-flag precedent.*

8. **D8 — Trailer-emission framework primitive forward-pointer = SILENT-IGNORE AT PARSE + RUNTIME with Group 1 unit test asserting no trailers regardless of `enable_response_trailers` setting.** Per SPEC §12 #8 + §8.1 + §1.1 amendment via §11.P7. PLAN settles: Group 1's `TestBuildCompiledConfig_EnableResponseTrailers_SilentIgnored` + `TestBuildCompiledConfig_ResponseTrailerPrefix_SilentIgnored` tests assert both fields parse without error AND the runtime path emits no trailers regardless (no `EncodeTrailers` mutation; the encode path's `DecodeTrailers` + `EncodeTrailers` are pass-through). Operator divergence-window if `enable_response_trailers: true` set on Envoy side (sees 4 `<prefix>bandwidth-{request,response}{,-filter}-delay-ms` trailers per §11.P7); envoy-go responses have no trailers. Documented at §13.4 forward-pointer notes. *Anchored: SPEC §12 #8 + §8.1 + §11.P7.*

9. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: f` SAME *filter instance.** Per ADR-0135 (planned) + SPEC §6.4 + planner-time decision 10. Phase 15 generalizes phase-14 compressor's same-`*filter` shape (which had asymmetric encoder-primary surface) to SYMMETRIC BOTH-direction. Per-stream state lives on the `*filter` struct so the encode-side throttle reads what the decode-side resolved at DecodeHeaders. Setting `Encoder: nil` would force per-route resolution into encode-side, but `dcb.RequestRouteConfig()` is decode-side only — per-route resolution at decode-side is structurally honest. ADR-0135 §Decision (iv) records. *Anchored: phase-14 ADR-0129 §Decision (iv) same-`*filter` precedent; SPEC §6.1 + §6.4; ADR-0135 (planned at Task 2).*

10. **PLAN-emerging — Filter-callback wiring hooks = BOTH `SetDecoderCallbacks(cb)` AND `SetEncoderCallbacks(cb)`; both store on the SAME *filter instance.** Implementer adds two methods on `*filter`: `func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }` + `func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }`. The framework's per-stream state machine calls both methods once per stream; the filter stores both references — `f.dcb` is used at `DecodeHeaders` for `RequestRouteConfig()` + at `DecodeData` timer-fire for `ContinueDecoding`; `f.ecb` is used at `EncodeData` timer-fire for `ContinueEncoding`. *Anchored: `internal/filter/http/types.go:75-78` HTTPFilter shape; ADR-0071 iteration protocol; SPEC §6.1 + §6.9; phase-14 ADR-0129 same-`*filter` precedent; decision 9 above.*

11. **PLAN-emerging — Fixture topology = TWO LISTENERS `l_test_a` + `l_test_b` with CLUSTER `c_backend_b` echoing back.** Per SPEC §7.2. Phase 15's 6 scenarios split across `l_test_a`'s 6 routes; scenarios 2 + 3 (request-side throttle) need an echo-backend to measure upstream-arrival time independent of response-side throttle; `l_test_b` is the upstream backend listener; `c_backend_b` is the cluster connecting them. The two-listener topology fits the existing `fixture.MultiListenerDriver` contract (mirrors phase-11 local_ratelimit fixture 0013's multi-listener topology). Echo-backend reuses the existing `test/helpers/echobackend/` shared helper introduced at phase-14 Task 10 (no new helper authored at phase 15). *Anchored: SPEC §7.2 + §7.4; phase-11 multi-listener precedent (0013); phase-14 echobackend helper shared-resource decision.*

12. **PLAN-emerging — BackendKind enum value = `HTTPBandwidthLimit BackendKind = 14`** (continues existing naming convention; next value after phase 14's `HTTPCompressor BackendKind = 13` at `test/differential/fixture/fixture.go`). Doc-comment matches the format used for `HTTPCompressor` mentioning the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go`. *Anchored: phase 14 PLAN planner-time decision 13 precedent; existing enum at `test/differential/fixture/fixture.go`.*

13. **PLAN-emerging — ADR anchor schedule per ADR-0044 ADR-on-impl convention = ADR-0135 + ADR-0136 at Task 2; ADR-0137 at Task 3 (bucket.go landing); ADR-0138 at Task 8 (14-stat filterStats registration finalization); ADR-0139 at Task 7 (per-route INDEPENDENT-stats wiring).** Per SPEC §8 + ADR-0044. Phase-15 ADRs are NOT pre-landed at SPEC commit (UNLIKE phase-14 compressor's pre-landing); they land at impl-time tasks in their final form per the phase-13 buffer convention. The implementer at each anchor task AUTHORS the ADR §Context/Decision/Consequences body (using the SPEC §8 roster entries + the per-section content scattered across SPEC §1-§13 as the source-of-truth) AND includes the ADR in the commit message (`"phase 15: ... [ADR-XXXX]"`) AND verifies the `Lands-in-task: Task N` field matches via `grep -nE '^Lands-in-task: Task N' docs/envoy-go/DECISIONS.md`. ADR-0125 §(xi) amendment paragraph ALREADY LANDED at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent — no PLAN-time re-anchor for ADR-0125 either. *Anchored: SPEC §8 + ADR-0044; phase-13 buffer ADR-on-impl convention (ADR-0125 + ADR-0126 + ADR-0127 + ADR-0128 all landed at impl-time tasks 2/3/4/12 NOT at SPEC commit); phase-14 compressor SPEC-time-pre-landing was the DIVERGENT precedent; phase-15 returns to phase-13 convention.*

14. **PLAN-emerging — Acceptance discipline at the per-task level = each task's acceptance bullet enumerates the verbatim verification commands AND the expected-output anchors (verbatim file contents OR command exit codes); per-task ADR-anchor verification (each task referencing an ADR confirms the ADR's `Lands-in-task: Task N` field matches AND the ADR text exists at HEAD).** Per phase-13/14 PLAN per-task acceptance precedent. Each impl task carries an `acceptance` paragraph naming the verbatim post-conditions (e.g., `go test -race -count=1 ./internal/filter/http/bandwidthlimit/` exit 0; `grep -nE '^## ADR-0137' docs/envoy-go/DECISIONS.md` returns 1 match; `git log -1 --format=%H -- ...` returns the just-committed SHA). The implementer copies the verbatim acceptance commands into PROGRESS.md per task. *Anchored: phase-13/14 PLAN per-task acceptance precedent; ADR-0044 ADR-on-impl first-use-commit reference discipline.*

15. **PLAN-emerging — `*_enforced` counter increment-by-`ticks` cumulative-match discipline.** Per SPEC §6.7 + §11.P3 + amendment 7. At timer-fire, the callback increments `*_enforced` by the `ticks` value returned from `throttleDuration` (not once-per-stream). This produces cumulative byte-equivalence with reference Envoy's per-fill_interval-tick increment semantic (Envoy bumps `*_enforced += 1` per chunk-emit during the throttle window; for an N-tick throttle, Envoy's cumulative `*_enforced` += N; envoy-go matches by bumping `+= ticks` once at the buffered-blast timer-fire). The Group 4 + 5 + 8 tests verify the convention; fixture 0017 scenario 1 expected `response_enforced += 20` for 10240-byte body at chunk_size=512 (20 ticks). *Anchored: SPEC §6.7 + §11.P3 + §1.1 amendment 7 + §13.4 forward-pointer notes `*_enforced` counter-semantic note.*

16. **PLAN-emerging — BEHAVIOR_CONTRACT.md `### envoy.filters.http.bandwidth_limit` subsection insertion point = AFTER `### envoy.filters.http.compressor` at line 1302 (landing-chronological per phase-13/14 precedent), NOT at HEAD per SPEC §13.1 stub-text alphabetical-canonical claim.** Per planner-time disposition resolving SPEC §13.1 inaccuracy. The CURRENT BEHAVIOR_CONTRACT.md has subsections ordered chronologically by landing phase (fault@921 < header_mutation@978 < local_ratelimit@1047 < csrf@1121 < buffer@1175 < compressor@1302), NOT alphabetically. SPEC §13.1 stub-text claims "alphabetical-canonical ordering of the existing subsection list is `bandwidth_limit < buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit`, so the new `### envoy.filters.http.bandwidth_limit` subsection inserts at the HEAD of the list" — this is inaccurate against the observed file state at master tip. PLAN settles: insert AFTER `### envoy.filters.http.compressor` (line 1302) preserving the landing-chronological order convention. The SPEC inaccuracy is documented in PROGRESS.md preamble; no SPEC patch is required (SPEC stays the authoritative design doc; PLAN is the authoritative impl shape). *Anchored: BEHAVIOR_CONTRACT.md observed state at master tip cd45af0 (lines 921 / 978 / 1047 / 1121 / 1175 / 1302); phase-13/14 PLAN insertion-point precedent (always inserted after the prior phase's subsection, NOT at alphabetical HEAD).*

These sixteen decisions are reproduced verbatim in `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The five ADRs anticipated by SPEC §8 (ADR-0135..ADR-0139). **AUTHORED AT IMPL-TIME per ADR-0044 ADR-on-impl convention** (phase-13 buffer pattern; UNLIKE phase-14 compressor's SPEC-time-pre-landing). Per-ADR Lands-in-task anchors:

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0135 | `internal/filter/http/bandwidthlimit/` package shape — single-token directory + ENCODER+DECODER `HTTPFilter` value with SAME *filter instance + 14-active-stat `filterStats` (8 counters + 6 gauges per §1.1 amendment 7) + ZERO framework deltas (FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 simultaneously) + boot-registration ordering | Task 2 (package skeleton + types + factory) |
| ADR-0136 | `compiledConfig` shape + 4-consumed/3-silent-ignored field decomposition + PGV-mirror filter-internal validation discipline + CODE-LEVEL extra check at per-route position for `limit_kbps` REQUIRED + envoy-go-own error wording | Task 2 (buildCompiledConfig + buildCompiledConfigPerRoute land here) |
| ADR-0137 | Body algorithm Path B-async (buffer-then-delayed-emit) with kbps-per-tick throttle math (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`; KiB/s units NOT kbps) + `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume reuse from phase-09 fault + wire-shape divergence-window (envoy-go silent-then-blast vs Envoy Path A rate-paced chunks; total-throttle-time equivalent ±70ms; chunk-arrival-time divergent) + `cb.OverwriteBody` NOT invoked + forward-pointer to future encode-side streaming framework phase + `*_enforced` increment-by-`ticks` cumulative-match discipline | Task 3 (`bucket.go` `throttleDuration` helper landing — the algorithmic core; the decode/encode-side wiring lands at Tasks 4 + 5 which CONSUME the helper) |
| ADR-0138 | 14-active-stat surface (8 counters + 6 gauges) per §1.1 amendment 7 + 8 + 9 + namespace shape `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; NOT HCM-rooted) per §11.P11 + Prometheus inlines stat_prefix into base name `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` (NO tag-extractor; NO new SN10 rule) per §11.P10 + histograms divergence-window (Envoy emits 2 unconditional transfer_duration; envoy-go MVP skips per phase-06.1 baseline; twin-series-filter discipline) + INDEPENDENT per-route stats + `*_enforced` increments by `ticks` per stream | Task 8 (newFilterStats finalization + stat-namespace integration tests; the filterStats struct declaration lands at Task 2 but the full namespace-shape codification + the per-stat semantic definitions land here) |
| ADR-0139 | Per-route INDEPENDENT-stats ratification — codifies phase 15 as SECOND row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent + phase-15's per-route discipline is a NEW canonical pattern (bare-`BandwidthLimit`-via-TPFC + code-level-required-`limit_kbps`-at-per-route) distinct from BOTH the 4th canonical (phase-11 ADR-0117 same proto but no code-level check) AND the 5th canonical (phase-13/14 ADR-0125 wrapper proto with oneof) + ADR-0125 §(xi) amendment paragraph LANDED at SPEC commit documenting the new pattern | Task 7 (resolvePerRouteConfig + sync.Map lazy-cache + newFilterStatsIfAbsent post-Freeze idempotent wiring) |

**Plus ADR-0125 amendment paragraph §(xi)** — ALREADY landed at SPEC commit `49e0361` per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) in-place-update precedent. No PLAN-time re-anchor; impl tasks reference §(xi) per ADR-0125's existing amendment text. The amendment documents: phase-15 was BRAINSTORM-hypothesized to be the THIRD row using disabled-OR-override 5th canonical; SPEC-time §11.P1 empirically REFUTED; phase-15 instead introduces a NEW canonical pattern (bare-message-via-TPFC + code-level required-limit_kbps at per-route position) that sits adjacent to the 4th canonical (phase-11 ADR-0117) as the 6th canonical entry in ADR-0125's discipline-shape catalog.

The implementer at each impl-anchor task AUTHORS the ADR §Context/Decision/Consequences body in DECISIONS.md (in the slot immediately after the prior ADR; ADR-0135 inserts after ADR-0134; ADR-0136 inserts after ADR-0135; etc.) AND includes the ADR in the commit message AND verifies via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning 1 match (the canonical authoring-check) AND verifies `Lands-in-task: Task N` field via `grep -nE '^Lands-in-task: Task N' docs/envoy-go/DECISIONS.md`.

**Inline supersessions / amendments anticipated** (cross-references only; **NO in-place ADR edits required by phase 15** — this is consistent with phases 12 + 13 + 14; UNLIKE phases 10 + 11 which each amended ADR-0073):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — UNCHANGED in phase 15. Phase 15's per-route is data-only (same `BandwidthLimit` proto reuse via TPFC). The wholesale-override discipline applies as-is. ADR-0125 amendment §(xi) (already landed at SPEC commit) extends ADR-0125's canonical-shape catalog to 6 entries without amending ADR-0073. NO in-place edit of ADR-0073.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 15. The 8-item deferral list (per SPEC §12) is captured INLINE at BEHAVIOR_CONTRACT §13.4 (the `### Phase 15 forward-pointer notes` subsection). NO new deferral ADRs (mirrors phase 10-14 SPEC §8.1 collapse precedent).
- **ADR-0044** (ADR-on-impl convention) — UNCHANGED in phase 15. The 5 ADRs (ADR-0135..ADR-0139) each carry a `Lands-in-task` field anchored at the first-use impl-task; the ADR body is authored at impl-time per the phase-13 convention.
- **ADR-0061** (stats Registry + SN1–SN9 rules) — UNCHANGED in phase 15. NO new SN flattening rule per SPEC §1.1 amendment 8 + §11.P10 + ADR-0138 §Decision (planned). Bandwidth_limit reuses the existing `internal/stats/name.go` default-branch flatten (dot→underscore substitution); the path `<stat_prefix>.http_bandwidth_limit.<counter>` flattens to Prometheus `envoy_<stat_prefix>_http_bandwidth_limit_<counter>` without any per-segment label promotion. Cross-reference recorded in ADR-0138 §Decision. NO in-place edit.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — UNCHANGED in phase 15. Cross-reference recorded in ADR-0135 §Consequences. NO in-place edit.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0135 §Consequences. The filter set extends from {bandwidthlimit absent, buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router} to {bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router}. NO in-place edit of ADR-0074.
- **ADR-0075** (HCM dispatch — wire-write path) — UNCHANGED in phase 15. The existing wire-write paths at `connection.go` + `h2dispatch.go` carry through unchanged; phase-15's `time.AfterFunc` + `ContinueDecoding/Encoding` async-resume mechanics are filter-chain-level, not wire-write-level. NO in-place edit.
- **ADR-0076** (framework body-buffer cap — `filterBufferLimitBytes = 1 << 20`) — UNCHANGED in phase 15. The cap stays armed on both sides; bodies > 1 MiB observe connection-level reset before reaching bandwidth_limit's DecodeData/EncodeData per ADR-0076. NO in-place edit.
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 15. Bandwidth_limit's `New` factory CONSUMES `ctx.Stats` to register the 14-counter+gauge filterStats per ADR-0138 §Decision. ADR-0135 §Consequences notes the 14-stat filterStats as the second-largest stat surface per filter to date in §9 family-rows (phase 11 had 4; phase 12 had 3; phase 13 had 0; phase 14 had 17; phase 15 has 14). NO in-place edit.
- **ADR-0101** (runtimeConfig shape + parser pattern) — extended cross-reference recorded in ADR-0136 §Consequences. The bandwidth_limit `compiledConfig` mirrors fault/csrf/buffer/compressor structurally (5 fields). Closure-capture + parse-at-New + read-only-shared-after-New discipline applies as-is. NO in-place edit.
- **ADR-0102** (terminal-replace + StopIteration localReplyDone gate) — UNCHANGED in phase 15. Bandwidth_limit does NOT use SendLocalReply (the throttle path is non-terminating; the timer-fire callback invokes ContinueDecoding/Encoding, not SendLocalReply). Contrasts with phase-11 local_ratelimit which DOES short-circuit on tryConsume-fail. NO in-place edit.
- **ADR-0117** (per-route bucket isolation as ADR-0073 wholesale-override consequence) — UNCHANGED §Decision sections. ADR-0139 directly inherits ADR-0117's machinery verbatim (lazy-cache `sync.Map`, `NewCounterIfAbsent` post-Freeze registration, `resolvePerRouteConfig` accessor); phase-15 is the SECOND row using stateful-override-with-INDEPENDENT-stats. ADR-0125 §(xi) amendment paragraph (LANDED at SPEC commit) documents the relationship. NO in-place edit of ADR-0117.
- **ADR-0125** (5th canonical disabled-OR-override) — §(xi) amendment paragraph ALREADY LANDED at SPEC commit. No further in-place edits.
- **ADR-0128** (HCM framework primitives — synthetic empty-terminal RunDecodeData + post-body CL reconciliation) — UNCHANGED in phase 15. Phase 15 CONSUMES ADR-0128's decode-side body-buffering machinery (the synthetic empty-terminal `RunDecodeData` ensures DecodeData(endStream=true) reaches the filter even on chunked-body EOF; the post-body Content-Length reconciliation is structurally unused by bandwidth_limit since body bytes are unchanged). Cross-reference recorded in ADR-0137 §Decision. NO in-place edit.
- **ADR-0131** (Path B body algorithm + OverwriteBody encode-side primitive) — UNCHANGED in phase 15. Phase 15 anticipates `cb.OverwriteBody` NOT invoked (the buffered-return path returns bytes unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding`). If impl-time framework-survey reveals OverwriteBody IS required for the same-bytes case, phase-15 reuses (not introduces) the primitive; ZERO-framework-deltas claim stays valid. Cross-reference recorded in ADR-0137 §Decision (vi). NO in-place edit.

These fourteen cross-references land at the tasks that anchor each affected ADR (ADR-0135 + ADR-0136 at Task 2; ADR-0137 at Task 3; ADR-0138 at Task 8; ADR-0139 at Task 7). **NO in-place edit of any pre-existing ADR is required by phase 15** — this is consistent with phase 12 + phase 13 + phase 14, divergent from phases 10 + 11 (each of which amended ADR-0073).

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (per the user's persistent preference for git worktrees recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`). The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-15-http-filter-bandwidth-limit-impl \
                 -b phase-15-http-filter-bandwidth-limit-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-15-http-filter-bandwidth-limit-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-15-http-filter-bandwidth-limit-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-15-http-filter-bandwidth-limit-impl -b phase-15-http-filter-bandwidth-limit-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md squash commit `49e0361` and its SHA-fill follow-up `cd45af0` immediately before, then the BRAINSTORM.md commits `fa125f2` (squash) + `e7a26ef` (SHA-fill) + earlier phase 14 commits. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `134`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (phase-15 anticipated ADR-0135..ADR-0139). If it returns `133` or lower, the SPEC commit's ADR-0125 §(xi) amendment must still be in DECISIONS.md (verify via `grep -nE '^## Amendment .per phase 15' docs/envoy-go/DECISIONS.md` returning 1 match).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/15-http-filter-bandwidth-limit/SPEC.md` returns `49e0361` (or descendant). If different, re-read SPEC + re-verify §11 empirical pins.
6. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/15-http-filter-bandwidth-limit/PLAN.md` returns the PLAN commit's SHA (filled at PLAN-session end). If a different SHA OR earlier than the SPEC, PLAN has been amended — re-read PLAN.
7. **Pristine tree.** `git status --porcelain` returns empty.
8. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
9. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014|Test.*0015|Test.*0016'` returns every fixture PASS. The 17 pre-existing fixtures (0000–0016) are the regression baseline.
10. **Pre-existing fuzzers run clean at 30s.** The 18 fuzzers from phases 02–14 run clean. Phase 15 adds the nineteenth (`FuzzBandwidthLimitConfigParse` per Task 9).
11. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
12. **`envoy.extensions.filters.http.bandwidth_limit.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3 BandwidthLimit | head -5` returns the `BandwidthLimit` proto type's exported fields without an `import path failed` error. If it fails, `go mod download` (or `go mod tidy` if a version bump is needed).
13. **Pre-existing `internal/filter/http/bandwidthlimit/` directory does NOT exist.** `test ! -d internal/filter/http/bandwidthlimit && echo "ok: bandwidthlimit absent"` returns success.
14. **Pre-existing `fixture.HTTPBandwidthLimit` does NOT exist.** `grep -nE 'HTTPBandwidthLimit' test/differential/fixture/fixture.go` returns 0 matches.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).
16. **Pre-existing `cmd/envoy-go/main.go` registers exactly the NINE filters expected at master `cd45af0`** — `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `9` matches: `router`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`. If 10+, another filter has been added concurrently; re-verify the registration ordering before adding the bandwidthlimit line.
17. **Pre-existing `BEHAVIOR_CONTRACT.md` carries the phase-14 `### envoy.filters.http.compressor` subsection** — `grep -n '^### envoy.filters.http.compressor' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns 1 match. If 0 matches or different line, the file has drifted; re-read SPEC §13.1 to re-anchor the new bandwidth_limit subsection insertion point.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the 5 ADRs (ADR-0135..ADR-0139) are NOT pre-landed at SPEC commit (phase-13 buffer convention; phase-14 compressor pre-landed which is the divergent precedent). Each ADR is authored AT its impl-time anchor task. The PROGRESS preamble ANTICIPATES the 5 ADRs (with each ADR's Lands-in-task anchor reproduced from this PLAN's per-ADR table) and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-15-http-filter-bandwidth-limit-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-15-http-filter-bandwidth-limit-impl
git log --oneline master | head -10                                   # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (cd45af0), SPEC squash (49e0361), BRAINSTORM SHA-fill (e7a26ef), BRAINSTORM squash (fa125f2), phase-14 commits...
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014|Test.*0015|Test.*0016' -v
                                                                       # expect: every fixture PASS (17 fixtures)
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 134
grep -nE '^## Amendment .per phase 15' docs/envoy-go/DECISIONS.md     # expect: 1 match (the ADR-0125 §(xi) amendment paragraph from SPEC commit)
git log -1 --format=%H -- docs/envoy-go/phases/15-http-filter-bandwidth-limit/SPEC.md
                                                                       # expect: 49e0361... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/bandwidthlimit && echo "ok: bandwidthlimit absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3 BandwidthLimit | head -5
                                                                       # expect: type BandwidthLimit struct { ... }
grep -cE 'HTTPBandwidthLimit' test/differential/fixture/fixture.go    # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
grep -cE 'httpReg.Register' cmd/envoy-go/main.go                      # expect: 9
grep -cn '^### envoy.filters.http.compressor' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: 1
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Author `PROGRESS.md` preamble**

Create `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` with the following structure:

````markdown
# Phase 15 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..14 PROGRESS.md structure.

## Preamble — execution preconditions

(Verbatim 17-precondition output captured during Task 1; all 17 green.)

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The 5 ADRs anticipated by SPEC §8 (ADR-0135..ADR-0139). **AUTHORED AT IMPL-TIME** per ADR-0044 ADR-on-impl convention (phase-13 buffer pattern; UNLIKE phase-14 compressor's SPEC-time-pre-landing). Per-ADR Lands-in-task anchors:

- **ADR-0135** `internal/filter/http/bandwidthlimit/` package shape — Task 2
- **ADR-0136** `compiledConfig` shape + 4-consumed/3-silent-ignored field decomposition + PGV-mirror filter-internal validation + CODE-LEVEL extra check at per-route for `limit_kbps` REQUIRED + envoy-go-own error wording — Task 2
- **ADR-0137** Body algorithm Path B-async + kbps-per-tick throttle math + `time.AfterFunc` async-resume reuse + wire-shape divergence-window + `cb.OverwriteBody` NOT invoked + `*_enforced` increment-by-`ticks` discipline — Task 3
- **ADR-0138** 14-active-stat surface + namespace `<stat_prefix>.http_bandwidth_limit.<counter>` + Prometheus inline-prefix (NO new SN10 rule) + histograms divergence-window + INDEPENDENT per-route stats — Task 8
- **ADR-0139** Per-route INDEPENDENT-stats ratification + NEW 6th canonical bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route — Task 7

**Plus ADR-0125 amendment paragraph §(xi)** — ALREADY landed at SPEC commit `49e0361` per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) in-place-update precedent.

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The sixteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — `bandwidthlimit.go` file split = TWO-WAY** (`bandwidthlimit.go` + `bucket.go`; the kbps-per-tick `throttleDuration` helper is the most-self-contained primitive at ~60-90 LoC).
2. **D2 — Fast-passthrough threshold = RESOLVED AT SPEC TIME via one-tick `fill_interval` floor** (no fast-passthrough except `bodySize == 0`).
3. **D3 — Pending-gauge Stop-races-Fire = `Stop() returns true → Dec here; ==false → trust callback` per SPEC §6.9** (Group 6 race-test validates; fallback to `markedActive atomic.Bool` per phase-09 fault if flaky).
4. **D4 — Per-route stat-counter cardinality bound = SILENT-ALLOW** (no cap in MVP; documented at §13.4).
5. **D5 — `enable_mode: DISABLED` listener-vs-per-route parity = UNIT-TEST in Group 7** (TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute).
6. **D6 — `fill_interval` granularity = RESOLVED AT SPEC TIME via kbps-per-tick formula** + `*_enforced` increment-by-`ticks` discipline at timer-fire.
7. **D7 — Per-route `runtime_enabled` interaction = SILENT-IGNORE** (Group 2 test asserts field is parsed but not honored).
8. **D8 — Trailer-emission framework forward-pointer = SILENT-IGNORE** (Group 1 tests assert no trailers regardless of `enable_response_trailers`).
9. **PLAN-emerging — `HTTPFilter` value = `Decoder: f, Encoder: f` SAME *filter** (per ADR-0135; mirrors phase-14 ADR-0129 generalized to symmetric BOTH-direction).
10. **PLAN-emerging — Filter-callback wiring = BOTH SetDecoderCallbacks AND SetEncoderCallbacks; both store on the SAME *filter instance** (`f.dcb` for RequestRouteConfig + ContinueDecoding; `f.ecb` for ContinueEncoding).
11. **PLAN-emerging — Fixture topology = TWO LISTENERS `l_test_a` + `l_test_b` with cluster `c_backend_b`** (echo-backend reuses existing `test/helpers/echobackend/` from phase-14).
12. **PLAN-emerging — BackendKind enum value = `HTTPBandwidthLimit BackendKind = 14`** (continues phase-14 `HTTPCompressor = 13`).
13. **PLAN-emerging — ADR anchor schedule = ADR-0135 + ADR-0136 at Task 2; ADR-0137 at Task 3; ADR-0138 at Task 8; ADR-0139 at Task 7** (per ADR-0044 ADR-on-impl + phase-13 buffer authored-at-impl-time precedent).
14. **PLAN-emerging — Acceptance discipline = per-task verbatim verification commands + ADR-anchor verification** (`grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returns 1 match).
15. **PLAN-emerging — `*_enforced` counter increment-by-`ticks` cumulative-match** (per SPEC §6.7 + §11.P3 + amendment 7; NOT once-per-stream).
16. **PLAN-emerging — BEHAVIOR_CONTRACT §13.1 insertion point = AFTER `### envoy.filters.http.compressor` at line 1302** (landing-chronological per phase-13/14 precedent; SPEC §13.1's alphabetical-canonical claim is inaccurate against observed file state).

## Task 1 entry

(commit SHA + step-by-step output appended here at Task 1 commit time)
````

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 15: PROGRESS.md preamble + execution-precondition check

Lands the lifecycle artefact PROGRESS.md with the verbatim 17-precondition
verification output + the 5-ADR Lands-in-task anchor schedule (ADR-0135 +
ADR-0136 at Task 2; ADR-0137 at Task 3; ADR-0138 at Task 8; ADR-0139 at
Task 7; per ADR-0044 ADR-on-impl + phase-13 buffer authored-at-impl-time
precedent) + the 16 planner-time deferred-decision resolutions (8 SPEC §12
+ 8 PLAN-emerging) verbatim from PLAN.md. All 17 preconditions green; no
production-code touched at this task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `internal/filter/http/bandwidthlimit/` package — `doc.go` + `bandwidthlimit.go` skeleton (TypeURL, types, compiledConfig + factoryState + filter + filterStats + parsePerRoute + buildCompiledConfig + buildCompiledConfigPerRoute + resolvePerRouteConfig + newFilterStats / newFilterStatsIfAbsent + New factory) + `bandwidthlimit_test.go` Group 1 + Group 2 tests [ADR-0135, ADR-0136]

**Files:**
- Create: `internal/filter/http/bandwidthlimit/doc.go`
- Create: `internal/filter/http/bandwidthlimit/bandwidthlimit.go`
- Create: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (NEW ADR-0135 + NEW ADR-0136 inserted after ADR-0134)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 2 entry)

This task lands the package skeleton + `New` factory PGV-mirror + `parsePerRoute` + `buildCompiledConfig` + `buildCompiledConfigPerRoute` + `resolvePerRouteConfig` + 14-stat `filterStats` struct + `newFilterStats` + `newFilterStatsIfAbsent` registration helpers + `factoryState` + `filter` per SPEC §6.1-§6.5 + §6.11 + ADR-0135 §Decision (i)-(vii) + ADR-0136 §Decision (i)-(vi). Per TDD discipline: write Group 1 + Group 2 tests FIRST; verify they FAIL (no package exists); then land doc.go + bandwidthlimit.go skeleton. ADR-0135 + ADR-0136 AUTHORED at this commit per ADR-0044 ADR-on-impl (phase-13 buffer convention; NOT pre-landed at SPEC commit).

**Precondition:** Task 1 commit on HEAD; pristine tree; `internal/filter/http/bandwidthlimit/` does not exist.
**Artifacts:** doc.go, bandwidthlimit.go (skeleton with `compiledConfig`, `factoryState`, `filter`, `filterStats`, `TypeURL`, `New`, `buildCompiledConfig`, `buildCompiledConfigPerRoute`, `parsePerRoute`, `(s *factoryState) resolvePerRouteConfig`, `newFilterStats`, `newFilterStatsIfAbsent`, stubs for `DecodeHeaders` / `DecodeData` / `DecodeTrailers` / `EncodeHeaders` / `EncodeData` / `EncodeTrailers` / `SetDecoderCallbacks` / `SetEncoderCallbacks` / `OnDestroy`); bandwidthlimit_test.go (Group 1 + 2); ADR-0135 + ADR-0136 authored; Task 2 PROGRESS entry.
**Acceptance:** `go build ./internal/filter/http/bandwidthlimit/...` clean; `go vet ./internal/filter/http/bandwidthlimit/...` clean; `golangci-lint run ./internal/filter/http/bandwidthlimit/...` clean; `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Group 1 + 2 tests PASS; `grep -nE '^## ADR-0135' docs/envoy-go/DECISIONS.md` returns 1 match; `grep -nE '^## ADR-0136' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0135 + ADR-0136 `Lands-in-task: Task 2` fields verified intact via `grep -nE 'Lands-in-task' docs/envoy-go/DECISIONS.md | grep -E '0135|0136'` returning 2 matches each naming Task 2; Task 2 entry appended to PROGRESS.md.

- [ ] **Step 1: Write the failing tests (Group 1 + Group 2)**

Create `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` with the test groups per SPEC §14.1. Group 1 covers New factory + buildCompiledConfig (~14 tests); Group 2 covers buildCompiledConfigPerRoute + parsePerRoute (~6 tests). Skeleton:

```go
package bandwidthlimit

import (
	"testing"
	"time"

	bandwidthlimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// --- Group 1: New factory + buildCompiledConfig ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on nil tc")
	}
}

func TestBuildCompiledConfig_StatPrefixEmpty_Rejected(t *testing.T) {
	_, err := buildCompiledConfig(&bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "",
		LimitKbps:  wrapperspb.UInt64(10),
	}, envoyhttp.FactoryCtx{}, false /*isPerRoute*/)
	if err == nil {
		t.Fatal("expected error on empty stat_prefix")
	}
}

// (additional Group 1 tests per SPEC §14.1 Group 1: ~14 tests covering
// PGV-mirror discipline + default values + silent-ignore semantics — see
// PLAN.md §"bandwidthlimit_test.go" Group 1 enumeration for the full list.)

// --- Group 2: buildCompiledConfigPerRoute + parsePerRoute ---

func TestBuildCompiledConfigPerRoute_LimitKbpsUnset_Rejected(t *testing.T) {
	_, err := buildCompiledConfig(&bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "override",
		// LimitKbps unset
	}, envoyhttp.FactoryCtx{}, true /*isPerRoute*/)
	if err == nil {
		t.Fatal("expected error: per-route requires limit_kbps")
	}
	expected := "bandwidth_limit: per-route entry requires limit_kbps to be set"
	if err.Error() != expected {
		t.Errorf("wrong error wording: got %q; want %q", err.Error(), expected)
	}
}

// (additional Group 2 tests per SPEC §14.1 Group 2: ~6 tests covering
// per-route REQUIRED-limit_kbps + parsePerRoute unmarshal discipline +
// `TestParsePerRoute_RuntimeEnabledOverride_SilentIgnored` per planner-time
// decision 7 + §11.P6.)
```

Implementer expands the test skeleton to the full ~20-test surface enumerated in PLAN's `bandwidthlimit_test.go` row of the file-structure table (Groups 1-2); test helpers `mustAny(t, msg proto.Message) *anypb.Any` + `freshFactoryCtx() envoyhttp.FactoryCtx` mirror the phase-11/13/14 precedents. Reference: phase-11 `local_ratelimit_test.go` Group 1 (~20 test cases for buildRuntimeConfig PGV-mirror) + phase-13 buffer's `buffer_test.go` Group 1 for the silent-ignored-field discipline.

- [ ] **Step 2: Run tests; verify Groups 1-2 FAIL (no package exists)**

```bash
go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/
# expect: BUILD FAIL (package does not exist) — every test fails to compile
```

- [ ] **Step 3: Author `doc.go`**

Create `internal/filter/http/bandwidthlimit/doc.go` per the file-structure table row above (~30 LoC). The doc file enumerates the 4 consumed / 3 silent-ignored field surface, the BOTH-direction Path B-async body algorithm with kbps-per-tick throttle math (KiB/s units), the per-route discipline (NEW 6th canonical bare-message-via-TPFC), the 14-stat filterStats namespace, and the ZERO framework deltas claim. Mirror `internal/filter/http/compressor/doc.go` structure verbatim.

- [ ] **Step 4: Author `bandwidthlimit.go` skeleton**

Create `internal/filter/http/bandwidthlimit/bandwidthlimit.go` with the public surface + types + helpers + stub method bodies per SPEC §6.1-§6.5 + §6.11 + ADR-0135 §Decision (i)-(vii) + ADR-0136 §Decision (i)-(vi). The Encode/Decode method bodies are STUBS returning Continue/DataContinue/TrailersContinue; Tasks 4-5 land the real bodies. The factory closure returns `envoyhttp.HTTPFilter{Name: filterName, Decoder: f, Encoder: f, PerRoute: parsePerRoute}` per planner-time decision 9.

Key code shapes verbatim per SPEC §6:

```go
package bandwidthlimit

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	bandwidthlimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"

const filterName = "envoy.filters.http.bandwidth_limit"

const defaultFillInterval = 50 * time.Millisecond

// compiledConfig per SPEC §6.2 + §1.1 amendments 3 + 4 + 5 + 6.
type compiledConfig struct {
	statPrefix   string
	enableMode   bandwidthlimitv3.BandwidthLimit_EnableMode
	limitKbps    uint64
	fillInterval time.Duration
	stats        *filterStats
}

// filterStats per SPEC §6.2 + §1.1 amendment 7 + ADR-0138.
// 14 active fields: 8 counters + 6 gauges.
// 2 histograms (request_transfer_duration, response_transfer_duration) NOT
// registered in MVP per phase-06.1 baseline + amendment 9 divergence-window.
type filterStats struct {
	// 8 counters
	requestEnabled            *stats.Counter
	requestEnforced           *stats.Counter
	requestIncomingTotalSize  *stats.Counter
	requestAllowedTotalSize   *stats.Counter
	responseEnabled           *stats.Counter
	responseEnforced          *stats.Counter
	responseIncomingTotalSize *stats.Counter
	responseAllowedTotalSize  *stats.Counter
	// 6 gauges
	requestPending       *stats.Gauge
	requestIncomingSize  *stats.Gauge
	requestAllowedSize   *stats.Gauge
	responsePending      *stats.Gauge
	responseIncomingSize *stats.Gauge
	responseAllowedSize  *stats.Gauge
}

// factoryState per SPEC §6.3 + ADR-0117 + IMPL-1.
type factoryState struct {
	listenerRC *compiledConfig
	perRoute   sync.Map // map[*bandwidthlimitv3.BandwidthLimit]*compiledConfig — per-route lazy-cache keyed by pointer-identity per ADR-0117 IMPL-1
	reg        *stats.Registry
}

// filter per SPEC §6.3.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks
	ecb   envoyhttp.EncoderFilterCallbacks

	requestRC     *compiledConfig
	requestActive bool
	requestBody   []byte
	requestTimer  *time.Timer

	responseRC     *compiledConfig
	responseActive bool
	responseBody   []byte
	responseTimer  *time.Timer
}

var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("bandwidth_limit: typed_config required")
	}
	var c bandwidthlimitv3.BandwidthLimit
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("bandwidth_limit: unmarshal: %w", err)
	}
	rc, err := buildCompiledConfig(&c, ctx, false /*isPerRoute*/)
	if err != nil {
		return nil, err
	}
	state := &factoryState{
		listenerRC: rc,
		reg:        ctx.Stats,
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{state: state}
		return envoyhttp.HTTPFilter{
			Name:     filterName,
			Decoder:  f,
			Encoder:  f,
			PerRoute: parsePerRoute,
		}
	}, nil
}

// (buildCompiledConfig, buildCompiledConfigPerRoute, parsePerRoute,
// resolvePerRouteConfig, newFilterStats, newFilterStatsIfAbsent helpers +
// Encode/Decode method stubs land per SPEC §6 verbatim per the file-structure
// table.)
```

NOTE: the skeleton above is a SHELL; the implementer fleshes it out per SPEC §6 with the full `buildCompiledConfig` + `buildCompiledConfigPerRoute` PGV-mirror checks (4 fields verbatim per §6.5 + per-route `limit_kbps` REQUIRED extra-check) + `parsePerRoute` proto-unmarshal + `resolvePerRouteConfig` lazy-cache via `sync.Map` (mirror phase-11 `local_ratelimit.go:305-337` verbatim) + `newFilterStats` + `newFilterStatsIfAbsent` (post-Freeze idempotent per ADR-0117) + the stub Encode/Decode methods returning `Continue`/`DataContinue`/`TrailersContinue`. Reference: phase-11 `local_ratelimit.go` Task 2 skeleton verbatim shape.

- [ ] **Step 5: Run tests; verify Groups 1-2 PASS**

```bash
go vet ./internal/filter/http/bandwidthlimit/...
golangci-lint run ./internal/filter/http/bandwidthlimit/...
go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/
# expect: all Group 1 + 2 tests PASS
```

- [ ] **Step 6: Author ADR-0135 + ADR-0136 in DECISIONS.md**

Insert two new ADR sections in `docs/envoy-go/DECISIONS.md` immediately after the current ADR-0134 (or after the current ADR tail if higher numbered). Each ADR uses the standard envoy-go ADR template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences).

**ADR-0135** (package shape per SPEC §6.1 + SPEC §1 item 1 + planner-time decision 9): document the `internal/filter/http/bandwidthlimit/` directory + Go-package identifier (single token; matches phase-11 `localratelimit/` precedent per ADR-0114 §2.1); files enumerated (`doc.go`, `bandwidthlimit.go`, `bucket.go`, `bandwidthlimit_test.go`, `fuzz_test.go`); public surface (`TypeURL` const + `New` HTTPFilterFactory); ENCODER+DECODER `HTTPFilter` value with SAME *filter instance (mirrors phase-14 ADR-0129 generalized to symmetric BOTH-direction per SPEC §1 item 4); 14-active-stat `filterStats` struct (8 counters + 6 gauges per amendment 7; 2 histograms deferred per amendment 9); boot-registration ordering alphabetical-after-router; ZERO framework deltas (FIRST §9 row since phase-12 csrf to introduce no new primitives; FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 framework-delta sets simultaneously). `Lands-in-task: Task 2`.

**ADR-0136** (compiledConfig shape per SPEC §6.2 + §1.1 amendments 3 + 4 + 5 + ADR-0101 precedent): document the 5-field `compiledConfig` struct + 4-consumed/3-silent-ignored decomposition + PGV-mirror filter-internal validation discipline + envoy-go-own error wording (per phase-13 ADR-0126 clear-text-error-discipline precedent); CODE-LEVEL extra check at per-route position for `limit_kbps` REQUIRED with verbatim error `"bandwidth_limit: per-route entry requires limit_kbps to be set"` mirroring Envoy filter source `"limit must be set for per route filter config"`; closure-capture + parse-at-New + read-only-shared-after-New discipline per ADR-0101. `Lands-in-task: Task 2`.

- [ ] **Step 7: Verify ADR-0135 + ADR-0136 intact at HEAD**

```bash
grep -nE '^## ADR-0135' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE '^## ADR-0136' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task: Task 2' docs/envoy-go/DECISIONS.md | head -5   # expect: ADR-0135 + ADR-0136 lines
```

- [ ] **Step 8: Append Task 2 entry to PROGRESS.md**

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/bandwidthlimit/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 15: bandwidthlimit package skeleton — doc.go + bandwidthlimit.go (TypeURL, types, factory, parsePerRoute, resolvePerRouteConfig, 14-stat filterStats) + Group 1+2 tests [ADR-0135, ADR-0136]

Lands the package skeleton + New factory PGV-mirror per SPEC §6.1 +
ADR-0136 §Decision (i)-(vi); parsePerRoute + resolvePerRouteConfig
mirror phase-11 ADR-0117 IMPL-1 (same proto for listener + per-route;
sync.Map lazy-cache keyed by *BandwidthLimit pointer); CODE-LEVEL extra
check at per-route position for limit_kbps REQUIRED per §11.P1
"bandwidth_limit: per-route entry requires limit_kbps to be set"
mirroring Envoy filter source; 14-active-stat filterStats struct
registered at New factory time per ADR-0138 §Decision (i) (full
registration deferred to Task 8 — this task just declares the struct +
skeleton newFilterStats helper). doc.go + bandwidthlimit.go skeleton +
Group 1+2 unit tests (~20 tests total) PASS.

ADR-0135 + ADR-0136 authored at this commit per ADR-0044 ADR-on-impl
(phase-13 buffer convention; NOT pre-landed at SPEC commit unlike
phase-14 compressor).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `bucket.go` — `throttleDuration` kbps-per-tick helper + Group 3 throttle-math unit tests [ADR-0137]

**Files:**
- Create: `internal/filter/http/bandwidthlimit/bucket.go`
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 3)
- Modify: `docs/envoy-go/DECISIONS.md` (NEW ADR-0137 inserted after ADR-0136)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 3 entry)

This task lands the algorithmic core of phase-15: the kbps-per-tick `throttleDuration` helper per SPEC §6.6 + §1.1 amendment 6 + §11.P15. The helper is a pure function (no state) with the closed-form formula `chunk_size = limit_kbps × 1024 × fill_interval_seconds`; `ticks = ceil(body_size / chunk_size)`; `duration = ticks × fill_interval`. Plus the foot-gun branch per amendment 10 (`limit_kbps == 0` + active enable_mode → arbitrarily-large 24h throttle to match Envoy's runtime-hang). Per TDD discipline: write Group 3 tests FIRST; verify they FAIL (function doesn't exist); then land `bucket.go`. ADR-0137 authored at this commit (the algorithmic-core ADR; first-use convention per ADR-0044).

**Precondition:** Task 2 commit on HEAD; Group 1 + 2 tests passing.
**Artifacts:** bucket.go (~60-90 LoC); bandwidthlimit_test.go Group 3 appended (~6 tests); ADR-0137 authored; Task 3 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-3 PASS; `grep -nE '^## ADR-0137' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0137 `Lands-in-task: Task 3` field verified; SPEC §6.6's empirical-verification table reproduced in bucket.go GoDoc.

- [ ] **Step 1: Append Group 3 tests (failing)**

Add Group 3 to `bandwidthlimit_test.go` with the test cases enumerated in PLAN's file-structure-table Group 3 row. Skeleton:

```go
// --- Group 3: throttleDuration kbps-per-tick arithmetic ---

func TestThrottleDuration_EmptyBody_ReturnsZero(t *testing.T) {
	dur, ticks := throttleDuration(0, 10, 50*time.Millisecond)
	if dur != 0 || ticks != 0 {
		t.Errorf("expected (0, 0); got (%v, %d)", dur, ticks)
	}
}

func TestThrottleDuration_LimitKbpsZero_ReturnsFootGun(t *testing.T) {
	// Per §1.1 amendment 10: limit_kbps=0 → arbitrarily-large throttle (24h).
	dur, ticks := throttleDuration(100, 0, 50*time.Millisecond)
	if dur < 23*time.Hour || dur > 25*time.Hour {
		t.Errorf("expected ~24h foot-gun throttle; got %v", dur)
	}
	if ticks != 1 {
		t.Errorf("expected ticks=1 (foot-gun marker); got %d", ticks)
	}
}

func TestThrottleDuration_KbpsPerTickMatrix(t *testing.T) {
	// Verbatim from SPEC §6.6 empirical-verification table.
	cases := []struct {
		body        int
		kbps        uint64
		fill        time.Duration
		wantTicks   uint64
		wantDur     time.Duration
	}{
		{100, 10, 50 * time.Millisecond, 1, 50 * time.Millisecond},     // one-tick floor (sub-chunk_size)
		{1024, 10, 50 * time.Millisecond, 2, 100 * time.Millisecond},   // 2 ticks @ 512 chunk_size
		{4000, 10, 50 * time.Millisecond, 8, 400 * time.Millisecond},   // 8 ticks
		{4000, 5, 50 * time.Millisecond, 16, 800 * time.Millisecond},   // 16 ticks @ 256 chunk_size
		{4000, 1, 50 * time.Millisecond, 79, 3950 * time.Millisecond},  // 79 ticks @ 51.2 chunk_size
	}
	for _, tc := range cases {
		dur, ticks := throttleDuration(tc.body, tc.kbps, tc.fill)
		if ticks != tc.wantTicks {
			t.Errorf("body=%d kbps=%d fill=%v: wantTicks=%d gotTicks=%d", tc.body, tc.kbps, tc.fill, tc.wantTicks, ticks)
		}
		if dur != tc.wantDur {
			t.Errorf("body=%d kbps=%d fill=%v: wantDur=%v gotDur=%v", tc.body, tc.kbps, tc.fill, tc.wantDur, dur)
		}
	}
}

// (additional Group 3 tests: ~3 more per the file-structure-table enumeration
// covering one-tick-floor parametrized + fill_interval granularity scaling +
// large-body 51200-byte sanity.)
```

- [ ] **Step 2: Run tests; verify Group 3 FAILS (function doesn't exist)**

```bash
go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/ -run 'TestThrottle'
# expect: BUILD FAIL — throttleDuration undefined
```

- [ ] **Step 3: Author `bucket.go`**

Create `internal/filter/http/bandwidthlimit/bucket.go` per SPEC §6.6 verbatim:

```go
package bandwidthlimit

import "time"

// throttleDuration computes the kbps-per-tick throttle duration for emitting
// bodySize bytes at limitKbps KiB/s (kibibytes-per-second per proto comment
// at bandwidth_limit.pb.go:95; NOT kilobits-per-second per SPEC §1.1
// amendment 6) with fillInterval governing chunk-emit cadence.
//
// Formula (per SPEC §6.6 + §11.P15 empirical):
//
//	chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds (bytes/tick)
//	ticks               = ceil(body_size / chunk_size_per_tick)
//	throttle_duration   = ticks × fill_interval
//
// Edge cases:
//
//   - bodySize == 0: returns (0, 0) — no throttle.
//
//   - limitKbps == 0 (listener-level operational foot-gun per §1.1 amendment 10
//     + §11 probeJ): returns (24*time.Hour, 1) — arbitrarily-large throttle to
//     match Envoy's runtime-hang behavior on listener-level missing limit_kbps
//     + active enable_mode. The ticks=1 marker lets the caller still increment
//     *_enforced by 1 (matching Envoy's behavior at the first tick).
//
//   - bodySize <= chunk_size_per_tick: returns (fillInterval, 1) — one-tick
//     floor; approximates Envoy's initial-burst capacity behavior within
//     ±70ms tolerance per §11.P9.
//
// Returns ticks alongside duration so the caller can increment *_enforced by
// ticks at stream-completion (per §11.P3 + §6.7 + §1.1 amendment 7 per-tick
// cumulative-match discipline).
//
// Empirical verification matrix (verbatim from SPEC §6.6):
//
//	| Body | limit_kbps | fill_interval | chunk_size | ticks | Predicted | Observed (§11.P9) |
//	|------|------------|---------------|------------|-------|-----------|---------------------|
//	|  100 |     10     |     50ms      | 512 bytes  |   1   |    50ms   |  5ms (initial-burst)|
//	| 1024 |     10     |     50ms      | 512 bytes  |   2   |   100ms   |    107ms            |
//	| 4000 |     10     |     50ms      | 512 bytes  |   8   |   400ms   |    359-367ms        |
//	| 4000 |      5     |     50ms      | 256 bytes  |  16   |   800ms   |    716-814ms        |
//	| 4000 |      1     |     50ms      | 51.2 bytes |  79   |  3950ms   |    3904ms           |
//
// Differences within ±70ms tolerance per §11.P9 absorb the initial-burst
// capacity approximation. The pure-function shape means no state allocation.
func throttleDuration(bodySize int, limitKbps uint64, fillInterval time.Duration) (duration time.Duration, ticks uint64) {
	if bodySize == 0 {
		return 0, 0
	}
	if limitKbps == 0 {
		// Foot-gun match per §1.1 amendment 10: arbitrarily-large throttle.
		return 24 * time.Hour, 1
	}
	fillSec := fillInterval.Seconds()
	chunkSize := uint64(float64(limitKbps) * 1024 * fillSec)
	if chunkSize == 0 {
		// Defensive (structurally unreachable given fill_interval >= 20ms PGV).
		chunkSize = 1
	}
	ticks = (uint64(bodySize) + chunkSize - 1) / chunkSize // ceil division
	duration = time.Duration(ticks) * fillInterval
	return duration, ticks
}
```

- [ ] **Step 4: Run tests; verify Group 3 PASSES**

```bash
go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/
# expect: Groups 1-3 PASS
```

- [ ] **Step 5: Author ADR-0137**

Insert ADR-0137 in `docs/envoy-go/DECISIONS.md` immediately after ADR-0136 per ADR-0044 ADR-on-impl first-use convention (the `throttleDuration` helper is the algorithmic-core landing).

**ADR-0137** content (per SPEC §1 item 5 + §3 + §6.6 + §11.P8 + §11.P15 + amendment 6):
- §Context: phase-13 ADR-0128 + phase-14 ADR-0131 framework primitives are reusable infrastructure; phase-15 composes against both without amendment + adds no new framework primitives.
- §Decision (i): body algorithm Path B-async (buffer-then-delayed-emit); ZERO framework deltas; `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume reuse from phase-09 fault.
- §Decision (ii): kbps-per-tick throttle math (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`; KiB/s units NOT kbps); the BRAINSTORM steady-rate formula is REFUTED at §11.P15.
- §Decision (iii): `*_enforced` increment-by-`ticks` at timer-fire for cumulative-counter byte-equivalence with reference Envoy's per-tick increment.
- §Decision (iv): one-tick `fill_interval` floor for sub-chunk-size bodies; matches Envoy's initial-burst behavior within ±70ms tolerance per §11.P9.
- §Decision (v): foot-gun branch `limit_kbps == 0` → 24h throttle matches Envoy's runtime-hang behavior per amendment 10 + probeJ.
- §Decision (vi): `cb.OverwriteBody` (phase-14 ADR-0131 primitive) anticipated NOT invoked; the framework's buffered-return path returns bytes unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding`. If impl-time framework-survey reveals it IS required, phase-15 REUSES (not introduces) the primitive; ZERO-framework-deltas claim stays.
- §Decision (vii): wire-shape divergence-window — envoy-go silent-then-blast vs Envoy Path A rate-paced chunks at `fill_interval` cadence; total-throttle-time equivalent ±70ms; chunk-arrival-time divergent. Forward-pointer to future encode-side streaming framework phase.
- §Alternatives considered: Path A streaming (rejected; requires symmetric `EmitChunk` / `ConsumeChunk` framework primitives not in envoy-go's existing framework); steady-rate formula (rejected; REFUTED at §11.P15); fast-passthrough threshold at 1ms (rejected; one-tick `fill_interval` floor is structurally honest).
- §Consequences: phase-15 is the FIRST §9 row since phase-12 csrf to introduce ZERO framework deltas; demonstrates phase-13 + phase-14 framework primitives as reusable infrastructure; future trailer-emission framework phase will land `EncoderFilterCallbacks.EmitTrailers` enabling the 4 deferred bandwidth-rate-limit trailers.
- `Lands-in-task: Task 3`.

- [ ] **Step 6: Verify ADR-0137 intact + Append Task 3 entry to PROGRESS.md + Commit**

```bash
grep -nE '^## ADR-0137' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task: Task 3' docs/envoy-go/DECISIONS.md | grep '0137'   # expect: 1 match

git add internal/filter/http/bandwidthlimit/bucket.go internal/filter/http/bandwidthlimit/bandwidthlimit_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 15: bucket.go — kbps-per-tick throttleDuration helper + Group 3 tests [ADR-0137]

Lands the algorithmic core per SPEC §6.6 + §1.1 amendment 6 + §11.P15:
chunk_size = limit_kbps × 1024 × fill_interval_seconds (KiB/s units NOT
kbps); ticks = ceil(body_size / chunk_size); throttle = ticks ×
fill_interval. Plus foot-gun branch limit_kbps==0 → 24h throttle per
amendment 10 + probeJ; one-tick fill_interval floor for sub-chunk-size
bodies per §11.P9. Pure function, no state. SPEC §6.6's
empirical-verification matrix reproduced verbatim in bucket.go GoDoc.

ADR-0137 authored per ADR-0044 ADR-on-impl + first-use convention. Group 3
unit tests (~6 tests) verify the formula against the SPEC §6.6 matrix
exactly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `DecodeHeaders` + `DecodeData` decode-side throttle bodies — per-route resolution + Active-flag cascade + kbps-per-tick timer arming + counter increments + `ContinueDecoding` resume + Group 4 tests

**Files:**
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit.go` (replace DecodeHeaders + DecodeData stubs with real bodies; also EncodeHeaders/Trailers stubs stay no-op for now)
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 4)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 4 entry)

This task lands the decode-side throttle body per SPEC §6.7. The flow: `DecodeHeaders` resolves per-route via `f.dcb.RequestRouteConfig()` → `state.resolvePerRouteConfig(msg)` → caches `*compiledConfig` on `f.requestRC` + cascades to `f.responseRC` (per-stream symmetric semantic per SPEC §6.7) + sets `f.requestActive` per `enable_mode` + `f.responseActive` symmetric. `DecodeData` on `!f.requestActive` returns `DataContinue`; on active + `!endStream` buffers + returns `DataStopIterationAndBuffer`; on `endStream=true` increments `*_enabled` + `*_incoming_total_size` + `*_incoming_size` + computes `throttle, ticks := throttleDuration(...)` + on `throttle == 0` fast-paths through `DataContinue` + on `throttle > 0` increments `*_pending` + arms `f.requestTimer = time.AfterFunc(throttle, ...)` + returns `DataStopIterationAndBuffer`. Timer-fire callback: increments `*_enforced += ticks` (per-tick cumulative match per §11.P3 + planner-time decision 15) + `*_allowed_total_size += bodyLen` + `*_allowed_size.Set(bodyLen)` + `*_pending.Dec()` + invokes `f.dcb.ContinueDecoding()`.

**Precondition:** Task 3 commit on HEAD; Groups 1-3 passing.
**Artifacts:** bandwidthlimit.go with real DecodeHeaders + DecodeData bodies; Group 4 tests appended (~12 tests); Task 4 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-4 PASS; per SPEC §6.7 verbatim code shape.

- [ ] **Step 1: Append Group 4 tests (failing)**

Add Group 4 to `bandwidthlimit_test.go` covering the 12 cases enumerated in PLAN's file-structure-table Group 4 row. Key tests: `TestDecodeHeaders_EnableModeRequest_RequestActiveTrue`, `TestDecodeHeaders_EnableModeResponse_ResponseActiveTrue`, `TestDecodeHeaders_EnableModeBoth_BothActive`, `TestDecodeHeaders_EnableModeDisabled_BothFalse`, `TestDecodeHeaders_PerRouteResolution_CachesRC`, `TestDecodeData_PassthroughWhenInactive_DataContinue`, `TestDecodeData_BufferedAccumulation_PreEndStream`, `TestDecodeData_EndStream_ZeroBody_FastPath`, `TestDecodeData_EndStream_SmallBody_OneTickFloor`, `TestDecodeData_EndStream_LargeBody_MultiTick`, `TestDecodeData_TimerFire_IncrementEnforcedByTicks`, `TestDecodeData_TimerFire_ContinueDecodingInvoked`. Use a test-double `DecoderFilterCallbacks` that records `ContinueDecoding` invocations + a settable `RequestRouteConfig()` return value.

- [ ] **Step 2: Run tests; verify Group 4 FAILS (stubs still in place)**

- [ ] **Step 3: Implement DecodeHeaders + DecodeData bodies per SPEC §6.7 verbatim**

The DecodeHeaders + DecodeData code shapes are at SPEC §6.7 lines 768-824; implement verbatim with the timer-fire callback closure capturing `f`, `ticks`, `bodyLen`. The callback:

```go
f.requestTimer = time.AfterFunc(throttle, func() {
    if f.requestRC.stats != nil {
        f.requestRC.stats.requestEnforced.Add(ticks)              // per-tick cumulative match per §11.P3
        f.requestRC.stats.requestAllowedTotalSize.Add(bodyLen)
        f.requestRC.stats.requestAllowedSize.Set(bodyLen)
        f.requestRC.stats.requestPending.Dec()
    }
    f.dcb.ContinueDecoding()
})
```

- [ ] **Step 4: Run tests; verify Groups 1-4 PASS**

- [ ] **Step 5: Append Task 4 entry to PROGRESS.md + Commit**

```bash
git add internal/filter/http/bandwidthlimit/bandwidthlimit.go internal/filter/http/bandwidthlimit/bandwidthlimit_test.go docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 15: DecodeHeaders + DecodeData decode-side throttle — per-route resolution + Active-cascade + timer arming + ContinueDecoding resume

Lands the decode-side body per SPEC §6.7 verbatim: per-route resolution
via dcb.RequestRouteConfig → resolvePerRouteConfig → cache on f.requestRC
+ cascade RC to f.responseRC (per-stream symmetric semantic); enable_mode
gating for f.requestActive + f.responseActive; buffered accumulation via
DataStopIterationAndBuffer; on endStream=true compute kbps-per-tick
throttle via throttleDuration (bucket.go); arm time.AfterFunc; on
timer-fire increment *_enforced += ticks (cumulative match per §11.P3 +
amendment 7), *_allowed_total_size + *_allowed_size, decrement *_pending,
invoke dcb.ContinueDecoding. Group 4 tests (~12 cases) PASS.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `EncodeHeaders` + `EncodeData` encode-side throttle bodies — symmetric to decode-side + `ContinueEncoding` resume + Group 5 tests

**Files:**
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit.go` (replace EncodeHeaders + EncodeData stubs with real bodies)
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 5)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 5 entry)

This task lands the encode-side throttle body symmetric to Task 4's decode-side per SPEC §6.8. The flow is structurally identical with `f.responseActive` / `f.responseBody` / `f.responseTimer` / response-* stats fields / `f.ecb.ContinueEncoding()` substituted. `EncodeHeaders` is no-op (`HeaderContinue`) since `responseRC` + `responseActive` were already cached at `DecodeHeaders` in Task 4 (per-stream symmetric semantic). The framework's buffered-return path emits the body bytes unchanged downstream after `ContinueEncoding` resumes the chain — `cb.OverwriteBody` is NOT invoked (the buffered bytes ARE the original bytes; the same-bytes case per ADR-0137 §Decision (vi)).

**Precondition:** Task 4 commit on HEAD; Groups 1-4 passing.
**Artifacts:** bandwidthlimit.go with real EncodeHeaders + EncodeData bodies; Group 5 tests appended (~8 tests); Task 5 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-5 PASS; per SPEC §6.8 verbatim code shape; framework-survey verification confirms `cb.OverwriteBody` NOT required (the buffered-return path emits bytes unchanged).

- [ ] **Step 1: Append Group 5 tests (failing)** — 8 cases per PLAN's file-structure-table Group 5 row (mirror of Group 4 with decode→encode + Continue→ContinueEncoding). Use a test-double `EncoderFilterCallbacks` that records `ContinueEncoding` invocations.

- [ ] **Step 2: Run tests; verify Group 5 FAILS (stubs still in place)**

- [ ] **Step 3: Implement EncodeHeaders + EncodeData bodies per SPEC §6.8 verbatim** — EncodeHeaders is a 1-line no-op returning `envoyhttp.HeaderContinue`. EncodeData mirrors DecodeData's structure with response-side substitutions per SPEC §6.8 lines 834-870. Timer-fire callback closes over `f`, `ticks`, `bodyLen`:

```go
f.responseTimer = time.AfterFunc(throttle, func() {
    if f.responseRC.stats != nil {
        f.responseRC.stats.responseEnforced.Add(ticks)
        f.responseRC.stats.responseAllowedTotalSize.Add(bodyLen)
        f.responseRC.stats.responseAllowedSize.Set(bodyLen)
        f.responseRC.stats.responsePending.Dec()
    }
    f.ecb.ContinueEncoding()
})
```

- [ ] **Step 4: Framework-survey verification — `cb.OverwriteBody` NOT invoked.** Trace `internal/filter/http/chain.go` honor-`DataStopIterationAndBuffer` semantics to confirm the buffered-return path emits the accumulated bytes through the HCM post-chain consumer WITHOUT requiring `cb.OverwriteBody`:

```bash
grep -nE 'DataStopIterationAndBuffer|encodeBodyOverride|OverwriteBody' internal/filter/http/chain.go internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go | head -20
```

If the trace reveals `OverwriteBody` IS required for the same-bytes case, document the impl-time finding in ADR-0137 §Decision (vi) inline-correction. ZERO-framework-deltas claim stays valid in either case (phase-15 REUSES the existing primitive).

- [ ] **Step 5: Run tests; verify Groups 1-5 PASS**

- [ ] **Step 6: Append Task 5 entry to PROGRESS.md + Commit** with message "phase 15: EncodeHeaders + EncodeData encode-side throttle — symmetric to decode-side + ContinueEncoding resume".

---

## Task 6: `OnDestroy` + Set callbacks + timer cleanup with Stop-races-Fire pending-gauge discipline + Group 6 tests including concurrent race test

**Files:**
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit.go` (replace OnDestroy stub with real body per SPEC §6.9; finalize SetDecoderCallbacks + SetEncoderCallbacks if not yet wired)
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 6)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 6 entry)

This task lands `OnDestroy` per SPEC §6.9 + §4 Stop-races-Fire discipline + planner-time decision 3. The shape per SPEC §6.9 verbatim:

```go
func (f *filter) OnDestroy() {
    if f.requestTimer != nil {
        if f.requestTimer.Stop() && f.requestRC != nil && f.requestRC.stats != nil {
            f.requestRC.stats.requestPending.Dec()
        }
    }
    if f.responseTimer != nil {
        if f.responseTimer.Stop() && f.responseRC != nil && f.responseRC.stats != nil {
            f.responseRC.stats.responsePending.Dec()
        }
    }
}
```

The `Stop() returns true` semantic means the timer was active and the callback was prevented from firing — so the OnDestroy path's `pending.Dec()` balances the `pending.Inc()` that armed the timer. If `Stop() returns false` (callback already ran or about to run), the timer-fire callback's own `pending.Dec()` handles the balance; OnDestroy does not Dec to avoid double-decrement. Per SPEC §4: this is race-clean by the timer's Stop() contract (Stop() and the callback are mutually exclusive). The Group 6 race test drives concurrent OnDestroy + timer-fire under `go test -race` to validate no panic, no negative gauge, no double-decrement.

**Precondition:** Task 5 commit on HEAD; Groups 1-5 passing.
**Artifacts:** bandwidthlimit.go OnDestroy + Set callbacks bodies; Group 6 tests appended (~5 tests); Task 6 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-6 PASS; race-test stays clean across N=100 iterations.

- [ ] **Step 1: Append Group 6 tests** — 5 cases per PLAN's file-structure-table Group 6 row: `TestOnDestroy_NoTimer_NoOp`, `TestOnDestroy_TimerActive_StopReturnsTrue_DecPending`, `TestOnDestroy_TimerFired_StopReturnsFalse_TrustCallback`, `TestOnDestroy_RaceConcurrent_NoDoubleDecrement`, `TestOnDestroy_BothDirectionsActive_BothCleanedUp`. The race-test pattern mirrors phase-09 fault's race-test discipline; arms timers with short throttle then concurrently invokes OnDestroy across N=100 iterations under `go test -race`; asserts the final pending gauge is 0 across all iterations.

- [ ] **Step 2: Run tests; verify Group 6 FAILS (stub OnDestroy still in place)**

- [ ] **Step 3: Implement OnDestroy + Set callbacks bodies per SPEC §6.9 verbatim**

- [ ] **Step 4: Run tests with race detector; verify Groups 1-6 PASS clean.** If `TestOnDestroy_RaceConcurrent_NoDoubleDecrement` flakes, fall back to the `markedActive atomic.Bool` pattern per phase-09 fault precedent at `fault.go:441-465` per planner-time decision 3: add `markedActive atomic.Bool` field on `*filter`; OnDestroy + timer-callback both invoke `decrementActive()` which uses `CompareAndSwap(true, false)` for race-clean exactly-once Dec. The simpler `Stop()` bool discriminator is PREFERRED per SPEC §6.9 + §4; markedActive is the fallback.

- [ ] **Step 5: Append Task 6 entry to PROGRESS.md + Commit** with message "phase 15: OnDestroy + Set callbacks + Stop-races-Fire pending-gauge discipline".

---

## Task 7: Per-route INDEPENDENT-stats wiring — `newFilterStatsIfAbsent` finalization + `resolvePerRouteConfig` lazy-cache + Group 7 tests [ADR-0139]

**Files:**
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit.go` (finalize newFilterStatsIfAbsent + resolvePerRouteConfig wiring; ensure buildCompiledConfigPerRoute uses newFilterStatsIfAbsent NOT newFilterStats)
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 7)
- Modify: `docs/envoy-go/DECISIONS.md` (NEW ADR-0139)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 7 entry)

This task finalizes per-route INDEPENDENT-stats wiring per SPEC §5 + §6.11 + §11.P4 + §11.P14 + ADR-0139. Tasks 2-6 laid the foundation; Task 7 ensures the wiring is complete: per-route entries use `newFilterStatsIfAbsent` (NOT `newFilterStats`) for post-Freeze idempotent registration per ADR-0117; per-route stat-counter increments do NOT touch listener-level counters (INDEPENDENT axis); per-route `enable_mode: DISABLED` is the disable mechanism per §1.1 amendment 1 + §11.P12. ADR-0139 authored at this commit per ADR-0044 ADR-on-impl.

**Precondition:** Task 6 commit on HEAD; Groups 1-6 passing.
**Artifacts:** bandwidthlimit.go finalized per-route wiring; Group 7 tests appended (~5 tests); ADR-0139 authored; Task 7 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-7 PASS; `grep -nE '^## ADR-0139' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0139 `Lands-in-task: Task 7` field verified.

- [ ] **Step 1: Append Group 7 tests** — 5 cases per PLAN's file-structure-table Group 7 row: `TestPerRoute_IndependentStats_Allocated`, `TestPerRoute_IndependentStats_ListenerUnaffected`, `TestPerRoute_DisableViaEnableModeDISABLED_NoCounterIncrements`, `TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute` (per planner-time decision 5 + §12 deferred #5), `TestPerRoute_LazyCache_SyncMapKey`.

- [ ] **Step 2: Run tests; verify Group 7 FAILS (per-route wiring incomplete)**

- [ ] **Step 3: Finalize per-route wiring.** Verify `buildCompiledConfigPerRoute` uses `newFilterStatsIfAbsent(ctx.Stats, statPrefix)` for per-route stat-allocation (NOT `newFilterStats`); verify `resolvePerRouteConfig` lazy-cache via `sync.Map.LoadOrStore` matches phase-11 `local_ratelimit.go:305-337` shape exactly.

- [ ] **Step 4: Run tests; verify Groups 1-7 PASS**

- [ ] **Step 5: Author ADR-0139** per the per-ADR Lands-in-task = Task 7 anchor. Content covers: §Context (BRAINSTORM-hypothesized 5th canonical; §11.P1 REFUTED; phase-15 inherits 4th canonical + introduces NEW 6th canonical); §Decision (i) per-route stats INDEPENDENT mirrors phase-11 ADR-0117; §Decision (ii) per-route allocation via `buildCompiledConfigPerRoute` + `newFilterStatsIfAbsent`; §Decision (iii) sync.Map lazy-cache keyed by `*BandwidthLimit` pointer-identity per ADR-0117 IMPL-1; §Decision (iv) 6th canonical pattern documented at ADR-0125 §(xi); §Decision (v) `enable_mode: DISABLED` is the disable mechanism; §Alternatives (SHARED-stats rejected per §11.P4); §Consequences (SECOND row using INDEPENDENT-stats; future filters can pick 4th or 6th canonical based on code-level extra check requirement); `Lands-in-task: Task 7`.

- [ ] **Step 6: Verify ADR-0139 + Append Task 7 entry to PROGRESS.md + Commit** with message "phase 15: per-route INDEPENDENT-stats wiring + newFilterStatsIfAbsent + resolvePerRouteConfig [ADR-0139]".

---

## Task 8: 14-stat `filterStats` finalization — `newFilterStats` registration helper + namespace `<stat_prefix>.http_bandwidth_limit.<counter>` + Group 8 stats integration tests [ADR-0138]

**Files:**
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit.go` (finalize newFilterStats / newFilterStatsIfAbsent registration helpers + 14-stat counter+gauge registration under namespace `<stat_prefix>.http_bandwidth_limit.<counter>` per ADR-0138)
- Modify: `internal/filter/http/bandwidthlimit/bandwidthlimit_test.go` (append Group 8)
- Modify: `docs/envoy-go/DECISIONS.md` (NEW ADR-0138)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 8 entry)

This task finalizes the 14-active-stat filterStats registration per SPEC §6.2 + §1.1 amendment 7 + §11.P3 + §11.P10 + §11.P11 + ADR-0138. The filterStats struct declaration landed at Task 2; this task finalizes `newFilterStats(reg, statPrefix)` to register all 14 stats under the namespace `<statPrefix>.http_bandwidth_limit.<counter>` (mirrors phase-11 `<statPrefix>.http_local_rate_limit.<counter>` shape per ADR-0118 + §11.P11 underscore-infix). The Prometheus rendering via `internal/stats/name.go` default-branch flatten produces `envoy_<statPrefix>_http_bandwidth_limit_<counter>{}` (NO labels, NO tag-extractor, NO new SN10 rule per amendment 8 + §11.P10).

**Precondition:** Task 7 commit on HEAD; Groups 1-7 passing.
**Artifacts:** bandwidthlimit.go finalized newFilterStats + newFilterStatsIfAbsent helpers; Group 8 tests appended (~4 tests); ADR-0138 authored; Task 8 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` shows Groups 1-8 PASS; `grep -nE '^## ADR-0138' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0138 `Lands-in-task: Task 8` verified.

- [ ] **Step 1: Append Group 8 tests** — 4 cases per PLAN's file-structure-table Group 8 row: `TestStatsNamespace_AllFourteenActiveStatsRegistered`, `TestStatsNamespace_UnderscoreInfix_NotHCMRooted`, `TestStatsNamespace_PromInlineFlatten_NoSN10`, `TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent`.

- [ ] **Step 2: Finalize newFilterStats helper** — register 14 stats under `<statPrefix>.http_bandwidth_limit.<counter>` namespace; 8 counters via `reg.NewCounter` + 6 gauges via `reg.NewGauge`; nil-registry tolerance per ADR-0085. The `newFilterStatsIfAbsent` variant uses `reg.NewCounterIfAbsent` + `reg.NewGaugeIfAbsent` for post-Freeze idempotency per ADR-0117. Verify the Prometheus rendering shape via Group 8 tests is `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` exactly (no label, no tag-extractor).

- [ ] **Step 3: Run tests; verify Groups 1-8 PASS**

- [ ] **Step 4: Author ADR-0138** per the per-ADR Lands-in-task = Task 8 anchor. Content covers: §Context (BRAINSTORM hypothesized 6 stats; §11.P3 REFUTED — 16 stats per stat_prefix); §Decision (i) 14 active stats (8c + 6g) per stat_prefix; §Decision (ii) namespace `<stat_prefix>.http_bandwidth_limit.<counter>` underscore infix NOT HCM-rooted per §11.P11; §Decision (iii) Prometheus rendering via existing default-branch flatten (NO labels, NO tag-extractor, NO new SN10 rule per §11.P10); §Decision (iv) per-route stats INDEPENDENT (cross-reference ADR-0139); §Decision (v) `*_enforced` increment-by-`ticks` cumulative-match per §11.P3; §Decision (vi) 2 histograms DEFERRED per phase-06.1 baseline + amendment 9 twin-series-filter; §Decision (vii) `*_incoming_size` / `*_allowed_size` gauges set at endStream-arrival / timer-fire then reset at OnDestroy; §Alternatives (HCM-rooted namespace rejected; new SN10 rule rejected; histogram emission in MVP rejected); §Consequences (phase-15 adds 14 names to BEHAVIOR_CONTRACT §13.2 stat-table 46→60; 2 histograms at §13.4 + §242); `Lands-in-task: Task 8`.

- [ ] **Step 5: Verify ADR-0138 + Append Task 8 entry to PROGRESS.md + Commit** with message "phase 15: 14-stat filterStats finalization — newFilterStats + namespace [ADR-0138]".

---


## Task 9: `FuzzBandwidthLimitConfigParse` fuzzer — 19th fuzzer in repo

**Files:**
- Create: `internal/filter/http/bandwidthlimit/fuzz_test.go`
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 9 entry)

Lands the 19th fuzzer per ADR-0018's "every parser/codec/filter ships a fuzzer" discipline. `FuzzBandwidthLimitConfigParse` fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. The fuzzer's interesting axes: random bytes vs. partial proto-shaped bytes vs. valid proto with random field values (notably `limit_kbps` and `fill_interval` extremes, including the foot-gun `limit_kbps: 0` boundary). 30s budget per ADR-0018.

**Precondition:** Task 8 commit on HEAD; Groups 1-8 passing.
**Artifacts:** fuzz_test.go (~80 LoC); Task 9 PROGRESS entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/bandwidthlimit/` PASS; `go test -fuzz=FuzzBandwidthLimitConfigParse -fuzztime=30s ./internal/filter/http/bandwidthlimit/` PASS (no panics, no `(nil, nil)` results).

- [ ] **Step 1: Author `fuzz_test.go`** with the 6 valid-config + 4 invalid-config seed corpus per PLAN's file-structure-table fuzz_test.go row. The fuzzer body:

```go
package bandwidthlimit

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

func FuzzBandwidthLimitConfigParse(f *testing.F) {
	// Seed corpus: 6 valid + 4 invalid.
	// ... seed adds via f.Add(...) ...

	f.Fuzz(func(t *testing.T, data []byte) {
		tc := &anypb.Any{
			TypeUrl: TypeURL,
			Value:   data,
		}
		factory, err := New(tc, envoyhttp.FactoryCtx{})
		if err != nil {
			// (nil, error) is acceptable.
			if factory != nil {
				t.Errorf("got (factory != nil, error != nil): violates contract")
			}
			return
		}
		// (factory, nil) is acceptable.
		if factory == nil {
			t.Errorf("got (nil, nil): violates contract")
		}
	})
}
```

- [ ] **Step 2: Run fuzzer at 30s budget** — `go test -fuzz=FuzzBandwidthLimitConfigParse -fuzztime=30s ./internal/filter/http/bandwidthlimit/`; expect: no panic, no contract violation.

- [ ] **Step 3: Run all phase-15 tests + the regression-suite of phase-14 fuzzers + the new fuzzer at 30s budget each** — verify Gate D (19 fuzzers green).

- [ ] **Step 4: Append Task 9 entry to PROGRESS.md + Commit** with message "phase 15: FuzzBandwidthLimitConfigParse — 19th fuzzer (30s budget)".

---

## Task 10: `cmd/envoy-go/main.go` register `bandwidthlimit.New` under `bandwidthlimit.TypeURL` + fixture infrastructure (`BackendKind=HTTPBandwidthLimit` enum + runner spawn helper switch-case)

**Files:**
- Modify: `cmd/envoy-go/main.go` (+1 import line + 1 register line)
- Modify: `test/differential/fixture/fixture.go` (+`HTTPBandwidthLimit BackendKind = 14` + doc-comment)
- Modify: `test/differential/runner_test.go` (+blank-import + switch-case dispatching to existing echobackend helper)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 10 entry)

This task wires phase-15's package into the runtime + the differential harness. The boot registration inserts immediately after `httpReg.Register(router.TypeURL, router.New)` and before `httpReg.Register(buffer.TypeURL, buffer.New)` per the alphabetical-after-router convention. The BackendKind enum value `HTTPBandwidthLimit = 14` continues phase-14's `HTTPCompressor = 13`. The runner switch-case dispatches to the existing `startEchoBackend` helper (phase-14 introduced; no new helper at phase 15) since scenarios 2 + 3 + 5 + 6 of fixture 0017 need echo-backend semantics.

**Precondition:** Task 9 commit on HEAD; all unit tests + fuzzer passing.
**Artifacts:** main.go +3 LoC; fixture.go +15 LoC; runner_test.go +15 LoC; Task 10 PROGRESS entry.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 10.

- [ ] **Step 1: Wire boot registration** in `cmd/envoy-go/main.go`:

```go
import (
    // ... existing imports including bandwidthlimit alphabetically among the filter packages ...
    "github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
)

func main() {
    // ... existing setup ...
    httpReg.Register(router.TypeURL, router.New)
    httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)  // NEW
    httpReg.Register(buffer.TypeURL, buffer.New)
    httpReg.Register(compressor.TypeURL, compressor.New)
    // ... cors, csrf, envoygotest, fault, header_mutation, localratelimit ...
    header_mutation.RegisterPerRouteValidator(httpReg)
    httpReg.Freeze()
}
```

- [ ] **Step 2: Add BackendKind enum value** in `test/differential/fixture/fixture.go`:

```go
// HTTPBandwidthLimit drives test/fixtures/0017-http-bandwidth-limit; the
// fixture uses the shared echobackend helper at
// test/helpers/echobackend/cmd/echobackend/main.go (introduced at phase 14
// Task 10) for scenarios 2 + 3 (cluster c_backend_b routes) which need
// upstream-arrival-time assertions independent of response-side throttle.
// Scenarios 1 + 4 + 5 + 6 use direct_response and do not require the
// echo-backend.
HTTPBandwidthLimit BackendKind = 14
```

- [ ] **Step 3: Wire runner dispatch** in `test/differential/runner_test.go`:

```go
import (
    _ "github.com/esalaine/envoy-go/test/fixtures/0017-http-bandwidth-limit/inputs"
)

// ... in the BackendKind switch ...
case fixture.HTTPBandwidthLimit:
    startEchoBackend(ctx, t, listenerB)
```

- [ ] **Step 4: Verify build + lint clean**

```bash
go build ./...
go vet ./...
golangci-lint run
grep -cE 'httpReg.Register' cmd/envoy-go/main.go   # expect: 10
grep -cE 'HTTPBandwidthLimit' test/differential/fixture/fixture.go   # expect: 1+
```

- [ ] **Step 5: Append Task 10 entry to PROGRESS.md + Commit** with message "phase 15: boot registration + BackendKind=HTTPBandwidthLimit enum + runner dispatch".

---

## Task 11: Fixture 0017 — `inputs/driver.go` (6-scenario driver; ±70ms tolerance; counter-delta scrape; histograms allow-list)

**Files:**
- Create: `test/fixtures/0017-http-bandwidth-limit/inputs/driver.go` (~220 LoC)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 11 entry)

Lands the differential-fixture driver per SPEC §7.4. The driver issues 6 scenarios per SPEC §7.1, measures wall-clock per scenario via `time.Now()` at request-issue + response-completion, asserts byte-exact body equivalence (bandwidth_limit does NOT transform bytes), wall-clock within **±70ms tolerance** per scenario, per-counter delta byte-equivalence on the 14 active stats per stat_prefix, AND filters out the 2 unconditional Envoy histograms via the twin-series-filter allow-list discipline before delta comparison.

**Precondition:** Task 10 commit on HEAD; build clean.
**Artifacts:** driver.go (~220 LoC); Task 11 PROGRESS entry.
**Acceptance:** driver compiles; ready for fixture-orchestration in Task 14.

- [ ] **Step 1: Author driver.go** with the 6-scenario shape per SPEC §7.4:

```go
package inputs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Tolerance is the ±70ms wall-clock window per SPEC §11.P9 + §13.5.
const Tolerance = 70 * time.Millisecond

// Throttle expectations per SPEC §7.1 (kbps-per-tick math; chunk_size = limit_kbps × 1024 × fill_interval_seconds).
var scenarioExpectations = []struct {
    name           string
    method, path   string
    expectStatus   int
    expectBodyLen  int  // byte-exact body length
    expectThrottle time.Duration  // total wall-clock ≈ this ±Tolerance
    counterDelta   map[string]uint64
}{
    // Scenario 1: response-only throttle (default route); 10 KiB body → 20 ticks × 50ms = 1000ms.
    {name: "scenario1_response_only", method: "GET", path: "/echo-response", expectStatus: 200, expectBodyLen: 10240, expectThrottle: 1000 * time.Millisecond, counterDelta: map[string]uint64{
        "default.http_bandwidth_limit.response_enabled":            1,
        "default.http_bandwidth_limit.response_enforced":           20,
        "default.http_bandwidth_limit.response_incoming_total_size": 10240,
        "default.http_bandwidth_limit.response_allowed_total_size":  10240,
    }},
    // ... scenarios 2-6 per SPEC §7.1 ...
}

// runScenarioN(ctx, baseURL) functions per scenario.
// measureRequestDuration(ctx, req) helper using time.Now() at issue + completion.
// assertByteExactBody helper.
// scrapeStatsFiltered helper that strips envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_* lines per amendment 9.
// assertCounterDeltas helper computing pre-scrape baseline vs post-scrape values.

// Driver entry point per the fixture-runner contract.
func Run(ctx context.Context, baseURL string) error {
    // ... 6 sequential scenarios; per-scenario assertions ...
}
```

- [ ] **Step 2: Verify driver compiles** — `go build ./test/fixtures/0017-http-bandwidth-limit/...` clean.

- [ ] **Step 3: Append Task 11 entry to PROGRESS.md + Commit** with message "phase 15: fixture 0017 driver — 6-scenario orchestration + ±70ms tolerance + histograms allow-list".

---

## Task 12: Fixture 0017 — `envoy.yaml` + `envoy-go.yaml` bootstraps (two listeners + cluster c_backend_b per SPEC §7.2)

**Files:**
- Create: `test/fixtures/0017-http-bandwidth-limit/envoy.yaml` (~110 LoC)
- Create: `test/fixtures/0017-http-bandwidth-limit/envoy-go.yaml` (~110 LoC)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 12 entry)

Lands the two-listener fixture bootstraps per SPEC §7.2. Listener `l_test_a` is the HCM listener with `bandwidth_limit → router` filter chain and the 6 routes per SPEC §7.2; listener `l_test_b` is the echo-backend cluster `c_backend_b`'s endpoint listener.

**Precondition:** Task 11 commit on HEAD.
**Artifacts:** envoy.yaml + envoy-go.yaml; Task 12 PROGRESS entry.
**Acceptance:** YAML files parse cleanly against reference Envoy v1.37.2; envoy-go-side YAML parses via existing bootstrap loader.

- [ ] **Step 1: Author envoy.yaml** with the listener `l_test_a` listener-level config `stat_prefix: default, enable_mode: REQUEST_AND_RESPONSE, limit_kbps: 10, fill_interval: 50ms` + 6 routes per SPEC §7.2 (`/echo-response` direct_response 10KiB; `/echo-request` cluster c_backend_b per-route `enable_mode: REQUEST`; `/echo-both` cluster c_backend_b; `/echo-tiny` direct_response 100 bytes; `/echo-disabled` direct_response 10KiB per-route `enable_mode: DISABLED, limit_kbps: 10` (the disable mechanism per amendment 1; still needs `limit_kbps` set per amendment 4); `/echo-override` direct_response 10KiB per-route `stat_prefix: override, enable_mode: RESPONSE, limit_kbps: 100, fill_interval: 50ms`).

- [ ] **Step 2: Author envoy-go.yaml** with equivalent two-listener topology.

- [ ] **Step 3: Quick smoke-test** — boot reference Envoy under the YAML; assert clean boot logs:

```bash
docker run --rm -d --name p15-fixture-smoke envoyproxy/envoy:v1.37.2 -c /etc/envoy/envoy.yaml -v /path/to/test/fixtures/0017-http-bandwidth-limit/envoy.yaml:/etc/envoy/envoy.yaml:ro
docker logs p15-fixture-smoke 2>&1 | head -20
# expect: no "Config rejected" lines
docker stop p15-fixture-smoke
```

- [ ] **Step 4: Append Task 12 entry to PROGRESS.md + Commit** with message "phase 15: fixture 0017 envoy.yaml + envoy-go.yaml — two-listener topology per SPEC §7.2".

---

## Task 13: Fixture 0017 — `expectations.yaml` + `README.md` (narrative-only documentation per ADR-0019)

**Files:**
- Create: `test/fixtures/0017-http-bandwidth-limit/expectations.yaml` (~55 LoC)
- Create: `test/fixtures/0017-http-bandwidth-limit/README.md` (~90 LoC)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 13 entry)

Lands the fixture's expectations declaration + the narrative README. The expectations YAML enumerates the 14 active stats per stat_prefix that the driver asserts AND the 2 histograms allow-list per amendment 9. The README documents the 6 scenarios + ±70ms tolerance + KiB/s units + histograms allow-list.

**Precondition:** Task 12 commit on HEAD.
**Artifacts:** expectations.yaml + README.md; Task 13 PROGRESS entry.

- [ ] **Step 1: Author expectations.yaml** with per-scenario counter-delta map + twin-series allow-list for the 2 transfer_duration histograms.

- [ ] **Step 2: Author README.md** with fixture overview + 6 scenarios + ±70ms tolerance discipline (per §11.P9) + KiB/s units note (per amendment 6) + histograms allow-list note (per amendment 9 + §242 twin-series-filter discipline).

- [ ] **Step 3: Append Task 13 entry to PROGRESS.md + Commit** with message "phase 15: fixture 0017 expectations.yaml + README — narrative documentation".

---

## Task 14: Fixture 0017 — driver counter-assertion fleshing + end-to-end differential pass

**Files:**
- Modify: `test/fixtures/0017-http-bandwidth-limit/inputs/driver.go` (flesh out counter-assertion bodies + histograms allow-list filter)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 14 entry)

Lands the end-to-end differential pass. Driver scrapes `/stats/prometheus` from both Envoy and envoy-go, strips the 2 transfer_duration histogram families, computes per-counter deltas vs pre-scrape baseline, asserts byte-equivalence on the 14 active stats. Plus per-scenario wall-clock within ±70ms tolerance.

**Precondition:** Tasks 11 + 12 + 13 committed; fixture infra in place.
**Artifacts:** driver.go counter-assertion fleshed; Task 14 PROGRESS entry.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0017'` returns PASS; both Envoy + envoy-go sides scrape identical counter deltas modulo histograms allow-list.

- [ ] **Step 1: Flesh out counter-assertion bodies** — implement `scrapeStatsFiltered` to strip `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_*` lines (bucket / sum / count families) before delta comparison.

- [ ] **Step 2: Run differential fixture 0017** end-to-end:

```bash
go test -count=1 ./test/differential/ -run 'Test.*0017' -v
# expect: PASS; 6 scenarios green; per-counter deltas byte-equivalent on 14 active stats; histograms allow-listed; wall-clocks within ±70ms tolerance
```

If any scenario flakes on wall-clock tolerance, the impl-task author may widen the tolerance to ±100ms per scenario (the ±70ms is SPEC's position per §11.P9 empirical worst-case; CI scheduling jitter may widen on slower hosts). Document the widening in PROGRESS.md.

- [ ] **Step 3: Run regression-suite** — all 18 fixtures (0000-0017) green:

```bash
go test -count=1 ./test/differential/ -v
# expect: all 18 fixtures PASS
```

- [ ] **Step 4: Append Task 14 entry to PROGRESS.md + Commit** with message "phase 15: fixture 0017 end-to-end differential pass — 6 scenarios green; 18 fixtures green; ±70ms tolerance".

---

## Task 15: BEHAVIOR_CONTRACT.md 5-edit bundle + ROADMAP row 15 in-progress→done + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§13.1 + §13.2 + §13.3 + §13.4 + §13.5 + §242 twin-series extension)
- Modify: `docs/envoy-go/ROADMAP.md` (row 15 `in-progress → done`; summary sharpening)
- Modify: `docs/envoy-go/STATE.md` (lifecycle-state advance to `phase 15 done; awaiting next planning`)
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 15 entry)

Lands the BEHAVIOR_CONTRACT patches per SPEC §13 + the lifecycle artefact advances. The 5 patches per planner-time decision 16: §13.1 new `### envoy.filters.http.bandwidth_limit` subsection inserted AFTER `### envoy.filters.http.compressor` at line 1302 (landing-chronological); §13.2 stat-table 46→60 names extension (14 new active + 2 deferred-histograms allow-list rows); §13.3 NEW equivalence-matrix row for fixture 0017; §13.4 NEW `### Phase 15 forward-pointer notes` subsection (~80 lines); §13.5 `## Timing tolerances` extension with ±70ms entry; `### Twin-series filter discipline` extension with phase-15 histogram allow-list. Plus the 6-gate phase-done verification per BOOTSTRAP_PROMPT.md §7.5.

**Precondition:** Task 14 commit on HEAD; differential fixture 0017 green.
**Artifacts:** BEHAVIOR_CONTRACT.md patched; ROADMAP.md row 15 done; STATE.md advanced; Task 15 PROGRESS entry.
**Acceptance:** 6 gates A-F all green at HEAD per BOOTSTRAP_PROMPT.md §7.5 + SPEC §14.7.

- [ ] **Step 1: Patch BEHAVIOR_CONTRACT.md** per SPEC §13.1-§13.5 verbatim:
  - §13.1 subsection inserted AFTER line 1302 `### envoy.filters.http.compressor` (NOT at HEAD per planner-time decision 16; SPEC §13.1 alphabetical-canonical claim is inaccurate against observed file state).
  - §13.2 stat-table extension: 14 new active rows (8 counters + 6 gauges) + 2 deferred-histogram allow-list rows (per amendment 9) per SPEC §13.2 verbatim.
  - §13.3 equivalence-matrix row per SPEC §13.3 verbatim.
  - §13.4 `### Phase 15 forward-pointer notes` subsection per SPEC §13.4 verbatim.
  - §13.5 `## Timing tolerances` extension with ±70ms entry per SPEC §13.5 verbatim.
  - `### Twin-series filter discipline` extension with phase-15 histogram allow-list entry.

- [ ] **Step 2: Flip ROADMAP row 15** `in-progress → done`; sharpen the summary text with the PLAN-confirmed 16-task + ~413-503 LoC production estimate.

- [ ] **Step 3: Advance STATE.md** lifecycle-state to `phase 15 done; awaiting next planning`; next-skill cleared (or set to next phase if ROADMAP names one); last-commit placeholder `<TBD>` (filled at SHA-fill follow-up post-squash).

- [ ] **Step 4: Run all 6 phase-done gates** per SPEC §14.7:

```bash
# Gate A: build + vet + lint clean
go build ./... && go vet ./... && golangci-lint run
# Gate B: race-test clean across all packages
go test -race -count=1 ./...
# Gate C: h2spec 53/53 PASS at ADR-0051 pin
make h2spec   # or the project's h2spec invocation
# Gate D: 19 fuzzers green at 30s/each
for fuzz in $(grep -rE '^func Fuzz' --include='*.go' . | awk -F: '{print $1, $2}' | ...); do ...; done
# (in practice: per-fuzzer go test -fuzz=... -fuzztime=30s; verify each PASS)
# Gate E: 18 differential fixtures (0000-0017) PASS
go test -count=1 ./test/differential/
# Gate F: BEHAVIOR_CONTRACT.md §13.1-§13.5 populated
grep -n '^### envoy.filters.http.bandwidth_limit' docs/envoy-go/BEHAVIOR_CONTRACT.md   # expect: 1 match
grep -n '^### Phase 15 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md       # expect: 1 match
```

All 6 gates must report green.

- [ ] **Step 5: Append Task 15 entry to PROGRESS.md + Commit** with message "phase 15: BEHAVIOR_CONTRACT 5-edit bundle + ROADMAP row 15 done + STATE.md advance + 6 gates green (phase-done)".

---

## Task 16: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/REVIEW.md`
- Modify: `docs/envoy-go/phases/15-http-filter-bandwidth-limit/PROGRESS.md` (append Task 16 entry)

Lands the end-of-phase REVIEW.md per `superpowers:requesting-code-review` skill. The reviewer subagent (dispatched from this task) validates the impl against the PLAN + SPEC + 6 gates + the acceptance checklist at SPEC §15 (12 claims).

**Precondition:** Task 15 commit on HEAD; 6 gates green.
**Artifacts:** REVIEW.md (~200 LoC); Task 16 PROGRESS entry.
**Acceptance:** REVIEW.md committed; phase-15 lifecycle-state-6 reached.

- [ ] **Step 1: Dispatch the code-reviewer subagent** per `superpowers:requesting-code-review` skill. Reviewer reads SPEC §15 acceptance checklist (12 claims) + PLAN's 16-task structure + the actual landed artefacts (filter package, fixture, ADRs, BEHAVIOR_CONTRACT patches, ROADMAP + STATE advances) + the 6-gate verification log from Task 15 PROGRESS entry.

- [ ] **Step 2: Author REVIEW.md** per the reviewer's findings + the phase-13/14 REVIEW.md template (Status, Summary, Verification against SPEC §15 12-claim acceptance checklist, 6-gate status, follow-up items if any).

- [ ] **Step 3: Append Task 16 entry to PROGRESS.md + Commit** with message "phase 15: REVIEW.md — end-of-phase review per superpowers:requesting-code-review".

---

## End of phase 15 implementation plan

The 16 tasks above sequence the phase-15 implementation per SPEC §6 code shapes + §7 fixture topology + §8 ADR roster + §14 testing strategy + §15 acceptance checklist. The PLAN settles 16 planner-time decisions (8 SPEC §12 + 8 PLAN-emerging) before implementation starts. ADRs ADR-0135..ADR-0139 are AUTHORED AT IMPL-TIME per ADR-0044 ADR-on-impl + phase-13 buffer convention (UNLIKE phase-14 compressor's SPEC-time-pre-landing). ADR-0125 §(xi) amendment paragraph ALREADY LANDED at SPEC commit `49e0361` per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) in-place-update precedent.

Production-LoC estimate: ~413-503 LoC (under ADR-0045's 1500-LoC threshold). Task count: 16 (under ADR-0045's 25-task threshold). The phase-15 release ships as a single ROADMAP row 15 per SPEC §1.4.

Successor session: `superpowers:subagent-driven-development` (per project memory `feedback_execution_style.md` always-subagent-driven preference) to execute the 16 tasks.

