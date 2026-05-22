# Phase 24.2 SPEC — `envoy.filters.http.ratelimit` (remaining actions + X-RateLimit headers + `RateLimitPerRoute`)

> **Lifecycle state:** SPEC.md authored (carved from the phase-24 parent master SPEC at the PLAN-time ADR-0045 split, ADR-0201); ROADMAP row `24.2` added `planned` at this split commit (depends-on `24.1`; parent row `24` stays `in-progress`; row `24.1` is `in-progress` and ships first). **24.2 becomes active only after 24.1 phase-done.** When activated, the successor session detects SKILL_ROUTING state 2 (SPEC exists, PLAN does not) and runs `superpowers:writing-plans` to author `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/PLAN.md`. This SPEC slice may be refined in-place at 24.2's lifecycle-state 1 if 24.1 IMPL surfaces a learning (the 18.2 precedent); the parent SPEC's empirical AMENDs are SETTLED.
>
> **Parent:** `docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` (the RETAINED parent master SPEC — carries the full §1.1 **AMEND-1..11 catalog**, the §4 descriptor engine [incl. §4.3 Axis-B composition + §4.4 stage filter + §4.7 X-RateLimit byte-pin], the §5.3 `RateLimitPerRoute`, the §6 code shapes, the §7 differential, the §10 ADR anchor map, the §11 D1–D7 matrix, the §13 BEHAVIOR_CONTRACT bundle, the §14 testing taxonomy, the §15 acceptance checklist, the §16 split axis). **This 24.2 SPEC details the remaining-surface slice only; it REFERENCES the parent's §1.1/§4.3/§4.4/§4.7/§5.3/§6/§7/§10/§11/§13/§14/§15 rather than repeating them.**
>
> **Sibling predecessor:** `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/SPEC.md` (the foundational core decision path + DELTA-1 `RateLimitClient` + DELTA-2 route-table exposure + the FULL PARSE-REJECT roster + the cluster-scoped stat surface + the `0032`/`0033` fixture directories). 24.2 builds the remaining actions + headers + per-route surface ON TOP of 24.1's landed package.
>
> **Authored:** 2026-05-22 (at the phase-24 PLAN-time split commit; ADR-0201).

---

## 1. Purpose (24.2 slice)

Phase 24.2 completes `envoy.filters.http.ratelimit` by adding, on top of 24.1's core decision path:

1. **The remaining 5 descriptor actions** — `source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match` (the other half of the parent §4.1 10-action engine). Includes the AMEND-11 key defaults: `query_parameters`→**"query_param"** (singular), `query_parameter_value_match`→"query_match"; `metadata` requires a config key; `masked_remote_address` CIDR-masking (`v4/v6_prefix_mask_len`); `source_cluster` always-true; the `metadata` value-extraction accessor (parent §12 item 2 — `streamInfo().dynamicMetadata()` DYNAMIC vs route-metadata ROUTE_ENTRY, confirmed at 24.2 IMPL against the existing stream-info accessor). (Parent §4.1 remaining rows.)
2. **The X-RateLimit DRAFT_VERSION_03 response headers** — `encode.go` + `headers.go` (parent §6.6 + §6.10): `x-ratelimit-limit/-remaining/-reset` emitted on ALL dispositions (OK/Error/OverLimit) when `enable_x_ratelimit_headers == DRAFT_VERSION_03`, driven by the MIN `limit_remaining` descriptor-status, with the `, <rpu>;w=<window_sec>[;name="<n>"]` quota-policy suffix + the unit→seconds map (parent §4.7 + AMEND-8; the byte format is parent §12 item 5, settled at 24.2 IMPL `headers_test.go`). Added to the BEHAVIOR_CONTRACT §7.2 response-header allow-list.
3. **`RateLimitPerRoute` — the NEW 10th canonical per-route shape** + the **ADR-0125 roster amendment 9 → 10** (`compiled_perroute.go`; parent §5.3; ADR-0199). `vh_rate_limits` (OVERRIDE/INCLUDE/IGNORE) honored; `override_option` PARSE-ACCEPTED-but-IGNORED (INERT per AMEND-4); `rate_limits[]` as the Axis-A embedded early-return; per-route `domain` override. RE-AMENDS after phase-23's REUSE-by-absence skip. The canonical-per-route roster grows **9 → 10** at this IMPL.
4. **The `stage` multi-stage path** — parse-time bucketing of route/vhost policies by `stage` (0-10) + per-request selection of the filter-stage bucket (parent §4.4). 24.1 evaluated only the default stage-0 bucket; 24.2 generalizes.
5. **The Axis-B `vh_rate_limits` cross-tier composition decision table** — the full INCLUDE / IGNORE / OVERRIDE table + the legacy `RouteAction.include_vh_rate_limits=true` force-include (parent §4.3 Axis B + AMEND-5). 24.1 covered only the OVERRIDE default.

