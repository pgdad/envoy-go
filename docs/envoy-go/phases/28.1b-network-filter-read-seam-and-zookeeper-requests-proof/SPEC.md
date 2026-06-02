# Phase 28.1b SPEC — the read-side seam + the `zookeeper_proxy` cross-side proof + the 28.1 completion bundle

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 28.1b** (`network-filter-read-seam-and-zookeeper-requests-proof`), the second/closing half of the **invoked ADR-0045 28.1a/28.1b split** (user-approved 2026-06-02; DECISIONS.md ADR-0045 §AMEND). Unlike the 22.x/25.x/26.x sub-phases, 28.1b is NOT a BRAINSTORM-time pre-split: it exists because the 28.1 IMPL Task-16 `0046` fixture exposed a design gap the 28.1 SPEC never covered (the read-side seam). The **28.1 SPEC** (`../28.1-network-filter-write-seam-and-zookeeper-requests/SPEC.md`) is this sub-phase's MASTER: its §3 (WriteFilter seam), §4 (zookeeperproxy package), §6 (PARSE-REJECT roster), §7 (stat surface), §8 (fixture taxonomy), §9 (BEHAVIOR_CONTRACT bundle), and §13 (R1–R7) remain authoritative. This SPEC ADDS the read-side seam design (§3 here), re-scopes the proof + completion surface onto 28.1b (§5/§6 here), and changes the 28.1a-landed code ONLY where the seam requires it (§4 here — one decoder-feed re-base). The next session, per SKILL_ROUTING state 2, authors the **28.1b PLAN** from this SPEC.

**Goal:** Land the symmetric **read-side seam** — a `readChainConn` conn-wrap that re-feeds post-terminal-handoff socket reads back through the read-filter chain, so `zookeeper_proxy` (and any future request-decoding network filter) sees EVERY frame per connection, matching reference Envoy's forever-re-iteration (PROGRESS.md Task-16 proof-of-cause) — then prove it cross-side (`0046-zookeeper-requests` re-enabled + green + R4), land `0047-zookeeper-boot-reject`, and close phase 28.1 with the completion bundle (ADR-0221/0222 bodies; BEHAVIOR_CONTRACT 136 → 337; ROADMAP sub-row advance).

**Architecture:** The existing `internal/filter/network/` package gains `readconn.go` (the `readChainConn`, mirroring `writeconn.go`/`prefixconn.go`'s embed-and-override-one-method shape), a `chainRuntime.replayRead` post-handoff replay path (append → re-iterate read filters → drain; bounded memory), and a `Buffer.TotalAppended()` monotonic counter that the zookeeperproxy decoder's high-water mark re-bases onto (the ONLY 28.1a-landed code this sub-phase modifies). `handleTerminal` installs the readChainConn under the SAME predicate as the writeChainConn (≥1 write filter), so the two seams wrap together and every existing production chain stays UNWRAPPED (R1). `manager.go`, `tcp_proxy`, HCM, and the zookeeperproxy config/stats/dispatch code are untouched.

**Tech Stack:** Go 1.26.2; the as-built 28.1a `internal/filter/network/` + `internal/filter/network/zookeeperproxy/` packages; the differential harness (`TCPSink` BackendKind + `fixture.StatsAsserter` + `differential.BootRejectFixture`); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). ZERO new third-party dependencies.

**Authored:** 2026-06-02. **Empirical-pin probe dates (inherited):** parent SPEC §11 (2026-06-01) + the 28.1a IMPL empirical pins (2026-06-02, PROGRESS.md Tasks 8/10/14). **Baseline-anchor pin date:** 2026-06-02 (this SPEC session, master tip `4bc9790` — §7.1).

---

## 1. Purpose / Mission

Phase 28.1b delivers the three things the ADR-0045 §AMEND assigns it:

1. **The read-side seam (§3).** envoy-go's chain runtime exits its read loop PERMANENTLY at terminal handoff (`internal/listener/manager.go` `serveNetworkChain:1066-1069`: `TerminalReady()` → `HandleTerminal()` → `return`), after which the terminal reads the downstream socket directly and no read filter's `OnData` ever runs again. Reference Envoy's `FilterManagerImpl::onRead` re-iterates the read-filter chain on EVERY socket read for the connection's lifetime. For a `[zookeeper_proxy, tcp_proxy]` chain this means envoy-go decodes only the FIRST socket read's frames (PROGRESS.md Task-16 divergence table: `ping_rq` ref=1 vs subj=0, `getdata_rq` 2 vs 0, `request_bytes` 307 vs 132). The Task-16 proof-of-cause (a temporary `readChainConn`, green then fully reverted) proved a conn-wrap re-feeding post-handoff reads through the read chain achieves EXACT cross-side parity. 28.1b designs the production shape of that seam.

