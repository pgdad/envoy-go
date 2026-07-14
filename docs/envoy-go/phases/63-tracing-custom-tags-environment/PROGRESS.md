# Phase 63 PROGRESS — tracing `custom_tags` `environment` SOURCE arm (ADR-0284; row 63 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-14, worktree `.worktrees/phase-63-plan`, branch `phase-63-tracing-environment-custom-tag-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 9.

## Task checklist (mirrors PLAN.md — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [x] Task 1 — `config.go` (`kindEnvironment` const + `EnvName` field) + `resolve.go` (`import "os"` + a `case kindEnvironment:`: `os.LookupEnv`, omit-iff-resolved-empty, ignores `headerLookup`) + `resolve_test.go` matrix (present / absent+default / absent+no-default→omit / present-empty→omit via `t.Setenv`); PURELY ADDITIVE (build green — parse still rejects environment); `-count=1` breaks on present-uses-env-value + present-empty-omits (Getenv-vs-LookupEnv, arm G) + omit-on-empty [TDD]
- [x] Task 2 — `config.go`: lift the `environment` reject in `parseCustomTags` (`:195-196`) → ACCEPT arm + empty-name PGV-parity reject; append a `kindEnvironment` spec through the EXISTING first-wins-dedup path (reserves the slot — arm F); dedup block UNCHANGED; `config_test.go` — REMOVE the `environment` reject row → `environment-empty-name` reject; `metadata` row STAYS; ADD `TestNewConfigAcceptCustomTagEnvironment` + `TestNewConfigCustomTagEnvironmentFirstWinsDedup`; ZERO call-site / field-type change; `-count=1` breaks on accept + empty-name substring + dedup-slot-reservation [TDD]
- [x] Task 3 — `span_test.go`: CONFIRM `BuildServerSpan`/`upsertAttr` UNCHANGED + a resolved-environment-KV-upserts-over-a-built-in test (arm B); `-count=1` break on always-append → count 2 [TDD]
- [x] Task 4 — Zipkin encoder unit test (a resolved environment tag in the `tags` map; node_id/zone dropped); `-count=1` break on adding `region` to the drop guard [TDD]
- [x] Task 5 — New OTLP fixture `0106-tracing-custom-tags-environment` (clone `0105`; environment tag `env_path` read from `PATH`; driver drives a PLAIN GET — no header; key-presence + value-non-empty cross-side; register in `runner_test.go`; `-count=1` differential break — break the assert's expected KEY only, `reference_vacuous_break_receiver_normalizes`; FULL `-run` selector, `reference_differential_run_selector`)
- [x] Task 6 — `FuzzHCMConfigParse` environment custom_tags seed (fuzzers 55 → 55; reconcile `^func Fuzz` before + after, `reference_fuzzer_count_docs_drift`)
- [x] Task 7 — `BEHAVIOR_CONTRACT.md` edits (RE-DERIVE the drifted lines; `environment` → CONSUMED; add empty-name reject; narrow the departure/deferred bullet to `metadata`)
- [x] Task 8 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` + `git diff go.mod` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 108-dir differential (byte-stable except `0106`) — controller-run on frozen HEAD, no-commit gate; evidence below
- [x] Task 9 — ADR-0284 body (§Decision/§Consequences) + STATE + ROADMAP (row 63 `done` + deferred-sentence narrow to `custom_tags (metadata)` + check-(2) re-run) + PROGRESS close + router roll (docs-only; squash/push are the controller's stage-close)

**PHASE 63 CLOSED AT IMPL (2026-07-14).** Row 63 flips `in-progress` → `done` at this IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0284 (§Decision/§Consequences landed in `DECISIONS.md`; §Context drafted at the SPEC, SPEC §13).

## D-ENV-* question dispositions (ALL DISPOSED at the SPEC — SPEC-63 §1/§11, via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2`, fresh container per arm)

