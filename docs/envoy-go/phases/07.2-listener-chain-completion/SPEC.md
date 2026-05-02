# Phase 07.2 — Listener-chain completion (`internal/listener/listenerfilter/` package, `tls_inspector` filter, `FilterChainMatch` beyond SNI, `Listener.default_filter_chain`, fixture `0008`)

**Phase id:** `07.2`
**Slug:** `07.2-listener-chain-completion`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (autonomous mode per ADR-0004; transcribes the parent phase-07 BRAINSTORM.md §1 split-table + the 07.2 sibling-stub README into formal SPEC shape and pins three empirical obligations against reference Envoy v1.37.2 — server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`)
**Depends on:** phase 07.1 (done at master `424485b` via 07.1's phase-done commit; planner-time-ordered per parent BRAINSTORM §1 — **architecturally independent**: 07.2's surface lives entirely under `internal/listener/`, sharing no production-code with 07.1's `internal/filter/http/` + `internal/filter/hcm/` work)
**Parent phase:** `07-filter-chain-framework` (in-progress; **closes at THIS phase's phase-done commit**, mirroring the 05/05.1/05.2 + 06/06.1/06.2 closure pattern recorded in `STATE.md` and the parent SPEC §5)
**Master design document:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` (read-only history at master `ee45aba`) and `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` (brainstorm-close artifact at master `da28039`). The parent BRAINSTORM §1 split-table fixed the per-sub-phase scope; the parent BRAINSTORM defers ALL listener-chain decisions to this sub-phase — those decisions are now settled and are this SPEC's contract. The 07.2 sibling-stub README at `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` (committed at master `ee45aba`) is the forward-looking placeholder this SPEC supersedes.
**Differential surface at end of sub-phase:** ONE new fixture lands. NEW differential fixture `test/differential/0008-listener-chain-match/` is differentially green (gate (a) **non-vacuous**): two listeners (`l_test_a` and `l_test_b`) each bound on a distinct OS-picked `127.0.0.1:0` plaintext port and carrying the same chain set — three `filter_chains[]` entries differing on `destination_port` / `source_prefix_ranges` plus a `default_filter_chain` fallback (per §7.4 the dual-listener construction is required to demonstrate the `destination_port` precedence dimension across distinct connections); driver issues five sequential TCP connections across both listeners — one per chain, plus one whose source matches no chain (default_filter_chain wins, exercised via a connection-4-only configuration variant that omits the catch-all `chain_other`), plus one that satisfies two chains' match dimensions simultaneously (chain-precedence empirically pinned per §11.3) — and asserts the per-connection backend-port distribution exactly matches a synthetic `[1,1,1,1,1]` mod-5 partition split across five distinct backend ports (one per chain), evidencing chain-selection equivalence across envoy-go and reference Envoy v1.37.2. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe` remain green (gate (b)). h2spec conformance gate (c) re-runs at the ADR-0051 pin and stays at 53/53 PASS (07.2's surface is pre-HCM, doesn't touch H2 wire code). Fuzz (d) re-runs the existing 9 fuzzers AND adds a new `FuzzFilterChainMatch` over the chain-match selection algorithm at the 30s ADR-0018 budget. Build/vet/lint/test (e) and REVIEW (f) apply normally. ROADMAP row `07.2` flips `planned → in-progress` at the SPEC commit (per the 07.1 REVIEW I-3 corrected pattern — do NOT leave it at `planned` through the impl session); rows `07.2` AND parent `07` flip `→ done` AT THE SAME phase-done commit per the parent SPEC §5 / 06.2 closure precedent.

---

## 1. Purpose

Phase 07.2 lands envoy-go's listener-side filter-chain completion — the listener-filter dispatch pipeline that runs BEFORE HCM (and before any other network filter) on every accepted downstream connection, the full `FilterChainMatch` algorithm with seven match dimensions beyond SNI, and the `Listener.default_filter_chain` fallback semantics. This is the second half of the parent ROADMAP row 07 (filter-chain framework) and the sub-phase that closes that parent row. Concretely:

1. **A new `internal/listener/listenerfilter/` package** — `ListenerFilter` interface, `ListenerFilterStatus` enum (`Continue`, `StopIteration`), `ListenerFilterCallbacks` (a narrow callback set: a peek-buffer reader plus the chain-match-input mutators that listener filters contribute to), `ListenerFilterRegistry` (Register / Lookup / Freeze; mirrors 07.1's `*HTTPRegistry` LBP-1 discipline per ADR-0072), `Pipeline` per-connection state machine (sequential dispatch; one filter at a time; current filter must finish before the next one starts), and `ChainMatchInputs` (a typed struct holding the seven match dimensions populated incrementally by listener filters and the connection-level facts already known at accept time). The package is the listener-side analog of 07.1's `internal/filter/http/` package; it is similarly small (~400-600 LoC of new machinery) and similarly anchored on a freeze-after-boot threaded registry. Two-step factory pattern (config-time + per-connection) mirrors 07.1's pattern from ADR-0072.

2. **One new concrete listener filter under `internal/listener/listenerfilter/`:**
   - **`tls_inspector/`** — real Envoy listener filter (`envoy.filters.listener.tls_inspector`); peeks the first ~512 bytes of the connection (without consuming them), parses the TLS ClientHello, extracts SNI (`server_name`) and ALPN offers (`application_layer_protocol_negotiation`), and contributes both to the `ChainMatchInputs.ServerName` and `ChainMatchInputs.ApplicationProtocols` fields. ~250 LoC + ~300 LoC tests. The filter runs only on TLS connections (it inspects the ClientHello byte preamble; on non-TLS connections it returns `Continue` with no chain-match-input contribution, leaving the chain-match algorithm to operate on the un-inspected connection-level facts). The peek-buffer is read via a dedicated `peeker` wrapper around the raw `net.Conn` that buffers reads internally so that the bytes consumed by the inspector are NOT consumed from the perspective of the downstream filter chain.

3. **One concrete listener filter explicitly DEFERRED:**
   - **`original_dst`** — the second canonical Envoy listener filter (`envoy.filters.listener.original_dst`; reads `SO_ORIGINAL_DST` socket option to recover the pre-iptables-redirect destination address, contributes to `ChainMatchInputs.DestinationPort` + the destination IP). Per Decision F (§5 below), `original_dst` is **deferred to a later phase** because (a) the MVP listener-filter dispatch pipeline is fully exercised by `tls_inspector` alone — the filter's contribution to `ChainMatchInputs.ServerName` + `.ApplicationProtocols` exercises the same dispatch surface `original_dst`'s contribution to `.DestinationPort` would; (b) `original_dst`'s deployment niche (transparent proxying behind iptables `REDIRECT` rules) is a niche use case not on the BOOTSTRAP MVP trunk; (c) the dispatch-pipeline contract is filter-agnostic — adding `original_dst` later is purely additive (a new package + a new Register call at boot) and does not require revisiting the 07.2 ADR set. Decision F records this with the rationale + the future-phase pointer.

4. **Full `FilterChainMatch` algorithm.** Phase 03's ADR-0033 narrowed `filter_chain_match` to `server_names` only (errors at parse on any other dimension). 07.2 expands the match dimensions to the seven Envoy v1.37.2-documented fields:

   - `destination_port` (int, optional) — the listener bind port; trivially the listener's own port in the MVP since envoy-go does not yet support multiple-listener-address-port pairs per listener (deferred to xDS LDS family). Carried through the algorithm for future-proofing — see §7.
   - `prefix_ranges` (CIDR list) — match on the **destination** IP of the accepted connection (i.e., `conn.LocalAddr().IP`).
   - `source_type` (enum: ANY / LOCAL / EXTERNAL) — `ANY` matches anything (the default in v3 proto); `LOCAL` matches if peer is loopback; `EXTERNAL` matches non-loopback.
   - `source_prefix_ranges` (CIDR list) — match on the **source** IP of the accepted connection (`conn.RemoteAddr().IP`).
   - `source_ports` (uint32 list) — match on the source port of the accepted connection.
   - `server_names` (string list) — SNI match (already honored by phase 03 / ADR-0033; semantics extended only by integration with the new precedence algorithm).
   - `transport_protocol` (string: `raw_buffer` / `tls` / empty) — match on the wire protocol family. `tls` matches when `tls_inspector` confirmed a ClientHello byte preamble; `raw_buffer` matches when no TLS preamble was detected (the empirical-pin evidence pinned in §11 below confirms this dichotomy on the ENVOY_TARGET-pinned image).
   - `application_protocols` (string list) — ALPN match. Populated by `tls_inspector` when a ClientHello carries an ALPN offer list; matches one of the listed protocol strings (e.g., `["h2", "http/1.1"]`).

   The eighth field `direct_source_prefix_ranges` (proxy-protocol original-source-IP) is **silently ignored** (not in scope; deferred to a future xDS / proxy-protocol family phase). See §7.

5. **Chain-match precedence algorithm.** Per Envoy's documented precedence (cited in §7.2 below from the `filter_chain_match.proto` upstream comments) and confirmed by §11.3 below's empirical pin against reference Envoy v1.37.2: chains are scored against an accepted connection in **most-specific-wins** order across the seven dimensions ranked in this priority list (highest priority first):

   1. `destination_port`
   2. `prefix_ranges` (destination IP / CIDR)
   3. `server_names`
   4. `transport_protocol`
   5. `application_protocols`
   6. `source_type`
   7. `source_prefix_ranges` (source IP / CIDR)
   8. `source_ports`

   For each connection, every chain is scored: a chain that does NOT specify a dimension at all is treated as "match anything for that dimension" (rank 0 — most generic). A chain that DOES specify a dimension only matches if the dimension's value matches; otherwise the chain is eliminated. Among non-eliminated chains, the chain with the **highest-priority specific dimension** wins. Ties on the top-priority specific dimension are broken by walking down the priority list. Final ties (chains identical on all eight dimensions) are configuration errors at `NewManager`-build time (per Decision G + §8 below). Empirical pin §11.3 validates this on a 2-chain probe across `destination_port` vs `source_prefix_ranges`.

6. **`Listener.default_filter_chain` fallback semantics.** Phase 03's listener manager errored at parse if `default_filter_chain` was set; 07.2 honors it. Per §11.1 + §11.2 empirical pins (resolved IN-SESSION against reference Envoy v1.37.2 server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`):

   - `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry's `filter_chain_match` matches. The connection is dispatched into the default chain's filter and proceeds normally.
   - **An empty-match `filter_chain` (no `filter_chain_match` set, or set to the zero proto) BEATS `default_filter_chain`** when both coexist. Pin §11.2 confirms this: with both an empty-match chain and a `default_filter_chain` on the same listener, every connection is dispatched into the empty-match chain; the `default_filter_chain` never fires. This is settled by Decision E + ADR-0080 (anticipated).
   - At most one `default_filter_chain` per listener (it's a single proto field, not a list — schematically guaranteed).
   - Phase 03's "at most one catch-all chain per listener" rule (the `catchAllCount > 1` check in `internal/listener/manager.go:308`) **stays** for `filter_chains[]` empty-match entries. The `default_filter_chain` is a SEPARATE structural slot and is not subject to that rule. Decision E records this enumeration of which ADR-0033 clauses stay vs are superseded.

7. **`listener_filters[]` dispatch contract.** A listener filter implements `ListenerFilter.Inspect(ctx, peeker, inputs)` returning `Continue` or `StopIteration` plus an optional error. The pipeline dispatches sequentially (filter[0] runs to completion before filter[1] starts; no async-resume in the MVP — see Decision A); each filter that returns `Continue` allows the pipeline to advance; `StopIteration` halts the pipeline (the filter must have already populated whatever inputs it intended; the chain-match algorithm runs on what's been populated so far). A 15-second **per-pipeline** inspection deadline (shared across all filters; a slow first filter eats subsequent filters' budget) applies per Decision B + Decision N + §5.4 + §6.5 below. Multi-filter pipelines are supported — Decision D — though the MVP only registers `tls_inspector`. The pipeline interacts with the listener manager's existing post-handshake dispatch path: the `tls_inspector` filter contributes SNI + ALPN to `ChainMatchInputs` BEFORE the TLS handshake completes (it operates on the ClientHello peek buffer); the chain-match algorithm fires AFTER the listener-filter pipeline finishes; the selected chain's TLS config (if any) is then used for the handshake; post-handshake dispatch falls through to the chain's terminal filter unchanged. See §5 for the integration sequence.

8. **Empirically pinned obligations.** Three empirical scrapes were executed IN-SESSION against reference Envoy v1.37.2 (`envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per the `/server_info` `version` field) on 2026-05-02 by this SPEC-drafting session. Verbatim Envoy server output is pinned in §11 below: (a) `default_filter_chain` honored as no-match fallback; (b) empty-match chain BEATS `default_filter_chain` when both coexist; (c) `destination_port` BEATS `source_prefix_ranges` in chain-match precedence when both could match. **All three resolved IN-SESSION** — no carry-forward. A fourth empirical-pin obligation (`tls_inspector`-populated ALPN feeding `application_protocols` chain-match) is documented as **carry-forward to impl time** per §11.4 + Decision K because it requires booting reference Envoy with a TLS-aware probe driver, not feasible without committing real probe-driver code (which the ADR-0004 hard-gate forbids at SPEC time).

9. **Anticipated ADRs:** seven ADRs per §10 below, numbered ADR-0077..ADR-0083 (next-free per the `DECISIONS.md` tail at master `424485b` being ADR-0076; the planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule).

10. **Partial supersession of ADR-0033.** §5.7 below enumerates clause-by-clause which parts of ADR-0033 (Phase-03 filter-chain subset) STAY in 07.2 vs are SUPERSEDED. **Canonical net-effect summary (§5.7 is the single source of truth):** clauses 1, 4, 7 are **fully preserved**; clauses 5, 6, 9 are **preserved with caveats** (clause 5: `default_filter_chain` may have an independent TLS posture; clause 6: plaintext multi-chain is now allowed except when any chain populates `server_names`; clause 9: SNI-internal sub-ordering becomes the tie-breaker WITHIN the new 8-dimension priority list's `server_names` slot, with no-match falling through to `default_filter_chain` if set); clauses 2, 3, 8 are **superseded** (clause 2: the `filter_chain_match` whitelist is partially superseded — only `direct_source_prefix_ranges` stays silent-skipped post-07.2, all other dimensions are honored; clause 3: the parse-time error on `default_filter_chain` is totally superseded — 07.2 honors the field; clause 8: silent-skip on `listener_filters[]` is totally superseded — 07.2 honors the field).

11. **`BEHAVIOR_CONTRACT.md` extensions** at the 07.2 phase-done commit per ADR-0052: a new top-level `## Listener filters` section is introduced (carrying the §11 empirical-pin blocks verbatim), and the existing `## TCP proxy` section's "Does not yet apply to" enumeration is amended to reflect the 07.2 promotions. `## TLS` section's "Scope boundaries" enumeration is similarly amended. The actual edit lands at impl time per the same ADR-0052 in-place-edit timing 06.1 / 06.2 / 07.1 used.

After phase 07.2, the project has proven the second half of its eighth central engineering claim: *envoy-go runs a real listener-filter dispatch pipeline before HCM (or any other network filter) on each accepted connection — supporting filters that peek the ClientHello to extract SNI + ALPN — and matches a downstream filter chain against any FilterChainMatch dimension Envoy v1.37.2 documents (port, IP CIDR, source-IP CIDR, source port, source-type, SNI, transport protocol, ALPN), with `default_filter_chain` as the documented no-match fallback — making the proxy a programmable middlebox aligned with Envoy's documented chain-match extensibility model.* The parent ROADMAP row `07` flips to `done` at THIS phase's phase-done commit per the parent SPEC §5 closure pattern.

## 2. Non-purposes

Phase 07.2 does **not** do any of the following. Most are explicit non-goals from the parent BRAINSTORM §9 / parent SPEC §3 / 07.2 sibling-stub README §3, or scope-narrowings the SPEC introduces by consolidating BRAINSTORM-time deferrals. Each non-purpose is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Listener-filter set non-goals

- **`envoy.filters.listener.original_dst`** — deferred per Decision F (§5 below). Rationale: the MVP dispatch pipeline is fully exercised by `tls_inspector` alone; `original_dst`'s deployment niche (iptables `REDIRECT`-based transparent proxying) is not on the BOOTSTRAP MVP trunk; adding the filter later is purely additive. → Network-filters family OR a dedicated transparent-proxy phase.
- **`envoy.filters.listener.proxy_protocol`** — proxy-protocol-v1 / v2 ingestion (PROXY-line-prefix protocol that recovers downstream-source-IP behind a Layer-4 LB). Deferred. → Future phase; potentially bundled with the `direct_source_prefix_ranges` chain-match dimension.
- **`envoy.filters.listener.http_inspector`** — sister of `tls_inspector` for plaintext HTTP/1 vs HTTP/2 detection (peeks bytes for `PRI * HTTP/2.0\r\n\r\n` H2 preface). Deferred — phase 05.1 ALPN-driven dispatch (ADR-0050) covers TLS H1-vs-H2; plaintext H1-vs-H2 is the `http_inspector`'s niche and 07.2 doesn't ship it. → Future phase.

### 2.2 Listener-filter dispatch protocol non-goals

- **Async-resume.** 07.1's HTTP-filter chain has async-resume (a filter returns `StopIteration` and later resumes via a callback). The 07.2 listener-filter pipeline is **synchronous-only** (Decision A): a filter's `Inspect` call returns synchronously; on `Continue` the pipeline advances; on `StopIteration` the pipeline halts. No `ContinueInspecting()` callback. Rationale: the listener-filter surface is much narrower than the HTTP-filter surface (the inputs are a peek-buffer + a known-shape connection-level facts struct; the output is a chain-match-inputs mutation); no realistic listener filter needs async-resume. Adding it later is straightforward (mirror 07.1's channel-based mechanic) but unjustified at MVP. → If a future listener filter (e.g., a network-side `ext_authz_listener`) needs async-resume, that filter's phase introduces it.
- **Per-listener-filter timeouts (configurable).** The proto field `listener_filters_timeout` (`google.protobuf.Duration`) on `Listener` is **honored** at parse time but **clamped to the global default of 15s** (Decision B). A user-set value below 1s OR above 60s errors at parse; values in [1s, 60s] are honored as-is. The global default is 15s if the field is unset. The `continue_on_listener_filters_timeout` field (which determines whether timeout is fatal vs. ignored) is **honored as-is** at the proto's declared semantics: `false` (default) → timeout aborts the connection; `true` → timeout treats the listener-filter pipeline as having returned `Continue` and proceeds. → Adjustments to the [1s, 60s] envelope land in a future hardening phase.
- **`listener_filters` extension config (typed_config beyond the file-name + typed-proto pair).** The proto field `listener_filters[].filter_disabled` (a CEL-expression-driven enable/disable per request) is **silently ignored** at parse time (07.2 silent-ignore set extension; see §9). Listener filters always run when present in the chain. → CEL family or a future xDS-listener-config phase.
- **Listener-filter chains beyond a single MVP filter.** 07.2's MVP registers ONE listener filter (`tls_inspector`); `original_dst` and `http_inspector` are both deferred. The pipeline machinery supports **multiple listener filters** (Decision D) — sequential dispatch in declaration order, each filter's `Inspect` running before the next's, all contributing to the same `ChainMatchInputs` struct — but the test surface only exercises 1-filter pipelines. Multi-filter pipelines are unit-tested in `pipeline_test.go` with two synthetic test-only filters (a `noop` + a `setSNI` probe; see §15.1). → Multi-filter coverage in a real workload arrives when a second concrete listener filter ships.

### 2.3 `FilterChainMatch` dimension non-goals

- **`direct_source_prefix_ranges`** — proxy-protocol-recovered downstream-source-IP CIDR match. **Silently ignored** at parse time (07.2 silent-ignore set extension; see §9). 07.2 cannot honor it because the proxy-protocol filter is out of scope (§2.1); adding the dimension without the filter would be a no-op that confuses future readers. → Bundled with the proxy-protocol filter phase.
- **`xds.type.matcher.v3.Matcher`-based filter-chain selection** (the unified-matcher API that supersedes `FilterChainMatch` in upstream Envoy's longer-term roadmap). Deferred — the unified-matcher API is a much larger surface that 07.2's contained scope cannot accommodate. → xDS family.
- **Filter-chain hot-update via xDS LDS.** 07.2 builds chains at `NewManager` time and never mutates them. xDS LDS-driven dynamic chain insertion / removal / order changes are out of scope. → xDS family.

### 2.4 Listener-level non-goals

- **`Listener.access_log[]` (listener-level access logging on chain-match-miss)** — silently ignored at parse time. The 06.2 access-log surface is HCM-internal; listener-scope access logging (e.g., a no-match drop record) is a separate observability surface. → Future observability-extension phase.
- **`Listener.connection_balance_config`** — per-listener connection balancing across worker threads. envoy-go runs single-goroutine accept-then-dispatch (no per-worker connection balancing); the field is silently ignored at parse time. → Concurrency-tuning family or a future scaling phase.
- **`Listener.bind_to_port` set to `false`** — non-bound listeners (used by `original_dst` deployments behind a single bound listener that fans out via socket-marking). Silently ignored at parse time; defaulted to `true`. → bundled with `original_dst`.
- **`Listener.reuse_port`** — `SO_REUSEPORT`-based load distribution across worker goroutines. envoy-go has a single accept goroutine per listener (no `SO_REUSEPORT` need); silently ignored. → Concurrency-tuning family.
- **`Listener.transparent`** — sets `IP_TRANSPARENT` socket option for transparent proxying. Silently ignored. → Bundled with `original_dst`.
- **`Listener.freebind`** — sets `IP_FREEBIND` socket option. Silently ignored. → Bundled with `original_dst`.

### 2.5 ALPN-disposition non-goals

ADR-0050 (phase 05.1) decided that ALPN-driven codec selection happens **inside `Filter.Handle`** (HCM type-asserts on `*tls.Conn` and reads `NegotiatedProtocol`), NOT at the listener-side filter-chain match step. 07.2 introduces the `application_protocols` chain-match dimension which IS a listener-side filter-chain match consultation of ALPN — so the question arises: does 07.2 SUPERSEDE ADR-0050?

**Decision (per Decision H + ADR-0083 anticipated): NO total supersession; both coexist.** ADR-0050's HCM-internal ALPN dispatch is the **codec-selection** mechanism (which Go-level codec implementation runs the request — phase-04 `runConnection` for H1, phase-05.1 `runH2` for H2). 07.2's `application_protocols` chain-match field is the **chain-selection** mechanism (which `filter_chain` entry runs at all — selecting between HCM-with-codec-AUTO and a hypothetical-future TCP-proxy-with-no-HCM-on-the-same-listener). The two are orthogonal and complementary:

- A user can deploy a single listener with `codec_type: AUTO` HCM as the only filter chain → ADR-0050 mechanic fires; 07.2's `application_protocols` is empty or matches everything; behavior is unchanged from 05.1.
- A user can deploy a listener with two chains, one matched on `application_protocols: [h2]` (terminal HCM with `codec_type: HTTP2`) + one on `application_protocols: [http/1.1]` (terminal HCM with `codec_type: HTTP1`) → 07.2's chain-match selects between them; ADR-0050's HCM-internal dispatch is a no-op because each chain's HCM has a forced `codec_type` (the `AUTO` branch never fires).

This non-supersession decision is recorded in ADR-0083 (anticipated; §10 below). The empirical-pin obligation that confirms the dispatch interaction (§11.4 carry-forward, Decision K) WOULD verify this with a real probe but is deferred to impl time as documented.

### 2.6 Process / lifecycle non-goals

- **Listener-filter pipeline metrics.** No listener-filter-scoped counters/gauges (`listener_filter.X.invocations`, `.failures`, etc.) at MVP. The 06.1 stats discipline supports adding them later (3-LoC per-call site change); 07.2's hot path is on the connection-accept side and adding metrics here without empirical reason would inflate the listener-scope metric set unnecessarily. → Future observability-extension phase.
- **xDS LDS dynamic listener filter updates.** Filter set is fixed at `NewManager` time; no live filter add/remove. → xDS family.

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 07.2)

Per doctrine `D-3.6`, phase 07.2 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 07.2 |
|---|---|
| (a) new/changed differential fixtures green | **Non-vacuous (single fixture).** New fixture `test/differential/0008-listener-chain-match/` passes: **two listeners** `l_test_a` and `l_test_b` each bound on a distinct OS-picked `127.0.0.1:0` plaintext port, both carrying the **same** chain set — three `filter_chains[]` entries (`chain_dstport_alpha` matching `destination_port: <l_test_a_port>`, `chain_srcprefix_loopback` matching `source_prefix_ranges: 127.0.0.1/32` AND `source_ports: [<known driver source port>]`, `chain_other` with empty `filter_chain_match` [catch-all]) plus a `default_filter_chain` fallback (`chain_default`). The dual-listener construction is required to demonstrate the `destination_port` precedence dimension across distinct connections (per §7.4 — a single bound port cannot distinguish chains by port). Driver issues five sequential TCP connections (some to `l_test_a`, some to `l_test_b` — see §7.4 for the per-connection routing) and asserts each connection's response body comes from the expected backend (one backend per chain — five distinct backend ports per side; the per-connection response body is the backend's listener address echoed back as a string, byte-equal between subject and reference). The five connections cover: (i) connection to `l_test_a` from a non-loopback source IP → hits `chain_dstport_alpha` (highest-precedence dimension); (ii) connection to `l_test_b` from `127.0.0.1` with the known driver source port → hits `chain_srcprefix_loopback`; (iii) connection to `l_test_b` from a non-loopback source → hits `chain_other` (catch-all, empty-match); (iv) connection to `l_test_b` from a non-loopback source where `chain_other` is removed from the candidate set (variant configured for the connection-4 sub-fixture) → hits `chain_default`; (v) connection to `l_test_a` from `127.0.0.1` with the known driver source port — both `chain_dstport_alpha` and `chain_srcprefix_loopback` match, precedence (§11.3 empirical pin) selects `chain_dstport_alpha`. Per-connection backend-port assertion is byte-equal across envoy-go and reference Envoy. |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe` all pass without regression. The 07.2 changes are additive: `internal/listener/manager.go`'s `validateFilterChainMatch` (currently rejecting most dimensions per ADR-0033) is amended to ACCEPT them and feed them into the new chain-match algorithm; `default_filter_chain` is honored instead of erroring; `listener_filters[]` is dispatched instead of silently skipped. Pre-existing fixtures' bootstraps don't set any of these new fields so the dispatch code paths aren't hit; the existing chain-match-by-SNI behavior under ADR-0033 is preserved as a special case of the new 8-dimension algorithm (a chain with only `server_names` set, on a TLS listener, with no `default_filter_chain` and no `listener_filters[]`, behaves identically). |
| (c) conformance suites pass | `test/conformance/h2spec/` re-runs at the ADR-0051 pin (`summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md`) and reports `failed == 0` over the unchanged threshold list (sections 3, 4, 5, 6 ex-6.6, 7, 8 — 53/53 PASS at the 05.1+05.2+06.1+07.1 baseline). 07.2 doesn't touch H2 wire code or HCM dispatch wiring; the change is pre-HCM only. Pin is NOT bumped (D-3.7 reserves pin bumps for dedicated phases). |
| (d) new/existing fuzzers run clean for CI short-budget | Existing 9 fuzzers (`internal/bootstrap.FuzzBootstrapLoad`, `internal/filter/tcpproxy.FuzzTcpProxyFilter`, `internal/tls.FuzzTLSContextParse`, `internal/filter/hcm.FuzzHCMConfigParse`, `internal/filter/hcm/h2.FuzzFrameStream`, `internal/filter/hcm/h2.FuzzHPACKDecode`, `internal/stats.FuzzPromTextFormat`, `internal/accesslog.FuzzAccessLogFormat`, `internal/filter/http.FuzzFilterChainParse`) run clean at the 30s ADR-0018 budget. **NEW:** `internal/listener.FuzzFilterChainMatch` runs clean at the same budget (fuzzes adversarial chain-match input combinations + adversarial chains-list configurations into the chain-match algorithm). Total: 10 fuzzers post-07.2. |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for the new `internal/listener/listenerfilter/` package (registry / pipeline / chain-match / tls_inspector; race-clean; freeze-after-boot enforcement); extended tests for `internal/listener/manager.go` (build-time `validateFilterChainMatch` accepting the new dimensions; chain-precedence sorting; `default_filter_chain` integration); the chain-match algorithm asserted on a synthetic 8-chain matrix. `go test -race ./...` clean — concurrent listener-filter `Inspect` calls (per-connection isolated, but accept-loop multi-conn parallelism); concurrent `ListenerFilterRegistry.Lookup` calls from N listener-manager constructors at boot; a `pipeline_test.go` race that shows two pipelines on different connections operating on independent `ChainMatchInputs` structs; the `peeker` wrapper's bytes-not-consumed invariant tested under concurrent inspection + downstream-Read scenarios. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

The phase-done commit subject (per `BOOTSTRAP_PROMPT.md` §5.3) is: `phase 07.2: phase-done — listener-chain-completion lands; ROADMAP rows 07.2 + 07 → done [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]`. The body explicitly names both ROADMAP-row transitions per parent SPEC §5.

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 07.2. The complete file inventory itemizes every signature change so PLAN time has no surprise scope (the same discipline 06.1 / 06.2 / 07.1 honored).

### 4.1 New production code (in 07.2)

- **`internal/listener/listenerfilter/doc.go`** — package overview: framework architecture, the `ListenerFilter` interface, the registry contract, the per-connection `Pipeline` lifecycle, the freeze-after-boot invariant, the peek-buffer discipline.
- **`internal/listener/listenerfilter/types.go`** — `ListenerFilter` interface (single method `Inspect(ctx, peeker, inputs) (ListenerFilterStatus, error)`), `ListenerFilterStatus` enum (`Continue`, `StopIteration`), `ChainMatchInputs` struct (the eight match dimensions: `DestinationIP net.IP`, `DestinationPort uint32`, `SourceIP net.IP`, `SourcePort uint32`, `ServerName string`, `TransportProtocol string` (`raw_buffer` / `tls`), `ApplicationProtocols []string`, plus a method `IsLoopbackSource() bool` for the `source_type: LOCAL` rule), `Peeker` interface (`Peek(n int) ([]byte, error)`), `ListenerFilterFactory` + `FilterInstanceFactory` two-step factory pattern, `FactoryCtx` (carries the registry pointer + parsed proto-helpers needed by per-filter parsers).
- **`internal/listener/listenerfilter/callbacks.go`** — concrete `peeker` implementation backed by a `bufio.Reader` wrapping the raw `net.Conn`; reads up to N bytes without consuming them from the perspective of the downstream filter chain. The wrapper presents itself as a `net.Conn` to the post-listener-filter dispatch code (i.e., `chainInfo.filter.Handle(ctx, peekedConn)` works unchanged).
- **`internal/listener/listenerfilter/registry.go`** — `ListenerFilterRegistry struct { mu sync.RWMutex; byTypeURL map[string]ListenerFilterFactory; frozen atomic.Bool }`. `NewListenerFilterRegistry()`, `Register(typeURL string, f ListenerFilterFactory)` (panics if frozen, panics on duplicate type_url), `Lookup(typeURL string) (ListenerFilterFactory, bool)`, `Freeze()` (idempotent). Mirrors 07.1's `*HTTPRegistry` LBP-1 (per ADR-0072) and 06.1's `*stats.Registry`.
- **`internal/listener/listenerfilter/pipeline.go`** — `Pipeline` per-connection sequential dispatch. Allocated by the listener manager's accept-loop on each accepted connection. Owns: filter instances (allocated via per-connection factories from the chainConfig), the `ChainMatchInputs` struct (initialized from `conn.LocalAddr()` + `conn.RemoteAddr()` at allocation), the `peeker` wrapper around the raw conn. Method: `Run(ctx context.Context, deadline time.Duration) (ChainMatchInputs, error)`. The pipeline iterates `filters[]` sequentially, calling each filter's `Inspect`; on `Continue` advances; on `StopIteration` halts; **a single per-pipeline `context.WithTimeout(ctx, deadline)` is established once before the loop and shared across all filters' `Inspect` calls** (per-pipeline shared budget per §5.4 + §6.5 + Decision N — NOT per-filter time-slicing); if any `Inspect` returns with `ctx.Err() != nil`, the pipeline returns a timeout error; the listener manager handles the timeout per the proto's `continue_on_listener_filters_timeout` field.
- **`internal/listener/listenerfilter/chainmatch.go`** — the chain-match precedence algorithm. Method: `SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)`. `ChainSpec` holds the parsed match dimensions for one chain. The algorithm: for each chain, compute (a) does it match the inputs (check each non-zero dimension; chains with all-zero match are ALWAYS eligible — they're "empty match" chains AKA catch-all); (b) the dimension-priority "specificity" score (the highest-priority specific dimension's index; lower index = more specific). Among eligible chains, the one with the highest specificity score (lowest priority-index of any specific dimension) wins; ties broken by the next-priority dimension; final ties (chains identical on all eight dimensions) are detected at `NewManager`-build time and error there. If no chain is eligible, fall through to `defaultChain`. If no `defaultChain`, return `ErrNoChainMatched`.
- **`internal/listener/listenerfilter/registry_test.go`**, **`pipeline_test.go`**, **`chainmatch_test.go`**, **`callbacks_test.go`** — unit tests per §15.1 below.
- **`internal/listener/listenerfilter/fuzz_test.go`** — `FuzzFilterChainMatch` per §15.6 below. Fuzzes adversarial `ChainMatchInputs` combinations + adversarial chain-spec lists into `SelectChain`; asserts: (i) the function never panics; (ii) returned chain is one of the input chains or `defaultChain` or nil; (iii) returned chain's match dimensions are all satisfied by the inputs; (iv) on identical-priority ties, the algorithm is deterministic (same inputs → same chain).

- **`internal/listener/listenerfilter/tls_inspector/tls_inspector.go`** — real Envoy listener filter `envoy.filters.listener.tls_inspector` (~250 LoC). `Inspect`: peek up to 4096 bytes from the connection; check whether the byte preamble is a TLS ClientHello (TLS record header `0x16 0x03 ...` followed by a HandshakeType.ClientHello byte); if YES, parse the ClientHello extension list to extract `server_name` + `application_layer_protocol_negotiation`; populate `inputs.ServerName` + `inputs.ApplicationProtocols`; set `inputs.TransportProtocol = "tls"`; return `Continue`. If NO (not a ClientHello — plaintext-data preamble or non-TLS protocol), set `inputs.TransportProtocol = "raw_buffer"`; return `Continue` (the chain-match algorithm handles the no-SNI case by passing-through). Hand-rolled minimal ClientHello parser — does NOT pull in the upstream Envoy C++ implementation. Per `crypto/tls` source (`crypto/tls/handshake_messages.go`), the ClientHello parsing is ~120 LoC of bit-shuffling; envoy-go does the same in-tree.
- **`internal/listener/listenerfilter/tls_inspector/tls_inspector_test.go`** — unit tests: ClientHello with SNI + ALPN populates inputs correctly; ClientHello without SNI populates `TransportProtocol="tls"` only; ClientHello with garbled extensions doesn't panic, returns `Continue` with whatever extracted so far (graceful-degradation discipline); non-TLS preamble (e.g., `GET / HTTP/1.1\r\n`) returns `Continue` with `TransportProtocol="raw_buffer"`; partial ClientHello (only first 50 bytes available) returns `Continue` with whatever extracted; the type_url + factory (`tls_inspector.New`) round-trip through the registry; concurrent inspection on independent connections is race-clean.
- **`internal/listener/listenerfilter/tls_inspector/parser.go`** — the hand-rolled minimal ClientHello parser (`parseClientHello(buf []byte) (sni string, alpns []string, ok bool)`). Pure function; no I/O. Adapted from `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello type, narrowed to extract only `server_name` + `application_layer_protocol_negotiation`. ~120 LoC.
- **`internal/listener/listenerfilter/tls_inspector/parser_test.go`** — table-driven tests on hand-crafted ClientHello byte sequences (one with both extensions; one with only SNI; one with only ALPN; one with no extensions; one truncated; one malformed length prefix).
- **`internal/listener/listenerfilter/tls_inspector/proto.go`** — proto-config marshaling for the upstream `envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector` proto. The proto carries `enable_ja3_fingerprinting` (silently-ignored at parse — JA3 not in scope) and `initial_read_buffer_size` (clamped to 4096 if set higher; defaulted to 4096 if unset; values < 256 errored at parse).

### 4.2 Changed production code (in 07.2)

- **`internal/listener/manager.go`** — substantially refactored. The `validateFilterChainMatch` function (currently at line 378, rejecting `destination_port` / `prefix_ranges` / `source_prefix_ranges` / `source_type` / `source_ports` / `application_protocols` and accepting only `server_names` + `transport_protocol == "tls"`) is rewritten to **parse and accept** all those dimensions, building a `*ChainSpec` per chain. The `chainSpecificityRank` function (currently at line 352 — the SNI-only specificity ranker: 0=exact, 1=suffix-wildcard, 2=universal-wildcard, 3=catch-all) is **kept as a sub-routine** for tie-breaking among chains that match on `server_names` (the empirical pin §11.3 confirms `server_names` is one of the eight priority dimensions; within `server_names` the existing specificity rank applies). The chain-sort step at line 327 (`sort.SliceStable(cis, ...)`) is replaced by the new `chainmatch.SelectChain` algorithm — which does NOT pre-sort chains; it scores each chain against the per-connection inputs at dispatch time. Chains are stored in declaration order; SelectChain operates on the unsorted slice. The `default_filter_chain` field is now parsed (previously erroring at line 251); a chain-with-empty-match is constructed for it and stored separately on `listenerRuntime.defaultChain` (new field). The `listener_filters[]` field is now parsed (previously silently-skipped); each entry is resolved via `*ListenerFilterRegistry.Lookup`, instances are constructed, and stored on `listenerRuntime.listenerFilters[]` (new field). The post-handshake `dispatch` function (currently at line 434, re-running SNI match) is **replaced** by a unified pre/post-handshake dispatch path: (1) accept connection; (2) construct `Pipeline` with the listener's listener-filter set; (3) run pipeline (gathers SNI + ALPN if `tls_inspector` is in chain) — pipeline output is `ChainMatchInputs`; (4) call `chainmatch.SelectChain(inputs, listenerRuntime.chains, listenerRuntime.defaultChain)` to pick the chain; (5) if chain has TLS, run TLS handshake (the `GetConfigForClient` callback is no longer needed for chain selection, but is still used to return the per-chain TLS config — refactored to a simpler form that just returns the already-selected chain's config); (6) dispatch to the chain's terminal filter. The `makeGetConfigForClient` callback (currently at line 413) is simplified — it no longer re-runs chain match; it returns the pre-selected chain's TLS config from a per-conn-cached selection. **Estimated diff: ~250 LoC added, ~50 LoC removed; net ~+200 LoC in `internal/listener/manager.go`.**
- **`internal/listener/manager.go` constructor signature** — `NewManagerWithBaseDirAndAllowH2C` gains a new parameter `lfRegistry *listenerfilter.ListenerFilterRegistry` (the boot-populated, frozen registry threaded from `cmd/envoy-go/main.go`). Existing constructors `NewManager` + `NewManagerWithBaseDir` delegate with the new parameter unchanged. Tests update mechanically to thread an empty (or `tls_inspector`-only) frozen registry.
- **`cmd/envoy-go/main.go`** — at boot: `lfReg := listenerfilter.NewListenerFilterRegistry()`; `lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)`; `lfReg.Freeze()`; threads `lfReg` into `listenerManager.New(...)`.
- **`internal/bootstrap/bootstrap.go`** — adds blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"` (carries the `TlsInspector` proto message) so `protojson` can round-trip 07.2 fixture bootstraps. Per ADR-0016's amendment policy, this addition is documented in PROGRESS, not a new ADR.

