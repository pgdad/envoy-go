# Phase 94 — `tls-connection-error-stat` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal.** Give `outcomeOther` — the fourth downstream-TLS handshake-outcome bucket, which today increments nothing — a predicate-gated `Inc()` landing `listener.<normalized-addr>.ssl.connection_error`, the FIFTH listener-scope TLS counter, closing the named departure at `BEHAVIOR_CONTRACT.md:1971`.

**Architecture.** One struct field and one `NewCounter` on the existing `rt.tlsMode` registration gate; one `case outcomeOther:` arm on the existing handshake-error switch, guarded by a package-level `isTransportHandshakeErr` that excludes a CLOSED, fully `errors.Is`-able transport-error complement. The `handshakeOutcome` taxonomy stays FOUR while the counters go to FIVE — `outcomeOther` becomes the only outcome mapping to a counter CONDITIONALLY. Instrumentation is TWO-LAYER: a new in-process counter table driving the REAL listener and the REAL `Inc` site (which this stage proved is a genuine guard), plus differential fixture `0120` for cross-side reference parity.

**Tech Stack.** Go 1.26.7 · `crypto/tls` · `internal/stats` registry · the differential harness (`test/differential`) against `envoyproxy/envoy:contrib-v1.37.2` (digest `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`).

**Spec.** `docs/envoy-go/phases/94-tls-connection-error-stat/SPEC.md` (500 lines). The plan argues from the spec; executors read both. ⚠️ **Read §0 of this plan FIRST — it refutes SIXTEEN of the spec's claims by execution, three of them load-bearing on the plan's own shape.**

---

## Global Constraints

Every task's requirements implicitly include this section.

- **ONE STAGE PER SESSION.** This document is the PLAN (lifecycle-state 2 -> 3). It writes NO production code. Row 94 STAYS `in-progress`.
- **`ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` are BYTE-UNTOUCHED at this PLAN.** MEASURED across three precedent PLAN commits (§1.2) — not assumed.
- **`ADR-0316` stays in the house `PROPOSED` form.** The tail STAYS `ADR-0316`; next-free STAYS `ADR-0317`. The IMPL completes the block and disarms the guard.
- **PATHSPEC-SCOPE every symbol assertion and every `sed`.** Both edit anchors are unique ONLY under `-- '*.go'` (§0.5).
- **`-count=1` is NOT optional for the differential** — the harness builds envoy-go as a SUBPROCESS, so a production edit is not a compile-time input and the cache serves a stale PASS.
- **Gate on OUTPUT, not exit code**, for `gofmt -l` (never exits non-zero). `golangci-lint` does exit non-zero but still gate on output. Its misspell runs in **locale US** — sweep British spellings in `.go` comments before every gate.
- **Every `-run` selector must be shown to select something.** A selector matching nothing prints `ok … [no tests to run]` and EXITS 0 — reproduced live at this stage (§0.16).
- **`go test` without `-v` prints zero `=== RUN`.** `RUN=0` beside `RC=0` is a vacuous green. Assert a non-zero denominator on every gate.
- **Anchored FAIL only:** `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`. An unanchored `grep -c FAIL` reads nonzero on a fully green tree.
- **`grep -c` prints `0` AND exits 1** — capture `v=$(… || true)`; never `$(… || echo 0)`, which emits two zeros.
- **`rc=$?` after a pipe returns the LAST command's status** — use `out=$(…); rc=$?` or `PIPESTATUS`. ⚠️ `INNER_EXIT` does not exist in this repo.
- **`go test ./...` drives Docker in TWO places.** Exclude both: `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`.
- **`-race` on the differential suite is VACUOUS** — the subject is an unraced subprocess.
- **Build with `-o` into scratch.** A bare `go build ./cmd/envoy-go/` drops an untracked binary in the worktree root.
- **`pgrep -f` / `pkill -f` match your own shell and kill the tool call (exit 144).** Kill only PIDs you captured from inspected `pgrep -laf` output.
- **NEVER tear down a container this session did not create.** A sibling `curl-world` session owns eleven containers; `reaper_*` (Ryuk) is created BY the harness and REUSED. Tear down BY NAME, only your own.

---

## 0. What this PLAN refuted by execution — SIXTEEN claims

The project discipline is that every stage refutes its predecessor by execution (92 IMPL twelve · 93 BRAINSTORM eleven · 93 SPEC eleven · 93 PLAN twelve · 93 IMPL nine · 94 BRAINSTORM eleven · 94 SPEC twelve). This stage refutes **SIXTEEN**. Method: three parallel measurement agents, each on its own worktree and branch off `820fc145` with a private scratch dir, Docker serialized to exactly ONE agent; all four trees proven clean, **nothing committed by any of them**. Everything below marked EXECUTED was executed.

### ⚠️ 0.1 — THE `sslLeafRoster` 2×2 CELL D IS REFUTED, AND WITH IT THE SPEC'S CENTRAL FALSIFIABILITY CLAIM

`SPEC.md` §6.2 states that with `sslLeafRoster` at FIVE, removing the predicate goes **+2 RED**, and concludes: *"Without the roster extension the row ships a predicate that no test can falsify."*

**EXECUTED — the full 2×2, `./internal/listener/`, RUN=132 each cell:**

| roster | predicate | SPEC claims | MEASURED |
|---|---|---|---|
| 4 | correct | 2 spelling pins red | 2 spelling pins red ✅ |
| 4 | removed | still only 2 — defect invisible | still only 2 ✅ |
| 5 | correct | only the 2 | only the 2 ✅ |
| 5 | **removed** | ⚠️ **+2 RED** | ❌ **still only the 2 — NO CHANGE** |

**The reason is structural and decisive.** The guard lives INSIDE `case outcomeOther:`. The two tests the SPEC names — `TestServeConnection_SSLFailVerifyErrorIncrements` (`manager_test.go:4674`) and `TestServeConnection_SSLFailVerifyNoCertIncrements` (`:4699`) — drive certificate-verification failures that classify to `outcomeVerifyError` / `outcomeNoCert`. **They never enter `case outcomeOther:` at all**, so removing a guard inside it provably cannot move them. Controller-verified by reading the disputed sites rather than trusting either the SPEC or the agent.

**The SPEC's number reproduces ONLY under a different mutation** — the `Inc` **hoisted above the switch**, unconditional on every handshake error:

```
roster=5, Inc HOISTED:  RC=1  RUN=132
  manager_test.go:4694: listener.…ssl.connection_error = 1, want 0 — only [fail_verify_error] may move on this arm
  manager_test.go:4712: listener.…ssl.connection_error = 1, want 0 — only [fail_verify_no_cert] may move on this arm
roster=4, Inc HOISTED:  only the 2 spelling pins red
```

That is the SPEC's §6.2 message **verbatim, including the `:4694` line number**. ⇒ **`SPEC.md` §8 NC 1 mislabels an Inc-SITE RELOCATION as "remove the predicate".**

**CONSEQUENCE FOR THIS PLAN — and it changes the plan's shape:**
- The `sslLeafRoster` extension **stays MANDATORY**, but for a DIFFERENT reason than the SPEC gives: it guards the **Inc SITE** (it is the only thing that catches an `Inc` leaking onto the cert arms) and it couples to registration through `counterValue`'s absent-name `t.Errorf`. **Isolating NC, EXECUTED:** adding `connection_error.Inc()` to the `outcomeVerifyError` arm alone reddens `:4694` while the no-cert arm stays green — so the roster's negative half is live, not vacuous.
- **It does NOT and never could make the PREDICATE falsifiable.** No `assertSSLCrossProduct` call site (`:4663 :4694 :4712 :4805`) reaches `outcomeOther`.
- ⇒ **The predicate needs a NEW test. Task 5 is that test**, and it is the single most load-bearing task in this plan.

### ⚠️ 0.2 — THE UNIT LAYER ALREADY REACHES `outcomeOther`; THE FIXTURE IS NOT THE ONLY INSTRUMENT

`SPEC.md` §7 states *"`outcomeOther` is produced by ZERO fixture arms anywhere in the tree ⇒ the row owes a new positive arm, or the `Inc` ships unexecuted"*, and builds the fixture case on it.

**EXECUTED — `panic("REACHED_OUTCOME_OTHER")` at the `Inc` site, `go test -v ./internal/listener/` (RUN=132):**

```
=== RUN   TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts
panic: REACHED_OUTCOME_OTHER
  …listener.(*listenerRuntime).serveConnection(…) manager.go:1321
```

Enumerated exhaustively with the panic replaced by a `log.Printf` marker (so no fail-fast masking), **exactly ONE existing test reaches it**:

```
TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts ||
  REACHED_OUTCOME_OTHER transport=false err=*errors.errorString
  tls: client requested unsupported application protocols (["bogus/9"])
```

`transport=false` ⇒ **the `Inc` FIRES TODAY, on an existing test, unasserted.** SPEC §7's narrower claim — *"adding the `Inc` changed the red set by ZERO tests"* — STAYS TRUE (that test asserts abort, not counters). But the implied conclusion that the fixture is the only possible instrument is **REFUTED**, and this stage went further and proved all six `SPEC.md` §2.2 arms are unit-drivable through the real listener (§0.3). **A SEVENTH arm — ALPN negotiation failure — is absent from §2.2's six-row table.**

Fixture `0120` remains owed, for **cross-side reference parity**, which no unit test can supply. It is no longer the row's only proof that the `Inc` executes.

### ⚠️ 0.3 — THE COUNTER TABLE IS A REAL GUARD: BUILT, RUN, AND NC'd BOTH WAYS

Driven through the REAL listener and the REAL `Inc` site (`startOneWayTLSListener`, raw `net.Dial`), with a **release barrier** — poll `downstream_cx_total` to N, then `downstream_cx_active` back to 0. That is sound because `serveConnection` defers `downstreamCxActive.Dec()` at `manager.go:1245`, which runs AFTER the outcome switch. **No sleeps.**

```
P94CNT|bad_version_TLS11_client|connection_error=1|want=1  PASS
P94CNT|plaintext_http          |connection_error=1|want=1  PASS
P94CNT|garbage_bytes           |connection_error=1|want=1  PASS
P94CNT|partial_hello_then_FIN  |connection_error=0|want=0  PASS
P94CNT|zero_bytes_then_FIN     |connection_error=0|want=0  PASS
P94CNT|partial_then_RST        |connection_error=0|want=0  PASS
P94STACK|3 excluded arms + 1 included arm -> connection_error=1 (want 1)  PASS
```

**NC'd BOTH WAYS — it fails when it should:**

| NC (neutralised, still compiles) | result |
|---|---|
| predicate removed (unconditional `Inc` on `outcomeOther`) | **RED**: all three exclusion arms read 1 want 0; **stacked control reads 4, want 1** |
| `Inc` removed (`_ = rt.sslConnectionError`) | **RED**: all three inclusion arms read 0 want 1; **stacked control reads 0, want 1** |

⇒ **ALL SIX §2.2 arms belong in the unit table; none needs the fixture.**

### ⚠️ 0.4 — THE REFERENCE CONTAINER CANNOT READ `filename:` CERT PATHS (controller finding)

`SPEC.md` §7.3 pins the `0018-http-rbac/envoy.yaml:216-226` form with `trusted_ca: { filename: … }`. **That path exists on the HOST, where envoy-go runs — it does NOT exist inside the reference CONTAINER.** `0018` gets away with it only because its driver implements `fixture.ReferenceLogMounter` and bind-mounts the PKI in (`runner_test.go:1170-1193`). The measurement agent's manual `docker run` masked this, because it mounted by hand.

Verified at this tip: `0110`, the closest TLS fixture, takes the **other** route — `inline_string:` PEMs rendered into the templated YAML (`0110/envoy.yaml:78-88`), with the driver comment at `:42` warning they are *"Pre-indented to 24 spaces."* And envoy-go supports it: `internal/tls/datasource.go:24-25` handles `*corev3.DataSource_InlineString`.

⇒ **DECISION (D-TLSCE-CERTDELIVERY, new at this PLAN): `0120` delivers PEM bytes via `inline_string:` on BOTH sides, rendered from committed `pki/` files at template time — the `0110` route. NO `ReferenceLogMounter`, NO bind-mounts.** The `validation_context` STRUCTURE the SPEC pinned (inline, not `combined_validation_context`, not SDS) is UNCHANGED; only the data-source specifier changes.

### ⚠️ 0.5 — `SPEC.md` §0 REFUTATION 3'S COUNTS ARE STALE (its conclusion is STRONGER than stated)

| pattern | scope | SPEC | MEASURED (lines / files) |
|---|---|---|---|
| `return outcomeOther` | `-- '*.go'` | unique | **1 / 1** (`manager.go:455`) — HOLDS |
| `return outcomeOther` | unscoped | 4 | **7 / 6** |
| `case outcomeVerifyError:` | `-- '*.go'` | unique | **1 / 1** (`manager.go:1295`) — HOLDS |
| `case outcomeVerifyError:` | unscoped | 3 | **7 / 6** |

Unscoped files, identical set for both: `STATE.md`, `74/PLAN.md`, `94/BRAINSTORM.md`, `94/SPEC.md`, `manager.go`, `next-prompt.txt`. This is `reference_self_incrementing_positive_control` — **the SPEC measured 4/3 before its own landing, and its own landing moved the number.** The governing conclusion (unique only under `-- '*.go'`; an unscoped `sed -i` corrupts landed documents) is now backed by **five** doc files instead of two. **RE-DERIVE; never quote.**

### 0.6 — `SPEC.md` §9's "8 sites across 5 FILES" is FALSE

The §6.2 table spans **FOUR** files (`manager_test.go`, `quic_test.go`, `name_test.go`, `helptext_test.go`). `name.go` appears in **ZERO** §6.2 rows and is already booked in §6.1's "Production — 2 files". A double count.

### ⚠️ 0.7 — `SPEC.md` §6.6's "no checksum assertion covers any roster file" is FALSE

Two gates cover `internal/stats/name.go`, both controller-verified:
- **A byte-exact golden.** `name_test.go:1153-1156` holds a hand-written duplicate of `noRecognizedSegmentErrFmt` (`name.go:43-46`), asserted byte-exact by `TestExtractTagsTerminalError_ByteStable` (`:1158`).
- **A source-AST gate.** `segmentcount_test.go` runs `parser.ParseFile(fset, "name.go", nil, 0)` at **`:79` AND `:133`**, cross-checking the message's claimed 13 roots / 4 mid-name segments against `ExtractTags`' actual AST.

**The set-difference OUTCOME (∅) SURVIVES** — `ssl` is neither a top-level root nor one of the four mid-name segments, and a `helpText` entry never touches `ExtractTags`. But §6.6's wording is refuted, and it matters because §6.1 schedules a `gofmt` pass over `name.go`. Correct wording: *"a byte-stable gate and an AST gate cover `name.go`; this row touches neither `noRecognizedSegmentErrFmt` nor `ExtractTags`."*

### 0.8 — `SPEC.md` §6.6's "`ci.yml` carries no `-run` selector" is FALSE

`.github/workflows/ci.yml:108`: `go test ./${{ matrix.pkg }}/ -run=^$ -fuzz=^${{ matrix.target }}$ -fuzztime=30s`. It IS a `-run` selector (deliberately empty, for fuzzing). The `ssl` half of the claim HOLDS. Outcome survives.

