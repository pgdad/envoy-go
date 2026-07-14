# Phase 62 PROGRESS — tracing `custom_tags` `request_header` SOURCE arm (ADR-0283; row 62 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-14, worktree `.worktrees/phase-62-plan`, branch `phase-62-tracing-request-header-custom-tag-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 9.

## Task checklist (mirrors PLAN.md — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [x] Task 1 — `resolve.go`: `CustomTagSpec`/`customTagKind` types (in `config.go`) + `ResolveCustomTags` resolver + `resolve_test.go` matrix (present / default / omit / multi→first / nil-lookup / literal / present-empty-not-default); purely additive (build green); `-count=1` breaks on first-value + omit-on-missing + present-empty [TDD] — commit `1ab35574`
- [x] Task 2 — `config.go`: reshape `parseCustomTags` → `([]CustomTagSpec, error)` (request_header accept + empty-name reject + first-wins dedup) + `TracingConfig.CustomTags []CustomTagSpec` field type + thread the THREE `accesslog_emit.go` call sites (`:55`/`:116`/`:177`) via `tracing.ResolveCustomTags(..., reqHeaderLookupH1/H2(...))`; `config_test.go` accept→spec shape + request_header accept + first-wins dedup + empty-name reject (DROP the stale `request_header`-reject row); ATOMIC build-green change; `-count=1` breaks on accept + empty-name substring + dedup [TDD] — commit `da0bc064`
- [x] Task 3 — `span_test.go`: CONFIRM `BuildServerSpan`/`upsertAttr` UNCHANGED + a resolved-request_header-KV-upserts-over-a-built-in test (arm B); `-count=1` break on always-append → count 2 [TDD] — commit `8a3f4fbe`
- [x] Task 4 — Zipkin encoder unit test (a resolved request_header tag in the `tags` map; node_id/zone dropped); `-count=1` break [TDD] — commit `28c7d536`
- [x] Task 5 — New OTLP fixture `0105-tracing-custom-tags-request-header` (clone `0102`; request_header tag `trace_user` from `x-trace-user`; driver SENDS the header on every request; present-case cross-side by key; register in `runner_test.go`; `-count=1` differential break — break the EXPECTATION only, `reference_vacuous_break_receiver_normalizes`; FULL `-run` selector, `reference_differential_run_selector`) — commit `3dee19f3`
- [x] Task 6 — `FuzzHCMConfigParse` request_header custom_tags seed (fuzzers 55 → 55; reconcile `^func Fuzz` before + after, `reference_fuzzer_count_docs_drift`) — commit `495112b6`
- [x] Task 7 — `BEHAVIOR_CONTRACT.md` edits (RE-DERIVE the drifted lines; `request_header` → CONSUMED; add empty-name reject; narrow the deferred bullet) — commit `514d78ae`
- [x] Task 8 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` + `git diff go.mod` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 107-dir differential (byte-stable except `0105`) — controller-run on HEAD `514d78ae`, no-commit gate; evidence below
- [x] Task 9 — ADR-0283 body (§Decision/§Consequences) + STATE + ROADMAP (row 62 `done` + deferred-sentence narrow + check-(2) re-run) + PROGRESS close + router roll (docs-only; squash/push are the controller's stage-close)

**PHASE 62 CLOSED AT IMPL (2026-07-14).** Row 62 flips `in-progress` → `done` at this IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0283 (§Decision/§Consequences landed in `DECISIONS.md`).

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

## Baselines (filled at IMPL Task 1 — verbatim command outputs, against master tip `460f761e` / the phase-62 PLAN commit `72704b02`)

- `go build ./...`: clean, no output
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): 106 (tail `0104-http3-downstream-get`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): 55
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): 38
- `go mod tidy -diff`: empty (`CustomTag_Header` getter is an already-resolved module; no NEW sub-package)
- request_header reject pre-check (`grep -n 'request_header type unsupported' internal/tracing/config.go`): `:156` (`tracing: custom_tags request_header type unsupported`, lifted by Task 2)
- stat surface: **1201** (docs-verified; registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): `## ADR-0282` (next-free ADR-0283)

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 9 tasks, margin ~6 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem to strand: the resolver + types, the parse reshape + field change, and the three call-site threadings all sit on the SAME tracing engine; the single new OTLP fixture does not spill into its own row.

## Liveness-break log (every break `-count=1`, confirmed WHICH fired, then restored byte-identical)

