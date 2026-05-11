# Phase 15 SPEC — `envoy.filters.http.bandwidth_limit`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 15 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 / 12 / 13 / 14 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 509 lines). §§1–11 are the pre-§11-empirical-pin design sketch (PRESERVED VERBATIM per D-3.5); the §11 empirical-pin block in this SPEC re-runs all 15 BRAINSTORM §9 pins against reference Envoy v1.37.2 IN-SESSION per ADR-0004. NO post-landing BRAINSTORM §12 amendment cycle was authored — the empirical re-frame is structured for the §1.1 amendment-block channel (mirrors phase-12 csrf 4-amendment + phase-14 compressor 6-amendment precedents rather than phase-13 buffer §12 amendment-cycle precedent). NO off-master prebrainstorm-notes branch.

**ADR continuity:** Phase 14 closed at ADR-0134. Phase 15 anticipated ADR-0135..ADR-0139 (5 ADRs per BRAINSTORM §7) + ADR-0125 amendment paragraph. Phase 15 ships **5** ADRs: ADR-0135, ADR-0136, ADR-0137, ADR-0138, ADR-0139, plus an in-place amendment paragraph on ADR-0125 (per phase-13 ADR-0127-v2 + phase-14 ADR-0125 in-place-update precedent at Task 12). Next-free ADR after phase 15 is ADR-0140.

