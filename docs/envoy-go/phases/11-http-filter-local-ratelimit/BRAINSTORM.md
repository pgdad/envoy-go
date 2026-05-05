# Phase 11 Brainstorm — `envoy.filters.http.local_ratelimit`

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-0 → 2 brainstorm session for phase 11 (`http-filter-local-ratelimit`), the FOURTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, and `header_mutation` at phase 10). The next session (lifecycle-state 2 → 3 for phase 11, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10 precedent) authors `docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-11-http-filter-local-ratelimit-brainstorm`, branch `phase-11-http-filter-local-ratelimit-brainstorm`, branched from master tip `97ed8b9` (the 10 phase-done REVIEW commit `phase 10: REVIEW — end-of-phase retrospective + N-1 carry-forward`). The 10 phase-done implementation commit `8e17e06` and its SHA-fill follow-up `2c80b30` precede the REVIEW; `97ed8b9` is the REVIEW-landing commit.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + MVP scope envelope + each major design decision via 11-question dialogue (Q1 filter selection, Q2 envelope, Q3 omnibus deferral shape, Q4 token-bucket primitive, Q5 package naming, Q6 per-route bucket isolation, Q7 stat surface, Q8 fixture scenarios, Q9 empirical pin set, Q10 ADR set, Q11 test scaffolding). The §9 family-row continuation is implicit — phase 09 set the precedent that subsequent §9 family-rows continue under the umbrella per ADR-0106; phase 10 confirmed the pattern at the second iteration. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0113), and the just-shipped phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md` section-for-section, reframed for the local_ratelimit scope and adapted for its specific surface area (no async-resume; first stateful per-route resource; new wrinkle is the token-bucket primitive itself + first-stateful-per-route discipline). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the 11 SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. The off-master branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (tip `fffe4b4`) holds advisory prework notes from a prior session that pivoted; per ADR-0106(e), this brainstorm consulted those notes as context but is the authoritative artefact and supersedes them.

---

## 1. Mission and scope confirmation (11 only)

ROADMAP row `11 | http-filter-local-ratelimit | 10 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 11 is the FOURTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 10 phase-done commit `8e17e06` (with follow-up `2c80b30` for SHA fill, REVIEW at `97ed8b9`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 58: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108). Phase 11 ships `local_ratelimit` as the FOURTH real filter — the canonical Envoy-style "local rate-limiting primitive" — and establishes the per-filter-phase pattern's fourth data point. It is also the FIRST production filter where per-route configuration implies independent stateful resources (cors / fault / header_mutation per-route is data-only; phase 11's per-route entries each own their own `tokenBucket`).

### 1.1 What 11 delivers as a self-contained whole

Phase 11 lands `envoy.filters.http.local_ratelimit` (the canonical Envoy local rate-limiting filter) under the 07.1 framework. Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):

1. **New `internal/filter/http/localratelimit/` package** owning the filter implementation. Package directory + Go package identifier are both `localratelimit` (no underscore) — diverges from the underscore-preserving directory pattern established by `header_mutation` (which used the directory name `header_mutation/`). Rationale: the filter's name reads more naturally as a single concept ("local rate-limiting") without the word-break, and the no-underscore form aligns with `cors` + `fault` directory naming where the proto type-name was already a single token. The departure is captured explicitly in ADR-0114 so future filters can apply either convention deliberately. Files mirror the `internal/filter/http/fault/` shape: `local_ratelimit.go` (filter type + factory + decode method + token-bucket primitive + runtimeConfig), `local_ratelimit_test.go` (unit tests), `fuzz_test.go` (the 15th fuzzer in the repo — `FuzzLocalRateLimitConfigParse`), `doc.go` (package overview + 5-consumed/14-deferred decomposition). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering `router.New`, `cors.New`, `envoygotest.New`, `fault.New`, `header_mutation.New` before the `httpReg.Freeze()` invocation) gains a sixth `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → cors → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`.

3. **Proto-config parsing** of `envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit`, the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 19 top-level fields (see §8 for the exhaustive deferral list). Phase 11 consumes 5 — `stat_prefix`, `token_bucket{max_tokens, tokens_per_fill, fill_interval}`, `status{code}` — and the per-route override container `LocalRateLimitPerRoute` (which carries the same `LocalRateLimit` body via TPFC, recursively re-parsed via `New`); the remaining 14 fields are silently ignored under the omnibus deferral discipline (ADR-0120; see §8.1).

4. **Token-bucket primitive (lazy refill, Option A).** The runtime mechanic: a `tokenBucket` struct holds `{maxTokens, tokensPerFill, fillInterval, mu sync.Mutex, tokens, lastRefillNs}`. On each `tryConsume()`: lock; compute `elapsedNs := time.Now().UnixNano() - lastRefillNs`; compute `refills := elapsedNs / int64(fillInterval)` (integer division); if `refills > 0`, add `refills * tokensPerFill` to `tokens`, cap at `maxTokens`, advance `lastRefillNs += refills * int64(fillInterval)`; if `tokens > 0`, decrement and return true; else return false. NO per-bucket goroutine, NO `time.Ticker`, NO signal channel. Time source is `time.Now().UnixNano()` which carries the monotonic component on Go ≥1.9 (per Go's `time` package documentation; wall-clock backward jumps do not break the math because UnixNano on Go ≥1.9 monotonically advances for `time.Now()`-derived values).

5. **Rate-limit decision in `DecodeHeaders`.** The filter resolves the most-specific `runtimeConfig` via 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) — phase-07.1 primitive. Increments `enabled` counter unconditionally. Calls `rc.bucket.tryConsume()`: if true, increments `ok` and returns `Continue`; if false, increments `rate_limited` AND `enforced` (lockstep under MVP per ADR-0118), invokes `cb.SendLocalReply(rc.statusCode, body, headers)`, and returns `StopIteration`. NO async-resume. NO encode-side state. `DecodeData` / `DecodeTrailers` / `Encode*` are pass-through. This filter reuses fault's request-side StopIteration + SendLocalReply primitives exactly — NO new framework primitive.

6. **Per-route bucket isolation as wholesale override.** Per the proto message `LocalRateLimitPerRoute` (which embeds a full `LocalRateLimit` body), each TPFC entry runs through `New` at config-load time; each `New` invocation allocates its own `runtimeConfig` + own `tokenBucket`. The 3-tier resolver picks the most-specific config per request; that config carries its own bucket pointer. The listener-level bucket is unused for routes that override. This falls out of ADR-0073's wholesale-override discipline already used by cors + fault + header_mutation — NO new framework primitive needed. Phase 11 is the FIRST production filter where per-route override implies independent **stateful** resources (cors's per-route is data-only — rules + flags; fault's `max_active_faults` is closure-shared atomic across the listener but not per-route distinct; header_mutation's per-route is data-only — mutation rules). ADR-0117 captures this as an ADR-0073 amendment paragraph: "wholesale-override extends to stateful resources without further framework support."

7. **Stat surface 22→26-name extension.** Four new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`: `http.<stat_prefix>.local_rate_limit.{enabled, ok, rate_limited, enforced}`. Under MVP (shadow-mode deferred), the invariant `enforced == rate_limited` holds at every step — they increment in lockstep. ADR-0118 captures the invariant + the natural-divergence point (when `filter_enforced` runtime-key support lands per the Runtime + hot restart family). Prometheus scrape form (subject to §9.P5 confirmation): `envoy_http_local_rate_limit_{enabled,ok,rate_limited,enforced}` with `envoy_http_conn_manager_prefix="<stat_prefix>"` label.

8. **Rate-limited response wire shape.** Status 429 default (`status.code` PGV-pinned `[400, 600)`); body literal `"local_rate_limited"` (18 bytes, no trailing newline — initial expectation, SPEC §9.P3 confirms verbatim); 4-header set lowercase wire-form (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy-go`); framing Content-Length (not chunked). Mechanism: `cb.SendLocalReply(rc.statusCode, body, headers)` followed by `StopIteration` from `DecodeHeaders` — same primitive fault's `abort` uses; same primitive cors short-circuits with on preflight.

