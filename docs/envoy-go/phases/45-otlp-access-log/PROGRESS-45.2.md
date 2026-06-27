# Phase 45.2 (otlp-access-log operator engine) IMPL — PROGRESS

**SPEC:** `SPEC-45.2.md`
**PLAN:** `PLAN-45.2.md`
**Worktree branch:** `phase-45.2-otlp-operator-engine`

45.2 is the **FINAL leg** of the OTLP access-log row (the second Observability-family row,
after the row-44 gRPC ALS opener). It lifts the OTLP access logger's
`body`/`attributes`/`resource_attributes` config from the 45.1 **STRICT-REJECT** to
**LIVE command-operator-templated content**, served by a NEW small `internal/accesslog`
operator engine (`otlpformat.go`) curated to the `Record`-mapped subset (STRICT-REJECT
operators outside it — the envoy-go-strict mirror of the reference's own unknown-operator
boot-reject).

45.2 ships as **ONE leg** (the operator engine — the FINAL chartered leg of the 2-leg OTLP
split). Its final IMPL task flips **ROADMAP row 45 → `done`** (per-leg completion, ADR-0106
+ `reference_roadmap_split_phase_row_done`). The Observability **FAMILY STAYS OPEN**
(tracing / stats-sinks / tap remain future rows).

---

## Task Checklist

- [x] T1  PROGRESS-45.2.md scaffold + baselines + the final ADR-0045 split re-check (D-OTLP-2-SPLIT-FINAL)
- [x] T2  the operator engine `internal/accesslog/otlpformat.go` (registry + `CompileOTLPTemplate`/`CompileOTLPValue`/`ValidateOTLPValue` + `Eval`)
- [x] T3  the operator-engine fuzzer `FuzzCompileOTLPValue` (45 → 46)
- [x] T4  the config-parse LIFT (`internal/bootstrap`) — remove the reject arms, compile-at-boot, `OTLPConfig.{Body,Attributes,ResourceAttributes}`
- [x] T5  the `buildLogRecord`/`buildResource`/`buildExportRequest` extensions
- [x] T6  the sink template threading + the grown `NewOTLPAccessLogSink` signature
- [x] T7  the main wiring (`cmd/envoy-go/main.go`)
- [x] T8  the `0085-otlp-access-log-operators` differential + subject unit tests
- [x] T9  `0085` deliberate breaks + 20/20 flake + full-package -race
- [x] T10 full 87-dir + six-gate + ADR-0259 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 45 → done) + fuzzer-count reconcile

---

## Baseline Counts (recorded at T1, verbatim)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
86

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | tail -1
test/fixtures/0084-otlp-access-log

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
45

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38

$ grep -rn 'internal/bootstrap' internal/accesslog/ || echo NO_CYCLE
NO_CYCLE

