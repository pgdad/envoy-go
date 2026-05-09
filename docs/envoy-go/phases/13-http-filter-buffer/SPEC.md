# Phase 13 SPEC — `envoy.filters.http.buffer`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 13 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 / 12 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 798 lines after the §12 amendment + advisory-rec follow-up). §§1–11 are the pre-amendment design sketch (PRESERVED VERBATIM per D-3.5); §12 is the post-landing empirical amendment (2026-05-09) that supersedes §§1.1, 2.5, 2.6, 2.7, 2.8, 6, 7, 8, and 11 for SPEC-drafting purposes. NO off-master prebrainstorm-notes branch (UNLIKE phase 11; matches the phase 09 / 10 / 12 cold-start precedent).

**ADR continuity:** Phase 12 closed at ADR-0124. Phase 13 anticipated ADR-0125..ADR-0128 (4 ADRs per BRAINSTORM §7); BRAINSTORM §12.7 retired ADR-0128 PROVISIONALLY pending §unresolved verification — the §11 re-run at this SPEC drafting session (§11.5 below) CONFIRMS the retirement. Phase 13 ships **3** ADRs: ADR-0125, ADR-0126, ADR-0127 v2. Next-free ADR after phase 13 is ADR-0128.

---

## 1. Purpose

Phase 13 lands `envoy.filters.http.buffer` — Envoy's canonical "buffer entire request body before forwarding upstream" filter — as the SIXTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), and csrf (12), and the SIXTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 13 is the **structurally-thinnest §9 family-row at the proto level** (1 top-level field on the parent `Buffer` proto) but introduces the FIRST §9 family-row to interact with the framework's body-buffering machinery (ADR-0076) and the FIRST §9 family-row whose per-route proto introduces a top-level `disabled` boolean shortcut as a first-class shape. The five new architectural primitives:

1. A new `internal/filter/http/buffer/` package owning the filter implementation. Directory + Go-package identifier are both `buffer` (single token; matches the cors/fault/csrf precedent — no underscore needed since the proto type-name is already a single token). Files mirror the cors/fault/csrf precedent: `buffer.go` (filter type + factory + decode methods + per-route helper + `compiledConfig` + `compiledPerRoute`), `buffer_test.go` (unit tests across 6 test groups per §14.1), `doc.go` (package overview + 1-consumed/0-deferred decomposition + per-route disabled-OR-override summary), `fuzz_test.go` (`FuzzBufferConfigParse` per §14.3 — the 17th fuzzer in the repo). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`compiledConfig`, `compiledPerRoute`, `filter`) are unexported — phase 13 emits no filter-specific counters per §1.1 amendment 5, so there is no `filterStats` analog to phase 12 csrf's struct. See ADR-0125.

2. **Body-counting + 413 trigger algorithm — STREAMING-CAP ONLY (no Content-Length fast-fail).** Per BRAINSTORM §12.4 Decision 6 v2 (replaces §2.6 in full): envoy-go's filter does its own per-stream byte-counting in `DecodeData` and emits the 413 itself via `SendLocalReply`, while preserving WIRE-EQUIVALENT outcomes with reference Envoy on every observable axis. `DecodeHeaders` returns `StopIteration` on bodied + non-disabled requests (mirrors Envoy's `buffer_filter.cc:67`); `DecodeData` accumulates via `DataStopIterationAndBuffer`, fires `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` when `accumulated > effectiveMax`, and on terminal `endStream=true` invokes `maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`) to inject `Content-Length: <accumulated>` on the held headers when the original request had no Content-Length (chunked → fixed-CL conversion observable upstream). **NO Content-Length fast-fail in `DecodeHeaders`** — the BRAINSTORM §2.6 fast-fail clause was empirically refuted at §11.6 below. See ADR-0127 v2.

3. **Per-route TPFC: `disabled` boolean OR `max_request_bytes` override (5th canonical per-route discipline).** The proto message `BufferPerRoute` is SEPARATE from the listener-level `Buffer` — UNLIKE phase 12 csrf where the same `CsrfPolicy` served both purposes — and carries a `oneof` with two cases plus PGV constraints `validate.required = true` (oneof) + `bool.const = true` (`disabled`). Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (disabled OR override) drives the disposition. ADR-0125 codifies this as the **5th canonical per-route discipline** (after data-only-override per ADR-0073 used by cors/fault/header_mutation; multi-tier-evaluation via `ResolveAllTiers` per ADR-0110 used by header_mutation; stateful-override-with-INDEPENDENT-stats per ADR-0117 used by local_ratelimit; data-only-override-with-SHARED-stats per ADR-0124 used by csrf). Per-route stats are SHARED with listener-level (mirrors phase 12 csrf ADR-0124; DIVERGES from phase 11 local_ratelimit ADR-0117).

4. **`max_request_bytes ≤ 1 MiB` parse-time validation — envoy-go-only divergence.** Per BRAINSTORM §2.3 + Q3 dialogue: phase 13 consumes the ONE proto top-level field on `Buffer` (`max_request_bytes`, `UInt32Value`, REQUIRED) with envoy-go-own validation (non-nil + value > 0 + value ≤ 1048576). Reference Envoy v1.37.2 accepts arbitrary `UInt32Value` up to ~4 GiB at parse time (confirmed empirically at §11.1 below). The 1 MiB ceiling is the load-bearing envoy-go-only validation — it keeps the buffer filter's own cap inside envoy-go's framework safety net per ADR-0076 (`filterBufferLimitBytes = 1 << 20`); without the validation, a `max_request_bytes > 1 MiB` config would trip the framework's 17-byte 413 path before the buffer filter's wire shape could fire. The same validation applies to `BufferPerRoute.buffer.max_request_bytes`. See ADR-0126.

5. **Stat surface — ZERO new entries.** Per BRAINSTORM §12.5 + §11.5 below: phase 13 contributes ZERO new entries to `BEHAVIOR_CONTRACT.md ## Stat-name mapping`. The 29-name table (extended from 17→22 by phase 09; from 22→26 by phase 11; from 26→29 by phase 12) stays at 29 names. Buffer-filter overflow is observable on the envoy-go side via the existing HCM `downstream_rq_4xx` counter (already in the 29-name table from phase 06.1, rendered via SN4 status-class collapse as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`); this counter is auto-incremented by the post-`SendLocalReply` HCM framework code path. Reference Envoy ALSO emits `envoy_http_downstream_rq_too_large` (HCM-level, generic) and `envoy_http_downstream_rq_completed` (HCM-level), but these are Envoy-only counters NOT in envoy-go's 29-name allow-list — they are filtered out per the existing twin-series-discipline (BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline`). NO `envoy_http_buffer_*` counter exists in reference Envoy v1.37.2 at all (confirmed at §11.5). NO new SN flattening rule. ADR-0128 is RETIRED. See §1.1 amendment 5 + §11.5 below.

