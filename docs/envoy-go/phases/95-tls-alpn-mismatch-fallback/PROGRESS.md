# Phase 95 — `tls-alpn-mismatch-fallback` — PROGRESS (the IMPL)

Lifecycle-state **3 -> DONE**. Row 95 flips `in-progress` -> `done`.

**What landed.** On the TCP downstream path, a client whose ALPN offer overlaps NONE of the chain's
`alpn_protocols` now **COMPLETES the handshake with no protocol selected and is SERVED** — matching
the pinned reference Envoy — instead of aborting with `no_application_protocol`. The implementation is
a TCP-only `GetConfigForClient` installed at the **ENTRY** of `internal/tls.NewDownstreamConfig`,
returning a **PER-HANDSHAKE `Clone()`** with `NextProtos = nil` when the offer is non-empty and
overlaps nothing. `NewQUICDownstreamConfig` is **byte-untouched**: QUIC's RFC 9001 §8.1 rejection is
PARITY, not a departure.

⚠️ **Every figure below is from THIS session's own runs**, re-derived at the publishing tip. Nothing is
inherited from the BRAINSTORM, SPEC, PLAN or from the controller's brief without re-derivation — and
§2.19 records a case where re-derivation contradicted the brief.

⚠️ **DEPARTURES ARE NAMED, NOT PAPERED OVER.** Three stand: **no `REVIEW.md`** (§4f), the **non-Docker
sweep that was RED on run 1 and green on run 2** (§4b — *a green rerun clears nothing*), and **two
files touched beyond `PLAN.md` §4's declared map** (§6).

---

## 0. Sentinel — RUN MECHANICALLY, BEFORE the row-95 flip

ACTUAL output, recorded not predicted:

```
(1) NOT DONE: row 95                     <- ONE line, correct and expected while row 95 is open
(2) 205:remaining deferred (not-yet-chartered) candidates:
    211:remaining deferred (not-yet-chartered) candidates:
    217:remaining deferred (not-yet-chartered) candidates:
    227:remaining deferred (not-yet-chartered) candidates:
    233:remaining deferred (not-yet-chartered) candidates:
    241:deferred candidates:              <- SIX
(3) (silent)
```

Per-line md5 of the six windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — the method
is stated because the digest is method-sensitive:

```
205 10d7807bf02d   211 4a92f7e62fc6   217 2a7eb298b9fd
227 242e53c6f7a3   233 b2680e6f4fbf   241 6caa1c3ce0e7
```

**All four NCs, plus the check-(2) positive control, BEFORE the flip:**

| control | result BEFORE |
|---|---|
| NC-A (row 62 doctored to `in-progress`) | **TWO** lines: `NOT DONE: row 62`, `NOT DONE: row 95` — landed, verified by inspecting `NC LANDED? [ in-progress ]` first |
| NC-B (`want=126` on the real file) | **TWO** lines: `NOT DONE: row 95`, `GATE FAIL: examined 127 data rows, expected 126` |
| NC-C (check-3 NC, `gRPC-family row` neutralised) | **FIRED** — `NEVER OPENED: gRPC`, residual **0** |
| NC-D (`-family row`, `--` passed before the pattern) | occurrences **96**, lines **68** |
| check-(2) positive control | residual **0**, neutralisation asserted at **6** substitutions |

Row 95's field count **BEFORE** installing: naive **NF=8**, escape-aware **NF=8**. Escape-aware
malformed rows are IDs **57** (NF=9) and **69** (NF=10) **only**; naive malformed **17**, escape-aware
**2**. ⚠️ The escape-aware form is run as `sed 's/\|//g' F | awk -F'|' …` with **NO file argument to
awk** — passing the file makes awk ignore stdin and print the NAIVE figure under the escape-aware label.

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately NOT created** — verified
absent at the git root and in this stage's worktree.

---

## 1. Execution shape

Subagent-driven per `feedback_execution_style`, **three streams on private worktrees**, each with its
own scratch directory, controller squashing at the close.

⚠️ **THE TASK ORDER WAS DELIBERATELY CHANGED, AND THE CHANGE IS DECLARED.** `PLAN.md` §5 numbers the
close as Task 17 and the six gates as Task 18. **The gates ran FIRST.** The row-93 and row-94
precedents both put the six-gate figures **inside the `ROADMAP.md` row summary**, so the PLAN's literal
order would have forced this close to **FORECAST** gate results it could instead measure. Everything in
§4 below is actual output.

**And one task was added that the PLAN does not contain: Task 9b.** Task 9 rewrote the false
present-tense production comments; **9b** repaired the line citations that *this row's own doc-comment
growth* had just invalidated, re-anchoring them on SYMBOLS so they stop rotting (§2.9).

| task | subject |
|---|---|
| T1 | `SPEC.md` §5.4 guard pins, RUN RED first — assertion 2 fires with the measured message |
| T2 | `SPEC.md` §4 landed **VERBATIM at the entry anchor** — symbol asserted, arm DRIVEN, panic control fired |
| T3 | the callback driven DIRECTLY, plus the base re-read that makes cell 10 visible |
| T4 | the wrong-side pin FLIPPED — cell 1's red goes green, failing-test SET delta measured |
| T5 | the two already-false present-tense claims rewritten; the guarded non-sites re-asserted |
| T6 | rows 7 and 8 land with `wantLeaves {handshake:1, no_certificate:1}` |
| T7 | the mTLS-preservation arms, discriminating on the counter TRIPLE |
| T8 | **NC 7 re-run — the authentication bypass REPRODUCED and it SERVES traffic** |
| T9 / T9b | false production comments rewritten; invalidated citations repaired and re-anchored |
| T10 | both `0120` YAMLs gain `alpn_protocols` as the first key of `common_tls_context` |
| T11 | fixture `0120` arm (vi) — non-overlapping ALPN offer, RECORDED and never drive-fatal |
| T12 | `0120` pins re-measured for the new arm shape — `connection_error` STAYS 3, `handshake` 1 -> 2 |
| T13 | **`SPEC.md` §12 cell 13 REPLACED** — arm (vii) pins `NegotiatedProtocol` per side |
| T14 | `0120` docs re-derived FROM THE DRIVER — seven arms, pins 3/3 |
| T15 | **ADR-0317 completed in place; house `PROPOSED` guard DISARMED** |
| T16 | `BEHAVIOR_CONTRACT.md:1944` redeemed **WITHIN-LINE** |
| T17 | this close — `ROADMAP.md` row flip, `STATE.md` roll + eviction, this file |
| T18 | the six gates — **RAN BEFORE T17, deliberately** |

