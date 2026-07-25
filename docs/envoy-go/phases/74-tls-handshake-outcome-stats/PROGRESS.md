# PROGRESS 74 — downstream TLS handshake-outcome stats (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert`)

> Live task ledger for the phase-74 IMPL. `PLAN.md` is the spine; **this file records what ACTUALLY happened per task** — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions **and whether their stated rationale survived scrutiny**, T3's Break D result (the QUIC registration discriminator), T4's Break C crash attribution, T6's Break G′ dispatch result, and every PLAN claim refuted by execution. A break that does not fire is a FINDING, not a nuisance.

## Stage pointer

- **PLAN done** (2026-07-24; flipped only AFTER the §Adversarial-pass record below was populated from the verifiers' actual reports — writing "done" over an open gate is the exact class this stage's own V2 caught in an earlier draft). The **9-task SINGLE-FLAT-ROW TDD spine (T1–T9)** landed; every SPEC §3/§7/§8/§9/§11/§12/§14 anchor RE-DERIVED at `ab13fc19` by four read-only agents plus controller re-verification. Docs-only: `PLAN.md` + `PROGRESS.md` are the ONLY two files in the phase-directory delta — **ZERO production `.go`; ROADMAP and DECISIONS UNTOUCHED; row 74 STAYS `in-progress`.** Worktree `.worktrees/phase-74-plan` off master `ab13fc19`, branch `phase-74-plan`. Sentinel re-run MECHANICALLY TWICE (worktree + landed master post-push): does **NOT** fire; `stop` NOT created. Counts UNCHANGED (fixtures 119 · fuzzers 55 · BackendKind 38 · stat surface 1201 · DECISIONS tail ADR-0296 PROPOSED). **Next → the phase-74 IMPL.**

