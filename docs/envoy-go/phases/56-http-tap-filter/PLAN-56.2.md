# Phase 56.2 Implementation Plan — the HTTP tap filter, bodies leg: body capture on the landed 56.1 spine (`DecodeData`/`EncodeData` accumulation into `HttpBufferedTrace_Message.body`, the per-direction `max_buffered_rx_bytes`/`max_buffered_tx_bytes` caps, `Body.truncated`, the now-DIVERGENT `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` render) + the `0100-http-tap-bodies` POST-echo differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller verifies each commit, re-runs gates on the frozen HEAD, performs deliberate-break/liveness verification ITSELF, and squashes + pushes at stage-close (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`).

**Goal:** Make the tap filter's two currently-inert data hooks (`DecodeData`/`EncodeData`, `tap.go:60-65` — `return DataContinue` pass-throughs) BUFFER request/response body bytes into the pending trace, honoring a per-direction byte cap; populate `data/tap/v3.HttpBufferedTrace_Message.body` (a `data/tap/v3.Body`) at stream end on a match; and render it per the sink's `format` — `JSON_BODY_AS_STRING` → `{"as_string": "<utf8>"}`, `JSON_BODY_AS_BYTES` → `{"as_bytes": "<base64>"}` — with `truncated` always emitted. Proven cross-side against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227) by the new `0100-http-tap-bodies` POST-echo differential (full / at-cap-boundary / truncated arms) plus in-package unit tests for the AS_BYTES render, multi-chunk accumulation, and the non-UTF8 sanitize boundary. It flips ROADMAP row 56 `done` at its six-gate.

**Architecture:** Body capture is a per-stream accumulation on the EXISTING `*tapFilter` value — no new type, no new interface, no new seam, no new package, no new go.mod module, no new stat, no new BackendKind, no new fuzzer. The one shared `*tapFilter` (installed in both `HTTPFilter.Decoder`/`.Encoder`, `config.go:38-46`) already owns `reqHdrs`/`respHdrs`/`sawReq`/`sawResp`; 56.2 adds four body fields (`reqBody`/`reqTrunc`/`respBody`/`respTrunc`) + two saw-body flags alongside, populates `format`+caps on the parsed `config`, and assembles a `Body` in `buildTrace`. The predicate engine, sink, `:status` synthesis, reject roster, `rq_tapped` counter, and emit lifecycle are ALL UNCHANGED from 56.1 (ADR-0273). The ONLY production package touched is `internal/filter/http/tap`. Byte-identical when no tap filter is configured. ANCHORS ADR-0274; the Observability family STAYS OPEN (its deferred-candidate list is non-empty).

**Tech Stack:** Go; the landed HTTP-filter data-hook seam (`StreamDecoderFilter.DecodeData` / `StreamEncoderFilter.EncodeData`, `internal/filter/http/types.go:56`/`:67`); `google.golang.org/protobuf/encoding/protojson` (the byte-exact renderer, unchanged marshal opts); the resolved `go-control-plane` tap protos (`data/tap/v3.Body`, `config/tap/v3.OutputConfig`); `strings.ToValidUTF8` (the AS_STRING sanitize); the Docker-bridge differential harness (`HTTPEchoBody` backend, REUSE). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Global Constraints

Every task's requirements implicitly include this section. Values are copied verbatim from SPEC-56.2 (re-derived against source at PLAN time — see the citation-drift corrections in "Key source seams").

- **The 56.1 spine is LANDED and FROZEN.** No change to the predicate engine (`internal/matchpredicate`), `internal/headermatch`, the emit lifecycle (`OnDestroy` resolve-then-emit, `trace.go:47-66`), the ONE-SHARED-VALUE constraint, the `:status` synthesis on a COPY (`tap.go:47-57`), the header rendering (`trace.go:21-35`), the `filePerTapSink` (`sink.go`), the `rq_tapped` counter, the `builtins.go`/`boot.go` wiring, or the full §6 reject roster. **56.2 is purely additive within `parseConfig`/`DecodeData`/`EncodeData`/`buildTrace`.**
- **Framework-zero-touch (PRODUCTION).** No change to any landed filter, to the HTTP-filter dispatch layer, to the callback surface, or to the trailer seams. The `test/differential` harness already carries `HostMount{Dir: true}` (56.1's D-TAP-DIRMOUNT) and `HTTPEchoBody` (BackendKind 6) — REUSE both; **no harness surgery this leg.**
- **The pinned marshal is UNCHANGED, byte-exact:** `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitDefaultValues: true}` + a trailing `"\n"` (`sink.go:26-31`). `EmitDefaultValues` renders `"truncated": false` on a full body — reference-identical (AMEND-TAP-BODY2-TRUNCFLAG). It also renders `"raw_value": ""`/`"trailers": []` while OMITTING nil message fields (so a never-captured `body` is absent). Do NOT switch to `EmitUnpopulated` (it would emit `"body": null`).
- **The cap default is 1024 when the `*wrapperspb.UInt32Value` wrapper is NIL** (AMEND-TAP-BODY2-DEFAULTCAP). A PRESENT wrapper uses `GetValue()` **including 0**. `nil → 1024` and `present-0 → 0` are DISTINCT and both are load-bearing (a present-0 cap yields an empty-but-truncated body, NOT a 1024-byte capture).
- **Truncation is strict `>`, never `>=`** (AMEND-TAP-BODY2-BOUNDARY): a body EXACTLY at the cap is NOT truncated. The comparison is `capturedLen + len(chunk) > cap`. This is the single most break-prone off-by-one of the leg.
- **Tap is a READ-ONLY OBSERVER.** `DecodeData`/`EncodeData` return `DataContinue` ALWAYS — NEVER `DataStopIterationAndBuffer`, NEVER `DataStopIterationNoBuffer`, NEVER `OverwriteBody`. Tap must not perturb the body flowing to the upstream/downstream. (Contrast the `buffer` filter, which returns `DataStopIterationNoBuffer` on overflow — a shape tap must NOT copy.)
- **Distinguish "hook fired, buffer empty" from "hook never fired."** A `sawReqBody`/`sawRespBody` bool is set on the FIRST `DecodeData`/`EncodeData` invocation (mirrors the landed `sawReq`/`sawResp`, `tap.go:25-26`). `Body` is populated IFF the saw-body flag is set — a cap-0-nonempty stream emits `{as_string: "", truncated: true}` (PRESENT), whereas a genuinely bodyless stream emits NO `body` (the hook never fired). A `len(buf)>0` gate would be WRONG (it drops the cap-0 case).
- **The AS_STRING non-UTF8 sanitize (D-TAP-BODY-UTF8, pinned below):** `Body_AsString` is built from `strings.ToValidUTF8(string(buf), "�")`, NOT raw `string(buf)`. Go's `protojson.Marshal` ERRORS on invalid UTF-8 in a proto3 `string` field, and the sink swallows a marshal error (`trace.go:65` `_ = f.cfg.sink.write(...)`) → a raw non-UTF8 AS_STRING body would silently drop the WHOLE trace. AS_BYTES is unaffected (base64 of arbitrary bytes always marshals). A documented DEPARTURE from the reference's specific lossy C++ mangling; lossless-of-the-trace and deterministic.
- **Per-task gates (every code task):** `gofmt -l` (must print nothing) + `golangci-lint run` on touched packages + `go vet ./...` + `go build ./...` (`feedback_pertask_gofmt_lint`).
- **COMMIT BEFORE YOU BREAK.** `git restore <file>` reverts to HEAD, not to "before the break". Finish → gate → commit → only then break, restore, re-run with `-count=1`.
- **Deliberate-break runs ALWAYS use `-count=1`** (`reference_differential_break_protocol_count1`) and the FULL subtest selector `-run 'TestDifferential/0100-http-tap-bodies'` (`reference_differential_run_selector` — a bare `-run '0100'` matches ZERO subtests and reports a vacuous PASS). **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — a break aborting earlier can MASK the intended one.
- **`t.Errorf` per independent property; `t.Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`). Never `t.Fatalf` mid-list — later properties become unreachable dead code.
- **Counts at IMPL exit:** stat surface **1201 (+0)**, fixtures **101 → 102** (`0100-http-tap-bodies`), fuzzers **53 (+0)**, BackendKind tail **38 (+0)**, DECISIONS tail **ADR-0273 → ADR-0274** (next-free ADR-0275), new Go packages **0**, new go.mod modules **0**.

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The 56.1 headers leg (landed at `e954961f`, ADR-0273) shipped the tap filter: a per-stream observer that watches a request/response pair, compiles a `MatchPredicate` into a tri-state tree, evaluates it over request+response HEADERS, and — at stream end, on a match — writes a byte-exact protojson `TraceWrapper` (an `HttpBufferedTrace`) to a per-stream `file_per_tap` file. 56.1 was deliberately body-free: it drove a `GET → 204` stream, so a zero-length body OMITTED the `body` field and the two accepted JSON formats (`AS_STRING`/`AS_BYTES`) were indistinguishable.