## 2. Non-purposes (24.2)

- Everything already landed at 24.1 (the package skeleton, `compiledConfig`, `RateLimitClient`, the 5 core actions, the dispositions, the stat surface, DELTA 2, the PARSE-REJECT roster, the `0032`/`0033` directories) is NOT re-landed — 24.2 EXTENDS the existing package.
- All parent §2 non-purposes (RTDS/runtime keying; `extension`/`dynamic_metadata` actions PARSE-REJECTed at 24.1; `RateLimit.Override` honor-as-absent; descriptor-value formatter syntax; gauges; multi-worker aggregation; `apply_on_stream_done`) apply.
- No NEW fixture directory and no NEW fuzzer (24.2 ADDS scenarios to the existing `0032` and seeds to the existing `FuzzRateLimitConfigParse` corpus).

## 3. Differential envelope (24.2 slice)

24.2 ADDS to the EXISTING `0032-http-ratelimit` directory (created at 24.1) — it creates NO new fixture dir (the dir count stays 35):

- **(f) `vh_inclusion`** [cross-side byte-exact] — INCLUDE vs OVERRIDE vs IGNORE drives which tier's descriptors are sent (parent §4.3 Axis B).
- **(g) `x_ratelimit_headers`** [cross-side byte-exact] — `enable_x_ratelimit_headers: DRAFT_VERSION_03` + the fake returns descriptor-statuses with `current_limit`/`limit_remaining`/`duration_until_reset` ⇒ `x-ratelimit-limit/-remaining/-reset` byte-exact (parent §7.1 scenario g + AMEND-8).
- **Scenario (d) `descriptor_actions` EXTENDED** with the remaining 5 actions (`source_cluster`/`masked_remote_address`/`metadata`/`query_parameters`/`query_parameter_value_match`) so the cross-side fake sees the full 10-action descriptor set.

The shared fake `RateLimitService` (`test/helpers/ratelimitgrpc/`) + the `fixture.HTTPGlobalRateLimitGRPC` BackendKind + the fixture `driver.go` already exist from 24.1; 24.2 extends the deterministic script map for the new scenarios.

## 4. Source-file roster (24.2 subset of parent §6.10)

| File | 24.2 scope | Anticipated LoC |
|---|---|---|
| `ratelimit/descriptors.go` | EXTEND the 24.1 engine with the remaining 5 actions + the §4.4 stage bucketing + the §4.3 Axis-B `vh_rate_limits` table + legacy force-include | ~150–250 (added) |
| `ratelimit/encode.go` | X-RateLimit header injection on all dispositions when enabled (parent §6.6) | ~80–140 |
| `ratelimit/headers.go` | DRAFT_VERSION_03 header construction — MIN-status + `;w=`/`;name=` quota suffix + unit→seconds (parent §4.7) | ~80–120 |
| `ratelimit/compiled_perroute.go` | `RateLimitPerRoute` TPFC compile (parent §6.7) | ~80–120 |
| `ratelimit/*_test.go` | parent §14.1 Layer A for the 24.2 surface (remaining-action descriptors; X-RateLimit MIN-status; perroute; stage; Axis-B) | ~300–500 (added) |
| `docs/envoy-go/DECISIONS.md` (ADR-0125) | the canonical-per-route roster amendment 9 → 10 (anchored in ADR-0199) | docs |

