# Phase 29.2 SPEC — `mongo_proxy` response side + correlation + the `op_query_active` gauge

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 29.2** (`network-filter-mongo-responses-and-correlation`), the SECOND of the phase-29 BRAINSTORM-time 3-way pre-split (29.1 / 29.2 / 29.3). It is authored per the phase-22.2 / phase-25.x / phase-28.2 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/29-network-filter-mongo-proxy/SPEC.md`) is this sub-phase's MASTER — its §3.2 (the 29.2 scope), §5 (proto roster), §7 (the 23-stat roster + the response/gauge created-at/incremented-at split), §8 (the fixture envelope), §11.4/§11.5 (the OP_REPLY/OP_COMMANDREPLY framing + correlation + the private-buffer model), §11.9 (dynamic metadata), §11.10 (close direction), and §12 (the 29.2-owned D-questions) remain authoritative; this SPEC EXECUTES + REFINES them into the per-Task surface. The phase-29 parent BRAINSTORM already drafted the ADR-0225 §Context (the 29.2 charter, `DECISIONS.md:14470`), so this sub-phase is **authored, NOT re-brainstormed** (the 28.2-SPEC-after-28.1-IMPL precedent). The next session, per BOOTSTRAP §5, authors the **29.2 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Complete `mongo_proxy`'s round-trip observability — wire the RESPONSE-side decoder into `OnWrite` (OP_REPLY/OP_COMMANDREPLY body decode + the 5 response-side counter increments), correlate replies to the 29.1 active-query list (requestID↔responseTo first-match-erase) under the ADR-0223 per-connection `sync.Mutex`, drive the project's FIRST differentially-mirrored gauge (`op_query_active` inc/dec), emit the dynamic-metadata Struct onto the ADR-0217 Bucket, and prove it cross-side with fixture `0051-mongo-responses` (the new `TCPMongoResponder` backend; gauge quiesced-point assertions) — at **ZERO** framework touch (the 28.2 zero-touch property) and **+0** stat creation (all 23 stats were created eagerly at 29.1).

**Architecture:** A mongoproxy-package + test-surface change ONLY (the 28.2 shape). The 29.1 per-connection `decoder` gains a write-side private reassembly buffer (`writeBuf`) + a `sync.Mutex`; `OnWrite` stops being a no-op and feeds the response decoder. The active-query list (`dec.queries`, written by 29.1 OnData, never read) becomes cross-goroutine state — pump A (`OnData`, request decode) appends; pump B (`OnWrite`, response decode) reads/erases — so the ADR-0223 minimal-critical-section mutex guards EXACTLY that list. The gauge increments ride the list lifecycle (inc at append, dec at correlated-reply erase + at connection-destroy teardown). `cx_destroy_*_with_active_rq` stay exist-at-zero (the D-P4 close-direction seam defers to 29.3 — §3.6). Dynamic-metadata emission writes the ADR-0217 Bucket on the REQUEST decode pass (insert/query, per upstream), cleared per pass. ZERO changes to `internal/filter/network/` framework files, `manager.go`, `tcp_proxy`, or HCM.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); the as-built `internal/filter/network/mongoproxy/` package (29.1 — extended in place); `internal/stats/` (`Gauge` Inc/Dec/Load — 06.1; `NewCounterIfAbsent`/`NewGaugeIfAbsent` — consumed, not modified); `internal/dynamicmetadata/` (the ADR-0217 Bucket — a minimal per-namespace clear may be added, §3.7); the differential harness + `fixture.StatsAsserter` + the existing label-aware `0049` scrape mechanics (the gauge-value + `# TYPE` scrape helpers already exist). ZERO new third-party `go.mod` dependencies.

**Authored:** 2026-06-04. **Empirical-pin probe date (inherited):** 2026-06-03 (parent SPEC §11 + the 29.1 SPEC §11.2 D-P2 re-probe). **Baseline-anchor re-pin date:** 2026-06-04 (this SPEC session — §9.1).

---

## 1. Purpose / Mission

Phase 29.2 delivers the mongo_proxy response side + correlation + the gauge (parent §3.2 item "29.2"; ADR-0225):

1. **The response decoder (ADR-0225).** The 29.1 `OnWrite` no-op stub is replaced by a response-decode feed over a NEW write-side private reassembly buffer: OP_REPLY(1) body decode (responseFlags + cursorID + startingFrom + numberReturned + reply documents) + OP_COMMANDREPLY(2011) body decode (metadata + commandReply + outputDocs); the 5 response-side fixed counters (`op_reply`, `op_reply_cursor_not_found` [flag 0x01], `op_reply_query_failure` [flag 0x02], `op_reply_valid_cursor` [cursorID ≠ 0], `op_command_reply`) wired live. These are the Reply(1)/CommandReply(2011) opcodes the 29.1 decoder RECOGNIZED-NOT-DECODED (valid envelope, consumed, NOT a `decoding_error`); 29.2 wires their body decode + counters (§3.2).

2. **requestID↔responseTo correlation + the per-connection mutex.** The 29.1 active-query list (`dec.queries`) is consumed: an OP_REPLY's `responseTo` is matched against a stored query's `requestID` (first-match-erase; only OP_QUERY created entries; uncorrelated replies charge ONLY the fixed `op_reply*` counters). The list is now cross-goroutine state → the ADR-0223 per-connection `sync.Mutex` guards it (§3.3 + §3.5).

3. **The `op_query_active` gauge — the project's first differentially-mirrored gauge.** `Inc` at OP_QUERY decode (the 29.1 list-append site), `Dec` at a correlated reply AND at connection destroy (the list teardown). The fixture asserts the gauge at QUIESCED points (all queries answered → 0 both sides; an unanswered-query arm → 1 both sides) — the D-P9 gauge-value parity design (§3.4 + §6.2).

4. **`emit_dynamic_metadata` emission (the ADR-0217 Bucket — third production writer).** Per decode pass, a Struct keyed by collection name → ListValue of operation strings (`"insert"`/`"query"` in v1.37.2), cleared at the top of every pass, under namespace `envoy.filters.network.mongo_proxy`. Differential-invisible → unit-test proof + a BEHAVIOR_CONTRACT note (§3.7).