---

## 2. ⚠️ WHAT THIS IMPL REFUTED BY EXECUTION — EIGHTEEN CLAIMS, PLUS A NINETEENTH FOUND AT THE CLOSE

Escalation across the phase: BRAINSTORM **9** · SPEC **11** · PLAN **17** · **IMPL EIGHTEEN** (plus
§2.19, found while writing this file).

### ⚠️ 2.1 — PLAN TASK 1 STEP 2's THIRD PIN, AS LITERALLY WORDED, IS SATISFIABLE BY DOING NOTHING

`ClientAuth` / `ClientCAs` are **phase-67 outputs**. They are green at the tip, green after the fix,
**and green under the authentication-bypass bug**, which corrupts the ALT config and never the base.
The pin therefore cannot discriminate any state this task exists to distinguish. Only an added
`GetConfigForClient != nil` assertion makes it fire.

⇒ **The FOURTH vacuous-pin cell of this phase, and the FIRST found at the IMPL** — the SPEC contributed
three. `reference_pin_can_fail_against_correct_code` and its mirror, the vacuous guard, are now the
phase's dominant failure mode.

### ⚠️ 2.2 — A DELETED `+0/+0` NEGATIVE-CONTROL ARM IS INVISIBLE TO EVERY GATE

A patch silently dropped fixture `0120`'s `record("clean_fin", …)` — **the discriminating control that
exercises the predicate's `io.EOF` term**. `go vet` stayed quiet. `gofmt` stayed quiet. The fixture's
**own cross-side pins** stayed quiet. Every one of them, because that arm **contributes ZERO by
design**: it moves no counter, so no total can miss it.

Only a **per-arm probe** caught it. The commits were rebuilt with the line restored and every
measurement re-derived from the rebuilt tree.

⇒ **A control whose whole point is contributing zero cannot be guarded by a total.** This generalises
`topic_gate_hygiene`'s counter-vs-value rule to the negative-control roster itself.

### 2.3 — PLAN §0.5's "TWO SITES ARE NOT HISTORY" IS FOUR

A **case-insensitive** sweep found `internal/tls/doc.go:5` and `internal/tls/config.go:25` making the
same already-false present-tense claim — the latter **19 lines above the very function that now
installs the callback**.

### 2.4 — SPEC §5.3's "ONE LISTENER, TWO ARMS" IS UNSTATEABLE ALONGSIDE THE REQUIRED COUNTER TRIPLE

The control arm drives `ssl.handshake` to 1, so `handshake == 0` on the withheld-certificate arm is
**impossible on a shared listener**. Per-arm listeners keep **exact equality** instead of degrading the
assertion to a delta.

### 2.5 — THE WEAK FORM OF ARM (b) WOULD BE GREEN AT THE UN-FIXED TIP — OBSERVED, NOT REASONED

Both variants measured `served=false` at the un-fixed tip, so a "not served" assertion **never fires**.
Only the which-error triple reddens it. This was measured, not argued.

### 2.6 — THE TLS1.3 REJECTED ARM READS `hsCompleted=true`

Direct evidence that a **handshake-completion probe answers the wrong question** — which is precisely
why the bypass in §2.7 needed a write-and-read probe rather than a completion flag.

### ⚠️ 2.7 — THE AUTHENTICATION BYPASS IS CONFIRMED BY EXECUTION AND IS WORSE THAN THE SPEC STATED

`SPEC.md` §0.1 predicted that a **build-time** clone would sit BEFORE `ClientCAs` / `ClientAuth` are
assigned and would therefore serve `NoClientCert`. NC 7 reproduced it, and the client was **not merely
admitted — it was SERVED**: a client presenting **no client certificate** through a
`require_client_certificate: true` chain completed the handshake **and got a full application round
trip**, at **both** TLS versions:

```
hsCompleted=true  served=true  negotiated=""  certSent=false
no_certificate=1  handshake=1  connection_error=0
```

Instrumented at both instants, the install anchor carries `NoClientCert`/`nil` while the handshake-time
clone carries `RequireAndVerifyClientCert`/non-nil. ⇒ **The ENTRY anchor is load-bearing, not
stylistic.**

### 2.8 — SPEC §5.2's "rest 0" IS REFUTED IN PRACTICE

The empty-map negative control exhibits `no_certificate = 1`, because `startOneWayTLSListener` sends no
`CertificateRequest` and every completing handshake there books it. A `wantLeaves` written from the
SPEC's figure goes **RED against CORRECT code**. It landed as `{handshake:1, no_certificate:1}`.

Its sibling, **§5.3's "assert the client-certificate signature" instruction, produces a FALSE RED** the
same way: at TLS1.2 the client reads `remote error: tls: handshake failure` with **no `certificate`
substring**. The version-invariant discriminator is the server-side counter triple.

### 2.9 — SEVEN LINE CITATIONS WERE INVALIDATED BY THIS ROW'S OWN DOC-COMMENT GROWTH

Six were enumerated; a **seventh was found beyond the controller's roster**. All were repaired and
**re-anchored on SYMBOLS**, so they stop rotting on the next comment edit. This is what Task 9b exists
for, and it is why the row touches `internal/xds/` at all (§6).

