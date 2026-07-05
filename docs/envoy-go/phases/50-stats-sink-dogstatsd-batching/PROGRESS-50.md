# Phase 50 Implementation Progress — `dog_statsd max_bytes_per_datagram` real multi-metric datagram batching

**Status:** IMPL DONE (8 / 8) — 2026-07-05. ROADMAP row 50 FLIPS `done`. ANCHORS ADR-0267 (§Decision/§Consequences landed IN-PLACE).

**Reference:** [PLAN-50.md](./PLAN-50.md) · [SPEC-50.md](./SPEC-50.md)

**Worktree branch:** `phase-50-stats-sink-dogstatsd-batching`

**Description:** The SEVENTH Observability-family row — transport-layer-only batching of consecutive formatted DogStatsd lines over the LANDED phase-49 DogStatsdSink. Honors an EXPLICIT bootstrap `max_bytes_per_datagram` field by buffering multiple metrics into single UDP datagrams until the next line would exceed the cap (boundary operator: STRICT `>` — a buffer landing EXACTLY at the cap still fits, live-proven). Absent or explicit `0` continues to emit one line per datagram, byte-identical to phase 49. **ANCHORS ADR-0267.** Row 50 flips **`done`** at this IMPL six-gate.

---

## Task Checklist

- [x] **Task 1:** Phase scaffolding — PROGRESS-50.md + baselines + the final ADR-0045 split re-check (D-DSDB-SPLIT)
- [x] **Task 2:** Lift the `max_bytes_per_datagram` strict-reject — `DogStatsdSinkConfig.MaxBytesPerDatagram uint64` + the `parseDogStatsdSinkConfig` edit (`internal/bootstrap/bootstrap.go`)
- [x] **Task 3:** The `appendLine`/`flush` buffer-accumulate-then-flush-on-overflow rewrite of `DogStatsdSink.Submit` (`internal/statssink/dogstatsd.go`)
- [x] **Task 4:** Boot wiring — thread `MaxBytesPerDatagram` into the `NewDogStatsdSink` call (`cmd/envoy-go/main.go`)
- [x] **Task 5:** `test/helpers/statsdrecv` additive accessors — `MaxLinesInAnyDatagram()` + `LinesInDatagram(name)`
- [x] **Task 6:** The `0094-stats-sink-dogstatsd-batching` differential fixture (driver + YAMLs + expectations + README)
- [x] **Task 7:** The +0 stat-surface guard (D-DSDB-STATS-FINAL) + the full differential + the six-gate
- [x] **Task 8:** ADR-0267 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + the fuzzer-count reconcile

---

## Baseline Counts (Task 1 — Recorded at Session Start)

### Command Output

**Build:**
```
BUILD_OK
```

**Fixture count:**
```
95
```

**Fuzzer count:**
```
52
```

**BackendKind tail (H2GoawayResponder):**
```
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38
```

**go mod tidy -diff (expect empty):**
```
(no output — clean)
```

**NewDogStatsdSink call sites (expect exactly 13: 11 + 1 + 1, all TWO-arg):**
```
internal/statssink/registration_test.go:87:	sink, err := NewDogStatsdSink("127.0.0.1:65535", "envoy")
internal/statssink/dogstatsd_test.go:21:	s, err := NewDogStatsdSink(addr, "dsdpfx")
internal/statssink/dogstatsd_test.go:43:	s, err := NewDogStatsdSink(addr, "dsdpfx")
internal/statssink/dogstatsd_test.go:68:	s, err := NewDogStatsdSink(addr, "dsdpfx")
internal/statssink/dogstatsd_test.go:88:	s, err := NewDogStatsdSink(addr, "p")
internal/statssink/dogstatsd_test.go:130:	dsd, err := NewDogStatsdSink(dsdAddr, "p")
internal/statssink/dogstatsd_test.go:178:	s, err := NewDogStatsdSink(addr, "p")
internal/statssink/dogstatsd_test.go:209:	s, err := NewDogStatsdSink(addr, "p")
internal/statssink/dogstatsd_test.go:230:	s, err := NewDogStatsdSink(addr, "envoy")
internal/statssink/dogstatsd_test.go:249:	s, err := NewDogStatsdSink(addr, "p")
internal/statssink/dogstatsd_test.go:266:	s, err := NewDogStatsdSink(addr, "p")
internal/statssink/dogstatsd_test.go:282:	s, err := NewDogStatsdSink("not a valid addr", "p")
cmd/envoy-go/main.go:222:			sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix)
```

**Count: 13 call sites (11 in dogstatsd_test.go + 1 in registration_test.go + 1 in main.go), all TWO-arg. ✓**

### Baseline Summary

| Metric | Baseline |
|--------|----------|
| Build | OK |
| Fixtures | 95 |
| Fuzzers | 52 |
| BackendKind tail | 38 (H2GoawayResponder) |
| Stat surface (H2 cluster) | 1200 |
| Stat surface (non-H2) | 1196 |
| DECISIONS tail | ADR-0266 |
| Next-free ADR | ADR-0267 |
| NewDogStatsdSink call sites | 13 (all TWO-arg) |
| go.mod state | Clean (tidy -diff empty) |

---

