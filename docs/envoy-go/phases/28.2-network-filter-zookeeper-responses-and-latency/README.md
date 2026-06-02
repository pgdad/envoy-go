# Phase 28.2 — `network-filter-zookeeper-responses-and-latency`

**Status:** SPEC authored (lifecycle-state 1 → 2 at this commit; ready for 28.2 PLAN authoring per `superpowers:writing-plans`). PLAN / PROGRESS / REVIEW artifacts still pending; land when this sub-phase enters subsequent lifecycle states.

**What this sub-phase is.** 28.2 is the **second/FINAL sub-phase of the phase-28 BRAINSTORM-time pre-split** (`28` parent; the 2-way DIRECTION-PROGRESSIVE split settled at BRAINSTORM Q1). It completes `zookeeper_proxy`'s round-trip observability: the RESPONSE-side decoder in `OnWrite`, xid correlation consuming the 28.1-laid structures (R5), the per-opcode `*_resp`/`watch_event` counter increments, latency measurement + the deterministic `enable_latency_threshold_metrics` fast/slow counter surface, fixture `0048-zookeeper-responses`, the 38th fuzzer, and the phase-28 completion bundle including the **parent-row-28 ROLLUP** (the THIRD §9 Network-filters-family row closes here).

**Masters (read first, in order):**

1. [`../28-network-filter-zookeeper-proxy/SPEC.md`](../28-network-filter-zookeeper-proxy/SPEC.md) — **the parent SPEC is this sub-phase's master.** Its §3.2 (the 28.2 scope detail), §5 (proto roster), §6.3 (the 28.2 PARSE-REJECT arms), §7 (the 201-counter roster), §8.3 (the `0048` fixture envelope), §11.4/§11.5/§11.7 (response framing + decoder-error + latency empirical pins), and §12 (D-P4/P6/P9) all remain authoritative; this SPEC executes + refines them.
2. [`../28.1b-network-filter-read-seam-and-zookeeper-requests-proof/SPEC.md`](../28.1b-network-filter-read-seam-and-zookeeper-requests-proof/SPEC.md) — §3.6 (the concurrency analysis + **THE 28.2 synchronization forward-pointer this SPEC discharges**) + §3.5 (observational boundaries).
3. [`../28.1-network-filter-write-seam-and-zookeeper-requests/SPEC.md`](../28.1-network-filter-write-seam-and-zookeeper-requests/SPEC.md) + [`PROGRESS.md`](../28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md) — the as-built request surface + the correlation structures laid for this response side.
4. `docs/envoy-go/DECISIONS.md` ADR-0221 (the conn-wrap seam, both halves, incl. the §Consequences forward-pointer) + ADR-0222 (the request package) + ADR-0223 §Context (this sub-phase's ADR; §Decision/§Consequences body lands at the 28.2 IMPL).

**Authoritative sub-phase SPEC:** [`./SPEC.md`](./SPEC.md) — the response decoder + correlation consumption + the per-connection mutex + latency-threshold counters + the `TCPZKResponder` backend + fixture `0048` + the completion bundle.

**Scope at 28.2** (per the parent SPEC §3.2 + ROADMAP row 28.2):

- (a) The **response decoder in `OnWrite`**: response framing (xid sniffing → connect-response special / watch events / control FIFO / data-map correlation), per-opcode `*_resp` + `response_bytes` + flag-gated `*_resp_bytes` increments, decode-failure → `decoder_error` + abandon (AMEND-A8 symmetry).
- (b) The **per-connection synchronization** (the 28.1b §3.6 / ADR-0221 §Consequences obligation): a `sync.Mutex` on the decoder guarding the two correlation maps against the goroutine-A (request decode) vs goroutine-B (response decode) race.
- (c) **Latency measurement + the fast/slow threshold counters** (`latency <= threshold` → fast; wire-opcode-keyed overrides; AMEND-A10).
- (d) Fixture **`0048-zookeeper-responses`** (cross-side `StatsAsserter`; the new ZK-aware `TCPZKResponder` backend with a fixed pre-response delay making BOTH threshold arms deterministic) + the R4 deliberate-break.
- (e) The **38th fuzzer** (`FuzzZookeeperResponseDecode`).
- (f) The **completion bundle**: ADR-0223 §Decision/§Consequences body in place, the BEHAVIOR_CONTRACT 28.2 bundle (incl. the latency-HISTOGRAM coverage boundary), the six-gate, and the **parent-row-28 ROLLUP** (parent + sub-row 28.2 flip `→ done` ATOMICALLY per the 18/19/22/24/25/26 precedent).

**Predecessor:** Phase 28.1 family complete (28.1a squash `8703aeb`; 28.1b squash `fdf40ea`).

**Successor:** none within phase 28 — this sub-phase CLOSES the family row. After phase 28 phase-done, 4 §9 Network-filters candidates remain (`redis`/`mongo`/`kafka_broker`/`thrift`); `mongo_proxy` is the natural next (consumer #2 of the ADR-0221 seam).
