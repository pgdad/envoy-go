# Phase 11 SPEC — `envoy.filters.http.local_ratelimit`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 11 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09/10 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 605 lines; commits `59e1be2` + `6ad8d8a` on branch `phase-11-http-filter-local-ratelimit-brainstorm`). Off-master advisory branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (tip `fffe4b4`, pushed to origin) carries pre-brainstorm context and is consulted but superseded per BRAINSTORM §1.6 + ADR-0106(e).

**ADR continuity:** Phase 10 closed at ADR-0113. Phase 11 anticipates ADR-0114..ADR-0120 (7 ADRs per BRAINSTORM §7) — refined to **6** ADRs at this SPEC per §8.1 consolidation (one BRAINSTORM-anticipated ADR folds inline; see §8.1).

---

## 1. Purpose

Phase 11 lands `envoy.filters.http.local_ratelimit` — Envoy's canonical token-bucket request rate-limiting primitive — as the FOURTH production HTTP filter in envoy-go after cors (07.1), fault (09), and header_mutation (10), and the THIRD top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family after fault and header_mutation. The five new architectural primitives:

1. A new `internal/filter/http/localratelimit/` package owning the filter implementation. The package mirrors the cors + fault + header_mutation precedent (`internal/filter/http/<name>/`): `local_ratelimit.go` (filter type + factory + decode method + token-bucket primitive + runtimeConfig parser + filterStats wiring), `local_ratelimit_test.go` (unit tests across 5 test groups per §6.5), `doc.go` (package overview + 5-consumed/14-deferred decomposition), `fuzz_test.go` (`FuzzLocalRateLimitConfigParse` per §14.3 — the 15th fuzzer in the repo). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`runtimeConfig`, `tokenBucket`, `filterStats`, `filter`) are unexported. **Directory + Go-package identifier are both `localratelimit` (no underscore)** — diverges from the underscore-preserving directory pattern established by phase 10's `header_mutation/`. Rationale captured in ADR-0114; the no-underscore form aligns with `cors/` and `fault/` whose proto type-names were already single tokens. See ADR-0114.

2. **Token-bucket primitive (Option A, lazy refill on access).** A `tokenBucket` struct holds `{maxTokens, tokensPerFill, fillInterval, mu sync.Mutex, tokens, lastRefillNs}`. On each `tryConsume()`: lock; compute `elapsedNs := time.Now().UnixNano() - lastRefillNs`; compute `refills := elapsedNs / int64(fillInterval)`; if `refills > 0`, add `refills * tokensPerFill` to `tokens`, cap at `maxTokens`, advance `lastRefillNs += refills * int64(fillInterval)`; if `tokens > 0`, decrement and return true; else return false. NO per-bucket goroutine; NO `time.Ticker`; NO signal channel. Time source `time.Now().UnixNano()` carries Go ≥1.9's monotonic component for `time.Now()`-derived values; arithmetic across `time.Now()` calls advances monotonically under wall-clock NTP corrections / leap seconds. Bucket lifetime spans the filter-instance lifetime (listener: process-exit; per-route: process-exit since envoy-go is static-config-only per `BOOTSTRAP_PROMPT.md` §3.1). Single `sync.Mutex` per bucket — declared LBP-1-adjacent at ADR-0116 (closure-capture half preserved; lock-free hot-path half deliberately departs since `elapsed → refills` is a multi-step CAS-resistant computation). See ADR-0116.

3. **Rate-limit decision in `DecodeHeaders`.** The filter resolves the most-specific `runtimeConfig` via 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) — the existing 07.1 framework primitive per `internal/filter/http/perroute.go:103–128` (per ADR-0073). Increments `enabled` counter unconditionally. Calls `rc.bucket.tryConsume()`: if `true`, increments `ok` counter and returns `Continue`; if `false`, increments `rate_limited` AND `enforced` counters in lockstep (MVP invariant per ADR-0118), invokes `cb.SendLocalReply(rc.statusCode, body, headers)`, and returns `StopIteration`. NO async-resume primitive (unlike phase 09's `delay`). NO encode-side state (unlike phase 10's `response_mutations`). `DecodeData` / `DecodeTrailers` / `Encode*` are pass-through. The filter reuses fault's request-side StopIteration + SendLocalReply primitives exactly — NO new framework primitive required. See ADR-0119.

4. **Per-route bucket isolation as wholesale override.** Per the proto message `LocalRateLimitPerRoute` (which embeds a full `LocalRateLimit` body via its `rate_limit` field), each TPFC entry runs through `New` at config-load time; each `New` invocation allocates its own `runtimeConfig` + own `tokenBucket`. The 3-tier resolver picks the most-specific config per request; that config carries its own bucket pointer. The listener-level bucket is unused for routes that override. This falls out of ADR-0073's wholesale-override discipline already used by cors + fault + header_mutation — NO new framework primitive needed. Phase 11 is the **FIRST production filter where per-route override implies independent stateful resources** (cors's per-route is data-only — rules + flags; fault's `max_active_faults` is closure-shared atomic across the listener but not per-route distinct; header_mutation's per-route is data-only — mutation rules). ADR-0117 carries this as an ADR-0073 amendment paragraph: "wholesale-override extends to stateful resources without further framework support." Confirmed empirically at §11.6: per-route `/strict` reqs increment ONLY the per-route stat-prefix counters; listener-level counters do NOT increment for per-route reqs. See ADR-0117.

5. **Stat surface 22→26-name extension.** Four new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`: `<stat_prefix>.http_local_rate_limit.{enabled, enforced, ok, rate_limited}` (text format) / `envoy_http_local_rate_limit_{enabled,enforced,ok,rate_limited}{envoy_local_http_ratelimit_prefix="<stat_prefix>"}` (Prometheus format). All four wired through `internal/stats.Registry` via per-instance `*atomic.Int64` slots in `filterStats`. The 22-name table from phase 09 (which extended 17→22) and unchanged through phase 10 grows to 26 names. Increment discipline: `enabled` every request reaching the filter; `ok` per `tryConsume` → true; `rate_limited` per `tryConsume` → false; `enforced` per `tryConsume` → false (lockstep with `rate_limited` under MVP). MVP invariant `enforced == rate_limited` at every step; future shadow-mode phase widens to `enforced ≤ rate_limited` when `filter_enforced` runtime-key support lands per the Runtime + hot restart family. See ADR-0118.

After phase 11, the project has proven the §9 HTTP filters family-expansion pattern carries through a FOURTH filter under the cors precedent's package-shape discipline, the fault precedent's `runtimeConfig` parser pattern, and a NEW per-route stateful-resource discipline (independent token buckets per per-route TPFC entry, falling out of ADR-0073's existing wholesale-override semantics): *envoy-go's HTTP filter framework can host a stateful rate-limiting primitive that carries per-route independent buckets via the existing 3-tier `PerRouteConfig.Resolve` accessor, with no framework extension; the existing fault `SendLocalReply` + `StopIteration` mechanism carries through verbatim for the rate-limited path; the stat surface extends from 22 to 26 names with a four-counter set whose `enforced == rate_limited` MVP invariant has a documented natural-divergence point at future shadow-mode landing; the 50ms `fill_interval` minimum is a filter-internal Envoy check (not PGV) and reflects in envoy-go's `New` factory as a sibling validation; all under flat top-level row expansion (per ADR-0106).* This is the FOURTH §9 family-row to land; subsequent filters (compression, jwt_authn, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session AMENDS BRAINSTORM design decisions in **three** load-bearing places:

- **§11.5 (stat-name shape) — MAJOR REVISION:** BRAINSTORM Decision 6 / §9.P5 hypothesized the Prometheus label `envoy_http_conn_manager_prefix="<stat_prefix>"`. The empirical pin proves this is **WRONG**: the actual label key is `envoy_local_http_ratelimit_prefix="<stat_prefix>"` (filter-specific tag-extractor, NOT the conn-manager-prefix shared across HCM stats). The text-format stat-name shape is `<stat_prefix>.http_local_rate_limit.{enabled,enforced,ok,rate_limited}` (4 counters, lexicographic order); the Prometheus-format shape is `envoy_http_local_rate_limit_{enabled,enforced,ok,rate_limited}{envoy_local_http_ratelimit_prefix="<stat_prefix>"}`. envoy-go's stat emission MUST use the filter-specific tag-extractor for byte-equivalent fidelity on `/stats/prometheus` scrapes. Tag-extraction COLLISION quirk observed: when `stat_prefix` matches an Envoy-internal tag-extractor name (e.g. literal `listener` matches `envoy.listener_address`), the Prometheus output mangles the metric name (loses the `envoy_http_local_rate_limit_` prefix; gains an extra `envoy_listener_address` label). The differential fixture 0013 MUST avoid magic prefix names; SPEC §7.4 pins safe values (`foo`, `bar`, `baz`, `qux`, `strict`). ADR-0118 + §13.1 carry this constraint; envoy-go's stat-name emission discipline (per `internal/stats.Registry` + the existing tag-extractor registration in `internal/admin/stats.go`) needs a NEW filter-specific tag-extractor registration (planner-time decision; see §12 deferred decision 1).

- **§11.3 (`server` header value) — MINOR REVISION:** BRAINSTORM §1.1 item 8 hypothesized `server: envoy-go` on the rate-limited response. The empirical pin proves Envoy v1.37.2 emits `server: envoy` (literal `envoy`, NOT `envoy-go`). envoy-go ALREADY emits `server: envoy` from its HCM `serverHeader()` returning the literal string `"envoy"` (per `internal/filter/hcm/codec.go:17` and `internal/filter/http/router/router.go:52`). This is consistent with the prior fault fixture 0011's expectations.yaml (`server: "envoy"`) and the header_mutation fixture 0012. NO envoy-go code change is needed for the server-header value; the BRAINSTORM hypothesis was simply incorrect and is corrected here. Fixture 0013's expectations.yaml encodes `server: "envoy"` (lowercase wire-form, value `envoy`). ADR-0119 records the corrected wire shape.

- **§11.7 (refill timing tolerance) — REVISION (tighter):** BRAINSTORM §11.5 / §9.P7 hypothesized ±20ms tolerance on the refill-quantum boundary (wider than fault's ±10ms because integer-division on `elapsed / fill_interval` quantizes refill instants). The empirical pin proves the boundary is **sharp at ≤5ms granularity**: across 99 trials sweeping delay values 180→400ms (with 4ms step in the tight band 196→204ms), zero spurious refills observed before t=200ms and zero missed refills observed at t≥200ms. The empirical envelope is bounded by the measurement floor (Python wall-clock `time.sleep` resolution ~1–5ms on Linux). SPEC §7.1 scenario 3 uses **±10ms tolerance** (NOT ±20ms) — conservative over the ~5ms measurement floor while allowing for CI scheduling jitter. ADR-0116 records the empirical jitter envelope; if the fixture flakes during phase 11 impl under heavy CI load, the PLAN author has the option to widen the tolerance to ±20ms (matching the original BRAINSTORM hypothesis) and record the empirical CI-jitter observation in BEHAVIOR_CONTRACT — but the SPEC default is ±10ms.

Two additional empirical findings carry FORWARD into design but do NOT amend BRAINSTORM decisions:

- **§11.2c (`fill_interval` minimum is filter-internal NOT PGV):** Envoy's 50ms minimum on `token_bucket.fill_interval` is enforced via a filter-internal check (error: `local rate limit token bucket fill timer must be >= 50ms`; error path: `source/server/config_validation/server.cc:76`). It is NOT a PGV constraint (which would surface via the `goo.gle/debug…: Proto constraint validation failed` envelope). envoy-go's `New` factory must implement the same filter-internal validation as a sibling check after the proto unmarshal succeeds, returning a non-nil error that mirrors Envoy's wording. ADR-0115 records the filter-internal-check discipline separately from the PGV-table discipline.

- **§11.2 / §11.4 (filter_enabled / filter_enforced runtime-key defaults are 0%):** The proto fields `filter_enabled` and `filter_enforced` (both `RuntimeFractionalPercent`) DEFAULT to 0% (off) — meaning that a `local_ratelimit` config with these fields OMITTED leaves the filter's rate-limit decision **entirely bypassed** in reference Envoy: every request is allowed; the bucket never decrements; the `enabled` counter never increments. §2.1 cluster 2 ("Runtime + shadow-mode subsystem") defers both fields (per §13.5 forward-pointer notes; ADR-0120 omnibus dropped per §8.1, deferral lives inline). The empirical evidence reveals that envoy-go's "silent ignore" of these fields would produce a divergence-from-Envoy in the *common case* of the field being omitted: envoy-go (silent-ignore + always-on) would rate-limit; reference Envoy (default-0%-off) would not. **§7.4 fixture configs MUST set both `filter_enabled` and `filter_enforced` to 100%** (`runtime_key: <unique>`, `default_value: { numerator: 100, denominator: HUNDRED }`) explicitly on BOTH the reference Envoy and envoy-go sides; envoy-go silent-ignores the configured 100% (treating its own behavior as always-100% under MVP) while reference Envoy honors the 100%. The differential equivalence holds because both ends are explicitly 100%. ADR-0118 + §13.1 + §13.5 carry forward-pointer notes documenting the divergence-window for users who omit these fields outside the fixture context.

### 1.2 Revised scope summary (post-§11 amendments)

After the §1.1 amendments, phase 11's in-scope architectural primitives are the FIVE listed at the head of §1, expressed as 8 BRAINSTORM-§1.1-style line items (BRAINSTORM's 8 in-scope items stay at 8; the new filter-specific tag-extractor registration in §11.5 is a planner-time consequence of the existing stat-emit primitive, not a separate primitive). Differential fixture has FOUR scenarios per §7.1 (BRAINSTORM §6.2 — UNCHANGED at 4 scenarios; phase 11 has no BRAINSTORM-§6.2 scenario-drop analogous to phase 10's protected-header drop). Stat-name extension is FOUR names (22→26 table extension). ADR list is REDUCED from the brainstorm's 7 anticipated (ADR-0114..ADR-0120) to **6** (ADR-0114..ADR-0119) per §8.1 consolidation; the omnibus deferral ADR-0120 is dropped (its content folds inline into §8 of this SPEC + §13.1's BEHAVIOR_CONTRACT subsection's "deferred field families" paragraph, mirroring phase-10's Slot 7 consolidation per its §8.1 — see §8.1 below).

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 9 + ADR-0106)

Phase 11 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, and stays unchanged in state across phase 11's landings. Phase 11 is the FOURTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10). Each subsequent HTTP filters family member (compression, jwt_authn, rbac, …) becomes its own top-level row at row 12, 13, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

### 1.4 ADR-0045 split-by-surface readiness

Phase 11 stays a SINGLE row at this SPEC. The implementation surface is estimated at:

- ~500–700 LoC production filter (`local_ratelimit.go` per BRAINSTORM §3.1)
- ~30 LoC `doc.go`
- ~200 LoC unit tests (5 test groups per §6.5)
- ~50 LoC fuzzer
- ~40 LoC framework deltas (ZERO if §12 deferred decision 1 settles to "filter-package-local tag-extractor registration"; up to ~40 if it settles to "extend `internal/admin/stats.go` tag-extractor registry pattern")
- ~360 LoC fixture (envoy.yaml ~80 + envoy-go.yaml ~80 + driver/main.go ~150 + backend/main.go ~30 + expectations.yaml ~50 + README.md ~50; total approximate)
- 1 line in `cmd/envoy-go/main.go` (registration site)
- ~50 LoC ROADMAP+STATE+BEHAVIOR_CONTRACT additions at SPEC commit (this SPEC does not modify production code)

Total: ~1100–1400 LoC across all bundles, with ~1000 in Go code. Task count estimate per the BRAINSTORM §3.1: ~12–16 tasks. Both metrics stay below ADR-0045's 1500-LoC / 25-task split-trigger thresholds. The PLAN author retains the ADR-0045 release valve if PLAN finds the surface exceeds either threshold; the natural split per BRAINSTORM §1.4 is `11.1 = token-bucket primitive + listener-only filter MVP` and `11.2 = per-route TPFC + 4th fixture scenario`. **SPEC's position: single-row.**

### 1.5 Provenance — relationship to off-master prebrainstorm notes branch

Per BRAINSTORM §1.6 + ADR-0106(e), the off-master branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (tip `fffe4b4`, pushed to origin) carries advisory pre-brainstorm notes from a 2026-05-04 session that pivoted (the user already had an in-flight phase-10 brainstorm targeting `header_mutation` in a different worktree). THIS SPEC consults the BRAINSTORM as authoritative; the prebrainstorm notes branch is a historical record that does NOT need merging. Where the prebrainstorm notes diverge from BRAINSTORM decisions (or this SPEC's §1.1 amendments), this SPEC wins. The notes branch stays in place on origin per the discipline.

---

## 2. Non-purposes

Phase 11 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.local_ratelimit`.

