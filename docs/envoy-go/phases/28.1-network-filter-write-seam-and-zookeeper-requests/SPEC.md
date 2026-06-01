# Phase 28.1 SPEC — the `network.WriteFilter` seam + `zookeeper_proxy` request side

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 28.1** (`network-filter-write-seam-and-zookeeper-requests`), the FIRST sub-phase of the phase-28 BRAINSTORM-time 2-way pre-split (28.1 / 28.2). It is authored per the phase-22.1 / phase-25.1 / phase-26.1 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/28-network-filter-zookeeper-proxy/SPEC.md`) already resolved the BRAINSTORM §10 D28-1..D28-10 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (parent §11), formalized the 2-way split surface-mapping (parent §3), and anchored the ADR-0221 + ADR-0222 + ADR-0223 §Context drafts in DECISIONS.md. This 28.1 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §13 RATIFIED-PENDING items, and **refines per-Task-level surface only**. It does NOT re-execute the empirical pins (no re-scrape; only the as-built line anchors + count baselines are re-pinned at §11.1 against the live session tip). The next session, per BOOTSTRAP §5, authors the **28.1 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Land the `network.WriteFilter` seam (the framework's write-direction dispatch half, deferred at 26.1 with an explicit API-revision allowance — ADR-0221) and the `internal/filter/network/zookeeperproxy/` package's REQUEST side (TypeURL + 9-field config parse + the 201-counter eager roster + the shallow request decoder + the two xid correlation structures + dynamic auth counters — ADR-0222), wired as the 7th built-in with the `.zookeeper.` Prometheus inline-prefix arm, proven by fixtures `0046-zookeeper-requests` (cross-side `StatsAsserter`) + `0047-zookeeper-boot-reject` and the 37th fuzzer.

**Architecture:** The existing `internal/filter/network/` package gains the `WriteFilter`/`WriteFilterCallbacks` interfaces, chain classification extended to read / write / both / terminal, REVERSE-chain-order write dispatch, and a `writeChainConn` that wraps the downstream `net.Conn` handed to `handleTerminal` (zero-write-filter chains UNWRAPPED → byte-identical back-compat; `TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM/`manager.go` untouched). A NEW `internal/filter/network/zookeeperproxy/` package implements BOTH `ReadFilter` and `WriteFilter` (one instance, both directions): config parse, the 201-counter EAGER roster under `<stat_prefix>.zookeeper.`, and the shallow request decoder; its 28.1 `OnWrite` is a no-op `Continue` stub (the response decoder is 28.2). Cross-side `StatsAsserter` per-opcode counter parity is the load-bearing differential proof.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/stats/` (06.1; `NewCounterIfAbsent`); `internal/filter/network/` (26.1/26.2/27); the differential harness + `fixture.StatsAsserter`. ZERO new third-party `go.mod` dependencies (the jute decode is plain `encoding/binary` big-endian reads).

**Authored:** 2026-06-01. **Empirical-pin probe date (inherited):** 2026-06-01 (parent SPEC §11). **Baseline-anchor re-pin date:** 2026-06-01 (this SPEC session, master tip `2a525ff` — §11.1).

---

## 1. Purpose / Mission

Phase 28.1 delivers two structurally coupled things (parent §3.2 item "28.1"):

1. **The `network.WriteFilter` seam (ADR-0221)** — the framework's write-direction (upstream→downstream) dispatch half, absent since 26.1 (read-filter-only scope per the 26 BRAINSTORM Q4), deferred WITH an explicit API-revision allowance written into ADR-0213 §Decision item 8 exactly for this moment. 28.1 CONSUMES that allowance: `zookeeper_proxy` is consumer #1; `mongo_proxy` is the anticipated consumer #2. The seam is the framework's THIRD structural extension (after the `TerminalFilter` seam at 26.2 / ADR-0215 and the override seam at 27 / ADR-0219) and follows the ADR-0219 no-ripple discipline: state threads via wrappers/ctx, never signature churn.

2. **The `zookeeper_proxy` request side (ADR-0222)** — the `internal/filter/network/zookeeperproxy/` package: TypeURL + 9-field config parse, the FULL 201-counter EAGER roster creation (creation parity per parent AMEND-A2 — response-side counters exist-at-zero until 28.2), the SHALLOW request decoder (framing + xid sniffing + opcode dispatch + min-length validation + decoder-internal reassembly + unconditional passthrough), the two xid→opcode correlation structures (written at 28.1, consumed at 28.2), per-opcode `*_rq` + `request_bytes` + `decoder_error` + flag-gated `*_rq_bytes`/`*_decoder_error` increments, and the dynamic per-scheme `auth.<scheme>_rq` counters.

Plus the integration surface: (3) registration as the **7th `builtins.RegisterBuiltins` built-in** + the `zookeeper_proxy/v3` blank-import in `internal/bootstrap/bootstrap.go`; (4) the **`.zookeeper.` INLINE-PREFIX arm** in `internal/stats/name.go` (parent AMEND-A4; the ADR-0138 shape, NOT a tag-extractor); (5) fixtures **`0046-zookeeper-requests`** (cross-side `StatsAsserter`) + **`0047-zookeeper-boot-reject`**; (6) the **37th fuzzer** `FuzzZookeeperRequestDecode`; (7) the ADR-0221 + ADR-0222 §Decision/§Consequences bodies, the BEHAVIOR_CONTRACT 28.1 bundle, and the STATE/ROADMAP advance (sub-row 28.1 `in-progress → done` at IMPL phase-done; parent row 28 STAYS `in-progress` — the ROLLUP is 28.2's).

After phase 28.1, the project has: a complete both-direction L4 chain framework (read + write + terminal); a request-side-observable `zookeeper_proxy` with live cross-side stat parity (337-counter project surface, the largest single-filter addition); and the correlation structures + parsed-but-unconsumed latency config that 28.2's response decoder consumes. 28.2 then completes the round-trip (response decoder + correlation + latency-threshold counters + the parent-row-28 ROLLUP).

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 11 AMENDs (A1–A11) in the parent SPEC §1.1 are the empirical-finding-driven scope revisions for phase 28. The amendments load-bearing for **28.1**:

- **AMEND-A1** (stat scope is `<stat_prefix>.zookeeper.<counter>` — prefix FIRST) — informs §4.4 roster creation + §7.1 + the §8.1 StatsAsserter expectations.
- **AMEND-A2** (201 EAGER macro counters; flags gate INCREMENTS never creation; surface 136 → 337) — informs §4.4 + §7.2/§7.3 + the §8.1 exists-at-zero arm.
- **AMEND-A3** (roster asymmetries: `connect_readonly_rq`/`_rq_bytes` rq-side only; NO `auth_rq` — lazy per-scheme `auth.<scheme>_rq` dynamic counters; `auth_resp*` ARE in the macro) — informs §4.4 + §4.5 (the connect-readonly + auth parse paths).
- **AMEND-A4** (Prometheus exposition is FLAT → the name.go arm is INLINE-PREFIX per ADR-0138, not a tag-extractor) — informs §7.4 + the D-P8 resolution (§12).
- **AMEND-A5** (connect distinguished by xid SNIFFING; special-xid set 0/−1/−2/−4/−8) — informs §4.5 dispatch.
- **AMEND-A6** (wire `OpCodes` enum: 26 values, gaps, `Close=−11` ≠ the proto's 27-value contiguous enum → mapping table) — informs §4.3 (the proto→wire mapping lands at 28.1 config parse).
- **AMEND-A7** (TWO correlation structures: data map + control FIFO queues) — informs §4.6.
- **AMEND-A8** (decoder-internal reassembly buffers; passthrough unconditional; never closes; never drains the chain buffer) — informs §4.5 + §4.7 + the R3 ratification.
- **AMEND-A9** (dynamic metadata DEFERRED → the decoder is SHALLOW; coverage-boundary record at 28.1) — informs §2.2 + §9.
- **AMEND-A10** (latency PGV: override `threshold` required + gte 1ms; `opcode` defined_only; `default_latency_threshold` gte 1ms when set; duplicate override → boot-reject) — informs §4.3 (the parse/validate code lands at 28.1; the fixture/consumption landing is 28.2's — parent D-P4).
- **AMEND-A11** (write path REVERSE/LIFO; StopIteration = no-forward, NO resume; writes enter at `ConnectionImpl::write` BEFORE the transport; zookeeper registers `addFilter` combined; its `onWrite` always `Continue`) — informs §3 (the entire seam design).

### 1.2 28.1-SPEC-additive contributions (what this document pins beyond the parent)

This 28.1 SPEC makes NO substantive scope revision vs the parent SPEC. Its ADDITIVE contributions:

- **§11.1 D-S1 baseline re-pin** — fixtures **47** (tail `0045-sni-cluster`) / fuzzers **36** / stat surface **136** / DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**) / all parent §4.1 line anchors re-verified against the live session tip `2a525ff`.
- **Parent D-question resolutions owned by 28.1** (§12): **D-P2** (shallow decode + the exact `WriteFilter` interface composition — `OnDestroy` ON `WriteFilter`, called once per instance), **D-P3** (read-filter writes do NOT route through the write chain — terminal-originated-writes-only, RESOLVED NO), **D-P5** (creation parity — ALL 201 counters created at 28.1, RESOLVED), **D-P7** (`writeChainConn.Write` returns `(len(p), nil)` on a chain-stopped write, RESOLVED), **D-P8** (the name.go arm is SHAPE-based, no allowlist, RESOLVED). **D-P1** (split gate) gets a SPEC-level re-check (§10.1: fits on the production-LoC basis); the FINAL gate re-check stays a 28.1-PLAN obligation per the parent.
- **The `TCPSink` BackendKind pin (§8.1.1)** — a NEW differential-runner backend kind (accept + read-discard + never write). Discovered at this SPEC session: the runner's existing backends (`TCPEcho=0` … `HTTPWasmPerRoute=27`, `test/differential/fixture/fixture.go:122-492`) all echo or respond. An ECHO backend breaks 28.1 stat parity: echoed ZK request bytes flow back through reference Envoy's `onWrite`, where its response decoder counts them (`*_resp` / `decoder_error` increments) — while envoy-go's 28.1 `OnWrite` stub counts nothing → cross-side divergence on counters the fixture never sent responses for. The `0046` backend MUST be silent. This refines parent §8.1's "canned-bytes TCP responder (it … need not reply)" into a hard requirement: it MUST NOT reply at 28.1.
- **The StatsAsserter scrape-mechanics refinement (§8.1.2)** — parent §8.1 sketched "the reference side reads `/stats?filter=zookeeper`, the subject side reads `/stats/prometheus`". The as-built `0043` precedent (`test/fixtures/0043-network-rbac/driver/driver.go:388-461`) scrapes **`/stats/prometheus` from BOTH sides** and compares the flattened forms. 28.1 adopts the 0043 mechanics verbatim — which also makes the R7 Prometheus-parity ratification intrinsic to every `0046` assertion (both sides' flattened names must agree for any value comparison to even locate the counter).
- **The write-only-filter boot posture pin (§3.6)** — `internal/listener/manager.go` stays UNTOUCHED at 28.1. Its boot classification (`buildNetworkChainFactory`, `manager.go:570-581`) accepts a both-direction filter through its existing `case network.ReadFilter` arm; a hypothetical write-ONLY filter would still hit the `default:` boot-reject. Pinned as a documented framework boundary (no production write-only consumer exists; lifting it is deferred under the API-revision allowance).
- **The 28.1 `OnWrite` stub semantics pin (§4.7)** — a pure no-op `Continue` (NOT a buffer-feeding stub): buffering write-direction bytes with no response decoder to drain them would grow unboundedly on long-lived connections.

---

## 2. Non-purposes

Phase 28.1 does NOT extend any subsystem beyond the minimum needed to land the seam + the request side under ADR-0221 + ADR-0222.

- **2.1 Response decoding OUT OF SCOPE.** The response decoder in `OnWrite` (response framing, connect-response special framing, watch events, xid correlation consumption, `*_resp`/`response_bytes` increments) is **28.2** (parent §3.2). The 28.1 `OnWrite` is a no-op `Continue` stub (§4.7). The correlation structures are WRITTEN at 28.1 but never read (R5).
- **2.2 Dynamic-metadata emission OUT OF SCOPE (deferred, not deferred-to-28.2).** Per parent AMEND-A9 the metadata mirror is DEFERRED project-wide with a BEHAVIOR_CONTRACT coverage-boundary record landing in the 28.1 bundle (§9). The ADR-0217 `*dynamicmetadata.Bucket` stays shaped-but-unwritten by zookeeperproxy.
- **2.3 Deep per-opcode payload parsing OUT OF SCOPE.** The decoder is SHALLOW (parent §2.1 + D-P2): framing + xid + opcode + min-length + the connect-readonly flag + the auth scheme string. Consequence (envoy-go-LENIENT departure, recorded at §9): a packet with a valid header but malformed payload counts as `<op>_rq` on envoy-go vs `decoder_error` upstream; the `0046` corpus contains no such packets.
- **2.4 Latency-threshold CONSUMPTION out of scope.** The latency fields are PARSED + VALIDATED at 28.1 (§4.3 — the full 9-field proto parse; the PGV-mirror reject arms exist as code) but CONSUMED at 28.2 (fast/slow increments + the boot-reject FIXTURE arms; parent §6.3 + D-P4).
- **2.5 No `StopIteration` from production write filters.** The seam pins the no-forward semantic (§3.5) and unit-tests it at the framework level, but NO production filter may return it at 28.x (documented-unsupported-by-consumers; mongo fault-delay is the anticipated first real consumer). zookeeperproxy's `OnWrite` always returns `Continue` (upstream parity — AMEND-A11).
- **2.6 Read-filter writes do NOT route through the write chain (D-P3 RESOLVED: NO).** A ReadFilter writing via `Connection().Write` (echo, direct_response — `chain.go:380-385`) bypasses the write chain. The write chain observes TERMINAL-originated writes only (§3.7). Sufficient for zookeeper (chain `[zookeeper_proxy, tcp_proxy]` — all upstream→downstream bytes come from tcp_proxy); recorded in BEHAVIOR_CONTRACT as a framework boundary; lifting it is deferred under the API-revision allowance.
- **2.7 Write-only filters are not bootable (§3.6).** The framework's classification + dispatch support a write-only `NetworkFilter` (unit-tested via `NewChainRuntime` directly), but `manager.go`'s boot classification rejects one (`resolved filter is neither a read nor a terminal network filter`). No production write-only consumer exists; `manager.go` is untouched at 28.1.
- **2.8 No real-ZooKeeper-server fixtures; no SASL/credential decode; no histograms; no `access_log` support** — all per parent §2.1 (the `access_log` field is parse-accept-IGNORE, upstream parity).
- **2.9 No new conformance harness.** h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate (phase 28 touches no HTTP path; HCM's chain has no write filters → no behavioral delta even through the seam).
- **2.10 No per-route surface.** Network filters carry no `typed_per_filter_config` (parent §2.2). The ADR-0125 roster is untouched.

---

## 3. Framework seam — the `network.WriteFilter` extension (ADR-0221)

In the existing `internal/filter/network/` package (NOT a new package). Refines parent §4.1 into production signatures + file split + dispatch semantics. The as-built anchors this extends (all re-verified at §11.1 against tip `2a525ff`):

- `internal/filter/network/types.go:29-48` — `ReadFilter` (embeds `NetworkFilter`); `types.go:61` — `FilterInstanceFactory func() NetworkFilter` (already returns the GENERAL `NetworkFilter` → a both-directions filter needs NO factory-signature change).
- `internal/filter/network/terminal.go:18-28` — the sealed `NetworkFilter` marker + `Marker` embeddable; `terminal.go:42-49` — `TerminalFilter.Handle(ctx, net.Conn)` (UNCHANGED).
- `internal/filter/network/chain.go:57-83` — `NewChainRuntime` classification (the type-switch to be restructured, §3.3); `chain.go:127-168` — the `chainRuntime` struct (gains a `writeFilters` field); `chain.go:215-227` — `handleTerminal` (the conn-wrap insertion point); `chain.go:321-326` — `onDestroy` (gains the once-per-instance dedupe).
- `internal/filter/network/prefixconn.go:12-28` — the `prefixConn` precedent (embed `net.Conn`, override one method) the `writeChainConn` mirrors.
- `internal/filter/network/callbacks.go:16-38` — `ReadFilterCallbacks` (UNCHANGED; `WriteFilterCallbacks` is a NEW separate interface).

### 3.1 New interfaces (production signatures; land at IMPL)

```go
// internal/filter/network/types.go — ADDITIONS (the read-filter surface is unchanged)

