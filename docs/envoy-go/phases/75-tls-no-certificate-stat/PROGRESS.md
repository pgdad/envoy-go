# PROGRESS 75 — the downstream TLS `ssl.no_certificate` success-path annotation at listener scope

> Live task ledger for the phase-75 IMPL. `PLAN.md` is the spine; **this file records what ACTUALLY happened per task** — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions **and whether their stated rationale survived scrutiny**, and every PLAN/SPEC claim refuted by execution. A break that does not fire is a FINDING, not a nuisance.
>
> ⚠️ **THIS ROW IS UNUSUAL: the PLAN stage already executed most of it.** SPEC §16 closed with *"NO BUILD OF ROW 75 EXISTS"*; that is no longer true. Three private build worktrees compiled the row, ran the unit suite and the FULL differential green, and executed **eight of the ten breaks**. **The IMPL's job is therefore verification-in-place and the docs half — not discovery.** Where the PLAN says `[RUN]`, do not re-derive from scratch; re-run and confirm.

## Stage pointer

- **PLAN done** (2026-07-25; flipped only AFTER §Adversarial-pass record below was populated from the agents' actual reports — writing "done" over an open gate is the exact class the phase-74 PLAN's own V2 caught in an earlier draft, and phase 74 had itself deleted phase-73's guardrail against it). The **11-task SINGLE-FLAT-ROW TDD spine (T1–T11)** landed with a **TEN-break roster of which EIGHT were EXECUTED at this stage**. Every SPEC anchor RE-DERIVED at `cedd2f27` by four read-only agents, **plus three build-by-execution agents in separate worktrees**, plus controller re-verification of every load-bearing correction. Docs-only: `PLAN.md` + `PROGRESS.md` are the ONLY two files in the phase-directory delta — **ZERO production `.go`; ROADMAP and DECISIONS BYTE-UNTOUCHED; row 75 STAYS `in-progress`.** Worktree `/home/esa/git/envoy-go-wt-p75plan` off master `cedd2f27`, branch `phase-75-plan`. Sentinel re-run MECHANICALLY TWICE (worktree + landed master post-push): does **NOT** fire; `stop` NOT created. Counts UNCHANGED (fixtures 119 · fuzzers 55 · stat surface 1204 · BackendKind 38 · modules 2 · DECISIONS tail ADR-0297 PROPOSED). **Next → the phase-75 IMPL.**

- **IMPL** — *(in progress; T1 landed `1522faf3`. Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, off master `9f5d667b`. Per-task record under `# IMPL record`.)*

## Adversarial-pass record (PLAN §1.5)

*(Written AFTER the pass, from what the agents and the controller actually found — **never asserted in advance over a placeholder**. ⚠️ The phase-74 `PLAN.md` did exactly that: it shipped `STATUS: COMPLETE` citing THIS file's phase-74 equivalent before that file existed, having deleted the phase-73 sentence that forbids it. The guardrail is restored in PLAN §1.5 verbatim, and **the controller verified this file exists on disk before flipping that status.**)*

**STATUS: COMPLETE.** Seven agents, disjoint remits, all in PRIVATE scratch; the three build agents in **separate git worktrees on their own branches**, never on `master`, never pushed.

| agent | remit | result |
|---|---|---|
| **A1** | `manager.go` + `internal/stats/{name,registry}.go` + the import gate | 1 REFUTED anchor, 10 new findings |
| **A2** | every test-side guard + the helper roster + the repo-wide sweep | 2 REFUTED, 7 new sites, **the structural analysis that reframed the row** |
| **A3** | `0110` + `0111` + `runner_test.go` dispatch + harness readiness | 3 REFUTED, 8 new findings |
| **A4** | all three docs, each edit APPLIED to a scratch copy and verified | 5 REFUTED, 10 new findings |
| **V1** | BUILT the production change + the positive arm + the variadic helper; 6 breaks | **the §1.3 headline** |
| **V2** | BUILT the guard half; 5 breaks; the `0111` analysis | the ordering inversion + the `-run` footgun |
| **V3** | BUILT the `0110` asserter, ran it against the LIVE reference; 9 breaks | **the first subject-side measurement** + F2/F3/F4 |

**THE EIGHT that changed the PLAN's instructions** — full text in `PLAN.md` §1.5:

| # | Agent | What it caught |
|---|---|---|
| **S1** | V1 (SEVERE) | **The row's discriminating break is INVERTED.** SPEC §10.1 and router item 5(a) both say the new positive arm *is* the discriminating break for the pinned predicate. Break D fires the **PHASE-74** mTLS arm instead; **the positive arm PASSES.** Break D′ — the same edit with the negative roster left at three leaves — leaves the **ENTIRE package GREEN** (`ok … 3.233s`). ⇒ **the roster extension is the SOLE guard on the row's central decision.** Controller-reproduced end-to-end. |
| **S2** | V3 (SEVERE) | **The fast-failure suppression is REFERENCE-ONLY.** envoy-go's `serveConnection` accounts `ssl.*` at step (6), strictly before the step-(7) upstream dial; the subject's numbers were **byte-identical** under a refused upstream while the reference gave four honest zeros. The SPEC states the hazard as general. |
| **S3** | V3 (SEVERE) | **The ABSENT check does not guard what the SPEC says.** Deleting the registration of a counter that IS `Inc`'d produces a nil-pointer SIGSEGV in the subject subprocess; the run dies at `structuralCheck` and `AssertStats` never executes. Its real remit is a name registered-but-absent-from-the-scrape, plus stopping the `want: 0` row from passing vacuously. |
| **S4** | V2 (SEVERE) | **The tree goes RED at the production change, before any guard edit.** Two `reflect.DeepEqual` exact-set pins compare a 3-element `want`. ⇒ **T1 orders the guard FIRST**, restoring normal red→green instead of recording an inverted shape. |
| **S5** | V2 (SEVERE) | **A stale `-run` selector on a UNIT test prints `ok … [no tests to run]` and EXITS 0.** Controller-reproduced live. A break command carrying the wrong side of T1's rename self-certifies green while executing nothing. A NEW SHAPE of `reference_differential_run_selector`. |
| **M6** | V2 (MAJOR) | **The "delete the registration" break destroys its own evidence full-package** — a SIGSEGV in a background goroutine names no test and aborts before the guard's output flushes. **Break B now MANDATES `-run` isolation**; the answer to "assertion or panic?" is BOTH, decided by the command. |
| **M7** | A4 (MAJOR) | **A new BEHAVIOR_CONTRACT paragraph would break SEVEN live line citations.** `:1849`–`:1859` are cited by line from ROADMAP/STATE/SPEC/BRAINSTORM. ⇒ **T8 appends IN PLACE to `:1853`**; the ledger line is the only line-adding edit (`5744 → 5746`). |
| **M8** | A4 (MAJOR) | **ADR-0297's OWN ¶7 and ¶9 are defective.** ¶7's grep claim is **self-falsifying** (`VERIFYIFGIVEN` now returns 2 lines / 3 occurrences, one of them ¶7 itself) — **the same species the phase-74 IMPL had just fixed in ADR-0296 ¶3, reproduced one ADR later.** ¶9's *"SELF vs OTHER-ADR (n=4)"* rule is refuted by a n=7 population with a third form, `:17211` being a phase-73 correction to a DIFFERENT landed ADR rendered INLINE. |

**Also folded:** RD-REGDOC (`registerListenerMetrics`' doc starts at `:351`, not `:358` — the SPEC's anchor edits a fragment) · RD-CALLSITES (the SPEC's three "callers" are test-func declaration lines; the real sites are `:4508/:4539/:4557`) · RD-TERROR (`t.Error`, not `t.Errorf`) · RD-POLLFILE (`pollCounter`/`counterValue` live in `quic_test.go`) · RD-CONV (`mkDownstreamTSInline` takes `string`; the SPEC's call does not compile) · RD-RENAME (three sites, incl. a cross-reference in a test the router said needs nothing) · **RD-BC-HEADING** (SPEC §9's *"no two-ADR heading precedent exists"* is refuted — there are **16**, and `:785` is the exact later-phase-extends shape) · RD-BLINDSPOT (**107/103/4**, and A4-N5 shows the recorded figure's *provenance* claim is false — 106/102 is the pre-BRAINSTORM number, invalidated by row 75's own addition in the very commit that claimed to re-derive it) · F9 (`three-fifths` was already wrong at the phase-74 tip, self-inconsistently within phase 74's own edits — **do not bump to four-fifths**) · F10 (the ledger is discontinuous in **TWO** places: the recorded `1200 → 1201` **and** an unrecorded `1198 → 1200`) · F11 (`:4685` is phase-29.1 mongo, not Wasm) · F12 (do NOT extend the `:152-157` listener table — phase 74 deliberately did not) · RD-PANICS (**five** panic sites in `registry.go`, not three — **a correction of this PLAN's own agent**) · RD-NOOUTCOME (do NOT touch `handshakeOutcome`) · RD-STATSSINK (a FIFTH stale count site, stale since phase 49) · RD-PREEXISTING (**`TestNewManager_ChainSelectionPropagation` already drives the phase-75 Inc site today** via `mkDownstreamTSInline`) · V1-E7 (two gofmt alignment traps) · A4-B14 (the candidates sentence has **ZERO interior periods** — that is *why* `[^.]*\.` binds, so no replacement may introduce a `.`).

**Three findings ACCEPTED AS-IS, reasoning recorded rather than instruction changed:**
- **A2's warning against a shared roster does not bite V1's `sslLeafRoster`** — it holds bare LEAF names while the spelling pins hold FULL names in independent literals, so no single misspelling satisfies both. **A2's constraint is retained as a standing invariant at T3.**
- **V1's empty-`wantSuffixes` `Fatal` is unproven dead-defensive code by construction** and is kept: three lines, and it closes the one hole the variadic signature opens.
- **V2's objection to the `0111` value assertion is retired on the SPEC's own executed evidence** (§Context D7 + §3.3 F1 show eager registration at `require=true`), **but the decision still goes V2's fallback way, for a different reason** — see below.

### ⚠️ A controller decision that REVERSED itself, recorded rather than quietly amended

The router required the `0111` question to be **decided explicitly**. Two agents recommended adding `envoy_listener_ssl_no_certificate` to `0111`'s three value rosters, and **the controller initially agreed**, on the ground that `0111`'s separate ABSENT check makes a `want: 0` a live cross-side registration guard rather than a vacuous `0 == 0`.

**Two findings then arrived that changed the answer.** (1) **F4:** the ABSENT check does NOT catch a deleted registration on a counter that is actually `Inc`'d — the crash kills the run first — so the "live registration guard" argument is much weaker than it looked. (2) **A4-N6:** of the NINE TLS-downstream fixtures, **only `0110` is `require=false`**; `0111` is `require=true` on both sides, so the annotation reads **0 on every arm, structurally**, and a value pin would document a vacuous `0 == 0` as coverage while `0110` now carries the real assertion **with a discriminating non-zero**.

⇒ **DECISION (PLAN §2.4): `0111`'s value rosters stay at THREE; `0111`'s PROSE gains `ssl.no_certificate` in its named-unasserted lists, with the reason.** The prose edit is **not optional** — RD-0111-CLOSED shows both passages are CLOSED ENUMERATIONS, so a name in neither list reads as asserted. **The reversal is recorded because a decision changed on evidence, and hiding that would make the reasoning unauditable.**

### What the build agents CONFIRMED by execution (the IMPL does NOT have to re-derive this)

- **The whole row COMPILES**, `gofmt`/`go vet`/`golangci-lint` clean, `go test ./internal/... -count=1` **zero failures**, `-race` green on `internal/listener` + `internal/stats`.
- **The FULL differential suite: `ok … 399.675s`, exit 0, no flake.** Fixture count stays **119**.
- **THE FIRST SUBJECT-SIDE MEASUREMENT, and exact cross-side agreement:**
  ```
  reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
  subject   ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
  ```
  The discriminating asymmetry (`handshake=2` vs `no_certificate=1`) is real and observed **on both sides**. `envoy_listener_ssl_no_certificate{envoy_listener_address="0.0.0.0_10446"} 1` (reference) vs `…{envoy_listener_address="___20016"} 1` (subject) — **metric NAMES byte-identical, addresses differ ONLY in the label.**
- ⚠️ **The subject binds the IPv6 WILDCARD** — the label is `___<port>`, not `0_0_0_0_<port>`. V3 wrote the keying comment wrongly and **caught it by execution**.
- **The Prometheus key `envoy_listener_ssl_no_certificate` was verified BY EXECUTION twice independently**, address-invariant across IPv4/wildcard/IPv6; `IsValidName` true for every form; `len(helpText)` 14 → 15.
- **Eight of ten breaks ran** with the outcomes in `PLAN.md`'s break map, including the two declared MUST-STAY-GREEN (D′, G) which both stayed green as required.

### ⚠️ NOT verified — the IMPL inherits no false confidence

- **`-race` over `./test/differential/`** was NOT run (the plain full suite is already ~400 s). T10 owes it or an explicit skip.
- **`ssl.no_certificate` on QUIC was never DRIVEN** — the registration is pinned and green, but no H3 connection confirmed the counter does not MOVE. The parity argument is STRUCTURAL (no accept loop for `kindQUIC`), hence name-independent — an argument, not a measurement.
- **The pinned predicate was never cross-checked at `require_client_certificate: true`** (`0109` not run). Break F proves the mode term is load-bearing *in envoy-go*; the reference-side evidence is the SPEC's wire trace.
- **Resumption/renegotiation never exercised** (`ssl.session_reused` 0 in every run) — the one scenario in which the pinned predicate could be wrong in production.
- **Break B's crash under `-race`, and whether it can contaminate a full-suite run** — isolation only.
- **Nothing reference-side was re-probed for the SEMANTICS docket.** Every §3 figure in T8's prose is transcribed from ADR-0297 §Context; **T8 must cite the ADR paragraph, not present them as re-derived.** The `0110` fixture leg is the one exception — it WAS executed live.
- **The absolute total `1205`** — no mechanical command; the **+1 delta** is solid, the TOTAL now rides **two** documented gaps.
- **Two of this PLAN's own agents had a correction refuted by the controller** (A1's "three panics" → five; A3's `cx_total == 3` precondition → non-discriminating). **Treat every RD-* row as a claim.**

## Task ledger (Status/Commit filled at the IMPL)

| # | Task | Breaks | Status | Commit |
|---|---|---|---|---|
| T1 | field + `rt.tlsMode`-gated registration + the TWO exact-set pins (**RED via the pins**) | A, B | **DONE** — RED-first, both breaks FIRED | `1522faf3` |
| T2 | `GateMatchesInc` +2 `t.Error` pointer assertions (the nil-Inc crash guard) | C | **DONE** — no one-step red available (T1 already green); Break C FIRED, predicates SILENT | `ea67486d` |
| T3 | **the guarded Inc + VARIADIC `assertSSLCrossProduct` + roster→4 + the positive arm** ⚠️ the row's load-bearing task | D, D′, E, F | **DONE** — RED-first; D fired the PHASE-74 arm, D′ FULLY GREEN, E and F fired the positive arm | `9b63c5aa` |
| T4 | `helpText` entry + `wantNames` + both count claims (**RED via `wantNames`**) | G, G′, G″ | **DONE** — RED-first; **G STAYED GREEN** (the silent-staleness finding), G′ FIRED, G″ GREEN | `621a899e` |
| T5 | `0110` cross-side `StatsAsserter` + `scrapeProm` + `var _` + the second precondition | H, I, J | **DONE** — first cross-side measurement, PASS on the first run; H fired both sides, I-a fired the ABSENT branch, I-b passed VACUOUSLY (the finding), J's both legs as predicted; E UNREACHABLE (F3) *(row filled at T6 — T5 left it blank)* | `11e9e89d` (+ record `f4e725ef`) |
| T6 | `0110`'s three stale `ssl.*` confessions (SPLIT the README bullet; the third clause INVERTS) | — | **DONE** — three sites retired **plus a FOURTH the 14-pattern sweep missed** (`(no StatsAsserter)`); leg (c) added; fixture GREEN | `64e26282` |
| T7 | `0111` prose: `no_certificate` joins the NAMED-UNASSERTED lists (driver BYTE-UNTOUCHED) | — | **DONE** — both closed enumerations + leg (c)'s roster line; driver byte-untouched (empty `diff --stat`); **`0111` EXECUTED for the first time — PASS** | `a1ac5027` |
| T8 | BEHAVIOR_CONTRACT: eleven in-place edits + ONE new ledger line (no new paragraphs) | — | **DONE** — 11 in-place `-N +N` rewrites + 1 added ledger line (5744 → 5746); the FOURTH `ssl.*` name recorded, both bare totals `1204` → **`1205`**, `:1849`'s heading EXTENDED (**SPEC §9's no-precedent claim REFUTED**, `:785` is the precedent); `:962` byte-identical and its four figures re-derived a FOURTH time; **TWO PLAN figures REFUTED** — RD-BC-TOTALS' `3 → 1` is actually **3 → 2**, and Step 3's `grep -c '1201'` *"UNCHANGED"* is contradicted by Step 2's own mandate (`1 → 2`); docs-only, ONE file *(row left blank by the T8 agent; filled at T10)* | `e6092777` |
| T9 | the stale-"three" prose sweep (5 files; none of it fails a test) | — | **DONE** — **3 files touched, not 5** (`name.go`/`name_test.go` were already fully retired at T4); 7 sites edited incl. **one the PLAN roster missed**; 2 PLAN entries REFUTED as already-done; PROSE-ONLY (zero non-comment diff lines) | `a4d4908c` |
| T10 | the six-gate + full differential + `-race` + counts + a TWO-CATEGORY envelope audit | — | **DONE** — six-gate ALL SILENT; differential run 1 FAILED (`0083`, port-bind startup flake, isolate-PASS) and run 2 **119/119/0 FAIL/0 SKIP**; fixture-dir set ≡ subtest set (`comm -3` empty); **`-race` RUN over `./...` INCLUDING the differential — 0 DATA RACE**, one FAIL = `0061` ring-hash spread (**`reference_0061_ring_hash_spread_flake`, SECOND occurrence — its "investigate margins" trigger has FIRED**), isolate-PASS ×4; imports **+0 production** (negative control RE-RUN and firing) / **+4 test** in one driver; ZERO new exported symbols, ZERO new dep edges; all six counts HOLD; sha256 **592 files / 0 mismatch / 0 missing**, EDIT ∩ GATE **empty**. ⚠️ **TWO PLAN gate commands REFUTED** — the import gate fails CLOSED (`FILENAME` prefix), the `go doc` gate fails OPEN (silently drops `./internal/stats`) | `e860b817` |
| T11 | ADR-0297 completed **and CORRECTED** + the ADR-0296 blockquote + ROADMAP row flip + narrow + STATE | — | | |

## Cross-cutting hazards (bind every task)

1. **⚠️ The roster extension in `assertSSLCrossProduct` is the row's PRIMARY guard, not cleanup.** Break D′ proves the package goes fully green without it, with the pinned predicate broken.
2. **⚠️ Re-derive every `-run` string after T1's rename.** A stale selector prints `ok … [no tests to run]` and **exits 0**.
3. **⚠️ Break B must be `-run` isolated** or its SIGSEGV destroys its own evidence.
4. **⚠️ Do NOT add a nil guard at the Inc site** — it would make Break B vacuous and mask the invariant. The +2 pointer assertions are the right defence.
5. **⚠️ Do NOT touch `handshakeOutcome`.** `no_certificate` is a success-path annotation outside the classifier switch.
6. **⚠️ Two gofmt traps:** the field's preceding comment splits the alignment run (single space, and that keeps phase-74 byte-identical); the `helpText` entry needs a **blank separator line** or gofmt re-pads three phase-74 lines.
7. **⚠️ `mkDownstreamTSInline` takes `string`; the PKI holds `[]byte`.** Explicit conversion or it does not compile.
8. **⚠️ BEHAVIOR_CONTRACT: append IN PLACE.** The ledger line is the only line-adding edit.
9. **⚠️ The ROADMAP candidates sentence must keep ZERO interior periods.**
10. **⚠️ The ADR-0288 singleton greps return 2, not 1.** The second hit is `STATE.md:7`, the rule statement. Never "fix" it to 1.
11. **⚠️ `git -C <abs-worktree-path>` for every git command**, with a `pwd` + branch + commit-count tripwire before any commit or gate run. **This hazard fired live during both the phase-75 BRAINSTORM and this PLAN's session.**
12. **Known flakes: NONE fired in any of this PLAN's three build worktrees**, including a full 400 s differential run. A failure at the IMPL is therefore **more likely real** than at most stages — isolate-re-run, then state the classification and its evidence.

# IMPL record

*(Sections owed at close: Task ledger — ACTUAL · Break ledger (all TEN, with which assertion fired and which did NOT) · PLAN/SPEC claims REFUTED BY EXECUTION · any fresh hazards · flake classifications WITH evidence · the envelope audited in TWO categories · counts at exit re-run MECHANICALLY.)*

## Task 1 — ACTUAL

**Commit `1522faf3`** — *"phase 75 T1: the sslNoCertificate field + the rt.tlsMode-gated registration, RED-first via the two exact-set name pins (+ the rename and its THIRD cross-reference)"*. Files: `internal/listener/manager.go` (+13/-3), `manager_test.go` (+18/-6), `quic_test.go` (+11/-2). Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, base master `9f5d667b`. **No push** (controller squash-pushes).

### Step 0 — collision greps + parallel-stream re-check at the IMPL tip

All five identifiers **0** over `--include='*.go'`, re-run at `9f5d667b` (not copied from the PLAN):

```
sslNoCertificate                               0
startOneWayTLSListener                         0
sslLeafRoster                                  0
TestServeConnection_SSLNoCertificateIncrements 0
no_certificate (GO ONLY)                       0
```

`git diff --stat e822f1ad HEAD -- '*.go' go.mod go.sum test/` ⇒ **EMPTY** ⇒ **RD-TREE still holds at the IMPL tip**; no parallel stream re-minted drift (`feedback_parallel_stream_mints_fresh_drift` does not bite).

### PLAN anchors — RE-DERIVED before use (not trusted)

| anchor | PLAN claim | re-derived at the IMPL tip | verdict |
|---|---|---|---|
| RD-FIELDS | fields `:180-182`, insert as new `:183` | `:178 downstreamCxTotal` · `:179 downstreamCxActive` · `:180 sslHandshake` · `:181 sslFailVerifyError` · `:182 sslFailVerifyNoCert` | **HOLDS** |
| RD-FIELDCOMMENT | "three" at `:175` and `:177`; `:174` "The two cx metrics" stays TRUE | exact; `:174` left byte-untouched | **HOLDS** |
| RD-REGDOC | doc starts `:351` (NOT `:358`); the ONE false sentence is at `:358` | doc span `:351-373`, `func` at `:374`; `:358` *"The three phase-74 ssl.\* counters are"* | **HOLDS** — the SPEC's `:358-373` really is a fragment |
| RD-REGFN | `:378 if rt.tlsMode {` · `:379-381` the three `NewCounter` · `:382 }` | exact; the 4th inserted at `:382` inside the EXISTING gate, **no new gate** | **HOLDS** |
| RD-RENAME | THREE sites: `:1940`, `:2019`, `:2023` | exactly three `*.go` hits, at `:1940` (cross-ref inside `TestListenerManager_AllocatesBaseListenerMetrics`' doc), `:2019` (own doc), `:2023` (decl) | **HOLDS** |
| RD-XSET | two exact-set pins, `manager_test.go:2055` / `quic_test.go:279` | `manager_test.go:2055 if !reflect.DeepEqual(got, want) {` (Errorf `:2056`) · `quic_test.go:279 if got := listenerSSLNames(…); !reflect.DeepEqual(…)` | **HOLDS** |
| RD-SORT | `ssl.no_certificate` sorts 4th/LAST — a pure APPEND | confirmed by execution: the GREEN `got` from `listenerSSLNames`' `sort.Strings` matches the appended `want` element-for-element | **HOLDS** |
| RD-POLLFILE | `counterValue` `Errorf`s on an ABSENT name and returns `-1` | fired live at RED: `quic_test.go:304: counter "…ssl.no_certificate" is not registered` then `:305 … = -1 … want 0` | **HOLDS** — the quic zero-loop's 4th assertion is FREE **and non-vacuous** |
| RD-RUNSTALE | a stale `-run` prints `ok … [no tests to run]` and **exits 0** | reproduced live post-rename (Step 6 below) | **HOLDS** |

**⚠️ One PLAN line-number claim did NOT survive the edit — RD-RENAME's POST-edit numbering.** PLAN Step 6 predicts the three post-edit hits at `:1940, :2019, :2023`. Actual: **`:1940, :2019, :2028`**. The declaration shifted +5 because the PLAN's own replacement doc comment is 9 lines where the original was 4. Cosmetic (the grep count of **3** is what the step gates on), recorded because a later task copying `:2023` as an `old_string` anchor would miss.

**Not a refutation but worth flagging for T9:** T1's verbatim quic_test.go block rewrites the `(1)` banner that T9's roster still lists as owed (`quic_test.go:272`). **T9 must re-derive its quic roster** — `:272` is already fixed; `:65`, `:226` and the `(4)` banner (now `:302`, *"all three ssl.\* counters are STILL ZERO"*) are still owed.

### Step 2 — RED, verbatim (and for the RIGHT reason: assertions ran, no build error)

```
=== RUN   TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames
    manager_test.go:2062: ssl name set = [listener.127_0_0_1_37567.ssl.fail_verify_error listener.127_0_0_1_37567.ssl.fail_verify_no_cert listener.127_0_0_1_37567.ssl.handshake], want [listener.127_0_0_1_37567.ssl.fail_verify_error listener.127_0_0_1_37567.ssl.fail_verify_no_cert listener.127_0_0_1_37567.ssl.handshake listener.127_0_0_1_37567.ssl.no_certificate]
--- FAIL: TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames (0.00s)
=== RUN   TestQUICListener_RegistersSSLNamesAtZero
    quic_test.go:287: QUIC listener ssl name set = [… .ssl.fail_verify_error … .ssl.fail_verify_no_cert … .ssl.handshake], want [… .ssl.fail_verify_error … .ssl.fail_verify_no_cert … .ssl.handshake … .ssl.no_certificate]
    quic_test.go:304: counter "listener.127_0_0_1_42675.ssl.no_certificate" is not registered
    quic_test.go:305: listener.127_0_0_1_42675.ssl.no_certificate = -1 after a completed H3 handshake, want 0
--- FAIL: TestQUICListener_RegistersSSLNamesAtZero (0.00s)
FAIL	github.com/pgdad/envoy-go/internal/listener	0.008s
```

**Both pins fired at their `reflect.DeepEqual` lines**, each a 3-element `got` against a 4-element `want`. The binary COMPILED (`=== RUN` lines present, per-assertion messages present) ⇒ this is a live red, not a build failure. RD-XSET's ordering mandate is therefore vindicated by execution: had the registration landed first, both of these would have been red with no guard written.

### Step 4 — GREEN, verbatim

```
--- PASS: TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames (0.00s)
--- PASS: TestQUICListener_RegistersSSLNamesAtZero (0.00s)
ok  	github.com/pgdad/envoy-go/internal/listener	0.008s
```
```
ok  	github.com/pgdad/envoy-go/internal/listener	3.221s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
```

**`internal/stats` is expected-green here** and was not run — the `helpText` entry is not yet added and nothing complains. **That silent staleness is T4's remit** (PLAN Step 4's own warning).

### Step 5 — hygiene, all silent

```
--- gofmt ---
--- vet ---
--- lint ---
ALL SILENT / exit=0
```

`gofmt -l internal/listener` named **nothing** ⇒ the gofmt trap resolved as the PLAN predicted: the new doc comment splits the alignment run, so `sslNoCertificate *stats.Counter` takes a **SINGLE** space before its type while the five fields above stay column-aligned. **Confirmed byte-identical phase-74 lines** — `git diff` shows the three `ssl*` field lines and both `cx` lines as pure CONTEXT, zero `+`/`-`. No hand-padding was needed or done.

### Step 6 — `-run` selectors RE-DERIVED, and the footgun DEMONSTRATED

```
=== Three (expect ZERO) ===
count=0
=== Four (expect 3) ===
internal/listener/manager_test.go:1940:// pinned by TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames, and
internal/listener/manager_test.go:2019:// TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames pins the EXACT
internal/listener/manager_test.go:2028:func TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames(t *testing.T) {
=== FOOTGUN DEMO ===
ok  	github.com/pgdad/envoy-go/internal/listener	0.004s [no tests to run]
EXIT=0
```

**RD-RUNSTALE re-confirmed LIVE at the IMPL tip:** the PRE-rename selector, which now matches nothing, prints `ok` and **exits 0**. Any downstream break command still carrying `…ExactlyThreeSSLNames` self-certifies green while executing nothing. The surviving `docs/` hits (phase-74 `PLAN.md:422/424/544/623`, phase-75 `SPEC.md:354`, `BRAINSTORM.md`, this PLAN's own `:50/:272/:481/:483`) are **HISTORICAL and were NOT edited.**

### Break A — reorder `want` (index 3 → index 1). **FIRED.**

Run AFTER the commit, `-count=1`:

```
=== RUN   TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames
    manager_test.go:2062: ssl name set = [… .ssl.fail_verify_error … .ssl.fail_verify_no_cert … .ssl.handshake … .ssl.no_certificate], want [… .ssl.fail_verify_error … .ssl.no_certificate … .ssl.fail_verify_no_cert … .ssl.handshake]
--- FAIL: TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames (0.00s)
```

**WHICH assertion fired:** `manager_test.go:2062`, the name-set `reflect.DeepEqual` — and it fired on **ORDER ALONE**: `got` and `want` are the same four-element SET, differing only in position. This is the assertion the doc comment's ⚠️ warns about, proven live rather than asserted. `git restore internal/listener/manager_test.go` ⇒ `git status --porcelain` **empty**.

### Break B — delete the registration line. **FIRED** (isolated, `-count=1`).

```
=== RUN   TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames
    manager_test.go:2062: ssl name set = [… fail_verify_error … fail_verify_no_cert … handshake], want [… fail_verify_error … fail_verify_no_cert … handshake … no_certificate]
--- FAIL: TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames (0.00s)
=== RUN   TestListenerMetrics_GateMatchesInc
--- PASS: TestListenerMetrics_GateMatchesInc (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/mixed_rejected_at_build (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/tls_listener (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/plaintext_listener (0.00s)
FAIL	github.com/pgdad/envoy-go/internal/listener	0.006s
```

**WHICH assertion fired:** the name-set `DeepEqual` at `manager_test.go:2062` (3 vs 4) — **and `TestListenerMetrics_GateMatchesInc` PASSED, correctly**, because T2's +2 pointer assertions have not landed yet. That is the pre-T2 shape the PLAN predicts, and it is also the evidence that **T2's break C is not redundant**: at this tip a deleted registration is invisible to `GateMatchesInc`.

**Extra probe (not required by the PLAN) — the SIGSEGV precondition is CONFIRMED CONDITIONAL on T3.** Full-package under Break B: `FAIL … 3.222s` with **named, honest failures** (`quic_test.go:305 … = -1 …`) and **no crash**, because the Inc site does not exist yet. ⇒ hazard 3's `-run`-isolation requirement is **T3-onward**, not universal; at T1 it is harmless belt-and-braces. Do not read this as refuting V2-M6.

`git restore internal/listener/manager.go` ⇒ `git status --porcelain` **empty**; re-green confirmed `ok … 3.225s`.

### Task 1 — no surprises beyond the two recorded above

Every RD-* row T1 depends on survived re-derivation at the IMPL tip. The only correction is the **post-edit** line number of the renamed declaration (`:2023` → `:2028`), plus the T9-roster overlap on `quic_test.go:272`.

## Task 2 — ACTUAL

**Commit `ea67486d`** — *"phase 75 T2: extend TestListenerMetrics_GateMatchesInc to the FOURTH counter pointer — the nil-Inc-is-a-process-crash guard"*. One file: `internal/listener/manager_test.go` (+19/-3). Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, base master `9f5d667b`, `git rev-list --count master..HEAD` = 2 before the commit. **No push.**

### PLAN anchors — RE-DERIVED before use (ALL FIVE had DRIFTED)

T1's edits to `manager_test.go` shifted every anchor in the PLAN's Task-2 section by **+6 lines**. Not one was usable as written. The PLAN's `**Files:**` line and its Step-1 prose both carry the pre-T1 numbers.

| PLAN anchor | PLAN claim (pre-T1) | re-derived at the T2 tip | verdict |
|---|---|---|---|
| non-nil half, insert point | after `:2199` | after **`:2205`** (`rt.sslFailVerifyNoCert == nil` block closes at `:2205`) | **DRIFTED +6** |
| nil half, insert point | after `:2249` | after **`:2255`** (`rt.sslFailVerifyNoCert != nil` block closes at `:2255`) | **DRIFTED +6** |
| `(b)` banner | `:2152` *"all THREE counter fields are NON-NIL"* | **`:2158`** | **DRIFTED +6** |
| `(c)` banner | `:2203` *"all THREE counter fields are NIL"* | **`:2209`** | **DRIFTED +6** |
| doc comment | `:2126` *"the three field pointers"* | **`:2132`** | **DRIFTED +6** |
| RD-TERROR | the pointer assertions use **`t.Error(`**, not `t.Errorf(` | confirmed at `:2198/:2201/:2204` and `:2248/:2251/:2254`; **0** `t.Errorf(` among them | **HOLDS** |

The `(b)` banner is a wrapped two-line comment, so the PLAN's replacement text *"all FOUR counter fields are NON-NIL (three phase-74 outcomes + phase 75's sslNoCertificate)"* was re-wrapped across `:2158-2159` rather than pasted on one line. Wording preserved verbatim.

Post-edit anchors, for T3+ (these WILL drift again): doc `:2132` · `(b)` banner `:2158-2159` · non-nil assertion `:2213-2214` · `(c)` banner `:2219` · nil assertion `:2269-2270`.

### Step 2 — NO ONE-STEP RED IS AVAILABLE HERE, and none was manufactured

**This is recorded as a deliberate departure from the RED-first form T1 used, not as an omission.** T1 landed both the `sslNoCertificate` field *and* its `rt.tlsMode`-gated registration in one commit. Therefore, at the T2 tip, `rt.sslNoCertificate` is already non-nil on a TLS listener and already nil on a plaintext one — **both new assertions pass the instant they compile.** There is no intermediate state of the tree in which they fail.

The alternatives were both rejected as false records:
- **Manufacturing a red** (writing the assertions inverted, or against a field that does not yet exist) would have produced either a fabricated failure or — the phase-74 trap the PLAN names explicitly — a **build failure** in which *zero* assertions run, logged as if it were an assertion red.
- **Deferring the assertions to T3** would have left Break B/D's failure mode as an unattributed background-goroutine SIGSEGV.

**This task's red is Break C**, run after the commit, below. That is the whole reason Break C is not optional bookkeeping for T2: it is T2's *only* evidence that the two new assertions are live.

### Step 2 — GREEN, verbatim (all three sub-tests present and RUN)

```
=== RUN   TestListenerMetrics_GateMatchesInc
=== RUN   TestListenerMetrics_GateMatchesInc/mixed_rejected_at_build
=== RUN   TestListenerMetrics_GateMatchesInc/tls_listener
=== RUN   TestListenerMetrics_GateMatchesInc/plaintext_listener
--- PASS: TestListenerMetrics_GateMatchesInc (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/mixed_rejected_at_build (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/tls_listener (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/plaintext_listener (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.004s
```

Three `=== RUN` sub-test lines and three `--- PASS` sub-test lines — so this is **not** the `[no tests to run]` self-certifying green of RD-RUNSTALE. The selector `TestListenerMetrics_GateMatchesInc` was **not** touched by T1's rename (only `…ExactlyThreeSSLNames` → `…ExactlyFourSSLNames` was), so it needed no re-derivation, but it was confirmed live by the three `=== RUN` lines rather than assumed.

### Step 3 — hygiene, all silent

```
--gofmt--
--vet--
--lint--
ALL SILENT
```

`gofmt -l internal/listener` named nothing; `go vet ./internal/listener/...` and `golangci-lint run ./internal/listener/...` both exited 0 with no output. No gofmt trap here — the T2 edits are inside function bodies and comments, touching no alignment run.

### Break C — delete T1's `rt.sslNoCertificate = r.NewCounter(prefix + "ssl.no_certificate")`. **FIRED, and ONLY it.**

Run AFTER the commit, `-count=1`, `-run` isolated:

```
=== RUN   TestListenerMetrics_GateMatchesInc
=== RUN   TestListenerMetrics_GateMatchesInc/mixed_rejected_at_build
=== RUN   TestListenerMetrics_GateMatchesInc/tls_listener
    manager_test.go:2214: TLS listener: rt.sslNoCertificate is NIL — Inc would panic the serveConnection goroutine
=== RUN   TestListenerMetrics_GateMatchesInc/plaintext_listener
--- FAIL: TestListenerMetrics_GateMatchesInc (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/mixed_rejected_at_build (0.00s)
    --- FAIL: TestListenerMetrics_GateMatchesInc/tls_listener (0.00s)
    --- PASS: TestListenerMetrics_GateMatchesInc/plaintext_listener (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.005s
```

**WHICH assertion fired:** exactly one — `manager_test.go:2214`, T2's own non-nil pointer assertion, from sub-test `tls_listener`. Line `:2214` is the `t.Error` body of the `if rt.sslNoCertificate == nil` block added by this task. **Exactly one failure line in the entire run.**

**⚠️ THE SILENCE IS THE FINDING, and it is CONFIRMED BY EXECUTION — not predicted.** Under Break C the three build-time predicate assertions in the same `tls_listener` sub-test printed **NOTHING**:

| assertion | line | printed under Break C |
|---|---|---|
| `if !rt.tlsMode` | `:2183` | **NOTHING** |
| `if ci.tlsCfg == nil` (inside the `:2189` loop over `chainByName`) | `:2190-2191` | **NOTHING** |
| `if rt.defaultChain != nil && rt.defaultChain.tlsCfg == nil` | `:2194-2195` | **NOTHING** |

All three are set at BUILD time (`manager.go:692` / `:562`), entirely **upstream** of `registerListenerMetrics`, and are therefore structurally incapable of observing a registration bug. This reproduces phase-74's V1-M2 and re-confirms PLAN §Task-2's ⚠️ *"THE POINTER HALF IS THE LOAD-BEARING HALF"* **by execution at the IMPL, not by citation.** Had the pointer half been omitted, Break C would have gone entirely green.

**`plaintext_listener` correctly PASSED** under Break C: with the registration deleted, `rt.sslNoCertificate` is nil there too, which is exactly what the nil half asserts. The nil half is therefore **NOT** exercised by Break C — its own discriminating break would be an *ungated* registration (moving the line outside `if rt.tlsMode`), which is not in this row's break ledger. Recorded as a known, accepted gap rather than papered over: the `(c)` half's job is to catch a *separate* or *absent* gate, and the two exact-set name pins from T1 already fail loudly in that scenario.

`git -C /home/esa/git/envoy-go-wt-p75impl restore internal/listener/manager.go` ⇒ `git status --porcelain` **empty**; full package re-green confirmed `ok  github.com/pgdad/envoy-go/internal/listener  3.219s`.

### Task 2 — surprises

1. **Every one of the PLAN's five Task-2 anchors had drifted** (+6, uniformly) — a full sweep, not a single stale line. The `⚠️ Anchor drift already observed` warning carried into this task was correct and load-bearing; every anchor was re-grepped rather than trusted.
2. **No one-step red exists for this task**, and that is a property of T1's commit shape, not a defect. Recorded above in full rather than papered over with a synthetic failure.
3. **Break C's discrimination is one-sided** (fires the non-nil half, cannot fire the nil half). Recorded as a gap with its rationale, per the break protocol's "a break that does not fire is a FINDING".

## Task 3 — ACTUAL ⚠️ THE ROW'S LOAD-BEARING TASK

**Commit `9b63c5aa`** — *"phase 75 T3: the guarded Inc + assertSSLCrossProduct made VARIADIC + the roster extension (the row's PRIMARY guard) + the one-way-TLS positive arm"*. Two files: `internal/listener/manager.go` (+6/-0), `internal/listener/manager_test.go` (+155/-12); 161 insertions, 12 deletions. Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, base master `9f5d667b`, `git rev-list --count master..HEAD` = 4 before the commit. **No push.**

**PLAN §1.3 REPRODUCED IN FULL AT THE IMPL.** Both halves of the headline held: Break D fires the **phase-74** test and leaves the new positive arm PASSING; Break D′ leaves the **entire package GREEN**. The roster extension is confirmed by execution — at this tip, not by citation — as the row's sole guard on the pinned predicate.

### PLAN anchors — RE-DERIVED before use (the call-site trio had drifted +22)

T1 and T2 together shifted the whole `assertSSLCrossProduct` region. The PLAN's Task-3 numbers are pre-T1.

| PLAN anchor | PLAN claim (pre-T1/T2) | re-derived at the T3 start tip | verdict |
|---|---|---|---|
| call site — `TestServeConnection_SSLHandshakeIncrements` | `:4508` | **`:4530`** | **DRIFTED +22** |
| call site — `TestServeConnection_SSLFailVerifyErrorIncrements` | `:4539` | **`:4561`** | **DRIFTED +22** |
| call site — `TestServeConnection_SSLFailVerifyNoCertIncrements` | `:4557` | **`:4579`** | **DRIFTED +22** |
| helper doc — *"the other two are exactly 0"* | `:4466` | **`:4488`** | **DRIFTED +22** |
| test doc — *"neither failure counter moves"* | `:4491-4492` | **`:4513-4514`** | **DRIFTED +22** |
| `assertSSLCrossProduct` declaration | *(implied `:4474`)* | **`:4496`** | **DRIFTED +22** |
| `startMutualTLSListener` | `:4455-4484` region | **`:4459`** (declaration) | **DRIFTED +4** — different drift than the trio, because T1's edits sit between the two regions |
| the Inc site — `manager.go` | *"the guarded Inc after `:1277`"* | `rt.sslHandshake.Inc()` at **`:1284`**, `dispatchConn = tlsConn` at **`:1285`** | **DRIFTED +7** (T1 added 10 production lines above it) |
| `mkDownstreamTSInline` | `:627`, takes **`string`** PEMs | **`:627`**, `func mkDownstreamTSInline(t *testing.T, certPEM, keyPEM string)` | **HOLDS** — RD-CONV confirmed; the conversions are mandatory |
| `handshakeTestPKI` | `:4032-4038`, holds **`[]byte`** | **`:4054-4060`**; `caCertPEM, caKeyPEM []byte` / `serverCertPEM, serverKeyPEM []byte` | **DRIFTED +22**; the `[]byte` claim **HOLDS** |
| `tlsConn` scope | declared `manager.go:1259` in the block BODY, not the if-init | **`:1266`** — `tlsConn := stdtls.Server(pkConn, selected.tlsCfg)`, plain block statement | **DRIFTED +7**; the *in-scope, no-plumbing-owed* claim **HOLDS** |
| `pollCounter` / `counterValue` | `quic_test.go:34` / `:66` | **`:34`** / **`:66`** | **HOLDS** (RD-POLLFILE — they are NOT in `manager_test.go`) |
| `normalizeAddr` | *(used by the helper)* | `manager.go:352` | **HOLDS** |

**The three landed call sites changed by ZERO bytes — verified MECHANICALLY, not asserted.** `git diff HEAD~1 -- internal/listener/manager_test.go | grep -E '^[-+].*assertSSLCrossProduct\(t, reg, addr'` returns **exactly one line**, the new `+` for the positive arm. The trio appears in neither the `-` nor the `+` side. §2.1's variadic decision delivered what it promised.

Post-T3 anchors, for T4+ (**these WILL drift again**): `sslLeafRoster` `:4497` · `assertSSLCrossProduct` decl `:4527` · `TestServeConnection_SSLHandshakeIncrements` `:4565` (call `:4580`) · `…FailVerifyError…` `:4591` (call `:4611`) · `…FailVerifyNoCert…` `:4616` (call `:4629`) · `startOneWayTLSListener` `:4647` · `TestServeConnection_SSLNoCertificateIncrements` `:4695` (call `:4722`) · `manager.go` guard `:1288`, Inc `:1289`.

### Step 2 — RED, verbatim, and CONFIRMED to be the POSITIVE HALF

```
=== RUN   TestServeConnection_SSLNoCertificateIncrements
    manager_test.go:4722: listener.127_0_0_1_45945.ssl.no_certificate = 0, want 1
--- FAIL: TestServeConnection_SSLNoCertificateIncrements (3.01s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.016s
```

**WHICH assertion fired — three independent confirmations that it is the positive half and NOT a precondition `Fatalf`:**

1. **The message text is unique to the positive half.** `= 0, want 1` is emitted only by `t.Errorf("%s = %d, want 1", …)` inside `assertSSLCrossProduct`'s `wantSuffixes` loop. The negative half's format string carries the `— only %v may move on this arm` suffix; both preconditions are prefixed `precondition:`. Neither appears.
2. **The elapsed time is the poll budget.** `3.01s` is `pollCounter`'s `3*time.Second` timeout expiring — i.e. the counter was polled to exhaustion. A dial failure or a `HandshakeComplete == false` would have `Fatalf`'d in milliseconds.
3. **`manager_test.go:4722` is the CALL SITE, not the `Errorf` line** — the expected consequence of `t.Helper()` in `assertSSLCrossProduct`, which re-attributes to the caller. `:4722` is `assertSSLCrossProduct(t, reg, addr, "handshake", "no_certificate")`, the last statement of the test, reached only *after* both preconditions passed. **The listener shape is therefore correct and the handshake genuinely COMPLETED** — the certificate-less client succeeded against the one-way listener, exactly as `startOneWayTLSListener` intends.

**The three phase-74 arms were confirmed still PASSING at the RED tip** (the roster had already grown to four, so this is a real check, not a formality):

```
--- PASS: TestServeConnection_SSLHandshakeIncrements (0.01s)
--- PASS: TestServeConnection_SSLFailVerifyErrorIncrements (0.01s)
--- PASS: TestServeConnection_SSLFailVerifyNoCertIncrements (0.01s)
ok  	github.com/pgdad/envoy-go/internal/listener	0.026s
```

They hold `no_certificate` at 0 for the reasons the PLAN gives: the success arm presents a trusted client cert, and the two failure arms `return` in the error branch before the success fall-through.

### Step 4 — GREEN, verbatim (all four arms, then the package, then `-race`)

```
=== RUN   TestServeConnection_SSLHandshakeIncrements
--- PASS: TestServeConnection_SSLHandshakeIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyErrorIncrements
--- PASS: TestServeConnection_SSLFailVerifyErrorIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyNoCertIncrements
--- PASS: TestServeConnection_SSLFailVerifyNoCertIncrements (0.01s)
=== RUN   TestServeConnection_SSLNoCertificateIncrements
--- PASS: TestServeConnection_SSLNoCertificateIncrements (0.01s)
ok  	github.com/pgdad/envoy-go/internal/listener	0.031s
```

Four `=== RUN` lines and four `--- PASS` lines — **not** the RD-RUNSTALE `[no tests to run]` self-certifying green. (The two sub-packages *did* print `[no tests to run]` for this selector, which is correct: they contain no `TestServeConnection_SSL*` test. Recorded so the string is not later mistaken for a footgun hit.)

Full package, plain:
```
ok  	github.com/pgdad/envoy-go/internal/listener	3.231s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.042s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
```

Full package, **`-race`** (the new arm adds a per-connection goroutine reading `ConnectionState()`, so this is load-bearing, not ceremony):
```
ok  	github.com/pgdad/envoy-go/internal/listener	4.340s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	1.052s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	1.014s
```

**No known flake fired** — not the `internal/cluster` `-race` outlier (different package), not the startup flake. Nothing to classify.

### Step 5 — hygiene, all silent

```
--- gofmt ---
--- vet ---
--- lint ---
ALL CLEAN
```

`gofmt -l internal/listener` named nothing; `go vet ./internal/listener/...` and `golangci-lint run ./internal/listener/...` both exited 0 with no output. No gofmt trap fired here — hazard 6's two traps belong to T1's field block and T4's `helpText` entry, and T3 touches neither alignment run.

### Break D — delete the `len(…) == 0` wrapper so the Inc is UNCONDITIONAL. **FIRED — at the PHASE-74 TEST, exactly as PLAN §1.3 predicts and against what the SPEC and the router claim.**

Run AFTER the commit, `-count=1`, `-run 'TestServeConnection_SSL'`:

```
=== RUN   TestServeConnection_SSLHandshakeIncrements
    manager_test.go:4580: listener.127_0_0_1_38855.ssl.no_certificate = 1, want 0 — only [handshake] may move on this arm
--- FAIL: TestServeConnection_SSLHandshakeIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyErrorIncrements
--- PASS: TestServeConnection_SSLFailVerifyErrorIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyNoCertIncrements
--- PASS: TestServeConnection_SSLFailVerifyNoCertIncrements (0.01s)
=== RUN   TestServeConnection_SSLNoCertificateIncrements
--- PASS: TestServeConnection_SSLNoCertificateIncrements (0.01s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.031s
```

**WHICH assertion fired:** exactly one — `manager_test.go:4580`, the **`TestServeConnection_SSLHandshakeIncrements`** call site, i.e. `assertSSLCrossProduct`'s **NEGATIVE** half reading `no_certificate` on the phase-74 **mTLS SUCCESS** arm. **⚠️ `TestServeConnection_SSLNoCertificateIncrements` — phase 75's own positive arm, the test the SPEC §10.1 and the router both name as this row's discriminating break — PASSED.** The mechanism is the one §1.3 states: an unconditional Inc still leaves `no_certificate == 1` on the arm that *wants* 1, so the positive half is structurally blind to over-firing (`reference_positive_arm_cannot_catch_overfiring`).

The two failure arms also passed, correctly: they `return` in the handshake-error branch and never reach the Inc, conditional or not.

### Break D′ — Break D **plus** `sslLeafRoster` reverted to the three phase-74 leaves. **DID NOT FIRE — and THAT IS THE FINDING.**

Full package, `-count=1`:

```
ok  	github.com/pgdad/envoy-go/internal/listener	3.235s
```

**THE ENTIRE PACKAGE IS GREEN WITH THE ROW'S CENTRAL PINNED DECISION BROKEN.** A declared MUST-STAY-GREEN break, reproduced at the IMPL tip on the committed tree. Two consequences, both recorded rather than inferred:

- **The one-line roster extension is the SOLE guard on the pinned predicate.** Had T3 shipped the positive arm and the variadic helper but left the roster at three, the row would have landed undefended *and green* — no test in the repo would have noticed an unconditional Inc.
- **T1's two exact-set spelling pins do NOT cover it, and this run proves it.** They stayed green throughout D′ because they pin *registration*, never *increment* — the counter is registered under the right name either way. The standing invariant (bare leaves on the roster, fully-qualified literals in the spelling pins) is therefore not merely stylistic: the two guards genuinely cover disjoint defects.

`git -C /home/esa/git/envoy-go-wt-p75impl restore internal/listener/manager.go internal/listener/manager_test.go` ⇒ `git status --porcelain` **empty**.

### Break E — delete ONLY the guarded Inc block, keep the registration. **FIRED at the positive arm, and ONLY it.**

`-run 'TestServeConnection_SSL'`:

```
=== RUN   TestServeConnection_SSLHandshakeIncrements
--- PASS: TestServeConnection_SSLHandshakeIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyErrorIncrements
--- PASS: TestServeConnection_SSLFailVerifyErrorIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyNoCertIncrements
--- PASS: TestServeConnection_SSLFailVerifyNoCertIncrements (0.01s)
=== RUN   TestServeConnection_SSLNoCertificateIncrements
    manager_test.go:4722: listener.127_0_0_1_39461.ssl.no_certificate = 0, want 1
--- FAIL: TestServeConnection_SSLNoCertificateIncrements (3.01s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.036s
```

**The "and nothing else" clause was VERIFIED, not assumed** — the FULL package was also run under Break E and produced exactly one failing test:

```
--- FAIL: TestServeConnection_SSLNoCertificateIncrements (3.01s)
    manager_test.go:4722: listener.127_0_0_1_42365.ssl.no_certificate = 0, want 1
FAIL	github.com/pgdad/envoy-go/internal/listener	6.229s
```

**⚠️ No crash, no SIGSEGV.** This is Break E, not Break B: the *registration* survives, so `rt.sslNoCertificate` stays non-nil and the deleted Inc is a silent no-op — the "registered but never Inc'd" counterfactual in its pure form. Restored; `git status --porcelain` **empty**.

### Break F — add the REFUTED client-auth mode term. **FIRED at the positive arm.**

Edit (one line, compiled first try — `selected.tlsCfg` is already the `*stdtls.Config` two lines above, and `stdtls` is already imported at `manager.go:5`, so no new import and no substitution was needed):

```go
		if selected.tlsCfg.ClientAuth != stdtls.NoClientCert && len(tlsConn.ConnectionState().PeerCertificates) == 0 {
```

```
=== RUN   TestServeConnection_SSLHandshakeIncrements
--- PASS: TestServeConnection_SSLHandshakeIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyErrorIncrements
--- PASS: TestServeConnection_SSLFailVerifyErrorIncrements (0.01s)
=== RUN   TestServeConnection_SSLFailVerifyNoCertIncrements
--- PASS: TestServeConnection_SSLFailVerifyNoCertIncrements (0.01s)
=== RUN   TestServeConnection_SSLNoCertificateIncrements
    manager_test.go:4722: listener.127_0_0_1_33689.ssl.no_certificate = 0, want 1
--- FAIL: TestServeConnection_SSLNoCertificateIncrements (3.01s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.036s
```

**This is the demonstration that the PINNED unconditional predicate is load-bearing.** The one-way listener built by `startOneWayTLSListener` carries no validation context and no `require_client_certificate`, so its `tlsCfg.ClientAuth` is `NoClientCert` and the added term short-circuits the Inc away. A mode-gated predicate would under-count against the reference on **every** one-way-TLS listener — which is precisely the deployment shape this row exists to annotate.

Restored; `git status --porcelain` **empty**; full package re-green confirmed `ok  github.com/pgdad/envoy-go/internal/listener  3.227s`.

### ⚠️ E and F are NOT separately discriminated — recorded honestly, per PLAN §1.3

**Breaks E and F fired the IDENTICAL assertion at the IDENTICAL line: `manager_test.go:4722`, `ssl.no_certificate = 0, want 1`.** Byte-for-byte the same message modulo the ephemeral port. They are distinguishable **by the EDIT, not by the OUTPUT**. Both are the same defect class — *the counter does not move when it must* — so this is acceptable, and it is stated here rather than smoothed into two independent-looking proofs. Anyone reading only the two output blocks cannot tell which break produced which, and **this record does not claim otherwise.**

### The FOUR breaks, as one table

| break | edit | predicted | ACTUAL | which test fired |
|---|---|---|---|---|
| **D** | delete the `len(…)==0` wrapper ⇒ unconditional Inc | phase-74 arm, negative half; positive arm PASSES | **as predicted** | `TestServeConnection_SSLHandshakeIncrements` `:4580` |
| **D′** | D + roster reverted to three leaves | **FULL package GREEN** | **as predicted — `ok … 3.235s`** | *(none — that is the finding)* |
| **E** | delete only the guarded Inc block | positive arm, `= 0, want 1`, nothing else | **as predicted; "nothing else" verified full-package** | `TestServeConnection_SSLNoCertificateIncrements` `:4722` |
| **F** | add `ClientAuth != NoClientCert &&` | positive arm, same message | **as predicted** | `TestServeConnection_SSLNoCertificateIncrements` `:4722` |

**All four reproduced the PLAN's predictions exactly. Zero PLAN break predictions were refuted at T3.**

### Task 3 — surprises

1. **The anchor drift was NOT uniform.** T2's drift was a clean +6 across all five anchors; T3's was **+22 for the `assertSSLCrossProduct` region, +4 for `startMutualTLSListener`, and +7 in `manager.go`** — because T1's and T2's edits are interleaved *between* the regions. A single "add N" correction would have mis-targeted two of the three groups. Every anchor was re-grepped individually.
2. **The zero-byte call-site claim was verifiable mechanically**, and was verified that way rather than by eye: the `git diff | grep` over the call-site pattern returns exactly one line, the new one.
3. **PLAN §1.3's headline reproduced with no deviation at all** — including the counter-intuitive half, that phase 75's own positive arm PASSES under the break the SPEC named it as guarding against. Nothing here refutes the PLAN; the PLAN's own refutation of the SPEC and the router is what held.
4. **No new hazards surfaced.** RD-CONV, RD-POLLFILE, RD-NOOUTCOME and the no-nil-guard constraint were all obeyed as written and none proved wrong; `handshakeOutcome` and `classifyHandshakeErr` are **byte-untouched** by this task.

## Task 4 — ACTUAL

**Commit `621a899e`** — *"phase 75 T4: the envoy_listener_ssl_no_certificate helpText entry + wantNames (RED-first) + both stale count claims in the map's doc"*. Files: `internal/stats/name.go` (+9/-4), `internal/stats/name_test.go` (+7/-0) — 2 files, +16/-4. Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, base master `9f5d667b`. **No push.** ⚠️ **This task touched `internal/stats/` ONLY** — zero bytes in `internal/listener/`.

### PLAN anchors — RE-DERIVED before use (T1–T3 shifted `internal/listener` by +4…+22; `internal/stats` was untouched, and that HELD — with ONE exception)

| PLAN anchor | claim | RE-DERIVED at the T3 tip | verdict |
|---|---|---|---|
| `name.go:445-455` | the `helpText` doc comment | `:445-455` — `// helpText maps a Prometheus name…` through `…so every emitted name wants an entry.` | **HOLDS exactly** |
| `name.go:448` | *"Of the **14** entries"* | `:448` — `// only. Of the 14 entries, the first 10 cover the 13 unique Prometheus names` | **HOLDS** |
| `name.go:451-452` | *"the last **three** are the phase-74 …"* | `:451-452` — `// Rule SN4); one is an 06.2 backpressure counter; and the last three are the` / `// phase-74 listener-scope TLS handshake outcomes, whose three-dot source names` | **HOLDS** |
| `name.go:471` (insert after) | `fail_verify_no_cert` is the last entry, `:472` is `}` | `:471` = `"envoy_listener_ssl_fail_verify_no_cert": …`, `:472` = `}` | **HOLDS** |
| `name_test.go:231-235` | `wantNames` | the slice literal is **`:230-234`** (`wantNames := []string{` at `:230`, `}` at `:234`); the three string elements are `:231-233`; `:235` is the `for` line | ⚠️ **OFF BY ONE** — the PLAN's span covers the three elements plus the `for`, not the literal. Harmless; recorded because the next row should not re-inherit it. |
| `name_test.go:222-223` | *"its doc"* | ⚠️ **REFUTED AS A SPAN.** `wantNames` has **no doc of its own**; `:222-223` is the middle of the **8-line function doc for `TestHelpText_ListenerSSLHandshakeOutcomes` (`:221-228`)**, and the PLAN's replacement text is **6 lines**, so it is neither the whole comment nor a clean sub-span. | **See the decision below.** |

**⚠️ The `:222-223` decision, stated rather than smoothed.** The PLAN's verbatim block was **APPENDED as a final paragraph** of the function doc comment (after `// self-equality is the degradation signature asserted below.`, separated by a bare `//`), **not substituted for `:222-223`**. Substituting would have destroyed the phase-74 rationale — the SN3 flattening explanation and the HELP-degradation signature — which is still true and still the reason that test exists. The block is present **byte-for-byte as the PLAN wrote it**. Anyone diffing the PLAN against the tree will find the text where the PLAN said the text should go, but **added, not swapped**; that is a deliberate departure and this record does not pretend the anchor was correct.

### Pre-flight — the two claims the PLAN said were guarded by NOTHING, re-verified at the IMPL tip

```
$ grep -rn 'len(helpText)' --include='*.go' .
$ echo "exit=$?"
exit=1
```
**Zero hits — RD-HELPDOC's headline HOLDS.** The "14 entries" claim is enforced by no test, no assertion, no vet check. It is prose with the authority of proximity.

```
$ awk '/^var helpText = map\[string\]string\{/,/^\}/' internal/stats/name.go | grep -c '^\t"'
14
```
**RD-HELPTEXT's count of 14 CONFIRMED mechanically at the IMPL tip** (the PLAN had verified it twice — statically and by `len()` under `-overlay`). After the GREEN step the same command returns **15**, which is the new doc's claim, likewise unguarded.

### Step 1–2 — RED, verbatim, and for the RIGHT reason

`wantNames` extended to four FIRST (element 4 appended), entry NOT yet added:

```
$ go test ./internal/stats/ -count=1 -v -run 'TestHelpText'
=== RUN   TestHelpText_Coverage
--- PASS: TestHelpText_Coverage (0.00s)
=== RUN   TestHelpText_AccessLogDropped
--- PASS: TestHelpText_AccessLogDropped (0.00s)
=== RUN   TestHelpText_ListenerSSLHandshakeOutcomes
    name_test.go:247: helpText missing entry for "envoy_listener_ssl_no_certificate"
--- FAIL: TestHelpText_ListenerSSLHandshakeOutcomes (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/stats	0.001s
```

**Exactly the message the PLAN predicted, at `name_test.go:247`** — the `!ok` arm, followed by `continue`, so the `got == ""` and `got == n` assertions were correctly **skipped** for the missing key (they are not dead code; they simply had nothing to inspect). This compiled and RAN — it is not a build failure masquerading as red.

### ⚠️ The `TestHelpText_Coverage` ASYMMETRY — the point of the whole task, observed live

**In the same run that failed, `TestHelpText_Coverage` PASSED.** It PASSES because its own hand-listed roster stops at `envoy_server_live` — it names **none** of the four `envoy_listener_ssl_*` keys, not the three phase-74 ones and not phase 75's. Its shape is forward-only in exactly the same way: `for _, n := range wantNames { if _, ok := helpText[n]; !ok { … } }`. **Neither helpText test walks `helpText` in the reverse direction.** A key added to the map is guarded by precisely one thing — somebody remembering to hand-add its name to a slice. That is the finding the doc block now records in the tree, and Break G below is its executable proof.

### Step 3–4 — GREEN, verbatim (the sub-selector, then the FULL package)

```
$ go test ./internal/stats/... -count=1 -v -run 'TestHelpText'
--- PASS: TestHelpText_Coverage (0.00s)
--- PASS: TestHelpText_AccessLogDropped (0.00s)
--- PASS: TestHelpText_ListenerSSLHandshakeOutcomes (0.00s)
ok  	github.com/pgdad/envoy-go/internal/stats	0.001s

$ go test ./internal/stats/... -count=1
ok  	github.com/pgdad/envoy-go/internal/stats	0.003s
ok  	github.com/pgdad/envoy-go/internal/stats/dynamic	0.013s
```

⚠️ **`internal/stats/dynamic` printed `[no tests to run]` under the `-run` selector and `ok` under the full run** — the `reference_differential_run_selector` footgun in its benign form. The full-package run above is what discharges it; the `-run` line alone would have been consistent with the package having no tests at all.

### Step 4 — hygiene, all silent

```
$ gofmt -l internal/stats     → (no output)
$ go vet ./internal/stats/... → (no output)
$ golangci-lint run ./internal/stats/... → (no output)
```

### ⚠️ The BLANK-SEPARATOR / NO-REPAD PROOF — verified by `git diff`, not by gofmt's silence alone

`gofmt -l` staying silent proves the file is *formatted*; it does **not** by itself prove the three phase-74 lines survived unchanged (a re-padded file is also perfectly gofmt-clean). The load-bearing evidence is the diff, where all three phase-74 entries appear as **pure CONTEXT lines** — leading space, no `+`, no `-`:

```
@@ -469,4 +471,6 @@ var helpText = map[string]string{
 	"envoy_listener_ssl_handshake":           "Total successful downstream TLS handshakes on the listener.",
 	"envoy_listener_ssl_fail_verify_error":   "Downstream TLS handshakes failed because client certificate chain verification failed.",
 	"envoy_listener_ssl_fail_verify_no_cert": "Downstream TLS handshakes failed because no client certificate was presented where one was required.",
+
+	"envoy_listener_ssl_no_certificate": "Successful downstream TLS handshakes in which the client presented no certificate.",
 }
```

**Three context lines, byte-identical, and the added key is left-aligned at ONE space** — the blank separator line ended the alignment run, so gofmt never considered padding the four keys to the 38-character `fail_verify_no_cert` width. Cross-cutting hazard 6's second trap **did not fire, because the countermeasure was applied**. The whole `name.go` hunk set is exactly two hunks: the doc paragraph and this one. **Zero phase-74 bytes changed in this commit.**

### The doc comment — BOTH count claims corrected, per RD-HELPDOC

`14` → `15` at `:448` was the obvious half. The **non-obvious** half is `:451-452`: *"the last three are the phase-74 listener-scope TLS handshake outcomes"* became false in a way a count-bump would not have caught — the last **four** entries are the `ssl.*` family but only three of them are phase-74, and `ssl.no_certificate` is a **SUCCESS-PATH ANNOTATION, not a member of the outcome trichotomy** (it co-occurs with `ssl.handshake`; it does not compete with it). The replacement paragraph splits the sentence into *"the next three are the phase-74 … outcomes; and the last is phase 75's … annotation, not a member of the outcome trichotomy"*, and re-scopes the SN3 sentence from *"whose three-dot source names"* (which bound only to the phase-74 three) to *"All four ssl.* entries have three-dot source names"*, with `<outcome>` widened to `<leaf>`. **A reviewer who fixed only the `14` would have left a live falsehood two lines below it.**

### Break G — delete the `helpText` entry, revert `wantNames` to three. **MUST STAY GREEN — and it DID. This is the row's silent-staleness finding.**

Both edits reverted to the pre-task state (`git diff --stat`: `name.go | 2 --`, `name_test.go | 1 -`), run AFTER committing, `-count=1`:

```
$ go test ./internal/stats/ -count=1
ok  	github.com/pgdad/envoy-go/internal/stats	0.003s
```

**FULL package green. Not one assertion anywhere in `internal/stats` noticed that a `helpText` entry had been deleted.** This is the executed proof of PLAN §Task-4's headline and of the doc block now landed in `name_test.go`: with the entry present and the slice at three the package is green (the PLAN's direction), and with the entry ABSENT and the slice at three the package is *also* green (this direction). **The map's contents are unguarded in both directions simultaneously.** `prom.go`'s fallback means the only user-visible consequence is `# HELP envoy_listener_ssl_no_certificate envoy_listener_ssl_no_certificate` — a silent HELP degradation that no test in this repo would report.

### Break G′ — entry still deleted, `wantNames` restored to four. **FIRED.**

```
$ go test ./internal/stats/ -count=1 -v -run 'TestHelpText'
=== RUN   TestHelpText_Coverage
--- PASS: TestHelpText_Coverage (0.00s)
=== RUN   TestHelpText_AccessLogDropped
--- PASS: TestHelpText_AccessLogDropped (0.00s)
=== RUN   TestHelpText_ListenerSSLHandshakeOutcomes
    name_test.go:247: helpText missing entry for "envoy_listener_ssl_no_certificate"
--- FAIL: TestHelpText_ListenerSSLHandshakeOutcomes (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/stats	0.001s
```

**The predicted assertion, at the predicted line `name_test.go:247`, in the predicted test** — and `TestHelpText_Coverage` PASSED alongside it once more, re-confirming the asymmetry under the broken tree as well as the healthy one. G′ is what converts G from *"nothing guards this"* into *"nothing guards this, and here is the single line that would have"*.

### Break G″ — `wantNames` at four, entry RESTORED. **GREEN.**

```
$ git status --porcelain   → (empty)
$ go test ./internal/stats/ -count=1 -v -run 'TestHelpText'
--- PASS: TestHelpText_Coverage (0.00s)
--- PASS: TestHelpText_AccessLogDropped (0.00s)
--- PASS: TestHelpText_ListenerSSLHandshakeOutcomes (0.00s)
ok  	github.com/pgdad/envoy-go/internal/stats	0.002s
```

⚠️ **G″ is the leg that makes G's green MEAN something** (`reference_liveness_break_needs_failing_baseline`). Without it, G's `ok` is consistent with two incompatible stories — *"the test ran and correctly had nothing to say"* and *"the test never ran at all"*. G′ shows the test CAN fail on this exact key; G″ shows it PASSES on this exact key when the key is present. Only with both pinned does G's green isolate to the one story that matters: **the test ran, it passed, and it passed because it was never asked about the deleted key.** A two-leg stack (G + G′ only) would have been a weaker proof dressed as a complete one.

### The THREE breaks, as one table

| break | edit | predicted | ACTUAL | which assertion |
|---|---|---|---|---|
| **G** | delete `helpText` entry + revert `wantNames` to three | **FULL package GREEN** | **as predicted — `ok … 0.003s`** | *(none — that is the finding)* |
| **G′** | entry still deleted, `wantNames` at four | `helpText missing entry for "envoy_listener_ssl_no_certificate"` | **as predicted, verbatim** | `TestHelpText_ListenerSSLHandshakeOutcomes` `name_test.go:247` |
| **G″** | `wantNames` at four, entry RESTORED | GREEN | **as predicted — `ok … 0.002s`** | all three `TestHelpText*` PASS |

`git restore` after each; **`git status --porcelain` empty and verified** before leaving the break sequence. **All three reproduced the PLAN's predictions exactly. Zero PLAN break predictions were refuted at T4.**

### Task 4 — surprises

1. **The `internal/stats` anchor forecast was RIGHT in substance and WRONG in one detail.** The task brief predicted the PLAN's `internal/stats` anchors would hold because T1–T3 never touched the package — and the four `name.go` anchors held to the line. But **two `name_test.go` anchors were already wrong IN THE PLAN, independent of any drift**: `:231-235` is off by one against the slice literal, and `:222-223` names a span that does not correspond to any coherent unit. **Anchor error is not only a drift phenomenon** — re-deriving caught a PLAN-time mistake that no amount of "+N" correction would have fixed, which is precisely why the instruction was to re-grep rather than to offset.
2. **The two count claims are BOTH still unguarded after this commit, by design.** `15` and *"the last is phase 75's"* are enforced by nothing; `grep -rn 'len(helpText)'` still returns 0. This task corrected the prose; it did not add the reverse-direction test that would prevent the next recurrence. **That is a deliberate scope boundary, not an oversight** — the landed `name_test.go` doc block is the mitigation the row chose, and it is a *convention*, not an *assertion*. A future row that wants a real guard should walk `helpText` and require every `envoy_listener_ssl_*` key to appear in the roster.
3. **`gofmt -l` silence was NOT accepted as the no-repad proof.** A re-padded map is gofmt-clean too, so the silence and the diff answer different questions. Both were run; the diff is the one recorded above. Had only `gofmt -l` been run, three phase-74 lines could have been rewritten in this commit with nothing to show for it.
4. **The `dynamic` sub-package's `[no tests to run]` under the `-run` selector** is a live instance of `reference_differential_run_selector` outside the differential suite. It exits 0 and prints `ok`. It was discharged by the unfiltered full-package run, not reasoned away.
5. **`internal/listener` is BYTE-UNTOUCHED by this commit**, as the task required — the two-file diffstat is the evidence.

---

## Task 5 — ACTUAL ⚠️ THE ROW'S FIRST CROSS-SIDE MEASUREMENT, AGAINST A LIVE ENVOY REFERENCE

**Commit `11e9e89d`** — *"phase 75 T5: 0110 gains a cross-side StatsAsserter — the FIRST cross-side assertion of envoy_listener_ssl_no_certificate (ref and subj agree exactly)"*. Files: `test/fixtures/0110-tls-require-client-cert-false/driver/driver.go` (+231/-1) — **1 file**. Worktree `/home/esa/git/envoy-go-wt-p75impl`, branch `phase-75-impl`, base master `9f5d667b`. **No push.** **Fixtures 119 → 119** (`0110` EXTENDED; no new fixture). Reference image `envoyproxy/envoy@sha256:7edd5b0f…` (contrib-v1.37.2), Docker Desktop 28.1.1.

### PLAN anchors — RE-DERIVED before use. ⚠️ **ZERO DRIFT AT T5, and that is itself the finding.**

T1–T4 moved `internal/listener` anchors by +4…+22 and mis-stated two `internal/stats` test anchors at PLAN time. **`test/fixtures/` was byte-untouched by this row before T5, and every anchor in it held to the line.** Re-derived anyway, not assumed:

| PLAN anchor | claim | RE-DERIVED at the T4 tip | verdict |
|---|---|---|---|
| `0111/driver.go:655` | `func (d *edfDriver) AssertStats(…)` | `:655` exactly | **HOLDS** |
| `0111/driver.go:739` | `func scrapeProm(…)` | `:739` exactly | **HOLDS** |
| `0111/driver.go:672-674` | the decode-ran precondition | `:672-674` — `if ref["envoy_listener_downstream_cx_total"] == 0 {` … `}` | **HOLDS** |
| `0111/driver.go:682-684` / `:687-691` / `:692-696` | the THREE rosters | `log.Printf` `:682-684`, `names` `:687-691`, `want` `:692-696` | **HOLDS, all three** |
| `0111/driver.go:698-713` | the ABSENT/value split | lookups `:698-699`, `!refOK` `:706-709`, `!subjOK` `:710-713` | **HOLDS** |
| `0111/driver.go:725-727` | the labelled-redundant cross-side tripwire | `:725-727` | **HOLDS** |
| `0110/driver.go:39` | `refListenerPort = 10446` (**RD-0110-PORT**) | `:39` — `refListenerPort = 10446` | **HOLDS — and the SPEC's `10447` is confirmed to be `0111`'s port** |
| `0110/driver.go:613` | `var _ fixture.Driver = (*rccfDriver)(nil)`, **exactly ONE `var _` line**, file **613** lines | `:613`, one `var _` line, 613 lines | **HOLDS exactly** |
| `0110/` collision sweep | ZERO `scrapeProm` / `AssertStats` / `StatsAsserter` / `prometheus` in the DRIVER | driver.go: 0 hits. (3 prose hits exist in `README.md:160` and `expectations.yaml:167,175` — **T6's remit**, not a collision.) | **HOLDS** |
| `runner_test.go:1347-1349` (**RD-DISPATCH**) | silent type assertion, no `else`/log/skip | `:1347` `if sa, ok := d.(fixture.StatsAsserter); ok {` · `:1348` `sa.AssertStats(t, ref.AdminAddr(), subj.AdminAddr())` · `:1349` `}` | **HOLDS exactly** |
| **RD-TRIPWIRE** | `var _ fixture.StatsAsserter` appears in exactly **2** fixture files | `0076/driver/driver_test.go`, `0111/driver/driver.go` ⇒ **2**. `0110` is now the **third**. | **HOLDS** |

**Post-T5 line numbers for the next task:** banner `:616` · `AssertStats` **`:655`** *(coincidentally the SAME line as `0111`'s)* · `scrapeProm` **`:782`** · `var _ fixture.Driver` `:842` · `var _ fixture.StatsAsserter` `:843` · file **843** lines.

### Step 2 — the run, against a LIVE reference container. **PASS on the first run.**

```
$ go test ./test/differential/ -count=1 -v -run 'TestDifferential/0110-tls-require-client-cert-false'
2026/07/25 16:13:16 listener "l_rccf": handshake: tls: failed to verify certificate: x509: certificate signed by unknown authority
2026/07/25 16:13:16 0110 AssertStats: reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
2026/07/25 16:13:16 0110 AssertStats: subject   ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
--- PASS: TestDifferential/0110-tls-require-client-cert-false (1.92s)
ok  	github.com/pgdad/envoy-go/test/differential	1.988s
```

**Exact cross-side agreement on all five values, reproducing the PLAN's executed figures byte-for-byte.** The discriminating asymmetry is REAL and observed on BOTH sides: `handshake=2` against `no_certificate=1`. Arms 1 and 3 both complete a handshake and both ACCEPT — the byte observable cannot separate them — while only arm 3 presents no certificate. **This is the counter layer proving something the accept/reject contrast structurally cannot.**

### ⚠️ The LITERAL WIRE LINES — captured by a temporary instrumented scrape, then `git restore`d

The committed asserter keys on the metric NAME, so the label is never quoted in its own output. The lines below were obtained by a throwaway raw-`/stats/prometheus` dump inserted into `AssertStats`, run once, and reverted (`git status --porcelain` verified empty afterward). **This is measured output, not the PLAN's figures re-quoted.**

```
WIRE reference | envoy_listener_downstream_cx_total{envoy_listener_address="0.0.0.0_10446"} 3
WIRE reference | envoy_listener_ssl_handshake{envoy_listener_address="0.0.0.0_10446"} 2
WIRE reference | envoy_listener_ssl_no_certificate{envoy_listener_address="0.0.0.0_10446"} 1
WIRE reference | envoy_listener_ssl_fail_verify_error{envoy_listener_address="0.0.0.0_10446"} 1
WIRE reference | envoy_listener_ssl_fail_verify_no_cert{envoy_listener_address="0.0.0.0_10446"} 0

WIRE subject   | envoy_listener_downstream_cx_total{envoy_listener_address="___20016"} 3
WIRE subject   | envoy_listener_ssl_handshake{envoy_listener_address="___20016"} 2
WIRE subject   | envoy_listener_ssl_no_certificate{envoy_listener_address="___20016"} 1
WIRE subject   | envoy_listener_ssl_fail_verify_error{envoy_listener_address="___20016"} 1
WIRE subject   | envoy_listener_ssl_fail_verify_no_cert{envoy_listener_address="___20016"} 0
```

**⚠️ The subject's label is `___20016`, NOT `0_0_0_0_20016` — CONFIRMED LIVE.** envoy-go binds the IPv6 wildcard; `normalizeAddr` strips the brackets and maps both `:` and `.` to `_`, so `[::]:20016` → `___20016`. **The metric NAMES are byte-identical across the two sides and only the label differs** — which is exactly what makes a name-keyed, label-ignoring assertion cross-side viable. The landed keying comment states this and carries both wire lines inline.

**A second observation the dump makes, recorded because it bears on future rows:** the reference emits the **full** `ssl.*` family on this listener (`ciphers`, `curves`, `sigalgs`, `versions`, `session_reused`, `connection_error`, `fail_verify_cert_hash`, `fail_verify_san`, `was_key_usage_invalid`, the four `ocsp_staple_*`, and two `certificate_expiration_unix_time_seconds` series), while the subject emits **exactly the four asserted names plus `downstream_cx_total`**. The asserted subset is precisely the cross-side intersection this row can pin; everything else in the reference's family is a name envoy-go does not emit at all. **This is why the assertion is a NAMED SUBSET and not an exact-set comparison** — an exact-set pin would fail on the reference's 15 extra series and would be asserting the framework gap, not the counter.

### Step 3 — gates, all silent

```
$ gofmt -l .                                             (silent)
$ go vet ./...                                           (silent)
$ golangci-lint run ./test/...                           (silent)
$ go mod tidy -diff ; echo rc=$?                         rc=0   (EMPTY)
$ git diff master -- go.mod go.sum | wc -l               0      (EMPTY)
$ ls -d test/fixtures/[0-9]*/ | wc -l                    119
```

**+0 fixtures, +0 modules, +0 production imports.** The four new imports (`log`, `math`, `net/http`, `strconv`) are TEST-side, which the PLAN permits explicitly.

### Break H — `want["envoy_listener_ssl_no_certificate"]` 1 → 2. **FIRED ON BOTH SIDES, at the value check.**

```
    runner_test.go:1348: ref envoy_listener_ssl_no_certificate = 1, want 2
    runner_test.go:1348: subj envoy_listener_ssl_no_certificate = 1, want 2
--- FAIL: TestDifferential/0110-tls-require-client-cert-false (1.87s)
```
Exactly the PLAN's prediction, verbatim. ⚠️ **The attribution line is `runner_test.go:1348` — the DISPATCH call site, not `driver.go`** — because `AssertStats` opens with `t.Helper()`. That is correct behaviour and not a lost anchor, but a reader hunting `driver.go:NNN` in the failure output will not find it. Recorded so the next row does not misread it as the runner failing.

### Break I-a — keep the two-stage form, DELETE the production registration of `ssl.fail_verify_no_cert` (`manager.go:387`). **FIRED, and ONLY it.**

```
    runner_test.go:1348: subj: envoy_listener_ssl_fail_verify_no_cert ABSENT from /stats/prometheus
--- FAIL: TestDifferential/0110-tls-require-client-cert-false (2.09s)
```
**The exact message the PLAN demanded.** ⚠️ Note what did **NOT** happen: **no SIGSEGV.** The run completed all three arms, passed `structuralCheck` and `CompareBytes`, and reached step 10 — **which is the direct confirmation of F4's mechanism.** `ssl.fail_verify_no_cert` is registered on this fixture but `Inc`'d on **none** of its three arms (arm 2 books `fail_verify_error`), so deleting its registration is *silent* rather than fatal. That is precisely why it — and **not** `ssl.no_certificate` — is the counter the ABSENT check genuinely protects here. The landed comment names it.

### Break I-b — same deletion, asserter SIMPLIFIED to a single-value lookup (comma-ok + `continue` dropped). **PASSED VACUOUSLY — and THAT is the whole point.**

```
2026/07/25 16:14:39 0110 AssertStats: reference … ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
2026/07/25 16:14:39 0110 AssertStats: subject   … ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
--- PASS: TestDifferential/0110-tls-require-client-cert-false (1.97s)
ok  	github.com/pgdad/envoy-go/test/differential	2.044s
```
**GREEN with the counter not registered at all on the subject**, and the diagnostic log still cheerfully printing `ssl.fail_verify_no_cert=0` — a map read of an absent key. `gofmt` and `go vet` were both silent on the simplified form. **I-a alone would have proved only that the check FIRES; I-b is what proves it was NECESSARY.** A single-value lookup would have shipped this row with a deletable counter reading `0 == 0` forever.

### Break J — dispatch liveness, STACKED (the naive form is vacuous). **BOTH LEGS AS PREDICTED, plus the tripwire confirmed.**

**Leg 1 — set `want["envoy_listener_ssl_handshake"] = 7`, everything else intact. RED:**
```
    runner_test.go:1348: ref envoy_listener_ssl_handshake = 2, want 7
    runner_test.go:1348: subj envoy_listener_ssl_handshake = 2, want 7
--- FAIL: TestDifferential/0110-tls-require-client-cert-false (2.06s)
```

**Leg 1b — the tripwire, before disarming it. Rename `AssertStats` → `AssertStatsX` with the `var _` line KEPT ⇒ COMPILE ERROR:**
```
vet: test/fixtures/0110-…/driver/driver.go:843:31: cannot use (*rccfDriver)(nil) (value of type
*rccfDriver) as fixture.StatsAsserter value in variable declaration: *rccfDriver does not
implement fixture.StatsAsserter (missing method AssertStats)
```
**That is the tripwire working** — it converts a silent, permanent loss of the entire stats leg into a build failure at the exact line.

**Leg 2 — the wrong `want` STILL in place, rename `AssertStats` → `AssertStatsX`, `var _ fixture.StatsAsserter` DELETED. GREEN, and the log lines VANISH:**
```
--- PASS: TestDifferential/0110-tls-require-client-cert-false (1.92s)
ok  	github.com/pgdad/envoy-go/test/differential	1.992s
```
`grep '0110 AssertStats'` over the full verbose output ⇒ **zero lines.** The identical tree that was RED one edit earlier is now GREEN **solely because the method no longer satisfies the interface** — the value pin is still wrong and nothing notices. **This is the stacked proof: green-after-rename is only evidence of dispatch when the pre-rename state was RED for a reason the rename cannot fix.**

### ⚠️ Break E (the fast-failure arm) — **UNREACHABLE on `0110`. A FINDING, not a gap (F3). NOT attempted, per the PLAN.**

`0110`'s `wantObservable` requires arms 1 and 3 to ECHO through the `tcp_proxy`. Any dead or refused upstream therefore breaks the observable and the run dies at `structuralCheck` (`runner_test.go:1274`, `subj drive:`) at **step 8** — two steps before `AssertStats` is dispatched at step 10. **`structuralCheck` OUTRANKS both of the asserter's preconditions on this fixture**, so the reference-side `ssl.*`-suppression hazard that PRECONDITION 2 defends against cannot be provoked here without neutralising `structuralCheck`, which is not a legitimate configuration. The PLAN reached it only by that neutralisation and recorded the numbers; **this task spent no cycle re-attempting it, by instruction.** The precondition is nonetheless retained: a cross-side fixture is only as strong as its weaker side, and the guard's real earnings are (a) stopping the `want: 0` row from passing vacuously and (b) turning three cryptic value mismatches into one named diagnosis. **The landed comment says exactly that, and says which side it defends against.**

### The FOUR breaks, as one table

| break | edit | predicted | ACTUAL | which assertion |
|---|---|---|---|---|
| **H** | `want[no_certificate]` 1 → 2 | fires on BOTH sides at the value check | **as predicted, verbatim** | `ref … = 1, want 2` **and** `subj … = 1, want 2` |
| **I-a** | delete `ssl.fail_verify_no_cert` registration, two-stage form kept | `subj: … ABSENT from /stats/prometheus` | **as predicted, verbatim; NO SIGSEGV — F4's mechanism confirmed** | the `!subjOK` ABSENT branch |
| **I-b** | same deletion + single-value lookup | **PASSES VACUOUSLY** | **as predicted — `ok … 2.044s`, log still prints `=0`** | *(none — that is the finding)* |
| **J** | wrong `want` (RED) → rename + `var _` deleted | GREEN, log lines vanish | **as predicted; and with `var _` KEPT the rename is a COMPILE ERROR** | *(none — the absence IS the evidence)* |
| **E** | dead upstream | **UNREACHABLE (F3)** | **not attempted, per PLAN** | *(dies at `structuralCheck`, step 8)* |

`git restore` after each; **`git status --porcelain` verified EMPTY** after every break, and the fixture re-verified GREEN as the final action of the sequence:
```
2026/07/25 16:15:14 0110 AssertStats: reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
2026/07/25 16:15:14 0110 AssertStats: subject   ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
--- PASS: TestDifferential/0110-tls-require-client-cert-false (1.78s)
```

**Zero PLAN break predictions were refuted at T5.** No pre-existing flake fired: the `subject ready: EOF` full-suite startup flake requires the full suite, and every run in this task was `-run`-isolated to a single fixture; all six runs completed with a container start, three arms and a clean teardown. **Classification: no failure in this task was a flake — every FAIL was a deliberate break, each confirmed by its own named assertion text.**

### Task 5 — surprises

1. **`test/fixtures/` really was drift-free, and re-deriving still earned its keep.** Every one of the eleven anchors held to the line — the opposite of T3's `+22` and T4's two PLAN-time errors. The value was in the *collision sweep*: `grep -rn 'StatsAsserter' 0110/` returns three hits, all prose in `README.md` and `expectations.yaml`, and a reader who stopped at the hit count could have concluded a collision existed. **The hits are T6's remit; the driver is clean.**
2. **`0110`'s `AssertStats` landed at `:655` — the SAME line as `0111`'s.** Pure coincidence of two similarly-sized drivers, and a genuine hazard for anyone grepping by line across the two fixtures.
3. **The failure attribution is `runner_test.go:1348`, not `driver.go`.** `t.Helper()` re-attributes every `Errorf` to the dispatch call site. Correct, but it means *all four* of this fixture's stats assertions report the same file:line, and they are told apart only by their message text.
4. **The reference emits 20 `ssl.*` series on this listener; the subject emits 4.** The asserted roster is the cross-side intersection. An exact-set comparison here would assert the framework gap rather than the counter — recorded because the phase-74 unit tests DO use exact-set pins (subject-side only, where the set is closed), and the two disciplines must not be confused across the seam.
5. **I-a's non-crash is the cleanest confirmation of F4 in the row.** The SPEC's framing predicted the ABSENT check guards a deleted registration; F4 showed that for an `Inc`'d counter it cannot, because the SIGSEGV kills the run first. I-a demonstrates the complementary half **positively**: on a registered-but-never-`Inc`'d counter, the deletion is silent, the run completes, and the ABSENT branch is the *only* thing that catches it. **The guard is real; it just guards the other counter.**

## Task 6 — ACTUAL ⚠️ A FOURTH STALE SITE, MISSED BY THE 14-PATTERN SWEEP

### PLAN anchors — RE-DERIVED before use (all three edit targets HELD; the completeness claim did NOT)

| anchor | PLAN says | RE-DERIVED at the T6 tip | verdict |
|---|---|---|---|
| `0110/README.md:160-163` | bundled bullet, live half starts MID-LINE at `:161` ("Never assert") | **`:160-163` exactly, and the live half does start mid-`:161`** | **HOLDS** |
| `0110/envoy.yaml:24` | single stale clause `# identity and NOT a stat (envoy-go emits no ssl.* family; see README).` | **`:24`, verbatim** | **HOLDS** |
| `0110/expectations.yaml:166-171` | three clauses, third INVERTS | **`:166-171`, all three present verbatim** | **HOLDS** |
| `0110/expectations.yaml:124-142` | `## Asserted`, legs (a) and (b) | **`:124-142`, legs (a)/(b) exactly** | **HOLDS** |
| `0111/README.md:167-174` (form template) | template span | **the bullet actually runs `:168-175`** — `:167` is the *alert text* bullet | **OFF BY ONE** |
| `0111/expectations.yaml:196-207` (RD-0111-TEMPLATE's correction of the SPEC) | block is `:196-207`, not `:196-201` | **`:196-207`, and it does APPEND rather than delete** | **HOLDS — RD-0111-TEMPLATE vindicated** |
| `0111/envoy.yaml:23-27` multi-line header | the `0110` line GROWS | **`:21-27`; the phase-74 addition is `:24-27`** | **HOLDS in substance** (the `0110` line grew 1 → 5) |
| RD-0110-STALE: *"exactly THREE stale sites inside `0110/`; no fourth"* | 14-pattern sweep | **REFUTED — see below** | **REFUTED** |

### ⚠️ The FOURTH stale site: `expectations.yaml`'s `(no StatsAsserter)` parenthetical

`expectations.yaml:175` (pre-edit) reads:

```
#   The sds.<secret>.* stat counters (no StatsAsserter).
```

The task brief and RD-0110-STALE both put this line on the **DO NOT TOUCH** list, on the ground that the `sds.*` counters *are* still correctly unasserted and *"`0111` kept its equivalent."* **The boundary is indeed still true. The parenthetical is not.** As of T5 this fixture HAS a `StatsAsserter`; `(no StatsAsserter)` now reads as a claim about the fixture, and it is FALSE at this tip. It is also **exactly the site `0111` did NOT keep** — phase 74 rewrote its equivalent to *"(not asserted by this fixture's StatsAsserter, which is CONFINED to `listener.<addr>.ssl.*` …)"* (`0111/expectations.yaml:211-214`) for precisely this reason. So the premise *"0111 kept its equivalent"* is true of the boundary and false of the parenthetical.

**Two of the brief's own criteria collide on this one line:** *"the `sds.*` boundary notes must still be PRESENT"* and *"every surviving hit must be TRUE at this tip."* Editing the parenthetical satisfies both; leaving it satisfies only the first. **Disposition: the parenthetical is CORRECTED, mirroring `0111`'s phase-74 wording, and the boundary is left standing and explicitly re-affirmed in the same note (*"The boundary itself is UNCHANGED: these counters remain unasserted"*).** `README.md:164-165` — the other DO-NOT-TOUCH site — makes **no** `StatsAsserter` claim and is **byte-untouched**, as instructed.

### Step 1 — the README bullet SPLIT (not deleted)

Two bullets replace one. The first is the RETIRED `ssl.*` boundary, quoting the retired text so the change is auditable rather than silent, and carrying all four asserted values plus the discrimination reason. The second is the `/listeners` guard, **preserved verbatim** from mid-`:161`→`:163` and labelled *"an INDEPENDENT boundary, unaffected by the retirement above and still LIVE."*

### Step 2 — `envoy.yaml`: comment only, 1 line → 5, ZERO config bytes changed

```
git diff --stat master -- test/fixtures/0110-tls-require-client-cert-false/envoy.yaml
 test/fixtures/0110-tls-require-client-cert-false/envoy.yaml | 6 +++++-
```
Every changed line begins with `#`. The `require_client_certificate: false` config is untouched.

### Step 3 — `expectations.yaml`: leg (c) added, clause 3 AMENDED not deleted

Leg (c) follows `0111/expectations.yaml:158-166` and adds what `0111`'s cannot: the four values with their per-arm attribution (`handshake=2` arms 1+3, `no_certificate=1` arm 3 only, `fail_verify_error=1` arm 2's forced send, `fail_verify_no_cert=0` never), the name-keyed/label-ignored cross-side keying, the ABSENT-vs-value split **with the F4 finding** (the guard earns its keep on `fail_verify_no_cert`, NOT on `no_certificate`, which SIGSEGVs on a deleted registration long before `AssertStats`), and the two per-side preconditions.

Clause 3 kept *"strictly STRONGER than a subject-only stat"* and appended *"— it is cross-side, and it now has a cross-side STAT beside it"*, per the `0111` template, then added the sharper `0110`-only reason: **the accept/reject contrast CANNOT distinguish arm 1 from arm 3 (both ACCEPTED at `require=false`), whereas `ssl.no_certificate=1` against `ssl.handshake=2` does.**

### Step 4 — the verification grep, ACTUAL

```
$ grep -rn 'ssl\.\|StatsAsserter\|infeasible\|INFEASIBLE\|emits no\|emits NO\|framework gap' \
    test/fixtures/0110-tls-require-client-cert-false/ --include='*.md' --include='*.yaml'
envoy.yaml:25:# COUNTER layer: the driver's AssertStats pins listener.<addr>.ssl.handshake=2,
envoy.yaml:26:# ssl.no_certificate=1, ssl.fail_verify_error=1 and ssl.fail_verify_no_cert=0 on
expectations.yaml:141:#       listener.<addr>.ssl.{handshake,no_certificate,fail_verify_error,
expectations.yaml:153:#       ssl.no_certificate=1 against ssl.handshake=2 is the DISCRIMINATOR this
expectations.yaml:158:#       counter it genuinely protects is ssl.fail_verify_no_cert (want 0, Inc'd
expectations.yaml:159:#       on no arm) — NOT ssl.no_certificate, which IS Inc'd on arm 3, so deleting
expectations.yaml:162:#       (the accept path ran) and ssl.handshake > 0 (the TLS path itself ran —
expectations.yaml:191:#   The ssl.* stat family is NO LONGER unasserted — phase 75 RETIRED that boundary,
expectations.yaml:192:#   and the old clause ("envoy-go emits NO ssl.* stats whatsoever … a verdict
expectations.yaml:193:#   StatsAsserter is therefore INFEASIBLE") was true up to phase 74 and is FALSE at
expectations.yaml:194:#   this tip: envoy-go now registers listener.<addr>.ssl.{handshake,no_certificate,
expectations.yaml:197:#   family: ssl.connection_error (envoy-go's `other` handshake outcome increments
expectations.yaml:199:#   ssl.ciphers/curves/versions breakdowns. The accept/reject CONTRAST remains the
expectations.yaml:204:#   at require=false BOTH are ACCEPTED — whereas ssl.no_certificate=1 against
expectations.yaml:205:#   ssl.handshake=2 does. Never assert /listeners or total_listeners_active, and
expectations.yaml:210:#   The sds.<secret>.* stat counters (NOT asserted by this fixture's StatsAsserter,
expectations.yaml:212:#   which is CONFINED to listener.<addr>.ssl.* — DriveSubject hard-stops both SDS
README.md:160:- **`ssl.*` stats are now ASSERTED CROSS-SIDE** (phase 75 — this boundary is
README.md:161:  RETIRED; the old *"envoy-go emits none, so a verdict `StatsAsserter` is
README.md:162:  infeasible"* text was true up to phase 74 and is FALSE at this tip). envoy-go
README.md:163:  registers `listener.<addr>.ssl.{handshake,no_certificate,fail_verify_error,
README.md:171:  Still out of scope: `ssl.connection_error` (envoy-go's `other` handshake
README.md:173:  `ssl.ciphers/curves/versions` breakdowns.
```
**Every surviving hit is TRUE at this tip.** The two hits that still contain the words `INFEASIBLE` / `infeasible` (`expectations.yaml:192-193`, `README.md:161-162`) are **quotations of the retired text, explicitly labelled FALSE at this tip** — a deliberate choice so the retirement is auditable, not a survival of the claim. The `sds.*` boundary notes are **PRESENT** on both files (`README.md:164-165` byte-untouched; `expectations.yaml:210-215` boundary intact, parenthetical corrected).

*(The `--include` filters keep `driver/driver.go` out of the listing; the unfiltered sweep additionally returns **15** driver hits — counted mechanically, `… driver/driver.go | wc -l` — all TRUE, and every one of them T5's own landed asserter code and prose (`:616`–`:745`, `:835`, `:843`). The `PLAN-65 C3` alert-text references at `:376`/`:544` match NONE of these patterns and do not appear in the sweep at all — RD-0110-STALE's *"driver carries ZERO stale claims"* HOLDS.)*

### Step 4 — the fixture run, ACTUAL

```
$ go test ./test/differential/ -count=1 -run 'TestDifferential/0110-tls-require-client-cert-false'
ok  	github.com/pgdad/envoy-go/test/differential	1.999s

$ … -v
2026/07/25 16:21:20 0110 AssertStats: reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
2026/07/25 16:21:20 0110 AssertStats: subject ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
--- PASS: TestDifferential (1.99s)
ok  	github.com/pgdad/envoy-go/test/differential	2.061s
```
Both runs used the FULL selector. **No flake fired; nothing to classify** — both runs were `-run`-isolated to one fixture, each started a container, drove three arms and tore down cleanly. The known `subject ready: EOF` startup flake needs the full suite (T10's remit).

### Task 6 — surprises

1. **The completeness claim was the thing that drifted, not the anchors.** All four `0110` edit targets held to the line — and the one statement that failed was the meta-claim *"a 14-pattern sweep confirmed no fourth."* A sweep that greps for the STALE vocabulary cannot see a site that goes stale in the OPPOSITE direction: `(no StatsAsserter)` was TRUE when written and was falsified by **this row's own T5**, not by phase 74. `reference_document_hygiene_claim_not_evidence`, applied to a sweep instead of a document.
2. **`0111` is the template for the fix AND the counter-example to the reason for skipping it.** The brief justified the DO-NOT-TOUCH with *"`0111` kept its equivalent"* — but `0111` kept the **boundary** and rewrote the **parenthetical**, which is exactly the edit that was being waived off.

## Task 7 — ACTUAL ⚠️ THE FIRST EXECUTION OF THE `0111` NAMED-SUBSET CHECK

### PLAN anchors — RE-DERIVED before use

| anchor | PLAN says | RE-DERIVED | verdict |
|---|---|---|---|
| `0111/README.md:167-174` | the closed-enumeration bullet | **the bullet is `:168-175`**; `:167` is the *alert text* bullet | **OFF BY ONE** (same drift as T6's template anchor — one PLAN error, cited twice) |
| `0111/expectations.yaml:197-203` | *"Still UNasserted from that family: …"* | **`:196-207`; the "Still UNasserted" sentence spans `:199-202`** | **HOLDS in substance** |
| `0111/expectations.yaml:159` | enumerates the three-name family | **`:159` exactly** — leg (c)'s roster line | **HOLDS** |
| `0111/driver/driver.go:682-696` (three value rosters) | stay at THREE names | **not read, not edited — byte-untouched** | **N/A by design** |

### Step 1 — the edits

Three sites, all prose:
- `README.md` — `ssl.no_certificate` added to the *"Still out of scope"* list **with the reason**: the name exists as of phase 75 and IS asserted cross-side **at `0110-tls-require-client-cert-false`, not here**, because this fixture's `require_client_certificate: true` rejects a no-cert connection (booking `ssl.fail_verify_no_cert`) so it never reaches the success-path annotation ⇒ **0 on every arm, structurally** ⇒ a value pin here would be a vacuous `0 == 0`.
- `expectations.yaml:196-207` — the same, in the `UNasserted` paragraph, naming `0110`'s discriminating non-zero (`no_certificate=1` against `handshake=2`) and why it is the only thing separating `0110`'s two ACCEPTING arms.
- `expectations.yaml:159` — leg (c)'s three-name roster annotated as **deliberately three**, with a pointer to the UNasserted paragraph. Without this, a reader who counts names in leg (c) and finds three where the family now has four reads it as an omission.

⚠️ **The `require=true ⇒ structurally 0` claim is DERIVED, not EXECUTED here.** It follows from `require_client_certificate: true` on both sides (PLAN §2.4 / A4-N6) plus T3's guarded `Inc`, which sits on the **success** path; this task did not instrument `0111`'s scrape to read the literal `0`. The prose says *"structurally"*, which is the honest strength of the claim.

### Step 2 — the driver is BYTE-UNTOUCHED, proven

```
$ git -C /home/esa/git/envoy-go-wt-p75impl diff --stat master -- \
    test/fixtures/0111-tls-cvc-empty-dynamic-fallback/driver/driver.go
$        (no output — empty, as required)
```

### Step 2 — ⚠️ THE FIRST EXECUTION of the named-subset check. **PASS.**

The PLAN verified by CODE READ only that `0111`'s asserter iterates a NAMED SUBSET, so T1's new name in the subject's scrape could not break it. **That is now EXECUTED:**

```
$ go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback' -v
2026/07/25 16:22:09 0111 AssertStats: reference ssl.handshake=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=1 (downstream_cx_total=3)
2026/07/25 16:22:09 0111 AssertStats: subject ssl.handshake=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=1 (downstream_cx_total=3)
--- PASS: TestDifferential/0111-tls-cvc-empty-dynamic-fallback (1.90s)
ok  	github.com/pgdad/envoy-go/test/differential	1.963s
```

**The code read is CONFIRMED by execution.** `0111`'s three phase-74 values are unchanged on BOTH sides at the phase-75 tip — the extra name in the subject's `/stats/prometheus` output is invisible to a named-subset iteration, and the sub-test is confirmed to have RUN (the sub-test PASS line is present, not `[no tests to run]`). **No flake fired**; the run was `-run`-isolated, started a container, drove three arms and tore down cleanly.

⚠️ **Note the shape difference this run makes concrete:** `0111` reads `handshake=1 / fail_verify_no_cert=1`, `0110` reads `handshake=2 / fail_verify_no_cert=0 / no_certificate=1`. Same three arms, same `downstream_cx_total=3`, opposite counter attribution for the no-cert arm — which is exactly why the value pin belongs at `0110` and the NAME belongs in `0111`'s unasserted list.

### Task 7 — surprises

1. **The one PLAN anchor that drifted, drifted in BOTH tasks.** `0111/README.md:167-174` is cited by RD-0111-TEMPLATE (as T6's form template) and by RD-0111-CLOSED (as T7's edit target); the bullet is `:168-175` in both. A single off-by-one, propagated to two rows — the copy-a-citation failure mode, from a PLAN rather than a subagent (`feedback_brief_citations_not_evidence`).
2. **The `0111` run was cheap and it was not redundant.** It is the only executed evidence in the row that T1's registration does not perturb a DIFFERENT fixture's cross-side stat assertions, and the ledger recorded it as unexecuted until now.

---

## Task 8 — ACTUAL ⚠️ ELEVEN IN-PLACE REWRITES + ONE ADDED LEDGER LINE; ZERO ANCHOR DRIFT — AND ONE STEP-3 EXPECTATION THAT CONTRADICTS STEP 2

### PLAN anchors — RE-DERIVED before use

| anchor | PLAN says | RE-DERIVED at the T8 tip | verdict |
|---|---|---|---|
| file length | 5744 lines, must end at 5746 | `wc -l` **5744** before, **5746** after | **HOLDS** |
| `:831` / `:847` | the two narrative bare totals, `Stat surface UNCHANGED at **1204**` | both exactly, `:831` graphite_statsd, `:847` OTLP | **HOLDS** |
| RD-BC-ROSTER — `grep -n 'ssl\.handshake'` ⇒ 916, 928, 1851, 1857, 1859, 5002 | six anchors | **all six, exactly** (before AND after) | **HOLDS** |
| RD-BC-962 | `:962`, 1002 chars / 1007 bytes, 0-based CHARACTER index **627**, 0-based BYTE offset **630** | **all four figures re-derived, identical, and the line is `cmp`-identical to master** | **HOLDS — survives a FOURTH challenge** |
| RD-BC-HEADING | 16 two-ADR headings; `:785` is the exact "later phase extends" semicolon precedent | `grep -cE` ⇒ **16** before, **17** after; `:785` read and matched byte-for-form | **HOLDS; SPEC §9 REFUTED as the PLAN said** |
| RD-LEDGER | tail `:5002`, `### Forward-pointer note (26.3)` at `:5004`; U+2014 / U+2192 / bold through the parenthetical | `cat -A` on the tail confirmed all three; heading confirmed at `:5004` | **HOLDS** |
| RD-BC-TOTALS | *"applied in a scratch copy: `grep -c '1204'` goes **3 → 1**"* | **ACTUAL 3 → 2** | ⚠️ **REFUTED** — see surprise 1 |
| PLAN Step 3 | `grep -c '1201'` *"expect UNCHANGED (1 line)"* | **ACTUAL 1 → 2** | ⚠️ **REFUTED BY STEP 2 ITSELF** — see surprise 2 |
| F12 | do NOT extend the `:152-157` listener table | not read as an edit site, not touched; **no diff hunk in the 100s** | **OBEYED** |

### Step 1 — the eleven in-place rewrites, every one of them `-N +N`

| line | what changed |
|---|---|
| `:831`, `:847` | `Stat surface UNCHANGED at **1204**` → **`1205`**, both |
| `:916` | RETIRED-names sentence extended to a FOURTH name; `0111` does NOT assert it (`require=true` ⇒ structurally 0 ⇒ vacuous `0 == 0`), the cross-side assertion is `0110` |
| `:918` | `emits three …ssl.* counters` → **four** (three at 74, a fourth at 75); departure explicitly **UNCHANGED by phase 75 too**, same reason — envoy-go BOOT-FAILS so no handshake completes |
| `:928` | C3 heading `three fixed …names by phase 74` → **FOUR fixed names** (three by ADR-0296, a fourth by ADR-0297); RETIRED-half body enumerates all four, names `0110` as the assertion site and says why not `0111` |
| `:1849` | **heading EXTENDED**, `:785`'s semicolon `<what> per phase <N> ADR-XXXX` form: `…(per phase 74, ADR-0296; the \`ssl.no_certificate\` success-path annotation per phase 75 ADR-0297)` |
| `:1851` | roster → four internal names + four Prometheus forms; fourth flagged **SUCCESS-PATH ANNOTATION, not a failure bucket**; **`three-fifths` REMOVED, not bumped**, replaced by an ENUMERATION (4 fixed retired / 4 dynamic surviving) with the reason the `5` was a fossil |
| `:1853` | **appended in place**: the predicate `len(tlsConn.ConnectionState().PeerCertificates) == 0` ALONE, its NO-client-auth term, and the BOTH-DIRECTIONS contrast with `ssl.fail_verify_no_cert` (over-counting accepted anonymous connections one way; double-booking a genuine no-cert REJECTION the other; mutually exclusive on every connection). Wire evidence **cited to ADR-0297 §Context ¶2/¶3 and explicitly marked as transcribed, NOT re-derived here** |
| `:1855` | *"three names and not four"* → the withheld fourth is **`ssl.connection_error`**, still withheld, NOT `ssl.no_certificate`; and this subsection recorded as the **true referent of the dangling "BEHAVIOR_CONTRACT B5/B6" citations** — no B-numbered step scheme exists in this file |
| `:1857` | `all three counters` → `all four`; the fourth **INHERITS** QUIC parity from phase 74's FAMILY-level probe, **NOT re-probed**; inheritance sound because the Inc site is unreachable for `kindQUIC` for a **structural, name-independent** reason (`Manager.Start` `continue`s at `manager.go:1078-1082`) |
| `:1859` | `ssl.* triple` → *"the `ssl.*` family — three names at phase 74, **FOUR** as of phase 75"*; `0110`'s asserter recorded as taking the **identical** label-ignoring `/stats/prometheus` posture. ⚠️ **All three `three` tokens on this line — the three DIVERGENCE CLASSES — left untouched** (verified: `grep -o 'three' | wc -l` ⇒ 3 before and after) |

### Step 2 — the ONE line-adding edit

Inserted at `:5004`, after `:5002`'s Phase-74 tail and its blank `:5003`, before `### Forward-pointer note (26.3)` (now `:5006`). Byte form copied from the **TAIL**, not `:5000`.

It records: +1 name; registered in the block that **ALREADY** gates on `rt.tlsMode` (no new gate / classifier / registration function); Inc'd on the success fall-through guarded on `len(…PeerCertificates) == 0` **ALONE**; plaintext **ZERO**; QUIC registered-and-permanently-zero (**PARITY**, inherited structurally, not re-probed); fixtures **119 → 119** (`0110` EXTENDED, **NOT `0111`** — with the vacuous-`0 == 0` reason); BackendKind **38 → 38**; fuzzers **+0**; ZERO new packages / modules / production imports / exported symbols; records **ADR-0297**.

⚠️ **BOTH ledger gaps recorded, NEITHER back-filled:** the known unattributed `1200 → 1201`, and the **SECOND, previously unrecorded** one — `Phase 46.1b` (`:4996`) closes at **1198** while `Phase 47.1` (`:4998`) opens at **1200**, an unattributed **+2** with no candidate identified. The **+1 DELTA** is stated as asserted-with-confidence; the absolute **1205** is stated as **DOCUMENTARY**, with an explicit warning that a mechanical re-derivation should be expected to disagree with it. No line was fabricated to close either gap.

### Step 3 — the verification block, ACTUAL OUTPUT

```
$ B=docs/envoy-go/BEHAVIOR_CONTRACT.md

$ wc -l $B
5746 docs/envoy-go/BEHAVIOR_CONTRACT.md          # 5744 -> 5746, exactly +2

$ grep -n 'ssl\.handshake' $B
916:  928:  1851:  1857:  1859:  5002:            # ALL SIX UNMOVED (identical to the pre-edit run)

$ grep -c '1204' $B ; grep -c '1205' $B
2                                                 # :5002 and :5004
3                                                 # :831, :847, :5004

$ grep -c '1201' $B
2                                                 # :5002 and :5004  ⚠️ PLAN said 1 — see surprise 2

$ sed -n '962p' $B | grep -c 'ssl\.no_certificate'
1

$ sed -n '5004p' $B | cat -A | head -c 200
**Phase 75 M-bM-^@M-^T 1204 M-bM-^FM-^R 1205 (+1) (the FOURTH listener-scope `ssl.*` name M-bM-^@M-^T the FIRST SUCCESS-PATH handshake annotation):** phase 75 (`listener.<normalized-addr>.ssl.no_certi

$ sed -n '5004p' $B | cat -A | tail -c 60
pect the re-derived figure to disagree with **1205**.**]**$

$ grep -cE '^#{2,4} .*ADR-[0-9]+.*ADR-[0-9]+' $B
17                                                # 16 -> 17, the :1849 heading joins them
```

`M-bM-^@M-^T` = U+2014 EM DASH, `M-bM-^FM-^R` = U+2192 RIGHTWARDS ARROW — **both present, no ASCII `->`** — and the bold runs **THROUGH** the parenthetical to the colon (`…annotation):**`), matching the tail and NOT `:5000`. The trailing `**]**` closes the ledger-gap block in the tail's own style.

### The `:962` PROOF — byte-identity AND both offsets, with UNITS

```
$ git -C /home/esa/git/envoy-go-wt-p75impl show master:docs/envoy-go/BEHAVIOR_CONTRACT.md | sed -n '962p' > /tmp/m962.txt
$ sed -n '962p' $B > /tmp/h962.txt
$ cmp /tmp/m962.txt /tmp/h962.txt && echo IDENTICAL
IDENTICAL

$ git -C /home/esa/git/envoy-go-wt-p75impl diff -U0 master -- $B | grep '^@@'
@@ -831 +831 @@      @@ -847 +847 @@      @@ -916 +916 @@      @@ -918 +918 @@
@@ -928 +928 @@      @@ -1849 +1849 @@    @@ -1851 +1851 @@    @@ -1853 +1853 @@
@@ -1855 +1855 @@    @@ -1857 +1857 @@    @@ -1859 +1859 @@    @@ -5003,0 +5004,2 @@
# NO hunk touches :962 — and no hunk touches anything in the 100s, so F12's :152-157 table is untouched too.

$ python3  # re-derived at the POST-EDIT tip
line 962: CHARS=1002  BYTES=1007
0-based CHARACTER index of ssl.no_certificate = 627
0-based BYTE offset      of ssl.no_certificate = 630
```

⚠️ **The unit distinction is stated, not elided: 627 is a CHARACTER index; 630 is a BYTE offset.** The 5-byte spread over 1002 characters is the line's five multi-byte punctuation glyphs. The anchor has now survived a **FOURTH** challenge.

⚠️ **`ssl.no_certificate` is NO LONGER the sole occurrence in the file** — by design. It now appears on **10 lines**: `916, 918, 928, 962, 1849, 1851, 1853, 1855, 1857, 5004` (17 occurrences). RD-BC-962's SOLE-OCCURRENCE property was a property of the *master* tip and is retired by this row; what survives, and what the ledger actually pinned, is `:962`'s **byte-identity**, proven above.

### The ANCHOR-STABILITY proof

Every `-N +N` hunk above is a same-line-number rewrite. `git diff --stat` reads **13 insertions, 11 deletions** = **11 rewritten lines + 2 added lines**, and the only `,N` hunk is the terminal `@@ -5003,0 +5004,2 @@`.

⇒ **`:1849 / :1851 / :1853 / :1855 / :1857 / :1859 / :5002` all sit at their original line numbers**, verified by `sed -n '<n>p'` after the edits (each printed its own original opening clause). The line citations from `ROADMAP.md:137`, `STATE.md:20/46/48`, phase-75 `SPEC.md` and `BRAINSTORM.md` are **all still valid**. F7's constraint is met exactly: **one** line-adding edit in the whole file, and it lands **below** every cited TLS-subsection anchor.

### Task 8 — surprises

1. **RD-BC-TOTALS' scratch-copy figure is REFUTED: `grep -c '1204'` goes 3 → 2, not 3 → 1.** The ledger states *"applied in a scratch copy: `grep -c '1204'` goes 3 → 1"*. It cannot: the new ledger line **opens with `1204 → 1205`**, so it necessarily carries a `1204` of its own. The scratch copy evidently applied the two total edits without the Step-2 insertion. **PLAN Step 3's own inline comment (`# expect 2 lines / 3 lines`) is the correct one** — the two figures inside one PLAN disagree, and the executed answer matches Step 3.
2. ⚠️ **PLAN Step 3's `grep -c '1201'  # expect UNCHANGED (1 line)` is contradicted by PLAN Step 2, which MANDATES the token.** Step 2 requires the new entry to record *"the known `1200 → 1201`"* gap; writing that necessarily puts `1201` on `:5004`. The two instructions cannot both be satisfied. **Step 2 was obeyed** (recording the gap is load-bearing; the grep count is a tripwire for *accidental* edits). The tripwire's INTENT is satisfied and verified by enumeration rather than by count: the pre-existing `1201` lives on `:5002` and is **byte-untouched** — `:5002` appears in no diff hunk. Recorded rather than smoothed over, per `reference_a_drift_correction_is_itself_a_claim`.
3. **The `:1851` retirement rewrite forced a second decision the PLAN did not spell out.** Removing the ratio required saying what the four names DO retire. The claim landed is *"the FIXED-name half of C3 in full"*, checked against `:928`, which enumerates exactly four surviving DYNAMIC families — so the enumeration is **4 retired / 4 surviving**, internally consistent with the same document. `ssl.connection_error` is deliberately **not** counted in either half: `:1855` records it as a separate NAMED DEPARTURE, not a C3 member, and conflating them would have minted a fifth wrong denominator.
4. **`:1849`'s heading now carries a backtick, and the two-ADR gate still matches it.** `grep -cE '^#{2,4} .*ADR-[0-9]+.*ADR-[0-9]+'` reads 17 — confirmed post-edit, not assumed. The first clause's existing `per phase 74, ADR-0296` comma was left byte-intact; only the appended clause takes `:785`'s comma-free `per phase 75 ADR-0297` form, so the precedent is matched without gratuitously rewriting phase 74's own attribution.
5. **Docs-only, as chartered.** `git diff --stat master` names exactly ONE file. No Go file, no fixture, no gate was touched, so no build/test gate is owed by this task.

---

## Task 9 — ACTUAL ⚠️ THREE FILES, NOT FIVE — AND A SITE THE PLAN ROSTER NEVER NAMED

**Commit:** `a4d4908c` *(the sha backfill itself lands as a one-line follow-up commit — a commit cannot cite its own sha)* · **Files:** `internal/listener/manager.go`, `internal/listener/manager_test.go`, `internal/listener/quic_test.go`

⚠️ **PROSE-ONLY, MECHANICALLY PROVEN.** The whole-diff non-comment filter is EMPTY:

```
$ git -C /home/esa/git/envoy-go-wt-p75impl diff -U0 \
    | grep -E '^[+-]' | grep -vE '^(\+\+\+|---)' | grep -vE '^[+-]\s*//'
<no output>
$ git -C /home/esa/git/envoy-go-wt-p75impl diff --stat
 internal/listener/manager.go      | 15 ++++++++++++++-
 internal/listener/manager_test.go | 19 +++++++++++--------
 internal/listener/quic_test.go    |  9 +++++----
 3 files changed, 30 insertions(+), 13 deletions(-)
```

Every added and removed line begins with `//`. **No behaviour changed, and none of these sites could have produced red** — which is the whole reason this is a task rather than a footnote.

### The re-derived roster — PLAN anchor → ACTUAL anchor, found by CONTENT

⚠️ **The PLAN's line numbers are pre-T1 and the drift is NON-UNIFORM.** Every site below was located by grepping its own stale phrase, never by line number.

| PLAN cite | Stale phrase (grepped) | ACTUAL line | Drift | Disposition |
|---|---|---|---|---|
| `manager.go:385-386` | *"…into the **three** counted buckets plus a fourth that counts NOTHING"* | **`:392-393`** | **+7** | **EDITED** (Step 1) |
| `manager.go:424` (RD-CLASSIFY) | `classifyHandshakeErr` decl | **`:431`** | **+7** | **NOT EDITED — RD-CLASSIFY RE-CONFIRMED** (below) |
| `manager_test.go:1936` | *"began carrying the **three** `listener.<addr>.ssl.*` counters"* | **`:1936`** | **0** | **EDITED** |
| `manager_test.go:1987-1989` | banner *"the three listener-scope ssl.\* counters … carries exactly three ssl.\* names"* | **`:1988-1989`** | **+1 at the head** | **EDITED** (the banner text now spans `:1988-1992`) |
| `manager_test.go:1993-1997` | `listenerSSLNames`' doc, *"WOULD PASS WITH ALL **THREE** NAMES MISSPELLED"* | **`:1995-1999`** (phrase on `:1999`) | **+2** | **EDITED** |
| `manager_test.go:4466` | *"the other two are exactly 0"* | **GONE** | — | **ALREADY DONE at T3** (rewritten to *"every UNNAMED counter in `sslLeafRoster` is exactly 0"*, now `:4504`) |
| `manager_test.go:4491-4492` | *"neither failure counter moves"* | **GONE** | — | **ALREADY DONE at T3** (rewritten to *"NO other ssl.\* leaf in `sslLeafRoster` moves"*, now `:4560`) |
| `manager_test.go:4561` | *"**three** ssl.\* pointers"* | **`:4729`** | **+168** | **EDITED** → *"four ssl.\* pointers"* |
| `quic_test.go:65` | `counterValue`'s doc, *"all **three** are zero"* | **`:65`** | **0** | **EDITED** → *"all four are zero"* |
| `quic_test.go:226` | *"registers all **THREE** ssl.\* counters"* | **`:226`** | **0** | **EDITED** → *"all FOUR ssl.\* counters (the three phase-74 outcome counters plus phase 75's ssl.no_certificate)"* |
| `quic_test.go:272` | the `(1)` REGISTRATION banner | **`:273`** | **+1** | **ALREADY DONE at T1** — reads *"all four names present, spelled exactly"* (the verbatim T1 block rewrote it) |
| `quic_test.go:295` | *"(4) …and all **three** ssl.\* counters are STILL ZERO"* | **`:303`** | **+8** | **EDITED** → *"all four"* |
| — **NOT IN ANY ROSTER** — | *"keeping **both** Inc points inside `if selected.tlsCfg != nil`"* | **`:4730`** (master `:4562`, drift **+168**) | — | **EDITED** → *"keeping **every ssl.\* Inc point**"* (see below) |

**Verified already-handled, as the PLAN claimed** *(each checked by reading the live text, not by trusting the PLAN)*:

- `manager_test.go:1940` / `:2019` / `:2126` / `:2152` / `:2203` → now **`:1941`** / **`:2020`** / **`:2135`** / **`:2161`** / **`:2222`**. **T1/T2 CONFIRMED, each by reading the live sentence** — `:1941` names `…RegistersExactlyFourSSLNames` (master read `…Three…`); `:2023` reads *"all three phase-74 names **plus phase 75's ssl.no_certificate**"*; `:2135` reads *"the **four** field pointers"*; `:2161` reads *"all **FOUR** counter fields are NON-NIL (three phase-74 …"*; `:2222` reads *"all **FOUR** counter fields are NIL"*. ⚠️ Drift in this block runs **+1 to +19** and is non-monotonic per region, so **not one** of these five was locatable by its PLAN line number.
- `internal/stats/name.go:448` (*"Of the **14** entries"*) / `:451-452` (*"the last **three** are the phase-74 … outcomes, whose three-dot source names"*) → now **`:448`** (*"Of the **15** entries"* — drift **0**) / **`:451-454`** (rewritten and GROWN, the trichotomy split out from the phase-75 entry). **T4 CONFIRMED**: *"Of the 15 entries … the next three are the phase-74 listener-scope TLS handshake outcomes; and the last is phase 75's listener-scope `ssl.no_certificate`"*. The arithmetic re-derived mechanically at this tip: **`helpText` has 15 entries** (`awk` over the literal), and 10 + 1 + 3 + 1 = 15. HOLDS.
- `internal/stats/name_test.go:222-223` (the `TestHelpText_ListenerSSLHandshakeOutcomes` doc head) → the doc block now runs to **`:236`**, with T4's additions at **`:231-236`**. **T4 CONFIRMED**: `wantNames` carries four names and the doc's *"this slice left at three, the whole package stayed GREEN"* is a **historical break record**, TRUE as written.

⇒ **This task touched THREE files, not the PLAN's "five" (`Files:` header said three; the commit-message text says five).** `name.go` and `name_test.go` were left BYTE-UNTOUCHED because T4 had already retired every claim in them.

### Step 1 — `handshakeOutcome`'s doc, and the RD-NOOUTCOME guard-rail stated IN THE CODE

The leading sentence (*"three counted buckets plus a fourth that counts NOTHING"*) is **left standing, because it is TRUE** — it counts OUTCOME buckets (`outcomeOK`/`outcomeVerifyError`/`outcomeNoCert` + `outcomeOther`), and phase 75 added no variant. What was missing is that a reader at +1 cannot tell "three" is about outcomes and not about the now-**four** `ssl.*` counters. Added, at `manager.go:397-406` (inside `handshakeOutcome`'s doc block, which runs `:392-430` ahead of `type handshakeOutcome` at `:431`):

- the taxonomy is an **ERROR-PATH** one — the classifier is consumed at exactly one site and never sees a successful handshake;
- ⚠️ *"three counted buckets" counts **OUTCOMES**, not `ssl.*` **COUNTERS*** — the listener scope carries **FOUR** `ssl.*` counters as of phase 75;
- **phase 75 added `ssl.no_certificate` WITHOUT adding a `handshakeOutcome` variant, and that is deliberate**: it is a SUCCESS-path annotation Inc'd *after* the error branch has already `return`ed, entirely OUTSIDE the `classifyHandshakeErr` switch;
- it is booked on a COMPLETED handshake that presented no client cert — **the exact complement of `outcomeNoCert` (a REJECTED handshake)**;
- **adding a `handshakeOutcome` variant for it, or routing it through the classifier at all, is a design error for that row.**

⇒ **RD-NOOUTCOME is now enforced by the doc comment a future author reads first**, not only by a PLAN row that expires with the phase.

### RD-CLASSIFY — RE-VERIFIED AT THIS TIP, AND IT HOLDS

```
$ grep -n 'noClientCertErrText is crypto\|^const noClientCertErrText\|^func classifyHandshakeErr' internal/listener/manager.go
427:// noClientCertErrText is crypto/tls's bare errors.New for "the client was asked
429:const noClientCertErrText = "tls: client didn't provide a certificate"
431:func classifyHandshakeErr(err error) handshakeOutcome {
```

**`classifyHandshakeErr` has NO doc comment.** `:430` is blank; the comment block at `:427-428` is the doc comment of `const noClientCertErrText` (`:429`), and Go's doc rules bind it there. **An instruction to "amend the comment above `classifyHandshakeErr`" still has NO TARGET** — unchanged from the PLAN's finding, only shifted +7. **Nothing was written above `:431`.**

### The site the PLAN roster MISSED — `manager_test.go:4730`

`TestServeConnection_PlaintextListenerIncrementsNoSSL`'s doc read *"keeping **both** Inc points inside `if selected.tlsCfg != nil`"*. **"Both" was already wrong before this row** — phase 74 landed **three** `ssl.*` `Inc` sites (`sslFailVerifyError`, `sslFailVerifyNoCert`, `sslHandshake`) and phase 75 makes it **four**. The 14-pattern sweep could not see it for the same reason the T6 finding escaped: **the stale token is `both`, not `three`**, so no "three"-shaped grep reaches it. Rewritten to *"keeping **every `ssl.*` Inc point**"* — a formulation that cannot go stale at the next `+1`. **This is the SECOND site in this phase found only by reading, after T6's `(no StatsAsserter)`.**

### Step 3 — the survivor grep, walked HIT BY HIT

```
$ grep -rn -i 'three' internal/listener/*.go internal/stats/name*.go | grep -i 'ssl\|counter\|pointer'
```

**11 hits, and every one is TRUE at four names:**

| # | Hit | Verdict |
|---|---|---|
| 1 | `name.go:451` *"the next **three** are the phase-74 listener-scope TLS handshake outcomes"* | **TRUE** — historically exact; the sentence continues *"and the last is phase 75's …"* |
| 2 | `name.go:454` *"All **four** ssl.\* entries have **three-dot** source names"* | **TRUE** — "three-dot" describes `listener.<addr>.ssl.<leaf>`, a DOT COUNT, not a family size; the family count in the same sentence already says four |
| 3 | `manager.go:363` *"The **three** phase-74 ssl.\* counters plus the one phase-75 ssl.\* counter (ssl.no_certificate — four in total)"* | **TRUE** — landed at T1 |
| 4 | `manager.go:398` *"⚠️ \"three counted buckets\" counts OUTCOMES, not ssl.\* COUNTERS"* | **TRUE** — this task's own disambiguation |
| 5 | `quic_test.go:226` *"all FOUR ssl.\* counters (the **three** phase-74 outcome counters plus phase 75's ssl.no_certificate)"* | **TRUE** — this task's edit |
| 6 | `name_test.go:224` *"the project's first **three-dot** listener names"* | **TRUE** — DOT COUNT again, not a family size |
| 7 | `name_test.go:235` *"with the phase-75 entry present and this slice left at **three**, the whole package stayed GREEN"* | **TRUE** — a verbatim record of T4's break G; a *historical* three |
| 8 | `manager_test.go:1936` *"the phase-74 `listener.<addr>.ssl.*` counters (**three** then, four as of phase 75's ssl.no_certificate)"* | **TRUE** — this task's edit |
| 9 | `manager_test.go:1990` *"carries exactly FOUR ssl.\* names (the **three** phase-74 handshake-outcome counters plus …)"* | **TRUE** — this task's edit |
| 10 | `manager_test.go:2023` *"all **three** phase-74 names plus phase 75's ssl.no_certificate"* | **TRUE** — landed at T1 |
| 11 | `manager_test.go:2161` *"all FOUR counter fields are NON-NIL (**three** phase-74 …"* | **TRUE** — landed at T2 |

⚠️ **The grep does NOT reach `manager.go:392-393`** — its stale-shaped token is `counted`, and `grep -i 'counter'` does not match `counted`. **A survivor grep that greps for `counter` cannot see a sentence that says `counted buckets`.** That site was reached only because Step 1 named it explicitly. Recorded as a limit of the gate, not as a pass.

A widened repo-wide sweep (`grep -rniE 'three ssl|all three|THREE NAMES|three counters|three counted' --include='*.go'`) returned **no further `ssl.*`-family staleness** — every other hit is an unrelated "all three" about test arms, fields, or backends. `0110/driver/driver.go:451`'s *"All three arms"* was checked against `structuralCheck`'s body and is **the three DRIVE arms** (trusted / untrusted / no-cert probe), untouched by T5's `StatsAsserter` addition. **Not stale.**

### Gates — EXECUTED

```
$ gofmt -l internal/listener internal/stats && echo "GOFMT CLEAN"
GOFMT CLEAN                       # empty output, then the echo
$ go vet ./internal/listener/... ./internal/stats/...
VET CLEAN
$ golangci-lint run ./internal/listener/... ./internal/stats/...
LINT CLEAN
$ go test ./internal/listener/... ./internal/stats/... -count=1
ok  	github.com/pgdad/envoy-go/internal/listener	3.229s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
ok  	github.com/pgdad/envoy-go/internal/stats	0.003s
ok  	github.com/pgdad/envoy-go/internal/stats/dynamic	0.013s
```

⚠️ **GREEN BEFORE AND GREEN AFTER, and that is the EXPECTED result, not evidence of correctness.** These edits are unasserted prose; `go test` cannot distinguish this task from an empty commit. **The evidence for this task is the survivor-grep table above and the empty non-comment diff filter — not the test run.** Stated plainly so the green does not read as coverage (`reference_liveness_break_needs_failing_baseline`).

### §2.5 disposition — RECORDED, NOT FIXED, and NAMED

Left **BYTE-UNTOUCHED** by design (confirmed absent from `git diff --stat`):

- **`internal/statssink/registration_test.go:25` / `:51` / `:80`** — *"(stays 1200 / 1196)"* / *"(stays 1200 / non-H2 1196)"*. Stale since **phase 49** (`65130bbe`) and never updated by phase 74's `+3`, so it was **already three phases wrong before this row touched anything**. **Unasserted by code**: the five guards compare a FRESH registry against 0 and never read the number, so nothing goes red at 1200, 1204, or 1205.
- **`internal/statssink/statsd_tcp.go:78`** — *"~1200 stats"*, already hedged by the tilde and therefore not falsified by `+1`.

**Rationale for NOT fixing:** `internal/statssink/**` is on this phase's **BYTE-UNTOUCHED sha256 roster** (Global Constraints). Fixing four prose lines would widen the row's delta into a package it does not otherwise touch, force the file off the gate roster, and trade a documented stale comment for an undocumented gate hole. **PLAN §5 carries it forward.** ⚠️ **Stating the disposition IS the deliverable** — an unnamed stale site is exactly how a roster goes stale (`reference_fuzzer_count_docs_drift`).

### Task 9 — surprises

1. **TWO of the PLAN's twelve roster entries were ALREADY RETIRED, both at T3, and neither was flagged as such.** `manager_test.go:4466` (*"the other two are exactly 0"*) and `:4491-4492` (*"neither failure counter moves"*) do not exist at this tip: T3's variadic rewrite of `assertSSLCrossProduct` and its `sslLeafRoster` re-doc replaced both sentences with roster-relative language that is **automatically correct at any family size**. Verified by diffing against `git show master:` — both phrases are present on master at **`:4466`** and **`:4492`** (the PLAN's `:4466` is exact; its `:4491-4492` names the doc line pair) and are **absent at HEAD**. ⇒ the PLAN's `manager_test.go` roster is **6 entries, of which 4 were owed**.
2. **`:4561` drifted `+168`, not the `+4…+22` the task brief warned about.** T3 inserted `startOneWayTLSListener` (≈40 lines) and `TestServeConnection_SSLNoCertificateIncrements` (≈50 lines) plus the expanded `assertSSLCrossProduct` doc **above** that anchor. **Any anchor cited from the PLAN below `manager_test.go:4500` is off by well over one screen.** Line-number-driven editing would have landed this change in the middle of an unrelated test.
3. **The stale token is not always "three".** The missed site said *"both"*. Combined with T6's *"(no StatsAsserter)"*, this row has now found **two** stale sites whose token no pattern in the 14-pattern sweep contains. The lesson is not "add `both` to the sweep" — it is that a count-shaped sweep cannot enumerate the words a human uses for a small number.
4. **The Step-3 gate has a blind spot it cannot report on itself.** `grep -i 'counter'` does not match `counted`, so the gate is structurally incapable of surfacing `manager.go:392`, the single most important site in this task. A green Step-3 grep is therefore **necessary but not sufficient**, and this record says so rather than presenting the clean output as closure.
5. **The T8 ledger row is still blank.** `T8`'s Status/Commit cells were never filled even though its `## Task 8 — ACTUAL` section landed at `e6092777`. **Not touched here** (it is another task's record), but named so it is not discovered at T10 as a surprise.

---

## Task 10 — ACTUAL ⚠️ NO SOURCE FILE MODIFIED. TWO PLAN GATE COMMANDS REFUTED BY EXECUTION, BOTH SILENTLY — ONE FAILS OPEN, ONE FAILS CLOSED

**Commit:** `e860b817` *(the sha backfill lands as a one-line follow-up commit — a commit cannot cite its own sha)* · **Files:** `docs/envoy-go/phases/75-tls-no-certificate-stat/PROGRESS.md` ONLY (this section + the T8 ledger row backfill). **Zero production, test, fixture or gate files touched.**

### Tripwire — run FIRST, and again before the commit

```
$ pwd
/home/esa/git/envoy-go            # ⚠️ the Bash cwd IS the main checkout; every command below is `cd $W && …` or `git -C $W …`
$ git -C /home/esa/git/envoy-go-wt-p75impl rev-parse --abbrev-ref HEAD
phase-75-impl
$ git -C /home/esa/git/envoy-go-wt-p75impl rev-list --count master..HEAD
16
```

⚠️ `reference_bash_cwd_reset_commits_to_main` **fired as a live condition, not a hypothetical**: `pwd` resolves to `/home/esa/git/envoy-go` (master) for the whole task. Every gate below was executed inside `$W` via an explicit `cd`, and every git command via `git -C $W`. A bare `go test` would have validated **master's pre-row tree** and gone green while proving nothing.

### Step 1 — the six-gate. **ALL SIX SILENT.**

| gate | command | ACTUAL |
|---|---|---|
| 1 | `gofmt -l .` | *(no output)* — `EXIT=0` |
| 2 | `go vet ./...` | *(no output)* — `VET_EXIT=0` |
| 3 | `go build ./...` | *(no output)* — `BUILD_EXIT=0` |
| 4 | `go mod tidy -diff` | *(no output)* — `TIDY_EXIT=0` (EMPTY) |
| 5 | `git -C $W diff master -- go.mod go.sum` | *(no output)* — `GOMOD_DIFF_EXIT=0` (EMPTY) |
| 6 | `golangci-lint run ./...` | *(no output)* — `LINT_EXIT=0`, **0 bytes** on stdout+stderr |

### Step 2 — the full differential. **RUN ONE FAILED. Diagnosed, isolate-re-run, and the FULL SUITE re-run CLEAN.**

**Run 1** — `go test ./test/differential/ -count=1 -v`:

```
--- FAIL: TestDifferential (396.82s)
    --- FAIL: TestDifferential/0083-grpc-access-log-headers (5.03s)
FAIL	github.com/pgdad/envoy-go/test/differential	400.802s
```
tally: **119 RUN / 118 PASS / 1 FAIL / 0 SKIP**.

The failure body, verbatim — and it is **not an assertion**:

```
=== RUN   TestDifferential/0083-grpc-access-log-headers
2026/07/25 16:47:41 listen: listen tcp 0.0.0.0:32867: bind: address already in use
exit status 1
    runner_test.go:342: backend[0] not ready: waitTCPDial: 127.0.0.1:32867 did not become reachable within 5s
```

⚠️ **CLASSIFICATION: a full-suite STARTUP flake — an ephemeral-port bind collision on the harness's own backend, on a fixture UNRELATED to this row.** The evidence, stated rather than asserted:

1. **The proxy never started and no comparison ever ran.** The error is `bind: address already in use` on the *backend* listener, surfaced by `runner_test.go:342`'s readiness wait — it precedes container creation. **Zero assertion output.** This is the same species as the documented `reference_differential_fullsuite_startup_flake` (`subject ready: EOF` on an unrelated fixture), differing only in which end of the readiness handshake lost the race.
2. **Isolate-re-run: PASS.** `go test ./test/differential/ -count=1 -v -run 'TestDifferential/0083-grpc-access-log-headers'` ⇒ `--- PASS: TestDifferential/0083-grpc-access-log-headers (4.03s)` / `ok … 4.094s`. ⚠️ The `-run` selector is proven to have **MATCHED** (`=== RUN   TestDifferential/0083-…` is present) — `reference_differential_run_selector`, whose no-match form prints `[no tests to run]` and exits 0.
3. **Structural non-causation.** `0083-grpc-access-log-headers` is a gRPC access-log fixture. This row's entire production delta is `internal/listener/manager.go` (+1 field, +1 registration, +5 guarded `Inc`) and `internal/stats/name.go` (+1 `helpText` entry). `internal/filter/**` and every access-log path are on the **sha256 BYTE-UNTOUCHED roster and verified 0-mismatch** at Step 5.
4. ⚠️ **PLAN hazard 12 says a failure here is "more likely real than at most stages" — that warning was HEEDED, not waved away.** The classification rests on (1)–(3) plus (5), not on "it's a known flake".

**Run 2** — the FULL suite re-run, `go test ./test/differential/ -count=1 -v`:

```
ok  	github.com/pgdad/envoy-go/test/differential	396.517s
```
tally: **119 RUN / 119 PASS / 0 FAIL / 0 SKIP** — `DIFF2_EXIT=0`. **This is the PLAN's expected result, reproduced exactly.** (5) `0083` passed in run 2 without any change to the tree.

#### The fixture-directory set EQUALS the subtest set — PROVEN, not assumed

```
$ ls -d test/fixtures/[0-9]*/ | sed 's#test/fixtures/##;s#/$##' | sort            # 119 lines
$ grep -oE '^=== RUN   TestDifferential/[^ ]+' <run-log> | sed 's#.*/##' | sort   # 119 lines
$ comm -3 <fixdirs> <subtests>
                                  # ← EMPTY
left-only:  0
right-only: 0
both:       119
```

⇒ **no fixture directory is un-driven and no subtest lacks a directory.** Combined with **0 SKIP**, no fixture was silently skipped for a missing driver registration.

#### ⚠️ The row's OWN new cross-side assertion is proven LIVE IN THE FULL SUITE, not merely "0110 passed"

From run 2's log, at `0110`:

```
2026/07/25 16:56:58 listener "l_rccf": handshake: tls: failed to verify certificate: x509: certificate signed by unknown authority
2026/07/25 16:56:58 0110 AssertStats: reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
2026/07/25 16:56:58 0110 AssertStats: subject   ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
```

Both sides printed, both agreeing, **`ssl.no_certificate=1` against `ssl.handshake=2`** — the discriminating non-zero. A green fixture alone could not distinguish "the asserter ran" from "the asserter was never dispatched" (`reference_differential_asserter_dispatch`, and Break J at T5); these lines settle it inside the *full* suite, not only under a `-run` selector.

### Step 2 (cont.) — `-race`. ⚠️ **RUN, NOT SKIPPED — and it COVERS `./test/differential/`.**

`go test ./... -count=1 -race` — **the differential package is inside `./...`, so PLAN §1.4's outstanding item is DISCHARGED, not deferred.**

```
RACE_EXIT=1
WARNING: DATA RACE   ⇒  0 occurrences across the whole run
ok packages:       124
[no test files]:     5
FAIL packages:       1   →  FAIL  github.com/pgdad/envoy-go/test/differential  403.617s
```

⚠️ **ZERO data races.** The single failure:

```
--- FAIL: TestDifferential (399.50s)
    --- FAIL: TestDifferential/0061-lb-ring-hash (3.18s)
        runner_test.go:1293: distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)
```

⚠️ **CLASSIFICATION: `reference_0061_ring_hash_spread_flake` — and this is its SECOND recorded occurrence.** The memory records exactly one prior occurrence (2026-07-12) and states *"a second → investigate margins"*. **That trigger has now fired.** Evidence for the classification:

1. **The failure text is the memory's, verbatim in shape** — a statistical *spread* assertion (`only 1 backend(s) nonzero, want >= 2`), not a wire, config or count mismatch. It is a distribution margin, the exact failure mode `reference_differential_band_sigma_margin` describes.
2. **`0061` PASSED in BOTH non-race full runs at this same tip** — `--- PASS: TestDifferential/0061-lb-ring-hash (3.12s)` in run 1 and `(3.16s)` in run 2.
3. **FOUR isolate re-runs UNDER `-race`, all PASS**: three bare (`ok … 4.563s / 4.444s / 4.583s`) and one `-v` that proves the selector matched — `=== RUN   TestDifferential/0061-lb-ring-hash` → `--- PASS … (3.36s)` / `ok … 4.456s`.
4. **Structural non-causation, gate-backed.** `ring_hash` lives in `internal/cluster/**`, which is on the **sha256 BYTE-UNTOUCHED roster and verified 0-mismatch**. This row registers and Incs one listener-scope TLS counter; it has no LB code path.

⚠️ **NOT smoothed over: this is a real follow-on owed to the project, outside this row.** It is recorded here rather than absorbed, per `reference_a_drift_correction_is_itself_a_claim`. **It was NOT on the PLAN's known-flake list** (hazard 12 enumerates six; this is a seventh).

⚠️ **NONE of the PLAN's six enumerated known flakes fired** — `grep` over the `-race` log for `TestOutlierDetector_ConcurrentEjectExactlyOnce`, `TestSDSEndToEnd_FetchFailure_BootFailsClosed`, `TestOptions_ZeroValue_NoOpDefaults`, `TestServerConn_TinyWindowDelivery`, `init_fetch_timeout` and `subject ready: EOF` returns **nothing**.

⚠️ **One honest limit on the `-race` differential result:** the differential took **403.6 s** under `-race` versus **396.5 s** without it — essentially unchanged. That is consistent with the run being dominated by container startup and I/O rather than by instrumented Go, so **"the differential ran under `-race`" is a true statement about the harness process; it is NOT evidence of deep race coverage of the subject's hot paths.** Stated so the figure is not over-read.

### Step 3 — the envelope, in TWO SEPARATE CATEGORIES

#### ⚠️ REFUTATION 1 — the PLAN's own Step-3 import gate CANNOT PASS AS WRITTEN. It fails CLOSED.

The pinned `impblock` prints `FILENAME"\t"$0`. The baseline is extracted to `/tmp/p75base/…` and HEAD is read at `internal/…`, so **every line differs on its path prefix**. Run literally, the gate emits a **31-line-removed / 31-line-added diff and exits 1 with a +0-import delta**:

```
$ diff -u /tmp/p75.base /tmp/p75.head && echo "GATE PASS: +0 PRODUCTION imports"
-/tmp/p75base/internal/listener/manager.go	bootstrapv3 "…/bootstrap/v3"
…
+internal/listener/manager.go	bootstrapv3 "…/bootstrap/v3"
…
DIFF_EXIT=1        # ← and NO "GATE PASS" line
```

**BASE=31 lines, HEAD=31 lines — the import sets are identical; 100% of the diff is path noise.** ⚠️ This is the mirror image of the defect RD-IMPGATE was written to fix: the phase-74 command **failed open** (hits + exit 0); the phase-75 replacement **fails closed** (exit 1 on a clean tree). Note the PLAN's own negative-control transcript prints `+manager.go\t"strconv"` — with **no path prefix** — so the control was demonstrably run against a *different*, basename-normalised form than the one the PLAN pinned. **`reference_quoting_is_not_executing`.**

**Corrected gate** — identical extraction, second `awk` variable carrying the basename:

```
$ impblock() { awk -v n="$2" '/^import \($/{f=1;next} f&&/^\)$/{exit} f&&NF{gsub(/^[ \t]+/,"");print n"\t"$0}' "$1"; }
BASE=31  HEAD=31
$ diff -u /tmp/p75.base2 /tmp/p75.head2 && echo "GATE PASS: +0 PRODUCTION imports (exit 0)"
GATE PASS: +0 PRODUCTION imports (exit 0)
```

**CATEGORY 1 — PRODUCTION imports: +0.** `internal/listener/manager.go` holds 27 imports at master and 27 at HEAD (`stdtls "crypto/tls"` and `github.com/pgdad/envoy-go/internal/stats` both already present); `internal/stats/name.go` holds 4 and 4.

**The NEGATIVE CONTROL, RE-RUN HERE — on a scratch copy under `/tmp`, never in `$W`:**

```
$ cp internal/listener/manager.go /tmp/p75neg/manager.go     # + insert "strconv" above "strings"
$ diff -u /tmp/p75.base2 /tmp/p75.neg2
 manager.go	stdtls "crypto/tls"
+manager.go	"strconv"
 manager.go	"strings"
NEGATIVE CONTROL OK: gate exit=1 and printed the added line
```

⇒ **the green above is EVIDENCE, not silence.** RD-IMPGATE's map-literal-immunity check also re-derived: `impblock internal/stats/name.go | grep -c envoy_listener_ssl` ⇒ **0**.

#### **CATEGORY 2 — TEST-side imports GREW, and that is PERMITTED.** Verified mechanically, per file:

| file | ADDED | REMOVED |
|---|---|---|
| `test/fixtures/0110-…/driver/driver.go` | **`"log"`, `"math"`, `"net/http"`, `"strconv"` — exactly the four the PLAN names, no more** | *(none)* |
| `internal/listener/manager_test.go` | *(none)* | *(none)* |
| `internal/listener/quic_test.go` | *(none)* | *(none)* |
| `internal/stats/name_test.go` | *(none)* | *(none)* |

⚠️ **These are TEST imports in a driver package. They are NOT a production-envelope violation and this record refuses to let them read as one.** Category 1 is `+0`; Category 2 is `+4`, all in one fixture driver, all in `stdlib`.

#### ⚠️ REFUTATION 2 — the PLAN's `go doc` gate SILENTLY DROPS HALF ITS SCOPE. It fails OPEN.

```
$ go doc -all ./internal/listener ./internal/stats > /tmp/p75.doc.literal
EXIT=0    stderr: (empty)    stdout: 8024 bytes
$ go doc -all ./internal/listener | wc -c     8024
$ go doc -all ./internal/stats    | wc -c    14192
```

`go doc` takes `<pkg> [symbol]`, not two packages. **The literal command produces `./internal/listener`'s doc ALONE — byte-for-byte, 8024 == 8024 — and `./internal/stats` never appears in the output, with no error and exit 0.** The gate would have gone green over `internal/stats` **without ever reading it**, in the very task whose second production file is `internal/stats/name.go`. ⚠️ **A gate that exits 0 while silently covering half its roster is worse than no gate.** Run PER PACKAGE:

| package | master baseline | HEAD | verdict |
|---|---|---|---|
| `./internal/listener` | 8024 bytes | 8024 bytes | `diff` ⇒ **IDENTICAL** |
| `./internal/stats` | 14192 bytes | 14192 bytes | `diff` ⇒ **IDENTICAL** |

⇒ **ZERO new exported symbols.** Consistent with the delta by construction: `sslNoCertificate` is an **unexported struct field** and `helpText` is an **unexported package-level map**.

The master baseline was derived from a **separate `git worktree add --detach /tmp/p75master master`** — `9f5d667b`, confirmed equal to `git -C /home/esa/git/envoy-go rev-parse master`. ⚠️ **The PLAN's `git stash` / `git stash pop` form was NOT used**: on a worktree with 16 commits ahead and a clean tree, `git stash` stashes **nothing** and the "baseline" would have been HEAD — a vacuous `diff` of a file against itself. The PLAN's own parenthetical offers the worktree route; it is the only correct one here.

```
$ diff /tmp/p75.deps.base /tmp/p75.deps.head && echo "IDENTICAL - no new dependency edge"
IDENTICAL - no new dependency edge          # ./internal/listener  439 == 439 packages
$ diff /tmp/p75.deps.stats.base /tmp/p75.deps.stats.head
IDENTICAL - no new dependency edge          # ./internal/stats      67 ==  67 packages
```

⇒ **no new dependency edge**, in either direction, in either package. *(`./internal/stats` was audited too, though the PLAN's line names only `./internal/listener` — the second production file deserved the same test.)*

### Step 3 (cont.) — the SHAPE gate on the four EDITED-not-sha256-gated test files (Global Constraints `:242`)

| file | `func Test*` at master → HEAD | delta |
|---|---|---|
| `internal/listener/manager_test.go` | 93 → **94** | **−1 renamed:** `…RegistersExactlyThreeSSLNames` → `…RegistersExactlyFourSSLNames` (T1's documented rename, its third cross-reference recorded there) · **+1 new:** `TestServeConnection_SSLNoCertificateIncrements` |
| `internal/listener/quic_test.go` | 4 → 4 | **zero** — additive-only inside an existing `want` set |
| `internal/stats/name_test.go` | 55 → 55 | **zero** |
| `test/fixtures/0110-…/driver/driver.go` | 0 → 0 | **zero** — a driver package, no test funcs by design |

⇒ **no pre-existing test was deleted or silently renamed away.** The single rename is the one T1 declared, and `go test` green (Step 2) closes the shape gate the Global Constraints asked for.

### Step 4 — counts, RE-RUN MECHANICALLY (every figure below is this task's own output, not copied from RD-BASELINE)

| # | command | ACTUAL | expected | verdict |
|---|---|---|---|---|
| 1 | `ls -d test/fixtures/[0-9]*/ \| wc -l` | **119** | 119 (+0) | **HOLDS** |
| 2 | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | **55** | 55 (+0) | **HOLDS** |
| 3 | `grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go` | `606:` (doc) and **`614:	H2GoawayResponder BackendKind = 38`** | tail VALUE 38 | **HOLDS** |
| 4 | `grep -cE '^\s+[a-z]' go.mod` | **67** | 67 | **HOLDS** |
| 5 | `grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md \| tail -1` | **`## ADR-0297`** | ADR-0297 | **HOLDS** |
| 6 | `grep -c '^## ADR-0298' docs/envoy-go/DECISIONS.md` | **0** | 0 | **HOLDS** |

Corroborating: highest numeric fixture directory is **`0117-tracing-custom-tags-metadata-cluster`** (`0118` absent), matching RD-BASELINE's `0118`-absent note. Figure #3 is a **TAIL VALUE**, not a count — the file declares 39 constants, `0`–`38`.

#### ⚠️ Stat surface **1205** — ASSERTED, NOT MEASURED. Say it plainly.

**There is NO mechanical command for the absolute total and none was invented here.** What this task can and does verify is only that the document is internally consistent with what T8 landed: `grep -c '1205'` over `BEHAVIOR_CONTRACT.md` ⇒ **3** — `:831`, `:847` (the two bare narrative totals) and `:5004` (the new ledger line, which opens `1204 → 1205 (+1)`).

- **The `+1` DELTA is sound and mechanically checkable** — exactly one name (`listener.<addr>.ssl.no_certificate`) is registered by this row, pinned by T1's two exact-set name tests and T4's `wantNames`.
- **The absolute `1205` is DOCUMENTARY.** It inherits an arithmetic chain known to be discontinuous in **TWO** places, both recorded at T8 `:5004` and neither back-filled: the unattributed `1200 → 1201` step phase 74 flagged, and the `46.1b` closes-at-**1198** / `47.1` opens-at-**1200** unattributed `+2` found at this PLAN.
- ⚠️ **A future phase needing an authoritative total must RE-DERIVE it mechanically and should expect the answer to disagree with 1205.**
- The four stale `internal/statssink` prose sites (*"stays 1200 / 1196"*) remain, **deliberately** (PLAN §5, T9's §2.5 disposition): that package is on the byte-untouched roster and is verified 0-mismatch at Step 5 below. **Named, not fixed, and not quietly dropped.**

### Step 5 — the BYTE-UNTOUCHED roster, **SET-DIFFERENCED AGAINST THE EDIT ROSTER FIRST** (`reference_plan_schedules_edits_to_a_byte_gated_file`)

**The EDIT roster, derived mechanically — `git -C $W diff master --name-only` ⇒ 13 files:**

```
docs/envoy-go/BEHAVIOR_CONTRACT.md
docs/envoy-go/phases/75-tls-no-certificate-stat/PROGRESS.md
internal/listener/manager.go
internal/listener/manager_test.go
internal/listener/quic_test.go
internal/stats/name.go
internal/stats/name_test.go
test/fixtures/0110-tls-require-client-cert-false/README.md
test/fixtures/0110-tls-require-client-cert-false/driver/driver.go
test/fixtures/0110-tls-require-client-cert-false/envoy.yaml
test/fixtures/0110-tls-require-client-cert-false/expectations.yaml
test/fixtures/0111-tls-cvc-empty-dynamic-fallback/README.md
test/fixtures/0111-tls-cvc-empty-dynamic-fallback/expectations.yaml
```

**The GATE roster, expanded mechanically from the Global-Constraints globs ⇒ 592 files.**

```
$ comm -12 <EDIT sorted> <GATE sorted>
                                   # ← EMPTY. No scheduled edit lands on a byte-gated file.
```

**Targeted confirmations, exactly as the PLAN demands:**

| file | in EDIT roster | in GATE roster |
|---|---|---|
| `internal/listener/manager_test.go` | **1** | **0** |
| `internal/listener/quic_test.go` | **1** | **0** |
| `internal/stats/name_test.go` | **1** | **0** |
| `test/fixtures/0110-…/driver/driver.go` | **1** | **0** |
| `test/fixtures/0111-…/README.md` *(prose)* | **1** | **0** |
| `test/fixtures/0111-…/expectations.yaml` *(prose)* | **1** | **0** |
| `test/fixtures/0111-…/driver/driver.go` | **0** | **1** |
| `test/fixtures/0110-…/envoy-go.yaml` | **0** | **1** |

**All eight as specified.** ⚠️ **One near-collision, named because it is one character from being the phase-73 SEVERE-1:** `test/fixtures/0110-…/**envoy.yaml**` IS edited (T6, comment-only: 1 line → 5, ZERO config bytes) while `test/fixtures/0110-…/**envoy-go.yaml**` is the gated one. Two different files whose names differ by three characters, on opposite sides of the same roster. The set difference is genuinely empty — but **Global Constraints `:242`'s "EDITED, not gated" list names only four test files and does NOT name `0110/envoy.yaml`, `0110/README.md`, `0110/expectations.yaml`, `0111/README.md` or `0111/expectations.yaml`.** The `:242` list is **incomplete as an EDIT roster**; only the mechanically derived `git diff --name-only` set is authoritative. Recorded so a future reader does not treat `:242` as exhaustive.

**The sha256 gate, run AFTER the set difference:**

```
FILES GATED: 592
MISMATCHES:  0
MISSING:     0
```

⚠️ **`MISSING: 0` is a distinct assertion from `MISMATCHES: 0`** — it proves no gated file was *deleted* (a deletion would otherwise read as "no mismatch"). Roster coverage as executed: `internal/listener/quic.go` · `internal/listener/listenerfilter/**` · `internal/tls/**` · `internal/xds/**` · `internal/boot/**` · `internal/bootstrap/**` · `internal/tracing/**` · `internal/cluster/**` · `internal/filter/**` · `internal/statssink/**` · `validate/**` · `cmd/**` · `go.mod` · `go.sum` · `0110-…/envoy-go.yaml` · `0111-…/driver/driver.go`.

### Task 10 — the whole gate sheet, as one table

| gate | result |
|---|---|
| `gofmt -l .` | SILENT |
| `go vet ./...` | SILENT |
| `go build ./...` | SILENT |
| `go mod tidy -diff` | EMPTY |
| `git diff master -- go.mod go.sum` | EMPTY |
| `golangci-lint run ./...` | SILENT (0 bytes) |
| full differential, run 1 | **FAIL** — `0083`, port-bind startup flake; 118/119 |
| `0083` isolate re-run | **PASS** (selector proven to match) |
| full differential, run 2 | **ok 396.517s — 119 RUN / 119 PASS / 0 FAIL / 0 SKIP** |
| fixture-dir set ≡ subtest set | `comm -3` **EMPTY**; 119 ≡ 119 |
| `0110` cross-side asserter live in the full suite | **both sides printed, `no_certificate=1` agreeing** |
| `go test ./... -count=1 -race` (**includes the differential**) | **0 DATA RACE**; 124 ok pkgs; 1 FAIL = `0061` ring-hash spread |
| `0061` isolate re-run under `-race` | **PASS ×4** (three bare + one `-v` proving the selector matched) |
| production imports (Category 1) | **+0**, with the negative control RE-RUN and FIRING |
| test imports (Category 2) | **+4 in one driver** (`log`, `math`, `net/http`, `strconv`) — PERMITTED |
| exported symbols, `internal/listener` | **IDENTICAL** to master |
| exported symbols, `internal/stats` | **IDENTICAL** to master |
| `go list -deps` edges, both packages | **IDENTICAL** (439 ≡ 439, 67 ≡ 67) |
| shape gate, 4 edited test files | one DECLARED rename + one new test; **nothing removed** |
| counts (6 mechanical) | **all six HOLD** |
| stat surface 1205 | **NOT MEASURED — delta asserted, total documentary, two ledger gaps** |
| EDIT ∩ GATE roster | **EMPTY** (13 vs 592) |
| sha256 byte-untouched gate | **592 files / 0 mismatches / 0 missing** |

### Task 10 — surprises

1. ⚠️ **TWO of the PLAN's own Step-3 gate commands are wrong, and they are wrong in OPPOSITE directions.** The import gate **fails closed** (31-line diff, exit 1, on a `+0` tree — the `FILENAME` prefix); the `go doc` gate **fails open** (exit 0 while silently reading only the first of its two packages). The first would have been caught by anyone who ran it; **the second would not have been — it goes green.** RD-IMPGATE was authored specifically to replace a gate that failed open, and its replacement introduced a fail-closed defect one line later. **Both were found by EXECUTION; neither is visible by reading.**
2. ⚠️ **The PLAN's negative-control transcript proves the control was run against a DIFFERENT command than the one pinned.** It reports `+manager.go\t"strconv"` — basename, no path — which the pinned `FILENAME` form cannot emit for the `/tmp/p75base/…` baseline. The control was real; the *command it validated* is not the command the PLAN wrote down. `reference_quoting_is_not_executing`, in its sharpest form yet this row.
3. ⚠️ **The PLAN's `git stash` baseline would have been VACUOUS here.** With a clean 16-commits-ahead worktree, `git stash` stashes nothing, `stash pop` errors, and the "before/after" `go doc` diff compares HEAD against HEAD — **guaranteed green, proving nothing**. The parenthetical fallback (a separate `git worktree add` of master) is not an alternative, it is the only correct route on this tree.
4. ⚠️ **A SEVENTH flake, not on the PLAN's list of six, and its escalation trigger has fired.** `0061-lb-ring-hash` failed **once, under `-race`, in the full suite**, on a distribution-spread margin — `reference_0061_ring_hash_spread_flake`, whose memory says *"one occurrence (2026-07-12); a second → investigate margins."* **This is that second occurrence.** Isolate-re-run PASS ×4 under `-race` and PASS in both non-race full runs; `internal/cluster/**` is sha256-verified byte-untouched, so this row cannot be the cause. **Investigating the σ-margin is a real follow-on the project now owes** (`reference_differential_band_sigma_margin`) — recorded, not absorbed.
5. **The `-race` differential result must not be over-read.** 403.6 s under `-race` versus 396.5 s without is ~2% — the run is dominated by container startup, so the honest claim is *"the harness process ran instrumented and reported no race"*, **not** *"the subject's hot paths are race-swept"*. PLAN §1.4's item is discharged; the coverage it buys is modest, and saying so is the point.
6. **Global Constraints `:242`'s EDIT roster is INCOMPLETE.** It names four test files; the tree's actual delta is 13 files, five of them fixture prose the `:242` list never mentions. The set difference is still empty — but only because the mechanically derived roster was used. **Trusting `:242` as the EDIT roster is exactly the phase-73 SEVERE-1 shape**, and `0110/envoy.yaml` (edited) sits three characters from `0110/envoy-go.yaml` (gated).
7. **Run 1's `0083` failure is a reminder that "the full suite is green" is a claim with a denominator.** The PLAN predicted `119 PASS / 0 FAIL` and run 2 delivered exactly that — but run 1, same tree, same commands, did not. **Both runs are recorded.** Reporting only run 2 would have been true and misleading.
---

## Task 11 (DECISIONS + ROADMAP) — ACTUAL ⚠️ THE ADR'S OWN ¶7 AND ¶9 WERE BOTH DEFECTIVE, AND THE ROW-75 CELL CARRIED FIVE STALE CLAIMS, NOT FOUR

**Commit:** *"phase 75 T11: ROW 75 -> done. ADR-0297 completed IN PLACE and CORRECTED (its own para7 self-falsifying grep + para9's refuted form rule), the ADR-0296 (g) blockquote, the ROADMAP row flip + FOUR stale claims + the narrow"* — **`6580063b`** (three files: `DECISIONS.md`, `ROADMAP.md`, and this record; +284/-5).

**Scope executed here:** Steps 1-5 and Step 8 — `docs/envoy-go/DECISIONS.md` and `docs/envoy-go/ROADMAP.md`, **exactly two files**. `STATE.md` and `next-prompt.txt` are the controller's (Step 6) and were **NOT touched**; the sentinel's second mechanical run against landed master (Step 7) is the controller's too.

```
$ git -C /home/esa/git/envoy-go-wt-p75impl diff --name-only
docs/envoy-go/DECISIONS.md
docs/envoy-go/ROADMAP.md
$ git -C /home/esa/git/envoy-go-wt-p75impl diff --stat
 docs/envoy-go/DECISIONS.md | 48 +++++++++++++++++++++++++++++++++++++++++++---
 docs/envoy-go/ROADMAP.md   |  4 ++--
```

### Step 1 — ADR-0297 completed IN PLACE, and its TWO OWN defects corrected

- **STATUS blockquote** (`:17324` → `:17326` post-insert): `PROPOSED` → **`COMPLETE`**, future → past tense, and it now names the two in-place §Context corrections so a reader meets them before the paragraphs.
- **§Decision + §Consequences APPENDED AFTER the RETAINED footer** — the ADR-0295/0296 shape, **not** ADR-0286's. Footer `*(§Decision + §Consequences land at the phase-75 IMPL.)*` survives at `:17352`; `### Decision (landed at the phase-75 IMPL)` opens at `:17354`.
- **No renumber.** DECISIONS tail is still `## ADR-0297`; `^## ADR-0298` is 0.
- **§Context ¶1-¶11 were NOT duplicated** (RD-ADR0297). §Decision records only what the row DID; §Consequences only what EXECUTED.

#### ⚠️ FIX ¶7 — the self-falsifying grep claim (F5), CONFIRMED as stated

Pre-edit, `VERIFYIFGIVEN` occurred on **TWO lines / THREE occurrences** in `DECISIONS.md` — `:17308` ×1 (ADR-0296 §Decision (g)) and `:17340` ×2 (¶7 itself: once quoting the deferral sentence, once inside the grep claim) — while ¶7 asserted *"exactly ONE hit — `:17308`, the citing sentence itself."* **¶7's own text made ¶7 false.**

**The fix taken:** (a) the token is **ELIDED** from the quoted deferral sentence with a bracketed editorial note, (b) the property is **restated SCOPED to ADR-0296's own block and with NO COUNT**, (c) an inline `**[CORRECTED at the phase-75 IMPL: …]**` records the defect, names it as the same species the phase-74 IMPL had just fixed in ADR-0296 ¶3, and states the reusable generalisation — **a whole-file grep COUNT asserted INSIDE the file it greps is self-falsifying by construction.**

**Post-edit, MEASURED (the correction did not mint a counter-example):**

```
$ python3 -c "…count 'VERIFYIFGIVEN' per line…" docs/envoy-go/DECISIONS.md
17308 1
TOTAL 1
```

⇒ the token now occurs **only** in the ADR-0296 sentence it belongs to. Neither the ¶7 correction, nor the ¶9 correction, nor the new §Decision/§Consequences, nor the ADR-0296 blockquote spells it.

#### ⚠️ FIX ¶9 — the refuted form rule (F6). **THE n=7 POPULATION WAS RE-DERIVED HERE, NOT COPIED.**

`reference_a_drift_correction_is_itself_a_claim` — this rule had already failed twice, so the population was re-enumerated mechanically before anything was written down. Every `CORRECTED` occurrence in the file was listed and each was inspected in context:

| form | n | instances (owning ADR / correcting phase) | self/other |
|---|---|---|---|
| indented `  > [CORRECTED at phase N/ADR-XXXX: …]` | **2** | `:16901` (in ADR-0286 / phase 67-ADR-0289), `:16910` (in ADR-0286 / phase 74-ADR-0296) | both **OTHER** |
| inline **bold** `**[CORRECTED at the phase-N IMPL: …]**` | **3** | `:17187` (in ADR-0294 / phase 72 = SELF), **`:17211` (in ADR-0294 / phase 73 = OTHER)**, `:17272` (in ADR-0296 / phase 74 = SELF) | 2 SELF, **1 OTHER** |
| inline *italic* `*(corrected at the phase-N IMPL from "…")*` | **2** | `:17213` ×2 (in ADR-0295 / phase 73) | both SELF |
| | **n=7** | | |

**Excluded as NON-instances, each inspected rather than assumed:** `:14355` (*"D-S28.2-1 CORRECTED"*, prose) · `:16469`/`:16471`/`:16475` (*"a CORRECTED formula"*, an ordinary adjective in the priority-LB ADR) · `:17193` and `:17252` (§Decision recaps) · `:17268` (a **TEMPLATE** describing the two forms) · `:17258` and `:17324` (STATUS forward-pointers).

**`:17211` is the counter-example that kills SELF-vs-OTHER — verified by bounds, not by assertion:** ADR-0294's heading is `:17175` and ADR-0295's is `:17213`, so `17175 < 17211 < 17213` places it **inside ADR-0294** (phase 72's ADR), and its text reads `**[CORRECTED at the phase-73 IMPL: the closing clause is REFUTED …]**` — a **later phase correcting a DIFFERENT, already-landed ADR, rendered INLINE.**

**The surviving discriminator is graft SCALE**, and the attachment was checked: `:16901` follows the bullet at `:16899` and `:16910` follows the C3 bullet at `:16908` — **both stand alone beneath a WHOLE ADR-0286 bullet they re-characterise.** ⇒ the **INDENTED form stays correct for phase 75**; only ¶9's REASON was replaced. ¶9's subsidiary claims were re-verified and **DO hold**: `:17209` is ADR-0294's `Documented boundaries` bullet and **is not a correction at all**, and the phase-74 blockquote sits at `:16910`, not immediately after `:16908`.

⚠️ **Recorded in ¶9 itself: the prescription has now survived THREE different wrong justifications — family, then self-vs-other, now scale** — and scale is stated as *the discriminator n=7 does not refute*, which is weaker than *the discriminator n=7 proves*.

⚠️ **One drafting hazard avoided:** ¶9's original text called `:16901`/`:16910` *"the only two instances in the file."* That clause would have been **falsified by this very row's own blockquote**, which makes three. It was replaced rather than carried forward — the same species as the ¶7 defect, one clause away from being reproduced a third time.

### Step 2 — the ADR-0296 §Decision (g) correction blockquote

Inserted after `:17308` and its existing blank `:17309`, with a **NEW blank line** after it because a `###` heading follows. It **LEADS WITH WHAT SURVIVES** (no counter was owed *in phase 74*; `0111` is `require_client_certificate: true` on both sides, so a success-path annotation reads 0 on every arm; **the deferral was RIGHT and phase 74's row was correctly scoped**), then records that the stated REASON fails on both halves, then separately notes the `registry.go:107` mis-cite and the dangling `B5/B6` pointer. **It claims NO ORDINAL** and it **does not spell the all-caps token or restate any whole-file count.**

**Leading bytes, `od -c`-verified against BOTH pre-existing instances:**

```
$ python3 -c "…first 4 bytes of lines 16901, 16910, 17310…"
16901 b'  > '
16910 b'  > '
17310 b'  > '
identical: True
```

⇒ `0x20 0x20 0x3E 0x20` on all three.

**The two live claims RE-VERIFIED before they were written down** (`feedback_brief_citations_not_evidence`):

| claim | command | ACTUAL | verdict |
|---|---|---|---|
| `internal/tls/config.go:79-84` returns `VerifyClientCertIfGiven` | read `:58-90` | `:79 clientAuthFor := func(require bool) stdtls.ClientAuthType {` · `:81 return stdtls.RequireAndVerifyClientCert` · **`:83 return stdtls.VerifyClientCertIfGiven`** · `:84 }`; the three-way doc is `:60-68`; `installPool` opens `:90` | **HOLDS** |
| ROADMAP row 67 is `done` | `grep -n '^\| 67 '` | `129:\| 67 \| tls-require-client-cert-false \| 66 \| **done** \|` | **HOLDS** |

**Also re-derived rather than inherited:** `internal/stats/registry.go:107` is `panic(… "duplicate metric registration: %q" …)` inside the `if _, dup := r.byName[name]` branch, and the **INVALID-NAME** panic is **`:117`**, inside `checkName` (`:115-118`). The ADR-0296 ¶7 mis-cite is real and is left uncorrected in place, noted in the blockquote.

### Step 3 — the citation fixups the insert forces (F8). **RE-DERIVED POST-INSERT, NOT ASSUMED `+2`.**

```
$ grep -n '^### Consequences (landed at the phase-74 IMPL)' docs/envoy-go/DECISIONS.md
17312:### Consequences (landed at the phase-74 IMPL)
$ grep -n 'The named departure that REMAINS' docs/envoy-go/DECISIONS.md
17316:- **The named departure that REMAINS** (BEHAVIOR_CONTRACT B5/B6) …
$ grep -n '^## ADR-0297' docs/envoy-go/DECISIONS.md
17324:## ADR-0297 — the downstream TLS `ssl.no_certificate` …
```

| old | **NEW (measured)** | cited from | disposition |
|---|---|---|---|
| `:17310` | **`:17312`** | phase-75 `SPEC.md` | **frozen — NOT rewritten** |
| `:17314` | **`:17316`** | `ROADMAP.md:137` | **FIXED here** |
| `:17314` | **`:17316`** | `STATE.md:48` | **controller's — left untouched, reported** |
| `:17314` | **`:17316`** | phase-75 `SPEC.md`, `BRAINSTORM.md` | **frozen — NOT rewritten** |
| `:17322` | **`:17324`** | (ADR-0297's own heading) | informational |
| `:17308`/`:17304`/`:17274`/`:16901`/`:16910` | unshifted | — | precede the insert |

⚠️ **The frozen stage documents (`SPEC.md`, `BRAINSTORM.md`) are DELIBERATELY NOT REWRITTEN**, stated here rather than left ambiguous: no written convention requires it and the lineage treats a landed stage document as a historical record. `STATE.md:48` is the controller's edit and is reported to it rather than made here.

### Step 4 — ROADMAP row 75: the flip + the stale claims. **FIVE, not four.**

```
$ sed -n '137p' docs/envoy-go/ROADMAP.md | cut -c1-60
| 75 | tls-no-certificate-stat | 74 | done |  | **BRAINSTORM
```

| # | stale claim | disposition |
|---|---|---|
| (i) | *"(the discriminator for that form is the **ADR FAMILY, not the phase gap**)"* | **REPLACED** with the n=7 / three-form enumeration and **graft SCALE** — and explicitly noting that **self-vs-other was ALSO refuted**, so the replacement is not the SPEC's replacement either |
| (ii) | *"This is the **THIRD** internal mis-pointer in ADR-0296"* | **NO ORDINAL claimed.** Restated as *not established*, keeping only the two verifiable phase-74 fixes as parenthetical fact |
| (iii) | *"`ProbeAdmin` at `:552`"* | **→ `:558`** — see below; **both** the cell's `:552` and the PLAN's corrected `:554` are stale |
| (iv) | *"all eight `B5` hits are `AMEND-B5`/phase-25.2 Wasm"* | **narrowed:** all eight ARE `AMEND-B5`, but the Wasm gloss is over-broad — SEVEN are phase-25.2 Wasm and `:4685` is phase-29.1 mongo. Conclusion unaffected |
| **(v)** | ⚠️ **`grep -n 'VERIFYIFGIVEN' … returns exactly ONE hit … the token exists nowhere else in the file`** | **NOT on the task brief's list of four, but F5 names `ROADMAP.md:137` explicitly.** The claim was FALSE at this tip. Restated **scoped, with no count, token not spelled** |

**(iii) RE-DERIVED at the IMPL tip, and the PLAN's own correction is refuted:**

```
$ grep -n 'ProbeAdmin' test/fixtures/0110-tls-require-client-cert-false/driver/driver.go
556:// ProbeAdmin issues GET /ready against each proxy's admin endpoint for the
558:func (*rccfDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
$ wc -l test/fixtures/0110-tls-require-client-cert-false/driver/driver.go
843 …
```

⇒ declaration **`:558`**, doc comment `:556-557`. **The PLAN's `:554` was correct pre-IMPL and is stale post-T5**, exactly as the task brief warned. `subjAdminPort` also drifted: the cell's `:313` is now `:317`, left as-is because it was not on the fix roster.

**(iv) RE-DERIVED:** 12 `B5` occurrences over 9 lines. Four of them are on `:1855` itself — the phase-74-landed note that says *"there is NO B-numbered step scheme anywhere in this file"* — leaving **eight `AMEND-B5` lines**: `:3955`, `:3960`, `:3970`, `:3982`, `:4051`, `:4074`, `:4151`, **`:4685`**. The nearest heading above `:4685` is **`:4678 ### envoy.filters.network.mongo_proxy`** (ROADMAP row 29.1, `network-filter-mongo-wire-and-requests`), **not** the Wasm section — its text is the *"EXACTLY-7-opcode decode envelope + OP_MSG-not-decoded"* bullet. **F11 CONFIRMED.**

**Plus the Step-3 citation fixup** (`DECISIONS.md:17304`/`:17314` → `:17304`/**`:17316`**, with the reason stated in the cell), and an **IMPL-close paragraph** appended in the phase-74 form: the flip, ADR-0297 completed-and-corrected, the ADR-0296 blockquote, what landed, the cross-side numbers, the inverted discriminating break, the counts, the six-gate, the escalated `0061` flake, and what was NOT verified.

### Step 5 — the deferred-sentence narrow (`ROADMAP.md:205`), **verified in a `/tmp` scratch copy BEFORE landing**

The RETIRED clause was edited; the candidates list was **not** touched — the sentence never named `no_certificate`, so **this is not a name deletion**. **The `three-fifths` ratio was REMOVED, not bumped** (F9): it was already wrong at the phase-74 tip, since phase 74's own `BEHAVIOR_CONTRACT.md:928` enumerates a FOUR-family surviving remainder. The retirement is now an **ENUMERATION**.

```
before: … three-fifths of which phase 74 RETIRED: envoy-go now emits handshake/fail_verify_error/
        fail_verify_no_cert at listener scope, landed by ADR-0296 as an inline add to ONE function …
after:  … whose FIXED-NAME half is now RETIRED BY ENUMERATION rather than by ratio: envoy-go emits
        handshake/fail_verify_error/fail_verify_no_cert at listener scope (ADR-0296, phase 74) and
        no_certificate beside them (ADR-0297, phase 75), FOUR names in all, each an inline add to
        ONE function …
```

**SCRATCH COPY (`/tmp/p75rm.md`), edit applied there FIRST:**

```
$ grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' /tmp/p75rm.md
3
$ grep -oE 'remaining deferred \(not-yet-chartered\) candidates:' /tmp/p75rm.md | wc -l
3
$ grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' /tmp/p75rm.md | tail -1 | rev | cut -c1-20 | rev
ervice`/force-trace.
$ python3 …  match LEN / interior periods / 'three-fifths' count
LEN 1104
interior periods 0
three-fifths in line: 0
```

**THEN the real file, same checks:**

```
$ grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md
3
$ grep -oE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md | wc -l
3
$ grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md | tail -1 | rev | cut -c1-20 | rev
ervice`/force-trace.
$ diff /tmp/p75rm.md docs/envoy-go/ROADMAP.md
(empty — IDENTICAL)
```

⚠️ **The HARD CONSTRAINT holds: interior periods 0 → 0.** Match length `999 → 1104` (the PLAN's scratch trial measured `999 → 1033`; the adopted wording is longer, and the only property that matters — zero interior periods, terminator still `force-trace.` — is unchanged). No `manager.go`, no `internal/…`, no abbreviation, no decimal was introduced.

### Step 8 — ADR shape verification, ACTUAL OUTPUT

```
$ D=docs/envoy-go/DECISIONS.md
$ awk '/^## ADR-0297/,0' $D | grep -c '^### Context'                    1
$ awk '/^## ADR-0297/,0' $D | grep -c '^### Decision'                   1     (was 0)
$ awk '/^## ADR-0297/,0' $D | grep -c '^### Consequences'               1     (was 0)
$ awk '/^## ADR-0297/,0' $D | grep -c '^\*(§Decision'                   1     (footer RETAINED)
$ grep -oE '^## ADR-[0-9]+' $D | tail -1                                ## ADR-0297   (tail UNCHANGED)
$ grep -c '^## ADR-0298' $D                                             0
$ grep -c '^  > ' $D                                                    3     (was 2)
$ awk '/^## ADR-0296/,/^## ADR-0297/' $D | grep -c '^### Decision'       1     (ADR-0296 stays COMPLETE)
```

**ALL EIGHT AS SPECIFIED.**

### The sentinel, run on the real ROADMAP at this tip

```
$ # (1) every ROADMAP row is `done`
$ grep -oE '^\| [0-9.]+ \| [a-z0-9-]+ \| [0-9.,  ]* \| (planned|in-progress|blocked|done[^|]*)' docs/envoy-go/ROADMAP.md \
    | awk -F'|' '{gsub(/ /,"",$5); if ($5 !~ /^done/) print "NOT DONE: row"$2}'
                     ← ⚠️ SILENT. Row 75 was the last non-`done` chartered row.

$ # (2) no family still carries deferred candidates
$ grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md | wc -l
3
$ grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md
3

$ # (3) every WORK family has been opened
NEVER OPENED: gRPC
NEVER OPENED: Runtime
NEVER OPENED: WASM
```

⚠️ **Check (1) went SILENT at this task, for the first time since the phase-74 IMPL** — recorded as the ACTUAL output, not as the predicted one. **Checks (2) and (3) still print, so the TERMINATION SENTINEL does NOT fire and `stop` MUST NOT be created.**

**Check (1)'s blind spot, RE-DERIVED here and not copied** (RD-BLINDSPOT — recorded wrong in two consecutive lineages):

```
data rows 107 range 31 137
matched 103
misses [31, 35, 83, 84]
```

⇒ **107 data rows / 103 matched / FOUR misses** — `:31` row `00` (em-dash in the "after" column), `:35` row `04` (the DOT in slug `http-1.1`), `:83`/`:84` rows `28.1a`/`28.1b` (the LETTER suffix). All four are `done`, so there is no current impact — **and row 75 at `:137` IS matched**, which is what makes check (1) go silent honestly rather than by a regex miss.

### Task 11 — surprises

1. ⚠️ **The row-75 cell carried FIVE stale claims, not the four on the task brief.** The fifth is the same F5 self-falsifying grep the ADR carried — F5 names `ROADMAP.md:137` explicitly, but Step 4's enumeration listed only four. Fixing four and leaving the fifth would have left a claim that is **false at this tip** in the very cell being closed. Flagged rather than silently absorbed.
2. ⚠️ **The PLAN's own `ProbeAdmin` correction is itself stale.** RD-ROADMAP-75 refuted the cell's `:552` and prescribed `:554`; at the IMPL tip the answer is **`:558`**, because T5 grew that driver to 843 lines. **A drift correction is itself a claim** — this one was true when written and false when consumed, which is exactly the class the ADR's own ¶7 defect belongs to. Three anchors, three different values, one function.
3. ⚠️ **¶9's original text contained a clause this row would itself have falsified** — *"the only two instances in the file"*, about the indented blockquote form. The phase-75 blockquote makes three. It was replaced rather than carried, but had the completion been a pure append with ¶9 left alone, **ADR-0297 would have shipped a SECOND self-falsifying count in the same commit that fixed the first.**
4. **The correction did not mint a counter-example, and that was MEASURED rather than assumed.** Post-edit the all-caps marker occurs exactly once in `DECISIONS.md`, at `:17308`. The elision-plus-descriptive-reference approach cost one bracketed editorial note in each of three places and is the only form that is stable under re-reading.
5. **The `three-fifths` removal is larger than a ratio fix.** Bumping it to "four-fifths" would have propagated a FOURTH wrong denominator — the true partition at the phase-74 tip was 3 retired + 4 surviving = seven, and the `5` is a fossil of a pre-74 two-family count. Stating the retirement as an ENUMERATION removes the denominator question entirely instead of answering it again.

---

## Stage close — the phase-75 IMPL (controller)

**Row 75 is `done`. Lifecycle-state `3 -> DONE`. ADR-0297 is COMPLETE. The sentinel does NOT fire and `stop` was NOT created.**

### What the controller did directly (not delegated)

`STATE.md` §Current rolled **IN PLACE** (never prepended — the ADR-0288 rule): `active-phase` -> IMPL done · `lifecycle-state` **3 -> DONE** · `next-skill` -> **the phase-76 BRAINSTORM** · `last-commit` / `last-updated` rewritten · `next-free ADR` updated to record ADR-0297 **COMPLETE** and ADR-0296 **no longer byte-untouched**. §Project counts refreshed (stat surface **1204 -> 1205**, and all four "Phase 74 landed +0" bullets re-pointed to phase 75). §Recent **re-capped at FIVE with its PREAMBLE updated** — the phase-75 IMPL bullet added, the **phase-74 PLAN** bullet dropped; the five now read phase-75 IMPL · phase-75 PLAN · phase-75 SPEC · phase-75 BRAINSTORM · phase-74 IMPL. `next-prompt.txt` rolled to the phase-76 BRAINSTORM (**TRACKED despite `.gitignore`**; edited in this worktree).

⚠️ **The three ADR-0288 singleton greps return `2`, not `1`, and were VERIFIED at 2 after every edit** — the second hit of each is `STATE.md:7`, the RULE STATEMENT itself. **Never "fix" the count to 1; that would delete the rule.**

⚠️ **The `:17314 -> :17316` citation fixup was applied to `STATE.md`'s phase-75 BRAINSTORM recap bullet.** The PLAN cited it as `STATE.md:48`; at this tip it lives on a different line — **found by CONTENT, not by the PLAN's line number.** The `ROADMAP.md:137` half was fixed at T11. The frozen `SPEC.md` / `BRAINSTORM.md` were **deliberately NOT rewritten** — no convention requires it and the lineage treats them as historical records.

### Sentinel — run MECHANICALLY by the controller, TWICE

Once in the stage worktree at the T11 tip and once on **landed master after the squash-push**. **IDENTICAL both times. It does NOT fire; `stop` was NOT created.**

- **(1) SILENT.** ⚠️ **The change of the stage.** Row 75 was the last non-`done` chartered row. Recorded as the ACTUAL output, and re-checked against the cell itself: row 75 at `:137` **is matched by the regex** and reads `done`, so the silence is honest rather than a regex miss.
- **(2) => 3** by both the `grep -cE` and the occurrence-count forms; the sentence still terminates at `force-trace.` and carries **ZERO interior periods**, so `[^.]*\.` still binds.
- **(3)** => `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`.

**Blind spot re-derived independently: 107 data rows (`:31`-`:137`) / 103 matched / FOUR misses**, all four `done`. ⚠️ **This figure had been recorded WRONG in two consecutive lineages** before the phase-75 IMPL; it was derived here rather than copied.

### The delta, mechanically

**EIGHT subagent dispatches over the 11-task spine -> 20 local commits + the controller's stage-close commit -> ONE squash.** Subagents committed **LOCALLY ONLY** and never pushed; the controller squash-pushed at close. Every subagent was pinned to the canonical worktree root and used `git -C <abs-path>`.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED LIVE AGAIN** — at T10 the tripwire found `pwd` had silently reset to the **MAIN checkout on `master`**, one step before a gate run. That is the third consecutive phase-75 session in which it fired, and the reason every gate in this stage ran through an explicit absolute path. **The main checkout stayed clean throughout.**

### What this stage did NOT verify — stated so the next phase inherits no false confidence

- **`ssl.no_certificate` on a RESUMED TLS 1.3 session was never probed** (`session_reused` 0 in every run). **The one scenario in which the pinned predicate could be wrong in production.**
- **QUIC was never DRIVEN.** The registration is pinned and green; the parity argument is STRUCTURAL and therefore name-independent, **but it is an argument, not a measurement** — and BEHAVIOR_CONTRACT `:1857` states it as INHERITED rather than re-derived.
- **The predicate was never cross-checked at `require_client_certificate: true`** (`0109` not run).
- **The stat-surface ABSOLUTE total was NOT re-derived** — no mechanical command exists, and the chain is now known discontinuous in **TWO** places. The **+1 DELTA** is asserted; **1205 is documentary** and is not presented as measured.
- **Every §3 reference figure in T8's prose is transcribed from ADR-0297 §Context, not re-executed** — cited as such.
- **`internal/statssink`'s four stale prose sites** (stale since phase 49) were **RECORDED, NOT FIXED**; that package is on the byte-untouched sha256 roster.

### Handed forward as genuinely owed

**`0061-lb-ring-hash`'s spread assertion is now at its THIRD recorded occurrence** (phase-57, phase-62, phase-75-under-`-race`). `internal/cluster/**` was sha256-verified byte-untouched and it was isolate-green 4/4, so it is structurally not this row — **but the margin/workload fix is owed as a Load-balancing-family maintenance item**, and `reference_0061_ring_hash_spread_flake` was updated to record the third hit. ⚠️ **Neither flake that fired at this IMPL was on the PLAN's roster of six known flakes — a stage brief's flake list is not the index.**
