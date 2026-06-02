# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `phase 28 DONE (28.1a + 28.1b + 28.2 all done; parent row rolled) — awaiting next phase brainstorm` (2026-06-02, this commit) — **Phase 28 (`network-filter-zookeeper-proxy`) is COMPLETE.** Sub-phase 28.2 landed: the unified per-connection decoder (`requestDecoder` → `decoder` rename + write-side `writeBuf` reassembly), the response dispatch table (5-row: connect/watch/control/data/unknown), the per-connection `sync.Mutex` (ADR-0221 forward-pointer DISCHARGED — ADR-0223 §Decision item 4; empirically proven load-bearing by the -race test), latency-threshold counters (`recordLatency`; `<=` INCLUSIVE; wire-opcode-keyed overrides; injectable-duration boundary tests), `OnWrite` glue (replaces the 28.1 no-op), the 38th fuzzer (`FuzzZookeeperResponseDecode` — 1.8M execs/30s clean), `TCPZKResponder BackendKind=29` (fixed 10ms delay; D-S28.2-2 triggers; D-S28.2-1 corrected 28-byte watch frame), fixture `0048-zookeeper-responses` (4 listeners / 8 arms / GREEN cross-side on the FIRST run; R4 both breaks recorded), ADR-0223 §Decision/§Consequences body + BEHAVIOR_CONTRACT 28.2 bundle. Phase 28 as a whole delivered: the `network.WriteFilter` seam (ADR-0221) + complete both-direction zookeeper_proxy counter parity (ADR-0222/0223) across 3 sub-phases (28.1a write-seam+request, 28.1b read-seam+cross-side-proof, 28.2 response+latency); six gates GREEN LIVE (50/50 differential; h2spec 53/53; proxy-wasm 10/10); THIRD §9 Network-filters-family row closes.
- **phase-directory:** `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/` holds README.md + SPEC.md + PLAN.md + PROGRESS.md. The 28/28.1a/28.1b directories stay the durable records of their phases.
- **lifecycle-state:** `phase 28 DONE` — SKILL_ROUTING **state 0** (no phase in flight → `superpowers:brainstorming` for the next phase).
- **next-skill:** `superpowers:brainstorming` (the next §9 Network-filters-family candidate or any other ROADMAP row; mongo_proxy is the natural next [consumer #2 of the conn-wrap seam, per ADR-0221 anticipation] but the choice is the brainstorm's; 4 candidates remain: redis/mongo/kafka_broker/thrift).
- **last-commit:** `fde21c9` — `phase 28.2: zookeeper_proxy response decoder + per-connection mutex (ADR-0221 forward-pointer discharged) + latency fast/slow counters + TCPZKResponder/0048 cross-side GREEN + 38th fuzzer + phase-28 ATOMIC rollup [ADR-0223]`. Substantive predecessors on master: the 28.2-PLAN squash `6c60d36`, the 28.2-SPEC squash `11920be`, the 28.1b-IMPL squash `fdf40ea`, the 28.1a-IMPL squash `8703aeb`.
- **last-updated:** 2026-06-02
- **next-free ADR:** `ADR-0224` (UNCHANGED — DECISIONS.md tail STAYS **ADR-0223**; the 28.2 IMPL minted NO new ADR number; the ADR-0223 body landed in-place at Task 10).

---

## Project counts at this commit

- **active differential fixtures:** **50** (tail `0048-zookeeper-responses`).
- **fuzzers:** **38** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`).
- **stat surface:** **337** (BEHAVIOR_CONTRACT doc count; increments-only at 28.2 — zero creation delta).
- **DECISIONS.md tail:** **ADR-0223** (next-free **ADR-0224**).
- **conformance:** h2spec 53/53; proxy-wasm all 10 families PASS (run LIVE at the 28.2 six-gate, 2026-06-02).

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
