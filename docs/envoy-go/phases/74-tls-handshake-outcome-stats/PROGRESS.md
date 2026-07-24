# PROGRESS 74 — downstream TLS handshake-outcome stats (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert`)

> Live task ledger for the phase-74 IMPL. `PLAN.md` is the spine; **this file records what ACTUALLY happened per task** — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions **and whether their stated rationale survived scrutiny**, T3's Break D result (the QUIC registration discriminator), T4's Break C crash attribution, T6's Break G′ dispatch result, and every PLAN claim refuted by execution. A break that does not fire is a FINDING, not a nuisance.

## Stage pointer

- **PLAN done** (2026-07-24; flipped only AFTER the §Adversarial-pass record below was populated from the verifiers' actual reports — writing "done" over an open gate is the exact class this stage's own V2 caught in an earlier draft). The **9-task SINGLE-FLAT-ROW TDD spine (T1–T9)** landed; every SPEC §3/§7/§8/§9/§11/§12/§14 anchor RE-DERIVED at `ab13fc19` by four read-only agents plus controller re-verification. Docs-only: `PLAN.md` + `PROGRESS.md` are the ONLY two files in the phase-directory delta — **ZERO production `.go`; ROADMAP and DECISIONS UNTOUCHED; row 74 STAYS `in-progress`.** Worktree `.worktrees/phase-74-plan` off master `ab13fc19`, branch `phase-74-plan`. Sentinel re-run MECHANICALLY TWICE (worktree + landed master post-push): does **NOT** fire; `stop` NOT created. Counts UNCHANGED (fixtures 119 · fuzzers 55 · BackendKind 38 · stat surface 1201 · DECISIONS tail ADR-0296 PROPOSED). **Next → the phase-74 IMPL.**

- **IMPL** — *(pending; populate at the IMPL close)*

## Adversarial-pass record (PLAN §1.4)

