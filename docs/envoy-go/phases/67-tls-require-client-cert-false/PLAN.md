# PLAN 67 — TLS `require_client_certificate: false` / verify-if-presented mTLS — Implementation Plan

> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — ZERO production `.go`. Worktree `.worktrees/phase-67-plan`, branch `phase-67-tls-require-client-cert-false-plan`, tip **`a15f4fca`**, per `feedback_git_worktrees`.
>
> **Row 67 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg). **ADR-0289's §Context is ALREADY DRAFTED** at the SPEC squash (`7cd69db6`); the IMPL **COMPLETES ADR-0289 IN PLACE** with §Decision/§Consequences — it does NOT append a new ADR. DECISIONS tail is **ADR-0289**, next-free **ADR-0290** (`[RUN]` at the dossier — the tail flip the SPEC pinned already happened at its squash; the parallel stream added no entry).
>
> **Baselines RE-DERIVED at `a15f4fca` (dossier `[RUN]`, NOT copied):** fixtures **111** (numeric tail `0109-xds-sds-combined-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`H2GoawayResponder`, `test/differential/fixture/fixture.go:614` — note the SPEC's bare "fixture.go:614" resolves there) · stat surface **1201** · go.mod: **1** `go.mod` file; the SPEC's "modules **2**" counts required modules in the lineage figure — **carry the SPEC's own metric unchanged** and re-check `git diff go.mod` after tidy as usual · green baseline `go test ./internal/tls/ ./internal/boot/ ./internal/listener/ ./internal/xds/ ./internal/filter/hcm/ -count=1` → **all ok**.
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 67`; check (2) prints **3** via the router's full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — cite the command, never the adjective); check (3) unchanged. **No deferred-sentence edit at ANY stage of this row** (SPEC §12).
>
> **⚠️ THE PARALLEL STREAM (LOUD).** Four commits landed BETWEEN the SPEC squash and this tip (`c7310cb8` boot SDS e2e + xds classification · `c109275d` QUIC negatives + TCP ALPN abort · `6cb7ffba` hcm mid-body reset · `b15a34ad` CI fuzz-matrix row · `a15f4fca` TEST_GAP append). **These are post-SPEC facts, NOT SPEC defects.** The dossier audited every new file line-by-line: **no test flips post-lift** (the only require=false YAMLs in the new e2e file carry cert-SDS with NO validation shape), but the stream created **absorptions this PLAN states loudly** (§1 items A1–A3) — four TEST_GAP sweep sites instead of two, a NEW chartering decision (`internal/boot` test-comment B19), and two free integration tripwires for T1's stays-green roster. Where this PLAN and the SPEC differ, **the dossier's `a15f4fca` facts govern** (they differ ONLY on the post-SPEC parallel stream — every SPEC code anchor re-derived ZERO-drift).

---

## 1. Absorption + correction ledger — every place this PLAN corrects or extends SPEC-67

**All SPEC §9/§11 code anchors re-derived at `a15f4fca` — every SYMBOL and CLAIM unchanged; FOUR SPEC ranges carry line-boundary corrections, recorded here (amendment M5)** (dossier §1: E3 `:66-69`, gate `:87`, arms `:89-120`/`:121-176`/`:177-187`, `:179-181`, `:188`, `:115-117`, `:143-149`, `:169-173`, `:381`/`:398`, `:402-407`, `:417-436`, QUIC `:200-223`, provider.go `:90-93`, config_test `:800-814`/`:999`/`:1009-1041`/`:1424-1454`/`:1456-1484`, fuzz_test `:210-228`/`:335`/`:341`, manager_test `:644`/`:1499-1516`, boot.go `:121`/`:131`/`:139-179`, BC anchors, DECISIONS `:16899`). **The four line-boundary corrections (symbols unchanged; verified by re-read):** (i) `internal/xds/provider.go` — the SPEC cites `:91-93`; the drifted sentence actually OPENS mid-line on `:90` and spans `:90-93`; **`:91-93` stays the B16 replacement scope**, `:90-93` is the full-sentence span — both stated where used; (ii) `TestCVC_RequireFalse_NeverYieldsNoClientCert` ends at config_test.go`:1484`, not the SPEC's `:1485`; (iii) `fakeProvider` spans config_test.go`:800-814`, not `:815`; (iv) the manager tripwire spans manager_test.go`:1499-1516`, not `:1517`. The ledger below is otherwise ALL absorption of post-SPEC reality plus structural decisions the SPEC delegated.

| # | SPEC-67 says | This PLAN says | Where |
|---|---|---|---|
| **A1** | B15: TEST_GAP sweep = `:133-137` + `:198-201` (two sites) | **FOUR sites, and the anchors MOVED** (the file is now 401 lines; `a15f4fca` did NOT only append — its first hunk `@@ -1,5 +1,9 @@` inserted a 4-line second-pass banner at the TOP of the file, which is what shifted every anchor below it — amendment M6): the two `build-reject` claims now at `:139-140`/`:203-205`, **PLUS two parallel-stream additions** — the §6.1 claim "a CVC listener with `require_client_certificate` false/absent **CANNOT boot — pinned twice**" (`:263-266`, grep hit `:264`; goes FALSE post-lift) and the §8 item-2 pending-probe item "reconcile the **three** serve-anyway doc sites" (`:384-387`, grep hit `:386`; DOUBLY stale: the probe already RAN at the SPEC, and the roster is FIVE sites). **The task text targets these by CLAIM + grep, never by line** (they will drift again). | T7 |
| **A2** | §15 pins `internal/boot` **BYTE-UNTOUCHED** | **CHARTERED EXCEPTION B19** (a decision the SPEC could not make — the target postdates it): `internal/boot/boot_sds_e2e_test.go:518-521`'s doc comment says D-RCCF-FETCHFAIL-POSTURE "**records the reference-side ambiguity**" — stale, because the SPEC **RESOLVED** the ambiguity (P1: uniform init-hold → fail-closed). Chartered exactly à la B16: a TEST-file comment-only correction, zero symbols, zero functional change; the envelope's REAL invariant — `internal/boot` **production** code (`boot.go`) byte-untouched, no behavior change anywhere in the package — holds unweakened. A "complete" fetch-failure-posture reconciliation that left a fresh stale copy standing in a test the row itself leans on (it's in T1's stays-green roster) would be the same false completeness claim B16's charter exists to prevent. Pinned replacement wording at T6. | §1.1, T6 |
| **A3** | T1's stays-green tripwire = `manager_test.go:1499` | Roster GAINS three parallel-stream integration tripwires: **`TestSDSEndToEnd_ValidationContextViaSDS_mTLS`** (require=true + SDS-VC — guards the hoisted SDS-VC arm), **`TestSDSEndToEnd_CVC_PoolSubstitution`** (require=true + CVC, force-send — guards the hoisted CVC arm; Break C fires its REFUSE subtest at the integration level — the accept subtest PASSES under that break, see the Break C row; it does NOT depend on the E3 substring — grep-verified), **`TestSDSEndToEnd_FetchFailure_BootFailsClosed`** (both subtests — boot-FAIL posture unchanged by the hoist). All three GREEN at `a15f4fca`; any post-IMPL failure in the four parallel-stream packages (`boot`, `listener`, `xds`, `hcm`) is a **REGRESSION, not an expected inversion** (dossier §2: nothing there flips). | T1 |
| **A4** | §10 sketches T1 (hoist + E3 atomic) and T2 (flip roster) as separate tasks | **The three TEST-FLIP sites land IN T1's commit, atomically with the lift** — phase-66's F5 lesson re-applied: subagents auto-commit per task, and a tree where E3 is retired but `TestCVC_RequireFalse_Rejected_E3` still asserts the reject is a RED tree at T1's own green gate. The flips ARE T1's red-first evidence (dossier §4: every flip-roster test GREEN at baseline, so each inversion is observably red pre-hoist). The green-neutral roster items (fuzz seed (i) comment — also T1; the 0109 comment sweep — T6 with its grep obligation) land where §10 put them, cross-referenced from T1's roster table so nothing is lost. | T1 |
| **A5** | §2/§13: over-sweep guard implicit | **Named NON-DRIFT sites the sweep must NOT touch**: `boot_sds_e2e_test.go:43` and `:540` (and the `:518` clause "no listener may come up serving with an unpopulated trust store") use "unpopulated trust store" as an **envoy-go counterfactual** ("must not serve with…"), not a reference characterization. The IMPL's drift-sweep grep will now hit them; each is **dispositioned as non-drift, byte-untouched**. An over-sweep is as wrong as an under-sweep. Also: `internal/listener/tls_handshake_negative_test.go` **PRE-EXISTED** at `facb0faa` (80 lines; only the ALPN test + `mkDownstreamTSInlineALPN` are new — the stream APPENDED 91 lines); do not treat the file as new when auditing. | T6 |
| **A6** | §10: "T3's new tests are written red-first" (sketch level) | Made concrete: post-T1 the new mapping tests would be born green, so **liveness is proven by the pre-verified breaks** (Break A re-run fires every per-shape mapping `Errorf`; Break D is the interface-pinned test's isolating break) plus one PLAN-drafted two-edit break for the corrupt-CA property (T2 — flagged NOT pre-compiled, substitution rule applies). The corrupt-CA property is structurally over-determined (error propagation AND the nil-pool guard both enforce it); a single compiling edit cannot make it return a nil error — recorded, not hidden. | T2 |

### 1.2 Adversarial-pass record

**Adversarial pass: RUN against draft `9b080b6f`; corrections applied by this amendment.** THREE independent verifiers:

- **PV1 — code-claims BY EXECUTION:** built the hoist from the PLAN's sketch in scratch and proved the flip roster COMPLETE — the untouched suite fails exactly and only the three roster tests, everything else green; applied and executed EVERY break; reproduced Break B's withholding physics from scratch with fresh PKI; re-derived every anchor/count.
- **PV2 — spine logic:** order-soundness simulated task-by-task; T1 atomicity re-derived from the tree; the corrupt-CA two-edit over-determination verified by code-path walk.
- **PV3 — process:** decomposition mapped 1:1 against SPEC §10/§15 and the router's owed bullets; commit hygiene; sentinel checks re-RUN and matching.

**TOTAL: 0 SEVERE · 5 distinct MODERATE · ~10 minor — ALL corrected in this amendment.** The moderates: **MOD-1** Break C's e2e firing set was wrong — EXECUTED truth: the `TestSDSEndToEnd_CVC_PoolSubstitution` ACCEPT subtest PASSES under the break (only the refuse subtest fires); **MOD-2** T6's post-sweep "zero remaining LIVING drifted sites" confirmation could not pass at T6 time — split T6-scoped/T9-full with an expected-hits classification table; **MOD-3** the pre-verified-verbatim status of Breaks A/C/D (and the corrupt-CA arity) is CONDITIONAL on the shared-closure mechanism — now a BINDING paragraph in the Break protocol; **MOD-4** the T6 commit phrase "five drift-site rewrites" collided with the canonical five-site drift roster (B2/B11/B12/B16/B17, completing only at T9) — renamed; **M5–M12** minors: four SPEC line-boundary anchor corrections recorded in §1's banner, the A1 anchor-shift explanation fixed (top-of-file insertion, not append), Break B's compile-cite repointed at the POLITE analogues (0108 driver.go:418 / 0109 driver.go:449), the loose "B1–B15 → T7" assignments made exact (B11@T6, B12@T9), a NEW require=false fetch-failure `vcErr` arm added to T2 (pinning the §3.5/§3.12(2) departure directly), the 0109-sweep disposition categories gained still-accurate-live-config + named http2 exclusions + the enumeration-protection note, T1's red-first item-3 observable count corrected three → TWO, and the two TEST_GAP site ranges made exact (`:263-266`/`:384-387`).

**Honest limits:** 0109's standing differential green and the 1201 stat surface were carried, not re-run (docker/mechanical-command scope); T2 test 4 (anchorless) remains a liveness-unproven regression pin, recorded per the phase-66 F2 discipline.

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-67 IMPL.
- **ONE functionally-edited production file: `internal/tls/config.go`.** Chartered comment-only exceptions, each named and pinned: `internal/xds/provider.go:90-93` (B16), `internal/tls/config_test.go:999` (B17, test file), `internal/boot/boot_sds_e2e_test.go:518-521` (B19, test file — A2). `internal/boot/boot.go`, `internal/listener` (incl. quic.go), `validate/`, `test/helpers/sdsserver` **BYTE-UNTOUCHED**.
- **Counts at the IMPL:** fixtures **111 → 112** (`0110-tls-require-client-cert-false`) · fuzzers **55 (+0, seeds only)** · stat surface **1201 (+0)** · BackendKind **38 (+0)** · go.mod **+0** (SPEC metric "2" carried; re-check `git diff go.mod` after tidy — `reference_new_subpackage_pulls_transitive_module`) · ZERO new packages · DECISIONS tail stays **ADR-0289** (completed IN PLACE; next-free ADR-0290).
- **The four-cell mapping is FIXED** (SPEC §1): true+anchor → `RequireAndVerifyClientCert` · false/absent+anchor → `VerifyClientCertIfGiven` · false/absent+no-anchor → `NoClientCert` · true+no-anchor → the RETAINED `:179-181` reject. Absent ≡ false (wrapper `BoolValue`, nil getter; no tri-state). **Assignment-adjacency binds** (§3.6): `VerifyClientCertIfGiven` is set ONLY in the arm that has already installed a non-nil pool.
- **The pinned §9 wording lands MECHANICALLY** — B1–B19 are named obligations with verbatim replacement text; never silent rewrites, never paraphrases.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`): pin the canonical root; controller verifies the MAIN checkout stays clean; deliberate breaks restore with **`git restore` only**; breaks run AFTER committing (`reference_break_protocol_commit_first`).
- **Subagents commit locally; the controller squash-pushes at stage-close** (`feedback_subagents_no_push`, `feedback_push_to_origin`). Locate commits by SUBJECT (`git log --grep 'phase 67'`), never by position.
- **`reference_sds_init_fetch_timeout_dial_budget_flake`** — a `TestProvider_FetchInitialCertificate_Timeout` failure under `-race` is PRE-EXISTING on master (one occurrence, 2026-07-16). Do not reflex-classify as a phase-67 regression; a SECOND occurrence justifies widening the budget.
- **`reference_0061_ring_hash_spread_flake`** — same rule for a 0061 spread failure during the full differential.

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks A–D below are **pre-verified compiling** at `a15f4fca` (dossier §5: applied in a scratch tree copy → `go vet` exit 0 → reverted; never committed) — **for A/C/D this status is conditional on the shared-closure mechanism; see the MECHANISM-CONDITIONALITY paragraph below (MOD-3)**. Any OTHER break in this PLAN is flagged as NOT pre-compiled: if it does not compile, **substitute a compiling equivalent, REPORT the substitution, record the TRUE result**.
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed.
- **`-count=1` on EVERY break** (`reference_differential_break_protocol_count1`).
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first (phase-66's isolating-break lesson).
- **A break that does NOT fire is a FINDING** — record it honestly in PROGRESS; do not route around it.
- **Full selector only:** `-run 'TestDifferential/0110-tls-require-client-cert-false'` — never bare `0110` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for broken preconditions** (`reference_fatalf_makes_assertions_unreachable`).

### The four pre-verified breaks (dossier §5 — adopt verbatim; exact compiling edits)

| id | Exact edit (COMPILES, `[RUN]`) | Must fire | Owner |
|---|---|---|---|
| **A** | In the post-lift mapping, change the false/absent **anchored** cell from `stdtls.VerifyClientCertIfGiven` to `stdtls.NoClientCert` (one identifier swap in `clientAuthFor`'s false branch / the equivalent site) | The flipped `TestCVC_RequireFalse_NeverYieldsNoClientCert` — post-lift its check compares `ClientAuth == stdtls.NoClientCert` DIRECTLY (config_test.go:1479-1481), so the mapping-value `Errorf` fires first and cannot be masked by an err-path `Fatalf` (post-flip err is nil). Re-run at T2: every per-shape mapping `Errorf`. Fixture level (not run as a break): 0110's untrusted arm would go accept-instead-of-reject (no CertificateRequest at `NoClientCert`) | T1 + T2 re-run |
| **B** | In the 0110 driver's untrusted arm, replace `cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) { return &badCert, nil }` with polite `cfg.Certificates = []stdtls.Certificate{badCert}` (compile-verified: the forced-send form being REPLACED matches the live boot_sds_e2e_test.go:432-435 site; the polite REPLACEMENT matches the live polite analogues 0108 driver.go:418 / 0109 driver.go:449 — amendment M7) | At require=false the server (IfGiven + advertised CAs) makes Go's polite client WITHHOLD the unacceptable-CA cert (`SupportsCertificate` filtering, SPEC §3.7) ⇒ handshake SUCCEEDS ⇒ the untrusted arm's `rejected` verdict assertion fails — the break harness catches the vacuous green. **Control (per `reference_deliberate_break_wrong_assertion`): the same polite mode at require=true does NOT fire** — 0109's driver IS polite and IS green; that standing green is the control, which is exactly WHY the require=false fixture needs its own break | T5 |
| **C** | In the hoisted CVC arm, guard the install: `if !require { return installPool(pool) }` then `return nil` (require=true CVC silently skips the pool) | The phase-66 CVC require=true unit tests (`RequireAndVerifyClientCert` + non-nil `ClientCAs` — the `Errorf`s at config_test.go:1687/:1690 and :1712), and at the e2e level **ONLY the refuse subtest** of `TestSDSEndToEnd_CVC_PoolSubstitution` (boot_sds_e2e_test.go:511 `t.Fatal` "…the SDS pool was MERGED…" on an accepted default-CA leaf). **The ACCEPT subtest PASSES under this break (EXECUTED — amendment MOD-1): the NoClientCert-shaped server sends no CertificateRequest, so the trusted-leaf handshake + echo still succeed — it is NOT part of the firing set.** Discriminates: proves the retained require=true path traverses the HOISTED arm, not a vestigial copy | T1 |
| **D** | Delete the `if pool == nil { return … }` guard from the install site (or whatever mechanism T1 picks to enforce §3.6) | `TestVerifyIfGiven_NilPool_Unconstructible`'s `(nil,nil)` `&fakeProvider{}` arm then yields `ClientAuth == VerifyClientCertIfGiven && ClientCAs == nil` — the exact asserted-unreachable state. This IS that test's isolating break (the property is unreachable from production fetchers, so no red-first run can prove it live) | T2 |

**⚠️ MECHANISM-CONDITIONALITY (BINDING — amendment MOD-3).** Breaks A/C/D's "pre-verified compiling — adopt verbatim" status, Break A's one-swap-fires-ALL-shapes T2 re-run claim, and the corrupt-CA break's quoted arity hold **ONLY in the dossier's shared-closure shape** (`installPool`/`clientAuthFor`). The T1 edit licenses the IMPL to keep per-arm adjacency instead; if it does:

- **Break A becomes one-swap-PER-ARM** — THREE edits, one per shape's arm, each applied and run separately, each firing ITS OWN shape's mapping `Errorf`. Under the per-arm form a non-firing shape is a **REAL finding about that arm** (its swap was applied and its test still passed), not noise.
- **Breaks C and D require IMPL-time compile re-verification** of the per-arm equivalents before use — their verbatim edits reference `installPool`, which does not exist in that shape (`reference_plan_break_instructions_dont_compile` applies FRESH; substitute-report-record).
- **The corrupt-CA break's arity is shape-dependent**: the T2-quoted `return nil, err` assumes the two-value `NewDownstreamConfig` return context; inside the shared closure the site is one-value — the compile-verified substitution is `return err` (→ `if err != nil && require { return err }`).

The IMPL records WHICH mechanism it landed and which break forms it therefore used.

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE at the dossier (`[RUN]`, repo-wide, `.worktrees` excluded):** `VerifyClientCertIfGiven` · `TestVerifyIfGiven_NilPool_Unconstructible` · the stem `TestNewDownstreamConfig_RequireFalse*` (any suffix — 0 hits) · `mkDownstreamTSVerifyIfPresented` · `forceSendClientCert`. **Verified FREE by THIS PLAN's own grep at `a15f4fca`** (`grep -rn --include='*.go'`, recorded): `clientAuthFor` (0 hits) · `installPool` (0 hits) · `TestNewDownstreamConfig_RequireAbsent*` (0 hits) · `TestInlineCorruptTrustedCA` (0 hits). **TAKEN — avoid exact reuse:** `TestCVC_RequireFalse_Rejected_E3` and `TestCVC_RequireFalse_NeverYieldsNoClientCert` (the former is retired at T1; the latter is kept and inverted IN PLACE — its name stays TRUE post-lift). **Same-name-different-package is fine:** the 0110 `package driver` re-declares 0109's helpers (`mustCA`/`mustLeaf`/`mustAllocatePort`/`structuralCheck`/`normalizeTLSErr`/`driveSide`/`wantObservable`) without collision — own package. The parallel stream's new identifiers (dossier §2d roster) live in `boot`/`listener`/`xds`/`hcm` — zero overlap with any name above. **Any FURTHER name the IMPL coins: grep first, record the check.** `test/fixtures/0110*` does not exist; `0110` appears nowhere under `test/differential/`; in-container port **10446** free (only SPEC.md self-references).

---

## File structure

```
internal/tls/config.go            [EDIT]  T1 (hoist + 3-way ClientAuth + E3 retired + theorem block moved intact), T6 (B11 + B18 comment rewrites)
internal/tls/config_test.go       [EDIT]  T1 (flip roster), T2 (new tests), T6 (B17 :999 message)
internal/tls/fuzz_test.go         [EDIT]  T1 (seed (i) comment flip), T3 (SEEDS only — count STAYS 55)
internal/xds/provider.go          [EDIT — COMMENT ONLY]  T6 (B16, the chartered exception; the classification switch UNTOUCHED)
internal/boot/boot_sds_e2e_test.go [EDIT — COMMENT ONLY] T6 (B19, chartered à la B16 — A2)
test/fixtures/0110-tls-require-client-cert-false/  [ADD]  T4 (driver/, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md), T5 (breaks)
test/fixtures/0109-*/             [COMMENT SWEEP]  T6 (the §3.8 enumerated set + grep obligation)
docs/envoy-go/BEHAVIOR_CONTRACT.md [EDIT]  T7 (B1–B10/B13–B14; B11@T6 is config.go, B12@T9 is DECISIONS — M8)
docs/TEST_GAP_ANALYSIS.md         [EDIT]  T7 (B15 — FOUR sites, A1)
docs/envoy-go/DECISIONS.md        [EDIT]  T9 (ADR-0289 completed IN PLACE + the B12 bracketed annotation)
internal/boot/boot.go · internal/listener/** · validate/** · test/helpers/sdsserver/**  [BYTE-UNTOUCHED]
```

---

## Task 1 — the ATOMIC core: hoist + three-way `ClientAuth` + E3 retirement + the flip roster, ONE commit

**Entry state:** clean `a15f4fca`-derived branch; `go test ./internal/tls/ -count=1` green; every flip-roster test GREEN (dossier §4 — that is what makes each inversion observably red).

**⚠️ THE LIFT, ITS GUARD PROPERTIES, AND THE TEST FLIPS ARE ONE COMMIT** (A4; `reference_lifted_reject_hidden_enforcement` — before lifting E3, this SPEC asked what else it silently enforced: mTLS-actually-on for the CVC shape, now enforced by the three-way mapping + the nil-pool guard landing in the SAME task). Phase-66 landed lift+guard atomically in the other direction; this row retires that guard and installs the replacement in one motion.

### The E3 flip roster IN FULL (SPEC §3.8 — every item tracked here; landing site per item)

| # | Site | Today | Post-lift | Lands |
|---|---|---|---|---|
| 1 | `TestCVC_RequireFalse_Rejected_E3` (config_test.go:1424-1454, both subtests: false AND absent-nil-`BoolValue`) | asserts err + the E3 substring | **RETIRED** (deleted); its inverted coverage is the NEW `TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven` (both subtests: success + `VerifyIfGiven` + non-nil pool) — the old name would lie post-lift | **T1** |
| 2 | `TestCVC_RequireFalse_NeverYieldsNoClientCert` (config_test.go:1456-1484, both subtests) | requires `err != nil` AND `ClientAuth != NoClientCert` | **err-half INVERTS** (want nil err); the never-NoClientCert half becomes the row's LIVE property — want `VerifyClientCertIfGiven`. Name kept: still true | **T1** |
| 3 | config_test.go:1009-1041, subtest `"require_client_certificate=false leaves the SDS validation_context INERT"` (parent `TestNewDownstreamConfig_SDS`, :885) | asserts NO fetch (`fakeProvider{vcErr: "FETCH MUST NOT HAPPEN"}`), `ClientCAs == nil`, `NoClientCert` | **ALL THREE assertions INVERT** (fetch fires — give the fake a pool, not a vcErr; pool installed; IfGiven) — but only TWO inversions are OBSERVABLE red (see protocol step 2 — amendment M11); subtest renamed to state consumption, e.g. `"require_client_certificate=false CONSUMES the SDS validation_context (verify-if-presented)"` | **T1** |
| 4 | fuzz seed (i) comment (fuzz_test.go:210-217; body :218-228 survives — it seeds CVC + require:false via `"downstream-sds"`) | narrates the E3 reject | **comment flips** to narrate the verify-if-presented consumption (the seed itself keeps passing; only its intent lied) | **T1** (green-neutral, but it narrates E3 — it goes with E3) |
| 5 | the 0109 comment-sweep set, FULLY ENUMERATED (SPEC §1.1 V3): envoy.yaml:16 + envoy-go.yaml:21 (`require_client_certificate: true is MANDATORY (PLAN-66 D1)`) · README.md:39 · expectations.yaml:24 · README.md:150-151 · expectations.yaml:137-139/:145-146 | documents E3 / require=true-MANDATORY as named boundaries | **comment sweep + grep obligation** `grep -rn 'MANDATORY\|E3\|require_client_certificate' test/fixtures/0109-*/` — every hit dispositioned | **T6** (green-neutral docs; cross-referenced so the roster cannot under-land) |

### RED-first protocol (the flips are the red)

1. Make ONLY the test edits (roster items 1–3). Run `go test ./internal/tls/ -count=1`.
2. **Each MUST fail, and for the RIGHT reason** — record the verbatim failures: items 1–2 red with the E3 error (`combined_validation_context requires require_client_certificate: true in phase 03` where success/IfGiven is now expected — the red MESSAGE quoting the retired substring is itself evidence the test exercises the exact reject being retired); item 3 red on **TWO observable inversions** (`ClientCAs` nil / `NoClientCert`) — EXECUTED count (amendment M11): the "no fetch fired" pre-state has NO observable of its own without adding call-recording to `fakeProvider` (the fake's un-installed pool IS the `ClientCAs` red); the IMPL MAY add call-recording to observe the fetch directly — its choice, not owed. A red for any OTHER reason (compile error, unrelated reject) is VACUOUS — stop and fix the test, per phase-66 F2.
3. Land the production edit (below). All flipped tests go green.

### The edit — `internal/tls/config.go` (symbols + `a15f4fca` lines; the dossier §3a skeleton COMPILED against the real signatures)

- **Retire E3** `:66-69` (the sole enforcement — call-graph re-derived at the SPEC; it guards nothing else).
- **Hoist** the three anchor arms out of the `:87` require gate: `require := ctx.GetRequireClientCertificate().GetValue()`; three-arm switch on `common.GetValidationContextType()` (same type-assertion shapes as today's `:89`/`:121`/`:177`); a shared `installPool(pool)` closure that **errors on `pool == nil`** then sets `cfg.ClientCAs = pool` AND `cfg.ClientAuth = clientAuthFor(require)` **adjacently** (`RequireAndVerifyClientCert` / `VerifyClientCertIfGiven`) — assignment-adjacency (§3.6) via a shared install site, the form the dossier compile-checked; the IMPL may keep per-arm adjacency instead, but the PROPERTY (IfGiven only ever set beside a just-installed non-nil pool) is fixed — **and choosing per-arm adjacency triggers the Break-protocol MECHANISM-CONDITIONALITY paragraph (MOD-3): Breaks A/C/D's verbatim status is closure-shape-only.**
- **The inline no-anchor branch** keys on `vc == nil || vc.GetTrustedCa() == nil` (compiles): require=true → the **RETAINED `:179-181` reject** (byte-identical — `tls: downstream: require_client_certificate=true requires validation_context.trusted_ca`); false/absent → return with zero-value `NoClientCert`.
- **The `:122-149` theorem block moves INTACT with the CVC arm** — the P5 comment `:143-149` **byte-intact** (ADR-0287 §Decision calls it MANDATORY). The `:169-173` fetch-error comment ALSO moves with the arm **as-is at T1**; its B18 cause-scoped rewrite lands at T6 (SPEC §10 places it there) — moving-then-rewriting is two commits of the same stage, never a shipped state.
- **Ordering preserved:** `commonTLSContextToConfig` at `:52` still runs BEFORE the anchor arms (P4's only constraint); the CVC arm still calls `provider.FetchInitialValidationContext` via `xds.ParseSDSConfig` exactly as landed (P3); the fetch now fires at ANY require value (§3.5 — the un-gated fetch; `internal/boot`'s pre-scan never read the field, so **boot.go takes NO edit**).

### GREEN exit

- `go test ./internal/tls/ -count=1` green (flips + all pre-existing).
- **Stays-green roster, run explicitly** (A3): `TestNewManager_MultiChain_RequireClientCert_Errors` (manager_test.go:1499-1516 — the §3.4 tripwire: require=true + no-anchor must still route to `:179-181`) · `TestSDSEndToEnd_ValidationContextViaSDS_mTLS` · `TestSDSEndToEnd_CVC_PoolSubstitution` · `TestSDSEndToEnd_FetchFailure_BootFailsClosed` (both subtests) · full `go test ./internal/boot/ ./internal/listener/ ./internal/xds/ ./internal/filter/hcm/ -count=1`. Any failure there is a REGRESSION.
- **Retained-reject roster BYTE-DIFFED** (SPEC §3.8): grep each verbatim substring and confirm byte-identical + count-unchanged — `:381-383` (`SDS-bound validation_context_sds_secret_config is not supported in phase 03`) · `:398-400` (`combined_validation_context is not supported in phase 03`) · E1 `:402-404` · E2 `:405-407` · the four `:417-436` sub-field rejects · `:179-181`. Plus grep the RETIRED substring (`combined_validation_context requires require_client_certificate: true in phase 03`) → its only remaining hits are docs/history.
- `gofmt -l internal/tls` silent · `go vet ./internal/tls/` · `golangci-lint run ./internal/tls/`.

**Breaks (after committing — `reference_break_protocol_commit_first`):** **Break A** (fires the inverted `:1479-1481` mapping check — confirm the mapping-value `Errorf`, not an err-path abort) · **Break C** (fires the phase-66 require=true CVC unit tests AND — at the e2e level — ONLY `TestSDSEndToEnd_CVC_PoolSubstitution`'s refuse subtest; the accept subtest PASSES under the break (MOD-1) — confirm which fired, at both levels). `git restore` after each; package re-green; `-count=1`.

**Commit:** `tls(phase 67 T1): hoist the anchor arms out of the require gate — three-way ClientAuth (assignment-adjacent), E3 RETIRED atomically with its flip roster (red-first inversions recorded), theorem block moved intact, retained rejects byte-diffed`

---

## Task 2 — new unit tests: the full mapping cross-product + the interface-pinned unconstructibility property

**Entry state:** T1 landed; hoisted code green.

**Tests (`internal/tls/config_test.go`), each cell/property its own `Errorf`; every test runs BOTH false and absent (nil `BoolValue`) subtests:**

1. `TestNewDownstreamConfig_RequireFalse_Inline_VerifyIfGiven` — inline `trusted_ca` ⇒ `VerifyClientCertIfGiven` + the loaded pool.
2. `TestNewDownstreamConfig_RequireFalse_SDSVC_VerifyIfGiven` — SDS-VC via `fakeProvider{pool: …}` ⇒ fetch fires, pool installed, IfGiven. **PLUS a fetch-FAILURE arm (amendment M9): `fakeProvider{vcErr: …}` at require=false ⇒ the boot error PROPAGATES (`tls: `-prefixed, nil cfg) — the §3.5/§3.12(2) departure instance (require=false + SDS is now boot-FAIL-capable) pinned DIRECTLY at the unit level.** Green-stable post-T1; its integration twin is `TestSDSEndToEnd_FetchFailure_BootFailsClosed`; no break owed (recorded as a pin, like test 4).
3. (CVC covered by T1's `TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven`.)
4. `TestNewDownstreamConfig_RequireFalse_Anchorless_NoClientCert` — no validation config AND the anchorless-VC shape (`validation_context: {}` / no `trusted_ca`) ⇒ `NoClientCert`, nil error (SPEC §3.2 decision 1). **Green-stable regression pin** (this cell is unchanged by the hoist) — recorded as a pin, no break owed; the require=true side of the naive-mapping hazard is the manager tripwire's job.
5. `TestInlineCorruptTrustedCA_RequireFalse_BootError` — require=false + corrupt inline `trusted_ca` file ⇒ **error** (`tls: `-prefixed), nil cfg (SPEC §3.12(1) — a stated DECISION on envoy-go's strict posture, NOT a parity claim; the reference's corrupt-CA config-validate posture was not probed — say so in the test comment).
6. **`TestVerifyIfGiven_NilPool_Unconstructible`** (§3.6, C7) — table-driven over the `xds.SecretProvider` INTERFACE (dossier §3c's compiling shape; `var _ xds.SecretProvider = &fakeProvider{}` holds; `&fakeProvider{}` yields the interface-legal `(nil, nil)`): NO config × provider-behavior combination — including `(nil, nil)` — yields `cfg.ClientAuth == VerifyClientCertIfGiven && cfg.ClientCAs == nil`. The `(nil,nil)` arm must produce an ERROR (the install site treats nil-pool-nil-err as an error), never a cfg in the forbidden state. (Hazard being pinned: IfGiven + nil pool = Go falls back to SYSTEM roots — rejects the legitimate client, admits anonymous ones; worst direction — `reference_go_client_cert_withholding`.)

**Liveness (A6):**
- **Break A re-run** ⇒ every per-shape mapping `Errorf` (tests 1, 2, and T1's CVC test) fires on the mapping value. Confirm each. **Diagnosis is mechanism-conditional (MOD-3):** under the SHARED-CLOSURE form, one swap fires all shapes — a non-firing shape means its test does not reach the mapping (a test defect, a finding); under the PER-ARM form the re-run is three edits, one per arm — FIRST confirm the per-arm break was actually applied to that arm, THEN treat a still-non-firing shape as a real finding.
- **Break D** ⇒ test 6's `(nil,nil)` arm fires on the exact forbidden-state assertion. This is its ONLY possible liveness proof (unreachable from production fetchers) — if it does not fire, the guard mechanism T1 picked is not what the test pins; reconcile, don't route around.
- **Corrupt-CA two-edit break (NOT pre-compiled — substitution rule applies):** apply Break D's guard deletion AND change the inline arm's error return (after `loadTrustedCAPool`) from unconditional to require-gated — in the two-value `NewDownstreamConfig` context `if err != nil { return nil, err }` → `if err != nil && require { return nil, err }`; **inside the shared closure the site is ONE-value — the verified substitution is `return err` → `if err != nil && require { return err }` (MOD-3: arity per the landed mechanism).** Under both edits a corrupt CA at require=false yields nil error + IfGiven + nil pool ⇒ test 5 fires on `err == nil` AND test 6 fires. **Why two edits:** the property is over-determined — error propagation and the nil-pool guard each independently force an error, so no single compiling edit produces the nil-error state (recorded as a strength, per `reference_probe_must_discriminate`: the break demonstrates BOTH enforcement layers must fail together).
- All breaks `-count=1`, `git restore`, re-green.

**GREEN exit:** `go test ./internal/tls/ -count=1` + per-task hygiene trio.

**Commit:** `tls(phase 67 T2): the mapping cross-product unit tests (3 shapes × {false, absent} + anchorless + corrupt-CA + the require=false fetch-failure vcErr arm) + the interface-pinned nil-pool unconstructibility property (Break D isolated; Break A re-fired per shape)`

---

## Task 3 — fuzz SEEDS (count STAYS 55; dispatch trap honored)

**Entry state:** T1 landed (seed (i)'s comment already flipped there).

**Edit (`internal/tls/fuzz_test.go`):** add seeds to `FuzzTLSContextParse` (fuzz_test.go:50) per the SPEC §7 DECISION — **the dispatch table binds** (re-derived: `"downstream"` → nil provider `:335`; `"downstream-sds"` → `cvcFuzzProvider` `:341`; sole assertion the `tls: ` prefix `:347-349`):

| new seed | side | why this side |
|---|---|---|
| inline `trusted_ca` + require:false | `"downstream"` | hoisted inline arm; loads pool; IfGiven |
| inline `trusted_ca` + require ABSENT | `"downstream"` | the absent twin |
| SDS-VC + require:false | `"downstream-sds"` | hoisted SDS-VC arm; fetch succeeds; IfGiven |
| CVC + require ABSENT | `"downstream-sds"` | completes the CVC pair (seed (i) is the false twin, surviving) |

**⚠️ Do NOT seed an SDS shape on `"downstream"`** — it dies at the retained `:381` nil-provider gate and pins only the `tls: ` prefix (the exact phase-66 T5 vacuity; SPEC §7's named trap).

**Verification:** each new seed's reached branch confirmed via a temporary diagnostic (then REMOVED) — phase-66 T5's protocol: every seed produces a state textually distinct from an earlier-reject path, ruling out "errors/passes earlier for an unrelated reason". `go test -run FuzzTLSContextParse -count=1` green; a short active-fuzz smoke (≈10s) — no panic, no corpus artifacts committed. **Reconcile `^func Fuzz` count = 55 BEFORE and AFTER** (`reference_fuzzer_count_docs_drift`).

**Commit:** `tls(phase 67 T3): require=false/absent fuzz seeds on the correct dispatch sides (+0 fuzzers, count reconciled 55→55; the downstream-side SDS-seed trap honored)`

---

## Task 4 — fixture `0110-tls-require-client-cert-false` (fixtures 111 → 112)

**Entry state:** T1–T3 landed; `test/fixtures/0110*` does not exist (verified).

**Design (SPEC §8 — CVC-primary at require=false, the 0109 chassis as a disciplined clone; dossier §3b's forced-send snippet COMPILES):**

- `tcp_proxy` echo; `BackendCount() == 1` (`reference_differential_backendcount_min_one`); the positive arms genuinely echo through it.
- **Both YAMLs:** CVC with inline `default_validation_context.trusted_ca` = **CA_Y** (the anchor that must LOSE) + `validation_context_sds_secret_config` served **CA_X** (the anchor that must WIN) — 0109's Design-A observable, now under `require_client_certificate: false`. **The wrapper field takes a BARE scalar** (`require_client_certificate: false`); `{value: false}` ERRORS (`reference_protojson_wrapper_scalar_not_object`).
- Per-side driver-owned `sdsserver.Server`s with **hard `Close`** (GracefulStop deadlocks — 0109 driver.go:297-299 note); **ARM-UNIQUE secret names**; the **served-this-arm precondition assert** (0109 `driveSide` :361-365 pattern; `feedback_probe_fresh_container_per_arm` — driver-owned servers need fresh-per-arm discipline, the phase-66 near-false-divergence).
- In-memory PKI via the cloned helpers (`mustCA`/`mustLeaf`/… — own `package driver`, no collision); `mustAllocatePort`; reference in-container port **10446**.
- **THREE arms:** `trusted` (client_X signed by CA_X, **FORCED-SEND**) → ok+echo · `untrusted` (client_Y signed by CA_Y — the INLINE default's CA, **FORCED-SEND** via `cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) { return &badCert, nil }`, bypassing `SupportsCertificate`) → rejected (proves BOTH the anchor is live at require=false AND the served pool REPLACED the inline default — a client_Y accept would be Design-C union or default-won) · `none` (neither `Certificates` nor `GetClientCertificate`) → ok+echo (**the row's discriminator vs require=true**).
- `structuralCheck` widened to the three-arm `wantObservable`; `normalizeTLSErr`; **PER-SIDE failure pins** — never cross-side string equality (`reference_differential_reference_parses_full_message`; the wire alert 48 agrees per SPEC §3.1, client-observed strings need not).
- **One fixture dir = ONE runner branch** — pure cross-side (`reference_differential_fixture_dispatch_constraint`); never assert `/listeners` or `total_listeners_active`, and **never treat a docker-proxy accept as listener liveness** (P1's trap). `reference_envoy_contrib_image_tagging` / `reference_host_gateway_ip_docker_desktop` apply.
- `expectations.yaml` + `README.md`: clone 0109's shape; state the proposition (*verify-if-presented at require=false across the CVC shape; the served CA replaces the inline default; the no-cert arm is the flag's discriminator*), the §3.3/§3.5 departures with the CORRECTED reference characterization (never serve-anyway wording), and the forced-send mandate with WHY (the withholding trap).
- 0018/0108/0109 expectations UNCHANGED (all require=true).

**GREEN exit:** `go test ./test/differential/ -run 'TestDifferential/0110-tls-require-client-cert-false' -count=1` PASS; fixture count 112 (`ls -d test/fixtures/[0-9]*/ | wc -l`); hygiene trio on the driver package.

**Commit:** `differential(phase 67 T4): fixture 0110-tls-require-client-cert-false — CVC-primary three-arm verdict at require=false with a FORCED-SEND untrusted arm (fixtures 111→112, port 10446, per-side failure pins)`

---

## Task 5 — 0110 deliberate breaks (the fixture is not done until its assertions are PROVEN live)

**Entry state:** T4 committed green. Every break: `-count=1`, full selector, `git restore` only, confirm WHICH assertion fired.

1. **Break B — the forced-send is load-bearing** (pre-verified compiling; the row's signature break). Apply the polite-mode swap in the untrusted arm ⇒ the handshake SUCCEEDS by withholding ⇒ the untrusted arm's `rejected` verdict fails ⇒ `structuralCheck` (or the verdict comparison) FIRES. Record the verbatim failure and WHICH assertion. **Control already standing:** 0109 (require=true, polite mode) is green on master — the same swap-shape does not fire at require=true, proving this break discriminates the require=false vacuous-green specifically. If Break B does NOT fire, the untrusted arm is vacuous — STOP; the fixture does not prove the anchor live.
2. **`structuralCheck` three-step demonstration** (phase-66 T6's protocol, re-demonstrated on 0110 — never asserted): (a) disable `structuralCheck` + SYMMETRIC served-CA break (both sides' SDS servers serve CA_Y instead of CA_X) ⇒ **observe the fixture ship PASS** (both sides flip identically; `CompareBytes` EQUAL — `reference_vacuous_break_receiver_normalizes`); record the PASS. (b) restore, re-apply ONLY the symmetric break with the check ENABLED ⇒ `structuralCheck` fires, naming side + arms. (c) ASYMMETRIC break (subject-side server only serves CA_Y) with the check disabled ⇒ `CompareBytes` mismatch — record the byte offset (proves `CompareBytes` independently live — `reference_differential_asserter_dispatch`).
3. **Per-side failure-pin break:** perturb one side's pinned untrusted-arm failure string by one character in the expectations ⇒ that side's pin assertion fires ALONE (proves the pin is live and per-side, not decorative).
4. **Served-this-arm assert break:** point one arm's client at the OTHER arm's secret-name/server (or skip the arm's serve) ⇒ the precondition assert fires BEFORE any verdict is compared (proves the stale-server trap is guarded — `feedback_probe_fresh_container_per_arm`).

Breaks 3–4 are NOT pre-compiled — substitution rule applies; report any substitution. A break that does not fire is a FINDING recorded in PROGRESS.

**Commit:** `differential(phase 67 T5): 0110 liveness demonstrated — Break B proves the forced-send load-bearing at require=false (0109's standing green is the require=true control); structuralCheck vacuous-PASS/fire/CompareBytes triplet; per-side pin + served-this-arm breaks` *(tree byte-identical to T4's code; this commit carries only PROGRESS/README break-log updates if any — otherwise record results in PROGRESS at stage-close and skip the commit)*

