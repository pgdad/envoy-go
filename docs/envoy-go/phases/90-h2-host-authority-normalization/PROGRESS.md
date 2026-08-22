# Phase 90 — `h2-host-authority-normalization` — PROGRESS

Append-only. One entry per lifecycle stage.

---

## BRAINSTORM — done 2026-08-19

**Base master:** `6b4bc7c0` · **Branch:** `phase-90-brainstorm` · **lifecycle-state DONE -> 1**
**Execution style:** subagent-driven per `feedback_execution_style` — three probe agents on disjoint
detached worktrees, disjoint port bands, private scratch each, **each committing nothing and each proving
its tree clean**; the controller ran the sentinel battery, the counts, and **re-derived every load-bearing
agent claim by execution**.

### What landed

- `docs/envoy-go/phases/90-h2-host-authority-normalization/BRAINSTORM.md` (new)
- `docs/envoy-go/phases/90-h2-host-authority-normalization/PROGRESS.md` (this file, new)
- `docs/envoy-go/ROADMAP.md` — row 90 registered `in-progress`, numstat **`1 0`** (a pure insertion),
  239 -> 240 lines, 121 -> 122 data rows, sentinel `want` **121 -> 122** in the SAME commit
- `docs/envoy-go/STATE.md` — rolled IN PLACE
- `next-prompt.txt` — rolled (`git add -f`)

**Docs-only: ZERO production `.go`, ZERO test `.go`.** `DECISIONS.md` and `BEHAVIOR_CONTRACT.md`
**BYTE-UNTOUCHED** — a BRAINSTORM adds no ADR. Next-free stays **`ADR-0312`**; the strict
`^> **STATUS: PROPOSED` guard **STAYS AT 0** and the SPEC re-arms it 0 -> 1.

### The pick

**`h2-host-authority-normalization`**, SELF-PICKED per the 2026-07-12 standing directive. Four candidates
costed at this tip, each by a built-run-and-reverted prototype:

| candidate | production cost | files | packages | verdict |
|---|---|---|---|---|
| **`host`/`:authority`** | **+15 / −1** | **1** | **1** | **PICKED** |
| decode-side trailers | +37 / −4 | 3 | 1 | rejected — Lua hook needs coroutine rework |
| ADR-0310 C1 drain | +161 / −31 | 3 | 1 | deferred — best NEXT row |
| ADR-0310 C2 `max_request_headers_kb` | +76 / −2 | 5 | 3 | deferred — must not go before C1 |
| ADR-0310 C3 SETTINGS | +31 / −19 | 3 | 1 | **REJECTED — measured ANTI-parity** |

### Sentinel — ACTUAL output, both sides of the row add

| | check (1) | check (2) | check (3) |
|---|---|---|---|
| **BEFORE** (`want=121`) | SILENT | SIX at `:199 :205 :211 :221 :227 :235` | SILENT |
| **AFTER** (`want=122`) | `NOT DONE: row 90` | SIX at `:200 :206 :212 :222 :228 :236` | SILENT |

⇒ **SENTINEL DOES NOT FIRE. `stop` NOT created** (verified absent at the git root and in the stage
worktree, both sides). Window COUNT and CONTENT unchanged; only anchors shift +1.

⚠️ **A DRAFT OF BRAINSTORM §8 PREDICTED "(1) SILENT" AFTER THE ADD AND WAS WRONG.** Row 90 is
`in-progress`, so check (1) MUST name it. Corrected to the measured output. **Caught only by running the
gate, not by reasoning about it.**

**All four NCs fired on the post-add file.** ⚠️ **NC-A CHANGES SHAPE while a row is in-progress**: it now
reads `NOT DONE: row 62` **and** `NOT DONE: row 90`, not row 62 ALONE. Both lines are required — row 62
proves the check is live, row 90 proves the add landed. NC-B `GATE FAIL: examined 122 data rows, expected
121`; NC-C residual 2 -> 0 then `NEVER OPENED: gRPC` with WASM silent; NC-D long **5** / short **1** /
union **6**.

### Counts at this close — re-derived mechanically, none copied

`ROADMAP.md` **240 / 122 rows** · `DECISIONS.md` **18277**, tail **ADR-0311**, next-free **ADR-0312**
(`grep -c '^## ADR-0312'` => 0), `^---$` **216**, headings **310**, strict `PROPOSED` guard **0** ·
`BEHAVIOR_CONTRACT.md` **5962** · `STATE_HISTORY.md` **506 -> 508** · `BOOTSTRAP_PROMPT.md` **522** ·
phase dirs **130 -> 131** · fixtures **121**, tail `0119-grpc-unary-trailers`, **`0120` FREE** ·
blank imports **121** (narrowed form; the unnarrowed reads 123 and is REFUTED) · fuzzers **55** ·
BackendKind tail **38** · stat surface **406** · slice-only-writer gate **6** · `-family row` **95 / 67** ·
`gRPC-family row` **2** · `Operational-tooling-family row` **3** · REVIEW.md **37 FILES**.

### Findings the next stage must not re-learn

1. **The phase-89 codec census is prose-contaminated AND structurally blind.** Config-only reads
   HTTP1 **212** / AUTO **2** / HTTP2 **0** / HTTP3 **0**, not 270/6/3; and the YAML view misses **46 of
   121** fixtures that build config in their Go driver. **The H2-capable downstream set is FOUR fixtures**
   — `0004`, `0079`, `0080`, `0119`.