- **T1 (resolver, `resolve_test.go`):** three isolated breaks, each confirmed to fire ONLY its own subtest. (a) first-value: swapping `vs[0]` for `vs[len(vs)-1]` in the request_header multi-value arm fired the `multi→first` subtest (expected the FIRST configured header value, got the LAST), confirming `ResolveCustomTags` really does take the first value, not a comma-join or the last. (b) omit-on-missing: removing the `HasDefault` guard so an absent header with no default fell through to an emitted empty-string `KV` fired the `omit` subtest (expected NO `KV` for that key, found one), confirming the omit branch is load-bearing (a NEW semantic vs `literal`, which never omits). (c) present-empty: collapsing the present-empty-valued-header case into the "absent" branch (so a present-but-empty header value resolved to the configured default instead of the empty string) fired the `present-empty-not-default` subtest, confirming the lookup's boolean `ok` (not `len(vs[0]) > 0`) gates presence.
- **T2 (parse/config, `config_test.go`):** three isolated breaks. (a) request_header accept: reverting the new `GetRequestHeader() != nil` arm to the phase-59 reject (`tracing: custom_tags request_header type unsupported`) fired the accept-shape subtest (expected a `CustomTagSpec{Kind: kindRequestHeader,...}`, got a parse error), confirming the lift is live. (b) empty-name substring: removing the `h.GetName() == ""` check fired the empty-name-reject subtest (expected the `empty name` error, got a successful parse), confirming the NEW PGV-parity reject is load-bearing, not vacuously subsumed by the empty-`tag` check (which runs on a DIFFERENT field). (c) first-wins dedup: removing the `seen[tag]` drop-guard so a colliding-key second `CustomTag` appended instead of being dropped fired the dedup subtest (expected `len(specs) == 1` keeping the FIRST value, got `2` with the LAST value present), confirming the dedup is genuinely first-wins, not the old last-wins `upsertAttr` behavior masking it.
- **T3 (span arm B, `span_test.go`):** reverting `upsertAttr` to an unconditional append (the phase-59 pre-fix shape, restored deliberately) made the built-in-collision subtest see `count=2` where `want=1`, confirming the upsert-by-key loop still correctly overrides a resolved request_header `KV` colliding with a built-in key — i.e. `BuildServerSpan`/`upsertAttr` genuinely are unchanged and the guarantee (`ResolveCustomTags` emits unique keys) still holds end-to-end.
- **T4 (Zipkin, `zipkin_test.go`):** adding the resolved request_header tag's key to the Zipkin encoder's built-in drop set fired the assertion on `tags["trace_user"]` (expected present, found absent), confirming the resolved custom tag reaches the Zipkin `tags` map via the same shared `Attrs`/`ResolveCustomTags` path as the OTLP exporter.
- **T5 (`0105` differential):** per `reference_vacuous_break_receiver_normalizes`/`reference_deliberate_break_wrong_assertion`, the break touched the driver's EXPECTATION only (setting the expected `trace_user` value to a WRONG literal, not the wire encoding) — this fired the cross-side custom-tag assertion on BOTH the subject and the reference side (not just one, and not a different, earlier assertion), confirming the `0105` fixture's assertion is live end-to-end and not masked by an earlier `Fatalf`. Run via the full `-run 'TestDifferential/0105-tracing-custom-tags-request-header'` selector (`reference_differential_run_selector`) with `-count=1` (`reference_differential_break_protocol_count1`).

## Anticipated exit counts (from SPEC-62 §14)

stat surface **1201 (+0)** · fixtures **106 → 107** (`0105-tracing-custom-tags-request-header`) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0283** (next-free ADR-0284) · **+0 go.mod modules, +0 packages**.

## Verify evidence (filled at Task 8 — verbatim, from the controller's Task-8 run on HEAD `514d78ae`)

- six-gate: ALL GREEN — `gofmt -l internal/ test/ cmd/` no output; `go vet ./...` clean; `go build ./...` clean; `go mod tidy -diff` EMPTY + `git diff go.mod`/`go.sum` empty; `golangci-lint run ./...` exit 0; `go test -race -count=1 ./internal/tracing/... ./internal/filter/hcm/...` all ok (`internal/tracing` 1.077s, `internal/filter/hcm` 1.064s, `internal/filter/hcm/h2` 1.640s).
- 107-dir differential (`go test ./test/differential/ -count=1`): `ok github.com/pgdad/envoy-go/test/differential 355.138s`, EXIT_CODE=0, byte-stable except the new `0105` (106 pre-existing dirs unchanged).

**Landed task commits:** T1 `1ab35574` · T2 `da0bc064` · T3 `8a3f4fbe` · T4 `28c7d536` · T5 `3dee19f3` · T6 `495112b6` · T7 `514d78ae` (T8 verify was a no-commit gate run; T9 is this docs commit).

**Exit counts (confirmed against the landed tree):** stat surface **1201** (+0) · fixtures **106 → 107** (`0105-tracing-custom-tags-request-header`) · fuzzers **55** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0282 → ADR-0283** (next-free **ADR-0284**). Row 62 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).
