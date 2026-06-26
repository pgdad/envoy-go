# Phase 44.3 (grpc-access-log header-capture) IMPL — PROGRESS

**SPEC:** `SPEC-44.3.md`
**PLAN:** `PLAN-44.3.md`
**Worktree branch:** `phase-44.3-header-capture-impl`

44.3 is the **FINAL chartered leg** of the by-concern 3-leg Observability-family-opener
split (44.1 core / 44.2 buffering / 44.3 header-capture). The 44.3 IMPL six-gate flips
**ROW 44 → `done`**; the **Observability FAMILY STAYS OPEN** (OTLP / tracing / stats-sinks
/ tap remain future Observability rows).

This leg makes `additional_{request,response}_headers_to_log` LIVE — the fields are
PARSE-ACCEPT-but-INERT through 44.1/44.2 (ADR-0255 §Consequences / ADR-0256 §Consequences).

---

## Task Checklist

- [x] T1  PROGRESS scaffold + baselines + the final ADR-0045 split re-check (D-HDR-SPLIT-FINAL)
- [x] T2  ALSConfig header-name parse arm (lowercased) + fuzzer seed corpus [TDD] — `b94a14bf`
- [x] T3  Record two-map extension + exported `CaptureHeaders` helper [TDD] — `7772b2b8`
- [x] T4  emit-hook capture (H1+H2) + response-header threading + Filter union [TDD; no-capture byte-stable] — `f6ad5e60`
- [x] T5  `buildHTTPAccessLogEntry` per-sink filter + `NewGrpcAccessLogSink` signature + `CaptureRequest/ResponseHeaderNames` [TDD, full-package -race] — `1d7f0568`
- [x] T6  `main.go` boot wiring pass-through — absorbed into T5 (`1d7f0568`)
- [x] T7  `0083-grpc-access-log-headers` differential fixture (fixtures 84→85) — `4c539a28`
- [x] T8  `0083` deliberate-break proofs + 20/20 flake + full-package -race
- [x] T9  full 85-dir differential + six-gate + ADR-0257 + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 44 → done; Observability family STAYS OPEN) + fuzzer reconcile

---

## Baseline Counts (recorded at T1)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
84