### 4.3 New harness and fixture code (in 07.2)

- **`test/differential/0008-listener-chain-match/`** — new differential fixture directory. Contents:
  - **`envoy-go.yaml`** — subject bootstrap. **Two listeners** `l_test_a` + `l_test_b` each binding `127.0.0.1:0` plaintext on distinct OS-picked ports (the dual-listener construction is required per §7.4 to demonstrate the `destination_port` precedence dimension across distinct connections). NO TLS / NO SNI in this fixture (the differential demonstration is on the **non-SNI** chain-match dimensions; SNI-based chain match is already covered by fixture 0002-tls-tcp). Both listeners carry the same 3 `filter_chains[]` entries (per §7.4 below for the exact match-dimension layout) plus the same `default_filter_chain`. Each chain's terminal filter is a TCP-proxy bound to a distinct STATIC cluster (one cluster per chain, one endpoint per cluster, one backend port per endpoint). 5 backends total: 4 chain-specific + 1 default-chain backend (shared across both listeners — same chain set, same backend pool). NO `listener_filters[]` (the fixture exercises chain-match without listener filters; `tls_inspector` integration is unit-tested separately per §15 + §11.4 carry-forward pin).
  - **`envoy.yaml`** — reference bootstrap. Same dual-listener / 3-chain + default shape as the subject. STRICT_DNS clusters per ADR-0010 (`host.docker.internal:<backend-port>` with `dns_lookup_family: V4_ONLY`); same backend-port allocation. `--concurrency 1` per ADR-0028.
  - **`expectations.yaml`** — prose description of the 5-connection workload + the per-connection expectation table (which backend port the connection should hit + which chain selected it).
  - **`README.md`** — explains the fixture's purpose (differential per-connection chain-selection equivalence), the STATIC-vs-STRICT_DNS divergence, the 5-connection shape, the chain-match-precedence demonstration (the connection that satisfies two chains' dimensions: `destination_port` wins over `source_prefix_ranges` per §11.3 empirical pin), the cross-reference to BEHAVIOR_CONTRACT `## Listener filters` (introduced at 07.2 phase-done).
  - **`driver/driver.go`** — `BackendCount() = 5`. `SubjectListenerNames() = ["l_test_a", "l_test_b"]`. `ReferenceListenerPorts() = [15008, 15009]` (one port per listener). `DriveReference(ctx, addrs)` / `DriveSubject(ctx, addrs)` issue 5 sequential TCP connections per §7.4's per-connection routing table (each connection picks a target listener address from the addrs map keyed by listener name, sends a fixed payload `"chain-probe\n"`, and reads the backend's response). The runner asserts the per-connection response body is byte-equal across subject and reference; equivalently, the per-connection backend-port distribution is identical. Connections that test specific source-port matches use a `net.DialContext` with a forced local-address binding (the driver pre-allocates a source port for those connections).
  - **`driver/driver_test.go`** — distribution-/expectation-assertion unit tests.
  - **`backends/main.go`** — small Go program that starts an HTTP/1.1 server on a configurable port; returns the listener address (host:port string) as the response body. 5 instances run, one per fixture chain.

