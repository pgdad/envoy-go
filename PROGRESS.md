# Phase 29.2 IMPL — PROGRESS

Worktree: `.worktrees/phase-29.2-network-filter-mongo-responses-and-correlation-impl`
Branch: `phase-29.2-network-filter-mongo-responses-and-correlation-impl` (off master `46dcd1b`)
Executing: `docs/envoy-go/phases/29.2-network-filter-mongo-responses-and-correlation/PLAN.md` (11 tasks)
Mode: subagent-driven-development (fresh subagent per task; two-stage review between tasks); subagents commit LOCAL-ONLY.

## Task 1 — baselines/anchors gate (DONE)

Confirmed at the IMPL-session tip `46dcd1b`:
- differential fixtures: **52** (tail `test/fixtures/0050-mongo-boot-reject`) → 53 at phase-done
- fuzzers: **39** (extend `FuzzMongoDecode`; no 40th)
- DECISIONS.md tail header: **## ADR-0226** (next-free **ADR-0227**; ADR-0225 body lands in-place at T11)
- BackendKind: `TCPSink = 28`, `TCPZKResponder = 29` (next-free **30** = `TCPMongoResponder`)
- stat surface: **360** (+0 at 29.2)

As-built anchors (codec.go / filter.go / stats.go) verified:
- `decoder` struct (codec.go:46-52): 7 fields; gains `mu`/`writeBuf`/`dynMeta` at T2/T3
- dispatch `opReply, opCommandReply: return true` (recognized-not-decoded, 29.1) present
- `decoderError`/`fail` present (to be CAS-converted at T3)
- `OnWrite` no-op stub present (filter.go); `OnDestroy` = `f.dec = nil`
- `opQueryActive` gauge created eagerly (stats.go)
- `Gauge.{Inc,Dec,Add,Load}` present; `Bucket.Set(filterName, key, *structpb.Value)`; `DynamicMetadata() *dynamicmetadata.Bucket` on ReadFilterCallbacks

Clean baseline: `go build ./...` clean; `go test ./internal/filter/network/mongoproxy/... -count=1` green.

## Task checklist

- [x] T1 — baselines/anchors gate + PROGRESS.md
- [x] T2 — decoder mutex + writeBuf/dynMeta fields + gauge Inc on request append (commit f98af16; both reviews ✅)
- [x] T3 — decodeOnWrite framing + dispatch + sniffing atomic.Bool (CAS at-most-once) (commit 7f3cea5; both reviews ✅; +1-line fuzz_test.go compile fix)
- [x] T4 — OP_REPLY/OP_COMMANDREPLY body decode + 5 response counters (commit 653843f + hardening 20f0e9a; both reviews ✅)
- [x] T5 — correlation takeQuery first-match-erase + gauge Dec on hit (commit c0a4a91; both reviews ✅)
- [x] T6 — OnDestroy residual-drain teardown + lifecycle invariant (commit 4bbe972; both reviews ✅)
- [x] T7 — OnWrite glue + concurrent race test + R9 mutex deliberate-break (commit afc3525 + hardening 44e5d7b; both reviews ✅)
- [x] T8 — emit_dynamic_metadata single-Set Bucket emission (commit 4088090; both reviews ✅)
- [x] T9 — extend FuzzMongoDecode to both directions (commit 73ed351; both reviews ✅; count stays 39 — verified canonical recipe)
- [x] T10 — TCPMongoResponder BackendKind 30 + acceptMongoResponder (commit 8c7f3cf + fix 50ca55b; both reviews ✅)
- [x] T11A — 0051-mongo-responses fixture cross-side GREEN (commit 068851a; both reviews ✅)
- [x] T11B — completion bundle (ADR-0225 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + next-prompt; commit 984147a) + six-gate evidence (8da9af4) + final whole-impl review ✅ (approve; 4 stale-comment cleanups swept in 469934f)

## Per-task evidence log

### T1
- Counts/anchors confirmed (above). Clean baseline green. No code change.

### T2 (commit f98af16)
- Added `mu`/`writeBuf`/`dynMeta` decoder fields + `appendQuery` (lock append, Inc outside lock); replaced 2 request-path append sites. `TestDecoder_GaugeIncsPerActiveQuery` fails→green. gofmt/lint clean; full pkg green under -race.
- Deviation: `//nolint:unused` on `writeBuf`/`dynMeta` (forward-declared; repo precedent lua/compiled_config.go:198). CARRY: drop the nolint when writeBuf gains a consumer (T3) and dynMeta gains a reader (T8).
- Spec review ✅; code-quality review ✅ (approve, no changes).