**56.2 adds ONE concern — body capture — to that proven spine.** Four things make it worth its own leg, and all four are load-bearing:

1. **Bodies arrive in CHUNKS on the decode side, but ONE call on the encode side.** On the H1 decode path the request body is streamed to the filter chain in ≤32 KiB chunks (`connection.go:632` — `buf := make([]byte, 32*1024)`, a `for` loop calling `chain.RunDecodeData(ctx, buf[:n], endStreamOnData)`), with a synthetic `RunDecodeData(ctx, nil, true)` at end (`connection.go:653`). The encode path (H1 `connection.go:743`, H2 `h2dispatch.go:582`) and the H2 decode path (`h2dispatch.go:539-540`) each deliver the whole body in exactly ONE `endStream=true` call. **Tap must SELF-ACCUMULATE `DecodeData` chunks into its own `[]byte` field across calls** — like the landed `buffer` filter (`buffer/buffer.go:222-243`: a filter-owned field updated every call). It must NOT assume a single whole-body call, and must NOT rely on a callback buffer accessor (`DecoderFilterCallbacks` has no `BufferDecodedBody`; only `EncoderFilterCallbacks.BufferEncodedBody()` exists — but tap keeps its own buffer in BOTH directions for uniformity).

2. **A body EXACTLY at the cap is NOT truncated.** Truncation is strict `>` (measured: cap 10 on a 10-byte body → `truncated: false`; cap 9 → `truncated: true`). The comparison is `capturedLen + len(chunk) > cap`. Get this `>=`-vs-`>` off-by-one wrong and a differential arm at body-length == cap flips its `truncated` flag — which is precisely what the differential's **boundary arm** and break (c) exist to catch.

3. **An empty-but-truncated body is DISTINCT from a bodyless stream.** A cap-0-nonempty stream captures nothing but sets `truncated: true` and emits `{as_string: "", truncated: true}` (the field is PRESENT). A genuinely bodyless stream (bodyless GET / zero-length response) never invokes the data hook at all — so no body is emitted. Tap must track "did the hook fire" (`sawReqBody`) separately from "is the buffer empty" (`len(reqBody) == 0`). A `len(buf) > 0` gate would wrongly drop the cap-0 case.

4. **Go's protojson rejects invalid UTF-8 in a `string` field — the reference does not.** Under `JSON_BODY_AS_STRING` a non-UTF8 body would make `protojson.Marshal` ERROR, and the sink swallows that error (`trace.go:65`), silently dropping the whole trace. The reference's C++ serializer instead mangles invalid UTF-8 lossily (unreproducible in Go). **Two consequences:** the differential drives UTF-8-safe (ASCII) bodies only and UNasserts non-UTF8 AS_STRING; and the IMPL sanitizes with `strings.ToValidUTF8` before building `Body_AsString` (never silently dropping). AS_BYTES (the proto default) is always faithful — base64 of arbitrary bytes.

### Key source seams (verified at PLAN time against master `1b344286`; re-confirm line numbers before editing — files evolve)