### 2.10 — THE LINE SHIFT IS BANDED, NOT ONE CONSTANT

The diff's offset reads **+0 / +8 / +9 / +14** across bands. This refutes the controller's own
generalisation of a reading taken from **one** band. **A single offset for a multi-hunk diff is a
claim, not a measurement.**

### 2.11 — PLAN §1.3's COST ESTIMATE UNDERSHOT `.go` LINES BY ~28%

Estimated ≈ **+812** `.go` lines; measured **+1038**. See §6.

### 2.12 — PLAN §7's `config.go` 629 PREDICTION IS STALE

It landed **642** — the PLAN figure predates Task 9's comment growth.

### 2.13 — NC 7 FOUND A DETECTOR THE PLAN DID NOT ENUMERATE

Arm (a) reddens on `certSent=false` **and** on `no_certificate=1`. The PLAN named one.

### 2.14 — A CASE-SENSITIVE SWEEP ACTUALLY MISSED ONE

`The five` reads **2** case-sensitively and **3** case-insensitively in one README. The
case-insensitivity rule is confirmed here **by a real miss**, not by argument.

### 2.15 — SPEC §5.2's "SIX EXISTING ROWS BYTE-FOR-BYTE UNTOUCHED" IS UNACHIEVABLE IN GO

Once the row struct gains a field, positional composite literals cannot compile unchanged; the rows
became **keyed** composite literals. **The NC, not the wording, is the guarantee.**

### 2.16 — A NEW UNREGISTERED FLAKE WAS FOUND

`TestFramer_ReaderGoroutineDoesNotLeak` (`internal/filter/hcm/h2`). Registered here for the first time;
full analysis in §4b.

### 2.17 — THE CONTROLLER'S OWN BRIEF MISCOUNTED THE GUARDED NON-SITES AS TEN; IT IS **NINE**

Caught by a subagent. `internal/listener/manager.go:138` is **appended to** by this row, so it is not a
byte-untouched guard and cannot be a member of the guarded set. ⚠️ Task 5's commit subject, written
before the recount, still says TEN; **the measured figure is NINE** and this file is the correct record.

### 2.18 — THE CONTROLLER'S BRIEF OFFERED `clientAuthFor` AS AN EXAMPLE SYMBOL; IT DOES NOT EXIST

Caught by a subagent **which greped before writing**, in `internal/tls`. An example symbol in a brief is
a claim like any other.

### ⚠️ 2.18b — A NONEXISTENT PACKAGE SELECTOR FAILS LOUDLY FOR THE WRONG REASON

`./internal/xds/sds/...` printed `no such file or directory` plus `FAIL [setup failed]` and **rc=1**,
three times — reading **exactly like a genuine test failure**. The package is `./internal/boot`. A
non-zero rc from a package selector must be triaged before it is believed.

### ⚠️ 2.19 — FOUND AT THIS CLOSE: THE CONTROLLER'S MERGED `--numstat` TOTAL DISAGREES WITH ITS OWN TABLE

The brief quotes `f647dd72..HEAD` as **`+1355 / -93`** over fifteen files. Re-derived at the publishing
tip, the same fifteen-row table sums to **`+1347 / -92`**, and `git diff --numstat` **RESTRICTED TO THE
SAME FIFTEEN-FILE SCOPE** agrees with the sum, not with the quoted total. **The per-file rows were
right; the total was not.** `reference_re_derive_at_the_publishing_commit` fires against the brief this
task was written from.

⚠️ **SCOPE CORRECTION, phase-95 final review (finding F3) — and it is a DIFFERENT defect from the
under-enumeration this subsection is about.** The under-enumerated total above is a **sum-vs-rows**
error. Separately, the *unrestricted* `git diff --numstat f647dd72..HEAD` does **not** print fifteen
rows at all at the publishing tip: this row's own FOUR close artefacts (`ROADMAP.md`, `STATE.md`,
`STATE_HISTORY.md` and this `PROGRESS.md`) land after the figure is taken, so the bare command reads
nineteen files and a larger total. **The fifteen-file figure is right for the scope it was measured
over and wrong for the command as bare-quoted.** §6 now states the scope and gives both figures. Do
not read this scope note as the sum-vs-rows defect above; they are independent.

---

## 3. The fix, and what actually guards it

**Site.** `internal/tls/config.go`, at the **ENTRY** of `NewDownstreamConfig` — **not at a return.**
The PLAN's own §0 records that the prototype's first "at the return" placement landed in the wrong one
of **six** return sites and changed nothing; an unscoped return-statement grep reads **seven**, and the
seventh is `NewQUICDownstreamConfig`'s, which the design forbids touching.

**Shape.** TCP only. On a `ClientHelloInfo` whose `SupportedProtos` is non-empty and intersects the
chain's `NextProtos` **not at all**, hand `crypto/tls` a `Clone()` of the fully-built config with
`NextProtos = nil`. Empty offer ⇒ untouched. Overlapping offer ⇒ untouched. The clone is taken
**per handshake, at handshake time**, which is what keeps `ClientCAs` and `ClientAuth` populated (§2.7).

**Reachability was proven, not inferred.** A `panic()` reachability control was placed in the mismatch
branch and **fired on 2 of 4 drives** — which also **retires PLAN row 8's post-fix value**, since once
the callback intercepts first, only row 7 discriminates. A green test run is not evidence a site is
exercised; the panic control is.

**Cross-side.** Fixture `0120-tls-connection-error` gained arm **(vi)** (non-overlapping offer,
RECORDED and never drive-fatal) and arm **(vii)** (the replacement for the vacuous cell 13, pinning
`NegotiatedProtocol` **per side**: subject `"h2"` -> `""`). Both YAMLs gained `alpn_protocols` as the
first key of `common_tls_context`, same key and same value on both sides. Arm arithmetic re-measured:
`ssl.connection_error` **STAYS 3**, `ssl.handshake` **1 -> 2**.