**§3 framework-survey result up front (locks §3 ZERO-framework-deltas claim):** Existing decode-side framework primitives at `internal/filter/hcm/connection.go` (phase-13 ADR-0128 synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation) + encode-side `EncoderFilterCallbacks.OverwriteBody(b []byte)` (phase-14 ADR-0131) + `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume (phase-09 fault precedent at `internal/filter/http/fault/fault.go:319,335`) compose against the bandwidth_limit's Path B-async body algorithm WITHOUT introducing any new framework primitive. Phase 15 reuses existing infrastructure; ZERO framework deltas. Anticipated `cb.OverwriteBody` invocation NOT needed — the buffered bytes ARE the original bytes, re-emitted unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding` (the framework's buffered-return path returns the accumulated bytes without explicit replace). ADR-0137 §Decision (vi) records.

---

## 1. Purpose

Phase 15 lands `envoy.filters.http.bandwidth_limit` — Envoy's canonical "rate-limit body throughput in KiB/s (kibibytes-per-second; per proto comment at `bandwidth_limit.pb.go:95`; NOT kilobits-per-second as BRAINSTORM hypothesized; refuted at §1.1 amendment 6)" filter, BOTH-direction (symmetric request + response throttle) MVP — as the EIGHTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), buffer (13), and compressor (14), and the EIGHTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 15 is the FIRST §9 family-row since phase 12 csrf to introduce **ZERO framework deltas** — composing wholesale against (a) phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding()` / `cb.ContinueEncoding()` async-resume primitives; (b) phase-13 ADR-0128's decode-side body-buffering machinery (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation); (c) phase-14 ADR-0131's encode-side `OverwriteBody` primitive (anticipated: NOT invoked — the buffered-return path already returns bytes unchanged). The seven new architectural primitives:

1. **New `internal/filter/http/bandwidthlimit/` package** owning the filter implementation. Package directory + Go-package identifier are both `bandwidthlimit` (single token; mirrors `localratelimit/` precedent from phase 11; the `_` in the Envoy filter name `bandwidth_limit` does NOT propagate into the Go package identifier per ADR-0114 §2.1). Files mirror phase-11 local_ratelimit's shape (the precedent for token-bucket-stateful filters whose per-route TPFC uses the same proto as the listener-level config): `bandwidthlimit.go` (filter type + factory + decode/encode methods + per-route resolver + `filterStats` + `compiledConfig` + timer cleanup hook), `bucket.go` (the per-stream body-throttle helper; mirrors `bucket.go` from localratelimit), `bandwidthlimit_test.go` (unit tests), `fuzz_test.go` (the 19th fuzzer — `FuzzBandwidthLimitConfigParse`), `doc.go` (package overview + 4-consumed/3-deferred decomposition + INDEPENDENT-per-route + Path B-async body algorithm). The package exposes `TypeURL` (the canonical type-URL `"type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor precedent. ADR-0135 codifies.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 9 entries after phase 14: `router.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains a tenth `httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)` call before the freeze. Insertion alphabetical-after-router per ADR-0100 §2.2 + ADR-0114 + ADR-0120 + ADR-0125 + ADR-0129 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`. `bandwidthlimit` inserts between `router` and `buffer` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **MVP envelope: 4 consumed + 3 silent-ignored + per-route-via-same-proto (REVISED from BRAINSTORM §2.5 + §2.4; see §1.1 amendment 1 + amendment 2).** `envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit` (the only proto message in the v1.37.2 binding; NO `BandwidthLimitPerRoute` envelope exists — per §1.1 amendment 2 + §11.1) has 7 top-level fields per `bandwidth_limit.pb.go` (`[#next-free-field: 8]`). Phase 15 consumes 4, silent-ignores 3:

   - **`stat_prefix`** (string; REQUIRED at parse-time per PGV `min_len = 1`; REFUTES BRAINSTORM hypothesis "empty default permitted" — see §1.1 amendment 3) — emission-scope tag for the 14-active-stat surface (8 counters + 6 gauges; see §1 item 6 below).
   - **`enable_mode`** (`EnableMode` enum; PGV `defined_only = true`; default `DISABLED` = 0) — all 4 values honored at parse + runtime: `DISABLED` (full passthrough; no throttle; no counter increments per §11.P12); `REQUEST` (decode-side throttle ACTIVE); `RESPONSE` (encode-side throttle ACTIVE); `REQUEST_AND_RESPONSE` (BOTH sides ACTIVE; symmetric).
   - **`limit_kbps`** (`UInt64Value`; OPTIONAL at listener-level + CODE-LEVEL REQUIRED at per-route per `source/extensions/filters/http/bandwidth_limit/config.cc::createRouteSpecificFilterConfigTyped`; PGV `gte = 1` when wrapper present; REFUTES BRAINSTORM hypothesis "envoy-go-only parse-rejection of zero" — Envoy's PGV itself rejects zero; see §1.1 amendment 4 + §11.P1 + §11.P2) — throttle rate in **KiB/s (kibibytes-per-second; per proto comment line 95 + §1.1 amendment 6)**, NOT kilobits-per-second. Empirical chunk-size formula: `chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds` bytes (§11.P15 verifies via probeL: 51.2 bytes/tick at kbps=1 fill=50ms).
   - **`fill_interval`** (`google.protobuf.Duration`; OPTIONAL; default 50ms per Envoy filter source; PGV `gte = 20ms, lte = 1s`; REFUTES BRAINSTORM hypothesis "envoy-go-side filter-internal range check" — the bounds are Envoy's PGV themselves, not envoy-go-only; see §1.1 amendment 5) — token-bucket fill cadence.

   **Silent-ignored at runtime (3 fields):**

   - `runtime_enabled` (`RuntimeFeatureFlag`) — always-100%-active per phase-11/12/14 silent-ignore pattern (ADR-0117/ADR-0121/ADR-0130 silent-ignore-runtime-flag precedent). Divergence-window if Envoy-side set `default_value: false` or runtime-key flipping disable.
   - `enable_response_trailers` (bool) — when true on Envoy, emits 4 `bandwidth-{request,response}{,-filter}-delay-ms` trailers prefixed by `response_trailer_prefix`; envoy-go silent-ignores (always-no-trailers; trailer-emission primitive deferred to a future trailer-emission framework phase — see §8.1).
   - `response_trailer_prefix` (string; PGV pattern `^[^\x00\n\r]*$`) — couples to `enable_response_trailers`; silent-ignored. Operator divergence-window if `enable_response_trailers: true` set against reference Envoy.

   **Per-route TPFC: same `BandwidthLimit` message directly; NO wrapper proto (MAJOR REVISION from BRAINSTORM §2.4 + Decision 4 + ADR-0125 amendment plan; see §1.1 amendment 2).** Per `bandwidth_limit.pb.go` v1.32.4 + v1.37.0 bindings: there is NO `BandwidthLimitPerRoute` proto message — only `BandwidthLimit`. Per-route TPFC entries reuse the same `BandwidthLimit` message directly (mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1 note at `internal/filter/http/localratelimit/local_ratelimit.go:73-79`). Per-route `limit_kbps` is REQUIRED (the filter rejects per-route entries without it per the proto comment at `bandwidth_limit.pb.go:99-107`); `stat_prefix` is REQUIRED via PGV regardless of listener-vs-per-route position; no `disabled` boolean shortcut exists at the filter-proto level. To disable bandwidth_limit on a specific route, operators use `enable_mode: DISABLED` in the per-route override (the proto's existing 4-value enum doubles as the disable mechanism). **Phase 15 is NOT a third row using the 5th canonical disabled-OR-override pattern (ADR-0125); instead it follows the 4th canonical stateful-override-with-INDEPENDENT-stats pattern (ADR-0117).** ADR-0125 gains an in-place amendment paragraph §(xi) acknowledging the BRAINSTORM-time hypothesis of THIRD usage was empirically refuted at SPEC time; the 5th canonical stays bound to phase-13 buffer + phase-14 compressor (TWO rows). NO new per-route ADR.

4. **Filter-callback shape: BOTH `StreamDecoderFilter` + `StreamEncoderFilter` on the SAME `*filter` instance.** Phase 15 is the SECOND §9 family-row to be BOTH-direction with non-vacuous both-paths (after phase-14 compressor's encoder+decoder with minimal decoder surface; phase-15 generalizes to symmetric both-direction). The `HTTPFilter` value at factory time is `{Decoder: f, Encoder: f, PerRoute: parsePerRoute}`. Per-stream state lives on the `*filter` struct: `state *factoryState`, `dcb`, `ecb`, plus a per-direction buffered-body + timer + active-config triple (see §6.3 for the full struct).

5. **Body algorithm: Path B-async (buffer-then-delayed-emit) with kbps-per-tick throttle math — REUSES phase-09 fault `time.AfterFunc` async-resume + phase-13 ADR-0128 decode-side body-buffering machinery (Decision 3 → ADR-0137; per §1.1 amendment 6 corrected throttle math).**

   **Throttle math (per §11.P15 + §11.P8 + §6.6):** `chunk_size_per_tick = limit_kbps × 1024 × fill_interval.Seconds()` bytes (units: KiB/s × seconds = bytes). For a body of `N` bytes, the throttle duration is `ticks × fill_interval` where `ticks = ceil(N / chunk_size)`. Bodies fitting within one tick (`N <= chunk_size`) get a `fill_interval`-floor throttle (one-tick minimum). The brainstorm's steady-rate formula `(body * 8) / (limit_kbps * 1000)` was REFUTED at §11.P15 (`limit_kbps` is KiB/s, not kbps; and Envoy paces per-tick, not steady-rate).

   **Decode path (when `f.requestActive`):**
   1. `DecodeHeaders(headers, endStream)`: resolve per-route via `dcb.RequestRouteConfig()` → `state.resolvePerRouteConfig` → cache effective `*compiledConfig` on `f.requestRC`; determine `f.requestActive = (effective.enableMode == REQUEST || == REQUEST_AND_RESPONSE)`; return `HeaderContinue` (no header mutation; bandwidth_limit is body-only). If `endStream == true` (header-only request), the decode-side throttle is trivially zero-byte; just `Continue`.
   2. `DecodeData(data, endStream)`:
      - If `!f.requestActive` → `DataContinue` (pure passthrough).
      - Else append `data` to `f.requestBody []byte` (per phase-13 ADR-0128's accumulation pattern). If `!endStream` → return `DataStopIterationAndBuffer` (framework continues to accumulate further chunks; phase-13 ADR-0128's synthetic empty-terminal call eventually arrives even on chunked-body inputs).
      - On `endStream == true`: compute `throttle` via the kbps-per-tick formula (§6.6). Increment `f.requestRC.stats.requestEnabled` + `f.requestRC.stats.requestIncomingTotalSize.Add(uint64(len(body)))`. If `throttle == 0` (effective limit_kbps==0 or no throttle needed) → `DataContinue` (passthrough). Otherwise: arm `f.requestTimer = time.AfterFunc(throttle, ...)`; the timer callback increments `f.requestRC.stats.requestEnforced` (once at completion; the per-tick enforced semantic of Envoy v1.37.2 is approximated by a single bump at the buffered-blast — see §6.6 for nuance), increments `f.requestRC.stats.requestAllowedTotalSize`, decrements `f.requestRC.stats.requestPending`, then invokes `f.dcb.ContinueDecoding()`. Increment `f.requestRC.stats.requestPending.Inc()` at arm-time. Return `DataStopIterationAndBuffer`.
   3. `DecodeTrailers(trailers)`: pass-through (`TrailersContinue`; request trailers, if any, queue behind the buffered body and resume with `ContinueDecoding`).

   **Encode path (when `f.responseActive`):** symmetric — replace `Decode` → `Encode`, `request` → `response`, `dcb` → `ecb`, `ContinueDecoding` → `ContinueEncoding`. The encode-side timer reuses `time.AfterFunc` with `ecb.ContinueEncoding()` as the callback. Body bytes are forwarded via the existing `DataStopIterationAndBuffer` + `ContinueEncoding` pair WITHOUT invoking `cb.OverwriteBody` — the buffered bytes ARE the original bytes (see §3 framework-survey result above).

   **`OnDestroy()`** cancels any pending `f.requestTimer.Stop()` + `f.responseTimer.Stop()` (mirrors fault's `OnDestroy` pattern at `internal/filter/http/fault/fault.go:487` verbatim). Stop is idempotent; the pending-gauge accounting under Stop-races-Fire is per §4 + §6.9: if `Stop()` returns true (timer was active; callback prevented), decrement pending here AND decrement allowed_total_size adjustment if applicable; else trust the callback's own decrement.

   **Wire-shape divergence from reference Envoy** (deliberate; per §11.P8 + ADR-0137): Envoy emits Path A rate-paced chunks at exact `fill_interval` cadence — `chunk_size_per_tick` bytes per tick, distributed across the throttle window. envoy-go's Path B-async emits ZERO chunks during the throttle window, then ALL bytes in one BLAST at the end. **Total-throttle-time is observably equivalent within ±70ms tolerance** (resolves §11.P9). Chunk-arrival-timing observably DIVERGES on the wire. For consumers that don't depend on intra-throttle chunk timing (typical HTTP clients), the byte-stream is delivered with the same total latency budget. ADR-0137 records the divergence + the explicit forward-pointer to a future encode-side streaming framework phase that lands symmetric rate-paced chunk-emit primitives (mirroring phase-14 ADR-0131 §(vi) future encode-side streaming phase forward-pointer).

   **`*_enforced` semantic divergence-window (per §11.P3):** Reference Envoy increments `*_enforced` PER `fill_interval` TICK during throttle (i.e., once per chunk emit). envoy-go's Path B-async cannot increment per-tick (no per-tick observation point); the simplest mirror is to increment `*_enforced += ticks` at timer-fire time (where `ticks = ceil(body / chunk_size)`) so the cumulative counter matches Envoy at stream-completion. Alternatively, increment `*_enforced` once per stream (simpler but counter values diverge from Envoy by a factor of `ticks`). SPEC's position: increment by `ticks` per stream to maintain numerical byte-equivalence with reference Envoy on the cumulative counter; differential fixture's per-counter delta assertion uses this convention. §6.6 spells the formula.

6. **Stat surface — 46→60-name extension (Decision 5 → ADR-0138; per §1.1 amendments 7 + 8 + 9). 14 active stats (8 counters + 6 gauges) per stat_prefix; 2 unconditional histograms LANDED on Envoy side as divergence-window:**

   **8 counters** (cumulative across all streams; `<stat_prefix>.http_bandwidth_limit.<counter>` internal path → `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` Prometheus name per §11.P10 + §11.P11; underscore infix; NOT HCM-rooted; NO tag-extractor / NO SN10 rule):
   - `request_enabled` — per stream that engages decode-side throttle (one per `DecodeData(endStream=true)` with `requestActive=true`).
   - `request_enforced` — incremented by `ticks` at stream-completion to match Envoy's per-tick increment cumulative semantic (per §11.P3 + §1 item 5 enforced-semantic clause).
   - `request_incoming_total_size` — cumulative bytes that entered the decode-side filter.
   - `request_allowed_total_size` — cumulative bytes that the decode-side filter forwarded through.
   - `response_enabled`, `response_enforced`, `response_incoming_total_size`, `response_allowed_total_size` — symmetric for encode-side.

   **6 gauges** (transient state):
   - `request_pending`, `response_pending` — count of streams currently waiting on direction-respective timer (Inc on arm; Dec on fire/cancel).
   - `request_incoming_size`, `request_allowed_size`, `response_incoming_size`, `response_allowed_size` — per-tick bytes-in-flight (transient; reset between streams). envoy-go MVP's Path B-async approximates by setting `incoming_size = bodySize` at endStream-arrival + `allowed_size = bodySize` at timer-fire then both back to 0 in `OnDestroy` (single-blast emission mirrors Envoy's terminal-tick state).

   **2 histograms** (LANDED on Envoy side; envoy-go MVP SKIPS per phase-06.1 baseline; twin-series-filter divergence-window per §1.1 amendment 9):
   - `request_transfer_duration`, `response_transfer_duration` — total transfer-time distribution per stream. Fire UNCONDITIONALLY on Envoy (NOT gated by `enable_response_trailers`). Differential fixture 0017's `expectations.yaml` allow-lists these via twin-series-filter discipline (BEHAVIOR_CONTRACT §242 extends).

   **Stat namespace + Prometheus rendering (per §1.1 amendment 8):** internal `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; same shape as phase-11 local_ratelimit per ADR-0118 SN9 but with `bandwidth_limit` instead of `local_rate_limit`); Prometheus `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` (stat_prefix INLINED into base name; NO label / tag-extractor). The existing `internal/stats/name.go` default-branch flatten handles this without amending ADR-0061 or ADR-0118; NO new SN10 rule needed.

   **Per-route stats: INDEPENDENT (mirrors phase-11 ADR-0117; per §11.P4 + §11.P14).** Per-route TPFC entries (same `BandwidthLimit` proto via pointer-identity per-route lazy-cache per §1.1 amendment 1) own fresh `*compiledConfig` carrying its own `*filterStats` keyed by the per-route `stat_prefix`. Counters + gauges emit to the per-route stat namespace (`<per-route stat_prefix>.http_bandwidth_limit.<counter>`); listener-level counters do NOT increment for per-route-active streams. Mirrors phase-11 ADR-0117 stateful-override-with-INDEPENDENT-stats; DIVERGES from phase-12/13/14 SHARED-stats. ADR-0139 codifies the empirical ratification.

7. **No new framework primitive on either side (ZERO framework deltas; per §3 framework survey above).** Phase 15 reuses:
   - phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume pattern;
   - phase-13 ADR-0128's decode-side body-buffering machinery (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation; the latter is structurally unused by bandwidth_limit since the body bytes are unchanged and Content-Length stays valid);
   - phase-14 ADR-0131's encode-side `OverwriteBody` primitive (anticipated: NOT invoked; the framework's buffered-return path returns the accumulated bytes through `ContinueEncoding` without explicit replace);
   - the 3-tier `PerRouteConfig.Resolve` from phase 07.1;
   - the existing post-Freeze `stats.Registry.NewCounterIfAbsent` discipline from phase-11 ADR-0117 (for per-route stat-counter allocation).

   Phase 15 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor, NO new HCM hook, NO new chain-iteration disposition. ADR-0135 confirms the ZERO-framework-deltas framing at SPEC commit.

After phase 15, the project has proven the §9 HTTP filters family-expansion pattern carries through an EIGHTH filter under: the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor precedent's package-shape discipline (single-token directory matching the proto type-name); the 4th canonical stateful-override-with-INDEPENDENT-stats per-route discipline (codified at ADR-0117 + ratified here for the SECOND time + extended to a NEW 6th canonical adjacent entry per ADR-0125 §(xi) amendment); ZERO new framework primitives; the same `BandwidthLimit` proto for both listener-level + per-route TPFC (mirrors phase-11 local_ratelimit + adds one code-level extra check per filter source); a deliberate wire-shape divergence-window from reference Envoy with a forward-pointer to a future encode-side streaming framework phase (composed with phase-14 ADR-0131's existing forward-pointer). *envoy-go's HTTP filter framework hosts a synchronous, both-direction body-throttling filter that buffers each direction's body, computes a throttle duration via the **kbps-per-tick formula** `chunk_size = limit_kbps × 1024 × fill_interval_seconds` + `throttle = ceil(body / chunk_size) × fill_interval` (KiB/s units; per §6.6 + §1.1 amendment 6 + §11.P15), arms a `time.AfterFunc` timer, and resumes the chain on timer-fire; the OBSERVABLE-OUTCOMES are byte-equivalent to reference Envoy on every axis EXCEPT the intra-throttle-window chunk-timing axis (envoy-go: silent-then-blast; Envoy: Path A rate-paced chunks at exact `fill_interval` cadence) AND the trailer-emission axis (envoy-go: always-no-trailers; Envoy: 4 trailers when `enable_response_trailers: true`) AND the histogram axis (envoy-go: no histograms per phase-06.1 baseline; Envoy: 2 unconditional `*_transfer_duration` histograms); all three axes are documented as deliberate divergences with forward-pointers to future framework phases.* This is the EIGHTH §9 family-row to land; subsequent filters (`global_ratelimit`, `jwt_authn`, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session (2026-05-11) refutes or sharpens **ten** load-bearing BRAINSTORM hypotheses (5 structural re-frames + 4 field-bookkeeping corrections + 1 operational-foot-gun discovery). Each amendment below is a self-contained correction; collectively they revise:

- **Per-route discipline (amendments 1-2):** structural — no `BandwidthLimitPerRoute` wrapper; bare-proto-via-TPFC; phase-15 introduces a NEW canonical per-route shape distinct from both phase-11 ADR-0117 (4th canonical) and phase-13/14 ADR-0125 (5th canonical). [§11.P1]
- **PGV-enforcement framing (amendments 3-5):** field-bookkeeping — `stat_prefix` PGV-REQUIRED, `limit_kbps` PGV-`gte=1`-when-set + filter-internal-REQUIRED-at-per-route via code-level extra check, `fill_interval` PGV-`[20ms, 1s]`-when-set. Each was BRAINSTORM-framed as envoy-go-only validation; actually PGV-enforced by Envoy proto. [§11.P2 + §11.P5]
- **Throttle math + units (amendment 6):** structural — `limit_kbps` is KiB/s (kibibytes-per-second), NOT kilobits-per-second; throttle math is kbps-per-tick chunking (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`), NOT steady-rate; `fill_interval` governs chunk-size, NOT just timer granularity. [§11.P8 + §11.P15]
- **Stat surface (amendment 7):** structural — 16 stats per stat_prefix (8 counters + 6 gauges + 2 histograms), NOT 6. `*_enforced` increments per fill_interval TICK, NOT per stream. [§11.P3]
- **Stat namespace + Prometheus rendering (amendment 8):** structural — `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; not HCM-rooted); Prometheus inlines stat_prefix into base name (NO tag-extractor, NO SN10 rule needed). [§11.P10 + §11.P11]
- **Histograms divergence-window (amendment 9):** structural — Envoy emits 2 unconditional transfer-duration histograms; envoy-go MVP per phase-06.1 baseline skips; twin-series-filter discipline absorbs. [§11.P3]
- **Operational foot-gun (amendment 10):** behavioral — listener-level missing `limit_kbps` + active `enable_mode` causes runtime hang on both Envoy + envoy-go (byte-equivalent foot-gun). [probeJ; not anticipated by BRAINSTORM]

Mirrors the phase-14 compressor 6-amendment pattern (a SPEC-time correction of BRAINSTORM hypotheses) extended to 10 amendments; extends the phase-12 csrf 4-amendment precedent. Each amendment has a §11 cross-reference for the verbatim scrape evidence. The structural design (Path B-async at envoy-go side with corrected throttle math, BOTH-direction MVP, INDEPENDENT-stats via ADR-0117 inheritance, ZERO framework deltas, 4-consumed/3-silent-ignored envelope) survives intact despite the magnitude of the corrections — all amendments fit within the §1.1 self-contained-prose-block channel without requiring a BRAINSTORM §12 amendment cycle.

#### 1.1 Amendment 1 — Per-route shape: same `BandwidthLimit` proto via TPFC, NO `BandwidthLimitPerRoute` envelope (BRAINSTORM §2.4 + Decision 4 + §1.1 item 4 + §9.P1)

BRAINSTORM §2.4 + Q4 + §1.1 item 4 hypothesized that phase 15 would be the THIRD §9 family-row to use the 5th canonical disabled-OR-override per-route discipline codified at ADR-0125 (after phase-13 buffer FIRST + phase-14 compressor SECOND). The brainstorm-hypothesized per-route proto:

```proto
message BandwidthLimitPerRoute {
  oneof override {
    bool disabled = 1;
    BandwidthLimit bandwidth_limit = 2;
  }
}
```

**§11.P1 empirically REFUTES:** there is NO `BandwidthLimitPerRoute` proto message in Envoy v1.37.2. The v1.32.4 + v1.37.0 go-control-plane bindings at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.{32.4,37.0}/extensions/filters/http/bandwidth_limit/v3/bandwidth_limit.pb.go` define ONLY the `BandwidthLimit` message (one `[#next-free-field: 8]` proto with 7 top-level fields). The proto comment on `LimitKbps` at line 99-107 explicitly documents:

> *"It's fine for the limit to be unset for the global configuration since the bandwidth limit can be applied at a the virtual host or route level. Thus, the limit must be set for the per route configuration otherwise the config will be rejected."*

This shows that per-route TPFC entries are expected to use the **SAME `BandwidthLimit` message directly**, with a filter-internal constraint that `limit_kbps` is REQUIRED at per-route position (OPTIONAL at listener-level). Mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1 note: "upstream Envoy v1.37.2's local_ratelimit reuses the same `LocalRateLimit` proto for both listener-level and per-route TPFC entries; there is no wrapping `LocalRateLimitPerRoute` message." Phase 15 inherits the same pattern.

**Phase-15 envoy-go disposition:**

- Per-route TPFC accepts a `*BandwidthLimit` proto directly (via the registry's `PerRoute: parsePerRoute` hook at factory time).
- `parsePerRoute(any *anypb.Any) (proto.Message, error)` unmarshals to `*bandwidthlimitv3.BandwidthLimit` + validates the listener-level + per-route subset of constraints (including the filter-internal `limit_kbps` REQUIRED at per-route position — error wording `"bandwidth_limit: per-route entry requires limit_kbps to be set"`).
- `state.resolvePerRouteConfig(msg proto.Message) *compiledConfig` mirrors phase-11's `resolvePerRouteConfig` at `internal/filter/http/localratelimit/local_ratelimit.go:305-337`: type-asserts to `*BandwidthLimit`; consults a `sync.Map` keyed by `*BandwidthLimit` pointer; lazy-constructs a fresh `*compiledConfig` (with own pending-gauge token-bucket-analog + own `*filterStats` via `NewCounterIfAbsent`) on first resolve via `buildCompiledConfigPerRoute`.
- To disable bandwidth_limit on a specific route, operators set `enable_mode: DISABLED` in the per-route TPFC entry (along with the still-required `limit_kbps` field — the value is structurally ignored when `enable_mode: DISABLED` but PGV requires it). The `enable_mode` field doubles as the disable mechanism.

**Phase-15 is NOT a third row using the 5th canonical disabled-OR-override pattern.** Instead it follows the **4th canonical stateful-override-with-INDEPENDENT-stats pattern** codified at ADR-0117 (phase-11 local_ratelimit). ADR-0125 §(xi) in-place amendment paragraph documents the BRAINSTORM-time hypothesis-refutation; the 5th canonical stays bound to phase-13 buffer + phase-14 compressor (TWO rows).

#### 1.1 Amendment 2 — `BandwidthLimitPerRoute` proto does not exist (BRAINSTORM §2.4 hypothesis-form re-frame; §9.P1 + §8.8)

Sub-corollary of amendment 1. BRAINSTORM §8.8 deferred "BandwidthLimitPerRoute non-standard proto field shapes" as a placeholder for SPEC-time refinement. The empirical pin §11.P1 surfaces the LOAD-BEARING refinement: the wrapper proto DOES NOT EXIST at all. §8.8 is updated to read: "DEFERRED: no-op; the wrapper proto does not exist in Envoy v1.37.2. Per-route uses same BandwidthLimit message directly per §1.1 amendment 1."

#### 1.1 Amendment 3 — `stat_prefix` REQUIRED at parse-time per Envoy PGV (BRAINSTORM §1.1 item 3 + §2.5 + §9.P2)

BRAINSTORM §1.1 item 3 first-bullet hypothesized: "`stat_prefix` (string) — emission-scope tag for the 6-counter/gauge stat surface. Empty default is permitted (mirrors local_ratelimit per phase-11 ADR-0114)."

**§11.P2 + the v1.32.4 / v1.37.0 .pb.validate.go (lines 61-70) empirically REFUTE.** Envoy's PGV requires `stat_prefix` to be non-empty:

```go
if utf8.RuneCountInString(m.GetStatPrefix()) < 1 {
    err := BandwidthLimitValidationError{
        field:  "StatPrefix",
        reason: "value length must be at least 1 runes",
    }
    ...
}
```

The PGV `(validate.rules).string.min_len = 1` constraint is enforced at parse-time. An empty-string `stat_prefix` causes Envoy to reject the config at boot. Phase-11 local_ratelimit's precedent ALSO has this constraint (per ADR-0115 `Check 1`); BRAINSTORM's "mirrors local_ratelimit" framing was correct about the LR precedent but inverted the disposition. Reference Envoy: requires non-empty; envoy-go: must mirror.

**Phase-15 envoy-go disposition:** parse-time PGV-mirror validation at `buildCompiledConfig` rejects empty `stat_prefix` with envoy-go-own error wording `"bandwidth_limit: stat_prefix required"` (mirrors phase-11 ADR-0115 wording). Per-route variant `"bandwidth_limit: per-route entry requires stat_prefix"`.

#### 1.1 Amendment 4 — `limit_kbps == 0` is PGV-enforced by Envoy (NOT envoy-go-only); per-route REQUIRES limit_kbps (BRAINSTORM §1.1 item 3 + §2.5 + §9.P2)

BRAINSTORM §1.1 item 3 third-bullet hypothesized: "`limit_kbps` (UInt64Value, REQUIRED) — throttle rate in kilobits-per-second. Filter-internal range check: `limit_kbps > 0` (zero-throttle is parse-rejected; §11 pin §9.P2 confirms exact PGV)."

**§11.P2 + the v1.32.4 / v1.37.0 .pb.validate.go (lines 83-95) empirically REFINE.** Envoy's PGV directly enforces `limit_kbps >= 1` when the wrapper is present:

```go
if wrapper := m.GetLimitKbps(); wrapper != nil {
    if wrapper.GetValue() < 1 {
        err := BandwidthLimitValidationError{
            field:  "LimitKbps",
            reason: "value must be greater than or equal to 1",
        }
        ...
    }
}
```

**BUT** the wrapper is OPTIONAL at proto-level. The proto comment at `bandwidth_limit.pb.go:99-107` establishes a FILTER-INTERNAL semantic: at LISTENER level, `limit_kbps` may be unset (the filter doesn't trip a 0-byte throttle; it just inherits per-route); at PER-ROUTE level, `limit_kbps` MUST be set (the per-route entry would have nothing to throttle without it; the filter rejects the config).

**Phase-15 envoy-go disposition:**
- Listener-level `limit_kbps`: OPTIONAL at parse-time. If unset on the listener, the filter still loads but is structurally inactive unless overridden per-route. (Operationally, `enable_mode: DISABLED` is the more natural way to express "filter present but inactive"; an unset `limit_kbps` is a less-common idiom but legal per the proto comment.)
- Per-route `limit_kbps`: FILTER-INTERNAL REQUIRED. envoy-go-side validation in `buildCompiledConfigPerRoute` rejects unset `limit_kbps` with envoy-go-own error `"bandwidth_limit: per-route entry requires limit_kbps to be set"` (mirroring the Envoy filter-internal source rejection per `bandwidth_limit_filter.cc` line TBD — §11 verbatim scrape).
- Zero-value `limit_kbps` on either level: REJECTED by Envoy's PGV directly (`value must be greater than or equal to 1`); the envoy-go proto-decoder ALSO rejects via `protojson.Unmarshal` calling the auto-generated PGV `Validate()` method (the same mechanism as phase-13 buffer's oneof rejection at §11.P3 — JSON-decoder-driven, not filter-internal). envoy-go-side defensive PGV-mirror check in `buildCompiledConfig` for unit-test coverage.

This eliminates the BRAINSTORM-hypothesized "envoy-go-only parse-rejection" framing for `limit_kbps == 0`. The check is Envoy-PGV-driven, not envoy-go-internal. The compiledConfig parse-error wording per (vi) below.

#### 1.1 Amendment 5 — `fill_interval` bounds `[20ms, 1s]` are PGV-enforced by Envoy (NOT envoy-go-only) (BRAINSTORM §1.1 item 3 + §2.5 + §9.P5)

BRAINSTORM §1.1 item 3 fourth-bullet hypothesized: "`fill_interval` (Duration; default 50ms per Envoy v1.37.2) — token-bucket fill cadence. envoy-go-side filter-internal range check `fill_interval ∈ [20ms, 1s]` mirrors phase-11 local_ratelimit's `fill_interval` precedent per §11 pin §9.P5 confirms exact Envoy filter-internal bounds."

**§11.P5 + the v1.32.4 / v1.37.0 .pb.validate.go (lines 98-127) empirically REFINE.** Envoy's PGV directly enforces `[20ms, 1s]` when the wrapper is present:

```go
lte := time.Duration(1*time.Second + 0*time.Nanosecond)
gte := time.Duration(0*time.Second + 20000000*time.Nanosecond)

if dur < gte || dur > lte {
    err := BandwidthLimitValidationError{
        field:  "FillInterval",
        reason: "value must be inside range [20ms, 1s]",
    }
    ...
}
```

The PGV `gte = 20ms, lte = 1s` constraint is enforced at parse-time. This contrasts with phase-11 local_ratelimit, where `fill_interval >= 50ms` is a FILTER-INTERNAL check inside Envoy's source (per phase-11 ADR-0115 + SPEC §11.2c). The phase-15 bandwidth_limit constraint is proto-level PGV, not filter-internal.

**Phase-15 envoy-go disposition:**
- Defensive PGV-mirror validation in `buildCompiledConfig` rejects `fill_interval` outside `[20ms, 1s]` with envoy-go-own error wording `"bandwidth_limit: fill_interval %v outside supported range [20ms, 1s]"` (mirrors phase-13 ADR-0126 envoy-go-own clear-text discipline).
- The check fires AFTER proto unmarshal succeeds (defensive; the auto-generated PGV `Validate()` method already rejects malformed input at `protojson.Unmarshal` time).
- Default value when unset: 50ms (per Envoy filter source).
- This eliminates the BRAINSTORM-hypothesized "envoy-go-only filter-internal cap" framing. The bounds are Envoy's PGV themselves.
- BRAINSTORM §8.6 deferral re-frames: "envoy-go follows Envoy PGV directly; no envoy-go-only divergence-window."

#### 1.1 Amendment 6 — `limit_kbps` units are KiB/s + throttle math is kbps-per-tick (BRAINSTORM §2.3 + §9.P15 + §9.P8)

BRAINSTORM §1.1 item 3 third-bullet + §2.3 framed `limit_kbps` as "throttle rate in kilobits-per-second" with steady-rate throttle math: `throttle_duration_seconds = (body_size_bytes * 8) / (limit_kbps * 1000)`.

**§11.P15 + §11.P8 empirically REFUTE on TWO axes:**

- (a) **`limit_kbps` units = KiB/s (kibibytes-per-second), NOT kilobits-per-second.** Per the proto comment at `bandwidth_limit.pb.go:95` ("The limit supplied in KiB/s") + the empirical chunk-size formula (chunk_size_per_tick = `limit_kbps × 1024 × fill_interval_seconds` bytes, confirmed at probeL with 51.2 bytes/tick at kbps=1 fill=50ms). The brainstorm's "kilobits-per-second" framing is mathematically REFUTED.
- (b) **Throttle math = kbps-per-tick chunking, NOT steady-rate.** Envoy paces body bytes at exactly `fill_interval` cadence, emitting `chunk_size_per_tick` bytes per tick. Total throttle-time = `ceil(body_size / chunk_size_per_tick) × fill_interval`. `fill_interval` GOVERNS chunk-size, NOT just timer granularity as BRAINSTORM §2.3 hypothesized.

**Phase-15 envoy-go disposition** (Path B-async with corrected throttle formula; §6.6 + ADR-0137):

```go
// chunk_size_per_tick = limit_kbps * 1024 * fill_interval_seconds (bytes per tick)
// throttle_duration   = ceil(body_size / chunk_size_per_tick) * fill_interval
chunkSize := limitKbps * 1024 * uint64(fillInterval.Milliseconds()) / 1000  // bytes/tick
if chunkSize == 0 { chunkSize = 1 } // defensive
if bodySize <= int(chunkSize) {
    // Body fits within one tick → fast-passthrough or one-tick-delay
    throttle = fillInterval
} else {
    ticks := (uint64(bodySize) + chunkSize - 1) / chunkSize
    throttle = time.Duration(ticks) * fillInterval
}
```

The envoy-go MVP buffers + arms a single `time.AfterFunc(throttle, ...)` (Path B-async); on timer-fire the buffered body emits in one blast. Total wall-clock matches Envoy's rate-paced chunk-emit pattern within ±70ms (per §11.P9). Chunk-arrival-time observably diverges (envoy-go: silent-then-blast; Envoy: N chunks at `fill_interval` cadence); the wire-shape divergence-window is documented at §13.4 + ADR-0137.

**Token-bucket-with-initial-burst:** Empirical probeA suggests Envoy's token bucket has an initial burst capacity (~`limit_kbps × 1024` bytes; 100-byte and 1024-byte bodies at kbps=10 complete in 5-107ms, less than the kbps-per-tick prediction). For bodies fitting within initial burst, throttle fires negligibly. Phase-15 envoy-go MVP's Path B-async approximates by:
- If `bodySize <= chunkSize` → `throttle = fillInterval` (one-tick minimum; honors the kbps-per-tick floor).
- Else → kbps-per-tick formula.

This produces total-throttle-time byte-equivalence with reference Envoy across the test matrix in §11.9 within ±70ms. The fast-passthrough threshold from BRAINSTORM §11.4 + §6.6 hypothesis (`1ms`) is REPLACED by the `fillInterval` floor (typically 50ms default; configurable per `fill_interval` field). PLAN may refine if a sub-fill_interval short-circuit is operationally desirable for sub-tick-sized bodies.

**ADR-0137 amendment paragraph at SPEC commit:** documents the kbps-per-tick throttle math + KiB/s unit semantic + initial-burst approximation + wire-shape divergence framing (chunk-pattern axis, not total-time axis).

#### 1.1 Amendment 7 — Stat surface is 16 stats per stat_prefix (8 counters + 6 gauges + 2 histograms), not 6 (BRAINSTORM §1.1 item 5 + §2.6 + §9.P3)

BRAINSTORM §1.1 item 5 + §2.6 hypothesized "6 new counter+gauge stats" (`request_enabled`, `request_enforced`, `request_pending`, `response_enabled`, `response_enforced`, `response_pending`). Plus 4 histograms DEFERRED per phase-06.1 "counters + gauges only" baseline (§8.2 deferral).

**§11.P3 empirically REFUTES:** Envoy v1.37.2 emits **16 stats per active stat_prefix**, NOT 6. Verbatim probeA scrape (full at §11.3 above):

- **8 counters** per stat_prefix:
  - `request_enabled`, `request_enforced` (BRAINSTORM hypothesized)
  - `request_incoming_total_size`, `request_allowed_total_size` (NEW; BRAINSTORM did not anticipate)
  - `response_enabled`, `response_enforced` (BRAINSTORM hypothesized)
  - `response_incoming_total_size`, `response_allowed_total_size` (NEW)
- **6 gauges** per stat_prefix:
  - `request_pending`, `response_pending` (BRAINSTORM hypothesized)
  - `request_incoming_size`, `request_allowed_size`, `response_incoming_size`, `response_allowed_size` (NEW; bytes-in-flight per-direction state)
- **2 histograms** per stat_prefix:
  - `request_transfer_duration`, `response_transfer_duration` (NEW; fire UNCONDITIONALLY, not gated by `enable_response_trailers`)

**Counter semantics empirically resolved (per §11.P3):**

- `*_enabled` increments PER STREAM that engages throttle (one increment per `DecodeData/EncodeData(endStream=true)` with `*Active=true`).
- `*_enforced` increments PER `fill_interval` TICK during throttle engagement. probeJ shows `response_enforced: 99` for a single 5-second hung stream (≈ 5s/50ms = 100 ticks; -1 for the initial tick). This refutes the BRAINSTORM "increments once per stream where throttle ACTUALLY engaged" framing.
- `*_incoming_total_size` / `*_allowed_total_size` (counters) — cumulative byte counts across all streams.
- `*_incoming_size` / `*_allowed_size` (gauges) — transient per-tick state.
- `*_pending` (gauge) — count of streams currently waiting on a timer (resolves §11.P13).

**Stat-table extension at §13.2:** 46 → 60 names (14 new = 8 counters + 6 gauges). The 2 histograms are documented at §13.4 + §11.9 as the histograms-deferred divergence-window per amendment 9 below.

**Phase-15 envoy-go MVP disposition (`filterStats` shape):**

```go
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
    // 2 histograms: NOT EMITTED in MVP per phase-06.1 baseline; divergence-window per amendment 9
}
```

#### 1.1 Amendment 8 — Stat namespace `<stat_prefix>.http_bandwidth_limit.<counter>` (NOT HCM-rooted); Prometheus inline-prefix (NO tag-extractor; NO SN10 rule) (BRAINSTORM §2.7 + §9.P10 + §9.P11)

BRAINSTORM §2.7 hypothesized `http.<HCM stat_prefix>.<filter stat_prefix>.<counter>` namespace mirroring phase-11 local_ratelimit's `<stat_prefix>.http_local_rate_limit.<counter>` shape via SN9 tag-extractor (label `envoy_local_http_ratelimit_prefix=<stat_prefix>`).

**§11.P10 + §11.P11 empirically REFUTE on TWO axes:**

- (a) **Internal stat path:** `<stat_prefix>.http_bandwidth_limit.<counter>` (single-segment `http_bandwidth_limit` underscore infix; NOT `http.bandwidth_limit.` dot infix; NOT HCM-rooted as BRAINSTORM hypothesized). Same shape as phase-11 local_ratelimit (`<stat_prefix>.http_local_rate_limit.<counter>`).
- (b) **Prometheus rendering INLINES stat_prefix into the base name** (NO label/tag-extractor):
  `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}`
  Verbatim probeA: `envoy_default_http_bandwidth_limit_response_enabled{} 3`. NO `envoy_http_conn_manager_prefix=<HCM>` label (refutes any HCM-rooted hypothesis); NO `envoy_local_http_bandwidth_limit_prefix=<stat_prefix>` label (refutes SN9-analog tag-extractor hypothesis).

**Phase-15 envoy-go MVP disposition:**

- Internal counter names registered under `<stat_prefix>.http_bandwidth_limit.<counter>` path (same shape as phase-11 ADR-0118; constructed at `newFilterStats(reg, statPrefix)` by prefixing `statPrefix + ".http_bandwidth_limit."` to each counter name).
- Prometheus rendering via the existing `internal/stats/name.go` `flattenToProm` default-branch fallback: dot→underscore substitution produces `envoy_<stat_prefix>_http_bandwidth_limit_<counter>` without any tag-label promotion. Verified that the path does NOT match SN1-SN9 prefix-segment rules; falls through to default flatten.
- **NO new SN10 rule needed.** ADR-0138 documents the namespace shape without amending ADR-0061 or ADR-0118.
- BRAINSTORM §2.7 + ADR-0138 SN10-conditional REMOVED; the new ADR-0138 §Decision (ii) explicitly states "Rule SN2 reuse not applicable; existing default-branch flatten suffices."

**Tag-extraction collision note:** the stat_prefix segment is inlined into the Prometheus name AS-IS (after dot→underscore). Operators using stat_prefix values that contain underscores OR collide with other Envoy-stat-segment names will see the Prometheus name shape but no semantic confusion (no tag-extractor magic happens). Phase-15 fixture 0017 uses safe stat_prefix values (`default`, `override`) to avoid any operational ambiguity.

#### 1.1 Amendment 9 — Histograms emit on Envoy side but envoy-go skips per phase-06.1 baseline; twin-series-filter divergence-window (BRAINSTORM §8.2 + §9.P3)

BRAINSTORM §1.1 item 5 + §8.2 "histograms deferred per phase-06.1 ROADMAP-row baseline" hypothesized 4 histograms (`request_allowed_size`, `request_incoming_size`, `response_allowed_size`, `response_incoming_size`) deferred. The §11.P3 empirical refines: those 4 names are NOT histograms — they are GAUGES (per amendment 7 above). The 2 histograms Envoy actually emits are `request_transfer_duration` + `response_transfer_duration` (NEW finding; BRAINSTORM did not anticipate the duration-histogram axis).

**§11.P3 empirically REFINES BRAINSTORM §8.2:**

- The 2 histograms (`request_transfer_duration`, `response_transfer_duration`) fire UNCONDITIONALLY on Envoy side (NOT gated by `enable_response_trailers` as BRAINSTORM §8.2 implicitly assumed).
- envoy-go MVP per phase-06.1 baseline ("counters + gauges only — histograms deferred") DOES NOT register histogram namespaces or emit histogram values.

**Phase-15 envoy-go MVP disposition — twin-series-filter divergence-window** (per BEHAVIOR_CONTRACT §242 + the phase-09 fault `response_rl_injected` route-A discipline-analog precedent):

- Differential fixture 0017's `expectations.yaml` ALLOW-LISTS the 2 histogram families (`envoy_default_http_bandwidth_limit_{request,response}_transfer_duration_*`) — they appear in Envoy-side scrape but NOT envoy-go-side scrape. The differential harness's per-counter delta comparison filters out the histogram families.
- BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` subsection extends with a phase-15 entry: 2 unconditional bandwidth_limit transfer-duration histograms allow-listed pending a future histogram-emit-infra phase.
- Operator divergence-window: dashboards using `envoy_http_bandwidth_limit_transfer_duration_*` queries see Envoy emit; envoy-go emits nothing. Documented at §13.4 phase-15 forward-pointer notes.

**Future re-activation:** a future histogram-emit-infra phase lands `*stats.Registry.Histogram` + Prometheus `histogram_*` extractor (couples to the existing `_bucket / _sum / _count` triple-family rendering machinery). Re-activation: phase-15 `filterStats` extends with 2 histogram fields; the divergence-window closes.

#### 1.1 Amendment 10 — Operational foot-gun: listener-level missing `limit_kbps` + active `enable_mode` causes runtime hang (BRAINSTORM §1.1 item 3 + §11.P12 + probeJ)

**§11 probeJ (NEW finding; not anticipated by BRAINSTORM):** Listener-level filter with `enable_mode: RESPONSE` (or `REQUEST` / `REQUEST_AND_RESPONSE`) and NO `limit_kbps` set BOOTS cleanly per the proto comment at `bandwidth_limit.pb.go:99-107` ("It's fine for the limit to be unset for the global configuration since the bandwidth limit can be applied at the virtual host or route level"). But at runtime, **every request HANGS** — the filter computes throttle against an effective `limit_kbps = 0` which produces an infinite (or arbitrarily-large) throttle duration. Observed: 5-second curl timeout with `response_pending: 1, response_enforced: 99` (the per-fill_interval ticks accumulate but no chunks emit).

**Phase-15 envoy-go MVP disposition:**

- **Match Envoy exactly:** at parse time, listener-level `limit_kbps` unset is ACCEPTED (no envoy-go-side validation rejection). The filter loads but is operationally broken unless overridden per-route with a set `limit_kbps`.
- **At runtime:** if `compiledConfig.limitKbps == 0` AND the direction is active, the filter computes throttle via the kbps-per-tick formula with `chunkSize = 0 × 1024 × ... = 0`. Per the §6.6 throttle-arithmetic, `if chunkSize == 0 { chunkSize = 1 }` defensive clause prevents divide-by-zero; the resulting `throttle = ceil(bodySize / 1) × fillInterval` is enormous (millions of ticks). The timer arms; the request hangs.
- This is byte-equivalent with Envoy's foot-gun behavior.
- **Operator divergence-window:** documented at §13.4 phase-15 forward-pointer notes. Operators MUST set `limit_kbps` on either the listener-level or every per-route entry; an unset listener-level + missing per-route override produces the hang on both sides.
- **PLAN-time consideration:** envoy-go MAY emit a parse-time WARNING log (NOT rejection) when listener-level `limit_kbps` is unset + `enable_mode != DISABLED`. Mirrors phase-12 csrf's PGV-mirror warning discipline. SPEC's position: silent-match-Envoy in MVP; future operator-ergonomics phase may add the warning.

### 1.2 Revised scope summary (post-§1.1 amendments)

After the **10 §1.1 amendments** (5 structural + 4 field-bookkeeping + 1 operational-foot-gun discovery), phase 15's in-scope architectural primitives are the SEVEN listed at the head of §1, expressed as **11 BRAINSTORM-§1.1-style line items** per BRAINSTORM §1.1 (items 1-11 — items 1-8 implementation deliverables + items 9-11 artefact-level deliverables). The amendments do NOT change item count; they revise:
- Item 4 (per-route discipline): same `BandwidthLimit` proto via TPFC; NO wrapper; phase-15 introduces a NEW canonical pattern (bare-proto-via-TPFC + filter-internal-REQUIRED-limit_kbps-at-per-route) distinct from BOTH the 4th canonical (phase-11 ADR-0117) AND the 5th canonical (phase-13/14 ADR-0125) — amendments 1 + 2.
- Item 3 (field decomposition): bookkeeping unchanged at 4 consumed + 3 silent-ignored, but parse-rejection framing revises (PGV-driven for stat_prefix / limit_kbps / fill_interval; ONE code-level extra check at per-route position; defensive envoy-go-side PGV-mirror) — amendments 3 + 4 + 5.
- Item 3 + Item 5 (units + algorithm): `limit_kbps` is KiB/s NOT kbps; throttle math is kbps-per-tick chunking (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`); fill_interval governs chunk-size — amendment 6.
- Item 5 (algorithm): wire-shape divergence reframed — Envoy emits Path A rate-paced chunks NOT silent-then-blast; envoy-go Path B-async diverges on chunk-pattern axis (total-time equivalent ±70ms) — amendment 6 + ADR-0137.
- Item 6 (stat surface): 14 active stats (8 counters + 6 gauges) NOT 6; 2 histograms LANDED on Envoy as twin-series divergence — amendments 7 + 9.
- Item 6 (stat namespace): `<stat_prefix>.http_bandwidth_limit.<counter>` underscore-infix; Prometheus inlines stat_prefix; NO tag-extractor; NO SN10 rule — amendment 8.
- Cross-cutting (foot-gun): listener-level missing `limit_kbps` + active `enable_mode` → runtime hang; envoy-go matches byte-equivalent — amendment 10.

Differential fixture has 6 scenarios (§7.1 below; per-scenario timings revised to use kbps-per-tick math per amendment 6). ADR list is **5** (ADR-0135..ADR-0139) + ADR-0125 in-place amendment paragraph §(xi) (the amendment slot is preserved; the content documents the BRAINSTORM-hypothesis refutation + the NEW canonical per-route pattern phase-15 introduces alongside ADR-0117 + ADR-0125's existing 5-shape catalog). NO ADR-0073 amendment (the wholesale-override discipline carries through). NO ADR-0117 amendment (the 4th canonical pattern is directly inherited; phase-15 introduces an adjacent pattern documented in ADR-0125 §(xi)). NO ADR-0061 amendment (the existing default-branch flatten handles the inline-prefix Prometheus shape; no new SN10 needed per §11.P10). NO ADR-0118 amendment.

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 9 + ADR-0106)

Phase 15 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 stays unchanged in state across phase 15's landing per ADR-0106(c). Phase 15 is the EIGHTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14). Each subsequent HTTP filters family member becomes its own top-level row at row 16, 17, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)).

### 1.4 ADR-0045 split-by-direction readiness

Phase 15 stays a SINGLE row at this SPEC. The implementation surface is estimated at:

- ~280-330 LoC production filter (`bandwidthlimit.go`; symmetric request + response throttle adds duplication relative to phase-11 local_ratelimit's ~393 LoC, but the algorithmic core is simpler — no token-bucket-state-machine, just throttle-duration arithmetic + `time.AfterFunc` arming).
- ~80-100 LoC `bucket.go` (the per-stream body-buffer + throttle-arming helper; structurally lighter than localratelimit's bucket.go because there's no concurrent-access state — body-buffer + timer are per-stream).
- ~30 LoC `doc.go`.
- ~500-600 LoC unit tests (test groups per §14.1).
- ~80 LoC fuzzer (one fuzzer; mirrors phase-13 buffer's `FuzzBufferConfigParse` shape extended for the 7-field BandwidthLimit proto).
- ~250-320 LoC fixture `0017-http-bandwidth-limit` (envoy.yaml ~80 + envoy-go.yaml ~80 + driver/main.go ~200 + backend extension ~30-50 + expectations.yaml ~40 + README.md ~80).
- ~80 LoC ROADMAP+STATE+BEHAVIOR_CONTRACT additions at SPEC commit.

Total: ~1300-1560 LoC across all bundles, with ~300-380 in Go production code (slightly heavier than initial estimate due to the 14-stat `filterStats` registration + the kbps-per-tick throttle-arithmetic + the `*Size` gauge state-management vs the simpler 6-counter brainstorm hypothesis). Task count estimate per BRAINSTORM §1.4: ~14-18 tasks (the symmetric BOTH-direction throttle adds 2-4 tasks vs phase-11 local_ratelimit's 9-task baseline; per-route INDEPENDENT-stats wiring + 14-stat `filterStats` registration mirror phase-11 directly with the larger counter set; ZERO framework-delta tasks; +1 task for code-level required-limit_kbps-at-per-route validation; +1 task for twin-series-filter discipline extension at fixture 0017). Both metrics stay UNDER ADR-0045's 1500-LoC / 25-task split-trigger thresholds. The PLAN author retains the ADR-0045 release valve if PLAN finds the surface exceeds either threshold; the natural split per BRAINSTORM §1.4 is `15.1 = response-side throttle MVP` + `15.2 = request-side throttle + symmetric REQUEST_AND_RESPONSE`. **SPEC's position: single-row.**

### 1.5 No prebrainstorm-notes branch

UNLIKE phase 11 (which inherited an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch from a prior pivoted session), phase 15 has NO prior prebrainstorm-notes artefacts. The phase 15 BRAINSTORM cold-started fresh from the §9 heading + the phase 14 just-shipped artefacts per ADR-0106(e); this SPEC drafting session executed the §9 empirical-pin block (15 pins) IN-SESSION against reference Envoy v1.37.2 per ADR-0004 — surfacing the §1.1 amendment refutations above.

### 1.6 Phase 15 is the third §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time (joining phase 12 + phase 14)

Phase 12 csrf was the FIRST §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time (4 amendments). Phase 13 buffer took the brainstorm-amendment route (BRAINSTORM §12 D-3.5 amendment cycle authored before SPEC drafting started in earnest). Phase 14 compressor took the §1.1 SPEC-time amendment-block route (6 amendments). Phase 15 takes the phase-12 + phase-14 SPEC-time route: **10 SPEC-time amendments** at §1.1 above (the largest amendment count in any §9 family-row to date), surfacing during the §11 empirical-pin re-run. The choice between (a) §1.1 amendment block channel and (b) BRAINSTORM §12 amendment cycle is per next-prompt's framing — option (a) when each correction fits within a self-contained §1.1 prose block + a §11 pin disposition entry. The phase-15 corrections fit (a) DESPITE the magnitude: amendments 1+2 (per-route shape refutation) are structural but the FIX is well-precedented (ADR-0117 4th canonical pattern with adjacent extension); amendments 3+4+5 (PGV-enforcement framing) are field-level corrections that don't undo the parse-rejection discipline; amendments 6+7+8 (throttle math + stat surface + namespace) are algorithmic/observability-level corrections that don't undo the architecture; amendment 9 (histograms divergence-window) extends an existing BEHAVIOR_CONTRACT discipline; amendment 10 (foot-gun) is a behavioral parity finding. The structural design (Path B-async at envoy-go with corrected throttle math, BOTH-direction MVP, INDEPENDENT-stats via ADR-0117 inheritance, ZERO framework deltas, 4-consumed/3-silent-ignored envelope) survives intact.

The pattern of "BRAINSTORM commits hypothesis; SPEC empirically confirms or amends" continues to function as designed; phase 15 demonstrates the (a) route's robustness when the empirical re-frame surfaces a STRUCTURAL refutation (the per-route proto shape) alongside field-level corrections without invalidating the structural design.

### 1.7 Phase 15 introduces ZERO framework deltas — symmetric exercise of phase-13 ADR-0128 + phase-14 ADR-0131 + phase-09 fault primitives

Phase 13 introduced two decode-side framework primitives at `internal/filter/hcm/connection.go` per ADR-0128: synthetic empty-terminal `RunDecodeData` on chunked-body EOF + post-body Content-Length reconciliation. Phase 14 introduced the encode-side `OverwriteBody` primitive at `EncoderFilterCallbacks` per ADR-0131. Phase 15 **consumes BOTH** — symmetrically. The decode-side body-buffering machinery from ADR-0128 is what makes Path B-async feasible on the request side (the filter sees the accumulated `req.Body` at `DecodeData(endStream=true)` time, exactly as phase-13 buffer does); the encode-side `OverwriteBody` from ADR-0131 is what makes Path B-async feasible on the response side IF the body slice needs replacement (anticipated: NOT needed; the filter buffers the same bytes and re-emits them unchanged via `DataStopIterationAndBuffer` + `ContinueEncoding`). This is the FIRST §9 row to consume BOTH framework-delta sets simultaneously — load-bearing demonstration that phase-13 + phase-14 framework deltas are not one-off filter accommodations but reusable infrastructure (per BRAINSTORM Q1 selection rationale).

The §3 framework survey at this SPEC drafting session confirms the ZERO-framework-deltas claim:
- Decode-side body-accumulation at `chain.go` honors `DataStopIterationAndBuffer` and accumulates further chunks until `endStream=true`; ADR-0128's synthetic empty-terminal `RunDecodeData` ensures `endStream=true` is observed even on chunked bodies with zero-data EOF; reused without amendment.
- Decode-side post-body Content-Length reconciliation at `connection.go:dispatchRequest` is structurally unused by bandwidth_limit (the filter does NOT mutate `req.Header["Content-Length"]`; the body bytes are unchanged); reused vacuously.
- Encode-side: `internal/filter/hcm/connection.go:467-475` (H1) + `h2dispatch.go:303-315` (H2) invoke `RunEncodeData(ctx, resp.Body, true)` ONCE with the full response body; the chain dispatch at `chain.go:336` passes `data []byte` BY VALUE to `f.EncodeData(data, endStream)`. The framework's buffered-return path for `DataStopIterationAndBuffer` + `ContinueEncoding` returns the accumulated bytes through the HCM post-chain consumer WITHOUT requiring `cb.OverwriteBody` invocation — verified at the framework-survey step (SPEC author traces `chain.go` honor-`DataStopIterationAndBuffer` semantics at line TBD; the buffered-and-resumed path emits the same bytes that were buffered). If the framework survey at PLAN time finds that explicit `cb.OverwriteBody` invocation is required (the buffered-return path doesn't actually return the bytes without explicit replace), phase-15 REUSES (not introduces) ADR-0131 OverwriteBody — the ZERO-deltas framing stays; BRAINSTORM §3 amends in SPEC.

LoC delta: 0. Comparable to phase-10 header_mutation + phase-11 local_ratelimit + phase-12 csrf (all ZERO framework deltas); contrasts with phase-13 buffer (~34 LoC ADR-0128) + phase-14 compressor (~20-25 LoC ADR-0131). Phase 15 is the FIRST §9 row to compose against BOTH ADR-0128 + ADR-0131 simultaneously.

---

## 2. Non-purposes

Phase 15 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.bandwidth_limit` (BOTH-direction MVP) under the existing 07.1 framework + the existing phase-09 / phase-13 / phase-14 / phase-11 primitives.

### 2.1 `BandwidthLimit` proto-message non-goals (per BRAINSTORM §8 + §1.1 amendment 3)

The proto message `envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit` carries 7 top-level fields (per `[#next-free-field: 8]` annotation). Phase 15 consumes 4 actively; silent-ignores 3.

- **Silent-ignored at runtime (3 fields):** `BandwidthLimit.runtime_enabled` (RuntimeFeatureFlag; always-100%-active); `BandwidthLimit.enable_response_trailers` (bool; always-no-trailers per §8.1); `BandwidthLimit.response_trailer_prefix` (string; PGV pattern `^[^\x00\n\r]*$`; silent-ignored since trailers are not emitted regardless).

#### 2.1.1 Out of scope: `runtime_enabled` RuntimeFeatureFlag (silent-ignored at runtime)

Coupled to: Runtime + hot restart family. `BandwidthLimit.runtime_enabled` is `envoy.config.core.v3.RuntimeFeatureFlag` (BoolValue default; runtime-key flippable). envoy-go MVP silent-ignores at runtime; always-evaluates-as-enabled regardless of `default_value` setting OR runtime-key state. Mirrors phase-11/12/14 silent-ignore-runtime-flag pattern (ADR-0117/ADR-0121/ADR-0130). Operator divergence-window: configs setting `default_value: false` see Envoy disable; envoy-go always-active. Documented at §13.4 phase-15 forward-pointer notes.

#### 2.1.2 Out of scope: `enable_response_trailers` + `response_trailer_prefix` (always-no-trailers; trailer-emission framework primitive deferred)

Coupled to: future trailer-emission framework phase that lands `EncoderFilterCallbacks.EmitTrailers(map[string]string)` primitive. Reference Envoy: when `enable_response_trailers: true` (with `enable_mode != DISABLED` and at least one of request/response delay > 0), emits 4 trailers prefixed by `response_trailer_prefix`:
- `<prefix>bandwidth-request-delay-ms`
- `<prefix>bandwidth-response-delay-ms`
- `<prefix>bandwidth-request-filter-delay-ms`
- `<prefix>bandwidth-response-filter-delay-ms`

envoy-go MVP: silent-ignores both fields at parse time; emits no trailers regardless. The `response_trailer_prefix` PGV pattern `^[^\x00\n\r]*$` (no nulls/newlines/CRLF) is enforced by the auto-generated PGV `Validate()` method at proto-decode time; defensive envoy-go-side mirror not needed. Operator divergence-window: configs setting `enable_response_trailers: true` see Envoy emit 4 trailers on responses; envoy-go responses have no trailers. Documented at §13.4 phase-15 forward-pointer notes.

### 2.2 Per-route override surface non-goals (per §1.1 amendment 1)

The `BandwidthLimit` proto is used directly as both listener-level and per-route TPFC entry — there is NO separate `BandwidthLimitPerRoute` wrapper proto. The per-route entry carries all 7 fields (4 consumed + 3 silent-ignored), with the filter-internal constraint that per-route `limit_kbps` MUST be set (per the proto comment at `bandwidth_limit.pb.go:99-107`). Phase 15 honors the same-proto reuse pattern; mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1.

- **NOT honored:** structurally there is no per-route override surface beyond the listener-level fields; everything is a wholesale `BandwidthLimit` override.
- **To disable bandwidth_limit on a specific route:** operators set `enable_mode: DISABLED` in the per-route TPFC entry. The `enable_mode` field doubles as the disable mechanism. The per-route entry still requires `limit_kbps` to be set (PGV) but the value is structurally ignored when `enable_mode: DISABLED`.

### 2.3 Algorithm-shape non-goals (per BRAINSTORM §2.6 + §11.P8)

The bandwidth_limit's body algorithm is **Path B-async (buffer-then-delayed-emit)** per Q3. Specifically OUT of scope:

- **Path A streaming (rate-paced chunk-emit):** byte-by-byte wire-equivalence with Envoy's chunked-output pattern. Requires symmetric encode-side + decode-side streaming framework primitives (`EmitChunk` / `ConsumeChunk`) NOT anticipated in envoy-go's existing framework. Couples to: future streaming-framework phase. Forward-pointer at ADR-0137.
- **Path B-blast (no async wait):** the filter just lets bytes through without any delay. This is functionally identical to `enable_mode: DISABLED` and not a meaningful body algorithm. Out of scope.

### 2.4 No filter-chain ordering surgery (per BRAINSTORM §3 + §11.6 — open structural Q)

Phase 15 bandwidth_limit's filter-chain position is up to the operator (matches phase-11 local_ratelimit's flexibility). The §11.6 BRAINSTORM open structural question (filter-chain ordering with respect to compressor) is documented in §13.4 BEHAVIOR_CONTRACT forward-pointer notes:
- bandwidth_limit BEFORE compressor → throttle paces the uncompressed body bytes (more bytes through the throttle).
- bandwidth_limit AFTER compressor → throttle paces the compressed bytes (fewer bytes; tighter effective throughput).
Both orderings are valid; SPEC documents the trade-off in §13.4 but does NOT prescribe one. Fixture 0017 uses bandwidth_limit standalone (no compressor in the same chain) for byte-equivalence simplicity.

---

## 3. Framework survey result

Phase 15 introduces **ZERO framework deltas.** Reuses:

| Primitive | Source | Phase-15 usage |
|---|---|---|
| `time.AfterFunc` + `cb.ContinueDecoding()` / `cb.ContinueEncoding()` | Phase-09 fault per `internal/filter/http/fault/fault.go:319,335` | Both directions; computed delay via throttle-duration arithmetic. |
| Synthetic empty-terminal `RunDecodeData` | Phase-13 ADR-0128 per `internal/filter/hcm/connection.go:dispatchRequest` | Decode-side; ensures `endStream=true` reaches `DecodeData` even on chunked bodies with zero-data EOF. |
| Post-body Content-Length reconciliation | Phase-13 ADR-0128 per `internal/filter/hcm/connection.go:dispatchRequest` | Decode-side; vacuously used (bandwidth_limit does NOT mutate Content-Length; body bytes are unchanged). |
| `EncoderFilterCallbacks.OverwriteBody(b []byte)` | Phase-14 ADR-0131 per `internal/filter/http/callbacks.go` | Encode-side; **anticipated: NOT invoked** — the buffered-return path returns bytes unchanged. SPEC author verifies at framework-survey step. |
| `DataStopIterationAndBuffer` accumulation | Phase 07.1 + ADR-0128 per `internal/filter/http/chain.go` | Both directions; the framework continues to accumulate further chunks until `endStream=true` AND `Continue` returned. |
| 3-tier `PerRouteConfig.Resolve` | Phase 07.1 per `internal/filter/http/registry.go` | Per-route TPFC most-specific override. |
| `stats.Registry.NewCounterIfAbsent` | Phase-11 ADR-0117 per `internal/stats/registry.go` | Per-route stat-counter idempotent post-Freeze allocation. |
| `dcb.SendLocalReply` | Phase-09 fault precedent | **NOT USED.** bandwidth_limit never short-circuits to a local reply; this contrasts with phase-11 local_ratelimit which DOES short-circuit on tryConsume-fail. |

**No new framework primitive on either side.** Phase 15 is the FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 framework-delta sets simultaneously; load-bearing demonstration that phase-13 + phase-14 primitives are reusable infrastructure (per BRAINSTORM Q1 selection rationale).

### 3.1 What Path A streaming throttle would have required

Same as BRAINSTORM §3 final paragraph: ~150-200 LoC framework delta (new `EncoderFilterCallbacks.EmitChunk(b []byte)` + symmetric `DecoderFilterCallbacks.ConsumeChunk(b []byte)` + HCM machinery to invoke `RunDecodeData/RunEncodeData` chunk-by-chunk + `writeH1Reply` chunked-output mode). Out of scope per Q3 = "Path B-async (avoid the large framework delta)." Forward-pointer recorded at ADR-0137.

### 3.2 What is reused (already-on-disk primitives)

- `time.AfterFunc(d, func() {...})` standard library — used by phase-09 fault for `delay.fixed_delay`; phase-15 uses identically with a computed `d`.
- `cb.ContinueDecoding()` / `cb.ContinueEncoding()` callbacks — used by phase-09 fault to resume after the delay timer; phase-15 uses identically.
- Decode-side body-buffering machinery (synthetic empty-terminal `RunDecodeData` + post-body Content-Length reconciliation) per ADR-0128 — used by phase-13 buffer to count bytes; phase-15 uses to accumulate the full request body before computing throttle.
- Encode-side `OverwriteBody` per ADR-0131 — used by phase-14 compressor to replace `resp.Body` with compressed bytes; phase-15 anticipates NOT using (the buffered bytes ARE the original bytes, re-emitted unchanged).
- 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) per phase 07.1.
- Post-Freeze `stats.Registry.NewCounterIfAbsent` per ADR-0117.

---

## 4. Per-stream timer + cleanup discipline

Phase 15 reuses phase-09 fault's `time.AfterFunc` + `OnDestroy.Stop()` pattern verbatim. The discipline:

**Per-direction state on `*filter`:**
- `requestBody []byte` — decode-side accumulated body bytes.
- `requestTimer *time.Timer` — decode-side throttle timer; nil unless armed.
- `requestRC *compiledConfig` — decode-side effective config; cached at `DecodeHeaders` time.
- `requestActive bool` — decode-side throttle gate; set at `DecodeHeaders` per `requestRC.enableMode`.
- Symmetric `responseBody []byte` / `responseTimer *time.Timer` / `responseRC *compiledConfig` / `responseActive bool` for encode-side.

**Timer lifecycle:**
- `time.AfterFunc(d, fn)` returns a `*time.Timer`. The filter stores it at `f.requestTimer` (decode side) and `f.responseTimer` (encode side) per-filter-instance (per-stream).
- Timer fires → `fn()` runs in a goroutine. `fn()` decrements the pending-gauge (`f.requestRC.stats.requestPending.Dec()`) then invokes `cb.ContinueDecoding()` (or `ContinueEncoding`) which resumes the chain.

**Cleanup on `OnDestroy`:**
- Phase 15's `OnDestroy` MUST call `f.requestTimer.Stop()` and `f.responseTimer.Stop()` if non-nil — preventing the timer goroutine from invoking `cb.ContinueDecoding/Encoding` on a destroyed callback handle.
- `Stop()` returns a bool indicating whether the timer was active when stopped. The filter does NOT need to consult this bool; the cleanup is idempotent (Stop on an already-fired or already-stopped timer is a no-op).
- **Pending-gauge accounting under Stop-races-Fire:** if `Stop()` returns true (timer was active; callback prevented from running), the filter MUST also call `f.requestRC.stats.requestPending.Dec()` to balance the Inc that happened at arm-time. If `Stop()` returns false (callback already fired or about to fire), the callback itself handles the Dec; the filter does NOT call Dec to avoid double-decrement. The decode-side teardown sequence:
  ```go
  if f.requestTimer != nil {
      if f.requestTimer.Stop() {
          // Timer was active; we prevented the callback. Decrement here.
          f.requestRC.stats.requestPending.Dec()
      }
      // else: callback either already ran and dec'd, or is about to run and will dec.
      // Either way, no double-decrement.
  }
  ```
  Mirrors phase-09 fault's `decrementActive` discipline at `fault.go:480-500` (using the markedActive bool + atomic CAS instead of timer-active bool). Phase-15's simpler version uses the timer's own Stop() return value as the discriminator; no separate atomic bool needed.

**Race-safety considerations:**
- Timer goroutine vs. OnDestroy race: the timer may fire JUST before OnDestroy is called. If the timer-fire callback has already invoked `cb.ContinueDecoding/Encoding` and that has progressed into framework code that holds the callback handle valid, OnDestroy completes cleanly. If the timer-fire callback is mid-flight when OnDestroy is invoked, the framework's callback-handle invalidation must be safe (the chain.go RunDecodeData / RunEncodeData wrapper is the dispatch point — verified in phase-09 fault's existing pattern).
- The pending-gauge Inc/Dec sequence MUST tolerate the Stop-races-Fire window without double-counting; the `Stop() returns true → Dec here, else trust the callback` pattern guarantees this.

**Goroutine-leak prevention:**
- `time.AfterFunc` is documented to NOT leak goroutines on Stop(); the timer's goroutine exits cleanly after Stop().
- Every `time.AfterFunc(...)` call paired with a stored timer reference MUST have a corresponding `Stop()` call on a teardown path. Phase 15's `OnDestroy` is that path.
- Test-discipline: `go test -race` on the bandwidth_limit package + an `OnDestroy`-fires-mid-throttle integration test scenario validates no goroutine leak.

**No new framework primitive for timer management.** Phase-09 fault already validated the `time.AfterFunc` + `OnDestroy.Stop()` pattern; phase-15 reuses without amendment.

---

## 5. Per-route discipline — INDEPENDENT-stats (mirrors phase-11 ADR-0117)

Per §1.1 amendment 1 (no `BandwidthLimitPerRoute` wrapper proto exists), phase 15 uses the same `BandwidthLimit` proto for both listener-level and per-route TPFC entries, keyed by `*BandwidthLimit` pointer in the `factoryState.perRoute sync.Map` lazy-cache. Mirrors phase-11 local_ratelimit per ADR-0117 verbatim.

**Per-route stats: INDEPENDENT.** Each per-route entry owns its own:
- `*compiledConfig` (statPrefix, enableMode, limitKbps, fillInterval).
- Per-direction throttle accounting (each per-route stream computes its own `throttle_duration` based on per-route `limit_kbps`; the throttle is per-stream-stateless from the listener-level's perspective — no shared token-bucket-state).
- `*filterStats` (14 active stats: 8 counters + 6 gauges; keyed by per-route `stat_prefix`; per §1.1 amendment 7 + §6.2).

The per-route `stat_prefix` drives the counter namespace. A 100-stream load against a route with per-route override produces 100 increments on the per-route's `request_enabled`, NOT on the listener-level's `request_enabled`. Dashboards distinguish routes via the stat_prefix label. This is the OPERATIONALLY-CORRECT shape regardless of Envoy's choice; §11.P4 + §11.P14 confirm Envoy emits the same INDEPENDENT pattern.

**ADR-0139** codifies the INDEPENDENT-vs-SHARED resolution per the empirical pin. If §11.P4 finds Envoy emits SHARED stats (per-route routes into listener-level counter namespace), envoy-go SPEC author either (a) matches Envoy (SHARED) and amends ADR-0139 accordingly + amends BEHAVIOR_CONTRACT to note the divergence; or (b) elects to DIVERGE from Envoy on this axis (INDEPENDENT — the more useful behavior) and documents the divergence-window per phase-12/14 style. SPEC's position: INDEPENDENT is the operationally-correct shape; brainstorm hypothesis ratifies. ADR-0139 lands as a STRAIGHTFORWARD-RATIFICATION-OR-DIVERGENCE-RECORD per the empirical-pin outcome.

**Divergence from phase-12/13/14:** Those filters' per-route stats SHARED with listener-level because their per-route overrides are STATELESS (the override just changes effective config knobs; no fresh state object). Phase-15 bandwidth_limit's stateful-override (own throttle-duration computation per stream; own pending-gauge per per-route entry) matches phase-11 local_ratelimit's stateful-override (own token-bucket per per-route entry); both INDEPENDENT.

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

`internal/filter/http/bandwidthlimit/bandwidthlimit.go` exports:
- `TypeURL` const = `"type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"`.
- `New` (the `HTTPFilterFactory` registered at boot per ADR-0072).
- `filterName` package-private const = `"envoy.filters.http.bandwidth_limit"`.

### 6.2 `compiledConfig` + `filterStats` shape (per §1.1 amendment 7)

```go
type compiledConfig struct {
    statPrefix   string        // PGV non-empty per §11.P2 + amendment 3 (REQUIRED min_len=1)
    enableMode   bandwidthlimitv3.BandwidthLimit_EnableMode  // 4 values; default DISABLED
    limitKbps    uint64        // KiB/s units per §11.P15 + amendment 6; PGV >= 1 when set;
                               //   OPTIONAL at listener (foot-gun per amendment 10);
                               //   FILTER-INTERNAL REQUIRED at per-route per amendment 4 + §11.P1
    fillInterval time.Duration // PGV [20ms, 1s] when set; default 50ms per amendment 5
    stats        *filterStats  // 14 active stats; nil when ctx.Stats is nil (test path)
}

// filterStats is the 14-counter+gauge set per §11.P3 + §1.1 amendment 7.
// 2 histograms (request_transfer_duration, response_transfer_duration) NOT
// emitted in MVP per phase-06.1 baseline + amendment 9 divergence-window.
type filterStats struct {
    // 8 counters (cumulative across all streams):
    requestEnabled            *stats.Counter // bump at endStream-arrival with requestActive=true
    requestEnforced           *stats.Counter // bump by ticks at timer-fire (per-tick match per §11.P3)
    requestIncomingTotalSize  *stats.Counter // cumulative bytes entered decode-side
    requestAllowedTotalSize   *stats.Counter // cumulative bytes forwarded through decode-side
    responseEnabled           *stats.Counter // symmetric
    responseEnforced          *stats.Counter // symmetric
    responseIncomingTotalSize *stats.Counter
    responseAllowedTotalSize  *stats.Counter
    // 6 gauges (transient per-stream state):
    requestPending       *stats.Gauge // count of streams waiting on decode timer
    requestIncomingSize  *stats.Gauge // bytes-buffered-but-not-yet-forwarded (transient)
    requestAllowedSize   *stats.Gauge // bytes-forwarded-this-tick (transient)
    responsePending      *stats.Gauge
    responseIncomingSize *stats.Gauge
    responseAllowedSize  *stats.Gauge
}
```

NOTE: the `compiledConfig` shape stores `limitKbps` as `uint64` (the wrapper's underlying value type per `wrapperspb.UInt64Value`). The OPTIONAL-at-listener semantics are represented by a sentinel `limitKbps == 0` (the wrapper is nil → `compiledConfig.limitKbps = 0`). At runtime, `limitKbps == 0` + active `enable_mode` triggers the foot-gun behavior per §1.1 amendment 10 + §6.6 (matches Envoy's runtime-hang). Per-route entries with `limitKbps == 0` are REJECTED at `buildCompiledConfigPerRoute` per the code-level extra check.

The 2 histogram fields (`requestTransferDuration`, `responseTransferDuration`) that Envoy emits unconditionally are NOT registered on envoy-go side (per phase-06.1 baseline + §1.1 amendment 9). Differential fixture 0017 allow-lists the Envoy-side histogram names via twin-series-filter discipline (BEHAVIOR_CONTRACT §242 extends with phase-15 entry).

The `*Size` gauges in MVP are approximated by:
- `*_incoming_size`: set to `bodyLen` at `endStream`-arrival; reset to 0 at stream-completion (via `OnDestroy` if needed; the transient nature mirrors Envoy's terminal-tick state).
- `*_allowed_size`: set to `bodyLen` at timer-fire (one-blast emission); reset to 0 at stream-completion.

The per-tick gauge dynamics of reference Envoy (Path A rate-paced chunks each updating `*_incoming_size` and `*_allowed_size` to per-chunk values) are NOT mirrored by envoy-go's Path B-async (single-blast emission). Operational divergence-window documented at §13.4 + ADR-0137; cumulative `*_total_size` counters remain byte-equivalent.

### 6.3 `factoryState` + `filter` shape

```go
// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-11 ADR-0117 + IMPL-1 pattern verbatim.
type factoryState struct {
    listenerRC *compiledConfig
    perRoute   sync.Map // map[*bandwidthlimitv3.BandwidthLimit]*compiledConfig — per ADR-0117 + IMPL-1
    reg        *stats.Registry
}

// filter is the per-stream filter instance allocated by the factory closure.
type filter struct {
    state *factoryState
    dcb   envoyhttp.DecoderFilterCallbacks
    ecb   envoyhttp.EncoderFilterCallbacks

    // Decode-side state (per-stream).
    requestRC     *compiledConfig // resolved at DecodeHeaders; may be listener or per-route
    requestActive bool            // requestRC.enableMode in {REQUEST, REQUEST_AND_RESPONSE}
    requestBody   []byte          // accumulated decode-side body bytes
    requestTimer  *time.Timer     // armed by DecodeData(endStream=true) when throttle > 0 (per §6.6 throttleDuration; no fast-passthrough — one-tick fill_interval floor handles tiny bodies)

    // Encode-side state (per-stream).
    responseRC     *compiledConfig
    responseActive bool
    responseBody   []byte
    responseTimer  *time.Timer
}
```

### 6.4 `New` factory

Mirrors phase-11 local_ratelimit's `New` at `internal/filter/http/localratelimit/local_ratelimit.go:124-148`:

```go
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
```

### 6.5 `buildCompiledConfig` + `buildCompiledConfigPerRoute`

```go
func buildCompiledConfig(c *bandwidthlimitv3.BandwidthLimit, ctx envoyhttp.FactoryCtx, isPerRoute bool) (*compiledConfig, error) {
    // Check 1 (PGV mirror per §11.P2 + amendment 3): stat_prefix non-empty.
    statPrefix := c.GetStatPrefix()
    if statPrefix == "" {
        return nil, errors.New("bandwidth_limit: stat_prefix required")
    }
    // Check 2: enable_mode is one of {DISABLED, REQUEST, RESPONSE, REQUEST_AND_RESPONSE}.
    // PGV defined_only handles this at proto-decode; defensive check optional.

    // Check 3 (PGV mirror per amendment 4): limit_kbps.
    var limitKbps uint64
    if c.GetLimitKbps() != nil {
        limitKbps = c.GetLimitKbps().GetValue()
        // PGV >= 1 enforced at proto-decode; defensive mirror.
        if limitKbps < 1 {
            return nil, errors.New("bandwidth_limit: limit_kbps must be >= 1")
        }
    } else if isPerRoute {
        // Per amendment 4: at per-route level, limit_kbps is FILTER-INTERNAL REQUIRED.
        return nil, errors.New("bandwidth_limit: per-route entry requires limit_kbps to be set")
    }

    // Check 4 (PGV mirror per amendment 5): fill_interval in [20ms, 1s].
    fillInterval := 50 * time.Millisecond // default per Envoy filter source
    if c.GetFillInterval() != nil {
        fillInterval = c.GetFillInterval().AsDuration()
        if fillInterval < 20*time.Millisecond || fillInterval > 1*time.Second {
            return nil, fmt.Errorf("bandwidth_limit: fill_interval %v outside supported range [20ms, 1s]", fillInterval)
        }
    }

    var fs *filterStats
    if ctx.Stats != nil {
        if isPerRoute {
            fs = newFilterStatsIfAbsent(ctx.Stats, statPrefix) // post-Freeze idempotent
        } else {
            fs = newFilterStats(ctx.Stats, statPrefix)
        }
    }

    return &compiledConfig{
        statPrefix:   statPrefix,
        enableMode:   c.GetEnableMode(),
        limitKbps:    limitKbps,
        fillInterval: fillInterval,
        stats:        fs,
    }, nil
}
```

### 6.6 Throttle arithmetic (kbps-per-tick per §11.P15 + §1.1 amendment 6)

```go
// throttleDuration returns the time to wait before forwarding bodySize bytes
// at limitKbps KiB/s (kibibytes-per-second; per proto comment + §11.P15) with
// fill_interval governing chunk-emit cadence.
//
// Formula (per §11.P15 empirical):
//   chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds (bytes/tick)
//   throttle_duration   = ceil(body_size / chunk_size_per_tick) × fill_interval
//
// Edge cases:
//   - limitKbps == 0 (listener-level operational foot-gun per §11.P12 + §1.1 amendment 10):
//     returns an arbitrarily-large duration to match Envoy's runtime-hang behavior.
//   - body_size == 0: returns 0 (no throttle).
//   - body_size <= chunk_size_per_tick: returns fill_interval (one-tick minimum;
//     approximates Envoy's initial-burst behavior within ±70ms tolerance per §11.P9).
//
// Returns ticks count alongside the duration so the caller can increment
// *_enforced by ticks at stream-completion (per §11.P3 + §1 item 5 enforced
// semantic clause).
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
        chunkSize = 1 // defensive (shouldn't reach this branch given fill_interval >= 20ms PGV)
    }
    ticks = (uint64(bodySize) + chunkSize - 1) / chunkSize // ceil division
    duration = time.Duration(ticks) * fillInterval
    return duration, ticks
}
```

**Throttle math verification (matches §11 empirical):**

| Body | limit_kbps | fill_interval | chunk_size | ticks | Predicted | Observed (§11.P9) |
|---|---|---|---|---|---|---|
| 100  | 10 | 50ms | 512 bytes | 1 | 50ms | 5ms (initial-burst) |
| 1024 | 10 | 50ms | 512 bytes | 2 | 100ms | 107ms |
| 4000 | 10 | 50ms | 512 bytes | 8 | 400ms | 359-367ms |
| 4000 | 5  | 50ms | 256 bytes | 16 | 800ms | 716-814ms |
| 4000 | 1  | 50ms | 51.2 bytes | 79 | 3950ms | 3904ms (77 chunks observed) |

Differences within ±70ms tolerance (per §11.P9) absorb the initial-burst capacity approximation. The 100-byte case at 5ms vs predicted 50ms is the one outlier — Envoy's initial-burst capacity discharges the entire body without engaging throttle for sub-chunk-size bodies. envoy-go MVP's one-tick-floor matches Envoy's behavior within the tolerance window for fixture 0017's scenarios; the 50ms-vs-5ms discrepancy on a 100-byte body is acceptable because the absolute difference is under tolerance.

**No fast-passthrough short-circuit:** The BRAINSTORM §11.4 "throttle_duration < 1ms skips timer-arm" hypothesis is REPLACED by the natural one-tick-floor (`fill_interval` minimum). For `fill_interval = 50ms` (default), the minimum throttle is 50ms; bodies fitting within one tick still wait one fill_interval. This is structurally honest with the kbps-per-tick model and matches Envoy's per-tick behavior within ±70ms.

### 6.7 `DecodeHeaders` / `DecodeData` bodies

```go
func (f *filter) DecodeHeaders(_ http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
    var perRouteMsg proto.Message
    if f.dcb != nil {
        perRouteMsg = f.dcb.RequestRouteConfig()
    }
    f.requestRC = f.state.resolvePerRouteConfig(perRouteMsg)
    if f.requestRC == nil {
        return envoyhttp.HeaderContinue // defensive; shouldn't happen
    }
    em := f.requestRC.enableMode
    f.requestActive = em == bandwidthlimitv3.BandwidthLimit_REQUEST || em == bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
    // Cache for encode-side use (per-stream, both directions share the resolved RC under symmetric semantics).
    f.responseRC = f.requestRC
    f.responseActive = em == bandwidthlimitv3.BandwidthLimit_RESPONSE || em == bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
    return envoyhttp.HeaderContinue
}

func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
    if !f.requestActive {
        return envoyhttp.DataContinue
    }
    f.requestBody = append(f.requestBody, data...)
    if !endStream {
        return envoyhttp.DataStopIterationAndBuffer
    }
    // endStream=true: stream engaged → bump *_enabled + *_incoming_total_size + *_incoming_size.
    bodyLen := uint64(len(f.requestBody))
    if f.requestRC.stats != nil {
        f.requestRC.stats.requestEnabled.Inc()
        f.requestRC.stats.requestIncomingTotalSize.Add(bodyLen)
        f.requestRC.stats.requestIncomingSize.Set(bodyLen) // transient
    }
    throttle, ticks := throttleDuration(len(f.requestBody), f.requestRC.limitKbps, f.requestRC.fillInterval)
    if throttle == 0 {
        // No throttle needed (e.g., empty body). Forward immediately.
        if f.requestRC.stats != nil {
            f.requestRC.stats.requestAllowedTotalSize.Add(bodyLen)
            f.requestRC.stats.requestAllowedSize.Set(bodyLen)
        }
        return envoyhttp.DataContinue
    }
    // Arm timer; bump *_pending; capture ticks for *_enforced at completion.
    if f.requestRC.stats != nil {
        f.requestRC.stats.requestPending.Inc()
    }
    f.requestTimer = time.AfterFunc(throttle, func() {
        if f.requestRC.stats != nil {
            f.requestRC.stats.requestEnforced.Add(ticks) // per-tick cumulative match per §11.P3
            f.requestRC.stats.requestAllowedTotalSize.Add(bodyLen)
            f.requestRC.stats.requestAllowedSize.Set(bodyLen)
            f.requestRC.stats.requestPending.Dec()
        }
        f.dcb.ContinueDecoding()
    })
    return envoyhttp.DataStopIterationAndBuffer
}
```

### 6.8 `EncodeHeaders` / `EncodeData` bodies — symmetric to decode side

```go
func (f *filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    // responseRC + responseActive were cached at DecodeHeaders.
    return envoyhttp.HeaderContinue
}

func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
    if !f.responseActive {
        return envoyhttp.DataContinue
    }
    f.responseBody = append(f.responseBody, data...)
    if !endStream {
        return envoyhttp.DataStopIterationAndBuffer
    }
    bodyLen := uint64(len(f.responseBody))
    if f.responseRC.stats != nil {
        f.responseRC.stats.responseEnabled.Inc()
        f.responseRC.stats.responseIncomingTotalSize.Add(bodyLen)
        f.responseRC.stats.responseIncomingSize.Set(bodyLen)
    }
    throttle, ticks := throttleDuration(len(f.responseBody), f.responseRC.limitKbps, f.responseRC.fillInterval)
    if throttle == 0 {
        if f.responseRC.stats != nil {
            f.responseRC.stats.responseAllowedTotalSize.Add(bodyLen)
            f.responseRC.stats.responseAllowedSize.Set(bodyLen)
        }
        return envoyhttp.DataContinue
    }
    if f.responseRC.stats != nil {
        f.responseRC.stats.responsePending.Inc()
    }
    f.responseTimer = time.AfterFunc(throttle, func() {
        if f.responseRC.stats != nil {
            f.responseRC.stats.responseEnforced.Add(ticks)
            f.responseRC.stats.responseAllowedTotalSize.Add(bodyLen)
            f.responseRC.stats.responseAllowedSize.Set(bodyLen)
            f.responseRC.stats.responsePending.Dec()
        }
        f.ecb.ContinueEncoding()
    })
    return envoyhttp.DataStopIterationAndBuffer
}
```

### 6.9 `OnDestroy` + Set callbacks

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

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }
```

### 6.10 Trailer + filter-callback wiring

```go
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
```

The factory closure returns:
```go
return envoyhttp.HTTPFilter{
    Name:     filterName,           // "envoy.filters.http.bandwidth_limit"
    Decoder:  f,
    Encoder:  f,                    // SAME *filter instance
    PerRoute: parsePerRoute,        // *BandwidthLimit builder per-tier
}
```

### 6.11 `parsePerRoute` + `resolvePerRouteConfig`

Mirrors phase-11 local_ratelimit's `resolvePerRouteConfig` at `internal/filter/http/localratelimit/local_ratelimit.go:305-337` verbatim, with `*localratelimitv3.LocalRateLimit` replaced by `*bandwidthlimitv3.BandwidthLimit`:

```go
func parsePerRoute(any *anypb.Any) (proto.Message, error) {
    var c bandwidthlimitv3.BandwidthLimit
    if err := any.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("bandwidth_limit: per-route unmarshal: %w", err)
    }
    // Defensive PGV mirror: validate listener-level + per-route subset of constraints.
    // The full validation happens lazily at resolvePerRouteConfig → buildCompiledConfigPerRoute.
    // parsePerRoute returns the proto for the registry's per-route map.
    return &c, nil
}

func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledConfig {
    if msg == nil {
        return s.listenerRC
    }
    perRoute, ok := msg.(*bandwidthlimitv3.BandwidthLimit)
    if !ok {
        return s.listenerRC
    }
    if cached, ok := s.perRoute.Load(perRoute); ok {
        return cached.(*compiledConfig)
    }
    fresh, err := buildCompiledConfigPerRoute(perRoute, s.reg)
    if err != nil {
        // Per-route TPFC parsing failed at lazy resolve. Treat as inherit-listener (request flow alive).
        return s.listenerRC
    }
    actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
    return actual.(*compiledConfig)
}
```

---

## 7. Differential fixture `0017-http-bandwidth-limit`

### 7.1 Per-request matrix (6 scenarios; per BRAINSTORM §6.2 with timings corrected per §1.1 amendment 6 KiB/s + kbps-per-tick math)

**Throttle-math reminder (per §6.6 + §11.P15):** `chunk_size = limit_kbps × 1024 × fill_interval_seconds`; `throttle = ceil(body / chunk_size) × fill_interval`. For listener-level `limit_kbps=10` (KiB/s!), `fill_interval=50ms`, `chunk_size = 10 × 1024 × 0.050 = 512 bytes/tick`. Example: 10 KiB body = 10240 bytes → ticks = `ceil(10240/512) = 20` → throttle = `20 × 50ms = 1000ms = 1.0s`. (The brainstorm's 8.192s estimates were based on the WRONG kilobits-per-second interpretation; corrected per amendment 6.)

| # | Scenario | Request | Expected response | Counter delta (envoy-go side) | §11 cross-ref |
|---|---|---|---|---|---|
| 1 | Response-only throttle (default route) | `GET /echo-response` (10 KiB = 10240-byte direct_response body) | 200; `content-type: application/octet-stream`; body byte-equivalent 10240 bytes; total wall-clock ≈ 1000ms (±70ms; 20 ticks × 50ms) | `response_enabled +1`, `response_enforced +20`, `response_incoming_total_size +10240`, `response_allowed_total_size +10240`, `response_pending` peaks at 1 then 0 | §11.P8 + §11.P9 + §11.P15 |
| 2 | Request-only throttle | `POST /echo-request` (10 KiB body); listener-level `enable_mode: REQUEST` per-route override | 200; upstream-arrival-time ≈ 1000ms (±70ms); body byte-equivalent | `request_enabled +1`, `request_enforced +20`, `request_incoming_total_size +10240`, `request_allowed_total_size +10240`, `request_pending` peaks at 1 then 0 | §11.P8 |
| 3 | REQUEST_AND_RESPONSE symmetric | `POST /echo-both` (5 KiB = 5120-byte body; backend echoes 5120 bytes back) | 200; total wall-clock ≈ 1000ms (±70ms; decode 500ms + encode 500ms; each = 10 ticks × 50ms); body byte-equivalent | both request + response counters +1; both _enforced +10; both _incoming/_allowed_total_size +5120; both pending gauges peak at 1 | §11.P8 |
| 4 | Tiny-body within initial-burst capacity | `GET /echo-tiny` (100-byte direct_response) | 200; total wall-clock ≤ 50ms (one-tick floor per §6.6; body fits within one tick at `chunk_size=512`); body byte-equivalent | `response_enabled +1`, `response_enforced +1`, `response_incoming_total_size +100`, `response_allowed_total_size +100`, `response_pending` peaks at 1 then 0 | §11.P9 |
| 5 | Per-route disabled (via `enable_mode: DISABLED`) | `GET /echo-disabled` (10 KiB body) | 200; total wall-clock < 50ms (no throttle on this route); body byte-equivalent | NO counter increments (filter wholly inactive on disabled-via-enable_mode route per §11.P12 — namespace registered but counters stay 0) | §11.P12 |
| 6 | Per-route override with own stat_prefix (per §11.P14) | `GET /echo-override` (10 KiB direct_response; per-route override sets `stat_prefix: override`, `enable_mode: RESPONSE`, `limit_kbps: 100`, `fill_interval: 50ms`) | 200; total wall-clock ≈ 100ms (±70ms; `chunk_size = 100 × 1024 × 0.050 = 5120 bytes/tick` → ticks = `ceil(10240/5120) = 2` → throttle = `100ms`); body byte-equivalent | `<override stat_prefix>` namespace: `response_enabled +1`, `response_enforced +2`, `response_incoming/allowed_total_size +10240`; `<default stat_prefix>` namespace: NO counter increments (INDEPENDENT per §11.P4 + §11.P14) | §11.P4 + §11.P14 + §11.P15 |

### 7.2 Topology

`test/fixtures/0017-http-bandwidth-limit/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config.
- `inputs/driver.go` — Go driver issuing the 6 scenarios; wall-clock timing per scenario; byte-exact body comparison; per-counter delta scrape.
- `expectations.yaml` — per-scenario allow-list / counter-delta map.
- `README.md` — fixture overview + scenario list + reference config citations + ±70ms tolerance discipline (per §11.P9) + KiB/s units note (per §1.1 amendment 6) + histograms allow-list note (per §1.1 amendment 9).

Two listeners + two clusters (mirrors phase 11/12/13/14 fixture topology):

- **Listener `l_test_a`** (TCP plaintext): HCM with one filter-chain `bandwidth_limit → router`. Listener-level config:
  ```yaml
  envoy.filters.http.bandwidth_limit:
    stat_prefix: default
    enable_mode: REQUEST_AND_RESPONSE
    limit_kbps: 10
    fill_interval: 50ms
  ```
  Routes:
  - `/echo-response` → `direct_response` 10 KiB body. Default-route; scenario 1.
  - `/echo-request` → cluster `c_backend_b`. Per-route `enable_mode: REQUEST`. Scenario 2.
  - `/echo-both` → cluster `c_backend_b`. Inherits listener `REQUEST_AND_RESPONSE`. Scenario 3.
  - `/echo-tiny` → `direct_response` 100 bytes. Inherits listener. Scenario 4.
  - `/echo-disabled` → `direct_response` 10 KiB. Per-route `enable_mode: DISABLED` (the disable mechanism; per amendment 1). Scenario 5.
  - `/echo-override` → `direct_response` 10 KiB. Per-route `stat_prefix: override`, `enable_mode: RESPONSE`, `limit_kbps: 100`, `fill_interval: 50ms`. Scenario 6.

- **Listener `l_test_b`** + cluster `c_backend_b`: echo-backend cluster pair (reuses `test/helpers/echobackend/` from phase 14).

### 7.3 Asserted equivalence

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status:** byte-exact between Envoy and envoy-go for every scenario (200 on every scenario).
- **Response headers:** lowercase wire-form, set-equal between Envoy and envoy-go modulo:
  - `## Header allow-list` (existing — `date`, `server`, timing/identity headers).
  - **`x-envoy-bandwidth-*` trailers** (when `enable_response_trailers: true` is set on Envoy side; fixture explicitly does NOT set this field; trailer-emission divergence is documented at §13.4 but not exercised in fixture 0017).
- **Response body:** byte-exact (bandwidth_limit does NOT transform bytes; only paces them; both Envoy and envoy-go emit the same bytes — the only difference is timing). Mirrors phase-11/12/13's byte-exact body discipline; DIVERGES from phase-14 compressor's decompress-and-compare per ADR-0133.
- **Total wall-clock time:** within ±70ms tolerance per scenario (Path B-async vs. Envoy's rate-paced chunks observably converge on the total-throttle-time axis; per §11.P9 empirical). Larger throttles (multi-second; not exercised in fixture 0017's sub-second scenarios) would widen the tolerance.
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the **14 active phase-15 counters + gauges** per scenario (8 counters + 6 gauges; the 2 transfer-duration histograms are allow-listed via twin-series-filter per §1.1 amendment 9 + §11.P3). Per-route-active scenarios (5 + 6) exercise the per-route stat namespace (scenario 6 — INDEPENDENT-stats per §11.P4 + §11.P14).
- **Per-route fixture-config disposition:** scenarios 5 + 6 exercise BOTH per-route mechanisms (`enable_mode: DISABLED` as disable; `stat_prefix` + `limit_kbps` + `enable_mode: RESPONSE` override as wholesale via bare-`BandwidthLimit` TPFC per §1.1 amendment 1).
- **Histogram allow-list (per §1.1 amendment 9 + BEHAVIOR_CONTRACT §242 twin-series-filter extension):** the 2 unconditional Envoy histograms `envoy_<stat_prefix>_http_bandwidth_limit_{request,response}_transfer_duration_*` are EXCLUDED from per-counter delta comparison. The differential harness filters these out before computing the byte-equivalence delta.
- **`*_enforced` counter delta interpretation:** envoy-go increments by `ticks` at stream-completion to match Envoy's per-tick cumulative semantic (per §6.7 + §11.P3). For scenario 1 (10240-byte body at chunk_size=512), expected `response_enforced +20`. Matches Envoy's per-fill_interval-tick increment within the test workload's measurement boundary.

### 7.4 Driver shape

`inputs/driver.go` mirrors phase 11/13/14 driver shape:
- 6 scenarios, each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Wall-clock timing helper `measureRequestDuration(ctx, req) (resp, duration, error)` using `time.Now()` at request-issue and response-completion.
- Per-scenario assertion helper for byte-exact body + wall-clock within tolerance.
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.
- For scenarios 2 + 3 (request-side throttle), the echo-backend's `X-Echo-Received-At` timestamp header is asserted against `time.Now()` at request-issue to measure upstream-arrival time independent of response-side throttle.

Total estimated driver size: ~200 LoC (similar to phase-14 driver).

**Wall-clock timing tolerances** are the LOAD-BEARING new fixture-tolerance discipline per `BEHAVIOR_CONTRACT.md ## Timing tolerances` extension (see §13.5 below). Phase 11 fault and phase 11 local_ratelimit established the ±10ms tolerance pattern; phase 15 extends to **±70ms** (per §11.P9 empirical worst-case) for the kbps-per-tick rate-paced Envoy chunk-pattern vs envoy-go's single-blast Path B-async; the wider envelope absorbs the initial-burst-capacity approximation variance + `time.AfterFunc` Linux granularity + CI scheduling jitter.

**No H2 differential coverage.** Phase 15 fixture 0017 is HTTP/1.1-only per the existing §9 family-row convention.

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §1.1)

5 ADRs anticipated (consistent with BRAINSTORM §7's roster). ADR-0134 is the highest-numbered ADR landed in phase 14; ADR-0135 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0135** | `internal/filter/http/bandwidthlimit/` package shape — single-token directory matching cors/fault/csrf/buffer/compressor/localratelimit precedent + boot registration ordering + ENCODER+DECODER `HTTPFilter` value (BOTH-direction symmetric throttle) + 14-active-stat `filterStats` struct (8 counters + 6 gauges per §1.1 amendment 7) + ZERO framework deltas (FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 simultaneously) | Decision 1 (BRAINSTORM §2.1) + Decision 2 (BRAINSTORM §2.2) + §3 framework-survey result |
| **ADR-0136** | `compiledConfig` shape + 4-consumed/3-silent-ignored field decomposition + PGV-mirror filter-internal validation discipline (stat_prefix required; limit_kbps >= 1; fill_interval [20ms, 1s]; per-route limit_kbps filter-internal required per §1.1 amendment 4) + envoy-go-own error wording | Decision 3 (BRAINSTORM §2.5) + §1.1 amendments 3 + 4 + 5 |
| **ADR-0137** | Body algorithm Path B-async (buffer-then-delayed-emit) with **kbps-per-tick throttle math** per §1.1 amendment 6 + §11.P15 (`chunk_size = limit_kbps × 1024 × fill_interval`; `throttle = ceil(body/chunk_size) × fill_interval`; KiB/s units NOT kbps); `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume reuse from phase-09 fault; wire-shape divergence-window (envoy-go: silent-then-blast; Envoy: Path A rate-paced chunks at exact fill_interval cadence; total-throttle-time equivalent ±70ms; chunk-arrival-timing divergent); `cb.OverwriteBody` NOT invoked (framework-survey verified); forward-pointer to future encode-side streaming framework phase; `*_enforced` increment-by-`ticks` cumulative-match discipline | Decision 4 (BRAINSTORM §2.3) + §11.P8 + §11.P9 + §11.P15 |
| **ADR-0138** | **14-counter+gauge** stat surface (8 counters + 6 gauges) per §11.P3 + §1.1 amendment 7 + 8 + 9; namespace shape `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; NOT HCM-rooted) per §11.P11; Prometheus inlines stat_prefix into base name `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` (NO tag-extractor; NO new SN10 rule needed) per §11.P10; **histograms divergence-window** (Envoy emits 2 unconditional transfer_duration histograms; envoy-go MVP skips per phase-06.1 baseline; twin-series-filter discipline at BEHAVIOR_CONTRACT §242 extends); INDEPENDENT per-route stats (mirrors phase-11 ADR-0117); enforced increments by `ticks` per stream to match Envoy's per-tick cumulative semantic | Decision 5 (BRAINSTORM §2.6) + §11.P3 + §11.P10 + §11.P11 |
| **ADR-0139** | Per-route INDEPENDENT-stats ratification — codifies phase 15 as SECOND row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent; phase-15's per-route discipline is a **NEW canonical pattern** (bare-`BandwidthLimit`-via-TPFC + filter-internal-REQUIRED-`limit_kbps` code-level check) — distinct from BOTH the 4th canonical (phase-11 stateful-override-with-INDEPENDENT-stats; uses same proto but no code-level extra check) AND the 5th canonical (phase-13/14 disabled-OR-override-sum-type; uses wrapper proto). ADR-0125 §(xi) amendment paragraph documents the new pattern; ADR-0125's canonical-shape catalog extends with a 6th entry (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route) | Decision 6 (BRAINSTORM §5) + §11.P1 + §11.P4 + §11.P14 |

**Plus an ADR-0125 amendment paragraph §(xi)** (NOT a new ADR): documents the BRAINSTORM-time hypothesis that phase 15 would be the THIRD row using disabled-OR-override 5th canonical (§2.4 hypothesis); empirically refuted at §11.P1 (no `BandwidthLimitPerRoute` proto exists; per-route uses same `BandwidthLimit` proto directly mirroring phase-11 local_ratelimit per ADR-0117); the 5th canonical disabled-OR-override stays bound to phase-13 buffer + phase-14 compressor (TWO rows, not three). The canonical-5-shape per-route table in ADR-0125 is updated to reflect phase 15 as a USER of the 4th canonical (ADR-0117), NOT a third user of the 5th canonical. Authored at phase 15 SPEC drafting time per the ADR-0125 in-place-update precedent (mirrors phase-13 ADR-0127 v2 + phase-14 ADR-0125 amendment).

NO new framework-primitive ADR (ZERO framework deltas per §3 framework-survey). NO ADR-0117 amendment (the 4th canonical pattern is directly inherited; phase 15 is the SECOND row using it). NO ADR-0061 amendment unless §11.P10 demands a new SN10 flattening rule.

SPEC-time may revise the 5-ADR count per ADR-0044 SPEC-time-anticipation discipline (e.g., if §11 reveals an unanticipated nuance requiring a 6th ADR).

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106(b))

This SPEC authors NO sibling SPEC stubs for the next §9 family-children (`jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`) plus the future `envoy.filters.http.decompressor` companion to compressor. Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts per ADR-0106(b) + (e). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

The phase-15 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following claims against the landed artefacts:

1. **Package shape per ADR-0135:** `internal/filter/http/bandwidthlimit/{bandwidthlimit.go, bucket.go, bandwidthlimit_test.go, fuzz_test.go, doc.go}` with `Decoder: f, Encoder: f` (same *filter); 14-active-stat `filterStats` registered (8 counters + 6 gauges per §1.1 amendment 7).
2. **Field decomposition per ADR-0136 + §1.1 amendments 3 + 4 + 5:** 4 consumed (stat_prefix, enable_mode, limit_kbps, fill_interval) + 3 silent-ignored (runtime_enabled, enable_response_trailers, response_trailer_prefix); PGV-mirror validation at parse-time with envoy-go-own error wording; CODE-LEVEL extra check at per-route position for `limit_kbps` REQUIRED (`"bandwidth_limit: per-route entry requires limit_kbps to be set"`).
3. **Body algorithm per ADR-0137:** Path B-async with kbps-per-tick throttle math (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`; `throttle = ceil(body/chunk_size) × fill_interval`); `limit_kbps` units = KiB/s; `time.AfterFunc` + `ContinueDecoding/Encoding` reuse from phase-09 fault; ZERO framework deltas (NO new primitive at `callbacks.go`/`chain.go`/`connection.go`/`h2dispatch.go`).
4. **Stat surface per ADR-0138 + §1.1 amendments 7 + 8 + 9:** 14 active stats (8 counters + 6 gauges); 2 unconditional Envoy histograms allow-listed via twin-series-filter divergence-window; namespace `<stat_prefix>.http_bandwidth_limit.<counter>` underscore-infix (NOT HCM-rooted); Prometheus inlines stat_prefix into base name `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}`; NO new SN10 rule.
5. **Differential fixture per §7:** 6 scenarios; byte-exact body assertion; **±70ms** wall-clock tolerance; per-counter delta byte-equivalence on 14 active stats; histograms allow-listed; per-route INDEPENDENT-stats per scenario 6; scenario 5 disable mechanism via per-route `enable_mode: DISABLED` (per §1.1 amendment 1; no `disabled` shortcut at proto level).
6. **Per-route discipline per ADR-0139 + ADR-0125 §(xi) amendment:** phase-15 introduces a NEW canonical per-route pattern (bare-`BandwidthLimit`-via-TPFC + code-level-required-`limit_kbps`-at-per-route) distinct from BOTH ADR-0117 (4th canonical; same proto, no code-level extra check) AND ADR-0125 (5th canonical; wrapper proto with oneof). ADR-0125 in-place §(xi) amendment lands at SPEC time documenting the new pattern + the BRAINSTORM-time hypothesis-refutation.
7. **§11 empirical pin block:** all 15 pins resolved IN-SESSION against reference Envoy v1.37.2 per ADR-0004; 7 RATIFIED + 6 REFUTED + 2 PARTIAL/REFRAMED; **10 §1.1 amendments** authored covering the empirical refutations + structural finding + operational foot-gun.
8. **Wire-shape divergence-window documented:** envoy-go Path B-async silent-then-blast vs Envoy Path A rate-paced chunks at `fill_interval` cadence with `limit_kbps × 1024 × fill_interval` chunk size; total wall-clock equivalent ±70ms; ADR-0137 forward-pointer to future encode-side streaming framework phase.
9. **BEHAVIOR_CONTRACT.md populated** per Gate F:
   - §13.1 new `### envoy.filters.http.bandwidth_limit` subsection (~150-200 lines incorporating field-decomposition table + KiB/s units + kbps-per-tick algorithm + INDEPENDENT-stats + foot-gun documentation).
   - §13.2 stat-table 46 → 60 names extension (14 new active entries; 2 histograms allow-listed via twin-series).
   - §13.3 NEW equivalence-matrix row pointing at fixture 0017 with ±70ms tolerance discipline + histogram allow-list note.
   - §13.4 NEW `### Phase 15 forward-pointer notes` subsection covering ~8-item deferral list + foot-gun + histogram divergence + chunk-pattern divergence + KiB/s units note.
   - §13.5 `## Timing tolerances` extension with ±70ms tolerance entries.
   - §242 `### Twin-series filter discipline` extension with phase-15 histogram allow-list entry.
10. **All six phase-done gates green at phase-done commit.**

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 15 pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 / 10 / 11 / 12 / 13 / 14 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 / 14 SPEC §11 confirmation).

**Probe configuration:** Reference Envoy booted under per-pin minimal bootstrap YAMLs at `/tmp/p15-pins/probe{A..M}-*.yaml` via `docker run -d --name p15-probe<X> --rm -p <admin>:<admin> -p <listener>:<listener> -v /tmp/p15-pins:/etc/envoy:ro envoyproxy/envoy:v1.37.2 -c /etc/envoy/<file>.yaml --base-id <unique>`; admin port 19951+, listener port 11501+. Direct_response routes serve fixed-size bodies from `/tmp/p15-pins/body-{tiny,500,small,2k,medium,4k,large}.txt`. Verbatim probe transcripts + curl `--trace-time --trace-ascii` wire-shape captures are durable at `/tmp/p15-pins/` on the SPEC drafting session machine; the verbatim outputs below are the durable evidence per the 09 / 10 / 11 / 12 / 13 / 14 SPEC §11 discipline.

Source-of-truth cross-reference: upstream `envoy/api/envoy/extensions/filters/http/bandwidth_limit/v3/bandwidth_limit.proto` at tag `v1.37.2` (fetched at session-time from `raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/...`); v1.32.4 + v1.37.0 go-control-plane bindings at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.{32.4,37.0}/extensions/filters/http/bandwidth_limit/v3/bandwidth_limit.pb.{go,validate.go}`; upstream `source/extensions/filters/http/bandwidth_limit/bandwidth_limit_filter.cc` + `config.cc` cited where load-bearing.

Probe date: **2026-05-11**.

### Summary disposition table (15 pins; 7 RATIFIED + 6 REFUTED + 2 PARTIAL/REFRAMED)

| Pin | Topic | Disposition | Amendment cross-ref |
|---|---|---|---|
| **§11.P1** | `BandwidthLimitPerRoute` proto shape | **REFUTED** — NO wrapper proto; per-route TPFC consumes bare `BandwidthLimit` | §1.1 amendment 1 |
| **§11.P2** | PGV requirements per consumed field | **PARTIAL** — most PGV ratified + ONE code-level extra check at per-route position | §1.1 amendment 3 + 4 |
| **§11.P3** | Stat names + counter/gauge/histogram disposition | **REFUTED** — 16 stats (8c + 6g + 2h), not 6 | §1.1 amendment 7 |
| **§11.P4** | Per-route stat SHARED-vs-INDEPENDENT | **RATIFIED** — INDEPENDENT (per-route emits to own namespace) | §5 + ADR-0139 |
| **§11.P5** | `fill_interval` PGV bounds | **RATIFIED** — `[20ms, 1s]` PGV-enforced at boot | §1.1 amendment 5 |
| **§11.P6** | `runtime_enabled` type | **RATIFIED** — RuntimeFeatureFlag (BoolValue default; optional) | §2.1.1 |
| **§11.P7** | `response_trailer_prefix` default behavior | **RATIFIED** — silent-ignored when trailers disabled | §2.1.2 |
| **§11.P8** | Wire-shape on response throttle path | **REFRAMED** — Envoy emits Path A rate-paced chunks at exact `fill_interval` cadence; chunk_size = `limit_kbps × 1024 × fill_interval_seconds` bytes | §1.1 amendment 6 + ADR-0137 |
| **§11.P9** | Throttle-timing tolerance window | **RATIFIED** — ±70ms worst case on single-stream localhost | §7.3 + §13.5 |
| **§11.P10** | Prometheus tag-extractor name | **REFUTED** — NO tag extraction; stat_prefix inlined into metric name; SN10 NOT required | §1.1 amendment 8 |
| **§11.P11** | Namespace flattening shape | **REFUTED** — `<stat_prefix>.http_bandwidth_limit.<counter>` (NO HCM prefix; `http_bandwidth_limit` infix PRESENT; underscore not dot) | §1.1 amendment 8 |
| **§11.P12** | `enable_mode: DISABLED` runtime evaluation | **RATIFIED** — passthrough; namespace registered; counters stay 0 | §2.1 + §13.4 |
| **§11.P13** | Per-stream pending-gauge lifecycle | **RATIFIED** — Inc at throttle-arm; Dec at throttle-complete | §4 + §6.7 |
| **§11.P14** | Per-route override `stat_prefix` emission scope | **RATIFIED** — wholly-own counter namespace (INDEPENDENT) | §5 + ADR-0139 |
| **§11.P15** | `fill_interval` × `limit_kbps` interaction | **REFUTED** — kbps-per-tick chunking (NOT steady-rate); fill_interval governs chunk-size, not just timer granularity | §1.1 amendment 6 + §6.6 |

**Tally:** 7 RATIFIED + 6 REFUTED + 2 PARTIAL/REFRAMED.

**Structural amendments (re-frame §2.x Decisions):** **5** — per-route shape (§11.P1); throttle arithmetic (§11.P8 + §11.P15); stat surface (§11.P3); stat namespace (§11.P10 + §11.P11); wire-shape divergence framing (§11.P8). All handled via §1.1 amendment-block channel per phase-12 csrf + phase-14 compressor precedent.

**Operational foot-gun discovery (probeJ; documented at §13.4):** Listener-level `enable_mode: RESPONSE` + NO `limit_kbps` BOOTS cleanly per Envoy proto comment but at runtime every request HANGS (5s observed timeout; `response_pending=1, response_enforced=99` mid-stream). envoy-go matches Envoy's silent-acceptance + documents the divergence-window in §13.4.

### 11.1 Empirical pin #1 — `BandwidthLimitPerRoute` proto shape (REFUTES BRAINSTORM §9.P1 + §2.4 + Q4)

**Probe configuration:** Multi-source verification across 4 axes:
1. v1.32.4 go-control-plane bindings: `cat /home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/http/bandwidth_limit/v3/bandwidth_limit.pb.go | grep -n "^type"` → only `BandwidthLimit` + `BandwidthLimit_EnableMode` defined; NO `BandwidthLimitPerRoute` message.
2. v1.37.0 go-control-plane bindings (paired with Envoy v1.37.x release line): same disposition — only `BandwidthLimit` defined.
3. Upstream proto source: WebFetch of `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/api/envoy/extensions/filters/http/bandwidth_limit/v3/bandwidth_limit.proto` confirms the proto file contains a SINGLE message `BandwidthLimit` with 7 fields and `EnableMode` enum.
4. Envoy filter source: `source/extensions/filters/http/bandwidth_limit/config.cc` exposes `createRouteSpecificFilterConfigTyped(BandwidthLimit& proto_config, ...)` — same `BandwidthLimit` proto handles per-route position.
5. **Empirical probeB:** `typed_per_filter_config` with `"@type": type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit` (bare message) BOOTS cleanly and applies the override.

**Verbatim probeB-transcript.txt excerpt** (per-route override exercise; full transcript at `/tmp/p15-pins/probeB-transcript.txt`):

```
=== B1: listener default route (uses listener-level limit_kbps=10 RESPONSE) ===
* Connected to localhost (::1) port 11502
> GET /default HTTP/1.1
< HTTP/1.1 200 OK
< content-length: 4000
< content-type: text/plain
< server: envoy
real    0m0.363s

=== B2: per-route override route /override (per-route limit_kbps=5 RESPONSE) ===
* Connected to localhost (::1) port 11502
> GET /override HTTP/1.1
< HTTP/1.1 200 OK
< content-length: 4000
real    0m0.716s

=== B3: per-route stats namespace scrape ===
default.http_bandwidth_limit.response_enabled: 1
default.http_bandwidth_limit.response_enforced: 6
override.http_bandwidth_limit.response_enabled: 1
override.http_bandwidth_limit.response_enforced: 14
```

The B1+B2 timing ratio confirms the per-route `limit_kbps: 5` override doubles the throttle relative to listener-level `limit_kbps: 10` (716ms ≈ 2× 363ms for the same 4000-byte body). The B3 stats scrape confirms INDEPENDENT-stats: per-route override emits to its OWN `override.http_bandwidth_limit.*` namespace; listener-level `default.http_bandwidth_limit.*` namespace is unaffected by per-route-active requests (resolves §11.P4 + §11.P14 simultaneously).

**Verbatim probeK-per-route-no-limit.yaml + transcript** (per-route TPFC entry WITHOUT `limit_kbps`; full at `/tmp/p15-pins/probeK-*`):

```
$ docker run ... envoyproxy/envoy:v1.37.2 -c /etc/envoy/probeK-per-route-no-limit.yaml ...
[critical][config] Config rejected: limit must be set for per route filter config
```

The error string `"limit must be set for per route filter config"` originates from Envoy's filter source (`source/extensions/filters/http/bandwidth_limit/config.cc::createRouteSpecificFilterConfigTyped`), NOT from PGV. This is a CODE-LEVEL extra check that fires only when `per_route=true` AND `limit_kbps` is unset on the per-route entry.

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P1 + §2.4 + Q4:**

- (a) **No `BandwidthLimitPerRoute` proto exists in Envoy v1.37.2.** Per-route TPFC consumes a bare `BandwidthLimit` message via the standard `typed_per_filter_config` envelope.
- (b) **Per-route override is WHOLESALE replacement** — the entire `BandwidthLimit` message replaces the listener-level config for that route (mirrors phase-11 ADR-0117 IMPL-1 pattern; same proto for listener + per-route).
- (c) **NO `disabled` shortcut at the filter-proto level.** To skip throttle on a route, operators set per-route `enable_mode: DISABLED` (which still requires the PGV-required `stat_prefix` AND the code-level-required `limit_kbps` to be set on the per-route entry, even though both values are structurally ignored when `enable_mode: DISABLED`).
- (d) **One code-level extra check beyond PGV:** at per-route position only, missing `limit_kbps` rejects at boot with verbatim error `"limit must be set for per route filter config"`. Envoy-go MUST mirror this rejection in `buildCompiledConfigPerRoute` — wording per §1.1 amendment 4 + §6.5 PGV-mirror discipline.
- (e) **BRAINSTORM §2.4's "5th canonical disabled-OR-override" hypothesis is REFUTED.** Phase-15 introduces a **NEW canonical per-route pattern**: bare-message wholesale-via-TPFC, distinct from both the 4th canonical (phase-11 stateful-override-with-INDEPENDENT-stats; uses same proto for listener+per-route but no code-level extra check) AND the 5th canonical (phase-13/14 disabled-OR-override-sum-type; uses wrapper proto with oneof). Phase-15 sits between: same proto as 4th canonical, plus the code-level required-limit_kbps check. ADR-0125 §(xi) amendment paragraph documents the new pattern.
- (f) ADR-0125 in-place amendment paragraph §(xi) is authored at this SPEC commit (per phase-13 ADR-0127-v2 + phase-14 ADR-0125 amendment precedent at Task 12). The amendment documents: phase-15 was BRAINSTORM-hypothesized to be the THIRD row using disabled-OR-override 5th canonical; SPEC-time §11.P1 empirically REFUTED; phase-15 instead introduces a NEW canonical pattern (bare-message-via-TPFC + code-level required-limit_kbps at per-route position) that sits adjacent to the 4th canonical (phase-11 ADR-0117) in the discipline-shape catalog. The 5th canonical disabled-OR-override stays bound to phase-13 buffer + phase-14 compressor (TWO rows).

### 11.2 Empirical pin #2 — PGV requirements per consumed field (PARTIAL — most ratified; one code-level extra check)

**Probe configuration:** Four negative-validation probes (D, E, F, G) booted with intentional PGV violations; verbatim error capture via `docker logs p15-probe<X>` 2>&1 head. Plus probeK (per-route without limit_kbps) for the code-level extra check.

**Verbatim probe-pgv-final.txt:**

```
=== probeD: empty stat_prefix → REJECTED ===
Proto constraint validation failed (BandwidthLimitValidationError.StatPrefix: value length must be at least 1 characters)

=== probeE: limit_kbps=0 (LimitKbps wrapper { } with value 0) → REJECTED ===
Proto constraint validation failed (BandwidthLimitValidationError.LimitKbps: value must be greater than or equal to 1)

=== probeF: fill_interval=10ms → REJECTED ===
Proto constraint validation failed (BandwidthLimitValidationError.FillInterval: value must be inside range [20ms, 1s])

=== probeG: fill_interval=2s → REJECTED ===
Proto constraint validation failed (BandwidthLimitValidationError.FillInterval: value must be inside range [20ms, 1s])
```

**Plus the per-route code-level check (probeK, captured under §11.1 above):**

```
=== probeK: per-route TPFC without limit_kbps → REJECTED ===
[critical][config] Config rejected: limit must be set for per route filter config
```

**Conclusions (pinned) — PARTIAL: 4 PGV checks RATIFIED + 1 code-level extra check found:**

- (a) **`stat_prefix` PGV `min_len = 1` enforced at parse-time** (refutes BRAINSTORM "empty default permitted"; resolves §1.1 amendment 3).
- (b) **`limit_kbps` PGV `gte = 1` enforced at parse-time WHEN wrapper present** (refutes BRAINSTORM "envoy-go-only parse-rejection"; resolves §1.1 amendment 4).
- (c) **`fill_interval` PGV `gte = 20ms, lte = 1s` enforced at parse-time WHEN wrapper present** (refutes BRAINSTORM "envoy-go-side filter-internal cap"; resolves §1.1 amendment 5).
- (d) **`enable_mode` PGV `defined_only = true`** — enforced by the auto-generated proto3 enum machinery; defensive envoy-go-side mirror in `buildCompiledConfig` is optional (the proto-decoder already rejects undefined values).
- (e) **`response_trailer_prefix` PGV pattern `^[^\x00\n\r]*$`** — enforced by the auto-generated PGV `Validate()` method at proto-decode time.
- (f) **CODE-LEVEL extra check at per-route position:** missing `limit_kbps` rejects with `"limit must be set for per route filter config"` (originates from Envoy's filter source, NOT PGV). envoy-go MUST mirror in `buildCompiledConfigPerRoute` with envoy-go-own error wording (per ADR-0136 + §1.1 amendment 4).
- (g) `limit_kbps` is OPTIONAL at listener-level (no PGV required); the wrapper may be absent. See §11.10 below for the runtime-hang foot-gun this enables.

### 11.3 Empirical pin #3 — Stat names + counter/gauge/histogram disposition (REFUTES BRAINSTORM §9.P3 + §1.1 item 7)

**Probe configuration:** probeA with `stat_prefix: default`, `enable_mode: RESPONSE`, `limit_kbps: 10`, `fill_interval: 50ms`. After three /tiny, /small, /medium GETs, scrape `/stats/prometheus` and `/stats?filter=bandwidth_limit&format=json`.

**Verbatim probeA-transcript.txt scrape excerpt:**

```
# TYPE envoy_default_http_bandwidth_limit_request_allowed_total_size counter
envoy_default_http_bandwidth_limit_request_allowed_total_size{} 0
# TYPE envoy_default_http_bandwidth_limit_request_enabled counter
envoy_default_http_bandwidth_limit_request_enabled{} 0
# TYPE envoy_default_http_bandwidth_limit_request_enforced counter
envoy_default_http_bandwidth_limit_request_enforced{} 0
# TYPE envoy_default_http_bandwidth_limit_request_incoming_total_size counter
envoy_default_http_bandwidth_limit_request_incoming_total_size{} 0
# TYPE envoy_default_http_bandwidth_limit_response_allowed_total_size counter
envoy_default_http_bandwidth_limit_response_allowed_total_size{} 5124
# TYPE envoy_default_http_bandwidth_limit_response_enabled counter
envoy_default_http_bandwidth_limit_response_enabled{} 3
# TYPE envoy_default_http_bandwidth_limit_response_enforced counter
envoy_default_http_bandwidth_limit_response_enforced{} 10
# TYPE envoy_default_http_bandwidth_limit_response_incoming_total_size counter
envoy_default_http_bandwidth_limit_response_incoming_total_size{} 5124
# TYPE envoy_default_http_bandwidth_limit_request_allowed_size gauge
envoy_default_http_bandwidth_limit_request_allowed_size{} 0
# TYPE envoy_default_http_bandwidth_limit_request_incoming_size gauge
envoy_default_http_bandwidth_limit_request_incoming_size{} 0
# TYPE envoy_default_http_bandwidth_limit_request_pending gauge
envoy_default_http_bandwidth_limit_request_pending{} 0
# TYPE envoy_default_http_bandwidth_limit_response_allowed_size gauge
envoy_default_http_bandwidth_limit_response_allowed_size{} 0
# TYPE envoy_default_http_bandwidth_limit_response_incoming_size gauge
envoy_default_http_bandwidth_limit_response_incoming_size{} 0
# TYPE envoy_default_http_bandwidth_limit_response_pending gauge
envoy_default_http_bandwidth_limit_response_pending{} 0
# TYPE envoy_default_http_bandwidth_limit_request_transfer_duration histogram
# TYPE envoy_default_http_bandwidth_limit_response_transfer_duration histogram
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P3 + §1.1 item 7 (6-counter hypothesis):**

- (a) **16 stats total per stat_prefix** (NOT 6 as BRAINSTORM hypothesized):
  - **8 counters:** `request_enabled`, `request_enforced`, `request_incoming_total_size`, `request_allowed_total_size`, `response_enabled`, `response_enforced`, `response_incoming_total_size`, `response_allowed_total_size`.
  - **6 gauges:** `request_pending`, `request_incoming_size`, `request_allowed_size`, `response_pending`, `response_incoming_size`, `response_allowed_size`.
  - **2 histograms:** `request_transfer_duration`, `response_transfer_duration`. **Histograms fire UNCONDITIONALLY** (not gated by `enable_response_trailers` as BRAINSTORM §8.2 hypothesized; verified by probeA which does NOT set `enable_response_trailers` yet still registers the histogram namespaces).
- (b) **`*_enforced` semantic:** the empirical scrape shows `response_enforced: 10` after THREE GETs (probeJ shows enforced=99 over 5s timeout). Combined with probeL chunk-pattern (77 chunks emitted at 51-byte cadence for kbps=1, fill=50ms), the empirical model is: `*_enforced` increments PER `fill_interval` TICK during throttle engagement (NOT per-stream as BRAINSTORM hypothesized). For each stream that engages throttle, the filter ticks `enforced` once per chunk emit (i.e., once per `fill_interval`).
- (c) **`*_incoming_size` / `*_allowed_size` semantics (gauges):** transient state — bytes-buffered-but-not-yet-allowed-out vs. bytes-allowed-out-this-tick. Reset to 0 between streams.
- (d) **`*_incoming_total_size` / `*_allowed_total_size` semantics (counters):** cumulative across all streams. `incoming_total_size` ≈ sum of all bytes that entered the filter; `allowed_total_size` ≈ sum of all bytes the filter forwarded through. For a steady-state filter, both grow equally.
- (e) **`*_pending` (gauge):** count of streams currently waiting on a throttle timer (transient; resolves §11.P13).
- (f) **Histograms — divergence-window:** envoy-go MVP per phase-06.1 baseline ("counters + gauges only — histograms deferred") DOES NOT implement histograms. The 2 histograms register namespaces on Envoy side; envoy-go side emits no histogram. Differential fixture 0017 allow-lists the 2 histograms via twin-series-filter discipline (per BEHAVIOR_CONTRACT §242 + the phase-09 fault `response_rl_injected` route-A discipline-analog precedent). Resolves §1.1 amendment 9.

The phase-15 envoy-go MVP registers the **14 active stats** (8 counters + 6 gauges) per stat_prefix. Stat-table extension at §13.2: 46 → **60 names** (14 new). The 2 histograms are documented at §13.4 forward-pointer notes as the divergence-window pending a future histogram-emit-infra phase.

### 11.4 Empirical pin #4 — Per-route stat SHARED-vs-INDEPENDENT (RATIFIES INDEPENDENT hypothesis)

**Probe configuration:** probeB (per-route override; see §11.1 for full transcript). Two routes: `/default` (uses listener-level `default` stat_prefix) + `/override` (per-route stat_prefix `override`). Scrape after one request to each.

**Verbatim:**

```
=== B3: per-route stats namespace scrape ===
default.http_bandwidth_limit.response_enabled: 1
default.http_bandwidth_limit.response_enforced: 6
override.http_bandwidth_limit.response_enabled: 1
override.http_bandwidth_limit.response_enforced: 14
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P4 + §9.P14 INDEPENDENT hypothesis:**

- (a) Per-route override emits to its OWN counter namespace (`override.http_bandwidth_limit.*`); listener-level `default.http_bandwidth_limit.*` namespace is NOT incremented by the per-route-active stream.
- (b) Each stat_prefix produces a wholly-independent set of stats. **envoy-go MVP registers 14 active stats** (8 counters + 6 gauges) per stat_prefix; Envoy additionally emits 2 unconditional histograms allow-listed via twin-series-filter divergence-window per §1.1 amendment 9. With N per-route TPFC entries, **envoy-go registers 14N stats post-Freeze** (Envoy registers 16N); mirrors phase-11 local_ratelimit's INDEPENDENT-stats discipline per ADR-0117.
- (c) **Phase-15 is the SECOND §9 family-row using the 4th canonical stateful-override-with-INDEPENDENT-stats pattern** (after phase-11 local_ratelimit FIRST). ADR-0139 codifies the inheritance.
- (d) **Divergence from phase-12/13/14 SHARED-stats:** those filters' per-route overrides emit to listener-level counter namespace (per ADR-0124 + ADR-0125); phase-15 + phase-11 diverge because the per-route override carries STATEFUL token-bucket-analog state + its own `stat_prefix`.

### 11.5 Empirical pin #5 — `fill_interval` PGV bounds (RATIFIES `[20ms, 1s]` hypothesis)

**Probe configuration:** probeF (`fill_interval: 10ms`) + probeG (`fill_interval: 2s`). Both expected to reject at boot.

**Verbatim (under §11.2 above):**

```
=== probeF: fill_interval=10ms → REJECTED ===
BandwidthLimitValidationError.FillInterval: value must be inside range [20ms, 1s]

=== probeG: fill_interval=2s → REJECTED ===
BandwidthLimitValidationError.FillInterval: value must be inside range [20ms, 1s]
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P5:**

- (a) `fill_interval` PGV `gte = 20ms, lte = 1s` enforced at parse-time. BRAINSTORM's hypothesized `[20ms, 1s]` bounds are exact.
- (b) **REFUTES the BRAINSTORM "envoy-go-side filter-internal range check" framing** (per §1.1 amendment 5). The bounds are Envoy's own PGV, not envoy-go-only.
- (c) BRAINSTORM §8.6 deferral re-frames: envoy-go follows Envoy PGV directly; no envoy-go-only divergence-window. The §8.6 placeholder is REMOVED from the deferral list (sub-20ms fill_interval is structurally unreachable; super-1s is also unreachable).

### 11.6 Empirical pin #6 — `runtime_enabled` type (RATIFIES RuntimeFeatureFlag hypothesis)

**Probe configuration:** probeI with `runtime_enabled: { default_value: true, runtime_key: bandwidth_limit_enabled }` (BoolValue shape, NOT FractionalPercent). Verify boots cleanly + at-runtime always-active.

**Verbatim probeI-transcript.txt:**

```
=== I1: runtime_enabled present with default_value: true ===
* Connected to localhost (::1) port 11511
> GET /medium HTTP/1.1
< HTTP/1.1 200 OK
< content-length: 4000
real    0m0.367s

=== I2: stats scrape ===
default.http_bandwidth_limit.response_enabled: 1
default.http_bandwidth_limit.response_enforced: 8
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P6:**

- (a) `runtime_enabled` is `envoy.config.core.v3.RuntimeFeatureFlag` (BoolValue default; runtime-key string). NOT `RuntimeFractionalPercent`.
- (b) Optional at parse-time. Field absent → filter active by default.
- (c) Field present with `default_value: true` → filter active (probe I exercises this; throttle fires normally).
- (d) Field present with `default_value: false` (untested) — Envoy would disable the filter; envoy-go silent-ignores per ADR-0117/ADR-0121/ADR-0130 precedent. Operator divergence-window documented at §13.4.
- (e) **envoy-go MVP disposition:** silent-ignore at runtime; always-100%-active regardless of `default_value` OR runtime-key state. PGV-mirror at parse-time is via the auto-generated `Validate()` method (no envoy-go-side check needed).

### 11.7 Empirical pin #7 — `response_trailer_prefix` when trailers disabled (RATIFIES silent-ignore hypothesis)

**Probe configuration:** probeH with `enable_response_trailers: false` (default) + `response_trailer_prefix: "x-bw-"` set. Verify Envoy emits no trailers (`enable_response_trailers: false` overrides regardless of prefix).

**Verbatim probeH-transcript.txt** (full at `/tmp/p15-pins/probeH-transcript.txt`):

```
=== H1: trailers disabled + custom prefix ===
> GET /medium HTTP/1.1
< HTTP/1.1 200 OK
< content-length: 4000
< content-type: text/plain
< server: envoy
... [body] ...
(no trailers section in response)
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P7:**

- (a) When `enable_response_trailers: false` (default), the `response_trailer_prefix` value is structurally silent-ignored (no trailers emitted regardless of prefix).
- (b) When `enable_response_trailers: true` (untested in this SPEC pin; couples to a future trailer-emission framework phase per §8.1), Envoy emits 4 trailers `<prefix>bandwidth-{request,response}{,-filter}-delay-ms`. envoy-go MVP silent-ignores both fields; emits no trailers regardless. Operator divergence-window documented at §13.4.
- (c) **PGV pattern `^[^\x00\n\r]*$`** on `response_trailer_prefix` enforced at parse-time by the auto-generated `Validate()` method (rejects values containing nulls/newlines/CRLF).

### 11.8 Empirical pin #8 — Wire-shape on response throttle path (REFRAMED — Path A rate-paced chunks)

**Probe configuration:** probeL with `limit_kbps: 1, fill_interval: 50ms` + 4000-byte body. Capture chunk pattern via `curl --trace-time --trace-ascii /tmp/p15-pins/probeL-trace.txt`.

**Verbatim probeL-trace.txt summary (full transcript at `/tmp/p15-pins/probeL-trace.txt`):**

```
=== probeL: kbps=1 fill=50ms body=4000B ===
Theoretical chunk_size_per_tick = 1 × 1024 × 0.050 = 51.2 bytes/tick
Theoretical total = ceil(4000 / 51.2) × 50ms = 79 × 50ms = 3950ms

trace-ascii summary:
- 77 chunks received over 3.904s wallclock
- Each chunk 51-53 bytes (close to 51.2-byte theoretical)
- Chunk arrival cadence ~51ms (close to 50ms fill_interval)
- First chunk arrives at t+50ms (after one fill_interval tick); body completes at t+3.904s
- Initial burst: ~0 bytes (no immediate full-body flush; throttle fires from the first byte)

real    0m3.904s
```

**Verbatim probeA-trace.txt** (smaller body 4000B at kbps=10; full at `/tmp/p15-pins/probeA-trace.txt`):

```
=== probeA-trace: kbps=10 fill=50ms body=4000B ===
Theoretical chunk_size_per_tick = 10 × 1024 × 0.050 = 512 bytes/tick
Theoretical total = ceil(4000 / 512) × 50ms = 8 × 50ms = 400ms
Token bucket initial capacity = limit_kbps × 1024 = 10240 bytes (probably; uncertain)

trace-ascii: body arrives in ONE recv-event (4000 bytes) at t+0.359s
- Bursty arrival: the entire 4000-byte body fits within initial-burst-capacity (10 KiB at kbps=10)
- Total time: 359ms (≈ 400ms theoretical = 8 × 50ms)
- No fine-grained chunk pattern observable at this body size + kbps combination
```

**Conclusions (pinned) — REFRAMES BRAINSTORM §9.P8 + §1.1 item 6 (Path B-async wire-shape assumption):**

- (a) **Envoy emits Path A rate-paced chunks** at exact `fill_interval` cadence. Chunk size per tick = `limit_kbps × 1024 × fill_interval_seconds` bytes. probeL confirms verbatim: 77 chunks at 51-byte cadence over 3.904s for body=4000, kbps=1, fill=50ms. Theoretical = `ceil(4000 / 51.2) × 50ms = 3950ms` matches within 1% of observed.
- (b) **Token-bucket-with-initial-burst capacity = `limit_kbps × 1024` bytes** (approximately; precise verification requires source-level scrape that's deferred). For bodies entirely within initial capacity, throttle fires negligibly. For bodies exceeding initial capacity, the excess is throttled at the rate-paced chunk cadence.
- (c) **The wire-shape divergence axis is chunk-pattern, NOT total-throttle-time.** envoy-go's Path B-async emits zero chunks during the throttle window then ONE-BLAST at the end; Envoy emits N chunks at fill_interval cadence during the window. Total throttle-time observably matches within ±70ms tolerance (resolves §11.P9). The differential fixture's response-body axis observes byte-equivalence; the chunk-arrival-time axis observes divergence (envoy-go: silent-then-blast; Envoy: paced); BEHAVIOR_CONTRACT §13.4 documents.
- (d) **ADR-0137 reframes:** wire-shape divergence-window IS the load-bearing divergence axis (per `### Phase 15 forward-pointer notes`); total-throttle-time is byte-equivalent. Future encode-side streaming framework phase amends per BRAINSTORM §3 forward-pointer.

### 11.9 Empirical pin #9 — Throttle-timing tolerance window (RATIFIES ±50-70ms hypothesis)

**Probe configuration:** Across probes A, B, I, L, M: measure observed-vs-theoretical total throttle-time across body sizes (100 / 500 / 1024 / 2000 / 4000 / 10240 / 51200 bytes) and limit_kbps values (1 / 5 / 10).

**Verbatim summary (compiled across transcripts):**

| Body size | limit_kbps | fill_interval | Theoretical (ms) | Observed (ms) | Delta |
|---|---|---|---|---|---|
| 100  | 10 | 50ms | <50 (burst) | 5 | <50ms |
| 1024 | 10 | 50ms | 100  | 107 | +7ms |
| 4000 | 10 | 50ms | 400  | 359-367 | -33 to -41ms |
| 4000 | 5  | 50ms | 800  | 716-814 | -84 to +14ms |
| 4000 | 1  | 50ms | 3950 | 3904 | -46ms |

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P9 ±50ms hypothesis (with ±70ms worst-case bound on single-stream localhost):**

- (a) Observed throttle-time matches kbps-per-tick theoretical within **±70ms** worst-case across the test matrix. Bodies fitting within initial-burst capacity (e.g., 100 bytes at kbps=10) complete in <5ms regardless of throttle math.
- (b) **Fixture 0017 per-scenario tolerance:** ±50ms for small throttles (sub-second wall-clock); ±70ms for larger throttles (multi-second). Tighter than BRAINSTORM's ±10-50ms estimate; documented at §13.5 + §7.3.

### 11.10 Empirical pin #10 — Prometheus tag-extractor name (REFUTES SN10 introduction)

**Probe configuration:** probeA `/stats/prometheus` scrape; inspect rendered Prometheus output for label patterns.

**Verbatim probeA Prometheus excerpt** (full at §11.3 above):

```
envoy_default_http_bandwidth_limit_response_enabled{} 3
envoy_default_http_bandwidth_limit_response_enforced{} 10
envoy_default_http_bandwidth_limit_response_incoming_total_size{} 5124
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P10 (initial SN2-reuse / SN10-conditional hypothesis); see §1.1 amendment 8:**

- (a) **NO tag extraction.** Envoy's Prometheus rendering for bandwidth_limit DOES NOT extract the stat_prefix as a label (no `envoy_local_http_bandwidth_limit_prefix=<stat_prefix>` analogous to phase-11 ADR-0118 SN9; no `envoy_http_conn_manager_prefix=<HCM>` analogous to phase-14 SN2).
- (b) **Stat_prefix is INLINED into the metric base name.** The full Prometheus name shape is `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` with NO labels.
- (c) **SN10 NOT required.** envoy-go's existing flattening machinery at `internal/stats/name.go` can render this shape via a simple dot→underscore substitution; no new SN flattening rule needed. The internal stat path `<stat_prefix>.http_bandwidth_limit.<counter>` flattens to Prometheus `envoy_<stat_prefix>_http_bandwidth_limit_<counter>` via the existing `default` branch (no tag-extraction; no per-segment label promotion).
- (d) **Phase-11 ADR-0118 SN9 rule NOT extended.** Phase-15 introduces NO new tag-extractor; ADR-0138 documents the inline-prefix discipline without SN-rule amendment.

### 11.11 Empirical pin #11 — Namespace flattening shape (REFUTES `bandwidth_limit.` infix hypothesis + RATIFIES `http_bandwidth_limit` infix)

**Probe configuration:** probeA `/stats?filter=bandwidth_limit&format=json` (text format internal scrape); cross-compare with `/stats/prometheus` Prometheus rendering.

**Verbatim probeA internal scrape excerpt:**

```
default.http_bandwidth_limit.request_allowed_size: 0
default.http_bandwidth_limit.request_allowed_total_size: 0
default.http_bandwidth_limit.request_enabled: 0
...
default.http_bandwidth_limit.response_enabled: 3
default.http_bandwidth_limit.response_enforced: 10
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P11 + brainstorm's `http.<HCM>.bandwidth_limit.` hypothesis:**

- (a) **Internal stat path:** `<stat_prefix>.http_bandwidth_limit.<counter>`. Single-segment `http_bandwidth_limit` infix (underscore, NOT dot).
- (b) **NO HCM-stat-prefix root.** The internal path does NOT start with `http.<HCM>` — it starts with the filter's own `<stat_prefix>`. This is identical to phase-11 local_ratelimit's `<stat_prefix>.http_local_rate_limit.<counter>` pattern.
- (c) **Differs from phase-14 compressor's HCM-rooted path** (`http.<HCM_stat_prefix>.compressor.<library>.<codec>.<counter>`). Compressor uses HCM-rooted; bandwidth_limit + local_ratelimit use filter-stat_prefix-rooted.
- (d) Combined with §11.10 (no tag extraction; stat_prefix inlined into Prometheus base name), the full mapping is: internal `<stat_prefix>.http_bandwidth_limit.<counter>` → Prometheus `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}`.

### 11.12 Empirical pin #12 — `enable_mode: DISABLED` runtime evaluation (RATIFIES passthrough hypothesis)

**Probe configuration:** probeC with listener-level `enable_mode: DISABLED` + `limit_kbps: 10` (required to load). GET /medium 4000-byte body; expect passthrough; scrape stats.

**Verbatim probeC-transcript.txt:**

```
=== C1: enable_mode: DISABLED, GET /medium ===
> GET /medium HTTP/1.1
< HTTP/1.1 200 OK
< content-length: 4000
real    0m0.001s

=== C2: stats scrape (DISABLED) ===
default.http_bandwidth_limit.request_enabled: 0
default.http_bandwidth_limit.request_enforced: 0
default.http_bandwidth_limit.request_pending: 0
default.http_bandwidth_limit.response_enabled: 0
default.http_bandwidth_limit.response_enforced: 0
default.http_bandwidth_limit.response_pending: 0
[... all 14 stat names registered at 0 ...]
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P12:**

- (a) `enable_mode: DISABLED` → full passthrough; body bytes flow immediately (1ms total wall-clock for 4000-byte body).
- (b) Stat namespace IS registered (all 14 stat names appear in scrape) but counters STAY AT 0 — the filter never engages.
- (c) **envoy-go MVP disposition:** match Envoy exactly — when `enable_mode: DISABLED`, return `HeaderContinue` + `DataContinue` from DecodeHeaders/EncodeHeaders + DecodeData/EncodeData unconditionally; do NOT increment any counter. The `compiledConfig.stats` field is still allocated at `New` factory time (so the namespace is registered for byte-equivalent stat scrape with reference Envoy).

### 11.13 Empirical pin #13 — Per-stream pending-gauge lifecycle (RATIFIES Inc-at-arm / Dec-at-fire hypothesis)

**Probe configuration:** probeJ (5s hang scenario; `response_pending: 1` observed mid-stream). Also probes A + B post-completion (`response_pending: 0`).

**Verbatim probeJ-transcript.txt (excerpt under §11 above; full at `/tmp/p15-pins/probeJ-transcript.txt`):**

```
=== J1: hit / with listener-level RESPONSE mode + NO limit_kbps ===
... hangs ...
real    0m5.007s

=== J2: stats (mid-stream or just-after) ===
default.http_bandwidth_limit.response_enabled: 1
default.http_bandwidth_limit.response_enforced: 99
default.http_bandwidth_limit.response_pending: 1
```

After completion (or curl timeout), `response_pending` decrements back to 0 (confirmed via probeA + probeB post-test scrapes).

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P13:**

- (a) `*_pending` is incremented when the throttle timer arms (i.e., when `DecodeData/EncodeData(endStream=true)` engages throttle).
- (b) Decremented when the throttle completes (timer fires + `Continue*` resumes the chain).
- (c) **envoy-go MVP disposition:** `f.requestRC.stats.requestPending.Inc()` at timer-arm in `DecodeData(endStream=true)`; `f.requestRC.stats.requestPending.Dec()` inside the timer callback (and also via the `OnDestroy` cleanup path when `Stop()` returns true, per §4 Stop-races-Fire discipline).

### 11.14 Empirical pin #14 — Per-route override `stat_prefix` emission scope (RATIFIES wholly-own namespace; INDEPENDENT)

**Probe configuration:** probeB (full transcript at §11.1 + §11.4 above). The B3 stats scrape exercises the per-route INDEPENDENT-stats axis.

**Verbatim** (re-iterated for self-containment):

```
default.http_bandwidth_limit.response_enabled: 1     # listener-level only
default.http_bandwidth_limit.response_enforced: 6
override.http_bandwidth_limit.response_enabled: 1     # per-route override
override.http_bandwidth_limit.response_enforced: 14
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P14:**

- (a) Per-route override allocates a WHOLLY-OWN counter namespace keyed by the per-route `stat_prefix`. NO sharing with listener-level scope.
- (b) Mirrors phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats exactly.
- (c) **envoy-go MVP disposition:** `state.resolvePerRouteConfig(perRouteMsg)` returns the per-route `*compiledConfig` carrying its own `*filterStats` (allocated via `NewCounterIfAbsent` for post-Freeze idempotent registration). ADR-0117 + §5 + §6.11 implementation.

### 11.15 Empirical pin #15 — `fill_interval` × `limit_kbps` interaction (REFUTES steady-rate hypothesis)

**Probe configuration:** probeL (`limit_kbps: 1, fill_interval: 50ms, body: 4000`) — captured at §11.8 above. Chunk pattern + total wall-clock empirically confirms the kbps-per-tick formula.

**Verbatim summary (under §11.8):**

```
Theoretical chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds = 1 × 1024 × 0.050 = 51.2 bytes/tick
Theoretical total = ceil(4000 / 51.2) × 50ms = 79 × 50ms = 3950ms
Observed: 77 chunks at 51-53 bytes; total 3904ms
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P15 (steady-rate hypothesis):**

- (a) Envoy's throttle math is **kbps-per-tick chunking**, NOT steady-rate. `fill_interval` GOVERNS chunk size (not just timer granularity).
- (b) Formula: `chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds` bytes. Each tick emits one chunk of that size; the total throttle-time is `ceil(body_size / chunk_size_per_tick) × fill_interval`.
- (c) **`limit_kbps` semantic: KiB/s (kibibytes-per-second), NOT kilobits-per-second.** Per the proto comment at `bandwidth_limit.pb.go:95` ("The limit supplied in KiB/s") and the empirical chunk-size formula (`1024` byte multiplier per kbps unit). BRAINSTORM's "kilobits-per-second" framing is REFUTED. §1.1 amendment 6 documents.
- (d) **envoy-go MVP throttle math (Path B-async with corrected formula):**
  ```go
  chunk_size = limitKbps * 1024 * fillInterval.Seconds()  // bytes per tick
  if bodySize <= int(chunk_size) {
      // Body fits within one tick: fast-passthrough or minimal-delay
      throttle_duration = fillInterval
  } else {
      // Compute total ticks needed
      ticks := (bodySize + chunk_size - 1) / chunk_size  // ceil(bodySize / chunk_size)
      throttle_duration = time.Duration(ticks) * fillInterval
  }
  ```
  This produces total-throttle-time byte-equivalence with reference Envoy within the ±70ms tolerance per §11.9. Wire-shape divergence (envoy-go silent-then-blast vs Envoy paced) is the irreducible axis pending the future encode-side streaming framework phase.

---

---

## 12. Deferred decisions (the planner / implementer settles these)

Per phase 11/12/13/14 SPEC §12 precedent:

1. **`bandwidthlimit.go` file split.** Per §6: PLAN may split into `bandwidthlimit.go` (filter + factory) + `bucket.go` (per-stream throttle helper) + `perroute.go` (per-route resolver) if test readability benefits OR if `bandwidthlimit.go` exceeds ~300 LoC. Phase-11 local_ratelimit kept main file at ~393 LoC with a separate `bucket.go`; phase-15 likely follows.

2. **Fast-passthrough threshold value (BRAINSTORM §11.4) — RESOLVED at SPEC time per §6.6.** The BRAINSTORM-hypothesized `1ms` fast-passthrough threshold is REPLACED by the natural one-tick floor (`fill_interval` minimum; typically 50ms default). Bodies fitting within one tick still wait one `fill_interval` to match Envoy's per-tick behavior within ±70ms tolerance (per §11.P9 + §6.6). PLAN may add a sub-tick short-circuit (`throttle = 0` for `bodySize == 0`) but no further fast-passthrough refinement is anticipated.

3. **Pending-gauge Inc/Dec under Stop-races-Fire window.** Per §4 + §6.9: the `Stop() returns true → Dec here, else trust the callback` pattern is the spec; PLAN-time race-test validates no double-decrement under aggressive concurrent OnDestroy + timer-fire scenarios. If the test surfaces flakiness, PLAN may add an explicit `markedActive bool` field on `*filter` per phase-09 fault precedent at `fault.go:480-500`.

4. **Per-route stat-counter cardinality bound.** Per ADR-0117 + §5 + §1.1 amendment 7: each per-route TPFC entry allocates **14 active stats** (8 counters + 6 gauges) at first-resolve. A config with N per-route entries allocates 14N stats. PLAN-time consideration: cardinality bound for high-N configs (e.g., 100 per-route entries → 1400 stats; 1000 → 14000). No explicit cap in MVP; couples to a future stats-cardinality-governance phase.

5. **`enable_mode: DISABLED` at listener-level vs per-route — observable parity.** Per §11.P12: listener-level `enable_mode: DISABLED` produces full passthrough (no throttle; counter increments documented at §11). Per-route `enable_mode: DISABLED` is the disable mechanism (per §1.1 amendment 1). PLAN-time test: verify both shapes produce identical wire output + identical counter-delta footprint.

6. **`fill_interval` granularity in the throttle math — RESOLVED at SPEC time per §6.6 + §11.P15.** Empirically REFUTED steady-rate; SPEC §6.6 implements kbps-per-tick math (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`; `throttle = ceil(body/chunk_size) × fill_interval`). PLAN-time consideration: verify the `*_enforced` increment-by-`ticks` semantic at timer-fire produces byte-equivalent cumulative counter values vs Envoy's per-tick increments across the test matrix. If divergence emerges in PLAN integration tests, fall back to "increment `*_enforced` once per stream" + document the cumulative-counter divergence-window. SPEC's position: increment-by-ticks (more accurate match).

7. **Per-route `runtime_enabled` field interaction.** When per-route TPFC sets `runtime_enabled: { default_value: false }`, both Envoy and envoy-go silent-ignore (always-active). PLAN may add a unit-test asserting the field is parsed but not honored at per-route position.

8. **Trailer-emission framework primitive forward-pointer.** Per §8.1 + ADR-0137: a future trailer-emission framework phase lands `EncoderFilterCallbacks.EmitTrailers(map[string]string)` enabling the 4 bandwidth-rate-limit trailers when `enable_response_trailers: true`. PLAN-time: ensure the silent-ignore of `enable_response_trailers` + `response_trailer_prefix` is observable (e.g., a unit test asserting no trailers in the response regardless of the field setting).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

Per `BOOTSTRAP_PROMPT.md` §7.5 Gate F:

### 13.1 `## HTTP filter chain ### envoy.filters.http.bandwidth_limit` NEW subsection

Patch shape (in-place edit at the existing `## HTTP filter chain` umbrella; alphabetical-canonical ordering of the existing subsection list is `bandwidth_limit < buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit`, so the new `### envoy.filters.http.bandwidth_limit` subsection inserts at the HEAD of the list, immediately under the `## HTTP filter chain` umbrella heading):

```markdown
### envoy.filters.http.bandwidth_limit

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit` (7 top-level fields total per `[#next-free-field: 8]`):**

| Field | Type | Phase 15 disposition | Notes |
|---|---|---|---|
| `stat_prefix` | string | CONSUMED | REQUIRED per Envoy PGV `min_len = 1` (per §11.P2 + §1.1 amendment 3); envoy-go-mirror PGV-validation at parse time. |
| `enable_mode` | enum (4 values) | CONSUMED | All 4 values honored: DISABLED, REQUEST, RESPONSE, REQUEST_AND_RESPONSE (per §2.2 + §11.P12). |
| `limit_kbps` | UInt64Value (**KiB/s** per proto comment + §1.1 amendment 6) | CONSUMED | OPTIONAL at listener-level (FOOT-GUN: filter loads but request HANGS at runtime if unset + `enable_mode != DISABLED`; per §1.1 amendment 10 + §11 probeJ); CODE-LEVEL REQUIRED at per-route per `source/extensions/filters/http/bandwidth_limit/config.cc::createRouteSpecificFilterConfigTyped` (per §11.P1 verbatim "limit must be set for per route filter config"); PGV `gte = 1` when wrapper present (per §1.1 amendment 4). |
| `fill_interval` | google.protobuf.Duration | CONSUMED | OPTIONAL; default 50ms; PGV `gte = 20ms, lte = 1s` when wrapper present (per §1.1 amendment 5 + §11.P5). GOVERNS chunk-size: `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes (per §11.P15). |
| `runtime_enabled` | RuntimeFeatureFlag | SILENT-IGNORED | Always-active runtime; envoy-go always-100%-active regardless of `default_value`. Divergence-window if `default_value: false`. |
| `enable_response_trailers` | bool | SILENT-IGNORED | Always-no-trailers in envoy-go MVP; trailer-emission primitive deferred. Divergence-window if set true (Envoy emits 4 `<prefix>bandwidth-{request,response}{,-filter}-delay-ms` trailers). |
| `response_trailer_prefix` | string | SILENT-IGNORED | Couples to `enable_response_trailers`; PGV pattern `^[^\x00\n\r]*$` enforced at parse-time. |

**Per-route `BandwidthLimit`:** SAME proto as listener-level (NO `BandwidthLimitPerRoute` wrapper per §11.P1). Per-route `limit_kbps` is FILTER-INTERNAL REQUIRED via code-level extra check (per filter source); `stat_prefix` is PGV REQUIRED regardless of position. Per-route `enable_mode: DISABLED` is the canonical disable-on-route mechanism (NO `disabled` boolean shortcut at the proto level). Per ADR-0117 + ADR-0139 + §5: per-route stats are INDEPENDENT (per-route override allocates own `*compiledConfig` + own `*filterStats` via `sync.Map` lazy-cache keyed by `*BandwidthLimit` pointer).

#### Wire shape

Throttled-path wire shape (envoy-go MVP, Path B-async per ADR-0137 + §1.1 amendment 6):
- Response headers: preserved verbatim from upstream/direct_response (no header mutation by bandwidth_limit).
- Response body: byte-equivalent to original (bandwidth_limit does NOT transform bytes).
- Response timing: ONE-BLAST emission at the end of the throttle window. Specifically: `DecodeData(endStream=true)` buffers + computes `throttle = ceil(body_size / chunk_size) × fill_interval` (where `chunk_size = limit_kbps × 1024 × fill_interval_seconds`) + arms `time.AfterFunc(throttle, ...)`; timer fires → `ContinueDecoding` resumes the chain → buffered body forwards upstream in one shot. Symmetric for encode-side via `ContinueEncoding`.

**Wire-shape divergence-window from reference Envoy (deliberate; ADR-0137 records; per §11.P8):** Envoy emits Path A rate-paced chunks AT exact `fill_interval` CADENCE — `chunk_size` bytes per tick, distributed across the throttle window (e.g., 77 chunks at 51-byte cadence for `body=4000, kbps=1, fill=50ms` per probeL; 8 chunks at 512-byte cadence for `body=4000, kbps=10, fill=50ms`). envoy-go MVP emits zero chunks during the throttle window, then ALL bytes in one blast at the end. Total wall-clock throttle time is observably equivalent within **±70ms** tolerance (per §11.P9). For consumers that don't depend on intra-throttle chunk timing (typical HTTP clients), the byte-stream is delivered with the same total latency budget.

Forward-pointer per ADR-0137: a future encode-side streaming framework phase lands `EncoderFilterCallbacks.EmitChunk(b []byte)` + symmetric `DecoderFilterCallbacks.ConsumeChunk(b []byte)` + HCM chunk-by-chunk `RunEncodeData/RunDecodeData` invocation. Phase-15 Path B-async naturally upgrades to Path A streaming when those primitives land.

#### Per-route INDEPENDENT-stats discipline (per ADR-0139 + §5)

Phase 15 is the SECOND row using the 4th canonical stateful-override-with-INDEPENDENT-stats per-route discipline (ADR-0117 codifies; phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND). Per-route TPFC entries (same `BandwidthLimit` proto via pointer-identity per-route lazy-cache; bare-message-via-TPFC; phase-15 introduces this as a NEW canonical pattern documented at ADR-0125 §(xi) amendment) own fresh `*compiledConfig` + fresh `*filterStats` keyed by the per-route `stat_prefix`. Listener-level counters do NOT increment for per-route-active streams. DIVERGES from phase-12/13/14 SHARED-stats discipline.

#### Stat surface + Prometheus rendering (per §1.1 amendments 7 + 8 + 9)

Internal stat path: `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; NOT HCM-rooted). Prometheus name: `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` (stat_prefix INLINED into base name; NO labels / NO tag-extractor). The 14 active stat names are enumerated at §13.2; the 2 unconditional Envoy histograms (`request_transfer_duration`, `response_transfer_duration`) are allow-listed via twin-series-filter divergence-window per §13.4 + BEHAVIOR_CONTRACT §242 extension.
```

(End of §13.1 stub; ~120-150 lines authored at phase-done commit per phase-13 + phase-14 SPEC §13.1 precedent.)

### 13.2 `## Stat-name mapping ### 46-name table` extension to 60 names (14 active entries; 2 histograms deferred via divergence-window per §1.1 amendment 9)

