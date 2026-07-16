# Phase 65 PROGRESS — xDS SDS `validation_context` (ADR-0286; row 65 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-16, worktree `.worktrees/phase-65-plan`, branch `phase-65-xds-sds-validation-context-plan`, off master `be419023`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 11.

## Task checklist (mirrors PLAN.md — 11 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [ ] Task 1 — `internal/xds/secret.go`: ADD `parseValidationSecret(resource, wantName, baseDir) (*x509.CertPool, error)` + the CVC-reject roster (`custom_validator_config` / `match_typed_subject_alt_names` / `verify_certificate_hash` / `verify_certificate_spki`, each `xds: sds: validation secret %q:`-prefixed, ADR-0080) + the FIRST production `crypto/x509` import in `internal/xds`; `secret_test.go` — `_Valid` (pool holds 1 subject) / `_WrongName` / `_WrongOneof` / `_CVCRejects` (4 rows) / `_NoTrustedCa` / `_BadPEM`; `TestParseSecret_WrongOneof` (`:175-187`) STAYS green (the two appliers stay DISJOINT); `-count=1` breaks on a dropped CVC arm + the PEM guard + the oneof check [TDD]
- [ ] Task 2 — `internal/xds/stream.go`: ADD `fetchValidationSecret` + `applyValidationResponse` (byte-parallel to `:38-95`, SHARING `errValidation` `:32` and `secretTypeURL()` — the Secret type URL is the SAME for both oneof arms, D1); `stream_test.go` (append after **`:179`**, not `:166`) — initial-request shape / ACK / NACK (prior-version + ErrorDetail) / transport-error-is-not-errValidation / empty-resources; `-count=1` breaks on the NACK version + ErrorDetail + the `%w` classification [TDD]
- [ ] Task 3 — `internal/xds/provider.go`: ADD `FetchInitialValidationContext` (parallel to `:47-75`, classification switch VERBATIM incl. `errValidation`-before-`ctx.Err()`) + the `SecretProvider` interface method (`:14-16`) **AND** `internal/tls`'s `fakeProvider` (`config_test.go:796-806`) gains it — **ONE COMMIT** (the interface change breaks `internal/tls`'s build otherwise; exactly TWO implementers repo-wide); `provider_test.go` (append after **`:160`**) — success / timeout / mgmt-down / rejected (+ `update_failure == 0` on reject); `-count=1` breaks on the attempt-counter position + the rejected→failure swap [TDD]
- [ ] Task 4 — `internal/tls/config.go`: lift the `:227-228` reject to a NO-OP gated `side != "downstream" || provider == nil` (**the `|| provider == nil` clause is CORRECTNESS, not defensiveness — QUIC reaches this arm with `side=="downstream"` AND a nil provider via `config.go:108`**); `config_test.go` — a regression FENCE: upstream STILL rejects / QUIC STILL rejects / downstream+nil-provider STILL refuses (byte-identical substring, ADR-0080); `-count=1` break on dropping the nil-provider clause → the QUIC arm must fire [TDD]
- [ ] Task 5 — `internal/tls/config.go`: `NewDownstreamConfig`'s `require_client_certificate` block (`:67-79`) gains the SDS branch (nil-provider reject → `xds.ParseSDSConfig` wrapped `tls: downstream: %w` → `FetchInitialValidationContext` → `cfg.ClientCAs`); the inline `else` stays BYTE-IDENTICAL; `config_test.go` — **the arm-5 subtest (`:936-957`) is REBUILT with a FULL `sds_config` (⚠️ C2 — the pre-65 input has NO `sds_config`; without the rebuild the ACCEPT flip is VACUOUS)** + a boot-FAIL test (ADR-0280 departure) + a `require==false`-INERT test (proving NO fetch is attempted); the rcc tests (`:286-316`/`:318-349`) STAY green; `-count=1` breaks incl. **the C2 vacuity proof** (revert the input → must fail `sds_config is required`) [TDD]
- [ ] Task 6 — `internal/boot/boot.go`: the `NewSDSProvider` pre-scan (`:138`) also detects `validation_context_sds_secret_config` via a NEW local `ctc` (**⚠️ without this T5's fetch path is UNREACHABLE in a real boot — `seen==0` → nil provider → the nil-provider reject; T8 depends on T5 AND T6**); `seen++` on BOTH arms so compose-two trips `seen>1` (`:147-148`, the DEFERRED edge); `boot_test.go` — validation-only-SDS builds a provider / both-via-SDS rejects / cert-only (`0103`) unchanged; `-count=1` breaks on the `seen++` + the whole arm [TDD]
- [ ] Task 7 — `test/helpers/sdsserver`: `WithValidationContext(name, trustedCAPEM)` + two fields + a `buildResponse` branch (`:118-136`); the generic TypeUrl derivation (`:133`) UNCHANGED (D1); the flat single-secret state STAYS flat (D3 — `0108` serves ONE secret per side; multi-secret is the deferred compose-two edge, do NOT refactor); helper unit test (right oneof + right trusted_ca + shared TypeUrl); `internal/xds/provider_test.go` (uses `WithSecret`) STAYS green; `-count=1` break on swapping the arms [TDD]
- [ ] Task 8 — NEW fixture `0108-xds-sds-validation-context` (`driver/`, the `0103` convention): in-memory PKI, **NO `pki/` dir** (D2 — 5 artifacts generated in `ensure()`; server cert injected `inline_string` into both yamls; no `HostMount`); two SDS receivers, one per side; static server cert + `validation_context_sds_secret_config` → `sds_cluster`; `node{id,cluster}` REQUIRED; the observable is a NORMALIZED two-arm verdict (D4 — `good=ok echo=…` + `bad=rejected`; **the handshake-failure TEXT is NEVER asserted — reference sends `unknown ca`, envoy-go sends `bad certificate`**); **NO `StatsAsserter` (⚠️ C3 — `ssl.fail_verify_error` DOES NOT EXIST in envoy-go)**; register at `runner_test.go` after `:134`; FULL `-run` selector; `-count=1` breaks (⚠️ two are SYMMETRIC and will NOT fire `CompareBytes` — see the residual risk below)
- [ ] Task 9 — fuzz SEEDS: `FuzzDiscoveryResponseParse` (`xds/fuzz_test.go:71`) gains a `validation_context` seed + the body drives `applyValidationResponse`; `FuzzTLSContextParse` (`tls/fuzz_test.go:24`, THREE-arg `f.Add`) gains a `require_client_certificate`+SDS-validation seed (nil-provider → the `tls: ` prefix invariant); prefer refactoring `selfSignedPEM` to `testing.TB` over a THIRD inline duplication; fuzzers **55 → 55** (reconcile before AND after, `reference_fuzzer_count_docs_drift`); delete any `testdata/fuzz/` corpus artifacts before commit
- [ ] Task 10 — `BEHAVIOR_CONTRACT.md` (**`:881`**, RE-DERIVED — item 5 of the phase-60 reject list; docs drift, re-confirm): downstream SDS `validation_context` REJECT → CONSUMED (fetch → `ClientCAs` + `RequireAndVerifyClientCert`; the ADR-0280 boot-FAIL departure extended; `require==false` inert; the CVC surface held; siblings STAY) + the `ssl.*` coverage boundary (C3)
- [ ] Task 11 — Verify: six-gate + **the cycle guard (`go list -deps ./internal/xds`, NO `...` — `internal/tls` MUST NOT appear)** + the full **110**-dir differential + ADR-0286 §Decision/§Consequences (recording the THREE PLAN-time corrections) + STATE + ROADMAP row 65 `done` + **the sentinel narrow at `:185`** + the sentinel re-run (does NOT fire — do NOT create `stop`) + PROGRESS close + router roll

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

## Baselines (fill at IMPL Task 1 — verbatim, against the phase-65 PLAN squash = master tip at IMPL start)

- `go build ./...`: [fill]
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): **109** at start, tail `0107-tracing-max-path-tag-length` (→ **110** after T8).
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`): **55** (55 after T9 — seeds only).
- BackendKind tail: **38** (`H2GoawayResponder`) — UNCHANGED (`sdsserver` is driver-owned, NOT a BackendKind — `reference_differential_grpc_receiver_driver_owned`).
- `go mod tidy -diff`: anticipated EMPTY (`crypto/x509` is stdlib; the tls protos are already resolved); modules stay **2**.
- stat surface: **1201** (+0 — the `sds.*` scope is DYNAMIC, keyed on the configured secret name; a new name yields new dynamic counters, not a new static-surface TYPE).
- DECISIONS tail: `## ADR-0285` at start (ADR-0286 body lands at T11; next-free ADR-0287).
- Cycle guard: `go list -deps ./internal/xds` → `internal/stats` + `internal/xds` ONLY. [re-confirm at T11]
- Anchors CONFIRMED vs the PLAN roster (RE-DERIVED against `be419023` — see the corrections above for the five that did NOT hold).