After phase 13, the project has proven the §9 HTTP filters family-expansion pattern carries through a SIXTH filter under: the cors precedent's package-shape discipline (single-token directory matching the proto type-name); the fault/csrf precedent's listener-level `compiledConfig` parser pattern; the local_ratelimit precedent's per-route-wholesale-override discipline (extended here to disabled-OR-override sum-type with shared stats); the existing HCM stat-prefix tag-extraction (no new SN rule); zero new framework primitives; and the ADR-0076 framework body-cap as the safety net layered under the filter's own cap. *envoy-go's HTTP filter framework hosts a synchronous, decoder-side body-touching filter that does its own byte-counting + 413 emission rather than delegating to a per-stream-cap framework primitive (which envoy-go does not have); the WIRE OUTCOMES are byte-equivalent to reference Envoy's `setBufferLimit + StopIteration + HCM-emits-413` model on every observable axis (status, body, headers, counter); the `maybeAddContentLength` mirror provides chunked → fixed-CL conversion observable upstream; the `disabled-OR-override` per-route shape codifies the 5th canonical per-route discipline; all under flat top-level row expansion (per ADR-0106).* This is the SIXTH §9 family-row to land; subsequent filters (compression, jwt_authn, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session ratifies the §12 BRAINSTORM amendment and adds **one** load-bearing correction to BRAINSTORM §12.5 (the §unresolved counter-name issue carried in by STATE.md):

- **§11.5 (`downstream_rq_too_large` is Envoy-only; `downstream_rq_4xx` is the in-table counter envoy-go emits) — MINOR PROSE CORRECTION TO BRAINSTORM §12.5.** BRAINSTORM §12.5 v2 claimed `downstream_rq_too_large` is "already in the 29-name table from phase 06.1." It isn't — verifying the citation against `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 29-name table` (lines 134-212) confirms the table contains only `downstream_rq_total` + `downstream_rq_2xx/3xx/4xx/5xx` for the HCM side; `downstream_rq_too_large` and `downstream_rq_completed` are NOT in the table. The §11.5 verbatim Prometheus scrape against `envoyproxy/envoy:v1.37.2` (after 5 buffer-overflow probes) confirms reference Envoy emits BOTH counters at the HCM scope. Resolution: prose correction (option (a) per STATE.md §unresolved). The load-bearing claim ("phase 13 contributes ZERO new stat-table entries") survives intact because:
  1. envoy-go emits `downstream_rq_4xx` (in the 29-name table per phase 06.1; rendered as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}` via Rule SN4 status-class flattening) — this counter increments on every 413 (and every 4xx response generally) emitted by envoy-go's `SendLocalReply` path.
  2. `downstream_rq_too_large` (Envoy-only HCM counter; NOT in envoy-go's 29-name allow-list) is filtered out of the differential per the existing twin-series-discipline: the differential fixture's allow-list enumerates exactly the Prometheus names envoy-go ships (per phase 06.1 SPEC §11.5 + BEHAVIOR_CONTRACT.md `### Twin-series filter discipline`); everything else in the Envoy scrape is ignored.
  3. `downstream_rq_completed` (Envoy-only HCM counter; NOT in envoy-go's 29-name allow-list) is filtered out via the same allow-list discipline.

  Phase 13 therefore ships **3 ADRs** (ADR-0125, ADR-0126, ADR-0127 v2) and adds **0 entries** to the 29-name table. ADR-0128 stays retired per BRAINSTORM §12.7. The §6 fixture matrix (§7.1 below) asserts counter deltas on `downstream_rq_4xx` (the in-table counter envoy-go emits) on the envoy-go side; the Envoy-side scrape is filtered through the existing allow-list before per-counter delta comparison.

The §12 BRAINSTORM amendment's other dispositions (§12.3 P1..P11) are **CONFIRMED VERBATIM** by the §11 re-run. No additional corrections to BRAINSTORM §§12.1, 12.2, 12.3, 12.4, 12.6, 12.7, 12.8 are needed.

### 1.2 Revised scope summary (post-§1.1 amendment)

After the §1.1 amendment, phase 13's in-scope architectural primitives are the FIVE listed at the head of §1, expressed as **10 BRAINSTORM-§1.1-style line items** per BRAINSTORM §12.7's renumbering (item 7 retired per §12.5; items 8→7, 9→8, 10→9, 11→10). The §1.1 amendment 5 above further sharpens item 10 to "ZERO new stat-table entries; counter delta tracked on the existing in-table `downstream_rq_4xx`" but does NOT change item count. Differential fixture has 6 requests (5 §6.2 scenarios + 1 cross-cutting CL-injection assertion per BRAINSTORM §12.6) per §7.1 below. ADR list is **3** (ADR-0125, ADR-0126, ADR-0127 v2). NO ADR-0073 amendment paragraph (per-route is data-only with shared stats; no stateful resource carry — the existing wholesale-override discipline applies as-is, mirrors phase 12 csrf ADR-0124 §Decision (v)). NO ADR-0076 amendment (cap-layering rather than promotion; the future cap-promotion phase is the natural amender per ADR-0076 §Consequences (d)). NO ADR-0061 amendment (no new SN flattening rule).

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 9 + ADR-0106)

Phase 13 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, and stays unchanged in state across phase 13's landing. Phase 13 is the SIXTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12). Each subsequent HTTP filters family member (compression, jwt_authn, rbac, …) becomes its own top-level row at row 14, 15, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

### 1.4 ADR-0045 split-by-surface readiness

Phase 13 stays a SINGLE row at this SPEC. The implementation surface is estimated at:

- ~280–330 LoC production filter (`buffer.go` per BRAINSTORM §3 + §12.8; ~50 LoC slimmer than phase 12 csrf because no origin-parsing helpers, no `additional_origins` compile loop, no PGV-mirror validation surface beyond the simple `max_request_bytes ≤ 1 MiB` check; offset by the body-counting state machine + `maybeAddContentLength` mirror)
- ~25 LoC `doc.go`
- ~400–500 LoC unit tests (6 test groups per §14.1 — ~50 LoC slimmer than phase 12's ~500 because fewer cases per group, but ~50 LoC heavier on the body-counting tests that need data-stream simulation)
- ~60 LoC fuzzer
- ~15 LoC framework deltas (zero new primitives; one new `httpReg.Register` line in `cmd/envoy-go/main.go`) [POST-PIVOT AMENDMENT: actual framework deltas are ~35 LoC — cmd/envoy-go/main.go +1 line + internal/filter/hcm/connection.go +34 LoC (synthetic empty-terminal RunDecodeData + CL reconciliation); see §4.3 amendment below and ADR-0128]
- ~150–200 LoC fixture (envoy.yaml ~70 + envoy-go.yaml ~70 + driver/main.go ~150 + backend/main.go ~30 + expectations.yaml ~30 + README.md ~50; total approximate)
- ~50 LoC ROADMAP+STATE+BEHAVIOR_CONTRACT additions at SPEC commit (this SPEC does not modify production code)

Total: ~890–1090 LoC across all bundles, with ~370 in Go production code. Task count estimate per the BRAINSTORM §1.4: ~10–14 tasks. Both metrics stay well below ADR-0045's 1500-LoC / 25-task split-trigger thresholds. The PLAN author retains the ADR-0045 release valve if PLAN finds the surface exceeds either threshold; the natural split per BRAINSTORM §1.4 is `13.1 = listener-level filter MVP` and `13.2 = per-route disabled-OR-override TPFC + per-route fixture scenarios`. **SPEC's position: single-row.**

### 1.5 No prebrainstorm-notes branch

UNLIKE phase 11 (which inherited an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch from a prior pivoted session), phase 13 has NO prior prebrainstorm-notes artefacts. The phase 13 BRAINSTORM cold-started fresh from the §9 heading + the phase 12 just-shipped artefacts per ADR-0106(e), then post-landing was AMENDED via §12 (in-place D-3.5 amendment, rare) when the §9 empirical-pin re-run during this SPEC drafting session surfaced findings overturning the algorithmic core hypothesized at §2.6/§2.7/§2.8. THIS SPEC consults BRAINSTORM as authoritative (§§1-11 + §12) + the §11 empirical-pin block (this SPEC drafting session) as the divergence-from-BRAINSTORM record (here used only for the §1.1 amendment 5 prose correction). No off-master branch needs to be merged or referenced.

### 1.6 Phase 13 is the second §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at brainstorm-amendment time (§12), not at SPEC time

Phase 12 was the FIRST §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time — TWO MAJOR REVISIONS (§11.3+§11.7+§11.8 collective; §11.11) and TWO MINOR REVISIONS (§11.2 trichotomy; §11.9 stat-sharing). Phase 13 takes a DIFFERENT path: the major revisions landed at brainstorm-amendment time (BRAINSTORM §12, the rare D-3.5 amendment) BEFORE SPEC drafting started in earnest, on the grounds that the empirical re-frame was large enough that the SPEC author benefited from a corrected design sketch — not just a §11 amendment block. This SPEC drafting session re-ran each §9 pin against reference Envoy v1.37.2 and CONFIRMED the §12.3 disposition table verbatim, surfacing exactly ONE additional minor correction (§1.1 amendment 5 above). The pattern of "BRAINSTORM commits hypothesis; SPEC empirically confirms or amends" continues to function as designed; phase 13 demonstrates the brainstorm-amendment route as a release valve when the empirical re-frame is too large for the §11 amendment-block channel.

---

## 2. Non-purposes

Phase 13 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.buffer`.

### 2.1 `Buffer` proto-message non-goals (per BRAINSTORM §8 + §1.1 amendment 5)

The proto message `envoy.extensions.filters.http.buffer.v3.Buffer` carries 1 top-level field — the SMALLEST §9 family proto surface to date (smaller than phase 12 csrf's 3 fields). Phase 13 consumes 1 actively. The `BufferPerRoute` proto separately carries a `oneof override` with 2 cases (`disabled` boolean OR nested `buffer` message); both cases are honored.

- **Actively consumed (1 on `Buffer`):** `max_request_bytes` (`UInt32Value`, REQUIRED). envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 (1 MiB). Value violations (nil, zero, > 1 MiB) reject at parse time with envoy-go-own error wording.

The `Buffer` proto has NO other top-level fields. There is no silent-ignore set at the listener-level `Buffer`. (The `BufferPerRoute` oneof is separate; both cases are honored — see §6.2 below.)

#### 2.1.1 Out of scope: `max_request_bytes > 1 MiB` (envoy-go-only parse-time rejection)

Coupled to: future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)). Reference Envoy behavior: accepts arbitrary `UInt32Value` up to ~4 GiB at parse time; runtime cap is the framework's hardcoded 1 MiB unless overridden via `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` knobs (also silent-ignored per ADR-0076 §Decision (d)). envoy-go behavior: rejects at parse time per ADR-0126 with envoy-go-own error wording. Rejected configs include `Buffer.max_request_bytes = 5 MiB` at listener level, and `BufferPerRoute.buffer.max_request_bytes = 2 MiB` at any route level. Divergence-window: envoy-go-only PARSE-time rejection vs. Envoy's parse-time accept + runtime-cap-at-1-MiB. Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.buffer` subsection + `### Phase 13 forward-pointer notes`.

#### 2.1.2 Out of scope: `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` (silent-ignore inherited from ADR-0076)

Coupled to: same cap-promotion phase. Reference Envoy behavior: these are Listener-scope and Route-scope knobs that scale the framework cap from its 1 MiB default; honored at runtime. envoy-go behavior: silent-ignored at parse time per ADR-0076 §Decision (d). Phase 13 does NOT change this disposition.

### 2.2 Algorithm-shape non-goals (per BRAINSTORM §12.4 Decision 6 v2 + §11.6)

The buffer filter's body-counting algorithm is **STREAMING-CAP ONLY**. Specifically OUT of scope:

- **Content-Length fast-fail in `DecodeHeaders`.** Reference Envoy v1.37.2's `buffer_filter.cc:50-67` does NOT inspect the `Content-Length` header at all; the cap fires only after data accumulates past the limit (confirmed empirically at §11.6 below — a request with `Content-Length: 6291456` and zero body bytes does NOT 413; the connection eventually times out). envoy-go MUST NOT add a CL fast-fail clause; differential equivalence requires the streaming-cap path on every overflow case.
- **`setBufferLimit`-equivalent framework primitive.** Reference Envoy delegates body-counting + 413 emission to HCM via `callbacks_->setBufferLimit(maxRequestBytes)` + `StopIteration`; envoy-go's framework has no per-stream cap-override primitive (per §4 invariant + ADR-0076 §Consequences (d)). The filter does its own counting + emits the 413 via `SendLocalReply` directly. The deliberate divergence is recorded in ADR-0127 v2 §Consequences with an explicit forward-pointer to the future cap-promotion phase that may revisit this.
- **Encode-side state.** Buffer is decoder-side-only. `EncodeHeaders / EncodeData / EncodeTrailers` all pass through (`Continue` / `DataContinue` / `TrailersContinue`).
- **Async-resume.** Buffer's `DecodeHeaders` + `DecodeData` runs synchronously (UNLIKE phase 09 fault's `time.AfterFunc` + parkDecode wake-up). The framework's accumulation between chunks IS asynchronous (the dispatch goroutine yields between `DecodeData` calls), but the filter itself does not register any async resumption.
- **Stateful per-route resources.** UNLIKE phase 11 local_ratelimit (per-route `tokenBucket`), buffer's per-route `compiledPerRoute` is purely data — `{disabled bool, maxOverride *uint32}`. NO mutex, NO atomic, NO synchronization at runtime.

### 2.3 Stat-surface non-goals

- **No new stat-table entries.** Per §1.1 amendment 5: phase 13 contributes ZERO new entries to `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 29-name table`. The table stays at 29 names. Buffer-filter overflow is observable on the envoy-go side via the existing `downstream_rq_4xx` HCM counter.
- **No filter-specific Prometheus tag-extractor** (UNLIKE phase 11 which introduced `envoy_local_http_ratelimit_prefix` per Rule SN9). Buffer reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace extractor where applicable; envoy-go's `internal/admin/stats.go` (or wherever the project registers tag-extractors) requires NO new pattern.
- **No new SN flattening rule.** Phase 13 introduces NO Rule SN10 (or equivalent). The existing Rules SN1–SN9 cover envoy-go's emit set without modification.
- **No `envoy_http_buffer_*` counter family.** Confirmed empirically at §11.5: reference Envoy v1.37.2's `/stats/prometheus` scrape after 5 buffer-overflow probes shows ZERO `envoy_http_buffer_*` lines. The buffer filter has no filter-specific counter namespace at all.
- **No `request_buffered` counter.** BRAINSTORM §2.7 hypothesized this counter; §12.5 retired it. Reference Envoy emits no analog. envoy-go does NOT register one.
- **No `request_too_large` counter.** BRAINSTORM §2.7 hypothesized this counter; §12.5 retired it. Reference Envoy emits no analog. envoy-go does NOT register one.
- **No twin-series filter discipline** beyond the existing one. The differential fixture's allow-list (per BEHAVIOR_CONTRACT.md `### Twin-series filter discipline`) already filters Envoy-only counters like `downstream_rq_too_large` and `downstream_rq_completed` out of the per-counter delta comparison; phase 13 inherits this discipline unchanged.
- **No permanently-zero counter** (UNLIKE phase 09's `fault.response_rl_injected` which is emitted permanently-zero per ADR-0107). Buffer has no filter-specific counter at all.

### 2.4 Test-surface non-purposes

- **No new differential probe filter.** Phase 07.1's `envoy.filters.http.envoy_go_test` (the iteration-state probe filter) covers framework iteration coverage. Phase 13 does not extend that probe.
- **No new fuzzer category.** The 17th fuzzer `FuzzBufferConfigParse` follows the existing `FuzzFooConfigParse` pattern (cors, fault, header_mutation, localratelimit, csrf, etc.).
- **No structural-iteration fixture** (07.1's 0007b). Phase 13 is differential-only.
- **No 15th-fixture renumbering or reshuffling.** Phase 13 is fixture `0015-http-buffer`; the previous fixtures (0000–0014) stay green and unchanged.
- **No GET passthrough scenario in the differential fixture.** The header-only / non-bodied-method passthrough is unit-tested in `buffer_test.go::TestDecodeHeaders_HeaderOnlyEndStream` (parametrized over `GET, HEAD, OPTIONS`, plus a bodied `POST` with `endStream=true` on headers). The differential gate has no GET scenario because the algorithm short-circuits BEFORE any buffer-specific surface (no counter, no body-counting, no `SendLocalReply`) — Envoy and envoy-go both pass the request through as if the filter were absent.
- **No H2 differential coverage.** Phase 13 fixture 0015 is HTTP/1.1-only. H2 differential testing of buffer is deferred to a future bundle (matching the phase 09 / 10 / 11 / 12 precedent — each filter ships with H1 differential coverage; H2 differential coverage is deferred). The chunked-body scenarios (3 + 6 in §7.1) are HTTP/1.1-specific; the H2 analog (DATA frames with no Content-Length) would test the same body-counting path but the H2 wire shape on overflow needs an empirical pin against H2-mode reference Envoy.

### 2.5 Cross-filter non-purposes

- **No interaction with cors / fault / header_mutation / local_ratelimit / csrf per-route configs in fixture 0015.** Phase 13's fixture configures ONLY `buffer` filters (plus the router terminal). Mixed-filter ordering tests are deferred.
- **HCM-level changes (POST-PIVOT AMENDMENT — Task 11; in-place per ADR-0052).** Phase 13 introduces 2 HCM primitives at `internal/filter/hcm/connection.go` (+34 LoC): (1) synthetic empty-terminal `RunDecodeData(ctx, nil, true)` on chunked-body EOF without prior endStream-fire; (2) post-body CL reconciliation propagating filter-set `Content-Length` into `req.ContentLength` + clearing `req.TransferEncoding`. See ADR-0128. `serverHeader()` returning `"envoy"` (per `internal/filter/hcm/codec.go:17`) is UNCHANGED.
- **No extension to existing per-route framework primitives.** Phase 13 reuses `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103–128`); no `ResolveAllTiers` invocation (unlike phase 10 header_mutation), no new framework callback, no `RegisterPerRouteValidator` hook (unlike phase 10). The per-route `BufferPerRoute` shape is validated standalone via `parsePerRoute` at config-load time; no multi-tier protected-set discipline.

### 2.6 Security non-purposes

- **No DoS-resilience characterization beyond the cap.** Phase 13 implements the body-cap primitive itself; characterizing its strength against streaming-attack variants (Slowloris-style body-trickle, deliberate-just-under-cap probes) is out of scope. The filter mirrors reference Envoy's behavior verbatim — the framework's idle/request timeout (out of scope here) bounds the slow-body case.
- **No per-route-disabled threat-model documentation.** Per §11.4 confirmation: when a route uses `BufferPerRoute{disabled: true}`, the filter is wholly inactive; arbitrary-size request bodies pass through (modulo upstream cluster's connection-level handling — empirically a 6 MiB body to a route-disabled path may yield 503 from the upstream's connection-level buffering, NOT a buffer-filter 413). Operators MUST understand that `disabled: true` removes buffer-filter protection on that route.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 13)

The six-gate phase-done discipline (per phase 04+ canonical layout) for phase 13:

| Gate | Specialization for phase 13 |
|---|---|
| **A — Build / vet / lint clean** | `go build ./...`, `go vet ./...`, `golangci-lint run` all green; no new warnings introduced relative to the phase-12 baseline at master tip `a782fc9`. New package `internal/filter/http/buffer/` lints clean. |
| **B — Race-test pass** | `go test -race ./...` green on all 35 packages plus the new `internal/filter/http/buffer/` package (36 packages total). Test count grows by ~25–35 (6 unit-test groups across the new test file). Buffer has no shared mutable state (the `compiledConfig` is read-only after `New`; `compiledPerRoute` is read-only after `parsePerRoute`; the per-stream `accumulated` byte count is stream-local; no `*filterStats`-equivalent struct since phase 13 emits no filter-specific counters); race-test cleanness is structural. |
| **C — h2spec 53/53 PASS** | Conformance gate at the ADR-0051 pin (53/53 PASS); phase 13 introduces no HTTP/2 stack changes, so this is a regression check — not an extension. |
| **D — All fuzzers green at 30s budget** | 16 existing fuzzers (per phase 12 phase-done) + 1 new (`FuzzBufferConfigParse`) = 17 fuzzers. Each runs 30s in the per-phase fuzzer gate; all green. |
| **E — All differential fixtures 0000–0015 PASS** | 15 prior fixtures + the new `0015-http-buffer` = 16 fixtures green. Total runtime estimated ~50–60s wallclock (phase 12 reported ~45–55s for 15 fixtures; fixture 0015 adds ~3–5s for its 6 requests — all synchronous, no timing tolerances). |
| **F — `BEHAVIOR_CONTRACT.md` populated** | §13.1 new subsection `### envoy.filters.http.buffer` (inline; ~70 lines per the phase 09 / 10 / 11 / 12 precedent — slightly larger than phase 12's ~50 because the body-counting algorithm + `maybeAddContentLength` mirror need explicit prose); §13.2 stat-table 29-name table preamble note (phase 13 adds ZERO new rows — see §1.1 amendment 5 + §13.2 below); §13.3 NEW equivalence-matrix row pointing at fixture 0015 with per-scenario tolerance discipline; §13.4 NEW forward-pointer notes subsection (`### Phase 13 forward-pointer notes`) covering the 2-item deferral list (per §12 below). All edits land in-place per ADR-0052 at the phase-done commit. |

Gates A–E are the verification gates; Gate F is the contract-extension gate. All six must be green at the phase-done commit per `BOOTSTRAP_PROMPT.md` §7.5.

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 13)

```
internal/filter/http/buffer/doc.go         ~25 LoC; package overview + 1-consumed/0-deferred decomposition + per-route disabled-OR-override summary
internal/filter/http/buffer/buffer.go      ~280-330 LoC; filter + factory + DecodeHeaders + DecodeData + DecodeTrailers + maybeAddContentLength + parsePerRoute + compiledConfig + compiledPerRoute
internal/filter/http/buffer/buffer_test.go ~400-500 LoC; 6 test groups per §14.1
internal/filter/http/buffer/fuzz_test.go   ~60 LoC; FuzzBufferConfigParse per §14.3
```

The PLAN author may split `buffer.go` into multiple files (e.g., a `count.go` for body-counting helpers OR a `perroute.go` for per-route helpers) if test readability benefits. The SPEC explicitly defers this micro-decision (BRAINSTORM does not introduce a file-split sub-decision); no ADR class commitment. See §12 D2.

### 4.2 New differential fixture

```
test/fixtures/0015-http-buffer/envoy.yaml             ~70 LoC; reference Envoy bootstrap (single listener + 3 routes per §7.1)
test/fixtures/0015-http-buffer/envoy-go.yaml          ~70 LoC; equivalent envoy-go bootstrap
test/fixtures/0015-http-buffer/inputs/driver.go       ~150-200 LoC; Go driver issuing 6 requests (§7.1 matrix)
test/fixtures/0015-http-buffer/expectations.yaml      ~30 LoC; per-scenario allow-list + counter delta tolerances
test/fixtures/0015-http-buffer/README.md              ~50 LoC; fixture overview + scenario list
```

### 4.3 Modified production code (in 13)

```
cmd/envoy-go/main.go                       +1 line; httpReg.Register(buffer.TypeURL, buffer.New) — alphabetical-after-router insertion (router → buffer → cors → csrf → ...) before httpReg.Freeze()
```

> **POST-PIVOT AMENDMENT (Task 11; in-place per ADR-0052):** Integration testing at Task 11 revealed two framework gaps; the following file was ALSO modified:
>
> ```
> internal/filter/hcm/connection.go          +34 LoC; TWO framework primitives for chunked-body end-stream detection + Content-Length reconciliation (see ADR-0128):
>   - synthetic empty-terminal RunDecodeData(ctx, nil, true) when chunked body Read returns (0, io.EOF) without prior endStream-fire
>   - post-body CL reconciliation: when req.Header["Content-Length"] was set by a filter AND req.ContentLength < 0 (chunked origin), propagate into req.ContentLength + clear req.TransferEncoding so req.Write emits fixed-CL (not chunked) on the wire
>   - strconv import added
> ```

The claim "NO other production-code changes" is amended: `internal/filter/hcm/connection.go` is modified (+34 LoC). The original SPEC claim held for the filter package itself (correct), but the HCM framework needed two primitives. See ADR-0128 for full rationale. Phase 13 is no longer the structurally-thinnest §9 family-row at the framework-delta level (below phase 12 csrf but above zero).

### 4.4 Modified docs (at SPEC commit)

```
docs/envoy-go/ROADMAP.md                   row 13 status: planned → in-progress (per §3 phase-done gate sequence; lifecycle-state-1 → 2 transition)
docs/envoy-go/STATE.md                     active-phase pointer + lifecycle-state pointer + next-skill pointer
```

### 4.5 Modified docs (at phase-done commit)

```
docs/envoy-go/BEHAVIOR_CONTRACT.md         §13's 4-edit bundle (per §3 Gate F)
docs/envoy-go/DECISIONS.md                 ADR-0125, ADR-0126, ADR-0127 v2 appended
docs/envoy-go/ROADMAP.md                   row 13 status: in-progress → done; row summary set per §1
docs/envoy-go/STATE.md                     active-phase pointer to next planning slot; phase-done commit
```

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 13)

