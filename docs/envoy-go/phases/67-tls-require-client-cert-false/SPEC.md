# SPEC 67 — TLS `require_client_certificate: false` / **verify-if-presented** mTLS (the FIFTH xDS-family row; `ClientAuth = VerifyClientCertIfGiven` across ALL THREE validation shapes — fully-inline / SDS-VC / CVC — at the TCP apply points; the anchor fetch UN-GATED from the require block; the phase-66 E3 reject RETIRED atomically; fixture `0110` CVC-primary with a FORCED-SEND driver)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-67-spec`, branch `phase-67-tls-require-client-cert-false-spec`, tip `facb0faa` (the phase-67 BRAINSTORM squash), per `feedback_git_worktrees`.
>
> **Row 67 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg). ADR-0289's §Context drafts HERE (ADR-0044-as-used; the citation drift is flagged at BRAINSTORM §7 and carried at §13); §Decision/§Consequences land at the IMPL. **The DECISIONS tail flips ADR-0288 → ADR-0289 AT THIS SPEC COMMIT** (§15) — unlike phase 66, no entry intervenes between SPEC and IMPL, so the SPEC's append IS the tail flip.
>
> **Evidence base — THREE input dossiers produced by parallel agents this session, each in a PRIVATE scratch** (`reference_parallel_subagents_private_scratch`): **(A)** LIVE reference probes — `envoyproxy/envoy:v1.37.2` digest-pinned (`sha256:c5e8a68e…f18bd`), fresh container per arm, fresh driver-owned SDS server per silent arm with an ARM-UNIQUE secret name and a served-this-arm assert: **P1** the fetch-failure-posture 2×2, **P2** the anchorless-VC 9-cell cross-product, **P3** the inline require=false re-witness control. **(B)** a mechanical code re-derivation at tip `facb0faa` — every `file:line` below re-derived there, `[RUN]`-marked greps, full-file reads of `config.go`/`fuzz_test.go`/the 0108+0109 drivers, Go 1.26.5 stdlib reads. **(C)** docs/process verbatim extracts — the three drift sites, SPEC-60 §11 arm B in full, the ADR-0044-as-used mechanics determined from git history, the BC paragraph map. Claims below are attributed **EXECUTED** (a dossier probe/`[RUN]`, named) or **RE-DERIVED** (declaration reading with `file:line` at `facb0faa`); a reading is never presented as a probe, and the dossiers' key outputs are repeated VERBATIM rather than paraphrased.
>
> **Baselines re-verified mechanically at `facb0faa` (Dossier B §14, `[RUN]`):** fixtures **111** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0109-xds-sds-combined-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/`) · DECISIONS tail **ADR-0288** (next-free **ADR-0289**; `grep -c '^## ADR-0289'` → 0, the number is FREE — Dossier C §8) · BackendKind tail **38** (`H2GoawayResponder`, fixture.go:614) · stat surface **1201** (docs-verified) · go.mod modules **2** (lineage figure; the single `go.mod` requires 67).
>
> **Sentinel expectation at this stage:** check (1) prints `NOT DONE: row 67` (registered `in-progress` at the BRAINSTORM stage-close); checks (2)+(3) unchanged (three live `candidates:` sentences; `NEVER OPENED: gRPC/Runtime/WASM`). **No deferred sentence is edited at ANY stage of this row** (§12).

---

## 1. Purpose / Mission

Hoist the three client-CA anchor arms of `NewDownstreamConfig` (`internal/tls/config.go`) — SDS-VC `:89-120` · CVC `:121-176` · inline `:177-187` — **out of** the `require_client_certificate` gate (`:87`), key them on **anchor presence**, and map `ClientAuth` **three ways** (the fourth cell never reaches a `ClientAuth` at all — it is a boot reject):

| `require_client_certificate` | anchor configured | outcome |
|---|---|---|
| `true` | yes | `RequireAndVerifyClientCert` (unchanged — today's `:188`) |
| `false` / ABSENT | yes | **`VerifyClientCertIfGiven`** (NEW — the row's core) |
| `false` / ABSENT | no | `NoClientCert` (unchanged zero value) |
| `true` | no | **boot reject** (RETAINED `:179-181` — the §3.4 envoy-go-STRICT departure; §3.12(3), T1) |

The SDS fetch fires at boot **regardless of require** (matching the reference's un-gated fetch — §3.5). The phase-66 **E3 reject retires atomically** with the lift (§3.8). Proven at the differential level by fixture **`0110-tls-require-client-cert-false`** (CVC-primary, three-arm, forced-send driver — §8). This is the direct pickup of SPEC-66 §13's item, by that landed doc's own instruction: *"Must be lifted across **all three** paths (CVC, plain SDS-VC, fully-inline) in one row"* (verbatim at SPEC-66:510 — Dossier C §7).

### 1.1 BRAINSTORM anticipations — what HELD, what this SPEC CORRECTS *(the drift ledger; drift found is the point)*

**HELD (re-derived/re-probed this session):** every `file:line` in the BRAINSTORM §6 roster items 1-3 re-derives EXACTLY at `facb0faa` (Dossier B §1: E3 `:66-69`, gate `:87`, arms `:89-120`/`:121-176`/`:177-187`, anchorless reject `:179-181`, `ClientAuth` `:188` — the repo's ONLY production assignment, `[RUN]` — QUIC `:200-223`, nil-provider gates `:381`/`:398`, E1/E2 `:402-407`, four sub-field rejects `:417-436`, boot pre-scan `:131`, 3 repo-wide `GetRequireClientCertificate` hits) · absent ≡ false by both mechanisms (B §13, A P2) · the P3 re-witness reproduced the 21-arm inline require=false row cell-for-cell (§3.2) · E3's sole-enforcement claim (B §3) · boot pre-scan require-agnostic, `internal/boot` NO EDIT · +0 packages/modules/stats/BackendKinds · the fetch-failure-posture pin itself (§3.3: the BRAINSTORM §2.4 characterization is CORRECT and now proven resource-type-uniform).

**CORRECTED — each flagged where it lands:**

| # | BRAINSTORM said | Corrected to | § |
|---|---|---|---|
| C1 | D-RCCF-FIXTURE: *"The SPEC verifies whether 0108/0109's driver already does this [forced-send]"* — open | **CLOSED as NO.** Both drivers are polite (`Certificates:` — 0108 driver.go:418, 0109 driver.go:449); Go 1.26.5 filters polite certs against server-advertised AcceptableCAs and both servers advertise. Forced-send is MANDATORY for 0110. | §3.7, §8 |
| C2 | The require=true anchorless-inline reject `:179-181` is merely "retained (unchanged)" | It is retained AND is now a **NEW NAMED envoy-go-STRICT departure**: the reference's require=true flag is **silently ineffective** without an anchor (P2 — serves unauthenticated, no CertificateRequest); envoy-go boot-fails. The BRAINSTORM did not know this. | §3.4 |
| C3 | E3 flip roster ≈ the E3-substring grep (2 test sites) | The roster is **BIGGER**: + `TestCVC_RequireFalse_NeverYieldsNoClientCert` (err!=nil half inverts) + the config_test.go:1009-1041 SDS-VC "INERT" subtest (ALL THREE assertions invert). | §3.8 |
| C4 | TEST_GAP_ANALYSIS stale claims at `:136` and `:200` | They are at **:133-137 and :198-201** (the load-bearing words at :135-136/:200-201 — B §12, C §11). AND both were **ALREADY stale against ADR-0147** (phase 16) — the IMPL sweep fixes text that was wrong BEFORE this row, not made wrong by it. | §9, §11 |
| C5 | `mkDownstreamTSRequireClientCert` at manager_test.go:647 | Declares at **:644** (:647 is the `RequireClientCertificate` field line inside it — same site, sharpened). Its consumer is a LIVE GUARD against a naive "no anchor → NoClientCert at any require" mapping (B §10). | §3.4, §10 |
| C6 | "the only listener-scope counter is `downstream_cx_total`" | Accurate for counters, but the scope also registers the **`downstream_cx_active` gauge** (manager.go:354 — B §9). | §7 |
| C7 | D-RCCF-NILPOOL-GUARD test: "unconstructible from any config" | Must be pinned against the **`xds.SecretProvider` INTERFACE**, not today's implementations: production fetchers can't return `(nil, nil)` but the interface permits it and the test-only `fakeProvider` does it (B §1 caveat). | §3.6 |
| C8 | §2.4's fail-closed wording | Sharpened, not contradicted: "fail-closed" is **per-connection AFTER bind** — `/ready` goes LIVE and the socket accepts, so liveness/readiness probes see a healthy server while 100% of TLS connections die (A P1). And Dossier C's hypothesis that the postures might differ per resource type **at the lifecycle level** is REFUTED: workers start at timeout for BOTH types (P1, 4/4 cells). | §3.3 |

**CORRECTED by the adversarial pass (this amendment — §1.2; kept HERE so this ledger stays the single list of every place this SPEC corrects earlier text):**

| # | The draft said | Corrected to | § |
|---|---|---|---|
| V1 | The serve-anyway drift is "exclusively" THREE phase-65-era sites | **FIVE living sites** — + the `internal/xds/provider.go:91-93` doc comment + the `internal/tls/config_test.go:999` failure message; ONE chartered doc-comment-only edit inside the otherwise zero-functional-change `internal/xds` envelope | §3.3, §9 (B16/B17), §11, §14, §15 |
| V2 | §1 mapping table: `any` × no-anchor → `NoClientCert` | FALSE at require=true — that cell is the RETAINED `:179-181` boot reject; the table gains its fourth row | §1, §14 |
| V3 | 0109 comment sweep = expectations.yaml:137-139 + one MANDATORY comment | The FULL enumerated set (6 sites) + a grep obligation so the IMPL can't under-sweep | §3.8 |
| V4 | B14 annotates only BC:1817's mTLS clause | The SAME line's "SDS" clause is equally stale (lifted at 60.1/60.2/65/66) — the annotation covers BOTH | §9 (B14) |
| V5 | config.go:169-173 moves INTACT inside the CVC arm | Its DEPARTURE sentence attaches the empty-dynamic ACK-and-serve posture to ALL THREE causes incl. timeout/unreachable — rewritten with cause-scoped wording | §3.13, §9 (B18), §11 |

### 1.2 Adversarial-pass record

**THREE independent verifiers ran against the draft at `1ddca9eb`, each in a PRIVATE scratch:**

- **V1 — code-claims re-derivation:** every `file:line`/grep/substring re-run at the tree. Found the **SEVERE** drift-roster gap (the serve-anyway roster missed TWO living code surfaces — §1.1 V1) + 4 moderate + 2 minor.
- **V2 — probe reproduction from scratch:** fresh PKI, self-written configs, its own silent-SDS server, fresh containers; image digest MATCHED `sha256:c5e8a68e…f18bd`. **CONFIRMED §3.1/§3.2/§3.3 cell-for-cell**, including the two most surprising pins — require=true + anchorless-VC serving unauthenticated with ZERO CertificateRequest (discriminated against a CertReq-present control with a force-transmitting client), and the init-hold → bind-at-timeout → per-connection-fail-closed posture uniform across both resource types with `downstream_context_secrets_not_ready` incrementing per attempt. **ZERO refuting defects**; 2 wording nits.
- **V3 — process/consistency:** all 15 docket items mapped one-for-one; ADR mechanics byte-checked against the ADR-0287 precedent; sentinel checks re-RUN and matching; rosters diffed. Found the mapping-table false cell + 3 minor/trivial.

**TOTAL: 13 distinct defects — 1 SEVERE, 5 MODERATE, the rest minor/trivial/nits — ALL corrected in this amendment.**

**V2's HONEST LIMITS, recorded:** the P1 UNREACHABLE column (2 of 4 cells) was NOT independently re-run (it rests on this session's input-probe run); 5 of 9 anchorless cells not re-run (the 4 run include both require=true headline cells); the require=true CVC control and the Go-side parity cells rest on the BRAINSTORM's adversarially-reproduced run.

**Scratch hygiene:** verifiers used PRIVATE scratches OUTSIDE the repo (`reference_parallel_subagents_private_scratch`); all `v2p67-*`/`p67spec-*` containers removed; worktree clean at every hand-off. *(Environment note: V2 repaired a host-level Docker/KVM ACL to run at all — outside the repo, no repo impact.)*

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8, as sharpened here)

Not QUIC client-auth (**final call: OUT** — §3.9; the BC boundary wording is EXTENDED to name the gap). Not the cert-provider oneof arms (fields 10/12 — per-arm statements land in BC, §3.10). Not the `ssl.*` stat family (framework surgery, ADR-0286 C3 — NOT a dependency, §7). Not SDS rotation, not the empty-dynamic Design B fallback, not `crl`. Not a `require=false` RBAC/principal fixture arm (BRAINSTORM §2.11; carried §13). Not the `validate`-path provider plumb (M2 keeps its byte-identical rejects — §3.11).

---

## 3. The change — the D-RCCF-* docket disposed one-for-one

*(All 15 BRAINSTORM §10 items appear with explicit dispositions: items 1-5, 7-10 and 12 in this section; item 6 D-RCCF-FIXTURE at §8; item 11 D-RCCF-FUZZSEED at §7; item 13 D-RCCF-STATS at §7; item 14 D-RCCF-SPLIT at §10; item 15 D-RCCF-DOCSHAPE at §9+§11.)*

### 3.1 The core mapping (D-RCCF-CLIENTAUTH — settled at the BRAINSTORM; control re-witnessed here) *(EXECUTED)*

The 21-arm cross-product landed at BRAINSTORM §2.3 pins verify-if-presented on all three shapes × {false, ABSENT} × {trusted, untrusted, none}, with the require=true CVC control, byte-identical wire alerts (48 `unknown_ca` / 116 `certificate_required`), and per-side failure strings. **V2's honest-limit obligation is discharged this session:** Dossier A **P3** re-witnessed the inline require=false row cell-for-cell, fresh containers, one connection per arm (`:1` counters — the pinned fact is WHICH counter moves):

- trusted (CA_A, forced) → ACCEPT, HTTP 200, `ssl.handshake:1`;
- untrusted (CA_B, **FORCED-SEND** — CertificateRequest present, cert transmitted) → REJECT, verbatim `SSL alert number 48` (`error:0A000418…tlsv1 alert unknown ca`), `ssl.fail_verify_error:1`, no `ssl.handshake`;
- none → ACCEPT, HTTP 200, `ssl.handshake:1` + `ssl.no_certificate:1`.

Go side (BRAINSTORM probe, standing): `VerifyClientCertIfGiven` + a real pool matches cell-for-cell with byte-identical alerts; client-observed failure STRINGS differ per side (OpenSSL `error:0A000418…` vs Go `remote error: tls: unknown certificate authority`) ⇒ fixture failure pins stay **PER-SIDE** (`reference_differential_reference_parses_full_message`); the wire bytes agree (`reference_wire_format_both_sides_see_same_bytes` — satisfied, not invoked).

**D-RCCF-ABSENT (settled; recorded):** the field is `*wrapperspb.BoolValue`; nil-getter `false`; NO production code inspects pointer nilness (`[RUN]` B §13 — zero `GetRequireClientCertificate() == nil` comparisons repo-wide). Reference: all ABSENT arms identical to false twins, **including the anchorless cells** (P2). No tri-state exists. YAML note: the wrapper takes a BARE scalar (`require_client_certificate: false`); `{value: false}` ERRORS (`reference_protojson_wrapper_scalar_not_object`).

### 3.2 D-RCCF-ANCHORLESS-VC-FALSE — **RESOLVED by probe P2**: the reference treats an anchorless VC EXACTLY like no validation config, in ALL NINE cells *(docket #1 — EXECUTED, Dossier A P2)*

The BRAINSTORM's one unprobed cell, probed fresh: `validation_context: {}` (literally the empty block — it parsed as-is, no PGV reject, no NACK, no validation-context-related boot warning [a generic `global_downstream_max_connections` resource-monitor warning appears at every boot]; all three configs `/ready`=`LIVE` <2s) × {`false`, ABSENT, **`true`**} × {none, CA_A-forced, CA_B-forced}. **All NINE cells outcome- and stat-IDENTICAL** (verbatim per cell): `New, TLSv1.3, Cipher is TLS_AES_256_GCM_SHA384` → `HTTP/1.1 200 OK` `PROBE-OK`; stats `ssl.handshake:1` + `ssl.no_certificate:1`. **NO CertificateRequest is ever sent** — discriminated, not assumed: s_client force-sends whenever a CertificateRequest arrives regardless of the CA-name list, yet the cert-armed cells still increment `ssl.no_certificate` (the cert was never transmitted ⇒ no request was on the wire); the P3 control with a real anchor prints `Acceptable client certificate CA names` and its CA_B cell DOES transmit (and draws alert 48). *(Curio recorded for future stat-readers: the empty VC still creates an `unnamed_ca_cert` expiration gauge pinned at int64-max.)*

**DECISIONS drafted from it:**
1. **require=false/absent + anchorless VC (no `trusted_ca`) → envoy-go boots with `ClientAuth = NoClientCert`** — matching the reference's traffic behavior (behaviorally identical to having no validation config at all). The hoisted inline arm keys on anchor presence: `vc == nil || vc.GetTrustedCa() == nil` at require=false/absent falls through to no-anchor.
2. **require=true + anchorless VC → the retained `:179-181` reject** — now recognized and NAMED as a departure (§3.4, C2).

### 3.3 D-RCCF-FETCHFAIL-POSTURE — **RESOLVED by probe P1: the postures do NOT differ per resource type**; the drift is exclusively the phase-65-era wording *(docket #5 — EXECUTED, Dossier A P1; the reconciliation the BRAINSTORM ordered)*

**The discriminating probe ran** — the full 2×2, {server-cert via SDS, validation-context via SDS} × {SILENT server, UNREACHABLE server}, fresh containers, silent SDS server logging every `DiscoveryRequest` (served-this-arm asserted in every `.sds.log`), `initial_fetch_timeout` left at the 15s default, observed t≈3s and t≈20s. **All four cells IDENTICAL:**

- t≈3s: `/ready` = `INITIALIZING`, `workers_started: 0`, listener port **UNBOUND in the container netns** (`/proc/net/tcp` — no `:28CC` row); host-side `curl -k` exit 35 is the **docker-proxy accept-then-reset trap**, NOT listener liveness;
- at the `initial_fetch_timeout` expiry (observed +15.0-15.2s): log `gRPC config: initial fetch timed out for type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret` → `all dependencies initialized. starting workers`; `/ready` → `LIVE`; port BINDS (state `0A`);
- post-bind: **EVERY connection — trusted, no-cert, curl alike — destroyed pre-handshake with NO TLS alert** (`unexpected eof`, `Cipher is (NONE)`), incrementing `listener.<addr>.server_ssl_socket_factory.downstream_context_secrets_not_ready` per attempt; **no `ssl.*` counter ever moves**; under the A2 arms the INLINE server cert does not rescue anything. Silent vs unreachable differ only in `update_failure` climbing during the hold.

**The reconciliation (per Dossier C §§1-3):**
- **SPEC-60 §11 arm B's RAW observation is CONSISTENT with this** — it observed the same init-hold-then-worker-start-then-every-handshake-fails shape for the server-cert type ("workers start … `no peer certificate available` / `SSL handshake has read 0 bytes`"). For a server cert, "serves cert-less" and "fails closed at TLS" are the same observable.
- **ADR-0280 (DECISIONS:16711), BC:886, and SPEC-60 §3.4 are DRIFT-FREE** — each keeps the every-handshake-fails clause ("starts workers CERT-LESS and lets every subsequent TLS handshake fail").
- **The DRIFT is the phase-65-era EXTENSIONS** that dropped that clause when re-targeting the claim at the trust-store type — **FIVE living sites** (the draft said "exclusively" three; the adversarial pass found two more living CODE surfaces — §1.1 V1): **BC:900** ("starts and serves with an unpopulated trust store" — never probed anywhere), **ADR-0286 D-SDSVC-FETCHTIMEOUT** (DECISIONS.md:16899 — "the reference serves anyway (probed at SPEC-60 §11 arm B)", a self-declared extrapolation from the server-cert probe), **`internal/tls/config.go:115-117`** ("the reference's serve-anyway"), **`internal/xds/provider.go:91-93`** (the `FetchInitialValidationContext` doc comment — "the documented envoy-go DEPARTURE from the reference's serve-anyway (ADR-0280, extended unchanged to this resource type…)"), and **`internal/tls/config_test.go:999`** (the test-failure message "…envoy-go boot-FAILS where the reference serves anyway — ADR-0280"). There is no "unpopulated trust store" state; there is a not-ready socket factory that kills every connection.

**THE PINNED CORRECTED CHARACTERIZATION:** *the reference init-holds (port unbound), then at `initial_fetch_timeout` starts workers and binds, then fails closed per-connection (`downstream_context_secrets_not_ready`); it never serves TLS traffic without the resource.* The italic sentence is byte-identical here, at B11's comment core, and at §14/ADR-0289; B2 and B12 carry separately-pinned EXPANDED restatements, B16 the comment-form rendering, B17 the compressed test-message form (all pinned at §9 — MIN-10: the draft's "used verbatim wherever stated" was loose). Nuance carried: fail-closed is per-connection AFTER bind — `/ready` goes LIVE and the socket accepts, so health probes see a healthy server while 100% of TLS connections die (C8).

**DECISION — the FIVE drifted sites are CORRECTED at the phase-67 IMPL, never silently** *(roster expanded from three by the adversarial pass — §1.1 V1)*: BC:900, the config.go:115-117 comment, the provider.go:91-93 doc comment, and the config_test.go:999 failure message are living surfaces, rewritten outright with replacement wording pinned VERBATIM at §9 (items B2, B11, B16, B17 — B16 is the ONE chartered doc-comment-only edit inside the otherwise zero-functional-change `internal/xds` envelope, §15); the ADR-0286 bullet — a DECISIONS entry, never silently rewritten — gains a bracketed **`[CORRECTED at phase 67/ADR-0289: …]`** annotation, pinned verbatim at §9 (item B12). **The phase-65 STAGE DOCS carrying the wording are HISTORY, not corrected** (stage docs are immutable records): 65-\*/PLAN.md:824/:1096/:1172/:1757, BRAINSTORM.md:128, SPEC-65:24/:284 — the config_test.go:999 string's ORIGIN at PLAN-65:1096 stays as history while its living copy is corrected (B17). **The envoy-go departure itself STANDS**: boot-FAIL (the ADR-0280 family, now correctly characterized on both sides) — behaviorally compatible at the traffic level (neither side EVER serves unverified traffic while the anchor is missing), divergent on lifecycle.

### 3.4 The NEW envoy-go-STRICT departure: require=true + anchorless VC *(from P2; C2 — the BRAINSTORM did not know it)*

P2's headline twin: **`require_client_certificate: true` with an anchorless `validation_context` is SILENTLY INEFFECTIVE in the reference** — no CertificateRequest, no rejection, no warning; the listener serves fully unauthenticated with `ssl.no_certificate` incrementing. The reference's require flag is only live when the VC carries an anchor. envoy-go's retained `:179-181` reject (`tls: downstream: require_client_certificate=true requires validation_context.trusted_ca` — RE-DERIVED verbatim, B §1) is therefore a **genuine envoy-go-STRICT departure: the reference silently serves an unauthenticated listener where envoy-go boot-fails.** Strictly safer; NAMED in BC (§9 item B8) and in ADR-0289 §Context (§14). The live test guard: `TestNewManager_MultiChain_RequireClientCert_Errors` (manager_test.go:1499-1517, helper at :644) asserts this reject's substring for the no-VC require=true shape — the hoisted inline arm must still route require=true + no-anchor into the reject, or that test flips red (a GOOD tripwire against the naive mapping — B §10).

### 3.5 D-RCCF-FETCHGATE — the un-gated fetch: a NEW ADR-0280-family departure instance, correctly characterized *(docket #4)*

Un-gating the anchor arms means a require=false + SDS-shape listener that boots green today (no fetch, no anchor) becomes **boot-FAIL-capable on fetch failure** — envoy-go: boot-FAIL; reference: the §3.3 pinned characterization. This is a NEW INSTANCE of the ADR-0280 departure family, named in ADR-0289 §Context with the CORRECTED reference characterization (never the serve-anyway wording). The management server cannot tell a require=false client from a require=true client (same `tls.v3.Secret`, same type URL; the BRAINSTORM's fetch-gating arms pinned identical request streams). Boot-time precision (RE-DERIVED, BRAINSTORM §4 held): `boot.NewSDSProvider`'s pre-scan counts all three SDS shapes **without reading `require_client_certificate`** (boot.go:139-142/:152-155/:174-179), so the provider already exists at require=false — **the fetch-gate move is entirely inside `internal/tls`; `internal/boot` takes NO edit.**

### 3.6 D-RCCF-NILPOOL-GUARD — assignment-adjacency + an INTERFACE-pinned unconstructibility test *(docket #3)*

The hazard (BRAINSTORM probe, standing): `VerifyClientCertIfGiven` + **nil** `ClientCAs` = Go x509 falls back to the **SYSTEM root pool** — the anchor-signed client is REJECTED (alert 48) while the no-cert client is ACCEPTED. Worst direction; no reference analogue (Go has no not-ready socket state).

**DECISION — construction order:** `ClientAuth = VerifyClientCertIfGiven` is set **ONLY in the same control-flow arm that has already installed a non-nil pool** (assignment-adjacency, mirroring how `:188` sits today relative to the three arms — RE-DERIVED B §1: every arm either `return nil, err` or executes `cfg.ClientCAs = pool` at `:120`/`:176`/`:186` before `:188` is reachable).

**DECISION — the test obligation, pinned against the INTERFACE (C7):** the `xds.SecretProvider` interface (internal/xds/provider.go:23) does not contractually forbid `FetchInitialValidationContext` returning `(nil, nil)`; the production fetchers cannot (secret.go:133-137 builds a non-nil pool on nil error; `loadTrustedCAPool` likewise, config.go:278-282 — RE-DERIVED B §1), but the test-only `fakeProvider` (config_test.go:800-815) returns `(f.pool, f.vcErr)` verbatim and `&fakeProvider{}` yields `(nil, nil)`. **The IMPL pins a test that NO config × provider-behavior combination — including a provider returning `(nil, nil)` — can yield `cfg.ClientAuth == VerifyClientCertIfGiven && cfg.ClientCAs == nil`.** (The SDS arms must treat a nil pool with nil error as an error, or structurally guarantee the state is unreachable; the PLAN picks the mechanism, the property is fixed here.)

### 3.7 The forced-send driver mandate *(from Dossier B §6 — C1: the BRAINSTORM's open question CLOSED as NO)*

**Both 0108 and 0109 drivers are polite** (`Certificates: []stdtls.Certificate{clientCert}` — 0108 driver.go:418, 0109 driver.go:449; full-file reads). Re-derived from the Go 1.26.5 stdlib actually in use (`[RUN]`, B §6): the server advertises acceptable CAs whenever `ClientCAs != nil` (handshake_server.go:670-672, tls13 :837-839); the client filters polite certs through `SupportsCertificate` (common.go:1533-1550) and **silently withholds** a cert whose chain has no `RawIssuer` among the advertised CAs. 0108's `clientBad` and 0109's `clientY` both chain to unserved CAs ⇒ against BOTH servers the bad cert is withheld and the client presents NOTHING. At require=true the untrusted arms went red for the WRONG mechanism (reject-for-absence, not verify-failure — verdict-correct but mechanism-ambiguous; 0109's expectations.yaml:145-146 partially documents the withholding). **At require=false the same withholding makes the untrusted arm indistinguishable from the none arm — both handshakes SUCCEED — a vacuous green.** ⇒ **0110's driver MUST force-send via `GetClientCertificate`** (bypasses `SupportsCertificate` entirely), extending `reference_go_client_cert_withholding`. §8 carries the design.

### 3.8 D-RCCF-E3-RETIRE — one atomic task; the flip roster is BIGGER than the substring grep *(docket #7 — RE-DERIVED, Dossier B §3; C3)*

**E3 guards nothing else — re-confirmed by call-graph re-derivation at `facb0faa`:** E3 (`:66-69`) fires only for downstream + live provider + well-formed CVC (past `:52`'s `commonTLSContextToConfig`, whose `:398`/`:401-407`/`:417-436` rejects all fire FIRST) + require false/absent. Everything else CVC-shaped is enforced elsewhere and survives independently. Post-retirement that same input flows into the hoisted CVC arm. **The lift and the retirement land in ONE task** (`reference_lifted_reject_hidden_enforcement` — phase 66 landed lift+E3 atomically in the other direction).

**The FLIP ROSTER (everything that inverts post-lift):**

| Site | Today | Post-lift |
|---|---|---|
| `TestCVC_RequireFalse_Rejected_E3` (config_test.go:1424-1454) | asserts err + the E3 substring, false AND absent | **retires/flips** to expect success + IfGiven |
| fuzz seed (i) comment (fuzz_test.go:210-228) | narrates the E3 reject | **comment flips** (the seed itself survives — the fuzz body asserts only the `tls: ` prefix on non-nil errors, so it does not FAIL, but its intent lies) |
| `TestCVC_RequireFalse_NeverYieldsNoClientCert` (config_test.go:1456-1485) | requires `err != nil` AND ClientAuth != NoClientCert | **err half inverts** (err becomes nil); the never-NoClientCert half becomes the row's LIVE property (want `VerifyClientCertIfGiven`) |
| config_test.go:1009-1041, subtest `"require_client_certificate=false leaves the SDS validation_context INERT"` | asserts NO fetch (`fakeProvider{vcErr: "FETCH MUST NOT HAPPEN"}`), `ClientCAs` nil, `NoClientCert` | **ALL THREE assertions invert** (fetch fires, pool installed, IfGiven) |
| the 0109 comment-sweep set — **FULLY ENUMERATED** (the draft under-listed it — §1.1 V3): envoy.yaml:16 + envoy-go.yaml:21 (`require_client_certificate: true is MANDATORY (PLAN-66 D1)`) · README.md:39 · expectations.yaml:24 · README.md:150-151 (the E3-boundary paragraph, the direct analogue of expectations.yaml:137-139) · expectations.yaml:137-139/:145-146 | documents E3 / require=true-MANDATORY as named boundaries | **comment sweep** (not an assertion flip). **Grep obligation so the IMPL can't under-sweep:** `grep -rn 'MANDATORY\|E3\|require_client_certificate' test/fixtures/0109-*/` — every hit dispositioned |

**RETAINED byte-identical** (the reject-roster discipline; roster byte-diffed at the IMPL) — verbatim substrings, RE-DERIVED B §1/§8: `tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03` (`:381-383`) · `tls: %s: combined_validation_context is not supported in phase 03` (`:398-400`) · `combined_validation_context.default_validation_context is required` (E1, `:402-404`) · `combined_validation_context.validation_context_sds_secret_config is required` (E2, `:405-407`) · the four sub-field `… is not supported in phase 03` rejects (`:417-436`) · `tls: downstream: require_client_certificate=true requires validation_context.trusted_ca` (`:179-181`, retained for require=true — §3.4). **RETIRED:** `tls: downstream: combined_validation_context requires require_client_certificate: true in phase 03` (`:66-69`). *(The ADR-0080 citation drift on this discipline stays flagged, follow-the-usage — BRAINSTORM §2.6, carried §13.)*

### 3.9 D-RCCF-QUIC — **FINAL CALL: OUT** *(docket #2)*

`NewQUICDownstreamConfig` (config.go:200-223, full body RE-DERIVED B §1) reads ONLY `GetCommonTlsContext()` from the inner `DownstreamTlsContext` and passes a **literal nil provider** (`:218`) — it never reads `require_client_certificate` and never touches `ClientCAs`/`ClientAuth`; `[RUN]` repo-wide: exactly 3 `GetRequireClientCertificate` hits, none in QUIC/listener code. Scoping IN would require (Dossier B §7's honest sizing): unwrapping + honoring the field + ClientCAs in `:200-223`; for SDS shapes a provider parameter + a manager.go:466 plumb + **pre-scan surgery** (boot.go:121/:131 matches only the plain `DownstreamTlsContext` type-URL and `continue`s QUIC sockets — touching `seen` accounting and the `seen>1` deferral); an unverified quic-go `ClientAuth`-enforcement question; and a bespoke QUIC reference probe (`HTTPExpectations` is TCP-only). One signature change rippling through manager+boot+prescan+a new probe surface — **not this row.** No QUIC probe was run this session (only owed if IN). **The row EXTENDS the BC boundary wording to name the QUIC client-auth gap explicitly** (§9 item B9); QUIC client-auth is a §13 deferred item.

### 3.10 D-RCCF-CERTPROVIDER — per-arm post-lift statements *(docket #10 — RE-DERIVED)*

Fields 10/12 (`validation_context_certificate_provider` / `…_instance`) remain UNREAD in production (`[RUN]` getter sweep → 0, standing). **Post-lift, per arm: they stay outside the anchor-arm switch ⇒ no pool ⇒ `ClientAuth` stays `NoClientCert` at ANY require value — the lift does NOT change their behavior silently, and this is now STATED**: at require=false the same no-anchor boot as today; at require=true the misleading-but-loud `:179-181` reject (the oneof nils `GetValidationContext()`) unchanged. Both land as named per-arm BC boundaries (§9 item B10).

### 3.11 D-RCCF-VALIDATE — M2/M3 rejects unchanged, byte-identical, require-independent *(docket #12 — RE-DERIVED, Dossier B §8)*

`validate/validate.go:49` threads a literal nil sdsProvider (comment at :48 verbatim: `// nil sdsProvider: validate does not dial/fetch SDS (phase 60.2 Task 5).`); the exported test-only ctors hardcode nil (manager.go:242/:257). Under nil providers both SDS shapes die at `:381-383`/`:398-400` — inside `commonTLSContextToConfig`, whose parameter is the `CommonTlsContext` and which therefore **cannot see `require_client_certificate`** (signature `:309`) — so both fire regardless of require, before E3, before the gate. **NO edit; the substrings stay byte-identical; stated in BC** (§9 item B7's consumer list survives). The `validate`-path divergence recorded at ADR-0287 §Consequences is unchanged in kind.

