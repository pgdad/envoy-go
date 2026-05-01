# Phase 07.2 — Listener-chain completion (sibling SPEC stub)

**Phase id:** `07.2`
**Slug:** `07.2-listener-chain-completion`
**Status:** `planned` (sibling SPEC stub; full SPEC drafted at 07.2's own lifecycle-state-1 brainstorm + state-2 SPEC session)
**Depends on:** `07.1-http-filter-framework` (planner-time-ordered; not architecturally dependent — see BRAINSTORM §1)
**Parent phase:** `07-filter-chain-framework` — parent-master SPEC at `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md`

---

## 1. Purpose of this stub

This file is a **sibling SPEC stub** lining up with the 06.2 / 05.2 stub precedent (`docs/envoy-go/phases/06.2-access-log/README.md` was a sibling stub from 06.1's SPEC commit until 06.2's own lifecycle-state-1 entered and its full SPEC was drafted). It exists so that:

1. The phase-07 ROADMAP split (parent `07` → sub-phases `07.1` + `07.2`) has a directory home for `07.2` immediately at the SPEC commit, mirroring the §4.1 artifact-layout invariant.
2. Future sessions reading the brainstorm-close BRAINSTORM.md can navigate directly to a 07.2-scoped placeholder rather than guessing where 07.2 lives.
3. The split-table in BRAINSTORM §1 has an addressable target file to cite.

The full 07.2 SPEC is authored at 07.2's lifecycle-state 1 → 2 transition (its own brainstorm + SPEC session), per `BOOTSTRAP_PROMPT.md` §5. **This stub is read-only history once that SPEC commits**; no edits land here after that point. Future edits target `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` (the full SPEC, drafted later).

---

## 2. Scope (forward-looking, per BRAINSTORM §1's split-table)

The 07.2 sub-phase covers the listener-side filter-chain completion items pre-deferred from phase 03 by ADR-0033 (Phase-03 filter-chain subset). Concretely, the in-scope items are:

1. **`listener_filters` framework.** A from-scratch listener-filter dispatch pipeline that runs BEFORE HCM (and before any other network filter) on each accepted downstream connection. Listener filters can inspect the connection at the wire level (TLS preamble, raw bytes, source/destination metadata) and contribute to chain-match dimensions. The phase-03 listener manager silently skips `listener_filters`; 07.2 honors it. The first concrete listener filter — likely `envoy.filters.listener.tls_inspector` (peeks at the TLS ClientHello to extract SNI before the chain-match step) and possibly `envoy.filters.listener.original_dst` — is part of 07.2's deliverable.
2. **`FilterChainMatch` fields beyond SNI.** Phase 03's ADR-0033 narrowed the filter-chain match to `server_names` (SNI) only. 07.2 expands the match dimensions to:
   - `destination_port` (the listener bind port — usually trivially the listener's own port, but xDS LDS may attach multiple listener-address-port pairs to one listener; the field exists in v3 proto for completeness)
   - `prefix_ranges` (CIDR-keyed match on downstream peer IP)
   - `source_prefix_ranges` (CIDR-keyed match on downstream peer source IP — distinct from `prefix_ranges` which is destination-side)
   - `source_ports` (downstream peer source-port match)
   - `application_protocols` (ALPN-derived chain-match — connects with phase 05's ALPN dispatch via ADR-0050; potentially supersedes ADR-0050's "ALPN inside HCM, not at chain-match" decision)
   - `transport_protocol` (`raw_buffer` vs `tls`; typically derived from a listener filter's contribution)
3. **`Listener.default_filter_chain`.** A fallback chain that runs when no `filter_chain.filter_chain_match` matches. Phase 03's listener manager errors at parse if this field is set; 07.2 honors it.

These three surfaces partially supersede `ADR-0033` (Phase-03 filter-chain subset). The full 07.2 SPEC enumerates which parts of ADR-0033 stay (e.g., the per-chain "exactly one terminal filter" rule remains; that's a network-filter-chain shape constraint, not a chain-match dimension) and which parts are superseded (`server_names`-only chain match → multi-dimensional match; absent `default_filter_chain` → honored).

The fixture(s) for 07.2 are TBD at 07.2's brainstorm time. Likely shape: a listener with multiple `filter_chain` entries differing on `prefix_ranges` or `application_protocols`, plus a `default_filter_chain` fallback; driver issues requests from different source IPs / with different ALPN offers and asserts the right chain is selected. The differential equivalence claim follows the existing differential discipline (per-chain dispatch outcome equivalent across envoy-go and reference Envoy v1.37.2).

---

## 3. Out-of-scope (deferred to later phases)

07.2 does NOT cover:

- Network-filter chain expansion within a single chain (multi-entry network chain with iteration, e.g., `redis_proxy + tcp_proxy` chained). That is a separate Network-filters family deliverable.
- HTTP-filter-chain framework — that's 07.1's scope, landed before 07.2.
- xDS LDS dynamic listener updates — xDS family.
- Listener-level access logging on chain-match-miss — out of scope unless the `default_filter_chain` design requires it.
- HTTP/3 + QUIC listener filters — HTTP/3 + QUIC family.

---

## 4. Dependencies and ordering

07.2 depends on **07.1's phase-done commit** for ROADMAP-ordering reasons only (per BRAINSTORM §1's "07.1 ships first" rationale: 07.1 unblocks the BOOTSTRAP §9 HTTP-filters family; 07.2 has no §9 dependents). Architecturally, 07.2 is independent of 07.1 — its surface lives entirely under `internal/listener/`, sharing no production-code with 07.1's `internal/filter/http/` + `internal/filter/hcm/` work. A future schedule pressure could in principle execute 07.2 before 07.1 by amending the ROADMAP-row depends-on column; this is not anticipated.

---

## 5. References

- **Parent master SPEC:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` (this commit's parent SPEC — the cross-cutting discipline).
- **BRAINSTORM:** `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` §1 (split table — the seed for this stub).
- **Phase-03 ADRs partially superseded by 07.2:** ADR-0033 (Phase-03 filter-chain subset; supersedes ADR-0025) — 07.2 expands the chain-match dimensions and honors `Listener.default_filter_chain`.
- **Phase-05 ADRs that 07.2 may revisit:** ADR-0050 (ALPN-driven codec selection inside `Filter.Handle`, NOT at the listener-side filter-chain match step) — 07.2's `application_protocols` chain-match field is the natural home for ALPN-as-chain-match; the 07.2 SPEC decides whether ADR-0050 stays (codec selection inside HCM) or is partially superseded (ALPN moves to chain-match).
- **Sibling-stub precedents:** `docs/envoy-go/phases/06.2-access-log/README.md` (06.2 was a stub from 06.1's SPEC commit until 06.2's full SPEC drafted) and `docs/envoy-go/phases/05.2-upstream-h2/README.md` (the 05.2 stub from 05.1's SPEC commit, mutatis mutandis).
- **STATE.md:** when 07.2 enters lifecycle-state 1, `STATE.md`'s `active-phase` flips to `07.2-listener-chain-completion` and a fresh BRAINSTORM session is opened against this stub.
