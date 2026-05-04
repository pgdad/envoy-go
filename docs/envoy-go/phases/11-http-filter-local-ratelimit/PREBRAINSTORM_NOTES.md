# Phase 11 Prebrainstorm Notes — `envoy.filters.http.local_ratelimit`

> **Status:** ADVISORY, NON-AUTHORITATIVE prework notes captured during a 2026-05-04
> session that started a phase-11 brainstorm under the assumption phase 10 was open
> for `local_ratelimit`, then pivoted (the user already had an in-flight phase-10
> brainstorm targeting `header_mutation` in another worktree). This file lives on
> branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes`, NOT on master.
> Cold-start procedure (BOOTSTRAP_PROMPT.md §1) does NOT read unmerged branches; a
> future phase-11 brainstormer is FREE to either consult this file or cold-start
> fresh from the §9 heading per ADR-0106(e). These notes are NOT a SPEC, NOT a
> committed BRAINSTORM, and have NOT been reviewed by the spec-document-reviewer
> subagent.
>
> **Prerequisites for this to become phase 11:** phase 10 (`http-filter-header-mutation`)
> must reach phase-done first. At that point, phase 11's `depends-on` is `10`.
> If a different filter is brainstormed for phase 11 before this one, these notes
> roll forward unchanged for whatever number `local_ratelimit` actually lands at
> (rename the directory and adjust the row number).

---

## 1. Filter selection rationale

`local_ratelimit` was the brainstormer's recommended pick from the §9 family
candidates (`ROADMAP.md` line 58) given:

- Reuses phase-09 fault's framework usage exactly: request-side `StopIteration`
  + `cb.SendLocalReply` for the rate-limited response. **No new framework primitive
  required** — no async-resume timer (lazy refill), no encode-side body iteration,
  no buffering, no cluster-replicated state.
- Opens the rate-limit family with a token-bucket primitive that `global_ratelimit`
  will reuse (via the descriptor-action dispatch subsystem deferred per §10 below).
- Bounded surface — estimated ~500–700 LoC implementation + ~200 LoC tests +
  ~200 LoC fixture; comfortably under ADR-0045's ~1500 LoC split-gate.
- LBP-1 succession progresses cleanly (06.1 → 07.1 → 08.1 → 08.1 → 08.2 → 09 → 11
  if header_mutation at phase 10 doesn't apply LBP-1; otherwise N+1).

Alternative candidates considered and rejected for second-§9-row position in the
session that produced these notes: `buffer` (would exercise a NEW primitive — buffer-
until-body-complete), `csrf` (too thin a surface; structurally just a "second
cors"), `compression` (encode-side body iteration is a NEW primitive but adds gzip/
brotli library deps), `jwt_authn` and the rest (framework-extending or
external-call-introducing).

## 2. MVP envelope (5 in-scope, 14 deferred)

User confirmed the envelope below in the 2026-05-04 session.

### 2.1 In-scope (5 fields consumed)

1. **`stat_prefix`** (required; PGV non-empty pin) — drives stat names.
2. **`token_bucket{max_tokens, tokens_per_fill, fill_interval}`** — single
   per-filter-instance bucket; lazy refill on access (Option A; see §3).
3. **`status{code}`** — rate-limited response status (default 429; PGV-pin
   range `[400, 600)`).
4. **3-tier `typed_per_filter_config` merge** (Route > VirtualHost >
   RouteConfiguration; most-specific override per ADR-0073) — phase-07.1
   primitive, already used by cors + fault. Per-route TPFC creates an
   independent `runtimeConfig` + independent `tokenBucket`.
5. **4 canonical stats** (extending `BEHAVIOR_CONTRACT.md ## Stat-name
   mapping` 22→26 names; subject to §6 empirical pin):
   `http.<stat_prefix>.local_rate_limit.{enabled,ok,rate_limited,enforced}`
   (all counters; `enforced == rate_limited` invariant under MVP since shadow
   mode is deferred).

### 2.2 Deferred (14 fields; omnibus deferral ADR proposed — see §5)

Each gets the ADR-0040 deferral format applied:

- `descriptors` — descriptor-based per-key buckets (large surface; opens
  descriptor-action dispatch shared with `global_ratelimit`; warrants its own
  follow-up phase).
- `rate_limits[]` — RateLimitAction list (descriptor-action dispatch; couples
  with `descriptors`).
- `filter_enabled` (`RuntimeFractionalPercent`) — runtime-key gating;
  Runtime + hot restart family.
- `filter_enforced` (`RuntimeFractionalPercent`) — shadow mode; same family.
- `request_headers_to_add_when_not_enforced` — shadow-mode header injection.
- `response_headers_to_add` — rate-limit response header injection.
- `local_rate_limit_per_downstream_connection` — per-connection bucket
  (different lifecycle; couples with filter chain teardown).
- `local_cluster_rate_limit` — clustered/replicated bucket (xDS / cluster
  state replication; xDS family).
- `stage` — multi-stage limiting.
- `enable_x_ratelimit_headers` — IETF X-RateLimit-Limit/Remaining/Reset.
- `vh_rate_limits` — virtual-host rate-limit policy override.
- `always_consume_default_token_bucket` — descriptor-coupled.
- `max_dynamic_descriptors` — descriptor-coupled.
- `rate_limited_as_resource_exhausted` — gRPC trailer; gRPC family.

## 3. Token bucket primitive (Option A — lazy refill)

User explicitly confirmed Option A in the session.

```go
// Lives in internal/filter/http/localratelimit/ (proposed package name).
// Closure-captured by the runtimeConfig (LBP-1 Nth application; N depends on
// what landed before phase 11).
type tokenBucket struct {
    maxTokens     int64
    tokensPerFill int64
    fillInterval  time.Duration

    mu            sync.Mutex
    tokens        int64
    lastRefillNs  int64 // time.Now().UnixNano() at last refill
}

