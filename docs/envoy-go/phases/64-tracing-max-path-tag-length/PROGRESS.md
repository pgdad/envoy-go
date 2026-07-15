# Phase 64 PROGRESS — tracing `max_path_tag_length` (ADR-0285; row 64 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-14, worktree `.worktrees/phase-64-plan`, branch `phase-64-tracing-max-path-tag-length-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`/`feedback_subagent_autocommit_claudemd`/`feedback_subagents_no_push`), fills the baseline block at Task 1, logs each task + every `-count=1` liveness-break outcome here, and closes it at Task 9.

## Task checklist (mirrors PLAN.md — 9 tasks, SINGLE FLAT ROW, ADR-0045 escape-valve UNCONSUMED)

- [ ] Task 1 — `config.go`: ADD `TracingConfig.MaxPathTagLength uint32` (`:25-40`); replace the `:112-114` `max_path_tag_length is unsupported` reject with the resolve arm (default 256 / explicit incl. 0) + set `cfg.MaxPathTagLength` post-dispatch alongside `cfg.CustomTags` (`:154`); `config_test.go` — REMOVE the `max_path_tag_length` reject row from `TestNewConfigRejectArms` (`:336-343`); ADD `TestNewConfigMaxPathTagLength` (explicit 128 / absent→256 / explicit-0-preserved); `-count=1` breaks on explicit + absent-default + explicit-0-preserved [TDD]
- [ ] Task 2 — NEW `internal/tracing/url.go`: `func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string` (byte-truncate `pathAndQuery` FIRST, then prepend `scheme://host`); NEW `url_test.go` matrix (under-cap / over-cap / explicit-0 / query-cut / default-256 / byte-boundary); `-count=1` break on dropping the truncation slice [TDD]
- [ ] Task 3 — `accesslog_emit.go`: rewire the THREE URL-build sites (H1 `:40` / H2 `:93` / H3 `:162`) through `tracing.BuildHTTPURL(..., f.tracingConfig.MaxPathTagLength)`; `span_emit_test.go` — ADD a `spanAttr` helper + H1 + H2 truncation tests (via `newTracingFilter`/`fakeExporter`, `MaxPathTagLength: 16`); confirm `span_test.go:100` (BuildServerSpan-direct http.url) STAYS green + existing bare-literal span-emit tests unaffected (ZERO-VALUE CAP TRAP); `-count=1` breaks on reverting the H1 site + the H2 site in isolation [TDD]
- [ ] Task 4 — `zipkin_test.go`: `TestZipkinEncodeTruncatedHTTPURL` (a truncated `http.url` surfaces verbatim in the Zipkin `tags` map; node_id/zone dropped); `-count=1` break on adding `http.url` to the encoder drop guard [TDD]
- [ ] Task 5 — NEW OTLP fixture `0107-tracing-max-path-tag-length` (clone `0106`; `max_path_tag_length: {value: 16}`; long-ASCII-path GET; cross-side `http.url` VALUE-equality on the truncated form `http://trace.example/abcdefghijklmno`; register in `runner_test.go`; `-count=1` differential break on the assert's expected VALUE only, `reference_vacuous_break_receiver_normalizes`; FULL `-run` selector, `reference_differential_run_selector`; authority-encoding-risk fallback documented)
- [ ] Task 6 — `FuzzHCMConfigParse` `max_path_tag_length` seed (incl. explicit 0) + `wrapperspb` import; fuzzers 55 → 55 (reconcile `^func Fuzz` before + after, `reference_fuzzer_count_docs_drift`)
- [ ] Task 7 — `BEHAVIOR_CONTRACT.md` (RE-DERIVE the drifted `:686` clause; `max_path_tag_length` REJECT → CONSUMED — truncates `http.url` `:path` to N bytes, default 256, explicit 0 = empty path, both exporters; sibling rejects STAY)
- [ ] Task 8 — Verify: six-gate (gofmt / golangci-lint / vet / build / `go mod tidy -diff` + `git diff go.mod` / `-race` on `internal/tracing` + `internal/filter/hcm`) + full 109-dir differential (byte-stable except `0107`) — controller-run on frozen HEAD, no-commit gate; evidence below
- [ ] Task 9 — ADR-0285 body (§Decision/§Consequences) + STATE + ROADMAP (row 64 `done` + deferred-sentence UNCHANGED + check-(2) re-run confirming one live Observability match) + PROGRESS close + router roll (docs-only; squash/push are the controller's stage-close)

**PHASE 64 CLOSES AT IMPL.** Row 64 flips `in-progress` → `done` at this IMPL six-gate (ADR-0106, the SOLE leg — `reference_roadmap_split_phase_row_done`). ANCHORS ADR-0285 (§Decision/§Consequences land in `DECISIONS.md`; §Context drafted at the SPEC, SPEC §13).

## D-MPTL-* question dispositions (ALL DISPOSED at the SPEC — SPEC-64 §1/§11, via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2`, fresh container per arm; EVERY anticipation HELD — no ADR-0044 flip)

- **D-MPTL-REFTRUNC → CONFIRMED** (§11 arm 0): the reference OTLP tracer TRUNCATES `http.url` at `max_path_tag_length` (cap=16, 27-byte path → `http://h.io/abcdefghijKLMNO`); provability confirmed, NO re-scope. Provider-neutral (`Tracing::HttpTracerUtility`), so applies identically to Zipkin.
- **D-MPTL-TARGET → PINNED** (§11 arm 0): only the `:path` (path+query) is truncated; the `scheme://host` prefix is added AFTER and NEVER truncated.
- **D-MPTL-DEFAULT → PINNED** (§11 arm 1): an ABSENT field caps at **256** (307-byte path → 11+256=267). The load-bearing behavior-change — envoy-go emits UNTRUNCATED today, so honoring 256 CLOSES a latent > 256 divergence; byte-stable for the < 256 corpus.
- **D-MPTL-ZERO → PINNED** (§11 arm 2): an explicit `max_path_tag_length: 0` truncates the `:path` to EMPTY → `scheme://host` only (NOT "unlimited"; the explicit 0 is preserved — phase-46 explicit-0 sampling precedent).
- **D-MPTL-QUERY → PINNED** (§11 arm 3): the query IS included in the truncated `:path`; a cut can land INSIDE it (`/p?query=abcdefghijklmnop` cap 16 → `/p?query=abcdefg`). ⇒ envoy-go's `r.URL.RequestURI()` (H1/H3) + `req.Path` (H2) are the correct unit.
- **D-MPTL-TRUNCUNIT → PINNED BYTES** (§11): every observed count byte-exact (16/256/0); Go `s[:n]` slices bytes. The `0107` fixture uses an ASCII path (byte==rune); the `url_test.go` matrix asserts byte truncation.
- **D-MPTL-REJECT → PINNED** (§6, §11 PGV re-derivation): `max_path_tag_length` is a `UInt32Value` with NO PGV numeric constraint (only the no-op embedded-wrapper switch) ⇒ absent + explicit-0 + any positive all structurally valid; NO new reject. The sibling tracing rejects STAY loud/distinct.
- **D-MPTL-TRUNC-LOCATION → DECIDED = Option A** (§3.3): a shared exported `tracing.BuildHTTPURL(scheme, host, pathAndQuery, maxPathTagLen)` at the three `accesslog_emit.go` URL-build sites; Option B (threading through `SpanInputs`/`BuildServerSpan`) REJECTED. `BuildHTTPURL`/`MaxPathTagLength` GREP-collision-confirmed clean.
- **D-MPTL-FIXTURE → DECIDED** (§8): ONE new OTLP fixture `0107` (fixtures 108 → 109), a small explicit cap + a long path, cross-side `http.url` VALUE-equality (deterministic from the request — NO harness env-injection).
- **D-MPTL-DEFAULT-PROOF → DECIDED** (§8): the default-256 truncation is a UNIT test on `BuildHTTPURL` (+ an ABSENT→256 config test), NOT a second > 256-path fixture; the ADR-0045 escape-valve stays UNCONSUMED.
- **D-MPTL-FUZZSEED → DECIDED** (§6): a SEED to the existing `FuzzHCMConfigParse`; fuzzer count STAYS 55.
- **D-MPTL-SPLIT → DECIDED** (§3.0): a SINGLE FLAT ROW (9 tasks); ADR-0045 escape-valve UNCONSUMED.
- **D-MPTL-DOCSHAPE → RE-DERIVED** (§12); re-verified against master tip `bb221ec5` at this PLAN (all `file:line` hold; `BuildHTTPURL`/`MaxPathTagLength` re-grep-confirmed collision-free).

## Baselines (fill at IMPL Task 1 — verbatim, against the phase-64 PLAN squash = master tip at IMPL start)

- `go build ./...`: (fill)
- fixtures (`ls -d test/fixtures/[0-9]*/ | wc -l`): (fill — expect 108, tail `0106-tracing-custom-tags-environment`)
- fuzzers (`grep -rhoE '^func Fuzz[A-Za-z0-9]+' --include='*.go' . | sort -u | wc -l`): (fill — expect 55)
- BackendKind tail: (fill — expect 38, `H2GoawayResponder`)
- `go mod tidy -diff`: (fill — expect empty; `GetMaxPathTagLength` getter + `wrapperspb` already-resolved; no NEW sub-package)
- stat surface: (fill — expect 1201; registration guards enforce +0)
- DECISIONS tail: (fill — expect `## ADR-0284`; body lands at T9 as ADR-0285, next-free ADR-0286)
- Anchors CONFIRMED vs PLAN roster: `config.go` reject `:112-114` / `cfg.CustomTags` `:154` / `TracingConfig` `:25-40`; `accesslog_emit.go` URL sites `:40`/`:93`/`:162`; `span_emit_test.go` `fakeExporter`/`newTracingFilter`; `config_test.go` reject row `:336-343` (no `wantSub`); `zipkin.go` drop guard; `fuzz_test.go` `:74`; `runner_test.go` `0106` register `:133`. `BuildHTTPURL`/`MaxPathTagLength` re-grep-confirmed collision-free.

## Liveness-break log (every break `-count=1`, confirmed WHICH fired, then restored byte-identical) — fill at IMPL

- **T1 (config, `config_test.go`):** (fill — explicit / absent-default-256 / explicit-0-preserved, each firing ONLY its own subtest)
- **T2 (helper, `url_test.go`):** (fill — dropping the truncation slice fires over-cap + explicit-0 + query-cut + default-256; boundary pinned by the exact-boundary row)
- **T3 (call sites, `span_emit_test.go`):** (fill — reverting the H1 site fires the H1 test; reverting the H2 site fires the H2 test; the existing bare-literal span-emit tests stay green)
- **T4 (Zipkin, `zipkin_test.go`):** (fill — adding `http.url` to the encoder drop guard fires the `tags[http.url]` assertion)
- **T5 (`0107` differential):** (fill — the assert's expected VALUE break fires BOTH sides' `http.url` assertion; VALUE-equality vs fallback decision recorded)

## Task-8 verify evidence (fill at IMPL — verbatim, controller-run on the frozen HEAD)

- six-gate: (fill)
- 109-dir differential (`go test ./test/differential/ -count=1`): (fill — byte-stable except the new `0107`)

**Landed task commits:** (fill — T1 … T9)

**Exit counts (confirm against the landed tree):** stat surface **1201** (+0) · fixtures **108 → 109** (`0107-tracing-max-path-tag-length`) · fuzzers **55** (+0) · BackendKind **38** (+0) · +0 packages · +0 go.mod modules · DECISIONS tail **ADR-0284 → ADR-0285** (next-free **ADR-0286**). Row 64 → `done` at this IMPL six-gate (ADR-0106, the SOLE leg).
