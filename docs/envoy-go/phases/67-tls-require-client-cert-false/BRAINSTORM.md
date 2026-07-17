# Phase 67 Brainstorm — `tls-require-client-cert-false` (the FIFTH xDS-family row; verify-if-presented mTLS — honor the client-CA trust anchor when `DownstreamTlsContext.require_client_certificate` is false/absent, across ALL THREE validation-source shapes (fully-inline / SDS-VC / CVC) at the TCP apply point: `ClientAuth = VerifyClientCertIfGiven`, the anchor fetch UN-GATED from the require block, and the phase-66 E3 reject RETIRED — the direct pickup of the item SPEC-66 §3.10 probed, pinned, and scoped out with the instruction that lifting it "belongs in a row that fixes all three together"; anticipated +0 packages / +0 modules / +0 stats / +0 BackendKinds; anticipated ONE new fixture `0110`)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only — ZERO production `.go`. Fresh worktree off master `0d4d4041`, branch `phase-67-tls-require-client-cert-false-brainstorm`, worktree `.worktrees/phase-67-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** the termination sentinel was re-evaluated MECHANICALLY in this worktree (tip `0d4d4041`), running next-prompt.txt's three checks verbatim. ACTUAL output this session: check (1) printed **NOTHING** (every chartered row is `done` — row 66 flipped at its IMPL close; this is the state the router warns about: checks (2)+(3) ALONE hold the loop open, and they DO); check (2) printed **THREE** live "candidates:" sentences (HTTP/3 `:176`, xDS `:186`, Observability `:196`; `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` ⇒ `3` — the command, not the adjective, per `reference_sentinel_deferred_sentence_live_vs_historical`); check (3) printed `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`. No `stop` file exists (`ls /home/esa/git/envoy-go/stop` → no such file). The sentinel does NOT fire. There is NO banked mid-lifecycle work (check (1) silent ⇒ no split leg awaiting a PLAN/IMPL), so the roller SELF-PICKS per the 2026-07-12 standing directive (§2.1).
>
> **Baselines re-verified against the worktree tip `0d4d4041` this session (commands re-run MECHANICALLY; NOT copied from the router):** fixtures **111** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0109-xds-sds-combined-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`; scope `internal/` only) · DECISIONS tail **ADR-0288** (`grep -oE '^## ADR-[0-9]+' … | tail -1`; next-free **ADR-0289**) · BackendKind tail **38** (`H2GoawayResponder BackendKind = 38`, `test/differential/fixture/fixture.go:614`) · stat surface **1201** (docs-verified; no mechanical count exists) · go.mod modules **2** (the phase-61.2 lineage figure — NOT a repo total; the single `go.mod` requires 67 modules, re-counted).
>
> **Evidence discipline in this document.** THREE inputs were produced by parallel agents this session, each in a PRIVATE scratch (`reference_parallel_subagents_private_scratch`): **(A)** a mechanically re-derived code map of every `require_client_certificate` consumer (repo-wide greps + reflection rosters + declaration re-derivation, `[RUN]`-marked); **(B)** LIVE probes — **21 fresh-container reference arms** against `envoyproxy/envoy:v1.37.2` (digest-pinned; bridge network + `host.docker.internal:host-gateway` per the harness's ExtraHosts mechanism; FRESH driver-owned SDS server per arm with an ARM-UNIQUE secret name and a served-this-arm assert in every `.sds.log`), plus SDS fetch-gating arms (silent SDS server, `/ready` + container-netns socket + stats observed at t≈3s and t≈19s), plus a Go `crypto/tls` cross-product (a throwaway client/server harness driving `VerifyClientCertIfGiven`/`RequestClientCert`/`RequireAndVerifyClientCert` × real/nil pools × polite/forced clients, with openssl alert-byte pins); **(C)** a placement/sizing/process dossier (candidate comparison, deferred-sentence roster, ADR anchors, slug collision checks). Claims below are attributed EXECUTED (probe/`[RUN]`) vs RE-DERIVED (declaration reading) throughout — a reading is never presented as a probe.

---

## 1. Mission and scope confirmation (67 — an APPLY-POINT RESHAPE on landed appliers, NOT a new resource, seam, or discovery machine)

### 1.1 What phase 67 delivers as a self-contained whole

Today, `NewDownstreamConfig` (`internal/tls/config.go:41`) builds the client-CA trust anchor — for ALL THREE validation shapes — **inside** the `if ctx.GetRequireClientCertificate().GetValue()` block (config.go:87), and sets the repo's ONLY production `ClientAuth` assignment (`RequireAndVerifyClientCert`, config.go:188 — `[RUN]` repo grep over `internal/`+`cmd/`+`validate/` minus tests) at its tail. At `require=false/absent`: the inline anchor is silently ignored (the trusted_ca file is never even opened), the SDS-VC anchor performs NO fetch, and CVC boot-FAILs on the phase-66 E3 reject (config.go:66-69). The reference instead implements **verify-if-presented**: the anchor is honored, a presented-but-untrusted cert is REJECTED, and only the no-cert cell differs from `require=true`.

The row:
1. **Hoists the three anchor arms** (SDS-VC config.go:89-120 · CVC :121-176 · inline :177-187) **out of the require gate**, keyed on anchor presence — the SDS fetch fires at boot regardless of `require_client_certificate` (matching the reference's un-gated fetch, §2.4).
2. **Maps `ClientAuth` three ways**: require=true → `RequireAndVerifyClientCert` (unchanged); require=false/absent + anchor → **`VerifyClientCertIfGiven`**; no anchor → `NoClientCert` (unchanged).
3. **Retires E3** (config.go:66-69) atomically with the lift — the reject whose sole enforcement was this row's gap (§2.6).
4. Proves it at the differential level with fixture **`0110-tls-require-client-cert-false`** (three-arm cross-side verdict, §2.12).

### 1.2 What phase 67 does NOT deliver (forward to §8)

Not QUIC client-auth (`NewQUICDownstreamConfig` never reads the field AT ALL — provisionally OUT, final call at the SPEC, with the boundary line extended, §2.7/D-RCCF-QUIC). Not the cert-provider oneof arms (fields 10/12 — UNREAD in production; they remain outside the anchor switch, §2.8). Not the `ssl.*` stat family (framework surgery, ADR-0286 C3 — the row does not depend on it, §5). Not SDS rotation, not the empty-dynamic Design B fallback, not `crl`. Not a `require=false` RBAC/principal fixture arm (newly deferred, §2.11/§8).

### 1.3 Phase-done as the FIFTH xDS-family row (family STAYS OPEN)

Row 67 registers `in-progress` at the stage-close commit (controller work following the adversarial pass — a separate commit in this same worktree before squash; the BRAINSTORM commit itself touches only this file) and flips `done` at its own IMPL six-gate (a SINGLE FLAT ROW, the sole leg — ADR-0106). Family placement per the §2.1 dossier: there is NO TLS/listener family (phase 03 `tls` is an MVP-trunk row); the item was raised by phase-65 §8, carried and probed by phase 66, its apply-point is where rows 65/66 landed, and two of its three shapes are SDS shapes. A `tls-*` slug inside the xDS family is precedented (Observability rows carry `tracing-*`/`stats-sink-*` slugs — slug prefix ≠ family name). The counter-consideration — the fully-inline shape has zero xDS involvement — is recorded and dismissed: the only alternative is opening a new family for one row, maximally non-smallest. **The row summary carries the literal phrase "the FIFTH xDS-family row" (sentinel check-3 phrasing).**

**No deferred sentence names this item** (`[RUN]` grep: `verify.?if.?present|IfGiven|require_client_certificate` hits ROADMAP lines 55/127/128/188/190 — none of the three sentence lines 176/186/196). It is a **§8-tier pickup** from SPEC-66 §13 / ADR-0287 §Consequences — the phase-64/66 precedent: **no sentence is narrowed at ANY stage, and the SPEC/IMPL must not fabricate a narrow** (§9).

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW *(self-answered; SPEC confirms D-RCCF-SPLIT)*

SPEC-66 §13 pre-sized this row ("needs `VerifyClientCertIfGiven` + a fetch-gate restructure"), and SPEC-66 §3.10(iii) sized its shape ("an apply-point reshape, not a branch") — two sections, cited split (as §2.1 item 1 does). Anticipated ~8-12 tasks concentrated in ONE function (`NewDownstreamConfig`) of ONE production file plus tests and one fixture — materially smaller than phase 65 (11 tasks, four new symbols) and comparable to phase 66 (9 tasks). Escape-valve armable, no split anticipated: there is no two-package surface to strand a leg (`internal/xds` and `internal/boot` are untouched, §3).

### 1.5 Package placement — ALL edits in ONE existing production file; ZERO new packages

`internal/tls/config.go` is the SOLE production edit surface (§6). `internal/boot/boot.go` needs **no edit** — the pre-scan is already require-agnostic (`[RUN]`-verified, §4). `internal/xds`, `internal/listener`, `validate/`, `test/helpers/sdsserver`: all UNTOUCHED.

### 1.6 Relationship to the existing seams

Phase 60.2 built the fetch; 65 proved the pool applier; 66 proved composition. **Phase 67 changes WHEN the landed appliers run and WHAT `ClientAuth` they conclude — it adds no applier, no seam, no wire shape.** The management server cannot tell a require=false client from a require=true client (same `tls.v3.Secret`, same type URL — the fetch-gating probe confirmed the reference's request stream is identical in both, §2.4).

---

## 2. Design decisions (D-RCCF-* — mnemonic per the D-COMBVC-* precedent; "RCCF" = Require-Client-Cert-False; collision-free: `[RUN]` repo grep `D-RCCF` → 0 hits)

### 2.1 Row + subject confirmation: `tls-require-client-cert-false` *(SELF-PICKED per the standing directive → row 67 registers at the stage-close commit)*

**Why verify-if-presented is the defensible pick — the ONLY candidate simultaneously (i) pre-sized SMALL in a landed doc, (ii) fully substrate-reusing, and (iii) already reference-PINNED:**
1. **Pre-sized by SPEC-66 §13** ("Must be lifted across all three paths … in one row; needs `VerifyClientCertIfGiven` + a fetch-gate restructure") and §3.10 ("an apply-point reshape, not a branch"). One function + one reject retirement + one fixture.
2. **Every substrate piece is landed and proven**: the 60.2 SotW stream, the 65/66 pool appliers, the `0108`/`0109` fixture pattern (per-side driver-owned `sdsserver`, in-memory PKI, `structuralCheck`, `normalizeTLSErr`, served-this-arm assert). Nothing is built; a gate is moved.
3. **The reference behavior was ALREADY probed and pinned at SPEC-66 §3.10** — and this session RE-probed it fresh across the full 21-arm cross-product (§2.3), so the SPEC starts from pinned evidence on all three shapes, not one.
4. **ADR-0287 §Consequences charters the scope**: "Lifting `require == false` belongs in a row that fixes ALL THREE paths (CVC, plain SDS-VC, fully-inline) together" — this row IS that row, by the landed doc's own instruction.

**Rejected alternatives** (each sized against SPEC-66 §13 / ADR-0287 / the three deferred sentences this session; deferred-sentence membership is NOT a discriminator — the phase-66 lesson):

- **The empty-dynamic fallback (SPEC-66 §3.9(a)) — the RUNNER-UP.** Pre-sized as "a real row" but requires **Design B**: a message-returning `internal/xds` seam plus relocating `parseValidationSecret`'s rejects — a landed-seam reshape with regression exposure to `0108`/`0109`. Real, probe-ready, larger blast radius. **Deferred.**
- **`xds-sds-upstream-server-cert`** — the VALUE-level constructibility cycle (mechanism recorded in SPEC-66 §13 / BRAINSTORM-66 §2.1.1 — do not re-derive); needs a boot-model reshape. The SAME cycle blocks its sibling half, **upstream SDS `validation_context`** — named here so the menu is complete; neither upstream half is reachable without that reshape. **Deferred.**
- **SDS rotation** — needs a live-update path into a built `tls.Config` AND the §3.11 implicit-watch scoping AND a `watched_directory` positive control that SPEC-66 §13 records as UNESTABLISHED ("A future rotation row must build one first"). **Deferred.**
- **SDS `initial_fetch_timeout`/backoff edges** — SPEC-66 §13: the reference side is "a proto reading, not a probe"; probe debt first. Backoff needs a retry loop envoy-go's initial-fetch-only SDS lacks. **Deferred.**
- **`crl`** — never sized standalone; reference UNPROBED; ADR-0287's all-paths-asymmetry rule imposes a three-path tax WITHOUT a pinned probe to start from. **Deferred.**
- **The compose-two edge** — behind the `seen>1` pre-scan deferral; needs the single-slot provider model generalized. **Deferred.**
- **CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc`** — each needs appliers plus a dynamic-update model the repo lacks (bootstrap rejects `dynamic_resources`, ADR-0280 §Context). LARGE each. **Deferred.**
- **HTTP/3-family candidates** (upstream H3 cluster / alt-svc / 0-RTT / h3spec / QuicProtocolOptions / **full QUIC transport-socket options** (live sentence, ROADMAP:176) / QUIC robustness) — all need fresh probes; `HTTPExpectations` is TCP-only so H3 assertion surfaces are bespoke; QuicProtocolOptions carries the bimodal-cost analysis of BRAINSTORM-66 §2.1. Medium-to-LARGE. **Deferred.**
- **Observability-family candidates** — OTLP-metrics sink (a whole new sink); the `ssl` stat family (**framework surgery**, ADR-0286 C3 — and NOT a dependency of this row, §5); `custom_tags` `metadata` (~19 call-site threading); `spawn_upstream_span`/`http_service`/force-trace (larger). **Deferred.**
- **DataSource `environment_variable`** (SPEC-66 §13; carried at §8) — blocked on the D-ENV-HARNESS env-injection seam SPEC-63 declined to build; a harness seam first, not a TLS row. **Deferred.**
- **Family openers (gRPC / Runtime / WASM)** — defensible only if the family's smallest row beats every candidate above; assessment: none does (each is a subsystem bring-up). Recorded and rejected. **Deferred.**