### 2.1 LocalRateLimit proto-message non-goals (per BRAINSTORM §8 + ADR-0120 omnibus deferral)

The proto message `envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit` carries 19 top-level fields. Phase 11 consumes 5 (`stat_prefix`, `token_bucket{max_tokens, tokens_per_fill, fill_interval}`, `status{code}` default 429, plus the `LocalRateLimitPerRoute` per-route container). The remaining 14 fields are silently ignored at config-load time (no warnings, no rejection — faithful to ADR-0040 silent-ignore discipline as established by cors / fault / header_mutation). The omnibus deferral organizes the 14 fields by 7–8 family-clusters (BRAINSTORM §8 / ADR-0120 anticipation; consolidated inline at §8.1 per this SPEC's §8.1 ADR-collapse).

Out of scope for phase 11:

- **Cluster 1 — Descriptor-action subsystem (4 fields):** `descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`. Couples to `global_ratelimit` future phase (descriptor-action dispatch is shared infrastructure between local and global rate-limit families).
- **Cluster 2 — Runtime + shadow-mode subsystem (3 fields):** `filter_enabled` (`RuntimeFractionalPercent`), `filter_enforced` (`RuntimeFractionalPercent`), `request_headers_to_add_when_not_enforced`. Couples to Runtime + hot restart family. **Per §1.1 amendment: fixture configs MUST set both `filter_enabled` and `filter_enforced` to explicit 100% on BOTH sides** for differential equivalence to hold; envoy-go's silent-ignore behavior is equivalent to "always-100%".
- **Cluster 3 — xDS cluster-state subsystem (1 field):** `local_cluster_rate_limit`. Couples to xDS / dynamic config family (clustered/replicated bucket via xDS cluster-state replication).
- **Cluster 4 — Response-side header injection (1 field):** `response_headers_to_add` (rate-limit response header injection — added to the 429 response).
- **Cluster 5 — Per-connection lifecycle (1 field):** `local_rate_limit_per_downstream_connection` (per-connection bucket; different lifecycle that couples with filter chain teardown).
- **Cluster 6 — Multi-stage limiting (1 field):** `stage` (allows ordered application of multiple rate-limit filters in a chain). Couples to descriptor-action subsystem.
- **Cluster 7 — X-RateLimit headers + vh policy (2 fields):** `enable_x_ratelimit_headers` (IETF X-RateLimit-Limit/Remaining/Reset), `vh_rate_limits` (virtual-host rate-limit policy override).
- **Cluster 8 — gRPC trailer mapping (1 field):** `rate_limited_as_resource_exhausted` (gRPC RESOURCE_EXHAUSTED trailer for gRPC-over-HTTP/2 rate-limit responses). Couples to gRPC family.

The 14-field deferral is captured inline at §13.1's BEHAVIOR_CONTRACT subsection (5-consumed / 14-ignored field map) + §13.5 forward-pointer notes; the omnibus ADR-0120 anticipated by BRAINSTORM is **dropped** per §8.1 consolidation (the deferral list is not load-bearing as a standalone ADR; it is a documentation artefact that lives in BEHAVIOR_CONTRACT).

### 2.2 Token-bucket primitive non-goals

The `tokenBucket` primitive is **lazy-refill on access**, single `sync.Mutex` per bucket, no per-bucket goroutine. Specifically OUT of scope:

- **Active timer-driven refill (Option B / Option C from BRAINSTORM §2.4):** `time.Ticker` per bucket + cancel-on-OnDestroy plumbing; signal-channel synchronization. Both rejected at BRAINSTORM time.
- **Lock-free / atomic-CAS hot path:** the `elapsed → refills` computation requires a multi-step compute-then-conditional-update sequence not amenable to a single CAS. Mutex is the deliberate choice; ADR-0116 records this as a deliberate departure from LBP-1's lock-free clause (closure-capture half preserved).
- **Cluster-replicated bucket state:** bucket state is per-process; xDS cluster-state replication (`local_cluster_rate_limit`) is deferred per §2.1 cluster 3.
- **Per-connection bucket lifecycle:** bucket lifetime is per-config (listener or per-route TPFC); per-connection bucket (via `local_rate_limit_per_downstream_connection`) is deferred per §2.1 cluster 5.
- **Wallclock monotonicity guarantee verification:** the SPEC takes Go's `time.Now()`-derived `UnixNano()` as monotonic per Go ≥1.9 documentation; unit tests do NOT exercise wall-clock backward-jump simulation (such testing requires test-only clock injection, deferred to a future hardening pass — noted in §12 deferred decision 5).

### 2.3 Test-surface non-purposes

- **No new differential probe filter.** Phase 07.1's `envoy.filters.http.envoy_go_test` (the iteration-state probe filter) covers framework iteration coverage. Phase 11 does not extend that probe.
- **No new fuzzer category.** The 15th fuzzer `FuzzLocalRateLimitConfigParse` follows the existing `FuzzFooConfigParse` pattern (cors, fault, header_mutation, etc.).
- **No structural-iteration fixture** (07.1's 0007b).  Phase 11 is differential-only; the iteration coverage table in §4.1 of this SPEC documents the iteration states exercised relative to the 07.1 framework's full iteration surface.
- **No 13th-fixture renumbering or reshuffling.** Phase 11 is fixture `0013-http-local-ratelimit`; the previous fixtures (0000–0012) stay green and unchanged.

### 2.4 Cross-filter non-purposes

- **No interaction with cors / fault / header_mutation per-route configs in fixture 0013.** Phase 11's fixture configures ONLY `local_ratelimit` filters (plus the router terminal). Mixed-filter ordering tests (e.g. fault + local_ratelimit on the same listener) are deferred to a future "filter-chain-ordering" hardening phase or to the existing 0007a-cors / 0011-http-fault fixture extensions if needed.
- **No HCM-level changes.** Phase 11 reuses the existing `internal/filter/hcm/` body discipline + `serverHeader()` returning `"envoy"` (per `internal/filter/hcm/codec.go:17`). Per §1.1 amendment, the brainstorm's hypothesis of `server: envoy-go` was incorrect; the existing `server: envoy` value is correct.
- **No extension to existing per-route framework primitives.** Phase 11 reuses `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103–128`); no `ResolveAllTiers` invocation (unlike phase 10), no new framework callback. The phase 10 `ResolveAllTiers` + `RequestRouteConfigsAllTiers` + `ResponseRouteConfigsAllTiers` extensions stay landed and unused by phase 11 — they are header_mutation-specific surface.

### 2.5 Security non-purposes

- **No DoS-resistance characterization.** Phase 11 implements the rate-limit primitive itself; characterizing its DoS-resistance under bursty traffic, mutex-contention storms, or adversarial config is out of scope. The BRAINSTORM §11.5 risk noting CI scheduling jitter handling is operational, not security.
- **No timing-attack characterization** of `tryConsume()`'s mutex hold time. The 5–10 nanosecond critical section is not characterized as a constant-time primitive; rate-limit decisions per route are assumed observable to local downstream callers anyway (via 429 responses).
- **No bucket-overflow protection beyond the explicit `if b.tokens > b.maxTokens { b.tokens = b.maxTokens }` cap.** No saturation arithmetic on `int64`; `int64` cannot overflow for any sane `maxTokens * time.Duration` product (max representable ~9.2 × 10¹⁸ — far exceeds practical bucket sizes × sub-second intervals).

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 11)

The six-gate phase-done discipline (per phase 04+ canonical layout) for phase 11:

| Gate | Specialization for phase 11 |
|---|---|
| **A — Build / vet / lint clean** | `go build ./...`, `go vet ./...`, `golangci-lint run` all green; no new warnings introduced relative to the phase-10 baseline at master tip `97ed8b9`. New package `internal/filter/http/localratelimit/` lints clean. |
| **B — Race-test pass** | `go test -race ./...` green on all 33 packages plus the new `internal/filter/http/localratelimit/` package (34 packages total). Test count grows by ~30–40 (5 unit-test groups across the new test file). |
| **C — h2spec 53/53 PASS** | Conformance gate at the ADR-0051 pin (53/53 PASS); phase 11 introduces no HTTP/2 stack changes, so this is a regression check — not an extension. |
| **D — All fuzzers green at 30s budget** | 14 existing fuzzers (per phase 10 phase-done) + 1 new (`FuzzLocalRateLimitConfigParse`) = 15 fuzzers. Each runs 30s in the per-phase fuzzer gate; all green. |
| **E — All differential fixtures 0000–0013 PASS** | 13 prior fixtures + the new `0013-http-local-ratelimit` = 14 fixtures green. Total runtime estimated 40–50s wallclock (phase 10 reported 39.76s for 13 fixtures; fixture 0013 adds ~3–5s for its 4 scenarios, dominated by scenario 3's 250ms + scenario 4's interleaved request batch). |
| **F — `BEHAVIOR_CONTRACT.md` populated** | §13.1 new subsection `### envoy.filters.http.local_ratelimit` (inline; ~50 lines per the phase 09 / 10 precedent); §13.2 stat-table 22→26 extension (4 new rows); §13.3 timing-tolerance row (scenario 3); §13.4 equivalence-matrix new row pointing at fixture 0013 with per-scenario tolerance; §13.5 forward-pointer notes (deferred field families per §2.1). All edits land in-place per ADR-0052 at the phase-done commit. |

Gates A–E are the verification gates; Gate F is the contract-extension gate. All six must be green at the phase-done commit per `BOOTSTRAP_PROMPT.md` §7.5.

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 11)

```
internal/filter/http/localratelimit/doc.go                  ~30 LoC; package overview + 5-consumed/14-deferred decomposition
internal/filter/http/localratelimit/local_ratelimit.go      ~500-700 LoC; filter + factory + tokenBucket + runtimeConfig + DecodeHeaders + filterStats
internal/filter/http/localratelimit/local_ratelimit_test.go ~200 LoC; 5 test groups per §6.5
internal/filter/http/localratelimit/fuzz_test.go            ~50 LoC; FuzzLocalRateLimitConfigParse (15th fuzzer)
```