2. **The differential arm CANNOT use `helpers.H2RoundTrip`**, proven at `x/net@v0.34.0`
   `http2/transport.go:2162` (drops a client-set `host`) and `:2146` (always synthesizes `:authority`).
   The instrument exists in-tree at `0119`'s driver.
3. **No existing test pins the defect** — controller-verified with a green baseline first, then green with
   the prototype across **69 packages**.
4. **ADR-0310 C3 is measured ANTI-parity** — the reference advertises no `0x6` either, with or without
   `max_request_headers_kb` set. Three documents file it as deferred parity; all three are wrong.
5. **The "~64 KiB encoded band" in ADR-0310 §Consequences (xi) is not reproducible.** RECORDED, NOT FIXED.
6. **A drift CORRECTION fired and was itself refuted** — the stat figure **406** was reported stale
   ("403"); 403 is a LINE count scoped to `internal/`, the canonical command counts occurrences repo-wide
   and reads 406. **The carried figure was right.**
7. **The ARM-A malformed-row figure reconciles only under an ESCAPE-AWARE field count** (naive reads 17).
8. **`STATE_HISTORY.md`'s "archive labels 202" is not reproducible** by any of six matcher forms. Carry no
   number; use the anchored form.
9. **Every port band this loop has assigned since phase 87 sits inside the kernel ephemeral range**
   (`32768 60999`) — which is also the mechanism behind the "driver-owned receiver port race" flake.

### Probe hygiene

All three probes: nothing committed, nothing pushed, every patched tracked file restored with
`sha256sum -c` verified, `git status --porcelain` and `git diff --stat 6b4bc7c0` both EMPTY, containers
torn down BY NAME, worktrees removed. Foreign containers (`infallible_booth`, `crazy_kare`, `golink-ai`,
`quizzical_goldstine`) deliberately left untouched.

⚠️ **A probe's `pgrep -f` matched and killed a SIBLING probe's process** (PID 2870478). The controller
issued an advisory; the victim discarded that arm entirely and re-ran every dependent arm with
`ss -ltnp` asserted before and after — **all re-run results held**. One flake observed:
`TestAcquireH2Stream_PromoteSkipsDrainingConn`, **6/6 green on retry**, and it fired **without `-race`**,
which widens the recorded `internal/cluster` outlier class.

---

## SPEC — done 2026-08-19

**Base master:** `f15d4f4e` · **Branch:** `phase-90-spec` · **lifecycle-state 1 -> 2**
**Execution style:** subagent-driven per `feedback_execution_style` — three probe agents on disjoint
detached worktrees, disjoint sub-32768 port bands (21000/22000/23000), private scratch each, **each
committing nothing and each proving its tree clean**; the controller ran the sentinel battery, re-derived
every count, and **re-measured every load-bearing agent claim on its own instruments**, refuting three of
them and one of its own predecessor's headline sentences.

### What landed

- `docs/envoy-go/phases/90-h2-host-authority-normalization/SPEC.md` (new)
- `docs/envoy-go/DECISIONS.md` — **ADR-0312 §Context** appended; strict `^> **STATUS: PROPOSED` guard
  **0 -> 1**; `18277 -> 18297`; headings **310 -> 311**; tail **ADR-0312**; ⚠️ `^---$` **STAYS 216**
- `docs/envoy-go/phases/90-h2-host-authority-normalization/PROGRESS.md` (this entry)
- `docs/envoy-go/STATE.md` rolled IN PLACE · `STATE_HISTORY.md` appended · `next-prompt.txt` rolled

**BYTE-UNTOUCHED:** `ROADMAP.md` (row 90 STAYS `in-progress`, `want` stays **122**) and
`BEHAVIOR_CONTRACT.md`. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

### The seven questions, all DISPOSED BY EXECUTION

| Q | decision |
|---|---|
| **Q1 SCOPE** | **D-90-SCOPE** — arms A + B, H/2 downstream leg only. Arm C, H1-B′ deferred; H1-D a NAMED DEPARTURE; H1-E closed as a non-divergence |
| **Q2 instrument** | **D-90-INSTRUMENT** — raw-framer client. `H2RoundTrip` refuted at the pinned source **and** on a live listener (all three shapes returned 200 while sending something else) |
| **Q3 fixture** | **D-90-FIXTURE** — extend `0004` in place; fixtures stay **121**, `0120` STAYS UNCONSUMED. **D-90-BACKEND** — add `r.Host`; BackendKind stays **38** |
| **Q4 arm C** | **D-90-REJECT: DEFERRED**, on four measured refutations of the recorded description |
| **Q5 H1-D** | **NAMED DEPARTURE** — envoy-go is the RFC 7230 §5.4-conformant side; parity costs ~5x the whole fix and rewrites the framing seam |
| **Q6 routing** | ⚠️ **THE PREMISE IS REFUTED** — routing is path-only; the blast radius is OBSERVABILITY |
| **Q7 skip-key** | **D-90-SKIP** — leave D-89-HOST's SKIP untouched; **retire its ground 2** and bank the follow-on |

### Six refutations by execution the PLAN must not re-learn

1. ⚠️ **`ROADMAP.md` row 90's own *"an empty authority is a ROUTE-MATCHING input"* is FALSE** — the route
   predicate is `matches(path string) bool` and cannot see the request; non-`["*"]` domains and
   `match.headers` both boot-reject. The IMPL corrects the row text.
2. ⚠️ **The BRAINSTORM named the WRONG SITE for arm C's reject** — `buildRequest` can signal only STREAM
   scope; the connection-scoped site is `(*serverStream).recvHeaders`.
