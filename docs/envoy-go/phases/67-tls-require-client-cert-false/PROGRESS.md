# Phase 67 PROGRESS — TLS `require_client_certificate: false` / verify-if-presented mTLS (ADR-0289; row 67 flips `done` at the IMPL six-gate — the SOLE leg, ADR-0106)

> SCAFFOLD written at the PLAN stage; the IMPL fills statuses, commits, break ACTUALs, and the six-gate evidence. Execution is subagent-driven (`feedback_execution_style` / `feedback_subagent_autocommit_claudemd` / `feedback_subagents_no_push`): each task commits LOCALLY only, controller squash-pushes at stage-close (commits located by SUBJECT — `git log --grep 'phase 67'`, never by position). The row's defining envelope: **ONE functionally-edited production file (`internal/tls/config.go`)**; chartered comment-only exceptions B16 (`internal/xds/provider.go`), B17 (`internal/tls/config_test.go:999`), B19 (`internal/boot/boot_sds_e2e_test.go` — the post-SPEC absorption, PLAN A2); `boot.go` / `internal/listener` / `validate/` / `sdsserver` BYTE-UNTOUCHED. ADR-0289 is COMPLETED IN PLACE (no new ADR; tail stays **ADR-0289**, next-free **ADR-0290**).

## Counts at entry (re-derived at `a15f4fca`, dossier `[RUN]`)

- fixtures **111** (tail `0109-xds-sds-combined-validation-context`) → anticipated **112**
- fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) → anticipated **55 (+0, seeds only)**
- DECISIONS tail **ADR-0289** (§Context drafted at the SPEC squash; next-free ADR-0290) → completed IN PLACE, tail unchanged
- BackendKind tail **38** (`H2GoawayResponder`, `test/differential/fixture/fixture.go:614`) → **38 (+0)**
- stat surface **1201** → **1201 (+0)**
- go.mod: +0 (SPEC lineage metric "2" carried; `git diff go.mod` re-checked after tidy)
- baseline green: `go test ./internal/tls/ ./internal/boot/ ./internal/listener/ ./internal/xds/ ./internal/filter/hcm/ -count=1` → all ok; every flip-roster test GREEN (the inversions are observable red)

## Task checklist (mirrors PLAN-67 — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape valve ARMABLE/unconsumed)

| T | Title | Status | Commit |
|---|---|---|---|
| T1 | ATOMIC core: hoist + three-way `ClientAuth` (assignment-adjacent) + E3 RETIRED + flip roster items 1–4 (red-first inversions) + theorem block moved intact + retained rejects byte-diffed + stays-green roster | done | c0144adc |
| T2 | Mapping cross-product unit tests (3 shapes × {false, absent}, anchorless, corrupt-CA, the require=false fetch-failure vcErr arm — amendment M9) + `TestVerifyIfGiven_NilPool_Unconstructible` (interface-pinned) | done | 50bb10db |
| T3 | Fuzz seeds on correct dispatch sides (+0 fuzzers; count reconciled 55 → 55) | done | 86967b0b |
| T4 | Fixture `0110-tls-require-client-cert-false` — CVC-primary, three arms, FORCED-SEND untrusted arm (fixtures 111 → 112) | done | d6bee060 |
| T5 | 0110 break protocol: Break B + structuralCheck triplet + per-side pin + served-this-arm | done | d6bee060 (breaks; tree byte-identical to T4) |
| T6 | Comment sweeps: B11/B18 (config.go) + B16 (provider.go, chartered) + B17 (:999) + **B19** (boot_sds_e2e, chartered — PLAN A2) + the 0109 enumerated set + grep discharged (three-category dispositions, M10) + non-drift dispositions (A5 + BC:914); T6-SCOPED `.go` grep only — the full-repo drift closure moved to T9 (MOD-2) | done | 3a231c1d |
| T7 | BC delta B1–B10/B13–B14 (pinned verbatim) + FOUR-site TEST_GAP sweep (B15) by claim+grep (A1); B11@T6, B12@T9 (M8) | done | 6e8f7f81 (+35f13315 B6 fixup) |
| T8 | VERIFY: six-gate + cycle guard + full 112-dir differential + `-race` + mechanical counts + envelope audit | done | — (verify; evidence below) |
| T9 | ADR-0289 completed IN PLACE + B12 annotation + ROADMAP/STATE/PROGRESS/router + sentinel + squash-push | done | aba88dfc (ADR) + stage-close squash |

## Red-first ledger (T1 — filled with VERBATIM observed failures; a red for the wrong reason is VACUOUS, stop)