- **D-ENV-SCOPE → environment added**; `metadata` rejects loudly (the SOLE remaining `custom_tags` departure) (§2, §6).
- **D-ENV-WIRE → PINNED** (§11 arm A): `{key:<tag>, value:{stringValue:<env value>}}` — a STRING attribute upserted against the built-ins (identical wire shape to `literal`/`request_header`).
- **D-ENV-MISSING → PINNED** (§11 arm A): env present→env value / env absent+default→default / env absent+no-default→**OMIT**.
- **D-ENV-EMPTYVAL → PINNED** (§11 arm G, NEW/unanticipated): env PRESENT-but-EMPTY → **OMIT** (not `""`, not the default) ⇒ unified rule **omit iff the resolved value is empty**, requiring `os.LookupEnv` (present-ness); DIVERGES from phase-62's request_header present-empty edge (emits `""`).
- **D-ENV-PRECEDENCE → PINNED** (§11 arms B/C/D), IDENTICAL to phase-62: custom OVERRIDES a colliding built-in (arm B, upsert); among CUSTOM tags with the SAME key the **FIRST in config order WINS** (arms C/D).
- **D-ENV-DEDUP → PINNED** (§11 arm F, load-bearing): an `environment` tag that RESOLVES TO NOTHING but is FIRST in config order STILL reserves its config-order key slot (the reference dedups by key at config-LOAD, before value resolution).
- **D-ENV-RESOLVE-TIME → DECIDED = Option B** (§1.1, §3.3): a `kindEnvironment` spec stored at parse + resolved per-span in `ResolveCustomTags` via `os.LookupEnv`; arm F + arm G FLIP it from the BRAINSTORM's anticipated Option A (ADR-0044 empirical refinement). Storing the spec reserves the dedup slot NATURALLY through the existing append path; Option A would need a special-case slot-reservation wart.
- **D-ENV-REJECT → PINNED** (§6, §11 arm E): the `metadata` substring unchanged; empty `environment.name` → a NEW PGV-parity reject (`custom_tag.pb.validate.go:468`, `min_len:1`, no pattern constraint); the reference ACCEPTS `environment` ⇒ the departure genuinely narrows to `metadata` alone.
- **D-ENV-FIXTURE → DECIDED** (§8): ONE new OTLP fixture `0106` (fixtures 107 → 108), KEY-PRESENCE via `PATH` (ZERO harness surgery, SINGLE FLAT ROW); value-equality + D-ENV-HARNESS injection DEFERRED; the value-resolution edges + the Zipkin path are UNIT tests.
- **D-ENV-HARNESS → DEFERRED** (§8): the value-equality env-injection surgery (testcontainers `ContainerRequest.Env` + `cmd.Env`, threaded per-fixture) is NOT one-line-per-starter; deferred in favor of key-presence.
- **D-ENV-FUZZSEED → DECIDED** (§6): a SEED to the existing `FuzzHCMConfigParse`; fuzzer count STAYS 55.
- **D-ENV-SPLIT → DECIDED** (§3.0): a SINGLE FLAT ROW (~8–10 anticipated; this PLAN lands 9 tasks); ADR-0045 escape-valve UNCONSUMED.
- **D-ENV-DOCSHAPE → RE-DERIVED** (§12); re-verified against master tip `33d5ed4c` at this PLAN (all `file:line` hold; `kindEnvironment`/`EnvName`/`os` re-grep-confirmed collision-free).

## Baselines (filled at IMPL Task 1 — verbatim, against the phase-63 PLAN squash `68d5cc32` = master tip)

- `go build ./...`: clean, no output
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): 107 (tail `0105-tracing-custom-tags-request-header`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): 55
- BackendKind tail: 38 (`H2GoawayResponder`)
- `go mod tidy -diff`: empty (`CustomTag_Environment` getter + stdlib `os` already-resolved; no NEW sub-package)
- stat surface: **1201** (docs-verified; registration guards enforce +0 — no counting command)
- DECISIONS tail: `## ADR-0283` (body lands at T9 as ADR-0284, next-free ADR-0285)
- Anchors CONFIRMED vs PLAN roster: `config.go` env reject `:195-196` / `metadata` reject / dedup block; `CustomTagSpec` + iota; `resolve.go` switch (NO import block pre-T1); `accesslog_emit.go` call sites `:55`/`:116`/`:177` (UNCHANGED); `span.go` `BuildServerSpan`/`upsertAttr`; `zipkin.go` drop guard; `fuzz_test.go`; `runner_test.go` `0105` register. `kindEnvironment`/`EnvName`/`os` re-grep-confirmed collision-free.

## Liveness-break log (every break `-count=1`, confirmed WHICH fired, then restored byte-identical)