// WriteFilter is the per-connection write-filter interface — the write-direction
// (upstream→downstream) half of the chain. Mirrors upstream Network::WriteFilter
// (onWrite + initializeWriteFilterCallbacks) + the envoy-go OnDestroy convention.
// A filter may implement ReadFilter, WriteFilter, or BOTH (one instance seeing
// both directions — upstream Network::Filter / addFilter parity, AMEND-A11).
//
//nolint:revive // ADR-0221 reserves the network.WriteFilter name for the write-direction dispatch surface.
type WriteFilter interface {
	NetworkFilter
	// OnWrite is called with the bytes a terminal filter writes downstream,
	// BEFORE they reach the downstream socket, in REVERSE chain order
	// (AMEND-A11 LIFO parity). Return Continue to forward; StopIteration to
	// stop the write (the bytes are NOT forwarded — upstream no-forward parity;
	// documented-unsupported-by-consumers at 28.x).
	OnWrite(buf *Buffer, endStream bool) Status
	// SetWriteFilterCallbacks injects the per-connection write callbacks. Called
	// once by the chain runtime at construction, before any OnWrite.
	SetWriteFilterCallbacks(cb WriteFilterCallbacks)
	// OnDestroy releases per-connection resources. For a filter implementing
	// BOTH ReadFilter and WriteFilter this is the SAME method (one instance);
	// the chain runtime calls OnDestroy exactly ONCE per filter instance.
	OnDestroy()
}

