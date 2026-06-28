# Phase 46.1b IMPL — span emission + OTLP (OpenTelemetry) trace export

the COMPLETING sub-leg of the 46.1 (core+OTLP) by-exporter leg; CLOSES ADR-0260

SPEC: SPEC-46.1.md | PLAN: PLAN-46.1b.md | worktree branch: `phase-46.1b-span-otlp`

---

## Baselines (verbatim)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
88

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
48

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38

$ grep -rn 'OTLPTracesClient\|coltracepb\|spans_sent' internal/ --include='*.go'
(no output)
```

---

## Task checklist

- [x] Task 1: PROGRESS scaffold + baselines
- [x] Task 2: OTLPTracesClient UNARY typed wrapper
- [x] Task 3: Span model + 16-attr roster + toProto
- [x] Task 4: OTLPExporter bounded-channel batching sink (full-pkg -race)
- [x] Task 5: tracer-scoped counters (+2 → 1198)
- [x] Task 6: ExporterProvider + boot-reject cluster gate
- [x] Task 7: thread ExporterProvider into HCM Filter
- [x] Task 8: span-end wiring (carry Decision to emit seam, H1+H2)
- [x] Task 9: boot wiring in main.go
- [x] Task 10: driver-owned test/helpers/otlptrace receiver
- [x] Task 11: 0087-tracing-otlp cross-side differential
- [x] Task 12: breaks + flake + six-gate + ADR-0260 body + docs (CLOSES the leg)

---

## Exit counts (re-verified at Task 12)

```
$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
89

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
48

$ grep -n 'H2GoawayResponder BackendKind' test/differential/fixture/fixture.go
606:	H2GoawayResponder BackendKind = 38

$ go mod tidy -diff && echo TIDY_EMPTY
TIDY_EMPTY
```

- **stat surface:** **1198** (+2 `tracing.opentelemetry.{spans_sent,spans_dropped}`, registered LAZILY only when an exporter builds; non-H2 **1194**)
- **fixtures:** **89** (tail `0087-tracing-otlp`)
- **fuzzers:** **48** (UNCHANGED — D-TRACE-FUZZER: no new fuzzer; `^func Fuzz` == 48 verified per `reference_fuzzer_count_docs_drift`)
- **BackendKind:** **38** (driver-owned `test/helpers/otlptrace` receiver, not a new BackendKind)
- **DECISIONS:** **ADR-0260** (CLOSED; next-free **ADR-0261**)
- **0 new Go packages, 0 new go.mod modules** (`go mod tidy -diff` EMPTY — OTLP trace protos resolve at the already-direct `go.opentelemetry.io/proto/otlp v1.0.0`)

## Deliberate-break results (Task 12; all with `-count=1`)

- **(a) span Name → name-assert FAIL:** changed `BuildServerSpan` name `"ingress"` to `"injected_wrong"` → `0087` name assertion FAILS (subject emits wrong name; differential detects)
- **(b) zeroed ParentSpanID → continuation parent_span_id FAIL:** zeroed the `ParentSpanID` field in `toProto()` → `0087` continuation prong `parent_span_id` assertion FAILS (continuation span has no parent; differential detects)
- **(c) skipped emit-seam Export → 0-spans poll-timeout FAIL:** removed `f.exporter.Submit(span)` call in `accesslog_emit.go` → `0087` poll-to-converge on `spans_sent` times out (subject never exports; differential detects)
- **(d) neutralized `spansSent.Add` → subject `spans_sent` FAIL:** replaced `spansSent.Add(int64(len(buf)))` with a no-op → `0087` subject-side `spans_sent >= N` assertion FAILS (counter stays 0)
- **(e) not-sampled negative (unit tests — differential cannot prove under `random_sampling=100`):** `TestSpanEmit_*_NotSampledNoExport` unit tests verify that not-sampled requests produce no `Submit` call and no exported spans; confirmed GREEN

## Flake gate

`0087-tracing-otlp` differential: **20/20 GREEN** (no transient failures across 20 runs with `-count=1`). The full 89-dir differential was also confirmed GREEN by the controller in parallel.

## Six-gate (controller-confirmed GREEN)

1. `gofmt -l .` = 0 (no drift)
2. `go vet ./...` clean
3. `go build ./...` OK
4. `golangci-lint run ./...` clean
5. `go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1` clean (the exporter writer goroutine + ticker is a background mutator — `reference_full_suite_race_after_background_mutator`; full packages required, not a subset)
6. `go mod tidy -diff` EMPTY

## ADR status

**ADR-0260 CLOSED** at this IMPL. §Decision/§Consequences body in `docs/envoy-go/DECISIONS.md` (tail advances ADR-0259 → **ADR-0260**; next-free **ADR-0261**, the 46.2 Zipkin+B3 anchor).