---

## 4. THE SIX-GATE POSTURE — ⚠️ NAMED, NOT CLAIMED

### (a) Differential suite — full

**`122/122 PASS, 0 FAIL, 0 SKIP`**, **RC=0**, **406.690s**, anchored FAIL **0**, panic **0**, **no
abort**. Every run `-count=1`.

Fixture set reconciled **BY NAME in BOTH directions**: dirs **122** = imports **122**, `comm` **0/0**
both ways, executed-vs-disk also **0/0**. ⚠️ **The extractor itself was NC'd on a scratch copy, and the
count-only form was PROVEN VACUOUS** — it stays **122** under a rename, while both `comm` arms fired.

⚠️ **Fixture `0120` is GREEN on its FIRST-EVER run with the fallback present.** It was deliberately
built on a branch based on the **un-fixed** tip so that its pre-fix RED could be measured first.

⚠️ **RE-RUN AT THE FIXED TIP `f667a411`, AFTER the final-review fix wave — because that wave changed
`internal/listener/manager.go`, which is on the SUBJECT's listener-build path, and narrowed config
acceptance at boot.** The fix wave argued its fixture edits were documentation-only; that argument
does not cover a `.go` file on the build path, so the gate was RE-EXECUTED rather than inherited:

| measure | value |
|---|---|
| `PIPESTATUS_RC` | **0** |
| `--- PASS: TestDifferential` | **399.93s** (package `ok … 404.306s`) |
| per-fixture `--- PASS` | **122** |
| anchored FAIL | **0** |
| `--- SKIP` | **0** |
| `^panic:` | **0** |
| abort | none |
| `0120-tls-connection-error` | **PASS (1.76s)** |

Denominator reconciled BY NAME in BOTH directions at that tip — `dirs=122  imports=122
executed=122`, `comm -3` EMPTY on dirs-vs-imports AND on imports-vs-executed (extraction anchored
on the blank-import LINE; a count-only check would have been vacuous). ⇒ the boot-acceptance change
broke nothing across 122 fixtures, **proven by execution rather than by the docs-only argument.**

### (b) Non-Docker sweep — ⚠️ **A DEPARTURE. BOTH RUNS ARE REPORTED.**

| run | result |
|---|---|
| **run 1** | `PIPESTATUS[0]`**=1**, anchored FAIL **8**, **TWO failing packages** — 123 ok + 2 FAIL + 110 no-test = **235**, RUN=**8731** |
| **run 2** (controller, same 235 packages, `-count=1`) | **RC=0**, 125 ok + 110 no-test = **235**, FAIL=**0** |

⚠️ **A GREEN RERUN CLEARS NOTHING** (`reference_recurring_flake_may_be_production_bug`). The two
failures, triaged rather than dismissed:

- **`TestSDSEndToEnd_FetchFailure_BootFailsClosed`** (`internal/boot`) — a **REGISTERED** flake
  (`reference_sds_dial_budget_flake`). **4/4 green in isolation at HEAD.** ⚠️ **It is NOT structurally
  immune**, and this is stated rather than glossed: `internal/boot` **does** depend on `internal/tls`
  and `internal/listener`. What weakens a causal link is narrower — the install is gated on a
  **non-empty `NextProtos`**, and that test drives an SDS fetch failure through a config with **no
  `alpn_protocols` at all**, so the new branch is unreachable on its path.
- **`TestFramer_ReaderGoroutineDoesNotLeak`** (`internal/filter/hcm/h2`) — ⚠️ **NOT PREVIOUSLY
  REGISTERED. NEW.** Client-preface `connection reset by peer` at `framer_leak_test.go:186`.
  `go list -deps -test` shows **no** dependency on `internal/tls`, `internal/listener` or
  `internal/xds`, and this row touches **no file in that package**. **3/3 green in isolation at HEAD.**
  ⇒ contention-sensitive under the parallel sweep. **RECORDED AS A NEWLY REGISTERED FLAKE**, not as a
  cleared one.

### (c) h2spec conformance

Exact: **`95 tests, 94 passed, 1 skipped, 0 failed`**. The skip is 6.9.2/2, invariant.

### (d) Fuzzers — reconciled against `^func Fuzz`

**56 targets / 48 files (+0).**

### (e) The ANCHORED panic gate — ⚠️ AND IT IS PROVEN LIVE

`^panic:|DATA RACE|SIGSEGV` reads **0**. Proven live in both arms: it **fired** on a scratch `^panic:`
and on a scratch `DATA RACE`, and **correctly read 0** on a green run containing a **mid-line**
`panic:` mention — which is exactly why the anchor is there.

### (f) `REVIEW.md`

**ABSENT — a STANDING DEPARTURE, NAMED not claimed.** The lifecycle's IMPL artefact is this
`PROGRESS.md`; no review artefact was produced for this row.

### (g) The NINE guarded non-sites — byte-untouched, AND PROVEN LIVE

Corrupting one literal on a scratch copy **fired on exactly that row**. ⚠️ **Their LINE numbers drifted
this row** — `manager_test.go:1276` -> `:1280`, `:1732` -> `:1736` — **so the guard is BY LITERAL, never
by line.** (Count corrected from TEN to NINE; see §2.17.)

### (h) Format and lint, gated on OUTPUT

`gofmt -l` **OUTPUT empty**. `golangci-lint` **v1.64.8, rc=0** on `internal/tls`, `internal/listener`,
`internal/xds` and the `0120` driver. US-locale misspell sweep clean.

---

## 5. Sentinel — RE-RUN AFTER THE ROW-95 FLIP

⚠️ **NC SHAPES CHANGE ACROSS A ROW FLIP. NOTHING BELOW IS INHERITED FROM §0.**