### ⚠️ 0.9 — `SPEC.md` §0 REFUTATION 7 (the `gofmt` realignment): THE CAUSE IS REFUTED AND THE EFFECT IS IN THE WRONG FILE

The SPEC claims the new `helpText` key exceeds the alignment column so `gofmt` rewrites the whole map block, and says to *"budget a realignment diff across the group."*

**EXECUTED.** Key lengths: `envoy_listener_ssl_connection_error` = **35**; the block's incumbent longest, `envoy_listener_ssl_fail_verify_no_cert` = **38**. **The new key is SHORTER.** A bare unaligned insertion yields:

```
gofmt -d internal/stats/name.go
-	"envoy_listener_ssl_connection_error": "…",
+	"envoy_listener_ssl_connection_error":    "…",
   "envoy_listener_ssl_handshake":           "…",   <- UNCHANGED
   "envoy_listener_ssl_fail_verify_error":   "…",   <- UNCHANGED
   "envoy_listener_ssl_fail_verify_no_cert": "…",   <- UNCHANGED
```

**Zero existing lines rewritten.** Hand-aligned (4 spaces after the colon), `gofmt -l internal/stats/` prints **nothing at all**. There is no group diff to budget.

**The realignment the row DOES incur is in `manager.go`, and the SPEC never mentions it** — one existing struct-field line, because `sslConnectionError` IS longer than `sslNoCertificate`:

```
-	sslNoCertificate *stats.Counter   // phase 75: …
+	sslNoCertificate   *stats.Counter // phase 75: …
 	sslConnectionError *stats.Counter // phase 94: …
```

### 0.10 — THE RED SET REPRODUCES; ITS ARITHMETIC CONSTANT DOES NOT

Production-only prototype, repo-wide excluding the two Docker drivers: **`RC=1`, `RUN=8704`, 234 packages, anchored `FAIL=8`.** The three-test membership reproduces EXACTLY (`TestHelpText_KeySetExact`, `TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames`, `TestQUICListener_RegistersSSLNamesAtZero`). **`FAIL=11` DRIFTED to 8** — the delta is exactly the wasm flake's three anchored lines; `TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure` **did not recur**. The membership claim stands; **the constant `11` is tip-specific and MUST NOT be inherited.**

### 0.11 — `SPEC.md` §0 REFUTATION 9'S COUNT IS FALSE; ITS SUBSTANCE HOLDS

"All ten hits are under `docs/`" is false as written: `tls_params` reads **56 lines across 13 files**, of which **4 are NOT under `docs/`** (`internal/tls/config.go`, `config_test.go`, `params.go`, and `next-prompt.txt`, which is self-referentially a hit). **The substantive claim is TRUE and was re-derived: `git grep -n 'tls_params' -- '*.yaml' '*.yml'` returns NO MATCHES (rc=1).** ⇒ **State it as the YAML-scope claim and quote NO total** — the total rots on every doc edit.

### 0.12 — `SPEC.md` §6.3's PHANTOM-`B5` COUNT IS FALSE

The SPEC says `:1971` plus *"four unrelated `AMEND-B5` contexts"*. Measured: **10 `B5` lines total** — `:1971`, **8** `AMEND-B5` lines (one being `AMEND-B1..B5`), and a **10th distinct-species hit at `:5964`** (*"the END_STREAM-off emit (B5)"*, a phase-93 break-arm label) the SPEC never mentions. The replacement anchor `BEHAVIOR_CONTRACT.md:1965` = `### Downstream TLS handshake-outcome stats (…)` is **EXACT**, and the phantom conclusion STANDS. But `BEHAVIOR_CONTRACT.md:810` opens `**B3 propagation …**`, so *"no B-numbered step scheme in that file at all"* is looser than stated.

### 0.13 — `SPEC.md` §7.2 UNDER-CITES ITS OWN EVIDENCE: PHASE 92 ALREADY LANDED `10126`

The SPEC frames the port as CONTESTED at phases 81/82/83 and settles it here. **Controller-verified: phase 92 already landed it explicitly** — `92/SPEC.md:79` (*"Census of `101xx` in `test/`: **10100-10125 and 10130-10140 are taken; 10126-10129 are free**"*), `:80`, `:393` (*"`0120`'s expected port **10120 is TAKEN** … it would need **10126**"*), and `92/PLAN.md:823`. **`10126` is a RE-DERIVATION of a landed prior measurement, not a fresh settlement.** That STRENGTHENS the pick; cite phase 92.

### 0.14 — `SPEC.md` §6.2 SITES 1 AND 2 LINE RANGES ARE OFF BY ONE; ONE `next-prompt.txt` CITE DRIFTED

`manager_test.go` want entries are **2138-2141** (literal `2137-2142`), not `:2136-2141`; `quic_test.go` entries **282-285** (literal `281-286`), not `:280-285`. Both `DeepEqual` cites HOLD. The code files are byte-unchanged since phases 86/75/80 ⇒ **a SPEC authoring error, not tree movement.** `next-prompt.txt:116` -> **`:104`**. Use the STABLE anchors given in §4.

### ⚠️ 0.15 — `SPEC.md` §6.3's "Also prose" LIST MISSES SEVEN LINES, TWO OF THEM HEADING THE ARMS §6.2 MANDATES EDITING

| file:line | text | why it goes false |
|---|---|---|
| `manager_test.go:2079-2080` | *"WOULD PASS WITH ALL **FOUR** NAMES MISSPELLED"* | five names after this row |
| `manager_test.go:2215` | *"Only the **four** field pointers — non-nil iff tlsMode"* | five pointers |
| **`manager_test.go:2241`** | *"all **FOUR** counter fields are NON-NIL"* | ⚠️ **heads site 7's TLS arm, OUTSIDE the `2281-2298` edit range** |
| **`manager_test.go:2302`** | *"all **FOUR** counter fields are NIL"* | ⚠️ **heads site 7's plaintext arm, OUTSIDE the `2340-2354` edit range** |
| `quic_test.go:65` | *"…\"all **four** are zero\" is trivially true"* | in `counterValue`'s doc comment |
| `quic_test.go:273-274` | *"all **four** names present, spelled exactly"* | immediately above the listed `:275-281` |

**A plan following the SPEC's byte ranges literally would land a fifth nil-assertion under a comment that still says "all FOUR".** These are MANDATORY edits, folded into Tasks 2 and 8.

### 0.16 — MISCELLANEOUS, EACH EXECUTED

- **`SPEC.md` §8 NC 5 is RICHER than stated.** Roster present / entry absent fires **TWO** tests, not one: `TestHelpText_KeySetExact` (`:139 missing:`) **and `TestHelpText_NoSelfEqualHelp`** (`:202 1 rendered HELP line(s) degraded to the metric name`).
- **`SPEC.md` §9's CI/scripts claim is VACUOUS for two of its three targets** — this repo has **no Makefile and no `scripts/`**. `ci.yml` is the only non-source build file.
- **`SPEC.md` §13's md5 method is unstated and the digest is method-sensitive.** The six recorded digests match ONLY with the trailing newline included (`sed -n 'Np' file | md5sum`); `tr -d '\n'` mismatches all six. Also its malformed-row identifiers **"57" and "69" are ROW IDs, at FILE LINES 119 and 131** — mixed units in one paragraph.
- **The stale-selector hazard REPRODUCED LIVE at this tip:** `go test ./internal/listener/ -count=1 -run 'TestListenerMetrics_TLSListenerRegistersExactlyFiveSSLNames'` ⇒ `ok … [no tests to run]`, **exit=0**.
- **`assertSSLCrossProduct` CANNOT express a pure negative arm** — it `t.Fatalf`s on zero `wantSuffixes` (`:4614`), with a comment explaining that such a call would pass vacuously. The exclusion arms need the STACKED shape (Task 5), not a bare `== 0`.
- **`connPair` (`manager_test.go:4270`) binds `127.0.0.1:0`.** The unit table needs **port 0**, not a banded port; the 12000-18000 banding rule is MOOT for it.

**Three of the sixteen refute this stage's own agents or its own controller**: the measurement agent's `sslLeafRoster` cell-D result was accepted only after the controller read the disputed call sites directly (`reference_contradicting_agents_find_the_variable`); the same agent's `tls_params` premise re-derivation corrected the SPEC's count while confirming its substance; and §0.4 is a controller finding that the Docker-owning agent's manual `docker run` had masked.

---

## 1. Stage scope, MEASURED

### 1.1 What this PLAN commit touches

`docs/envoy-go/STATE.md` · `docs/envoy-go/STATE_HISTORY.md` · `docs/envoy-go/phases/94-tls-connection-error-stat/PLAN.md` · `next-prompt.txt`. Nothing else.

### 1.2 Why — MEASURED across three precedent PLAN commits, not inferred from wording

`git show --numstat` at this tip:

| precedent | files |
|---|---|
| `90010c4c` (phase 93 PLAN) | `STATE.md` · `STATE_HISTORY.md` (+2) · `phases/93/PLAN.md` (1031) · `next-prompt.txt` |
| `bae5e24d` (phase 75 PLAN) | `STATE.md` · `phases/75/PLAN.md` (1394) · `phases/75/PROGRESS.md` (+115) · `next-prompt.txt` |
| `acedfd2b` (phase 77 PLAN) | `STATE.md` · `phases/77/PLAN.md` (2212) · `phases/77/PROGRESS.md` (+97) · `next-prompt.txt` |

- **ALWAYS, all three:** `STATE.md`, `next-prompt.txt`, the phase's own `PLAN.md`.
- **NEVER, all three:** ⚠️ **`DECISIONS.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`.** ⇒ `ADR-0316` STAYS `PROPOSED`, tail STAYS `ADR-0316`, next-free STAYS `ADR-0317`, and the SPEC §11 contract map STAYS PINNED. The IMPL lands it.
- **CONDITIONAL — both evaluated at this tip, not inherited:**
  - `STATE_HISTORY.md` — driven entirely by whether §Recent is at the ADR-0288 five-entry cap. **It IS at cap (five entries, verified `grep -c '^- \*\*prior active-phase:\*\*' docs/envoy-go/STATE.md` = 5)** ⇒ this PLAN **owes an eviction and a `+2` parenthetical append**.
  - `phases/94/PROGRESS.md` — created at the 75 and 77 PLANs, **NOT** at the 93 PLAN. **Phase 94 FOLLOWS THE 93 POSTURE: no `PROGRESS.md` at the PLAN.** `STATE.md` continues to record *"No `PROGRESS.md`, no `REVIEW.md`"* as a **STANDING DEPARTURE, NAMED NOT CLAIMED**. This plan is not changing that posture; the IMPL creates `PROGRESS.md` per the lifecycle.

### 1.3 The split gate — EVALUATED, NOT SPLIT, AND THE REASONING IS STATED SO A REVIEWER CAN OVERTURN IT

`BOOTSTRAP_PROMPT.md` §6.1 triggers a split if the plan exceeds **~25 numbered tasks** OR **~1500 lines of code** of net change.

**Task count: 18** (§5). Under the gate.

**LoC — derived from MEASURED precedent, not guessed:**

| component | basis | estimate |
|---|---|---|
| `internal/listener/manager.go` | phase 75 IMPL landed +30/-4 for the fourth name | **+45 / -20** |
| `internal/stats/name.go` | phase 75 landed +8/-4 | **+8 / -4** |
| test edits, 8 sites | phase 75 landed +197 `manager_test.go` +14 `quic_test.go` +8 `name_test.go` | **+40** |
| the NEW counter unit table (Task 5) | measured prototype | **+300** |
| fixture `0120` | **fixture `0118`'s creating commit `4d7f63c2` landed +1144** (README 166 · driver 467 · envoy-go.yaml 135 · envoy.yaml 159 · expectations 216) + `runner_test.go` +1 | **+1150** |
| `0120/pki/` (3-5 committed PEMs) | `0119` ships 26 lines of PEM | **+40** |
| `0110`/`0111` prose, 4 files | phase 75 landed +16/+46/+9/+14 | **+25** |

**Total ≈ +1608 / -25.** Counting only `.go` files: manager.go 45 + name.go 8 + tests 340 + driver.go ~600 + runner_test.go 1 ≈ **+995**.

**DECISION: DO NOT SPLIT.** Three grounds, in order of weight:

1. **The measured precedent is directly comparable and did not split.** Phase 74 — the row that CREATED this counter family — landed **+1658 / -140** in one phase (non-docs ≈ +1467) on a **9-task** PLAN. This row's estimate sits beside it.
2. **The `~1500` figure reads on "lines of code."** The `.go` total is ≈ **+995**; the remainder is YAML config, committed PEM, and Markdown fixture prose. On the narrow reading the gate is not crossed at all; on the broad reading it is crossed by ~7%.
3. ⚠️ **The natural split line is the one split that would defeat the row's purpose.** The only clean seam is {counter + unit table} / {fixture `0120`}. Splitting there ships a counter whose cross-side parity against the reference is unproven — and cross-side parity is the entire reason the departure at `BEHAVIOR_CONTRACT.md:1971` is being closed. Note this is a WEAKER objection than it would have been before §0.2: the unit table now proves the `Inc` executes, so a split would no longer ship an *unexecuted* `Inc`. It would ship an *unreconciled* one.

⚠️ **Splitting would additionally force a `ROADMAP.md` edit at this stage** (parent row + two child rows, `want` 126 -> 128), which §1.2 measured that a PLAN commit does not make. That is a consequence of the choice, not a reason for it — `BOOTSTRAP_PROMPT.md` §6.2 explicitly authorises a splitting PLAN to touch `ROADMAP.md` and stop.

**If the IMPL discovers any single task's sub-steps blowing past ~10 items, §6.1's mid-execution trigger applies and the split happens then.** The likeliest candidate is Task 12 (the drive arms).

---

## 2. Sentinel — RUN MECHANICALLY AT `820fc145`, ACTUAL OUTPUT

```
(1) NOT DONE: row 94                      <- ONE line, CORRECT and EXPECTED while row 94 is open
(2) 204: 210: 216: 226: 232: 240:         <- SIX
(3) (silent)
```

**All four NCs re-run, all four FIRED:**

| NC | result |
|---|---|
| **A** — row 62 doctored to `in-progress` | **TWO** lines (`row 62`, `row 94`); NC landed, inspected `[ in-progress ]` |
| **B** — `want=125` on the real file | **TWO**: `row 94` + `GATE FAIL: examined 126 data rows, expected 125` |
| **C** — `gRPC-family row` doctored out | residual **0**; FIRED `NEVER OPENED: gRPC` |
| **D** — `-family row` with the `--` guard | occurrences **96**, LINES **68** |

**CHECK-(2) POSITIVE CONTROL:** doctoring BOTH phrases out of a scratch copy drives check (2) to **0**. Not a stuck gate.

⚠️ **The six window lines are BYTE-IDENTICAL to the SPEC's record** — md5 (first 12, `sed -n 'Np' file | md5sum`, **trailing newline INCLUDED — see §0.16**): `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 4ad940205410 b2680e6f4fbf 6caa1c3ce0e7`. **That digest match is the proof no deferred-candidate line was tidied.**

**Field-count instrument, baselined:** row 94 is **NF=8 under BOTH forms**, with **7** pipe characters. Malformed baseline: **escape-aware exactly TWO** — row ids **57** (`NF=9`) and **69** (`NF=10`), at file lines 119 and 131; the **naive** form reads **17** and disagrees with the escape-aware form on row 57 (naive `NF=13`) while agreeing on row 69. ⚠️ **Any gate must state WHICH form it uses.**

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately not created** (verified absent at the git root and in every stage worktree).