PLAN author may split `local_ratelimit.go` into `local_ratelimit.go` + `bucket.go` + `parse.go` for readability — §6 leaves the file split open. Estimated total: ~780–980 LoC + 200 unit + 50 fuzz = ~1030–1230 LoC of new Go code.

### 4.2 Changed production code (in 11)

```
cmd/envoy-go/main.go                  +1 line: httpReg.Register(localratelimit.TypeURL, localratelimit.New) before httpReg.Freeze
                                       Insertion ordering: alphabetical-after-router per ADR-0100 §2.2 convention:
                                       router → cors → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze
```

(Per §11.5 + §12 deferred decision 1, an ADDITIONAL change may land in `internal/admin/stats.go` or equivalent for the filter-specific tag-extractor registration. PLAN author resolves the location.)

### 4.3 New harness and fixture code (in 11)

```
test/fixtures/0013-http-local-ratelimit/envoy.yaml         ~80 LoC; reference Envoy STRICT_DNS, 2 listeners, filter_enabled+filter_enforced=100% explicit
test/fixtures/0013-http-local-ratelimit/envoy-go.yaml      ~80 LoC; envoy-go STATIC, same 2 listeners, filter_enabled+filter_enforced fields PRESENT (silent-ignored by envoy-go)
test/fixtures/0013-http-local-ratelimit/backend/main.go    ~30 LoC; simple HTTP backend echoing 200 + request marker
test/fixtures/0013-http-local-ratelimit/driver/main.go     ~150 LoC; 4-scenario orchestration; per-scenario teardown for state-reset
test/fixtures/0013-http-local-ratelimit/expectations.yaml  ~50 LoC; per-scenario counter delta + body byte-exact + header set + tolerance
test/fixtures/0013-http-local-ratelimit/README.md          ~50 LoC; SPEC §7 narrative
```

Estimated ~440 LoC fixture-bundle (Go + YAML + Markdown). Fixture directory structure mirrors `test/fixtures/0011-http-fault/` and `0012-http-header-mutation/`.

### 4.4 Changed documentation and state (in 11)

Lands across SPEC commit (this commit) + impl commits + phase-done commit:

```
docs/envoy-go/ROADMAP.md           SPEC commit: APPEND row 11 (status `in-progress`); phase-done commit: flip to `done` + finalize summary
docs/envoy-go/STATE.md             SPEC commit: lifecycle-state spec-complete + next-skill writing-plans + last-commit SHA-fill
                                   subsequent commits: per-task SHA-fill + state advance + last-updated bump
docs/envoy-go/DECISIONS.md         impl commits: append ADR-0114..ADR-0119 (6 ADRs per §8) + ADR-0073 amendment paragraph (per ADR-0117)
                                   NO ADR additions at SPEC commit (phase 09 / 10 precedent: ADRs land during impl, NOT at SPEC)
docs/envoy-go/BEHAVIOR_CONTRACT.md phase-done commit: §13 4-edit bundle (NEW subsection + stat-table extension + timing tolerance + equivalence-matrix + forward-pointer notes)
docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md     SPEC commit: this file
docs/envoy-go/phases/11-http-filter-local-ratelimit/PLAN.md     authored next session per writing-plans
docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md authored at PLAN time; per-task SHA-fill during impl
docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md   authored at phase-done close
```

This SPEC commit's diff stat: 1 new file (this SPEC.md) + 1 new ROADMAP row + STATE.md update. Total ~1100 SPEC lines + ~3 ROADMAP lines + ~10 STATE.md lines.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 11)

```
   cmd/envoy-go/main.go
        │
        │ httpReg.Register(localratelimit.TypeURL, localratelimit.New)        [NEW LINE; phase 11]
        ▼
   internal/filter/http/registry.go
        │
        │ Register(typeURL, factory) → Freeze() → Resolve(typeURL) at HCM build
        ▼
   internal/filter/http/localratelimit/                                         [NEW PACKAGE; phase 11]
        ├── local_ratelimit.go    (filter + factory + tokenBucket + runtimeConfig + DecodeHeaders)
        ├── local_ratelimit_test.go
        ├── fuzz_test.go          (FuzzLocalRateLimitConfigParse)
        └── doc.go
        │
        │ uses:
        ▼
   internal/filter/http/perroute.go            (existing; PerRouteConfig.Resolve — 3-tier, most-specific)
   internal/filter/http/                        (existing; HTTPFilter interface, Continue/StopIteration enums)
   internal/stats/registry.go                   (existing; per-instance *atomic.Int64 emit primitives)
   internal/admin/stats.go                      (POSSIBLY changed; filter-specific tag-extractor registration per §11.5 + §12 D1)
   sync                                         (stdlib; sync.Mutex per bucket)
   time                                         (stdlib; time.Now().UnixNano() monotonic clock)
```

Untouched (load-bearing absence):

```
internal/filter/http/perroute.go               (existing 3-tier Resolve; phase 11 reuses; NO ResolveAllTiers needed unlike phase 10)
internal/filter/http/registry.go               (existing extension registry + Freeze; phase 11 adds one Register call site upstream)
internal/filter/http/cors/                     (untouched)
internal/filter/http/fault/                    (untouched; reused as the SendLocalReply + StopIteration precedent)
internal/filter/http/header_mutation/          (untouched; phase 11 explicitly diverges from its underscore-preserving directory pattern per ADR-0114)
internal/filter/http/router/                   (untouched)
internal/filter/http/envoygotest/              (untouched)
internal/filter/hcm/                           (untouched; HCM stays the chain runner; serverHeader() literal "envoy" preserved)
internal/listener/                             (untouched)
internal/cluster/                              (untouched)
internal/admin/                                (untouched in core; possibly extended for tag-extractor registration per §12 D1)
internal/drain/                                (untouched)
```

### 5.2 Per-request flow — listener-only allow path (canonical, scenario 1)

Request: `GET /something HTTP/1.1` arriving on listener `l_basic` configured with `local_ratelimit` (max=10, fill=10, interval=1s, stat_prefix=foo, filter_enabled=100%, filter_enforced=100%).

```
1. HCM IngressFilter.DecodeHeaders fires.
2. HCM resolves filter chain; localratelimit.filter.DecodeHeaders called.
3. filter.DecodeHeaders:
   a. Resolve runtimeConfig via PerRouteConfig.Resolve: returns listener-level rc.
   b. Increment rc.stats.enabled (atomic.Int64.Add).
   c. rc.bucket.tryConsume():
      - mu.Lock()
      - elapsedNs = time.Now().UnixNano() - lastRefillNs
      - refills = elapsedNs / int64(fillInterval)   // = 0 if elapsed < interval
      - if refills > 0: tokens += refills * tokensPerFill; cap at maxTokens; lastRefillNs += refills * fillInterval
      - if tokens > 0: tokens-- ; return true
      - mu.Unlock()
   d. tryConsume returned true (initial bucket = max=10 → 9 after).
   e. Increment rc.stats.ok.
   f. Return Continue.
4. HCM advances chain to router; cluster dial; backend response.
5. EncodeHeaders/EncodeData: pass-through (filter has NO encode-side state).
6. Backend response surfaces; HCM emits to client.
7. Counters: enabled=1, ok=1, rate_limited=0, enforced=0 (delta from baseline).
```

### 5.3 Per-request flow — listener-only rate-limited path (scenario 2)

Request: same listener but bucket already empty (max=2 after 2 prior reqs). 3rd request arrives within fill_interval.

```
1. filter.DecodeHeaders called.
2. Resolve runtimeConfig: listener-level rc.
3. Increment rc.stats.enabled.
4. rc.bucket.tryConsume():
   - mu.Lock()
   - elapsedNs = small (< fillInterval); refills = 0; tokens = 0
   - return false
   - mu.Unlock()
5. tryConsume returned false.
6. Increment rc.stats.rate_limited and rc.stats.enforced (lockstep MVP per ADR-0118).
7. Build 4-header set: OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}
   (HCM/router auto-injects content-length=18, date=<now-RFC1123>, server=envoy on response wire)
8. cb.SendLocalReply(429, []byte("local_rate_limited"), headers)
9. Return StopIteration.
10. HCM's localReplyDone gate ensures the chain short-circuits without dialing the upstream
    (per phase 09 ADR-0102's terminal-replace pattern).
11. Counters: enabled+1, ok=0, rate_limited+1, enforced+1.
12. Wire response: HTTP/1.1 429 Too Many Requests + 4 headers (content-length, content-type, date, server) + body "local_rate_limited" (18 bytes, no LF).
```

### 5.4 Per-request flow — refill after fill_interval (scenario 3)

Bucket configured max=1, fill=1, interval=200ms. Request sequence at t=0, t=10ms, t=250ms.

```
t=0:    tryConsume: bucket=1 → 0; allow → 200
t=10ms: tryConsume: elapsed=10ms; refills = 10/200 = 0; bucket=0; reject → 429
t=250ms: tryConsume: elapsed=250ms; refills = 250/200 = 1; bucket = 0 + 1*1 = 1, capped at 1
        lastRefillNs += 1 * 200ms = 200ms; tokens=1 → 0; allow → 200
```

The integer-division `elapsedNs / int64(fillInterval)` is the core quantization rule. SPEC §11.7 measured the boundary as sharp at ≤5ms granularity in reference Envoy; envoy-go's lazy-refill primitive matches naturally. Driver tolerance ±10ms around the t=200ms boundary (per §1.1 amendment from BRAINSTORM's ±20ms hypothesis).

### 5.5 Per-request flow — per-route override (scenario 4)

Listener `l_per_route` with listener-level config (max=10, stat_prefix=qux) + route `/strict` carrying `LocalRateLimitPerRoute` TPFC override (max=1, fill=1, interval=60s, stat_prefix=strict). Request to `/strict` (already saw 1 prior allow):

```
1. filter.DecodeHeaders called.
2. PerRouteConfig.Resolve: most-specific config is per-route TPFC; returns rc_strict (independent runtimeConfig + independent tokenBucket).
3. Increment rc_strict.stats.enabled.
4. rc_strict.bucket.tryConsume(): bucket=0 (after prior allow); elapsed < 60s; refills=0; reject.
5. Increment rc_strict.stats.rate_limited + rc_strict.stats.enforced.
6. SendLocalReply(429, body, headers); StopIteration.
7. Listener-level rc.stats and listener-level rc.bucket are NOT TOUCHED (wholesale-override; per ADR-0117 = ADR-0073 amendment).

Compare: request to /loose (no per-route override) on the same listener:
1. PerRouteConfig.Resolve: no per-route config; returns listener-level rc_listener.
2. Increment rc_listener.stats.enabled.
3. rc_listener.bucket.tryConsume(): bucket=10 → 9; allow.
4. Increment rc_listener.stats.ok; Continue.
5. Listener-level counters increment; rc_strict counters do NOT.
```

This is the FIRST production filter to demonstrate that ADR-0073's wholesale-override discipline carries through to STATEFUL per-route resources (independent buckets per per-route TPFC entry). ADR-0117 records the discipline-extension as an ADR-0073 amendment paragraph.

### 5.6 Concurrency model

Per-bucket: single `sync.Mutex` guarding `{tokens, lastRefillNs}`. Hot-path holding time: 5–10 nanoseconds typical (compute elapsed, integer-divide, conditional update, decrement). Lock contention bounded by per-route request rate; per-route TPFC isolation means listener-level traffic and per-route traffic don't compete on the same bucket.

Per-filter-instance: `runtimeConfig` is a `*runtimeConfig` reference; the underlying `runtimeConfig` value (including the closure-captured `*tokenBucket`) is shared across all goroutines processing requests through that filter instance. The `*runtimeConfig` is closure-captured at boot-time `New` and never mutated — it is read-only thread-safe. Per-route `runtimeConfig` instances are similarly immutable post-config-load.