---

## Task 6 — comment sweeps: B11 + B18 + B16 + B17 + **B19** + the 0109 enumerated set (+ the NON-DRIFT dispositions)

**Entry state:** T1–T5 landed. **This task is comment/docs-only in code terms: ZERO symbols, ZERO functional change — verify the diff is comment-only (`git diff` inspection + full package tests green).**

1. **B11 — `internal/tls/config.go:115-117`** (drift site; now inside the hoisted SDS-VC arm): replace with SPEC §9 B11's pinned six-line comment VERBATIM (init-hold → bind-at-timeout → per-connection fail-closed; never serve-anyway).
2. **B18 — the CVC arm's fetch-error comment (was `:169-173`, now moved with the arm):** replace with SPEC §9 B18's pinned cause-scoped comment VERBATIM (empty-dynamic ACK-and-serve stays scoped to ITS cause; timeout/unreachable get the corrected characterization). The P5 block `:143-149` stays byte-intact — verify with `git diff` that the theorem block shows as MOVED, not edited.
3. **B16 — `internal/xds/provider.go` (the drifted sentence opens mid-line on `:90` and spans `:90-93`; the final-sentence REPLACEMENT SCOPE is the SPEC's `:91-93` — the §1 line-boundary record, M5)** (the chartered doc-comment-only `internal/xds` exception, SPEC §15): replace the doc comment's final sentence with B16's pinned replacement VERBATIM. **The classification switch is UNTOUCHED** — the new `provider_classification_test.go` pins the `errValidation`-before-`ctx.Err()` ORDER inside `FetchInitialValidationContext` (dossier §2c); it pins no comment wording, so B16 cannot interfere — but run `go test ./internal/xds/ -count=1` anyway.
4. **B17 — `internal/tls/config_test.go:999`:** replace the failure message with the pinned string VERBATIM: `t.Fatal("expected a boot failure, got nil (envoy-go boot-FAILS where the reference init-holds then fails closed per-connection — ADR-0280 family, characterization corrected at ADR-0289)")` (compile-verified at the dossier).
5. **B19 — `internal/boot/boot_sds_e2e_test.go:518-521` (the CHARTERED post-SPEC absorption — A2).** Replace ONLY the stale parenthetical `(the phase-67 drift question D-RCCF-FETCHFAIL-POSTURE records the reference-side ambiguity; envoy-go's own posture is boot-FAIL and this test is its integration-level pin)` with the pinned:
   ```go
   // (D-RCCF-FETCHFAIL-POSTURE was RESOLVED at the phase-67 SPEC — probe P1,
   // {server-cert, validation-context} × {silent, unreachable}, all four cells
   // identical: the reference init-holds (port unbound), then at
   // initial_fetch_timeout starts workers and binds, then fails closed
   // per-connection (downstream_context_secrets_not_ready); envoy-go's own
   // posture is boot-FAIL — ADR-0280 family, characterization corrected at
   // ADR-0289 — and this test is its integration-level pin).
   ```
   The comment's surrounding counterfactual clause ("no listener may come up serving with an unpopulated trust store") **STAYS** — it states envoy-go's obligation, not a reference characterization. `boot.go` stays byte-untouched; run `go test ./internal/boot/ -count=1`.
6. **The 0109 sweep** (flip-roster item 5): the six enumerated sites, then discharge the grep obligation `grep -rn 'MANDATORY\|E3\|require_client_certificate' test/fixtures/0109-*/` — **every hit dispositioned into one of THREE categories (amendment M10)**: rewritten to the post-67 truth · correctly-historical · **still-accurate live config** (the actual config lines envoy-go.yaml:66 / envoy.yaml:69 — the fixture legitimately STAYS require=true). The grep's http2 hits (`MANDATORY http2_protocol_options` — envoy.yaml:116 / envoy-go.yaml:111 / expectations.yaml:48; THREE, re-verified at this amendment) are UNRELATED to this row — named and EXCLUDED. **NOTE: expectations.yaml:145-146 (the withholding-manifestation note, part of the enumerated set) matches NONE of the grep terms — it is protected by the ENUMERATION, not the grep; the grep is the under-sweep FLOOR, the enumeration is the ROSTER.** Rewrites state: require=true is now a CHOICE (0110 covers false); the E3 boundary paragraph is retired-with-history.
7. **NON-DRIFT dispositions (A5) — recorded, byte-untouched:** boot_sds_e2e_test.go:43/:540 (+ the :518 counterfactual clause), **and BC:914** — the "an unpopulated trust store that accepted everything" contrast clause, a counterfactual the SPEC dispositions UNCHANGED (amendment MOD-2). Then a **T6-SCOPED confirmation ONLY**: re-run the `.go`-side grep (`grep -rn 'serves anyway\|serve-anyway\|unpopulated trust store' --include='*.go'`) and confirm T6's OWN five comment targets (B11/B18/B16/B17/B19) are landed — post-T6 that grep's only remaining `.go` hits are the dispositioned counterfactuals (boot_sds_e2e :43 / the :519 clause / :540). **The FULL-repo "zero remaining LIVING drifted sites" confirmation CANNOT pass at T6 time and MOVES to T9 (MOD-2):** at T6 the docs-side grep still legitimately hits sites scheduled LATER — BC:900 (B2 lands T7) · TEST_GAP:386 (T7) · DECISIONS.md:16899 (the B12 bracketed annotation lands T9) · STATE.md:18 (rewritten only at T9's pointer edit). See T9's expected-hits classification table. (Dropped from the previous draft's expected-hits list: BC:2582/:5390 — they are ADR-0147 `require_client_certificate` recaps carrying NO serve-anyway/unpopulated wording; this grep never hits them.) **Roster identity (MOD-4): the canonical FIVE-site serve-anyway drift roster is B2/B11/B12/B16/B17 and completes only ACROSS T6/T7/T9 — T6 lands three of the five (B11/B16/B17); B2 lands T7; B12 lands T9.**

**Commit:** `docs+comments(phase 67 T6): the five T6 comment rewrites land — B11/B18 (config.go), B16 (the chartered provider.go doc-comment), B17 (:999 message), B19 (the chartered boot_sds_e2e stale-ambiguity sentence — post-SPEC absorption) + the full 0109 sweep with its grep discharged; non-drift counterfactuals dispositioned untouched (the canonical five-site drift roster B2/B11/B12/B16/B17 completes at T9)`

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B10/B13–B14) + the FOUR-site TEST_GAP sweep (B15)

**Entry state:** T6 landed (B11 and B16–B19 already in code). Docs-only. **Exact §9 assignment (amendment M8): T7 carries B1–B10/B13–B15; B11 landed at T6 (config.go); B12 lands at T9 (the DECISIONS annotation).**

**BC (`docs/envoy-go/BEHAVIOR_CONTRACT.md`) — apply SPEC §9 MECHANICALLY, pinned wording verbatim:** B1 (:898 re-scope) · **B2** (:900 full replacement paragraph — drift site #1) · B3 (:902 INERT ¶ RETIRED) · B4 (:906 qualifier) · B5 (:912 UNCHANGED — verify only) · B6 (:920 conjunct) · B7 (:928 E3 divergence item RETIRED with history) · B8 (NEW require=true anchorless-VC departure ¶) · B9 (:936 (a) closed / (b) QUIC sharpened) · B10 (NEW cert-provider per-arm statements) · B13 (:950 + the NEW 0110 Differential-coverage ¶) · B14 (the NEW `## TLS` Supported ¶ after :1817 + the BC:1817 two-clause annotation). Anchor by SYMBOL/first-clause, not line (lines cited as of `facb0faa`; BC took zero parallel-stream edits — dossier §1 — but re-locate each anyway).

**TEST_GAP (`docs/TEST_GAP_ANALYSIS.md`) — FOUR sites, targeted BY CLAIM + GREP (A1; the anchors moved once already):**

| # | Locate by | The stale claim | Fix |
|---|---|---|---|
| 1 | `grep -n 'build-reject' docs/TEST_GAP_ANALYSIS.md` (first hit; ~:139-140) | "blocked — envoy-go build-rejects `require_client_certificate`, so there is no runtime mTLS to drive" | rewrite to post-67 truth (require=true since phase 16/ADR-0147; verify-if-presented since 67; runtime mTLS drivable — 0018/0108/0109/0110). **Pre-existing drift (stale against ADR-0147 BEFORE this row) — fixed in passing, flagged as such** |
| 2 | same grep (second hit; ~:203-205) | "first needs `require_client_certificate` to be a supported runtime feature (it currently build-rejects), so feature + test land together" | same rewrite, same pre-existing flag |
| 3 | `grep -n 'CANNOT boot' docs/TEST_GAP_ANALYSIS.md` (~:263-266, hit :264 — M12; §6.1 append) | "the phase-66 E3 security reject (a CVC listener with `require_client_certificate` false/absent CANNOT boot — pinned twice…)" — **FALSE post-lift** | rewrite: E3 retired at 67; the property test now pins verify-if-presented (IfGiven), the pin-twice discipline carried forward. **Parallel-stream addition** |
| 4 | `grep -n 'serve-anyway doc sites' docs/TEST_GAP_ANALYSIS.md` (~:384-387, hit :386 — M12; §8 item 2) | "Resolve the D-RCCF-FETCHFAIL-POSTURE drift with the discriminating reference probe phase 67 already obligates, then reconcile the **three** serve-anyway doc sites" — **DOUBLY stale** (probe RAN at the SPEC; roster is FIVE) | mark COMPLETED by this row: probe ran at SPEC-67 (P1), all five living sites reconciled (B2/B11/B12/B16/B17). **Parallel-stream addition** |

Post-sweep: re-run both greps + `grep -n 'three serve-anyway'` ⇒ zero stale claims remain.

**Commit:** `docs(phase 67 T7): BEHAVIOR_CONTRACT B1–B10/B13–B14 applied verbatim (CVC/SDS-VC false-arm CONSUMED, B2 drift paragraph, B8 new departure, B14 TLS Supported ¶) + the FOUR-site TEST_GAP sweep (B15 — two pre-existing build-reject claims, two parallel-stream staleness sites) targeted by claim+grep; B11 landed T6, B12 lands T9`

---

## Task 8 — VERIFY: the six-gate + cycle guard + full differential + `-race` + mechanical counts

Controller-run on the frozen pre-stage-close HEAD:

1. `gofmt -l internal/ test/ cmd/` — SILENT
2. `go vet ./...` — exit 0
3. `go build ./...` — exit 0
4. `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` EMPTY (+0 modules)
5. `golangci-lint run ./...` — exit 0
6. **FULL differential:** `go test ./test/differential/ -count=1` — all **112** dirs, exit 0. The 111 pre-existing dirs byte-stable. `reference_differential_fullsuite_startup_flake`: a `subject ready: EOF` on an UNRELATED fixture is a startup race — isolate-re-run to discriminate; `reference_0061_ring_hash_spread_flake` on a second occurrence → investigate margins.

**Plus:**
- **Cycle guard:** `go list -deps ./internal/xds | grep 'envoy-go/internal'` (**no `...`**) ⇒ `internal/stats` + `internal/xds` ONLY (`reference_xds_config_seam_transitive_cycle_guard` — TYPE-level only; trivially true, B16 is a comment, but ASSERT it).
- **`-race` on touched packages:** `go test ./internal/tls/ ./internal/boot/ -race -count=1` (the `init_fetch_timeout` flake caveat stands; `reference_full_suite_race_after_background_mutator`).
- **Counts MECHANICAL, never copied:** fixtures **112** (tail `0110-tls-require-client-cert-false`) · fuzzers **55** (`^func Fuzz`) · BackendKind **38** · DECISIONS tail **ADR-0289** · stat surface **1201**.
- **Envelope audit:** `git diff master --stat` shows `internal/xds` = provider.go comment-only; `internal/boot` = boot_sds_e2e_test.go comment-only; `boot.go`/`internal/listener`/`validate/`/`sdsserver` absent from the diff.

*(No separate commit — T8's evidence lands in PROGRESS at T9.)*

---

## Task 9 — ADR-0289 completed IN PLACE + B12 + stage-close (controller-adjacent)

- **ADR-0289: COMPLETE IN PLACE** — append §Decision + §Consequences to the EXISTING entry (ADR-0044 pattern; the §Context landed at the SPEC squash). **Do NOT append a new ADR; do NOT renumber.** Tail stays ADR-0289; next-free ADR-0290. §Decision records the landed mechanism (hoist shape, `clientAuthFor`/install-site adjacency, the interface-pinned property, forced-send driver); §Consequences records the flip roster as executed, the B19 absorption, and the memory updates.
- **B12** — the DECISIONS:16899 ADR-0286 bullet gains SPEC §9 B12's bracketed `[CORRECTED at phase 67/ADR-0289: …]` annotation VERBATIM, appended in place (a DECISIONS entry is never silently rewritten).
- **FULL-repo serve-anyway drift closure (MOVED here from T6 — amendment MOD-2).** After B2 (T7), B12 (this task), and the STATE.md pointer edit land, re-run BOTH greps — `grep -rn 'serves anyway\|serve-anyway\|unpopulated trust store' --include='*.go'` and the same terms over `docs/` — and confirm **zero remaining LIVING drifted sites**. Every hit must fall in a row of this expected-hits classification table; a hit that fits no row is a FINDING:

  | hit | class |
  |---|---|
  | *(any living drifted site)* | **NONE expected — a hit here fails the gate** |
  | boot_sds_e2e_test.go:43 / the :519 clause / :540 · BC:914 | **counterfactual** ("must not serve with…" / the contrast clause) — dispositioned A5/MOD-2, byte-untouched |
  | BC:900 (B2) · config.go B11 site · provider.go B16 site · config_test.go:999 (B17) · the TEST_GAP §8 item (T7) | **corrected-in-place** — post-rewrite the wording survives only inside a NEGATION ("never serve-anyway") or no longer matches; verify per-site |
  | DECISIONS.md:16899 | **historical-with-correction** — the original sentence STAYS (never silently rewritten) with the B12 bracketed annotation appended |
  | DECISIONS.md:17026 (ADR-0289 §Context) | **historical/still-accurate** — quotes the drift in order to record its correction |
  | STATE.md:18 | **rewritten at THIS task's pointer edit** (pre-edit it already used the wording only inside `NEVER "serves anyway"`) |
  | STATE.md:44/:46 | **lineage-history** — prior-active-phase recaps, immutable |
  | phase-65/66/67 stage docs (`docs/envoy-go/phases/**`) | **immutable history** |
  | 0109 fixture config lines (require=true) | **still-accurate-config** — not matched by THESE terms (they belong to T6's 0109 grep); listed so the category set is total |
- **ROADMAP row 67 → `done`** at the six-gate (ADR-0106, SOLE leg; `reference_roadmap_split_phase_row_done`). **Deferred sentence UNTOUCHED** (SPEC §12 — do NOT fabricate a narrow).
- **STATE.md:** edit §Current pointer IN PLACE; demote to §Recent lineage capped at five; update counts.
- **PROGRESS.md:** finalize — every break's ACTUAL firing assertion (including any that did NOT fire), the verbatim red-first records, T5's three-step demonstration, any break substitutions.
- **Router roll** (`next-prompt.txt` — TRACKED despite .gitignore; edit in the stage worktree; locate by SUBJECT).
- **Sentinel re-run MECHANICALLY:** check (1) goes silent when row 67 flips; (2) still prints 3 via the full-phrase command; (3) unchanged ⇒ does NOT fire; no `stop` file.
- **Memory updates owed (SPEC §13 + this PLAN's):** (i) extend `reference_go_client_cert_withholding` with the settled 0108/0109 polite-mode fact + the forced-send mandate + Break B's require=true control; (ii) the serve-anyway → init-hold-fail-closed drift-correction lesson (five living sites; roster by repo-wide grep, never memory); (iii) NEW from this PLAN: **a post-SPEC parallel stream can mint FRESH copies of the exact drift a row is retiring** (B19: a test landed BETWEEN SPEC and PLAN carried the just-resolved ambiguity wording; sweep rosters must be re-derived at the PLAN tip, and byte-untouched pins need a chartered-exception mechanism rather than silent violation or silent staleness).
- **Squash-push by the controller** at stage-close.

**Commit (stage-close docs):** `phase 67 (tls-require-client-cert-false) IMPL: …` (controller composes at close).

---

## Self-review against SPEC-67

| SPEC obligation | Where |
|---|---|
| Three-way mapping, four cells; absent ≡ false | Global, T1, T2 |
| Assignment-adjacency + interface-pinned unconstructibility (§3.6, C7) | T1 edit, T2 test 6, Break D |
| ATOMIC lift + E3 retirement, flip roster IN FULL (§3.8) | **T1** (roster table, items 1–4) + T6 (item 5) — A4 |
| Retained-reject roster byte-diffed | T1 GREEN exit |
| `:122-149` theorem/P5 block moved INTACT | T1 edit, T6 item 2 verify |
| require=true anchorless routing preserved (§3.4 tripwire) | T1 stays-green roster |
| Un-gated fetch; `internal/boot` production NO EDIT (§3.5) | T1 edit, T8 envelope audit |
| Forced-send driver mandate (§3.7 — CLOSED, not open) | T4, **Break B** (T5) |
| Fuzz seeds on correct dispatch sides; +0 fuzzers (§7) | T3 |
| Fixture 0110 CVC-primary, three arms, per-side pins, port 10446 (§8) | T4, T5 |
| B1–B10/B13–B15 pinned wording (§9) | T7 (M8: B11@T6, B12@T9) |
| B11/B16/B17/B18 pinned wording (§9) | T6 |
| **B19 (this PLAN's chartered addition — A2)** | **T6 item 5** |
| TEST_GAP sweep — **four sites, claim+grep (A1)** | T7 |
| ADR-0289 completed IN PLACE + B12 (§14, §15) | T9 |
| Six-gate + cycle guard + counts (§15) | T8 |
| Sentinel: nothing owed (§12) | header, T9 |
| Memory updates (§13 + new) | T9 |

**Task count: 9** — inside the SPEC's ~9 anticipation. **ADR-0045 escape valve ARMABLE, UNCONSUMED — no split**: one production file; no two-package surface can strand a leg. T1→T2→T3 sequential on `internal/tls`; T4→T5 sequential on the fixture; T6/T7 independent after T5; T8/T9 close.

**⚠️ The IMPL's standing instruction: a PLAN is not evidence either** (phase-66's PLAN carried nine draft defects; its IMPL then found more). **RE-DERIVE this document; do not execute it.** Where it cites, go look (the TEST_GAP lines WILL have drifted again — that is why T7 targets claims); where it claims control flow, walk the call graph; default to REFUTED. Start where this PLAN is most confident: T1's roster table, Break B's control reasoning, and the A2/B19 charter.