*(Menu-completeness note, added by the adversarial pass: three dispositions above — full QUIC transport-socket options, DataSource `environment_variable`, and the upstream SDS validation-context half — were missing from the originally recorded comparison. None plausibly beats the pick; the pick survives unchanged.)*

### 2.2 Scope: the "three paths" are one axis of a THREE-AXIS cross-product — the row's roster stated precisely *(D-RCCF-ROSTER)*

Re-derived mechanically (input A), the `require_client_certificate` surface is a cross-product, and SPEC-66 §3.10's "three paths" names only the validation-shape axis:

- **Axis 1 — apply points**: A1 `filter_chains[i].transport_socket` (manager.go:472, TCP) and A2 `default_filter_chain` (manager.go:554, TCP) both call `NewDownstreamConfig` — **IN scope**. A3 QUIC (manager.go:466 → `NewQUICDownstreamConfig`, no provider parameter) **never reads the field** — disposed at §2.7.
- **Axis 2 — entry modes**: M1 normal boot (provider non-nil iff pre-scan `seen==1`); M2 `--mode validate` (`validate/validate.go:49` threads a literal nil — the package contains no TLS logic of its own); M3 the exported test-only ctors (nil at manager.go:242/:257). SDS shapes keep rejecting under nil providers (config.go:381/:398) **regardless of require** — no edit, but stated (D-RCCF-VALIDATE, §2.10).
- **Axis 3 — validation shapes**: the oneof has FIVE arms by reflection (`[RUN]`, input A §4): inline (3), SDS-VC (7), CVC (8) — the row's three — plus the two cert-provider arms (10/12), UNREAD in production (§2.8).