func (b *tokenBucket) tryConsume() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    nowNs := time.Now().UnixNano()
    elapsedNs := nowNs - b.lastRefillNs
    refills := elapsedNs / int64(b.fillInterval) // integer division
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

**Why Option A:** matches Envoy's own implementation; no per-bucket goroutine
(rejected Option B's `time.Ticker` per bucket — fragile teardown plumbing,
mirrors fault's `time.AfterFunc` cancel-on-OnDestroy mechanics from ADR-0102
but with more state); not blocking (rejected Option C's signal-channel — wrong
shape vs. Envoy's non-blocking local_ratelimit).

**Time source:** `time.Now().UnixNano()` honors the monotonic clock per Go ≥1.9.

## 4. Per-route bucket isolation

This filter is the FIRST production filter where per-route TPFC override
implies independent stateful resources (cors's per-route config is data-only;
fault's `max_active_faults` was a closure-shared `*atomic.Int64` but
not per-route-distinct). The natural pattern falls out of phase-07.1 ADR-0073's
wholesale-override discipline:

- Each TPFC entry runs through `New` at config-load time.
- Each `New` invocation allocates its own `runtimeConfig` + own `tokenBucket`.
- The 3-tier resolver picks the most-specific config per request; that config
  carries its own bucket pointer.
- The listener-level bucket is unused for routes that override.

No new framework primitive is needed; this falls out of how ADR-0073 already
works for cors and fault.

## 5. Anticipated ADRs (count = 7)

ADR numbering depends on what's used by phase 10 (header_mutation anticipates
ADR-0108..ADR-0114). If header_mutation lands ADR-0114 as its highest, phase 11
starts at ADR-0115.

| Slot | Title |
|---|---|
| 1 | Filter package shape `internal/filter/http/localratelimit/` + registration ordering |
| 2 | `runtimeConfig` shape + 5-consumed / 14-silent-ignore decomposition + PGV constraints |
| 3 | `tokenBucket` primitive + Option-A lazy-refill mechanics + LBP-1 Nth application |
| 4 | Per-route bucket isolation as a consequence of ADR-0073's wholesale-override |
| 5 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension (or whatever number is current; depends on what header_mutation extends) + `enforced == rate_limited` invariant under MVP |
| 6 | Rate-limited response mechanics + body byte-exact + header set + status-text mapping |
| 7 | **OMNIBUS** deferral of 14 LocalRateLimit fields (mirrors ADR-0089 admin-endpoint deferral pattern) |

## 6. Empirical pins for SPEC author (8 pins; resolve IN-SESSION per ADR-0004)

The SPEC author runs reference Envoy v1.37.2 (the `ENVOY_TARGET.md`-pinned image)
against minimal local_ratelimit configs and captures verbatim observations.

- **§6.1 `stat_prefix` PGV.** Missing `stat_prefix` → reject at boot? Confirm error shape.
- **§6.2 `token_bucket` PGV.** (a) `max_tokens=0` → reject? (b) `tokens_per_fill=0`
  → defaults to 1 or rejects? (c) `fill_interval` minimum: 50ms doc claim;
  observe actual minimum.
- **§6.3 Rate-limited response body shape.** Trigger rate-limit; capture verbatim
  status line, body bytes, header set (lowercase wire-form), framing
  (Content-Length vs. chunked). **Initial expectation:** status `429`, body
  literal `"local_rate_limited"` (18 bytes), 4-header set similar to fault's
  abort response. SPEC author confirms or amends.
- **§6.4 Default `status.code`.** Omit `status` field; observe response code.
  **Expected:** 429. Confirm + pin.
