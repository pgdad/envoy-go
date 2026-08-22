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