3. ⚠️ **Arm C's rule is authority VALIDITY, not emptiness — and `host` is validated INDEPENDENTLY.** A
   *valid* `:authority` beside an *empty* `host` is torn down.
4. ⚠️ **Arm C's reaction is CONFIG-DEPENDENT and the recorded stat is incomplete** — the default emits
   **zero bytes**; `http2.rx_messaging_error` is the classifier that survives both postures; **neither
   stat exists in the subject.**
5. ⚠️ **H1-B is MIS-ATTRIBUTED** — HTTP/1.0 *with* a valid `Host` still 426s, so the 426 is the VERSION.
   The genuine arm (**H1-B′**, HTTP/1.1 with no `Host` ⇒ ref 400 / subj 200) was absent from the record.
6. ⚠️ **The BRAINSTORM's provenance grep reads 1, not 0** — at this tip and at its own. The conclusion
   survives (the match is the English word in *"acquired ADR authority"*); the measurement does not.

**Two corrections that are NOT refutations:** the "69 packages ok" figure is a flake artifact — the
clean-tip denominator is **70**; and the recorded `hpack.NewDecoder(n, nil)` SIGSEGV is **narrower** than
stated — safe when installed as `Framer.ReadMetaHeaders`.

### Cost and guard

Prototype **+34 / −0**, ONE file, ONE package, post-`gofmt`, symbol-asserted. Prior floor `+15/−1`
reproduces as a *minimum* (`+14/−1` comment-free) and is **overrun 2.3x** once the rule's guards are
written. `./internal/...` ⇒ **RC=0, 70 ok, 0 FAIL**, anchored panic gate **0**.
⚠️ **`buildRequest`'s authority is COMPLETELY UNPINNED** — corrupting it unconditionally leaves the whole
tree green — so the unit roster must cover it specifically. RED baseline captured with the `:authority`-only
positive control **PASSING** and arms A/B/C each failing on the predicted axes; 5/5 green with the
prototype.

### Sentinel

(1) **`NOT DONE: row 90`** at `want=122` · (2) **SIX** at `:200 :206 :212 :222 :228 :236` · (3) **SILENT**.
⇒ **TWO checks block it; `stop` NOT created.** All four NCs fired: NC-A `row 62` **AND** `row 90` ·
NC-B `GATE FAIL: examined 122 … expected 121` · NC-C residual 2⇒0 then `NEVER OPENED: gRPC`, WASM silent ·
NC-D long **5** / short **1** / union **6**.

### Probe hygiene

All three agents plus the controller: nothing committed, nothing pushed, every patched tracked file
restored with `sha256sum -c` verified, `git status --porcelain` and `git diff --stat f15d4f4e` both EMPTY,
containers (`a90-ref`, `b90-ref`, `b90-ref2`, `ctl90-ref`) torn down **BY NAME**, port bands released.
Foreign containers `infallible_booth`, `crazy_kare`, `golink-ai`, `quizzical_goldstine` deliberately
untouched. No `pgrep -f` collision this stage; the controller killed only a PID whose cmdline carried its
own scratch path. **No flake observed in any run at this stage** — `TestAcquireH2Stream_PromoteSkipsDrainingConn`
did not recur.

---

## PLAN — done 2026-08-22

**Stage:** lifecycle-state **2 -> 3**. Base master `c7fef29b`, branch `phase-90-plan`, ONE squashed commit.
Lifecycle position re-derived **from the phase DIRECTORY**, not from prose: `BRAINSTORM.md` + `SPEC.md` +
`PROGRESS.md` and **no `PLAN.md`** ⇒ BOOTSTRAP §5 state **2** ⇒ output `PLAN.md`. `STATE.md` agreed
independently (`lifecycle-state: 2`, `next-skill: the phase-90 PLAN`).

### What landed

`PLAN.md` created (**1039 lines**). Docs-only: **ZERO production `.go`, ZERO test `.go`** — gated by
`git diff --stat c7fef29b -- '*.go'` ⇒ **EMPTY**. `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md`
all **BYTE-UNTOUCHED** (verified by EMPTY DIFF against master, each individually). Row 90 stays
`in-progress`, `want` stays **122**, next-free stays **`ADR-0313`**, strict `PROPOSED` guard **STAYS
ARMED at 1**. Phase dirs stay **131** — a PLAN adds no directory.

### ⚠️ FIVE SPEC CLAIMS REFUTED BY EXECUTION, AND ONE MEASUREMENT AGENT

1. ⚠️ **THERE IS NO `host` BRANCH IN `stream.go`.** ADR-0312 §Context ¶1 reads as though `buildRequest`
   has a `host`-specific `regular.Add` site. `grep -in 'host' internal/filter/hcm/h2/stream.go` returns
   **exactly two lines, `:503` and `:507`, both the WRITE of `authority`**. A regular `host` reaches the
   **generic** `regular.Add` at `:472` with zero special-casing. ⇒ **the IMPL ADDS a branch, it does not
   modify one** — which is why the measured prototype is `−0`.
2. ⚠️ **THE GUARD ASYMMETRY IS BY PROPERTY, NOT BY SYMBOL.** SPEC §9.3 says `buildH2Request` has
   demonstrated sensitivity and `buildRequest` zero. Executed: `if h.Name == "host" { continue }`
   **unconditionally inside `buildH2Request`** — the exact production behaviour arm A introduces — leaves
   **`rc=0`, both packages `ok`. VACUOUS.** Only `H2Request.Authority`'s **VALUE** is pinned anywhere.
   ⇒ the roster is rebuilt around **four PROPERTIES**, not two symbols.
