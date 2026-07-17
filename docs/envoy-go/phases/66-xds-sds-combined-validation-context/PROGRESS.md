# Phase 66 PROGRESS — xDS SDS `combined_validation_context` (ADR-0287; row 66 flips `done` at the IMPL six-gate — the SOLE leg, ADR-0106)

> The IMPL executed `PLAN.md` task-by-task, subagent-driven (`feedback_execution_style` / `feedback_subagent_autocommit_claudemd` / `feedback_subagents_no_push`), each task committing LOCALLY only and squashed to a single master commit by the controller at stage-close (located by SUBJECT — `git log --grep 'phase 66'`, never by position — `reference_next_prompt_tracked_despite_gitignore`). **T1–T3 landed in a PRIOR (cut-off) session; THIS session RESUMED at T3's break-debt and drove T4–T9.** The row's defining property: **ZERO new symbols in `internal/xds`** — `internal/xds`, `parseValidationSecret`, `FetchInitialValidationContext`, and `test/helpers/sdsserver` are BYTE-UNTOUCHED. ADR-0287 is COMPLETED IN PLACE (no new ADR; the tail stays **ADR-0288**, next-free **ADR-0289**).

## Task checklist (mirrors PLAN-66 — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [x] **T1** — lift the `combined_validation_context is not supported in phase 03` reject (downstream + live-provider) + E1/E2 (the two PGV-`required` halves, envoy-go runs NO PGV) + **E3** (require:true enforced in `NewDownstreamConfig`, landed ATOMICALLY with the lift — SECURITY-RELEVANT) — commit `57dbde17`; 8 tests green; C1 confirmed empirically via Break D.
- [x] **T2** — re-point the four inline sub-field rejects (`custom_validator_config` / `match_typed_subject_alt_names` / `verify_certificate_hash` / `verify_certificate_spki`) at `default_validation_context` (the `inlineVC` selector) — commit `44669a78`; 4 verified-non-vacuous REDs (`got nil error`) + 5 tests green.
- [x] **T3** — the apply-point (nil-provider guard + `xds.ParseSDSConfig` + the `tls: ` wrap + `FetchInitialValidationContext` + `cfg.ClientCAs` + the mandatory P5 comment) — commit `a315e810`; 5 tests green. **Break debt owed at landing** (discharged this session, see below).
- [x] **T3-debt** — discharged; no commit (tree byte-identical at `a315e810`; report `task-3debt-report.md`).
- [x] **T4** — the boot pre-scan THIRD arm (CVC inner SDS half; pure-inline skipped) — commit `6cdf3db3`; review clean (spec ✅, Approved, 0 findings).
- [x] **T5** — fuzz SEEDS (a NEW `downstream-sds` dispatch side) — commit `88120f98`; review clean; the vacuous-seed deviation ADJUDICATED correct.
- [x] **T6** — fixture `0109-xds-sds-combined-validation-context` (110 → 111) — commit `187fb890`; review clean; the three-step `structuralCheck` demonstration.
- [x] **T7** — `BEHAVIOR_CONTRACT.md`: CVC REJECTED → CONSUMED + 12 named boundaries — commit `51884ad1`; review Approved (1 Minor rolled up, fixed at the final-review fix wave).
- [x] **T8** — fix the three stale/false `internal/tls/config.go` comments (comment-only diff) — commit `3f600cbc`; review Approved, 0 findings; the PER-ARM roster adjudication. (The T7-M1 declarative-prose fix did NOT land at T8 despite being flagged then — it landed at the final-review fix wave; see below.)
- [x] **T9** — six-gate + ADR-0287 in place + STATE/ROADMAP/PROGRESS + router roll + squash-push (this docs commit closes the stage).

## Prior-session summary (T1–T3) + the session-cutoff note

**T1–T3 landed by the cut-off session, located by SUBJECT** (`git log --grep`):
- **T1 `57dbde17`** — the reject-lift + E1/E2/E3. E3 lands ATOMICALLY with the lift: without it a CVC listener with `require_client_certificate` false/absent boots SUCCESSFULLY with `ClientCAs` nil and `ClientAuth == NoClientCert` (a silently unauthenticated listener). Break D empirically confirmed C1.
- **T2 `44669a78`** — the four re-pointed rejects, each shown RED first (`got nil error`, non-vacuous).
- **T3 `a315e810`** — the apply-point; CODE done, 5 tests green, but with break debt owed (below).