```
(1) (silent)                             <- row 95 now reads `done`; want STAYS 127
(2) 205:remaining deferred (not-yet-chartered) candidates:
    211:remaining deferred (not-yet-chartered) candidates:
    217:remaining deferred (not-yet-chartered) candidates:
    227:remaining deferred (not-yet-chartered) candidates:
    233:remaining deferred (not-yet-chartered) candidates:
    241:deferred candidates:              <- STILL SIX
(3) (silent)
```

| control | BEFORE | **AFTER** |
|---|---|---|
| NC-A (row 62 doctored) | TWO lines | **ONE** line: `NOT DONE: row 62` |
| NC-B (`want=126`) | TWO lines | **ONE** line: `GATE FAIL: examined 127 data rows, expected 126` |
| NC-C (`gRPC-family row` neutralised) | FIRED, residual 0 | **FIRED**, residual **0** |
| NC-D (`-family row`, `--` passed) | 96 / 68 | **96 / 68** |
| check-(2) positive control | residual 0, 6 substitutions | residual **0**, **6** substitutions |

⚠️ **NC-B ALSO DROPPED FROM TWO LINES TO ONE**, not only NC-A — it loses its `NOT DONE: row 95` line by
the same mechanism. ⚠️ **CORRECTION, MEASURED AGAINST THE SOURCE RATHER THAN ASSUMED: `next-prompt.txt`
DID forecast this.** Its NC-B block reads *"READS TWO LINES BEFORE THE FLIP … ONE AFTER"*, so the router
was right about both NCs; the omission was in the controller's Task-17 brief, which restated NC-A's shape
change and not NC-B's. **Both shapes were re-measured at this tip, neither inherited** — which is why the
gap in the brief cost nothing.

**Per-line md5 of the six windows AFTER the edit** (trailing newline INCLUDED):

```
205 10d7807bf02d   211 4a92f7e62fc6   217 2a7eb298b9fd
227 242e53c6f7a3   233 b2680e6f4fbf   241 6caa1c3ce0e7
```

**ALL SIX BYTE-IDENTICAL to §0.** ⚠️ The phase-94 rule held: **NEVER RE-SPELL A SENTINEL MATCH PHRASE
INSIDE A SENTINEL WINDOW** — at phase 94 the sentence asserting the sentinel could not move MOVED it,
six -> seven. This row's summary sits at `ROADMAP.md:157`, far outside every window, and says
"BANKED, NOT CHARTERED" where the phrase would otherwise be spelled.

**Field counts, BEFORE and AFTER installing the summary:**

| form | BEFORE | AFTER |
|---|---|---|
| naive `awk -F'\|' '/^\\\| *95 /{print NF}'` | **8** | **8** |
| escape-aware (`sed 's/\\\|//g' F \| awk …`, **no file argument to awk**) | **8** | **8** |
| naive malformed rows, whole file | **17** | **17** |
| escape-aware malformed rows, whole file | **2** (IDs 57, 69) | **2** (IDs 57, 69) |

The appended summary carries **zero pipe characters** — the pipe was **reworded away**, never escaped.
Row 95 still carries exactly **7** `|` characters, the delimiters only. `ROADMAP.md` STAYS **245**
lines and **127** data rows: a FLIP, not an ADD.

⇒ **THE SENTINEL STILL DOES NOT FIRE.** Check (2) reads SIX; **no candidate line was deleted and none
was "tidied"** — deleting the last one ends the project, and the history is `0 -> 1 -> 3 -> 4 -> 5 -> 6`
across ~40 phases. `stop` was evaluated after the flip and **deliberately NOT created**.

---

## 6. Cost — MEASURED by `--numstat`, never by `--stat`

⚠️ **THE SCOPE IS PART OF THE FIGURE — a bare `git diff --numstat f647dd72..HEAD` does NOT reproduce
it** (phase-95 final review, finding F3). The fifteen-file table below is **the implementation range,
EXCLUDING this row's own four close artefacts**, which land after the figure is taken. The command that
reproduces it EXACTLY is:

```
git diff --numstat f647dd72..0cdb9050 -- . \
  ':(exclude)docs/envoy-go/ROADMAP.md' \
  ':(exclude)docs/envoy-go/STATE.md' \
  ':(exclude)docs/envoy-go/STATE_HISTORY.md' \
  ':(exclude)docs/envoy-go/phases/95-tls-alpn-mismatch-fallback/PROGRESS.md'
```

⇒ **fifteen files, `+1347 / -92`**; `.go` only, **nine files, `+1038 / -56`**. The **FULL RANGE** at the
same commit, close artefacts INCLUDED, reads **nineteen files, `+1829 / -103`**. Both re-derived at
`0cdb9050`, which is named rather than written as `HEAD` because `HEAD` moves: the phase-95 final-review
fix wave lands after `0cdb9050` and grows both figures again. The per-file rows:

```
docs/envoy-go/BEHAVIOR_CONTRACT.md                            1   1
docs/envoy-go/DECISIONS.md                                   41   1
internal/listener/manager.go                                  6   0
internal/listener/manager_test.go                           570  18
internal/listener/tls_handshake_negative_test.go             55  21
internal/tls/config.go                                       62   2
internal/tls/config_test.go                                 200   0
internal/tls/doc.go                                           6   2
internal/xds/secret.go                                        4   2
internal/xds/secret_test.go                                   2   1
test/fixtures/0120-tls-connection-error/README.md           103  17
test/fixtures/0120-tls-connection-error/driver/driver.go     133  10
test/fixtures/0120-tls-connection-error/envoy-go.yaml         8   0
test/fixtures/0120-tls-connection-error/envoy.yaml            8   0
test/fixtures/0120-tls-connection-error/expectations.yaml   148  17
```

⚠️ **THE ROW TOUCHES TWO FILES BEYOND `PLAN.md` §4's DECLARED MAP — DECLARED HERE, NOT SILENT.**
`internal/xds/secret.go` and `internal/xds/secret_test.go` are **comment-only** edits repairing
citations that **this row's own doc-comment growth** invalidated (§2.9). This is a departure from the
PLAN's scope map and is named as one.