> **Two SPEC citation drifts caught at PLAN time (`feedback_brief_citations_not_evidence` — this row has paid out FOUR times; re-derive, never copy).** The SPEC's data-hook prose is correct, but two of its `file:line` citations are stale — cite the re-derived locations below, NOT the SPEC's:
> - **The encode-side one-call guarantee is NOT at `router.go:625`** (there is no `internal/filter/hcm/router.go`). It is at the two `RunEncodeData(ctx, resp.Body, true)` call sites: H1 `connection.go:743`, H2 `h2dispatch.go:582` — each invoked ONCE with `endStream=true` and the whole `resp.Body`. (`resp.Body` is fully materialized upstream in the cluster/action dispatch, not in an HCM `router.go`.)
> - **The proto accessors resolve at DIFFERENT line numbers than the SPEC cites** (the SPEC's `common.pb.go:544/549`, `:112/119` are for a different rev). In the resolved `github.com/envoyproxy/go-control-plane` module the accessors are: `OutputConfig.GetMaxBufferedRxBytes()`/`GetMaxBufferedTxBytes()` (return `*wrapperspb.UInt32Value`); `Body` struct field `BodyType isBody_BodyType` with arms `*Body_AsBytes{AsBytes []byte}` / `*Body_AsString{AsString string}` and `Truncated bool`; `OutputSink.GetFormat()` returns `OutputSink_Format` (enum `OutputSink_JSON_BODY_AS_BYTES = 0`, `OutputSink_JSON_BODY_AS_STRING = 1`). **Cite the accessor NAMES; the IMPL re-confirms line numbers against its own module cache.**

**The landed tap package** (`internal/filter/http/tap/`, ADR-0273 — the spine 56.2 extends):
- `config.go:24-29` — `type config struct { prog *matchpredicate.Program; sink *filePerTapSink; rqTapped *stats.Counter; recordConn bool }`. **56.2 adds three fields**: `bodyAsString bool`, `maxRx uint32`, `maxTx uint32`.
- `config.go:51` — `func parseConfig(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (*config, error)`.
- `config.go:100-110` — `oc := sc.GetOutputConfig()` (the `*OutputConfig`), the `streaming` reject, the `len(oc.GetSinks()) != 1` reject, `s := oc.GetSinks()[0]`. **56.2 reads `oc.GetMaxBufferedRxBytes()`/`GetMaxBufferedTxBytes()` here** (the caps live on `OutputConfig`, siblings of `sinks` — NOT on the sink).
- `config.go:113-122` — the `format` switch: `case JSON_BODY_AS_STRING, JSON_BODY_AS_BYTES:` (accepted, comment "Indistinguishable at 56.1"), the three `PROTO_*` rejects, the `default` reject. **56.2 stores the choice** (currently the switch discards `f`).
- `config.go:149` — `cfg := &config{prog: prog, sink: sink, recordConn: t.GetRecordDownstreamConnection()}`. **56.2 sets the three new fields here.**
- `tap.go:16-27` — `type tapFilter struct { cfg *config; decCB ...; encCB ...; reqHdrs/respHdrs http.Header; sawReq/sawResp bool }`. **56.2 adds `reqBody []byte`, `reqTrunc bool`, `respBody []byte`, `respTrunc bool`, `sawReqBody bool`, `sawRespBody bool`.**
- `tap.go:60-65` — the TWO INERT HOOKS to replace:
  ```go
  func (f *tapFilter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
  func (f *tapFilter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
  ```
- `trace.go:68-88` — `func (f *tapFilter) buildTrace() *datatapv3.TraceWrapper`. At `:70-78` it builds `bt.Request`/`bt.Response` `*HttpBufferedTrace_Message` with `Headers` only (the comment at `:71-73` says "Body and Trailers stay nil: no body at 56.1"). **56.2 sets `.Body` on each Message when the saw-body flag is set.**
- `sink.go:26-31` — `marshalOpts` (UNCHANGED). `sink.go:57-62` — `write` does `marshalOpts.Marshal(tw)` and returns the error; **the caller `trace.go:65` swallows it** (`_ = f.cfg.sink.write(...)`) — the reason the UTF-8 marshal error must be pre-empted by sanitize.

**The data-hook seam** (`internal/filter/http/types.go`):
- `types.go:56` — `DecodeData(data []byte, endStream bool) FilterDataStatus` (on `StreamDecoderFilter`).
- `types.go:67` — `EncodeData(data []byte, endStream bool) FilterDataStatus` (on `StreamEncoderFilter`).
- `types.go:31-38` — `const ( DataContinue FilterDataStatus = iota; DataStopIterationAndBuffer; DataStopIterationNoBuffer )`. **Tap returns `DataContinue` always.**

**The accumulation precedent** (`internal/filter/http/buffer/buffer.go` — SHAPE only, NOT reused):
- `buffer.go:65-72` — the filter struct holds a filter-owned `accumulated uint32` (a COUNT, not the bytes).
- `buffer.go:222-243` — `DecodeData`: `f.accumulated += uint32(len(data))` (`:228`), strict-`>` overflow `if f.accumulated > f.effectiveMax` (`:230`) → **`SendLocalReply(413, ...)` + `return DataStopIterationNoBuffer`** (`:231-234`). **Tap COPIES the filter-owned-field-updated-every-call SHAPE, but NOT the stop-iteration/413 behavior** — tap keeps the actual bytes and always returns `DataContinue`.

**The one-whole-body encode precedent** (`internal/filter/http/compressor/compressor.go`):
- The compressor's `EncodeData` guards `if !endStream { return DataContinue }` (Step 2) and relies on the whole-body-in-one-call encode guarantee, then `f.ecb.OverwriteBody(compressed)`. **Tap must NOT copy either the `!endStream` early-return (the DECODE side genuinely chunks) or `OverwriteBody` (tap is read-only).** This is a shape to contrast, not follow.

**The differential harness** (`test/differential/`, 56.1 D-TAP-DIRMOUNT already landed):
- `test/differential/fixture/fixture.go:165-175` — `HTTPEchoBody BackendKind = 6`: an out-of-process HTTP/1.1 backend (spawns `test/fixtures/0007b-iteration-probe/backends/main.go`) that returns 200 OK **with the request body if non-empty**, else the fixed 8-byte body `"backend\n"`. **So ONE POST populates BOTH `request.body` (rx, the POST body) and `response.body` (tx, the echoed body) with the same known UTF-8 content.** No new BackendKind — the tail stays 38.
- The `0099-http-tap-headers` driver (`test/fixtures/0099-http-tap-headers/driver/driver.go`) is the COMPLETE MODEL for `0100`: it registers via `fixture.RegisterFixture`, implements `Driver`+`BackendKindAware`+`ReferenceLogMounter`+`StatsAsserter`, bind-mounts a host dir via `HostMount{Dir: true}`, renders `envoy.yaml`/`envoy-go.yaml` templates, drives traffic, and in `AssertStats` polls+globs `out_*.json`, decodes each as a `datatapv3.TraceWrapper`, and asserts payload subsets + `rq_tapped`. **`0100` is `0099` with: `any_match` instead of `and_match`; the caps added to `output_config`; `HTTPEchoBody` instead of `HTTPStatusHeader`; three POSTs instead of bodyless GETs; body-PRESENT + `as_string` + `truncated` assertions instead of body-ABSENT.**
- `fixture.go:64` — `TB interface { Errorf; Fatalf; Helper }`. **No `Logf`** (`reference_fixture_tb_has_no_logf`).
- Asserter dispatch: `0100` is CROSS-SIDE, so ALL trace assertions go in `AssertStats(t, refAdminAddr, subjAdminAddr)` (`reference_differential_asserter_dispatch` — `AssertSubject` runs only on the reference-less path and would never fire here).
- `BackendCount()` returns **≥1** (`reference_differential_backendcount_min_one`).

### Proto facts (verified at PLAN time against the resolved `go-control-plane` module cache — re-confirm at IMPL)

- `data/tap/v3.Body` (constructed in `trace.go`):
  ```go
  // AS_STRING (format == JSON_BODY_AS_STRING):
  &datatapv3.Body{BodyType: &datatapv3.Body_AsString{AsString: sanitized}, Truncated: trunc}
  // AS_BYTES (format == JSON_BODY_AS_BYTES, the proto default 0):
  &datatapv3.Body{BodyType: &datatapv3.Body_AsBytes{AsBytes: buf}, Truncated: trunc}
  ```
  `BodyType` is the `body_type` oneof; `Truncated` is field 3 (`bool`), emitted unconditionally by `EmitDefaultValues`.
- `data/tap/v3.HttpBufferedTrace_Message` has `.Body *Body` (field 2). Set it on `bt.Request`/`bt.Response`.
- `config/tap/v3.OutputConfig`: `GetMaxBufferedRxBytes() *wrapperspb.UInt32Value` (field 2), `GetMaxBufferedTxBytes() *wrapperspb.UInt32Value` (field 3). Both `nil` when unset. A present value's `GetValue()` is a `uint32` (0 is a valid present value).
- `config/tap/v3.OutputSink`: `GetFormat() OutputSink_Format`; `OutputSink_JSON_BODY_AS_BYTES = 0` (proto default), `OutputSink_JSON_BODY_AS_STRING = 1`.
- **`go mod tidy -diff`** is anticipated EMPTY (all protos already resolved at 56.1).

### Discipline (honor on EVERY task)

- **A brief's citations are not evidence** (`feedback_brief_citations_not_evidence`). Every `file:line` above was re-derived from source at PLAN time; the IMPL re-derives AGAIN before editing. Two SPEC citations were already found stale (see the callout).
- **Design breaks that FIRE** (the 56.1 IMPL caught TWO MORE vacuous-proof classes at the unit level — PROGRESS-56.1 §Findings IMPL-1/IMPL-2). Each task's deliberate break is engineered to actually fail the intended assertion; the "Vacuous-proof guard" note in each task names the trap.
- **Subagent worktree hygiene:** deliberate breaks can detach HEAD (`feedback_subagent_worktree_detach`) — restore with `git restore` only (never checkout-sha/amend), re-verify the branch each task. Write to the canonical worktree root with worktree-relative paths (`feedback_subagent_worktree_path_targeting`); verify the main repo stays clean.

---

## D-question resolutions (the SPEC §12 PLAN pins — settled here)

### D-TAP-BODY-UTF8 → **sanitize the AS_STRING body with `strings.ToValidUTF8(string(buf), "�")`; NEVER silently drop**

Under `JSON_BODY_AS_STRING`, `Body_AsString` is built from `strings.ToValidUTF8(string(buf), "�")` (U+FFFD REPLACEMENT CHARACTER), not raw `string(buf)`. **Rationale:** Go's `protojson.Marshal` returns an error for invalid UTF-8 in a proto3 `string` field; the sink swallows a marshal error (`trace.go:65`), so a raw non-UTF8 AS_STRING body would silently drop the entire trace. Sanitizing keeps the trace (lossless-of-the-trace) and is deterministic. This is a DOCUMENTED DEPARTURE from the reference's specific (unreproducible) lossy C++ mangling — recorded in `BEHAVIOR_CONTRACT` and ADR-0274 as a coverage boundary. **AS_BYTES is unaffected** (base64 of arbitrary bytes always marshals), so the sanitize is applied ONLY on the AS_STRING arm. The `0100` differential drives UTF-8-safe (ASCII) bodies and never exercises this path; a UNIT test (Task 4) proves the sanitize is live (feed non-UTF8 bytes + `bodyAsString=true` → marshal must NOT error, and `as_string` is the sanitized form).

### D-TAP-BODY-ASBYTES-DIFF → **UNIT-tested, not a second differential listener**

The AS_BYTES render is proven by a UNIT test in `internal/filter/http/tap` (Task 4): `buildTrace` with a captured body + `bodyAsString=false` → `marshalOpts.Marshal` → assert `"as_bytes": "<base64>"` present, `"as_string"` ABSENT, `truncated` rendered. **Rationale:** AS_BYTES is `base64.StdEncoding` (with padding) that Go's protojson produces byte-identically to the reference (AMEND-TAP-BODY2-BYTES — probe-verified `"abcdefghij"` → `"YWJjZGVmZ2hpag=="`), so a unit test is a complete proof. A second AS_BYTES listener in `0100` would add driver complexity for a Go-native-deterministic render with no additional signal. The `0100` differential drives `JSON_BODY_AS_STRING` (one `format` per config).

### D-TAP-BODY-CHUNKTEST → **UNIT-tested (two `DecodeData` calls), not a differential >32KiB arm**

Cross-chunk accumulation is proven by a UNIT test (Task 3) feeding two+ `DecodeData` calls and asserting the concatenation (plus a >32 KiB total to exercise the real chunk size, and a chunk-straddling-the-cap case). **Rationale:** a differential >32 KiB body is slow, and the payload comparison is chunk-agnostic anyway (the reference's chunk boundaries are invisible to a decoded-payload comparison). The unit test directly exercises the accumulation logic that the differential cannot see.

### D-TAP-BODY-EARLYOUT → **DEFER the optimization; buffer unconditionally up to the cap, discard on no-match**

The IMPL does NOT skip body buffering when the predicate is already resolvably-False at `DecodeHeaders`. It buffers unconditionally up to the cap (the cap bounds memory) and discards on no-match at `OnDestroy` (the existing `if !ev.Resolve() { return }`, `trace.go:55-57`, already drops everything). **Rationale:** the reference buffers per the config caps regardless of the predicate; an early-out adds a correctness-sensitive branch (a response arm may still flip a partially-Undetermined tree, so a request-headers-False node is not final), for a memory saving the cap already bounds. **If ever adopted, it must NOT change any observable trace** (memory-only) — but it is out of scope for 56.2.

### FINAL ADR-0045 split-gate re-check (SPEC §10's escape valve — CONSUMED, re-confirmed)

The ADR-0045 split gate was CONSUMED at the phase-56 BRAINSTORM (Q6, by-concern split) and re-affirmed at SPEC-56.1 §2 / SPEC-56.2 §10. 56.2 is additive on a proven spine. This PLAN's honest decomposition is **7 tasks** (T1 baselines + 6 substantive from SPEC §10) — comfortably under the `~15` ceiling, margin **8**. No re-split. **Standing instruction for the IMPL:** if any task grows a second independent deliverable, split it into a NEW task and re-open the gate — do NOT push spillover past the 56.2 IMPL (56.2 is the FINAL leg of phase 56; there is no 56.3).

---

## File structure (decomposition locked here)

**Production (the ONLY package touched — `internal/filter/http/tap/`):**
- `config.go` — MODIFY `config` struct (+`bodyAsString`, `maxRx`, `maxTx`) + `parseConfig` (store format, resolve caps). *(Task 2)*
- `tap.go` — MODIFY `tapFilter` struct (+4 body fields, +2 saw-body flags) + the two data hooks (accumulate, cap, strict-`>` truncation, saw-body flag, always `DataContinue`). Add an unexported `accumulate` helper to DRY the two directions. *(Task 3)*
- `trace.go` — ADD `bodyProto(buf []byte, trunc bool, asString bool) *datatapv3.Body` + WIRE it into `buildTrace` (set `.Body` on each Message iff the saw-body flag is set). *(Task 4)*
- `sink.go` — UNCHANGED.

**Tests (in-package + fixture):**
- `config_test.go` — ADD cap-resolution + format-storage tests (nil→1024, present→value, present-0→0, AS_STRING/AS_BYTES; the PROTO_* rejects stay green). *(Task 2)*
- `tap_test.go` — ADD accumulation tests (single-chunk, multi-chunk, >32 KiB, at/under/over cap, chunk-straddles-cap, cap-0, always-`DataContinue`, both directions). *(Task 3)*
- `trace_test.go` (or the existing emit/marshal test file — the IMPL picks the landed name) — ADD `bodyProto`/`buildTrace` render tests (AS_STRING, AS_BYTES base64, truncated true/false, empty-but-truncated PRESENT, omitted-when-not-fired via RAW-BYTES, UTF-8 sanitize). *(Task 4)*

**Fixture (`test/fixtures/0100-http-tap-bodies/`):**
- `envoy.yaml` — reference bootstrap (STRICT_DNS, `any_match`, caps, `HTTPEchoBody`). *(Task 5)*
- `envoy-go.yaml` — subject bootstrap (STATIC, same shape). *(Task 5)*
- `driver/driver.go` — the driver (three POSTs + `AssertStats` glob-and-decode). *(Tasks 5+6)*

**Docs (the ADR-0052 atomic bundle, landed at the LAST task):**
- `docs/envoy-go/DECISIONS.md` — ADR-0274 §Decision/§Consequences (§Context already drafted at the SPEC). *(Task 7)*
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the body-capture delta. *(Task 7)*
- `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (flip row 56 `done`), `README.md`, `PROGRESS-56.2.md`. *(Task 7)*

---

## Task 1: Re-record the baselines in `PROGRESS-56.2.md` + confirm the ADR-0045 split re-check

**Files:**
- Create: `docs/envoy-go/phases/56-http-tap-filter/PROGRESS-56.2.md`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: the baseline block + the 7-task checklist that every later task ticks.

- [ ] **Step 1: Re-run the baseline commands against THIS worktree's cold-start HEAD** (do not assume the SPEC's figures still hold):

```bash
git log --oneline -1
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l          # expect 101
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l   # expect 53
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1              # expect ADR-0273
grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1     # expect 38
```

Expected: `BUILD_OK`, fixtures **101** (tail `0099-http-tap-headers`), fuzzers **53**, DECISIONS tail **ADR-0273** (next-free ADR-0274), BackendKind tail **38** (`H2GoawayResponder`). Stat surface **1201** (a live reference figure; the tree enforces a delta — 56.2 adds **+0**, so no delta guard changes).

- [ ] **Step 2: Write `PROGRESS-56.2.md`** — paste the literal output into a "Baseline Counts" block, scaffold the 7-task checklist (all `- [ ]`), and add the empty "Deliberate-break ledger" table (filled at the IMPL with the LITERAL failing text — the controller re-performs every break itself) and "Findings log" section. Mirror the shape of `PROGRESS-56.1.md`.

- [ ] **Step 3: Record the ADR-0045 split re-check** — state the honest task count (**7**), the ceiling (`~15`), the margin (**8**), and the standing instruction ("if a task grows a second deliverable, split + re-open the gate; do NOT spill past the 56.2 IMPL").

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/56-http-tap-filter/PROGRESS-56.2.md
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T1 baselines + ADR-0045 split re-check"
```

---

## Task 2: `config.go` — store the `format` + resolve both caps (nil → 1024; present → value incl. 0) [TDD]

**Files:**
- Modify: `internal/filter/http/tap/config.go` (the `config` struct `:24-29`; `parseConfig` — add cap resolution near `:100-110`; store the format in the switch `:113-122`; set the fields at `:149`).
- Test: `internal/filter/http/tap/config_test.go` (ADD to the landed file).

**Interfaces:**
- Consumes: `config/tap/v3.OutputConfig.GetMaxBufferedRxBytes()/GetMaxBufferedTxBytes()` (`*wrapperspb.UInt32Value`); `OutputSink.GetFormat()` (`OutputSink_Format`).
- Produces: `config.bodyAsString bool`, `config.maxRx uint32`, `config.maxTx uint32` (read by Tasks 3+4). The cap-resolution helper `resolveCap(w *wrapperspb.UInt32Value) uint32` (unexported; nil → 1024, else `w.GetValue()`).

- [ ] **Step 1: Write the failing tests** (append to `config_test.go`). Drive `parseConfig` (or `New`) with a full valid Tap proto and assert the resolved `config`:

```go
// defaultTapBytesCap is the cap applied when max_buffered_*_bytes is unset
// (AMEND-TAP-BODY2-DEFAULTCAP; probe-pinned 1024 against contrib-v1.37.2).
const defaultTapBytesCap = 1024

func TestParseConfig_CapResolution(t *testing.T) {
	tests := []struct {
		name        string
		rx, tx      *wrapperspb.UInt32Value // nil = unset
		wantRx      uint32
		wantTx      uint32
	}{
		{"both unset -> default 1024", nil, nil, 1024, 1024},
		{"rx present 20 -> 20", wrapperspb.UInt32(20), nil, 20, 1024},
		{"tx present 40 -> 40", nil, wrapperspb.UInt32(40), 1024, 40},
		{"present ZERO -> 0 (NOT 1024)", wrapperspb.UInt32(0), wrapperspb.UInt32(0), 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mustParseTapWithCaps(t, taptapv3.OutputSink_JSON_BODY_AS_STRING, tt.rx, tt.tx)
			if cfg.maxRx != tt.wantRx {
				t.Errorf("maxRx = %d, want %d", cfg.maxRx, tt.wantRx)
			}
			if cfg.maxTx != tt.wantTx {
				t.Errorf("maxTx = %d, want %d", cfg.maxTx, tt.wantTx)
			}
		})
	}
}

func TestParseConfig_FormatStored(t *testing.T) {
	asStr := mustParseTapWithCaps(t, taptapv3.OutputSink_JSON_BODY_AS_STRING, nil, nil)
	if !asStr.bodyAsString {
		t.Errorf("JSON_BODY_AS_STRING: bodyAsString = false, want true")
	}
	asBytes := mustParseTapWithCaps(t, taptapv3.OutputSink_JSON_BODY_AS_BYTES, nil, nil)
	if asBytes.bodyAsString {
		t.Errorf("JSON_BODY_AS_BYTES: bodyAsString = true, want false")
	}
}
```

Add a `mustParseTapWithCaps` helper that builds a valid `httptapv3.Tap` (an `any_match` predicate, a `file_per_tap` sink to a temp dir, the given `format`, and `OutputConfig{MaxBufferedRxBytes: rx, MaxBufferedTxBytes: tx}`), wraps it in an `anypb.Any`, calls `parseConfig`, and returns the `*config` (Fatalf on parse error — a broken precondition). Model it on the landed `config_test.go` helpers.

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/http/tap/ -run 'TestParseConfig_(CapResolution|FormatStored)' -count=1 -v`
Expected: compile error (`cfg.maxRx`/`cfg.bodyAsString` undefined) or FAIL.

- [ ] **Step 3: Implement** — add the three fields + resolve. In the `config` struct (`config.go:24-29`):

```go
type config struct {
	prog         *matchpredicate.Program
	sink         *filePerTapSink
	rqTapped     *stats.Counter
	recordConn   bool
	bodyAsString bool   // 56.2: JSON_BODY_AS_STRING vs _AS_BYTES render choice
	maxRx        uint32 // 56.2: request-body cap (nil wrapper -> 1024)
	maxTx        uint32 // 56.2: response-body cap (nil wrapper -> 1024)
}
```

In the format switch (`config.go:113-122`), record the choice (the two PROTO branches + default still reject, unchanged):

```go
var asString bool
switch f := s.GetFormat(); f {
case taptapv3.OutputSink_JSON_BODY_AS_STRING:
	asString = true
case taptapv3.OutputSink_JSON_BODY_AS_BYTES:
	asString = false // the proto default
case taptapv3.OutputSink_PROTO_BINARY,
	taptapv3.OutputSink_PROTO_BINARY_LENGTH_DELIMITED,
	taptapv3.OutputSink_PROTO_TEXT:
	return nil, fmt.Errorf("tap: output_config.sinks[0].format %v is not supported", f)
default:
	return nil, fmt.Errorf("tap: unknown output_config.sinks[0].format %v", f)
}
```

Add the cap-resolution helper (default 1024 on a nil wrapper; present value including 0):

```go
// resolveCap returns the per-direction body cap. A nil UInt32Value wrapper
// means the field was unset, which the reference caps at 1024 bytes
// (AMEND-TAP-BODY2-DEFAULTCAP). A PRESENT wrapper uses its value, including 0
// (a 0 cap yields an empty-but-truncated body, distinct from unbounded).
func resolveCap(w *wrapperspb.UInt32Value) uint32 {
	if w == nil {
		return 1024
	}
	return w.GetValue()
}
```

Set the fields at the `cfg` construction (`config.go:149`), reading the caps off `oc` (the `*OutputConfig`):

```go
cfg := &config{
	prog:         prog,
	sink:         sink,
	recordConn:   t.GetRecordDownstreamConnection(),
	bodyAsString: asString,
	maxRx:        resolveCap(oc.GetMaxBufferedRxBytes()),
	maxTx:        resolveCap(oc.GetMaxBufferedTxBytes()),
}
```

Add the `wrapperspb` import (`google.golang.org/protobuf/types/known/wrapperspb`).

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/http/tap/ -count=1` — all green (including the landed reject-roster tests).

- [ ] **Step 5: Gate + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/... && go vet ./... && go build ./...
git add internal/filter/http/tap/config.go internal/filter/http/tap/config_test.go
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T2 config -- store format + resolve caps (nil->1024, present-0->0)"
```

- [ ] **Step 6: Deliberate break (controller re-performs) — the present-ZERO trap.** Change `resolveCap` to `if w == nil || w.GetValue() == 0 { return 1024 }`. Run `go test ./internal/filter/http/tap/ -run 'TestParseConfig_CapResolution/present_ZERO' -count=1 -v`. **Expected: the "present ZERO -> 0" subtest FIRES** (`maxRx = 1024, want 0`). This proves the nil-vs-present-0 distinction is live — the single most likely wrong reading of the wrapper. `git restore` and re-run to confirm green.

> **Vacuous-proof guard:** a test that only checks the nil→1024 case would pass even if the impl mapped present-0→1024 (a real, tempting bug: "0 looks like unset"). The `present ZERO` row + the break together prove the distinction. Do NOT collapse the two into one assertion.

---

## Task 3: `tap.go` — the `DecodeData`/`EncodeData` accumulation (append up to cap, strict-`>` truncation, saw-body flag, always `DataContinue`) [TDD]

**Files:**
- Modify: `internal/filter/http/tap/tap.go` (the `tapFilter` struct `:16-27`; the two data hooks `:60-65`).
- Test: `internal/filter/http/tap/tap_test.go` (ADD to the landed file).

**Interfaces:**
- Consumes: `config.maxRx`, `config.maxTx` (Task 2).
- Produces: `tapFilter.reqBody []byte`, `.reqTrunc bool`, `.respBody []byte`, `.respTrunc bool`, `.sawReqBody bool`, `.sawRespBody bool` (read by Task 4's `buildTrace`). The unexported helper `accumulate(buf []byte, trunc *bool, chunk []byte, cap uint32) []byte` (DRYs the two directions).

- [ ] **Step 1: Write the failing tests** (append to `tap_test.go`). Exercise the accumulation directly on a `*tapFilter` with a hand-built `config`:

```go
func newBodyFilter(maxRx, maxTx uint32) *tapFilter {
	return &tapFilter{cfg: &config{maxRx: maxRx, maxTx: maxTx}}
}

func TestDecodeData_SingleChunkUnderCap(t *testing.T) {
	f := newBodyFilter(20, 20)
	if st := f.DecodeData([]byte("0123456789"), true); st != envoyhttp.DataContinue {
		t.Errorf("DecodeData status = %v, want DataContinue (tap is read-only)", st)
	}
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want %q", f.reqBody, "0123456789")
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false (10 < cap 20)")
	}
	if !f.sawReqBody {
		t.Errorf("sawReqBody = false, want true (hook fired)")
	}
}

func TestDecodeData_MultiChunkAccumulates(t *testing.T) {
	f := newBodyFilter(100, 100)
	f.DecodeData([]byte("AAA"), false)
	f.DecodeData([]byte("BBB"), false)
	f.DecodeData(nil, true) // synthetic end (connection.go:653)
	if string(f.reqBody) != "AAABBB" {
		t.Errorf("reqBody = %q, want %q (chunks must concatenate)", f.reqBody, "AAABBB")
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false")
	}
}

func TestDecodeData_Over32KiBAccumulates(t *testing.T) {
	f := newBodyFilter(1 << 20, 1 << 20) // 1 MiB cap, above 32 KiB
	chunk := bytes.Repeat([]byte("x"), 32*1024)
	f.DecodeData(chunk, false)
	f.DecodeData(chunk, true) // 64 KiB total across two real-sized chunks
	if len(f.reqBody) != 64*1024 {
		t.Errorf("reqBody len = %d, want %d (must accumulate across >32KiB)", len(f.reqBody), 64*1024)
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false (64KiB < 1MiB cap)")
	}
}

func TestDecodeData_AtCapNotTruncated(t *testing.T) { // strict-> boundary
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("0123456789"), true) // exactly 10 == cap
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want full 10 bytes", f.reqBody)
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want FALSE (body length == cap is NOT truncated; strict >)")
	}
}

