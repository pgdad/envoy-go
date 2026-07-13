# Phase 59 PROGRESS — tracing `custom_tags` (LITERAL tag type only) (ADR-0277; row 59 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-12, worktree `.worktrees/phase-59-plan`, branch `phase-59-tracing-custom-tags-literal-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 8.

## Task checklist (mirrors PLAN.md)

- [x] Task 1 — `config.go`: `tracingv3` import + `CustomTags []KV` field + `parseCustomTags` (6 arms) + provider-switch restructure; accept-literal test (OTel + Zipkin) + 6 reject sub-tests (distinct substrings); `-count=1` liveness break per reject arm + the accept [TDD] — commit `33a21094`
- [x] Task 2 — `span.go`: `customTags []KV` param on `BuildServerSpan` + `upsertAttr` + upsert loop; thread `f.tracingConfig.CustomTags` at both `accesslog_emit.go` call sites (`:55`/`:116`); update all 7 test call sites (span_test ×6, zipkin_test ×1); append + upsert-override tests; `-count=1` break on the override [TDD; folds SPEC task 4] — commit `4f24df7e`
- [x] Task 3 — Zipkin encoder unit test (literal tag in `tags` map; node_id/zone dropped); `-count=1` break — commit `4f5fe06e`
- [x] Task 4 — New OTLP fixture `0102-tracing-custom-tags-literal` (clone `0087`); NON-colliding literal tag; `Errorf`-per-property cross-side-by-key assertion; register + `-count=1` differential break (confirm WHICH fires) — commit `903f633c`
- [x] Task 5 — `FuzzHCMConfigParse` custom_tags seed (fuzzers 54 → 54; reconcile `^func Fuzz` before + after) — commit `5b92708a`
- [x] Task 6 — `BEHAVIOR_CONTRACT.md` edits (`:686` strict-reject roster + `:739` Zipkin deferred bullet) — commit `40bf8d2e`
- [x] Task 7 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 104-dir differential (byte-stable except `0102`) — verify evidence recorded below
- [x] Task 8 — ADR-0277 body (§Decision/§Consequences) + STATE (`:7`) + ROADMAP (row 59 `done` + deferred-sentence narrow + check-(2) re-run) + PROGRESS close (docs-only; router roll + squash/push are the controller's stage-close, out of this task's scope)

**PHASE 59 CLOSED AT IMPL (2026-07-12).** Row 59 flips `in-progress` → `done` at this IMPL six-gate (ADR-0106, the SOLE leg). ANCHORS ADR-0277 (§Decision/§Consequences landed in `DECISIONS.md`).

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

- `go build ./...`: clean, no output
- fixtures (`ls -d test/fixtures/0*/ | wc -l`): 103 (tail `0101-stats-sink-graphite`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): 54
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): 38
- `go mod tidy -diff`: empty (`tracingv3` is an already-resolved module)
- custom_tags reject pre-check (`grep -n 'custom_tags unsupported' internal/tracing/config.go`): `:83` (the wholesale reject, replaced by Task 1)
- stat surface: **1201** (docs-verified; registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): `## ADR-0276` (next-free ADR-0277)

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 8 tasks, margin ~7 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem to strand: the parse + 6 reject arms live in one `NewConfig` helper; the span-emit append is one function; both exporters share the `Attrs` seam; the single new OTLP fixture does not spill into its own row.

## Liveness-break log (fill at IMPL — every break `-count=1`, confirm WHICH fired)

- T1 (6 reject arms + accept): each of the 6 reject arms + the accept case broke in isolation — every reject substring fired on its OWN subtest (no cross-firing), confirming the dispatch-by-getter ordering (empty-tag before the type switch, typeless as the fallthrough) is load-bearing.
- T2 (upsert override — always-append → count 2, want 1): reverting `upsertAttr` to unconditional append made the colliding-key case emit `count=2 want=1` on the built-in-collision subtest, confirming the upsert-by-key loop is load-bearing.
- T3 (Zipkin — add custom_env to the drop set): adding `custom_env` to the Zipkin encoder's built-in drop set fired the assertion on `tags["custom_env"]` (expected present, found absent), confirming the literal tag reaches the Zipkin `tags` map via the shared `Attrs` seam.
- T4 (0102 — customTagValue = "WRONG", both sides fire): setting the fixture driver's expected `customTagValue` to `"WRONG"` fired `assertCustomTag` on ALL 24 span-checks on BOTH the subject and the reference side, confirming the cross-side-by-key assertion is live on the full check set (not vacuous on a subset).

## Anticipated exit counts (from SPEC-59 §14)

stat surface **1201 (+0)** · fixtures **103 → 104** (`0102-tracing-custom-tags-literal`) · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0277** (next-free ADR-0278) · **+0 go.mod modules, +0 packages**.

## Verify evidence (filled at Task 7 — verbatim, from the controller's Task-7 run)

- six-gate: ALL GREEN — `gofmt -l internal/ test/ cmd/` no output; `golangci-lint run ./...` exit 0; `go vet ./...` clean; `go build ./...` clean; `go mod tidy -diff` empty (exit 0); `go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...` all ok (`internal/tracing` 1.080s, `internal/filter/hcm` 1.063s, `internal/filter/hcm/h2` 1.644s).
- 104-dir differential (`go test ./test/differential/ -count=1`): `ok github.com/pgdad/envoy-go/test/differential 342.119s`, EXIT_CODE=0, byte-stable except the new `0102`.

**Landed task commits:** T1 `33a21094` · T2 `4f24df7e` · T3 `4f5fe06e` · T4 `903f633c` · T5 `5b92708a` · T6 `40bf8d2e`.

**Exit counts (confirmed against the landed tree):** stat surface **1201** (+0) · fixtures **103 → 104** (`0102-tracing-custom-tags-literal`) · fuzzers **54** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0276 → ADR-0277** (next-free **ADR-0278**). Row 59 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).