**⚠️ Session-cutoff note (`reference_break_protocol_commit_first`).** The prior session ended mid-break — a deliberate break was left UN-RESTORED in the worktree. The resuming controller restored it with **`git restore` only** (never re-running the broken tree, never re-applying the break), then verified the worktree byte-identical to `a315e810` before dispatching T3-debt. Lesson recorded: breaks run AFTER committing, and an agent killed mid-break leaves the tree BROKEN — `git status` the stage worktree at session start and diff any modification before trusting it (`feedback_subagent_worktree_detach`). This is why T3-debt could re-derive the two owed breaks against a known-clean base.

## Liveness-break log (accumulating; every break `-count=1`, WHICH assertion fired confirmed, then restored byte-identical)

> Provenance is labelled per entry (as in the phase-65 record): **RE-DERIVED THIS SESSION** entries were run against a known-clean base and their VERBATIM failure captured; **CONTROLLER-RELAYED** entries are quoted from the T1/T2 subagents' own reports and were NOT re-run when this record was written.

### T1 — Break D (E3 deletion) [CONTROLLER-RELAYED]
Deleting E3 ⇒ `TestCVC_RequireFalse_NeverYieldsNoClientCert` fired with the message pinning `ClientAuth == NoClientCert — a SILENTLY UNAUTHENTICATED listener`. **C1 (the security invariant) empirically confirmed** — the reject-lift without E3 converts today's loud boot-FAIL into a silent no-mTLS boot.

### T2 — four RED-first sub-field rejects [CONTROLLER-RELAYED]
Post-T1 / pre-T2, all four re-pointed CVC sub-field tests failed `got nil error` (non-vacuous — the RED window says `got nil error`, NOT the vacuous `combined_validation_context is not supported`, PLAN F2). Each is a genuine reach past the retained gate to the re-pointed reject.

### T3-debt — Break 1 (Design C pool-UNION) EXECUTED and REFUTED [RE-DERIVED THIS SESSION]
The T3 commit owed Break 1 (its result was never captured). Re-run against the clean `a315e810` base: the CVC arm's `cfg.ClientCAs = pool` was preceded by a compiled pool-union (`loadDataSource(defTC, baseDir)` + `pool.AppendCertsFromPEM(defBytes)`), then:
```
=== RUN   TestCVC_ServedPoolWins_DefaultTrustedCaNotRead
    config_test.go:1715: a leaf signed by CA_Y VERIFIES against ClientCAs — default_validation_context.trusted_ca was read into the pool (this is Design C, the rejected pool-UNION; the equivalence theorem requires the served pool to win outright)
--- FAIL: TestCVC_ServedPoolWins_DefaultTrustedCaNotRead (0.00s)
```
The FIRST assertion (leaf CA_X still verifies, `:1712`) did NOT fire — a union keeps CA_X too, so `:1715` is the SOLE discriminator. **Design C (pool-union) is thereby REFUTED at the unit level** — the equivalence theorem's observable (the served pool wins OUTRIGHT) is PROVEN, not asserted. Informational: `TestCVC_RequireTrue_InstallsSDSPoolAsClientCAs` stayed PASS under the break because it asserts pointer identity and `AppendCertsFromPEM` mutates the same `*x509.CertPool` in place — so it CANNOT catch a pool-union; `TestCVC_ServedPoolWins_...` is the only discriminator.

