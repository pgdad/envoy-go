# Phase 12 SPEC — `envoy.filters.http.csrf`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 12 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 499 lines; commits `7fd9213` + `ba58c7e` + `399532c` + `c2e7559` + `bb29bb0` on branch `phase-12-http-filter-csrf-brainstorm`). NO off-master prebrainstorm-notes branch (UNLIKE phase 11; phase 12 cold-started fresh from the §9 heading + the just-shipped phase 11 artefacts per ADR-0106(e)).

**ADR continuity:** Phase 11 closed at ADR-0119. Phase 12 anticipated ADR-0120..ADR-0124 (5 ADRs per BRAINSTORM §7) — refined to **5** ADRs at this SPEC; the empirical-pin amendments do NOT add a sixth ADR (they reshape the existing 5; see §1.1 below).

---

## 1. Purpose

Phase 12 lands `envoy.filters.http.csrf` — Envoy's canonical same-origin enforcement filter — as the FIFTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), and local_ratelimit (11), and the FIFTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 12 is the SMALLEST §9 family-row to date by every dimension (LoC, task count, ADR count, fixture-scenario count, deferral surface). The five new architectural primitives:

1. A new `internal/filter/http/csrf/` package owning the filter implementation. Directory + Go-package identifier are both `csrf` (single token; matches the cors/fault precedent — no underscore needed since the proto type-name is already a single token). Files mirror the cors/fault precedent: `csrf.go` (filter type + factory + decode method + origin-parsing helpers + `runtimeConfig` + `filterStats`), `csrf_test.go` (unit tests across 6 test groups per §6.5), `doc.go` (package overview + 1-consumed/1-PGV-validated-not-honored/1-deferred decomposition; see §1.1 amendment 3 for the field-count revision), `fuzz_test.go` (`FuzzCsrfPolicyConfigParse` per §14.3 — the 16th fuzzer in the repo). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`runtimeConfig`, `compiledOrigin`, `filterStats`, `filter`) are unexported. See ADR-0120.

2. **Origin extraction + comparison algorithm — HOST:PORT-ONLY equality.** Per §11.3 / §11.7 / §11.8 empirical findings, the csrf filter compares `hostAndPort(source_origin)` against `hostAndPort(target_origin)` where `target_origin` is constructed from `<scheme>://<:authority>` and `hostAndPort()` strips the scheme prefix. **Scheme is NEVER compared** — it is computed only to make the URL parseable. The `additional_origins[].StringMatcher.exact` value is matched against the source's `host[:port]` form (NOT the full `<scheme>://<host>[:<port>]` URL). Source-origin extraction follows a 3-way disposition (per §11.2 trichotomy): (a) `Origin: null` literal short-circuits to `EMPTY_STRING` → `missing_source_origin` counter (NO Referer fallback); (b) `Origin:` empty value or `Origin:` header absent → fall back to Referer (`hostAndPort(Referer)` is used as source_origin); (c) `Origin:` non-empty, non-`null` that fails URL parsing → the verbatim raw string is used as source_origin (NO Referer fallback) — almost always rejects since the verbatim string mismatches `hostAndPort(target)`. NO normalization (no case folding; no default-port stripping; trailing slash IS stripped because the URL parser drops the path component). Method gate is `{POST, PUT, DELETE, PATCH}` (case-sensitive uppercase string match against `:method`); other methods short-circuit to `Continue` BEFORE any counter increment or origin parsing. Per §11.1 / §11.2 / §11.3 / §11.7 / §11.8 empirical confirmations + amendments. See ADR-0122.

3. **Rate-decision discipline in `DecodeHeaders`.** The filter resolves the most-specific `runtimeConfig` via 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) — the existing 07.1 framework primitive per `internal/filter/http/perroute.go:103–128` (per ADR-0073). On modifying-method requests: extracts source origin per §1.1 amendment 2's trichotomy; constructs target origin's `hostAndPort`; looks for byte-equal match against target OR any `additional_origins[]` exact entry; on match → increments `request_valid`, returns `Continue`; on mismatch with non-empty source → increments `request_invalid`, calls `cb.SendLocalReply(403, "Invalid origin", headers)`, returns `StopIteration`; on empty source → increments `missing_source_origin`, calls `cb.SendLocalReply(403, "Invalid origin", headers)`, returns `StopIteration`. Non-modifying methods short-circuit to `Continue` before any counter touch. NO async-resume primitive (unlike phase 09's `delay`). NO encode-side state. NO stateful per-route resources (unlike phase 11's `tokenBucket`). `DecodeData` / `DecodeTrailers` / all `Encode*` are pass-through. Reuses fault.abort + local_ratelimit's request-side `SendLocalReply` + `StopIteration` primitives exactly — NO new framework primitive. See ADR-0123.

4. **Per-route override as data-only wholesale-replacement.** Per the proto message `CsrfPolicy` itself (which serves as both listener-level and per-route-TPFC type — the proto file defines exactly ONE top-level message, no separate `CsrfPolicyPerRoute` wrapper), each TPFC entry runs through `New` at config-load time, allocating its own `*runtimeConfig` with its own compiled `additional_origins` slice (an opaque list of normalized-origin strings). The 3-tier resolver picks the most-specific config per request; that config's `additional_origins` is consulted (NOT the listener-level fallback). Per-route is **purely data-only** — no stateful resources, no atomic counters, no mutex-protected state. **Per-route stats are SHARED with listener-level** (per §11.9 amendment, divergence from phase 11's local_ratelimit precedent): per-route `runtimeConfig` carries only the `*compiledOrigin` slice; the `*filterStats` pointer is the listener-level (HCM-scoped) one — there is exactly ONE counter series per HCM scope regardless of how many per-route TPFC entries exist. This is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern; phase 11's "wholesale stateful override + independent stats" pattern (ADR-0117) is the precedent for stateful per-route filters, but phase 12 is data-only and stat-sharing. Phase 12 adds NO amendment to ADR-0073 (the wholesale-override discipline applies as-is; stats are simply not part of the override). Confirmed empirically at §11.9.

5. **Stat surface 26→29-name extension.** Three new counters under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`: `http.<HCM stat_prefix>.csrf.{missing_source_origin, request_invalid, request_valid}` (text format) / `envoy_http_csrf_{missing_source_origin, request_invalid, request_valid}{envoy_http_conn_manager_prefix="<HCM stat_prefix>"}` (Prometheus format). All three wired through `internal/stats.Registry` via per-instance `*atomic.Int64` slots in `filterStats`. The 26-name table (extended from 17→22 by phase 09; from 22→26 by phase 11) grows to 29 names. **Reuses existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor** (per §11.6 — UNLIKE phase 11 which introduced filter-specific `envoy_local_http_ratelimit_prefix` / Rule SN9, phase 12 introduces NO new SN flattening rule and NO new tag-extractor; csrf reuses the existing HCM-namespace SN2 rule). NO `shadow_request_invalid` counter — confirmed empirically at §11.6 that Envoy emits the same 3-counter family in shadow-only mode (no separate `*_shadow` family). `shadow_enabled` is structurally tied to deferred Runtime + hot restart family and produces no observable counter under the all-defaults envoy-go MVP. See ADR-0124.

After phase 12, the project has proven the §9 HTTP filters family-expansion pattern carries through a FIFTH filter under: the cors precedent's package-shape discipline (single-token directory matching the proto type-name); the fault precedent's `runtimeConfig` parser pattern; the local_ratelimit precedent's per-route-wholesale-override discipline (extended here to data-only-with-shared-stats); the existing HCM stat-prefix tag-extraction (no new SN rule); and the ADR-0101 §3 parse-time-drop StringMatcher discipline applied to the `additional_origins` repeated-StringMatcher field. *envoy-go's HTTP filter framework hosts a synchronous, request-side-only origin-enforcement filter with no framework extension; the existing fault `SendLocalReply` + `StopIteration` mechanism carries through verbatim for the rejection path; the stat surface extends from 26 to 29 names with shared-with-listener per-route stat semantics; the `host:port` only comparison discipline (no scheme, no normalization) is mirrored verbatim from upstream Envoy v1.37.2; the parse-time-drop discipline for `additional_origins[].StringMatcher` non-exact variants matches phase 09 fault `headers` discipline per ADR-0101 §3 verbatim; all under flat top-level row expansion (per ADR-0106).* This is the FIFTH §9 family-row to land; subsequent filters (compression, jwt_authn, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session AMENDS BRAINSTORM design decisions in **four** load-bearing places, plus carries forward **three** confirmations:

- **§11.3 + §11.7 + §11.8 (origin comparison shape) — MAJOR REVISION (Decision 4 / ADR-0122):** BRAINSTORM Decision 4 hypothesized "full-origin equality comparison" with normalization (lowercase scheme/host; strip default port; canonical form `<scheme>://<host>[:<port>]`) and `additional_origins[].exact` matched against the full normalized origin string. The empirical pins prove this is **WRONG**:

    1. **Scheme is NOT part of the equality.** Reference Envoy v1.37.2's `csrf_filter.cc targetOriginValue` constructs `<scheme>://<:authority>` only to make the URL parseable; `hostAndPort()` then strips the scheme prefix. Source-side `hostAndPort()` does the same. The byte-equality is between two `host[:port]` strings — scheme NEVER enters the check. `X-Forwarded-Proto` is irrelevant (confirmed §11.3 probes C/D). BRAINSTORM hypothesis (scheme-from-listener-TLS-state) is technically correct as to where `:scheme` originates but irrelevant to the equality.
    2. **NO case normalization.** `https://APP.EXAMPLE.TEST` does NOT match `app.example.test` (§11.7 probe A2/A3). The `Http::Utility::Url::initialize` parser preserves authority case verbatim.
    3. **NO default-port stripping.** `https://app.example.test:443` (default port for https) does NOT match `app.example.test` (no port). To support implicit-default-port equivalence, the operator must explicitly add BOTH port-suffixed and bare entries (§11.7 probe A4 + supplementary `p7b-norm.yaml`).
    4. **Trailing slash IS stripped** (path component dropped) — `https://app.example.test/` → `app.example.test` (§11.7 probe A7). This is via the URL parser's `hostAndPort()` extraction, NOT a separate normalization step.
    5. **`additional_origins[].exact` is matched against `hostAndPort(source)` — host[:port] form, NOT the full URL with scheme** (§11.8 probes A/B/C/D). An entry like `exact: "https://app.example.test"` will NEVER match a real `Origin:` header. **OPERATOR FOOTGUN** — SPEC §6.4 + BEHAVIOR_CONTRACT §13.1 must call this out unmistakably.

    envoy-go MVP MUST mirror verbatim: the filter's match function receives the source's `host[:port]` (no scheme prefix) and compares against the listener's compiled list (also `host[:port]` form, populated from each `additional_origins[].exact` value verbatim — NO config-load-time normalization). Fixture 0014 configs MUST use `host[:port]` form for `additional_origins[].exact` values; full-URL forms would never match. ADR-0122 records the corrected algorithm + the operator-footgun caveat.

- **§11.2 (origin parse-failure trichotomy) — MAJOR REVISION (Decision 4 / ADR-0122):** BRAINSTORM Decision 4 hypothesized 2 cases: `Origin` parses → use it; else fall back to Referer. The empirical pin proves Envoy's `sourceOriginValue()` has THREE distinct branches:

    1. `Origin: null` (literal 4-char string `"null"`) → short-circuits to `EMPTY_STRING` (NO Referer fallback). Source-origin is empty → counter increments `missing_source_origin` → reject. (§11.2 probes A, I.)
    2. `Origin:` value yields empty `hostAndPort()` (i.e., absent header, empty header value, or value that the URL parser produces an empty host for) → fall back to Referer's `hostAndPort()`. (§11.2 probes B/F, G.)
    3. `Origin:` non-empty, non-`null`, non-empty-`hostAndPort` BUT URL parse fails → return the verbatim raw string. The verbatim string then mismatches `hostAndPort(target)` and `additional_origins[].exact` entries (unless an entry happens to equal that exact verbatim string). NO Referer fallback is consulted. (§11.2 probes C/H, J/K, plus §11.3 probe G.)

    envoy-go MUST replicate three distinct behaviors. ADR-0122 captures the trichotomy. SPEC §6.4 + BEHAVIOR_CONTRACT §13.1 enumerate all three branches.

