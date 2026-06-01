# Phase 28 — Network `WriteFilter` seam + `zookeeper_proxy` (parent master SPEC)

> **For agentic workers:** this is the PARENT SPEC for the phase-28 2-way pre-split (28.1 / 28.2). It is NOT directly executable. Per the phase-22 / phase-24 / phase-25 / phase-26 parent-row precedent, each sub-phase lands its own SPEC → PLAN → IMPL in dedicated sessions. This parent SPEC: (1) resolves the BRAINSTORM §10 D28-1..D28-10 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (§11), (2) formalizes the 2-way split surface-mapping + per-sub-phase scope boundaries (§3), and (3) anchors the phase-28 ADR §Context drafts ADR-0221..ADR-0223 (§10). The next session, per BOOTSTRAP §5 + the per-sub-phase precedent, authors the **28.1 SPEC**.

**Goal:** Land the network `WriteFilter` seam (the framework's write-direction half, deferred at 26.1 with an explicit API-revision allowance) and `envoy.filters.network.zookeeper_proxy` — a passive both-direction ZooKeeper-protocol observability sniffer — at both-direction COUNTER parity, across a 2-way direction-progressive pre-split.

**Architecture:** The existing `internal/filter/network/` framework gains a `WriteFilter` interface (`OnWrite(buf *Buffer, endStream bool) Status`) + chain classification extended to read / write / both / terminal + a REVERSE-chain-order write dispatch, delivered by wrapping the downstream `net.Conn` handed to `handleTerminal` in a `writeChainConn` (the `prefixConn` precedent) — `TerminalFilter.Handle` signature UNCHANGED, `tcp_proxy`/HCM untouched. A NEW `internal/filter/network/zookeeperproxy/` package implements BOTH `ReadFilter` and `WriteFilter`: config parse of the 9-field proto, the 201-counter eager stat roster under scope `<stat_prefix>.zookeeper.`, the request decoder (28.1) and response decoder + xid correlation + latency-threshold counters (28.2). Cross-side `StatsAsserter` per-opcode counter parity is the load-bearing differential proof (the filter never mutates bytes — a body differential is vacuous).

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/stats/` (06.1); `internal/filter/network/` (26.1/26.2/27); the differential harness + `StatsAsserter`. ZERO new third-party `go.mod` dependencies (the jute decode is plain `encoding/binary` big-endian reads).

**Authored:** 2026-06-01. **Empirical-pin probe date:** 2026-06-01.

---

## 1. Mission summary

Phase 28 is the **THIRD §9 Network-filters-family row** (after the phase-26 family-parent and the phase-27 `sni_cluster` flat row) and a parent pre-split row per ADR-0106 (the project's FOURTH BRAINSTORM-time pre-split, after 22/25/26). It delivers two structurally coupled things:

1. **The `network.WriteFilter` seam** — the framework's write-direction (upstream→downstream) dispatch half, absent since 26.1 (read-filter-only scope per 26 BRAINSTORM Q4), deferred WITH an explicit API-revision allowance written into ADR-0213 exactly for this moment. Phase 28.1 CONSUMES that allowance: zookeeper_proxy is consumer #1; mongo_proxy is the anticipated consumer #2.
2. **`envoy.filters.network.zookeeper_proxy`** — a passive observability sniffer that decodes the ZooKeeper client protocol (jute framing) in BOTH directions and emits per-opcode counters. It is the family's first stats-PRIMARY filter (`echo`/`direct_response`/`sni_cluster` have zero stats; `rbac_network` has 4): its entire purpose IS its stat surface, making the cross-side `StatsAsserter` differential the load-bearing proof rather than a supplementary one.

The design was settled at BRAINSTORM via a 4-question user dialogue (`docs/envoy-go/phases/28-network-filter-zookeeper-proxy/BRAINSTORM.md` §2): Q0 subject = `zookeeper_proxy`; Q1 both-direction COUNTER parity + 2-way pre-split (histograms stay deferred per ADR-0060); Q2 upstream-faithful `WriteFilter` + `writeChainConn` conn-wrap delivery; Q3 hermetic synthesized-byte fixtures + cross-side `StatsAsserter`. This SPEC does NOT re-litigate those decisions; it executes the empirical pins they deferred and formalizes the surface-mapping.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D28-1..D28-10 scrape against Envoy v1.37.2 + go-control-plane v1.32.4 + the live reference image CONFIRMED the SPEC-blocking hypothesis (D28-1) and REFINED/REFUTED several others. The load-bearing amendments to the BRAINSTORM design, each carried into the relevant §§ below:

- **AMEND-A1 (D28-1/D28-3 — stat scope ORDER REVERSED).** The emitted stat scope is **`<stat_prefix>.zookeeper.<counter>`** (e.g. `zkprobe.zookeeper.getdata_rq`), NOT the BRAINSTORM's `zookeeper.<stat_prefix>.*` hypothesis. Upstream `config.cc:27` builds `fmt::format("{}.zookeeper", proto_config.stat_prefix())` and pools all macro counters under it. Confirmed live: the booted reference image emits `zkprobe.zookeeper.connect_rq` etc. See §7.1.
- **AMEND-A2 (D28-3 — the counter roster is 201 EAGER macro counters, not ~25–30/direction).** Upstream's `ALL_ZOOKEEPER_PROXY_STATS(COUNTER)` macro (`filter.h:30-231`) declares **201 counters**, ALL created eagerly at config load (`POOL_COUNTER_PREFIX`; confirmed live — the booted image exposes the full roster at 0 before any traffic): 4 plain (`decoder_error`, `request_bytes`, `response_bytes`, `watch_event`) + 28 `<op>_rq` + 29 `<op>_rq_bytes` + 28 `<op>_decoder_error` + 28 `<op>_resp` + 28 `<op>_resp_bytes` + 28 `<op>_resp_fast` + 28 `<op>_resp_slow`. The three `enable_per_opcode_*` flags + `enable_latency_threshold_metrics` gate INCREMENTS, never creation. Project stat surface goes 136 → **337** at family-row-done — the BRAINSTORM's "~190–240" total was an underestimate. See §7.
- **AMEND-A3 (D28-3 — roster asymmetries).** The per-opcode roster is NOT uniform: `connect` has `connect_readonly_rq`/`connect_readonly_rq_bytes` variants (rq-side only); there is **NO `auth_rq`** in the macro (auth/SetAuth requests are counted via LAZY dynamic per-scheme counters `auth.<scheme>_rq` through a `StatNameSet`, falling back to `unknown_scheme_rq`) yet `auth_resp`/`auth_resp_bytes`/`auth_resp_fast`/`auth_resp_slow` ARE in the macro. envoy-go mirrors the macro roster exactly + the dynamic per-scheme auth counters via the `NewCounterIfAbsent` dynamic-name convention (the rbac per-policy precedent). See §7.2.
- **AMEND-A4 (D28-1 Prometheus probe — NO tag extraction; the name.go arm is INLINE-PREFIX, not a tag-extractor).** Reference Envoy's `/stats/prometheus` emits zookeeper stats as FLAT names `envoy_<stat_prefix>_zookeeper_<counter>{}` with an EMPTY label set (probed live with two different stat_prefix values — the prefix lands in the metric NAME, never as a label). The BRAINSTORM's "prom tag-extractor arm (the `.rbac.` precedent)" is REFUTED: the correct precedent is the phase-15 bandwidth_limit INLINE-PREFIX detection (ADR-0138 — dot→underscore + `envoy_` prefix, NO label promotion). envoy-go's `internal/stats/name.go` default branch errors on unrecognized prefixes (`name.go:243`), so a NEW `.zookeeper.` inline-prefix arm IS required — but it is the ADR-0138 shape, not the ADR-0218 shape. See §7.4.
- **AMEND-A5 (D28-4 — connect is distinguished by XID SNIFFING; the special-xid set is larger).** The decoder distinguishes a connect request by `xid == 0` (`XidCodes::ConnectXid`), per-packet — there is NO first-packet state machine. The full special-xid set is `Connect=0`, `Watch=-1`, `Ping=-2`, `Auth=-4`, `SetWatches=-8` (the BRAINSTORM listed only −1/−2/−4). See §11.4.
- **AMEND-A6 (D28-4 — wire opcodes ≠ proto enum).** The decoder's wire `OpCodes` enum has **26 values with GAPS and a negative value** (`Ping=11` not 10; `CreateTtl=21`; `Close=-11`; `SetAuth=100`; `SetWatches=101`; `GetEphemerals=103`; `GetAllChildrenNumber=104`; `SetWatches2=105`; `AddWatch=106`), while the proto's `LatencyThresholdOverride.Opcode` enum is 27 contiguous values 0..26 in a DIFFERENT order. The two are connected by an explicit mapping table (upstream `filter.h:311-339` `opcodeMap`). envoy-go needs both enums + the mapping. See §5.3 + §11.4.
- **AMEND-A7 (D28-4 — TWO correlation structures, not one map).** Upstream keeps `requests_by_xid_` (data requests, xid > 0: single entry per xid, overwritten on insert, ERASED on response lookup) AND `control_requests_by_xid_` (control requests — connect/ping/auth/setwatches: a FIFO **queue** per control xid, since control xids repeat). A response whose xid has no pending request → `decoder_error`. The BRAINSTORM's "the per-connection xid→opcode tracking map" (singular) is refined to this two-structure shape. See §11.4.
- **AMEND-A8 (D28-4/D28-5 — the decoder owns its OWN partial-packet buffers; chain bytes always pass through untouched).** Upstream's decoder never drains/modifies the chain buffer (passthrough is unconditional — `onData`/`onWrite` ALWAYS return `Continue`, the connection is NEVER closed by the filter, decode failure only logs at debug + counts `decoder_error`). Partial packets are reassembled in the decoder's own `zk_filter_read_buffer_`/`zk_filter_write_buffer_` (prepend + measure-full-packets + copy-out-remainder). On a decode failure the REMAINING packets in the current buffer are abandoned (no resync), but later buffers keep decoding. envoy-go's zookeeperproxy mirrors this: `OnData`/`OnWrite` always `Continue`, never drain the chain `Buffer` (beyond reading), and keep internal per-direction reassembly buffers. See §11.5.
- **AMEND-A9 (D28-6 — dynamic metadata DEFERRED; refines the BRAINSTORM's anticipated-MIRROR).** Upstream emits per-message dynamic metadata under `envoy.filters.network.zookeeper_proxy` (keys `opname`/`path`/`watch`/`bytes`/`version`/`create_type`/… per event, CLEARED at the top of every `onData`/`onWrite`). Mirroring it requires FULL per-opcode payload parsing (path strings, versions, ACLs, multi-transactions — the bulk of upstream's ~1100-line decoder.cc), and the emission has ZERO observable surface in envoy-go (no in-repo consumer of connection-level dynamic metadata, invisible to the differential, cleared per-message). Per the YAGNI / every-surface-exercised discipline + the ADR-0060 deferral precedent, **dynamic metadata is DEFERRED** with a BEHAVIOR_CONTRACT coverage-boundary record; the decoder does header-level + length-validation ("shallow") decode only. The ADR-0217 bucket stays available; a future consumer (or mongo_proxy) can lift this. See §2.1 + §11.6.
- **AMEND-A10 (D28-2/D28-8 — latency-threshold PGV + boot-reject arms).** `LatencyThresholdOverride.threshold` is PGV-REQUIRED **and** gte 1ms; `.opcode` is PGV `defined_only`; `default_latency_threshold` is optional but gte 1ms when set; a DUPLICATE opcode override is an upstream config-load `EnvoyException` (a boot-reject parity arm for 28.2). Threshold comparison is **`latency <= threshold` → fast** (inclusive). Overrides are keyed by the WIRE opcode int (via the proto→wire mapping). See §6.3 + §11.7.
- **AMEND-A11 (D28-7 — write-path semantics pinned).** Upstream write filters dispatch in **REVERSE config order** (LIFO: `addWriteFilter` front-inserts; `onWrite` iterates front→back — config `[A,B,C]` ⇒ write order `C→B→A`). `StopIteration` on the write path means "this write does not proceed" — the data is left in the caller's buffer, nothing reaches the transport, and there is NO `continueWriting()` (upstream source comment: "we don't support restart/continue on the write path"). Writes enter the chain via `ConnectionImpl::write` → `filter_manager_.onWrite()` BEFORE the transport socket — exactly the seam the `writeChainConn` conn-wrap mirrors. zookeeper_proxy registers via `addFilter` (combined read+write, one instance) and its `onWrite` ALWAYS returns `Continue`. See §4.1 + §11.8.

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0220**; next-free **ADR-0221**. This SPEC anchors the phase-28 §Context drafts ADR-0221..ADR-0223 (three ADRs, locked at BRAINSTORM §7; §Decision/§Consequences bodies land at each sub-phase IMPL per ADR-0044). No ADR number is consumed at SPEC time beyond the §Context drafts (a SPEC drafts §Context only). The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. All ten D28 pins are resolved at this session (§11); the remaining open items are sub-phase-SPEC/PLAN D-questions (§12), not empirical pins.

---

## 2. Scope — non-purposes + REUSES-not-consumed

### 2.1 Non-purposes (deferred; per BRAINSTORM §8 + AMEND-A9)

- **Latency HISTOGRAMS** (`connect_response_latency`, `<opname>_latency`, `unknown_opcode_latency` — upstream's lazy StatNameSet histogram family) — deferred per ADR-0060 (project-wide histogram deferral); BEHAVIOR_CONTRACT coverage-boundary record at 28.2. The deterministic latency-threshold COUNTERS (`*_resp_fast`/`*_resp_slow`) are in scope at 28.2 — they deliver the SLO-style signal using only counter machinery.
- **Dynamic-metadata emission** (namespace `envoy.filters.network.zookeeper_proxy`, keys `opname`/`path`/`watch`/…) — DEFERRED per AMEND-A9 (zero observable surface; requires deep payload parsing). BEHAVIOR_CONTRACT coverage-boundary record at 28.1.
- **Deep per-opcode payload parsing** (path strings, ACLs, versions, multi-transaction recursion) — deferred WITH the metadata it exists to serve. The 28.1 decoder is "shallow": framing + xid + opcode + length validation + counter dispatch. Consequence: a packet with a valid header but malformed PAYLOAD counts as `<op>_rq` on envoy-go but `decoder_error` upstream — recorded as an envoy-go-lenient departure (the fixture corpus contains no such packets; see §8.1).
- **The `access_log` proto field** — upstream `[#not-implemented-hide:]` AND grep-confirmed completely unread by upstream's filter/config code (§11.2). Mirrored as parse-accept-ignore (NOT a reject — upstream accepts and ignores it).
- **WriteFilter halt/buffer semantics** (`StopIteration` on the write path + `injectWriteDataToFilterChain` resume) — the seam pins StopIteration as "the write does not proceed" (upstream parity) but NO production filter may return it at 28.x (documented-unsupported-by-consumers; mongo fault-delay is the anticipated first real consumer). Unit-tested at the framework level only.
- **Real-ZooKeeper-server integration fixtures** — out of scope; hermetic synthesized bytes only (Q3).
- **SASL / auth-credential deep decode** — the filter counts auth requests (per-scheme dynamic counters) but never decodes credential payloads (upstream parity — upstream also only reads the scheme string).
- **The remaining protocol proxies** (`redis`, `mongo`, `kafka_broker`, `thrift`) — each its own future family phase. `mongo_proxy` is the natural next (consumer #2 of the WriteFilter seam).

### 2.2 REUSE-by-absence: no per-route surface

Network filters carry no `typed_per_filter_config` surface (phase-26 parent SPEC §2.2 confirmation; re-confirmed by absence — the zookeeper_proxy proto has no `*PerRoute` message, §11.3). The ADR-0125 roster is untouched.

### 2.3 REUSES (not new primitives)

- `internal/filter/network/` (26.1/26.2/27) — the ReadFilter chain, drainable `Buffer`, freeze-after-boot `*Registry`, `chainRuntime`, `prefixConn` (the conn-wrap precedent the `writeChainConn` mirrors), `builtins.RegisterBuiltins`.
- `internal/stats/` (06.1) — `*stats.Registry` counters (`NewCounterIfAbsent` for the dynamic per-scheme auth names); `internal/stats/name.go` Prometheus flattening (the new `.zookeeper.` inline-prefix arm follows the ADR-0138 bandwidth_limit shape).
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; UNTOUCHED by 28 (its `connection.write`-equivalent — writing to the downstream `net.Conn` it owns — is what the `writeChainConn` intercepts).
- The differential harness + `fixture.StatsAsserter` (+ the `reference_differential_fixture_dispatch_constraint` + `reference_differential_asserter_dispatch` memory constraints).
- `envoy.extensions.filters.network.zookeeper_proxy.v3` proto bindings — already vendored (go-control-plane v1.32.4); blank-import added to `internal/bootstrap/bootstrap.go` (echo/sni_cluster precedent).
- The freeze-after-boot registry discipline (ADR-0072/0079), the two-step factory (ADR-0079), the iteration-status protocol (ADR-0038/0213), single-goroutine-per-connection (ADR-0071 spirit), atomic landing + six-gate (ADR-0052), byte-stable PARSE-REJECT wording (ADR-0080).
- **NOT consumed:** `internal/dynamicmetadata/` (the ADR-0217 bucket — stays shaped-but-unwritten by zookeeper per AMEND-A9); the ADR-0219 upstream-cluster-override seam (zookeeper never overrides routing).

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q1; 28.1 sizing caveat pinned here

The 2-way DIRECTION-PROGRESSIVE pre-split was settled at BRAINSTORM Q1 (ROADMAP rows 28.1/28.2 already `planned`). No SPEC-time re-decision. The D28-9 envelope (§11.9 + §15) confirms 28.2 fits the ADR-0045 gate comfortably; **28.1 straddles it** (~1150–1500 production LoC / ~16–20 tasks, dominated by the 201-counter roster + the decoder + two fixture dirs). The 28.1 PLAN MUST re-check the gate; if it trips, the pre-authorized split axis is **28.1a** (WriteFilter seam + config parse + eager counter roster + `0047` boot-reject fixture) / **28.1b** (request decoder + xid maps + `0046` cross-side fixture + fuzzer) — recorded as D-P1 (§12), the 26.3 D-P1 precedent. Resolved as a 28.1-PLAN decision, NOT a parent-SPEC blocker.

### 3.1 Split surface-mapping table (per phase-22/25/26 §3.1 precedent)

| Surface element | 28.1 | 28.2 |
|---|---|---|
| `network.WriteFilter` interface + chain classification (read/write/both/terminal) | **lands** | — |
| REVERSE-order write dispatch + `writeChainConn` conn-wrap delivery | **lands** | — |
| Write-path StopIteration posture (no-forward; documented-unsupported-by-consumers) | **lands** | — |
| `internal/filter/network/zookeeperproxy/` package + TypeURL + config parse (9 fields) | **lands** | — |
| The 201-counter EAGER roster creation under `<stat_prefix>.zookeeper.` | **lands (full roster)** | — |
| Request decoder (framing + xid sniffing + opcode dispatch + min-length validation) | **lands** | — |
| The two xid→opcode correlation structures (data map + control queues) | **lands (written)** | consumed |
| Per-opcode `*_rq` (+ `connect_readonly_rq`) increments + `request_bytes` + `decoder_error` | **lands** | — |
| Flag-gated `*_rq_bytes` + per-opcode `*_decoder_error` increments | **lands** | — |
| Dynamic per-scheme `auth.<scheme>_rq` counters (`NewCounterIfAbsent`) | **lands** | — |
| Response decoder in `OnWrite` (response framing + connect-response special + watch events) | — | **lands** |
| Per-opcode `*_resp` + `response_bytes` + `watch_event` + flag-gated `*_resp_bytes` increments | — | **lands** |
| Latency measurement + `*_resp_fast`/`*_resp_slow` threshold counters + override map | — | **lands** |
| 7th `builtins.RegisterBuiltins` registration + `bootstrap.go` blank-import | **lands** | — |
| `internal/stats/name.go` `.zookeeper.` INLINE-PREFIX arm (ADR-0138 shape, AMEND-A4) | **lands** | — |
| Differential fixtures | +2 (`0046-zookeeper-requests` cross-side, `0047-zookeeper-boot-reject`) | +1 (`0048-zookeeper-responses` cross-side) |
| New fuzzers | +1 (request-decoder, 37th) | +1 (response-decoder, 38th — D-P6) |
| BEHAVIOR_CONTRACT bundle | 28.1 bundle (subsection + departures + metadata coverage boundary) | 28.2 bundle (+ histogram coverage boundary) |
| Anticipated ADRs | 0221, 0222 | 0223 |
| ROADMAP | 28.1 `planned → in-progress` at 28.1 SPEC; `→ done` at 28.1 IMPL | 28.2 same; **parent row 28 ROLLUP `→ done` ATOMICALLY with 28.2** |

### 3.2 Per-sub-phase scope detail

**28.1 `network-filter-write-seam-and-zookeeper-requests`** — (a) the **`network.WriteFilter` seam** in the existing `internal/filter/network/` package: the `WriteFilter` interface (`OnWrite(buf *Buffer, endStream bool) Status` + `SetWriteFilterCallbacks` analogue + `OnDestroy` shared via `NetworkFilter`), chain classification extended so a registered filter may satisfy `ReadFilter`, `WriteFilter`, or both (upstream `Network::Filter` parity — one instance, both directions), the write chain run in REVERSE chain order (AMEND-A11), delivery via `writeChainConn` wrapping the downstream `net.Conn` handed to `handleTerminal` (its `Write` runs the write chain then forwards to the real conn; for a chain with zero write filters it is a transparent passthrough → byte-identical to today), `StopIteration`-on-write = the bytes are NOT forwarded (upstream parity) + documented-unsupported-by-consumers; `TerminalFilter.Handle` signature UNCHANGED, `tcp_proxy`/HCM untouched, all existing fixtures byte-exact (the seam's back-compat gate). CONSUMES the ADR-0213 API-revision allowance (ADR-0221). (b) The **`internal/filter/network/zookeeperproxy/` package, request side**: TypeURL via `proto.MessageName` (§5.1), config parse of the 9-field proto (`stat_prefix` REQUIRED → boot-reject; `max_packet_bytes` default 1 MiB; the three `enable_per_opcode_*` flags; the latency fields parsed + validated at 28.1 but consumed at 28.2; `access_log` parse-accept-ignore), the 201-counter EAGER roster creation (creation parity per AMEND-A2 — response-side counters exist-at-zero until 28.2), the shallow request decoder (4-byte BE length prefix + xid sniffing + opcode dispatch + min-length table + `max_packet_bytes` check + internal reassembly buffer), the two correlation structures (written at 28.1, consumed at 28.2), per-opcode `*_rq` + `request_bytes` + `decoder_error` + flag-gated `*_rq_bytes`/`*_decoder_error` increments, the dynamic per-scheme auth counters; `OnNewConnection` = no-op `Continue` (sticky-halt constraint); `OnData`/`OnWrite` ALWAYS `Continue` (AMEND-A8; the 28.1 `OnWrite` body is a stub that feeds the internal write-reassembly buffer or no-ops — D-P7 pins). (c) Registration as the **7th built-in** + the `zookeeper_proxy/v3` blank-import in `bootstrap.go`. (d) The **`.zookeeper.` inline-prefix arm** in `internal/stats/name.go` (AMEND-A4). (e) Fixtures **`0046-zookeeper-requests`** (cross-side; §8.1) + **`0047-zookeeper-boot-reject`** (§8.2). (f) The **request-decoder fuzzer** (37th). (g) ADR-0221 + ADR-0222 §Decision/§Consequences bodies; BEHAVIOR_CONTRACT 28.1 bundle; STATE/ROADMAP advance (sub-row 28.1 `→ done`).

**28.2 `network-filter-zookeeper-responses-and-latency`** — (a) the **response decoder** in `OnWrite`: response framing (length + xid + zxid + error), the connect-response special framing (no zxid/error), watch-event handling (xid −1 → `watch_event`, NOT correlated), xid correlation against the 28.1 structures (data map erase-on-lookup; control FIFO queues; unknown xid → `decoder_error`), per-opcode `*_resp` + `response_bytes` + flag-gated `*_resp_bytes` increments. (b) **Latency measurement** (request-timestamp → response) + the `enable_latency_threshold_metrics` fast/slow counters (`<=` → fast per AMEND-A10; `default_latency_threshold` 100 ms default; `latency_threshold_overrides` keyed by wire opcode via the proto→wire mapping; duplicate-override → boot-reject). (c) Fixture **`0048-zookeeper-responses`** (cross-side; §8.3) with DETERMINISTIC threshold arms (1 h → all fast; 1 ms → all slow). (d) The **response-decoder fuzzer** (38th — D-P6). (e) ADR-0223 §Decision/§Consequences body; BEHAVIOR_CONTRACT 28.2 bundle (incl. the histogram coverage boundary); the **parent-row-28 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 28.2 per the 18/19/22/24/25/26 precedent) + the six-gate.

---

## 4. Framework primitives — 1 framework-seam EXTENSION (28.1) + 1 NEW filter package (28.1/28.2)

### 4.1 EXTENSION: the `network.WriteFilter` seam (ADR-0221; lands at 28.1)

In the existing `internal/filter/network/` package (NOT a new package). The as-built anchor points this extends (verified this session):

- `internal/filter/network/types.go:29-48` — `ReadFilter` (embeds `NetworkFilter`); `types.go:61` — `FilterInstanceFactory func() NetworkFilter` (already returns the GENERAL `NetworkFilter`, so a both-directions filter needs NO factory-signature change).
- `internal/filter/network/terminal.go:18-28` — the sealed `NetworkFilter` marker + `Marker` embeddable; `terminal.go:42-49` — `TerminalFilter.Handle(ctx, net.Conn)` (UNCHANGED).
- `internal/filter/network/chain.go:57-83` — `NewChainRuntime` classification switch (extended: a filter may be `ReadFilter`, `WriteFilter`, both, or `TerminalFilter`); `chain.go:215-227` — `handleTerminal` (the conn-wrap insertion point — it already wraps with `prefixConn` for buffered prefixes; the `writeChainConn` wraps OUTSIDE/AROUND that, so the terminal sees `writeChainConn(prefixConn(conn))` or `writeChainConn(conn)`).
- `internal/filter/network/prefixconn.go:12-28` — the `prefixConn` precedent (embed `net.Conn`, override one method).

The seam's pinned shape:

1. **`WriteFilter` interface**: `OnWrite(buf *Buffer, endStream bool) Status` + `SetWriteFilterCallbacks(cb WriteFilterCallbacks)` + (via `NetworkFilter` embedding) the sealed marker. `OnDestroy` stays on `ReadFilter`; a write-only filter gets its own `OnDestroy` — exact interface composition is a 28.1-SPEC refinement (D-P2). A filter implementing BOTH interfaces is ONE instance seeing both directions (upstream `addFilter` parity, AMEND-A11).
2. **`WriteFilterCallbacks`**: minimal at 28.1 — a `Connection() Connection` accessor (zookeeper needs nothing else; upstream's `WriteFilterCallbacks` carries `injectWriteDataToFilterChain` + `disableClose`, both deferred under the API-revision allowance).
3. **Chain classification**: `NewChainRuntime` classifies each `NetworkFilter` into the read prefix, the write set, and the optional trailing terminal. A both-directions filter appears in BOTH the read prefix and the write set (same instance).
4. **REVERSE-order write dispatch** (AMEND-A11): the write chain iterates the registered write filters in REVERSE config-chain order. For `[zookeeper_proxy, tcp_proxy]` there is one write filter, so order is trivially `[zookeeper_proxy]`; the rule is pinned for the multi-write-filter future (mongo).
5. **`writeChainConn` delivery**: `handleTerminal` wraps the conn it hands to `TerminalFilter.Handle` in a `writeChainConn` IFF the chain has ≥1 write filter (zero write filters → no wrap → byte-identical to today, the back-compat invariant). `writeChainConn.Write(p)` builds a `*Buffer` over `p`, runs the write chain (reverse order), and — if every filter returned `Continue` — forwards the (unmodified, since zookeeper never mutates) bytes to the wrapped conn. `StopIteration` → the bytes are NOT forwarded (upstream `ConnectionImpl::write` early-return parity); the `Write` call reports success to the terminal (the terminal cannot distinguish — D-P7 pins the exact return-value semantics).
6. **Pure-read-path writes**: a ReadFilter writing via `Connection().Write` (echo, direct_response) does NOT pass through the write chain at 28.1 (those writes happen before any terminal handoff; upstream's model routes ALL connection writes through the chain, but envoy-go's read-filter `Connection().Write` is a different code path — `chain.go:380-385`). Pinned as a 28.1 KNOWN boundary: the write chain observes TERMINAL-originated writes only. This is sufficient for zookeeper (whose chain is `[zookeeper_proxy, tcp_proxy]` — all upstream→downstream bytes come from tcp_proxy) and is recorded in BEHAVIOR_CONTRACT; lifting it (routing `connection.Write` through the write chain too) is deferred under the API-revision allowance until a consumer needs it (D-P3).

### 4.2 NEW: `internal/filter/network/zookeeperproxy/` (ADR-0222 + ADR-0223; lands across 28.1/28.2)

Go package `zookeeperproxy` (single-token-joined per the `directresponse`/`snicluster` precedent). Implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection). Anticipated layout (28.1/28.2 SPECs/PLANs finalize the file split): `config.go` (parse + validation + the latency-override map), `stats.go` (the 201-counter roster table + eager creation + the dynamic auth counters), `decoder.go` (framing + request decode at 28.1; response decode at 28.2), `filter.go` (the ReadFilter/WriteFilter glue), `zookeeperproxy.go` (TypeURL + `New` factory).

### 4.3 Framework-delta accretion shape

Phase 28 continues framework GROWTH: the WriteFilter seam is the framework's THIRD structural extension (after the TerminalFilter seam at 26.2 and the override seam at 27), and the FIRST deferred-with-allowance surface to be consumed by the consumer it was deferred for (ADR-0213 Q4 → ADR-0221) — the YAGNI/extract-at-consumer discipline working as designed.

---

## 5. Proto-field roster (per §11.3 D28-2)

All rosters transcribed from go-control-plane v1.32.4 (`extensions/filters/network/zookeeper_proxy/v3/zookeeper_proxy.pb.go` + `.pb.validate.go`); verified by `proto.MessageName` run in-session.

### 5.1 TypeURL

`proto.MessageName(&zookeeper_proxyv3.ZooKeeperProxy{})` = `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` → **`@type` = `type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`** (the `extensions.` segment, per `reference_network_filter_typeurl_extensions`; pinned by an IMPL Task-1 `proto.MessageName` test, never the docs string).

### 5.2 `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` (9 fields; `[#next-free-field: 10]`)

| Go field | proto field | tag | Go type | default / PGV | 28.x disposition |
|---|---|---|---|---|---|
| `StatPrefix` | `stat_prefix` | 1 | `string` | **PGV-required min 1 rune** | REQUIRED → boot-reject (28.1; the `0047` fixture) |
| `AccessLog` | `access_log` | 2 | `string` | `[#not-implemented-hide:]`; no PGV | parse-accept-IGNORE (upstream parity — completely unread upstream, §11.2) |
| `MaxPacketBytes` | `max_packet_bytes` | 3 | `*wrapperspb.UInt32Value` | default **1 MiB** when unset; no PGV bound | SUPPORT (28.1; oversized → `decoder_error` + abandon buffer) |
| `EnableLatencyThresholdMetrics` | `enable_latency_threshold_metrics` | 4 | `bool` | false | parsed 28.1; consumed 28.2 (gates fast/slow increments) |
| `DefaultLatencyThreshold` | `default_latency_threshold` | 5 | `*durationpb.Duration` | default **100 ms** when unset; **PGV gte 1ms when set** | parsed+validated 28.1; consumed 28.2 |
| `LatencyThresholdOverrides` | `latency_threshold_overrides` | 6 | `[]*LatencyThresholdOverride` | no PGV count constraint | parsed+validated 28.1; consumed 28.2; duplicate opcode → boot-reject (28.2 arm) |
| `EnablePerOpcodeRequestBytesMetrics` | `enable_per_opcode_request_bytes_metrics` | 7 | `bool` | false | SUPPORT (28.1; gates `*_rq_bytes` increments) |
| `EnablePerOpcodeResponseBytesMetrics` | `enable_per_opcode_response_bytes_metrics` | 8 | `bool` | false | parsed 28.1; consumed 28.2 (gates `*_resp_bytes`) |
| `EnablePerOpcodeDecoderErrorMetrics` | `enable_per_opcode_decoder_error_metrics` | 9 | `bool` | false | SUPPORT (28.1; gates per-opcode `*_decoder_error`) |

### 5.3 `LatencyThresholdOverride` (2 fields) + its 27-value `Opcode` enum

| Go field | proto field | tag | Go type | PGV |
|---|---|---|---|---|
| `Opcode` | `opcode` | 1 | `LatencyThresholdOverride_Opcode` enum | **defined_only** |
| `Threshold` | `threshold` | 2 | `*durationpb.Duration` | **required + gte 1ms** |

The proto `Opcode` enum (27 contiguous values 0..26): 0=Connect, 1=Create, 2=Delete, 3=Exists, 4=GetData, 5=SetData, 6=GetAcl, 7=SetAcl, 8=GetChildren, 9=Sync, 10=Ping, 11=GetChildren2, 12=Check, 13=Multi, 14=Create2, 15=Reconfig, 16=CheckWatches, 17=RemoveWatches, 18=CreateContainer, 19=CreateTtl, 20=Close, 21=SetAuth, 22=SetWatches, 23=GetEphemerals, 24=GetAllChildrenNumber, 25=SetWatches2, 26=AddWatch.

**This proto enum is NOT the wire opcode set** (AMEND-A6). The wire `OpCodes` (decoder) enum has 26 values with gaps + a negative: Connect=0, Create=1, Delete=2, Exists=3, GetData=4, SetData=5, GetAcl=6, SetAcl=7, GetChildren=8, Sync=9, **Ping=11**, GetChildren2=12, Check=13, Multi=14, Create2=15, Reconfig=16, CheckWatches=17, RemoveWatches=18, CreateContainer=19, **CreateTtl=21**, **Close=−11**, **SetAuth=100**, **SetWatches=101**, **GetEphemerals=103**, **GetAllChildrenNumber=104**, **SetWatches2=105**, **AddWatch=106**. The proto→wire mapping table (upstream `filter.h:311-339`) is mirrored in `config.go` to key the latency-override map by wire opcode.

---

## 6. PARSE-REJECT roster (per §11.3 + AMEND-A10)

### 6.1 Wording discipline

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a table test at IMPL. Boot-reject PARITY arms (mirroring an upstream PGV/config-load failure) are distinguished from envoy-go-strict DEPARTURE arms. Phase 28 has NO departure-class rejects (every reject below mirrors an upstream reject) — the departures in this phase are all LENIENT (envoy-go accepts/ignores where upstream also accepts, or envoy-go counts where upstream errors — §2.1 shallow-decode note), recorded in BEHAVIOR_CONTRACT, never as rejects.

### 6.2 28.1 PARSE-REJECT arms

- `zookeeper-stat-prefix-required` — boot-reject PARITY (mirrors the `stat_prefix` PGV min-1-rune rule, §5.2). The load-bearing `0047` fixture arm.
- Framework-level: unknown network-filter `typed_config` type_url → existing boot-reject (no new arm).
- `access_log`: NOT a reject (parse-accept-ignore; upstream parity).
- `max_packet_bytes`: NOT a reject (no PGV constraint; any uint32 accepted — including 0, which makes every packet oversized → `decoder_error`; upstream parity).

### 6.3 28.2 PARSE-REJECT arms (parsed/validated at 28.1 config parse; their FIXTURE/test landing is 28.2's — D-P4)

- `zookeeper-latency-override-threshold-required` / `zookeeper-latency-override-threshold-too-small` — boot-reject PARITY (PGV `required` + `gte 1ms`, §5.3).
- `zookeeper-latency-override-opcode-undefined` — boot-reject PARITY (PGV `defined_only`).
- `zookeeper-default-latency-threshold-too-small` — boot-reject PARITY (PGV `gte 1ms` when set).
- `zookeeper-latency-override-duplicate-opcode` — boot-reject PARITY (upstream config-load `EnvoyException` "Duplicate latency threshold overrides" — `config.cc:43-50`; NOT a PGV rule, a constructor-time check).

NOTE: because the 28.1 config parse validates the FULL proto (all 9 fields incl. the latency fields), these arms' parse code lands at 28.1; whether their boot-reject FIXTURE arms land in `0047` at 28.1 or wait for 28.2 is D-P4 (anticipated: the PGV-mirror arms are unit-tested at 28.1, and 28.2 decides whether `0047` gains fixture arms or they stay unit-test-only).

---

## 7. Stat surface (per §11.1 D28-1 + §11.2 D28-3 + AMEND-A1/A2/A3/A4)

### 7.1 Scope/prefix shape — `<stat_prefix>.zookeeper.<counter>` (AMEND-A1)

Upstream: `config.cc:27` `fmt::format("{}.zookeeper", proto_config.stat_prefix())` + `POOL_COUNTER_PREFIX`. Emitted names: `<stat_prefix>.zookeeper.<counter>` (confirmed live: `zkprobe.zookeeper.connect_rq`). envoy-go mirrors this internal naming exactly (the differential `StatsAsserter` + the Prometheus arm depend on it).

### 7.2 The 201-counter roster (AMEND-A2/A3) — the per-opcode opname table

The opname list (28 names; the stats-macro spelling, all lowercase): `connect`, `connect_readonly`*, `ping`, `auth`*, `getdata`, `create`, `create2`, `createcontainer`, `createttl`, `setdata`, `getchildren`, `getchildren2`, `getallchildrennumber`, `getephemerals`, `delete`, `exists`, `getacl`, `setacl`, `sync`, `check`, `multi`, `reconfig`, `setauth`†, `setwatches`, `setwatches2`, `addwatch`, `checkwatches`, `removewatches`, `close`. (*` connect_readonly` appears ONLY in the `_rq`/`_rq_bytes` families; *`auth` appears in the `_resp`/`_resp_bytes`/`_resp_fast`/`_resp_slow`/`_decoder_error`/`_rq_bytes` families but has NO `auth_rq`; † `setauth` is the wire opcode whose opname is `auth` — there are no `setauth_*` counters.) The exact 201-name roster is the upstream macro transcribed verbatim; the 28.1 SPEC/IMPL pins it as a Go table with a `TestCounterRoster_MatchesUpstreamMacro` byte-stable test.

| Family | Count | Created | Incremented | Gated by |
|---|---|---|---|---|
| `decoder_error`, `request_bytes` | 2 | 28.1 | 28.1 | never gated |
| `response_bytes`, `watch_event` | 2 | 28.1 | 28.2 | never gated |
| `<op>_rq` (incl. `connect_readonly_rq`; NO `auth_rq`) | 28 | 28.1 | 28.1 | never gated |
| `<op>_rq_bytes` (incl. both connect variants + `auth_rq_bytes`) | 29 | 28.1 | 28.1 | `enable_per_opcode_request_bytes_metrics` |
| `<op>_decoder_error` (incl. `connect_decoder_error`) | 28 | 28.1 | 28.1 | `enable_per_opcode_decoder_error_metrics` |
| `<op>_resp` (incl. `auth_resp`) | 28 | 28.1 | 28.2 | never gated |
| `<op>_resp_bytes` | 28 | 28.1 | 28.2 | `enable_per_opcode_response_bytes_metrics` |
| `<op>_resp_fast` | 28 | 28.1 | 28.2 | `enable_latency_threshold_metrics` (latency ≤ threshold) |
| `<op>_resp_slow` | 28 | 28.1 | 28.2 | `enable_latency_threshold_metrics` (latency > threshold) |
| **Total** | **201** | | | |

**Creation parity at 28.1 (anticipated; D-P5 confirms at the 28.1 SPEC):** all 201 counters are created EAGERLY at config parse from 28.1 onward (upstream creation parity — the booted reference image exposes the full roster at 0 with the default flags). Response-side counters exist-at-zero until their increment paths land at 28.2; the `0046` fixture asserts this exists-at-zero state cross-side (a creation-parity assertion the 28.2 increments cannot regress).

**Dynamic (non-macro) counters:** the per-scheme auth-request counters `<stat_prefix>.zookeeper.auth.<scheme>_rq` (e.g. `auth.digest_rq`; unknown scheme → `auth.unknown_scheme_rq`) are created LAZILY upstream via a StatNameSet. envoy-go mirrors them via `NewCounterIfAbsent` (the rbac per-policy dynamic-name precedent); they are NOT counted in the static 337 surface.

### 7.3 Project stat-count delta

136 → **337** at family-row-done (+201; the largest single-filter stat addition in the project, superseding the BRAINSTORM's ~190–240 estimate). All +201 are CREATED at 28.1 (creation parity); the increment surface completes at 28.2. Dynamic auth-scheme counters excluded from the static count (config/traffic-dependent, lazily created — the rbac `policy.<name>.*` precedent).

### 7.4 Prometheus exposition — the `.zookeeper.` INLINE-PREFIX arm (AMEND-A4)

Reference Envoy v1.37.2 `/stats/prometheus` (probed live, two stat_prefix values): zookeeper stats emit as **flat** `envoy_<stat_prefix>_zookeeper_<counter>{}` — the stat_prefix is part of the metric NAME; the label set is EMPTY; every metric family has a `# TYPE … counter` line. There is NO upstream tag extraction for this filter.

envoy-go's `internal/stats/name.go` default branch ERRORS on unrecognized prefixes (`name.go:243`), so a new arm is required — but it is the **ADR-0138 bandwidth_limit INLINE-PREFIX shape** (dot→underscore + `envoy_` prefix; NO label promotion), NOT the `.rbac.` tag-extractor shape (ADR-0218) the BRAINSTORM hypothesized. Detection: segment `.zookeeper.` with a dot-free `<stat_prefix>` head → `envoy_` + `strings.ReplaceAll(internal, ".", "_")`, no labels. Because the roster is 201 names + dynamic auth names, the arm validates by SHAPE (the `.zookeeper.` segment + dot-free prefix), not a per-counter allowlist — the exact validation posture is pinned at the 28.1 SPEC (D-P8), balancing the ADR-0138 allowlist precedent against a 201-name list's maintenance cost.

### 7.5 envoy-go-strict / envoy-go-lenient departure flags (BEHAVIOR_CONTRACT at each IMPL)

- The latency HISTOGRAM family unmirrored (ADR-0060; coverage boundary at 28.2).
- Dynamic metadata unmirrored (AMEND-A9; coverage boundary at 28.1).
- Shallow-decode leniency: payload-malformed packets count as `<op>_rq` on envoy-go vs `decoder_error` upstream (§2.1; coverage boundary at 28.1; the fixture corpus contains no such packets).
- The write chain observes terminal-originated writes only (§4.1 item 6; coverage boundary at 28.1).
- `access_log` parse-accept-ignore (upstream parity, not a departure — recorded for completeness).

---

## 8. Differential fixture taxonomy (+3 across two sub-phases)

Full cross-side against reference Envoy v1.37.2. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE directories (one dir = one runner branch). Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` (cross-side path; `AssertStats(t, refAdminAddr, subjAdminAddr)`) and MUST be proven live via a deliberate-break. The body differential is intrinsically vacuous for this filter (bytes pass through unchanged on both sides) — the stat comparison IS the proof. Numbering continues from `0045` (master tip tail); re-pinned at each sub-phase IMPL Task 1.

### 8.1 `0046-zookeeper-requests` (28.1; cross-side)

Chain `[zookeeper_proxy, tcp_proxy]` on BOTH sides; the fixture backend is a canned-bytes TCP responder (it accepts and reads; it need not reply for the request-side arms). Driver sends hand-crafted jute-encoded ZK frames; `StatsAsserter` compares the mirrored counters cross-side (scraping both admin endpoints; the reference side reads `/stats?filter=zookeeper`, the subject side reads `/stats/prometheus` through the §7.4 flattening — the exact scrape mechanics mirror the `0043` rbac driver). Arms:

1. **connect** — the 48-byte connect frame (xid 0 special framing) → `connect_rq` +1 both sides.
2. **multi-opcode sequence** — connect + ping (xid −2) + getdata (xid 1, opcode 4) + create (xid 2, opcode 1) + close (xid 3, opcode −11) in one connection → `connect_rq`/`ping_rq`/`getdata_rq`/`create_rq`/`close_rq` each +1; `request_bytes` equal cross-side.
3. **digit-suffixed opcodes** — create2 (opcode 15) + getchildren2 (opcode 12) + setwatches2 (opcode 105) → their `_rq` counters +1 (the digit-truncation regression guard, per `reference_proto_roster_extraction_digits`).
4. **garbage bytes** — a frame whose length prefix exceeds the remaining bytes / random bytes → `decoder_error` +1 both sides; connection NOT broken (passthrough proven by the tcp_proxy still relaying the garbage to the backend byte-exact).
5. **flag-gated arm** — a second listener (or second fixture bootstrap pair) with `enable_per_opcode_request_bytes_metrics: true` → `getdata_rq_bytes` increments equal cross-side; with the flag false (arm 2) it stays 0 on both sides.
6. **eager-roster / exists-at-zero arm** — assert a sample of response-side counters (`getdata_resp`, `getdata_resp_fast`) exist at 0 on BOTH sides (creation parity, §7.2).
7. **deliberate-break liveness proof** — recorded in the driver comments + README per the `0030` lesson: e.g. temporarily asserting `getdata_rq == 2` (when 1 is sent) must FAIL on both runner paths.

### 8.2 `0047-zookeeper-boot-reject` (28.1; boot-reject)

Missing `stat_prefix` → both sides reject at boot (PGV-mirror; boot-stderr substring parity). Whether the 28.2 latency-override reject arms (§6.3) extend this dir or stay unit-test-only is D-P4.

### 8.3 `0048-zookeeper-responses` (28.2; cross-side)

Chain `[zookeeper_proxy, tcp_proxy]` both sides; the backend is a canned-bytes responder that replies to each request with a hand-crafted ZK response frame (correlated xid + zxid + error 0). Arms (28.2 SPEC finalizes): request+response round-trips → `*_resp` parity; connect → connect-response special framing → `connect_resp`; a watch-event push (xid −1) → `watch_event`; an unknown-xid response → `decoder_error`; DETERMINISTIC threshold arms — `enable_latency_threshold_metrics: true` + `default_latency_threshold: 3600s` → ALL responses fast (`*_resp_fast` == `*_resp`); a second arm with `default_latency_threshold: 0.001s` (1 ms, the PGV minimum) → all (or nearly all) slow — the 28.2 SPEC pins the exact deterministic construction (the 1 ms arm must be provably deterministic cross-side, e.g. by a backend that delays its response ≥ 10 ms so both sides exceed 1 ms).

### 8.4 Total fixture-dir count

47 → **50** (+2 at 28.1; +1 at 28.2). No new conformance harness (matches 26/27); h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at each sub-phase six-gate.

---

## 9. Behavior-contract delta (per ADR-0052 atomic landing)

BEHAVIOR_CONTRACT.md gains phase-28 content in two passes (one bundle per sub-phase IMPL final task):

- **28.1 bundle**: NEW `### Network filter chain framework — WriteFilter seam (28.1 amendment)` block (the write-direction dispatch + reverse order + StopIteration posture + the terminal-originated-writes-only boundary); NEW `### envoy.filters.network.zookeeper_proxy` subsection (request-side semantics; the 201-counter roster + creation parity; the `<stat_prefix>.zookeeper.` scope; the Prometheus inline-prefix flattening; the shallow-decode leniency; the dynamic-metadata coverage boundary; the per-scheme auth dynamic counters); stat table 136 → 337.
- **28.2 bundle**: the response-side + latency-threshold extension of the zookeeper subsection (correlation semantics; watch events; the deterministic fast/slow counters); the latency-HISTOGRAM coverage-boundary record (ADR-0060); parent-row-28 family rollup note.

---

## 10. ADR anchor map (3 §Context drafts at THIS parent-SPEC commit)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL) + the BRAINSTORM §7 locked numbering + the phase-25/26 parent-SPEC precedent. At THIS parent-SPEC commit, ADR-0221 + ADR-0222 + ADR-0223 §Context drafts are appended to DECISIONS.md (tail ADR-0220 → ADR-0223; next-free → ADR-0224). The 28.1 IMPL lands the 0221/0222 bodies; the 28.2 IMPL lands the 0223 body. (The phase-26 precedent drafted only the first sub-phase's ADRs at the parent SPEC; phase 28 drafts all three because the 28.2 ADR's §Context is fully determined by THIS session's empirical pins — deferring its draft to the 28.2 SPEC would re-derive the same evidence. The 28.2 SPEC may still amend the 0223 §Context in place if its own session refines it.)

- **ADR-0221** *(28.1)* — the `network.WriteFilter` seam: the `WriteFilter`/`WriteFilterCallbacks` interfaces + read/write/both/terminal chain classification + REVERSE-order write dispatch (AMEND-A11) + `writeChainConn` conn-wrap delivery (zero-write-filter chains unwrapped → byte-identical back-compat) + StopIteration-on-write = no-forward + documented-unsupported-by-consumers + the terminal-originated-writes-only boundary; CONSUMES the ADR-0213 API-revision allowance (consumer #1 zookeeper_proxy; anticipated #2 mongo_proxy).
- **ADR-0222** *(28.1)* — the `zookeeper_proxy` filter, request side: TypeURL + 9-field config parse + the 201-counter EAGER roster under `<stat_prefix>.zookeeper.` (creation parity; AMEND-A1/A2/A3) + the shallow request decoder (xid sniffing + special xids + min-length validation + internal reassembly buffers; AMEND-A5/A8) + the two correlation structures (AMEND-A7) + dynamic per-scheme auth counters + the `.zookeeper.` Prometheus inline-prefix arm (AMEND-A4) + the dynamic-metadata DEFERRAL (AMEND-A9) + fixtures `0046`/`0047` + the 37th fuzzer.
- **ADR-0223** *(28.2)* — the response-side decoder + xid correlation + watch events + latency-threshold fast/slow counters (`<=` semantics; wire-opcode-keyed overrides; duplicate-override boot-reject; AMEND-A10) + the deterministic-threshold differential discipline + fixture `0048` + the 38th fuzzer + the latency-histogram coverage boundary + the parent-row-28 ROLLUP.

Next-free after phase-28 phase-done ≈ **ADR-0224**.

---

## 11. Empirical-pin block (D28-1..D28-10 resolved at this SPEC session)

Parallel-subagent-fan-out scrape executed during this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-01.** The 10 pins span both sub-phases; resolved once here; sub-phase SPECs reference this block.

**Reference source corpus:**

1. **The live `envoyproxy/envoy:v1.37.2` docker image** (image id `c5e8a68e52f4`, present locally): `--mode validate` + a real boot of a `[zookeeper_proxy, tcp_proxy]` listener + admin `/stats` and `/stats/prometheus` scrapes (two stat_prefix values).
2. **go-control-plane v1.32.4 bindings** at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/zookeeper_proxy/v3/`: `zookeeper_proxy.pb.go` + `zookeeper_proxy.pb.validate.go`; `proto.MessageName` run in-session (worktree probe test, deleted after).
3. **Upstream Envoy v1.37.2 source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/filters/network/zookeeper_proxy/{filter.h,filter.cc,config.h,config.cc,decoder.h,decoder.cc,utils.h,utils.cc}`; `envoy/network/filter.h`; `source/common/network/{filter_manager_impl.h,filter_manager_impl.cc,connection_impl.cc}`.
4. **envoy-go codebase** at master `0ccd392`: `internal/filter/network/{types,terminal,prefixconn,callbacks,buffer,chain,registry}.go`; `internal/filter/network/builtins/builtins.go`; `internal/stats/name.go`; `internal/bootstrap/bootstrap.go`; `test/fixtures/0043-network-rbac/driver/driver.go`; `test/differential/fixture/fixture.go`.

### Summary disposition table (10 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D28-1 (SPEC-BLOCKING) — reference image ships zookeeper_proxy | **CONFIRMED** (validate OK; boots; 201 eager counters live) + REFINES the stat scope order | A1 |
| §11.2 | D28-2 — proto roster + PGV + TypeURL | CONFIRMS (9 fields; stat_prefix required; 27-value proto enum) + REFINES (latency PGV gte-1ms/required; access_log unread upstream) | A10 |
| §11.3 | D28-3 — stat scope + counter roster + eager/lazy | REFINES (201 eager macro counters; `<stat_prefix>.zookeeper.` scope; flag-gating = increments not creation; auth/connect_readonly asymmetries; lazy auth-scheme + histogram families) | A1, A2, A3 |
| §11.4 | D28-4 — wire framing + special xids + opcode enum + correlation | REFINES (xid sniffing; 5 special xids incl. SetWatches −8; 26-value gapped wire enum ≠ proto enum; TWO correlation structures) | A5, A6, A7 |
| §11.5 | D28-5 — decoder-error semantics + passthrough + max_packet_bytes | CONFIRMS (passthrough unconditional; never closes) + REFINES (decoder-internal reassembly buffers; abandon-buffer-no-resync; oversized → decoder_error) | A8 |
| §11.6 | D28-6 — dynamic metadata mirror-or-defer | RESOLVES: **DEFER** (refutes the BRAINSTORM's anticipated-mirror — requires deep payload parsing; zero observable surface) | A9 |
| §11.7 | D28-8 — latency-threshold semantics | CONFIRMS (`<=` → fast; 100 ms default; per-opcode overrides) + REFINES (wire-opcode keying; duplicate-override boot-reject; PGV bounds) | A10 |
| §11.8 | D28-7 — write-path semantics | CONFIRMS (REVERSE/LIFO order; conn-wrap justified by ConnectionImpl::write entry) + REFINES (StopIteration = no-forward, NO resume; zookeeper registers addFilter combined; onWrite always Continue) | A11 |
| §11.9 | D28-9 — per-sub-phase envelope | REFINES (28.1 straddles the gate → D-P1 pre-authorized split axis; 28.2 fits) | — |
| §11.10 | D28-10 — fuzzer envelope | RESOLVES (37th request-decoder fuzzer at 28.1; 38th response-decoder fuzzer at 28.2) | — |

### 11.1 D28-1 (SPEC-BLOCKING) — the reference image ships zookeeper_proxy: CONFIRMED

`docker run --rm -v …:/probe:ro envoyproxy/envoy:v1.37.2 --mode validate -c /probe/envoy.yaml` → `configuration '/probe/envoy.yaml' OK`, exit 0, for a bootstrap with a `[zookeeper_proxy, tcp_proxy]` filter chain (`stat_prefix: zkprobe`; one static cluster). A real boot stays up (`starting main dispatch loop`; listener live at `/listeners`), with NO zookeeper-related warnings. The admin `/stats` endpoint exposes the FULL eager counter roster at 0 before any traffic — names of the form `zkprobe.zookeeper.<counter>` (e.g. `zkprobe.zookeeper.connect_rq`, `zkprobe.zookeeper.create2_rq`, `zkprobe.zookeeper.getallchildrennumber_rq`, `zkprobe.zookeeper.decoder_error`, `zkprobe.zookeeper.watch_event`, `zkprobe.zookeeper.getdata_resp_fast`). zookeeper_proxy is a CORE extension at `source/extensions/filters/network/zookeeper_proxy/` (not contrib). **The Q3 cross-side strategy stands.**

### 11.2 D28-2 — proto roster + PGV + TypeURL

§5 transcribes the full roster. Key PGV facts (`zookeeper_proxy.pb.validate.go`): `stat_prefix` min 1 rune (required); `access_log` unconstrained (and grep-confirmed completely unread by upstream filter/config code — the only `access_log` token in the four upstream source files is an unused `#include`); `max_packet_bytes` unconstrained (doc default 1 MiB); `default_latency_threshold` gte 1ms WHEN SET (not required; doc default 100 ms); `LatencyThresholdOverride.opcode` defined_only; `LatencyThresholdOverride.threshold` REQUIRED + gte 1ms. `proto.MessageName` (run in-session) = `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`; the nested message is `…v3.LatencyThresholdOverride`. The proto `Opcode` enum has exactly 27 contiguous values 0..26 (digit-suffixed names `Create2`/`GetChildren2`/`SetWatches2` intact — extracted with the digit-inclusive discipline per `reference_proto_roster_extraction_digits`).

### 11.3 D28-3 — stat scope + the 201-counter roster + eager/lazy

Upstream `filter.h:30-231` `ALL_ZOOKEEPER_PROXY_STATS(COUNTER)` declares 201 counters (verified by grep-count against the fetched macro): 4 plain + 28 `_rq` + 29 `_rq_bytes` + 28 `_decoder_error` + 28 `_resp` + 28 `_resp_bytes` + 28 `_resp_fast` + 28 `_resp_slow`. Created EAGERLY at config load: `filter.cc:30` `stats_(generateStats(stat_prefix, scope))` over `POOL_COUNTER_PREFIX(scope, prefix)` (`filter.h:307-309`); scope prefix `config.cc:27` `fmt::format("{}.zookeeper", proto_config.stat_prefix())`. Live-confirmed (§11.1). Asymmetries: `connect_readonly_rq`/`connect_readonly_rq_bytes` exist (rq-side only); NO `auth_rq` (auth requests counted via the LAZY StatNameSet per-scheme counters `auth.<scheme>_rq`, builtin schemes {digest, host, ip, world, x509, unknown_scheme…} — `filter.cc:45-46,306-313`); `auth_resp*` counters ARE in the macro (the SetAuth opcode's opname is `auth`). The four `enable_*` flags gate INCREMENTS only (`filter.cc:256-299` — the ungated `decoder_error`/`request_bytes`/`response_bytes` always increment; the per-opcode gated families increment only when their flag is true). The flag-false gated counters exist at 0 forever — confirming creation-vs-increment separation. LAZY families (NOT mirrored / deferred): per-scheme auth counters (mirrored — §7.2) and the latency histograms `connect_response_latency`/`<opname>_latency`/`unknown_opcode_latency` (deferred — ADR-0060).

### 11.4 D28-4 — wire framing + special xids + wire opcodes + correlation structures

Big-endian throughout (`peekBEInt<int32_t/int64_t>`); 4-byte length prefix EXCLUDES itself. **Request path** (`decoder.cc:45-254` decodeOnData): read len → `ensureMinLength(len, XID_LENGTH + INT_LENGTH)` → `ensureMaxLength(len)` → peek xid → switch on `XidCodes`: `ConnectXid=0` → parseConnect (zxid+timeout+session skip + password skip + optional readonly bool; NO opcode field); `PingXid=-2` → onPing; `AuthXid=-4` → parseAuthRequest; `SetWatchesXid=-8` → parseSetWatchesRequest; default (xid > 0) → peek opcode → per-opcode dispatch. Connect detection is per-packet xid SNIFFING — no first-packet state machine. **Response path** (`decoder.cc:256-359` decodeOnWrite): len → xid → connect-xid → parseConnectResponse (proto_version+timeout+session+password; NO zxid/error); `WatchXid=-1` → parseWatchEvent (returns no opcode — never correlated); else → fetch the pending request (latency + opcode) → zxid(8) + error(4) → onResponse. **Wire `OpCodes` enum** (`decoder.h:30-58`): 26 values, gaps at 10/20/102, `Ping=11`, `CreateTtl=21`, `Close=-11`, `SetAuth=100`, `SetWatches=101`, `GetEphemerals=103`, `GetAllChildrenNumber=104`, `SetWatches2=105`, `AddWatch=106`. **Correlation** (`decoder.h:208-213`): `requests_by_xid_` flat map (data xids; insert overwrites; response lookup ERASES) + `control_requests_by_xid_` map of FIFO queues (control xids repeat). Unknown response xid → `InvalidArgumentError` → `onDecodeError` → `decoder_error`.

### 11.5 D28-5 — decoder-error + passthrough + max_packet_bytes + internal buffering

`decoder_error` increments on every `onDecodeError` call: parse failures (buffer underflow "read beyond buffer size", `ensureMinLength` "packet is too small", `ensureMaxLength` "packet is too big"), unknown opcode, unknown response xid, caught exceptions. **Passthrough is unconditional**: `ZooKeeperFilter::onData`/`onWrite` (`filter.cc:200-208`) always return `Continue`; the decoder never drains/modifies the chain buffer (its only drain removes its OWN prepended reassembly bytes); the connection is never closed by the filter. On a decode failure the remaining packets in the CURRENT buffer are abandoned (no resync), but later buffers keep decoding (the correlation maps persist). `max_packet_bytes` exceeded → "packet is too big" → `decoder_error` + abandon buffer + pass through. **Partial packets** are reassembled in the decoder's own `zk_filter_read_buffer_`/`zk_filter_write_buffer_` (`decoder.cc:868-961`: prepend saved bytes → measure full packets via length prefixes → decode the full ones → copy the trailing partial into the save buffer). envoy-go mirrors this internal-buffer model (the chain `Buffer` is read, never drained, by zookeeperproxy).

### 11.6 D28-6 — dynamic metadata: DEFER (resolves §2.7 mirror-or-defer)

Upstream emits per-message metadata under namespace `envoy.filters.network.zookeeper_proxy` (`NetworkFilterNames::get().ZooKeeperProxy`), keys per event (`opname` always; `path`/`watch`/`bytes`/`version`/`create_type`/`mode`/`event_type`/`client_state`/`zxid`/`error`/`timeout`/`protocol_version`/`readonly` per handler — the full table is in the §11 research record), CLEARED at the top of every `onData`/`onWrite`. Emitting `path`/`watch`/`version`/… requires parsing each opcode's payload (the bulk of upstream's ~1100-line decoder.cc). envoy-go has NO consumer of connection-level dynamic metadata (grep-confirmed; the ADR-0217 rbac shadow pair is the only writer and nothing reads it), the emission is invisible to the differential, and the Q1 scope is COUNTER parity. **Resolution: DEFER** (AMEND-A9) with a BEHAVIOR_CONTRACT coverage-boundary record at 28.1; the decoder is shallow (header + length validation); the ADR-0217 bucket stays available for a future consumer.

### 11.7 D28-8 — latency-threshold semantics

`ZooKeeperFilterConfig::errorBudgetDecision` (`filter.cc:134-154`): if `!enable_latency_threshold_metrics_` → None (no increment); else threshold = per-opcode override (keyed by WIRE opcode int via the proto→wire `opcodeMap`, `filter.h:311-339`) else `default_latency_threshold_` (default 100 ms, `PROTOBUF_GET_MS_OR_DEFAULT`); **`latency <= threshold` → Fast** (inclusive), else Slow → `<opname>_resp_fast`/`<opname>_resp_slow`. Connect handled inline in `onConnectResponse`; opcodes outside the op_code_map skip fast/slow. Duplicate opcode override → config-load `EnvoyException` (`config.cc:43-50`); unknown proto opcode → `EnvoyException` (`filter.cc:174-181`). Both are 28.2 boot-reject parity arms (§6.3).

### 11.8 D28-7 — write-path semantics

`envoy/network/filter.h`: `WriteFilter { onWrite(Buffer&, bool end_stream); initializeWriteFilterCallbacks(WriteFilterCallbacks&) }`; `Filter : public WriteFilter, public ReadFilter` (combined); FilterManager doc: "Add a write filter… Filters are invoked in LIFO order (the last added filter is called first)" vs read FIFO. `filter_manager_impl.cc:13-30`: `addWriteFilter` FRONT-inserts into `downstream_filters_`; `addReadFilter` BACK-inserts into `upstream_filters_`; `addFilter` = both. `onWrite` (`:197-214`) iterates `downstream_filters_` front→back; a `StopIteration` aborts the loop and `ConnectionImpl::write` (`connection_impl.cc:561-593`) EARLY-RETURNS before `write_buffer_->move(data)` — the data never reaches the transport; source comment: "currently we don't support restart/continue on the write path". There is NO `continueWriting()`; the only resume is a filter-owned `injectWriteDataToFilterChain` → `rawWrite` (bypasses the chain). **Entry point**: every `connection.write(data, end_stream)` → `filter_manager_.onWrite()` BEFORE buffering/transport — exactly what `writeChainConn` mirrors by intercepting the terminal's `Write` calls. zookeeper_proxy registers via `filter_manager.addFilter(std::make_shared<ZooKeeperFilter>(…))` (`config.cc:59-61`) — ONE instance, both directions. Its `onWrite` ALWAYS returns `Continue` (`decodeAndBuffer`'s only two return statements are both `Continue`).

### 11.9 D28-9 — per-sub-phase envelope (re-estimated against the empirical findings)

**28.1**: WriteFilter seam (~150–250 production LoC: interface + classification + writeChainConn + dispatch) + zookeeperproxy config parse + proto→wire mapping (~150–200) + the 201-counter roster table + eager creation + dynamic auth counters (~150–200) + the shallow request decoder + xid structures + reassembly buffer (~300–450) + filter glue (~80–100) + builtins/bootstrap/name.go arm (~80–120) + fixtures `0046`/`0047` drivers (~500–700, the `0043` driver is 637 LoC) + the 37th fuzzer (~60) ≈ **~1150–1500 production LoC (excl. unit tests), ~16–20 tasks** — STRADDLES the ADR-0045 gate → D-P1 (the 28.1 PLAN re-checks; pre-authorized 28.1a/28.1b axis per §3.0). **28.2**: response decoder + correlation consumption + watch events (~200–300) + latency thresholds + override map consumption (~100–150) + filter glue (~50) + fixture `0048` driver (~450–600) + the 38th fuzzer (~60) + rollup docs ≈ **~600–900 production LoC, ~10–14 tasks** — fits.

### 11.10 D28-10 — fuzzer envelope

The request decoder (28.1) and response decoder (28.2) are distinct entry points with distinct framing (request: xid+opcode; response: xid+zxid+error) → one fuzzer each: `FuzzZookeeperRequestDecode` (37th, 28.1) + `FuzzZookeeperResponseDecode` (38th, 28.2). Both feed random bytes through the decoder asserting no-panic + no-chain-buffer-mutation. Fuzzer count recipe re-confirmed at each IMPL Task 1: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` = 36 at master tip (NOTE: the recipe is scoped to `./internal` — an unscoped `find .` reaches untracked tooling artifacts, e.g. `.claude/`, and over-counts; spec-reviewer advisory).

---

## 12. SPEC-time D-questions for sub-phase SPEC / PLAN resolution

- **D-P1 (28.1 split-gate).** The 28.1 envelope straddles the ADR-0045 gate (§11.9). **Resolution at:** 28.1 PLAN. **Pre-authorized split axis if it trips:** 28.1a (WriteFilter seam + config parse + eager roster + `0047`) / 28.1b (request decoder + xid structures + `0046` + fuzzer). Anticipated: fits as one sub-phase on the production-LoC basis (the 26.x accounting precedent).
- **D-P2 (decoder depth + WriteFilter interface composition).** Shallow decode (header + min-length table) vs structural-skip (validate payload framing without extracting values). **Resolution at:** 28.1 SPEC. Anticipated: shallow + a per-opcode min-length table (the §2.1 leniency departure recorded). Also: the exact `WriteFilter` interface composition (`OnDestroy` placement; whether `SetWriteFilterCallbacks` is separate from `SetReadFilterCallbacks` for a both-directions filter). Anticipated: a both-directions filter receives both callback injections; `OnDestroy` called once.
- **D-P3 (read-filter writes through the write chain).** Does `Connection().Write` (the read-filter write path) also route through the write chain? **Resolution at:** 28.1 SPEC. Anticipated: NO (terminal-originated writes only, §4.1 item 6) — zookeeper needs nothing more; recorded as a framework boundary under the API-revision allowance.
- **D-P4 (boot-reject fixture arm roster).** Does `0047` carry only the `stat_prefix` arm (28.1), with the latency-override reject arms (§6.3) unit-test-only — or does 28.2 extend `0047` with fixture arms? **Resolution at:** 28.2 SPEC. Anticipated: `stat_prefix` fixture arm at 28.1; latency arms unit-test-only at 28.2 (boot-reject fixture dirs stay one-per-sub-phase-need; the dispatch-constraint memory).
- **D-P5 (creation parity).** All 201 counters created at 28.1 (creation parity, §7.2) vs request-side-only creation. **Resolution at:** 28.1 SPEC. Anticipated: all 201 (the upstream model creates the full struct from one macro; the exists-at-zero differential arm depends on it).
- **D-P6 (response-decoder fuzzer).** A separate 38th fuzzer at 28.2 vs folding response decode into the 37th. **Resolution at:** 28.2 SPEC. Anticipated: separate (distinct entry points; §11.10).
- **D-P7 (writeChainConn Write return semantics).** What `(n int, err error)` does `writeChainConn.Write` return when the chain stops the write (StopIteration)? **Resolution at:** 28.1 SPEC. Anticipated: `(len(p), nil)` — the terminal cannot distinguish (upstream's terminal also cannot: `ConnectionImpl::write` returns void); a dropped write surfaces as downstream silence, exactly as upstream.
- **D-P8 (name.go arm validation posture).** Shape-based detection (`.zookeeper.` segment + dot-free prefix, permissive) vs a 201-name allowlist (the ADR-0138 14-name precedent). **Resolution at:** 28.1 SPEC. Anticipated: shape-based (a 201-name allowlist is unmaintainable; the wasm.* permissive-rule precedent at `name.go:114` supports shape-based for large rosters).
- **D-P9 (0048 deterministic-slow construction).** How the all-slow arm guarantees latency > 1 ms cross-side deterministically. **Resolution at:** 28.2 SPEC. Anticipated: a fixture backend that sleeps ≥ 10 ms before responding (both sides measure ≥ 10 ms > 1 ms).

---

## 13. RATIFIED-PENDING-IMPL items

- **R1 (seam back-compat).** A chain with ZERO write filters gets NO `writeChainConn` wrap → `handleTerminal` byte-identical to today. Ratified by ALL existing fixtures (0000..0045) staying byte-exact green at 28.1 (the seam's regression gate).
- **R2 (creation parity).** The 201-counter roster + the `<stat_prefix>.zookeeper.` scope match upstream name-for-name. Ratified by the `0046` exists-at-zero arm + a `TestCounterRoster_MatchesUpstreamMacro` byte-stable test.
- **R3 (passthrough invariant).** zookeeperproxy NEVER mutates/drains the chain buffer, never closes the connection, never returns StopIteration. Ratified by the `0046` garbage-bytes arm (passthrough proven byte-exact through tcp_proxy) + unit tests.
- **R4 (StatsAsserter liveness).** Every `0046`/`0048` stat assertion is proven live via a recorded deliberate-break (the `reference_differential_asserter_dispatch` discipline; the 0030 dead-assertion lesson).
- **R5 (correlation hand-off).** The 28.1 xid structures are written-but-unread until 28.2 (like the 26.1 DynamicMetadata accessor was shaped-but-unwritten). Ratified at 28.2 by the `0048` correlation arms.
- **R6 (fuzzer + fixture + stat counts).** Each sub-phase IMPL Task 1 re-pins: fuzzers 36 (→37 at 28.1, →38 at 28.2; recipe scoped to `./internal` per §11.10), fixtures 47 (tail `0045`; →49 at 28.1, →50 at 28.2), stat surface 136 (→337 at 28.1 creation), DECISIONS.md tail ADR-0223 (next-free ADR-0224) — against the live IMPL-session tip.
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` zookeeper lines match reference Envoy's flat shape `envoy_<prefix>_zookeeper_<counter>{}` (no labels). Ratified by the `0046` StatsAsserter scrape comparing the flattened forms.

---

## 14. Test surface

Per the §9 family precedent (unit + fuzz + differential + race), per sub-phase:

- **28.1**: Layer A unit tests at `internal/filter/network/` (write classification; reverse-order dispatch; writeChainConn forward/stop/passthrough-when-empty; back-compat: zero-write-filter chains unwrapped) + `internal/filter/network/zookeeperproxy/` (config parse incl. all PGV arms + the proto→wire mapping + duplicate-override reject; the counter roster table; request decode per opcode incl. connect/ping/auth/setwatches special xids + digit-suffixed opcodes + garbage + oversized + partial-packet reassembly; flag gating; dynamic auth counters) + `internal/stats/` (the `.zookeeper.` arm); Layer C the 37th fuzzer; Layer D fixtures `0046` + `0047` + the FULL back-compat suite (47 existing dirs — the seam's regression gate); Layer E `-race -short` across touched packages.
- **28.2**: unit tests for response decode (framing + connect-response + watch event + correlation + unknown xid + latency fast/slow boundary `<=`) + the override map; the 38th fuzzer; fixture `0048` + the full suite (50 dirs); race.
- **Six-gate checklist** (per phase-22/24/25/26/27): `go build` / `go vet` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 28 touches no HTTP path; HCM is untouched by the seam since its chain has no write filters). All outputs quoted into PROGRESS.md (run honestly). Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

---

## 15. Per-sub-phase split-gate confirmation (D28-9 → ADR-0045)

| Sub-phase | Production LoC | Tasks | ADR-0045 gate (~25t / ~1500 LoC) | Verdict |
|---|---|---|---|---|
| 28.1 | ~1150–1500 | ~16–20 | straddles | ⚠️ fits-with-caveat (D-P1 pre-authorized 28.1a/28.1b split if the PLAN trips) |
| 28.2 | ~600–900 | ~10–14 | fits | ✅ |

Each sub-phase is independently shippable + delivers value: 28.1 ships the write seam + a request-side-observable zookeeper filter with live cross-side stat parity; 28.2 completes the round-trip observability + latency signal + the family rollup. The 2-way pre-split holds at parent-SPEC time; the 28.1 sizing caveat is a PLAN decision, not a parent-SPEC blocker.

---

## 16. Stage-close handoff

Per ADR-0004/0005 (autonomous adaptation): this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, STATE.md advances to lifecycle-state 2-for-28.1 with `next-skill = superpowers:writing-plans` scoped to the **28.1 SPEC** (per the per-sub-phase precedent — the next session authors the 28.1 sub-phase SPEC, not the 28.1 PLAN, mirroring 22.1/25.1/26.1). ROADMAP: parent row 28 STAYS `in-progress`; sub-rows 28.1/28.2 STAY `planned` (28.1 flips `planned → in-progress` at the **28.1 SPEC** commit, NOT at this parent SPEC — the 26.x precedent). The parent SPEC + the ADR-0221/0222/0223 §Context drafts are squash-merged to master + pushed; next-prompt.txt is rewritten for the 28.1-SPEC cold-start.

---

## Appendix A — Phase 28 ADR landing summary

- **ADR-0221** *(§Context drafted at this SPEC; body at 28.1 IMPL)* — the `network.WriteFilter` seam: `WriteFilter`/`WriteFilterCallbacks` interfaces; read/write/both/terminal classification; REVERSE-order write dispatch; `writeChainConn` conn-wrap delivery (zero-write-filter chains unwrapped — byte-identical back-compat); StopIteration-on-write = no-forward + documented-unsupported-by-consumers; terminal-originated-writes-only boundary; CONSUMES the ADR-0213 API-revision allowance.
- **ADR-0222** *(§Context drafted at this SPEC; body at 28.1 IMPL)* — the `zookeeper_proxy` filter, request side: TypeURL + 9-field config parse; the 201-counter EAGER roster under `<stat_prefix>.zookeeper.` (creation parity); the shallow request decoder (xid sniffing, special xids, min-length validation, internal reassembly); the two correlation structures; dynamic per-scheme auth counters; the `.zookeeper.` Prometheus inline-prefix arm; the dynamic-metadata DEFERRAL; fixtures `0046`/`0047`; the 37th fuzzer.
- **ADR-0223** *(§Context drafted at this SPEC; body at 28.2 IMPL)* — the response-side decoder + xid correlation + watch events + latency-threshold fast/slow counters + the deterministic-threshold differential discipline + fixture `0048` + the 38th fuzzer + the latency-histogram coverage boundary + the parent-row-28 ROLLUP.