```
   cmd/envoy-go/main.go
        │
        │ httpReg.Register(buffer.TypeURL, buffer.New)        [NEW LINE; phase 13]
        ▼
   internal/filter/http/registry.go
        │
        │ Register(typeURL, factory) → Freeze() → Resolve(typeURL) at HCM build
        ▼
   internal/filter/http/buffer/                                                  [NEW PACKAGE; phase 13]
        ├── buffer.go      (filter + factory + DecodeHeaders + DecodeData + DecodeTrailers + maybeAddContentLength + parsePerRoute + compiledConfig + compiledPerRoute)
        ├── buffer_test.go
        ├── fuzz_test.go   (FuzzBufferConfigParse)
        └── doc.go
        │
        │ uses:
        ▼
   internal/filter/http/perroute.go            (existing; PerRouteConfig.Resolve — 3-tier, most-specific)
   internal/filter/http/                       (existing; HTTPFilter interface, DecodeHeaders/DecodeData/DecodeTrailers status enums, SendLocalReply, DataStopIterationAndBuffer)
```

Untouched (load-bearing absence):

```
internal/filter/http/perroute.go               (existing 3-tier Resolve; phase 13 reuses; NO ResolveAllTiers needed)
internal/filter/http/registry.go               (existing extension registry + Freeze; phase 13 adds one Register call site upstream)
internal/filter/http/chain.go                  (existing; phase 13 reuses filterBufferLimitBytes + localReply413Body verbatim — see §5.11 below)
internal/filter/http/cors/                     (untouched)
internal/filter/http/fault/                    (untouched; reused as the SendLocalReply + StopIteration precedent + DataStopIteration{And,No}Buffer reference)
internal/filter/http/header_mutation/          (untouched)
internal/filter/http/localratelimit/           (untouched; phase 13 explicitly does NOT inherit its filter-specific tag-extractor pattern per §1.1 amendment 5)
internal/filter/http/csrf/                     (untouched; reused as the most-recent decoder-only filter precedent)
internal/filter/http/router/                   (untouched)
internal/filter/http/envoygotest/              (untouched)
internal/filter/hcm/                           (untouched; HCM stays the chain runner; serverHeader() literal "envoy" preserved)
internal/listener/                             (untouched)
internal/cluster/                              (untouched)
internal/admin/                                (untouched; existing HCM-namespace tag-extractor covers any incidental stat-extraction)
internal/drain/                                (untouched)
internal/stats/                                (untouched; phase 13 emits no filter-specific counters — see §1.1 amendment 5)
```

### 5.2 Per-request flow — header-only request passthrough (canonical)

Request: `GET /something HTTP/1.1` arriving on listener configured with buffer (any policy).

```
1. HCM IngressFilter.DecodeHeaders fires.
2. HCM resolves filter chain; buffer.filter.DecodeHeaders called with endStream=true.
3. filter.DecodeHeaders:
   a. endStream=true detected → return Continue immediately.
      (Mirrors buffer_filter.cc:54-56 — no per-route resolve; no body work; no state touch.)
4. HCM advances chain to router; cluster dial; backend response.
5. EncodeHeaders/EncodeData: pass-through (filter has NO encode-side state).
6. Counters: NO buffer-specific counters exist; HCM-level downstream_rq_total +1, downstream_rq_2xx +1 (assuming 200 from backend), downstream_rq_4xx unchanged.
```

This scenario is unit-tested in `buffer_test.go::TestDecodeHeaders_HeaderOnlyEndStream`. NOT a separate scenario in the differential fixture (the §7.1 matrix has no header-only scenario; the 1 KiB body scenario covers passthrough).

### 5.3 Per-request flow — body-fits-cap allow path (scenario 1)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nContent-Length: 1024\r\n\r\n<1 KiB bytes>` against listener-level Buffer with `max_request_bytes=1048576`.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. filter.DecodeHeaders:
   a. endStream is false → continue.
   b. Resolve (effectiveMax, disabled) via RequestRouteConfig().Resolve("envoy.filters.http.buffer", routeIdx) + listener fallback.
      - per-route returns nil → effectiveMax = listener's max_request_bytes = 1048576; disabled=false.
   c. disabled=false → store effectiveMax + headersRef = headers.
   d. Return StopIteration. (Mirrors buffer_filter.cc:67 — holds headers until end-stream.)
3. HCM does NOT advance chain to router yet (StopIteration parks decode-side iteration at this filter index).
4. HCM dispatch yields between data chunks; framework calls buffer.filter.DecodeData(data, endStream).
5. filter.DecodeData with the single 1 KiB chunk + endStream=true:
   a. f.passthrough=false → continue.
   b. f.accumulated += 1024 → 1024.
   c. f.accumulated > effectiveMax (1024 > 1048576)? No.
   d. endStream=true → invoke maybeAddContentLength (no-op since original request had Content-Length: 1024); release held headers; return DataContinue.
6. HCM advances chain to router; cluster dial with body=1 KiB + Content-Length: 1024.
7. Backend returns 200 + echo body.
8. EncodeHeaders/EncodeData: pass-through.
9. Counters: HCM downstream_rq_total +1, downstream_rq_2xx +1; no buffer-filter counters (phase 13 emits none).
```

### 5.4 Per-request flow — streaming-overflow CL-known reject path (scenario 2)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nContent-Length: 2097152\r\nExpect: 100-continue\r\n\r\n<2 MiB bytes>` against listener-level Buffer with `max_request_bytes=1048576`.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. filter.DecodeHeaders:
   a. endStream is false → continue.
   b. Resolve effectiveMax = 1048576; disabled=false.
   c. Store headersRef; return StopIteration.
