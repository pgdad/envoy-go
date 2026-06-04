# Phase 29.2 PLAN — `mongo_proxy` response side + correlation + the `op_query_active` gauge

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). After a temporary deliberate break (Task 11 R4 liveness), use `go test -count=1` to defeat result caching (`reference_differential_break_protocol_count1`). Cross-side fixtures MUST use `fixture.StatsAsserter` (`reference_differential_asserter_dispatch`); the responder backend MUST emit CORRELATED bytes whose `responseTo` echoes the request `requestID` (`reference_wire_format_both_sides_see_same_bytes`). Wire-derived dynamic stat segments stay guarded by `stats.IsValidName` (`reference_dynamic_stat_name_charset_guard`) — already in place from 29.1, untouched here.

**Goal:** Complete `mongo_proxy`'s round-trip observability — replace the 29.1 `OnWrite` no-op with a RESPONSE-side decoder (OP_REPLY/OP_COMMANDREPLY body decode + the 5 response counters), correlate replies to the 29.1 active-query list (requestID↔responseTo first-match-erase) under a per-connection `sync.Mutex` (ADR-0223), drive the project's FIRST differentially-mirrored gauge (`op_query_active` inc/dec), emit the `emit_dynamic_metadata` Struct onto the ADR-0217 Bucket, and prove it cross-side with fixture `0051-mongo-responses` (the new `TCPMongoResponder` backend, BackendKind 30; gauge quiesced-point arms) — at ZERO framework touch and +0 stat creation (all 23 stats were created eagerly at 29.1).

**Architecture:** A `mongoproxy`-package + test-surface change ONLY (the 28.2 zero-touch shape). The 29.1 per-connection `decoder` gains a write-side private reassembly buffer (`writeBuf`) + a `sync.Mutex` guarding EXACTLY the active-query list (`dec.queries`); `sniffing` becomes an `atomic.Bool` (D-S29.2-4) so the at-most-once `decoding_error` path is race-clean across both pumps. `OnWrite` feeds `decodeOnWrite` (goroutine B); `OnData` still feeds `decodeOnData` (goroutine A). Correlation Decs the gauge on a first-match-erase hit; `OnDestroy` drains the residual list and Decs the gauge per entry (gauge → 0 at connection end). `emit_dynamic_metadata` accumulates this pass's collection→ops during the request decode and the filter writes a single StructValue to the per-connection Bucket (the §3.7 single-Set model — D-S29.2-3, zero `internal/dynamicmetadata/` change). `cx_destroy_*_with_active_rq` stay exist-at-zero (D-P4 close-direction coverage boundary → 29.3). ZERO changes to `internal/filter/network/` framework files, `manager.go`, `tcp_proxy`, HCM, or `internal/stats/`.

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Extends the as-built `internal/filter/network/mongoproxy/` package (29.1) IN PLACE; consumes `internal/stats/` (`Gauge` Inc/Dec/Add — 06.1) + `internal/dynamicmetadata/` (the ADR-0217 Bucket — `Set` only, no code change) + the differential harness + `fixture.StatsAsserter` + the `0049` label-aware scrape helpers (`scrapeMongoStats`/`scrapeTypeLine` — reused verbatim). ZERO new third-party `go.mod` dependencies.

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §12.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **11 tasks** / **~330–540 production LoC** (the SPEC §12.1 envelope, re-confirmed at PLAN time on the 26.x/28.2 accounting basis — fixture drivers + unit tests excluded):

| Deliverable | Production LoC | Tasks |
|---|---|---|
| `codec.go` response path (`decodeOnWrite`/`nextWriteMessage`/`decodeResponseMessage`/`decodeReply`/`decodeCommandReply`/`takeQuery`/`onDestroy`) | ~140–200 | 3–7 |
| `codec.go`/`filter.go` struct fields + the mutex/gauge wiring on the 29.1 request paths | ~30–60 | 2 |
| `filter.go` (the OnWrite feed + the OnDestroy gauge teardown) | ~20–40 | 6–7 |
| `emit_dynamic_metadata` emission (single-Set model; per-pass accumulate + clear) | ~50–90 | 8 |
| the fuzzer extend (response-direction reach + the mutex race test) | ~20–40 | 9 |
| `TCPMongoResponder` BackendKind + runner arm (test-side; counted for completeness) | ~70–110 | 10 |
| **Total (production basis)** | **~330–540** | **11** |

Both axes well under the gate (11 ≤ ~25 tasks; ~540 ≤ ~1500 LoC) → **NO split. 29.2 proceeds as ONE sub-phase** (the 28.2 single-sub-phase precedent; comparable to 28.2's ~360–490). The `0051` driver (~600–800 LoC; the `0048`/`0049` precedents 865/887) is excluded per the accounting precedent (excluded from the gate either way). No pre-authorized split axis exists for 29.2.

## PLAN-time D-question dispositions (SPEC §10.2)

- **D-S29.2-4 (the `sniffing` flag synchronization) — RESOLVED at PLAN: `sniffing atomic.Bool`.** The connection-lifetime sniffing flag is touched on BOTH pumps once `decodeOnWrite` exists (goroutine A on the request path; goroutine B on the response path). The PLAN converts the 29.1 `sniffing bool` to `atomic.Bool` (Task 3) and folds the at-most-once `decoding_error` charge into a single `CompareAndSwap(true, false)` in `decoderError()`. This keeps the per-connection `sync.Mutex` STRICTLY on `dec.queries` (cleaner than the SPEC §3.5 "conservative" mu-for-sniffing sketch — the read path never needs the lock) and is race-clean by construction (atomic Load/Store; the CAS makes the counter charge exactly-once even under a simultaneous both-pump error). The `-race -count=5` test (Task 7) is the proof.
- **D-S29.2-5 (the residual-list drain at OnDestroy) — RESOLVED at PLAN: snapshot-count-under-lock, Dec-by-count outside the lock.** `decoder.onDestroy()` takes `mu`, records `n := len(d.queries)`, clears the slice, releases `mu`, then `opQueryActive.Add(int64(-n))` (gauge math outside the critical section — the ADR-0223 discipline). Tested at Task 6.
- **D-S29.2-3 (dynamic-metadata Bucket model) — RESOLVED at PLAN: the single-Set StructValue model (SPEC-PREFERRED option (a)).** Zero `internal/dynamicmetadata/` change: the whole mongo namespace metadata is ONE `*structpb.Value` (a `StructValue` whose fields ARE the collection names → each a `ListValue` of op strings) written under one conventional key via a single `Bucket.Set("envoy.filters.network.mongo_proxy", "operations", sv)` per request pass. Per-pass clear is FREE (the next emitting pass overwrites the single value); an empty pass skips the `Set` (SPEC §3.7 "or skip the `Set` if empty"). The `"operations"` wrapper key is a unit-test-asserted internal detail (differential-invisible — no cross-side surface, no in-repo consumer).
- **The decoder accumulates; the filter emits (refines the SPEC §3.7 emission seam).** Keeping the decoder unit-testable in isolation (the 29.1 `dec.queries`-on-the-decoder precedent), the decoder accumulates this pass's `collection → []op` into a `dec.dynMeta map[string][]string` field (gated by `cfg.emitDynamicMetadata`, reset at the top of each `decodeOnData` pass); the FILTER (which owns the `network.ReadFilterCallbacks` → the Bucket) reads `f.dec.dynMeta` after `decodeOnData` and performs the single `Set`. `structpb` stays out of the decoder. The metadata collection key is the post-dot collection token (the same token the `collection.<c>.*` stats use) — differential-invisible (D-P11), pinned for internal consistency.
- **The mutex lives ON THE DECODER (`dec.mu`), guarding EXACTLY `dec.queries`.** The 29.1 `codec.go:42-45` forward-pointer marks the site. `appendQuery` (request path) + `takeQuery` (response path) + `onDestroy` (teardown) are the only three lockers; entries are copied OUT by value under the lock; gauge `Inc`/`Dec` + counter increments run OUTSIDE the lock (the co-located `Inc` at append is atomic and harmless). Removing `mu` MUST trip `-race` (Task 7 — R9).
- **No write-side high-water mark (the 28.2 asymmetry; ADR-0223 §Decision item 1).** `writeChainConn.Write` allocates a FRESH per-`Write` `*Buffer`, so every upstream→downstream byte arrives EXACTLY ONCE as its own `buf.Bytes()` slice. `decodeOnWrite` appends `buf.Bytes()` directly to `writeBuf` — NO `chainConsumed`-style tracking on the write side.
- **IMPL-owned D-questions left to their tasks:** D-S29.2-1 (response reply-frame byte minimums vs upstream `codec_impl.cc`/`bson_impl.cc` v1.37.2 — Tasks 4 + 10; the minimal valid empty OP_REPLY = responseFlags(0) + cursorID(0) + startingFrom(0) + numberReturned(0) + zero docs = 20 body bytes; the minimal OP_COMMANDREPLY = empty metadata BSON (5 bytes) + empty commandReply BSON (5 bytes) + zero outputDocs = 10 body bytes); D-S29.2-2 (OP_GET_MORE reply disposition — Task 10; anticipated: the responder MAY reply, the reply is uncorrelated [GetMore created no active-query entry] → the §3.3 miss path → fixed counters only; the load-bearing arms use OP_QUERY/OP_COMMAND).

---

## File Structure

**Created:**
- `test/fixtures/0051-mongo-responses/driver/driver.go` — the cross-side response-side label-aware `StatsAsserter` fixture (the `TCPMongoResponder` backend; the 9 arms incl. the gauge quiesced-point arms; `cx_destroy_*` presence-only). The `0049` driver is the structural template (bootstrap render + `scrapeMongoStats`/`scrapeTypeLine`/`canonicalize`/`httpGet` reused verbatim).
- `test/fixtures/0051-mongo-responses/README.md` — the fixture envelope + the R4 deliberate-break record + the "no dynamic-metadata fixture surface" note.
- `PROGRESS.md` (worktree root, Task 1) — the per-task six-gate evidence log (run honestly; the R4 breaks recorded with `-count=1` output).

**Modified:**
- `internal/filter/network/mongoproxy/codec.go` — `decoder` gains `writeBuf []byte` + `mu sync.Mutex` + `dynMeta map[string][]string`; `sniffing bool` → `sniffing atomic.Bool`; `decoderError` → CAS; `appendQuery`/`takeQuery`/`onDestroy` helpers; `decodeOnWrite`/`nextWriteMessage`/`decodeResponseMessage`/`decodeReply`/`decodeCommandReply`; the `opReply`/`opCommandReply` write-side dispatch arm; the request-path list-append sites move under `mu` + Inc the gauge + record dynamic metadata.
- `internal/filter/network/mongoproxy/codec_test.go` — response-decode + correlation + gauge + dynamic-metadata-accumulation unit tests + the `-race` cross-goroutine test + the response wire-builder test helpers (`respMsg`/`opReplyBody`/`opCommandReplyBody`); mechanical `sniffing` → `.Load()` read updates.
- `internal/filter/network/mongoproxy/filter.go` — `OnWrite` no-op → the `decodeOnWrite` feed; `OnData` gains the dynamic-metadata emit call; `OnDestroy` drains the residual list + Decs the gauge; the `emitDynamicMetadata` helper + the namespace/key consts.
- `internal/filter/network/mongoproxy/filter_test.go` — the OnWrite-feeds-response-decoder test (replaces `TestFilter_OnWriteIsNoOp`); the OnDestroy-gauge-teardown test; the dynamic-metadata emit test (gated on/off) with a fake callbacks + Bucket.
- `internal/filter/network/mongoproxy/fuzz_test.go` — `FuzzMongoDecode` EXTENDED to feed both `decodeOnData` AND `decodeOnWrite`; mechanical `sniffing` → `.Load()`.
- `internal/filter/network/mongoproxy/doc.go` — the package-doc 29.2/29.3 forward-pointers (response side LANDED; fault/drain/close-direction → 29.3).
- `test/differential/fixture/fixture.go` — `TCPMongoResponder BackendKind = 30` (after `TCPZKResponder = 29`).
- `test/differential/runner_test.go` — the `TCPMongoResponder` backend dispatch arm (`acceptMongoResponder` + `mongoRespondLoop`) + the `0051` driver blank-import + a `TestMongoResponderBackend` unit test.
- `docs/envoy-go/DECISIONS.md` — ADR-0225 §Decision/§Consequences body IN PLACE (no new ADR number; the §Context D-P4 AMEND already landed at the SPEC commit).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Task 11).