func TestDecodeData_OverCapTruncates(t *testing.T) {
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("0123456789ABCDEF"), true) // 16 > cap 10
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want first 10 bytes only", f.reqBody)
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (16 > cap 10)")
	}
}

func TestDecodeData_ChunkStraddlesCap(t *testing.T) {
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("012345"), false)   // 6 bytes, under cap
	f.DecodeData([]byte("6789ABCD"), true)  // 6+8=14 > 10: append prefix "6789" (4 = 10-6)
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want %q (only cap-capturedLen prefix of the straddling chunk)", f.reqBody, "0123456789")
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (14 > cap 10)")
	}
}

func TestDecodeData_CapZeroNonEmpty_EmptyButTruncated(t *testing.T) {
	f := newBodyFilter(0, 0)
	f.DecodeData([]byte("nonempty"), true)
	if len(f.reqBody) != 0 {
		t.Errorf("reqBody len = %d, want 0 (cap 0 captures nothing)", len(f.reqBody))
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (cap 0 + non-empty body -> truncated)")
	}
	if !f.sawReqBody {
		t.Errorf("sawReqBody = false, want true (hook fired -> body must be PRESENT even when empty)")
	}
}

func TestEncodeData_SymmetricIntoRespBody(t *testing.T) {
	f := newBodyFilter(20, 5)
	if st := f.EncodeData([]byte("HELLOWORLD"), true); st != envoyhttp.DataContinue {
		t.Errorf("EncodeData status = %v, want DataContinue", st)
	}
	if string(f.respBody) != "HELLO" {
		t.Errorf("respBody = %q, want %q (cap 5)", f.respBody, "HELLO")
	}
	if !f.respTrunc {
		t.Errorf("respTrunc = false, want true (10 > cap 5)")
	}
	if !f.sawRespBody {
		t.Errorf("sawRespBody = false, want true")
	}
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/http/tap/ -run 'TestDecodeData|TestEncodeData' -count=1 -v`
Expected: compile error (fields/helper undefined) or FAIL.

- [ ] **Step 3: Implement.** Extend the struct (`tap.go:16-27`):

```go
type tapFilter struct {
	cfg *config

	decCB envoyhttp.DecoderFilterCallbacks
	encCB envoyhttp.EncoderFilterCallbacks

	reqHdrs  http.Header
	respHdrs http.Header
	sawReq   bool
	sawResp  bool

	// 56.2 body capture: filter-owned accumulation (buffer/buffer.go SHAPE).
	reqBody     []byte
	reqTrunc    bool
	respBody    []byte
	respTrunc   bool
	sawReqBody  bool // hook fired at least once (distinct from len(reqBody)==0)
	sawRespBody bool
}
```

Replace the two inert hooks (`tap.go:60-65`) with accumulating ones sharing one helper:

```go
// accumulate appends chunk to buf, honoring the byte cap. Truncation is strict
// `>` (AMEND-TAP-BODY2-BOUNDARY): a body EXACTLY at the cap is NOT truncated.
// Once the cap is reached *trunc is set and no further bytes are appended.
// Returns the (possibly-grown) buffer.
func accumulate(buf []byte, trunc *bool, chunk []byte, cap uint32) []byte {
	room := int(cap) - len(buf)
	if len(chunk) > room { // strict >: len(chunk) fits iff len(chunk) <= room
		if room > 0 {
			buf = append(buf, chunk[:room]...)
		}
		*trunc = true
		return buf
	}
	return append(buf, chunk...)
}

// DecodeData accumulates the request body up to cfg.maxRx. Tap is a READ-ONLY
// observer: it returns DataContinue ALWAYS (never StopIteration/OverwriteBody)
// so the body flows unperturbed to the upstream. endStream is accepted for
// signature conformance; the buffer IS the state, no finalization is needed.
func (f *tapFilter) DecodeData(data []byte, _ bool) envoyhttp.FilterDataStatus {
	f.sawReqBody = true
	f.reqBody = accumulate(f.reqBody, &f.reqTrunc, data, f.cfg.maxRx)
	return envoyhttp.DataContinue
}

// EncodeData is symmetric into respBody up to cfg.maxTx.
func (f *tapFilter) EncodeData(data []byte, _ bool) envoyhttp.FilterDataStatus {
	f.sawRespBody = true
	f.respBody = accumulate(f.respBody, &f.respTrunc, data, f.cfg.maxTx)
	return envoyhttp.DataContinue
}
```

*(Note the strict-`>` semantics: `room = cap - len(buf)`; a chunk of length `== room` fits with no truncation — `len(chunk) > room` is false — matching "body exactly at cap is not truncated". Rename the `cap` parameter if the linter flags shadowing the builtin.)*

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/http/tap/ -count=1` — all green.

- [ ] **Step 5: Gate + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/... && go vet ./... && go build ./...
git add internal/filter/http/tap/tap.go internal/filter/http/tap/tap_test.go
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T3 tap -- DecodeData/EncodeData accumulate with strict-> truncation, always DataContinue"
```

- [ ] **Step 6: Deliberate breaks (controller re-performs THREE, each isolated).**
  - **(3a) strict `>` → `>=`:** change `accumulate`'s comparison to `len(chunk) >= room`. Run `-run 'TestDecodeData_AtCapNotTruncated' -count=1 -v`. **Expected: `reqTrunc = true, want FALSE` FIRES** (the at-cap body wrongly truncates). This mirrors the differential's boundary arm at the unit level. `git restore`, re-run green.
  - **(3b) drop the saw-body flag on cap-0:** guard `f.sawReqBody = true` behind `if len(data) > 0 && f.cfg.maxRx > 0`. Run `-run 'TestDecodeData_CapZeroNonEmpty' -count=1 -v`. **Expected: `sawReqBody = false, want true` FIRES.** Proves the cap-0 body is still PRESENT. `git restore`, re-run green.
  - **(3c) return `DataStopIterationAndBuffer`:** change `DecodeData` to return `envoyhttp.DataStopIterationAndBuffer`. Run `-run 'TestDecodeData_SingleChunkUnderCap' -count=1 -v`. **Expected: `DecodeData status = ..., want DataContinue` FIRES.** Proves the read-only-observer contract. `git restore`, re-run green.

> **Vacuous-proof guard:** the at-cap test MUST use a body length == cap (not < cap), or break (3a) cannot flip it. The cap-0 test MUST assert `sawReqBody` (not just `reqTrunc`), or break (3b) is invisible. The status assertion MUST be a distinct `t.Errorf` (not folded into a Fatalf precondition), or break (3c) would leave the payload checks unreachable.

---

## Task 4: `trace.go` — `bodyProto` + `buildTrace` wiring (Body iff hook fired; oneof from format; the UTF-8 sanitize) [TDD]

**Files:**
- Modify: `internal/filter/http/tap/trace.go` (ADD `bodyProto`; wire into `buildTrace` `:68-88`).
- Test: `internal/filter/http/tap/trace_test.go` (ADD; or the landed emit/marshal test file — the IMPL uses the existing name).

**Interfaces:**
- Consumes: `tapFilter.reqBody/reqTrunc/respBody/respTrunc/sawReqBody/sawRespBody` (Task 3); `config.bodyAsString` (Task 2); `marshalOpts` (`sink.go`, unchanged).
- Produces: `bodyProto(buf []byte, trunc bool, asString bool) *datatapv3.Body`; `buildTrace` now sets `.Body` on each Message.

- [ ] **Step 1: Write the failing tests.** Render a body through `bodyProto` + `marshalOpts.Marshal` and assert the JSON (use `encoding/json.Compact` canonicalization + substring pins — the detrand caveat, ADR-0273 §Consequences; NEVER raw byte-compare protojson):

```go
func marshalBody(t *testing.T, b *datatapv3.Body) string {
	t.Helper()
	raw, err := marshalOpts.Marshal(&datatapv3.HttpBufferedTrace{
		Request: &datatapv3.HttpBufferedTrace_Message{Body: b},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err) // AS_STRING sanitize must PREVENT this
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact: %v", err)
	}
	return buf.String()
}

func TestBodyProto_AsString(t *testing.T) {
	s := marshalBody(t, bodyProto([]byte("0123456789"), false, true))
	if !strings.Contains(s, `"as_string":"0123456789"`) {
		t.Errorf("AS_STRING render missing as_string: %s", s)
	}
	if strings.Contains(s, `"as_bytes"`) {
		t.Errorf("AS_STRING must NOT render as_bytes: %s", s)
	}
	if !strings.Contains(s, `"truncated":false`) {
		t.Errorf("truncated:false must be emitted (EmitDefaultValues): %s", s)
	}
}

func TestBodyProto_AsBytesBase64(t *testing.T) {
	// AMEND-TAP-BODY2-BYTES: "abcdefghij" -> "YWJjZGVmZ2hpag==" (Go-native).
	s := marshalBody(t, bodyProto([]byte("abcdefghij"), false, false))
	if !strings.Contains(s, `"as_bytes":"YWJjZGVmZ2hpag=="`) {
		t.Errorf("AS_BYTES render wrong base64: %s", s)
	}
	if strings.Contains(s, `"as_string"`) {
		t.Errorf("AS_BYTES must NOT render as_string: %s", s)
	}
}

func TestBodyProto_TruncatedTrue(t *testing.T) {
	s := marshalBody(t, bodyProto([]byte("0123456789"), true, true))
	if !strings.Contains(s, `"truncated":true`) {
		t.Errorf("truncated:true must render: %s", s)
	}
}

func TestBodyProto_AsStringSanitizesNonUTF8(t *testing.T) {
	// D-TAP-BODY-UTF8: raw string(buf) would make protojson.Marshal ERROR and
	// the sink swallow it -> whole-trace drop. Sanitize keeps the trace.
	nonUTF8 := []byte{0xff, 0xfe, 0x41, 0x42} // invalid UTF-8 + "AB"
	s := marshalBody(t, bodyProto(nonUTF8, false, true)) // marshalBody Fatalfs on error
	if !strings.Contains(s, "AB") {
		t.Errorf("sanitized as_string should retain the valid tail: %s", s)
	}
}

func TestBuildTrace_BodyPresentWhenHookFired_EmptyButTruncated(t *testing.T) {
	f := &tapFilter{cfg: &config{bodyAsString: true, prog: mustAnyMatchProgram(t)}}
	f.sawReq, f.reqHdrs = true, http.Header{}
	f.sawReqBody, f.reqBody, f.reqTrunc = true, nil, true // cap-0 shape
	raw, err := marshalOpts.Marshal(f.buildTrace())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"body"`)) {
		t.Errorf("empty-but-truncated body must be PRESENT (hook fired): %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"truncated": true`)) && !bytes.Contains(raw, []byte(`"truncated":true`)) {
		t.Errorf("empty-but-truncated body must carry truncated:true: %s", raw)
	}
}

