# Phase 46.1a Implementation Progress — the header-level request-tracing engine

**Phase:** 46.1a (IMPL) — the FIRST sub-leg of the 46.1 (core+OTLP) by-exporter split

**SPEC reference:** [SPEC-46.1.md](../SPEC-46.1.md)

**Sub-leg designation:** 46.1a = the header-level tracing engine (internal/tracing package + HCM parse arm + 5 tracing.* counters + dispatch wiring + 0086 differential); 46.1b = span emission + OTLP export (ADR-0260 closes at 46.1b IMPL)

**Worktree branch:** `phase-46.1a-tracing-header-engine`

---

## Task Checklist (12 tasks)

- [ ] **Task 1:** PROGRESS scaffold + baselines + the D-TRACE-SPLIT re-check
- [ ] **Task 2:** internal/tracing skeleton — TracingConfig + RandSource seam
- [ ] **Task 3:** x-request-id generate/preserve/byte-14 stamp + FuzzStampRequestID
- [ ] **Task 4:** W3C traceparent extract/inject + FuzzExtractTraceparent
- [ ] **Task 5:** proto→TracingConfig parse + 8 STRICT-REJECT arms (tracing.NewConfig)
- [ ] **Task 6:** Decide sampling precedence + classification + decision engine + config/trace/v3 blank-import
- [ ] **Task 7:** RegisterHCMCounters + Record + 5 counter registration in parseFilterWithCtx
- [ ] **Task 8:** HCM tracing parse wiring (config.go parseFilterWithCtx + the config field)
- [ ] **Task 9:** HCM dispatch wiring — decision call + header stamp + counter increment (H1 + H2 seams)
- [ ] **Task 10:** 0086-tracing-request-id differential fixture + driver + assertions
- [ ] **Task 11:** deliberate breaks + flake isolation + `-race` full-package gate
- [ ] **Task 12:** full 88-fixture suite + six-gate verification + BEHAVIOR/STATE/ROADMAP docs update

---

## Baseline Counts (recorded at Task 1)

### Build sanity check
```
go build ./... && echo BUILD_OK
```
Output:
```
BUILD_OK
```

### Fixture count (expect 87 — tail 0085-otlp-access-log-operators)
```
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
```
Output:
```
87
```

### Fuzzer count (expect 46)
```
grep -rh '^func Fuzz' --include='*.go' . | wc -l
```
Output:
```
46
```

### BackendKind enum tail (expect H2GoawayResponder = 38)
```
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
```
Output:
```
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38
```

### Tracing production code check (expect: router.go:763 no-inject comment + extauthz x-request-id reader; NO production tracing engine + NO hcm.GetTracing() call)
```
grep -rn 'GetTracing\|traceparent\|x-request-id' internal/ --include='*.go' | grep -v _test
```
Output:
```
internal/filter/http/router/router.go:763:// The router does NOT inject x-envoy-*, x-forwarded-*, or x-request-id
internal/filter/http/extauthz/attributes.go:465://     pseudo-headers + HCM-injected x-forwarded-proto / x-request-id /
internal/filter/http/extauthz/extauthz.go:106:	// requestID is the value of the x-request-id header (HCM-injected per §11.P4).
internal/filter/http/extauthz/extauthz.go:1015:	//   - the canonical x-request-id from the incoming client headers;
internal/filter/http/extauthz/extauthz.go:1033:	authReq.requestID = headers.Get("x-request-id")
```

**Summary:** ✓ All baseline counts match expected values. No production tracing engine yet; the references are only documentation comments + the ext_authz reader (unrelated to the tracing engine).

---

## Anticipated Exit Counts (at Task 12 completion)

- **Stat surface:** 1196 (+5 from baseline 1191) — the five `http.<stat_prefix>.tracing.{client_enabled,health_check,not_traceable,random_sampling,service_forced}` HCM-scoped counters (registered only when a tracing HCM is configured)
- **Fixtures:** 88 (+1 from baseline 87) — the new `0086-tracing-request-id` differential fixture
- **Fuzzers:** 48 (+2 from baseline 46) — `FuzzStampRequestID` (Task 3) + `FuzzExtractTraceparent` (Task 4)
- **BackendKind enum:** 38 (UNCHANGED) — no new BackendKind; the differential reuses `HTTPHeaderMutation` (Kind 9)
- **DECISIONS record:** ADR-0259 (UNCHANGED; ADR-0260 body lands at 46.1b IMPL) — the D-TRACE-SPLIT resolution, D-TRACE-CONFIG-HOME, D-TRACE-RNG-SEAM, D-TRACE-RECEIVER-WIRING, D-TRACE-STATS-FINAL (46.1a slice), D-TRACE-FUZZER, D-TRACE-SPANEND-PLUMBING (deferred), D-TRACE-SPAWN-UPSTREAM-SPAN all live in SPEC-46.1.md §12; the engine closure + the resource/attribute definitions close ADR-0260 at 46.1b IMPL.
- **New go.mod modules:** 0 — the trace config protos resolve at the already-present `go-control-plane/envoy` module; `go mod tidy -diff` anticipated EMPTY.

