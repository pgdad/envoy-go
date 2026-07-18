# Phase 68 Brainstorm — `tls-cvc-empty-dynamic-fallback` (the SIXTH xDS-family row; honor the reference's `combined_validation_context` MERGE semantics on an ACKed-but-EMPTY dynamic half — fall back to `default_validation_context.trusted_ca` and SERVE where envoy-go today boot-FAILS; the direct pickup of "Design B", the runner-up phase 66 filed as a NAMED ADR-0280-family departure and phase 67 re-deferred; anticipated +0 packages / +0 modules / +0 stats / +0 BackendKinds / +0 fuzzers; anticipated ONE new fixture `0111`; anticipated ONE new `internal/xds` exported symbol — the FIRST break of the recent zero-new-symbol streak, stated loudly)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-68-brainstorm`, branch `phase-68-empty-dynamic-fallback-brainstorm`, off master `92cd1647` (the phase-67 IMPL squash), per `feedback_git_worktrees`.
>
> **Row 68 registers `in-progress`** at this stage-close commit per the ROADMAP §Schema invariant (`depends-on 67`); it flips `done` at the phase-68 IMPL six-gate (ADR-0106, the SOLE leg unless split). The roller SELF-PICKED this subject per the 2026-07-12 standing directive (no human pick; the termination sentinel does NOT fire — checks (2)+(3) still print).
>
> **Evidence base — THREE input dossiers produced by parallel agents this session, each in a PRIVATE scratch OUTSIDE the repo** (`reference_parallel_subagents_private_scratch`): **(A)** LIVE reference probes — `envoyproxy/envoy:v1.37.2`, fresh container per arm, a throwaway SotW SDS sibling-container logging every `DiscoveryRequest` (served-this-arm asserted), forced-send client certs via `openssl s_client -cert`, TLS-layer discrimination — the empty-served-vs-timeout 2×2 plus the empty-both and require cross-products. **(B)** a mechanical code re-derivation at `92cd1647` — the seam constructibility, the P1–P5 delta, the envelope. **(C)** candidate-comparison + docs/ADR/sentinel extracts. Claims below are attributed **EXECUTED** (a Dossier A probe arm, named) or **RE-DERIVED** (declaration reading with `file:line` at `92cd1647`); a reading is never presented as a probe. **RE-DERIVE this document; do not execute it** (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`).
>
> **Baselines re-verified mechanically at `92cd1647` (Dossier C):** fixtures **112** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0110-tls-require-client-cert-false`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/`) · DECISIONS tail **ADR-0289** (next-free **ADR-0290**) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** · go.mod modules **2** (lineage figure; the single `go.mod` requires 67).

---

## 1. Mission and scope confirmation (68 — a MERGE-SEMANTICS completion on the landed CVC applier, NOT a new resource, seam, or discovery machine)

### 1.1 What phase 68 delivers as a self-contained whole

Phase 66 delivered `combined_validation_context` (CVC) by **pool substitution** (ADR-0287, Design A): the SDS-delivered dynamic pool wins outright and `default_validation_context.trusted_ca` is NEVER read. That is provably identical to the reference's `MergeFrom` **on the SUCCESS path only** — because a successful fetch guarantees a non-empty dynamic `trusted_ca` (theorem premise P3, config.go:149-151). Phase 66 knew and NAMED the gap: when the SDS server serves an ACKed-but-**EMPTY** validation context (a `Secret` whose `validation_context` carries no `trusted_ca`), the reference merges (empty contributes nothing) so the **default's `trusted_ca` survives** and the listener SERVES — where envoy-go boot-FAILS. That was filed as "Design B", the runner-up, and left as a named departure (SPEC-66:267,509; the cause-scoped comment lives at `internal/tls/config.go:185-193`).

Phase 68 closes it: the CVC arm of `NewDownstreamConfig`, on an **ACK-succeeded-but-empty** dynamic fetch, falls back to `default_validation_context.trusted_ca` (via the existing `loadTrustedCAPool`) and installs it as `ClientCAs` with the phase-67 three-way `ClientAuth` — matching the reference cell-for-cell. **On envoy-go's honored surface (only `trusted_ca` is honored — P1) the reference `MergeFrom` reduces EXACTLY to "use the dynamic `trusted_ca` if present, else the default's"** — so the fallback IS the honored-surface projection of the merge; no `proto.Merge` is needed, and ADR-0287's pre-characterization of Design B as "a message-returning seam" (DECISIONS.md:16975) is **pessimistic** (B §5, C §1).

### 1.2 What phase 68 does NOT deliver (forward to §8)

Not a general `MergeFrom` (envoy-go honors only `trusted_ca`; the other ten `CertificateValidationContext` fields stay a shared silent gap or a shared reject — unchanged). Not the plain SDS-VC empty-served case (no inline default to fall back to — stays boot-FAIL, a RETAINED departure — §2.6). Not the compose-two CVC edge (`seen>1`). Not the PGV `default_validation_context`-required constraint as an automatic reject (PGV-only on the reference; a stated decision — §2.7). Not QUIC (no client-auth path at all — phase 67 D-RCCF-QUIC). Not the `ssl` stat family.

### 1.3 Phase-done as the SIXTH xDS-family row (family STAYS OPEN)

Row 68 is the SIXTH xDS-family row (row 67 self-registered as the FIFTH; the landed xDS-family lineage runs 60 SDS-server-cert → 65 SDS-validation-context → 66 CVC → 67 verify-if-presented — rows 61/62/63/64 are HTTP/3 and tracing, NOT xDS). The xDS family STAYS OPEN — its live deferred sentence (ROADMAP:188: SDS rotation + upstream SDS + CDS/EDS + …) is UNCHANGED by this row, which is a §13-tier pickup named in NO live `candidates:` sentence (C §3; §9).

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW *(SPEC confirms D-EDF-SPLIT)*

~8–9 tasks, ONE functionally-edited production file (`internal/tls/config.go`) plus ONE new `internal/xds` exported symbol (a classifier — §2.2). No two-package surface can strand a leg (`internal/boot`/`validate/`/`internal/listener` untouched). ADR-0045 escape valve ARMABLE, no split anticipated.

### 1.5 Package placement — the functional edit is ONE existing file; ONE new `internal/xds` export

All functional change is in `internal/tls/config.go` (the CVC arm's error branch). The ONE structural addition is a single exported symbol in `internal/xds` so `internal/tls` can distinguish "ACK-succeeded-but-empty" from "fetch-failed/timeout" (the classifier `errValidation` is today unexported, stream.go:33 — B §1). **This BREAKS the recent "zero new `internal/xds` symbols" streak (DECISIONS.md:16973) — stated LOUDLY here and owed as an explicit envelope call at the SPEC** (§5). It is minimal (one sentinel/helper, no behavior change in `internal/xds`), and the cycle-guard direction is safe (internal/tls already imports internal/xds; the export adds NO dependency to internal/xds — B §4; `reference_xds_config_seam_transitive_cycle_guard`, TYPE-level).

### 1.6 Relationship to the existing seams

REUSES verbatim: `provider.FetchInitialValidationContext` (or a variant — §2.3) · `xds.ParseSDSConfig` · the SotW stream · `loadTrustedCAPool` (config.go:298) · the CVC pre-checks in `commonTLSContextToConfig` (the four sub-field rejects re-pointed at `default_validation_context`, config.go:442-461, landed at phase 66) · the phase-67 `installPool`/`clientAuthFor` closures + the nil-pool guard · the 0109 fixture chassis (§2.11).

### 1.7 Adversarial-pass record

An independent verifier re-derived every load-bearing claim at `92cd1647`, cross-checked the EXECUTED claims against Dossier A per-arm, and stress-tested the candidate adjudication (default REFUTED). **Verdict: the PICK is SOUND; ZERO SEVERE; the design direction is unchanged.** All code anchors, the seam constructibility (one `internal/xds` export, no `proto.Merge`, no boot/validate/listener edits), the P4-not-reintroduced call-order, every probe-arm claim, the §2.1 adjudication, and the counts/sentinel/ADR re-derived CORRECT. Findings corrected in this document: **M1** — the "ACK-succeeded-but-empty" shorthand names the REFERENCE's posture, but envoy-go TODAY classifies the empty resource as `errValidation`/NACK, leaving a RESIDUAL SDS-stream posture divergence after the fallback → now NAMED as D-EDF-STREAMPOSTURE (§2.12, §10); the classifier discriminator (§2.2) was independently confirmed sound regardless of the framing. **m2** — the `trusted_ca:{inline_bytes:""}` set-but-empty wire shape is DISTINCT from the specifier-unset case → added as a separate arm to the D-EDF-CLASSIFY probe roster (§2.2, §10). **m1** — the three live `candidates:` sentences are at ROADMAP:178/188/198 (corrected below); the "empty-dynamic in none" claim was confirmed correct.

---

## 2. Design decisions (D-EDF-* — mnemonic "EDF" = Empty-Dynamic-Fallback; collision-free: `[RUN]` repo grep `D-EDF` → 0 hits)

### 2.1 Row + subject confirmation: `tls-cvc-empty-dynamic-fallback` *(SELF-PICKED per the standing directive → row 68 registers at the stage-close commit)*

**The pick, and the FULL rejected roster adjudicated (not dismissed — the phase-67 one-liners are replaced with real reasoning, per C §2):**

| Candidate | Size | Reject-to-lift / observable move | Reference-pinnable | Verdict |
|---|---|---|---|---|
| **empty-dynamic CVC fallback (Design B)** — PICKED | 1 fn + 1 export, ~8–9 tasks | boot-FAIL → ACK-and-SERVE (rich three-arm cross-side contrast) | **YES — LIVE-PROBED this session** (Dossier A) | **PICKED** — richest observable, only candidate already reference-confirmed, substrate fully landed, cheaper NOW than at phase 66 (the sub-field-reject relocation phase 66 costed is ALREADY DONE — C §1) |
| missing `default:` arm (the `commonTLSContextToConfig` switch at config.go:367, rejecting the silently-ignored cert-provider oneof arms 10/12) | smaller | a reject-ADD only; arms 10/12 are `[#not-implemented-hide:]` + deprecated | **WEAK** — reference likely also ignores/handles specially; no clean cross-side move | **REJECTED** — smaller than B but fails "defensible": a low-value reject-add with weak differential observability. "Smallest" without a crisp cross-side contrast is not "smallest DEFENSIBLE" |
| OTLP-metrics stats sink | new sink — historically MULTI-LEG (cf. phase 47's split) | new sink, reuses the abstraction; narrows ROADMAP:198 | yes | **REJECTED for THIS row** — narrows a live sentence (a real point in its favor) but is a NEW sink, LARGER than B → violates smallest-first MORE than B. A strong FUTURE pick |
| `crl` | larger (new enforcement) | new field, no reject to lift | yes | deferred |
| compose-two CVC edge (`seen>1`) | reshape risk on 0103 accounting | edge of the CVC surface | partial | deferred |
| SDS rotation (`watched_directory` positive control first) | larger (new watch seam) | new machinery | yes | deferred |
| `ssl` stat family | framework surgery (ADR-0286 C3) | by-charter NOT-small | yes | deferred by charter |
| `xds-sds-upstream-server-cert` | boot-model reshape | — | — | **BLOCKED** (VALUE-level constructibility cycle; mechanism recorded phase-66 §2.1 — do NOT re-derive) |

**Why B over the smaller missing-`default:`-arm:** the standing directive is "smallest DEFENSIBLE first", and "defensible" in this project has always included **differential testability** — every row lands a fixture proving a cross-side contrast (ADR-0286 C3). The missing-default-arm produces no crisp cross-side move (both sides fail to authenticate on a hidden deprecated field), so it is weakly defensible despite being smaller. B produces a three-arm verdict (fallback-CA accepted · other-CA rejected · boot-succeeds-where-envoy-go-fails) and is ALREADY live-probed. B is the smallest candidate that is also crisply testable and reference-confirmed.

### 2.2 D-EDF-CLASSIFY — the seam signal: distinguish ACK-succeeded-but-EMPTY from fetch-failed *(EXECUTED, Dossier A arms 1 vs 3; RE-DERIVED, Dossier B §1 — the KEY open question for the SPEC)*

**EXECUTED (A):** the reference distinguishes the two SHARPLY. Arm 1 (default=CA_A, dynamic serves empty VC, require=true): `update_success=1`, `update_rejected=0`, `init_fetch_timeout=0`, listener LIVE, `downstream_context_secrets_not_ready=0` → **falls back to CA_A and serves** (CA_A accept, CA_B reject `unknown_ca`, no-cert reject `certificate_required`). Arm 3 (silent-but-reachable AND unreachable, identical): `init_fetch_timeout=1`, `update_success=0`, listener LIVE but **every connection fail-closed** (ClientHello written → 0 bytes + EOF, no `ssl.handshake`) → **no fallback on timeout**. The discriminating signal is **ACK/`update_success` (→ merge → fall back → serve) vs `init_fetch_timeout` (→ fail-closed)**.

**RE-DERIVED (B):** at the point `config.go:184` receives the fetch error, the two cases ARE distinct code paths but NOT cleanly separable — the empty case is produced by `parseValidationSecret` (secret.go:105) → nil `trusted_ca` → `dataSourceBytes` "none of inline_bytes…" default arm (secret.go:46) → wrapped `errValidation` in `applyValidationResponse` (stream.go:164); timeout takes `FetchInitialValidationContext`'s `ctx.Err()` arm (provider.go:115-117, a distinct error string). But `errValidation` (stream.go:33) is **unexported**, so `config.go` cannot `errors.Is` it.

**DECISION direction (SPEC settles the exact shape — this is the row's ONE place it could quietly grow):** expose the classification across the seam. Two shapes:
1. **Coarse** — export `errValidation` (or a helper `xds.IsValidationReject(err) bool`); fall back on ANY validation-reject. RISK: `errValidation` also covers corrupt-PEM / wrong-name / unsupported-sub-field, so envoy-go would fall back on a CORRUPT served `trusted_ca` too — which the reference may NACK rather than merge. Over-broad.
2. **Narrow (RECOMMENDED, SPEC-confirm)** — a new narrow sentinel `errEmptyValidationContext` wrapped ONLY at the `dataSourceBytes`-none site (secret.go:46 — the specifier oneof unset), so envoy-go falls back ONLY on the truly-empty (no-`trusted_ca`-specifier) case; a corrupt/unparseable served `trusted_ca` stays boot-FAIL. This matches the MERGE model faithfully (empty = contributes nothing; a present-but-broken value is a real error). **Wire-shape nuance (adversarial pass, §1.7 m2):** a `trusted_ca:{inline_bytes:""}` (specifier SET but empty) is a DISTINCT shape — it reaches `AppendCertsFromPEM`=false at secret.go:134-135 (a parse failure), NOT the `dataSourceBytes`-none arm at secret.go:46 — so under the narrow design it classifies as CORRUPT → boot-FAIL, not fallback. Whether that matches the reference is unprobed.

**OPEN QUESTION for the SPEC (D-EDF-CLASSIFY):** run a discriminating probe with THREE served shapes as separate arms (not run this session — Dossier A limit): (i) **no `trusted_ca` specifier at all** (the truly-empty case → confirmed fallback on the reference, A arm 1); (ii) **`trusted_ca:{inline_bytes:""}`** (specifier set, empty — m2); (iii) **a corrupt/malformed `trusted_ca`** (specifier set, unparseable PEM). If the reference NACKs/rejects (ii)/(iii) (does NOT fall back), the narrow sentinel scoped to (i)-only is MANDATORY; if the reference merges-and-drops a broken value too, coarse may suffice. Lean NARROW absent contrary evidence (safer: a set-but-broken anchor must never silently degrade to the default).

### 2.3 D-EDF-FETCHSHAPE — how the fetch reports "ACKed-empty" *(RE-DERIVED, B §1 — interacts with the phase-67 nil-pool guard)*

Today `FetchInitialValidationContext` returns `(*x509.CertPool, error)` and an empty VC yields an ERROR (not `(nil, nil)`); the phase-67 `installPool` guard treats a `(nil, nil)` as an error (config.go:89-92) precisely to foreclose the `VerifyClientCertIfGiven` + nil-`ClientCAs` → system-roots hazard (`reference_go_client_cert_withholding`). The fallback must NOT weaken that guard. **DECISION direction:** keep the empty case as a CLASSIFIED error out of the fetch (per §2.2), and let the CVC arm, on that classified error, take the fallback branch — which either installs the DEFAULT's non-nil pool (guard satisfied, assignment-adjacency preserved) or, if the default is also empty, reaches the no-anchor outcome (§2.5). The SPEC picks between (a) classified-error-then-fallback and (b) a fetch variant returning `(nil, empty=true, nil)`; (a) is preferred for leaving the phase-67 guard byte-intact.

### 2.4 D-EDF-MERGE-EQUIVALENCE — the P1–P5 theorem EXTENDS; the departure ADR-0287 excluded is CLOSED *(RE-DERIVED, B §3)*

The phase-66 equivalence theorem (config.go:137-164) proved substitution ≡ `MergeFrom` **on the success path**. Design B extends it to the empty path:
- **P1** (only `trusted_ca` honored) — UNCHANGED; the fallback honors only the default's `trusted_ca`.
- **P2** (DataSource oneof REPLACES) — UNCHANGED.
- **P3** (a successful fetch guarantees the specifier, so the default can never contribute) — **RESTATED**: the guarantee holds for a NON-EMPTY ACK; an EMPTY ACK is the branch where the default DOES contribute. The theorem becomes **`substitution-on-nonempty + default-fallback-on-empty ≡ MergeFrom` on the honored surface** — closing the very "empty-dynamic" departure ADR-0287 excluded.
- **P4** (the four sub-field rejects re-pointed at `default_validation_context`) — **NOT re-introduced as a hazard**: those rejects already run in `commonTLSContextToConfig` (config.go:442-461) BEFORE the switch, so the fallback only ever reaches a default carrying `trusted_ca` alone; a default demanding `match_typed_subject_alt_names` etc. already boot-FAILS upstream. The load-bearing premise phase 66 nearly omitted stays enforced.
- **P5** (the cross-seam `dataSourceBytes`/`loadDataSource` coincidence) — the hazard is REDUCED: the fallback reads the default via internal/tls's OWN `loadTrustedCAPool`, not across the seam. The MANDATORY P5 comment block (config.go:158-164) moves/updates with the CVC arm intact.

### 2.5 D-EDF-EMPTYBOTH — resolved-empty (default also empty) → serve UNAUTHENTICATED; the phase-67 require departure recurs *(EXECUTED, A arms 4a/5; a stated DECISION)*

**EXECUTED (A):** arm 4a (default `{}` + dynamic empty) and arms 5a/5b (plain top-level SDS-VC empty) → the reference ACKs, LIVE, **serves UNAUTHENTICATED** — ALL clients incl. no-cert ACCEPT 200; **`require_client_certificate: true` is SILENTLY NOT ENFORCED** (enforcement is contingent on a non-empty RESOLVED `trusted_ca`). This is the SAME phenomenon as phase-67's require=true-anchorless departure (BC B8): the reference's require flag is only live behind a real anchor.

**DECISION direction (SPEC pins):** for a CVC whose resolved anchor is empty on BOTH halves, envoy-go maps to the no-anchor outcome — `NoClientCert` at require=false/absent (traffic-identical to the reference), and at require=true the **RETAINED envoy-go-STRICT boot reject** (the reference silently serves unauthenticated; envoy-go refuses — strictly safer, the phase-67 departure family). This keeps the phase-67 nil-pool guard and the require=true-needs-an-anchor invariant intact. The exact routing (which reject substring, which arm) is a SPEC detail; the SEMANTICS is fixed here.

### 2.6 D-EDF-PLAINSDS — the plain SDS-VC empty-served case stays boot-FAIL (RETAINED departure) *(RE-DERIVED B §5; EXECUTED A arm 5)*

The plain `validation_context_sds_secret_config` arm (config.go:99-135) has NO inline default to fall back to; an empty served VC there has nothing to merge with. **DECISION:** it stays boot-FAIL — a RETAINED ADR-0280-family departure (the reference serves unauthenticated; envoy-go refuses). The fallback is **CVC-ONLY** by construction. Fully-inline and QUIC are unaffected. Stated as a named boundary in BC.

### 2.7 D-EDF-PGV — `default_validation_context` required is a PGV-only reference constraint *(EXECUTED, A arm 4b; `reference_pgv_forecloses_go_hazard`)*

**EXECUTED (A):** arm 4b (a CVC with NO `default_validation_context` at all) → the reference **PGV BOOT-REJECTS** "DefaultValidationContext: value is required" BEFORE any SDS fetch. envoy-go runs NO PGV (`reference_pgv_forecloses_go_hazard`), so this is a **deliberate-divergence decision, not automatic**. **DECISION direction:** the SPEC decides whether envoy-go adds an explicit reject for a CVC missing `default_validation_context` (matching the reference's boot-reject, strictly-safer, cheap) or leaves it as a silent gap. Lean toward ADDING the reject (it is the merge model's precondition — an empty-dynamic fallback with no default to fall back to is meaningless), but this is a stated SPEC call, probed here.

### 2.8 D-EDF-ABSENT / require interaction — verify-if-presented against the FALLBACK anchor *(EXECUTED, A arm 6/d)*

**EXECUTED (A):** with a real fallback CA (arm 1), `require_client_certificate` interacts exactly as phase 67 established, but against the RESOLVED (fallback) anchor: require=true enforces trust AND presence (CA_B rejected, no-cert rejected); require=false = verify-if-presented — CA_A accept, **CA_B still rejected** (forced-send proves a REAL verify against the fallback, not a vacuous accept), no-cert accept. The phase-67 three-way `ClientAuth` mapping applies unchanged to the fallback pool via `installPool`.

### 2.9 D-EDF-CERTPROVIDER / other-fields — unchanged silence *(RE-DERIVED)*

The ten non-`trusted_ca` `CertificateValidationContext` fields stay a SHARED silent gap (both sides) or a shared reject (the four re-pointed sub-field rejects). The cert-provider oneof arms (10/12) remain UNREAD. No change; the fallback touches only `trusted_ca`.

### 2.10 D-EDF-DRAGGED — semantics the fallback drags in, each a SPEC decision *(RE-DERIVED)*

1. Empty-served + non-empty default → fall back + serve (§2.2, the core). 2. Empty-both → resolved-empty outcome (§2.5). 3. Missing default → PGV-only, stated decision (§2.7). 4. Corrupt served `trusted_ca` → coarse-vs-narrow (§2.2, the growth risk). 5. Timeout/unreachable → RETAINED boot-FAIL (unchanged — the reference fail-closes, not falls back; A arm 3). 6. The nil-pool guard stays intact (§2.3).

### 2.11 D-EDF-FIXTURE — `0111-tls-cvc-empty-dynamic-fallback` on the 0109 chassis *(direction; SPEC pins)*

Anticipated ONE new fixture (fixtures 112 → 113). CVC, `default_validation_context.trusted_ca` = CA_A, dynamic SDS serves an EMPTY `validation_context`; per-side driver-owned `sdsserver.Server` serving the empty VC (arm-unique secret name + served-this-arm assert). Three-arm cross-side verdict at require=true (or a require pair): **CA_A-signed client (forced-send) → accept+echo · CA_B-signed (forced-send) → rejected (alert 48) · no-cert → rejected** — proving the fallback anchor is CA_A and LIVE (not a vacuous accept-all). Control (already covered by 0109): non-empty dynamic REPLACES the default. FORCED-SEND driver MANDATORY (`reference_go_client_cert_withholding`; phase-67 D-RCCF-FIXTURE). SDS-VC-empty and empty-both covered by unit tests + the probe pins. Discipline pins: one fixture dir = ONE runner branch; never treat a docker-proxy accept as listener liveness (A arm 3's trap); full selector `TestDifferential/0111-…`; `-count=1` on breaks; `Errorf` per property.

### 2.12 D-EDF-STREAMPOSTURE — the RESIDUAL divergence: envoy-go NACKs the empty resource where the reference cleanly ACKs *(RE-DERIVED B; EXECUTED A arm 1 — the adversarial pass, §1.7 M1)*

**A framing precision the adversarial pass caught:** the trigger is an EMPTY served `validation_context` that **the REFERENCE classifies as a clean ACK** (`update_success=1`, `update_rejected=0`, `errorDetail=<nil>` — A arm 1). **envoy-go TODAY classifies the SAME resource as `errValidation`** — a NACK-worthy reject: `parseValidationSecret` errors on the nil `trusted_ca` → `applyValidationResponse` wraps `errValidation` (stream.go:164) → a **NACK is sent on the wire** with the prior version kept, and `sds.update_rejected` increments (provider.go:112-114). So the "ACK-succeeded-but-empty" shorthand elsewhere in this doc names the REFERENCE's posture; on envoy-go's side the signal the fallback keys on is precisely this **`errValidation` classification** (§2.2 — the discriminator is sound regardless of the ACK/NACK framing).

**The RESIDUAL divergence, now NAMED:** the fallback recovers the LISTENER (it serves against the default CA, matching the reference's traffic behavior), but the SDS-STREAM posture still diverges — envoy-go NACKs + increments `update_rejected` where the reference ACKs + increments `update_success`. **DECISION owed at the SPEC (D-EDF-STREAMPOSTURE):** either (a) RECLASSIFY an ACKed-empty `validation_context` as a stream-level SUCCESS in envoy-go (so it ACKs like the reference — larger, touches `applyValidationResponse`/the classification path) while still yielding the empty/fallback signal to `config.go`; or (b) accept and NAME the NACK + `update_rejected` posture as a documented RESIDUAL departure (traffic-equivalent, stream-posture-divergent — the ADR-0280-family pattern). Lean (b) for a SMALL row (the traffic behavior — the differentially-observable surface — already matches; the stream-posture difference is not asserted by any fixture), but the SPEC MUST pick explicitly, not leave it silent.

### 2.13 D-EDF-STATS / D-EDF-FUZZSEED — +0 stats; +0 fuzzers (seeds only, dispatch-verified) *(RE-DERIVED B §6)*

**Stats +0** (surface stays 1201): the outcome proof is the cross-side accept/reject contrast (ADR-0286 C3); the `ssl.*` family is reference-only. `sds.*` lifecycle counters reused — but note D-EDF-STREAMPOSTURE (§2.12): under option (b) `sds.update_rejected` moves where the reference's `update_success` would, a stream-posture difference no fixture asserts. **Fuzz +0** (count stays 55): `FuzzTLSContextParse`'s `cvcFuzzProvider` (fuzz_test.go:31) always returns a VALID pool, so the empty-ACK path is not fuzz-reachable without a new provider mode; SEEDS only, and the SPEC must avoid the phase-66/67 downstream-side SDS-seed vacuity trap (the dispatch-side verification is owed at the SPEC).

---

## 3. Framework-survey result — a merge-completion on landed appliers; ZERO new packages/modules, ONE new `internal/xds` export

### 3.1 The change surface
`internal/tls/config.go` — the CVC arm's error branch (config.go:183-198): on a CLASSIFIED empty-ACK error, fall back to `loadTrustedCAPool(cvc.GetDefaultValidationContext(), baseDir, "downstream")` + `installPool`; the P1–P5 comment block updated (§2.4). ONE new exported symbol in `internal/xds` (the classifier — §2.2). Everything else UNTOUCHED.

### 3.2 NEW packages: NONE. go.mod modules: NONE ADDED (lineage figure stays 2). Re-check `git diff go.mod` after tidy anyway (`reference_new_subpackage_pulls_transitive_module`).

### 3.3 REUSES — §1.6.

### 3.4 The cycle guard STANDS (B §4)
The new export is a sentinel/helper in `internal/xds`; `internal/tls` already imports `internal/xds`, so the export adds NO import to `internal/xds` and creates no new edge. Re-verify `go list -deps ./internal/xds` (no `...`) at the IMPL — TYPE-level (`reference_xds_config_seam_transitive_cycle_guard`; the VALUE-level upstream-SDS cycle is a DIFFERENT, unrelated block).

---

## 4. Bootstrap-level applicability — the SDS pre-scan is ALREADY CVC-aware and require-agnostic

`boot.NewSDSProvider`'s pre-scan counts the CVC's inner SDS half without reading `require_client_certificate` (phase 66/67 established) — so the provider already exists for this shape. **`internal/boot` takes NO edit.** The change is entirely inside `internal/tls` (+ the one `internal/xds` export).

## 5. Stat surface + envelope hypothesis — +0 stats; the ONE new `internal/xds` symbol stated LOUDLY

+0 stats. The envelope's notable fact: **ONE new `internal/xds` exported symbol** — the FIRST break of the recent zero-new-symbol streak (DECISIONS.md:16973). It is minimal and behavior-neutral in `internal/xds`, but it IS a real envelope change the SPEC must call explicitly (not a silent relaxation). ADR-0287's Design-B pre-characterization ("a message-returning seam") was pessimistic (B §5): no message crosses the seam; the export is one classifier, not a new fetch signature.

## 6. Edit-site enumeration — cited by SYMBOL with `file:line` as of master `92cd1647` (SPEC re-derives)

**Production:** `internal/tls/config.go` — the CVC arm `case *tlsv3.CommonTlsContext_CombinedValidationContext:` (~:136) and its fetch-error branch (~:183-198) `[FALLBACK — the core]` · the P1–P5 block (~:137-164) `[UPDATE — P3 restated, P5 moved intact]` · the CVC pre-checks in `commonTLSContextToConfig` (~:420-461) `[VERIFY re-pointed rejects gate the fallback]` · `loadTrustedCAPool` (:298) `[REUSE]` · `installPool`/`clientAuthFor` (:78-96) `[REUSE — guard intact]`. `internal/xds` — `errValidation` (stream.go:33) `[EXPORT or add a narrow sibling — §2.2]` · `parseValidationSecret` (secret.go:105) / `dataSourceBytes` (secret.go:46) `[possible narrow wrap-site]`. **UNTOUCHED:** `internal/boot`, `validate/`, `internal/listener`, `test/helpers/sdsserver`.

**Test/harness:** new unit tests (empty-served → fallback; empty-both → resolved-empty; corrupt-served → boot-FAIL per §2.2; require cross-product) `[ADD]` · fuzz seeds `[ADD, dispatch-verified]` · `test/fixtures/0111-tls-cvc-empty-dynamic-fallback/` `[ADD]` · 0109 `[VERIFY unchanged]`.

**Docs:** BEHAVIOR_CONTRACT (the empty-dynamic Supported wording + the retained plain-SDS-VC/empty-both departures) · DECISIONS ADR-0290 (§Context at the SPEC) · ROADMAP/STATE/next-prompt `[STAGE-CLOSE]`.

## 7. Anticipated ADRs — 1 at the phase-68 IMPL: ADR-0290 (`tls-cvc-empty-dynamic-fallback`)

ADR-0290 (§Context drafts at the SPEC per ADR-0044; §Decision/§Consequences at the IMPL). It COMPLETES the ADR-0287 CVC lineage: substitution → substitution-plus-empty-fallback ≡ `MergeFrom` on the honored surface; the named departure ADR-0287/0289 filed is CLOSED for the CVC shape. Next-free after: ADR-0291.

## 8. Deferred items (the SPEC-67 §13 roster carried MINUS this row, PLUS what this row newly defers)

Carried: `xds-sds-upstream-server-cert` (VALUE-level cycle; do not re-derive) · SDS rotation (`watched_directory` positive control FIRST) · the `validation_context_type`/`commonTLSContextToConfig` switch's missing `default:` arm (§2.1 — adjudicated, deferred) · `crl` · the compose-two CVC edge (`seen>1`) · OTLP-metrics stats sink (the strongest FUTURE pick — narrows ROADMAP:198) · the `ssl` stat family (framework surgery) · HTTP/3 `QuicProtocolOptions` · DataSource `environment_variable` · tracing `custom_tags` `metadata` · CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc` · gRPC/Runtime/WASM family openers · QUIC client-auth · the cert-provider oneof arms (10/12) · a require=false RBAC/principal fixture arm · the ADR-0080 + ADR-0044 citation drifts.

**Newly deferred at THIS row:** the plain SDS-VC empty-served serve-unauthenticated behavior (envoy-go keeps boot-FAIL — §2.6, a retained departure, not a lift) · the corrupt-served-`trusted_ca` coarse-vs-narrow probe if the SPEC defers it (§2.2).

## 9. Cross-references against prior phases' deferred-items lists — pickup + sentinel maintenance

The pick is the phase-66/67 "Design B" runner-up (SPEC-66:267,509; SPEC-67:356) — a §13-tier pickup. **It is named in NO live `remaining deferred (not-yet-chartered) candidates:` sentence** (C §3, `[RUN]`: the three live sentences at ROADMAP:178/188/198 do NOT contain "empty-dynamic"/"Design B"). So **NO deferred sentence is narrowed at ANY stage of this row** (the phase-64/66/67 precedent — do NOT fabricate a narrow, `reference_sentinel_deferred_sentence_live_vs_historical`). Registering row 68 `in-progress` re-opens sentinel check (1); checks (2)+(3) stay as-is (three live sentences; `NEVER OPENED: gRPC/Runtime/WASM`) ⇒ the sentinel still does NOT fire.

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-EDF-CLASSIFY (the growth risk):** coarse `errValidation` export vs a narrow `errEmptyValidationContext` sentinel — run a discriminating probe with THREE served shapes as separate arms: (i) no `trusted_ca` specifier, (ii) `trusted_ca:{inline_bytes:""}` (set-but-empty — m2), (iii) a corrupt/malformed PEM. If the reference NACKs/rejects (ii)/(iii), the narrow sentinel scoped to (i)-only is MANDATORY. Lean NARROW. §2.2.
- **D-EDF-STREAMPOSTURE (the residual divergence):** reclassify the ACKed-empty resource as a stream-level SUCCESS in envoy-go (ACK like the reference — larger) vs accept + NAME the NACK + `update_rejected` posture as a documented residual departure (smaller — the traffic surface already matches). Lean the latter for a small row, but PICK EXPLICITLY. §2.12.
- **D-EDF-FETCHSHAPE:** classified-error-then-fallback vs a `(nil, empty=true, nil)` fetch variant — prefer the former (phase-67 nil-pool guard byte-intact). §2.3.
- **D-EDF-PGV:** add an explicit reject for a CVC missing `default_validation_context` (match the reference's PGV boot-reject) or leave a silent gap? Lean ADD. §2.7.
- **D-EDF-EMPTYBOTH-REQUIRE:** the exact routing of empty-both + require=true (which retained reject) — confirm the phase-67 require=true-needs-an-anchor invariant covers it. §2.5.
- **D-EDF-FIXTURE:** require pair vs single require value; whether to add an empty-both fixture arm or leave it to units. §2.11.
- **D-EDF-FUZZSEED:** the dispatch-side for an empty-ACK seed (a new `cvcFuzzProvider` empty mode?) — dispatch-verify to avoid the phase-66/67 vacuity trap. §2.13.

## 11. Prior-phase lessons applied

- `reference_quoting_is_not_executing` / `feedback_brief_citations_not_evidence` — every anchor re-derived at `92cd1647`; the reference behavior LIVE-PROBED, not cited from BC B18.
- The phase-66 lesson (a BRAINSTORM planned unbuildable `proto.Merge` because it CITED the seam return type) — this row EXPLICITLY confirms NO `proto.Merge` is needed and that the fallback reads the default via internal/tls's own loader (B §5); the "message-returning seam" pre-characterization is refuted by re-derivation.
- `reference_probe_must_discriminate` — arm 1 vs arm 3 (empty-served vs timeout) and the forced-send client (CA_B still rejected under the fallback) are DISCRIMINATING; the served-corrupt discriminator is NAMED as owed at the SPEC.
- `reference_pgv_forecloses_go_hazard` — the `default_validation_context`-required rule is PGV-only; recorded as a deliberate-divergence decision, not automatic.
- `reference_go_client_cert_withholding` — forced-send MANDATORY in 0111; the nil-pool guard stays intact.
- `reference_lifted_reject_hidden_enforcement` — the empty-ACK "reject" being lifted here (the fetch-error boot-FAIL for the empty case) is checked for hidden enforcement: it only ever gated the CVC empty case; timeout/plain-SDS-VC keep their boot-FAIL (§2.6, §2.10 item 5).
- `feedback_git_worktrees` / `feedback_execution_style` / `reference_parallel_subagents_private_scratch` — fresh worktree; three parallel input agents in private scratch; adversarial verification before landing.

## 12. Section closeout

Phase 68 completes the CVC lineage: honor the reference's `MergeFrom` on an ACKed-empty dynamic half by falling back to `default_validation_context.trusted_ca` and serving — closing the named ADR-0280/0287/0289 departure for the CVC shape. LIVE-PROBED (Dossier A): empty-served → fall back + serve; timeout → fail-closed (no fallback); empty-both → serve unauthenticated; missing default → PGV-only. Constructible (Dossier B): ONE production function + ONE new `internal/xds` export (the streak break, stated loudly); CVC-only; ~8–9 tasks; +0 packages/modules/stats/fuzzers/BackendKinds; fixtures 112 → 113 (`0111`). Adjudicated (Dossier C): B is the smallest DEFENSIBLE candidate (crisp cross-side observable + already reference-confirmed); the smaller missing-`default:`-arm loses on differential value, OTLP-metrics loses on size. The SPEC re-derives, runs the served-corrupt discriminator, and settles D-EDF-CLASSIFY/FETCHSHAPE/PGV. Row 68 registers `in-progress`; the sentinel does NOT fire.