**The row's scope: axes A1/A2 × all provider modes' existing gates × shapes 3/7/8.** Everything else on the cross-product is a named boundary, not a silence.

### 2.3 D-RCCF-CLIENTAUTH — the core semantics, PINNED on both sides this session *(EXECUTED evidence)*

**Reference (probe B, 21 fresh-container arms, `envoyproxy/envoy:v1.37.2`):** the full cross-product {inline, sdsvc, cvc} × {false, ABSENT} × {trusted, untrusted, none} plus a 3-arm `require=true` CVC control:

| shape × require | trusted (anchor-signed) | untrusted (other CA) | none |
|---|---|---|---|
| all three × false | ACCEPT, HTTP 200, `ssl.handshake:2` | REJECT, **alert 48 unknown_ca**, `ssl.fail_verify_error:2` | ACCEPT, HTTP 200, **`ssl.no_certificate:2`** |
| all three × ABSENT | identical | identical | identical |
| cvc × **true** (control) | ACCEPT | REJECT, alert 48 | REJECT, **alert 116 certificate_required**, `ssl.fail_verify_no_cert:2` |

*(Stat-value arm design: these counters are per-connection and each probe arm drove TWO connections, hence `:2` — a one-connection arm reads `:1`. The pinned fact per cell is WHICH counter increments, not its absolute value.)*

**PINNED:** (1) verify-if-presented is live on ALL THREE shapes; (2) **ABSENT ≡ explicit false** — all 9 absent arms outcome- and stat-identical to their false twins; (3) the require=true control reproduces SPEC-66 §3.10's table fresh, and **the ONLY cell that moves when the field flips is `none`** (116+`fail_verify_no_cert` ⇄ ACCEPT+`no_certificate`); (4) the discriminating reference stat roster is `ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert` / `ssl.no_certificate` — reference-side ONLY (envoy-go emits ZERO `ssl.*` stats, §5).

**Go side (probe B, crypto/tls harness + openssl alert pins):** `VerifyClientCertIfGiven` + a real pool matches the reference **cell-for-cell with byte-identical wire alerts** — untrusted-forced → server `*tls.CertificateVerificationError`, openssl sees **SSL alert number 48**; `RequireAndVerifyClientCert` + no cert → **alert 116**. A `RequestClientCert` control ACCEPTED the untrusted cert (peerCerts=1), proving the IfGiven reject is the verify step, not the request step. **Client-observed failure STRINGS differ per side** (OpenSSL `error:0A000418…` vs Go `remote error: tls: unknown certificate authority`) ⇒ fixture failure pins stay **PER-SIDE**, established practice (`reference_differential_reference_parses_full_message`); the wire alert bytes agree, so no wire divergence exists to explain away (`reference_wire_format_both_sides_see_same_bytes` — satisfied, not invoked as a deviation).

### 2.4 D-RCCF-FETCHGATE — the reference does NOT gate the SDS fetch on require; envoy-go's un-gated fetch is a NEW ADR-0280 departure instance *(EXECUTED evidence; the SPEC must NAME it)*

**Probe B fetch-gating arms (silent SDS server = accepts the stream, never sends; served-this-arm assert = request logged, no nonce-ACK):** with `require=false`, the reference **fires the validation-context fetch at boot** (request logged in every arm), the init manager holds workers un-started (`/ready` = `INITIALIZING`; **the listener port is UNBOUND in the container netns** — `/proc/net/tcp` verified; the TCP accept a host client sees is docker-proxy, then reset ⇒ curl exit 35 — **a fixture trap: never treat a docker-proxy accept as listener liveness**), and after the default 15s `initial_fetch_timeout` the listener binds and **FAILS CLOSED**: every connection — trusted AND no-cert — is destroyed pre-handshake with NO alert (full stat path: `listener.<addr>.server_ssl_socket_factory.downstream_context_secrets_not_ready`, read `:2`; every `ssl.*` counter stays 0). A `require=true` control is **identical** — the field flips NOTHING in fetch behavior. **Under CVC, the inline default CA does NOT rescue an undelivered dynamic half** — consistent with phase-66 Design A (SDS REPLACES; no fallback-before-delivery). Also pinned: `total_listeners_active` reads 1 and `/listeners` lists the listener THROUGHOUT warming — do not assert on them.

**Consequence for envoy-go:** un-gating the fetch means a `require=false` + SDS-shape listener that boots green today (no fetch, no anchor) becomes **boot-FAIL-capable on fetch failure** — the ADR-0280 posture (boot-FAIL where the reference degrades to not-ready fail-closed). This is a **NEW INSTANCE of the ADR-0280 departure family** — reference: hold-workers-then-fail-closed-per-connection; envoy-go: boot-FAIL — behaviorally compatible at the traffic level (neither side EVER serves unverified traffic while the anchor is missing), divergent on lifecycle. **The SPEC must name it as such**, exactly as ADR-0287 named its parse-error sibling.

**⚠️ Probe-vs-landed-docs DRIFT (found by the adversarial pass; a named obligation, NOT a silent rewrite).** Three landed sites record the reference as serve-anyway on validation-context fetch failure: BEHAVIOR_CONTRACT.md:900 ("starts and serves with an unpopulated trust store"), ADR-0286 D-SDSVC-FETCHTIMEOUT ("the reference serves anyway (probed at SPEC-60 §11 arm B)" — an EXTRAPOLATION from the SERVER-CERT probe), and the landed comment at `internal/tls/config.go:116` ("the reference's serve-anyway"). This session's probe pinned the OPPOSITE for validation-context resources: init-hold (workers un-started, port unbound) then fail-closed close-no-alert, identical at require=true/false, no inline-default-CA rescue under CVC. The SPEC reconciles the per-resource-type reference posture BEFORE drafting ADR-0289 §Context — whether SPEC-60's arm-B extrapolation was wrong or the postures genuinely differ per resource type — ideally by a discriminating server-cert-fetch-failure probe (D-RCCF-FETCHFAIL-POSTURE, §6 items 9/14 + §10).

### 2.5 D-RCCF-NILPOOL-GUARD — `VerifyClientCertIfGiven` + nil `ClientCAs` fails in the WORST direction; the construction order must make the state unrepresentable *(EXECUTED evidence)*