- **`test/differential/runner.go`** (extended) — registration update: blank-import the new fixture-0008 driver package. The runner's per-fixture loop calls each driver's hooks per the existing in-band pattern. `0008` registers as `RequiresReference: true`.

### 4.4 Changed documentation and state (in 07.2)

- **`docs/envoy-go/ROADMAP.md`** — row `07.2`: `status: planned → in-progress` flipped at the SPEC commit (per the corrected pattern from 07.1 REVIEW I-3 finding — the 07.1 SPEC commit failed to flip its own row to `in-progress` and that's the precedent NOT to repeat); transitions to `done` at the 07.2 phase-done commit; row `07` (parent) flips `in-progress → done` AT THE SAME phase-done commit.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → phase 08 entry at lifecycle-state 0).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (extended in-place per ADR-0052's authorization, mirroring the 06.1 / 06.2 / 07.1 in-place-edit pattern at phase-done) — adds a new top-level section `## Listener filters` populated with the §11 empirical-pin blocks from this SPEC + the chain-match algorithm rules + the `default_filter_chain` semantics + the listener-filter dispatch contract + a new equivalence-matrix row. Amends the existing `## TCP proxy` "Does not yet apply to" to remove "Filter chain matching (`filter_chain_match` non-empty) — phase 07" (now in scope) and "Multiple filters in a chain — phase 07" (still partially out of scope — multiple network filters in one chain is still out; multiple listener filters in one pipeline IS in scope; the entry is rewritten). Amends the existing `## TLS` "Scope boundaries" to remove "ALPN-driven filter-chain selection", "non-SNI filter-chain match fields", "`Listener.default_filter_chain`", "`listener_filters` (still silently skipped)" — all now in scope. **The in-place edit lands at the 07.2 phase-done commit, NOT at the SPEC commit** (mirrors the 06.1 / 06.2 / 07.1 in-place-edit timing per the same ADR-0052 discipline).
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — UNCHANGED in 07.2 (no pin bump; D-3.7 reserves pin bumps for dedicated phases).
- **`docs/envoy-go/DECISIONS.md`** — seven new ADRs introduced by phase 07.2, numbered ADR-0077..ADR-0083 (next-free per the `DECISIONS.md` tail at master `424485b` being ADR-0076; the planner re-verifies next-free at write time per ADR-0004's autonomous-numbering rule). Topics enumerated in §10 below; the ADRs themselves are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it). ADR-0077 anchors the ROADMAP row-flip already landed at the SPEC commit, mirroring ADR-0070's 07.1 pattern.
- **`docs/envoy-go/phases/07-filter-chain-framework/SPEC.md`** — UNCHANGED in 07.2 (the parent master SPEC is read-only history once drafted at master `ee45aba`).
- **`docs/envoy-go/phases/07.2-listener-chain-completion/README.md`** — UNCHANGED in 07.2 (the sibling SPEC stub is read-only history once this SPEC commits, per the stub's §1 "this stub is read-only history once that SPEC commits" clause). Future readers consult THIS SPEC, not the stub.

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 07.2)

Phase 07.2 introduces one new package tree (`internal/listener/listenerfilter/`), substantially refactors `internal/listener/manager.go` (constructor signature widens + the dispatch path gains a listener-filter pipeline + the chain-match algorithm replaces the SNI-only specificity sort + `default_filter_chain` + `listener_filters[]` are honored), and threads a `*listenerfilter.ListenerFilterRegistry` parameter through one constructor chain (`listener.NewManager*`).

```
cmd/envoy-go/main.go                                  (MODIFIED: alloc *listenerfilter.ListenerFilterRegistry,
                                                       register tls_inspector, Freeze; thread through
                                                       listener-manager constructor chain)
cmd/envoy-go/main_test.go                             (MODIFIED: bootstrap variants thread a Registry)
internal/bootstrap/bootstrap.go                       (MODIFIED: blank-import tls_inspector v3 proto)
internal/listener/                                    (existing; substantially extended)
   doc.go                                             (UPDATE: add 07.2 architectural overview)
   manager.go                                         (MODIFIED: see §4.2 — ~+200 LoC net)
   manager_test.go                                    (MODIFIED: extend chain-match cases)
   listener_test.go                                   (MODIFIED: per-listener filter-set tests)
internal/listener/listenerfilter/                     (NEW package tree)
   doc.go                                             (NEW)
   types.go                                           (NEW: ListenerFilter, ListenerFilterStatus,
                                                       ChainMatchInputs, Peeker, factory pattern)
   callbacks.go                                       (NEW: peeker concrete, net.Conn wrapper)
   registry.go                                        (NEW: ListenerFilterRegistry + Register/Lookup/Freeze)
   pipeline.go                                        (NEW: per-connection sequential dispatch)
   chainmatch.go                                      (NEW: 8-dimension precedence algorithm)
   types_test.go, callbacks_test.go,
     registry_test.go, pipeline_test.go,
     chainmatch_test.go                               (NEW)
   fuzz_test.go                                       (NEW: FuzzFilterChainMatch)
internal/listener/listenerfilter/tls_inspector/      (NEW package; single concrete listener filter)
   tls_inspector.go                                   (NEW: ~250 LoC; ListenerFilter impl)
   tls_inspector_test.go                              (NEW)
   parser.go                                          (NEW: ~120 LoC ClientHello extractor)
   parser_test.go                                     (NEW)
   proto.go                                           (NEW: TlsInspector proto-config marshaling)
   doc.go                                             (NEW)
test/differential/0008-listener-chain-match/         (NEW fixture directory)
   envoy.yaml, envoy-go.yaml,
   expectations.yaml, README.md,
   driver/driver.go, driver/driver_test.go,
   backends/main.go                                   (NEW)
test/differential/runner.go                           (MODIFIED: register fixture 0008)

docs/envoy-go/ROADMAP.md                              (MODIFIED: row 07.2 planned → in-progress at SPEC,
                                                       → done at phase-done; row 07 → done at phase-done)
docs/envoy-go/STATE.md                                (MODIFIED at each lifecycle transition)
docs/envoy-go/BEHAVIOR_CONTRACT.md                    (MODIFIED at phase-done per ADR-0052)
docs/envoy-go/DECISIONS.md                            (NEW: ADR-0077..ADR-0083 at impl time)
docs/envoy-go/phases/07.2-listener-chain-completion/  (this directory)
   SPEC.md                                            (THIS file, drafted at this SPEC commit)
   PLAN.md                                            (drafted at lifecycle-state 2 → 3)
   PROGRESS.md                                        (drafted at impl-session-entry; SHA-fill convention)
   REVIEW.md                                          (drafted at lifecycle-state 5 → 6)
```

### 5.2 Per-connection lifecycle (new dispatch path)

The accept-loop's per-connection pipeline (replaces the current `acceptLoop` → direct-dispatch / `serveTLS` → `dispatch` shape):

```
listener.Manager.acceptLoop:
  for {
    raw, err := ln.Accept()
    if err != nil { ... }
    rt.downstreamCxTotal.Inc()
    rt.downstreamCxActive.Inc()
    go func(c net.Conn) {
      defer rt.downstreamCxActive.Dec()

      // (1) Allocate per-connection ChainMatchInputs from connection-level facts
      inputs := ChainMatchInputs{
        DestinationIP:   c.LocalAddr().(*net.TCPAddr).IP,
        DestinationPort: uint32(c.LocalAddr().(*net.TCPAddr).Port),
        SourceIP:        c.RemoteAddr().(*net.TCPAddr).IP,
        SourcePort:      uint32(c.RemoteAddr().(*net.TCPAddr).Port),
        // ServerName / TransportProtocol / ApplicationProtocols left zero;
        // populated by listener filters (e.g., tls_inspector) below.
      }

      // (2) Wrap raw conn in a peeker so listener filters can read without consuming
      pkConn := newPeekerConn(c)

      // (3) Run listener-filter pipeline
      //     If pipeline times out and continue_on_listener_filters_timeout=false, abort.
      //     If timeout=true, treat as Continue (inputs partially populated).
      pipelineErr := pipeline.Run(ctx, rt.listenerFilters, pkConn, &inputs, rt.lfTimeoutMs)
      if pipelineErr != nil && !rt.continueOnLfTimeout {
        log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, pipelineErr)
        _ = pkConn.Close()
        return
      }

      // (4) Run chain-match algorithm on populated inputs
      selected, err := chainmatch.SelectChain(inputs, rt.chains, rt.defaultChain)
      if err != nil {
        log.Printf("listener %q: no chain matched: %v", rt.name, err)
        _ = pkConn.Close()
        return
      }

      // (5) If selected chain has TLS, run handshake (using selected chain's tls.Config)
      var dispatchConn net.Conn = pkConn
      if selected.tlsCfg != nil {
        tlsConn := stdtls.Server(pkConn, selected.tlsCfg)
        if err := tlsConn.HandshakeContext(ctx); err != nil {
          log.Printf("listener %q: handshake: %v", rt.name, err)
          _ = pkConn.Close()
          return
        }
        dispatchConn = tlsConn
      }

      // (6) Dispatch to selected chain's terminal filter
      selected.filter.Handle(ctx, dispatchConn)
    }(raw)
  }
```

Compared to phase-03's flow (per `internal/listener/manager.go:513-558`), the changes are:

- The pre-handshake `GetConfigForClient`-callback-based chain selection is REPLACED. Chain selection happens AFTER the listener-filter pipeline runs and BEFORE the TLS handshake (if any). For chains with TLS, the selected chain's `tls.Config` is passed to `stdtls.Server()` directly, NOT via `GetConfigForClient` (because the chain is already known by then).
- A two-phase peek-then-handshake interaction is unavoidable for the `tls_inspector` case: the inspector reads the ClientHello bytes (via the peeker) BEFORE the TLS handshake; the same bytes are then re-read by `stdtls.Server.HandshakeContext()` from the peeker's buffered reader. The peeker discipline is what makes this work — see §5.3.
- For listeners with no `listener_filters[]`, the pipeline run in step (3) is a no-op (zero filters; `inputs` left at the connection-level-fact defaults; chain match operates on those alone). For listeners with no TLS and no listener filters, the path is identical in cost to phase-03's flow.

### 5.3 Peeker discipline (the bytes-not-consumed invariant)

The `tls_inspector` filter peeks the first ~512 bytes of the connection (typically a single ClientHello fits). After the listener-filter pipeline finishes, the same bytes must be available for the TLS handshake (or for whatever filter consumes the connection downstream). The `peekerConn` wrapper achieves this:

```go
type peekerConn struct {
  net.Conn         // the raw conn
  br *bufio.Reader // buffers reads
}

func newPeekerConn(c net.Conn) *peekerConn {
  return &peekerConn{Conn: c, br: bufio.NewReaderSize(c, peekerBufSize)}
}

// Peek implements the Peeker interface. Returns up to n bytes without consuming.
func (p *peekerConn) Peek(n int) ([]byte, error) {
  return p.br.Peek(n)
}

// Read OVERRIDES net.Conn.Read to drain from the buffered reader first.
// Once the buffer is exhausted, falls through to the underlying conn.
func (p *peekerConn) Read(b []byte) (int, error) { return p.br.Read(b) }
```

The peeker's `bufio.Reader.Peek(n)` returns the next n bytes WITHOUT advancing the read position — Go's stdlib guarantees this. Subsequent `Read` calls drain the same buffer, then transition to the underlying `net.Conn` once the buffer is exhausted. This is the same trick `crypto/tls.Conn.Handshake()` uses internally for resumption-cookie peek-back.

Buffer size: `peekerBufSize = 4096`. Decision (Decision C, §10 below + ADR-0079 anticipated): hardcoded constant; matches Envoy's `tls_inspector.initial_read_buffer_size` default. Larger ClientHellos (e.g., with many extensions, especially with PQ keys) require Envoy's `initial_read_buffer_size` configuration knob; envoy-go honors that knob with a clamp [256, 65536] per §4.1's `proto.go`.

### 5.4 Pipeline-deadline mechanism

Per Decision B + the `Listener.listener_filters_timeout` proto field's documented semantics. The pipeline's `Run` method enforces a per-pipeline deadline (NOT per-filter):

```go
func (p *Pipeline) Run(ctx, filters, peeker, inputs *ChainMatchInputs, timeoutMs uint32) error {
  if len(filters) == 0 || timeoutMs == 0 {
    return nil // no filters or zero deadline = no-op
  }
  ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
  defer cancel()
  for i, f := range filters {
    status, err := f.Inspect(ctx, peeker, inputs)
    if err != nil { return fmt.Errorf("listener-filter %d: %w", i, err) }
    if ctx.Err() != nil { return fmt.Errorf("listener-filter pipeline timeout: %w", ctx.Err()) }
    if status == StopIteration { return nil }
  }
  return nil
}
```

The deadline default per Decision B: 15s (matches Envoy's documented default for `listener_filters_timeout`). User-set values in [1s, 60s] are honored; values outside that envelope error at parse-time per §4.1's parser. The `continue_on_listener_filters_timeout` proto field is consulted by the listener manager AFTER `Run` returns: if `false` (default), a timeout-error from `Run` aborts the connection; if `true`, the listener manager treats the timeout as if `Run` returned nil (the chain-match algorithm runs on the partially-populated inputs).

### 5.5 Chain-match precedence algorithm (the heart of 07.2)

Per §1 #5 + the documented Envoy precedence + §11.3 empirical pin: chains are scored against an accepted connection's `ChainMatchInputs` in a 2-pass algorithm:

**Pass 1 — eligibility filter.** For each chain `c`, check each non-zero match dimension on `c.spec`; if any non-zero dimension does not match `inputs`, eliminate `c`. Empty match (`spec` with all-zero dimensions) is universally eligible (catch-all).

**Pass 2 — specificity scoring.** For each surviving chain, compute its specificity vector across the eight priority-ordered dimensions:

```
priorityOrder = [destination_port, prefix_ranges, server_names, transport_protocol,
                 application_protocols, source_type, source_prefix_ranges, source_ports]
```

Each chain `c`'s specificity vector is `[hasDim_0, hasDim_1, ..., hasDim_7]` where `hasDim_i = 1` if `c.spec` specifies the i-th dimension and `0` otherwise. The specificity vector is treated as an 8-bit integer (most-significant-bit = highest-priority dimension); higher integer = more specific. The chain with the highest specificity integer wins.

Within each dimension, ties are broken at finer grain when applicable:
- `prefix_ranges` ties: longer prefix length (more specific CIDR) wins. E.g., `192.168.1.0/24` beats `192.168.0.0/16` if both match.
- `source_prefix_ranges`: same rule.
- `server_names` ties: per ADR-0033 clause 9 (which 07.2 preserves as a special case): exact match > suffix wildcard (`*.foo.test`) > universal wildcard (`*`) > catch-all (empty). Same `chainSpecificityRank` function from `internal/listener/manager.go:352` is reused.
- All other dimensions: exact value match only — no sub-ordering.

**Final tie-breaker:** chains identical on all eight dimensions (e.g., two chains with the exact same `destination_port + source_prefix_ranges + ...`) are a config error at `NewManager`-build time. The validator detects them and returns `listener: %q: filter_chains[i] and filter_chains[j] have identical filter_chain_match — ambiguous selection`. This is the same discipline as duplicate listener-name rejection at `manager.go:185`.

**No-match → default.** If no chain is eligible, the algorithm falls through to `defaultChain` (if set) or returns `ErrNoChainMatched` (the listener manager logs and closes the connection).

**Empty-match-chain-vs-default.** Per §11.2 empirical pin: when both an empty-match chain (in `filter_chains[]`) AND `default_filter_chain` exist, the empty-match chain WINS. This is because the empty-match chain is universally eligible (Pass 1 admits it; Pass 2 gives it specificity score 0 — but every other chain is also eligible if they have no specific dimensions, and the empty-match chain is preferred when it's the only catch-all candidate). Concretely: the `defaultChain` is consulted ONLY when `len(eligibleChains) == 0` after Pass 1; an empty-match chain is always eligible (it's a chain in `filter_chains[]`); therefore `defaultChain` is reached ONLY when no `filter_chains[]` entry is eligible — i.e., every chain in `filter_chains[]` has at least one specific dimension AND the inputs satisfy none of those chains' dimensions.

### 5.6 Concurrency model

| Actor | Operation | Frequency | Locking |
|---|---|---|---|
| Boot | `ListenerFilterRegistry.Register` | Once per filter, at process start | `Registry.mu` Lock; panics if frozen |
| Boot | `ListenerFilterRegistry.Lookup` from `NewManager` | Once per listener-filter entry per listener at boot | `Registry.mu` RLock |
| Per-connection | `Pipeline.Run` | Per accepted connection | None — single goroutine per connection drives the pipeline |
| Per-connection | listener filter `Inspect` | Per filter per connection | None — driven by `Pipeline.Run` only |
| Per-connection | `chainmatch.SelectChain` | Per accepted connection | None — pure function over per-connection inputs + immutable chain list |
| Per-connection | `peekerConn.Peek` / `peekerConn.Read` | Per inspector + per filter consume | None — single goroutine per connection |

**Key invariant — single-goroutine-per-connection.** The accept-loop's per-connection goroutine is the only goroutine that drives the listener-filter pipeline AND the chain-match algorithm AND the post-handshake dispatch. The `peekerConn` is owned by that goroutine; no concurrent Read/Peek across goroutines. This makes the pipeline and chain-match algorithm lock-free.

**ListenerFilterRegistry freeze invariant** (mirrors 06.1 LBP-1 and 07.1 ADR-0072): `Register` panics post-Freeze; `Freeze()` is idempotent and called from `cmd/envoy-go/main.go` after all `Register` calls. A unit test sets the frozen flag and panics on `Register` after.

**Chain-list immutability.** The `[]*ChainSpec` and `*ChainSpec.defaultChain` are constructed once at `NewManager` time and never mutated. Concurrent `chainmatch.SelectChain` calls are read-only on the chain list; safe by construction.

### 5.7 ADR-0033 supersession enumeration (per Decision I, §10 below + ADR-0078 anticipated)

ADR-0033 (Phase-03 filter-chain subset) has 9 clauses. Each is now classified as STAYS or SUPERSEDED:

| Clause | Content | 07.2 disposition |
|---|---|---|
| 1 | `filter_chains` must be ≥ 1 (structural requirement) | **STAYS.** 07.2 preserves: a listener with zero `filter_chains[]` AND no `default_filter_chain` errors at parse. A listener with zero `filter_chains[]` AND a `default_filter_chain` is allowed (the default-only listener is a documented Envoy pattern; 07.2 honors it per Decision E). The ADR-0033 wording "must be ≥ 1" is updated to "the union of `filter_chains[]` and `default_filter_chain` must contribute at least one chain". |
| 2 | `filter_chain_match` may populate ONLY `server_names` + `transport_protocol == "tls"` | **PARTIALLY SUPERSEDED.** 07.2 honors the full 8-dimension `FilterChainMatch` (per §1 #4 + §5.5). Only `direct_source_prefix_ranges` remains silently-ignored (§9). |
| 3 | `Listener.default_filter_chain` set → error | **TOTALLY SUPERSEDED.** 07.2 honors the field per §1 #6. |
| 4 | `transport_socket` may be nil (plaintext) or carry a `DownstreamTlsContext` (TLS) | **STAYS.** Unchanged. |
| 5 | If any chain's `transport_socket` is non-nil, every chain on that listener must carry one — mixed TLS/plaintext error | **STAYS** (with caveat). The mixed-TLS-and-plaintext-on-one-listener restriction is preserved for `filter_chains[]` entries; the `default_filter_chain` MAY have its own `transport_socket` independent of the `filter_chains[]` entries' TLS posture. Decision E records this clarification. Rationale: `default_filter_chain` is structurally separate and Envoy's documented semantics support an independent TLS posture there; envoy-go follows. |
| 6 | Plaintext listeners with more than one `filter_chain` error | **PARTIALLY SUPERSEDED.** A plaintext listener with multiple `filter_chains[]` is now allowed if the chains use non-SNI dimensions for matching (`destination_port`, `prefix_ranges`, `source_*`). The original ADR-0033 rationale (SNI cannot match on plaintext) is preserved as a special case: a plaintext listener with multiple chains where AT LEAST ONE chain populates `server_names[]` errors at parse with the same `manager.go:323` message. The other six dimensions are valid-on-plaintext. |
| 7 | `require_client_certificate=true` on any chain errors | **STAYS.** Unchanged. |
| 8 | `listener_filters` is silently skipped (phase-02 carryover; phase 07 filter-chain framework revisits) | **TOTALLY SUPERSEDED.** 07.2 honors `listener_filters[]` per §1 #1 + §5.2. |
| 9 | Selection at handshake, in priority order: most-specific exact SNI > suffix-wildcard > universal wildcard > catch-all > no match (handshake fails) | **PRESERVED AS SPECIAL CASE.** The SNI-internal sub-ordering (exact > `*.foo` > `*` > empty) stays as the tie-breaker WITHIN the `server_names` priority slot of the new 8-dimension algorithm (§5.5). The `chainSpecificityRank` function at `manager.go:352` is reused verbatim. The "handshake fails" no-match case is replaced by "fall through to `default_filter_chain` if set; otherwise close conn" per §1 #6. |

**Net effect:** clauses 1, 4, 7 fully preserved; clauses 5, 6, 9 preserved with caveats; clauses 2, 3, 8 superseded.

## 6. Listener-filter framework surface — interfaces, status enum, callbacks

### 6.1 Filter interface

```go
type ListenerFilter interface {
  Inspect(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error)
  OnDestroy()
}
```

A filter implements `Inspect` synchronously (no async-resume per Decision A). The filter reads from `peeker` (without consuming) and writes to `*inputs` (mutating the chain-match inputs in place). On `Continue` the pipeline advances; on `StopIteration` the pipeline halts (whatever inputs were populated stand). On non-nil error, the pipeline aborts and the listener-manager handles the error per the `continue_on_listener_filters_timeout` field.

**Why no `SetCallbacks`?** Listener filters have a much narrower surface than HTTP filters — they read `peeker`, write `inputs`, return status. No need for a callback-bearing setter. Decision A records the simplification.

### 6.2 Status enum

```go
type ListenerFilterStatus int
const (
  Continue       ListenerFilterStatus = 0
  StopIteration  ListenerFilterStatus = 1
)
```

Two states only — there's no `StopIterationAndBuffer` (no body to buffer; the peeker buffer is not under filter control). Out-of-MVP variants (e.g., `RestartChainMatch` — re-run chain match after this filter modifies inputs) are not introduced; the inputs are naturally accumulated and the chain match runs once at end-of-pipeline.

### 6.3 Peeker interface

```go
type Peeker interface {
  Peek(n int) ([]byte, error)
}
```

Returns up to n bytes WITHOUT consuming. Implementations: the `peekerConn` wrapper from §5.3. Peek beyond the buffer's capacity (`peekerBufSize = 4096`) returns `bufio.ErrBufferFull`; filters that need more bytes for parsing should issue a smaller Peek (or, by Decision C, the buffer-size cap is lifted via the proto-config `initial_read_buffer_size` field). A filter that needs the full ClientHello (which can be ~1-2 KiB) operates within the 4 KiB default cleanly.

### 6.4 Two-step factory pattern (mirrors 07.1 ADR-0072)

```go
type ListenerFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)
type FilterInstanceFactory func() ListenerFilter   // called once per connection
```

Step 1 (NewManager-build time): `ListenerFilterFactory` parses + validates `typed_config`, returns a `FilterInstanceFactory` closure. Step 2 (per-connection): the closure allocates a fresh filter instance bound to the parsed config. Mirrors Envoy's `FilterFactoryFn` and 07.1's same pattern. Per-config validation cost paid once; per-connection cost is one allocation.

### 6.5 Pipeline deadline (per Decision B)

Per `Listener.listener_filters_timeout` proto field. Default 15s. Configurable in [1s, 60s] envelope; values outside error at parse. Per-pipeline deadline (not per-filter); `Pipeline.Run` issues a `context.WithTimeout(ctx, timeout)` once and the per-filter `Inspect` calls share that deadline — a slow first filter eats the second filter's budget. Decision B + ADR-0081 anticipated record this.

## 7. FilterChainMatch precedence rules

### 7.1 Eight match dimensions

The 07.2 chain-match algorithm consults eight dimensions of the v3 `FilterChainMatch` proto, populated from connection-level facts plus listener-filter-contributed inputs (per §1 #4):

| Dimension | Type | Source |
|---|---|---|
| `destination_port` | uint32 (optional) | connection-level: `conn.LocalAddr().(*net.TCPAddr).Port` |
| `prefix_ranges` | list of CIDR | connection-level: `conn.LocalAddr().(*net.TCPAddr).IP` matched against each CIDR |
| `server_names` | list of string | listener-filter contributed: `tls_inspector` peeks ClientHello SNI |
| `transport_protocol` | string | listener-filter contributed: `tls_inspector` sets `tls` or `raw_buffer` |
| `application_protocols` | list of string | listener-filter contributed: `tls_inspector` peeks ClientHello ALPN |
| `source_type` | enum (ANY/LOCAL/EXTERNAL) | connection-level: derived from `conn.RemoteAddr()` (loopback vs not) |
| `source_prefix_ranges` | list of CIDR | connection-level: `conn.RemoteAddr().(*net.TCPAddr).IP` matched against each CIDR |
| `source_ports` | list of uint32 | connection-level: `conn.RemoteAddr().(*net.TCPAddr).Port` matched against each port |

The ninth field `direct_source_prefix_ranges` (proxy-protocol) is silently ignored (§9).

### 7.2 Precedence ordering — citation + empirical confirmation

Per the `filter_chain_match.proto` upstream comments (Envoy v1.37.2 source `api/envoy/config/listener/v3/listener_components.proto`):

> Order matters as the chains are checked in order. (...) The most specific chain is matched. The order is checked from the more specific to the less specific: `destination_port`, `prefix_ranges`, `server_names`, `transport_protocol`, `application_protocols`, `source_type`, `source_prefix_ranges`, `source_ports`.

§11.3 empirical pin (resolved in this SPEC session against reference Envoy v1.37.2 server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) confirms this ordering on a 2-chain probe (`destination_port` vs `source_prefix_ranges`).

### 7.3 Algorithm pseudocode (reference for implementation)

```
function SelectChain(inputs ChainMatchInputs, chains []ChainSpec, default *ChainSpec) → *ChainSpec:
  eligible = []
  for c in chains:
    if matches(c.spec, inputs):  // every non-zero dimension of c.spec is satisfied by inputs
      eligible.append(c)
  if len(eligible) == 0:
    if default != nil: return default
    return nil, ErrNoChainMatched

  // score each eligible chain by specificity vector
  best = eligible[0]
  bestScore = specificityScore(best.spec)
  for c in eligible[1:]:
    cScore = specificityScore(c.spec)
    if cScore > bestScore:
      best = c; bestScore = cScore
    else if cScore == bestScore:
      // tie: walk down the priority list with finer-grain tie-breakers
      best = breakTie(best, c, inputs)
  return best
```

`specificityScore` returns an 8-bit integer where bit `i` is set if dimension `priorityOrder[i]` is present (most-significant-bit = `destination_port`). `breakTie(a, b, inputs)` returns whichever chain has the more-specific value within the highest-priority dimension where they differ in specifics (e.g., longer CIDR prefix on `prefix_ranges`, more-specific SNI per `chainSpecificityRank`).

**Worst case:** O(N × D) where N = number of chains, D = 8 (constant). For typical N (10-100 chains), the algorithm runs in microseconds per connection — well below the listener-filter timeout budget.

### 7.4 Fixture-0008 chain layout (concrete demonstration)

The fixture `0008-listener-chain-match` demonstrates the algorithm using **two listeners** sharing the same chain set. The dual-listener construction is required so that `destination_port` is a genuine discriminator across connections — a single-port listener cannot exercise the `destination_port` priority dimension. This SPEC commits to the dual-listener shape (no PLAN deferral); the planner records the exact 5-connection layout in the fixture's `expectations.yaml`.

Two listeners (`l_test_a` bound on `127.0.0.1:<port_a>`, `l_test_b` bound on `127.0.0.1:<port_b>`, both plaintext, both ports OS-picked) carry the **same** chain set:

```yaml
# Same filter_chains[] + default_filter_chain on BOTH l_test_a and l_test_b:
filter_chains:
- name: chain_dstport_alpha
  filter_chain_match:
    destination_port: <port_a>             # matches l_test_a only
  filters: [tcp_proxy → c_dstport]         # backend on port P_dstport

- name: chain_srcprefix_loopback
  filter_chain_match:
    source_prefix_ranges:
    - { address_prefix: 127.0.0.1, prefix_len: 32 }
    source_ports: [<known_driver_port>]    # driver pre-allocates this source port
  filters: [tcp_proxy → c_srcprefix]       # backend on port P_srcprefix

- name: chain_other
  filter_chain_match: {}                    # empty match — catch-all
  filters: [tcp_proxy → c_other]            # backend on port P_other

default_filter_chain:
  name: chain_default
  filters: [tcp_proxy → c_default]          # backend on port P_default
```

To exercise the `default_filter_chain` no-match path (connection 4), the driver targets a **third configuration variant** (`envoy-go-c4.yaml` / `envoy-c4.yaml`) in which `chain_other` is omitted from the chain set so that no `filter_chains[]` entry can match a non-loopback connection to `l_test_b`. The runner loads this variant for connection 4 only; the primary variant is used for connections 1, 2, 3, 5. (The variant-driven approach is a non-controversial harness extension — the fixture has two `envoy*.yaml` pairs.) The planner records the exact load-sequence in `expectations.yaml`.

Per-connection workload (5 connections — each row names the target listener and the expected winning chain explicitly):

| # | Target listener | Source IP / Port | Configuration variant | Expected winning chain | Backend port | Demonstrates |
|---|---|---|---|---|---|---|
| 1 | `l_test_a` (port_a) | non-loopback / OS-picked | primary | `chain_dstport_alpha` (only chain whose `destination_port` matches `port_a`) | P_dstport | `destination_port` populated chain wins over catch-all `chain_other` |
| 2 | `l_test_b` (port_b) | 127.0.0.1 / `<known_driver_port>` | primary | `chain_srcprefix_loopback` (`source_prefix_ranges` + `source_ports` match; `chain_dstport_alpha`'s `port_a` ≠ `port_b` so it's eliminated) | P_srcprefix | `source_prefix_ranges` + `source_ports` populated chain wins over catch-all |
| 3 | `l_test_b` (port_b) | non-loopback / OS-picked | primary | `chain_other` (catch-all empty-match — no other chain matches) | P_other | empty-match catch-all wins when no specific chain matches |
| 4 | `l_test_b` (port_b) | non-loopback / OS-picked | `c4` variant (`chain_other` removed) | `chain_default` (no `filter_chains[]` entry eligible — fall through to default) | P_default | `default_filter_chain` consulted when no `filter_chains[]` entry matches |
| 5 | `l_test_a` (port_a) | 127.0.0.1 / `<known_driver_port>` | primary | `chain_dstport_alpha` (BOTH `chain_dstport_alpha` AND `chain_srcprefix_loopback` match; precedence per §11.3 selects the `destination_port` chain) | P_dstport | precedence: `destination_port` (priority 1) BEATS `source_prefix_ranges` (priority 7) |

The differential equivalence claim is per-connection: each of the 5 connections produces a byte-equal response body across subject (envoy-go) and reference (Envoy v1.37.2). The per-connection backend-port distribution is therefore identical. Per Decision G, this design is committed in the SPEC; the planner refines the YAML at PLAN time but cannot revisit the dual-listener-with-c4-variant shape without an ADR.

## 8. `default_filter_chain` semantics

### 8.1 When consulted

Per §11.1 + §11.2 empirical pins. The `default_filter_chain` is consulted ONLY when the chain-match algorithm's Pass 1 (§5.5) yields zero eligible `filter_chains[]` entries. If at least one entry is eligible (including any empty-match chain, which is universally eligible), `default_filter_chain` is NEVER consulted.

### 8.2 Per-listener at-most-one

`Listener.default_filter_chain` is a single proto field (not a list); structurally guaranteed at most one. No additional validation needed.

### 8.3 TLS posture independence

Per Decision E + §5.7 clause 5: `default_filter_chain` MAY carry its own `transport_socket` independent of the `filter_chains[]` entries' TLS posture. Concretely: a listener with three plaintext `filter_chains[]` AND a TLS `default_filter_chain` IS valid (the inverse — TLS `filter_chains[]` + plaintext `default_filter_chain` — is also valid). The mixed-TLS-and-plaintext rule (ADR-0033 clause 5) applies WITHIN `filter_chains[]` only, not across `filter_chains[]` and `default_filter_chain`.

### 8.4 Interaction with empty-match chain

Per §11.2 empirical pin. When both an empty-match `filter_chain` (in `filter_chains[]`) AND a `default_filter_chain` exist, the empty-match chain WINS — `default_filter_chain` is never consulted. The empirical pin covered TCP-proxy-as-terminal-filter; the same semantics apply to HCM and any other terminal filter (the dispatch is filter-agnostic).

The phase-03 ADR-0033 clause "at most one catch-all chain per listener" (currently enforced at `internal/listener/manager.go:308` by the `catchAllCount > 1` check) is **preserved** for `filter_chains[]` empty-match entries — at most one. The `default_filter_chain` is a SEPARATE structural slot and does NOT count toward the `catchAllCount`. So a listener may have 0 or 1 empty-match chain AND 0 or 1 `default_filter_chain` — independently. Both 0/0, 1/0, 0/1, 1/1 are valid. Decision E enumerates this.

## 9. Differential / structural fixture

### 9.1 Equivalence claim

`0008-listener-chain-match` is **differential** (equivalence-with-Envoy): per-connection backend-port routing decision (i.e., which of the 5 backends each connection ends up at) byte-equal across envoy-go and reference Envoy v1.37.2 (modulo the existing differential ignore-list — none of the ignored fields apply to TCP-payload-byte-equality).

### 9.2 Driver outline

(See §7.4 above for the per-connection workload matrix.) Driver in-band; runner registers `RequiresReference: true`. The per-connection assertion: each TCP connection's response body is the backend's listener address (as a string `127.0.0.1:NNNN`) — distinct per backend → distinct per chain. The runner asserts the per-connection (`subjectResponse`, `referenceResponse`) pair matches.

### 9.3 Fixture id assignment

Per §7.4 + Decision G: single fixture `0008-listener-chain-match`, NOT split. The 8-dimension chain-match surface fits in one fixture (5 connections cover the priority-ordering corners — `destination_port` wins, `source_prefix_ranges` wins when alone, empty-match catches the catch-all, no-match falls to default, two-dimensions-match resolved by precedence). The 07.1 precedent of two fixtures (`0007a` + `0007b`) was driven by the differential-vs-structural split; 07.2 has no analogous split (no envoy-go-only test surface analogous to `envoy_go_test`).

### 9.4 Differential dimension chosen

Backend-port routing is the differential dimension. Alternatives considered:

- **Per-chain log line emission.** Rejected: Envoy and envoy-go log differently; line-by-line equivalence is not asserted in any other fixture.
- **Per-chain stat counter.** Considered: the `tcp.<stat_prefix>.downstream_cx_total` counter would tick on the chain whose TCP-proxy handled the connection. Rejected: would require fixture to read both proxies' stats endpoints; complicates the driver. Backend-port routing is simpler.
- **Per-chain backend-port distinction.** Selected. Each chain dispatches to a TCP-proxy bound to a distinct cluster with a distinct endpoint port. The per-connection response body is the backend's address (deterministic per port). Subject-vs-reference equality on response body is the assertion.

Decision G records this.

## 10. ADRs anticipated

Seven ADRs anticipated for 07.2, numbered ADR-0077..ADR-0083 (next-free per the `DECISIONS.md` tail at master `424485b` being ADR-0076; the planner re-verifies next-free at PLAN write time per ADR-0004's autonomous-numbering rule). The ADRs are authored at impl-time per the envoy-go convention (the SPEC names + describes them; the implementation commit lands the ADR alongside the production-code change that anchors it).

The numbering below is the expected mapping based on topical ordering; the planner may reorder commit-time landings if that reads more naturally in PLAN.md, in which case the actual ADR number assignments may permute (the ADR-0066..ADR-0069 block in 06.2 used non-monotonic commit-time ordering — this is permitted and recorded in each ADR's `Lands-in-task` field).

- **ADR-0077 — Phase-07.2 scope decision (split confirmation + listener-filter MVP boundary).** Status: Accepted. Doctrine: D-3.5 + D-3.6. Decision: 07.2 covers (a) `listener_filters[]` framework with `tls_inspector` as the first concrete filter; (b) full 8-dimension `FilterChainMatch` algorithm; (c) `Listener.default_filter_chain` honored. Explicit deferrals: `original_dst`, `proxy_protocol`, `http_inspector` listener filters; `direct_source_prefix_ranges` chain-match dimension; xDS LDS; listener-level access logging on chain-match-miss. Rationale: the MVP dispatch pipeline is exercised by `tls_inspector` alone; the additional surfaces are purely additive (new packages + new Register calls). Mirrors ADR-0070's 07.1 scope-confirmation pattern. **Anchors the ROADMAP edit landed at this SPEC commit (row 07.2 → in-progress)** — which is the 07.1 REVIEW I-3 pattern continued. Lands-in-task: 07.2 PLAN Task 1 (PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at the SPEC commit).

- **ADR-0078 — ADR-0033 partial supersession enumeration.** Status: Accepted. Supersedes (partial): ADR-0033 (Phase-03 filter-chain subset). Doctrine: D-3.5. Decision: clauses 1, 4, 7 fully preserved; clauses 5, 6, 9 preserved with caveats (TLS posture independence for `default_filter_chain`; multi-chain plaintext allowed for non-SNI dimensions; SNI-specificity preserved as tie-breaker within `server_names` priority slot); clauses 2, 3, 8 superseded (`FilterChainMatch` now 8-dimension; `default_filter_chain` honored; `listener_filters[]` honored). Full clause-by-clause table in §5.7 above. Lands-in-task: 07.2 PLAN Task wherever `internal/listener/manager.go:validateFilterChainMatch` is rewritten.

- **ADR-0079 — Listener-filter dispatch protocol shape (sync-only; narrow `Inspect(peeker, inputs)` surface; freeze-after-boot registry).** Status: Accepted. Doctrine: D-3.2 + D-3.5. Decision: `ListenerFilter` interface single method `Inspect(ctx, peeker, inputs) (Status, error)`; `Status` enum 2-state (`Continue`, `StopIteration`); synchronous-only (no async-resume — Decision A); `*ListenerFilterRegistry` threaded constructor (mirrors 07.1 ADR-0072 + 06.1 LBP-1); two-step factory pattern; per-connection sequential dispatch (Decision D supports multi-filter pipelines but MVP registers only `tls_inspector`); 4096-byte default peeker buffer (Decision C; clamped [256, 65536] via `initial_read_buffer_size` proto). Rationale: listener filters have a much narrower surface than HTTP filters (peek + populate inputs + return); async-resume + watermark events + body-buffering would all be unjustified machinery at MVP. Lands-in-task: 07.2 PLAN Task wherever `internal/listener/listenerfilter/types.go` + `pipeline.go` + `registry.go` first land.

- **ADR-0080 — `default_filter_chain` semantics (no-match fallback; empty-match chain wins; TLS posture independent).** Status: Accepted. Doctrine: D-3.3 + D-3.5. Decision: `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry's `filter_chain_match` matches the per-connection `ChainMatchInputs`; an empty-match chain in `filter_chains[]` BEATS `default_filter_chain` when both coexist; `default_filter_chain` may carry an independent `transport_socket` (TLS or plaintext) regardless of the `filter_chains[]` entries' TLS posture. The empirical pin in §11 (this SPEC) is the durable evidence — verbatim Envoy v1.37.2 stats output captured at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`. Supersedes: ADR-0033 clause 3. Lands-in-task: 07.2 PLAN Task wherever `chainmatch.SelectChain`'s default-fallback path lands.

- **ADR-0081 — `FilterChainMatch` 8-dimension precedence algorithm.** Status: Accepted. Doctrine: D-3.5 + D-3.6. Decision: priority order `[destination_port, prefix_ranges, server_names, transport_protocol, application_protocols, source_type, source_prefix_ranges, source_ports]`; eligibility-then-specificity 2-pass algorithm per §5.5; sub-orderings within `prefix_ranges` (longer prefix wins) and `server_names` (exact > suffix > universal > catch-all per ADR-0033 clause 9 preserved as special case); final ties at `NewManager`-build time error with `ambiguous selection`. The empirical pin in §11.3 (this SPEC) is the durable evidence for the priority ordering — verbatim Envoy v1.37.2 stats output confirming `destination_port` BEATS `source_prefix_ranges`. Supersedes: ADR-0033 clause 2 (partial). Lands-in-task: 07.2 PLAN Task wherever `chainmatch.SelectChain` lands.

- **ADR-0082 — `listener_filters_timeout` honored in [1s, 60s]; default 15s; `continue_on_listener_filters_timeout` honored.** Status: Accepted. Doctrine: D-3.5 + D-3.6. Decision: `Listener.listener_filters_timeout` proto field honored, with values clamped/validated to the [1s, 60s] envelope; default 15s if unset; values outside envelope error at parse with `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope`. `continue_on_listener_filters_timeout` honored as-is per the proto's documented semantics (`false` = abort connection on timeout; `true` = treat timeout as Continue and proceed to chain match with partial inputs). Rationale: the [1s, 60s] envelope reflects realistic deployment values; values < 1s risk false-positive timeouts under CI scheduler jitter; values > 60s indicate misconfiguration (the listener-filter pipeline should NEVER take longer). Lands-in-task: 07.2 PLAN Task wherever `Pipeline.Run`'s timeout enforcement + the bootstrap parser's envelope check land.

- **ADR-0083 — ADR-0050 disposition (no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist).** Status: Accepted. Settles: ADR-0050 (ALPN-driven codec selection inside `Filter.Handle`). Doctrine: D-3.5. Decision: ADR-0050 stays in force; 07.2's `application_protocols` chain-match field and ADR-0050's HCM-internal ALPN dispatch are orthogonal mechanisms (chain-selection vs codec-selection). They coexist by construction: a single-HCM-with-AUTO-codec listener uses ADR-0050's mechanic; a multi-chain listener with per-chain `application_protocols` + per-chain forced `codec_type` uses 07.2's mechanic; the AUTO branch in HCM is a no-op when `codec_type` is forced. Empirical-pin §11.4 carry-forward (per Decision K) WOULD verify this with a real probe but is deferred to impl time. Lands-in-task: 07.2 PLAN Task wherever the integration is documented (likely the PROGRESS preamble; this ADR is mainly explanatory and doesn't anchor a code change).

**Inline supersessions** (recorded in the ADRs above, not as separate ADRs):

- **ADR-0033 partial supersession** by ADR-0078 + ADR-0080 + ADR-0081. Clauses 2, 3, 8 superseded; rest preserved or preserved-with-caveats (full table §5.7).
- **ADR-0050 NOT superseded** by ADR-0083 — explicit non-supersession recorded.

(Phase 06.1 had 6 ADRs; 06.2 had 4; 07.1 had 7. Phase 07.2's 7 sits at the same high end as 07.1 — appropriate for a sub-phase that introduces a new dispatch pipeline + supersedes a load-bearing prior ADR + introduces a 4-decision matrix on chain-match algorithm + default-chain + timeout + ALPN-disposition.)

## 11. Empirical-pin block

Mirrors 06.1's Rule SN4 empirical-pin block (in `BEHAVIOR_CONTRACT.md ## Stat-name mapping` per ADR-0061), 06.2's verbatim access-log pin (per ADR-0066), and 07.1's four-pin block (per ADR-0075 + the 07.1 SPEC §11). Three of the four pin probes were executed against reference Envoy v1.37.2 at the `ENVOY_TARGET.md`-pinned image SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per the `/server_info` `version` field) on 2026-05-02 by this SPEC-drafting session. Each verbatim block below is paste-from-terminal output. The fourth pin (`tls_inspector`-populated ALPN) is **carry-forward to impl time** per Decision K (§12 below) because resolving it requires committing real probe-driver code (the ADR-0004 hard-gate forbids implementation artifacts at SPEC time).

These three blocks are what `BEHAVIOR_CONTRACT.md ## Listener filters` will carry verbatim at the 07.2 phase-done in-place edit (per ADR-0052; see §13).

### 11.1 Empirical pin #1 — `default_filter_chain` honored as no-match fallback

**Probe configuration:** listener with one specific-match `filter_chain` (`source_prefix_ranges: 127.0.0.1/32`) + a `default_filter_chain`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_loopback` for the loopback-source chain; `tcp_default` for the default chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-defaultchain.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_loopback
      filter_chain_match:
        source_prefix_ranges:
        - { address_prefix: 127.0.0.1, prefix_len: 32 }
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_loopback, cluster: c_loopback } } ]
    default_filter_chain:
      name: chain_default
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_default, cluster: c_default } } ]
```

**Verbatim Envoy `/server_info` (server SHA confirmation):**

```
$ curl -s http://127.0.0.1:19901/server_info | python3 -c "import json,sys; print(json.load(sys.stdin)['version'])"
5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL
```

**Verbatim Envoy `/config_dump` (chain-shape confirmation that Envoy parsed the config without error):**

```
LISTENER l_test
  CHAIN chain_loopback fcm= {"source_prefix_ranges": [{"address_prefix": "127.0.0.1", "prefix_len": 32}]}
  DEFAULT chain_default fcm= {}
```

**Probe (a) — connection from non-loopback source (Docker NAT bridge IP):**

Driver: a TCP connection from the host's Docker bridge address (i.e., NOT 127.0.0.1 from Envoy's perspective due to user-mode NAT). The connection is closed immediately because both backend clusters point to non-listening ports — but the dispatch decision is recorded in the per-chain `downstream_cx_total` stat counter.

Verbatim stats output (`/stats?filter=tcp_(loopback|default).downstream_cx_total`):

```
tcp.tcp_default.downstream_cx_total: 1
tcp.tcp_loopback.downstream_cx_total: 0
```

**Probe (b) — connection from 127.0.0.1 (intra-container loopback):**

Driver: `docker exec envoy-pin2 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'` — inside the Envoy container so the source IP is 127.0.0.1.

Verbatim stats output after probe (b):

```
tcp.tcp_default.downstream_cx_total: 1
tcp.tcp_loopback.downstream_cx_total: 1
```

**Conclusions (pinned):**

- `default_filter_chain` is honored: a connection that doesn't match any `filter_chains[]` entry IS dispatched into the default chain (`tcp_default.downstream_cx_total` ticked from 0 → 1 on probe (a)).
- A connection that matches a specific `filter_chains[]` entry is dispatched there (NOT to the default): `tcp_loopback.downstream_cx_total` ticked from 0 → 1 on probe (b); `tcp_default` did NOT tick a second time.
- envoy-go MUST honor `default_filter_chain`: when no `filter_chains[]` entry's `filter_chain_match` matches the per-connection inputs, the algorithm falls through to `default_filter_chain` and dispatches there.

### 11.2 Empirical pin #2 — empty-match `filter_chain` BEATS `default_filter_chain`

**Probe configuration:** listener with one empty-match `filter_chain` (`filter_chain_match` not set — equivalent to all-zero) + a `default_filter_chain`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_empty` for the empty-match chain; `tcp_default` for the default chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-emptyandef.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_empty
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_empty, cluster: c_a } } ]
    default_filter_chain:
      name: chain_default
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_default, cluster: c_b } } ]
```

**Boot:** Envoy starts cleanly (no parse-time error on having both an empty-match chain AND a `default_filter_chain` — the only pre-flight warning is the unrelated `connection limit` notice).

**Probe — connection from 127.0.0.1 (intra-container; would match either chain since both have no specifying dimensions):**

```
$ docker exec envoy-pin3 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'
cat: -: Connection reset by peer
```

**Verbatim stats output (`/stats?filter=tcp_(empty|default).downstream_cx_total`):**

```
tcp.tcp_default.downstream_cx_total: 0
tcp.tcp_empty.downstream_cx_total: 1
```

**Conclusions (pinned):**

- An empty-match `filter_chain` BEATS `default_filter_chain` when both coexist: `tcp_empty.downstream_cx_total` ticked from 0 → 1; `tcp_default.downstream_cx_total` STAYED AT 0.
- The `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry is eligible. An empty-match chain is universally eligible (it's a chain in `filter_chains[]` with no specific dimensions to fail), so it always wins over `default_filter_chain`.
- envoy-go's chain-match algorithm MUST: (a) treat empty-match `filter_chains[]` entries as universally eligible per Pass 1 of §5.5; (b) fall through to `default_filter_chain` ONLY if zero `filter_chains[]` entries are eligible.

### 11.3 Empirical pin #3 — `destination_port` BEATS `source_prefix_ranges` (precedence-ordering confirmation)

**Probe configuration:** listener with two `filter_chains[]` entries — one matching `destination_port: 10000` (the listener's own bind port; will match every connection on this listener), one matching `source_prefix_ranges: 127.0.0.1/32`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_dstport` for the destination-port chain; `tcp_srcprefix` for the source-prefix chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-precedence.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_dstport
      filter_chain_match:
        destination_port: 10000
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_dstport, cluster: c_a } } ]
    - name: chain_srcprefix
      filter_chain_match:
        source_prefix_ranges:
        - { address_prefix: 127.0.0.1, prefix_len: 32 }
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_srcprefix, cluster: c_b } } ]
```

**Probe — connection from 127.0.0.1 (intra-container; satisfies BOTH `destination_port: 10000` AND `source_prefix_ranges: 127.0.0.1/32`):**

```
$ docker exec envoy-pin4 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'
cat: -: Connection reset by peer
```

**Verbatim stats output (`/stats?filter=tcp_(dstport|srcprefix).downstream_cx_total`):**

```
tcp.tcp_dstport.downstream_cx_total: 1
tcp.tcp_srcprefix.downstream_cx_total: 0
```

**Conclusions (pinned):**

- When both chains match a connection, `destination_port` BEATS `source_prefix_ranges`: `tcp_dstport.downstream_cx_total` ticked from 0 → 1; `tcp_srcprefix.downstream_cx_total` STAYED AT 0.
- This confirms the priority ordering documented in `filter_chain_match.proto` upstream comments and codified in §7.2: `destination_port` (priority slot 0) is more specific than `source_prefix_ranges` (priority slot 6). The chain whose specifying dimension is at a higher-priority slot wins.
- envoy-go's `chainmatch.SelectChain` MUST score chains by the priority-ordered specificity vector per §5.5 and select the highest-scoring eligible chain.

### 11.4 Empirical pin #4 — `tls_inspector`-populated ALPN feeds `application_protocols` chain-match

**Status: RESOLVED at Task 16 of phase 07.2 impl session per Decision K.**

**Probe configuration:** listener with `tls_inspector` listener filter + two filter_chains discriminated by `application_protocols` (h2 vs http/1.1). Each chain has its own DownstreamTlsContext (real cert+key, ephemeral self-signed, generated at probe time only — NOT committed) advertising both ALPNs (`alpn_protocols: ["h2", "http/1.1"]`) and a per-chain TCP-proxy with distinct `stat_prefix` (`tcp_h2` for the h2 chain; `tcp_h1` for the http/1.1 chain). Bootstrap at `/tmp/envoy-07.2-impl-empirical/envoy-tls-alpn.yaml` (NOT committed; impl-time scratch directory per the SPEC §11 empirical-pin convention):

```yaml
static_resources:
  listeners:
  - name: l_tls
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    listener_filters:
    - name: envoy.filters.listener.tls_inspector
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
    filter_chains:
    - name: chain_h2
      filter_chain_match: { application_protocols: ["h2"] }
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
          common_tls_context:
            tls_certificates:
            - certificate_chain: { filename: /etc/tls/server.crt }
              private_key:       { filename: /etc/tls/server.key }
            alpn_protocols: ["h2", "http/1.1"]
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_h2, cluster: c_h2 } } ]
    - name: chain_h1
      filter_chain_match: { application_protocols: ["http/1.1"] }
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
          common_tls_context:
            tls_certificates:
            - certificate_chain: { filename: /etc/tls/server.crt }
              private_key:       { filename: /etc/tls/server.key }
            alpn_protocols: ["h2", "http/1.1"]
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_h1, cluster: c_h1 } } ]
```

**Verbatim Envoy `/server_info` (server SHA confirmation — same image as §11.1–§11.3):**

```
$ curl -s http://127.0.0.1:19901/server_info | python3 -c "import json,sys; print(json.load(sys.stdin)['version'])"
5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL
```

**Verbatim Envoy `/config_dump` (chain-shape confirmation that Envoy parsed the listener_filters[] + application_protocols match without error):**

```
LISTENER l_tls
  LFILTER envoy.filters.listener.tls_inspector
  CHAIN chain_h2 fcm= {"application_protocols": ["h2"]}
  CHAIN chain_h1 fcm= {"application_protocols": ["http/1.1"]}
