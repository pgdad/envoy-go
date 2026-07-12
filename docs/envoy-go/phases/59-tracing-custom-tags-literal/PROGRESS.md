# Phase 59 PROGRESS — tracing `custom_tags` (LITERAL tag type only) (ADR-0277; row 59 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-12, worktree `.worktrees/phase-59-plan`, branch `phase-59-tracing-custom-tags-literal-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 8.

## Task checklist (mirrors PLAN.md)

- [ ] Task 1 — `config.go`: `tracingv3` import + `CustomTags []KV` field + `parseCustomTags` (6 arms) + provider-switch restructure; accept-literal test (OTel + Zipkin) + 6 reject sub-tests (distinct substrings); `-count=1` liveness break per reject arm + the accept [TDD]
- [ ] Task 2 — `span.go`: `customTags []KV` param on `BuildServerSpan` + `upsertAttr` + upsert loop; thread `f.tracingConfig.CustomTags` at both `accesslog_emit.go` call sites (`:55`/`:116`); update all 7 test call sites (span_test ×6, zipkin_test ×1); append + upsert-override tests; `-count=1` break on the override [TDD; folds SPEC task 4]
- [ ] Task 3 — Zipkin encoder unit test (literal tag in `tags` map; node_id/zone dropped); `-count=1` break
- [ ] Task 4 — New OTLP fixture `0102-tracing-custom-tags-literal` (clone `0087`); NON-colliding literal tag; `Errorf`-per-property cross-side-by-key assertion; register + `-count=1` differential break (confirm WHICH fires)
- [ ] Task 5 — `FuzzHCMConfigParse` custom_tags seed (fuzzers 54 → 54; reconcile `^func Fuzz` before + after)
- [ ] Task 6 — `BEHAVIOR_CONTRACT.md` edits (`:686` strict-reject roster + `:739` Zipkin deferred bullet)
- [ ] Task 7 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 104-dir differential (byte-stable except `0102`)
- [ ] Task 8 — ADR-0277 body (§Decision/§Consequences) + STATE (`:7`) + ROADMAP (row 59 `done` + deferred-sentence narrow + check-(2) re-run) + PROGRESS close + router roll

## D-question dispositions (ALL TEN DISPOSED at the SPEC — SPEC-59 §1)

- **D-CT-SCOPE → literal-only**; the other three types reject loudly (§2.2, §6).
- **D-CT-LITERAL-WIRE → PINNED** (§11 arm literal-otlp): `{key:<tag>, value:{stringValue:<literal.value>}}`, appended after the built-ins.
- **D-CT-ZIPKIN-WIRE → PINNED** (§11 arm zipkin): the tag appears in the Zipkin `tags` map as `"<tag>":"<value>"`.
- **D-CT-PRECEDENCE → PINNED, CONTRADICTS the BRAINSTORM** (§11 arm precedence-otlp): the reference OVERRIDES a colliding built-in (last-write-wins), NOT append ⇒ `BuildServerSpan` UPSERTS-by-key (§3.3).
- **D-CT-CONFIG-SEAM → DECIDED** (§3): `CustomTags []KV` on `TracingConfig`, parsed provider-neutrally, threaded as a `customTags []KV` param to `BuildServerSpan`.
- **D-CT-REJECT → PINNED** (§6): three type-DEPARTURE + three PGV-parity structural rejects, all six ADR-0080-distinct.
- **D-CT-FIXTURE → DECIDED** (§8): ONE new OTLP fixture `0102` (fixtures 103 → 104); the Zipkin encoder path is a unit test (escape-valve unconsumed).
- **D-CT-FUZZSEED → DECIDED** (§6): a SEED to the existing `FuzzHCMConfigParse`; fuzzer count STAYS 54.
- **D-CT-SPLIT → DECIDED** (§3.0): a SINGLE FLAT ROW; ADR-0045 escape-valve UNCONSUMED.
- **D-CT-DOCSHAPE → RE-DERIVED** (§12); re-verified against master tip `36f012cf` at this PLAN.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: _TBD_
- fixtures (`ls -d test/fixtures/0*/ | wc -l`): _TBD_ (expect 103, tail `0101-stats-sink-graphite`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): _TBD_ (expect 54)
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): _TBD_ (expect BackendKind 38)
- `go mod tidy -diff`: _TBD_ (expect empty — `tracingv3` is an already-resolved module)
- custom_tags reject pre-check (`grep -n 'custom_tags unsupported' internal/tracing/config.go`): _TBD_ (expect `:82-83`, the wholesale reject about to be replaced)
- stat surface: **1201** (docs-verified; registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): _TBD_ (expect `## ADR-0276`, next-free ADR-0277)

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 8 tasks, margin ~7 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem to strand: the parse + 6 reject arms live in one `NewConfig` helper; the span-emit append is one function; both exporters share the `Attrs` seam; the single new OTLP fixture does not spill into its own row.

## Liveness-break log (fill at IMPL — every break `-count=1`, confirm WHICH fired)

- T1 (6 reject arms + accept): _TBD_
- T2 (upsert override — always-append → count 2, want 1): _TBD_
- T3 (Zipkin — add custom_env to the drop set): _TBD_
- T4 (0102 — customTagValue = "WRONG", both sides fire): _TBD_

## Anticipated exit counts (from SPEC-59 §14)

stat surface **1201 (+0)** · fixtures **103 → 104** (`0102-tracing-custom-tags-literal`) · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0277** (next-free ADR-0278) · **+0 go.mod modules, +0 packages**.

## Verify evidence (filled at Task 7 — verbatim)

- six-gate: _TBD_
- 104-dir differential (`go test ./test/differential/ -count=1`): _TBD_ (expect `ok`, exit 0, byte-stable except `0102`)
