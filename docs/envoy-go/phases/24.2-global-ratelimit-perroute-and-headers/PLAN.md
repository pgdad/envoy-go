# Phase 24.2 — `envoy.filters.http.ratelimit` (remaining actions + X-RateLimit headers + `RateLimitPerRoute`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete `envoy.filters.http.ratelimit` (the SEVENTEENTH §9 production HTTP filter, GLOBAL rate limit) on top of 24.1's core decision path — land the **remaining 5 descriptor actions** (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`) + the **X-RateLimit DRAFT_VERSION_03 response headers** + **`RateLimitPerRoute`** (the NEW 10th canonical per-route shape + the ADR-0125 roster amendment 9 → 10) + the **`stage` multi-stage bucketing path** + the **Axis-B `vh_rate_limits` cross-tier composition table** + scenarios (f) `vh_inclusion` + (g) `x_ratelimit_headers` to the EXISTING `0032-http-ratelimit/` (no new fixture dir) + EXTENDS scenario (d) `descriptor_actions` to all 10 actions + extends the existing `FuzzRateLimitConfigParse` corpus (no new fuzzer). Flips ROADMAP row 24.2 `planned → done` AND parent row 24 `in-progress → done` [ROLLUP] at the phase-done commit — verifying the FULL parent §15 acceptance UNION.

**Architecture:** EXTENDS the 24.1-landed `internal/filter/http/ratelimit/` package (no new top-level Go package). NEW files: `encode.go` + `headers.go` (X-RateLimit emission on all dispositions when `enable_x_ratelimit_headers == DRAFT_VERSION_03`, MIN-status across multi-descriptor responses + `;w=`/`;name=` quota suffix + unit→seconds map per parent §4.7 + AMEND-8) + `compiled_perroute.go` (`RateLimitPerRoute` TPFC compile per parent §5.3 / ADR-0199 — `vh_rate_limits` honored, `override_option` PARSE-ACCEPTED-but-INERT per AMEND-4, route-additional `rate_limits[]` Axis-A early-return, per-route `domain` override). MODIFIED files: `descriptors.go` (replace 24.1's `actionUnsupportedAt241` stub with the 5 remaining actions + AMEND-11 key defaults `query_parameters→"query_param"` SINGULAR / `query_parameter_value_match→"query_match"` / `metadata` requires config key / `masked_remote_address` CIDR-masking / `source_cluster` always-true; add the §4.4 stage multi-bucket selection; add the §4.3 Axis-B cross-tier composition table) + `compiled_config.go` (parse-time stage bucketing; thread per-route lookup into config build) + `dispositions.go` (replace the 24.1 X-RateLimit STUB with the real injection point — store the response descriptor-statuses, apply via encoder hook) + `decode_headers.go` (consume `RateLimitPerRoute` via `f.dcb.RequestRouteConfig()`; Axis-A early-return; per-route `domain` override) + `internal/filter/hcm/` (TPFC-resolver registration for the `RateLimitPerRoute` TypeURL — the 10th canonical). The descriptor engine remains filter-owned (the framework surfaces raw policy + the TPFC resolver does the typed unmarshal; the §4 engine turns actions into descriptors per AMEND-9). The `metadata`-action value-extraction uses `streamInfo().dynamicMetadata()` for DYNAMIC + route-metadata for ROUTE_ENTRY — confirmed at IMPL against the existing stream-info accessor (parent §12 item 2). The X-RateLimit byte format is byte-pinned against reference Envoy v1.37.2 captured headers (parent §12 item 5) at `headers_test.go`. The 24.2 phase-done commit lands ADR-0199 (FULL §Decision + §Consequences) + the ADR-0197 IN-PLACE §Decision amendment (X-RateLimit + remaining-actions slice) + the ADR-0125 §(xv) AMENDMENT 9 → 10 (anchored in ADR-0199) + the BEHAVIOR_CONTRACT completion bundle (per-route + X-RateLimit allow-list + descriptor-engine completion).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 (proto pin per ADR-0008/ADR-0051; `envoy/extensions/filters/http/ratelimit/v3` for `RateLimitPerRoute` — verified present in v1.32.4 with the AMEND-4-confirmed 4-field shape `vh_rate_limits` + `override_option` [INERT `[#not-implemented-hide:]`] + `rate_limits[]` + `domain`; `envoy/extensions/common/ratelimit/v3` for `RateLimitDescriptor_Entry` + the per-descriptor `RateLimitResponse_DescriptorStatus` with `current_limit`/`limit_remaining`/`duration_until_reset`/`quota`; `envoy/type/v3.RateLimitUnit` for the unit→seconds map per AMEND-8; `envoy/config/route/v3` for `RouteAction.IncludeVhRateLimits` legacy force-include per AMEND-5; `envoy/config/core/v3.Metadata` + the existing `streamInfo().dynamicMetadata()` accessor for the `metadata` action's value-extraction per AMEND-11). Reuses `internal/filter/http/ratelimit/` (the 24.1 package), `internal/filter/http` (`DecoderFilterCallbacks` + `EncoderFilterCallbacks`; `RequestRouteConfig()` TPFC resolver path per ADR-0073), `internal/filter/hcm` (TPFC compiler registration alongside the existing `RateLimitPerRoute`-adjacent per-route filters like `header_mutation`/`oauth2`/`lua`), `internal/stats`, `internal/cluster`. Reuses the 24.1-landed shared fake gRPC `RateLimitService` (`test/helpers/ratelimitgrpc/`) for the fixture extensions; the fake's deterministic-script map gets extended at Task 6 to emit per-descriptor statuses with `current_limit`/`limit_remaining`/`duration_until_reset` for scenario (g). golangci-lint 1.64.8 (ADR-0009). Reference Envoy `v1.37.2` (ADR-0008/ENVOY_TARGET.md). Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext synthetic backend (no new listener topology).

---

## Scope check — why phase 24.2 ships as one sub-phase row (ADR-0045 split-gate re-verification)

Phase 24 was SPLIT at PLAN time (the FIRST PLAN-time ADR-0045 split application; ADR-0201) into `24.1-global-ratelimit-core-and-route-table` (DONE 2026-05-23; squash-merge `a4fdc75`) + `24.2-global-ratelimit-perroute-and-headers` (this PLAN). 24.1 IMPL squash landed 61 files / +11041 / -25 LoC — well above the gate but distributed across IMPL (≈2.7k LoC production code in `internal/filter/http/ratelimit/`) + tests + fixtures + fake gRPC service + ADR/BEHAVIOR_CONTRACT/PROGRESS/REVIEW docs. This 24.2 PLAN-time re-evaluation per `superpowers:writing-plans` GATE + SKILL_ROUTING state-2 GATE + ADR-0045 §6 + `BOOTSTRAP_PROMPT.md` §6.1 confirms **single sub-phase landing** for 24.2 (no further nested split):

