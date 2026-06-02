# Phase 28.1b — `network-filter-read-seam-and-zookeeper-requests-proof`

**Status:** SPEC authored (lifecycle-state 1 → 2 at this commit; ready for 28.1b PLAN authoring per `superpowers:writing-plans`). PLAN / PROGRESS / REVIEW artifacts still pending; land when this sub-phase enters subsequent lifecycle states.

**What this sub-phase is.** 28.1b is the **second/closing half of the invoked ADR-0045 28.1a/28.1b split** (user-approved 2026-06-02; DECISIONS.md ADR-0045 §AMEND). It is NOT a BRAINSTORM-time pre-split sub-phase: phase 28.1 (`network-filter-write-seam-and-zookeeper-requests`) was split at IMPL Task 16 when the `0046-zookeeper-requests` differential fixture exposed a SPEC design gap — envoy-go's chain runtime exits its read loop permanently at terminal handoff, so `zookeeper_proxy` decodes only the FIRST frame per connection while reference Envoy decodes EVERY frame. 28.1a (DONE, squash `8703aeb`) landed the WriteFilter seam + the complete zookeeperproxy request package + the disabled `0046` driver. 28.1b designs and lands the symmetric **read-side seam**, greens `0046`, adds `0047`, and lands the 28.1 completion bundle.

**Masters (read first, in order):**

1. [`../28.1-network-filter-write-seam-and-zookeeper-requests/SPEC.md`](../28.1-network-filter-write-seam-and-zookeeper-requests/SPEC.md) — **the 28.1 SPEC is this sub-phase's master SPEC.** Its §3 (WriteFilter seam), §4 (zookeeperproxy package), §6 (PARSE-REJECT roster), §7 (stat surface), §8 (fixture taxonomy incl. the `0047` design), §9 (BEHAVIOR_CONTRACT bundle), and §13 (R1–R7) all remain authoritative; the 28.1b SPEC extends §3 with the read-side seam and re-scopes the §8/§9 landing surface onto 28.1b.
2. [`../28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md`](../28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md) — the 28.1a per-task record, the Task-16 BLOCKED analysis (the cross-side divergence table + the green-then-reverted `readChainConn` proof-of-cause), and the 28.1a closure entry (gate outputs + split rationale).
3. [`../28-network-filter-zookeeper-proxy/SPEC.md`](../28-network-filter-zookeeper-proxy/SPEC.md) + [`BRAINSTORM.md`](../28-network-filter-zookeeper-proxy/BRAINSTORM.md) — the parent SPEC (empirical pins §11; the §10.1 pre-authorized split axis the invoked split diverges from) and parent BRAINSTORM.

**Authoritative sub-phase SPEC:** [`./SPEC.md`](./SPEC.md) — the read-side seam design (`readChainConn` + the replay path + the `Buffer.TotalAppended` decoder-feed re-base + the R1 wrap predicate + the concurrency pins), the `0046`/`0047` proof surface, and the 28.1 completion bundle.

**Scope at 28.1b** (per ADR-0045 §AMEND + STATE.md):

- (a) The symmetric **read-side seam**: a `readChainConn` conn-wrap that re-feeds post-terminal-handoff socket reads back through the read-filter chain, so a request-decoding read filter sees EVERY frame per connection (reference-Envoy re-iteration parity). R1-preserving wrap predicate; bounded-memory replay; the decoder-feed re-base onto `Buffer.TotalAppended`.
- (b) **`0046-zookeeper-requests` RE-ENABLED + GREEN** (uncomment the runner blank-import; drop the DISABLED banner) + the R4 deliberate-break liveness proof + the fixture README.
- (c) **`0047-zookeeper-boot-reject`** (the 28.1 SPEC §8.2 design, landed here).
- (d) **The 28.1 completion bundle**: the ADR-0221 + ADR-0222 §Decision/§Consequences bodies in place (no new ADR number; ADR-0221's body covers BOTH conn-wrap seams per its §Context §AMEND), the BEHAVIOR_CONTRACT 28.1 bundle incl. the stat-table roll **136 → 337**, and the ROADMAP advance (sub-row 28.1b → `done`; parent row 28 STAYS `in-progress` — the rollup is 28.2's).

**This directory vs the 28.1 directory.** Per the STATE.md sub-phase-directory decision (ratified at this SPEC — see SPEC.md §6.4): the 28.1 directory remains the durable record of 28.1a (its SPEC/PLAN/PROGRESS are NOT rewritten); 28.1b's SPEC/PLAN/PROGRESS/REVIEW live HERE.

**Predecessor:** Phase 28.1a DONE (squash `8703aeb`).

**Successor:** Phase 28.2 (`network-filter-zookeeper-responses-and-latency`) — depends on 28.1 (i.e. on 28.1b completing the 28.1 surface).
