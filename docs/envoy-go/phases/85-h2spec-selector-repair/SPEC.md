# SPEC 85 — h2spec-selector-repair

**Stage:** SPEC (lifecycle-state 1 -> 2). **Date:** 2026-08-09.
**Base master:** `cbaf5010961d3768fea3bc095fa5b11832bc5890` (from `git rev-parse master`), branch `phase-85-spec`.
**Method:** THREE investigation agents on disjoint remits — A1 (EXECUTED probes: the Q2 reference-container run, the Q1 subject flake/timing triple, the JUnit XML capture; own DETACHED worktree at `cbaf5010`, port band 46600-46699, docker containers `a1p85-*`, all torn down BY NAME and the worktree removed and verified); A2 (read-only: Q4 blast-radius enumeration, Q5 threshold-structure design facts, CI facts); A3 (read-only: the Q7 citation sweep). Every load-bearing claim controller-re-derived; agent claims that did NOT survive as stated are in §9.
**Charter:** `BRAINSTORM.md` §3 (scope) and §4 (the seven questions this SPEC disposes). A SPEC is docs-only: ZERO production `.go`, ZERO test `.go`, `ROADMAP.md` BYTE-UNTOUCHED, sentinel `want` stays 117.

---

## 0. SENTINEL — RE-RUN MECHANICALLY AT THIS TIP. IT DOES **NOT** FIRE; `stop` WAS **NOT** CREATED

Input measured **235 lines / 117 data rows** first. At `want=117`:

| check | output at this tip |
|---|---|
| **(1)** | **`NOT DONE: row 85`** — the single EXPECTED line while phase 85 is open at lifecycle-state 1; the denominator assertion printed nothing |
| **(2)** | **SIX** — `:195 :201 :207 :217 :223 :231` (re-derived, not copied) |
| **(3)** | **SILENT** |

The condition is a CONJUNCTION and checks (1) and (2) print ⇒ the sentinel does NOT fire. `ls stop` => `No such file or directory` at the repo root AND the stage worktree root.

**All four NCs fired, observed not predicted:** row-62 doctoring => `NOT DONE: row 62` **AND** `NOT DONE: row 85`, with `NC LANDED? [ in-progress ]` inspected before trusting the result · `want=116` on the real file => `GATE FAIL: examined 117 data rows, expected 116` · check-(3) doctoring (residual `gRPC-family row` confirmed **2 -> 0** on the doctored copy first) => `NEVER OPENED: gRPC` while WASM stayed correctly silent · check-(2) one-arm counts: long arm alone **5**, short arm alone **1**, union **6** (a one-arm strip is NOT an NC for the union).

**Leak-axis baselines carried for the roll-forward (this SPEC edits `ROADMAP.md` NOT AT ALL, so every one must be INVARIANT at close):** lines **235** · data rows **117** · check-(2) union **6** · `-family row` **95 occurrences / 67 LINES** (`--`-guarded) · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.

---

## 1. WHAT THIS SPEC DISPOSES

The BRAINSTORM chartered five deliverables (§3.1: selectors, zero-case guard, JUnit parse repair, the four production fixes, docs+ADR) and left seven questions (§4). This SPEC answers all seven, adds ONE new scope decision the BRAINSTORM did not know about (§4.4: the symmetric CLIENT-side SETTINGS gap, found by A2), drafts ADR-0307 §Context STATUS `PROPOSED`, and prices the row by ENUMERATION (§7). Decision names: **D-85-SEQ** (Q3), **D-85-CI** (Q1), **D-85-REF** (Q2), **D-85-WALK** (Q4), **D-85-GUARD** (Q5), **D-85-66** (Q6), **D-85-SWEEP** (Q7), **D-85-CLIENT** (new).

---

## 2. D-85-SEQ — Q3 DISPOSED: **ONE IMPL LEG, ONE ATOMIC COMMIT. NO SPLIT, NO RED TIP.**

The hazard: selectors + zero-case guard land RED unless the four production fixes land in the same commit (the corrected gate exits 1 at this tip — measured at the BRAINSTORM, re-measured by A1 here).

**Decision: a SINGLE IMPL leg.** All five deliverables land in ONE commit: production fixes (conn.go + flow.go), selector flip, guards, JUnit parse fix, and the doc reconciliation. Rationale, enumerated:

1. **The enumerated cost fits one leg** (§7: ~420-750 net `.go` central band) — far under the ~1500 §6.1 trigger that forced the 84.1/84.2 split. A split would buy nothing: there is no independently-green intermediate product (fixes-without-selector-flip is invisible to the gate; selector-flip-without-fixes is RED).
2. **Within the commit, TDD order still holds** (the PLAN sequences it): unit tests for the four fixes FIRST (they fail on the unfixed tree without any h2spec involvement), production fixes to green them, THEN the selector flip + guards, THEN one full `TestH2Spec` run proving **95/95-modulo-skip green** — the commit's gate evidence.
3. **The row's ROADMAP flip rides the same commit** as its IMPL close per `reference_roadmap_split_phase_row_done` (single-leg row: flip at the one IMPL).
4. Rejected alternative — *two legs, fixes first at 53/53-green then flip*: every intermediate tip IS green, but leg 1 ships production behavior changes whose ONLY gate arrives in leg 2 (`reference_lifted_reject_hidden_enforcement`: land lift + guard atomically). Rejected.

---

## 3. D-85-CI — Q1 DISPOSED: **ENROLL. THE GATE IS MEASURED DETERMINISTIC (3/3 ZERO-VARIANCE) AND COSTS ~9 s WARM AGAINST 23 MINUTES OF HEADROOM.**

CI facts, measured by A2 at this tip:

- Exactly ONE workflow, `.github/workflows/ci.yml`, three jobs: `lint-vet-test` (`go test -short -race ./...` — `TestH2Spec` skips under `-short` at `h2spec_test.go:32-34`, so this job can NEVER reach it), `differential` (`go test ./test/differential/... -timeout 20m -v`, job cap `timeout-minutes: 30`, a `docker version` step proving the daemon, testcontainers + cold reference-image pulls already exercised), `fuzz-smoke` (10-target matrix, includes the h2 package's `FuzzHPACKDecode`/`FuzzFrameStream`).
- **Zero h2spec/conformance references anywhere under `.github/`** (`/usr/bin/grep -rni 'h2spec\|conformance' .github/` exits 1) — the gate has NEVER been in CI.
- Budget headroom: the differential job's last full run is ~6.5 min against a 30-min cap; the h2spec gate adds a `go build ./cmd/envoy-go` (bounded 2 min, `h2spec_test.go:207`), a one-time image pull, and a ~4 s container run.

**The discriminator, MEASURED (A1, three consecutive corrected-selector runs at `cbaf5010`):**

| run | `go test` elapsed | h2spec inner | result |
|---|---|---|---|
| 1 | 17.189 s (cold build) | 6.5837 s | 95/90/1/4, exit 1 |
| 2 | 8.941 s | 6.5868 s | 95/90/1/4, exit 1 |
| 3 | 8.755 s | 6.5818 s | 95/90/1/4, exit 1 |

Failing IDs IDENTICAL all three runs ({6.5.2/1, 6.5.2/3, 6.5.2/4, 6.9.2/1}); inner-time spread **5 ms**. Zero variance — no flake. (Sampled n=3 on an idle host; the BRAINSTORM's 3.77 s was the BROKEN 53-case set — the corrected set's inner time is **6.58 s**, and that is the figure CI budgeting uses.)

**Decision: ENROLL.** The IMPL adds a `go test ./test/conformance/h2spec/ -timeout 5m -v` step to the existing `differential` job (docker already proven there; ~10-15 workflow lines), in the SAME commit as the fixes + selector flip — so CI first sees the gate in its repaired, green state (D-85-SEQ). The gate's own internal timeouts (5 m overall / 3 m container, `h2spec_test.go:101/:145`) sit comfortably inside the job's 30-minute cap alongside the ~6.5-minute differential suite. Posture caveat named: CI adds a cold image pull for `summerwind/h2spec` (~the differential job already pulls the reference image cold — the precedent stands) and the enrollment claim is "deterministic at n=3 locally", not "flake-proof forever"; a CI-side flake, if one ever appears, is a recordable finding, not a license to unenroll silently.

---

## 4. D-85-REF — Q2 DISPOSED: **MEASURED. THE REFERENCE FAILS FOUR SECTION-6 CASES, FULLY DISJOINT FROM THE SUBJECT'S FOUR — SPEC-84's "THREE" IS REFUTED ON THE COUNT, CONFIRMED ON DISJOINTNESS.**

SPEC-84 recorded "a disjoint set of three" section-6 failures on the reference — explicitly UNVERIFIED at the BRAINSTORM (§5.3) and carried as a question, never a fact. A1 ran the pinned `envoyproxy/envoy:contrib-v1.37.2` under the corrected full selector set (strict mode), TWICE, against the SAME bootstrap shape the subject harness uses (`h2spec_test.go:52-93` cloned verbatim; the reference booted it first try, zero config changes — itself a small parity datum). Controller re-derived the failure sets from the saved raw logs (`ref-run.txt`/`ref-run2.txt`), not from the agent's prose.

**Measured: `95 tests, 82 passed, 1 skipped, 12 failed` — BOTH runs, `Finished in 57.09 s` both.** Eleven failures are stable across both runs; the twelfth SLOT FLIPS within section 8 (run 1: 8.1.2.1/3 pseudo-header-as-trailers; run 2: 8.1/1 second-HEADERS-without-END_STREAM — each passed in the other run). **The reference is NOT flake-free under this harness (n=2), while the subject is zero-variance (n=3)** — one more ground for D-85-CI enrolling the SUBJECT gate only.

**Section-6 reference failures — FOUR, stable both runs: {6.3/1 (PRIORITY with 0x0 stream id), 6.7/2 (invalid PING -> GOAWAY), 6.9.1/2 (conn window past 2^31-1), 6.9.1/3 (stream window past 2^31-1)}.** Overlap with the subject's {6.5.2/1, 6.5.2/3, 6.5.2/4, 6.9.2/1}: **EMPTY**. The reference PASSES all five 6.5.2 cases and 6.9.2/1; the subject PASSES all four reference-failing cases (and all seven non-section-6 reference failures: 5.1/8, 5.1/9, 5.1/11, 5.1/12, 5.1.1/2, 5.3.1/2, 7/1, plus the flaky section-8 slot). Many reference failures are Timeout-shaped — which is also why its run takes 57 s against the subject's 6.6 s.

**Standing rule, decided now regardless of A1's numbers:** for a CONFORMANCE gate the project fixes to SPEC, not to reference — RFC 9113 MUSTs bind the subject even where upstream Envoy fails them. Any case the reference ALSO fails is recorded in `CONFORMANCE_PINS.md`'s new section-6 audit rows as a reference-side observation (with the h2spec case ID and the reference's actual behavior), NOT copied into the subject's expectations, and NOT treated as a license to skip the subject fix. The differential harness is unaffected — h2spec is a per-side conformance gate, not a cross-side differential (`test/conformance/`, not `test/differential/`).

**What lands where:** `CONFORMANCE_PINS.md`'s appended corrected-scope record gets a reference-observation paragraph (the four section-6 IDs + the totals + the flaky-slot caveat + the measurement date and method), and ADR-0307 §Context records the full 12-case set. Nothing reference-side enters the subject's expectations (the standing rule above). **Method caveat, named:** A1 measured the reference container-to-container over the docker bridge IP (a `127.0.0.1`-bound host publish is unreachable from the bridge) rather than the subject harness's host-gateway path; both are container-to-target TCP, but the network path differs from the harness's and the record says so.

---

## 5. D-85-WALK — Q4 DISPOSED: **UNIT + h2spec SUFFICE. NO DIFFERENTIAL FIXTURE. THE COVERAGE HOLE IS MEASURED AT ZERO.**

A2's enumeration (all cites re-verified at `cbaf5010`):

- **The suspicion is CONFIRMED at exactly zero:** no test anywhere exercises a mid-connection SETTINGS_INITIAL_WINDOW_SIZE change. All 28 `InitialWindowSize` hits classify as production code, framer round-trips, or HANDSHAKE-time initial SETTINGS (conn_test.go:892/:1119, IWS ∈ {1, 16}); every `WriteSettings` in every test file is the client's initial handshake frame. Zero differential fixtures set the field at all.
- **The walk's full surface:** per-stream send window `serverStream.sendW *window` (`stream.go:73`; `window` at `flow.go:12-16`, own `w.mu`), seeded in `newServerStream` from `onHeaders`'s `peerInitWindow` (`conn.go:372-376`); DATA debits via `ss.sendW.reserveBlocking` (`conn.go:735`); the iterable registry `ServerConn.streams map[uint32]*serverStream` (`conn.go:50`) under `s.mu` (`conn.go:49`). Lock order `s.mu -> w.mu` matches existing code — no inversion.
- **What the compliant walk does** (RFC 9113 §6.9.2): in `onSettings`'s `SettingInitialWindowSize` case (`conn.go:519-520`), capture `old` BEFORE overwriting; `delta := int32(new) - int32(old)`; under `s.mu`, apply `delta` to every live stream's `sendW`. Negative deltas are legal and already handled — `reserveBlocking`'s `for w.n <= 0` loop (`flow.go:50`) blocks correctly on a driven-negative window. A SETTINGS-caused adjustment pushing any stream window past 2^31-1 is a **connection error** FLOW_CONTROL_ERROR (unlike WINDOW_UPDATE's stream error); `safeAddInt32` (`conn.go:25-31`) is reusable and has NO existing SETTINGS-path use. The conn-level `s.sendW` is NOT touched (§6.9.2: connection window changes only via WINDOW_UPDATE).
- **A sharper method than copying the existing pattern:** the existing overflow checks (`conn.go:567-568`, `stream.go:223-224`) are check-then-act across two `w.mu` acquisitions — a tolerated TOCTOU. The PLAN should add a single-critical-section `window.adjust(delta int32) (ok bool)` to `flow.go` rather than reproduce it (~10-15 lines).
- **No existing test breaks:** every SETTINGS value any test sends is valid (HeaderTableSize=64, IWS∈{1,16,65535}, MaxFrameSize=16384, or empty). `FuzzFrameStream` asserts only no-panic + `h2:` error prefix; `connError(...)` returns are compatible.

**Decision: unit tests + the repaired h2spec gate are the row's coverage; NO differential fixture** (+0 fixtures, as chartered). Grounds: (a) h2spec 6.9.2/1 is a deterministic external oracle for exactly this behavior; (b) a differential fixture needs a raw-framer driver to inject mid-stream SETTINGS (the 0119 first-of-kind precedent, 669 lines) — a ~10x cost for a second oracle on a behavior already double-gated; (c) the zero-coverage finding is recorded in ADR-0307 rather than patched with fixture weight this row does not need. The unit arms MUST include: delta-increase, delta-decrease (driven-negative then unblocked by WINDOW_UPDATE), the 2^31-1 overflow -> GOAWAY(FLOW_CONTROL_ERROR), and new-stream-after-change seeding.

**The `<= 0 -> 65535` seeding quirk** (`conn.go:373-375`, mirrored client-side at `client.go:192-195`): the code cannot distinguish "peer never announced IWS" from "peer explicitly announced IWS=0" — both read as uint32 zero and get the 65535 default. An explicit IWS=0 announcement MUST leave new streams at 0. The PLAN probes whether any corrected-set h2spec case discriminates this (6.9.2/* set the window low or zero); the fix rides the same `onSettings`/seeding edit if RED, and is recorded in ADR-0307 as a named latent quirk if no case reaches it.

---

## 6. D-85-GUARD + D-85-66 — Q5 AND Q6 DISPOSED: **THREE-LAYER GUARD; THE 6.6 COMMENT IS REWRITTEN TO STATE THE MEASURED VACUITY**

A2's failure-mode decomposition — three distinct classes, each needing its own layer:

| failure mode | example | closed by |
|---|---|---|
| declared selector matches NOTHING | the live phase-85 defect: slash-form `http2/6/9` is a silent no-op | **layer 1** — per-selector ≥1-case guard |
| a suite silently drops out | image bump loses `5.1.2` while `http2/5` still matches 5.1, 5.3.1… | **layer 2** — named-suite enumeration (layer 1 passes: the selector still matched SOMETHING) |
| a suite's case count shrinks | `5.1` present but `tests=` 13 -> 2 | **layer 3** — per-suite minimum case counts |

**Decision: implement ALL THREE**, hooked BEFORE the `if s.Tests == 0 { continue }` skip at `h2spec_test.go:310-312` discards the evidence (the skip itself is deleted — it is the shape-32 blindness):

1. **Layer 1:** every entry of `thresholdSections` must have matched ≥1 case: map each declared selector to its dotted prefix and require at least one suite whose name-prefix matches with summed `Tests > 0`. The mapping is per-selector-PREFIX, not per-suite-equality — one selector (`http2/5`) fans out to many suites (5.1, 5.1.1, 5.1.2, 5.3.1, 5.4.1, 5.5).
2. **Layers 2+3:** a pinned roster in `h2spec.go` (the typed-mirror pattern the pin digest already uses; `CONFORMANCE_PINS.md` stays the authoritative human-readable table per the file's own header) mapping suite -> minimum case count, derived from the corrected-run table. ⚠️ **Keyed on the `package` attribute (`package="http2/6.5.2"`), NOT on `id`, NOT on a name-prefix parse — DECIDED FROM THE CAPTURED XML:** the report also carries `hpack/*` and `generic/*` suites, and the `id` values COLLIDE across families (`id="6.1"` appears on BOTH `<testsuite name="6.1. DATA" package="http2/6.1">` AND `<testsuite name="6.1. Indexed Header Field Representation" package="hpack/6.1">` — controller-verified in `h2spec.xml`). An `id`- or dotted-prefix-keyed guard double-matches; the `package` attr is unambiguous and machine-stable. The guard therefore (i) adds `Package`/`ID` attrs to `junitTestSuite`, (ii) filters to `package` prefix `http2/` before any counting (the non-http2 suites are zero-test noise in this run but are NOT guaranteed to stay so across a pin refresh).
3. The roster changes ONLY via the pin-refresh procedure (`CONFORMANCE_PINS.md:7-18`) — a digest-pinned image cannot drift counts on its own.

**D-85-66 — Q6:** the `http2/6/6` exclusion comment (`h2spec.go:30`) and the doc-comment's exclusion rationale (`h2spec.go:17-20`) are REWRITTEN, not deleted: the measured fact is that the pinned image ships NO 6.6 suite at all (`--dryrun -S http2/6` lists none; the 8.2 probe covers client-sent PUSH_PROMISE and runs green), so the exclusion excludes nothing. The comment must say exactly that — "6.6 absent from the pinned image (measured 2026-08-08); exclusion retained as documentation of ADR-0051's intent; see ADR-0307" — because DELETING it invites a future reader to add `http2/6.6`, which would select zero cases and trip layer 1. `CONFORMANCE_PINS.md:59`'s exclusion paragraph gets the same correction (it is a RECONCILE target, §8). ADR-0051 itself stays recorded-not-fixed; ADR-0307 records the vacuity.

---

## 7. THE JUNIT PARSE REPAIR — ROOT CAUSE MEASURED FROM THE CAPTURED XML: **h2spec EMITS `<error>`, NEVER `<failure>` — AND `<testcase>` HAS NO `name` ATTRIBUTE AT ALL**

The measured defect (BRAINSTORM §1.3.1): in the corrected failing run, `tc.Failure` parsed nil for ALL FOUR failing cases — `assertThreshold`'s per-case `FAILED:` lines never printed. Root cause, controller-verified against the saved `h2spec.xml` (19898 bytes, captured by A1 from a live failing run):

- Whole-file counts: `<failure` = **0**, `<error>` = **4**. h2spec writes every failing case as an `<error>` child; the suite attrs correspondingly read `errors="3" failures="0"` (which is why the SUITE-level gate still counted correctly — `s.Failures + s.Errors` at `h2spec_test.go:316` — while per-case identity was blind: only the reporting layer was broken, a compensating-structure detail worth naming).
- A failing case, verbatim: `<testcase package="http2/6.5.2" classname="SETTINGS_ENABLE_PUSH (0x2): Sends the value other than 0 or 1" time="2.0007"><error>GOAWAY Frame (Error Code: PROTOCOL_ERROR)\nConnection closed\nSETTINGS Frame (length:0, flags:0x01, stream_id:0)</error></testcase>`.
- **Second latent bug found by the same capture:** `<testcase>` carries only `package`/`classname`/`time` — NO `name` attr — so `junitTestCase.Name` parses as `""` and even a fixed failure-parse would print `FAILED: : <msg>` with an empty case name. The `<error>` element has no `message` attr (content is chardata: expected frame(s), then actual), so the existing Message-then-Text fallback works once the element is seen. Skips appear as a `<skipped></skipped>` child + `skipped="1"` suite attr.

**The fix (exact, for the PLAN):** extend `junitTestCase` with `ClassName string \`xml:"classname,attr"\`` and `Error *junitFailure \`xml:"error"\``; treat a case as failed when `Failure != nil || Error != nil` (coalesce); print `tc.ClassName` (fall back to `tc.Name` for generic-JUnit tolerance). ~10-20 lines including the reporting change.

---

## 8. D-85-SWEEP — Q7 DISPOSED: **THE FULL CENSUS, WITH THE RECONCILE SET SMALL AND CLOSED**

A3's sweep, controller-spot-verified. The reconcile set (edited at IMPL) is FIVE files; everything else is RECORD (ADR-0307 states the blanket reinterpretation: every historical "h2spec 53/53 green" was real output of a gate running 44% short of declared scope).

| target | hits | disposition |
|---|---|---|
| `test/conformance/h2spec/h2spec.go` | selector strings :25-34, doc comment :17-20, 6.6 comment :30 | **RECONCILE — the fix itself** |
| `docs/envoy-go/CONFORMANCE_PINS.md` | 18-suite table :37-57 (zero 6.x rows, Total 53), exclusion ¶ :59, first-run record :61-65 | **RECONCILE, append-style per the file's own audit discipline**: the 2026-04-25 record STAYS (it is true history — the broken selector set genuinely produced 53/53); a NEW corrected-scope table (24 suites incl. six 6.x rows, Total 95) and a NEW run record are APPENDED with the correction dated and cross-referenced to ADR-0307 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | normative threshold `:2052-2054`; ADR-0055 prose `:2056` ("section 5/6 coverage" — half-false); five historical 53/53 evidence ¶s (`:1464 :1669 :4263 :5002 :5009`) | **RECONCILE `:2054` + `:2056` ONLY, riding ADR-0307** — ADR-0052 `:1821` makes an ADR the MANDATED vehicle, and `:1818-1819` say a threshold RELAXATION needs supersession: this is a WIDENING (53 -> 95 cases, failed==0 unchanged), so a new ADR (not a superseding one) satisfies both sentences. The five historical evidence paragraphs are RECORDED, not rewritten |
| `docs/envoy-go/STATE.md` | `:38` — `conformance: h2spec 53/53` — the living false line | **RECONCILE at IMPL close** (flips with the normal §Current roll) |
| `.github/workflows/ci.yml` | zero h2spec refs today | **RECONCILE — D-85-CI ENROLLS** (§3): the h2spec step joins the `differential` job at IMPL |
| `docs/envoy-go/DECISIONS.md` | ADR-0051 §2 `:1747-1750` (the false scope sentence, verbatim in ADR-0307), `:1732-1737` + `:1764-1766` (moot PUSH_PROMISE rationale), ADR-0052 `:1803`; 24 more historical 53/53 cites | **RECORD via ADR-0307** (append-only) |
| `docs/envoy-go/ROADMAP.md` | 33x `53/53` on 31 historical row lines | **RECORD** — retro-editing 31 rows is exactly the leak-audit hazard; the ONLY sanctioned IMPL edit is row 85's `in-progress -> done` flip |
| `docs/envoy-go/phases/**` | 527 hits in 226 files | **RECORD** (never retro-edited) |
| `docs/envoy-go/STATE_HISTORY.md` | 6 hits | **RECORD** (append-only) |
| `docs/TEST_GAP_ANALYSIS.md:28` | "h2spec (53/53 **sections**)" — doubly false | **RECORD** — controller call: the doc is a dated point-in-time snapshot (its own fixture/fuzzer counts are stale at 101/46 vs today's 121/55); ADR-0307 names it |
| root `PROGRESS.md` `:106 :143` | stray phase-32.1 stage doc at the repo root | **RECORD**; the stray location is flagged as a documentary defect, not this row's job |
| `BOOTSTRAP_PROMPT.md`, `REVIEW_FINDINGS.md:190`, conn.go case-cites (:65 :105 :137 :154 :290 :333), `docs/superpowers/*` | scope-neutral or truthful (cited sections genuinely ran) | **untouched** |

---

## 9. D-85-CLIENT — NEW SCOPE DECISION: **THE SYMMETRIC CLIENT-SIDE GAP IS OUT, RECORDED, AND NAMED IN ADR-0307**

A2 found what the BRAINSTORM did not know: `ClientConn.dispatchFrame`'s SETTINGS case (`client.go:376-407`) applies peer SETTINGS with ZERO validation (`:388-402`) and performs NO streams walk on an `InitialWindowSize` change — the exact server-side defect pair, mirrored. The iteration surface exists (`cc.streams sync.Map`, `.Range` used for GOAWAY fan-out at `:628-638`); the same `<= 0 -> 65535` seeding quirk exists at `:192-195`.

**Decision: OUT of this row.** Grounds, in order of weight: (1) **h2spec only exercises the server side — the repaired gate CANNOT cover a client-side fix**, and landing an ungated production change is the `reference_one_sided_gate_for_a_two_sided_fix` shape this project keeps refusing; (2) the charter confines production edits to the server path (`BRAINSTORM.md` §2 item 4); (3) the client H2 path already carries a deferred robustness row (the CONTINUATION-blind response loop, recorded in FOUR documents) — the SETTINGS gap joins it so the future client-robustness row prices BOTH, with its own gate. ADR-0307 §Context names it so it cannot silently vanish.

---

## 10. COST — ENUMERATED, NOT SCALED (every figure a LOWER BOUND; the prior has fired on SIX consecutive rows)

Against the BRAINSTORM §3.3 floor (9 chars + ~15-25 harness + ~40-80 production + ~100-200 unit-test + ~25 docs + one ADR), this SPEC's enumeration RAISES three buckets (order-of-work is in §2's structural sequence; the PLAN owes the task decomposition):

| bucket | enumerated content | lines (lower bound) |
|---|---|---|
| selectors | 9 one-char edits (`h2spec.go:25-34`) + doc-comment + 6.6-comment rewrite (§6) | ~10-15 |
| guards (3 layers) | delete the `:310` skip; per-selector ≥1-case check; suite roster (24 entries) + prefix-match + min-count assertions | ~70-120 |
| JUnit parse fix | `Error` field + `ClassName` + coalesce + reporting (§7 — root cause measured, blast radius closed) | ~10-20 |
| production: SETTINGS validation | shared validator called from BOTH `onSettings` (`conn.go:508-538`) and `readClientSettings` (`settings.go:79-108` — the handshake path has the SAME zero validation, A2 Task-1c caveat 1): ENABLE_PUSH ∈ {0,1}, MAX_FRAME_SIZE ∈ [16384, 2^24-1] -> GOAWAY(PROTOCOL_ERROR); the IWS > 2^31-1 arm (-> FLOW_CONTROL_ERROR) INCLUDED IFF the PLAN's probe shows 6.5.2/2's current PASS is accidental (`reference_compensating_defects_cancel_in_the_gate_metric` — the mechanism must be identified either way) | ~40-70 |
| production: §6.9.2 walk | capture-old + delta + `window.adjust` (new, single-critical-section, `flow.go`) + streams walk under `s.mu` + overflow -> GOAWAY(FLOW_CONTROL_ERROR) | ~35-55 |
| unit tests | per-arm: ENABLE_PUSH invalid; MFS low/high/boundary-valid; handshake-path variants; delta increase / decrease-negative-then-unblock / overflow / new-stream seeding; guard NCs (a doctored selector must FAIL the harness — `reference_gate_command_negative_control`) | ~250-450 |
| docs | CONFORMANCE_PINS append incl. the reference-observation ¶ (~40-60); BEHAVIOR_CONTRACT `:2054`+`:2056` (~5-10 net); `.github/workflows/ci.yml` enrollment step (~10-15); STATE/ROADMAP flips; router roll | ~55-90 |
| ADR-0307 | §Context at this SPEC (drafted, STATUS `PROPOSED`); §Decision+§Consequences at IMPL | ~60-100 total |

**Net `.go` central band ~420-750** — one leg (§2), no fixture, no BackendKind, no module, no port, stat surface anticipated **+0** (no `NewCounter`/`NewGauge` site in any enumerated edit; asserted at IMPL by call-site enumeration 208/36, never `TestNoNewStat*`). Likeliest unpriced lines, named: a 6.9.2-adjacent case newly RED once the walk lands (h2spec cases interact — the walk changes what 6.9.2/2-skipped and 6.9.1/* observe); any `readClientSettings` reject path needing its own error-plumbing shape (the handshake path pre-dates the `connError` machinery it must now call); the IWS=0 seeding-quirk fix if the PLAN's probe turns it RED (§5).

---

## 11. GATE POSTURE (the six-gate standing statement — departures NAMED, not complied around)

**(a)/(b)** differential: this row anticipates **+0 fixtures** (121 stays 121) and does not touch `test/differential/` — owed as posture at this SPEC (zero `.go` changed ⇒ not-exercised, not green); the IMPL runs the full suite with `INNER_EXIT` asserted. **(c)** conformance: h2spec carries the standing caveat — "NO ROW MAY CITE h2spec 53/53 AS FRAME-LEVEL EVIDENCE" — until THIS row's IMPL lands; this SPEC does not pre-cite its own future repair (the exact defect it fixes). `test/conformance/grpc/` stays deferred in writing (SPEC-84 §4); proxy-wasm 10 of 16 families. **(d)** fuzz: `FuzzFrameStream` already traverses `onSettings` and will explore the new reject paths; no new fuzzer owed (no new parse surface — SETTINGS values are already-parsed uint32s); fuzzers **55** in **48** files. **(e)** `INNER_EXIT` owed at IMPL on every differential launch and `go test ./...`; this SPEC ran neither (docs-only; A1's probe ran `TestH2Spec` in a detached worktree and its exits are reported honestly in §3-§4). **(f)** STANDING DEPARTURE, named: no `REVIEW.md`; 37 of 126 phase dirs carry one, none since 25.3.

---

## 12. ADR-0307 — DRAFTED AT THIS SPEC, STATUS `PROPOSED`

Next-free derived FROM THE TAIL: `DECISIONS.md` **17990** lines, **305** `^## ADR-` headings, tail `## ADR-0306` (COMPLETE, at `:17928`), `^## ADR-0307` = **0** (headings+1 COLLIDES at the ADR-0209 gap — never use it). The SPEC appends a new `## ADR-0307` heading + `### Context` block + STATUS blockquote `PROPOSED`; §Decision/§Consequences are appended IN PLACE at the IMPL (the ADR-0306 precedent). The strict guard `^> \*\*STATUS: PROPOSED` arms **0 -> 1** at this commit — a LIVE POINTER no later stage may "fix"; the STATUS census (`^> \*\*STATUS: ` blockquote matcher) goes **19 -> 20**; the tail moves to ADR-0307 and next-free becomes **ADR-0308 FROM THE NEW TAIL**. §Context content: the selector defect + both-ways measurement; the four MUST violations with code-site causes; the JUnit parse gap; the vacuous 6.6 exclusion; ADR-0051 §2's false sentence QUOTED (`:1747-1750`) and ADR-0052 `:1803`'s repeat; the D-85-CLIENT symmetric gap; the D-85-REF reference measurement; the Q7 census summary (§8's table, compressed); the D-85-SEQ single-leg rationale.

---

## 13. REFUTATION LEDGER — WHAT THIS STAGE ESTABLISHED AGAINST ITS PREDECESSORS, AND AGENT CLAIMS THAT DID NOT SURVIVE

### 13.1 Predecessor claims refuted or corrected BY EXECUTION

1. **SPEC-84's "the reference fails a disjoint set of THREE section-6 cases" — REFUTED ON THE COUNT: FOUR** ({6.3/1, 6.7/2, 6.9.1/2, 6.9.1/3}, stable n=2, controller-re-derived from raw logs) — **CONFIRMED on disjointness** (empty overlap with the subject's four). The BRAINSTORM was right to carry it as a question, not a fact.
2. **The BRAINSTORM's "3.77 s" container figure is the BROKEN gate's runtime** — true as stated for the 53-case set, but NOT the CI-budget figure: the corrected 95-case set runs **6.58 s** inner (n=3, 5 ms spread). Both figures are now recorded with their selector sets; carrying "3.77 s" forward into a CI decision would have understated the cost 1.7x.
3. **The zero-case skip's blast radius is WIDER than the BRAINSTORM stated:** the captured XML shows the report also carries `generic/*` and `hpack/*` suites and that `id` values collide across families (§6) — so the naive "assert every suite has cases" repair-shape would have been WRONG in a new way (double-matching / noise-tripping). The BRAINSTORM's repair sketch survives only with the `package`-key refinement.

### 13.2 Agent claims that did NOT survive as stated

1. **A3: "ENVOY_TARGET.md DOES NOT EXIST"** — false as stated; the file is `docs/envoy-go/ENVOY_TARGET.md` (controller-verified by `find`); every in-repo pointer uses the docs-relative path. The agent looked only at the repo root. No documentary defect exists.
2. **A1: "14 zero-test `generic/*` suites"** — the controller's line-count of the same XML reads **13** `package="generic` testsuites. CONTESTED, NO NUMBER CARRIED (`reference_a_drift_correction_is_itself_a_claim`); the load-bearing fact — non-`http2/` suites exist and the guard must filter on `package` prefix — is verified either way and does not depend on the count.
3. **A2's suite-name-format inference** ("the dotted prefix is the safer key") — superseded by the captured XML: the prefix COLLIDES with `hpack/*` ids; the `package` attr is the key (§6). The inference was flagged as unmeasured by A2 itself; recorded here because the SPEC initially adopted it.

---

## 14. NEXT

**PLAN.** It owes: the task decomposition of §2's single leg in TDD order; the §10 enumeration REFUTED BY EXECUTION (build the validator + walk against the unit arms, then the corrected gate); the 6.5.2/2 accidental-pass probe (§10); the IWS=0 seeding-quirk probe (§5); the guard NCs (a doctored selector MUST redden the harness); and the D-85-CI enrollment mechanics if enrolled.