⚠️ **THE MARGIN IS TWO.** Check (1) by an open row, check (2) by six lines. **Closing row 94 removes only the first.** ⚠️ **DO NOT "TIDY" A DEFERRED-CANDIDATE LINE — DELETING THE LAST ONE ENDS THE PROJECT.** And narrowing cannot move check (2): the check keys on the PHRASE, and removing a clause leaves phrase, line and count untouched.

---

## 3. The design record, as CORRECTED by this stage

The `D-TLSCE` docket is RESOLVED in `SPEC.md` §3 and is NOT re-litigated. What follows records only what this stage CHANGED or ADDED.

| id | disposition |
|---|---|
| **D-TLSCE-SEAM** | UNCHANGED. Predicate at the Inc site (`case outcomeOther:`), not inside `classifyHandshakeErr`. Taxonomy stays FOUR, counters go to FIVE. |
| **D-TLSCE-PREDICATE** | UNCHANGED in content. ⚠️ `net.ErrClosed` is DEFENSIVE and UNEXERCISED and **must be labelled as such in the code comment**. `context.DeadlineExceeded` is behaviour-matching, not defensive. |
| **D-TLSCE-NILGATE** | UNCHANGED, and REPRODUCED: deleting the registration while keeping the `Inc` SIGSEGVs **while `TestListenerMetrics_GateMatchesInc` PASSES**. Cross-confirmed — the crashing test IS the ALPN test of §0.2, the sole reacher. |
| **D-TLSCE-HELPTEXT** | Insert FIRST within the `ssl.*` group. PREPEND confirmed four ways. ⚠️ **The `gofmt` rationale is REFUTED (§0.9)** — hand-align and `gofmt -l` prints nothing. |
| **D-TLSCE-FALSIFIABILITY** | ⚠️ **NEW, and it replaces the SPEC's answer.** The `sslLeafRoster` extension guards the **Inc SITE**; the **PREDICATE** is guarded by the new counter table of Task 5. §0.1. |
| **D-TLSCE-CERTDELIVERY** | ⚠️ **NEW.** PEM bytes reach BOTH sides via `inline_string:`, rendered from committed `pki/` at template time (`0110` route). No `ReferenceLogMounter`. §0.4. |

### 3.1 The predicate, verbatim — this is what Task 4 lands

```go
// isTransportHandshakeErr reports whether a downstream TLS handshake error is a
// TRANSPORT failure rather than an SSL PROTOCOL error. The reference books
// ssl.connection_error IFF BoringSSL reports a protocol error (ADR-0316 §Context
// ¶2, measured); a transport EOF or reset books NOTHING under ssl.*.
//
// The POSITIVE population is open-ended and untypeable. The COMPLEMENT is closed
// and every member is errors.Is-able, so this matches the complement and the
// caller Inc()s otherwise. There is DELIBERATELY no message-text matching here —
// unlike the outcomeNoCert arm, which needs it because crypto/tls exports four
// error TYPES and ZERO error VALUES.
//
// ⚠️ net.ErrClosed is DEFENSIVE and UNEXERCISED: no measured arm produces it.
// It is retained because a closed listener racing an in-flight handshake is a
// real shape, but no test in this tree drives it and none claims to.
func isTransportHandshakeErr(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.DeadlineExceeded)
}
```

⚠️ **THE `io.EOF` TERM IS LOAD-BEARING AND EXERCISED BY A FIXTURE ARM.** Measured against the reference: a clean-FIN connection yields a bare `EOF`, `classifyHandshakeErr` has no EOF branch so it returns `outcomeOther`, **and the reference increments NOTHING.** Without the `io.EOF` term, fixture arm (iv) goes RED on the subject side. The discriminating negative control is not decoration — it is the arm that exercises this term.

⚠️ **THE `tls.RecordHeaderError` BY-VALUE FOOTGUN.** It is returned BY VALUE. `errors.As(err, &value)` is true; `errors.As(err, &pointer)` **compiles and is permanently false**. This predicate uses `errors.Is` only and does not touch it; recorded so no future arm reintroduces it.

---

## 4. File structure

**Production (2 files):**

| file | responsibility | change |
|---|---|---|
| `internal/listener/manager.go` | the counter field, its registration, the predicate, the Inc arm, six prose blocks | `sslConnectionError *stats.Counter` beside `sslNoCertificate`; `NewCounter(prefix + "ssl.connection_error")` inside the `rt.tlsMode` gate; `isTransportHandshakeErr`; `case outcomeOther:`; prose |
| `internal/stats/name.go` | the Prometheus HELP entry and its doc-comment enumeration | one `helpText` entry FIRST in the `ssl.*` group + enumeration updates |

**Tests (4 files — NOT five; §0.6):** `internal/listener/manager_test.go` (spelling pin, roster, `GateMatchesInc` ×2, rename ×4, prose, **the new counter table**) · `internal/listener/quic_test.go` (spelling pin, prose) · `internal/stats/name_test.go` (`wantNames` slice, comment rewrite) · `internal/stats/helptext_test.go` (`helpTextRoster` twin).

**New fixture `test/fixtures/0120-tls-connection-error/`:** `driver/driver.go` · `envoy.yaml` · `envoy-go.yaml` · `expectations.yaml` · `README.md` · `pki/{ca.pem,server.pem,server.key.pem,client.pem,client.key.pem}`. Plus **one line** in `test/differential/runner_test.go` — the only file outside the fixture directory that is touched.

**Fixture prose going false (4 files):** `0110/README.md:171-173` · `0110/expectations.yaml:224-226` · `0111/README.md:172-174` · `0111/expectations.yaml:229-231`.

**Docs at the IMPL:** `DECISIONS.md` (ADR-0316 §Decision + §Consequences) · `BEHAVIOR_CONTRACT.md` (the §11 pinned map) · `ROADMAP.md` (row 155 SUPERSEDED, row 94 -> `done`) · `STATE.md` · `PROGRESS.md`.

### 4.1 STABLE ANCHORS — use these, not line numbers

Line anchors drift; §0.14 caught two already wrong in the SPEC. Every task below anchors on literal text:

| target | STABLE anchor |
|---|---|
| `manager_test.go` ssl want slice | `prefix + "ssl.fail_verify_error",` (unique in file) |
| `quic_test.go` ssl want slice | `prefix + "ssl.fail_verify_error",` (unique in file) |
| the spelling-pin comparison | `if !reflect.DeepEqual(got, want)` |
| the QUIC comparison | `if got := listenerSSLNames(reg, addr);` |
| the roster | `var sslLeafRoster = []string{` |
| the registration gate | `rt.sslNoCertificate = r.NewCounter(prefix + "ssl.no_certificate")` |
| the field block | `sslNoCertificate *stats.Counter` |
| the Inc switch | `case outcomeNoCert:` followed by `rt.sslFailVerifyNoCert.Inc()` |
| the classifier | `func classifyHandshakeErr(err error) handshakeOutcome {` |
| `helpText` ssl group | `"envoy_listener_ssl_handshake":` |
| `helpTextRoster` ssl entry | `"listener.<addr>.ssl.no_certificate"` (verify exact form in file before editing) |
| `wantNames` slice | `func TestListenerSSLHandshakeOutcomes` enclosing block |

⚠️ **PATHSPEC-SCOPE EVERY `sed`.** `return outcomeOther` and `case outcomeVerifyError:` are unique only under `-- '*.go'`; unscoped each reads 7 lines across 6 files including three landed phase documents (§0.5).

---

## 5. Tasks

**18 tasks.** The count is DERIVED at this stage, not inherited — `SPEC.md` §9 deliberately carries no figure because four mutually inconsistent estimates are live in two different units. Ordering is TDD-first within each task and dependency-ordered across them: the spelling pins (T1) must be red before the registration lands, the matched pair (T3) must land together, and the `Inc` (T5) must not land before its nil guard (T2).

⚠️ **Every task ends with a commit.** Subagents commit LOCALLY on their own stage branch with EXPLICIT PATHSPECS; the controller merges and squashes.

---

### Task 1: The fifth name — spelling pins RED first, then registration

**Files:**
- Modify: `internal/listener/manager.go` (field + registration)
- Test: `internal/listener/manager_test.go`, `internal/listener/quic_test.go`

**Interfaces:**
- Produces: `listenerRuntime.sslConnectionError *stats.Counter`, registered as `listener.<normalized-addr>.ssl.connection_error` when `rt.tlsMode` is true, NIL otherwise. Every later task depends on this field name.

- [ ] **Step 1: Write the failing tests** — extend both exact-set slices. ⚠️ **The new name PREPENDS** (position 0, confirmed four ways under `LC_ALL=C` and `sort.Strings`, in both the dotted and Prometheus projections).

In `manager_test.go`, anchored on `prefix + "ssl.fail_verify_error",`, insert ABOVE it:

```go
		prefix + "ssl.connection_error",
```

so the slice reads:

```go
	want := []string{
		prefix + "ssl.connection_error",
		prefix + "ssl.fail_verify_error",
		prefix + "ssl.fail_verify_no_cert",
		prefix + "ssl.handshake",
		prefix + "ssl.no_certificate",
	}
```

Apply the identical insertion in `quic_test.go` at its own `prefix + "ssl.fail_verify_error",`.

- [ ] **Step 2: Rename the test — 4 sites, `Four` -> `Five`**

`TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames` -> `TestListenerMetrics_TLSListenerRegistersExactlyFiveSSLNames`, at `manager_test.go` sites `:2021 :2102 :2111 :4576` (verify each by symbol, not line).

```sh
git -C <worktree> grep -n 'RegistersExactlyFourSSLNames' -- 'internal/listener/*.go'   # expect exactly 4
sed -i 's/RegistersExactlyFourSSLNames/RegistersExactlyFiveSSLNames/g' internal/listener/manager_test.go
git -C <worktree> grep -n 'RegistersExactlyFourSSLNames' -- 'internal/listener/*.go'   # expect 0
```

⚠️ **PATHSPEC-SCOPED.** Historical occurrences in `75/PLAN.md` (10), `75/PROGRESS.md` (12), `DECISIONS.md` (1), `94/BRAINSTORM.md` (1), `94/SPEC.md` (2) and `next-prompt.txt` (1) **MUST NOT be renamed.**

- [ ] **Step 3: Run to verify they FAIL, and NC the selector first**

```sh
go test -v -count=1 ./internal/listener/ -run 'TestListenerMetrics_TLSListenerRegistersExactlyFiveSSLNames|TestQUICListener_RegistersSSLNamesAtZero' 2>&1 | tee /tmp/t1.log
grep -c '=== RUN' /tmp/t1.log     # MUST be non-zero — a selector matching nothing prints ok and exits 0
```

Expected: **FAIL**, both tests, `got` missing `ssl.connection_error` from `want`.

- [ ] **Step 4: Add the field**

In `manager.go`, anchored on `sslNoCertificate *stats.Counter`, add BELOW it. ⚠️ **`gofmt` realigns `sslNoCertificate` by one space** because `sslConnectionError` is longer (§0.9) — that one-line churn is expected and correct:

```go
	sslNoCertificate   *stats.Counter // phase 75: completed handshake, no client cert presented
	// sslConnectionError is phase 94's ERROR-PATH counter for the fourth outcome
	// bucket. It is the ONLY ssl.* counter that outcomeOther maps to
	// CONDITIONALLY: it Inc's on an SSL PROTOCOL error and stays put on a
	// TRANSPORT failure (see isTransportHandshakeErr). The handshakeOutcome
	// taxonomy stays FOUR while this scope's counters go to FIVE.
	sslConnectionError *stats.Counter // phase 94: handshake failed with an SSL PROTOCOL error
```

- [ ] **Step 5: Register it**, anchored on the `ssl.no_certificate` line inside the `rt.tlsMode` gate:

```go
		rt.sslNoCertificate = r.NewCounter(prefix + "ssl.no_certificate")
		rt.sslConnectionError = r.NewCounter(prefix + "ssl.connection_error")
```

- [ ] **Step 6: Run to verify PASS**

```sh
go test -v -count=1 ./internal/listener/ -run 'TestListenerMetrics_TLSListenerRegistersExactlyFiveSSLNames|TestQUICListener_RegistersSSLNamesAtZero' 2>&1 | tee /tmp/t1b.log
v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/t1b.log || true); echo "FAIL=$v"   # want 0
grep -c '=== RUN' /tmp/t1b.log                                                       # want non-zero
gofmt -l internal/listener/    # gate on OUTPUT — want empty
```

- [ ] **Step 7: Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go internal/listener/quic_test.go
git commit -m "phase 94 (tls-connection-error-stat) T1: register the fifth listener-scope ssl.* counter"
```

---

### Task 2: The nil gate — the fifth `GateMatchesInc` assertion, in the SAME commit as registration lands

⚠️ **The omission's failure mode is a PROCESS CRASH, and a PASSING test does not catch it.** EXECUTED: deleting the registration while keeping the `Inc` SIGSEGVs the process **while `TestListenerMetrics_GateMatchesInc` PASSES**.

**Files:** Modify/Test: `internal/listener/manager_test.go`

- [ ] **Step 1: Extend BOTH arms.** The TLS arm asserts non-nil, the plaintext arm asserts nil. Anchored on the existing `sslNoCertificate` assertion in each arm:

TLS arm (`~:2281-2298`):
```go
	if rt.sslConnectionError == nil {
		t.Errorf("TLS listener: rt.sslConnectionError is NIL — Inc would panic the serveConnection goroutine")
	}
```

Plaintext arm (`~:2340-2354`):
```go
	if rt.sslConnectionError != nil {
		t.Errorf("plaintext listener: rt.sslConnectionError is NON-NIL — the tlsMode gate leaked")
	}
```

⚠️ **Use `t.Errorf`, not `t.Fatalf`** — `reference_fatalf_makes_assertions_unreachable`: a `Fatalf` on the first pointer makes every later pointer assertion dead code.

- [ ] **Step 2: Fix the two stale headers §0.15 found.** They sit OUTSIDE the edit ranges and a literal reading of the SPEC misses them:

`manager_test.go:2241`: *"all **FOUR** counter fields are NON-NIL (three phase-74 outcomes + phase 75's sslNoCertificate)"* -> **FIVE** *(three phase-74 outcomes + phase 75's sslNoCertificate + phase 94's sslConnectionError)*.
`manager_test.go:2302`: *"all **FOUR** counter fields are NIL"* -> **FIVE**.

- [ ] **Step 3: NC it — prove the assertion does work.** Neutralise (do NOT revert; a build break proves nothing): comment out only the registration line from Task 1 Step 5.

```sh
go test -v -count=1 ./internal/listener/ -run 'TestListenerMetrics_GateMatchesInc' 2>&1 | tee /tmp/t2nc.log
grep -c '=== RUN' /tmp/t2nc.log     # want 4 — the selector selects the parent + both subtests
```

Expected: **FAIL**, `tls_listener` subtest, `rt.sslConnectionError is NIL — Inc would panic the serveConnection goroutine`; `plaintext_listener` PASSES. Restore the line.

- [ ] **Step 4: Verify PASS after restore**, same command, `FAIL=0`, `=== RUN` = 4.

- [ ] **Step 5: Commit**

```bash
git add internal/listener/manager_test.go
git commit -m "phase 94 (tls-connection-error-stat) T2: the fifth GateMatchesInc nil/non-nil assertion on both arms"
```

---

### Task 3: `helpText` + `helpTextRoster` + `wantNames` — a MATCHED TRIPLE, ONE commit

⚠️ **`helpText` and `helpTextRoster` are a matched pair: EITHER ALONE REDDENS `internal/stats`.** The `name_test.go` slice is the third member and is **SILENTLY GREEN if omitted** — that is the documented gap, and it is exactly why it is mandatory.

**Files:** Modify `internal/stats/name.go`; Test `internal/stats/helptext_test.go`, `internal/stats/name_test.go`

- [ ] **Step 1: `helpText` entry, FIRST within the `ssl.*` group.** Anchored on `"envoy_listener_ssl_handshake":`, insert ABOVE it. ⚠️ **Hand-align to the group's existing column** — §0.9 measured that the new key is SHORTER than the incumbent longest, so gofmt rewrites nothing if you align it:

```go
	"envoy_listener_ssl_connection_error":    "Downstream TLS handshakes failed with an SSL protocol error.",
	"envoy_listener_ssl_handshake":           "Total successful downstream TLS handshakes on the listener.",