- [ ] `TestNewDownstreamConfig_RequireFalse_CVC_VerifyIfGiven` (replacing retired `TestCVC_RequireFalse_Rejected_E3`, both subtests) — expected red: the E3 substring where success+IfGiven is now wanted. ACTUAL: —
- [ ] `TestCVC_RequireFalse_NeverYieldsNoClientCert` (err-half inverted, both subtests) — expected red: the E3 error where nil err is now wanted. ACTUAL: —
- [ ] `TestNewDownstreamConfig_SDS` subtest (ex-"INERT", three assertions inverted) — expected red: **TWO observables** — `ClientCAs` nil / `NoClientCert` (the "no fetch fired" pre-state has no observable without `fakeProvider` call-recording — IMPL's choice, not owed; amendment M11). ACTUAL: —

## Break ledger (every break `-count=1`; confirm WHICH assertion fired; `git restore` only; a break that does NOT fire is a FINDING)

> **Mechanism-conditionality (PLAN Break protocol, BINDING — MOD-3):** A/C/D's pre-compiled-verbatim status and the corrupt-CA arity hold ONLY in the shared-closure shape. Per-arm adjacency ⇒ Break A becomes one-swap-PER-ARM (three edits, each firing its own shape's `Errorf`; a non-firing shape is then a real finding) and C/D need IMPL-time compile re-verification (`reference_plan_break_instructions_dont_compile` fresh). Record WHICH mechanism landed.