**Other measured counts at this tip**, each re-derived:

| object | value |
|---|---|
| `internal/tls/config.go` | 582 -> **642** (PLAN §7 said 629 — §2.12) |
| `DECISIONS.md` | 18902 -> **18942**; `^---$` STAYS **216**; `^## ADR-` STAYS **316**; bare `^## ` STAYS **324** |
| ADR tail / next-free | STAYS **ADR-0317** / **ADR-0318** (TAIL-derived; headings+1 collides at the ADR-0209 gap) |
| house `PROPOSED` guard | **DISARMED 1 -> 0**, verified BY LINE and BY ADR (backward heading search resolves to `18874 ## ADR-0317`) |
| ADR-0231 decoy at `:14866` | STAYS **1**, byte-identical, and is a **different matcher** from the house form |
| `BEHAVIOR_CONTRACT.md` | STAYS **5989** lines — the `:1944` edit is WITHIN-LINE, 357 -> **1841** chars; `:1961` and `:1971` md5-identical |
| fixtures / phase dirs | **122** (+0; `0121` still FREE) / **136** (+0) |
| fuzzers / BackendKind tail / `go.mod` | **56 / 48** (+0) / **38** (+0) / **67** require entries (+0) |
| `go list ./...` | **237** (**235** excluding the two Docker drivers) |
| stat surface | **+0** — a DELTA, **never an absolute**; three inconsistent absolutes are live at one tip, and by the ledger's own convention a `+0` row earns **no chain entry** |

**What `+0` means here:** no new metric name. What changes is **which** of `ssl.handshake` /
`ssl.connection_error` a mismatch arm increments.

---

## 7. What this row did NOT do — banked, not chartered

- **The driver-owned receiver port race** (~36 driver files by the probe-and-rebind shape, not 14) —
  still the most defensible next pick.
- **No fixture asserts `NegotiatedProtocol` cross-side outside `0120`** — a whole class of ALPN
  divergence is unguarded.
- **No fixture combines SDS with `alpn_protocols`** — recorded at **ADR-0317 §Context ¶8** as ABSENT
  COVERAGE, deliberately not chartered.
- **FOUR MORE STALE CITATIONS, already false at `f647dd72`** and therefore **not this row's**:
  `internal/xds/secret.go:98` cites `config.go:13` (the import is at `:14`); `manager_test.go:4482`
  cites `(:653)` (the func is at `:655`); and `config_test.go:1395 :1412 :1415 :1430 :1431` cite
  `:227` / `:141` / `:90-113` / `:108`, all pointing at unrelated text — the
  `NewQUICDownstreamConfig (config.go:90-113)` claim is badly wrong, that function was at `:273`.
- `expectations.yaml`'s first line still reads *"Phase 94 fixture …"* while `README.md` says *"Phase 94,
  extended by phase 95"* — a one-line inline clarification, deliberately not made.
- The **nine** history-class `GetConfigForClient` comment sites stay **GUARDED, not consolidated**.
- `0108`'s two *"emits NO `ssl.*` stats whatsoever"* confessions · `0118:31`'s falsified *"TLS/SDS
  band"* · `0061-lb-ring-hash`'s σ-margin second occurrence · 1xx interim responses · the H/1
  no-`Host` divergence · the pooled-upstream-lifetime defect · the other TEN fixed `ssl.*` names and
  FOUR dynamic families, blocked on NAMING.

---

## 8. FINAL WHOLE-BRANCH REVIEW — APPROVED WITH FINDINGS; ALL FIVE CLOSED IN THIS BRANCH

The final review returned **APPROVED WITH FINDINGS**: ONE Important behavioural hole this row opened,
and four prose defects, TWO of which were already false at the landed tip. All five are closed below,
before the squash.

### ⚠️ F1 (IMPORTANT) — THIS ROW'S OWN "TCP-ONLY" SAFETY CLAIM WAS TRUE OF THE SYMBOL AND FALSE OF THE PATH

`ADR-0317` **D-ALPNFB-TCPONLY** and `internal/tls/config.go`'s doc comment both said the ALPN-mismatch
fallback is never installed on a QUIC path, on the strength of `NewQUICDownstreamConfig` being
byte-untouched. **That is a statement about the SYMBOL. The PATH was open.**

`internal/listener/manager.go`'s `filter_chains[]` loop branches on `kind == kindQUIC` and calls
`NewQUICDownstreamConfig`. Its **`default_filter_chain`** block did **NOT** — it called
`NewDownstreamConfig` **UNCONDITIONALLY**. And `manager.go`'s clause-1 check accepts a listener with
**zero `filter_chains[]` and only a `default_filter_chain`**, while `(*listenerRuntime).quicTLSConfig()`
returns `rt.defaultChain.tlsCfg` **FIRST**. So a `kindQUIC` listener whose default chain carried a
**plain** `DownstreamTlsContext` with `alpn_protocols` built through the **TCP** builder, got
`installALPNMismatchFallback` installed, and had that very config handed to `quic.Listen` — a
non-overlapping offer then received a clone with `NextProtos = nil`, `negotiateALPN`'s
`if quic && len(serverProtos) != 0` read FALSE, and **the RFC 9001 §8.1 rejection this row's own ADR
calls mandated was silently DISABLED.** Before this row the same config rejected. **This row opened it.**

⚠️ **THE HOLE WAS RUN RED BEFORE IT WAS CLOSED. A green run after the fact is not evidence it existed.**
`TestBuildListenerRuntime_QUICDefaultFilterChain_PlainTLSRejects` was written and run against the
**un-fixed** code first, and printed, verbatim:

```
--- FAIL: TestBuildListenerRuntime_QUICDefaultFilterChain_PlainTLSRejects (0.00s)
    manager_test.go:1033: HOLE: a QUIC default_filter_chain built through the TCP builder —
    quicTLSConfig() returns a config with GetConfigForClient != nil (NextProtos=[h3]), so the
    phase-95 ALPN-mismatch fallback is installed on the config handed to quic.Listen and
    negotiateALPN's RFC 9001 Section 8.1 rejection is DISABLED
```

Its **positive companion** was red on the same run for the mirror-image reason — a properly
QUIC-wrapped `default_filter_chain` could not build at all, because the TCP builder rejected the QUIC
type URL:

```
--- FAIL: TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos (0.00s)
    manager_test.go:1058: NewManager(quic listener, quic-wrapped default_filter_chain): listener:
    "quic_listener_default_chain": default_filter_chain: tls: downstream: unexpected type_url
    "type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport"
```

⇒ **the default chain was not merely permissive about QUIC, it was wrong in BOTH directions.**

**THE FIX MIRRORS THE BRANCH THAT ALREADY EXISTED** — no new policy. The `default_filter_chain` block
now carries the same `kind == kindQUIC` branch, so:

- a **plain** `DownstreamTlsContext` on a QUIC default chain **BOOT-REJECTS** with
  `NewQUICDownstreamConfig`'s existing `"unexpected quic transport_socket type_url %q"` error —
  byte-for-byte the config-parity reject `filter_chains[]` already produced for the same shape;
- a **properly QUIC-wrapped** default chain **builds**, keeps `NextProtos`, and carries **no**
  `GetConfigForClient`, so the RFC 9001 guard stays ARMED.

Both tests pass after the fix (`RUN 2 / FAIL 0`, `-count=1 -v`). ⚠️ **No existing test or fixture
regresses, VERIFIED rather than trusted**: a sweep for a QUIC listener carrying a `default_filter_chain`
returns **ZERO** hits across every fixture YAML and every `_test.go`, and the full `./internal/listener/`
package is green.

`ADR-0317` §Context ¶4 and D-ALPNFB-TCPONLY are **amended IN PLACE**. ⚠️ That is correct and is **NOT**
an append-only violation: `DECISIONS.md` is append-only for landed ADRs of **EARLIER** phases, and
`ADR-0317` is **THIS row's own ADR, landed in this same unsquashed branch**. All four structural guards
re-measured after the amendment and **UNCHANGED**: `^---$` **216**, `^## ADR-` **316**, bare `^## `
**324**, tail **ADR-0317**, house `PROPOSED` guard **0**.

### F2 (MINOR) — `STATE.md`'s `PROGRESS.md` LINE COUNT WAS FALSE AT THE TIP

`STATE.md` stated `PROGRESS.md` (**465**). Commit `0cdb9050` added `+6/-2` to `PROGRESS.md` and did not
roll `STATE.md`. **A false present-tense count in the one file whose own preamble promises a session may
trust a grep of it.** Re-derived by `wc -l` at this tip and corrected there.

### F3 (MINOR) — A `--numstat` FIGURE QUOTED WITHOUT ITS SCOPE

§6 and §2.19 named `git diff --numstat f647dd72..HEAD` and reported *"fifteen files, `+1347 / -92`"*.
The bare command reads **nineteen files, `+1829 / -103`** at `0cdb9050`, because this row's own four
close artefacts land after the figure is taken. **The number is right for the scope it was measured over
and wrong for the command as quoted.** §6 now quotes the exclusion-pathspec command that reproduces the
fifteen-file figure EXACTLY and gives the full-range figure beside it; §2.19's *"agrees with the sum"*
claim is scoped to the same fifteen files, with the scope defect explicitly separated from the
sum-vs-rows defect §2.19 is actually about. Both re-derived. The `ROADMAP.md` row-95 summary carries the
same scope clause.

### F4 (MINOR, AND FALSE AT THE LANDED TIP) — THE "COUNTERS ARE BLIND" MEASUREMENT DESCRIBED THE SIX-ARM SHAPE

