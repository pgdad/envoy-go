# Phase 13 Brainstorm — `envoy.filters.http.buffer`

**Status:** brainstorm complete + post-landing empirical amendment (§12 added 2026-05-09). This document captures the design decisions reached during the lifecycle-state-0 → 1 brainstorm session for phase 13 (`http-filter-buffer`), the SIXTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, and `csrf` at phase 12). The next session (lifecycle-state 1 → 2 for phase 13, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10/11/12 precedent) authors `docs/envoy-go/phases/13-http-filter-buffer/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004. **§12 below supersedes the affected portions of §§1.1, 2.5, 2.6, 2.7, 2.8, 6, 7, 8, and 11 in light of post-landing empirical findings; §§1–11 are preserved verbatim as the pre-amendment design sketch.**

**Brainstorm session:** worktree `.worktrees/phase-13-http-filter-buffer-brainstorm`, branch `phase-13-http-filter-buffer-brainstorm`, branched from master tip `a782fc9` (the phase 12 phase-done REVIEW commit `phase 12: REVIEW — end-of-phase retrospective + N-1 carry-forward`). The phase 12 phase-done implementation commit `4f4ed39` and its SHA-fill follow-up `2706168` precede the REVIEW; `a782fc9` is the REVIEW-landing commit.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + MVP scope envelope + each major design decision via 4-question dialogue (Q1 path selection — §9 family-child vs. Path B Runtime + hot restart infra phase; Q2 family-child selection from §9 list — `buffer` chosen from `compression / bandwidth_limit / rbac / other`; Q3 cap interaction with ADR-0076 — MVP requires `max_request_bytes ≤ 1 MiB` parse-time-validated; Q4 per-route discipline — `disabled` boolean AND `buffer.max_request_bytes` override BOTH supported under a new ADR codifying the 5th canonical per-route shape). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0124), and the just-shipped phase 12 + phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 + 12 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/12-http-filter-csrf/BRAINSTORM.md` section-for-section, reframed for the buffer scope and adapted for its specific surface area (the structurally-thinnest §9 row at the proto level — only ONE field on the parent `Buffer` proto; first body-touching HTTP filter; first per-route discipline introducing a `disabled` boolean shortcut as a first-class shape). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 13 — this brainstorm cold-started fresh from the §9 heading + the phase 12 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-08. Last-updated: 2026-05-09 (§12 amendment).

---

## 1. Mission and scope confirmation (13 only)

ROADMAP row `13 | http-filter-buffer | 12 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 13 is the SIXTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 12 phase-done commit `4f4ed39` (with follow-up `2706168` for SHA fill, REVIEW at `a782fc9`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 58: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114); `csrf` shipped in phase 12 (`internal/filter/http/csrf/` per ADR-0120). Phase 13 ships `buffer` as the SIXTH real filter — the canonical Envoy-style "buffer entire request body before forwarding upstream" filter — and establishes the per-filter-phase pattern's sixth data point. It is also the FIRST §9 family-row to interact with the framework's body-buffering machinery (ADR-0076) and the FIRST §9 family-row whose per-route proto introduces a top-level `disabled` boolean shortcut as a first-class shape.

### 1.1 What 13 delivers as a self-contained whole

Phase 13 lands `envoy.filters.http.buffer` (the canonical Envoy buffer filter) under the 07.1 framework. Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):

1. **New `internal/filter/http/buffer/` package** owning the filter implementation. Package directory + Go package identifier are both `buffer` (single token; no underscore needed since the proto type-name is already a single token). Files mirror the `internal/filter/http/csrf/` shape from phase 12: `buffer.go` (filter type + factory + decode methods + per-route helper + filterStats struct + compiledConfig), `buffer_test.go` (unit tests), `fuzz_test.go` (the 17th fuzzer in the repo — `FuzzBufferConfigParse`), `doc.go` (package overview + 1-consumed/0-deferred decomposition + per-route disabled-OR-override summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering `router.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains an eighth `httpReg.Register(buffer.TypeURL, buffer.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → buffer → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`. Buffer inserts between `router` and `cors` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing** of `envoy.extensions.filters.http.buffer.v3.Buffer`, the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has exactly **1 top-level field** — `max_request_bytes` (`UInt32Value`, REQUIRED at parse time). Phase 13 consumes the field with envoy-go-own validation discipline: the value MUST be non-nil, `value > 0`, AND `value ≤ 1048576` (1 MiB). The third constraint is an envoy-go-only validation NOT present in reference Envoy (which accepts arbitrary `UInt32Value` up to ~4 GiB) — its rationale is the framework-side cap codified at ADR-0076. The 1 MiB ceiling closes the divergence-window where a config with `max_request_bytes > 1 MiB` would be accepted by reference Envoy but trip the framework's 17-byte 413 path before the buffer filter's wire shape could fire. Per Q3 = "MVP requires max_request_bytes ≤ 1 MiB", this is the brainstormer-committed envelope shape; ADR-0126 codifies the validation + carries an explicit forward-pointer to the future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)).

