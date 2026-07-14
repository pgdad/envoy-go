# Phase 63 PROGRESS — tracing `custom_tags` `environment` SOURCE arm (ADR-0284; row 63 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-14, worktree `.worktrees/phase-63-plan`, branch `phase-63-tracing-environment-custom-tag-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 9.

## Task checklist (mirrors PLAN.md — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [ ] Task 1 — `config.go` (`kindEnvironment` const + `EnvName` field) + `resolve.go` (`import "os"` + a `case kindEnvironment:`: `os.LookupEnv`, omit-iff-resolved-empty, ignores `headerLookup`) + `resolve_test.go` matrix (present / absent+default / absent+no-default→omit / present-empty→omit via `t.Setenv`); PURELY ADDITIVE (build green — parse still rejects environment); `-count=1` breaks on present-uses-env-value + present-empty-omits (Getenv-vs-LookupEnv, arm G) + omit-on-empty [TDD]
- [ ] Task 2 — `config.go`: lift the `environment` reject in `parseCustomTags` (`:195-196`) → ACCEPT arm + empty-name PGV-parity reject; append a `kindEnvironment` spec through the EXISTING first-wins-dedup path (reserves the slot — arm F); dedup block UNCHANGED; `config_test.go` — REMOVE the `environment` reject row → `environment-empty-name` reject; `metadata` row STAYS; ADD `TestNewConfigAcceptCustomTagEnvironment` + `TestNewConfigCustomTagEnvironmentFirstWinsDedup`; ZERO call-site / field-type change; `-count=1` breaks on accept + empty-name substring + dedup-slot-reservation [TDD]
- [ ] Task 3 — `span_test.go`: CONFIRM `BuildServerSpan`/`upsertAttr` UNCHANGED + a resolved-environment-KV-upserts-over-a-built-in test (arm B); `-count=1` break on always-append → count 2 [TDD]
- [ ] Task 4 — Zipkin encoder unit test (a resolved environment tag in the `tags` map; node_id/zone dropped); `-count=1` break on adding `region` to the drop guard [TDD]
- [ ] Task 5 — New OTLP fixture `0106-tracing-custom-tags-environment` (clone `0105`; environment tag `env_path` read from `PATH`; driver drives a PLAIN GET — no header; key-presence + value-non-empty cross-side; register in `runner_test.go`; `-count=1` differential break — break the assert's expected KEY only, `reference_vacuous_break_receiver_normalizes`; FULL `-run` selector, `reference_differential_run_selector`)
- [ ] Task 6 — `FuzzHCMConfigParse` environment custom_tags seed (fuzzers 55 → 55; reconcile `^func Fuzz` before + after, `reference_fuzzer_count_docs_drift`)
- [ ] Task 7 — `BEHAVIOR_CONTRACT.md` edits (RE-DERIVE the drifted lines; `environment` → CONSUMED; add empty-name reject; narrow the departure/deferred bullet to `metadata`)
- [ ] Task 8 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` + `git diff go.mod` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 108-dir differential (byte-stable except `0106`) — controller-run on frozen HEAD, no-commit gate; evidence below
- [ ] Task 9 — ADR-0284 body (§Decision/§Consequences) + STATE + ROADMAP (row 63 `done` + deferred-sentence narrow to `custom_tags (metadata)` + check-(2) re-run) + PROGRESS close + router roll (docs-only; squash/push are the controller's stage-close)

**PHASE 63 CLOSES AT IMPL.** Row 63 flips `in-progress` → `done` at the IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0284 (§Decision/§Consequences land in `DECISIONS.md` at the IMPL per ADR-0044; §Context drafted at the SPEC, SPEC §13).

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

## Baselines (fill at IMPL Task 1 — verbatim command outputs, against master tip `33d5ed4c` / the phase-63 PLAN commit)

- `go build ./...`: _(fill)_
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): _(fill — expect 107, tail `0105-tracing-custom-tags-request-header`)_
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): _(fill — expect 55)_
- BackendKind tail: _(fill — expect 38, `H2GoawayResponder`)_
- `go mod tidy -diff`: _(fill — expect empty; `CustomTag_Environment` getter + stdlib `os` are already-resolved; no NEW sub-package)_

## Liveness-break log (fill per task at IMPL — the assertion that fired, confirming WHICH)

_(fill at IMPL)_

## Task-8 verify evidence (fill at IMPL — six-gate + 108-dir differential)

_(fill at IMPL)_