Plus three artifact-level deliverables:

9. **Differential fixture `0013-http-local-ratelimit`** under `test/fixtures/0013-http-local-ratelimit/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising four scenarios (per §6.2 below). The fixture asserts response status, body byte-exact, header set lowercase wire-form, counter deltas via `/stats/prometheus` scrape equivalence, and per-route-tier independent bucket isolation. Includes a refill-timing scenario (scenario 3) with a ±20ms tolerance pin per §9.P7.

10. **`BEHAVIOR_CONTRACT.md` 4-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.fault` from phase 09 and `### envoy.filters.http.header_mutation` from phase 10): a NEW `### envoy.filters.http.local_ratelimit` subsection covering the 5-consumed / 14-ignored field map, the rate-limited response wire shape (status, body bytes, header set, framing), the `enforced == rate_limited` MVP invariant, and the per-route wholesale-override semantics. Plus the 22→26-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0013 with per-scenario tolerance discipline. Plus forward-pointer notes for the deferred field families (descriptors → global_ratelimit; runtime keys → Runtime + hot restart; cluster-replicated → xDS).

11. **Anticipated 7 ADRs (ADR-0114 through ADR-0120)** per §7 below. ADR-0113 is the highest-numbered ADR landed in phase 10; ADR-0114 is the next-free.

### 1.2 What 11 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the omnibus deferral discipline established in this brainstorm (Q3=A; ADR-0120 organizes the 14 fields by 7 family-clusters). The summary: descriptor-based per-key buckets, runtime-key gating, shadow mode, per-connection bucket lifecycle, cluster-replicated state, multi-stage limiting, X-RateLimit IETF headers, vh_rate_limits policy, and gRPC trailer mapping are all out-of-scope. None are blockers for closing row 11 phase-done; all 14 are silently ignored at config-load time (no warnings; faithful to the cors / fault / header_mutation deferral discipline).

### 1.3 Phase-done as a §9 family-row landing

Phase 11's phase-done commit closes ROADMAP row `11` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 11 is the FOURTH §9 family-row to land (after 07.1-cors, 09-fault, and 10-header_mutation). The next §9 family-row will be numbered `12` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 11 is **single-row at brainstorm time** — a cohesive ~500–700 LoC implementation slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split is:

- **11.1 = token-bucket primitive + listener-only filter MVP**: the `tokenBucket` type + lazy-refill `tryConsume` + listener-level `runtimeConfig` parsing + 4-counter stats + rate-limited response. Differential fixture covers listener-only scenarios (basic-allow, basic-rate-limited, refill-after-fill_interval).
- **11.2 = per-route TPFC + 4th fixture scenario**: per-route `LocalRateLimitPerRoute` parsing + bucket-pointer-isolation discipline + per-route-override fixture scenario.

This split mirrors phase 10's anticipated-but-unused split and the 08.1 (admin-endpoints) + 08.2 (graceful-drain) shape. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the modest LoC estimate (~500–700 impl + ~200 tests + ~50 fuzzer + ~200 fixture-Go-driver/backend + ~210 fixture-yaml/README = ~1200–1400 total when including yaml configs and README; ~1000 if counting Go code alone) and modest task count estimate (~12–16 tasks). Both estimates remain comfortably under ADR-0045's 1500 LoC / 25 task split-trigger upstream of either accounting.

### 1.5 Seed-stub alignment

Like phases 09 and 10, phase 11 has NO sibling SPEC stub — phase 11 enters fresh after the phase 10 close. The §9 family-children list at ROADMAP line 58 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `csrf`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 Provenance — relationship to the off-master prebrainstorm notes branch

The branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (tip `fffe4b4`, pushed to origin) carries a 269-line `PREBRAINSTORM_NOTES.md` written during a 2026-05-04 session that started a phase-11 brainstorm under the assumption phase 10 was open for `local_ratelimit`, then pivoted (the user already had an in-flight phase-10 brainstorm targeting `header_mutation` in a different worktree). Per ADR-0106(e), a future phase-11 brainstormer is FREE to either consult those notes or cold-start fresh from the §9 heading. THIS brainstorm consulted them as advisory context — every decision in this document was independently validated through the 11-question Q&A with the user — but THIS document is the authoritative artefact. Where the prebrainstorm notes diverge from this document, this document wins. Specifically: (a) Q5 settled package directory `localratelimit/` (no underscore) — the notes proposed the same; (b) Q3 settled omnibus deferral ADR shape — the notes proposed the same; (c) Q4 settled Option A lazy-refill token bucket — the notes proposed the same. The notes branch is left in place on origin as a historical record; it does not need merging.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

This section is the brainstorm's decision log. Each Decision states **what** is chosen, **why** that option vs. its alternatives, what **deferred-pin** obligations (if any) remain for SPEC-time empirical work, and what **ADR anchor** the SPEC author should expect. ADR numbering starts at **ADR-0114** (next-free; phase 10 closed at ADR-0113 per `DECISIONS.md`).

### 2.1 Filter package layout *(Decision 1 → ADR-0114)*

**Decision:** New package `internal/filter/http/localratelimit/` (directory + Go package identifier both `localratelimit`, no underscore) with files mirroring the cors + fault + header_mutation precedent: `local_ratelimit.go`, `local_ratelimit_test.go`, `fuzz_test.go`, `doc.go`. The package exports two top-level symbols: `TypeURL` (string constant, `"type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"`) and `New` (the `HTTPFilterFactory`). All other types (`filter`, `runtimeConfig`, `tokenBucket`, `filterStats`) are unexported. Filename underscores (`local_ratelimit.go`) match the proto v3 type-name's underscore form, mirroring `header_mutation.go` precedent — only the directory + package identifier drop the underscore.

**Why this vs. alternatives:**
- *Why directory `localratelimit/` (no underscore) instead of `local_ratelimit/` (mirroring header_mutation precedent)?* The user explicitly chose this naming in Q5 of the brainstorm dialogue. The name reads as a single concept ("local rate-limiting"); the no-underscore form aligns with `cors/` and `fault/` whose proto type-names were already single tokens. The pattern across §9 family-rows is now non-uniform (cors, fault, localratelimit no-underscore; header_mutation underscore-preserving) — ADR-0114 explicitly captures the rationale so future filter brainstorms can apply either convention deliberately rather than by accident.
- *Why not a single `internal/filter/http/local_ratelimit.go` flat file?* The existing per-filter discipline is unanimous (cors, fault, router, envoygotest, header_mutation each get their own subpackage). Subpackage isolation prevents future name collisions and is the project's convention.
- *Why not the Envoy-source-style path `internal/extensions/filters/http/local_ratelimit/`?* envoy-go is explicitly NOT mirroring Envoy's C++ source structure (`MISSION.md` §2.2 non-purpose). The `internal/filter/http/<name>/` pattern is the project's own convention.

**Deferred to SPEC:** the exact file split between `local_ratelimit.go` and any helper files (e.g. whether to factor `tokenBucket` into its own file `bucket.go`) — the SPEC author chooses based on test readability. No ADR-class commitment from brainstorm.

**ADR anchor:** ADR-0114 — Filter package shape conformance with cors + fault precedent + explicit departure rationale from header_mutation underscore-preserving directory pattern.

### 2.2 Extension-registry registration *(Decision 2 → ADR-0114 consequence)*

**Decision:** `cmd/envoy-go/main.go` adds a single new line `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` between the existing `header_mutation` registration (last `httpReg.Register` line per phase-10 boot ordering) and the `httpReg.Freeze()` call. The registration ordering is alphabetical-after-router per the ADR-0100 §2.2 convention codified at phase-09 brainstorm time: `router (first) → cors → envoy_go_test → fault → header_mutation → local_ratelimit`. Per ADR-0072, registration ordering does not affect runtime behavior; this is a stylistic discipline only. Phase 11 introduces NO `RegisterPerRouteValidator` hook (unlike phase 10's `header_mutation`) — per-route configs are independently valid (no multi-tier protected-set discipline like header_mutation's; each `runtimeConfig` validates standalone via the same `New` path).