**Untouched (pinned — the §4 zero-touch property; a regression gate):** `internal/filter/network/` chain.go / readconn.go / writeconn.go / types.go / callbacks.go / terminal.go / registry.go; `internal/listener/manager.go`; `internal/filter/tcpproxy/`; `internal/filter/hcm/`; `internal/stats/` (gauge.go / prom.go / name.go / registry.go — the gauge primitive + the four-rule arm are consumed, NOT modified); `internal/dynamicmetadata/` (the single-Set model needs zero change — D-S29.2-3); `internal/bootstrap/bootstrap.go`; `internal/filter/network/builtins/` (mongoproxy already registered at 29.1); `internal/filter/network/mongoproxy/{config,stats,bson,mongoproxy}.go` (no parse/roster/BSON change — the response decode reuses `bsonReader`/`parseDocument` verbatim and calls the existing eager counters + gauge).

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
ls -d test/fixtures/[0-9]* | wc -l            # expect 52; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0050-mongo-boot-reject
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 39
grep -nE "^## ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | tail -1       # expect ## ADR-0226
grep -n "BackendKind = 29\|BackendKind = 28" test/differential/fixture/fixture.go  # TCPSink=28, TCPZKResponder=29
```
Expected: fixtures **52** (tail `0050-mongo-boot-reject`); fuzzers **39**; DECISIONS.md tail-ADR-header **ADR-0226** (next-free **ADR-0227**; the ADR-0227 grep matches elsewhere are "next-free" text, NOT a header — assert against the `^## ADR-` header recipe); `TCPSink = 28`, `TCPZKResponder = 29` (next-free BackendKind **30**). 29.2 lands `0051` → **53**, the EXTENDED `FuzzMongoDecode` (still **39**), `TCPMongoResponder = 30`, and the ADR-0225 §Decision/§Consequences body IN PLACE (no new ADR number consumed).

- [ ] **Step 2: Re-confirm the stat surface = 360**

The count STATE.md / BEHAVIOR_CONTRACT.md report as **360** (the 29.1 `337 → 360` extension; `BEHAVIOR_CONTRACT.md` stat-table). Do NOT invent a new recipe. Expected: **360**. 29.2 lands **+0** (all 23 mongo stats created eagerly at 29.1; the 5 response counters + the gauge go increment-active; the 2 `cx_destroy_*` stay exist-at-zero) → stays **360** at Task 11.

- [ ] **Step 3: Re-confirm the as-built anchors (§9.1) the response path extends**

```bash
sed -n '46,53p;107,132p;137,147p' internal/filter/network/mongoproxy/codec.go   # decoder struct; decodeMessage dispatch (incl. the opReply/opCommandReply recognized-not-decoded arm); decoderError/fail
sed -n '63,68p;75,77p' internal/filter/network/mongoproxy/filter.go             # the OnWrite no-op stub to replace; OnDestroy
sed -n '40,51p' internal/filter/network/mongoproxy/stats.go                      # the eager roster incl. opQueryActive
grep -n "func (g \*Gauge)" internal/stats/gauge.go                              # Inc/Dec/Add/Load (atomic.Int64)
grep -n "func (b \*Bucket) Set" internal/dynamicmetadata/dynamicmetadata.go     # Set(filterName, key, *structpb.Value)
grep -n "DynamicMetadata()" internal/filter/network/callbacks.go                # the ReadFilterCallbacks accessor
```
Expected: the `decoder` struct at `codec.go:46-53` (the 29.2 mutex forward-pointer comment present); the `opReply, opCommandReply: return true` recognized-not-decoded arm at `codec.go:122-125`; `decoderError`/`fail` at `codec.go:137-147`; the `OnWrite` no-op stub at `filter.go:63-68`; `OnDestroy` at `filter.go:75-77`; `opQueryActive *stats.Gauge` created at `stats.go:49`; `Gauge.{Inc,Dec,Add,Load}` present; `Bucket.Set(filterName, key string, value *structpb.Value)`; `DynamicMetadata() *dynamicmetadata.Bucket` on `ReadFilterCallbacks`.

- [ ] **Step 4: Confirm the clean baseline + record in PROGRESS.md**

```bash
go build ./... && go test ./internal/filter/network/mongoproxy/... -count=1
```
Expected: build clean; all mongoproxy tests green. Create `PROGRESS.md` at the worktree root with the confirmed counts (52 / 39 / 360 / ADR-0226 / BackendKind 30-next-free) + a per-task checklist. **No commit** (no code change) — or an optional `docs: 29.2 IMPL PROGRESS.md baseline gate` commit if the controller wants the log tracked.

---

## Task 2: `decoder` struct fields + the active-query-list mutex + the gauge `Inc` on the request path

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (the `decoder` struct; the two request-path append sites)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

This task adds the cross-goroutine state fields and moves the 29.1 list-append under the mutex, co-locating the gauge `Inc`. `sniffing` STAYS a plain `bool` here (still single-goroutine until `decodeOnWrite` lands at Task 3); the `atomic.Bool` conversion is Task 3. All existing 29.1 tests stay green.

- [ ] **Step 1: Write the failing test (gauge Inc rides the append)**

Add to `codec_test.go`:
```go
func TestDecoder_GaugeIncsPerActiveQuery(t *testing.T) {
	// Each decoded OP_QUERY appends to the active-query list AND Incs the gauge;
	// the list-size↔gauge invariant holds on the request-only path (§3.4).
	d, ms := newTestDecoder(t)
	f1 := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	f2 := msg(2, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	both := append(f1, f2...)
	d.decodeOnData(both, int64(len(both)))
	if got := ms.opQueryActive.Load(); got != 2 {
		t.Errorf("op_query_active = %d, want 2 (one Inc per active query)", got)
	}
	if len(d.queries) != 2 {
		t.Errorf("active-query list = %d, want 2 (gauge must track list size)", len(d.queries))
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestDecoder_GaugeIncsPerActiveQuery -count=1`
Expected: FAIL — `op_query_active = 0, want 2` (no gauge `Inc` wired yet).

- [ ] **Step 3: Add the struct fields + the `appendQuery` helper**

In `codec.go`, add `"sync"` to the import block. Extend the `decoder` struct (keep the existing fields; ADD the three new ones + the forward-pointer comment update):
```go
type decoder struct {
	cfg           *compiledConfig
	stats         *mongoStats
	chainConsumed int64
	readBuf       []byte
	sniffing      bool // starts true; set false on the first decode error (lifetime). → atomic.Bool at Task 3.
	queries       []activeQuery
	// 29.2 cross-goroutine state:
	mu       sync.Mutex          // guards EXACTLY queries (append/take/drain). ADR-0223 — narrowed to one list.
	writeBuf []byte              // goroutine-B-only response reassembly buffer (no high-water mark — 28.2 asymmetry).
	dynMeta  map[string][]string // this-pass collection→ops accumulator (emit_dynamic_metadata; Task 8).
}
```
Add the locker helper (the ONLY request-path writer of `queries`):
```go
// appendQuery records a decoded OP_QUERY under mu and Incs the op_query_active
// gauge (the list-size↔gauge invariant; §3.4). The append + Inc are co-located
// under the request-path lock; the atomic Inc is lock-free but kept here so the
// invariant holds at every quiesced point. The pre-handoff request path takes
// the lock too (uniformity over cleverness — ADR-0223 §Decision item 3).
func (d *decoder) appendQuery(aq activeQuery) {
	d.mu.Lock()
	d.queries = append(d.queries, aq)
	d.mu.Unlock()
	d.stats.opQueryActive.Inc()
}
```

- [ ] **Step 4: Replace the two 29.1 append sites with `appendQuery`**

In `decodeQuery`, replace the `$cmd` success append (`codec.go:225`) `d.queries = append(d.queries, aq)` → `d.appendQuery(aq)`, and the non-command success append (`codec.go:265`) `d.queries = append(d.queries, aq)` → `d.appendQuery(aq)`.

- [ ] **Step 5: Run the new test + the full 29.1 suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — `TestDecoder_GaugeIncsPerActiveQuery` green; ALL existing 29.1 codec/filter/bson/stats/config tests still green (the append sites behave identically; the gauge Inc is additive and untested by the 29.1 suite).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.2 Task 2: decoder mutex + writeBuf/dynMeta fields + op_query_active gauge Inc on the request append"
```
Expected: `gofmt -l` prints nothing; lint clean.

---

## Task 3: Response framing + dispatch + the race-clean `sniffing` (atomic.Bool)

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go`
- Test: `internal/filter/network/mongoproxy/codec_test.go`

Lands `decodeOnWrite` + `nextWriteMessage` + `decodeResponseMessage` + the `opReply`/`opCommandReply` write-side dispatch (bodies stubbed to a bare consume in this task; the body decode + counters land at Task 4), and converts `sniffing` to `atomic.Bool` with a CAS-based at-most-once `decoderError` (D-S29.2-4) so the shared decode-error path is race-clean across both pumps.

- [ ] **Step 1: Write the failing tests (write-side framing + direction-shared sniffing-off)**

Add the response wire-builder helpers + tests to `codec_test.go`:
```go
// respMsg builds a response wire message with an EXPLICIT responseTo (the msg()
// helper hardcodes responseTo=0; the response path correlates on it).
func respMsg(reqID, responseTo, opCode int32, body []byte) []byte {
	total := int32(16 + len(body))
	out := append(leI32(total), leI32(reqID)...)
	out = append(out, leI32(responseTo)...)
	out = append(out, leI32(opCode)...)
	return append(out, body...)
}

// opReplyBody: responseFlags(int32) + cursorID(int64) + startingFrom(int32) +
// numberReturned(int32) + numberReturned BSON docs.
func opReplyBody(flags int32, cursorID int64, docs ...[]byte) []byte {
	out := append(leI32(flags), leI64(cursorID)...)
	out = append(out, leI32(0)...)                // startingFrom
	out = append(out, leI32(int32(len(docs)))...) // numberReturned
	for _, dc := range docs {
		out = append(out, dc...)
	}
	return out
}

func TestDecodeOnWrite_PartialFrameReassembly(t *testing.T) {
	d, ms := newTestDecoder(t)
	full := respMsg(7, 1, 1, opReplyBody(0, 0)) // a minimal empty OP_REPLY (responseTo 1)
	// Feed the first 10 bytes (partial header) — nothing decoded.
	d.decodeOnWrite(full[:10])
	if ms.counters["op_reply"].Load() != 0 {
		t.Fatalf("op_reply fired on a partial write frame")
	}
	// Feed the rest (cumulative is NOT used on the write side — fresh per-Write
	// buffers; feed only the remaining bytes).
	d.decodeOnWrite(full[10:])
	if ms.counters["op_reply"].Load() != 1 {
		t.Errorf("op_reply = %d after full write frame, want 1", ms.counters["op_reply"].Load())
	}
}

func TestDecodeOnWrite_ShortMessageLengthIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	bad := append(leI32(8), make([]byte, 12)...) // messageLength 8 < 16
	d.decodeOnWrite(bad)
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a messageLength < 16 on the write side must be a decoding_error")
	}
}

func TestDecodeOnWrite_UnexpectedOpcodeIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// A request opcode (OP_QUERY 2004) on the RESPONSE stream is malformed.
	d.decodeOnWrite(respMsg(1, 0, 2004, nil))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a non-reply opcode on the write side must be a decoding_error")
	}
}

func TestDecoder_SniffingOffIsDirectionShared(t *testing.T) {
	// An error on the READ side turns sniffing off for the connection; a
	// subsequent WRITE-side frame then decodes NOTHING (AMEND-B6 direction-shared).
	d, ms := newTestDecoder(t)
	d.decodeOnData(msg(1, 2013, nil), int64(len(msg(1, 2013, nil)))) // OP_MSG → error
	if ms.counters["decoding_error"].Load() != 1 || d.sniffing.Load() {
		t.Fatalf("read-side error did not turn sniffing off")
	}
	d.decodeOnWrite(respMsg(7, 1, 1, opReplyBody(0, 0))) // a valid reply, but sniffing is off
	if ms.counters["op_reply"].Load() != 0 {
		t.Errorf("op_reply must stay 0 — sniffing is off for the connection (direction-shared)")
	}
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("decoding_error must stay 1 (at-most-once across both directions)")
	}
}
```