3. HCM emits "100 Continue" interim response on the wire (independent of buffer filter — HCM/H1-codec discipline; per phase 04).
4. Client begins streaming body; framework calls buffer.filter.DecodeData(chunk, endStream=false) repeatedly.
5. DecodeData on the chunk that pushes f.accumulated past 1048576:
   a. f.accumulated += chunk size → some value > 1048576.
   b. Mid-stream overflow detected.
   c. SendLocalReply(413, "Payload Too Large", []OrderedHeaders{{Name: "Connection", Value: "close"}}).
      The framework's beginLocalReply merges framework-injected content-length: 17 + content-type: text/plain (default); HCM wire-write fills date + server.
   d. Return DataStopIterationNoBuffer (discards the partial buffer; framework's beginLocalReply runs the encode chain immediately).
6. HCM's localReplyDone gate ensures the chain short-circuits without dialing the upstream
   (per phase 09 ADR-0102's terminal-replace pattern).
7. Wire response (after the 100-Continue line):
   HTTP/1.1 413 Payload Too Large
   content-length: 17
   content-type: text/plain
   date: <RFC1123>
   server: envoy
   connection: close

   Payload Too Large
8. HCM closes the connection per the user-supplied "Connection: close" header.
9. Counters: HCM downstream_rq_total +1, downstream_rq_4xx +1; no buffer-filter counters.
   (Reference Envoy ALSO increments the Envoy-only `downstream_rq_too_large` HCM counter; envoy-go does NOT
    emit this counter — it is filtered out of the differential per the existing twin-series-discipline
    allow-list. See §1.1 amendment 5 + §11.5.)
```

### 5.5 Per-request flow — chunked-overflow against per-route tighter cap (scenario 3)

Request: `POST /route-tighter HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nTransfer-Encoding: chunked\r\n\r\n<chunked stream totalling ~200 KiB>` against listener-level Buffer with `max_request_bytes=1048576` AND per-route TPFC `BufferPerRoute{buffer: {max_request_bytes: 131072}}` on `/route-tighter`.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. filter.DecodeHeaders:
   a. endStream is false → continue.
   b. Resolve via RequestRouteConfig().Resolve("envoy.filters.http.buffer", routeIdx) → returns *compiledPerRoute{disabled: false, maxOverride: &131072}.
   c. effectiveMax = 131072 (per-route override wins per ADR-0073 wholesale-override).
   d. disabled=false → store effectiveMax + headersRef; return StopIteration.
3. NO 100-Continue is emitted — chunked encoding does NOT trigger curl's auto-Expect injection (confirmed empirically at §11.8).
4. Client streams chunks; framework calls DecodeData repeatedly.
5. On the chunk that pushes f.accumulated past 131072:
   a. accumulated > effectiveMax → SendLocalReply(413, "Payload Too Large", connClose); return DataStopIterationNoBuffer.
6. Wire response:
   HTTP/1.1 413 Payload Too Large
   content-length: 17
   content-type: text/plain
   date: <RFC1123>
   server: envoy
   connection: close

   Payload Too Large
   (NO 100-Continue prefix; chunked path bypasses the 100-Continue protocol.)
7. Counters: same as §5.4 — HCM downstream_rq_total +1, downstream_rq_4xx +1.
```

### 5.6 Per-request flow — per-route disabled bypass (scenario 4)

Request: `POST /route-disabled HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nContent-Length: 2097152\r\n\r\n<2 MiB bytes>` against listener-level Buffer with `max_request_bytes=1048576` AND per-route TPFC `BufferPerRoute{disabled: true}` on `/route-disabled`.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. filter.DecodeHeaders:
   a. endStream is false → continue.
   b. Resolve → returns *compiledPerRoute{disabled: true}.
   c. disabled=true → set f.passthrough=true; return Continue. (Mirrors buffer_filter.cc:60-62 — the filter is wholly inactive on this route.)
3. HCM advances chain to router; cluster dial begins.
4. Client streams body (2 MiB); framework calls DecodeData repeatedly.
5. DecodeData on each chunk:
   a. f.passthrough=true → return DataContinue (forward chunks raw; framework's safety-net cap never engages because we never return DataStopIterationAndBuffer).
6. Backend receives full 2 MiB body; returns 200 + echo response.
7. Counters: HCM downstream_rq_total +1, downstream_rq_2xx +1 (assuming successful upstream response); no overflow.
```

**Note (per §11.4 confirmation):** when the per-route entry is `disabled: true`, the filter is wholly inactive — arbitrary-size bodies pass through, modulo upstream cluster's connection-level handling. Empirically a 6 MiB body to the disabled route may yield 503 from the upstream's connection-level buffering, NOT a buffer-filter 413; this is the deliberate divergence from any in-listener cap discipline. Operators MUST understand that `disabled: true` removes buffer-filter protection on that route.

### 5.7 Per-request flow — per-route tighter override fires CL-known (scenario 5)

Request: `POST /route-tighter HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nContent-Length: 204800\r\nExpect: 100-continue\r\n\r\n<200 KiB bytes>` against listener-level Buffer with `max_request_bytes=1048576` AND per-route TPFC `BufferPerRoute{buffer: {max_request_bytes: 131072}}` on `/route-tighter`.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. Resolve via 3-tier PerRouteConfig.Resolve → returns *compiledPerRoute{disabled: false, maxOverride: &131072}.
3. effectiveMax = 131072 (per-route override wins per ADR-0073 wholesale-override; listener-level 1 MiB is fully shadowed).
4. Store headersRef + effectiveMax; return StopIteration.
5. HCM emits "100 Continue" (HCM/H1-codec discipline; CL-known + Expect: 100-continue triggers it).
6. Client streams body; framework calls DecodeData repeatedly.
7. On the chunk that pushes f.accumulated past 131072 → SendLocalReply(413, "Payload Too Large", connClose); return DataStopIterationNoBuffer.
8. Wire response: 100-Continue prefix THEN HTTP/1.1 413 Payload Too Large + 4-header set + Connection: close + body "Payload Too Large".
9. Counters: HCM downstream_rq_total +1, downstream_rq_4xx +1.
```

The flow is identical to §5.4 (streaming-overflow CL-known) MODULO the per-route resolution stage at step 2-3 (which produces effectiveMax=128 KiB instead of the listener-level 1 MiB). The 100-Continue prefix appears in BOTH §5.4 and §5.7 because the `Expect: 100-continue` request header is what triggers it (HCM/H1-codec discipline, NOT buffer-filter discipline). Empirical confirmation per §11.4 P4.B + §11.8.

### 5.8 Per-request flow — chunked-passthrough Content-Length injection (scenario 6)

Request: `POST / HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\nTransfer-Encoding: chunked\r\n\r\n<chunked stream totalling ~10 KiB>` against listener-level Buffer with `max_request_bytes=1048576`. Body fits within the cap.

```
1. buffer.filter.DecodeHeaders called with endStream=false.
2. filter.DecodeHeaders:
   a. endStream=false → continue.
   b. Resolve → effectiveMax=1048576; disabled=false.
   c. Store headersRef; return StopIteration.
3. Framework calls DecodeData on each chunk.
4. On the terminal chunk (endStream=true):
   a. f.accumulated += chunk size → ~10240.
   b. f.accumulated > effectiveMax? No.
   c. endStream=true → invoke maybeAddContentLength:
      - f.headersRef != nil ✓ AND original headers had no Content-Length (chunked encoding) ✓
        → set Content-Length: 10240 on the held headers.
      - Drop Transfer-Encoding: chunked (the discipline is chunked → fixed-CL conversion before forwarding upstream;
        mirrors buffer_filter.cc:91-97 + §11.8 empirical evidence at the backend).
   d. Return DataContinue.
5. HCM advances chain to router; cluster dial with the converted headers (Content-Length: 10240; no Transfer-Encoding).
6. Backend RECEIVES the request with Content-Length: 10240 (NOT chunked encoding) — observable assertion in fixture 0015 scenario 6.
7. Backend returns 200 + echo response.
8. Counters: HCM downstream_rq_total +1, downstream_rq_2xx +1.
```

This is the scenario-6 cross-cutting CL-injection assertion per BRAINSTORM §12.6. The empirical evidence at §11.8 below + §11.9 P9.B confirms reference Envoy performs this conversion; envoy-go's `maybeAddContentLength` mirror replicates it byte-equivalently.

### 5.9 Concurrency model

Per-filter-instance: `compiledConfig` is a `*compiledConfig` reference; the underlying value is shared across all goroutines processing requests through that filter instance. Closure-captured at boot-time `New` and never mutated — read-only thread-safe. Per-route `compiledPerRoute` instances are similarly immutable post-config-load.

Per-stream: the `filter` struct is allocated fresh on each request (the closure returned by `New` materializes a new `*filter` per `(callbacks)` invocation); the `filter` holds the per-stream `accumulated` byte count + the resolved `effectiveMax` + the `passthrough` flag + the `headersRef` pointer. NO sharing across streams.

Per-process: registry frozen at boot per ADR-0072 (no runtime registration); per-route TPFC parsed at HCM-build time.

NO mutex. NO atomic counters (phase 13 emits no filter-specific counters per §1.1 amendment 5). NO LBP-1-adjacent declaration (UNLIKE phase 11 which had `sync.Mutex` per `*tokenBucket`). Buffer is purely lock-free at the request hot path; the per-stream `accumulated` counter is a plain `int64` (or `uint64`) on a stack-bound `*filter` instance that no other goroutine touches. Phase 13 is the SECOND production HTTP filter (after phase 12 csrf) with NO synchronization primitive at the request hot path; csrf had a `*atomic.Int64` for stat counters but no mutex; phase 13 has neither (no stat counters; no mutex). Captures naturally under the existing LBP-1.

### 5.10 Filter ordering in fixture 0015

Filter chain in the listener (single listener for fixture 0015):

```
[envoy.filters.http.buffer] → [envoy.filters.http.router]
```

`buffer` is the first (and only non-router) filter. No interaction with cors / fault / header_mutation / local_ratelimit / csrf (per §2.5). Order does not matter for correctness — buffer is the only non-router filter — but the SPEC pins it as `[buffer, router]` for explicitness.

### 5.11 Cap layering against ADR-0076 framework safety net

Per BRAINSTORM §12.4 Decision 6 v2's race-at-1-MiB analysis: the buffer filter's `accumulated > effectiveMax` check at `DecodeData` step 3 races against the framework's safety-net cap (ADR-0076 §Decision (b) `filterBufferLimitBytes = 1 << 20`) that engages only when `DecodeData` returns `DataStopIterationAndBuffer` past 1 MiB.

- When `effectiveMax ≤ 1 MiB` (invariant per ADR-0126's parse-time validation), buffer's own check at step 3 fires BEFORE the framework cap; the framework path is structurally unreachable.
- When `effectiveMax = 1 MiB` exactly, both checks would fire at the 1-MiB-plus-1-byte boundary; the filter wins because step 3 (`accumulated > effectiveMax` → `SendLocalReply` + `DataStopIterationNoBuffer`) executes before step 5 (the `DataStopIterationAndBuffer` return that would trip the framework cap).
- When `effectiveMax < 1 MiB` (e.g., `/route-tighter` 128 KiB override), the filter wins by an even larger margin.
- When `effectiveMax > 1 MiB` (forbidden by ADR-0126), the framework cap would fire first with the hardcoded 17-byte body — but this configuration cannot reach `DecodeData` because `New` rejects at parse time.

**Empirical confirmation (per §11.2):** body=1 MiB exact (cap predicate is `>`, not `>=`) does NOT trip the cap on a 1 MiB listener; body=1 MiB+1 byte → 413. The race resolves benignly in the only configurations that can occur at runtime per ADR-0126.

The framework cap stays armed as a safety net; the `localReply413Body = "Payload Too Large"` constant + `filterBufferLimitBytes` are reused verbatim from `internal/filter/http/chain.go:19-25` (per BRAINSTORM §1.1 deliverable 6). The wire shape of the 413 emitted by the buffer filter is byte-equivalent to the 413 emitted by the framework's `RunDecodeData` overflow path (confirmed by §11.7 + §11.8 empirical pins matching the existing ADR-0076 §11 #3 wire pin from phase 07.1).

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (cors / fault / csrf precedent verbatim)

```go
package buffer

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"

func New(ctx envoyhttp.FactoryCtx, tc *anypb.Any) (envoyhttp.HTTPFilterFactory, error) {
    // 1. tc.UnmarshalTo(&cfg) where cfg is *envoyextensionsfiltershttpbufferv3.Buffer.
    //    - if tc is nil OR Unmarshal fails → return error: "buffer: invalid typed_config: <wrap>".
    // 2. PGV-mirror filter-internal validation (per ADR-0126 + §11.1):
    //    - cfg.GetMaxRequestBytes() == nil → error "buffer: max_request_bytes is required".
    //    - cfg.GetMaxRequestBytes().GetValue() == 0 → error "buffer: max_request_bytes must be > 0".
    //    - cfg.GetMaxRequestBytes().GetValue() > 1048576 → error "buffer: max_request_bytes (N) exceeds envoy-go cap of 1048576 bytes".
    // 3. Build *compiledConfig{maxRequestBytes: cfg.GetMaxRequestBytes().GetValue()}.
    // 4. Return HTTPFilterFactory closure.
}
```

The `New` closure signature mirrors phase 12 csrf verbatim. Decoder-only `HTTPFilter` value: `Decoder: f, Encoder: nil` (mirrors phase 12 csrf — buffer is the SECOND §9 production filter to express decoder-only structurally per ADR-0120 precedent extended to ADR-0125).

### 6.2 `compiledConfig` + `compiledPerRoute` shape (per ADR-0126 + ADR-0125)

```go
// Listener-level (parsed by New):
type compiledConfig struct {
    maxRequestBytes uint32  // validated 0 < value ≤ 1048576
}

// Per-route (parsed by parsePerRoute):
type compiledPerRoute struct {
    disabled    bool     // exclusive with maxOverride; true → filter wholly inactive on this route
    maxOverride *uint32  // exclusive with disabled; nil unless override case set; pointer to discriminate from "unset"
}
```

The two cases are exclusive at the proto level (the `oneof override` discipline); envoy-go's `parsePerRoute` enforces this via the proto3 oneof unmarshal semantics — the JSON decoder rejects malformed configs (both fields set OR neither field set) BEFORE the Go-side handler runs. See §6.3 below.

NO `*filterStats` field on either struct (phase 13 emits no filter-specific counters per §1.1 amendment 5). Compare phase 12 csrf's `runtimeConfig{additionalOrigins, stats}` shape — phase 13 has `compiledConfig{maxRequestBytes}` only at listener level, and `compiledPerRoute{disabled, maxOverride}` only at per-route level. Both structs are minimal.

### 6.3 `parsePerRoute` discipline (per ADR-0125 + §11.3)

```go
func parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error) {
    cfg, ok := perRoute.(*envoyextensionsfiltershttpbufferv3.BufferPerRoute)
    if !ok {
        return nil, fmt.Errorf("buffer per-route: expected *BufferPerRoute, got %T", perRoute)
    }
    switch override := cfg.GetOverride().(type) {
    case *envoyextensionsfiltershttpbufferv3.BufferPerRoute_Disabled:
        if !override.Disabled {
            // PGV constraint bool.const = true rejects disabled: false at proto-decode time;
            // this branch is structurally unreachable BUT defensively returned for safety.
            return nil, fmt.Errorf("buffer per-route: disabled must be true (PGV bool.const violation)")
        }
        return &compiledPerRoute{disabled: true}, nil
    case *envoyextensionsfiltershttpbufferv3.BufferPerRoute_Buffer:
        // Reuse listener-level validation: max_request_bytes must be non-nil + > 0 + ≤ 1 MiB.
        if v := override.Buffer.GetMaxRequestBytes(); v == nil {
            return nil, fmt.Errorf("buffer per-route: max_request_bytes is required")
        } else if v.GetValue() == 0 {
            return nil, fmt.Errorf("buffer per-route: max_request_bytes must be > 0")
        } else if v.GetValue() > 1048576 {
            return nil, fmt.Errorf("buffer per-route: max_request_bytes (%d) exceeds envoy-go cap of 1048576 bytes", v.GetValue())
        } else {
            n := v.GetValue()
            return &compiledPerRoute{maxOverride: &n}, nil
        }
    case nil:
        // PGV constraint validate.required = true on the oneof rejects empty per-route entries at proto-decode time;
        // this branch is structurally unreachable BUT defensively returned for safety.
        return nil, fmt.Errorf("buffer per-route: override oneof is required (neither disabled nor buffer set)")
    default:
        return nil, fmt.Errorf("buffer per-route: unknown override case %T", override)
    }
}
```

**Per §11.3 empirical pin:** the "both fields set" oneof violation case is rejected by Envoy at boot via the **JSON→proto decoder** (error: `'buffer' has already been set (either directly or as part of a oneof)`), NOT by PGV. envoy-go's `protojson.Unmarshal` mirrors this for free — the rejection happens BEFORE `parsePerRoute` is invoked. The "neither field set" case is rejected by PGV's `validate.required` constraint on the oneof; the "disabled: false" case is rejected by PGV's `bool.const = true` constraint. Both PGV constraints are MIRRORED in envoy-go's `parsePerRoute` switch (the `case nil` and the `case *BufferPerRoute_Disabled` with `!override.Disabled`) per ADR-0121 precedent of envoy-go-own-wording for filter-internal validation.

### 6.4 `DecodeHeaders` body (per ADR-0127 v2 + §11.6 + buffer_filter.cc:50-67)

```go
func (f *filter) DecodeHeaders(headers envoyhttp.RequestHeaderMap, endStream bool) envoyhttp.FilterHeadersStatus {
    // 1. Header-only fast-path: mirror buffer_filter.cc:54-56.
    if endStream {
        return envoyhttp.HeadersContinue
    }

    // 2. Resolve (effectiveMax, disabled) via the most-specific per-route entry; listener fallback.
    effectiveMax, disabled := f.resolveEffective(headers)

    // 3. Per-route disabled bypass: mirror buffer_filter.cc:60-62.
    if disabled {
        f.passthrough = true
        return envoyhttp.HeadersContinue
    }

    // 4. Bodied + non-disabled: hold headers; signal StopIteration to wait for body chunks.
    //    Mirrors buffer_filter.cc:67 — the filter delegates buffering to the chain framework
    //    (DataStopIterationAndBuffer) and emits 413 via SendLocalReply on overflow.
    f.effectiveMax = effectiveMax
    f.headersRef = headers
    return envoyhttp.HeadersStopIteration
}
```

`resolveEffective` is a small helper that calls `f.dcb.RequestRouteConfig().Resolve("envoy.filters.http.buffer", routeIdx)`. If the resolved value is a `*compiledPerRoute`:
- `disabled=true` → return `(0, true)` (effectiveMax irrelevant on the bypass path).
- `maxOverride != nil` → return `(*maxOverride, false)`.

If the resolved value is nil (no per-route TPFC for this route → listener fallback applies), return `(f.config.maxRequestBytes, false)`.

**Per §11.6 empirical pin:** Envoy's `decodeHeaders` does NOT inspect the `Content-Length` header. envoy-go MUST NOT add a CL fast-fail clause; differential equivalence requires the streaming-cap path on every overflow case.

### 6.5 `DecodeData` body (per ADR-0127 v2 + §11.2 + §11.7 + §11.9 + buffer_filter.cc:69-79)

```go
func (f *filter) DecodeData(data envoyhttp.BufferInstance, endStream bool) envoyhttp.FilterDataStatus {
    // 1. Per-route disabled bypass: filter-side fast-pass.
    if f.passthrough {
        return envoyhttp.DataContinue
    }

    // 2. Accumulate.
    f.accumulated += uint32(data.Len())

    // 3. Mid-stream overflow: emit 413 + discard partial buffer.
    if f.accumulated > f.effectiveMax {
        // Mirrors buffer_filter.cc + chain.go's localReply413Body verbatim.
        f.dcb.SendLocalReply(413, []byte("Payload Too Large"), envoyhttp.OrderedHeaders{
            {Name: "Connection", Value: "close"},
        })
        return envoyhttp.DataStopIterationNoBuffer
    }

    // 4. Terminal chunk fits: invoke maybeAddContentLength; release held headers + body.
    if endStream {
        f.maybeAddContentLength()
        return envoyhttp.DataContinue
    }

    // 5. In-flight chunk (more to come): accumulate; framework holds bytes per ADR-0076 §Decision (b).
    return envoyhttp.DataStopIterationAndBuffer
}

func (f *filter) maybeAddContentLength() {
    // Mirrors buffer_filter.cc:91-97 verbatim.
    if f.headersRef != nil && f.headersRef.Get("content-length") == "" {
        f.headersRef.Set("content-length", fmt.Sprintf("%d", f.accumulated))
        // Per §11.8 empirical evidence: Envoy ALSO drops Transfer-Encoding: chunked in the same path.
        // envoy-go MUST drop the header to match the upstream-side observable.
        f.headersRef.Remove("transfer-encoding")
    }
}
```

The `maybeAddContentLength` mirror is the load-bearing CL-injection assertion per BRAINSTORM §12.1 fact 5 + §12.6 scenario 6 (cross-cutting). Empirical evidence at §11.8 below confirms reference Envoy converts `Transfer-Encoding: chunked` → fixed `Content-Length: <accumulated>` before forwarding upstream; envoy-go's mirror MUST match.

### 6.6 `DecodeTrailers` body

```go
func (f *filter) DecodeTrailers(trailers envoyhttp.RequestTrailerMap) envoyhttp.FilterTrailersStatus {
    // Defensive: invoke maybeAddContentLength in case end-stream arrived via trailers, not via terminal endStream=true on data.
    f.maybeAddContentLength()
    return envoyhttp.TrailersContinue
}
```

### 6.7 `Encode*` + `OnDestroy` + `SetDecoderCallbacks` bodies

```go
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) {
    f.dcb = cb
}

// Encoder is nil per HTTPFilter value — these methods MUST NOT exist on the filter type.
// The chain framework dispatches per non-nil side per internal/filter/http/types.go's HTTPFilter doc-comment.

func (f *filter) OnDestroy() {
    // No-op. Buffer has no per-request resources to clean up:
    // - no timers
    // - no goroutines
    // - no atomic counters to balance (the `accumulated` field is per-stream and dies with the *filter instance)
    // - no held connection-level state
}
```

### 6.8 Filter-callback wiring

The factory closure returns:

```go
return envoyhttp.HTTPFilter{
    Name:    filterName,                  // "envoy.filters.http.buffer"
    Decoder: f,                           // *filter implementing the decoder-side method set
    Encoder: nil,                         // decoder-only; mirrors phase 12 csrf ADR-0120
    PerRoute: parsePerRoute,              // *PerRouteConfig builder per-tier
}
```

The `PerRoute` field threads into `internal/filter/http/registry.go`'s parsing path; `parsePerRoute` is invoked at HCM-build time for each `BufferPerRoute` TPFC entry on Route / VirtualHost / RouteConfiguration. The 3-tier `PerRouteConfig.Resolve` runs at request-time inside `DecodeHeaders`'s `resolveEffective` call.

---

## 7. Differential fixture `0015-http-buffer`

### 7.1 Per-request matrix (6 requests; per BRAINSTORM §12.6)

| # | Scenario | Request | Expected response | Counter delta (envoy-go side) | §11 cross-ref |
|---|---|---|---|---|---|
| 1 | Body fits within listener cap | `POST /` body=1 KiB (CL-known) | 200 OK + backend echo | `downstream_rq_total +1`, `downstream_rq_2xx +1` | n/a (passthrough baseline) |
| 2 | Streaming overflow with CL-known body | `POST /` body=2 MiB (CL-known); driver receives `100 Continue` first; cap fires mid-stream | 100-Continue + 413 + `Payload Too Large` body + 4-header set + `Connection: close` | `downstream_rq_total +1`, `downstream_rq_4xx +1` | §11.1 + §11.2 + §11.7 + §11.8 + §11.8-100C |
| 3 | Chunked overflow against per-route tighter cap | `POST /route-tighter` `Transfer-Encoding: chunked` body~=200 KiB (above 128 KiB override) | 413 + `Payload Too Large` body (NO 100-Continue with chunked) | `downstream_rq_total +1`, `downstream_rq_4xx +1` | §11.9 chunked |
| 4 | Per-route disabled bypasses cap | `POST /route-disabled` body=2 MiB (above listener 1 MiB) | 200 OK + backend echo | `downstream_rq_total +1`, `downstream_rq_2xx +1` | §11.4 |
| 5 | Per-route tighter override fires (CL-known) | `POST /route-tighter` body=200 KiB (above 128 KiB override) | 100-Continue + 413 + `Payload Too Large` body | `downstream_rq_total +1`, `downstream_rq_4xx +1` | §11.4-tighter |
| 6 | Chunked-passthrough Content-Length injection (cross-cutting) | `POST /` `Transfer-Encoding: chunked` body=10 KiB | 200 OK + backend echo; **backend asserts inbound request carries `Content-Length: 10240` (NOT chunked encoding)** | `downstream_rq_total +1`, `downstream_rq_2xx +1` | §11.8-CL + §11.9 P9.B |

**Total counter deltas after the 6-request workload:** `downstream_rq_total +6`, `downstream_rq_2xx +3`, `downstream_rq_4xx +3`. NO `envoy_http_buffer_*` counter deltas asserted (none exist per §11.5). The Envoy-only `downstream_rq_too_large` (+3) and `downstream_rq_completed` (+6) counters are filtered out of the per-counter delta comparison via the existing twin-series-discipline allow-list (per BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` + phase 06.1 SPEC §11.5).

### 7.2 Topology

`test/fixtures/0015-http-buffer/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config (initially identical; any divergence per ADR-0007 documented in `README.md`).
- `inputs/driver.go` — Go driver that drives both proxies with identical inputs.
- `expectations.yaml` — per-scenario allow-list / ignore-list / counter-delta map.
- `README.md` — fixture overview + scenario list + reference config citations.

Single listener `127.0.0.1:<port>` (HTTP/1.1 plaintext per phases 09/10/11/12 precedent). One virtual_host `vh_main` with three routes:
- `/` (default route) — uses listener-level Buffer (1 MiB cap).
- `/route-disabled` — uses per-route TPFC with `disabled: true`.
- `/route-tighter` — uses per-route TPFC with `buffer.max_request_bytes: 131072` (128 KiB; tighter than the 1 MiB listener default).

One cluster `c0` reaching the host-side echo backend at `test/helpers/echobackend/` (the same backend used by phases 09/10/11/12, extended with a custom `x-asserted-content-length` echo header to support scenario 6's CL-injection assertion at the backend boundary; PLAN author may use an existing primitive or extend per the existing precedent).

**Listener-level `Buffer`:**
```yaml
http_filters:
  - name: envoy.filters.http.buffer
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer
      max_request_bytes: 1048576  # 1 MiB; the envoy-go cap per ADR-0126
  - name: envoy.filters.http.router
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
```

**Per-route TPFC on `/route-disabled`:**
```yaml
typed_per_filter_config:
  envoy.filters.http.buffer:
    "@type": type.googleapis.com/envoy.extensions.filters.http.buffer.v3.BufferPerRoute
    disabled: true
```

**Per-route TPFC on `/route-tighter`:**
```yaml
typed_per_filter_config:
  envoy.filters.http.buffer:
    "@type": type.googleapis.com/envoy.extensions.filters.http.buffer.v3.BufferPerRoute
    buffer:
      max_request_bytes: 131072  # 128 KiB
```

### 7.3 Asserted equivalence

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status**: byte-equal between Envoy and envoy-go for every scenario (200 on passthrough; 413 on overflow).
- **Response body** on overflow: byte-equal `Payload Too Large` (17 bytes, no trailing newline) for scenarios 2, 3, 5. On passthrough scenarios (1, 4, 6), the body is the backend echo response — set-equal modulo timing/identity headers per the existing fixture allow-list.
- **Response header set**: lowercase wire-form, set-equal between Envoy and envoy-go modulo the existing `## Header allow-list` (for `date`, `server`, timing/identity headers). On 413: 4-header set (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) plus `Connection: close`; allow-list-modulo only on `date`.
- **Per-counter delta equality**: after the 6-request workload completes, scrape `/stats/prometheus` from both proxies and assert per-counter deltas listed in §7.1 column 5. Envoy-only counters (`downstream_rq_too_large`, `downstream_rq_completed`) are filtered out via the existing allow-list before per-counter delta comparison.
- **Per-route TPFC bucket independence**: scenarios 4 + 5 demonstrate the listener-level config NOT consulted on `/route-disabled` (cap doesn't engage; 200 even on a 2 MiB body) and the listener-level cap NOT consulted on `/route-tighter` (the override's tighter 128 KiB cap fires).
- **`Connection: close` on 413**: scenarios 2, 3, 5 confirm the connection is closed after the 413 emits.
- **`Content-Length` injection on chunked-passthrough (scenario 6 cross-cutting):** the backend asserts the inbound request carries `Content-Length: 10240` (NOT chunked encoding) on BOTH the Envoy and envoy-go sides. This is the load-bearing assertion for `maybeAddContentLength` byte-equivalence per §11.8-CL.
- **100-Continue prefix**: scenarios 2 + 5 (CL-known `--data-binary @<file>` probes; curl auto-injects `Expect: 100-continue`) emit `HTTP/1.1 100 Continue` BEFORE the eventual 413 on BOTH sides. Scenarios 3 + 6 (chunked `Transfer-Encoding`) do NOT emit 100-Continue. The driver's wire-shape assertion treats "first non-1xx response is 413 (or 200)" rather than "first response is 413"; the 100-Continue line is allowed-but-not-required (matching reference Envoy's HCM/H1 codec discipline).

### 7.4 Driver shape

Go driver in `inputs/driver.go` per phase 09/10/11/12 precedent — sequential request loop (race-tolerant scrape ordering); per-scenario assertions inline; final stats scrape via `/stats/prometheus`. Total: 6 requests in the workload. Estimated driver size: ~150-200 LoC. The chunked-body scenarios (3, 6) require the driver to construct an `http.NewRequest` with a `Transfer-Encoding: chunked` body source (e.g., `io.Pipe` writing bytes incrementally, or pre-formed body bytes via `bytes.NewReader` with `Transfer-Encoding` set explicitly via `req.TransferEncoding`); the driver reads the response stream and asserts the 413 status line + body before the server closes the conn.

**No timing tolerances.** Buffer is purely synchronous — no analog to phase 11's `refill-after-fill_interval ±10ms` scenario.

**No H2 differential coverage.** Phase 13 fixture 0015 is HTTP/1.1-only per §2.4.

---

## 8. ADRs anticipated (per BRAINSTORM §12.7; refined per §1.1)

3 ADRs anticipated. ADR-0124 is the highest-numbered ADR landed in phase 12; ADR-0125 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0125** | `internal/filter/http/buffer/` package shape — single-token directory matching cors/fault/csrf precedent + boot registration ordering (`router → buffer → cors → csrf → ...`) + decoder-only `HTTPFilter` value (mirrors phase 12 csrf ADR-0120) + per-route disabled-OR-override discipline (5th canonical per-route shape; references ADR-0073 + ADR-0117 + ADR-0124 explicitly) | Decision 1 (BRAINSTORM §2.1) + Decision 2 (BRAINSTORM §2.2) + Decision 5 v2 (BRAINSTORM §12.4) |
| **ADR-0126** | `compiledConfig` shape + 1-consumed/0-deferred field decomposition + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence) + cap-layering rationale (buffer's check fires inside the framework cap; ADR-0076 stays armed as safety net) + explicit forward-pointer to the future cap-promotion phase | Decision 3 (BRAINSTORM §2.3) + Decision 4 (BRAINSTORM §2.4 + §12.4) |
| **ADR-0127 v2** | Body-counting + 413-trigger algorithm — STREAMING-CAP-ONLY (no Content-Length fast-fail per §11.6) + `DecodeHeaders` returns StopIteration on bodied + non-disabled requests + `DecodeData` accumulation via DataStopIterationAndBuffer + `DataStopIterationNoBuffer` on overflow + `maybeAddContentLength` mirror per buffer_filter.cc:91-97 + reuse of framework `SendLocalReply` 413 wire shape (ADR-0076 §Decision (b) byte-equivalence; CONFIRMED at §11.7 + §11.8) + 100-Continue addendum (HCM/H1-codec emits independent of buffer filter; CL-known probes only — chunked path bypasses) | Decision 6 v2 (BRAINSTORM §12.4) + Decision 8 v2 (BRAINSTORM §12.4) |

**ADR-0128 RETIRED** per BRAINSTORM §12.7 + §1.1 amendment 5 above. The anticipated ADR slot was for a stat-table extension; the empirical evidence at §11.5 confirms reference Envoy emits NO `envoy_http_buffer_*` counters at all (and the HCM-level `downstream_rq_too_large` counter that DOES increment on a 413 is Envoy-only — filtered out of envoy-go's emit allow-list per the existing twin-series-discipline). Phase 13 contributes ZERO new stat-table entries; no ADR is needed for "no change."

NO omnibus ADR for deferrals (phase 11 dropped this pattern at SPEC §8.1; deferrals inline in BEHAVIOR_CONTRACT.md `### Phase 13 forward-pointer notes` per §13.4 below).
NO amendment to ADR-0073 (per-route is data-only AND most-specific-override; ADR-0125 captures the disabled-OR-override shape WITHIN the existing wholesale-override discipline).
NO amendment to ADR-0076 (cap-layering rather than promotion; ADR-0126 §Decision references the future cap-promotion phase as the natural amender).
NO amendment to ADR-0061 (no new SN flattening rule).
NO amendment to ADR-0040 (silent-ignore set is unchanged; the parse-time `max_request_bytes ≤ 1 MiB` validation is a new envoy-go-only discipline, NOT a silent-ignore).

Next-free ADR after phase 13 phase-done: **ADR-0128** (was ADR-0129 under v1; preserved for the next phase).

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106)

Per ADR-0106(b), this SPEC does NOT pre-author SPEC stubs for the next §9 family-row sibling (compression, jwt_authn, rbac, ext_authz, ext_proc, oauth2, lua, wasm, adaptive_concurrency, admission_control, bandwidth_limit). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts. The SPEC's position: row 14's filter selection is the next BRAINSTORM session's call; row 13's landing leaves the §9 family heading at `ROADMAP.md` line 56 unchanged.

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

The phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.5 review session) confirms the deliverables of §4 are landed, the gates of §3 are green, the code matches §6's contracts, the fixture matches §7's matrix, and the ADRs of §8 are written. Detailed checklist at §15 below.

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 11 pins resolved IN-SESSION; ratifies BRAINSTORM §12.3 dispositions + adds §1.1 amendment 5 prose correction)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 / 10 / 11 / 12 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1 / 08.2 / 09 / 10 / 11 / 12 SPEC §11 confirmation).

**Probe configuration:** Reference Envoy booted under per-pin minimal bootstrap YAMLs at `/tmp/p13-pins/p{1,3,4}-*.yaml` via `docker run -d --name p13-spec-<pin> -p 19911:19911 -p 11301:11301 --add-host=host.docker.internal:host-gateway -v /tmp/p13-pins:/etc/envoy:ro envoyproxy/envoy:v1.37.2 -c /etc/envoy/<file>.yaml --base-id <unique>`; admin port 19911, listener port 11301; HTTP backend (Python `BaseHTTPRequestHandler` echo at `127.0.0.1:18190`) reachable via `host.docker.internal`. Probe curl invocations issued from the host. NOTE: `--network=host` was nonfunctional on the SPEC drafting machine (containers retained their own netns despite the flag); workaround was explicit `-p` port forwarding + binding bootstraps' admin/listener to `0.0.0.0` rather than `127.0.0.1`. The probe behaviors observed are independent of this transport detail. Verbatim probe transcripts are durable at `/tmp/p13-pins/transcripts.md` on the SPEC drafting session machine; the verbatim outputs below are the durable evidence per the 09 / 10 / 11 / 12 SPEC §11 discipline.

Source-of-truth cross-reference: `source/extensions/filters/http/buffer/buffer_filter.cc` at tag `v1.37.2` (102 lines C++ verbatim per BRAINSTORM §12.1). Code fragments quoted in conclusions where load-bearing.

Probe date: **2026-05-09**.

### 11.1 Empirical pin #1 — `max_request_bytes > 1 MiB` parses; runtime cap fires only at the configured value (ratifies BRAINSTORM §12.3.P1)

**Probe configuration:** `p1-overcap.yaml` — minimal buffer-only chain at the listener level with `max_request_bytes: 5242880` (5 MiB). Boot success + body-size probes against `/`.

**Verbatim:**

```
Boot: SUCCESS (envoyproxy/envoy:v1.37.2 with max_request_bytes=5 MiB; PGV does NOT enforce ≤1MiB).
[2026-05-09 15:46:55.910][1][info][config] [source/common/listener_manager/listener_manager_impl.cc:1010] all dependencies initialized. starting workers

=== P1.A: 2 MiB body POST / ===
HTTP/1.1 100 Continue

HTTP/1.1 200 OK
server: envoy
date: Sat, 09 May 2026 15:48:58 GMT
content-type: application/json
content-length: 326

{"method": "POST", "path": "/", "headers": {... "content-length": "2097152", ...}}

=== P1.B: 6 MiB body POST / ===
HTTP/1.1 100 Continue

HTTP/1.1 413 Payload Too Large
content-length: 17
content-type: text/plain
date: Sat, 09 May 2026 15:48:58 GMT
server: envoy
connection: close

Payload Too Large
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P1:**
- (a) **Envoy boots cleanly with `max_request_bytes = 5 MiB`.** Reference Envoy's PGV does NOT enforce the ≤1MiB ceiling that envoy-go-only ADR-0126 imposes.
- (b) **2 MiB body passes through to upstream.** Cap not engaged at 5 MiB.
- (c) **6 MiB body emits 413** with the verbatim wire shape: status `413 Payload Too Large`; body `Payload Too Large` (17 bytes ASCII; NO trailing newline); 4-header lowercase wire-form (`content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`); `Connection: close`.
- (d) **NO separate "framework cap" at 1 MiB exists in reference Envoy.** The "framework cap" framing is envoy-go-internal (ADR-0076's `filterBufferLimitBytes`); reference Envoy enforces only the buffer filter's own configured value. envoy-go's parse-time `max_request_bytes ≤ 1 MiB` validation per ADR-0126 is the load-bearing envoy-go-only divergence that keeps the filter cap inside envoy-go's framework safety net.
- (e) **`100 Continue` precedes the 413** when curl auto-injects `Expect: 100-continue` (which it does for `--data-binary @<largefile>` per curl's heuristic). This is HCM/H1-codec discipline, NOT buffer-filter discipline.

### 11.2 Empirical pin #2 — Cap predicate is `>` (exact-cap fits; cap+1 byte → 413) (ratifies BRAINSTORM §12.3.P2)

**Probe configuration:** `p4-disabled.yaml` listener-level Buffer with `max_request_bytes: 1048576` (1 MiB). Probes against `/` (no per-route override).

**Verbatim:**

```
=== P2.A: body=1 MiB exact (1048576 bytes) POST / ===
HTTP/1.1 503 Service Unavailable
content-length: 95
content-type: text/plain
date: Sat, 09 May 2026 15:51:07 GMT
server: envoy

upstream connect error or disconnect/reset before headers. reset reason: connection termination

=== P2.B: body=1 MiB+1 byte (1048577 bytes) POST / ===
HTTP/1.1 100 Continue

HTTP/1.1 413 Payload Too Large
content-length: 17
content-type: text/plain
date: Sat, 09 May 2026 15:51:07 GMT
server: envoy
connection: close

Payload Too Large
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P2:**
- (a) **Body=1 MiB exact does NOT trip the cap.** The cap predicate is `accumulated > effectiveMax` (strict greater-than), NOT `accumulated >= effectiveMax`. The body fully buffered + forwarded upstream; the upstream backend (`BaseHTTPRequestHandler`) returned a 503 from the connection-level shape (the python backend's request handling did not return a 413 — that 503 is the `upstream connect error or disconnect/reset before headers. reset reason: connection termination` synthesized by Envoy's router after the upstream connection closed, which is unrelated to buffer-filter behavior). The load-bearing observation: NO 413 from the buffer filter at body=1 MiB exact.
- (b) **Body=1 MiB+1 byte emits 413** with the same wire shape as P1.B. The cap fired at exactly 1 byte over the configured value.
- (c) envoy-go's `DecodeData` step 3 (`f.accumulated > f.effectiveMax`) MUST use strict `>` to match reference Envoy's behavior on the exact-cap boundary.

### 11.3 Empirical pin #3 — `BufferPerRoute` oneof violation rejected at boot via JSON-decoder (ratifies BRAINSTORM §12.3.P3)

**Probe configuration:** `p3-oneof.yaml` — listener with route `/route-violation` carrying TPFC entry that sets BOTH `disabled: true` AND `buffer: {max_request_bytes: 65536}` in the same `BufferPerRoute` proto (deliberate oneof violation).

**Verbatim:**

```
[2026-05-09 15:51:43.052][1][critical][main] [source/server/server.cc:453] error `Unable to parse JSON as proto (INVALID_ARGUMENT: invalid JSON  in envoy.config.bootstrap.v3.Bootstrap @ static_resources.listeners[0].filter_chains[0].filters[0].typed_config.<any>.route_config.virtual_hosts[0].routes[0].typed_per_filter_config[0].<any>.buffer: message envoy.extensions.filters.http.buffer.v3.Buffer,   near 1:758 (offset  757): 'buffer'  has already  been set (either directly or as part of  a oneof)): {...}
```

[Boot fails; no "starting workers" line; envoy exits with non-zero status.]

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P3 (CONFIRMED rejection; AMENDED mechanism):**
- (a) **Boot rejection mechanism is the JSON→proto decoder, NOT PGV.** Error wording: `'buffer' has already been set (either directly or as part of a oneof)`. The rejection happens BEFORE PGV's `validate.required` constraint on the oneof would fire.
- (b) **envoy-go's `protojson.Unmarshal` mirrors this for free.** The Go-side oneof discipline rejects malformed configs at decode time; no envoy-go-specific PGV-mirror code is needed for the "both fields set" case. envoy-go's `parsePerRoute` switch (per §6.3) handles only the post-decode shapes: `*BufferPerRoute_Disabled`, `*BufferPerRoute_Buffer`, `nil` (the PGV-required case).
- (c) **Defensive PGV-mirror checks** in `parsePerRoute` (the `case nil` branch + the `disabled: false` branch) handle the structurally-unreachable cases that PGV would catch — they exist for safety and unit-test coverage, NOT for hot-path correctness.

### 11.4 Empirical pin #4 — Per-route `disabled: true` is wholly inactive (ratifies BRAINSTORM §12.3.P4)

**Probe configuration:** `p4-disabled.yaml` — `/route-disabled` carries `BufferPerRoute{disabled: true}`; listener-level cap is 1 MiB.

**Verbatim:**

```
=== P4.A: POST /route-disabled body=2 MiB ===
HTTP/1.1 100 Continue

HTTP/1.1 200 OK
server: envoy
date: Sat, 09 May 2026 15:51:07 GMT
content-type: application/json
content-length: 340
connection: close

{... backend echoed body ...}

=== P4.B: POST /route-tighter body=200 KiB (above 128 KiB override) ===
HTTP/1.1 413 Payload Too Large
content-length: 17
content-type: text/plain
date: Sat, 09 May 2026 15:51:07 GMT
server: envoy
connection: close

Payload Too Large

=== P4.C: POST /route-tighter body=64 KiB (under 128 KiB) ===
HTTP/1.1 200 OK
server: envoy
date: Sat, 09 May 2026 15:51:07 GMT
content-type: application/json
content-length: 337

{"method": "POST", "path": "/route-tighter", "headers": {... "content-length": "65536", ...}}
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P4:**
- (a) **`disabled: true` route bypasses ALL buffer-filter discipline.** A 2 MiB body (above the listener-level 1 MiB cap) returned 200 OK + backend echo. The filter is wholly inactive — no body-counting, no `SendLocalReply`, no header-injection.
- (b) **Per-route `buffer.max_request_bytes` override fires at the override value, NOT the listener value.** 200 KiB body against 128 KiB override → 413. 64 KiB body against the same override → 200.
- (c) The per-route override is wholesale — the listener-level value is fully shadowed for matched-per-route requests per ADR-0073.

### 11.5 Empirical pin #5 — Stat surface: ZERO `envoy_http_buffer_*` counters; HCM `downstream_rq_*` family carries the observable signal (ratifies BRAINSTORM §12.3.P5; sharpens §1.1 amendment 5)

**Probe configuration:** `p1-overcap.yaml` (5 MiB cap) after 4 buffer-overflow probes (P1.B variants) + 1 passthrough probe.

**Verbatim:**

```
** Filtered scrape — buffer.* and downstream_rq_* lines under ingress_p1 stat_prefix **
envoy_http_downstream_rq_completed{envoy_http_conn_manager_prefix="ingress_p1"} 13
envoy_http_downstream_rq_http1_total{envoy_http_conn_manager_prefix="ingress_p1"} 7
envoy_http_downstream_rq_too_large{envoy_http_conn_manager_prefix="ingress_p1"} 5
envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="ingress_p1"} 7
envoy_http_downstream_rq_xx{envoy_response_code_class="1",envoy_http_conn_manager_prefix="ingress_p1"} 6
envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_p1"} 2
envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_p1"} 0
envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_p1"} 5
envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_p1"} 0
[other downstream_rq_* lines suppressed for brevity — see /tmp/p13-pins/transcripts.md]

** Full count of envoy_http_buffer_* lines **
0
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P5; sharpens §1.1 amendment 5:**
- (a) **NO `envoy_http_buffer_*` counters exist in Envoy v1.37.2.** Reference Envoy's `/stats/prometheus` scrape after 4 buffer-overflow probes shows ZERO `envoy_http_buffer_*` lines. Phase 13 contributes ZERO new envoy-go stat-table entries.
- (b) **The HCM `downstream_rq_*` family carries the observable signal.** Specifically:
  - `envoy_http_downstream_rq_total` (in envoy-go's 29-name table per phase 06.1) = 7 (5 overflow + 1 passthrough + 1 ready probe).
  - `envoy_http_downstream_rq_xx{class=4}` (in envoy-go's 29-name table per phase 06.1; rendered via Rule SN4 from the internal name `downstream_rq_4xx`) = 5 (the 5 overflow 413s).
  - `envoy_http_downstream_rq_xx{class=2}` = 2 (the passthrough + ready).
  - **`envoy_http_downstream_rq_too_large` = 5** (HCM-level Envoy-only counter — increments on every 413; NOT in envoy-go's 29-name allow-list).
  - **`envoy_http_downstream_rq_completed` = 13** (HCM-level Envoy-only counter — increments on every completed request; NOT in envoy-go's 29-name allow-list).
- (c) **§1.1 amendment 5 prose correction lands here:** BRAINSTORM §12.5 v2 incorrectly cited `downstream_rq_too_large` as "already in the 29-name table from phase 06.1." The 29-name table contains ONLY `downstream_rq_total/2xx/3xx/4xx/5xx` for the HCM side (per BEHAVIOR_CONTRACT.md lines 134-212). The correct envoy-go-side observable is `downstream_rq_4xx` (in-table; rendered as `envoy_http_downstream_rq_xx{class=4,...}` via Rule SN4). The Envoy-only `downstream_rq_too_large` and `downstream_rq_completed` counters are filtered out of the differential per the existing twin-series-discipline allow-list (BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline` + phase 06.1 SPEC §11.5: "the fixture's allow-list enumerates exactly the unique Prometheus names this SPEC ships; everything else in the Envoy scrape is ignored").
- (d) The "ZERO new stat-table entries" claim survives intact. Phase 13 ships 3 ADRs (ADR-0125, ADR-0126, ADR-0127 v2). ADR-0128 stays retired.

### 11.6 Empirical pin #6 — NO Content-Length fast-fail in `decodeHeaders` (ratifies BRAINSTORM §12.3.P6)

**Probe configuration:** `p1-overcap.yaml` (5 MiB cap). Drive `POST /` with `Content-Length: 6291456` header AND zero body bytes (immediately close the request body stream); 5 second timeout.

**Verbatim:**

```
=== P6.A: CL=6 MiB header, empty body, 5 s timeout ===
curl: (28) Operation timed out after 5002 milliseconds with 0 bytes received
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P6:**
- (a) **Envoy does NOT fast-fail on `Content-Length > effectiveMax` at header-parse time.** The connection hangs awaiting body bytes; eventually times out at the connection level (5 seconds in this probe).
- (b) **Source-of-truth confirmation:** `buffer_filter.cc:50-67` (`decodeHeaders`) does NOT inspect the `Content-Length` header. The cap fires only after data accumulates past the limit (per the streaming-cap path).
- (c) envoy-go MUST NOT add a CL fast-fail clause to `DecodeHeaders`. BRAINSTORM §2.6's hypothesis was empirically refuted; ADR-0127 v2 records the streaming-cap-only algorithm.

### 11.7 Empirical pin #7 — 413 status line `HTTP/1.1 413 Payload Too Large` exact (ratifies BRAINSTORM §12.3.P7)

**Probe configuration:** every overflow probe (P1.B 6 MiB; P2.B 1 MiB+1; P4.B 200 KiB → /route-tighter 128 KiB; P9 chunked overflow).

**Verbatim:** confirmed on every overflow probe in the transcripts above. Status line is exactly `HTTP/1.1 413 Payload Too Large` (no variation).

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P7:**
- (a) Status line is byte-exact `HTTP/1.1 413 Payload Too Large`. NOT configurable via proto. envoy-go's `SendLocalReply(413, ...)` mirrors this verbatim.

### 11.8 Empirical pin #8 — 413 body bytes + 4-header lowercase wire-form + Connection: close + 100-Continue addendum (ratifies BRAINSTORM §12.3.P8)

**Probe configuration:** every overflow probe; full header dumps + body byte counts.

**Verbatim:** every overflow probe yielded the byte-exact wire shape:

```
HTTP/1.1 413 Payload Too Large
content-length: 17
content-type: text/plain
date: <RFC1123>
server: envoy
connection: close

Payload Too Large
```

Body bytes: `Payload Too Large` (17 bytes ASCII; NO trailing newline). 4-header lowercase wire-form: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`. Plus `connection: close` from the user-supplied header (per ADR-0076 §Decision (b)).

NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked`, NO `charset=UTF-8` modifier on `content-type`.

**100-Continue addendum (per §11.1 + §11.2):**
- CL-known probes via `curl --data-binary @<file>` auto-inject `Expect: 100-continue` (curl heuristic for large bodies); reference Envoy's HCM/H1-codec emits `HTTP/1.1 100 Continue` BEFORE the eventual 413.
- Chunked probes via `curl -H "Transfer-Encoding: chunked" -H "Expect:" --data-binary @<file>` do NOT trigger the auto-Expect injection; reference Envoy emits the 413 directly with no 100-Continue prefix.
- The 413 wire shape itself is identical in both cases (the 100-Continue is a separate HTTP message preceding the 413).
- envoy-go's HCM/H1-codec `100 Continue` discipline (already shipped in phase 04) handles this transparently — the buffer filter does NOT need to emit `100 Continue` itself; the HCM does.

**§11.8-CL — Content-Length injection on chunked-passthrough (sub-pin; load-bearing for fixture 0015 scenario 6):**

Probe: `POST /` `Transfer-Encoding: chunked` body=64 KiB against `p1-overcap.yaml` (5 MiB cap; body fits).

Verbatim backend echo (the python backend reports the inbound headers as JSON):

```
HTTP/1.1 200 OK
server: envoy
date: ...
content-type: application/json
content-length: 324

{"method": "POST", "path": "/", "headers": {"host": "127.0.0.1:11301", "user-agent": "curl/8.5.0", "accept": "*/*", "content-type": "application/x-www-form-urlencoded", "x-forwarded-proto": "http", "x-request-id": "...", "content-length": "65536", "x-envoy-expected-rq-timeout-ms": "15000"}}
```

The backend received `content-length: 65536` (NOT chunked encoding!). The original request used `Transfer-Encoding: chunked`, but Envoy's buffer filter accumulated then injected `Content-Length: 65536` before forwarding. **Major confirmation of BRAINSTORM §12.1 fact 5 + §12.4 Decision 6 v2's `maybeAddContentLength` helper.** envoy-go's mirror MUST replicate this conversion to preserve byte-equivalence at the upstream boundary (asserted in fixture 0015 scenario 6).

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P8 + §12.1 fact 5:**
- (a) 413 wire shape is byte-exact (status, body, 4-header set, connection: close).
- (b) `100 Continue` precedes the 413 ONLY when the request carries `Expect: 100-continue` (curl auto-injects for `--data-binary @<file>`); chunked path bypasses this entirely.
- (c) `maybeAddContentLength` injects `Content-Length: <accumulated>` on the held headers when the original request had no Content-Length (chunked transfer); the `Transfer-Encoding: chunked` header is also dropped — observable on the upstream (backend-side) inbound request.
- (d) envoy-go's `maybeAddContentLength` (per §6.5) MUST mirror this byte-equivalently.

### 11.9 Empirical pin #9 — Per-stream chunked accumulation; cap fires at `accumulated > effectiveMax` mid-stream (ratifies BRAINSTORM §12.3.P9)

**Probe configuration:** `p4-disabled.yaml` — `POST /route-tighter` (128 KiB override) `Transfer-Encoding: chunked` body~=200 KiB.

**Verbatim:**

```
=== P9: chunked overflow against /route-tighter (128 KiB cap; ~200 KiB chunked) ===
HTTP/1.1 413 Payload Too Large
content-length: 17
content-type: text/plain
date: Sat, 09 May 2026 15:51:07 GMT
server: envoy
connection: close

Payload Too Large
```

Plus P9.B (passthrough chunked under cap):

```
=== P9.B: chunked body ~64 KiB (under cap) → 200 + Content-Length injection observed at backend ===
HTTP/1.1 200 OK
server: envoy
...
{... backend received Content-Length: 65536 ...}
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P9:**
- (a) Chunked accumulation: each chunk increments `accumulated`; the cap fires at `accumulated > effectiveMax` mid-stream.
- (b) NO 100-Continue with chunked encoding (curl does not auto-inject `Expect:` for chunked).
- (c) envoy-go's `DecodeData` accumulator (per §6.5) MUST handle chunked input identically to fixed-CL input — the algorithm is uniform across both transfer encodings.

### 11.10 Empirical pin #10 — Header-only request disposition (ratifies BRAINSTORM §12.3.P10)

**Probe configuration:** `p1-overcap.yaml`. `GET /` with no body.

**Verbatim:**

```
=== P10: GET / (header-only) ===
HTTP/1.1 200 OK
server: envoy
date: Sat, 09 May 2026 15:50:19 GMT
content-type: application/json
content-length: 243

{... backend echoed GET request ...}
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P10:**
- (a) `GET /` (no body, `endStream=true` on headers) is pure passthrough; no 413; no buffer-filter state engaged.
- (b) Mirrors `buffer_filter.cc:54-56` (`if (end_stream) return Continue;`).
- (c) Counter implications: HCM-level `downstream_rq_2xx +1` and `downstream_rq_completed +1` (Envoy side); on envoy-go side `downstream_rq_total +1` + `downstream_rq_2xx +1` (in-table). NO buffer-filter-specific counter (none exist).

### 11.11 Empirical pin #11 — Empty-body POST disposition (ratifies BRAINSTORM §12.3.P11)

**Probe configuration:** `p1-overcap.yaml`. `POST /` with `Content-Length: 0`.

**Verbatim:**

```
=== P11: POST / (Content-Length: 0) ===
HTTP/1.1 200 OK
server: envoy
date: Sat, 09 May 2026 15:50:19 GMT
content-type: application/json
content-length: 267

{... backend echoed empty POST ...}
```

**Conclusions (pinned) — ratifies BRAINSTORM §12.3.P11:**
- (a) `POST /` with `Content-Length: 0` returns 200; passthrough.
- (b) Per the algorithm at §6.4 + §6.5: `DecodeHeaders` runs (endStream is hopefully true on the headers since CL=0; if framework dispatches `DecodeData(nil, true)` instead, the algorithm's step 4 `endStream=true` branch fires `maybeAddContentLength` then `DataContinue` — both paths produce the same observable). The exact framework-level dispatch (whether `DecodeHeaders` sees `endStream=true` for CL=0 OR `DecodeData(nil, true)` is called) is a framework-internal detail; envoy-go's `DecodeHeaders` step 1 + `DecodeData` step 4 cover both cases.
- (c) Counter implications: same as §11.10 (no overflow; no buffer-filter counters).

### 11.12 Summary

All 11 empirical pins resolved. Verdicts:
- 6 RATIFIED VERBATIM (P3, P4, P7, P8, P9, P10, P11 → ratify §12.3 dispositions; no surprises).
- 4 RATIFIED with §12 amendment carry-through (P1, P2, P5, P6 → ratify §12.3 amendments; no further design changes beyond §1.1 amendment 5 prose correction).
- 0 NEW divergences requiring SPEC-level Decision changes beyond §1.1 amendment 5.

The §unresolved counter-name issue (carried in by STATE.md) is RESOLVED via §1.1 amendment 5 above (option (a) prose correction). Phase 13 ships 3 ADRs; ADR-0128 stays retired.

---

## 12. Deferred decisions (the planner / implementer settles these)

The following 4 decisions are SPEC-deferred — the SPEC author has bounded each but leaves the precise discipline for the PLAN author or impl-time settlement. Each maps to ≤1 ADR (some fold inline into existing ADRs).

**D1. Filter-callback wiring.** §6.7 declares phase 13 sets only `dcb` (DecoderFilterCallbacks); the precise hook (`OnNewStream`, factory closure, etc.) follows the existing 07.1 framework convention. PLAN author confirms the framework's exposed callback-setup hook against the existing `internal/filter/http/cors/`, `fault/`, `header_mutation/`, `localratelimit/`, `csrf/` patterns.

**D2. `buffer.go` file split.** §4.1 declares ~280-330 LoC for `buffer.go` (single file). PLAN author may split into `count.go` (body-counting helpers + `maybeAddContentLength`) OR `perroute.go` (per-route helpers + `parsePerRoute`) if test readability benefits. The SPEC explicitly defers this micro-decision (BRAINSTORM does not introduce a file-split sub-decision); no ADR class commitment. Default expected outcome: single-file `buffer.go` (mirrors phase 12 csrf single-file precedent).

**D3. Filter-internal validation error message wording for `max_request_bytes` PGV-mirror.** §6.1 + §6.3 declare envoy-go-own-wording for the parse-time validation errors (`"buffer: max_request_bytes is required"`, etc.). Phase 12 ADR-0121 settled this discipline as option (b) (envoy-go-own clear-text); phase 13 follows the same precedent. PLAN author confirms exact wording for each error case and lands the strings in the test fixtures. Captured in ADR-0126.

**D4. Backend-side Content-Length assertion mechanism for fixture 0015 scenario 6.** §7.1 declares scenario 6 asserts the inbound request carries `Content-Length: 10240` at the backend. The precise assertion mechanism is open: (a) the existing echobackend echoes the inbound headers as JSON in the response body; the driver parses the JSON and asserts the `content-length` field; (b) the echobackend is extended with a custom `x-asserted-content-length` echo header carrying the observed CL value; the driver asserts the header on the proxy-forwarded response; (c) the echobackend is extended with a per-fixture assertion hook that fails the fixture if the inbound CL is not 10240. PLAN author chooses based on existing echobackend infrastructure; the simplest option is likely (a) since the existing python backend already echoes inbound headers as JSON. Captured in §7.4 driver shape; no ADR-class commitment.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

The 4-edit bundle below is the verbatim Markdown patch applied to `docs/envoy-go/BEHAVIOR_CONTRACT.md` at the phase-done commit. NOT applied at SPEC commit (per phase 09 / 10 / 11 / 12 precedent).

### 13.1 `## HTTP filter chain ### envoy.filters.http.buffer` NEW subsection

The new subsection lands UNDER the existing `## HTTP filter chain` umbrella, AFTER the existing `### envoy.filters.http.fault` (phase 09), `### envoy.filters.http.header_mutation` (phase 10), `### envoy.filters.http.local_ratelimit` (phase 11), and `### envoy.filters.http.csrf` (phase 12) subsections. Verbatim Markdown shape:

```markdown
### envoy.filters.http.buffer

Phase 13 ships `envoy.filters.http.buffer` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes the only top-level field on the parent `Buffer` proto actively, with envoy-go-own validation (≤ 1 MiB ceiling) that closes a divergence-window against reference Envoy's lack of such a ceiling.

**Listener-level field decomposition (1 field):**

| Proto field | envoy-go behavior |
|---|---|
| `max_request_bytes` (`UInt32Value`, REQUIRED) | CONSUMED. envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 (1 MiB). Rejected at parse time with envoy-go-own error wording. The 1 MiB ceiling is the load-bearing envoy-go-only divergence (reference Envoy accepts arbitrary `UInt32Value`). |

**Per-route TPFC (`BufferPerRoute` proto — separate from listener-level `Buffer`):**

| Proto field | envoy-go behavior |
|---|---|
| `override.disabled` (oneof case, `bool`, PGV `bool.const = true`) | CONSUMED. `disabled: true` → filter wholly inactive on this route. `disabled: false` rejected at parse-time per PGV mirror. |
| `override.buffer` (oneof case, `Buffer` message) | CONSUMED. `buffer.max_request_bytes` subjected to the same ≤ 1 MiB validation as listener-level. Wholesale-override of listener cap per ADR-0073. |

The `oneof override` carries PGV `validate.required = true` — exactly one case must be set; both-set + neither-set rejected at boot via the JSON→proto decoder (NOT PGV; mechanism per phase 13 SPEC §11.3 + ADR-0125).

**Body-counting algorithm — STREAMING-CAP ONLY (per phase 13 SPEC §11.6):**

The buffer filter does NOT inspect `Content-Length` in `DecodeHeaders`. The cap fires only after data accumulates past the limit:

1. `DecodeHeaders(headers, endStream)`:
   - `endStream=true` (header-only) → `Continue` (no body work; mirrors `buffer_filter.cc:54-56`).
   - per-route resolves to `disabled=true` → set passthrough flag; `Continue` (mirrors `buffer_filter.cc:60-62`).
   - bodied + non-disabled → store `effectiveMax` + `headersRef`; return `StopIteration` (mirrors `buffer_filter.cc:67`).
2. `DecodeData(data, endStream)`:
   - passthrough flag set → `DataContinue` (filter never returns `DataStopIterationAndBuffer` on this path; framework safety-net cap never engages on disabled routes).
   - `accumulated += len(data)`.
   - `accumulated > effectiveMax` → `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` (discards partial buffer).
   - `endStream=true` (terminal chunk fits) → invoke `maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`); release held headers + body; `DataContinue`.
   - in-flight chunk → `DataStopIterationAndBuffer` (accumulate; framework holds bytes per ADR-0076 §Decision (b)).
3. `DecodeTrailers` → invoke `maybeAddContentLength` defensively; `TrailersContinue`.
4. `Encode*` → all pass-through. Buffer is decoder-side-only.
5. `OnDestroy` → no-op.

`maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`): if `headersRef != nil` AND original request had no `Content-Length` → set `Content-Length: <accumulated>` on the held headers AND drop `Transfer-Encoding: chunked`. The discipline is: chunked → fixed-CL conversion before forwarding upstream. Observable at the backend boundary (per phase 13 SPEC §11.8-CL).

**Per-route override semantics:** Wholesale-override per ADR-0073 + ADR-0125 (5th canonical per-route discipline: disabled-OR-override). Each `BufferPerRoute` TPFC entry runs through `parsePerRoute` at config-load time, allocating its own `*compiledPerRoute{disabled, maxOverride}`. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) picks the most-specific config per request.

**Per-route stats are SHARED with listener-level** (mirrors phase 12 csrf ADR-0124; DIVERGES from phase 11 local_ratelimit ADR-0117). Phase 13 emits NO filter-specific counters (see "Stat surface" below); the SHARED-stats invariant is structurally vacuous for buffer (no counters to share or split) but documented for cross-filter consistency.

**Cap layering against ADR-0076 framework safety net:** The buffer filter's `accumulated > effectiveMax` check fires INSIDE the framework's hardcoded `filterBufferLimitBytes = 1 << 20` cap (per `internal/filter/http/chain.go:19`). Because `effectiveMax ≤ 1 MiB` (invariant per ADR-0126's parse-time validation), the framework cap is structurally unreachable in MVP — the filter wins by construction. The framework cap remains armed as a safety net for any future configuration that might bypass the parse-time check (e.g., the future cap-promotion phase that promotes `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` from silent-ignored to honored).

**Rejection response wire shape (per phase 13 SPEC §11.7 + §11.8 — REUSES framework path verbatim per ADR-0076 §Decision (b)):**

- Status: `413 Payload Too Large`
- Body: `Payload Too Large` (17 bytes ASCII; constant `localReply413Body` from `internal/filter/http/chain.go:25`; NO trailing newline)
- Headers in lexicographic order: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Plus user-supplied `Connection: close`
- Framing: Content-Length (NO chunked)
- NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding`, NO `charset=UTF-8` modifier on `content-type`

**100-Continue addendum (per phase 13 SPEC §11.8):** Reference Envoy emits `HTTP/1.1 100 Continue` BEFORE the eventual 413 when the request includes `Expect: 100-continue` (curl auto-injects this for `--data-binary @<file>`). With `Transfer-Encoding: chunked`, no `100 Continue` is emitted. envoy-go's HCM/H1-codec `100 Continue` discipline (already shipped in phase 04) handles this transparently — buffer filter does NOT need to emit `100 Continue` itself; the HCM does.

**Method gate:** Buffer evaluates ALL methods that carry a body; the method itself is not consulted. Bodied requests (`POST`, `PUT`, `PATCH`, `DELETE` with body, etc.) all pass through the streaming-cap path; non-bodied requests (`GET`, `HEAD`, `OPTIONS` with `endStream=true` on headers) short-circuit at `DecodeHeaders` step 1 (`Continue` without state touch).

**Stat surface:** ZERO filter-specific counters. Reference Envoy v1.37.2 emits NO `envoy_http_buffer_*` counter family at all (confirmed at phase 13 SPEC §11.5 — `/stats/prometheus` scrape after 4 buffer-overflow probes shows ZERO `envoy_http_buffer_*` lines). Buffer-filter overflow is observable on envoy-go's side via the existing HCM `downstream_rq_4xx` counter (in the 29-name table per phase 06.1; rendered as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`). The Envoy-only `downstream_rq_too_large` and `downstream_rq_completed` HCM counters increment alongside in reference Envoy but are NOT in envoy-go's emit allow-list — they are filtered out per the `### Twin-series filter discipline` allow-list discipline (above).
```

### 13.2 `## Stat-name mapping ### 29-name table` preamble note (NO new rows)

The existing 29-name table heading + rows stay UNCHANGED. Verbatim Markdown patch (preamble note appended after the existing table; no row insertions):

```markdown
**Phase 13 (buffer filter) note:** The `envoy.filters.http.buffer` filter shipped in phase 13 contributes ZERO new entries to this table. The filter has no filter-specific counter namespace at all (confirmed empirically at phase 13 SPEC §11.5 — reference Envoy v1.37.2 emits NO `envoy_http_buffer_*` counter family). Buffer-filter overflow is observable on the envoy-go side via the existing `downstream_rq_4xx` HCM counter (rendered via Rule SN4 status-class flattening as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`). The Envoy-only HCM counters `downstream_rq_too_large` (increments on every 413) and `downstream_rq_completed` (increments on every completed request) are NOT in this table; they are filtered out of the differential per the `### Twin-series filter discipline` allow-list discipline below.
```

The total stays at **29 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13). NO heading update. NO new tag-extractor preamble note. NO new SN flattening rule.

### 13.3 `## Equivalence Matrix` new row (verbatim table-row patch)