```

⚠️ **NOT at EOF** — an EOF append breaks the tail-anchored doc clause *"the last five are phase 80's `sds.` root"*.

- [ ] **Step 2: Update the doc-comment enumeration** in the same file: *"Of the **30** entries"* -> **31**; *"the next three are the phase-74 listener-scope TLS handshake outcomes; then phase 75's listener-scope `ssl.no_certificate`"* gains phase 94's `ssl.connection_error`; and *"**All four** `ssl.*` entries have three-dot source names"* -> **All five**.

- [ ] **Step 3: `helpTextRoster` twin** in `helptext_test.go`, matching the internal (dotted) form used by its siblings — read the exact neighbouring form before writing:

```go
	"listener.<addr>.ssl.connection_error",
```

- [ ] **Step 4: `wantNames` slice** in `name_test.go` — the Prometheus projection, **position 0** (it prepends):

```go
		"envoy_listener_ssl_connection_error",
		"envoy_listener_ssl_fail_verify_error",
```

- [ ] **Step 5: Rewrite the stale trap comment** at `name_test.go:232-237`. Its cited evidence is FALSE at this tip: it claims that at the phase-75 PLAN a fifth `helpText` entry with the slice left short kept *"the whole package GREEN"*. `helptext_test.go` post-dates phase 75 and now reddens a bare `helpText` addition. Replace the evidence clause with: *"the silent gap is now keyed on `helpTextRoster`: with BOTH the entry and the roster present and this slice left short, the package stays GREEN (NC 6, phase 94) — which is why this slice is mandatory and unguarded."*

- [ ] **Step 6: Run the three NCs.** Each neutralises, never reverts:

```sh
run_stats () { go test -v -count=1 ./internal/stats/ 2>&1 | tee "$1"; \
  echo "RUN=$(grep -c '=== RUN' "$1")  FAIL=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' "$1" || true)"; }
```

| NC | mutation | MUST produce |
|---|---|---|
| **4** | entry present, roster entry removed | **RED**, 1 test: `TestHelpText_KeySetExact`, `helptext_test.go:141 … extra: [envoy_listener_ssl_connection_error]` |
| **5** | roster present, entry removed | **RED, TWO tests** — `TestHelpText_KeySetExact` (`:139 missing:`) **AND `TestHelpText_NoSelfEqualHelp`** (`:202 1 rendered HELP line(s) degraded to the metric name`). ⚠️ Two, not one (§0.16) |
| **6** | both present, slice left at FOUR | ⚠️ **GREEN — `RC=0 RUN=146 FAIL=0`.** This is the documented silent gap, reproduced deliberately |

- [ ] **Step 7: Verify the full triple is green, and gofmt prints nothing**

```sh
go test -v -count=1 ./internal/stats/ 2>&1 | tee /tmp/t3.log
grep -c '=== RUN' /tmp/t3.log    # want 146
v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/t3.log || true); echo "FAIL=$v"   # want 0
gofmt -l internal/stats/          # gate on OUTPUT — want EMPTY (hand-aligned, §0.9)
```

- [ ] **Step 8: Commit** — all three members together.

```bash
git add internal/stats/name.go internal/stats/helptext_test.go internal/stats/name_test.go
git commit -m "phase 94 (tls-connection-error-stat) T3: helpText + helpTextRoster + wantNames as one matched triple"
```

---

### Task 4: The predicate — `isTransportHandshakeErr` and its PREDICATE-level table

**Files:** Modify `internal/listener/manager.go`; Test `internal/listener/manager_test.go`

**Interfaces:**
- Produces: `func isTransportHandshakeErr(err error) bool` — package-level in `internal/listener`. Task 5 consumes it.

- [ ] **Step 1: Write the failing predicate table.** Build it on PRODUCTION-REPRESENTATIVE values from live handshakes, never hand-written strings (§0 of the SPEC, refutation 1). Reuse `connPair` (`manager_test.go:4270`, binds `127.0.0.1:0` — **use port 0, not a banded port**) and the `liveHandshakeErr` idiom at `:4326`.

```go
func TestIsTransportHandshakeErr(t *testing.T) {
	// Each arm drives a REAL server-side crypto/tls handshake failure over a
	// loopback TCP pair — never net.Pipe, which deadlocks a client-cert
	// handshake. The values are what production actually produces.
	cases := []struct {
		name        string
		drive       func(t *testing.T) error // returns the SERVER-side handshake error
		wantIsTrans bool
	}{
		{"bad_version_TLS11_client", driveBadVersion, false},
		{"plaintext_http", drivePlaintext, false},
		{"garbage_bytes", driveGarbage, false},
		{"partial_hello_then_FIN", drivePartialThenFIN, true},
		{"zero_bytes_then_FIN", driveZeroThenFIN, true},
		{"partial_then_RST", drivePartialThenRST, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.drive(t)
			if err == nil {
				t.Fatalf("%s: handshake unexpectedly SUCCEEDED — the arm is vacuous", tc.name)
			}
			if got := isTransportHandshakeErr(err); got != tc.wantIsTrans {
				t.Errorf("%s: isTransportHandshakeErr(%T %q) = %v, want %v",
					tc.name, err, err.Error(), got, tc.wantIsTrans)
			}
			// NON-VACUITY: every arm must classify to outcomeOther, or it is
			// pinning a different question than this row's.
			if o := classifyHandshakeErr(err); o != outcomeOther {
				t.Errorf("%s: classifyHandshakeErr = %v, want outcomeOther", tc.name, o)
			}
		})
	}
}
```

⚠️ **Use `t.Errorf` per property, not `t.Fatalf`** — otherwise the non-vacuity check becomes dead code whenever the predicate assertion fails.

- [ ] **Step 2: Add the identity-vs-text NC as a permanent test (NC 2).** This one stays in the tree — it is what proves the table reads IDENTITY and not TEXT:

```go
func TestIsTransportHandshakeErr_ReadsIdentityNotText(t *testing.T) {
	// syscall.ECONNRESET.Error() is BYTE-IDENTICAL to this string, but a bare
	// errors.New carrying that text is a *errors.errorString with no Is/Unwrap,
	// so errors.Is is FALSE. If this test ever flips, the predicate has started
	// matching message text — the exact practice the design forbids.
	synth := errors.New("connection reset by peer")
	if synth.Error() != syscall.ECONNRESET.Error() {
		t.Fatalf("precondition lost: the synthetic text no longer matches the sentinel's")
	}
	if isTransportHandshakeErr(synth) {
		t.Errorf("isTransportHandshakeErr matched a SYNTHETIC string error — it is reading TEXT, not identity")
	}
}
```

- [ ] **Step 3: Run to verify FAIL** (`isTransportHandshakeErr` undefined — a compile error is the expected red here):

```sh
go test -count=1 ./internal/listener/ 2>&1 | head -20   # expect: undefined: isTransportHandshakeErr
```

- [ ] **Step 4: Add the predicate** to `manager.go`, verbatim from §3.1, immediately below `classifyHandshakeErr`. Add `"context"`, `"io"`, `"net"`, `"syscall"` to the import block as needed.

- [ ] **Step 5: Run to verify PASS**

```sh
go test -v -count=1 ./internal/listener/ -run 'TestIsTransportHandshakeErr' 2>&1 | tee /tmp/t4.log
grep -c '=== RUN' /tmp/t4.log   # want 8 (parent + 6 subtests + the identity test)
v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/t4.log || true); echo "FAIL=$v"   # want 0
```

- [ ] **Step 6: NC the predicate itself** — neutralise `isTransportHandshakeErr` to `return false`. Expected: the three transport arms go RED. Restore.

- [ ] **Step 7: Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 94 (tls-connection-error-stat) T4: isTransportHandshakeErr + its production-representative predicate table"
```

---

### Task 5: ⚠️ THE COUNTER TABLE — the load-bearing task of this plan

This is the task that makes the PREDICATE falsifiable. `SPEC.md` §6.2 believed `sslLeafRoster` did that job; §0.1 proved it cannot and never could. **Without this task the row ships a predicate no test can falsify.**

**Files:** Modify `internal/listener/manager.go` (the `Inc` arm); Test `internal/listener/manager_test.go`

**Interfaces:**
- Consumes: `isTransportHandshakeErr` (T4), `rt.sslConnectionError` (T1).
- Produces: `TestServeConnection_SSLConnectionErrorCounter` and its stacked control.

- [ ] **Step 1: Write the failing counter table.** It drives the REAL listener and the REAL `Inc` site — not the classifier directly. Use the existing `startOneWayTLSListener` helper.

⚠️ **THE RELEASE BARRIER IS MANDATORY AND THERE ARE NO SLEEPS.** Poll `downstream_cx_total` up to N, then poll `downstream_cx_active` back to 0. That is sound because `serveConnection` defers `downstreamCxActive.Dec()` at `manager.go:1245`, which runs AFTER the outcome switch — so observing active back at 0 proves the switch has already executed.

```go
// awaitDrained is the release barrier: it waits until `want` connections have
// been accepted AND all of them have finished serveConnection. Polling the
// gauge is sound because serveConnection defers downstreamCxActive.Dec()
// (manager.go:1245), which runs AFTER the handshake-outcome switch — so
// active==0 proves every outcome has been booked. NO SLEEPS.
//
// ⚠️ gaugeValue does NOT exist in this package — counterValue (quic_test.go:66)
// is counter-only. This task must ADD gaugeValue alongside it, in the same
// reg.Walk shape. ⚠️ NEVER register a stat inside Registry.Walk: the callback
// runs under RLock and registering re-enters the write lock, DEADLOCKING the
// process (reference_registry_walk_lock_inversion).
func gaugeValue(t *testing.T, reg *stats.Registry, name string) int64 {
	t.Helper()
	var (
		val   int64
		found bool
	)
	reg.Walk(func(m stats.Metric) {
		if m.Name() != name {
			return
		}
		// ⚠️ Gauge's accessor is Load() int64 — there is NO Value() method
		// (internal/stats/gauge.go:56). Mirror counterValue's type-assertion
		// shape rather than switching on Type().
		if g, ok := m.(*stats.Gauge); ok {
			val = g.Load()
			found = true
		}
	})
	if !found {
		t.Errorf("gauge %q is not registered", name)
		return -1
	}
	return val
}

func awaitDrained(t *testing.T, reg *stats.Registry, addr string, want int64) {
	t.Helper()
	prefix := "listener." + normalizeAddr(addr) + "."
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if counterValue(t, reg, prefix+"downstream_cx_total") >= want &&
			gaugeValue(t, reg, prefix+"downstream_cx_active") == 0 {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("release barrier timed out: downstream_cx_total never reached %d with active back at 0", want)
}
```

```go
func TestServeConnection_SSLConnectionErrorCounter(t *testing.T) {
	// Every arm drives ONE real connection into a REAL listener and asserts the
	// fifth counter AND that no other ssl.* leaf moved. Arms are single-cause.
	// ⚠️ counterValue returns int64 (quic_test.go:66), NOT uint64. Keep `want`
	// int64 or the comparison will not compile.
	cases := []struct {
		name string
		dial func(t *testing.T, addr string) // drives exactly one connection
		want int64
	}{
		{"bad_version_TLS11_client", dialMaxTLS11, 1},
		{"plaintext_http", dialPlaintextHTTP, 1},
		{"garbage_bytes", dialGarbage, 1},
		{"partial_hello_then_FIN", dialPartialThenFIN, 0},
		{"zero_bytes_then_FIN", dialZeroThenFIN, 0},
		{"partial_then_RST", dialPartialThenRST, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// ⚠️ TRUE SIGNATURE: startOneWayTLSListener(t, pki) (*stats.Registry, string)
			// — manager_test.go:4730. It takes a handshakeTestPKI and returns only
			// (reg, addr); teardown is registered internally via t.Cleanup. There is
			// NO stop func to defer. pki comes from mkTestPKI(t) (:4238), the shape
			// TestServeConnection_SSLFailVerifyErrorIncrements uses at :4675.
			pki := mkTestPKI(t)
			reg, addr := startOneWayTLSListener(t, pki)
			tc.dial(t, addr)
			awaitDrained(t, reg, addr, 1)

			got := counterValue(t, reg, "listener."+normalizeAddr(addr)+".ssl.connection_error")
			if got != tc.want {
				t.Errorf("%s: ssl.connection_error = %d, want %d", tc.name, got, tc.want)
			}
			// ⚠️ ASSERT WHICH DID NOT FIRE. A pin proving connection_error moved
			// says nothing about whether a cert counter also moved.
			for _, leaf := range []string{"handshake", "fail_verify_error", "fail_verify_no_cert", "no_certificate"} {
				if v := counterValue(t, reg, "listener."+normalizeAddr(addr)+".ssl."+leaf); v != 0 {
					t.Errorf("%s: ssl.%s = %d, want 0 — only connection_error may move on this arm", tc.name, leaf, v)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Write the STACKED negative control — MANDATORY**

⚠️ **A bare `== 0` exclusion arm cannot distinguish "the predicate excluded it" from "nothing ran at all."** `assertSSLCrossProduct` cannot express a pure negative arm — it `t.Fatalf`s on zero `wantSuffixes` (`:4614`), with a comment explaining that such a call passes vacuously. The stacked shape is the fix:

```go
func TestServeConnection_SSLConnectionErrorCounter_Stacked(t *testing.T) {
	// The three EXCLUDED arms then ONE INCLUDED arm, on the SAME listener.
	// A bare `== 0` on an exclusion arm is indistinguishable from "nothing ran";
	// stacking makes the excluded arms observable — they must contribute ZERO to
	// a counter that a single later arm drives to EXACTLY 1.
	// NC EVIDENCE: with the predicate removed this reads 4; with the Inc removed
	// it reads 0. Only the correct implementation reads 1.
	pki := mkTestPKI(t)
	reg, addr := startOneWayTLSListener(t, pki)

	dialPartialThenFIN(t, addr)
	dialZeroThenFIN(t, addr)
	dialPartialThenRST(t, addr)
	dialPlaintextHTTP(t, addr)
	awaitDrained(t, reg, addr, 4)

	if got := counterValue(t, reg, "listener."+normalizeAddr(addr)+".ssl.connection_error"); got != 1 {
		t.Errorf("stacked: ssl.connection_error = %d, want 1 "+
			"(3 transport arms must contribute 0, 1 protocol arm must contribute 1)", got)
	}
}
```

- [ ] **Step 3: Run to verify FAIL** — the `case outcomeOther:` arm does not exist yet, so every inclusion arm reads 0 and the stacked control reads 0.

```sh
go test -v -count=1 ./internal/listener/ -run 'TestServeConnection_SSLConnectionErrorCounter' 2>&1 | tee /tmp/t5.log
grep -c '=== RUN' /tmp/t5.log   # want 9 (2 parents + 6 subtests + stacked); NEVER accept 0
```

- [ ] **Step 4: Land the `Inc` arm.** Anchored on `case outcomeNoCert:` in the handshake-error switch:

```go
			switch classifyHandshakeErr(err) {
			case outcomeVerifyError:
				rt.sslFailVerifyError.Inc()
			case outcomeNoCert:
				rt.sslFailVerifyNoCert.Inc()
			case outcomeOther:
				// phase 94: the reference books ssl.connection_error IFF BoringSSL
				// reports an SSL PROTOCOL error; a TRANSPORT EOF or reset books
				// NOTHING under ssl.*. The predicate matches the closed transport
				// COMPLEMENT and we Inc otherwise (ADR-0316).
				if !isTransportHandshakeErr(err) {
					rt.sslConnectionError.Inc()
				}
			}