- **Task count: 9** (Pre-Task 0 + Tasks 1–8) — comfortably under the ADR-0045 25-task split-gate (mirrors the 24.1 PLAN's 12-task structure, smaller because 24.2 EXTENDS an existing package rather than scaffolding a new one).
- **24.2 IMPL LoC envelope (re-estimated against the post-24.1 baseline):**

| Surface | Anticipated LoC (added; production + tests) |
|---|---|
| `ratelimit/descriptors.go` (5 remaining actions + AMEND-11 defaults + `metadata` accessor + §4.4 stage bucketing + §4.3 Axis-B composition table + legacy force-include; Task 1+2+4) | ~150–300 added |
| `ratelimit/encode.go` (NEW; X-RateLimit injection on all dispositions; Task 5) | ~80–140 |
| `ratelimit/headers.go` (NEW; DRAFT_VERSION_03 byte construction; Task 5) | ~80–120 |
| `ratelimit/compiled_perroute.go` (NEW; `RateLimitPerRoute` TPFC compile; Task 3) | ~80–160 |
| `ratelimit/compiled_config.go` (parse-time stage bucketing + TPFC compiler registration plumbing; Task 2 + Task 3) | ~30–60 added |
| `ratelimit/decode_headers.go` (Axis-A perroute consumption + per-route `domain` override; Task 3 + Task 4) | ~30–60 added |
| `ratelimit/dispositions.go` (replace the 24.1 X-RateLimit STUB; store descriptor-statuses; encoder-hook delegation; Task 5) | ~20–50 added |
| `internal/filter/hcm/` (TPFC compiler registration for `RateLimitPerRoute`; Task 3) | ~10–30 added |
| `ratelimit/*_test.go` ADDED (TestDescriptors_PerAction for the 5 new + AMEND-11 keys; TestDescriptors_StageFilter; TestDescriptors_AxisB_Composition; TestPerRoute_*; TestEncodeHeaders_*; TestHeaders_DRAFT_VERSION_03_ByteShape; existing files extended) | ~400–700 added |
| `test/fixtures/0032-http-ratelimit/` (scenarios f + g + d-extension added; no new dir) | ~250–450 added |
| `test/helpers/ratelimitgrpc/` (fake-service script extension for per-descriptor statuses on scenario g) | ~30–80 added |
| `ratelimit/fuzz_test.go` corpus extension (no new fuzzer; existing `FuzzRateLimitConfigParse` gets new seeds) | ~30–80 added |
| `docs/envoy-go/DECISIONS.md` — ADR-0199 §Decision + §Consequences FULL (Task 3) + ADR-0197 IN-PLACE §Decision amendment (X-RateLimit + remaining-actions slice; Task 5) + ADR-0125 §(xv) AMENDMENT 9 → 10 (anchored in ADR-0199; Task 3) | docs ~200–350 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` completion bundle (X-RateLimit response-header allow-list + per-route canonical-patterns 9→10 + descriptor-engine completion to all 10 actions; Task 8) | docs ~150–250 |
| `docs/envoy-go/{ROADMAP,STATE}.md` + `docs/envoy-go/phases/24.2-.../{PROGRESS,REVIEW}.md` | docs ~250–450 |
| **GRAND TOTAL** | **~1810–3030 LoC (production + tests + fixtures + docs)** |

- **Production code alone (added): ~480–820 LoC** — well under the ADR-0045 ~1500-LoC trigger. The total inflates because tests + docs + BEHAVIOR_CONTRACT + ADRs land atomically with the IMPL per ADR-0052 / ADR-0044 (the 24.1 IMPL squash demonstrated the same ratio: ~11k LoC squash incl. all the verbatim PROGRESS outputs + REVIEW + BEHAVIOR_CONTRACT + ADR bodies, despite ~2.7k LoC of actual production code).
- **24.2 ships as a single sub-phase row.** No further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward — matches the phase-18.2 + 19.2 + 22.2/22.3 sibling sub-phase PLAN precedent). The 24.2 phase-done squash-merge **CLOSES row 24.2** (`planned → done`) AND **simultaneously closes the parent row 24** (`in-progress → done`) — the rollup discipline per the 18/19/22 precedent (the commit-message body names both transitions for grep-verifiability).

---

## ADRs introduced/landed by this plan

The 4 phase-24 §Context drafts (ADR-0197..0200) are anchored at the parent-SPEC commit per ADR-0044. 24.1 landed the §Decision + §Consequences bodies for ADR-0197[core] + ADR-0198 + ADR-0200. 24.2 lands the REMAINING ADR work at the materializing Tasks:

| ADR | Subject | §Decision + §Consequences body lands at |
|---|---|---|
| **ADR-0199 (FULL)** | `RateLimitPerRoute` NEW 10th canonical per-route shape ("data-only-with-vh-inclusion-enum") + ADR-0125 §(xv) roster amendment 9 → 10 — `vh_rate_limits` (OVERRIDE/INCLUDE/IGNORE) drives cross-tier descriptor composition; `override_option` PARSE-ACCEPTED-but-IGNORED (INERT `[#not-implemented-hide:]` per AMEND-4); route-additional `rate_limits[]` Axis-A early-return; per-route `domain` override. RE-AMENDS after phase-23's REUSE-by-absence skip; ENDS the 2-row deferral streak (phase-23 + phase-24.1). | **Task 3** (`compiled_perroute.go` + ADR-0125 amendment) |
| **ADR-0197 IN-PLACE §Decision amendment** | X-RateLimit DRAFT_VERSION_03 headers (`x-ratelimit-limit`/`x-ratelimit-remaining`/`x-ratelimit-reset`; MIN-status across multi-descriptor responses + `;w=`/`;name=` quota-policy suffix + unit→seconds map; emitted on ALL dispositions when enabled per AMEND-8) + descriptor-engine completion to the FULL 10 actions (the remaining 5 from 24.1's `actionUnsupportedAt241` stub). The in-place edit extends ADR-0197 §Decision per ADR-0052 amendment discipline (not a new ADR). | **Task 5** (`encode.go` + `headers.go`) |

**ADR-0202 escape-valve reserve (UNCONSUMED at 24.1 phase-done — D-hypothesis HELD; carries forward to 24.2 IMPL).** The parent SPEC §12 item-1 highest-risk byte-confirmation (the DELTA-2 chain-seed type + accessor return-type) RESOLVED at 24.1 Task 5 as RAW-PROTO SEED CONFIRMED; the escape valve did NOT fire. Phase 24's remaining D-hypothesis firing surfaces lie in 24.2:

- **Task 1 — `metadata`-action dynamic-metadata accessor surface (parent §12 item 2).** The `streamInfo().dynamicMetadata()` (DYNAMIC=0) vs route-metadata (ROUTE_ENTRY=1) accessor chain. PLAN hypothesis: the existing stream-info `DynamicMetadata()` accessor is sufficient (no Lua-bridge `dynmd` extension needed at 24.2 — the `metadata` action's read-only dynamic-metadata access matches the phase-22.2 lua-bridge READ path's exposure). If the existing accessor cannot satisfy the `metadata` action's segmented `metadata_key` lookup (the `Metadata::metadataValue` chain over the `MetadataKey.path` segments), fire ADR-0202 (§Context + §Decision + §Consequences at the Task-1 commit per ADR-0044).
- **Task 5 — X-RateLimit MIN-status quota-policy byte-edge (parent §12 item 5).** The exact `, <rpu>;w=<sec>[;name="<n>"]` concatenation + the MIN `limit_remaining` selection across multi-descriptor responses + the unit→seconds mapping (parent §4.7 + AMEND-8). PLAN hypothesis: the upstream `ratelimit_headers.cc:13-65` byte format is reproducible without divergence; the `headers_test.go` byte-pin against captured upstream headers settles the edge cases. If the byte format diverges (e.g., quoting discipline on `name=` with embedded characters; or `MIN_status` selection tie-breakers on equal `limit_remaining`), fire ADR-0202.

PLAN hypothesis: **ADR-0202 stays UNCONSUMED at phase-24 phase-done — HOLD-with-known-risk**. If fired, all the body lands at the firing Task's commit per ADR-0044. Next-free ADR after 24.2 phase-done advances `ADR-0202` (unconsumed) → `ADR-0203` if-and-only-if fired, else stays at ADR-0202.

---

## Planner-time deferred-decision resolution (the PLAN author settles these; recorded in PROGRESS preamble at Pre-Task 0)

Reproduced in PROGRESS at Pre-Task 0 so the IMPL subagents inherit them:

- **D-RL8 (`metadata` action's value-extraction accessor; parent §12 item 2 + AMEND-11).** **RECOMMENDED:** use the existing `streamInfo().DynamicMetadata()` accessor (already exposed on `DecoderFilterCallbacks` for the phase-22.2 lua-bridge READ path) for `MetadataSource_DYNAMIC=0`; use the matched route's `RouteEntry.Metadata()` accessor for `MetadataSource_ROUTE_ENTRY=1`. The segmented `MetadataKey.path` chain (each `key` is one segment) descends via `proto.Message → google.protobuf.Struct → google.protobuf.Value` per the upstream `Metadata::metadataValue` reference at `source/common/config/metadata.cc`. If the existing stream-info accessor does NOT expose a `Metadata` accessor for ROUTE_ENTRY (Task 1's first action confirms), the 24.1 DELTA-2 set-once-by-dispatch pattern (ADR-0165 / ADR-0198) extends — add a `RouteMetadata()` `DecoderFilterCallbacks` accessor (seeded at HCM dispatch alongside the `RouteRateLimits()` plumbing). If neither path fits cleanly, fire ADR-0202 (the §12-item-2 surface — escape-valve target #1).
- **D-RL9 (X-RateLimit DRAFT_VERSION_03 byte format; parent §12 item 5 + AMEND-8).** **RECOMMENDED:** reproduce the upstream `ratelimit_headers.cc:13-65` byte format verbatim: `x-ratelimit-limit: <MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` (MIN selection by `limit_remaining`; quota-policy suffix per descriptor with non-zero window; comma-separated descriptor segments; `;name=` value quoted per upstream); `x-ratelimit-remaining: <MIN.limit_remaining>`; `x-ratelimit-reset: <MIN.duration_until_reset.seconds>`. Unit→seconds: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000, UNKNOWN/0 ⇒ no quota-policy segment for that descriptor. Byte-pinned at Task 5 `headers_test.go` against captured upstream headers (cross-side scenario (g) at Task 6 provides the final verification). MIN-selection tie-breakers (equal `limit_remaining`): preserve insertion order (= descriptor-list order = action-list order per AMEND-6) — the FIRST equal-minimum status wins. If the byte format diverges (e.g., quoting of `name=` with embedded special characters; or rounding rules on fractional `duration_until_reset.seconds`), fire ADR-0202 (the §12-item-5 surface — escape-valve target #2).
- **D-RL10 (`RateLimitPerRoute` TPFC compiler registration path).** **RECOMMENDED:** register the `RateLimitPerRoute` TPFC compiler in `internal/filter/hcm/` alongside the existing per-route TPFC registrations (mirror the `header_mutation`/`oauth2`/`lua`/`cors` per-route compilation precedents — each filter registers a typed unmarshal for its per-route shape against the TPFC dispatch keyed by TypeURL per ADR-0073). The compiler validates the per-route message (`RateLimitPerRoute.override_option` accepted-but-IGNORED per AMEND-4; `vh_rate_limits` enum bounds; `rate_limits[]` recursively validated via the EXISTING `ratelimit.ValidateRouteRateLimits` exported validator from 24.1 Task 3) and produces a `*compiledPerRoute` opaque type. The filter consumes it via `f.dcb.RequestRouteConfig()` per the standard TPFC resolver path. The TypeURL: `"type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"` (byte-stable per ADR-0143 SN1).
- **D-RL11 (Axis-A vs Axis-B precedence at request time).** Reaffirms parent §4.3 AMEND-4 + AMEND-5: per-request walk is **Axis-A first (per-route `RateLimitPerRoute.rate_limits[]` non-empty ⇒ early-return, walk ONLY that list)**; otherwise **Axis-B (route policy + the `vh_rate_limits`-conditional vhost policy)**. `RateLimitPerRoute.override_option` is PARSE-ACCEPTED-but-INERT per AMEND-4 (NEVER read at request-time); the upstream-parity decision is locked. The legacy `RouteAction.include_vh_rate_limits=true` (parent §4.3) forces INCLUDE regardless of the `vh_rate_limits` enum. The per-route `domain` (`RateLimitPerRoute.domain`) overrides the filter-config `domain` when set; absent ⇒ filter-config domain.
- **D-RL12 (X-RateLimit emission scope per disposition).** Parent §4.7 + AMEND-8: X-RateLimit headers (when `enable_x_ratelimit_headers == DRAFT_VERSION_03`) emit on **ALL dispositions** (OK + OVER_LIMIT + error). The error/fail-closed reply path uses a **nullptr mutate-callback** per AMEND-8 — the 24.2 implementation honors this by NOT emitting X-RateLimit on the fail-closed `SendLocalReply` (the response is constructed without the encoder hook participating); a 24.1 `dispositions.go` STUB pin (no X-RateLimit on fail-closed) is retained. X-RateLimit DOES emit on the fail-OPEN path (continue-downstream ⇒ encoder hook runs ⇒ X-RateLimit injected).
- **D-RL13 (scenario (f) `vh_inclusion` dispatch).** Cross-side byte-exact via `CompareBytes` per the existing `0032` fixture's runner branch. The cross-side scenario exercises INCLUDE / OVERRIDE / IGNORE with both a route policy AND a vhost policy in play — the descriptors emitted to the fake `RateLimitService` differ per the §4.3 table; both sides MUST emit the same set; the fake returns OK uniformly so the cross-side response is also byte-exact.
- **D-RL14 (scenario (g) `x_ratelimit_headers` dispatch).** Cross-side byte-exact via `CompareBytes` per the existing `0032` runner branch. The fake `RateLimitService` returns per-descriptor statuses with `current_limit`/`limit_remaining`/`duration_until_reset` populated; both sides MUST emit byte-exact `x-ratelimit-*` headers per the AMEND-8 format. Per the X-RateLimit response-header allow-list extension at Task 8 (BEHAVIOR_CONTRACT §7.2), the headers are in the documented set-equal discipline (NO ignore-list — these are byte-exact).
- **D-RL15 (scenario (d) `descriptor_actions` extension).** Cross-side byte-exact (same dispatch as 24.1's `descriptor_actions` row). The extension covers all 10 actions: 24.1 covered `generic_key`/`request_headers`/`remote_address`/`destination_cluster`/`header_value_match`; 24.2 ADDS `source_cluster`/`masked_remote_address`/`metadata`/`query_parameters`/`query_parameter_value_match`. The fake asserts descriptor-set equality across the two sides (per the `0032` driver's existing cross-side discipline + the 24.1 (d-core) precedent).
- **D-RL16 (fuzzer corpus extension — no new fuzzer).** The existing `FuzzRateLimitConfigParse` gets ~10-20 NEW seeds: each of the 5 remaining action arms; `RateLimitPerRoute` with each `vh_rate_limits` value + with `override_option` set (PARSE-ACCEPTED-but-IGNORED arm exercise); `stage` boundary arms (0, 5, 10 — already arm `>10`); the legacy `include_vh_rate_limits=true` arm. Project fuzzer count stays at **33** (no new fuzzer).
- **D-RL17 (BEHAVIOR_CONTRACT bundle completion at atomic-landing).** Per ADR-0052 atomic-landing discipline + parent §13 bundle:
  - (1) the `### envoy.filters.http.ratelimit` subsection EXTENDS (engine completion to all 10 actions + the `metadata` accessor disposition + the stage multi-bucket discipline + the Axis-B `vh_rate_limits` decision table + the X-RateLimit DRAFT_VERSION_03 emission discipline + the per-route `RateLimitPerRoute.domain` override + `RateLimitPerRoute.override_option` accepted-but-INERT departure note);
  - (2) the `## Stat-name mapping` 4-counter section gets a per-route `domain`-qualifier paragraph (per AMEND-1: when a per-route `domain` is set, the stat names are UNCHANGED — `domain` is a descriptor-tier override, not a stat namespace; 110 → 114 stays);
  - (3) the per-route canonical-patterns cross-reference caption updates "through phase 24.1" → "through phase 24" and the §(xv) AMENDMENT 9 → 10 paragraph documents the `RateLimitPerRoute` 10th canonical;
  - (4) the response-header allow-list paragraph adds `x-ratelimit-limit` + `x-ratelimit-remaining` + `x-ratelimit-reset` (set-equal byte-exact per scenario (g)). No new departure records (the 3 from 24.1 already cover the only envoy-go-strict departures at the 24.2 surface; `override_option` accepted-but-INERT is upstream-parity, NOT a departure).
- **D-RL18 (atomic-landing ROADMAP rollup).** Per the 18/19/22 sub-phase rollup precedent: the 24.2 phase-done commit flips BOTH row 24.2 (`planned → done`) AND parent row 24 (`in-progress → done`) in ONE commit, and the commit-message body names both transitions for grep-verifiability ("phase 24.2: ... [ADR-0199, ADR-0197-amend, ADR-0125 §(xv)] — also closes parent row 24 [ROLLUP per 18/19/22 precedent]"). STATE.md re-advances per BOOTSTRAP §4.1 to whatever follows phase 24 in §9 (after this phase-done, the §9 HTTP-filters family closes to **1 remaining row: `wasm`** — STATE points at the next family member OR "awaiting next planning" if no §9 row is next-due).
- **D-P1 (task numbering).** Pre-Task 0 (PROGRESS preamble + preconditions) is the ritual prefix; the functional tasks are Tasks 1–8. Each Task maps 1:1 to a PROGRESS.md entry (D-P3).
- **D-P2 (subagent dispatch).** Per `superpowers:subagent-driven-development`, each Task is dispatched to a fresh `general-purpose` subagent with the Task's dispatch outline + a two-stage review between Tasks.
- **D-P3 (PROGRESS discipline).** Each Task appends a PROGRESS.md entry quoting the six-gate-relevant command outputs verbatim + the commit SHA.

---

## Task graph (sequential vs parallelizable per D-P2)

- **Pre-Task 0** (PROGRESS preamble + 12-precondition verification) — sequential prerequisite for everything.
- **Task 1** (`descriptors.go` remaining 5 actions + AMEND-11 key defaults + `metadata` accessor; ADR-0202 escape-valve target #1) — depends on Pre-Task 0. The FIRST functional Task; replaces the 24.1 `actionUnsupportedAt241` stub.
- **Task 2** (`stage` multi-stage bucketing path; `compiled_config.go` parse-time bucketing + `descriptors.go` walker filter) — depends on Task 1 (file overlap on `descriptors.go`).
- **Task 3** (`compiled_perroute.go` + ADR-0199 FULL §Decision + §Consequences + ADR-0125 §(xv) AMENDMENT 9 → 10) — depends on Pre-Task 0 ONLY (file-disjoint from Tasks 1+2 — new file `compiled_perroute.go` + DECISIONS.md edits). **PARALLELIZABLE with Tasks 1 + 2** in principle but the IMPL pipeline runs sequential per D-P2 (subagent two-stage review between Tasks; parallel dispatch is harder to review).
- **Task 4** (Axis-B `vh_rate_limits` cross-tier composition table + legacy force-include; `descriptors.go` walker + `decode_headers.go` Axis-A consumption) — depends on Tasks 1 + 3 (needs the engine extension + the `compiled_perroute.go` per-route shape).
- **Task 5** (`encode.go` + `headers.go` + ADR-0197 IN-PLACE §Decision amendment for X-RateLimit + remaining-actions slice; ADR-0202 escape-valve target #2; `dispositions.go` STUB replacement) — depends on Tasks 1 + 4 (the X-RateLimit headers consume the response descriptor-statuses + need the engine completion for the descriptor source).
- **Task 6** (Differential fixture EXTENSIONS to `0032` — scenarios (f) + (g) + (d-extension); the fake-service script extension for per-descriptor statuses) — depends on Task 5 (X-RateLimit emission live).
- **Task 7** (Fuzzer corpus extension — no new fuzzer; existing `FuzzRateLimitConfigParse` gets ~10-20 new seeds covering the new actions + perroute + stage + legacy include arms) — depends on Tasks 1 + 3 (fuzz the extended config surface).
- **Task 8** (Atomic landing — BEHAVIOR_CONTRACT bundle completion + STATE re-advance + ROADMAP row 24.2 done + parent row 24 done [ROLLUP] + REVIEW.md) — depends on everything.

**Sequential bottlenecks:** Pre-Task 0 → Task 1 → Task 2 → Task 4 (the `descriptors.go` chain); Pre-Task 0 → Task 3 → Task 4 (the per-route chain); Tasks 1+4 → Task 5; Task 5 → Task 6; Tasks 1+3 → Task 7; Tasks 1-7 → Task 8.

---

## Execution preconditions

The IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`):

```bash
git worktree add /home/esa/git/envoy-go/.worktrees/phase-24.2-global-ratelimit-perroute-and-headers-impl \
                 -b phase-24.2-global-ratelimit-perroute-and-headers-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-24.2-global-ratelimit-perroute-and-headers-impl
```

where `<PLAN-tip-SHA>` is the master tip after THIS PLAN.md squash-merge + its SHA-fill follow-up.

The 12 preconditions verified at Pre-Task 0 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-24.2-global-ratelimit-perroute-and-headers-impl`.
2. **Master tail.** `git log --oneline master | head -6` shows THIS 24.2 PLAN.md squash + its SHA-fill follow-up at the head, with the 24.1 IMPL squash `a4fdc75` + the 24.1 SHA-fill `dbd2d3b` reachable in the recent history. If not, `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` ≥ `go1.26.2`; `golangci-lint version` = `1.64.8` (ADR-0009); `docker version` reports client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `201` (ADR-0201 at master tip post-24.1-IMPL). Higher → another phase landed; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0199' docs/envoy-go/DECISIONS.md` returns `1` (§Context anchored at the parent SPEC commit; §Decision + §Consequences land at Task 3). `grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md` returns `1` (24.1 [core] body landed; Task 5 IN-PLACE amends it). `grep -cE '^## ADR-0202' docs/envoy-go/DECISIONS.md` returns `0` (escape-valve UNCONSUMED post-24.1; stays UNCONSUMED at 24.2 phase-done unless Task 1 or Task 5 fires it).
6. **NO 24.1-bound code regression at this worktree.** Per BOOTSTRAP §4.1 invariant 2 — the 24.1 surface MUST stay intact. `test -d internal/filter/http/ratelimit && test -f internal/grpcclient/ratelimit_client.go && test -d test/helpers/ratelimitgrpc && test -d test/fixtures/0032-http-ratelimit && grep -q 'HTTPGlobalRateLimitGRPC' test/differential/fixture/fixture.go && echo "ok: phase-24.1 surface present"` returns success.
7. **Parent SPEC + 24.2 SPEC SHAs.** `git log -1 --format=%H -- docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` + `.../24.2-global-ratelimit-perroute-and-headers/SPEC.md` return the split commit (or descendant). If different, re-read.
8. **Pristine tree.** `git status --porcelain` returns empty.
9. **Pre-existing suite green.** `go test -count=1 -short ./...` clean (incl. the 24.1-landed `internal/filter/http/ratelimit/` package).
10. **Pre-existing differential baseline green.** All 35 fixture directories (0000-0033 inclusive) PASS (lua 0026-0029 + multi-listener combined runs may hit the documented `freeTCPPort` flake per 22.2 REVIEW §7.4 — re-run in isolation). 24.2 ADDS scenarios to existing `0032` (NO new fixture dir; stays 35).
11. **Fuzzer baseline.** `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `33`. 24.2 EXTENDS the existing `FuzzRateLimitConfigParse` corpus (NO new fuzzer; count stays 33).
12. **NEW 24.2 surfaces absent at IMPL cold-start.** `test ! -f internal/filter/http/ratelimit/encode.go && test ! -f internal/filter/http/ratelimit/headers.go && test ! -f internal/filter/http/ratelimit/compiled_perroute.go && ! grep -q 'actionSourceCluster\|actionMaskedRemoteAddress\|actionMetadata\|actionQueryParameters\|actionQueryParameterValueMatch' internal/filter/http/ratelimit/descriptors.go && ! grep -q 'scenario.*vh_inclusion\|scenario.*x_ratelimit_headers' test/fixtures/0032-http-ratelimit/README.md && echo "ok: phase-24.2-new-surfaces absent"` returns success.

If all 12 pass, proceed to Pre-Task 0 + Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 12-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md`

This pre-task verifies the `## Execution preconditions` block and creates PROGRESS.md so subsequent tasks have an append target. Records the 2-ADR-landing-map + the conditional ADR-0202 + the D-RL8..D-RL18 + D-P1..D-P3 resolutions verbatim.

**Precondition:** worktree exists at `phase-24.2-...-impl`; all 12 preconditions report green.
**Acceptance:** all 12 preconditions green; PROGRESS.md preamble committed.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` and confirm expected output.
- [ ] **Step 2: Author `PROGRESS.md` preamble** — (a) 12-precondition verification (verbatim outputs); (b) the 2-ADR landing table (ADR-0199 @ Task 3 / ADR-0197 in-place @ Task 5) + the conditional ADR-0202 escape-valve mapping (Task 1 `metadata`-accessor / Task 5 X-RateLimit byte-edge); (c) D-RL8..D-RL18 + D-P1..D-P3 reproduced verbatim; (d) a Pre-Task 0 entry slot for the commit-SHA.
- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Pre-Task 0: PROGRESS.md preamble + 12-precondition verification"
```

---

## Task 1: `descriptors.go` — the 5 remaining actions + AMEND-11 key defaults + `metadata` accessor

**Files:**
- Modify: `internal/filter/http/ratelimit/descriptors.go` (replace `actionUnsupportedAt241()` with the 5 remaining actions; ~150–250 LoC added)
- Modify: `internal/filter/http/ratelimit/descriptors_test.go` (new table rows for the 5 actions + AMEND-11 key default arms + `metadata` accessor arms; ~200–350 LoC added)
- Possible modify: `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` + `internal/filter/hcm/*.go` (IF D-RL8's RouteMetadata accessor extension fires — first action of this Task determines)
- Append: PROGRESS.md (Task 1 entry; record D-RL8 outcome + whether ADR-0202 fired)

Lands the 5 remaining canonical actions per parent §4.1: `source_cluster` (key literal `"source_cluster"`, value = filter's `local_service_cluster` node service-cluster name, always-true), `destination_cluster` already at 24.1 — NOT re-landed at this Task; the 5 NEW are `source_cluster` + `masked_remote_address` (key literal `"masked_remote_address"`, CIDR-masked remote IP via `v4_prefix_mask_len`/`v6_prefix_mask_len`, false if not an IP) + `metadata` (key from config `descriptor_key` REQUIRED, value from `MetadataKey.path` segment chain over `MetadataSource_DYNAMIC=0`→`streamInfo().DynamicMetadata()` / `MetadataSource_ROUTE_ENTRY=1`→route metadata, `default_value` fallback per AMEND-11, conditional drop on `skip_if_absent`) + `query_parameters` (key from config `descriptor_key` with **AMEND-11 default `"query_param"` SINGULAR**, value from first matching query-param via `query_param_name`, conditional drop) + `query_parameter_value_match` (key from config `descriptor_key` with AMEND-11 default `"query_match"`, value = `descriptor_value` (or `default_value`), false if `expect_match` [default true] ≠ query-match per `router_ratelimit.cc:297,304-328`). REMOVES the 24.1 `actionUnsupportedAt241()` stub. **D-RL8 byte-confirmation** (the `metadata` accessor surface): FIRST action of this Task surveys the existing `streamInfo().DynamicMetadata()` accessor against the `Metadata::metadataValue` segmented-key reference; if it satisfies both DYNAMIC + ROUTE_ENTRY paths, proceed; if ROUTE_ENTRY needs a NEW `RouteMetadata()` accessor (the DELTA-2 plumbing extension), add it (still no ADR-0202 if it's a clean ADR-0165 extension); if neither fits, fire ADR-0202.

**Precondition:** Pre-Task 0 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/ -run 'TestDescriptors'` clean (all 5 new actions + AMEND-11 keys + `metadata` accessor + conditional-drop arms PASS); the 24.1 `descriptor_actions` (d-core) cross-side fixture still GREEN (Gate D regression check on `0032/d-core`); D-RL8 outcome recorded in PROGRESS (RouteMetadata extension or NOT; ADR-0202 fired or NOT).

**Subagent dispatch outline** (D-P2 `general-purpose`):
> FIRST ACTION (D-RL8 byte-confirmation): survey the existing `streamInfo().DynamicMetadata()` accessor on `DecoderFilterCallbacks` (per phase-22.2 lua-bridge READ path) against the upstream `Metadata::metadataValue` reference (the segmented `MetadataKey.path` chain over `proto.Message → google.protobuf.Struct → google.protobuf.Value`). If it satisfies DYNAMIC + ROUTE_ENTRY paths, proceed. If ROUTE_ENTRY needs a NEW `RouteMetadata() proto.Message` `DecoderFilterCallbacks` accessor — add it via the ADR-0165 set-once-by-dispatch pattern (mirrors the 24.1 DELTA-2 `RouteRateLimits()` plumbing — chain field + setter + accessor + the matched-route's metadata seeded at HCM dispatch). If neither fits cleanly, STOP, escalate, fire ADR-0202 (§Context + §Decision + §Consequences at this Task's commit). Then: replace `actionUnsupportedAt241()` with the 5 NEW action functions (each per parent §4.1 line-cited semantics + AMEND-11 key defaults); extend the action-dispatch switch; remove the stub. Record the D-RL8 outcome verbatim in PROGRESS.

- [ ] **Step 1: Confirm D-RL8 accessor surface.** Survey existing `streamInfo().DynamicMetadata()` + route-metadata accessor; record the disposition (no extension needed / RouteMetadata extension added / ADR-0202 fired) in PROGRESS.
- [ ] **Step 2: Write the failing tests** in `descriptors_test.go`:
  - `TestDescriptors_PerAction_SourceCluster` (key = `"source_cluster"`; value = filter's `local_service_cluster` from `FactoryCtx.Node.GetCluster()`; always-true).
  - `TestDescriptors_PerAction_MaskedRemoteAddress` (v4 + v6 cases; CIDR masking with configured `v4_prefix_mask_len`/`v6_prefix_mask_len`; key = `"masked_remote_address"`; false if not an IP).
  - `TestDescriptors_PerAction_Metadata_Dynamic` (DYNAMIC=0; value via `streamInfo().DynamicMetadata()` segmented chain; `default_value` fallback; `skip_if_absent` conditional drop).
  - `TestDescriptors_PerAction_Metadata_RouteEntry` (ROUTE_ENTRY=1; value via route-metadata accessor; same fallback + drop semantics).
  - `TestDescriptors_PerAction_QueryParameters` (key default = `"query_param"` SINGULAR per AMEND-11; value = first matching query-param; param-absent ⇒ `skip_if_absent ? skip : drop`).
  - `TestDescriptors_PerAction_QueryParameterValueMatch` (key default = `"query_match"`; value = `descriptor_value` or `default_value`; `expect_match` true [default] ≠ match ⇒ drop).
  - `TestDescriptors_AMEND11_KeyDefaults_ByteStable` (table-driven; asserts every default-key string byte-exact per ADR-0080: `generic_key`→`"generic_key"`, `header_value_match`→`"header_match"`, `query_parameters`→`"query_param"` SINGULAR, `query_parameter_value_match`→`"query_match"`).
  - `TestDescriptors_EmptyActionDrop_Extended` (the §4.5 TWO behaviors extended for the new actions: action returns false ⇒ whole descriptor dropped + loop breaks; empty-key entry skipped, descriptor survives).
- [ ] **Step 3: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestDescriptors' -v`. Expected: FAIL (the 5 new symbols undefined; the 24.1 `actionUnsupportedAt241` stub still in place).
- [ ] **Step 4: Implement the 5 actions in `descriptors.go`** — remove `actionUnsupportedAt241`; add `actionSourceCluster`, `actionMaskedRemoteAddress`, `actionMetadata`, `actionQueryParameters`, `actionQueryParameterValueMatch`; extend the dispatch switch in `applyAction` to route the new arms; preserve the §4.5 empty-action-drop discipline; AMEND-11 key defaults as package-level consts asserted by `TestDescriptors_AMEND11_KeyDefaults_ByteStable`.
- [ ] **Step 5: Run tests to verify they pass + regression sweep** — `go test -race ./internal/filter/http/ratelimit/`. Expected: PASS. Run the 0032/d-core regression: `go test -count=1 ./test/differential/ -run 'TestDifferential/0032-http-ratelimit'` still GREEN (24.1 scenarios stay green).
- [ ] **Step 6: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/descriptors.go internal/filter/http/ratelimit/descriptors_test.go docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 1: descriptors.go — 5 remaining actions (source_cluster/masked_remote_address/metadata/query_parameters/query_parameter_value_match) + AMEND-11 key defaults + metadata accessor"
```

---

## Task 2: `stage` multi-stage bucketing path

**Files:**
- Modify: `internal/filter/http/ratelimit/compiled_config.go` (parse-time stage bucketing 0-10; ~20–40 LoC added)
- Modify: `internal/filter/http/ratelimit/descriptors.go` (per-request stage selection in the walker; ~30–50 LoC added)
- Modify: `internal/filter/http/ratelimit/descriptors_test.go` + possibly `compiled_config_test.go` (`TestDescriptors_StageFilter`; ~80–140 LoC added)
- Append: PROGRESS.md (Task 2 entry)

Lands the §4.4 stage multi-stage bucketing path. At parse time, the route + vhost `rate_limits[]` slices are bucketed by `stage` (0-10) into `compiledPolicies[stage]` slots (size 11, the upstream `MAX_STAGE_NUMBER+1=11` invariant). At request time, only policies whose `stage` equals the filter's configured `stage` (the existing `compiledConfig.stage` field, parsed at 24.1 with the `stage > 10` PARSE-REJECT) are evaluated. 24.1 evaluated only the default stage-0 bucket; 24.2 generalizes — a filter configured at `stage=5` walks only `compiledPolicies[5]`. **Depends on Task 1** (`descriptors.go` file overlap — sequential to avoid merge conflicts; the walker filter sits adjacent to the action-dispatch switch).

**Precondition:** Task 1 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/ -run 'TestDescriptors_StageFilter|TestBuildCompiledConfig_Stage'` clean (all rows PASS — stage=0 default evaluated; stage=5 picks only 5-bucket policies; stage>10 still PARSE-REJECTs via 24.1's arm); 24.1 fixtures still GREEN (Gate D regression — 0032 + 0033 PASS).

**Subagent dispatch outline:**
> Author the §4.4 stage path. PARSE-TIME (in `buildCompiledConfig` or the route-table parse path — confirm location at IMPL via the actual 24.1 file shape): bucket each compiled route + vhost policy into `[11]compiledStageBucket` slots indexed by `policy.GetStage().GetValue()` (UInt32Value default 0; the 24.1 PARSE-REJECT `stage > 10` arm in `ValidateRouteRateLimits` already covers the upper bound — re-confirm). REQUEST-TIME (in `buildDescriptors` or its caller): select the bucket at `compiledConfig.stage` and walk ONLY those policies. The TWO bucket sources (route + vhost) compose per Task 4's Axis-B table (this Task's scope is the bucket structure + the per-request selection; Axis-B walks both buckets when applicable). The 24.1 default-stage-0 path remains the baseline behavior.

- [ ] **Step 1: Write the failing test** — `TestDescriptors_StageFilter_DefaultStageZero` (filter stage=0; route has stage-0 + stage-3 policies; only stage-0 walked); `TestDescriptors_StageFilter_NonZeroStage` (filter stage=5; route has stage-5 + stage-3 policies; only stage-5 walked); `TestDescriptors_StageFilter_AllBucketsEmpty` (no stage-N policy at filter stage; zero descriptors ⇒ continue); `TestBuildCompiledConfig_StageBucketing_ParseTime` (a config with policies at stages 0/3/5/10 compiles into 11 buckets with the right occupancy).
- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/filter/http/ratelimit/ -run 'TestDescriptors_StageFilter|TestBuildCompiledConfig_StageBucketing' -v`. Expected: FAIL (24.1 evaluates all policies regardless of stage; the bucket structure does not exist).
- [ ] **Step 3: Implement parse-time bucketing + request-time selection** per the dispatch outline.
- [ ] **Step 4: Run tests to verify they pass + regression sweep** — `go test -race ./internal/filter/http/ratelimit/` PASS; `go test -count=1 ./test/differential/ -run 'TestDifferential/003[23]'` GREEN.
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/compiled_config.go internal/filter/http/ratelimit/descriptors.go internal/filter/http/ratelimit/descriptors_test.go internal/filter/http/ratelimit/compiled_config_test.go docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 2: stage multi-stage bucketing path (§4.4) — parse-time [11]bucket + per-request filter-stage selection"
```

---

## Task 3: `compiled_perroute.go` — `RateLimitPerRoute` 10th-canonical TPFC compile + ADR-0199 + ADR-0125 §(xv) AMENDMENT 9 → 10

**Files:**
- Create: `internal/filter/http/ratelimit/compiled_perroute.go` (~80–160 LoC; `RateLimitPerRoute` TPFC compile)
- Create: `internal/filter/http/ratelimit/compiled_perroute_test.go` (~200–350 LoC; table-driven coverage)
- Modify: `internal/filter/hcm/` (TPFC compiler registration for the `RateLimitPerRoute` TypeURL; ~10–30 LoC added)
- Modify: `internal/filter/http/ratelimit/ratelimit.go` (export the per-route compile entry point if needed; ~5–15 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0199 §Decision + §Consequences FULL body + ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph (per ADR-0044 in-place edit discipline)
- Append: PROGRESS.md (Task 3 entry)

Lands `RateLimitPerRoute` the NEW 10th canonical per-route shape per parent §5.3 + ADR-0199. The TPFC compiler validates the per-route message and produces a `*compiledPerRoute` opaque type holding: `vhRateLimits` (the enum, OVERRIDE/INCLUDE/IGNORE — driving Axis-B at Task 4); `rateLimits` (the Axis-A embedded `[]*routev3.RateLimit` — early-return wins per AMEND-4); `domain` (the per-route domain override, empty ⇒ use filter-config domain). `override_option` is PARSE-ACCEPTED (no error) but the value is DISCARDED — NEVER read at request time (AMEND-4 INERT discipline; logged via a doc-comment + a unit-test row that constructs a config with each enum value + asserts the compiled output ignores it). The TPFC compiler reuses the EXISTING `ratelimit.ValidateRouteRateLimits` from 24.1 Task 3 to recursively validate the embedded `rate_limits[]` slice (per §5.2 — the same `disable_key`/`extension`/`dynamic_metadata` PARSE-REJECT arms apply to the per-route-embedded list). **ADR-0199 §Decision + §Consequences FULL body lands at this Task's commit + the ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph (anchored in ADR-0199 — the canonical-per-route roster grows). Depends on Pre-Task 0 ONLY** (file-disjoint from Tasks 1+2; PARALLELIZABLE in principle but D-P2 runs sequential).

**Precondition:** Pre-Task 0 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/... ./internal/filter/hcm/...` clean; `TestPerRoute_VhRateLimits_Honored` + `TestPerRoute_OverrideOption_AcceptedButIgnored` + `TestPerRoute_Domain_Override` + `TestPerRoute_RateLimits_AxisA_Compile` all PASS; ADR-0199 §Decision + §Consequences (non-placeholder) present (`grep -A2 '^### Decision' docs/envoy-go/DECISIONS.md` under ADR-0199 is non-placeholder); ADR-0125 §(xv) AMENDMENT paragraph anchored (`grep -c 'RateLimitPerRoute' docs/envoy-go/DECISIONS.md` under ADR-0125 returns non-zero).

**Subagent dispatch outline:**
> Author `compiled_perroute.go` per parent §6.7 + ADR-0199 §Context. `RateLimitPerRoute` has 4 fields per AMEND-4: `vh_rate_limits` (enum; honored at request-time Task 4), `override_option` (PARSE-ACCEPTED, INERT, discarded), `rate_limits[]` (Axis-A early-return list — recursively validated via the EXISTING `ratelimit.ValidateRouteRateLimits`), `domain` (per-route domain override). TypeURL: `"type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"` (byte-stable per ADR-0143 SN1). Register the TPFC compiler in `internal/filter/hcm/` alongside the other per-route filter registrations (mirror `header_mutation`/`oauth2`/`lua` patterns — confirm exact registration path at IMPL). Per ADR-0085 nil-tolerance. Then write ADR-0199 §Decision + §Consequences FULL body in DECISIONS.md replacing the placeholder + add the ADR-0125 §(xv) AMENDMENT paragraph anchored in ADR-0199 (the canonical-per-route roster grows 9 → 10 with `RateLimitPerRoute` as the "data-only-with-vh-inclusion-enum" 10th — phase-23 SKIPPED via REUSE-by-absence; phase-24.1 SKIPPED via roster-defer; phase-24.2 RE-AMENDS at this Task per the 22.1→22.3 anticipation→landing precedent).

- [ ] **Step 1: Write the failing tests** in `compiled_perroute_test.go`:
  - `TestPerRoute_TypeURL_ByteStable` (the TypeURL const byte-exact per ADR-0143 SN1).
  - `TestPerRoute_VhRateLimits_Honored` (3 rows: OVERRIDE=0[default], INCLUDE=1, IGNORE=2 — the compiled output preserves the enum verbatim for Task 4 consumption).
  - `TestPerRoute_OverrideOption_AcceptedButIgnored` (4 rows: DEFAULT, OVERRIDE_POLICY, INCLUDE_POLICY, IGNORE_POLICY — each value PARSE-ACCEPTS; the compiled output exposes NO override-option field; AMEND-4 INERT contract).
  - `TestPerRoute_Domain_Override` (empty + non-empty `domain` cases; per-route domain wins over filter-config domain when set).
  - `TestPerRoute_RateLimits_AxisA_Compile` (embedded `rate_limits[]` slice compiles into the per-route opaque; `ValidateRouteRateLimits` reuse — embedded slice with `disable_key` PARSE-REJECTs with the same wording as the route-table arm at 24.1 Task 3).
  - `TestPerRoute_TPFC_Registration` (the HCM TPFC dispatch resolves the `RateLimitPerRoute` TypeURL to the compiler; per-route config in a TPFC entry parses end-to-end).
- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestPerRoute' -v`. Expected: FAIL (symbols undefined; compiler not registered).
- [ ] **Step 3: Author `compiled_perroute.go`** — `compiledPerRoute` struct + `BuildCompiledPerRoute(*anypb.Any) (*compiledPerRoute, error)` + the TPFC TypeURL const + reuse of `ValidateRouteRateLimits` for the embedded slice; register the TPFC compiler in `internal/filter/hcm/` at the existing per-route registration site (verify the precise pattern against `header_mutation`/`oauth2`/`lua` per-route registration at IMPL).
- [ ] **Step 4: Run tests to verify they pass** — Expected: PASS.
- [ ] **Step 5: Land ADR-0199 §Decision + §Consequences FULL body** in DECISIONS.md replacing the placeholder; ANCHOR the ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph in ADR-0199 §Consequences (the 10th canonical "data-only-with-vh-inclusion-enum" + the BEHAVIOR_CONTRACT per-route cross-reference landing at Task 8). Per ADR-0044 in-place edit discipline.
- [ ] **Step 6: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/... ./internal/filter/hcm/...
git add internal/filter/http/ratelimit/compiled_perroute.go internal/filter/http/ratelimit/compiled_perroute_test.go internal/filter/http/ratelimit/ratelimit.go internal/filter/hcm/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 3: compiled_perroute.go — RateLimitPerRoute 10th canonical TPFC compile [ADR-0199 FULL, ADR-0125 §(xv) 9->10]"
```

---

## Task 4: Axis-B `vh_rate_limits` cross-tier composition table + legacy force-include

**Files:**
- Modify: `internal/filter/http/ratelimit/descriptors.go` (the §4.3 walk: route policy always walked; vhost policy conditional per `vh_rate_limits` enum + the legacy `include_vh_rate_limits` force-include; ~50–100 LoC added)
- Modify: `internal/filter/http/ratelimit/decode_headers.go` (consume `RateLimitPerRoute` via `f.dcb.RequestRouteConfig()`; Axis-A early-return; per-route `domain` override; ~30–60 LoC added)
- Modify: `internal/filter/http/ratelimit/descriptors_test.go` + new tests (the §4.3 4-row decision table + the legacy force-include arm; ~150–250 LoC added)
- Append: PROGRESS.md (Task 4 entry)

Lands the §4.3 Axis-B cross-tier composition table + the legacy `RouteAction.include_vh_rate_limits=true` force-include arm. 24.1 covered ONLY the OVERRIDE-default (route policy walked + vhost walked when route empty). 24.2 generalizes to the FULL table:

| `vh_rate_limits` | route has `rate_limits` | VH `rate_limits` walked? | route `rate_limits` walked? |
|---|---|---|---|
| `OVERRIDE` (0, default) | yes (non-empty) | NO | yes |
| `OVERRIDE` (0, default) | no (empty) | YES | yes (empty no-op) |
| `INCLUDE` (1) | any | YES (both tiers) | yes |
| `IGNORE` (2) | any | NO | yes |

Plus the legacy override: `RouteAction.include_vh_rate_limits=true` forces `INCLUDE` regardless of the enum (per parent §4.3 + AMEND-5). At request time, the walker reads the per-route `compiledPerRoute` (Task 3) via `f.dcb.RequestRouteConfig()` to fetch `vh_rate_limits` + the Axis-A embedded `rate_limits[]` + the `domain` override. **Axis-A precedence (D-RL11):** if `compiledPerRoute.rateLimits != nil && len > 0`, walk ONLY that list (early-return; the route-table + vhost-table policies + the `vh_rate_limits` enum are bypassed entirely per AMEND-4). The per-route `domain` (if non-empty) overrides the filter-config domain. **Depends on Tasks 1 + 3** (needs the engine extension + the `compiled_perroute` shape).

**Precondition:** Tasks 1 + 3 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/ -run 'TestDescriptors_AxisB|TestDescriptors_AxisA_EarlyReturn_PerRoute|TestDecodeHeaders_PerRoute_Domain'` clean (all rows PASS); the 24.1 fixtures `0032/b/c/d-core/e/h` + `0033` still GREEN.

**Subagent dispatch outline:**
> Implement the §4.3 walker. In `decode_headers.go`: resolve `RateLimitPerRoute` via `f.dcb.RequestRouteConfig()` (per ADR-0073 TPFC resolver path); if non-nil + `rateLimits` non-empty ⇒ Axis-A early-return walk ONLY that list (the per-route domain override applies; the route-table + vhost-table are skipped). Else ⇒ Axis-B walk: route policy walked unconditionally; vhost policy conditional per the §4.3 table (read `RateLimitPerRoute.vhRateLimits` if non-nil; default OVERRIDE if nil — i.e., the standard upstream default applies absent any per-route override; check the legacy `RouteAction.GetIncludeVhRateLimits()` for the force-include arm — accessible via the 24.1-landed `VirtualHostRateLimits()` plumbing or a sibling RouteAction accessor pinned at IMPL). The per-request domain wins per D-RL11. Honor the §4.4 stage selection (Task 2) within each tier.

- [ ] **Step 1: Write the failing tests** — `TestDescriptors_AxisB_OverrideDefault_RouteHasRateLimits` (route non-empty + OVERRIDE ⇒ vhost SKIPPED — was 24.1 default; re-confirms regression intact); `TestDescriptors_AxisB_OverrideDefault_RouteEmpty` (route empty + OVERRIDE ⇒ vhost WALKED); `TestDescriptors_AxisB_Include` (route + vhost BOTH walked); `TestDescriptors_AxisB_Ignore` (vhost NEVER walked); `TestDescriptors_AxisB_LegacyForceInclude` (`RouteAction.include_vh_rate_limits=true` ⇒ vhost walked regardless of enum); `TestDescriptors_AxisA_EarlyReturn_PerRoute` (per-route `rate_limits[]` non-empty ⇒ walk only that, route-table + vhost-table skipped); `TestDecodeHeaders_PerRoute_Domain` (per-route `domain` overrides filter-config `domain` in the `RateLimitRequest`).
- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestDescriptors_AxisB|TestDescriptors_AxisA_EarlyReturn_PerRoute|TestDecodeHeaders_PerRoute_Domain' -v`. Expected: FAIL (24.1 only covered OVERRIDE-default + no Axis-A; new arms missing).
- [ ] **Step 3: Implement the walker** in `descriptors.go` (the Axis-B table + legacy force-include) + `decode_headers.go` (the TPFC-resolve + Axis-A early-return + per-route `domain` override). Use the EXISTING 24.1 `VirtualHostRateLimits()` accessor + introduce a sibling `VirtualHostIncludeVhRateLimits()` accessor IF the legacy field is not already exposed (verify against the 24.1 callbacks surface; the legacy field is `RouteAction.include_vh_rate_limits` — it's a RouteAction property, may already be accessible).
- [ ] **Step 4: Run tests to verify they pass + regression sweep** — `go test -race ./internal/filter/http/ratelimit/` PASS; `go test -count=1 ./test/differential/ -run 'TestDifferential/003[23]'` GREEN (24.1 fixtures unchanged).
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/descriptors.go internal/filter/http/ratelimit/decode_headers.go internal/filter/http/ratelimit/descriptors_test.go internal/filter/http/ratelimit/decode_headers_test.go docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 4: Axis-B vh_rate_limits cross-tier composition table + legacy include_vh_rate_limits force-include (§4.3 / AMEND-5)"
```

---

## Task 5: `encode.go` + `headers.go` — X-RateLimit DRAFT_VERSION_03 emission + ADR-0197 IN-PLACE §Decision amendment

**Files:**
- Create: `internal/filter/http/ratelimit/encode.go` (~80–140 LoC; per-stream encode hook; reads stored response descriptor-statuses; applies the `x-ratelimit-*` triple)
- Create: `internal/filter/http/ratelimit/headers.go` (~80–120 LoC; DRAFT_VERSION_03 byte construction — MIN-status across multi-descriptor responses + `;w=`/`;name=` quota-policy suffix + unit→seconds map)
- Create: `internal/filter/http/ratelimit/encode_test.go` (~150–250 LoC)
- Create: `internal/filter/http/ratelimit/headers_test.go` (~150–250 LoC; byte-pin against captured upstream headers per parent §12 item 5; ADR-0202 escape-valve target #2)
- Modify: `internal/filter/http/ratelimit/dispositions.go` (replace the 24.1 X-RateLimit STUB with the real store-and-defer-to-encoder path; ~20–50 LoC added)
- Modify: `internal/filter/http/ratelimit/dispositions_test.go` (add tests for the X-RateLimit injection on OK + OVER_LIMIT + fail-open; assert NO injection on fail-closed per D-RL12; ~100–180 LoC added)
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0197 IN-PLACE §Decision amendment (the X-RateLimit + remaining-actions slice; per ADR-0052 in-place edit discipline)
- Append: PROGRESS.md (Task 5 entry — RECORD the D-RL9 byte-confirmation outcome + whether ADR-0202 fired)

Lands the X-RateLimit DRAFT_VERSION_03 response-header injection. Per parent §4.7 + AMEND-8 + D-RL12: when `enable_x_ratelimit_headers == DRAFT_VERSION_03`, the filter emits `x-ratelimit-limit/-remaining/-reset` on ALL dispositions where the encoder hook runs (OK + OVER_LIMIT 429 + fail-OPEN continue). The fail-CLOSED 500 path uses nullptr-mutate (no encoder hook participation) ⇒ NO X-RateLimit headers (the 24.1 dispositions byte-shape stays intact). The headers are driven by the MIN `limit_remaining` descriptor-status across multi-descriptor responses (D-RL9). The `x-ratelimit-limit` value has the format `<MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` (comma-separated quota-policy segments per descriptor with non-zero window). Unit→seconds: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000, UNKNOWN/0 ⇒ no quota-policy segment for that descriptor. **D-RL9 byte-confirmation** (the X-RateLimit byte format): finalized at `headers_test.go` against captured upstream headers (a side-by-side capture from reference Envoy v1.37.2 against the existing fake-RLS scenario); ADR-0202 escape-valve target #2 (fires only if the byte format diverges — e.g., quoting discipline on `name=` with embedded chars; rounding rules on fractional `duration_until_reset.seconds`; tie-breakers on equal `limit_remaining` MIN). **ADR-0197 IN-PLACE §Decision amendment** lands at this Task's commit per ADR-0052 (the X-RateLimit + remaining-actions slice is added to ADR-0197 §Decision; the 24.1-landed CORE slice stays unchanged). **Depends on Tasks 1 + 4** (the 24.1 dispositions stub for X-RateLimit becomes live; the descriptor-engine completion + Axis-B walks must be in place to source the descriptor-statuses).

**Precondition:** Tasks 1 + 4 complete.
**Acceptance:** `go build ./...` + `go vet` + `golangci-lint run` clean; `go test -race -count=1 ./internal/filter/http/ratelimit/ -run 'TestEncodeHeaders|TestHeaders|TestDispositions_XRateLimit'` clean (all rows PASS); `TestHeaders_DRAFT_VERSION_03_ByteShape` byte-exact against the captured upstream headers (D-RL9 outcome recorded — byte-match confirmed or ADR-0202 fired); the 24.1 dispositions tests still PASS (no regression to OK/OVER_LIMIT/error byte-shape); ADR-0197 §Decision amendment present (`grep -c 'X-RateLimit\|DRAFT_VERSION_03' docs/envoy-go/DECISIONS.md` under ADR-0197 increases vs the 24.1 baseline).

**Subagent dispatch outline:**
> FIRST ACTION (D-RL9 byte-pin capture): set up a side-by-side capture: configure both envoy-go (with `enable_x_ratelimit_headers: DRAFT_VERSION_03`) and reference Envoy v1.37.2 against the SAME shared fake `RateLimitService` (the 24.1-landed `test/helpers/ratelimitgrpc/`), drive a request whose descriptors trigger a non-zero `limit_remaining` + `duration_until_reset`, capture the upstream `x-ratelimit-*` response headers verbatim. Use those captured bytes as the `wantBytes` in `TestHeaders_DRAFT_VERSION_03_ByteShape`. If the byte format diverges at IMPL (e.g., `;name=` quoting / fractional rounding / MIN tie-breakers), STOP, escalate, fire ADR-0202 (§Context + §Decision + §Consequences at this Task's commit per ADR-0044). Then author `headers.go` (the byte-construction primitive — MIN selection, quota-policy suffix, unit→seconds) + `encode.go` (the per-stream encode hook reading the stored response descriptor-statuses; applies the triple via the encoder callbacks `EncodeHeaders` hook). Modify `dispositions.go` to STORE the response descriptor-statuses on the per-stream filter struct (replace the 24.1 STUB note `// 24.2: X-RateLimit DRAFT_VERSION_03 injection (parent §6.6)` with the live store); the encoder hook reads them and applies the headers via the `EncoderFilterCallbacks` mutate path. NOT applied on fail-CLOSED 500 per D-RL12 (the encoder hook does not participate on the nullptr-mutate path). Then write the ADR-0197 IN-PLACE §Decision amendment (the X-RateLimit + remaining-actions slice paragraph; per ADR-0052).

- [ ] **Step 1: Capture upstream X-RateLimit bytes** for the D-RL9 byte-pin (record the captured bytes verbatim in PROGRESS + as the `wantBytes` in `headers_test.go`).
- [ ] **Step 2: Write the failing tests** in `headers_test.go` + `encode_test.go` + `dispositions_test.go` extensions:
  - `TestHeaders_DRAFT_VERSION_03_ByteShape` (byte-exact assertions for `x-ratelimit-limit`/`-remaining`/`-reset` against the D-RL9 captured bytes — covers MIN-status across multi-descriptor; `;w=`/`;name=` quota suffix; unit→seconds for SECOND/MINUTE/HOUR/DAY/WEEK/MONTH/YEAR; UNKNOWN/0 ⇒ no segment).
  - `TestHeaders_MIN_Selection_TieBreaker` (equal `limit_remaining` ⇒ first equal-minimum status wins per insertion order).
  - `TestEncodeHeaders_OK_AppliesXRateLimit` (DRAFT_VERSION_03 + OK disposition ⇒ headers injected at encode time).
  - `TestEncodeHeaders_OverLimit_AppliesXRateLimit` (DRAFT_VERSION_03 + OVER_LIMIT 429 ⇒ headers injected in AMEND-8 order: RLS `response_headers_to_add` → `x-envoy-ratelimited: true` (unless suppressed) → X-RateLimit triple → filter-config `response_headers_to_add`).
  - `TestEncodeHeaders_FailOpen_AppliesXRateLimit` (error + fail-open default ⇒ headers injected; the OK continue path runs through the encoder).
  - `TestEncodeHeaders_FailClosed_NoXRateLimit` (`failure_mode_deny=true` ⇒ 500 + nullptr-mutate ⇒ NO X-RateLimit per D-RL12 — confirms the 24.1 byte-shape).
  - `TestEncodeHeaders_OFF_NoEmission` (`enable_x_ratelimit_headers: OFF` ⇒ NO X-RateLimit on any disposition).
  - `TestDispositions_XRateLimit_Stored_OnAllDispositions` (the per-stream state holds the response descriptor-statuses after dispatch; encoder hook reads them).
- [ ] **Step 3: Run tests to verify they fail** — `go test ./internal/filter/http/ratelimit/ -run 'TestEncodeHeaders|TestHeaders|TestDispositions_XRateLimit' -v`. Expected: FAIL (`encode.go`/`headers.go` files absent; dispositions STUB still in place).
- [ ] **Step 4: Implement `headers.go`** — the byte-construction primitive (`buildXRateLimitHeaders(statuses []*RateLimitResponse_DescriptorStatus) (limit, remaining, reset string)`); the MIN selection + quota-policy suffix + unit→seconds map per D-RL9.
- [ ] **Step 5: Implement `encode.go`** — the per-stream `EncodeHeaders` hook reading the stored statuses + applying the triple via the `EncoderFilterCallbacks` mutate path.
- [ ] **Step 6: Modify `dispositions.go`** — replace the 24.1 X-RateLimit STUB note with the live store of the response descriptor-statuses on the per-stream filter struct (the `f.responseStatuses` field; the encoder hook reads it). NOT applied on fail-CLOSED per D-RL12.
- [ ] **Step 7: Run tests to verify they pass + regression sweep** — `go test -race ./internal/filter/http/ratelimit/` PASS; `go test -count=1 ./test/differential/ -run 'TestDifferential/003[23]'` GREEN.
- [ ] **Step 8: Land ADR-0197 IN-PLACE §Decision amendment** in DECISIONS.md — extend ADR-0197 §Decision with the X-RateLimit + remaining-actions slice paragraph (per ADR-0052 in-place edit discipline; the 24.1 CORE slice stays unchanged; the amendment paragraph cross-references this Task's commit + Task 1's commit for the remaining-actions slice).
- [ ] **Step 9: Verify gates + append PROGRESS + commit** (record D-RL9 outcome + whether ADR-0202 fired)

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/encode.go internal/filter/http/ratelimit/headers.go internal/filter/http/ratelimit/encode_test.go internal/filter/http/ratelimit/headers_test.go internal/filter/http/ratelimit/dispositions.go internal/filter/http/ratelimit/dispositions_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 5: encode.go + headers.go — X-RateLimit DRAFT_VERSION_03 emission on all dispositions [ADR-0197 IN-PLACE §Decision amendment]"
```

---

## Task 6: Differential fixture EXTENSIONS to `0032-http-ratelimit/` — scenarios (f) + (g) + (d-extension)

**Files:**
- Modify: `test/fixtures/0032-http-ratelimit/{README.md,envoy.yaml,envoy-go.yaml,expectations.yaml,inputs/driver.go}` (add scenarios f + g + extend d; ~250–450 LoC added; NO new fixture dir)
- Modify: `test/helpers/ratelimitgrpc/` (extend the fake-service script to emit per-descriptor statuses with `current_limit`/`limit_remaining`/`duration_until_reset` for scenario g; ~30–80 LoC added)
- Append: PROGRESS.md (Task 6 entry)

Lands the differential fixture extensions per parent §7.1 + the 24.2 SPEC §3. NO new fixture dir (the count stays 35). Three additions:

- **Scenario (f) `vh_inclusion`** — cross-side byte-exact via `CompareBytes` per D-RL13. The config carries both a route policy AND a vhost policy with distinct descriptors; the `vh_rate_limits` enum is exercised across 3 sub-scenarios (OVERRIDE / INCLUDE / IGNORE) — the descriptors emitted to the fake `RateLimitService` differ per the §4.3 table; both sides MUST emit the same set; the fake returns OK uniformly so the cross-side response is also byte-exact.
- **Scenario (g) `x_ratelimit_headers`** — cross-side byte-exact via `CompareBytes` per D-RL14. The config sets `enable_x_ratelimit_headers: DRAFT_VERSION_03`; the fake returns per-descriptor statuses with `current_limit`/`limit_remaining`/`duration_until_reset` populated (driving multi-descriptor MIN-status selection); both sides MUST emit byte-exact `x-ratelimit-*` headers per the AMEND-8 format. The fake script extension here exercises the per-descriptor status path the 24.1 fake stubbed (24.1 returned `overall_code` only; 24.2 adds the per-descriptor `statuses[]` slice with the 3-field status messages).
- **Scenario (d) `descriptor_actions` EXTENSION** — cross-side byte-exact per D-RL15. The 24.1 (d-core) scenario covered 5 actions (`generic_key`/`request_headers`/`remote_address`/`destination_cluster`/`header_value_match`); 24.2 EXTENDS the existing scenario with the 5 remaining (`source_cluster`/`masked_remote_address`/`metadata`/`query_parameters`/`query_parameter_value_match`) so the cross-side fake sees the FULL 10-action descriptor set. The fake asserts descriptor-set equality across the two sides.

**Depends on Task 5** (X-RateLimit emission live for scenario g; the remaining 5 actions from Task 1 + the Axis-B walks from Task 4 for scenarios f + d-extension).

**Precondition:** Tasks 1 + 3 + 4 + 5 complete.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'TestDifferential/0032-http-ratelimit'` GREEN (all 8 scenarios PASS: 24.1 a/b/c/d-core/e/h + 24.2 f/g/d-extension); cross-side byte-exact on (b)/(c)/(d-FULL-10)/(e)/(f)/(g); the fake-service extension does NOT regress the 24.1 scenarios (a/b/c/d-core/e/h still GREEN).

**Subagent dispatch outline:**
> Extend `test/fixtures/0032-http-ratelimit/` with 3 new scenarios + extend the existing (d) scenario. The driver pattern is the 24.1 precedent (allocate-port → render-both-YAMLs → `ratelimitgrpc.NewAtAddr` → per-scenario `Script` → drive probes → `Stop`). Per D-RL13/14/15 all 3 additions are cross-side byte-exact via the existing `CompareBytes` runner branch — NO new asserter type needed. The fake-service script extension at `test/helpers/ratelimitgrpc/` adds per-descriptor `statuses[]` emission for scenario g (proto-number-faithful per AMEND-6; the 3-field `DescriptorStatus{code, current_limit, limit_remaining, duration_until_reset}` is emitted by NUMBER, optionals omitted when unset). Per `reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch; cross-side scenarios share the cross-side branch; the existing 24.1 (h) `stat_surface` REMAINS on the `StatsAsserter` branch per `reference_differential_asserter_dispatch` (it was proven live at 24.1; no change to its dispatch).

- [ ] **Step 1: Extend the fake-service script** at `test/helpers/ratelimitgrpc/` to emit per-descriptor `statuses[]` (proto-number-faithful per AMEND-6; the 3-field DescriptorStatus emitted by NUMBER with optionals omitted when unset).
- [ ] **Step 2: Author scenarios (f) + (g) + extend (d)** in the fixture — both YAMLs + expectations + driver entries. Scenario (f) covers 3 sub-cases (OVERRIDE / INCLUDE / IGNORE); scenario (g) covers DRAFT_VERSION_03 + at-least-2-descriptors (to exercise the MIN-status selection); (d) adds the 5 remaining actions.
- [ ] **Step 3: Run the fixture** — `go test -count=1 ./test/differential/ -run 'TestDifferential/0032-http-ratelimit' -v`. Expected: GREEN (all 8 scenarios).
- [ ] **Step 4: Verify gates + append PROGRESS + commit**

```bash
go test -count=1 ./test/differential/ -run 'TestDifferential/0032-http-ratelimit' -v
git add test/fixtures/0032-http-ratelimit/ test/helpers/ratelimitgrpc/ docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 6: 0032-http-ratelimit fixture extensions — scenarios (f) vh_inclusion + (g) x_ratelimit_headers + (d) extended to all 10 actions"
```

---

## Task 7: Fuzzer corpus extension (no new fuzzer; project count stays 33)

**Files:**
- Modify: `internal/filter/http/ratelimit/fuzz_test.go` (no change to the fuzz function itself; the corpus extension is `testdata/`-driven; ~5–20 LoC for any new fuzz wrapper)
- Modify/Create: `internal/filter/http/ratelimit/testdata/fuzz/FuzzRateLimitConfigParse/` (~10–20 new seeds; ~30–80 LoC corpus added)
- Append: PROGRESS.md (Task 7 entry)

Extends the EXISTING `FuzzRateLimitConfigParse` corpus per D-RL16. ~10-20 new seeds covering: each of the 5 remaining action arms; `RateLimitPerRoute` with each `vh_rate_limits` value + with `override_option` set (PARSE-ACCEPTED-but-IGNORED arm exercise); `stage` boundary arms (0, 5, 10); the legacy `RouteAction.include_vh_rate_limits=true` arm; per-route `domain` override; an X-RateLimit OFF→DRAFT_VERSION_03 arm. Must-never-panic across the extended `buildCompiledConfig` + the FULL 10-action descriptor-engine compile + the new `compiled_perroute.go` compile. **Project fuzzer count stays at 33** (NO new fuzzer; this is corpus extension only). **Depends on Tasks 1 + 3** (the engine extension + the per-route compile must be in place for the seeds to exercise non-stub paths).

**Precondition:** Tasks 1 + 3 complete.
**Acceptance:** `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ -v` PASS (seed corpus clean — every new seed passes the must-never-panic invariant); 30s live-fuzz clean: `go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/` no panic, no crasher; project fuzzer count `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `33` (unchanged).

**Subagent dispatch outline:**
> Per ADR-0018 fuzzer-corpus discipline + D-RL16: add ~10-20 new seed files under `internal/filter/http/ratelimit/testdata/fuzz/FuzzRateLimitConfigParse/`. Each seed is a `go test fuzz v1` formatted file containing a `[]byte` with a serialized `RateLimit` filter-config Any (or a malformed bytestring to exercise the PARSE path). New seeds: (a) each of the 5 new action arms; (b) `RateLimitPerRoute` shapes — each `vh_rate_limits` value + each `override_option` value (PARSE-ACCEPTED-INERT); (c) stage 0/5/10 boundary; (d) legacy include_vh_rate_limits=true; (e) per-route `domain` non-empty; (f) `enable_x_ratelimit_headers` DRAFT_VERSION_03 + OFF. Must-never-panic across the FULL extended engine compile.

- [ ] **Step 1: Author the new seeds** under `testdata/fuzz/FuzzRateLimitConfigParse/` (one file per seed; `go test fuzz v1` format).
- [ ] **Step 2: Run the seed corpus** — `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ -v`. Expected: PASS (every seed; no panic).
- [ ] **Step 3: Run live fuzz 30s** — `go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/`. Expected: no panic, clean exit.
- [ ] **Step 4: Verify fuzzer count unchanged** — `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `33`.
- [ ] **Step 5: Verify gates + append PROGRESS + commit**

```bash
go build ./... && go vet ./... && golangci-lint run ./internal/filter/http/ratelimit/...
git add internal/filter/http/ratelimit/testdata/ internal/filter/http/ratelimit/fuzz_test.go docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PROGRESS.md
git commit -m "phase 24.2 Task 7: FuzzRateLimitConfigParse corpus extension (no new fuzzer; +10-20 seeds for remaining actions + perroute + stage + legacy include + xratelimit)"
```

---

## Task 8: Atomic landing — BEHAVIOR_CONTRACT completion + STATE + ROADMAP rows 24.2 + 24 [ROLLUP] + REVIEW

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 24.2 completion bundle per parent §13 — D-RL17; ~150–250 LoC added; atomic per ADR-0052)
- Modify: `docs/envoy-go/ROADMAP.md` (row `24.2` `planned → done`; per-cell IMPL-done annotation; **parent row `24 in-progress → done` [ROLLUP per 18/19/22 precedent]**)
- Modify: `docs/envoy-go/STATE.md` (re-advance per BOOTSTRAP §4.1 — active-phase to whatever follows 24 in §9 / "awaiting next planning" if none; lifecycle-state 6 → 0/1 depending; record D-hypothesis disposition for ADR-0202)
- Create: `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/REVIEW.md`
- Append: PROGRESS.md (Task 8 entry + the six-gate verbatim outputs)

The atomic-landing task per the phase-09..23 + 24.1 precedent. The ADR §Decision/§Consequences bodies ALREADY landed at their functional Tasks (ADR-0199 @ Task 3, ADR-0197 IN-PLACE amendment @ Task 5) + the ADR-0125 §(xv) AMENDMENT 9 → 10 landed at Task 3 — this Task does NOT re-land them; it lands the doc-state + the BEHAVIOR_CONTRACT completion bundle + REVIEW + the simultaneous ROADMAP row 24.2 + parent row 24 flip per D-RL18. **Depends on Tasks 1–7.**

**Precondition:** Tasks 1–7 complete; all six gates GREEN at HEAD.
**Acceptance:** all six gates GREEN (verbatim in PROGRESS + REVIEW); BEHAVIOR_CONTRACT completion bundle landed; **ROADMAP row 24.2 `done` AND parent row 24 `done` [ROLLUP] in the same commit**; STATE re-advanced; REVIEW.md authored; the FULL parent §15 acceptance UNION verified (24.1 partial items 7/9/15/16 closed + new items 12/14 + the doc-state UNION at item 17 + the end-to-end audit-trail at item 18).

**Subagent dispatch outline:**
> Run the six-gate verification (A build / B vet+lint / C race / D differential 35/35 incl. extended 0032 / E fuzz 33 with extended corpus / F h2spec 53/53) capturing verbatim outputs. Land the 24.2 BEHAVIOR_CONTRACT completion bundle per D-RL17: (1) the `### envoy.filters.http.ratelimit` subsection EXTENDS (engine completion to all 10 actions + the `metadata` accessor disposition + the §4.4 stage multi-bucket discipline + the §4.3 Axis-B `vh_rate_limits` decision table + the X-RateLimit DRAFT_VERSION_03 emission discipline + the per-route `RateLimitPerRoute.domain` override + the `RateLimitPerRoute.override_option` accepted-but-INERT departure note); (2) the per-route canonical-patterns cross-reference caption updates "through phase 24.1" → "through phase 24" + the §(xv) AMENDMENT 9 → 10 paragraph documents the `RateLimitPerRoute` 10th canonical (cross-references ADR-0199 + ADR-0125); (3) the response-header allow-list paragraph adds `x-ratelimit-limit` + `x-ratelimit-remaining` + `x-ratelimit-reset` (set-equal byte-exact per scenario g); (4) the stat-name mapping section adds a per-route `domain`-qualifier paragraph (per AMEND-1: `domain` is a descriptor-tier override, NOT a stat namespace — 110 → 114 stays; per-route `domain` does NOT extend the qualifier surface). NO new departure records (the 3 from 24.1 cover the only envoy-go-strict departures; `override_option` accepted-but-INERT is upstream-parity). Flip ROADMAP row 24.2 → done AND parent row 24 → done **in the same atomic commit** per D-RL18; the commit-message body names both transitions for grep-verifiability. Re-advance STATE for whatever follows 24 (the §9 family closes to 1 remaining row: `wasm`; STATE either points at the next family member or "awaiting next planning"). Author REVIEW.md per `superpowers:requesting-code-review` (the full parent §15 acceptance UNION confirmed; six-gate evidence; the 24.1 partial items closed; the D-hypothesis disposition recorded — ADR-0202 stays UNCONSUMED at 24.2 phase-done per the PLAN hypothesis, UNLESS Task 1 or Task 5 fired it). Run the plan-document-reviewer-equivalent confirmation that all SPEC §6 gates + the 24.2 §7 acceptance + the full parent §15 UNION are GREEN.

- [ ] **Step 1: Run the six gates** — capture verbatim:

```bash
go build ./...                                              # Gate A
go vet ./... && golangci-lint run                           # Gate B
go test -race -count=1 ./...                                # Gate C
go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'  # Gate D — 35/35 (0000-0033 incl. extended 0032)
go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/  # Gate E — seed corpus
go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/  # Gate E — 30s live
go test -v -count=1 ./test/conformance/h2spec/              # Gate F — 53/53
```

- [ ] **Step 2: Land the BEHAVIOR_CONTRACT.md completion bundle** per D-RL17, atomic per ADR-0052 — extends the existing 24.1 subsection in-place (no new top-level subsection — the bundle COMPLETES the parent §13 bundle started at 24.1 Task 12).
- [ ] **Step 3: Flip ROADMAP rows 24.2 + 24** in the same commit per D-RL18 — row 24.2 `planned → done` (per-cell IMPL-done annotation; the 8-task IMPL trail + 2 ADR landings + 6-gate verbatim summary); row 24 `in-progress → done` (the rollup discipline — the commit-message body names both transitions); the parent row's `sub-phases` column stays `24.1, 24.2` (unchanged).
- [ ] **Step 4: Re-advance STATE.md** to lifecycle-state 6 → next-phase / awaiting-next-planning per BOOTSTRAP §4.1. The §9 family closes from 2 remaining rows (post-24.1 = `wasm` + the rollup-pending parent 24) to **1 remaining row** (`wasm`) at this commit. Next-skill: `superpowers:brainstorming` (if a follow-on §9 row is next-due) or "awaiting next planning" (per the discretion of the next session — write whichever fits the post-24 landscape). Record the D-hypothesis disposition: ADR-0202 stays UNCONSUMED at 24.2 phase-done (D-hypothesis HELD again), UNLESS Task 1's `metadata`-accessor or Task 5's X-RateLimit byte-edge fired it. Next-free ADR stays at `ADR-0202` (unless fired ⇒ `ADR-0203`).
- [ ] **Step 5: Author REVIEW.md** per `superpowers:requesting-code-review` — verify the FULL parent §15 acceptance UNION: 24.1 partial items 7 (full 0032 a-h + d-extension + 0033) / 9 (all 10 actions + empty-action-drop) / 15 (ADR-0199 + ADR-0197 in-place amendment + ADR-0125 9→10 landed) / 16 (BEHAVIOR_CONTRACT completion bundle landed); + new items 12 (X-RateLimit DRAFT_VERSION_03 headers per AMEND-8) / 14 (`RateLimitPerRoute` 10th canonical + ADR-0125 9→10); + item 17 (doc-state UNION; STATE re-advanced; ROADMAP rows 24.2 + 24 both `done`; 19 HTTP filters still wired; §9 family → 1 remaining row `wasm`); + item 18 (end-to-end audit-trail SPEC → 24.1 + 24.2 [SPEC → PLAN → PROGRESS → REVIEW] chains). Quote the six-gate verbatim outputs.
- [ ] **Step 6: Append PROGRESS + commit** (the phase-done squash-merge to master happens after REVIEW approval per the phase-09..23 stage-close pattern):

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/
git commit -m "phase 24.2 Task 8: BEHAVIOR_CONTRACT completion bundle + ROADMAP rows 24.2 done + 24 done [ROLLUP] + STATE re-advance + REVIEW.md"
```

---

## Audit-trail summary

SPEC (24.2 slice + parent master) → this PLAN (8 functional tasks + Pre-Task 0 = 9 total) → PROGRESS (1:1 per task) → REVIEW. The 2 24.2-landing ADR touches land at their functional Tasks: ADR-0199 §Decision + §Consequences FULL + ADR-0125 §(xv) AMENDMENT 9 → 10 @ Task 3; ADR-0197 IN-PLACE §Decision amendment (X-RateLimit + remaining-actions slice) @ Task 5. The conditional ADR-0202 escape-valve is wired to TWO surfaces — the `metadata`-accessor at Task 1 (D-RL8) + the X-RateLimit byte-edge at Task 5 (D-RL9). The BEHAVIOR_CONTRACT.md completion bundle + the ROADMAP rows 24.2 + 24 simultaneous flip (D-RL18 rollup) + STATE re-advance + REVIEW land atomically at Task 8. The full parent §15 acceptance UNION (closing the 24.1 partial items + new items 12 + 14 + 17 + 18) is verified at Task 8's REVIEW. The 24.1 IMPL surface is INTACT throughout (precondition 6 verified at Pre-Task 0; no regression to the 24.1-landed `internal/filter/http/ratelimit/` 8-prod + 6-test files; the descriptor engine EXTENDS the 24.1 surface without re-landing it). Project fuzzer count stays at 33 (Task 7 is corpus extension only); fixture directory count stays at 35 (Task 6 extends `0032` in-place); HTTP filter count stays at 19; stat count stays at 114 (per-route `domain` is descriptor-tier, NOT a stat namespace per AMEND-1); BEHAVIOR_CONTRACT departure count stays at 18 (the 3 from 24.1 cover the only envoy-go-strict departures at the 24.2 surface; `override_option` accepted-but-INERT is upstream-parity). The phase-24 closure: row 24 `in-progress → done` at this Task 8 commit; §9 HTTP-filters family closes to **1 remaining row: `wasm`**; the canonical-per-route roster grows 9 → 10; the FIRST cross-namespace cluster-stat-charge pattern (landed at 24.1) + the FIRST PLAN-time ADR-0045 split application (landed at 24-SPEC ADR-0201) + the FIRST PLAN-time anchored §(xv) AMENDMENT landing (at 24.2 Task 3) all close cleanly.