```

**Probe (a) — TLS connection with `NextProtos: ["h2"]`:**

A Go probe (`probe.go` in the scratch directory) issues `tls.Dial("tcp", "127.0.0.1:19000", &tls.Config{InsecureSkipVerify: true, NextProtos: ["h2"]})`. The probe receives `cs.NegotiatedProtocol == "h2"` from `conn.ConnectionState()`, confirming the TLS handshake completed and Envoy advertised h2.

Verbatim stats output (`/stats?filter=tcp_(h2|h1).downstream_cx_total`):

```
tcp.tcp_h1.downstream_cx_total: 0
tcp.tcp_h2.downstream_cx_total: 1
```

**Probe (b) — TLS connection with `NextProtos: ["http/1.1"]`:**

Same probe with `--alpn http/1.1`. Receives `cs.NegotiatedProtocol == "http/1.1"`.

Verbatim stats output after probe (b):

```
tcp.tcp_h1.downstream_cx_total: 1
tcp.tcp_h2.downstream_cx_total: 1
```

**Verbatim `tls_inspector` per-listener-filter stats (corroborates the inspector ran and observed the ALPN extension):**

```
$ curl -s "http://127.0.0.1:19901/stats?filter=tls_inspector"
tls_inspector.alpn_found: 2
tls_inspector.alpn_not_found: 0
tls_inspector.client_hello_too_large: 0
tls_inspector.sni_found: 0
tls_inspector.sni_not_found: 2
tls_inspector.tls_found: 2
tls_inspector.tls_not_found: 0
tls_inspector.bytes_processed: P0(nan,1400) P25(nan,1425) P50(nan,1450) P75(nan,1475) P90(nan,1490) P95(nan,1495) P99(nan,1499) P99.5(nan,1499.5) P99.9(nan,1499.9) P100(nan,1500)
```

**Conclusions (pinned):**

- An HTTPS connection offering `NextProtos: ["h2"]` is dispatched to `chain_h2` (the chain whose `application_protocols: [h2]` matches): `tcp_h2.downstream_cx_total` ticked from 0 → 1 on probe (a); `tcp_h1.downstream_cx_total` STAYED AT 0.
- An HTTPS connection offering `NextProtos: ["http/1.1"]` is dispatched to `chain_h1` (the chain whose `application_protocols: [http/1.1]` matches): `tcp_h1.downstream_cx_total` ticked from 0 → 1 on probe (b); `tcp_h2.downstream_cx_total` STAYED AT 1 (did NOT tick further).
- The `tls_inspector` per-filter counters confirm the listener filter ran on both connections (`tls_found: 2`, `alpn_found: 2`, `sni_not_found: 2` — neither probe sent SNI). This is the empirical demonstration that `tls_inspector` populates `inputs.ApplicationProtocols` from the ClientHello's ALPN extension; without `tls_inspector` in `listener_filters[]`, `application_protocols` chain-match cannot fire (the chain-match algorithm has no source of ALPN data otherwise).
- envoy-go MUST run the `tls_inspector` listener filter BEFORE chain-match selection so that `inputs.ApplicationProtocols` is populated when chains discriminate on `application_protocols`. The §13.1 BEHAVIOR_CONTRACT integration enforces this at the phase-done commit (Task 17). ADR-0083 confirms this is orthogonal to ADR-0050's HCM-internal AUTO-codec dispatch — the chain-match `application_protocols` is what selects the chain, not the codec; the chain's terminal filter (HCM with forced `codec_type`, or here a TCP-proxy as the minimal probe) consumes the chain selection result.

**Empirical-pin scaffolding:** The probe lives at `/tmp/envoy-07.2-impl-empirical/{envoy-tls-alpn.yaml, server.crt, server.key, probe.go}` and is NOT committed (per the SPEC §11 empirical-pin convention — the verbatim outputs above are the durable evidence; the scaffolding is reproducible by re-running the probe against the same pinned image).

### 11.5 Synchronization with BEHAVIOR_CONTRACT.md

The three resolved blocks above (§§11.1–11.3) plus the §11.4 carry-forward at-impl-time will all be paste-verbatim into `BEHAVIOR_CONTRACT.md ## Listener filters` at the 07.2 phase-done in-place edit (per §13 below). The §11 block + the §13 block are synchronized (no drift permitted; future image bumps per `ENVOY_TARGET.md`'s refresh procedure that alter any of the four shapes require updating both locations in the same commit, mirroring the 06.1 / 06.2 / 07.1 paste-verbatim discipline).