2. **The cross-side proof (§5).** `0046-zookeeper-requests` is re-enabled (the runner blank-import uncommented; the driver's DISABLED banner dropped), goes green on all 7 arms, and the R4 deliberate-break liveness protocol (deferred at Task 16 — meaningless on a red baseline) is executed and recorded. `0047-zookeeper-boot-reject` lands per the 28.1 SPEC §8.2 design. Differential dirs: 47 active + 1 disabled → **49 active**.

3. **The 28.1 completion bundle (§6).** The ADR-0221 + ADR-0222 §Decision/§Consequences bodies land IN PLACE (no new ADR number — §6.1); the BEHAVIOR_CONTRACT 28.1 bundle lands (the framework-seam block now covering BOTH directions, the zookeeper_proxy subsection, the stat table **136 → 337**); ROADMAP sub-row 28.1b flips `in-progress → done`; parent row 28 STAYS `in-progress` (the rollup is 28.2's — §6.3 ratifies this default).

### 1.1 What 28.1a landed — NOT re-scoped here

Per the ADR-0045 §AMEND + STATE.md, the following are DONE at squash `8703aeb` and this SPEC does not re-design them: the `network.WriteFilter` seam (interfaces, chain classification, dual injection, `writeChainConn`, `handleTerminal` write-wrap — ADR-0221); the complete `internal/filter/network/zookeeperproxy/` request package (9-field config parse + PARSE-REJECT arms, the 201-counter eager roster + dynamic auth counters, the shallow request decoder + D-S28.1-1 min-length table + the correlation structures, both-directions filter glue — ADR-0222); the 7th built-in + bootstrap blank-import; the `.zookeeper.` `name.go` Prometheus arm; the 37th fuzzer; the `TCPSink` BackendKind + the `0046` driver (7 arms + AssertStats, committed-but-DISABLED).

**The single 28.1a-landed surface this SPEC modifies** is the decoder's chain-buffer feed (`requestDecoder.chainConsumed` + `decodeOnData`'s signature) — the §3.3 re-base, required because the 28.1 SPEC §4.5 high-water-mark contract assumes a buffer that NEVER shrinks, an assumption the bounded-memory replay path (§3.2) deliberately does not preserve. The change is behavior-identical on the pre-handoff path (§3.3 equivalence argument) and is covered by mechanical unit-test updates plus new drain-regime tests.

### 1.2 28.1b-SPEC-additive contributions

- **The read-side seam production design (§3)** — the `readChainConn` + `replayRead` + composition + the R1 wrap predicate + the observational boundaries + the concurrency pins. The 28.1 SPEC never designed this surface (its absence is the split's root cause).
- **The `Buffer.TotalAppended` decoder-feed re-base (§3.3, D-28.1b-1)** — the explicit redesign of the 28.1 SPEC §4.5 high-water-mark contract (NOT a silent break: the cumulative-metric principle is preserved; only its basis changes from physical buffer length to total-appended count).
- **The concurrency analysis (§3.6)** — post-handoff replay runs on the terminal's downstream-pump goroutine; the 28.1b race surface is empty; the 28.2 response decoder WILL create one (the correlation structures) — a LOAD-BEARING forward-pointer pinned here for ADR-0223/the 28.2 SPEC.
- **The ADR disposition (§6.1)** — the read seam folds into ADR-0221's body (the terminal-handoff conn-wrap seam, both directions); no new ADR number; a one-line in-place §AMEND on ADR-0221 §Context lands at THIS SPEC commit.
- **The ROADMAP rollup ratification (§6.3)** — 28.1b flips its own sub-row; the parent-row-28 rollup stays atomic with 28.2 (the 18/19/22/24/25/26 final-sub-phase precedent).
- **The sub-phase directory decision (§6.4)** — a fresh `28.1b-…/` directory (this one), per the 25.x/26.x per-sub-phase-directory precedent; the 28.1 directory remains the durable 28.1a record.

---

## 2. Non-purposes

- **2.1 No re-design of anything 28.1a landed** (§1.1). The WriteFilter seam, the zookeeperproxy package (beyond the §3.3 feed re-base), the builtins/bootstrap/name.go integration, and the 37th fuzzer are DONE.
- **2.2 Response decoding stays OUT OF SCOPE** (28.2 / ADR-0223). The zookeeperproxy `OnWrite` stays a pure no-op `Continue`. The correlation structures stay written-but-unread (R5).
- **2.3 No general post-handoff filter API.** The replay path is OBSERVATIONAL (§3.5): post-handoff `OnData` Status is ignored, post-handoff `Connection().Close` is not acted on, `ContinueReading` is meaningless. No production consumer needs any of these (zookeeperproxy always Continues, never closes, never drains — R3); supporting them is deferred under the ADR-0213/0221 API-revision allowance to the first consumer that needs them. They are documented framework boundaries (§6.2), not silent gaps.
- **2.4 No change to pre-handoff chain semantics.** The eager `OnNewConnection` pass, `connHalted`, `runData`'s park-on-StopIteration, `ContinueReading`, `CloseRequested`/`CloseType`, the upstream-cluster override, and `terminalReady` are all UNCHANGED (`chain.go:230-356`). Existing read filters (echo, direct_response, rbac_network, sni_cluster) observe zero behavioral delta — their chains never satisfy the wrap predicate (§3.4) so they never see the replay path at all.
- **2.5 No `manager.go` change.** `serveNetworkChain`'s read loop + handoff structure (`manager.go:1025-1091`) and `buildNetworkChainFactory`'s boot classification (`manager.go:534-599`) are untouched. The seam lives entirely inside `handleTerminal`'s conn composition — exactly like the 28.1a write seam. 28.1b's production diff to `internal/listener/` is ZERO files (the same §3.6 posture the 28.1 SPEC pinned).
- **2.6 No new fuzzer.** The 37th fuzzer (`FuzzZookeeperRequestDecode`) already covers the decoder the replay path feeds; the replay path itself is deterministic plumbing covered by unit + race + differential layers (§11). Fuzzer count STAYS 37.
- **2.7 No histograms, no dynamic metadata, no real-ZooKeeper-server fixtures, no per-route surface** — all per the 28.1 SPEC §2 / parent §2 (unchanged carried boundaries).
- **2.8 No new conformance harness.** h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected (the seam is network-chain-only; HCM's chain has zero write filters → never wrapped).

---

## 3. The read-side seam (extends 28.1 SPEC §3; lands under ADR-0221)

### 3.0 Root cause + PoC evidence (inherited facts)

- **Root cause** (PROGRESS.md Task 16 + 28.1a closure): `serveNetworkChain`'s read loop returns at `TerminalReady()` → `HandleTerminal()` (`manager.go:1066-1069`). For a `[zookeeper_proxy, tcp_proxy]` chain, `terminalReady()` (`chain.go:230-232`) becomes true at the end of the FIRST `OnData` pass (zookeeperproxy Continues → `resumeIdx` reaches `len(filters)`), so handoff happens after the first socket read and `zookeeper_proxy.OnData` never runs again.
- **PoC** (PROGRESS.md "Proof-of-cause"): a temporary `readChainConn` re-feeding every terminal-side socket read through the read filters achieved EXACT cross-side parity on all 10 `l_plain` counters (`connect_rq=2 ping_rq=1 getdata_rq=2 create_rq=1 close_rq=1 create2_rq=1 getchildren2_rq=1 setwatches2_rq=1 decoder_error=1 request_bytes=307`) and the fixture went GREEN. The proof files were fully reverted; this SPEC designs the production shape.

### 3.1 `readChainConn` (`readconn.go`, NEW) + conn composition

```go
// internal/filter/network/readconn.go (NEW) — read-side seam conn for terminal
// handoff (ADR-0221 §AMEND — the read-direction half of the terminal-handoff
// conn-wrap seam). Mirrors prefixconn.go / writeconn.go's embed-and-override-
// one-method shape.

// readChainConn re-feeds every post-handoff downstream socket read through the
// read-filter chain (rt.replayRead) BEFORE returning the bytes to the terminal,
// restoring upstream FilterManagerImpl::onRead re-iteration parity for chains
// that need it (the §3.4 predicate). All non-Read methods promote from the
// embedded net.Conn.
type readChainConn struct {
	net.Conn               // the RAW downstream conn (innermost wrap — §3.1 composition)
	rt       *chainRuntime // the per-connection runtime whose read filters get the replay
}

func newReadChainConn(c net.Conn, rt *chainRuntime) *readChainConn {
	return &readChainConn{Conn: c, rt: rt}
}

// Read reads from the wrapped conn, replays any received bytes through the
// read-filter chain (observational — §3.5), then returns them to the terminal.
// Replay-before-return makes stat increments visible BEFORE the bytes are
// forwarded upstream (deterministic ordering for the 0046 scrape — §5.1).
// io.EOF additionally delivers a final endStream replay (pre-handoff read-loop
// symmetry, manager.go:1073-1077).
func (r *readChainConn) Read(b []byte) (int, error) {
	n, err := r.Conn.Read(b)
	if n > 0 {
		r.rt.replayRead(b[:n], false)
	}
	if err != nil && errors.Is(err, io.EOF) {
		r.rt.replayRead(nil, true)
	}
	return n, err
}
```

**Composition (extends 28.1 SPEC §3.5 item 1).** `handleTerminal` (`chain.go:239-267`) wraps in this order, innermost → outermost:

1. **`readChainConn`** FIRST (innermost — wraps `rt.conn` directly, iff the §3.4 predicate holds). Innermost is load-bearing: the `prefixConn`'s buffered prefix (the pre-handoff bytes the read filters ALREADY saw) must NOT be re-fed — `prefixConn.Read` serves the prefix without delegating to its inner conn (`prefixconn.go:21-28`), so prefix bytes bypass the replay; only LIVE post-handoff socket reads pass through `readChainConn.Read`.
2. **`prefixConn`** SECOND (iff undrained buffered prefix — unchanged from 26.2).
3. **`writeChainConn`** THIRD (outermost, iff ≥1 write filter — unchanged from 28.1a).

The terminal therefore sees one of: `writeChainConn(prefixConn(readChainConn(conn)))`, `writeChainConn(readChainConn(conn))` (no prefix), `writeChainConn(prefixConn(conn))` / `writeChainConn(conn)` (impossible under the shared predicate — §3.4 makes read+write wraps install together), `prefixConn(conn)`, or `conn` (the unwrapped R1 shapes). Reads promote: terminal → writeChainConn (embedded) → prefixConn (prefix, then delegate) → readChainConn (live read + replay) → raw conn. Writes promote: terminal → writeChainConn (write chain) → prefixConn (embedded) → readChainConn (embedded) → raw conn.

### 3.2 The replay path — `chainRuntime.replayRead` (`chain.go` EXTEND)

```go
// replayRead re-iterates the read-filter chain over post-handoff downstream
// bytes (the read-side seam, ADR-0221 §AMEND). It restores upstream
// FilterManagerImpl::onRead parity for wrapped chains: every read filter's
// OnData runs, in chain order, on every socket read for the connection's
// lifetime. The replay is OBSERVATIONAL (§3.5): Status is ignored (the bytes
// are already committed to the terminal via readChainConn.Read's return), and
// the buffer is fully drained after the pass (bounded memory — the bytes'
// forward path is the terminal's conn, not the chain buffer).
//
// Called from readChainConn.Read on the terminal's downstream-pump goroutine
// (§3.6 concurrency pin) — never concurrently with the pre-handoff read loop
// (handoff is a happens-before edge: the loop returns before Handle spawns the
// pumps).
func (rt *chainRuntime) replayRead(p []byte, endStream bool) {
	rt.buf.Append(p)
	for _, f := range rt.filters {
		_ = f.OnData(rt.buf, endStream) // Status ignored — observational (§3.5)
	}
	rt.buf.Drain(rt.buf.Len())
}
```

Pinned semantics:

1. **All read filters, chain order.** Upstream re-iterates the whole read chain; so does the replay. (For 28.x's only wrapped production chain — `[zookeeper_proxy, tcp_proxy]` — the list is `[zookeeperproxy]`; the multi-read-filter case is unit-tested with synthetic filters.)
2. **The replay reuses `rt.buf`** (the same per-connection `Buffer` the pre-handoff path uses) so `Buffer.TotalAppended()` (§3.3) is continuous across the handoff boundary — this continuity is what makes the decoder-feed re-base seamless.
3. **Drain-after-pass.** The runtime drains `rt.buf` fully after each replay pass. The chain buffer's pre-handoff job (holding undrained bytes for the prefixConn handoff) is over; post-handoff it is purely an observation vehicle, and retaining bytes would grow memory unboundedly over a long-lived connection (the rejected D-28.1b-1 alternative (a), §8.1). Filters still NEVER drain (R3 unchanged — the RUNTIME drains, mirroring upstream where the terminal-most filter consumes the buffer).
4. **endStream replay on EOF** — one final `OnData(nil, true)` pass, mirroring the pre-handoff loop's EOF delivery (`manager.go:1073-1077`). zookeeperproxy ignores endStream; pinned for contract completeness.
5. **No `OnNewConnection` interaction.** Every read filter's `OnNewConnection` already ran pre-handoff (handoff requires `resumeIdx >= len(filters)`, which requires every filter to have Continued). The replay calls `OnData` only.

### 3.3 The decoder-feed re-base — `Buffer.TotalAppended` (D-28.1b-1; redesigns 28.1 SPEC §4.5's high-water-mark basis)

**The problem (the next-prompt §3 constraint).** The 28.1 SPEC §4.5 / as-built `requestDecoder.chainConsumed` (`decoder.go:34-39`) is a high-water mark over the PHYSICAL chain-buffer length: `decodeOnData(chainBytes []byte)` feeds `chainBytes[chainConsumed:]` and sets `chainConsumed = len(chainBytes)` (`decoder.go:69-73`). That contract assumes the buffer NEVER SHRINKS. Two events break that assumption once the read seam exists: (i) `handleTerminal` drains the buffer into the prefixConn at handoff (`chain.go:241-245`); (ii) the §3.2 replay drains it after each pass. A naive replay would feed a buffer whose length restarts below the stale mark — new bytes silently dropped, then mis-sliced. **The fix must be explicit, not accidental** (the high-water-mark mechanism is unit-proven at 28.1a and its no-double-count guarantee must survive).

**The design: re-base the mark from physical length onto a monotonic appended-bytes counter.**

```go
// internal/filter/network/buffer.go — ADDITION
//
// total is the monotonic count of bytes ever Appended to this Buffer. Unlike
// Len(), it is unaffected by Drain — it only grows. Filters that need to
// distinguish never-before-seen bytes from re-delivered bytes (the
// zookeeperproxy request decoder) track novelty against TotalAppended instead
// of Len, which makes their tracking immune to WHO drains the buffer and WHEN
// (the filter never drains — R3; the runtime drains at handoff and after each
// post-handoff replay pass).
func (b *Buffer) TotalAppended() int { return b.total }   // Append does b.total += len(p)
```

```go
// internal/filter/network/zookeeperproxy/decoder.go — the re-based feed
// (replaces decoder.go:69-84; the frames loop below it is UNCHANGED)

// decodeOnData feeds the current chain-buffer contents into the decoder.
// totalAppended is the buffer's monotonic Buffer.TotalAppended() value; the
// high-water mark (chainConsumed) is kept against IT, not against the physical
// buffer length, so the feed is correct regardless of runtime drains (the
// 28.1b read seam drains the buffer at handoff and after every post-handoff
// replay pass). The never-before-seen bytes are always the trailing
// (totalAppended − chainConsumed) bytes of chainBytes — bytes are only ever
// appended at the tail and only ever drained at the head, and the runtime
// never drains bytes the filters have not yet been shown.
func (d *requestDecoder) decodeOnData(chainBytes []byte, totalAppended int) {
	if newCount := totalAppended - d.chainConsumed; newCount > 0 {
		d.readBuf = append(d.readBuf, chainBytes[len(chainBytes)-newCount:]...)
		d.chainConsumed = totalAppended
	}
	for { /* nextFrame/decodeFrame loop — UNCHANGED (decoder.go:74-84) */ }
}
```

```go
// internal/filter/network/zookeeperproxy/zookeeperproxy.go — OnData (the only
// filter-glue change; signature UNCHANGED)
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}
```

**Equivalence argument (why this is NOT a behavior change on the proven path).** On any execution where the buffer is never drained between a filter's `OnData` calls — i.e. every pre-handoff execution, every existing unit test, and the entire 28.1a behavior envelope — `TotalAppended() == Len()` holds at every `OnData`, so the re-based feed selects byte-for-byte the same slice the 28.1a feed selects (`chainBytes[len−(total−mark):] == chainBytes[mark:]` when `total == len`). The 28.1a decoder unit tests update MECHANICALLY (pass `len(chainBytes)` as `totalAppended`); their assertions are unchanged. New tests cover the drain regimes (§11.1).

**Soundness invariant (pinned; unit-tested).** The re-base is sound iff the runtime never drains bytes a filter has not yet been shown. Both drain sites satisfy it by construction: (i) the handoff drain (`chain.go:241-245`) happens only after the final pre-handoff pass completed (every filter saw the buffer); (ii) the replay drain (§3.2) happens after the replay pass delivered the buffer to every read filter. The invariant is stated as a `Buffer`/chain contract comment + a chain unit test (a tracking filter asserts it sees every appended byte exactly once across both regimes).

**Why the mark stays on the decoder** (not moved to the filter or the runtime): D-S28.1-3 resolved it onto the decoder at 28.1a (PLAN-resolved, unit-proven); the re-base changes its BASIS, not its OWNER. Moving it would be churn with no benefit.

### 3.4 The wrap predicate (R1 — LOAD-BEARING)

**Predicate: install the `readChainConn` IFF `len(rt.writeFilters) > 0` — the EXACT predicate that installs the `writeChainConn`** (`chain.go:256`). The two seams wrap together: a chain gets both conn-wraps or neither.

```go
// chain.go handleTerminal — the read-wrap insertion (BEFORE the prefixConn wrap,
// so the readChainConn is innermost — §3.1 composition):
conn := rt.conn
if len(rt.writeFilters) > 0 {            // the SHARED seam predicate (R1)
	conn = newReadChainConn(conn, rt)
}
if rt.buf.Len() > 0 { conn = newPrefixConn(conn, prefix) }    // unchanged
if len(rt.writeFilters) > 0 { conn = newWriteChainConn(conn, dispatch) }  // unchanged
```

Pinned rationale + consequences:

1. **R1 preservation is structural.** Every existing production chain — `tcp_proxy`-only, HCM, `[echo]`, `[direct_response]`, `[rbac_network, tcp_proxy]`, `[sni_cluster, tcp_proxy]`, and all 47 existing fixture dirs — has ZERO write filters, so it gets NEITHER wrap and `handleTerminal` stays byte-identical to 28.1a (which was byte-identical to 26.2/27 for those chains). The 47-dir back-compat gate ratifies this (R1, §9).
2. **The predicate is equivalent, TODAY, to "≥1 read-decoding filter."** The only filter that needs post-handoff reads is a request-decoding filter, and every planned request-decoding filter (zookeeper 28.x; redis/mongo/kafka_broker/thrift — the §9 candidates) is a request/response protocol decoder, hence a both-directions filter, hence a `WriteFilter`. The next-prompt's "equivalently ≥1 read-decoding filter" equivalence holds for the entire planned roster. The SPEC pins `len(writeFilters) > 0` as the predicate because it is already computed, already classifies correctly, and requires no new interface.
3. **The future exception is named + deferred.** A hypothetical read-decoding filter that is NOT a write filter (a one-direction protocol sniffer) would need post-handoff reads but miss this predicate. No such filter exists or is planned. When one appears, the predicate generalizes via an opt-in marker interface (the rejected-as-premature D-28.1b-2 alternative (c), §8.1) under the ADR-0213/0221 API-revision allowance — the same deferral pattern as the write-only-filter boot boundary (28.1 SPEC §3.6).
4. **Hot-path cost when wrapped:** one extra interface indirection per Read + one `Buffer.Append` copy + one read-chain pass + one Drain per socket read — accepted for protocol-decoding chains (the decode work dominates); ZERO cost for unwrapped chains (the R1 population).

### 3.5 Observational boundaries (post-handoff filter-callback semantics)

The replay is OBSERVATIONAL. Three pre-handoff callback capabilities have no post-handoff effect; each is pinned as a documented framework boundary (BEHAVIOR_CONTRACT, §6.2), not silently dropped:

| Capability | Pre-handoff (unchanged) | Post-handoff (the replay) | Why acceptable |
|---|---|---|---|
| `OnData` → `StopIteration` | parks the pass at that filter (`chain.go:343-353`) | **IGNORED** — the bytes are already committed to the terminal (readChainConn returns them regardless) | Upstream divergence (upstream StopIteration blocks tcp_proxy from seeing the bytes). No production consumer: zookeeperproxy ALWAYS Continues (R3); future decoders are passthrough decoders too. Honoring it would require the seam to suppress the terminal's read — a flow-control surface deferred to the first consumer that needs it. |
| `Connection().Close(...)` | read loop exits + closes (`manager.go:1053-1054, 1070-1072`) | **NOT ACTED ON** — `closeReq` is recorded but nothing post-handoff checks it (the terminal owns the conn lifecycle) | No production consumer: zookeeperproxy never closes. A decoding filter that must kill a connection post-handoff (e.g. a future protocol-violation policy) needs a terminal-integration surface — deferred under the API-revision allowance. |
| `ContinueReading()` | resumes a parked chain (`chain.go:400-411`) | **MEANINGLESS** — no parked state exists post-handoff (Status is ignored) | Follows from row 1. |

### 3.6 Concurrency analysis + the 28.2 forward-pointer (LOAD-BEARING)

The chain runtime is single-goroutine pre-handoff (ADR-0213: the `serveNetworkChain` read-loop goroutine). The seam changes the post-handoff picture; this section pins it.

**Who runs what, post-handoff** (anchored on `tcp_proxy.Handle`, `internal/filter/tcpproxy/filter.go:134-138`):

- **Goroutine A** (downstream→upstream pump: `io.Copy(upstream, downstream)`): calls `readChainConn.Read` → `replayRead` → every read filter's `OnData` → the zookeeperproxy REQUEST decoder.
- **Goroutine B** (upstream→downstream pump: `io.Copy(downstream, upstream)`): calls `writeChainConn.Write` → every write filter's `OnWrite` → (at 28.2) the zookeeperproxy RESPONSE decoder.
- **The serveNetworkChain goroutine**: blocked inside `Handle` (`wg.Wait()`, `filter.go:138`) until both pumps exit; runs `OnDestroy` (the deferred `rtChain.OnDestroy()`, `manager.go:1034`) strictly AFTER both pumps join.

**Pinned 28.1b race posture:**

1. **Pre-handoff → post-handoff handoff is race-free**: the read loop returns before `Handle` is called; `Handle` spawning goroutines A/B is a happens-before edge for all chain-runtime + decoder state.
2. **Goroutine A vs goroutine B share NO mutable state at 28.1b**: A touches `rt.buf` + the request decoder (readBuf, chainConsumed, correlation maps) + counter increments (atomic by construction — `internal/stats`); B's `OnWrite` is a pure no-op (28.1 SPEC §4.7 pin) and `writeChainConn.Write` allocates a fresh per-Write `Buffer`. **The 28.1b race surface is empty.** Verified by `go test -race` plus a dedicated unit test driving a wrapped chain with concurrent live pumps (§11.1).
3. **Goroutine A vs `OnDestroy` is race-free**: OnDestroy runs after both pumps join (item above).
4. **THE 28.2 FORWARD-POINTER (must be carried into ADR-0223 / the 28.2 SPEC):** when 28.2's response decoder lands in `OnWrite`, goroutine B will READ + ERASE the correlation structures that goroutine A WRITES — a data race under the as-built lock-free design. **The 28.2 SPEC must add synchronization** (the anticipated shape: a per-connection `sync.Mutex` on the decoder guarding the correlation maps + any shared latency state; the per-direction reassembly buffers stay unshared/lock-free). This is a direct consequence of the read seam's goroutine placement and is pinned HERE so 28.2 cannot miss it. (Upstream has no such race only because libevent serializes both directions onto one dispatcher thread.)

### 3.7 File split (lands at IMPL)

| File | Change | Responsibility |
|---|---|---|
| `internal/filter/network/readconn.go` | **NEW** | the `readChainConn` (§3.1) |
| `internal/filter/network/readconn_test.go` | **NEW** | read-passthrough / replay-delivery / prefix-not-re-fed / EOF-endStream / error-propagation unit tests |
| `internal/filter/network/buffer.go` | EXTEND | `TotalAppended()` + the `total` field (§3.3) |
| `internal/filter/network/buffer_test.go` | EXTEND | TotalAppended monotonicity under Append/Drain |
| `internal/filter/network/chain.go` | EXTEND | `replayRead` (§3.2) + the `handleTerminal` read-wrap insertion (§3.4) |
| `internal/filter/network/chain_test.go` | EXTEND | wrap-predicate (R1 zero-write-filter ⇒ no wraps) / composition order / replay-to-all-filters / drain-after-pass / the §3.3 soundness invariant / the §3.6 race test |
| `internal/filter/network/zookeeperproxy/decoder.go` | MODIFY | `decodeOnData(chainBytes, totalAppended)` re-base (§3.3) |
| `internal/filter/network/zookeeperproxy/zookeeperproxy.go` | MODIFY | `OnData` passes `buf.TotalAppended()` (§3.3) |
| `internal/filter/network/zookeeperproxy/decoder_test.go` | UPDATE | mechanical signature updates + the new drain-regime tests |
| `types.go`, `terminal.go`, `callbacks.go`, `registry.go`, `prefixconn.go`, `writeconn.go`, `upstreamcluster.go` | UNCHANGED | — |
| `internal/listener/manager.go`, `internal/filter/tcpproxy/`, HCM | **UNCHANGED** | §2.5 |

---

## 4. The `zookeeperproxy` package delta

The ONLY package changes are the two §3.3 lines (the `decodeOnData` signature + `OnData` passing `TotalAppended`) and their tests. Config parse, the PARSE-REJECT arms, the 201-counter roster, the dynamic auth counters, the min-length table, the correlation structures, `OnWrite`, `OnNewConnection`, `OnDestroy`, and the factory are all UNTOUCHED (28.1 SPEC §4 stays authoritative as-built).

---

## 5. The proof surface — fixtures (re-scoped from 28.1 SPEC §8)

### 5.1 `0046-zookeeper-requests` — RE-ENABLE + GREEN + R4

The driver (881 LoC, 7 arms + `AssertStats`, complete and correct per PROGRESS.md Task 16) is committed-but-DISABLED. 28.1b:

1. **Re-enable**: uncomment the blank-import (`test/differential/runner_test.go:72-77`) + restore the import group format (`gofmt`/`goimports`); drop the driver's "DISABLED at 28.1a" doc banner (`driver.go:5-24`), replacing it with a one-paragraph "re-enabled at 28.1b (the read seam)" note that cites this SPEC.
2. **Green**: all 7 arms pass cross-side. The Task-16 divergence table is the expected delta: with the seam, the subject matches the reference on all 10 `l_plain` counters (the PoC already demonstrated exactly this — §3.0). Arm-by-arm seam dependency: arm 1 (single-frame connect) was already green; arm 2 (multi-opcode sequence) + arm 3 (digit-suffixed opcodes) + arm 4's recovery frame REQUIRE the seam (frames 2..n of their connections arrive post-handoff); arm 5 (single-frame, `l_flags`) was already green; arm 6 (exists-at-zero) was already green; arm 7 is the R4 protocol below.
3. **R4 deliberate-break (deferred from Task 16 — requires the green baseline that now exists)**: (a) temporarily assert `getdata_rq == 3` (when 2 is driven) → MUST fail; (b) temporarily disable the `.zookeeper.` name.go arm → arm 6's lookup MUST miss → fail. Both breaks reverted; protocol + outputs recorded in driver comments + the fixture README + PROGRESS.md (the `reference_differential_asserter_dispatch` liveness discipline).
4. **Author the fixture README** (deferred from Task 16 so it documents the as-shipped green result): topology, the TCPSink rationale, the 7 arms, the seam dependency, the R4 record.

### 5.2 `0047-zookeeper-boot-reject` — lands per the 28.1 SPEC §8.2 design, VERBATIM

No design delta: a `[zookeeper_proxy, tcp_proxy]` chain whose zookeeper `typed_config` has no `stat_prefix` → BOTH sides reject at boot; driver implements `fixture.Driver` + `differential.BootRejectFixture` (`harness.go:340-352`), `ExpectedBootErrorSubstring() = "stat_prefix"`; symmetric mode; a minimal unused cluster satisfies the zero-cluster boot reject; the `0044-network-rbac-boot-reject` driver (220 LoC) is the template. (The 28.1a PARSE-REJECT arm + wording `zookeeper_proxy: stat_prefix is required` already exist and are unit-proven — this fixture is its differential proof.)

### 5.3 Counts

Differential dirs: 47 active + 1 disabled → **49 active** (`0046` re-enabled + `0047` new); tail `0047-zookeeper-boot-reject`. Fuzzers: **37** (unchanged — §2.6). The FULL 47-dir pre-existing suite is the R1 back-compat gate and re-runs green at the six-gate.

---

## 6. The 28.1 completion bundle

### 6.1 ADR disposition — NO new ADR number (D-28.1b-3)

- **ADR-0221** (`DECISIONS.md:14228`): the §Decision/§Consequences body lands at the 28.1b IMPL completion task, covering **BOTH halves of the terminal-handoff conn-wrap seam**: the 28.1a write side (interfaces, classification, reverse dispatch, `writeChainConn`, D-P7, the §3.6/§3.7 boundaries of the 28.1 SPEC) AND the 28.1b read side (this SPEC §3: `readChainConn`, `replayRead`, the TotalAppended re-base, the shared wrap predicate, the observational boundaries, the concurrency pins + 28.2 forward-pointer). **A one-line in-place §AMEND on ADR-0221's §Context lands AT THIS SPEC COMMIT** recording the scope extension ("the seam is the terminal-handoff conn-wrap seam, both directions; the read-direction half was forced by the Task-16 0046 discovery and designed at the 28.1b SPEC") — the same in-place-§AMEND mechanism as ADR-0045's split record. Rationale for folding rather than minting ADR-0224: the read seam is the symmetric completion of the SAME structural extension (same predicate, same conn-wrap mechanism, same consumer, same API-revision allowance); STATE.md's 28.1a closure already pinned "no further phase-28 ADR numbers anticipated" with full knowledge of the 28.1b read seam.
- **ADR-0222** (`DECISIONS.md:14247`): the §Decision/§Consequences body lands at the 28.1b IMPL completion task per its existing §Context draft (the request package — unchanged scope), plus the §3.3 feed re-base note.
- **DECISIONS.md tail STAYS ADR-0223; next-free STAYS ADR-0224.** ADR-0223's body lands at 28.2 (and must carry the §3.6 synchronization forward-pointer).

### 6.2 BEHAVIOR_CONTRACT 28.1 bundle (ONE atomic landing per ADR-0052, at the 28.1b IMPL completion task)

Extends the 28.1 SPEC §9 enumeration with the read-seam additions:

- The `### Network filter chain framework — terminal-handoff conn-wrap seam (28.1 amendment)` block: BOTH directions — the 28.1 SPEC §9 write-side items PLUS the read-side replay semantics (§3.2), the shared wrap predicate + R1 (§3.4), the three observational boundaries (§3.5), and the goroutine-placement note (§3.6).
- The `### envoy.filters.network.zookeeper_proxy` subsection per the 28.1 SPEC §9 (request-side semantics, the 201-counter roster + creation parity, the `<stat_prefix>.zookeeper.` scope, the Prometheus flattening, the dynamic auth counters, the shallow-decode leniency departure, the dynamic-metadata coverage boundary, the `access_log` note, the parsed-not-consumed latency note) — now writable as cross-side-PROVEN facts (0046 green).
- **Stat table: 136 → 337** (the 201 zookeeper rows). The roll happens HERE (not at 28.1a) because the BEHAVIOR_CONTRACT records cross-side-PROVEN surface and the proof is `0046` (the deliberate 28.1a deferral, PROGRESS.md "Counts at 28.1a").
- The 28.2 forward-pointer note (response decoder + correlation consumption + latency counters + the synchronization obligation + the parent-row rollup).

### 6.3 ROADMAP advance + the rollup decision (D-28.1b-4 — RATIFIED: default)

At the 28.1b IMPL phase-done commit: sub-row **28.1b `in-progress → done`**; parent row **28 STAYS `in-progress`**; sub-row 28.2 STAYS `planned`. The parent-row-28 ROLLUP (parent → `done`) happens at **28.2 phase-done**, per the 18/19/22/24/25/26 precedent (the parent rolls at the FINAL sub-phase) — 28.2 is the final sub-phase and ADR-0223 + the response surface + the latency counters are its scope. The alternative (28.1b performs an early partial rollup) is REJECTED: it would leave no atomic close for the phase-28 family and contradicts every prior family's precedent.

### 6.4 Sub-phase directory decision (RATIFIED: fresh directory)

28.1b artifacts live in `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/` (this directory; created at this SPEC commit with README + SPEC.md; PLAN/PROGRESS/REVIEW land here at their lifecycle states). The 28.1 directory is the durable, unmodified record of 28.1a (per the 25.x/26.x per-sub-phase-directory precedent; STATE.md named this the expected default). The 28.1 directory's PROGRESS.md is NOT extended further.

---

## 7. SPEC-time empirical pins

### 7.1 D-S28.1b-0 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip **`4bc9790`** (the docs-only next-prompt repoint trailing the 28.1a-IMPL squash `8703aeb` by +2) at this SPEC session. The 28.1b IMPL Task-1 first-action gate RE-RUNS these against the live IMPL-session tip.

**Counts:**

- Differential fixture dirs: **48** on disk (`0000`–`0046`; tail `0046-zookeeper-requests`), of which **47 active** + `0046` committed-but-DISABLED (blank-import commented at `runner_test.go:72-77`). 28.1b → **49 active** (+ `0047`).
- Fuzzers: **37** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`; the raw repo-wide count is 38 incl. one helper-adjacent fuzzer — PROGRESS.md "Counts at 28.1a"). 28.1b: unchanged.
- Stat surface (BEHAVIOR_CONTRACT table): **136** (`BEHAVIOR_CONTRACT.md:462` — the 26.3 roll). 28.1b → **337** (§6.2).
- DECISIONS.md tail: **ADR-0223** (§Context drafts at `:14228`/`:14247`/`:14266`); next-free **ADR-0224**; the ADR-0045 §AMEND split record at `:1526`.
- Conformance: h2spec 53/53; proxy-wasm 10/10 (re-run live at the 28.1a closure).

**As-built anchors (the §3 design extends/modifies these exact sites):**

- `internal/listener/manager.go:1025-1091` — `serveNetworkChain` (the read loop; the handoff-return at `:1066-1069`; the EOF delivery at `:1073-1077`); `:534-599` — `buildNetworkChainFactory` (untouched).
- `internal/filter/network/chain.go:230-232` — `terminalReady`; `:239-267` — `handleTerminal` (the prefix drain at `:241-245`; the write-wrap at `:256-262`; the wrap-insertion site §3.4 extends); `:303-310` — `onData`; `:323-356` — `runData`; `:366-381` — `onDestroy`; `:146-192` — the `chainRuntime` struct (`filters`/`writeFilters`/`buf` fields).
- `internal/filter/network/buffer.go:9-31` — `Buffer` (`Append`/`Bytes`/`Len`/`Drain`; the §3.3 extension site).
- `internal/filter/network/writeconn.go:13-48` — `writeChainConn` (the symmetric precedent); `prefixconn.go:12-28` — `prefixConn` (the composition partner).
- `internal/filter/network/zookeeperproxy/decoder.go:30-53` — `requestDecoder` (the `chainConsumed` mark at `:34-39`); `:69-84` — `decodeOnData` (the §3.3 modification site); `:90-106` — `nextFrame` (unchanged).
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go:65-68` — `OnData` (the §3.3 modification site); `:76` — the no-op `OnWrite`; `:48-54` — the both-directions `filter` struct.
- `internal/filter/tcpproxy/filter.go:101-139` — `Handle` (the §3.6 goroutine anchors: pump A at `:136`, pump B at `:137`, `wg.Wait()` at `:138`).
- `test/differential/runner_test.go:72-77` — the commented `0046` blank-import; `:832` + `:1263-1269` — the `TCPSink` arm + `acceptSinkCounting`; `:~1069-1070` — the cross-side-path StatsAsserter dispatch.
- `test/differential/fixture/fixture.go:70-77` — `StatsAsserter`; `:493-502` — `TCPSink BackendKind = 28`; `:505-508` — `BackendKindAware`; `test/differential/harness.go:312-352` — `BootRejectFixture`.
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` — 881 LoC; the DISABLED banner at `:5-24`; `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` — the `0047` template (220 LoC).

### 7.2 The divergence + PoC evidence (inherited from PROGRESS.md Task 16 — the design's empirical ground truth)

- Reference (every frame decoded): `connect_rq=2 ping_rq=1 getdata_rq=2 create_rq=1 close_rq=1 create2_rq=1 getchildren2_rq=1 setwatches2_rq=1 decoder_error=1 request_bytes=307`.
- Subject at 28.1a (first-read-only): `connect_rq=2 ping_rq=0 getdata_rq=0 create_rq=0 close_rq=0 create2_rq=1 getchildren2_rq=0 setwatches2_rq=0 decoder_error=1 request_bytes=132`.
- Subject WITH the PoC readChainConn: ALL TEN equal to the reference; fixture GREEN.
- `l_flags` (single-frame arm 5): perfect parity WITHOUT the seam — confirming the gap is exclusively the multi-socket-read case.

### 7.3 The 28.1a empirical pins (inherited, unchanged)

The five 28.1a-session pins carry forward and constrain nothing new at 28.1b (recorded for the PLAN's awareness): the dynamic auth counter shape `<stat_prefix>.zookeeper.auth.<scheme>_rq`; the `auth.unknown_scheme_rq` collapse for non-builtin schemes; the real ZK auth frame layout (floor 20 bytes); SetAuth-as-DATA-request = upstream decode error; the upstream `setauth_*` family never incremented.

---

## 8. SPEC-time D-questions

### 8.1 Resolved at this SPEC

- **D-28.1b-1 (the decoder-feed regime) — RESOLVED: re-base the high-water mark onto `Buffer.TotalAppended` + runtime drain-after-replay (§3.3).** Alternatives rejected: **(a) cumulative continuation** — keep `rt.buf` undrained for the connection's lifetime (the PoC's shape: zero decoder change) — REJECTED because the chain buffer would grow unboundedly on long-lived connections (a ZooKeeper session is long-lived by design; retaining every request byte forever is a real production flaw, and contradicts the project's bounded-memory discipline — cf. the 37th fuzzer's bounded-reassembly-buffer assertion); **(c) an opt-in post-handoff delta interface** (e.g. `OnPostHandoffRead([]byte)`) — REJECTED because it bifurcates the read API into two paths every future decoder must implement correctly (forgetting the second path silently reproduces the 28.1a bug as a permanent API trap), and it has no upstream analogue (upstream has ONE `onData`).
- **D-28.1b-2 (the wrap predicate) — RESOLVED: `len(rt.writeFilters) > 0`, shared with the write seam (§3.4).** Alternatives rejected: **(b) wrap every chain with ≥1 read filter** (upstream-maximalist parity) — REJECTED: it changes the post-handoff byte path for the existing `[rbac_network, tcp_proxy]` / `[sni_cluster, tcp_proxy]` production chains (R1 risk) for zero benefit (neither filter decodes post-handoff data); **(c) an opt-in marker interface** — DEFERRED as the documented future generalization (§3.4 item 3), not needed by any existing or planned filter.
- **D-28.1b-3 (ADR disposition) — RESOLVED: no new ADR number; the read seam folds into ADR-0221's body; §Context §AMEND at this SPEC commit (§6.1).**
- **D-28.1b-4 (ROADMAP rollup) — RESOLVED: 28.1b flips only its own sub-row; the parent rollup is 28.2's (§6.3).**
- **D-28.1b-5 (replay recipients) — RESOLVED: ALL read filters in chain order (§3.2 item 1)** — upstream re-iteration parity; the alternative (replay only to write-filter-implementing read filters) saves nothing today (the wrapped chain's only read filter IS the decoder) and diverges from upstream for no reason.

### 8.2 Open for PLAN / IMPL resolution

- **D-S28.1b-1 (Buffer.total field representation).** `int` vs `int64` for the monotonic counter (a very-long-lived connection could exceed 2^31 bytes on 32-bit platforms). **Resolution at:** IMPL (anticipated: `int64`, with `TotalAppended() int64` and the decoder mark widened to match — costless).
- **D-S28.1b-2 (the race-test shape).** The §3.6 item-2 dedicated race test: a synthetic wrapped chain under concurrent pump goroutines, or re-use of the 0046 fixture under `-race`. **Resolution at:** PLAN (anticipated: a `chain_test.go` unit test with synthetic filters + `net.Pipe`, so the race gate does not depend on docker).
- **D-S28.1b-3 (0046 driver banner replacement wording).** **Resolution at:** IMPL (the §5.1 item-1 one-paragraph note).

---

## 9. RATIFIED-PENDING items (re-scoped from 28.1 SPEC §13)

- **R1 (seam back-compat — now covers BOTH wraps).** Chains with zero write filters get NEITHER conn-wrap → `handleTerminal` byte-identical to 26.2/27. Ratified by ALL 47 pre-existing fixture dirs staying byte-exact green at the 28.1b six-gate.
- **R3 (passthrough invariant — unchanged owner, new clause).** zookeeperproxy never drains/mutates the chain buffer, never closes, never StopIterations — now additionally: the RUNTIME's replay drain (§3.2) is not a violation of R3 (R3 binds filters, not the runtime). Ratified by the 0046 arm-4 survival proof + the §3.3 soundness-invariant unit test.
- **R4 (StatsAsserter liveness — DEFERRED FROM 28.1a, lands here).** The §5.1 item-3 deliberate-break protocol, executed on the green baseline and recorded.
- **R5 (correlation hand-off — unchanged).** Still written-but-unread; 28.2 ratifies.
- **R6 (counts).** IMPL Task 1 re-pins: fixtures 47-active/48-on-disk → 49 active; fuzzers 37 → 37; stats 136 → 337; DECISIONS tail ADR-0223 / next-free ADR-0224 (both UNCHANGED at 28.1b close).
- **R7 (Prometheus parity — unchanged mechanics).** Ratified intrinsically by the 0046 both-sides-prometheus scrape going green.
- **R8 (NEW — re-iteration parity).** Every frame of every connection is decoded, regardless of how many socket reads deliver it. Ratified by the 0046 multi-frame arms (2/3/4) going green cross-side — these arms are exactly the re-iteration proof.

---

## 10. Per-task structure (~10 tasks; the SPEC-anticipated task spine)

The 28.1b PLAN authors the exact bite-sized TDD tasks (it may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate (§7.1 recipes re-run against the live IMPL tip) | §7.1 / R6 |
| 2 | `Buffer.TotalAppended` + monotonicity unit tests | §3.3 |
| 3 | Decoder feed re-base (`decodeOnData` signature + mark re-base + `OnData` pass-through) + mechanical test updates + new drain-regime tests | §3.3 / §4 |
| 4 | `readconn.go` `readChainConn` + unit tests (passthrough / replay / prefix-not-re-fed / EOF / errors) | §3.1 |
| 5 | `chain.go` `replayRead` + `handleTerminal` read-wrap insertion + composition/predicate/back-compat unit tests + the soundness-invariant test | §3.2 / §3.4 |
| 6 | The §3.6 concurrency race test (synthetic wrapped chain under live concurrent pumps, `-race`) | §3.6 |
| 7 | `0046` re-enable + cross-side GREEN + R4 deliberate-break + fixture README | §5.1 / R4 / R8 |
| 8 | `0047-zookeeper-boot-reject` fixture | §5.2 |
| 9 | Completion bundle: ADR-0221 (both-seams body) + ADR-0222 bodies in place + BEHAVIOR_CONTRACT 28.1 bundle (incl. 136 → 337) | §6.1 / §6.2 |
| 10 | Six-gate (incl. the FULL 49-dir differential suite + the 47-dir R1 back-compat gate) + STATE.md + ROADMAP sub-row 28.1b → done + next-prompt.txt for the 28.2-SPEC cold-start | §6.3 / §11.2 |

### 10.1 ADR-0045 split-gate check

Production-LoC estimate (the 26.x accounting basis — production code; fixture drivers + tests excluded): `readconn.go` ~35; `buffer.go` +~8; `chain.go` +~30; decoder/filter re-base ~10; **total ~80–100 production LoC**, ~10 tasks. Trivially within the ~1500-LoC / ~25-task gate — **no further split**. (The `0047` driver ~220 LoC and the test surface are excluded per the established accounting.)

---

## 11. Test surface + acceptance checklist

### 11.1 Test surface

- **Layer A — framework unit tests** (`internal/filter/network/`): `Buffer.TotalAppended` monotonicity under Append/Drain; `readChainConn` (live-read replay / prefix bytes NOT re-fed / EOF endStream replay / read-error propagation / zero-byte reads not replayed); `replayRead` (all-filters chain-order delivery / Status ignored — incl. a LIVE assertion that a mid-chain synthetic StopIteration does NOT halt delivery to later filters / drain-after-pass / endStream); `handleTerminal` (the shared wrap predicate; all composition shapes; **zero-write-filter chains produce NEITHER wrap — the R1 test**); the §3.3 soundness invariant (a tracking filter sees every appended byte exactly once across pre-handoff + post-handoff regimes); the §3.6 race test (`-race`, synthetic chain, concurrent pumps over `net.Pipe`).
- **Layer A — zookeeperproxy unit tests**: the mechanical `decodeOnData` signature updates (assertions unchanged — the §3.3 equivalence); NEW drain-regime tests (feed → drain → feed: no double-count, no drop; the handoff-boundary sequence: cumulative feed → drain → delta feeds); the existing multi-read/partial-frame/garbage tests re-pass.
- **Layer D — differential**: `0046` (7 arms, cross-side, R4) + `0047` (boot-reject) + the FULL 47-dir back-compat suite (R1) → 49/49 green.
- **Layer E — race**: `go test -race -short ./internal/filter/network/...` (now including the post-handoff replay path).
- **Per-task hygiene**: `gofmt -l` + `golangci-lint run` on touched packages, every task (`feedback_pertask_gofmt_lint`).

### 11.2 Six-gate checklist (the 22/24/25/26/27/28.1a precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (**49 dirs** incl. the 47-dir R1 back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected; re-run live if the harness is available). All outputs quoted into PROGRESS.md honestly (incl. any `freeTCPPort` TOCTOU flakes, re-run in isolation per the 28.1a-closure precedent).

### 11.3 28.1b IMPL acceptance checklist

1. The read-side seam lands per §3 (readconn.go + replayRead + TotalAppended re-base + the shared wrap predicate); `manager.go`/`tcp_proxy`/HCM untouched; all 47 pre-existing fixtures byte-exact green (R1).
2. `0046-zookeeper-requests` re-enabled + GREEN on all 7 arms + the R4 deliberate-break recorded + the fixture README authored (R4/R8).
3. `0047-zookeeper-boot-reject` lands and is green; counts: 49 active fixture dirs, 37 fuzzers, stat table 337 (R6).
4. ADR-0221 (both-seams body) + ADR-0222 §Decision/§Consequences bodies in place; DECISIONS.md tail STAYS ADR-0223 (no new number); the BEHAVIOR_CONTRACT 28.1 bundle lands (§6.2).
5. Six gates green (§11.2); STATE.md advanced; ROADMAP sub-row 28.1b `in-progress → done`; parent row 28 STAYS `in-progress`; next-prompt.txt rewritten for the 28.2-SPEC cold-start.

---

## 12. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 28.1b flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x precedent); parent row 28 STAYS `in-progress`; 28.2 STAYS `planned`. The ADR-0221 §Context one-line §AMEND (§6.1) lands at this SPEC commit. STATE.md advances to lifecycle-state 2-for-28.1b with `next-skill = superpowers:writing-plans` scoped to the **28.1b PLAN** (`docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 28.1b-PLAN cold-start. Per `feedback_execution_style` the 28.1b IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies.

---

## Appendix A — Cross-references

| 28.1b SPEC § | Master section | Relationship |
|---|---|---|
| §1 Purpose | ADR-0045 §AMEND + STATE.md + 28.1 SPEC §1 | refines (the split's second half) |
| §2 Non-purposes | 28.1 SPEC §2 + parent §2 | inherits + adds §2.3 (no post-handoff filter API) |
| §3 Read-side seam | 28.1 SPEC §3 (the write seam) | EXTENDS (the symmetric half; NEW design) |
| §3.3 Feed re-base | 28.1 SPEC §4.5 + D-S28.1-3 | REDESIGNS the mark's basis (explicitly, per the split mandate) |
| §4 zookeeperproxy delta | 28.1 SPEC §4 | inherits as-built; modifies only the feed |
| §5 Fixtures | 28.1 SPEC §8 | re-scopes the landing onto 28.1b; 0047 inherited verbatim |
| §6 Completion bundle | 28.1 SPEC §9 + §16 | re-scopes onto 28.1b; adds the read-seam block + the ADR/rollup/directory decisions |
| §7 Empirical pins | 28.1 SPEC §11 + PROGRESS.md Task 16 | re-pins against tip `4bc9790`; inherits the divergence/PoC evidence |
| §9 RATIFIED-PENDING | 28.1 SPEC §13 (R1–R7) | re-scopes; adds R8 |
| §10 Task spine | 28.1 SPEC §10 | NEW (28.1b's own spine; split-gate check) |
