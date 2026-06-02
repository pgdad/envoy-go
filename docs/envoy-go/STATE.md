# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `phase 28.2 PLAN done (next = 28.2 IMPL)` (2026-06-02, this commit) — **the phase 28.2 (`network-filter-zookeeper-responses-and-latency`) PLAN is authored + plan-document-reviewer APPROVED (first iteration; 2 advisories folded in).** `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PLAN.md` is the **11-task** bite-sized TDD decomposition of the 28.2 SPEC §11 spine: (1) baselines/anchors gate → (2) decoder rename (`requestDecoder` → `decoder`) + `writeBuf`/`mu` fields + request-path locking → (3) write-side reassembly + uncorrelated dispatch (watch/unknown/short/oversized) → (4) correlated dispatch + correlation consumption (erase-on-lookup / FIFO pop / the connect_readonly→connect mapping) + byte accounting → (5) latency-threshold counters (`<=` INCLUSIVE; wire-opcode-keyed overrides; injectable-duration `recordLatency`) → (6) `OnWrite` glue + the §3.6 concurrent request/response race test (`-race -count=5`) → (7) the 38th fuzzer (`FuzzZookeeperResponseDecode`) → (8) `TCPZKResponder` BackendKind=29 + `acceptZKResponder` runner arm → (9) `0048-zookeeper-responses` 4-listener/8-arm cross-side fixture + R4 (`-count=1`) + README → (10) ADR-0223 §Decision/§Consequences body + BEHAVIOR_CONTRACT 28.2 bundle → (11) six-gate (50-dir differential) + STATE.md + **the ROADMAP ATOMIC rollup (sub-row 28.2 AND parent row 28 → done)** + next-prompt.txt. **The ADR-0045 split-gate was RE-CHECKED at PLAN: NO split** (11 tasks / ~360–490 production LoC). PLAN-time resolutions: **D-S28.2-2** (the `TCPZKResponder` triggers: `getacl` → wrong-xid, `exists` → watch-event push; fixed delay = 10 ms) + **D-S28.2-4** (parallel frame-scanner methods) + 3 PLAN-discovered refinements (write-side `responseError` abandons `writeBuf`; correlate-then-validate order — upstream parity; injectable-duration `recordLatency` signature). D-S28.2-1/-3/-5 stay IMPL-owned (Tasks 3/4/1 first actions).
- **phase-directory:** `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/` holds README.md + SPEC.md + PLAN.md (this commit). PROGRESS.md is created at IMPL Task 1; REVIEW.md lands at its lifecycle state. The 28/28.1/28.1b directories stay the durable records of their phases.
- **lifecycle-state:** `phase 28.2 PLAN done (next = 28.2 IMPL)` — SKILL_ROUTING **state 3** (PLAN.md exists, implementation incomplete → `superpowers:subagent-driven-development` per `feedback_execution_style`, or `superpowers:executing-plans`).
- **next-skill:** `superpowers:subagent-driven-development` (the **28.2 IMPL** — execute the 11-task PLAN task-by-task; subagents commit LOCAL-ONLY, the controller squash-merges + pushes at stage-close — `feedback_subagents_no_push`/`feedback_push_to_origin`; per-task `gofmt -l` + `golangci-lint` on touched packages — `feedback_pertask_gofmt_lint`; ALL differential runs `-count=1` — `reference_differential_break_protocol_count1`). The IMPL's final task executes the **ATOMIC parent-row-28 ROLLUP** (sub-row 28.2 AND parent row 28 → `done` in the same commit; family candidates 5 → 4) and advances STATE to SKILL_ROUTING state 0 (next-phase brainstorm).
- **last-commit:** `6c60d36` — `phase 28.2 PLAN: 11-task TDD decomposition (response decoder + mutex + latency + TCPZKResponder/0048 + rollup); split-gate NO-split; D-S28.2-2 resolved; reviewer approved`. Substantive predecessor on master: the 28.2-SPEC squash `11920be`.
- **last-updated:** 2026-06-02
- **next-free ADR:** `ADR-0224` (UNCHANGED — DECISIONS.md tail STAYS **ADR-0223**; the 28.2 PLAN minted NO new ADR number and touched no docs beyond PLAN.md/ROADMAP/STATE/next-prompt. The ADR-0223 §Decision/§Consequences body lands at the 28.2 IMPL Task 10).

---

## Project counts at this commit

- **active differential fixtures:** **49** (tail `0047-zookeeper-boot-reject`; unchanged — `0048-zookeeper-responses` lands at the 28.2 IMPL Task 9 → 50).
- **fuzzers:** **37** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`; unchanged — the 38th lands at the 28.2 IMPL Task 7).
- **stat surface:** **337** (BEHAVIOR_CONTRACT doc count; unchanged — 28.2 wires increments only, zero creation delta).
- **DECISIONS.md tail:** **ADR-0223** (next-free **ADR-0224**).
- **conformance:** h2spec 53/53; proxy-wasm all 10 families PASS (as of the 28.1b six-gate; re-run at the 28.2 six-gate).

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