Per-process: registry frozen at boot per ADR-0072 (no runtime registration); per-route TPFC parsed at HCM-build time; bucket lifetime = process lifetime (envoy-go is static-config-only; xDS-driven config reload is a future-phase concern per ADR-0008's static-only baseline).

ADR-0116 records this concurrency model as LBP-1-adjacent: closure-capture half preserved (matches phase 06.1 / phase 09 / phase 10's discipline); lock-free hot-path half deliberately departs (the multi-step elapsed-compute is not amenable to a single CAS; mutex is the natural choice). Phase 11 is the FIRST production filter to declare this LBP-1 succession-adjacent stance explicitly. The SPEC's position is that LBP-1 stays as documented in `BOOTSTRAP_PROMPT.md` §6.1 (closure-capture + lock-free hot path); phase 11 is an LBP-1-adjacent application; future per-filter reviewers consult ADR-0116 as the precedent for stateful-resource filters that need locking.

### 5.7 Filter ordering in fixture 0013

Filter chain in both `l_basic` and `l_per_route` listeners:

```
[envoy.filters.http.local_ratelimit] → [envoy.filters.http.router]
```

`local_ratelimit` is the first (and only non-router) filter. No interaction with cors / fault / header_mutation in fixture 0013 (per §2.4). Order does not matter for correctness — local_ratelimit is the only non-router filter — but the SPEC pins it as `[local_ratelimit, router]` for explicitness.

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (fault precedent verbatim, modulo unused stats fields)

```go
package localratelimit

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"

func New(ctx envoyhttp.FactoryCtx, tc *anypb.Any) (envoyhttp.HTTPFilterFactory, error) {
    // 1. tc.UnmarshalTo(&cfg) where cfg is *envoyextensionsfiltershttplocalratelimitv3.LocalRateLimit.
    // 2. Validate filter-internal constraints (50ms fill_interval minimum per §11.2c; PGV constraints
    //    are validated by proto unmarshal layer per §11.1/§11.2a/§11.2b-ii).
    // 3. Build runtimeConfig + tokenBucket + filterStats (closure-captured by the returned factory).
    // 4. Return a closure that constructs per-request *filter values referencing the closure-captured rc.
}
```

The `envoyhttp.FactoryCtx` 3-field shape from ADR-0100 stays as-is. Phase 11 consumes 2 of the 3 fields:
- `ctx.Stats` (consumed for filterStats wiring, per §6.6).
- `ctx.StatPrefix` (NOT consumed in the factory builder — `cfg.StatPrefix` from the proto is the source of truth per §11.5; HCM-level stat_prefix is stored in `ctx.StatPrefix` for cross-filter discipline but local_ratelimit's filter-level stat_prefix is a separate field). Note: the existing `ctx.StatPrefix` field carries the HCM connection-manager prefix (e.g. `ingress_http`), NOT the filter-level local_ratelimit stat_prefix.

### 6.2 `runtimeConfig` shape (per ADR-0115)

```go
type runtimeConfig struct {
    statPrefix string         // from cfg.StatPrefix (PGV non-empty per §11.1)
    bucket     *tokenBucket   // closure-captured per filter-instance / per per-route entry
    statusCode int            // from cfg.Status.Code (default 429 per §11.4; PGV [400, 600))
    body       []byte         // literal "local_rate_limited" (18 bytes; per §11.3 + ADR-0119)
    stats      *filterStats   // 4 counters scoped by stat_prefix
}

type filterStats struct {
    enabled, ok, rateLimited, enforced *atomic.Int64
}
```

Per-route `runtimeConfig` instances are independent per-TPFC-entry; each carries its own bucket pointer + own stats. The 14 deferred top-level fields (`descriptors`, `rate_limits`, `filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced`, `response_headers_to_add`, `local_rate_limit_per_downstream_connection`, `local_cluster_rate_limit`, `stage`, `enable_x_ratelimit_headers`, `vh_rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`, `rate_limited_as_resource_exhausted`) are unmarshalled (proto unmarshal is uniform per ADR-0040) but NOT captured into `runtimeConfig`. NO warnings; NO rejections.

### 6.3 Per-instance `filter` struct

```go
type filter struct {
    rc  *runtimeConfig                         // closure-captured at factory time; immutable
    dcb envoyhttp.DecoderFilterCallbacks       // set in OnNewStream (or equivalent) per the existing 07.1 pattern
}
```

Phase 11 does NOT consume `EncoderFilterCallbacks` (no encode-side state); the existing framework's `OnNewStream` wiring sets only `dcb`. (PLAN author confirms which precise callback-setup hook the existing framework exposes — see §12 deferred decision 2.)

### 6.4 `tokenBucket` primitive (per ADR-0116)

```go
type tokenBucket struct {
    maxTokens, tokensPerFill int64
    fillInterval             time.Duration

    mu           sync.Mutex
    tokens       int64
    lastRefillNs int64  // time.Now().UnixNano() at last refill
}

func newTokenBucket(maxTokens, tokensPerFill int64, fillInterval time.Duration) *tokenBucket {
    return &tokenBucket{
        maxTokens:    maxTokens,
        tokensPerFill: tokensPerFill,
        fillInterval: fillInterval,
        tokens:       maxTokens,                // initial fill = max
        lastRefillNs: time.Now().UnixNano(),    // baseline for refill arithmetic
    }
}

func (b *tokenBucket) tryConsume() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    nowNs := time.Now().UnixNano()
    elapsedNs := nowNs - b.lastRefillNs
    if refills := elapsedNs / int64(b.fillInterval); refills > 0 {
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

PLAN author may move this into a sibling file `bucket.go` for readability (the file split is a planner-time decision; ADR-0114 leaves it open).

### 6.5 `DecodeHeaders` body discipline (per ADR-0119)

```go
func (f *filter) DecodeHeaders(_ context.Context, _ envoyhttp.RequestHeaders, _ bool) (envoyhttp.HeadersStatus, error) {
    f.rc.stats.enabled.Add(1)
    if f.rc.bucket.tryConsume() {
        f.rc.stats.ok.Add(1)
        return envoyhttp.HeadersStatusContinue, nil
    }
    f.rc.stats.rateLimited.Add(1)
    f.rc.stats.enforced.Add(1)  // lockstep with rate_limited per ADR-0118 MVP invariant
    f.dcb.SendLocalReply(f.rc.statusCode, f.rc.body, envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    return envoyhttp.HeadersStatusStopIteration, nil
}
```

(Note: `RequestHeaders` is unused — no header-based gating for phase 11 MVP; future shadow-mode phase may add `request_headers_to_add_when_not_enforced` injection.)

`DecodeData`, `DecodeTrailers`, `EncodeHeaders`, `EncodeData`, `EncodeTrailers`, `OnDestroy` are all pass-through (return `Continue` / no-op).

The 4-header wire-form (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) is produced by the SendLocalReply call site (`Content-Type` from the filter) + HCM/router downstream auto-injection (`content-length`, `date`, `server` per the existing fault precedent in `internal/filter/http/fault/fault.go:321` + `internal/filter/http/router/router.go:70`).

### 6.6 `filterStats` wiring (per ADR-0118 + §11.5)

The 4-counter `filterStats` is wired through the existing `internal/stats.Registry`:

```go
type filterStats struct {
    enabled, ok, rateLimited, enforced *atomic.Int64
}

func newFilterStats(reg *stats.Registry, statPrefix string) *filterStats {
    return &filterStats{
        enabled:     reg.NewCounter(statPrefix + ".http_local_rate_limit.enabled"),
        ok:          reg.NewCounter(statPrefix + ".http_local_rate_limit.ok"),
        rateLimited: reg.NewCounter(statPrefix + ".http_local_rate_limit.rate_limited"),
        enforced:    reg.NewCounter(statPrefix + ".http_local_rate_limit.enforced"),
    }
}
```

Stat-name templates:

- **Text format:** `<stat_prefix>.http_local_rate_limit.{enabled, enforced, ok, rate_limited}` (4 names; lexicographic order in `/stats` output).
- **Prometheus format:** `envoy_http_local_rate_limit_{enabled, enforced, ok, rate_limited}{envoy_local_http_ratelimit_prefix="<stat_prefix>"}`.

The Prometheus tag-extraction MUST extract `<stat_prefix>` into the label key `envoy_local_http_ratelimit_prefix` (NOT `envoy_http_conn_manager_prefix` per §1.1 amendment + §11.5). PLAN author resolves the precise tag-extractor registration site — the existing tag-extractor mechanism in `internal/admin/stats.go` (or wherever the project registers its tag-extractor patterns) needs a NEW pattern for the `local_rate_limit` filter (see §12 deferred decision 1).

### 6.7 Per-route 3-tier resolve (existing primitive; reused per ADR-0117)

Phase 11 reuses `internal/filter/http/perroute.go:103–128`'s existing `PerRouteConfig.Resolve` (most-specific tier wins; ADR-0073 wholesale-override). NO `ResolveAllTiers` invocation (unlike phase 10). NO new framework primitive.

The resolver returns a `*runtimeConfig` value (the `*runtimeConfig` returned by the most-specific tier's `BuildPerRouteConfig` factory invocation) for the request. The filter dereferences the `*runtimeConfig` to get the closure-captured `*tokenBucket` + `*filterStats`. Wholesale-override means: if Route-tier has a TPFC entry, the listener-level config is **entirely shadowed** for that request — listener-level `*tokenBucket` + `*filterStats` are NOT touched. Confirmed empirically at §11.6.

ADR-0117 is the load-bearing ADR codifying that ADR-0073's wholesale-override discipline carries through to STATEFUL resources (independent buckets + independent stat counters per per-route TPFC entry).

---

## 7. Differential fixture `0013-http-local-ratelimit`

### 7.1 Equivalence claims (per BRAINSTORM §6 + §1.1 amendments)

Four scenarios, mirrored across reference Envoy v1.37.2 (STRICT_DNS) and envoy-go (STATIC), per ADR-0019's differential equivalence discipline.

| # | Scenario | Listener | Config | Workload | Asserts |
|---|---|---|---|---|---|
| 1 | basic-allow | `l_basic` | bucket cap 10, per-fill 10, interval 1s, stat_prefix=foo, filter_enabled+enforced=100% | 5 reqs back-to-back via `l_basic` | 5x 200; counter deltas `enabled=5, ok=5, rate_limited=0, enforced=0`; `/stats/prometheus` scrape equivalence |
| 2 | basic-rate-limited | `l_basic` | bucket cap 2, per-fill 2, interval 60s, stat_prefix=bar, filter_enabled+enforced=100% | 5 reqs back-to-back via `l_basic` | first 2× 200, last 3× 429; counter deltas `enabled=5, ok=2, rate_limited=3, enforced=3`; rate-limited response: status `429 Too Many Requests`, body byte-exact `local_rate_limited` (18 bytes, no LF), 4 headers in order (`content-length: 18`, `content-type: text/plain`, `date: <allow-listed>`, `server: envoy`), framing Content-Length |
| 3 | refill-after-fill_interval | `l_basic` | bucket cap 1, per-fill 1, interval 200ms, stat_prefix=baz, filter_enabled+enforced=100% | 3 reqs at t=0/10ms/250ms via `l_basic` | t=0 → 200, t=10ms → 429, t=250ms → 200; **±10ms tolerance per §1.1 amendment** on the t=250ms boundary (the t=0 + t=10ms checks are effectively zero-tolerance) |
| 4 | per-route-override | `l_per_route` | listener bucket cap 10, stat_prefix=qux; route `/strict` TPFC bucket cap 1, per-fill 1, interval 60s, stat_prefix=strict; route `/loose` no TPFC | 3 reqs each route, interleaved `/strict`,/loose`,`/strict`,/loose`,`/strict`,/loose` | `/strict`: 1× 200 + 2× 429; `/loose`: 3× 200; `strict`-prefixed counters `enabled=3, ok=1, rate_limited=2, enforced=2`; `qux`-prefixed counters `enabled=3, ok=3, rate_limited=0, enforced=0` (per §11.6 wholesale-override; listener counters do NOT increment for `/strict` reqs) |

Per-scenario teardown enforces state-reset (envoy-go + reference Envoy both restarted between scenarios) — token-bucket state does NOT leak between scenarios. This discipline is established by phase 09's fault driver and inherited.

### 7.2 Driver outline

Single Go binary `test/fixtures/0013-http-local-ratelimit/driver/main.go` orchestrates all four scenarios in sequence per the 0011-fault and 0012-header_mutation precedents:

```
driverMain():
  for each scenario in [scenario1, scenario2, scenario3, scenario4]:
    1. Spawn reference Envoy (docker run) on disjoint port pair (admin + listener).
    2. Spawn envoy-go on disjoint port pair.
    3. Wait for /ready on both.
    4. Issue scenario workload (HTTP/1.1 GETs to listener port) per the scenario's config.
       For scenario 3: tight time.Sleep schedule (t=0 immediate; t=10ms +10ms; t=250ms +250ms total — using time.Now() basis to respect ±10ms tolerance).
    5. Scrape /stats/prometheus from both admin endpoints.
    6. Compare:
       - per-request status codes
       - per-request response headers (lowercase wire-form, 4-header set for 429)
       - per-request response body bytes (18-byte "local_rate_limited" for 429; backend response for 200)
       - counter deltas across the 4 stat names
       - tag-extracted Prometheus label `envoy_local_http_ratelimit_prefix`
    7. Tear down both servers (SIGTERM + wait); reap processes.
  Report: PASS if all scenarios match; FAIL with first-divergence dump otherwise.
```

Tolerance handling: scenario 3's t=250ms boundary uses ±10ms wallclock tolerance; the driver enforces this by computing the actual delay relative to the t=0 baseline and comparing against the expected band [200ms, 260ms] for "refill must have happened by". If the actual 3rd-request delay falls outside the band, the driver fails fast with a diagnostic dump. (PLAN author may switch to a retry-with-deadline strategy if simple time.Sleep proves insufficient under CI load — see §12 deferred decision 4.)

### 7.3 Fixture bootstrap (per BRAINSTORM §7; port-disambiguated)

```yaml
# envoy.yaml fragment (reference Envoy STRICT_DNS):
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_basic
      address: { socket_address: { address: 0.0.0.0, port_value: 0 } }   # driver-rendered
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_basic
                route_config:
                  name: rc_basic
                  virtual_hosts:
                    - name: vh_basic
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: foo                  # safe non-magic value per §1.1 amendment
                      token_bucket:
                        max_tokens: 10                  # scenario 1; scenario 2 uses 2; scenario 3 uses 1
                        tokens_per_fill: 10
                        fill_interval: 1s               # scenario 3 uses 200ms; scenario 2 uses 60s
                      filter_enabled:                   # MUST be 100% explicit per §1.1 amendment
                        runtime_key: local_rate_limit_enabled_l_basic
                        default_value: { numerator: 100, denominator: HUNDRED }
                      filter_enforced:                  # MUST be 100% explicit per §1.1 amendment
                        runtime_key: local_rate_limit_enforced_l_basic
                        default_value: { numerator: 100, denominator: HUNDRED }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    - name: l_per_route
      address: { socket_address: { address: 0.0.0.0, port_value: 0 } }   # driver-rendered
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_per_route
                route_config:
                  name: rc_per_route
                  virtual_hosts:
                    - name: vh_per_route
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/strict" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.local_ratelimit:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimitPerRoute
                              rate_limit:
                                stat_prefix: strict
                                token_bucket: { max_tokens: 1, tokens_per_fill: 1, fill_interval: 60s }
                                filter_enabled: { runtime_key: __strict_enabled, default_value: { numerator: 100, denominator: HUNDRED } }
                                filter_enforced: { runtime_key: __strict_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                        - match: { prefix: "/loose" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: qux
                      token_bucket: { max_tokens: 10, tokens_per_fill: 10, fill_interval: 1s }
                      filter_enabled: { runtime_key: __qux_enabled, default_value: { numerator: 100, denominator: HUNDRED } }
                      filter_enforced: { runtime_key: __qux_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: 0 } } }   # driver-rendered
```

The envoy-go.yaml mirrors the same 2 listeners with `STATIC` cluster type per the project convention (existing fixtures' STATIC vs STRICT_DNS asymmetry). Both `filter_enabled` and `filter_enforced` fields are PRESENT in envoy-go.yaml even though envoy-go silent-ignores them (per §2.1 cluster 2 / §13.5; ADR-0120 omnibus dropped per §8.1, deferral lives inline) — the field presence ensures byte-equivalent config-load behavior.

### 7.4 Backend shape

Simple HTTP backend (`backend/main.go`) — equivalent to fixture 0011's backend per the 0011 + 0012 precedent: ~30 LoC; one Go HTTP server listening on a driver-allocated port; 200 responses with body containing a request marker (`backend\n` literal, mirroring 0011's body); no special handling for `/strict` or `/loose` (the rate-limit decision happens in Envoy/envoy-go before the upstream call).

### 7.5 Header allow-list extensions (inheriting phase 10 lessons)

Baseline proxy-injected headers (carry-forward from phases 09 / 10): `x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*`.

Phase 11-specific:

- **Rate-limited path (scenarios 2, 3, 4):** add `date` to per-scenario allow-list (not global) — same discipline as phase 09's fault abort response.
- **Allow path (scenarios 1, 4 `/loose`):** no additional headers added by the rate-limit filter (`enable_x_ratelimit_headers` defaults OFF; deferred per §2.1 cluster 7 / §13.5 (ADR-0120 omnibus dropped per §8.1); confirmed empirically at §11.8).
- **`connection: close` on rate-limited path:** if reference Envoy injects `connection: close` under HTTP/1.1 hop-by-hop semantics during the rate-limited path, the allow-list adds it. PLAN author validates during impl; the empirical pin §11.3 did NOT observe this header (Envoy v1.37.2 omits it for the rate-limited 429 response in the captured probe). SPEC's position: NOT in allow-list; PLAN author adds if observed during fixture validation.

### 7.6 Differential gate scope clarification

Per ADR-0019, the differential equivalence claim covers the four scenarios above. It does NOT cover:

- Workloads outside the scenario specs (e.g. mixed allow + rate-limit reqs interleaved within a single scenario beyond the prescribed pattern; rate-limit decisions during burst-load exceeding the bucket cap by orders of magnitude).
- Internal Envoy implementation details (tokens internal representation; lock-acquire latency; goroutine vs thread-pool scheduling).
- Stat-prefix tag-extraction collision cases (e.g. setting `stat_prefix=listener` triggers the Prometheus-mangling quirk per §11.5 / §1.1 amendment). The fixture deliberately uses safe values (`foo`, `bar`, `baz`, `qux`, `strict`).

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §1.1 + §8.1)

Phase 10 closed at ADR-0113. Phase 11 anticipates **6** ADRs (ADR-0114..ADR-0119), down from BRAINSTORM's 7 (ADR-0120 omnibus deferral folds inline per §8.1):

| Slot | ID | Title |
|---|---|---|
| 1 | ADR-0114 | Filter package shape `internal/filter/http/localratelimit/` (no underscore; `localratelimit.go` filename retains underscore matching proto type-name) + extension-registry registration ordering. Captures rationale for departing from header_mutation's underscore-preserving directory pattern. |
| 2 | ADR-0115 | `runtimeConfig` shape + 5-consumed / 14-silent-ignored field decomposition + PGV constraint table (resolves §11.1, §11.2, §11.4) + filter-internal `fill_interval` ≥ 50ms validation discipline (resolves §11.2c). |
| 3 | ADR-0116 | `tokenBucket` primitive + Option-A lazy-refill mechanics + monotonic-time semantics + LBP-1-adjacent declaration + empirical refill-timing tolerance ±10ms (resolves §11.7, narrows BRAINSTORM ±20ms hypothesis). |
| 4 | ADR-0117 | Per-route bucket isolation as ADR-0073 wholesale-override consequence (first stateful per-route filter; ADR-0073 amendment paragraph) (resolves §11.6). |
| 5 | ADR-0118 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension + `enforced == rate_limited` MVP invariant + future shadow-mode widening point + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registration (resolves §11.5). |
| 6 | ADR-0119 | Rate-limited response mechanics + body byte-exact `local_rate_limited` (18 bytes, no LF) + 4-header set lowercase wire-form (`content-length`, `content-type`, `date`, `server: envoy`) + 429 default status + `SendLocalReply` reuse from phase 09 fault precedent (resolves §11.3, §11.4, §11.8). |

ADR-0073 amendment paragraph: lands under ADR-0073's body in `DECISIONS.md` per the convention (per ADR-0117 = ADR-0073 amendment).

### 8.1 Consolidation candidates

Per the phase 10 SPEC §8.1 precedent, the SPEC author may consolidate ADRs at SPEC time when the would-be standalone ADR is a thin documentation artefact rather than a load-bearing decision. Phase 11 consolidations:

- **ADR-0120 (BRAINSTORM-anticipated omnibus deferral) → DROPPED.** The 14-field deferral list does not bear a load-bearing decision — it is a documentation artefact captured in §13.1's BEHAVIOR_CONTRACT subsection's "deferred field families" paragraph + §13.5 forward-pointer notes. ADR-0040 silent-ignore discipline is already established; spelling out 14 deferred fields under a new ADR-class commitment (analogous to fault's 8 separate ADR-0104 paragraphs) creates `DECISIONS.md` noise without adding decisions. Mirrors phase 10's drop of ADR-0114 (no-stats) per its §8.1.

The remaining 6 ADRs (ADR-0114..ADR-0119) are each load-bearing:

- **ADR-0114** captures a non-uniform package-naming choice with future-precedent implications.
- **ADR-0115** captures the 50ms filter-internal validation discipline (filter-internal vs PGV split).
- **ADR-0116** captures the LBP-1-adjacent declaration + the empirical timing tolerance tightening.
- **ADR-0117** is a multi-precedent ADR-0073 amendment (carries forward to all future stateful per-route filters).
- **ADR-0118** establishes a NEW filter-specific Prometheus tag-extractor registration (precedent-setting for future filters with their own stats).
- **ADR-0119** captures the wire-shape including the corrected `server: envoy` value and confirmed 18-byte body.

Each pin maps to ≥1 ADR; each ADR cites the pin(s) it resolves. Anticipated count is firm at 6.

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106)

Per ADR-0106(b) (no-sibling-stub discipline), this SPEC does NOT pre-author SPEC stubs for siblings (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `csrf`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts. The §9 heading at `ROADMAP.md` line 56 stays unchanged across phase 11's landing per ADR-0106(c). Phase 11's SPEC is the FOURTH validation of the §9 family-row discipline (phase 09 + 10 + 11 inheriting from cors @ 07.1 + first-row-establishment at 09 + second-iteration validation at 10 + third-iteration validation at 11).

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

### 10.1 Lifecycle correctness

- This SPEC author session lands SPEC.md (this file) + flips ROADMAP row 11 status `planned → in-progress` (or appends row 11 directly as `in-progress` since the row didn't exist at BRAINSTORM commit time per BRAINSTORM §10) + advances STATE.md to `lifecycle-state: spec-complete` + sets `next-skill: superpowers:writing-plans` + updates `last-commit` SHA.
- Per the phase 09 + 10 SPEC-commit pattern, NO ADRs are added to `DECISIONS.md` at SPEC commit. ADRs ADR-0114..ADR-0119 land during impl commits per the per-task SHA-fill discipline.
- Per the phase 09 + 10 SPEC-commit pattern, NO `BEHAVIOR_CONTRACT.md` edits land at SPEC commit. The §13 4-edit bundle lands at the phase-done commit per ADR-0052.
- Per user memory + phase 09 + 10 precedent: SPEC commit lands on the `phase-11-http-filter-local-ratelimit-spec` worktree branch; ff-merge into master happens after PLAN + impl + REVIEW + phase-done commits all stack on the same branch (or successor branches per the user's worktree-per-stage preference).

### 10.2 Empirical-pin discipline

- §11 carries verbatim observations from reference Envoy v1.37.2 (image SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` per `ENVOY_TARGET.md`; probe date 2026-05-05 per the SPEC drafting machine's clock).
- All 8 §9 BRAINSTORM empirical pins resolved IN-SESSION per ADR-0004 hard-gate.
- §1.1 enumerates the 3 amendments (§11.5 stat-label, §11.3 server-header, §11.7 tolerance) + 2 forward-carrying findings (§11.2c filter-internal-not-PGV, §11.2/§11.4 runtime-key-defaults-0%).

### 10.3 Scope envelope

- 5-consumed / 14-deferred field map matches BRAINSTORM §1.1 + §8.
- Differential fixture has 4 scenarios (no scenario drop unlike phase 10's 5→4); no scenario merge.
- Stat surface 22→26 (4 new counters); table extension at §13.2.
- ADR list 6 (ADR-0114..ADR-0119); ADR-0120 dropped per §8.1; ADR-0073 amendment paragraph lands per ADR-0117.
- Total LoC estimate ~1100–1400; task count estimate ~12–16; both below ADR-0045 split-trigger thresholds; phase stays single-row.

### 10.4 No 09 / 10-introduced regressions

- Phase 09's `fault` filter package: untouched.
- Phase 10's `header_mutation` filter package: untouched.
- Phase 10's framework deltas (`PerRouteConfig.ResolveAllTiers`, `RequestRouteConfigsAllTiers`, `ResponseRouteConfigsAllTiers`, `RegisterPerRouteValidator`): untouched and unused by phase 11. Their continued presence is expected (they are header_mutation-specific surface; no filter beyond header_mutation invokes them).
- Existing 13 differential fixtures (0000–0012): untouched and expected to stay green at phase 11 phase-done.
- 14 existing fuzzers: untouched and expected to stay green at phase 11 phase-done.

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 8 pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 + phase 10 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1 / 08.2 / 09 / 10 SPEC §11 confirmation).

**Probe configuration:** Reference Envoy booted under per-pin minimal bootstrap YAMLs via `docker run --rm --network=host -v /tmp/p11-pins:/etc/envoy:ro envoyproxy/envoy:v1.37.2 -c /etc/envoy/<file>.yaml --base-id <unique>`; admin and listener ports allocated within the 10000–10999 / 9901–9909 range; HTTP backend `python3 -m http.server <port>` (or header-echo equivalent for §11.8). Probe curl invocations issued from the same host. Capture transcripts at `/tmp/p11-pins/p*.{yaml,csv,bin,txt}` of the SPEC drafting session machine are transient artifacts not committed; the verbatim outputs below are the durable evidence per the 09 + 10 SPEC §11 discipline.

Probe date: **2026-05-05**.

### 11.1 Empirical pin #1 — `stat_prefix` PGV (resolves BRAINSTORM §9.P1)

**Probe configuration:** envoy.yaml with `local_ratelimit` typed_config; `stat_prefix` field omitted; all other fields valid (`max_tokens=10, tokens_per_fill=10, fill_interval=1s`). File: `p1-no-stat-prefix.yaml`.

**Verbatim Envoy boot-failure tail:**

```
[2026-05-05 11:26:47.610][1][critical][main] [source/server/server.cc:453] error
`goo.gle/debugproto: Proto constraint validation failed
(LocalRateLimitValidationError.StatPrefix: value length must be at least 1 characters)`
initializing config
```

**Conclusions (pinned):**
- (a) Envoy v1.37.2 REJECTS `stat_prefix` MISSING at **CONFIG-LOAD TIME** with a hard PGV error.
- (b) PGV constraint name: `LocalRateLimitValidationError.StatPrefix: value length must be at least 1 characters`. Initial expectation (reject at boot, PGV non-empty constraint typical) **CONFIRMED**.
- (c) envoy-go's `New` factory MUST mirror by validating `cfg.StatPrefix` non-empty at parse time; non-empty constraint is a PGV-class check that passes through proto unmarshal validation by default if envoy-go uses `protoreflect`-based PGV plumbing, or surfaces as a `New`-factory error otherwise. PLAN author confirms which mechanism produces the byte-equivalent surface (or whether envoy-go's `New` adds an explicit non-empty check — see §12 deferred decision 3).
- (d) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` PGV-table paragraph (§13.1) and ADR-0115 §Decision section.

### 11.2 Empirical pin #2 — `token_bucket` PGV (resolves BRAINSTORM §9.P2)

#### 11.2a — `max_tokens=0`

**Probe configuration:** Config with `max_tokens: 0, tokens_per_fill: 1, fill_interval: 1s, stat_prefix: test`. File: `p2a-max-tokens-zero.yaml`.

**Verbatim:**

```
[2026-05-05 11:27:10.533][1][critical][main] [source/server/server.cc:453] error
`goo.gle/debugonly: Proto constraint validation failed
(LocalRateLimitValidationError.TokenBucket: embedded message failed validation
| caused by TokenBucketValidationError.MaxTokens: value must be greater than 0)`
initializing config
```

**Conclusions:** REJECTED at boot. Constraint: `TokenBucketValidationError.MaxTokens: value must be greater than 0`. NOTE: the constraint is on the SHARED `TokenBucket` proto (`envoy.type.v3.TokenBucket.MaxTokens`), NOT local_ratelimit-specific. envoy-go's `New` validates `cfg.TokenBucket.MaxTokens > 0`.

#### 11.2b — `tokens_per_fill`

**(i) Field OMITTED**: Config `max_tokens: 5, fill_interval: 60s`, `tokens_per_fill` omitted, both runtime keys at 100%. Boot succeeded; 8 sequential reqs: reqs 1–5 return 200, reqs 6–8 return 429. Stats: `enabled=8, ok=5, rate_limited=3, enforced=3`. Conclusion: ACCEPTED at boot; default-fill behavior consistent with `tokens_per_fill = 1` (proto default). envoy-go's `New` accepts omitted field; uses 1 as the default.

**(ii) Field explicitly `0`**:

```
[2026-05-05 11:27:11.807][1][critical][main] [source/server/server.cc:453] error
`goo.gle/debugonly: Proto constraint validation failed
(LocalRateLimitValidationError.TokenBucket: embedded message failed validation
| caused by TokenBucketValidationError.TokensPerFill: value must be greater than 0)`
initializing config
```

REJECTED at boot. Constraint: `TokenBucketValidationError.TokensPerFill: value must be greater than 0`. envoy-go's `New` validates `cfg.TokenBucket.TokensPerFill > 0` after defaulting from omitted field.

#### 11.2c — `fill_interval` minimum

**Probe configuration:** Config with `max_tokens=1, tokens_per_fill=1`; vary `fill_interval` ∈ {`0.010s`, `0.020s`, `0.049s`, `0.050s`, `0.051s`}. Used `--mode validate` for fast iteration. Files: `p2c-{10,20,49,50,51}ms.yaml`.

**Verbatim (10ms / 20ms / 49ms — same error string):**

```
[...][critical][main] [source/server/config_validation/server.cc:76] error
initializing configuration '/etc/envoy/p2c-<N>ms.yaml':
local rate limit token bucket fill timer must be >= 50ms
```

**Verbatim (50ms / 51ms — config-validation pass; main dispatch loop reached on foreground run.)**

**Conclusions (pinned):**
- (a) **50ms minimum enforced.** `<50ms` rejected; `≥50ms` accepted.
- (b) **The check is FILTER-INTERNAL, NOT PGV.** Error path is `source/server/config_validation/server.cc:76` (the project's config-validation server), NOT `source/server/server.cc:453` (the PGV envelope). Error string is the bare `local rate limit token bucket fill timer must be >= 50ms`, NOT a wrapped PGV constraint. envoy-go MUST replicate this distinct error string + filter-internal validation discipline (run after proto unmarshal succeeds; before `runtimeConfig` build).
- (c) ADR-0115 records the filter-internal validation discipline separately from the PGV-table discipline.
- (d) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` PGV-table paragraph (§13.1) — annotated as filter-internal NOT PGV.

### 11.3 Empirical pin #3 — Rate-limited response wire shape (resolves BRAINSTORM §9.P3)

**Probe configuration:** Config: `max_tokens=1, tokens_per_fill=1, fill_interval=60s, stat_prefix=test`, `filter_enabled=100%`, `filter_enforced=100%`. Sent 2 sequential `curl -is http://localhost:10130/`. File: `p3-wire-shape.yaml`. Captured raw bytes via `xxd`.

**Verbatim hex of req-2 response (150 bytes):**

```
00000000: 4854 5450 2f31 2e31 2034 3239 2054 6f6f  HTTP/1.1 429 Too
00000010: 204d 616e 7920 5265 7175 6573 7473 0d0a   Many Requests..
00000020: 636f 6e74 656e 742d 6c65 6e67 7468 3a20  content-length:
00000030: 3138 0d0a 636f 6e74 656e 742d 7479 7065  18..content-type
00000040: 3a20 7465 7874 2f70 6c61 696e 0d0a 6461  : text/plain..da
00000050: 7465 3a20 5475 652c 2030 3520 4d61 7920  te: Tue, 05 May
00000060: 3230 3236 2031 313a 3333 3a30 3520 474d  2026 11:33:05 GM
00000070: 540d 0a73 6572 7665 723a 2065 6e76 6f79  T..server: envoy
00000080: 0d0a 0d0a 6c6f 6361 6c5f 7261 7465 5f6c  ....local_rate_l
00000090: 696d 6974 6564                           imited
```

**Body bytes alone (18 bytes; MD5 = `397e830923f3080ba63b3d38b53678ac`):**

```
6c 6f 63 61 6c 5f 72 61 74 65 5f 6c 69 6d 69 74 65 64
l  o  c  a  l  _  r  a  t  e  _  l  i  m  i  t  e  d
```

Last byte = `0x64` ('d'). **No trailing newline.**

**Conclusions (pinned):**
- (a) Status line: `HTTP/1.1 429 Too Many Requests\r\n`. Status text: `Too Many Requests` (RFC 7231 status text for 429).
- (b) Headers in EMISSION ORDER (lexicographic): `content-length: 18`, `content-type: text/plain`, `date: Tue, 05 May 2026 11:33:05 GMT`, `server: envoy`. All lowercase wire-form.
- (c) Header/body separator: `\r\n\r\n` (CRLF + CRLF).
- (d) Body literal: `local_rate_limited` (18 bytes; ASCII). **No trailing newline.** Initial expectation (`local_rate_limited`, 18 bytes, no LF) **CONFIRMED**.
- (e) Framing: **Content-Length: 18** (NOT chunked).
- (f) **`server: envoy`** — NOT `server: envoy-go` as BRAINSTORM hypothesized. envoy-go ALREADY emits `server: envoy` from its existing HCM `serverHeader()` returning literal `"envoy"` (per `internal/filter/hcm/codec.go:17` and `internal/filter/http/router/router.go:52`). NO envoy-go code change needed; SPEC §1.1 amendment carries the corrected wire shape forward.
- (g) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` rate-limited-response paragraph (§13.1) and ADR-0119 §Decision section.

### 11.4 Empirical pin #4 — Default `status.code` (resolves BRAINSTORM §9.P4)

Captured implicitly in §11.3 (status line of req-2). Config: `local_ratelimit` with NO `status` field at all.

**Conclusion:** **429 confirmed** (status line `HTTP/1.1 429 Too Many Requests`). Initial expectation **CONFIRMED**. PGV bound `[400, 600)` per the proto definition; envoy-go's `New` validates `cfg.Status.Code ∈ [400, 600)` if explicitly configured; defaults to 429 if omitted.

### 11.5 Empirical pin #5 — Stat names on `/stats/prometheus` (resolves BRAINSTORM §9.P5)

**Probe configuration:** Config `stat_prefix=test, max_tokens=100, tokens_per_fill=100, fill_interval=1s`, `filter_enabled=100%, filter_enforced=100%`. Sent 5 reqs (all 200). Scraped `/stats/prometheus`. File: `p5-stats.yaml`.

**Verbatim Prometheus output (5-req scenario):**

```
# TYPE envoy_http_local_rate_limit_enabled counter
envoy_http_local_rate_limit_enabled{envoy_local_http_ratelimit_prefix="test"} 5
# TYPE envoy_http_local_rate_limit_enforced counter
envoy_http_local_rate_limit_enforced{envoy_local_http_ratelimit_prefix="test"} 0
# TYPE envoy_http_local_rate_limit_ok counter
envoy_http_local_rate_limit_ok{envoy_local_http_ratelimit_prefix="test"} 5
# TYPE envoy_http_local_rate_limit_rate_limited counter
envoy_http_local_rate_limit_rate_limited{envoy_local_http_ratelimit_prefix="test"} 0
```

**Verbatim text-format `/stats`:**

```
test.http_local_rate_limit.enabled: 5
test.http_local_rate_limit.enforced: 0
test.http_local_rate_limit.ok: 5
test.http_local_rate_limit.rate_limited: 0
```

**Conclusions (pinned):**
- (a) **Exactly 4 counters per stat_prefix.** Names (lexicographic): `enabled`, `enforced`, `ok`, `rate_limited`. NO additional `local_rate_limit_*` family stats observed (no `near_limit`, no `total_pending`, no dynamic-metadata gauges). The 4-counter set is the complete surface.
- (b) **Prometheus label key: `envoy_local_http_ratelimit_prefix`** (filter-specific tag-extractor). NOT `envoy_http_conn_manager_prefix` as BRAINSTORM §9.P5 hypothesized. **MAJOR REVISION** per §1.1.
- (c) Text-format template: `<stat_prefix>.http_local_rate_limit.{enabled, enforced, ok, rate_limited}`. Note the underscore-separated `http_local_rate_limit` segment (matches Envoy's standard filter-stat-name convention).
- (d) Prometheus-format template: `envoy_http_local_rate_limit_{enabled, enforced, ok, rate_limited}{envoy_local_http_ratelimit_prefix="<stat_prefix>"}`.
- (e) **Tag-extraction collision quirk:** when `stat_prefix` matches an Envoy-internal tag-extractor name (e.g. literal `listener` matches `envoy.listener_address`), the Prometheus output is mangled — the metric name LOSES the `envoy_http_local_rate_limit_` prefix and gains an extra `envoy_listener_address` label. envoy-go's stat-emission must replicate this quirk for byte-equivalent fidelity, OR the differential fixture must avoid magic prefix names. Phase 11 SPEC takes the latter route: §7.4 fixture uses `foo`, `bar`, `baz`, `qux`, `strict` — none collide. Future phases extending stat-prefix coverage may need to address the collision (deferred to a stat-name-discipline phase if it becomes load-bearing).
- (f) envoy-go's `internal/admin/stats.go` (or wherever the project registers tag-extractors) needs a NEW pattern for the `local_rate_limit` filter — the tag-extractor must extract the `<stat_prefix>` segment from `<stat_prefix>.http_local_rate_limit.<counter>` into the `envoy_local_http_ratelimit_prefix` label. PLAN author resolves the precise registration site (see §12 deferred decision 1).
- (g) Lands in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26 table (§13.2) and ADR-0118 §Decision section.

### 11.6 Empirical pin #6 — Per-route override observability (resolves BRAINSTORM §9.P6)

**Probe configuration:** Listener `local_ratelimit` with `stat_prefix=foo (chosen non-magic), max_tokens=10, fill 1s, filter_enabled+enforced=100%`. Route `/strict` with `typed_per_filter_config.envoy.filters.http.local_ratelimit` overriding to `stat_prefix=strict, max_tokens=1, fill 60s, filter_enabled+enforced=100%`. Route `/loose` has no override. Sent 5 reqs to `/loose`, then 3 reqs to `/strict`. File: `p6b-per-route-foo.yaml`.

**Verbatim Prometheus output:**

```
# TYPE envoy_http_local_rate_limit_enabled counter
envoy_http_local_rate_limit_enabled{envoy_local_http_ratelimit_prefix="foo"} 5
envoy_http_local_rate_limit_enabled{envoy_local_http_ratelimit_prefix="strict"} 3
# TYPE envoy_http_local_rate_limit_enforced counter
envoy_http_local_rate_limit_enforced{envoy_local_http_ratelimit_prefix="foo"} 0
envoy_http_local_rate_limit_enforced{envoy_local_http_ratelimit_prefix="strict"} 2
# TYPE envoy_http_local_rate_limit_ok counter
envoy_http_local_rate_limit_ok{envoy_local_http_ratelimit_prefix="foo"} 5
envoy_http_local_rate_limit_ok{envoy_local_http_ratelimit_prefix="strict"} 1
# TYPE envoy_http_local_rate_limit_rate_limited counter
envoy_http_local_rate_limit_rate_limited{envoy_local_http_ratelimit_prefix="foo"} 0
envoy_http_local_rate_limit_rate_limited{envoy_local_http_ratelimit_prefix="strict"} 2
```

**Conclusions (pinned):**
- (a) **Per-route counters are INDEPENDENT** of listener counters. /strict reqs increment ONLY the `strict` series (3 enabled, 1 ok, 2 rate_limited, 2 enforced). /loose reqs increment ONLY the `foo` series (5 enabled, 5 ok).
- (b) **Wholesale-override semantics CONFIRMED.** Per-route TPFC FULLY REPLACES the listener filter for matched requests; listener-level `*tokenBucket` + `*filterStats` are NOT touched for /strict reqs. This empirically validates ADR-0073 wholesale-override carrying through to STATEFUL per-route resources, codified in ADR-0117 as an ADR-0073 amendment paragraph.
- (c) Each `stat_prefix` gets its own independent Prometheus counter series with the `envoy_local_http_ratelimit_prefix` label scoping. envoy-go's stat-emission must replicate this independence: per-route `runtimeConfig` carries its own `*filterStats` populated with the per-route `stat_prefix`.
- (d) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` per-route paragraph (§13.1) and ADR-0117 §Decision section.

### 11.7 Empirical pin #7 — Refill timing tolerance (resolves BRAINSTORM §9.P7)

**Probe configuration:** Config `max_tokens=1, tokens_per_fill=1, fill_interval=0.200s, stat_prefix=test, filter_enabled+enforced=100%`. Two driver scripts:
- `p7-driver.py`: 11 delay values × 3 iterations = 33 trials, sweep 180→400ms.
- `p7b-driver.py`: 9 delay values × 5 iterations = 45 trials, tight band 196→204ms.
- Each trial: full envoy restart (fresh state), then probe at t=0 (drain), t=10ms (confirm empty), t=DELAY (refill check).

**Combined results:**

| delay (ms) | trials | code at t=DELAY |
|---|---|---|
| 180, 190, 195, 196, 197, 198, 199 | 24 | **all 429** (still rate-limited) |
| 200, 201, 202, 203, 204, 205, 210, 220, 230, 250, 300, 400 | 52 | **all 200** (refilled) |

Boundary measurement: `actual_delay_ms` (between t=0 and the t=DELAY req hitting envoy) was always `delay + 0.06..0.09ms` — i.e. negligible HTTP RT overhead.

**Conclusions (pinned):**
- (a) **Refill is sharp at the configured `fill_interval` wall-clock boundary.** No upward jitter observed: at delay=200.06ms a refill is always honored; at delay=199.07ms it never is.
- (b) Empirical envelope: ±0ms upward (refill at exactly `fill_interval`), with a measurement-floor of ~70μs (HTTP RT) plus Python `time.sleep` resolution ~1–5ms.
- (c) **BRAINSTORM ±20ms hypothesis is conservative; SPEC narrows to ±10ms** (per §1.1 amendment) — bounded by the ~5ms measurement floor + a small CI-jitter safety margin.
- (d) envoy-go's lazy-refill primitive on access (single `sync.Mutex` per bucket; `time.Now().UnixNano()` monotonic clock) computes refill quanta as `floor((now - last_refill) / fill_interval) >= 1` — matches Envoy's behavior naturally.
- (e) **No active timer drives the refill** in reference Envoy — it happens on the next access ≥ `last_refill + fill_interval`, matching envoy-go's design.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## Timing tolerances` (§13.3) and ADR-0116 §Decision section.

### 11.8 Empirical pin #8 — Headers on success path (resolves BRAINSTORM §9.P8)

**Probe configuration:** Config `max_tokens=100, tokens_per_fill=100, fill_interval=1s, stat_prefix=test, filter_enabled=100%, filter_enforced=100%`. Backend: header-echo Python server. Sent 1 successful req. File: `p8-success-headers.yaml`.

**Verbatim:**

```
HTTP/1.1 200 OK
server: envoy
date: Tue, 05 May 2026 11:35:50 GMT
content-type: application/json
x-backend-marker: yes
x-envoy-upstream-service-time: 0
transfer-encoding: chunked

{
  "path": "/test",
  "headers": {
    "host": "localhost:10180",
    "user-agent": "curl/8.5.0",
    "accept": "*/*",
    "x-forwarded-proto": "http",
    "x-request-id": "f36bdd44-6945-4f84-bf2e-9c6349655c0b",
    "x-envoy-expected-rq-timeout-ms": "15000"
  }
}
```

**Conclusions (pinned):**
- (a) **NO `x-ratelimit-*` headers on either side** (downstream response or upstream request) on the allow path. Initial expectation **CONFIRMED**.
- (b) Downstream response headers added by envoy: `server`, `date`, `x-envoy-upstream-service-time` (HCM-level, NOT rate-limit-specific).
- (c) Upstream request headers added by envoy: `x-forwarded-proto`, `x-request-id`, `x-envoy-expected-rq-timeout-ms` (router/HCM standard, NOT rate-limit-specific).
- (d) `enable_x_ratelimit_headers` defaults OFF (per the proto default + the empirical observation); IETF X-RateLimit-* injection requires explicit configuration. Deferred per §2.1 cluster 7 / §13.5 (ADR-0120 omnibus dropped per §8.1; deferral lives inline).
- (e) envoy-go MVP's silent-ignore of `enable_x_ratelimit_headers` is byte-equivalent on the success path (NO headers added on either side regardless of the field's presence/absence). Confirmed.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` allow-path paragraph (§13.1) and ADR-0119 §Decision section.

### 11.9 Synchronization with `BEHAVIOR_CONTRACT.md`

The 8-pin empirical block above is the source of truth for the §13 4-edit bundle (which lands at the phase-done commit per ADR-0052):

- §13.1 NEW subsection draws from §11.1 (PGV non-empty), §11.2 (token-bucket PGV + filter-internal 50ms), §11.3 (rate-limited wire shape + 4 headers + 18-byte body), §11.4 (default 429), §11.6 (per-route wholesale-override), §11.8 (allow-path no headers).
- §13.2 22→26 stat-table extension draws from §11.5 (4 counter names + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix`).
- §13.3 timing-tolerance row draws from §11.7 (±10ms fill_interval refill boundary; sharp lazy-refill semantics).
- §13.4 equivalence-matrix new row draws from §11.3, §11.7, §11.6 + the fixture 0013 4-scenario topology.
- §13.5 forward-pointer notes draw from the 14-field deferral (§2.1 / ADR-0120 collapse) + the runtime-key default-0% divergence-window note (§1.1 amendment).

---

## 12. Deferred decisions (the planner / implementer settles these)

The following 5 decisions are SPEC-deferred — the SPEC author has bounded each but leaves the precise discipline for the PLAN author or impl-time settlement. Each maps to ≤1 ADR (some fold inline into existing ADRs).

**D1. Tag-extractor registration site for `envoy_local_http_ratelimit_prefix`.** §11.5 establishes the filter-specific Prometheus tag-extractor. The registration site is one of: (a) `internal/admin/stats.go` (extending an existing tag-extractor table); (b) within `internal/filter/http/localratelimit/` package's `init()` function (filter-package-local registration consuming a registry-pattern primitive from `internal/admin`); (c) a new file `internal/admin/tag_extractors_local_ratelimit.go` (split for organization). PLAN author chooses the most natural location given the existing tag-extractor mechanism (which the SPEC author has not exhaustively reviewed in this session). ADR-0118 captures the chosen site.

**D2. Filter-callback wiring.** §6.3 declares phase 11 sets only `dcb` (DecoderFilterCallbacks); the precise hook (`OnNewStream`, factory closure, etc.) follows the existing 07.1 framework convention. PLAN author confirms the framework's exposed callback-setup hook against the existing `internal/filter/http/cors/`, `fault/`, `header_mutation/` patterns.

**D3. PGV plumbing for `stat_prefix` non-empty + `max_tokens > 0` + `tokens_per_fill > 0`.** §11.1 / §11.2a / §11.2b-ii establish the PGV constraints; envoy-go may surface them via (a) `protoreflect`-based PGV runtime checks (if such plumbing already exists for cors / fault / header_mutation), or (b) explicit non-empty / `> 0` checks in the `New` factory after `tc.UnmarshalTo`. PLAN author surveys existing PGV discipline (likely option (b) given prior phases' explicit-check patterns) and applies uniformly. ADR-0115 captures the chosen mechanism.

**D4. Scenario 3 retry-with-deadline harness option.** §7.2 driver outline + §7.5 timing tolerance establish ±10ms as the SPEC default. If the fixture flakes during phase 11 impl under heavy CI load, PLAN author may adopt either (a) a wider tolerance (e.g. ±20ms back to BRAINSTORM hypothesis) or (b) a retry-with-deadline harness around scenario 3's t=250ms boundary check. ADR-0116 stays at ±10ms as the default; PLAN author records the chosen option in PROGRESS.md or via an ADR-0116 amendment paragraph if widened.

**D5. Test-only clock injection for wallclock-monotonicity testing.** §2.2 declares wallclock backward-jump simulation OUT of scope for phase 11 unit tests. If the PLAN author chooses to add such tests (mockable `time.Now()` via a `clock` interface threaded through `tokenBucket`), ADR-0116 amends to capture the discipline; otherwise the SPEC's position holds (no test-only clock injection in MVP). Future hardening pass may revisit.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

The 4-edit bundle below is the verbatim Markdown patch applied to `docs/envoy-go/BEHAVIOR_CONTRACT.md` at the phase-done commit. NOT applied at SPEC commit (per phase 09 / 10 precedent).

### 13.1 `## HTTP filter chain ### envoy.filters.http.local_ratelimit` NEW subsection

The new subsection lands UNDER the existing `## HTTP filter chain` umbrella, AFTER the existing `### envoy.filters.http.fault` (phase 09) and `### envoy.filters.http.header_mutation` (phase 10) subsections. Verbatim Markdown shape:

```markdown
### envoy.filters.http.local_ratelimit

Phase 11 ships `envoy.filters.http.local_ratelimit` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes 5 of 19 top-level fields and silent-ignores the other 14 per the deferral list below + ADR-0040 silent-ignore discipline.

**Consumed fields (5):**

| Proto field | envoy-go behavior |
|---|---|
| `stat_prefix` | Required (PGV non-empty per ADR-0115). Used as the stat-name prefix and the Prometheus tag-extracted label `envoy_local_http_ratelimit_prefix` value. |
| `token_bucket.max_tokens` | Required (PGV `> 0` per shared `TokenBucket` proto). Initial bucket fill = `max_tokens`. |
| `token_bucket.tokens_per_fill` | Optional; default `1` (matches Envoy proto default). PGV `> 0` if explicitly set. |
| `token_bucket.fill_interval` | Required. Filter-internal validation: `≥ 50ms` (matches Envoy v1.37.2 filter-internal check; error string verbatim: `local rate limit token bucket fill timer must be >= 50ms`). NOT a PGV constraint per ADR-0115. |
| `status.code` | Optional; default 429. PGV `[400, 600)` if explicitly set. Status text follows RFC 7231 (`Too Many Requests` for 429). |

**Silent-ignored fields (14, organized by family):**

- *Descriptor-action* (4): `descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors` — couples to `global_ratelimit` future phase.
- *Runtime + shadow-mode* (3): `filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced` — couples to Runtime + hot restart family. **Note:** reference Envoy defaults `filter_enabled` + `filter_enforced` to 0% (off); fixture configs MUST set both to 100% explicitly for differential equivalence. envoy-go's silent-ignore is equivalent to "always-100%" — divergence-window applies if user omits these fields outside the fixture context.
- *xDS cluster-state* (1): `local_cluster_rate_limit` — couples to xDS / dynamic config family.
- *Response-side header injection* (1): `response_headers_to_add`.
- *Per-connection lifecycle* (1): `local_rate_limit_per_downstream_connection`.
- *Multi-stage limiting* (1): `stage` — couples to descriptor-action subsystem.
- *X-RateLimit headers + vh policy* (2): `enable_x_ratelimit_headers`, `vh_rate_limits`.
- *gRPC trailer mapping* (1): `rate_limited_as_resource_exhausted` — couples to gRPC family.

**Token-bucket primitive:** Lazy refill on access (Option A); single `sync.Mutex` per bucket; no per-bucket goroutine; `time.Now().UnixNano()` monotonic clock (Go ≥1.9 guarantee). Hot-path: 5–10 nanoseconds typical (compute elapsed → integer-divide by `fill_interval` → conditional add → decrement).

**Per-route override semantics:** Wholesale-override per ADR-0073 + ADR-0117 (ADR-0073 amendment). Each `LocalRateLimitPerRoute` TPFC entry runs through `New` at config-load time, allocating its own `*runtimeConfig` + own `*tokenBucket` + own `*filterStats`. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration) picks the most-specific config per request. Listener-level state is NOT touched for per-route reqs (per §11.6 empirical confirmation).

**Rate-limited response wire shape (per §11.3 empirical):**

- Status: `429 Too Many Requests`
- Body: `local_rate_limited` (18 bytes ASCII; NO trailing newline)
- Headers in lexicographic order: `content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Framing: Content-Length

**Allow-path response (per §11.8 empirical):** NO `x-ratelimit-*` headers added by the filter on either side (request or response). Standard HCM/router headers (`server`, `date`, `x-envoy-*`) are unrelated to this filter.

**MVP invariant:** `enforced == rate_limited` at every step (per ADR-0118). Future shadow-mode phase widens to `enforced ≤ rate_limited` when `filter_enforced` runtime-key support lands.
```

### 13.2 `## Stat-name mapping ### 22-name table` 22→26 extension

The existing 22-name table (extended by phase 09 from 17→22; unchanged through phase 10) gains 4 new rows. Verbatim Markdown patch:

```markdown
... [existing 22 rows] ...
| `<stat_prefix>.http_local_rate_limit.enabled`     | counter | filter | local_ratelimit | every request reaching the filter (§11.5) |
| `<stat_prefix>.http_local_rate_limit.ok`          | counter | filter | local_ratelimit | request not rate-limited (`tryConsume` → true; §11.5) |
| `<stat_prefix>.http_local_rate_limit.rate_limited`| counter | filter | local_ratelimit | request rate-limited (`tryConsume` → false; §11.5) |
| `<stat_prefix>.http_local_rate_limit.enforced`    | counter | filter | local_ratelimit | request rate-limited AND enforced (lockstep with `rate_limited` under MVP per ADR-0118; §11.5) |
```

Plus the heading update: `### 22-name table` → `### 26-name table`. Plus the table preamble note about the new filter-specific Prometheus tag-extractor:

```markdown
**Filter-specific Prometheus tag-extractor (added in phase 11 per ADR-0118):** `<stat_prefix>.http_local_rate_limit.<counter>` extracts the `<stat_prefix>` segment into the Prometheus label `envoy_local_http_ratelimit_prefix`. NOTE: tag-extraction collisions occur if `<stat_prefix>` matches an Envoy-internal tag-extractor name (e.g. `listener` collides with `envoy.listener_address`); the differential fixture 0013 uses safe values (`foo`, `bar`, `baz`, `qux`, `strict`).
```

### 13.3 `## Timing tolerances` extension

Verbatim Markdown patch (new row appended to the existing timing-tolerances table):

```markdown
| fixture 0013 scenario 3 (refill-after-fill_interval) | t=250ms refill boundary | ±10ms wall-clock | per ADR-0116 + §11.7 empirical (BRAINSTORM ±20ms hypothesis narrowed; PLAN author may widen back to ±20ms with retry-with-deadline harness if CI flakes per §12 D4) |
```

### 13.4 `## Equivalence Matrix` new row (verbatim table-row patch)

Verbatim Markdown patch (new row appended to the existing equivalence-matrix table):

```markdown
| 0013-http-local-ratelimit | scenario1: 5 reqs / cap=10 / fill=10 / interval=1s — 5×200; scenario2: 5 reqs / cap=2 — 2×200 + 3×429 (§11.3 wire shape); scenario3: 3 reqs / cap=1 / fill=1 / interval=200ms (refill ±10ms per §11.7); scenario4: 3+3 reqs interleaved /strict + /loose — wholesale-override (§11.6) | per-scenario tolerance per §13.3 timing-tolerances; lowercase wire-form 4-header set on 429; counter deltas across 4 stat names with `envoy_local_http_ratelimit_prefix` Prometheus label |
```

### 13.5 Forward-pointer notes (per BRAINSTORM §11 inline supersessions/amendments)

Verbatim Markdown patch (appended to the existing `## Forward-pointer notes` section):

```markdown
### Phase 11 forward-pointer notes

**Deferred field families** (silent-ignored per ADR-0040; see §13.1 / phase 11 SPEC §2.1 for the full 14-field list):

- Descriptor-action subsystem (4 fields) → couples to `global_ratelimit` future phase under `BOOTSTRAP_PROMPT.md` §9 HTTP filters family.
- Runtime + shadow-mode subsystem (3 fields, including `filter_enabled` and `filter_enforced` `RuntimeFractionalPercent` fields) → couples to Runtime + hot restart family. **Divergence-window:** envoy-go silent-ignores these fields; reference Envoy defaults both to 0% (off). Differential fixture configs MUST set both to 100% explicitly; users running envoy-go with these fields set to non-100% values will diverge from Envoy (envoy-go behaves as always-100%, Envoy honors the percentage).
- xDS cluster-state (1 field: `local_cluster_rate_limit`) → couples to xDS / dynamic config family.
- Response-side header injection (1 field: `response_headers_to_add`) → standalone follow-on.
- Per-connection lifecycle (1 field: `local_rate_limit_per_downstream_connection`) → standalone follow-on.
- Multi-stage limiting (1 field: `stage`) → couples to descriptor-action subsystem.
- X-RateLimit headers + vh policy (2 fields: `enable_x_ratelimit_headers`, `vh_rate_limits`) → standalone follow-on.
- gRPC trailer mapping (1 field: `rate_limited_as_resource_exhausted`) → couples to gRPC family.

**Tag-extraction collision quirk:** when `local_ratelimit.stat_prefix` matches an Envoy-internal tag-extractor name (`listener`, `http`, `cluster`, etc.), Envoy v1.37.2 mangles the Prometheus metric name. envoy-go's tag-extractor registration replicates the standard non-collision case; collision-mangling parity is OUT of scope for phase 11 (SPEC's position; see §1.1 amendment + §11.5 conclusions (e)).
```

---

## 14. Testing strategy (per BRAINSTORM §6 + §11 amendments)

### 14.1 Unit tests (`internal/filter/http/localratelimit/local_ratelimit_test.go`)

Five test groups (~200 LoC total):

- **Group 1 — `tokenBucket` mechanics:** test `tryConsume` lazy-refill arithmetic across boundary cases (no refill, single-quantum refill, multi-quantum refill cap, max-cap clamp, negative-elapsed safety check). Constructor parameters explicitly varied.
- **Group 2 — `New` factory PGV + filter-internal validation:** test that `New` rejects empty `stat_prefix` (per §11.1), `max_tokens=0` (§11.2a), `tokens_per_fill=0` explicit (§11.2b-ii), `fill_interval<50ms` (§11.2c filter-internal), `status.code` outside `[400, 600)` (§11.4 PGV). Also test acceptance of omitted `tokens_per_fill` (§11.2b-i) defaulting to 1.
- **Group 3 — `runtimeConfig` parsing:** test that all 14 deferred fields are silent-ignored (no warnings, no rejections) for representative non-empty values. Test that `LocalRateLimitPerRoute` parsing recursively builds a `*runtimeConfig` via the same `New` (per ADR-0117).
- **Group 4 — `DecodeHeaders` integration:** test the full DecodeHeaders flow with a mock `DecoderFilterCallbacks` for both allow path (counters incremented; Continue returned) and rate-limited path (counters incremented including `rateLimited` + `enforced` lockstep; `SendLocalReply(429, body, 1-header set)` invoked; StopIteration returned).
- **Group 5 — Per-route bucket independence:** test that two `runtimeConfig` instances built from independent `New` invocations carry independent `*tokenBucket` + `*filterStats` pointers; counter increments on one do not affect the other (validates §11.6 empirical).

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/localratelimit/...` green. `go vet`, `golangci-lint run` clean. Mutex hot-path is race-clean by construction (`tryConsume` is the sole writer; single `sync.Mutex` discipline).

### 14.3 Fuzzers

New fuzzer `FuzzLocalRateLimitConfigParse` in `internal/filter/http/localratelimit/fuzz_test.go`:

```go
func FuzzLocalRateLimitConfigParse(f *testing.F) {
    f.Add(...)  // a few well-formed seeds
    f.Fuzz(func(t *testing.T, raw []byte) {
        any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
        _, _ = New(envoyhttp.FactoryCtx{...}, any)
        // expectation: no panic, no goroutine leak, no resource leak
    })
}
```

This is the 15th fuzzer in the repo (14 existing per phase 10 phase-done + this new one). Fuzz budget: 30s per the existing per-phase fuzzer gate.

### 14.4 Existing fuzzers re-run

All 14 existing fuzzers (FuzzBootstrapConfigParse, FuzzCORSConfigParse, FuzzFaultConfigParse, FuzzHeaderMutationConfigParse, FuzzConfigDumpFormat, FuzzAccessLogFormat, etc.) continue to pass at 30s budget. Phase 11 introduces no shared fuzz-input surface changes that would invalidate existing fuzzers.

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin; phase 11 introduces no HTTP/2 stack changes (the rate-limit filter operates above the codec layer per §5.6 concurrency model).

### 14.6 Differential 0000–0012 + 0013

13 prior fixtures (0000-tcp-echo through 0012-http-header-mutation) continue to pass; phase 11 adds the new `0013-http-local-ratelimit` (4 scenarios per §7.1). Total wallclock estimated 40–50s for 14 fixtures (phase 10 reported 39.76s for 13 fixtures; fixture 0013's scenario 3 adds 250ms + scenario 4's interleaved batch adds modest overhead).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Per §3 above:

- Gate A: build / vet / lint clean.
- Gate B: race-test pass on all 34 packages.
- Gate C: h2spec 53/53 PASS.
- Gate D: 15 fuzzers green at 30s budget.
- Gate E: 14 differential fixtures 0000–0013 PASS.
- Gate F: BEHAVIOR_CONTRACT.md populated with §13's 4-edit bundle.

All six gates green at the phase-done commit.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

The phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.5 review session) confirms:

- [ ] `internal/filter/http/localratelimit/` package exists with files matching §4.1 (allowing PLAN-author file split per §4.1's note).
- [ ] `cmd/envoy-go/main.go` registers `localratelimit.New` against `localratelimit.TypeURL` before `httpReg.Freeze()`, alphabetical-after-router insertion ordering.
- [ ] Token-bucket primitive matches §6.4 (lazy-refill on access; single mutex; monotonic-time arithmetic).
- [ ] `runtimeConfig` shape matches §6.2 (5 consumed fields; 14 silent-ignored).
- [ ] `DecodeHeaders` body matches §6.5 (Continue allow path; SendLocalReply + StopIteration rate-limited path; lockstep `enforced == rate_limited` counter increments).
- [ ] Per-route override semantics match §5.5 + §11.6 (independent bucket per per-route TPFC; listener-level state not touched).
- [ ] Stat surface 22→26 with the new filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registered (D1 settled).
- [ ] Rate-limited response wire shape matches §11.3 (4 headers in order; 18-byte `local_rate_limited` body; status `429 Too Many Requests`; Content-Length framing; `server: envoy`).
- [ ] Differential fixture 0013 4 scenarios green (per-scenario tolerance per §7.1).
- [ ] `FuzzLocalRateLimitConfigParse` green at 30s budget (15 fuzzers total).
- [ ] All 13 prior differential fixtures still green; 14 prior fuzzers still green; h2spec 53/53 still PASS.
- [ ] `BEHAVIOR_CONTRACT.md` populated with §13's 4-edit bundle at phase-done commit.
- [ ] ADRs ADR-0114..ADR-0119 (6 ADRs) authored in `DECISIONS.md`; ADR-0073 amendment paragraph appended per ADR-0117.
- [ ] ROADMAP row 11 status `in-progress → done` at phase-done commit.
- [ ] `STATE.md` updated to `lifecycle-state: phase-11-complete; awaiting next planning` at phase-done commit; SHA-fill applied per the phase-04..10 SHA-fill convention.
- [ ] Phase 11 SPEC's §1.1 amendments (3 revisions + 2 forward-carrying findings) all faithfully reflected in implementation + fixture + BEHAVIOR_CONTRACT.

---

## 16. References

- `docs/envoy-go/phases/11-http-filter-local-ratelimit/BRAINSTORM.md` (605 lines; phase 11 brainstorm; this directory).
- `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` (1348 lines; structural precedent).
- `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (1305 lines; first §9 family-row precedent; SendLocalReply + StopIteration pattern source).
- `docs/envoy-go/BOOTSTRAP_PROMPT.md` (project-wide doctrine; §4 invariants; §6 split discipline; §7.5 phase-done gates; §8 MVP-trunk-close; §9 family expansion).
- `docs/envoy-go/MISSION.md` §2.2 (non-purpose: NOT mirroring Envoy C++ source structure).
- `docs/envoy-go/ENVOY_TARGET.md` (reference Envoy v1.37.2 image SHA pin per ADR-0008).
- `docs/envoy-go/DECISIONS.md` ADR-0008 (Envoy pin), ADR-0040 (silent-ignore discipline), ADR-0045 (split-by-surface release valve), ADR-0052 (BEHAVIOR_CONTRACT in-place edit at phase-done), ADR-0072 (extension-registry boot-fail-fast), ADR-0073 (per-route wholesale-override), ADR-0074 (cors no-stats analogue), ADR-0100 (FactoryCtx 3-field shape), ADR-0102 (terminal-replace + StopIteration localReplyDone gate), ADR-0103 (fault abort wire shape — body byte-exact precedent), ADR-0104 (fault deferral ADR pattern), ADR-0106 (flat top-level rows for §9 family), ADR-0108..ADR-0113 (phase 10 ADRs).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ## HTTP filter chain (existing umbrella; phase 11 adds `### envoy.filters.http.local_ratelimit` subsection per §13.1).
- `internal/filter/http/perroute.go:103–128` (existing `PerRouteConfig.Resolve`; phase 11 reuses).
- `internal/filter/http/fault/fault.go:321` (SendLocalReply + StopIteration precedent; `OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}` shape mirrored by phase 11).
- `internal/filter/hcm/codec.go:17` (`serverHeader()` returning literal `"envoy"` — confirms §1.1 amendment).
- `internal/filter/http/router/router.go:52` (router's `serverHeader()` returning literal `"envoy"` — confirms §1.1 amendment).
- Reference Envoy v1.37.2 proto: `envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit` (19-field message; phase 11 consumes 5).
- Reference Envoy v1.37.2 proto: `envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimitPerRoute` (1-field message: `rate_limit *LocalRateLimit`; phase 11 parses recursively).
- Reference Envoy v1.37.2 source: `source/server/config_validation/server.cc:76` (filter-internal 50ms minimum check on `fill_interval`; per §11.2c).

## End of phase 11 SPEC.