### T3-debt — the isolating `tls: `-prefix break (severing (i) ⊨ (ii)) [RE-DERIVED THIS SESSION]
The prior bare-`GetName()` break (T3 Break 2) fired BOTH properties of `TestCVC_MalformedSDSConfig_Rejected`, but property (ii) (the `tls: ` prefix invariant) was **ENTAILED by (i)** (reject-at-all) under it — so (ii) was not independently proven live. The isolating break returns the `ParseSDSConfig` error UNWRAPPED (property (i) still fires — the config is still rejected — but the `tls: ` prefix is gone):
```
=== RUN   TestCVC_MalformedSDSConfig_Rejected
    config_test.go:1742: error = "xds: sds: envoy_grpc cluster_name is required", want the `tls: ` prefix (the FuzzTLSContextParse invariant)
--- FAIL: TestCVC_MalformedSDSConfig_Rejected (0.00s)
```
Property (ii) at `:1742` fired ALONE; property (i) at `:1731` (`err == nil`) stayed green (the error is present, just unprefixed). The two properties are separate `t.Error` calls, so (ii) is now proven independently live — **the entailment is severed** (`reference_deliberate_break_wrong_assertion`: a break that fires can still prove nothing when one property entails another; add an isolating break). Package green after restore.

### T4 — arm-deletion break [RE-DERIVED THIS SESSION]
Protocol: verify green → COMMIT (`6cdf3db3`) → delete the third pre-scan arm → `git restore` → re-verify. With the arm deleted:
```
--- FAIL: TestNewSDSProvider_CVCValidationOnlySDS_BuildsProvider (0.00s)
    boot_test.go:500: NewSDSProvider: got nil provider — a CVC listener with its SDS half present must build a provider (seen==1)
```
**Confirmed the failure is the `provider == nil` assertion (`boot_test.go:500`), NOT a panic** — clean `--- FAIL`, no goroutine stack, no `panic:` line. `TestNewSDSProvider_CVCPureInline_ReturnsNilNil` stayed PASS (no arm recognizes the CVC oneof pre-fix, so `seen` stays 0 regardless — the expected regression-pin case). Restored byte-identical; all 12 boot tests green.

### T6 — the three-step `structuralCheck` demonstration (0109's load-bearing break log) [RE-DERIVED THIS SESSION]
Protocol: verify green → COMMIT (`187fb890`) → three breaks, all `-count=1`, FULL `-run` selector.

**Step 1/3 — vacuous PASS** (`structuralCheck` disabled + SYMMETRIC served-CA break: both SDS servers serve CA_Y instead of CA_X). Both sides flip identically to `good=REJECTED …\nbad=ACCEPTED\n`, so `CompareBytes` compares EQUAL and the fixture ships PASS:
```
--- PASS: TestDifferential (2.10s)
    --- PASS: TestDifferential/0109-xds-sds-combined-validation-context (2.10s)
```
→ demonstrates `reference_vacuous_break_receiver_normalizes` on 0109: a pure-`CompareBytes` fixture would ship GREEN on a completely broken trust anchor.

**Step 2/3 — `structuralCheck` FAILS** (`git restore`, re-apply ONLY the symmetric served-CA break, check now ENABLED). It is `structuralCheck` that fires, naming the side AND both arms:
```
runner_test.go:1240: ref drive: reference: SDS combined_validation_context structural check FAILED:
      positive arm: client_X chains to the SDS-SERVED CA (CA_X) and MUST be accepted (...)
      negative arm: client_Y chains to the INLINE default CA (CA_Y) and MUST be rejected (the SDS-served pool REPLACES the inline default; if client_Y is accepted the pools UNIONED — Design C — and the headline proposition is refuted)
      want: "good=ok echo=phase66-cvc-probe\nbad=rejected\n"
      got:  "good=REJECTED err=handshake-or-roundtrip-failed\nbad=ACCEPTED\n"
--- FAIL: TestDifferential/0109-xds-sds-combined-validation-context (2.13s)
```
The `got:` line is exactly the stream a pure-`CompareBytes` fixture would have PASSED → `structuralCheck` (not `CompareBytes`) is what caught the symmetric break.