- [ ] **Step 2: Run them to verify they fail / don't compile**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecodeOnWrite|TestDecoder_SniffingOff' -count=1`
Expected: COMPILE FAIL — `d.decodeOnWrite` undefined; `d.sniffing.Load` undefined (sniffing is a `bool`).

- [ ] **Step 3: Convert `sniffing` to `atomic.Bool` + CAS `decoderError`**

In `codec.go` change the import `"sync"` block to add `"sync/atomic"`. Change the struct field:
```go
	sniffing atomic.Bool // starts true; CAS true→false on the first decode error (connection lifetime, direction-shared).
```
Update `newDecoder`:
```go
func newDecoder(cfg *compiledConfig, ms *mongoStats) *decoder {
	d := &decoder{cfg: cfg, stats: ms}
	d.sniffing.Store(true)
	return d
}
```
Replace `decoderError` + `fail` with the race-clean CAS form (the at-most-once charge is exactly-once even under a simultaneous both-pump error; buffer release moves to each direction's loop so goroutine B never touches `readBuf`):
```go
// decoderError charges decoding_error AT MOST ONCE per connection and turns
// sniffing off for the connection lifetime (direction-shared; D-S29.1-6 /
// AMEND-B6). The CompareAndSwap makes the charge exactly-once even if BOTH pumps
// hit a decode error simultaneously (29.2 — sniffing is now cross-goroutine).
// Buffer release is direction-local (each loop nils its OWN buffer when sniffing
// goes off), so goroutine B never writes goroutine A's readBuf.
func (d *decoder) decoderError() {
	if d.sniffing.CompareAndSwap(true, false) {
		d.stats.inc("decoding_error")
	}
}

// fail is the codec's error shorthand: take the decoding_error path, return false.
func (d *decoder) fail() bool { d.decoderError(); return false }
```
Update `decodeOnData` to use `d.sniffing.Load()` and release `readBuf` after the loop when sniffing went off (replacing the 29.1 in-`decoderError` readBuf release):
```go
func (d *decoder) decodeOnData(chainBytes []byte, totalAppended int64) {
	if !d.sniffing.Load() {
		d.chainConsumed = totalAppended
		d.readBuf = nil
		return
	}
	if d.cfg.emitDynamicMetadata { // per-pass clear (Task 8 reads it; harmless when nil)
		d.dynMeta = nil
	}
	if newCount := totalAppended - d.chainConsumed; newCount > 0 {
		d.readBuf = append(d.readBuf, chainBytes[int64(len(chainBytes))-newCount:]...)
		d.chainConsumed = totalAppended
	}
	for {
		m, ok := d.nextMessage()
		if !ok {
			break
		}
		if !d.decodeMessage(m) {
			break
		}
	}
	if !d.sniffing.Load() {
		d.readBuf = nil
	}
}
```
> NOTE: `nextMessage` keeps calling `d.decoderError()` on a `msgLen < 16`; it no longer needs to release `readBuf` itself (the post-loop release handles it). Leave `nextMessage` otherwise unchanged.

- [ ] **Step 4: Add `decodeOnWrite` + `nextWriteMessage` + `decodeResponseMessage`**

Append to `codec.go`:
```go
// decodeOnWrite feeds the response decoder the write-direction (upstream→
// downstream) bytes. writeChainConn.Write allocates a FRESH *Buffer per Write
// (ADR-0221), so p arrives exactly once — appended directly to writeBuf with NO
// high-water mark (the 28.2 structural asymmetry; ADR-0225). NEVER drains the
// chain buffer, never closes, never halts (R3 extended to the write side).
func (d *decoder) decodeOnWrite(p []byte) {
	if !d.sniffing.Load() {
		d.writeBuf = nil
		return
	}
	d.writeBuf = append(d.writeBuf, p...)
	for {
		m, ok := d.nextWriteMessage()
		if !ok {
			break
		}
		if !d.decodeResponseMessage(m) {
			break
		}
	}
	if !d.sniffing.Load() {
		d.writeBuf = nil
	}
}

// nextWriteMessage extracts one complete wire message from writeBuf (the
// nextMessage shape over the write-side buffer). Partial frame → wait; a
// messageLength < 16 → decode error.
func (d *decoder) nextWriteMessage() ([]byte, bool) {
	if len(d.writeBuf) < 16 {
		return nil, false
	}
	msgLen := int32(binary.LittleEndian.Uint32(d.writeBuf[0:4]))
	if msgLen < 16 {
		d.decoderError()
		return nil, false
	}
	if int64(len(d.writeBuf)) < int64(msgLen) {
		return nil, false // partial frame — wait for more bytes
	}
	m := d.writeBuf[:msgLen]
	d.writeBuf = d.writeBuf[msgLen:]
	return m, true
}

// decodeResponseMessage parses the MsgHeader of a response frame and dispatches
// by opcode. responseTo (m[8:12]) drives correlation (§3.3). Only OP_REPLY(1) and
// OP_COMMANDREPLY(2011) are valid on the response stream; any other opcode is a
// decoding_error (the request side's default-throw parity). Returns false on a
// decode failure (the decoding_error path has already run).
func (d *decoder) decodeResponseMessage(m []byte) bool {
	responseTo := int32(binary.LittleEndian.Uint32(m[8:12]))
	opCode := int32(binary.LittleEndian.Uint32(m[12:16]))
	body := m[16:]
	switch opCode {
	case opReply:
		return d.decodeReply(responseTo, body)
	case opCommandReply:
		return d.decodeCommandReply(body)
	default:
		d.decoderError()
		return false
	}
}