## D-DSDB-SPLIT Confirmation (ADR-0045 — Re-checked at Task 1)

**NO sub-split.** This row is a SINGLE FLAT ROW with **8 tasks** (one fewer than phase 49's 9, since no new fuzzer task and no new bootstrap-dispatch-arm task needed — we edit an EXISTING arm's body).

**Escape-valve status:** UNCONSUMED (the LoC budget is comfortable: ~80–150 prod LoC: the `appendLine`/`flush` rewrite ~30 LoC; the config field + parse-arm edit ~5 LoC; the `main.go` third-argument thread ~1 LoC; the `statsdrecv` accessor pair ~15–20 LoC; the driver ~700 LoC but almost entirely a clone of the landed `0093` driver — well under ADR-0045's gate).

---

## Final Exit Counts (Task 8 — re-verified fresh at close, not copied from Task 1)

| Metric | Baseline | Exit (re-verified) | Delta |
|--------|----------|------|-------|
| Stat surface (H2 cluster) | 1200 | 1200 | +0 |
| Stat surface (non-H2) | 1196 | 1196 | +0 |
| Fixtures | 95 | **96** | +1 (0094-stats-sink-dogstatsd-batching) |
| Fuzzers | 52 | **52** | +0 (UNCHANGED — D-DSDB-FUZZER-SEED resolved: existing `FuzzDogStatsdSinkConfigParse` seed already covers the field) |
| BackendKind | 38 | **38** | +0 (extended UDP receiver is driver-owned, NOT a new BackendKind) |
| DECISIONS tail | ADR-0266 | **ADR-0267** | +1 (ADR-0267 anchored, §Decision/§Consequences landed IN-PLACE at this IMPL; next-free ADR-0268) |
| go.mod modules | clean | **clean** | +0 new modules (`go mod tidy -diff` re-run, empty) |
| go packages | N/A | N/A | +0 new packages (all edits in existing packages) |

### Re-verification command output (fresh at Task 8, per `reference_fuzzer_count_docs_drift` discipline — never copy a prior task's numbers verbatim)

**Build:**
```
$ go build ./...
BUILD_OK
```

**Fixture count:**
```
$ ls -d test/fixtures/*/ | wc -l
96
```

**Fuzzer count:**
```
$ grep -rh '^func Fuzz' --include='*.go' . | wc -l
52
```

**BackendKind tail (unchanged, re-confirmed):**
```
$ grep -rn "BackendKind = 38" --include="*.go" .
test/differential/fixture/fixture.go:606:	H2GoawayResponder BackendKind = 38
```

**go mod tidy -diff (expect empty):**
```
$ go mod tidy -diff
(no output — clean)
```

**DECISIONS tail:**
```
$ grep -n "^## ADR-" docs/envoy-go/DECISIONS.md | tail -1
## ADR-0267 — `dog_statsd max_bytes_per_datagram` real multi-metric datagram batching ...
```

All FIVE re-verified numbers match the anticipated exit counts exactly (96 fixtures / 52 fuzzers / 38 BackendKind / clean go.mod / ADR-0267 tail) — no drift.

### Six-gate final verification (Task 8, on the frozen HEAD)

- `go build ./...` — PASS
- `go test ./... -count=1` — PASS (full suite; `test/differential` 96-dir run included; two transient unrelated flakes observed and isolate-re-run clean — see task-8-report.md for detail)
- `go vet ./...` — clean (implied by prior task gates; re-confirmed no new findings in touched packages)
- `gofmt -l` — clean on all touched packages
- `golangci-lint` — clean on all touched packages (per Task 1-7 per-task discipline)
- `0094` deliberate breaks (a)/(b) LIVE + 20/20 flake-soak + full-package `-race` on `internal/statssink` + `internal/stats` — all GREEN (verified at Task 7; re-confirmed unaffected by Task 8's docs-only changes)

**Row 50 FLIPS `done`. ANCHORS ADR-0267. Phase 50 (dog_statsd max_bytes_per_datagram batching) COMPLETE.**

---

## Key Design Decisions (Locked at PLAN time)

- **Boundary operator (LOAD-BEARING, live-proven):** the overflow comparison is `prospective > cap` (STRICT) — a buffer landing EXACTLY at the cap after appending STILL FITS. Do NOT implement `>=`.
- **Oversized single-line handling:** a single line whose own formatted length exceeds the cap is sent alone in its own oversized datagram — **no error, no drop, no truncation, and NO special-cased branch** (it falls out of the SAME general algorithm).
- **Absent-cap handling:** an ABSENT field or an explicit `0` degenerates to exactly one line per datagram, byte-identical to phase-49 behavior — **NO special case needed.**
- **Buffer type:** `strings.Builder` (matches `formatTagSuffix`'s EXISTING use for stylistic consistency within the same file).
- **No parser change to `statsdrecv`:** the datagram-level `\n`-split at `statsdrecv.go:131` already exists; the new accessors are purely observational (last-seen-per-name + server-wide max).
- **Driver design for `0094`:** a deliberately LONG `backendName` (~160 chars) forces cluster-tagged lines past the cap alone; short `statPrefix` (`"hcm_local"`, the `0093` value) keeps HCM-tagged lines small enough to co-batch.