## 12. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 07.2 silently ignores the following at parse time (no error, no honored behavior). These extend the 04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 silently-ignored sets per ADR-0041's amendment policy:

- `Listener.access_log[]` (listener-level access logging on chain-match-miss).
- `Listener.connection_balance_config` (per-listener connection balancing).
- `Listener.bind_to_port = false` (non-bound listeners).
- `Listener.reuse_port` (`SO_REUSEPORT`).
- `Listener.transparent` (`IP_TRANSPARENT`).
- `Listener.freebind` (`IP_FREEBIND`).
- `FilterChainMatch.direct_source_prefix_ranges` (proxy-protocol original-source-IP).
- `listener_filters[].filter_disabled` (CEL-driven per-conn disable).
- `tls_inspector.enable_ja3_fingerprinting` (JA3 fingerprinting).

The full silently-ignored set is the union of phases 04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1's silently-ignored sets plus 07.2's amendment above. The phase-04..07.1 ignored sets are NOT amended by this list — only extended. ADR-0041 (the original silent-ignore ADR) is amended (not superseded) to record the 07.2 additions; the amendment shape mirrors the 05.1 + 05.2 + 06.1 + 06.2 + 07.1 amendments.

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 New `## Listener filters` section (full population)

A new top-level section is added to `docs/envoy-go/BEHAVIOR_CONTRACT.md` between the existing `## HTTP filter chain` (line 514) and `## xDS wire state machine` (line 250 — note: the file's section ordering places `## xDS wire state machine` at line 250 BEFORE `## HTTP filter chain` at line 514; the new `## Listener filters` section is placed after `## HTTP filter chain` and before `## xDS wire state machine` per the planner's discretion at impl-time). The section is populated with the per-§§5–8 design subsections + the three §11 empirical-pin blocks verbatim + the §11.4 carry-forward block (filled at impl time per Decision K):