Probe B hazard arms H1/H2: `VerifyClientCertIfGiven` with a **nil** pool does NOT mean "accept everything" — Go's x509 verification falls back to the **SYSTEM root pool**, so the **legitimately anchor-signed client is REJECTED (alert 48)** while the no-cert client is ACCEPTED. A bug that sets `ClientAuth` before the pool is installed (or on a failed fetch) breaks trusted clients and admits anonymous ones — and does NOT mirror the reference's fail-closed not-ready state (Table-4 divergence-by-construction: Go has no not-ready socket state). **Decision: the restructure must set `ClientAuth = VerifyClientCertIfGiven` ONLY in the same control-flow arm that has already installed a non-nil pool** (assignment-adjacency, mirroring how :188 sits today relative to the anchor arms), and the SPEC pins a test that the nil-pool+IfGiven state is unconstructible from any config. This is the row's analogue of phase-66's E3 rationale: the lift must never yield a listener whose auth posture silently differs from its config.

### 2.6 D-RCCF-E3-RETIRE — the phase-66 E3 reject retires ATOMICALLY with the lift *(re-derived; reject-roster discipline)*

E3 (config.go:66-69, `combined_validation_context requires require_client_certificate: true in phase 03`) exists PRECISELY to prevent the envelope-lift from yielding a silently unauthenticated listener at require=false (ADR-0287 §Decision, confirmed by deliberate break twice in phase 66). This row implements the semantics E3 stood in for, so **E3's substring retires in the same task that lands the lift** — a lift+guard atomicity mirror of `reference_lifted_reject_hidden_enforcement`: re-derived from code (input A), E3's ONLY enforcement is the require=false CVC shape; nothing else hides behind it. **All RETAINED rejects stay byte-identical** — the nil-provider gates (config.go:381/:398), the CVC presence checks (:402-407), the four sub-field rejects (:417-436), and the require=true inline anchor-less reject (:179-181) — per the reject-roster discipline SPEC-65/66 attribute to "ADR-0080-distinct substrings". **Citation-drift flag (input C, follow-the-usage):** the literal ADR-0080 body is `default_filter_chain` semantics and contains no reject-substring rule, yet two landed SPECs cite it as that discipline. This row follows the established usage and flags the drift; fixing the attribution is a docs-hygiene item OUT of this row (§8).

### 2.7 D-RCCF-QUIC — the QUIC sibling: recommend OUT of scope, with the boundary line EXTENDED to name it *(provisional; final call at the SPEC, §10)*

`[RUN]` (input A): `grep -rn "GetRequireClientCertificate" --include='*.go'` repo-wide minus `.worktrees/` → **exactly 3 hits** — config.go:67 (E3), config.go:87 (the gate), fuzz_test.go:214 (a comment). `NewQUICDownstreamConfig` (config.go:200-223) **NEVER reads the field** — client-auth is silently ignored at true AND false, and inline `trusted_ca` is silently dropped even at require=true (no QUIC ClientCAs path exists). SPEC-66's boundary wording (verbatim, SPEC-66:69: "Not QUIC CVC (nil provider ⇒ the reject stays, ADR-0080)") does **not** cover this gap — it is about the CVC envelope, not client-auth. ADR-0287 §Consequences records it as "DISCOVERED, NOT FIXED". **Provisional decision: QUIC client-auth stays OUT of this row.** Scoping it IN would drag: a ClientCAs/ClientAuth implementation against quic-go's consumption of the same `*stdtls.Config` (quic.go:37), AND — for SDS anchors — pre-scan surgery, because `boot.go:131` matches only the plain `DownstreamTlsContext` type-URL and **skips QUIC sockets** (`[RUN]`-verified), a provider-plumbing change with its own blast radius. The row instead **EXTENDS the BEHAVIOR_CONTRACT boundary line to name the QUIC client-auth gap explicitly** (today it is named only for the CVC envelope; :936(b) stays with sharpened wording). The SPEC makes the final in/out call (§10) — with a reference probe of QUIC client-cert behavior ONLY if it scopes IN.

### 2.8 D-RCCF-CERTPROVIDER — the two cert-provider oneof arms: unchanged silence, now STATED per-arm *(re-derived)*

Fields 10/12 (`validation_context_certificate_provider` / `..._instance`) are UNREAD in production (`[RUN]` getter sweep → 0 hits). TODAY at require=false they silently boot with no anchor; at require=true they hit the misleading-but-loud `requires validation_context.trusted_ca` reject (config.go:180) because the oneof nils `GetValidationContext()`. **POST-LIFT, per arm: they remain outside the anchor-arm switch ⇒ no pool is built ⇒ `ClientAuth` stays `NoClientCert` — at require=false the lift must NOT change their behavior silently, and does not: same no-anchor boot, now stated.** At require=true their misleading reject is unchanged. Both facts land in the BEHAVIOR_CONTRACT as named per-arm boundaries rather than the current undifferentiated silence.

### 2.9 D-RCCF-ABSENT — absent ≡ false, on BOTH sides, by two INDEPENDENT mechanisms; no tri-state exists *(EXECUTED both sides)*

Go side (`[RUN]`, input A): the field is `*wrapperspb.BoolValue`; a nil getter returns `false`, so absent and explicit-false are indistinguishable via `GetValue()` (distinguishable only by pointer nilness — which no code inspects). Reference side (probe B): all 9 ABSENT arms are outcome- and stat-identical to their false twins. **No divergence is possible and no tri-state handling is owed.** Fixture note (`reference_protojson_wrapper_scalar_not_object`): the wrapper takes a BARE scalar in YAML/JSON — `require_client_certificate: false` directly; `{value: false}` would ERROR.

### 2.10 D-RCCF-DRAGGED — the semantic changes the restructure DRAGS IN, each needing a SPEC decision *(re-derived; §10)*

Un-gating the anchor arms changes behavior for configs that never reached them before:
1. **require=false + corrupt inline `trusted_ca`** → becomes a boot error (the file was previously never opened). Reference-parity plausible but the SPEC states it as a decision, not a side effect.
2. **require=false + SDS fetch failure** → boot-FAIL (the §2.4 departure instance).
3. **require=true + anchor-less inline VC** → the :179-181 reject STAYS (unchanged).
4. **require=false + anchor-less inline VC** (a `validation_context` present but no `trusted_ca`, require absent/false) → **UNPROBED on the reference this session** — the one cell the 21-arm matrix did not cover. The SPEC must probe it before choosing reject vs NoClientCert-boot (§10).
5. **D-RCCF-VALIDATE** — M2/M3 nil-provider modes: SDS shapes keep their byte-identical phase-03 rejects regardless of require (config.go:381/:398). No edit; the `validate`-path divergence recorded at ADR-0287 (§Consequences) is unchanged in kind.
6. **D-RCCF-P-PREMISES** — phase-66's equivalence-theorem premises P1-P5 must be RE-DERIVED under the moved gate. Anticipation: P3 is fetch-dependent, not require-dependent, so the theorem survives; P5 (the baseDir/DataSource coincidence) is a STANDING HAZARD this row re-touches by editing `NewDownstreamConfig` — the mandatory call-site comment must survive the restructure intact.

### 2.11 The no-cert-accepted consequence downstream of the handshake *(re-derived `[RUN]`; one new deferred item)*