### 3.12 D-RCCF-DRAGGED — the hoist's dragged-in semantics, each a stated DECISION *(docket #9)*

1. **require=false + corrupt inline `trusted_ca` → a boot error** (the file was previously never opened). **STATED AS A DECISION, not a parity claim:** the reference's config-validate posture on a corrupt CA was **NOT probed this session** — said honestly; the decision stands on envoy-go's ADR-0280-family strict posture (malformed config fails loudly at boot), consistent with `loadTrustedCAPool`'s existing behavior at require=true.
2. **require=false + SDS fetch failure → boot-FAIL** — the §3.5 departure instance.
3. **require=true + anchorless inline VC → the `:179-181` reject STAYS** — now a named departure (§3.4).
4. **require=false/absent + anchorless VC → NoClientCert boot** — resolved by P2 (§3.2).
5. M2/M3 — §3.11. 6. P1-P5 — §3.13.

### 3.13 D-RCCF-P-PREMISES — P1-P5 all survive the hoist *(docket #8 — RE-DERIVED, Dossier B §5)*

- **P1** (only `trusted_ca` honored): enforced by `parseValidationSecret`'s four sub-field rejects (secret.go:117-128) + the four `:417-436` rejects — neither inside the require gate. UNTOUCHED.
- **P2** (DataSource oneof replaces on merge): proto-semantics; no code site in the moved region. UNTOUCHED.
- **P3** (successful fetch ⇒ specifier set): a property of the FETCH, which the hoist relocates but does not alter — fetch-dependent, not require-dependent. UNTOUCHED provided the hoisted CVC arm calls the same `FetchInitialValidationContext`.
- **P4** (four rejects re-pointed at `default_validation_context`): lives at `:417-436` inside `commonTLSContextToConfig`, called at `:52` — BEFORE E3 and the gate; **already require-independent TODAY** (at require=false they fire before E3 does). The only ordering constraint: the hoist must not reorder `:52` after the anchor arms (the natural shape).
- **P5** — the MANDATORY call-site comment, **config.go:143-149 VERBATIM** (it sits inside the CVC arm the restructure moves — **the whole `:122-149` theorem block MUST move with the arm INTACT**; ADR-0287 §Decision calls the comment MANDATORY, §Consequences a STANDING HAZARD with no tooling guard):