### T3 (commit 7f3cea5)
- `sniffing` → `atomic.Bool`; `newDecoder` Store(true); `decoderError` CAS at-most-once; readBuf-release relocated to decodeOnData post-loop; per-pass dynMeta clear added (gated, exercised at T8). Added `decodeOnWrite`/`nextWriteMessage`/`decodeResponseMessage` + stub `decodeReply`/`decodeCommandReply`. 4 new write-side tests fail→green. Removed writeBuf nolint; kept dynMeta nolint.
- Necessary deviation: 1-line `d.sniffing`→`.Load()` in fuzz_test.go (atomic.Bool can't be value-copied; required to compile). No fuzzer logic change.
- Spec review ✅; code-quality review ✅ (CAS exactly-once + readBuf-release verified empirically under -race -count=5).

### T4 (commit 653843f + hardening 20f0e9a)
- Replaced stub decodeReply/decodeCommandReply with full body decode. decodeReply: flags/cursorID/startingFrom/numberReturned + N docs → op_reply + cursor_not_found(0x01) + query_failure(0x02) + valid_cursor(cursorID≠0); counters charge ONLY after successful doc-walk; malformed → fail(). decodeCommandReply: metadata+commandReply+0..N outputDocs → op_command_reply; never touches gauge. Correlation deferred to T5 (responseTo unused).
- Adaptation: test `want` map typed `uint64` (Counter.Load→uint64; gauge→int64). Hardening commit added `op_reply==0`/`op_command_reply==0` assertions to the two malformed tests (liveness proven: moving inc above doc-walk fails the assertion).
- Spec review ✅; code-quality review ✅ (approve).

### T8 (commit 4088090)
- `decoder.recordOp(collection, op)` (gated, lazy-init, append) called from decodeQuery both success paths (op="query") + decodeInsert (op="insert", captures previously-discarded fullColl cstring, post-dot token). `filter.emitDynamicMetadata()` builds ONE structpb StructValue (collection→ListValue of op strings) via single Set("envoy.filters.network.mongo_proxy","operations",sv); gated + empty-skip. Wired into OnData after decode feed. Removed dynMeta nolint. ZERO internal/dynamicmetadata/ change (D-S29.2-3).
- Test-harness fix: driveOnData takes a SHARED *network.Buffer (PLAN's fresh-per-call broke the monotonic TotalAppended high-water → pass 2 under-fed). Validated faithful to one-Buffer-per-connection (chain.go rt.buf). Per-pass-clear proof live (driven by decodeOnData d.dynMeta=nil reset).
- Spec review ✅; code-quality review ✅ (structpb confirmed absent from codec; recordOp lock-free goroutine-A-only; nesting + insertion order correct; nil-f.cb deref unreachable per chain.go).

### T11B six-gate evidence (run before docs)
- GATE 1 `go build ./...` → clean.
- GATE 2 `go vet ./...` → clean.
- GATE 3 `golangci-lint run` (full repo) → exit 0.
- GATE 4 `go test ./... -race -short` → 80 packages ok, 0 fail.
- GATE 5 `go test ./test/differential/ -count=1` (full 53-dir) → all mongo fixtures (0049/0050/0051) byte-exact PASS. **Pre-existing environmental flake:** the full-suite run intermittently fails ONE random unrelated HTTP/wasm fixture with `subject ready: EOF` (subject-subprocess startup probe timing). Reproduced IDENTICALLY on MASTER @46dcd1b (→ 0012-http-header-mutation), proving it is NOT a 29.2 regression (29.2 touches zero HTTP code; framework byte-untouched). Branch runs flaked 0019/0025 then 0034 then 0021; ALL pass on isolated/subset re-run. Confirmation run of the 3 mongo + all 5 previously-flaked HTTP fixtures TOGETHER → 8/8 PASS (22s).
- GATE 6 conformance (h2spec 53/53 + proxy-wasm 10/10) → asserted-UNAFFECTED (29.2 touches no HTTP/h2/wasm path; zero code change there; last green 29.1 six-gate 2026-06-04).
- Counts at phase-done: fixtures **53**, fuzzers **39**, stats **360**, BackendKind tail **30**, DECISIONS tail **ADR-0226** (next-free ADR-0227).

### T11A (commit 068851a)
- Fixture `0051-mongo-responses` (single-listener l_resp/mongo_r → TCPMongoResponder). 6 arms (plain round-trip; 3 flag variants 7001/7002/7003; OP_COMMAND round-trip; withhold 7777; uncorrelated 7005; malformed 7004). StatsAsserter cross-side GREEN vs reference v1.37.2 (Docker). Arm-accounting LIVE-VERIFIED.
- **op_query=7 (NOT PLAN's estimate of 6)** — arm6 malformed sends a VALID OP_QUERY request (only the response is malformed). op_reply=5, op_command=1, decoding_error=1, 3 flag counters=1, op_command_reply=1. Gauge op_query_active==0 at rest. cx_destroy_* PRESENCE-ONLY (ref=3/subj=0; D-P4). delays_injected/cx_drain_close present==0.
- Unanswered-gauge: approach (B) — cross-side proves answered→0 + residual-drain→0; ==1-while-open is unit-covered (T6). (Drive* methods get only proxy addr, not admin addr.)
- **R4 deliberate-breaks (-count=1):** (a) op_reply want 6→FAIL; (b) skip Dec→subj gauge=4 want 0 FAIL; (c) skip Inc→subj gauge=-7 want 0 FAIL. All reverted GREEN; codec.go byte-identical. Break (b) re-proven independently by spec reviewer; gauge math (b:+4, c:-7) reproduced by code reviewer.
- Spec review ✅; code-quality review ✅ (approve). Fixture count → 53.

### T9 (commit 73ed351)
- Extended FuzzMongoDecode to feed BOTH decodeOnData + decodeOnWrite over one decoder; 3 response seeds (empty OP_REPLY / OP_COMMANDREPLY / malformed numReturned-lies); 4 invariants (no-panic, input-immutability, direction-shared sniffing-off at-most-once, readBuf+writeBuf bounded). respSeed/replyBodySeed/docSeed wrappers. NO 40th fuzzer — canonical `grep "^func Fuzz" internal/**/fuzz_test.go | wc -l` = **39** confirmed.
- Adaptation: writeBuf bound `len(data)+16`→`2*len(data)+16` (decodeOnWrite fed `data` twice + no high-water → legit 2*len(data) accumulation; not a prod bug). 20s fuzz run clean (~5.4M execs).
- Spec review ✅; code-quality review ✅ (invariant 3 live for write side; immutability valid for shared slice; bound correction sound).

### T10 (commit 8c7f3cf + fix 50ca55b)
- `TCPMongoResponder BackendKind = 30` + `acceptMongoResponder`/`mongoRespondLoop` (16-byte LE MsgHeader framing; correlated OP_REPLY/OP_COMMANDREPLY, responseTo echoes requestID; marker requestIDs: withhold 7777, cursorNotFound 7001, queryFailure 7002, validCursor 7003+cursorID 4242, malformedReply 7004, uncorrelated 7005=reqID+50000). `mongoReqFrame` + `TestMongoResponderBackend`. Dispatch arm mirrors TCPZKResponder. Zero production code.
- **Spec-review bug caught + fixed (50ca55b):** original 7004 emitted `replyBody(0,0,1)` = a WELL-FORMED 1-doc reply (replyBody always appends ndocs docs). Fixed to inline 20-byte body (numberReturned=1, NO doc) → genuinely malformed → decodeReply parseDocument on empty reader → decoding_error, op_reply NOT charged. Verified by throwaway test (decoding_error==1, op_reply==0) both by impl + re-review.
- Spec review ✅ (re-review after fix); code-quality review ✅ (wire frames byte-verified vs codec; inline-malformed the right structural choice vs corrupting replyBody invariant; framing-read hardened). Marker consts mirrored driver-local in T11 (PLAN design).

### T5 (commit c0a4a91)
- `takeQuery(responseTo)` first-match-erase under mu (copy-out by value, order-preserving slice-delete); correlation block in decodeReply Decs gauge OUTSIDE the lock on a hit. 3 correlation tests (first-match-erase decs gauge; uncorrelated miss charges fixed only; command-reply does not correlate). activeQuery.requestID matched verbatim.
- Spec review ✅; code-quality review ✅ (slice-delete stale-tail analyzed benign — bounded by per-conn lifetime + T6 onDestroy drain; command-reply non-correlation assertion is live).

### T6 (commit 4bbe972)
- `decoder.onDestroy()`: snapshot n under mu, `d.queries=nil`, Add(-n) OUTSIDE lock guarded by `if n>0` (idempotent, no negative gauge; D-S29.2-5). `filter.OnDestroy` drains via f.dec.onDestroy() then nils f.dec. 3 tests (residual drain→0; lifecycle inc2/dec1/drain1→0 with list↔gauge invariant; filter-level drain+release).
- Spec review ✅; code-quality review ✅ (gauge Inc/Dec/Add proven a balanced additive group netting 0/conn; nil drops backing array resolving T5 tail-leak; idempotency verified under -race). Non-blocking nits (terminal list assertion covered by sibling test) — no action.

### T7 (commit afc3525 + hardening 44e5d7b)
- `filter.OnWrite` no-op → `f.dec.decodeOnWrite(buf.Bytes()); return Continue` (never halts, never drains; 28.2 no-high-water). Replaced TestFilter_OnWriteIsNoOp with OnWriteFeedsResponseDecoder + OnWriteNeverDrainsChainBuffer. Added TestDecoderConcurrentRequestResponseRace (200 req via decodeOnData goroutine A + 200 resp via decodeOnWrite goroutine B over ONE decoder; asserts op_reply==200, hardened with op_query==200).
- **R9 deliberate-break (-count=1, reference_differential_break_protocol_count1):** commenting out the appendQuery+takeQuery mu.Lock/Unlock → `WARNING: DATA RACE` (appendQuery write codec.go:68 vs takeQuery read codec.go:80 on shared dec.queries) + FAIL under `-race -count=1`. Reverted byte-identical → clean PASS. codec.go untouched in the commit. Re-proven INDEPENDENTLY by the spec reviewer.
- nil-safety traced: f.dec eagerly non-nil; OnWrite fenced behind the pump-join happens-before edge (terminal.Handle returns before onDestroy) → unguarded form (matching OnData) correct.
- Spec review ✅; code-quality review ✅ (approve).
