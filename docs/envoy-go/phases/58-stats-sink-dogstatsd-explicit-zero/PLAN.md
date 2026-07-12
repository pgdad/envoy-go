# Phase 58 Implementation Plan — `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix: a SINGLE reject arm in `parseDogStatsdSinkConfig` (`bootstrap.go:712`) mirroring the phase-57 graphite arm (`bootstrap.go:750-752`) VERBATIM modulo the sink name (`dog_statsd max_bytes_per_datagram must be greater than 0`, ADR-0080-distinct) + the CONVERSION of the pre-existing accept-test into a reject arm + 3 `bootstrap.go` doc fixes + 4 `BEHAVIOR_CONTRACT.md` edits + one fuzz SEED (count STAYS 54) — NO new fixture / NO new fuzzer / +0 stats / +0 packages / +0 modules — a SINGLE FLAT ROW; ANCHORS ADR-0276; row 58 flips `done` at this IMPL six-gate (the SOLE leg)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended, per `feedback_execution_style`) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`, e.g. `.worktrees/phase-58-impl`, branch `phase-58-stats-sink-dogstatsd-explicit-zero-impl`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Execution lessons: the global CLAUDE.md makes dispatched subagents AUTO-COMMIT (`feedback_subagent_autocommit_claudemd`) — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF (Task 2 Step 5), and squashes + pushes at stage-close. Every task brief must pin the CANONICAL WORKTREE ROOT + worktree-relative paths (`feedback_subagent_worktree_path_targeting`) and carry the GIT-HYGIENE block (`git restore` only — no `checkout <sha>`, no `commit --amend`; re-verify the branch is undetached after each task, `feedback_subagent_worktree_detach`). This row is TINY, but the traps still apply.

**Goal:** A bootstrap `stats_sinks[]` entry whose `typed_config` is `envoy.config.metrics.v3.DogStatsdSink` carrying an EXPLICIT `max_bytes_per_datagram: 0` now boot-REJECTS with envoy-go's own message `bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0` — mirroring the reference's PGV `gt: 0` rule (`stats.pb.validate.go:1144-1157`) that the reference boot-rejects (SPEC-58 §11 A1, live-pinned exit 1) and that the phase-57 graphite arm already enforces. An ABSENT `max_bytes_per_datagram` still parses to `0` = one-line-per-datagram (unchanged — the `*wrapperspb.UInt64Value` wrapper makes nil-absent distinguishable from explicit-0). Closes the reference-parity gap SPEC-57 §2 recorded.

**Architecture:** A FOUR-line reject arm added to `parseDogStatsdSinkConfig` (`internal/bootstrap/bootstrap.go`) before the existing `append` (`:721`), `if w := dsd.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 { return fmt.Errorf(...) }` — byte-for-byte the graphite arm at `bootstrap.go:750-752` with `graphite_statsd`→`dog_statsd`. Every referenced symbol (`dsd.GetMaxBytesPerDatagram`, `*wrapperspb.UInt64Value`, `fmt.Errorf`) is already imported and consumed by the existing `bootstrap.go:724`. No new interface/type/seam/package/module. The row's actual RISK is the NON-additive test edit (the pre-existing `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` asserts the OLD accept behavior and must CONVERT to a reject) and the doc reconciliation across three `bootstrap.go` comments + four `BEHAVIOR_CONTRACT.md` sites that assert the old semantics. Byte-identical when no explicit-0 dog_statsd cap is configured; regression anchor = the full 103-dir differential (none of which configures one).