- **§11.11 (`filter_enabled` is PGV-REQUIRED; no UNSET trap exists) — MAJOR REVISION (Decision 3 / ADR-0121):** BRAINSTORM Decision 3 hypothesized that envoy-go silent-ignores `filter_enabled` at config-load time (matching phase 11 local_ratelimit's discipline of silent-ignoring `filter_enabled` + `filter_enforced`). The empirical pin proves this is **structurally impossible** at the differential boundary:

    1. **`filter_enabled` is REQUIRED at PGV.** Reference Envoy rejects boot with the verbatim error `goo.gle/debugonly: ... CsrfPolicyValidationError.FilterEnabled: value is required` if the field is omitted (§11.11 probe #1).
    2. **`filter_enabled.default_value` (the inner `RuntimeFractionalPercent.default_value`) is also REQUIRED at PGV.** Boot rejects with `RuntimeFractionalPercentValidationError.DefaultValue: value is required` if the inner default_value is omitted (§11.11 probe #2).
    3. `shadow_enabled` IS optional at parse time (§11.11 probe #3 boots successfully with `shadow_enabled` absent).
    4. With `filter_enabled.default_value=0%`: filter SHORT-CIRCUITS at `decodeHeaders` entry — all 3 counters stay at 0 (§11.11 probe #3 wire trace + Prometheus output).
    5. With `filter_enabled=0%, shadow_enabled=100%`: stats increment (request_valid / request_invalid / missing_source_origin) but NO 403 is emitted (shadow-only). The same 3-counter family is used; NO separate `*_shadow` family (§11.11 probe #4).

    The "UNSET trap" anticipated in BRAINSTORM (the phase 11 analog of "docstring claims 100%-active when unset; runtime actually defaults to 0%-OFF") **does NOT apply** to csrf — the user CANNOT omit `filter_enabled`; PGV rejects at boot. Consequently, envoy-go MVP for phase 12 must:

    - **Validate at parse-time** that the proto's `filter_enabled` field is non-nil AND its inner `default_value` is non-nil. Boot fails if either is missing, mirroring Envoy's PGV behavior (this is a NEW filter-internal validation discipline; the proto messages do not carry runtime-validating `protoreflect` plumbing in envoy-go's existing protobuf-runtime model). Verbatim error mirroring Envoy's PGV envelope is OUT of scope for MVP — envoy-go emits its own boot-failure error message stating the field requirement (PLAN author resolves the exact wording per phase 11's ADR-0115 filter-internal-validation precedent).
    - **Silent-ignore the actual percentage value at runtime** (always treat as 100%-active regardless of `default_value.numerator`). This is the same discipline phase 11 settled on for `local_ratelimit.filter_enabled` — runtime-key + percentage-roll handling couples to the deferred Runtime + hot restart family.
    - **`shadow_enabled` is silent-ignored at parse-time** (does not require non-nil at parse since Envoy accepts omission). Runtime is always-100%-enforce (never-shadow).
    - **Fixture 0014 configs MUST set `filter_enabled.default_value: { numerator: 100, denominator: HUNDRED }` explicitly on BOTH reference Envoy and envoy-go sides.** Reference Envoy requires it at boot (PGV); envoy-go validates the field's presence at parse time (mirroring Envoy's PGV); the runtime behavior is byte-equivalent because both sides see effective-100%. `shadow_enabled` is omitted on both sides for the differential gate (§11.11 probe #3 confirms always-100% behavior with `shadow_enabled` absent).

    ADR-0121 records the corrected envelope: 1 field actively consumed (`additional_origins`), 1 field PGV-validated-but-silent-ignored-at-runtime (`filter_enabled`), 1 field optional + silent-ignored (`shadow_enabled`).

- **§11.9 (per-route stats are SHARED with listener-level) — MINOR REVISION (Decision 7 / ADR-0124):** BRAINSTORM Decision 7 was AMBIGUOUS on per-route stat semantics; the §9.P9 empirical pin was scoped to "wholesale-override yes/no" but did not pin counter-series independence. The actual finding is sharper: **per-route TPFC carries `CsrfPolicy` only — there is NO independent `*filterStats` per per-route entry**. All counter increments (across listener-level dispatches AND per-route dispatches) anchor at the SAME `*csrfStats` instance — registered once per HCM scope at filter-chain build time, keyed by the HCM-level `stat_prefix`. This **diverges from phase 11's local_ratelimit precedent** (where each per-route TPFC entry carried its own `stat_prefix` + independent `*filterStats` series) and is structurally simpler. envoy-go MUST mirror: per-route `runtimeConfig` holds the `[]compiledOrigin` slice (and the resolved `filter_enabled` "validated, silent-ignored" marker) — but its `*filterStats` pointer is the listener-level (HCM-scoped) one. ADR-0124 captures this; it adds NO ADR-0073 amendment paragraph (unlike ADR-0117 which carried the stateful-per-route discipline).

Three additional findings carry FORWARD into design but do NOT amend BRAINSTORM decisions:

- **§11.6 (no `shadow_request_invalid` counter — CONFIRMED):** BRAINSTORM Decision 7 dropped `shadow_request_invalid` from MVP stat surface; the empirical pin confirms reference Envoy v1.37.2 does NOT emit it under all-defaults config either (only the 3 standard counters). Shadow-only mode reuses the regular 3-counter family — the same `request_invalid` increments whether the filter enforces or shadows. envoy-go's stat-name extension stays at 3 counters; the 26→29-name table extension is correct.

- **§11.6 (Prometheus tag-extractor reuse — CONFIRMED):** BRAINSTORM Decision 7 hypothesized csrf reuses the existing `envoy_http_conn_manager_prefix` extractor (no new filter-specific extractor like phase 11's `envoy_local_http_ratelimit_prefix`). Empirically confirmed. envoy-go's `internal/admin/stats.go` requires NO new tag-extractor pattern. Phase 12 introduces NO new SN flattening rule (Rules SN1-SN8 from ADR-0061, plus Rule SN9 from ADR-0118 are unchanged; phase 12 adds nothing).

- **§11.10 (rejection wire shape — CONFIRMED):** BRAINSTORM Decision 8 hypothesized 4-header lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`); status 403; body `Invalid origin` (14 bytes, no LF); NO chunked, NO `cache-control`, NO `x-content-type-options`, NO `charset=UTF-8` modifier. Empirically confirmed verbatim against reference Envoy. ADR-0123 records the wire shape unchanged from BRAINSTORM hypothesis.

### 1.2 Revised scope summary (post-§1.1 amendments)

After the §1.1 amendments, phase 12's in-scope architectural primitives are the FIVE listed at the head of §1, expressed as 8 BRAINSTORM-§1.1-style line items (BRAINSTORM's 8 in-scope items stay at 8; the §11 amendments reshape the algorithms within Decision 4 + Decision 3 + Decision 7 but do NOT add or remove primitives). Differential fixture has 6 scenarios per §7.1 (per BRAINSTORM §6.2, scenario numbering preserved 1-5 + 7 with scenario 6 unit-only). Stat surface is THREE names (26→29 table extension). ADR list is **5** (ADR-0120..ADR-0124); the §11 amendments reshape ADR-0121 + ADR-0122 + ADR-0124 substantively but do not add a sixth ADR. ADR-0073 amendment paragraph is NOT added (per-route is data-only with shared stats — no stateful resource carry; no new framework primitive).

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 9 + ADR-0106)

Phase 12 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, and stays unchanged in state across phase 12's landing. Phase 12 is the FIFTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11). Each subsequent HTTP filters family member (compression, jwt_authn, rbac, …) becomes its own top-level row at row 13, 14, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

### 1.4 ADR-0045 split-by-surface readiness

Phase 12 stays a SINGLE row at this SPEC. The implementation surface is estimated at:

- ~250–400 LoC production filter (`csrf.go` per BRAINSTORM §3; substantially smaller than phase 11's `local_ratelimit.go` because no token-bucket primitive, no monotonic-time arithmetic, no LBP-1-adjacent concurrency declaration)
- ~25 LoC `doc.go`
- ~150–200 LoC unit tests (6 test groups per §6.5)
- ~40 LoC fuzzer
- ~20 LoC framework deltas (zero new primitives; one new `httpReg.Register` line in `cmd/envoy-go/main.go`)
- ~250–350 LoC fixture (envoy.yaml ~70 + envoy-go.yaml ~70 + driver/main.go ~150 + backend/main.go ~30 + expectations.yaml ~30 + README.md ~50; total approximate)
- ~50 LoC ROADMAP+STATE+BEHAVIOR_CONTRACT additions at SPEC commit (this SPEC does not modify production code)

Total: ~750–1100 LoC across all bundles, with ~500 in Go code. Task count estimate per the BRAINSTORM §1.4: ~10–14 tasks. Both metrics stay well below ADR-0045's 1500-LoC / 25-task split-trigger thresholds. The PLAN author retains the ADR-0045 release valve if PLAN finds the surface exceeds either threshold; the natural split per BRAINSTORM §1.4 is `12.1 = listener-level filter MVP` and `12.2 = per-route TPFC + per-route fixture scenario`. **SPEC's position: single-row.**

### 1.5 No prebrainstorm-notes branch

UNLIKE phase 11 (which inherited an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch from a prior pivoted session), phase 12 has NO prior prebrainstorm-notes artefacts. The phase 12 BRAINSTORM cold-started fresh from the §9 heading + the phase 11 just-shipped artefacts per ADR-0106(e). THIS SPEC consults BRAINSTORM as authoritative + the §11 empirical-pin block (this SPEC drafting session) as the divergence-from-BRAINSTORM record. No off-master branch needs to be merged or referenced.

### 1.6 Phase 12 is the first §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time

Phases 09 / 10 / 11 each hit minor revisions (phase 10's MAJOR §11.1 revision being the most prominent — BRAINSTORM's 5-name protected-set elevated to 6-name + EAGER per-route validation; recorded as ADR-0111 with the explicit "MAJOR amendment to BRAINSTORM Decision 11" framing). Phase 12 hits **TWO MAJOR REVISIONS** (§11.3+§11.7+§11.8 collective; §11.11) and TWO MINOR REVISIONS (§11.2 trichotomy; §11.9 stat-sharing). The pattern of "BRAINSTORM commits hypothesis; SPEC empirically confirms or amends" continues to function as designed. The empirical-pin discipline (BRAINSTORM §9 → SPEC §11) is the project's primary mechanism for catching docstring-trust traps and design-by-analogy errors before they reach the fixture / impl / review gates.

---

## 2. Non-purposes

Phase 12 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.csrf`.

### 2.1 CsrfPolicy proto-message non-goals (per BRAINSTORM §8 + §11.11 amendment)

The proto message `envoy.extensions.filters.http.csrf.v3.CsrfPolicy` carries 3 top-level fields (the smallest §9 family proto surface to date). Phase 12 consumes 1 actively, validates 1 at parse-time but silent-ignores its runtime value, and silent-ignores 1 entirely:

- **Actively consumed (1):** `additional_origins` (`[]StringMatcher`, repeated). StringMatcher.exact variant only with non-empty value is honored. Other StringMatcher variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) are dropped at PARSE time per ADR-0101 §3 verbatim discipline ("only `HeaderMatcher_StringMatch` with non-empty `Exact` value is honored. All other variants … are silent-ignored at parse time"). Non-exact entries do NOT survive the `New` factory; they are filtered out before `runtimeConfig` is constructed. Empty-value `exact` entries are also dropped (matches ADR-0101's "non-empty Exact value is honored" qualifier).

- **Parse-time-validated, runtime-silent-ignored (1):** `filter_enabled` (`RuntimeFractionalPercent`). Per §11.11 amendment: PGV-REQUIRED at parse time (envoy-go's `New` rejects with a non-nil error if the field is nil OR if its inner `default_value` is nil — mirroring Envoy's PGV envelope at the filter-internal level per the phase 11 ADR-0115 filter-internal-validation precedent); silent-ignored at runtime (always-100%-active; the percentage value is read but not consulted). Couples to deferred Runtime + hot restart family. Fixture 0014 sets `default_value: { numerator: 100, denominator: HUNDRED }` explicitly on BOTH sides for differential equivalence (Envoy requires it at PGV; envoy-go validates presence at parse time; runtime is always-100% on both).

- **Silent-ignored entirely (1):** `shadow_enabled` (`RuntimeFractionalPercent`). Optional at PGV (probe #3 confirms boot succeeds without it). Couples to deferred Runtime + hot restart family. envoy-go silent-ignores at parse time (no validation; no rejection if present-but-malformed-`default_value`); runtime is always-never-shadow regardless of proto value. Fixture 0014 omits the field on both sides per §11.11 probe #3 baseline confirming all-defaults parity.

The 1+1+1 = 3-field surface is the complete proto. csrf has NO `stat_prefix` proto field (the proto has only 3 fields, none of them stat-related); stats anchor at the HCM-level stat_prefix per §11.6 amendment.

#### 2.1.1 Out of scope: `additional_origins` StringMatcher non-exact variants

Coupled to whatever future phase lands the full StringMatcher engine (TBD; not currently a §9 family heading). Variants deferred at PARSE time: `prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`. Discipline: PARSE-TIME drop per ADR-0101 §3 (NOT match-time-keep-and-fail). Mirrors phase 09 fault `headers` field verbatim. Future re-activation: when the full StringMatcher engine lands, non-exact variants become operational at parse time; existing fixtures continue to work since they only use `exact`. Re-activation does NOT change the architectural shape (parse-time-decision rather than runtime-decision); it just expands the set of variants that survive parse.

#### 2.1.2 Out of scope: `filter_enabled` percentage-gating + runtime-key handling

Coupled to Runtime + hot restart family. envoy-go validates the field's presence at parse time (per §11.11) but silent-ignores the actual `default_value.numerator` at runtime (always 100%). The `runtime_key` string is also unread (envoy-go has no Runtime layer to look up the key against). When the Runtime + hot restart family lands, this field becomes operational (the divergence-window between envoy-go's always-100% and Envoy's percentage-gating closes for users who explicitly set non-100% values).

#### 2.1.3 Out of scope: `shadow_enabled` shadow-mode evaluation

Coupled to Runtime + hot restart family + a future shadow-mode evaluator. envoy-go silent-ignores at parse time (no PGV validation since Envoy itself permits omission); runtime is always-never-shadow. When shadow mode lands, the field becomes operational; the divergence-window closes; the same 3-counter family continues to be used (per §11.6 confirmation: there is no separate `*_shadow` counter family in reference Envoy — shadow mode reuses `request_invalid` etc.).

### 2.2 Algorithm-shape non-goals (per §11.3 + §11.7 + §11.8 amendments)

The csrf comparison algorithm is **HOST:PORT-ONLY equality**. Specifically OUT of scope:

- **Scheme equality.** Per §11.3 amendment, scheme is computed only to make the URL parseable; `hostAndPort()` strips it on both sides. envoy-go does NOT add a scheme equality check for differential equivalence. Operators wanting scheme-discrimination must use `additional_origins` with explicit port-suffixed entries (e.g., `app.example.test:443` matches `https://app.example.test:443` but not `http://app.example.test:80`).
- **Case folding.** Per §11.7 amendment, `https://APP.EXAMPLE.TEST` does NOT match `app.example.test`. Operators authoring fixture configs / production configs MUST use the lowercase host form they expect Origin headers to carry.
- **Default-port stripping.** Per §11.7 amendment, `https://app.example.test:443` does NOT match `app.example.test`. Operators wanting both forms to match must add both entries to `additional_origins` (or rely on browsers' empirically-uniform behavior of NOT including default ports in `Origin:` headers).
- **`X-Forwarded-Proto`.** Per §11.3 amendment, the header is irrelevant. The `:scheme` pseudo-header (set by HCM from listener TLS state) is consumed for URL parsing but its value is stripped. envoy-go does NOT consult any forwarded-proto header for csrf evaluation.
- **Origin/Referer header presence-only** (i.e., "any non-empty Origin or Referer is sufficient"). The csrf filter REQUIRES the value to either parse to a non-empty `hostAndPort` or be a verbatim string that matches the target / `additional_origins` list. Mere presence is insufficient.

### 2.3 Stat-surface non-goals

- **No filter-specific Prometheus tag-extractor** (UNLIKE phase 11 which introduced `envoy_local_http_ratelimit_prefix`). csrf reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace extractor per §11.6 confirmation. envoy-go's `internal/admin/stats.go` (or wherever the project registers tag-extractors) requires NO new pattern; the existing HCM-namespace SN2 rule covers `http.<HCM stat_prefix>.csrf.<counter>` extraction.
- **No new SN flattening rule.** Phase 12 introduces NO Rule SN10 (or equivalent). The 26-name → 29-name extension is purely additive under the existing SN1–SN9 ruleset (Rule SN9 was introduced by phase 11 ADR-0118 for filter-specific extractors; phase 12 does NOT use SN9 — it uses SN2). Phase 12 extends ADR-0061's enumeration count from 26 to 29 names; no rule change.
- **No `shadow_request_invalid` counter** (per §11.6 confirmation; reference Envoy does NOT emit it either).
- **No twin-series filter discipline** (per BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline`): csrf emits ONE series per HCM stat_prefix per counter. NO per-cluster fan-out, NO per-route emission. All 3 counters are flat under the HCM stat_prefix.
- **No permanently-zero counter** (UNLIKE phase 09's `fault.response_rl_injected` which is emitted permanently-zero per ADR-0107 because it structurally would never increment in MVP). csrf's MVP stat surface drops `shadow_request_invalid` entirely (matching reference Envoy which also does not emit it under all-defaults config).
- **No `enabled` analog** (UNLIKE phase 11's `local_rate_limit.enabled` which counts every evaluated request). csrf's gate is the method check + the disposition table; `request_valid` counts the allow-passthrough cases; `request_invalid` + `missing_source_origin` count the rejection cases. There is no separate "evaluated all modifying requests" counter; the sum equals the modifying-method request count by construction.

### 2.4 Test-surface non-purposes

- **No new differential probe filter.** Phase 07.1's `envoy.filters.http.envoy_go_test` (the iteration-state probe filter) covers framework iteration coverage. Phase 12 does not extend that probe.
- **No new fuzzer category.** The 16th fuzzer `FuzzCsrfPolicyConfigParse` follows the existing `FuzzFooConfigParse` pattern (cors, fault, header_mutation, localratelimit, etc.).
- **No structural-iteration fixture** (07.1's 0007b). Phase 12 is differential-only; the iteration coverage table in §4.1 of this SPEC documents the iteration states exercised relative to the 07.1 framework's full iteration surface.
- **No 14th-fixture renumbering or reshuffling.** Phase 12 is fixture `0014-http-csrf`; the previous fixtures (0000–0013) stay green and unchanged.
- **No GET passthrough scenario in the differential fixture** (per BRAINSTORM §6.2 note on scenario 6). The non-modifying-method passthrough is unit-tested in `csrf_test.go::TestDecodeHeaders_NonModifyingMethods` (parametrized over `GET`, `HEAD`, `OPTIONS`, `TRACE` plus at least one custom verb like `PROPFIND` or `CONNECT`). The differential gate has no GET scenario because the algorithm short-circuits BEFORE any csrf-specific surface (no counter, no origin parse, no `SendLocalReply`) — Envoy and envoy-go both pass the request through as if the filter were absent.

### 2.5 Cross-filter non-purposes

- **No interaction with cors / fault / header_mutation / local_ratelimit per-route configs in fixture 0014.** Phase 12's fixture configures ONLY `csrf` filters (plus the router terminal). Mixed-filter ordering tests (e.g. cors + csrf on the same listener) are deferred to a future "filter-chain-ordering" hardening phase or to the existing 0007a-cors / 0011-http-fault / 0012-http-header-mutation / 0013-http-local-ratelimit fixture extensions if needed.
- **No HCM-level changes.** Phase 12 reuses the existing `internal/filter/hcm/` body discipline + `serverHeader()` returning `"envoy"` (per `internal/filter/hcm/codec.go:17`). The brainstorm's hypothesis on `server: envoy` was correct and required no §1.1 amendment (matches phase 11's correction of the BRAINSTORM-hypothesized `envoy-go`; the `envoy` value is the canonical wire-form per §11.10 confirmation).
- **No extension to existing per-route framework primitives.** Phase 12 reuses `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103–128`); no `ResolveAllTiers` invocation (unlike phase 10 header_mutation), no new framework callback, no `RegisterPerRouteValidator` hook (unlike phase 10). The phase 10 `ResolveAllTiers` + `RequestRouteConfigsAllTiers` + `ResponseRouteConfigsAllTiers` extensions stay landed and unused by phase 12 — they are header_mutation-specific surface.

### 2.6 Security non-purposes

- **No browser-CSRF threat-model characterization.** Phase 12 implements the same-origin enforcement primitive itself; characterizing its strength against advanced CSRF attack variants (subdomain takeover, DNS rebinding, HTTP-to-HTTPS upgrade attacks against `Origin:` validation) is out of scope. The filter mirrors reference Envoy's behavior verbatim — including the §11.7 + §11.8 footguns (NO scheme comparison; NO normalization). Operators relying on csrf for security MUST understand the host:port-only equality semantics.
- **No timing-attack characterization** of `tryMatch()` against `additional_origins[]`. The list iteration is O(n) string comparisons; no timing-uniform implementation is provided. Per-route TPFC sizes are bounded by config size (no unbounded list growth at runtime).
- **No `Origin: null` CSRF threat-model documentation** (e.g., the WebKit-historic `Origin: null` semantics for sandboxed iframes / file:// origins). envoy-go faithfully replicates Envoy's behavior of treating `Origin: null` as an empty source_origin (rejection path); the security implications are the same as upstream Envoy.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 12)

The six-gate phase-done discipline (per phase 04+ canonical layout) for phase 12:

| Gate | Specialization for phase 12 |
|---|---|
| **A — Build / vet / lint clean** | `go build ./...`, `go vet ./...`, `golangci-lint run` all green; no new warnings introduced relative to the phase-11 baseline at master tip `bb29bb0`. New package `internal/filter/http/csrf/` lints clean. |
| **B — Race-test pass** | `go test -race ./...` green on all 34 packages plus the new `internal/filter/http/csrf/` package (35 packages total). Test count grows by ~25–35 (6 unit-test groups across the new test file). csrf has no shared mutable state (the `runtimeConfig` is read-only after `New`; `*filterStats` uses `atomic.Int64`); race-test cleanness is structural. |
| **C — h2spec 53/53 PASS** | Conformance gate at the ADR-0051 pin (53/53 PASS); phase 12 introduces no HTTP/2 stack changes, so this is a regression check — not an extension. |
| **D — All fuzzers green at 30s budget** | 15 existing fuzzers (per phase 11 phase-done) + 1 new (`FuzzCsrfPolicyConfigParse`) = 16 fuzzers. Each runs 30s in the per-phase fuzzer gate; all green. |
| **E — All differential fixtures 0000–0014 PASS** | 14 prior fixtures + the new `0014-http-csrf` = 15 fixtures green. Total runtime estimated ~45–55s wallclock (phase 11 reported ~43–45s for 14 fixtures; fixture 0014 adds ~3–5s for its 6 scenarios — all synchronous, no timing tolerances). |
| **F — `BEHAVIOR_CONTRACT.md` populated** | §13.1 new subsection `### envoy.filters.http.csrf` (inline; ~50 lines per the phase 09 / 10 / 11 precedent); §13.2 stat-table 26→29 extension (3 new rows, NO new tag-extractor); §13.3 NEW equivalence-matrix row pointing at fixture 0014 with per-scenario tolerance discipline; §13.4 NEW forward-pointer notes subsection (`### Phase 12 forward-pointer notes`) covering the 3-item deferral list (per §8 below). All edits land in-place per ADR-0052 at the phase-done commit. |

Gates A–E are the verification gates; Gate F is the contract-extension gate. All six must be green at the phase-done commit per `BOOTSTRAP_PROMPT.md` §7.5.

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 12)

```
internal/filter/http/csrf/doc.go         ~25 LoC; package overview + 1-consumed/1-PGV-validated-not-honored/1-deferred decomposition
internal/filter/http/csrf/csrf.go        ~250-400 LoC; filter + factory + origin-parsing helpers + runtimeConfig + DecodeHeaders + filterStats
internal/filter/http/csrf/csrf_test.go   ~150-200 LoC; 6 test groups per §6.5
internal/filter/http/csrf/fuzz_test.go   ~40 LoC; FuzzCsrfPolicyConfigParse (16th fuzzer)
```

PLAN author may split `csrf.go` into `csrf.go` + `origin.go` (origin-parsing helpers) for readability — §6 leaves the file split open. Estimated total: ~465–665 LoC of new Go code.

### 4.2 Changed production code (in 12)

```
cmd/envoy-go/main.go         +1 line: httpReg.Register(csrf.TypeURL, csrf.New) before httpReg.Freeze
                              Insertion ordering: alphabetical-after-router per ADR-0100 §2.2 convention:
                              router → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze
                              csrf inserts between `cors` and `envoy_go_test` (alphabetical).
```

NO additional changes in `internal/admin/stats.go` (per §11.6 confirmation: NO new tag-extractor needed; the existing HCM-namespace extractor covers the new 3 csrf counters).

NO changes in `internal/filter/http/perroute.go` (existing 3-tier `Resolve` reused as-is; no `ResolveAllTiers` extension needed; no `RegisterPerRouteValidator` hook needed).

NO changes in `internal/stats/registry.go` (existing `NewCounter` + `NewCounterIfAbsent` primitives reused; no new method needed).

### 4.3 New harness and fixture code (in 12)

```
test/fixtures/0014-http-csrf/envoy.yaml          ~70 LoC; reference Envoy STRICT_DNS, 1 listener, 2 routes (default + /route-only), filter_enabled=100% explicit
test/fixtures/0014-http-csrf/envoy-go.yaml       ~70 LoC; envoy-go STATIC, same listener, filter_enabled field PRESENT (validated at parse, silent-ignored at runtime by envoy-go)
test/fixtures/0014-http-csrf/backend/main.go     ~30 LoC; simple HTTP backend echoing 200 + request marker (or reused from 0011/0012/0013 helpers)
test/fixtures/0014-http-csrf/driver/main.go      ~150 LoC; 6-scenario orchestration; per-scenario state-reset (csrf has no per-process state; restart-between is overkill — sequential drives suffice)
test/fixtures/0014-http-csrf/expectations.yaml   ~30 LoC; per-scenario counter delta + body byte-exact + header set
test/fixtures/0014-http-csrf/README.md           ~50 LoC; SPEC §7 narrative
```

Estimated ~400 LoC fixture-bundle (Go + YAML + Markdown). Fixture directory structure mirrors `test/fixtures/0011-http-fault/`, `0012-http-header-mutation/`, `0013-http-local-ratelimit/`. UNLIKE 0013, NO timing-sensitive scenarios — csrf is purely synchronous (no `time.Sleep` at the t=DELAY check; per-scenario teardown is optional since csrf has no leakable state).

### 4.4 Changed documentation and state (in 12)

Lands across SPEC commit (this commit) + impl commits + phase-done commit:

```
docs/envoy-go/ROADMAP.md           SPEC commit: flip row 12 status `planned → in-progress`; phase-done commit: flip to `done` + finalize summary
docs/envoy-go/STATE.md             SPEC commit: lifecycle-state spec-complete + next-skill writing-plans + last-commit SHA-fill
                                   subsequent commits: per-task SHA-fill + state advance + last-updated bump
docs/envoy-go/DECISIONS.md         impl commits: append ADR-0120..ADR-0124 (5 ADRs per §8); NO ADR-0073 amendment paragraph; NO ADR-0061 amendment
                                   NO ADR additions at SPEC commit (phase 09 / 10 / 11 precedent: ADRs land during impl, NOT at SPEC)
docs/envoy-go/BEHAVIOR_CONTRACT.md phase-done commit: §13 4-edit bundle (NEW subsection + stat-table extension + equivalence-matrix new row + forward-pointer notes new subsection)
docs/envoy-go/phases/12-http-filter-csrf/SPEC.md     SPEC commit: this file
docs/envoy-go/phases/12-http-filter-csrf/PLAN.md     authored next session per writing-plans
docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md authored at PLAN time; per-task SHA-fill during impl
docs/envoy-go/phases/12-http-filter-csrf/REVIEW.md   authored at phase-done close
```

This SPEC commit's diff stat: 1 new file (this SPEC.md) + 1 ROADMAP row update + STATE.md update. Total ~1100 SPEC lines + ~3 ROADMAP lines + ~10 STATE.md lines.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 12)

```
   cmd/envoy-go/main.go
        │
        │ httpReg.Register(csrf.TypeURL, csrf.New)        [NEW LINE; phase 12]
        ▼
   internal/filter/http/registry.go
        │
        │ Register(typeURL, factory) → Freeze() → Resolve(typeURL) at HCM build
        ▼
   internal/filter/http/csrf/                                                  [NEW PACKAGE; phase 12]
        ├── csrf.go        (filter + factory + runtimeConfig + origin-parsing helpers + DecodeHeaders + filterStats)
        ├── csrf_test.go
        ├── fuzz_test.go   (FuzzCsrfPolicyConfigParse)
        └── doc.go
        │
        │ uses:
        ▼
   internal/filter/http/perroute.go            (existing; PerRouteConfig.Resolve — 3-tier, most-specific)
   internal/filter/http/                       (existing; HTTPFilter interface, Continue/StopIteration enums)
   internal/stats/registry.go                  (existing; per-instance *atomic.Int64 emit primitives via NewCounter)
   net/url                                     (stdlib; for Origin/Referer URL parsing — see §6.4 detail)
   strings                                     (stdlib; case-sensitive comparisons, split, trim)
```

Untouched (load-bearing absence):

```
internal/filter/http/perroute.go               (existing 3-tier Resolve; phase 12 reuses; NO ResolveAllTiers needed)
internal/filter/http/registry.go               (existing extension registry + Freeze; phase 12 adds one Register call site upstream)
internal/filter/http/cors/                     (untouched)
internal/filter/http/fault/                    (untouched; reused as the SendLocalReply + StopIteration precedent)
internal/filter/http/header_mutation/          (untouched)
internal/filter/http/localratelimit/           (untouched; phase 12 explicitly does NOT inherit its filter-specific tag-extractor pattern per §11.6)
internal/filter/http/router/                   (untouched)
internal/filter/http/envoygotest/              (untouched)
internal/filter/hcm/                           (untouched; HCM stays the chain runner; serverHeader() literal "envoy" preserved)
internal/listener/                             (untouched)
internal/cluster/                              (untouched)
internal/admin/                                (untouched; existing HCM-namespace tag-extractor covers csrf stats per §11.6)
internal/drain/                                (untouched)
```

### 5.2 Per-request flow — non-modifying-method passthrough (canonical, scenario 6 unit-only)

Request: `GET /something HTTP/1.1` arriving on listener configured with csrf (any policy).

```
1. HCM IngressFilter.DecodeHeaders fires.
2. HCM resolves filter chain; csrf.filter.DecodeHeaders called.
3. filter.DecodeHeaders:
   a. Read :method → "GET".
   b. Method ∉ {POST, PUT, DELETE, PATCH} → return Continue immediately.
      NO origin parse, NO counter increment, NO state touch.
4. HCM advances chain to router; cluster dial; backend response.
5. EncodeHeaders/EncodeData: pass-through (filter has NO encode-side state).
6. Counters: ALL 3 csrf counters unchanged from baseline (regardless of Origin/Referer presence).
```

This scenario is unit-tested in `csrf_test.go::TestDecodeHeaders_NonModifyingMethods` parametrized over `GET, HEAD, OPTIONS, TRACE` + at least one custom verb (`PROPFIND` or similar). NOT in the differential fixture (per BRAINSTORM §6.2 note + §2.4 above).

### 5.3 Per-request flow — same-origin allow path (scenario 1)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nOrigin: http://127.0.0.1:<port>\r\n` over a plaintext listener.

```
1. csrf.filter.DecodeHeaders called.
2. Read :method → "POST"; Method ∈ modifying set → continue.
3. Resolve runtimeConfig via PerRouteConfig.Resolve: returns listener-level rc.
4. (filter_enabled is parse-validated; runtime-silent-ignored — filter always proceeds as if 100%-active.)
5. Compute target_origin's hostAndPort:
   a. Read :scheme → "http" (HCM sets from listener TLS state); read :authority/Host → "127.0.0.1:<port>".
   b. Construct URL "http://127.0.0.1:<port>"; parse via net/url.Parse.
   c. hostAndPort = url.Host = "127.0.0.1:<port>".
6. Compute source_origin's hostAndPort (per §6.4 trichotomy):
   a. Read Origin header → "http://127.0.0.1:<port>" (non-empty; non-"null").
   b. Parse via net/url.Parse → scheme="http", host="127.0.0.1:<port>".
   c. hostAndPort = "127.0.0.1:<port>".
7. Compare: source.hostAndPort == target.hostAndPort? "127.0.0.1:<port>" == "127.0.0.1:<port>" → true.
8. Increment rc.stats.requestValid (atomic.Int64.Add(1)).
9. Return Continue.
10. HCM advances chain; backend response.
11. Counters: request_valid=1, request_invalid=0, missing_source_origin=0 (delta from baseline).
```

### 5.4 Per-request flow — cross-origin reject path (scenario 2)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nOrigin: https://evil.test\r\n` over the same listener.

```
1. csrf.filter.DecodeHeaders called.
2. Method "POST" ∈ modifying.
3. Resolve runtimeConfig: listener-level rc with additional_origins=["app.example.test"].
4. Compute target.hostAndPort = "127.0.0.1:<port>".
5. Compute source.hostAndPort:
   a. Origin = "https://evil.test"; parse → host="evil.test"; hostAndPort = "evil.test".
6. Compare:
   a. source ("evil.test") == target ("127.0.0.1:<port>")? No.
   b. source matches any rc.additionalOrigins[]? Iterate the compiled-origin list: "evil.test" == "app.example.test"? No.
7. source non-empty → request_invalid path.
8. Increment rc.stats.requestInvalid.
9. Build 4-header set: OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}.
   (HCM/router auto-injects content-length=14, date=<now-RFC1123>, server=envoy on response wire.)
10. cb.SendLocalReply(403, []byte("Invalid origin"), headers).
11. Return StopIteration.
12. HCM's localReplyDone gate ensures the chain short-circuits without dialing the upstream
    (per phase 09 ADR-0102's terminal-replace pattern).
13. Counters: request_invalid=1; request_valid=missing_source_origin=0 (delta from baseline).
14. Wire response: HTTP/1.1 403 Forbidden + 4 headers (content-length, content-type, date, server) + body "Invalid origin" (14 bytes, no LF).
```

### 5.5 Per-request flow — additional_origins exact-match allow path (scenario 3)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nOrigin: https://app.example.test\r\n` against rc with `additional_origins=["app.example.test"]` (HOST:PORT form per §11.8 amendment).

```
1. csrf.filter.DecodeHeaders called; method "POST" ∈ modifying.
2. Compute target.hostAndPort = "127.0.0.1:<port>".
3. Compute source.hostAndPort:
   a. Origin = "https://app.example.test"; parse → host="app.example.test" (no port; default port 443 NOT appended); hostAndPort = "app.example.test".
4. Compare:
   a. source ("app.example.test") == target ("127.0.0.1:<port>")? No.
   b. source matches rc.additionalOrigins[]? "app.example.test" == "app.example.test"? Yes.
5. source matched → request_valid path.
6. Increment rc.stats.requestValid.
7. Return Continue.
```

**Note on operator footgun (per §11.8 amendment):** if the listener config had `additional_origins: [{exact: "https://app.example.test"}]` (full-URL form), the compiled list would be `["https://app.example.test"]` and step 4(b) would mismatch (source `"app.example.test"` ≠ list entry `"https://app.example.test"`), incorrectly rejecting the legitimate cross-origin request. SPEC §6.4 + BEHAVIOR_CONTRACT §13.1 must call this out unmistakably.

### 5.6 Per-request flow — missing source origin reject path (scenario 4)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\n` (NO `Origin`, NO `Referer`).

```
1. csrf.filter.DecodeHeaders called; method "POST" ∈ modifying.
2. Compute target.hostAndPort.
3. Compute source.hostAndPort (per §6.4 trichotomy):
   a. Read Origin → absent. Empty hostAndPort → fall back to Referer.
   b. Read Referer → absent. Empty hostAndPort.
   c. source = "" (EMPTY_STRING).
4. source empty → missing_source_origin path.
5. Increment rc.stats.missingSourceOrigin.
6. SendLocalReply(403, "Invalid origin", headers); StopIteration.
```

The same path is taken for `Origin: null` (§6.4 case 1) and for `Origin:` empty + Referer absent (§6.4 case 2).

### 5.7 Per-request flow — Referer fallback allow path (scenario 5)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nReferer: http://127.0.0.1:<port>/somepage\r\n` (NO `Origin`).

```
1. csrf.filter.DecodeHeaders called; method "POST" ∈ modifying.
2. Compute target.hostAndPort = "127.0.0.1:<port>".
3. Compute source.hostAndPort:
   a. Origin absent. Empty hostAndPort → fall back to Referer.
   b. Referer = "http://127.0.0.1:<port>/somepage"; parse → host="127.0.0.1:<port>" (path "/somepage" dropped); hostAndPort = "127.0.0.1:<port>".
4. Compare: source == target? Yes.
5. Increment rc.stats.requestValid; return Continue.
```

### 5.8 Per-request flow — per-route TPFC override (scenario 7)

Listener `l_main` with listener-level config (additional_origins=[`app.example.test`]) + route `/route-only` carrying TPFC override (additional_origins=[`route-only.test`]).

```
Request A: POST /route-only Origin: https://route-only.test
1. PerRouteConfig.Resolve: most-specific config is per-route TPFC; returns rc_route (independent runtimeConfig with its own []compiledOrigin slice).
2. (Per §11.9 amendment: rc_route.stats == listener-level *filterStats — per-route DOES NOT carry independent stats.)
3. target.hostAndPort = "127.0.0.1:<port>"; source.hostAndPort = "route-only.test".
4. source != target; source matches rc_route.additionalOrigins[]? "route-only.test" == "route-only.test"? Yes.
5. Increment rc_route.stats.requestValid (= listener-level requestValid).
6. Return Continue.

Request B: POST / Origin: https://route-only.test (default route)
1. PerRouteConfig.Resolve: no per-route config; returns rc_listener.
2. target.hostAndPort = "127.0.0.1:<port>"; source.hostAndPort = "route-only.test".
3. source != target; source matches rc_listener.additionalOrigins[]? "route-only.test" == "app.example.test"? No.
4. source non-empty → request_invalid path.
5. Increment rc_listener.stats.requestInvalid (= the SAME *filterStats; counters AGGREGATE across routes).
6. SendLocalReply(403, "Invalid origin"); StopIteration.
```

This is the FIRST production filter to demonstrate that ADR-0073's wholesale-override discipline carries through to per-route data-only configs WITH SHARED stats (i.e., per-route runtimeConfig replaces listener-level data but does NOT carry an independent counter series). Phase 11's local_ratelimit is the precedent for per-route stateful resources WITH INDEPENDENT stats (ADR-0117). Phase 12 is the inverse pattern; ADR-0124 captures it. Confirmed empirically at §11.9.

### 5.9 Concurrency model

Per-filter-instance: `runtimeConfig` is a `*runtimeConfig` reference; the underlying `runtimeConfig` value (including the `[]compiledOrigin` slice + the `*filterStats` pointer) is shared across all goroutines processing requests through that filter instance. The `*runtimeConfig` is closure-captured at boot-time `New` and never mutated — it is read-only thread-safe. Per-route `runtimeConfig` instances are similarly immutable post-config-load.

Per-process: registry frozen at boot per ADR-0072 (no runtime registration); per-route TPFC parsed at HCM-build time; counter increments via `*atomic.Int64.Add(1)` (race-clean by construction).

NO mutex. NO LBP-1-adjacent declaration (UNLIKE phase 11 which had `sync.Mutex` per `*tokenBucket` and ADR-0116's LBP-1-adjacent commentary). csrf is purely lock-free at the request hot path; the `additional_origins` slice iteration is a read-only loop over an immutable slice; the counter increments are atomic; the `*filterStats` is a struct of `*atomic.Int64`s allocated once at `New` time and never reallocated. Phase 12 is the FIRST production HTTP filter with stats that does NOT need any synchronization primitive at the request hot path. Captures naturally under the existing LBP-1 (closure-capture half preserved; lock-free half preserved — no departure required).

### 5.10 Filter ordering in fixture 0014

Filter chain in the listener (single listener for fixture 0014):

```
[envoy.filters.http.csrf] → [envoy.filters.http.router]
```

`csrf` is the first (and only non-router) filter. No interaction with cors / fault / header_mutation / local_ratelimit (per §2.5). Order does not matter for correctness — csrf is the only non-router filter — but the SPEC pins it as `[csrf, router]` for explicitness.

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (cors / fault / localratelimit precedent verbatim)

```go
package csrf

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"

func New(ctx envoyhttp.FactoryCtx, tc *anypb.Any) (envoyhttp.HTTPFilterFactory, error) {
    // 1. tc.UnmarshalTo(&cfg) where cfg is *envoyextensionsfiltershttpcsrfv3.CsrfPolicy.
    // 2. Validate filter-internal constraints (filter_enabled non-nil + filter_enabled.default_value non-nil
    //    per §11.11 amendment + ADR-0121).
    // 3. Build runtimeConfig (closure-captured by the returned factory):
    //    a. Compile additional_origins[] — for each entry, if StringMatcher.exact non-empty → append
    //       value to []compiledOrigin slice. Drop non-exact variants + empty exact entries (ADR-0101 §3).
    //    b. Allocate filterStats — register 3 counters via ctx.Stats.NewCounter (HCM stat_prefix scoping).
    // 4. Return a closure that constructs per-request *filter values referencing the closure-captured rc.
}
```

The `envoyhttp.FactoryCtx` 3-field shape from ADR-0100 stays as-is. Phase 12 consumes 2 of the 3 fields:
- `ctx.Stats` (consumed for filterStats wiring, per §6.6).
- `ctx.StatPrefix` (consumed for stat naming — csrf has NO `stat_prefix` proto field; the namespace anchor is the HCM-level `stat_prefix` per §11.6 confirmation).
- `ctx.Registry` (NOT consumed; csrf does no cross-filter lookup).

### 6.2 `runtimeConfig` shape (per ADR-0121)

```go
type runtimeConfig struct {
    additionalOrigins []string         // compiled — only surviving exact-with-non-empty-value entries; verbatim values from proto
    stats             *filterStats     // listener-level stats; per-route runtimeConfig SHARES this pointer per §11.9
}

type filterStats struct {
    requestValid, requestInvalid, missingSourceOrigin *atomic.Int64
}
```

Per-route `runtimeConfig` instances are independent per-TPFC-entry FOR DATA (each carries its own `additionalOrigins` slice from its own `New` invocation) but SHARE the listener-level `*filterStats` pointer (per §11.9 amendment — see §6.6 for the wiring detail). The 1 deferred top-level field (`shadow_enabled`) is unmarshalled (proto unmarshal is uniform per ADR-0040) but NOT captured into `runtimeConfig`. The `filter_enabled` field is parse-validated (presence + inner default_value presence) but its value is NOT captured into `runtimeConfig` (silent-ignored at runtime). NO warnings; NO rejections beyond the parse-time PGV-mirror check.

### 6.3 Per-instance `filter` struct

```go
type filter struct {
    rc  *runtimeConfig                         // closure-captured at factory time; immutable
    dcb envoyhttp.DecoderFilterCallbacks       // set in OnNewStream (or equivalent) per the existing 07.1 pattern
}
```

Phase 12 does NOT consume `EncoderFilterCallbacks` (no encode-side state); the existing framework's `OnNewStream` wiring sets only `dcb`. (PLAN author confirms which precise callback-setup hook the existing framework exposes — see §12 deferred decision 1.)

### 6.4 Origin extraction + comparison algorithm (per ADR-0122 + §1.1 amendments 1, 2)

```go
// Source-origin extraction trichotomy (per §11.2 amendment):
func sourceOriginValue(headers RequestHeaders) string {
    originVal := headers.Get("Origin")
    if originVal == "null" {                          // case 1: literal "null" → empty
        return ""
    }
    // case 2 + 3: parse Origin
    sourceHostPort := hostAndPort(originVal)
    if sourceHostPort != "" {
        return sourceHostPort                         // case 3 (parsed) OR case 3' (verbatim if URL parse failed)
    }
    // case 2: empty hostAndPort → fall back to Referer
    refererVal := headers.Get("Referer")
    return hostAndPort(refererVal)
}

// hostAndPort: parse an absolute URL; return host[:port], or verbatim string on parse failure
// (per §11.3 source-of-truth: Envoy's Http::Utility::Url::initialize → return hostAndPort if parse OK,
//  else return raw input verbatim)
func hostAndPort(absoluteURL string) string {
    if absoluteURL == "" {
        return ""
    }
    u, err := url.Parse(absoluteURL)
    if err != nil || u.Host == "" {
        return absoluteURL                            // verbatim on parse failure (matches Envoy)
    }
    return u.Host                                     // u.Host is host[:port] form
}

// Target-origin construction (per §11.3 amendment — scheme is NOT compared):
func targetOriginValue(headers RequestHeaders, isTLS bool) string {
    var scheme string
    if schemeVal := headers.Get(":scheme"); schemeVal != "" {
        scheme = schemeVal
    } else if isTLS {
        scheme = "https"
    } else {
        scheme = "http"
    }
    host := headers.Get(":authority")                 // HTTP/2; for HTTP/1.1 fall back to Host header
    if host == "" {
        host = headers.Get("Host")
    }
    return hostAndPort(scheme + "://" + host)         // hostAndPort strips scheme; only host[:port] survives
}

// Comparison + disposition (called from DecodeHeaders only when method ∈ modifying).
// Per Envoy csrf_filter.cc decodeHeaders evaluation order: additional_origins are checked
// FIRST, then target equality. Outcome is identical for byte-equal source/target/list — but
// the byte-shape of the code mirrors upstream verbatim:
func evaluate(rc *runtimeConfig, source, target string) (allow bool, missing bool) {
    if source == "" {
        return false, true                            // missing_source_origin
    }
    for _, additional := range rc.additionalOrigins {
        if source == additional {
            return true, false                        // request_valid (additional_origins match)
        }
    }
    if source == target {
        return true, false                            // request_valid (same-origin)
    }
    return false, false                               // request_invalid
}
```

**Operator-facing semantics (per §11.7 + §11.8 amendments):**

- The comparison is **byte-exact equality on `host[:port]` strings**. NO case folding (uppercase `Origin: HTTPS://APP.EXAMPLE.TEST` does NOT match lowercase config). NO default-port stripping (`Origin: https://app.example.test:443` does NOT match config entry `app.example.test`). Trailing slashes are stripped via `url.Parse` (path component drops). 
- `additional_origins[].exact` values are stored verbatim from the proto. Operators MUST write them in `host[:port]` form WITHOUT scheme prefix. Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header (because the source's `hostAndPort` is `app.example.test`, not `https://app.example.test`). **OPERATOR FOOTGUN** — BEHAVIOR_CONTRACT §13.1 documents this with a worked example.
- `Origin: null` literal is the ONLY value that short-circuits to empty source (NO Referer fallback). Empty `Origin:` (or absent) DOES fall back to Referer. Non-empty unparseable `Origin:` (e.g., `Origin: not-a-url`, `Origin: example.com` without scheme) is taken verbatim and almost-always rejects (since the verbatim string mismatches the target's `host[:port]`).

PLAN author confirms the precise Go stdlib URL parsing semantics against `url.Parse` to ensure byte-equivalence with Envoy's `Http::Utility::Url::initialize` for the §6.4 algorithm — particularly for edge cases like `//host:port` (no scheme), `127.0.0.1:port` (no `://`), and IPv6 literal hosts like `[::1]:8080`. Spot-checks during PLAN time per §12 deferred decision 2.

### 6.5 `DecodeHeaders` body discipline (per ADR-0123)

```go
var modifyingMethods = map[string]struct{}{
    "POST":   {},
    "PUT":    {},
    "DELETE": {},
    "PATCH":  {},
}

func (f *filter) DecodeHeaders(_ context.Context, headers envoyhttp.RequestHeaders, _ bool) (envoyhttp.HeadersStatus, error) {
    method := headers.Get(":method")                  // HCM normalizes HTTP/1.1 method to upper-case
    if _, ok := modifyingMethods[method]; !ok {
        return envoyhttp.HeadersStatusContinue, nil   // non-modifying: short-circuit BEFORE any state touch
    }

    target := targetOriginValue(headers, f.dcb.DownstreamTLS())
    source := sourceOriginValue(headers)

    allow, missing := evaluate(f.rc, source, target)
    switch {
    case allow:
        f.rc.stats.requestValid.Add(1)
        return envoyhttp.HeadersStatusContinue, nil
    case missing:
        f.rc.stats.missingSourceOrigin.Add(1)
    default:
        f.rc.stats.requestInvalid.Add(1)
    }

    f.dcb.SendLocalReply(403, []byte("Invalid origin"), envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    return envoyhttp.HeadersStatusStopIteration, nil
}
```

`DecodeData`, `DecodeTrailers`, `EncodeHeaders`, `EncodeData`, `EncodeTrailers`, `OnDestroy` are all pass-through (return `Continue` / no-op).

The 4-header wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) is produced by the SendLocalReply call site (`Content-Type` from the filter) + HCM/router downstream auto-injection (`content-length`, `date`, `server` per the existing fault precedent in `internal/filter/http/fault/fault.go:321` + `internal/filter/http/router/router.go:70`).

### 6.6 `filterStats` wiring (per ADR-0124 + §11.6)

The 3-counter `filterStats` is wired through the existing `internal/stats.Registry`:

```go
type filterStats struct {
    requestValid, requestInvalid, missingSourceOrigin *atomic.Int64
}

func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
    return &filterStats{
        requestValid:        reg.NewCounter("http." + hcmStatPrefix + ".csrf.request_valid"),
        requestInvalid:      reg.NewCounter("http." + hcmStatPrefix + ".csrf.request_invalid"),
        missingSourceOrigin: reg.NewCounter("http." + hcmStatPrefix + ".csrf.missing_source_origin"),
    }
}
```

Stat-name templates:

- **Text format:** `http.<HCM stat_prefix>.csrf.{missing_source_origin, request_invalid, request_valid}` (3 names; lexicographic order in `/stats` output).
- **Prometheus format:** `envoy_http_csrf_{missing_source_origin, request_invalid, request_valid}{envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`.

The Prometheus tag-extraction reuses the existing `envoy_http_conn_manager_prefix` extractor (per §11.6 confirmation; SN2 rule from ADR-0061 covers the `http.<stat_prefix>.<rest>` form). NO new tag-extractor pattern. NO new SN flattening rule.

**Per-route stat-sharing (per §11.9 amendment):** the `*filterStats` pointer is allocated ONCE per HCM scope at `New` time (when the listener-level CsrfPolicy is parsed). Per-route TPFC entries that carry their own `CsrfPolicy` produce their own `*runtimeConfig` (with their own `additionalOrigins` slice), but the `*filterStats` pointer is the SAME listener-level pointer (the per-route `New` invocation does NOT re-register its own counters — it reuses the listener-level registration). PLAN author resolves the precise wiring mechanism: either (a) the per-route `New` is called with a context carrying the listener-level `*filterStats` pointer, or (b) the per-route `runtimeConfig` is built differently from the listener-level one (e.g., via a separate `parsePerRouteConfig` helper that takes an existing `*filterStats` argument). Mechanism choice is §12 deferred decision 4.

### 6.7 Per-route 3-tier resolve (existing primitive; reused per ADR-0073, no amendment)

Phase 12 reuses `internal/filter/http/perroute.go:103–128`'s existing `PerRouteConfig.Resolve` (most-specific tier wins; ADR-0073 wholesale-override). NO `ResolveAllTiers` invocation (unlike phase 10). NO new framework primitive.

The resolver returns a `*runtimeConfig` value (the `*runtimeConfig` returned by the most-specific tier's `BuildPerRouteConfig` factory invocation) for the request. The filter dereferences the `*runtimeConfig` to get the closure-captured `*filterStats` (which is the listener-level pointer per §6.6) + the resolved `additionalOrigins` slice. Wholesale-override means: if Route-tier has a TPFC entry, the listener-level config's data (additionalOrigins) is **entirely shadowed** for that request — but the stats counters are SHARED. Confirmed empirically at §11.9.

ADR-0073 needs NO additional amendment for phase 12 — the wholesale-override discipline already extends to data-only per-route configs with shared stats; phase 11's ADR-0117 amendment paragraph (which extended wholesale-override to STATEFUL per-route resources with INDEPENDENT stats) is the load-bearing precedent for the contrast. Phase 12 is the inverse pattern — DATA-ONLY per-route with SHARED stats — and it falls within the original ADR-0073 wholesale-override semantics without further amendment.

---

## 7. Differential fixture `0014-http-csrf`

### 7.1 Equivalence claims (per BRAINSTORM §6 + §1.1 amendments)

Six scenarios, mirrored across reference Envoy v1.37.2 (STRICT_DNS) and envoy-go (STATIC), per ADR-0019's differential equivalence discipline. Scenario numbering preserved from BRAINSTORM §6.2 (1-5 + 7; scenario 6 unit-only).

| # | Scenario | Route | Workload | Asserts |
|---|---|---|---|---|
| 1 | same-origin POST allowed | `/` | `POST / Origin: http://127.0.0.1:<port>` | 200 + backend body passthrough; `request_valid +1` |
| 2 | cross-origin POST rejected | `/` | `POST / Origin: https://evil.test` | 403 + `Invalid origin` body (14 bytes) + 4-header lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`); `request_invalid +1` |
| 3 | `additional_origins` exact-match allowed | `/` | `POST / Origin: https://app.example.test` | 200 + backend body; `request_valid +1` (per §11.8 amendment: `additional_origins` entry MUST be `app.example.test` host:port form, NOT `https://app.example.test`) |
| 4 | no source-origin rejected | `/` | `POST /` (no `Origin`, no `Referer`) | 403 + `Invalid origin` + 4-header set; `missing_source_origin +1` |
| 5 | Referer fallback allowed | `/` | `POST / Referer: http://127.0.0.1:<port>/somepage` (no `Origin`) | 200 + backend body; `request_valid +1` |
| 7 | per-route wholesale override | `/route-only` + `/` | (a) `POST /route-only Origin: https://route-only.test` → 200 + backend body; (b) `POST / Origin: https://route-only.test` → 403 + `Invalid origin` (matches neither listener-default nor `app.example.test`) | mixed: 200 + 403; `request_valid +1`, `request_invalid +1` (counters AGGREGATE per §11.9 — no independent series) |

**No timing tolerances.** UNLIKE phase 11 fixture 0013 which had a `refill-after-fill_interval ±10ms` scenario, phase 12 fixture 0014 has NO timing-sensitive scenarios — csrf is purely synchronous.

**No H2 differential coverage.** Phase 12 fixture 0014 is HTTP/1.1-only. H2 differential testing of csrf is deferred (matching the phase 09 / 10 / 11 precedent).

Per-scenario teardown is OPTIONAL — csrf has NO per-process state to leak (no token bucket, no goroutine, no timer, no mutex-protected counters). The driver may run all 7 requests (scenarios 1, 2, 3, 4, 5, 7a, 7b) in a single sequential pass against a single Envoy + envoy-go boot, then scrape `/stats/prometheus` once at the end and compare deltas.

### 7.2 Driver outline

Single Go binary `test/fixtures/0014-http-csrf/driver/main.go` orchestrates all six scenarios per the 0011-fault / 0012-header_mutation / 0013-local_ratelimit precedents:

```
driverMain():
  1. Spawn reference Envoy (docker run) on disjoint port pair (admin + listener).
  2. Spawn envoy-go on disjoint port pair.
  3. Wait for /ready on both.
  4. Issue 7 sequential requests (scenarios 1, 2, 3, 4, 5, 7a, 7b); for each:
     - Issue identical request to both Envoy and envoy-go.
     - Compare per-request status codes.
     - Compare per-request response headers (lowercase wire-form, 4-header set for 403).
     - Compare per-request response body bytes (14-byte "Invalid origin" for 403; backend response for 200).
  5. Scrape /stats/prometheus from both admin endpoints.
  6. Compare counter deltas across the 3 stat names with `envoy_http_conn_manager_prefix` Prometheus label.
  7. Tear down both servers (SIGTERM + wait); reap processes.
  Report: PASS if all scenarios match; FAIL with first-divergence dump otherwise.
```

NO retry-with-deadline harness needed (csrf is synchronous; no timing tolerance). Driver size: ~150 LoC estimated.

### 7.3 Fixture bootstrap (per BRAINSTORM §7; port-disambiguated)

```yaml
# envoy.yaml fragment (reference Envoy STRICT_DNS):
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9914 }
static_resources:
  listeners:
    - name: l_main
      address: { socket_address: { address: 0.0.0.0, port_value: 0 } }   # driver-rendered
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_csrf
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh_main
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/route-only" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.csrf:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy
                              filter_enabled:
                                runtime_key: __route_only_enabled
                                default_value: { numerator: 100, denominator: HUNDRED }
                              additional_origins:
                                - exact: "route-only.test"   # HOST form per §11.8 amendment (NOT "https://route-only.test")
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.csrf
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy
                      filter_enabled:                                # MUST be 100% explicit per §1.1 amendment 3 / §11.11
                        runtime_key: __l_main_csrf_enabled
                        default_value: { numerator: 100, denominator: HUNDRED }
                      additional_origins:
                        - exact: "app.example.test"   # HOST form per §11.8 amendment (NOT "https://app.example.test")
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

The envoy-go.yaml mirrors the same listener with `STATIC` cluster type per the project convention. `filter_enabled` is PRESENT in envoy-go.yaml even though envoy-go silent-ignores its percentage value (per §2.1 cluster filter-enabled clause; §11.11 amendment) — the field presence ensures byte-equivalent config-load behavior (both Envoy and envoy-go validate presence at parse time; both run as effective-100%). `shadow_enabled` is OMITTED on both sides per §2.1.3.

### 7.4 Backend shape

Simple HTTP backend (`backend/main.go`) — equivalent to fixture 0011/0012/0013's backend per the 0011 + 0012 + 0013 precedent: ~30 LoC; one Go HTTP server listening on a driver-allocated port; 200 responses with a constant body marker (`backend\n` literal, mirroring 0011/0013's body); no special handling for `/route-only` (the csrf decision happens in Envoy/envoy-go before the upstream call; backend sees only the allowed requests).

### 7.5 Header allow-list extensions (inheriting phase 09 / 10 / 11 lessons)

Baseline proxy-injected headers (carry-forward from phases 09 / 10 / 11): `x-forwarded-for`, `x-forwarded-proto`, `x-request-id`, `x-envoy-*`.

Phase 12-specific:

- **Rejection path (scenarios 2, 4, 7b):** add `date` to per-scenario allow-list (not global) — same discipline as phase 09's fault abort response + phase 11's local_ratelimit 429 response.
- **Allow path (scenarios 1, 3, 5, 7a):** no additional headers added by the csrf filter on either side. Standard HCM/router headers (`server`, `date`, `x-envoy-*`) are unrelated to this filter.
- **`connection: close` on rejection path:** if reference Envoy injects `connection: close` under HTTP/1.1 hop-by-hop semantics during the rejection path, the allow-list adds it. PLAN author validates during impl; the empirical pin §11.10 did NOT observe this header on the 138-byte rejection wire trace (only the 4-header set was emitted, no connection: close). SPEC's position: NOT in allow-list; PLAN author adds if observed during fixture validation.

### 7.6 Differential gate scope clarification

Per ADR-0019, the differential equivalence claim covers the six scenarios above. It does NOT cover:

- Workloads outside the scenario specs (e.g. `Origin:` with non-RFC-3986 syntax that the URL parser handles differently between Go's `net/url` and C++'s `Http::Utility::Url::initialize`; explicit IPv6 literals; very long Origin strings that hit URL-parse buffer limits).
- Internal Envoy implementation details (CsrfPolicy proto representation; StringMatcher implementation choice).
- StringMatcher non-exact variants (deferred per §2.1.1 — fixture exclusively uses `exact`).
- `filter_enabled` percentage values other than 100% (deferred per §2.1.2 — fixture exclusively uses 100%).
- `shadow_enabled` semantics (deferred per §2.1.3 — fixture omits the field entirely).

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §1.1)

Phase 11 closed at ADR-0119. Phase 12 anticipates **5** ADRs (ADR-0120..ADR-0124), unchanged from BRAINSTORM §7's anticipated count:

| Slot | ID | Title |
|---|---|---|
| 1 | ADR-0120 | Filter package shape `internal/filter/http/csrf/` (single-token directory matching cors precedent + boot registration ordering `router → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`). 4-file split (`csrf.go`, `csrf_test.go`, `fuzz_test.go`, `doc.go`) with PLAN-author option to extract `origin.go` for readability. |
| 2 | ADR-0121 | `runtimeConfig` shape + 1-consumed/1-PGV-validated-not-honored/1-deferred field decomposition. AMENDED from BRAINSTORM Decision 3 per §1.1 amendment 3: `filter_enabled` is PGV-REQUIRED (cannot silent-ignore at parse time); validate presence + inner `default_value` non-nil at `New` time; silent-ignore the percentage value at runtime. `shadow_enabled` is OPTIONAL at parse + silent-ignored at runtime. `additional_origins[].StringMatcher.exact` non-empty values consumed; non-exact variants dropped at PARSE time per ADR-0101 §3 discipline (NOT match-time-keep-and-fail). |
| 3 | ADR-0122 | Origin extraction trichotomy (Origin: `null` literal → empty; Origin empty/absent → Referer fallback; Origin non-empty unparseable → verbatim string) + comparison algorithm (HOST:PORT-only equality; scheme stripped on both sides; NO normalization — case preserved, default ports preserved; trailing slash stripped via URL parser) + method gate (canonical 4-method set `{POST, PUT, DELETE, PATCH}`) + `additional_origins[].exact` matched against `host[:port]` form (NOT full URL with scheme — operator footgun). MAJOR AMENDMENT to BRAINSTORM Decisions 4 + 5 per §1.1 amendments 1 + 2. |
| 4 | ADR-0123 | Rejection path wire shape — `SendLocalReply(403)` + body byte-exact `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + `StopIteration` from `DecodeHeaders` — reuses fault.abort/local_ratelimit primitive. CONFIRMED unchanged from BRAINSTORM Decision 8 per §1.1 confirmation. |
| 5 | ADR-0124 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension for 3 `csrf.*` counters + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag) + drop `shadow_request_invalid` from MVP stat surface (couples to deferred shadow mode; reference Envoy also does not emit it under all-defaults config per §11.6 confirmation) + per-route stats SHARED with listener-level (per §11.9 amendment; no independent counter series per per-route entry — diverges from phase 11's local_ratelimit precedent; ADR-0073 wholesale-override applies as-is for data, stats simply not part of the override). |

NO omnibus ADR for deferrals (BRAINSTORM §8 already settles inline; deferrals live in BEHAVIOR_CONTRACT.md `### Phase 12 forward-pointer notes` per §13.4 below). NO amendment to ADR-0073 (per-route is data-only; the wholesale-override discipline applies as-is, with stats being shared rather than per-route-independent). NO amendment to ADR-0061 (no new SN flattening rule). NO amendment to ADR-0040 (silent-ignore set extension is mechanical, captured in ADR-0121).

### 8.1 Consolidation candidates

Per the phase 10 + phase 11 SPEC §8.1 precedents, the SPEC author may consolidate ADRs at SPEC time when the would-be standalone ADR is a thin documentation artefact rather than a load-bearing decision. Phase 12 has NO consolidation candidates — all 5 ADRs are load-bearing:

- **ADR-0120** captures the filter-package-shape continuity (single-token directory matching cors/fault precedent + boot registration ordering).
- **ADR-0121** captures the 1-consumed/1-PGV-validated-not-honored/1-deferred decomposition + the §1.1 amendment 3 (PGV-required `filter_enabled`).
- **ADR-0122** captures the §1.1 amendments 1 + 2 (host:port-only comparison + origin-extraction trichotomy + operator footgun) — substantively reshaped from BRAINSTORM hypothesis.
- **ADR-0123** captures the wire-shape (confirmed unchanged from BRAINSTORM).
- **ADR-0124** captures the stat-extension + the §1.1 amendment 4 (per-route stats shared).

Each pin maps to ≥1 ADR; each ADR cites the pin(s) it resolves. Anticipated count is firm at 5.

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106)

Per ADR-0106(b) (no-sibling-stub discipline), this SPEC does NOT pre-author SPEC stubs for siblings (`compression`, `global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts. The §9 heading at `ROADMAP.md` line 56 stays unchanged across phase 12's landing per ADR-0106(c). Phase 12's SPEC is the FIFTH validation of the §9 family-row discipline (phase 09 + 10 + 11 + 12 inheriting from cors @ 07.1 + first-row-establishment at 09 + second-iteration validation at 10 + third-iteration validation at 11 + fourth-iteration validation at 12).

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

### 10.1 Lifecycle correctness

- This SPEC author session lands SPEC.md (this file) + flips ROADMAP row 12 status `planned → in-progress` + advances STATE.md to `lifecycle-state: spec-complete` + sets `next-skill: superpowers:writing-plans` + updates `last-commit` SHA.
- Per the phase 09 + 10 + 11 SPEC-commit pattern, NO ADRs are added to `DECISIONS.md` at SPEC commit. ADRs ADR-0120..ADR-0124 land during impl commits per the per-task SHA-fill discipline.
- Per the phase 09 + 10 + 11 SPEC-commit pattern, NO `BEHAVIOR_CONTRACT.md` edits land at SPEC commit. The §13 4-edit bundle lands at the phase-done commit per ADR-0052.
- Per user memory + phase 09 + 10 + 11 precedent: SPEC commit lands on the `phase-12-http-filter-csrf-spec` worktree branch; ff-merge into master happens after PLAN + impl + REVIEW + phase-done commits all stack on the same branch (or successor branches per the user's worktree-per-stage preference).

### 10.2 Empirical-pin discipline

- §11 carries verbatim observations from reference Envoy v1.37.2 (image SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` per `ENVOY_TARGET.md`; probe date 2026-05-07 per the SPEC drafting machine's clock).
- All 11 §9 BRAINSTORM empirical pins resolved IN-SESSION per ADR-0004 hard-gate.
- §1.1 enumerates the 4 amendments (§11.3+§11.7+§11.8 collective host:port-only; §11.2 trichotomy; §11.11 PGV-required; §11.9 stat-sharing) + 3 confirmations (§11.6 no-shadow-counter; §11.6 tag-extractor reuse; §11.10 wire shape).

### 10.3 Scope envelope

- 1-consumed / 1-PGV-validated-not-honored / 1-deferred field map matches §2.1 amended decomposition (canonical short label per §1 item 1).
- Differential fixture has 6 scenarios (mirrors BRAINSTORM §6.2; no scenario drop, no scenario merge); scenario 6 (GET passthrough) unit-only.
- Stat surface 26→29 (3 new counters); table extension at §13.2; NO new tag-extractor (reuses HCM-namespace SN2).
- ADR list 5 (ADR-0120..ADR-0124); NO consolidation; NO ADR-0073 amendment paragraph; NO ADR-0061 amendment.
- Total LoC estimate ~750–1100; task count estimate ~10–14; both well below ADR-0045 split-trigger thresholds; phase stays single-row.

### 10.4 No 09 / 10 / 11-introduced regressions

- Phase 09's `fault` filter package: untouched.
- Phase 10's `header_mutation` filter package: untouched.
- Phase 10's framework deltas (`PerRouteConfig.ResolveAllTiers`, `RequestRouteConfigsAllTiers`, `ResponseRouteConfigsAllTiers`, `RegisterPerRouteValidator`): untouched and unused by phase 12. Their continued presence is expected.
- Phase 11's `localratelimit` filter package: untouched.
- Phase 11's framework deltas (`stats.Registry.NewCounterIfAbsent`): untouched (csrf uses the standard `NewCounter` path since per-route stats are SHARED with listener-level — no need for the post-Freeze-idempotent registration that local_ratelimit's per-route independent stats required).
- Phase 11's filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` (Rule SN9): untouched and not extended by phase 12 (csrf reuses HCM-namespace SN2 per §11.6).
- Existing 14 differential fixtures (0000–0013): untouched and expected to stay green at phase 12 phase-done.
- 15 existing fuzzers: untouched and expected to stay green at phase 12 phase-done.

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 11 pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 + phase 10 + phase 11 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1 / 08.2 / 09 / 10 / 11 SPEC §11 confirmation).

**Probe configuration:** Reference Envoy booted under per-pin minimal bootstrap YAMLs via `docker run --rm --network=host -v /tmp/p12-pins:/etc/envoy:ro envoyproxy/envoy:v1.37.2 -c /etc/envoy/<file>.yaml --base-id <unique>`; admin ports allocated 9911-9918, listener ports 11001-11008; HTTP backend (Python `BaseHTTPRequestHandler` header-echo) running in a sibling host-netns container at `127.0.0.1:18190`. Probe curl invocations issued from a sidecar `curlimages/curl:latest` container (also `--network=host`) so that the prober shares the same network namespace as the envoy listener. Capture transcripts at `/tmp/p12-pins/p*.{yaml,bin,txt}` of the SPEC drafting session machine are transient artifacts; the verbatim outputs below are the durable evidence per the 09 / 10 / 11 SPEC §11 discipline.

Source-of-truth cross-reference: `source/extensions/filters/http/csrf/csrf_filter.{cc,h}` at tag `v1.37.2`. Code fragments quoted in conclusions where load-bearing.

Probe date: **2026-05-07**.

### 11.1 Empirical pin #1 — Method gate (resolves BRAINSTORM §9.P1)

**Probe configuration:** `p1-method.yaml` — minimal csrf-only chain, `filter_enabled.default_value=100/HUNDRED`, NO `additional_origins`. Sent each method to `http://127.0.0.1:11001/` with NO `Origin` header. If csrf evaluates the method, an empty source_origin triggers `missing_source_origin` → 403.

**Verbatim per-method first-line responses:**

```
=== GET ===
HTTP/1.1 200 OK
=== HEAD ===
HTTP/1.1 200 OK
=== OPTIONS ===
HTTP/1.1 200 OK
=== TRACE ===
HTTP/1.1 200 OK
=== CONNECT ===
HTTP/1.1 400 Bad Request
content-length: 11
content-type: text/plain
date: Thu, 07 May 2026 20:03:46 GMT
server: envoy
connection: close

Bad Request
=== PROPFIND ===
HTTP/1.1 200 OK
=== POST ===
HTTP/1.1 403 Forbidden
content-length: 14
content-type: text/plain
date: Thu, 07 May 2026 20:03:47 GMT
server: envoy

Invalid origin
=== PUT ===
HTTP/1.1 403 Forbidden

Invalid origin
=== DELETE ===
HTTP/1.1 403 Forbidden

Invalid origin
=== PATCH ===
HTTP/1.1 403 Forbidden

Invalid origin
```

**Conclusions (pinned):**
- (a) **csrf evaluates EXACTLY `{POST, PUT, DELETE, PATCH}`.** `GET`, `HEAD`, `OPTIONS`, `TRACE`, `PROPFIND` skip the filter entirely (200 — passes through to backend even with no Origin).
- (b) `CONNECT` is rejected at the HCM level (400 Bad Request, body `"Bad Request"`, 11 bytes, `connection: close`) **before csrf is reached**. This is unrelated to csrf.
- (c) Source-of-truth confirms in `csrf_filter.cc isModifyMethod()`:
  ```
  return (method_type == method_values.Post || method_type == method_values.Put ||
          method_type == method_values.Delete || method_type == method_values.Patch);
  ```
  Initial BRAINSTORM expectation **CONFIRMED**.
- (d) envoy-go's csrf filter MUST gate exclusively on the 4-method set (case-sensitive uppercase string match against `:method`). PROPFIND and other WebDAV / non-RFC-7231 methods are NOT in the gate.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` method-gate paragraph (§13.1) and ADR-0122 §Decision section.

### 11.2 Empirical pin #2 — Origin parse-failure / null cases (resolves BRAINSTORM §9.P2)

**Probe configuration:** `p2-origin.yaml` — minimal csrf chain with `filter_enabled=100%`, NO `additional_origins`; default target_origin = `127.0.0.1:11002` (host:port form, derived from `Host: 127.0.0.1:11002`).

**Verbatim:**

```
=== A: Origin: null (literal string), no Referer ===
HTTP/1.1 403 Forbidden
content-length: 14

Invalid origin

=== B: Origin: <empty> (header sent with empty value), no Referer ===
HTTP/1.1 403 Forbidden

Invalid origin

=== C: Origin: not-a-url, no Referer ===
HTTP/1.1 403 Forbidden

Invalid origin

=== D: Origin: http://127.0.0.1:11002 (matches dest), no Referer ===
HTTP/1.1 200 OK

=== E: Origin: null + Referer: http://127.0.0.1:11002/ ===
HTTP/1.1 403 Forbidden

Invalid origin

=== F: Origin: <empty> + Referer: http://127.0.0.1:11002/ ===
HTTP/1.1 200 OK

=== G: NO Origin header + Referer: http://127.0.0.1:11002/ ===
HTTP/1.1 200 OK

=== H: Origin: not-a-url + Referer: http://127.0.0.1:11002/ ===
HTTP/1.1 403 Forbidden

Invalid origin

=== I: Origin: 'null' + matching Referer ===
HTTP/1.1 403 Forbidden

Invalid origin

=== J: Origin: example.com (no scheme) + matching Referer ===
HTTP/1.1 403 Forbidden

Invalid origin

=== K: Origin: http:// (truncated) + matching Referer ===
HTTP/1.1 403 Forbidden

Invalid origin
```

**Source-of-truth fragment** (`csrf_filter.cc sourceOriginValue`):

```
constexpr absl::string_view NullOrigin{"null"};
...
if (origin_value == NullOrigin) {
  return Envoy::EMPTY_STRING;
}
const auto origin = hostAndPort(origin_value);
if (!origin.empty()) {
  return origin;
}
return hostAndPort(headers.getInlineValue(referer_handle.handle()));
```

And `hostAndPort()`:

```
if (url.initialize(absolute_url, /*is_connect=*/false)) {
  return std::string(url.hostAndPort());
}
return std::string(absolute_url);  // parse failure -> return raw string verbatim
```

**Conclusions (pinned) — significant amendments to BRAINSTORM hypothesis:**
- (a) **`Origin: null` (literal 4-char string `"null"`) does NOT fall back to Referer.** It short-circuits to `EMPTY_STRING` directly, then increments `missing_source_origin` and rejects. BRAINSTORM hypothesized fallback; **AMENDED** — there is no fallback for the `null` literal.
- (b) **An empty `Origin:` header value DOES fall back to Referer** (probe F: empty + matching Referer → 200; probe B: empty without Referer → 403). The empty origin produces empty `hostAndPort` → falls into the `referer_handle` branch. BRAINSTORM hypothesis on empty-Origin fallback **CONFIRMED**.
- (c) **A header that is absent altogether (NO `Origin:`) DOES fall back to Referer** (probe G).
- (d) **Origin values that are non-empty non-null strings that fail URL parsing (`not-a-url`, `example.com`, `http://`) are treated as VERBATIM source-origin strings and compared against the destination/additional list** — they NEVER trigger Referer fallback (probes C, H, J, K all 403 even with matching Referer). The `hostAndPort()` helper returns the raw string when URL parse fails; that raw string then mismatches the dest hostAndPort and the request is rejected. **MAJOR AMENDMENT** to the BRAINSTORM "parse-failure → fallback" framing.
- (e) Counter implication: probes A/I/B/G all increment `missing_source_origin` ONLY for cases where `sourceOriginValue` returns `EMPTY_STRING`, i.e., (i) literal `Origin: null`, (ii) empty `Origin:` AND empty/missing Referer, (iii) NO `Origin:` AND empty/missing Referer. All other "parse-failure-looking" inputs are treated as a non-matching source_origin string → `request_invalid`.
- (f) envoy-go MUST replicate three distinct behaviors:
    1. `Origin: null` → empty source_origin (NO referer fallback, NO verbatim string).
    2. `Origin:` empty OR `Origin:` absent → empty hostAndPort → fall back to Referer's hostAndPort.
    3. `Origin:` non-empty, non-`null`, fails URL parse → verbatim string is the source_origin.
- (g) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` source-extraction paragraph (§13.1) and ADR-0122 §Decision section.

### 11.3 Empirical pin #3 — Destination scheme inference (resolves BRAINSTORM §9.P3)

**Probe configuration:** `p2-origin.yaml` — plaintext listener on `127.0.0.1:11002`, no TLS. Probes vary the source-Origin scheme/port and `X-Forwarded-Proto` header.

**Verbatim:**

```
=== A: Origin: https://127.0.0.1:11002 (cross-scheme; same host:port) ===
HTTP/1.1 200 OK

=== B: Origin: http://127.0.0.1:11002 (matching) ===
HTTP/1.1 200 OK

=== C: X-Forwarded-Proto: https + Origin: https://127.0.0.1:11002 ===
HTTP/1.1 200 OK

=== D: X-Forwarded-Proto: https + Origin: http://127.0.0.1:11002 ===
HTTP/1.1 200 OK

=== F: Origin: ftp://127.0.0.1:11002 (utterly different scheme) ===
HTTP/1.1 200 OK

=== G: Origin: //127.0.0.1:11002 (no scheme, has authority — URL parse fails, returns verbatim "//127.0.0.1:11002") ===
HTTP/1.1 403 Forbidden

=== K: Origin: 127.0.0.1:11002 (host:port only — URL parse fails, returns verbatim) ===
HTTP/1.1 200 OK

=== H: Origin: http://127.0.0.1:99 (port mismatch) ===
HTTP/1.1 403 Forbidden

=== J: Origin: gibberish://127.0.0.1:11002 (unknown scheme) ===
HTTP/1.1 200 OK
```

**Source-of-truth fragment** (`csrf_filter.cc targetOriginValue`):

```
const auto absolute_url = fmt::format(
    "{}://{}", headers.Scheme() != nullptr ? headers.getSchemeValue() : "http", host_value);
return hostAndPort(absolute_url);
```

The scheme is computed (from `:scheme` pseudo-header — set by HCM based on listener TLS state) but immediately discarded by `hostAndPort()`, which keeps only the `host[:port]` portion of the URL.

**Conclusions (pinned) — significant amendments:**
- (a) **The csrf comparison is HOST:PORT-ONLY on both sides; scheme is NOT enforced.** The `:scheme` pseudo-header is consumed only to make the URL parseable; `hostAndPort()` then strips it. BRAINSTORM hypothesis ("scheme comes from listener TLS state") is technically correct as to where `:scheme` originates but irrelevant — **the scheme never enters the equality check**. **MAJOR AMENDMENT.**
- (b) **`X-Forwarded-Proto` has NO effect** on csrf. Probes C, D show identical results to A, B. BRAINSTORM hypothesis "scheme NOT from X-Forwarded-Proto" **CONFIRMED**, AND additionally pinned: scheme isn't compared at all.
- (c) URL parsing is handled by `Http::Utility::Url::initialize` which accepts arbitrary scheme tokens. `ftp://`, `gibberish://`, and any other `<scheme>://...` parse successfully and yield the same `hostAndPort`. Source-Origin strings WITHOUT a scheme (`//127.0.0.1:11002`, `127.0.0.1`) fail URL parsing and are returned verbatim — `127.0.0.1:11002` happens to match the dest hostAndPort verbatim (probe K=200), while `//127.0.0.1:11002` doesn't (literally `//127.0.0.1:11002` ≠ `127.0.0.1:11002`).
- (d) **envoy-go's csrf filter MUST replicate scheme-stripping**: parse the source Origin URL, take `host:port`, compare against `host:port` derived from `Host:` header alone (no scheme prefix needed in target_origin equivalence). Mirror this verbatim — do NOT add a scheme equality check.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` comparison-algorithm paragraph (§13.1) and ADR-0122 §Decision section.

### 11.4 Empirical pin #4 — Rejection status code (resolves BRAINSTORM §9.P4)

Captured implicitly across §11.1, §11.2, §11.5, and §11.10. Verbatim status-line evidence inlined at §11.1 (`HTTP/1.1 403 Forbidden` for `POST`/`PUT`/`DELETE`/`PATCH` cases) + §11.2 (probes A/B/C/E/H/I/J/K all show `HTTP/1.1 403 Forbidden`) + §11.10's full hex dump (offset 0x00-0x0d shows `4854 5450 2f31 2e31 2034 3033 2046 6f72 6269 6464 656e` = ASCII `HTTP/1.1 403 Forbidden`).

**Conclusion:** **403 Forbidden CONFIRMED.** Source: `callbacks_->sendLocalReply(Http::Code::Forbidden, "Invalid origin", ...)`. Status code is hardcoded — there is no proto field to override (in contrast to local_ratelimit's `status.code`).

Lands in ADR-0123 §Decision section.

### 11.5 Empirical pin #5 — Rejection body bytes (resolves BRAINSTORM §9.P5)

**Probe configuration:** `p2-origin.yaml` — cross-origin POST.

**Verbatim hex of body alone (14 bytes; MD5 = `7433f3a046afcebee10e455dd26b0eb6`):**

```
00000000: 496e 7661 6c69 6420 6f72 6967 696e       Invalid origin
```

Last byte = `0x6e` ('n'). **No trailing newline.**

**Source-of-truth:** `sendLocalReply(Http::Code::Forbidden, "Invalid origin", nullptr, absl::nullopt, RcDetails::get().OriginMismatch);` — the literal `"Invalid origin"` is passed as the body string. `RcDetails::OriginMismatch = "csrf_origin_mismatch"` (used as the `response_code_details` for access logs, NOT in the wire body).

**Conclusions (pinned):**
- (a) Body literal: **`Invalid origin`** (14 bytes; ASCII; no trailing newline). Initial BRAINSTORM expectation **CONFIRMED**.
- (b) envoy-go MUST emit byte-equivalent `Invalid origin` (14 bytes, last byte 'n', no LF).
- (c) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` rejection-body paragraph (§13.1) and ADR-0123 §Decision section.

### 11.6 Empirical pin #6 — Stats Prometheus form (resolves BRAINSTORM §9.P6)

**Probe configuration:** `p2-origin.yaml` (HCM stat_prefix=`hcm`, csrf default policy 100% enabled, NO additional_origins). Sequence: 3× same-origin POST (request_valid) + 2× cross-origin POST (request_invalid) + 2× no-Origin POST (missing_source_origin).

**Verbatim Prometheus output (post-load):**

```
# TYPE envoy_http_csrf_missing_source_origin counter
envoy_http_csrf_missing_source_origin{envoy_http_conn_manager_prefix="hcm"} 2
# TYPE envoy_http_csrf_request_invalid counter
envoy_http_csrf_request_invalid{envoy_http_conn_manager_prefix="hcm"} 2
# TYPE envoy_http_csrf_request_valid counter
envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="hcm"} 3
```

**Verbatim text-format `/stats`:**

```
http.hcm.csrf.missing_source_origin: 2
http.hcm.csrf.request_invalid: 2
http.hcm.csrf.request_valid: 3
```

**Source-of-truth (`csrf_filter.h`):**

```
#define ALL_CSRF_STATS(COUNTER)         \
  COUNTER(missing_source_origin)        \
  COUNTER(request_invalid)              \
  COUNTER(request_valid)
```

And `csrf_filter.cc generateStats`: `const std::string final_prefix = prefix + "csrf.";`.

**Conclusions (pinned):**
- (a) **Exactly 3 counters per HCM scope.** Names (lexicographic): `missing_source_origin`, `request_invalid`, `request_valid`. NO `*_shadow` counter family — shadow-only mode shares the same counters (validated via §11.11 below).
- (b) **Prometheus label key: `envoy_http_conn_manager_prefix`** with value = the HCM `stat_prefix` field. BRAINSTORM hypothesis **CONFIRMED**. Note this matches the standard HCM tag-extractor (NOT a csrf-filter-specific tag-extractor, in contrast to phase 11's local_ratelimit `envoy_local_http_ratelimit_prefix`).
- (c) Text-format template: `http.<hcm-stat-prefix>.csrf.{missing_source_origin, request_invalid, request_valid}`.
- (d) Prometheus-format template: `envoy_http_csrf_{missing_source_origin, request_invalid, request_valid}{envoy_http_conn_manager_prefix="<hcm-stat-prefix>"}`.
- (e) **No `shadow_request_invalid` is emitted.** When `filter_enabled=0%` and `shadow_enabled=100%`, the regular `request_invalid` counter increments instead (see §11.11). Under all-defaults configuration, only the 3 counters appear.
- (f) When BOTH `filter_enabled=0%` AND `shadow_enabled` absent (or 0%), the filter short-circuits at `decodeHeaders` entry **before any counter increment** — `csrf.*` stats stay at 0 (validated via §11.11 zero-percent probe).
- (g) envoy-go's `internal/admin/stats.go` tag-extractor table needs NO new entry for csrf — it reuses the existing HCM stat-prefix extractor pattern (Rule SN2 from ADR-0061). Confirmed.
- (h) Lands in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29 table (§13.2) and ADR-0124 §Decision section.

### 11.7 Empirical pin #7 — Origin normalization (resolves BRAINSTORM §9.P7)

**Probe configuration:** `p7-norm.yaml` with `additional_origins: [{exact: "app.example.test"}, {exact: "https://withscheme.example.test"}]`.

**Verbatim:**

```
additional_origins=[exact:'app.example.test', exact:'https://withscheme.example.test']

=== A1: Origin: https://app.example.test                  -> 200 OK
=== A2: Origin: HTTPS://APP.EXAMPLE.TEST                  -> 403 Forbidden
=== A3: Origin: https://APP.EXAMPLE.TEST                  -> 403 Forbidden
=== A4: Origin: https://app.example.test:443              -> 403 Forbidden
=== A5: Origin: http://app.example.test:80                -> 403 Forbidden
=== A6: Origin: https://app.example.test:8443             -> 403 Forbidden
=== A7: Origin: https://app.example.test/                 -> 200 OK   (trailing slash stripped)

=== B1: Origin: https://withscheme.example.test           -> 403 Forbidden
       (host-only "withscheme.example.test" doesn't match exact "https://withscheme.example.test")
=== B2: Origin: withscheme.example.test                   -> 403 Forbidden  (URL parse fails, verbatim "withscheme.example.test" doesn't match "https://withscheme.example.test")
=== B3: Origin: https://withscheme.example.test:443       -> 403 Forbidden
```

**Supplementary probe (`p7b-norm.yaml`, `additional_origins=[exact:"app.example.test:443", exact:"app.example.test:8443"]`):**

```
=== A: Origin: https://app.example.test:443       -> 200 OK    (port literally 443 preserved)
=== B: Origin: https://app.example.test           -> 403 Forbidden  (no port doesn't match :443 entry)
=== C: Origin: https://app.example.test:8443      -> 200 OK
```

**Source-of-truth:** `Http::Utility::Url::initialize` performs RFC-3986-conformant URL parsing — it preserves authority verbatim (no case folding on the host, no default-port stripping). `hostAndPort()` returns the raw `host[:port]` substring.

**Conclusions (pinned) — major amendments:**
- (a) **NO case normalization.** Uppercase scheme/host strings produce a different `hostAndPort()` and fail to match. envoy-go MUST NOT lowercase. BRAINSTORM hypothesis **AMENDED**.
- (b) **NO default-port stripping.** `https://app.example.test:443` yields `app.example.test:443` (NOT stripped to `app.example.test`); `http://app.example.test:80` yields `app.example.test:80`. To support implicit-default-port matching, the operator must explicitly add both port-suffixed and bare entries. BRAINSTORM hypothesis **AMENDED**.
- (c) **Trailing slash IS stripped** (path component is dropped) — A7 → 200. The URL parser yields hostAndPort cleanly.
- (d) **`additional_origins` matches against `hostAndPort(source_origin)` — NOT the full origin string with scheme.** Per source: `additional_origin->match(source_origin)` where `source_origin = hostAndPort(...)`. This means an entry like `exact: "https://app.example.test"` will NEVER match a real `Origin:` header — the operator must write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (host + port). **MAJOR PIN AMENDMENT to BRAINSTORM** "additional_origins matches full-origin string". envoy-go SPEC must document this precisely; it is a known operator footgun.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` comparison-algorithm + operator-footgun paragraphs (§13.1) and ADR-0122 §Decision section.

### 11.8 Empirical pin #8 — `additional_origins` matching target (resolves BRAINSTORM §9.P8)

**Probe configuration:** `p8-match.yaml` with `additional_origins: [{exact: "app.example.test"}]`.

**Verbatim:**

```
=== A: Origin: http://app.example.test       -> 200 OK   (scheme http; host:port -> "app.example.test" matches)
=== B: Origin: https://app.example.test      -> 200 OK   (scheme https; host:port -> "app.example.test" matches)
=== C: Origin: https://app.example.test:8443 -> 403 Forbidden  (host:port -> "app.example.test:8443" mismatches)
=== D: Origin: https://other.test            -> 403 Forbidden
```

**Conclusions (pinned):**
- (a) **`additional_origins` matches the `hostAndPort` form of the source origin** (i.e., `host[:port]`, NO scheme prefix). Scheme is IGNORED on the source side.
- (b) **Port presence in the Origin string IS significant** — `https://app.example.test` (no port → hostAndPort=`app.example.test`) MATCHES `exact: "app.example.test"`, but `https://app.example.test:8443` does NOT (hostAndPort=`app.example.test:8443`).
- (c) **The matching predicate is the full Envoy `StringMatcher` machinery** (`Matchers::StringMatcherImpl`) — operators may use `exact`, `prefix`, `suffix`, `contains`, `safe_regex`, etc. The phase 12 fixture sticks to `exact` for the differential.
- (d) BRAINSTORM hypothesis "full-origin string `<scheme>://<host>[:<port>]`" is **AMENDED** to "host[:port], no scheme".
- (e) envoy-go's csrf filter MUST run StringMatcher predicates against the `host:port` form, NOT `scheme://host:port`. For envoy-go MVP, the surviving exact entries are stored verbatim as host:port strings; the runtime comparison is byte-equality between the source's `hostAndPort` and each compiled exact entry.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` additional-origins-matching paragraph (§13.1) and ADR-0122 §Decision section.

### 11.9 Empirical pin #9 — Per-route TPFC wholesale-override (resolves BRAINSTORM §9.P9)

**Probe configuration:** `p9-route.yaml`. Listener-level: `additional_origins=[A.test]`. Route `/route` per-route TPFC: `additional_origins=[B.test]`. Default route `/`: no override (inherits listener).

**Verbatim:**

```
=== A: POST /route Origin: https://A.test   -> 403 Forbidden  (per-route REPLACES listener; A.test is not allowed on /route)
=== B: POST /route Origin: https://B.test   -> 200 OK         (per-route allows B.test on /route)
=== C: POST /     Origin: https://B.test    -> 403 Forbidden  (default route uses listener; B.test is not in listener allow-list)
=== D: POST /     Origin: https://A.test    -> 200 OK         (default route uses listener; A.test allowed)

=== prometheus (after 4 reqs) ===
envoy_http_csrf_missing_source_origin{envoy_http_conn_manager_prefix="hcm"} 0
envoy_http_csrf_request_invalid{envoy_http_conn_manager_prefix="hcm"} 2
envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="hcm"} 2
```

**Source-of-truth (`csrf_filter.cc determinePolicy`):**

```
void CsrfFilter::determinePolicy() {
  const CsrfPolicy* policy = Http::Utility::resolveMostSpecificPerFilterConfig<CsrfPolicy>(callbacks_);
  if (policy != nullptr) {
    policy_ = policy;        // wholesale replace
  } else {
    policy_ = config_->policy();
  }
}
```

**Conclusions (pinned):**
- (a) **Wholesale-override CONFIRMED.** The per-route `CsrfPolicy` ENTIRELY REPLACES the listener `CsrfPolicy` for matched requests; there is no field-level merging of `additional_origins`, `filter_enabled`, or `shadow_enabled`.
- (b) Default routes (no TPFC) fall through to listener config — confirmed by C/D.
- (c) **Stats are SHARED across listener and per-route policies** within the same HCM scope. The `CsrfFilterConfig` (which owns `stats_`) is constructed once per HCM filter chain; the per-route override is a `CsrfPolicy` (the `Router::RouteSpecificFilterConfig` subclass) which does NOT carry its own stats. The 2 `request_valid` and 2 `request_invalid` increments are the SUM of /route and / requests — there is no independent per-route counter series. Contrast phase 11 (local_ratelimit) where per-route `stat_prefix` yields independent series. **DIVERGENCE FROM PHASE 11 PRECEDENT** that envoy-go must replicate.
- (d) envoy-go's csrf filter implementation MUST mirror this: per-route `runtimeConfig` carries only the `CsrfPolicy` fields (no `*csrfStats`), and stat increments always go to the listener-level `*csrfStats` registered for the HCM scope.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` per-route-stats-shared paragraph (§13.1) and ADR-0124 §Decision section.

### 11.10 Empirical pin #10 — Header set on rejection (resolves BRAINSTORM §9.P10)

**Probe configuration:** `p2-origin.yaml`. Cross-origin POST → captured full wire bytes via `curl -is --output -`.

**Verbatim full-wire hex (138 bytes total):**

```
00000000: 4854 5450 2f31 2e31 2034 3033 2046 6f72  HTTP/1.1 403 For
00000010: 6269 6464 656e 0d0a 636f 6e74 656e 742d  bidden..content-
00000020: 6c65 6e67 7468 3a20 3134 0d0a 636f 6e74  length: 14..cont
00000030: 656e 742d 7479 7065 3a20 7465 7874 2f70  ent-type: text/p
00000040: 6c61 696e 0d0a 6461 7465 3a20 5468 752c  lain..date: Thu,
00000050: 2030 3720 4d61 7920 3230 3236 2032 303a   07 May 2026 20:
00000060: 3036 3a34 3120 474d 540d 0a73 6572 7665  06:41 GMT..serve
00000070: 723a 2065 6e76 6f79 0d0a 0d0a 496e 7661  r: envoy....Inva
00000080: 6c69 6420 6f72 6967 696e                 lid origin
```

**Headers in EMISSION ORDER (lexicographic):**
1. `content-length: 14`
2. `content-type: text/plain`
3. `date: Thu, 07 May 2026 20:06:41 GMT`
4. `server: envoy`

**Conclusions (pinned):**
- (a) Status line: `HTTP/1.1 403 Forbidden\r\n`. Status text `Forbidden` (RFC 7231).
- (b) Headers in EMISSION ORDER (lexicographic): `content-length`, `content-type`, `date`, `server`. All lowercase wire-form.
- (c) `content-length: 14` (NOT `transfer-encoding: chunked`). Body is sent in a single Content-Length-framed write.
- (d) `content-type: text/plain` (**NO `; charset=UTF-8` modifier**). Initial BRAINSTORM expectation **CONFIRMED**.
- (e) `server: envoy` — matches phase 09 / 10 / 11 precedent. envoy-go's existing HCM `serverHeader()` returning `"envoy"` produces byte-equivalent output without modification.
- (f) **NO `cache-control`, NO `x-content-type-options: nosniff`, NO `transfer-encoding`** on the rejection (in contrast to admin handlers like `/ready`). The rejection wire shape is identical to phase 11's 429 wire shape modulo the status line and body literal.
- (g) Header/body separator: `\r\n\r\n`.
- (h) envoy-go's csrf filter calls into `SendLocalReply` (or its envoy-go equivalent) which emits the canonical 4-header set in lexicographic order. No csrf-specific header customization required.
- (i) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` rejection-wire-shape paragraph (§13.1) and ADR-0123 §Decision section.

### 11.11 Empirical pin #11 — `filter_enabled` runtime default with field UNSET (resolves BRAINSTORM §9.P11)

**Probe configuration #1 (UNSET):** `p11a-unset.yaml` — `CsrfPolicy` with `additional_origins: [{exact: "app.example.test"}]` and **NO `filter_enabled` field at all**.

**Verbatim boot-failure (PGV) — `--mode validate`:**

```
[2026-05-07 20:09:02.659][1][critical][main] [source/server/config_validation/server.cc:76] error initializing configuration '/etc/envoy/p11a-unset.yaml': goo.gle/debugonly
additional_origins {
  exact: "app.example.test"
}
: Proto constraint validation failed (CsrfPolicyValidationError.FilterEnabled: value is required)
```

**Probe configuration #2 (`filter_enabled: {}` empty):** `p11b-empty.yaml` — same but `filter_enabled: {}` (no `default_value`).

**Verbatim:**

```
[2026-05-07 20:09:14.132][1][critical][main] [source/server/config_validation/server.cc:76] error initializing configuration '/etc/envoy/p11b-empty.yaml': goo.gle/debugonly
filter_enabled {
}
additional_origins {
  exact: "app.example.test"
}
: Proto constraint validation failed (CsrfPolicyValidationError.FilterEnabled: embedded message failed validation | caused by RuntimeFractionalPercentValidationError.DefaultValue: value is required)
```

**Probe configuration #3 (0% — boots):** `p11c-zero.yaml` — `filter_enabled.default_value=0/HUNDRED`. Boots successfully. Cross-origin POST → 200 OK (passes through). Stats: all 3 counters at 0.

**Verbatim:**

```
=== POST cross-origin Origin: https://evil.test ===
HTTP/1.1 200 OK
server: envoy
date: Thu, 07 May 2026 20:09:29 GMT

envoy_http_csrf_missing_source_origin{envoy_http_conn_manager_prefix="hcm"} 0
envoy_http_csrf_request_invalid{envoy_http_conn_manager_prefix="hcm"} 0
envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="hcm"} 0
```

**Probe configuration #4 (filter_enabled=0%, shadow_enabled=100%):** `p11d-shadow.yaml`.

**Verbatim:**

```
=== POST cross-origin    -> 200 OK  (passes through)
=== POST same-origin     -> 200 OK
=== POST no-Origin       -> 200 OK

envoy_http_csrf_missing_source_origin{envoy_http_conn_manager_prefix="hcm"} 1
envoy_http_csrf_request_invalid{envoy_http_conn_manager_prefix="hcm"} 1
envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="hcm"} 1
```

**Source-of-truth (`csrf_filter.cc decodeHeaders`):**

```
if (!policy_->enabled() && !policy_->shadowEnabled()) {
  return Http::FilterHeadersStatus::Continue;  // bypass: NO stat increments
}
...
if (policy_->shadowEnabled() && !policy_->enabled()) {
  return Http::FilterHeadersStatus::Continue;  // shadow-only: stats incremented above, but no rejection
}
callbacks_->sendLocalReply(Http::Code::Forbidden, "Invalid origin", ...);
```

**Conclusions (pinned) — significant pin clarifications:**
- (a) **`filter_enabled` is REQUIRED at PGV.** It cannot be omitted (probe #1 → boot fails) and its inner `default_value` cannot be omitted either (probe #2 → boot fails). The "trap" anticipated in BRAINSTORM (UNSET→active vs UNSET→inactive) **does NOT apply** — the user must always set the field. **AMENDED**.
- (b) PGV constraint names:
    1. `CsrfPolicyValidationError.FilterEnabled: value is required` — when the field itself is omitted.
    2. `RuntimeFractionalPercentValidationError.DefaultValue: value is required` — when `filter_enabled` is present but `default_value` is omitted.
- (c) **`filter_enabled.default_value=0/HUNDRED` boots successfully** and disables the filter — all 3 counters stay at 0 (the filter short-circuits BEFORE incrementing).
- (d) **`shadow_enabled` IS optional** (probe #3 boots fine without it). When omitted, `policy_.has_shadow_enabled()` is false and `shadowEnabled()` returns false.
- (e) **Shadow-only mode (filter_enabled=0%, shadow_enabled=100%)**: stats increment normally (request_valid / request_invalid / missing_source_origin) but no 403 is emitted. The same 3-counter family is used — there is NO separate `*_shadow` family.
- (f) envoy-go MUST validate at parse-time:
    1. `cfg.FilterEnabled != nil && cfg.FilterEnabled.DefaultValue != nil` (REQUIRED).
    2. `cfg.ShadowEnabled` may be nil (treated as `shadow_enabled` disabled).
- (g) **Phase 12 fixture decision (compelled by the PGV REQUIRED constraint):** `filter_enabled.default_value: 100/HUNDRED` MUST be set explicitly on every csrf filter instance in the differential fixture (listener-level AND any per-route TPFC instances). NO config can omit `filter_enabled`. This is the OPPOSITE of phase 11's outcome (where local_ratelimit's `filter_enabled` was OPTIONAL with an UNSET-trap defaulting to inactive).
- (h) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` deferred-fields paragraph (§13.1) and ADR-0121 §Decision section + ADR-0124 §Decision section (shadow-mode counter-sharing).

### 11.12 Synchronization with `BEHAVIOR_CONTRACT.md`

The 11-pin empirical block above is the source of truth for the §13 4-edit bundle (which lands at the phase-done commit per ADR-0052):

- §13.1 NEW subsection draws from §11.1 (method gate), §11.2 (origin trichotomy), §11.3 (scheme stripping), §11.5 (rejection body), §11.7 (no normalization + operator footgun), §11.8 (additional_origins host:port matching), §11.9 (per-route stats shared), §11.10 (rejection header set), §11.11 (filter_enabled PGV-required + shadow_enabled optional).
- §13.2 26→29 stat-table extension draws from §11.6 (3 counter names + reuse of `envoy_http_conn_manager_prefix` extractor).
- §13.3 NEW equivalence-matrix row draws from §11.10, §11.9, §11.11 + the fixture 0014 6-scenario topology.
- §13.4 NEW forward-pointer notes subsection draws from the 3-item deferral list (§2.1.1 / §2.1.2 / §2.1.3) + the §11.11 PGV-required note.

---

## 12. Deferred decisions (the planner / implementer settles these)

The following 4 decisions are SPEC-deferred — the SPEC author has bounded each but leaves the precise discipline for the PLAN author or impl-time settlement. Each maps to ≤1 ADR (some fold inline into existing ADRs).

**D1. Filter-callback wiring.** §6.3 declares phase 12 sets only `dcb` (DecoderFilterCallbacks); the precise hook (`OnNewStream`, factory closure, etc.) follows the existing 07.1 framework convention. PLAN author confirms the framework's exposed callback-setup hook against the existing `internal/filter/http/cors/`, `fault/`, `header_mutation/`, `localratelimit/` patterns.

**D2. URL-parse semantics for `hostAndPort()`.** §6.4 declares envoy-go uses `net/url.Parse` to extract `hostAndPort` from Origin / Referer / target URLs, falling back to verbatim string on parse failure. PLAN author spot-checks against Envoy's `Http::Utility::Url::initialize` for edge cases: `//host:port` (no scheme), `127.0.0.1:port` (no `://`), IPv6 literal hosts `[::1]:8080`, embedded user-info `user:pass@host`, and very long Origin strings. If any edge case produces a divergent `hostAndPort`, PLAN author either (a) writes a custom `hostAndPort()` helper that matches Envoy's behavior verbatim, or (b) records the divergence in BEHAVIOR_CONTRACT §13.1's "edge-case fidelity" paragraph + adds an ADR documenting the deliberate divergence. Default expected outcome: `net/url.Parse` matches `Http::Utility::Url::initialize` for all common cases.

**D3. Filter-internal validation error message wording for `filter_enabled` PGV-mirror.** §6.1 + §11.11 establish that envoy-go's `New` factory rejects `cfg.FilterEnabled == nil` and `cfg.FilterEnabled.DefaultValue == nil` at parse time. The exact error message wording is PLAN-author-resolved: either (a) mirror Envoy's PGV envelope ("CsrfPolicyValidationError.FilterEnabled: value is required") for diagnostic parity, or (b) emit envoy-go's own clear-text wording ("csrf: filter_enabled is required"). Phase 11's ADR-0115 used option (b) for the 50ms fill_interval validation; phase 12 likely follows the same precedent. Captured in ADR-0121.

**D4. Per-route stats wiring mechanism.** §6.6 declares per-route `runtimeConfig` SHARES the listener-level `*filterStats` pointer. The precise wiring mechanism is open: (a) the per-route `New` is called with a context carrying the listener-level `*filterStats` pointer; (b) per-route `runtimeConfig` is built via a separate `parsePerRouteConfig` helper that takes an existing `*filterStats` argument; (c) per-route `New` re-parses the proto and re-registers counters using `NewCounterIfAbsent` (the phase 11 idempotent-registration primitive — would silently return the existing counter pointer per `*stats.Registry`'s LBP-1 amendment in ADR-0118). Option (c) requires no framework extension and is the simplest; option (a) is the cleanest but requires a small `FactoryCtx` extension; option (b) requires a new exported helper from the csrf package. PLAN author chooses based on existing per-route-builder discipline in `internal/filter/http/perroute.go`. Captured in ADR-0124.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

The 4-edit bundle below is the verbatim Markdown patch applied to `docs/envoy-go/BEHAVIOR_CONTRACT.md` at the phase-done commit. NOT applied at SPEC commit (per phase 09 / 10 / 11 precedent).

### 13.1 `## HTTP filter chain ### envoy.filters.http.csrf` NEW subsection

The new subsection lands UNDER the existing `## HTTP filter chain` umbrella, AFTER the existing `### envoy.filters.http.fault` (phase 09), `### envoy.filters.http.header_mutation` (phase 10), and `### envoy.filters.http.local_ratelimit` (phase 11) subsections. Verbatim Markdown shape:

```markdown
### envoy.filters.http.csrf

Phase 12 ships `envoy.filters.http.csrf` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes 1 of 3 top-level fields actively, validates 1 at parse-time but silent-ignores its runtime value, and silent-ignores 1 entirely.

**Field decomposition (3 fields):**

| Proto field | envoy-go behavior |
|---|---|
| `additional_origins` | CONSUMED. Repeated `StringMatcher`. Only `exact` variant with non-empty value is honored; non-exact variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) are dropped at PARSE time per ADR-0101 §3 discipline. Empty-value `exact` entries are also dropped. |
| `filter_enabled` | PGV-VALIDATED at parse-time (per phase 12 SPEC §11.11): envoy-go's `New` factory rejects with a non-nil error if the field is nil OR if its inner `default_value` is nil — mirroring Envoy's PGV envelope. SILENT-IGNORED at runtime: the percentage value is read but not consulted; the filter always evaluates as if 100%-active. Couples to deferred Runtime + hot restart family. |
| `shadow_enabled` | OPTIONAL at parse-time (Envoy permits omission); SILENT-IGNORED at runtime; always-never-shadow regardless of proto value. Couples to deferred Runtime + hot restart family. |

**Method gate:** csrf evaluates only modifying-method requests `{POST, PUT, DELETE, PATCH}` (case-sensitive uppercase string match against `:method`). Non-modifying methods (`GET`, `HEAD`, `OPTIONS`, `TRACE`, `CONNECT`, `PROPFIND`, custom verbs) short-circuit to `Continue` BEFORE any state touch — no counter increment, no origin parse. CONNECT may be rejected at the HCM level (400 Bad Request) before csrf is reached; this is unrelated to csrf. (Per phase 12 SPEC §11.1.)

**Origin extraction trichotomy** (per phase 12 SPEC §11.2):

1. `Origin: null` (literal 4-char string `"null"`) → empty source_origin → `missing_source_origin` counter increment → reject. NO Referer fallback.
2. `Origin:` empty value OR `Origin:` header absent → empty `hostAndPort()` → fall back to Referer's `hostAndPort()`. If Referer also yields empty `hostAndPort()` → empty source_origin → `missing_source_origin` → reject.
3. `Origin:` non-empty, non-`null`, BUT URL parse fails (e.g., `Origin: not-a-url`) → return the verbatim raw string as source_origin. NO Referer fallback. The verbatim string almost always rejects (since it mismatches the target's `hostAndPort` and any `additional_origins[].exact` entry — unless an entry happens to equal that exact verbatim string).

**Comparison algorithm — HOST:PORT-ONLY equality** (per phase 12 SPEC §11.3 / §11.7 / §11.8):

- Source `hostAndPort` is computed via URL parse of the `Origin:` (or `Referer:`) value; if parse succeeds, the result is `host[:port]`. If parse fails, the verbatim raw string is used.
- Target `hostAndPort` is computed via URL parse of `<scheme>://<:authority>`, where `<scheme>` is the request's `:scheme` pseudo-header (set by HCM from listener TLS state) and `<:authority>` is the `:authority` pseudo-header (HTTP/2) or `Host` header (HTTP/1.1). The scheme is consumed only to make the URL parseable; `hostAndPort()` strips it.
- Equality is byte-exact between the two `host[:port]` strings.
- **NO case folding.** `https://APP.EXAMPLE.TEST` does NOT match `app.example.test`. Operators MUST author configs in the lowercase form they expect Origin headers to carry.
- **NO default-port stripping.** `https://app.example.test:443` does NOT match `app.example.test`. To support implicit-default-port equivalence, operators must explicitly add both port-suffixed and bare entries to `additional_origins`.
- **Trailing slash IS stripped** (path component dropped via URL parser). `https://app.example.test/` yields `hostAndPort = app.example.test`.
- **`X-Forwarded-Proto` is irrelevant.** Scheme is computed only for URL parsing; its value is stripped before equality.

**Operator footgun (per phase 12 SPEC §11.7 + §11.8):** `additional_origins[].exact` is matched against the source's `host[:port]` form — NOT the full URL with scheme. Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header, because the source's `hostAndPort` is `app.example.test` (not `https://app.example.test`). Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port); DO NOT include the scheme prefix. envoy-go faithfully replicates Envoy's behavior; this is a known footgun in the upstream spec.

**Per-route override semantics:** Wholesale-override per ADR-0073. Each `CsrfPolicy` TPFC entry runs through `New` at config-load time, allocating its own `*runtimeConfig` with its own compiled `[]additionalOrigins` slice. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration) picks the most-specific config per request. Listener-level data is NOT touched for per-route reqs.

**Per-route stats are SHARED with listener-level** (per phase 12 SPEC §11.9; **diverges from phase 11 local_ratelimit precedent**). The per-route `runtimeConfig` carries only the `additional_origins` data; stat counter increments always go to the listener-level `*filterStats` registered for the HCM scope. There is exactly ONE counter series per HCM stat_prefix per counter, regardless of how many per-route TPFC entries exist. Confirmed empirically: 4 requests across listener-level (`/`) and per-route (`/route`) increment the counters as a SUM (e.g., 2 valid + 2 invalid total, NOT split into separate series).

**Rejection response wire shape (per phase 12 SPEC §11.10 empirical):**

- Status: `403 Forbidden`
- Body: `Invalid origin` (14 bytes ASCII; NO trailing newline)
- Headers in lexicographic order: `content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Framing: Content-Length (NO chunked)
- NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding`, NO `charset=UTF-8` modifier on `content-type`

**Allow-path response (per phase 12 SPEC §11.6 empirical):** NO csrf-specific headers added on either side (request or response). Standard HCM/router headers (`server`, `date`, `x-envoy-*`) are unrelated to this filter.

**Stat surface (3 counters per HCM scope):**

- `http.<HCM stat_prefix>.csrf.request_valid` — incremented when modifying-method request's source origin matches target OR any `additional_origins[].exact`.
- `http.<HCM stat_prefix>.csrf.request_invalid` — incremented when source origin is determinable but matches neither.
- `http.<HCM stat_prefix>.csrf.missing_source_origin` — incremented when source origin is undeterminable (Origin: null literal, OR both Origin and Referer missing/yield-empty-hostAndPort).

NO `shadow_request_invalid` counter — confirmed reference Envoy v1.37.2 also does not emit it under all-defaults config (shadow-only mode reuses the regular 3-counter family).
```

### 13.2 `## Stat-name mapping ### 26-name table` 26→29 extension

The existing 26-name table (extended by phase 09 from 17→22; by phase 11 from 22→26) gains 3 new rows. Verbatim Markdown patch:

```markdown
... [existing 26 rows] ...
| `http.<stat_prefix>.csrf.request_valid`         | counter | filter | csrf | modifying request whose source origin matches target or `additional_origins[].exact` (§11.6) |
| `http.<stat_prefix>.csrf.request_invalid`       | counter | filter | csrf | modifying request whose source origin is determinable but matches neither (§11.6) |
| `http.<stat_prefix>.csrf.missing_source_origin` | counter | filter | csrf | modifying request whose source origin is undeterminable (§11.6) |
```

Plus the heading update: `### 26-name table` → `### 29-name table`. NO new tag-extractor preamble note (UNLIKE phase 11; csrf reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace SN2 extractor — no new pattern needed). NO new SN flattening rule.

### 13.3 `## Equivalence Matrix` new row (verbatim table-row patch)

Verbatim Markdown patch (new row appended to the existing equivalence-matrix table):

```markdown
| HTTP filter `envoy.filters.http.csrf` | 0014-http-csrf: scenario1: same-origin POST → 200; scenario2: cross-origin POST → 403 (§11.10 wire shape: content-length=14, body=`Invalid origin`, 4-header lowercase set); scenario3: `additional_origins` host:port match → 200; scenario4: no source-origin → 403 + `missing_source_origin +1`; scenario5: Referer fallback → 200; scenario7: per-route TPFC wholesale-override (§11.9 — per-route data REPLACES listener data; counters AGGREGATE since stats are SHARED). All 6 scenarios HTTP/1.1 plaintext; no timing tolerances (csrf is purely synchronous). NOT asserted: StringMatcher non-exact variants (deferred — drop at PARSE per ADR-0101 §3), `filter_enabled` percentage values other than 100% (deferred — Runtime + hot restart family), `shadow_enabled` semantics (deferred), H2 differential coverage. |
```

### 13.4 Forward-pointer notes (per BRAINSTORM §11 inline supersessions/amendments)

Verbatim Markdown patch (appended to the existing `## Forward-pointer notes` section):

```markdown
### Phase 12 forward-pointer notes

**Deferred field families** (silent-ignored / parse-validated-but-runtime-ignored per ADR-0040 + ADR-0121; see `### envoy.filters.http.csrf ### Field decomposition` above + phase 12 SPEC §2.1 for the full 3-field map):

- `filter_enabled` (`RuntimeFractionalPercent`) — PGV-required at parse-time (envoy-go validates presence of the field + its inner `default_value` per phase 12 SPEC §11.11; mirrors Envoy's PGV envelope). Silent-ignored at runtime; envoy-go always evaluates as if 100%-active. **Divergence-window:** users who explicitly set `default_value < 100%` will see Envoy gate by percentage (where `default_value=0%` short-circuits the filter entirely, all 3 counters stay at 0), envoy-go always-100%. Differential fixture 0014 sets explicit 100% on both sides for byte-equivalent equivalence. Couples to Runtime + hot restart family.
- `shadow_enabled` (`RuntimeFractionalPercent`) — OPTIONAL at parse-time (Envoy permits omission). Silent-ignored at runtime; envoy-go always-never-shadow. **Stat coupling:** when `filter_enabled=0%` and `shadow_enabled=100%` in reference Envoy, the same 3-counter family increments (request_valid / request_invalid / missing_source_origin) but no 403 is emitted; envoy-go's MVP cannot reach this state since it always evaluates as 100%-enforce. Couples to Runtime + hot restart family.
- `additional_origins[].StringMatcher` non-exact variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) — dropped at PARSE time per ADR-0101 §3 discipline. Empty-value `exact` entries also dropped. Couples to whatever future phase lands the full StringMatcher engine (TBD; not currently a §9 family heading).

**Operator footgun (per phase 12 SPEC §11.7 + §11.8):** `additional_origins[].exact` matches the source's `host[:port]` form (NOT the full URL with scheme). Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header. Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port). envoy-go faithfully replicates Envoy's behavior; this is a footgun in the upstream spec, NOT an envoy-go-specific quirk.

**No new tag-extractor:** csrf reuses the existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (Rule SN2 from ADR-0061). UNLIKE phase 11's local_ratelimit which introduced filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118), phase 12 introduces NO new SN flattening rule and NO new tag-extractor pattern.

**Per-route stats SHARED with listener-level:** csrf is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern. Phase 11's local_ratelimit is the precedent for "wholesale stateful override + independent stats" (per ADR-0117). The two patterns coexist under ADR-0073's wholesale-override discipline; future stateful per-route filters with their own stat namespaces follow phase 11; future data-only per-route filters with HCM-scoped stats follow phase 12.
```

---

## 14. Testing strategy (per BRAINSTORM §11 + §1.1 amendments)

### 14.1 Unit tests (`internal/filter/http/csrf/csrf_test.go`)

Six test groups (~150-200 LoC total):

- **Group 1 — `New` factory PGV + filter-internal validation (per §11.11 / §1.1 amendment 3):** test that `New` rejects `filter_enabled == nil` AND rejects `filter_enabled.DefaultValue == nil`; test that `New` accepts `filter_enabled.default_value=100%` (boots successfully) AND `filter_enabled.default_value=0%` (boots successfully — envoy-go silent-ignores the percentage value at runtime); test that `New` accepts `shadow_enabled` absent (treats as never-shadow) AND `shadow_enabled` present (silent-ignored at runtime).
- **Group 2 — `additional_origins` parse-time discipline (per §11.7 + §11.8 + ADR-0101 §3):** test that non-exact StringMatcher variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) are DROPPED at parse time and do NOT survive into the resulting `runtimeConfig`'s `additionalOrigins` slice; test that empty-value `exact` entries are also dropped; test that surviving entries are stored verbatim (no normalization at parse time — the operator's `host[:port]` form is preserved byte-for-byte).
- **Group 3 — `DecodeHeaders` non-modifying methods (per §11.1):** parametrized over `GET, HEAD, OPTIONS, TRACE` plus at least one custom verb (`PROPFIND`); all return `Continue` immediately, no counter increments, no origin parsing invoked. **This subsumes scenario 6 from the BRAINSTORM Q5 dialogue (and is the unit-only coverage for the GET-passthrough not-in-fixture-0014 case).**
- **Group 4 — `DecodeHeaders` origin-extraction trichotomy (per §11.2):** test all three branches: (i) `Origin: null` → `missing_source_origin` (no Referer fallback even with valid Referer); (ii) `Origin:` empty + valid Referer → use Referer's `hostAndPort`; (iii) `Origin: not-a-url` (verbatim) → use verbatim string as source (almost always rejects). Plus the "absent" case (no Origin header + valid Referer → use Referer) and the "absent + absent" case (→ `missing_source_origin`).
- **Group 5 — `DecodeHeaders` host:port-only equality (per §11.3 + §11.7 + §11.8):** test that uppercase Origin (`HTTPS://APP.EXAMPLE.TEST`) does NOT match lowercase config; test that explicit-default-port Origin (`https://app.example.test:443`) does NOT match the no-port config entry; test that trailing-slash Origin (`https://app.example.test/`) MATCHES (slash stripped via URL parser); test that `additional_origins: [{exact: "https://app.example.test"}]` (full-URL form, the operator footgun) does NOT match `Origin: https://app.example.test` (because compile-list entry retains the scheme but source's hostAndPort drops it).
- **Group 6 — `DecodeHeaders` per-route override + shared stats (per §11.9):** test that two `runtimeConfig` instances built from independent `New` invocations carry independent `additionalOrigins` slices but SHARE the listener-level `*filterStats` pointer; counter increments on per-route AGGREGATE with listener-level (the SAME counter series). Plus a test that listener-level `*filterStats` is touched on the per-route dispatch path (i.e., the per-route filter does NOT have an empty-stats no-op).

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/csrf/...` green. `go vet`, `golangci-lint run` clean. csrf has no shared mutable state (the `runtimeConfig` is read-only after `New`; counters use `atomic.Int64`); race-test cleanness is structural — no mutex needed.

### 14.3 Fuzzers

New fuzzer `FuzzCsrfPolicyConfigParse` in `internal/filter/http/csrf/fuzz_test.go`:

```go
func FuzzCsrfPolicyConfigParse(f *testing.F) {
    f.Add(...)  // a few well-formed seeds
    f.Fuzz(func(t *testing.T, raw []byte) {
        any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
        _, _ = New(envoyhttp.FactoryCtx{...}, any)
        // expectation: no panic, no goroutine leak, no resource leak
    })
}
```

This is the 16th fuzzer in the repo (15 existing per phase 11 phase-done + this new one). Fuzz budget: 30s per the existing per-phase fuzzer gate.

Optional secondary corpus: pre-seeded `additional_origins` with malformed StringMatcher patterns (regex, etc.) — those dropped at parse time per ADR-0101 §3 discipline; fuzzer should not panic during the parse-time filter step.

### 14.4 Existing fuzzers re-run

All 15 existing fuzzers (FuzzBootstrapConfigParse, FuzzCORSConfigParse, FuzzFaultConfigParse, FuzzHeaderMutationConfigParse, FuzzLocalRateLimitConfigParse, FuzzConfigDumpFormat, FuzzAccessLogFormat, etc.) continue to pass at 30s budget. Phase 12 introduces no shared fuzz-input surface changes that would invalidate existing fuzzers.

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin; phase 12 introduces no HTTP/2 stack changes (the csrf filter operates above the codec layer per §5.9 concurrency model).

### 14.6 Differential 0000–0013 + 0014

14 prior fixtures (0000-tcp-echo through 0013-http-local-ratelimit) continue to pass; phase 12 adds the new `0014-http-csrf` (6 scenarios per §7.1). Total wallclock estimated 45–55s for 15 fixtures (phase 11 reported ~43–45s for 14 fixtures; fixture 0014 adds ~3–5s — all synchronous, no timing tolerances).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Per §3 above:

- Gate A: build / vet / lint clean.
- Gate B: race-test pass on all 35 packages.
- Gate C: h2spec 53/53 PASS.
- Gate D: 16 fuzzers green at 30s budget.
- Gate E: 15 differential fixtures 0000–0014 PASS.
- Gate F: BEHAVIOR_CONTRACT.md populated with §13's 4-edit bundle.

All six gates green at the phase-done commit.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

The phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.5 review session) confirms:

- [ ] `internal/filter/http/csrf/` package exists with files matching §4.1 (allowing PLAN-author file split per §4.1's note).
- [ ] `cmd/envoy-go/main.go` registers `csrf.New` against `csrf.TypeURL` before `httpReg.Freeze()`, alphabetical-after-router insertion ordering (`router → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`).
- [ ] `New` factory PGV-mirror validation matches §11.11: rejects `filter_enabled == nil` and `filter_enabled.DefaultValue == nil`; accepts `shadow_enabled` absent.
- [ ] `runtimeConfig` shape matches §6.2 (1 actively-consumed field; 1 PGV-validated-not-honored at runtime; 1 silent-ignored entirely).
- [ ] Origin extraction trichotomy matches §11.2 + §6.4 (Origin: null → empty; empty/absent Origin → Referer fallback; non-empty unparseable Origin → verbatim).
- [ ] Comparison algorithm matches §11.3 + §11.7 + §11.8 + §6.4 (host:port-only equality; NO scheme; NO normalization; trailing slash stripped via URL parser; `additional_origins` matches host:port form).
- [ ] `DecodeHeaders` body matches §6.5 (Continue for non-modifying methods; Continue for allow path; SendLocalReply + StopIteration for reject paths; counter increments per disposition table).
- [ ] Per-route override semantics match §5.8 + §11.9 (data-only wholesale-override; SHARED listener-level stats — no independent counter series per per-route entry).
- [ ] Stat surface 26→29 with the 3 csrf counters under `http.<HCM stat_prefix>.csrf.<counter>` reusing the existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (NO new SN flattening rule).
- [ ] Rejection response wire shape matches §11.10 (4 headers in order: `content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`; 14-byte `Invalid origin` body, no LF; status `403 Forbidden`; Content-Length framing; NO charset modifier; NO chunked; NO cache-control / x-content-type-options).
- [ ] Differential fixture 0014 6 scenarios green (no timing tolerance; sequential pass acceptable).
- [ ] `FuzzCsrfPolicyConfigParse` green at 30s budget (16 fuzzers total).
- [ ] All 14 prior differential fixtures still green; 15 prior fuzzers still green; h2spec 53/53 still PASS.
- [ ] `BEHAVIOR_CONTRACT.md` populated with §13's 4-edit bundle at phase-done commit.
- [ ] `DECISIONS.md` carries 5 new ADRs (ADR-0120..ADR-0124) anchored per §8.

---

*End of phase 12 SPEC.*
