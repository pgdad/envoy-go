# Phase 97 — `quic-chain-selection-order` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal.** Make the QUIC/HTTP-3 serving path select its filter chain with the same mandated algorithm the
TCP path uses — `listenerfilter.SelectChain` over the ORDERED `rt.chainSpecs` with `rt.defaultSpec` as the
fallback — so that an eligible indexed chain stops being silently bypassed, an INELIGIBLE one stops being
served anyway, and the chain that supplies the FILTERS is the chain that supplied the TLS identity. Make
the repair falsifiable by unit arms on BOTH sides of every eligibility conditional and by a new cross-side
differential fixture.

**Architecture.** `internal/listener/quic.go` carries two chain accessors. `quicTLSConfig()` runs at Start
(before any connection exists) and again per connection; `quicChain()` runs per connection. Neither calls
`SelectChain`. Both reach around it into `rt.chainByName`, an UNORDERED map that CONTAINS the default slot,
and they use two different predicates — `quicChain` prefers `rt.defaultChain` whenever it is non-nil at all,
`quicTLSConfig` prefers it only when its `tlsCfg` is also non-nil. The consequence measured at the SPEC is
not an inverted preference but TOTAL NON-EVALUATION: envoy-go answers *"the default chain"* on every QUIC
arm and evaluates no `filter_chain_match` dimension at all. The repair stamps the two QUIC-constant inputs
(`transport_protocol: "quic"`, `application_protocols: ["h3"]`), reads destination and source from the
connection, reads `ServerName` out of `conn.ConnectionState().TLS`, and routes both accessors through one
selector — keeping them SEPARATE, because they answer at two different moments.

**Tech Stack.** Go; `internal/listener` and `internal/listener/listenerfilter`; the differential harness
under `test/differential` + `test/fixtures`; the pinned reference `envoyproxy/envoy:contrib-v1.37.2`.

**Spec.** `docs/envoy-go/phases/97-quic-chain-selection-order/SPEC.md` (874 lines). Its §14 is the
eight-item list this PLAN discharges; §2 is the twelve-arm reference measurement this PLAN carries forward
INTACT and does not re-derive; §4 is the DECIDED and MEASURED repair; §11 is the pinned edit map. The PLAN
argues FROM the SPEC and the SPEC travels with it — executors read both.

## Global Constraints

- **Reference pin:** `envoyproxy/envoy:contrib-v1.37.2`, digest `7edd5b0fd763…`, verified against
  `docs/envoy-go/ENVOY_TARGET.md:3-4` before any arm is trusted. ⚠️ `ENVOY_TARGET.md` is under
  `docs/envoy-go/`, NOT the repo root.
- **envoy-go boots with `-c`, NOT `--config-path`.** Its validate flag is `-mode validate`; the
  reference's is `--mode validate`. ⚠️ `timeout` rc=124 is shared by a HEALTHY server and a HUNG boot —
  read the OUTPUT, never the code.
- ⚠️ **An omitted `clusters:` key BOOT-REJECTS envoy-go.** Every probe and fixture config carries a
  placeholder STATIC cluster the route never references. `BackendCount()` must be >= 1 — the runner
  rejects 0.
- ⚠️ **The reference container cannot read `filename:` cert paths** — deliver PEMs as `inline_string:`,
  indented ONE level deeper than the `inline_string:` key, and keep PEM substitutions OUT of YAML
  comments. ⚠️ **For a UDP/QUIC listener publish UDP (`-p <port>:<port>/udp`) AND the admin TCP port;
  `--network host` is what does NOT work.**
- ⚠️ **Choose NON-1xx sentinel statuses in probes.** A `direct_response` status of `111` came back as
  `200` on the H3 path while `222` came back as `222`. This row uses **`222`** everywhere.
- ⚠️ **`-count=1` is not optional** on any differential run; the suite's failure mode is a SILENT PASS.
  The full suite takes ~400s. ⚠️ **`-race` on the differential suite is VACUOUS** — the subject there is
  an unraced subprocess; run `-race` on the FULL `internal/listener` package instead.
- ⚠️ **`gofmt -l` never exits non-zero — gate on OUTPUT.** `golangci-lint`'s misspell runs in locale US:
  sweep British spellings out of `.go` comments before the gate; Markdown prose may use them freely.
- ⚠️ **`go test` without `-v` prints zero `=== RUN`** — `RUN=0` beside `RC=0` is a vacuous green. A
  `-run` selector matching nothing prints `[no tests to run]` and EXITS 0; a selector naming a package
  that does not exist prints `FAIL … [setup failed]` and EXITS 1. Confirm every selector resolves with
  `go list ./path/...` before believing a FAIL.
- ⚠️ **`grep -c` counts LINES, not occurrences, and on zero matches prints `0` AND exits 1** — capture
  with `v=$(cmd || true)`, never `$(cmd || echo 0)`. Pass `--` before any pattern starting with `-`.
- ⚠️ **`grep` IS A SHELL FUNCTION WRAPPING `ugrep` IN *BOTH* THE CONTROLLER SHELL *AND* THE SUBAGENT
  SHELL** — see §0.7, which refutes the router's method note 3j. ugrep honours `.gitignore`, and
  `.gitignore` lists `next-prompt.txt`, so a recursive bare `grep` is BLIND to it in EVERY shell in this
  environment. Use `/usr/bin/grep`, `command grep -r`, `git grep`, or a direct path. ⚠️ `command grep`
  does NOT survive `xargs`. ⚠️ `git check-ignore` reassures you wrongly — git exempts TRACKED files.
- ⚠️ **There is NO `recover()` in any non-test file under `internal/listener` (including
  `listenerfilter/**`) or `internal/tls`** — MEASURED at this PLAN, matcher
  `grep -rn 'recover()' --include='*.go' internal/listener internal/tls | grep -v '_test\.go'`, zero
  output. A panic on the accept goroutine ABORTS THE TEST BINARY; it does not fail a test. Gate every
  panic check on the anchored form `^panic:|DATA RACE|SIGSEGV`, and PROVE that gate live before believing
  a zero. ⚠️ **A fail-fast reachability control measures ONE site per run** — isolate.
- **Port band for ad-hoc probes: `15000-19000` minus `18080-19000`** (a sibling `curl-world` session holds
  that range) and minus the 28 fixture `15xxx` literals of §0.6. The differential reserves `20000..31007`
  and `11000..14999`; `net.ipv4.ip_local_port_range` starts at 32768. Check with `ss -tan` AND `ss -uan`
  (ALL states), never `ss -ltn`. ⚠️ **For a unit table, use port 0.**
- **Every task ends with a commit.** Subagents commit LOCALLY on their own stage branch with EXPLICIT
  PATHSPECS; the controller merges and squashes. ⚠️ **Subagents do not push.** ⚠️ Use
  `git -C <abs-worktree-path>` for every git command — the Bash tool's cwd silently resets to the repo
  root, and commits then land on `master`. ⚠️ **`go build ./cmd/envoy-go/` drops an untracked binary in
  the worktree root** — build with `-o` into scratch.
- ⚠️ **Docker is serialized to exactly ONE agent at a time.** Never tear down a container this session did
  not create, and tear down BY NAME. A `reaper_*` testcontainers Ryuk container is created by the
  differential itself and is REUSED — leave it alone.

---

## 0. What this PLAN refuted, by execution — SIXTEEN claims

Method note 2: every stage's job is to refute its predecessor by execution. The phase-97 SPEC refuted
ELEVEN, **two of them by its own measurement agents**, and told this stage (§14 item 5) to do the same to
it. Every figure below was produced by running its command at this PLAN's own tip. ⚠️ **THREE of the
sixteen were produced by THIS stage's own agents, and one of those three corrected its own summary line
against its own table mid-report** — the third consecutive row on which a stage's own review seam is the
source. **Expect to be refuted by your own agents, not only by your successor.**

### 🔴 0.1 — `SPEC.md` §5's DIAGNOSIS OF THE `[]struct{` ZERO IS WRONG, AND §14 ITEM 4's PRESCRIBED MATCHER REPRODUCES THE FALSE ZERO

`SPEC.md` §5 states that `manager_test.go` has *"**zero** single-line `[]struct{` matches, because its
tables are written with the brace on the following line."* §14 item 4 then instructs this PLAN to
*"re-derive with a matcher that sees a brace on the following line."*

**The stated reason is FALSE, and following the instruction would have reproduced the same zero.**
Measured on `internal/listener/manager_test.go` at this tip:

| matcher | reads |
|---|---|
| `grep -cE '\[\]struct\{'` (no space) | **0** |
| `grep -cE '\[\]struct[ ]*$'` (brace on the FOLLOWING line — the SPEC's prescription) | **0** |
| `grep -cE '\[\]struct[ ]*\{'` (space allowed) | **6** |
| `grep -c '\[\]struct'` (bare) | **6** |

The six tables sit at `:1092 :1484 :4777 :5444 :5691 :6102`, each with its `for _, tc := range` loop at
`:1109 :1559 :4792 :5456 :5752 :6157`. **gofmt writes `[]struct {` with a SPACE**; the brace is on the
same line in all six. ⇒ **the only matcher correct by construction is `\[\]struct[ ]*\{` or the bare
`\[\]struct`.** ⚠️ **This also refutes the router's standing method note 44**, which carries the same
false reason. **A matcher's vocabulary can be wrong in a direction its author did not predict** — the
SPEC diagnosed the right symptom and named the wrong cause, and the prescription inherited the error.

### 🔴 0.2 — `SPEC.md` §5's "THE BOTH-SLOTS SHAPE HAS NEVER BEEN CONSTRUCTED IN THIS PACKAGE" IS FALSE AS WRITTEN

**SEVEN** sites in `internal/listener` construct a listener carrying BOTH a non-empty `FilterChains` and a
non-nil `DefaultFilterChain`. **All seven are TCP.** Enumerated by `grep -rn 'DefaultFilterChain'
internal/listener/` and read one by one:

| # | chains / default at | enclosing symbol |
|---|---|---|
| 1 | `integration_test.go:228` / `:240` | `mkChainsListener` (`:218`) — two matched chains + default |
| 2 | `manager_test.go:1438` / `:1441` | `TestParseDefaultFilterChainNoLongerErrors` |
| 3 | `manager_test.go:2625` / `:2634` | subtest `"default_chain_tls_ineligible_plaintext_chain"` in `TestListenerMetrics_GateMatchesInc` |
| 4 | `manager_test.go:2995` / `:2998` | `TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain` |
| 5 | `manager_test.go:3151` / `:3154` | `TestParseDefaultFilterChainBuildErrorIsSinglePrefixed` |
| 6 | `manager_test.go:3499` / `:3505` | `TestUnifiedDispatchDefaultFilterChainFallback` — DIALS, asserts tag byte `'D'` |
| 7 | `manager_test.go:6411` / `:6420` | `TestServeConnection_DefaultFilterChainTLS_ShapeB_IneligibleFilterChain` — DIALS |

**The defensible statement is QUIC-SCOPED:** the both-slots shape ON A QUIC LISTENER has never been
constructed here. The three UDP/QUIC listener literals in the package (`manager_test.go:850`, `:897`,
`:991`) do not intersect the seven. ⚠️ **AND NO QUIC LISTENER ANYWHERE IN THE PACKAGE CARRIES A
`filter_chain_match`** — `sed -n '834,904p;979,999p' manager_test.go | grep -n 'FilterChainMatch'`
returns nothing, while the package-wide count is **53**, all TCP. **That is the real coverage statement,
and it is stronger than the one the SPEC wrote.**

### 🔴 0.3 — `SPEC.md` §11's PLAN SCOPE LINE IS UNDER-ENUMERATED, FOR THE **SECOND CONSECUTIVE ROW**

§11 states *"The PLAN lands `PLAN.md` only."* Measured with `git show --numstat` across **FIVE**
precedent PLAN commits, located by the anchored `^phase NN (` subject form and cross-checked against the
loose form:

| precedent | files touched (add/del) |
|---|---|
| `e69f1c58` (92 PLAN) | `STATE.md` 9/9 · `STATE_HISTORY.md` **2/0** · `phases/92-…/PLAN.md` 1482/0 · `next-prompt.txt` 88/96 |
| `90010c4c` (93 PLAN) | `STATE.md` 9/8 · `STATE_HISTORY.md` **2/0** · `phases/93-…/PLAN.md` 1031/0 · `next-prompt.txt` 76/65 |
| `db539e7d` (94 PLAN) | `STATE.md` 9/9 · `STATE_HISTORY.md` **2/0** · `phases/94-…/PLAN.md` 1662/0 · `next-prompt.txt` 93/106 |
| `f647dd72` (95 PLAN) | `STATE.md` 10/10 · `STATE_HISTORY.md` **2/0** · `phases/95-…/PLAN.md` 865/0 · `next-prompt.txt` 88/78 |
| `0eab42e3` (96 PLAN) | `STATE.md` 10/10 · `STATE_HISTORY.md` **2/0** · `phases/96-…/PLAN.md` 1666/0 · `next-prompt.txt` 49/43 |

**FOUR files, identical in all five**, `STATE_HISTORY.md` at exactly **+2 / -0** every time.

⚠️ **THE PHASE-96 PLAN ALREADY REFUTED THIS EXACT LINE, AT ITS OWN §0.2, AND THE PHASE-97 SPEC SHIPPED IT
AGAIN VERBATIM.** A refutation recorded in a sibling phase directory does not reach the next SPEC's
author. **§11's four NEGATIVES remain CORRECT and are what the line exists to enforce** — no `.go`, no
`ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no `DECISIONS.md`; zero of the five precedents touches any of
them. The refutation is of the word *only*. ⇒ **`ADR-0319` STAYS `PROPOSED`, the house guard STAYS
ARMED, the tail STAYS `ADR-0319`, next-free STAYS `ADR-0320`, and §11's IMPL edit map STAYS PINNED.**

### 🔴 0.4 — `SPEC.md` §11's "`BEHAVIOR_CONTRACT.md` IS NOT ON THE IMPL ROSTER EITHER" IS REFUTED BY THE PRECEDENT IT POINTS AT

§11 says the row lands **+0 stat names** — TRUE, and this PLAN does not disturb it — and then leaves open
*"whether a `+0, UNCHANGED` ledger entry is owed is a question for the IMPL against the phase-96
precedent."* **MEASURED, and the precedent answers it: yes.** Matcher
`grep -noE '^\*\*Phase [0-9.]+ [-—][^:]{0,60}' docs/envoy-go/BEHAVIOR_CONTRACT.md`:

| ledger line | entry |
|---|---|
| `:5107` | `**Phase 44.2 — 1189 → 1189 (+0, UNCHANGED)` |
| `:5109` | `**Phase 44.3 — 1189 → 1189 (+0, UNCHANGED)` |
| `:5113` | `**Phase 45.2 — 1191 → 1191 (+0, UNCHANGED)` |
| `:5119` | `**Phase 47.1 — 1200 → 1200 (+0, UNCHANGED)` |
| `:5121` | `**Phase 51 — 1200 → 1200 (+0, UNCHANGED)**` |
| `:5141` | `**Phase 96 — +0, UNCHANGED (no new stat NAME; the row changes WHICH listener shapes register the existing five)` |

**SIX `+0` chain entries exist**, and the phase-96 entry states the rule in its own text: *"A `+0` row
DOES earn a chain entry — phases 44.2, 44.3, 45.2, 47.1 and 51 each carry one."* ⇒ **the phase-97 IMPL
OWES a `Phase 97 — +0, UNCHANGED` chain entry**, and it must follow the **phase-96 form and not the
sibling `A → B` form**: phase 96 deliberately quotes **NO absolute**, because three mutually inconsistent
stat-surface absolutes are live in this tree at one tip and a contested count earns **no number**
(§Task 21). ⚠️ **`BEHAVIOR_CONTRACT.md` IS THEREFORE ON THE IMPL ROSTER, AS ROW 11** — and §0.5 puts it
there for a second, independent reason.

### 🔴 0.5 — THE BYTE-UNTOUCHED ROSTER AND THE PROSE-RECONCILIATION SET **INTERSECT AT `internal/listener/manager.go`** — THE PHASE-96 §0.13 CLASS, ONE ROW LATER

The repair deletes both `for _, ci := range rt.chainByName` loops (§4.1 point 5) and replaces
`quicTLSConfig`'s precedence with a Start-time `SelectChain`. **A present-tense sentence asserting the
OLD precedence is carried at FIVE live sites.** Occurrence set from
`/usr/bin/grep -rni --exclude-dir=.git 'quicTLSConfig' .`, each read in full and adjudicated:

| # | site | text, in one clause | falsified by this row? | on the SPEC's roster? |
|---|---|---|---|---|
| 1 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:1973` | *"`quicTLSConfig()` (`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` FIRST, before consulting `chainByName`"* | **YES**, on BOTH halves — the precedence AND the existence of the map walk; and the `:56-58` anchor drifts | **NO** — §11 excludes the file |
| 2 | `internal/listener/manager.go:396` | the SAME sentence, in `registerListenerMetrics`'s doc | **YES**, both halves | **NO** — §11 puts the file on the BYTE-UNTOUCHED roster |
| 3 | `internal/listener/quic_test.go:237` | *"`quicTLSConfig()` (`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` BEFORE consulting `chainByName`"* | **YES**, both halves | YES — §11 rows 4 and 5 |
| 4 | `internal/listener/manager_test.go:977` | `mkQUICListenerDefaultChain`'s doc: *"`quicTLSConfig` returns `rt.defaultChain.tlsCfg` FIRST — so whatever this chain's transport_socket builds is what reaches `quic.Listen`"* | **REASON only.** With zero `filter_chains[]` the Start-time `SelectChain` over an empty `chainSpecs` returns `defaultSpec`, so the CONCLUSION survives | YES — §11 row 3 |
| 5 | `docs/envoy-go/DECISIONS.md:19020` | ADR-0319 §Context: *"`quicTLSConfig()` returns `rt.defaultChain.tlsCfg` before consulting `chainByName`"* | **TENSE only** — correct as a statement of the defect, becomes past tense once §Decision lands | YES — §11 row 8 |

**TWO NON-CARRIERS, checked and deliberately left:** `internal/tls/config.go:620` and
`internal/listener/manager.go:740` both narrate the *pre-phase-95* hole in the PAST tense
(*"reached THIS function and `quicTLSConfig()` then handed the result to `quic.Listen`"*). Neither
asserts the precedence. **`internal/tls/**` therefore STAYS byte-untouched.**

⇒ **`internal/listener/manager.go` CANNOT BE BOTH BYTE-UNTOUCHED AND RECONCILED.** This PLAN resolves it
by moving the file onto the edit roster under a **COMMENT-ONLY** constraint that is mechanically gated
(§Task 13), rather than by leaving a fifth stale present-tense claim in the tree. ⚠️ **The alternative —
record-and-leave, the `ROADMAP.md:229` precedent — is rejected HERE because nothing forces it: the
`:229` claim is unfixable only because it sits inside a sentinel window on a margin of one, and
`manager.go:396` sits inside no gate at all.**

### ⚠️ 0.6 — THE ROUTER'S METHOD NOTE 3j IS REFUTED: `grep` IS A `ugrep` SHELL FUNCTION IN THE **SUBAGENT** SHELL TOO

Method note 3j asserts *"In a SUBAGENT shell `grep` is `/usr/bin/grep` (GNU 3.11) and sees it fine."*
A measurement agent ran `type grep` in its own shell at this tip and reported a **shell function**
wrapping `ugrep` — `exec -a ugrep "$_cc_bin" -G --ignore-files --hidden -I --exclude-dir=.git …` — the
same wrapper the controller shell carries. **The variable is not the shell; the wrapper is installed in
both.** ⇒ **a recursive bare `grep` is blind to `next-prompt.txt` in EVERY shell in this environment**,
because ugrep honours `.gitignore` and `.gitignore:2` lists it. **Every brief this row issues must name
`/usr/bin/grep`, `command grep -r`, `git grep`, or a direct path**, and the agent that found this had
cross-checked six figures under both binaries before trusting either. ⚠️ **`command grep` does not
survive `xargs`; `git check-ignore` reassures you wrongly, because git exempts TRACKED files.**

### ⚠️ 0.7 — `buildListenerRuntime` DOES NOT EXIST AS A SYMBOL

`SPEC.md` §3.3 says *"`buildListenerRuntime` writes `chainByName[defaultSpec.Name] = defaultChain`."*
The write is REAL and verified verbatim at **`internal/listener/manager.go:779`**, inside the
`if dfc := l.GetDefaultFilterChain(); dfc != nil` block — **but the enclosing symbol is
`buildListenerRuntimeWithCtx`** (`manager.go:575`), a fifteen-parameter function. `grep -n 'func
buildListenerRuntime(' internal/listener/manager.go` reads **0**. The `TestBuildListenerRuntime_*` test
names preserve the older spelling and are what makes the mistake easy. **Anchor on
`buildListenerRuntimeWithCtx`.**

### ⚠️ 0.8 — THE ROUTER'S POST-ROLL `STATE.md` TIE SHAPE IS WRONG

`next-prompt.txt` method note 26 says *"The post-roll list carries THREE entries dated `2026-09-08` at
its head and TWO dated `2026-09-07` at its tail."* Measured, in list order, at this tip:

| position | entry | date |
|---|---|---|
| 1 (`:46`) | phase 97 BRAINSTORM done | **2026-09-09** |
| 2 (`:48`) | phase 96 IMPL done | 2026-09-08 |
| 3 (`:50`) | phase 96 PLAN done | 2026-09-08 |
| 4 (`:52`) | phase 96 SPEC done | 2026-09-07 |
| 5 (`:54`) | phase 96 BRAINSTORM done | 2026-09-07 |

Histogram: **1 × 09-09, 2 × 09-08, 2 × 09-07**. The head is ONE entry, not three. **The half the router
got right is the one that matters: the tie AT THE TAIL is TWO-WIDE**, so the date narrows the evictee
field to two and **LIST POSITION picks the tail** — `phase 96 (listener-default-chain-tlsmode)
BRAINSTORM done`. ⚠️ **A DIRECT DATE READ STILL CANNOT PICK THE EVICTEE ALONE**, exactly as method note
26 warns; only its description of the shape was wrong. **READ IT, do not inherit it** — including from
this PLAN.

### ⚠️ 0.9 — THE `:229` LINE-LENGTH FIGURE IS BYTES-INCLUDING-NEWLINE, AND THE STALE CLAIM'S OCCURRENCE SET IS **FOUR LINES, NOT ONE**

The router says the stale swallowed-panic claim *"sits at byte offset 42927 of a 47234-byte line."*
Measured four ways on `docs/envoy-go/ROADMAP.md:229`:

| measure | value |
|---|---|
| bytes INCLUDING the trailing newline (`sed -n '229p' … \| wc -c`) | **47234** |
| bytes EXCLUDING it (`… \| tr -d '\n' \| wc -c`) | **47233** |
| characters (`LC_ALL=C.UTF-8 awk 'NR==229{print length($0)}'`) | **46753** |
| `LC_ALL=C awk` (byte semantics) | **47233** |

**The router's figure reconciles only as bytes-including-newline** — method note 32 exactly: name the
measure or the figure is meaningless. ⚠️ **AND THE CLAIM IS NOT CONFINED TO `:229`.**
`/usr/bin/grep -noi 'swallowed-panic' docs/envoy-go/ROADMAP.md` reads lines **136, 139, 229**;
`/usr/bin/grep -noi 'neither crashes nor boots'` reads **136, 159, 229**. **The union is FOUR lines —
136, 139, 159, 229 — and `:159` is THIS ROW'S OWN.** Only `:229` sits inside a sentinel window. **Recorded,
not repaired by this stage** (a PLAN edits no ROADMAP row), and handed to the IMPL, which flips `:159`
and can reconcile it there without touching a window. ⚠️ **`:136` and `:139` are OUTSIDE every window
and are the cheapest fold-in this project has.**

### 🔴 0.10 — `SPEC.md` §5.1's ROSTER IMPLIES EVERY ARM IS RED AT THE UN-FIXED TIP. **FOUR OF THE TEN ARE GREEN, STRUCTURALLY**

§14 item 2 says to *"order the spine so the un-fixed tip is SPENT as the negative control."* Correct — but
the tip does not redden every arm, and a TDD spine that expects it to will read four false passes as
successes. Derived from the tip's own code (`quic.go:74-82`: `if rt.defaultChain != nil { return
rt.defaultChain }`, else the first entry of `rt.chainByName`):

| arm | shape | expected | AT THE UN-FIXED TIP | |
|---|---|---|---|---|
| a | empty-match indexed + default | indexed | **default** | 🔴 RED |
| b | `destination_port` ineligible + default | default | default | 🟢 **GREEN** |
| c | `destination_port` ineligible, NO default | nil ⇒ closed | the ineligible indexed chain | 🔴 RED |
| d | `transport_protocol: "quic"` + default | indexed | **default** | 🔴 RED |
| e | `transport_protocol: "tls"` + default | default | default | 🟢 **GREEN** |
| f | `application_protocols: ["h3"]` + default | indexed | **default** | 🔴 RED |
| g | `application_protocols: ["h2"]` + default | default | default | 🟢 **GREEN** |
| h | matching `server_names`, driven | indexed | **default** | 🔴 RED |
| i | non-matching `server_names`, driven | default | default | 🟢 **GREEN** |
| j | accessor identity, PLAINTEXT default slot | one chain | TLS from indexed, filters from default | 🔴 RED |

**RED-at-tip: a, c, d, f, h, j — SIX. GREEN-at-tip: b, e, g, i — FOUR.** ⚠️ **THE FOUR GREENS ARE NOT A
TDD FAILURE; THEY ARE `SPEC.md` §0.3's FALSE-AGREEMENT CLASS SHOWING UP INSIDE OUR OWN SUITE.** The
subject answers *"default"* on every arm, so it coincidentally matches wherever *default* is the right
answer. **Their falsifiability comes ONLY from NC roster row 2**, which is why that row is not optional
and why §Task 15 scores it per arm rather than per run. ⚠️ **A row that agrees for a reason the mechanism
cannot supply is a FALSE agreement — ask whether the subject could have answered anything else.**

### 🔴 0.11 — ARM **j** MUST USE A **PLAINTEXT** DEFAULT SLOT, OR IT IS GREEN AT THE TIP AND PINS NOTHING

`SPEC.md` §5.1 row j specifies only *"both-slots listener"*. **Measured against the tip's two predicates:
if BOTH slots carry TLS, `quicTLSConfig()` returns `rt.defaultChain.tlsCfg` and `quicChain()` returns
`rt.defaultChain` — they AGREE, and arm j is a vacuous green.** The two accessors differ by exactly the
`tlsCfg != nil` conjunct (`SPEC.md` §3.2), so only a default slot with a **nil `tlsCfg`** separates them.

**That shape BUILDS, and the reason is asymmetric and worth stating:** `manager.go:657-660` boot-rejects a
`filter_chains[i]` with no `transport_socket` on a QUIC listener (*"quic listener requires a
transport_socket (mandatory TLS)"*), while the `default_filter_chain` branch at `manager.go:727-728`
carries **no such kind check** — `if ts := dfc.GetTransportSocket(); ts != nil` simply leaves `dfcTLS`
nil. **That asymmetry IS the banked D2-QUICTS divergence** (`SPEC.md` §2.4), and arm j consumes it as a
test fixture without repairing it. ⇒ **arm j = plaintext `default_filter_chain` + QUIC-wrapped
empty-match `filter_chains[0]`**, which is `BRAINSTORM.md` §2.2's measured cross-wiring shape.

⚠️ **AND ARM j MUST USE AN *ELIGIBLE* INDEXED CHAIN.** With an INELIGIBLE one and a plaintext default,
the post-fix accessors legitimately disagree — the Start-time selection falls to the default slot, whose
`tlsCfg` is nil, so `quicTLSConfig` falls back to the first TLS-bearing chain in `chainSpecs` slice order
while `quicChain(conn)` returns the default. **That is `SPEC.md` §4.4's residual, not a defect**, and an
arm j built on it would be RED against CORRECT code.

### ⚠️ 0.12 — `ChainMatchInputs` HAS **SEVEN** FIELDS FOR **EIGHT** DIMENSIONS; `source_type` IS DERIVED, NOT STORED

`internal/listener/listenerfilter/types.go:24-58`. The struct's own doc says *"the eight chain-match
dimensions"* and then declares seven fields — `DestinationIP`, `DestinationPort`, `SourceIP`,
`SourcePort`, `ServerName`, `TransportProtocol`, `ApplicationProtocols`. **`source_type` is computed from
`SourceIP` by `(*ChainMatchInputs).IsLoopbackSource()` (`types.go:56-58`) and consumed by `matches` at
`chainmatch.go:137-142` against `ChainSpec.SourceTypeLocal` / `.SourceTypeExternal`.** `SPEC.md` §3.5's
table is field-complete and never says so; §5.3 then defers `source_type` alongside
`source_prefix_ranges` and `source_ports` as though all three were stored inputs. **The consequence is
favourable and should be stated rather than discovered: filling `SourceIP` from `conn.RemoteAddr()`
activates `source_type` for free**, which is why §5.3's deferral is about MEASUREMENT, not about
mechanism.

### ⚠️ 0.13 — `catchAllCount` IS A LOCAL VARIABLE, NOT A CHECK

`SPEC.md` §3.4 names `catchAllCount`, `findIdenticalChainSpecs` and `validateQUICOptions` as three things
that fail to enforce a QUIC chain count. **The conclusion is CONFIRMED three ways** (§Task 0 anchors), but
one of the three is not a symbol you can call: `catchAllCount` is an `int` declared at
`manager.go:607`, incremented at `:685-687`, and checked at `:715-717`. **`validateQUICOptions`
(`manager.go:532`) takes `(name string, q *listenerv3.QuicProtocolOptions)` and never sees the chain
list — that is a SIGNATURE-LEVEL proof, stronger than a reading.** `findIdenticalChainSpecs`
(`manager.go:1064`) rejects duplicate SHAPES, not multiplicity. ⚠️ **AND `catchAllCount`'s error message
names `server_names` while its predicate is `spec.Empty`, all eight dimensions** — a message/predicate
mismatch, recorded, not repaired.

### ⚠️ 0.14 — ON QUIC, `ApplicationProtocols` IS A LISTENER **CONSTANT**, NOT THE CLIENT'S OFFER LIST — THE AGREEMENT WITH THE REFERENCE IS REAL AND THE MECHANISM DIFFERS

On TCP, `ChainMatchInputs.ApplicationProtocols` is the client's ALPN **offer list**, extracted from the
ClientHello by `tls_inspector`. On QUIC there is no ClientHello to peek: `quic.Conn.ConnectionState().TLS`
is a `crypto/tls.ConnectionState` whose `NegotiatedProtocol` is a **scalar**, the single negotiated value,
not the offer list. §4.1 point 2 therefore stamps the **constant** `[]string{"h3"}`.

**That is sound at this listener kind and the reason is structural:** the QUIC config's `NextProtos` is
`["h3"]`, so negotiation can only yield `"h3"` or fail the handshake outright — a connection that reaches
`serveQUICConnection` negotiated `h3` by construction. It agrees with the reference on `SPEC.md` §2 arms
G and G2. ⚠️ **BUT THE MECHANISM IS NOT THE TCP ONE, AND A FUTURE ROW THAT ADDS A SECOND QUIC ALPN WOULD
BREAK THE CONSTANT SILENTLY.** Stated here so it is not re-derived, and banked at §10.

### ⚠️ 0.15 — `quicChain()` HAS **ZERO** TEST CALL SITES

`grep -rn 'quicTLSConfig\|quicChain' internal/listener/` — the only test-side CALL sites in the package
are `manager_test.go:1042` and `manager_test.go:1075`, **both `rt.quicTLSConfig()`**. `quic_test.go:237`
is a comment. **`quicChain()`, the accessor this row exists to repair, is called by no test at all.**
That sharpens `SPEC.md` §0.6: the coverage finding is not merely that the suite is green under an
inverting patch — one of the two accessors has never been called from a test in the four phases since it
landed. ⚠️ **`TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos` NEVER CALLS
`mgr.Start`** — it is a construction-time test, so it cannot see anything registration-gated or
Start-ordered ([[reference_registration_time_is_not_construction_time]]).

### ⚠️ 0.16 — THE FOUR ADDRESS HELPERS ARE `*net.TCPAddr`-ONLY AND FAIL **SILENTLY** ON UDP

`localIP` / `localPort` / `remoteIP` / `remotePort` (`manager.go:1562-1588`) each comma-ok assert
`*net.TCPAddr` and return `nil` / `0` otherwise. `quic.Conn.LocalAddr()` and `.RemoteAddr()` return
`*net.UDPAddr`. **Reusing the four would compile, run, and produce an all-zero destination and source —
every `destination_port` arm would then match only a chain specifying port 0, and every `source_*`
dimension would silently read as unset.** §4.1 point 2's comma-ok-asserted `*net.UDPAddr` extraction is
therefore not a stylistic choice; it is the only correct one that also keeps `manager.go`'s CODE
byte-untouched. **Do not widen the four helpers** — that would put executable lines into a file this row
holds to comment-only edits (§0.5).

---

## 1. Stage scope, MEASURED

### 1.1 What this PLAN commit touches — FOUR files

`docs/envoy-go/phases/97-quic-chain-selection-order/PLAN.md` (new) ·
`docs/envoy-go/STATE.md` (rolled IN PLACE) ·
`docs/envoy-go/STATE_HISTORY.md` (**+2 / -0**, parenthetical append) ·
`next-prompt.txt` (rolled, `git add -f` — it is TRACKED but gitignored). **Nothing else.**

⚠️ **NOT `PLAN.md` "only"** — §0.3 measures the real scope across five precedents. ⚠️ **And nothing under
`internal/` or `test/`, no `ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no `DECISIONS.md`** — those four
negatives hold in 5/5 precedents. ⇒ **every figure in §8 belonging to those files must STAY unchanged
across this stage**, and §8 records the pre-edit baseline so the close can prove it. **A PLAN adds no ADR,
so the house `PROPOSED` guard stays ARMED through it.**