**Tech Stack:** Go 1.23.0, module `github.com/pgdad/envoy-go` (re-derived from `go.mod` this PLAN — NOT `esalaine`, the PLAN-50 stale-module-path warning per `feedback_brief_citations_not_evidence`). Proto: `github.com/envoyproxy/go-control-plane/envoy/config/metrics/v3` (`DogStatsdSink`) — ALREADY imported by `bootstrap.go` as `metricsconfigv3` and consumed at `:713`/`:724`. **ZERO new go.mod modules, ZERO new packages.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1201 → 1201** (+0 — a parse-reject registers no stat); fixtures **103 → 103** (+0 — a boot-reject never reaches the differential runner, `reference_differential_fixture_dispatch_constraint`; tail stays `0101-stats-sink-graphite`); fuzzers **54 → 54** (+0 — a SEED to the EXISTING `FuzzDogStatsdSinkConfigParse`, NOT a new fuzzer; reconcile per `reference_fuzzer_count_docs_drift` — re-count `^func Fuzz` after the edit); BackendKind **38 → 38** (`H2GoawayResponder` stays the tail); DECISIONS tail **ADR-0275 → ADR-0276** (the §Decision/§Consequences land at Task 6 per ADR-0044; next-free ADR-0277); **+0 go.mod modules, +0 packages** (`go mod tidy -diff` EMPTY at every task).
- **Process anchors:** ADR-0044 (ADR §Context was drafted at the SPEC — SPEC-58 §13; §Decision/§Consequences land at THIS IMPL, Task 6) · ADR-0045 (escape-valve UNCONSUMED — a single flat row of 6 tasks, margin ~9 under the `~15` ceiling; re-confirm at Task 1) · ADR-0080 (strict-reject anti-silent-divergence — the message substring `dog_statsd max_bytes_per_datagram must be greater than 0` is DISTINCT from graphite's `graphite_statsd max_bytes_per_datagram must be greater than 0`; the sink-name prefix IS the distinguisher) · ADR-0106 (row 58 flips `done` here, the SOLE leg — NO parent rollup) · ADR-0266/0267 (the dog_statsd sink substrate this row hardens) · ADR-0275 (the graphite arm this row mirrors) · ADR-0276 (this row — ANCHORED at Task 6).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit for the code task (Task 2). The failing test and the impl live in the SAME task so the task ends GREEN (the phase-57 Task-2 precedent) — see the "Task boundary note" below.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on touched packages + `go vet` + `go build ./...` for every code task.
- **Break protocol** (`reference_differential_break_protocol_count1`): the Task-2 deliberate-break liveness run uses `-count=1` (go-test caching serves a stale PASS otherwise); after the break, CONFIRM WHICH assertion fired from the failure output (`reference_deliberate_break_wrong_assertion` — the converted reject row must fire, NOT an earlier abort); `Errorf` per independent property, `Fatalf` only for a broken precondition (`reference_fatalf_makes_assertions_unreachable` — the `TestDogStatsdSink_Rejects` harness already uses `Fatalf` on the `err == nil` precondition and `Errorf` per substring, correctly).
- **The NON-additive test edit is the load-bearing risk** (`D-DZ-TESTROWS`, SPEC-58 §10): `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` (`bootstrap_test.go:2550`) asserts the OLD (now-wrong) accept behavior — DELETE it, do NOT merely supplement. `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (`:2528`) STAYS (the wrapper-type asymmetry absent-accept-vs-explicit-0-reject is the whole point). `TestDogStatsdSink_AcceptMaxBytesPerDatagram512` (`:2503`) STAYS (512 ≠ 0, still accepted).
- **Sentinel discipline** (Task 6): after the ROADMAP deferred-list roll (dog_statsd OUT), re-run the sentinel check-(2) grep — EXACTLY ONE live "candidates:" match, correct content (`reference_sentinel_deferred_sentence_live_vs_historical`). OTLP-metrics + the tracing quartet remain ⇒ check (2) still prints ⇒ the sentinel still does NOT fire ⇒ do NOT create `stop`.
- **`next-prompt.txt` IS TRACKED** (`reference_next_prompt_tracked_despite_gitignore`): edit it inside the stage worktree, fold into the stage squash; locate prior squashes by SUBJECT (`git log --grep`), never by position.
- **RE-DERIVE every `file:line` from source AGAIN at the IMPL** (`feedback_brief_citations_not_evidence` — this PLAN's citations are RE-VERIFIED against master `3c6739ae` but a brief's citation is not evidence; the files evolve). The roster is §"Edit-site roster" below (from SPEC-58 §12).

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding the SMALLEST row the project has chartered: a single strict-reject arm that hardens an already-landed sink to reference parity, plus doc reconciliation. **No new file, no new package, no new module, no new fixture, no new fuzzer, no new stat, no new BackendKind.** The ONLY novel production content is four lines (the reject arm), which are a byte-for-byte copy of the graphite arm that landed at phase 57 modulo the sink-name prefix.

**Why the fix is possible AND safe.** `DogStatsdSink.max_bytes_per_datagram` is a `*wrapperspb.UInt64Value`: nil ⇔ absent (skip the check, accept, parse to 0 = one-line-per-datagram), non-nil-with-value-0 ⇔ explicit 0 (reject). The `w != nil` guard is what preserves the absent-accept path verbatim — it mirrors the reference PGV's own `wrapper != nil` guard (`stats.pb.validate.go:1144`). For a `uint64`, `w.GetValue() == 0` and the reference's `<= 0` are equivalent (no negatives); envoy-go uses `== 0` to match the graphite arm byte-for-byte.

**What ALREADY works (verified at PLAN time 2026-07-12 against master `3c6739ae`; RE-CONFIRM line numbers before editing — files evolve):**

- **`internal/bootstrap/bootstrap.go`** —
  - `parseDogStatsdSinkConfig` (`:712-727`): unmarshal into `metricsconfigv3.DogStatsdSink` (`:713`), `parseUDPSinkAddressAndPrefix(... "dog_statsd", "dog_statsd_specifier", idx)` (`:717`, the missing-specifier reject), then the `append` (`:721-725`) with `MaxBytesPerDatagram: dsd.GetMaxBytesPerDatagram().GetValue()` UNCHECKED (`:724` — the gap). The reject arm goes **before the append** (`:721`), exactly where the graphite arm sits.
  - **`parseGraphiteStatsdSinkConfig`** (`:741-759`) is the TEMPLATE: its reject arm is at `:750-752`:
    ```go
    if w := g.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
        return fmt.Errorf("bootstrap: stats_sinks[%d]: graphite_statsd max_bytes_per_datagram must be greater than 0", idx)
    }
    ```
  - Three STALE doc comments to fix (§"Edit-site roster"): the `DogStatsdSinkConfig` struct-field comment (`:329`, `0 (absent or explicit) means "one metric per datagram"`) — flip to absent-only, mirroring the graphite field comment (`:345`, `0 (absent only — explicit 0 is parse-rejected)`); the `parseDogStatsdSinkConfig` func doc (`:702-711`, `0 (absent or explicit) means one metric per datagram (phase-49 behavior, UNCHANGED)` at `:704-705`) — split absent-accept from explicit-0-reject; the `parseGraphiteStatsdSinkConfig` NOTE (`:738-740`, `the landed dog_statsd parse does NOT enforce its identical PGV rule … NOT fixed here`) — now FALSE, update to name phase 58 (ADR-0276). The graphite struct doc (`:332-346`) and func doc (`:729-740`) are the mirror templates to copy the wording from.
- **`internal/bootstrap/bootstrap_test.go`** —
  - `TestDogStatsdSink_Rejects` (`:2458-2501`): table of `{name, topLevel, errSubs}`; per case `Load` → `if err == nil { Fatalf }` → `for sub := range errSubs { if !Contains { Errorf } }`. Two existing rows (`missing_dog_statsd_specifier`, `sibling_unknown_typeurl`). ADD an `explicit_max_bytes_per_datagram_zero` row here.
  - The graphite reject row `explicit_max_bytes_per_datagram_zero` (`:2889-2900`) is the TEMPLATE shape:
    ```go
    {
        name: "explicit_max_bytes_per_datagram_zero",
        topLevel: `stats_sinks:
      - name: envoy.stat_sinks.graphite_statsd
        typed_config:
          "@type": ` + graphiteStatsdSinkType + `
          address:
            socket_address: {address: 127.0.0.1, port_value: 8125}
          max_bytes_per_datagram: 0
    `,
        errSubs: []string{"bootstrap:", "graphite_statsd max_bytes_per_datagram must be greater than 0"},
    },
    ```
  - `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` (`:2548-2569`) — CONVERT (delete). `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (`:2526-2546`) — KEEP. `TestDogStatsdSink_AcceptMaxBytesPerDatagram512` (`:2503-2524`) — KEEP. The local `dogStatsdSinkType` const is already defined and used by these tests.