---

## D-TRACE-SPLIT Re-Check (the 46.1a/46.1b sub-leg boundary record)

### Full-leg LoC estimate (~840 production LoC)
The as-built components:
- `internal/tracing` decision.go + propagation.go + requestid.go + rand.go + stats.go + config.go ≈ 290 LoC
- `internal/filter/hcm/config.go` tracing parse arm + blank-import ≈ 120 LoC
- `internal/filter/hcm/connection.go` dispatch wiring (H1 + H2) ≈ 70 LoC
- **46.1a subtotal: ~480 LoC** (below 505 estimate; the estimate was conservative)
- THEN (46.1b, deferred):
  - `span.go` (span model + 16-attr roster) ≈ 120 LoC
  - `grpcclient.go OTLPTracesClient` (UNARY typed wrapper) ≈ 57 LoC
  - `OTLPExporter` (buffered writer-goroutine) ≈ 180 LoC
  - dispatch carry-forward (Decision to accesslog_emit.go) ≈ 30 LoC
  - tracer-scoped counter registration ≈ 15 LoC
  - bootstrap/cluster-exists gate ≈ 10 LoC
  - **46.1b subtotal: ~412 LoC**
- **Grand total: ~892 LoC** (over the 840 estimate by ~60 LoC from added carrier wiring — acceptable variance)

### The 46.1a/46.1b boundary (the header-propagation cut)
46.1a contains ALL header-level logic — the header is the OBSERVABLE boundary for 46.1a (what the upstream receives + what the HCM counters record). 46.1b begins AFTER the headers are forwarded; it owns the span-lifecycle (the span model + the span attributes + the exporter).

| Component | 46.1a | 46.1b | Notes |
|-----------|-------|-------|-------|
| `internal/tracing/decision.go` | X | — | sampling decision + classification |
| `internal/tracing/propagation.go` | X | — | traceparent extract/inject + tracestate pass-through |
| `internal/tracing/requestid.go` | X | — | x-request-id generate/stamp |
| `internal/tracing/rand.go` | X | — | RandSource seam (production + test) |
| `internal/tracing/stats.go` | X | — | HCMCounters registration + Record |
| `internal/tracing/config.go` | X | — | TracingConfig struct + proto→config parse |
| `internal/tracing/span.go` | — | X | span model + 16-attr roster |
| Dispatch seam (connection.go) | X (decision call) | — (at 46.1a) | carry-forward to accesslog_emit.go is 46.1b |
| HCM parse arm (config.go) | X | — | TracingConfig build + counter registration + RNG setup |
| 5 tracing.* counters | X | — | client_enabled, health_check, not_traceable, random_sampling, service_forced |
| 2 tracer-scoped counters | — | X | spans_sent, spans_dropped (at 46.1b) |
| OTLPTracesClient | — | X | grpcclient.go wrapper |
| OTLPExporter | — | X | buffered writer-goroutine over TraceService.Export |
| config/trace/v3 blank-import | X | — | ADR-0016, lands in bootstrap for protojson registry |
| 0086 differential (header-echo) | X | — | cross-side header forwarding + subject-side counter assertions |
| 0087 differential (span-export) | — | X | cross-side span payload + subject-side exporter gauges |

### ADR-0260 closure timing
- **SPEC-46.1 §12 D-TRACE-SPLIT decision** pinned the boundary + the sub-split "present open questions as pinned" — all D-TRACE-* decisions live in the SPEC, CLOSED FOR 46.1a.
- **ADR-0260 body** (the engine design + the decision precedence + the resource/attribute definitions) is WRITTEN during 46.1a IMPL (visible in SPEC-46.1.md §11) but the ADR itself is formally CLOSED (status → `Decided`) at the **46.1b IMPL** — the completion task (Task 12 → docs update) records the closure.
- The 46.1a sandbox still runs without the exporter, proving the header logic in isolation (the differential asserts the headers, not the spans).

---

## Tasks 2–10 — BUILT (each committed, controller-verified)

