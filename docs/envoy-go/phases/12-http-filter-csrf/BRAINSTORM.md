# Phase 12 Brainstorm — `envoy.filters.http.csrf`

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-0 → 1 brainstorm session for phase 12 (`http-filter-csrf`), the FIFTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, and `local_ratelimit` at phase 11). The next session (lifecycle-state 1 → 2 for phase 12, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10/11 precedent) authors `docs/envoy-go/phases/12-http-filter-csrf/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-12-http-filter-csrf-brainstorm`, branch `phase-12-http-filter-csrf-brainstorm`, branched from master tip `0f3a710` (the phase 11 phase-done REVIEW commit `phase 11: REVIEW — end-of-phase retrospective + N-1 carry-forward`). The phase 11 phase-done implementation commit `1de512d` and its SHA-fill follow-up `dfa08c9` precede the REVIEW; `0f3a710` is the REVIEW-landing commit.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + MVP scope envelope + each major design decision via 5-question dialogue (Q1 filter selection from §9 family-children list; Q2 MVP envelope shape — additional_origins-only consume + 3 stats; Q3 origin comparison algorithm — full-origin equality with normalization; Q4 request-evaluation gate — canonical 4-method set + Origin-then-Referer extraction + reject-when-undeterminable; Q5 differential fixture scenario set — 6 scenarios with Referer fallback retained). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0119), and the just-shipped phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/11-http-filter-local-ratelimit/BRAINSTORM.md` section-for-section, reframed for the csrf scope and adapted for its specific surface area (smallest §9 row to date; no async-resume; no stateful per-route resources; no new framework primitive; no proto `stat_prefix` field — reuses HCM-level stats anchor). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 12 — this brainstorm cold-started fresh from the §9 heading + the phase 11 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-07. Last-updated: 2026-05-07.

---

## 1. Mission and scope confirmation (12 only)

ROADMAP row `12 | http-filter-csrf | 11 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 12 is the FIFTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 11 phase-done commit `1de512d` (with follow-up `dfa08c9` for SHA fill, REVIEW at `0f3a710`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 58: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114). Phase 12 ships `csrf` as the FIFTH real filter — the canonical Envoy-style "same-origin enforcement" filter — and establishes the per-filter-phase pattern's fifth data point. It is also the SMALLEST §9 family-row to date by every dimension (LoC, task count, ADR count, fixture-scenario count, deferral surface).

### 1.1 What 12 delivers as a self-contained whole

Phase 12 lands `envoy.filters.http.csrf` (the canonical Envoy CSRF filter) under the 07.1 framework. Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):

1. **New `internal/filter/http/csrf/` package** owning the filter implementation. Package directory + Go package identifier are both `csrf` (single token; no underscore needed since the proto type-name is already a single token). Files mirror the `internal/filter/http/cors/` shape: `csrf.go` (filter type + factory + decode method + origin-parsing helpers + filterStats struct + runtimeConfig), `csrf_test.go` (unit tests), `fuzz_test.go` (the 16th fuzzer in the repo — `FuzzCsrfPolicyConfigParse`), `doc.go` (package overview + 1-consumed/2-deferred decomposition). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering `router.New`, `cors.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains a seventh `httpReg.Register(csrf.TypeURL, csrf.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`. Csrf inserts between `cors` and `envoy_go_test` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing** of `envoy.extensions.filters.http.csrf.v3.CsrfPolicy`, the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 3 top-level fields (the smallest §9 family proto surface to date). Phase 12 consumes 1 — `additional_origins[].StringMatcher.exact` (the StringMatcher's `exact` variant only with non-empty value; non-exact variants — `prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case` — are dropped at PARSE time, matching phase 09 fault `headers` discipline per ADR-0101 §3 verbatim: "only `HeaderMatcher_StringMatch` with non-empty `Exact` value is honored. All other variants … are silent-ignored at parse time"). The remaining 2 fields (`filter_enabled`, `shadow_enabled` — both `RuntimeFractionalPercent` couplings to the Runtime + hot restart family) are silently ignored at config-load time under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1; see §8 below).

4. **Method gate (Q4 = a₁).** The filter evaluates only modifying-method requests; non-modifying methods pass through with no counter increments and no origin extraction. Modifying-method set hardcoded as `{POST, PUT, DELETE, PATCH}`, case-sensitive match against the `:method` pseudo-header (HTTP/2) or method token (HTTP/1.1 — normalized upper-case by the HCM before filter dispatch). Methods outside this set — `GET`, `HEAD`, `OPTIONS`, `TRACE`, `CONNECT`, custom verbs — return `Continue` immediately from `DecodeHeaders` without inspecting any other request state. Empirical pin §11.P1 confirms exact set against Envoy v1.37.2.

5. **Source-origin extraction (Q4 = b).** For modifying methods only: read `Origin` header first; if present and parseable as origin (scheme + host + optional port) → that is the source origin; else read `Referer` header; if present and URL-parseable to scheme + host → derive origin from URL (`<scheme>://<host>[:<port>]`). If both missing or both unparseable → source origin is undeterminable. `Origin: null` and empty-string `Origin:` are empirical-pin-driven (§11.P2) — likely treated as parse-failure → fall through to Referer.

6. **Origin comparison algorithm (Q3 = α).** Both source and destination origins are normalized before comparison: scheme lowercased; host lowercased (DNS labels are case-insensitive per RFC 1035); port stripped if explicit-default-for-scheme (80 for http, 443 for https), else preserved numerically; canonical form `<scheme>://<host>[:<port>]` — no path, no query, no userinfo, no fragment. Source-origin matches destination-origin if and only if the normalized canonical forms are byte-equal. Source-origin matches `additional_origins` if any list entry's StringMatcher (exact variant only) matches the source-origin's full normalized canonical form (NOT bare host). The destination origin is constructed per request from: the `Host`/`:authority` header (parsed for host + optional port) + scheme inferred from listener TLS state (`https` if downstream connection is TLS-terminated, else `http`) + default port stripped per the same normalization rule. Empirical pin §11.P3 confirms scheme inference (likely from listener TLS state, NOT from `X-Forwarded-Proto`).

7. **Disposition table + rate-limit decision in `DecodeHeaders`.** The filter resolves the most-specific `runtimeConfig` via 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) — phase-07.1 primitive, reused as-is, no new framework primitive. The disposition is determined by the table:

    | Method | Source origin | Match destination | Match `additional_origins[].exact` | Disposition | Counter |
    |---|---|---|---|---|---|
    | ∉ modifying | — | — | — | `Continue` | (none) |
    | ∈ modifying | undeterminable (per Q4 = c₁) | — | — | `SendLocalReply(403)` + `StopIteration` | `missing_source_origin +1` |
    | ∈ modifying | == destination | yes | n/a | `Continue` | `request_valid +1` |
    | ∈ modifying | != destination | no | yes | `Continue` | `request_valid +1` |
    | ∈ modifying | != destination | no | no | `SendLocalReply(403)` + `StopIteration` | `request_invalid +1` |

    `additional_origins[]` (the runtime-visible slice — already filtered to surviving exact-with-non-empty-value entries at parse time per ADR-0101 §3 discipline) iterated in proto-declared order; first `exact` match wins. NO async-resume; NO encode-side state. `DecodeData` / `DecodeTrailers` / all `Encode*` are pass-through. csrf reuses fault.abort + local_ratelimit's request-side `StopIteration` + `SendLocalReply` primitives exactly — NO new framework primitive.