- **`internal/bootstrap/dogstatsd_fuzz_test.go`** — `FuzzDogStatsdSinkConfigParse` (the existing fuzzer; seed corpus fed through `Load`, no-panic contract). Current seeds include a `max_bytes_per_datagram: 512` seed (`:46-53`, comment mislabeled "(reject)" — 512 is ACCEPTED). The graphite fuzzer's explicit-0 seed (`graphite_fuzz_test.go:63-70`) is the template:
    ```go
    // explicit max_bytes_per_datagram: 0 (reject)
    f.Add([]byte(head + `stats_sinks:
      - name: envoy.stat_sinks.dog_statsd
        typed_config:
          "@type": ` + dogStatsdType + `
          address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
          max_bytes_per_datagram: 0
    `))
    ```
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — four edit sites (`:785` dog_statsd Strict-rejects, `:787` dog_statsd batching, `:821` graphite Strict-rejects NOTE, `:834` consumption summary — §"Edit-site roster").

---

## Task boundary note (a deliberate, documented consolidation of the SPEC/router T2+T3 sketch)

SPEC-58 §10 and the router sketch the code work as two tasks — **T2** "the reject arm + doc fixes" and **T3** "the test convert + the `-count=1` liveness break." This PLAN MERGES them into ONE TDD task (Task 2) because splitting the test from the impl across a task boundary would leave Task-2-as-test committed RED (the new reject row fails until the arm exists) — violating the project's commit-per-task-ends-green discipline (subagents auto-commit, `feedback_subagent_autocommit_claudemd`; a red commit is a defect). This is exactly the phase-57 Task-2 shape (failing-test Step 1 + impl Step 3 in one task, ending green). The router's INTENT is fully honored: the test convert is the load-bearing edit (Task 2 Steps 1/3), the arm is the graphite mirror (Step 3), the doc fixes are atomic with the production `.go` change (Step 3, the phase-57 stale-comment precedent), and the arm is proven LIVE with a `-count=1` deliberate break (Step 5, controller-executed). The net task count is 6 (SPEC anticipated ~5-7; margin ~9 under the ADR-0045 `~15` ceiling — escape-valve UNCONSUMED).

---

## File structure (decomposition locked here)

**Production (modified):**
- `internal/bootstrap/bootstrap.go` — the reject arm in `parseDogStatsdSinkConfig` (before `:721`) + three doc-comment fixes (`:329`, `:702-711`, `:738-740`).

**Test (modified):**
- `internal/bootstrap/bootstrap_test.go` — DELETE `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero`; ADD the `explicit_max_bytes_per_datagram_zero` row to `TestDogStatsdSink_Rejects`.
- `internal/bootstrap/dogstatsd_fuzz_test.go` — ADD the explicit-0 seed (no new fuzzer).