// WriteFilterCallbacks is the per-connection callback surface handed to each
// WriteFilter via SetWriteFilterCallbacks. Minimal at 28.1 (D-P2): zookeeper
// needs only the connection accessor. Upstream's WriteFilterCallbacks carries
// injectWriteDataToFilterChain + disableClose — both deferred under the
// ADR-0213/0221 API-revision allowance until a consumer needs them.
//
//nolint:revive // ADR-0221 reserves the network.WriteFilterCallbacks name.
type WriteFilterCallbacks interface {
	// Connection returns the per-connection accessor (the same concrete impl
	// the ReadFilterCallbacks Connection() returns).
	Connection() Connection
}
```

Design pins (each carried into the IMPL task that lands it, §10):

- **`OnDestroy` ON `WriteFilter` (D-P2 resolved).** Interface symmetry with `ReadFilter`: a write-only filter gets a destroy hook; a both-directions filter defines `OnDestroy` once (one method satisfies both interfaces) and the runtime calls it exactly ONCE per instance (§3.3 dedupe). The parent's alternative (`OnDestroy` only on `ReadFilter`) would leave write-only filters without cleanup.
- **`SetWriteFilterCallbacks` is SEPARATE from `SetReadFilterCallbacks` (D-P2 resolved).** A both-directions filter receives BOTH injections (each exactly once, at chain construction). Mirrors upstream's separate `initializeReadFilterCallbacks`/`initializeWriteFilterCallbacks` on the combined `Network::Filter`.
- **`endStream` on `OnWrite` is always `false` at 28.1.** The `writeChainConn` intercepts `net.Conn.Write` calls, which carry no half-close signal (the same advisory posture as the 26.1 `Connection.Write` endStream note, `chain.go:377-385`). The parameter exists for upstream-parity (`onWrite(Buffer&, bool end_stream)`) + the future inject path. Unit-tested as always-false at 28.1.
- **`Status` is REUSED** (the existing two-value `Continue`/`StopIteration` enum, `types.go:13-23`). On the write path `StopIteration` means "this write does not proceed" (no resume — upstream has no `continueWriting()`; AMEND-A11). No new enum values.

### 3.2 File split (lands at IMPL)

| File | Change | Responsibility |
|---|---|---|
| `types.go` | EXTEND | + `WriteFilter` + `WriteFilterCallbacks` interfaces (§3.1) |
| `chain.go` | EXTEND | classification restructure (§3.3) + `chainRuntime.writeFilters` field + write-callbacks injection + `handleTerminal` wrap (§3.5) + `onDestroy` once-per-instance dedupe |
| `writeconn.go` | **NEW** | the `writeChainConn` (§3.5) — mirrors `prefixconn.go`'s single-override shape |
| `writeconn_test.go` | **NEW** | forward / stop / reverse-order / empty-chain-passthrough / endStream-false unit tests |
| `chain_test.go` | EXTEND | classification (read/write/both/terminal) + both-filter dual-injection + OnDestroy-once + back-compat (zero-write-filter chains produce NO wrap) |
| `terminal.go`, `callbacks.go`, `buffer.go`, `registry.go`, `prefixconn.go`, `upstreamcluster.go` | UNCHANGED | — |
| `internal/listener/manager.go` | **UNCHANGED** | §3.6 — the boot classification's existing `case network.ReadFilter` arm accepts a both-direction filter |

### 3.3 Chain classification extension (read / write / both / terminal)

`NewChainRuntime` (`chain.go:57-83`) currently classifies via a type-switch (`case TerminalFilter` / `case ReadFilter` / `default` ignore). A type-switch CANNOT classify a both-directions filter into two sets (the first matching case wins), so the classification is restructured into independent type-asserts:

```go
// inside NewChainRuntime — replaces the chain.go:62-74 type-switch
var (
	read     []ReadFilter
	write    []WriteFilter // CHAIN order; dispatch reverses (§3.4)
	terminal TerminalFilter
)
for _, f := range filters {
	if t, ok := f.(TerminalFilter); ok {
		terminal = t // boot validation (manager.go) guarantees uniqueness + last position
		continue
	}
	if rf, ok := f.(ReadFilter); ok {
		read = append(read, rf)
	}
	if wf, ok := f.(WriteFilter); ok {
		write = append(write, wf)
	}
	// A NetworkFilter that is neither (sealed-marker-only) is defensively
	// ignored, exactly as today.
}
```

Pinned contract:

1. A filter implementing BOTH interfaces appears in BOTH `read` and `write` — the SAME instance (upstream `addFilter` parity). zookeeperproxy is the first such filter.
2. The `chainRuntime` gains a `writeFilters []WriteFilter` field (chain order). `newChainRuntime`'s read-callbacks injection loop (`chain.go:185-187`) is mirrored by a write-callbacks injection loop: every `WriteFilter` receives `SetWriteFilterCallbacks` exactly once at construction, with a concrete `*writeCallbacks{rt}` whose `Connection()` returns the SAME `rt.cxn` the read callbacks return.
3. **`onDestroy` once-per-instance (D-P2).** `chain.go:321-326` currently iterates read filters. Extended: iterate read filters (call `OnDestroy`), then write filters SKIPPING any instance already destroyed (identity comparison — a `map[NetworkFilter]bool` of seen instances; filter instances are pointers, hence comparable). A both-directions filter's `OnDestroy` runs exactly once.
4. The eager `OnNewConnection` pass, the `OnData` iteration, `connHalted`, `ContinueReading`, `closeReq`/`closeType`, and the upstream-cluster override (`chain.go:236-369`) are all UNCHANGED — write filters do not participate in read-direction dispatch.

### 3.4 REVERSE-order write dispatch (AMEND-A11)

The write chain dispatches in REVERSE chain order (upstream LIFO parity: `addWriteFilter` front-inserts; config `[A, B, C]` ⇒ write dispatch `C → B → A`). Pinned mechanics: `chainRuntime.writeFilters` stores CHAIN order; `handleTerminal` hands the `writeChainConn` a REVERSED copy (dispatch order), so `writeChainConn.Write` iterates its slice front-to-back. For 28.1's only production chain (`[zookeeper_proxy, tcp_proxy]` — one write filter) the order is trivially `[zookeeper_proxy]`; the rule is pinned + unit-tested with two synthetic write filters for the multi-write-filter future (mongo).

### 3.5 `writeChainConn` delivery + D-P7 return semantics

```go
// internal/filter/network/writeconn.go (NEW) — write-chain conn for terminal handoff

// writeChainConn runs the write-filter chain over every terminal-originated
// downstream write BEFORE forwarding to the wrapped conn. Mirrors upstream
// ConnectionImpl::write → filter_manager_.onWrite() → transport (AMEND-A11).
// All non-Write methods promote from the embedded net.Conn (so Read still
// reaches a wrapped prefixConn's buffered-prefix replay).
type writeChainConn struct {
	net.Conn                // the wrapped conn (prefixConn or the raw downstream conn)
	filters  []WriteFilter  // DISPATCH order (already reversed by handleTerminal)
}

func newWriteChainConn(c net.Conn, dispatch []WriteFilter) *writeChainConn {
	return &writeChainConn{Conn: c, filters: dispatch}
}