```go
// P5 — this rests on a COINCIDENCE, not a guarantee: internal/xds's
// dataSourceBytes and internal/tls's loadDataSource are arm-for-arm
// identical (differing only in error-string prefix), and main.go passes the
// SAME filepath.Dir(*cfgPath) as baseDir to BOTH NewSDSProvider and
// boot.Construct. Nothing structurally enforces either equality. If either
// ever diverges, this equivalence is SILENTLY falsified and CVC goes wrong
// with no test to catch it.
```

*(One CVC-arm comment does NOT move byte-intact — §1.1 V5: the fetch-error DEPARTURE comment at `:169-173` structurally attaches the phase-66 empty-dynamic ACK-and-serve sentence to ALL THREE error causes, including timeout/unreachable, whose pinned posture is init-hold-then-fail-closed. It is REWRITTEN with cause-scoped wording, pinned at §9 item B18. The P5 pin above — `:143-149` inside the `:122-149` theorem block, moved INTACT — is separate and STANDS.)*

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

One production function restructured; one reject retired; the `ClientAuth` mapping widened via a stdlib constant (`VerifyClientCertIfGiven` — `[RUN]` B §11: currently 0 hits repo-wide, FREE). No new seam, applier, resource type, or wire shape. **REUSES verbatim:** `provider.FetchInitialValidationContext` / `xds.ParseSDSConfig` / the SotW stream (relocated call sites only) · `loadTrustedCAPool` · the 0109 driver chassis (§8). Re-check `git diff go.mod` after tidy at the IMPL anyway (`reference_new_subpackage_pulls_transitive_module`). The 60.2 cycle guard stands trivially (`internal/xds` takes ZERO import/functional change — its sole edit is the B16 doc-comment correction, §15); re-verify `go list -deps ./internal/xds` (no `...`) at the IMPL as usual — TYPE-level only.

