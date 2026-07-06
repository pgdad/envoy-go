# Phase 51 Implementation Progress — extract `internal/boot.Construct` + `internal/filter/http/builtins` + public `validate` package + `--mode validate` CLI flag

**Status:** IMPL DONE (6/6).

**Reference:** [PLAN-51.md](./PLAN-51.md) · [SPEC-51.md](./SPEC-51.md)

**Worktree branch:** `phase-51-bootstrap-validate-mode`

**Description:** The FIRST Operational-tooling-family row — make bootstrap config validation reachable from OUTSIDE the binary's normal boot path. Extract the tail of `cmd/envoy-go/main.go`'s construction sequence into a shared `internal/boot.Construct` function; extract the inline 20-call `httpReg` registration block into a new `internal/filter/http/builtins` package; add a new public package `github.com/esalaine/envoy-go/validate` (`Bootstrap`/`BootstrapFile` functions); add a `--mode validate` CLI flag to `main.go`. No change to WHAT is validated anywhere — every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` stays byte-for-byte unchanged. **ANCHORS ADR-0268.** Row 51 flips **`done`** at this IMPL six-gate (a single unsplit row — ADR-0106 not implicated); the Operational tooling family STAYS OPEN.

---

## Task Checklist

- [x] **Task 1:** Phase scaffolding — PROGRESS-51.md + baselines + the final ADR-0045 split re-check
- [x] **Task 2:** `internal/filter/http/builtins` extraction — mirrors the network sibling WHERE applicable (no `Deps` struct) [TDD]
- [x] **Task 3:** `internal/boot.Construct` extraction — the core refactor (Tasks 2-3 together move ~120 lines; Task 3 adds the listener manager wire) [TDD]
- [x] **Task 4:** `validate` package — the public facade (`Bootstrap`/`BootstrapFile` + construction-boundary tests) [TDD]
- [x] **Task 5:** CLI `--mode validate` flag + a test [TDD]
- [x] **Task 6:** Completion: ADR-0268 §Decision/§Consequences, BEHAVIOR_CONTRACT + ROADMAP edits, the six-gate (no fuzzer increment)

---

## Baseline Counts (Task 1 — Recorded at Session Start)

### Command Output

**Build:**
```
BUILD_OK
```

**Fixture count:**
```
96
```

**Fuzzer count:**
```
52
```

**Package status (internal/boot, internal/filter/http/builtins, validate):**
```
ls: cannot access 'internal/boot': No such file or directory
ls: cannot access 'internal/filter/http/builtins': No such file or directory
ls: cannot access 'validate': No such file or directory
```

**go mod tidy -diff (expect empty):**
```
(no output — clean)
```

**httpReg.Register count in main.go:**
```
20
```

**RegisterPerRouteValidator count in main.go:**
```
5
```

### Baseline Summary

| Metric | Baseline |
|--------|----------|
| Build | OK |
| Fixtures | 96 |
| Fuzzers | 52 |
| BackendKind tail | 38 |
| Stat surface (H2 cluster) | 1200 |
| Stat surface (non-H2) | 1196 |
| DECISIONS tail | ADR-0267 |
| Next-free ADR | ADR-0268 |
| httpReg.Register count | 20 |
| RegisterPerRouteValidator count | 5 |
| go.mod state | Clean (tidy -diff empty) |
| internal/boot package | Does not exist |
| internal/filter/http/builtins package | Does not exist |
| validate package | Does not exist |

---

## D-VALIDATE-SPLIT Confirmation (ADR-0045 — Re-checked at Task 1)

**NO sub-split.** This row is a SINGLE FLAT ROW with **6 tasks** (one completion task at the end for ADR-0268/BEHAVIOR_CONTRACT/ROADMAP docs + six-gate).

**Escape-valve status:** UNCONSUMED. The LoC budget is comfortable:
- ~120 LoC moved verbatim across two package extractions (`internal/filter/http/builtins` ~55 LoC + `internal/boot` ~55 LoC helpers and relocated types)
- ~50 LoC of genuinely new `validate` package code (`Bootstrap`/`BootstrapFile` + package-level doc + error handling)
- ~20 LoC of CLI flag wiring (`--mode validate` branch, `validate.Bootstrap` call, flag parsing)
- Task 4 construction-boundary test suite is ~120 LoC (table-driven unit tests, not counted toward escape valve as they are test code)

**Total production code: ~190 LoC, well under ADR-0045's gate (conservative ~300 LoC threshold for soft-split consideration).**

---

## Anticipated Exit Counts (Re-verified at Task 6)

| Metric | Baseline | Anticipated Exit | Delta |
|--------|----------|------------------|-------|
| Stat surface (H2 cluster) | 1200 | 1200 | +0 |
| Stat surface (non-H2) | 1196 | 1196 | +0 |
| Fixtures | 96 | 96 | +0 (no new differential fixture — this is a refactor + tooling layer, zero wire-surface) |
| Fuzzers | 52 | 52 | +0 (no new fuzzer — D-VALIDATE-FUZZER: construction-level code operates on already-`bootstrap.Load`-validated input) |
| BackendKind | 38 | 38 | +0 (no new filter/sink type) |
| DECISIONS tail | ADR-0267 | ADR-0268 | +1 (ADR-0268 anchored here; §Decision/§Consequences landed) |
| Next-free ADR | ADR-0268 | ADR-0269 | +1 |
| go.mod modules | clean | clean | +0 new modules (`go mod tidy -diff` empty) |
| Go packages (production) | N/A | N/A | +3 new packages (`internal/boot`, `internal/filter/http/builtins`, `validate`) |

---

## Key Design Decisions (Locked at PLAN time)

- **`internal/boot.Construct` does NOT build the cluster manager** (AMEND-VALIDATE-DEPGRAPH boundary): `cluster.NewManagerWithBaseDir` stays a call the CALLER makes itself, identically, in both `main.go` and `validate.Bootstrap`. Do NOT move `cluster.NewManagerWithBaseDir` into `Construct` — it would create a dependency cycle with `sinks`/`tracingProvider`, which the caller must build using the `cm`-derived dialer BEFORE `Construct` is callable.

- **HTTP filter construction defers boot-singleton injection to per-chain build time** (SPEC-51 §1.1 AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS): `internal/filter/http/builtins.RegisterBuiltins` takes NO `Deps` struct (unlike `internal/filter/network/builtins`). All 20 HTTP filter `Register` calls are bare constructor function references — NONE captures `cm`/`drainMgr`/`httpClient`/`tracingProvider` in a closure.

- **CLI uses `validate.Bootstrap`, not `validate.BootstrapFile`** (SPEC §3.5): the `--mode validate` handler in `main.go` opens the file itself and calls `validate.Bootstrap(f, filepath.Dir(*cfgPath), *allowH2C)` so `-allow-h2c` composes. Do NOT simplify this to `validate.BootstrapFile(*cfgPath)` — that silently drops `-allow-h2c`.

- **Exit codes** (pinned at SPEC): `0` valid / `1` invalid / `2` usage error.

- **Refactor regression discipline (LOAD-BEARING for Tasks 2-3):** a pure code-movement task is verified by the EXISTING test suite staying green, not by new assertions. Task 2 gates on the FULL `cmd/envoy-go` test suite (`go test ./cmd/envoy-go/... -count=1`) plus a `internal/filter/http/builtins`-local unit test. Task 3 gates on the FULL EXISTING differential suite (`go test ./test/differential/... -run TestDifferential -count=1`, all 96 fixtures) AND the FULL `cmd/envoy-go` test suite — byte-stability proof.

---

## Task 6 — Final Six-Gate Output (ANCHORS ADR-0268)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ gofmt -l . | grep -v '^vendor/' ; echo "gofmt: $?"
gofmt: 1   (grep found no lines — empty listing, i.e. gofmt clean)

$ golangci-lint run ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ go test ./... -count=1
ok   (all packages pass, including the full ./test/differential/... TestDifferential suite)

$ go test ./test/differential/... -run TestDifferential -race -count=1
ok   github.com/esalaine/envoy-go/test/differential   (all 96 fixtures pass under -race)

$ go mod tidy -diff
(no output — clean, EMPTY)

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
52

$ ls -d test/fixtures/*/ | wc -l
96
```

All eight commands GREEN / matching expectations: BUILD_OK; gofmt empty; golangci-lint clean; go vet clean; full `go test ./... -count=1` pass; full 96-fixture differential suite pass under `-race`; `go mod tidy -diff` EMPTY; fuzzer count 52 (UNCHANGED); fixture count 96 (UNCHANGED).

## Status: IMPL DONE (6/6)

Phase 51 (bootstrap-validate-mode) IMPL complete. `internal/boot.Construct`/`NewTracingProvider`, `internal/filter/http/builtins.RegisterBuiltins` (no `Deps`), the public `github.com/esalaine/envoy-go/validate` package, and the `--mode validate` CLI flag are all landed and verified against the final frozen HEAD. **ANCHORS ADR-0268** (§Decision/§Consequences landed in `docs/envoy-go/DECISIONS.md`). **ROADMAP row 51 (`bootstrap-validate-mode`) FLIPS `done`** — the SOLE leg (ADR-0106; NO parent rollup); the Operational tooling family STAYS OPEN. Counts UNCHANGED except THREE new Go packages + ADR-0267 → ADR-0268 (next-free ADR-0269).
