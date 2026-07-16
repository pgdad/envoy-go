# Phase 65 PROGRESS — xDS SDS `validation_context` (ADR-0286; row 65 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-16, worktree `.worktrees/phase-65-plan`, branch `phase-65-xds-sds-validation-context-plan`, off master `be419023`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 11.

## Task checklist (mirrors PLAN.md — 11 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [x] Task 1 — `internal/xds/secret.go`: ADD `parseValidationSecret(resource, wantName, baseDir) (*x509.CertPool, error)` + the CVC-reject roster (`custom_validator_config` / `match_typed_subject_alt_names` / `verify_certificate_hash` / `verify_certificate_spki`, each `xds: sds: validation secret %q:`-prefixed, ADR-0080) + the FIRST production `crypto/x509` import in `internal/xds`; `secret_test.go` — `_Valid` (pool holds 1 subject) / `_WrongName` / `_WrongOneof` / `_CVCRejects` (4 rows) / `_NoTrustedCa` / `_BadPEM`; `TestParseSecret_WrongOneof` (`:175-187`) STAYS green (the two appliers stay DISJOINT); `-count=1` breaks on a dropped CVC arm + the PEM guard + the oneof check [TDD]
- [x] Task 2 — `internal/xds/stream.go`: ADD `fetchValidationSecret` + `applyValidationResponse` (byte-parallel to `:38-95`, SHARING `errValidation` `:32` and `secretTypeURL()` — the Secret type URL is the SAME for both oneof arms, D1); `stream_test.go` (append after **`:179`**, not `:166`) — initial-request shape / ACK / NACK (prior-version + ErrorDetail) / transport-error-is-not-errValidation / empty-resources; `-count=1` breaks on the NACK version + ErrorDetail + the `%w` classification [TDD]
- [x] Task 3 — `internal/xds/provider.go`: ADD `FetchInitialValidationContext` (parallel to `:47-75`, classification switch VERBATIM incl. `errValidation`-before-`ctx.Err()`) + the `SecretProvider` interface method (`:14-16`) **AND** `internal/tls`'s `fakeProvider` (`config_test.go:796-806`) gains it — **ONE COMMIT** (the interface change breaks `internal/tls`'s build otherwise; exactly TWO implementers repo-wide); `provider_test.go` (append after **`:160`**) — success / timeout / mgmt-down / rejected (+ `update_failure == 0` on reject); `-count=1` breaks on the attempt-counter position + the rejected→failure swap [TDD]
- [x] Task 4 — `internal/tls/config.go`: lift the `:227-228` reject to a NO-OP gated `side != "downstream" || provider == nil` (**the `|| provider == nil` clause is CORRECTNESS, not defensiveness — QUIC reaches this arm with `side=="downstream"` AND a nil provider via `config.go:108`**); `config_test.go` — a regression FENCE: upstream STILL rejects / QUIC STILL rejects / downstream+nil-provider STILL refuses (byte-identical substring, ADR-0080); `-count=1` break on dropping the nil-provider clause → the QUIC arm must fire [TDD]
- [x] Task 5 — `internal/tls/config.go`: `NewDownstreamConfig`'s `require_client_certificate` block (`:67-79`) gains the SDS branch (nil-provider reject → `xds.ParseSDSConfig` wrapped `tls: downstream: %w` → `FetchInitialValidationContext` → `cfg.ClientCAs`); the inline `else` stays BYTE-IDENTICAL; `config_test.go` — **the arm-5 subtest (`:936-957`) is REBUILT with a FULL `sds_config` (⚠️ C2 — the pre-65 input has NO `sds_config`; without the rebuild the ACCEPT flip is VACUOUS)** + a boot-FAIL test (ADR-0280 departure) + a `require==false`-INERT test (proving NO fetch is attempted); the rcc tests (`:286-316`/`:318-349`) STAY green; `-count=1` breaks incl. **the C2 vacuity proof** (revert the input → must fail `sds_config is required`) [TDD]
- [x] Task 6 — `internal/boot/boot.go`: the `NewSDSProvider` pre-scan (`:138`) also detects `validation_context_sds_secret_config` via a NEW local `ctc` (**⚠️ without this T5's fetch path is UNREACHABLE in a real boot — `seen==0` → nil provider → the nil-provider reject; T8 depends on T5 AND T6**); `seen++` on BOTH arms so compose-two trips `seen>1` (`:147-148`, the DEFERRED edge); `boot_test.go` — validation-only-SDS builds a provider / both-via-SDS rejects / cert-only (`0103`) unchanged; `-count=1` breaks on the `seen++` + the whole arm [TDD]
- [x] Task 7 — `test/helpers/sdsserver`: `WithValidationContext(name, trustedCAPEM)` + two fields + a `buildResponse` branch (`:118-136`); the generic TypeUrl derivation (`:133`) UNCHANGED (D1); the flat single-secret state STAYS flat (D3 — `0108` serves ONE secret per side; multi-secret is the deferred compose-two edge, do NOT refactor); helper unit test (right oneof + right trusted_ca + shared TypeUrl); `internal/xds/provider_test.go` (uses `WithSecret`) STAYS green; `-count=1` break on swapping the arms [TDD]
- [x] Task 8 — NEW fixture `0108-xds-sds-validation-context` (`driver/`, the `0103` convention): in-memory PKI, **NO `pki/` dir** (D2 — 5 artifacts generated in `ensure()`; server cert injected `inline_string` into both yamls; no `HostMount`); two SDS receivers, one per side; static server cert + `validation_context_sds_secret_config` → `sds_cluster`; `node{id,cluster}` REQUIRED; the observable is a NORMALIZED two-arm verdict (D4 — `good=ok echo=…` + `bad=rejected`; **the handshake-failure TEXT is NEVER asserted — reference sends `unknown ca`, envoy-go sends `bad certificate`**); **NO `StatsAsserter` (⚠️ C3 — `ssl.fail_verify_error` DOES NOT EXIST in envoy-go)**; register at `runner_test.go` after `:134`; FULL `-run` selector; `-count=1` breaks (⚠️ two are SYMMETRIC and will NOT fire `CompareBytes` — see the residual risk below)
- [x] Task 9 — fuzz SEEDS: `FuzzDiscoveryResponseParse` (`xds/fuzz_test.go:71`) gains a `validation_context` seed + the body drives `applyValidationResponse`; `FuzzTLSContextParse` (`tls/fuzz_test.go:24`, THREE-arg `f.Add`) gains a `require_client_certificate`+SDS-validation seed (nil-provider → the `tls: ` prefix invariant); prefer refactoring `selfSignedPEM` to `testing.TB` over a THIRD inline duplication; fuzzers **55 → 55** (reconcile before AND after, `reference_fuzzer_count_docs_drift`); delete any `testdata/fuzz/` corpus artifacts before commit
- [x] Task 10 — `BEHAVIOR_CONTRACT.md` (**`:881`**, RE-DERIVED — item 5 of the phase-60 reject list; docs drift, re-confirm): downstream SDS `validation_context` REJECT → CONSUMED (fetch → `ClientCAs` + `RequireAndVerifyClientCert`; the ADR-0280 boot-FAIL departure extended; `require==false` inert; the CVC surface held; siblings STAY) + the `ssl.*` coverage boundary (C3)
- [x] Task 11 — Verify: six-gate + **the cycle guard (`go list -deps ./internal/xds`, NO `...` — `internal/tls` MUST NOT appear)** + the full **110**-dir differential + ADR-0286 §Decision/§Consequences (recording the THREE PLAN-time corrections) + STATE + ROADMAP row 65 `done` + **the sentinel narrow at `:185`** + the sentinel re-run (does NOT fire — do NOT create `stop`) + PROGRESS close + router roll

**PHASE 65 CLOSES AT IMPL.** Row 65 flips `in-progress` → `done` at this IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0286 (§Decision/§Consequences land in `DECISIONS.md`; §Context drafted at the SPEC, SPEC §13).

## D-SDSVC-* question dispositions (ALL DISPOSED at the SPEC — SPEC-65 §1/§11, via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2`; EVERY anticipation HELD — no ADR-0044 flip)

- **D-SDSVC-REFSERVE → CONFIRMED** (§11 arms 1/2/3): the reference SERVES + APPLIES an SDS `validation_context` as the mTLS client-cert trust anchor. Arm 1 (CA-signed client cert) → **200 `OK-MTLS`** + `ssl.handshake:1`; arm 2 (wrong/un-served CA) → `unknown ca` + `ssl.fail_verify_error:1`; arm 3 (no cert) → `certificate required` + `ssl.fail_verify_no_cert:1`. THREE DISTINCT outcomes ⇒ verification is LIVE, not vacuous.
- **D-SDSVC-RESOURCE → PINNED** (§11 `config_dump`): the served resource is `Secret{name, validation_context: CertificateValidationContext{trusted_ca: {inline_bytes}}}` under `dynamic_active_secrets` (the `Secret_ValidationContext` oneof, proto field 4 — the DYNAMIC SDS machinery, NOT static).
- **D-SDSVC-REQUIRE-SCOPE → PINNED** (§11 arm 3): `require_client_certificate: true` + an SDS `validation_context` = MANDATORY mTLS (the phase-65 scope, mirroring phase 16). `false` (verify-if-present) DEFERS and is INERT.
- **D-SDSVC-FETCHTIMEOUT → DERIVED** (§2.5, ADR-0280; not re-probed): the fetch reuses the phase-60.2 `FetchInitial*` timeout machinery, so the boot-FAIL DEPARTURE (vs the reference's serve-cert-less, probed at SPEC-60 §11 arm B) extends unchanged.
- **D-SDSVC-CVC-REJECT → RE-DERIVED** (§2.6, §6): the SDS applier mirrors the inline CVC sub-field reject roster (`internal/tls/config.go:234-245`) with `xds: sds:`-prefixed distinct substrings (`reference_strict_reject_sibling_typeurl_gap`). `crl` is unchecked on BOTH paths — a documented SHARED gap, not a new asymmetry.
- **D-SDSVC-PROVIDER → DECIDED = Option A** (§3.2): the parallel chain (`parseValidationSecret`/`applyValidationResponse`/`fetchValidationSecret`/`FetchInitialValidationContext`) returning `*x509.CertPool`; the landed `*stdtls.Certificate` chain UNTOUCHED (Option B's DRY generalization declined — the landed chain is load-bearing for the passing `0103`).
- **D-SDSVC-APPLY-POINT → DECIDED** (§3.3): fetch + install in `NewDownstreamConfig`'s `require_client_certificate` block (gated `require==true` ⇒ no wasteful fetch for the inert `false` case); `commonTLSContextToConfig` only no-ops the reject for downstream+provider. A REFINEMENT over BRAINSTORM §2.6.
- **D-SDSVC-BOOT-SCAN → DECIDED** (§3.4): extend `boot.NewSDSProvider`'s pre-scan (`boot.go:138`); compose-two DEFERRED via the existing `seen>1` guard.
- **D-SDSVC-FIXTURE → PINNED** (§8): ONE new dir `0108`; driver-owned `sdsserver`; the driver presents a client cert via its own `tls.Config{Certificates}` (NO runner API exists — re-confirmed at this PLAN).
- **D-SDSVC-NEGATIVE → DECIDED at the SPEC, CORRECTED at this PLAN** (see C3 below): positive-primary + a wrong-CA negative. **The SPEC's subject-side `ssl.fail_verify_error` `StatsAsserter` is INFEASIBLE and is REPLACED by a cross-side accept/reject CONTRAST.**
- **D-SDSVC-STATS → DECIDED** (§7): +0 counter TYPES; reuse the 5-counter `SDSStats` on a NEW dynamic `sds.<validation-secret>.*` scope. Surface STAYS **1201**.
- **D-SDSVC-FUZZSEED → DECIDED** (§6): SEEDS only; fuzzer count STAYS **55**.
- **D-SDSVC-SPLIT → DECIDED** (§3.0): a SINGLE FLAT ROW (11 tasks); the ADR-0045 escape-valve (65.1 applier / 65.2 wiring) documented ARMABLE but UNCONSUMED.
- **D-SDSVC-DOCSHAPE → RE-DERIVED** (§12) — **and CORRECTED at this PLAN; see below.**

## ⚠️ PLAN-time corrections to the SPEC (RE-DERIVED against master `be419023` by three independent readers — `feedback_brief_citations_not_evidence`)

The SPEC's line numbers are mostly EXACT. Five substantive defects were found. Full argument in `PLAN.md` §"PLAN-time corrections"; summary:

- **C1 🔴 `ParseSDSConfig` has 14 reject arms, not the SPEC's "9"** (`internal/xds/config.go:22-79`, mechanically counted). No build impact — it is REUSED verbatim — but no task should try to "mirror the 9".
- **C2 🔴 The T5 arm-5 test-flip is VACUOUS as specified.** `config_test.go:936-957` passes `SdsSecretConfig{Name: "validation-secret"}` with **NO `sds_config`**; routed through `ParseSDSConfig` it fires `sds_config is required` and never reaches the provider. The input MUST be REBUILT with a full `sds_config`. **The highest-risk vacuity trap in the phase.**
- **C3 🔴 `ssl.fail_verify_error` DOES NOT EXIST in envoy-go** — grep-confirmed: ZERO `ssl.*` stats in `internal/`+`cmd/`; the only listener-scope counter is `downstream_cx_total` (`internal/listener/manager.go:353`). The SPEC observed it on the REFERENCE (§11 arm 2) and assumed parity. **The §8 negative-arm `StatsAsserter` cannot be written.** CORRECTION: the negative arm asserts a driver-observed NORMALIZED verdict, CROSS-SIDE — strictly STRONGER than a subject-only stat (it proves cross-side agreement AND the anchor). The missing `ssl.*` handshake-outcome family is recorded as a NAMED COVERAGE BOUNDARY (`reference_close_direction_framework_gap` precedent) and added to the Observability deferred sentence at T11. Do NOT add the stat inline — that is a framework-surgery row and would blow the +0-stat envelope.
- **C4 🟡 Fixture-clone facts:** `0103`'s driver is `driver/driver.go` (NOT `inputs/`); `0018`'s DTC is `envoy-go.yaml:164-174` and is **`filename:`-based, NOT inline**; `CompareBytes` is `test/differential/diff.go:19` (NOT the fixture package); **the two clone-parents use INCOMPATIBLE PKI models** (`0103` COMMITS PEMs; `0018` GENERATES at `init()` + `.gitignore`s them) — resolved by D2 (in-memory, no `pki/` dir).
- **C5 🟡 Citation drift:** `NewQUICDownstreamConfig` is `:90-113` (`:87-88` is the doc comment) — its `:108` nil-provider call is EXACT and load-bearing; the inline-CVC block closes at `:246`; `loadDataSource` is `datasource.go:20`; `stream_test.go` spans `:55-179` and `provider_test.go` `:59-160` (the SPEC's `:166`/`:137` are where the LAST test BEGINS — wrong as insertion anchors); `WithSecret` is `:43-46`; `ensure()` is `:63-87`; the SPEC's `ctc` local does NOT exist today (the patch INTRODUCES it — not an error); "ALL test fakes" is exactly **ONE** (`fakeProvider`, `config_test.go:796-806`; two implementers repo-wide).

**Collision re-check (RE-GREPPED at this PLAN):** `parseValidationSecret` · `applyValidationResponse` · `fetchValidationSecret` · `FetchInitialValidationContext` · `WithValidationContext` — **all five FREE** (zero hits in ANY `.go` file repo-wide; only phase-65 planning-doc prose).

**Cycle guard VERIFIED at this PLAN:** `go list -deps ./internal/xds` (no `...`) → the entire envoy-go dep set is exactly `internal/stats` + `internal/xds`. `internal/tls` MUST NOT appear after the IMPL (`reference_xds_config_seam_transitive_cycle_guard`).

## PLAN-time design decisions (SPEC-delegated or C3/C4-forced)

- **D1** — the `tls.v3.Secret` type URL is SHARED by both oneof arms ⇒ `secretTypeURL()` reused verbatim; `sdsserver`'s generic derivation (`:133`) needs NO change; no second resource type on the wire.
- **D2** — `0108` PKI is generated **IN-MEMORY** in `ensure()`; **NO `pki/` dir**. Neither parent's model adopted (C4). The server cert is injected `inline_string:` into both yamls (the driver returns config STRINGS), the CA goes out over SDS `inline_bytes`, the client certs feed the driver's `tls.Config` — nothing touches disk, so no `HostMount`/mount/`.gitignore`. Freshness is safe: the observable is a verdict, not `0103`'s `serial=`. **`inline_string`, NOT `inline_bytes`** (proto `bytes` ⇒ protojson demands base64).
- **D3** — `0108` serves exactly ONE secret per side (static server cert) ⇒ `seen==1`; `sdsserver`'s flat state STAYS flat; the ignored `names` param and the `first`-gating hang risk do NOT bite. Do NOT generalize to multi-secret (the deferred compose-two edge; would risk `0103`).
- **D4** — the observable is a NORMALIZED two-arm verdict in ONE byte stream (`CompareBytes` takes one `[]byte`). `0103`'s `driveSide` (`:171-177`) is unusable — it reports what the SERVER presented; `0108` reports whether the proxy ACCEPTED our client cert.
- **D5** — extend `SecretProvider` (cost: exactly TWO implementations) rather than forking a second interface.
- **D6** — the `sds.<secretName>.*` stat scope keys on NAME only, not resource type; same-name secrets would share counters (idempotent, no panic). `0108` uses a distinct name; compose-two already rejects. No action; noted for a future row.

## Baselines (filled at IMPL — verbatim, against the phase-65 PLAN squash `a71ced0d` = master tip at IMPL start)

- `go build ./...`: clean (against master tip `a71ced0d`, the phase-65 PLAN squash).
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): **109** at start, tail `0107-tracing-max-path-tag-length` (→ **110** after T8).
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`): **55** (55 after T9 — seeds only).
- BackendKind tail: **38** (`H2GoawayResponder`) — UNCHANGED (`sdsserver` is driver-owned, NOT a BackendKind — `reference_differential_grpc_receiver_driver_owned`).
- `go mod tidy -diff`: anticipated EMPTY (`crypto/x509` is stdlib; the tls protos are already resolved); modules stay **2**.
- stat surface: **1201** (+0 — the `sds.*` scope is DYNAMIC, keyed on the configured secret name; a new name yields new dynamic counters, not a new static-surface TYPE).
- DECISIONS tail: `## ADR-0285` at start (ADR-0286 body landed at T11; next-free ADR-0287).
- Cycle guard: `go list -deps ./internal/xds` → `internal/stats` + `internal/xds` ONLY. **RE-CONFIRMED at T11 on the frozen HEAD — HOLDS** (see the verify block).
- Anchors CONFIRMED vs the PLAN roster (RE-DERIVED against `be419023` — see the corrections above for the five that did NOT hold). **⚠️ NINE FURTHER defects were found in the PLAN ITSELF at IMPL — see the log below.**

## Liveness-break log (every break `-count=1`, confirmed WHICH fired, then restored byte-identical) — filled at IMPL

> **⚠️ HONESTY NOTE on this block's provenance.** Each task ran as a fresh subagent that committed locally; the per-break transcripts were NOT preserved in-tree (no commit message or test comment carries them). Entries here draw on three DISTINCT provenance classes, and each is labelled:
>
> 1. **CONTROLLER-RELAYED** — the T1/T2/T4/T7 transcripts below are quoted **verbatim from the task subagents' own reports**, held by the controller and transcribed here at a later pass. They were **NOT re-run** when this record was written, and **neither the T11 agent nor the transcribing agent observed them directly.** They are the subagents' claims, faithfully relayed — one link stronger than anticipation, one link weaker than a transcript this session re-derived. A relayed report is not a re-derivation (`feedback_brief_citations_not_evidence`).
> 2. **IN-TREE / STRUCTURAL** — verifiable today from the landed tree, the commit ledger, or the `0108` README's in-tree break record.
> 3. **CONTROLLER'S OWN FINDINGS** — observed by the controller directly (e.g. the T11 verify block).
>
> Where a detail was NOT preserved in ANY class it still says so — it is never reconstructed from anticipation. An unverifiable PROGRESS entry is worth less than an honest gap.

- **T1 (applier, `secret_test.go`, `fc68ef07`) — 3 breaks, all `-count=1`.** [provenance: CONTROLLER-RELAYED from the T1 subagent's report; not re-run here.] The applier tests landed (`_Valid` / `_WrongName` / `_WrongOneof` / the 4-row `_CVCRejects` / `_NoTrustedCa` / `_BadPEM`) and `TestParseSecret_WrongOneof` (`:175-187`) STAYS green unchanged — the two appliers are DISJOINT (each rejects the other's oneof arm), which is a STRUCTURAL property of the landed code, not a test result.
  - **Break 1** — deleted the `verify_certificate_hash` reject. Exactly ONE test fired:
    ```
    --- FAIL: TestParseValidationSecret_CVCRejects/verify_certificate_hash
        secret_test.go:311: expected error, got nil
    ```
    The three sibling subtests (`custom_validator_config`, `match_typed_subject_alt_names`, `verify_certificate_spki`) all still PASSed — **the 4-row roster is not vacuously passing on a shared earlier arm.**
  - **Break 2** — `if !pool.AppendCertsFromPEM(caPEM)` → `_ = pool.AppendCertsFromPEM(caPEM)`. Exactly ONE test fired:
    ```
    --- FAIL: TestParseValidationSecret_BadPEM
        secret_test.go:340: expected error, got nil
    ```
    **`_Valid` explicitly PASSed — its `len(pool.Subjects()) != 1` check did NOT fire**, so the pool is genuinely populated and `_Valid` was not vacuous.
  - **Break 3** — accept the wrong oneof arm. Exactly ONE test fired:
    ```
    --- FAIL: TestParseValidationSecret_WrongOneof
        secret_test.go:272: error = "xds: sds: validation secret \"validation_ca\": trusted_ca: xds: sds: data source: none of inline_bytes, inline_string, filename set", want it to contain "is not a validation_context"
    ```
  - **⚠️ PLAN defect (break instruction):** the PLAN's LITERAL break (`GetValidationContext()` → `GetTlsCertificate()`) **does not compile** — `*tlsv3.TlsCertificate` has no `GetCustomValidatorConfig`/`GetTrustedCa`/etc., so it is a BUILD failure, which proves nothing about WHICH assertion fires. **DEVIATION:** a compiling equivalent of identical intent was substituted — `if vc == nil && sec.GetTlsCertificate() == nil`.
  - **⚠️ Incidental:** `golangci-lint`'s `misspell` rejected the PLAN's British "licence" in two comments → "license" (prose only).
- **T2 (stream arm, `stream_test.go`, `e56cf154`) — 3 breaks, all `-count=1`, FULL-package runs so a wrong-test fire would be visible.** [provenance: CONTROLLER-RELAYED from the T2 subagent's report; not re-run here.] The stream-arm tests landed (initial-request shape / ACK / NACK / transport-error-is-not-`errValidation` / empty-resources), appended after `:179`. Each break fired exactly ONE assertion, in the ANTICIPATED test, with NO collateral failures.
  - **Break 1** — NACK `VersionInfo` → `resp.GetVersionInfo()`:
    ```
    --- FAIL: TestFetchValidationSecret_NackOnValidationFailure (0.00s)
        stream_test.go:267: NACK VersionInfo = "v2", want empty (keep the PRIOR version on reject)
    ```
  - **Break 2** — dropped the NACK `ErrorDetail`:
    ```
    --- FAIL: TestFetchValidationSecret_NackOnValidationFailure (0.00s)
        stream_test.go:273: NACK ErrorDetail is nil, want the validation failure detail
    ```
  - **Break 3** — dropped the `errValidation` wrap in `applyValidationResponse`:
    ```
    --- FAIL: TestFetchValidationSecret_NackOnValidationFailure (0.00s)
        stream_test.go:260: error = xds: sds: secret "validation_ca" is not a validation_context (unsupported oneof arm), want it to wrap errValidation
    ```
    Break 3 confirms **the NACK classification is LIVE** ⇒ T3's `update_rejected` accounting keys off a REAL signal, not a constant.
  - **⚠️ PLAN defect (break instruction):** the PLAN's LITERAL break 3 ("drop the `%w: `") leaves `fmt.Errorf("%v", errValidation, err)` — two args, one verb — which **`go vet` rejects**. **DEVIATION:** substituted `fmt.Errorf("%v", err)` — identical intent, compiles, and fired the anticipated assertion.
  - **Landed chain byte-untouched (verified in-tree by the subagent):** `git diff fc68ef07 -- internal/xds/stream.go` → **72 insertions, 0 deletions**; `grep -c '^-[^-]'` over the diff → **0**. `0103` untouched.
- **T3 (provider, `provider_test.go`):** **⚠️ break 2 did NOT fire — a REAL, RECORDED coverage gap.** Swapping the `errValidation`-before-`ctx.Err()` classification ordering changed NO test outcome: **no test creates the discriminating condition** (a validation failure racing a live deadline). The ordering is copied VERBATIM from the landed `FetchInitialCertificate` (`provider.go:47-75`) and inherits that arm's semantics, but **phase 65 adds NO coverage for it and none is claimed.** The break not firing IS the finding, not a nuisance to route around. **⚠️ PLAN defect found here:** the PLAN's test snippet used `counterValue`, which **does not exist**.
- **T4 (reject-lift fence, `config_test.go`):** **⚠️ the PLAN's premise was WRONG.** The PLAN specified an upstream-STILL-rejects subtest as part of the fence; it is **unreachable and vacuous** — `validation_context_type` is a **oneof**, so selecting the SDS arm makes `GetValidationContext()` return nil and `NewUpstreamConfig` refuses EARLIER with `tls: upstream: validation_context.trusted_ca is required (...)`, **NOT** the phase-03 SDS substring. **QUIC is the guard's ONLY live consumer** — via the `provider == nil` half (`config.go:108` passes `side == "downstream"` with a nil provider, C5's correction, which is why that clause is CORRECTNESS and not belt-and-braces). The `side != "downstream"` half is therefore DEAD from today's entry points; it is RETAINED so a future upstream caller cannot silently skip validation. **This false claim reached a SHIPPED code comment and was corrected in T5-fix `af55ac9e`** (`feedback_brief_citations_not_evidence`: the controller's uncited PROSE asserted the opposite of the control flow).
  - **1 break, `-count=1`** (`d37f084a`) [provenance: CONTROLLER-RELAYED from the T4 subagent's report; not re-run here] — dropped the `|| provider == nil` clause:
    ```
    --- FAIL: TestValidationContextSDS_SiblingRejectsStay/quic_downstream_(nil_provider)_still_rejects
        config_test.go:1157: quic error = "tls: downstream: no tls_certificates configured", want it to contain "SDS-bound validation_context_sds_secret_config is not supported in phase 03"
    --- FAIL: TestValidationContextSDS_SiblingRejectsStay/downstream_with_NIL_provider_still_rejects
        config_test.go:1177: downstream/nil-provider error = "tls: downstream: no tls_certificates configured", ...
    --- PASS: TestValidationContextSDS_SiblingRejectsStay/upstream_still_rejects
    ```
    The intended `quic downstream (nil provider)` subtest fired on the intended `wantSub` assertion (`:1157`) — **NOT an earlier Fatal** (`reference_deliberate_break_wrong_assertion`). **C5 is thereby proven FROM BEHAVIOR, not from prose:** QUIC DOES reach the arm with `side=="downstream"` + a nil provider, and without the clause it falls straight through — the error degrades to "no tls_certificates configured", i.e. **validation is silently SKIPPED.**
  - **Regression fence (as designed):** pre-change all 3 subtests PASS — correct for a fence, which pins existing behavior rather than driving new behavior — and post-change all 3 still PASS. The break above is what makes the fence non-vacuous.
- **T5 (apply-point, `config_test.go`):** **⚠️ the C2 vacuity proof FIRED as predicted** — reverting the arm-5 input to the pre-65 `&tlsv3.SdsSecretConfig{Name: "validation-secret"}` (no `sds_config`) fails with **`tls: downstream: xds: sds: SdsSecretConfig sds_config is required`**: it dies at `ParseSDSConfig` (`internal/xds/config.go:33`) and **never reaches the provider**, so the reject→ACCEPT flip against that input would have proven NOTHING. The input was REBUILT with a full `sds_config` (via `sdsSecretConfig`, `config_test.go:813`). **Break 3 proved the `provider == nil` guard is UNREACHABLE dead code** — `commonTLSContextToConfig` refuses first; retained as documented defense-in-depth, NOT the live gate. **⚠️ TWO further PLAN defects:** the PLAN's snippet did not compile (`cfg.ClientCAs` vs the real `cfg.TLSConfig.ClientCAs`), and the boot-FAIL subtest **PASSED pre-implementation** (vacuous) until an `initial fetch timed out` assertion was added to make it bite.
- **T6 (boot pre-scan, `boot_test.go`):** validation-only-SDS builds a provider / both-via-SDS rejects (`seen>1`) / cert-only (`0103`) unchanged. **⚠️ TWO PLAN defects:** every helper name in the PLAN's snippet was WRONG (the real idiom is YAML-template based), and **the PLAN's break-2 expectation was logically impossible** — it reproduces RED rather than discriminating.
- **T7 (`sdsserver`):** `WithValidationContext` + two fields + the `buildResponse` branch; the generic TypeUrl derivation (`:133`) UNCHANGED (D1); the flat single-secret state STAYS flat (D3); `internal/xds/provider_test.go` (uses `WithSecret`) STAYS green. **⚠️ FINDING: mutual exclusion is last-branch-wins, NOT enforced** — passing both `WithSecret` and `WithValidationContext` silently serves the validation_context regardless of option order. Per D3 no guard was added; noted for a future compose-two row. **⚠️ PLAN defect:** the snippet did not compile (`New(t, ...)` takes `t` and returns no error).
  - **1 break, `-count=1`** (`695d7765`) [provenance: CONTROLLER-RELAYED from the T7 subagent's report; not re-run here] — swapped the `buildResponse` switch arms (the cert secret served when `vcSecretName != ""`):
    ```
    === RUN   TestWithValidationContext_ServesValidationSecret
        sdsserver_test.go:195: Secret is not a validation_context — wrong oneof arm served
    --- FAIL: TestWithValidationContext_ServesValidationSecret (0.00s)
    ```
    The intended assertion fired and was the **ONLY** failure line: the break deliberately PRESERVED `Name: s.vcSecretName`, so the `Secret.Name` check stayed green and the failure **isolated cleanly on the oneof arm** — no wrong-assertion masking (`reference_deliberate_break_wrong_assertion`).
  - **The RED step was GENUINE:** `undefined: WithValidationContext` — a real compile failure, not a pre-passing test.
  - **Cert-arm guard GREEN:** `go test ./internal/xds/ -count=1` → `ok  0.261s` (the `0103` path did not regress).
- **T8 (`0108` differential):** ⚠️ **see the residual-risk resolution below — this is the phase's load-bearing break log.** In-tree record: `test/fixtures/0108-xds-sds-validation-context/README.md` §"Why the driver carries a structural check". Breaks (1) and (2) are **SYMMETRIC** and were caught by `structuralCheck`, NOT by `CompareBytes`; **the trap was DEMONSTRATED** (with the check disabled, break (1) ships PASS). **⚠️ PLAN defect: break (3) was itself VACUOUS** (both receivers serve the same CA ⇒ a no-op); T8 substituted a genuinely ASYMMETRIC break (the subject's receiver serves the UNSERVED CA), which fires `CompareBytes` with **a mismatch at byte offset 5** when the structural check is disabled — that is how `CompareBytes` was proven live. FULL selector throughout (`reference_differential_run_selector`). **⚠️ TLS-1.3 hazard handled:** under TLS 1.3 the client's `Handshake()` returns BEFORE the server's verdict on the client cert, so a handshake-only probe would report a REJECTION as SUCCESS — silently inverting the negative arm; `mtlsEcho` drives the FULL round trip (handshake → write → read), which is version-independent.
- **T9 (fuzz seeds):** fuzzers **55 before AND 55 after** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`; reconciled both ways per `reference_fuzzer_count_docs_drift`) — SEEDS only, no new `Fuzz*` func. `FuzzDiscoveryResponseParse` gains a `validation_context` seed and its body now also drives `applyValidationResponse`; `FuzzTLSContextParse` gains seed (e) — `require_client_certificate=true` + SDS validation_context, pinning the `"tls: "` prefix invariant across the nil-provider reject. **Duplication RETIRED, not added:** `selfSignedPEM` took `testing.TB` cleanly (12 call sites, ZERO ripple), so `mustValidSecretAnyBytes` now calls it instead of inlining a THIRD copy of cert generation. **⚠️ PLAN defect: the seed rationale was false.** No `testdata/fuzz/` corpus artifacts were committed. **⚠️ COVERAGE GAP (honest):** the `tls: downstream: %w` wrap of `ParseSDSConfig` is **NOT** fuzz-covered — seed (e) reaches the `commonTLSContextToConfig` reject FIRST (a LIVE provider is needed to reach the wrap, and the fuzzer passes none). Its coverage rests on the unit tests, which do supply a `fakeProvider`.

**⚠️ ELEVEN PLAN defects found by RE-DERIVATION across T1–T10.** `feedback_brief_citations_not_evidence` earned its keep on a PLAN that had ITSELF corrected five SPEC defects: `counterValue` did not exist (T3) · the T5 snippet did not compile (`cfg.ClientCAs` vs `cfg.TLSConfig.ClientCAs`) · the T4 upstream subtest was unreachable/vacuous (the oneof finding) · a T5 boot-FAIL subtest passed pre-implementation · every T6 helper name was wrong · T6's break-2 expectation was logically impossible · the T7 snippet did not compile · T8's break (3) was vacuous · T9's seed rationale was false · **the T1 break-3 instruction did not compile** (`*tlsv3.TlsCertificate` lacks the accessors the arm calls) · **the T2 break-3 instruction was `go vet`-rejected** (dropping the `%w: ` leaves `fmt.Errorf("%v", errValidation, err)` — two args, one verb). **A PLAN is not evidence either.**

> **⚠️ A defect CLASS the earlier tally missed: the PLAN's BREAK instructions are as defect-prone as its test snippets — and they fail more quietly.** Two of the eleven (T1 break 3, T2 break 3) are breaks that **do not build**. That is the worst failure mode in this block: a non-compiling break yields a red screen that LOOKS like a fired assertion but proves **NOTHING about which assertion is live** — the very property the break exists to establish, and a close cousin of `reference_deliberate_break_wrong_assertion`. Both were caught only because the subagents substituted a COMPILING equivalent of identical intent and then re-derived WHICH test fired. **A break must RUN to be evidence.**

## ⚠️ The `-race` finding — observed ONCE; PRE-EXISTING, NOT a phase-65 regression

One run of `go test -race -count=1 ./internal/xds/... ./internal/tls/... ./internal/boot/... ./test/helpers/sdsserver/...` failed:

```
--- FAIL: TestProvider_FetchInitialCertificate_Timeout (0.21s)
    provider_test.go:116: init_fetch_timeout = 0, want 1
```

It was **INVESTIGATED, not reflex-classified** (`reference_0061_ring_hash_spread_flake` — a first occurrence still deserves a mechanism, not a shrug):

- The test is **BYTE-IDENTICAL to master** — verified by diffing the function body; phase 65 only APPENDED to `provider_test.go` (`@@ -158,3 +162,133 @@`).
- Master and branch each pass the identical 4-package `-race` command **3/3**; the branch full-package passes **3/3**; **6** further runs under full 32-core CPU saturation all passed. Could NOT be force-reproduced.
- **Mechanism:** the test's 200ms budget covers BOTH the gRPC dial and the recv. If `StreamSecrets(ctx)` misses that budget it returns an `open stream` error → `incUpdateFailure` → `init_fetch_timeout` stays 0, while `err != nil` and `cert == nil` keep **every other assertion green** — exactly matching the single observed failure line.
- Phase 65's new validation tests use `vcFakeOpener` (a **FAKE** — no gRPC dial), so they are **structurally immune**.

**Conclusion: a PRE-EXISTING latent timing sensitivity in a LANDED MASTER test, not a phase-65 regression.** The master test was deliberately **NOT** "fixed" in this row — that is a separate concern with its own evidence bar.

## Task-11 verify evidence (controller-run on the frozen HEAD `af55ac9e` — verbatim)

- **six-gate — ALL GREEN:**
```
gofmt -l internal/ test/ cmd/        -> SILENT
go vet ./...                         -> exit 0
go build ./...                       -> exit 0
go mod tidy -diff                    -> EMPTY (exit 0)
git diff --exit-code master -- go.mod go.sum -> EMPTY (modules STAY 2)
golangci-lint run ./...              -> exit 0 (clean)
```
- **cycle guard** (`go list -deps ./internal/xds | grep 'envoy-go/internal'` — **NO `...`**, `reference_xds_config_seam_transitive_cycle_guard`):
```
github.com/pgdad/envoy-go/internal/stats
github.com/pgdad/envoy-go/internal/xds
```
  ⇒ **`internal/tls` does NOT appear. GUARD HOLDS.** (`parseValidationSecret` DUPLICATES the CertPool build rather than calling `internal/tls.loadTrustedCAPool` — `internal/tls` imports `internal/xds` at `config.go:13`, so the reverse edge would cycle.)
- **110-dir differential** (`go test ./test/differential/ -count=1`): **`ok  375.884s`, EXIT=0** — the FULL 110-dir suite. The 109 pre-existing dirs byte-stable (phase 65 LIFTS a reject; it cannot change any passing fixture's bytes) plus the new `0108`.
- **sentinel re-run (all three checks, MECHANICAL) — does NOT fire; `stop` NOT created:**
  - **(1) prints NOTHING** — row 65 is now `done`, and every other ROADMAP row already was. Check (1) NO LONGER blocks `stop`.
  - **(2) prints THREE live "candidates:" sentences** — HTTP/3 (`:175`, unchanged), xDS (`:185`, **NARROWED** `SDS validation_context/upstream SDS` → `upstream SDS (server-cert + validation_context)`), Observability (`:193`, **EXTENDED** with the `ssl` handshake-outcome family per C3). ⇒ three families STAY OPEN. **NOTE: line `:193` ALSO carries a HISTORICAL `candidates were:` recap — not the live sentence** (`reference_sentinel_deferred_sentence_live_vs_historical`).
  - **(3) prints `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`** ⇒ THREE families never opened.
  - **Checks (2) and (3) print ⇒ the sentinel does NOT fire.** `stop` was NOT created (`ls stop` → No such file or directory).
- **counts RE-DERIVED on the landed tree (not copied):**
  - **stat surface 1201 (+0)** — method: there is NO mechanical counting command in this repo (phases 62/63 state so explicitly: *"docs-verified; registration guards enforce +0 — no counting command"*). Re-derivation is two-part: (i) `BEHAVIOR_CONTRACT.md`'s authoritative figure still reads **1201** (`:827`), UNCHANGED by this row; (ii) `git diff master -- '*.go' | grep -E '^\+.*(NewCounter|NewGauge|NewHistogram|RegisterSDSStats)'` returns **FOUR** hits, **all four in `internal/xds/provider_test.go`** — **ZERO production stat-registration call sites added**. ⇒ **+0**. The `sds.<secretName>.*` scope is DYNAMIC (keyed on the configured secret name): a new name yields new dynamic counters, not a new static-surface TYPE.
  - **BackendKind 38 (+0)** — method: the kinds are EXPLICIT numeric constants in `test/differential/fixture/fixture.go`; `grep -nE '^\t[A-Za-z0-9_]+ BackendKind = [0-9]+' test/differential/fixture/fixture.go | tail -1` → **`H2GoawayResponder BackendKind = 38`** (39 constants, 0-indexed ⇒ tail 38). `git diff master --stat` shows `fixture.go` is **NOT** in the diff at all. The `sdsserver` is DRIVER-owned, not a `BackendKind` (`reference_differential_grpc_receiver_driver_owned`).
  - fixtures **110** (`ls -d test/fixtures/[0-9]*/ | wc -l`; tail `0108-xds-sds-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · DECISIONS tail **`## ADR-0286`** (`grep -oE '^## ADR-[0-9]{4}' docs/envoy-go/DECISIONS.md | tail -1`) · modules **2**.

**Landed task commits:** T1 `fc68ef07` · T2 `e56cf154` · T3 `6c6c5156` · T4 `d37f084a` · T5 `d45e6b98` (+ T5-fix `af55ac9e`) · T6 `b654bae7` · T7 `695d7765` · T8 `b233af5c` · T9 `21608151` · T10 `d337f6dc` (T11 this docs commit). All LOCAL (`feedback_subagents_no_push`); squashed to a single master commit at stage-close by the controller, located by SUBJECT (`git log --grep`), never by position.

**Exit counts (CONFIRMED against the landed tree — see the re-derivation methods above):** stat surface **1201** (+0) · fixtures **109 → 110** (`0108-xds-sds-validation-context`) · fuzzers **55** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0285 → ADR-0286** (next-free **ADR-0287**). Row 65 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).

## ⚠️ THE RESIDUAL RISK — **RESOLVED at T8**; the mechanism, and the proof that the trap was REAL

**The risk as flagged at the PLAN.** `PLAN.md` T8 Step 5's breaks (1) "serve a different CA over SDS" and (2) "sign `client_bad` with the served CA" are **SYMMETRIC** — they change BOTH sides identically, so a pure `CompareBytes` fixture still compares EQUAL and PASSES (`reference_vacuous_break_receiver_normalizes`). Without an in-driver **STRUCTURAL** check, `0108` would prove only *"both sides agree"*, NOT *"the SDS-served CA is the actual trust anchor"* — which is the entire point of the row. The PLAN made resolving this a **completion condition**: *"If breaks (1)/(2) cannot be made to fail, the fixture is vacuous and the row is not done."*

**The mechanism shipped (T8).** `structuralCheck(side, out)` at the end of `driveSide`, returning an `error`. The runner turns a Drive error into `t.Fatalf("ref drive: …")` / `t.Fatalf("subj drive: …")`, so a symmetric break **fails loudly AND names the side**. Both arms are checked independently and all violations report together (`reference_fatalf_makes_assertions_unreachable` — `Errorf` per independent property, not a `Fatalf` that strands the second arm). In-tree record: `test/fixtures/0108-xds-sds-validation-context/README.md` §"Why the driver carries a structural check".

**⚠️ THE TRAP WAS DEMONSTRATED, NOT ASSUMED.** With `structuralCheck` DISABLED, break (1) — serve a **DIFFERENT** CA over SDS — ships **PASS**: both sides emit

```
good=REJECTED err=handshake-or-roundtrip-failed
bad=ACCEPTED
```

…and compare **EQUAL**. **A pure-`CompareBytes` fixture would have shipped GREEN on a completely broken trust anchor.** With the check ENABLED, break (1) and break (2) (sign `client_bad` with the SERVED CA → `bad=ACCEPTED`) both **FAIL loudly**.

**The passing byte stream (both sides):**

```
good=ok echo=phase65-mtls-probe
bad=rejected
```

**⚠️ The PLAN's break (3) was ITSELF vacuous** — both receivers serve the same CA, so it is a no-op. T8 substituted a genuinely **ASYMMETRIC** break (the subject's receiver serves the UNSERVED CA), which fires `CompareBytes` with **a mismatch at byte offset 5** when the structural check is disabled. That is how `CompareBytes` was proven live rather than assumed live.

**⚠️ Honest consequence — recorded, not glossed.** Since exactly ONE structurally-valid stream exists, the structural check **strictly SUBSUMES `CompareBytes` for this fixture**: any deviation trips the structural check first. `CompareBytes` is retained as the harness-standard cross-side leg, but `0108`'s cross-side value is *agreement on a shape both sides independently satisfy*, not a discriminating byte comparison. The proof obligation is nevertheless discharged — and by a **stronger** instrument than the SPEC's infeasible subject-only `ssl.fail_verify_error` `StatsAsserter` (C3), because the accept/reject **contrast** proves the anchor AND cross-side agreement, while a subject-only stat proves nothing cross-side.

**Also normalized (never asserted as text).** The reference (BoringSSL) sends the TLS alert `unknown ca`; envoy-go (Go `crypto/tls`) sends `bad certificate`; and the reject can additionally surface as `client didn't provide a certificate` (Go's TLS client withholds a cert that does not match the `CertificateRequest`'s acceptable-CA hint — a hint itself derived from the SDS-served CA). Same proposition, three manifestations. The negative arm records only the stable token `rejected` (the `0045` `closeOK` idiom) — a driver asserting the error string cross-side would fail 100% of the time.