// decodeReply / decodeCommandReply land at Task 4. STUB (this task) — consume the
// frame as recognized-not-yet-decoded so the framing/dispatch tests pass without
// the body decode. Replaced wholesale at Task 4.
func (d *decoder) decodeReply(responseTo int32, body []byte) bool {
	d.stats.inc("op_reply")
	return true
}
func (d *decoder) decodeCommandReply(body []byte) bool {
	d.stats.inc("op_command_reply")
	return true
}
```
> The Task-3 `decodeReply` stub increments only `op_reply` (enough for the framing tests, which feed an empty reply). Task 4 replaces both functions with the full body decode + the flag/cursor counters + (Task 5) correlation. `responseTo` is an unused param in the stub — Go permits unused function parameters, so this compiles.

- [ ] **Step 5: Mechanical `sniffing` read updates in existing tests**

In `codec_test.go`, update the two direct `d.sniffing` reads:
- `TestCodec_OpMsgIsDecodingError`: `if d.sniffing {` → `if d.sniffing.Load() {`
- `TestCodec_ReplyAndCommandReplyRecognizedNotDecoded`: `if !d.sniffing {` → `if !d.sniffing.Load() {`

(The `fuzz_test.go` `d.sniffing` read is updated at Task 9.)

- [ ] **Step 6: Run the response framing tests + the full suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — the four new write-side tests green; ALL 29.1 tests green (the `sniffing` atomic conversion + the readBuf-release relocation are behavior-preserving: `decoding_error` still at-most-once, `readBuf` still released on error).

- [ ] **Step 7: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.2 Task 3: decodeOnWrite framing + response dispatch + sniffing atomic.Bool (CAS at-most-once decoding_error)"
```

---

## Task 4: OP_REPLY + OP_COMMANDREPLY body decode + the 5 response counters

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (replace the Task-3 `decodeReply`/`decodeCommandReply` stubs)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

- [ ] **Step 1: Write the failing tests (each flag bit; valid_cursor; doc walk; OP_COMMANDREPLY; malformed)**

Add to `codec_test.go`:
```go
func TestDecodeReply_Counters(t *testing.T) {
	cases := []struct {
		name     string
		flags    int32
		cursorID int64
		ndocs    int
		want     map[string]int64
	}{
		{"plain-empty", 0, 0, 0, map[string]int64{"op_reply": 1}},
		{"cursor-not-found", 0x01, 0, 0, map[string]int64{"op_reply": 1, "op_reply_cursor_not_found": 1}},
		{"query-failure", 0x02, 0, 0, map[string]int64{"op_reply": 1, "op_reply_query_failure": 1}},
		{"valid-cursor", 0, 42, 1, map[string]int64{"op_reply": 1, "op_reply_valid_cursor": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ms := newTestDecoder(t)
			docs := make([][]byte, tc.ndocs)
			for i := range docs {
				docs[i] = simpleQuery()
			}
			d.decodeOnWrite(respMsg(7, 0, 1, opReplyBody(tc.flags, tc.cursorID, docs...)))
			for suf, want := range tc.want {
				if got := ms.counters[suf].Load(); got != want {
					t.Errorf("%s = %d, want %d", suf, got, want)
				}
			}
		})
	}
}

func TestDecodeReply_MalformedBodyIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	// numberReturned claims 1 doc, but the body carries a truncated BSON doc.
	body := append(leI32(0), leI64(0)...) // flags + cursorID
	body = append(body, leI32(0)...)      // startingFrom
	body = append(body, leI32(1)...)      // numberReturned = 1
	body = append(body, leI32(99)...)     // a doc claiming 99 bytes, none follow
	d.decodeOnWrite(respMsg(7, 0, 1, body))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_REPLY doc must be a decoding_error")
	}
}

func opCommandReplyBody(outputDocs ...[]byte) []byte {
	out := append(bsonDocEmpty(), bsonDocEmpty()...) // metadata + commandReply (both empty docs)
	for _, dc := range outputDocs {
		out = append(out, dc...)
	}
	return out
}

// bsonDocEmpty is a 5-byte empty BSON document {len=5}{0x00}.
func bsonDocEmpty() []byte { return doc() }

func TestDecodeCommandReply_Counter(t *testing.T) {
	d, ms := newTestDecoder(t)
	d.decodeOnWrite(respMsg(7, 0, 2011, opCommandReplyBody()))
	if ms.counters["op_command_reply"].Load() != 1 {
		t.Errorf("op_command_reply = %d, want 1", ms.counters["op_command_reply"].Load())
	}
	// OP_COMMANDREPLY does NOT touch the gauge (no correlation).
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("OP_COMMANDREPLY must not touch the gauge")
	}
}

func TestDecodeCommandReply_MalformedIsError(t *testing.T) {
	d, ms := newTestDecoder(t)
	body := append(leI32(99), make([]byte, 4)...) // metadata claims 99 bytes, none follow
	d.decodeOnWrite(respMsg(7, 0, 2011, body))
	if ms.counters["decoding_error"].Load() != 1 {
		t.Errorf("a malformed OP_COMMANDREPLY must be a decoding_error")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecodeReply|TestDecodeCommandReply' -count=1`
Expected: FAIL — the Task-3 stubs increment `op_reply`/`op_command_reply` unconditionally but never decode the body, so the flag/cursor counters stay 0 and the malformed-body cases do NOT error (`op_reply` fires on garbage).

- [ ] **Step 3: Replace the Task-3 stubs with the full body decode**

In `codec.go`, replace the two stub functions:
```go
// decodeReply decodes an OP_REPLY body (parent §11.4): responseFlags(int32) +
// cursorID(int64) + startingFrom(int32) + numberReturned(int32) + exactly
// numberReturned BSON documents. Charges op_reply + the flag/cursor counters; a
// malformed body → decoding_error. Correlation (§3.3) lands at Task 5.
func (d *decoder) decodeReply(responseTo int32, body []byte) bool {
	r := &bsonReader{buf: body}
	flags, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	cursorID, err := r.readInt64()
	if err != nil {
		return d.fail()
	}
	if _, err := r.readInt32(); err != nil { // startingFrom
		return d.fail()
	}
	numReturned, err := r.readInt32()
	if err != nil {
		return d.fail()
	}
	for i := int32(0); i < numReturned; i++ {
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_reply")
	if flags&0x01 != 0 {
		d.stats.inc("op_reply_cursor_not_found")
	}
	if flags&0x02 != 0 {
		d.stats.inc("op_reply_query_failure")
	}
	if cursorID != 0 {
		d.stats.inc("op_reply_valid_cursor")
	}
	return true
}

// decodeCommandReply decodes an OP_COMMANDREPLY body (parent §11.4):
// metadata(BSON) + commandReply(BSON) + 0..N outputDocs(BSON, loop to end of
// body). Charges op_command_reply. Does NOT correlate + does NOT touch the gauge
// (only OP_REPLY correlates against the active-query list — parent §11.4 item 7).
func (d *decoder) decodeCommandReply(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := parseDocument(r); err != nil { // metadata
		return d.fail()
	}
	if _, err := parseDocument(r); err != nil { // commandReply
		return d.fail()
	}
	for r.remaining() > 0 { // outputDocs
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_command_reply")
	return true
}
```
> `responseTo` is still unused (correlation lands at Task 5) — Go permits the unused parameter.

- [ ] **Step 4: Run the body-decode tests + the full suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — all four new tests green (each flag bit; valid_cursor; doc walk; OP_COMMANDREPLY; both malformed → decoding_error); the Task-3 write-side framing tests still green (they feed empty replies → only `op_reply`).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.2 Task 4: OP_REPLY/OP_COMMANDREPLY body decode + the 5 response counters"
```

---

## Task 5: Correlation consumption (`takeQuery` first-match-erase) + the gauge `Dec` on a hit

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (`takeQuery` + the correlation block in `decodeReply`)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

- [ ] **Step 1: Write the failing tests (first-match-erase; only-OP_QUERY; miss charges fixed only)**

Add to `codec_test.go`:
```go
func TestCorrelation_FirstMatchEraseDecsGauge(t *testing.T) {
	d, ms := newTestDecoder(t)
	// Two OP_QUERYs (requestIDs 11, 12) → gauge 2, list len 2.
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	q2 := msg(12, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	both := append(q1, q2...)
	d.decodeOnData(both, int64(len(both)))
	if ms.opQueryActive.Load() != 2 || len(d.queries) != 2 {
		t.Fatalf("setup: gauge=%d len=%d, want 2/2", ms.opQueryActive.Load(), len(d.queries))
	}
	// A reply with responseTo=11 correlates the first query → erase + gauge Dec.
	d.decodeOnWrite(respMsg(99, 11, 1, opReplyBody(0, 0)))
	if ms.opQueryActive.Load() != 1 {
		t.Errorf("gauge = %d after one correlated reply, want 1", ms.opQueryActive.Load())
	}
	if len(d.queries) != 1 || d.queries[0].requestID != 12 {
		t.Errorf("first-match-erase failed: %+v", d.queries)
	}
}

func TestCorrelation_UncorrelatedMissChargesFixedOnly(t *testing.T) {
	d, ms := newTestDecoder(t)
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q1, int64(len(q1)))
	// A reply whose responseTo (777) matches NO pending query: op_reply +1, gauge UNCHANGED.
	d.decodeOnWrite(respMsg(99, 777, 1, opReplyBody(0, 0)))
	if ms.counters["op_reply"].Load() != 1 {
		t.Errorf("op_reply must still fire for an uncorrelated reply")
	}
	if ms.opQueryActive.Load() != 1 {
		t.Errorf("an uncorrelated reply must NOT change the gauge (still 1 in-flight query)")
	}
	if len(d.queries) != 1 {
		t.Errorf("an uncorrelated reply must not erase any entry")
	}
}

func TestCorrelation_CommandReplyDoesNotCorrelate(t *testing.T) {
	d, ms := newTestDecoder(t)
	q1 := msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q1, int64(len(q1)))
	// An OP_COMMANDREPLY echoing responseTo=11 must NOT erase the OP_QUERY entry
	// (only OP_REPLY correlates — parent §11.4 item 7).
	d.decodeOnWrite(respMsg(99, 11, 2011, opCommandReplyBody()))
	if ms.opQueryActive.Load() != 1 || len(d.queries) != 1 {
		t.Errorf("OP_COMMANDREPLY must not correlate against the active-query list")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestCorrelation -count=1`
Expected: FAIL — `TestCorrelation_FirstMatchEraseDecsGauge` fails (gauge stays 2; no `takeQuery` wired); the miss/command-reply tests may pass vacuously (no Dec yet) — they lock in the correct behavior once `takeQuery` lands.

- [ ] **Step 3: Add `takeQuery` + the correlation block in `decodeReply`**

Add to `codec.go`:
```go
// takeQuery removes + returns (by value) the FIRST active query whose requestID
// matches the reply's responseTo (upstream first-match-erase; parent §11.4 item
// 7). Holds mu for the scan + erase ONLY; the returned copy (incl. start, for
// the 29.3-deferred latency) is used outside the lock. ok=false → uncorrelated.
func (d *decoder) takeQuery(responseTo int32) (activeQuery, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.queries {
		if d.queries[i].requestID == responseTo {
			aq := d.queries[i]
			d.queries = append(d.queries[:i], d.queries[i+1:]...)
			return aq, true
		}
	}
	return activeQuery{}, false
}
```
In `decodeReply`, insert the correlation block AFTER the counter increments, BEFORE `return true`:
```go
	// Correlation (§3.3): a first-match-erase hit Decs the gauge OUTSIDE the lock
	// (the entry is already copied out). A miss leaves the gauge untouched —
	// uncorrelated replies (no pending query) charge only the fixed op_reply*
	// counters above. The copied entry's start time.Time rides for 29.3 latency.
	if _, ok := d.takeQuery(responseTo); ok {
		d.stats.opQueryActive.Dec()
	}
	return true
```

- [ ] **Step 4: Run the correlation tests + the full suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — first-match-erase Decs the gauge + erases the matching entry; the miss/command-reply paths leave the gauge + list untouched.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.2 Task 5: requestID-responseTo correlation (first-match-erase) + gauge Dec on a hit"
```

---

## Task 6: `OnDestroy` gauge teardown (residual-list drain) + the inc/dec lifecycle invariant

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (`onDestroy`), `internal/filter/network/mongoproxy/filter.go` (`OnDestroy`)
- Test: `internal/filter/network/mongoproxy/codec_test.go`, `internal/filter/network/mongoproxy/filter_test.go`

- [ ] **Step 1: Write the failing tests (residual drain → gauge 0; full lifecycle invariant)**

Add to `codec_test.go`:
```go
func TestOnDestroy_DrainsResidualGauge(t *testing.T) {
	// Two never-answered queries → gauge 2; onDestroy drains the residual list and
	// Decs the gauge per entry → gauge 0 (the connection-close teardown, §3.4).
	d, ms := newTestDecoder(t)
	q := append(
		msg(11, 2004, opQueryBody("db.collection1", 0, simpleQuery())),
		msg(12, 2004, opQueryBody("db.collection1", 0, simpleQuery()))...,
	)
	d.decodeOnData(q, int64(len(q)))
	if ms.opQueryActive.Load() != 2 {
		t.Fatalf("setup: gauge=%d, want 2", ms.opQueryActive.Load())
	}
	d.onDestroy()
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge = %d after onDestroy, want 0 (residual drain Dec)", ms.opQueryActive.Load())
	}
	if len(d.queries) != 0 {
		t.Errorf("onDestroy must clear the residual list")
	}
}

func TestGaugeLifecycle_Invariant(t *testing.T) {
	// inc(2) → dec(1 correlated) → destroy(drains 1) → 0. The list-size↔gauge
	// invariant holds at each step.
	d, ms := newTestDecoder(t)
	q := append(
		msg(11, 2004, opQueryBody("db.c1", 0, simpleQuery())),
		msg(12, 2004, opQueryBody("db.c1", 0, simpleQuery()))...,
	)
	d.decodeOnData(q, int64(len(q)))
	d.decodeOnWrite(respMsg(99, 11, 1, opReplyBody(0, 0))) // answer query 11
	if ms.opQueryActive.Load() != int64(len(d.queries)) || ms.opQueryActive.Load() != 1 {
		t.Fatalf("after one answer: gauge=%d len=%d, want 1/1", ms.opQueryActive.Load(), len(d.queries))
	}
	d.onDestroy()
	if ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge = %d at end of connection, want 0", ms.opQueryActive.Load())
	}
}
```
Add to `filter_test.go` (replacing nothing — additive):
```go
func TestFilter_OnDestroyDrainsGauge(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	f.OnData(&buf, false)
	if reg.NewGaugeIfAbsent("mongo.p.op_query_active").Load() != 1 {
		t.Fatalf("setup: gauge != 1 after one query")
	}
	f.OnDestroy()
	if reg.NewGaugeIfAbsent("mongo.p.op_query_active").Load() != 0 {
		t.Errorf("OnDestroy must drain the gauge to 0")
	}
	if f.dec != nil {
		t.Errorf("OnDestroy must still release the decoder")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestOnDestroy|TestGaugeLifecycle|TestFilter_OnDestroyDrains' -count=1`
Expected: COMPILE FAIL — `d.onDestroy` undefined; then (after Step 3 partial) the filter test fails because `OnDestroy` only nils the decoder (no drain).

- [ ] **Step 3: Add `decoder.onDestroy` + wire `filter.OnDestroy`**

Add to `codec.go`:
```go
// onDestroy drains the residual active-query list at connection teardown and
// Decs op_query_active once per still-live entry so the gauge returns to 0 when
// the connection ends (mirrors upstream's ActiveQuery destructor for every live
// entry). The list is cleared under mu; the gauge math runs OUTSIDE the lock
// (snapshot-count-then-Add; D-S29.2-5). Idempotent — a second call drains 0.
func (d *decoder) onDestroy() {
	d.mu.Lock()
	n := len(d.queries)
	d.queries = nil
	d.mu.Unlock()
	if n > 0 {
		d.stats.opQueryActive.Add(int64(-n))
	}
}
```
In `filter.go`, replace `OnDestroy`:
```go
// OnDestroy drains any residual active-query entries (Dec the gauge per entry so
// it returns to 0 at connection end — §3.4) then drops the per-connection
// decoder. Called exactly once per filter instance (the 28.1a dedupe); it runs
// strictly after both pumps join (the ADR-0221 happens-after edge), so the
// onDestroy lock is uncontended.
func (f *filter) OnDestroy() {
	if f.dec != nil {
		f.dec.onDestroy()
	}
	f.dec = nil
}
```

- [ ] **Step 4: Run the teardown tests + the full suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — the residual drain returns the gauge to 0; the full lifecycle invariant holds; `OnDestroy` still releases the decoder.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/filter.go internal/filter/network/mongoproxy/codec_test.go internal/filter/network/mongoproxy/filter_test.go
git commit -m "phase 29.2 Task 6: OnDestroy gauge teardown (residual-list drain) + the inc/dec lifecycle invariant"
```

---

## Task 7: `OnWrite` glue (replace the no-op) + the concurrent request/response race test (R9)

**Files:**
- Modify: `internal/filter/network/mongoproxy/filter.go` (`OnWrite`)
- Test: `internal/filter/network/mongoproxy/filter_test.go` (replace `TestFilter_OnWriteIsNoOp`), `internal/filter/network/mongoproxy/codec_test.go` (the `-race` test)

- [ ] **Step 1: Write the failing tests (OnWrite feeds the response decoder; the race test)**

In `filter_test.go`, REPLACE `TestFilter_OnWriteIsNoOp` with:
```go
func TestFilter_OnWriteFeedsResponseDecoder(t *testing.T) {
	f, reg := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(respMsg(7, 0, 1, opReplyBody(0, 0))) // an empty OP_REPLY on the write side
	if f.OnWrite(&buf, false) != network.Continue {
		t.Error("OnWrite must return Continue")
	}
	if reg.NewCounterIfAbsent("mongo.p.op_reply").Load() != 1 {
		t.Errorf("OnWrite must feed the response decoder (op_reply != 1)")
	}
}

func TestFilter_OnWriteNeverDrainsChainBuffer(t *testing.T) {
	// R3 extended to the write side: the write chain Buffer is observational.
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	var buf network.Buffer
	buf.Append(respMsg(7, 0, 1, opReplyBody(0, 0)))
	before := buf.Len()
	f.OnWrite(&buf, false)
	if buf.Len() != before {
		t.Errorf("OnWrite drained the write chain buffer: %d → %d", before, buf.Len())
	}
}
```
In `codec_test.go`, add the concurrency test:
```go
func TestDecoderConcurrentRequestResponseRace(t *testing.T) {
	// R9: two goroutines over ONE decoder — A drives decodeOnData with a request
	// stream, B drives decodeOnWrite with the matching response stream. With mu
	// guarding dec.queries this is race-clean; REMOVING mu MUST trip `go test
	// -race`. Run under `-race -count=5`.
	d, ms := newTestDecoder(t)
	const n = 200
	reqs := make([][]byte, n)
	reps := make([][]byte, n)
	for i := 0; i < n; i++ {
		id := int32(i + 1)
		reqs[i] = msg(id, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
		reps[i] = respMsg(int32(10000+i), id, 1, opReplyBody(0, 0))
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var total int64
		for _, r := range reqs {
			total += int64(len(r))
			d.decodeOnData(r, total)
		}
	}()
	go func() {
		defer wg.Done()
		for _, r := range reps {
			d.decodeOnWrite(r)
		}
	}()
	wg.Wait()
	// No assertion on the exact gauge value (the interleaving is nondeterministic —
	// a reply may arrive before its query); the point is race-freedom + no panic.
	// At minimum op_reply counted every fed reply.
	if ms.counters["op_reply"].Load() != int64(n) {
		t.Errorf("op_reply = %d, want %d", ms.counters["op_reply"].Load(), n)
	}
}
```
> Add `"sync"` to the `codec_test.go` imports.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_OnWrite|TestDecoderConcurrent' -count=1`
Expected: FAIL — `TestFilter_OnWriteFeedsResponseDecoder` fails (`op_reply` stays 0 — the no-op stub); the race test passes functionally but proves nothing until the glue is real.

- [ ] **Step 3: Replace the `OnWrite` no-op with the response feed**

In `filter.go`, replace `OnWrite`:
```go
// OnWrite feeds the response decoder the write-direction (upstream→downstream)
// bytes and ALWAYS returns Continue (R3 extended to the write side; upstream
// onWrite parity — never halts). Replaces the 29.1 no-op stub.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	return network.Continue
}
```

- [ ] **Step 4: Run the glue tests + the race test under `-race -count=5`**

```bash
go test ./internal/filter/network/mongoproxy/ -count=1
go test ./internal/filter/network/mongoproxy/ -run TestDecoderConcurrentRequestResponseRace -race -count=5
```
Expected: PASS both — the OnWrite feed drives `op_reply`; the concurrent test is race-clean.

- [ ] **Step 5: Prove the mutex is LOAD-BEARING (R9 deliberate-break, `-count=1`)**

Temporarily comment out the `d.mu.Lock()`/`d.mu.Unlock()` pair in `appendQuery` AND the `d.mu.Lock()`/`defer d.mu.Unlock()` in `takeQuery` (the two contended lockers). Run:
```bash
go test ./internal/filter/network/mongoproxy/ -run TestDecoderConcurrentRequestResponseRace -race -count=1
```
Expected: a `DATA RACE` report (concurrent slice append/read/realloc on `dec.queries`). **Revert the comments**; re-run with the locks restored → clean. Record both outputs in `PROGRESS.md` (`reference_differential_break_protocol_count1`: `-count=1` so result-caching can't serve a stale PASS).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/filter.go internal/filter/network/mongoproxy/filter_test.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.2 Task 7: OnWrite response feed (replaces the no-op) + the concurrent request/response race test (R9)"
```

---

## Task 8: `emit_dynamic_metadata` emission (the single-Set Bucket model)

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (the `recordOp` helper + the two request-decode emit sites), `internal/filter/network/mongoproxy/filter.go` (the `emitDynamicMetadata` helper + the OnData emit call + the namespace/key consts)
- Test: `internal/filter/network/mongoproxy/filter_test.go`

- [ ] **Step 1: Write the failing tests (namespace/keys/clear; gated-off no-emit)**

Add a fake callbacks providing a real Bucket to `filter_test.go`:
```go
import (
	// ... existing ...
	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
)

type fakeReadCallbacks struct {
	network.ReadFilterCallbacks
	dm *dynamicmetadata.Bucket
}

func (cb *fakeReadCallbacks) DynamicMetadata() *dynamicmetadata.Bucket { return cb.dm }

func driveOnData(f *filter, frames ...[]byte) {
	var buf network.Buffer
	for _, fr := range frames {
		buf.Append(fr)
	}
	f.OnData(&buf, false)
}

func TestEmitDynamicMetadata_CollectionToOps(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p", EmitDynamicMetadata: true})
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	// One pass: an OP_QUERY on collection1 + an OP_INSERT on collection2.
	driveOnData(f,
		msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())),
		msg(2, 2002, append(append(leI32(0), cstr("db.collection2")...), simpleQuery()...)),
	)
	v, ok := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations")
	if !ok {
		t.Fatalf("dynamic metadata not emitted")
	}
	fields := v.GetStructValue().GetFields()
	if got := fields["collection1"].GetListValue().GetValues(); len(got) != 1 || got[0].GetStringValue() != "query" {
		t.Errorf("collection1 ops = %v, want [query]", got)
	}
	if got := fields["collection2"].GetListValue().GetValues(); len(got) != 1 || got[0].GetStringValue() != "insert" {
		t.Errorf("collection2 ops = %v, want [insert]", got)
	}
}

func TestEmitDynamicMetadata_PerPassOverwriteClear(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p", EmitDynamicMetadata: true})
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	driveOnData(f, msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	driveOnData(f, msg(2, 2004, opQueryBody("db.collection2", 0, simpleQuery())))
	v, _ := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations")
	fields := v.GetStructValue().GetFields()
	if _, present := fields["collection1"]; present {
		t.Errorf("per-pass clear failed: collection1 from pass 1 still present in pass 2")
	}
	if _, present := fields["collection2"]; !present {
		t.Errorf("pass-2 collection2 missing")
	}
}

func TestEmitDynamicMetadata_GatedOff(t *testing.T) {
	f, _ := buildFilter(t, &mongo_proxyv3.MongoProxy{StatPrefix: "p"}) // emit flag default false
	cb := &fakeReadCallbacks{dm: dynamicmetadata.NewBucket()}
	f.SetReadFilterCallbacks(cb)
	driveOnData(f, msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	if _, ok := cb.dm.Get("envoy.filters.network.mongo_proxy", "operations"); ok {
		t.Errorf("no metadata may be emitted when emit_dynamic_metadata is false")
	}
}
```

- [ ] **Step 2: Run to verify they fail / don't compile**

Run: `go test ./internal/filter/network/mongoproxy/ -run TestEmitDynamicMetadata -count=1`
Expected: FAIL — no metadata emitted (no `recordOp`/`emitDynamicMetadata` wired).

- [ ] **Step 3: Add `recordOp` + the two request-decode emit sites in `codec.go`**

Add the accumulator helper:
```go
// recordOp accumulates this request pass's collection→op for emit_dynamic_metadata
// (gated; goroutine-A-only — the request decode is sequential, no lock). The map
// is reset at the top of each decodeOnData pass (per-pass clear; §3.7).
func (d *decoder) recordOp(collection, op string) {
	if !d.cfg.emitDynamicMetadata {
		return
	}
	if d.dynMeta == nil {
		d.dynMeta = map[string][]string{}
	}
	d.dynMeta[collection] = append(d.dynMeta[collection], op)
}
```
In `decodeQuery`, record `"query"` at BOTH success points (the `collection` local is computed before the `$cmd` branch). Add `d.recordOp(collection, "query")` immediately before the `$cmd` path's `d.appendQuery(aq); return true` AND immediately before the non-command path's `d.appendQuery(aq); return true`:
```go
		// ... $cmd path, name != "" branch:
			aq.command = name
			d.recordOp(collection, "query")
			d.appendQuery(aq)
			return true
```
```go
	// ... non-command path, end:
	d.recordOp(collection, "query")
	d.appendQuery(aq)
	return true
```
In `decodeInsert`, capture the collection (currently the cstring is discarded) and record `"insert"`:
```go
func (d *decoder) decodeInsert(body []byte) bool {
	r := &bsonReader{buf: body}
	if _, err := r.readInt32(); err != nil { // flags
		return d.fail()
	}
	fullColl, err := r.readCString()
	if err != nil {
		return d.fail()
	}
	for r.remaining() > 0 {
		if _, err := parseDocument(r); err != nil {
			return d.fail()
		}
	}
	d.stats.inc("op_insert")
	if dot := strings.IndexByte(fullColl, '.'); dot >= 0 {
		d.recordOp(fullColl[dot+1:], "insert")
	}
	return true
}
```
> `decodeOnData` already resets `d.dynMeta = nil` at the top of each pass when `emitDynamicMetadata` is on (added at Task 3 Step 3). No change there.
>
> **Post-error note (advisory, intentional):** the Task-3 `decodeOnData` sniffing-off early-return path returns BEFORE the `d.dynMeta = nil` reset, so after a connection's first decode error `dynMeta` retains the last successful pass's accumulation and `emitDynamicMetadata` may re-`Set` that same value on subsequent `OnData` calls. This is harmless and intentional — an idempotent overwrite of the SAME `(namespace, key)` with the SAME value, connection-level, and differential-invisible (no cross-side surface, no in-repo consumer). Do NOT add a clear in the early-return branch; the unit tests (which never error mid-emit) are unaffected.

- [ ] **Step 4: Add `emitDynamicMetadata` + the OnData emit call in `filter.go`**

Add `"google.golang.org/protobuf/types/known/structpb"` to the `filter.go` imports. Add the consts + helper:
```go
const (
	// mongoMetadataNamespace is the dynamic-metadata namespace (parent §11.9 /
	// AMEND-B11). mongoMetadataKey is the single-Set wrapper key under which the
	// whole collection→ops StructValue is written (D-S29.2-3 — a unit-test-asserted
	// internal detail; differential-invisible, no cross-side surface).
	mongoMetadataNamespace = "envoy.filters.network.mongo_proxy"
	mongoMetadataKey       = "operations"
)

// emitDynamicMetadata writes THIS request pass's collection→ops map to the
// per-connection dynamic-metadata Bucket as ONE StructValue (the §3.7 single-Set
// model — the next emitting pass overwrites it, giving per-pass clear for free).
// Gated by emit_dynamic_metadata; a no-op when the flag is off or the pass
// produced no insert/query (SPEC §3.7 "skip the Set if empty"). The Bucket is
// nil-receiver tolerant (ADR-0085).
func (f *filter) emitDynamicMetadata() {
	if !f.cfg.emitDynamicMetadata || len(f.dec.dynMeta) == 0 {
		return
	}
	fields := make(map[string]*structpb.Value, len(f.dec.dynMeta))
	for coll, ops := range f.dec.dynMeta {
		vals := make([]*structpb.Value, len(ops))
		for i, op := range ops {
			vals[i] = structpb.NewStringValue(op)
		}
		fields[coll] = structpb.NewListValue(&structpb.ListValue{Values: vals})
	}
	sv := structpb.NewStructValue(&structpb.Struct{Fields: fields})
	f.cb.DynamicMetadata().Set(mongoMetadataNamespace, mongoMetadataKey, sv)
}
```
Wire it into `OnData` (after the decode feed):
```go
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	f.emitDynamicMetadata()
	return network.Continue
}
```

- [ ] **Step 5: Run the metadata tests + the full suite green**

Run: `go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS — collection→ops emitted; per-pass overwrite-clear works; gated-off emits nothing. The existing `TestFilter_OnDataFeedsDecoder`/`TestFilter_OnDataNeverDrainsChainBuffer` still green (the emit flag is off → `emitDynamicMetadata` returns before touching the nil `f.cb`).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/filter.go internal/filter/network/mongoproxy/filter_test.go
git commit -m "phase 29.2 Task 8: emit_dynamic_metadata single-Set Bucket emission (collection→ops; per-pass clear; gated)"
```

---

## Task 9: Extend the 39th fuzzer to the response opcodes (D-P6)

**Files:**
- Modify: `internal/filter/network/mongoproxy/fuzz_test.go`

The direction-agnostic decoder → ONE fuzzer covers both directions (no 40th; count stays 39). Feed arbitrary bytes through BOTH `decodeOnData` and `decodeOnWrite`; assert no panic + chain-buffer immutability + sniffing-off idempotence across directions.

- [ ] **Step 1: Extend `FuzzMongoDecode` to feed both directions**

In `fuzz_test.go`: update the seed corpus to add response frames, change the body to feed `decodeOnWrite` as well, and update the `d.sniffing` read to `.Load()`. Replace the body of `f.Fuzz(...)`:
```go
	// Seed corpus additions: a valid empty OP_REPLY, an OP_COMMANDREPLY, an
	// OP_REPLY claiming docs that don't follow (malformed).
	f.Add(respSeed(1, 1, replyBodySeed(0, 0, 0)))
	f.Add(respSeed(1, 2011, append(docSeed(), docSeed()...)))
	f.Add(respSeed(1, 1, replyBodySeed(0, 0, 5))) // numberReturned=5, no docs

	f.Fuzz(func(t *testing.T, data []byte) {
		reg := stats.NewRegistry()
		cfg := &compiledConfig{statPrefix: "fuzz", commands: map[string]bool{"isMaster": true}}
		ms := newMongoStats(reg, "fuzz")
		cfg.stats = ms
		d := newDecoder(cfg, ms)

		orig := append([]byte(nil), data...)

		// Feed BOTH directions through the one decoder (the same bytes drive the
		// request and response decode paths). Invariant 1: no panic (implicit).
		d.decodeOnData(data, int64(len(data)))
		d.decodeOnWrite(data)
		errAfterFirst := ms.counters["decoding_error"].Load()
		sniffingAfterFirst := d.sniffing.Load()

		// Feed a second cumulative round on the read side + a second write round.
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))
		d.decodeOnWrite(data)

		// Invariant 2: the input was never mutated (R3, both directions).
		if !bytes.Equal(data, orig) {
			t.Fatal("decode mutated the chain bytes")
		}
		// Invariant 3: once sniffing is off, decoding_error never increments again
		// on EITHER direction (direction-shared at-most-once).
		if !sniffingAfterFirst && ms.counters["decoding_error"].Load() != errAfterFirst {
			t.Fatalf("decoding_error grew after sniffing-off: %d → %d",
				errAfterFirst, ms.counters["decoding_error"].Load())
		}
		// Invariant 4: both private buffers stay bounded.
		if len(d.readBuf) > len(doubled)+16 || len(d.writeBuf) > len(data)+16 {
			t.Fatalf("a private buffer grew unboundedly: read=%d write=%d", len(d.readBuf), len(d.writeBuf))
		}
	})
```
Add the response seed helpers at the bottom of `fuzz_test.go`:
```go
// respSeed builds a response wire frame (16-byte LE header, responseTo=0).
func respSeed(reqID, opCode int32, body []byte) []byte { return msg(reqID, opCode, body) }

// replyBodySeed builds an OP_REPLY body: flags + cursorID + startingFrom +
// numberReturned (+ no docs — numReturned may LIE for the malformed seed).
func replyBodySeed(flags int32, cursorID int64, numReturned int32) []byte {
	out := append(leI32(flags), leI64(cursorID)...)
	out = append(out, leI32(0)...)
	return append(out, leI32(numReturned)...)
}

// docSeed is a minimal empty BSON document (5 bytes).
func docSeed() []byte { return doc() }
```

- [ ] **Step 2: Run the fuzzer briefly + confirm the count is unchanged (39)**

```bash
go test ./internal/filter/network/mongoproxy/ -run 'xxxxNoMatchxxxx' -fuzz=FuzzMongoDecode -fuzztime=20s
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 39 (unchanged)
go test ./internal/filter/network/mongoproxy/ -count=1                 # seed-corpus run via the normal test
```
Expected: the fuzz run finds no crash within 20s; the fuzzer count stays **39** (`FuzzMongoDecode` EXTENDED, not added); the normal `-count=1` run (which exercises the seed corpus) is green.

- [ ] **Step 3: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/mongoproxy/
golangci-lint run ./internal/filter/network/mongoproxy/...
git add internal/filter/network/mongoproxy/fuzz_test.go
git commit -m "phase 29.2 Task 9: extend FuzzMongoDecode to the response opcodes (both directions; no 40th)"
```

---

## Task 10: `TCPMongoResponder` BackendKind 30 + the runner backend arm + the response wire builders

**Files:**
- Modify: `test/differential/fixture/fixture.go` (the `TCPMongoResponder = 30` const)
- Modify: `test/differential/runner_test.go` (the dispatch arm + `acceptMongoResponder` + `mongoRespondLoop` + a `TestMongoResponderBackend` unit test)

The `0049` `TCPSink` is request-side-only; `0051` needs a backend that emits CORRELATED OP_REPLY/OP_COMMANDREPLY frames (so the reference's `onWrite` decoder fires + correlates). This task adds the responder; Task 11 builds the `0051` driver against it.

- [ ] **Step 1: Add the `TCPMongoResponder` BackendKind**

In `test/differential/fixture/fixture.go`, after `TCPZKResponder BackendKind = 29` (with a doc comment in the existing style):
```go
	// TCPMongoResponder is a MongoDB-aware canned-response TCP backend (29.2 SPEC
	// §6.1): for every complete request frame (16-byte LE MsgHeader) it parses
	// messageLength + requestID + opCode ONLY (it is NOT a MongoDB server) and
	// writes a CORRELATED response whose responseTo echoes the request requestID —
	// OP_QUERY(2004) → OP_REPLY(1), OP_COMMAND(2010) → OP_COMMANDREPLY(2011),
	// reply-flag/cursor variants by a marker the driver controls, and an
	// UNANSWERED-query trigger that withholds the reply (the gauge quiesced-point
	// arm). OP_INSERT/OP_KILL_CURSORS get no reply (fire-and-forget). NEW
	// BackendKind per reference_differential_fixture_dispatch_constraint; the
	// TCPZKResponder = 29 precedent.
	TCPMongoResponder BackendKind = 30
```

- [ ] **Step 2: Write the failing responder unit test**

In `runner_test.go`, add (mirroring `TestZKResponderBackend` near line 1889):
```go
func TestMongoResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptMongoResponder(ln, &accepts)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	// An OP_QUERY(2004) requestID 11 → a correlated OP_REPLY(1) whose responseTo == 11.
	req := mongoReqFrame(11, 2004, "db.collection1")
	if _, err := c.Write(req); err != nil {
		t.Fatalf("write: %v", err)
	}
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(c, hdr); err != nil {
		t.Fatalf("read reply header: %v", err)
	}
	msgLen := int32(binary.LittleEndian.Uint32(hdr[0:4]))
	responseTo := int32(binary.LittleEndian.Uint32(hdr[8:12]))
	opCode := int32(binary.LittleEndian.Uint32(hdr[12:16]))
	if opCode != 1 {
		t.Errorf("reply opCode = %d, want 1 (OP_REPLY)", opCode)
	}
	if responseTo != 11 {
		t.Errorf("reply responseTo = %d, want 11 (correlation echo)", responseTo)
	}
	rest := make([]byte, msgLen-16)
	if _, err := io.ReadFull(c, rest); err != nil {
		t.Fatalf("read reply body: %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./test/differential/ -run TestMongoResponderBackend -count=1`
Expected: COMPILE FAIL — `acceptMongoResponder` / `mongoReqFrame` undefined.

- [ ] **Step 4: Add `acceptMongoResponder` + `mongoRespondLoop` + the helpers**

In `runner_test.go`, add (the `acceptZKResponder` sibling — the LE mongo wire mirror; place near the ZK responder block ~line 1307):
```go
// TCPMongoResponder trigger markers (D-S29.2-2 / SPEC §6.1). The responder peeks
// the request frame's requestID (bytes 4-8) + opCode (bytes 12-16) only. A marker
// requestID selects a reply-flag variant or the unanswered-query withhold; the
// driver assigns these requestIDs so both sides see identical correlated bytes.
const (
	mongoReplyCursorNotFound int32 = 0x01 // responseFlags 0x01
	mongoReplyQueryFailure   int32 = 0x02 // responseFlags 0x02
)

// mongoMarkerWithhold is the requestID the responder treats as the unanswered-
// query trigger: it reads the request but writes NO reply (the gauge stays at 1
// while the connection is open — §3.4 / §6.2 arm 4).
const mongoMarkerWithhold int32 = 7777

// mongoMarkerCursorNotFound / mongoMarkerQueryFailure / mongoMarkerValidCursor /
// mongoMarkerUncorrelated select the reply variant by requestID.
const (
	mongoMarkerCursorNotFound int32 = 7001
	mongoMarkerQueryFailure   int32 = 7002
	mongoMarkerValidCursor    int32 = 7003
	mongoMarkerMalformedReply int32 = 7004
	mongoMarkerUncorrelated   int32 = 7005
)

// acceptMongoResponder accepts connections, counts them, and runs the
// MongoDB-aware canned-response loop on each (the TCPMongoResponder backend —
// 29.2 SPEC §6.1; the acceptZKResponder sibling).
func acceptMongoResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go mongoRespondLoop(c)
	}
}

// mongoRespondLoop reads complete request frames (16-byte LE MsgHeader framing)
// and writes correlated canned responses until the client closes. It parses ONLY
// messageLength + requestID + opCode; it is NOT a MongoDB server.
func mongoRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	le32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return b
	}
	le64 := func(v int64) []byte {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(v))
		return b
	}
	// respFrame builds a response with responseTo echoed; opCode is OP_REPLY(1) or
	// OP_COMMANDREPLY(2011); a fresh responder requestID (constant 90000).
	respFrame := func(responseTo, opCode int32, body []byte) []byte {
		out := append(le32(int32(16+len(body))), le32(90000)...)
		out = append(out, le32(responseTo)...)
		out = append(out, le32(opCode)...)
		return append(out, body...)
	}
	emptyDoc := []byte{0x05, 0x00, 0x00, 0x00, 0x00} // {len=5}{terminator}
	replyBody := func(flags int32, cursorID int64, ndocs int32) []byte {
		out := append(le32(flags), le64(cursorID)...)
		out = append(out, le32(0)...)      // startingFrom
		out = append(out, le32(ndocs)...)  // numberReturned
		for i := int32(0); i < ndocs; i++ {
			out = append(out, emptyDoc...)
		}
		return out
	}
	commandReplyBody := func() []byte { return append(append([]byte(nil), emptyDoc...), emptyDoc...) }

	for {
		var hdr [16]byte
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return // client closed / EOF
		}
		msgLen := int32(binary.LittleEndian.Uint32(hdr[0:4]))
		if msgLen < 16 || msgLen > 1<<20 {
			return // malformed / hostile
		}
		body := make([]byte, msgLen-16)
		if _, err := io.ReadFull(c, body); err != nil {
			return
		}
		reqID := int32(binary.LittleEndian.Uint32(hdr[4:8]))
		opCode := int32(binary.LittleEndian.Uint32(hdr[12:16]))

		switch opCode {
		case 2004: // OP_QUERY → a correlated OP_REPLY, variant by the marker requestID
			switch reqID {
			case mongoMarkerWithhold:
				// withhold — no reply (the unanswered-query gauge arm)
			case mongoMarkerCursorNotFound:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(mongoReplyCursorNotFound, 0, 0)))
			case mongoMarkerQueryFailure:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(mongoReplyQueryFailure, 0, 0)))
			case mongoMarkerValidCursor:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(0, 4242, 1)))
			case mongoMarkerMalformedReply:
				// a well-framed OP_REPLY whose numberReturned lies (claims 1 doc, none) →
				// the reference + envoy-go both hit a decoding_error on this frame.
				_, _ = c.Write(respFrame(reqID, 1, replyBody(0, 0, 1)))
			case mongoMarkerUncorrelated:
				// a reply whose responseTo matches NO sent query (responseTo = reqID+50000)
				_, _ = c.Write(respFrame(reqID+50000, 1, replyBody(0, 0, 0)))
			default:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(0, 0, 0))) // plain empty reply
			}
		case 2010: // OP_COMMAND → a correlated OP_COMMANDREPLY
			_, _ = c.Write(respFrame(reqID, 2011, commandReplyBody()))
		default:
			// OP_INSERT(2002) / OP_GET_MORE(2005) / OP_KILL_CURSORS(2007): no reply
			// (fire-and-forget; D-S29.2-2 — get_more not exercised by the load-bearing
			// arms). Read-and-drop.
		}
	}
}

// mongoReqFrame builds a minimal request frame for the responder unit test.
func mongoReqFrame(reqID, opCode int32, fullColl string) []byte {
	le32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return b
	}
	body := append(le32(0), append([]byte(fullColl), 0x00)...) // flags + cstring collection
	body = append(body, le32(0)...)                            // numberToSkip
	body = append(body, le32(0)...)                            // numberToReturn
	body = append(body, 0x05, 0x00, 0x00, 0x00, 0x00)         // empty query doc
	out := append(le32(int32(16+len(body))), le32(reqID)...)
	out = append(out, le32(0)...)      // responseTo
	out = append(out, le32(opCode)...) // opCode
	return append(out, body...)
}
```
Wire the dispatch arm into the backend-allocation switch (after the `case fixture.TCPZKResponder:` block ~line 857):
```go
		case fixture.TCPMongoResponder:
			// MongoDB-aware canned responder (29.2 SPEC §6.1): correlated
			// OP_REPLY/OP_COMMANDREPLY frames so the reference's onWrite response
			// decoder fires + correlates.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptMongoResponder(ln, bo.accepts)
```

- [ ] **Step 5: Run the responder unit test + confirm the BackendKind**

```bash
go test ./test/differential/ -run TestMongoResponderBackend -count=1
grep -n "TCPMongoResponder BackendKind = 30" test/differential/fixture/fixture.go
```
Expected: PASS — the responder echoes responseTo; `TCPMongoResponder = 30` present.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l test/differential/
golangci-lint run ./test/differential/...
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 29.2 Task 10: TCPMongoResponder BackendKind 30 + acceptMongoResponder canned-reply backend"
```

---

## Task 11: `0051-mongo-responses` cross-side fixture + the completion bundle

**Files:**
- Create: `test/fixtures/0051-mongo-responses/driver/driver.go`, `test/fixtures/0051-mongo-responses/README.md`
- Modify: `test/differential/runner_test.go` (the `0051` driver blank-import)
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`

This task lands the load-bearing differential fixture (the `TCPMongoResponder` backend; the gauge quiesced-point arms; the R4 deliberate-break) then the atomic completion bundle.

### 11A — the `0051` driver

- [ ] **Step 1: Author `test/fixtures/0051-mongo-responses/driver/driver.go`**

Model it on `test/fixtures/0049-mongo-requests/driver/driver.go`. REUSE VERBATIM (copy from the `0049` driver): `leI32`/`leI64`/`cstr`/`bsonInt32`/`bsonDoc`/`mongoMsg`/`opQuery`/`opCommand`; `renderBootstrap`/`bootstrapParams`/`mongoProxyType`/`tcpProxyType`; `scrapeMongoStats`/`scrapeTypeLine`/`canonicalize`/`httpGet`; `ProbeAdmin`; `driveFrames`/`sleepCtx`/`emitArm`. The `0051`-specific pieces:

```go
const (
	fixtureName     = "0051-mongo-responses"
	refAdminPort    = 9901
	refLRespPort    = 19142 // distinct from 0049's 19140/19141
	statPrefixResp  = "mongo_r"
)

func init() { fixture.RegisterFixture(fixtureName, &mongoResponsesDriver{}) }

type mongoResponsesDriver struct{}

// BackendKind returns TCPMongoResponder: the correlated canned-reply backend
// (BackendKind 30) so the reference's onWrite response decoder fires + correlates.
func (*mongoResponsesDriver) BackendKind() fixture.BackendKind { return fixture.TCPMongoResponder }

func (*mongoResponsesDriver) BackendCount() int            { return 1 }
func (*mongoResponsesDriver) SubjectListenerName() string  { return "l_resp" }
func (*mongoResponsesDriver) ReferenceListenerPort() int   { return refLRespPort }
```

The driver is SINGLE-listener (`l_resp`, `stat_prefix mongo_r`) — render a one-listener bootstrap (simplify `renderBootstrap` to a single `l_resp` chain `[mongo_proxy, tcp_proxy]` → `c_resp` cluster pointing at the `TCPMongoResponder`; the `0049` two-listener render trimmed to one). Implement `ReferenceBootstrap`/`SubjectConfig` accordingly. (No `MultiListenerDriver` — the single-listener `Driver` `DriveReference`/`DriveSubject` suffice.)

The wire builders this driver ADDS (beside the copied `opQuery`/`opCommand`) — the response-side reply builders are NOT needed by the driver (the RESPONDER emits replies); the driver only sends REQUESTS with the marker requestIDs the responder recognizes (Task 10). So no new reply builders in the driver.

`DriveReference`/`DriveSubject` → a shared `driveProxy(ctx, addr, side)` that runs the arms in order (the `0049` `emitArm`/`driveFrames` discipline), closing each connection so the gauge quiesces, then sleeps `settleDelay` before returning. The arms (SPEC §6.2):

```go
// Arm 1 (plain reply round-trip): OP_QUERY reqID 1 → the responder's empty
// OP_REPLY (responseTo 1) → op_reply +1; correlated → op_query_active settles 0.
// driveAndReadReply waits for the reply bytes to round-trip before closing so the
// gauge is observed at rest (D-P9 quiesced point).
driveAndReadReply(ctx, addr, opQuery(1, dbColl, 0, bsonDoc(bsonInt32("a", 1)...)))

// Arm 2 (reply-flag variants): three FRESH connections, reqIDs = the markers
// (mongoMarkerCursorNotFound 7001 / mongoMarkerQueryFailure 7002 /
// mongoMarkerValidCursor 7003) → op_reply_cursor_not_found / _query_failure /
// _valid_cursor each +1; each correlated (gauge back to 0).
//   (the marker requestIDs are the constants Task 10 defined; mirror them here as
//    driver-local consts so both sides send identical bytes.)

// Arm 3 (OP_COMMAND round-trip): OP_COMMAND reqID 20 → OP_COMMANDREPLY →
// op_command_reply +1; no active-query entry → gauge untouched.

// Arm 4 (the gauge quiesced-point arms):
//   (i)  ANSWERED: arm 1 already proves answered→0.
//   (ii) UNANSWERED: OP_QUERY reqID = mongoMarkerWithhold (7777) on a connection
//        the driver HOLDS OPEN; the responder withholds the reply → scrape while
//        open → op_query_active == 1 both sides; then close → a post-close
//        re-scrape (AssertStats runs after all connections closed) → 0 both sides.
//        (The open-connection scrape is the load-bearing == 1 assertion — see
//        AssertStats's two-phase scrape note below.)

// Arm 5 (uncorrelated reply): OP_QUERY reqID = mongoMarkerUncorrelated (7005) →
// the responder emits a reply whose responseTo matches NO sent query → op_reply
// +1, gauge UNCHANGED by the correlation miss (the sent query still self-answers?
// NO — the responder sends ONLY the uncorrelated reply, so the reqID-7005 query
// stays in the list → that connection's gauge drains at close). The cross-side
// signal is op_reply +1 with NO valid_cursor/flag counters.

// Arm 6 (malformed-reply decoding_error, FRESH conn): OP_QUERY reqID =
// mongoMarkerMalformedReply (7004) → the responder emits a well-framed OP_REPLY
// whose numberReturned lies → decoding_error +1 both sides; a follow-up valid
// reply on the SAME conn increments nothing (direction-shared sniffing-off).

// Arm 7+8 are assertion-only (cx_destroy presence; gauge TYPE + ==0; no
// dynamic-metadata fixture surface). Arm 9 is the recorded R4 break (README).
```

> **AssertStats two-phase scrape for the UNANSWERED arm.** The `op_query_active == 1` assertion requires scraping WHILE the withheld-reply connection is open. The cleanest harness-compatible approach (the runner calls `AssertStats` ONCE after `DriveSubject`/`DriveReference` return): the driver holds the withhold connection open in a struct field opened during `driveProxy`, scrapes the `== 1` point INLINE inside `driveProxy` (a private scrape against the admin addr passed via the driver, OR — simpler — assert the unanswered `== 1` via a dedicated in-`driveProxy` scrape using the same `scrapeMongoStats` helper against the admin endpoint), records the verdict in the returned bytes, THEN closes the connection so the final `AssertStats` sees the quiesced `== 0`. **PLAN/IMPL note:** if threading the admin addr into `driveProxy` is awkward, fall back to asserting the unanswered gauge `== 1` purely in the SUBJECT-side unit tests (Task 6 already proves the open-connection gauge == 1 at the decoder level) and keep `0051`'s gauge arms to the answered `== 0` + the post-close `== 0` cross-side (still proving the inc+dec+drain round-trip). The IMPL chooses based on the runner's admin-addr availability inside the Drive* methods; record the choice in the README. The load-bearing cross-side gauge proof is answered→0 (R-GAUGE); the unanswered→1 is best-effort cross-side + guaranteed unit-side.

Implement `AssertStats(t, refAdminAddr, subjAdminAddr)` with the cumulative expectations (the `0049` AssertStats structure: `scrapeMongoStats` both sides → a `[]struct{key string; want int64}` checked against both maps, ABSENT reported distinctly from wrong-value):

```go
expectations := []struct{ key string; want int64 }{
	// op_reply: arm1(1) + arm2 cursorNotFound(1) + queryFailure(1) + validCursor(1)
	//           + arm5 uncorrelated(1) + arm6 malformed... NO (malformed → decoding_error, not op_reply).
	//   plain(1)+cnf(1)+qf(1)+vc(1)+uncorrelated(1)+withhold(0, no reply) = 5
	{`envoy_mongo_op_reply{envoy_mongo_prefix="mongo_r"}`, 5},
	{`envoy_mongo_op_reply_cursor_not_found{envoy_mongo_prefix="mongo_r"}`, 1},
	{`envoy_mongo_op_reply_query_failure{envoy_mongo_prefix="mongo_r"}`, 1},
	{`envoy_mongo_op_reply_valid_cursor{envoy_mongo_prefix="mongo_r"}`, 1},
	{`envoy_mongo_op_command_reply{envoy_mongo_prefix="mongo_r"}`, 1},
	{`envoy_mongo_decoding_error{envoy_mongo_prefix="mongo_r"}`, 1}, // arm 6 malformed reply
	// op_query re-proves the request surface under the response load (arms 1,2×3,4ii,5 = 6 queries; arm3 is OP_COMMAND).
	{`envoy_mongo_op_query{envoy_mongo_prefix="mongo_r"}`, 6},
	{`envoy_mongo_op_command{envoy_mongo_prefix="mongo_r"}`, 1},
}
```
> **The arm-accounting table is the GROUND TRUTH** — author it as a comment block above `driveProxy` (the `0049` discipline) and re-verify the `want` values LIVE cross-side when the fixture first runs. The exact op_reply/op_query totals depend on the final arm set the IMPL lands; the table is authored to match the arms exactly, then proven by the cross-side run (if the reference disagrees, the table is wrong — fix the table, not the assertion). Then the exists-at-zero + gauge-TYPE + `cx_destroy_*` presence block (copied from `0049` AssertStats, retargeted to the single `mongo_r` prefix):
```go
// op_query_active gauge: PRESENT, # TYPE … gauge, == 0 at rest (all conns closed).
// cx_destroy_local/remote_with_active_rq: PRESENT both sides, value NOT compared
//   (D-P4 close-direction coverage boundary — §3.6; the reference increments one
//   per query-bearing close, envoy-go increments neither until 29.3).
// delays_injected, cx_drain_close: PRESENT, == 0 both sides.
```

- [ ] **Step 2: Author the `0051` driver-internal helpers**

`driveAndReadReply(ctx, addr, frame)` — dial, write the frame, read back any reply bytes (a bounded `conn.Read` with a short deadline so a withheld reply doesn't block forever), close. This makes the answered arms quiesce before the connection closes (D-P9). Add a driver-local `settleDelay` (the `0049` 750ms) before `driveProxy` returns.

- [ ] **Step 3: Register the driver blank-import**

In `runner_test.go`, add the `0051` driver blank-import beside the `0049`/`0050` imports:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0051-mongo-responses/driver"
```

- [ ] **Step 4: Run the `0051` fixture cross-side**

```bash
go test ./test/differential/ -run 'Differential/0051-mongo-responses' -count=1 -v
```
Expected: PASS both sides — all arms green; the gauge answered→0 + cx_destroy presence + decoding_error parity hold. (Requires Docker for the reference Envoy per `reference_docker_probe_bridge_network`; the responder backend is a local goroutine reachable from both the dockerized reference [via `host.docker.internal`] and the subprocess.)

- [ ] **Step 5: The R4 deliberate-break liveness proof (`-count=1`)**

Record THREE breaks in the README + `PROGRESS.md` (each reverted; `reference_differential_break_protocol_count1`):
- (a) temporarily assert `op_reply == 6` (when 5 is received) → MUST fail on BOTH runner paths with `-count=1`.
- (b) temporarily skip the gauge `Dec` at the correlated reply (`codec.go` `decodeReply` correlation block) → MUST fail the answered `op_query_active == 0` arm (subject-side).
- (c) temporarily disable the gauge `Inc` (`appendQuery`) → MUST fail the unanswered `== 1` arm (or the subject-side unit lifecycle test if the gauge arm is unit-only per Step 1's note).
```bash
go test ./test/differential/ -run 'Differential/0051-mongo-responses' -count=1   # asserts the break FAILS
```
Revert all three; re-run green.

- [ ] **Step 6: Author the README + commit 11A**

`test/fixtures/0051-mongo-responses/README.md` — the fixture envelope (the `0049` README shape): topology `[mongo_proxy, tcp_proxy]` → `TCPMongoResponder`; the 9 arms; the gauge quiesced-point design; the `cx_destroy_*` presence-only (D-P4) note; the "NO dynamic-metadata fixture surface — proven by unit tests only (§3.7)" note; the R4 break record.
```bash
gofmt -l test/fixtures/0051-mongo-responses/ test/differential/
golangci-lint run ./test/differential/... ./test/fixtures/0051-mongo-responses/...
git add test/fixtures/0051-mongo-responses/ test/differential/runner_test.go
git commit -m "phase 29.2 Task 11A: fixture 0051-mongo-responses cross-side green (gauge quiesced-point arms; cx_destroy presence; R4 break)"
```

### 11B — the completion bundle (atomic, per ADR-0052)

- [ ] **Step 7: Land the ADR-0225 §Decision/§Consequences body IN PLACE**

In `docs/envoy-go/DECISIONS.md`, fill the ADR-0225 §Decision + §Consequences bodies (the §Context + the 2026-06-04 D-P4 AMEND already landed at the SPEC commit; NO new ADR number — tail STAYS ADR-0226). The body's blueprint is SPEC §3 (the OnWrite feed §3.1; the OP_REPLY/OP_COMMANDREPLY decode + the 5 counters §3.2; correlation §3.3; the gauge §3.4; the per-connection mutex §3.5; the D-P4 close-direction coverage boundary §3.6; the dynamic-metadata single-Set Bucket §3.7) + §5 + §6. Record the resolved D-questions: D-S29.2-3 (single-Set model), D-S29.2-4 (sniffing atomic.Bool + CAS), D-S29.2-5 (snapshot-count drain). Forward-pointer to 29.3 (the close-direction accessor + `cx_destroy_*` value parity; fault delay; access log; drain; the ROLLUP).

- [ ] **Step 8: Land the BEHAVIOR_CONTRACT 29.2 bundle (§8)**

In `docs/envoy-go/BEHAVIOR_CONTRACT.md`, extend the `### envoy.filters.network.mongo_proxy` subsection: the response-side decode semantics; the 5 response counters increment-active; the `op_query_active` gauge inc/dec lifecycle (the project's FIRST mirrored gauge); the requestID↔responseTo correlation + the per-connection-mutex concurrency note; the dynamic-metadata emission (namespace + collection→ops + per-pass clear + the differential-invisible note); the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary CLOSED. NEW coverage boundaries: the dynamic-HISTOGRAM families (ADR-0060 — recorded HERE per §5.3); the `cx_destroy_*` close-direction boundary (D-P4 — value parity → 29.3); the dynamic-metadata differential-invisibility (AMEND-B11). Stat table: **360 → 360** (+0 creation — explicitly a no-creation increment-wiring delta).

- [ ] **Step 9: Advance STATE.md + ROADMAP.md**

- `STATE.md`: active-phase → `phase 29.3 SPEC` (the next cold-start); `last-commit` → the 29.2-IMPL squash SHA (filled post-squash); `next-skill` → the 29.3 SPEC authoring path; counts → fixtures **53**, fuzzers **39**, stats **360**, DECISIONS tail **ADR-0226** (next-free **ADR-0227**); BackendKind tail **30**.
- `ROADMAP.md`: sub-row **29.2 `in-progress → done`**; **parent row 29 STAYS `in-progress`** (the ROLLUP is 29.3's); 29.3 STAYS `planned`.

- [ ] **Step 10: The six-gate (per §13.2) — quote every output into PROGRESS.md**

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./... -race -short
go test ./test/differential/ -count=1               # the FULL 53-dir suite byte-exact (incl. the 52-dir back-compat gate)
# h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — 29.2 touches no HTTP path)
```
Expected: build/vet/lint clean; `-race -short` green; all 53 differential dirs green; conformance unchanged. Record fixtures **53** (`ls -d test/fixtures/[0-9]* | wc -l`), fuzzers **39**, stats **360**.

- [ ] **Step 11: Rewrite `next-prompt.txt` for the 29.3-SPEC cold-start**

Rewrite `next-prompt.txt` to open the **29.3 SPEC** (`superpowers:writing-plans`/SPEC-authoring per SKILL_ROUTING; the async halt/resume seam + fault delay + access log + `cx_drain_close` + the deferred close-direction seam + the parent-row-29 ROLLUP; ADR-0226 body lands at 29.3 IMPL). Name the live master tip + the 29.2-IMPL squash SHA (filled post-squash). Carry the project memories (worktree / push / subagents-no-push / per-task gofmt-lint / wire-format-both-sides / the D-P4 close-direction framework-gap boundary 29.3 CLOSES).

- [ ] **Step 12: Commit the bundle**

```bash
gofmt -l . 2>/dev/null | grep -v vendor || true
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt PROGRESS.md
git commit -m "phase 29.2 IMPL completion bundle: ADR-0225 body + BEHAVIOR_CONTRACT 29.2 + STATE/ROADMAP (sub-row 29.2 done; parent 29 stays in-progress) + six-gate"
```

> **Controller (post-IMPL):** squash-merge the 29.2-IMPL branch to master, fill the squash SHA into STATE.md `last-commit` + next-prompt.txt, and PUSH (`feedback_push_to_origin`). Subagents commit LOCAL-ONLY (`feedback_subagents_no_push`).

---

## Plan Review + Execution Handoff

After this PLAN lands, the controller:
1. Dispatches the `plan-document-reviewer` subagent (plan path + SPEC path; not session history). Folds advisories; re-dispatches on ❌ (≤3 iterations).
2. On ✅, advances STATE.md (active-phase → `phase 29.2 PLAN done`; next-skill → the 29.2 IMPL via `superpowers:subagent-driven-development` per SKILL_ROUTING state 3 + `feedback_execution_style`); ROADMAP sub-row 29.2 STAYS `in-progress`; rewrites `next-prompt.txt` for the **29.2-IMPL** cold-start; commits + squash-merges + pushes.
3. The 29.2 IMPL runs **subagent-driven**: a fresh subagent per task (Tasks 1–11), two-stage review between tasks; subagents commit LOCAL-ONLY; the controller squash-merges + pushes at stage-close.
