# PROGRESS 75 — the downstream TLS `ssl.no_certificate` success-path annotation at listener scope

> Live task ledger for the phase-75 IMPL. `PLAN.md` is the spine; **this file records what ACTUALLY happened per task** — red-first verbatims, WHICH break assertion fired (and any that did NOT), substitutions **and whether their stated rationale survived scrutiny**, and every PLAN/SPEC claim refuted by execution. A break that does not fire is a FINDING, not a nuisance.
>
> ⚠️ **THIS ROW IS UNUSUAL: the PLAN stage already executed most of it.** SPEC §16 closed with *"NO BUILD OF ROW 75 EXISTS"*; that is no longer true. Three private build worktrees compiled the row, ran the unit suite and the FULL differential green, and executed **eight of the ten breaks**. **The IMPL's job is therefore verification-in-place and the docs half — not discovery.** Where the PLAN says `[RUN]`, do not re-derive from scratch; re-run and confirm.

## Stage pointer

- **PLAN done** (2026-07-25; flipped only AFTER §Adversarial-pass record below was populated from the agents' actual reports — writing "done" over an open gate is the exact class the phase-74 PLAN's own V2 caught in an earlier draft, and phase 74 had itself deleted phase-73's guardrail against it). The **11-task SINGLE-FLAT-ROW TDD spine (T1–T11)** landed with a **TEN-break roster of which EIGHT were EXECUTED at this stage**. Every SPEC anchor RE-DERIVED at `cedd2f27` by four read-only agents, **plus three build-by-execution agents in separate worktrees**, plus controller re-verification of every load-bearing correction. Docs-only: `PLAN.md` + `PROGRESS.md` are the ONLY two files in the phase-directory delta — **ZERO production `.go`; ROADMAP and DECISIONS BYTE-UNTOUCHED; row 75 STAYS `in-progress`.** Worktree `/home/esa/git/envoy-go-wt-p75plan` off master `cedd2f27`, branch `phase-75-plan`. Sentinel re-run MECHANICALLY TWICE (worktree + landed master post-push): does **NOT** fire; `stop` NOT created. Counts UNCHANGED (fixtures 119 · fuzzers 55 · stat surface 1204 · BackendKind 38 · modules 2 · DECISIONS tail ADR-0297 PROPOSED). **Next → the phase-75 IMPL.**

- **IMPL** — *(not started; T1–T11 ledger below)*

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
| T1 | field + `rt.tlsMode`-gated registration + the TWO exact-set pins (**RED via the pins**) | A, B | | |
| T2 | `GateMatchesInc` +2 `t.Error` pointer assertions (the nil-Inc crash guard) | C | | |
| T3 | **the guarded Inc + VARIADIC `assertSSLCrossProduct` + roster→4 + the positive arm** ⚠️ the row's load-bearing task | D, D′, E, F | | |
| T4 | `helpText` entry + `wantNames` + both count claims (**RED via `wantNames`**) | G, G′, G″ | | |
| T5 | `0110` cross-side `StatsAsserter` + `scrapeProm` + `var _` + the second precondition | H, I, J | | |
| T6 | `0110`'s three stale `ssl.*` confessions (SPLIT the README bullet; the third clause INVERTS) | — | | |
| T7 | `0111` prose: `no_certificate` joins the NAMED-UNASSERTED lists (driver BYTE-UNTOUCHED) | — | | |
| T8 | BEHAVIOR_CONTRACT: eleven in-place edits + ONE new ledger line (no new paragraphs) | — | | |
| T9 | the stale-"three" prose sweep (5 files; none of it fails a test) | — | | |
| T10 | the six-gate + full differential + `-race` + counts + a TWO-CATEGORY envelope audit | — | | |
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

*(To be written at the phase-75 IMPL. Sections owed: Task ledger — ACTUAL · Break ledger (all TEN, with which assertion fired and which did NOT) · PLAN/SPEC claims REFUTED BY EXECUTION · any fresh hazards · flake classifications WITH evidence · the envelope audited in TWO categories · counts at exit re-run MECHANICALLY.)*
