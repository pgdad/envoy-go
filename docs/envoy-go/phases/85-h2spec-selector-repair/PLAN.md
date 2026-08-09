# PLAN 85 — h2spec-selector-repair: **the whole change set was BUILT AND RUN — the repaired gate reads `95/94/1/0` five times — and the measurement moved FOUR SPEC claims**: the 6.5.2/2 "accidental pass" is GENUINE delegation to x/net's parse-time guard (the conditional validator arm dissolves, and a REAL wrong-code handshake defect surfaces in its place); the "latent" IWS=0 quirk is REAL but **unit-only-discriminable** (the two probe agents CONFLICTED and P1's execution won); the SPEC's layer-2 guard is **DELETABLE ONE ROSTER ENTRY AT A TIME** without a reverse check; and the roster is **31 suites, not 24**, with the measured cost **net +757** landing just ABOVE the §10 band top — the seventh consecutive `reference_measured_prototype_is_a_lower_bound` firing

**Stage:** PLAN (lifecycle-state `2` -> `3`). **ROW 85 STAYS `in-progress`**; `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` are **BYTE-UNTOUCHED** and the sentinel `want` **STAYS 117**. Base master **`be018027`**, taken from `git rev-parse master` at session start and **not** from any SHA quoted in the router. Branch `phase-85-plan`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

**ADR-0307 is already drafted, STATUS `PROPOSED`, §Context-only. A PLAN adds no ADR and does not touch `DECISIONS.md`.** The strict guard `^> \*\*STATUS: PROPOSED` is **1** at this tip and **stays 1** through this stage; the IMPL disarms it (§Decision + §Consequences appended in place after the retained italic footer, no renumber, no `---`).

---

## What was EXECUTED at this stage

**TWO probe agents**, each in its own **DETACHED** worktree off `be018027`, private scratch, private port bands (P1: 46700-46799, P2: 46800-46899), any manually-launched container named `p2p85-*` and removed BY NAME (P1 launched none manually; testcontainers self-cleaned). **P1 built and ran a working prototype of the row's ENTIRE change set** — unit arms proven RED first on the unfixed tree, production fixes, harness repair, the corrected gate at `95/94/1/0` five times, and the three guard NCs fired (one only after ITS OWN design was repaired, §1.2) — then destroyed it with byte-exact sha256 proof, TWICE (it re-applied the identical set to fold in P2's mid-flight findings, re-measured, re-destroyed). **P2 answered the SPEC's two open measurement questions** (the 6.5.2/2 accidental-pass mechanism; IWS=0 discrimination) by frame-level execution with a standalone raw-framer probe, h2spec `--verbose` capture, and an instrumented-subject IWS census. Nothing was committed by either agent; nothing was pushed; both worktrees removed and verified gone; the canonical repo's working tree untouched.

⚠️ **The two agents CONFLICTED on one load-bearing claim** (whether the gate can reach 95-green without the quirk fix) and the conflict was resolved BY EXECUTION, not adjudicated by prose — §1.3/§2.2. One P2 inference and two SPEC enumerations did not survive; the ledger records each.

Every load-bearing claim below was **re-derived by the controller** from the agents' quoted command output. Where an agent's or the SPEC's claim did not survive, it is recorded as such (§1.1-§1.6; the P2-vs-P1 conflict resolution in §1.3/§2.2).

---

## 1. PLAN re-derivation ledger — what this stage REFUTED or MEASURED

**NINE findings, SIX load-bearing (⚠️).**

### 1.1 ⚠️ SPEC §10's conditional IWS arm rests on a WRONG premise — the 6.5.2/2 pass is GENUINE, not accidental

The SPEC conditioned the IWS>2^31-1 validator arm on "the probe shows 6.5.2/2's current PASS is accidental". The probe (P2, frame-level, independently corroborated by P1's arm 5) shows it is NOT: x/net v0.34.0's `parseSettingsFrame` (frame.go:746-753) rejects the value at parse time and `translateFramerErr` already routes `ConnectionError(ErrCodeFlowControl)` to GOAWAY(FLOW_CONTROL_ERROR). Full mechanism + dispositions in §2.1. The SPEC's *"the same x/net check is also why 6.9.2/3 passes"* corollary is P2-measured, and the SPEC's brief-stage hypothesis (the int32/clamp path at `conn.go:372-375`) is REFUTED — `clientS.InitialWindowSize` is never assigned the oversized value.

### 1.2 ⚠️ SECOND — the SPEC's layer-2 guard is DELETABLE ONE ENTRY AT A TIME: NC-2 as SPEC'd did NOT fire

D-85-GUARD's layer 2 ("named-suite roster") iterates ROSTER keys against the report — so deleting a roster entry deletes the check that would notice (`the gate PASSED, rc=0, 95/94/1/0` with `http2/6.7` removed — P1, measured). The repair is a REVERSE check (~9 lines): every `http2/*` suite with `Tests > 0` must be rostered. With it, NC-2 fires: `guard layer 2: suite "http2/6.7" ran 4 case(s) but has no expectedSuites roster entry`. **`reference_replacement_gate_inherits_defect` in its purest form — the NEW guard's own failure modes had to be probed, and one was found broken before it ever shipped.** Task 3's guard code includes the reverse direction.

### 1.3 ⚠️ THIRD — the two probe agents CONFLICTED on the quirk fix's gate-visibility, and execution resolved it AGAINST the co-requisite claim

P2: "the gate cannot reach 95-green without the announced-flag fix." P1: **95/94/1/0 three consecutive times WITHOUT it** — a consistent clamp in seeding + effective-old compensates exactly, and `pendingDispatch` (`conn.go:60-66`) closes the timing hole. Full resolution in §2.2. The carried conclusions: the quirk fix is IN (RFC MUST) but **unit-gated, gate-invariant**; seeding and effective-old MUST share one computation; and ⚠️ **a probe agent's inference about a cell it never ran is a claim, not a measurement** (`reference_a_drift_correction_is_itself_a_claim` — this stage's instance is P2 extrapolating walk-without-quirk from baseline behavior).

### 1.4 ⚠️ FOURTH — the roster is 31 `http2/*` suites, NOT the SPEC's 24