`driver.go`'s `wantNegotiatedProtocol` comment and `README.md`'s table both said, in the present tense,
that deleting `alpn_protocols` from the subject YAML leaves the fixture GREEN with both sides at `3`/`2`.
**That was measured BEFORE arm (vii) existed.** At the landed SEVEN-arm shape the pins are `3`/**`3`**
and the same deletion is **caught**: arm (vii) is strict, `record()`s a prob, `driveSide` returns an
error and the runner's `t.Fatalf("subj drive: %v", err)` reddens the fixture. ⚠️ **THE MEASUREMENT IS
RETAINED, NOT DELETED** — it is the evidence that the counters are blind, which is precisely why arm
(vii) exists. Both passages are **re-scoped** to say it was taken on the six-arm shape and to state what
the seven-arm shape does instead: the deletion is caught **by arm (vii)**, still **NOT** by the counters.

### F5 (MINOR) — TWO OF THE THREE ENTRY GUARDS ARE UNREACHABLE, AND THE PROSE OVERSTATED THE TESTED SURFACE

`installALPNMismatchFallback` guards on `cfg == nil`, `len(cfg.NextProtos) == 0` and
`cfg.GetConfigForClient != nil`. `commonTLSContextToConfig` returns either `(nil, err)` or
`(non-nil, nil)`, so **`cfg == nil` is dead**; no caller assigns `GetConfigForClient` before the install,
so **`cfg.GetConfigForClient != nil` is dead**. Only `len(cfg.NextProtos) == 0` is live, and it is the
only one SPEC §12 cell 9 exercises. ⚠️ **THE CODE IS LEFT ALONE** — both are cheap defensive invariants
and `SPEC.md` §4 is frozen verbatim. **Only the prose is corrected**, in `internal/tls/config.go` and
`internal/tls/config_test.go`: ONE guard is live and exercised by a negative control, TWO are defensive
and currently unreachable, and no pin claims to exercise them. `SPEC.md:217`'s *"three guards … each
separately testable"* is a **prior-stage artefact and is left byte-untouched**; the correction is
recorded here and at the code, where a reader meets it.

---

## §9 — THE SCOPED RE-REVIEW OF THE FIX WAVE, AND WHAT IT REFUTED

The final whole-branch review's fix wave (`b8d29a00`, `f667a411`) was itself put through ONE scoped
re-review of the range `0cdb9050..f667a411`. Verdict: **APPROVED WITH FINDINGS — one Important, four
Minor.** ⚠️ **The Important one refuted a claim the fix wave had just landed in FOUR documents**, which
is the point of the seam: the stage that refutes you may be your own review of your own correction.

Adjudication: the SDD budget of one fix dispatch plus one scoped re-review was spent, so F-A..F-D were
closed by the CONTROLLER directly — all four are prose-only with **zero code change** — rather than by
opening a second wave. F-E was banked. Nothing here touches `.go` logic; the only `.go` edit is a
comment.

### F-A — IMPORTANT. "Pure config parity, no new policy" was INCOMPLETE, and the omitted half is a WIDENING.

The F1 fix routes a QUIC `default_filter_chain` to `NewQUICDownstreamConfig`. That does not only
*narrow* acceptance (the plain-`DownstreamTlsContext` shape now boot-rejects); it also **WIDENS** it. A
`kindQUIC` listener whose only chain is a `default_filter_chain` wrapping `require_client_certificate:
true` plus a `trusted_ca` **used to boot-REJECT** (the TCP builder refused the QUIC type URL) and **now
BUILDS — with `ClientAuth = NoClientCert` and `ClientCAs = nil`**, because `NewQUICDownstreamConfig`
never evaluates `require_client_certificate` at all.

⚠️ **Nothing that previously shipped regresses**: the identical shape under `filter_chains[]` already
behaved exactly this way (the pre-existing, DECIDED gap at `BEHAVIOR_CONTRACT.md` item 8(b) /
D-RCCF-QUIC). The fix is still correct — it makes the default chain consistent with a posture the tree
had already adopted, and it closes a VACUOUS RFC 9001 §8.1 rejection. But **parity with a known gap
inherits that gap into a shape that used to be refused**, and an operator writing a mutual-TLS QUIC
listener in that shape now boots and serves H3 accepting any client presenting no certificate.

⚠️ Note the shape of the error: a bare "no new policy" claim is the SAME symbol-vs-path mistake this
row already recorded once at F1. The claim was true of the *narrowing* and false of the *whole change*.
The QUIC client-authentication gap stays OUT of scope (D-RCCF-QUIC); what this row owed, and now
discharges, is the DISCLOSURE — landed at `ADR-0317` D-ALPNFB-TCPONLY, here, and in `STATE.md`.

Incidentally discharged and recorded so it is not re-chartered: phase-61 deferred item **M-FB1**.

### F-B — MINOR. This row FALSIFIED a `BEHAVIOR_CONTRACT.md` sentence and did not correct it.

`BEHAVIOR_CONTRACT.md` item 11 read *"(A QUIC listener's `default_filter_chain` goes through plain
`NewDownstreamConfig` and IS covered by the pre-scan — consistent with the rest of the model.)"* — true
when written, **false from the moment F1 landed**, and the file was untouched by the fix range. ⚠️ That
is this row's own failure mode reappearing in the one file the row exists to keep accurate. Corrected
WITHIN-LINE (the file's 5989-line count is a guarded constant): the SDS pre-scan model is now
**UNIFORM** — no QUIC-listener shape has its inner `DownstreamTlsContext` inspected — which strengthens
the "benign and arguably load-bearing" reading rather than weakening it, since no QUIC shape can
inflate `seen`.

### F-C — MINOR. F4 was fixed in TWO of THREE copies.

The six-arm measurement was re-scoped in `driver/driver.go` and `README.md`, but a **third** copy
survived in `expectations.yaml`, present-tense, at the header comment AND — self-contradictingly —
inside **arm (vii)'s own `role:` field**, where it claimed the subject-side deletion "leaves the stat
counters reading 3/2 on both sides and the fixture GREEN". Arm (vii) is precisely what catches that
deletion. Both re-scoped to the past tense with the seven-arm reality stated (pins 3/3, caught by arm
(vii), still NOT by the counters) and an explicit *"do not reconcile `handshake_delta` back down to 2"*.
YAML re-parsed after the edit. ⚠️ **The lesson is the roster one: a corrected claim must be reconciled
across its whole occurrence SET, not at the sites the fixer happened to open.**

### F-D — MINOR. A citation that does not hold.

`manager_test.go` cited *"`TestBuildListenerRuntime_QUICKind`'s sibling roster"* for the
`filter_chains[]` half of the config-parity claim. `TestBuildListenerRuntime_QUICKind` (`:935`) is a
POSITIVE build test with no reject arm and no roster. Re-derived: the reject string has exactly TWO
in-tree test hits — this row's own new assertion, and `internal/tls/config_test.go:1229`, which pins the
BUILDER's error and not the listener-level parity. ⇒ **the `filter_chains[]` half is pinned NOWHERE**,
so dropping that branch would leave the package green. The false citation is replaced by that statement
rather than by a different citation.

### F-E — MINOR. The mirroring is PARTIAL. Banked, not fixed.

`filter_chains[]` rejects a QUIC chain with no `transport_socket` at config-parse (*"quic listener
requires a transport_socket (mandatory TLS)"*). The `default_filter_chain` block carries no such arm, so
that shape still BUILDS. ⚠️ **NOT a safety hole** — `internal/listener/quic.go` catches the nil config at
Start (*"quic listener has no TLS config (mandatory TLS not built)"*) before `quic.Listen`. It is a
deferred-reject / config-parity departure, PRE-EXISTING and unchanged by this row. Independently
measured by the controller and by the re-reviewer, who agreed. **Banked as the smallest defensible next
candidate; not folded into a closed row.**