8. **Per-route bucket isolation as wholesale override (data-only).** Per the proto message `CsrfPolicy` itself (which serves as both listener-level and per-route-TPFC type — there is NO separate `CsrfPolicyPerRoute` wrapper; the proto file defines exactly one top-level message), each TPFC entry runs through `New` at config-load time; each `New` invocation allocates its own `runtimeConfig` with its own compiled `additional_origins` slice (an opaque list of normalized-origin strings, one per entry whose StringMatcher.exact is non-empty). The 3-tier resolver picks the most-specific config per request; that config's `additional_origins` is consulted (NOT the listener-level fallback). Listener-level config is NOT merged with per-route — wholesale-replacement per ADR-0073. UNLIKE phase 11 (first stateful per-route filter, ADR-0117 amendment to ADR-0073), phase 12's per-route is purely **data-only** (just a slice of strings) — NO stateful resources, NO atomic counters, NO mutex-protected runtime state. Phase 12 adds NO amendment to ADR-0073.

**Plus three artifact-level deliverables:**

9. **Differential fixture `0014-http-csrf`** under `test/fixtures/0014-http-csrf/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising six scenarios per Q5 = B+5 (per §6 below). The fixture asserts response status, body byte-exact (rejection body `Invalid origin`, 14 bytes), header set lowercase wire-form, counter deltas via `/stats/prometheus` scrape equivalence, and per-route-tier independent disposition. NO timing-sensitive scenarios (csrf is purely synchronous — no analog to phase 11's `refill-after-fill_interval ±10ms` scenario).

10. **`BEHAVIOR_CONTRACT.md` 4-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.fault` from phase 09, `### envoy.filters.http.header_mutation` from phase 10, and `### envoy.filters.http.local_ratelimit` from phase 11): a NEW `### envoy.filters.http.csrf` subsection covering the 1-consumed / 2-ignored field map, the rejection response wire shape (status 403, body bytes `Invalid origin`, 4-header set, framing), the per-route wholesale-override semantics, and the method-gate + origin-extraction algorithms. Plus the 26→29-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0014 with per-scenario tolerance discipline. Plus a NEW `### Phase 12 forward-pointer notes` subsection under `## Forward-pointer notes` covering the 3-item deferral list (per §8 below).

11. **Anticipated 5 ADRs (ADR-0120 through ADR-0124)** per §7 below. ADR-0119 is the highest-numbered ADR landed in phase 11; ADR-0120 is the next-free.

### 1.2 What 12 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 precedent; deferrals are 3 items grouped by family-coupling). The summary: percentage-based filter gating (`filter_enabled`), shadow-mode evaluation (`shadow_enabled`), and StringMatcher non-exact variants on `additional_origins` are all out-of-scope. None are blockers for closing row 12 phase-done; the percentage fields are silently ignored at config-load time (no warnings; faithful to the cors / fault / header_mutation / local_ratelimit deferral discipline); the StringMatcher non-exact variants are dropped at parse time per ADR-0101 §3 discipline (non-exact entries do not survive the `New` factory).

### 1.3 Phase-done as a §9 family-row landing

Phase 12's phase-done commit closes ROADMAP row `12` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 12 is the FIFTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, and 11-local_ratelimit). The next §9 family-row will be numbered `13` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 12 is **single-row at brainstorm time** — a cohesive ~250-450 LoC implementation slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split would be:

- **12.1 = listener-level filter MVP**: the filter type + factory + `DecodeHeaders` impl + origin-parsing helpers + 3-counter stats + rejection response + listener-level `runtimeConfig` parsing. Differential fixture covers listener-only scenarios (1, 2, 3, 4, 5).
- **12.2 = per-route TPFC**: per-route `CsrfPolicy` parsing (reusing the same proto type) + 3-tier resolver wiring + per-route-override fixture scenario (7).

This split mirrors phase 10 + phase 11's anticipated-but-unused split. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the modest LoC estimate (~250-450 impl + ~150-250 tests + ~40 fuzzer + ~150-250 fixture-Go-driver/backend + ~150 fixture-yaml/README = ~750-1100 total when including yaml configs and README; ~450 if counting Go code alone) and modest task count estimate (~10-14 tasks). Both estimates remain comfortably under ADR-0045's 1500 LoC / 25 task split-trigger upstream of either accounting. Phase 12 is structurally smaller than phase 11 because: (a) no token-bucket primitive; (b) no stateful per-route resources; (c) only 1 consumed proto field; (d) only 3 stats vs. 4; (e) only 6 fixture scenarios vs. 4 with timing sensitivity.

### 1.5 Seed-stub alignment

Like phases 09, 10, and 11, phase 12 has NO sibling SPEC stub — phase 12 enters fresh after the phase 11 close. The §9 family-children list at ROADMAP line 58 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 (which inherited an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch from a prior pivoted session), phase 12 has NO prior prebrainstorm-notes artefacts. This brainstorm cold-started fresh from the §9 heading + the phase 11 just-shipped artefacts per ADR-0106(e).

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

This section is the brainstorm's decision log. Each Decision states **what** is chosen, **why** that option vs. its alternatives, what **deferred-pin** obligations (if any) remain for SPEC-time empirical work, and what **ADR anchor** the SPEC author should expect. ADR numbering starts at **ADR-0120** (next-free; phase 11 closed at ADR-0119 per `DECISIONS.md`).

### 2.1 Filter package layout *(Decision 1 → ADR-0120)*

**Decision:** New package `internal/filter/http/csrf/` (directory + Go package identifier both `csrf`, single token — matches the cors/fault precedent; no underscore needed since the proto type-name is already a single token) with files mirroring the cors + fault + header_mutation + local_ratelimit precedent: `csrf.go`, `csrf_test.go`, `fuzz_test.go`, `doc.go`. The package exports two top-level symbols: `TypeURL` (string constant, `"type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"`) and `New` (the `HTTPFilterFactory`). All other types (`filter`, `runtimeConfig`, `compiledOrigin`, `filterStats`) are unexported. NO filename underscores needed (csrf is a single token, unlike `header_mutation` and `local_rate_limit` which had underscored proto type-names). Filename is `csrf.go` (mirrors `cors.go`).

**Why this vs. alternatives:**
- *Why directory `csrf/` (single token)?* The proto type-name is `CsrfPolicy` — a single token. The directory naming convention across §9 family-rows is now: cors (single token), fault (single token), header_mutation (underscore-preserving for the multi-token proto type), localratelimit (no-underscore for the multi-token proto type — explicit departure per ADR-0114), csrf (single token, matches cors/fault). The single-token cases are unambiguous; the multi-token cases have flexibility (header_mutation chose underscore-preserving; localratelimit chose no-underscore; ADR-0114 captured the rationale). For csrf, the single-token nature removes the choice — it's just `csrf`.
- *Why not a single `internal/filter/http/csrf.go` flat file?* The existing per-filter discipline is unanimous (cors, fault, router, envoygotest, header_mutation, localratelimit each get their own subpackage). Subpackage isolation prevents future name collisions and is the project's convention.
- *Why not the Envoy-source-style path `internal/extensions/filters/http/csrf/`?* envoy-go is explicitly NOT mirroring Envoy's C++ source structure (`MISSION.md` §2.2 non-purpose). The `internal/filter/http/<name>/` pattern is the project's own convention.

**Deferred to SPEC:** the exact file split between `csrf.go` and any helper files (e.g., whether to factor origin-parsing into its own file `origin.go`) — the SPEC author chooses based on test readability. No ADR-class commitment from brainstorm.

**ADR anchor:** ADR-0120 — Filter package shape conformance with cors/fault/header_mutation/localratelimit precedent + boot registration ordering.

### 2.2 Extension-registry registration *(Decision 2 → ADR-0120 consequence)*