$ grep -rc '^func Fuzz' --include='*.go' . | awk -F: '{s+=$2} END{print s}'
44

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38
```

Baseline summary:
- stat surface: **1189** (H2 cluster; non-H2 **1185**)
- fixtures: **84** (incl letter-suffixed `0007a`/`0007b`; tail `0082-grpc-access-log-buffering`)
- fuzzers: **44**
- BackendKind tail: **38** (`H2GoawayResponder`)
- DECISIONS tail: **ADR-0256** (next-free **ADR-0257** — appears only as forward references in DECISIONS.md, no ADR-0257 section yet)

NOTE: `grep -cE '^[0-9]{4}-'` UNDERCOUNTS the fixtures by 2 — the glob form
`ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` is authoritative (= 84).

---

## Anticipated EXIT Counts

| Metric | Baseline | Exit | Delta | Note |
|---|---|---|---|---|
| stat surface | 1189 | 1189 | 0 | UNCHANGED — NO new header-capture stat (AMEND-HDR-5) |
| fixtures | 84 | 85 | +1 | `0083-grpc-access-log-headers` |
| fuzzers | 44 | 44 | 0 | UNCHANGED — existing `FuzzParseHttpGrpcAccessLogConfig` covers the header fields (T2 adds seed corpus only) |
| BackendKind tail | 38 | 38 | 0 | the receiver is driver-owned, REUSES `HTTPFixedBody` — NOT a new BackendKind |
| DECISIONS tail | ADR-0256 | ADR-0257 | +1 | gRPC ALS header-capture ADR |

---

## D-HDR-SPLIT-FINAL Re-check (ADR-0045 soft gate)

Estimated production LoC breakdown (~135 prod LoC):

| Component | Est. LoC |
|---|---|
| two `ALSConfig` fields + parse-arm reads/lowercase | ~18 |
| two `Record` maps + the `captureHeaders` helper | ~38 |
| emit-hook capture H1+H2 + response-param threading through ~12 call sites | ~42 |
| `buildHTTPAccessLogEntry` filter + `NewGrpcAccessLogSink`/`newGrpcSinkWithCapacity` signature + two `headerCaptureSink` methods + `parseFilterWithCtx` union derivation | ~35 |
| `main.go` pass-through | ~2 |
| **Total** | **~135** |

~135 prod LoC — sits **well under the ADR-0045 soft gate**. **44.3 ships as ONE leg**
(the FINAL chartered leg of the by-concern 3-leg split). The 44.3 IMPL six-gate flips
**ROW 44 → `done`**; the **Observability FAMILY STAYS OPEN**. (Bookkeeping re-check only;
no code change.)

---

## Task 2 — ALSConfig header-name parse arm + fuzzer seed corpus

Status: DONE (commit `b94a14bf`).

- `ALSConfig` gained `RequestHeaderNames` / `ResponseHeaderNames` `[]string` fields; the
  bootstrap parse arm reads them from the OUTER `HttpGrpcAccessLogConfig`
  (`GetAdditional{Request,Response}HeadersToLog()`) and lowercases each name once at
  parse time via `lowerAll` (AMEND-HDR-1; empty/nil input stays nil — byte-stable
  no-capture path). The fields remained PARSE-ACCEPT-but-INERT through 44.1/44.2 and
  become live here.
- Header-bearing seed corpus added to `FuzzParseHttpGrpcAccessLogConfig`
  (D-HDR-FUZZER-CORPUS); the existing fuzzer covers the new fields, so the fuzzer
  count stays **44** (no new `^func Fuzz`).

---

## Task 3 — Record two-map extension + exported CaptureHeaders helper

Status: DONE (commit `7772b2b8`).

- `accesslog.Record` extended with `RequestHeaders` / `ResponseHeaders`
  `map[string]string` fields (both nil on the no-capture path — byte-stable).
- New shared exported helper `accesslog.CaptureHeaders(names, lookup)`
  (`internal/accesslog/capture.go`) implementing the three AMENDs: lowercase keys
  (caller passes already-lowercased names — AMEND-HDR-1), comma-join of multi-value
  headers with `strings.Join(vals, ",")` (no space — AMEND-HDR-3), and omit-absent
  (presence, not value-emptiness, is the discriminator — AMEND-HDR-2). Returns nil
  for an empty name list.

---

## Task 4 — emit-hook capture (H1+H2) + response-header threading + Filter union

Status: DONE (commit `f6ad5e60`).

- `captureRecordHeaders` on the HCM `Filter` populates `rec.RequestHeaders` /
  `rec.ResponseHeaders` from the configured capture UNION via per-flavor lookups
  (`reqHeaderLookupH1` over `http.Header.Values`; `reqHeaderLookupH2` /
  `respHeaderLookup` scan the ordered slices case-insensitively, COLLECTING all
  matching values in wire order). Maps are allocated ONLY when the corresponding
  union is non-empty (D-HDR-RECORD-CAPTURE-SCOPE), so the no-capture path stays
  byte-stable.
- Response headers threaded through the ~12 `emitAccessLog{,H2}` call sites
  (D-HDR-RESPONSE-THREADING); a nil response carrier (error sites with no response)
  yields a nil resp-lookup → response capture skipped, `ResponseHeaders` nil.
- The capture union (`alsReqHeaderNames` / `alsRespHeaderNames`) lives on the Filter,
  derived at parse time as the union of all gRPC-ALS sinks' configured names.

---

## Task 5 — buildHTTPAccessLogEntry per-sink filter + NewGrpcAccessLogSink signature

Status: DONE (commit `1d7f0568`).

- `buildHTTPAccessLogEntry` gained `reqHdrNames`/`respHdrNames` params and emits
  `Request.RequestHeaders` / `Response.ResponseHeaders` proto maps via the new
  `filterCaptured(captured, names)` helper, which copies the sink's OWN configured
  names out of the emit-hook UNION (D-HDR-SINK-FILTER — keeps multi-sink fan-out
  per-sink correct) and returns nil when empty/nothing-matched (byte-identical to the
  no-capture path).
- `NewGrpcAccessLogSink` signature extended with `CaptureRequestHeaderNames` /
  `CaptureResponseHeaderNames` (the sink's own configured names).
- Full-package `-race` clean (the sink writer + 44.2 buffering ticker goroutines).

---

## Task 6 — main.go boot wiring pass-through

Status: DONE — absorbed into T5 (commit `1d7f0568`). The boot path passes the parsed
per-sink capture names through `NewGrpcAccessLogSink` and the union to the Filter; no
separate commit was required.

---

## Task 7 — 0083-grpc-access-log-headers differential fixture

Status: DONE.

### Step 1 — backend-origin response header VERIFIED (D-HDR-DIFFERENTIAL-DRIVE / AMEND-HDR-4)

- `fixture.HTTPFixedBody` ⇒ `test/fixtures/0006-access-log/backends/main.go`
  (the runner's `startHTTPFixedBodyBackend` spawns this binary). Its handler sets
  `w.Header().Set("Content-Type", "text/plain")` before the 17-byte fixed body.
- envoy-go surfaces it at the emit site: `connection.go:715`
  `f.emitAccessLog(req, status, bytesSent, picked, startTime, resp.Headers)` —
  `resp.Headers` is the post-encode-chain surfaced upstream response (the same
  bytes wire-written downstream), so the backend-origin `content-type` is present.
- **VERIFIED captured response header: `content-type: text/plain`** (confirmed
  live on BOTH sides by the test dump below — `resp=map[content-type:text/plain]`).
- Parse confirms the two header lists are read from the OUTER
  `HttpGrpcAccessLogConfig` (`bootstrap.go:409-410`
  `cfg.GetAdditional{Request,Response}HeadersToLog()`), NOT `common_config`.

### Authored

- `envoy.yaml` / `envoy-go.yaml`: copied 0081; `log_name "0083"`; added
  `additional_request_headers_to_log: ["x-req-foo","x-req-missing","x-req-multi"]`
  + `additional_response_headers_to_log: ["content-type"]` on the OUTER message
  (alongside `common_config`).
- `driver/driver.go`: copied 0081's `alsDriver`; `fixtureName =
  "0083-grpc-access-log-headers"`; `refListenerPort = 10083` (next-free 0083
  analog, confirmed unused); `numRequests = 8` kept. `fireProbe` adds
  `X-Req-Foo: bar` + two-valued `X-Req-Multi: m1,m2` (X-Req-Missing NOT set);
  `assertEntries` extends the verbatim 7-field subset with
  `request.request_headers` DeepEqual `{x-req-foo:bar, x-req-multi:m1,m2}` +
  explicit `x-req-missing` absent + `response.response_headers` DeepEqual
  `{content-type:text/plain}`. `FIXTURE_0083_DUMP` env-gated per-entry map dump.
- `expectations.yaml` / `README.md`: rewritten for the header-capture purpose,
  drive, AMEND-HDR-4 backend-origin note, host-reachability table.
- `runner_test.go`: blank-import of the 0083 driver package added after 0082.

### Run (correct selector) — PASS

`FIXTURE_0083_DUMP=1 go test ./test/differential/ -run 'TestDifferential/0083' -count=1 -v`
⇒ `--- PASS: TestDifferential/0083-grpc-access-log-headers (10.57s)`.

Non-vacuity proof (both sides, all 8 entries):
```
ref  entry N: req=map[x-req-foo:bar x-req-multi:m1,m2] resp=map[content-type:text/plain]
subj entry N: req=map[x-req-foo:bar x-req-multi:m1,m2] resp=map[content-type:text/plain]
```
Maps non-empty on BOTH sides ⇒ the DeepEqual assertions are live.

- **x-req-multi cross-side join HELD** (`m1,m2` identical both sides) — the
  CONTINGENCY (drop x-req-multi) was NOT applied.
- x-req-missing OMITTED both sides; subject `logs_written == 8`.
- Fixture count: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **85**.

### Gates

- `gofmt -l test/` ⇒ clean (after `gofmt -w` on the new driver).
- `golangci-lint run ./test/...` ⇒ exit 0.
- `go build ./...` ⇒ clean.

---

## Task 8 — 0083 deliberate-break proofs + flake gate + full-package -race

Status: DONE. VERIFICATION-ONLY task — no production change committed (each break was
reverted with `git restore` before the next). Docker reachable (reference Envoy
contrib-v1.37.2). Every break-run and the `-race` run used `-count=1` (go-test caching
footgun, reference_differential_break_protocol_count1) with the selector
`-run 'TestDifferential/0083'`. Baseline: PASS (4.3s).

### Step 1 — deliberate-break liveness proofs (all 5 BIT)

Each break: edit ONE production line → run 0083 → confirm FAIL → `git restore` →
re-run → confirm PASS. The asserting line is `runner_test.go:1281` (the driver's
per-entry `reflect.DeepEqual` of the captured maps + the 0081 7-field subset, run on
BOTH sides for all 8 entries).

**(a) capture bites (load-bearing 44.3 break)** — `accesslog_emit.go`
`captureRecordHeaders`: inserted an early `return` (no-op, leaves both maps nil).
⇒ FAIL:
```
subject entry 0: request.request_headers = map[], want map[x-req-foo:bar x-req-multi:m1,m2]
```
Restore → PASS.

**(b) lowercase-key bite** — `bootstrap.go` `lowerAll`: replaced
`out[i] = strings.ToLower(s)` with `out[i] = s` (skip the lowercasing; kept the
`strings` import referenced so the package still compiled).
- Path used: **the mixed-case-config path** (the PLAN's documented fallback). With the
  ORIGINAL all-lowercase config names, skipping ToLower did NOT bite (PASS) — the
  config names (`x-req-foo`/`x-req-missing`/`x-req-multi`) are already lowercase, so
  ToLower is a no-op on them. This is EXPECTED (not a vacuity finding).
- To prove the lowercasing is load-bearing, TEMPORARILY changed the request config
  name `x-req-foo` → `X-Req-Foo` in BOTH `envoy.yaml` and `envoy-go.yaml` (this break
  only), with `lowerAll` still broken. ⇒ FAIL:
```
subject entry 0: request.request_headers = map[X-Req-Foo:bar x-req-multi:m1,m2], want map[x-req-foo:bar x-req-multi:m1,m2]
```
  (subject preserves the mixed-case wire/config name; reference lowercases ⇒ key
  mismatch). Restore `bootstrap.go` + BOTH yamls → PASS.

**(c) comma-join bite** — `capture.go` `CaptureHeaders`: changed
`strings.Join(vals, ",")` → `strings.Join(vals, ";")`. ⇒ FAIL:
```
subject entry 0: request.request_headers = map[x-req-foo:bar x-req-multi:m1;m2], want map[x-req-foo:bar x-req-multi:m1,m2]
```
(`x-req-multi` becomes `m1;m2` on the subject). Restore → PASS.

**(d) omit-absent bite** — `capture.go` `CaptureHeaders`: added an
`else { out[name] = "" }` so absent names are stored as empty instead of omitted.
⇒ FAIL:
```
subject entry 0: request.request_headers = map[x-req-foo:bar x-req-missing: x-req-multi:m1,m2], want map[x-req-foo:bar x-req-multi:m1,m2]
```
(the configured-but-absent `x-req-missing` appears as `{"x-req-missing":""}` on the
subject). Restore → PASS.

**(e) aggregated 7-field payload still bites** — `mapping.go`
`buildHTTPAccessLogEntry`: set `UserAgent: ""` (dropped `rec.UserAgent`). ⇒ FAIL:
```
subject entry 0: request.user_agent = "", want "als-probe/1"
```
(proves the 44.1 7-field subset is still live alongside the new header maps). Restore
→ PASS.

ALL FIVE breaks bit. No vacuous assertion found.

### Step 2 — flake gate (20/20 PASS)

```
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0083' -count=1; done
```
⇒ **20/20 PASS** (no `subject ready: EOF` startup-race flake).

### Step 3 — full `internal/accesslog` package -race

```
go test ./internal/accesslog/ -race -count=1
```
⇒ `ok  github.com/esalaine/envoy-go/internal/accesslog  1.077s` — **PASS, no race**
(the sink writer goroutine + the 44.2 buffering ticker goroutine are clean under
-race).

### Clean-tree confirmation

After all five restores, `git status --short` showed the working tree clean (no
production file modified) before this PROGRESS commit.

---

## Task 9 — full 85-dir differential + six-gate + docs completion

Status: DONE. Docker reachable (reference Envoy `contrib-v1.37.2`). The house completion
gate run from the worktree root; ALL SIX gates GREEN.

### Step 1 — the six-gate (all GREEN)

| Gate | Command | Result |
|---|---|---|
| gofmt | `gofmt -l . \| wc -l` | **0** |
| lint | `golangci-lint run ./...` | exit **0** (clean) |
| vet | `go vet ./...` | exit **0** |
| build | `go build ./...` | exit **0** |
| full suite | `go test ./... -count=1` | exit **0** — `ok test/differential 258.241s`; ZERO `FAIL` across the whole tree |
| race | `go test ./internal/accesslog/ -race -count=1` | `ok ... 1.077s` (no race) |
| go.mod | `go mod tidy -diff` | EMPTY (exit 0) |

**Byte-stability confirmation (the regression anchor).** The full **85-dir differential**
passed `-count=1` with ZERO `FAIL`. `0081`/`0082` (which configure NO additional headers
⇒ empty capture union ⇒ nil `Record` maps ⇒ byte-identical emitted protos) STAY green; the
82 prior non-ALS fixtures byte-identical (no fixture moved) — the no-capture path is
byte-stable as designed (D-HDR-RECORD-CAPTURE-SCOPE). `0083-grpc-access-log-headers` passed
cross-side EXACT. NO startup-race (`subject ready: EOF`) flake required isolate-re-running.

### Step 2 — exit counts (each VERIFIED)

| Metric | Value | Verification |
|---|---|---|
| stat surface | **1189** (non-H2 **1185**) | UNCHANGED — NO new header-capture stat (AMEND-HDR-5) |
| fixtures | **85** | `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* \| wc -l` == 85 (tail `0083-grpc-access-log-headers`) |
| fuzzers | **44** | `grep -rc '^func Fuzz' --include='*.go' . \| awk -F: '{s+=$2} END{print s}'` == 44 |
| BackendKind tail | **38** | `H2GoawayResponder = 38` (driver-owned receiver REUSES `HTTPFixedBody`) |
| DECISIONS tail | **ADR-0257** | next-free ADR-0258 |
| go.mod modules | UNCHANGED | `go mod tidy -diff` EMPTY |
| internal packages | UNCHANGED | `capture.go` lives in the existing `internal/accesslog` |

### Steps 3-5 — docs landed

- **ADR-0257** (`DECISIONS.md`): the full entry — header paragraph + §Context (promoted
  from SPEC-44.3 §13, PROPOSED → ACCEPTED) + §Decision (the `ALSConfig` header-name
  parse arm / the `Record` two-map + EXPORTED `CaptureHeaders` helper / the emit-hook
  capture + response-threading + capture-union on the Filter / the per-sink
  `buildHTTPAccessLogEntry` filter + `NewGrpcAccessLogSink` signature / the `0083`
  differential) + §Consequences (NEITHER supersedes; NO new stat/BackendKind/fuzzer/
  package/module; no-capture path byte-stable; `additional_*_trailers_to_log` +
  `AccessLog.filter` STAY inert; AMEND-HDR-4 backend-origin response constraint; row 44
  → `done`, the Observability family STAYS OPEN). DECISIONS tail ADR-0256 → ADR-0257.
- **BEHAVIOR_CONTRACT.md**: the `### Access log — gRPC ALS streaming sink` block gained
  the `**The header capture (44.3, ADR-0257)**` paragraph (AMEND-HDR-1..5);
  `additional_{request,response}_headers_to_log` MOVED from the PARSE-ACCEPT-but-INERT /
  "Does not yet apply to" lists into the supported set (a new `### Applies to` 44.3
  bullet) — leaving `additional_*_trailers_to_log` + `AccessLog.filter` deferred. The
  stat-surface block STAYS 1189.