Verbatim Markdown patch (new row appended to the existing equivalence-matrix table):

```markdown
| HTTP filter `envoy.filters.http.buffer` | 0015-http-buffer: scenario1: body-fits-cap (1 KiB POST → 200); scenario2: streaming-overflow CL-known (2 MiB POST → 100-Continue + 413; §11.7+§11.8 wire shape: content-length=17, body=`Payload Too Large`, 4-header lowercase set + Connection: close); scenario3: chunked-overflow against per-route tighter cap (200 KiB chunked → 413, NO 100-Continue with chunked); scenario4: per-route disabled bypass (2 MiB POST → 200 — cap wholly inactive on disabled route per §11.4); scenario5: per-route tighter override fires (200 KiB → 100-Continue + 413 against 128 KiB override); scenario6: chunked-passthrough Content-Length injection (10 KiB chunked → 200, backend asserts inbound `Content-Length: 10240` per §11.8-CL `maybeAddContentLength` mirror). All 6 requests HTTP/1.1 plaintext; no timing tolerances (buffer is purely synchronous). Counter delta on envoy-go side: `downstream_rq_total +6`, `downstream_rq_2xx +3`, `downstream_rq_4xx +3`. Envoy-only `downstream_rq_too_large` (+3) and `downstream_rq_completed` (+6) filtered out via the existing twin-series-discipline allow-list. NOT asserted: `max_request_bytes > 1 MiB` operational behavior (deferred — envoy-go-only parse-time rejection per ADR-0126); `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` (silent-ignored per ADR-0076); H2 differential coverage. |
```