```
## Listener filters

*Introduced by phase 07.2. Justified by ADR-0077 (scope), ADR-0078 (ADR-0033
partial supersession), ADR-0079 (dispatch protocol), ADR-0080 (default_filter_chain
semantics; supersedes ADR-0033 clause 3), ADR-0081 (8-dimension precedence
algorithm; supersedes ADR-0033 clause 2 partial), ADR-0082 (listener_filters_timeout
[1s,60s] envelope), ADR-0083 (no supersession of ADR-0050; coexistence).*

### Asserted equivalence
- `default_filter_chain` honored as no-match fallback — verbatim scrape pinned in
  ### Empirical evidence (default_filter_chain fallback) below.
- Empty-match `filter_chain` BEATS `default_filter_chain` when both coexist —
  verbatim scrape pinned in ### Empirical evidence (empty-match-vs-default) below.
- `destination_port` BEATS `source_prefix_ranges` in chain-match precedence
  when both could match — verbatim scrape pinned in ### Empirical evidence
  (precedence-ordering) below.
- `tls_inspector`-populated ALPN feeds `application_protocols` chain-match —
  verbatim scrape pinned in ### Empirical evidence (tls_inspector ALPN) below
  (resolved at 07.2 impl time per the SPEC §11.4 carry-forward).
- 8-dimension chain-match precedence ordering: destination_port > prefix_ranges >
  server_names > transport_protocol > application_protocols > source_type >
  source_prefix_ranges > source_ports.

### Not asserted
- xDS LDS dynamic listener-filter / chain insertion / removal / reorder.
- `direct_source_prefix_ranges` chain-match dimension (proxy-protocol;
  silently ignored).
- `Listener.connection_balance_config`, `bind_to_port`, `reuse_port`,
  `transparent`, `freebind` (silently ignored).
- `listener_filters[].filter_disabled` (CEL-driven per-conn disable;
  silently ignored).
- `tls_inspector.enable_ja3_fingerprinting` (silently ignored).

### Listener-filter dispatch protocol
- Synchronous-only (no async-resume).
- `ListenerFilter.Inspect(ctx, peeker, inputs)` returns Continue or
  StopIteration; on StopIteration the pipeline halts with whatever inputs
  were populated.
- Per-pipeline timeout (`Listener.listener_filters_timeout`); default 15s;
  honored in [1s, 60s] envelope; `continue_on_listener_filters_timeout`
  honored as proto-documented.
- Single-goroutine-per-connection iteration; no per-filter goroutine.
- `ListenerFilterRegistry` is boot-populated, frozen-after-boot (mirrors 07.1
  `*HTTPRegistry` LBP-1 per ADR-0072).

### Chain-match algorithm
- 8 dimensions: destination_port, prefix_ranges, server_names,
  transport_protocol, application_protocols, source_type, source_prefix_ranges,
  source_ports.
- 2-pass algorithm: (1) eligibility (every non-zero dimension must match);
  (2) specificity scoring by priority-ordered vector.
- Tie-breakers within dimensions: longer CIDR prefix (prefix_ranges /
  source_prefix_ranges); SNI-specificity (exact > suffix > universal >
  catch-all per ADR-0033 clause 9 preserved as special case); exact value
  match for all other dimensions.
- Final ties (chains identical on all 8 dimensions) error at NewManager-build
  time.
- No-match → `default_filter_chain` (if set) → close conn (otherwise).

### default_filter_chain semantics
- Consulted ONLY when no `filter_chains[]` entry is eligible.
- Empty-match chain in `filter_chains[]` BEATS `default_filter_chain` when
  both coexist (the empty-match chain is universally eligible).
- TLS posture independent of `filter_chains[]` entries' TLS posture (mixed
  TLS-and-plaintext rule applies WITHIN `filter_chains[]` only).
- ADR-0033 clause 5 preserved with caveat: at most one catch-all chain in
  `filter_chains[]` AND at most one `default_filter_chain` (independent).

### Empirical evidence (default_filter_chain fallback)
[verbatim block from §11.1 — Envoy v1.37.2 server SHA confirmation +
config_dump + per-chain stats output]

### Empirical evidence (empty-match-vs-default)
[verbatim block from §11.2 — stats output showing tcp.tcp_empty.downstream_cx_total: 1
+ tcp.tcp_default.downstream_cx_total: 0]

### Empirical evidence (precedence-ordering)
[verbatim block from §11.3 — stats output showing tcp.tcp_dstport.downstream_cx_total: 1
+ tcp.tcp_srcprefix.downstream_cx_total: 0]

### Empirical evidence (tls_inspector ALPN)
[verbatim block resolved at impl time per §11.4 carry-forward]

### Applies to
- Phase 07.2 onward (listener filters + 8-dimension FilterChainMatch +
  default_filter_chain).

### Does not yet apply to
- Listener filters beyond `tls_inspector` (`original_dst`, `proxy_protocol`,
  `http_inspector` deferred — §2.1).
- xDS LDS dynamic listener configuration (xDS family).
- `direct_source_prefix_ranges` chain-match dimension (bundled with
  proxy-protocol filter phase).
- HTTP/3 + QUIC listener filters (HTTP/3 family).
```