```

- [ ] **Step 5: Run to verify PASS** — same command, `FAIL=0`, `=== RUN` = 9.

- [ ] **Step 6: ⚠️ NC IT BOTH WAYS — this is what makes it a GUARD rather than a passing test**

| NC | mutation (neutralise, keep it compiling) | MUST produce |
|---|---|---|
| **1** | remove the predicate: `rt.sslConnectionError.Inc()` unconditional inside `case outcomeOther:` | **RED** — `partial_hello_then_FIN`, `zero_bytes_then_FIN`, `partial_then_RST` each read 1 want 0; **stacked reads 4, want 1** |
| **1b** | remove the `Inc`: replace the body with `_ = rt.sslConnectionError` | **RED** — all three inclusion arms read 0 want 1; **stacked reads 0, want 1** |

⚠️ **`SPEC.md` §8 NC 1's instruction to "run it both ways" against `sslLeafRoster` is REPLACED by this table.** The roster 2×2 is REFUTED (§0.1); this pair is the real control. Both were EXECUTED at the PLAN stage and both fire.

- [ ] **Step 7: Reachability control — prove the site is exercised**

⚠️ **A green run is not evidence a site is exercised.** Temporarily replace the `Inc` with `panic("REACHED")` and confirm the package panics, naming which tests reach it. Expected reachers: the six new inclusion/exclusion arms, the stacked control, and `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` (§0.2). Restore.

- [ ] **Step 8: Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 94 (tls-connection-error-stat) T5: the predicate-gated Inc and the counter table that falsifies it"
```

---

### Task 6: `sslLeafRoster` -> five, and its ISOLATING NC

⚠️ **Its purpose is NOT what the SPEC says.** It guards the **Inc SITE** — it is the only thing that catches an `Inc` leaking onto the cert arms — and it couples to registration through `counterValue`'s absent-name `t.Errorf`. It does NOT guard the predicate (§0.1).

**Files:** Test `internal/listener/manager_test.go`

- [ ] **Step 1: Extend the roster**, anchored on `var sslLeafRoster = []string{`:

```go
var sslLeafRoster = []string{"handshake", "fail_verify_error", "fail_verify_no_cert", "no_certificate", "connection_error"}
```

- [ ] **Step 2: Update the roster's doc comment** to record what it does and does not guard:

```go
// ⚠️ This roster guards the Inc SITE, not the PREDICATE. Extending it catches an
// Inc that leaks onto the cert arms (measured: hoisting the Inc above the switch
// reddens the two cert tests at :4694/:4712). It CANNOT catch a broken predicate,
// because no call site here reaches outcomeOther — see
// TestServeConnection_SSLConnectionErrorCounter for that guard (phase 94).
```

- [ ] **Step 3: ⚠️ THE ISOLATING NC — prove the roster's negative half is live.** A NC that leaves your control green proves nothing; this is the second, isolating NC. Add `rt.sslConnectionError.Inc()` to the `outcomeVerifyError` arm ONLY:

```sh
go test -v -count=1 ./internal/listener/ -run 'TestServeConnection_SSLFailVerify' 2>&1 | tee /tmp/t6nc.log
grep -c '=== RUN' /tmp/t6nc.log   # want non-zero
```

Expected: `TestServeConnection_SSLFailVerifyErrorIncrements` **RED** with `manager_test.go:4694: … ssl.connection_error = 1, want 0 — only [fail_verify_error] may move on this arm`, and `…NoCertIncrements` **GREEN**. That asymmetry is the proof the roster discriminates per-arm. Restore.

- [ ] **Step 4: Verify green**, then **commit**

```bash
git add internal/listener/manager_test.go
git commit -m "phase 94 (tls-connection-error-stat) T6: sslLeafRoster to five — guarding the Inc SITE, with its isolating NC"
```

---

### Task 7: `manager.go` prose — SIX blocks, and ONE guarded NON-site

**Files:** Modify `internal/listener/manager.go`

⚠️ **A naive `four` sweep is WRONG.** An exhaustive case-insensitive `four` sweep of `manager.go` returns exactly **6 lines** (175, 177, 364, 393, 399, 414); five belong to real sites and `:414` must be LEFT ALONE.

- [ ] **Step 1: Edit the six blocks**

| site | change |
|---|---|
| `:175-177` | *"the four `ssl.*` counters … all four pointers stay NIL"* -> **five** / **all five** |
| `:363-364` | *"the one phase-75 `ssl.*` counter (`ssl.no_certificate` — **four in total**)"* -> add phase 94's, **five in total** |
| `:392-393` | *"the three counted buckets plus a fourth that **counts NOTHING**"* -> the fourth now counts CONDITIONALLY; state the FOUR-outcomes / FIVE-counters asymmetry |
| `:398-400` | *"the listener scope carries **FOUR** `ssl.*` counters as of phase 75"* -> **FIVE** as of phase 94 |
| `:408-412` | *"land in `outcomeOther`, **which increments nothing**. The reference books those under `ssl.connection_error`; that asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5)"* -> the departure is **CLOSED**; `outcomeOther` now increments `ssl.connection_error` when the error is not a transport failure |
| `:1291-1293` | *"`outcomeOther` deliberately increments NOTHING — … a name this row does not land. That asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5)"* -> superseded by the `case outcomeOther:` arm landed in T5 |

- [ ] **Step 2: ⚠️ DO NOT EDIT `:414`** — *"`crypto/tls` exports **four** error TYPES and ZERO error VALUES"*. That `four` counts `crypto/tls` error types, not `ssl.*` counters, and it stays TRUE.

- [ ] **Step 3: Fix the phantom `B5` at `:412` and `:1293`.** Both cite *"BEHAVIOR_CONTRACT B5"*. **There is no B-numbered step scheme for this**: the file's `B5` hits are `:1971` (the narration itself), eight `AMEND-B5` contexts, and an unrelated phase-93 break label at `:5964` (§0.12). Replace with the real anchor — the subsection heading **`### Downstream TLS handshake-outcome stats`** (`BEHAVIOR_CONTRACT.md:1965`). ⚠️ **The propagated copy at `DECISIONS.md:17316` is NOT edited** (append-only, ADR-0288 §Decision 4); ADR-0316 records the correction.

- [ ] **Step 4: Sweep British spellings** in every comment you touched — `golangci-lint`'s misspell runs in locale US and has fired on three consecutive stages' prototypes.

- [ ] **Step 5: Verify and commit**

```sh
gofmt -l internal/listener/                       # gate on OUTPUT — want empty
golangci-lint run ./internal/listener/...         # gate on OUTPUT — want empty
git -C <worktree> grep -n -i 'four' -- 'internal/listener/manager.go'   # expect exactly 1 line: :414
```

```bash
git add internal/listener/manager.go
git commit -m "phase 94 (tls-connection-error-stat) T7: the six manager.go prose blocks + the phantom B5 anchor fix"
```

---

### Task 8: Test prose — the SPEC's list PLUS the seven lines §0.15 found

**Files:** Test `internal/listener/manager_test.go`, `internal/listener/quic_test.go`

- [ ] **Step 1: The SPEC's listed ranges** — `manager_test.go` `:2016-2021 :2068-2072 :2102-2110 :2349-2351 :4569-4579 :4596-4604 :4809`; `quic_test.go` `:225-226 :234 :275-281 :303`. Update every `four`/`FOUR` that counts `ssl.*` names or pointers.

- [ ] **Step 2: The seven lines the SPEC missed** (§0.15) — `manager_test.go:2079-2080`, `:2215`, `quic_test.go:65`, `:273-274`. ⚠️ `manager_test.go:2241` and `:2302` were already handled in **Task 2 Step 2**; confirm they were.

- [ ] **Step 3: Prove the sweep is complete, both files**

```sh
git -C <worktree> grep -n -i -- 'four' -- 'internal/listener/manager_test.go' 'internal/listener/quic_test.go'
```

Every surviving hit must be justified in the commit message — a `four` that counts something other than `ssl.*` names/pointers is legitimate; anything else is a miss.

- [ ] **Step 4: Verify green and commit**

```sh
go test -v -count=1 ./internal/listener/ 2>&1 | tee /tmp/t8.log
grep -c '=== RUN' /tmp/t8.log ; v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/t8.log || true); echo "FAIL=$v"
```

```bash
git add internal/listener/manager_test.go internal/listener/quic_test.go
git commit -m "phase 94 (tls-connection-error-stat) T8: test prose, including the seven sites the SPEC roster missed"
```

---

### Task 9: Fixture `0120` — the PKI

⚠️ **NO COMMITTED CERT IN THIS TREE CARRIES `clientAuth`.** All four `0002-tls-tcp` leaves are `TLS Web Server Authentication` only, and `0119`'s committed leaf is server-only. `0018` does mint a proper client cert, but via an `init()` generator whose outputs are `.gitignore`d (`test/fixtures/0018-http-rbac/pki/.gitignore`). **`0120` cannot reuse committed PKI and must ship its own.**

**DECISION: COMMIT the PEMs, following `0119`'s precedent** (`pki/ca.pem`, `pki/listener.pem`, `pki/listener.key.pem` committed, 20-year validity `2026-01-01` -> `2046-01-01`, read via a `readPEM` helper at `0119/driver/driver.go:116-123`). **Rejected: `0018`'s generator**, because it writes into the worktree at test time, which fights this project's "prove the tree clean" discipline and would make `git status --porcelain` dirty after every differential run.

**Files:** Create `test/fixtures/0120-tls-connection-error/pki/{ca.pem,server.pem,server.key.pem,client.pem,client.key.pem}`

- [ ] **Step 1: Generate once, with a throwaway program in scratch** (NOT committed). Mirror `0018/pki/gen.go`'s shapes: **P256** throughout; CA self-signed with `IsCA`, `KeyUsageCertSign`; server leaf `ExtKeyUsageServerAuth` + DNS SAN `localhost` + IP SANs `127.0.0.1`/`::1`; client leaf `ExtKeyUsageClientAuth` + a SPIFFE URI SAN. Validity `NotBefore 2026-01-01T00:00:00Z`, `NotAfter 2046-01-01T00:00:00Z` — matching `0119` exactly so the fixture does not expire mid-project.

- [ ] **Step 2: Verify every artefact before committing it**

```sh
for f in server client; do
  openssl x509 -in test/fixtures/0120-tls-connection-error/pki/$f.pem -noout -dates -ext extendedKeyUsage,subjectAltName
done
openssl verify -CAfile test/fixtures/0120-tls-connection-error/pki/ca.pem \
  test/fixtures/0120-tls-connection-error/pki/server.pem \
  test/fixtures/0120-tls-connection-error/pki/client.pem
```

Expected: server prints `TLS Web Server Authentication`, client prints `TLS Web Client Authentication`, both verify OK, both dated 2026->2046. ⚠️ **A server leaf without `clientAuth` on the CLIENT cert makes the `require_client_certificate: true` positive arm fail for the wrong reason** — check this, do not assume it.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/pki/
git commit -m "phase 94 (tls-connection-error-stat) T9: fixture 0120 PKI — the tree's first committed clientAuth leaf"
```

---

### Task 10: Fixture `0120` — the two configs, and BOOT BOTH SIDES BEFORE ANY ARM

⚠️ **`SPEC.md` §16 item 2 makes this the gate on everything downstream, and it is now MEASURED rather than asserted: BOTH SIDES BOOT ON `tls_params`.** No YAML in this tree has ever shipped that block (`git grep -n 'tls_params' -- '*.yaml' '*.yml'` returns NO MATCHES). The measurement stands, but **re-run it at the IMPL's own tip before writing arms** — the boot is the precondition for every later task.

**Files:** Create `test/fixtures/0120-tls-connection-error/envoy.yaml`, `test/fixtures/0120-tls-connection-error/envoy-go.yaml`

- [ ] **Step 1: Write `envoy.yaml` (the REFERENCE side).** Template variables follow the `0118`/`0110` idiom.

⚠️ **PEMs go in as `inline_string:`, NOT `filename:` (D-TLSCE-CERTDELIVERY, §0.4).** A `filename:` path exists on the host but NOT inside the reference container. ⚠️ **Pre-indent every PEM block to the exact column** — `0110/envoy.yaml:42` records them "Pre-indented to 24 spaces". ⚠️ **Never put a PEM inside a `#` comment** — `0108/envoy.yaml:40-45` records that continuation lines splatter outside the comment and yield invalid YAML.

⚠️ **The reference cluster MUST be `STRICT_DNS` + `host.docker.internal`, NOT `STATIC` by bridge IP.** MEASURED: `docker network inspect bridge` reports gateway `172.17.0.1`, but `ip addr show docker0` shows **nothing** — Docker here is VM-backed and `host.docker.internal` resolves to `192.168.65.2`. A `STATIC`-by-bridge-IP reference cluster is **silently unreachable**, and §0's finding (c) shows the failure mode is a *silently zeroed positive control*, not a boot error. This is the repo's standing ADR-0010 convention; it refines rather than contradicts the general "prefer STATIC by IP" guidance, which holds only where the name would not resolve.

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} }