**Docs (modified, Tasks 4 & 6):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (four edits, Task 4), `docs/envoy-go/DECISIONS.md` (ADR-0276 body, Task 6), `docs/envoy-go/STATE.md` (Task 6), `docs/envoy-go/ROADMAP.md` (row 58 `done` + deferred-list roll, Task 6), `docs/envoy-go/phases/58-stats-sink-dogstatsd-explicit-zero/PROGRESS.md` (scaffolded at THIS PLAN session; Task 1 fills baselines, Task 6 closes it), `next-prompt.txt` (the router roll, controller-owned, Task 6).

**Created:** NONE (no new production file, test file, fixture, or fuzzer).

---

## Task 1: Baselines into the PROGRESS.md scaffold + the final ADR-0045 re-check

**Files:**
- Modify: `docs/envoy-go/phases/58-stats-sink-dogstatsd-explicit-zero/PROGRESS.md` (scaffold committed by THIS PLAN session)

- [ ] **Step 1: Record the baseline counts** (verbatim outputs pasted into PROGRESS.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                          # expect 103 (tail 0101-stats-sink-graphite)
grep -rn '^func Fuzz' --include='*.go' . | wc -l                        # expect 54
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go        # the BackendKind tail (38)
go mod tidy -diff                                                       # expect EMPTY
grep -n 'max_bytes_per_datagram must be greater than 0' internal/bootstrap/bootstrap.go   # expect ONE hit (graphite, :751) — dog_statsd not yet added
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1          # expect ## ADR-0275 (next-free ADR-0276)
```
Baseline: stat surface **1201** / fixtures **103** / fuzzers **54** / BackendKind **38** / DECISIONS tail **ADR-0275** (next-free **ADR-0276**).

- [ ] **Step 2: Confirm the ADR-0045 split disposition** in PROGRESS.md (SINGLE FLAT ROW, 6 tasks, margin ~9 under the `~15` ceiling; escape-valve UNCONSUMED) and the anticipated exit counts (stat **1201** +0 / fixtures **103** +0 / fuzzers **54** +0 / BackendKind **38** +0 / DECISIONS **ADR-0276**).

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/58-stats-sink-dogstatsd-explicit-zero/PROGRESS.md
git commit -m "phase 58 Task 1: baselines into PROGRESS + ADR-0045 single-flat-row re-check (dog_statsd explicit-zero parity; ANCHORS ADR-0276; row 58 flips done at this IMPL)"
```

---

## Task 2: The reject arm + the test convert + the three `bootstrap.go` doc fixes + the `-count=1` liveness break [TDD; CONTROLLER-EXECUTED break]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`, `internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: `dsd.GetMaxBytesPerDatagram()` (`*wrapperspb.UInt64Value`), `fmt.Errorf` — both already imported/used at `bootstrap.go:724`/`:715`. No new symbol.
- Produces: no exported surface change (a parse-time reject); the `TestDogStatsdSink_Rejects` table gains the `explicit_max_bytes_per_datagram_zero` case.

- [ ] **Step 1: Write the failing test** — CONVERT the test surface in `bootstrap_test.go`:
  - **DELETE** `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` (`:2548-2569`) entirely (its doc comment too — it asserts the now-wrong accept behavior).
  - **ADD** to `TestDogStatsdSink_Rejects`'s `cases` slice (`:2464-2487`, after the `sibling_unknown_typeurl` case), the graphite-mirror row:
```go
		{
			name: "explicit_max_bytes_per_datagram_zero",
			topLevel: `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdSinkType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
      max_bytes_per_datagram: 0
`,
			errSubs: []string{"bootstrap:", "dog_statsd max_bytes_per_datagram must be greater than 0"},
		},
```
  - LEAVE `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (`:2526`) and `TestDogStatsdSink_AcceptMaxBytesPerDatagram512` (`:2503`) untouched.

- [ ] **Step 2: Run to verify it fails** — the arm does not exist yet, so explicit-0 still parses to an accepted config and `Load` returns nil:
```bash
go test ./internal/bootstrap/ -run 'TestDogStatsdSink_Rejects/explicit_max_bytes_per_datagram_zero' -count=1 -v
```
Expected: FAIL — `Load: want error for explicit_max_bytes_per_datagram_zero, got nil` (the `Fatalf` on the `err == nil` precondition). (Use the full `TestDogStatsdSink_Rejects/explicit_max_bytes_per_datagram_zero` selector, NOT a bare fragment — `reference_differential_run_selector`.)

- [ ] **Step 3: Implement** in `bootstrap.go`:
  - **The reject arm** — in `parseDogStatsdSinkConfig`, BEFORE the `append` (currently `:721`), after the `parseUDPSinkAddressAndPrefix` error check (`:718-720`):
```go
	if w := dsd.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0", idx)
	}