### 13.2 New `## Equivalence Matrix` row

The `## Equivalence Matrix` table at `BEHAVIOR_CONTRACT.md` lines 9–28 gains a new row appended:

```
| Dimension              | Equivalence claim                                        | Allow-list / tolerance                |
|------------------------|----------------------------------------------------------|---------------------------------------|
| Listener filters       | Per-connection chain-selection equivalence: which       | Differential covers chain-selection   |
|                        | filter_chain is dispatched is byte-equal across         | only (which backend each connection   |
|                        | envoy-go and reference Envoy. Verified via per-         | is routed to). Listener-filter        |
|                        | connection backend-port routing in fixture 0008.        | internal byte-level behavior          |
|                        | Chain-match precedence ordering, default_filter_chain   | (e.g., tls_inspector parser output)   |
|                        | fallback semantics, and empty-match-vs-default          | is unit-tested only.                  |
|                        | resolution are verbatim-pinned at the ENVOY_TARGET SHA. |                                       |
```

### 13.3 Existing-section amendments

The existing `## TCP proxy` (line 330) and `## TLS` (line 372) sections of `BEHAVIOR_CONTRACT.md` mention listener-side scope boundaries that 07.2 changes:

- **`## TCP proxy ### Does not yet apply to` (line 360):** the entries "Filter chain matching (`filter_chain_match` non-empty) — phase 07" and "Multiple filters in a chain — phase 07" are removed. The "Multiple filters in a chain" entry is rewritten to clarify: "Multiple network filters in a single filter_chain (e.g., chained `redis_proxy + tcp_proxy`) — Network filters family. Multiple listener filters in a listener_filters[] pipeline IS supported as of 07.2."
- **`## TLS ### Scope boundaries` (line 405):** the entries "ALPN-driven filter-chain selection", "non-SNI filter-chain match fields", "`Listener.default_filter_chain`", "`listener_filters` (still silently skipped)" are all removed (now in scope). A forward-pointer is added: "See `## Listener filters` for the listener-side filter primitives."

The phase-03 BEHAVIOR_CONTRACT mention of ADR-0033's filter-chain subset is preserved verbatim — the wire-level claims don't change; the ADR-0033 reference remains accurate (it's now noted as partially superseded by ADR-0078 + ADR-0080 + ADR-0081 per §5.7 above; the partial-supersession-but-not-deletion discipline is the same one phase 07.1 used for ADR-0040 + ADR-0042 references).

## 14. Deferred decisions (the planner / implementer settles these)

Items the SPEC names but does not finalize; the planner closes them in PLAN.md or the implementer closes them at task time per the SPEC's recommendation.

1. **Decision A — Listener-filter dispatch protocol: sync-only at MVP.** Settled per §6.1 + ADR-0079.

2. **Decision B — Listener-filter pipeline timeout: 15s default, [1s, 60s] envelope.** Settled per §6.5 + ADR-0082. Planner re-verifies at PLAN time that the envelope-check error message is consistent with the rest of `internal/listener/manager.go`'s error-message conventions.

3. **Decision C — Peeker buffer size: 4096 default; clamped [256, 65536].** Settled per §5.3 + ADR-0079. Planner re-verifies the lower bound (256) at PLAN time — TLS ClientHellos minimum is ~50 bytes; 256 is safe.

4. **Decision D — Multi-listener-filter pipelines: supported at MVP via sequential dispatch; only `tls_inspector` registered.** Settled per §1 #1 + §6.1 + §6.4 + ADR-0079. The PLAN's `pipeline_test.go` task includes a 2-filter test using two synthetic test-only filters (a `noop` + a `setSNI` probe; OR, equivalently, two `tls_inspector` instances with different proto config — the latter is preferable since it avoids test-only Go code in production packages).

5. **Decision E — `default_filter_chain` semantics: no-match fallback; empty-match wins; TLS independent.** Settled per §5.7 + §8 + ADR-0080.

6. **Decision F — `original_dst` listener filter: deferred.** Settled per §1 #3 + §2.1 + ADR-0077. Planner records in PLAN.md as a pending future-phase item.

7. **Decision G — Fixture id + shape: single fixture `0008-listener-chain-match`; differential; backend-port routing as the differential dimension; dual-listener (`l_test_a` + `l_test_b`) construction with a connection-4-only configuration variant (`chain_other` omitted) for the no-match → `default_filter_chain` path.** Settled per §7.4 + §9 + ADR-0077. The dual-listener-with-c4-variant shape is committed in the SPEC; the planner refines the YAML at PLAN time (resolving exact backend-port allocations, the exact `<known_driver_port>` value, source-bind details for connections 2 and 5) and records the 5-connection sequence in `expectations.yaml` but cannot revisit the dual-listener-with-c4-variant shape without an ADR.

8. **Decision H — ADR-0050 disposition: no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist.** Settled per §2.5 + ADR-0083.

9. **Decision I — ADR-0033 supersession enumeration: clauses 1, 4, 7 stay; 5, 6, 9 stay-with-caveat; 2, 3, 8 superseded.** Settled per §5.7 + ADR-0078.

10. **Decision J — Listener-filter API mirrors 07.1's HTTP-filter API at the surface level (registry + 2-step factory + freeze-after-boot) but is narrower (1 method `Inspect`; 2-state status; no callbacks).** Settled per §6 + ADR-0079.

11. **Decision K — `tls_inspector`-populated ALPN empirical pin: carry-forward to impl time.** Settled per §11.4. The implementer produces the verbatim Envoy evidence at the first listener-filter integration task in PLAN.md and pins it in BEHAVIOR_CONTRACT.md at phase-done.

12. **Decision L — `chainmatch.SelectChain` chain ordering: declaration order (input-list order) preserved; chains NOT pre-sorted; specificity-scored at dispatch time.** Settled per §5.5. Rationale: pre-sorting chains by specificity at `NewManager` time would require defining a total order across the 8-dimension specificity vector — but the order is already implicit in the priority list (§7.2); pre-sorting adds no benefit and obscures the algorithm. Planner records in PLAN.md.

13. **Decision M — Concrete ADR numbers ADR-0077..ADR-0083.** Per `DECISIONS.md` tail at master `424485b` being ADR-0076, the next-free is ADR-0077; 07.2's seven ADRs land at ADR-0077..ADR-0083. The planner re-verifies next-free at write time (per ADR-0004's autonomous-numbering rule) and assigns the seven anticipated topics to the seven numbers in the order they're authored in PLAN.md. The topical ordering above (scope / ADR-0033-supersession / dispatch-protocol / default-chain / chain-match-algorithm / timeout / ADR-0050-disposition) is the suggested authoring order; the planner may permute.

14. **Decision N — `pipeline.go` per-filter timeout split: per-pipeline (Decision B) NOT per-filter.** Settled per §6.5. Per-filter timeout split would force `len(filters)` context-derivations per connection AND would penalize multi-filter pipelines unfairly (a slow first filter can eat the budget; that's correct because the user's budget is the budget for all filters combined). Planner records in PLAN.md.

15. **Decision O — `chainmatch.go` builds the `[]*ChainSpec` from the bootstrap's `filter_chains[]` at `NewManager` time; the `*ChainSpec` is immutable thereafter.** Settled per §5.6. Two viable shapes: (a) a `[]*ChainSpec` slice on `listenerRuntime.chains` (as proposed in §4.2 + §5.1) — concurrent reads are safe by construction; (b) a sync.Map keyed by chain-name — overkill for this scale (chain count is single-digit on typical deployments). **Recommendation: (a)** — simpler. Planner records in PLAN.md.

## 15. Testing strategy

### 15.1 Unit tests (`internal/listener/listenerfilter/`)

- **`registry_test.go`** — Register / Lookup / duplicate-name panic / Freeze / post-Freeze panic; concurrent `Lookup` calls (race-clean under `-race`).
- **`pipeline_test.go`** — sequential dispatch with all status combinations (Continue / StopIteration); per-pipeline timeout (synthesized slow filter; deadline exceeded; `continue_on_listener_filters_timeout` true → returns nil with partial inputs; false → returns error); 2-filter pipeline with a synthetic test-only `noop` filter + a `setInputsTLS` filter that mutates `inputs.TransportProtocol`; ctx cancel mid-pipeline aborts; the per-connection `ChainMatchInputs` struct is populated correctly from `conn.LocalAddr()` / `conn.RemoteAddr()` at Pipeline construction.
- **`chainmatch_test.go`** — 8-dimension chain-match correctness: each priority dimension tested in isolation (1 chain matching only on `destination_port` vs another matching only on `source_prefix_ranges` → former wins per §11.3 pin); multi-dimension chains (a chain matching both `destination_port` AND `prefix_ranges` beats a chain matching only `destination_port` because tie-breaker on next priority dimension applies); empty-match chain in `filter_chains[]` beats `default_filter_chain` (§11.2 pin); `default_filter_chain` consulted only when no chain eligible (§11.1 pin); `prefix_ranges` longer-prefix tie-breaker (`192.168.1.0/24` beats `192.168.0.0/16`); `server_names` SNI-specificity tie-breaker (exact > suffix > universal > catch-all per ADR-0033 clause 9); identical chains → `NewManager`-build error; no chain + no default → `ErrNoChainMatched`.
- **`callbacks_test.go`** — `peekerConn.Peek(n)` returns first n bytes without consuming; subsequent `Read` returns the same bytes; Peek beyond `peekerBufSize` returns `bufio.ErrBufferFull`; concurrent peek + read is safe (single-goroutine invariant means we test sequential — but we DO test interleaved peek-read on the same goroutine for correctness).

### 15.2 Unit tests (`internal/listener/listenerfilter/tls_inspector/`)

- **`parser_test.go`** — table-driven on hand-crafted ClientHello byte sequences: full ClientHello with both SNI + ALPN; SNI-only; ALPN-only; no extensions; truncated; malformed length prefix; ALPN with multiple protocols (e.g., `["h2", "http/1.1"]`); ALPN with single protocol; SNI with multiple hostnames (per the `ServerNameList` proto — uses only the first per Envoy convention).
- **`tls_inspector_test.go`** — full filter behavior: real ClientHello (generated via `crypto/tls.Conn` writing to a `bytes.Buffer`) → `Inspect` populates `inputs.ServerName`, `inputs.TransportProtocol="tls"`, `inputs.ApplicationProtocols`; non-TLS preamble (e.g., HTTP/1.1 GET) → `Inspect` sets `inputs.TransportProtocol="raw_buffer"`, leaves SNI + ALPN at zero; concurrent inspection on independent connections is race-clean; the type_url + factory (`tls_inspector.New`) round-trip through the registry.

