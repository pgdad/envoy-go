# Phase 62 PROGRESS — tracing `custom_tags` `request_header` SOURCE arm (ADR-0283; row 62 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-14, worktree `.worktrees/phase-62-plan`, branch `phase-62-tracing-request-header-custom-tag-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 9.

## Task checklist (mirrors PLAN.md — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [ ] Task 1 — `resolve.go`: `CustomTagSpec`/`customTagKind` types (in `config.go`) + `ResolveCustomTags` resolver + `resolve_test.go` matrix (present / default / omit / multi→first / nil-lookup / literal / present-empty-not-default); purely additive (build green); `-count=1` breaks on first-value + omit-on-missing + present-empty [TDD]
- [ ] Task 2 — `config.go`: reshape `parseCustomTags` → `([]CustomTagSpec, error)` (request_header accept + empty-name reject + first-wins dedup) + `TracingConfig.CustomTags []CustomTagSpec` field type + thread the THREE `accesslog_emit.go` call sites (`:55`/`:116`/`:177`) via `tracing.ResolveCustomTags(..., reqHeaderLookupH1/H2(...))`; `config_test.go` accept→spec shape + request_header accept + first-wins dedup + empty-name reject (DROP the stale `request_header`-reject row); ATOMIC build-green change; `-count=1` breaks on accept + empty-name substring + dedup [TDD]
- [ ] Task 3 — `span_test.go`: CONFIRM `BuildServerSpan`/`upsertAttr` UNCHANGED + a resolved-request_header-KV-upserts-over-a-built-in test (arm B); `-count=1` break on always-append → count 2 [TDD]
- [ ] Task 4 — Zipkin encoder unit test (a resolved request_header tag in the `tags` map; node_id/zone dropped); `-count=1` break [TDD]
- [ ] Task 5 — New OTLP fixture `0105-tracing-custom-tags-request-header` (clone `0102`; request_header tag `trace_user` from `x-trace-user`; driver SENDS the header on every request; present-case cross-side by key; register in `runner_test.go`; `-count=1` differential break — break the EXPECTATION only, `reference_vacuous_break_receiver_normalizes`; FULL `-run` selector, `reference_differential_run_selector`)
- [ ] Task 6 — `FuzzHCMConfigParse` request_header custom_tags seed (fuzzers 55 → 55; reconcile `^func Fuzz` before + after, `reference_fuzzer_count_docs_drift`)
- [ ] Task 7 — `BEHAVIOR_CONTRACT.md` edits (RE-DERIVE the drifted lines; `request_header` → CONSUMED; add empty-name reject; narrow the deferred bullet)
- [ ] Task 8 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` + `git diff go.mod` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 107-dir differential (byte-stable except `0105`)
- [ ] Task 9 — ADR-0283 body (§Decision/§Consequences) + STATE + ROADMAP (row 62 `done` + deferred-sentence narrow + check-(2) re-run) + PROGRESS close + router roll (docs-only; squash/push are the controller's stage-close)

**PHASE 62 CLOSES AT IMPL.** Row 62 flips `in-progress` → `done` at the IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0283 (§Decision/§Consequences land at the IMPL per ADR-0044).

## D-RH-* question dispositions (ALL ELEVEN DISPOSED at the SPEC — SPEC-62 §1, via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2`, fresh container per arm)