static_resources:
  listeners:
    - name: l_conn_err
      address:
        socket_address: { address: 0.0.0.0, port_value: {{.ListenerPort}} }
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              require_client_certificate: true
              common_tls_context:
                tls_params:
                  tls_minimum_protocol_version: TLSv1_2
                tls_certificates:
                  - certificate_chain:
                      inline_string: |
{{.ServerCertIndented}}
                    private_key:
                      inline_string: |
{{.ServerKeyIndented}}
                validation_context:
                  trusted_ca:
                    inline_string: |
{{.CACertIndented}}
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_0120
                cluster: c_echo

  clusters:
    - name: c_echo
      type: STRICT_DNS
      dns_lookup_family: V4_ONLY
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: host.docker.internal, port_value: {{.BackendPort}} }
```

- [ ] **Step 2: Write `envoy-go.yaml` (the SUBJECT side)** — identical except the admin port, and the cluster is `STATIC` on `127.0.0.1:{{.BackendPort}}` (the subject runs on the host, so there is no container boundary and no DNS indirection).

⚠️ **`TLSv1_0`/`TLSv1_1` MUST NOT appear in either YAML.** `internal/tls/params.go:62-83` returns `"%s is not supported in phase 03"` and **boot-rejects envoy-go**. The bad-version arm is produced **CLIENT-side only** (Task 12 arm i).

- [ ] **Step 3: ⚠️ BOOT BOTH SIDES ON THESE EXACT FILES BEFORE WRITING ANY ARM**

```sh
# SUBJECT — note -c, not --config-path; build with -o into scratch
go build -o "$SCRATCH/envoy-go" ./cmd/envoy-go/
"$SCRATCH/envoy-go" -c "$SCRATCH/0120-envoy-go.yaml" 2>&1 | tee "$SCRATCH/subj-boot.log" &
# REFERENCE — pin BY DIGEST; -p publishing WORKS, --network host does NOT
docker run -d --name p94-t10-ref \
  --add-host=host.docker.internal:host-gateway \
  -p 12126:10126 -p 12901:9901 \
  envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8 \
  -c /etc/envoy/envoy.yaml
```

⚠️ **Verify the digest against `docs/envoy-go/ENVOY_TARGET.md:4` before trusting any arm** (the file is at `docs/envoy-go/ENVOY_TARGET.md`, NOT the repo root).

**Acceptance — discriminate on OUTPUT, never on the exit code** (`timeout` exit 124 is shared by a healthy server and a hung boot):
- Subject log contains `listener l_conn_err ready on` and `envoy-go ready`.
- Reference log contains `loading 1 listener(s)` and `starting main dispatch loop`, with **zero** warnings naming `tls_params`.
- `curl -s localhost:12901/config_dump | grep -c tls_minimum_protocol_version` -> **non-zero** — proving the block is RETAINED, not merely tolerated.

- [ ] **Step 4: Tear down BY NAME, only what you created**

```sh
docker rm -f p94-t10-ref     # BY NAME. Never touch curl-world-* or reaper_*.
kill "$SUBJ_PID"             # the PID you captured. NEVER pkill -f.
```

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/envoy.yaml test/fixtures/0120-tls-connection-error/envoy-go.yaml
git commit -m "phase 94 (tls-connection-error-stat) T10: fixture 0120 configs — the tree's first tls_params YAML, booted on both sides"
```

---

### Task 11: Fixture `0120` — driver skeleton and ALL THREE registration gates

⚠️ **A missing blank import is `t.Skipf`'d SILENTLY GREEN (`runner_test.go:200`) and NO fixture-count gate exists anywhere in the tree.**

**Files:** Create `test/fixtures/0120-tls-connection-error/driver/driver.go`; Modify `test/differential/runner_test.go` (ONE line)

**Interfaces:**
- Produces: `fixtureName = "0120-tls-connection-error"`, `refListenerPort = 10126`, `type connErrDriver struct{}` implementing `fixture.Driver` and `fixture.StatsAsserter`.

- [ ] **Step 1: Gate 1 — the directory.** `test/fixtures/0120-tls-connection-error/` with a `driver/` subdirectory. ⚠️ **This tree has TWO layouts — 97 fixtures use `<dir>/driver/` and 24 use `<dir>/inputs/`; a naive `driver`-only roster reports 24 phantom failures. `0120` uses `driver/`.**

- [ ] **Step 2: Gate 2 — `RegisterFixture` from `init()`**, name STRING-EQUAL to the directory name:

```go
const (
	fixtureName = "0120-tls-connection-error"

	// In-container reference Envoy ports. The "10<fixture index>" convention
	// yields 10120 for 0120 — but 0028 HOLDS 10120 as part of its 10120-10125
	// run (0028/inputs/driver.go:65-70). 10126 is the minimal index-preserving
	// repair: the first free port above the occupying run. Verified free (zero
	// hits in test/, *.go, *.yaml) and ALREADY LANDED for this fixture by phase
	// 92 (92/SPEC.md:79, :393; 92/PLAN.md:823).
	// ⚠️ Do NOT cite 0118:31's "10450 is the TLS/SDS band" carve-out: its premise
	// is measurably false — 0112 and 0113 carry ZERO DownstreamTlsContext.
	refAdminPort    = 9901
	refListenerPort = 10126
)

type connErrDriver struct{}

func init() { fixture.RegisterFixture(fixtureName, &connErrDriver{}) }
```

- [ ] **Step 3: Gate 3 — the blank import.** ⚠️ **The ONLY file outside the fixture directory that this row touches.** In `test/differential/runner_test.go`, after the `0119` line:

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0120-tls-connection-error/driver"
```

- [ ] **Step 4: The mandatory compile-time interface assertions.** ⚠️ **`AssertStats` dispatch is an UNGUARDED type assertion with no `else` (`runner_test.go:1349`) — a signature typo makes `ok == false` and the ENTIRE stats leg VANISHES GREEN.** `0118/driver/driver.go:588-594` marks this MANDATORY:

```go
// Compile-time interface assertions. ⚠️ The StatsAsserter one is MANDATORY: the
// runner dispatches the stats step via a SILENT type assertion (runner_test.go:1349,
// no else branch), so a signature typo makes ok == false and the whole assertion
// NEVER RUNS while every tool stays quiet.
var (
	_ fixture.Driver        = (*connErrDriver)(nil)
	_ fixture.StatsAsserter = (*connErrDriver)(nil)
)
```

- [ ] **Step 5: `BackendCount` and the remaining `fixture.Driver` surface.** ⚠️ **`BackendCount()` MUST return >= 1** — `runner_test.go:242-245` `t.Fatalf`s on 0. `0118/driver/driver.go:64-68` is the precedent for a fixture driving no backend traffic.

```go
// BackendCount stays 1: the failing arms never reach the upstream, but the
// runner rejects 0. The default TCPEcho kind is the minimum viable shape and
// the positive arm's echo round-trip DOES traverse it — see AssertStats for why
// that round-trip is load-bearing rather than decorative. +0 BackendKinds.
func (*connErrDriver) BackendCount() int           { return 1 }
func (*connErrDriver) SubjectListenerName() string { return "l_conn_err" }
func (*connErrDriver) ReferenceListenerPort() int  { return refListenerPort }
```

- [ ] **Step 6: ⚠️ ASSERT THE FIXTURE SET BY NAME, IN BOTH DIRECTIONS.** No count gate exists, so prove the reconciliation mechanically:

```sh
ls -d test/fixtures/*/ | sed 's|test/fixtures/||; s|/$||' | sort > /tmp/dirs.txt
grep -o 'test/fixtures/[^/]*/driver' test/differential/runner_test.go | sed 's|test/fixtures/||; s|/driver||' | sort > /tmp/imports.txt
comm -23 /tmp/dirs.txt /tmp/imports.txt   # dirs with no import — MUST be empty except the 24 inputs/ fixtures
comm -13 /tmp/dirs.txt /tmp/imports.txt   # imports with no dir  — MUST be EMPTY
grep -c '0120-tls-connection-error' test/differential/runner_test.go   # want 1
```

⚠️ **Account for the two-layout trap:** the 24 `inputs/` fixtures import `…/inputs`, not `…/driver`. Extract BOTH suffixes, or the first `comm` reports 24 phantoms. Baseline before this task: **121 = 121, both directions empty.** After: **122 = 122.**

- [ ] **Step 7: NC the registration.** Comment out the blank import and run the fixture selector. Expected: `t.Skipf("no driver registered for fixture …")` — **a SILENT GREEN**, `exit 0`. That is the failure mode this step exists to make visible. Restore, then confirm the set-difference above returns to empty.

- [ ] **Step 8: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/driver/driver.go test/differential/runner_test.go
git commit -m "phase 94 (tls-connection-error-stat) T11: fixture 0120 driver skeleton + all three registration gates"
```

---

### Task 12: Fixture `0120` — the five drive arms, ALL inside the SINGLE `Drive` pair

⚠️ **NO PRECEDENT EXISTS FOR THIS DRIVE LAYER, AND THAT IS A FINDING.** Every fixture TLS client in this tree (`0004 0018 0045 0079 0080 0108-0111 0119`) pins `MinVersion: VersionTLS12` and is configured to SUCCEED at the protocol layer. `0110`/`0111` drive failing handshakes, but exclusively CERTIFICATE-verification ones. **`0120` is the FIRST.** Build from `0110`'s client-harness shape while inventing the failure modes.

⚠️ **ONE DIRECTORY = ONE RUNNER BRANCH** ⇒ exactly one `DriveReference`/`DriveSubject` pair and at most one `AssertStats`. **All five arms sequence INSIDE that single pair**, in the `0110` `driveSide` shape.

⚠️ **THIS IS THE SPLIT-GATE WATCH POINT.** If these steps blow past ~10 sub-items in practice, `BOOTSTRAP_PROMPT.md` §6.1's mid-execution trigger applies (§1.3).

**Files:** Modify `test/fixtures/0120-tls-connection-error/driver/driver.go`

- [ ] **Step 1: The arm sequencer.** Follow `0110/driver/driver.go:453-505`: write one line per arm into a `bytes.Buffer`, then structurally check the whole observable and report **ALL** violations in ONE error (`reference_fatalf_makes_assertions_unreachable`).

```go
func (d *connErrDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "reference", addr)
}

func (d *connErrDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, "subject", addr)
}

// driveSide sequences all five arms against ONE side, in a FIXED order, and
// returns a NON-NIL EMPTY slice so CompareBytes has a defined result on both
// sides. ⚠️ It deliberately returns []byte{} rather than what it read: arm (iii)
// DIVERGES ON THE WIRE — the reference replies with a 7-byte fatal TLS alert
// (15 03 01 00 02 02 46) while envoy-go replies with nothing and EOFs. Returning
// read bytes would fail CompareBytes for a reason that is not this row's subject
// (reference_wire_format_both_sides_see_same_bytes).
//
// ⚠️ ALL DISCRIMINATION LIVES IN AssertStats, NOT HERE. normalizeTLSErr collapses
// every failure to one constant (the 0110:610 shape) because BoringSSL and Go
// emit different client-visible alerts, so the drive bytes can only distinguish
// FAILED from SUCCEEDED. Which failure occurred is a STATS question.
func (d *connErrDriver) driveSide(ctx context.Context, side, addr string) ([]byte, error) {
	var probs []string
	record := func(arm string, err error) { log.Printf("0120 %s arm=%s err=%v", side, arm, err) }

	// (v) POSITIVE CONTROL, run FIRST so a broken upstream is caught before any
	// negative arm can be misread. ⚠️ It asserts the ECHO ROUND-TRIP, not just
	// the handshake: MEASURED, a cluster that cannot reach its backend lets the
	// client report HANDSHAKE=OK while the reference's ssl.handshake reads 0,
	// because tcp_proxy tears the downstream connection down before the
	// handshake is booked. A handshake-only pin would go RED with the config
	// looking fine and the client reporting success.
	if echo, err := d.mtlsEcho(ctx, addr, []byte(probePayload)); err != nil {
		probs = append(probs, fmt.Sprintf("valid: handshake/roundtrip FAILED: %v", err))
	} else if !bytes.Equal(echo, []byte(probePayload)) {
		probs = append(probs, fmt.Sprintf("valid: echo mismatch: got %q want %q", echo, probePayload))
	}
	record("valid", nil)

	// (i) bad version — CLIENT-side only. MaxVersion TLS 1.1 against the TLS 1.2
	// floor. ⚠️ NEVER lower the floor in YAML: TLSv1_0/TLSv1_1 in a tls_params
	// block BOOT-REJECTS envoy-go (internal/tls/params.go:62-83).
	record("bad_version", d.expectHandshakeFailure(ctx, addr, tlsMaxVersion(stdtls.VersionTLS11)))
	// (ii) plaintext HTTP to the TLS port.
	record("plaintext", d.expectRawFailure(ctx, addr, []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")))
	// (iii) garbage bytes.
	record("garbage", d.expectRawFailure(ctx, addr, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11}))
	// (iv) ⚠️ THE DISCRIMINATING NEGATIVE CONTROL. Connect and FIN with zero
	// bytes. The server sees a bare io.EOF, classifyHandshakeErr returns
	// outcomeOther, and the predicate's io.EOF term must SUPPRESS the Inc.
	// The reference books NOTHING here (MEASURED: connection_error +0). Without
	// this arm the fixture proves only that the counter CAN move, never that the
	// predicate DISCRIMINATES — and this arm is what exercises the io.EOF term.
	record("clean_fin", d.expectCleanFIN(ctx, addr))

	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %s", side, strings.Join(probs, "; "))
	}
	return []byte{}, nil
}
```

- [ ] **Step 2: ⚠️ EVERY ARM MUST FORCE-SEND AND PRINT WHAT IT SENT.** `reference_go_client_cert_withholding` extends from untrusted to UNPARSEABLE certs: at the phase-94 BRAINSTORM, Go silently sent an EMPTY chain for an unparseable leaf under TLS 1.3, so an arm measured the CLIENT rather than the server. `0111:165-167` records the sibling failure — *"arm 2 DEGRADES INTO arm 3 … while `CompareBytes` stays green."* Use `GetClientCertificate` to force the chain onto the wire and `log.Printf` the certificate count actually sent. ⚠️ **`fixture.TB` has no `Logf`** — use `log.Printf` to record.

- [ ] **Step 3: Arm helpers.** `mtlsEcho` dials with `MinVersion: VersionTLS12`, the client cert forced, `InsecureSkipVerify: false` against the fixture CA. `expectRawFailure` dials plain TCP, writes the payload, and requires the connection to fail or close without an echo. `expectCleanFIN` dials plain TCP and immediately closes — writing nothing.

- [ ] **Step 4: `ProbeAdmin`** — `/ready` only, via `helpers.HTTPGetReadyRaw`, the `0110`/`0118` shape.

- [ ] **Step 5: Verify each arm INDIVIDUALLY against a live reference before wiring the asserter.** Per-arm before/after `/stats/prometheus` snapshots, not one end-of-run scrape. Expected reference deltas — **MEASURED at the PLAN stage**:

| arm | `connection_error` | `handshake` |
|---|---|---|
| (v) valid + client cert | **+0** | **+1** |
| (i) bad version | **+1** | +0 |
| (ii) plaintext | **+1** | +0 |
| (iii) garbage | **+1** | +0 |
| (iv) clean FIN | **+0** | **+0** |

⚠️ **Also re-run the over-firing control** (3× clean-FIN -> no movement; 3× valid -> `handshake` +3 only; 2× bad-version -> `connection_error` +2 only). A positive arm cannot catch an over-firing counter — `reference_positive_arm_cannot_catch_overfiring`.