$ go mod tidy -diff && echo TIDY_CLEAN
TIDY_CLEAN
```

Baseline summary:
- stat surface: **1191** (H2 cluster; non-H2 **1187**)
- fixtures: **86** (incl letter-suffixed `0007a`/`0007b`; tail `0084-otlp-access-log`)
- fuzzers: **45**
- BackendKind tail: **38** (`H2GoawayResponder`)
- `internal/bootstrap` reference inside `internal/accesslog/`: **NO_CYCLE** (D-OTLP-2-COMPILE-SITE:
  the bootstrap→accesslog dependency stays acyclic — bootstrap compiles templates at parse via
  the exported `internal/accesslog` engine; no reverse import)
- `go mod tidy -diff`: **TIDY_CLEAN** (no go.mod delta anticipated this phase — the operator
  engine is pure-Go over the already-direct `go.opentelemetry.io/proto/otlp`)
- DECISIONS tail: **ADR-0258** (next-free **ADR-0259**; ADR-0259 appears in DECISIONS.md only as
  forward references — no ADR-0259 section yet)

All six baseline probes match the PLAN-anticipated values EXACTLY (86 / 45 / 38 / NO_CYCLE /
TIDY_CLEAN / ADR-0258). No baseline mismatch.

NOTE: the glob form `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` is authoritative
(= 86); a `grep -cE '^[0-9]{4}-'` form UNDERCOUNTS by the letter-suffixed dirs.

---

## Anticipated EXIT Counts

| Metric | Baseline | Exit | Delta | Note |
|---|---|---|---|---|
| stat surface | 1191 | 1191 | 0 | UNCHANGED — the operator engine adds NO stat (non-H2 stays 1187) |
| fixtures | 86 | 87 | +1 | `0085-otlp-access-log-operators` |
| fuzzers | 45 | 46 | +1 | `FuzzCompileOTLPValue` |
| BackendKind tail | 38 | 38 | 0 | UNCHANGED — the OTLP logs receiver is driver-owned (`test/helpers/otlplogs`) |
| DECISIONS tail | ADR-0258 | ADR-0259 | +1 | the OTLP operator-engine ADR |
| go.mod modules | — | +0 | 0 | 0 new go.mod modules (pure-Go engine over the already-direct otlp proto) |

---

## D-OTLP-2-SPLIT-FINAL Re-check (ADR-0045 soft gate)

Estimated production LoC breakdown (≈270 prod LoC):

| Component | Est. LoC |
|---|---|
| `otlpformat.go` — total | **≈175** |
| &nbsp;&nbsp;• registry | ≈20 |
| &nbsp;&nbsp;• `CompileOTLPTemplate` scanner | ≈45 |
| &nbsp;&nbsp;• `CompileOTLPValue` walker | ≈40 |
| &nbsp;&nbsp;• `ValidateOTLPValue` | ≈20 |
| &nbsp;&nbsp;• `Eval` | ≈35 |
| &nbsp;&nbsp;• the 3 types (`OTLPTemplate`/`OTLPValueTemplate`/`OTLPAttrTemplate`) | ≈15 |
| the bootstrap lift (remove reject arms + compile-at-boot + `OTLPConfig.{Body,Attributes,ResourceAttributes}`) | ≈45 |
| the `buildLogRecord`/`buildResource`/`buildExportRequest` extensions | ≈30 |
| the sink + `NewOTLPAccessLogSink` signature threading | ≈15 |
| `main.go` wiring | ≈5 |
| **Total** | **≈270** |

≈270 prod LoC — sits **at the ADR-0045 soft gate**. **45.2 ships as ONE leg** (the operator
engine — the FINAL chartered leg of the 2-leg OTLP split). The 45.2 IMPL six-gate flips
ROADMAP **row 45 → `done`** (per-leg, ADR-0106 + `reference_roadmap_split_phase_row_done`,
row-36/row-39 precedent); the Observability **FAMILY STAYS OPEN**. (Bookkeeping re-check
only; no code change.)

---

## T9 — `0085` deliberate-break proofs + flake gate + full-package `-race`

VERIFICATION-ONLY (no production change survives — every break is `git restore`-reverted,
working tree confirmed clean after each). Every `go test` used `-count=1` (defeats the
stale-PASS caching bug) and the `-run 'TestDifferential/0085'` selector (the bare-`0085`
form matches zero subtests → vacuous green). Reference Envoy: `contrib-v1.37.2` (Docker).
Baseline (pre-break) run: PASS.

### Step 1 — Deliberate-break proofs (4 LIVE assertions)

Each: break ONE production line → 0085 FAILS → `git restore` → `git status` clean → re-run PASS.

| # | Production break | Failing assertion message | Restore → re-run |
|---|---|---|---|
| (a) | `internal/accesslog/otlpformat.go` — `"REQ(:METHOD)"` extractor `return r.Method` → `return "X"` | `subject record 0: body "X /health HTTP/1.1 200 17" does not match "^GET /health HTTP/1\.1 200 \d+$"` | clean → PASS |
| (b) | `internal/accesslog/otlpformat.go` — `CompileOTLPValue` `*AnyValue_KvlistValue` arm dropped the child recursion (returns an empty-kvlist template) | `subject record 0: nested.inner_code = "", want "200"` | clean → PASS |
| (c) | `internal/accesslog/otlpmapping.go` — `buildResource` dropped `attrs = append(attrs, resourceAttrs...)` | `subject ResourceLogs 0: service_name = "", want "envoy-go-test"` | clean → PASS |
| (d) | `internal/bootstrap/bootstrap.go` — resource_attributes path made to operator-SUBSTITUTE (`CompileOTLPValue` + `vt.Eval(&accesslog.Record{Authority: "otlp.example"})` replacing the literal `kv.Value`) instead of the verbatim `ValidateOTLPValue` pass-through | `subject ResourceLogs 0: authority_literal = "otlp.example", want "%REQ(:AUTHORITY)%" (literal, NOT substituted)` | clean → PASS |

Break (d) approach note (AMEND-OPS-1 liveness): a one-line break is not faithful here —
the literal pass-through is the ABSENCE of substitution, so to prove the assertion bites we
inject substitution at the bootstrap parse (compile each resource_attributes value, then
`Eval` it against a synthetic `Record{Authority: "otlp.example"}`, replacing the stored
literal `kv.Value`). The driver expectation was NOT touched. With substitution active
`authority_literal` becomes `"otlp.example"` and the LITERAL-pass-through assertion FAILS;
restored → PASS. All 4 assertions are LIVE — no fixture hole.

### Step 2 — Flake gate

20 consecutive `-count=1` runs of `-run 'TestDifferential/0085'`: **20/20 PASS** (no
`subject ready: EOF` startup-race, no assertion flake).

### Step 3 — Full `internal/accesslog` package `-race`

`go test ./internal/accesslog/ -race -count=1`:

```
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.107s
```

PASS, no race (full package — the writer goroutine is a background mutator that a `-run`
subset would miss).

Post-task `git status`: only this PROGRESS doc modified (no leftover break). Branch
`phase-45.2-otlp-operator-engine` intact.

---

## Task 10 — full 87-dir six-gate + docs (2026-06-27)

### The six-gate (verbatim)

```
$ gofmt -l . | wc -l
0