func (w *writeChainConn) Write(p []byte) (int, error) {
	buf := &Buffer{}
	buf.Append(p)
	for _, f := range w.filters {
		if f.OnWrite(buf, false) == StopIteration {
			// Upstream parity: the write does not proceed; nothing reaches the
			// transport. The terminal cannot distinguish (D-P7): report success.
			return len(p), nil
		}
	}
	// Forward the POST-CHAIN buffer bytes (upstream parity: the transport sees
	// the filtered buffer). At 28.x no filter mutates, so buf.Bytes() == p.
	if _, err := w.Conn.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
```

Pinned semantics:

1. **Wrap composition + ordering.** `handleTerminal` (`chain.go:215-227`) wraps in this order: `prefixConn` FIRST (inner — iff undrained buffered prefix), `writeChainConn` SECOND (outer — iff ≥1 write filter). The terminal therefore sees `writeChainConn(prefixConn(conn))`, `writeChainConn(conn)`, `prefixConn(conn)`, or `conn`. Reads promote through `writeChainConn` to the inner conn (prefix replay intact); writes pass through the chain then the inner conn.
2. **Zero-write-filter chains get NO wrap** — `handleTerminal` is byte-identical to today for every existing chain (`tcp_proxy`-only, HCM, the 26.x/27 fixtures). This is the seam's back-compat invariant (R1), ratified by ALL 47 existing fixture dirs staying byte-exact green.
3. **D-P7 RESOLVED: chain-stopped writes return `(len(p), nil)`.** The terminal cannot distinguish a stopped write from a delivered one — exactly as upstream, where `ConnectionImpl::write` returns `void` and a StopIteration leaves the data in the caller's buffer with no error signal. A dropped write surfaces as downstream silence.
4. **Forwarded bytes are the POST-CHAIN buffer** (`buf.Bytes()`), not `p` — upstream parity (write filters may mutate; the transport sees the filtered buffer). At 28.x no production filter mutates, so this is byte-identical to forwarding `p`; the distinction is pinned for the mongo future and unit-tested with a synthetic mutating write filter.
5. **Underlying-write errors propagate** as `(0, err)` (the terminal's existing downstream-write error handling — e.g. tcp_proxy's pump exit — sees the error exactly as it would from the raw conn). The byte-count fidelity caveat under mutation is moot at 28.x (no mutation) and documented in the writeconn.go comment.
6. **Per-Write allocation posture.** Each `Write` allocates one `*Buffer` + one `Append` copy. Accepted at 28.1 (simplicity > zero-copy; tcp_proxy's pump already copies via `io.Copy` buffers). A pooling optimization is a non-goal.

### 3.6 Boot classification posture — `manager.go` UNTOUCHED

`buildNetworkChainFactory` (`internal/listener/manager.go:534-599`) validates the chain shape at boot via a one-shot sample chain: pass 1 (`:570-581`) classifies each filter (`case network.TerminalFilter` / `case network.ReadFilter` / `default` → boot-reject `resolved filter is neither a read nor a terminal network filter`); pass 2 (`:586-590`) enforces terminal-last. Pinned 28.1 posture:

- A both-direction filter (zookeeperproxy implements `ReadFilter`) matches `case network.ReadFilter` → **no manager.go change is needed**. The boot-side `[read*, terminal?]` shape validation subsumes `[read-or-both*, terminal?]` with zero code delta.
- A write-ONLY filter still boot-rejects via the `default:` arm. This is a PINNED framework boundary (not an oversight): no production write-only consumer exists; the every-surface-exercised discipline forbids building a bootable-but-unconsumed surface; the existing reject wording stays byte-stable (ADR-0080). The framework-level write-only classification (§3.3) IS unit-tested via direct `NewChainRuntime` construction. Lifting the boot boundary (extending pass 1 + its wording) is deferred under the API-revision allowance to the first write-only consumer.
- Consequence: 28.1's production diff to `internal/listener/` is ZERO files.

### 3.7 Terminal-originated-writes-only boundary (D-P3 RESOLVED: NO)

`Connection.Write` (`chain.go:380-385` — the read-filter write path used by echo/direct_response) writes DIRECTLY to `rt.conn` and does NOT route through the write chain. Upstream routes ALL connection writes through `onWrite`; envoy-go at 28.1 routes only terminal-originated writes (those issued through the conn handed to `TerminalFilter.Handle`). Justification: (i) zookeeper's chain `[zookeeper_proxy, tcp_proxy]` has ALL upstream→downstream bytes produced by tcp_proxy — the boundary is invisible to phase 28; (ii) routing `Connection.Write` through the chain would let a read filter's write re-enter ITS OWN `OnWrite` (zookeeperproxy is both) — a re-entrancy hazard upstream avoids structurally (different objects) that envoy-go would have to special-case; (iii) no consumer needs it. Recorded in BEHAVIOR_CONTRACT (§9) as a framework boundary under the API-revision allowance.

---

## 4. The `zookeeperproxy` package, request side (ADR-0222)

NEW Go package `internal/filter/network/zookeeperproxy/` (package `zookeeperproxy`, single-token-joined per the `directresponse`/`snicluster` precedent). Implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection).

### 4.1 File split (lands at IMPL)

| File | Responsibility |
|---|---|
| `doc.go` | package doc — the zookeeper_proxy request side; ADR-0222 cross-refs; the 28.2 forward-pointer |
| `zookeeperproxy.go` | `TypeURL` (via `proto.MessageName` — §4.2) + `NewFactory(reg *stats.Registry)` + the `filter` struct glue (§4.7) |
| `config.go` | `compiledConfig` + `parseConfig` (9-field parse + PGV-mirror validation + the proto→wire opcode mapping + the latency-override map) — §4.3 |
| `stats.go` | the opname table + the 201-suffix roster + `rosterStats` eager creation + the dynamic auth-scheme counters — §4.4 |
| `decoder.go` | the shallow request decoder (framing + reassembly + xid dispatch + min-length + counters) + the two correlation structures — §4.5/§4.6 |
| `*_test.go` | per-file unit tests (§15.1) |
| `fuzz_test.go` | `FuzzZookeeperRequestDecode` (the 37th fuzzer — §10 Task) |

### 4.2 TypeURL + factory shape

```go
// TypeURL is derived via proto.MessageName, NEVER a hand-typed docs string
// (reference_network_filter_typeurl_extensions; the rbac.go:38 precedent —
// NOT the snicluster.go:27 const-string shape).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&zookeeper_proxyv3.ZooKeeperProxy{}))

// NewFactory returns the zookeeperproxy NetworkFilterFactory with the stats
// registry closure-captured (the rbac NewFactory(reg) / D-26.3-3 precedent —
// network.FactoryCtx carries no stats registry).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory
```

Pinned: `proto.MessageName` resolves to `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` (parent §5.1; the `extensions.` segment). The IMPL Task-1 pinning test asserts `TypeURL` ends in the parent §5.1 string — derivation by `proto.MessageName`, assertion against the empirically-pinned literal.

The factory parses + validates ONCE at boot (ADR-0079 two-step factory) and creates the 201-counter roster ONCE per distinct `stat_prefix` at parse time (§4.4). The returned `FilterInstanceFactory` allocates a fresh `*filter` per connection, all sharing the boot-parsed `*compiledConfig` (incl. the shared roster counters — counter increments are atomic; the per-connection state is the decoder + correlation structures, §4.5/§4.6).

### 4.3 Config parse (9 fields; the proto→wire mapping; the latency parse-not-consume split)

Parses the FULL 9-field proto (parent §5.2 roster, inherited verbatim). Per-field 28.1 disposition:

| Field | 28.1 parse behavior |
|---|---|
| `stat_prefix` | REQUIRED (PGV min-1-rune mirror) → boot-reject `zookeeper_proxy: stat_prefix is required` (§6.1; the `0047` arm) |
| `access_log` | parse-accept-IGNORE (upstream parity — completely unread upstream; parent §11.2) |
| `max_packet_bytes` | default 1 MiB when unset; any uint32 accepted incl. 0 (no PGV bound; 0 → every packet oversized → `decoder_error`; upstream parity) |
| `enable_latency_threshold_metrics` | parsed + stored; CONSUMED at 28.2 |
| `default_latency_threshold` | parsed; PGV-mirror gte 1ms WHEN SET → reject arm (§6.2); default 100 ms when unset; CONSUMED at 28.2 |
| `latency_threshold_overrides` | parsed; PGV-mirror per-override `threshold` required + gte 1ms, `opcode` defined_only; DUPLICATE opcode → reject (upstream config-load exception mirror); built into a `map[wireOpcode]time.Duration` keyed by WIRE opcode via the proto→wire mapping; CONSUMED at 28.2 |
| `enable_per_opcode_request_bytes_metrics` | parsed + CONSUMED at 28.1 (gates `*_rq_bytes` increments) |
| `enable_per_opcode_response_bytes_metrics` | parsed + stored; CONSUMED at 28.2 |
| `enable_per_opcode_decoder_error_metrics` | parsed + CONSUMED at 28.1 (gates per-opcode `*_decoder_error` increments) |

**The proto→wire opcode mapping** (parent §5.3 + AMEND-A6) lands in `config.go` at 28.1: the proto's 27-value contiguous `LatencyThresholdOverride_Opcode` enum (0..26) maps to the wire `OpCodes` values (26 values with gaps + `Close=−11`; `Ping=11` not 10; `SetAuth=100`; …). Both enums + the mapping table are transcribed as Go constants/tables with a byte-stable `TestProtoToWireOpcodeMapping` test against the parent §5.3 + §11.4 pinned values. The mapping is NEEDED at 28.1 only to validate + key the override map (consumed at 28.2); it also provides the wire-opcode → opname table the decoder dispatches on (§4.5).

### 4.4 The 201-counter EAGER roster (D-P5 RESOLVED: all 201 at 28.1) + dynamic auth counters

**Creation parity (D-P5).** ALL 201 macro counters are created EAGERLY at config parse, including the 140 response-side counters (28 `_resp` + 28 `_resp_bytes` + 28 `_resp_fast` + 28 `_resp_slow` + `response_bytes` + `watch_event` + the response-side share of `_decoder_error`… — see the family table below) whose increment paths land at 28.2. Rationale: (i) upstream creates the full struct from one macro at config load — the booted reference image exposes all 201 at 0 (parent §11.1); (ii) the `0046` exists-at-zero arm (§8.1 arm 6) asserts this cross-side, making creation parity a differentially-proven invariant the 28.2 increments cannot regress; (iii) a request-side-only creation would force the 28.2 IMPL to touch the roster table (churn).

**The roster shape (`stats.go`).** The 28-name opname table (parent §7.2, the stats-macro spelling): `connect`, `ping`, `auth`, `getdata`, `create`, `create2`, `createcontainer`, `createttl`, `setdata`, `getchildren`, `getchildren2`, `getallchildrennumber`, `getephemerals`, `delete`, `exists`, `getacl`, `setacl`, `sync`, `check`, `multi`, `reconfig`, `setauth`†, `setwatches`, `setwatches2`, `addwatch`, `checkwatches`, `removewatches`, `close` (+ `connect_readonly` in the `_rq`/`_rq_bytes` families only). († the SetAuth wire opcode's opname is `auth` — there are no `setauth_*` counters; the `setauth` table entry maps to the `auth` counter family.) The family table (parent §7.2, inherited):

| Family | Count | Created | Incremented | Gated by |
|---|---|---|---|---|
| `decoder_error`, `request_bytes` | 2 | **28.1** | **28.1** | never |
| `response_bytes`, `watch_event` | 2 | **28.1** | 28.2 | never |
| `<op>_rq` (incl. `connect_readonly_rq`; NO `auth_rq`) | 28 | **28.1** | **28.1** | never |
| `<op>_rq_bytes` (incl. both connect variants + `auth_rq_bytes`) | 29 | **28.1** | **28.1** | `enable_per_opcode_request_bytes_metrics` |
| `<op>_decoder_error` (incl. `connect_decoder_error`) | 28 | **28.1** | **28.1** | `enable_per_opcode_decoder_error_metrics` |
| `<op>_resp` (incl. `auth_resp`) | 28 | **28.1** | 28.2 | never |
| `<op>_resp_bytes` | 28 | **28.1** | 28.2 | `enable_per_opcode_response_bytes_metrics` |
| `<op>_resp_fast` | 28 | **28.1** | 28.2 | `enable_latency_threshold_metrics` |
| `<op>_resp_slow` | 28 | **28.1** | 28.2 | `enable_latency_threshold_metrics` |
| **Total** | **201** | | | |

**Implementation shape:** a `rosterSuffixes() []string` function (or package-level table) producing the exact 201 suffixes, and a `rosterStats` struct holding `counters map[string]*stats.Counter` keyed by suffix, created via `reg.NewCounterIfAbsent("<stat_prefix>.zookeeper." + suffix)` per suffix. `NewCounterIfAbsent` (NOT `NewCounter`): (i) it is post-Freeze-permitted (`internal/stats/registry.go:142-171` — the docstring's "PERMITTED post-Freeze" + the freeze-bypassing body; the network-filter config parse runs at listener-manager build, whose ordering relative to the stats `Freeze()` must not matter); (ii) two listeners/chains sharing a `stat_prefix` share counters idempotently (the rbac `newFilterStats` precedent, `internal/filter/network/rbac/rbac.go:187-198`). The byte-stable **`TestCounterRoster_MatchesUpstreamMacro`** test pins all 201 suffixes against a sorted golden list transcribed from the upstream macro (the R2 ratification; the digit-suffixed names `create2`/`getchildren2`/`setwatches2`/`getallchildrennumber` guard against `reference_proto_roster_extraction_digits`-class truncation).

**Dynamic per-scheme auth counters.** `<stat_prefix>.zookeeper.auth.<scheme>_rq` (e.g. `auth.digest_rq`; unknown scheme → `auth.unknown_scheme_rq`), created LAZILY at decode time via `reg.NewCounterIfAbsent` (the rbac per-policy dynamic-name precedent) — post-Freeze-permitted is REQUIRED here (decode time is runtime). NOT counted in the static 337 surface (§7.3). The scheme string is the only payload value the shallow decoder extracts beyond the connect-readonly flag (§4.5 auth arm).

### 4.5 The shallow request decoder (`decoder.go`)

Mirrors the upstream decoder contract pinned at parent §11.4/§11.5, at SHALLOW depth (D-P2). All multi-byte reads are big-endian (`encoding/binary.BigEndian`).

**Decoder-internal reassembly (AMEND-A8).** The decoder owns its OWN `readBuf []byte`. `decodeOnData(chainBytes []byte)`: append a COPY of the chain buffer's NEW bytes to `readBuf` (the chain `Buffer` is read, NEVER drained — passthrough is the chain runner's job, not the decoder's), then loop: while `readBuf` holds a complete frame (4-byte length prefix, length EXCLUDES itself), decode it and consume it from `readBuf`; a trailing partial frame stays in `readBuf` for the next read. NOTE on "new bytes": because the chain `Buffer` accumulates undrained bytes across reads (zookeeperproxy never drains), the filter tracks a `consumed int` high-water mark into the chain buffer and feeds only `buf.Bytes()[consumed:]` to the decoder per `OnData` call (the alternative — re-feeding the whole buffer — double-counts). This high-water-mark mechanism is a pinned IMPL detail with a dedicated multi-read unit test.

**Per-frame dispatch (AMEND-A5; xid sniffing, per-packet, no state machine):**

1. `len < 8` after the length prefix → min-length failure (`decoder_error` path).
2. `len > max_packet_bytes` → "packet too big" → `decoder_error` path.
3. Peek xid (first 4 bytes of the frame). Switch:
   - **xid == 0 (ConnectXid)** → connect request: parse protocol_version(4) + last_zxid(8) + timeout(4) + session_id(8) + password(4-byte-len + bytes) + OPTIONAL trailing readonly bool(1). Readonly present + true → `connect_readonly_rq` +1; else `connect_rq` +1. Record in the CONTROL queue (§4.6).
   - **xid == −2 (PingXid)** → `ping_rq` +1; control queue.
   - **xid == −4 (AuthXid)** → auth request: skip the type int(4), parse the scheme string (4-byte-len + bytes) → dynamic `auth.<scheme>_rq` +1 (§4.4); flag-gated `auth_rq_bytes`; control queue.
   - **xid == −8 (SetWatchesXid)** → `setwatches_rq` +1; control queue.
   - **default (data request)** → peek opcode int(4) → wire-opcode → opname lookup (§4.3 table). Known opcode → `<opname>_rq` +1 + per-opcode min-length validation (a per-opcode minimum-frame-length table transcribed from upstream `decoder.cc` at IMPL; failures → `decoder_error` path); record in the DATA map (§4.6). Unknown opcode → `decoder_error` path.
4. Every successfully decoded request: `request_bytes` += the frame's wire footprint (the 4-byte length prefix + `len` payload bytes — the IMPL-verified accounting whose cross-side equality is the `0046` arm-2 proof); flag-gated `<opname>_rq_bytes` += the same.

**The `decoder_error` path (AMEND-A8):** `decoder_error` +1; flag-gated `<opname>_decoder_error` +1 when the failing frame's opcode is known (header-level failures with no known opcode increment only the plain counter); ABANDON the remaining bytes in the current `readBuf` (no resync — upstream parity); the connection is NEVER closed; later `OnData` calls decode fresh bytes normally; the correlation structures persist.

### 4.6 The two correlation structures (written at 28.1; consumed at 28.2)

Per AMEND-A7, mirroring upstream `decoder.h:208-213`:

- **`requestsByXid map[int32]pendingRequest`** — data requests (xid > 0). Insert OVERWRITES an existing entry for the same xid. (28.2's response lookup ERASES.)
- **`controlRequestsByXid map[int32][]pendingRequest`** — control requests (connect 0 / ping −2 / auth −4 / setwatches −8): a FIFO queue per control xid (control xids repeat).
- `pendingRequest` carries `{opname string, wireOpcode int32, start time.Time}` — the fields 28.2's correlation + latency measurement need. The `start` timestamp is recorded at 28.1 (cheap; avoids a 28.2 struct revision).

Both structures are PER-CONNECTION (fields on the per-connection decoder, not the shared config). Written by every successful request decode; NEVER read at 28.1 (R5 — the 26.1 shaped-but-unwritten precedent). Unbounded-growth note: upstream's maps also grow unboundedly for never-answered requests; envoy-go mirrors (no eviction); the structures die with the connection (`OnDestroy`).

### 4.7 Filter glue (`zookeeperproxy.go` — the both-directions filter)

```go
type filter struct {
	network.Marker
	cfg     *compiledConfig   // shared, boot-parsed (incl. the roster counters)
	decoder *requestDecoder   // per-connection (reassembly buf + correlation structures)
	cb      network.ReadFilterCallbacks
	wcb     network.WriteFilterCallbacks
}
```

- **`OnNewConnection() → network.Continue`** — no-op (the `reference_network_read_filter_onnewconnection_halts` constraint: an OnNewConnection StopIteration is a sticky halt that would block all OnData).
- **`OnData(buf, endStream) → network.Continue` ALWAYS** — feeds `decoder.decodeOnData` with the chain buffer's new bytes (§4.5 high-water mark); NEVER drains the chain buffer, never closes, never halts (AMEND-A8 passthrough; the R3 ratification).
- **`OnWrite(buf, endStream) → network.Continue` ALWAYS — a PURE NO-OP at 28.1 (§1.2 pin).** It does NOT buffer write-direction bytes: with no response decoder to drain it, a write-side reassembly buffer would grow unboundedly on long-lived connections. The write-side reassembly buffer is created WITH the response decoder at 28.2. (The 28.1 OnWrite body exists so the filter satisfies `WriteFilter` and exercises the seam end-to-end — the `0046` fixture's traffic DOES flow through `writeChainConn` → `OnWrite` even though OnWrite counts nothing.)
- **`SetReadFilterCallbacks` / `SetWriteFilterCallbacks`** — store both (the both-directions dual injection, §3.3).
- **`OnDestroy`** — drops the per-connection decoder (the correlation structures + reassembly buffer die with the connection). Called exactly once (§3.3 dedupe).

### 4.8 The 7th built-in + bootstrap blank-import

- `internal/filter/network/builtins/builtins.go`: `reg.Register(zookeeperproxy.TypeURL, zookeeperproxy.NewFactory(deps.StatsRegistry))` — the 7th registration, mirroring the rbac_network closure-capture shape (`builtins.go:59`); the package doc-comment's "six built-in network filters" count is updated to seven. Registration order is behavior-neutral (ADR-0072).
- `internal/bootstrap/bootstrap.go`: blank-import `_ ".../envoy/extensions/filters/network/zookeeper_proxy/v3"` (the echo/sni_cluster precedent at `bootstrap.go:76-77,87` — required for `@type` Any resolution at config load; differential bootstraps also need ≥1 cluster per `reference_network_filter_typeurl_extensions`).

---

## 5. Proto-field roster (cross-reference parent §5)

INHERITED VERBATIM from parent §5.1 (TypeURL) + §5.2 (the 9-field `ZooKeeperProxy` table with PGV + defaults) + §5.3 (`LatencyThresholdOverride` + the 27-value proto enum + the 26-value wire enum + the mapping). No re-transcription here. The 28.1 IMPL Task-1 gate re-confirms `proto.MessageName` + the field roster against go-control-plane v1.32.4 in-tree before writing the parser (the parity gate), per the 26.1/26.3/27 Task-1 precedent.

---

## 6. PARSE-REJECT roster (cross-reference parent §6)

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a `TestParseRejectConstants_ByteStable` table test at IMPL. The error prefix for all zookeeperproxy arms is **`zookeeper_proxy: `** (mirrors `rbac_network: ` / `tcpproxy: `; the exact byte-stable wording of each arm is finalized at IMPL — D-S28.1-2). Phase 28 has NO envoy-go-strict departure-class rejects (parent §6.1) — every arm below mirrors an upstream PGV/config-load failure.

### 6.1 The load-bearing 28.1 arm (fixture-proven)

- **`zookeeper-stat-prefix-required`** — missing/empty `stat_prefix` → boot-reject (PGV min-1-rune mirror). Anticipated wording: `zookeeper_proxy: stat_prefix is required`. The `0047` fixture arm (§8.2): BOTH sides reject at boot; common stderr substring `stat_prefix`.

### 6.2 The latency PGV-mirror arms (code at 28.1; unit-test-only at 28.1; fixture disposition is 28.2's — parent D-P4)

Because the 28.1 config parse validates the FULL proto (§4.3), these arms' parse code + unit tests land at 28.1:

- `zookeeper-latency-override-threshold-required` / `zookeeper-latency-override-threshold-too-small` (PGV `required` + `gte 1ms`)
- `zookeeper-latency-override-opcode-undefined` (PGV `defined_only`)
- `zookeeper-default-latency-threshold-too-small` (PGV `gte 1ms` when set)
- `zookeeper-latency-override-duplicate-opcode` (the upstream config-load `EnvoyException` mirror — `config.cc:43-50`)

Whether `0047` gains fixture arms for these at 28.2 (or they stay unit-test-only) is parent D-P4, resolved at the 28.2 SPEC.

### 6.3 Framework-level arms (existing; no new wording)

- Unknown network-filter `typed_config` type_url → the existing unified reject (`manager.go:551`); zookeeperproxy joins the registry, no new arm.
- Write-only filter at boot → the existing `default:` reject (`manager.go:580`) — UNCHANGED wording (§3.6 pinned boundary).
- `access_log` / `max_packet_bytes`: NOT rejects (parse-accept; §4.3).

---

## 7. Stat surface (cross-reference parent §7)

### 7.1 Scope shape — `<stat_prefix>.zookeeper.<counter>` (AMEND-A1, inherited)

envoy-go mirrors upstream's internal naming exactly (the `StatsAsserter` + the Prometheus arm depend on it). Internal registration name = `<stat_prefix>.zookeeper.<suffix>` for all 201 + the dynamic `auth.<scheme>_rq` names.

### 7.2 The roster + creation parity

Inherited from parent §7.2; refined into the §4.4 implementation shape (the suffix table + `rosterStats` map + `NewCounterIfAbsent` + the R2 roster test). D-P5 RESOLVED: all 201 created at 28.1.

### 7.3 Project stat-count delta — 136 → **337** at 28.1

All +201 land at 28.1 (creation parity). 28.2 adds ZERO new counter names (it adds increments only). The dynamic auth-scheme counters are excluded from the static count (config/traffic-dependent; the rbac `policy.<name>.*` precedent). The BEHAVIOR_CONTRACT stat table gains the 201 rows in the 28.1 bundle (§9).

### 7.4 Prometheus exposition — the `.zookeeper.` INLINE-PREFIX arm (AMEND-A4; D-P8 RESOLVED: shape-based)

Reference Envoy emits zookeeper stats FLAT: `envoy_<stat_prefix>_zookeeper_<counter>{}` (empty label set; parent §11.1 live probe). envoy-go's `internal/stats/name.go` default branch errors on unrecognized prefixes (`name.go:243`), so a NEW arm is required. **D-P8 RESOLVED: SHAPE-based detection, NO per-counter allowlist:**

```go
// In flattenToProm's default branch, after the .rbac. arm (name.go:226-242),
// before the final unrecognized-prefix error (name.go:243):
//
// Phase-28.1 zookeeper_proxy INLINE-PREFIX detection (ADR-0222; parent AMEND-A4;
// the ADR-0138 bandwidth_limit + wasm.* permissive-shape precedents). Internal
// name <stat_prefix>.zookeeper.<counter> (counter MAY contain dots — the dynamic
// auth.<scheme>_rq family) flattens to envoy_<stat_prefix>_zookeeper_<counter>
// with NO label promotion (upstream applies no tag extraction to this filter).
const zkSegment = ".zookeeper."
if idx := strings.Index(internal, zkSegment); idx > 0 {
	prefix := internal[:idx]
	if !strings.ContainsRune(prefix, '.') {
		base = "envoy_" + strings.ReplaceAll(internal, ".", "_")
		return base, nil, nil
	}
}
```

Rationale (vs the ADR-0138 14-name allowlist): 201 static names + an open-ended dynamic family make an allowlist unmaintainable; the `wasm.` arm (`name.go:88-122`) is the established permissive precedent for large/open rosters ("the rule is intentionally permissive — no per-counter allow-list"). The validation is the SHAPE: a `.zookeeper.` segment with a dot-free head. Counter names containing dots (e.g. `auth.digest_rq`) flatten correctly via the full-string dot→underscore. Documented acceptance: any future stat named `<x>.zookeeper.<y>` from another subsystem would match this arm (the same acceptance the wasm. arm carries). KEEP-IN-SYNC comment pointing at `zookeeperproxy/stats.go`.

### 7.5 envoy-go-strict / envoy-go-lenient departure flags (BEHAVIOR_CONTRACT 28.1 bundle)

Inherited from parent §7.5, the 28.1-landing subset: dynamic metadata unmirrored (AMEND-A9 coverage boundary); shallow-decode leniency (payload-malformed → `<op>_rq` not `decoder_error`); the terminal-originated-writes-only write-chain boundary (§3.7); the write-only-filter boot boundary (§3.6); `access_log` parse-accept-ignore (parity, recorded for completeness). The latency-HISTOGRAM coverage boundary is 28.2's.

---

## 8. Differential fixture taxonomy (+2; cross-reference parent §8)

Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE dirs (one dir = one runner branch). Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `fixture.StatsAsserter` (`test/differential/fixture/fixture.go:70-77`; dispatched ONLY on the cross-side path, `runner_test.go:1048-1050`) and MUST be proven live via a deliberate-break. Numbering continues from `0045` (the verified tail, §11.1): 28.1 lands **`0046` + `0047`** → 47 → **49** dirs.

### 8.1 `0046-zookeeper-requests` (cross-side; the load-bearing fixture)

**Topology.** Chain `[zookeeper_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 docker + envoy-go subprocess). TWO listeners (the `0043` `MultiListenerDriver` precedent, `driver.go:203-207`):

| Listener | zookeeper config | Reference port | Purpose |
|---|---|---|---|
| `l_plain` | `stat_prefix: zk_plain`; all flags false (defaults) | 15046 | arms 1–4, 6 |
| `l_flags` | `stat_prefix: zk_flags`; `enable_per_opcode_request_bytes_metrics: true` + `enable_per_opcode_decoder_error_metrics: true` | 15047 | arm 5 |

Both listeners route to ONE cluster → ONE backend.

#### 8.1.1 The `TCPSink` BackendKind (NEW runner plumbing — §1.2 pin)

The fixture backend MUST be SILENT (accept + read-discard + never write). With the existing `TCPEcho` backend, request bytes echo back through reference Envoy's `onWrite` response decoder → ref-side `*_resp`/`decoder_error` increments that envoy-go's 28.1 OnWrite stub never mirrors → cross-side divergence (and the arm-6 exists-at-zero assertions break). 28.1 adds:

- **`TCPSink BackendKind = 28`** (`test/differential/fixture/fixture.go` — the next free value after `HTTPWasmPerRoute = 27`): the runner backend accepts connections, drains reads (`io.Copy(io.Discard, conn)`), never writes, closes on client close. The accept counter increments as for `TCPEcho` (the existing per-backend `atomic.Uint64`).
- The `0046` driver implements `BackendKindAware` (`fixture.go:495-498`) returning `TCPSink`.
- Forward note: 28.2's `0048` (which NEEDS responses) will use a driver-controlled responder backend — a separate 28.2-SPEC concern; `TCPSink` stays request-side-only.

#### 8.1.2 StatsAsserter mechanics (the 0043 precedent, adopted verbatim)

The driver implements `fixture.StatsAsserter` — `AssertStats(t TB, refAdminAddr, subjAdminAddr string)`. Both sides are scraped via **`GET /stats/prometheus`** (the `0043` `scrapeRBACStats`/`parseRBACPromBody` mechanics, `driver.go:388-461`): parse lines matching the `_zookeeper_` infix into `name → value`; expected counters are looked up via their FLATTENED form `envoy_<prefix>_zookeeper_<suffix>` (no labels — AMEND-A4). Consequence: every value assertion intrinsically asserts Prometheus name parity (R7) — if envoy-go's §7.4 arm produced a different flattened name, the lookup would miss and the assertion would fail.

#### 8.1.3 Arms (refines parent §8.1's 7 arms)

Each arm drives BOTH sides identically (`DriveReference`/`DriveSubject` via `MultiListenerDriver`), then `AssertStats` compares per-counter expected values on both sides. The drive output (the `CompareBytes` surface) is side-independent per-arm verdict lines (`arm <name> sent=<n> verdict=<v>\n` — the `0043` verdict-line precedent); the body differential is intrinsically vacuous for a passive sniffer — the stat comparison IS the proof.

1. **connect** (`l_plain`, fresh conn): one hand-crafted connect frame (xid 0; protocol_version 0 + zxid 0 + timeout + session 0 + 16-byte password; NO readonly flag) → `zk_plain.zookeeper.connect_rq` == 1 both sides.
2. **multi-opcode sequence** (`l_plain`, one conn): connect + ping (xid −2) + getdata (xid 1, wire op 4) + create (xid 2, wire op 1) + close (xid 3, wire op −11), written as separate frames with small inter-write delays → `connect_rq`/`ping_rq`/`getdata_rq`/`create_rq`/`close_rq` each +1; **`request_bytes` EQUAL cross-side** (the byte-accounting proof, §4.5 item 4).
3. **digit-suffixed opcodes** (`l_plain`, one conn): create2 (wire op 15) + getchildren2 (wire op 12) + setwatches2 (wire op 105) as data requests → `create2_rq`/`getchildren2_rq`/`setwatches2_rq` each == 1 (the `reference_proto_roster_extraction_digits` regression guard).
4. **garbage + connection-survival** (`l_plain`, one conn): a frame whose 4-byte length prefix exceeds `max_packet_bytes` (1 MiB default) → `decoder_error` +1 both sides; then (after a ≥200 ms pause so both sides see a separate socket read — the abandon-buffer semantic applies per-buffer) a valid getdata frame → `getdata_rq` +1 both sides. Proves: decode failure increments the counter, does NOT close the connection (passthrough/never-close — R3), and later buffers decode normally.
5. **flag-gated increments** (`l_flags`): a getdata frame → `zk_flags.zookeeper.getdata_rq_bytes` > 0 AND EQUAL cross-side; cross-check `zk_plain.zookeeper.getdata_rq_bytes` == 0 on both sides (the flag gates increments, not creation — AMEND-A2).
6. **eager-roster / exists-at-zero** (both prefixes): `getdata_resp`, `getdata_resp_fast`, `watch_event`, `response_bytes` all PRESENT and == 0 on both sides (creation parity D-P5/R2; the `TCPSink` backend guarantees zero response-direction traffic).
7. **deliberate-break liveness proof** (R4; the `0030` dead-assertion lesson): recorded in driver comments + README + PROGRESS.md at IMPL — e.g. temporarily asserting `getdata_rq == 2` (when 1 is sent) MUST fail on both runner paths; temporarily disabling the §7.4 name.go arm MUST fail arm 6 (the lookup misses).

### 8.2 `0047-zookeeper-boot-reject` (boot-reject; symmetric)

The `0044-network-rbac-boot-reject` precedent (`test/fixtures/0044-network-rbac-boot-reject/driver/driver.go`; 220 LoC). A `[zookeeper_proxy, tcp_proxy]` chain whose zookeeper `typed_config` has NO `stat_prefix` → BOTH sides reject at boot (PGV mirror — §6.1). Driver implements `fixture.Driver` + `differential.BootRejectFixture` (`harness.go:340-352`): `BootRejectScript() ""` (inline config); `ExpectedBootErrorSubstring() "stat_prefix"` (case-sensitive; present in the reference's PGV violation text AND in envoy-go's `zookeeper_proxy: stat_prefix is required`). Symmetric mode (NOT `SubjectOnlyBootRejectFixture`). A minimal unused cluster satisfies the zero-cluster boot reject (the `0044` precedent + `reference_network_filter_typeurl_extensions`).

### 8.3 Total fixture-dir count

47 → **49** at 28.1 phase-done (+2). The full 47-dir existing suite is the seam's back-compat regression gate (R1) and re-runs green at the six-gate. No new conformance harness (§2.9).

---

## 9. Behavior-contract delta (the 28.1 bundle; per ADR-0052 atomic landing)

ONE atomic bundle at the 28.1 IMPL final task:

- NEW `### Network filter chain framework — WriteFilter seam (28.1 amendment)` block: the write-direction dispatch + REVERSE order + StopIteration-no-forward (documented-unsupported-by-consumers) + the terminal-originated-writes-only boundary (§3.7) + the write-only-filter boot boundary (§3.6) + the `writeChainConn` composition + zero-write-filter back-compat.
- NEW `### envoy.filters.network.zookeeper_proxy` subsection: request-side semantics (§4.5); the 201-counter roster + creation parity (§4.4); the `<stat_prefix>.zookeeper.` scope; the Prometheus inline-prefix flattening (§7.4); the dynamic per-scheme auth counters; the shallow-decode leniency departure; the dynamic-metadata coverage boundary (AMEND-A9); the `access_log` parse-accept-ignore note; the parsed-not-consumed latency-field note (28.2 forward-pointer).
- Stat table: 136 → **337** (the 201 new rows).
- A forward-pointer note: 28.2 lands the response decoder + correlation consumption + latency-threshold counters + the latency-histogram coverage boundary + the parent-row-28 ROLLUP.

---

## 10. Per-task structure (~18 tasks; the SPEC-anticipated task spine)

The 28.1 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **47** (tail `0045`) + fuzzers **36** + stat surface **136** + DECISIONS tail **ADR-0223** (next-free **ADR-0224**) + `proto.MessageName` TypeURL pinning test + the §3 as-built line anchors, against the live IMPL-session tip | §11 / R6 |
| 2 | `WriteFilter` + `WriteFilterCallbacks` interfaces (`types.go`) + the concrete `*writeCallbacks` impl | §3.1 |
| 3 | Chain classification restructure (`chain.go`: independent type-asserts + `writeFilters` field + dual callback injection + `onDestroy` once-per-instance) + classification/both-filter/destroy-dedupe unit tests | §3.3 |
| 4 | `writeconn.go` `writeChainConn` (forward / StopIteration-no-forward / reverse-order / post-chain-bytes / error propagation) + unit tests incl. two synthetic write filters + a synthetic mutating filter | §3.4 / §3.5 |
| 5 | `handleTerminal` wrap insertion (prefixConn-inner / writeChainConn-outer; zero-write-filter unwrapped) + back-compat unit tests | §3.5 |
| 6 | `zookeeperproxy` package skeleton + `TypeURL` + `NewFactory` + config parse (9 fields + PGV mirrors + the proto→wire mapping + the latency-override map) + parse unit tests | §4.2 / §4.3 |
| 7 | PARSE-REJECT arms + `TestParseRejectConstants_ByteStable` (§6.1 + §6.2 wording finalization) | §6 |
| 8 | `stats.go`: the 201-suffix roster table + `rosterStats` eager creation + `TestCounterRoster_MatchesUpstreamMacro` + the dynamic auth-scheme counter helper | §4.4 / R2 |
| 9 | `decoder.go` part 1: framing + reassembly + the chain-buffer high-water mark + connect/ping/auth/setwatches special-xid parsing | §4.5 |
| 10 | `decoder.go` part 2: data-request opcode dispatch + min-length table + `max_packet_bytes` + the `decoder_error` path + counter increments (incl. flag gating) + the correlation-structure writes | §4.5 / §4.6 |
| 11 | Filter glue: the both-directions `filter` struct + OnData/OnWrite/OnDestroy + multi-read/partial-frame/garbage unit tests | §4.7 |
| 12 | The 7th built-in registration + `bootstrap.go` blank-import + a boot smoke test (a `[zookeeper_proxy, tcp_proxy]` bootstrap boots; the 201 counters exist at 0) | §4.8 |
| 13 | The `.zookeeper.` `name.go` arm + flattening unit tests (incl. the dotted `auth.digest_rq` case + the dot-free-prefix guard) | §7.4 |
| 14 | The 37th fuzzer `FuzzZookeeperRequestDecode` (random bytes → decoder: no panic, no chain-buffer mutation, internal buffer bounded by `max_packet_bytes` accounting) | §15.1 Layer C |
| 15 | The `TCPSink` BackendKind runner plumbing + `0046` driver part 1 (bootstraps + frame-crafting helpers + Drive/MultiListener wiring) | §8.1.1 |
| 16 | `0046` driver part 2 (the StatsAsserter + the 7 arms + deliberate-break recording) — the fixture goes green cross-side | §8.1.2 / §8.1.3 / R4 |
| 17 | `0047-zookeeper-boot-reject` fixture | §8.2 |
| 18 | Completion bundle: BEHAVIOR_CONTRACT 28.1 bundle (§9) + ADR-0221/0222 §Decision/§Consequences bodies in-place (ADR-0044) + STATE.md + ROADMAP sub-row 28.1 `in-progress → done` + the six-gate (incl. the FULL 49-dir differential suite + the 47-dir back-compat gate) | §9 / §15.2 |

### 10.1 ADR-0045 split-gate — SPEC-level re-check (parent D-P1)

Production-LoC estimate against the §3/§4 refined surface (the 26.x accounting basis: production code; fixture drivers + unit tests EXCLUDED):

| Deliverable | Production LoC |
|---|---|
| WriteFilter seam (interfaces + classification + writeconn.go + handleTerminal) | ~150–250 |
| zookeeperproxy config.go (parse + PGV + proto→wire mapping) | ~150–200 |
| zookeeperproxy stats.go (roster table + eager creation + dynamic auth) | ~150–200 |
| zookeeperproxy decoder.go (framing + reassembly + dispatch + counters + correlation) | ~300–450 |
| zookeeperproxy filter glue + doc | ~80–100 |
| builtins + bootstrap.go + name.go arm + TCPSink runner plumbing | ~100–150 |
| The 37th fuzzer | ~60 |
| **Total (production basis)** | **~990–1410** |

**Verdict: fits as ONE sub-phase on the production-LoC basis** (under the ~1500 gate; the ~18-task spine is under the ~25-task gate). The fixture drivers (~700–900 LoC across `0046`+`0047`, the `0043`/`0044` precedents being 637+220) are excluded per the 26.x accounting precedent (27 PLAN: the 522-LoC `0045` driver excluded from the ~180–400 net-new figure). **The 28.1 PLAN remains the FINAL gate-check** (parent D-P1): if the bite-sized TDD decomposition exceeds ~25 tasks, the pre-authorized split axis is **28.1a** (Tasks 1–8, 12–13, 17: seam + config + roster + builtins/name.go + `0047`) / **28.1b** (Tasks 9–11, 14–16: decoder + correlation + `0046` + fuzzer).

---

## 11. SPEC-time empirical-pin block (cross-reference parent §11 + the D-S1 sub-pin)

The 28.1 SPEC does NOT re-execute the parent §11 D28-1..D28-10 pins (resolved once at the parent SPEC against Envoy v1.37.2; inherited here — §1.1). The 28.1-additive pin:

### 11.1 D-S1 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip **`2a525ff`** (the docs-only next-prompt repoint trailing the parent-SPEC squash `a532159` by +2) at this SPEC session. These are the source of the §10 Task-1 first-action gate; the IMPL Task-1 RE-RUNS them against the live IMPL-session tip (tips advance via docs-only commits between sessions; the gate catches drift).

- **Differential fixture-dir count = 47**; numbering tail = **`0045-sni-cluster`** (`ls -d test/fixtures/[0-9]*/ | wc -l` = 47). 28.1 lands `0046` + `0047` → **49**.
- **Fuzzer count = 36** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` — the recipe scoped to `./internal` per the parent §11.10 reviewer advisory). The 28.1 `FuzzZookeeperRequestDecode` is the **37th**.
- **Stat surface = 136** (BEHAVIOR_CONTRACT stat table; the 26.3 rbac rows were the last delta). 28.1 lands +201 → **337**.
- **DECISIONS.md tail = ADR-0223** (the three phase-28 §Context drafts landed at the parent SPEC — `DECISIONS.md:14226` ADR-0221, `:14245` ADR-0222, `:14264` ADR-0223); **next-free ADR-0224**. 28.1 IMPL fills the ADR-0221 + ADR-0222 §Decision/§Consequences bodies IN PLACE (no new ADR number; ADR-0044).
- **As-built framework anchors re-verified** (all parent §4.1 anchors hold at `2a525ff`): `types.go:29-48` (ReadFilter), `types.go:61` (FilterInstanceFactory → NetworkFilter), `terminal.go:18-28` (sealed marker), `terminal.go:42-49` (TerminalFilter.Handle), `chain.go:57-83` (NewChainRuntime classification), `chain.go:127-168` (chainRuntime struct), `chain.go:215-227` (handleTerminal), `chain.go:321-326` (onDestroy), `chain.go:380-385` (Connection.Write — the D-P3 bypass), `prefixconn.go:12-28`, `callbacks.go:16-38`, `manager.go:534-599` (buildNetworkChainFactory) + `:570-581` (boot classification) + `:580` (the write-only default reject), `name.go:88-122` (the wasm permissive arm) + `:226-242` (the rbac arm) + `:243` (the default error), `builtins.go` (6 registrations; rbac closure-capture at `:59`), `bootstrap.go:76-77,87` (network-filter blank-imports), `rbac.go:38` (TypeURL via proto.MessageName) + `:187-198` (newFilterStats via NewCounterIfAbsent), `registry.go:142-171` (NewCounterIfAbsent post-Freeze-permitted).
- **Differential-harness anchors**: `fixture.go:15-52` (Driver), `:70-77` (StatsAsserter), `:122-492` (the BackendKind roster `TCPEcho=0` … `HTTPWasmPerRoute=27` — NO sink kind exists), `:495-498` (BackendKindAware), `:584-589` (MultiListenerDriver); `harness.go:340-352` (BootRejectFixture); `runner_test.go:1048-1050` (the StatsAsserter cross-side-only dispatch); `0043/driver/driver.go:388-461` (the both-sides `/stats/prometheus` scrape mechanics), `:633-637` (the interface assertions).

---

## 12. SPEC-time D-questions — parent resolutions + 28.1-additive PLAN/IMPL questions

### 12.1 Parent D-questions RESOLVED at this SPEC

- **D-P2 (decoder depth + WriteFilter interface composition) — RESOLVED.** Decoder: SHALLOW (framing + xid + opcode + min-length + connect-readonly + auth-scheme; §4.5); the payload-malformed leniency departure recorded (§7.5). Interface: `WriteFilter` = `NetworkFilter` + `OnWrite` + `SetWriteFilterCallbacks` + `OnDestroy`; a both-directions filter receives BOTH callback injections; `OnDestroy` called exactly ONCE per instance (§3.1/§3.3).
- **D-P3 (read-filter writes through the write chain) — RESOLVED: NO.** Terminal-originated writes only (§3.7). Recorded as a BEHAVIOR_CONTRACT framework boundary.
- **D-P5 (creation parity) — RESOLVED: ALL 201 at 28.1** (§4.4). The `0046` arm-6 exists-at-zero assertion is the differential proof.
- **D-P7 (writeChainConn Write return semantics) — RESOLVED: `(len(p), nil)`** on a chain-stopped write (§3.5 item 3); post-chain bytes forwarded; underlying errors propagate as `(0, err)`.
- **D-P8 (name.go arm validation posture) — RESOLVED: SHAPE-based** (`.zookeeper.` segment + dot-free prefix; no allowlist; the wasm-permissive precedent; §7.4).
- **D-P1 (28.1 split gate) — SPEC-level re-check: FITS** as one sub-phase on the production-LoC basis (~990–1410 LoC / ~18 tasks; §10.1). The PLAN performs the FINAL re-check; the pre-authorized 28.1a/28.1b axis stands if it trips.

(Parent D-P4, D-P6, D-P9 are 28.2-owned and untouched here.)

### 12.2 28.1-additive D-questions for PLAN / IMPL resolution

- **D-S28.1-1 (per-opcode min-length table values).** The §4.5 data-request min-length validation needs the per-opcode minimum-frame-length values transcribed from upstream `decoder.cc`. **Resolution at:** IMPL Tasks 9–10 (transcribe + unit-test against the parent §11.4 framing pins). Anticipated: a small `map[int32]int` table; opcodes whose shallow parse needs no payload read use the universal `xid+opcode` 8-byte minimum.
- **D-S28.1-2 (PARSE-REJECT byte-stable wording).** Finalize the §6 arm wording + the `TestParseRejectConstants_ByteStable` table. **Resolution at:** IMPL Task 7. Anticipated prefix: `zookeeper_proxy: `.
- **D-S28.1-3 (the chain-buffer high-water-mark mechanism).** §4.5's "feed only the new bytes" tracking — a `consumed int` on the filter vs the decoder. **Resolution at:** PLAN / IMPL Task 9. Anticipated: a field on the per-connection decoder; a multi-read unit test proves no double-count.
- **D-S28.1-4 (0046 frame-crafting helper shape).** Hand-crafted jute frames as Go byte-slice builders vs hex-literal constants. **Resolution at:** IMPL Task 15. Anticipated: small builder helpers (`connectFrame(readonly bool)`, `dataFrame(xid, opcode int32, payload []byte)`) in the driver package — readable + reusable by the 28.2 `0048` driver.
- **D-S28.1-5 (TCPSink backend close semantics).** Whether the sink backend half-closes or fully closes when the client closes. **Resolution at:** IMPL Task 15. Anticipated: read-until-EOF then close (simplest; no observable difference for a sniffer fixture).

---

## 13. RATIFIED-PENDING items (cross-reference parent §13, scoped to 28.1)

- **R1 (seam back-compat).** Zero-write-filter chains get NO `writeChainConn` wrap → `handleTerminal` byte-identical to today. Ratified by ALL 47 existing fixture dirs staying byte-exact green at the 28.1 six-gate (the seam's regression gate).
- **R2 (creation parity).** The 201-counter roster + the `<stat_prefix>.zookeeper.` scope match upstream name-for-name. Ratified by `TestCounterRoster_MatchesUpstreamMacro` (the 201-suffix golden list) + the `0046` arm-6 exists-at-zero assertions.
- **R3 (passthrough invariant).** zookeeperproxy NEVER drains/mutates the chain buffer, never closes the connection, never returns StopIteration from OnData/OnWrite. Ratified by the `0046` arm-4 connection-survival proof + unit tests asserting the chain buffer is byte-identical before/after OnData.
- **R4 (StatsAsserter liveness).** Every `0046` stat assertion proven live via a recorded deliberate-break (§8.1.3 arm 7; the `reference_differential_asserter_dispatch` discipline).
- **R5 (correlation hand-off).** The two correlation structures are written-but-unread at 28.1 (the 26.1 shaped-but-unwritten precedent). Ratified at 28.2 by the `0048` correlation arms; at 28.1 a unit test asserts the structures are populated by request decode.
- **R6 (counts).** IMPL Task 1 re-pins fuzzers 36→37, fixtures 47→49, stats 136→337, DECISIONS tail ADR-0223 (next-free ADR-0224) against the live IMPL-session tip (§11.1 recipes).
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` zookeeper lines match the reference's flat shape `envoy_<prefix>_zookeeper_<counter>{}`. Ratified intrinsically by the §8.1.2 both-sides-prometheus-scrape mechanics (a name-shape mismatch makes every value assertion miss).

---

## 14. BEHAVIOR_CONTRACT.md edit bundle

ONE atomic bundle at IMPL Task 18, per ADR-0052: the §9 enumerated edits (the WriteFilter-seam framework block + the zookeeper_proxy subsection + the 136→337 stat table delta + the 28.2 forward-pointer). Departure/coverage-boundary records folded in per §7.5.

---

## 15. Test surface + 28.1 IMPL acceptance checklist

### 15.1 Test surface (per parent §14, scoped to 28.1)

- **Layer A — framework unit tests** (`internal/filter/network/`): classification (read/write/both/terminal; both-filter dual-injection; OnDestroy-once); reverse-order dispatch (two synthetic write filters); `writeChainConn` (forward / stop-no-forward / post-chain-bytes-with-mutating-filter / error propagation / endStream-false); `handleTerminal` wrap composition (all four conn shapes) + zero-write-filter back-compat.
- **Layer A — zookeeperproxy unit tests**: config parse (all 9 fields; every §6 PGV-mirror arm; the proto→wire mapping; duplicate-override reject); the roster (201 suffixes; eager creation; idempotent shared-prefix creation; flag-false counters exist-at-zero); request decode (connect/connect-readonly/ping/auth-scheme/setwatches special xids; data-request dispatch incl. digit-suffixed opcodes; garbage/oversized/unknown-opcode → decoder_error; partial-frame reassembly across reads; the high-water mark no-double-count; flag gating; correlation-structure population — R5); the chain buffer never drained (R3).
- **Layer A — stats unit tests** (`internal/stats/`): the `.zookeeper.` arm (plain counters; the dotted `auth.digest_rq` case; dot-free-prefix guard; non-matching names still error).
- **Layer C — fuzz**: the 37th fuzzer `FuzzZookeeperRequestDecode` (arbitrary bytes → decoder: no panic; chain buffer unmutated; internal reassembly buffer never exceeds `max_packet_bytes` + one frame).
- **Layer D — differential**: `0046` (cross-side StatsAsserter; 7 arms) + `0047` (boot-reject) + the FULL 47-dir back-compat suite (R1) → 49/49 green.
- **Layer E — race**: `go test -race -short` across `internal/filter/network/...` + `internal/stats/` (the shared roster counters under concurrent connections).

### 15.2 Six-gate checklist (per the 22/24/25/26/27 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (49 dirs incl. the 47-dir back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 28 touches no HTTP path; HCM's chain has zero write filters). All outputs quoted into PROGRESS.md (run honestly). Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

### 15.3 28.1 IMPL acceptance checklist

1. The `WriteFilter` seam lands per §3 (interfaces + classification + reverse dispatch + `writeChainConn` + the §3.6/§3.7 boundaries); `manager.go`/`tcp_proxy`/HCM untouched; all 47 existing fixtures byte-exact green (R1).
2. The `zookeeperproxy` package lands per §4 (config parse + the 201-counter eager roster + the shallow request decoder + the correlation structures + the dynamic auth counters + the no-op OnWrite stub).
3. The 7th built-in + `bootstrap.go` blank-import + the `.zookeeper.` name.go arm land (§4.8/§7.4).
4. Fixtures `0046` + `0047` green (incl. the `TCPSink` BackendKind); the 37th fuzzer lands; counts: fixtures 47→49, fuzzers 36→37, stats 136→337 (R6).
5. ADR-0221 + ADR-0222 §Decision/§Consequences bodies land in place (DECISIONS.md tail STAYS ADR-0223; no new number consumed); the BEHAVIOR_CONTRACT 28.1 bundle lands (§14).
6. Six gates green (§15.2); STATE.md advanced; ROADMAP sub-row 28.1 `in-progress → done`; parent row 28 STAYS `in-progress` (the ROLLUP is 28.2's); next-prompt.txt rewritten for the 28.2-SPEC cold-start.

---

## 16. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 28.1 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x precedent); parent row 28 STAYS `in-progress`; 28.2 STAYS `planned`. STATE.md advances to lifecycle-state 2-for-28.1-PLAN with `next-skill = superpowers:writing-plans` scoped to the **28.1 PLAN** (`docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 28.1-PLAN cold-start. Per `feedback_execution_style` the 28.1 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies.

---

## Appendix A — Cross-references to parent SPEC

| 28.1 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (28.1 detail) | refines |
| §1.1 AMENDs | parent §1.1 (A1–A11) | inherits the 28.1-load-bearing subset |
| §1.2 Additive pins | — | NEW (TCPSink; scrape mechanics; write-only boot posture; OnWrite stub) |
| §2 Non-purposes | parent §2 + §3.2 | refines (28.1-scoped) |
| §3 WriteFilter seam | parent §4.1 (sketch → production signatures) | refines; resolves D-P2/D-P3/D-P7 |
| §4 zookeeperproxy | parent §4.2 + §11.3/§11.4/§11.5 | refines; resolves D-P2/D-P5 |
| §5 Proto roster | parent §5 | INHERITS verbatim |
| §6 PARSE-REJECT | parent §6.1/§6.2/§6.3 | refines (28.1 arms; wording at IMPL) |
| §7 Stat surface | parent §7 | refines; resolves D-P5/D-P8 |
| §8 Fixtures | parent §8.1/§8.2/§8.4 | refines (arms + TCPSink + scrape mechanics) |
| §9 Behavior contract | parent §9 (28.1 bundle) | refines |
| §10 Tasks + split-gate | parent §11.9 + §15 (28.1 row) | NEW (task spine); D-P1 SPEC-level re-check |
| §11 Empirical pins | parent §11 (D-S1 sub-pin only) | inherits; adds the baseline/anchor re-pin |
| §12 D-questions | parent §12 | resolves D-P1(partial)/P2/P3/P5/P7/P8; adds D-S28.1-1..5 |
| §13 RATIFIED-PENDING | parent §13 (R1–R7) | scoped to 28.1 |

## Appendix B — Phase 28.1 ADR landings summary

- **ADR-0221** (the `network.WriteFilter` seam) — §Context drafted at the parent SPEC (`DECISIONS.md:14226`); §Decision + §Consequences bodies land at 28.1 IMPL Task 18 per ADR-0044. This SPEC's §3 is the body's blueprint: the interfaces (§3.1), classification (§3.3), reverse dispatch (§3.4), `writeChainConn` + D-P7 (§3.5), the write-only boot boundary (§3.6), the terminal-originated-writes-only boundary / D-P3 (§3.7). CONSUMES the ADR-0213 §Decision item 8 API-revision allowance.
- **ADR-0222** (the `zookeeper_proxy` filter, request side) — §Context drafted at the parent SPEC (`DECISIONS.md:14245`); §Decision + §Consequences bodies land at 28.1 IMPL Task 18. This SPEC's §4 + §7 + §8 are the body's blueprint: the package (§4.1–§4.8), the roster + D-P5 (§4.4), the shallow decoder + D-P2 (§4.5), the correlation structures (§4.6), the name.go arm + D-P8 (§7.4), the fixtures + the TCPSink pin (§8).
- DECISIONS.md tail STAYS **ADR-0223** at 28.1 phase-done (no new ADR number consumed); next-free **ADR-0224**. ADR-0223's body lands at 28.2.
