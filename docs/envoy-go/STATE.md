# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `03-tls`
- **phase-directory:** `docs/envoy-go/phases/03-tls/` — does not yet exist; the next session creates it as its first file-system act (per BOOTSTRAP_PROMPT §4.1 invariant 3).
- **lifecycle-state:** `1` — Phase in ROADMAP, directory does not exist. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 1, the next session creates the phase directory and runs `superpowers:brainstorming` scoped to THIS phase, producing `SPEC.md`.
- **next-skill:** `superpowers:brainstorming`
- **next-skill-scope:** Brainstorm `docs/envoy-go/phases/03-tls/SPEC.md` for phase 03. ROADMAP row 03 summary: *"Downstream TLS termination + upstream TLS origination + SNI. TLS TCP fixture green."* (ROADMAP.md:34). The SPEC must: (i) define downstream TLS termination on the phase-02 listener manager — `filter_chains[].transport_socket` with `envoy.transport_sockets.tls` (`DownstreamTlsContext`), consuming `common_tls_context.tls_certificates[]` (cert + key) at more than a skeleton; (ii) define upstream TLS origination on the phase-02 cluster manager — `transport_socket` on the cluster with `UpstreamTlsContext`, including SNI (`sni`) and peer validation (`validation_context.trusted_ca` / `match_typed_subject_alt_names`); (iii) define filter-chain match by SNI (`filter_chain_match.server_names[]`), elevating the listener manager's match semantics beyond phase-02's single-chain happy path — an ADR must explicitly name the supersession of phase-02's simplified match rule (mirroring ADR-0021 ↔ ADR-0007 / ADR-0026 ↔ ADR-0015 precedent); (iv) enumerate phase-done gates (SPEC §3 equivalent) including at minimum a new differential fixture (e.g. `0002-tls-tcp`) that terminates downstream TLS, originates upstream TLS to a TLS-speaking backend, and verifies byte-exact proxied payload equivalence against upstream Envoy — plus a second fixture variant covering SNI-based filter-chain selection if the SPEC judges it in-scope; (v) preserve without regression all pre-existing fixture gates — phase-02's `0001-tcp-proxy-rr` round-robin-distribution assertion, phase-01's admin `/ready` byte-exact surface, and phase-00's `0000-tcp-echo` TCP echo surface (gate (b) — pre-existing fixtures green); (vi) decide the TLS stack implementation — pure `crypto/tls` vs BoringSSL-via-cgo vs a third option — with a rationale ADR locking the choice (phase-03 is the project's first non-trivial cryptographic surface, so this is the phase's most load-bearing decision); (vii) decide how Envoy's TLS parameter surface (`tls_params.tls_minimum_protocol_version`, `tls_maximum_protocol_version`, `cipher_suites`, `ecdh_curves`, `signature_algorithms`, ALPN `alpn_protocols`) maps to the chosen stack — phase-03 establishes the byte-exact equivalence rule on the wire (handshake bytes, `Server Hello`, selected cipher, ALPN echo) against upstream Envoy, so the equivalence surface must be nailed down here rather than deferred; (viii) resolve phase-03 deferred decisions at SPEC time: whether `session_tickets_disabled` / session resumption is gated in phase 03 or deferred; whether OCSP stapling (`ocsp_staple_policy`) is gated in phase 03 or deferred; whether mTLS (`require_client_certificate` + `validation_context`) is gated in phase 03 or deferred to a TLS-family sub-phase. Depends-on: phase 02 (done). Phase-02 SPEC §9's explicit deferral *"Upstream TLS → phase 03"* (SPEC.md:442) is the direct hand-off; phase-02 §2 deferrals still further deferred (HTTP/1.1 CM, HTTP/2, observability, filter chain framework, admin/drain beyond `/ready`, dynamic xDS) remain out of scope. The eight Minor REVIEW findings from phase-02 (ADR-0028 SPEC-anticipation gap, ADR-0028 bundling, ADR physical-order drift, unused `ctx` in `Filter.Handle`, unbounded goroutine in `readyListenerAddrs`, `""` sentinel in `FixtureDriver.Drive`, prose-rather-than-data `expectations.yaml`, missing BEHAVIOR_CONTRACT→ADR-0028 cross-link) are carried as candidate hygiene items for phase 03's state-1 brainstorming to triage into the SPEC, absorb into a doctrine sweep, or re-defer explicitly.
- **last-commit:** 49b8893
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