4. **Per-route TPFC: `disabled` boolean OR `max_request_bytes` override (Q4 = "both shapes").** Per the proto message `BufferPerRoute` (the per-route-TPFC type, separate from the listener-level `Buffer` — UNLIKE phase 12's csrf where the same `CsrfPolicy` served both purposes), per-route entries carry a oneof with two cases: (a) `disabled: true` — the filter is wholly inactive on this route, no buffering, no counter increments; (b) `buffer: { max_request_bytes: <UInt32Value> }` — a wholesale override of the listener-level cap (subject to the same ≤ 1 MiB validation). Both shapes are honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (disabled OR override) drives the disposition. ADR-0125 codifies this as the **5th canonical per-route discipline** (after data-only-override per ADR-0073 used by cors/fault/header_mutation; multi-tier-evaluation via `ResolveAllTiers` per ADR-0110 used by header_mutation; stateful-override-with-independent-stats per ADR-0117 used by local_ratelimit; data-only-override-with-shared-stats per ADR-0124 used by csrf).

5. **Disposition tables + body-cap decision in `DecodeHeaders` + `DecodeData`.** The filter resolves the most-specific `compiledPerRoute` via 3-tier `PerRouteConfig.Resolve`, then computes the effective cap + per-stream disposition. Split into two tables for clarity (the `endStream` boolean on the headers-call is distinct from the `endStream` boolean on each data-call; the prior single-table form conflated the two columns).

    **Table A — `DecodeHeaders(headers, endStream)` disposition:**

    | Per-route resolve | `endStream` on headers | Content-Length known | CL > effectiveMax | `DecodeHeaders` disposition | Counter | Notes |
    |---|---|---|---|---|---|---|
    | `disabled=true` | (any) | (any) | (any) | `Continue`; set `f.passthrough=true` | (none) | route opted out; `DecodeData` short-circuits via the passthrough flag |
    | nil OR `override` | `endStream=true` (header-only request) | n/a | n/a | `Continue` | (none) | no body expected; `DecodeData` will not be called |
    | nil OR `override` | `endStream=false` | yes | yes | `SendLocalReply(413, "Payload Too Large", connClose)` + `StopIteration` | `request_too_large +1` | CL fast-fail; `DecodeData` will not be called |
    | nil OR `override` | `endStream=false` | yes | no | `Continue` | (none) | begin streaming-cap path; `DecodeData` accumulates |
    | nil OR `override` | `endStream=false` | no (Transfer-Encoding chunked OR header malformed) | n/a | `Continue` | (none) | begin streaming-cap path; `DecodeData` accumulates |

    **Table B — `DecodeData(data, endStream)` disposition** (entered only when `f.passthrough==false` AND `DecodeHeaders` returned `Continue` AND not header-only; otherwise `DecodeData` is either skipped or fast-passthrough):

    | `f.passthrough` | `accumulated += len(data)` then check | `endStream` on data | `DecodeData` disposition | Counter |
    |---|---|---|---|---|
    | `true` | (skipped) | (any) | `DataContinue` (passthrough; framework safety-net cap never engages because we never return `DataStopIterationAndBuffer`) | (none) |
    | `false` | `accumulated > effectiveMax` (mid-stream overflow) | (any) | `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` | `request_too_large +1` |
    | `false` | `accumulated ≤ effectiveMax` | `endStream=true` (terminal chunk fits) | `DataContinue` (release the fully-buffered body to upstream) | `request_buffered +1` |
    | `false` | `accumulated ≤ effectiveMax` | `endStream=false` (in-flight chunk; more to come) | `DataStopIterationAndBuffer` (accumulate; framework holds bytes per ADR-0076) | (none yet) |

    The disposition uses `DataStopIterationAndBuffer` for in-flight buffering (the framework's existing accumulation path); `DataContinue` to release the fully-buffered body to the upstream; `DataStopIterationNoBuffer` on overflow (discards the partial buffer; the framework's `beginLocalReply` drives the encode chain immediately). NO async-resume; NO encode-side state. `DecodeTrailers` / all `Encode*` are pass-through.

6. **413 wire shape — REUSE framework path (ADR-0076).** When the buffer filter trips the cap (either Content-Length fast-fail OR mid-stream overflow), it calls `SendLocalReply(413, "Payload Too Large", connClose)` with byte-equal arguments to the framework's existing decode-overflow path codified at ADR-0076 §Decision (b): status `413`, body `"Payload Too Large"` (17 bytes ASCII; constant `localReply413Body`; no trailing newline per §11 #3 empirical pin), 4-header lowercase wire-form (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`), plus user-supplied `Connection: close`. Hypothesis: byte-equivalent to reference Envoy's buffer-filter 413 (which also reuses Envoy's stock 413 path). SPEC §11 empirical pin confirms via scrape probe; if a divergence surfaces, ADR-0127 records the divergence + the SPEC author files an amendment.

7. **Stat surface — 29→31-name extension (Decision 7 → ADR-0128).** 2 new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping` (extending the phase-12 29-name table to 31 names): `http.<HCM stat_prefix>.buffer.request_buffered` (counter; increments once per request that fully buffers within `effectiveMax`) and `http.<HCM stat_prefix>.buffer.request_too_large` (counter; increments once per request that trips the cap, either via Content-Length fast-fail OR mid-stream overflow). Per-route stats are SHARED with listener-level (NOT independent per-route — buffer is data-only per-route per Decision 5; mirrors phase-12 csrf ADR-0124 SHARED-stats discipline; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats discipline). Empirical pin §11.P5 confirms exact counter names + Prometheus form against Envoy v1.37.2 scrape.

8. **No new framework primitive.** Phase 13 reuses (a) `SendLocalReply` from fault/local_ratelimit/csrf precedent; (b) `DataStopIterationAndBuffer` from the existing chain.go iteration discipline; (c) the 3-tier `PerRouteConfig.Resolve` from phase 07.1; (d) the framework's body-cap synthesis from ADR-0076 (as the safety-net cap above buffer's own ≤ 1 MiB enforcement); (e) the per-stream filterStats pattern from csrf phase-12. Phase 13 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor, NO new SN flattening rule.

**Plus three artifact-level deliverables:**

9. **Differential fixture `0015-http-buffer`** under `test/fixtures/0015-http-buffer/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising five scenarios per §6 below. The fixture asserts response status, body byte-exact (overflow body `Payload Too Large`, 17 bytes), header set lowercase wire-form, counter deltas via `/stats/prometheus` scrape equivalence, and per-route-tier independent disposition (both `disabled` and `override` shapes exercised). NO timing-sensitive scenarios (buffer is purely synchronous — no analog to phase 11's `refill-after-fill_interval ±10ms` scenario).

10. **`BEHAVIOR_CONTRACT.md` 4-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.fault`, `### envoy.filters.http.header_mutation`, `### envoy.filters.http.local_ratelimit`, `### envoy.filters.http.csrf` subsections): a NEW `### envoy.filters.http.buffer` subsection covering the 1-consumed / 0-ignored field map, the 413 response wire shape (status 413, body bytes `Payload Too Large`, 4-header set, framing), the per-route disabled-OR-override semantics, the envoy-go-only `max_request_bytes ≤ 1 MiB` validation divergence, and the body-counting algorithm (Content-Length fast-fail + streaming-cap path). Plus the 29→31-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0015 with per-scenario tolerance discipline. Plus a NEW `### Phase 13 forward-pointer notes` subsection under `## Forward-pointer notes` covering the 2-item deferral list (per §8 below).

11. **Anticipated 4 ADRs (ADR-0125 through ADR-0128)** per §7 below. ADR-0124 is the highest-numbered ADR landed in phase 12; ADR-0125 is the next-free.

### 1.2 What 13 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12 precedent; deferrals are 2 items grouped by family-coupling). The summary: configurable per-stream cap above 1 MiB (`per_connection_buffer_limit_bytes`, `per_request_buffer_limit_bytes`, plus arbitrary `max_request_bytes` values > 1 MiB) is out-of-scope. None are blockers for closing row 13 phase-done; the buffer-filter's own `max_request_bytes > 1 MiB` is rejected at parse time per ADR-0126 (envoy-go-only divergence; documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.buffer` subsection); the listener/route per-stream-cap fields stay silent-ignored per ADR-0076 verbatim. No StringMatcher entries, no header-matchers, no rate-limit primitives — buffer is the simplest §9 family proto shape.

### 1.3 Phase-done as a §9 family-row landing

Phase 13's phase-done commit closes ROADMAP row `13` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 13 is the SIXTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, and 12-csrf). The next §9 family-row will be numbered `14` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 13 is **single-row at brainstorm time** — a cohesive ~280-350 LoC implementation slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split would be:

- **13.1 = listener-level filter MVP**: the filter type + factory + `DecodeHeaders` + `DecodeData` body-counting impl + Content-Length fast-fail + streaming-cap path + 2-counter stats + 413 wire shape via framework reuse + listener-level `compiledConfig` parsing. Differential fixture covers listener-only scenarios (1, 2, 3).
- **13.2 = per-route disabled-OR-override TPFC**: per-route `BufferPerRoute` parsing (the new oneof handling) + 3-tier resolver wiring + per-route-disabled fixture scenario (4) + per-route-override fixture scenario (5) + ADR-0125 codification of the 5th canonical per-route discipline.

This split mirrors phase 10 + phase 11 + phase 12's anticipated-but-unused split. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the modest LoC estimate (~280-350 impl + ~450-550 tests + ~60 fuzzer + ~150-200 fixture-Go-driver/backend + ~150 fixture-yaml/README = ~1100-1300 total when including yaml configs and README; ~350 if counting Go production code alone) and modest task count estimate (~10-12 tasks). Both estimates remain comfortably under ADR-0045's 1500 LoC / 25 task split-trigger upstream of either accounting. Phase 13 is structurally smaller than phase 12 at the proto-surface level (1 field vs. 3 fields) but slightly larger at the algorithmic level due to the body-counting + per-route oneof complexity.

### 1.5 Seed-stub alignment

Like phases 09, 10, 11, and 12, phase 13 has NO sibling SPEC stub — phase 13 enters fresh after the phase 12 close. The §9 family-children list at ROADMAP line 58 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 13 has NO such branch. The brainstorm dialogue (Q1-Q4 over the user-Claude exchange) was sufficient to settle MVP envelope + cap interaction + per-route discipline without preliminary scoping notes. This matches the phase 09 / 10 / 12 cold-start precedent.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 8 decisions below are the phase-13-specific design choices. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 Filter package layout *(Decision 1 → ADR-0125)*

**Decision:** New package `internal/filter/http/buffer/` (directory + Go package identifier both `buffer`, single token — matches the cors/fault/csrf precedent; no underscore needed since the proto type-name is already a single token) with files mirroring the cors + fault + csrf precedent: `buffer.go`, `buffer_test.go`, `fuzz_test.go`, `doc.go`. The package exports two top-level symbols: `TypeURL` (string constant, `"type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"`) and `New` (the `HTTPFilterFactory`). All other types (`filter`, `compiledConfig`, `compiledPerRoute`, `filterStats`) are unexported. NO filename underscores needed. Filename is `buffer.go` (mirrors `cors.go`, `csrf.go`).

**Why this vs. alternatives:**
- *Why directory `buffer/` (single token)?* The proto type-name is `Buffer` — a single token. The directory naming convention across §9 family-rows is now: cors / fault / header_mutation (underscore-preserving) / localratelimit (no-underscore per ADR-0114) / csrf / buffer (single token). The single-token cases are unambiguous; phase 13 inherits that.
- *Why not a single `internal/filter/http/buffer.go` flat file?* The existing per-filter discipline is unanimous (cors, fault, router, envoygotest, header_mutation, localratelimit, csrf each get their own subpackage). Subpackage isolation prevents future name collisions and is the project's convention.

**Deferred to SPEC:** the exact file split between `buffer.go` and any helper files (e.g., whether to factor body-counting into its own file `count.go`). The SPEC author chooses based on test readability. No ADR-class commitment from brainstorm.

**ADR anchor:** ADR-0125 — Filter package shape conformance with cors/fault/header_mutation/localratelimit/csrf precedent + boot registration ordering + per-route disabled-OR-override discipline (5th canonical shape).

### 2.2 Extension-registry registration *(Decision 2 → ADR-0125 consequence)*

**Decision:** `cmd/envoy-go/main.go` adds a single new line `httpReg.Register(buffer.TypeURL, buffer.New)` before the existing `cors` registration. The registration ordering is alphabetical-after-router per the ADR-0100 §2.2 convention codified at phase-09 brainstorm time: `router (first) → buffer → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit`. Per ADR-0072, registration ordering does not affect runtime behavior; this is a stylistic discipline only. Phase 13 introduces NO `RegisterPerRouteValidator` hook (unlike phase 10's `header_mutation`) — per-route configs are independently valid (no multi-tier protected-set discipline like header_mutation's; each `compiledPerRoute` validates standalone via the same `parsePerRoute` path).

**Why this vs. alternatives:**
- *Why not registration-order = config-list-order?* Registration order is a global discipline; config-list order is per-listener / per-route. Decoupling avoids cross-cutting coupling (already settled at phase-09 brainstorm time).
- *Why no per-route validator hook?* Phase 10's hook was driven by the multi-tier protected-header eager-validation requirement (which only surfaces when multiple tiers' configs interact). Phase 13's per-route configs are wholesale-override (Decision 5 below); each validates standalone at `parsePerRoute` time. No multi-tier interaction means no eager-validation hook.

**Deferred to SPEC:** none — the line edit is mechanical.

**ADR anchor:** ADR-0125 (consequence; no separate ADR for registration).

### 2.3 MVP envelope: 1-consumed/0-deferred field decomposition *(Decision 3 → ADR-0126)*

**Decision (per Q3 = "MVP requires max_request_bytes ≤ 1 MiB"):** Phase 13 consumes the ONE proto top-level field on `Buffer`:

- **`max_request_bytes`** (`UInt32Value`, REQUIRED) — **CONSUMED** with envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 (1 MiB). Value violations (nil, zero, > 1 MiB) reject at parse time with envoy-go-own error wording (mirrors ADR-0121 precedent — e.g., `"buffer: max_request_bytes is required"`, `"buffer: max_request_bytes must be > 0"`, `"buffer: max_request_bytes (N) exceeds envoy-go cap of 1048576 bytes"`).

The `Buffer` proto has NO other top-level fields. There is no silent-ignore set at the listener-level proto. (`BufferPerRoute` separately has the `disabled` / `buffer` oneof; see Decision 5.)

The 1 MiB ceiling is the load-bearing envoy-go-only validation: reference Envoy v1.37.2 accepts arbitrary `UInt32Value` up to ~4 GiB and would not reject `max_request_bytes = 5 MiB` at parse time. envoy-go rejects per ADR-0126 to close the divergence-window where a > 1 MiB config would trip the framework's 17-byte 413 path (ADR-0076) before the buffer filter's wire shape could fire.

**Why this envelope vs. alternatives:**
- *Why consume the only proto field?* The proto surface IS the field; nothing to defer.
- *Why parse-time validation rather than match-time-truncate-to-1MiB?* Truncation is silent and surprises the operator; parse-time rejection surfaces the discrepancy at config-load (the operator sees the error during boot or LDS push). Consistent with the project's "explicit divergence at config-load over silent runtime drift" discipline (BEHAVIOR_CONTRACT §13).
- *Why 1 MiB exactly (matches ADR-0076 hardcoded `filterBufferLimitBytes`)?* The framework cap is the safety-net; allowing buffer filter values up to but not above the framework cap means buffer's own check fires first inside the cap; the framework cap remains armed but unreachable in MVP. Future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)) lifts both ceilings together.

**Deferred to SPEC:** the precise wire-shape of the parse-time error message format (mirrors ADR-0121 envoy-go-own wording); empirical pin §11.P1 confirms reference Envoy's behavior on the same invalid configs (does Envoy accept `max_request_bytes = 5 MiB`? expected: yes — and that's the divergence we close).

**ADR anchor:** ADR-0126 — `compiledConfig` shape + 1-consumed field decomposition + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence; explicit forward-pointer to the future cap-promotion phase that amends ADR-0076's hardcoded `filterBufferLimitBytes`).

### 2.4 Cap interaction with ADR-0076 framework primitive *(Decision 4 → ADR-0126 consequence)*

**Decision:** The buffer filter's own `max_request_bytes` enforcement layers ABOVE the framework's hardcoded 1 MiB cap (ADR-0076 §Decision (a)). Specifically:

- The buffer filter performs its own per-stream byte-counting against `effectiveMax` (resolved listener-or-per-route value, ≤ 1 MiB by parse-time invariant).
- When buffer's own check trips (Content-Length fast-fail OR streaming overflow), the filter calls `SendLocalReply(413, "Payload Too Large", connClose)` directly — same wire shape as ADR-0076's framework synthesis, just driven by the filter rather than by the framework's `RunDecodeData` overflow branch.
- The framework's hardcoded 1 MiB cap (`filterBufferLimitBytes`) remains armed as a safety net but is structurally unreachable in MVP: if `effectiveMax ≤ 1 MiB` (invariant), then `accumulated > effectiveMax` triggers BEFORE `len(decodeBuf)+len(data) > filterBufferLimitBytes`. The framework path never fires.

**Why this layering vs. alternatives:**
- *Why filter-driven 413 vs. let the framework handle overflow?* Letting the framework handle overflow requires the buffer filter to set `effectiveMax` via a framework-side per-stream cap override (Option B from the cap-interaction Q3 dialogue). That requires a framework primitive (per-stream cap slot) — premature per ADR-0076 §Consequences (d) which identifies compression as the natural amender. Filter-driven 413 keeps phase 13 framework-delta-free.
- *Why 1 MiB exactly as the upper bound for buffer's own check?* Below 1 MiB: buffer's own check fires first, buffer-driven 413 wire shape emits, differential equivalence holds. At 1 MiB exactly: race between buffer's own check (fires at `accumulated > 1 MiB`) and framework's check (fires at `len(decodeBuf)+len(data) > 1 MiB`) — but both paths emit the same byte-equal 413 (per Decision 6); no divergence either way. Above 1 MiB (forbidden by ADR-0126): framework cap fires first with the hardcoded 17-byte body before buffer's check even sees the data; differential drift surfaces. ADR-0126's parse-time rejection prevents this.

**Deferred to SPEC:** §11.P2 — confirm the race-at-exactly-1-MiB case is differentially benign via empirical scrape (drive a request with body exactly at 1 MiB; assert both proxies emit byte-equal 413 with same headers).

**ADR anchor:** ADR-0126 (consequence; cap-layering is the architectural rationale for the parse-time validation).

### 2.5 Per-route TPFC discipline: disabled-OR-override *(Decision 5 → ADR-0125)*

**Decision (per Q4 = "both shapes"):** Per-route `typed_per_filter_config` for buffer carries a `*BufferPerRoute` value (a SEPARATE proto from the listener-level `Buffer` — UNLIKE phase 12 csrf where the same `CsrfPolicy` served both purposes). The `BufferPerRoute` proto has a oneof with two cases:

- **`Disabled{Disabled: true}`** — the filter is wholly inactive on this route. `DecodeHeaders` short-circuits to `Continue`; no body-counting, no `DataStopIterationAndBuffer`, no counter increments. The route bypasses ALL buffer-filter discipline (including the framework's 1 MiB safety-net cap, which is filter-driven via `DataStopIterationAndBuffer` — when the filter never returns that status, the cap never engages).

- **`Buffer{Buffer: &Buffer{MaxRequestBytes: <UInt32Value>}}`** — wholesale override of the listener-level cap. Subject to the same ≤ 1 MiB validation as the listener-level field. The override replaces the listener-level value entirely (NOT field-level merge per ADR-0073 wholesale-override discipline).

- Neither case set: `parsePerRoute` returns `nil` (silent no-op; per-route entry is treated as absent; listener-level fallback applies). Mirrors ADR-0073 silent-no-op precedent.

- BOTH cases set (proto oneof violation): rejected at parse time. Empirical pin §11.P3 confirms reference Envoy's behavior; hypothesis: Envoy's PGV rejects oneof-violation configs at boot.

**Per-route is data-only.** UNLIKE phase 11 (first stateful per-route filter, ADR-0117 amendment to ADR-0073 — per-route entries each owned a `tokenBucket`), phase 13's per-route entries hold ONLY a tiny struct `{disabled bool, maxOverride *uint32}`. NO stateful resources, NO atomic counters, NO mutex-protected runtime state. **Per-route stats are SHARED with listener-level** (mirrors phase-12 csrf ADR-0124 SHARED-stats discipline; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats discipline). Phase 13 adds NO amendment to ADR-0073.

**5th canonical per-route shape.** ADR-0125 codifies "disabled-OR-override" as the 5th canonical per-route discipline:

| # | Discipline | Shape | Stats | Anchor | First user |
|---|---|---|---|---|---|
| 1 | data-only override (most-specific) | wholesale config replacement | shared | ADR-0073 | cors @ 07.1 |
| 2 | multi-tier evaluation | each tier's config evaluated; effects combined | shared | ADR-0110 (ADR-0073 amendment) | header_mutation @ 10 |
| 3 | stateful override with INDEPENDENT stats | per-tier stateful resources + per-tier counters | independent | ADR-0117 (ADR-0073 amendment) | local_ratelimit @ 11 |
| 4 | data-only override with SHARED stats | wholesale config replacement; shared listener-level counters | shared | ADR-0124 | csrf @ 12 |
| **5** | **disabled-OR-override** | **`disabled: true` shortcut OR wholesale override; per-route is structurally a sum type** | **shared** | **ADR-0125** | **buffer @ 13** |

**Why this vs. alternatives:**
- *Why support both shapes in MVP rather than just the override?* The `disabled: true` shortcut is the canonical buffer-filter deployment shape — operators commonly disable buffering on file-upload endpoints while keeping it enabled across the rest of the listener. Without the shortcut, operators must use a special-cased override config (e.g., `max_request_bytes = U32_MAX`) which would be rejected by ADR-0126's ≤ 1 MiB validation. Supporting both shapes makes per-route disable structurally clean.
- *Why wholesale-override vs. field-level merge?* ADR-0073 establishes wholesale-override as the cors/fault/local_ratelimit/csrf precedent. Phase 13 inherits that precedent. Field-level merge for `max_request_bytes` (e.g., min/max of listener-level and per-route values) is plausible but diverges from Envoy.

**Deferred to SPEC:** §11.P3 — confirm PGV oneof-violation behavior; §11.P4 — confirm per-route disabled bypass behavior (does the route really receive arbitrary-size bodies? — relevant for fixture scenario 4).

**ADR anchor:** ADR-0125 — Per-route disabled-OR-override discipline (5th canonical shape) + package shape + boot registration. References ADR-0073 + ADR-0117 + ADR-0124 explicitly.

### 2.6 Body counting + 413 trigger algorithm *(Decision 6 → ADR-0127)*

**Decision:** The filter's body-counting algorithm:

**`DecodeHeaders(headers, endStream)`:**
1. Resolve `(effectiveMax, disabled)` via `resolveEffective(listener, perRoute)` (a small helper local to the package).
2. If `disabled` → set `f.passthrough = true`; return `Continue`.
3. Set `f.effectiveMax = effectiveMax`.
4. If `endStream` (header-only request) → return `Continue` (no body to buffer; no counter touch — empty-body POST/PUT/etc. counts as a fully-buffered request only if it goes through the streaming path with `DecodeData(nil, true)`; the `endStream`-on-headers path is the optimized fast-path for true header-only requests like GET).
5. Read `Content-Length` header. If parseable as integer AND `cl > effectiveMax` → fast-fail: increment `f.stats.requestTooLarge`, call `SendLocalReply(413, "Payload Too Large", connClose)`, return `StopIteration`.
6. Else return `Continue` (begin streaming-cap path).

**`DecodeData(data, endStream)`:**
1. If `f.passthrough` → return `DataContinue` (route disabled; forward raw chunks; framework's safety-net cap never engages because we never return `DataStopIterationAndBuffer`).
2. `f.accumulated += len(data)`.
3. If `f.accumulated > f.effectiveMax` → mid-stream overflow: increment `f.stats.requestTooLarge`, call `SendLocalReply(413, "Payload Too Large", connClose)`, return `DataStopIterationNoBuffer`.
4. If `endStream` → increment `f.stats.requestBuffered`, return `DataContinue` (release the fully-buffered body to the upstream).
5. Else return `DataStopIterationAndBuffer` (accumulate; yield to next chunk; framework holds the bytes in `c.decodeBuf` per ADR-0076 §Decision (b) accumulation discipline).

**`DecodeTrailers(trailers)`** → return `TrailersContinue` (body already complete via the `endStream=true` path in step 4; trailer is bookkeeping).

**`EncodeHeaders / EncodeData / EncodeTrailers`** → pass-through (`Continue` / `DataContinue` / `TrailersContinue`). Buffer is decoder-side-only.

**`OnDestroy`** → no-op. Buffer has no per-request resources to clean up (no timers, no goroutines, no atomic counters to balance; the `accumulated` counter is per-stream and dies with the filter instance).

**Counter increment ordering** (load-bearing for differential):
- `request_buffered` fires **once per fully-buffered request**, on the chunk where `endStream=true` AND `accumulated ≤ effectiveMax`.
- `request_too_large` fires **once per overflowing request**, on either the Content-Length fast-path OR the streaming-cap path — never both (the two paths are mutually exclusive on the same stream because the fast-path returns `StopIteration` from `DecodeHeaders` before any `DecodeData` chunk arrives).
- Disabled per-route requests touch **zero counters**.
- Header-only requests (`endStream=true` on `DecodeHeaders`) touch **zero counters**.

**Why this algorithm vs. alternatives:**
- *Why Content-Length fast-fail vs. always-streaming-cap?* Reference Envoy is hypothesized to early-reject on Content-Length alone (§11.P6 confirms via empirical scrape). Without the fast-fail, envoy-go ships a known divergence-window where clients with `Content-Length: 99GB` get 413 immediately from Envoy but only after streaming bytes from envoy-go.
- *Why `DataStopIterationNoBuffer` on overflow vs. `DataStopIterationAndBuffer`?* The body is being rejected — there's no value in retaining the partial buffer; the framework's `beginLocalReply` path drives the encode chain immediately. Mirrors the framework's own ADR-0076 overflow disposition.

**Deferred to SPEC (empirical pins §11.P5, §11.P6, §11.P7, §11.P8):** see §9.

**ADR anchor:** ADR-0127 — Body-counting + 413-trigger algorithm + Content-Length fast-fail discipline + reuse of framework `SendLocalReply` 413 wire shape (ADR-0076 §Decision (b) byte-equivalence hypothesis).

### 2.7 Stat surface — 29→31-name extension *(Decision 7 → ADR-0128)*

**Decision:** 2 new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 29-name table` (extending to 31-name table):

| Stat name | Type | Increments when |
|---|---|---|
| `http.<HCM stat_prefix>.buffer.request_buffered` | counter | request body fully buffers within `effectiveMax` (emits at `endStream=true` AND `accumulated ≤ effectiveMax`) |
| `http.<HCM stat_prefix>.buffer.request_too_large` | counter | request exceeds `effectiveMax` either via Content-Length fast-fail OR via mid-stream overflow |

**Namespace anchor:** buffer has NO `stat_prefix` proto field (the parent `Buffer` proto has only the `max_request_bytes` field; the `BufferPerRoute` has only the disabled/override oneof). Stats anchor at the HCM's stat_prefix (the same root used by HCM-level stats like `downstream_rq_total`). NO new flattening rule needed — phase 11's SN9 was specific to filters with their own proto `stat_prefix`; buffer reuses the existing HCM-stat_prefix tag-extraction (`envoy_http_conn_manager_prefix` Prometheus tag). Phase 13 does NOT extend ADR-0061's flattening-rule set (Rules SN1-SN8 introduced by phase 06.1, Rule SN9 introduced by phase 11).

**Twin-series filter discipline (per BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline`):** buffer emits ONE series per stat_prefix per counter — no twin-series (no per-cluster fan-out, no per-route emission). Both counters are flat under the HCM stat_prefix. NO permanently-zero counter (unlike phase 09's `fault.response_rl_injected` which is emitted permanently-zero for parity per ADR-0107). The two counters cleanly cover the disposition space — `request_buffered + request_too_large` equals the count of bodied requests evaluated by the filter; disabled-route + header-only paths are excluded from both counters by construction.

**Per-route stats are SHARED with listener-level** (per Decision 5 + ADR-0124 precedent). The same listener-level `*filterStats` pointer is consulted on listener-level requests AND per-route-override requests AND per-route-disabled-bypass paths (the bypass path simply doesn't touch the counter). When the per-route entry is `override`, the body-counting path uses the override cap but the stats touch the SHARED listener-level counter. ADR-0128 §Decision documents the SHARED-stats choice + references ADR-0124 verbatim.

**Why 2 counters vs. more:**
- *Why no per-disposition-type counter (e.g., `request_too_large_content_length` vs. `request_too_large_streaming`)?* Empirical scrape against reference Envoy is hypothesized to emit the same counter on both paths. Distinguishing the source within the counter would diverge.
- *Why no `enabled` analog (cf. local_ratelimit `enabled` counter)?* local_ratelimit's `enabled` counts every evaluated request; for buffer, the analog is `request_buffered + request_too_large` and adding a separate counter is redundant.

**Deferred to SPEC (empirical pin §11.P5):** confirm Prometheus form against Envoy v1.37.2 scrape. Hypothesized form: `envoy_http_buffer_request_buffered{envoy_http_conn_manager_prefix="<HCM stat_prefix>"} <count>` and analogous for `request_too_large`. SPEC §11 confirms exact metric name + label set verbatim.

**ADR anchor:** ADR-0128 — `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 29→31-name extension for 2 `buffer.*` counters + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag) + per-route stats SHARED with listener-level (references ADR-0124 verbatim).

### 2.8 413 wire shape — reuse ADR-0076 framework path *(Decision 8 → ADR-0127 consequence)*

**Decision:** The 413 emitted by the buffer filter on Content-Length fast-fail or mid-stream overflow is byte-equivalent to the 413 emitted by the framework's existing `RunDecodeData` overflow path (ADR-0076 §Decision (b)):

- **Mechanism:** `cb.SendLocalReply(413, "Payload Too Large", connCloseHeaders)` — direct reuse of the framework's `beginLocalReply` path.
- **Status:** `413 Payload Too Large` (NOT configurable via proto; the canonical buffer-filter overflow status — empirical pin §11.P7 confirms).
- **Body:** byte-exact `Payload Too Large` (17 bytes ASCII; constant `localReply413Body` from `internal/filter/http/chain.go:25`; no trailing newline per ADR-0076 §11 #3 empirical pin).
- **Header set on the wire** (lowercase wire-form, 4 headers + connection control): `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`, plus user-supplied `Connection: close` (framework injects + HCM emits). NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked`.

`DecodeData` returns `DataStopIterationNoBuffer` post-`SendLocalReply` (per Decision 6); the framework's `beginLocalReply` runs the encode chain in reverse declaration order; HCM dispatch's wire-write path emits the bytes per ADR-0075.

**Why reuse vs. distinct buffer-filter wire shape:**
- The framework's existing wire shape is already pinned at SPEC §11 #3 (ADR-0076) and shipped at phase 07.1 commit. Reference Envoy's buffer-filter 413 reuses Envoy's stock 413 path (hypothesized — empirical pin §11.P7 confirms). Reusing the same wire shape on envoy-go side preserves byte-equivalence in fixtures by construction; introducing a distinct wire shape would risk an unnecessary divergence-window.
- The framework's `Connection: close` discipline matches reference Envoy's discipline on body-overflow 413 (the connection is closed after the local reply emits because the upstream-bound body is mid-stream and the wire-protocol state is no longer recoverable for keepalive).

**Deferred to SPEC (empirical pins §11.P7, §11.P8):**
- §11.P7: confirm exact status line (`HTTP/1.1 413 Payload Too Large`).
- §11.P8: confirm exact body bytes + header set verbatim against Envoy v1.37.2 scrape on a buffer-filter overflow probe (drive `POST /` with `Content-Length: 2097152` against a buffer config with `max_request_bytes: 1048576`; assert byte-equivalent to the ADR-0076 framework-path scrape from phase 07.1).

**ADR anchor:** ADR-0127 (consequence; the 413 wire shape is part of the body-counting algorithm captured by ADR-0127).

---

## 3. Iteration protocol consequences

Phase 13 buffer interacts with the 07.1 HTTP filter framework as a **purely-synchronous, decoder-side body-touching filter**:

- **`DecodeHeaders`**: full implementation. Reads `:method` (informationally; no method gate), `Content-Length`. Resolves per-route via `RequestRouteConfig().Resolve("buffer", routeIdx)`. Returns `Continue` (passthrough) or `StopIteration` (fast-fail 413).
- **`DecodeData`**: full implementation. Counts accumulated bytes against `effectiveMax`. Returns `DataContinue` (passthrough on disabled, OR endStream-with-fit), `DataStopIterationAndBuffer` (accumulate, framework holds bytes per ADR-0076 accumulation), or `DataStopIterationNoBuffer` (overflow, after `SendLocalReply`).
- **`DecodeTrailers`**: pass-through (`return Continue`). Buffer handles body completion via `endStream=true` on `DecodeData`.
- **`EncodeHeaders` / `EncodeData` / `EncodeTrailers`**: pass-through. Buffer does not modify response state.
- **`OnDestroy`**: no-op. Buffer has no per-request resources to clean up.

**No new framework primitive.** Phase 13 reuses the existing `SendLocalReply` + `StopIteration` machinery from phase 09 fault. Phase 13 reuses the existing `DataStopIterationAndBuffer` accumulation discipline from ADR-0076. Phase 13 reuses the existing 3-tier `PerRouteConfig.Resolve` from phase 07.1. Phase 13 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor.

**No async-resume.** UNLIKE phase 09 fault (`time.AfterFunc` + parkDecode wake-up), buffer's `DecodeHeaders` + `DecodeData` runs synchronously. The disposition is computed inline; `Continue` / `StopIteration` / `DataStopIterationAndBuffer` is returned without parking. The framework's accumulation between chunks IS asynchronous (the dispatch goroutine yields between `DecodeData` calls), but the filter itself does not register any async resumption.

**No stateful per-route resources.** UNLIKE phase 11 local_ratelimit (per-route `tokenBucket`), buffer's per-route `compiledPerRoute` is purely data — `{disabled bool, maxOverride *uint32}`. Each `parsePerRoute` invocation allocates a fresh struct; the struct is read-only at runtime; no mutex, no atomic, no synchronization.

**Listener-level lifecycle.** Buffer listener-level filter instance is a singleton per listener — created once at boot, used across all requests routed through that listener. Per-stream `filter` instances are allocated fresh on each request (the closure returned by `New` materializes a new `*filter` per `(callbacks)` invocation); the `filter` holds the per-stream `accumulated` byte count + the resolved `effectiveMax` + the `passthrough` flag.

**Filter-instance vs. compiledConfig separation.** Per the cors/fault/header_mutation/local_ratelimit/csrf precedent, the `filter` struct holds `*compiledConfig` (pointer to the listener-level compiled config — IMMUTABLE; shared across streams) + a per-stream resolved `*compiledPerRoute` (set during `DecodeHeaders` after `Resolve`); the `compiledConfig` struct holds `{maxRequestBytes uint32, stats *filterStats}`. The factory `New` constructs the `compiledConfig`; the `DecodeHeaders` callback resolves the per-request config (via `Resolve`) and computes the disposition.

---

## 4. Framework deltas — none anticipated

Phase 13 introduces ZERO new framework primitives. Specifically:

- NO new `HTTPFilterFactoryCtx` field (phase 09 added one; phase 13 does NOT).
- NO new `*HTTPRegistry` method (phase 11 added `NewCounterIfAbsent` to `stats.Registry`; phase 13 reuses as-is for the 2 counters).
- NO new `PerRouteConfig` accessor (phase 10 added `ResolveAllTiers`; phase 13 uses the existing 3-tier `Resolve`).
- NO new `RegisterPerRouteValidator` hook (phase 10 added the eager-validation hook; phase 13 does NOT — per-route configs validate standalone via `parsePerRoute`).
- NO amendment to ADR-0073 (phase 10 added an amendment for multi-tier evaluation via ADR-0110; phase 11 added an amendment for stateful per-route via ADR-0117; phase 13 does NOT add a third — per-route is data-only AND most-specific-override; ADR-0125 captures the "disabled-OR-override" shape WITHIN the existing ADR-0073 wholesale-override discipline rather than amending it).
- NO amendment to ADR-0076 (phase 13 layers ABOVE the framework cap rather than promoting it to per-stream tunability; the future cap-promotion phase — likely compression — authors that amender per ADR-0076 §Consequences (d)).
- NO new `internal/stats/name.go` SN flattening rule (phase 11 added Rule SN9; phase 13 does NOT — stats anchor at HCM stat_prefix, reusing existing `envoy_http_conn_manager_prefix` tag-extraction).
- NO load-bearing HCM deviation (UNLIKE phase 12 csrf which carry-forwarded an 8-line `:authority` injection at `internal/filter/hcm/connection.go` + `h2dispatch.go` — buffer reads no pseudo-headers; the existing pseudo-header-injection coverage is sufficient).

This makes phase 13 the structurally-thinnest §9 family-row at the framework-delta level, matching phase 12 csrf. The phase touches only:
- `internal/filter/http/buffer/` (new package, 4 files).
- `cmd/envoy-go/main.go` (1-line registration insert).
- `test/fixtures/0015-http-buffer/` (new fixture, ~5 files).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4-edit bundle: new subsection + 29→31 stat-table extension + new equivalence-matrix row + new forward-pointer subsection).
- `docs/envoy-go/DECISIONS.md` (ADR-0125 through ADR-0128, append-only).
- `docs/envoy-go/ROADMAP.md` (row 13 status flips planned → in-progress → done).
- `docs/envoy-go/STATE.md` (active-phase pointer).
- `docs/envoy-go/phases/13-http-filter-buffer/` (BRAINSTORM.md authored at this commit; SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md authored over phase lifecycle).

---

## 5. Stats — see §2.7

(See Decision 7 in §2.7 above. No additional content here — kept as a section anchor for symmetry with phase 09/10/11/12 BRAINSTORM structures where stats are §5.)

---

## 6. Differential fixture (`0015-http-buffer`)

### 6.1 Topology

`test/fixtures/0015-http-buffer/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config (initially identical; any divergence per ADR-0007 is documented in `README.md`).
- `inputs/driver.go` — Go driver that drives both proxies with identical inputs.
- `expectations.yaml` — per-scenario allow-list / ignore-list / stats-name mapping / timing tolerances.
- `README.md` — fixture overview + scenario list + reference config citations.

Single listener `127.0.0.1:<port>` (HTTP/1.1 plaintext per phases 09/10/11/12 precedent — H2 differential testing of buffer is deferred). One virtual_host `vh_main` with three routes:
- `/` (default route) — uses listener-level Buffer.
- `/route-disabled` — uses per-route TPFC with `disabled: true`.
- `/route-tighter` — uses per-route TPFC with `buffer.max_request_bytes: 131072` (128 KiB; tighter than the 1 MiB listener default).

One cluster `c0` reaching the host-side echo backend at `test/helpers/echobackend/` (the same backend used by phases 09/10/11/12).

**Listener-level `Buffer`:**
```yaml
max_request_bytes: 1048576  # 1 MiB; the envoy-go cap per ADR-0126
```

**Per-route TPFC on `/route-disabled`:**
```yaml
disabled: true
```

**Per-route TPFC on `/route-tighter`:**
```yaml
buffer:
  max_request_bytes: 131072  # 128 KiB
```

### 6.2 5 scenarios

| # | Scenario | Request | Expected response | Counter delta |
|---|---|---|---|---|
| 1 | Body fits within listener cap | `POST /` body=1 KB | 200 + backend body passthrough | `request_buffered +1` |
| 2 | Content-Length fast-fail | `POST /` `Content-Length: 1572864` (1.5 MiB) body=raw bytes | 413 + `Payload Too Large` body + 4-header set + `Connection: close` | `request_too_large +1` |
| 3 | Streaming overflow (chunked, body grows past cap) | `POST /` `Transfer-Encoding: chunked` body~=600 KB streamed against `/route-tighter` (128 KiB cap) | 413 mid-stream + `Payload Too Large` body | `request_too_large +1` |
| 4 | Per-route disabled bypasses cap | `POST /route-disabled` body=1 KB (small body for fixture-tractability; the per-route disable means listener cap doesn't engage) | 200 + backend body | (zero counter touch) |
| 5 | Per-route tighter override trips at smaller cap | `POST /route-tighter` body=200 KB (above 128 KiB override) | 413 + `Payload Too Large` body | `request_too_large +1` |

**Note on scenario 4 body size.** The brainstorm-time hypothesis is to keep the body small (1 KB) to keep the fixture tractable; the per-route `disabled` shape's correctness is asserted by zero counter touch + 200 status, NOT by an oversized body that probes the framework's safety-net cap. Empirical pin §11.P4 verifies whether reference Envoy's per-route disable truly bypasses ALL cap discipline (or whether some upstream connection-cap intervenes); fixture body sizing finalizes after that pin lands.

### 6.3 Asserted equivalence

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status**: byte-equal between Envoy and envoy-go for every scenario (200 on passthrough; 413 on overflow).
- **Response body** on overflow: byte-equal `Payload Too Large` (17 bytes, no trailing newline) for scenarios 2, 3, 5. On passthrough scenarios (1, 4), the body is the backend echo response — set-equal modulo timing/identity headers.
- **Response header set**: lowercase wire-form, set-equal between Envoy and envoy-go modulo the existing `## Header allow-list` (for `date`, `server`, timing/identity headers). On 413: 4-header set (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) plus `Connection: close`; allow-list-modulo only on `date`.
- **Per-counter delta equality**: after the workload completes, scrape `/stats/prometheus` from both proxies and assert per-counter deltas:
    - `buffer.request_buffered`: Envoy `+1` vs. envoy-go `+1` (scenario 1).
    - `buffer.request_too_large`: Envoy `+3` vs. envoy-go `+3` (scenarios 2, 3, 5).
- **Per-route TPFC bucket independence**: scenarios 4 + 5 demonstrate the listener-level config NOT consulted on `/route-disabled` (cap doesn't engage) and the listener-level cap NOT consulted on `/route-tighter` (the override's tighter cap fires).
- **`Connection: close` on 413**: scenarios 2, 3, 5 confirm the connection is closed after the 413 emits (the H1 codec writes `Connection: close` and tears the conn down per ADR-0076 §Decision (b)).

### 6.4 Driver shape

Go driver in `inputs/driver.go` per phase 09/10/11/12 precedent — sequential request loop (race-tolerant scrape ordering); per-scenario assertions inline; final stats scrape via `/stats/prometheus`. Total: 5 requests in the workload (one per scenario; no scenario splits). Estimated driver size: ~150-200 LoC. The chunked-body scenario (3) requires the driver to construct an `http.NewRequest` with a `Transfer-Encoding: chunked` body source (e.g., `io.Pipe` writing bytes incrementally); the driver reads the response stream and asserts the 413 status line + body before the server closes the conn.

**No timing tolerances.** UNLIKE phase 11 fixture 0013 which had a `refill-after-fill_interval ±10ms` scenario, phase 13 fixture 0015 has NO timing-sensitive scenarios — buffer is purely synchronous. Each scenario's response is dispatched within microseconds of the cap being tripped (or the backend response arriving).

**No H2 differential coverage.** Phase 13 fixture 0015 is HTTP/1.1-only. H2 differential testing of buffer is deferred (matching the phase 09/10/11/12 precedent — each filter ships with H1 differential coverage; H2 differential coverage is deferred to a future bundle). The chunked-body scenario (3) is HTTP/1.1-specific; the H2 analog (DATA frames with no Content-Length) would test the same body-counting path, but the H2 wire shape on overflow (RST_STREAM vs. trailers vs. local-reply DATA) needs an empirical pin against H2-mode reference Envoy.

---

## 7. Anticipated ADRs (ADR-0125 through ADR-0128)

4 ADRs anticipated. ADR-0124 is the highest-numbered ADR landed in phase 12; ADR-0125 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0125** | `internal/filter/http/buffer/` package shape — single-token directory matching cors/fault/csrf precedent + boot registration ordering (`router → buffer → cors → csrf → ...`) + `TypeURL` constant + `New` factory + 4-file split + per-route disabled-OR-override discipline (5th canonical per-route shape; references ADR-0073 + ADR-0117 + ADR-0124 explicitly) | Decision 1 (§2.1) + Decision 2 (§2.2) + Decision 5 (§2.5) |
| **ADR-0126** | `compiledConfig` shape + 1-consumed field decomposition + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence) + cap-layering rationale (buffer's check fires inside the framework cap; ADR-0076 stays armed as safety net) + explicit forward-pointer to the future cap-promotion phase | Decision 3 (§2.3) + Decision 4 (§2.4) |
| **ADR-0127** | Body-counting + 413-trigger algorithm — `DecodeHeaders` Content-Length fast-fail discipline + `DecodeData` accumulation + `DataStopIterationAndBuffer` reuse + `DataStopIterationNoBuffer` on overflow + reuse of framework `SendLocalReply` 413 wire shape (ADR-0076 §Decision (b) byte-equivalence hypothesis) | Decision 6 (§2.6) + Decision 8 (§2.8) |
| **ADR-0128** | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 29→31-name extension for 2 `buffer.*` counters (`request_buffered`, `request_too_large`) + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag) + per-route stats SHARED with listener-level (references ADR-0124 verbatim) | Decision 7 (§2.7) |

NO omnibus ADR for deferrals (phase 11 dropped this pattern at SPEC §8.1; deferrals inline in BEHAVIOR_CONTRACT.md `### Phase 13 forward-pointer notes` per §8 below). NO amendment to ADR-0073 (per-route is data-only — no stateful resource carry; ADR-0125 captures the disabled-OR-override shape WITHIN the existing wholesale-override discipline). NO amendment to ADR-0076 (cap-layering rather than promotion; ADR-0126 §Decision references the future cap-promotion phase as the natural amender). NO amendment to ADR-0061 (no new SN flattening rule). NO amendment to ADR-0040 (silent-ignore set is unchanged; the parse-time `max_request_bytes ≤ 1 MiB` validation is a new envoy-go-only discipline, NOT a silent-ignore).

---

## 8. Deferral list

2 deferral items, organized inline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12 precedent). Both items land in `BEHAVIOR_CONTRACT.md ### Phase 13 forward-pointer notes` (a NEW subsection under `## Forward-pointer notes`, sibling to the existing `### Phase 11 forward-pointer notes` and `### Phase 12 forward-pointer notes`).

### 8.1 `max_request_bytes > 1 MiB` (envoy-go-only parse-time rejection)

**Coupled to:** future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)).
**Reference Envoy behavior:** accepts arbitrary `UInt32Value` up to ~4 GiB at parse time; runtime cap is the framework's hardcoded 1 MiB unless overridden via `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` knobs.
**envoy-go behavior:** **rejects at parse time** per ADR-0126 with envoy-go-own error wording. Rejected configs include:
- `Buffer.max_request_bytes = 5 MiB` at listener level.
- `BufferPerRoute.buffer.max_request_bytes = 2 MiB` at any route level.
**Divergence-window:** envoy-go-only PARSE-time rejection vs. Envoy's parse-time accept + runtime-cap-at-1-MiB. Operators with existing configs targeting `max_request_bytes > 1 MiB` against reference Envoy MUST adjust their config (lower the value) to load on envoy-go. Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.buffer` subsection + `### Phase 13 forward-pointer notes`.
**Future re-activation:** the cap-promotion phase amends ADR-0076 to make `filterBufferLimitBytes` per-stream tunable via the listener's `per_connection_buffer_limit_bytes` + the route's `per_request_buffer_limit_bytes` knobs (currently silent-ignored per ADR-0076 §Decision (d)). Once that lands, ADR-0126 amends to remove the parse-time ≤ 1 MiB validation and `max_request_bytes` becomes operationally equivalent to reference Envoy.