3. ⚠️ **COST +36/−0, NOT +34/−0**; `hostField` **6**, not 7; and a **THIRD** symbol the SPEC never names,
   `hostSeen` (**8**), is *required* — a bare `hostField != ""` cannot distinguish ABSENT from
   PRESENT-AND-EMPTY, nor express first-occurrence-wins. `reference_measured_prototype_is_a_lower_bound`
   fires a **third** time; the cause is again under-enumeration.
4. ⚠️ **THE FIXTURE CONSTRAINT IS OVER-BROAD.** SPEC §8.2's *"must not hit `/api`"* is false —
   `counts[idx]++` occurs at **exactly one line, `driver.go:308`**, inside the `/api/v1/<n>` loop, and the
   eight phase-89 arms already hit `/api/v1/reflect-headers/` through a different loop. ⇒ a `p90*` path
   takes the documented `a5` fall-through to `prefix: "/api"` and **BOTH `0004` YAMLs stay BYTE-UNTOUCHED**,
   removing the route edit and the `TestRenderBootstrap_*` updates entirely.
5. ⚠️ **"THE 69 WAS A MEASUREMENT ARTIFACT" IS PARTIALLY REFUTED.** A 69 reading was reproduced **at this
   stage** and it came with **`rc=1` and a real `--- FAIL`** (`TestSDSEndToEnd_FetchFailure_BootFailsClosed`,
   `internal/boot`). A **second** flake, `TestProvider_FetchInitialCertificate_Timeout` (`internal/xds`),
   fired on **3 of 8** full-suite runs. Both pass **5/5 in isolation**; both are the standing SDS
   dial-budget class. ⚠️ **Causation excluded MECHANICALLY, not by re-running:**
   `go list -deps -test ./internal/xds/ | grep -c 'filter/hcm/h2'` ⇒ **0** — that test binary does not
   link the patched package at all.
6. ⚠️ **AND A MEASUREMENT AGENT WAS REFUTED BY THE CONTROLLER.** An agent reported that `net/http` lifts
   the authority out of `r.Header`, so `0004`'s reflected block *"can never contain it"*. At the pinned
   source, `x/net@v0.34.0/http2/server.go` deletes **only `Trailer`** (`:2341`) and has **no `Del("Host")`**
   — a regular `host` survives into `req.Header` as `Host`. **SPEC §8.4's table is correct.**
   `feedback_brief_citations_not_evidence` applies to **subagent reports** exactly as to stage briefs.

### What the PLAN froze

**D-90-DUP** (NEW — the SPEC did not reach it): `:authority` has **no duplicate guard** while
`:method`/`:path`/`:scheme` all do, so an implementer following the local idiom would add a reject that is
an **unpriced behaviour change**. `authoritySeen` tracks **PRESENCE ONLY**; duplicate `:authority` keeps
silent last-wins, and a **stability-pin test** asserts the non-change so it is a measurement, not a claim.
**D-90-TARGET**: the differential axis is `host`-presence + `r.Host`. A route assertion is **forbidden**
(SPEC §4); the access-log/Zipkin axis is **rejected on a new measurement** — **not one of the four
H2-capable fixtures carries an access-log or tracing surface** — and is registered as follow-on **(vi)**.
**D-90-YAML**: both `0004` YAMLs byte-untouched (refutation 4).
**Arm block is NOT fail-fast** — it follows `0119`'s never-return-an-error discipline, because `0004`'s
in-band `return nil, fmt.Errorf(...)` would let the first red arm mask the rest.

### Sentinel — ACTUAL output, ONE side (`ROADMAP.md` byte-untouched)

(1) **`NOT DONE: row 90`**, denominator asserted **122** · (2) **SIX** at `:200 :206 :212 :222 :228 :236` ·
(3) **SILENT**. ⇒ **TWO checks block it; `stop` was NOT created** (verified absent at the git root and in
the stage worktree). **ALL FOUR NCs FIRED:** NC-A (`NC LANDED? [ in-progress ]` inspected FIRST) ⇒
`NOT DONE: row 62` **AND** `NOT DONE: row 90` — **both required** · NC-B `want=121` ⇒
`GATE FAIL: examined 122 data rows, expected 121` · NC-C residual **2 -> 0** confirmed first ⇒
`NEVER OPENED: gRPC`, WASM correctly silent · NC-D long **5** / short **1** / union **6**.
The **discriminating** provenance grep reads **0 across all six windows**, positive control **22** on row
90's own line, fabricated-token NC **0** six times.

### Counts and gate hazards found at this stage

⚠️ **`DECISIONS.md` 311 is `^## ADR-`-SCOPED** — bare `^## ` reads **319** (8 `## Amendment` headings).
Both are right; they measure different things. **State the scope whenever the number is restated.**
⚠️ **`0004` has SEVEN `H2RoundTrip` CALL SITES, not nine** — `grep -c` counts LINES and `:433`/`:739` are
prose. ⚠️ **`discoverFixtures` ends at `:1499`, not `:1497`.** ⚠️ **The brief's phantom gate
`git grep -c 'h2.parseHeadersForRequest'` reads 1, not 0** — it counts a comment citation; the
*definition* selector `^func.*parseHeadersForRequest` is the one reading 0.
⚠️ **THE `*H2Request` COMPANION COUNTER IS VACUOUS AS A CONTROL** — it reads 0, exactly like a fabricated
selector, so it cannot discriminate; use `git grep -c 'H2Request'` (**20** files).
⚠️ **NEW GATE HAZARD: on `-v` output, unanchored `grep -c 'FAIL'` reads 11 on a FULLY GREEN tree** (nine
`INFO wasm: FAIL_CLOSED/FAIL_OPEN` lines + two lines of a test *name* containing `boot-FAILS`). Use
`grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`, which reads 0 green.
⚠️ **AND THE `\|` TRAP FIRED ON THE CONTROLLER'S OWN SELF-REVIEW** — `grep -ciE 'a\|b'` matches a LITERAL
pipe, so three real hits read **0** and briefly looked like coverage gaps.

