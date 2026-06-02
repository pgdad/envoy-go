# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `phase 28.2 SPEC done (next = 28.2 PLAN)` (2026-06-02, this commit) — **the phase 28.2 (`network-filter-zookeeper-responses-and-latency`) SPEC is authored + spec-document-reviewer APPROVED (first iteration).** The sub-phase directory `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/` (README + SPEC) was created at this session; ROADMAP sub-row 28.2 flipped `planned → in-progress` at this commit (ADR-0106 / the 26.x/28.1b precedent). What the 28.2 SPEC pins: **the unified per-connection decoder** (`requestDecoder` → `decoder` rename + write-side `writeBuf` reassembly + response dispatch by leading-int32 sniffing: connect-response special framing / watch events xid −1 / control FIFO pop / data-map erase-on-lookup / unknown → `decoder_error`; decode-failure → abandon-no-resync, AMEND-A8 symmetry; NO write-side TotalAppended — `writeChainConn.Write` allocates a fresh per-Write Buffer so OnWrite sees each byte exactly once); **the §3.6 synchronization design DISCHARGING the ADR-0221 §Consequences forward-pointer** (a per-connection `sync.Mutex` guarding EXACTLY the two correlation maps; reassembly buffers lock-free per-goroutine; entries copied out under the lock; the ADR-0223 §Decision body records it at IMPL); **the connect_readonly → connect response-opname mapping** (the closed-roster panic trap — `respOpNames` has no `connect_readonly_resp`); **the latency fast/slow surface** (`latency <= threshold` → fast INCLUSIVE; wire-opcode-keyed overrides; consumption-only — all latency config parsed at 28.1); the parent **D-P4/P6/P9 resolutions** (latency rejects unit-test-only / separate 38th fuzzer / the fixed-delay `TCPZKResponder` deterministic-threshold construction) — all user-confirmed at the SPEC design dialogue; fixture **`0048-zookeeper-responses`** (4 listeners; 8 arms incl. the wire-opcode-keyed override arm + R4 `-count=1`); the **ADR-0045 split-gate re-check (~360–490 production LoC / ~11 tasks → NO split)**; and the **phase-28 completion bundle** (ADR-0223 body + BEHAVIOR_CONTRACT 28.2 bundle + the ATOMIC parent-row-28 ROLLUP + the six-gate) as the IMPL's final tasks.
- **phase-directory:** `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/` holds README.md + SPEC.md (this commit). PLAN.md / PROGRESS.md / REVIEW.md land at their lifecycle states. The 28/28.1/28.1b directories stay the durable records of their phases.
- **lifecycle-state:** `phase 28.2 SPEC done (next = 28.2 PLAN)` — SKILL_ROUTING **state 2** (SPEC.md exists, PLAN.md does not → `superpowers:writing-plans` scoped to the 28.2 PLAN).
- **next-skill:** `superpowers:writing-plans` (the **28.2 PLAN** — bite-sized TDD decomposition of the 28.2 SPEC §11 task spine [~11 tasks]: baselines gate → decoder rename + mutex → response framing/dispatch → correlation consumption → latency counters → OnWrite glue + race test → 38th fuzzer → `TCPZKResponder` BackendKind + runner arm → `0048` fixture + R4 → ADR-0223 body + BEHAVIOR_CONTRACT bundle → six-gate + ATOMIC parent rollup). PLAN-time decisions to resolve: D-S28.2-2 (responder trigger encoding + delay constant); the PLAN re-checks the split-gate. Per `feedback_execution_style` the PLAN authoring is self-driven; the 28.2 IMPL executes via `superpowers:subagent-driven-development` (subagents commit LOCAL-ONLY; the controller squash-merges + pushes — `feedback_subagents_no_push`).
- **last-commit:** `11920be` — `phase 28.2 SPEC: response decoder + latency-threshold counters + per-connection mutex + TCPZKResponder/0048 design + phase-28 rollup plan; ROADMAP 28.2 → in-progress; ADR-0223 §Context §AMEND`. Substantive predecessor on master: the 28.1b-IMPL squash `fdf40ea`.
- **last-updated:** 2026-06-02
- **next-free ADR:** `ADR-0224` (UNCHANGED — DECISIONS.md tail STAYS **ADR-0223**; the 28.2 SPEC minted NO new ADR number — only a one-line §AMEND on ADR-0223 §Context landed at this commit. The ADR-0223 §Decision/§Consequences body lands at the 28.2 IMPL and records the per-connection mutex; no further phase-28 ADR numbers anticipated).

---

## Project counts at this commit

- **active differential fixtures:** **49** (tail `0047-zookeeper-boot-reject`; unchanged — `0048-zookeeper-responses` lands at the 28.2 IMPL → 50).
- **fuzzers:** **37** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`; unchanged — the 38th lands at the 28.2 IMPL).
- **stat surface:** **337** (BEHAVIOR_CONTRACT doc count; unchanged — 28.2 wires increments only, zero creation delta).
- **DECISIONS.md tail:** **ADR-0223** (next-free **ADR-0224**).
- **conformance:** h2spec 53/53; proxy-wasm all 10 families PASS (as of the 28.1b six-gate).

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
