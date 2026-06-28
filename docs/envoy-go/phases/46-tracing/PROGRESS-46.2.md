# Phase 46.2 IMPL — Zipkin span exporter + B3 trace-context propagation (the SECOND `tracing.Exporter`)

FINAL chartered tracing leg; ANCHORS ADR-0261; flips ROADMAP row 46 → done

SPEC: `docs/envoy-go/phases/46-tracing/SPEC-46.2.md` | PLAN: `docs/envoy-go/phases/46-tracing/PLAN-46.2.md` | worktree branch: `phase-46.2-impl`

---

## ADR-0045 note: NO sub-split

This is the FINAL chartered tracing leg (per ADR-0106). It does NOT sub-split further — 46.2 is atomic: one exporter, one codec, one differential, one ADR. All 13 tasks land on this single branch.

---

## Baselines (verbatim)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
89

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
48

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38

$ grep -rn 'ExtractB3\|InjectB3\|ZipkinExporter\|tracing.zipkin\|zipkinTypeName' internal/ --include='*.go'
(no output)
```

Baseline: stat surface **1198** (H2 cluster; non-H2 **1194**) / fixtures **89** (tail `0087-tracing-otlp`) / fuzzers **48** / BackendKind **38** / DECISIONS tail **ADR-0260** (next-free **ADR-0261**).

---

## Task checklist

- [x] Task 1: PROGRESS scaffold + baselines
- [x] Task 2: B3 codec — `ExtractB3` + `InjectB3` + `FuzzExtractB3` (`b3.go`) [TDD]
- [x] Task 3: `DecideWithContext` — lift trace-context extraction to the caller (`decision.go`) [TDD]
- [x] Task 4: Zipkin v2 JSON span encoder + the `Authority` carry (`zipkin.go` + `span.go`) [TDD]
- [x] Task 5: Provider dispatch + the Zipkin `NewConfig` parse arm + strict-rejects (`config.go`) [TDD]
- [x] Task 6: Zipkin tracer-scoped counters `tracing.zipkin.{spans_sent,spans_dropped}` (+2 → 1200) [TDD]
- [x] Task 7: `ZipkinExporter` + the `ZipkinTransport` seam (`zipkin.go`) [TDD, full-package -race]
- [x] Task 8: `ExporterProvider` Zipkin arm + the boot-reject gate (`exporter.go`) [TDD]
- [x] Task 9: HCM provider-aware extract/inject + the `Authority` carry + byte-stability (`connection.go`/`h2dispatch.go`/`accesslog_emit.go`) [TDD]
- [x] Task 10: Boot wiring — hoist `httpClient`; thread the `ZipkinTransport` into `NewExporterProvider` (`main.go`)
- [x] Task 11: Driver-owned HTTP Zipkin collector (`test/helpers/zipkincollector`)
- [x] Task 12: `0088-tracing-zipkin` cross-side EXACT differential + subject unit tests
- [x] Task 13: Deliberate breaks + flake-soak + full-package -race + full 90-dir differential + six-gate + docs (ADR-0261, row 46 → done)

---

## Anticipated exit counts (re-verify at Task 13)

- **stat surface:** **1200** (+2 `tracing.zipkin.{spans_sent,spans_dropped}`; non-H2 **1196**; registered LAZILY only when a Zipkin exporter builds)
- **fixtures:** **90** (tail `0088-tracing-zipkin`)
- **fuzzers:** **49** (`FuzzExtractB3` added at Task 2; reconcile via `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 49` per `reference_fuzzer_count_docs_drift`)
- **BackendKind:** **38** (driver-owned `test/helpers/zipkincollector` HTTP/JSON receiver — NOT a new BackendKind; `reference_differential_grpc_receiver_driver_owned`)
- **DECISIONS:** **ADR-0261** (ANCHORED here; next-free **ADR-0262**)
- **0 new Go packages, 0 new go.mod modules** (`go mod tidy -diff` anticipated EMPTY — `ZipkinConfig` resolves at the already-direct `go-control-plane/envoy v1.32.4`; v2 JSON uses stdlib `encoding/json`)

---

## Verification (controller-run on the frozen HEAD)

**Six-gate — GREEN:**
- `gofmt -l .` → empty
- `golangci-lint run ./...` → clean
- `go vet ./...` → clean
- `go build ./...` → clean
- `go test ./... -count=1` → green (all unit packages)
- full **90-dir** differential `go test ./test/differential/ -count=1` → `ok ... 270.885s` (clean, no flake). *(An earlier full-`./...` run hit the KNOWN isolatable `subject ready: EOF` startup flake on the UNRELATED `0013-http-local-ratelimit`; isolate-re-run passed `ok 3.546s` → confirmed flake-not-regression per `reference_differential_fullsuite_startup_flake`.)*

**Full-package `-race`** (`internal/tracing` + `internal/filter/hcm`): both `ok` (~1.07s each).

**`0088-tracing-zipkin` deliberate breaks — all 4 LIVE** (each restored via `git restore` after; `-count=1` per `reference_differential_break_protocol_count1`):
- (a) span name → `"ingress"` ⇒ FAIL `subject span 0: name = "ingress", want "trace.example"`
- (b) drop B3 continuation ⇒ FAIL `subject continuation spans: got 0 with traceId=0123…, want 4`
- (c) kind → `"CLIENT"` ⇒ FAIL `kind = "CLIENT", want "SERVER"`
- (d) skip export ⇒ FAIL `Zipkin collector: timed out waiting for 12 spans (got 0)`

**Flake-soak:** `0088` 20/20 PASS.

**Correctness fix (post-Task-12).** `ExtractB3` now derives our server span's parent from the incoming SPAN-id (NOT the 4th `b3` field / `X-B3-ParentSpanId`), per SPEC §11 D-TRACE-ZIPKIN-B3 — the 4th field is accepted-but-ignored as the caller's grandparent.

**Count reconcile (re-verified, fast commands):**
- **stat surface 1200** (non-H2 **1196**) = 1198 + 2 (`tracing.zipkin.spans_sent` + `tracing.zipkin.spans_dropped`, lazy-registered on first Zipkin exporter build)
- **fixtures 90** — `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` → 90, tail `0088-tracing-zipkin`
- **fuzzers 49** — `grep -rh '^func Fuzz' --include='*.go' . | wc -l` → 49 (`FuzzExtractB3` present at `internal/tracing/b3_fuzz_test.go`); running total reconciled 48 → 49 per `reference_fuzzer_count_docs_drift`
- **BackendKind 38** — UNCHANGED (driver-owned `test/helpers/zipkincollector` HTTP/JSON receiver)
- **DECISIONS ADR-0261** — ANCHORED (next-free **ADR-0262**)
- `go mod tidy -diff` → EMPTY (0 new modules; 0 new Go packages)

**Docs landed (Task 13):** ADR-0261 §Decision + §Consequences body in `DECISIONS.md` (§Context promoted from SPEC-46.2 §13); the `### Request tracing — Zipkin tracing provider + B3 propagation` subsection in `BEHAVIOR_CONTRACT.md`; `STATE.md` active-phase → `phase 46.2 IMPL done`; **ROADMAP row 46 (`tracing`) FLIPPED `in-progress` → `done`** (46.1 + 46.2 COMPLETE — the Observability FAMILY STAYS OPEN).
