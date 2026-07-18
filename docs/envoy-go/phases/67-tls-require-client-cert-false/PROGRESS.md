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
| T1 | ATOMIC core: hoist + three-way `ClientAuth` (assignment-adjacent) + E3 RETIRED + flip roster items 1–4 (red-first inversions) + theorem block moved intact + retained rejects byte-diffed + stays-green roster | pending | — |
| T2 | Mapping cross-product unit tests (3 shapes × {false, absent}, anchorless, corrupt-CA, the require=false fetch-failure vcErr arm — amendment M9) + `TestVerifyIfGiven_NilPool_Unconstructible` (interface-pinned) | pending | — |
| T3 | Fuzz seeds on correct dispatch sides (+0 fuzzers; count reconciled 55 → 55) | pending | — |
| T4 | Fixture `0110-tls-require-client-cert-false` — CVC-primary, three arms, FORCED-SEND untrusted arm (fixtures 111 → 112) | pending | — |
| T5 | 0110 break protocol: Break B + structuralCheck triplet + per-side pin + served-this-arm | pending | — |
| T6 | Comment sweeps: B11/B18 (config.go) + B16 (provider.go, chartered) + B17 (:999) + **B19** (boot_sds_e2e, chartered — PLAN A2) + the 0109 enumerated set + grep discharged (three-category dispositions, M10) + non-drift dispositions (A5 + BC:914); T6-SCOPED `.go` grep only — the full-repo drift closure moved to T9 (MOD-2) | pending | — |
| T7 | BC delta B1–B10/B13–B14 (pinned verbatim) + FOUR-site TEST_GAP sweep (B15) by claim+grep (A1); B11@T6, B12@T9 (M8) | pending | — |
| T8 | VERIFY: six-gate + cycle guard + full 112-dir differential + `-race` + mechanical counts + envelope audit | pending | — |
| T9 | ADR-0289 completed IN PLACE + B12 annotation + ROADMAP/STATE/PROGRESS/router + sentinel + squash-push | pending | — |

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

- [ ] `gofmt -l internal/ test/ cmd/` → SILENT
- [ ] `go vet ./...` → exit 0
- [ ] `go build ./...` → exit 0
- [ ] `go mod tidy -diff` EMPTY + `git diff --exit-code master -- go.mod go.sum` NO DIFF
- [ ] `golangci-lint run ./...` → exit 0
- [ ] full differential `go test ./test/differential/ -count=1` — **112** dirs, exit 0 (startup-flake / 0061-flake discrimination rules per PLAN T8)
- [ ] cycle guard `go list -deps ./internal/xds | grep 'envoy-go/internal'` (no `...`) → `internal/stats` + `internal/xds` ONLY
- [ ] `-race`: `go test ./internal/tls/ ./internal/boot/ -race -count=1` (init_fetch_timeout flake = pre-existing on FIRST occurrence)
- [ ] counts MECHANICAL: fixtures **112** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0289** · stat surface **1201**
- [ ] envelope audit: `internal/xds` comment-only · `internal/boot` test-comment-only (B19) · `boot.go`/`listener`/`validate/`/`sdsserver` ABSENT from `git diff master --stat`
- [ ] retained-reject roster byte-diff re-confirmed on the final tree; retired E3 substring gone from code
- [ ] stays-green roster (PLAN A3): manager_test tripwire + the three SDS e2e tripwires — green on the final tree

## Stage-close checklist (T9)

- [ ] ADR-0289 §Decision/§Consequences appended IN PLACE (no new ADR; tail ADR-0289, next-free ADR-0290)
- [ ] B12 bracketed annotation at DECISIONS:16899 (verbatim SPEC §9 B12)
- [ ] full-repo serve-anyway drift closure (MOVED from T6 — MOD-2): both greps re-run AFTER B2/B12/pointer-edit land; zero LIVING drifted sites; every hit classified per PLAN T9's expected-hits table (a hit fitting no row is a FINDING)
- [ ] ROADMAP row 67 → `done`; deferred sentence UNTOUCHED (no fabricated narrow)
- [ ] STATE.md pointer edited in place; lineage capped at five
- [ ] router roll (`next-prompt.txt` — tracked; by SUBJECT)
- [ ] sentinel re-run mechanically: (1) silent post-flip, (2) prints 3 via full-phrase command, (3) unchanged — does NOT fire; no `stop`
- [ ] memory updates: `reference_go_client_cert_withholding` extension · serve-anyway drift lesson · the B19 parallel-stream-mints-fresh-drift lesson (PLAN T9)
- [ ] controller squash-push