## 5. ADR landing (24.2)

Per the parent §10 anchor map + ADR-0201 split mapping, 24.2 lands:

- **ADR-0199 (FULL)** — §Decision + §Consequences for `RateLimitPerRoute` (the 10th canonical "data-only-with-vh-inclusion-enum") + the **ADR-0125 roster amendment 9 → 10** (the IN-PLACE ADR-0125 amendment paragraph).
- **ADR-0197 (REMAINING slice)** — §Decision + §Consequences for the X-RateLimit DRAFT_VERSION_03 headers + the remaining-actions descriptor coverage (the CORE slice landed at 24.1).

The §Context drafts for ADR-0197/0199 are already anchored at the parent SPEC commit. **Escape-valve reserve: ADR-0202** (could fire at 24.2 IMPL for the X-RateLimit MIN-status quota-policy byte-edge or the `metadata`-action accessor path; hypothesized UNCONSUMED at phase-24 phase-done per the parent §10-C D-hypothesis, re-mapped across the sub-phases).

## 6. Six-gate checklist (24.2)

Identical matrix to the parent §7.4, scoped to the 24.2 surface (the fixture count stays 35 — 24.2 adds scenarios, not dirs):

- **Gate A — build:** clean across the extended `ratelimit/` package.
- **Gate B — vet + lint:** clean; no new suppressions.
- **Gate C — race:** clean (the encode-side header injection + the per-route compile).
- **Gate D — differential:** **35/35** GREEN; `0032` (f)/(g) cross-side byte-exact + (d) extended with the remaining actions; the 24.1 scenarios stay GREEN.
- **Gate E — fuzz:** `FuzzRateLimitConfigParse` clean at 30s/seed with the extended corpus; no panics across 33 fuzzers.
- **Gate F — h2spec:** 53/53 PASS at the ADR-0051 pin.

All six GREEN ⇒ the row-24.2 status flip `planned → done`. **The 24.2 phase-done commit ALSO flips the parent row `24` `in-progress → done`** (the rollup discipline — the commit-message body names both transitions for grep-verifiability, per the 18/19/22 precedent), and verifies the FULL parent §15 acceptance UNION.

## 7. Acceptance checklist (24.2 — completes the parent §15 UNION)

The 24.2 reviewer confirms the parent §15 items not closed at 24.1: the X-RateLimit DRAFT_VERSION_03 headers (item 12); the `RateLimitPerRoute` 10th canonical + ADR-0125 9→10 (item 14); the remaining descriptor-action fidelity (item 9, completion); the `0032` (f)/(g) + (d)-extension differential coverage (item 7, completion); ADR-0199 + the ADR-0197 remaining slice landed (item 15, completion); the BEHAVIOR_CONTRACT per-route + response-header-allow-list additions (item 16, completion); and the **doc-state UNION** at parent §15 item 17 — DECISIONS.md ADR-0197..0200 full bodies + the ADR-0125 9→10 amendment at final state; next-free ADR-0202 (D-hypothesis: ADR-0202 UNCONSUMED at phase-done, or the escape-valve disposition recorded); STATE.md re-advanced; **ROADMAP parent row 24 flipped to `done`** (rollup with row 24.2); 19 HTTP filters wired; §9 family at **1 remaining row** (`wasm`); and the end-to-end audit-trail (item 18) across the parent SPEC → 24.1 + 24.2 (SPEC → PLAN → PROGRESS → REVIEW) chain.