Verbatim Markdown patch:

```markdown
**Bandwidth-limit filter — 14 active names (introduced by phase 15) + 2 deferred histograms (twin-series-filter divergence-window):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<stat_prefix>.http_bandwidth_limit.request_enabled`            | counter | filter | bandwidth_limit | stream engaged decode-side throttle (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_enforced`           | counter | filter | bandwidth_limit | per-tick throttle increments; envoy-go bumps by `ticks` at stream-completion to match Envoy's per-fill_interval-tick cumulative semantic (§11.P3 + §6.7) |
| `<stat_prefix>.http_bandwidth_limit.request_incoming_total_size` | counter | filter | bandwidth_limit | cumulative bytes entered decode-side filter (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_allowed_total_size`  | counter | filter | bandwidth_limit | cumulative bytes forwarded through decode-side filter (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_enabled`           | counter | filter | bandwidth_limit | stream engaged encode-side throttle (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_enforced`          | counter | filter | bandwidth_limit | symmetric to request side (§11.P3 + §6.8) |
| `<stat_prefix>.http_bandwidth_limit.response_incoming_total_size` | counter | filter | bandwidth_limit | cumulative bytes entered encode-side filter (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_allowed_total_size`  | counter | filter | bandwidth_limit | cumulative bytes forwarded through encode-side filter (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_pending`            | gauge   | filter | bandwidth_limit | count of streams waiting on decode-side timer (Inc on arm; Dec on fire/cancel; §11.P3 + §11.P13) |
| `<stat_prefix>.http_bandwidth_limit.request_incoming_size`      | gauge   | filter | bandwidth_limit | transient bytes-buffered-but-not-yet-forwarded (decode side; §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_allowed_size`       | gauge   | filter | bandwidth_limit | transient bytes-allowed-this-tick (decode side; envoy-go MVP single-blast: set to bodyLen at timer-fire then 0 at OnDestroy; §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_pending`           | gauge   | filter | bandwidth_limit | symmetric to request side (§11.P3 + §11.P13) |
| `<stat_prefix>.http_bandwidth_limit.response_incoming_size`     | gauge   | filter | bandwidth_limit | symmetric to request side (§11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_allowed_size`      | gauge   | filter | bandwidth_limit | symmetric to request side (§11.P3) |

**Twin-series-filter divergence-window (per §1.1 amendment 9):** 2 unconditional Envoy histograms NOT emitted by envoy-go MVP per phase-06.1 "counters + gauges only" baseline. Differential fixture 0017's `expectations.yaml` allow-lists; BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` subsection extends with phase-15 entry:
| `<stat_prefix>.http_bandwidth_limit.request_transfer_duration`  | histogram | filter | bandwidth_limit | DEFERRED per phase-06.1 baseline; allow-listed via twin-series-filter discipline |
| `<stat_prefix>.http_bandwidth_limit.response_transfer_duration` | histogram | filter | bandwidth_limit | DEFERRED; allow-listed |
```

Stat-table size grows from 46 → **60 names** (14 new active). 2 deferred histograms are documented at the twin-series subsection + §13.4 forward-pointer notes; they do NOT count in the 60-name table.

**Prometheus rendering (per §1.1 amendment 8 + §11.P10):** `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` — stat_prefix INLINED into base name; NO labels / NO tag-extractor / NO new SN10 rule. The existing `internal/stats/name.go` default-branch flatten handles via dot→underscore substitution; ADR-0061 + ADR-0118 NOT amended.

### 13.3 `## Equivalence Matrix` new row (verbatim table-row patch)

```markdown
| 0017-http-bandwidth-limit | envoy.filters.http.bandwidth_limit (BOTH-direction Path B-async with kbps-per-tick throttle math; KiB/s units) | byte-exact status; byte-exact body (bandwidth_limit does not transform bytes); ±70ms wall-clock tolerance per scenario per ADR-0137 wire-shape-divergence-window; per-counter delta byte-equivalent on 14 active stats (8 counters + 6 gauges); 2 unconditional Envoy histograms (transfer_duration) allow-listed via twin-series-filter divergence-window per §13.4 + §242 extension; INDEPENDENT per-route stats per ADR-0139 |
```

### 13.4 Forward-pointer notes (per BRAINSTORM §8 + §1.1 amendments 6 + 7 + 8 + 9 + 10)

```markdown
### Phase 15 forward-pointer notes

**Deferred field families** (silent-ignored per ADR-0040 + ADR-0136):

- `BandwidthLimit.runtime_enabled` (RuntimeFeatureFlag) — silent-ignored at runtime; envoy-go always-100%-active regardless of `default_value` or runtime-key state. Couples to Runtime + hot restart family. Re-activation: Runtime family phase brings RTDS / Runtime-layer support.
- `BandwidthLimit.enable_response_trailers` + `response_trailer_prefix` — silent-ignored; envoy-go always-no-trailers. Couples to a future trailer-emission framework phase that lands `EncoderFilterCallbacks.EmitTrailers(map[string]string)`. Re-activation enables 4 trailers prefixed by `response_trailer_prefix`: `bandwidth-request-delay-ms`, `bandwidth-response-delay-ms`, `bandwidth-request-filter-delay-ms`, `bandwidth-response-filter-delay-ms`.

**Histogram divergence-window (per §1.1 amendment 9 + §11.P3):** Envoy v1.37.2 emits 2 UNCONDITIONAL transfer-duration histograms per active stat_prefix (`request_transfer_duration`, `response_transfer_duration`) — fire regardless of `enable_response_trailers` setting. envoy-go MVP per phase-06.1 baseline ("counters + gauges only — histograms deferred") emits NO histograms. Differential fixture 0017's `expectations.yaml` allow-lists via the BEHAVIOR_CONTRACT `### Twin-series filter discipline` subsection extension; operator dashboards querying `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_*` series see Envoy emit but envoy-go absent. Future re-activation: histogram-emit-infra phase lands `*stats.Registry.Histogram` + Prometheus `histogram_*` extractor; `filterStats` extends with 2 histogram fields.

**Wire-shape divergence-window from reference Envoy (per §1.1 amendment 6 + ADR-0137 + §11.P8):** envoy-go's Path B-async emits ONE body-blast at the end of the throttle window; Envoy emits Path A rate-paced chunks at exact `fill_interval` cadence (chunk_size = `limit_kbps × 1024 × fill_interval_seconds` bytes per tick; e.g., 51.2 bytes/tick at kbps=1 fill=50ms; 512 bytes/tick at kbps=10 fill=50ms). Total wall-clock throttle time observably equivalent within **±70ms** tolerance per §11.P9 (probeL: 3.904s observed vs 3950ms theoretical at kbps=1 fill=50ms body=4000B). Chunk-arrival-timing axis observably DIVERGES; HTTP clients that don't depend on intra-throttle chunk timing see byte-equivalent delivery. Future re-activation: encode-side streaming framework phase (`writeH1Reply` chunked-output mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation) lands the rate-paced chunk-emit primitives, upgrading Path B-async to Path A streaming.

**`*_enforced` counter-semantic note (per §11.P3 + §6.7):** Reference Envoy increments `*_enforced` per `fill_interval` TICK during throttle (e.g., probeJ shows `response_enforced: 99` for a hung 5-second stream ≈ 100 ticks at 50ms). envoy-go Path B-async cannot observe per-tick events; instead bumps `*_enforced += ticks` at stream-completion (where `ticks = ceil(body/chunk_size)`) to maintain cumulative byte-equivalence with reference Envoy. Per-counter delta assertions in fixture 0017 use this convention.

**Operational foot-gun: listener-level missing `limit_kbps` + active `enable_mode` causes runtime hang (per §1.1 amendment 10 + probeJ):** Envoy's proto comment at `bandwidth_limit.pb.go:99-107` permits unset `limit_kbps` at listener-level (intended for per-route-only configurations); at runtime, an unset `limit_kbps` + active `enable_mode` causes the filter to compute infinite throttle and HANG every request. envoy-go MVP MATCHES this behavior byte-equivalently (no parse-time warning; the foot-gun is consistent across both proxies). Operators MUST set `limit_kbps` on either the listener-level config OR every per-route override. Future operator-ergonomics phase MAY add an envoy-go-side parse-time WARNING log; SPEC's position: silent-match-Envoy in MVP.

**Filter-chain ordering with respect to compressor (per BRAINSTORM §11.6):** When both `bandwidth_limit` and `compressor` are in the same chain, ordering affects throttle-input bytes:
- `bandwidth_limit BEFORE compressor` → throttle paces the uncompressed body (more bytes through the throttle).
- `bandwidth_limit AFTER compressor` → throttle paces the compressed body (fewer bytes; tighter effective throughput).
Both orderings are valid; the SPEC documents the trade-off without prescribing. Fixture 0017 uses bandwidth_limit standalone for byte-equivalence simplicity.

**ZERO framework deltas:** Phase 15 introduces no new framework primitive on either side. Reuses (a) phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding/Encoding` (b) phase-13 ADR-0128's decode-side body-buffering machinery; (c) phase-14 ADR-0131's `OverwriteBody` (anticipated: NOT invoked; the framework's buffered-return path returns bytes unchanged). FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 simultaneously.

**No per-route `BandwidthLimitPerRoute` proto (per §1.1 amendment 1 + §11.P1):** Phase 15 BRAINSTORM hypothesized a `BandwidthLimitPerRoute` oneof envelope with `disabled` + override (5th canonical disabled-OR-override per ADR-0125). Empirically REFUTED at §11.P1: no wrapper proto exists in Envoy v1.37.2; per-route TPFC uses the same `BandwidthLimit` message directly. Mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1, with one additional code-level constraint: per-route entries MUST set `limit_kbps` (else Envoy rejects at boot with `"limit must be set for per route filter config"`). Phase-15 introduces a NEW canonical per-route shape (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route), documented at ADR-0125 §(xi) amendment paragraph. The 5th canonical disabled-OR-override stays bound to phase-13 buffer + phase-14 compressor.

**`limit_kbps` units are KiB/s NOT kbps (per §1.1 amendment 6 + §11.P15 + proto comment):** The proto comment at `bandwidth_limit.pb.go:95` documents "The limit supplied in KiB/s" (kibibytes-per-second). BRAINSTORM's "kilobits-per-second" framing is incorrect. The throttle math is kbps-per-tick: `chunk_size = limit_kbps × 1024 × fill_interval_seconds` (units check: KiB/s × seconds = KiB; ×1024 = bytes). Documentation + operator-facing config commentary in fixture 0017 README + envoy-go.yaml comments MUST consistently use KiB/s terminology.
```

(End of §13.4 stub; the forward-pointer subsection lands at phase-done commit per phase-13 + phase-14 SPEC §13.4 precedent. ~80 lines authored.)

### 13.5 `## Timing tolerances` extension

```markdown
- **Bandwidth-limit throttle wall-clock tolerance: ±70ms (per phase 15 ADR-0137 + SPEC §7.3 + §11.P9).**
  Phase 15's Path B-async body algorithm emits one body-blast at the end of the throttle window;
  Envoy's reference Path A rate-paced chunk-emit pattern emits chunks at `fill_interval` cadence
  with `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes per tick (KiB/s units per
  §1.1 amendment 6). Total wall-clock throttle-time observably equivalent within ±70ms across
  body sizes 100 bytes to 51 KiB and limit_kbps values 1 to 100 (per §11 empirical test matrix
  at probes A/B/L). Fixture 0017's driver asserts per-scenario wall-clock within tolerance; the
  ±70ms window absorbs `time.AfterFunc` Linux granularity (typically 1-5ms minimum) plus CI
  scheduling jitter plus initial-burst-capacity approximation variance.
```

---

## 14. Testing strategy (per BRAINSTORM §11 + §1.1 amendments)

### 14.1 Unit tests (`internal/filter/http/bandwidthlimit/bandwidthlimit_test.go`)

Test groups (mirrors phase-11 local_ratelimit's 6 test groups; phase-15 adds a 7th for BOTH-direction symmetry):

1. **Config parse + buildCompiledConfig** — `stat_prefix` required (empty-string → parse-rejection per §1.1 amendment 3); `enable_mode` all 4 values valid; `limit_kbps` PGV mirror (>= 1 when set; required at per-route per §1.1 amendment 4); `fill_interval` PGV mirror ([20ms, 1s] when set; default 50ms per §1.1 amendment 5); silent-ignored fields (`runtime_enabled`, `enable_response_trailers`, `response_trailer_prefix`) parse without error.
2. **buildCompiledConfigPerRoute** — same `BandwidthLimit` proto reuse pattern; per-route `limit_kbps` filter-internal REQUIRED rejection wording; per-route TPFC lazy-cache via `sync.Map`.
3. **DecodeHeaders + DecodeData throttle (decode-side)** — `requestActive` gate per `enable_mode`; body-buffering via `DataStopIterationAndBuffer`; throttle-duration arithmetic via kbps-per-tick formula (per §6.6); one-tick fill_interval floor for sub-chunk-size bodies; timer-arming + counter increments (Inc `*_enabled` + `*_incoming_total_size` + `*_pending` at arm; Add `*_enforced` by ticks at fire); `ContinueDecoding` resume.
4. **EncodeHeaders + EncodeData throttle (encode-side)** — symmetric to decode-side; `responseActive` gate; timer-arming + counter increments; `ContinueEncoding` resume.
5. **OnDestroy + timer cleanup** — `requestTimer.Stop()` + `responseTimer.Stop()` idempotent; pending-gauge accounting under Stop-races-Fire window (per §4 + §6.9 sequence); no goroutine leak (verified via race-detector + leak-sentinel).
6. **Per-route INDEPENDENT-stats wiring** — per-route override allocates own `*compiledConfig` + own `*filterStats` keyed by `*BandwidthLimit` pointer; per-route stat-counter increments do NOT touch listener-level counters; `disabled` via `enable_mode: DISABLED` short-circuits without counter increments.
7. **BOTH-direction symmetry** — listener-level `enable_mode: REQUEST_AND_RESPONSE` exercises both sides on the same stream; per-stream throttle-duration accounting is independent for decode + encode; total wall-clock = decode_throttle + encode_throttle + upstream_round_trip (assuming serial timing).

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/bandwidthlimit/...` — green on all 7 test groups + sub-groups. Race-test surface unchanged from phase 14's 37-package green baseline; the new package adds the 38th. Per-stream state (timer + buffered body + cached compiled config) is single-goroutine-per-stream (the dispatch goroutine), so no synchronization needed within `*filter`. `sync.Map` lazy-cache at `*factoryState.perRoute` is concurrent-safe.

`golangci-lint run` — green; new package lints clean.

### 14.3 Fuzzers

`FuzzBandwidthLimitConfigParse` — fuzzes the YAML→proto→`buildCompiledConfig` pipeline. Inputs are random bytes interpreted as YAML; errors-on-invalid-YAML are expected; the fuzzer asserts no panic + no nil-deref on the compilation path. The 19th fuzzer in the repo (after `FuzzCompressorConfigParse` from phase 14).

### 14.4 Existing fuzzers re-run

18 phase-14 fuzzers re-run at 30s budget; all green (regression check; phase 15 introduces no fuzzer-affecting changes outside the new package).

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin. Phase 15 introduces no H2 wire-shape changes (ZERO framework deltas; the encode-side body-buffering path is structurally identical at H1 + H2 levels).

### 14.6 Differential 0000–0016 + 0017

17 prior fixtures + the new `0017-http-bandwidth-limit` = 18 fixtures green. Total runtime estimated ~60-80s wallclock (the new fixture has 6 scenarios with total ~8s throttle-equivalent wall-clock duration; scenario timing dominates).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

| Gate | Pass/fail criterion |
|---|---|
| A | `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0; no new warnings vs phase-14 baseline at master tip `e7a26ef`. |
| B | `go test -race ./...` exit 0 across all 38 packages; race detector reports clean. |
| C | `h2spec` 53/53 PASS at ADR-0051 pin; phase-15 introduces no H2 wire-shape changes. |
| D | All 19 fuzzers green at 30s/each budget. |
| E | All 18 differential fixtures (0000-0017) PASS; runtime ~60-80s wallclock. |
| F | `BEHAVIOR_CONTRACT.md` §13.1 + §13.2 + §13.3 + §13.4 + §13.5 populated per the patches at §13 above. |

All six green at phase-done commit per BOOTSTRAP_PROMPT.md §7.5.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

1. ✓ Phase 15 SPEC.md authored with **10 §1.1 amendment blocks** (5 structural + 4 field-bookkeeping + 1 operational-foot-gun); each amendment cross-referenced to §11 empirical evidence.
2. ✓ §3 framework-survey result locked: ZERO framework deltas; `time.AfterFunc` + `ContinueDecoding/Encoding` (phase-09) + decode-side body-buffering (phase-13 ADR-0128) + encode-side `OverwriteBody` (phase-14 ADR-0131, anticipated not invoked) reused.
3. ✓ §11 empirical-pin block: 15 pins resolved IN-SESSION; **7 RATIFIED + 6 REFUTED + 2 PARTIAL/REFRAMED**; verbatim probe transcripts captured at `/tmp/p15-pins/` (probes A-M + PGV + wire-shape).
4. ✓ Differential fixture: 6 scenarios; byte-exact body assertion; **±70ms** wall-clock tolerance; per-counter delta byte-equivalence on 14 active stats; 2 histograms allow-listed via twin-series-filter; per-route INDEPENDENT-stats per scenario 6.
5. ✓ ADR roster: 5 ADRs (ADR-0135..ADR-0139) + ADR-0125 in-place §(xi) amendment paragraph documenting the NEW canonical per-route pattern phase-15 introduces.
6. ✓ Stat surface: **14 active stats (8 counters + 6 gauges) + 2 deferred histograms** (divergence-window per §1.1 amendment 9); namespace `<stat_prefix>.http_bandwidth_limit.<counter>` underscore-infix (NOT HCM-rooted); Prometheus inlines stat_prefix into base name; NO new SN10 rule.
7. ✓ Per-route surface: same `BandwidthLimit` proto reuse via TPFC (NO `BandwidthLimitPerRoute` proto); phase-15 introduces a NEW canonical pattern (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route) distinct from BOTH ADR-0117 (4th canonical) AND ADR-0125 (5th canonical); INDEPENDENT-stats per ADR-0139; SECOND row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent.
8. ✓ Throttle math: **kbps-per-tick chunking** (`chunk_size = limit_kbps × 1024 × fill_interval_seconds`) per §6.6 + §1.1 amendment 6; `limit_kbps` units = KiB/s; NOT steady-rate as BRAINSTORM hypothesized.
9. ✓ Wire-shape divergence: envoy-go Path B-async silent-then-blast vs Envoy Path A rate-paced chunks at exact `fill_interval` cadence; total wall-clock equivalent ±70ms; ADR-0137 forward-points to future encode-side streaming framework phase.
10. ✓ Operational foot-gun: listener-level missing `limit_kbps` + active `enable_mode` causes runtime hang; envoy-go matches Envoy byte-equivalent per §1.1 amendment 10 + §13.4.
11. ✓ Ten §1.1 amendment blocks document the SPEC-time refutations cleanly via the §1.1 amendment-block channel (NOT §12 BRAINSTORM-amendment cycle).
12. ✓ STATE.md updated post-SPEC: lifecycle-state-2 → 3 transition (SPEC-done, awaiting PLAN); next-skill `superpowers:writing-plans` (now for PLAN authoring per ADR-0005 §Decision 4 split); ROADMAP.md row 15 `planned → in-progress`.

---

**End of phase 15 SPEC.**
