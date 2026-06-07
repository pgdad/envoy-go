# Phase 30 Implementation Plan — reference-image pin-refresh to `envoyproxy/envoy:contrib-v1.37.2`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Swap the SHA-pinned differential reference image from the standard Envoy variant to the contrib variant of the *same* upstream version (`envoyproxy/envoy:v1.37.2` → `envoyproxy/envoy:contrib-v1.37.2`), re-baseline all 54 existing differential fixtures byte-identical-PASS against it, and land ADR-0227's §Decision/§Consequences body superseding ADR-0008 — ZERO production LoC.

**Architecture:** A pure pin-refresh per `ENVOY_TARGET.md` doctrine D-3.7. The ONLY runtime-load-bearing change is the two pin lines in `docs/envoy-go/ENVOY_TARGET.md`: `test/differential/harness.go::parseEnvoyTarget` reads `**Tag:**`/`**SHA256:**` from it, and `StartReferenceProxy(ctx, pin, …)` boots `pin.SHA256` as the testcontainers image, so editing the two pin lines atomically repoints every reference-booting fixture. No Go code changes; the contrib image is the standard image plus extra compiled-in extensions (a behavioral superset), proven by the unchanged 54-fixture differential surface.

**Tech Stack:** Docker / testcontainers-go (the differential harness — unchanged); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227, superseding ADR-0008); Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008; UNCHANGED — phase 30 adds no go.mod dep).

---

## Phase-shape notes (read before Task 1)