- [ ] **Step 6: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/driver/driver.go
git commit -m "phase 94 (tls-connection-error-stat) T12: 0120's five drive arms, incl. the discriminating clean-FIN control"
```

---

### Task 13: Fixture `0120` — `AssertStats`, keyed on NAME and asserting BOTH directions

**Files:** Modify `test/fixtures/0120-tls-connection-error/driver/driver.go`

- [ ] **Step 1: Copy `scrapeProm`'s essentials** from `0111/driver/driver.go:828-878`: split on `{` / `LastIndexByte('}')`, key on `line[:open]`, handle the bare and trailing-timestamp variants, **`strconv.ParseFloat` NOT `ParseUint`** (histogram lines carry `nan`/`inf`), skip non-finite and negative, accumulate `out[name] += uint64(v)`.

- [ ] **Step 2: Key on the metric NAME and IGNORE the `envoy_listener_address` LABEL VALUE.** ⚠️ **This is REQUIRED, and now MEASURED rather than argued**: the reference renders `envoy_listener_address="0.0.0.0_10126"` while the subject renders `envoy_listener_address="___10126"` (envoy-go binds `[::]:10126`). Keying on the name resolves all three cross-side scope divergences at once — dots, IPv6 brackets, and `stat_prefix` — because the Prometheus name carries none of them.

- [ ] **Step 3: Assert BOTH directions on every arm.**

```go
func (d *connErrDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	for _, side := range []struct{ name, addr string }{{"reference", refAdminAddr}, {"subject", subjAdminAddr}} {
		got, err := scrapeProm(side.addr)
		if err != nil {
			t.Fatalf("%s: scrape: %v", side.name, err)
		}
		// POSITIVE: three protocol-error arms moved the counter, and ONLY three.
		if v := got["envoy_listener_ssl_connection_error"]; v != wantConnectionError {
			t.Errorf("%s: ssl.connection_error = %d, want %d "+
				"(3 protocol arms Inc; the clean-FIN transport arm MUST NOT)", side.name, v, wantConnectionError)
		}
		// POSITIVE: exactly one successful handshake.
		if v := got["envoy_listener_ssl_handshake"]; v != wantHandshake {
			t.Errorf("%s: ssl.handshake = %d, want %d", side.name, v, wantHandshake)
		}
		// ⚠️ NEGATIVE HALF — assert WHICH DID NOT FIRE. A pin proving
		// connection_error moved says nothing about whether a cert counter also
		// moved. No arm presents a bad or missing client cert, so both cert
		// counters MUST stay 0 on BOTH sides.
		for _, n := range []string{
			"envoy_listener_ssl_fail_verify_error",
			"envoy_listener_ssl_fail_verify_no_cert",
		} {
			if v := got[n]; v != 0 {
				t.Errorf("%s: %s = %d, want 0 — no arm drives a certificate failure", side.name, n, v)
			}
		}
	}
}
```

⚠️ **`wantConnectionError` and `wantHandshake` are ABSOLUTE values, not deltas** — that is the harness convention, and it is why every arm count in Task 12 is fixed and why adding a sixth arm later invalidates them.

- [ ] **Step 3b: ⚠️ Do NOT pin `connection_error` on the subject before T5 has landed.** Pre-IMPL the name is unregistered, so a `0` pin hits the ABSENT branch and goes RED — the vacuity only bites AFTER registration. A zero pin gates **REGISTRATION ONLY and can never gate the INCREMENT** (`reference_counter_cannot_gate_a_value`).

- [ ] **Step 4: NC 8 — prove the discriminating arm discriminates.** Temporarily assert that the clean-FIN arm DOES move the counter (`wantConnectionError + 1`). Expected: **RED on both sides.** Restore.

- [ ] **Step 5: NC the dispatch itself.** Rename `AssertStats` to `AssertStatsX` and re-run the fixture. Expected: the whole stats leg **vanishes silently** and the fixture still passes — the failure mode the compile-time assertion in T11 exists to prevent. Restore, and confirm the compile-time assertion catches it.

- [ ] **Step 6: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/driver/driver.go
git commit -m "phase 94 (tls-connection-error-stat) T13: 0120 AssertStats — name-keyed, both directions, dispatch-guarded"
```

---

### Task 14: Fixture `0120` — `expectations.yaml` and `README.md`

**Files:** Create `test/fixtures/0120-tls-connection-error/expectations.yaml`, `test/fixtures/0120-tls-connection-error/README.md`

- [ ] **Step 1: `expectations.yaml`** in the `0118`/`0110` shape: the arm table, the reference-measured absolute values, and the cross-side divergences that are DELIBERATELY unasserted (the `envoy_listener_address` label value; the arm-(iii) wire-level alert divergence).

- [ ] **Step 2: `README.md`** documenting, at minimum: that `0120` is **the first fixture in this tree to drive deliberately failing TLS handshakes that are not certificate failures**; that it is **the first to ship a `tls_params` block**, booted on both sides at the phase-94 PLAN; the port rationale (`10126`, citing phase 92); the `inline_string` cert-delivery decision and why `filename:` cannot work for the reference container; and the clean-FIN arm's role as the discriminating negative control.

⚠️ **Write it for a stranger with zero prior context** (`BOOTSTRAP_PROMPT.md:140`). Never "as discussed earlier".

- [ ] **Step 3: Run the fixture end-to-end**

```sh
go test ./test/differential/ -run 'TestDifferential/0120-tls-connection-error' -count=1 -v 2>&1 | tee /tmp/t14.log
grep -c '=== RUN' /tmp/t14.log   # MUST be non-zero — a non-matching selector prints ok and exits 0
v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/t14.log || true); echo "FAIL=$v"
```

⚠️ **`-count=1` IS NOT OPTIONAL** — the harness builds envoy-go as a SUBPROCESS, so a production edit is not a compile-time input to this test binary and the cache serves a stale PASS. **Gate (a)'s failure mode is a SILENT PASS.**

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0120-tls-connection-error/expectations.yaml test/fixtures/0120-tls-connection-error/README.md
git commit -m "phase 94 (tls-connection-error-stat) T14: 0120 expectations + README"
```

---

### Task 15: `0110` / `0111` prose — four files that go FALSE

**Files:** Modify `test/fixtures/0110-tls-require-client-cert-false/README.md`, `…/expectations.yaml`, `test/fixtures/0111-tls-cvc-empty-dynamic-fallback/README.md`, `…/expectations.yaml`

- [ ] **Step 1:** Each states in landed prose that `ssl.connection_error` *"increments nothing — a named departure"* — at `0110/README.md:171-173`, `0110/expectations.yaml:224-226`, `0111/README.md:172-174`, `0111/expectations.yaml:229-231`. All four go FALSE. Rewrite to record that phase 94 landed the name under a transport-exclusion predicate, and that these two fixtures still observe it at **0** because none of their arms produces a non-certificate handshake failure.

- [ ] **Step 2: NO driver change is needed.** `0110/driver/driver.go:794-799` and `0111/driver/driver.go:774-778` iterate CLOSED NAMED SUBSETS, so a fifth metric name cannot redden them — verified. **Do not "helpfully" add the fifth name to those subsets**: it would couple two unrelated fixtures to this row and break their absolute arm arithmetic.

- [ ] **Step 3: ⚠️ `0108` IS OUT OF SCOPE, DELIBERATELY.** `0108/README.md:136-140` and `0108/expectations.yaml:104-106` assert *"envoy-go emits NO `ssl.*` stats whatsoever"* — **already false since phase 74**. `0110`/`0111` retired that wording; `0108` was never updated. It is PRE-EXISTING DRIFT, recorded in §9 as a deferred item, and **not** this row's delta.

- [ ] **Step 4: Re-run both fixtures** (`-count=1 -v`, non-zero `=== RUN`), then **commit**.

```bash
git add test/fixtures/0110-tls-require-client-cert-false/ test/fixtures/0111-tls-cvc-empty-dynamic-fallback/
git commit -m "phase 94 (tls-connection-error-stat) T15: retire the 'increments nothing' prose in 0110/0111"
```

---

### Task 16: `ADR-0316` — complete the block and DISARM the house guard

**Files:** Modify `docs/envoy-go/DECISIONS.md`

⚠️ **APPEND-ONLY (ADR-0288 §Decision 4).** §Decision and §Consequences are **APPENDED IN PLACE** after the retained italic footer. **No renumber. No `---` separator.** ⇒ `^---$` **STAYS 216**; `^## ADR-` **STAYS 315**; bare `^## ` **STAYS 323**; tail **STAYS ADR-0316**; next-free **STAYS ADR-0317**.

- [ ] **Step 1: Append §Decision** — the seam (predicate at the Inc site, taxonomy stays FOUR while counters go to FIVE), the closed-complement predicate, the `helpText` placement choice, and the two decisions this PLAN added: **D-TLSCE-FALSIFIABILITY** (§0.1) and **D-TLSCE-CERTDELIVERY** (§0.4).

- [ ] **Step 2: Append §Consequences** — including, explicitly:
  - `outcomeOther` is now the ONLY outcome mapping to a counter CONDITIONALLY; the 1:1 outcome-to-counter reading is wrong in both directions.
  - `net.ErrClosed` is a DEFENSIVE, UNEXERCISED predicate member.
  - The ADR-0296 figures (`~9-11` tasks, ONE `io.EOF` predicate) are **SUPERSEDED**, and `phases/77/BRAINSTORM.md:216`'s `~12-15` / three-predicate / deny-list-OPEN position is recorded as measurement-correct but conclusion-refuted. **Neither file is edited** — append-only.
  - The phantom `BEHAVIOR_CONTRACT B5` correction (§0.12), including that the propagated copy at `DECISIONS.md:17316` is deliberately NOT edited.

- [ ] **Step 3: DISARM the house guard.** Change the STATUS blockquote from `PROPOSED` to `ACCEPTED`, keeping the house form.

⚠️ **NAME NO COUNT FOR EITHER `PROPOSED` FORM IN THAT LINE** — the line is itself a hit of both forms, so any figure it named would be falsified by its own landing. **Verify BY LINE AND BY ADR, never by the count alone:**

```sh
grep -n '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md    # after: want ZERO hits
grep -n '^\*\*Status:\*\* PROPOSED' docs/envoy-go/DECISIONS.md   # the ADR-0231 DECOY at :14866 — UNCHANGED
```

⚠️ **The two forms are DIFFERENT REGEXES and must never be conflated; never gate on the unanchored form nor on the middle-ground `^\*\*Status:\*\*.*PROPOSED`.** Resolve any hit to its enclosing ADR by a BACKWARD heading search, per `reference_adr_doctrine_misattribution`:

```sh
awk 'NR<=<HITLINE> && /^## ADR-/ {h=$0; n=NR} END {print n, h}' docs/envoy-go/DECISIONS.md
```

- [ ] **Step 4: Re-derive the structural counts and commit**

```sh
for p in '^---$' '^## ADR-' '^## '; do printf '%s = %s\n' "$p" "$(grep -c "$p" docs/envoy-go/DECISIONS.md || true)"; done
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1     # want ## ADR-0316
grep -c '^## ADR-0317' docs/envoy-go/DECISIONS.md || true          # want 0
```

⚠️ **DERIVE next-free FROM THE TAIL, NEVER FROM THE HEADING COUNT.** The id space is SPARSE (exactly one gap, `ADR-0209`), so headings+1 reads **316** — an id already TAKEN.

---

### Task 17: The contract, the roadmap, and row 94 -> `done`

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/ROADMAP.md`

⚠️ **`:1971` IS ONE ENORMOUS PARAGRAPH LINE. EVERY EDIT IS WITHIN-LINE.** A `sed`-style line replacement destroys neighbouring propositions. **Edit by literal text**, and re-verify afterwards that the line still carries its other propositions.

- [ ] **Step 1: The pinned map** (`SPEC.md` §11, unchanged by this stage):

| site | edit |
|---|---|
| `:1971` | **DELETE ONLY** the clause *"and is still blocked on enumerating its membership"*. ⚠️ **KEEP** the *"The full membership … is UNENUMERATED"* sentence beside it — different propositions; only the second survives |
| `:1971` | Retitle the departure **CLOSED**. Record the §2.1 reference rule and the FOUR-outcomes / FIVE-counters asymmetry |
| `:1967` | *"three listener-scope counters … EXTENDED by phase 75 … to a FOURTH"* -> the **FIFTH**, phase-94 attribution |
| `:1973` | QUIC permanent-zero parity **INHERITS BY CONSTRUCTION, NOT BY MEASUREMENT** — `serveConnection` is the sole `Inc` site for the family and `Manager.Start`'s accept loop `continue`s on `rt.kind == kindQUIC`. ⚠️ **State it as inheritance, exactly as phase 75 did. Do NOT claim a QUIC probe that was not run** |
| `:1975` | The cross-side scope divergences apply to the fifth name as pre-existingly as to the first four; none is re-opened |

- [ ] **Step 2: Verify the surgical edit did not damage its neighbours**

```sh
grep -c 'UNENUMERATED' docs/envoy-go/BEHAVIOR_CONTRACT.md                       # must still be >= 1
grep -c 'still blocked on enumerating its membership' docs/envoy-go/BEHAVIOR_CONTRACT.md || true   # want 0
```

- [ ] **Step 3: `ROADMAP.md:155`** — mark row 93's *"~4-6 production lines"* and *"its reference side is an unprobed doc claim"* **SUPERSEDED**, citing ADR-0316. **Both halves are FALSE**: the reference side was probed at the phase-94 BRAINSTORM (twelve arms) and re-probed at this PLAN.

⚠️ **COUNT THE ROW'S FIELDS UNDER BOTH FORMS, BEFORE AND AFTER.** Row 93 must stay **NF=8**:

```sh
awk -F'|' '/^\| *93 /{print "naive NF=" NF}' docs/envoy-go/ROADMAP.md
awk '/^\| *93 /{s=$0; gsub(/\\\|/,"@",s); print "escape-aware NF=" split(s,a,"|")}' docs/envoy-go/ROADMAP.md
```

⚠️ **An unescaped `|` in a ROADMAP cell passes check (1) but breaks the field count.** Escape every literal pipe as `\|`.

- [ ] **Step 4: Row 94 -> `done`.** A **FLIP, not an ADD** ⇒ `want` **STAYS 126** and `ROADMAP.md` stays **244** lines.

- [ ] **Step 5: Re-run the sentinel.** ⚠️ **Check (1) collapses to SILENT and the margin returns to ONE.** Re-measure NC-A and NC-B — **their shapes CHANGE with this flip** (both drop from TWO lines to ONE). ⚠️ **NEVER inherit a NC shape across a row flip.** Confirm check (2) still reads **SIX** and that all six window md5s are unchanged — **do not tidy a deferred-candidate line.**

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/DECISIONS.md
git commit -m "phase 94 (tls-connection-error-stat) T17: close the named departure, supersede row 155's claims, row 94 -> done"
```

---

### Task 18: Full verification sweep — the SIX-GATE POSTURE

⚠️ **NAME DEPARTURES; DO NOT CLAIM COMPLIANCE.**

- [ ] **(a) Differential suite — full**

```sh
go test ./test/differential/ -count=1 -v 2>&1 | tee /tmp/g-a.log; rc=${PIPESTATUS[0]}
echo "RC=$rc RUN=$(grep -c '=== RUN' /tmp/g-a.log)"
v=$(grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/g-a.log || true); echo "FAIL=$v"
```

Expected **122/122 PASS** (121 today + `0120`). ⚠️ **ASSERT THE FIXTURE SET BY NAME, BOTH DIRECTIONS** — a count alone cannot see a rename. ⚠️ **Takes ~400s.** ⚠️ **`-race` here is VACUOUS** — the subject is an unraced subprocess.

- [ ] **(b) Non-Docker sweep, gated on `PIPESTATUS[0]` AND a set reconciliation**

```sh
go test -count=1 $(go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$') 2>&1 | tee /tmp/g-b.log
rc=${PIPESTATUS[0]}; echo "RC=$rc"
```

⚠️ **Reconcile the package SET, not just the count** — `reference_harness_exit_code_is_not_command_exit_code`. ⚠️ **`INNER_EXIT` DOES NOT EXIST in this repo**; do not invent it.

- [ ] **(c) h2spec conformance** — expect `95 tests, 94 passed, 1 skipped, 0 failed`.
- [ ] **(d) Fuzzers** — expect **56 targets / 48 files**, unchanged (**+0**: this row adds no config field, so no parse arm). Reconcile against `^func Fuzz` before quoting.
- [ ] **(e) The ANCHORED panic gate**

```sh
v=$(grep -cE '^panic:|DATA RACE|SIGSEGV' /tmp/g-a.log /tmp/g-b.log || true); echo "PANIC=$v"   # want 0
```

- [ ] **(f) `REVIEW.md`** — **a STANDING DEPARTURE, NAMED NOT CLAIMED.** This project does not produce one; say so rather than implying step 5 ran.

- [ ] **Per-package lint, gated on OUTPUT**

```sh
gofmt -l internal/listener/ internal/stats/ test/fixtures/0120-tls-connection-error/   # want EMPTY
golangci-lint run ./internal/listener/... ./internal/stats/...                          # want EMPTY
```

- [ ] **Known-flake register — a green run clears NOTHING.** Live: two SDS dial-budget flakes plus `TestSDSEndToEnd_FetchFailure_BootFailsClosed` · the driver-owned receiver port race · `internal/httpclient` zero-value · the two 84.2-era flakes · the REFERENCE h2spec section-8 flip · `TestOutlierDetector_ConcurrentEjectExactlyOnce` · `0061-lb-ring-hash`'s σ-margin · **`TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure`** (wasm, 1/400-trial, surfaced at the phase-94 SPEC, **did NOT recur at this PLAN**). ⚠️ **`TestServerConn_TinyWindowDelivery` IS NOT A FLAKE** — phase 91 fixed a live production deadlock; a recurrence is a REGRESSION of row 91.

- [ ] **Commit `PROGRESS.md`** with every command output quoted verbatim.

---

## 6. The NC roster, CORRECTED

`SPEC.md` §8's roster is superseded where marked. Every NC NEUTRALISES and leaves the package compiling — **a NC that is a build break proves nothing.**

| # | NC | MUST produce | status |
|---|---|---|---|
| **1** | Remove the predicate (unconditional `Inc` inside `case outcomeOther:`) | **RED** — 3 exclusion arms read 1 want 0; **stacked reads 4 want 1** | ⚠️ **REWRITTEN.** The SPEC ran this against `sslLeafRoster`, where it is INVISIBLE at both roster sizes (§0.1) |
| **1b** | Remove the `Inc` (`_ = rt.sslConnectionError`) | **RED** — 3 inclusion arms read 0 want 1; **stacked reads 0 want 1** | **NEW** — the second, isolating direction |
| **1c** | Hoist the `Inc` above the switch | **RED** — the two cert tests at `:4694`/`:4712` | **NEW** — this is what the SPEC's cell D actually measured |
| **2** | Synthetic string error for the real `*net.OpError` | the arm **FLIPS** | REPRODUCED; landed permanently as `TestIsTransportHandshakeErr_ReadsIdentityNotText` |
| **3** | Delete the registration, keep the `Inc` | **SIGSEGV**, with `GateMatchesInc` still PASSING unless extended | REPRODUCED, both halves + the fix |
| **4** | Entry present, roster absent | **RED** `extra:`, 1 test | REPRODUCED |
| **5** | Roster present, entry absent | **RED** `missing:`, ⚠️ **TWO tests** | REPRODUCED, RICHER than the SPEC stated |
| **6** | Both present, slice at four | ⚠️ **GREEN** — the documented silent gap | REPRODUCED |
| **7** | Fixture registered, blank import omitted | **`t.Skipf` — SILENTLY GREEN** | to run at T11 |
| **8** | Clean-FIN arm asserted as MOVING the counter | **RED** both sides | to run at T13 |
| **9** | `AssertStats` renamed | the stats leg **vanishes GREEN** | **NEW** — to run at T13 |

⚠️ **NC THE GATE COMMANDS THEMSELVES.** Every `-run` selector must be shown to select something — reproduced live at this stage, a non-matching selector prints `ok … [no tests to run]` and **exits 0**.

---

## 7. Cost — MEASURED and ESTIMATED, labelled separately

**MEASURED at this tip:** production files **2** · `manager.go` prose blocks **6** (+1 guarded non-site) · mandatory test-edit sites **8** across **4** files (⚠️ **not 5** — §0.6) · fixture-prose files **4** · rename sites **4** (roster complete; zero hits in `ci.yml`, and this repo has **no Makefile and no `scripts/`**, so two thirds of that claim is vacuous) · `helpText` entries **30** (AST walk and textual count AGREE; `len(helpText)` guarded by **nothing**) · byte-gate set difference **∅** (⚠️ but two gates DO cover `name.go` — §0.7) · red set **3 tests**, `RUN=8704`, anchored `FAIL=8` (⚠️ **not 11**).

**ESTIMATED — labelled, not measured:** production **+45/-20**; the counter table **+300**; fixture `0120` **≈+1150**, anchored on `0118`'s creating commit `4d7f63c2` (**+1144**); PKI **+40**; total **≈+1608/-25**, of which **≈+995** is `.go`. ⚠️ **`reference_measured_prototype_is_a_lower_bound` HAS FIRED NINE CONSECUTIVE ROWS, ALWAYS BY UNDER-ENUMERATING FILES** — this stage already extended the roster twice (§0.7's two `name.go` gates, §0.15's seven prose lines). **Treat every figure here as a LOWER BOUND.**

**Axis deltas:** fixtures **121 -> 122** · stat surface **+1 name** *(delta only — three different absolutes are live in this tree at one tip; per `reference_a_drift_correction_is_itself_a_claim`, on a contested count: NO NUMBER)* · BackendKinds **+0** (tail stays 38) · fuzzers **+0** · `go.mod` **+0** · packages **+1** (`0120/driver`) · phase dirs **135** (unchanged) · ROADMAP rows **126** (unchanged — row 94 FLIPS, it does not ADD).

---

## 8. Deferred — newly surfaced by THIS PLAN, none chartered

- **`0108`'s two *"envoy-go emits NO `ssl.*` stats whatsoever"* confessions** — false since phase 74, outside this row's delta.
- **`0118/driver/driver.go:31`'s falsified *"TLS/SDS band"* characterization** (`0112`/`0113` carry ZERO `DownstreamTlsContext`).
- **No `len(helpText)` guard exists**, and `name.go` carries two ungated prose counts.
- **`manager.go:441` cites `handshake_server.go:964, go1.26.5`**; at the live toolchain (go1.26.7) that text is at `:970`. A stdlib cite that drifts with the toolchain; the tripwire itself is sound because the test builds the error from a LIVE handshake.
- **`net.ErrClosed` is an unexercised predicate member**, and the synthetic `errors.New("connection reset by peer")` has no measured production producer — recorded as residuals, not claimed impossible.
- ⚠️ **NEW: a SEVENTH handshake-failure arm — ALPN negotiation failure** — exists and reaches `outcomeOther` today (§0.2). It is absent from `SPEC.md` §2.2's six-row table and is NOT added to the unit table here, because it requires an ALPN-configured listener; against `startOneWayTLSListener` the handshake SUCCEEDS. A cheap follow-on.
- ⚠️ **NEW: `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` will silently begin incrementing `ssl.connection_error`** once T5 lands, and asserts nothing about it. Consider a counter assertion there.
- **Carried:** GET `/runtime` (the strongest runner-up) · `POST /runtime_modify` · lifting the six `runtime_key` rejects · 1xx interim responses · the four dynamic `ssl` families · the other TEN fixed `ssl.*` names · `0061-lb-ring-hash`'s σ-margin second occurrence.

**None is added to any ROADMAP `candidates:` sentence at this stage** — stated as a commitment the IMPL must verify by grep, not as an accomplished fact. ⚠️ **This row NARROWS window `:226` (item 2 of three) at the IMPL. Narrowing is NOT sentinel progress**: check (2) keys on the PHRASE, so removing a clause leaves phrase, line and count untouched. Charter it as narrowing.

---

## 9. Cite hygiene — what the IMPL must NOT inherit

**Wrong at this tip, corrected here:** `manager_test.go` ssl want entries are **2138-2141** (literal 2137-2142), not `:2136-2141` · `quic_test.go` **282-285** (literal 281-286), not `:280-285` · `next-prompt.txt:116` -> **`:104`** · the malformed-row identifiers **57**/**69** are **ROW IDs**, at **file lines 119/131** · the six window md5s require the **trailing newline** (`sed -n 'Np' f | md5sum`) · `tls_params` reads **56 lines / 13 files**, 4 outside `docs/` — **quote no total; state the YAML-scope claim** · the phantom-`B5` context count is **10 lines**, not five.

**Standing rules this row exercised:**
- **A "drift-proof" anchor is itself a claim, and its uniqueness can be SCOPE-DEPENDENT.** Both edit anchors are unique only under `-- '*.go'`; unscoped each reads **7 lines / 6 files** — and the SPEC's own landing moved that number (`reference_self_incrementing_positive_control`). **RE-DERIVE, never quote.**
- **A measurement in a STAGE ARTIFACT is not landed.** Phase 77's reference measurement stayed in its BRAINSTORM and was lost for seventeen rows. `ADR-0316 §Context ¶2` is the repair, and it is **this row's most durable deliverable**.
- **Both agents can be right.** The prose-roster disagreement resolved by READING THE DISPUTED SITE, not by picking a winner (`reference_contradicting_agents_find_the_variable`).
- **A green run is not evidence a site is exercised** — use a `panic()` reachability control. This row needed it acutely, and it overturned a SPEC conclusion (§0.2).
- **A NC that leaves your control green is not evidence the control works** — you need a SECOND, ISOLATING NC (§0.1, §0.3, T6 Step 3).
- **Assert WHICH error fired, and also which did NOT.** Keep table rows single-cause.
- **A probe client can withhold the very thing the arm exists to test** — force-send and print what was sent.

---

## 10. `SPEC.md` §16 coverage — every owed item, and the ONE deliberate deviation

| §16 item | discharged where | note |
|---|---|---|
| 1. Derive its own task count | §5 | **18**, derived here. No figure inherited — four inconsistent ones are live in two units |
| 2. Boot both sides on `tls_params` BEFORE any arm | **T10 Step 3**, and MEASURED at this stage | ⚠️ **RISK CLEARED**: both sides boot, and `/config_dump` proves the reference RETAINS the block. Re-run at the IMPL's tip |
| 3. Counter table on production-representative values; identity-vs-text NC; `sslLeafRoster` 2×2; **run NC 1 both ways** | **T4, T5, T6**; §0.1, §0.3 | ⚠️ **Run both ways, and the SPEC's cell D was REFUTED.** NC 1 is REWRITTEN (§6) |
| 4. All `0120` arms inside the single `Drive` pair, incl. the clean-FIN control | **T12** | The clean-FIN arm is also what EXERCISES the predicate's `io.EOF` term (§3.1) |
| 5. Assert the fixture set BY NAME, both directions | **T11 Step 6** | With the 97-`driver/` / 24-`inputs/` two-layout trap handled |
| 6. `helpText` + `helpTextRoster` + slice in ONE commit; budget the `gofmt` realignment | **T3** | ⚠️ The realignment premise is **REFUTED** (§0.9); the real one-line churn is in `manager.go` (T1 Step 4) |
| 7. Land the fifth `GateMatchesInc` assertion **in the same commit as the `Inc`** | **T2** (guard) and **T5** (`Inc`) | ⚠️ **DELIBERATE DEVIATION — see below** |
| 8. Set-difference the §6 roster at the PLAN's own tip; re-derive every §9 and §13 count | **§0 (sixteen refutations), §2, §7** | The roster was EXTENDED twice, not merely confirmed |

### ⚠️ The deviation on item 7, stated rather than made silently

`SPEC.md` §16 item 7 requires the fifth `GateMatchesInc` assertion to land **in the same commit as the `Inc`**, because the omission's failure mode is a process crash. **This plan lands the guard EARLIER — at T2, three tasks before the `Inc` at T5.**

That is **strictly safer, not a relaxation.** The SPEC's requirement exists to guarantee that an `Inc` never exists in the tree without its nil guard. Landing the guard first satisfies that invariant at every commit boundary, whereas landing them together satisfies it only at the boundary. There is no commit in this sequence at which `rt.sslConnectionError.Inc()` exists unguarded.

**What the IMPL must NOT do** is reorder T5 before T2. If tasks are executed out of order, item 7's original coupling becomes mandatory again and the two must be merged into one commit.

---

## 11. Self-review — run against the spec with fresh eyes, defects fixed inline

**Spec coverage:** all eight §16 items map to tasks (§10). All six `D-TLSCE` docket entries are carried in §3. All eight §6.2 sites map to T1/T2/T3/T6/T8; all six §6.3 prose sites to T7; all four §6.5 files to T15; the three §7.1 registration gates to T11; the §7.5 asserter to T13; the §11 contract map to T17; §10's ADR to T16. **No spec requirement is without a task.**

**Placeholder scan:** no `TBD`, no "implement later", no "add appropriate error handling", no "similar to Task N", no "write tests for the above". Every code step carries real code. The two places that legitimately defer detail — T12 Step 3's arm helpers and T14's `expectations.yaml` — name the exact precedent file and line to copy the shape from.

**Type consistency — THREE DEFECTS FOUND AND FIXED:**
1. ⚠️ **`gaugeValue` does not exist in `internal/listener`.** `counterValue` (`quic_test.go:66`) is counter-only. T5 now ADDS `gaugeValue` explicitly, in `counterValue`'s exact `reg.Walk` + type-assertion shape.
2. ⚠️ **`Gauge` has no `Value()` method.** Its accessor is **`Load() int64`** (`internal/stats/gauge.go:56`). Corrected.
3. ⚠️ **`startOneWayTLSListener` is `(t, pki handshakeTestPKI) (*stats.Registry, string)`** (`manager_test.go:4730`) — it takes a PKI and returns only two values; teardown is internal via `t.Cleanup`. The draft's `reg, addr, stop := startOneWayTLSListener(t)` would not have compiled. Corrected to the `mkTestPKI(t)` shape used at `:4675`. **`counterValue` returns `int64`, not `uint64`** — the table's `want` field is typed accordingly.

⚠️ **All three would have been compile errors on the IMPL's first run.** They are recorded rather than silently fixed, because `reference_measured_prototype_is_a_lower_bound` has fired NINE consecutive rows and this is the tenth instance of the same species: **an inherited roster of "existing helpers" that was never checked against the tree.**

**Verified to EXIST at this tip** (`git grep -n 'func <sym>' -- 'internal/listener/*.go'`): `startOneWayTLSListener` `:4730` · `startMutualTLSListener` `:4542` · `counterValue` `quic_test.go:66` · `connPair` `:4270` · `liveHandshakeErr` `:4326` · `mkTestPKI` `:4238` · `assertSSLCrossProduct` `:4610` · `listenerSSLNames` `:2081` · `normalizeAddr` `manager.go:352`. In the fixture layer: `mustRender`, `mustReadFixtureFile`, `probePayload`, `normalizeTLSErr`, `scrapeProm` all exist in `0110`/`0118`.