**No `PROGRESS.md` at this PLAN** — the 92/93/94/95/96 posture; `SPEC.md` §11 row 10 assigns it to the
IMPL. **A STANDING DEPARTURE, named rather than claimed.**

### 1.2 The split gate — EVALUATED, NOT SPLIT, AND THE MARGIN IS **ONE TASK**

`BOOTSTRAP_PROMPT.md` §6.1, read at the repo root at this tip (`:283-292`), triggers a split if `PLAN.md`
exceeds **~25 numbered tasks** OR estimates exceed **~1500 lines of code** of net change. ⚠️ **§5 appears
VERBATIM TWICE** — §5 (`:209`) and the §11 Skill Routing Appendix (`:456`); the second copy's offset is
NON-constant, so both were located by HEADING, never by arithmetic.

**Task count: 24** (§6). **DERIVED here** — `SPEC.md` §14 item 1 deliberately quotes none.

**LoC, anchored on measured precedents rather than on an unanchored guess.** Every "basis" cell names a
file that exists at this tip and was measured with `wc -l` or `git show --numstat`:

| component | basis | estimate |
|---|---|---|
| `internal/listener/quic.go` — the five-point edit | **MEASURED**: the SPEC's built-run-and-reverted prototype, `git diff --numstat` | **+87 / -23** |
| `internal/listener/quic.go` — both accessor doc-comments (§4.5) | the two current blocks are 3 and 6 lines | **+30 / -9** |
| `internal/listener/manager_test.go` — the both-slots QUIC builder | `mkQUICListenerDefaultChain` is 21 lines, `mkQUICListenerHCM` 24; this one is parameterized on a `filter_chain_match` AND a default-slot mode | **+90** |
| `internal/listener/manager_test.go:977` — doc reconciliation (§0.5 site 4) | one comment block | **+8 / -5** |
| `internal/listener/quic_test.go` — arms a-g and j | eight table-free arms on the `quic_test.go` house style (that file has ZERO tables under every matcher form — §4) | **+330** |
| `internal/listener/quic_test.go` — driven arms h and i | `TestQUICListener_ServesH3GET` measures 47 lines; these two add an explicit `ServerName` and a served-chain assertion | **+180** |
| `internal/listener/quic_test.go:237` — doc reconciliation (§0.5 site 3) | one comment block | **+10 / -6** |
| `internal/listener/manager.go:396` — COMMENT-ONLY reconciliation (§0.5 site 2) | one comment block | **+12 / -8** |
| `test/fixtures/0122-…/driver/driver.go` | `0104`'s driver measures **339** with ONE inline template pair; `0122` carries two chains per side and a two-name stat assertion | **+470** |
| `test/fixtures/0122-…/expectations.yaml` | `0104`'s measures **93** | **+110** |
| `test/fixtures/0122-…/README.md` | `0104`'s measures **131** | **+190** |
| `test/differential/runner_test.go` | the blank import | **+1** |
| `docs/envoy-go/DECISIONS.md` — ADR-0319 §Decision + §Consequences | precedent: the phase-96 IMPL `c7bd2880` reads **84 / 6** | **+80 / -6** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the ledger entry + `:1973` (§0.4, §0.5) | precedent: `c7bd2880` reads **3 / 1** | **+6 / -2** |
| `docs/envoy-go/ROADMAP.md` — row 97 flip | precedent: `c7bd2880` reads **2 / 2** | **+2 / -2** |

⚠️ **THIS ROW SHIPS NO PKI.** `0104` reuses `internal/listener`'s existing `testAlphaCertPEM` /
`testAlphaKeyPEM` pair inline in both templates and has **no `pki/` directory at all**; `0122` copies that
shape. Phase 96 spent **201** lines on `0121/pki/**` (three PEMs at 29 plus `pki/gen/main.go` at 172) that
this row does not spend.

**Totals, each labelled with its accounting — a figure without its measure is meaningless:**