## Liveness-break log (every break `-count=1`, confirmed WHICH fired, then restored byte-identical) — fill at IMPL

- **T1 (applier, `secret_test.go`):** [fill]
- **T2 (stream arm, `stream_test.go`):** [fill]
- **T3 (provider, `provider_test.go`):** [fill]
- **T4 (reject-lift fence, `config_test.go`):** [fill — the `|| provider == nil` drop MUST fire the QUIC arm]
- **T5 (apply-point, `config_test.go`):** [fill — MUST include the C2 vacuity proof: reverting the arm-5 input fires `sds_config is required`]
- **T6 (boot pre-scan, `boot_test.go`):** [fill]
- **T7 (`sdsserver`):** [fill]
- **T8 (`0108` differential):** [fill — ⚠️ record which breaks were SYMMETRIC and how the structural check caught them; see the residual risk]
- **T9 (fuzz seeds):** [fill — count 55 before AND after]

## Task-11 verify evidence (fill at IMPL — verbatim, controller-run on the frozen HEAD)

- six-gate: [fill]
- cycle guard (`go list -deps ./internal/xds`, no `...`): [fill — `internal/tls` MUST NOT appear]
- 110-dir differential (`go test ./test/differential/ -count=1`): [fill]
- sentinel re-run (all three checks): [fill — anticipated: still does NOT fire; do NOT create `stop`]

**Landed task commits:** [fill T1…T11]. Squashed to a single master commit at stage-close by the controller.

**Exit counts (confirm against the landed tree):** stat surface **1201** (+0) · fixtures **109 → 110** (`0108-xds-sds-validation-context`) · fuzzers **55** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0285 → ADR-0286** (next-free **ADR-0287**). Row 65 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).

## ⚠️ Residual risk carried into the IMPL (the single most likely place phase 65 ships a vacuous test)

`PLAN.md` T8 Step 5's breaks (1) "serve a different CA" and (2) "sign `client_bad` with the served CA" are **SYMMETRIC** — they change BOTH sides identically, so a pure `CompareBytes` fixture still compares EQUAL and PASSES (`reference_vacuous_break_receiver_normalizes`). Without an additional **in-driver STRUCTURAL check** (the subject's own bytes match `good=ok` + `bad=rejected`), `0108` would prove only "both sides agree", NOT "the SDS-served CA is the actual trust anchor" — which is the entire point of the row. **The IMPL MUST resolve this and record the chosen mechanism here.** If breaks (1)/(2) cannot be made to fail, the fixture is vacuous and the row is not done.