**Step 3/3 — `CompareBytes` live independently** (ASYMMETRIC break: subject-side SDS server ONLY serves CA_Y, reference stays CA_X; `structuralCheck` disabled). The reference emits the correct stream, the subject flips → `CompareBytes` mismatch:
```
runner_test.go:1279: differential mismatch:
    first divergence at offset 5
    ref [0..21]:  |good=ok echo=pha|se66-|
    subj[0..21]:  |good=REJECTED er|r=han|
```
**Byte offset 5** — the `good=` prefix (5 bytes) shared, then `o`(0x6f) vs `R`(0x52), arithmetically consistent with 0108's documented offset 5. Restored byte-identical; green re-verified. A NEW served-this-arm precondition assert was ADDED (0108 lacks it; each side's SDS receiver must record ≥1 `StreamSecrets` request — `reference_probe_fresh_container_per_arm` / `feedback_probe_fresh_container_per_arm`).

### The nil-provider guard — DELIBERATELY UNPROVEN
The apply-point's nil-provider defense-in-depth guard is recorded as **deliberately unproven — unreachable by construction**: `commonTLSContextToConfig` refuses the nil-provider CVC shape FIRST, so no break can fire this guard. It is retained as documented defense-in-depth (a future caller relaxing that reject cannot nil-deref here), NOT the live gate. This mirrors the phase-65 SDS-VC sibling exactly (ADR-0286's `provider == nil` unreachable-dead-code finding). The break not firing IS the finding, not a nuisance to route around.

## T5 — the vacuous-seed discovery + the `downstream-sds` dispatch addition (deviation, ADJUDICATED)

`FuzzTLSContextParse`'s existing `f.Fuzz` closure dispatches the `"downstream"` side with `NewDownstreamConfig(ts, "", nil)` — the provider is ALWAYS nil. So EVERY CVC-shaped seed, however shaped, would land on the retained nil-provider gate (`combined_validation_context is not supported in phase 03`) BEFORE E1/E2/E3 or the four re-pointed rejects — vacuously "covering" nothing (`reference_probe_must_discriminate` at the seed level: **a seed is a probe too**; caught by the implementer). **Deviation (adjudicated correct):** a NEW `"downstream-sds"` dispatch case was added INSIDE the existing fuzzer, backed by a deterministic package-level `cvcFuzzProvider` (a fixed-pool `*fakeProvider`, race-safe, ignoring names, guarded on the arbitrary-input path to `FetchInitialCertificate`). Each new seed's reached branch was confirmed by a temporary diagnostic (then removed) — every seed produced a message textually DISTINCT from every other and matching the corresponding unit test's asserted substring, ruling out "errors earlier for an unrelated reason." Seed 13 pins the CVC-specific retained gate via the nil-provider `"downstream"` side (the realistic QUIC / `validate.Bootstrap` / no-SDS-`main.go` entry point). Fuzzer count 55 before AND after (canonical `^func Fuzz` grep, reconciled both ways — `reference_fuzzer_count_docs_drift`); a 10s active-fuzz smoke run (525k+ execs, 32 workers) found no panic and wrote no corpus artifacts.

## T8 — the PER-ARM roster adjudication (the session's OWN correction of its router's conflated count)