Verify-if-presented admits connections with `len(PeerCertificates)==0` into surfaces that previously (under mTLS-on) always saw a cert. `[RUN]` repo grep (input A): **every** production deref of `PeerCertificates[0]` sits behind a length guard (manager.go:1321; hcm/connection.go:40/:443; hcm/filter.go:185; hcm/h3dispatch.go:199; wasm/abi_callbacks.go:1299/:1382/:1413; lua/ssl.go:163/:428 — `filter/http/chain.go` mentions the slice only in doc comments, no deref) — **no crash hazard**. Principal-dependent behavior (RBAC principals, Lua/WASM cert introspection) sees empty principals — defined behavior, but the interaction is not newly EXERCISED by this row: `0110` is `tcp_proxy` (the 0108/0109 pattern), so HTTP-principal surfaces are outside the fixture's scope, and the one existing mTLS-principal fixture (`0018-http-rbac`) runs `require=true` (`[RUN]`-verified, envoy-go.yaml:168). **A `require=false`+RBAC arm is a NEW deferred item (§8)** — this row makes the state reachable and guards it; proving the principal-empty semantics cross-side is its own small row.

### 2.12 D-RCCF-FIXTURE — `0110-tls-require-client-cert-false`: a three-arm cross-side verdict on the 0109 chassis *(direction; SPEC pins)*

- **Shape**: `tcp_proxy` echo (0108/0109 pattern), driver-owned **per-side** `sdsserver` (fresh per side, ARM-UNIQUE secret names, the 0109 **served-this-arm precondition assert**, `normalizeTLSErr`, `structuralCheck`), in-memory PKI. `BackendCount` ≥1 (`reference_differential_backendcount_min_one`); **one fixture dir = ONE runner branch** (`reference_differential_fixture_dispatch_constraint`).
- **The verdict**: the 0109 mechanism widened to THREE arms at require=false — `trusted→ok+echo · untrusted→rejected · no-cert→ok+echo` — compared cross-side via `CompareBytes` with per-side failure pins. The no-cert→ok arm is the row's discriminator (it flips vs require=true); the untrusted→rejected arm proves the anchor is LIVE (not a vacuous accept-all).
- **⚠️ The driver trap, caught by a DISCRIMINATING control (probe B):** a Go client handed the untrusted cert via `tls.Config.Certificates` **SILENTLY WITHHOLDS it** when its issuer is absent from the server-advertised acceptable-CA list — turning the untrusted arm into a second none arm and shipping a **vacuous green**. **`0110`'s driver MUST force-send via `GetClientCertificate`** (the probe's "forced" mode; openssl-s_client-like). The SPEC verifies whether `0108`/`0109`'s driver already does this or relied on same-CA politeness.
- **Which shape(s) the fixture covers**: recommend **one SDS shape (SDS-VC or CVC) as the fixture's primary** — it additionally proves the un-gated fetch LIVE at require=false (the SDS request observably fires, the served-this-arm assert witnesses it) — with the inline and remaining shapes covered by unit tests + the existing require=true fixtures. Exact split is a SPEC decision (§10); a three-fixture spread would triple the differential cost for one mechanism.
- **Do NOT assert** `/listeners` or `total_listeners_active` for warming, and never treat a docker-proxy accept as liveness (§2.4).

### 2.13 D-RCCF-STATS / D-RCCF-FUZZSEED — +0 stats; fuzz seeds with the count decided at the SPEC