**Why this vs. alternatives:**
- *Why not registration-order = config-list-order?* Registration order is a global discipline; config-list order is per-listener / per-route. Decoupling avoids cross-cutting coupling (already settled at phase-09 brainstorm time).
- *Why no per-route validator hook?* Phase 10's hook was driven by the multi-tier protected-header eager-validation requirement (which only surfaces when multiple tiers' configs interact). Phase 11's per-route configs are wholesale-override (Q6=A); each validates standalone at `New` time. No multi-tier interaction means no eager-validation hook.

**Deferred to SPEC:** none — the line edit is mechanical.

**ADR anchor:** ADR-0114 — folded into the package-shape ADR (the registration line is a one-line consequence of the package shape).

### 2.3 Proto-config parsing + `runtimeConfig` shape *(Decision 3 → ADR-0115)*

**Decision:** The `New` factory unmarshals `tc *anypb.Any` into a `*envoyextensionsfiltershttplocalratelimitv3.LocalRateLimit` value via `tc.UnmarshalTo(&cfg)`. The factory builds a long-lived `*runtimeConfig` value held by the factory closure, capturing:

```go
type runtimeConfig struct {
    statPrefix string         // from cfg.StatPrefix (PGV non-empty pin §9.P1)
    bucket     *tokenBucket   // closure-captured per filter-instance / per per-route entry
    statusCode int            // from cfg.Status.Code (default 429; PGV-pinned [400, 600))
    stats      *filterStats   // 4 counters scoped by stat_prefix
}

type filterStats struct {
    enabled, ok, rateLimited, enforced *atomic.Int64  // wired through internal/stats.Registry
}
```

The 14 deferred top-level fields (`descriptors`, `rate_limits`, `filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced`, `response_headers_to_add`, `local_rate_limit_per_downstream_connection`, `local_cluster_rate_limit`, `stage`, `enable_x_ratelimit_headers`, `vh_rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`, `rate_limited_as_resource_exhausted`) are silently ignored at unmarshal time — `cfg` carries them but `runtimeConfig` does not capture them. No warning, no reject. Faithful to ADR-0040 silent-ignore discipline (cors, fault, header_mutation precedent).

**Why this vs. alternatives:**
- *Why parse all 19 fields and silent-ignore 14, vs. parse only 5 and discard the rest?* The proto unmarshal is uniform — `tc.UnmarshalTo` parses all fields whether the filter consumes them or not. The decomposition is at `runtimeConfig` build-time, not unmarshal-time. This matches cors / fault / header_mutation's pattern.
- *Why not warn on unconsumed fields?* ADR-0040 silent-ignore — Envoy itself silent-ignores unknown-by-version fields during proto migration; envoy-go matches that semantics for in-version-but-not-implemented fields. Future shadow-mode / runtime-keys / descriptor-action phases extend the consumed set without requiring config migration.

**Deferred to SPEC:** PGV constraint enumeration for the 5 consumed fields — `stat_prefix` non-empty (§9.P1); `token_bucket.max_tokens > 0` (§9.P2a); `token_bucket.tokens_per_fill` default-or-positive (§9.P2b); `token_bucket.fill_interval` minimum (50ms doc claim — §9.P2c confirms actual minimum); `status.code ∈ [400, 600)` (§9.P4 — initial expectation 429 default).

**ADR anchor:** ADR-0115 — `runtimeConfig` shape + 5-consumed / 14-silent-ignore decomposition + PGV constraint table.

### 2.4 Token-bucket primitive *(Decision 4 → ADR-0116)*

**Decision:** Option A — lazy refill on access. Single `sync.Mutex` per bucket. NO per-bucket goroutine. NO `time.Ticker`. NO signal channel.

```go
type tokenBucket struct {
    maxTokens, tokensPerFill int64
    fillInterval             time.Duration

    mu           sync.Mutex
    tokens       int64
    lastRefillNs int64  // time.Now().UnixNano() at last refill
}

func (b *tokenBucket) tryConsume() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    nowNs := time.Now().UnixNano()
    elapsedNs := nowNs - b.lastRefillNs
    refills := elapsedNs / int64(b.fillInterval)  // integer division
    if refills > 0 {
        b.tokens += refills * b.tokensPerFill
        if b.tokens > b.maxTokens {
            b.tokens = b.maxTokens
        }
        b.lastRefillNs += refills * int64(b.fillInterval)
    }
    if b.tokens > 0 {
        b.tokens--
        return true
    }
    return false
}
```