### 8.2 `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` (silent-ignore inherited from ADR-0076)

**Coupled to:** same cap-promotion phase.
**Reference Envoy behavior:** these are Listener-scope and Route-scope knobs that scale the framework cap from its 1 MiB default; honored at runtime.
**envoy-go behavior:** silent-ignored at parse time per ADR-0076 §Decision (d). Phase 13 does NOT change this disposition.
**Divergence-window:** unchanged from phase 07.1 + ADR-0076 baseline. Buffer filter does not lift this restriction in MVP.
**Future re-activation:** the cap-promotion phase promotes both fields from silent-ignored to honored, mirroring ADR-0073's promotion path for `typed_per_filter_config`. Once that lands, the buffer filter's `max_request_bytes ≤ 1 MiB` parse-time check (per ADR-0126) is re-evaluated; the natural design is to lift the check to ≤ `per_connection_buffer_limit_bytes`-derived per-stream cap, OR to leave the buffer filter's check at 1 MiB and let the framework cap go higher (in which case buffer's check fires first; same shape as MVP just with a higher framework ceiling).

---

## 9. Empirical pins for SPEC §11

The SPEC author scrapes reference Envoy v1.37.2 in-session per ADR-0004 and confirms each pin verbatim before authoring SPEC §11. If any empirical pin diverges from BRAINSTORM hypothesis, SPEC §11 records the divergence + the SPEC author decides whether to adopt Envoy's behavior (likely) or to file an ADR for deliberate divergence (rare). Phase 12's `filter_enabled` empirical pin (§11.11 in csrf SPEC) revealed a docstring-trust trap-failure analog to phase 11's; phase 13 has structurally fewer pin-driven divergence opportunities because the proto surface is so much smaller (1 field vs. 3).