func TestBuildTrace_BodyOmittedWhenHookNeverFired(t *testing.T) {
	f := &tapFilter{cfg: &config{bodyAsString: true, prog: mustAnyMatchProgram(t)}}
	f.sawReq, f.reqHdrs = true, http.Header{}
	// sawReqBody stays FALSE (bodyless stream)
	raw, err := marshalOpts.Marshal(f.buildTrace())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// RAW-BYTES check: a decode cannot tell OMITTED from null (PROGRESS-56.1
	// IMPL-1). Grep the raw bytes for any "body" key.
	if bytes.Contains(raw, []byte(`"body"`)) {
		t.Errorf("body must be OMITTED when the hook never fired: %s", raw)
	}
}
```

*(`mustAnyMatchProgram` compiles an `any_match: true` predicate so `buildTrace`'s Request assembly runs; reuse or add near the landed emit-test helpers.)*

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/http/tap/ -run 'TestBodyProto|TestBuildTrace_Body' -count=1 -v`
Expected: compile error (`bodyProto` undefined) or FAIL.

- [ ] **Step 3: Implement.** Add `bodyProto` to `trace.go`:

```go
// bodyProto renders a captured body as a data/tap/v3.Body. Under AS_STRING the
// bytes are sanitized to valid UTF-8 (D-TAP-BODY-UTF8): Go's protojson.Marshal
// ERRORS on invalid UTF-8 in a proto3 string field and the sink swallows that
// error (trace.go OnDestroy), so a raw non-UTF8 body would silently drop the
// whole trace. strings.ToValidUTF8 is a documented DEPARTURE from the
// reference's lossy C++ mangling -- lossless-of-the-trace and deterministic.
// AS_BYTES base64-encodes arbitrary bytes faithfully (the proto default).
// EmitDefaultValues renders truncated unconditionally (AMEND-TAP-BODY2-TRUNCFLAG).
func bodyProto(buf []byte, trunc bool, asString bool) *datatapv3.Body {
	if asString {
		return &datatapv3.Body{
			BodyType:  &datatapv3.Body_AsString{AsString: strings.ToValidUTF8(string(buf), "�")},
			Truncated: trunc,
		}
	}
	return &datatapv3.Body{
		BodyType:  &datatapv3.Body_AsBytes{AsBytes: buf},
		Truncated: trunc,
	}
}
```