- **IMPL done** (2026-07-24). **ROW 74 FLIPS `in-progress` → `done`** at the T8/T9 six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW). Worktree `.worktrees/phase-74-impl` off master `f1221a4a` (the phase-74 PLAN squash), branch `phase-74-impl`. **Nine subagent-driven tasks, nine local commits, squashed at close.** All T1–T9 landed; **all TWELVE breaks run, every one with its predicted outcome** (Break F, the one declared *must not fire*, did not fire). Six-gate FULLY GREEN incl. the **full 119-fixture differential (402s)** and `go test ./... -race`. Stat surface **1201 → 1204**; +0 fixtures (119) · +0 fuzzers (55) · +0 BackendKinds (38) · +0 modules · **+0 PRODUCTION imports** · ZERO new packages · **ZERO new exported symbols** (asserted by `go doc -all` set-diff, not by inspection). ADR-0296 COMPLETED IN PLACE (PROPOSED → COMPLETE; tail stays ADR-0296, next-free ADR-0297); ADR-0286 §Consequences C3 corrected via the indented `:16901`-form blockquote. Sentinel re-run MECHANICALLY TWICE (worktree + landed master post-push): **does NOT fire; `stop` NOT created** — check (1) went SILENT as predicted, but (2) ⇒ **3** and (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **Next → the phase-75 BRAINSTORM** (roller SELF-PICKS).

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

*(populated at the phase-74 IMPL close from what the tasks ACTUALLY reported — never predicted. Every break below was RUN; every firing set is the observed one, not the PLAN's forecast.)*

## Task ledger — ACTUAL

| Task | Commit | Result |
|---|---|---|
| **T1** classifier | `c0892f57` | Red: `undefined: handshakeOutcome` / `outcomeOK` / `outcomeNoCert` / `outcomeVerifyError` (build failure). Green in **0.008s** with the loopback `connPair` — the PLAN's own figure, reproduced. Live no-cert string confirmed at BOTH TLS versions; live untrusted arm is a `*tls.CertificateVerificationError` with `len(UnverifiedCertificates)==1`; the no-cert arm is NOT a CVE. **+0 production imports** (one hunk, `@@ -354,6 +354,59 @@`, no import block). |
| **T2** fields + `rt.tlsMode` gate + NAME-SET guard | `37951982` | **Two-stage red** (see F-2 below). Gate is `rt.tlsMode` **ALONE**. Field block gofmt-aligned on paste. D7 rename landed with the full `:1918-1934` doc rewrite. |
| **T3** QUIC registration test | `1b413d84` | PASS on arrival (as designed — the break is what makes it evidence). `quic.go` sha256 `071cb7a5…0edb` — **identical to master**. Only the cx COUNTER asserted; the gauge half left honestly unpinned. |
| **T4** the two Inc points | `e56e2087` | Red: all three counters at 0 (assertion-level, no staging needed). `-race` green. One hunk, no import block. |
| **T5** `helpText` ×3 | `64e37e7d` | Red: three `helpText missing entry for …`. Entry count **11 → 14**, derived mechanically. |
| **T6** `0111` cross-side `StatsAsserter` | `f632945b` | **PASS. The reference emitted `handshake=1 fail_verify_error=1 fail_verify_no_cert=1`, `downstream_cx_total=3` — matching the subject EXACTLY.** The cross-side leg is real; the subject-only fallback was **not** needed and was never taken. `envoy-go.yaml` sha256 unchanged. Fixtures stay **119**. |
| **T7** BEHAVIOR_CONTRACT B1–B8 | `dc245426` | 5730 → 5744 lines (+14). Gates: `1201` ⇒ **1** · `1201` at `NR<4980` ⇒ **0** · `1204` ⇒ **3** · `\b1200\b` ⇒ **10** (was 13). The ten historically-correct `1200`s untouched. |
| **T8** six-gate | *(no commit)* | **ALL GREEN.** `gofmt -l .` silent · `go vet ./...` · `go build ./...` · `go mod tidy -diff` EMPTY · `git diff master -- go.mod go.sum` EMPTY · `golangci-lint run ./...` · **full differential `ok 402.512s`, 119 subtests, 119 PASS, 0 SKIP, 0 FAIL** (fixture-dir set ↔ subtest set `comm -3` EMPTY). `-race`: `./internal/listener ./internal/stats` green; `./... -race` green on re-run (one pre-existing flake, evidenced below). `go list -deps ./internal/listener`: **439 packages both sides, diff IDENTICAL**. sha256 roster: **569 files checked, 0 mismatches**; EDIT ∩ GATED = **EMPTY**. |
| **T9** ADR-0296 + C3 + ROADMAP + close | `71ca7d7f` | ADR-0296 `### Decision` **0 before / 1 after**, `### Consequences` likewise; STATUS PROPOSED → COMPLETE; footer RETAINED; tail stays **ADR-0296**, `^## ADR-0297` ⇒ **0**. C3 blockquote indentation `cat -A`-verified byte-identical to `:16901`. Row 74 → `done`. |

## Break ledger — ALL TWELVE RUN, every one as predicted

| Break | Predicted | **ACTUAL** |
|---|---|---|
| **A** `other` → `verifyError` | 5 rows fire | **FIRED, exactly 5**, names matching the PLAN's list exactly; the four cert-ish rows and `_TLS12` stayed green. |
| **F** hand-written string — **MUST NOT FIRE** | stays green | **DID NOT FIRE.** Green with the const mutated one character — the demonstration that the hand-written form is self-consistent and therefore worthless. |
| **F′** mutate the const, LIVE construction | 2 rows fire | **FIRED**, exactly the TLS-1.3 and TLS-1.2 live no-cert rows. F+F′ is a real cross-product. |
| **E** gate dropped | plaintext fires, TLS stays green | **FIRED.** ⚠️ **And `GateMatchesInc` fired EXCLUSIVELY through the three counter-pointer assertions** — the `tlsMode`/`tlsCfg` predicates printed nothing and `/tls_listener` passed outright. **V1-M2 reproduced and confirmed: the pointer half is load-bearing; the build-time-predicate version would have stayed green.** |
| **E′** misspell one name | name-set fires | **FIRED.** Counterfactual recorded: the misspelled slice still has **length 3**, so a `len(got)!=3` or `countMetrics`-style cardinality guard — the landed `statssink` precedent — **would have been GREEN**. |
| **D** ADD the refuted `kind != kindQUIC` gate — **the row's DISTINGUISHING break** | QUIC (1) fires; TLS + plaintext green | **FIRED, and assertion (1) — the `reflect.DeepEqual` NAME SET — is the FIRST and discriminating failure.** The three `counter … is not registered` lines from `counterValue` appeared as predicted. `driveH3`'s precondition SUCCEEDED, so the break did not fire by killing the drive. |
| **E, second half** | plaintext fires; QUIC must NOT | **Plaintext fired; QUIC stayed PASS.** The two tests are **not entangled** — the stop-condition was not triggered. |
| **B** swap the two failure arms | exactly 2 of 3 | **FIRED in EXACTLY TWO of three**, success arm green — and **both halves of each cross-product fired** (the `= 0, want 1` positive AND the `= 1, want 0` negative), which is the discriminating signature. |
| **C** Inc outside the TLS block | PROCESS CRASH | **CRASHED, with the exact predicted stack:** `sync/atomic.(*Uint64).Add` → **`stats.(*Counter).Inc` `counter.go:22`** → **`listener.(*listenerRuntime).serveConnection` `manager.go:1283`** (the relocated Inc itself), `created by …acceptLoop`. F3 confirmed verbatim. |
| **C′** non-crashing variant | only if C is ambiguous | **NOT RUN, NOT NEEDED** — the panic frame IS the Inc site and `-run` isolated the crashing test, so attribution was never ambiguous. Running it would have replaced a stronger proof with a weaker one. |
| **G** break one asserted counter | stats assertion fires | **FIRED**, and it is the STATS assertion (`ref/subj envoy_listener_ssl_fail_verify_error = 1, want 2`) — **no `CompareBytes` hex dump anywhere**. |
| **G′** the dispatch break | fixture stays green | **STAYED GREEN — THE FINDING.** With the method renamed the two `0111 AssertStats:` log lines vanished entirely: hard proof the assertion never ran. Restoring only the method name (leaving `var _` commented) went RED ⇒ **`var _ fixture.StatsAsserter` is a TRIPWIRE, not the dispatch mechanism.** ⚠️ **Substituted — see below.** |
| **H** `sendPolite` on the untrusted arm, subject side only | `fail_verify_error` → 0, `fail_verify_no_cert` → 2 | **FIRED, exactly as predicted**: subject `handshake=1 fail_verify_error=0 fail_verify_no_cert=2`; four assertions fired; **`CompareBytes` stayed GREEN.** The executable proof that the RD3 forced-send disclaimer **INVERTS at the counter layer**. |

**One substitution, with its rationale — and the rationale is itself a claim, so it is stated for review.** **The PLAN's literal Break G′ is VACUOUS.** As written (rename the method, leave `want` at the *passing* value 1) a green result is consistent with BOTH "the assertion never ran" AND "the assertion ran and passed" — it discriminates nothing, which is the exact `reference_probe_must_discriminate` failure. **The PLAN's own parenthetical for G′ applies recursively to G′ itself.** Substitution: run G′ **stacked on Break G** (`want: 2`, i.e. a *failing* assertion) so green can only mean "never invoked", then restore only the method name for the other half. Result is a clean 2×2: broken-and-renamed ⇒ green; broken-and-named ⇒ red.

## PLAN/SPEC claims REFUTED BY EXECUTION at this stage

1. **The check-(1) BLIND-SPOT figure was stale in the PLAN itself** — it recorded *"104 table rows, 102 matched, TWO misses"*. Re-derived at the IMPL tip: **106 data rows, 102 matched, FOUR misses** — `| 00 | bootstrap | — |` (em-dash column), `| 04 | http-1.1 |` (dotted slug), and **`28.1a`/`28.1b`** (the `^\| [0-9.]+ \|` id field cannot match a letter suffix). All four are `done`, so no current impact — **but the PLAN had dropped two the phase-73 close already knew about, which is exactly the failure the "RE-DERIVE, never copy" instruction on that figure exists to prevent.**
2. **A FIFTH stale claim in the row-74 ROADMAP cell**, beyond the PLAN's roster of four: the cell repeats the whole-file `grep 'VerifyPeerCertificate\|handshake-error callback' ⇒ 0`, which is now **3** (all three hits are sentences quoting the phrase in order to refute it). Corrected in the same form as the ¶3 defect.
3. **The PLAN's own "+0 production imports" gate command is UNRELIABLE.** `git diff master -- … | grep -E '^\+' | grep -E '^\+\s*(_|[a-z]+ )?"'` returns **3 hits and exits 0** — all false positives (T5's three `helpText` map-literal lines). Anyone gating on the exit code would read a PASS as a FAIL. The decisive checks are the hunk headers (no hunk touches either import block) and a direct extracted-import-block diff (IDENTICAL, 31 / 6 lines).
4. **The PLAN's in-code comment for T6 was false**: *"V2 verified it [the cross-side check] stays green under both Break G and Break H."* Under Break H the cross-side check **DID fire, twice**. The clause was not copied into the driver.
5. **T2's predicted red is UNREACHABLE IN ONE STEP** (F-2). `GateMatchesInc`'s pointer assertions — added by the PLAN's own adversarial pass — reference `rt.ssl*` fields that Step 2 creates, so the whole test binary fails to COMPILE and no assertion runs. A two-stage red (compile failure → add fields only → assertion-level red) was executed so the promised evidence was actually obtained. **A single-stage execution would have recorded a build failure as if it were the assertion red.**
6. **A THIRD stale-comment site in the PRODUCTION file** that the D6/F5 roster missed (F-1): `manager.go` carried `"surfaced by Task 10's AllocatesTwoMetricsPerListener test"`, which the T2 rename would have left dangling. Fixed in place.
7. **A FOURTH RD3 site** the PLAN's three-site list missed: the arm-2 inline comment in `driveSide` (`0111/driver.go:416-420`), which also said forced-send is *"NOT because it flips the require=true observable"*. Left standing it would have contradicted the three revised sites.
8. **Anchor drift within the row itself.** T1–T3 grew `manager.go` by ~180 lines, so every PLAN `file:line` for T4 onward was stale: `HandshakeContext` `:1178` → **`:1260`**; the mixed-chain reject `:516-525` → **`:575`**; `tlsMode` set `:639` → **`:692`**; `tlsCfg` set `:510` → **`:562`**; the launch-loop `kindQUIC` `continue` `:997-1001` → **`:1078-1082`** (and a second, earlier bind-loop `continue` at `:1044-1054` the PLAN never named); `normalizeAddr` `:342` → **`:347-349`**. ⚠️ **The T2 and T4 commit messages were taken from the PLAN verbatim as instructed and therefore carry the STALE `:516-525` / `:639` / `:1178` / `:1183` anchors.** Recorded rather than rewritten.
9. **Break F's "NAMED SUBSTITUTION (do not re-derive it)" did not apply** — in the landed layout the table row still consumes `liveNoCertErr`, so no `declared and not used` occurred and no binding deletion was needed. V1 hit it "deterministically"; the canonical tree does not.
10. **T5's old doc comment was internally inconsistent** before this row touched it: *"The 11 entries cover the 13 unique Prometheus names emitted by 06.1 … plus one 06.2 backpressure counter"* — 11 total minus the one 06.2 counter leaves **10** entries covering 13 names (the SN4 collapse). The rewrite states the split explicitly rather than propagating it.
11. **PLAN import forecasts were incomplete**: `fmt` (not just `reflect`) was absent from `quic_test.go`; the `0111` driver also needed `log` and `math` beyond the forecast `net/http`/`strconv`.

## Two fresh hazards this IMPL discovered (not in any phase-74 document)

- **`io.Copy(conn, conn)` is not a usable in-test echo backend.** `*net.TCPConn` implements `io.ReaderFrom`, so `io.Copy` splices the socket into itself and the echo never returns; the plaintext round trip died on an i/o timeout **at the backend**, which reads as a proxy bug. Proven with a direct-to-backend probe before blaming the proxy. Replaced with an explicit read/write loop.
- **T1's PKI presents a LEAF-ONLY client chain deliberately** — unlike the copy-source `0111/driver.go:255-257`, which appends the issuing CA. Appending it would make `len(cve.UnverifiedCertificates) == 2` and fire T1's own `want 1` assertion. A future editor must not "fix" the chain to match the driver.

## The one `-race` failure, and why it is PRE-EXISTING — evidence, not assertion

`internal/boot` `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server…` failed once under the full `go test ./... -race` run (`boot error = "… recv response: rpc error: code = DeadlineExceeded …", want it to mention the initial-fetch timeout`), and the whole-suite re-run was green (125 `ok`, **0 `DATA RACE`** in both runs).

1. **Isolate-re-run: green 3/3** (`-run` alone `-count=1`, `-count=5`, whole package).
2. **REPRODUCED ON MASTER** — in the main checkout on `master`, `-count=20` under 48 CPU burners on 32 cores failed **3/20 with the byte-identical error string**. A reproduction, not an inference.
3. The file is byte-identical to master and `internal/boot/**` is on the sha256 roster with 0 mismatches.
4. Mechanism: `initial_fetch_timeout: 0.2s` — the exact 200 ms budget the indexed flake documents; under load the gRPC dial+recv eats it, so the error arrives as `DeadlineExceeded` on `recv response` rather than the provider's `initial fetch timed out` classification.

⚠️ **This WIDENS the indexed memory rather than matching it.** `reference_sds_init_fetch_timeout_dial_budget_flake` names `internal/xds` `TestProvider_FetchInitialCertificate_Timeout`; what fired here is a **different test in a different package** sharing the **same 200 ms-budget mechanism** — a second, previously-unindexed instance. It now also has a **reproducible master-side recipe** (48 burners, `-count=20`, ~15% hit rate), which the original entry explicitly lacked.

None of the other listed flakes (`internal/cluster` `-race`, the full-suite startup `subject ready: EOF`, `internal/httpclient`, `internal/filter/hcm/h2`) fired in either run.

## Envelope — audited in TWO SEPARATE CATEGORIES

- **PRODUCTION: +0 imports — CONFIRMED** two ways (hunk headers; extracted import-block diff IDENTICAL). **ZERO new exported symbols** — `go doc -all ./internal/listener` and `./internal/stats` diffed against master: **IDENTICAL, 146 / 293 lines.** ZERO new packages. `go.mod`/`go.sum` byte-unchanged ⇒ **+0 modules**.
- **TEST: 15 additions, permitted and enumerated.** `manager_test.go` +9 (`crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509/pkix`, `encoding/pem`, `errors`, `math/big`, `reflect`, `sort`) · `quic_test.go` +2 (`fmt`, `reflect`) · `name_test.go` **+0** · `0111/driver/driver.go` +4 (`log`, `math`, `net/http`, `strconv`).

## Counts at exit — re-run MECHANICALLY in the worktree, never copied

fixtures **119** (+0) · fuzzers **55** (+0) · **stat surface 1201 → 1204 (+3)** · BackendKind tail **38** (+0; a TAIL VALUE — the file declares 39 constants, 0–38) · go.mod modules **2** (lineage figure; the single `go.mod` requires 67) · DECISIONS tail **ADR-0296 COMPLETE**, next-free **ADR-0297** · ZERO new packages · ZERO new exported symbols.