The captured XML carries **49** suites total: **31** `http2/*` + 13 `generic/*` + 5 `hpack/*` (the non-http2 ones all zero-test under our selectors; the SPEC's contested 13-vs-14 `generic` count stays contested and stays load-irrelevant). The 24-suite figure in D-85-GUARD/§10 under-counted by seven; the measured roster (§Task 3) sums to exactly **95**. The `id` collision is re-confirmed (`id="6.1"` on both `http2/6.1` and `hpack/6.1`) — the `package` key survives.

### 1.5 ⚠️ FIFTH — the §10 cost enumeration is REFUTED UPWARD at its top: measured net **+757** against the ~420-750 central band

Per-bucket measurement in §7. The overage is exactly the SPEC's unenumerated items: the reverse-layer-2 guard (§1.2), the quirk unit arm + announced flag (§1.3/§2.2), the `readClientSettings` plumbing (named by the SPEC as "likeliest unpriced" — it materialized at 4+9 lines), and regression-pin comments. **The SEVENTH consecutive `reference_measured_prototype_is_a_lower_bound` firing, and the cause is again under-ENUMERATION, not under-scaling** — and this one fired on a band the SPEC had already called "every figure a LOWER BOUND".

### 1.6 ⚠️ SIXTH — TWO "expected RED" arms arrived GREEN, and both reclassifications matter

Arm 5 (mid-connection IWS>2^31-1) — green via §1.1's parse-time guard. Arm 7d (new-stream seeding after a mid-connection IWS change) — green because `onSettings` already stores the value and seeding already reads it; the defect was never "new streams", only "live streams" (and the explicit-0 case, §2.2). Both stay in the suite as REGRESSION PINS, labeled as such; **neither may be counted as a RED anchor** (`reference_liveness_break_needs_failing_baseline`: green can mean "already correct", and a task list that claims them as TDD reds would be lying about its own ordering).

### 1.7 SEVENTH — the JUnit shape claims all re-confirmed at the tip

`<testcase>` carries only `package`/`classname`/`time` (no `name`); a fully-green file contains ZERO `<failure>` elements; the invariant skip is 6.9.2/2 ("window size to be negative"). The SPEC's §7 fix shape (add `ClassName` + `Error`, coalesce, print ClassName) survives measurement unchanged.

### 1.8 EIGHTH — NC-1's discriminating output doubles as the historical-defect demonstration

With one selector doctored back to slash form, h2spec ran only **86** tests and the OLD harness would have been **silently green**; the repaired harness fails with layer 1 naming the selector plus three layer-3 lines (6.9/6.9.1/6.9.2 at 0<min). The guard catches the original phase-85 defect shape by construction, demonstrated not asserted.

### 1.9 NINTH — a lint trap for the IMPL: `gofmt` flags the roster map's alignment

The 31-entry `expectedSuites` literal must be `gofmt -w`'d before the golangci gate (P1 hit it live). Trivial, but the IMPL's per-task lint discipline should expect it.

---

## 2. The two probe verdicts, and the conditional arms they decide

### 2.1 The 6.5.2/2 probe — **the pass is GENUINE, and the SPEC's conditional IWS arm dissolves: the guard lives in the VENDORED FRAMER, not in envoy-go**

**MEASURED at the frame level (P2, standalone raw-framer probe + h2spec `--verbose`, both saved):** the oversized SETTINGS (IWS=2^31) **never reaches `onSettings` at all**. x/net v0.34.0's `parseSettingsFrame` (frame.go:746-753) rejects `SETTINGS_INITIAL_WINDOW_SIZE > 2^31-1` at frame-PARSE time, returning `ConnectionError(ErrCodeFlowControl)` from `ReadFrame`; that flows through `translateFramerErr` (`internal/filter/hcm/h2/framer.go:54-57`) into the frame loop's conn-error branch (`conn.go:160-170`) → `emitGoaway(FLOW_CONTROL_ERROR)` (`conn.go:676`) → drain and close. The probe's frame log shows the GOAWAY(FLOW_CONTROL_ERROR) arriving immediately; h2spec's verbose block accepts it as the expected connection error (its own logger prints the frame it sent as `??? Frame (Failed to parse the frame)` — h2spec's display parser enforces the same bound). **The brief's candidate hypothesis (int32 conversion → `<=0`→65535 clamp) is REFUTED** — `clientS.InitialWindowSize` is never assigned the oversized value. The same parse-time guard is why **6.9.2/3 passes** while 6.5.2/1,/3,/4 fail: x/net validates ONLY IWS among setting values.

**Verdict and the dispositions it forces:**

1. **`reference_compensating_defects_cancel_in_the_gate_metric` does NOT bite** — the pass is deterministic, RFC-correct delegation to the vendored framer, not a masking defect.
2. **An IWS>2^31-1 arm inside `validateSetting` is UNREACHABLE dead code on the server read paths** (both `onSettings` and `readClientSettings` sit behind the same `fr.ReadFrame`). Disposition, MEASURED by P1: the branch is KEPT as documented defense-in-depth at a cost of **3 code lines + the delegation comment** — it protects against an x/net upgrade relaxing the parse-time check. The mid-connection unit arm is kept as a **REGRESSION PIN of the delegated guard** (P1 measured it **PASS on the unfixed tree** — arm 5, the only "expected RED" that arrived green mid-connection), NOT counted as a RED anchor.
3. ⚠️ **A REAL wrong-code defect the probe exposed on the handshake path:** `readClientSettings` wraps ANY `ReadFrame` error as `ErrProtocolError` (`settings.go:80-83`), so a HANDSHAKE-time oversized IWS today yields GOAWAY(**PROTOCOL_ERROR**) — RFC 9113 wants FLOW_CONTROL_ERROR. h2spec never tests this (6.5.2/2 sends the oversized frame mid-connection), but the fix-to-SPEC rule binds: the handshake IWS arm asserts FLOW_CONTROL_ERROR and went RED on the unfixed tree with exactly the wrong-code signature (`GOAWAY code = PROTOCOL_ERROR, want FLOW_CONTROL_ERROR`, P1 Phase A). The plumbing fix, measured: **reuse `translateFramerErr` inside `readClientSettings` (4 code lines) + `Run` step-3 own-code emission (+9/−2, of which 5 code)**.

### 2.2 The IWS=0 seeding-quirk probe — **the quirk is REAL but UNIT-ONLY-DISCRIMINABLE; the two probe agents CONFLICTED and the conflict was resolved BY EXECUTION**

**MEASURED (P2: baseline + quirk-only patch + IWS census; P1: walk-without-quirk + walk-plus-quirk, gate and unit level):**

- **P2's halves:** baseline (neither fix) fails 6.9.2/1 with `Actual: DATA Frame (length:3, …)` — the explicit IWS=0 clamped to 65535, the body escaping immediately. Quirk-only (no walk) fails it with `Actual: HEADERS Frame …` then Timeout — seeds 0 correctly, but the 0→1 raise never reaches the live stream. The IWS census: 97 connections announce the default 65535; exactly THREE announce anything else — **5.1.2/1** (IWS=0, verdict-invariant), **6.5.3/1** (IWS=100 then 1 in one frame, last-wins, passes both ways), **6.9.2/1**. 6.9.2/3's oversized value is parse-rejected upstream (§2.1). The invariant **1 skipped** is **6.9.2/2** in every run either agent made.
- ⚠️ **P2's extrapolation "the two fixes are JOINTLY REQUIRED — the gate cannot reach 95-green without the quirk fix" did NOT SURVIVE P1's execution** (`reference_refutation_must_answer_the_claim_as_stated` cuts both ways — a probe agent's inference from two cells of a 2×2 is a claim about the third cell it never ran). P1's walk WITHOUT the announced flag ran **95/94/1/0 three consecutive times**: with the effective-old computed by the SAME `<=0 -> 65535` clamp as seeding, seed-65535 + delta(new−65535) lands on the same final window as seed-0 + delta(new−0), and the burst-deferred dispatch (`pendingDispatch`, `conn.go:60-66`) applies the SETTINGS raise before the action's first DATA write — the timing hole P2 predicted is closed by an existing mechanism.
- **The quirk defect is nonetheless REAL:** P1's arm 8 (announced IWS=0 at handshake → response DATA must be HELD) went RED on a tree with the FULL walk landed — `DATA (2 bytes) arrived despite announced SETTINGS_INITIAL_WINDOW_SIZE=0` — and green only after the announced-flag fix. The gate metric was invariant (95/94/1/0) across the quirk fix: **the gate cannot gate it; the unit arm MUST.**

**Disposition: the announced-flag fix is IN** (an RFC 9113 MUST for the window before any subsequent SETTINGS arrives; fix-to-SPEC binds) — `InitialWindowSizeAnnounced bool` on `clientSettings`, set at BOTH assignment sites; seeding and the walk's effective-old BOTH key on it. ⚠️ **The binding constraint the conflict surfaced: seeding and effective-old MUST use the SAME computation** (both clamp-based or both announced-based) — an inconsistent pair reds 6.9.2/1 (`reference_shared_codepath_defeats_per_arm_counts`'s cousin: the two sites form one compensating pair). ADR-0307 records the quirk as *"gate-invariant, unit-gated"* — the SPEC's "named latent quirk if unreachable" branch is DEAD (it IS reachable, by the unit arm).

---

## 3. Global constraints

- **Go 1.23** (`.github/workflows/ci.yml:14,39,92`). **golangci-lint v1.64.8** via `golangci-lint-action@v6.5.2` (`ci.yml:21-23`), `disable-all: true` with **9** linters: `govet errcheck staticcheck unused ineffassign gofmt goimports misspell revive`.
- ⚠️ **`misspell` runs locale US** — a British spelling in a `.go` comment FAILS. ⚠️ **`gofmt -l` NEVER exits non-zero — gate on OUTPUT**, and negative-control the linter (inject a British spelling, watch it fire, restore by checksum).
- **New fixtures: 0** (121 stays 121; the h2spec gate is `test/conformance/`, not `test/differential/`). **New BackendKind: 0. New port: 0. go.mod: +0** (every import is already-present stdlib or x/net). **Stat surface: +0** — no `NewCounter`/`NewGauge` site in any enumerated edit; asserted at IMPL by call-site enumeration **208/36 re-derived at base AND tip**, NEVER via `TestNoNewStat*` (proven blind by execution at the 84.1 PLAN).
- **D-85-SEQ binds the shape: ONE IMPL leg, ONE atomic commit** — fixes + selector flip + guards + JUnit parse + docs + CI enrollment together; TDD order INSIDE the leg; no red tip at any point.
- `ROADMAP.md` is touched **only** by the IMPL, **in place**, row 85's `status` field only (`in-progress` -> `done`, the split-phase rule is moot — single leg). `want` **stays 117**; check (1) goes silent at the flip.
- ⚠️ **h2spec citation caveat stays LIVE until this row's IMPL lands**: "NO ROW MAY CITE h2spec 53/53 AS FRAME-LEVEL EVIDENCE." This PLAN does not pre-cite the repaired gate as present-tense evidence.
- **Reference observations are RECORDED, never copied into expectations** (D-85-REF fix-to-SPEC rule): the reference's four section-6 failures {6.3/1, 6.7/2, 6.9.1/2, 6.9.1/3} and its flaky section-8 twelfth slot go into `CONFORMANCE_PINS.md` prose only.

---

## 4. File structure — the IMPL's edit surface, re-derived at `be018027`

**Production (3 files, 1 package):**

| file | what changes | owed by |
|---|---|---|
| `internal/filter/hcm/h2/settings.go` (or a new `settings_validate.go`) | shared `validateSetting` called from BOTH `readClientSettings` (`:91-106`) and `onSettings`; ENABLE_PUSH ∈ {0,1}, MAX_FRAME_SIZE ∈ [16384, 2^24-1] -> PROTOCOL_ERROR; the IWS>2^31-1 branch KEPT as 3-line defense-in-depth behind x/net's parse-time guard (§2.1 item 2); `clientSettings` gains `InitialWindowSizeAnnounced bool` (§2.2); `readClientSettings` preserves the framer's `ConnectionError` code instead of blanket `ErrProtocolError` (§2.1 item 3) | D-85-WALK / SPEC §10 / §2.1 / §2.2 |
| `internal/filter/hcm/h2/conn.go` | `onSettings` (`:507-538`): validator call + announced-flag set + §6.9.2 walk (announced-based effective old, delta, walk `s.streams` under `s.mu`, overflow -> `connError(ErrFlowControlError)`); seeding (`:372-375`) keys on the announced flag (§2.2); `Run` step 3 (`:117-120`): emit the returned error's OWN code, not unconditional `ErrProtocolError` | D-85-WALK / §2.2 |
| `internal/filter/hcm/h2/flow.go` | `window.adjust(delta int32) bool` — single critical section, int64 overflow check against MaxInt32, negative legal, signal on becoming-positive | D-85-WALK |

**Harness (2 files, same package):**

| file | what changes | owed by |
|---|---|---|
| `test/conformance/h2spec/h2spec.go` | nine selector strings slash->dot (`:25-34`); `expectedSuites` roster map (24 `http2/*` packages -> min case counts); 6.6 comment REWRITTEN (`:30`) + doc-comment (`:17-20`) | D-85-GUARD, D-85-66 |
| `test/conformance/h2spec/h2spec_test.go` | `junitTestSuite` gains `Package`/`ID` attrs; `junitTestCase` gains `ClassName`/`Error`; failed = `Failure != nil \|\| Error != nil`; the `:310-312` zero-case skip DELETED; three guard layers (per-selector ≥1 case, roster presence, per-suite min counts), all filtered to `package` prefix `http2/` | D-85-GUARD, §7 |

**Test (new file only — no existing test file edited):** `internal/filter/hcm/h2/settings_validate_test.go` — the prototype carried all 14 arms in this ONE file at **504 lines** (a second file is permitted if the IMPL splits validation from walk arms; the count is what is pinned, not the file boundary).

**Docs (the D-85-SWEEP reconcile set + close mechanics):** `CONFORMANCE_PINS.md` (append-style), `BEHAVIOR_CONTRACT.md` `:2052-2054` + `:2056` only, `DECISIONS.md` ADR-0307 completion, `STATE.md` (`:38` + §Current roll), `ROADMAP.md` (row-85 flip ONLY), `.github/workflows/ci.yml`, `phases/85-*/PROGRESS.md`, `next-prompt.txt`.

---

## 5. Task decomposition — the D-85-SEQ single leg, TDD order INSIDE it

> **For agentic workers:** subagent-driven per `feedback_execution_style`; subagents commit LOCALLY only and NEVER push; ⚠️ **D-85-SEQ overrides the per-task-commit default: the leg lands as ONE squashed commit** — intra-leg task boundaries are checkpoint-review boundaries, not commit boundaries; the controller squashes at close. Fresh worktree off the CURRENT master tip (from `git rev-parse master`, not from a SHA quoted here), branch `phase-85-impl`. `git -C <abs-path>` for every git command (the Bash cwd silently resets). No `phase-85-*` worktree outlives the stage.

### Task 1 — the unit arms, written FIRST and proven RED on the unfixed tree

**Files:** Create `internal/filter/hcm/h2/settings_validate_test.go` (validator arms, both paths) and `internal/filter/hcm/h2/conn_settings_test.go` (§6.9.2 walk arms). No existing test file is edited.
**Interfaces:** consumes the existing `startServerConn(t, ctx, disp, DefaultServerSettings)` + `writeClientPreface` + x/net `http2.NewFramer` idiom (`conn_test.go:345-438` is the model); produces the RED anchors Task 2 greens.

- [ ] **Step 1: mid-connection validation arms** (drive the full handshake exactly as `conn_test.go:352-393`, then send the offending SETTINGS frame and assert the GOAWAY code on the wire):
  - ENABLE_PUSH=2 → GOAWAY(PROTOCOL_ERROR) — RED anchor
  - MAX_FRAME_SIZE=16383 → GOAWAY(PROTOCOL_ERROR) — RED anchor
  - MAX_FRAME_SIZE=16777216 → GOAWAY(PROTOCOL_ERROR) — RED anchor
  - INITIAL_WINDOW_SIZE=2147483648 → GOAWAY(**FLOW_CONTROL_ERROR**) — ⚠️ **NOT a RED anchor: GREEN ON ARRIVAL** (§2.1 — x/net parse-time guard). Kept as a REGRESSION PIN of the delegated guard; label it as a pin in the test comment.
  - explicit IWS=0 at handshake → a new stream's response DATA is HELD until WINDOW_UPDATE (§2.2) — RED anchor for the quirk fix
  - CONTROLS (green before AND after): MAX_FRAME_SIZE=16384 and 16777215 → SETTINGS ACK, no GOAWAY.

```go
// The assertion shape, per arm (the conn_test.go:412-427 pattern):
if err := fr.WriteSettings(http2.Setting{ID: http2.SettingEnablePush, Val: 2}); err != nil { ... }
deadline := time.Now().Add(3 * time.Second)
_ = clientConn.SetReadDeadline(deadline)
for {
    f, err := fr.ReadFrame()
    if err != nil { break }
    if f.Header().Type == http2.FrameGoAway {
        if gf := f.(*http2.GoAwayFrame); gf.ErrCode != http2.ErrCodeProtocol {
            t.Errorf("GOAWAY code = %v, want PROTOCOL_ERROR", gf.ErrCode)
        }
        return
    }
    if f.Header().Type == http2.FrameSettings && f.(*http2.SettingsFrame).IsAck() {
        t.Fatal("server ACKed an invalid SETTINGS frame") // the RED reading on the unfixed tree
    }
}
```

- [ ] **Step 2: handshake-path arms** — the same invalid values sent as the client's FIRST SETTINGS frame (before any ACK exchange), asserting GOAWAY with the value's own code. ⚠️ The handshake IWS arm asserts **FLOW_CONTROL_ERROR and is RED on the unfixed tree for the WRONG-CODE reason** (§2.1 item 3: `readClientSettings` blanket-wraps every `ReadFrame` error as `ErrProtocolError`, so today the subject emits GOAWAY(PROTOCOL_ERROR)) — this arm forces Task 2's `readClientSettings` + `Run` plumbing edits.
- [ ] **Step 3: §6.9.2 walk arms** (open a stream with a response the dispatcher holds >64 KiB so the send window is observable, per the `TestServerConn_WriteData_RespectsPerStreamSendWindow` model at `conn_test.go:1124`):
  - (a) increase: client announces IWS=65535 at handshake, opens a stream, then sends SETTINGS IWS=70000 → the in-flight stream's pending DATA advances by exactly +4465 without any WINDOW_UPDATE.
  - (b) decrease-negative-then-unblock: handshake IWS=1000, stream consumes it, SETTINGS IWS=0 drives the window negative → a pending write stays blocked; a stream-level WINDOW_UPDATE unblocks it.
  - (c) overflow: drive a live stream's window so that adjusted past 2^31-1 → GOAWAY(FLOW_CONTROL_ERROR) — a CONNECTION error, unlike WINDOW_UPDATE's stream error (`onWindowUpdate` `conn.go:559+` keeps its stream-scoped shape; assert the difference).
  - (d) new-stream seeding: after SETTINGS IWS=70000, a NEWLY opened stream seeds at 70000 — ⚠️ **NOT a RED anchor: GREEN ON ARRIVAL** (§1.6 — `onSettings` already stores, seeding already reads). Kept as a REGRESSION PIN, labeled as such.
- [ ] **Step 4: run RED and RECORD.** `go test ./internal/filter/hcm/h2/ -run 'TestSettingsValidate|TestSettingsWalk|TestHandshake' -count=1`. Measured on the unfixed tree at this PLAN (P1 Phase A): **ELEVEN RED** — mid-conn EP=2, MFS=16383, MFS=2^24 ("expected GOAWAY(PROTOCOL_ERROR); got SETTINGS ACK" each); handshake EP/MFS-low/MFS-high (same signature); handshake IWS ("GOAWAY code = PROTOCOL_ERROR, want FLOW_CONTROL_ERROR"); walk 7a ("live stream send window = 65535, want 70000"); walk 7b ("DATA (10 bytes) arrived while window should be negative (−50)"); walk 7c ("expected GOAWAY(FLOW_CONTROL_ERROR); got SETTINGS ACK"); arm 8 ("DATA (2 bytes) arrived despite announced SETTINGS_INITIAL_WINDOW_SIZE=0"). **THREE GREEN by design:** the MFS boundary controls, arm 5, arm 7d (pins). ⚠️ A RED arm failing for the WRONG reason (build error, harness bug) is not a RED anchor — read each failure line (`reference_deliberate_break_wrong_assertion`).

### Task 2 — production: shared validator, `Run` code plumbing, `window.adjust`, the walk

**Files:** Modify `internal/filter/hcm/h2/settings.go`, `internal/filter/hcm/h2/conn.go`, `internal/filter/hcm/h2/flow.go`.
**Interfaces:** produces `validateSetting(st http2.Setting) *Error` (settings.go) and `(*window).adjust(delta int32) bool` (flow.go); consumed by `onSettings`, `readClientSettings`, and Task 1's arms.

- [ ] **Step 1: the shared validator** in `settings.go`:

```go
// validateSetting enforces the RFC 9113 §6.5.2 value constraints for one
// SETTINGS parameter. nil means acceptable. The returned error carries the
// RFC-mandated connection-error code: PROTOCOL_ERROR for ENABLE_PUSH and
// MAX_FRAME_SIZE violations, FLOW_CONTROL_ERROR for an INITIAL_WINDOW_SIZE
// above 2^31-1. Called from BOTH the handshake path (readClientSettings) and
// the mid-connection path (ServerConn.onSettings) — phase 85; ADR-0307.
func validateSetting(st http2.Setting) *Error {
	switch st.ID {
	case http2.SettingEnablePush:
		if st.Val > 1 {
			return &Error{Code: ErrProtocolError, Msg: "SETTINGS_ENABLE_PUSH must be 0 or 1"}
		}
	case http2.SettingMaxFrameSize:
		if st.Val < 16384 || st.Val > 16777215 {
			return &Error{Code: ErrProtocolError, Msg: "SETTINGS_MAX_FRAME_SIZE outside [2^14, 2^24-1]"}
		}
	case http2.SettingInitialWindowSize:
		// Values > 2^31-1 are rejected at frame-PARSE time by the vendored
		// x/net framer (parseSettingsFrame returns
		// ConnectionError(ErrCodeFlowControl) before this code runs), so this
		// arm is defense-in-depth against an x/net behavior change, not a
		// live rejection path. Measured cost: 3 code lines (PLAN §2.1).
		if st.Val > 2147483647 {
			return &Error{Code: ErrFlowControlError, Msg: "SETTINGS_INITIAL_WINDOW_SIZE exceeds 2^31-1"}
		}
	}
	return nil
}
```

  Call it from `readClientSettings`'s `ForeachSetting` (`settings.go:91-106`) — validate-then-apply, first error wins, captured outside the closure — and from `onSettings` (`conn.go:515-532`) the same way.
- [ ] **Step 2: the error-code plumbing, BOTH layers** (§2.1 item 3):
  - `readClientSettings` (`settings.go:80-83`) stops blanket-wrapping `ReadFrame` errors as `ErrProtocolError` — preserve the framer's `ConnectionError` code (reuse `translateFramerErr` or `errors.As` on `http2.ConnectionError`), so a handshake-time parse-rejected IWS surfaces FLOW_CONTROL_ERROR.
  - `Run` step 3 (`conn.go:117-120`) emits the returned error's OWN code:

```go
if err := readClientSettings(s.fr, &s.clientS); err != nil {
	code := ErrProtocolError
	var h2err *Error
	if errors.As(err, &h2err) {
		code = h2err.Code
	}
	s.emitGoaway(code)
	return err
}
```

- [ ] **Step 2b: the announced-flag quirk fix** (§2.2 — CO-REQUISITE for 6.9.2/1; P2's measured patch shape):
  - `clientSettings` gains `InitialWindowSizeAnnounced bool` (`settings.go:32-38`), set at BOTH assignment sites (`readClientSettings` `settings.go:95-96`, `onSettings` `conn.go:519-520`).
  - Seeding (`conn.go:372-375`): `peerInitWindow := int32(s.clientS.InitialWindowSize); if !s.clientS.InitialWindowSizeAnnounced { peerInitWindow = 65535 }` — an announced value cannot exceed 2^31-1 (parse-rejected upstream), so the int32 conversion is safe.
- [ ] **Step 3: `window.adjust`** in `flow.go` — a SINGLE critical section (do NOT copy the existing two-acquisition check-then-act at `conn.go:567-568`):

```go
// adjust applies an INITIAL_WINDOW_SIZE delta (RFC 9113 §6.9.2) atomically.
// Reports false when the adjusted window would leave int32 range (the caller
// treats that as a connection error FLOW_CONTROL_ERROR). A negative result is
// legal — reserveBlocking blocks until a WINDOW_UPDATE replenishes. On a
// positive delta that leaves the window > 0, signal a blocked reserver the
// same way replenish does.
func (w *window) adjust(delta int32) bool {
	w.mu.Lock()
	sum := int64(w.n) + int64(delta)
	if sum > math.MaxInt32 || sum < math.MinInt32 {
		w.mu.Unlock()
		return false
	}
	w.n = int32(sum)
	wake := delta > 0 && w.n > 0
	w.mu.Unlock()
	if wake {
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
	return true
}
```

- [ ] **Step 4: the walk** in `onSettings`'s `SettingInitialWindowSize` case (`conn.go:519-520`), inside the existing `ForeachSetting` with the error captured out:

```go
case http2.SettingInitialWindowSize:
	// Effective previous value MUST be computed the same way stream seeding
	// computes it (the announced-flag form from Step 2b) or never-announced
	// connections walk with a wrong delta.
	old := int32(s.clientS.InitialWindowSize)
	if !s.clientS.InitialWindowSizeAnnounced {
		old = 65535
	}
	s.clientS.InitialWindowSize = setting.Val
	s.clientS.InitialWindowSizeAnnounced = true
	if delta := int32(setting.Val) - old; delta != 0 {
		s.mu.Lock()
		for id, ss := range s.streams {
			if !ss.sendW.adjust(delta) {
				s.mu.Unlock()
				walkErr = connError(ErrFlowControlError,
					fmt.Sprintf("SETTINGS_INITIAL_WINDOW_SIZE adjustment overflows stream %d send window", id))
				return nil
			}
		}
		s.mu.Unlock()
	}
```

  The conn-level `s.sendW` is NOT touched (§6.9.2: the connection window changes only via WINDOW_UPDATE). Lock order `s.mu -> w.mu` matches existing code. Validation runs BEFORE application (an invalid frame applies none of its values).
- [ ] **Step 5: run Task 1's arms GREEN**, then the FULL package: `go test ./internal/filter/hcm/h2/ -count=1` and `-count=1 -race` (the walk adds cross-goroutine window writes; the FULL package is the discriminating run, not `-run` isolation — `reference_full_suite_race_after_background_mutator`). Any pre-existing test reddened is a FINDING to report, not silently fix. Measured at this PLAN (P1 Phase B): **all 14 arms green, full package ok, `-race` ok, ZERO pre-existing tests reddened.**

### Task 3 — harness repair: JUnit parse fix, selector flip, three-layer guard, 6.6 comment

**Files:** Modify `test/conformance/h2spec/h2spec.go`, `test/conformance/h2spec/h2spec_test.go`.
**Interfaces:** produces `expectedSuites map[string]int` (h2spec.go) consumed by the layer-2/3 guard; the guard runs BEFORE `assertThreshold`'s aggregation.

- [ ] **Step 1: JUnit structs** (`h2spec_test.go:262-280`) — h2spec emits `<error>` children, NEVER `<failure>` (whole-file 0 vs 4, measured at the SPEC), and `<testcase>` has NO `name` attr:

```go
type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Package   string          `xml:"package,attr"` // e.g. "http2/6.5.2" — the guard key; id COLLIDES across families
	ID        string          `xml:"id,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`      // EMPTY in h2spec output — kept for generic-JUnit tolerance
	ClassName string        `xml:"classname,attr"` // h2spec puts the case description here
	Failure   *junitFailure `xml:"failure"`
	Error     *junitFailure `xml:"error"` // h2spec's actual failure element
}
```

  In `assertThreshold`'s per-case loop: `fail := tc.Failure; if fail == nil { fail = tc.Error }`; the printed name is `tc.ClassName` falling back to `tc.Name`.
- [ ] **Step 2: selector flip** — the nine slash-form strings (`h2spec.go:25-34`) become dotted (`"http2/6.1"` … `"http2/6.10"`); the `:30` comment and the doc comment (`:17-20`) are REWRITTEN per D-85-66: *"6.6 absent from the pinned image (measured 2026-08-08) — the exclusion excludes nothing; retained as documentation of ADR-0051's intent; re-adding `http2/6.6` would select zero cases and trip guard layer 1; see ADR-0307."*
- [ ] **Step 3: the roster** in `h2spec.go`: `var expectedSuites = map[string]int{...}` — all **31** `http2/*` packages (§1.4 — NOT the SPEC's 24) with their MINIMUM case counts, measured from the prototype's captured run (sums to 95):

```go
// expectedSuites pins every http2/* testsuite the pinned image runs under
// thresholdSections, mapped to its minimum case count (guard layers 2+3;
// phase 85 / ADR-0307). Keyed on the <testsuite> package attribute — id
// values COLLIDE across the hpack/generic families. Changes ride the
// pin-refresh procedure (CONFORMANCE_PINS.md) only.
var expectedSuites = map[string]int{
	"http2/3.5": 2, "http2/4.1": 3, "http2/4.2": 3, "http2/4.3": 3,
	"http2/5.1": 13, "http2/5.1.1": 2, "http2/5.1.2": 1, "http2/5.3.1": 2,
	"http2/5.4.1": 2, "http2/5.5": 2,
	"http2/6.1": 3, "http2/6.2": 4, "http2/6.3": 2, "http2/6.4": 3,
	"http2/6.5": 3, "http2/6.5.2": 5, "http2/6.5.3": 2, "http2/6.7": 4,
	"http2/6.8": 1, "http2/6.9": 3, "http2/6.9.1": 3, "http2/6.9.2": 3,
	"http2/6.10": 6, "http2/7": 2,
	"http2/8.1": 1, "http2/8.1.2": 1, "http2/8.1.2.1": 4, "http2/8.1.2.2": 2,
	"http2/8.1.2.3": 7, "http2/8.1.2.6": 2, "http2/8.2": 1,
}
```

  ⚠️ `gofmt -w` the literal before the lint gate (§1.9 — the alignment trips golangci's gofmt rule).
- [ ] **Step 4: the guard**, replacing the `if s.Tests == 0 { continue }` skip (`h2spec_test.go:310-312` — DELETED; it is the shape-32 blindness):

```go
// Guard (phase 85): three layers, keyed on the <testsuite> package attribute.
// The report also carries hpack/* and generic/* suites whose id values COLLIDE
// with http2 ones (id="6.1" twice in the captured XML) — filter FIRST.
var httpSuites []junitTestSuite
for _, s := range suites {
	if strings.HasPrefix(s.Package, "http2/") {
		httpSuites = append(httpSuites, s)
	}
}
// Layer 1: every declared selector matched >= 1 case (a selector like
// "http2/5" fans out: match pkg == sel || HasPrefix(pkg, sel+".")).
for _, sel := range thresholdSections {
	total := 0
	for _, s := range httpSuites {
		if s.Package == sel || strings.HasPrefix(s.Package, sel+".") {
			total += s.Tests
		}
	}
	if total == 0 {
		t.Errorf("h2spec guard layer 1: declared selector %q matched ZERO cases — a silent no-op selector (the phase-85 defect shape)", sel)
	}
}
// Layers 2+3: the pinned roster.
seen := make(map[string]int, len(httpSuites))
for _, s := range httpSuites {
	seen[s.Package] += s.Tests
}
for pkg, minCases := range expectedSuites {
	got, ok := seen[pkg]
	switch {
	case !ok:
		t.Errorf("h2spec guard layer 2: suite %q absent from the report (image drift?)", pkg)
	case got < minCases:
		t.Errorf("h2spec guard layer 3: suite %q ran %d case(s), pinned minimum is %d", pkg, got, minCases)
	}
}
// Layer 2 REVERSE direction (§1.2 — without this, the roster is deletable
// one entry at a time: a removed key is simply never iterated, measured):
// every http2/* suite that actually ran must be rostered.
for pkg, got := range seen {
	if got > 0 {
		if _, ok := expectedSuites[pkg]; !ok {
			t.Errorf("h2spec guard layer 2: suite %q ran %d case(s) but has no expectedSuites roster entry", pkg, got)
		}
	}
}
```

- [ ] **Step 5: run the full gate** `go test ./test/conformance/h2spec/ -count=1 -v` — expected **`95 tests, 94 passed, 1 skipped, 0 failed` → PASS** (the prototype read exactly this FIVE times: capture + two pre-quirk stability runs + two post-quirk runs; the skip is 6.9.2/2, invariant). Run TWICE at the IMPL (-count=1 each).

### Task 4 — the guard NCs, fired on the REPAIRED harness (`reference_gate_command_negative_control`)

**Files:** temporary doctored copies only — restored byte-identically (checksum, not eye) before the leg closes.

- [ ] **NC-1 (layer 1):** doctor ONE selector back to slash form (`"http2/6.9"` → `"http2/6/9"`); the gate MUST FAIL naming that selector. Measured at this PLAN: rc=1, `guard layer 1: selector "http2/6/9" matched no test cases in the JUnit report — selector/suite drift`, plus three layer-3 lines (6.9/6.9.1/6.9.2 at 0<min) — and h2spec ran only **86** tests, which the OLD harness would have reported silently green (§1.8).
- [ ] **NC-2 (layer 2, REVERSE direction):** delete one `expectedSuites` entry; the gate MUST FAIL via the reverse check. ⚠️ Measured: the FORWARD-only form the SPEC specified **did NOT fire** (rc=0 — §1.2); with the reverse check: rc=1, `guard layer 2: suite "http2/6.7" ran 4 case(s) but has no expectedSuites roster entry`.
- [ ] **NC-3 (layer 3):** raise one roster minimum above the real count (6.7: 4→99); the gate MUST FAIL with the count comparison. Measured: rc=1, `guard layer 3: suite "http2/6.7" ran 4 case(s), pinned minimum is 99`.
- [ ] Restore all three, verify by `sha256sum` against the pre-doctor state, and re-run the gate green once. ⚠️ The NC ITSELF can be broken — each NC's failure line must name the layer that fired, not merely "test failed" (`reference_deliberate_break_wrong_assertion`).

### Task 5 — CI enrollment (D-85-CI), same commit

**Files:** Modify `.github/workflows/ci.yml` — the `differential` job (`:32-63`), after the `differential suite` step:

```yaml
      - name: h2spec conformance (phase 85 / ADR-0307)
        # The repaired 95-case strict gate (was 53/53 under nine silently
        # no-op slash-form selectors since 2026-04-25 — ADR-0307). Joins this
        # job because docker + testcontainers are already proven here; the
        # gate's internal timeouts (5m overall / 3m container,
        # h2spec_test.go) sit inside this job's 30-minute cap alongside the
        # ~6.5-minute differential suite. Measured deterministic at n=3
        # locally (95/90/1/4 identical, 5 ms inner spread); a CI-side flake,
        # if one ever appears, is a recordable finding, not a license to
        # unenroll silently.
        run: go test ./test/conformance/h2spec/ -timeout 5m -v
```

  ~13 lines. The `lint-vet-test` job can never reach the gate (`-short` skips at `h2spec_test.go:32-34`) — no other job changes.

### Task 6 — the D-85-SWEEP doc set + ADR-0307 completion + row flip

Per the IMPL close mechanics (§8 below) items 5-10 — the five-file reconcile set (`h2spec.go` landed in Task 3; `CONFORMANCE_PINS.md` append-style; `BEHAVIOR_CONTRACT.md` `:2052-2054`+`:2056` riding ADR-0307; `STATE.md:38` + §Current roll; `ci.yml` landed in Task 5), the ADR-0307 §Decision+§Consequences in-place completion (strict guard 1 -> 0), the ROADMAP row-85 flip with the whole-file leak check, PROGRESS.md, and the router roll.

### Task 7 — the leg's gate evidence, run LAST

Per close mechanics items 1-4: full `TestH2Spec` green (the commit's evidence, quoted in the commit message), full differential suite with `INNER_EXIT`, full `go test ./...`, `-race` on the h2 package, stat-surface call-site enumeration 208/36 at base AND tip, sentinel + all four NCs re-run with ACTUAL output recorded.

---

## 6. The break roster — every gate proven able to go RED at this stage, BY EXECUTION

**FOURTEEN arms, ELEVEN proven RED on the unfixed tree** (the Task-1 Step-4 table: three mid-conn validation arms, four handshake arms including the wrong-code IWS signature, three walk arms, the quirk arm), **TWO proven green-by-design and RECLASSIFIED as regression pins** (arm 5, arm 7d — §1.6; counting them as reds would misstate the TDD ordering), **ONE control pair** (MFS boundaries, green both sides). Plus the **THREE guard NCs fired on the repaired harness** (§Task 4 — including the layer-2 form that did NOT fire as SPEC'd and was repaired before shipping). Every failure line was read and matched to its intended mechanism; no arm reddened for a wrong reason. ⚠️ The IMPL re-runs the eleven REDs at its own tip before Task 2 greens them — a break roster goes stale within its own row (`reference_break_roster_goes_stale_within_its_own_row`).

---

## 7. Cost — the SPEC §10 enumeration REFUTED BY EXECUTION (P1, `git diff --numstat` + new-file `wc -l`, final quirk-fix-included state)

| bucket | SPEC §10 band | **MEASURED** (+/−) | note |
|---|---|---|---|
| selectors + 6.6/doc comment rewrite | ~10-15 | **+13/−10** | h2spec.go |
| guards: roster | (inside ~70-120) | **+42/−0** | 31 entries, not 24 (§1.4) |
| guards: 3 layers + REVERSE layer 2 + filter + skip-deletion | ~70-120 | **≈+63/−6** | reverse check unpriced by SPEC (§1.2) |
| JUnit parse fix | ~10-20 | **≈+31/−7** | Package/ID/ClassName/Error + reporting |
| SETTINGS validation, both paths, incl. `Run` + `readClientSettings` plumbing | ~40-70 | **≈+71/−8** | plumbing was SPEC-named "likeliest unpriced"; materialized (§2.1) |
| IWS=0 quirk fix (announced flag, 2 set-sites, seeding, effective-old) | 0 (SPEC's conditional branch) | **≈+19/−9** | mandatory per §2.2 |
| §6.9.2 walk + `window.adjust` | ~35-55 | **≈+54/−0** | flow.go +31, conn.go ≈+23 |
| unit tests (ONE new file `settings_validate_test.go`) | ~250-450 | **+504** | 14 arms incl. pins + controls |
| **TOTAL net `.go`** | **~420-750 central band** | **+757 net** (797 gross − 40 del) | production +127, harness +126, tests +504 |

**The band's top is EXCEEDED by the unenumerated items alone** (§1.5): pre-quirk snapshot read net +702 (inside the band); the P2-mandated quirk+arm added +55. Docs/ADR/ci.yml buckets (SPEC ~55-90 + ~60-100) are NOT in the `.go` figure and stand as enumerated; the ci.yml step measured ~13 lines (§Task 5). ⚠️ **+757 is itself a LOWER BOUND for the IMPL** — the prototype wrote minimal comments in places the house style wants more, and the IMPL adds the ADR-0307 §Decision/§Consequences and the doc sweep on top. The IMPL's budget: **~760-900 net `.go`**, overrun above that band a recordable finding.

---

## 8. IMPL close mechanics (the controller's own tail, in order)

1. **The commit's gate evidence, run LAST in the leg** (D-85-SEQ): one full `TestH2Spec` green (expected `95 tests, 94 passed, 1 skipped, 0 failed`), quoted in the commit message.
2. **Full `go test ./...`** with honest exit capture (`out=$(...); rc=$?`) and the anchored panic gate `^panic:|DATA RACE|SIGSEGV` on any log it greps. **Full differential suite** `go test ./test/differential/... -timeout 20m` with `INNER_EXIT` asserted — this row anticipates **121/121** and +0 fixtures; the two reserved flake bands and the documented `0079`/`0081` classes apply.
3. **`-race`** on `internal/filter/hcm/h2` (the walk adds cross-goroutine window writes — the full-package `-race` run is the discriminating one, not `-run` isolation).
4. **Stat surface:** call-site enumeration re-derived at base AND tip; expect **208/36 -> 208/36** (+0).
5. **ADR-0307 completion in place** (§Decision + §Consequences after the retained footer; strict guard **1 -> 0** — correct at IMPL only); `DECISIONS.md` stays append-only above the block; next-free moves to nothing new (tail stays ADR-0307, next-free ADR-0308).
6. **`BEHAVIOR_CONTRACT.md` `:2052-2054` + `:2056`** riding ADR-0307 per ADR-0052 `:1821` — before the edit, ENUMERATE the by-line cites the insert would shift (the standing method note; the five historical 53/53 evidence ¶s at `:1464 :1669 :4263 :5002 :5009` are RECORDED, not rewritten).
7. **`CONFORMANCE_PINS.md` append-style**: the 2026-04-25 53/53 record STAYS; append the corrected-scope table (24 suites incl. six 6.x rows, Total 95), the new run record, and the reference-observation ¶ (the four disjoint reference failures + flaky twelfth slot + bridge-vs-host-gateway method caveat).
8. **`STATE.md:38`** flips (`conformance: h2spec 53/53` -> the corrected figure); §Current rolls in place; §Recent evicts its oldest to `STATE_HISTORY.md` strictly append-only (numstat `N 0`, base a byte-exact prefix via `cmp`, absence guard 0/0 with THREE controls re-derived FROM THE FILE).
9. **`ROADMAP.md` row 85 `in-progress` -> `done`** — the ONLY ROADMAP edit; whole-file leak check before AND after (lines 235, rows 117, union 6, `-family row` 95/67, `gRPC-family row` 2, `Operational-tooling-family row` 3, ARM-A flags 119+131 only). ⚠️ After the flip, sentinel check (1) goes SILENT — re-run all three checks + all four NCs and record ACTUAL output; the sentinel still does NOT fire (check (2) prints SIX).
10. **`ci.yml` enrollment step** (§5 Task 5). **PROGRESS.md** stage entry. **`next-prompt.txt` roll** (`git add -f`) to the post-85 state: no banked mid-lifecycle work will remain — the roller SELF-PICKS next per the standing directive.
11. Controller squash to ONE commit on `phase-85-impl`, subject `phase 85 (h2spec-selector-repair) IMPL: ...`, merge to master, push (`feedback_push_to_origin`). Worktree hygiene: `git worktree list` shows only `master` + own stage worktree; no `phase-85-*` worktree outlives its stage.

---

## 9. Sentinel record at THIS stage close

Re-run MECHANICALLY at the stage tip (worktree `phase-85-plan` off `be018027`; `ROADMAP.md` verified BYTE-IDENTICAL to master by empty `git diff`). Input measured **235 lines / 117 data rows** first. At `want=117`: **(1) `NOT DONE: row 85`** — the single EXPECTED line while phase 85 is open at lifecycle-state 3, denominator assertion silent · **(2) SIX — `:195 :201 :207 :217 :223 :231`** · **(3) SILENT**. The condition is a CONJUNCTION and checks (1) and (2) print ⇒ **the sentinel does NOT fire; `stop` NOT created** (`ls stop` => `No such file or directory`, repo root AND stage worktree). **All four NCs fired, observed not predicted:** row-62 doctoring => `NOT DONE: row 62` AND `NOT DONE: row 85` (`NC LANDED? [ in-progress ]` inspected first) · `want=116` => `GATE FAIL: examined 117 data rows, expected 116` · check-(3) doctoring (residual confirmed **2 -> 0** on the doctored copy first) => `NEVER OPENED: gRPC` with WASM correctly silent · check-(2) one-arm counts **5** (long) / **1** (short), union **6**. **Leak axes INVARIANT by whole-file count:** lines **235** · rows **117** · union **6** · `-family row` **95/67** (`--`-guarded) · `gRPC-family row` **2** · `Operational-tooling-family row` **3** · ARM-A flags **119+131 ONLY** (row 85 at `:147` NOT flagged).

## NEXT

**IMPL — phase 85, the single leg** per the task list above. It owes: the TDD sequence with the gate evidence LAST; the guard NCs re-fired on the landed tree; the six-gate posture statement with departures named; the D-85-SWEEP doc set; ADR-0307 completed in place; the row flip; the router roll.