Wire it into `buildTrace` (`trace.go:70-78`), setting `.Body` IFF the saw-body flag is set:

```go
if f.sawReq {
	m := &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.reqHdrs)}
	if f.sawReqBody {
		m.Body = bodyProto(f.reqBody, f.reqTrunc, f.cfg.bodyAsString)
	}
	bt.Request = m
}
if f.sawResp {
	m := &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.respHdrs)}
	if f.sawRespBody {
		m.Body = bodyProto(f.respBody, f.respTrunc, f.cfg.bodyAsString)
	}
	bt.Response = m
}
```

Add the `strings` import.

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/http/tap/ -count=1` — all green.

- [ ] **Step 5: Gate + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/... && go vet ./... && go build ./...
git add internal/filter/http/tap/trace.go internal/filter/http/tap/trace_test.go
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T4 trace -- bodyProto (oneof from format, UTF-8 sanitize) + buildTrace wiring (Body iff hook fired)"
```

- [ ] **Step 6: Deliberate breaks (controller re-performs FOUR, each isolated).**
  - **(4a) swap the oneof:** in `bodyProto`'s AS_STRING branch emit `Body_AsBytes` instead. Run `-run 'TestBodyProto_AsString' -count=1 -v`. **Expected: the `as_string` missing + `as_bytes` present assertions FIRE.** `git restore`.
  - **(4b) drop the sanitize:** change AS_STRING to raw `string(buf)`. Run `-run 'TestBodyProto_AsStringSanitizesNonUTF8' -count=1 -v`. **Expected: `marshal: <invalid UTF-8> ` Fatalf FIRES** (proving the sanitize prevents the whole-trace drop). `git restore`.
  - **(4c) gate Body on `len(buf) > 0`:** change the `if f.sawReqBody` guard to `if len(f.reqBody) > 0`. Run `-run 'TestBuildTrace_BodyPresentWhenHookFired' -count=1 -v`. **Expected: `empty-but-truncated body must be PRESENT` FIRES** (cap-0 wrongly omits). `git restore`.
  - **(4d) always set Body:** set `.Body` unconditionally (drop the `if f.sawReqBody`). Run `-run 'TestBuildTrace_BodyOmittedWhenHookNeverFired' -count=1 -v`. **Expected: `body must be OMITTED when the hook never fired` FIRES.** `git restore`. (Confirm the RAW-BYTES check fires, not a decode-based check — a decode cannot tell omitted from null, PROGRESS-56.1 IMPL-1.)