Stats: **+0** (§5). Fuzz: the E3 seed at fuzz_test.go:220 (require=false CVC, currently expecting the E3 reject) flips to expect success — plus NEW require=false seeds for all three shapes across the existing dispatch sides. Anticipation: SEEDS ONLY, count stays **55**; but phase-66 T5 proved a seed is a probe (a hardcoded nil provider upstream of a seed's gate makes it vacuous) — the SPEC re-derives the fuzz harness's provider dispatch before committing to +0 (§10).

---

## 3. Framework-survey result — an apply-point reshape; ZERO new packages/modules/symbols

### 3.1 The change surface
One production function (`NewDownstreamConfig`) restructured; one reject retired; `ClientAuth` mapping widened. No new seam, applier, resource type, or wire shape.

### 3.2 NEW packages: NONE. go.mod modules: NONE ADDED (lineage figure stays 2; the `crypto/tls` `VerifyClientCertIfGiven` constant is stdlib). Re-check `git diff go.mod` after tidy anyway (`reference_new_subpackage_pulls_transitive_module`).

### 3.3 REUSES
- `provider.FetchInitialValidationContext` / `xds.ParseSDSConfig` / the SotW stream — **verbatim**, relocated call sites only.
- `loadTrustedCAPool` (phase 03) — verbatim.
- `sdsserver` + the 0109 driver chassis (PKI, per-side servers, served-this-arm assert, `normalizeTLSErr`, `structuralCheck`) — the 0110 driver is a disciplined clone with a third arm.
- The phase-66 P1-P5 theorem — re-derived, not rebuilt (§2.10).

### 3.4 The 60.2 cycle guard STANDS trivially
`internal/xds` is untouched; `go list -deps ./internal/xds` (no `...`) must still show `internal/stats` + `internal/xds` only — re-verified at the IMPL as usual. TYPE-level only, per the memory's own caveat.

---

## 4. Bootstrap-level applicability — a PER-LISTENER downstream TLS sub-field; the pre-scan is ALREADY require-agnostic

`require_client_certificate` lives on `DownstreamTlsContext` — per-listener, per-filter-chain; not a bootstrap knob. **`boot.NewSDSProvider` needs NO edit**: `[RUN]`-verified (input A), the pre-scan counts cert-SDS (boot.go:139-142), SDS-VC (:152-155), and the CVC SDS half (:174-179) **without reading `require_client_certificate`**, so the provider already exists at require=false for both SDS shapes — the fetch-gate move is entirely inside `internal/tls`. (Two boot-time precisions already true today at require=false+SDS-VC: the provider constructs — registering `sds.*` stats and creating the LAZY gRPC client, no network I/O — and ParseSDSConfig's rejects fire synchronously. The lift changes the fetch, not the scan.) The pre-scan's QUIC blindness (boot.go:131) is load-bearing for §2.7's sizing.

---

## 5. Stat surface hypothesis — +0 (67); the ssl-stat boundary and why this row does NOT depend on it

**+0. Surface stays 1201.** The reference discriminates all four outcome classes via `ssl.handshake`/`ssl.fail_verify_error`/`ssl.fail_verify_no_cert`/`ssl.no_certificate` (probe B) — but **envoy-go emits ZERO `ssl.*` stats** (ADR-0286 §Consequences C3: grep-confirmed twice in phase 65; the only listener-scope counter is `downstream_cx_total`). C3 also records the STRONGER replacement this row inherits: the proof obligation ("the anchor is the ACTUAL trust anchor, not a vacuous accept-all") is discharged by the **cross-side accept/reject CONTRAST**, which a subject-only stat could never prove. `0110`'s three-arm verdict (§2.12) is exactly that contrast, widened by one arm. **The ssl-stat framework-surgery row (Observability sentence, `:196`) is NOT a dependency** — honest limit, same as 0108/0109: without subject-side ssl stats the fixture cannot distinguish WHY an arm failed, only THAT the verdict contrast matches per-side pins. The `sds.*` lifecycle counters are reused verbatim. envoy-go-strict departure flags gained: the §2.4 boot-FAIL instance; retired: the E3 divergence bullet (BEHAVIOR_CONTRACT :928).

---

## 6. Edit-site enumeration — cited by SYMBOL with `file:line` as of master `0d4d4041` (SPEC re-derives; D-RCCF-DOCSHAPE)

**Production — `internal/tls/config.go` (the SOLE production file):**
1. `NewDownstreamConfig` — E3 retirement (:66-69) — atomic with the lift (§2.6). `[EDIT]`
2. `NewDownstreamConfig` — the require gate (:87): anchor arms SDS-VC (:89-120) / CVC (:121-176) / inline (:177-187) hoisted out, keyed on anchor presence; the require=true anchor-less inline reject (:179-181) retained for require=true; the require=false anchor-less-VC cell per the §10 probe. `[EDIT — the row's core]`
3. `NewDownstreamConfig` — `ClientAuth` mapping (:188 → three-way, §2.3) with the D-RCCF-NILPOOL-GUARD construction order (§2.5) and the P5 comment preserved (§2.10). `[EDIT]`

**Production — everything else:** `internal/boot/boot.go` `[NO EDIT — pre-scan require-agnostic, §4]` · `internal/xds` `[UNTOUCHED]` · `internal/listener` (incl. quic.go) `[UNTOUCHED under the §2.7 recommendation]` · `validate/` `[UNTOUCHED]`.

**Test / harness:**
4. `internal/tls/config_test.go` — the ":1017 `RequireClientCertificate deliberately absent`" region flips from ignore-expectations to verify-if-presented expectations; new three-shape × {false, absent} × {anchor} unit tests; the nil-pool-unconstructible test (§2.5); E3-expectation tests (`TestCVC_RequireFalse_Rejected_E3` et al.) retire/flip. `[EDIT/ADD]`
5. `internal/tls/fuzz_test.go` — the :220-region require=false CVC E3 seed flips; new require=false seeds across shapes; the harness's provider dispatch re-derived first (§2.13). `[EDIT]`
6. `internal/listener/manager_test.go:647` — a require=true **ANCHORLESS-INLINE** construction (`mkDownstreamTSRequireClientCert`: require=true + inline `tls_certificates`, NO validation context — not a CVC shape); its consumer `TestNewManager_MultiChain_RequireClientCert_Errors` asserts the error contains `require_client_certificate`, today satisfied by the :179-181 anchorless-inline reject. Expectation unchanged — that reject is RETAINED (§2.6) — but the site is IN the blast radius and is re-verified. `[VERIFY/EDIT]`
7. Fixtures `0018-http-rbac` / `0108` / `0109` — **expectations UNCHANGED**: all three run `require_client_certificate: true` (`[RUN]`-verified this session: 0018 envoy-go.yaml:168; 0108 envoy.yaml:66/envoy-go.yaml:64; 0109 envoy.yaml:69 — and 0109's own comment records require=true as MANDATORY under phase-66 scope, a comment this row's lift OBSOLETES and must sweep). `[VERIFY + comment sweep]`
8. `test/fixtures/0110-tls-require-client-cert-false/` — driver (0109 clone + third arm + forced-send client), YAMLs, expectations, README. `[ADD]`

**Docs:**
9. `BEHAVIOR_CONTRACT.md` — RETIRE :902 (the "INERT (DEFERRED)" scope paragraph) and :928 (the E3 LOUD-divergence item); FIX :936(a) (SDS-VC silent-ignore — closed by the row) and sharpen :936(b) (QUIC — stays, EXTENDED per §2.7); EDIT :898/:920 (supported-paragraphs gain the false→`VerifyClientCertIfGiven` arm + the un-gated fetch); EDIT :906 (its "WITH a live SDS provider AND `require_client_certificate: true` is CONSUMED" qualifier goes stale post-lift — the qualifier gains the false arm); EDIT :900 (the Departure paragraph's "starts and serves with an unpopulated trust store" — CONTRADICTED for validation-context resources by this session's probe; edited only per the SPEC's D-RCCF-FETCHFAIL-POSTURE reconciliation, §2.4/§10, never silently); NEW supported sentence in the phase-03/16 TLS section (the inline shape rides :902's mirror-clause today and loses its anchor when :902 retires); historical recaps :2582/:5390 stay as history; :914/:924/:926/:932/:940/:944/:950 unchanged. `[IMPL]`
10. `ROADMAP.md` — row 67 (`in-progress` at the stage-close commit — controller work following the adversarial pass, a separate commit in this same worktree before squash; the BRAINSTORM commit touches only this file; summary carries "the FIFTH xDS-family row"); **NO deferred-sentence edit at ANY stage** (§9). `[STAGE-CLOSE: row]`
11. `STATE.md` — §Current pointer edited IN PLACE (ADR-0288 discipline; demote to §Recent lineage, cap FIVE). `[STAGE-CLOSE]`
12. `DECISIONS.md` — ADR-0289 §Context at the SPEC (ADR-0044 — citation-drift flag, §7); §Decision/§Consequences at the IMPL. `[SPEC/IMPL]`
13. `next-prompt.txt` — the router roll (TRACKED — `reference_next_prompt_tracked_despite_gitignore`). `[STAGE-CLOSE]`
14. Fetch-failure-posture DRIFT sites beyond the contract (D-RCCF-FETCHFAIL-POSTURE, §2.4/§10): `DECISIONS.md` ADR-0286 D-SDSVC-FETCHTIMEOUT ("the reference serves anyway (probed at SPEC-60 §11 arm B)" — a server-cert-probe extrapolation) and the landed comment at `internal/tls/config.go:116` ("the reference's serve-anyway"); together with BC:900 (item 9) these are edited only AFTER the SPEC's reconciliation — do not silently rewrite history. `[SPEC decides; edit follows]`
15. `docs/TEST_GAP_ANALYSIS.md:136` and `:200` — both claim envoy-go "build-rejects `require_client_certificate`" — stale post-lift; sweep with the §8 docs-hygiene items. `[IMPL]`

---

## 7. Anticipated ADRs — 1 at the phase-67 IMPL: ADR-0289 (`tls-require-client-cert-false` / verify-if-presented)

§Context drafts at the SPEC per ADR-0044; §Decision/§Consequences land at the IMPL. **A BRAINSTORM anchors no ADR and reserves nothing** — ADR-0289 is the anticipated next-free number (tail re-derived ADR-0288 this session), not a reservation. Content anticipation: the three-way ClientAuth mapping, the fetch-gate move, the E3 retirement, the §2.4 ADR-0280-family departure instance, the QUIC/cert-provider boundaries, the nil-pool guard.

**Citation-drift flag (mirrors the §2.6 ADR-0080 flag; same docs-hygiene pass, §8):** ADR-0044's literal body is a BEHAVIOR_CONTRACT HTTP/1.1-subsection decision, not the §Context-drafts-at-the-SPEC lifecycle rule house usage attributes to it; the usage is uniform across landed SPECs. This row follows the usage and flags the drift — fixing the attribution is OUT of this row.

---

## 8. Deferred items (the SPEC-66 §13 roster carried MINUS this row, PLUS what this row newly defers)

Carried forward from SPEC-66 §13 / ADR-0287 (this row consumes ONLY the verify-if-presented item):
- **`xds-sds-upstream-server-cert`** — VALUE-level constructibility cycle; boot-model reshape. Mechanism recorded — do not re-derive.
- **The empty-dynamic fallback (Design B)** — the message-returning seam; this row's runner-up (§2.1).
- **SDS rotation** — incl. the §3.11 implicit-watch evidence; `watched_directory` positive control FIRST (unestablished).
- **The `validation_context_type` switch's missing `default:` arm** — ambient; both unhandled arms proto-hidden + deprecated.
- **HTTP/3 `QuicProtocolOptions`** · **DataSource `environment_variable`** (D-ENV-HARNESS seam declined at SPEC-63) · **tracing `custom_tags` `metadata`** · **the compose-two edge** · **SDS `initial_fetch_timeout`/backoff edges** (reference side still a proto reading) · **`crl`** (never sized standalone; UNPROBED) · **repeated-concatenate + bool-OR merge rules** (structurally unreachable) · **the `ssl` stat family** (framework surgery, ADR-0286 C3 — NOT consumed by §5's contrast design) · **CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc`** · **gRPC / Runtime / WASM family openers**. All carry forward.

NEWLY deferred by THIS row:
- **QUIC client-auth** (`NewQUICDownstreamConfig` reads neither `require_client_certificate` nor any anchor path; inline trusted_ca dropped even at require=true) — now an EXPLICITLY named boundary rather than a discovered silence (§2.7); scoping it in needs quic-go ClientCAs wiring + pre-scan surgery (boot.go:131 skips QUIC sockets).
- **The cert-provider oneof arms (fields 10/12)** — per-arm behavior now STATED (§2.8); honoring them is its own row.
- **A `require=false` + RBAC/principal fixture arm** — the empty-`PeerCertificates` state becomes reachable and length-guard-safe here, but no fixture exercises principal-dependent surfaces with it (`0018` runs require=true) (§2.11).
- **The ADR-0080 + ADR-0044 citation drifts** — the reject-substring discipline is attributed to an ADR whose body is `default_filter_chain` (§2.6), and the §Context-at-SPEC lifecycle rule to an ADR whose body is a BEHAVIOR_CONTRACT HTTP/1.1 subsection (§7); house usage is uniform in both cases — follow-the-usage this row, fix both attributions in one docs-hygiene pass.

The termination sentinel does NOT fire: checks (2) and (3) print (preamble — ACTUAL output recorded there).

---

## 9. Cross-references against prior phases' deferred-items lists — pickup + sentinel maintenance

This row PICKS UP the SPEC-66 §13 item *"`require_client_certificate: false` / verify-if-presented (§3.10) — … Must be lifted across all three paths (CVC, plain SDS-VC, fully-inline) in one row; needs `VerifyClientCertIfGiven` + a fetch-gate restructure"* — which is itself the carried phase-65 §8 item, re-carried at phase-66 §8 and tagged D-COMBVC-REQUIRE-FALSE (§2.2/§3.10 there). **CONSUMED here.** Every other §13 item is re-carried at §8 unconsumed.

**Sentinel maintenance — NOTHING is owed:**
- **No deferred-sentence edit at ANY stage.** The item is a §8-tier pickup — it appears in NO live "candidates:" sentence (`[RUN]` grep, §1.3). The phase-64 precedent (Observability sentence byte-identical through the pickup) and the phase-66 precedent (SPEC-66 §12: "Do NOT fabricate a narrow") both bind. Check (2) keeps printing three sentences.
- **No check-(3) slug-list edit.** The pick opens no family — xDS is already listed and already opened (rows 60.1/60.2/65/66); the row summary's "the FIFTH xDS-family row" phrasing keeps the family's opener grep satisfied.
- Check (1) resumes printing (`NOT DONE: row 67`) the moment row 67 registers `in-progress` — the loop stays held open by all three checks again.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

Ranked by blast radius:
- **D-RCCF-ANCHORLESS-VC-FALSE** — the ONE unprobed cell: reference behavior for require=false/absent + a `validation_context` present but WITHOUT `trusted_ca` (inline). Probe it (fresh container per arm) before choosing reject vs boot-NoClientCert. The require=true twin keeps the :179-181 reject. §2.10(4).
- **D-RCCF-QUIC** — final in/out call on QUIC client-auth (§2.7). If OUT (recommended): pin the extended boundary wording. If IN: probe reference QUIC client-cert behavior first + size the pre-scan surgery honestly.
- **D-RCCF-NILPOOL-GUARD** — pin the construction order making IfGiven+nil-pool unconstructible, and its test (§2.5).
- **D-RCCF-FETCHGATE** — pin the §2.4 departure wording (reference: init-hold then fail-closed `downstream_context_secrets_not_ready`; envoy-go: boot-FAIL) as an ADR-0280-family instance in ADR-0289 §Context.
- **D-RCCF-FETCHFAIL-POSTURE** — reconcile the per-resource-type REFERENCE fetch-failure posture BEFORE drafting ADR-0289 §Context: three landed sites (BC:900, ADR-0286 D-SDSVC-FETCHTIMEOUT, config.go:116 — §6 items 9/14) say serve-anyway (a SERVER-CERT-probe extrapolation, SPEC-60 §11 arm B); this session's validation-context probe pinned init-hold-then-fail-closed (§2.4). Decide whether the arm-B extrapolation was wrong or the postures genuinely differ per resource type — ideally by a discriminating server-cert-fetch-failure probe (fresh container; does the listener serve cert-less or hold-then-fail-closed?). Do NOT silently rewrite history.
- **D-RCCF-FIXTURE** — the shape split: which of SDS-VC/CVC is `0110`'s primary; inline coverage via units vs a second fixture (recommend units); the forced-send driver (`GetClientCertificate`) verified against the 0108/0109 chassis. §2.12.
- **D-RCCF-E3-RETIRE** — confirm by re-derivation that E3 guards nothing else; land lift+retirement in ONE task; retained-reject roster byte-diffed. §2.6.
- **D-RCCF-P-PREMISES** — re-derive P1-P5 under the moved gate; P5 comment survives. §2.10(6).
- **D-RCCF-DRAGGED** — corrupt-inline-CA-at-require-false boot error: state as decision; probe the reference's config-validate posture on it only if cheap. §2.10(1).
- **D-RCCF-CERTPROVIDER** — per-arm post-lift statements land in BEHAVIOR_CONTRACT. §2.8.
- **D-RCCF-FUZZSEED** — re-derive the fuzz harness's provider dispatch (the phase-66 T5 lesson: a seed can be vacuous behind an earlier gate) before committing to seeds-only/+0; decide +0 vs +N there. §2.13.
- **D-RCCF-VALIDATE** — confirm M2/M3 rejects unchanged byte-identical; state it in the contract. §2.10(5).
- **D-RCCF-STATS** — confirm +0 (1201). §5.
- **D-RCCF-SPLIT** — confirm single flat row (~8-12 tasks); ADR-0045 valve armable. §1.4.
- **D-RCCF-DOCSHAPE** — re-derive the §6 roster (line numbers WILL drift off `0d4d4041`; symbols are the citation). §6.

---

## 11. Prior-phase lessons applied

- **`reference_quoting_is_not_executing` — applied by construction.** Every executable-semantics claim in this document traces to an EXECUTED probe: the reference cross-product (21 fresh arms), the fetch-gating arms, the Go `ClientAuth` × pool × client-mode table, the alert-byte pins, the nil-getter wrapper probe. Seam signatures were re-derived from declarations (input A §3 — `NewDownstreamConfig`, `FetchInitialValidationContext`, `ParseSDSConfig` et al.), the phase-66 extension of the lesson (a cited return type planned unbuildable code).
- **`reference_probe_must_discriminate` — and it PAID.** The Go probe ran polite AND forced client modes plus a `RequestClientCert` control: the polite mode exposed the **silent cert-withholding trap** (§2.12) that would have shipped a vacuous-green untrusted arm, and the control isolated the verify step as the rejector. An absent-only or single-arm probe would have pinned nothing.
- **`reference_pgv_forecloses_go_hazard` (generalized)** — no Go-derived divergence was recorded without probing the reference: the nil-pool hazard (Go-side) was checked against the reference's not-ready behavior and recorded as divergent-by-construction (Table 4), not as a parity claim.
- **`reference_code_comment_not_evidence`** — the E3-scope claim (§2.6) and the QUIC-gap claim (§2.7) rest on repo-wide getter greps and call-graph walks, not on config.go's comments; fixture require=true claims (§6 item 7) were verified in the YAMLs, not their headers. Greps stated REPO-WIDE minus `.worktrees/` — the `validate/` blind spot covered by construction this time.
- **`feedback_brief_citations_not_evidence`** — input A/B/C citations spot-re-derived before drafting: E3 at config.go:66-69, the gate at :87, `ClientAuth` at :188, QUIC at :200-223, boot pre-scan at :131, config_test.go:1017, fuzz_test.go:214-222, manager_test.go:647, the three fixtures' require=true — all HELD against `0d4d4041`.
- **`feedback_probe_fresh_container_per_arm` + the phase-66 served-this-arm extension** — all 21+ arms ran FRESH containers AND fresh driver-owned SDS servers with ARM-UNIQUE secret names; every SDS arm's transcript carries its own `SDSPROBE REQUEST … resource_names=[vc-<arm>]` witness.
- **`reference_parallel_subagents_private_scratch`** — three parallel input agents, each in a private scratch; findings merged only through their reports.
- **`reference_lifted_reject_hidden_enforcement`** — asked what ELSE E3 enforced before retiring it (§2.6: nothing — re-derived); the lift and the retirement land atomically, as phase 66 landed lift+E3 atomically in the other direction.
- **A count is only correct WITH its scope** (ADR-0287) — every count above carries one: fixtures 111 (numeric dirs), fuzzers 55 (`internal/` only), getter hits 3 (repo minus `.worktrees/`), modules 2 (lineage figure) vs 67 (go.mod requires). *(A raw consumer-line aggregate previously recorded here was DROPPED by the adversarial pass as irreproducible — its command/pattern/scope were not stated and nothing downstream consumes it.)*
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — check (2) evaluated by the exact command (`⇒ 3`), never by eye; the Observability line's historical "candidates were:" recaps not miscounted.
- **`reference_differential_run_selector` / `_break_protocol_count1` / `_fatalf_makes_assertions_unreachable`** — bind the future IMPL: full `TestDifferential/0110-tls-require-client-cert-false` selectors, `-count=1` on every deliberate break, `Errorf` per independent property, confirm WHICH assertion fired.
- **`feedback_git_worktrees` / `feedback_subagent_worktree_path_targeting` / `feedback_subagents_no_push`** — this stage ran in the pinned worktree; the main checkout stays untouched; commit is LOCAL, the controller squash-pushes (and per the router: pushes the HELD phase-66 squash FIRST).

**Adversarial verification record (phase-66 precedent — counts recorded in the document).** Three independent adversarial verifiers ran against the drafted document before stage-close: **V1** (code-claims re-derivation — every `file:line`, grep, and roster re-derived), **V2** (probe reproduction with fresh PKI and fresh containers — the SDS-VC require=false column, absent≡false, the require=true control, the fetch-gating arms incl. the docker-proxy trap and the CVC no-rescue, and the Go `crypto/tls` table incl. the nil-pool worst-direction and the cert-withholding trap vs the real reference, ALL independently reproduced), **V3** (process/consistency). They found **FIFTEEN distinct defects, ZERO severe** — all corrected in this amendment. The one finding that UPGRADES the row's obligations: the fetch-failure-posture DRIFT (BC:900 / ADR-0286 D-SDSVC-FETCHTIMEOUT / config.go:116 all record serve-anyway — a server-cert-probe extrapolation — where this session's validation-context probe pinned init-hold-then-fail-closed) → the new **D-RCCF-FETCHFAIL-POSTURE** obligation (§2.4/§6/§10). **V2's honest limits:** the inline-shape and CVC handshake reference cells rest on this session's 21-arm run (V2 reproduced the SDS-VC column plus both gating arms, not the full matrix); the SPEC's probe session re-witnesses one pinned cell as a control. Scratch hygiene: the three INPUT agents used private scratch (preamble); the VERIFIERS used named in-worktree throwaway dirs (`_probe-v1/`/`_probe-v2/`), deleted before their exit — `git status --short` clean at this amendment.

---

## 12. Section closeout

**Settled:** subject **`tls-require-client-cert-false`**, SELF-PICKED per the standing directive as the only candidate simultaneously pre-sized small, fully substrate-reusing, and reference-pinned (runner-up: the empty-dynamic Design B fallback) — §2.1; scope = the TCP apply points × all three validation shapes, stated as a three-axis roster (§2.2); the core mapping `VerifyClientCertIfGiven` pinned cell-for-cell against the reference with byte-identical wire alerts and per-side failure strings (§2.3); the un-gated fetch pinned on the reference and named as a NEW ADR-0280-family departure instance for envoy-go (§2.4); the nil-pool worst-direction hazard pinned and countered by construction order (§2.5); E3 retires atomically (§2.6); QUIC provisionally OUT with the boundary extended (§2.7); cert-provider arms stated per-arm (§2.8); absent ≡ false on both sides by independent mechanisms (§2.9); the dragged-in semantic changes enumerated with ONE unprobed cell sent to the SPEC (§2.10); no-cert principal surfaces length-guard-safe, RBAC arm newly deferred (§2.11); fixture `0110` = 0109 chassis + third arm + forced-send driver (§2.12).

**Anticipated moves at the phase-67 IMPL (docs-only now):** fixtures **111 → 112** (`0110-tls-require-client-cert-false`) · stat surface **1201 (+0)** · fuzzers **55 (+0 anticipated; D-RCCF-FUZZSEED decides at the SPEC)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0288 → ADR-0289** · go.mod modules **2 (+0)** · ZERO new packages · `internal/xds`/`internal/boot`/`validate/` BYTE-UNTOUCHED · **NO deferred sentence narrowed at any stage** (§9).

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified mechanically against the worktree tip `0d4d4041` this session):** fixtures **111** · fuzzers **55** · stat surface **1201** · BackendKind **38** · DECISIONS tail **ADR-0288** (next-free **ADR-0289**) · go.mod modules **2**.

**ROADMAP/STATE at BRAINSTORM-DONE (the stage-close commit — controller work following the adversarial pass, a separate commit in this same worktree before squash; the BRAINSTORM commit touches only this file):** row 67 registers `in-progress` with the "FIFTH xDS-family row" summary; STATE.md §Current pointer edits in place to `phase 67 BRAINSTORM done / NEXT = phase-67 SPEC`; the router rolls. Per §9, sentinel check (1) resumes printing (`NOT DONE: row 67`) the moment that registration lands.

**Next → the phase-67 SPEC** — whose first obligations are: probe the ONE uncovered cell (D-RCCF-ANCHORLESS-VC-FALSE), make the QUIC final call (D-RCCF-QUIC), re-derive the fuzz harness's provider dispatch (D-RCCF-FUZZSEED), re-derive P1-P5 under the moved gate, and draft ADR-0289 §Context per ADR-0044 — running, not quoting, every executable claim it inherits from this document.