*(Written AFTER the pass, from what the verifiers actually found — **never asserted in advance over a placeholder**. ⚠️ An earlier draft of `PLAN.md` did exactly that: it shipped `STATUS: COMPLETE` citing THIS file before this file existed, having deleted the phase-73 sentence that forbids it. V2 caught it as its own SEVERE-1. Recorded here rather than quietly fixed, because it is the same species as the row's own thesis.)*

**STATUS: COMPLETE.** Two verifiers, disjoint remits, both in PRIVATE scratch (`reference_parallel_subagents_private_scratch`), both against this branch's tip.
**V1 (code claims, BY EXECUTION — built T1→T4 from the PLAN's skeletons pasted VERBATIM and ran every break): 2 SEVERE, 2 MODERATE, 4 MINOR.**
**V2 (process, consistency, SPEC-coverage, stage-close mechanics — re-ran ~120 anchors): 1 SEVERE, 5 MAJOR, 11 MODERATE, 10 MINOR.**

**V2's verdict on the ledger, which cuts both ways:** *"every one of the PLAN's twelve claimed corrections of the SPEC holds … That earned trust is what makes S1 disqualifying."*

**The SIX that changed the IMPL's instructions** — full text in `PLAN.md` §1.4:

| # | Verifier | What it caught |
|---|---|---|
| **S1** | V1 (SEVERE) | **`net.Pipe()` DEADLOCKS.** T1's live-handshake helper could not work as written — the no-cert arm hangs to a 45s panic timeout with NO failing assertion, because `net.Pipe` is unbuffered and both sides block writing. Structural. ⇒ **loopback TCP pair**; T1 then passes in 0.008s. |
| **S2** | V1 (SEVERE) | **Break F FIRED at the scope originally given** — the one break declared "must not fire" would have tripped the stage-stopping rule on a false alarm, because `TestClassifyHandshakeErr_TLS12` carries its own live no-cert arm. ⇒ Break F now covers BOTH arms; its "mutate in both places" sub-step corrected. |
| **S1** | V2 (SEVERE) | **PLAN §1.4 self-certified a completed adversarial pass over a `PROGRESS.md` that did not exist**, having deleted phase-73's explicit guardrail against exactly that. ⇒ guardrail restored verbatim; §1.4 populated from real reports. |
| **M1** | V2 (MAJOR) | **T7's own verification gate was unsatisfiable by T7's own instruction** — Step 3 mandates a ledger line containing `1201` while Step 5 gated on `grep '1201' ⇒ 0`. ⇒ both gates rewritten (expect exactly 1; scoped `awk` for the pre-ledger region). |
| **M2** | V2 (MAJOR) | **The FOURTH stale QUIC site, in the very cell T9 opens** — `ROADMAP.md:136` still says *"leaving QUIC handshakes uncounted is a DEPARTURE that must be written down"*. Plus *"CROSS ADRs — a first"* (refuted) and a self-inconsistent split-family count. ⇒ T9 Step 4 now names all FOUR. |
| **M3** | V2 (MAJOR) | **Break H was contradictory and would have killed the fixture before `AssertStats` ran** — `mtlsEcho` has no `side` param (so the cross-side leg could never fire) and deleting `sendForced` also strips the trusted arm ⇒ `structuralCheck` fails ⇒ `t.Fatalf` in the DRIVE, step 10 never executes. ⇒ rewritten as a `sendPolite` mode on one side only. |
| **M2** | V1 (MODERATE) | **`TestListenerMetrics_GateMatchesInc` could not detect the failure it exists to prevent** — both its predicates are set at BUILD time, upstream of registration; under Break E it STAYED GREEN. ⇒ legs (b)/(c) now assert the three `*stats.Counter` pointers themselves. |

**Also folded:** V2-M4 (T6's cross-side check is ENTAILED — kept as a redundant tripwire, but no longer reported as an independent property) · V2-M5 (placeholder count 3 → **11**) · V1-M1 (Break F does not compile: `declared and not used: liveNoCertErr`; substitution now named in advance) · **V2-Mo1 (`SPEC §3.10` DOES NOT EXIST** — SPEC runs §3.1–§3.7; cited three times in the single most load-bearing instruction; all repointed to ADR-0296 §Context ¶8(ii), `DECISIONS.md:17274`) · V1-m2/V2-Mo2 (Break A fires **FIVE** rows, not four) · V2-Mo3 (E′ and C′ were missing from the break map; "extras" was 3, actually 5) · V1-m3 (`counterValue` must `t.Errorf` on an absent name or T3 assertion (4) is vacuous under Break D) · V2-Mo4 (T5's test file was a TBD in disguise → `name_test.go:217-218`) · V2-Mo5 (`quic_test.go:121-126`, not `:106`) · V2-Mo6 (`sort` and `reflect` missing from both the roster and the files) · **V2-Mo7 (the identifier roster missed six names; `manager_test.go:559` already carries a doc comment for a DIFFERENT `testPKI` ⇒ renamed `handshakeTestPKI`; a naive `counterValue` re-run returns 120 repo-wide and must not read as a collision)** · **V2-Mo8 (`ssl.no_certificate` — which SPEC §13.1 asserts is recorded in ADR-0296 §Context — is in NEITHER the ADR nor any B-step; `grep -c` over the ADR block ⇒ 0)** · V2-Mo9 (the coverage walk skipped §1/§2/§13) · V2-Mo10 (five operative memories uncited) · V2-Mo11 (File-structure missing four edited files) · V1-m1 (the T2 field block was not gofmt-clean — the PLAN's own Step-5 gate failed on its own paste) · V2-m1 (ADR-0044's headings are `:1426`/`:1430`) · **V2-m2 (RD-CORRFORM's arrow was falsified by its own evidence: `:17209` IS later-phase-corrects-earlier-ADR and uses the INLINE form ⇒ the discriminator is the ADR FAMILY, not the phase gap)** · V2-m3 (RD-HELPTEXT's "zero `ssl` substring" is self-refuting — 3 hits inside `acce**ssl**og_dropped`) · V2-m4/m5 (`:973`; one anchor spelled two ways) · V2-m6 (FOUR subject-only `AssertStats` shapes, not two) · V2-m7 · V2-m8 (check (1)'s blind spot had been dropped) · V2-m9 (nested backticks in commit messages trigger command substitution) · V2-m10 (no command for "ZERO new exported symbols").

**Two findings ACCEPTED AS-IS, reasoning recorded rather than instruction changed:**
- **V1-m4 — Deviation #2 (`outcomeOK` over the SPEC's bare `ok`) is a STYLE call, not a forced one.** V1 renamed all four to the SPEC table's bare names: `go vet` rc=0, `golangci-lint` rc=0, tests `ok 0.166s`. A package-level `ok` is legally shadowed by every `v, ok := m[k]`. The deviation stands on readability grounds and now says so.
- **V2-M4's cross-side leg is retained** despite being entailed — three lines, survives a refactor to per-side `want` values, makes the cross-side claim legible at the call site.

### What V1 CONFIRMED by execution (carry as evidence — this is what the IMPL does NOT have to re-derive)

- Task 1's code block **compiles verbatim** and classifies every table row correctly.
- The live no-cert string is exactly `tls: client didn't provide a certificate` **at BOTH TLS 1.3 and TLS 1.2**; the live untrusted arm really is `*tls.CertificateVerificationError` with `len(UnverifiedCertificates)==1` at both; **the no-cert arm is NOT a CVE**, so the `errors.As`-first ordering holds.
- Break F′ fires exactly the two live no-cert rows and nothing else — **F/F′ is a real cross-product.**
- `registerListenerMetrics` **sees `rt.tlsMode` already set** (RD-TLSMODE-ORDER). A TLS listener registers exactly the three names with exact strings; a plaintext listener registers **ZERO**; the charset guard passes for both the IPv4 and IPv6 forms.
- Break E′ fires the name-set assertion — **showing exactly what a cardinality guard would miss.**
- The QUIC test passes on arrival (H3 `status=200 proto=3`), `downstream_cx_total` moves, all three `ssl.*` stay 0. **Break D fires assertion (1)** while TLS and plaintext stay green. **Break E's second half: the QUIC test does NOT fire** — not entangled; the stop-condition is not triggered.
- All three T4 arms move exactly the predicted counter, cross-products hold. **The untrusted arm REALLY needs `GetClientCertificate`** — run both ways: forced-send ⇒ `fail_verify_error=1`; `Certificates:`-only ⇒ `fail_verify_no_cert=1`. The collapse is real.
- **Break B fires in EXACTLY TWO of three.** **Break C is a PROCESS CRASH with the predicted stack** — `stats.(*Counter).Inc` at `counter.go:22` under `listener.(*listenerRuntime).serveConnection`, `created by ...acceptLoop`. **F3 confirmed verbatim; C′ was not needed.**
- **"+0 production imports" HOLDS** — four hunks in `manager.go`, none in the import block; `go.mod`/`go.sum` no-diff.
- **F1 and F2 HOLD** — no PKI in the corpus, no helper missed; `mkDownstreamTSRequireClientCert` boot-rejects with `require_client_certificate=true requires validation_context.trusted_ca`.
- **RD-SN3 holds by execution** — `flattenToProm("listener.0_0_0_0_10000.ssl.handshake")` ⇒ `envoy_listener_ssl_handshake` + label. **D7 holds** — the two-metrics test PASSES with +3, its name going silently false with no red.
- Full regression green including `-race`; no pre-existing failures surfaced.

### ⚠️ NOT verified — the IMPL inherits no false confidence

- **Nothing reference-side was executed at this stage** — no Docker, no Envoy probes. Every parity claim (including the whole justification for `rt.tlsMode` alone) rests on the SPEC's probe record; V1 verified only that *envoy-go* behaves that way.
- **T5–T9 were never executed.** The `0111` `AssertStats` **has still never been written or run**; F6/F7/F8/F9 and Breaks **G, G′, H** remain read-derived.
- V1 worked in sibling `phase74_t*_test.go` files, **so the T2 rename and the `:1911-1927` doc rewrite were never exercised in place.**
- Whether `net.Pipe` also deadlocks in the *untrusted* arm is unknown — the no-cert arm hangs first.
- The SPEC's own unresolved list is untouched: session resumption, the undriven `fail_verify_san`/`fail_verify_cert_hash`/`was_key_usage_invalid`/`ocsp_staple_*`, `ssl.connection_error`'s membership, listener-vs-filter-chain keying reference-side, and the multi-`tls_certificate` `e3b0c442` collision risk.

## Task ledger (Status/Commit filled at the IMPL)

| Task | Status | Commit | Red-first / breaks / notes |
|---|---|---|---|
| T1 classifier (`handshakeOutcome` + `classifyHandshakeErr` + the LIVE-handshake no-cert case) | *(pending)* | | Step 0 re-runs the collision greps **plus the parallel-stream check**. Red: `undefined: classifyHandshakeErr` / `handshakeOutcome` / `outcomeOK`. ⚠️ **Loopback TCP pair, NEVER `net.Pipe` (V1-S1 — it deadlocks silently to a 45s timeout).** ⚠️ Untrusted arm MUST force `GetClientCertificate`. ⚠️ `context.Background()`, never a deadline ctx. Breaks **A** (five rows, not four), **F** (MUST NOT fire — scope BOTH live no-cert arms; delete the unused `liveNoCertErr` binding), **F′** (mutation control). |
| T2 fields + `rt.tlsMode`-gated registration + the NAME-SET guard | *(pending)* | | ⚠️ **GATE IS `rt.tlsMode` ALONE — NO KIND CHECK** (SPEC `:1`/`:315`/`:320` stale-carry the refuted reading). Red: three name-set assertions with empty `got`; the plaintext test passes on arrival. ⚠️ `GateMatchesInc` MUST assert the three **counter pointers** (V1-M2 — the build-time predicates stay green under Break E). ⚠️ Field block must be gofmt-aligned. D7 rename covers **`:1911-1927`**. Breaks **E**, **E′**. |
| T3 QUIC registration test (Break D's target) | *(pending)* | | Passes on arrival — **that is why the break ships with it.** ⚠️ `counterValue` must `t.Errorf` on an ABSENT name or (4) is vacuous. ⚠️ Assert only `downstream_cx_total` (RD-QUICTEST — the gauge half is unpinned and there is no gauge poller). Assertion (3) is what makes (4) non-vacuous. `quic.go` sha256-UNCHANGED. Breaks **D**, **E second half** (QUIC must NOT fire). |
| T4 the two Inc points + three increment tests | *(pending)* | | Red: all three counters at 0. Needs `mkDownstreamTSMutualTLS` — ⚠️ `mkDownstreamTSRequireClientCert:644` boot-REJECTS (F2). Each test asserts the **cross-product**. Breaks **B** (exactly two of three), **C** (⚠️ **a PROCESS CRASH — confirm the stack names `stats.(*Counter).Inc` under `serveConnection`**), **C′** (only if attribution is ambiguous). `-race` mandatory. |
| T5 `helpText` ×3 + the `:445-451` doc | *(pending)* | | Test at `name_test.go:217-218`. ⚠️ HELP text says *"certificate chain verification failed"*, NOT *"client certificate rejected"*. Doc comment 11 → 14 entries (F4). |
| T6 `0111` `StatsAsserter` + FOUR boundary retirements + RD3 | *(pending)* | | ⚠️ **`var _ fixture.StatsAsserter = (*edfDriver)(nil)` MANDATORY.** ⚠️ ABSENT check SEPARATE, with `continue` (the 0055 shape, NOT 0005's zero-defaulting snapshot). ⚠️ Each boundary note BUNDLES a live `/listeners` guard — do not blanket-delete. Cross-side leg is ENTAILED (V2-M4). Breaks **G**, **G′** (the dispatch break — proves it runs at all), **H** (⚠️ rewritten: a `sendPolite` mode on ONE side; the naive form kills the trusted arm and step 10 never runs). |
| T7 BEHAVIOR_CONTRACT B1–B8 | *(pending)* | | ⚠️ B4 is **SHARED PARITY, not a departure**. ⚠️ B7: `1201`→1204 at `:831`/`:847`; the **three narrative** stale `1200`s only — **DO NOT touch the other ten**, which are historically correct. New ledger line after `:4988`. The `1200 → 1201` hole is **RECORDED, not invented**. ⚠️ Step 5's gate expects `grep -c '1201'` ⇒ **1**, not 0 (V2-M1). |
| T8 verify (six-gate + envelope) | *(pending)* | | Envelope audited in **TWO categories** — production +0 imports vs test imports which DO grow (incl. `sort`/`reflect`). "ZERO new exported symbols" asserted by `go doc -all` set-diff. BYTE-UNTOUCHED roster set-differenced against the EDIT roster (V2: EMPTY intersection at the PLAN). |
| T9 ADR-0296 + ADR-0286 C3 + ROADMAP + close | *(pending)* | | ADR-0296 completed IN PLACE after the RETAINED `:17280` footer; mirror **ADR-0295, not ADR-0286**; `### Decision` 0 before / 1 after. Fix the `:17256` ¶6→¶4(i) mis-pointer and ¶3's **self-refuting grep** (RD-GREP0 — it is now 1, and the SPEC's own append caused it). C3 correction as an INDENTED blockquote (`:16901` form, two literal spaces). ⚠️ **ROADMAP `:136` carries FOUR stale claims**; the narrow is on **`:204`**, not `:202`, and must ADD `curves`/`sigalgs` **and REPLACE THE PROSE**. Record `ssl.no_certificate` (V2-Mo8). Check (2) must STAY **3**. |

## Findings carried from the PLAN (RE-DERIVED at `ab13fc19`; RE-VERIFY at the IMPL tip)

**Every finding below is itself a claim** (`reference_a_drift_correction_is_itself_a_claim`) — re-run the grep before any of them becomes an `old_string`.

- **RD-QUICSTALE (the one that would have broken the IMPL)** — `SPEC.md:1`, `:315`, `:320` carry the REFUTED "gate QUIC out" reading against the SPEC's own §3.4/§16. V2 confirmed the sweep is exhaustive **within `SPEC.md`**; the FOURTH site is **`ROADMAP.md:136`**. `STATE.md` is CLEAN.
- **RD-CTXBOUND** — the SPEC's premise (*"ZERO `context.WithTimeout` in production"*) is FALSE (`listenerfilter/pipeline.go:43`); the conclusion survives because `Pipeline.Run` returns only an `error` and `serveConnection` never rebinds `ctx`. **The code is one refactor away from the hazard.**
- **RD-GREP0** — the SPEC's `grep -c 'VerifyPeerCertificate\|handshake-error callback'` ⇒ 0 is now **1**, the sole hit being the ADR-0296 paragraph that asserts it is 0. **Do not read 1 as drift.**
- **F1/F2/F3** (SEVERE) — no mTLS PKI in the corpus · `mkDownstreamTSRequireClientCert` boot-rejects · a nil `*stats.Counter`.Inc in a goroutine with no `recover()` is a **process crash**.
- **F4–F10** — the `helpText` doc comment, the `:1911-1927` doc span, `AssertStats`'s two-argument reality, the 28-fixture Prometheus precedent and its four subject-only fallbacks, `0111`'s absent listener `stat_prefix`, the `0055` `continue` shape, and "BackendKind tail 38" being a tail value not a count.

### Cross-cutting hazards (bind every task)

- **A break that does not COMPILE proves nothing**; a break that fires the WRONG assertion proves nothing either; **a substitution's RATIONALE is itself a claim.** ⚠️ **And a break that is declared "must not fire" can fire for a scope reason** — V1-S2 is exactly that, caught before the IMPL.
- **Confirm WHICH assertion fired — including for PANICS.** Break C's firing is a process crash; its stack is the attribution.
- **`-count=1` on every differential break**; **breaks run AFTER committing**; full `-run` selector only.
- **Known PRE-EXISTING flakes — never reflex-classify as phase-74 regressions:** `internal/cluster` `-race` (`outlier_test.go:766`), the full-suite startup flake, the SDS dial-budget flake, and the two unindexed load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`) — isolate-green, NOT root-caused.
- **Controller worktree hazard** (`reference_bash_cwd_reset_commits_to_main`): `git -C <abs-worktree-path>` everywhere; tripwire `pwd` + branch (**NEVER `master`**) + commit count before any commit or gate run.

---

# IMPL record

*(populated at the phase-74 IMPL close — per-task outcome, the break ledger with every break's ACTUAL firing assertion, PLAN refutations found by execution, and the six-gate results)*