| ID | Subject | Hypothesis (BRAINSTORM-committed) | Empirical confirmation needed |
|---|---|---|---|
| §11.P1 | Reference Envoy's parse-time disposition on `max_request_bytes > 1 MiB` | Envoy accepts; envoy-go rejects (ADR-0126 envoy-go-only divergence) | Drive Envoy v1.37.2 boot with `max_request_bytes: 5242880` (5 MiB); confirm Envoy boots cleanly. Confirm runtime behavior on a 2 MiB body: framework cap fires at 1 MiB with framework's 17-byte body. |
| §11.P2 | Race-at-exactly-1-MiB | Body exactly at 1 MiB triggers buffer's filter check first; both proxies emit byte-equal 413 | Drive `POST /` with body=1 MiB; assert 413 + `Payload Too Large` from BOTH proxies; confirm header set + counter delta byte-equal. |
| §11.P3 | `BufferPerRoute` oneof violation (both `disabled` and `buffer` set) | Envoy's PGV rejects at boot | Construct config with both fields set; observe Envoy boot/LDS-load behavior. Confirm rejection mechanism (PGV vs. runtime-validation). |
| §11.P4 | Per-route `disabled: true` bypass discipline | Filter is wholly inactive on the route; arbitrary-size bodies pass through (modulo HCM/connection-level caps); zero counter touch | Drive `POST /route-disabled` with body=2 MiB; observe whether Envoy returns 200 (filter bypassed) or 413 (some other cap intervened). Scrape stats: confirm `buffer.*` counters did not increment for the disabled-route path. |
| §11.P5 | Stats Prometheus form | `envoy_http_buffer_request_buffered{envoy_http_conn_manager_prefix="<HCM stat_prefix>"} <count>` and `envoy_http_buffer_request_too_large{...}` analogous | Scrape `/stats/prometheus` after a defined load; confirm exact metric names + label sets + tag-extraction. Confirm ABSENCE of any `buffer.*_with_disposition` per-path-distinguishing counter. |
| §11.P6 | Content-Length fast-fail discipline | Envoy early-rejects on `Content-Length > effectiveMax` at header-parse time (before any body bytes are read) | Drive `POST /` with `Content-Length: 2097152` and zero-byte body (immediately close the request body stream); observe whether Envoy returns 413 immediately (fast-fail) or accepts the request (waits for body to stream). |
| §11.P7 | 413 status line | `HTTP/1.1 413 Payload Too Large` exact | Scrape: confirm exact status line on a buffer-filter overflow probe. |
| §11.P8 | 413 body bytes + header set | byte-exact `Payload Too Large` (17 bytes, no LF) + 4-header lowercase wire-form (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + `Connection: close` | Scrape: byte-count + `xxd`-equivalent dump of the response body; full header dump on a 413 buffer-filter overflow probe; confirm absence of `cache-control`, `x-content-type-options`, `transfer-encoding: chunked`, and `charset=UTF-8` modifier. Confirm match with the existing ADR-0076 framework-path scrape from phase 07.1 (same wire shape both paths). |
| §11.P9 | Per-stream buffer accumulation under `Transfer-Encoding: chunked` | Each chunk increments `accumulated`; the cap fires at `accumulated > effectiveMax` (mid-stream) | Drive `POST /route-tighter` with `Transfer-Encoding: chunked` and incremental writes; observe at which byte-count Envoy emits 413. Confirm match with envoy-go's `accumulated > effectiveMax` discipline. |
| §11.P10 | Header-only request disposition | `GET /` (no body, `endStream=true` on headers) is a pure passthrough; no counter touch | Scrape: drive `GET /` against a buffer-configured listener; confirm `request_buffered` and `request_too_large` counters BOTH zero. |
| §11.P11 | Empty-body `POST` disposition | `POST /` with `Content-Length: 0` AND `endStream=true`-on-headers fast-path → `Continue` from `DecodeHeaders` directly; counter touch is the question | Drive `POST /` with `Content-Length: 0`; observe whether Envoy increments `request_buffered` (treating empty body as fully-buffered) or skips the counter (treating empty body as no-body). The brainstorm hypothesis is "header-only fast-path → no counter touch" but Envoy may differ; pin confirms. |

If the SPEC author finds any divergence from these hypotheses, SPEC §11 records the divergence verbatim + reconciles with the BRAINSTORM Decision (most likely by amending the Decision in SPEC; the BRAINSTORM is not edited post-landing per D-3.5 + D-3.4).

---

## 10. ROADMAP delta

### 10.1 New row added by this brainstorm

This brainstorm appends one new row to `docs/envoy-go/ROADMAP.md` under the MVP Trunk table:

```
| 13 | http-filter-buffer | 12 | planned | | <summary post-SPEC> |
```

Status starts at `planned`. The next session (lifecycle-state 1 → 2; skill `superpowers:writing-plans` per ADR-0005) flips to `in-progress` upon authoring `phases/13-http-filter-buffer/SPEC.md`. The phase-done commit flips to `done` per the lifecycle-state-6 transition (BOOTSTRAP_PROMPT.md §5).

### 10.2 §9 family heading at ROADMAP line 56 stays unchanged

Per ADR-0106(c), the §9 family heading at ROADMAP line 56 (`### HTTP filters family`) is a conceptual umbrella, not a row. Phase 13's landing does NOT modify the heading's text or position. The phase-done commit message body explicitly states: (1) ROADMAP row 13 flips planned → in-progress → done; (2) the §9 family heading at ROADMAP line 56 stays unchanged; (3) phase 13 is the SIXTH §9 family-row to land.

### 10.3 No-sibling-stub discipline (per ADR-0106(b))

This brainstorm does NOT pre-populate stub rows for the other §9 family-children (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 11. Test scaffolding

### 11.1 Unit tests (`buffer_test.go`)

Likely 18-25 test functions across 6 groups (mirrors phase-12 csrf's 6-group structure):

**G1: Factory parse** — `TestNew_*`:
- valid policy (`max_request_bytes=1MiB`); roundtrip via the `New` factory.
- nil policy (proto unmarshal failure → returns error).
- nil `max_request_bytes` field (UInt32Value unset → returns error with `"buffer: max_request_bytes is required"`).
- `max_request_bytes.value == 0` (returns error).
- `max_request_bytes.value > 1 MiB` (returns error with `"buffer: max_request_bytes (N) exceeds envoy-go cap of 1048576 bytes"`).
- malformed Any (proto unmarshal failure).

**G2: PerRoute parse** — `TestParsePerRoute_*`:
- `Disabled{Disabled: true}` parses to `&compiledPerRoute{disabled: true}`.
- `Buffer{Buffer: &Buffer{MaxRequestBytes: 65536}}` parses to `&compiledPerRoute{maxOverride: &65536}`.
- `Buffer{Buffer: &Buffer{MaxRequestBytes: 0}}` rejects (same envoy-go-own wording as listener).
- `Buffer{Buffer: &Buffer{MaxRequestBytes: 5MiB}}` rejects (≤ 1 MiB validation applies to per-route too).
- both fields set (PGV oneof violation hypothesis; pin §11.P3) — disposition empirically pinned.
- neither field set returns nil (silent no-op).

**G3: DecodeHeaders** — `TestDecodeHeaders_*`:
- per-route `disabled` → passthrough (returns `Continue`; sets `f.passthrough=true`).
- per-route `disabled` does NOT touch counters.
- header-only request (`endStream=true`) → passthrough (returns `Continue`; no counter touch).
- `Content-Length` known and ≤ effectiveMax → returns `Continue`.
- `Content-Length` known and > effectiveMax → fast-fail 413 + `request_too_large +1`.
- `Content-Length` malformed (multi-value, non-integer) → falls through to streaming-cap path.
- nil per-route → uses listener-level cap.
- nil `RequestRouteConfig()` (defensive) → uses listener-level cap.

**G4: DecodeData** — `TestDecodeData_*`:
- accumulation across multiple chunks below cap → `DataStopIterationAndBuffer` per chunk; counter untouched until endStream.
- single-chunk fits within cap with `endStream=true` → `DataContinue` + `request_buffered +1`.
- single-chunk overflows cap → `DataStopIterationNoBuffer` + `request_too_large +1` + `SendLocalReply` invoked.
- multi-chunk total exceeds cap mid-stream → 413 fires on the overflowing chunk.
- empty body (`endStream=true`, `len(data)==0`) → `DataContinue` + `request_buffered +1` (matches header-only-on-DecodeData semantics).
- per-route `disabled` (passthrough flag set) → `DataContinue` per chunk; framework's safety-net cap never engages because filter never returns `DataStopIterationAndBuffer`.

**G5: Per-route integration** — `TestPerRouteIntegration_*`:
- listener default used when per-route nil.
- per-route override smaller-than-listener (cap fires at the smaller value).
- per-route override larger-than-listener within 1 MiB (cap fires at the larger value).
- per-route disabled bypasses all cap discipline.
- per-route resolve called once per stream (not per chunk; covered by spying on the callback).

**G6: Stats** — `TestStats_*`:
- `request_buffered` fires once per fully-buffered request.
- `request_too_large` fires once on Content-Length fast-fail.
- `request_too_large` fires once on streaming overflow.
- mutually-exclusive paths verified (CL fast-fail returns `StopIteration` from `DecodeHeaders`; `DecodeData` is never called → can't double-increment).
- disabled route touches zero counters.
- header-only request touches zero counters.
- aggregation across listener+per-route paths uses the SHARED `*filterStats` pointer (per Decision 7 + ADR-0124 SHARED-stats discipline).

### 11.2 Fuzzer (`fuzz_test.go`)

`FuzzBufferConfigParse` — the **17th fuzzer** in the repo. Fuzz target: random bytes → `protojson.Unmarshal` into `*Buffer` → `New(ctx, cfg)`; assert no panic + invariant `(factory, nil) ∨ (nil, error)`. Seed corpus: 4-6 entries (valid 1KB / valid 1MiB / invalid 0 / invalid >1MiB / malformed bytes / empty Any).

The 16 prior fuzzers (per phase 12 STATE.md): trivially listed by `find . -name 'fuzz_test.go'` — phases 06.1 through 12 each landed at least one. Phase 13 adds one.

### 11.3 No new conformance suite

Buffer is HTTP-filter-only; no h2spec/h3spec implications. The phase 13 phase-done gate runs the existing h2spec at the ADR-0051 pin (53/53 PASS) — no new conformance addition.

### 11.4 Race-test discipline

All unit tests + the existing differential suite + the new fixture 0015 must pass under `go test -race ./...` per the phase-done gate (BOOTSTRAP_PROMPT.md §7.5(e)). Buffer has no shared mutable state at the filter-instance level (the `compiledConfig` is read-only after `New`; `compiledPerRoute` is read-only after `parsePerRoute`; the per-stream `accumulated` counter is stream-local; the `filterStats` counters use the existing `atomic.AddUint64` discipline) — race-test cleanness is structurally guaranteed; no special discipline required.

---

## 12. Empirical amendment — 2026-05-09 (post-landing)

**Trigger.** SPEC-authoring session 2026-05-09 began executing the §9 empirical-pin obligations against `envoyproxy/envoy:v1.37.2` (digest `sha256:c5e8a68e52f4…`) per ADR-0004. The first three pins surfaced findings that overturn the algorithmic core hypothesized in §2.6 + §2.7 + §2.8. The user paused SPEC drafting and chose the rare D-3.5 amendment route (§12) over routing the divergence through SPEC §11 alone, on the grounds that the empirical re-frame is large enough that the SPEC author benefits from a corrected design sketch — not just a §11 amendment block. §§1–11 above are PRESERVED VERBATIM (D-3.5 immutability of the original brainstorm landing); §12 documents the post-landing reconciliation and SUPERSEDES the affected design Decisions for SPEC-drafting purposes.

**Scope of preservation.** §§1.1 (deliverable count), 1.4 (LoC envelope), 2.5 (per-route discipline), 2.6 (body-counting algorithm), 2.7 (stat surface), 2.8 (413 wire shape), 6 (fixture matrix), 7 (ADR roster), 8 (deferral list), and 11 (test scaffolding) are SUPERSEDED in part or whole by §12. §§1.2 (forward-pointers), 1.3 (phase-done framing), 1.5 (no-sibling-stub), 1.6 (no-prebrainstorm-branch), 2.1 (package layout), 2.2 (boot registration), 2.3 (1-consumed/0-deferred field decomposition), 2.4 (cap-layering rationale), 3 (iteration protocol), 4 (no framework deltas — STILL HOLDS), 5 (stats anchor — superseded by §12.5), 9 (empirical pin enumeration — RESOLVED in §12.3 below), and 10 (ROADMAP delta) are unchanged.

### 12.1 Source-of-truth excerpts (`source/extensions/filters/http/buffer/buffer_filter.cc` at tag `v1.37.2`)

The Envoy buffer filter is 102 lines of C++ (verbatim fetched from `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/http/buffer/buffer_filter.cc`). The load-bearing methods:

```cpp
// buffer_filter.cc:50-67
Http::FilterHeadersStatus BufferFilter::decodeHeaders(Http::RequestHeaderMap& headers,
                                                      bool end_stream) {
  if (end_stream) {
    // If this is a header-only request, we don't need to do any buffering.
    return Http::FilterHeadersStatus::Continue;
  }
  initConfig();                        // resolves per-route most-specific config
  if (settings_->disabled()) {
    // The filter has been disabled for this route.
    return Http::FilterHeadersStatus::Continue;
  }
  callbacks_->setBufferLimit(settings_->maxRequestBytes());   // delegates cap to HCM
  request_headers_ = &headers;
  return Http::FilterHeadersStatus::StopIteration;            // holds headers until end-stream
}

// buffer_filter.cc:69-79
Http::FilterDataStatus BufferFilter::decodeData(Buffer::Instance& data, bool end_stream) {
  content_length_ += data.length();
  if (end_stream || settings_->disabled()) {
    maybeAddContentLength();
    return Http::FilterDataStatus::Continue;
  }
  // Buffer until the complete request has been processed or the ConnectionManagerImpl sends a 413.
  return Http::FilterDataStatus::StopIterationAndBuffer;
}

// buffer_filter.cc:91-97
void BufferFilter::maybeAddContentLength() {
  // request_headers_ is initialized iff plugin is enabled.
  if (request_headers_ != nullptr && request_headers_->ContentLength() == nullptr) {
    ASSERT(!settings_->disabled());
    request_headers_->setContentLength(content_length_);
  }
}
```

Five algorithmic facts derive from the code:

1. **Header-only fast-path.** `end_stream=true` on `decodeHeaders` returns `Continue` immediately; the filter never engages.
2. **Per-route disabled bypass.** `settings_->disabled()` on a non-end-stream request returns `Continue` without setting any cap; the filter does NO body work; `decodeData` short-circuits via the same `disabled()` check.
3. **Cap delegation, not enforcement.** When the filter does engage, it calls `callbacks_->setBufferLimit(maxRequestBytes)` and returns `StopIteration` from `decodeHeaders`. The cap is enforced by HCM's per-stream buffer limit machinery, not by the filter. The 413 on overflow is emitted by `ConnectionManagerImpl`, NOT by the filter — see the line 77 source comment "Buffer until the complete request has been processed or the ConnectionManagerImpl sends a 413."
4. **No Content-Length fast-fail.** Nowhere does the filter inspect the `Content-Length` header. The cap fires only after data accumulates past the limit. (Probe §12.3.P6 confirms: a request with `Content-Length: 6291456` and zero body bytes does NOT 413 — Envoy waits for body, eventually times out the connection.)
5. **Content-Length injection on chunked completion.** `maybeAddContentLength()` injects `Content-Length: <accumulated>` on the held headers when end-stream arrives AND the original request had no Content-Length (chunked transfer). This is observable on the upstream side: Envoy converts chunked → fixed-CL before forwarding.

### 12.2 BufferPerRoute proto + decoder rejection mechanism (resolves §9.P3)

The per-route message (verbatim from `buffer.proto` v3, type-URL `envoy.extensions.filters.http.buffer.v3.BufferPerRoute`):

```protobuf
message BufferPerRoute {
  oneof override {
    option (validate.required) = true;
    bool disabled = 1 [(validate.rules).bool.const = true];
    Buffer buffer = 2;
  }
}
```

Two PGV constraints: (a) the oneof has `validate.required` — exactly one of `disabled` or `buffer` must be set; (b) `disabled` has `bool.const = true` — only `disabled: true` is accepted (omitting it OR setting `disabled: false` are both rejected).

Probe §12.3.P3 confirms: setting BOTH `disabled: true` AND `buffer: {…}` in the same `BufferPerRoute` entry rejects at boot, but the rejection mechanism is the **JSON→proto decoder**, not PGV. The error wording is `'disabled' has already been set (either directly or as part of a oneof)` — surfaced from protobuf-cpp's `JsonStringToMessage` before PGV runs. This is structurally upstream of the PGV `validate.required` check.

For envoy-go (which uses `protojson.Unmarshal`), the same proto3 oneof discipline applies: the JSON decoder rejects oneof violations before any PGV-mirror validation sees the message. envoy-go's `parsePerRoute` inherits this for free; no envoy-go-specific PGV-mirror code is needed for the oneof-violation case.

### 12.3 Empirical pin disposition table (resolves §9.P1..P11)

Probe machinery: 5 per-pin bootstrap YAMLs at `/tmp/p13-pins/p{1,3,4}-*.yaml` (P2/P5/P6/P7/P8/P9/P10/P11 share `/tmp/p13-pins/p1-overcap.yaml` or `/tmp/p13-pins/p4-disabled.yaml` since they vary only the request shape against a stable bootstrap); reference Envoy in `--network=host` Docker container; backend Python `BaseHTTPRequestHandler` on host loopback reached via `host.docker.internal` from the container netns; `curlimages/curl:latest` sidecar `--network=host` issues probes. Verbatim probe transcripts are durable on this commit's machine at `/tmp/p13-pins/` and will be re-captured + recorded in SPEC §11 by the next session per phase 09/10/11/12 §11 discipline. The summary below is the reconciliation; the SPEC author re-runs each probe and records the verbatim output in SPEC §11.

| Pin | BRAINSTORM hypothesis | Empirical reality | Verdict | Lands at |
|---|---|---|---|---|
| §9.P1 | Envoy boots cleanly with `max_request_bytes=5 MiB`; framework cap fires at 1 MiB on a 2 MiB body | Envoy boots cleanly with 5 MiB cap (✓); 2 MiB body **passes through** to upstream (5 MiB cap not engaged); 6 MiB body emits 413 with framework's 17-byte body | **AMENDED.** Envoy enforces the buffer filter's own cap (no separate "framework 1 MiB cap"). The "framework cap" framing is envoy-go-internal (ADR-0076's `filterBufferLimitBytes`); reference Envoy has no such hardcoded cap distinct from the filter's value. | §12.4 Decision 6 v2 |
| §9.P2 | Body exactly at 1 MiB triggers buffer's check first; both proxies emit 413 | Body exactly at 1 MiB → **200 OK** (cap is `>`, not `>=`); 1 MiB + 1 byte → 413 | **AMENDED.** Cap predicate is `accumulated > effectiveMax`; exact-cap fits. | §12.4 Decision 6 v2 |
| §9.P3 | PGV rejects oneof violation at boot | Rejected at boot, but mechanism is JSON→proto decoder error `'disabled' has already been set (either directly or as part of a oneof)`, NOT PGV | **CONFIRMED rejection; AMENDED mechanism.** envoy-go's `protojson.Unmarshal` mirrors this for free. | §12.4 Decision 5 v2 |
| §9.P4 | Per-route `disabled: true` bypasses ALL cap discipline | CONFIRMED. `POST /route-disabled` body=2 MiB → 200 (passthrough); body=6 MiB → 503 from upstream (NOT a buffer 413; upstream cluster's connection-level handling rejected). Filter is wholly inactive when `settings_->disabled()` is true. | **CONFIRMED.** | §12.4 Decision 5 v2 |
| §9.P5 | `envoy_http_buffer_request_buffered{…}` + `envoy_http_buffer_request_too_large{…}` 2-counter Prometheus surface; 29→31 stat-table extension via ADR-0128 | **NO `buffer.*` Prometheus counters exist in Envoy v1.37.2.** `/stats/prometheus` scrape after 4 buffer-overflow probes shows zero `envoy_http_buffer_*` metrics. The only relevant counter is `envoy_http_downstream_rq_too_large{envoy_http_conn_manager_prefix="ingress_pX"}` (HCM-level, generic — already in the 29-name table from phase 06.1; phase 13 contributes ZERO new stat-table entries). | **MAJOR AMENDMENT.** Phase 13's stat-table delta is ZERO. ADR-0128 retired (see §12.5). The buffer filter is observed entirely via the existing HCM `downstream_rq_*` family. | §12.5 Decision 7 v2 |
| §9.P6 | Envoy CL fast-fails at `decodeHeaders` time when `Content-Length > effectiveMax` | Envoy does **NOT** fast-fail. Probe with `Content-Length: 6291456` and 0 body bytes hangs (no response in 5 s; connection eventually times out). Probe with `Content-Length` matching body size emits `100 Continue` then 413 only AFTER the body stream exceeds the cap. The filter never reads `Content-Length` (see source §12.1 `decodeHeaders`). | **MAJOR AMENDMENT.** No CL fast-fail. The body-counting path is purely streaming. | §12.4 Decision 6 v2 |
| §9.P7 | `HTTP/1.1 413 Payload Too Large` exact status line | CONFIRMED on every overflow probe (P1 6 MiB; P2-edge 1 MiB+1; P4-C 2 MiB; P4-D 200 KiB chunked; P9 200 KiB chunked). | **CONFIRMED.** | §12.4 Decision 8 v2 |
| §9.P8 | Body bytes byte-exact `Payload Too Large` (17 bytes, no LF) + 4-header lowercase wire-form (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + `Connection: close` | CONFIRMED. Addendum: when curl uses `--data-binary @<large-file>`, Envoy emits `100 Continue` BEFORE the eventual 413 (curl auto-injects `Expect: 100-continue` for large bodies). With `Transfer-Encoding: chunked`, NO `100 Continue` is emitted (curl does not inject `Expect:` for chunked). The 413 wire shape itself is identical in both cases. | **CONFIRMED + 100-Continue addendum.** | §12.4 Decision 8 v2 + SPEC §11 record |
| §9.P9 | `Transfer-Encoding: chunked` accumulation: cap fires at `accumulated > effectiveMax` mid-stream | CONFIRMED. Chunked 200 KiB → /route-tighter (128 KiB cap) → 413; chunked 64 KiB → /route-tighter → 200. | **CONFIRMED.** | §12.4 Decision 6 v2 |
| §9.P10 | Header-only `GET /` is pure passthrough; zero counter touch on buffer-specific stats | CONFIRMED (vacuously, since no `buffer.*` counter exists per §9.P5). HCM-level `downstream_rq_2xx + 1` and `downstream_rq_completed + 1` are the only deltas — same as any other GET. | **CONFIRMED.** | §12.5 Decision 7 v2 |
| §9.P11 | Empty-body `POST` (`Content-Length: 0`) increments `request_buffered` (or doesn't — pin asks) | The `request_buffered` counter does not exist per §9.P5; question is moot. Empirically, `POST` with `CL=0` returns 200; only HCM-level `downstream_rq_2xx + 1` and `downstream_rq_completed + 1` increment. | **MOOT.** | §12.5 Decision 7 v2 |

### 12.4 Re-framed Decisions 5, 6, 8 (supersede §2.5 + §2.6 + §2.8 for SPEC-drafting purposes)

#### Decision 5 v2 — Per-route TPFC: still disabled-OR-override, but PGV mechanism amended

**Decision (carries §2.5 + amends mechanism):** `BufferPerRoute` proto carries the oneof `{disabled: true, buffer: Buffer}`. envoy-go's `parsePerRoute` accepts each shape and produces a `*compiledPerRoute` value:

- `disabled: true` → `&compiledPerRoute{disabled: true}` (filter is wholly inactive on this route).
- `buffer: {max_request_bytes: N}` → `&compiledPerRoute{maxOverride: &N}` (subject to the same ≤ 1 MiB validation as listener-level per ADR-0126; rejection wording mirrors ADR-0121 precedent).
- Both fields set → **rejected by `protojson.Unmarshal` BEFORE `parsePerRoute` runs**. The rejection mechanism is the proto3 oneof discipline (JSON decoder error: `'disabled' has already been set (either directly or as part of a oneof)`). envoy-go inherits this for free; no PGV-mirror code is needed for the oneof case. PGV's `validate.required` constraint on the oneof is structurally downstream of the JSON decode failure and is never reached for this input class.
- Neither field set → **rejected by PGV's `validate.required = true` constraint** (NOT silent no-op as §2.5 hypothesized; envoy-go MUST PGV-mirror the `validate.required` constraint at parse time per ADR-0121 precedent). Wording: `"buffer per-route: override oneof is required"`.
- `disabled: false` → **rejected by PGV's `bool.const = true` constraint** (the proto only accepts `disabled: true`). envoy-go MUST PGV-mirror this.

The rest of §2.5 (5th canonical per-route discipline, ADR-0125 anchor, no amendment to ADR-0073, SHARED stats with listener-level) is **unchanged**. The 5-row canonical-shape table at §2.5 still holds.

#### Decision 6 v2 — Body-counting algorithm (REPLACES §2.6 in full)

**Decision (replaces §2.6):** envoy-go's filter algorithm DOES NOT mirror Envoy's `setBufferLimit + StopIteration + HCM-emits-413` model directly — envoy-go's framework lacks a per-stream cap-override primitive (per §4 invariant + ADR-0076 §Consequences (d) which defers cap promotion to a future phase). Instead, envoy-go's filter does its own per-stream byte-counting in `DecodeData` and emits the 413 itself via `SendLocalReply`, while preserving WIRE-EQUIVALENT outcomes with Envoy on every observable axis (status, body, headers, counter).

**Algorithm:**

`DecodeHeaders(headers, endStream)`:
1. If `endStream` → return `Continue` (header-only request; filter does no work).
2. Resolve `(effectiveMax, disabled)` via `RequestRouteConfig().Resolve("buffer", routeIdx)` + listener fallback.
3. If `disabled` → set `f.passthrough = true`; return `Continue` (per-route bypass; `DecodeData` short-circuits).
4. Else: store `effectiveMax`, store `f.headersRef = headers` (for §maybeAddContentLength mirror in step 7); return **`StopIteration`** (mirrors Envoy line 66 — holds headers until end-stream so that the upstream-side observable is the complete request with corrected Content-Length).

`DecodeData(data, endStream)`:
1. If `f.passthrough` → return `DataContinue` (route disabled; forward chunks raw; framework's safety-net cap never engages because we never return `DataStopIterationAndBuffer`).
2. `f.accumulated += len(data)`.
3. If `f.accumulated > f.effectiveMax` → **mid-stream overflow**: increment HCM `downstream_rq_too_large` (see §12.5); call `SendLocalReply(413, "Payload Too Large", connClose)`; return `DataStopIterationNoBuffer` (discards the partial buffer; framework's `beginLocalReply` runs the encode chain immediately).
4. If `endStream`: invoke `maybeAddContentLength` (step 7) to inject `Content-Length: <accumulated>` on the held headers when the request was chunked; release the held headers + body; return `DataContinue`.
5. Else (in-flight chunk; more to come): return `DataStopIterationAndBuffer` (accumulate; framework holds bytes per ADR-0076 §Decision (b)).

`DecodeTrailers(trailers)`:
- Invoke `maybeAddContentLength` (in case end-stream arrived via trailers, not via terminal `endStream=true` data chunk); return `TrailersContinue`.

`maybeAddContentLength` (helper, mirrors `buffer_filter.cc:91-97`):
- If `f.headersRef != nil` AND the original request had no `Content-Length` header → set `Content-Length: <f.accumulated>` on the held headers. The discipline is: chunked → fixed-CL conversion before forwarding upstream. This is observable on the backend; phase 13 fixture 0015 must assert byte-equivalence on the upstream-received `Content-Length`.

`EncodeHeaders/EncodeData/EncodeTrailers`: pass-through (`Continue`/`DataContinue`/`TrailersContinue`); buffer is decoder-side-only.

`OnDestroy`: no-op; buffer has no per-request resources to clean up.

**Why this divergence is acceptable.** envoy-go's filter does work that Envoy delegates to HCM (byte-counting + 413 emission), but the WIRE OUTCOMES are byte-equivalent: status `413`, body `Payload Too Large` 17 bytes, 4-header set lowercase wire-form, `Connection: close`, plus the same `downstream_rq_too_large` counter increment (§12.5). The structural divergence is observable only via `maybeAddContentLength` semantics + trailer-arrival edge cases, which §6 fixture 0015 covers.

**Counter increment ordering** (load-bearing for differential):
- HCM `downstream_rq_too_large` fires **once per overflowing request** on the chunk where `accumulated > effectiveMax`.
- Disabled per-route requests touch zero `buffer.*` counters (because none exist) AND zero `downstream_rq_too_large` (the cap never engages on the bypass path).
- Header-only requests (`endStream=true` on `DecodeHeaders`) touch zero `buffer.*` counters AND zero `downstream_rq_too_large`.
- Empty-body POST (`Content-Length: 0`) touches zero `buffer.*` counters AND zero `downstream_rq_too_large` (no overflow).

**ADR anchor for v2:** ADR-0127 (renumbered to ADR-0127 v2) — Body-counting algorithm + maybeAddContentLength mirror + reuse of framework `SendLocalReply` 413 wire shape. The Decision 6 v1 Content-Length fast-fail clause is REMOVED; the §6 fixture matrix scenario 2 is restructured (§12.6).

#### Decision 8 v2 — 413 wire shape + 100-Continue addendum (supersedes §2.8)

**Decision (carries §2.8 + 100-Continue addendum):** The 413 emitted by the buffer filter on mid-stream overflow is byte-equivalent to the 413 emitted by the framework's existing `RunDecodeData` overflow path (ADR-0076 §Decision (b)):
- Status: `HTTP/1.1 413 Payload Too Large`.
- Body: `Payload Too Large` (17 bytes ASCII; constant `localReply413Body` from `internal/filter/http/chain.go:25`; no trailing newline).
- Headers (lowercase wire-form): `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`, plus user-supplied `Connection: close`.

**100-Continue addendum (§12.3.P8 finding):** Reference Envoy emits `HTTP/1.1 100 Continue` BEFORE the eventual 413 when the request includes `Expect: 100-continue` (curl auto-injects this for large bodies via `--data-binary @<file>`). With `Transfer-Encoding: chunked`, no `100 Continue` is emitted. envoy-go's HCM/H1-codec `100 Continue` discipline (already shipped in phase 04) handles this transparently — buffer filter does NOT need to emit `100 Continue` itself; the HCM does. Phase 13 fixture 0015 driver MUST account for the `100 Continue` line in transcripts when curl-style probes are used; the assertion shape becomes "first non-1xx response is 413" rather than "first response is 413".

**ADR anchor for v2:** ADR-0127 v2 (consequence; same anchor as Decision 6 v2).

### 12.5 Re-framed Decision 7 — Stat surface (REPLACES §2.7 + retires ADR-0128)

**Decision (replaces §2.7):** **Phase 13 contributes ZERO new entries to `BEHAVIOR_CONTRACT.md ## Stat-name mapping`. The 29-name table stays at 29 names.** Buffer-filter overflow increments the existing HCM-level counter `http.<HCM stat_prefix>.downstream_rq_too_large` (already in the 29-name table from phase 06.1); buffer-filter passthrough increments `http.<HCM stat_prefix>.downstream_rq_2xx + downstream_rq_completed` (also pre-existing).

**Mechanism in envoy-go:** the `SendLocalReply(413, …)` call in `DecodeData` triggers HCM's `downstream_rq_4xx` + `downstream_rq_too_large` counters via the existing post-413 counter-emission discipline shipped in phase 06.1. The buffer filter does NOT directly increment any counter; the HCM-level counters are incremented automatically by the post-`SendLocalReply` framework code path.

**Prometheus form (§12.3.P5 verbatim scrape):**
```
# TYPE envoy_http_downstream_rq_too_large counter
envoy_http_downstream_rq_too_large{envoy_http_conn_manager_prefix="ingress_p4"} 4
```
This is the `envoy_http_conn_manager_prefix`-tagged HCM counter from phase 06.1 + ADR-0061 SN-rule set; no new SN rule is needed. Phase 13 retires the SN-rule extension hypothesized in §2.7.

**ADR-0128 retired.** The anticipated ADR slot is no longer needed (no new stat-table entry; no new SN rule; no SHARED-vs-INDEPENDENT stats decision since there's nothing buffer-specific to share). Phase 13 anticipated ADRs become **3, not 4**: ADR-0125, ADR-0126, ADR-0127 v2. The next-free ADR after phase 13 is ADR-0128 (preserved for the next phase).

**§5 (BRAINSTORM stats placeholder section) is updated.** The §5 anchor stays as a structural placeholder; its content is now "see §12.5" rather than "see §2.7."

### 12.6 Re-framed §6 fixture 0015 (5 scenarios, restructured)

The fixture topology in §6.1 (single listener, three routes `/`, `/route-disabled`, `/route-tighter`, listener-level cap 1 MiB, `/route-disabled` per-route disabled, `/route-tighter` per-route 128 KiB override) is **unchanged**.

The 5-scenario matrix in §6.2 is restructured to drop the now-impossible Content-Length fast-fail scenario:

| # | Scenario | Request | Expected | Counter delta |
|---|---|---|---|---|
| 1 | Body fits within listener cap | `POST /` body=1 KB (CL-known) | 200 + backend echo | `downstream_rq_2xx +1`, `downstream_rq_completed +1` |
| 2 | **Streaming overflow with CL-known body** (replaces §6.2 row 2 "CL fast-fail") | `POST /` body=2 MiB (CL-known) — driver sends `100 Continue` first; cap fires mid-stream | 100-Continue + 413 + `Payload Too Large` body + 4-header set + `Connection: close` | `downstream_rq_4xx +1`, `downstream_rq_too_large +1`, `downstream_rq_completed +1` |
| 3 | **Chunked overflow against per-route cap** | `POST /route-tighter` `Transfer-Encoding: chunked` body~=200 KiB (above 128 KiB override) | 413 + `Payload Too Large` (NO 100-Continue with chunked) | `downstream_rq_4xx +1`, `downstream_rq_too_large +1`, `downstream_rq_completed +1` |
| 4 | Per-route disabled bypasses cap | `POST /route-disabled` body=2 MiB (above listener 1 MiB) | 200 + backend echo | `downstream_rq_2xx +1`, `downstream_rq_completed +1` |
| 5 | Per-route tighter override fires | `POST /route-tighter` body=200 KiB (above 128 KiB override) | 100-Continue + 413 + `Payload Too Large` | `downstream_rq_4xx +1`, `downstream_rq_too_large +1`, `downstream_rq_completed +1` |

**Plus a 6th assertion (cross-cutting, not a new request):** `Content-Length` injection on chunked-passthrough. Driver issues a 6th request `POST /` `Transfer-Encoding: chunked` body=10 KB; backend asserts the inbound request carries `Content-Length: 10240` (NOT chunked encoding). This exercises the `maybeAddContentLength` mirror per §12.4 Decision 6 v2.

**Total fixture requests: 6 (was 5 in §6.2).** Counter equivalence: `downstream_rq_2xx +3`, `downstream_rq_4xx +3`, `downstream_rq_too_large +3`, `downstream_rq_completed +6`. No `buffer.*` counters asserted (none exist).

### 12.7 Re-framed §1.1 deliverable count + §7 ADR roster + §11 obligations

**§1.1 deliverables (was 11; now 10):**
- Items 1, 2, 3, 4, 5, 6, 8, 9, 10 unchanged (with 10 amended: stat-table delta from "29→31" to "29 stays at 29"; new subsection still authored documenting buffer's reuse of `downstream_rq_too_large`; forward-pointer subsection still authored).
- Item 7 (stat surface 29→31 extension) **retired** (§12.5).
- Item 11 (4 ADRs) becomes **item 10 prime: 3 ADRs** (§12.7 below).

**§7 ADR roster (was 4, now 3):**
- **ADR-0125** unchanged — package shape + boot registration + per-route disabled-OR-override 5th canonical shape (PGV mechanism amendment per §12.4 Decision 5 v2 surfaces here).
- **ADR-0126** unchanged — `compiledConfig` shape + 1-consumed/0-deferred + `max_request_bytes ≤ 1 MiB` parse-time validation + cap-layering rationale. (The cap-layering rationale tightens slightly: the ceiling now serves to keep buffer's filter cap inside envoy-go's framework safety net per ADR-0076; reference Envoy has no such layering.)
- **ADR-0127 v2** — Body-counting algorithm + `maybeAddContentLength` mirror + reuse of framework `SendLocalReply` 413 wire shape. (Decision 6 v2 + Decision 8 v2.) The Content-Length fast-fail clause from v1 is REMOVED. The 100-Continue addendum is recorded.
- **ADR-0128 RETIRED.** No new ADR; the slot is preserved for the next phase.

Next-free ADR after phase 13 phase-done: **ADR-0128** (was ADR-0129 under v1).

**§8 deferral list unchanged.** The 2 inline deferrals (8.1 `max_request_bytes > 1 MiB`; 8.2 `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes`) still apply with the same forward-pointer to the cap-promotion phase.

**§11 SPEC-author residual obligations.** Most §9 pins are RESOLVED in §12.3; the SPEC author re-runs each probe and records verbatim transcripts in SPEC §11 per ADR-0004 (no reduction in §11 verbosity), but no Decision-level questions remain open. The SPEC author MAY surface new questions only if the re-run scrape reveals divergence from §12.3's table. Anticipated wall-clock for §11 re-run: ~30-45 min (5 bootstrap YAMLs + 11 probes + transcripts). Anticipated SPEC.md size: ~1100-1300 lines (was hypothesized ~1500-1700; the smaller stat-table delta + simpler algorithm trim ~300 lines).

**§11 test scaffolding (was 18-25 unit tests across 6 groups in §11.1):**
- **G1 Factory parse** — same 6 cases.
- **G2 PerRoute parse** — same 6 cases, but the "both fields set" case now asserts `protojson.Unmarshal` returns oneof-violation error (not custom envoy-go validation); the "neither field set" case becomes a PGV-mirror rejection (not silent-no-op).
- **G3 DecodeHeaders** — restructured: drop the 3 `Content-Length`-specific cases; add a case asserting `StopIteration` is returned when bodied + not disabled (was hypothesized `Continue` in v1).
- **G4 DecodeData** — restructured: same 6 cases, but the "single-chunk overflows cap" case asserts the `downstream_rq_too_large` HCM counter increments (not a hypothetical `request_too_large` buffer.* counter). Add a case asserting `maybeAddContentLength` injects `Content-Length` on chunked-end-stream.
- **G5 Per-route integration** — same 5 cases.
- **G6 Stats** — restructured: assert `downstream_rq_too_large` increments (not `request_too_large`); drop the `request_buffered` cases entirely (counter does not exist); the disabled-bypass and header-only cases now assert ZERO increment of `downstream_rq_too_large` (which is meaningful since that counter exists for HCM-level reasons).

### 12.8 What §12 does NOT change

- §1.2-§1.4 (forward-pointers, phase-done framing, LoC envelope) unchanged. §1.4 LoC estimate trims slightly (~280-330 impl + ~400-500 tests + ~60 fuzzer + ~150-200 fixture-Go = ~890-1090 total; still well below ADR-0045's 1500 LoC trigger).
- §1.5-§1.6 (no-sibling-stub, no-prebrainstorm-branch) unchanged.
- §2.1-§2.4 (package layout, boot registration, MVP envelope, cap layering) unchanged.
- §3 (iteration protocol consequences) unchanged at the structural level; only DecodeHeaders status code differs (StopIteration now, was Continue) — already covered by §12.4.
- **§4 (no framework deltas) STILL HOLDS.** The amendment preserves zero framework primitives. Despite the algorithmic re-frame, no new HTTPFilterFactoryCtx field, HTTPRegistry method, PerRouteConfig accessor, or `setBufferLimit`-equivalent is added. The deliberate divergence from Envoy's `setBufferLimit + HCM-emits` model is recorded in ADR-0127 v2 §Consequences with an explicit forward-pointer to the future cap-promotion phase that may revisit this.
- §10 (ROADMAP delta) unchanged. Row 13 status stays `planned`. The amendment commit is docs-only; ROADMAP row text and §9 family heading are unchanged.

### 12.9 Next-session input (replaces the closer below)

The next session (lifecycle-state 1 → 2 for phase 13) authors `docs/envoy-go/phases/13-http-filter-buffer/SPEC.md` against §§1-12 of this BRAINSTORM. The §11 empirical-pin re-run is fast (most pins resolved by §12.3) but the verbatim transcripts MUST land in SPEC §11 per ADR-0004 and the phase 09/10/11/12 §11 discipline. The SPEC author authors 3 anticipated ADRs (ADR-0125, ADR-0126, ADR-0127 v2) — ADR-0128 is retired per §12.7. The §6 fixture matrix has 6 requests (5 originally hypothesized + 1 cross-cutting CL-injection assertion).

Header-of-this-doc reminder (the `Last-updated:` field updated to `2026-05-09 (§12 amendment)`): the original §1-§11 design sketch is preserved verbatim per D-3.5; §12 supersedes the affected portions for SPEC-drafting purposes.

---

## End of phase 13 brainstorm (with §12 amendment 2026-05-09)

Next-session input: this BRAINSTORM.md (§§1-12) + `BOOTSTRAP_PROMPT.md` §5 lifecycle-state-1 → 2 transition + `SKILL_ROUTING.md` + the 3 ADRs from §12.7 (ADR-0125 + ADR-0126 + ADR-0127 v2) anchored as anticipated. The next session authors `docs/envoy-go/phases/13-http-filter-buffer/SPEC.md` via `superpowers:writing-plans` (routed through SPEC-authoring step first per phase 09/10/11/12 precedent). The §9 empirical-pin obligations are LARGELY RESOLVED in §12.3; the SPEC author re-runs each probe per ADR-0004 and records verbatim transcripts in SPEC §11, but no Decision-level questions remain open absent re-run divergence from §12.3.