## 5. Identifier hygiene *(collision checks — `[RUN]`, Dossier B §11)*

`VerifyClientCertIfGiven` / `IfGiven` / `VerifyIfPresented` / `mkDownstreamTSVerifyIfPresented` → 0 hits each, FREE. `RequireFalse` → 5 hits, all in the two phase-66 test names + the fuzz comment — new test names must avoid those two exact symbols. `test/fixtures/0110-*` does not exist; `0110` appears nowhere in `test/differential/*.go`. Reference in-container port **10446** free (`[RUN]` → 0 hits; 10445 is 0109's). PLAN re-greps any further drafted names before adoption (`reference_spec_drafted_identifier_collision_check`).

## 6. Reject roster — retained / retired / flipped

Consolidated at §3.8 (retained substrings verbatim; E3 retired; flip roster). No NEW reject substring is introduced by this row. The retained roster is byte-diffed at the IMPL.

## 7. Stat surface **+0** (D-RCCF-STATS, docket #13) · Fuzz **+0 fuzzers, SEEDS only, dispatch-verified** (D-RCCF-FUZZSEED, docket #11)

**Stats: +0; surface stays 1201.** `[RUN]` (B §9): zero `ssl.*` stat registrations (the sole `"ssl` production hit is a Lua method-table key, not a registry name); listener scope registers exactly `downstream_cx_total` (counter) + `downstream_cx_active` (gauge) (manager.go:353-354 — C6). The reference discriminates outcomes via `ssl.handshake`/`ssl.fail_verify_error`/`ssl.fail_verify_no_cert`/`ssl.no_certificate` — reference-side ONLY; the proof obligation is discharged by the cross-side accept/reject CONTRAST (ADR-0286 C3), which 0110's three-arm verdict delivers. The `ssl` stat family row (Observability sentence) is NOT a dependency. `sds.*` lifecycle counters reused verbatim.

**Fuzz: +0 fuzzers; count stays 55.** The harness re-derived in full (Dossier B §4 — the phase-66 T5 obligation): one fuzzer `FuzzTLSContextParse` (fuzz_test.go:50); dispatch sides `"downstream"` → nil provider (`:335`), `"downstream-sds"` → live `cvcFuzzProvider` (a `&fakeProvider{pool: …}`, `:31-37`/`:341`), `"upstream"` (`:342-343`); sole assertion the `tls: ` prefix (`:347-349`). **Per-shape require=false seed reachability POST-LIFT:**

| shape | side | post-lift fate | verdict |
|---|---|---|---|
| inline | `"downstream"` | hoisted inline arm; loads pool; IfGiven | **FEASIBLE** |
| SDS-VC | `"downstream"` | dies at the retained `:381` nil-provider gate | **VACUOUS — the trap; do NOT seed here** |
| SDS-VC | `"downstream-sds"` | hoisted SDS-VC arm; fetch succeeds; IfGiven | **FEASIBLE** |
| CVC | `"downstream-sds"` | E3 gone → hoisted CVC arm; err == nil | **FEASIBLE** (seed (i) survives; only its comment lies) |

**DECISION:** new require=false seeds — inline via `"downstream"`; SDS-VC and CVC via `"downstream-sds"`. **The PLAN must not commit the trap**: an SDS-shape seed on `"downstream"` dies at the retained gates and pins only the `tls: ` prefix (the exact phase-66 T5 vacuity). Seed (i)'s stale comment (`:210-228`) flips (§3.8).

---

## 8. Differential fixture — `0110-tls-require-client-cert-false`, **CVC-primary**, three arms, FORCED-SEND driver *(D-RCCF-FIXTURE, docket #6 — DECIDED)*

**Primary shape DECISION: CVC at require=false.** It is the single shape that simultaneously exercises (i) **the E3-retired shape end-to-end** — the row's headline lift, cross-side; (ii) **the un-gated fetch LIVE at require=false** — the served-this-arm assert witnesses the boot-time SDS request; (iii) **pool-substitution-at-require-false** (the phase-66 Design A observable under the new ClientAuth mode) — in one fixture. **Considered and REJECTED: SDS-VC-primary** — it covers the fetch but not the retired reject's shape (the row's most load-bearing inversion would then have no differential witness). Coverage split: **SDS-VC** via unit tests + existing 0108 (require=true, unchanged); **inline** via unit tests + the P3 probe pin. A three-fixture spread would triple the differential cost for one mechanism. Fixtures **111 → 112**.

**Design (0109 chassis, disciplined clone — symbols per Dossier B §6):** `tcp_proxy` echo; per-side driver-owned `sdsserver.Server`s with hard `Close` (GracefulStop deadlock note at 0109 driver.go:297-299); ARM-UNIQUE secret names; the served-this-arm precondition assert (`driveSide` :361-365 pattern); `structuralCheck` widened to a three-arm `wantObservable`; `normalizeTLSErr`; in-memory PKI (`mustCA`/`mustLeaf`/… :155-240); `mustAllocatePort`; `BackendCount() == 1` (`reference_differential_backendcount_min_one`); reference in-container port **10446**.

**The verdict — THREE arms at require=false:** `trusted → ok+echo` · `untrusted → rejected` · **`no-cert → ok+echo`** (the row's discriminator — it flips vs require=true) — compared cross-side via `CompareBytes` with **PER-SIDE failure pins** (strings differ per side; wire alert 48 agrees — §3.1). The untrusted arm proves the anchor is LIVE (not a vacuous accept-all).

**MANDATORY (C1, settled — B §6 closed the question):** the untrusted arm's client **forced-sends via `GetClientCertificate`** — 0108/0109's polite `Certificates:` mode withholds an unacceptable-CA cert, and at require=false that collapses the untrusted arm into a second none arm (vacuous green). The no-cert arm uses a config with neither `Certificates` nor `GetClientCertificate` set.

**Discipline pins:** one fixture dir = ONE runner branch (`reference_differential_fixture_dispatch_constraint`); never assert `/listeners` or `total_listeners_active` for warming and **never treat a docker-proxy accept as listener liveness** (P1's trap, re-confirmed); full selector `TestDifferential/0110-tls-require-client-cert-false` (`reference_differential_run_selector`); `-count=1` on every deliberate break; `Errorf` per independent property; confirm WHICH assertion fired; 0018/0108/0109 expectations UNCHANGED (all require=true — `[RUN]`, standing); the 0109 comment sweep — the FULL enumerated set + grep obligation at §3.8.

---

## 9. Behavior-contract + drift-site edit map — replacement wording pinned VERBATIM *(D-RCCF-DOCSHAPE part 1, docket #15; all edits land at the IMPL, atomically with ADR-0289)*

*(Anchor clauses re-derived at `facb0faa` — Dossier C §9's paragraph map. Line numbers cited as of `facb0faa`; symbols/first-clauses are the citation.)*

**B1 — BC:898 (SDS-VC Supported).** The clause `under require_client_certificate: true is CONSUMED: … installs it as the listener's ClientCAs with ClientAuth = RequireAndVerifyClientCert` is re-scoped. Pinned replacement clause: **"is CONSUMED regardless of `require_client_certificate`: the boot-time SDS fetch fires unconditionally and the delivered pool installs as the listener's `ClientCAs`, with `ClientAuth = RequireAndVerifyClientCert` at `require_client_certificate: true` and `ClientAuth = VerifyClientCertIfGiven` at false/absent (verify-if-presented, phase 67/ADR-0289; absent ≡ false — wrapper `BoolValue`, nil getter)"**. The paragraph's tail is otherwise preserved.

**B2 — BC:900 (DRIFT SITE, living surface — rewritten outright).** Pinned full replacement paragraph:

> **Departure (ADR-0280, extended to this resource type; reference characterization CORRECTED at phase 67/ADR-0289): envoy-go BOOT-FAILS; the reference init-holds then fails closed per-connection.** A fetch timeout or an unreachable management server propagates a classified error (`tls: downstream: SDS validation secret %q: …`) out of the listener build → boot FAILS. The reference instead init-holds (workers un-started, listener port UNBOUND), then at `initial_fetch_timeout` (default 15s) starts workers and binds, then destroys EVERY connection pre-handshake (`listener.<addr>.server_ssl_socket_factory.downstream_context_secrets_not_ready` per attempt; no TLS alert; no `ssl.*` movement) — it never serves TLS traffic without the resource. Probed live at the phase-67 SPEC across {server-cert, validation-context} × {silent, unreachable}: all four cells identical. Note `/ready` goes LIVE and the socket accepts post-timeout, so health probes see a healthy server while 100% of TLS connections die. Traffic-equivalent to envoy-go's boot-FAIL (neither side ever serves unverified traffic without the anchor); divergent on lifecycle. Strictly-safer, consistent with envoy-go's synchronous boot model.

**B3 — BC:902 (the "INERT (DEFERRED)" scope paragraph).** **RETIRED** — false post-lift (the fetch un-gates; B1 states the new scope).

**B4 — BC:906 (Siblings STAY).** The `WITH a live SDS provider AND require_client_certificate: true is CONSUMED` qualifier gains the false arm. Pinned replacement qualifier: **"WITH a live SDS provider is CONSUMED at any `require_client_certificate` value (phase 67/ADR-0289)"**. The retained-consumer list survives verbatim (§3.11).

**B5 — BC:912 (C3 coverage boundary).** UNCHANGED (the row rides it — §7).

**B6 — BC:920 (CVC Supported).** The conjunct `downstream + a live SDS provider + require_client_certificate: true` drops its third term. Pinned replacement: **"downstream + a live SDS provider (any `require_client_certificate` value; `true` → `RequireAndVerifyClientCert`, false/absent → `VerifyClientCertIfGiven` — phase 67/ADR-0289)"**.

**B7 — BC:928 (the E3 divergence item 4).** **RETIRED** with a history-preserving replacement. Pinned: **"4. *(RETIRED at phase 67/ADR-0289.)* `require_client_certificate: false` + CVC was REJECTED by phases 66–67's E3 guard while the reference honored the anchor; phase 67 lifts verify-if-presented across all three validation shapes and retires E3 — the divergence is CLOSED."**

**B8 — NEW item (the require=true anchorless-VC departure — §3.4).** Pinned:

> **Departure (envoy-go-STRICT, named at phase 67/ADR-0289): `require_client_certificate: true` with an anchorless `validation_context` (no `trusted_ca`) — envoy-go BOOT-FAILS (`tls: downstream: require_client_certificate=true requires validation_context.trusted_ca`); the reference SILENTLY serves an effectively-unauthenticated listener.** Probed at the phase-67 SPEC (9 cells: {false, absent, true} × {no-cert, trusted-forced, untrusted-forced}): the reference accepts `validation_context: {}`, never sends a CertificateRequest in ANY cell (discriminated against a CertReq-present control), and accepts every client with `ssl.no_certificate` incrementing — the reference's require flag is only live when the VC carries an anchor. envoy-go's retained reject is strictly safer. At require=false/absent the anchorless shape boots `NoClientCert` on both sides (traffic-identical to no validation config).

**B9 — BC:936.** Half **(a)** (SDS-VC silent-ignore) — **CLOSED by this row**; the item's (a) text is replaced by a closure note citing phase 67/ADR-0289. Half **(b)** (QUIC) — **SHARPENED**, pinned: **"(b) `NewQUICDownstreamConfig` never evaluates `require_client_certificate` AT ALL — QUIC client-authentication is entirely absent (the flag is ignored at true AND false, and inline `trusted_ca` is silently dropped even at require=true; no QUIC `ClientCAs` path exists; the boot pre-scan skips QUIC sockets, boot.go type-URL match). OUT of phase 67's scope by decision (D-RCCF-QUIC); a future QUIC client-auth row needs quic-go `ClientAuth` wiring plus pre-scan surgery."**

**B10 — NEW per-arm cert-provider statements (§3.10).** Pinned: **"The `validation_context_certificate_provider` / `…_instance` oneof arms (fields 10/12) are UNREAD in production: no anchor is built ⇒ `ClientAuth` stays `NoClientCert` at ANY `require_client_certificate` value (at `true` they draw the misleading-but-loud `requires validation_context.trusted_ca` reject, since the oneof nils `GetValidationContext()`). Stated per-arm at phase 67/ADR-0289; honoring them is its own row."**

**B11 — `internal/tls/config.go:115-117` (DRIFT SITE, living surface — rewritten outright).** Pinned replacement comment:

```go
// A timeout / unreachable management server boot-FAILS the listener — the
// documented envoy-go DEPARTURE (ADR-0280 family; reference characterization
// corrected at ADR-0289): the reference instead init-holds (port unbound),
// then at initial_fetch_timeout starts workers and binds, then fails closed
// per-connection (downstream_context_secrets_not_ready) — it never serves
// TLS traffic without the resource.
```

**B12 — DECISIONS.md:16899, the ADR-0286 D-SDSVC-FETCHTIMEOUT bullet (DRIFT SITE — a DECISIONS entry, NEVER silently rewritten).** The bullet gains a bracketed annotation appended in place. Pinned:

> [CORRECTED at phase 67/ADR-0289: "the reference serves anyway" was an over-extrapolation from SPEC-60 §11 arm B — a SERVER-CERT probe; the "(probed at …)" self-citation was honest about the provenance but the wording dropped ADR-0280's every-handshake-fails clause. A phase-67 SPEC probe ({server-cert, validation-context} × {silent, unreachable}, all four cells identical) pinned the uniform reference posture: init-hold (port unbound) → at `initial_fetch_timeout` workers start + bind → every connection destroyed pre-handshake (`downstream_context_secrets_not_ready`); the reference never serves TLS traffic without the resource. The boot-FAIL departure itself stands unchanged.]

**B13 — BC:950 (0109 coverage note).** The closing clause `Both proxies configure the required require_client_certificate: true (item 4)` — pinned replacement: **"Both proxies configure `require_client_certificate: true` (mandatory under phase-66 scope; the phase-67 lift makes it a choice — fixture `0110` covers the false arm)"**. Plus the standard NEW `0110` Differential-coverage paragraph at the IMPL.

**B14 — the NEW inline verify-if-presented Supported sentence — LANDING SPOT DECIDED: the `## TLS` section (BC:1782), appended to `### Scope boundaries` after :1817.** Why there and not the SDS region: C §9 established there is NO dedicated downstream-mTLS Supported subsection anywhere; the fully-inline shape has zero xDS involvement, so a reader of the static-TLS contract looks in `## TLS` — where BC:1817 still lists "mTLS validation on the downstream side" among the not-implemented (never annotated through ADR-0147/65/66); the SDS sections already gain their per-shape false-arm wording via B1/B6, so a third copy there would be redundant. Pinned NEW paragraph:

> **Supported (phase 67/ADR-0289) — downstream verify-if-presented mTLS, all three validation shapes.** With a downstream client-CA trust anchor configured (inline `validation_context.trusted_ca`, SDS-delivered `validation_context_sds_secret_config`, or `combined_validation_context`) and `require_client_certificate` false or absent, envoy-go sets `ClientAuth = VerifyClientCertIfGiven`: a presented client certificate is verified against the anchor (untrusted ⇒ handshake reject, TLS alert 48 `unknown_ca`); a connection presenting no certificate is accepted. `require_client_certificate: true` keeps `RequireAndVerifyClientCert` (phase 16/ADR-0147); no anchor keeps `NoClientCert`. Absent ≡ false. Matches the reference cell-for-cell with byte-identical wire alerts (probed at the phase-67 BRAINSTORM + SPEC); client-observed failure strings differ per side.

And BC:1817 — where **TWO clauses of the phase-03 not-implemented list are now stale**, the "mTLS validation on the downstream side" clause AND the "SDS" clause on the SAME line (the draft flagged only the first — §1.1 V4) — gains a pinned trailing annotation covering BOTH: **"*(Two entries in this list have since been lifted. mTLS validation on the downstream side: `require_client_certificate: true` at phase 16/ADR-0147, SDS-delivered anchors at phases 65/66, verify-if-presented at false/absent at phase 67/ADR-0289 — see the Supported paragraph below. SDS: the discovery substrate at phase 60.1/ADR-0278, the first (server-cert) applier at phase 60.2/ADR-0280, the validation-context applier at phase 65/ADR-0286, `combined_validation_context` at phase 66/ADR-0287.)*"**

**B15 — `docs/TEST_GAP_ANALYSIS.md` :133-137 + :198-201 (C4).** Both "envoy-go build-rejects `require_client_certificate`" claims were ALREADY stale against ADR-0147 and become doubly stale post-67. The IMPL sweep rewrites both bullets to the post-67 truth (require=true since phase 16; verify-if-presented since phase 67; the missing-client-cert rejection test now exists at 0018/0108/0109/0110) — **flagged as pre-existing drift this row fixes in passing, not drift it creates.**

**B16 — `internal/xds/provider.go:91-93` (DRIFT SITE #4, living surface — the `FetchInitialValidationContext` doc comment; found by the adversarial pass, §1.1 V1).** The ONE chartered doc-comment-only correction inside the otherwise zero-functional-change `internal/xds` envelope (§15 states the decision and rationale). Pinned replacement for the comment's final sentence (`// returns an error, which boot-FAILS…` through `// …D-SDSVC-FETCHTIMEOUT).`):

```go
// returns an error, which boot-FAILS the listener — the documented envoy-go
// DEPARTURE (ADR-0280 family; reference characterization corrected at
// ADR-0289): the reference init-holds (port unbound), then at
// initial_fetch_timeout starts workers and binds, then fails closed
// per-connection (downstream_context_secrets_not_ready) — it never serves
// TLS traffic without the resource. (SPEC-65 §11 D-SDSVC-FETCHTIMEOUT.)
```

**B17 — `internal/tls/config_test.go:999` (DRIFT SITE #5, living surface — the test-failure message; found by the adversarial pass, §1.1 V1).** Its string ORIGINATES at PLAN-65:1096 (which stays as history). Pinned replacement string:

```go
t.Fatal("expected a boot failure, got nil (envoy-go boot-FAILS where the reference init-holds then fails closed per-connection — ADR-0280 family, characterization corrected at ADR-0289)")
```

**B18 — `internal/tls/config.go:169-173` (the CVC arm's fetch-error DEPARTURE comment — §1.1 V5, §3.13).** The current comment attaches the empty-dynamic ACK-and-serve sentence structurally to ALL THREE causes; rewritten cause-scoped as the block moves with the hoisted CVC arm. Pinned replacement:

```go
// A served validation context with no usable trusted_ca, a timeout, or an
// unreachable management server boot-FAILS the listener (ADR-0280 family).
// DEPARTURE, per cause: for a served-but-EMPTY dynamic context the reference
// ACKs, falls back to the DEFAULT CA, and SERVES (the phase-66 empty-dynamic
// departure); for timeout/unreachable the reference init-holds (port
// unbound), then at initial_fetch_timeout starts workers and binds, then
// fails closed per-connection (downstream_context_secrets_not_ready —
// characterization corrected at ADR-0289). envoy-go refuses to boot on all
// three causes.
```

**Historical recaps BC:2582/:5390 stay as history; so do the phase-65 STAGE DOCS carrying the serve-anyway wording (65-\*/PLAN.md:824/:1096/:1172/:1757, BRAINSTORM.md:128, SPEC-65:24/:284 — immutable records, §3.3). BC:914/:924/:926/:932/:938/:940/:942/:944 unchanged.**

---

## 10. Test plan + task surface *(D-RCCF-SPLIT, docket #14 — CONFIRMED: a SINGLE FLAT ROW)*

**~9 tasks, ONE production file** — within the BRAINSTORM's ~8-12; materially smaller than phase 65 (11) and comparable to phase 66 (9). **ADR-0045 escape valve ARMABLE, no split anticipated** — no two-package surface can strand a leg (`internal/xds`/`internal/boot`/`validate/` untouched). Sketch (the PLAN decomposes; TDD):

- **T1** — the hoist + three-way `ClientAuth` (assignment-adjacency, §3.6) + **E3 retirement ATOMIC** + the `:122-149` theorem block moved intact (§3.13) + require=true anchorless routing preserved (§3.4's tripwire test stays green).
- **T2** — the flip roster (§3.8): four test sites inverted/flipped, each shown RED first where it pins a NEW property.
- **T3** — new unit tests: three shapes × {false, absent} × anchor → IfGiven + pool; anchorless × {false, absent} → NoClientCert; corrupt-inline-CA at require=false → boot error (§3.12); the **interface-pinned nil-pool unconstructibility test** (§3.6, incl. the `(nil,nil)` fakeProvider arm).
- **T4** — fuzz seeds (§7; the dispatch-side trap honored) + seed (i) comment flip.
- **T5-T6** — fixture `0110` (§8) + deliberate breaks: `structuralCheck` liveness, a break proving the untrusted arm's FORCED-SEND is live (e.g. flip to polite mode → the arm must go green-for-the-wrong-reason and the break harness must CATCH it), per-side failure-pin breaks. `-count=1`; full selectors; confirm WHICH assertion fired; non-compiling break ⇒ substitute + report.
- **T7** — comment sweeps: config.go:115-117 (B11) + :169-173 (B18), provider.go:91-93 (B16 — the chartered `internal/xds` doc-comment edit), the config_test.go:999 message (B17), and the FULL 0109 set (§3.8's enumerated roster + the grep obligation).
- **T8** — BC delta (§9 B1-B15; the code-comment items B16-B18 land via T7) + TEST_GAP sweep.
- **T9** — ADR-0289 completed IN PLACE (§Decision/§Consequences; banner flip) + ROADMAP/STATE/router (controller-adjacent, stage-close).

Coverage properties, each an independent `Errorf` (`reference_fatalf_makes_assertions_unreachable`): the three-way mapping per shape; fetch-fires-at-require=false (the INERT inversion); nil-pool unconstructibility; retained-reject roster byte-diff; M2/M3 unchanged; the manager_test.go:1499 guard green.

## 11. Edit-site roster *(D-RCCF-DOCSHAPE part 2, docket #15 — RE-DERIVED at `facb0faa`, Dossier B §1: ZERO drift from the BRAINSTORM's `0d4d4041` citations)*

**Production — `internal/tls/config.go` (SOLE functionally-edited production file):** E3 `:66-69` `[RETIRE]` · the gate `:87` + arms `:89-120`/`:121-176`/`:177-187` `[HOIST — the core]` · `:179-181` `[RETAIN for require=true]` · `ClientAuth` `:188` `[→ three-way]` · the `:122-149` theorem/P5 block `[MOVE INTACT]` · `:115-117` comment `[REWRITE — B11]` · `:169-173` comment `[REWRITE — B18]`. **Everything else:** `internal/boot/boot.go` `[NO EDIT]` · `internal/xds` `[ONE chartered doc-comment-only correction — provider.go:91-93, B16 (drift site #4); otherwise BYTE-UNTOUCHED, zero symbols, zero functional change — §15]` · `internal/listener` (incl. quic.go) · `validate/` · `test/helpers/sdsserver` `[UNTOUCHED]`.

**Test/harness:** config_test.go `:1009-1041`/`:1424-1454`/`:1456-1485` `[FLIP]` · config_test.go `:999` `[MESSAGE CORRECTION — B17 (drift site #5)]` + new tests `[ADD]` · fuzz_test.go `:210-228` `[COMMENT FLIP]` + seeds `[ADD]` · manager_test.go `:644`/`:1499-1517` `[VERIFY — live guard]` · fixtures 0018/0108/0109 `[VERIFY + the §3.8 enumerated comment sweep]` · `test/fixtures/0110-tls-require-client-cert-false/` `[ADD]`.

**Docs:** BEHAVIOR_CONTRACT.md per §9 `[IMPL]` · DECISIONS.md — ADR-0289 §Context **AT THIS COMMIT** (§14), completion + the B12 annotation at the IMPL · docs/TEST_GAP_ANALYSIS.md `:133-137`/`:198-201` `[IMPL sweep — C4]` · ROADMAP/STATE/next-prompt `[STAGE-CLOSE — controller]`.

## 12. Sentinel maintenance — NOTHING is owed

**No deferred-sentence edit at ANY stage** — the item is a §8-tier pickup appearing in NO live `candidates:` sentence (`[RUN]` standing; C §6 re-verified: the xDS sentence at ROADMAP:187 does not name it). Do NOT fabricate a narrow (phase-64/66 precedents). Check (2) keeps printing three sentences (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` ⇒ 3 — C §6 `[RUN]`); check (3) unaffected (no new family; "the FIFTH xDS-family row" phrasing stands in row 67); check (1) keeps printing `NOT DONE: row 67` until the IMPL six-gate.

## 13. Deferred items *(the BRAINSTORM §8 roster carried MINUS NOTHING — this SPEC consumes no deferred item — PLUS nothing newly deferred at this stage)*

Carried unchanged from BRAINSTORM §8: **`xds-sds-upstream-server-cert`** (+ the upstream validation_context sibling; VALUE-level cycle, mechanism recorded — do not re-derive) · **the empty-dynamic Design B fallback** (the runner-up) · **SDS rotation** (implicit-watch evidence; `watched_directory` positive control FIRST) · **the `validation_context_type` switch's missing `default:` arm** · **HTTP/3 `QuicProtocolOptions`** · **DataSource `environment_variable`** (D-ENV-HARNESS) · **tracing `custom_tags` `metadata`** · **the compose-two edge** · **SDS `initial_fetch_timeout`/backoff edges** · **`crl`** · **repeated-concatenate + bool-OR merge rules** · **the `ssl` stat family** (framework surgery — NOT consumed by §7's contrast design) · **CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc`** · **gRPC / Runtime / WASM family openers** · **QUIC client-auth** (§3.9 — the boundary now EXTENDED in BC wording, B9) · **the cert-provider oneof arms (10/12)** (§3.10) · **a `require=false` + RBAC/principal fixture arm** (0018 runs require=true) · **the ADR-0080 + ADR-0044 citation drifts** (follow-the-usage this row; one docs-hygiene pass later).

**Memory updates owed at the IMPL:** (i) extend `reference_go_client_cert_withholding` with the settled 0108/0109 fact (both drivers polite; negative arms verdict-correct but mechanism-ambiguous at require=true; forced-send mandatory for any require=false untrusted arm); (ii) the serve-anyway → init-hold-fail-closed drift correction (a probe extrapolated across resource types entered FIVE landed living sites — and a three-site roster survived a full draft until an adversarial re-grep found the other two; re-probe per resource type before extending a departure's wording, and roster drift sites by REPO-WIDE grep, not by memory).

## 14. ADR continuity — the ADR-0289 §Context DRAFT (anchored here; full entry at the phase-67 IMPL)

*(Mirrored VERBATIM in `docs/envoy-go/DECISIONS.md` at this SPEC commit — the tail flips ADR-0288 → ADR-0289 here; the IMPL completes the entry IN PLACE.)*

> **ADR-0289 — TLS `require_client_certificate: false` / verify-if-presented mTLS (the FIFTH xDS-family row; `ClientAuth = VerifyClientCertIfGiven` across ALL THREE validation shapes; the anchor fetch UN-GATED from the require block; the phase-66 E3 reject RETIRED; a SINGLE FLAT ROW, the SOLE leg — FLIPS ROW 67 `done`; the xDS family STAYS OPEN).**
>
> **§Context (drafted at the phase-67 SPEC per ADR-0044).** Phase 60.1 built the SDS discovery substrate (ADR-0278); 60.2 wired the first applier (ADR-0280); 65 delivered the validation-context pool applier (ADR-0286); 66 proved CVC composition by pool substitution (ADR-0287) and guarded its envelope with E3 — an explicit reject of CVC + `require_client_certificate` false/absent, chosen there over a three-path restructure, with the instruction that lifting `require == false` "belongs in a row that fixes ALL THREE paths (CVC, plain SDS-VC, fully-inline) together". This row is that row. It hoists the three anchor arms of `NewDownstreamConfig` out of the require gate, keys them on anchor presence, and maps `ClientAuth` three ways across four cells: `true` + anchor → `RequireAndVerifyClientCert` (unchanged); false/absent + anchor → `VerifyClientCertIfGiven`; false/absent + no anchor → `NoClientCert`; `true` + no anchor never reaches a `ClientAuth` at all — the RETAINED boot reject, the envoy-go-STRICT departure named below. Absent ≡ false on both sides by independent mechanisms (the field is a `*wrapperspb.BoolValue` whose nilness no production code inspects; all nine reference ABSENT probe arms are outcome- and stat-identical to their false twins). The reference mapping is pinned cell-for-cell — a 21-arm cross-product at the BRAINSTORM plus a fresh SPEC-time re-witness control — with byte-identical wire alerts (48 `unknown_ca` for a presented-untrusted cert; the no-cert cell is the ONLY one that moves when the flag flips) and per-side client-observed failure strings.
>
> The hoist un-gates the SDS anchor fetch from `require_client_certificate` (the reference fetches identically at both values), which makes a require=false SDS-shape listener boot-FAIL-capable on fetch failure — a NEW instance of the ADR-0280 boot-FAIL departure family, and the occasion for a reconciliation: five phase-65-era living sites (BEHAVIOR_CONTRACT:900, ADR-0286's D-SDSVC-FETCHTIMEOUT bullet, `internal/tls/config.go:115-117`, the `internal/xds/provider.go:91-93` doc comment, and the `internal/tls/config_test.go:999` test-failure message) characterized the reference's validation-context fetch-failure posture as "serves anyway / serves with an unpopulated trust store" — an extrapolation from SPEC-60 §11 arm B, a SERVER-CERT probe, that dropped ADR-0280's every-handshake-fails clause. A discriminating SPEC-time probe ({server-cert, validation-context} × {silent SDS, unreachable SDS}; all four cells identical) pinned the uniform posture: **the reference init-holds (port unbound), then at `initial_fetch_timeout` starts workers and binds, then fails closed per-connection (`downstream_context_secrets_not_ready`); it never serves TLS traffic without the resource** — while `/ready` goes LIVE and the socket accepts, so health probes see a healthy server as 100% of TLS connections die. ADR-0280, BC:886, and SPEC-60's own wording are drift-free; arm B's raw observation is consistent. The four living drifted code/doc surfaces are rewritten outright at this row's IMPL with that corrected characterization — including ONE chartered doc-comment-only correction inside the otherwise zero-functional-change `internal/xds` envelope (provider.go:91-93; a "complete" reconciliation that left drifted wording standing in production code would be a false completeness claim); the ADR-0286 bullet — a DECISIONS entry, never silently rewritten — gains a bracketed correction annotation; the phase-65 stage docs carrying the wording stay as history (immutable records). The boot-FAIL departure itself stands: traffic-equivalent, lifecycle-divergent.
>
> E3 retires atomically with the lift — re-derived to guard nothing else — and its flip roster is larger than the reject-substring grep: `TestCVC_RequireFalse_Rejected_E3`, the fuzz seed (i) comment, the err-half of `TestCVC_RequireFalse_NeverYieldsNoClientCert` (its never-NoClientCert half becomes the row's live property, now wanting `VerifyClientCertIfGiven`), and the SDS-VC "INERT" subtest whose three assertions (no fetch / nil `ClientCAs` / `NoClientCert`) all invert. Every other reject is retained byte-identical: the nil-provider gates (which sit in `commonTLSContextToConfig`, cannot see the field, and fire regardless of require — the `validate` path and the test-only constructors keep their exact substrings), E1/E2, the four sub-field rejects (already require-independent; they fire before E3 today), and the require=true anchorless-inline reject. That last reject is newly recognized as an **envoy-go-STRICT departure**: a SPEC-time nine-cell probe showed the reference accepts `validation_context: {}` and treats it exactly like no validation config at every require value — no CertificateRequest is ever sent (discriminated against a CertReq-present control), so `require_client_certificate: true` with an anchorless VC is **silently ineffective** in the reference, serving an unauthenticated listener where envoy-go boot-fails. At false/absent the anchorless shape boots `NoClientCert` on both sides. A related hazard is made unrepresentable by construction: `VerifyClientCertIfGiven` with a nil `ClientCAs` falls back to Go's SYSTEM roots — rejecting the legitimate client while admitting anonymous ones — so the assignment is made only in the control-flow arm that installed a non-nil pool, and a test pinned against the `xds.SecretProvider` INTERFACE (which, unlike the production fetchers, permits `(nil, nil)`) proves no config × provider behavior yields `VerifyClientCertIfGiven` + nil pool. Requiring a boot error for a corrupt inline `trusted_ca` at require=false is a stated decision on envoy-go's strict posture, not a parity claim (the reference's corrupt-CA config-validate posture was not probed). Phase-66's P1–P5 equivalence-theorem premises all survive the hoist (P4 was already require-independent; the mandatory P5 coincidence comment moves with the CVC arm intact). QUIC stays OUT: `NewQUICDownstreamConfig` never reads the field, and scoping it in would need quic-go `ClientAuth` wiring plus pre-scan surgery — the boundary wording is extended to name the QUIC client-auth gap explicitly. The cert-provider oneof arms remain outside the anchor switch: `NoClientCert` at any require value, now stated per-arm.
>
> Proven differentially by fixture `0110-tls-require-client-cert-false` (fixtures 111 → 112), CVC-primary at require=false — the one shape exercising the E3-retired path end-to-end, the un-gated fetch live (served-this-arm assert), and pool substitution under the new mode in a single three-arm verdict (trusted → ok+echo · untrusted → rejected · no-cert → ok+echo, the discriminator vs require=true), with SDS-VC and inline covered by unit tests plus the existing 0108 and the probe pins. The driver **must force-send the untrusted certificate via `GetClientCertificate`**: both 0108 and 0109 use polite `Certificates:` mode, and Go's client silently withholds a cert whose issuer is absent from the server-advertised acceptable-CA list — harmless at require=true (reject-for-absence), but at require=false it would collapse the untrusted arm into a second no-cert arm and ship a vacuous green. Envelopes: +0 packages, +0 modules, +0 stats (the outcome proof is the cross-side accept/reject contrast, per ADR-0286 C3), +0 fuzzers (seeds only, dispatch-verified: SDS-shape seeds must ride the `"downstream-sds"` side or die vacuously at the retained nil-provider gates), zero functional edits outside `internal/tls` production code (the sole exception, deliberate and chartered: the `internal/xds/provider.go:91-93` doc-comment drift correction).
>
> *(§Decision + §Consequences land at the phase-67 IMPL.)*

## 15. Exit — counts + expectations at SPEC-DONE

**Counts at this SPEC commit (docs-only apart from the DECISIONS append):** fixtures **111** · fuzzers **55** · stat surface **1201** · BackendKind **38** · go.mod modules **2** · **DECISIONS tail flips ADR-0288 → ADR-0289 AT THIS COMMIT** (the §Context draft append IS the tail flip — expected and correct; next-free becomes ADR-0290; the router's "re-derive, never trust" tail check must expect `## ADR-0289`).

**Anticipated at the phase-67 IMPL:** fixtures **111 → 112** (`0110-tls-require-client-cert-false`) · stat surface **1201 (+0)** · fuzzers **55 (+0)** · BackendKind **38 (+0)** · go.mod **2 (+0)** · ZERO new packages · `internal/boot`/`validate/` BYTE-UNTOUCHED · **`internal/xds`: ZERO new symbols, ZERO functional/behavioral change** — byte-untouched EXCEPT the **ONE chartered doc-comment-only correction** (provider.go:91-93, §9 B16) under D-RCCF-FETCHFAIL-POSTURE. *This carve-out is a DELIBERATE DECISION, not a silent relaxation of the draft's "BYTE-UNTOUCHED" pin: provider.go:91-93 is drift site #4 of the serve-anyway roster (§3.3), and a "complete" reconciliation that left the drifted wording standing in production code would be a false completeness claim; the envelope's REAL invariant — no new symbols, no behavior change in `internal/xds` — holds unweakened.* · ADR-0289 completed IN PLACE.

**Sentinel:** re-verified expectation — check (1) keeps printing `NOT DONE: row 67`; checks (2)+(3) unchanged; **no deferred-sentence edit at any stage** (§12); no `stop` file.

**Probe hygiene:** ALL probe harnesses this session (the P1/P2/P3 configs, the silent-SDS Go module, PKI, logs) lived in the parallel agents' PRIVATE scratches OUTSIDE the repo (per `reference_parallel_subagents_private_scratch`; Dossier A §Hygiene: containers removed, processes exited, `git status --short` clean) — **nothing to delete in-tree**; the worktree contains only this SPEC and the DECISIONS append at commit time.

**Next → the phase-67 PLAN.** Its inheritance: the three-way mapping with assignment-adjacency (§3.6) and the interface-pinned unconstructibility property; the ATOMIC lift+E3-retirement with the FULL flip roster (§3.8 — bigger than the substring grep); the `:122-149` block moved INTACT; the forced-send driver mandate (§3.7 — the question is CLOSED, not open); the fuzz dispatch trap (§7); the pinned §9 wording applied MECHANICALLY (the three drift corrections are named obligations, never silent rewrites). **RE-DERIVE this document; do not execute it.** Where it cites, go look; where it claims control flow, walk the call graph; default to REFUTED.