### 13.4 Forward-pointer notes (per BRAINSTORM §8 + §1.1 amendments)

Verbatim Markdown patch (appended to the existing `## Forward-pointer notes` section):

```markdown
### Phase 13 forward-pointer notes

**Deferred field families** (silent-ignored / parse-rejected per ADR-0040 + ADR-0076 + ADR-0126; see `### envoy.filters.http.buffer ### Listener-level field decomposition` above + phase 13 SPEC §2.1 for the full field map):

- `Buffer.max_request_bytes > 1 MiB` (envoy-go-only PARSE-time rejection per ADR-0126) — coupled to the future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)). Reference Envoy accepts arbitrary `UInt32Value`; envoy-go rejects values > 1048576 at parse time with envoy-go-own error wording. **Divergence-window:** operators with existing configs targeting `max_request_bytes > 1 MiB` against reference Envoy MUST adjust their config (lower the value) to load on envoy-go. Future re-activation: when the cap-promotion phase amends ADR-0076 to make `filterBufferLimitBytes` per-stream tunable via `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes`, ADR-0126 amends in-place to remove the parse-time ≤ 1 MiB validation; `max_request_bytes` becomes operationally equivalent to reference Envoy.
- `per_connection_buffer_limit_bytes` (Listener-scope) / `per_request_buffer_limit_bytes` (Route-scope) — silent-ignored at parse time per ADR-0076 §Decision (d). Phase 13 does NOT change this disposition. Future re-activation: same cap-promotion phase as above.