### 15.3 Unit tests (`internal/listener/`)

- **`manager_test.go`** (extended) — `validateFilterChainMatch` accepts the seven new dimensions (parses without error); rejects `direct_source_prefix_ranges` (silently ignored — does NOT error); chain-precedence sorting (the tests previously exercising `chainSpecificityRank` are extended to exercise the new 8-dimension algorithm via a synthetic 5-chain matrix).
- **`listener_test.go`** (extended) — listener-filter pipeline integration tests: a listener with a `tls_inspector` listener-filter populates inputs correctly; chain-match algorithm runs after the pipeline; `default_filter_chain` is honored on no-match; timeout-on-listener-filter aborts when `continue_on_listener_filters_timeout=false`.
- **`integration_test.go`** (NEW) — end-to-end accept-loop test: a TLS connection with SNI + ALPN dispatches to the matching chain; a plaintext connection dispatches to the matching `destination_port` chain; a non-matching connection falls to `default_filter_chain`. These tests exercise the full §5.2 dispatch path under unit-test conditions (no Docker; pure Go).

### 15.4 Differential fixture `0008-listener-chain-match`

(See §7.4 + §9 above for matrix + equivalence claim.) Per-connection backend-port routing: byte-equal across envoy-go and Envoy. Driver in-band; runner registers `RequiresReference: true`.

### 15.5 h2spec re-run (gate (c))

Phase 07.2's surface is pre-HCM; the H2 wire codec is unchanged. The h2spec gate at 53/53 must remain green — the listener-filter pipeline runs BEFORE HCM dispatches; nothing about chain-match changes how H2 frames flow on the wire after the chain is selected. Existing gate (c) re-runs unchanged at the ADR-0051 SHA pin.

### 15.6 Fuzzers (gate (d))

Existing 9 fuzzers re-run at the 30s ADR-0018 budget. **NEW: `internal/listener/listenerfilter.FuzzFilterChainMatch`** — fuzzes adversarial chain-match input combinations + adversarial chain-spec lists into `chainmatch.SelectChain`. Cheap (~80 LoC); adversarial-config bugs are the most likely class of bug in the new chain-match algorithm. Total: 10 fuzzers post-07.2.

The fuzzer's input space:

- **`ChainMatchInputs`** corners: zero/loopback/IPv6/large-port destination IP+port; zero/loopback/IPv6/large-port source IP+port; empty/short/long/UTF-8 server name; empty/`tls`/`raw_buffer`/garbage transport protocol; empty/single/long/duplicate ALPN list.
- **`[]*ChainSpec`** corners: 0/1/2/100 chains; identical chains (should error at build time, but fuzzer feeds them to `SelectChain` directly to verify the runtime no-error contract); chains with overlapping CIDRs; chains with longer/shorter prefix lengths; chains with exact/wildcard SNI; chains with mixed-priority dimensions.
- **`defaultChain`** corners: nil; empty-match.

Assertions:

1. `SelectChain` never panics.
2. Returned chain is one of the input chains, OR `defaultChain`, OR `nil` (with `ErrNoChainMatched`).
3. Returned chain's match dimensions are all satisfied by `inputs`.
4. On identical-priority ties, the algorithm is deterministic (same inputs → same chain) — the fuzzer asserts this by running the same input twice and asserting the result is identical.

### 15.7 Race detector + lint (gate (e))

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean across (per §5.6):

- N goroutines accepting connections on the same listener, each running its own pipeline (no shared mutable state).
- Concurrent `ListenerFilterRegistry.Lookup` calls from N listener-manager constructors at boot.
- `peekerConn.Peek` + `peekerConn.Read` interleaved on the same connection.
- The registry's `Freeze()` invariant: post-Freeze `Register` panics; concurrent `Freeze` calls are idempotent.

Unit tests in `pipeline_test.go` + `registry_test.go` exercise each. Differential fixture `0008-listener-chain-match` indirectly stresses end-to-end concurrency under load.

## 16. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 07.2's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) **non-vacuous** (fixture 0008 differentially green; per-connection backend-port routing byte-equal subject vs reference).
- [ ] `internal/listener/listenerfilter/` package exists; `ListenerFilter` interface + status enum + `ChainMatchInputs` + `Peeker` interface defined in `types.go`; `peekerConn` concrete implementation in `callbacks.go`; `ListenerFilterRegistry` + `Register` + `Lookup` + `Freeze` in `registry.go`; `Pipeline` + per-connection sequential dispatch in `pipeline.go`; `chainmatch.SelectChain` 8-dimension algorithm in `chainmatch.go`; per-package `doc.go`.
- [ ] `internal/listener/listenerfilter/tls_inspector/` package exists; `tls_inspector.New` factory registered at boot; preflight ClientHello-parse populates `inputs.ServerName` + `.ApplicationProtocols` + `.TransportProtocol="tls"`; non-TLS preamble sets `.TransportProtocol="raw_buffer"`; per-package `doc.go`.
- [ ] `internal/listener/manager.go` is rewritten: `validateFilterChainMatch` accepts the seven new dimensions; `default_filter_chain` is parsed (no longer errors); `listener_filters[]` is parsed; chain-match is dispatched via `chainmatch.SelectChain` (the SNI-only specificity sort `chainSpecificityRank` at line 352 is preserved as the tie-breaker WITHIN `server_names`); the post-handshake `dispatch` function is replaced by the unified pre/post-handshake path per §5.2.
- [ ] `internal/listener/manager.go` constructor signature widens: `NewManagerWithBaseDirAndAllowH2C` gains a `*listenerfilter.ListenerFilterRegistry` parameter; `NewManager` + `NewManagerWithBaseDir` delegate.
- [ ] `cmd/envoy-go/main.go` allocates `*listenerfilter.ListenerFilterRegistry`, registers `tls_inspector`, calls `Freeze()` BEFORE `listenerManager.New(...)`.
- [ ] **`ListenerFilterRegistry` freeze-after-boot invariant enforced**: post-Freeze `Register("X", ...)` panics with `listenerfilter: registry frozen: cannot register %q post-boot`. Unit test in `registry_test.go` asserts the panic.
- [ ] **All three §11 in-session empirical pins verbatim-present** in §11 of THIS SPEC (the SPEC commit, not a follow-up). Reviewer at REVIEW time grep-checks: (a) §11.1's stats output for `tcp.tcp_default.downstream_cx_total` + `tcp.tcp_loopback.downstream_cx_total` showing distinct chain-dispatch outcomes; (b) §11.2's stats output for `tcp.tcp_empty.downstream_cx_total: 1, tcp.tcp_default.downstream_cx_total: 0`; (c) §11.3's stats output for `tcp.tcp_dstport.downstream_cx_total: 1, tcp.tcp_srcprefix.downstream_cx_total: 0`.
- [ ] **§11.4 carry-forward pin documented + obligation routed to PLAN's first listener-filter integration task** (per Decision K).
- [ ] **`BEHAVIOR_CONTRACT.md ## Listener filters` section landed at phase-done commit (NOT SPEC commit)** with the three resolved §11 empirical-pin blocks verbatim AND the §11.4 carry-forward block updated with the impl-time-resolved verbatim Envoy output, plus the chain-match algorithm rules + the `default_filter_chain` semantics + the listener-filter dispatch contract + the equivalence-matrix new row per §13.1 below. The §11 block + the §13 block are paste-verbatim-synchronized (no drift; future image bumps require updating both in the same commit).
- [ ] `BEHAVIOR_CONTRACT.md ## Equivalence Matrix` has the new "Listener filters" row from §13.2 below.
- [ ] `BEHAVIOR_CONTRACT.md ## TCP proxy "Does not yet apply to"` updated: "Filter chain matching (`filter_chain_match` non-empty) — phase 07" entry removed; "Multiple filters in a chain — phase 07" entry rewritten to clarify the in-scope (multiple listener filters) vs out-of-scope (multiple network filters in a single chain).
- [ ] `BEHAVIOR_CONTRACT.md ## TLS "Scope boundaries"` updated: "ALPN-driven filter-chain selection", "non-SNI filter-chain match fields", "`Listener.default_filter_chain`", "`listener_filters` (still silently skipped)" all removed from the out-of-scope enumeration.
- [ ] All seven 07.2 ADRs (the planner-assigned ADR-0077..ADR-0083 mapping to scope / ADR-0033-supersession / dispatch-protocol / default-chain / chain-match-algorithm / timeout / ADR-0050-disposition) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. Inline-supersession notes in ADR-0078 (partial supersession of ADR-0033) and ADR-0080 + ADR-0081 (further partial supersessions of ADR-0033 clauses) are explicit. ADR-0083 explicitly NOT-superseding-ADR-0050 is explicit. The ADR-numbering-shift discipline from ADR-0045 + ADR-0004 is honored (the planner verified next-free at write time and the seven numbers are contiguous; topical-vs-commit-order non-monotonicity is permitted and recorded in each ADR's `Lands-in-task` field per the 06.2 + 07.1 precedent).
- [ ] Fixture `0008-listener-chain-match/` is committed in full: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The 5-connection workload + per-connection backend-port-equivalence assertion shape is implemented; runner registers as `RequiresReference: true`.
- [ ] `test/conformance/h2spec/` is UNCHANGED; pin still at the ADR-0051 SHA; 53/53 PASS.
- [ ] No phase-04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 fixture (`0000`/`0001`/`0002`/`0003`/`0004`/`0005`/`0006`/`0007a`/`0007b`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `STATE.md` is at lifecycle-state 6 for 07.2; `ROADMAP.md` row `07.2` is `done`; row `07` (parent) is `done` AT THE SAME COMMIT as 07.2's phase-done. The §3 + §5.3 phase-done commit's message names every ADR introduced (ADR-0077..ADR-0083) AND both ROADMAP-row transitions (`07.2 → done` AND `07 → done`).
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the phase-04..07.1 verification protocol; SHA-fill for each task entry per the convention.
- [ ] **`FuzzFilterChainMatch` is committed** under `internal/listener/listenerfilter/fuzz_test.go`; runs clean at the 30s ADR-0018 budget; total fuzzer count post-07.2 is 10.
- [ ] No third-party listener-filter or chain-match library is imported. The `internal/listener/listenerfilter/` package's external dependencies are limited to the Go standard library (`bufio`, `context`, `crypto/tls` (read-only — for ClientHello parser reference), `errors`, `fmt`, `net`, `sync`, `sync/atomic`, `time`) plus `google.golang.org/protobuf` (proto runtime) and the upstream `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3` for the `TlsInspector` proto type only.

When all boxes above are checked, phase 07.2 is `done`, the parent row `07` is also `done` (at the same commit), and the project advances to phase 08 (admin-api-and-drain) at lifecycle-state 0.

## 17. References

- **Parent BRAINSTORM:** `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` — the brainstorm-close design source for both 07.1 and 07.2; §1's split table and §9's deferred-items table are the load-bearing inputs to this SPEC.
- **Parent master SPEC:** `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` — phase-07 parent; §3 (split rationale) + §5 (parent-row closure discipline) anchor THIS SPEC's mirror.
- **07.2 sibling-stub README:** `docs/envoy-go/phases/07.2-listener-chain-completion/README.md` — forward-looking placeholder this SPEC supersedes.
- **07.1 SPEC:** `docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md` — the structural template this SPEC mirrors (§-numbering, header tone, acceptance-bullet format, empirical-pin block structure). 07.1's `*HTTPRegistry` + freeze-after-boot pattern (per ADR-0072) is mirrored by 07.2's `*ListenerFilterRegistry`.
- **06.2 SPEC:** `docs/envoy-go/phases/06.2-access-log/SPEC.md` — the structural template for sub-phases that close their parent row at phase-done; §1's parent-closure language is mirrored in this SPEC's preamble.
- **05.2 SPEC:** `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` — earlier sub-phase-closure precedent.
- **BEHAVIOR_CONTRACT.md:** `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the contract this SPEC's §13 extensions land in (in-place edit at phase-done per ADR-0052). Existing `## TCP proxy` (line 330) and `## TLS` (line 372) sections are amended; new `## Listener filters` section is introduced.
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Cited verbatim in each §11 empirical-pin sub-block. The three resolved pin probes used the image at this SHA against bootstraps shaped to elicit each pinned shape (configs in `/tmp/envoy-07.2-empirical/` during the SPEC session; not committed to repo since they're SPEC-time scaffolding, not fixture artifacts; rm'd at session-end).
- **ADR-0033** (Phase-03 filter-chain subset): partially superseded by 07.2 per §5.7 + ADR-0078 + ADR-0080 + ADR-0081.
- **ADR-0050** (ALPN-driven codec selection inside `Filter.Handle`): NOT superseded by 07.2 per Decision H + ADR-0083.
- **ADR-0052** (BEHAVIOR_CONTRACT in-place edit authorization): cited (not amended) for the 07.2 phase-done in-place edit timing.
- **ADR-0070** (Phase-07 planner-time split): cited; this SPEC is the second sub-phase under that split's parent SPEC.
- **ADR-0072** (`*HTTPRegistry` threaded constructor map; freeze-after-boot): cited; this SPEC's `*ListenerFilterRegistry` mirrors the same discipline.
- **DECISIONS.md:** `docs/envoy-go/DECISIONS.md` — ADR-0001 (template), ADR-0003 (master FF protocol; affects how the SPEC commit's ROADMAP edit lands and is recorded), ADR-0004 (autonomous-numbering rule + autonomous-brainstorming adaptation under which THIS SPEC was authored), ADR-0008 (Envoy pin, referenced via ENVOY_TARGET.md), ADR-0010 (`dns_lookup_family: V4_ONLY` for STRICT_DNS reference clusters; cited in fixture-0008 reference bootstrap), ADR-0017 (small-mechanical-fixes do not require ADRs), ADR-0018 (fuzzer 30s short-budget policy), ADR-0028 (`--concurrency 1` reference invocation), ADR-0033 (partially superseded by 07.2), ADR-0040 (totally superseded by 07.1's ADR-0071; not affected by 07.2), ADR-0041 (silent-ignore set; extended by 07.2 per §12), ADR-0045 (planner-time-split discipline), ADR-0050 (ALPN dispatch — NOT superseded by 07.2), ADR-0051 (h2spec pin SHA), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization), ADR-0061 (Rule SN4 empirical-pin pattern this SPEC's §11 mirrors), ADR-0066 (06.2 access-log empirical-pin pattern this SPEC's §11 also mirrors), ADR-0070 (parent phase-07 split ADR), ADR-0072 (07.1 HTTP-registry threading pattern; mirrored), ADR-0076 (last extant ADR at master `424485b`; the 07.2 ADRs start at ADR-0077).
- **BOOTSTRAP_PROMPT cross-references:**
  - **§5** (Phase Lifecycle State Machine) — the lifecycle states 1 (SPEC drafting; this commit's deliverable) → 6 (REVIEW approved + phase-done) that 07.2 traverses.
  - **§5.3** (Commit message format) — the phase-done commit message format `phase 07.2: phase-done — listener-chain-completion lands; ROADMAP rows 07.2 + 07 → done [ADR-0077, ..., ADR-0083]` plus differential-surface + conformance summary.
  - **§6.2** (How to split — planner-time-split discipline) — the discipline ADR-0045 invokes for the 07.1 + 07.2 split; this SPEC honors §6.2 by being the second sibling sub-phase SPEC under the parent.
  - **§7.5** (Phase-done gate — six-gate checklist) — the gate set §3 specializes for 07.2.
  - **§4.1** (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit) — the row-flip discipline §4.4 honors per the 07.1 REVIEW I-3 corrected pattern.
- **ROADMAP.md:** `docs/envoy-go/ROADMAP.md` — rows `07`, `07.1`, `07.2` per the split landed at master `ee45aba`; row `07.2` flips `planned → in-progress` at this SPEC commit.
- **STATE.md:** `docs/envoy-go/STATE.md` — `active-phase: 07.2-listener-chain-completion`; `lifecycle-state: 1 → 2 → 3` over the upcoming sessions.
- **PROGRESS-style precedents:** `docs/envoy-go/phases/06.1-stats-prometheus/PROGRESS.md`, `docs/envoy-go/phases/06.2-access-log/PROGRESS.md`, `docs/envoy-go/phases/07.1-http-filter-framework/PROGRESS.md` — the SHA-fill convention 07.2's PROGRESS.md will mirror.
- **Phase-03 SPEC:** `docs/envoy-go/phases/03-tls/SPEC.md` — the source of the silent-skip / parse-error decisions ADR-0033 codified; cited for context on what 07.2 promotes.
- **Existing listener implementation:** `internal/listener/manager.go` (585 lines at master `424485b`) — the substantial-refactor target; key extension points cited in §4.2.
- **Envoy proto reference:** `github.com/envoyproxy/go-control-plane/envoy/config/listener/v3/listener_components.proto` — the `FilterChainMatch` proto defining the 8 + 1 dimensions and their priority order (cited in §7.2).