- **§6.5 Stat names on the wire.** `/stats/prometheus` scrape under defined
  load; confirm `envoy_http_local_rate_limit_{enabled,ok,rate_limited,enforced}`
  exact form with `envoy_http_conn_manager_prefix` label. Document twin-series
  filter discipline for any `local_rate_limit_*` family Envoy emits that
  envoy-go does NOT.
- **§6.6 Per-route override observability.** Per-route TPFC stricter bucket
  vs. listener config; confirm independent counter increments.
- **§6.7 Fill-interval refill timing tolerance.** `fill_interval=200ms`,
  `tokens_per_fill=1`, `max_tokens=1`; req at t=0 ok, t=10ms rate-limited,
  t=250ms ok. Pin tolerance (expect ±20ms; token-bucket math has more
  variance than fault's ±10ms).
- **§6.8 Header set on success.** When NOT rate-limited, does Envoy add any
  injected headers? **Expected:** No (`enable_x_ratelimit_headers` defaults
  OFF; deferred per §2.2). Confirm.

## 7. Differential fixture — 4 scenarios

Fixture number depends on what phase 10 (header_mutation) lands; that phase
anticipates `0012-http-header-mutation`. Phase 11's fixture would then be
`0013-http-local-ratelimit`. Update on landing.

1. **basic-allow** — bucket sized large enough; 5 reqs all succeed. Asserts
   `enabled=5, ok=5, rate_limited=0`.
2. **basic-rate-limited** — bucket cap 2; 5 reqs; first 2 ok, last 3
   rate-limited. Asserts response body byte-exact + header set + counters.
3. **refill-after-fill_interval** — bucket cap 1, fill_interval 300ms; 3
   reqs at t=0/100ms/400ms; expect ok/rate-limited/ok per ±20ms tolerance.
4. **per-route-override** — listener bucket cap 10; route `/strict` has
   TPFC bucket cap 1; route `/loose` no TPFC. 3 reqs each route; confirms
   independent bucket isolation.

## 8. Test scaffolding

- **Unit** (`localratelimit_test.go`): factory-time PGV validation;
  `tokenBucket` math correctness (refill arithmetic, zero elapsed,
  max_tokens cap, monotonic time edge cases); per-route override resolution
  under 3-tier merge; stats increments under each path; rate-limited
  response shape via mock callbacks.
- **Fuzzer** `FuzzLocalRateLimitConfigParse` (Nth fuzzer; N depends on
  what header_mutation adds at phase 10). Fuzzes proto unmarshal +
  factory-time validation. Mirrors `FuzzFaultConfigParse` discipline.

## 9. Surface estimate + ADR-0045 readiness

- Implementation: ~500–700 LoC.
- Tests: ~200 LoC.
- Fixture: ~200 LoC.
- **Total: ~1000 LoC, ~12–16 PLAN tasks.**

Comfortably under ADR-0045's ~1500 LoC / ~25 task split-gate. Single-row
brainstorm. SPEC/PLAN authors retain release valve if scope grows.

## 10. Outstanding questions for the actual phase-11 brainstormer

These were not settled in the prework session — they need user input or
empirical resolution at brainstorm/SPEC time:

1. **Omnibus deferral ADR vs. per-field deferrals.** The session settled on
   omnibus (mirroring ADR-0089). Phase 09 took the per-field approach
   (ADR-0104 etc.). Future brainstormer can pick either.
2. **Body shape on rate-limit.** Initial expectation is `"local_rate_limited"`
   (18 bytes; same length as fault's `"fault filter abort"`). §6.3 must
   confirm; if it differs, the body-byte-exact equivalence claim adjusts.
3. **`enforced == rate_limited` invariant.** Under MVP shadow mode is deferred,
   so the two counters are equal. ADR captures this as deferred-divergence;
   future shadow-mode phase widens.
4. **Time source semantics.** `time.Now().UnixNano()` carries the monotonic
   component; verify under test that wall-clock jumps don't break the bucket
   math. (Should be fine because monotonic clock dominates UnixNano on Go ≥1.9
   for time.Now()-derived values.)

## 11. Provenance

- Captured 2026-05-04 in a Claude Code session that started a brainstorm
  under the assumption phase 10 was open. Master tip at session start was
  `3066c72` (phase 09 REVIEW); during the session, master advanced to
  `71b6401` after merging the existing phase-10 header_mutation brainstorm
  + SHA-fill.
- This file lives on branch
  `phase-11-http-filter-local-ratelimit-prebrainstorm-notes`, branched from
  `71b6401`. The branch is intentionally NOT merged to master.
- User explicitly requested preservation when offered the choice: "preserve
  it for phase 11."
