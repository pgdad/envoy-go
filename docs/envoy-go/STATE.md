# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `phase 28.1b IMPL done (next = 28.2 SPEC)` (2026-06-02, this commit) — **phase 28.1b (`network-filter-read-seam-and-zookeeper-requests-proof`), the second/closing half of the user-approved ADR-0045 28.1a/28.1b invoked split, is IMPLEMENTED + all six gates GREEN.** The 10-task TDD PLAN executed via `superpowers:subagent-driven-development`. What 28.1b landed: **the symmetric READ-side seam** — `readChainConn` (innermost conn-wrap; replay-before-return; EOF endStream replay; non-EOF error passthrough) + `chainRuntime.replayRead` (all read filters in chain order, Status IGNORED — observational §3.5, drain-after-pass) + `Buffer.TotalAppended()` (int64, D-S28.1b-1) decoder-feed re-base onto the monotonic counter (the §3.3 equivalence — on never-drained executions `TotalAppended()==Len()`, so the re-based feed is byte-identical and existing decoder assertions are unchanged) + the SHARED `len(writeFilters) > 0` wrap predicate (R1 — zero-write-filter `tcp_proxy`-only chains stay UNWRAPPED) composing `writeChainConn(prefixConn(readChainConn(conn)))` (readChainConn innermost so prefixConn's replayed prefix bytes are NOT re-fed) + the soundness invariant (every appended byte seen exactly once) + the §3.6 concurrent-pumps race test (net.Pipe unit shape, `-race -count=5` clean — the 28.1b race surface is empty, OnWrite is a no-op). **`0046-zookeeper-requests` RE-ENABLED + cross-side `StatsAsserter` GREEN on all 7 arms** (arms 2/3/4 = the multi-socket-read R8 re-iteration proof the seam closes; `request_bytes=307` cross-side equality) + R4 deliberate-break liveness. `0047-zookeeper-boot-reject` landed (port 15049; symmetric PGV-mirror, the `0044` template). **The completion bundle** — ADR-0221 (both conn-wrap seam halves) + ADR-0222 §Decision/§Consequences bodies landed IN PLACE (no new ADR number; DECISIONS tail STAYS ADR-0223), BEHAVIOR_CONTRACT 28.1 bundle incl. the stat-table roll **136 → 337** (cross-side-PROVEN by the green 0046) + the three observational post-handoff boundaries (§3.5) + the conn-wrap-seam framework block carrying the **28.2 synchronization forward-pointer**. The **phase-28.1 family is now complete except the 28.2 response side**; parent row 28 STAYS `in-progress` (the rollup is 28.2's job — the established final-sub-phase precedent, D-28.1b-4).
- **phase-directory:** `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/` now holds README.md + SPEC.md + PLAN.md + **PROGRESS.md** (the 10-task IMPL record). The 28.1 directory (`docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/`) stays the durable, unmodified record of 28.1a. The parent directory `docs/envoy-go/phases/28-network-filter-zookeeper-proxy/` stays canonical; **the 28.2 sub-phase directory is created at the 28.2 SPEC session.**
- **lifecycle-state:** `phase 28.1b IMPL done (next = 28.2 SPEC)` — SKILL_ROUTING **state 1** (the 28.2 sub-phase directory does not yet exist → `superpowers:brainstorming` scoped to the 28.2 SPEC, per the per-sub-phase precedent).
- **next-skill:** `superpowers:brainstorming` (the **28.2 SPEC** — the response decoder in `OnWrite` + xid correlation CONSUMPTION against the 28.1-laid tracking map + per-opcode `*_resp`/`watch_event` counters + the deterministic `enable_latency_threshold_metrics` fast/slow latency-threshold counter surface + `0048-zookeeper-responses` (cross-side StatsAsserter; DETERMINISTIC threshold arms) + the parent-row-28 ROLLUP). **The 28.2 SPEC MUST carry the 28.1b SPEC §3.6 correlation-structure synchronization obligation into ADR-0223** (a per-connection mutex on the correlation maps — goroutine B's OnWrite response decoder vs goroutine A's replay-path request decoder). Per `feedback_execution_style` brainstorm is self-driven; the subsequent PLAN executes subagent-driven.
- **last-commit:** `(filled at stage-close by the controller — the squash SHA)`. Substantive predecessor on master: the 28.1b-PLAN squash `eff30e8`.
- **last-updated:** 2026-06-02
- **next-free ADR:** `ADR-0224` (UNCHANGED — DECISIONS.md tail STAYS **ADR-0223**; the 28.1b IMPL minted NO new ADR number — the ADR-0221 (both halves) + ADR-0222 §Decision/§Consequences bodies landed IN PLACE per ADR-0044 in-place-body discipline. The ADR-0223 body lands at the 28.2 IMPL — and **ADR-0223 MUST carry the 28.1b SPEC §3.6 correlation-structure synchronization obligation**; no further phase-28 ADR numbers anticipated).

---

## Project counts at this commit

- **active differential fixtures:** **49** (tail `0047-zookeeper-boot-reject`; `0046` re-enabled + `0047` added at 28.1b).
- **fuzzers:** **37** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`; unchanged — no new fuzzer at 28.1b).
- **stat surface:** **337** (BEHAVIOR_CONTRACT doc count; the 28.1 `+201` zookeeper roster rolled 136 → 337 at 28.1b Task 9, cross-side-PROVEN by the green 0046).
- **DECISIONS.md tail:** **ADR-0223** (next-free **ADR-0224**).
- **conformance:** h2spec 53/53; proxy-wasm all 10 families PASS.

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