- T2 internal/tracing skeleton (TracingConfig + RandSource/CryptoRand) — `a857900f`
- T3 x-request-id generate/preserve/byte-14 stamp + FuzzStampRequestID — `5e815a51`
- T4 W3C traceparent extract/inject + tracestate + FuzzExtractTraceparent — `f8edea68`
- T5 tracing.NewConfig proto→config + 8 STRICT-REJECT arms — `b87b031f`
- T6 tracing.Decide full sampling precedence + config/trace/v3 blank-import — `e46cd873`
- T7 RegisterHCMCounters (5 HCM-scoped tracing.* counters) + Record — `d0b00bcd`
- T8 HCM parse wiring (lift hcm.GetTracing(); no-tracing ⇒ no counters byte-stable) — `1d1323ed`
- T9 HCM dispatch wiring (Decide + x-request-id + traceparent inject + counter, H1+H2) — `c9b462c6`
- T10 0086-tracing-request-id differential (cross-side header-echo) — `75938b51`

Code-quality review (cumulative engine+integration, master..c9b462c6): **APPROVED, no Critical/Important issues** (the two highest-risk seams — the Decide float-ordering and the H2 value-copy re-set — confirmed correct + test-proven; race-free by construction). 5 Minor items noted (uppercase-hex leniency, fakeRand exhaustion default, upsertH2Header backing-array assumption, H2-preserve test gap, the client-force/overall-cap counter-vs-nibble model) — none blocking; the two reference-assumption items (#1, #4) deliberately NOT folded absent a live probe.

## Task 11 — VERIFICATION (controller-run; no production change committed)

**Deliberate-break proofs (each `-count=1`, broke → confirmed 0086 FAIL → `git restore`):**
- (a) neutralize the x-request-id index-14 nibble stamp ⇒ FAIL `subject plain[0]: x-request-id index-14 nibble = "4", want "9"` ✓ live
- (b) force Decide to ignore the incoming traceparent ⇒ FAIL on the continuation trace-id invariant ✓ live
- (c) skip InjectTraceparent at the H1 dispatch seam ⇒ FAIL `subject plain[0]: traceparent "" does not match 00-<32hex>-<16hex>-01` ✓ live
- (d) skip the random_sampling counter Inc ⇒ FAIL `subject http.hcm_local.tracing.random_sampling: got 0, want 8` ✓ live
- (e) byte-stability: covered by the Task-9 unit tests `TestDispatchRequest_NoTracing_ByteStable` / `TestWriteH2_NoTracing_ByteStable` — the `if f.tracingConfig != nil` guard is structurally load-bearing (the no-tracing path nil-panics without it).

**Flake gate:** 20/20 PASS on `go test ./test/differential/ -run 'TestDifferential/0086' -count=1`.
**Full-package -race:** `go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1` ⇒ clean (both packages).

### Cross-side departure found at T10 (documented, per-side pinned) — SUPERSEDED by Task 10b
Under `random_sampling:100`, the reference stamps the CONTINUATION x-request-id nibble `9` (its reason reflects the LOCAL sampling decision, independent of the inbound traceparent), while envoy-go marks a continued trace `NoTrace`/`4` (per the SPEC §11 "continued keeps 4" pin). The 0086 driver pins this PER-SIDE (reference `9`, subject `4`) — both load-bearing — and keeps the continuation trace-id cross-side EXACT. This contradicts the SPEC §11 continuation-nibble pin against the live reference; flagged for the 46.1b / final review (the §11 probe likely observed continuation under non-100 sampling). **→ RESOLVED at Task 10b (below): envoy-go now MATCHES the reference; the departure no longer exists.**

### Task 10b — SPEC §11 D-TRACE-REQUESTID / AMEND-TRACE-REQUESTID-NIBBLE CORRECTION (empirically re-probed at the 46.1a IMPL via 0086)
A CONTINUED-and-SAMPLED trace stamps the x-request-id nibble `'9'` (Sampled), NOT `'4'` (NoTrace) as the 2026-06-27 SPEC probe claimed. envoy-go matches the reference (continued nibble = the inbound sampled bit: Sampled⇒`9`, not-sampled⇒`4`); the COUNTER class stays `not_traceable`. The 0086 differential asserts the continuation nibble `'9'` CROSS-SIDE EXACT. User-decided 2026-06-28.

Changes: `internal/tracing/decision.go` (continued branch sets `Reason = Sampled` when `ic.Sampled`, class UNCHANGED) + `decision_test.go` (continued cases ⇒ `Sampled`/`9`; continued-not-sampled UNCHANGED `NoTrace`) + `internal/filter/hcm/connection_test.go` (the H1+H2 continued dispatch tests ⇒ nibble `9`, `not_traceable` counter still `1`) + the 0086 driver/README/expectations (continuation nibble now cross-side EXACT `9`; per-side `4` expectation removed) + the SPEC §11 row + AMEND bullet annotated `[CORRECTED 46.1a IMPL …]`.

## Status Summary

- **Tasks 1–11 done + verified.** Next: Task 12 (full 88-dir + six-gate + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile; row 46 STAYS in-progress; ADR-0260 body deferred to 46.1b).