- **D-RH-SCOPE → request_header added**; `environment`/`metadata` reject loudly (§2, §6).
- **D-RH-WIRE → PINNED** (§11 arm A `t_present`): `{key:<tag>, value:{stringValue:<header value>}}` — a STRING attribute upserted against the built-ins.
- **D-RH-MISSING → PINNED** (§11 arm A): present→value / absent+default→default / absent+empty-default→**OMIT** (a NEW semantic vs `literal`).
- **D-RH-MULTIVALUE → PINNED** (§11 arm A `t_multi`): a header sent twice yields the **FIRST** value; envoy-go uses `lookup(name)[0]`.
- **D-RH-PRECEDENCE → PINNED, REFINES the landed engine** (§11 arms B/C/D): custom OVERRIDES a colliding built-in (arm B, upsert); among CUSTOM tags with the SAME key the **FIRST in config order WINS** (arms C/D) — CONTRADICTS the landed `upsertAttr` last-wins ⇒ FIRST-wins dedup at PARSE (§1.1, §3.2).
- **D-RH-CONFIGMODEL → DECIDED** (§3.2): `TracingConfig.CustomTags []KV` → ORDERED `[]CustomTagSpec`, first-wins-deduped by key at parse.
- **D-RH-RESOLVE-SEAM → DECIDED** (§3.3): `ResolveCustomTags(specs, headerLookup) []KV` in `internal/tracing`, threaded at the THREE `accesslog_emit.go` sites; `BuildServerSpan` UNCHANGED.
- **D-RH-REJECT → PINNED** (§6, §11 arm E): `environment`/`metadata` substrings unchanged; empty `request_header.name` → a NEW PGV-parity reject; the reference ACCEPTS `request_header` ⇒ the departure genuinely narrows.
- **D-RH-FIXTURE → DECIDED** (§8): ONE new OTLP fixture `0105` (fixtures 106 → 107); the default/omit/multi/dedup edges + the Zipkin path are UNIT tests.
- **D-RH-FUZZSEED → DECIDED** (§6): a SEED to the existing `FuzzHCMConfigParse`; fuzzer count STAYS 55.
- **D-RH-SPLIT → DECIDED** (§3.0): a SINGLE FLAT ROW (~11–13 anticipated; this PLAN lands 9 tasks); ADR-0045 escape-valve UNCONSUMED.
- **D-RH-DOCSHAPE → RE-DERIVED** (§12); re-verified against master tip `460f761e` at this PLAN.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: _(fill)_
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): _(fill; expect 106, tail `0104-http3-downstream-get`)_
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): _(fill; expect 55)_
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): _(fill; expect 38)_
- `go mod tidy -diff`: _(fill; expect empty — `CustomTag_Header` getter is an already-resolved module; no NEW sub-package)_
- request_header reject pre-check (`grep -n 'request_header type unsupported' internal/tracing/config.go`): _(fill; expect `:156`, lifted by Task 2)_
- stat surface: **1201** (docs-verified; registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): _(fill; expect `## ADR-0282`, next-free ADR-0283)_

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 9 tasks, margin ~6 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem to strand: the resolver + types, the parse reshape + field change, and the three call-site threadings all sit on the SAME tracing engine; the single new OTLP fixture does not spill into its own row.

## Liveness-break log (fill at IMPL — every break `-count=1`, confirm WHICH fired)

- T1 (resolver): _(fill — first-value `vs[0]`→`vs[len-1]`; omit-on-missing drop-guard; present-empty-not-default)_
- T2 (parse/config): _(fill — request_header accept; empty-name substring; first-wins dedup drop-guard)_
- T3 (span arm B): _(fill — always-append → count 2, want 1)_
- T4 (Zipkin): _(fill — add trace_user to the encoder drop set)_
- T5 (0105 differential): _(fill — break the EXPECTATION only; confirm BOTH sides fire on the custom-tag assertion)_

## Anticipated exit counts (from SPEC-62 §14)

stat surface **1201 (+0)** · fixtures **106 → 107** (`0105-tracing-custom-tags-request-header`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0283** (next-free ADR-0284) · **+0 go.mod modules, +0 packages**.

## Verify evidence (filled at Task 8 — verbatim, from the controller's Task-8 run)

- six-gate: _(fill)_
- 107-dir differential (`go test ./test/differential/ -count=1`): _(fill)_

**Landed task commits:** _(fill T1..T7, T9)_

**Exit counts (confirmed against the landed tree):** _(fill)_ — Row 62 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).
