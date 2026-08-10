# BRAINSTORM 86 — validate-sds-nil-provider

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Date:** 2026-08-10.
**Base master:** `766d98ad2803557b811cfb83b198e57e8c896210` (from `git rev-parse master`), branch `phase-86-brainstorm`.
**Method:** SELF-PICKED per the 2026-07-12 standing directive; no banked mid-lifecycle work existed at this tip (phase 85 CLOSED, row 85 `done`, every chartered row `done` — sentinel check (1) SILENT at `want=117` before this stage). ⚠️ **NAMED DEPARTURE from the three-investigation-agent pattern of BRAINSTORM-84/85: this stage's probes were executed INLINE by the controller** (four `--mode validate` CLI executions against a binary built at this tip, plus code reads and greps — no detached probe worktree was needed because nothing in the repo tree was edited; the binary was built with `-o` into session scratch and every probe config lives in scratch). Every load-bearing claim below is first-hand execution or a direct code read at `766d98ad`; carried claims are labeled as carried.

---

## 1. THE HEADLINE

### 1.1 ⚠️ THE `--mode validate` SDS DIVERGENCE IS THREE-ARMED, NOT ONE-ARMED — ALL THREE ARMS REPRODUCED BY EXECUTION AT THIS TIP

The carried record (BRAINSTORM-83 §5.6, BRAINSTORM-84 §2.2, BRAINSTORM-85 §2.2, STATE.md, ADR-0307-era candidate ledgers) prices "the `validate` nil-`sdsProvider` bug" off ONE reject arm (`internal/tls/config.go:436`). Executed at this tip (binary built at `766d98ad` with `go build -o <scratch>`, configs adapted from fixtures `0103`/`0108`/`0109`'s shapes with template ports substituted from the 47000-47099 band — values only, **validate binds nothing**):

| arm | downstream config shape | `--mode validate` result (MEASURED) | reject site |
|---|---|---|---|
| **A** | `tls_certificate_sds_secret_configs` (fixture `0103`'s shape) | **exit 1** — `tls: downstream: SDS-delivered certificate requires a live SDS provider (unavailable in this mode)` | `internal/tls/config.go:387-389` |
| **B** | `validation_context_sds_secret_config` (fixture `0108`'s shape) | **exit 1** — `tls: downstream: SDS-bound validation_context_sds_secret_config is not supported in phase 03` | `internal/tls/config.go:436-438` |
| **C** | `combined_validation_context` (fixture `0109`'s shape) | **exit 1** — `tls: downstream: combined_validation_context is not supported in phase 03` | `internal/tls/config.go:453-455` |
| **control** | same listener, SDS arm removed (static file-based TLS) | **exit 0** — `configuration OK` | n/a — the reject is NOT vacuous |

All three rejects share the same discriminator: `provider == nil`, and `validate.Bootstrap` (`validate/validate.go:48-49`) threads a **literal nil** `sdsProvider` into `boot.Construct` unconditionally. The ordinary boot path HONORS all three shapes (phases 60.2/65/66): `cmd/envoy-go/main.go:156` builds a live provider via `boot.NewSDSProvider`'s pre-scan, so `provider != nil` whenever an SDS arm is present and none of the three rejects fires — re-read at this tip at the `config.go:407-435` consumer-enumeration comment and `DECISIONS.md:16708` (the ADR-0280 config seam). BRAINSTORM-84 §2.2 additionally EXECUTED the boot-side discriminator (boot builds a live provider and dials; the reject arm is never reached) — carried, re-confirmed here by code read only.

**The divergence is user-facing on the package's OWN charter:** `validate` exists for Kubernetes Gateway API implementations to pre-validate envoy-go bootstrap config (`validate/validate.go:7-10`, phase 51, ADR-0268) — and SDS-bound TLS is a first-class Gateway shape. A config the proxy boots is a config the validator rejects.

### 1.2 ⚠️ THE FIX IS A PARITY CONTRACT, NOT A DELETED IF-STATEMENT — WRONG IN BOTH DIRECTIONS IF DONE NAIVELY

Two findings this stage adds to the carried record:

1. **A pure reject-lift UNDER-rejects.** Boot does not merely tolerate SDS shapes — it VALIDATES them: `boot.NewSDSProvider` enforces the node id/cluster boot requirement (arm 7), the one-SDS-secret MVP cap, and `xds.ParseSDSConfig`'s structural arms before any dial. `validate.Bootstrap` never calls the pre-scan at all, so lifting the three rejects without replicating the pre-scan's validation arms would make validate ACCEPT configs boot REFUSES — the opposite divergence. The row's contract is: **validate accepts iff boot accepts (modulo fetch/dial)**.
2. **The nil-provider guard has OTHER consumers that must keep rejecting.** Enumerated in the `config.go:407-435`/`:447-452` comments and re-verified by call-site grep at this tip: `NewQUICDownstreamConfig` (literal nil provider — QUIC carries no SDS, `manager.go:567`), and the exported test-only constructors `listener.NewManager`/`NewManagerWithBaseDir` (hardcode nil). The discriminator for "validate mode" therefore **cannot be `provider == nil`** — this is `reference_lifted_reject_hidden_enforcement` (land the lift and the guard atomically), known in advance this time.

Also enumerated: the fix must skip **both fetch sites**, not just pass the guards — the blocking `FetchInitialCertificate` at `config.go:390` (arm A) and the `require_client_certificate` fetch-and-install block in `NewDownstreamConfig` (arms B/C). `ParseSDSConfig` already runs BEFORE the arm-A reject (`config.go:383-386`), so structural validation of the SdsSecretConfig itself is free on that arm.

### 1.3 Coverage hole, measured

`validate/validate_test.go` (432 lines) contains **zero** SDS mentions (`grep -c 'sds\|SDS'` = 0); `cmd/envoy-go/main_test.go` (1502 lines) likewise zero. The three reproduced rejects above are the row's ready-made RED anchors.

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

**PICKED — `validate-sds-nil-provider`.** Registered as an **Operational-tooling-family MAINTENANCE row claiming NO family ordinal** (the ADR-0298/ADR-0300/row-85 precedent — a maintenance row repairs a landed deliverable and does not extend a charter). Provenance: BRAINSTORM-83 §5.6 ("a cheap live divergence found in passing"), carried through BRAINSTORM-84 §2.2 and BRAINSTORM-85 §2.2 — **from OUTSIDE the six family windows** (the phase-85 provenance shape), so nothing rolls out of any live window sentence at row-done. ⚠️ **The Operational-tooling window's "an RTDS/SDS validate companion" candidate (`ROADMAP.md:231`) is ADJACENT but DISTINCT and is deliberately untouched:** the companion is a FEATURE (validating dynamically-delivered resources); this row REPAIRS the landed phase-51 static-bootstrap validator's divergence from the boot path it mirrors. A successor SPEC finding the two to be the same thing must say so explicitly and apply the narrowing rule then — this stage's claim is that they are not.

Why it is the smallest **defensible** candidate at this tip:

1. **It is the smallest candidate on the board that changes something real** — re-derived floor §3.3; every cheaper alternative (stat-surface recount) changes no behavior and repairs no defect.
2. **It is a genuine user-facing production divergence, reproduced at this tip with a positive control** (§1.1) — not a doc contradiction, not a hygiene item.
3. **It repairs the landed phase-51 deliverable toward its own chartered purpose** (Gateway pre-apply validation; SDS is a first-class Gateway shape).
4. **Three consecutive BRAINSTORMs carried it as "the strongest sub-row-sized production bug", and BRAINSTORM-83 §5.6 said it "should be swept into the next Operational-tooling row"** — row 85 WAS that family's next row and could not sweep it (the gate repair filled the row); this row is the sweep, chartered standalone because a bug fix with three reject arms, a parity contract, and its own ADR is a row, not a fold-in.

### 2.1 Rejected alternatives, re-derived at this tip

| rejected alternative | re-derived cost at this tip | why rejected |
|---|---|---|
| **CONTINUATION two-sided repair** (server discard `conn.go:255-259` + client blindness `h2/client.go` no-`ContinuationFrame` arm + the client-side SETTINGS gap `h2/client.go:376-407` named beside it in ADR-0307) | est. 2-4x row 85 (row 85 realized **+1046 net `.go`**); needs its own gates on BOTH sides (h2spec MEASURED not to cover either — 6.10 ran 6/6 GREEN over the live discard at the 85 lineage; the client side has no conformance driver at all) | The strongest KNOWN product defect on the board and still the natural next LARGE row — but not the smallest defensible, and its gate does not exist yet. Deferral does not orphan it: recorded in ADR-0307, four phase docs, and the gRPC family window. A future session that picks it should charter a SPLIT phase (86-precedent does not apply; 84's two-leg shape does). |
| **Stat-surface mechanical recount** (the 1205-vs-1207 contradiction, WIDENED and both figures DOC-SOURCED) | ~0 production; one read-only counter + doc reconciliation | Cheapest on the board but changes no behavior and repairs no defect; BRAINSTORM-85 already recorded "its deliverable can ride any future +0 row". This row is NOT +0 (it lands production lines), and packing a second subject into a maintenance row violates the one-subject discipline. Remains available. |
| **`ssl.connection_error`** (Observability window) | floor **+444 net `.go` VERIFIED** (BRAINSTORM-84 §2.2's whole-`.go` measurement; NOT re-measured here) | Largest sub-500 candidate; the phase-75 production-only-vs-whole-`.go` category error is the standing trap on its "small" framing. |
| **`test/conformance/grpc/`** | test-only ~400-1100 lines; **9 of 26** interop cases reachable (carried) | 65% vacuous at birth; a later gRPC-family row's job. |
| **REVIEW.md restoration** (37 of 126 phase dirs) | n/a | Process-not-product; retro-writing fabricates review acts. Standing departure, named per posture. |
| **Hygiene fold-ins** (`harness_test.go:208` port inventory, xDS cycle-guard automation) | ~10 / ~50-100 lines (carried) | Too thin standalone; NOT folded into this row either — a bug-fix row with a parity contract has no spare scope; they remain named fold-in candidates for a future maintenance row. |
| **The six family-window paragraphs as a "candidate"** | n/a | NOT A ROW — settled mechanically at BRAINSTORM-85 §2.1 (prose windows onto ~42 candidates; every retire-shape is the self-clearing-matcher defect, doctrinally foreclosed). The only legitimate move remains chartering ONE real item, which this row does (from outside the windows). |

---

## 3. SCOPE

### 3.1 IN

1. **Accept-and-skip-fetch for the three downstream SDS shapes under validate** (arms A/B/C of §1.1): all structural validation runs (ParseSDSConfig arms; the presence checks at `config.go:457-462`; the E1/E2-adjacent checks), no fetch, no dial.
2. **Boot-parity validation in validate mode** — replicate `boot.NewSDSProvider`'s pre-scan validation WITHOUT its dial: node id/cluster requirement (arm 7), the one-SDS-secret MVP cap, SDS cluster shape checks (exact reusable surface is SPEC Q2).
3. **A validate-mode discriminator that is NOT `provider == nil`** (§1.2 item 2): QUIC's nil-reject and the test-only constructors' nil-reject stay BYTE-IDENTICAL, with guard-preservation tests landing atomically with the lift (`reference_lifted_reject_hidden_enforcement`).
4. **Unit arms per reject-lift + guard-preservation NCs + CLI subprocess arms** in `validate/validate_test.go` and `cmd/envoy-go/main_test.go` (RED anchors = the three §1.1 rejects, already observed at this tip).
5. **Docs:** ONE new ADR (**ADR-0308** from the tail — the SPEC's §Context draft re-arms the strict `PROPOSED` guard 0 -> 1), reconciling the phase-60.2 "validate does not dial/fetch SDS" record (`validate/validate.go:48`, phase 60.2 Task 5) — the no-dial decision SURVIVES; the reject was its over-broad implementation. `BEHAVIOR_CONTRACT.md` deltas ride the ADR per ADR-0052 `:1821` if any contract statement changes (SPEC Q4 sweeps).

### 3.2 OUT — each with its basis

| excluded | basis |
|---|---|
| **Upstream SDS** (`side != "downstream"` halves of all three rejects) | unsupported EVERYWHERE (boot path included) — no divergence exists; it stays a deferred xDS-family window candidate |
| **QUIC SDS** | unsupported by design (phase 61.1); the nil-provider reject there is load-bearing and this row PRESERVES it |
| **The RTDS/SDS validate companion** (Op-tooling window) | a FEATURE, distinct from this REPAIR (§2); window sentence untouched |
| **Any differential fixture** | validate has NO differential surface (phase-51 precedent — no wire behavior to compare); testing is unit + CLI-subprocess |
| **The stat-surface recount, hygiene fold-ins** | §2.1 — one subject per row |

### 3.3 COST POSTURE

⚠️ **The carried ~30-40 prod + 60-120 test (BRAINSTORM-84 §2.2) was priced off ONE arm and is REFUTED as a central estimate — it is now a floor's floor.** Re-derived at this tip from the enumerated surface (three reject arms, two fetch-site skips, the pre-scan-parity reuse, the discriminator threading, guard-preservation tests): **floor ~50-90 net production `.go` + ~150-300 test `.go`**, 3-6 tasks, zero new package edges expected (`go list -deps ./validate` already carries `internal/xds` — carried from BRAINSTORM-84, re-verify at SPEC). ⚠️ Every figure is a LOWER BOUND — `reference_measured_prototype_is_a_lower_bound` has fired EIGHT consecutive times, cause always under-ENUMERATION; the likeliest unenumerated lines here are (a) the pre-scan-reuse refactor in `internal/boot` (NewSDSProvider currently builds the dialing provider inline with its validation), and (b) the combined-arm's fetch path inside `NewDownstreamConfig`'s `require_client_certificate` block. The SPEC's job is to ENUMERATE by prototype, per the phase-84/85 lineage.

---

## 4. OPEN QUESTIONS FOR THE SPEC

1. **Fix mechanism** — enumerate and MEASURE at least: (a) a sentinel validate-only `SecretProvider` recognized by the TLS layer (type-assert or interface method; zero signature changes); (b) a mode flag threaded `Construct -> NewManagerWithBaseDirAndAllowH2C -> buildListenerRuntimeWithCtx -> NewDownstreamConfig -> commonTLSContextToConfig` (wide but explicit); (c) a no-fetch provider returning placeholder material (REJECT-leaning: fabricates state). The chosen shape must leave QUIC/test-constructor nil-rejects byte-identical.
2. **The exact boot-parity surface** — enumerate BY EXECUTION what boot rejects for SDS shapes that validate must also reject: arm-7 node requirement, one-secret cap, unknown/ill-shaped SDS cluster references, `ParseSDSConfig` arms 1-4,8,9. Establish whether cluster-name resolution failures surface at build or at dial (only build-time ones belong to validate).
3. **Reference behavior re-verification** — BRAINSTORM-83 §5.6's "the reference validates OK" is CARRIED, never re-verified: run pinned `contrib-v1.37.2` `--mode validate` on all three arm shapes and the boot-parity negative shapes; record per-side results in the row's docs (pins style: recorded observations, never expectations).
4. **Contract sweep** — enumerate every `BEHAVIOR_CONTRACT.md`/ADR statement about validate-mode behavior (ADR-0268, ADR-0280's validate mentions, phase-60.2 Task 5) and state which the new ADR reconciles vs records.
5. **Test placement** — per-arm unit tests in `validate/` vs `internal/tls/`; which existing tests already pin the QUIC nil-reject (grep at SPEC); CLI subprocess arm shapes in `main_test.go`.
6. **Error-message stability** — arm B/C rejects reuse the phase-03 BYTE-IDENTICAL reject strings (ADR-0080 discipline noted at `config.go:430`, `:447`); the lift must not alter what OTHER consumers see. Confirm the ADR-0080 constraint set.
7. **Anticipated counts** — fixtures +0 (no differential surface) · fuzzers +0 (no new config field; re-check `reference_fuzzer_count_docs_drift`) · stat surface +0 (validate registers no stats — verify) · BackendKind +0 · go.mod +0 (re-run `go mod tidy -diff` and the dep-graph check at SPEC).

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE ESTABLISHED

### 5.1 Load-bearing

1. **The bug is THREE-armed** (§1.1, all three rejects + positive control EXECUTED at `766d98ad`) — the carried one-arm framing (`config.go:436` alone) is REFUTED as the bug's extent; every prior cost figure inherited that under-count.
2. **A pure reject-lift is wrong in BOTH directions** (§1.2) — under-reject via the missing boot-parity pre-scan; over-lift via the QUIC/test-constructor guard consumers. The row's contract is validate-accepts-iff-boot-accepts (modulo fetch).
3. **Both fetch sites enumerated** — `config.go:390` and `NewDownstreamConfig`'s `require_client_certificate` block; skipping the rejects without skipping the fetches would dial from validate (violating the phase-60.2 no-dial decision, which this row PRESERVES).
4. **The coverage hole is measured ZERO** (§1.3) — no SDS test exists on either the package or CLI surface.
5. **The positive control ran** — static-TLS `configuration OK`, exit 0: the rejects are live code, not dead arms.

### 5.2 Carried, NOT re-verified here (SPEC owes them)

- **The reference-side "validates OK"** (BRAINSTORM-83 §5.6) — needs the reference container (SPEC Q3).
- **The boot-side live-dial discriminator run** (BRAINSTORM-84 §2.2) — re-confirmed at this tip by CODE READ only (`config.go:407-435`, `main.go:156`, `DECISIONS.md:16708`).
- **`go list -deps ./validate` carries `internal/xds`** (BRAINSTORM-84 §2.2) — re-verify at SPEC before claiming zero new package edges.

### 5.3 Agent claims

None — no investigation agents this stage (§Method departure, named). The claims-not-surviving ledger is therefore empty by construction, not by success.

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE, BEFORE AND AFTER THE EDIT. IT DOES **NOT** FIRE

Input measured **235 lines / 117 data rows** BEFORE anything was written (whole-file counts, worktree at `766d98ad`).

| check | BEFORE edits (`want=117`) | AFTER edits (`want=118`) |
|---|---|---|
| **(1)** | **SILENT** — every chartered row `done`; the second-ever silent reading, re-observed | **`NOT DONE: row 86`** — the single expected line while this phase is open |
| **(2)** | **SIX** — `:195 :201 :207 :217 :223 :231` | **SIX** — anchors shifted by the row insert to `:196 :202 :208 :218 :224 :232` (re-derived, not predicted) |
| **(3)** | **SILENT** | **SILENT** |

⇒ the condition is a CONJUNCTION; check (2) prints (and after the edit check (1) prints) ⇒ **the sentinel does NOT fire**. `stop` was **NOT** created (`ls stop` => `No such file or directory`, repo root AND worktree, before and after).

**NCs, ALL FIRED, both before and after the edit:** row-62 doctoring => `NOT DONE: row 62` ALONE before / `row 62` AND `row 86` after, with `NC LANDED? [ in-progress ]` inspected first · `want` off-by-one => `GATE FAIL: examined 117 data rows, expected 116` before / `examined 118 data rows, expected 117` after · check-(3) doctoring (residual `gRPC-family row` confirmed **2 -> 0** on the doctored copy first) => `NEVER OPENED: gRPC`, WASM control correctly silent · check-(2) one-arm strips => **5** (long alone) / **1** (short alone), union **6**, never 6 -> 0.

**Leak check, whole-file before/after counts (never a diff grep):** lines **235 -> 236** · data rows **117 -> 118** · check-(2) union **6 -> 6** · `-family row` **95 occurrences / 67 lines -> 95 / 67** (the row's "Operational-tooling-family MAINTENANCE row" phrasing deliberately does not match the check-(3) pattern — rows 76/78/85 precedent) · `gRPC-family row` **2 -> 2** · `Operational-tooling-family row` **3 -> 3** · ARM-A well-formedness flags **{119, 131} ONLY** (row 86 at `:148` is clean; escape-aware pipe-split, field count == 8). **No sentinel matcher string was written into `ROADMAP.md`.**

---

## 7. COUNTS RE-DERIVED AT THIS TIP

- differential fixtures **121** (numeric tail `0119-grpc-unary-trailers`, next-free `0120`) · phase dirs **126 -> 127** (this stage adds `86-validate-sds-nil-provider/`) · fuzzers not re-counted (no fuzz-adjacent claim made here; the row anticipates +0 — SPEC Q7 re-checks)
- `DECISIONS.md` **18050** lines · **306** `^## ADR-` headings · tail **ADR-0307** (COMPLETE) · `^## ADR-0308` = **0** ⇒ next-free **ADR-0308 from the tail** (headings+1 COLLIDES at the ADR-0209 gap) · strict `PROPOSED` guard **0 — DISARMED, correct between phases; the phase-86 SPEC re-arms it** · BYTE-UNTOUCHED this stage
- `BEHAVIOR_CONTRACT.md` **5955** · `STATE.md` **64** · `STATE_HISTORY.md` **474 -> 476** (this close's eviction) · `BOOTSTRAP_PROMPT.md` **522** at the repo root
- `ROADMAP.md` **235 -> 236** lines / **117 -> 118** data rows; row 86 at `:148`
- validate surface: `validate/validate.go` **64** · `validate/validate_test.go` **432** (zero SDS) · `cmd/envoy-go/main_test.go` **1502** (zero SDS) · `internal/tls/config.go` **529**; the three reject sites and both fetch sites as cited in §1

---

## 8. BROKEN-GATE LEDGER

No NEW shape. The divergence is a PRODUCT defect, not a gate defect; the row's own future unit arms take their RED anchors from the three §1.1 rejects (observed at this tip). The nearest standing shape is `reference_lifted_reject_hidden_enforcement` (a lifted reject's hidden enforcement) — engaged PROSPECTIVELY in §1.2/§3.1 rather than after the fact.

---

## 9. HYGIENE

No investigation agents; no docker; no reference container. Probes: one `go build -o <scratch>/repro/envoy-go ./cmd/envoy-go/` at `766d98ad` plus four CLI executions (`--mode validate`) against configs in session scratch — **the repo tree was never edited by any probe** (`git status --porcelain` in the stage worktree shows only this stage's own doc/registration edits; the main checkout shows only the pre-existing untracked `.claude/`). Template port values 47000-47014 appeared in scratch configs only and were NEVER BOUND (validate binds nothing); the next session should band from **47100-47199** if it binds real ports. `go.mod`/`go.sum` untouched. This stage lands **ZERO production `.go`, ZERO test `.go`** — docs only.

---

## 10. NEXT

**SPEC.** It owes: the seven §4 open questions (Q2 and Q3 need EXECUTION — the boot-parity negative shapes and the reference container run); ADR-0308 §Context drafted STATUS `PROPOSED` (re-arming the strict guard 0 -> 1, the live pointer its IMPL disarms); the cost ENUMERATION by prototype (§3.3's floor is a floor — the eighth-firing lineage says the SPEC must enumerate, not estimate); and the fix-mechanism decision (Q1) with the guard-preservation constraint stated as a hard invariant.