- **Zero production LoC → no TDD red/green.** This phase touches NO Go production or test code (SPEC §4). The "test" for the substantive task (T3) is the full differential suite *run* against the flipped pin — a gate, not a new assertion. Each task below is therefore structured as *act → verify-with-exact-command → commit*, not write-failing-test → implement.
- **The only runtime-load-bearing edit is `ENVOY_TARGET.md`** (SPEC §3.1). Everything else is documentation: `BEHAVIOR_CONTRACT.md`, `DECISIONS.md`, `ROADMAP.md`, `STATE.md`, `next-prompt.txt`, `PROGRESS.md`.
- **The image is already pulled locally** with the digest the SPEC captured (`sha256:7edd5b0f…`); T1 verifies this rather than re-pulling.
- **Frozen sites — do NOT touch** (SPEC §3.4 / §7.2): `BEHAVIOR_CONTRACT.md:548` (the phase-06.2 access-log capture-provenance block); every historical `PROGRESS.md`/`SPEC.md`/`PLAN.md` mention of the old `c5e8a68e…` digest; the fixture-driver comments; the zookeeper `stats.go`/`stats_test.go` probe-date headers; the `harness_test.go` `v1.34.0`/`abc123def456` parser sample (SPEC §8 / D30-5).
- **Subagent discipline:** subagents commit local-only (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Each subagent runs `gofmt -l` only if it touched Go (it won't) and otherwise verifies its named gate with the exact command.
- **ADR-0045 split-gate:** 0 production LoC / 6 tasks → **NO split** (SPEC §10.1).

### Anchors verified at PLAN authoring (master tip `7869b07`)

| Anchor | Value | Source |
|---|---|---|
| differential fixtures | **54** (tail `0052-mongo-fault-delay`) | `ls -d test/fixtures/[0-9]* \| wc -l` |
| fuzzers | **39** | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` |
| stat surface | **360** | BEHAVIOR_CONTRACT doc count |
| BackendKind tail | **30** (`TCPMongoResponder`) | SPEC §13 |
| DECISIONS tail | **ADR-0227** (§Context anchored; §Decision/§Consequences placeholders present) | `DECISIONS.md` |
| contrib image digest | `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8` | `docker inspect` (present locally) |
| `ENVOY_TARGET.md` Tag line | line **3** | `envoyproxy/envoy:v1.37.2` (to flip) |
| `ENVOY_TARGET.md` SHA256 line | line **4** | `…c5e8a68e…` (to flip) |
| `ENVOY_TARGET.md` Pinned in | line **7** | `ADR-0008` (to flip → ADR-0227) |
| `ENVOY_TARGET.md` Last verified | line **8** | `2026-04-21` (to flip → IMPL date) |
| BEHAVIOR_CONTRACT pin lines | **728**, **804** (update); **548** frozen | `grep -n c5e8a68e` |

---

## Task 1: Baselines / anchors gate

Establish the regression baseline: confirm every count the SPEC pins, confirm the contrib image is present locally at the exact digest, and run a clean six-gate **on the UNCHANGED (standard-image) pin** so any later divergence is attributable to the swap alone.

**Files:**
- Read-only: `test/fixtures/`, `internal/**/fuzz_test.go`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/ENVOY_TARGET.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- No file is modified in this task.

- [ ] **Step 1: Verify the static anchors**

Run:
```bash
echo "fixtures: $(ls -d test/fixtures/[0-9]* | wc -l) (expect 54, tail $(ls -d test/fixtures/[0-9]* | tail -1))"
echo "fuzzers:  $(grep -rh '^func Fuzz' $(find ./internal -name fuzz_test.go) | wc -l) (expect 39)"
echo "DECISIONS tail: $(grep -oE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1) (expect ADR-0227)"
grep -n 'c5e8a68e' docs/envoy-go/BEHAVIOR_CONTRACT.md
```
Expected:
- `fixtures: 54 (… tail test/fixtures/0052-mongo-fault-delay)`
- `fuzzers:  39`
- `DECISIONS tail: ## ADR-0227`
- the `grep -n` prints exactly three lines: **548** (frozen), **728**, **804**.

- [ ] **Step 2: Verify the contrib image is present locally at the captured digest**

Run:
```bash
docker inspect --format='{{index .RepoDigests 0}}' envoyproxy/envoy:contrib-v1.37.2
```
Expected (exact):
```
envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8
```
If the image is absent, pull it first: `docker pull envoyproxy/envoy:contrib-v1.37.2` then re-run the inspect and confirm the digest matches byte-for-byte. **If the digest does NOT match `7edd5b0f…`, STOP** and surface it — the SPEC's central evidence is keyed to this digest (SPEC §5.1).

- [ ] **Step 3: Confirm the current pin is still the standard image (the un-flipped baseline)**

Run:
```bash
grep -nE '^\*\*(Tag|SHA256|Pinned in|Last verified):\*\*' docs/envoy-go/ENVOY_TARGET.md
```
Expected: Tag `envoyproxy/envoy:v1.37.2` (line 3), SHA256 `…c5e8a68e…` (line 4), Pinned in `ADR-0008` (line 7), Last verified `2026-04-21` (line 8). This is the regression baseline; T2 flips it.

- [ ] **Step 4: Run the six-gate on the UNCHANGED pin (regression baseline)**

Run each, confirming EXIT=0:
```bash
go build ./... && echo "BUILD ok"
go vet ./... && echo "VET ok"
golangci-lint run && echo "LINT ok"
go test -race -short ./... 2>&1 | tail -20
go test ./test/differential/... -count=1 2>&1 | tail -10
go test ./test/conformance/... -count=1 2>&1 | tail -10
```
Expected:
- `BUILD ok`, `VET ok`, `LINT ok`.
- `-race -short`: all packages `ok` (a transient HTTP-fixture flake on a first run is a known artifact — re-run the affected package in isolation to confirm; see STATE 29.3 note).
- differential: `ok github.com/esalaine/envoy-go/test/differential …` EXIT=0, all 54 dirs PASS **against the standard image** (this is the before-state).
- conformance: `ok …/test/conformance/h2spec` (53/53) + `ok …/test/conformance/proxy-wasm` (10/10), EXIT=0.

This establishes that master is green BEFORE the swap — so T3's re-baseline isolates the image change.

- [ ] **Step 5: Commit (baseline-record only — no file change)**

This task changes no files (it is a verification gate). There is nothing to commit. Record the baseline result in the task hand-off note to the controller (counts confirmed, image digest confirmed, six-gate green on the standard pin). Proceed to Task 2.

---

## Task 2: Flip `ENVOY_TARGET.md` to the contrib pin

The single runtime-load-bearing edit. Repoint the four pin lines to the contrib variant. The release-notes URL is UNCHANGED (same upstream version `v1.37.2`).

**Files:**
- Modify: `docs/envoy-go/ENVOY_TARGET.md:3-8`

- [ ] **Step 1: Edit the Tag line (line 3)**

Change:
```
**Tag:** `envoyproxy/envoy:v1.37.2`
```
to:
```
**Tag:** `envoyproxy/envoy:contrib-v1.37.2`
```

- [ ] **Step 2: Edit the SHA256 line (line 4)**

Change:
```
**SHA256:** `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`
```
to:
```
**SHA256:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
```

- [ ] **Step 3: Edit the Pinned in + Last verified lines (lines 7-8)**

Change:
```
**Pinned in:** ADR-0008
**Last verified:** 2026-04-21
```
to (use the actual IMPL date for `Last verified`):
```
**Pinned in:** ADR-0227 (supersedes ADR-0008)
**Last verified:** 2026-06-06
```
Leave line 5 (`**Upstream release notes:** https://www.envoyproxy.io/docs/envoy/v1.37.2/…`) and line 6 (`**Envoy proto major version:** v3`) UNCHANGED — same upstream version.

- [ ] **Step 4: Verify the parser still extracts the new pin**

Run:
```bash
go test ./test/differential/ -run TestParseEnvoyTarget -count=1 -v 2>&1 | tail -15
```
Expected: PASS. (`TestParseEnvoyTarget_PullsTagAndDigest` feeds a hardcoded `v1.34.0` *sample* doc — it tests the parser, not the live pin — so it is unaffected by the flip and must stay green. This confirms the file is still well-formed for `parseEnvoyTarget`.)

- [ ] **Step 5: Confirm the booted image is now the contrib digest**

Run:
```bash
grep -nE '^\*\*(Tag|SHA256|Pinned in|Last verified):\*\*' docs/envoy-go/ENVOY_TARGET.md
```
Expected: Tag `envoyproxy/envoy:contrib-v1.37.2`, SHA256 `…7edd5b0f…`, Pinned in `ADR-0227 (supersedes ADR-0008)`, Last verified `2026-06-06`.

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/ENVOY_TARGET.md
git commit -m "phase 30 T2: flip ENVOY_TARGET.md pin to envoyproxy/envoy:contrib-v1.37.2 (ADR-0227)"
```

---

## Task 3: Re-baseline the 54-fixture differential suite against the contrib pin

The authoritative re-run (SPEC §5.2 is the advance evidence; this is the load-bearing gate). With the pin flipped in T2, the harness now boots `envoyproxy/envoy:contrib-v1.37.2` for every reference-side fixture.

**Files:**
- No file modified (this is a gate run on the T2-flipped pin).

- [ ] **Step 1: Run the full differential suite against the flipped pin**

Run:
```bash
go test ./test/differential/... -count=1 2>&1 | tail -15
```
The `-count=1` defeats go-test result caching (`reference_differential_break_protocol_count1`) so the run actually boots the contrib container rather than serving a cached PASS from the T1 standard-image run.

Expected: byte-identical PASS — `ok github.com/esalaine/envoy-go/test/differential …` + `ok …/test/differential/fixture …`, process **EXIT=0**, ZERO divergence. All 54 fixture directories pass against the contrib image exactly as they passed against the standard image in T1 (the contrib image is a behavioral superset).

- [ ] **Step 2: Classify any divergence (D-3.7 step 3 — only if Step 1 is NOT a clean PASS)**

If ANY fixture that PASSed in T1 FAILs here, do NOT land the pin. Per SPEC §5.4, classify before proceeding:
- (a) fix envoy-go to match the contrib image's behavior (with the fix's own evidence), OR
- (b) record the divergence as a `BEHAVIOR_CONTRACT.md` extension with its own ADR (next-free after ADR-0227 = **ADR-0228**), OR
- (c) revert the pin.

The SPEC-session re-baseline (§5.2) observed **none** — divergence is not expected (contrib is a superset of the same v1.37.2 source). If Step 1 is a clean PASS, this step is a no-op; record "divergence: none" in the hand-off.

- [ ] **Step 3: Commit (gate-record only — no file change)**

This task changes no files. Record the result (54/54 byte-identical PASS against the contrib image, EXIT=0, divergence: none) in the controller hand-off. Proceed to Task 4.

---

## Task 4: Conformance sanity

Re-run the conformance suites against the flipped pin. They are **image-independent** (SPEC §5.3 — `test/conformance/` never references `StartReferenceProxy`/`ENVOY_TARGET`/the pin), so this is a standing sanity gate, not a swap-sensitive proof; green is expected and load-bearing for the six-gate.

**Files:**
- No file modified.

- [ ] **Step 1: Confirm image-independence (grep evidence)**

Run:
```bash
grep -rln "StartReferenceProxy\|ENVOY_TARGET\|parseEnvoyTarget\|EnvoyPin\|pin.SHA256\|envoyproxy/envoy" test/conformance/ || echo "GREP CLEAN — conformance references no reference image"
```
Expected: `GREP CLEAN — conformance references no reference image`.

- [ ] **Step 2: Run the conformance suites**

Run:
```bash
go test ./test/conformance/... -count=1 2>&1 | tail -10
```
Expected: `ok …/test/conformance/h2spec` (h2spec 53/53) + `ok …/test/conformance/proxy-wasm` (proxy-wasm 10/10), EXIT=0.

- [ ] **Step 3: Commit (gate-record only — no file change)**

No file change. Record "conformance green: h2spec 53/53, proxy-wasm 10/10; image-independence grep clean" in the hand-off. Proceed to Task 5.

---

## Task 5: ADR-0227 body + `BEHAVIOR_CONTRACT.md` pin lines

Land the ADR-0227 §Decision/§Consequences body in place (the §Context already anchored at the SPEC), and move the two current-pin `BEHAVIOR_CONTRACT.md` lines to the contrib digest + ADR-0227. Leave the frozen site (548) untouched.

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0227 §Decision + §Consequences placeholder blocks — search for the `_(Drafted at the phase-30 SPEC …)_` italic placeholders under `### Decision` and `### Consequences` within the ADR-0227 entry)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md:728`, `:804` (the two `### Applies to` `ENVOY_TARGET pin` bullets) + add a phase-30 pin-refresh note

- [ ] **Step 1: Fill the ADR-0227 §Decision body**

In `DECISIONS.md`, locate the ADR-0227 entry's `### Decision` section, which currently holds the placeholder:
```
_(Drafted at the phase-30 SPEC per ADR-0044; the §Decision body lands at the phase-30 IMPL alongside the `ENVOY_TARGET.md` edit — the swap to `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0f…`, the 54-fixture re-baseline gate, the two `BEHAVIOR_CONTRACT.md` current-pin line updates, and the conformance sanity re-run.)_
```
Replace it with the decision body (exact prose is an IMPL micro-decision; it MUST state, load-bearing): the pin is swapped to `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`; the `ENVOY_TARGET.md` `**Tag:**`/`**SHA256:**`/`**Pinned in:**`/`**Last verified:**` lines are updated (T2); the full 54-fixture differential suite is re-baselined byte-identical-PASS against the contrib image as the load-bearing gate (T3); the two `BEHAVIOR_CONTRACT.md` current-pin lines move to the contrib digest + ADR-0227 (this task); the conformance gates re-run green as an image-independent sanity check (T4); the release-notes URL and proto major version are unchanged (same upstream version); ADR-0008 is superseded.

- [ ] **Step 2: Fill the ADR-0227 §Consequences body**

Locate the `### Consequences` placeholder:
```
_(Drafted at the phase-30 SPEC per ADR-0044; the §Consequences body lands at the phase-30 IMPL — the unblocked contrib-filter surface for phase 31 `kafka_broker`; counts unchanged [54 fixtures / 39 fuzzers / 360 stats / BackendKind 30]; ROADMAP row 30 `in-progress → done`, no parent rollup [flat infra row]; next-prompt opens the phase-31 `kafka_broker` BRAINSTORM cold-start.)_
```
Replace it with the consequences body stating, load-bearing: the contrib-filter surface is now reachable to the differential harness — phase 31 `kafka_broker` is unblocked (its v3 proto still requires the `/contrib` go.mod dep, which lands in phase 31 with its first consumer); the latent contrib filters (`sip_proxy`/`mysql_proxy`/`postgres_proxy`/`rocketmq_proxy`/`generic_proxy`) become similarly reachable; counts are UNCHANGED (54 fixtures / 39 fuzzers / 360 stats / BackendKind tail 30 / DECISIONS tail STAYS ADR-0227 — this body lands in-place, no new number); ROADMAP row 30 flips `in-progress → done` with NO parent rollup (flat infra row); the divergence policy (D-3.7 step 3) yielded "none" — the contrib image is a behavioral superset.

- [ ] **Step 3: Update the two current-pin `BEHAVIOR_CONTRACT.md` lines (728, 804)**

Each line currently reads:
```
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).
```
Change BOTH (728 and 804) to:
```
- ENVOY_TARGET pin v1.37.2 (contrib) at `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8` (ADR-0227, superseding ADR-0008).
```

- [ ] **Step 4: Leave line 548 FROZEN — verify it was NOT touched**

Run:
```bash
grep -n 'c5e8a68e' docs/envoy-go/BEHAVIOR_CONTRACT.md
```
Expected: exactly ONE remaining match — line **548** (the phase-06.2 capture-provenance block, frozen per SPEC §7.2). Lines 728/804 no longer match `c5e8a68e` (they now carry `7edd5b0f`). If more than one line still matches, an edit was missed.

- [ ] **Step 5: Add the phase-30 pin-refresh note to `BEHAVIOR_CONTRACT.md`**

Add a short phase-30 note (SPEC §7.3 / ADR-0052) recording the pin-refresh — the variant swap (standard → contrib, same upstream v1.37.2) and the superset-parity outcome (54/54 byte-identical re-baseline). Place it where the contract records per-phase pin/doctrine notes (follow the existing in-file convention for phase notes; exact location + wording an IMPL micro-decision).

- [ ] **Step 6: Verify the docs are well-formed**

Run:
```bash
grep -c 'ADR-0227' docs/envoy-go/DECISIONS.md
grep -n '7edd5b0f' docs/envoy-go/BEHAVIOR_CONTRACT.md
```
Expected: ADR-0227 referenced (count ≥ 1); two `BEHAVIOR_CONTRACT.md` lines (the former 728/804) now carry `7edd5b0f`.

- [ ] **Step 7: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 30 T5: ADR-0227 Decision/Consequences body + BEHAVIOR_CONTRACT contrib pin lines"
```

---

## Task 6: Completion bundle

The atomic landing: the full six-gate GREEN LIVE on the contrib pin, the lifecycle docs advanced, and the next-prompt rewritten to open the phase-31 `kafka_broker` BRAINSTORM cold-start.

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md` (row 30 `in-progress → done`)
- Modify: `docs/envoy-go/STATE.md` (active-phase / lifecycle-state / next-skill / counts → phase 30 done)
- Modify: `next-prompt.txt` (rewrite → phase-31 `kafka_broker` BRAINSTORM cold-start)
- Create: `docs/envoy-go/phases/30-pin-refresh-envoy-contrib/PROGRESS.md`

- [ ] **Step 1: Run the full six-gate LIVE on the contrib pin**

Run each, confirming EXIT=0:
```bash
go build ./... && echo "BUILD ok"
go vet ./... && echo "VET ok"
golangci-lint run && echo "LINT ok"
go test -race -short ./... 2>&1 | tail -20
go test ./test/differential/... -count=1 2>&1 | tail -10
go test ./test/conformance/... -count=1 2>&1 | tail -10
```
Expected: all green — `BUILD ok`/`VET ok`/`LINT ok`; `-race -short` all `ok`; differential `ok …/test/differential` EXIT=0 (54/54 against the contrib pin); conformance h2spec 53/53 + proxy-wasm 10/10 EXIT=0. (This is the authoritative atomic gate per ADR-0052 — the differential is re-run here as part of the bundle even though T3 already ran it.)

- [ ] **Step 2: Flip ROADMAP row 30 to `done`**

In `docs/envoy-go/ROADMAP.md`, change row 30's status from `in-progress` to `done`. NO parent rollup (phase 30 is a flat infrastructure row, not a §9 family sub-row). Verify no other row changes.

- [ ] **Step 3: Advance `STATE.md`**

Update `docs/envoy-go/STATE.md`:
- `active-phase:` → `phase 30 (pin-refresh-envoy-contrib) IMPL done` with the as-built summary (the pin flipped to `contrib-v1.37.2` @ `7edd5b0f…`; 54-fixture re-baseline byte-identical-PASS; ADR-0227 body landed superseding ADR-0008; two BEHAVIOR_CONTRACT lines updated; counts unchanged).
- `lifecycle-state:` → phase 30 CLOSED; per SKILL_ROUTING, next is the phase-31 `kafka_broker` BRAINSTORM (new family row → its own brainstorm, NOT a SPEC).
- `next-skill:` → `superpowers:brainstorming` for the phase-31 `kafka_broker` §9 row.
- `last-commit:` → (filled by the controller at squash; leave a placeholder note that the controller updates).
- `next-free ADR:` → **ADR-0228** (ADR-0227 body landed in-place; no new number consumed at IMPL).
- Project counts block: UNCHANGED (54 fixtures / 39 fuzzers / 360 stats / BackendKind 30 / DECISIONS tail ADR-0227) — but update the reference-pin line to `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0f…` (ADR-0227).

- [ ] **Step 4: Rewrite `next-prompt.txt` for the phase-31 cold-start**

Replace `next-prompt.txt` with a cold-start prompt for the **phase-31 `kafka_broker` §9 Network-filters BRAINSTORM** (the BRAINSTORM §4 forward-pointer; SPEC §12.2 D-S30-3). It must:
- name the landing tip (the phase-30 IMPL squash — controller fills the SHA after squash),
- state that phase 30 (the pin-refresh) is CLOSED and the contrib image is now the differential reference pin (ADR-0227),
- point at phase 31 as the `kafka_broker` family row that adds the `/contrib v1.32.4` go.mod dep with its first consumer,
- route to `superpowers:brainstorming` (a new family row needs its own brainstorm, not a SPEC),
- list the read-first set (the phase-30 SPEC §1.3/§13 for the forward-pointer + counts; the phase-30 BRAINSTORM §2.1 for the three kafka_broker blockers + §4; `ENVOY_TARGET.md` now on the contrib pin; the §9 family roster {redis / thrift remain after kafka_broker}).

- [ ] **Step 5: Write `PROGRESS.md`**

Create `docs/envoy-go/phases/30-pin-refresh-envoy-contrib/PROGRESS.md` — the durable phase-30 IMPL record: the task-by-task outcome (T1 baseline green on standard pin; T2 pin flipped; T3 54/54 byte-identical re-baseline on contrib, divergence none; T4 conformance green; T5 ADR-0227 body + contract lines; T6 six-gate green + lifecycle), the final six-gate evidence, and the counts (unchanged). Follow the existing `PROGRESS.md` shape from a prior phase (e.g. `29.3-network-filter-mongo-fault-delay-and-access-log/PROGRESS.md`).

- [ ] **Step 6: Final verification — counts + pin + gate**

Run:
```bash
echo "fixtures: $(ls -d test/fixtures/[0-9]* | wc -l) (expect 54)"
echo "fuzzers:  $(grep -rh '^func Fuzz' $(find ./internal -name fuzz_test.go) | wc -l) (expect 39)"
grep -E '^\*\*(Tag|SHA256):\*\*' docs/envoy-go/ENVOY_TARGET.md
grep -n 'c5e8a68e' docs/envoy-go/BEHAVIOR_CONTRACT.md
git status --short
```
Expected: 54 / 39; Tag `contrib-v1.37.2` + SHA256 `7edd5b0f…`; exactly one `c5e8a68e` match (line 548, frozen); `git status` shows only the intended doc deltas.

- [ ] **Step 7: Commit**

```bash
git add docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md next-prompt.txt docs/envoy-go/phases/30-pin-refresh-envoy-contrib/PROGRESS.md
git commit -m "phase 30 T6: completion bundle — six-gate green on contrib pin; ROADMAP row 30 done; next-prompt → phase-31 kafka_broker BRAINSTORM"
```

---

## Stage-close (controller, after all tasks green)

1. Verify the branch is clean and the six-gate is green LIVE (re-run T6 Step 1 if any doubt).
2. Squash-merge `phase-30-pin-refresh-envoy-contrib-impl` (or this PLAN branch's IMPL successor) into `master` with a single descriptive commit.
3. Update `STATE.md` `last-commit:` + `next-prompt.txt` tip SHA to the squash commit (the project's SHA-fill follow-up convention).
4. Push `master` to origin (`feedback_push_to_origin`).
5. Clean up the worktree per `superpowers:finishing-a-development-branch`.

## ADR-0045 split-gate — final re-check

6 tasks / **0 production LoC** → far under the `> ~25 tasks OR > ~1500 LoC` gate. **NO split** (SPEC §10.1).