**Decision:** `cmd/envoy-go/main.go` adds a single new line `httpReg.Register(csrf.TypeURL, csrf.New)` between the existing `cors` registration and the `envoygotest` registration. The registration ordering is alphabetical-after-router per the ADR-0100 §2.2 convention codified at phase-09 brainstorm time: `router (first) → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit`. Per ADR-0072, registration ordering does not affect runtime behavior; this is a stylistic discipline only. Phase 12 introduces NO `RegisterPerRouteValidator` hook (unlike phase 10's `header_mutation`) — per-route configs are independently valid (no multi-tier protected-set discipline like header_mutation's; each `runtimeConfig` validates standalone via the same `New` path).

**Why this vs. alternatives:**
- *Why not registration-order = config-list-order?* Registration order is a global discipline; config-list order is per-listener / per-route. Decoupling avoids cross-cutting coupling (already settled at phase-09 brainstorm time).
- *Why no per-route validator hook?* Phase 10's hook was driven by the multi-tier protected-header eager-validation requirement (which only surfaces when multiple tiers' configs interact). Phase 12's per-route configs are wholesale-override (Decision 6 below); each validates standalone at `New` time. No multi-tier interaction means no eager-validation hook.

**Deferred to SPEC:** none — the line edit is mechanical.

**ADR anchor:** ADR-0120 (consequence; no separate ADR for registration).

### 2.3 MVP envelope: 1-consumed/2-deferred field decomposition *(Decision 3 → ADR-0121)*

**Decision (per Q2 = Option C):** Phase 12 consumes 1 of 3 proto top-level fields:

- **`additional_origins`** (`[]StringMatcher`, repeated) — **CONSUMED** with StringMatcher.exact variant only (with non-empty value). Other StringMatcher variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) are dropped at PARSE time per ADR-0101 §3 verbatim discipline ("only `HeaderMatcher_StringMatch` with non-empty `Exact` value is honored. All other variants … are silent-ignored at parse time"). Non-exact entries do NOT survive the `New` factory; they are filtered out before `runtimeConfig` is constructed. This is architecturally cleaner than match-time-fail (no dead data carried into runtime) and matches the established phase 09 precedent verbatim.

The remaining 2 fields are silently ignored at config-load time (no warnings):