> **Vacuous-proof guard:** the omitted-body test MUST grep RAW BYTES for `"body"` (a decoded `GetBody()==nil` cannot distinguish omitted from an `EmitUnpopulated` `null`, exactly the 56.1 IMPL-1 defect). The empty-but-truncated test MUST drive `sawReqBody=true, reqBody=nil` — if it drove a non-empty body, break (4c)'s `len>0` gate would still pass and the break would be vacuous.

---

## Task 5: Fixture `0100-http-tap-bodies` — the YAMLs + the driver (three POSTs against `HTTPEchoBody`)

**Files:**
- Create: `test/fixtures/0100-http-tap-bodies/envoy.yaml`
- Create: `test/fixtures/0100-http-tap-bodies/envoy-go.yaml`
- Create: `test/fixtures/0100-http-tap-bodies/driver/driver.go`

**Interfaces:**
- Consumes: `HTTPEchoBody` (BackendKind 6); `HostMount{Dir: true}`; the `Driver`/`BackendKindAware`/`ReferenceLogMounter`/`StatsAsserter` interfaces (all landed).
- Produces: a registered `0100-http-tap-bodies` fixture whose `AssertStats` (Task 6) performs the cross-side trace comparison.

- [ ] **Step 1: Write the two bootstraps** — COPY `0099`'s `envoy.yaml`/`envoy-go.yaml` verbatim, then change exactly three things (keep `stat_prefix: tap_probe`, the `file_per_tap` sink, `format: JSON_BODY_AS_STRING`, and the STRICT_DNS/STATIC cluster split):
  1. **Predicate:** replace the `and_match{...}` block with `any_match: true` (bodies are the subject, not the predicate — every stream taps).
  2. **Caps:** add to `output_config` (siblings of `sinks`, NOT inside the sink):
     ```yaml
     output_config:
       max_buffered_rx_bytes: {{.Cap}}
       max_buffered_tx_bytes: {{.Cap}}
       sinks:
       - format: JSON_BODY_AS_STRING
         file_per_tap:
           path_prefix: {{.TapPrefix}}
     ```
     with `Cap` rendered as a chosen **C = 20**.
  3. **Route/backend:** unchanged shape — the cluster still points at `{{.BackendHost}}:{{.BackendPort}}`; the driver's `BackendKind()` returns `HTTPEchoBody` so the runner spawns the echo backend.

- [ ] **Step 2: Write `driver/driver.go`** — COPY the `0099` driver as the skeleton (the `RegisterFixture`/`newTapDriver`/host-mount/template/`ProbeAdmin`/`pollTraces`/`scrapeCounter` machinery is REUSED verbatim), then change:
  - `init()` registers `"0100-http-tap-bodies"`; `fixtureDir()` doc updates to the new path.
  - `BackendKind()` returns `fixture.HTTPEchoBody` (was `HTTPStatusHeader`).
  - The bootstrap renders pass `"Cap": capC` (a `const capC = 20`).
  - `drive` issues **three POSTs** through the one config (all ASCII — UTF-8-safe, AMEND-TAP-BODY2-UTF8), recording the body it drove so `AssertStats` can compare:

```go
const (
	capC       = 20                     // max_buffered_rx/tx_bytes
	bodyFull   = "0123456789"           // 10 bytes  (< C: full, not truncated)
	bodyBound  = "0123456789ABCDEFGHIJ" // 20 bytes  (== C: full, NOT truncated -- strict >)
	bodyTrunc  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcd" // 40 bytes (> C: truncated to first 20)
	nTraces    = 3
)

func (d *tapDriver) drive(ctx context.Context, addr string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	var out bytes.Buffer
	post := func(body string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/echo",
			strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("content-type", "text/plain")
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		fmt.Fprintf(&out, "POST %d %d\n", len(body), resp.StatusCode)
		return nil
	}
	for _, b := range []string{bodyFull, bodyBound, bodyTrunc} {
		if err := post(b); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}
```

*(The `HTTPEchoBody` backend echoes the POST body back as the response body, so each POST populates BOTH `request.body` (rx) and `response.body` (tx) with the same content — one config exercises both directions.)*

- [ ] **Step 3: Register the fixture in the runner if required** — check whether `test/differential/runner_test.go` needs the driver package blank-imported (grep for how `0099` is wired). If the runner auto-discovers via `init()`+`RegisterFixture`, no runner edit is needed; if it blank-imports each driver, add the `0100` import alongside `0099`.

- [ ] **Step 4: Build the fixture package** (the assertions land in Task 6; here just prove it compiles + registers):

Run: `go build ./test/... && go vet ./test/...`
Expected: clean. `go test ./test/differential/ -run 'TestDifferential/0100-http-tap-bodies' -count=1 -list '.*'` lists the subtest (proves registration; `reference_differential_run_selector`).