Time source: `time.Now().UnixNano()` carries the monotonic component on Go ≥1.9 (per Go's `time` package documentation: time values returned by `time.Now()` carry monotonic clock readings; arithmetic on `t.UnixNano()` derived values is monotonic for `t1, t2` from `time.Now()` regardless of wall-clock changes). Wall-clock backward jumps do not break the math. Initial state: `tokens = maxTokens`, `lastRefillNs = time.Now().UnixNano()` set at `New` time.

**LBP-1 succession bookkeeping.** Per BOOTSTRAP_PROMPT.md §6.1 / phase 06.1's invariant, LBP-1 (lock-free / closure-captured runtime config) is the project's hot-path discipline. The token bucket is closure-captured by the `runtimeConfig` and the `runtimeConfig` is closure-captured by the filter factory — that part is consistent with LBP-1. However, the bucket's hot path uses one `sync.Mutex` rather than being lock-free. The SPEC author resolves the LBP-1 succession bookkeeping one of two ways: (a) tighten LBP-1 to "closure-captured" without the lock-free clause, recording phase 11 as the Nth application; or (b) scope phase 11 as LBP-1-adjacent (closure-capture pattern follows LBP-1; mutex hot-path is acknowledged divergence from the lock-free clause). ADR-0116 resolves this explicitly.

**Why this vs. alternatives:**
- *Why Option A (lazy refill)?* Matches Envoy's own implementation (Envoy's local_ratelimit is non-blocking, no per-bucket thread). No per-bucket teardown plumbing needed (mirrors fault's `time.AfterFunc` cancel-on-OnDestroy mechanics from ADR-0102 but without the timer state). Integer arithmetic is exact under monotonic time.
- *Why not Option B (`time.Ticker` per bucket)?* Per-bucket goroutine + cancel-on-OnDestroy plumbing is fragile when bucket lifetime spans listener drain + per-route bucket reaping. Heavier than needed.
- *Why not Option C (signal-channel)?* Wrong shape vs. Envoy's non-blocking local_ratelimit; introduces blocking semantics not present in the source.

**Deferred to SPEC:** §9.P2c (`fill_interval` minimum — observe actual; 50ms doc claim); §9.P7 (refill timing tolerance — initial expectation ±20ms wider than fault's ±10ms because integer division on `elapsed / fill_interval` quantizes refill instants); LBP-1 succession decision (a) vs. (b) above.

**ADR anchor:** ADR-0116 — `tokenBucket` primitive, lazy-refill mechanics, monotonic-time semantics, and LBP-1 succession bookkeeping.

### 2.5 Per-route bucket isolation *(Decision 5 → ADR-0117 + ADR-0073 amendment)*

**Decision:** Wholesale override per ADR-0073's existing per-route discipline. Each `LocalRateLimitPerRoute.rate_limit` (which carries a full `LocalRateLimit` body) runs through `New` at config-load time; each `New` invocation allocates its own `runtimeConfig` + own `tokenBucket`. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration) picks the most-specific config per request; that config carries its own bucket pointer. The listener-level bucket is unused for routes that override. NO new framework primitive needed; this falls out of how ADR-0073 already works for cors / fault / header_mutation.

Phase 11 is the FIRST production filter where per-route override implies independent **stateful** resources (cors's per-route is data-only — rules + flags; fault's `max_active_faults` is closure-shared atomic across the listener but not per-route distinct; header_mutation's per-route is data-only — mutation rules). ADR-0117 captures this as an ADR-0073 amendment paragraph: "wholesale-override extends to stateful resources without further framework support."

**Why this vs. alternatives:**
- *Why wholesale override (independent buckets)?* Falls out of ADR-0073's existing semantics — no new framework primitive. Matches Envoy's actual local_ratelimit per-route behavior (best knowledge; SPEC §9.P6 confirms via empirical pin).
- *Why not hierarchical AND (multi-bucket consumption: request rate-limited if EITHER listener bucket OR per-route bucket runs dry)?* Not Envoy's wire behavior for local_ratelimit. Would force a new framework primitive (multi-bucket consumption ordering). Avoided.

**Deferred to SPEC:** §9.P6 (per-route override observability — confirm independent counter increments via `/stats/prometheus` scrape under per-route stricter bucket).

**ADR anchor:** ADR-0117 — Per-route bucket isolation as a consequence of ADR-0073's wholesale-override (first stateful per-route filter; ADR-0073 amendment paragraph).

### 2.6 Stat surface 22→26-name extension *(Decision 6 → ADR-0118)*

**Decision:** Four new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`: `http.<stat_prefix>.local_rate_limit.{enabled, ok, rate_limited, enforced}`. All four wired through `internal/stats.Registry` via per-instance `*atomic.Int64` slots in `filterStats`. The 22-name table from phase 09 (which extended 17→22) and unchanged through phase 10 grows to 26 names.

Increment discipline:
- `enabled`: every request reaching the filter.
- `ok`: request not rate-limited (`tryConsume` → true).
- `rate_limited`: request rate-limited (`tryConsume` → false).
- `enforced`: request rate-limited AND enforced (under MVP, every rate-limited request is enforced; `enforced == rate_limited` invariant).

**MVP invariant:** `enforced == rate_limited` at every step. Future shadow-mode phase widens: when `filter_enforced` runtime-key support lands, `enforced ≤ rate_limited` becomes the post-shadow-mode invariant, with the gap recorded by `request_headers_to_add_when_not_enforced` (also deferred per ADR-0120 omnibus). ADR-0118 captures the invariant + the natural-divergence point.

Prometheus scrape form (subject to §9.P5 confirmation): `envoy_http_local_rate_limit_{enabled,ok,rate_limited,enforced}` with `envoy_http_conn_manager_prefix="<stat_prefix>"` label. Per-route configs use the same stat_prefix (PGV-required); per-route counters increment independently of listener counters (§9.P6 confirms).

**Why this vs. alternatives:**
- *Why all 4 counters including `enforced`?* Differential parity with reference Envoy on `/stats/prometheus`; permanently-equal counter under MVP mirrors phase 09's `fault.response_rl_injected` permanently-zero parity stat.
- *Why not 3 counters (drop `enforced`)?* Would degrade Envoy-faithful contract; SPEC's twin-series filter discipline (BEHAVIOR_CONTRACT mapping rules SN1–SN8) handles asymmetry but introduces a divergence-from-Envoy-wire-form claim. Avoided.

**Deferred to SPEC:** §9.P5 (stat names on the wire — `/stats/prometheus` exact form + `envoy_http_conn_manager_prefix` label confirmation; document any `local_rate_limit_*` family Envoy emits that envoy-go does NOT, applying twin-series filter discipline per BEHAVIOR_CONTRACT mapping rules SN1–SN8).

**ADR anchor:** ADR-0118 — `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension + `enforced == rate_limited` MVP invariant + future shadow-mode widening point.

### 2.7 Rate-limited response mechanics *(Decision 7 → ADR-0119)*

**Decision:** Status 429 default; body literal `"local_rate_limited"` (18 bytes, no trailing newline — initial expectation, §9.P3 confirms verbatim); 4-header set lowercase wire-form (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy-go`); framing Content-Length (not chunked). Mechanism: `cb.SendLocalReply(rc.statusCode, body, headers)` followed by `StopIteration` from `DecodeHeaders` — same primitive fault's `abort` uses; same primitive cors short-circuits with on preflight.

PGV-pin: `status.code ∈ [400, 600)` per §9.P4 (initial expectation; SPEC author confirms).

**Why this vs. alternatives:**
- *Why `SendLocalReply` + `StopIteration` mechanics?* Reuses fault's request-side terminal-replace pattern verbatim. NO new framework primitive.
- *Why body literal `"local_rate_limited"` initial expectation?* Mirrors fault's `"fault filter abort"` (18 bytes) — same length suggestive of Envoy's "filter-name-derived" body convention. SPEC §9.P3 confirms exact bytes.

**Deferred to SPEC:** §9.P3 (rate-limited response wire shape — verbatim status, body bytes, header set, framing); §9.P4 (default status code — 429 expectation); §9.P8 (header set on success — expect no headers added; X-RateLimit-* deferred per ADR-0120).

**ADR anchor:** ADR-0119 — Rate-limited response mechanics + body byte-exact + 4-header set + status-text mapping + 429 default + `SendLocalReply` reuse.

### 2.8 Omnibus deferral of 14 fields *(Decision 8 → ADR-0120)*

**Decision:** A single ADR-0120 captures the deferral of all 14 unconsumed top-level fields, organized by 7 family-clusters. This mirrors ADR-0089's omnibus admin-endpoint deferral pattern (rather than phase 09's per-field approach where genuinely independent deferrals each got their own paragraph). Phase 11's deferrals cluster more cleanly than phase 09's; one ADR with 7 subsections is structurally clearer than 14 separate paragraphs.

Family-clusters (each → one ADR-0120 subsection):

1. **Descriptor-action subsystem** — `descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`. Couples to `global_ratelimit` future phase (descriptor-action dispatch is shared infrastructure between local and global rate-limit families).
2. **Runtime + shadow-mode subsystem** — `filter_enabled` (RuntimeFractionalPercent runtime-key gating), `filter_enforced` (shadow-mode toggle), `request_headers_to_add_when_not_enforced` (shadow-mode header injection). Couples to Runtime + hot restart family.
3. **xDS cluster-state subsystem** — `local_cluster_rate_limit` (clustered/replicated bucket via xDS cluster-state replication). Couples to xDS / dynamic config family.
4. **Response-side header injection** — `response_headers_to_add` (rate-limit response header injection). Standalone follow-on.
5. **Per-connection lifecycle** — `local_rate_limit_per_downstream_connection` (per-connection bucket; different lifecycle that couples with filter chain teardown). Standalone follow-on.
6. **Multi-stage limiting** — `stage`. Couples to descriptor-action subsystem.
7. **X-RateLimit headers + vh policy** — `enable_x_ratelimit_headers` (IETF X-RateLimit-Limit/Remaining/Reset headers), `vh_rate_limits` (virtual-host rate-limit policy override). Standalone follow-ons.
8. **gRPC trailer mapping** — `rate_limited_as_resource_exhausted` (gRPC RESOURCE_EXHAUSTED trailer). Couples to gRPC family.

(Note: 14 fields, 8 family-clusters as enumerated; the "7 family-clusters" framing in the design summary reflects merging clusters 4 + 7 into "response-side header / vh-policy" since they share the response-injection theme. ADR-0120's exact subsection count is 7 or 8 — SPEC author chooses the consolidation level.)

**Why this vs. alternatives:**
- *Why omnibus (one ADR with subsections) vs. per-field (one ADR per cluster)?* Phase 09's per-field approach was driven by genuinely independent deferrals (header_delay coupled with header_abort but other fields didn't cluster). Phase 11's deferrals cluster cleanly into 7–8 family-themes. One ADR with subsections per family is structurally clearer than 7–8 paragraph stubs and reduces ADR-0040 noise: ADR-0089 set the omnibus precedent; using it here keeps `DECISIONS.md` navigable.
- *Why not hybrid (omnibus for descriptor-action; per-field for everything else)?* Hybrid creates an asymmetric reading order in `DECISIONS.md`. Omnibus is uniform.

**Deferred to SPEC:** SPEC author may consolidate or split subsections as ADR-0120 is drafted; the brainstorm's anticipated 7–8 subsection count is anticipatory.

**ADR anchor:** ADR-0120 — OMNIBUS deferral of 14 LocalRateLimit fields, organized by 7–8 family-clusters per the §8 subsection list.

### 2.9 Family-expansion shape *(Decision 9 → settled by ADR-0106)*

**Decision:** Phase 11's row lands as flat top-level row `11` per ADR-0106 — NOT as a sub-phase of any §9 parent. ADR-0106 (drafted at phase 09 brainstorm time, codified at phase 09 SPEC time, validated at phase 09 + 10 phase-done) is now settled discipline. No new ADR needed.

The §9 family heading at `ROADMAP.md` line 56 stays unchanged across phase 11's landing. The next §9 family-row will be numbered `12`. The brainstormer's choice from the §9 family list (`ROADMAP.md` line 58) applies for phase 12 — no sibling-stub authored here.

**ADR anchor:** none new; ADR-0106 cited.

---

## 3. Surface inventory (11 only)

### 3.1 New files (created in 11)

```
internal/filter/http/localratelimit/doc.go                   (~30 LoC; package overview + 5-consumed/14-deferred decomposition)
internal/filter/http/localratelimit/local_ratelimit.go       (~500-700 LoC; filter type + factory + tokenBucket + runtimeConfig + DecodeHeaders + filterStats wiring)
internal/filter/http/localratelimit/local_ratelimit_test.go  (~200 LoC; 5 test groups per §6.5)
internal/filter/http/localratelimit/fuzz_test.go             (~50 LoC; FuzzLocalRateLimitConfigParse — 15th fuzzer)

test/fixtures/0013-http-local-ratelimit/envoy.yaml           (~80 LoC; reference Envoy v1.37.2 STRICT_DNS, 2 listeners, 4 scenarios via separate runs)
test/fixtures/0013-http-local-ratelimit/envoy-go.yaml        (~80 LoC; envoy-go STATIC, same 2 listeners)
test/fixtures/0013-http-local-ratelimit/backend/main.go      (~30 LoC; simple HTTP backend echoing 200)
test/fixtures/0013-http-local-ratelimit/driver/main.go       (~150 LoC; 4-scenario orchestration; exercises basic-allow, basic-rate-limited, refill-after-fill_interval, per-route-override)
test/fixtures/0013-http-local-ratelimit/expectations.yaml    (~50 LoC; per-scenario counter delta + body byte-exact + header set + tolerance)
test/fixtures/0013-http-local-ratelimit/README.md            (~50 LoC; SPEC §7.1 narrative)

docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md  (~600-900 LoC; written next session)
docs/envoy-go/phases/11-http-filter-local-ratelimit/PLAN.md  (~400-600 LoC; written after SPEC)
docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md   (~50 LoC at start; grows during impl)
docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md     (~300-500 LoC at phase-done)
```

### 3.2 Modified files (in 11)

```
cmd/envoy-go/main.go                  (+1 line: localratelimit.Register before httpReg.Freeze)
docs/envoy-go/DECISIONS.md            (+ADR-0114..ADR-0120 + ADR-0073 amendment paragraph per ADR-0117)
docs/envoy-go/BEHAVIOR_CONTRACT.md    (4-edit bundle: new ### envoy.filters.http.local_ratelimit subsection; 22→26 stat table; new equivalence-matrix row; forward-pointer notes)
docs/envoy-go/ROADMAP.md              (new row 11; status flips planned → in-progress at SPEC commit; in-progress → done at phase-done commit)
docs/envoy-go/STATE.md                (advance per phase lifecycle; last-commit + last-updated SHA-fill at each commit)
```

### 3.3 Untouched files (load-bearing absence)

```
internal/filter/http/perroute.go      (existing 3-tier Resolve; phase 11 reuses; NO ResolveAllTiers needed unlike phase 10)
internal/filter/http/registry.go      (existing extension registry + Freeze; phase 11 adds one Register call site upstream)
internal/filter/http/cors/            (untouched)
internal/filter/http/fault/           (untouched; reused as the filter-package-shape reference)
internal/filter/http/header_mutation/ (untouched; phase 11 explicitly diverges from its underscore-preserving directory pattern per ADR-0114)
internal/filter/http/router/          (untouched)
internal/filter/http/envoygotest/     (untouched)
internal/filter/hcm/                  (untouched; HCM stays the chain runner)
internal/stats/                       (untouched; phase 11 wires through existing Registry primitives)
internal/listener/                    (untouched)
internal/cluster/                     (untouched)
internal/admin/                       (untouched)
internal/drain/                       (untouched)
```

---

## 4. Iteration-state coverage map

### 4.1 Synchronous request-side filter with terminal-replace path

Phase 11's filter exercises two iteration states from the 07.1 framework:

- **Continue (decode-side, allow path):** `DecodeHeaders` returns `Continue` after `tryConsume` → true. `DecodeData` / `DecodeTrailers` are pass-through (no buffering, no inspection). `Encode*` is pass-through (no encode-side state).
- **StopIteration + SendLocalReply (decode-side, rate-limited path):** `DecodeHeaders` returns `StopIteration` after `tryConsume` → false; before returning, calls `cb.SendLocalReply(429, body, headers)`. Filter chain terminates at the decode-side; no upstream cluster involvement; `Encode*` is never called for the rate-limited path.

NO async-resume primitive (unlike fault's `delay`). NO encode-side state (unlike header_mutation's response mutations). NO buffer-until-body-complete (unlike future buffer/compression filters).

### 4.2 Coverage relative to fault + cors + header_mutation

- **vs. cors:** cors short-circuits via SendLocalReply on preflight (decode-side terminal-replace) and injects response headers on non-preflight (no encode iteration). Phase 11 covers the same SendLocalReply pattern, scaled to a non-preflight rate-limit decision.
- **vs. fault:** fault exercises `delay` async-resume (decode-side `StopIteration` + `time.AfterFunc` + `cb.ContinueDecoding`) AND `abort` terminal-replace (decode-side `SendLocalReply` + `StopIteration`). Phase 11 reuses fault's `abort` path verbatim — same primitive, different decision criterion (token-bucket vs. percentage-roll).
- **vs. header_mutation:** header_mutation exercises encode-side iteration with state mutation (`EncodeHeaders` non-error path) AND multi-tier per-route evaluation. Phase 11 exercises NEITHER — encode is pass-through; per-route is wholesale-override via existing `Resolve`.
- **NEW coverage in phase 11:** first stateful per-route resource (independent `tokenBucket` per per-route TPFC entry, validated by direct address inspection in unit tests).

---

## 5. Token-bucket primitive — phase 11 specifics

### 5.1 Primitive boundary

The `tokenBucket` type is unexported, lives in `internal/filter/http/localratelimit/local_ratelimit.go` (or a sibling file `bucket.go` if SPEC author splits for readability), and exposes a single method `tryConsume() bool`. It is closure-captured by `runtimeConfig`; `runtimeConfig` is closure-captured by the filter factory. Bucket lifetime spans the filter-instance lifetime — for the listener-level config, the bucket is born at boot's `New` call and dies at process exit; for per-route entries, the bucket is born at config-load and dies at the next config reload (xDS family — out-of-scope for phase 11 since envoy-go's static-only config means buckets last until process exit).

### 5.2 Concurrency model

Single `sync.Mutex` per bucket. Hot-path callers hold the lock for the duration of the elapsed-compute + token-decrement (5–10 nanoseconds typical). Lock contention is bounded by request rate per route (per-route TPFC isolation means listener-level traffic and per-route traffic don't compete on the same bucket).

NO `atomic` arithmetic on the token count — the `elapsed → refills` computation is multi-step (compute, conditional update, advance lastRefillNs) and not amenable to a single CAS. Mutex is the natural choice. ADR-0116 records this as the deliberate departure from LBP-1's lock-free clause; the closure-capture aspect of LBP-1 IS preserved.

### 5.3 Time-source semantics

`time.Now().UnixNano()` returns nanoseconds since Unix epoch, but on Go ≥1.9 the underlying `time.Time` value carries a monotonic clock reading; arithmetic across `time.Now()` calls advances monotonically even under wall-clock backward jumps (NTP corrections, leap seconds). The `lastRefillNs` field is therefore safe to compare against future `time.Now().UnixNano()` calls without wall-clock concerns. Unit tests exercise this assumption explicitly via a small mock or direct `time.Now()` call sequencing in `local_ratelimit_test.go`.

### 5.4 Empirical pin §9.P7

The refill timing tolerance — initial expectation ±20ms wider than fault's ±10ms — is the only timing-dependent primitive in phase 11. Integer division `elapsedNs / int64(fillInterval)` quantizes refill instants by `fillInterval`; for `fillInterval=200ms`, a request arriving 199ms after the last refill sees no new tokens, while a request 201ms after sees one fresh token. Differential equivalence with reference Envoy depends on this quantization being identical across implementations. SPEC §9.P7 measures actual reference Envoy behavior and pins the tolerance.

---

## 6. Differential fixture 0013-http-local-ratelimit — scenarios + driver shape

### 6.1 Fixture topology

Two listeners in one fixture (mirroring phase 10's 0012 dual-listener layout):

- **`l_basic`** — exercises listener-only rate-limiting. Three of four scenarios (basic-allow, basic-rate-limited, refill-after-fill_interval) use this listener via different `LocalRateLimit` configs across separate driver runs (the driver tears down and re-spawns Envoy + envoy-go between scenarios for state-reset, mirroring fault's per-scenario teardown discipline).
- **`l_per_route`** — exercises per-route TPFC override. One scenario (per-route-override) uses this listener with two routes: `/strict` (TPFC `LocalRateLimitPerRoute` with bucket cap 1) and `/loose` (no TPFC, falls through to listener bucket cap 10).

Both reference Envoy and envoy-go run identical configs (modulo cluster discovery: STRICT_DNS for reference, STATIC for envoy-go per the project convention).

### 6.2 Scenarios

| # | Name | Config | Workload | Asserts |
|---|---|---|---|---|
| 1 | basic-allow | bucket cap 10, per-fill 10, interval 1s | 5 reqs back-to-back via `l_basic` | 5x 200; counter deltas `enabled=5, ok=5, rate_limited=0, enforced=0`; `/stats/prometheus` scrape equivalence |
| 2 | basic-rate-limited | bucket cap 2, per-fill 2, interval 60s | 5 reqs back-to-back via `l_basic` | first 2x 200, last 3x 429; counter deltas `enabled=5, ok=2, rate_limited=3, enforced=3`; rate-limited response body byte-exact + 4-header set lowercase wire-form + Content-Length framing |
| 3 | refill-after-fill_interval | bucket cap 1, per-fill 1, interval 300ms | 3 reqs at t=0/100ms/400ms via `l_basic` | t=0 → 200, t=100ms → 429, t=400ms → 200; **±20ms tolerance per §9.P7** on the t=400ms boundary |
| 4 | per-route-override | listener bucket cap 10; route `/strict` TPFC bucket cap 1 (per-fill 1, interval 60s); route `/loose` no TPFC | 3 reqs each route, interleaved | `/strict`: 1x 200 + 2x 429; `/loose`: 3x 200; per-route stat-prefix counters increment **independently** of listener counters per §9.P6 |

### 6.3 Driver shape

Single Go binary `test/fixtures/0013-http-local-ratelimit/driver/main.go` orchestrates all four scenarios in sequence:

1. For each scenario: spawn reference Envoy + envoy-go on disjoint port pairs; wait for `/ready`; issue scenario workload; scrape `/stats/prometheus` from each; compare counter deltas + response status + response body + response header set per `expectations.yaml`; tear down.
2. Per-scenario teardown enforces state-reset — token-bucket state does not leak between scenarios. (Phase 09's fault driver established this discipline; phase 10 inherited; phase 11 continues.)
3. Tolerance handling: scenario 3's t=400ms boundary uses ±20ms wallclock tolerance; the driver enforces this by retrying-with-deadline rather than racing.

### 6.4 Header-allow-list extensions

Inheriting phase 10's three-iteration body-strip allow-list lessons:

- Baseline proxy-injected headers: `x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*` (carry-forward from phase 09 / 10).
- Rate-limited path adds (per scenario 2): `date` (timestamp drift; per-scenario allow-list, not global).
- Allow path adds: nothing (`enable_x_ratelimit_headers` defaults OFF; deferred per ADR-0120; §9.P8 confirms no headers added).
- Reference Envoy may inject `connection: close` on the rate-limited path under certain conditions (HTTP/1.1 hop-by-hop semantics); SPEC author validates and adds to allow-list if observed.

### 6.5 Timing tolerance

Scenario 3 is the only timing-dependent scenario. ±20ms tolerance is wider than fault's ±10ms because token-bucket integer division on `elapsed / fill_interval` quantizes refill instants — a request 1ms before vs. 1ms after a quantum boundary sees different bucket states. SPEC §9.P7 pins the empirical tolerance against reference Envoy v1.37.2 measurement.

---

## 7. Anticipated ADRs (ADR-0114 through ADR-0120)

Phase 10 closed at ADR-0113. Phase 11 anticipates 7 ADRs:

| Slot | ID | Title |
|---|---|---|
| 1 | ADR-0114 | Filter package shape `internal/filter/http/localratelimit/` (no underscore) + registration ordering. Includes rationale for departing from header_mutation's underscore-preserving directory pattern. |
| 2 | ADR-0115 | `runtimeConfig` shape + 5-consumed / 14-silent-ignore decomposition + PGV constraints (resolves §9.P1, §9.P2, §9.P4). |
| 3 | ADR-0116 | `tokenBucket` primitive + Option-A lazy-refill mechanics + monotonic-time semantics + LBP-1 succession bookkeeping (resolves §9.P2c PGV minimum, §9.P7 tolerance). |
| 4 | ADR-0117 | Per-route bucket isolation as a consequence of ADR-0073's wholesale-override (first stateful per-route filter; ADR-0073 amendment paragraph) (resolves §9.P6). |
| 5 | ADR-0118 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension + `enforced == rate_limited` MVP invariant + future shadow-mode widening point (resolves §9.P5). |
| 6 | ADR-0119 | Rate-limited response mechanics + body byte-exact + 4-header set + status-text mapping + 429 default + `SendLocalReply` reuse (resolves §9.P3, §9.P4, §9.P8). |
| 7 | ADR-0120 | OMNIBUS deferral of 14 local_ratelimit fields, organized by 7–8 family-clusters: descriptor-action; Runtime + shadow-mode; xDS cluster-state; response-side header injection; per-connection lifecycle; multi-stage; X-RateLimit headers + vh policy; gRPC trailer. |

Each pin maps to ≥1 ADR; each ADR cites the pin(s) it resolves. SPEC author may consolidate (e.g., fold ADR-0118 into a subsection of BEHAVIOR_CONTRACT inline, mirroring phase-10's Slot 7 consolidation per SPEC §8.1) but the BRAINSTORM's anticipated count stays 7.

---

## 8. Out-of-scope deferrals (omnibus per ADR-0120)

Per Q3=A (omnibus deferral) and Decision 8 above, all 14 unconsumed top-level fields are deferred under a single ADR-0120 organized by family-cluster. Per ADR-0040 silent-ignore discipline, none generate config-load warnings; all are silently absent from `runtimeConfig`.

### 8.1 Cluster 1 — Descriptor-action subsystem (4 fields)

- `descriptors` — descriptor-based per-key buckets; opens descriptor-action dispatch shared with `global_ratelimit`. Couples to global_ratelimit future phase.
- `rate_limits` — `RateLimitAction` list; descriptor-action dispatch; couples with `descriptors`.
- `always_consume_default_token_bucket` — descriptor-coupled (controls whether the default bucket is consumed when no descriptor matches).
- `max_dynamic_descriptors` — descriptor-coupled (cap on dynamically-allocated per-key buckets).

### 8.2 Cluster 2 — Runtime + shadow-mode subsystem (3 fields)

- `filter_enabled` (`RuntimeFractionalPercent`) — runtime-key gating; allows percentage-based filter activation. Couples to Runtime + hot restart family.
- `filter_enforced` (`RuntimeFractionalPercent`) — shadow mode; controls whether rate-limit decisions short-circuit (`enforced == rate_limited`) or merely log (`enforced < rate_limited`).
- `request_headers_to_add_when_not_enforced` — shadow-mode header injection on the upstream path when a rate-limit decision was made but not enforced.

### 8.3 Cluster 3 — xDS cluster-state subsystem (1 field)

- `local_cluster_rate_limit` — clustered/replicated bucket; bucket state replicated across instances via xDS cluster-state. Couples to xDS / dynamic config family.

### 8.4 Cluster 4 — Response-side header injection (1 field)

- `response_headers_to_add` — rate-limit response header injection (added to the 429 response).

### 8.5 Cluster 5 — Per-connection lifecycle (1 field)

- `local_rate_limit_per_downstream_connection` — per-connection bucket (different lifecycle; couples with filter chain teardown for state cleanup).

### 8.6 Cluster 6 — Multi-stage limiting (1 field)

- `stage` — multi-stage limiting (allows ordered application of multiple rate-limit filters in a chain). Couples to descriptor-action subsystem.

### 8.7 Cluster 7 — X-RateLimit headers + vh policy (2 fields)

- `enable_x_ratelimit_headers` — IETF X-RateLimit-Limit / X-RateLimit-Remaining / X-RateLimit-Reset response header injection.
- `vh_rate_limits` — virtual-host rate-limit policy override (interaction with vh-level rate-limit configuration).

### 8.8 Cluster 8 — gRPC trailer mapping (1 field)

- `rate_limited_as_resource_exhausted` — gRPC RESOURCE_EXHAUSTED trailer mapping for gRPC-over-HTTP/2 rate-limit responses. Couples to gRPC family.

(SPEC author may consolidate clusters 4 + 7 into a "response-side injection / vh policy" cluster, yielding 7 subsections instead of 8. The brainstorm leaves the cardinality flexible.)

---

## 9. Empirical-pin obligations for SPEC author (resolved against Envoy v1.37.2)

The SPEC author runs reference Envoy v1.37.2 (the `ENVOY_TARGET.md`-pinned image) against minimal `local_ratelimit` configs and captures verbatim observations. Per ADR-0004, all 8 pins are resolved IN-SESSION at SPEC-drafting time — NOT deferred to later sessions.

### 9.1 P1 — `stat_prefix` PGV

Configure `local_ratelimit` with `stat_prefix` MISSING. Observe whether reference Envoy:
1. Rejects at config-load (boot-time error)?
2. Accepts with empty stat_prefix?
3. Emits a load-time warning?

Capture the verbatim error message shape. Initial expectation: **reject at boot** (PGV non-empty constraint typical).

### 9.2 P2 — `token_bucket` PGV

Three sub-pins:

- **§9.P2a:** `max_tokens=0` — reject? accept-and-no-tokens-ever? Initial expectation: reject (PGV `gt: 0`).
- **§9.P2b:** `tokens_per_fill=0` (or omitted; defaults documented at 1) — reject, default to 1, or accept zero? Initial expectation: default to 1 if omitted; reject if explicitly 0.
- **§9.P2c:** `fill_interval` minimum — Envoy doc claims 50ms minimum. Configure 10ms, 20ms, 49ms, 50ms, 51ms; capture which are rejected vs. accepted. Initial expectation: 50ms minimum enforced; sub-50ms rejected.

### 9.3 P3 — Rate-limited response wire shape

Trigger rate-limit (cap=1, per-fill=1, interval=60s; first req succeeds, second triggers). Capture verbatim:
- Status line (HTTP/1.1 form expected: `HTTP/1.1 429 Too Many Requests`).
- Body bytes (initial expectation: literal `"local_rate_limited"`, 18 bytes, no trailing newline).
- Header set (lowercase wire-form, exact ordering — capture both sets and sort for comparison).
- Framing (Content-Length vs. chunked).

Initial expectation: 429, body literal `"local_rate_limited"` (18 bytes), 4-header set similar to fault's abort response (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy-go`). SPEC author confirms or amends.

### 9.4 P4 — Default `status.code`

Configure `local_ratelimit` with `status` field OMITTED. Trigger rate-limit. Capture response status code. Initial expectation: **429**.

### 9.5 P5 — Stat names on the wire

Configure `local_ratelimit` with `stat_prefix=test`. Apply small load (5 reqs, cap=10, all should succeed). Scrape `/stats/prometheus`. Capture verbatim:

- Exact stat-name form including the `envoy_http_conn_manager_prefix` label.
- Full set of stats Envoy emits with `local_rate_limit_*` substring.

Initial expectation: `envoy_http_local_rate_limit_{enabled,ok,rate_limited,enforced}` with `envoy_http_conn_manager_prefix="test"` label. Document twin-series filter discipline for any `local_rate_limit_*` family Envoy emits that envoy-go does NOT (per BEHAVIOR_CONTRACT mapping rules SN1–SN8).

### 9.6 P6 — Per-route override observability

Configure listener with bucket cap=10 + route `/strict` TPFC bucket cap=1 (with stat_prefix derived from per-route config). Issue requests interleaved against `/strict` and `/loose`. Scrape `/stats/prometheus` after. Confirm:

- Per-route counters increment **independently** of listener counters.
- Stat-prefix labeling correctly identifies the per-route counter series.

Initial expectation: independent counters per per-route stat_prefix; no double-counting.

### 9.7 P7 — Fill-interval refill timing tolerance

Configure `fill_interval=200ms`, `tokens_per_fill=1`, `max_tokens=1`. Issue req at t=0 (succeeds, bucket empties), t=10ms (rate-limited; bucket still empty), t=250ms (succeeds; refill quantum at t=200ms). Pin tolerance band on the t=250ms transition. Initial expectation: ±20ms tolerance — token-bucket integer division on `elapsed / fill_interval` quantizes refill instants more coarsely than fault's `time.AfterFunc` (±10ms).

### 9.8 P8 — Header set on success

When NOT rate-limited (allow path), capture upstream request header set + downstream response header set. Confirm Envoy adds NO injected headers (X-RateLimit-*, etc.). Initial expectation: **no headers added** (`enable_x_ratelimit_headers` defaults OFF; deferred per ADR-0120 cluster 7).

---

## 10. ROADMAP row addition

This brainstorm appends a single new row to `docs/envoy-go/ROADMAP.md`, immediately after the existing row 10:

```
| 11 | http-filter-local-ratelimit | 10 | planned |  | New `internal/filter/http/localratelimit/` package implementing `envoy.filters.http.local_ratelimit` (Envoy v1.37.2 canonical local rate-limiting filter) under the 07.1 framework. FOURTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10). MVP envelope per BRAINSTORM §1.1: 5 fields consumed (`stat_prefix`, `token_bucket{max_tokens, tokens_per_fill, fill_interval}`, `status{code}` default 429, 3-tier `typed_per_filter_config` merge per ADR-0073 wholesale-override, 4 stats `enabled/ok/rate_limited/enforced`); 14 fields silently ignored under omnibus deferral ADR-0120 organized by 7–8 family-clusters (descriptor-action; Runtime + shadow-mode; xDS cluster-state; response-side header injection; per-connection lifecycle; multi-stage; X-RateLimit headers + vh policy; gRPC trailer). Token-bucket primitive Option A — lazy refill on access, single sync.Mutex per bucket, no per-bucket goroutine, monotonic time via `time.Now().UnixNano()` (Go ≥1.9 monotonic guarantee). FIRST production filter where per-route TPFC override implies independent stateful resources (per-route entries each own a `tokenBucket` per ADR-0117 = ADR-0073 amendment); cors / fault / header_mutation per-route is data-only. Differential fixture `0013-http-local-ratelimit` (4 scenarios per BRAINSTORM §6.2: basic-allow, basic-rate-limited, refill-after-fill_interval ±20ms, per-route-override). Stat surface 22→26 names (4 new counters; `enforced == rate_limited` MVP invariant per ADR-0118). Rate-limited response body byte-exact `"local_rate_limited"` (18 bytes; subject to §9.P3 confirmation); 4-header set lowercase wire-form; SendLocalReply mechanism mirrors fault abort. NO new framework primitive required (unlike phase 10's `ResolveAllTiers`). 15th fuzzer `FuzzLocalRateLimitConfigParse`. Anticipated ADRs ADR-0114..ADR-0120 (7 ADRs per BRAINSTORM §7; ADR-0114 captures package-name divergence from header_mutation's underscore-preserving directory pattern). Per ADR-0106 (BRAINSTORM Decision 12 of phase 09; validated at phases 10 + 11), §9 family-rows are flat top-level rows; phase 11 lands as row `11`, NOT as a sub-phase of any §9 parent. ADR-0045 surface-split release valve stays available if SPEC/PLAN find > ~1500 LoC / > ~25 tasks; brainstorm's position is single-row at ~1000 LoC / ~12–16 tasks. |
```

The row's `status` flips:
- `planned` → `in-progress` at the SPEC commit (lifecycle 2 → 3).
- `in-progress` → `done` at the phase-done commit (lifecycle 6).

The row's `summary` cell may receive minor edits at SPEC + impl + phase-done commits (per the existing project precedent at row 10's evolution: brainstorm placeholder → SPEC fill-out → phase-done final).

The §9 heading at ROADMAP line 56 is NOT modified (per ADR-0106(c)).

---

## 11. Open questions / risks

### 11.1 Token-bucket primitive mutex hot-path vs. LBP-1 lock-free clause

Decision 4's `tokenBucket` uses a single `sync.Mutex` for the elapsed-compute + token-decrement critical section. LBP-1 (per phase 06.1's invariant) pairs lock-free hot path with closure-capture. Phase 11 preserves the closure-capture half but departs from the lock-free half. ADR-0116 records this departure explicitly.

**Mitigation:** ADR-0116 either tightens LBP-1 to "closure-captured" without the lock-free clause (recording phase 11 as the Nth application) OR scopes phase 11 as LBP-1-adjacent (closure-capture pattern follows LBP-1; mutex hot-path is acknowledged divergence). SPEC author chooses based on how other phases applying LBP-1 actually used it (a lock-free atomic counter at phase 06.1; a closure-captured `*atomic.Int64` at phase 09 fault's `max_active_faults`). Both interpretations have precedent — the brainstorm leaves the choice to SPEC.

### 11.2 Empirical pin §9.P3 body-bytes divergence

The initial expectation `"local_rate_limited"` (18 bytes) is suggestive of Envoy's "filter-name-derived" body convention (matching fault's `"fault filter abort"` 18-byte literal). If reference Envoy emits a different literal (e.g., `"reached local rate limit"`), the body byte-exact equivalence claim shifts and ADR-0119 records the corrected literal.

**Mitigation:** §9.P3 empirically pins. Fixture scenario 2's expectations.yaml encodes the SPEC-confirmed bytes. No risk to phase scope.

### 11.3 Empirical pin §9.P5 stat-name unexpected divergence

The initial expectation `envoy_http_local_rate_limit_{enabled,ok,rate_limited,enforced}` matches Envoy's naming convention for HTTP filters under the conn_manager prefix. If Envoy emits additional stats under the `local_rate_limit_*` substring (e.g., `local_rate_limit.token_bucket.refill_count` or `local_rate_limit.config_reload_count`), the twin-series filter discipline applies (BEHAVIOR_CONTRACT mapping rules SN1–SN8). envoy-go would NOT emit those stats; SPEC author records the omissions.

**Mitigation:** SPEC author scrapes Envoy under load and enumerates the full `local_rate_limit_*` stat set; ADR-0118 + BEHAVIOR_CONTRACT subsection record both the emitted-by-envoy-go set and the omitted-but-emitted-by-Envoy set.

### 11.4 Per-route TPFC `LocalRateLimitPerRoute` proto-message shape

The proto message `LocalRateLimitPerRoute` carries a single field `rate_limit` (which embeds `LocalRateLimit`). Phase 11 parses this via `New` recursively (each per-route `LocalRateLimit` body goes through the same factory). If `LocalRateLimitPerRoute` carries additional fields beyond `rate_limit` (e.g., a per-route `disable` flag or a per-route descriptor override), phase 11 silently ignores them under ADR-0120's omnibus deferral.

**Mitigation:** SPEC author validates the v3 proto-message shape during ADR-0115 drafting and adds any additional `LocalRateLimitPerRoute` fields to the omnibus deferral list. No risk to phase scope.

### 11.5 Refill timing ±20ms tolerance under CI load

Scenario 3's ±20ms tolerance assumes typical CI scheduling jitter. Under heavy CI load (parallel test runs, container scheduling delays), the ±20ms band may be too tight. If the fixture flakes during phase 11 impl, the SPEC author or PLAN author widens the tolerance (e.g., to ±30ms or ±50ms) and records the empirical CI-jitter observation in BEHAVIOR_CONTRACT.

**Mitigation:** SPEC author runs scenario 3 ≥10 times in succession during empirical-pin §9.P7 to characterize the jitter distribution, not just the median. The PLAN author has the option to add a retry-with-deadline harness around scenario 3's t=400ms boundary check.

### 11.6 Package-name precedent ambiguity for phase 12+

Phase 11's directory `localratelimit/` (no underscore) departs from phase 10's `header_mutation/` (underscore-preserving). Phase 12+ brainstormers face an ambiguous precedent: do they pick no-underscore (phase 11 + cors + fault) or underscore-preserving (phase 10)? ADR-0114 captures the rationale for phase 11's choice but does NOT prescribe a uniform discipline going forward.

**Mitigation:** ADR-0114 explicitly notes "future filter brainstorms may apply either convention deliberately." Each phase-N brainstorm makes its own choice with rationale captured in its filter-package-shape ADR. The non-uniformity is a deliberate design property, not a bug.

---

## 12. Handoff to SPEC author

The next session, per the state machine §5 step 2 (BRAINSTORM.md exists, SPEC.md does not), is the SPEC-authoring session. Skill: `superpowers:writing-plans` per ADR-0005, but routed through SPEC-authoring first per the phase 09 + 10 precedent (the project's lifecycle has BRAINSTORM → SPEC → PLAN → impl → review).

The SPEC author MUST:

1. Read this `BRAINSTORM.md` in full.
2. Resolve all 8 §9 empirical pins against reference Envoy v1.37.2 IN-SESSION (per ADR-0004) — NOT defer to later sessions.
3. Author `docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md` covering: §1 mission, §§2–N implementation surface (per the §3 surface inventory in this brainstorm), §11 empirical-pin block (§9 here), §15 acceptance checklist (per the phase 09 + 10 SPEC pattern).
4. NOT begin implementation. NOT author PLAN.md. NOT modify any Go file.
5. Run `spec-document-reviewer` subagent loop per ADR-0004 (max 3 iterations; if the loop exceeds, set STATE.md `lifecycle-state` = `blocked` + `block-reason` and exit).
6. On reviewer-approved SPEC, update STATE.md to point at the next session's `superpowers:writing-plans` invocation; commit the SPEC + STATE.md update.

ADR numbering: SPEC author uses ADR-0114 onward; the brainstorm's anticipated list (§7) is anticipatory — SPEC author may consolidate, split, or extend.

ROADMAP row 11 status flips `planned → in-progress` at the SPEC commit (per the standard lifecycle).

---

*End of phase 11 BRAINSTORM.md.*