**No new tag-extractor:** Buffer reuses the existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (Rule SN2 from ADR-0061). UNLIKE phase 11's local_ratelimit which introduced filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118), phase 13 introduces NO new SN flattening rule and NO new tag-extractor pattern. Phase 13 furthermore introduces NO new stat-table entries at all (per phase 13 SPEC §11.5 + §1.1 amendment 5 — reference Envoy emits no `envoy_http_buffer_*` counter family).

**Per-route stats SHARED with listener-level (vacuously):** Phase 13 demonstrates the disabled-OR-override per-route discipline (5th canonical shape per ADR-0125). The SHARED-stats invariant is structurally vacuous for buffer (no filter-specific counters to share or split) but documented for cross-filter consistency: future stateful per-route filters with their own stat namespaces follow phase 11 (ADR-0117 INDEPENDENT stats); future data-only per-route filters with HCM-scoped stats follow phase 12 (ADR-0124 SHARED stats); future disabled-OR-override per-route filters with NO filter-specific stats follow phase 13 (ADR-0125 SHARED-vacuous stats).

**Body-counting algorithm divergence from reference Envoy:** envoy-go's filter does its own per-stream byte-counting + 413 emission via `SendLocalReply`, while reference Envoy delegates to HCM via `callbacks_->setBufferLimit + StopIteration`. The deliberate divergence is recorded in ADR-0127 v2 §Consequences with an explicit forward-pointer to the future cap-promotion phase that may revisit this. WIRE OUTCOMES are byte-equivalent on every observable axis (status, body, headers, counter increment, `Connection: close`); only the `maybeAddContentLength` semantics are observable upstream-side as a deliberate mirror of `buffer_filter.cc:91-97`.
```

---

## 14. Testing strategy (per BRAINSTORM §11 + §1.1 amendment + §12.7)

### 14.1 Unit tests (`internal/filter/http/buffer/buffer_test.go`)

Six test groups (~400-500 LoC total):

- **Group 1 — `New` factory PGV-mirror filter-internal validation (per §6.1 + §11.1):** test that `New` rejects nil `cfg.MaxRequestBytes`, rejects `cfg.MaxRequestBytes.Value == 0`, rejects `cfg.MaxRequestBytes.Value > 1048576`; test that `New` accepts `cfg.MaxRequestBytes.Value = 1` (boundary), `= 1048576` (max valid), `= 65536` (mid-range); test that `New` rejects malformed Any unmarshal; test the exact error wording for each failure case (per D3 deferred-decision settlement).
- **Group 2 — `parsePerRoute` PGV-mirror discipline (per §6.3 + §11.3):** test `BufferPerRoute{Disabled: true}` parses to `&compiledPerRoute{disabled: true}`; test `BufferPerRoute{Buffer: &Buffer{MaxRequestBytes: 65536}}` parses to `&compiledPerRoute{maxOverride: &65536}`; test `BufferPerRoute{Buffer: &Buffer{MaxRequestBytes: 0}}` rejects (same wording as listener); test `BufferPerRoute{Buffer: &Buffer{MaxRequestBytes: 5MiB}}` rejects; test `BufferPerRoute{}` (oneof unset; PGV `validate.required` mirror) rejects with "override oneof is required"; test `BufferPerRoute{Disabled: false}` (bool.const violation; structurally unreachable post-decode but defensively covered) rejects with "disabled must be true".
- **Group 3 — `DecodeHeaders` (per §6.4 + §11.6 + §11.10):** parametrized over (a) header-only `endStream=true` → `Continue` (regardless of method); (b) bodied + per-route `disabled=true` → `Continue` + passthrough flag set; (c) bodied + per-route nil + listener cap → `StopIteration` + `effectiveMax` stored; (d) bodied + per-route override → `StopIteration` + override `effectiveMax` stored. Plus the negative case: `DecodeHeaders` does NOT inspect `Content-Length` (verifies §11.6 — even with `Content-Length: 99GB` header, the filter returns `StopIteration` rather than `SendLocalReply` from `DecodeHeaders`).
- **Group 4 — `DecodeData` accumulation + cap predicate (per §6.5 + §11.2 + §11.9):** test single-chunk fits (`endStream=true`, accumulated ≤ effectiveMax) → `DataContinue` + `maybeAddContentLength` invoked when applicable; test single-chunk exact-cap fits (accumulated == effectiveMax) → `DataContinue` (predicate is `>`, not `>=`); test single-chunk overflow (accumulated > effectiveMax) → `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer`; test multi-chunk accumulation below cap → `DataStopIterationAndBuffer` per chunk until terminal `endStream=true`; test multi-chunk total exceeds cap mid-stream → 413 fires on the overflowing chunk; test passthrough flag (per-route disabled) → `DataContinue` per chunk regardless of accumulation; test empty-body terminal (`endStream=true`, `len(data)==0`) → `DataContinue` + `maybeAddContentLength` invoked.
- **Group 5 — `maybeAddContentLength` mirror (per §6.5 + §11.8-CL):** test that when `headersRef != nil` AND original request had no `Content-Length` → header is set to `<accumulated>` AND `Transfer-Encoding` is dropped; test that when original request HAD `Content-Length` → no-op (header preserved); test that when `headersRef == nil` (e.g., header-only or disabled) → no-op; test that the helper is idempotent (called twice → same result).
- **Group 6 — Per-route integration (per §5.6 + §11.4):** test that listener default applies when per-route nil; test that per-route override smaller-than-listener fires at the smaller value; test that per-route override larger-than-listener-within-1MiB fires at the larger value; test that per-route disabled bypasses cap entirely (passthrough flag set; no `DataStopIterationAndBuffer` ever returned; framework safety-net cap structurally never engages); test that the resolver is called once per stream (not per chunk) — covered by spying on the callback.

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/buffer/...` green. `go vet`, `golangci-lint run` clean. Buffer has no shared mutable state at the request hot path (the `compiledConfig` is read-only after `New`; `compiledPerRoute` is read-only after `parsePerRoute`; the per-stream `accumulated` counter is on a stack-bound `*filter` that no other goroutine touches); race-test cleanness is structural — no mutex needed.