```
  - **Doc fix 1 — the `DogStatsdSinkConfig` struct-field comment** (`:329`): replace
    `MaxBytesPerDatagram uint64 // NEW (ADR-0267): 0 (absent or explicit) means "one metric per datagram" (phase-49 behavior, UNCHANGED); >0 batches consecutive lines up to the cap`
    with (mirroring the graphite field comment at `:345`):
    `MaxBytesPerDatagram uint64 // 0 (absent only — explicit 0 is parse-rejected, ADR-0276) = one metric per datagram (phase-49 behavior); >0 batches consecutive lines up to the cap`
  - **Doc fix 2 — the `parseDogStatsdSinkConfig` func doc** (`:704-705`): replace
    `// parses the max_bytes_per_datagram field (ADR-0267, phase-50): 0 (absent or\n// explicit) means one metric per datagram (phase-49 behavior, UNCHANGED); >0`
    with
    `// parses the max_bytes_per_datagram field (ADR-0267, phase-50): an ABSENT\n// max_bytes_per_datagram parses to 0 = one metric per datagram (phase-49\n// behavior); an EXPLICIT max_bytes_per_datagram: 0 is a REFERENCE-PARITY reject\n// (ADR-0276, the PGV gt:0 rule — the *wrapperspb.UInt64Value wrapper makes\n// absent distinguishable from explicit 0, mirroring parseGraphiteStatsdSinkConfig); >0`
    (keeping the rest of the func doc — the missing-specifier reject sentence, the oneof note, the protocol-ignored/prefix-default lines — verbatim).
  - **Doc fix 3 — the `parseGraphiteStatsdSinkConfig` NOTE** (`:738-740`): replace
    `// NOTE the landed dog_statsd parse does NOT enforce its identical PGV rule\n// (bootstrap.go parseDogStatsdSinkConfig consumes GetValue() unchecked) —\n// a pre-existing phase-50 parity gap, deferred (SPEC-57 §2), NOT fixed here.`
    with
    `// NOTE the sibling dog_statsd parse enforces the IDENTICAL PGV rule as of\n// phase 58 (ADR-0276) — parseDogStatsdSinkConfig rejects an explicit\n// max_bytes_per_datagram: 0 with its own dog_statsd-prefixed substring.`

- [ ] **Step 4: Run to verify it passes** — full package (proves no other test asserted the old accept behavior):
```bash
go test ./internal/bootstrap/ -count=1
```
Expected: ALL PASS — the new reject row passes; `TestDogStatsdSink_AcceptMaxBytesPerDatagramAbsent` (absent ⇒ 0) and `...AcceptMaxBytesPerDatagram512` (512 ⇒ 512) still pass (the arm skips nil and non-zero wrappers). `go mod tidy -diff` ⇒ EMPTY.

- [ ] **Step 5: Prove the arm LIVE — the `-count=1` deliberate break** [CONTROLLER-EXECUTED, never delegated]. Temporarily DELETE the reject arm (the 3 lines added in Step 3), then:
```bash
go test ./internal/bootstrap/ -run 'TestDogStatsdSink_Rejects/explicit_max_bytes_per_datagram_zero' -count=1 -v
```
Expected: FAIL with `Load: want error for explicit_max_bytes_per_datagram_zero, got nil` — CONFIRM this exact line fires (the converted reject row), NOT an unrelated abort (`reference_deliberate_break_wrong_assertion`). The `-count=1` defeats go-test caching (`reference_differential_break_protocol_count1`). Then RESTORE the arm (`git restore internal/bootstrap/bootstrap.go` reverts ONLY the arm deletion — re-apply Step 3's arm if the restore is broader; NO `checkout <sha>`, NO `--amend`, `feedback_subagent_worktree_detach`); re-run `go test ./internal/bootstrap/ -count=1` ⇒ GREEN. Record the observed failure line in PROGRESS.md.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git status --porcelain   # ONLY bootstrap.go + bootstrap_test.go modified; the break reverted
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 58 Task 2: dog_statsd explicit-max_bytes_per_datagram:0 reject arm in parseDogStatsdSinkConfig (mirrors the phase-57 graphite arm modulo the sink name; ADR-0080-distinct substring) + CONVERT the accept-test to a TestDogStatsdSink_Rejects arm (Absent/512 accept-tests kept) + 3 stale bootstrap.go doc fixes; arm proven LIVE via a -count=1 deliberate break (ADR-0276)"
```

---

## Task 3: The `FuzzDogStatsdSinkConfigParse` explicit-0 seed (fuzzers 54 → 54)

**Files:**
- Modify: `internal/bootstrap/dogstatsd_fuzz_test.go`

- [ ] **Step 1: Add the seed.** In `FuzzDogStatsdSinkConfigParse`, after the existing `max_bytes_per_datagram: 512` seed (`:46-53`), add the explicit-0 seed (the graphite precedent `graphite_fuzz_test.go:63-70`):
```go
	// explicit max_bytes_per_datagram: 0 (reject — ADR-0276)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.dog_statsd
    typed_config:
      "@type": ` + dogStatsdType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
      max_bytes_per_datagram: 0
`))
```
  **OPTIONAL in-passing fix:** the existing `512` seed's comment (`:46`) reads `// max_bytes_per_datagram set (reject)` but 512 is ACCEPTED — correct it to `// max_bytes_per_datagram: 512 set (accept)` while editing this block (a phase-57-precedent in-passing doc fix; a micro-scope fold, not required).

