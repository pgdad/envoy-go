# Phase 28.2 SPEC — the `zookeeper_proxy` response decoder + latency-threshold counters + the phase-28 rollup

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 28.2** (`network-filter-zookeeper-responses-and-latency`), the second/FINAL sub-phase of the phase-28 BRAINSTORM-time pre-split. The **parent SPEC** (`../28-network-filter-zookeeper-proxy/SPEC.md`) is this sub-phase's MASTER: its §3.2 (the 28.2 scope detail), §5 (proto roster), §6.3 (PARSE-REJECT arms), §7 (stat surface), §8.3 (the `0048` envelope), §11.4/§11.5/§11.7 (the response-framing / decoder-error / latency empirical pins), and §12 (D-P4/P6/P9) remain authoritative. This SPEC ADDS the response-decoder production design (§3), the per-connection synchronization design discharging the ADR-0221 §Consequences forward-pointer (§3.6 here), the latency-counter design (§4), the `0048` + `TCPZKResponder` proof surface (§5), and the phase-28 completion bundle incl. the parent-row ROLLUP (§7). The next session, per SKILL_ROUTING state 2, authors the **28.2 PLAN**.

**Goal:** Complete `zookeeper_proxy`'s round-trip observability — the response decoder in `OnWrite` consuming the 28.1-laid correlation structures (R5), the per-opcode `*_resp`/`watch_event`/`response_bytes` counter increments, latency measurement + the deterministic `enable_latency_threshold_metrics` fast/slow counter surface — prove it cross-side (`0048-zookeeper-responses`), and close phase 28 (ADR-0223 body; BEHAVIOR_CONTRACT 28.2 bundle; the parent-row-28 ROLLUP; the six-gate).

**Architecture:** The as-built `internal/filter/network/zookeeperproxy/` per-connection `requestDecoder` is renamed `decoder` (it now decodes BOTH directions, mirroring upstream's single `DecoderImpl`) and gains a write-side reassembly buffer (`writeBuf`), the response dispatch (xid sniffing: connect-response special / watch event / control FIFO / data-map correlation), the latency fast/slow accounting, and a **per-connection `sync.Mutex`** guarding the two correlation maps (the goroutine-A request decode vs goroutine-B response decode race — the 28.1b SPEC §3.6 / ADR-0221 §Consequences forward-pointer, discharged here). `OnWrite` replaces its 28.1 no-op body with the decoder feed and ALWAYS returns `Continue` (R3 passthrough unchanged). The framework (`chain.go`/`readconn.go`/`writeconn.go`/`buffer.go`), `manager.go`, `tcp_proxy`, and HCM are untouched.

**Tech Stack:** Go 1.26.2; the as-built `internal/filter/network/` + `internal/filter/network/zookeeperproxy/` packages (28.1a/28.1b); the differential harness (`fixture.StatsAsserter` + a NEW `TCPZKResponder` BackendKind); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). ZERO new third-party dependencies.

**Authored:** 2026-06-02. **Empirical-pin probe dates (inherited):** parent SPEC §11 (2026-06-01) + the 28.1a/28.1b IMPL pins (2026-06-02). **Baseline-anchor pin date:** 2026-06-02 (this SPEC session, master tip `8334d1d` — §8.1).

---

## 1. Purpose / Mission

Phase 28.2 delivers the six things the parent SPEC §3.2 + ROADMAP row 28.2 assign it:

1. **The response decoder (§3).** The 28.1 `OnWrite` is a pure no-op: the WriteFilter seam's only consumer is a stub, and the two correlation structures laid at 28.1 (R5) have never been read. 28.2 lands the response decoder — response framing, xid correlation, per-opcode `*_resp` counters, watch events — making the seam's existence justified end-to-end (ADR-0223 §Context: "without 28.2, the seam's only consumer would be a write-direction no-op").

2. **The synchronization obligation (§3.6 — LOAD-BEARING).** The 28.1b read seam placed the request decoder on goroutine A (the downstream→upstream pump's replay path) and the response decoder on goroutine B (the upstream→downstream pump's `OnWrite`). Both touch the per-connection correlation maps. The 28.1b race surface was empty (OnWrite was a no-op); 28.2's is NOT. **A per-connection `sync.Mutex` guards the correlation maps**, and ADR-0223's body records it — discharging the forward-pointer pinned in ADR-0221 §Consequences + the 28.1b SPEC §3.6 + the BEHAVIOR_CONTRACT conn-wrap-seam block.

3. **Latency-threshold counters (§4).** `pendingRequest.start` (recorded at 28.1) → response-decode latency → the `enable_latency_threshold_metrics`-gated `<opname>_resp_fast`/`<opname>_resp_slow` counters (`latency <= threshold` → fast, AMEND-A10; wire-opcode-keyed overrides). The HISTOGRAM family stays deferred (ADR-0060 coverage boundary).

4. **The cross-side proof (§5).** Fixture `0048-zookeeper-responses` (cross-side `StatsAsserter`, the `0046` template) over a NEW ZK-aware `TCPZKResponder` backend whose fixed pre-response delay makes BOTH deterministic-threshold arms provable. R4 deliberate-break recorded. Differential dirs 49 → **50**.

5. **The 38th fuzzer (§6).** `FuzzZookeeperResponseDecode` — the response path's distinct framing gets its own fuzzer (parent §11.10 / D-P6).

6. **The phase-28 completion bundle (§7).** ADR-0223 §Decision/§Consequences body in place (no new ADR number); the BEHAVIOR_CONTRACT 28.2 bundle; the six-gate; and the **parent-row-28 ROLLUP** — sub-row 28.2 AND parent row 28 flip `in-progress → done` ATOMICALLY (the 18/19/22/24/25/26 final-sub-phase precedent), closing the THIRD §9 Network-filters-family row.

### 1.1 What 28.1a/28.1b landed — NOT re-scoped here

Per STATE.md + the ADR-0221/0222 bodies, the following are DONE (squashes `8703aeb` + `fdf40ea`) and this SPEC does not re-design them: the terminal-handoff conn-wrap seam, BOTH halves (`writeChainConn` + `readChainConn` + `replayRead` + `Buffer.TotalAppended` + the shared R1 wrap predicate); the complete request package (config parse incl. ALL latency-field validation + PARSE-REJECT arms, the 201-counter eager roster, the shallow request decoder + min-length table, the correlation-structure WRITES, the dynamic auth counters); the 7th built-in + blank-import + `.zookeeper.` name.go arm; the 37th fuzzer; fixtures `0046`/`0047`.

**The 28.1-landed surfaces this SPEC modifies:** (i) `decoder.go` — the `requestDecoder` struct (renamed, extended with the write side + the mutex); (ii) `zookeeperproxy.go` — the no-op `OnWrite` body (replaced) + `OnDestroy` (unchanged semantics); (iii) `stats.go`/`config.go` — UNTOUCHED (all needed counters + config fields already exist). The framework packages are untouched (§2.4).

### 1.2 28.2-SPEC-additive contributions