- **T1 (resolver, `resolve_test.go`):** three isolated breaks, each fired ONLY its own subtest. (a) present-uses-env-value: breaking the `os.LookupEnv` read fired the present subtest. (b) present-empty-omits (the D-ENV-EMPTYVAL / arm-G proof): swapping `os.LookupEnv` for `os.Getenv` (which cannot distinguish present-empty from absent) fired the present-empty→omit subtest, confirming the env arm needs present-ness AND the omit-iff-resolved-empty rule (a present-but-empty var must OMIT, not emit `""` — DIVERGES from the request_header present-empty edge). (c) omit-on-empty: removing the `v != ""` emit-guard so an empty resolved value emitted a `KV` fired the omit subtests. Additive invariant held (parse still rejects environment, so nothing constructs a `kindEnvironment` spec in production).
- **T2 (parse/config, `config_test.go`):** three isolated breaks. (a) accept: reverting the new `GetEnvironment() != nil` arm to the phase-59/62 `environment type unsupported` reject fired the accept-shape subtest. (b) empty-name substring: removing the `e.GetName() == ""` check fired the `environment-empty-name` reject subtest (the NEW PGV-parity reject, not vacuously subsumed by the empty-`tag` check). (c) dedup-slot-reservation (arm F): removing the `seen[tag]` drop-guard so a colliding-key second tag appended fired the env-dedup subtest, confirming an env tag placed FIRST reserves its config-order slot NATURALLY through the existing append path even when it resolves to nothing. (One mid-break git-restore mishap during break #1 self-corrected; the controller CONFIRMED the final committed diff is correct.)
- **T3 (span arm B, `span_test.go`):** reverting `upsertAttr` to an unconditional append made the built-in-collision subtest see `count=2` where `want=1`; the arm-B test asserts count==1 AND the value came via the env method — confirming `BuildServerSpan`/`upsertAttr` are genuinely UNCHANGED (`span.go` byte-identical to master) and the resolved environment KV correctly overrides a colliding built-in.
- **T4 (Zipkin, `zipkin_test.go`):** adding `region` to the Zipkin encoder's built-in drop set fired the assertion on `tags["region"]` (expected `us-east-2`, found absent), confirming the resolved environment tag reaches the Zipkin `tags` map via the same shared `Attrs`/`ResolveCustomTags` path; `node_id`/`zone` still dropped. `zipkin.go` byte-identical to master.
- **T5 (`0106` differential):** per `reference_vacuous_break_receiver_normalizes`, the break touched the assert's expected KEY only (`env_path` → a wrong key) — this fired the missing-key assertion on BOTH the subject and the reference side (11 reference + 12 subject spans), confirming the `0106` fixture's key-presence assertion is live end-to-end and not masked by an earlier `Fatalf`. The resolved `PATH` value DIFFERS per side (reference container vs subject subprocess), so the assertion is key-presence + value-non-empty, NOT value-equality; `Errorf`-per-property. Run via the full `-run 'TestDifferential/0106-tracing-custom-tags-environment'` selector with `-count=1`; no other fixture dir mutated.

## Task-8 verify evidence (filled at IMPL — verbatim, controller-run on HEAD `a28f0178`)

- six-gate: ALL GREEN — `gofmt -l internal/ test/ cmd/` no output; `go vet ./...` clean; `go build ./...` clean; `go mod tidy -diff` EMPTY + `git diff go.mod`/`go.sum` empty; `golangci-lint run ./...` exit 0; `go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...` all ok (`internal/tracing` 1.079s, `internal/filter/hcm` 1.067s, `internal/filter/hcm/h2` 6.544s).
- 108-dir differential (`go test ./test/differential/ -count=1`): `ok github.com/pgdad/envoy-go/test/differential 360.265s`, EXIT_CODE=0, byte-stable except the new `0106` (107 pre-existing dirs byte-stable).

**Landed task commits:** T1 `092e39c3` · T2 `8993bef9` · T3 `48ee2ef4` · T4 `54d539b0` · T5 `02f52aad` · T6 `5f1c1a54` · T7 `a28f0178` (T8 verify was a no-commit gate run; T9 is this docs commit).

**Exit counts (confirmed against the landed tree):** stat surface **1201** (+0) · fixtures **107 → 108** (`0106-tracing-custom-tags-environment`) · fuzzers **55** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0283 → ADR-0284** (next-free **ADR-0285**). Row 63 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).
