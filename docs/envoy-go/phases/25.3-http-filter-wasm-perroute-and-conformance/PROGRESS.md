# Phase 25.3 — Implementation PROGRESS

> Authoritative input: `docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/PLAN.md` (1,043-line PLAN; 15-task TDD task graph across 6 tiers A/B/C/D/E/F). 25.3 SPEC: `.../SPEC.md` (5 AMEND-C wire-shape pins + 7 D-pins + §15 32-item acceptance checklist). 25.3 BRAINSTORM: `.../BRAINSTORM.md` (Q1-Q7). Closest IMPL-execution precedent: `docs/envoy-go/phases/25.2-.../PROGRESS.md` (22-task subagent-driven + two-stage review + 6-gate evidence discipline) + `25.1-.../PROGRESS.md` (17-task).

**Scope.** Phase 25.3 is the **per-route + multi-plugin + reload + env_vars + conformance-harness THIRD-and-FINAL sub-phase** of `envoy.filters.http.wasm` (the NINETEENTH §9 production HTTP filter; parent envelope D 3-way PRE-SPLIT). 15 tasks across 6 tiers: Tier A (Tasks 1-4) `internal/wasm/` core evolution (NEW `registry.go` + raw-vm_id shared-data + NEW `reload.go` + NEW `env_vars.go`); Tier B (Task 5) phase-21 clock MIGRATION onto a unified `internal/clock` superset (F1 reconciliation); Tier C (Tasks 6-9) `internal/filter/http/wasm/` extensions (NEW `perroute.go`; compiled_config lift-5-arms + 3-NEW-arms + registry wiring; stats 128→132; dispatch integration); Tier D (Tasks 10-12) fuzzer FOLD + differential fixtures 0038 + 0039; Tier E (Tasks 13-14) `test/conformance/proxy-wasm/` harness seed + 10 family ports; Tier F (Task 15) atomic landing + ROLLUP close of parent row 25 + §9 family.

**Execution discipline.** `superpowers:subagent-driven-development` — fresh implementer subagent per task + two-stage review (spec compliance, then code quality) between tasks. Each task: failing-test-first → minimal-impl → run-with-expected-output → gates → commit.

**IMPL worktree:** `.worktrees/phase-25.3-http-filter-wasm-perroute-and-conformance-impl`. **IMPL branch:** `phase-25.3-http-filter-wasm-perroute-and-conformance-impl` (branched off master tip `85d39f7`).

---

## Pre-Task — baseline verification (verbatim, from the IMPL worktree root)

### Worktree branch
```
$ git rev-parse --abbrev-ref HEAD
phase-25.3-http-filter-wasm-perroute-and-conformance-impl
```
PASS.

### Master tip
```
$ git log --oneline -3 master
85d39f7 next-prompt.txt: repoint master-tip references to 4afb89a (actual HEAD)
4afb89a next-prompt.txt: rewrite for 25.3 IMPL cold-start (post-25.3-PLAN 1aec3fc/99519f0)
99519f0 phase 25.3 PLAN stage-close: STATE.md SHA-fill (TBD-25.3-PLAN-SQUASH -> 1aec3fc)
```
PASS — branched off `85d39f7` (docs-only repoint past the prompted-anticipated `4afb89a`).

### Toolchain
```
$ go version           → go1.26.2 linux/amd64           (≥ go1.23.0 wazero floor; AMEND-A1)
$ golangci-lint version → v1.64.8                         (ADR-0009 pin)
$ rustc --version       → 1.94.0 (4a4ef493e 2026-03-02)
$ rustup target ...     → wasm32-wasip1 INSTALLED          (offline .wasm build; CI needs no Rust)
$ docker version        → 28.4.0 client / 28.1.1 server    (differential harness only)
```
PASS.

### Baseline gates (build / vet / lint / race-short)
```
$ go build ./...            → exit 0
$ go vet ./...              → exit 0
$ golangci-lint run         → exit 0
$ go test -race -short ./... → exit 0 (all ok / no test files; no failures)
```
PASS — all four code gates GREEN.

### Baseline counts (reconciled vs PLAN)
```
$ grep -rn "^func Fuzz" --include=*_test.go | wc -l       → 35   (PLAN: 35 ✓)
   NOTE: unanchored `grep "func Fuzz"` returns 36 — the 36th is a COMMENT line at
   internal/filter/http/wasm/fuzz_hostcall_test.go:70 (`// grep -rh "^func Fuzz" ...`),
   NOT a fuzz target. The anchored `^func Fuzz` count is the true 35.
$ ls test/fixtures/ | grep -cE '^[0-9]{4}-'               → 37
   NOTE: 0007a-cors + 0007b-iteration-probe have a letter after the 4 digits so they
   are excluded by `^[0-9]{4}-`. Total fixture dirs INCLUDING 0007a/0007b = 39 (the
   "39/39 differential" gate count). 0000-0037 sequential. PLAN baseline 39 ✓.
$ stat total (wasm_test.go single-source-of-truth comments) → 128 (PLAN: 128 ✓)
```
PASS — fuzzers 35, fixtures 39, stat 128 all reconcile with the PLAN baseline.

### ADR tail
```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | ... | tail -1  → 212
```
PASS — tail ADR-0212 (3 §Context drafts 0210/0211/0212 anchored at SPEC); next-free ADR-0213.

### Differential 39/39 + h2spec 53/53
INHERITED-GREEN by docs-only-master-tip argument (25.3 PLAN + the next-prompt repoints
are docs-only; no code changed since the 25.2 phase-done gate that recorded 39/39 + 53/53).
Will be EXERCISED at Tasks 11/12 (differential → 41/41) + Task 15 (full six-gate run).

**Baseline disposition: GREEN. Cleared to begin Task 1.**

---