- [ ] **Step 5: Commit** (the driver's `AssertStats` is a stub returning immediately at this task, filled in Task 6 — or fold Steps into Task 6 if the reviewer prefers one deliverable; per Task Right-Sizing the config+driver-skeleton and the assertions+breaks are separable reviewer gates):

```bash
gofmt -l test/fixtures/0100-http-tap-bodies/ && go vet ./test/...
git add test/fixtures/0100-http-tap-bodies/
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T5 fixture 0100 -- YAMLs + driver (three POSTs vs HTTPEchoBody); fixtures 101 -> 102"
```

> **Note (fixture-dispatch constraint, `reference_differential_fixture_dispatch_constraint`):** `0100` is an all-cross-side fixture (both proxies capture). The reject roster stays UNIT-tested (Task 2), NOT in this dir — a boot-reject cannot share a cross-side fixture dir. `BackendCount()` returns 1 (`reference_differential_backendcount_min_one`).

---

## Task 6: `0100` `AssertStats` — the glob-and-decode assertions + the 4 deliberate breaks

**Files:**
- Modify: `test/fixtures/0100-http-tap-bodies/driver/driver.go` (fill `AssertStats`).

**Interfaces:**
- Consumes: the driven bodies (Task 5); the landed `pollTraces`/`scrapeCounter`/`assertSubset` helpers (COPY from `0099`).
- Produces: the cross-side body-payload + `truncated`-flag + body-PRESENT assertions.

- [ ] **Step 1: Write `AssertStats`** — poll+glob both sides' `out_*.json` (expect `nTraces` == 3), decode each as a `datatapv3.TraceWrapper`, and assert per-property (`t.Errorf` each, NEVER `t.Fatalf` mid-list). Because the three traces are written per-POST but filenames are unpredictable, MATCH each trace to its expected arm by the captured `request.body` length (10/20/20 — note the truncated arm captures 20 bytes) — or, more robustly, collect the observed (as_string, truncated) pairs into a set and assert the set equals the expected set. Expected per arm (both `request.body` and `response.body`, cross-side EXACT):

| arm | driven len | expected `as_string` | expected `truncated` |
|---|---|---|---|
| full | 10 | `"0123456789"` | `false` |
| boundary | 20 (== C) | `"0123456789ABCDEFGHIJ"` (FULL) | **`false`** (strict `>`) |
| truncated | 40 (> C) | `"0123456789ABCDEFGHIJ"` (first 20) | `true` |

Assertions (each side, each trace):
- trace COUNT == 3; `http.tap_probe.tap.rq_tapped` == 3 (both sides).
- `request.body` and `response.body` are PRESENT (not nil) on EVERY arm — the inverse of `0099`'s absent-assertion.
- `request.body.as_string` / `response.body.as_string` == the expected value for that arm — cross-side EXACT.
- `request.body.truncated` / `response.body.truncated` == the expected bool — a POSITIVE breakable assertion.
- the boundary-arm trace asserts `truncated == false` at driven-length == C (the strict-`>` proof — this is the arm break (c) flips).

Reuse `assertSubset` for the header subsets if desired (unchanged from `0099`), but the NEW signal is the body block. Model the decode + per-file loop on `0099`'s `assertSide`.

- [ ] **Step 2: Run the differential (both sides)** — requires Docker (the reference container, `reference_docker_probe_bridge_network`):

Run: `go test ./test/differential/ -run 'TestDifferential/0100-http-tap-bodies' -count=1 -v`
Expected: PASS (both sides capture identical bodies + truncation flags).

- [ ] **Step 3: Deliberate breaks (controller re-performs FOUR, each `-count=1`, edit SUBJECT-side production code so the SUBJECT trace violates — confirm the failure text names `subject/...`):**
  - **(a) leave `DecodeData`/`EncodeData` inert** (revert to `return DataContinue` pass-throughs, no capture). **Expected: the `request.body`/`response.body` PRESENT assertion FIRES on all arms** (bodies absent). Proves capture is LIVE.
  - **(b) ignore the cap** (in `accumulate`, always `append(buf, chunk...)`, never truncate). **Expected: the truncated arm's `truncated == true` AND its 20-byte-prefix `as_string` assertions FIRE** (captures 40 bytes, `truncated: false`). Proves the cap is HONORED.
  - **(c) strict `>` → `>=`** (in `accumulate`). **Expected: ONLY the BOUNDARY arm's `truncated`-flag assertion FIRES** (len==20==C flips `false → true`); the captured payload is UNCHANGED (the prefix that fits is `cap-capturedLen` = the whole 20 bytes regardless of `>`-vs-`>=`), and the full/truncated arms stay green. Per `reference_deliberate_break_wrong_assertion`, CONFIRM the `truncated`-flag check fired, NOT the payload check. This arm JUSTIFIES the boundary POST.
  - **(d) wrong oneof for AS_STRING** (in `bodyProto`, emit `Body_AsBytes` under AS_STRING). **Expected: the `as_string` payload assertions FIRE** (decode to empty). If (d) also perturbs `truncated`, add an isolating variant and confirm WHICH fired.

  Record the LITERAL failing text of each break in `PROGRESS-56.2.md`'s ledger.

- [ ] **Step 4: Full-suite + race gates.** `go test ./test/differential/ -count=1` (watch for the `subject ready: EOF` startup flake on UNRELATED fixtures — isolate-re-run to discriminate, `reference_differential_fullsuite_startup_flake`). If any earlier task added a background goroutine, run the FULL tap package under `-race` (`reference_full_suite_race_after_background_mutator`) — though 56.2 adds none (single-goroutine-per-stream, `buffer` precedent).

- [ ] **Step 5: Gate + commit**

```bash
gofmt -l test/fixtures/0100-http-tap-bodies/ && go vet ./test/...
git add test/fixtures/0100-http-tap-bodies/driver/driver.go docs/envoy-go/phases/56-http-tap-filter/PROGRESS-56.2.md
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T6 0100 AssertStats -- cross-side body payload + truncation + 4 deliberate breaks"
```

---

## Task 7: Docs bundle — ADR-0274 §Decision/§Consequences + `BEHAVIOR_CONTRACT` + `STATE`/`ROADMAP`/`README`/`PROGRESS` (the ADR-0052 atomic landing)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0274 body — §Context is drafted at the SPEC §13).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`.
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/56-http-tap-filter/README.md`, `.../PROGRESS-56.2.md`.

**Interfaces:**
- Consumes: everything landed in Tasks 2-6.
- Produces: the atomic six-gate docs delta; row 56 → `done`.

- [ ] **Step 1: ADR-0274 §Decision + §Consequences** — pick up the §Context DRAFT (SPEC-56.2 §13) and add §Decision (store `format`+caps on `config`; accumulate `DecodeData`/`EncodeData` into filter-owned buffers with strict-`>` truncation and a default-1024 cap; `bodyProto` renders the format→oneof with the AS_STRING `ToValidUTF8` sanitize; `Body` populated IFF the saw-body flag fired; tap returns `DataContinue` always) + §Consequences (the non-UTF8 AS_STRING coverage boundary; the cap-0 empty-but-truncated distinction; `0100` differential + unit AS_BYTES/chunk/sanitize proofs; +0 stat/BackendKind/fuzzer/module). DECISIONS tail → **ADR-0274** (next-free ADR-0275). Per ADR-0044 the body lands at THIS IMPL.

- [ ] **Step 2: `BEHAVIOR_CONTRACT.md` delta** (SPEC §9): the body-capture model (accumulate chunks per direction, honor the cap, populate `Message.body` at stream end on a match); cap semantics (default 1024 unset; strict-`>` truncation; cap-0-nonempty → empty-but-truncated PRESENT vs bodyless → omitted); the `format`→oneof render (`as_string`/`as_bytes` std base64, `truncated` always emitted); the non-UTF8 AS_STRING coverage boundary (sanitize, never silently drop). The 56.1 blocks are UNCHANGED.

- [ ] **Step 3: ROADMAP** — flip **row 56** to `done` (`reference_roadmap_split_phase_row_done` + ADR-0106 — a split phase's row flips `done` once ALL legs land; 56.2 is the final leg). Update the Observability family paragraph if it names in-progress rows. **Do NOT clear the family's deferred-candidate list** (it stays non-empty — the family STAYS OPEN).

- [ ] **Step 4: STATE + README** — STATE: advance the active-phase header, add the 56.2 IMPL history entry. README: mark 56.2 (bodies) IMPL DONE, 56 complete; record the IMPL counts + the break ledger outcome.

- [ ] **Step 5: PROGRESS-56.2** — tick all task checkboxes; fill the deliberate-break ledger with LITERAL failing text; write the Findings log (any vacuous-proof classes caught at the IMPL, per the 56.1 precedent).

- [ ] **Step 6: Reconcile the exit counts** — re-run the Task-1 baseline commands; assert fixtures **102**, fuzzers **53** (+0), DECISIONS tail **ADR-0274**, BackendKind **38** (+0). `go mod tidy -diff` EMPTY. `grep -c '^func Fuzz'` reconcile (`reference_fuzzer_count_docs_drift`).

- [ ] **Step 7: Full verification + commit**

```bash
go build ./... && go test ./... -count=1   # full suite green
gofmt -l . ; go vet ./...
git add docs/ README.md   # (paths as touched)
git commit -m "phase 56.2 (http-tap-filter, bodies leg) IMPL: T7 docs bundle -- ADR-0274 body + BEHAVIOR_CONTRACT + ROADMAP row 56 done + STATE/README/PROGRESS"
```

---

## Final review + handoff

- [ ] **Self-review vs SPEC-56.2** (the writing-plans checklist): every SPEC §10 item maps to a task (§10.1→T2, §10.2→T3, §10.3→T4, §10.4→T5, §10.5→T6, §10.6→T7, +T1 baselines); every §11 empirical pin is honored (default 1024→T2; strict-`>`→T3+T6(c); `truncated` always→T4; AS_BYTES base64→T4; cap-0→T3+T4; chunk reality→T3); all four §12 D-questions are pinned above.
- [ ] **Controller squash + push at stage-close** — squash the seven task commits (fold the `next-prompt.txt` roll into the squash per `reference_next_prompt_tracked_despite_gitignore`), re-run the full suite on the frozen HEAD, push per `feedback_push_to_origin`.
- [ ] **Row 56 → `done`; phase 56 COMPLETE.** The Observability family STAYS OPEN. Roll `next-prompt.txt` forward to the next Observability-family row (or a new subject per the ROADMAP), NOT a 56.3 (there is none — 56.2 is the final leg).

**This plan is docs-only at the PLAN stage.** The IMPL executes the seven tasks above, subagent-driven, in a fresh worktree `.worktrees/phase-56.2-impl`, branch `phase-56.2-http-tap-filter-impl`.