$ golangci-lint run ./...
(clean — no output)

$ go vet ./...
(clean)

$ go build ./...
(ok)

$ go test ./... -count=1
… (all unit + conformance packages ok) …
FAIL	github.com/esalaine/envoy-go/test/differential	264.316s   # RUN 1
--- FAIL: TestDifferential/0024-http-oauth2 (0.97s)
    runner_test.go:1062: subj drive: setup oauthbackend: listen 127.0.0.1:40869: bind: address already in use

# RUN 2 (clean re-attempt):
FAIL	github.com/esalaine/envoy-go/test/differential	260.813s
--- FAIL: TestDifferential/0012-http-header-mutation (3.51s)
    runner_test.go:1147: subj start (attempt 3): subject ready: EOF

$ go test ./internal/accesslog/ -race -count=1
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.107s

$ go mod tidy -diff && echo TIDY_CLEAN
TIDY_CLEAN
```

### Startup-race-flake disposition (`reference_differential_fullsuite_startup_flake`)

Both full-suite differential FAILs are the documented UNRELATED startup-race-flake class — RUN 1 hit a port-bind race on `0024-http-oauth2` (`bind: address already in use`), RUN 2 hit a `subject ready: EOF` on `0012-http-header-mutation` (a DIFFERENT unrelated fixture, after 3 fresh-port retries). NEITHER touches OTLP. Isolate-re-run confirmed each + the OTLP fixtures GREEN:

```
$ go test ./test/differential/ -run 'TestDifferential/0024-http-oauth2' -count=1
ok  	github.com/esalaine/envoy-go/test/differential	1.124s

$ go test ./test/differential/ -run 'TestDifferential/(0012-http-header-mutation|0084-otlp-access-log|0085-otlp-access-log-operators)' -count=1 -v
--- PASS: TestDifferential/0012-http-header-mutation
--- PASS: TestDifferential/0084-otlp-access-log
--- PASS: TestDifferential/0085-otlp-access-log-operators
ok  	github.com/esalaine/envoy-go/test/differential	9.861s
```

⇒ the only full-suite failures are transient unrelated startup races (both isolate-re-ran GREEN); the OTLP byte-stability anchor (`0084`) + the new `0085` operator differential both PASS; no non-OTLP fixture has a real assertion divergence. The gate is GREEN modulo the flake class.

### Counts (each VERIFIED empirically)

- stat surface **1191** UNCHANGED (non-H2 **1187**) — the operator engine adds no stat.
- fixtures **87** (`ls -d test/fixtures/*/ | wc -l` == 87; tail `0085-otlp-access-log-operators`).
- fuzzers **46** (`grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 46; the new `FuzzCompileOTLPValue`).
- BackendKind tail **38** UNCHANGED (driver-owned receiver).
- DECISIONS tail **ADR-0258 → ADR-0259** (next-free **ADR-0260**).
- ZERO new packages + ZERO new go.mod modules (`go mod tidy -diff` EMPTY).

### Docs landed

ADR-0259 §Decision/§Consequences (PROPOSED → ACCEPTED) in `DECISIONS.md`; the `### Access log — OpenTelemetry (OTLP) access-log sink` BEHAVIOR_CONTRACT subsection extended (operator engine + lifted rejects + curated-set departure; stat surface UNCHANGED 1191); STATE active-phase → `phase 45.2 … IMPL done`; ROADMAP row 45 (`otlp-access-log`) FLIPS `in-progress` → `done` (the FINAL leg; the Observability FAMILY STAYS OPEN); fuzzer running total reconciled 45 → 46. T10 ✓.