- **The response-decoder production design (§3)** — dispatch, correlation consumption semantics, the connect-readonly→connect response-opname mapping (§3.4 item 4 — an IMPL-panic trap this SPEC defuses), the decode-failure symmetry.
- **The synchronization design (§3.6)** — the mutex's exact scope (correlation maps only; reassembly buffers stay lock-free per-goroutine; entries copied out under the lock).
- **The latency-counter design (§4)** — the upstream `errorBudgetDecision` mirror.
- **The `TCPZKResponder` BackendKind (§5.1)** — the fixture.go:500-anticipated "driver-controlled responder", with the fixed-delay deterministic-threshold construction (resolves parent D-P9).
- **The parent D-question resolutions (§9.1)** — D-P4 (latency rejects unit-test-only), D-P6 (separate 38th fuzzer), D-P9 (fixed-delay responder), all user-confirmed at this SPEC's design dialogue.
- **The rollup execution (§7.3)** — the atomic parent + sub-row flip, family candidates 5 → 4.

---

## 2. Non-purposes

- **2.1 No histograms.** The `*_response_latency` HISTOGRAM family (`connect_response_latency`/`<opname>_latency`/`unknown_opcode_latency`) stays DEFERRED per ADR-0060 — the project-wide histogram deferral. Recorded as a BEHAVIOR_CONTRACT coverage boundary at this sub-phase (§7.2). The fast/slow threshold COUNTERS are the deterministic stand-in (BRAINSTORM Q1).
- **2.2 No dynamic metadata, no deep payload parsing.** AMEND-A9 deferral carried unchanged. The response decoder is SHALLOW symmetric to the request decoder: framing + xid + zxid/error presence + length validation; response payloads (stat structs, data blobs, ACL lists) are never parsed. The shallow-decode leniency departure extends to the response side (a response with a valid header but malformed payload counts as `<op>_resp` on envoy-go vs `decoder_error` upstream; the fixture corpus contains no such frames).
- **2.3 No new PARSE-REJECT arms.** All five latency PARSE-REJECT arms landed (byte-stable + unit-proven) at 28.1a (`config.go:148-155`). 28.2 adds NO new reject wording. Their differential disposition is D-P4, resolved here: **unit-test-only** (§9.1).
- **2.4 No framework changes.** `chain.go`, `readconn.go`, `writeconn.go`, `buffer.go`, `types.go`, `manager.go`, `tcp_proxy`, HCM: ZERO modifications. The seam is complete; 28.2 is purely a zookeeperproxy-package + test-surface change. (The §3.6 race test lands in the zookeeperproxy package, not the framework package.)
- **2.5 No real-ZooKeeper-server fixtures, no per-route surface, no new conformance harness** — all per the parent §2 carried boundaries. h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate.
- **2.6 No `access_log` change** — parse-accept-ignore carried.
- **2.7 No new ADR number.** ADR-0223's §Decision/§Consequences body lands IN PLACE at the 28.2 IMPL (ADR-0044). DECISIONS.md tail STAYS ADR-0223; next-free STAYS ADR-0224. A one-line in-place §AMEND on ADR-0223 §Context lands AT THIS SPEC COMMIT (§7.1).

---

## 3. The response decoder (extends the as-built `decoder.go`; lands under ADR-0223)

### 3.1 The unified decoder (D-28.2-1 — user-confirmed)

The as-built `requestDecoder` (`decoder.go:30-56`) is **renamed `decoder`** — it now decodes both directions, mirroring upstream's single `DecoderImpl` (one per-connection object owning both `zk_filter_read_buffer_` and `zk_filter_write_buffer_` plus the correlation maps). The rename is package-internal and mechanical (`newRequestDecoder` → `newDecoder`; the `filter.decoder` field name is already direction-neutral).

Additive fields:

```go
type decoder struct {
	cfg   *compiledConfig
	stats *rosterStats

	// ... chainConsumed, readBuf — UNCHANGED (the request/read side) ...

	// writeBuf is the decoder-internal write-side reassembly buffer (the
	// upstream zk_filter_write_buffer_ mirror). Fed by OnWrite; complete
	// response frames are decoded + consumed; a trailing partial frame
	// survives until the next OnWrite; a decode failure ABANDONS it (no
	// resync — AMEND-A8 symmetry). Accessed ONLY by goroutine B (§3.6).
	writeBuf []byte

	// mu guards the two correlation maps below — the ONLY state shared
	// between goroutine A (request decode: replayRead → OnData) and
	// goroutine B (response decode: writeChainConn.Write → OnWrite).
	// The reassembly buffers (readBuf / writeBuf) and chainConsumed are
	// single-goroutine-owned and stay OUTSIDE the lock (§3.6).
	mu                   sync.Mutex
	requestsByXid        map[int32]pendingRequest
	controlRequestsByXid map[int32][]pendingRequest
}
```

### 3.2 The `OnWrite` feed (replaces the 28.1 no-op; `zookeeperproxy.go`)

```go
// OnWrite feeds the decoder's write-side reassembly buffer with the
// upstream→downstream bytes and ALWAYS returns Continue (AMEND-A8
// unconditional passthrough; R3 — the filter never mutates the chain Buffer,
// never halts the write, never closes). Each OnWrite call sees a FRESH
// per-Write *Buffer (writeChainConn.Write allocates one per call), so the
// bytes are appended directly — no TotalAppended high-water mark is needed on
// the write side (every byte arrives exactly once by construction).
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnWrite(buf.Bytes())
	return network.Continue
}
```

Pinned semantics:

1. **No write-side `TotalAppended` machinery.** `writeChainConn.Write` (`writeconn.go:34-48`) allocates a fresh `*Buffer` per `Write` call, so `OnWrite` sees each upstream→downstream byte exactly once. The decoder appends `buf.Bytes()` to its own `writeBuf` and reassembles frames there. (Contrast the read side, where the SHARED per-connection `rt.buf` + runtime drains forced the 28.1b §3.3 re-base — that asymmetry is structural, not an oversight, and is recorded in ADR-0223's body.)
2. **`decodeOnWrite(p []byte)`**: append `p` to `writeBuf` → loop `nextWriteFrame()` (the §3.3 framing) → `decodeResponseFrame(frame)` per complete frame → on decode failure the `decoder_error` path runs and `writeBuf` is abandoned (no resync).
3. **`nextWriteFrame` shares the `nextFrame` shape**: 4-byte BE length prefix (excludes itself), `max_packet_bytes` check → oversized → `decoder_error` + abandon. The IMPL may extract a shared frame-scanning helper parameterized by buffer (D-S28.2-4) or keep two near-identical methods; either is acceptable.
4. **`endStream` is ignored** (the as-built `writeChainConn` always passes `false`; pinned for contract completeness).
5. **R3 unchanged**: the chain `Buffer` handed to `OnWrite` is read, never drained, never mutated; the return is ALWAYS `Continue`.

### 3.3 Response dispatch (parent §11.4; xid sniffing)

`decodeResponseFrame(frame []byte)` dispatches on the frame's leading int32 (universal min length: 4 bytes; shorter → `decoder_error`):

| Leading int32 | Path | Pinned semantics |
|---|---|---|
| `0` (connectXid) | **connect response** | Special framing: `proto_version(4) + timeout(4) + session_id(8) + password(4-byte len + bytes)` — **NO zxid, NO error** (parent §11.4). Min length 20 + password. Correlates by FIFO-popping `controlRequestsByXid[connectXid]` (§3.4); empty queue → `decoder_error`. Counters: `connect_resp` (+ latency §4). The leading-int32-is-zero sniff is unambiguous: data xids are > 0 and a connect response's first field (proto_version) is 0 on the wire — exactly upstream's dispatch. |
| `-1` (watchXid) | **watch event** | Server-initiated push; **never correlated** (parent §11.4 — "returns no opcode"). Shallow validation: `xid(4) + event_type(4) + client_state(4) + path-len(4)` minimum = 16 (exact min-length verified against upstream `decoder.cc` parseWatchEvent at IMPL — D-S28.2-1). Counters: `watch_event` + `response_bytes`; NO per-opcode `_resp`, NO latency. |
| `-2`/`-4`/`-8` (control xids) | **control response** | Standard response framing: `xid(4) + zxid(8) + error(4)` minimum = 16. Correlates by FIFO-popping `controlRequestsByXid[xid]` (§3.4); empty queue → `decoder_error`. Counters: `<opname>_resp` from the popped entry (+ latency §4). (ping → `ping_resp`; auth → `auth_resp`; setwatches → `setwatches_resp`.) |
| `> 0` (data xid) | **data response** | Standard framing: `xid(4) + zxid(8) + error(4)` minimum = 16. Correlates against `requestsByXid` — lookup + **ERASE** (§3.4); missing xid → `decoder_error` (upstream `InvalidArgumentError` parity). Counters: `<opname>_resp` from the erased entry (+ latency §4). |
| any other negative | **unknown** | `decoder_error` (no per-opcode counter), abandon buffer — mirrors upstream's unknown-xid → `onDecodeError` path. |

**Byte accounting (every successfully decoded response frame, ALL rows above):** `response_bytes += wireFootprint(frame)` (= 4-byte prefix + frame; ungated — the request-side `request_bytes` symmetry) + the flag-gated `<opname>_resp_bytes += wireFootprint(frame)` when `enable_per_opcode_response_bytes_metrics` AND the frame has a per-opcode attribution (watch events have none → `response_bytes` only).

**Decode failure** (short frame, oversized frame, empty correlation queue, unknown xid): `decoder_error` increments (+ the flag-gated `<opname>_decoder_error` when the opname is known from a correlation hit), `writeBuf` is abandoned (no resync), the connection is NEVER closed, passthrough is unconditional — the request-side AMEND-A8 semantics, symmetric.

### 3.4 Correlation consumption (R5 ratified; the parent AMEND-A7 structures)

1. **Data responses** (`xid > 0`): `requestsByXid` lookup + **erase-on-lookup** (upstream parity — a second response with the same xid finds nothing → `decoder_error`). The popped `pendingRequest` supplies the opname (for `<opname>_resp`), the wire opcode (for the §4 override lookup), and `start` (for the latency).
2. **Control responses** (xids `0`/`-2`/`-4`/`-8`): `controlRequestsByXid[xid]` **FIFO pop** (front entry removed; control xids repeat, so a queue — AMEND-A7). Empty queue → `decoder_error`.
3. **Watch events**: never touch the correlation structures.
4. **The connect-readonly → connect response-opname mapping (IMPL-panic trap — pinned here).** A readonly connect request's control-queue entry carries opname `connect_readonly` (the as-built `onConnect`, `decoder.go:163-171`), but the response-side roster has **NO `connect_readonly_resp`** (parent AMEND-A3: `connect_readonly` is rq-side-only; `respOpNames` excludes it — `stats.go:119-152`). The response decoder MUST use opname `connect` for ALL connect-response counters (`connect_resp`/`connect_resp_bytes`/`connect_resp_fast`/`connect_resp_slow`) regardless of the popped entry's opname — upstream parity (upstream's `onConnectResponse` always increments `connect_resp`). A naive `inc(entry.opname + "_resp")` would panic on `rosterStats.inc`'s closed-roster check.
5. **This consumption CLOSES the 28.1 known boundary** ("control queues grow unbounded — nothing drains them until 28.2", ADR-0222 §Decision item 5): responses now drain both structures. The residual growth case — an upstream that never responds — is **upstream-parity behavior** (upstream's `requests_by_xid_`/`control_requests_by_xid_` also grow without responses), no longer an envoy-go-specific gap; recorded as such in BEHAVIOR_CONTRACT (§7.2).

### 3.5 What stays request-side-as-built

`OnNewConnection` (no-op Continue), `OnData` (the TotalAppended feed), the request dispatch/min-length table, the dynamic auth counters, `OnDestroy` (drops the decoder; runs strictly after both pumps join — ADR-0221 §Consequences, so it needs NO lock), `SetReadFilterCallbacks`/`SetWriteFilterCallbacks`: ALL unchanged except that request-side correlation WRITES now take the §3.6 lock.

### 3.6 The synchronization design (LOAD-BEARING — discharges the ADR-0221 §Consequences forward-pointer)

**The race.** Post-handoff, goroutine A (downstream→upstream pump → `readChainConn.Read` → `replayRead` → `OnData` → request decode) and goroutine B (upstream→downstream pump → `writeChainConn.Write` → `OnWrite` → response decode) run CONCURRENTLY. The request path WRITES the correlation maps; the response path READS + ERASES them. Under the as-built lock-free design this is a data race (Go map concurrent read/write → runtime fatal). Upstream has no such race only because libevent serializes both directions onto one dispatcher thread.

**The design: one per-connection `sync.Mutex` on the decoder, guarding EXACTLY the two correlation maps.**

| State | Owner | Locking |
|---|---|---|
| `requestsByXid` + `controlRequestsByXid` | shared (A writes; B reads/erases) | **`decoder.mu`** — every map access (request-path insert/append, response-path lookup/erase/pop) holds the lock; entries are COPIED OUT under the lock; all counter increments + latency math happen OUTSIDE the lock |
| `readBuf` + `chainConsumed` | goroutine A only (pre-handoff: the serveNetworkChain goroutine, happens-before A) | lock-free |
| `writeBuf` | goroutine B only | lock-free |
| `cfg` / `stats` (incl. all counters) | shared, read-only / atomic | lock-free (`stats.Counter` increments are atomic by construction; `compiledConfig` is immutable after boot) |
| the decoder pointer itself (`filter.decoder`) | `OnDestroy` nils it strictly AFTER both pumps join | lock-free (the ADR-0221 happens-after edge) |

Pinned consequences:

1. **Lock granularity is per-map-access, not per-frame.** The lock is held only for the map mutation/lookup (nanoseconds), never across counter increments, latency computation, or byte parsing. No lock ordering issues exist (there is exactly one lock).
2. **`pendingRequest` is copied by value under the lock** (it is a 3-field struct: string + int32 + `time.Time`); after the copy, the response path operates on its private copy.
3. **The pre-handoff request path also takes the lock** (uniformity over cleverness): pre-handoff there is no goroutine B, so the lock is uncontended and costs one atomic CAS; making the locking unconditional removes any "which regime am I in" state and keeps the invariant trivially auditable.
4. **The race test** (NEW; `decoder_test.go`): two goroutines over one decoder — one driving `decodeOnData` with a request stream, one driving `decodeOnWrite` with the matching response stream — under `go test -race -count=5`. This is the zookeeperproxy-package analogue of the 28.1b framework-level concurrent-pumps test (which stays green unchanged — its synthetic filters are not zookeeperproxy).
5. **ADR-0223's §Decision body records this design** — the forward-pointer in ADR-0221 §Consequences ("the 28.2 SPEC MUST add synchronization — the anticipated shape: a per-connection `sync.Mutex` on the decoder guarding the correlation maps (+ any shared latency state); the per-direction reassembly buffers stay unshared/lock-free") is discharged EXACTLY as anticipated. There is no "shared latency state" beyond the maps themselves (`start` rides inside `pendingRequest`).

### 3.7 File delta

| File | Change | Responsibility |
|---|---|---|
| `internal/filter/network/zookeeperproxy/decoder.go` | EXTEND + rename | `requestDecoder` → `decoder`; `writeBuf` + `mu`; `decodeOnWrite`/`nextWriteFrame`/`decodeResponseFrame`/response-correlation/latency (§3/§4); lock the request-path map writes |
| `internal/filter/network/zookeeperproxy/decoder_test.go` | EXTEND | response-path unit tests + the §3.6 race test + mechanical rename updates |
| `internal/filter/network/zookeeperproxy/zookeeperproxy.go` | MODIFY | `OnWrite` body (§3.2); doc-comment updates |
| `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go` | EXTEND | OnWrite-feeds-decoder + Continue-always tests |
| `internal/filter/network/zookeeperproxy/fuzz_test.go` | EXTEND | the 38th fuzzer (§6) |
| `internal/filter/network/zookeeperproxy/config.go`, `stats.go` | **UNCHANGED** | all config fields + all 201 counters already exist |
| `internal/filter/network/` (framework), `internal/listener/`, `internal/stats/`, `internal/bootstrap/` | **UNCHANGED** | §2.4 |
| `test/differential/fixture/fixture.go` | EXTEND | `TCPZKResponder BackendKind = 29` (§5.1) |
| `test/differential/runner_test.go` | EXTEND | the `TCPZKResponder` backend arm + the `0048` blank-import |
| `test/fixtures/0048-zookeeper-responses/` | **NEW** | driver + README (§5.2) |

---

## 4. Latency-threshold counters (parent §11.7 / AMEND-A10; lands under ADR-0223)

### 4.1 Measurement + decision (the upstream `errorBudgetDecision` mirror)

On every CORRELATED response (data / control / connect — never watch events):

```
latency := time.Since(entry.start)            // entry = the popped pendingRequest
if !cfg.enableLatencyThresholdMetrics { skip }  // flag gates INCREMENTS, not creation (AMEND-A2)
threshold := cfg.latencyThresholdOverrides[entry.wireOpcode]  // wire-opcode-keyed (AMEND-A6/A10)
if absent { threshold = cfg.defaultLatencyThreshold }          // 100 ms default (already parsed)
if latency <= threshold { inc(respOpname + "_resp_fast") }     // INCLUSIVE (AMEND-A10)
else                    { inc(respOpname + "_resp_slow") }
```

Pinned semantics:

1. **`respOpname`** is the §3.4-item-4 mapped opname (`connect` for connect responses regardless of readonly; the entry's opname otherwise).
2. **The comparison is inclusive** (`<=` → fast) — parent §11.7 (`filter.cc:134-154`).
3. **The override map is keyed by WIRE opcode** — `cfg.latencyThresholdOverrides` is already built at 28.1 config parse via `protoToWireOpcode` (`config.go:192-209`); 28.2 only READS it. The connect response uses `entry.wireOpcode == opConnect (0)` — an override `{opcode: Connect, threshold: …}` therefore applies to connect responses (upstream parity: connect is in the proto enum + the opcodeMap).
4. **Watch events never get fast/slow** (uncorrelated — no request timestamp exists).
5. **`decoder_error` responses never get fast/slow** (no correlation hit → no entry).
6. **All latency config is parsed-and-validated at 28.1** (`config.go`): 28.2 is consumption-only — zero parse changes, zero new PARSE-REJECT arms (§2.3).

### 4.2 Determinism posture

`time.Since` makes per-response latency NONDETERMINISTIC in absolute value — which is exactly why the differential proof uses EXTREME thresholds (parent AMEND-A10 / the deterministic-threshold differential discipline): the `0048` fast arm uses `default_latency_threshold: 3600s` (no real round-trip takes an hour → ALL responses fast on both sides) and the slow arm uses `0.001s` (the PGV minimum) with a backend that delays ≥ 10 ms before responding (→ ALL responses slow on both sides). The discipline is recorded in ADR-0223's body; unit tests cover the boundary cases deterministically with injected timestamps (a `pendingRequest.start` set in the past).

---

## 5. The proof surface — fixture `0048` + the `TCPZKResponder` backend

### 5.1 `TCPZKResponder` (NEW BackendKind = 29; resolves parent D-P9; user-confirmed D-28.2-2)

The as-built `TCPSink` (= 28) is pinned request-side-only (`fixture.go:493-502`: "28.2's 0048 uses a driver-controlled responder — a separate kind"). 28.2 adds:

```go
// TCPZKResponder is a ZooKeeper-aware canned-response TCP backend: for every
// complete ZK request frame it reads (4-byte BE length prefix + frame), it
// waits a FIXED delay (zkResponderDelay, ~10ms), then writes a correlated
// canned response frame. Added at 28.2 for 0048-zookeeper-responses
// (SPEC §5). The fixed delay is the deterministic-threshold construction
// (parent D-P9): every measured latency is ≥ the delay on BOTH sides, so a
// 1ms threshold makes every response slow and a 3600s threshold makes every
// response fast — no cross-side timing nondeterminism.
TCPZKResponder BackendKind = 29
```

Responder behavior (runner-side `acceptZKResponder`, the `acceptSinkCounting` sibling):

1. **Connect request** (leading int32 of frame == 0) → connect response: `proto_version(0) + timeout(4) + session_id(8) + password(len 0)` = 20-byte frame.
2. **Any other request** (data or control xid) → standard response: `xid(echoed) + zxid(8, monotonic per connection) + error(4, 0)` = 16-byte frame.
3. **Trigger — wrong-xid** (a designated request opcode, anticipated `getacl`): respond with `xid + 1000` instead of the echoed xid → uncorrelated on both sides → `decoder_error`.
4. **Trigger — watch-event push** (a designated request opcode, anticipated `exists`): write the standard response, THEN write an unsolicited watch-event frame (`xid -1 + event_type + client_state + path`) → `watch_event` on both sides.
5. **Fixed pre-response delay** (~10 ms) before EVERY response write (item 1/2; the triggers inherit it).
6. The exact trigger-opcode encoding + delay constant are PLAN-time decisions (D-S28.2-2); the SPEC pins the four behaviors + the fixed-delay discipline.

The responder parses ONLY the request frame's length prefix + leading xid + (for triggers) the opcode int — ~40 lines of `encoding/binary`; it is NOT a ZooKeeper server (no session/watch semantics).

### 5.2 `0048-zookeeper-responses` (cross-side; the `0046` driver template)

Chain `[zookeeper_proxy, tcp_proxy]` on BOTH sides; ONE shared `TCPZKResponder` backend; **four listeners** per side (multi-listener per the `0046`/`0043` precedent; one `StatsAsserter.AssertStats` over all four stat_prefix scopes):

| Listener | stat_prefix | zookeeper_proxy config | Arms |
|---|---|---|---|
| `l_resp` | `zk_resp` | defaults (no latency metrics, no flags) | 1–3 |
| `l_fast` | `zk_fast` | `enable_latency_threshold_metrics: true`, `default_latency_threshold: 3600s` | 4 |
| `l_slow` | `zk_slow` | `enable_latency_threshold_metrics: true`, `default_latency_threshold: 0.001s`, `latency_threshold_overrides: [{opcode: GetData, threshold: 3600s}]` | 5–6 |
| `l_rflags` | `zk_rflags` | `enable_per_opcode_response_bytes_metrics: true` | 7 |

Arms (every assertion via cross-side `StatsAsserter`; the body differential is intrinsically vacuous — parent §8):

1. **Round-trips** (`l_resp`): connect + getdata(xid 1) + create(xid 2) + ping + close(xid 3), each answered → `connect_resp`/`getdata_resp`/`create_resp`/`ping_resp`/`close_resp` each +1; `response_bytes` equal cross-side; the request-side `*_rq` counters ALSO equal (the 28.1 surface re-proven through round-trips). With latency metrics DISABLED on this listener: all `*_resp_fast`/`*_resp_slow` stay 0 (flag-gating proof).
2. **Watch event** (`l_resp`): exists(xid 4) [push trigger] → `exists_resp` +1 AND `watch_event` +1; `watch_event` is never correlated (no fast/slow).
3. **Unknown xid + survival** (`l_resp`): getacl(xid 5) [wrong-xid trigger] → `decoder_error` +1; then sync(xid 6) on the SAME connection → `sync_resp` +1 (the connection survives + later responses decode — the abandon-no-resync recovery proof, arm-4-of-0046 symmetry).
4. **All-fast** (`l_fast`): connect + getdata(xid 1) + setdata(xid 2) → `connect_resp_fast`/`getdata_resp_fast`/`setdata_resp_fast` each +1; ALL `*_resp_slow` == 0. (3600 s threshold; deterministic.)
5. **All-slow** (`l_slow`): connect + setdata(xid 1) + delete(xid 2) → `connect_resp_slow`/`setdata_resp_slow`/`delete_resp_slow` each +1; their `*_resp_fast` == 0. (1 ms threshold + ≥10 ms responder delay; deterministic.)
6. **Override** (`l_slow`, same connection as arm 5 or a fresh one): getdata(xid 3) → `getdata_resp_FAST` +1 while setdata/delete were SLOW — proves the wire-opcode-keyed override map consumption (GetData override 3600 s beats the 1 ms default).
7. **Flag-gated resp-bytes** (`l_rflags`): getdata(xid 1) round-trip → `getdata_resp_bytes` > 0 and equal cross-side; on `l_resp` (flag false) `getdata_resp_bytes` stays 0 on both sides.
8. **R4 deliberate-break** (recorded procedure per the `reference_differential_asserter_dispatch` discipline + the 0030 lesson): (a) temporarily assert `getdata_resp == 2` (when 1 is driven) → MUST fail both runner paths (`-count=1` per `reference_differential_break_protocol_count1` — go test caching serves stale PASSes otherwise); (b) temporarily make the §4.1 comparison exclusive (`<` instead of `<=`)… [NOT differentially visible under extreme thresholds — the unit boundary test owns that]; instead (b) = temporarily skip the `connect_resp` increment → arm 1 MUST fail. Both breaks reverted; protocol + outputs recorded in driver comments + the fixture README + PROGRESS.md.

**Port allocation:** reference listener ports continue the `150NN` convention from `0046`/`0047` (15047/15048/15049 taken) → `l_resp`=15050, `l_fast`=15051, `l_slow`=15052, `l_rflags`=15053 (re-pinned at IMPL Task 1 against the live fixture roster — D-S28.2-3).

**Dispatch constraints honored:** `0048` is a CROSS-SIDE fixture (one runner branch — `reference_differential_fixture_dispatch_constraint`); ALL stat assertions go through `StatsAsserter` (`reference_differential_asserter_dispatch`); the driver redefines wire opcodes locally (no `internal/` import — the 0046 import-cycle precedent).

### 5.3 Counts

Differential dirs: 49 → **50** (tail `0048-zookeeper-responses`). The FULL 49-dir pre-existing suite re-runs green at the six-gate (the no-regression gate — `0046`/`0047` prove the request side undisturbed by the decoder rename + locking).

---

## 6. The 38th fuzzer (resolves parent D-P6 — separate; user-confirmed D-28.2-3)

`FuzzZookeeperResponseDecode` (`fuzz_test.go` EXTEND): feeds arbitrary bytes through `decodeOnWrite` on a decoder pre-loaded with a few pending requests (so the correlation paths are reachable), asserting: no panic; `writeBuf` stays bounded (≤ max_packet_bytes + partial-frame slack — the 37th fuzzer's bounded-reassembly discipline); the correlation maps never grow from response input (responses only erase); counter operations never panic (the closed-roster `inc` cannot receive an unknown suffix — the §3.4-item-4 mapping is exactly what this assertion guards). Seed corpus: a valid data response, a connect response, a watch event, a control (ping) response, an unknown-xid response, a truncated frame, an oversized frame. Fuzzers 37 → **38** (recipe: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`, scoped to `./internal` — parent §11.10).

---

## 7. The phase-28 completion bundle

### 7.1 ADR disposition — NO new ADR number

- **ADR-0223** (`DECISIONS.md:14324`): the §Decision/§Consequences body lands at the 28.2 IMPL completion task per its existing §Context draft, covering: the response decoder (§3 here), **the per-connection mutex (§3.6 — the discharged ADR-0221 forward-pointer)**, the latency-threshold counters + the deterministic-threshold differential discipline (§4), the `TCPZKResponder` + `0048` (§5), the 38th fuzzer (§6), the latency-HISTOGRAM coverage boundary, and the parent-row-28 ROLLUP. **A one-line in-place §AMEND on ADR-0223's §Context lands AT THIS SPEC COMMIT** (the ADR-0221-at-28.1b-SPEC precedent) recording: the unified-decoder + per-connection-mutex synchronization design, the `TCPZKResponder` fixed-delay deterministic-threshold construction, and the D-P4/P6/P9 resolutions (§9.1).
- **ADR-0221/ADR-0222**: bodies are COMPLETE (landed at 28.1b); untouched at 28.2 except that ADR-0221 §Consequences' forward-pointer is now satisfied (the 0223 body cross-references it as discharged).
- **DECISIONS.md tail STAYS ADR-0223; next-free STAYS ADR-0224.** Phase 28 closes having consumed exactly the three BRAINSTORM-locked numbers.

### 7.2 BEHAVIOR_CONTRACT 28.2 bundle (ONE atomic landing per ADR-0052, at the 28.2 IMPL completion task)

- The `### envoy.filters.network.zookeeper_proxy` subsection EXTENDS with the response side: the response dispatch table (§3.3), the correlation-consumption semantics + erase-on-lookup + FIFO-pop (§3.4), the connect-readonly→connect response mapping, the latency fast/slow semantics (`<=` inclusive; wire-opcode-keyed overrides; flag-gated), the response-side shallow-decode leniency departure (§2.2), and the watch-event semantics.
- The conn-wrap-seam framework block's **28.2 forward-pointer is RESOLVED**: the per-connection decoder mutex is recorded as the landed synchronization (the goroutine-A/goroutine-B race closed).
- The **latency-HISTOGRAM coverage-boundary record** (ADR-0060): `connect_response_latency` + `<opname>_latency` + `unknown_opcode_latency` unmirrored.
- The 28.1 "control queues grow unbounded" boundary is REWRITTEN as upstream-parity behavior (§3.4 item 5).
- **Stat table: STAYS 337** — zero creation delta (all response-side counters were created at 28.1; 28.2 wires increments). The table's zookeeper rows gain "incremented at 28.2" annotations where applicable.
- An explicit note distinguishing the **27-value proto `Opcode` enum** (which keys `latency_threshold_overrides` config-side via `protoToWireOpcode`) from the **26-value gapped wire-opcode enum** (which the decoder dispatches on) — the two rosters differ by design (AMEND-A6) and the distinction is pinned to pre-empt future reviewer confusion (spec-reviewer advisory).
- The parent-row-28 family-close note (the THIRD §9 row done; 4 candidates remain).

### 7.3 ROADMAP rollup (the 18/19/22/24/25/26 final-sub-phase precedent)

At the 28.2 IMPL phase-done commit, ATOMICALLY: sub-row **28.2 `in-progress → done`** AND parent row **28 `in-progress → done`** (with the final-counts summary in the parent row's notes — the 26-parent-row template). The §9 family candidate list drops to 4 (`redis`/`mongo`/`kafka_broker`/`thrift`). STATE.md advances to "awaiting next phase brainstorm" (SKILL_ROUTING state 0) unless the session pins the next phase; next-prompt.txt is rewritten accordingly.

---

## 8. SPEC-time empirical pins

### 8.1 D-S28.2-0 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip **`8334d1d`** (the docs-only next-prompt repoint trailing the 28.1b-IMPL squash `fdf40ea` by +2) at this SPEC session. The 28.2 IMPL Task-1 first-action gate RE-RUNS these against the live IMPL-session tip.

**Counts:**

- Differential fixture dirs: **49 active** (`0000`–`0047`; tail `0047-zookeeper-boot-reject`). 28.2 → **50** (+ `0048`).
- Fuzzers: **37** (canonical `./internal`-scoped recipe). 28.2 → **38**.
- Stat surface (BEHAVIOR_CONTRACT table): **337** (the 28.1b roll). 28.2 → **337** (unchanged — increments only).
- DECISIONS.md tail: **ADR-0223** (§Context at `:14324`; §Decision/§Consequences body PENDING — lands at the 28.2 IMPL); next-free **ADR-0224**.
- Conformance: h2spec 53/53; proxy-wasm 10/10 (re-run live at the 28.1b closure).
- Baseline build + tests at this SPEC session's worktree: `go build ./...` clean; `go test ./internal/filter/network/... -count=1` all packages ok.

**As-built anchors (the §3–§6 design extends/modifies these exact sites):**

- `internal/filter/network/zookeeperproxy/decoder.go:30-56` — the `requestDecoder` struct (rename target; `requestsByXid`/`controlRequestsByXid` at `:50,55`); `:75-90` — `decodeOnData` (untouched); `:96-112` — `nextFrame` (the `nextWriteFrame` template); `:116-141` — `decodeFrame` (the request dispatch the response dispatch mirrors); `:147-173` — `onConnect` (the connect_readonly entry-opname source); `:216-219` — `recordControl` (a lock site); `:308-334` — `onDataRequest` (`requestsByXid` write at `:332` — a lock site); `:224` — `wireFootprint` (shared accounting basis); `:239-245` — `decoderError` (the response path reuses it).
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go:76` — the no-op `OnWrite` (the §3.2 replacement site); `:65-68` — `OnData` (unchanged); `:88` — `OnDestroy` (unchanged).
- `internal/filter/network/zookeeperproxy/config.go:118-137` — `compiledConfig` (the latency fields at `:128-130`, parsed-not-consumed — the §4 consumption targets); `:148-155` — the PARSE-REJECT constants (unchanged); `:192-209` — the override-map build (unchanged).
- `internal/filter/network/zookeeperproxy/stats.go:119-152` — `respOpNames` (28 names; NO `connect_readonly` — the §3.4-item-4 trap); `:204-220` — `inc`/`add` (panic-on-unknown-suffix).
- `internal/filter/network/writeconn.go:34-48` — `writeChainConn.Write` (fresh per-Write Buffer; `endStream=false`; the §3.2 item-1 basis).
- `internal/filter/network/chain.go:389-395` — `replayRead` (the goroutine-A path); `internal/filter/tcpproxy/filter.go:134-138` — the two pumps + `wg.Wait()` (the §3.6 goroutine anchors).
- `test/differential/fixture/fixture.go:493-502` — `TCPSink = 28` + the 0048-responder forward-pointer comment (the §5.1 insertion site); `:505-510` — `BackendKindAware`.
- `test/differential/runner_test.go:71-72` — the `0046`/`0047` blank-imports (the `0048` import lands below them); `:827-840` — the `TCPSink` backend arm (the `TCPZKResponder` arm's sibling); `:1258-1269` — `acceptSinkCounting` (the `acceptZKResponder` template).
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` — the multi-listener + StatsAsserter + local-opcode-constants driver template (875 LoC); ref ports 15047/15048; `0047` took 15049.

### 8.2 Inherited empirical pins (constrain the §3/§4 design; no re-probe needed)

- **Response framing** (parent §11.4, probed against upstream `decoder.cc:256-359`): connect response = proto_version+timeout+session+password, NO zxid/error; watch event = never correlated; standard response = xid+zxid(8)+error(4); unknown response xid → `decoder_error`.
- **Latency semantics** (parent §11.7, `filter.cc:134-154`): `<=` inclusive; per-opcode override keyed by wire opcode via `opcodeMap`; default 100 ms; flag gates increments.
- **Decoder-error/passthrough** (parent §11.5): unconditional passthrough; abandon-no-resync; never close.
- **Write-path mechanics** (parent §11.8 + ADR-0221): `OnWrite` runs on goroutine B via `writeChainConn.Write`; one filter instance, both directions.
- **The roster asymmetries** (parent AMEND-A3 + the 28.1a live-dump): no `connect_readonly_resp`; `auth_resp*` exist (SetAuth opname = `auth`); `setauth_resp*` exist-but-never-incremented (upstream parity).

---

## 9. SPEC-time D-questions

### 9.1 Resolved at this SPEC (user-confirmed at the design dialogue, 2026-06-02)

- **D-28.2-1 (decoder/synchronization shape) — RESOLVED: unified per-connection decoder + ONE `sync.Mutex` guarding the correlation maps (§3.1/§3.6).** Alternatives rejected: a separate `responseDecoder` + shared locked correlation store (more churn, no upstream analogue); channel-based serialization onto one goroutine (over-engineered; adds latency measurement skew).
- **D-28.2-2 (0048 backend; resolves parent D-P9) — RESOLVED: NEW `TCPZKResponder` BackendKind = 29, ZK-aware canned responder with a FIXED ~10 ms pre-response delay (§5.1).** The fixed delay is the deterministic-threshold construction: one backend serves both the all-fast (3600 s) and all-slow (1 ms) arms. Alternatives rejected: extending `TCPSink` (contradicts the as-built fixture.go:500 pin; risks the green `0046`); per-arm backends (needless backend-count complexity).
- **D-28.2-3 (resolves parent D-P6) — RESOLVED: separate 38th fuzzer `FuzzZookeeperResponseDecode` (§6).** Distinct entry point + distinct framing; also fuzzes the mutex.
- **D-28.2-4 (resolves parent D-P4) — RESOLVED: the five latency PARSE-REJECT arms stay unit-test-only.** No `0047` extension, no new boot-reject dir. Rationale: the arms are byte-stable + unit-proven since 28.1a; `0047` already proves the boot-reject MECHANISM cross-side (PGV-mirror substring parity); boot-reject fixture dirs stay one-per-sub-phase-need (the dispatch-constraint memory). The duplicate-opcode reject (an upstream constructor-time check, not PGV) is covered by the same rationale — its unit test asserts the byte-stable wording.
- **D-28.2-5 (connect-readonly response mapping) — RESOLVED at this SPEC: connect responses always use opname `connect` (§3.4 item 4)** — upstream parity + the closed-roster panic trap.

### 9.2 Open for PLAN / IMPL resolution

- **D-S28.2-1 (exact special-framing min-lengths).** The connect-response (20 + password) and watch-event (16) minimums are this SPEC's shallow-decode pins; the IMPL verifies them against upstream `decoder.cc` `parseConnectResponse`/`parseWatchEvent` ensureMinLength calls (the D-S28.1-1 transcription discipline) and adjusts if upstream differs. **Resolution at:** IMPL (the response-dispatch task's first action).
- **D-S28.2-2 (responder trigger encoding + delay constant).** Which request opcodes trigger the wrong-xid / watch-event-push behaviors (anticipated `getacl` / `exists`), and the exact delay (anticipated 10 ms). **Resolution at:** PLAN.
- **D-S28.2-3 (0048 port assignments).** Anticipated 15050–15053; re-pinned at IMPL Task 1 against the live fixture roster.
- **D-S28.2-4 (frame-scanner extraction).** Whether `nextFrame`/`nextWriteFrame` share an extracted helper or stay parallel methods. **Resolution at:** IMPL (either acceptable; prefer whichever reads cleaner after the rename).
- **D-S28.2-5 (zxid/error field disposition).** The response decoder reads past zxid(8)+error(4) for min-length validation but extracts neither (shallow decode — no consumer). **Resolution at:** IMPL confirms upstream emits no counter keyed on the error value (parent §11.4 says error only feeds dynamic metadata, which is deferred); if upstream counts nonzero-error responses differently, this becomes an AMEND.

---

## 10. RATIFIED-PENDING items (extends the parent §13 / 28.1b §9 rosters)

- **R1 (seam back-compat) — unchanged owner; re-verified.** All 47 zero-write-filter fixture dirs stay byte-exact green at the 28.2 six-gate (the decoder rename + mutex touch nothing on their path).
- **R3 (passthrough invariant) — extended to the write side.** zookeeperproxy never drains/mutates the chain `Buffer` in `OnWrite`, never returns StopIteration, never closes. Ratified by `0048` round-trips passing through tcp_proxy byte-exact + unit tests.
- **R4 (StatsAsserter liveness).** The §5.2 arm-8 deliberate-break protocol on `0048`, executed with `-count=1` (`reference_differential_break_protocol_count1`).
- **R5 (correlation hand-off) — RATIFIED HERE.** The 28.1-written structures are consumed by the 28.2 response decoder; the `0048` correlation arms (1/3/5/6) are the ratification proof.
- **R6 (counts).** IMPL Task 1 re-pins: fixtures 49 → 50; fuzzers 37 → 38; stat surface 337 (unchanged); DECISIONS tail ADR-0223 / next-free ADR-0224 (both unchanged at 28.2 close — phase 28 ends having minted no number beyond the BRAINSTORM three).
- **R7 (Prometheus parity) — unchanged mechanics.** The `.zookeeper.` flattening already covers all `_resp*` names (same shape); ratified intrinsically by the `0048` both-sides-prometheus scrape.
- **R9 (NEW — synchronization soundness).** Concurrent request decode (goroutine A) + response decode (goroutine B) over one decoder is race-free. Ratified by the §3.6 race test (`-race -count=5`) + the `0048` fixture (live concurrent pumps under real traffic) + the six-gate race run.
- **R10 (NEW — bounded write-side memory).** `writeBuf` is bounded by `max_packet_bytes` + one partial frame; the correlation maps are bounded by outstanding (unanswered) requests — upstream-parity (§3.4 item 5). Ratified by the 38th fuzzer's bounded-buffer assertion + unit tests.

---

## 11. Per-task structure (~11 tasks; the SPEC-anticipated task spine)

The 28.2 PLAN authors the exact bite-sized TDD tasks (it may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate (§8.1 recipes re-run against the live IMPL tip) | §8.1 / R6 |
| 2 | Decoder rename (`requestDecoder` → `decoder`) + `writeBuf`/`mu` fields + request-path lock acquisition + mechanical test updates (all existing tests stay green) | §3.1 / §3.6 |
| 3 | Response framing + dispatch (`decodeOnWrite`/`nextWriteFrame`/`decodeResponseFrame`: connect / watch / control / data / unknown / oversized / short) + unit tests | §3.2 / §3.3 |
| 4 | Correlation consumption (erase-on-lookup + FIFO pop + empty-queue/missing-xid → `decoder_error` + the connect-readonly→connect mapping) + `response_bytes`/flag-gated `*_resp_bytes` + unit tests | §3.4 |
| 5 | Latency-threshold counters (the §4.1 decision mirror; injected-timestamp boundary tests incl. the `<=` inclusive edge) | §4 |
| 6 | `OnWrite` glue (replace the no-op) + the §3.6 concurrent request/response race test (`-race -count=5`) | §3.2 / §3.6 / R9 |
| 7 | The 38th fuzzer (`FuzzZookeeperResponseDecode`) + seed corpus | §6 |
| 8 | `TCPZKResponder` BackendKind + the runner backend arm (`acceptZKResponder`) | §5.1 |
| 9 | `0048-zookeeper-responses` driver + cross-side GREEN on all arms + R4 deliberate-break + fixture README | §5.2 / R4 / R5 |
| 10 | Completion bundle: ADR-0223 §Decision/§Consequences body + BEHAVIOR_CONTRACT 28.2 bundle | §7.1 / §7.2 |
| 11 | Six-gate (incl. the FULL 50-dir differential suite) + STATE.md + the ROADMAP ATOMIC rollup (sub-row 28.2 + parent row 28 → done) + next-prompt.txt for the next-phase cold-start | §7.3 / §12.2 |

### 11.1 ADR-0045 split-gate check

Production-LoC estimate (the 26.x accounting basis — production code; fixture drivers + tests excluded): decoder response path + correlation + latency (~250–350); `OnWrite` glue (~10); `fixture.go` BackendKind + runner responder arm (~100–130); **total ~360–490 production LoC**, ~11 tasks. Comfortably within the ~1500-LoC / ~25-task gate — **no split**. (The `0048` driver ~700–900 LoC + the test surface are excluded per the established accounting. The parent §11.9 estimated 28.2 at ~600–900 production LoC including the driver-adjacent surfaces; this re-check confirms "fits" either way.)

---

## 12. Test surface + acceptance checklist

### 12.1 Test surface

- **Layer A — zookeeperproxy unit tests** (`decoder_test.go` + `zookeeperproxy_test.go`): the mechanical rename updates; response dispatch per row of the §3.3 table (connect / watch / control ping+auth+setwatches / data / unknown-negative / short / oversized / partial-frame reassembly across multiple OnWrite calls / abandon-no-resync recovery); correlation (erase-on-lookup; double-response → `decoder_error`; FIFO ordering on repeated control xids; empty-queue → `decoder_error`; connect-readonly → `connect_resp`); byte accounting (`response_bytes` always; `*_resp_bytes` flag-gated both ways); latency (injected timestamps: `latency == threshold` → fast [the inclusive edge]; `latency > threshold` → slow; override beats default; flag-off → neither; watch events → neither); the §3.6 race test (concurrent decodeOnData + decodeOnWrite, `-race -count=5`); `OnWrite` always-Continue + chain-Buffer-never-mutated.
- **Layer C — fuzz**: the 38th fuzzer (§6); the 37th re-runs unchanged (the rename is transparent to it).
- **Layer D — differential**: `0048` (all arms cross-side + R4) + the FULL 49-dir pre-existing suite (R1 + the `0046`/`0047` request-side no-regression gate) → 50/50 green, `-count=1`.
- **Layer E — race**: `go test -race -short ./internal/filter/network/...` (now exercising the locked correlation paths).
- **Per-task hygiene**: `gofmt -l` + `golangci-lint run` on touched packages, every task (`feedback_pertask_gofmt_lint`).

### 12.2 Six-gate checklist (the 22/24/25/26/27/28.1 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (**50 dirs**) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected; re-run live if the harness is available). All outputs quoted into PROGRESS.md honestly (`-count=1` on the differential + any flake re-runs in isolation per the 28.1a/28.1b closure precedent).

### 12.3 28.2 IMPL acceptance checklist

1. The response decoder lands per §3 (dispatch + correlation + the connect-readonly mapping + decode-failure symmetry); `OnWrite` replaces the no-op; the chain `Buffer` is never mutated; `Continue` always returned.
2. The per-connection mutex lands per §3.6; the race test is green `-race -count=5`; ADR-0223's body records the synchronization (the ADR-0221 forward-pointer discharged).
3. The latency fast/slow counters land per §4 (`<=` inclusive; wire-opcode-keyed overrides; flag-gated); unit boundary tests green.
4. `0048-zookeeper-responses` + `TCPZKResponder` land and are GREEN on all arms cross-side; R4 recorded; R5 ratified; fixture README authored.
5. The 38th fuzzer lands; counts: 50 fixture dirs, 38 fuzzers, stat surface 337, DECISIONS tail ADR-0223 / next-free ADR-0224 (R6).
6. ADR-0223 §Decision/§Consequences body in place; the BEHAVIOR_CONTRACT 28.2 bundle lands (§7.2).
7. Six gates green (§12.2); STATE.md advanced; **ROADMAP sub-row 28.2 AND parent row 28 flip `→ done` ATOMICALLY**; next-prompt.txt rewritten for the next-phase cold-start (SKILL_ROUTING state 0 unless the closing session pins the next phase).

---

## 13. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 28.2 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.1b precedent); parent row 28 STAYS `in-progress` (it rolls at the 28.2 IMPL, §7.3). The ADR-0223 §Context one-line §AMEND (§7.1) lands at this SPEC commit. STATE.md advances to lifecycle-state 2-for-28.2 with `next-skill = superpowers:writing-plans` scoped to the **28.2 PLAN** (`docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 28.2-PLAN cold-start. Per `feedback_execution_style` the 28.2 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies (subagents commit LOCAL-ONLY; the controller squash-merges + pushes).

---

## Appendix A — Cross-references

| 28.2 SPEC § | Master section | Relationship |
|---|---|---|
| §1 Purpose | parent SPEC §3.2 + ROADMAP row 28.2 + ADR-0223 §Context | executes |
| §2 Non-purposes | parent §2 + 28.1b §2 | inherits + adds §2.3/§2.4 |
| §3 Response decoder | parent §11.4/§11.5 + ADR-0222 (the as-built request package) | EXTENDS (the write-direction half; NEW design) |
| §3.4 item 4 | parent AMEND-A3 + stats.go respOpNames | defuses (the connect_readonly resp trap) |
| §3.6 Synchronization | 28.1b SPEC §3.6 + ADR-0221 §Consequences | DISCHARGES (the forward-pointer) |
| §4 Latency counters | parent §11.7 / AMEND-A10 + config.go (parsed-not-consumed) | consumes |
| §5 Fixture + backend | parent §8.3 / D-P9 + fixture.go:500 | executes + resolves |
| §6 Fuzzer | parent §11.10 / D-P6 | resolves (separate 38th) |
| §7 Completion bundle | parent §9/§10 + ADR-0223 + the 18/19/22/24/25/26 rollup precedent | executes (incl. the ATOMIC parent rollup) |
| §8 Empirical pins | parent §11 + 28.1a/28.1b PROGRESS | re-pins against tip `8334d1d` |
| §9 D-questions | parent §12 (D-P4/P6/P9) | resolves (user-confirmed) + opens D-S28.2-1..5 |
| §10 RATIFIED-PENDING | parent §13 + 28.1b §9 | re-scopes; ratifies R5; adds R9/R10 |
| §11 Task spine | 28.1b §10 | NEW (28.2's own spine; split-gate check) |