| Break | Task | Pre-compiled? | Expected firing assertion | ACTUAL (verbatim at IMPL) |
|---|---|---|---|---|
| **A** — anchored false cell → `NoClientCert` (identifier swap in `clientAuthFor` false branch) | T1 | YES (dossier §5.1) | flipped `TestCVC_RequireFalse_NeverYieldsNoClientCert` mapping-value `Errorf` (direct `ClientAuth == stdtls.NoClientCert` compare, :1479-1481 — cannot be masked by err-path) | — |
| **A re-run** | T2 | YES | EVERY per-shape mapping `Errorf` (inline / SDS-VC / CVC tests) — each shape individually confirmed | — |
| **C** — require=true CVC skips the pool (`if !require { return installPool(pool) }; return nil`) | T1 | YES (dossier §5.3) | phase-66 CVC require=true unit tests (`RequireAndVerifyClientCert` + non-nil `ClientCAs` — the `Errorf`s at config_test.go:1687/:1690, :1712) AND, at the e2e level, **ONLY the refuse subtest** of `TestSDSEndToEnd_CVC_PoolSubstitution` (boot_sds_e2e_test.go:511 `t.Fatal` "…SDS pool was MERGED…"). **The accept subtest PASSES under this break (EXECUTED, MOD-1: NoClientCert-shaped server sends no CertificateRequest — handshake+echo succeed) and is NOT in the firing set.** Hoisted arm proven the live require=true path | — |
| **D** — delete the nil-pool guard at the install site | T2 | YES (dossier §5.4) | `TestVerifyIfGiven_NilPool_Unconstructible` `(nil,nil)` fakeProvider arm — the forbidden `IfGiven && ClientCAs==nil` state assertion (this break IS the test's liveness proof) | — |
| corrupt-CA **two-edit** break (Break D's deletion + `if err != nil && require` — arity per the landed mechanism: closure shape returns `return err`, MOD-3) | T2 | NO — substitute+report if needed | `TestInlineCorruptTrustedCA_RequireFalse_BootError` fires on `err == nil` AND test 6 fires (over-determination demonstrated: both layers must fail together) | — |
| **B** — untrusted arm polite-mode swap (`Certificates:` for `GetClientCertificate`) | T5 | YES (dossier §5.2, verified on the live boot_sds_e2e analogue) | 0110 untrusted-arm `rejected` verdict / `structuralCheck` — the vacuous-green (withholding) state CAUGHT. Control: 0109's standing polite green = the require=true non-firing control | — |
| structuralCheck step (a) — symmetric served-CA break, check DISABLED | T5 | — (0109-protocol adaptation) | **expected: fixture ships PASS** (`reference_vacuous_break_receiver_normalizes` re-demonstrated) — record the PASS | — |
| structuralCheck step (b) — same break, check ENABLED | T5 | — | `structuralCheck` fires, naming side + arms | — |
| structuralCheck step (c) — ASYMMETRIC (subject-only), check disabled | T5 | — | `CompareBytes` mismatch — record the byte offset | — |
| per-side failure-pin one-char perturbation | T5 | NO | that side's pin assertion ALONE | — |
| served-this-arm precondition break | T5 | NO | the precondition assert BEFORE any verdict compare | — |

## Six-gate checklist (T8; controller-run on the frozen HEAD — evidence verbatim)

- [x] `gofmt -l internal/ test/ cmd/` → SILENT
- [x] `go vet ./...` → exit 0
- [x] `go build ./...` → exit 0
- [x] `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` NO DIFF
- [x] `golangci-lint run ./...` → exit 0
- [x] full differential `go test ./test/differential/ -count=1` — **112** dirs, exit 0 (startup-flake / 0061-flake discrimination rules per PLAN T8)
- [ ] cycle guard `go list -deps ./internal/xds | grep 'envoy-go/internal'` (no `...`) → `internal/stats` + `internal/xds` ONLY
- [x] `-race`: `go test ./internal/tls/ ./internal/boot/ -race -count=1` (init_fetch_timeout flake = pre-existing on FIRST occurrence)
- [x] counts MECHANICAL: fixtures **112** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0289** · stat surface **1201**
- [x] envelope audit: `internal/xds` comment-only · `internal/boot` test-comment-only (B19) · `boot.go`/`listener`/`validate/`/`sdsserver` ABSENT from `git diff master --stat`
- [x] retained-reject roster byte-diff re-confirmed on the final tree; retired E3 substring gone from code
- [x] stays-green roster (PLAN A3): manager_test tripwire + the three SDS e2e tripwires — green on the final tree

## Stage-close checklist (T9)

- [x] ADR-0289 §Decision/§Consequences appended IN PLACE (no new ADR; tail ADR-0289, next-free ADR-0290)
- [x] B12 bracketed annotation at DECISIONS:16899 (verbatim SPEC §9 B12)
- [x] full-repo serve-anyway drift closure (MOVED from T6 — MOD-2): both greps re-run AFTER B2/B12/pointer-edit land; zero LIVING drifted sites; every hit classified per PLAN T9's expected-hits table (a hit fitting no row is a FINDING)
- [x] ROADMAP row 67 → `done`; deferred sentence UNTOUCHED (no fabricated narrow)
- [x] STATE.md pointer edited in place; lineage capped at five
- [x] router roll (`next-prompt.txt` — tracked; by SUBJECT)
- [x] sentinel re-run mechanically: (1) silent post-flip, (2) prints 3 via full-phrase command, (3) unchanged — does NOT fire; no `stop`
- [x] memory updates: `reference_go_client_cert_withholding` extension · serve-anyway drift lesson · the B19 parallel-stream-mints-fresh-drift lesson (PLAN T9)
- [x] controller squash-push


## T8/T9 completion evidence (controller-run on the frozen HEAD, 2026-07-18)

- **Six-gate ALL GREEN:** gofmt -l silent · go vet ./... exit 0 · go build ./... exit 0 · go mod tidy -diff EMPTY · git diff master -- go.mod go.sum EMPTY · golangci-lint run ./... exit 0.
- **Full differential:** `ok  github.com/pgdad/envoy-go/test/differential  375.345s` — EXIT 0, **0 FAIL**, all **112** dirs (0110 registered + green; 111 pre-existing byte-stable). No startup flakes.
- **Cycle guard:** `go list -deps ./internal/xds | grep envoy-go/internal` → `internal/stats` + `internal/xds` ONLY.
- **-race:** `go test ./internal/tls/ ./internal/boot/ -race -count=1` → ok (tls 1.101s / boot 1.281s); no init_fetch_timeout flake.
- **Counts:** fixtures **112** (tail 0110) · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0289** · stat surface **1201** (docs) · go.mod modules **2**.
- **Envelope audit:** `git diff master --stat` production `.go` = `internal/tls/config.go` (functional) + `internal/xds/provider.go` (chartered comment-only B16) ONLY; `boot.go`/`internal/listener`/`validate/`/`sdsserver` ABSENT.
- **Retained-reject byte-diff:** all five substrings = 1 in config.go; the retired E3 substring = 0 in all `internal/` `.go`.
- **Break ACTUALs:** full verbatim firings recorded in the per-task reports (scratchpad task-{1,2,4}-report.md) — T1 Break A (mapping-value Errorf) + Break C (require=true CVC units + ONLY the refuse e2e subtest); T2 Break A (all 3 shapes) + Break D (test6 (nil,nil) arm) + corrupt-CA two-edit (test5 on err==nil); T4/T5 Break B (untrusted-arm structuralCheck on the polite-withhold vacuous green, 0109 the control) + the structuralCheck triplet (2a PASS / 2b fires / 2c CompareBytes offset 8) + per-side pin + served-this-arm. Every break re-greened.
- **Drift closure (T9 gate):** both greps re-run after B12 + the STATE pointer edit; every hit classifies per the expected-hits table (counterfactuals boot_sds_e2e:43/519/545 + BC:914 · corrected-in-place BC:900/config.go/provider.go/config_test.go:999/TEST_GAP · historical-with-correction DECISIONS:16899 · ADR-0289 §Context DECISIONS:17026 · STATE pointer/lineage). Zero LIVING drift.
- **Task/branch commits:** c0144adc (T1) · 50bb10db (T2) · 86967b0b (T3) · d6bee060 (T4/T5) · 3a231c1d (T6) · 6e8f7f81 + 35f13315 (T7) · aba88dfc (T9 ADR) · + the stage-close docs squash. Controller squash-pushes at close.