| accounting | phase 97 estimate | phase 96 MEASURED (`c7bd2880`) | phase 94 MEASURED (`0a985a35`) | verdict |
|---|---|---|---|---|
| `.go` only | ≈ **+1218 / -51** | **+1285** | **+1143** | under both |
| `.go` + `expectations.yaml` (phase 96's own comparator accounting) | ≈ **+1328** | **+1725** | **+1635** | **under BOTH precedents, and NEITHER was split** |
| the above + `README.md` | ≈ **+1518** | **+1980** | — | above the ~1500 line on the WIDEST accounting only |

`PROGRESS.md` and `next-prompt.txt` are excluded from every column, on the same exclusion phase 94 and 96
are measured under: a transcript and a router are not lines of code.

**DECISION: DO NOT SPLIT.** Four grounds, in order of weight:

1. ⚠️ **On the SAME accounting as the two measured structural precedents, this row is SMALLER than both,
   and neither was split.** `+1328` against phase 96's `+1725` and phase 94's `+1635`. The comparison is
   like-for-like — both precedents also created one differential fixture from nothing and added arms to
   the same test package.
2. **The only clean seam is {unit layer} / {fixture `0122`}, and it defeats the row in both directions.**
   `SPEC.md` §0.6 and §4.3 measured the consequence: `go test ./internal/listener/... -count=1` is rc=0
   both before and after a patch that REVERSES which chain serves every QUIC request. A `97.1` shipping
   the production edit with only the fixture would gate nothing at unit level; a `97.2` shipping the arms
   alone would gate nothing cross-side. **Splitting the pins away from the code is the one split that
   defeats this row.**
3. **The production edit is ONE FILE**, measured at `+87 / -23`. Everything else is the surface that makes
   it falsifiable.
4. **24 tasks against ~25.** ⚠️ **SUB-STEP COUNTS MEASURED, NOT ASSERTED** (`awk` over `^- \[ \] ` per `^### Task ` heading): the per-task histogram runs
   3, 5, 5, 5, 5, 6, 6, 6, 6, 6, 6, 6, 6, 7, 7, 7, 7, 8, 8, 8, 8, 9, **11, 11**. **Tasks 17 and 19 carry ELEVEN each**, at
   `BOOTSTRAP_PROMPT.md` §6.1's mid-execution line. **Named here rather than smoothed** — see the note below.

⚠️⚠️ **THREE THRESHOLDS ARE AT OR PAST THEIR LINE, AND ALL THREE ARE STATED RATHER THAN SMOOTHED:**
the task margin is **ONE** (24 against ~25); the widest LoC accounting is already **over `~1500`**
(≈ +1518); and **Tasks 17 and 19 each carry ELEVEN sub-steps**, past §6.1's ~10.
⚠️ **THE SUB-STEP READING IS THE WEAKEST OF THE THREE AND THE REASON IS STRUCTURAL:** §6.1's
mid-execution clause fires when sub-steps *"blow up past ~10 items once contact with reality reveals
complexity"* — it is about DISCOVERY during execution, not about a written checklist. Tasks 17 and 19
are long because they enumerate CONSTRAINTS TO SATISFY (port census, `stat_prefix` collision,
`BackendCount`, cert delivery, template placeholder order; four registration gates, two `comm`
directions, the extractor NC, the character-class trap), not because they are eleven sequential
units of work. **If either one's real work does blow up during execution, §6.1's remedy is the split
AT THAT MOMENT.** **`BOOTSTRAP_PROMPT.md` §6.1's MID-EXECUTION trigger is the remedy, not a
retroactive re-reading of this verdict**: if any single task's sub-steps blow past ~10 once contact with
reality reveals complexity, the split happens THEN. The measured candidates are **Task 17** (the `0122`
driver, two inline template pairs, ELEVEN sub-steps) and **Task 19** (the four registration gates, ELEVEN),
with **Task 10** (the inputs builder plus the selector, EIGHT) the likeliest to grow on contact —
it is where the resolve-move interaction lands.

⚠️ **AND THE ESTIMATE IS A LOWER BOUND.** [[reference_measured_prototype_is_a_lower_bound]] has now fired
**SEVENTEEN consecutive rows**, and at phase 97 it fired on the SIZE and the SHAPE together: the
BRAINSTORM's floor of `+7 / -10` became the SPEC's measured `+87 / -23`, a **12x** growth **on a shape
that was also the wrong one**. Treat every cell above as a floor.

⚠️ **BOTH PRECEDENT FIGURES WERE RE-DERIVED FIRST-HAND AT THIS TIP, NOT INHERITED FROM THE PHASE-96
PLAN'S TABLE** ([[feedback_brief_citations_not_evidence]]). `git show --numstat 0a985a35` gives phase 94
`.go`-only **73 + 419 + 14 + 1 + 8 + 9 + 1 + 618 = 1143** and, adding `envoy.yaml` 103, `envoy-go.yaml`
89, `expectations.yaml` 242 + 7 + 6 and five PEMs at 45, **1635**. `git show --numstat c7bd2880` gives
phase 96 `.go`-only **17 + 556 + 13 + 1 + 526 + 172 = 1285** and, adding 111 + 125 + 175 and three PEMs at
29, **1725**.

---

## 2. Sentinel — RUN MECHANICALLY AT THIS STAGE'S OWN TIP, ACTUAL OUTPUT

A PLAN edits no `ROADMAP.md` row, so the shapes **should** stay — and *should stay* is still a
measurement. All three checks, all four NCs and the check-(2) positive control were RUN, at this tip, with
`/usr/bin/grep` (§0.6). **`stop` was evaluated and deliberately NOT created**, verified absent at the git
root and in this stage's worktree.

### 2.1 The three checks

- **check (1)**, `want=129`: **ONE** line, `NOT DONE: row 97`. Non-silent is the NORMAL mid-phase state —
  this row is registered `in-progress` and check (1) goes silent again at the IMPL.
- **check (2)**: **SIX**, at `:207 :213 :219 :229 :235 :243`.
- **check (3)**: **SILENT**.

### 2.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

- **NC-A** (row 62 doctored to `in-progress` in a scratch copy, `want=129`): the substitution was
  INSPECTED FIRST — `NC LANDED? [ in-progress ]` — then **TWO** lines, `NOT DONE: row 62` and
  `NOT DONE: row 97`.
- **NC-B** (`want=128` on the real file): **TWO** lines, `NOT DONE: row 97` and
  `GATE FAIL: examined 129 data rows, expected 128`.
- **NC-C** (`gRPC-family row` -> `gRPC-XXXXXX row` in a scratch copy): residual **0**, and
  `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D**: `-family row` **96** occurrences / **68** lines, with `--` before the pattern.
- **check-(2) positive control**: BOTH phrases substituted and the substitution ASSERTED rather than
  assumed — residual **0**, `candidatesXX` **6**. ⚠️ **Substituting only the shorter phrase leaves a
  residual of 5 and reads like a finding**; the longer phrase does not contain it as a substring.

**Per-line md5 of the six windows, trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex) —
⚠️ **the digest is METHOD-SENSITIVE and the method is stated because of it** — **ALL SIX BYTE-IDENTICAL
to the phase-97 SPEC close and to the phase-97 BRAINSTORM close:**

`207 10d7807bf02d` · `213 4a92f7e62fc6` · `219 2a7eb298b9fd` · `229 242e53c6f7a3` ·
`235 b2680e6f4fbf` · `243 6caa1c3ce0e7`

**The escape-aware malformed set is UNCHANGED at exactly two rows** — file line **119** (row **57**,
NF **9**) and **131** (row **69**, NF **10**). ⚠️ **The correct form passes NO file argument to `awk`**;
passing one makes `awk` ignore stdin and print the NAIVE count under the escape-aware label.

⇒ **THE SENTINEL DOES NOT FIRE.**

### 2.3 Row-shape baseline — measured BEFORE this stage's edits, for the IMPL to diff against

Row 97 sits at `docs/envoy-go/ROADMAP.md:159` and reads **`in-progress`**. Its field count is **8 under
BOTH forms** — naive `awk -F'|' 'NR==159{print NF}'` reads **8**, and the escape-aware
`sed 's/\\|//g' … | awk -F'|' 'NR==159{print NF}'` also reads **8**. ⚠️ **THE IMPL FLIPS THIS ROW AND MUST
RE-COUNT UNDER BOTH FORMS, BEFORE AND AFTER** — an unescaped `|` in a summary cell passes check (1) and
silently breaks the field count, and it fired against the phase-96 IMPL's own author, who wrote a Go `||`
into row 96's narrative. **Reword a pipe away rather than escaping it.**

⚠️⚠️ **THE MARGIN IS ONE.** Checks (1) and (3) carry no structural weight — (1) is merely borrowing a
voice from this open row. **Only check (2)'s SIX stands between this project and `stop`. Do not tidy a
candidate line, and do not "fix" the six.** ⚠️ **Check (2) matches the WHOLE FILE**, so a row-97 summary
cell at the IMPL that spelled either match phrase would mint a SEVENTH hit and read as a finding.
⚠️ **NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW** — the phase-94 IMPL's own sentence
asserting the sentinel could not move MOVED IT.

### 2.4 The eviction instruments — a PAIR, run on BOTH files, with an NC and a positive control

⚠️ **THE BARE FORMS ANSWER NOTHING.** `STATE.md`'s strict count reads **5** and is invariant under WHICH
entry is evicted. Only the LABEL-BOUND pair discriminates, and it was run on both files at this tip
(`/usr/bin/grep -cF -- "<label>"`, capture-with-`|| true`):

| label | `STATE.md` | `STATE_HISTORY.md` | role |
|---|---|---|---|
| `phase 96 (listener-default-chain-tlsmode) BRAINSTORM done` | **1** | **0** | **THE EVICTEE** — must go 1 -> 0 and 0 -> 1 |
| `phase 96 (listener-default-chain-tlsmode) SPEC done` | 1 | 0 | the tie's other member, RETAINED |
| `phase 95 (tls-alpn-mismatch-fallback) IMPL done` | 0 | **1** | **positive control on an ARCHIVED label** |
| `phase XX (fabricated-label) NEVER done` | 0 | 0 | **fabricated-label NC** — zero in BOTH |

**The evictee is picked by LIST POSITION, not by date** (§0.8): the date narrows the field to the two
`2026-09-07` entries and position picks the tail. **Archive guard at this tip:** strict **163** /
parenthetical **70** / loose **233**, and **163 + 70 = 233** exactly under the anchored-occurrence forms.
`STATE_HISTORY.md` **566** lines. ⚠️ **The strict form must read 163 with DELTA 0 at the close** — a
correctly-shaped parenthetical append moves only the parenthetical. ⚠️ **The raw line delta is `+2`, not
`+1`** — a blank line PLUS the entry line. ⚠️ **This stage's archive line names NO positive-control
figure**: the archive's positive controls are SELF-INCREMENTING.

---

## 3. The multi-UDP-listener question — **ANSWERED**, discharging `SPEC.md` §14 item 3

**VERDICT: the differential harness does NOT support two UDP/QUIC listeners in one fixture.** `SPEC.md`
§6.2 recorded it as UNVERIFIED and told this PLAN to settle it before any future row assumes it. It is
settled, by reading the code rather than by running the suite.

**The failure is NOT in port plumbing, and that is the part worth carrying forward.** Publishing is
already N-capable: `startReferenceProxy` (`test/differential/harness.go:111-118`) takes
`tcpPorts, udpPorts []int` and formats one `ExposedPorts` entry per member, and the address maps are keyed
by port with a separate `ListenerUDPAddr` accessor (`harness.go:213`). **A two-UDP fixture would publish
and map today with zero harness change.**

**What collapses it is the ADDRESS DISPATCH in `runFixture` (`test/differential/runner_test.go:239`),
which is transport-blind and single-valued at two distinct places:**

1. `runner_test.go:1210-1213` — the single-listener path resolves exactly ONE UDP address:
   `refAddr := ref.ListenerAddr(d.ReferenceListenerPort())`, overwritten by
   `refAddr = ref.ListenerUDPAddr(d.ReferenceListenerPort())` when `refIsUDP`. One scalar.
2. `runner_test.go:1244-1246` — the `MultiListenerDriver` path resolves EVERY address through the **TCP**
   map, unconditionally: `refAddrs[name] = ref.ListenerAddr(ports[i])`. For a `/udp`-published port that
   map returns `""`, so `DriveReferenceMulti` would be handed empty-string addresses for **both**
   listeners — a silent vacuous failure, not a clean error.

**And the marker itself is fixture-wide, not per-port.** `fixture.ReferenceListenerIsUDP`
(`test/differential/fixture/fixture.go:79-85`) is a nullary `bool`; `ReferenceListenerPort()`
(`fixture.go:36-38`) is a scalar whose own doc says *"the in-container TCP port"*; the one plural form,
`MultiListenerDriver.ReferenceListenerPorts()` (`fixture.go:725-730`), has **no transport dimension at
all**. ⇒ **a MIXED 1×TCP + 1×UDP fixture is broken for the same reason** — the single bool would push the
TCP port to `/udp` and its `ListenerAddr` would go empty.

**Readiness is NOT a discriminator and the plan should not spend a task on it.** There is no per-listener
readiness probe of any transport: `grep -n 'wait\.For\|WaitingFor' test/differential/*.go` returns exactly
two hits, `harness.go:133` and `harness.go:431`, both `wait.ForHTTP("/ready").WithPort("9901/tcp")`. A
second UDP listener is exactly as unprobed as the first.

**What a future row would have to change — THREE edits, none of them in `startReferenceProxy`:**

1. `fixture.go:83-85` — replace the nullary marker with a per-port form (e.g.
   `ReferenceUDPListenerPorts() []int`), keeping `ReferenceListenerIsUDP` as a deprecated shim so `0104`
   stays byte-stable.
2. `runner_test.go:1196-1213` — partition `refPorts` into `tcpPorts`/`udpPorts` by that predicate and pass
   BOTH to `startReferenceProxy`; today it passes `nil, refPorts`.
3. `runner_test.go:1244-1246` — dispatch `refAddrs[name]` per port between `ListenerAddr` and
   `ListenerUDPAddr`.

⚠️ **A PRE-EXISTING ADJACENT GAP, ALREADY DOCUMENTED IN-TREE AT `runner_test.go:1196-1201`:** the
`ReferenceLogMounter` branch **wins over** `refIsUDP`, so a UDP fixture that also mounts silently exposes
`/tcp` (`StartReferenceProxyWithMounts`, `harness.go:201-203`, passes `udpPorts=nil`). `0122` mounts
nothing, so it is not exposed to this — **stated so a future row does not rediscover it.**

⚠️ **PORT PUBLISHING IS DERIVED FROM THE FIXTURE'S GO DECLARATION, NOT FROM PARSING ITS YAML.** The
bootstrap is passed opaquely as `--config-yaml` (`harness.go:132`) and the harness never parses it —
matcher `grep -n 'yaml\|Unmarshal\|expectations' test/differential/runner_test.go`, **zero** hits.
**Consequence for `0122`: a template declaring a listener whose port the driver does not declare is
published to nothing.**

⇒ **`SPEC.md` §6.2's decision stands and is now justified by mechanism rather than by caution: `0122`
carries ONE listener, the ELIGIBLE arm, and the ineligible arm lives in the unit arms b and c**, which can
build any shape freely. **The subject side is NOT the blocker** — `readyListenerAddrs`
(`harness.go:63-79`) already builds a name -> addr map from one `envoy-go listener <name> ready on <addr>`
line per listener, so N subject QUIC listeners already resolve by name. ⚠️ **One caveat recorded for a
future row: the subject's derived ports come from `freeTCPPortBlock` (`harness_test.go:281-316`), which
probes with `net.Listen("tcp", …)` only — a derived port used for a QUIC listener is never UDP-probed.
A flake risk, not a structural blocker.**

---

## 4. The test surface, READ — discharging `SPEC.md` §14 item 4

Every figure below names its matcher and was run at this tip. **§0.1 refutes the SPEC's own diagnosis of
the `[]struct{` zero; this section is what a correct matcher reports.**

### 4.1 Inventory

| file | lines | `^func Test` |
|---|---|---|
| `internal/listener/integration_test.go` | 294 | 1 |
| `internal/listener/listener_test.go` | 145 | 1 |
| `internal/listener/manager_test.go` | **6467** | **104** |
| `internal/listener/quic_negative_test.go` | 150 | 2 |
| `internal/listener/quic_test.go` | **326** | 4 |
| `internal/listener/tls_handshake_negative_test.go` | 205 | 2 |

Matchers: `wc -l`, `grep -c '^func Test'`.

### 4.2 Table structure — per file, under FOUR matcher forms

| file | `for _, tc := range` | `for _, tt := range` | `\[\]struct\{` | `\[\]struct[ ]*\{` | `\[\]struct[ ]*$` | `t\.Helper()` |
|---|---|---|---|---|---|---|
| `integration_test.go` | 1 | 0 | 0 | — | 0 | 2 |
| `listener_test.go` | 0 | 0 | 0 | — | 0 | 0 |
| `manager_test.go` | **6** | **0** | **0** | **6** | **0** | **73** |
| `quic_negative_test.go` | **0** | 0 | 0 | **0** | 0 | 2 |
| `quic_test.go` | **0** | 0 | 0 | **0** | 0 | 3 |
| `tls_handshake_negative_test.go` | 0 | 0 | 0 | — | 0 | 1 |

⇒ **BOTH QUIC test files are table-FREE under every matcher form.** Every test in them is a straight-line
arm. `quic_negative_test.go:137` has a `for _, datagram := range [][]byte{…}` literal loop, which is a
payload list, not a case table. **The new arms therefore follow the file's house style: one `func Test…`
per arm, no table** — which is also what `SPEC.md` §5.1 rows h and i force anyway, since they must drive.

`manager_test.go`'s six tables sit at `:1092 :1484 :4777 :5444 :5691 :6102` with their loops at
`:1109 :1559 :4792 :5456 :5752 :6157`; every loop binds `tc` and the `tt` idiom does not appear anywhere
in the package.

### 4.3 What exists to build on, and what does not

**Every QUIC listener builder lives in `manager_test.go`.** `quic_test.go` and `quic_negative_test.go`
define no listener builders of their own.

| builder | line | sets `FilterChains` | sets `DefaultFilterChain` | takes a `filter_chain_match` |
|---|---|---|---|---|
| `mkQUICDownstreamTS` | `:803` | — (returns a `*corev3.TransportSocket`) | — | — |
| `mkQUICListener` | `:834` | **yes**, 1 entry | no | **no** |
| `mkQUICListenerWithOptions` | `:863` | yes (delegates) | no | **no** |
| `mkQUICListenerHCM` | `:881` | **yes**, 1 entry, ALPN hard-wired `["h3"]` | no | **no** |
| `mkQUICListenerDefaultChain` | `:979` | **no** (nil) | **yes** | **no** |

⇒ **the PLAN adds ONE builder** (Task 2) taking a `*listenerv3.FilterChainMatch` and a default-slot mode.
⚠️ **`mkQUICListenerHCM` is the right parent, not `mkQUICListener`** — the arms must distinguish WHICH
chain served, and only an HCM terminal gives a per-chain observable. `mkQUICListener`'s terminal is a
`tcp_proxy` (`mkTcpProxyFilter`), which serves no H3 at all; `serveQUICConnection` logs
*"chain terminal is not H3-capable"* and closes (`quic.go:133-140`).

⚠️ **`mkQUICListenerHCM`'s route is `direct_response` 200 `"OK\n"` on `/health`** via
`mkHCMFilterWithCodec` (`manager_test.go:1933`). **The new builder must parameterize the response body per
chain** — that is the arm's discriminator. Prefer **status `222`** over `200` for consistency with the
fixture and with the H3 1xx finding (Global Constraints).

### 4.4 The driven-arm template, and the input it does NOT send

The package has three copies of one H3 client shape: `driveH3` (`quic_test.go:94-124`, returns `error` so
a dial failure is a PRECONDITION rather than a property), the inline block in
`TestQUICListener_ServesH3GET` (`quic_test.go:177-223`), and `h3GetHealth` (`quic_negative_test.go:61-88`).
All three build:

```go
rt := &http3.Transport{
    TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
    QUICConfig:      &quic.Config{},
}
```

🔴 **`ServerName` IS NOT SET IN ANY OF THE THREE.** Matcher `grep -rni 'servername'
internal/listener/` — every `ServerName:` in a client config in this package lives in
`tls_handshake_negative_test.go` (`:71 :81 :169 :183`), all TCP. **Zero in either QUIC file.** And the
target is a `127.0.0.1:<port>` literal, so `crypto/tls` sends **no SNI at all** by default. ⇒ **arms h and
i MUST set `ServerName` explicitly**, or both are vacuous — exactly the trap `SPEC.md` §2's own rig had to
disarm on the reference side, reproduced on the subject side. **Name the inputs your probe SENDS, not only
the ones it asserts.**

Address plumbing on every driven arm: `mgr.Listeners()` -> assert `len(infos) == 1` -> `infos[0].Addr`.
Neither QUIC test file imports `test/helpers` — matcher spelled `test/helpers` AND `testhelpers`; **none
of the six `*_test.go` files in the package matches either.** The new arms need no new dependency.

The raw-QUIC (non-H3) dial template, for any arm that wants the negotiated state rather than a response,
is `TestQUICListener_HandshakeALPNh3` (`quic_test.go:151-156`): `quic.DialAddr(ctx, addr, clientTLS,
&quic.Config{})` then `conn.ConnectionState().TLS`.

### 4.5 The `GetConfigForClient` pin `SPEC.md` §1 and §4.4 rely on

Matcher `grep -rni 'getconfigforclient' internal/ test/` — **36 hits across 8 files**. In
`internal/listener` there are exactly **TWO executable assertions**:

| site | enclosing symbol | what it is |
|---|---|---|
| `manager_test.go:1043-1045` | `TestBuildListenerRuntime_QUICDefaultFilterChain_PlainTLSRejects` (`:1033`) | a **diagnostic** arm inside an `if err == nil` branch the test expects never to be taken; the primary assertions are two `strings.Contains` checks at `:1048` and `:1051` |
| `manager_test.go:1082-1084` | `TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos` (`:1062`) | **THE "stays nil" PIN**, a `t.Error` |

**`SPEC.md`'s claim is VERIFIED**, with the enclosing symbol named. ⚠️ **There is NO differential-side
pin** — zero `GetConfigForClient` hits under `test/` except three prose comments in `0002-tls-tcp`.
⚠️ **AND THE PIN'S OWN TEST NEVER CALLS `mgr.Start`** (§0.15): it is construction-time only, so it is
blind to anything the repair does at Start. **The IMPL must keep it green and must not mistake it for
coverage of the Start-time selection.**

### 4.6 Reachability, and why a failing arm here can look like a crash

Matcher `grep -rn 'recover()' --include='*.go' internal/listener internal/tls | grep -v '_test\.go'` —
**NO OUTPUT.** Zero `recover()` in any non-test file under either tree, `listenerfilter/**` included.
Widened case-insensitively with tests included, the nine hits are all test-side calls
(`listenerfilter/registry_test.go:34,47`; `tls_inspector/parser_test.go:103`;
`tls_inspector/tls_inspector_test.go:108,215`) or prose asserting the absence
(`manager_test.go:2336 :2449 :2568 :5248 :6296`). **`internal/tls` has zero hits of any kind.**

⇒ **a nil deref on the accept goroutine ABORTS THE TEST BINARY.** The package's own
`assertDefaultChainTLSPosture` (`manager_test.go:6277-6300`) is built around exactly this: it asserts the
five `ssl.*` counter POINTERS **pre-dial**, with `t.Errorf` on every property and never `t.Fatalf`, so no
property becomes dead code. **Any new arm that drives traffic must assert its pointers BEFORE the first
byte crosses the wire** ([[reference_nil_stats_counter_inc_crashes_goroutine]]) — and this row's arms
drive at a shape whose counters ARE registered (phase 96 widened `tlsMode` to cover the default slot), so
the expected state is *no crash*, which is exactly why the **anchored gate must be PROVEN LIVE** rather
than read as 0 (Task 1).

---

## 5. STABLE ANCHORS — use these, never line numbers

⚠️ **EVERY LINE NUMBER IN THIS DOCUMENT AND IN `SPEC.md` IS DERIVED DATA.** This row inserts into
`quic.go` in multiple places, so the shift is **BANDED, not a constant**
([[reference_line_shift_after_insert_is_banded]]). **Re-locate every cite by LITERAL TEXT
(`grep -nF -- "<exact line>" <path>`), and anchor on the ENCLOSING SYMBOL.**

| what | anchor |
|---|---|
| the QUIC start path | `func (rt *listenerRuntime) startQUIC(ctx context.Context, reg *stats.Registry) error` — `internal/listener/quic.go` |
| the resolve write to move | the statement `rt.addr = udpConn.LocalAddr().String()` inside `startQUIC` |
| the Start-time TLS call | `tlsCfg := rt.quicTLSConfig()` inside `startQUIC` |
| the per-connection serve path | `func (rt *listenerRuntime) serveQUICConnection(ctx context.Context, conn *quic.Conn)` |
| the per-connection chain call | `ci := rt.quicChain()` inside `serveQUICConnection` |
| the per-connection TLS call | the `TLSConfig:  rt.quicTLSConfig(),` field of the `&http3.Server{…}` literal |
| the nil-selection close to KEEP | `if ci == nil {` … `_ = conn.CloseWithError(0, "")` inside `serveQUICConnection` |
| the two accessors | `func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config` and `func (rt *listenerRuntime) quicChain() *chainInfo` |
| the two loops to DELETE | `for _, ci := range rt.chainByName {` — **TWO occurrences, one in each accessor** |
| the selector | `func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)` — `internal/listener/listenerfilter/chainmatch.go` |
| the inputs struct | `type ChainMatchInputs struct` — `internal/listener/listenerfilter/types.go` |
| the eligibility predicate | `func matches(c *ChainSpec, inputs *ChainMatchInputs) bool` — `chainmatch.go` |
| the SNI matcher | `func sniMatchAny(patterns []string, sni string) bool` — `chainmatch.go` |
| the ALPN matcher | `func alpnMatchAny(want, offered []string) bool` — `chainmatch.go` |
| the TCP call-site pattern to mirror | `selectedSpec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)` inside `func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn)` — `manager.go` |
| the spec -> info lookup | `selected := rt.chainByName[selectedSpec.Name]` in the same function |
| the runtime fields | `chainSpecs []*listenerfilter.ChainSpec`, `defaultSpec *listenerfilter.ChainSpec`, `defaultChain *chainInfo`, `chainByName map[string]*chainInfo` in `type listenerRuntime struct` |
| the default-slot map write | `chainByName[defaultSpec.Name] = defaultChain` inside `func buildListenerRuntimeWithCtx(…)` — ⚠️ **NOT `buildListenerRuntime`** (§0.7) |
| the QUIC mandatory-TLS reject (indexed slot ONLY) | `quic listener requires a transport_socket (mandatory TLS)` in `buildListenerRuntimeWithCtx` |
| the default-slot TS branch with NO kind check | `if ts := dfc.GetTransportSocket(); ts != nil {` in `buildListenerRuntimeWithCtx` |
| the `transport_protocol` enum gate | `case "", "tls", "raw_buffer", "quic":` inside `func parseChainSpec(` — `manager.go` |
| the TCP-only address helpers **not to widen** | `func localIP(c net.Conn) net.IP`, `localPort`, `remoteIP`, `remotePort` — `manager.go` |
| the stale-precedence comment to reconcile | the literal `returns rt.defaultChain.tlsCfg FIRST` in `manager.go` and `returns rt.defaultChain.tlsCfg BEFORE` in `quic_test.go` (§0.5) |
| the fixture registration seam | `fixture.RegisterFixture(fixtureName, &…{})` in the driver `init()`, and the blank import block in `test/differential/runner_test.go` |
| the UDP marker | `type ReferenceListenerIsUDP interface` — `test/differential/fixture/fixture.go` |

**Key facts the anchors above encode, each verified at this tip:**

- **`SelectChain` returns a `*ChainSpec`, never a `*chainInfo`.** The caller must map spec -> info through
  `rt.chainByName[spec.Name]`, exactly as the TCP path does.
- **When nothing is eligible, `SelectChain` returns `defaultChain` WITHOUT evaluating its match** — and
  `defaultSpec` is built `Empty: true` unconditionally, whatever the config's `default_filter_chain`
  declares. When `defaultChain` is nil it returns `(nil, ErrNoChainMatched)`.
- **`matches` compares `TransportProtocol` by exact, case-sensitive `!=`.** The CHAIN's empty string means
  "unspecified" and skips the dimension; there is **no reverse wildcard** — a chain spelling `"quic"`
  against an unset input is INELIGIBLE. That asymmetry is the whole reason the constant must be stamped.
- **`sniMatchAny` returns TRUE for the literal pattern `"*"` regardless of the SNI, empty string
  included.** Any statement about `server_names` eligibility under an empty `ServerName` must carry that
  exception.
- **`chainInfo` has exactly three fields** — `serverNames`, `tlsCfg`, `netChainFactory` — and **no
  `Name`**. The spec -> info binding is the map key alone.
- **`quic.Conn.ConnectionState()` returns a VALUE**, mutex-guarded (`connection.go:723-733` of
  `quic-go v0.54.1`), and its `.TLS` field is a **`crypto/tls.ConnectionState`**, the standard-library
  type, not a quic-go one. `.ServerName`, `.NegotiatedProtocol`, `.LocalAddr()` and `.RemoteAddr()` all
  exist; `LocalAddr`/`RemoteAddr` return `net.Addr` carrying `*net.UDPAddr` on this path.

---

## 6. Tasks

**24 tasks.** ⚠️ **THE ORDER IS LOAD-BEARING AND THE EVIDENCE IS UNRECOVERABLE AFTERWARDS.** Tasks 1-8
land and RUN at the **UN-FIXED TIP**, because that tip IS the negative control for NC roster row 1 and
because §0.10's RED/GREEN split can only be observed before Task 9. **Do not reorder Tasks 1-8 after 9.**

**Every task ends with a commit** (`feedback_subagent_autocommit_claudemd`), on the stage branch, with
explicit pathspecs. **Subagents do not push.** ⚠️ Use `git -C <abs-worktree-path>` for every git command.

### Task 1: Prove the anchored panic gate LIVE, and record the un-fixed baseline

- [ ] Record the tip: `git -C <wt> rev-parse HEAD`, and `sha256sum internal/listener/quic.go`.
- [ ] Run the package baseline WITH `-v` and capture both figures:
      `go test ./internal/listener/... -count=1 -v 2>&1 | tee $SCRATCH/base.txt; RC=${PIPESTATUS[0]}`,
      then `grep -c '^=== RUN' $SCRATCH/base.txt` and
      `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' $SCRATCH/base.txt` (capture with `|| true`).
      ⚠️ **`RUN=0` beside `RC=0` is a VACUOUS GREEN.**
- [ ] Run the anchored panic gate over that same output: `grep -cE '^panic:|DATA RACE|SIGSEGV'` — expect
      **0**.
- [ ] ⚠️ **PROVE THE GATE LIVE.** Insert a bare `panic("PROBE")` at the TOP of `quicChain` **and nothing
      else**, run `go test ./internal/listener/ -count=1 -run 'TestQUICListener' -v`, and confirm the gate
      reads **>= 1** and the trace names `quicChain`. Revert; `sha256sum -c` the pre-patch digest.
- [ ] ⚠️ **ISOLATE — ONE SITE PER RUN.** Repeat the same probe for `quicTLSConfig` in a SEPARATE run.
      **With a `panic()` in two sites the run aborts on the first and says NOTHING about the second**
      ([[reference_matched_negative_per_dimension]]); `SPEC.md` §0.6 records that this exact mistake
      masked the second site once already.
- [ ] Record which existing tests reach each accessor. Expect `quicTLSConfig` reached (via
      `manager_test.go:1042` and `:1075`) and `quicChain` reached only through the DRIVEN H3 tests —
      **it has zero direct test call sites** (§0.15).
- [ ] Confirm the selector `./internal/listener/...` RESOLVES (`go list ./internal/listener/...`) before
      believing any FAIL — a package selector that does not resolve prints `[setup failed]` and exits 1.
- [ ] Commit: the baseline numbers into `PROGRESS.md`'s first section. No `.go` change survives this task.

### Task 2: The both-slots QUIC builder, and arm **a** — RED AT TIP

- [ ] In `manager_test.go`, add ONE builder beside `mkQUICListenerDefaultChain`. Suggested shape:
      `mkQUICListenerChains(t *testing.T, fcm *listenerv3.FilterChainMatch, body string, withDefault bool,
      defaultTLS bool, defaultBody string) *listenerv3.Listener`. It MUST:
      parent on **`mkQUICListenerHCM`**, not `mkQUICListener` (§4.3 — a `tcp_proxy` terminal serves no H3);
      set `FilterChains[0].FilterChainMatch = fcm`; give each chain its own HCM `stat_prefix` AND its own
      `direct_response` body so the ARM CAN TELL WHICH CHAIN SERVED; and, when `withDefault`, set
      `DefaultFilterChain` with a QUIC-wrapped transport socket if `defaultTLS`, or **NO
      `transport_socket` at all** if not (§0.11 — that is the only shape that separates the two accessors).
- [ ] ⚠️ **Give the builder a doc comment that states BOTH asymmetries it depends on**: a
      `filter_chains[i]` with no `transport_socket` BOOT-REJECTS on a QUIC listener while the
      `default_filter_chain` slot does not, and that asymmetry is the banked D2-QUICTS divergence this
      builder consumes without repairing.
- [ ] Add arm **a**: empty-match indexed chain + QUIC-TLS default slot. Assert **which `*chainInfo`
      `rt.quicChain()` returns**, by pointer equality against `rt.chainByName["<listener>/filter_chains[0]"]`
      — an EXACT equality, not a non-nil floor. ⚠️ **Use `t.Errorf` per property, never `t.Fatalf`**;
      reserve `Fatalf` for a broken precondition (the listener failed to build). **Each message names its
      own property.**
- [ ] RUN it. **Expect RED**, with the failure showing the DEFAULT chain returned.
- [ ] ⚠️ **NC THE ARM ITSELF**: temporarily assert the OPPOSITE (default wins) and confirm it goes GREEN,
      proving the arm is executable and its assertion is live. Revert.
- [ ] Commit `manager_test.go` + `quic_test.go`.

### Task 3: Arms **b** and **c** — `destination_port`, ONE red and ONE green, and say WHY

- [ ] Arm **b**: indexed chain with `filter_chain_match.destination_port` set to a port the listener is
      NOT bound to, plus a QUIC-TLS default slot. Expected selection: **the default chain**.
- [ ] Arm **c**: the same ineligible indexed chain with **NO default slot**. Expected: `quicChain()`
      returns **nil**, and `serveQUICConnection` CLOSES the connection — which is exactly what the
      reference does with `no filter chain found` (`SPEC.md` §2 arm E).
- [ ] ⚠️ **RECORD IN THE ARM'S OWN DOC COMMENT THAT b IS GREEN AT THE UN-FIXED TIP AND WHY** (§0.10): the
      subject answers *"default"* on every arm, so it matches here by coincidence, not by evaluation.
      **b's falsifiability comes from NC roster row 2, not from red-at-tip.** A reader who sees b pass
      before Task 9 must not read that as coverage.
- [ ] ⚠️ **ARM b NEEDS A BOUND PORT TO BE INELIGIBLE AGAINST.** A `port_value: 0` listener resolves its
      port at Start, so either (i) drive the arm through `mgr.Start` and read `mgr.Listeners()[0].Addr`,
      or (ii) assert against the CONFIGURED address only and add a second, port-0 variant for Task 9's
      resolve-move check. **Task 16's NC roster row 6 depends on one of these existing — decide here and
      say which.**
- [ ] RUN both. Expect **c RED**, **b GREEN**. Record both.
- [ ] Commit.

### Task 4: Arms **d** and **e** — `transport_protocol`, a MATCHED PAIR

- [ ] Arm **d**: indexed chain with `filter_chain_match.transport_protocol: "quic"` + default slot.
      Expected: **indexed**.
- [ ] Arm **e**: the byte-identical listener with `transport_protocol: "tls"`. Expected: **default**.
- [ ] ⚠️ **d ALONE CANNOT DISTINGUISH "the constant was stamped and matched" FROM "the dimension is
      unenforced."** The pair is the gate. `SPEC.md` §2.3 disarms the same trap on the reference side.
- [ ] Confirm at this tip that BOTH values PARSE — `parseChainSpec`'s enum gate accepts `{"", "tls",
      "raw_buffer", "quic"}` — so neither arm boot-rejects and both are genuinely runtime arms.
- [ ] RUN both. Expect **d RED**, **e GREEN**.
- [ ] Commit.

### Task 5: Arms **f** and **g** — `application_protocols`, a MATCHED PAIR

- [ ] Arm **f**: indexed chain with `application_protocols: ["h3"]` + default slot. Expected: **indexed**.
- [ ] Arm **g**: byte-identical with `["h2"]`. Expected: **default**.
- [ ] ⚠️ **Record §0.14 in the arms' doc comment**: on QUIC this input is a listener CONSTANT, not the
      client's offer list, because `crypto/tls.ConnectionState.NegotiatedProtocol` is a scalar. The arms
      agree with the reference (`SPEC.md` §2 arms G/G2) **by a different mechanism than TCP's**, and a
      future row adding a second QUIC ALPN breaks the constant silently.
- [ ] RUN both. Expect **f RED**, **g GREEN**.
- [ ] Commit.

### Task 6: Arms **h** and **i** — DRIVEN `server_names`, and the input the template does not send

- [ ] These CANNOT be nil-conn arms. `ServerName` arrives only with a real connection, so both must
      `mgr.Start(ctx)`, dial real H3, and assert **which chain served** from the response body (the
      per-chain `direct_response` of Task 2) — a nil-conn arm asserting `server_names` behaviour would be
      an invented input no production path produces.
- [ ] 🔴 **SET `ServerName` EXPLICITLY ON THE CLIENT `stdtls.Config`.** §4.4: none of the package's three
      H3 client copies sets it, and the dial target is a `127.0.0.1:<port>` literal, so `crypto/tls`
      sends **no SNI at all** by default. Without this both arms are vacuous.
- [ ] Arm **h**: indexed chain with `server_names: ["alpha.envoy-go.test"]`, client sends that SNI.
      Expected: **indexed**.
- [ ] Arm **i**: byte-identical listener, client sends a NON-matching name. Expected: **default**.
- [ ] ⚠️ **The leaf must actually carry that SAN or the handshake fails before selection runs.** The
      package's `testAlphaCertPEM` carries `SAN alpha.envoy-go.test`; keep `InsecureSkipVerify: true` on
      the client so arm i's non-matching name does not fail verification instead of falling to the default
      — **arm i must fail the MATCH, not the HANDSHAKE.** State that in the arm's comment.
- [ ] ⚠️ **Prove the dial reached the server**: assert the response body, not merely that the dial
      returned. A probe client that only completes a handshake answers "did the handshake complete", not
      "was the connection served".
- [ ] RUN both. Expect **h RED**, **i GREEN**.
- [ ] Commit.

### Task 7: Arm **j** — the accessor-identity pin, on the PLAINTEXT-default shape

- [ ] Build: QUIC-wrapped **empty-match** `filter_chains[0]` + a `default_filter_chain` with **NO
      `transport_socket`**. §0.11 proves this is the ONLY shape that separates the two accessors, and that
      it BUILDS.
- [ ] Assert that the chain `quicTLSConfig()` draws its config from and the chain `quicChain(conn)`
      returns are **THE SAME CHAIN**. ⚠️ **`quicTLSConfig` returns a `*stdtls.Config`, not a
      `*chainInfo`** — so assert `rt.quicTLSConfig() == selected.tlsCfg` by POINTER, where `selected` is
      what `quicChain` returned. **Pointer identity, not "both non-nil".**
- [ ] ⚠️ **THE INDEXED CHAIN MUST BE ELIGIBLE** (§0.11): with an ineligible one the post-fix accessors
      legitimately disagree, and the arm would be RED against CORRECT code
      ([[reference_pin_can_fail_against_correct_code]]).
- [ ] RUN it. **Expect RED at the tip**, with the failure showing TLS from `filter_chains[0]` and filters
      from the default slot — `BRAINSTORM.md` §2.2's measured cross-wiring, now pinned.
- [ ] Commit.

### Task 8: RECORD the un-fixed-tip roster — the negative control, SPENT

- [ ] Run the ten new arms as one selection and record, PER ARM, RED or GREEN. **Expected: RED = {a, c, d,
      f, h, j}, GREEN = {b, e, g, i}** (§0.10).
- [ ] ⚠️ **CONFIRM WHICH ASSERTION FIRED on every RED arm**, not merely that the arm failed. A first
      divergence can mask later ones.
- [ ] Run the anchored panic gate over the output — expect **0**, and note it was PROVEN LIVE at Task 1.
- [ ] Write the roster into `PROGRESS.md` as a TABLE. ⚠️ **This table is NC roster row 1's evidence and it
      cannot be reproduced after Task 9** — row 1's mutation (make the selector prefer `rt.defaultChain`
      first again) reproduces exactly this tip.
- [ ] ⚠️ **DIFF THE ARM ROSTER, NOT THE COUNTERS**, at every later checkpoint: a `+0/+0` arm can be
      silently deleted with every gate staying green.
- [ ] Commit `PROGRESS.md`.

### Task 9: Production edit 1 of 3 — MOVE THE RESOLVE

- [ ] In `startQUIC`, move `rt.addr = udpConn.LocalAddr().String()` to immediately after the successful
      `net.ListenUDP`, **above** the `tlsCfg := rt.quicTLSConfig()` call.
- [ ] ⚠️ **DECLARE THE ONE BEHAVIOUR CHANGE IN THE COMMIT BODY** (`SPEC.md` §4.6): on `startQUIC`'s two
      failure paths (`tlsCfg == nil`, `quic.Listen` error) `rt.addr` now holds the RESOLVED address where
      it previously held the configured one, so `Manager.Start`'s `listener: %q: bind %s: %w` text changes
      for a `port_value: 0` listener from `:0` to the resolved port.
- [ ] **PROVE nothing pins either message.** Grep the tree for `bind %s` and for
      `quic listener has no TLS config` and report every hit with its file and enclosing symbol. The SPEC
      says no test and no fixture pins either; **re-verify at your tip rather than inheriting it.**
- [ ] ⚠️ **This is the ONLY reason a port-0 arm behaves consistently at both moments.** Without it the
      Start-time selection sees `…:0` while the per-connection selection sees the resolved port, and a
      `destination_port` chain is selected differently at the two moments — **the same class of defect
      this row repairs.**
- [ ] Assert the edit landed BY SYMBOL, not by build: `grep -n 'rt.addr = udpConn.LocalAddr()'` must now
      precede `grep -n 'tlsCfg := rt.quicTLSConfig()'` in `quic.go`. **A build is not evidence an edit
      landed.**
- [ ] `gofmt -l internal/listener/` — gate on OUTPUT, empty. `go vet ./internal/listener/...` rc 0.
- [ ] Commit.

### Task 10: Production edit 2 of 3 — the inputs builder, the selector, and `quicChain(conn)`

- [ ] Add `func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn)
      listenerfilter.ChainMatchInputs`. It ALWAYS stamps `TransportProtocol: "quic"` and
      `ApplicationProtocols: []string{"h3"}` (measured: `SPEC.md` §2 arms F/F2 and G/G2).
      - `conn == nil` (Start): destination from `rt.addr` via `net.ResolveUDPAddr`; `ServerName` empty;
        source unset.
      - `conn != nil`: destination from `conn.LocalAddr()`, source from `conn.RemoteAddr()`, **both
        comma-ok asserted to `*net.UDPAddr`**; `ServerName` from `conn.ConnectionState().TLS.ServerName`.
- [ ] 🔴 **DO NOT REUSE `localIP` / `localPort` / `remoteIP` / `remotePort`** (§0.16). They comma-ok assert
      `*net.TCPAddr` and return `nil`/`0` on a UDP address — SILENTLY. Reusing them would make every
      `destination_port` arm match only a chain specifying port 0. **And do not widen them**: that puts
      executable lines into `manager.go`, which this row holds to comment-only edits (§0.5).
- [ ] Add `func (rt *listenerRuntime) selectQUICChain(conn *quic.Conn) *chainInfo`:
      `listenerfilter.SelectChain(rt.quicChainMatchInputs(conn), rt.chainSpecs, rt.defaultSpec)`, then
      `rt.chainByName[spec.Name]`, returning **nil on error**. ⚠️ **`SelectChain` returns a `*ChainSpec`,
      never a `*chainInfo`** — the map lookup is mandatory, and it is the same shape `serveConnection`
      uses on TCP.
- [ ] Change `quicChain` to take the conn and return `selectQUICChain(conn)`. Update its one call site.
- [ ] ⚠️ **KEEP `serveQUICConnection`'s existing nil-to-`CloseWithError`.** A nil selection MUST close —
      that is what the reference does with `no filter chain found` (`SPEC.md` §2 arm E), and arm **c**
      pins it.
- [ ] ⚠️ **Log the selection error before closing**, mirroring `serveConnection`'s
      `log.Printf("listener %q: chain-match: %v", …)`. A silent close is indistinguishable from a healthy
      one in a differential run.
- [ ] `gofmt -l` empty · `go vet` rc 0 · `go build ./...` rc 0 (build the binary with `-o` into scratch,
      never into the worktree root).
- [ ] Commit.

### Task 11: Production edit 3 of 3 — `quicTLSConfig` stays connection-INDEPENDENT, and BOTH loops die

- [ ] `quicTLSConfig()` keeps its nullary signature — **`quic.Listen` demands one config before any
      connection exists**, so any repair that makes it depend on per-connection state is unimplementable
      at `startQUIC`'s call site.
- [ ] Its body becomes: the **Start-time selection**'s `tlsCfg` (`selectQUICChain(nil)`), else the first
      TLS-bearing chain **in `rt.chainSpecs` SLICE ORDER**, else the default slot's `tlsCfg`, else nil.
- [ ] 🔴 **BOTH `for _, ci := range rt.chainByName` LOOPS ARE DELETED.** `rt.chainByName` is a MAP that
      CONTAINS the default slot (`buildListenerRuntimeWithCtx` writes
      `chainByName[defaultSpec.Name] = defaultChain`), so both loops range over the union in
      nondeterministic order and can return the default chain by map-order accident while the code reads
      as if it were choosing an indexed one. **Removing the iteration removes the hazard; documenting it
      does not.** Gate: `grep -c 'for _, ci := range rt.chainByName' internal/listener/quic.go` must read
      **0** (capture with `|| true`).
- [ ] ⚠️ **The fallback ORDER matters and is not arbitrary.** `chainSpecs` slice order is the config's
      `filter_chains[]` order; the default slot is consulted LAST, which is ADR-0080 §Decision 2's rule.
- [ ] ⚠️ **`quicTLSConfig` is called at TWO MOMENTS** — `startQUIC` and the `&http3.Server{…}` literal in
      `serveQUICConnection`. Both invocations must return the same thing, or the cross-wiring this row
      exists to close comes back. Being connection-independent is what guarantees that.
- [ ] Confirm `TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos` still passes,
      **BY NAME** (`-run` with the exact name, `-v`), and that it did not silently skip. It calls
      `quicTLSConfig()` on a **never-Started** runtime, so `rt.addr` is the configured string,
      `net.ResolveUDPAddr` still parses it, and the Start-time selection still resolves.
- [ ] `gofmt -l` empty · `go vet` rc 0 · `go build ./...` rc 0.
- [ ] Commit.

### Task 12: ALL TEN ARMS GREEN, plus the build and race gates

- [ ] Run the ten arms. **Expect all ten GREEN**, and record the `=== RUN` count moving from Task 8's
      figure to this one — `RUN` must be non-zero and unchanged in membership (§Task 8's roster diff).
- [ ] Run the WHOLE package: `go test ./internal/listener/... -count=1 -v`, `PIPESTATUS[0]` rc 0, anchored
      FAIL matcher `^(FAIL|--- FAIL)|^ *--- FAIL` reading **0**, anchored panic gate reading **0**.
- [ ] ⚠️ **RUN `-race` ON THE FULL `internal/listener` PACKAGE**, not on a `-run`-narrowed selection: a
      selector matching nothing prints `[no tests to run]` and **exits 0**. The new driven arms start a
      manager and a background accept goroutine, which is exactly the shape only a full-package `-race`
      catches.
- [ ] ⚠️ **A GREEN SUITE IS NOT A PASS UNTIL THE NCs RUN.** `SPEC.md` §0.6 measured this package green
      under a patch that REVERSES selection. Tasks 14-16 are what makes this green mean something.
- [ ] Commit.

### Task 13: The comment reconciliation — FIVE sites, and `manager.go` under a COMMENT-ONLY GATE

- [ ] **`quic.go`, both accessor doc-comments (§4.5).** Rewrite both to describe the **two-moment split**:
      `quicTLSConfig` answers at Start and must be connection-independent; `quicChain` answers per
      connection. **Delete the "supports exactly one chain" precondition claim** — the repair removes the
      map iteration, so the precondition stops being one, and the code now supports N chains by the
      mandated algorithm. ⚠️ **DO NOT ADD A BOOT-TIME CHAIN-COUNT REJECT** (`SPEC.md` §4.5): it would
      refuse configs the reference ACCEPTS and SERVES (§2 arms A, F, G, H).
- [ ] ⚠️ **THE TWO CURRENT COMMENTS CONTRADICT EACH OTHER** (`SPEC.md` §0.4) — only `quicChain`'s asserts
      the one-chain precondition; `quicTLSConfig`'s says the opposite (*"if multiple chains exist, the
      first non-nil TLS config is used"*). **Both are replaced, not one.**
- [ ] **`quic_test.go:237`** — the *"returns `rt.defaultChain.tlsCfg` BEFORE consulting `chainByName`"*
      sentence. Re-locate by LITERAL text; the `quic.go:56-58` anchor inside it is now wrong twice over.
      **Lead with what survives**: the ADR-0318 conclusion is unaffected; only the mechanism sentence
      changes.
- [ ] **`manager_test.go:977`** — `mkQUICListenerDefaultChain`'s doc. §0.5: the CONCLUSION survives (with
      zero `filter_chains[]` the Start-time selection over an empty `chainSpecs` still returns
      `defaultSpec`), the REASON does not. Rewrite the reason, keep the conclusion.
- [ ] 🔴 **`manager.go:396`** — the same sentence in `registerListenerMetrics`'s doc (§0.5 site 2). This
      file is on `SPEC.md` §11's BYTE-UNTOUCHED roster and the roster and the reconciliation set
      INTERSECT here. **Resolve it by editing the COMMENT and gating that the edit is comment-only:**
      ```sh
      git -C <wt> diff -- internal/listener/manager.go \
        | grep -E '^[+-]' | grep -vE '^(\+\+\+|---)' | grep -vE '^[+-][[:space:]]*//' | wc -l   # want 0
      ```
      ⚠️ **RUN THAT GATE OVER AN INPUT KNOWN TO TRIP IT** before believing its zero — add a throwaway
      executable line, confirm the gate reads non-zero, revert.
- [ ] **`BEHAVIOR_CONTRACT.md:1973`** is site 1 and lands at Task 21 with the ledger entry, because both
      edits are in the same file and one commit per file keeps the numstat legible.
- [ ] **State which hits you are deliberately LEAVING and why**: `internal/tls/config.go:620` and
      `manager.go:740` narrate the pre-phase-95 hole in the PAST tense and assert no precedence (§0.5).
      ⇒ **`internal/tls/**` stays byte-untouched.**
- [ ] `gofmt -l` empty · `go build ./...` rc 0 · full package green.
- [ ] Commit.

### Task 14: NC roster rows 1 and 2 — and row 2 is scored PER ARM

- [ ] **Row 1** — make the selector prefer `rt.defaultChain` first again. **Arm a must redden.**
      ⚠️ **Its evidence is Task 8's table**: this mutation reproduces the un-fixed tip exactly, so score it
      against that table arm by arm, not against a single pass/fail.
- [ ] **Row 2** — make the selector prefer `rt.chainSpecs[0]` unconditionally, the BRAINSTORM's refuted
      shape (`SPEC.md` §0.2). **Arms b AND c must redden.**
      🔴 **THIS IS THE ONLY FALSIFIABILITY ARMS b, e, g AND i HAVE** (§0.10) — they are GREEN at the
      un-fixed tip, so nothing else in this plan proves they do any work. **Score row 2 per arm and record
      which assertion fired in each.**
- [ ] ⚠️ **NEUTRALISE, NEVER REVERT** — the package must still compile, and the `-run` selector must
      actually match (a selector matching nothing prints `[no tests to run]` and EXITS 0).
- [ ] ⚠️ **BEFORE RUNNING EACH NC, NAME THE MECHANISM that would carry its mutation to a failure.** Phase
      96 found two roster rows whose specified mutation was structurally incapable of reddening. **If you
      cannot name one, the control is vacuous.**
- [ ] Restore, `sha256sum -c` the pre-NC digests, and re-run the package green.
- [ ] Commit the NC results into `PROGRESS.md`.

### Task 15: NC roster rows 3, 4 and 5 — each with a COMPANION THAT MUST STAY GREEN

- [ ] **Row 3** — drop `TransportProtocol: "quic"` from the inputs. **Arm d must redden; arm e must STAY
      GREEN.** If e also reddens the pair is not isolating what it claims.
- [ ] **Row 4** — drop `ApplicationProtocols: ["h3"]`. **Arm f must redden; arm g must STAY GREEN.**
- [ ] **Row 5** — drop `ServerName` from the per-connection inputs. **Arm h must redden; arm i must STAY
      GREEN** — i passes under a blank SNI too, which is exactly why h is the discriminating arm.
- [ ] ⚠️ **AN NC THAT LEAVES ITS COMPANION GREEN IS THE POINT, AND AN NC THAT LEAVES THE *TARGET* GREEN IS
      A FINDING.** Record both halves of every pair. A run where BOTH members redden means the pair is not
      isolating.
- [ ] ⚠️ **Row 5's mutation must not also break the handshake.** Dropping `ServerName` from the INPUTS
      struct is a selection change; it must not touch the client's `stdtls.Config`.
- [ ] Restore, digest-verify, re-run green. Commit.

### Task 16: NC roster rows 6 and 7

- [ ] **Row 6** — revert the `rt.addr` resolve move. **The port-0 arm of b must redden**: the Start-time
      selection sees `…:0` while the per-connection selection sees the resolved port, so a
      `destination_port` chain flips between the two moments.
      ⚠️ **THIS ROW DEPENDS ON TASK 3 HAVING BUILT A PORT-0 VARIANT.** If Task 3 chose the
      drive-through-`Start` option instead, **say so and re-derive what row 6 can still falsify** — a
      roster row whose target arm does not exist is a vacuous control.
- [ ] **Row 7** — make `quicTLSConfig()` return the default slot unconditionally. **Arm j must redden**
      (the accessors re-diverge).
- [ ] Restore, digest-verify, re-run green. Commit.

### Task 17: Fixture `0122-quic-chain-selection` — directory, driver, and BOTH inline templates

- [ ] `mkdir test/fixtures/0122-quic-chain-selection/driver`. ⚠️ **`0122` IS FREE** — `grep -rn '0122'
      test/` returns two lines, neither a fixture (`0028-…/inputs/driver.go:67`'s `refLB2TestPort = 10122`
      is a substring, and `0014-http-csrf/expectations.yaml:64` is `ADR-0122`).
- [ ] **Copy `0104-http3-downstream-get`'s SHAPE, not `0121`'s.** `0104` ships **NO `.yaml` bootstrap
      files and NO `pki/` directory at all** — both bootstraps are `const referenceTmpl` / `const
      subjectTmpl` string literals inside `driver/driver.go`, with the cert and key delivered
      `inline_string:` from `internal/listener`'s existing `testAlphaCertPEM` / `testAlphaKeyPEM` pair
      (SAN `alpha.envoy-go.test`, valid 2026-2046). **`0122` copies that. It ships no PKI.**
- [ ] **Reference port `15122`.** ⚠️ **CENSUS THE BAND, DO NOT INHERIT IT** (`SPEC.md` §0.8): **28 distinct
      `15xxx` literals** live under `test/fixtures/*/driver` and `*/inputs`, spanning `15000`-`15011`,
      `15042`-`15056` and `15104`, plus `15360`. `15104` is the only one at or above `15100`. The
      convention that produced it is `15000 + <fixture index>` — **a derived observation NO document
      states, recorded here rather than assumed.** `grep -rn '15122' test/` reads **0**.
- [ ] **Shape: ONE listener** (§3 — the harness supports exactly one UDP listener per fixture, now proven
      by mechanism). `filter_chains[0]` with **NO `filter_chain_match`**, its own
      `envoy.transport_sockets.quic` transport socket, HCM `stat_prefix: chain_indexed`, `direct_response`
      status **222** with a body naming the chain; PLUS a `default_filter_chain` with its **OWN** QUIC
      transport socket and `stat_prefix: chain_default` and a different body.
- [ ] 🔴 **THE TWO `stat_prefix` VALUES MUST DIFFER, AND THAT IS LOAD-BEARING, NOT COSMETIC.** Two HCMs
      sharing a `stat_prefix` — in one listener across two filter chains, exactly this shape — panic
      envoy-go at boot with `panic: stats: duplicate metric registration:
      "http.<prefix>.downstream_rq_total"`, rc=2 in 0 seconds (`BRAINSTORM.md` §4.1, measured on both
      paths). **This fixture sits one identifier away from the strongest banked candidate on the roster.
      Say so in the README.**
- [ ] ⚠️ **`BackendCount()` MUST BE >= 1** even though the route is a pure `direct_response`: the runner
      rejects 0, and envoy-go boot-rejects a config with no `clusters` key. The subject template carries a
      throwaway STATIC cluster the route never references — `0104`'s constraint, inherited by measurement.
- [ ] ⚠️ **`HTTPExpectations` IS TCP-ONLY.** The H3 arm drives through the driver's own `DriveReference` /
      `DriveSubject` hooks, on the `0104` precedent.
- [ ] ⚠️ **The reference container cannot read `filename:` certs** — `inline_string:`, indented ONE level
      deeper than the key, and NO PEM substitution inside a YAML comment.
- [ ] ⚠️ **The reference template hard-codes `0.0.0.0:15122`; the subject template takes `%d`
      placeholders** in `0104`'s field order (admin, listener, cluster). `SubjectConfig`'s
      `fmt.Sprintf` argument order must match the template's field order.
- [ ] Add `ReferenceListenerIsUDP() bool { return true }` and the compile-time assertion
      `var _ fixture.ReferenceListenerIsUDP = <driver>{}` — `0104` is the ONLY other implementor in the
      tree and both its declaration and its assertion are the template.
- [ ] Commit the directory.

### Task 18: Fixture `0122` — `expectations.yaml` and `README.md`

- [ ] **Assertions: a NAMED SUBSET, never a whole map.** Measured at the SPEC on arm A: the reference
      emits **156** lines across the two HCM scopes while envoy-go emits **10** — exactly
      `http.{indexed,default}.downstream_rq_{2xx,3xx,4xx,5xx,total}` — and the reference ADDITIONALLY
      emits a listener-qualified `listener.<addr>.http.<prefix>.*` scope of **12** lines that envoy-go
      does not emit at all. ⚠️ **AND THE LISTENER ADDRESS TOKEN DIFFERS CROSS-SIDE** (`0.0.0.0_…` vs
      `127_0_0_1_…`), so any assertion keyed on the full name is cross-side infeasible.
- [ ] ⇒ **Pin exactly TWO names per side**: `http.chain_indexed.downstream_rq_total` **>= 1** and
      `http.chain_default.downstream_rq_total` **== 0**, plus the response body via the runner's byte
      comparison.
- [ ] ⚠️ **`== 0` ON A NAME THE SUBJECT NEVER EMITS IS SILENTLY VACUOUS, NOT RED** — a scrape map returns
      the zero value for a missing key. **Prove BOTH names EXIST on BOTH sides before pinning either**
      (`SPEC.md` §0.9 / method note 46 / [[reference_sds_empty_ack_narrow_classifier]] class). If
      `chain_default` is absent from the subject scrape rather than present-at-zero, the pin must assert
      PRESENCE separately or be dropped and said to be dropped.
- [ ] `README.md` on the `0104` shape (**131** lines) plus this row's specifics: the twelve-arm reference
      table's arm A row, the `stat_prefix` collision hazard of Task 17, and the reason the INELIGIBLE arm
      does not ride here (§3 — one fixture directory dispatches to exactly one runner branch, and the
      harness supports one UDP listener).
- [ ] ⚠️ **Name what is deliberately NOT asserted, in a CLOSED enumeration**, so a name in neither list
      reads as asserted: no `listener.<addr>.http.*` scope (the subject has none), no `ssl.*`, no
      histogram, no `tracing.*`.
- [ ] Commit.

### Task 19: The FOUR registration gates, PROVEN and NC'd

- [ ] **Gate 1** — `fixture.RegisterFixture("0122-quic-chain-selection", &…{})` in the driver `init()`.
- [ ] **Gate 2** — the blank import in `test/differential/runner_test.go`.
- [ ] **Gate 3** — **byte-identity between the directory name and the registered string.** Assert it
      mechanically, not by eye.
- [ ] **Gate 4** — the `NNNN-` / `NNNNa-` directory-name shape `discoverFixtures` enumerates.
- [ ] ⚠️ **ALL FOUR SKIP RATHER THAN FAIL** (`SPEC.md` §0.9). Gates 1-3 converge on a `t.Skipf`; **gate 4
      produces no subtest at all — not even a skip line.** ⇒ **SCORE ON THE FIXTURE-SET SET-DIFFERENCE,
      NEVER ON THE EXIT CODE.**
- [ ] Run the extractor and both `comm` directions:
      ```sh
      extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
        | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
      ```
      against `ls -d test/fixtures/*/ | sed 's#test/fixtures/##; s#/$##' | sort`. **Expect 124 = 124, both
      directions EMPTY.** Baseline at this tip: **123 = 123**, split **99 `driver/` + 24 `inputs/`**.
- [ ] ⚠️ **NC THE EXTRACTOR BY RENAMING ONE IMPORT IN A SCRATCH COPY.** ⚠️ **A COUNT-ONLY CHECK IS
      VACUOUS — the import count is INVARIANT under a rename**, so the rename control is what "both
      directions" belongs to. ⚠️ **A pure DELETION fires only `comm -23`.**
- [ ] ⚠️ **`ls -d test/fixtures/*/ | wc -l` is the correct counter.** The plausible `^[0-9]{4}-` character
      class reads **121**, dropping `0007a` and `0007b`.
- [ ] Run the FULL differential suite with `-count=1` (~400s, FOREGROUND — a subagent's background Bash
      dies at the turn boundary). Assert the fixture set BY NAME in both directions, **123 -> 124**.
- [ ] ⚠️ **CHECK FOR SIBLING SESSIONS BEFORE BLAMING THIS ROW FOR A PORT FLAKE**, and never tear down a
      container this session did not create — BY NAME only. A `reaper_*` Ryuk container is the
      differential's own and is REUSED.
- [ ] Commit.

### Task 20: NC roster row 8 — the gate that is SILENTLY GREEN if missed

- [ ] Delete the `0122` blank import from `test/differential/runner_test.go`, run the suite, and observe
      that it **STAYS GREEN**: the mutation produces a `t.Skipf`, so rc=0 with no FAIL line is exactly
      what a missing gate looks like.
- [ ] 🔴 **SCORE IT ON THE FIXTURE-SET SET-DIFFERENCE, NOT ON THE EXIT CODE.** `comm -23` must now name
      `0122-quic-chain-selection`. **This is the roster row most likely to be mis-scored.**
- [ ] Restore, re-run, both directions EMPTY again.
- [ ] ⚠️ **DIFF THE ARM ROSTER — a, b, c, d, e, f, g, h, i, j and NC rows 1-8 — not the counters.** A
      `+0/+0` arm can be silently deleted with every gate staying green.
- [ ] Commit the NC results into `PROGRESS.md`.

### Task 21: `ADR-0319` completed IN PLACE, and `BEHAVIOR_CONTRACT.md` — TWO edits, ONE file each

- [ ] **`DECISIONS.md`** — append §Decision and §Consequences to ADR-0319 **AFTER the RETAINED italic
      footer**, never replacing it; flip `> **STATUS: PROPOSED` to `ACCEPTED` **in place**. ⚠️ **NO
      renumber, NO `**Status:**` line, and NO `---` separator** — `^---$` must not move from **216**.
      **This DISARMS the house guard**; verify BY LINE and BY ADR (backward `^## ADR-` heading search),
      never by a count, and **write no count of either matcher in prose the grep matches** — the phase-93
      SPEC falsified itself doing exactly that.
- [ ] ⚠️ **RE-TENSE ADR-0319 §Context's `quicTLSConfig` sentence** (§0.5 site 5): it describes the DEFECT
      and is correct as drafted, but reads as a present-tense claim once §Decision lands. **Lead with what
      survives.**
- [ ] ⚠️ **ADR-0044 DOES NOT CONTAIN THE DRAFTING DISCIPLINE THIS PROJECT CITES IT FOR** — confirmed by
      reading it in full at the phase-97 SPEC; it is about the `BEHAVIOR_CONTRACT` HTTP/1.1 subsection.
      The real discipline is the shared ADR-0294-through-0319 block form. **Do not cite ADR-0044 for it,
      and quote NO count of the misattributing lines** — any figure would be falsified by its own landing.
- [ ] **`BEHAVIOR_CONTRACT.md`, edit 1** — the `Phase 97 — +0, UNCHANGED` ledger chain entry, on the
      **phase-96 form at `:5141`** and NOT the sibling `A → B` form (§0.4). ⚠️ **QUOTE NO ABSOLUTE**:
      three mutually inconsistent stat-surface absolutes are live in this tree at one tip, and on a
      contested count the answer is **no number**. Enforcement remains the per-phase `TestNoNewStat*`
      delta guards.
- [ ] **`BEHAVIOR_CONTRACT.md`, edit 2** — `:1973`, the *"`quicTLSConfig()` (`quic.go:56-58`) returns
      `rt.defaultChain.tlsCfg` FIRST, before consulting `chainByName`"* sentence (§0.5 site 1). **Both
      halves are now false and the `:56-58` anchor is wrong twice over.** ⚠️ **LEAD WITH WHAT SURVIVES:**
      the ADR-0318 conclusion — the five `ssl.*` names are registered and permanently zero on QUIC, and
      that is PARITY — is untouched by this row. Only the mechanism sentence changes. ⚠️ **Re-locate by
      LITERAL text; `:1973` is a single 40k+ character line.**
- [ ] ⚠️ **The row lands +0 stat NAMES.** It changes which chain serves, not which counters exist.
- [ ] Commit.

### Task 22: `ROADMAP.md` — row 97 -> `done`, under the field-count gate

- [ ] Flip row 97's status cell `in-progress` -> `done` and fill the summary with the IMPL's own result.
- [ ] 🔴 **COUNT FIELDS UNDER BOTH FORMS, BEFORE AND AFTER — want 8.**
      ```sh
      awk -F'|' 'NR==159{print "naive NF="NF}' docs/envoy-go/ROADMAP.md
      sed 's/\\|//g' docs/envoy-go/ROADMAP.md | awk -F'|' 'NR==159{print "escape-aware NF="NF}'
      ```
      Baseline at this PLAN's tip: **8 under BOTH** (§2.3). ⚠️ **An unescaped `|` passes check (1) and
      silently breaks the field count**, and it fired against the phase-96 IMPL's own author, who wrote a
      Go `||` into row 96's narrative. **Reword a pipe away rather than escaping it.**
- [ ] 🔴 **NEVER SPELL EITHER SENTINEL MATCH PHRASE IN THE NEW SUMMARY CELL.** Check (2) matches the WHOLE
      FILE, so a seventh hit would read as a finding. **The margin is ONE** (§2.3).
- [ ] Re-run all three sentinel checks + all four NCs + the check-(2) positive control. **Check (1) should
      now be SILENT** (this row was its only voice); check (2) still **SIX**, byte-identical digests;
      check (3) SILENT. **NC-A and NC-B each now read ONE line, not two** — ⚠️ **the NC shapes CHANGE at a
      row flip; do not inherit this PLAN's TWO.**
- [ ] ⚠️ **THE `+0 fixtures` CELL IS WRONG FOR THIS ROW** — it lands **+1** (`0122`). State the delta the
      row actually landed.
- [ ] **FOLD-IN, cheapest on the roster:** `ROADMAP.md:136` and `:139` carry the same stale
      swallowed-panic claim as the unfixable `:229` (§0.9) and sit **OUTSIDE every sentinel window**.
      Repair those two, leave `:229`, and say which you left and why.
- [ ] Commit.

### Task 23: The byte-untouched roster, and the SIX-GATE sweep

- [ ] **Byte-untouched roster, asserted by `sha256sum` and SET-DIFFERENCED against the edit roster:**
      `internal/listener/listenerfilter/**`, `internal/tls/**`, `internal/stats/**`, and
      `test/fixtures/0104-http3-downstream-get/**`.
      🔴 **`internal/listener/manager.go` IS NO LONGER ON THIS ROSTER** (§0.5) — it moves to the edit
      roster under Task 13's comment-only gate, and that gate is what replaces the digest here.
- [ ] Capture the digests BEFORE any edit and verify with `sha256sum -c` at the close. ⚠️ **Assert the
      roster is a PARTITION**: no path may appear on both rosters, and the union must cover every file the
      diff touches. Compute the set difference mechanically, not by reading.
- [ ] **THE SIX GATES — NAME DEPARTURES, DO NOT CLAIM COMPLIANCE:**
      - **(a) Differential** — the FULL suite, `-count=1`, fixture set asserted BY NAME in both `comm`
        directions, **123 -> 124**. FOREGROUND.
      - **(b) Non-Docker sweep** — `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`,
        gated on `PIPESTATUS[0]` **plus a SET RECONCILIATION**. ⚠️ **BOTH Docker drivers excluded.**
        Baseline at this tip: `go list ./...` **239**, **237** after the exclusion.
      - **(c) h2spec** — `95 tests, 94 passed, 1 skipped, 0 failed`, the skip being 6.9.2/2, invariant.
      - **(d) fuzzers** — **56 targets / 48 FILES** at this tip, anticipated **+0** (the row consumes no
        new config field, so there is no new parse arm). ⚠️ **48 is FILES and 56 is TARGETS.**
      - **(e) The ANCHORED panic gate** `^panic:|DATA RACE|SIGSEGV`, **0** *and PROVEN LIVE* at Task 1.
      - **(f) No `REVIEW.md`** — a STANDING DEPARTURE, named.
- [ ] ⚠️ **`-race` ON THE DIFFERENTIAL SUITE IS VACUOUS** — the subject there is an unraced subprocess.
      The race gate is Task 12's full-package run.
- [ ] ⚠️ **`go mod tidy -diff` EMPTY and `git diff -- go.mod go.sum` EMPTY.** The row adds no sub-package,
      so `reference_new_subpackage_pulls_transitive_module` does not bite — **re-check anyway.**
      `go.mod` require entries at this tip: **67** under a structural `awk` extractor.
- [ ] `golangci-lint` over the touched packages, with British spellings swept out of `.go` comments first
      (its misspell runs in locale **US**).
- [ ] Commit the gate transcript into `PROGRESS.md`.

### Task 24: `PROGRESS.md`, `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt` — the close

- [ ] `PROGRESS.md` — the full task transcript, the RED/GREEN roster of Task 8 beside Task 12's, the NC
      results, the six-gate output, and **every claim this IMPL refuted by execution**.
- [ ] `STATE.md` — rolled **IN PLACE**, §Current pointer edited, never prepended. Evict the oldest
      §Recent entry using the **LABEL-BOUND PAIR on BOTH files**, with a **fabricated-label NC** reading 0
      in both and a **positive control on a label that IS in the archive**. ⚠️ **The bare forms answer
      nothing.** ⚠️ **Roll the §Recent PREAMBLE SENTENCE too, without spelling the evictee's label.**
- [ ] `STATE_HISTORY.md` — **ONE INLINE LINE** in the PARENTHETICAL form. **Strict guard DELTA 0**, raw
      delta **+2** (a blank line PLUS the entry line). ⚠️ **Name NO positive-control figure in the archive
      line** — the archive's controls are self-incrementing.
- [ ] `next-prompt.txt` — rolled, `git add -f` (TRACKED but gitignored).
- [ ] ⚠️ **RE-DERIVE EVERY LINE COUNT YOU QUOTE IN THE SAME COMMIT AS THE EDIT THAT MOVES IT.** A section
      headed *"re-derived at this stage's own tip"* is false if it names a commit that is not the one it
      ships on.
- [ ] ONE squashed commit, merged and pushed, **with the FULL SLUG *AND* the STAGE WORD in its subject** —
      phase 95's IMPL landed with no stage token at all, so `^phase 95 (.*) IMPL` reads ZERO.

---

## 7. The negative-control roster, CORRECTED

`SPEC.md` §12's eight rows are carried forward with **three corrections and one addition**, each derived
in §0 and each stated as a mechanism rather than a preference.

| # | neutralise | arm that must redden | companion that must STAY GREEN | correction |
|---|---|---|---|---|
| 1 | prefer `rt.defaultChain` first again | **a** | — | ⚠️ **its evidence is Task 8's table** and cannot be reproduced after Task 9 — score it arm by arm |
| 2 | prefer `rt.chainSpecs[0]` unconditionally | **b, c** | — | 🔴 **this is the ONLY falsifiability arms b, e, g and i have** (§0.10). Score PER ARM. |
| 3 | drop `TransportProtocol: "quic"` | **d** | **e** | — |
| 4 | drop `ApplicationProtocols: ["h3"]` | **f** | **g** | — |
| 5 | drop `ServerName` from the per-connection inputs | **h** | **i** | ⚠️ mutate the INPUTS struct, not the client's `stdtls.Config`, or it breaks the handshake instead of the match |
| 6 | revert the `rt.addr` resolve move | the **port-0 variant of b** | — | ⚠️ **depends on Task 3 having BUILT that variant.** If Task 3 chose the drive-through-`Start` shape instead, re-derive what row 6 can still falsify — **a roster row whose target arm does not exist is a vacuous control.** |
| 7 | `quicTLSConfig()` returns the default slot unconditionally | **j** | — | ⚠️ **arm j must be built on the PLAINTEXT-default, ELIGIBLE-indexed shape** (§0.11), or row 7 is scoring a vacuous arm |
| 8 | delete the `0122` blank import | the fixture-set NAME check | — | 🔴 **the suite STAYS GREEN.** Score on the set-difference, never on rc. |
| **9** | **NEW: make `quicChainMatchInputs` return the zero struct for `conn != nil`** | **a, d, f, h** | **b, e, g, i, c** | proves the per-connection builder is REACHED at all. Without it, rows 3-5 each prove one FIELD is read while nothing proves the STRUCT is built on the connection path. |

⚠️ **BEFORE RUNNING ANY ROW, NAME THE MECHANISM THAT CARRIES ITS MUTATION TO A FAILURE.** Phase 96 found
two roster rows whose specified mutation was structurally incapable of reddening. **If you cannot name
one, the control is vacuous** ([[reference_nc_mutation_cannot_reach_a_failure]]).

⚠️ **NEUTRALISE, NEVER REVERT.** The package must still compile, and every `-run` selector must actually
match — one that matches nothing prints `[no tests to run]` and **exits 0**.

⚠️ **AN NC THAT LEAVES A COMPANION GREEN IS THE POINT; AN NC THAT LEAVES ITS TARGET GREEN IS A FINDING.**
Record both halves of every pair, and a run where BOTH members of a pair redden means the pair is not
isolating what it claims.

---

## 8. Counts — RE-DERIVED AT THIS PLAN'S OWN TIP

Every figure below was produced by running its command in the commit that ships these edits, not copied
forward. ⚠️ **A section headed "re-derived at this stage's own tip" is FALSE if it names a commit that is
not the one it ships on** — a stage's own edits are part of its tip.

### 8.1 Figures this stage MOVES

| file | before | after | note |
|---|---|---|---|
| `PLAN.md` | — | **1555** | new; re-derived by `wc -l` at the publishing tip, in the same commit that writes this cell |
| `STATE.md` | 65 | 65 | rolled IN PLACE, net 0 |
| `STATE_HISTORY.md` | 566 | 568 | raw delta **+2** — a blank line PLUS the entry line |
| archive strict | 163 | **163** | ⚠️ **DELTA 0**, as a correctly-shaped parenthetical append must be |
| archive parenthetical | 70 | 71 | |
| archive loose | 233 | 234 | **163 + 71 = 234** exactly, under the anchored-occurrence forms |
| `next-prompt.txt` | 279 | rolled | `git add -f` |

### 8.2 Figures this stage must NOT move — the baseline the close proves against

`ROADMAP.md` **247** lines / **129** data rows / row 97 `in-progress` at file line **159**, field count
**8 under both forms** · `BEHAVIOR_CONTRACT.md` **5991** · `DECISIONS.md` **19080**, `^---$` **216**,
`^## ADR-` **318**, bare `^## ` **326**, tail **ADR-0319**, next-free **ADR-0320** (`^## ADR-0320` reads
**0**) · the house `PROPOSED` guard **ARMED**, its single hit at `DECISIONS.md:19054` resolving by
BACKWARD heading search to `## ADR-0319` · the ADR-0231 decoy at `:14866`, resolving to `## ADR-0231` ·
**ZERO production `.go` bytes.**

⚠️ **NEVER DERIVE next-free FROM THE HEADING COUNT** — the id space is sparse at the single `0209` gap, so
heading arithmetic yields a TAKEN id. ⚠️ **AND THE HEADING REGEX `^## ADR-[0-9]+[:—]` IS ITSELF HOLED** by
`## ADR-0127 v2`, which carries a version suffix between the id and the colon. **Derive from the TAIL.**
⚠️ **DO NOT QUOTE A COUNT FOR EITHER `PROPOSED` MATCHER IN PROSE THE GREP MATCHES.**

### 8.3 Unmoved elsewhere, re-derived here

`BRAINSTORM.md` **396** · `SPEC.md` **874** · phase dirs **138** · fixtures **123**
(`ls -d test/fixtures/*/ | wc -l`; ⚠️ a `^[0-9]{4}-` character class reads **121**, dropping `0007a` and
`0007b`), tail `0121-listener-default-chain-tls`, **`0122` FREE** · blank-import extractor **123 = 123**,
both `comm` directions EMPTY, split **99 `driver/` + 24 `inputs/`** · fuzzers **56 targets / 48 FILES** ·
`go.mod` **67** require entries under a structural `awk` extractor — ⚠️ **the named character-class form
`^\s+[a-z0-9./-]+ v[0-9]` reads 62 at this tip**, and the gap is exactly five lines the class cannot
spell: one underscore (`client_model`) and four uppercase initials (`AdaLogics`, `Azure`, `Microsoft` ×2)
· `go list ./...` **239**, **237** excluding the two Docker drivers · BackendKind tail **38**
(`H2GoawayResponder`, `test/differential/fixture/fixture.go:614`) · `-family row` **96 occurrences / 68
lines** (⚠️ pass `--` before the pattern) · `internal/listener/quic.go` **168** ·
`internal/listener/manager.go` **1649** · `internal/listener/manager_test.go` **6467** ·
`internal/listener/quic_test.go` **326** · `internal/listener/quic_negative_test.go` **150**.

**Anticipated axis deltas for the IMPL, each an ANTICIPATION and not a measurement:** stat NAMES **+0** ·
fixtures **123 -> 124** · BackendKinds **+0** (the fixture uses the in-process branch and implements
neither `BackendKindAware` nor `PerHostBackendKind`) · fuzzers **+0** · `go.mod` modules **+0** · new
packages **0** · ADRs **+1**, completed in place.

**Anchors this row WILL move**, to be re-located by LITERAL text and never by a scalar shift: every
`quic.go` line number in this document, in `SPEC.md`, in `BRAINSTORM.md`, in `BEHAVIOR_CONTRACT.md:1973`,
in `manager.go:396` and in `quic_test.go:237`. ⚠️ **A MULTI-INSERT LINE SHIFT IS BANDED, NOT A CONSTANT.**

### 8.4 The one self-referential figure

`PLAN.md`'s own line count is the only figure in §8.1 that this document cannot state before it is
finished. **It is patched in after the last section is appended and re-verified with `wc -l` immediately
before committing** — the discipline the phase-97 SPEC used on its own **874**.

---

## 9. Cost — MEASURED and ESTIMATED, labelled separately

| axis | value | label |
|---|---|---|
| `internal/listener/quic.go`, the five-point edit | **+87 / -23** | **MEASURED** — built, run against twelve arms, reverted |
| everything else in §1.2's table | ≈ **+1131 / -28** `.go`, ≈ **+300** YAML/Markdown | **ESTIMATED** |
| `.go`-only total | ≈ **+1218 / -51** | estimate over a measured core |
| phase 96 `.go`-only, MEASURED | **+1285** | `git show --numstat c7bd2880` |
| phase 94 `.go`-only, MEASURED | **+1143** | `git show --numstat 0a985a35` |

⚠️ **THE ESTIMATE IS A FLOOR.** [[reference_measured_prototype_is_a_lower_bound]] has fired **seventeen
consecutive rows**. At phase 96 it INVERTED a comparison rather than widening a range; at phase 97 it
killed the predecessor's prototype outright, growing the floor 12x AND changing its shape. **If the IMPL's
actual net change crosses `~1500` on the `.go` reading, `BOOTSTRAP_PROMPT.md` §6.1's mid-execution clause
is the remedy — not a retroactive re-reading of §1.2's verdict.**

---

## 10. Deferred — surfaced by THIS PLAN, none chartered

- 🔴 **`sniMatchAny`'s `*.` SUFFIX IS MULTI-LABEL, AND THE REFERENCE'S IS NOT.**
  `chainmatch.go:278-290` implements `"*.foo.test"` as `strings.HasSuffix(sni, ".foo.test")`, so
  `a.b.foo.test` MATCHES. Envoy's `server_names` wildcard is single-label. ⚠️ **UNMEASURED against a live
  reference, and it affects the TCP path as much as the QUIC one.** Surfaced while reading the matcher for
  arms h and i; **not touched by this row**, whose arms use exact names precisely so they do not depend on
  it.
- ⚠️ **A SECOND QUIC ALPN WOULD SILENTLY BREAK THE STAMPED CONSTANT** (§0.14). `ApplicationProtocols` is a
  listener constant on QUIC because `crypto/tls.ConnectionState.NegotiatedProtocol` is a scalar. Correct
  today because the QUIC config's `NextProtos` is exactly `["h3"]`; **a row that adds a second value must
  revisit this, and nothing will fail if it does not.**
- ⚠️ **`catchAllCount`'s ERROR MESSAGE NAMES `server_names` WHILE ITS PREDICATE IS `spec.Empty`** — all
  eight dimensions (§0.13). A one-line message fix, outside every sentinel window. **Recorded, not
  repaired.**
- ⚠️ **`ROADMAP.md:136` AND `:139` CARRY THE SAME STALE SWALLOWED-PANIC CLAIM AS `:229`** and sit OUTSIDE
  every sentinel window (§0.9). **Assigned to the IMPL at Task 22** as the cheapest fold-in on the roster;
  `:229` stays.
- ⚠️ **THE DIFFERENTIAL HARNESS CANNOT CARRY TWO UDP LISTENERS, OR A MIXED TCP+UDP PAIR** (§3), and the
  `ReferenceLogMounter` branch WINS over `refIsUDP`. Three named edits would close it. **A future row's
  first task, not this one's.**
- ⚠️ **`freeTCPPortBlock` PROBES ONLY TCP** (`harness_test.go:281-316`), so a derived port used for a QUIC
  listener is never UDP-probed. A flake risk on any future multi-QUIC fixture.
- **Everything `SPEC.md` §9 banks, unchanged and unchartered:** per-connection TLS *identity* dispatch
  (§4.4's residual, bounded above by §2 arm H and UNMEASURED on the certificate axis, and reconcilable
  only against ADR-0317 `D-ALPNFB-TCPONLY`) · **D2-QUICTS**, reference-confirmed with its exact message
  `no transport socket specified for connection oriented UDP listener`, failing CLOSED — and **consumed as
  a test fixture by arm j without being repaired** (§0.11) · `source_type` / `source_prefix_ranges` /
  `source_ports` on QUIC, where the mechanism now covers them for free (§0.12) but the reference was never
  measured · `parseChainSpec`'s `transport_protocol` over-strictness · the `0066`/`0067`/`0068`/`0071`
  dead-port false-green · envoy-go's missing `listener.<addr>.http.<prefix>.*` scope · duplicate listener
  ADDRESSES · the driver-owned receiver port race · the SDS × `alpn_protocols` fixture · ADR-0025's stale
  `Accepted` status and the ADR-0044 misattribution · the HCM `stat_prefix` duplicate-registration panic,
  still the strongest banked candidate and **one identifier away from this row's own fixture** (Task 17).

---

## 11. `SPEC.md` §14 coverage — every owed item

| # | owed | discharged where | how |
|---|---|---|---|
| 1 | derive the task count; evaluate the §6.1 split gate and record it either way | **§1.2** | **24 tasks**, derived. Gate EVALUATED, **NOT tripped**, against two MEASURED structural precedents (`c7bd2880` **+1725**, `0a985a35` **+1635**, neither split) — **and the margin is stated: ONE task, and the widest LoC accounting is already over `~1500`.** |
| 2 | order the spine so the un-fixed tip is SPENT as the negative control | **§6, Tasks 1-8** | Tasks 1-8 land and RUN at the un-fixed tip; Task 8's table IS NC row 1's evidence and cannot be reproduced after Task 9. ⚠️ **AND §0.10 REFUTES THE ASSUMPTION UNDERNEATH THE INSTRUCTION** — four of the ten arms are GREEN at that tip, structurally. |
| 3 | verify the multi-UDP-listener question and record the answer either way | **§3** | **ANSWERED: NOT supported.** Mechanism named (`runFixture`'s transport-blind, single-valued address dispatch at two sites), three repair edits named, and the NON-causes ruled out (publishing and readiness are both fine). |
| 4 | read the tests before writing tasks against them; re-derive §5's structure figures | **§4**, **§0.1**, **§0.2**, **§0.15** | Full per-file structure table under FOUR matcher forms. ⚠️ **The SPEC's own diagnosis is REFUTED** — the zero comes from a SPACE, not a following-line brace, so the prescribed matcher would have reproduced it. Both QUIC files are table-FREE. `quicChain()` has ZERO test call sites. |
| 5 | re-derive every figure at the PLAN's own publishing commit | **§8** | Every figure re-run at this tip; the two precedent cost figures re-derived FIRST-HAND rather than inherited from the phase-96 PLAN's table. |
| 6 | do NOT inherit the NC shapes | **§2.1, §2.2** | `want` **129**, NC-A **TWO**, NC-B **TWO**, check (2) **SIX**, NC-C FIRED, NC-D **96 / 68**, PC **6 / residual 0** — all RUN, not inherited. **And §2.3 records that the shapes CHANGE at Task 22's row flip.** |
| 7 | land `PLAN.md` only; `ROADMAP`, `BEHAVIOR_CONTRACT`, `DECISIONS` byte-untouched; guard stays ARMED | **§1.1**, **§0.3** | Scope MEASURED across FIVE precedents: **FOUR files**, not one. The four NEGATIVES hold and are enforced; the guard stays ARMED; `ADR-0319` stays `PROPOSED`. |
| 8 | carry §2's reference table forward INTACT, §2.5 included | **§6 Tasks 17-18, §10** | The twelve-arm table is NOT re-derived. Arm A's measured stat surface drives Task 18's named subset; arm E drives arm **c**; arms F/F2 and G/G2 drive the stamped constants and arms d/e and f/g; arm H drives arms h/i. **§2.5's NOT-RUN arms are carried in §10 as banked, not as a footnote** — per-chain distinct certificates, `source_type`/`source_*`, and out-of-domain `transport_protocol` values. |

---

## 12. Self-review — run against the SPEC with fresh eyes

- **Does every task end in a commit?** Yes, 24 of 24.
- **Does any task carry more than ~10 sub-steps?** ⚠️ **YES — Tasks 17 and 19, at ELEVEN each.**
  MEASURED, not eyeballed; the full histogram is in §1.2. **This is stated as a threshold crossed, not
  as a clean pass**, with the structural argument for why it does not force a split written beside it.
  ⚠️ **An earlier draft of this very line said "the largest are Tasks 17 and 23 at nine" and was FALSE
  at the commit that would have published it** — the same defect class §0.4 and §0.3 catch in the SPEC,
  found here by running `awk` over this document instead of trusting its author.
- **Does every arm have a matched negative?** Yes — a/none (the headline), b+c, d+e, f+g, h+i, j/NC-7.
  ⚠️ **Arm a is the one arm WITHOUT a matched negative, and that is deliberate**: its negative is the
  un-fixed tip itself, spent at Task 8 and re-run as NC row 1.
- **Is any assertion structurally incapable of failing?** The four GREEN-at-tip arms are, until NC row 2
  runs (§0.10). **That is stated at the arm, in the roster, and in the task.**
- **Is any gate read as zero without being proven live?** No — Task 1 proves the panic gate, Task 13
  proves the comment-only gate, Task 19 NCs the extractor, Task 20 NCs the blank import.
- **Does any figure lack its matcher?** Checked; §8 names one per figure and §4 names four for the table
  question alone.
- **Does the PLAN edit anything outside its four files?** No. §1.1 is the roster and §8.2 is the baseline
  that proves it.
- **Does the PLAN re-derive `SPEC.md` §2, §4 or §10.2?** No. §11 item 8 records exactly which downstream
  decisions each arm feeds.
- **What would a reviewer most likely overturn?** §1.2's split verdict, on the widest LoC accounting
  (≈ **+1518**), which is already over `~1500`. **The counter-argument is stated rather than hidden: the
  two measured structural precedents are LARGER on the like-for-like accounting and neither was split, and
  the only available seam defeats the row in both directions.** A reviewer who weighs the absolute
  threshold above the precedent should split into `97.1` (production + unit arms) and `97.2` (fixture
  `0122`) — and should read §1.2 ground 2 first, because `SPEC.md` §0.6 measured that `97.1` would ship
  with a suite that is green under a patch reversing the very behaviour it lands.