### Split gate — EVALUATED, not assumed

**NINE tasks** (≤ ~25) and **~+322 to ~+581 LoC** (≤ ~1500, the upper bound **2.6× under**).
⇒ **NOT TRIPPED. NO SPLIT**; `want` stays **122**, no sub-phase row minted. ⚠️ The bound holds **because**
arm C, H1-B′, H1-D and the H3 leg are deferred — an IMPL that absorbs any of them re-opens the gate.

### Probe hygiene

Three measurement agents; **two read-only in the canonical root, one on its own throwaway worktree**
(`wt-probe-90-red`, created off `c7fef29b`, used, and **removed** with its branch deleted). Every patched
tracked file restored with `sha256sum -c` verified; the probe test file deleted; the probe worktree's
`git status --porcelain` and `git diff --stat c7fef29b` both EMPTY before teardown. Canonical root
untouched throughout (`?? .claude/` only, pre-existing). **No commits by any agent. No Docker container
started and no differential run launched at this stage** — the §7 recipe is designed from read code, and
the PLAN says so in §12. No `pgrep -f` used. No port bound.

---

## IMPL — done 2026-08-22

**Stage:** lifecycle-state **3 -> DONE**. Base master `1f147262`, branch `phase-90-impl`, FOUR task commits
squashed at the close. Execution style: subagent-driven per `feedback_execution_style`, one agent per task
group, all work in the worktree `wt-phase-90-impl` per `feedback_git_worktrees`; no agent pushed.

### What landed

Code: `internal/filter/hcm/h2/stream.go` **+48/−0** · `internal/filter/hcm/h2/authority_norm_test.go`
(**NEW**) **+122/−0** · `test/fixtures/0004-h2-routing/driver/driver.go` **+354/−4** ·
`test/fixtures/0004-h2-routing/backends/main.go` **+7/−0**. Docs: `DECISIONS.md` (ADR-0312 §Decision +
§Consequences appended **IN PLACE**, guard **1 -> 0**), `ROADMAP.md` (row 90 -> `done` + the false
*"ROUTE-MATCHING input"* sentence corrected), `BEHAVIOR_CONTRACT.md:2034` rider,
`test/fixtures/0004-h2-routing/README.md`, this file, `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`.
**+0 on every count axis** — fixtures **121**, blank imports **121**, BackendKind tail **38**, new ports
none, `go.mod`/`go.sum` byte-untouched, fuzzers **55 / 48 files**, stat surface **406**, `0120` unconsumed,
**both `0004` YAMLs BYTE-UNTOUCHED**, `^---$` in `DECISIONS.md` still **216**, next-free still **ADR-0313**.

### T1 — RED census (commit `a25cabd8`)

`internal/filter/hcm/h2/authority_norm_test.go` NEW, tests only, **ZERO production bytes**. `package h2`,
in-package (both symbols unexported). Helper names `hf` / `carrierValues` landed **without rename** — the
seven `hf` hits in the package are all function-scoped `for _, hf := range …` loop variables.
**T1b reachability control:** `panic("NC-REACH-buildRequest")` as `buildRequest`'s first statement ⇒
`rc=1`, `panic: NC-REACH-buildRequest [recovered, repanicked]`, package FAIL. **Run TWICE** — with the new
file present (panic trips at `authority_norm_test.go:78`, proving THIS roster reaches the symbol) and with
it moved aside (same panic, proving the PRE-EXISTING suite reaches it too — the PLAN's §5.1 claim
re-derived, not inherited). Reverted; `sha256sum -c` ⇒ `stream.go: OK`.
**T1c census** (`-count=1 -v`): **`rc=1`, `=== RUN` 204, `--- PASS` 199, anchored FAIL 8** (5 test lines +
3 summary), and **199 + 5 = 204 reconciles the FULL denominator** — every non-roster test in the package is
green, so the RED is exclusively this roster's. PASS: `P_authority_only` (positive control — the roster is
**not** vacuously red) and `TestAuthorityNormalization_DuplicateAuthorityUnchanged` (the D-90-DUP pin).
FAIL: `A_both` (2 properties), `B_host_only` (5), `C_empty_authority` (2), `E_dup_host_first_wins` (5).