5. **The integration surface** — (a) fixture **`0051-mongo-responses`** (cross-side `StatsAsserter`; the new `TCPMongoResponder` backend — BackendKind 30 — emitting correlated OP_REPLY bytes; gauge quiesced-point arms; `cx_destroy_*` presence-only); (b) the **39th fuzzer extended** to the response opcodes (D-P6 — no 40th); (c) the ADR-0225 §Decision/§Consequences body + the §Context D-P4 AMEND, the BEHAVIOR_CONTRACT 29.2 bundle, and the STATE/ROADMAP advance (sub-row 29.2 `in-progress → done`; parent row 29 STAYS `in-progress` — the ROLLUP is 29.3's).

After phase 29.2, the project has: a FULLY round-trip-observable `mongo_proxy` (request + response counter parity + the first mirrored gauge); the active-query list consumed (R5 ratified end-to-end); the dynamic metadata emitted. 29.3 then lands the async halt/resume seam + fault delay + the access log + `cx_drain_close` + the deferred close-direction seam (this SPEC's D-P4 boundary) + the parent ROLLUP.

### 1.1 Parent AMENDs + 29.1 outputs load-bearing for 29.2

- **AMEND-B1** (`mongo.<stat_prefix>.<counter>` scope) — the response counters + the gauge live under the same scope; the 29.1 four-rule `name.go` arm ALREADY handles `envoy_mongo_op_reply*` + the gauge `# TYPE … gauge` line (§5.2).
- **AMEND-B3** (the roster is EXACTLY 22 counters + 1 gauge) — the 5 response counters + the gauge + the 2 `cx_destroy_*` were CREATED eagerly at 29.1; 29.2 wires increments only → **+0 creation** (§5.1).
- **AMEND-B11 / §11.9** (dynamic-metadata shape — namespace `envoy.filters.network.mongo_proxy`; collection → ListValue of ops; `"insert"`/`"query"` only; per-pass clear) — §3.7.
- **AMEND-B12 / §11.10** (`cx_destroy_*_with_active_rq` need close DIRECTION; an as-built framework gap) — §3.6 (D-P4 RESOLVED: coverage boundary).
- **Parent §11.4 correlation pin** (ONLY OP_QUERY creates entries; OP_REPLY correlates `query.requestID == reply.responseTo`, first match erased) — §3.3.
- **Parent §11.4/§11.5 wire layout** (OP_REPLY = responseFlags(int32) + cursorID(int64) + startingFrom(int32) + numberReturned(int32) + N BSON docs; OP_COMMANDREPLY = metadata(BSON) + commandReply(BSON) + 0..N outputDocs; reply flags 0x01/0x02; valid_cursor = cursorID ≠ 0; the private-buffer copy/sniff model; sniffing-off-on-error) — §3.1/§3.2.
- **29.1 outputs consumed:** the active-query list (`activeQuery{requestID, collection, command, callsite, start}` — `codec.go:30-36`; written every OP_QUERY, never read); the eager 23-stat roster (`stats.go` — the 5 reply counters + the `op_query_active` gauge + the 2 `cx_destroy_*` all exist-at-zero); the `mongo.` four-rule `name.go` arm (handles the fixed reply counters + the gauge TYPE line — no new arm); the `OnWrite` no-op stub + the 29.2 forward-pointer comments (`filter.go:63-68`, `codec.go:42-45`, `codec.go:122-125`).

### 1.2 29.2-SPEC-additive contributions (what this document pins beyond the parent + 29.1)

- **§3.6 D-P4 RESOLVED: COVERAGE BOUNDARY (close-direction seam deferred to 29.3).** The parent AMEND-B12 / D-P4 left "a minimal close-direction accessor OR a coverage boundary." The as-built investigation (§9.1) confirms close DIRECTION is genuinely NOT observed by the framework today — `OnDestroy()` carries none, and the post-handoff close is detected inside `tcp_proxy`'s two pump goroutines (via EOF) which record neither which side closed nor expose it. Threading it therefore requires a `tcp_proxy` + `chain.go` touch — NOT the "small accessor the chain runtime already observes" the parent anticipated. To keep 29.2 **framework-zero-touch** (the 28.2 precedent) and avoid a second isolated ripple, D-P4 resolves to a coverage boundary: `cx_destroy_*_with_active_rq` stay exist-at-zero / PRESENCE-ONLY in `0051` (extending the 29.1 AMEND-C2 posture), and the close-direction accessor + value parity fold into **29.3**, where ADR-0226's async halt/resume seam ALREADY opens the pump/terminal/`chain.go` area (one ripple — the ADR-0219 no-ripple discipline). Recorded as a one-line in-place AMEND on ADR-0225's §Context at the SPEC commit (the ADR-0223-at-28.2-SPEC precedent).
- **§3.5 The active-query-list mutex** — the ADR-0223 pattern applied to mongo's narrower correlation surface (one slice, not two maps): a per-connection `sync.Mutex` on the decoder guarding EXACTLY `dec.queries`; entries copied out / erased under the lock; counter + gauge math OUTSIDE it; the pre-handoff request path locks too (uniformity).
- **§3.4 The gauge mirror** — the first differentially-compared gauge: the inc/dec lifecycle pinned (inc at append, dec at correlated-reply erase, dec-per-remaining at destroy), the atomic-gauge-outside-the-lock discipline, and the D-P9 quiesced-point assertion design.
- **§6.1 The `TCPMongoResponder` BackendKind = 30** — the next free value after `TCPZKResponder = 29` (the 28.2 precedent); a mongo-aware canned-response backend emitting correlated OP_REPLY frames so the reference's response decoder fires.
- **D-P6/P9/P10/P11 RESOLVED** (§10.1): the fuzzer EXTENDS the 39th (no 40th); the gauge quiesced-point design = driver-waits-for-reply-round-trip; the StatsAsserter gauge support works as-is (the `0049` scrape helpers already parse gauge values + `# TYPE`); the dynamic-metadata proof = unit-test-only + a coverage note.

---

## 2. Non-purposes

Phase 29.2 does NOT extend any subsystem beyond the minimum needed to land the response side under ADR-0225.

- **2.1 Fault-delay / the async halt/resume seam OUT OF SCOPE.** The `delay` field (parsed + PGV-validated at 29.1) is CONSUMED at 29.3 (ADR-0226: the ACTIVE `ContinueReading` + cross-goroutine safety + post-handoff read-halt honoring + `delays_injected`). 29.2's `OnData` still ALWAYS returns `Continue`; `OnWrite` ALWAYS returns `Continue` (the response decoder never halts — upstream `onWrite` parity, parent §11.5).
- **2.2 Access-log + `cx_drain_close` OUT OF SCOPE.** `access_log` (parse-stored at 29.1) is consumed at 29.3 (AMEND-B10 unit-test fallback); `cx_drain_close` (reply-completion drain close) is 29.3's.
- **2.3 The close-direction seam OUT OF SCOPE (D-P4 → 29.3).** `cx_destroy_local/remote_with_active_rq` get NO increment at 29.2 — exist-at-zero, presence-only in `0051` (§3.6). The framework stays untouched.
- **2.4 The framework is UNTOUCHED.** ZERO changes to `internal/filter/network/` (chain.go / readconn.go / writeconn.go / types.go / callbacks.go / terminal.go / registry.go), `internal/listener/manager.go`, `tcp_proxy`, HCM, or `internal/stats/` (the gauge primitive + the four-rule name.go arm are consumed, not modified). The ONLY out-of-`mongoproxy/` production touch CONSIDERED is a minimal `internal/dynamicmetadata/` per-namespace clear helper (§3.7) — a shared-primitive addition (NOT a network-framework change), and only if the SPEC's preferred single-Set model (§3.7) is not adopted at IMPL.
- **2.5 The dynamic HISTOGRAM families** (`cmd.<cmd>.reply_*`, `collection.<c>.query.reply_*`, callsite `reply_*`) — deferred per ADR-0060; the coverage-boundary record lands in the 29.2 bundle (this is the response side that would populate them — parent §7.2 / §9; the 29.1 SPEC §2.5 deferred the record HERE).
- **2.6 No new fixed stats; no new `name.go` arm; no new built-in.** All 23 stats were created at 29.1; the `mongo.` four-rule arm already handles the reply counters + the gauge; mongoproxy is already the 8th built-in. 29.2 wires increments + the response decode only.
- **2.7 No real-MongoDB-server fixtures; no OP_MSG corpus; no histograms; no per-route surface; no new conformance harness** — all per parent §2.
- **2.8 The parent-row-29 ROLLUP is NOT 29.2's.** Parent row 29 STAYS `in-progress` after 29.2; the rollup (parent + sub-row 29.3 → done ATOMICALLY) is 29.3's (the parent §3 / 28.x non-final-sub-phase precedent).
- **2.9 No retroactive zookeeper changes.** The zookeeperproxy package, its fixtures, and its decoder are untouched.

---

## 3. The response side + correlation + the gauge + metadata (ADR-0225)

Extends the as-built `internal/filter/network/mongoproxy/` package (29.1) IN PLACE. The decoder is already direction-agnostic in shape (`decodeMessage` dispatches by opcode — `codec.go:107-132`); 29.2 wires the two response opcodes' bodies + the correlation/gauge/metadata.

### 3.1 The `OnWrite` response feed (replaces the 29.1 no-op stub)

The 29.1 `OnWrite` was a PURE NO-OP that did not even buffer (`filter.go:63-68`). 29.2 replaces the body with a write-side decode feed over a NEW private buffer:

```go
// OnWrite feeds the response decoder the write-direction (upstream→downstream)
// bytes and ALWAYS returns Continue (R3 extends to the write side; upstream
// onWrite parity — never halts). Replaces the 29.1 no-op stub.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	return network.Continue
}
```

**No write-side `TotalAppended` high-water mark (the 28.2 structural-asymmetry pin, ADR-0223 §Decision item 1).** `writeChainConn.Write` (ADR-0221; `writeconn.go:34-48`) allocates a FRESH per-`Write` `*Buffer` for each `OnWrite` call, so every upstream→downstream byte arrives EXACTLY ONCE as its own `buf.Bytes()` slice. The decoder appends `buf.Bytes()` directly to its private `writeBuf` — NO `chainConsumed`-style high-water tracking (that read-side machinery exists only because the runtime drains the SHARED chain buffer mid-stream; that draining is structurally absent on the write side). This is the deliberate asymmetry vs the read path, recorded in ADR-0225's body.

`decodeOnWrite(p []byte)`: `append(writeBuf, p...)` → loop `nextWriteMessage()` (the `nextMessage` shape: 16-byte LE header readable AND `len(writeBuf) >= messageLength`; `messageLength < 16` → decode error; partial frame → wait, never an error) → `decodeResponseMessage(m)` per complete frame. A decode failure runs the SAME `decoding_error`/sniffing-off path the request side uses (`decoderError()` — `codec.go:137-144`): one `decoding_error` increment (at most once per connection, shared with the read path via the single `sniffing` flag), `sniffing = false`, and BOTH private buffers released. The connection is never closed; `OnWrite` always returns `Continue`.

**Sniffing is connection-lifetime + direction-shared (AMEND-B6).** The single `decoder.sniffing` flag governs BOTH directions: once any decode (request or response) errors, sniffing goes off for the connection and `decodeOnWrite` (like `decodeOnData`) stops decoding and drops bytes. The flag is now touched on BOTH goroutines (A reads/writes it on the request path; B reads/writes it on the response path), so it REQUIRES synchronization — even though it only ever transitions monotonically true→false, an unsynchronized cross-goroutine read/write still trips the `-race` detector. The pin is "race-clean"; the mechanism (read/write under `mu` alongside the list — the conservative default, since the response path already takes the lock per frame — vs an `atomic.Bool`) is an IMPL choice the `-race` test settles (D-S29.2-4).

### 3.2 Response dispatch + the OP_REPLY/OP_COMMANDREPLY decode (parent §11.4)

The 29.1 `decodeMessage` already routes `opReply`/`opCommandReply` to a recognized-not-decoded `return true` (`codec.go:122-125`). 29.2 replaces that arm with body decode. All multi-byte reads little-endian (`bsonReader` — reused verbatim from 29.1's `bson.go`).

**OP_REPLY (opcode 1) body** (after the 16-byte MsgHeader; `responseTo` is `m[8:12]`): responseFlags(int32) → cursorID(int64) → startingFrom(int32) → numberReturned(int32) → exactly `numberReturned` BSON documents (each `parseDocument`). From the decoded fields:

- `op_reply` +1 (every decoded OP_REPLY).
- responseFlags bit `0x01` (CursorNotFound) → `op_reply_cursor_not_found` +1.
- responseFlags bit `0x02` (QueryFailure) → `op_reply_query_failure` +1.
- cursorID ≠ 0 → `op_reply_valid_cursor` +1.
- Correlation by `responseTo` (§3.3); a correlated hit → the gauge `Dec` (§3.4).
- A malformed reply body (bad BSON, underflow, `numberReturned` mismatch) → the `decoding_error`/sniffing-off path (parent §11.5; the request-side `fail()` shorthand reused).

**OP_COMMANDREPLY (opcode 2011) body** (parent §11.4): metadata(BSON) → commandReply(BSON) → 0..N outputDocs(BSON, loop to end of body) → `op_command_reply` +1. OP_COMMANDREPLY does NOT correlate against the active-query list (upstream: only OP_REPLY correlates against `ActiveQuery`; OP_COMMAND requests never created list entries — parent §11.4 item 7) and does NOT touch the gauge.

**Dispatch (the 29.2 edit to `decodeMessage`):** `case opReply: return d.decodeReply(responseTo, body)` / `case opCommandReply: return d.decodeCommandReply(body)`. The `responseTo` int32 is read from `m[8:12]` (the 29.1 `decodeMessage` reads `requestID` from `m[4:8]` and `opCode` from `m[12:16]`; the response path adds the `responseTo` read). Everything else in `decodeMessage` is unchanged.

### 3.3 Correlation consumption (R5 ratified; parent §11.4 item 7)

Upstream correlation is NARROWER than zookeeper's (one list, no control/data split): ONLY OP_QUERY messages created `ActiveQuery` entries at 29.1; an OP_REPLY correlates by `query.requestID == reply.responseTo`, FIRST MATCH erased, loop breaks; uncorrelated replies charge ONLY the fixed `op_reply*` counters.

The 29.1 list is `dec.queries []activeQuery` (`codec.go:52`, `activeQuery{requestID, collection, command, callsite, start}`). The 29.2 correlation:

```go
// takeQuery removes + returns the FIRST active query whose requestID matches the
// reply's responseTo (upstream first-match-erase). Holds mu for the scan + erase
// ONLY; the returned copy is used (gauge Dec + future latency) outside the lock.
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

- A hit → the gauge `Dec` (§3.4), OUTSIDE the lock (the entry is already copied out). The `start time.Time` rides in the copy for the 29.3-deferred latency (recorded at 29.1; never consumed at 29.2 — no histogram).
- A miss → no gauge change; the fixed `op_reply*` counters were already charged. Uncorrelated replies are upstream-normal (a reply with no pending query — e.g. an unsolicited or already-answered reply).
- **No second-response-same-requestID handling beyond first-match-erase** (mongo, unlike zookeeper's data map, does not treat a duplicate as a `decoder_error` — upstream just finds nothing and charges fixed counters; the loop-and-erase naturally yields a miss on the second).

### 3.4 The `op_query_active` gauge — the project's FIRST differentially-mirrored gauge

The `op_query_active` gauge was CREATED at 29.1 (`stats.go:49`, `reg.NewGaugeIfAbsent(...)`; `mongoStats.opQueryActive`). 29.2 wires the increments mirroring upstream's `ActiveQuery` ctor/dtor accounting (parent §7.2: "inc at `ActiveQuery` construction, dec at destruction"):

- **`Inc` at OP_QUERY decode** — the 29.1 list-append sites (`codec.go:225` for the $cmd path, `codec.go:265` for the non-command path) each gain `d.stats.opQueryActive.Inc()` alongside the `append`. The append + the Inc happen under the request-path lock (§3.5); the atomic `Inc` itself is lock-free but is co-located for a consistent list-size↔gauge invariant.
- **`Dec` at a correlated reply** — `takeQuery` hit (§3.3) → `opQueryActive.Dec()` outside the lock.
- **`Dec` at connection destroy** — `OnDestroy` drains the remaining list under the lock (the queries never answered before close) and `Dec`s the gauge once per drained entry, so the gauge returns to 0 when the connection ends. This mirrors upstream's `ActiveQuery` destructor running for every still-live entry at filter teardown.

**Gauge math is atomic + OUTSIDE the critical section (the ADR-0223 discipline).** The `stats.Gauge` is an `atomic.Int64` (`gauge.go` — `Inc`/`Dec`/`Add`/`Load`, lock-free). The DECISION to inc/dec depends on the list (mutex-guarded), but the gauge op itself runs after the lock is released (correlated-reply Dec) or while draining a copied count (destroy). The list-size↔gauge invariant (`len(queries)` == gauge value, per connection, modulo in-flight) is the unit-test + race-test anchor (§3.5).

**The D-P9 quiesced-point assertion design (parent D-P9 RESOLVED).** The differential gauge comparison is deterministic IFF scraped at a QUIESCED point — no reply in flight. The `0051` driver achieves this by WAITING for each reply to round-trip (the `TCPMongoResponder` echoes a correlated OP_REPLY; the driver reads the reply bytes back before scraping) so the gauge has settled. Two arm shapes: (i) an ANSWERED arm — query + matching reply received + connection at rest → gauge `op_query_active == 0` both sides; (ii) an UNANSWERED arm — query sent, the responder configured to withhold the reply for that requestID, connection still open at scrape → gauge `== 1` both sides (then closed; a post-close re-scrape returns to 0 via the destroy teardown). See §6.2.

### 3.5 The per-connection `sync.Mutex` (the ADR-0223 pattern; LOAD-BEARING)

**The race.** At 29.1 the active-query list was single-goroutine (written only on the read/request path; never read). At 29.2 it becomes cross-goroutine: post-handoff, goroutine A (downstream→upstream pump → `readChainConn.Read` → `replayRead` → `OnData` → request decode) APPENDS entries (+ gauge Inc); goroutine B (upstream→downstream pump → `writeChainConn.Write` → `OnWrite` → response decode) READS + ERASES entries (+ gauge Dec). Lock-free → a Go concurrent slice read/write/realloc → a runtime data race. Upstream has no race only because libevent serializes both directions onto one dispatcher thread; envoy-go's two pumps are genuinely concurrent (the 28.2 §3.6 analysis, mongo-specialized).

**The design (mirrors ADR-0223 / 28.2 §3.6, narrowed to one list).** ONE per-connection `sync.Mutex` on the `decoder` guards EXACTLY `dec.queries` (and `dec.sniffing` per D-S29.2-4) — nothing else:

| State | Owner | Locking |
|---|---|---|
| `queries []activeQuery` | shared (A appends; B reads/erases; OnDestroy drains) | **`decoder.mu`** — every list access holds the lock; entries COPIED OUT under the lock; gauge `Inc`/`Dec` + counter increments happen OUTSIDE it |
| `sniffing` bool | shared (A + B set it false on decode error; both read it) | read/written under `mu` (the conservative pin — the response path already takes the lock per frame; D-S29.2-4) |
| `readBuf` + `chainConsumed` | goroutine A only | lock-free |
| `writeBuf` | goroutine B only | lock-free |
| `cfg` / `stats` (counters + the gauge) | shared, read-only / atomic | lock-free (`Counter`/`Gauge` ops atomic; `compiledConfig` immutable post-boot) |
| the DynamicMetadata Bucket | goroutine A only (insert/query emit on the request pass — §3.7) | lock-free (per-connection, request-path-sequential) |
| `filter.dec` pointer | `OnDestroy` nils strictly AFTER both pumps join | lock-free (the ADR-0221 happens-after edge: `tcpproxy` `wg.Wait()` precedes `OnDestroy`) |

**Pinned consequences (the ADR-0223 carry-over):**
1. **Lock granularity is per-list-access, not per-frame** — held only for the scan + erase (or the append), never across counter/gauge math or BSON parsing.
2. **`activeQuery` copied by value under the lock** (`takeQuery` returns a copy); the response path then operates on its private copy.
3. **The pre-handoff request path ALSO takes the lock** (uniformity over cleverness — pre-handoff the lock is uncontended, one atomic CAS; removes "which regime am I in" branching). The 29.1 `codec.go` list-append sites + the gauge Inc move inside `d.mu.Lock()/Unlock()`.
4. **`OnDestroy` needs no lock for the pointer drop** but DOES take `mu` to drain the residual list + Dec the gauge (it runs strictly after both pumps join, so the lock is uncontended; taking it is harmless uniformity and keeps the drain correct if the join edge is ever weakened).
5. **The race test** (NEW, `codec_test.go` or `filter_test.go`): two goroutines over one decoder — one driving `decodeOnData` with a request stream, one driving `decodeOnWrite` with the matching response stream — under `go test -race -count=5`. Removing `mu` MUST trigger an immediate `-race` report (empirically load-bearing, the 28.2 R9 precedent).

The 29.1 `codec.go:42-45` forward-pointer comment ("NO mutex at 29.1 … the ADR-0223 per-connection mutex arrives at 29.2 with the cross-goroutine OnWrite reader") marks the site; 29.2 lands the `mu sync.Mutex` field on the `decoder` struct.

### 3.6 `cx_destroy_*_with_active_rq` — the D-P4 coverage boundary (close-direction deferred to 29.3)

Upstream keys `cx_destroy_local_with_active_rq` vs `cx_destroy_remote_with_active_rq` on the `Network::ConnectionEvent` (LocalClose vs RemoteClose) delivered to `onEvent` while the active-query list is non-empty (parent §11.10; `proxy.cc:355-376`). envoy-go's as-built framework does NOT expose close direction to a network filter:

- `ReadFilter.OnDestroy()` carries no event/reason (`types.go:29-48`); `chainRuntime.onDestroy` (`chain.go:405-420`) calls it with no direction.
- The framework records close TYPE (FlushWrite / NoFlush — `callbacks.go:40-57`, `chainRuntime.closeType`), NOT who closed.
- For a `[mongo_proxy, tcp_proxy]` chain, the post-handoff close is detected inside `tcp_proxy`'s two pump goroutines (`tcpproxy/filter.go:134-139` — each `io.Copy` returns on its side's EOF) which record neither direction nor expose it; the pure-read close loop (`manager.go:1073-1079`) sees only downstream EOF and runs only for non-terminal chains.

**D-P4 RESOLVED: COVERAGE BOUNDARY at 29.2; the close-direction seam folds into 29.3.** Threading direction requires a `tcp_proxy` + `chain.go` touch (recording which pump EOF'd first → a `CloseDirection` on `chainRuntime` → a callback accessor → mongo reads it at `OnDestroy`) — a genuine framework ripple, NOT the minimal accessor the parent anticipated. To keep 29.2 framework-zero-touch (the 28.2 precedent) and honor ADR-0219 (one ripple, not two), the two counters stay **exist-at-zero / increment-deferred** at 29.2:

- `0051` asserts `cx_destroy_local_with_active_rq` + `cx_destroy_remote_with_active_rq` PRESENT both sides (creation parity), value NOT compared (extending the 29.1 AMEND-C2 posture). The reference increments one of them on every connection close WITH A STILL-ACTIVE (unanswered) query — i.e. only when the active-query list is non-empty at close (`proxy.cc:355-376`; a connection whose queries were all answered before close increments neither); envoy-go increments neither until 29.3 → values legitimately differ → presence-only.
- 29.3 (ADR-0226), which already opens the pump/terminal/`chain.go` area for the async halt/resume seam, lands the minimal close-direction accessor + the `cx_destroy_*` direction-keyed increment + the `0051`/`0052` value-parity arms.
- **This §Context AMEND SUPERSEDES the parent roster tables for the two `cx_destroy_*` counters:** parent §7.2's "Incremented: 29.2" roster-table cell and parent §3.1's "2 `cx_destroy_*` … lands 29.2" split-table cell are re-scoped to 29.3 (the increment + value parity), NOT a silent contradiction — the parent §11.10 / AMEND-B12 explicitly sanctioned "a minimal close-direction accessor … else a coverage boundary," and ADR-0224 §Consequences already omitted `cx_destroy_*` from its "increment-wire at 29.2" list. The two counters' CREATION stays at 29.1 (eager); only their increment moves to 29.3.
- Recorded as: (i) a one-line in-place AMEND on ADR-0225's §Context at THIS SPEC commit (the ADR-0223-at-28.2-SPEC precedent — no new ADR number); (ii) a BEHAVIOR_CONTRACT coverage-boundary entry in the 29.2 bundle (§8); (iii) a 29.3 forward-pointer in the ADR-0225 §Decision body + the parent ROADMAP 29.3 row.

This is a SUB-PHASE boundary (closed at 29.3), not a permanent departure — the analogue of the 29.1 OP_REPLY recognized-not-decoded boundary (closed at 29.2).

### 3.7 `emit_dynamic_metadata` emission (the ADR-0217 Bucket — third production writer)

The `emit_dynamic_metadata` bool was parsed + stored at 29.1 (`config.go`; consumed here). When true, upstream emits per decode pass a Struct under namespace `envoy.filters.network.mongo_proxy`, keyed by collection name (the resource) → a ListValue of operation strings, appended per decoded message; only `"insert"` (decodeInsert) and `"query"` (decodeQuery) are emitted in v1.37.2 (`update`/`delete` keys defined but TODO-unused); the namespace's fields are CLEARED at the top of every `doDecode` pass (parent §11.9; `proxy.cc:77-90,327-334`).

**Where it fires.** Both emitting operations (`insert`, `query`) are REQUEST-side (decodeInsert / decodeQuery, goroutine A / `OnData`). So the Bucket write happens on the request decode pass — the emission machinery is ASSIGNED to 29.2 (parent §3.2d) but wires into the EXISTING 29.1 `decodeQuery`/`decodeInsert` paths, gated by `cfg.emitDynamicMetadata`. The Bucket is therefore single-goroutine (goroutine A) — NO mutex (§3.5).

**The Bucket surface (ADR-0217).** `f.cb.DynamicMetadata() *dynamicmetadata.Bucket` (`callbacks.go`; `chain.go:452` → the per-connection bucket, created at accept, `Reset()` at OnDestroy). `Bucket.Set(filterName, key, *structpb.Value)` overwrites at `(filterName, key)`; `Bucket.Reset()` clears the WHOLE bucket (all namespaces) — too coarse for a per-pass, per-namespace clear.

**The per-pass clear — the pinned model (D-S29.2-3).** Two faithful options; the SPEC PREFERS option (a):

- **(a) Single-Set model (PREFERRED — zero `internal/dynamicmetadata/` change).** Model the entire mongo namespace metadata as ONE `*structpb.Value` (a `StructValue` whose fields ARE the collection names → each a `ListValue` of op strings), written under one conventional key via a SINGLE `Set("envoy.filters.network.mongo_proxy", <key>, structValue)` per pass. Per-pass clear is FREE: the next pass overwrites the single value. At the top of each `OnData` decode pass, accumulate this pass's collection→ops into a fresh local `map`; after the pass, build the StructValue and `Set` it (or skip the `Set` if empty). This overwrites cleanly and requires no per-namespace clear primitive. The `<key>` wrapper is a unit-test-asserted internal detail (differential-invisible — no cross-side surface, no in-repo consumer).
- **(b) Per-collection-key model + a minimal `Bucket` clear.** Set each collection as its own `(filterName, collection)` entry (the namespace's fields ARE the collections — closest to upstream's `mutable_fields()[resource]`), and add a minimal `Bucket.ClearNamespace(filterName string)` to `internal/dynamicmetadata/` called at the top of each pass. This is more faithful to upstream's namespace-field layout but adds a shared-primitive method (a small, in-scope ADR-0217-consistent addition — NOT a network-framework change).

Both are differential-invisible; the choice is an IMPL detail (D-S29.2-3). The SPEC pins the SEMANTICS (namespace `envoy.filters.network.mongo_proxy`; collection → ops; `"insert"`/`"query"` only; per-pass clear; gated by `emit_dynamic_metadata`) and PREFERS (a) (zero primitive change). **Proof (D-P11 RESOLVED): unit-test-only + a BEHAVIOR_CONTRACT coverage note** — the emission has zero cross-side observability (connection-level metadata has no in-repo consumer; the differential cannot see it). NO retroactive zookeeper-metadata obligation (parent AMEND-B11: mongo's emission consumes only fields its decoder already extracts).

### 3.8 File delta

| File | 29.2 change |
|---|---|
| `codec.go` | `decoder` gains `writeBuf []byte` + `mu sync.Mutex`; `decodeOnWrite` + `nextWriteMessage` + `decodeResponseMessage` + `decodeReply` + `decodeCommandReply` + `takeQuery`; the `opReply`/`opCommandReply` dispatch arm wired; the request-path list-append sites take `mu` + Inc the gauge |
| `filter.go` | `OnWrite` no-op → the `decodeOnWrite` feed; `OnDestroy` drains the residual list under `mu` + Decs the gauge per entry |
| `config.go` | (no parse change — `emitDynamicMetadata` already parsed at 29.1; the §3.7 emission reads it) |
| `stats.go` | (no roster change — the 5 reply counters + the gauge already eager; the response increments call `ms.inc(...)` / `ms.opQueryActive.Dec()`) |
| `doc.go` | the package-doc 29.2/29.3 forward-pointers updated (response side LANDED; fault/drain → 29.3) |
| `internal/dynamicmetadata/` | ONLY under §3.7 option (b) — a minimal `ClearNamespace`; under the PREFERRED option (a), UNTOUCHED |
| `*_test.go` | the response-decode + correlation + gauge + metadata unit tests + the `-race` cross-goroutine test |
| `fuzz_test.go` | the 39th fuzzer EXTENDED to the response opcodes + the mutex (D-P6) |

---

## 4. Framework touchpoints — NONE (the 28.2 zero-touch property, re-pinned)

29.2's production diff to `internal/filter/network/` (excluding the `mongoproxy/` subpackage), `internal/listener/`, `internal/filter/tcpproxy/`, `internal/http/`, and `internal/stats/` is **ZERO files**. The response decoder rides the as-built WriteFilter seam (qualified for the 28.1b read seam intrinsically by mongoproxy's both-direction registration — `reference_network_chain_terminal_handoff_ends_ondata`; the 0049/0051 traffic flows through `writeChainConn → OnWrite`). The gauge primitive + the four-rule name.go arm are consumed unchanged. The ONLY production touch outside `mongoproxy/` that 29.2 might make is a minimal `internal/dynamicmetadata/` per-namespace clear (§3.7 option (b)) — a shared-primitive addition, NOT a network-framework change, and avoidable entirely via §3.7 option (a). The close-direction seam (D-P4) is DEFERRED to 29.3 precisely to preserve this zero-touch property (§3.6). The FULL 52-dir existing fixture suite must stay byte-exact green at the 29.2 six-gate (the back-compat regression gate).

---

## 5. Stat surface (cross-reference parent §7; +0 creation)

### 5.1 No creation delta — 360 → **360**

All 23 fixed mongo stats were created EAGERLY at 29.1 (D-P1). 29.2 wires increments only: the 5 response counters (`op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`, `op_command_reply`) go increment-active; the `op_query_active` gauge goes inc/dec-active. The 2 `cx_destroy_*` counters stay exist-at-zero (D-P4 — §3.6). The 2 fault/drain counters (`delays_injected`, `cx_drain_close`) stay exist-at-zero until 29.3. **Project stat surface stays 360** (the 28.2 "+0 creation" precedent). The dynamic `cmd.*`/`collection.*`/callsite counters remain excluded from the static count (config/traffic-dependent).

### 5.2 Prometheus exposition — NO new `name.go` arm

The 29.1 `mongo.` four-rule tag-extractor arm (`name.go`; AMEND-C1) already handles the fixed reply counters (`mongo.<sp>.op_reply*` → `envoy_mongo_op_reply*{envoy_mongo_prefix="<sp>"}`) and the gauge (`mongo.<sp>.op_query_active` → `# TYPE envoy_mongo_op_query_active gauge`; `prom.go:62-66` MetricGauge handling produces the `gauge` TYPE). 29.2 adds NO new arm. The gauge's `# TYPE … gauge` line + value are scraped by the existing `0049` driver helpers (`scrapeMongoStats` parses the value; `scrapeTypeLine` reads the TYPE line) — reused verbatim by `0051` (D-P10 RESOLVED: gauge support works as-is; §6.2).

### 5.3 Departure flags + coverage boundaries (the 29.2 BEHAVIOR_CONTRACT subset)

- **The dynamic-HISTOGRAM families DEFERRED (ADR-0060).** `cmd.<cmd>.reply_num_docs`/`.reply_size`/`.reply_time_ms`; `collection.<c>.query.reply_*`; callsite `reply_*` — unmirrored; the coverage-boundary RECORD lands in the 29.2 bundle (this is the response side that would populate them — parent §9 / the 29.1 SPEC §2.5 deferral target). The `start time.Time` recorded per active query at 29.1 is the latency basis these would consume; it stays unconsumed at 29.2.
- **The `cx_destroy_*` close-direction coverage boundary (D-P4 — §3.6).** Value parity deferred to 29.3; presence-only at 29.2.
- **The dynamic-metadata emission differential-invisible (AMEND-B11 — §3.7).** Unit-test proof + a coverage note (no fixture surface).
- The 29.1-landed departures (boot-window eager creation; runtime-key gating; the `stats.IsValidName` guard on wire-derived dynamic segments) carry forward unchanged.

---

## 6. The proof surface — fixture `0051` + the `TCPMongoResponder` backend

Per `reference_differential_fixture_dispatch_constraint`: `0051` is CROSS-SIDE (one runner branch). Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Numbering continues from `0050` (the 29.1 tail): 29.2 lands **`0051`** → 52 → **53** dirs.

**Fixture-design constraints (carry-over + the response specifics):** (i) the backend MUST emit CORRELATED OP_REPLY bytes (so the reference's `onWrite` response decoder fires + correlates — unlike 29.1's silent `TCPSink`); (ii) ALL stat assertions are post-first-connection (AMEND-B4); (iii) decoding-error arms use FRESH connections (AMEND-B6 — sniffing-off is connection-lifetime, now direction-shared); (iv) the gauge is asserted only at QUIESCED points (D-P9 — the driver waits for the reply round-trip; §3.4); (v) `cx_destroy_*` are PRESENCE-ONLY (D-P4 — §3.6); (vi) the responder's reply `responseTo` MUST echo the request's `requestID` for correlation.

### 6.1 `TCPMongoResponder` — NEW BackendKind = 30 (the 28.2 `TCPZKResponder` = 29 precedent)

The existing silent `TCPSink` (BackendKind 28) is request-side-only (any bytes it writes would drive the reference's response decoder — the 29.1 AMEND-C4 lesson). `0051` needs a backend that emits CORRELATED OP_REPLY frames. 29.2 adds **`TCPMongoResponder` BackendKind = 30** (`fixture.go`; the next free value) — a mongo-aware canned-response TCP backend. For each complete request frame it reads (16-byte LE MsgHeader framing — it parses ONLY messageLength + requestID + opCode; it is NOT a MongoDB server):

1. **OP_QUERY (2004)** → write a correlated **OP_REPLY (1)** frame: MsgHeader (messageLength + a fresh responder requestID + `responseTo = request.requestID` + opCode 1) + responseFlags(0) + cursorID(0) + startingFrom(0) + numberReturned(0) + zero reply docs (a minimal valid empty reply — `op_reply` + (cursorID==0 → NOT valid_cursor) on both sides). Variants by a designated marker (a query-flag bit or collection name the driver controls): a CursorNotFound reply (responseFlags 0x01), a QueryFailure reply (0x02), a valid-cursor reply (cursorID ≠ 0) — to exercise the three reply-flag counters.
2. **OP_COMMAND (2010)** → write a correlated **OP_COMMANDREPLY (2011)** frame (metadata + commandReply + zero outputDocs) → `op_command_reply` both sides.
3. **The UNANSWERED-query trigger** (a designated requestID or collection) → read the request but WITHHOLD the reply → the query stays in the active-query list → the gauge quiesced at `1` both sides while the connection is open (§3.4 / §6.2 arm).
4. **OP_INSERT / OP_GET_MORE / OP_KILL_CURSORS** → MongoDB protocol sends NO reply for these (fire-and-forget; only OP_QUERY/OP_GET_MORE-cursor and OP_COMMAND elicit replies — and the legacy fire-and-forget insert/update/delete never reply). The responder writes NOTHING for insert/kill_cursors; for OP_GET_MORE it MAY write an OP_REPLY (upstream get_more elicits a reply) — the exact get_more disposition is a PLAN/IMPL upstream-transcription detail (D-S29.2-2); the load-bearing arms are OP_QUERY→OP_REPLY + OP_COMMAND→OP_COMMANDREPLY.

The responder reuses the `0049` little-endian wire/BSON builder helpers (`bsonDoc(...)`, `opReply(responseTo, flags, cursorID, docs...)`, `opCommandReply(...)` — added beside the 29.1 `opQuery`/`opInsert` builders, shared with the driver; D-S29.1-3 generalized). Runner-side plumbing mirrors the `TCPZKResponder` arm: a `BackendKindAware` driver returning `fixture.TCPMongoResponder`; an `acceptMongoResponder` runner backend (the `acceptZKResponder` sibling) + the `0051` blank-import in `runner_test.go`. Exact reply-frame byte minimums verified against upstream `bson_impl`/`codec_impl` at IMPL (D-S29.2-1).

### 6.2 `0051-mongo-responses` (cross-side; the load-bearing fixture)

**Topology.** Chain `[mongo_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 docker + envoy-go subprocess). Listener(s) route to ONE cluster → ONE `TCPMongoResponder` backend (BackendKind 30). Multi-listener if distinct configs are needed (the `0049` `MultiListenerDriver` precedent); the default is a single `l_resp` listener (`stat_prefix: mongo_r`) unless an arm needs a second config.

**StatsAsserter mechanics.** The driver implements `fixture.StatsAsserter`; both sides scraped via `GET /stats/prometheus`; lines parsed into (name + canonicalized sorted-label-set) → value maps (the `0049` label-aware mechanics reused verbatim, incl. `scrapeMongoStats` + `scrapeTypeLine`). The gauge is scraped identically (its value via `scrapeMongoStats`, its `# TYPE … gauge` line via `scrapeTypeLine`) — D-P10: no new harness machinery.

**Arms (the SPEC-anticipated spine; the PLAN/IMPL finalizes):**

1. **plain reply round-trip** (answered; quiesced): OP_QUERY → the responder's empty OP_REPLY (responseTo echo) → `op_reply` +1 both sides; correlated → `op_query_active` settles to **0** both sides (asserted after the reply round-trips + the connection at rest). Also re-proves `op_query` +1 (the 29.1 request surface still green under the response load).
2. **reply-flag variants** (answered): a CursorNotFound reply → `op_reply_cursor_not_found` +1; a QueryFailure reply → `op_reply_query_failure` +1; a valid-cursor reply (cursorID ≠ 0) → `op_reply_valid_cursor` +1 — each both sides, each correlated (gauge back to 0).
3. **OP_COMMAND round-trip** (answered): OP_COMMAND → OP_COMMANDREPLY → `op_command_reply` +1 both sides; OP_COMMAND created NO active-query entry → the gauge is untouched by this arm (stays 0).
4. **the gauge quiesced-point arms (D-P9 — the load-bearing gauge proof):** (i) ANSWERED — N queries all answered → `op_query_active == 0` both sides at rest; (ii) UNANSWERED — a query whose reply the responder withholds → `op_query_active == 1` both sides while the connection is OPEN (scraped after the request round-trips to the backend but before close); then the connection closes → a post-close re-scrape shows `== 0` both sides (the destroy teardown Dec). The gauge `# TYPE … gauge` line asserted present + equal both sides.
5. **uncorrelated reply** (the responder emits an OP_REPLY whose `responseTo` matches NO sent query — a designated trigger): `op_reply` +1 both sides; the gauge UNCHANGED (no correlation hit) — proving uncorrelated replies charge fixed counters only (§3.3).
6. **malformed-reply decoding_error** (FRESH conn): the responder emits a well-framed OP_REPLY whose BSON doc is malformed (bad element type 0x13) → `decoding_error` +1 both sides; sniffing off for the connection (a follow-up valid reply on the SAME conn increments nothing — the AMEND-B6 direction-shared sniffing-off proof, asserted cross-side).
7. **`cx_destroy_*` presence + exists-at-zero** (after the answered arms, all connections closed): `cx_destroy_local_with_active_rq` + `cx_destroy_remote_with_active_rq` PRESENT both sides, value NOT compared (D-P4 — §3.6); `delays_injected`, `cx_drain_close` PRESENT and == 0 both sides; `op_query_active` PRESENT, `# TYPE … gauge`, == 0 both sides (fully quiesced).
8. **dynamic-metadata** — NO fixture arm (differential-invisible — §3.7); proven by unit tests only. `0051` README records this explicitly (the "no fixture surface" note).
9. **deliberate-break liveness proof** (R4; the `0030`/`0049` lesson): recorded in driver comments + README + PROGRESS.md at IMPL — e.g. (a) temporarily asserting `op_reply == 2` (when 1 is received) MUST fail on both runner paths with `-count=1`; (b) temporarily skipping the gauge `Dec` at correlated reply MUST fail the quiesced `op_query_active == 0` arm (subject-side); (c) temporarily disabling the §3.4 gauge `Inc` MUST fail the unanswered `== 1` arm. Both reverted; recorded.

### 6.3 Counts

52 → **53** at 29.2 phase-done (+1; tail `0051-mongo-responses`). The full 52-dir existing suite is the back-compat gate (§4) and re-runs green at the six-gate. (29.3 adds `0052` → 54.) No new conformance harness.

---

## 7. The fuzzer — EXTEND the 39th (D-P6 RESOLVED)

mongo's decoder is direction-agnostic (a single opcode dispatch serves both directions — unlike zookeeper's two distinct framings, which warranted a separate 38th fuzzer). **D-P6 RESOLVED: EXTEND the 39th `FuzzMongoDecode`** (no 40th). The fuzzer's reach grows to the response opcodes: arbitrary bytes are fed through BOTH `decodeOnData` and `decodeOnWrite` (the same decoder), asserting (a) no panic across both directions (incl. OP_REPLY/OP_COMMANDREPLY bodies + the `stats.IsValidName` guard carried from 29.1); (b) the chain buffer is never mutated; (c) sniffing-off idempotence across directions (once any decode errors, further input on EITHER direction decodes/increments nothing); (d) NEW — the mutex: a `-race` fuzz seed or a dedicated `-race -count=5` concurrent test (§3.5) exercises concurrent `decodeOnData`/`decodeOnWrite` so the race detector validates the critical section. Fuzzer count stays **39**.

---

## 8. Behavior-contract delta (the 29.2 bundle; per ADR-0052 atomic landing)

ONE atomic bundle at the 29.2 IMPL final task:

- The `### envoy.filters.network.mongo_proxy` subsection (created at 29.1) gains: the response-side decode semantics (§3.2); the 5 response counters going increment-active; the `op_query_active` gauge inc/dec lifecycle (the project's first mirrored gauge); the requestID↔responseTo correlation + the per-connection-mutex concurrency note; the dynamic-metadata emission (namespace + collection→ops + per-pass clear + the differential-invisible note); the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary CLOSED.
- **NEW coverage boundaries:** the dynamic-HISTOGRAM families (ADR-0060 — recorded HERE per §5.3); the `cx_destroy_*` close-direction boundary (D-P4 — §3.6; value parity → 29.3); the dynamic-metadata differential-invisibility note (AMEND-B11).
- Stat table: **360 → 360** (+0 creation; the 5 response counters + the gauge go increment-active; explicitly noted as a no-creation increment-wiring delta — the 28.2 "+0" precedent).
- Forward-pointers to the 29.3 bundle (fault-delay / access-log / drain / the close-direction seam + cx_destroy value parity / the parent ROLLUP).

---

## 9. SPEC-time empirical pins

The 29.2 SPEC does NOT re-execute the parent §11 D29-1..D29-12 pins (resolved once at the parent SPEC; inherited) NOR the 29.1 §11.2 D-P2 re-probe (the dynamic-stat shapes are pinned; the response counters + the gauge use the SAME four-rule arm, already proven). No new live probe is needed — the response counters' Prometheus shape is the fixed-stat shape (`envoy_mongo_<leaf>{envoy_mongo_prefix=...}`) already pinned by the 29.1 §11.2 probe + source cross-check; the gauge's `gauge` TYPE is pinned by the same probe (`envoy_mongo_op_query_active` is the sole gauge TYPE) + the `0049` arm-8 cross-side TYPE assertion already GREEN.

### 9.1 D-S29.2-0 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip (`git log --oneline -1` at this session = the docs-only `b4dbe40` trailing the 29.1-IMPL squash `5ef5215`). These are the source of the §12 Task-1 first-action gate; the IMPL Task-1 RE-RUNS them against the live IMPL-session tip.

- **Differential fixture-dir count = 52**; numbering tail = **`0050-mongo-boot-reject`**. 29.2 lands `0051` → **53**. Recipe `ls -d test/fixtures/[0-9]* | wc -l` = 52.
- **Fuzzer count = 39** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`). 29.2 EXTENDS `FuzzMongoDecode` (no count change → stays **39**).
- **Stat surface = 360** (BEHAVIOR_CONTRACT stat table; +23 mongo at 29.1). 29.2 lands +0 → **360**.
- **DECISIONS.md tail = ADR-0226** (`DECISIONS.md:14470` ADR-0225 §Context; `:14489` ADR-0226 §Context); **next-free ADR-0227**. 29.2 IMPL fills the ADR-0225 §Decision/§Consequences body IN PLACE (no new ADR number; ADR-0044) + the one-line §Context D-P4 AMEND at THIS SPEC commit.
- **As-built anchors re-verified** (the §3/§6 design anchors): the mongoproxy package — `codec.go:30-36` (`activeQuery`), `:46-53` (the `decoder` struct + the 29.2 mutex forward-pointer), `:64-83` (`decodeOnData`/the decode loop), `:88-103` (`nextMessage`), `:107-132` (`decodeMessage` + the `opReply`/`opCommandReply` recognized-not-decoded arm), `:137-147` (`decoderError`/`fail`), `:152-267` (`decodeQuery` + the list-append + the gauge-inc sites), `:320-402` (the other request-opcode decoders to mirror); `filter.go:58-61` (`OnData`), `:63-68` (the `OnWrite` no-op stub to replace), `:75-77` (`OnDestroy`); `stats.go:40-51` (the eager roster incl. the gauge), `:55-61` (`inc`). The framework — `internal/stats/gauge.go` (`Inc`/`Dec`/`Add`/`Load`, atomic.Int64); `internal/stats/prom.go:56-80` (the `# TYPE … gauge` emission); `internal/filter/network/writeconn.go:34-48` (the fresh-per-Write Buffer — the no-high-water-mark basis); `internal/filter/network/chain.go:405-420` (`onDestroy`), `:452` (`DynamicMetadata()` → the bucket), `:184-212` (the `closeType`/bucket lifecycle); `internal/filter/network/callbacks.go:26-29` (the `DynamicMetadata()` accessor), `:40-57` (`CloseType` — the NO-direction record); `internal/filter/tcpproxy/filter.go:134-139` (the two pumps + `wg.Wait()` — the §3.5 goroutine anchors + the §3.6 close-detection sites); `internal/dynamicmetadata/dynamicmetadata.go` (`Set`/`Get`/`Reset`/`Snapshot`); `internal/filter/network/rbac/rbac.go` (the existing Bucket producer — the §3.7 pattern).
- **Differential-harness anchors**: `test/differential/fixture/fixture.go` BackendKind roster `… TCPSink = 28, TCPZKResponder = 29` (next-free **30**); the `BackendKindAware` + `StatsAsserter` + `MultiListenerDriver` sites; `test/fixtures/0049-mongo-requests/driver/driver.go` (the LE wire/BSON builders + `scrapeMongoStats`/`scrapeTypeLine` + the label-aware StatsAsserter — the `0051` template); `test/fixtures/0048-zookeeper-responses/driver/driver.go` + the `acceptZKResponder` runner arm (the `TCPMongoResponder` plumbing template).

### 9.2 Inherited empirical pins (constrain §3/§6; no re-probe)

- Parent §11.4 OP_REPLY layout (responseFlags int32 + cursorID int64 + startingFrom int32 + numberReturned int32 + N BSON docs) + OP_COMMANDREPLY (metadata + commandReply + outputDocs) + flag bits 0x01/0x02 + valid_cursor = cursorID ≠ 0 + correlation `requestID == responseTo` first-match-erase (§3.2/§3.3).
- Parent §11.5 private-buffer copy/sniff model + sniffing-off-on-error (§3.1).
- Parent §11.9 dynamic-metadata shape (§3.7); §11.10 close-direction gap (§3.6).
- 29.1 §11.2 D-P2 probe: the fixed-stat + gauge Prometheus shapes (§5.2); `cx_destroy_local_with_active_rq: 4` on the reference (the AMEND-C2 → D-P4 presence-only basis, §3.6).
- ADR-0223 §Decision (the per-connection-mutex + no-write-side-high-water-mark + copy-out-under-lock discipline, §3.1/§3.5).

---

## 10. SPEC-time D-questions

### 10.1 Parent D-questions RESOLVED at this SPEC

- **D-P4 (close-direction seam) — RESOLVED: COVERAGE BOUNDARY; the seam folds into 29.3** (§3.6). The as-built investigation (§9.1) refutes the parent's "minimal accessor the chain runtime already observes" — direction genuinely requires a `tcp_proxy` + `chain.go` touch. Keeping 29.2 framework-zero-touch (the 28.2 precedent) + ADR-0219 (one ripple) → presence-only at 29.2; value parity + the accessor at 29.3. Recorded as an ADR-0225 §Context AMEND + a BEHAVIOR_CONTRACT boundary.
- **D-P6 (response-decode fuzzer) — RESOLVED: EXTEND the 39th** (§7). The direction-agnostic decoder → one fuzzer covers both directions; no 40th. Count stays 39.
- **D-P9 (gauge quiesced-point assertion) — RESOLVED: driver-waits-for-reply-round-trip** (§3.4 / §6.2 arm 4). Answered → 0 both sides; unanswered (responder withholds the reply) → 1 both sides while open, 0 after close. No timing nondeterminism (the gauge is scraped only at rest).
- **D-P10 (StatsAsserter gauge support) — RESOLVED: works as-is** (§5.2 / §6.2). The `0049` `scrapeMongoStats` (value) + `scrapeTypeLine` (`# TYPE … gauge`) helpers handle the gauge identically to a counter; no new harness machinery.
- **D-P11 (dynamic-metadata proof surface) — RESOLVED: unit-test-only + a coverage note** (§3.7). Zero cross-side observability; no fixture arm.

### 10.2 29.2-additive D-questions for PLAN / IMPL resolution

- **D-S29.2-1 (response reply-frame byte minimums).** The exact OP_REPLY / OP_COMMANDREPLY minimum-length + empty-doc encodings the `TCPMongoResponder` emits + the decoder validates, verified against upstream `codec_impl.cc`/`bson_impl.cc` v1.37.2 (the 28.2 D-S28.2-1 watch-event-16-vs-28 lesson — `reference_wire_format_both_sides_see_same_bytes`). **Resolution at:** IMPL (the responder + decoder tasks).
- **D-S29.2-2 (OP_GET_MORE reply disposition).** Whether the responder replies to OP_GET_MORE (upstream get_more elicits an OP_REPLY) and whether 29.2 correlates it (GetMore created NO active-query entry at 29.1 → an OP_REPLY whose responseTo matches a get_more requestID is uncorrelated → fixed counters only). **Resolution at:** PLAN/IMPL. Anticipated: the responder MAY reply; the reply is uncorrelated (the §3.3 miss path) — no gauge change; the load-bearing arms use OP_QUERY/OP_COMMAND.
- **D-S29.2-3 (dynamic-metadata Bucket model).** Single-Set StructValue (PREFERRED — zero primitive change) vs per-collection-key + a minimal `Bucket.ClearNamespace` (§3.7). **Resolution at:** IMPL. Anticipated: the single-Set model.
- **D-S29.2-4 (the `sniffing` flag synchronization).** Whether the connection-lifetime `sniffing` bool is read/written under `mu`, made an `atomic.Bool`, or left as-is given it only transitions true→false once (§3.1/§3.5). **Resolution at:** IMPL (the `-race` test decides). Anticipated: under `mu` (the response path already locks per frame) OR `atomic.Bool` — both race-clean; the SPEC pins "race-clean," not the mechanism.
- **D-S29.2-5 (the residual-list drain at OnDestroy).** Whether `OnDestroy` Decs the gauge per residual entry under the lock or snapshots-the-count-then-Decs (§3.4/§3.5). **Resolution at:** IMPL. Anticipated: take `mu`, record `len(queries)`, clear, release, Dec by the count (gauge math outside the lock).

---

## 11. RATIFIED-PENDING items (cross-reference parent §13 + the 29.1 SPEC §13, scoped to 29.2)

- **R1 (back-compat).** The 52 existing fixture dirs stay byte-exact green at the 29.2 six-gate (the §4 zero-touch property; the `0049`/`0050` request fixtures re-green under the now-live response path — proving the response decode does not perturb the request-side surface).
- **R3 (passthrough invariant, extended to the write side).** mongoproxy NEVER mutates/drains the chain buffer (read OR write), never closes, never returns StopIteration (29.2 still has no fault delay). `OnWrite` always returns `Continue`. Decode errors (either direction) → sniffing off + passthrough continues. Ratified by `0051` arm-6 passthrough + unit tests asserting the write chain buffer is byte-identical before/after `OnWrite`.
- **R4 (StatsAsserter liveness).** Every `0051` stat assertion proven live via a recorded deliberate-break with `-count=1` (§6.2 arm 9 — incl. the gauge-specific breaks).
- **R5 (correlation hand-off — RATIFIED end-to-end).** The 29.1 active-query list is now READ + ERASED by the response decoder; `0051` arms 1–4 prove the round-trip; the unit tests prove first-match-erase + the only-OP_QUERY-creates-entries invariant.
- **R6 (counts).** IMPL Task 1 re-pins fixtures 52→53, fuzzers 39 (unchanged), stats 360 (unchanged), DECISIONS tail ADR-0226 (next-free ADR-0227) against the live IMPL-session tip (§9.1 recipes).
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` mongo response lines + the gauge (value + `# TYPE … gauge`) match the reference's tag-extracted shape. Ratified intrinsically by the `0051` label-aware both-sides-scrape mechanics (§5.2/§6.2).
- **R9 (concurrency — NEW, the 28.2 R9 analogue).** The per-connection mutex is both necessary + sufficient: `TestDecoderConcurrentRequestResponseRace` (`-race -count=5`) is GREEN with `mu` and reports a race WITHOUT it (§3.5). Plus the live `0051` concurrent pumps (both goroutines active under real traffic, all arms green cross-side).
- **R-GAUGE (NEW).** The project's first differentially-mirrored gauge holds value parity at quiesced points (answered → 0; unanswered → 1; post-close → 0) on both sides (§3.4/§6.2 arm 4).

---

## 12. Per-task structure (~11 tasks; the SPEC-anticipated spine)

The 29.2 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine, ordered for green-compiling dependency (the 28.2 ordering logic: anchors → struct+lock → pure decode fns → correlation/gauge → glue+race → metadata → fuzzer → backend → fixture → docs → six-gate):

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **52** (tail `0050`) + fuzzers **39** + stat surface **360** + DECISIONS tail **ADR-0226** (next-free **ADR-0227**) + the §9.1 as-built anchors, against the live IMPL-session tip | §9.1 / R6 |
| 2 | `decoder` struct: add `writeBuf []byte` + `mu sync.Mutex`; move the 29.1 request-path list-append + gauge-Inc sites under `mu`; mechanical existing-test updates (all 29.1 tests stay green) | §3.5 / §3.4 |
| 3 | Response framing + dispatch: `decodeOnWrite` + `nextWriteMessage` + `decodeResponseMessage` + the `opReply`/`opCommandReply` dispatch arm + the write-side `decoding_error`/sniffing-off path + unit tests (partial/oversized/short frames; the direction-shared sniffing-off) | §3.1 / §3.2 |
| 4 | OP_REPLY + OP_COMMANDREPLY body decode + the 5 response counters (`op_reply` + the three flag/cursor counters + `op_command_reply`) + unit tests (each flag bit; valid_cursor; numberReturned docs; malformed → decoding_error) | §3.2 |
| 5 | Correlation consumption: `takeQuery` (first-match-erase under `mu`) + the gauge `Dec` on a hit + uncorrelated-miss handling + unit tests (first-match; only-OP_QUERY-entries; miss charges fixed only) | §3.3 / §3.4 |
| 6 | `OnDestroy` gauge teardown (drain residual list under `mu`, Dec per entry) + the `op_query_active` inc/dec lifecycle unit tests (list-size↔gauge invariant) | §3.4 |
| 7 | `OnWrite` glue (replace the no-op) + the concurrent request/response race test (`-race -count=5`) — R9 | §3.1 / §3.5 / R9 |
| 8 | `emit_dynamic_metadata` emission (the single-Set Bucket model; per-pass clear; collection→ops; gated) + unit tests (the namespace/keys/clear; the gated-off no-emit) — D-P11 | §3.7 |
| 9 | The 39th fuzzer EXTENDED to the response opcodes + the mutex (D-P6) | §7 |
| 10 | `TCPMongoResponder` BackendKind 30 + the runner backend arm (`acceptMongoResponder`) + the OP_REPLY/OP_COMMANDREPLY builders | §6.1 |
| 11 | `0051-mongo-responses` driver + cross-side GREEN all arms (incl. the gauge quiesced-point + cx_destroy presence + R4 break) + README; then the completion bundle: ADR-0225 §Decision/§Consequences body + the §Context D-P4 AMEND + the BEHAVIOR_CONTRACT 29.2 bundle (§8) + STATE.md + ROADMAP sub-row 29.2 `in-progress → done` (parent row 29 STAYS `in-progress`) + next-prompt.txt + the six-gate (full 53-dir suite) | §6.2 / §8 / §13 |

### 12.1 ADR-0045 split-gate — SPEC-level check

Production-LoC estimate against the §3/§6 surface (production code; fixture drivers + unit tests EXCLUDED — the 26.x/28.2 accounting basis):

| Deliverable | Production LoC |
|---|---|
| `codec.go` response path (`decodeOnWrite`/`nextWriteMessage`/`decodeResponseMessage`/`decodeReply`/`decodeCommandReply`/`takeQuery`) | ~140–200 |
| `filter.go` (the OnWrite feed + the OnDestroy gauge teardown) + the `decoder` struct fields | ~30–60 |
| the gauge inc/dec wiring + the mutex acquisition on the 29.1 paths | ~20–40 |
| `emit_dynamic_metadata` emission (the single-Set model; per-pass accumulate + clear) | ~50–90 |
| the fuzzer extend (response-direction reach + mutex) | ~20–40 |
| `TCPMongoResponder` BackendKind + runner arm (test-side; counted for completeness) | ~70–110 |
| **Total (production basis)** | **~330–540** |

**Verdict: fits as ONE sub-phase** (well under the ~1500 gate; ~11 tasks under the ~25-task gate) — comparable to 28.2's ~360–490. The `0051` driver (~700–900 LoC; the `0048`/`0049` precedents 865/875) is excluded per the accounting precedent. **The 29.2 PLAN remains the FINAL gate-check** (parent §3.0); no pre-authorized split axis is anticipated (the 28.2 single-sub-phase precedent).

---

## 13. Test surface + 29.2 IMPL acceptance checklist

### 13.1 Test surface (per parent §14, scoped to 29.2)

- **Layer A — mongoproxy unit tests**: response decode (OP_REPLY body — each flag bit 0x01/0x02; valid_cursor cursorID≠0; numberReturned-doc walk; OP_COMMANDREPLY body; malformed reply → decoding_error; partial/oversized write frames; the direction-shared sniffing-off; the write-side `writeBuf` reassembly across multiple `OnWrite` calls); correlation (first-match-erase; only-OP_QUERY-entries; uncorrelated-miss → fixed counters only; the `takeQuery` copy-out); the gauge (Inc at append; Dec at correlated reply; Dec-per-residual at destroy; the list-size↔gauge invariant); the dynamic-metadata emission (namespace + collection→ops + `"insert"`/`"query"` only + per-pass clear + gated-off no-emit); the chain buffer never drained/mutated on the write side (R3).
- **Layer E — race**: `TestDecoderConcurrentRequestResponseRace` (`-race -count=5`; mutex necessary + sufficient — R9) + `go test -race -short` across `internal/filter/network/...` (the shared roster stats + the gauge under concurrent connections).
- **Layer C — fuzz**: the EXTENDED `FuzzMongoDecode` (both directions: no panic; chain buffer unmutated; sniffing-off idempotence across directions; the mutex under concurrent feed).
- **Layer D — differential**: `0051` (cross-side label-aware StatsAsserter; the gauge quiesced-point arms; cx_destroy presence; 9 arms) + the FULL 52-dir back-compat suite (R1) → 53/53 green.
- Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

### 13.2 Six-gate checklist (per the 28.2 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (53 dirs incl. the 52-dir back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — 29.2 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly — `reference_differential_break_protocol_count1` for the R4 breaks).

### 13.3 29.2 IMPL acceptance checklist

1. The response decoder lands per §3 (OP_REPLY/OP_COMMANDREPLY body decode + the 5 response counters; the `OnWrite` feed replacing the no-op; the write-side `writeBuf`); the framework is untouched (§4).
2. Correlation consumes the 29.1 active-query list (first-match-erase) under the per-connection mutex (§3.3/§3.5 — R5/R9); the `op_query_active` gauge inc/dec lifecycle is live (§3.4 — R-GAUGE).
3. `cx_destroy_*` stay exist-at-zero / presence-only (D-P4 — §3.6); the dynamic-metadata emission lands unit-test-proven (§3.7 — D-P11).
4. Fixture `0051` green (the `TCPMongoResponder` backend; the gauge quiesced-point arms; cx_destroy presence; the R4 break); the fuzzer EXTENDED; counts: fixtures 52→53, fuzzers 39 (unchanged), stats 360 (unchanged) (R6).
5. ADR-0225 §Decision/§Consequences body lands in place (DECISIONS.md tail STAYS ADR-0226; no new number); the §Context D-P4 AMEND lands at the SPEC commit; the BEHAVIOR_CONTRACT 29.2 bundle lands (§8).
6. Six gates green (§13.2); STATE.md advanced; ROADMAP sub-row 29.2 `in-progress → done`; **parent row 29 STAYS `in-progress`** (the ROLLUP is 29.3's); next-prompt.txt rewritten for the 29.3-SPEC cold-start.

---

## 14. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 29.2 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.x precedent); parent row 29 STAYS `in-progress`; 29.3 STAYS `planned`. ALSO at this commit: the one-line in-place AMEND on ADR-0225's §Context (the D-P4 coverage-boundary re-scope — §3.6; the ADR-0223-at-28.2-SPEC precedent; no new ADR number). STATE.md advances to lifecycle-state 2-for-29.2-PLAN with `next-skill = superpowers:writing-plans` scoped to the **29.2 PLAN** (`docs/envoy-go/phases/29.2-network-filter-mongo-responses-and-correlation/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 29.2-PLAN cold-start. Per `feedback_execution_style` the 29.2 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies.

---

## Appendix A — Cross-references

| 29.2 SPEC § | Master / precedent § | Relationship |
|---|---|---|
| §1 Purpose | parent §3.2 (29.2) + ADR-0225 §Context | executes |
| §1.1 AMENDs + 29.1 outputs | parent §1.1 (B1/B3/B11/B12) + 29.1 §3.6/§3.7 | inherits + consumes |
| §1.2 Additive pins | — | NEW (D-P4 boundary; the mutex; the gauge mirror; BackendKind 30; D-P6/9/10/11) |
| §2 Non-purposes | parent §2 + §3.2 (29.3 scope) | refines (29.2-scoped) |
| §3.1 OnWrite feed | parent §11.5 + 28.2 §3.2 + ADR-0223 §Decision item 1 | EXECUTES (no-high-water-mark mirror) |
| §3.2 Response decode | parent §11.4 (OP_REPLY/OP_COMMANDREPLY layout) | mirrors verbatim |
| §3.3 Correlation | parent §11.4 item 7 + ADR-0225 §Context | executes (first-match-erase) |
| §3.4 The gauge | parent §7.2 + D-P9 | NEW (first mirrored gauge) |
| §3.5 The mutex | ADR-0223 §Decision item 4 + 28.2 §3.6 | mirrors (narrowed to one list) |
| §3.6 cx_destroy / D-P4 | parent §11.10 / AMEND-B12 + ADR-0219 | RESOLVES (coverage boundary → 29.3) |
| §3.7 Dynamic metadata | parent §11.9 / AMEND-B11 + ADR-0217 | executes (Bucket; per-pass clear) |
| §4 Framework touchpoints | 28.2 §0 zero-touch + parent §4.1 | re-pins ZERO-touch at 29.2 |
| §5 Stat surface | parent §7 | refines (+0 creation; increment-wiring) |
| §6 Fixtures | parent §8.3 + 29.1 §8.1 (label-aware scrape) + 28.2 §5.1 (responder) | refines (0051 + BackendKind 30) |
| §7 Fuzzer | parent §11.12 (D-P6) | RESOLVES (extend the 39th) |
| §8 Behavior contract | parent §9 (29.2 bundle) | refines |
| §9 Empirical pins | parent §11 + 29.1 §11.2 | inherits; re-pins D-S29.2-0 (no re-probe) |
| §10 D-questions | parent §12 (D-P4/6/9/10/11) | resolves; adds D-S29.2-1..5 |
| §11 RATIFIED-PENDING | parent §13 + 28.2 §10 (R9) | scoped to 29.2 (+ R-GAUGE) |
| §12 Tasks + split-gate | parent §15 (29.2 row) + 28.2 §11 | NEW (task spine); gate re-check |

## Appendix B — Phase 29.2 ADR landing summary

- **ADR-0225** (the `mongo_proxy` response side + correlation + the gauge) — §Context drafted at the parent SPEC (`DECISIONS.md:14470`); §Decision + §Consequences bodies land at 29.2 IMPL Task 11 per ADR-0044. This SPEC's §3 + §5 + §6 are the body's blueprint: the OnWrite response feed (§3.1), the OP_REPLY/OP_COMMANDREPLY decode + the 5 counters (§3.2), the correlation (§3.3), the gauge (§3.4), the per-connection mutex (§3.5), the D-P4 close-direction coverage boundary (§3.6), the dynamic-metadata Bucket emission (§3.7), the `0051` fixture + `TCPMongoResponder` (§6). The one-line §Context AMEND (the D-P4 re-scope) lands at THIS SPEC commit.
- DECISIONS.md tail STAYS **ADR-0226** at 29.2 phase-done (no new ADR number consumed); next-free **ADR-0227**. The ADR-0226 body (the async halt/resume seam + fault delay + access log + drain + the close-direction seam) lands at 29.3.