- [ ] **Step 2: Run the fuzzer (seed corpus only) + reconcile the count:**
```bash
go test ./internal/bootstrap/ -run 'FuzzDogStatsdSinkConfigParse' -count=1        # PASS — Load never panics on the new seed (it returns a reject error)
grep -rn '^func Fuzz' --include='*.go' . | wc -l                                  # 54 (a SEED never adds a fuzzer — reference_fuzzer_count_docs_drift)
```
Expected: PASS; the `^func Fuzz` count STAYS **54**.

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
git add internal/bootstrap/dogstatsd_fuzz_test.go
git commit -m "phase 58 Task 3: seed FuzzDogStatsdSinkConfigParse with the explicit-max_bytes_per_datagram:0 reject arm (graphite fuzzer precedent; fuzzers stay 54 -- a seed never adds a fuzzer; ADR-0276)"
```

---

## Task 4: The four `BEHAVIOR_CONTRACT.md` edits

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Edit site #1 — dog_statsd Strict-rejects** (`:785`). ADD the explicit-0 reject arm to the dog_statsd "Strict-rejects (boot)" sentence, mirroring the graphite Strict-rejects wording (`:821`): after "a missing `dog_statsd_specifier` / nil `socket_address`", add "; and an EXPLICIT `max_bytes_per_datagram: 0` — a REFERENCE-PARITY reject (ADR-0276; the reference's PGV `gt: 0` rule, IDENTICAL to graphite's phase-57 arm; possible because the field is a `*wrapperspb.UInt64Value` so nil-absent is distinguishable from explicit-zero)." **OPTIONAL in-passing fold:** the line's "(naming all three supported sinks)" parenthetical is a latent phase-57 staleness — the sibling-reject message now names FOUR sinks; correct "three" → "four" while editing this line (a phase-57-precedent in-passing doc fix).

- [ ] **Step 2: Edit site #2 — dog_statsd batching** (`:787`). Split the now-wrong sentence "An ABSENT `max_bytes_per_datagram` (or an explicit `0`) continues to emit EXACTLY one line per datagram …": change it to "An ABSENT `max_bytes_per_datagram` continues to emit EXACTLY one line per datagram, byte-identical to phase 49; an EXPLICIT `max_bytes_per_datagram: 0` is now a boot-reject (ADR-0276, reference parity) — it never reaches the sink." Keep the rest of the batching paragraph (the general two-comparison accumulate-then-flush algorithm) verbatim.

- [ ] **Step 3: Edit site #3 — graphite Strict-rejects NOTE** (`:821`). Flip the trailing NOTE "**NOTE:** the reference's `DogStatsdSink` proto carries the IDENTICAL PGV `gt: 0` rule that graphite's parse arm now enforces, but the landed phase-50 dog_statsd parse does NOT enforce it (an explicit `0` is consumed unchecked) — a pre-existing parity gap, recorded as a deferred candidate, NOT fixed by this row." to "**NOTE:** the sibling `DogStatsdSink` parse enforces the IDENTICAL PGV `gt: 0` rule as of phase 58 (ADR-0276) — an explicit `max_bytes_per_datagram: 0` is boot-rejected there too, closing the parity gap this NOTE originally recorded."

- [ ] **Step 4: Edit site #4 — the consumption summary** (`:834`). In the "Does not yet apply to (stats sinks)" bullet, drop the "EXCEPT" clause: change "the dog_statsd sink's `address`/`prefix`/`max_bytes_per_datagram` are ALL consumed (phase 49 + phase 50) EXCEPT the explicit-`max_bytes_per_datagram: 0` PGV reject, a recorded deferred parity candidate (phase 57 §2)" to "the dog_statsd sink's `address`/`prefix`/`max_bytes_per_datagram` are ALL consumed (phase 49 + phase 50), INCLUDING the explicit-`max_bytes_per_datagram: 0` reject (phase 58, ADR-0276)". Leave the graphite clause that follows it intact.

- [ ] **Step 5: Verify the historical sites were NOT touched** — ADR-0275 §Consequences and any SPEC-57 §2/§11 references are point-in-time records (ADR-0276 references them as the gap's provenance); confirm the diff touches only the four active sites:
```bash
git diff --stat docs/envoy-go/BEHAVIOR_CONTRACT.md   # only BEHAVIOR_CONTRACT.md, ~4 hunks
```

- [ ] **Step 6: Commit**
```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 58 Task 4: BEHAVIOR_CONTRACT -- dog_statsd explicit-max_bytes_per_datagram:0 reject added (rejects + batching), graphite NOTE flipped to 'now enforced by phase 58', consumption summary EXCEPT-clause dropped (ADR-0276)"
```

---

## Task 5: Verification — +0 stat surface + the full 103-dir differential + the six-gate on the frozen HEAD

**Files:** none new — verification only.

- [ ] **Step 1: +0 stat surface** — a parse-reject registers no stat; the dog_statsd registration path is untouched:
```bash
go test ./internal/statssink/ -run 'TestNoNewStat' -count=1     # all four sink registration guards PASS
go test ./internal/bootstrap/ ./internal/stats/ -count=1        # PASS; surface unchanged 1201
```
- [ ] **Step 2: Full differential** (live Docker, 103 dirs — the regression anchor; NONE configures an explicit-0 dog_statsd cap, so all stay GREEN — byte-stability, SPEC-58 §3.3):
```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
- [ ] **Step 3: The six-gate:**
```bash
gofmt -l $(git diff --name-only master -- '*.go')   # empty
golangci-lint run ./...                             # clean
go vet ./...                                        # clean
go build ./...                                      # BUILD_OK
go test ./... -count=1                              # ALL PASS
go mod tidy -diff                                   # EMPTY
```
- [ ] **Step 4: Commit**
```bash
git commit --allow-empty -m "phase 58 Task 5: +0 stat surface (1201) via four registration guards; full 103-dir differential green (byte-stable — no fixture configures an explicit-0 cap); six-gate green"
```