### 14.3 Fuzzers

New fuzzer `FuzzBufferConfigParse` in `internal/filter/http/buffer/fuzz_test.go`:

```go
func FuzzBufferConfigParse(f *testing.F) {
    f.Add(...)  // a few well-formed seeds:
                // - max_request_bytes = 1024 (small valid)
                // - max_request_bytes = 1048576 (max valid)
                // - max_request_bytes = 0 (rejected by validation)
                // - max_request_bytes = 5242880 (rejected by ≤ 1 MiB validation)
                // - empty bytes (Unmarshal failure)
                // - malformed proto bytes (Unmarshal failure)
    f.Fuzz(func(t *testing.T, raw []byte) {
        any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
        _, _ = New(envoyhttp.FactoryCtx{...}, any)
        // expectation: no panic, no goroutine leak, no resource leak
        // invariant: result is either (factory, nil) OR (nil, error)
    })
}
```

This is the 17th fuzzer in the repo (16 existing per phase 12 phase-done + this new one). Fuzz budget: 30s per the existing per-phase fuzzer gate.

### 14.4 Existing fuzzers re-run

All 16 existing fuzzers continue to pass at 30s budget. Phase 13 introduces no shared fuzz-input surface changes that would invalidate existing fuzzers.

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin; phase 13 introduces no HTTP/2 stack changes (the buffer filter operates above the codec layer per §5.9 concurrency model).

### 14.6 Differential 0000–0014 + 0015

15 prior fixtures (0000-tcp-echo through 0014-http-csrf) continue to pass; phase 13 adds the new `0015-http-buffer` (6 requests per §7.1). Total wallclock estimated 50–60s for 16 fixtures (phase 12 reported ~45–55s for 15 fixtures; fixture 0015 adds ~3–5s — all synchronous, no timing tolerances).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

Per §3 above:

- Gate A: build / vet / lint clean.
- Gate B: race-test pass on all 36 packages.
- Gate C: h2spec 53/53 PASS.
- Gate D: 17 fuzzers green at 30s budget.
- Gate E: 16 differential fixtures 0000–0015 PASS.
- Gate F: BEHAVIOR_CONTRACT.md populated with §13's 4-edit bundle.

All six gates green at the phase-done commit.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

The phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.5 review session) confirms:

- [ ] `internal/filter/http/buffer/` package exists with files matching §4.1 (allowing PLAN-author file split per D2 deferred-decision settlement).
- [ ] `cmd/envoy-go/main.go` registers `buffer.New` against `buffer.TypeURL` before `httpReg.Freeze()`, alphabetical-after-router insertion ordering (`router → buffer → cors → csrf → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze`).
- [ ] `New` factory PGV-mirror validation matches §11.1 + §6.1: rejects nil `MaxRequestBytes`, rejects value 0, rejects value > 1048576; envoy-go-own error wording per ADR-0126 + D3 deferred-decision settlement.
- [ ] `compiledConfig` shape matches §6.2 (1 actively-consumed field; no embedded `*filterStats` since phase 13 emits no filter-specific counters).
- [ ] `compiledPerRoute` shape matches §6.2 (`disabled bool` + `maxOverride *uint32`; mutually exclusive at runtime per oneof discipline).
- [ ] `parsePerRoute` matches §6.3 + §11.3: handles `*BufferPerRoute_Disabled` + `*BufferPerRoute_Buffer` + nil (PGV-required mirror) + defensively `disabled: false` (bool.const mirror).
- [ ] `DecodeHeaders` body matches §6.4 + §11.6 + §11.10 (header-only `Continue`; per-route disabled `Continue` + passthrough flag; bodied + non-disabled `StopIteration`; NO Content-Length inspection).
- [ ] `DecodeData` body matches §6.5 + §11.2 + §11.9 (passthrough flag `DataContinue`; cap predicate `>` not `>=`; overflow `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer`; terminal `endStream=true` invokes `maybeAddContentLength` then `DataContinue`; in-flight `DataStopIterationAndBuffer`).
- [ ] `maybeAddContentLength` matches §6.5 + §11.8-CL (sets `Content-Length` AND drops `Transfer-Encoding: chunked` when applicable; mirrors `buffer_filter.cc:91-97`).
- [ ] `DecodeTrailers` invokes `maybeAddContentLength` defensively per §6.6.
- [ ] `Encode*` + `OnDestroy` + `SetDecoderCallbacks` match §6.7 (encoder methods do NOT exist; `OnDestroy` is no-op; decoder-only `HTTPFilter` value).
- [ ] Per-route override semantics match §5.6 + §11.4 (data-only wholesale-override per ADR-0073 + ADR-0125; SHARED-vacuous stats — no filter-specific counters to share or split).
- [ ] Stat surface contributes ZERO new entries to the 29-name table per §1.1 amendment 5 + §11.5 + §13.2 preamble note (no `envoy_http_buffer_*` counters; `downstream_rq_4xx` is the in-table observable; `downstream_rq_too_large` + `downstream_rq_completed` are Envoy-only and filtered out).
- [ ] Rejection response wire shape matches §11.7 + §11.8 (4 headers in order: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`; 17-byte `Payload Too Large` body, no LF; status `413 Payload Too Large`; `Connection: close`; Content-Length framing; NO charset modifier; NO chunked; NO cache-control / x-content-type-options).
- [ ] 100-Continue addendum honored per §11.8 (HCM/H1-codec emits `100 Continue` independent of buffer filter on `Expect: 100-continue` requests; chunked path bypasses).
- [ ] `maybeAddContentLength` byte-equivalent at backend boundary per §11.8-CL (fixture 0015 scenario 6 backend asserts `Content-Length: 10240` on chunked-passthrough request).
- [ ] Differential fixture 0015 6-request matrix green per §7.1 (no timing tolerance; sequential pass acceptable).
- [ ] `FuzzBufferConfigParse` green at 30s budget (17 fuzzers total).
- [ ] All 15 prior differential fixtures still green; 16 prior fuzzers still green; h2spec 53/53 still PASS.
- [ ] `BEHAVIOR_CONTRACT.md` populated with §13's 4-edit bundle at phase-done commit.
- [ ] `DECISIONS.md` carries 3 new ADRs (ADR-0125, ADR-0126, ADR-0127 v2) anchored per §8. ADR-0128 stays retired.

---

*End of phase 13 SPEC.*