- **STATE.md**: new active-phase `phase 44.3 (grpc-access-log) IMPL done` (row 44 → done,
  family STAYS OPEN; counts 1189 / 85 / 44 / 38 / ADR-0257; NEXT → the next
  Observability-family row [OTLP access logger BRAINSTORM]); the prior PLAN-done bullet
  demoted to `prior active-phase`.
- **ROADMAP.md**: row 44 status `in-progress` → **done** + the 44.3 IMPL-done note
  appended; the `### Observability family` section updated (`gRPC ALS` DONE; the family
  STAYS OPEN — OTLP / tracing / stats sinks / tap remain).
- **Fuzzer reconcile (D-HDR-FUZZER / `reference_fuzzer_count_docs_drift`):** `^func Fuzz`
  == **44** (UNCHANGED). All live/current count references across STATE/BEHAVIOR_CONTRACT/
  ROADMAP/DECISIONS read 44; the only `fuzzers 43` hits are historical `43 → 44` deltas
  in the 44.1-era blocks (era-correct, NOT a stale running total). No drift to fix.

### Path-targeting note (`feedback_subagent_worktree_path_targeting`)

The four shared docs (DECISIONS/BEHAVIOR_CONTRACT/ROADMAP/STATE) were first edited via an
absolute path that resolved to the MAIN checkout (`/home/esa/git/envoy-go/docs/...`) rather
than this worktree. Caught at the PROGRESS edit; the four-file diff was patched into the
worktree (the docs are identical between `master` and the branch HEAD, so the patch applied
cleanly) and the main checkout was `git restore`d clean. All committed docs live in THIS
worktree.