The router's T8 instruction proposed a FIVE-path roster for the plain SDS-VC arm's nil-provider guard (three production + two test-only), copied with adaptation from the CVC arm's already-landed comment. Independent RE-DERIVATION for THIS arm (execution + repo-wide grep, not `internal/`-scoped) REFUTED the third production item: the roster is PER-ARM, and **a count is only correct WITH its scope** (`reference_code_comment_not_evidence`).
- **The CVC arm's retained gate: THREE production consumers** — `NewQUICDownstreamConfig`, `validate.Bootstrap`, AND `main.go`'s ordinary boot path at `seen == 0` (kept live because `NewSDSProvider`'s pre-scan deliberately SKIPS a pure-inline CVC — `boot.go:172-177` — returning `(nil, nil)`).
- **The plain SDS-VC arm's gate: TWO production consumers** — `NewQUICDownstreamConfig`, `validate.Bootstrap`. `main.go`@`seen == 0` is **STRUCTURALLY EXCLUDED** because the pre-scan counts the plain SDS-VC oneof UNCONDITIONALLY (`boot.go:152-155`), so that shape forces `seen ≥ 1`; the SDS-VC oneof has no pure-inline variant to create the CVC arm's escape hatch.
- Both arms additionally have the TWO exported test-only constructors `listener.NewManager` / `NewManagerWithBaseDir` (zero non-test callers; `NewUpstreamConfig` cannot reach either guard — the oneof nils `GetValidationContext()` and it refuses earlier).

The landed comments keep each arm's roster distinct, each TRUE of its own arm; the false "exactly ONE live consumer" text is retired. Recommendation carried to ADR-0287 + memory. (The T7-M1 minor — three process-referential meta-commentary spots in the shipped `BEHAVIOR_CONTRACT.md`, incl. negated-form banned phrases producing false grep hits — was flagged at T8 but NOT converted then; it was converted to purely declarative prose at the final-review fix wave, the facts themselves verified correct throughout.)

## Six-gate evidence (controller-run on the frozen HEAD `3f600cbc`, 2026-07-16)

- **gofmt** `gofmt -l internal/ test/ cmd/` → **SILENT**
- **vet** `go vet ./...` → **exit 0**
- **build** `go build ./...` → **exit 0**
- **tidy** `go mod tidy -diff` → **EMPTY** + `git diff --exit-code master -- go.mod go.sum` → **NO DIFF** (modules stay 2)
- **lint** `golangci-lint run ./...` → **exit 0**
- **full differential** `go test ./test/differential/ -count=1` (all **111** dirs; no startup flake) → `ok  github.com/pgdad/envoy-go/test/differential  380.044s`, exit 0 (110 pre-existing byte-stable — the row LIFTS a reject, it cannot change a passing fixture's bytes — plus the new `0109`)
- **cycle guard** `go list -deps ./internal/xds | grep envoy-go/internal` (no `...`) → `internal/stats` + `internal/xds` ONLY — `internal/tls` does NOT appear; GUARD HOLDS (`reference_xds_config_seam_transitive_cycle_guard`)
- **-race** `go test ./internal/tls/ ./internal/boot/ -race -count=1` → ok `tls` **1.126s**, ok `boot` **1.054s** (no `init_fetch_timeout` flake recurrence — `reference_sds_init_fetch_timeout_dial_budget_flake`)
- **counts (MECHANICAL):** fixtures **111** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0109-xds-sds-combined-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · DECISIONS tail **ADR-0288** (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1` — ADR-0287 COMPLETED in place, the tail did NOT flip)

## The entailment lesson (banked)

T3's two owed breaks are the phase's clearest teaching pair. Break 1 (Design C) proved the theorem's OBSERVABLE (served pool wins outright) by REFUTING the union at the unit level. The prefix break severed an ENTAILMENT: a break that fires two properties proves the second live ONLY if the second is not entailed by the first under that break — the bare-`GetName()` break made property (ii) (the `tls: ` prefix) entailed by (i) (reject-at-all), so an ISOLATING break (unwrap the error: (i) still fires, (ii) fails alone) was required to prove (ii) independently live (`reference_deliberate_break_wrong_assertion`). Together with the nil-provider guard recorded as deliberately unproven, this row's liveness log carries both a firing-but-non-discriminating break and a non-firing-by-construction guard, each recorded HONESTLY.

## Exit counts (re-derived on the landed tree)

stat surface **1201 (+0)** · fixtures **110 → 111** (`0109-xds-sds-combined-validation-context`) · fuzzers **55 (+0, seeds)** · BackendKind tail **38 (+0)** (`sdsserver` is DRIVER-owned, not a `BackendKind` — `reference_differential_grpc_receiver_driver_owned`) · +0 Go packages · go.mod modules **2 (+0)** · **ZERO new symbols in `internal/xds`** · DECISIONS tail **ADR-0288** (ADR-0287 COMPLETED in place; next-free **ADR-0289**). Row 66 flips `in-progress` → `done` at this IMPL six-gate (the SOLE leg — no parent rollup, ADR-0106).

**Landed task commits (LOCAL; squashed to ONE master commit at stage-close, located by SUBJECT):** T1 `57dbde17` · T2 `44669a78` · T3 `a315e810` (+ T3-debt discharge, no commit — tree byte-identical) · T4 `6cdf3db3` · T5 `88120f98` · T6 `187fb890` · T7 `51884ad1` · T8 `3f600cbc` (T9 this docs commit).