⚠️ **THREE PLAN §5.5 DISCREPANCIES, REPORTED RATHER THAN SMOOTHED.** (1) `E_dup_host_first_wins` was
**undescribed at the PLAN**; it fails on FIVE properties, identically shaped to `B_host_only` — no promotion
happens at all, so *"first wins"* is never even reached, and both host values survive on the carrier AND in
the decode map (`[first.example second.example]`). (2) **§5.5's transcript does not match the §5.3 code it
claims produced it** (`host on carrier = true, want false` vs the table's `carrier carries host %v, want
none`); the §5.3 code is authoritative and is what landed. (3) §5.5 under-reports `C_empty_authority` at
**one** failing property; it actually fails on **two** (carrier AND decode map) — §5.5 omits its
`req.Header` line.

### T2 + T3 — production, both symbols (commit `c00d0420`)

**T2 `buildH2Request`:** ADDS a `case "host":` arm — the pre-image grep read **exactly `:503` and `:507`,
both writes of `authority`**, so there was no `host` branch and no `host` identifier in the file to modify.
The arm latches the FIRST occurrence and suppresses EVERY occurrence from the carrier; `authoritySeen`
records `:authority` **presence**; the promotion runs after the loop under `!authoritySeen` only.
**T3 `buildRequest`:** `continue` before `regular.Add` for `host`, latching the first value. ⚠️ **The drop
MUST precede the `Add`** — `http.Header.Add` routes through `textproto` and canonicalizes the key to
`"Host"`, so a later `delete("host")` would miss it. `authoritySeen` is set at the `:authority` case and the
effective authority is resolved after the loop, so **both** P4 fields (`u.Host` `:551`, `Host:` `:555`)
carry it.
**D-90-DUP HELD:** `authoritySeen` records **presence only**; no duplicate reject was added alongside its
three sibling `…Seen` booleans, and the stability pin still passes.

⚠️ **`hostSeen`'s NECESSITY IS NARROWER THAN PLAN §9.1 STATES.** Measured here: it is required for
**first-occurrence-wins** — `hostField != ""` cannot distinguish *"not yet latched"* from *"latched to an
empty first value"*, so `host: ""` then `host: x` would wrongly promote `x` — but it is **NOT** required for
the promote condition, where `!authoritySeen` alone suffices (an absent `host` leaves `hostField == ""` and
the authority is already `""`).

### T4 — GREEN census (folded into `c00d0420`)

h2 package **`rc=0`, `=== RUN` 204, `--- PASS` 204, anchored FAIL 0** — T1's 199 + 5 = 204 = 204, so no arm
was added, removed or silently skipped. `./internal/...` `rc=0`, **ok=70**, anchored FAIL 0. `gofmt -l`
**output** empty. Symbol assertions, qualified and pathspec-scoped: `authoritySeen` **8**, `hostField`
**7**, `hostSeen` **10**. ⚠️ **`golangci-lint` first flagged TWO `behaviour` misspellings under locale US**
— one in this change and one **pre-existing in T1's committed `authority_norm_test.go:101`**, i.e. T1 ran a
lint gate that did not gate on OUTPUT. Both corrected here.

⚠️ **PLAN §10.3's STATED MECHANISM IS REFUTED.** §10.3 said the slice-only-writer gate would still read 6
because *"this row MODIFIES writer #1"*. In the landed diff **writer #1 is byte-for-byte unmodified** — the
new `case "host":` diverts before `default:` ever runs. **The gate reads 6 for a different reason than
predicted**, and a reviewer diffing that line would find nothing and could wrongly conclude the carrier
suppression was never implemented.

### T5 + T6 — the `0004` fixture arms (commit `3076010c`)

**T5 `backends/main.go` (+7):** emit `x-observed-authority: <r.Host>` **AFTER** the sorted reflected block,
never folded into `names` — a lexical sort would relocate it and re-baseline every phase-89 arm.
`git grep 'r\.Host'` read `rc=1` before the edit.
**T6 `driver/driver.go`:** four phase-90 arms (P/A/B/E) driven by a raw `http2.Framer`, the `0119`
instrument shape. **ONE FRESH TLS(ALPN h2) CONNECTION PER ARM, each with its OWN per-connection
`hpack.Decoder`** (`reference_hpack_decoder_must_be_per_connection`); ALPN asserted BEFORE the preface.
**NOT fail-fast** — every failure is recorded IN the transcript, all arms ALWAYS run, and the runner's
cross-side byte compare IS the assertion (`reference_failfast_driver_masks_later_red_arms`). Every arm sends
a **fixed literal** authority, never the dial address. `p90ObservedValue` keeps ABSENT (`<absent>`)
distinguishable from PRESENT-AND-EMPTY (`""`), which is load-bearing for arm B.
**NO YAML EDIT:** `/api/v1/reflect-headers/p90*` falls through to `- match: { prefix: "/api" }` exactly as
`a5` does; `counts[idx]++` stays the sole occurrence inside the `/api/v1/<n>` loop, so `AssertDistribution`'s
`[3,3,3]` is untouched.

⚠️ **AN OFFLINE MEASUREMENT REFUTED ARM E's STATED DISCRIMINATOR BEFORE THE DIFFERENTIAL EVER RAN.** Raw
framer straight at the fixture backend's handler over TLS+h2, no proxy: x/net's H/2 server leaves a regular
`host` field in `r.Header` (PLAN §4 / refutation 6 **CONFIRMED**), and given TWO `host` fields and no
`:authority` it sets `r.Host` to the **FIRST** one. ⇒ `x-observed-authority: first.example` is by itself
**NON-DISCRIMINATING** for the proxy's first-occurrence latch — the backend latches first-wins on its own.

### T7 — break protocol, and the REMOVAL of arm P90-E (commit `94fa0ccd`)

**FOUR break arms at FOUR DISTINCT injection sites, and HALF OF THEM ARE DIFFERENTIALLY SILENT.** B1 (delete
the `case "host":` suppression) reddened **P90-A at byte offset 179** on the `host=` boolean. B2 (drop the
`!authoritySeen` guard) reddened **P90-A at offset 164** on the `auth=` **value** — a different property,
different bytes, fully discriminated from B1. **B3** (delete the decode-map host-drop) and **B4**
(first-wins → last-wins) are **BOTH differentially SILENT** and reddened the **unit** roster only — B3 on
property P3, B4 on exactly one arm. ⚠️ Recorded as an **OBSERVED RESULT, not a prediction**: a break that
reddens nothing anywhere is indistinguishable from one that was never applied, so each was asserted to have
landed by **grepping the patched file**, not by observing a build.

⚠️ **PLAN §7.5's B2 PREDICTION IS REFUTED, AND THE INSTRUCTED BREAK DOES NOT COMPILE.** §7.5 predicted B2
would redden **P90-P**. It cannot: P90-P sends `:authority` only and **no** `host`, so `hostSeen` is false
and removing `!authoritySeen` cannot change its outcome **under any input**; B2 reddens P90-A on the
authority-**value** axis instead. And deleting `!authoritySeen` as literally instructed leaves
`declared and not used: authoritySeen` at two sites — a `reference_plan_break_instructions_dont_compile`
firing, and a **dangerous** one: the build failure reddens BOTH the unit and the differential run with
`RUN=0`, **which reads exactly like a successful break**. A compiling equivalent was substituted.

⚠️ **B1's DIFFERENTIAL EVIDENCE IS HALF-OBSERVABLE.** The cross-side comparator reports the **first**
divergence and stops, so B1 could only be shown to redden P90-A; **P90-B's divergence under B1 is MASKED.**
The unit roster (`B_host_only` red) is what carries that half. A first-divergence-only comparator cannot
enumerate the arms a break reddens.

⚠️ **AND THE ROW DISCOVERED A NEW REFERENCE DIVERGENCE THAT KILLED ONE OF ITS OWN ARMS.** P90-E was **RED at
the UNBROKEN tip**, deterministically, first divergence at transcript offset **231** — the first byte after
the literal `p90-E:` — with everything before it cross-side identical, so P90-P/A/B and the whole
pre-existing `0004` transcript already passed post-fix and **E alone diverged, on the REFERENCE side**.
MEASURED with a standalone raw-framer probe against `envoyproxy/envoy:contrib-v1.37.2`: a **SECOND** regular
`host` field is rejected at the codec layer — `Invalid HTTP header field was received: frame type: 1,
stream: 1, name: [host]`, details `http2.invalid.header.field`, reset reason *connection termination*.
**NO GOAWAY and NO RST_STREAM reach the client** (confirmed on a second pass that completed the SETTINGS
handshake first so a GOAWAY could not be missed); the connection is closed and the arm reads a bare EOF.
**The refusal is by ARITY, NOT VALUE** — two *identical* `host` values are refused identically — and holds
with `:authority` also present. Stats moved: `http2.rx_messaging_error`, `downstream_cx_protocol_error`,
`downstream_rq_rx_reset`. ⇒ **this is the MIRROR of H1-D: on H/1 the SUBJECT is the stricter side, on H/2
the REFERENCE is.** Testing first-occurrence-wins REQUIRES two `host` fields, so **the axis is NOT
DIFFERENTIABLE IN PRINCIPLE** — arm P90-E was **removed**, first-wins stays pinned at the **unit** layer by
`TestAuthorityNormalization/E_dup_host_first_wins` (shown by counterfactual to be the **sole** first-wins
discriminator in the tree, deliberately untouched), and the reject is banked as deferred follow-on **(vii)**
rather than implemented — it is reference-side admission control, out of a promote/suppress charter.
Removed with it: the now-unused `p90FirstHost` / `p90SecondHost` literals — ⚠️ **Go does NOT flag unused
constants, so their removal was verified by grep, not by the compiler.** The differential roster is **three**
arms and the `0004` workload is **42** requests/side, **re-derived not guessed**: 27 + 2 (phase-87) +
2 (phase-88) + 8 (phase-89) + 3 (phase-90), updated on `drive()`, `DriveReference()` and `DriveSubject()`.

### T8 — docs

ADR-0312 completed **in place**: §Decision (D-90-SCOPE, D-90-DUP, D-90-SKIP, D-90-TARGET, D-90-YAML) +
§Consequences (i)–(xix), no renumber, no new `---`. Strict guard `^> **STATUS: PROPOSED` **1 -> 0**;
`^---$` still **216**; `^## ADR-` **311**; bare `^## ` **319**; next-free still **ADR-0313**.

⚠️ **THE ADR'S OWN APPEND POINT DID NOT EXIST.** ADR-0312's STATUS line **and** PLAN §8.3 both directed the
IMPL to append *"after the RETAINED italic footer"*. **ADR-0312 had no such footer** — its block ended at
§Context ¶7 — while **four sibling ADRs** carry the exact form `*§Decision and §Consequences follow at the
phase-NN IMPL.*`; the phase-90 SPEC omitted it. **The IMPL added the missing footer and then appended after
it**, so the ADR is now structurally identical to its predecessors and the STATUS line's instruction is true.

⚠️ **A GATE THAT READS THE RIGHT NUMBER CAN BE POINTED AT THE WRONG LINE.** Two plausible strict `PROPOSED`
guard forms **both read exactly `1`** on the pre-IMPL tree: the canonical `^> **STATUS: PROPOSED`
(ADR-0312, `:18281`) and `^**Status:** PROPOSED` (a historical **ADR-0231** line, `:14866`). ⇒ **the 1 -> 0
disarm must be verified by LINE AND ADR, never by the count alone** — the decoy would have read a successful
disarm as a failure, and a change to that historical line would read as a successful disarm that never
happened.

⚠️ **PLAN §8.3 SAID "FIVE DEFERRED FOLLOW-ONS"; §11/§13 SAY SIX; THE IMPL REGISTERED SEVEN** — the seventh
being the duplicate regular-`host` reject discovered by this row's own T7.

`README.md` drift **this row introduced** was fixed: the workload figure **39 -> 42** in both the purpose
line and the request-schedule heading (`driver.go` already carried 42 in three doc comments), the three
phase-90 arms documented, and both scope limits (arm C deferred, arm E not-differentiable-in-principle)
written out. ⚠️ **`:27`'s *"pre-existing 31 round-trips are byte-untouched"* is still TRUE and was left
alone.**

### T9 — gates against the FINAL tree

**(a)/(b)** `go test ./...` **`rc=0`**, set reconciliation **exact at 236 packages** (127 ok + 109
no-test-files), zero difference in both directions. Full differential suite **`rc=0`, 121 `=== RUN` /
121 `--- PASS` / 0 SKIP**, fixture set reconciled against `ls -d test/fixtures/*/` with zero difference both
directions, and **`--- PASS: TestDifferential/0004-h2-routing` asserted EXPLICITLY** — `runner_test.go:200`
`t.Skipf`s an unregistered fixture and **no fixture-count gate exists anywhere in the tree**.
**(c)** h2spec is **STRUCTURALLY INCAPABLE of anchoring this row** — its harness configures
`envoy.filters.http.router` ALONE with `direct_response` routes and never goes upstream, so no authority is
ever forwarded; **no h2spec figure is cited**. grpc-conformance deferred in writing; proxy-wasm 10/16.
**(d)** fuzzers **55 / 48 files**, **+0**. **(e)** the **ANCHORED** panic gate `^panic:|DATA RACE|SIGSEGV`
read **0** on every differential launch. **(f)** no `REVIEW.md` — the standing departure, **NAMED**.

⚠️ **TWO GATE COMMANDS WERE THEMSELVES BROKEN DURING THIS ROW, AND BOTH FAILED SILENT-CLEAN.**
`git grep -o -- -e 'NewCounter('` made git read `-e` as a revision and die outright; and `rc=$?` taken after
`… | cat` returned **cat's** status `0` where the real status was `1`.
`reference_gate_command_negative_control` applies to the gates a stage writes **for itself**.

⚠️ **PLAN §10.2's FLAKE-CAUSATION EXCLUSION WAS UNEARNED FOR HALF ITS SCOPE.** The PLAN excluded both named
SDS flakes with *"that test binary does not link the patched package at all"*. Re-run here:
`go list -deps -test ./internal/xds/ | grep -c 'filter/hcm/h2'` ⇒ **0**, but **`./internal/boot/` ⇒ 1 — the
`boot` flake's binary DOES link the patched package.** The claim was true of `xds` **only**. It was replaced
with a direct exclusion: `panic(...)` injected as the first statement of **both** patched functions leaves
`go test ./internal/boot/` at `rc=0` with **zero panic hits**, so neither function is executed by that
binary, with a positive control proving the injection was live. **An exclusion that covers only half its
named scope is worse than none, because it is quoted as though it covered both.**

⚠️ **A STANDING FINDING, NOT A FLAKE: `TestServerConn_TinyWindowDelivery` RECURRED.** Phase-80
`PLAN.md:636` records it **FIXED** at `f46ba419`/`f2dd994a` and states that *"a recurrence is now a FINDING,
not a re-run"*. It already recurred once at phase 85 (`STATE_HISTORY.md:486`). **This is the SECOND post-fix
recurrence.** It cleared 5/5 scoped-isolated and 3/3 full-package, and is causally excluded from row 90
(`go list -deps -test ./internal/filter/hcm/h2/ | grep -c 'fixtures/0004'` ⇒ 0, and `stream.go`
byte-identical to the pre-break snapshot) — but it is **flagged as a finding, not absorbed as a flake**.

### Cost, re-measured at THIS IMPL's own publishing commit

Production **+48 / −0**, ONE file, ONE package. Unit **+122 / −0** · differential driver **+354 / −4** ·
fixture backend **+7 / −0**. The BRAINSTORM floor was `+15/−1`, the SPEC prototype `+34/−0`, the PLAN
prototype `+36/−0`. ⚠️ **`reference_measured_prototype_is_a_lower_bound` FIRES A FOURTH CONSECUTIVE TIME,
and the cause is again UNDER-ENUMERATION, not estimation error.**

### Sentinel — ACTUAL output, AFTER the row-90 flip

(1) **SILENT** (row 90 now `done`) · (2) **SIX** at `:200 :206 :212 :222 :228 :236` · (3) **SILENT**.
⇒ **ONE check blocks it; `stop` was NOT created.** NCs: **NC-A** returned to the ONE-line
`NOT DONE: row 62` form (row 90's line is gone, exactly as the flip predicts) · **NC-B** `want=121` ⇒
`GATE FAIL: examined 122 data rows, expected 121`.

### Probe hygiene

All work in the dedicated worktree `wt-phase-90-impl`; the canonical root was never written to. Every break
arm reverted with `sha256sum -c` before the next task started, and every probe file deleted. Reference
containers were torn down **BY NAME** per `reference_parallel_agents_shared_machine_namespaces`. No agent
pushed; the controller squashes.
