# Phase 28 Brainstorm — Network WriteFilter seam + `zookeeper_proxy` (parent row; THIRD §9 Network-filters-family row)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 28 (`network-filter-zookeeper-proxy`), the **THIRD §9 Network-filters-family row** (after the phase-26 family-parent and the phase-27 `sni_cluster` flat row). Phase 28 lands `envoy.filters.network.zookeeper_proxy` — a passive observability sniffer that decodes the ZooKeeper client protocol in **both directions** and emits per-opcode counters — and, as its prerequisite, the **network WriteFilter seam** that the phase-26 framework explicitly deferred (26 BRAINSTORM Q4 / ADR-0213 API-revision allowance).

The next session (lifecycle-state 1 → 2 for phase 28, skill `superpowers:writing-plans` scoped to **parent SPEC authoring** per the phase 22 / 24 / 25 / 26 parent-row precedent) authors `docs/envoy-go/phases/28-network-filter-zookeeper-proxy/SPEC.md` based on this brainstorm — that parent SPEC formalizes the 2-way split surface-mapping + executes the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004. The per-sub-phase SPEC sessions (28.1 / 28.2) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the 22.1 / 25.1 / 26.1 precedent.

**Brainstorm session:** worktree `.worktrees/phase-28-network-filter-zookeeper-proxy-brainstorm`, branch `phase-28-network-filter-zookeeper-proxy-brainstorm`, branched from master tip `6f3babb` (`next-prompt.txt: repoint master-tip reference to 560e0a6` — a docs-only repoint commit). Substantive predecessor on master: `ab8098f` (the phase-27 IMPL squash — `sni_cluster` + the connection-scoped upstream-cluster-override seam, ADR-0219/0220).

**Brainstorm mode:** interactive with a live human. The user picked the subject + each major design decision via a 4-question dialogue:

- **Q0 subject selection** — `zookeeper_proxy` chosen from the 5 remaining §9 Network-filters candidates {redis / mongo / kafka_broker / thrift / zookeeper}, plus the option of any other §9 family. Rationale: the smallest well-bounded candidate; a pure observability read+write sniffer that exercises the existing read-filter framework, motivates the deferred WriteFilter seam, and adds the family's first stats-heavy differential surface. (`redis`/`thrift` are large terminal-proxy surfaces; `kafka_broker` is an Envoy **contrib** extension with reference-image availability risk; `mongo` is the natural NEXT candidate after the WriteFilter seam exists.)
- **Q1 scope envelope** — `Both-direction counters` chosen from {Both-direction counters (2-way pre-split) / Request-side-only MVP (single phase) / Full parity incl. histograms (3-way pre-split lifting ADR-0060)}. The filter's per-opcode **counter** surface is mirrored in both directions; the latency **histogram** family (`*_response_latency`) stays deferred per ADR-0060 (recorded as a coverage boundary); the deterministic latency-threshold counters (`enable_latency_threshold_metrics` → fast/slow) ARE in scope at 28.2.
- **Q2 write-seam shape** — `Upstream-faithful WriteFilter + conn-wrap delivery` chosen from {WriteFilter + conn-wrap / Narrow response-observer tap / Terminal-cooperative Handle-signature change}. A `network.WriteFilter` interface (`OnWrite(buf, endStream) Status`) + a write chain run in REVERSE order per Envoy semantics, delivered by the chain runtime wrapping the downstream `net.Conn` handed to the terminal (the `prefixConn` precedent) — `TerminalFilter.Handle` signature UNCHANGED, `tcp_proxy` untouched.
- **Q3 differential strategy** — `Synthesized-bytes + StatsAsserter` chosen from {Synthesized-bytes hermetic fixtures + cross-side StatsAsserter / Real ZooKeeper server in the loop / Unit tests + passthrough-only fixture}. Hand-crafted jute-encoded ZK wire bytes driven through a `[zookeeper_proxy, tcp_proxy]` chain on BOTH sides; the mirrored per-opcode counters compared cross-side via `StatsAsserter`; deliberate-break liveness proof per fixture.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0220), and the as-built §9 framework (26.1/26.2/26.3/27). Empirical pins requiring evidence against Envoy v1.37.2 are enumerated in §10 and deferred to parent-SPEC-drafting time per the phase 09–27 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/26-network-filter-chain-and-rbac/BRAINSTORM.md` section-for-section (the parent pre-split precedent), reframed for the WriteFilter-seam + zookeeper_proxy scope + the 2-way pre-split. Phase 28 sits in a structurally meaningful position: it **consumes the phase-26 Q4 write-filter deferral** (the ADR-0213 API-revision-allowance clause written exactly for this moment); it is the family's **first stats-heavy observability filter** (the largest single-filter stat addition in the project); and its WriteFilter seam **unblocks `mongo_proxy`** (the next anticipated family candidate, also a both-direction sniffer). Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-01.

---

## 1. Mission and scope confirmation (28 only)

ROADMAP row `28 | network-filter-zookeeper-proxy | 27 | in-progress | 28.1, 28.2 | …` (added by this brainstorm) is the parent row this brainstorm registers as `in-progress` with sub-phase list `28.1, 28.2`. The two sub-rows `28.1 | network-filter-write-seam-and-zookeeper-requests | 27 | planned | | …` and `28.2 | network-filter-zookeeper-responses-and-latency | 28.1 | planned | | …` are also registered by this brainstorm (long-prefix slug convention; phase-22/25/26 precedent). The phase-27 IMPL squash `ab8098f` is the parent row's `depends-on` anchor.

The Network filters family lists candidates at `ROADMAP.md` (§ Feature Families): `redis, mongo, kafka_broker, thrift, zookeeper [scope TBD], echo, direct_response, sni_cluster, rbac network`. Phases 26 + 27 landed `echo`, `direct_response`, `rbac network`, `sni_cluster`. Phase 28 lands **`zookeeper`** (resolving its `[scope TBD]` marker per Q1). After phase 28 phase-done, **4** family candidates remain (`redis`, `mongo`, `kafka_broker`, `thrift`). Branch/directory/Go-package identifiers: parent branch `phase-28-network-filter-zookeeper-proxy-brainstorm`, parent directory `28-network-filter-zookeeper-proxy/`, filter package `internal/filter/network/zookeeperproxy/` (Go package `zookeeperproxy`, single-token-joined per the `directresponse`/`snicluster` precedent).

Phase 28 is also: (i) the FIRST §9 row to **add the write-direction (response) half of the network filter chain** — at master tip the framework is read-filter + terminal only (`ReadFilter.OnNewConnection`/`OnData` + `TerminalFilter.Handle`); upstream→downstream bytes are invisible to chain filters. Phase 28.1 lands the `WriteFilter` seam, consuming the API-revision allowance ADR-0213 reserved at 26.1 (26 BRAINSTORM Q4: "read-filter only; write deferred until a consumer needs it" — zookeeper_proxy is that consumer). (ii) the FIRST §9 **stats-primary** filter — `echo`/`direct_response`/`sni_cluster` have zero stats and `rbac_network` has 4; zookeeper_proxy's whole purpose IS its stat surface (~25–30 per-opcode counters per direction), making the cross-side `StatsAsserter` differential the load-bearing proof rather than a supplementary one. (iii) the FIRST filter whose per-connection state machine spans BOTH directions — the xid→opcode correlation map written by the read path (requests) and consumed by the write path (responses) for `*_resp` attribution + latency measurement.

### 1.1 What phase 28 delivers as a self-contained whole (envelope: both-direction counters per Q1)

Phase 28 lands the network WriteFilter seam + `envoy.filters.network.zookeeper_proxy` at both-direction **counter** parity, across TWO sub-phases:

1. **Sub-phase 28.1** (`network-filter-write-seam-and-zookeeper-requests`) — delivers: (a) the **`network.WriteFilter` seam** — the `WriteFilter` interface (`OnWrite(buf *Buffer, endStream bool) Status`), chain classification extended so a registered filter may implement `ReadFilter`, `WriteFilter`, or both (upstream `Network::Filter` parity); the write chain run in REVERSE chain order per Envoy semantics; delivery via the chain runtime wrapping the downstream `net.Conn` handed to `handleTerminal` (a `writeChainConn`; `prefixConn` precedent) so terminal writes (upstream→downstream bytes) pass through the write chain — `TerminalFilter.Handle` signature UNCHANGED, `tcp_proxy`/HCM untouched; `StopIteration`-on-write semantics pinned at SPEC (anticipated: documented-unsupported until a consumer needs it — zookeeper always Continues); (b) the **`internal/filter/network/zookeeperproxy/` package** — TypeURL via `proto.MessageName` (the `extensions.` lesson, `reference_network_filter_typeurl_extensions`), config parse of the 9-field `ZooKeeperProxy` proto (`stat_prefix` REQUIRED → boot-reject; `max_packet_bytes` default 1 MiB; the three `enable_per_opcode_*` opt-in flags; the latency fields parsed at 28.1 but consumed at 28.2; `access_log` is upstream `[#not-implemented-hide:]` — parse posture pinned at SPEC), the **request-side decoder** (4-byte length-prefix framing, connect-request special-case, xid/opcode dispatch, per-opcode `*_rq` counters, `decoder_error` counter, the per-connection xid→opcode tracking map laid down for 28.2), dynamic-metadata writes via the connection-scoped `*dynamicmetadata.Bucket` (rbac shadow precedent; mirror-or-defer pinned at SPEC); (c) registration as the **7th built-in** (`builtins.RegisterBuiltins` single insertion; D27-S2 precedent) + the `zookeeper_proxy/v3` proto blank-import in `internal/bootstrap/bootstrap.go` (echo precedent); (d) the **Prometheus tag-extractor arm** for the zookeeper stat family at `internal/stats/name.go` (the `.rbac.` precedent); (e) the **`0046-zookeeper-requests`** cross-side differential fixture (StatsAsserter per-opcode `*_rq` parity arms + a garbage-bytes `decoder_error` arm + deliberate-break proof) + the **`0047-zookeeper-boot-reject`** fixture (missing `stat_prefix`); (f) the **request-decoder fuzzer** (37th); (g) the BEHAVIOR_CONTRACT.md 28.1 bundle + STATE/ROADMAP advance (sub-row 28.1 `planned → done`).

2. **Sub-phase 28.2** (`network-filter-zookeeper-responses-and-latency`) — delivers: (a) the **response-side decoder** in `OnWrite` — response framing, xid correlation against the 28.1 tracking map, per-opcode `*_resp` counters, watch-event handling (xid −1), ping/auth special xids (−2 / −4); (b) **latency measurement** (request-timestamp → response) + the `enable_latency_threshold_metrics` fast/slow counter surface (`*_resp_fast` / `*_resp_slow`; `default_latency_threshold` default 100 ms; `latency_threshold_overrides` per the proto's 27-value opcode enum) — counters only; the `*_response_latency` HISTOGRAM family stays deferred per ADR-0060 with a BEHAVIOR_CONTRACT coverage-boundary record; (c) the per-opcode response-bytes opt-in counters (scope pinned at SPEC); (d) the **`0048-zookeeper-responses`** cross-side differential fixture (resp-counter parity + correlation arms + DETERMINISTIC threshold arms via extreme thresholds — e.g. threshold 1 h → all fast, threshold 0/1 ms → all slow — avoiding cross-side timing nondeterminism); (e) optionally the response-decoder fuzzer (38th; SPEC pins); (f) the BEHAVIOR_CONTRACT.md 28.2 bundle + the parent-row-28 ROLLUP (parent flips `in-progress → done` ATOMICALLY with sub-row 28.2 per the 18/19/22/24/25/26 precedent) + the six-gate.

### 1.2 What phase 28 does NOT deliver (forward to §8)

See §8. Highlights: latency histograms (ADR-0060); real-ZooKeeper-server fixtures; the `access_log` proto field (upstream-not-implemented); WriteFilter `StopIteration`/halt + write-buffering semantics beyond what zookeeper needs; the remaining protocol proxies (`redis`/`mongo`/`kafka_broker`/`thrift`).

### 1.3 Phase-done as the THIRD Network-filters-family row landing

After phase 28, the family candidate count drops 5 → **4** (`redis`, `mongo`, `kafka_broker`, `thrift`). The WriteFilter seam landed here is the prerequisite `mongo_proxy` (the natural next candidate — also a both-direction sniffer with per-opcode stats + fault-delay) needs; `redis`/`thrift` additionally need terminal-proxy + upstream-routing surfaces of their own.

### 1.4 ADR-0045 split readiness — 2-way pre-split chosen per Q1

Per ADR-0045 §6, the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 28's full surface is anticipated to exceed the task gate as a single phase:

- The WriteFilter seam (interface + chain classification + reverse-order write dispatch + `writeChainConn` + tests): ~150–300 LoC.
- The zookeeperproxy package, request side (config parse + framing + opcode dispatch + counters + xid map + metadata): ~400–650 LoC.
- The response side (response framing + correlation + watch events + latency-threshold counters): ~300–500 LoC.
- Fixtures (3 dirs incl. hand-crafted ZK byte corpora) + fuzzer(s) + tag-extractor + docs: ~300–500 LoC.
- Task counts: ~12–16 (28.1) + ~10–14 (28.2) ≈ 22–30 total — straddles the gate as one phase; comfortably under it per sub-phase.

Total anticipated ~1150–1950 LoC / ~22–30 tasks → the 2-way pre-split at BRAINSTORM time (the project's FOURTH BRAINSTORM-time pre-split after 22/25/26). The split axis is natural and DIRECTION-PROGRESSIVE: 28.1 = the write seam + everything request-side (independently shippable: a request-side-observable zookeeper filter with live stats + fixtures); 28.2 = everything response-side (correlation + latency). Each sub-phase re-checks the gate at its own PLAN time.

### 1.5 Seed-stub alignment + package naming

No seed-stub for zookeeper exists. Phase 28.1 creates `internal/filter/network/zookeeperproxy/` from scratch (package `zookeeperproxy`; matches the `directresponse`/`snicluster` single-token-joined convention). The WriteFilter seam lands IN the existing `internal/filter/network/` framework package (types.go / chain.go / a new writeconn.go) — NOT a new package.

### 1.6 No prebrainstorm-notes branch

No `phase-28-*-prebrainstorm-notes` branch exists. Phase 28 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 28's relationship to prior framework deltas

Phase 28 continues the framework-delta-GROWTH posture. Prior lineage (abridged; see 26 BRAINSTORM §1.7 for the full list): 07.1 HTTP filter framework → … → 26.1 `internal/filter/network/` read-filter framework → 26.3 `internal/rbac/` + connection-scoped dynamic-metadata → 27 the connection-scoped upstream-cluster-override seam (ADR-0219). **Phase 28.1 — the `network.WriteFilter` seam** (the framework's third structural extension after the TerminalFilter seam at 26.2 and the override seam at 27), explicitly anticipated by ADR-0213's API-revision-allowance clause. This is the project's first deferred-with-allowance framework surface to be CONSUMED by the consumer it was deferred for — the YAGNI/extract-at-consumer discipline working as designed.

---

## 2. Design decisions

### 2.1 Subject selection: `zookeeper_proxy` *(Q0 → phase 28 row registered)*

**Decision:** Phase 28 = `envoy.filters.network.zookeeper_proxy` (proto `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`; bindings present in go-control-plane v1.32.4 — verified locally at brainstorm time).

**Rationale:** Smallest well-bounded remaining candidate; pure observability (no proxying logic, no upstream cluster surface of its own); exercises the read-filter framework + motivates the deferred write seam; its stat-primary nature exercises the `StatsAsserter` differential machinery as the load-bearing proof. `kafka_broker` carries contrib-image risk; `redis`/`thrift` are large terminal surfaces; `mongo` is better attempted AFTER the write seam exists.

**Anticipated ADRs:** ADR-0222 (the zookeeper_proxy filter; see §7).

### 2.2 Scope envelope: both-direction counters; histograms deferred *(Q1 → 2-way pre-split; ADR-0222/0223)*

**Decision:** Mirror upstream's per-opcode COUNTER surface in both directions (requests at 28.1; responses + latency-threshold fast/slow counters at 28.2). The latency HISTOGRAM family (`*_response_latency`) is NOT mirrored — deferred per ADR-0060 (the project-wide histogram deferral), recorded as a BEHAVIOR_CONTRACT coverage boundary (the `downstream_cx_*` precedent from phase 27).

**Rationale:** Request-only would leave the filter half-blind (its purpose is round-trip observability). Full histogram parity would require lifting ADR-0060 (registry + Prometheus exposition + differential comparison semantics for histograms) — a framework project of its own, not justified by one filter; the deterministic fast/slow threshold counters deliver the SLO-style latency signal upstream designed them for, using only existing counter machinery.

### 2.3 Write-seam shape: upstream-faithful `WriteFilter` + conn-wrap delivery *(Q2 → ADR-0221)*

**Decision:** `network.WriteFilter` interface — `OnWrite(buf *Buffer, endStream bool) Status`. A registered filter may implement `ReadFilter`, `WriteFilter`, or both. The write chain runs in REVERSE chain order (Envoy `Network::FilterManager::onWrite` parity). Delivery: the chain runtime wraps the downstream `net.Conn` it hands to `handleTerminal` in a `writeChainConn` whose `Write` runs the write chain before forwarding to the real downstream conn. `TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM untouched. `StopIteration`-on-write: pinned at SPEC (anticipated documented-unsupported — framework error/close — until a consumer needs halt+buffer semantics).

**Rationale:** The conn-wrap keeps the no-ripple property phase 27 established (no Handle-signature churn, no fake-double churn). Reverse-order write dispatch is the upstream-faithful semantic and costs nothing extra. The narrow "response-observer tap" alternative would diverge from Envoy's model and need rework at the first mutating/halting write filter (mongo fault-delay); the terminal-cooperative alternative reintroduces exactly the signature ripple 27 avoided.

**Anticipated ADRs:** ADR-0221 (the WriteFilter seam; consumes the ADR-0213 API-revision allowance).

### 2.4 Differential strategy: synthesized bytes + cross-side StatsAsserter *(Q3 → fixture envelope §6)*

**Decision:** Hermetic fixtures with hand-crafted jute-encoded ZK wire bytes; chain `[zookeeper_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 + envoy-go); the fixture backend replies with canned ZK response bytes; `StatsAsserter` compares the mirrored per-opcode counters cross-side; every assertion proven live via a deliberate-break (the `reference_differential_asserter_dispatch` discipline). NO real ZooKeeper server.

**Rationale:** The filter is a passive sniffer — a body differential is intrinsically vacuous (bytes pass through unchanged on both sides), so the stat comparison IS the proof. Synthesized bytes keep fixtures hermetic/deterministic per the project's fixture style; a real ZK server would add a container dependency + nondeterministic background traffic (pings, watches) that breaks byte/count determinism.

### 2.5 Stat surface: upstream-parity counters + envoy-go-strict departures *(self-answered per §9 precedent; SPEC pins roster)*

Upstream-parity per-opcode counter roster (anticipated scope `zookeeper.<stat_prefix>.*` — SPEC empirically pins the exact scope shape + opcode list against Envoy v1.37.2). The proto's 27-value opcode enum (Connect, Close, Create, Create2, CreateContainer, CreateTtl, Delete, Exists, GetAcl, GetAllChildrenNumber, GetChildren, GetChildren2, GetData, GetEphemerals, Multi, Ping, Reconfig, RemoveWatches, SetAcl, SetAuth, SetData, SetWatches, SetWatches2, AddWatch, Check, CheckWatches, Sync) is the brainstorm-time hypothesis for the per-opcode roster; upstream's stat list may add non-enum names (watch_event / connect_readonly) — SPEC pins.

### 2.6 Zero new third-party go.mod deps *(self-answered)*

The jute decode (length-prefixed big-endian primitives) is implemented in-house (~plain `encoding/binary`); the proto bindings are already vendored. Matches the 26/27 posture.

### 2.7 Dynamic-metadata emission *(self-answered posture; SPEC pins mirror-or-defer)*

Upstream zookeeper_proxy emits per-message dynamic metadata (opname / path / bytes / watch). envoy-go has the connection-scoped `*dynamicmetadata.Bucket` (ADR-0217, first written by rbac shadow at 26.3). The SPEC pins whether 28.1 mirrors the metadata writes (second production write through the bucket) or defers them (coverage-boundary record). Anticipated: MIRROR — the bucket exists, the write is cheap, and it exercises ADR-0217's second consumer.

---

## 3. Framework-survey result — 1 framework-seam extension + 1 NEW filter package + 0 new go.mod deps

### 3.1 EXTENSION: the `network.WriteFilter` seam *(per Q2; ADR-0221; lands at 28.1)*

In the existing `internal/filter/network/` package: the `WriteFilter` interface + chain-classification extension (read / write / both / terminal) + reverse-order write dispatch + the `writeChainConn` downstream-conn wrapper handed to `handleTerminal`. Consumes the ADR-0213 API-revision allowance. zookeeper_proxy = consumer #1; mongo_proxy = anticipated consumer #2.

### 3.2 NEW: `internal/filter/network/zookeeperproxy/` *(per Q0+Q1; ADR-0222/0223; lands across 28.1/28.2)*

Config parse + request decoder + per-connection xid→opcode map + counters + metadata (28.1); response decoder + correlation + latency-threshold counters (28.2). Implements BOTH `ReadFilter` and `WriteFilter`.

### 3.3 REUSES

- `internal/filter/network/` (26.1/26.2/27) — ReadFilter chain, Buffer, registry, builtins seam, chainRuntime.
- `internal/dynamicmetadata/` connection-scoped Bucket (22.2/26.3, ADR-0217) — per-message metadata writes (if §2.7 mirrors).
- `internal/stats/` (06.1) — counters via `NewCounterIfAbsent` dynamic-name convention; the `internal/stats/name.go` Prometheus tag-extractor arm pattern (26.3 `.rbac.` precedent).
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; untouched by 28.
- The differential harness + `StatsAsserter` (+ the fixture-dispatch + asserter-dispatch memory constraints).
- `envoy.extensions.filters.network.zookeeper_proxy.v3` proto bindings (go-control-plane v1.32.4 — verified present).

---

## 4. Per-route applicability — none (network filters are not route-scoped)

Per the 26 BRAINSTORM §4 confirmation: network filters carry no `typed_per_filter_config` surface. Not applicable to phase 28.

---

## 5. Stat surface hypothesis

### 5.1 28.1 (requests; SPEC pins)

~27–30 per-opcode `*_rq` counters + `decoder_error` + `request_bytes` (+ the per-opcode rq-bytes opt-in family if the SPEC keeps `enable_per_opcode_request_bytes_metrics` in the 28.1 envelope). Anticipated +26–35.

### 5.2 28.2 (responses + latency; SPEC pins)

~27–30 per-opcode `*_resp` counters + `watch_event` + `response_bytes` + the fast/slow threshold pairs (`*_resp_fast`/`*_resp_slow` — emitted only when `enable_latency_threshold_metrics`; whether the differential mirrors them per-opcode or for a pinned subset is a SPEC decision) + per-opcode resp-bytes opt-in. Anticipated +30–70.

### 5.3 Project stat count delta

136 → ~190–240 at family-row-done — **the largest single-filter stat addition in the project**. SPEC pins the exact mirrored roster; any upstream counters NOT mirrored land as BEHAVIOR_CONTRACT coverage-boundary records (the `downstream_cx_*` precedent).

### 5.4 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The `*_response_latency` histogram family unmirrored (ADR-0060); the `access_log` proto field (upstream `[#not-implemented-hide:]`) — parse posture pinned at SPEC (anticipated parse-accept-ignore mirroring upstream's not-implemented status, NOT a reject); WriteFilter StopIteration documented-unsupported.

---

## 6. Differential fixture envelope — anticipated three directories across 2 sub-phases

### 6.1 28.1 fixtures (+2)

- **`0046-zookeeper-requests`** (cross-side): chain `[zookeeper_proxy, tcp_proxy]` both sides; driver sends hand-crafted ZK connect + ping + getdata + create (+ a multi-opcode arm) byte sequences; backend = canned-bytes responder; `StatsAsserter` compares the mirrored `*_rq` + `decoder_error` counters; one garbage-bytes arm proves `decoder_error` + passthrough-not-broken; deliberate-break liveness proof recorded.
- **`0047-zookeeper-boot-reject`** (boot-reject; separate dir per the fixture-dispatch constraint): missing `stat_prefix` → both sides reject at boot.

### 6.2 28.2 fixtures (+1)

- **`0048-zookeeper-responses`** (cross-side): request+response round-trips; `StatsAsserter` on `*_resp` + `watch_event` + the threshold counters under DETERMINISTIC extreme thresholds (1 h → all fast; minimal → all slow).

### 6.3 Total

47 → **50**. SPEC pins exact numbering + arm rosters.

### 6.4 No conformance harness

No new conformance harness (matches 26/27). The h2spec + proxy-wasm gates re-run asserted-unaffected at each sub-phase six-gate.

---

## 7. Anticipated ADRs — ~3 ADRs (ADR-0221 .. ADR-0223)

Next-free ADR at master tip is **ADR-0221** (DECISIONS.md tail ADR-0220; the ADR-0209 escape-valve reserve stands unconsumed).

- **ADR-0221** *(28.1)* — the `network.WriteFilter` seam: interface + reverse-order write chain + `writeChainConn` delivery + StopIteration posture; consumes the ADR-0213 API-revision allowance.
- **ADR-0222** *(28.1)* — the `zookeeper_proxy` filter: both-direction counter envelope, request-side decoder + xid map + per-opcode `*_rq` surface, the ADR-0060 histogram-deferral coverage boundary, the dynamic-metadata posture.
- **ADR-0223** *(28.2)* — the response-side decoder + xid correlation + latency-threshold counter surface + the deterministic-threshold differential discipline.

Next-free after phase 28 phase-done ≈ **ADR-0224**. §Context drafts land at the parent/sub-phase SPECs; §Decision/§Consequences bodies at each IMPL per ADR-0044.

---

## 8. Deferred items

- **Latency histograms** (`*_response_latency`) — deferred per ADR-0060; coverage-boundary record at 28.2.
- **WriteFilter halt/buffer semantics** (`StopIteration` on the write path) — documented-unsupported until a consumer needs it (mongo fault-delay is the anticipated first).
- **`access_log` proto field** — upstream-not-implemented; mirrored as parse-accept-ignore (SPEC pins).
- **Real-ZooKeeper-server integration fixtures** — out of scope; hermetic synthesized bytes only.
- **The remaining protocol proxies** — `redis`, `mongo`, `kafka_broker`, `thrift` — each its own future family phase. `mongo_proxy` is the natural next (consumer #2 of the WriteFilter seam).
- **SASL / auth-data deep decode** — the filter counts `setauth` opcodes but does not decode credential payloads (upstream parity; SPEC confirms).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **26 BRAINSTORM §8 / Q4** ("Write-filter chain (`onWrite`/`WriteFilter`) — deferred; API-revision allowance in the 26.1 callbacks") — **CONSUMED by phase 28.1.** This is the explicit closure pickup: the deferral was written with zookeeper/mongo in mind; ADR-0221 cites ADR-0213's allowance clause.
- **27 BRAINSTORM/SPEC** — the connection-scoped override seam (ADR-0219) is NOT needed by zookeeper_proxy (it never overrides routing); no interaction.
- **26.3 D-26.3-4** (rbac SNI/mTLS arms differential gap) — not picked up here; carried.

---

## 10. BRAINSTORM-time open questions for parent-SPEC-time resolution (empirical pins against Envoy v1.37.2 per ADR-0004)

The parent SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25/26/27 SPEC precedent) against Envoy v1.37.2 source + the standard reference image + go-control-plane v1.32.4 bindings:

- **D28-1** *(SPEC-BLOCKING for Q3)* — confirm the standard `envoyproxy/envoy:v1.37.2` reference image ships `zookeeper_proxy` (boot a listener with it; it is a core extension at `source/extensions/filters/network/zookeeper_proxy`, not contrib — verify, since this kills-or-keeps the cross-side strategy).
- **D28-2** — exact proto field roster + PGV constraints + defaults (`stat_prefix` required; `max_packet_bytes` default 1 MiB; `default_latency_threshold` default 100 ms; the three `enable_per_opcode_*` flags; `access_log` not-implemented posture).
- **D28-3** — the exact stat scope + naming Envoy v1.37.2 emits (`zookeeper.<stat_prefix>.*` hypothesis) + the full per-opcode counter roster (the 27-value enum-derived list §2.5 vs upstream's stat macro list, which may add non-enum names incl. watch_event/connect_readonly) + which counters are eager vs lazy.
- **D28-4** — ZK wire-protocol framing pins: connect-request framing (no opcode), request framing (xid + opcode), response framing (xid + zxid + err), special xids (ping −2, auth −4, watch event −1), close semantics — enough to hand-craft the fixture byte corpora + the decoder contract.
- **D28-5** — decoder-error semantics: what upstream counts as `decoder_error`; passthrough behavior on decode failure (the filter must NEVER break the connection); `max_packet_bytes` exceeded behavior (skip vs error).
- **D28-6** — dynamic-metadata emission shape (namespace + key roster) + the §2.7 mirror-or-defer decision.
- **D28-7** — Envoy `Network::FilterManager` write-path semantics: reverse-order onWrite confirmation; what upstream does on write-path StopIteration (to phrase envoy-go's documented-unsupported posture faithfully).
- **D28-8** — latency-threshold counter semantics: exact fast/slow stat names, which opcodes get them, the threshold comparison semantics (≤ vs <), per-opcode override mapping (the 27-value enum → opcode ints).
- **D28-9** — the per-sub-phase task/LoC envelope confirming each sub-phase fits the ADR-0045 gate (re-checked again at each PLAN).
- **D28-10** — fuzzer envelope: the request-decoder fuzzer at 28.1 (37th) confirmed; whether 28.2 adds a response-decoder fuzzer (38th) or folds response decode into one fuzzer.

---

## 11. Prior-phase lessons applied

- **Differential liveness must be proven** (`reference_differential_asserter_dispatch`; the 0030 dead-assertion lesson). Applied: the StatsAsserter IS the load-bearing proof here; every fixture records a deliberate-break.
- **Cross-side XOR boot-reject per fixture dir** (`reference_differential_fixture_dispatch_constraint`). Applied: `0046`/`0048` cross-side; `0047` boot-reject; never mixed.
- **TypeURL via `proto.MessageName`, never the docs string** (`reference_network_filter_typeurl_extensions`). Applied: pinning-test at 28.1 Task 1.
- **OnNewConnection must Continue** (`reference_network_read_filter_onnewconnection_halts`). Applied: zookeeper_proxy's OnNewConnection is a no-op Continue; all decode work in OnData/OnWrite.
- **No-ripple seam additions** (phase-27 ADR-0219: thread state via ctx/wrappers, never signature churn). Applied: the conn-wrap write delivery keeps `TerminalFilter.Handle` + all fakes untouched.
- **Defer-with-allowance, consume-at-consumer** (ADR-0213 Q4 deferral → consumed here). Applied: ADR-0221 cites the allowance; the seam lands WITH its first consumer, never speculatively.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`). Applied at every IMPL.

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 28 = `zookeeper_proxy`, the third §9 Network-filters row; (Q1) both-direction COUNTER parity — requests at 28.1, responses + latency-threshold counters at 28.2; histograms deferred per ADR-0060; 2-way DIRECTION-PROGRESSIVE pre-split; (Q2) the upstream-faithful `network.WriteFilter` seam with reverse-order write chain + `writeChainConn` delivery (no Handle-signature ripple), consuming the ADR-0213 API-revision allowance; (Q3) hermetic synthesized-byte fixtures with cross-side `StatsAsserter` as the load-bearing proof. Self-answered per §9 precedent: upstream-parity counter roster + envoy-go-strict departures; zero new go.mod deps; dynamic-metadata anticipated-mirror (SPEC pins). One framework-seam extension (WriteFilter) + one NEW filter package (`zookeeperproxy`). Anticipated 3 ADRs (ADR-0221..0223), fixtures 47 → 50, stat surface 136 → ~190–240 (largest single-filter addition; SPEC pins), fuzzers 36 → 37–38.

The next session authors the parent SPEC (`superpowers:writing-plans` scoped to parent-SPEC authoring per the 22/24/25/26 parent-row precedent), executing the §10 D28-1..D28-10 empirical pins IN-SESSION against Envoy v1.37.2 per ADR-0004, anchoring the ADR-0221/0222/0223 §Context drafts, and formalizing the 2-way split surface-mapping. Per ADR-0106, parent row 28 registers `in-progress` with sub-phases listed at this BRAINSTORM-DONE commit; sub-rows 28.1 + 28.2 register `planned`.