---

## Task 6: ADR-0276 body + STATE/ROADMAP (row 58 `done`) + the sentinel re-check + PROGRESS close + the router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/58-stats-sink-dogstatsd-explicit-zero/PROGRESS.md`, `next-prompt.txt`

- [ ] **Step 1: ADR-0276** — copy the §Context draft from SPEC-58 §13 into DECISIONS.md (after ADR-0275), then append:
  - **§Decision** — a single reject arm in `parseDogStatsdSinkConfig` (`internal/bootstrap/bootstrap.go`, before the append) mirroring the phase-57 graphite arm VERBATIM modulo the sink name: `if w := dsd.GetMaxBytesPerDatagram(); w != nil && w.GetValue() == 0 { return fmt.Errorf("bootstrap: stats_sinks[%d]: dog_statsd max_bytes_per_datagram must be greater than 0", idx) }` (ADR-0080-distinct from graphite's substring; the `*wrapperspb.UInt64Value` wrapper's `w != nil` guard preserves the absent-accept path, matching the reference PGV's own `wrapper != nil` guard). The pre-existing `TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` accept-test CONVERTED into a `TestDogStatsdSink_Rejects` arm (the absent-accept + 512-accept sibling tests preserved); three `bootstrap.go` doc comments + four `BEHAVIOR_CONTRACT.md` sites reconciled; a seed added to the existing `FuzzDogStatsdSinkConfigParse` (no new fuzzer). A SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed); no seam ADR (the graphite reject shape is reused).
  - **§Consequences** — +0 stat surface (1201), +0 fixtures (103), +0 fuzzers (54), +0 BackendKind (38), +0 packages/modules; the dog_statsd and graphite sinks reach parse-time parity on `max_bytes_per_datagram`; the inherited hostname-accepting DEPARTURE (statsd/dog_statsd `net.ResolveUDPAddr` accepts hostnames the reference rejects) is orthogonal and unchanged; the Observability family STAYS OPEN (OTLP-metrics + the tracing quartet remain deferred). DECISIONS tail → **ADR-0276** (next-free ADR-0277).

- [ ] **Step 2: STATE.md** — active-phase → `phase 58 (stats-sink-dogstatsd-explicit-zero) IMPL done` (demote the SPEC/PLAN lines to prior); counts: stat **1201** / fixtures **103** (tail `0101-stats-sink-graphite`) / fuzzers **54** / BackendKind **38** / DECISIONS **ADR-0276** (next-free ADR-0277). NEXT = the next router decision (the Observability family + Operational-tooling stay open; five families never opened — the sentinel does NOT fire).

- [ ] **Step 3: ROADMAP.md** — row 58 → **`done`** (sole leg, ADR-0106 — `reference_roadmap_split_phase_row_done` does not apply, this is not a split row); the Observability family's LIVE deferred-candidates sentence rolls the dog_statsd candidate OUT (keeping OTLP-metrics + tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace). The new sentence:
  `remaining deferred (not-yet-chartered) candidates: OTLP-metrics stats sink + tracing custom_tags/spawn_upstream_span/http_service/force-trace.`

- [ ] **Step 4: Re-run the sentinel check-(2) grep** (`reference_sentinel_deferred_sentence_live_vs_historical`):
```bash
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md | wc -l   # EXACTLY 1
```
Expected: EXACTLY ONE live match, containing OTLP-metrics + the tracing quartet and NO dog_statsd. ⇒ check (2) still prints ⇒ the sentinel still does NOT fire ⇒ do NOT create `stop`. Also re-run check (1) (row 58 now `done` ⇒ prints nothing for row58) and check (3) (five families still never opened ⇒ prints them) to confirm the whole sentinel does not fire.

- [ ] **Step 5: PROGRESS.md** — all six tasks checked; the Task-2 break log line recorded (the observed `got nil` failure); FINAL counts pasted from re-run baseline commands (fixtures 103, fuzzers 54 via `grep -rn '^func Fuzz' --include='*.go' . | wc -l`, `go mod tidy -diff` EMPTY); anticipated-vs-actual MATCH on every count.

- [ ] **Step 6: Roll `next-prompt.txt`** (TRACKED — edit in the worktree, fold into the squash; locate prior squashes by SUBJECT only, `reference_next_prompt_tracked_despite_gitignore`): the phase-58 IMPL is DONE; the next stage is the next router decision (the Observability family + Operational-tooling remain open; five families never opened — the sentinel does NOT fire; do NOT create `stop`). Per the 2026-07-12 standing directive the roller SELF-PICKS the next subject (smallest defensible candidate — likely OTLP-metrics stats sink or a tracing follow-on; record the pick + rejected alternatives in that BRAINSTORM).

- [ ] **Step 7: Final suite on the frozen HEAD + commit**
```bash
go build ./... && go test ./... -count=1 && grep -rn '^func Fuzz' --include='*.go' . | wc -l   # ALL PASS; 54
git add docs/envoy-go/ next-prompt.txt
git commit -m "phase 58 Task 6: ADR-0276 full entry (§Decision/§Consequences) + STATE/ROADMAP (row 58 done; deferred-list rolls dog_statsd explicit-zero OUT, OTLP+tracing remain -- sentinel check-(2) still ONE live match) + PROGRESS close + router roll -- dog_statsd explicit-zero parity COMPLETE"
```

---

## Self-Review (run at PLAN authoring — issues found were fixed inline)

- **Spec coverage:** SPEC-58 §3.1 (the reject arm) → Task 2 Step 3; §3.2 (the three doc-comment fixes) → Task 2 Step 3; §3.3 (byte-stability / 103-dir regression) → Task 5 Step 2; §5 (proto roster — `max_bytes_per_datagram` now explicit-0-rejects) → Task 2; §6 (reject roster + the fuzzer SEED) → Tasks 2/3; §7 (+0 stat surface) → Task 5 Step 1; §8 (+0 fixtures — no new fixture) → covered (no fixture task); §9/§12 (the four BEHAVIOR_CONTRACT edits) → Task 4; §10 (the CONVERT-not-ADD test shape + `-count=1` liveness) → Task 2 Steps 1/5; §11 (the D-DZ-REJECTMSG probe pins) → honored verbatim in the arm's message + ADR-0276; §12 (edit-site roster) → Tasks 2/3/4; §13 (ADR-0276 §Context) → Task 6 Step 1; §14 (exit counts + ROADMAP/STATE) → Task 6. No gaps.
- **Every SPEC citation RE-DERIVED from source this session** (`feedback_brief_citations_not_evidence`): the arm site (`bootstrap.go:721` append, `:724` unchecked `GetValue`), the graphite template (`:750-752`), the three doc sites (`:329`, `:702-711`, `:738-740`), the graphite mirror docs (`:345`, `:729-740`), the test sites (`TestDogStatsdSink_Rejects` `:2458`, `...AcceptZero` `:2548` convert, `...AcceptAbsent` `:2526` keep, `...Accept512` `:2503` keep, the graphite reject row `:2889-2900`), the fuzz seed template (`graphite_fuzz_test.go:63-70`), and the four BEHAVIOR_CONTRACT sites (`:785`/`:787`/`:821`/`:834`) all verified against master `3c6739ae`. Module path confirmed `github.com/pgdad/envoy-go` (NOT the PLAN-50 stale `esalaine`).
- **Placeholder scan:** every code step carries the actual code (the arm, the reject row, the seed, the doc-comment before/after text); test names, error substrings, and commands are literal. No TBD/TODO.
- **Type consistency:** the arm's `w != nil && w.GetValue() == 0` matches the graphite arm byte-for-byte modulo the substring; the reject row's `errSubs` (`"bootstrap:"`, `"dog_statsd max_bytes_per_datagram must be greater than 0"`) matches the arm's `fmt.Errorf` output; the fuzz seed's YAML matches the reject-row YAML (explicit `max_bytes_per_datagram: 0`).
- **Task-boundary consolidation:** the SPEC/router T2+T3 sketch (arm-then-test) is merged into ONE TDD Task 2 (test-then-impl, ending GREEN) to avoid a red commit — documented in the "Task boundary note"; the `-count=1` liveness break is Task 2 Step 5 (controller-executed).
- **Sentinel discipline:** Task 6 Step 4 re-runs the check-(2) grep and asserts EXACTLY ONE live match after the roll — the dog_statsd candidate rolls OUT, OTLP + tracing remain, so the sentinel does NOT fire (`reference_sentinel_deferred_sentence_live_vs_historical`).

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/58-stats-sink-dogstatsd-explicit-zero/PLAN.md`.** Per the router + `feedback_execution_style`, the phase-58 IMPL is **subagent-driven** (superpowers:subagent-driven-development) in a FRESH worktree off master (`.worktrees/phase-58-impl`, branch `phase-58-stats-sink-dogstatsd-explicit-zero-impl`); subagents commit locally only; the controller verifies each commit, executes the Task-2 Step-5 `-count=1` liveness break ITSELF, re-runs the full suite on the frozen HEAD, and squashes + pushes at stage-close. The next router stage after this PLAN lands is the phase-58 IMPL.