- **`filter_enabled`** (`RuntimeFractionalPercent`) — **DEFERRED** to Runtime + hot restart family. Envoy default 100% when unset (per Envoy v1.37.2 docstring: "This field defaults to 100/HUNDRED"). Divergence-window: differential fixture configs leave the field unset on both sides (both sides default to 100%-active); users who explicitly set `default_value < 100%` will see Envoy gate by percentage, envoy-go always-100%.
- **`shadow_enabled`** (`RuntimeFractionalPercent`) — **DEFERRED** to Runtime + hot restart family. Envoy default 0% when unset (filter is in enforcement mode, not shadow). Differential fixture leaves unset on both sides. The `shadow_request_invalid` counter is dropped from MVP stat surface (it's structurally tied to non-zero `shadow_enabled` rolls).

**Why this envelope vs. alternatives:**
- *Why consume only 1 of 3 fields?* Consuming `additional_origins` is the meaningful surface for differential testing — it's the only field that influences per-request disposition. The two `RuntimeFractionalPercent` fields couple to runtime-key handling (Runtime + hot restart family, currently unscheduled). Without runtime-key support, percentage gating without runtime-key tuning is rarely useful for a security filter that admins want to tune live; the operational value of partially consuming `filter_enabled` is low. Consuming `additional_origins` only matches phase 11's exact discipline (local_ratelimit silently-ignored both `filter_enabled` and `filter_enforced` percentage fields, behaving as always-100%-enabled).
- *Why drop `shadow_request_invalid` counter?* The counter is structurally tied to the partial-rollout case (it only increments when `filter_enabled` is OFF and `shadow_enabled` is ON — both deferred). Emitting it as a permanently-zero counter for parity (analogous to phase 09's `fault.response_rl_injected` per ADR-0107) is an option but adds noise without operational signal. The clean choice for csrf MVP is to omit the counter entirely; SPEC §11.P6 confirms via empirical scrape whether reference Envoy emits it under the deferred-rollup configuration.

**Deferred to SPEC:** the precise wire shape of the silent-ignore for the two percentage fields (config-load-time reads them, runtime ignores them — same shape as phase 11's `filter_enabled`/`filter_enforced`).

**ADR anchor:** ADR-0121 — `runtimeConfig` shape + 1-consumed/2-deferred decomposition + StringMatcher non-exact variants dropped at PARSE time per ADR-0101 §3 discipline + Runtime-family coupling for both percentage fields + drop `shadow_request_invalid` from MVP stat surface.

### 2.4 Origin extraction + comparison algorithm *(Decision 4 → ADR-0122)*

**Decision (per Q3 = α + Q4 = b):** The filter performs **full-origin equality comparison** with the following normalization rules and source-extraction order:

**Source-origin extraction:**
1. Read `Origin` request header. If present and parseable as origin (regex-equivalent: `^<scheme>://<host>(:<port>)?$`) → that is the source origin.
2. Else read `Referer` request header. If present and URL-parseable (via `net/url.Parse`) and the URL has scheme + host → derive origin from URL (`<scheme>://<host>[:<port>]`). That is the source origin.
3. Else (both missing or both unparseable) → source origin is undeterminable.

**Destination-origin construction:**
- Host: from `:authority` pseudo-header (HTTP/2) or `Host` header (HTTP/1.1), parsed as `host[:port]`.
- Scheme: inferred from listener TLS state — `https` if downstream connection is TLS-terminated, else `http`. Empirical pin §11.P3 confirms (Envoy may also consult `X-Forwarded-Proto` — needs scrape evidence).
- Port: from the host string if present; else default for scheme.

**Normalization rules (applied to both source and destination):**
- Scheme lowercased.
- Host lowercased (DNS labels are case-insensitive per RFC 1035).
- Port: if explicit and equal to default-for-scheme (80 for http, 443 for https) → drop; else preserve numerically.
- Result: canonical form `<scheme>://<host>[:<port>]` — no path, no query, no userinfo, no fragment.

**Comparison:**
- Source-origin matches destination-origin if and only if the normalized canonical forms are byte-equal (case-folded scheme + host; port-equal modulo default).
- Source-origin matches `additional_origins` if any surviving list entry's StringMatcher.exact value (treated as a normalized canonical-origin-string at config-load time) byte-equals the normalized source-origin. Non-exact StringMatcher variants and empty-value exact entries are dropped at parse time per ADR-0101 §3 discipline; the runtime only sees the surviving entries.

**Why full-origin (Q3 = α) vs. host-only (β) vs. defer-to-empirical (γ):**
- (α) **CHOSEN.** Envoy's csrf filter is documented as a same-origin enforcement filter; same-origin in browser-security is full-triple per RFC 6454. Full-origin is the only flavor where the StringMatcher input format is unambiguous (the full normalized-origin string).
- (β) host-only is permissive — `https://api.example.com:8443` and `http://api.example.com` both match each other under host-only, defeating the security intent.
- (γ) defer-to-empirical leaves a load-bearing semantic question to SPEC time, which is unusual for this project — the project's discipline is BRAINSTORM commits, SPEC empirically confirms or amends. Other phases (09, 10, 11) all committed to algorithm shape at brainstorm time and used SPEC §11 only for surface-level empirical confirmations (header sets, body bytes, status codes).

**Why Origin-then-Referer fallback (Q4 = b):**
- Origin is the modern same-origin signal (introduced for CSRF defense; security-only); Referer is the legacy fallback (privacy concerns, often stripped). Origin-first matches all major CSRF defense guidance (OWASP, MDN, RFC 6454).
- Both missing/unparseable → undeterminable (per disposition table; rejected per Q4 = c₁).

**Deferred to SPEC (empirical pins §11.P2, §11.P3, §11.P7, §11.P8):**
- §11.P2: `Origin: null` and empty-string `Origin:` disposition (parse-failure → fall through, OR treated as undeterminable directly).
- §11.P3: scheme-from-listener-TLS-state vs. scheme-from-`X-Forwarded-Proto`.
- §11.P7: case-folding + default-port-stripping + trailing-slash normalization details.
- §11.P8: `additional_origins` matching target — full triple vs. bare host (BRAINSTORM hypothesizes full triple per α; SPEC confirms).

**ADR anchor:** ADR-0122 — Origin extraction (Origin → Referer fallback; parse-failure → undeterminable) + full-origin equality comparison + normalization rules (lowercase scheme/host; strip default port; canonical form `<scheme>://<host>[:<port>]`) + scheme-from-listener-TLS-state + `additional_origins[].exact` matched against full normalized origin string.

### 2.5 Method gate *(Decision 5 → ADR-0122)*

**Decision (per Q4 = a₁):** The modifying-method set is hardcoded as `{POST, PUT, DELETE, PATCH}`, case-sensitive match against the `:method` pseudo-header (HTTP/2) or method token (HTTP/1.1, normalized upper-case by HCM before filter dispatch). Methods outside this set return `Continue` immediately from `DecodeHeaders` without inspecting any other request state — no Origin/Referer parsing, no counter increments, no stats touched.

**Why this set vs. alternatives:**
- *Why hardcoded set vs. evaluate-non-safe-method?* The Fetch spec's CORS-preflight modifying-method set is exactly `{POST, PUT, DELETE, PATCH}`. Broader sets (e.g., "any method except GET/HEAD/OPTIONS/TRACE") would diverge from reference Envoy on uncommon methods (CONNECT, PROPFIND, custom verbs). Empirical pin §11.P1 confirms Envoy's exact set.
- *Why exclude TRACE?* TRACE has no body and is read-only by HTTP/1.1 §4.3.8 / §4.3.6; it's never CSRF-relevant.

**Deferred to SPEC:** §11.P1 — confirm Envoy's exact method set against scrape evidence.

**ADR anchor:** ADR-0122 (consequence; method gate is part of the algorithm captured by ADR-0122).

### 2.6 Per-route TPFC discipline *(Decision 6 → reuses ADR-0073, no amendment)*

**Decision:** Per-route `typed_per_filter_config` for csrf carries a `*CsrfPolicy` value (the same proto type as listener-level — there is NO separate `CsrfPolicyPerRoute` wrapper; the proto file defines exactly one top-level message). Each TPFC entry is parsed via `New` at config-load time → produces a fresh `*runtimeConfig` with its own compiled `additional_origins` slice (an opaque list of normalized-origin strings). The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific config per request; that config is the wholesale override.

**Per-route is data-only.** UNLIKE phase 11 (first stateful per-route filter, ADR-0117 amendment to ADR-0073 — per-route entries each owned a `tokenBucket`), phase 12's per-route entries hold ONLY a slice of normalized-origin strings. NO stateful resources, NO atomic counters, NO mutex-protected runtime state. Phase 12 adds NO amendment to ADR-0073.

**ADR-0073 has been amended TWICE prior to phase 12** — once by ADR-0110 (phase 10 multi-tier `ResolveAllTiers` accessor for filters whose proto semantics demand cross-tier evaluation rather than most-specific-override), and once by ADR-0117 (phase 11 stateful per-route bucket carry, codifying that wholesale-override extends to stateful resources without further framework support). Phase 12 inherits the wholesale-override discipline as established by both amendments — uses the existing 3-tier `Resolve` (NOT `ResolveAllTiers`, since csrf is most-specific-override per the cors/fault precedent, NOT multi-tier) and treats per-route data as wholesale-replacement (matching ADR-0117's general "wholesale-replacement extends across all data shapes" reading). Phase 12 adds NO third amendment.

**Why this vs. alternatives:**
- *Why same proto type (no per-route wrapper)?* The `CsrfPolicy` proto file defines exactly ONE top-level message. The Envoy-style design is that the same `CsrfPolicy` serves both listener-level and per-route-TPFC purposes. envoy-go follows the proto.
- *Why wholesale-override vs. field-level merge?* ADR-0073 establishes wholesale-override as the cors/fault/local_ratelimit precedent. Phase 12 inherits that precedent without amendment. Field-level merge for `additional_origins` (e.g., union the listener-level list with the per-route list) is plausible but diverges from Envoy and from the established envoy-go discipline. Wholesale-override is the canonical interpretation.

**Deferred to SPEC:** §11.P9 — confirm Envoy's per-route TPFC discipline via scrape evidence (listener `[A]`, per-route `[B]`; confirm route requests with origin `A` are REJECTED on the route — listener fallback NOT consulted).

**ADR anchor:** none new (reuses ADR-0073 verbatim; no amendment).

### 2.7 Stat surface — 26→29-name extension *(Decision 7 → ADR-0124)*

**Decision:** 3 new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 26-name table` (extending to 29-name table):

| Stat name | Type | Increments when |
|---|---|---|
| `http.<HCM stat_prefix>.csrf.request_valid` | counter | modifying request whose source origin matches destination or `additional_origins[].exact` |
| `http.<HCM stat_prefix>.csrf.request_invalid` | counter | modifying request whose source origin is determinable but matches neither destination nor any `additional_origins[].exact` |
| `http.<HCM stat_prefix>.csrf.missing_source_origin` | counter | modifying request whose source origin is undeterminable (both Origin + Referer missing/unparseable) |

**Namespace anchor:** csrf has NO `stat_prefix` proto field (the proto has only 3 fields, none of them stat-related). Stats anchor at the HCM's stat_prefix (the same root used by HCM-level stats like `downstream_rq_total`). NO new flattening rule needed — phase 11's SN9 was specific to filters with their own proto `stat_prefix`; csrf reuses the existing HCM-stat_prefix tag-extraction. Specifically: the existing `envoy_http_conn_manager_prefix` Prometheus tag covers it; no new tag-extractor required. Phase 12 does NOT extend ADR-0061's flattening-rule set (Rules SN1-SN8 introduced by phase 06.1, Rule SN9 introduced by phase 11).

**Twin-series filter discipline (per BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline`):** csrf emits ONE series per stat_prefix per counter — no twin-series (no per-cluster fan-out, no per-route emission). All 3 counters are flat under the HCM stat_prefix. NO permanently-zero counter (unlike phase 09's `fault.response_rl_injected` which is emitted permanently-zero for parity per ADR-0107). NO `enabled` analog (unlike phase 11's `local_ratelimit.enabled` which counts every evaluated request) — csrf's gate is the method check, not a percentage roll; `request_valid` counts all modifying-method passthroughs (the analog of local_ratelimit's `ok`); there's no separate "evaluated all modifying requests" counter because that's structurally equal to `request_valid + request_invalid + missing_source_origin`.

**Why drop `shadow_request_invalid`:**
- The counter increments only when `filter_enabled` is OFF and `shadow_enabled` is ON (per Envoy's CsrfPolicy proto docstring). Both deferred per Decision 3.
- Emitting it as a permanently-zero counter (analogous to phase 09 fault.response_rl_injected per ADR-0107) is an option but adds noise without operational signal. The cleaner MVP choice is to omit it entirely.
- §11.P6 empirical pin confirms whether reference Envoy emits the counter under the all-defaults config; if YES, envoy-go either matches (emit permanently-zero) or documents the divergence (don't emit; document at BEHAVIOR_CONTRACT.md `### envoy.filters.http.csrf` subsection). The divergence path is the BRAINSTORM-preferred default.

**Deferred to SPEC (empirical pins §11.P6):**
- §11.P6: confirm Prometheus form against Envoy v1.37.2 scrape. Hypothesized form: `envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="<HCM stat_prefix>"} <count>` (and analogous for the other two). SPEC §11 confirms exact metric name + label set verbatim.

**ADR anchor:** ADR-0124 — `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension for 3 `csrf.*` counters + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag) + drop `shadow_request_invalid` from MVP stat surface (couples to deferred shadow mode).

### 2.8 Rejection wire shape *(Decision 8 → ADR-0123)*

**Decision (per fault/local_ratelimit precedent):** On rejection (either `request_invalid` or `missing_source_origin` disposition):

- **Mechanism:** `cb.SendLocalReply(403, body, headers)` followed by `return StopIteration` from `DecodeHeaders`. Same primitive fault.abort + local_ratelimit use. NO async-resume; NO encode-side state.
- **Status:** `403 Forbidden` (NOT configurable via proto; the canonical csrf rejection status — empirical pin §11.P4 confirms).
- **Body:** byte-exact `Invalid origin` (14 bytes, NO trailing newline). Empirical pin §11.P5 verifies bytes verbatim against Envoy v1.37.2.
- **Header set on the wire** (lowercase wire-form, 4 headers): `content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`. NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked`, NO `charset=UTF-8` modifier on the content-type. (Note: `server: envoy` matches phase 11's correction of the BRAINSTORM-hypothesized `envoy-go` — the wire form is reference Envoy's, NOT envoy-go's identity.)

`DecodeData` / `DecodeTrailers` / all `Encode*` methods are pass-through. csrf is request-side-only.

**Why this vs. alternatives:**
- *Why 403 vs. configurable?* Envoy's CsrfPolicy proto has no status field; the rejection status is hardcoded at 403 in v1.37.2. envoy-go matches.
- *Why `Invalid origin` body (14 bytes)?* Empirical hypothesis based on Envoy CSRF filter docs + source. SPEC §11.P5 confirms verbatim.
- *Why 4-header set lowercase wire-form?* Matches the established fault.abort + local_ratelimit precedent (4 headers, lowercase, no chunked/cache-control extras). Empirical pin §11.P10 confirms verbatim.

**Deferred to SPEC (empirical pins §11.P4, §11.P5, §11.P10):** confirm status, body bytes, header set verbatim against Envoy v1.37.2 scrape.

**ADR anchor:** ADR-0123 — Rejection path wire shape — `SendLocalReply(403)` + body byte-exact `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + `StopIteration` from `DecodeHeaders` — reuses fault.abort/local_ratelimit primitive.

---

## 3. Iteration protocol consequences

Phase 12 csrf interacts with the 07.1 HTTP filter framework as a **purely-synchronous, request-side-only filter**:

- **`DecodeHeaders`**: full implementation. Reads `:method`, `Origin`, `Referer`, `Host`/`:authority`. Returns `Continue` (passthrough) or `StopIteration` after `SendLocalReply` (rejection).
- **`DecodeData`**: pass-through (`return Continue`). csrf does not inspect request bodies — there's nothing for it to check on data frames since the disposition is already settled in headers.
- **`DecodeTrailers`**: pass-through.
- **`EncodeHeaders` / `EncodeData` / `EncodeTrailers`**: pass-through. csrf does not modify response state.
- **`OnDestroy`**: no-op. csrf has no per-request resources to clean up (no timers, no goroutines, no atomic counters to balance).

**No new framework primitive.** Phase 12 reuses the existing `SendLocalReply` + `StopIteration` machinery from phase 09 fault (request-side terminal-replace). Phase 12 reuses the existing 3-tier `PerRouteConfig.Resolve` from phase 07.1. Phase 12 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor.

**No async-resume.** UNLIKE phase 09 fault (`time.AfterFunc` + parkDecode wake-up), csrf's `DecodeHeaders` runs synchronously to completion in a single dispatch. The disposition is computed inline; `Continue` or `StopIteration` is returned without parking.

**No stateful per-route resources.** UNLIKE phase 11 local_ratelimit (per-route `tokenBucket`), csrf's per-route runtimeConfig is purely data — a slice of normalized-origin strings. Each `New` invocation allocates a fresh slice; the slice is read-only at runtime; no mutex, no atomic, no synchronization.

**Listener-level lifecycle.** csrf listener-level filter instance is a singleton per listener — created once at boot, used across all requests routed through that listener. No per-request allocation overhead (the runtimeConfig pointer is shared; the disposition computation is stateless).

**Filter-instance vs. runtimeConfig separation.** Per the cors/fault/header_mutation/local_ratelimit precedent, the `filter` struct holds `*runtimeConfig` (pointer to the resolved config — listener-level OR most-specific per-route after `Resolve`); the `runtimeConfig` struct holds the compiled `additional_origins` slice + `filterStats` pointer. The factory `New` constructs the `runtimeConfig`; the `DecodeHeaders` callback resolves the per-request config (via `Resolve`) and computes the disposition.

---

## 4. Framework deltas — none anticipated

Phase 12 introduces ZERO new framework primitives. Specifically:

- NO new `HTTPFilterFactoryCtx` field (phase 09 added one; phase 12 does NOT).
- NO new `*HTTPRegistry` method (phase 11 added `NewCounterIfAbsent` to `stats.Registry`; phase 12 reuses as-is for the 3 counters).
- NO new `PerRouteConfig` accessor (phase 10 added `ResolveAllTiers`; phase 12 uses the existing 3-tier `Resolve`).
- NO new `RegisterPerRouteValidator` hook (phase 10 added the eager-validation hook; phase 12 does NOT — per-route configs validate standalone via `New`).
- NO amendment to ADR-0073 (phase 10 added an amendment for multi-tier evaluation via ADR-0110; phase 11 added an amendment for stateful per-route via ADR-0117; phase 12 does NOT add a third — per-route is data-only AND most-specific-override).
- NO new `internal/stats/name.go` SN flattening rule (phase 11 added Rule SN9; phase 12 does NOT — stats anchor at HCM stat_prefix, reusing existing `envoy_http_conn_manager_prefix` tag-extraction).

This makes phase 12 the structurally-thinnest §9 family-row to date. The phase touches only:
- `internal/filter/http/csrf/` (new package, 4 files).
- `cmd/envoy-go/main.go` (1-line registration insert).
- `test/fixtures/0014-http-csrf/` (new fixture, ~5 files).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4-edit bundle: new subsection + 26→29 stat-table extension + new equivalence-matrix row + new forward-pointer subsection).
- `docs/envoy-go/DECISIONS.md` (ADR-0120 through ADR-0124, append-only).
- `docs/envoy-go/ROADMAP.md` (row 12 status flips planned → in-progress → done).
- `docs/envoy-go/STATE.md` (active-phase pointer).
- `docs/envoy-go/phases/12-http-filter-csrf/` (BRAINSTORM.md authored at this commit; SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md authored over phase lifecycle).

---

## 5. Stats — see §2.7

(See Decision 7 in §2.7 above. No additional content here — kept as a section anchor for symmetry with phase 09/10/11 BRAINSTORM structures where stats are §5.)

---

## 6. Differential fixture (`0014-http-csrf`)

### 6.1 Topology

`test/fixtures/0014-http-csrf/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config (initially identical; any divergence per ADR-0007 is documented in `README.md`).
- `inputs/driver.go` — Go driver that drives both proxies with identical inputs.
- `expectations.yaml` — per-scenario allow-list / ignore-list / stats-name mapping / timing tolerances.
- `README.md` — fixture overview + scenario list + reference config citations.

Single listener `127.0.0.1:<port>` (HTTP/1.1 plaintext per phases 09/10/11 precedent — H2 differential testing of csrf is deferred). One virtual_host `vh_main` with two routes:
- `/` (default route) — uses listener-level CsrfPolicy.
- `/route-only` — uses per-route TPFC override.

One cluster `c0` reaching the host-side echo backend at `test/helpers/echobackend/` (the same backend used by phases 09/10/11).

**Listener-level `CsrfPolicy`:**
```yaml
additional_origins:
  - exact: "https://app.example.test"
```

**Per-route TPFC on `/route-only`:**
```yaml
additional_origins:
  - exact: "https://route-only.test"
```

`filter_enabled` and `shadow_enabled` initial-fixture state is **conditional on §11.P11 empirical pin outcome**: BRAINSTORM hypothesizes both unset on both sides (Envoy convention per docstring: `filter_enabled` defaults to 100%-active, `shadow_enabled` defaults to 0%-shadow; envoy-go silent-ignores both fields and is effectively always-100%-active never-shadow regardless). However, phase 11's analogous local_ratelimit pin overturned the docstring-trust hypothesis at SPEC §11.2/§11.4 — the actual runtime default was 0%-OFF — forcing fixture configs to set explicit 100% on BOTH sides. SPEC author resolves §11.P11 in-session against Envoy v1.37.2 BEFORE finalizing fixture 0014 configs; if the pin reveals the same docstring-trust trap, both `envoy.yaml` and `envoy-go.yaml` must set `filter_enabled: {default_value: {numerator: 100, denominator: HUNDRED}}` explicitly (envoy-go side is mechanical since the field is silent-ignored runtime-side; Envoy side is required for differential equivalence).

### 6.2 6 scenarios (per Q5 = B+5)

| # | Scenario | Request | Expected response | Counter delta |
|---|---|---|---|---|
| 1 | Same-origin POST allowed | `POST /` `Origin: http://127.0.0.1:<port>` | 200 + backend body passthrough | `request_valid +1` |
| 2 | Cross-origin POST rejected | `POST /` `Origin: https://evil.test` | 403 + `Invalid origin` body + 4-header set | `request_invalid +1` |
| 3 | `additional_origins` exact-match allowed | `POST /` `Origin: https://app.example.test` | 200 + backend body | `request_valid +1` |
| 4 | No source-origin rejected | `POST /` (no `Origin`, no `Referer`) | 403 + `Invalid origin` + 4-header set | `missing_source_origin +1` |
| 5 | Referer fallback allowed | `POST /` `Referer: http://127.0.0.1:<port>/somepage` (no `Origin`) | 200 + backend body | `request_valid +1` |
| 7 | Per-route wholesale override | (a) `POST /route-only` `Origin: https://route-only.test` → 200; (b) `POST /` `Origin: https://route-only.test` → 403 (matches neither listener-default nor `app.example.test`) | mixed: 200 + 403 in one scenario | `request_valid +1`, `request_invalid +1` |

**Note on numbering.** Scenario 6 (GET passthrough) is intentionally absent from the differential fixture — covered by unit tests in `csrf_test.go` instead (see §11 Test scaffolding). The numbering is preserved so that mapping to the brainstorm dialogue Q5 is unambiguous.

### 6.3 Asserted equivalence

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status**: byte-equal between Envoy and envoy-go for every scenario (200 on passthrough; 403 on rejection).
- **Response body** on rejection: byte-equal `Invalid origin` (14 bytes, no trailing newline) for scenarios 2, 4, 7b. On passthrough scenarios (1, 3, 5, 7a), the body is the backend echo response — set-equal modulo timing/identity headers.
- **Response header set**: lowercase wire-form, set-equal between Envoy and envoy-go modulo the existing `## Header allow-list` (for `date`, `server`, timing/identity headers). On rejection: 4-header set (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`); allow-list-modulo only on `date`.
- **Per-counter delta equality**: after the workload completes, scrape `/stats/prometheus` from both proxies and assert per-counter deltas:
    - `csrf.request_valid`: Envoy `+4` vs. envoy-go `+4` (scenarios 1, 3, 5, 7a).
    - `csrf.request_invalid`: Envoy `+2` vs. envoy-go `+2` (scenarios 2, 7b).
    - `csrf.missing_source_origin`: Envoy `+1` vs. envoy-go `+1` (scenario 4).
- **Per-route TPFC bucket independence**: scenario 7 demonstrates the listener-level config NOT consulted on `/route-only`, and per-route config NOT consulted on `/`.

### 6.4 Driver shape

Go driver in `inputs/driver.go` per phase 09/10/11 precedent — sequential request loop (race-tolerant scrape ordering); per-scenario assertions inline; final stats scrape via `/stats/prometheus`. Total: 7 requests in the workload (scenarios 1, 2, 3, 4, 5, 7a, 7b — scenario 7 contributes 2 requests). Estimated driver size: ~150-200 LoC.

**No timing tolerances.** UNLIKE phase 11 fixture 0013 which had a `refill-after-fill_interval ±10ms` scenario, phase 12 fixture 0014 has NO timing-sensitive scenarios — csrf is purely synchronous. Each scenario's response is dispatched within microseconds of the request reaching `DecodeHeaders`; no timer, no async-resume.

**No H2 differential coverage.** Phase 12 fixture 0014 is HTTP/1.1-only. H2 differential testing of csrf is deferred (matching the phase 09/10/11 precedent — each filter ships with H1 differential coverage; H2 differential coverage is deferred to a future bundle).

---

## 7. Anticipated ADRs (ADR-0120 through ADR-0124)

5 ADRs anticipated. ADR-0119 is the highest-numbered ADR landed in phase 11; ADR-0120 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0120** | `internal/filter/http/csrf/` package shape — single-token directory matching cors precedent + boot registration ordering (`router → cors → csrf → ...`) + `TypeURL` constant + `New` factory + 4-file split | Decision 1 (§2.1) + Decision 2 (§2.2) |
| **ADR-0121** | `runtimeConfig` shape + 1-consumed/2-deferred field decomposition (`additional_origins[].StringMatcher.exact` non-empty value consumed; `filter_enabled` + `shadow_enabled` silent-ignored under Runtime + hot restart family deferral) + StringMatcher non-exact variants dropped at PARSE time per ADR-0101 §3 discipline (NOT match-time-keep-and-fail) | Decision 3 (§2.3) |
| **ADR-0122** | Origin extraction (Origin → Referer fallback) + comparison algorithm (full-origin equality per Q3 = α) + method gate (canonical 4-method set per Q4 = a₁) + normalization rules (lowercase scheme/host; strip default port; canonical `<scheme>://<host>[:<port>]`) + scheme inference from listener TLS state + `additional_origins[].exact` matched against full normalized origin string | Decision 4 (§2.4) + Decision 5 (§2.5) |
| **ADR-0123** | Rejection path wire shape — `SendLocalReply(403)` + body byte-exact `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + `StopIteration` from `DecodeHeaders` — reuses fault.abort/local_ratelimit primitive | Decision 8 (§2.8) |
| **ADR-0124** | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension for 3 `csrf.*` counters + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag) + drop `shadow_request_invalid` from MVP stat surface (couples to deferred shadow mode) | Decision 7 (§2.7) |

NO omnibus ADR for deferrals (phase 11 dropped this pattern at SPEC §8.1; deferrals inline in BEHAVIOR_CONTRACT.md `### Phase 12 forward-pointer notes` per §8 below). NO amendment to ADR-0073 (per-route is data-only — no stateful resource carry; Decision 6 §2.6). NO amendment to ADR-0061 (no new SN flattening rule; Decision 7 §2.7). NO amendment to ADR-0040 (silent-ignore set extension is mechanical, captured in ADR-0121).

---

## 8. Deferral list

3 deferral items, organized inline (no omnibus ADR per phase 11 SPEC §8.1 precedent). All three land in `BEHAVIOR_CONTRACT.md ### Phase 12 forward-pointer notes` (a NEW subsection under `## Forward-pointer notes`, sibling to the existing `### Phase 11 forward-pointer notes`).

### 8.1 `filter_enabled` (RuntimeFractionalPercent)

**Coupled to:** Runtime + hot restart family.
**Envoy default per docstring:** 100% when unset (per Envoy v1.37.2 csrf.pb.go:33-41 docstring: "This field defaults to 100/HUNDRED").
**Envoy actual runtime default:** EMPIRICAL-PIN OPEN — see §11.P11. Phase 11's analogous `local_ratelimit.filter_enabled` pin (SPEC §11.2/§11.4) overturned the docstring-trust hypothesis (actual runtime default was 0%-OFF), forcing fixture configs to set explicit 100% on both sides. Phase 12 SPEC author resolves the same question for csrf in-session before finalizing fixture 0014.
**envoy-go behavior:** silent-ignored at config-load time; runtime is effectively always-100%-active regardless of proto value.
**Divergence-window:** differential fixture 0014 fixture-config decision is conditional on §11.P11 outcome (see §6.1). Users who explicitly set `default_value < 100%` will see Envoy gate by percentage, envoy-go always-100%. Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.csrf` subsection + `### Phase 12 forward-pointer notes`.
**Future re-activation:** when Runtime + hot restart family lands, the field becomes operational; the divergence-window closes.

### 8.2 `shadow_enabled` (RuntimeFractionalPercent)

**Coupled to:** Runtime + hot restart family.
**Envoy default per docstring:** 0% when unset; further per csrf.pb.go:43-50, the field is "intended to be used when filter_enabled is off and will be ignored otherwise" — so its observable runtime effect is conditional on `filter_enabled`'s state.
**Envoy actual runtime default:** EMPIRICAL-PIN OPEN — see §11.P11 (same trap-detection scope covers `shadow_enabled`).
**envoy-go behavior:** silent-ignored at config-load time; runtime is effectively never-shadow regardless of proto value.
**Divergence-window:** differential fixture 0014 fixture-config decision is conditional on §11.P11 outcome. Users who explicitly set `default_value > 0%` while also having `filter_enabled` off will see Envoy enter shadow mode (evaluate-but-don't-enforce), envoy-go always-enforce. Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.csrf` subsection + `### Phase 12 forward-pointer notes`.
**Stat coupling:** the `shadow_request_invalid` counter is dropped from MVP stat surface (per Decision 7 §2.7). When shadow mode lands, the counter is added back; the 29-name table extends to 30 names.
**Future re-activation:** when Runtime + hot restart family lands, the field becomes operational; the divergence-window closes; the counter is added back.

### 8.3 `additional_origins[].StringMatcher` non-exact variants

**Coupled to:** whatever future phase lands the full StringMatcher engine (TBD; not currently a §9 family heading).
**Variants deferred:** `prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`. The `exact` variant (non-empty value) is consumed.
**envoy-go behavior:** PARSE-TIME drop. Non-exact entries do NOT survive the `New` factory — they are filtered out before `runtimeConfig` is constructed; the runtime never sees them. Empty-value `exact` entries are also dropped (mirroring ADR-0101's "non-empty Exact value is honored" qualifier). The list as a whole still functions normally — only the surviving exact-entries enter the match loop.
**Discipline:** mirrors phase 09 fault `headers` field verbatim (StringMatcher non-exact variants dropped at parse time per ADR-0101 §3). NOT match-time-keep-and-fail.
**Future re-activation:** when the full StringMatcher engine lands, non-exact variants become operational at parse time; existing fixtures continue to work since they only use `exact`. Re-activation does NOT change the architectural shape (parse-time-decision rather than runtime-decision); it just expands the set of variants that survive parse.

---

## 9. Empirical pins for SPEC §11

The SPEC author scrapes reference Envoy v1.37.2 in-session per ADR-0004 and confirms each pin verbatim before authoring SPEC §11. If any empirical pin diverges from BRAINSTORM hypothesis, SPEC §11 records the divergence + the SPEC author decides whether to adopt Envoy's behavior (likely) or to file an ADR for deliberate divergence (rare). Phase 11 hit one such divergence at SPEC §11.5 (Prometheus tag-extractor name `envoy_local_http_ratelimit_prefix` — corrected the BRAINSTORM-hypothesized `envoy_http_conn_manager_prefix`); phase 12 is structurally less likely to hit such divergences because the proto surface is so much smaller, and structurally most pins below are confirmation rather than discovery.

| ID | Subject | Hypothesis (BRAINSTORM-committed) | Empirical confirmation needed |
|---|---|---|---|
| §11.P1 | Method gate | `{POST, PUT, DELETE, PATCH}` exact set | Verify Envoy v1.37.2 source/scrape: which methods trigger eval. Confirm `GET`, `HEAD`, `OPTIONS`, `TRACE` are passthrough; confirm uncommon methods (`CONNECT`, `PROPFIND`, custom verbs) are passthrough. |
| §11.P2 | Origin parse-failure cases | `Origin: null` and empty-`Origin: ` fall through to Referer | Scrape: send `POST /` with `Origin: null` (no Referer) and observe response — does Envoy treat `null` as missing → fallback or as parse-failure → reject? Same for empty Origin. |
| §11.P3 | Destination scheme inference | TLS-state of downstream connection sets scheme; `X-Forwarded-Proto` NOT consulted | Scrape: drive `POST /` over plaintext listener with `Origin: https://127.0.0.1:<port>` (mismatched scheme) — does Envoy reject? Confirm scheme comes from listener-state, not request headers. Also: drive with `X-Forwarded-Proto: https` set explicitly, Origin matching https; does Envoy accept? |
| §11.P4 | Rejection status code | `403 Forbidden` hardcoded | Scrape: confirm exact status line (`HTTP/1.1 403 Forbidden`). |
| §11.P5 | Rejection body bytes | `Invalid origin` (14 bytes, no trailing newline) | Scrape: byte-count + `xxd`-equivalent dump of the response body. Confirm zero trailing whitespace/newline. |
| §11.P6 | Stats Prometheus form | `envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="<HCM stat_prefix>"} <count>` and analogous; `shadow_request_invalid` may or may not be emitted under all-defaults config | Scrape `/stats/prometheus` after a defined load; confirm exact metric name + label set + tag-extraction. Also confirm whether `shadow_request_invalid` is emitted permanently-zero under the default `shadow_enabled=0%` config. |
| §11.P7 | Origin normalization | scheme/host lowercased; default-port stripped; trailing slash N/A | Scrape: `Origin: HTTPS://APP.EXAMPLE.TEST:443` (uppercase, default port) vs config `additional_origins: [exact: "https://app.example.test"]` — does Envoy match? Also: what about explicit non-default port `https://app.example.test:8443`? |
| §11.P8 | `additional_origins` matching target | full-origin string `<scheme>://<host>[:<port>]` | Scrape: confirm StringMatcher receives full triple, not bare host. Drive `Origin: http://app.example.test` against config `additional_origins: [exact: "https://app.example.test"]` — does Envoy reject (scheme differs)? |
| §11.P9 | Per-route TPFC wholesale-override | listener `additional_origins` NOT merged with per-route | Scrape: listener `[A]`, per-route `[B]`; confirm route requests with origin `A` are REJECTED (not allowed by listener fallback). |
| §11.P10 | Header set on rejection | 4-header lowercase wire-form (`content-length`, `content-type`, `date`, `server: envoy`); `content-length: 14`; `content-type: text/plain` (no `charset=UTF-8` modifier) | Scrape: full header dump on a 403 csrf rejection; confirm absence of `cache-control`, `x-content-type-options`, `transfer-encoding: chunked`, and absence of `charset=UTF-8` modifier. |
| §11.P11 | `filter_enabled` runtime default with field UNSET | csrf proto docstring claims `filter_enabled` defaults to `100%/HUNDRED` when unset → filter evaluates every modifying request | **Highest-priority pin (precedent: phase 11 SPEC §11.2/§11.4 found `local_ratelimit.filter_enabled` defaults to 0%-OFF at runtime despite suggestive docstring, forcing fixture configs to set explicit 100% on BOTH sides).** Scrape reference Envoy v1.37.2 with NO `filter_enabled` field set: drive a cross-origin `POST /` with `Origin: https://evil.test` against listener-level CsrfPolicy that has `additional_origins: [exact: "https://app.example.test"]` (no `filter_enabled` field at all). Does Envoy reject (filter active per docstring) or allow (filter inactive — same trap as phase 11)? **Fixture decision is conditional on this pin's outcome:** if filter active when unset → fixture leaves `filter_enabled` unset on both sides per BRAINSTORM hypothesis (§6.1, §8.1); if filter inactive when unset → fixture must set explicit `filter_enabled: {default_value: {numerator: 100, denominator: HUNDRED}}` on the Envoy side (envoy-go silent-ignores either way; behavior is always-100%). Same trap-detection scope applies to `shadow_enabled`: per csrf.pb.go:43-50 the `shadow_enabled` field is "ignored when filter_enabled is on," so its runtime default behavior under all-defaults config is also empirically open. |

If the SPEC author finds any divergence from these hypotheses, SPEC §11 records the divergence verbatim + reconciles with the BRAINSTORM Decision (most likely by amending the Decision in SPEC; the BRAINSTORM is not edited post-landing per D-3.5 + D-3.4).

---

## 10. ROADMAP delta

### 10.1 New row added by this brainstorm

This brainstorm appends one new row to `docs/envoy-go/ROADMAP.md` under the MVP Trunk table:

```
| 12 | http-filter-csrf | 11 | planned | | <summary post-SPEC> |
```

Status starts at `planned`. The next session (lifecycle-state 1 → 2; skill `superpowers:writing-plans` per ADR-0005) flips to `in-progress` upon authoring `phases/12-http-filter-csrf/SPEC.md`. The phase-done commit flips to `done` per the lifecycle-state-6 transition (BOOTSTRAP_PROMPT.md §5).

### 10.2 §9 family heading at ROADMAP line 56 stays unchanged

Per ADR-0106(c), the §9 family heading at ROADMAP line 56 (`### HTTP filters family`) is a conceptual umbrella, not a row. Phase 12's landing does NOT modify the heading's text or position. The phase-done commit message body explicitly states: (1) ROADMAP row 12 flips planned → in-progress → done; (2) the §9 family heading at ROADMAP line 56 stays unchanged; (3) phase 12 is the FIFTH §9 family-row to land.

### 10.3 No-sibling-stub discipline (per ADR-0106(b))

This brainstorm does NOT pre-populate stub rows for the other §9 family-children (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 11. Test scaffolding

### 11.1 Unit tests (`csrf_test.go`)

Likely 12-18 test functions covering the algorithm + factory + per-route surface:

- `TestNew_*` — factory: valid policy, invalid Any (proto unmarshal failure), `additional_origins` with non-exact StringMatcher (entry-skip — test the entry is parsed but never matches at runtime).
- `TestDecodeHeaders_NonModifyingMethods` — parametrized over `GET, HEAD, OPTIONS, TRACE` (and at least one custom verb like `CONNECT` or `PROPFIND`); all return `Continue` immediately, no counter increments, no origin parsing invoked. **This subsumes scenario 6 from the Q5 dialogue.**
- `TestDecodeHeaders_SameOrigin` — `Origin` matches `Host` (HTTP plaintext listener variant; HTTPS TLS listener variant — verifies scheme inference).
- `TestDecodeHeaders_CrossOrigin` — `Origin` differs from `Host`; rejected with 403 + `Invalid origin` + 4-header set.
- `TestDecodeHeaders_AdditionalOriginsExactMatch` — `additional_origins` allows otherwise-cross-origin request.
- `TestDecodeHeaders_RefererFallback` — no `Origin`, `Referer` parses to matching origin.
- `TestDecodeHeaders_RefererFallback_Unparseable` — `Referer` is malformed → fallthrough → reject as missing.
- `TestDecodeHeaders_OriginNullValue` — `Origin: null` (per §11.P2 outcome).
- `TestDecodeHeaders_OriginEmptyValue` — `Origin: ` empty string.
- `TestDecodeHeaders_OriginNormalization` — uppercase scheme + default port + uppercase host all normalize correctly.
- `TestDecodeHeaders_DestinationSchemeFromTLS` — TLS listener → `https`; plaintext listener → `http`.
- `TestDecodeHeaders_PerRouteOverride` — per-route TPFC produces independent disposition (listener config NOT consulted on per-route hit).
- `TestRuntimeConfig_StringMatcherNonExact` — non-exact entries are dropped at parse time (do NOT survive `New`); `runtimeConfig` carries only the surviving exact entries (per ADR-0101 §3 discipline).
- `TestStats_Counters` — `request_valid` / `request_invalid` / `missing_source_origin` increment per the disposition table.

### 11.2 Fuzzer (`fuzz_test.go`)

`FuzzCsrfPolicyConfigParse` — the **16th fuzzer** in the repo. Fuzz target: random bytes → `protojson.Unmarshal` into `*CsrfPolicy` → `New(ctx, cfg)`; assert no panic. Optional secondary corpus: pre-seeded `additional_origins` with malformed StringMatcher.regex patterns (those dropped at parse time per ADR-0101 §3 discipline; should not panic during the parse-time filter step).

The 15 prior fuzzers (per phase 11 STATE.md): trivially listed by `find . -name 'fuzz_test.go'` — phases 06.1 through 11 each landed at least one. Phase 12 adds one.

### 11.3 No new conformance suite

csrf is HTTP-filter-only; no h2spec/h3spec implications. The phase 12 phase-done gate runs the existing h2spec at the ADR-0051 pin (53/53 PASS) — no new conformance addition.

### 11.4 Race-test discipline

All unit tests + the existing differential suite + the new fixture 0014 must pass under `go test -race ./...` per the phase-done gate (BOOTSTRAP_PROMPT.md §7.5(e)). csrf has no shared mutable state at the filter-instance level (the `runtimeConfig` is read-only after `New`; no per-request synchronization needed) — race-test cleanness is structurally guaranteed; no special discipline required.

---

## End of phase 12 brainstorm

Next-session input: this BRAINSTORM.md + `BOOTSTRAP_PROMPT.md` §5 lifecycle-state-1 → 2 transition + `SKILL_ROUTING.md` + the 5 ADRs from §7 (ADR-0120 through ADR-0124) anchored as anticipated. The next session authors `docs/envoy-go/phases/12-http-filter-csrf/SPEC.md` via `superpowers:writing-plans` (routed through SPEC-authoring step first per phase 09/10/11 precedent), executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.
