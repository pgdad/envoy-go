# PLAN 75 — the downstream TLS `ssl.no_certificate` success-path annotation at listener scope — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — **ZERO production `.go`**; the only two files in the phase-directory delta are this `PLAN.md` and `PROGRESS.md`. Worktree `/home/esa/git/envoy-go-wt-p75plan`, branch `phase-75-plan`, tip **`cedd2f27`** (the phase-75 router commit — master), per `feedback_git_worktrees`.
>
> **Row 75 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, `reference_roadmap_split_phase_row_done`). **ADR-0297's §Context is ALREADY DRAFTED** at the SPEC squash (`DECISIONS.md:17322`, STATUS **PROPOSED**, §Context only — **ELEVEN** body paragraphs ¶1-¶11 — closing with `*(§Decision + §Consequences land at the phase-75 IMPL.)*`). The IMPL **COMPLETES ADR-0297 IN PLACE**: append `### Decision` + `### Consequences` after the RETAINED footer. No renumber, no new ADR; tail stays **ADR-0297**, next-free **ADR-0298**.
>
> **Baselines RE-DERIVED at `cedd2f27` (`[RUN]` in this worktree, NOT copied):** fixtures **119** · fuzzers **55** · BackendKind tail **38** (the file declares **39** constants, 0-38 — a TAIL VALUE, not a count) · go.mod **67** requires (the tracked lineage figure is **2**) · stat surface **1204** (BEHAVIOR_CONTRACT doc count — **NO mechanical command**, and **not** re-derived) · DECISIONS tail **ADR-0297** PROPOSED, `grep -c '^## ADR-0298'` ⇒ **0** · ADR-0297 block shape `### Context` **1** / `### Decision` **0** / `### Consequences` **0** / footer **1** · `go build ./...` clean · `go test ./internal/listener/ ./internal/stats/ -count=1` **ok / ok**.
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 75`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — **cite the command, never the adjective**; the FOUR `candidates were:` recaps inside `:205` are HISTORICAL); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **All three RUN at THIS PLAN close, TWICE** (worktree + landed master post-push).
>
> **⚠️ NO PARALLEL STREAM.** `git diff --stat e822f1ad HEAD -- '*.go' go.mod go.sum test/` is **EMPTY** — the delta over the SPEC's derivation tip is four docs files (`DECISIONS.md` +30/-0, `STATE.md`, the new `SPEC.md`, `next-prompt.txt`). The production tree is byte-identical to what the SPEC re-derived, so §1 is a full independent re-verification rather than a drift sweep, and **any failing Go cite was wrong WHEN WRITTEN.** `feedback_parallel_stream_mints_fresh_drift` does not bite at this PLAN — **but re-run the check at the IMPL tip anyway.**
>
> **⚠️ RE-DERIVE, do not assume.** A PLAN is not evidence; **a SPEC is not evidence either** (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED. Take every `file:line` from THIS document or from SPEC 75 — never from the phase-65/67/74 documents, whose cites are one-to-many stages stale.
>
> **⚠️ THIS PLAN IS UNUSUAL: MOST OF IT WAS EXECUTED.** SPEC §16 closes with *"What NO ONE checked, stated plainly: **NO BUILD OF ROW 75 EXISTS.**"* That is no longer true. Three private build worktrees compiled the row, ran the full unit suite green, and executed **eight of the ten breaks** — including the two that **REFUTE the SPEC's and the router's central claim about where this row's discriminating break lives** (§1.2 RD-BREAK / §1.3). Where a step below says `[RUN]`, it happened; where it says `[IMPL]`, it is still owed.

---

## 1. Re-derivation ledger — every SPEC §3/§5/§7/§8/§9/§10/§11/§12/§14 anchor re-opened at `cedd2f27`

**All SPEC anchors RE-DERIVED at `cedd2f27` by FOUR read-only agents on disjoint remits** (A1: `internal/listener/manager.go` + `internal/stats/{name,registry}.go` + the import gate; A2: every TEST-side guard + the helper roster + the repo-wide guard sweep; A3: `0110` + `0111` + `runner_test.go` dispatch + the harness readiness legs; A4: `BEHAVIOR_CONTRACT.md` + `DECISIONS.md` + `ROADMAP.md`, each proposed edit APPLIED TO A SCRATCH COPY and mechanically verified), **plus THREE build-by-execution agents in private worktrees** (V1: the production change + the positive arm + `assertSSLCrossProduct` + six breaks; V2: the guard half + its breaks + the `0111` decision analysis; V3: the `0110` cross-side asserter against the LIVE reference), **plus controller re-verification of every load-bearing correction** (`reference_a_drift_correction_is_itself_a_claim` — the controller personally re-ran the collision greps, the parallel-stream diff, the sorted-position derivation, the `:962` offset in both units, the ledger byte-form, the two-ADR-heading population, the blind-spot row count, and **reproduced Break D and Break D′ end-to-end**).

**RESULT: FOURTEEN SPEC/router claims REFUTED and TWELVE new findings (F1–F12), FOUR of the findings SEVERE.** The single most consequential is **RD-BREAK**: the SPEC and the router both state that the new positive unit arm *is* this row's discriminating break, and **that is inverted** — the positive arm cannot detect the defect in question, and the guard that can is a one-line roster extension both documents treat as accommodation for the new arm rather than as the row's central defence. It was caught by building the row and running the break, not by reading.

### 1.1 The RD-* ledger

| # | Anchor / SPEC claim | RE-DERIVED at `cedd2f27` | Where |
|---|---|---|---|
| **RD-TREE** | Parallel stream since the SPEC's derivation tip `e822f1ad`? | **NO.** `git diff --stat e822f1ad HEAD -- '*.go' go.mod go.sum test/` ⇒ **EMPTY**. Full delta = 4 docs files (`DECISIONS.md` +30/-0, `STATE.md`, `SPEC.md` new, `next-prompt.txt`). ⇒ any failing Go cite was **wrong when written**. | §1 |
| **RD-IDENT** | SPEC §5: the drafted identifiers are free | **ALL FREE — controller-run repo-wide `--include='*.go'`:** `sslNoCertificate` **0** · `startOneWayTLSListener` **0** · `no_certificate` under `internal/` **0** · `no_certificate` across ALL `*.go` **0**. ⚠️ `no_certificate` has **104 hits under `docs/`** — a repo-wide grep WITHOUT `--include='*.go'` reads as a collision and is not one. New this PLAN: `sslLeafRoster` **0**. **Re-run all five at the IMPL tip** (T1 Step 0). | T1, T3 |
| **RD-FIELDS** | SPEC §11: the three `*stats.Counter` fields at `manager.go:180-182` | **HOLDS exactly.** `:180 sslHandshake` · `:181 sslFailVerifyError` · `:182 sslFailVerifyNoCert`; `:178 downstreamCxTotal`, `:179 downstreamCxActive`. The 4th field inserts **as new line `:183`**. | T1 |
| **RD-FIELDCOMMENT** | SPEC D4: the field-block comment `:172-177` says "three" | **HOLDS.** "three" at **`:175`** and **`:177`**; `:174` *"The two cx metrics"* **stays TRUE — do not touch it.** | T1 |
| **RD-REGDOC** ⚠️ | SPEC D4: `registerListenerMetrics`'s doc comment is `:358-373` | **REFUTED — it starts at `:351`.** Span is **`:351-373`**; `:358` is a *middle* sentence, so a task anchored on "358-373" edits a FRAGMENT. Exactly **ONE** token in the whole comment goes false — `:358` *"The **three** phase-74 ssl.\* counters"* — and it is wrong a **second** way: the 4th counter is phase-**75**, so it cannot simply become *"four phase-74"*. `:361` *"15+ other zero-valued names"*, `:365` *"the full ssl.\* family"*, `:354` *"two listeners"* all stay true. | T1 |
| **RD-REGFN** | SPEC §11: body `:374-383`, `prefix` `:375`, the three ssl calls `:379-381` | **ALL HOLD.** `:378 if rt.tlsMode {` · `:379-381` the three `r.NewCounter` · `:382 }`. The 4th registration inserts at `:382`, **inside the existing gate. NO new gate.** | T1 |
| **RD-INCSITE** | SPEC §3.1/§11: `tlsConn` declared `:1259`, `Inc` at `:1277`, `dispatchConn = tlsConn` `:1278` | **ALL HOLD, byte-exact.** Whole block **`:1258-1279`** (`var dispatchConn net.Conn = pkConn` `:1257`; chain dispatch `:1284`). `:1259 tlsConn := stdtls.Server(pkConn, selected.tlsCfg)` is a **plain block-body statement**; the if-init at `:1260` binds only `err`. ⇒ **`tlsConn` is genuinely in scope at the fall-through; NO plumbing owed.** | T3 |
| **RD-CLASSIFY** ⚠️ | SPEC §1.1: `classifyHandshakeErr` is consumed at `:1266`, error branch only | **CONSUMPTION HOLDS. BUT its DECLARATION is `:424` and it has NO DOC COMMENT.** The `:392` *"BEHAVIOR_CONTRACT B5"* comment belongs to **`type handshakeOutcome`** (`:385-410`, type at `:411`); the comment immediately above `:424` (`:420-421`) is the doc comment of **`const noClientCertErrText` (`:422`)**. ⇒ **an instruction to "amend the comment above `classifyHandshakeErr`" HAS NO TARGET.** | T9 |
| **RD-NOOUTCOME** ⚠️ | Not in the SPEC | **`manager.go:385-386` is a GUARD-RAIL the row must not trip.** `handshakeOutcome`'s doc reads *"classifies a downstream TLS handshake result into the **three** counted buckets plus a fourth that counts NOTHING"* — actively CONFUSING at +1 (four *counters*, still three *outcome buckets* + `outcomeOther`). ⇒ **phase 75 must NOT add a `handshakeOutcome` variant.** `no_certificate` is a success-path annotation entirely OUTSIDE the `classifyHandshakeErr` switch. **Anything touching `handshakeOutcome` is a design error for this row.** | T9 |
| **RD-IMPORTS** | SPEC §4: `crypto/tls` + `internal/stats` already imported ⇒ +0 production imports | **HOLDS.** `stdtls "crypto/tls"` **`:5`** · `crypto/x509` `:6` · `errors` `:7` · `.../internal/stats` **`:30`**. `ConnectionState()` needs nothing further. **CONFIRMED BY BUILD** — V1/V2/V3 all compiled with zero import changes. | T1, T10 |
| **RD-IMPGATE** ⚠️ | SPEC §4: the phase-74 "+0 production imports" gate is UNRELIABLE | **CONFIRMED BY REPRODUCTION, and a replacement is pinned WITH A NEGATIVE CONTROL.** The phase-74 form returns **14 hits and exits 0** on the phase-74 delta. **Replacement:** extract each touched file's `import (…)` block at `master` and at HEAD, sort, `diff -u`. **NEGATIVE CONTROL RUN: inserting `"strconv"` into a scratch `manager.go` makes it exit 1** and print `+manager.go\t"strconv"` ⇒ a green gate is evidence, not silence. `impblock name.go \| grep -c envoy_listener_ssl` ⇒ **0** (map-literal immunity). | T10 |
| **RD-HELPTEXT** | SPEC §11: `helpText` map `:456-472`, phase-74 entries `:469-471`, **14** entries | **ALL HOLD; the count verified TWICE** — statically and **BY EXECUTION** (`len(helpText)` ⇒ 14, via `go test -overlay` which never writes the worktree). Blank line `:468` separates the two groups. | T4 |
| **RD-HELPDOC** | SPEC D4: `name.go:448` says "Of the 14 entries" | **HOLDS, and the drift is WIDER than D4 records.** `:448` *"Of the **14** entries"* → 15; **`:451-452` ALSO** *"the last **three** are the phase-74 listener-scope TLS handshake outcomes"* — the last four are not all phase-74, and `ssl.no_certificate` is an **annotation**, not a member of the outcome trichotomy. ⚠️ **`grep -rn 'len(helpText)' --include='*.go'` ⇒ 0** — the count claim is guarded by NOTHING. | T4 |
| **RD-PROMKEY** | SPEC §5: the Prometheus key is `envoy_listener_ssl_no_certificate` | **CONFIRMED BY EXECUTION, twice independently** (A1 and V1, both via `go test -overlay`): `ExtractTags("listener.127_0_0_1_10000.ssl.no_certificate")` ⇒ residual `listener.ssl.no_certificate`, label `envoy_listener_address="127_0_0_1_10000"`; `flattenToProm` ⇒ **`envoy_listener_ssl_no_certificate`**. Address-INVARIANT: holds for `127_0_0_1_10000`, `0_0_0_0_10000`, `__1_10000` and `___45259` (IPv6). `IsValidName` ⇒ **true** for every form. | T4, T5 |
| **RD-PANICS** ⚠️ | SPEC §5: `NewCounter` panics on an invalid name (`registry.go:117`) and a duplicate (`:107`) | **BOTH HOLD — and the population is FIVE, not two.** `grep -n 'panic(' internal/stats/registry.go` ⇒ `:107` duplicate · `:117` invalid-name · **`:129` FROZEN** · **`:165`** `NewCounterIfAbsent: registered as non-Counter` · **`:212`** the Gauge equivalent. Only `:107`/`:117` are reachable from `registerListenerMetrics` (plain `NewCounter`, pre-Freeze per `manager.go:356`) ⇒ **the SPEC's conclusion survives; the count does not.** ⚠️ Recorded because A1 first reported "THREE" and the controller refuted it — the correction-is-a-claim rule, instantiated against this PLAN's own agent. | T1 |
| **RD-SORT** | SPEC §10 item 4: the new `want` entry sits **4th, after `ssl.handshake`** | **CONFIRMED BY DERIVATION, three times independently** (controller `sort`, A2's Go program, V2's `LC_ALL=C sort` + `sort.Strings`): `fail_verify_error`, `fail_verify_no_cert`, `handshake`, **`no_certificate`** ⇒ position **4 / LAST** (`'n'` 0x6e > `'h'` 0x68). **A pure APPEND — nothing shifts.** Sortedness is enforced by `listenerSSLNames`'s `sort.Strings` (`manager_test.go:2006`), which **auto-collects** the new name from the registry. ⚠️ The SPEC flagged this as a hazard to derive; it is real in principle and **absent in fact** — say so rather than reserving caution for it. | T1 |
| **RD-CALLSITES** ⚠️ | SPEC §11: `assertSSLCrossProduct`'s callers are `:4493`, `:4519`, `:4544` | **REFUTED — those are the enclosing TEST FUNC DECLARATION lines.** `4493 func TestServeConnection_SSLHandshakeIncrements` · `4519 func …SSLFailVerifyErrorIncrements` · `4544 func …SSLFailVerifyNoCertIncrements`. The **CALL sites are `:4508` / `:4539` / `:4557`**. Helper doc opens `:4464`, func `:4474`, roster `:4480`. | T3 |
| **RD-TERROR** ⚠️ | SPEC §11 / router item 7: `GateMatchesInc`'s pointer assertions use `Errorf` | **REFUTED — they use `t.Error(`, not `t.Errorf(`.** 3 occurrences of `t.Error(` and **0** of `t.Errorf(` in the non-nil half. Non-fatal either way, so they still fire independently — but **the +2 must match `t.Error`.** Assertion lines are `:2191/:2194/:2197` and `:2241/:2244/:2247`; `:2190`/`:2240` are the `// THE LOAD-BEARING HALF.` comments. | T2 |
| **RD-POLLFILE** ⚠️ | Implied by SPEC §10.1 | **`pollCounter` (`:34`) and `counterValue` (`:66`) are declared in `quic_test.go`, NOT `manager_test.go`.** Same package so it compiles, but a task citing them in `manager_test.go` points at the wrong file. **`counterValue` `Errorf`s on an ABSENT name and returns `-1`** — that is precisely what makes a zero-loop non-vacuous, and what gives the roster extension a FREE registration assertion. `pollCounter` by contrast **silently returns 0** for an absent name (burning its full timeout), so it is safe only for the positive half where 0≠1 fails anyway. | T3 |
| **RD-RENAME** ⚠️ | SPEC §10 item 4: rename `…RegistersExactlyThreeSSLNames` + its doc comment | **THE RENAME TOUCHES THREE SITES, not two.** `:1940` (a cross-reference inside **ANOTHER test's** doc comment — `TestListenerManager_AllocatesBaseListenerMetrics`, which the router listed under *"needs NOTHING"*), `:2019` (its own doc comment), `:2023` (the declaration). Post-rename `grep` over `*.go` must be CLEAN; the remaining hits are historical `docs/` records (phase-74 `PLAN.md`, phase-75 `BRAINSTORM.md:229`/`SPEC.md:354`) and **must NOT be edited.** | T1 |
| **RD-RUNSTALE** ⚠️⚠️ | Not in any phase-75 document | **A STALE `-run` SELECTOR ON A UNIT TEST REPORTS `ok` AND EXITS 0.** Controller-reproduced LIVE: `go test ./internal/listener/ -count=1 -run 'TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames'` at the CURRENT tip ⇒ `ok … 0.004s [no tests to run]`, **exit=0**. ⇒ a break command carrying the post-rename name BEFORE T1 lands (or the pre-rename name after) **self-certifies GREEN while executing NOTHING.** A NEW SHAPE of `reference_differential_run_selector`, previously indexed only for the differential runner's subtests. **Every `-run` string in this PLAN must be re-derived after T1.** | ALL |
| **RD-XSET** ⚠️⚠️ | Not in the SPEC | **THE TREE GOES RED AT THE PRODUCTION CHANGE, before any guard edit.** TWO `reflect.DeepEqual` **exact-set** pins compare a 3-element `want` against what `listenerSSLNames` collects: `manager_test.go:2055` and **`quic_test.go:279`**. Registering a 4th name makes BOTH red immediately. ⇒ **T1 must land the `want` extensions FIRST (RED, `got` 3 / `want` 4) and the registration SECOND (GREEN)** — otherwise the task ends on a red tree. V2 reported the inverse framing (*"the guard IS the pre-existing red"*); **ordering the guard first restores normal TDD and is what this PLAN mandates.** | T1 |
| **RD-BREAK** ⚠️⚠️⚠️ | SPEC §10.1 **and** router item 5(a): the positive arm *"IS the row's DISCRIMINATING break — deleting the `len(…) == 0` guard leaves every existing test green and fails only this one"* | **REFUTED ON BOTH HALVES. THE PREDICTION IS INVERTED.** See §1.3 — controller-reproduced end-to-end. Break D (unconditional Inc) fires **`TestServeConnection_SSLHandshakeIncrements`**, a PHASE-74 test, via `assertSSLCrossProduct`'s **NEGATIVE** half; **the new positive arm PASSES.** And Break D′ — the same edit with `sslLeafRoster` left at three leaves — leaves the **ENTIRE package GREEN** (`ok … 3.233s`). ⇒ **the roster extension is the SOLE guard on this row's central pinned decision**, not accommodation for the new arm. | T3 |
| **RD-CONV** ⚠️ | SPEC §10.1: build the arm from `mkDownstreamTSInline` | **CONCLUSION HOLDS; THE CALL DOES NOT COMPILE AS IMPLIED.** `mkDownstreamTSInline(t *testing.T, certPEM, keyPEM string)` takes **`string`**, while `handshakeTestPKI.serverCertPEM/serverKeyPEM` are **`[]byte`** (`:4032-4038`). The call needs `string(pki.serverCertPEM), string(pki.serverKeyPEM)`. Confirmed one-way TLS: `:627-649` carries `TlsCertificates` ONLY — no `ValidationContextType`, no `RequireClientCertificate`. | T3 |
| **RD-MTLS** | SPEC §10.1: `startMutualTLSListener` CANNOT supply the arm | **CONFIRMED.** `:4437-4462`; `mkDownstreamTSMutualTLS` (`:4361`) sets `RequireClientCertificate: wrapperspb.Bool(true)` at `:4364` **plus** an inline `trusted_ca` at `:4374-4380`, which per `internal/tls/config.go:65` yields `stdtls.RequireAndVerifyClientCert` ⇒ a no-cert connection **FAILS** and books `fail_verify_no_cert`. It can never produce a completed handshake with an empty peer chain. | T3 |
| **RD-CHAINNIL** | Implied by SPEC §10.1 | **`mkTLSChain(nil, …)` ⇒ `FilterChainMatch == nil` ⇒ DEFAULT chain ⇒ NO `tls_inspector` listener filter needed** (`:681-684`). `startMutualTLSListener:4442` already relies on this and documents it. Use the 4-arg `NewManager(boot, cm, reg, testHTTPRegistry())` convenience wrapper, **not** `NewManagerWithBaseDirAndAllowH2C`. | T3 |
| **RD-PREEXISTING** ⚠️ | Not in any phase-75 document | **A PRE-EXISTING TEST ALREADY DRIVES THE PHASE-75 INC SITE.** `TestNewManager_ChainSelectionPropagation` (`manager_test.go:1624`) builds its chains with **`mkDownstreamTSInline`** (`:1639-1640`, controller-verified) — ONE-WAY TLS — and dials them, so it completes certificate-free handshakes **today**. ⇒ **no new test is needed to REACH the Inc site** (a positive arm is still needed to ASSERT its value), and the nil-counter crash hazard is immediate and unavoidable in the existing suite rather than hypothetical. This is how V2 discovered the Break-B crash. | T2, T3 |
| **RD-BC-ROSTER** | SPEC §9: `grep -n 'ssl\.handshake'` ⇒ 916, 928, 1851, 1857, 1859, 5002 | **CONFIRMED EXACTLY, all six.** `:1851` opens *"Introduced by phase 74 — **three** listener-scope counters"* ⇒ "three" goes false. | T8 |
| **RD-BC-962** | SPEC D6: `ssl.no_certificate` is the SOLE occurrence, line 962, offset 627 | **CONFIRMED, AND THE UNIT DISTINCTION IS THE WHOLE POINT.** `grep -c` ⇒ **1**, at `:962`. The line is **1002 chars / 1007 bytes**; 0-based **CHARACTER** index **627**; 0-based **BYTE** offset **630**. **The BRAINSTORM's 627 HOLDS as a character index.** ⚠️ This anchor has now survived **THREE** challenges across two stages. **Cite the unit or do not cite the number.** | T8 |
| **RD-BC-HEADING** ⚠️⚠️ | SPEC §9: *"**No two-ADR heading precedent exists in this file** — leave the heading"* | **REFUTED by enumerating the population.** `grep -cE '^#{2,4} .*ADR-[0-9]+.*ADR-[0-9]+' BEHAVIOR_CONTRACT.md` ⇒ **16**. **Two are exactly phase 75's shape — a LATER phase extending an EARLIER row's heading, SEMICOLON-joined:** **`:785`** `### Stats sinks — the dog_statsd UDP sink with tags (per phase 49 ADR-0266; **batching per phase 50 ADR-0267**)` ← **THE PRECEDENT**, form `<what> per phase <N> ADR-XXXX`; and **`:5736`** (HTTP/3, a three-clause multi-phase accretion). Also `:885` (`ADR-0278 / ADR-0280`) plus eleven `+`-joined same-phase cases. ⇒ **the heading MAY and SHOULD be extended.** A PLAN obeying SPEC §9 would have left the heading falsely attributing a four-name roster to one ADR. | T8 |
| **RD-LEDGER** | SPEC §7/R3/R4: the ledger is `:4950`/`:4954-5002`; the tail form uses `→` U+2192 with bold running THROUGH the parenthetical | **ALL CONFIRMED, `cat -A`-verified.** `grep -c '^### Stat surface'` ⇒ 1 at **`:4950`**. `:4998` Phase 47.1 · `:5000` Phase 51 · **`:5002` Phase 74 = THE TAIL** · **`:5004` `### Forward-pointer note (26.3)`**. Tail bytes: `**Phase 74 M-bM-^@M-^T 1201 M-bM-^FM-^R 1204 (+3) (…):**` ⇒ em dash **U+2014**, arrow **U+2192**, **bold closes AFTER the parenthetical, at the colon**. `:5000` closes bold **BEFORE** it ⇒ **the form is NOT uniform; MATCH THE TAIL.** The new line inserts after `:5002`, before the `:5004` heading. | T8 |
| **RD-BC-TOTALS** | SPEC §7: `:831`/`:847` are the two narrative bare totals, 1204 → 1205 | **CONFIRMED, and the phase-74 precedent is what makes them LIVE rather than historical** — phase 74 bumped exactly these two 1201 → 1204. `:831` sits in the graphite_statsd sink section, `:847` in the OTLP one; both read `Stat surface UNCHANGED at **1204**`. Applied in a scratch copy: `grep -c '1204'` goes **3 → 1**, `grep -c '1205'` **0 → 3** (two totals + the new ledger line). | T8 |
| **RD-ROADMAP-75** ⚠️⚠️ | Not in the SPEC | **THE ROW-75 CELL (`ROADMAP.md:137`) CARRIES THREE STALE CLAIMS, and the IMPL opens that very cell.** (i) *"(the discriminator for that form is the **ADR FAMILY, not the phase gap**)"* — **REFUTED** by SPEC §1.1 R1 / ADR-0297 ¶9 (n=4; `:16901` is a SAME-FAMILY case using the INDENTED form; the rule is SELF-vs-OTHER-ADR). (ii) *"This is the **THIRD** internal mis-pointer in ADR-0296"* — **NOT ESTABLISHED** per SPEC D5; ADR-0297 ¶7 explicitly **claims no ordinal**. (iii) *"`ProbeAdmin` at `:552`"* — **REFUTED**: it is `:554`; `:552-553` is its doc comment. ⇒ **T11 fixes all three in the same edit as the row flip.** This is the phase-74 V2-M2 class, recurring one row later. | T11 |
| **RD-BLINDSPOT** ⚠️ | Router: check (1)'s blind spot is *"106 data rows, 102 matched, FOUR misses"* | **REFUTED — BOTH TOTALS ARE OFF BY ONE, and it was WRONG WHEN WRITTEN.** RE-DERIVED: `grep -cE '^\| [0-9]'` ⇒ **107**; sentinel-regex matches ⇒ **103**; misses ⇒ **4**. The main table runs **`:31`–`:137`** inclusive = 137−31+1 = **107**. ⚠️ The ROADMAP is **BYTE-IDENTICAL** between `e822f1ad` and HEAD (`git diff --stat` EMPTY) and **BOTH tips measure 107/103** ⇒ not drift. **The MISS ENUMERATION HOLDS** — `00` (em-dash "after" column) · `04` (DOT in slug `http-1.1`) · `28.1a` · `28.1b` (letter suffix), **all four `done`.** Recorded wrong in **two consecutive lineages** now (phase-74 PLAN: *"104 rows, 102 matched, TWO misses"*). | PLAN close |
| **RD-ADR0297** | SPEC §14: ADR-0297 is §Context-only | **CONFIRMED.** Heading **`:17322`**; `awk '/^## ADR-0297/,0'` ⇒ `### Context` **1**, `### Decision` **0**, `### Consequences` **0**, `^\*(§Decision` **1**. Tail is `## ADR-0297`; `grep -c '^## ADR-0298'` ⇒ **0**. ⚠️ §Context carries **ELEVEN** numbered paragraphs (¶1-¶11), **more than SPEC §14's list of eight items (a)-(h) implies** — T11 must not duplicate what ¶1-¶11 already say. **This also discharges V1's open item F8:** the in-code `(ADR-0297)` citation is correct. | T11 |
| **RD-ADR0296FORM** | SPEC §14.1: the correction is an INDENTED `  > [CORRECTED …]` blockquote; leading bytes `0x20 0x20 0x3E 0x20`; `grep -c '^  > '` ⇒ 2 | **CONFIRMED** — `:16901` and `:16910`, the only two instances, `od -c`-verified byte-identical. A `###` heading follows ADR-0296 §Decision (g) at `:17310`, so a **blank line after** the inserted blockquote is required. `grep -n 'VERIFYIFGIVEN'` ⇒ **exactly one hit, `:17308`** — the citing sentence itself. §Context ¶7 (`:17274`) contains **ZERO** `require_client_certificate`. `internal/tls/config.go:79-84` returns `stdtls.VerifyClientCertIfGiven`, documented `:60-68`; ROADMAP row 67 (`:129`) is **`done`**. | T11 |
| **RD-0110-PORT** ⚠️ | SPEC §8.1's keying comment cites `listener.0.0.0.0_10447` | **`10447` IS `0111`'s PORT.** `0110`'s own reference port is **`10446`** (`0110/driver/driver.go:39`); `0111`'s is `10447` (`:46`). ⇒ **the transplanted keying comment must be re-pointed to 10446**, or it documents the wrong fixture. Next-free reference port is **10450** (max allocated 10449 at `0113/driver/driver.go:115`; `grep -rn '10450' test/` ⇒ 0). Moot for this row, load-bearing for the deferred roster. | T5 |
| **RD-0110-ANCHORS** | SPEC R5 + §8: the `0110` anchors | **MOSTLY HOLD, ONE REFUTED.** HOLD: `wantObservable` `:89` · `BackendCount` `:289` (`return 1`) · `SubjectConfig` `:313` · SDS hard-stop `:334-341` · `ProbeAdmin` **`:554`** (doc `:552-553`) · `var _ fixture.Driver = (*rccfDriver)(nil)` `:613`, **exactly ONE `var _` line** · receiver `rccfDriver` (`:110`) · driver **613** lines · **ZERO** `scrapeProm`/`AssertStats`/`StatsAsserter`/`prometheus` hits in the whole dir ⇒ no collision. **REFUTED:** SPEC R5 corrects the arm cite by saying *"`:362-374` is the doc comment"* — the doc comment is actually **`:362-387`** (func at `:388`); `:362-374` is only a PREFIX. The arms proper are **`:402-435`** (`:400` is `var out bytes.Buffer`). ⚠️ **A drift correction is itself a claim, and this one anchor has now been mis-stated twice.** | T5 |
| **RD-0110-LIVE** | SPEC §3.6: `0110` has a LIVE upstream both sides ⇒ the fast-failure race is NOT blocking | **CONFIRMED STRUCTURALLY, not merely from the YAML.** `0110` implements **neither** `BackendKindAware` **nor** `PerHostBackendKind`, so the runner takes `uniformKind := fixture.TCPEcho` (`runner_test.go:254`; `TCPEcho = 0`, `fixture.go:137`) and the in-process branch `case fixture.TCPEcho, fixture.HTTPEcho:` (**`:271-283`**), which contains **no `waitTCPDial`**. Reference cluster `envoy.yaml:105-116` (`STRICT_DNS` → `host.docker.internal:{{.BackendPort}}`); subject `envoy-go.yaml:102-111` (`STATIC` → `127.0.0.1:{{.BackendPort}}`). ⇒ **NOT BLOCKING.** | T5 |
| **RD-SHAPEA** | SPEC §3.5: nothing pre-moves `0110`'s `ssl.*` | **CONFIRMED, all five legs.** Reference readiness `wait.ForHTTP("/ready").WithPort("9901/tcp")` (`harness.go:133`) — **admin only**. Subject readiness reads `cmd.StdoutPipe()` via `bufio` (`harness.go:59-94`, called `:266`) — **no `net.Dial` anywhere in `StartSubjectProxy`**. `ProbeAdmin` (`:554`) is step 9, `GET /ready` only. `waitTCPDial` (`runner_test.go:2250-2267`) has **27 call sites, all inside non-`TCPEcho`/`HTTPEcho` arms**, and its own doc says *"used to observe **subprocess-backend** readiness"*. | T5 |
| **RD-DISPATCH** | SPEC §8.2: dispatch is a silent type assertion at `runner_test.go:1347-1349`; both admin addrs already threaded | **CONFIRMED — no `else`, no log, no skip.** `sa.AssertStats(t, ref.AdminAddr(), subj.AdminAddr())`; both addresses already held from step 9 (`:1330`) ⇒ **`0110` owes ZERO plumbing.** `fixture.StatsAsserter` is `test/differential/fixture/fixture.go:75-77`: `AssertStats(t TB, refAdminAddr, subjAdminAddr string)` — takes **`fixture.TB`**, returns **nothing**, order **(t, ref, subj)**. `fixture.TB` (`:64-68`) is **exactly `Errorf`/`Fatalf`/`Helper`** — **no `Logf`, no `Cleanup`** ⇒ record via `log.Printf`. | T5 |
| **RD-TRIPWIRE** | SPEC §8.2: `var _ fixture.StatsAsserter` is a tripwire, not the dispatch mechanism | **CONFIRMED, and the tripwire is RARE.** `grep -rln 'var _ fixture\.StatsAsserter' test/fixtures/` ⇒ **2** (`0111/driver/driver.go:800`, `0076/driver/driver_test.go:87`) while **82** fixture dirs define an `AssertStats` method ⇒ 80 dispatch with no compile-time guard at all. `0110` becomes the **third** instance. ⚠️ A raw `grep -rln 'AssertStats' test/fixtures/` returns **185 FILES** (prose in READMEs/expectations/YAML plus one `.rs`) — **not a count of asserters.** | T5 |
| **RD-0110-STALE** | SPEC §8.4: THREE stale `ssl.*` sites in `0110`; the README bullet is BUNDLED | **ALL THREE CONFIRMED VERBATIM; no fourth inside `0110/`** (14-pattern sweep). (1) `README.md:160-163` — the false half runs `:160`→mid-`:161`; **the still-live `/listeners` guard starts MID-LINE at `:161` ("Never assert")** and runs to `:163` ⇒ **SPLIT, do not delete.** (2) `envoy.yaml:24` — single clause. (3) `expectations.yaml:166-171` — three clauses, incl. the inverting *"strictly STRONGER than a subject-only stat"*. **`driver/driver.go` carries ZERO stale claims** (its `PLAN-65 C3` refs at `:376`/`:544` are about ALERT TEXT and stay true). **Do NOT touch** `README.md:164-165` / `expectations.yaml:173-175`. | T6 |
| **RD-0111-TEMPLATE** ⚠️ | SPEC §8.4: the template is `0111/README.md:168-175` and `expectations.yaml:196-201` | **README HOLDS; THE EXPECTATIONS SPAN IS REFUTED — the block runs `:196-207`**, and `:196-201` is only its first six lines. ⚠️ **And the template's handling of the third clause is the instruction the SPEC does not give:** `0111` did **NOT delete** *"strictly STRONGER than a subject-only stat"* — it **APPENDED** *"— it is cross-side, and it now has a cross-side STAT beside it."* ⇒ **the `0110` rewrite AMENDS that clause rather than removing it.** Also `0111/envoy.yaml`'s corrected header is **MULTI-LINE (`:23-27`)**, so `0110:24` **GROWS**. | T6 |
| **RD-0111-ROSTERS** | SPEC §11: `0111`'s three rosters at `:682-684`, `:687-691`, `:692-696`; ABSENT/value split `:698-713`; precondition `:672-674` | **ALL EXACT.** `AssertStats` `:655` (doc `:629-654`), `scrapeProm` `:739` (doc `:731-738`), loop opens `:697`, lookups `:698-699`, `!refOK` `:706-709`, `!subjOK` `:710-713`, value checks `:714-719`, the labelled-redundant cross-side tripwire `:725-727`. ⚠️ The `log.Printf` at `:682-686` carries the roster in **both** its format string **and** its args — **two edits, easy to half-do.** Imports `0110` must GAIN: **exactly four** — `log`, `math`, `net/http`, `strconv`. | T5, T7 |
| **RD-0111-CLOSED** ⚠️ | SPEC §11: *"no value changes … the PLAN should decide explicitly"* | **THE DECISION IS FORCED IN ONE DIRECTION AND THE SPEC'S FRAMING MISSES WHY.** `0111`'s prose is a **CLOSED ENUMERATION**: `README.md:167-174` says the driver *"pins all three"* and then names the remainder (*"Still out of scope: `ssl.connection_error`"*); `expectations.yaml:197-203` names *"Still UNasserted from that family: `ssl.connection_error` … and the `ssl.ciphers/curves/versions` breakdowns"*. **A name in NEITHER list reads as ASSERTED.** ⇒ **`0111` must be edited whether or not the value assertion is added** — the *"no value changes ⇒ no edit"* option does not exist. §2.4 records the decision and its reasons. | T7 |
| **RD-STATSSINK** ⚠️ | Not in any prior roster | **A FIFTH stale stat-surface count, outside every phase-74/75 roster.** `internal/statssink/registration_test.go:25`, `:51`, `:80` read *"(stays 1200 / 1196)"* / *"(stays 1200 / non-H2 1196)"* — dating to **phase 49** (`65130bbe`), never updated by phase 74's +3. **Unasserted by code** (the five guards compare a FRESH registry against 0), so nothing goes red. Related: `internal/statssink/statsd_tcp.go:78` (*"~1200 stats"*, already hedged). §2.5 records the disposition. | T9 |
| **RD-GUARDSWEEP** | SPEC §11: *"Swept and CLEAN"* — the five `TestNoNewStat*`, no golden/testdata, the Lua hits | **ALL THREE CONFIRMED BY COMMAND.** The five guards (`registration_test.go:26/53/81/109/137`) route through `countMetrics` (`:13`) asserting `!= 0` on a **fresh** `stats.NewRegistry()` — no listener booted. Every `testdata`/`golden` dir swept: `internal/stats/testdata/` holds only `fuzz/`; `grep -rln "ssl\|listener\."` over the plausible ones ⇒ **no output**. `internal/filter/http/lua/ssl.go` is the `Connection:ssl()` userdata (`sslMethods` `:89-102`) — **zero stat names, zero registry interaction.** ⇒ **the SPEC's roster of EXECUTABLE guards is COMPLETE**; everything additional found by this PLAN is prose, a boundary ledger, or an opportunity. | T10 |
| **RD-BASELINE** | SPEC §15 counts | **ALL CONFIRMED at tip, controller-run:** fixtures **119** (`0118` absent; highest numeric dir `0117`) · fuzzers **55** · BackendKind tail **38** (file declares **39** constants, 0-38 — a TAIL VALUE) · go.mod **67** requires (lineage figure **2**) · DECISIONS tail **ADR-0297** PROPOSED, next-free **ADR-0298** · stat surface **1204** (doc-sourced; **NO mechanical command; NOT re-derived**). | T10 |

### 1.2 Findings this re-derivation surfaced that the SPEC does NOT carry

**Every finding below is itself a claim** (`reference_a_drift_correction_is_itself_a_claim`) — re-run the grep before any of them becomes an `old_string`. Two of this PLAN's own agents had a correction refuted by the controller; that is recorded in place rather than smoothed over.

- **F1 (SEVERE) — the row's discriminating break is not where the SPEC and the router put it.** Full treatment in §1.3. It is the reason T3 is written the way it is.
- **F2 (SEVERE) — THE FAST-FAILURE SUPPRESSION IS REFERENCE-ONLY. envoy-go does NOT have it.** SPEC §3.6 characterises the race as a general precondition on *"every future `ssl.*` fixture"*. **Executed both sides:** with the cluster pointed at `127.0.0.1:1` the **reference** yields four honest zeros with `downstream_cx_total=3`, while the **subject's numbers are BYTE-IDENTICAL to a healthy run** (`handshake=2 no_certificate=1`). The mechanism: `serveConnection` completes the handshake at step (6) and only dispatches to the network chain at step (7) (`manager.go:1258-1284`), so **envoy-go accounts `ssl.*` strictly BEFORE the upstream dial**; Envoy C++ dials at accept time, concurrently. ⇒ **the hazard is a REFERENCE-side property, not a shared one**, and `reference_ssl_stats_suppressed_by_fast_failing_upstream` should be read as reference-scoped. The *rule* ("give the listener a live upstream or none") still stands, because a cross-side fixture is only as strong as its weaker side.
- **F3 (SEVERE) — `0110`'s `structuralCheck` OUTRANKS both of the asserter's preconditions.** Arms 1 and 3 must echo, so any dead upstream breaks the observable and the run dies at step 8 (`runner_test.go:1274 subj drive:`) long before step 10. ⇒ **Break E is UNREACHABLE on `0110` without neutralising `structuralCheck`**, and that is a property of the fixture, not a defect. Recorded so the IMPL does not spend a cycle trying.
- **F4 (SEVERE) — the ABSENT check does NOT guard what the SPEC says it guards.** SPEC §8.2 frames the comma-ok/`continue` split as the guard against a counter that *"fails to REGISTER"*. **Executed:** deleting the production registration while the `Inc` remains produces a **nil-pointer SIGSEGV in the subject subprocess** (`manager.go` Inc site → `internal/stats/counter.go:22` `atomic.(*Uint64).Add`), which kills the connection goroutine, fails arm 3 (`none=REJECTED`), and the run dies at `structuralCheck` — **`AssertStats` never runs and the ABSENT branch never fires.** ⇒ the ABSENT check's real remit is narrower and still worth having: **a name that is registered but absent from the SCRAPE** (e.g. a reference-version change dropping it, or a Prometheus-name mismatch), plus stopping the `want: 0` row from passing vacuously. **Keep it; restate what it defends.** Break I proves the narrower claim directly.
- **F5 (SEVERE) — ADR-0297 §Context ¶7's grep claim is SELF-FALSIFYING, exactly as ADR-0296 ¶3's was one ADR earlier.** `grep -n 'VERIFYIFGIVEN' docs/envoy-go/DECISIONS.md` ⇒ **TWO lines (`:17308`, `:17340`) / THREE occurrences**, while ¶7 (at `:17340`) asserts *"exactly ONE hit — `:17308`, the citing sentence itself."* It was true until it landed, and ¶7's own text made it false. **The phase-74 IMPL fixed precisely this defect in ADR-0296 ¶3 and phase 75's SPEC then reproduced it in ADR-0297 ¶7.** ⇒ **the correction blockquote must NOT restate that grep and must NOT spell the all-caps token** — every restatement mints another counter-example. T11 also fixes ¶7's wording in place. `ROADMAP.md:137` carries the same defect.
- **F6 (SEVERE) — ADR-0297 §Context ¶9's OWN correction is WRONG, and this is the THIRD consecutive wrong answer to one methodological question.** ¶9 replaced the BRAINSTORM's *"ADR FAMILY"* rule with *"SELF vs OTHER-ADR (n=4)"*. **Refuted by enumerating the real population, n=7 with a THIRD form:**

  | form | n | instances (owning ADR / correcting phase) | self/other |
  |---|---|---|---|
  | indented `  > [CORRECTED at phase N/ADR-XXXX: …]` | **2** | `:16901` (in ADR-0286 / phase 67), `:16910` (in ADR-0286 / phase 74) | both **OTHER** |
  | **inline bold `**[CORRECTED at the phase-N IMPL: …]**`** | **3** | `:17187` (in ADR-0294 / phase 72 = SELF), **`:17211` (in ADR-0294 / phase 73 = OTHER)**, `:17272` (in ADR-0296 / phase 74 = SELF) | 2 SELF, **1 OTHER** |
  | inline italic `*(corrected at the phase-N IMPL from "…")*` | **2** | `:17213` ×2 (in ADR-0295 / phase 73) | both SELF |

  **`:17211` is the counter-example** — controller-verified: it sits inside **ADR-0294** (phase 72's ADR) and reads `**[CORRECTED at the phase-73 IMPL: the closing clause is REFUTED …]**`, i.e. a LATER phase correcting a DIFFERENT, already-landed ADR, rendered **INLINE**. ⇒ **SELF-vs-OTHER does not discriminate either.** (`:17268` is a TEMPLATE — `phase-NN`, `<pointer>` — not an instance.) **The only discriminator the n=7 population supports is graft SCALE:** inline attaches to the specific clause it contradicts; **indented stands alone where an entire bullet or paragraph is re-characterised** — and both indented instances re-characterise a whole ADR-0286 bullet. ⇒ **the INDENTED form remains correct for phase 75** (it re-characterises the whole of §Decision (g)), **but ¶9's stated reason must be replaced.** ⚠️ **The prescription has now survived three different wrong justifications — family, then self-vs-other, now scale.** T11 corrects ¶9 in place. ¶9's subsidiary claims DO hold (`:17209` is not a correction; `:16901` is same-family-and-indented, so family cannot discriminate).
- **F7 (MODERATE) — a new PARAGRAPH in the TLS subsection would break live line citations; an in-place append breaks none.** `:1849/:1851/:1853/:1855/:1857/:1859/:5002` are all cited BY LINE from `ROADMAP.md:137`, `STATE.md:20/46/48`, phase-75 `SPEC.md:20/28/29/276/334-343` and `BRAINSTORM.md:136/240/321`. Inserting a paragraph after `:1853` shifts `:1855→:1857`, `:1857→:1859`, `:1859→:1861` and silently invalidates all of them. ⇒ **T8 APPENDS IN PLACE to `:1853`**, and the ledger entry after `:5002` becomes the **only line-adding edit in the file** (`5744 → 5746`). Every other cited anchor keeps its exact line number — verified by difflib opcodes over the applied scratch copy.
- **F8 (MODERATE) — inserting the ADR-0296 blockquote shifts FIVE live `DECISIONS.md` citations by +2, and TWO of them are in files the IMPL edits anyway.** `:17308/:17304/:17274/:16901/:16910` precede the insert and are unshifted; but **`:17310 → :17312`** (cited from phase-75 `SPEC.md:476`) and **`:17314 → :17316`** (cited from **`ROADMAP.md:137`**, **`STATE.md:48`**, phase-75 `SPEC.md:414`, `BRAINSTORM.md:321`). ADR-0297's heading moves `:17322 → :17324`. ⇒ **T11 carries an explicit citation-fixup step** for `ROADMAP.md:137` and `STATE.md:48` (both edited at T11 regardless). ⚠️ **Frozen stage documents (`SPEC.md`, `BRAINSTORM.md`) are NOT rewritten** — no written convention requires it and the lineage treats them as historical records.
- **F9 (MODERATE) — `"three-fifths"` was ALREADY WRONG at the phase-74 tip, self-inconsistently within phase 74's own edits. Do NOT bump it to "four-fifths".** `BC:1851`, `BC:5002`, ADR-0296's heading (`:17256`), ADR-0296 §Consequences (`:17315`) and `ROADMAP:205` all say *"three-fifths"*. But `BC:928` — landed by the **same phase** — enumerates a **FOUR**-family dynamic remainder. 3 retired + 4 surviving = **seven**, so it was three-SEVENTHS; the `5` is a fossil of the pre-74 two-family count. ⇒ **T8 and T11 REMOVE the ratio and state the retirement as an ENUMERATION**, rather than propagating a fourth wrong denominator.
- **F10 (MODERATE) — the stat-surface ledger is discontinuous in TWO places, not one.** `:5002`'s `[LEDGER GAP]` records only `1200 → 1201`. Controller-verified, it misses a second: **`Phase 46.1b` (`:4996`) closes at `1196 → 1198`, and `Phase 47.1` (`:4998`) opens at `1200 → 1200`** — an unrecorded **+2** between them. ⇒ **T8's new entry RECORDS both gaps and back-fills NEITHER** (the phase-74 precedent: writing a ledger line from inference would be inventing a record).
- **F11 (MODERATE) — the row-75 ROADMAP cell's `B5` account is OVER-BROAD.** It says *"all eight `B5` hits are `AMEND-B5`/phase-25.2 Wasm"*. All eight ARE `AMEND-B5`, but **`:4685` is phase-29.1 mongo**, not Wasm. The conclusion (BC has no B-numbered step scheme; the departure lives at `:1855`) stands. ⇒ folded into T11's cell rewrite.
- **F12 (MINOR, PREVENTIVE) — the phase-06.1 canonical listener stat table was NOT extended by phase 74, and phase 75 must not extend it either.** `BEHAVIOR_CONTRACT.md:152-157` carries **"Listener — 2 names:"** (`downstream_cx_total`/`downstream_cx_active`) under a heading (`:148`) whose extension list stops at 24.1. Phase 74 added three listener names and deliberately left it alone. **Recorded so the IMPL does not "discover" it as a missed site** and mint an inconsistency with phase 74.

### 1.3 ⚠️⚠️ THE HEADLINE — the row's discriminating break is NOT where the SPEC and the router say it is

**What the record claims.** SPEC §10.1: *"⚠️ **This arm is also the row's DISCRIMINATING break.** Deleting the `len(…) == 0` guard (making the Inc unconditional) leaves every existing test green and fails only this one."* Router item 5(a) repeats it verbatim in substance.

**BOTH HALVES OF BOTH CLAIMS ARE REFUTED. The prediction is INVERTED.** Built and run in a private worktree, then **reproduced independently by the controller** (committed tree, `git restore`d after, `-count=1` throughout):

**Break D** — delete the `if len(tlsConn.ConnectionState().PeerCertificates) == 0 { … }` wrapper so the Inc is unconditional:

```
--- FAIL: TestServeConnection_SSLHandshakeIncrements (0.01s)
    manager_test.go:4602: listener.127_0_0_1_46359.ssl.no_certificate = 1, want 0 — only [handshake] may move on this arm
FAIL	github.com/pgdad/envoy-go/internal/listener	0.030s
```

It fires the **PHASE-74 mTLS success arm**, through `assertSSLCrossProduct`'s **NEGATIVE** half. **The new positive arm PASSED.** Mechanically obvious in hindsight: an unconditional Inc still leaves `no_certificate == 1` on the arm that *wants* 1, so the positive half is structurally blind to it.

**Break D′ — the decisive stacked control.** Break D **plus** the negative roster left at the three phase-74 leaves (i.e. the row landed without the roster extension):

```
ok  	github.com/pgdad/envoy-go/internal/listener	3.233s
```

**THE ENTIRE PACKAGE IS GREEN with the row's central pinned decision broken.**

**The consequence, and it changes the task spine.** Only an arm where the counter must stay **0** while a handshake **COMPLETES** can detect an unconditional Inc. Exactly one such arm exists — phase 74's mTLS success arm — and it only asserts `no_certificate == 0` **once the negative roster is extended to four.** Therefore:

- **The roster extension in `assertSSLCrossProduct` is the SOLE guard on the row's central pinned decision.** It is NOT cosmetic accommodation for the new arm. A task list treating it as optional cleanup ships the row undefended, and green.
- **The POSITIVE arm and the ROSTER EXTENSION guard DIFFERENT defects and neither substitutes for the other.** Positive arm ⇒ catches *"registered but never Inc'd"* (Break E) and *"mode-gated"* (Break F). Roster extension ⇒ catches *"Inc'd unconditionally"* (Break D). **The row needs BOTH, and T3 lands them together.**
- **An honest limit, reported rather than papered over:** Breaks E and F fire the **IDENTICAL** assertion (`no_certificate = 0, want 1` at the positive arm). They are distinguishable by the EDIT, not by the OUTPUT. Same defect class, so this is acceptable — but **this PLAN does not claim they are separately discriminated.**

**Why it was missed.** Both documents reasoned from *"the positive arm is the only test that asserts the counter MOVES"* — true — to *"therefore it is the discriminating break"* — false. The defect in question is the counter moving **too often**, which only a negative assertion can see. It took building the row and running the break; no amount of reading the SPEC would have surfaced it.

### 1.4 What is still NOT verified — the IMPL inherits no false confidence

Per `reference_quoting_is_not_executing`. **This section is much shorter than phase 74's, because this PLAN executed most of the row** — which makes the residue more important, not less.

- **The full differential suite was run ONCE, plain, not under `-race`.** `ok … 399.675s`, exit 0, no flake. **`-race` over the differential package is NOT covered** — T10 owes it or must say plainly that it was skipped and why.
- **`ssl.no_certificate` on QUIC was never driven.** The registration roster was extended and `quic_test.go` is green, but **no H3 connection was driven to confirm the counter does not MOVE.** The parity argument is STRUCTURAL (`Manager.Start` never launches an accept loop for `kindQUIC`, so `serveConnection`'s TLS block is unreachable) and therefore name-independent — but it is an argument, not a measurement. SPEC §3.7 already lists this as unprobed; it stays unprobed.
- **The pinned predicate was never cross-checked at `require_client_certificate: true`.** `0109` was not run. Break F proves the mode term is load-bearing *in envoy-go*; it does not prove the reference agrees at `require=true`. SPEC §3.1's wire trace is the evidence, and it is reference-side.
- **Resumption/renegotiation.** `ssl.session_reused` was 0 in every run, so a RESUMED TLS 1.3 session was never exercised. SPEC §1.2 flags it; it remains open, and it is the one scenario in which the pinned predicate could be wrong in production.
- **Break B's crash under `-race`, and whether it can contaminate a full-suite run.** Tested in isolation only.
- **The absolute total `1205`.** No mechanical command exists; the **+1 DELTA** is solid, the TOTAL rides two now-documented inferred ledger steps (F10). Unchanged from the SPEC's posture.
- **Nothing reference-side was re-probed for the SEMANTICS docket.** Every §3 figure quoted in T8's prose is transcribed from ADR-0297 §Context, not re-executed here. **T8 must cite the ADR paragraph, not present them as independently re-derived.** The one exception is the `0110` fixture leg, which WAS executed against the live reference at this PLAN.
- **Two of this PLAN's own agents had a correction refuted by the controller** (A1's "three panics" → five; A3's proposed `cx_total == 3` precondition → non-discriminating). Both are recorded in §1.1/§2.3. **Treat every RD-* row as a claim, not a result.**

### 1.5 Adversarial-pass record

*(This section is written AFTER the pass, from what the agents and the controller actually found — **never asserted in advance over a placeholder**. That failure mode is the phase-69 cited-but-unwritten class; the phase-74 PLAN deleted phase-73's guardrail against it and then committed the exact failure it names, shipping `STATUS: COMPLETE` citing a `PROGRESS.md` that did not yet exist. **The guardrail is restored here verbatim.** ⚠️ This PLAN's `PROGRESS.md` was written BEFORE this section was populated; the controller verified it exists on disk before flipping this status.)*

**STATUS: COMPLETE** — populated from seven agents' ACTUAL reports plus controller re-verification. All seven ran on disjoint remits with **PRIVATE scratch** (`reference_parallel_subagents_private_scratch`); the three build agents ran in **separate git worktrees on their own branches**, never on `master`, never pushed.

**A1** (production + `internal/stats`): 1 REFUTED anchor, 10 new findings. **A2** (every test-side guard; the structural analysis that reframed the row): 2 REFUTED anchors, 7 new sites. **A3** (fixtures + dispatch + harness): 3 REFUTED anchors, 8 new findings. **A4** (all three docs, each edit APPLIED to a scratch copy and mechanically verified): 5 REFUTED anchors, 10 new findings. **V1** (built the production change + the positive arm + the variadic helper; ran 6 breaks): the §1.3 headline. **V2** (built the guard half; ran 5 breaks): the ordering inversion and the `-run` footgun. **V3** (built the `0110` asserter and ran it against the LIVE reference; ran 9 breaks): the first subject-side measurement and F2/F3/F4.

**The EIGHT that changed this document's instructions:**

- **V1-S1 (SEVERE) — the discriminating break is inverted (§1.3).** The SPEC's and the router's central claim about this row's own defence is wrong in both directions, and the stacked control D′ shows the row ships green with its pinned decision broken. **T3 is restructured around it, and the roster extension is promoted from cleanup to the row's primary guard.**
- **V3-S2 (SEVERE) — the fast-failure suppression is REFERENCE-ONLY (F2).** envoy-go accounts `ssl.*` strictly before the upstream dial. The SPEC states the hazard as general. **T5's precondition prose is rewritten to say which side it defends against, and `reference_ssl_stats_suppressed_by_fast_failing_upstream` is re-scoped.**
- **V3-S3 (SEVERE) — the ABSENT check does not guard what the SPEC says (F4).** A deleted registration crashes the subject subprocess and kills the run before `AssertStats`. **T5 keeps the guard and restates its remit; Break I proves the narrower claim.**
- **V2-S4 (SEVERE) — the tree goes RED at the production change, before any guard edit (RD-XSET).** Two `reflect.DeepEqual` exact-set pins compare a 3-element `want`. V2 reported this as an inverted TDD shape (*"the guard IS the pre-existing red"*). **This PLAN instead ORDERS THE GUARD FIRST, restoring normal red→green** — T1 Step 1 extends both `want` slices (RED), Step 3 lands the registration (GREEN).
- **V2-S5 (SEVERE) — a stale `-run` selector on a UNIT test reports `ok` and exits 0 (RD-RUNSTALE).** Controller-reproduced live. **Every `-run` string in this PLAN is stated post-rename, and T1 Step 6 re-derives them.**
- **V2-M6 (MAJOR) — the "delete the registration" break destroys its own evidence full-package.** A SIGSEGV in a background goroutine names no test and aborts the binary before the guard's output flushes. **Break B now MANDATES `-run` isolation, and the PLAN records that the answer to "assertion or panic?" is BOTH, decided by the command.**
- **A4-M7 (MAJOR) — a new BC paragraph would break seven live line citations (F7).** **T8 appends IN PLACE to `:1853`**; the ledger line becomes the only line-adding edit.
- **A4-M8 (MAJOR) — ADR-0297's own ¶7 and ¶9 are defective (F5, F6),** the ¶7 defect being the same self-refuting-grep species the phase-74 IMPL had just fixed in ADR-0296 ¶3. **T11 corrects ADR-0297 in place while completing it, and the blockquote is written to avoid minting further counter-examples.**

**Also folded:** RD-REGDOC (the doc comment starts at `:351`, so the SPEC's anchor edits a fragment) · RD-CALLSITES (the SPEC's three "callers" are test-func declaration lines; the real call sites are `:4508/:4539/:4557`) · RD-TERROR (`t.Error`, not `t.Errorf`) · RD-POLLFILE (`pollCounter`/`counterValue` live in `quic_test.go`) · RD-CONV (`mkDownstreamTSInline` takes `string`; the SPEC's call does not compile) · RD-RENAME (three sites, incl. a cross-reference in a test the router said needs nothing) · RD-BC-HEADING (16 two-ADR headings; `:785` is the exact precedent) · RD-BLINDSPOT + A4-N5 (107/103/4, and the recorded figure's *provenance* claim is false — 106/102 is the pre-BRAINSTORM number, invalidated by row 75's own addition in the very commit that claimed to re-derive it) · F9 (`three-fifths` was already wrong; do not bump to four-fifths) · F10 (two ledger gaps) · F11 (`:4685` is mongo, not Wasm) · F12 (do not extend the `:152-157` table) · RD-PANICS (five panic sites, not three — **a correction of this PLAN's own agent**) · RD-NOOUTCOME (do not touch `handshakeOutcome`) · RD-STATSSINK (a fifth stale count site) · RD-PREEXISTING (a pre-existing test already drives the Inc site) · V1-E7 (two gofmt alignment traps) · A4-B14 (the candidates sentence has ZERO interior periods — that is *why* `[^.]*\.` binds, so no replacement may introduce a `.`).

**Three findings ACCEPTED AS-IS, with the reasoning recorded rather than the instruction changed:**

- **A2's warning against a shared roster does not apply to V1's `sslLeafRoster`, and the constraint is kept as an invariant.** A2 argued a package-level roster would let ONE misspelling satisfy both the spelling pin and the cross-product helper. **Controller adjudication: it cannot here** — `sslLeafRoster` holds bare LEAF names (`"handshake"`) while the exact-set spelling pins hold FULL names (`prefix + "ssl.handshake"`) in their own independent literals. The roster is retained for its real benefit (adding a leaf extends the negative half of all four call sites at once), and **A2's constraint is written into T3 as a standing invariant: the spelling pins keep their own literals and are never refactored onto `sslLeafRoster`.**
- **V2's objection to the `0111` value assertion is retired on the SPEC's own executed evidence, but the decision goes V2's FALLBACK way for a different reason.** See §2.4 — including an explicit reversal of the controller's own earlier position.
- **V1's empty-`wantSuffixes` `Fatal` is unproven dead-defensive code by construction** (no call site can reach it) and is kept anyway: it costs three lines and it closes the one hole the variadic signature opens, since a zero-suffix call would silently degrade into an all-zero assertion that passes with every Inc deleted.

---

## 2. Decisions this PLAN owes explicitly

*(The router required four of these to be DECIDED, not left implicit: the `0111` question, the break roster, the guard roster, and the deferred-sentence narrow. "Not deciding is how a roster goes stale.")*

### 2.1 `assertSSLCrossProduct` — VARIADIC, and the three landed call sites change by ZERO bytes

**Signature:** `func assertSSLCrossProduct(t *testing.T, reg *stats.Registry, addr string, wantSuffixes ...string)`.

Chosen over three alternatives, and the reason is not style. Go's variadic call rules make `assertSSLCrossProduct(t, reg, addr, "handshake")` compile **unchanged**, so "preserve the existing call sites" is met byte-for-byte rather than by careful re-editing — which matters because those three arms are now the row's primary defence (§1.3). Rejected: a `[]string` parameter forces `[]string{…}` wrappers at all three sites; a `map[string]uint64` expected-value table **discards the positive-polls / negative-reads-once asymmetry**, which is load-bearing because `pollCounter` returns 0 for both *"zero"* and *"unregistered"* while `counterValue` `Errorf`s on absence (RD-POLLFILE); a second `alsoWant string` parameter is not extensible and forces a `""` sentinel at three sites.

**Verified by execution:** compiles, `gofmt` clean, `golangci-lint` clean, all four arms PASS, and the three unchanged call sites behave identically.

### 2.2 The positive arm's listener — a NEW one-way helper, and the two guards are not interchangeable

`startOneWayTLSListener` mirrors `startMutualTLSListener` with `mkDownstreamTSInline` instead of `mkDownstreamTSMutualTLS`. `startMutualTLSListener` **cannot** supply the arm (RD-MTLS: `RequireClientCertificate: true` + `trusted_ca` ⇒ `RequireAndVerifyClientCert` ⇒ a no-cert client FAILS and books `fail_verify_no_cert`). **No new PKI, no new transport-socket builder** — it reuses `handshakeTestPKI` and `startEchoBackend`. ⚠️ `mkDownstreamTSInline` takes **`string`** PEMs while the PKI holds `[]byte` (RD-CONV), so the call needs explicit conversion or it does not compile.

**And per §1.3 the arm is NOT the row's discriminating break.** It guards *never-Inc'd* and *mode-gated*; the roster extension guards *unconditional*. **Both are mandatory.**

### 2.3 `0110`'s second precondition — `ssl_handshake > 0`, NOT `downstream_cx_total == 3`

An agent proposed *"pin `downstream_cx_total == 3` exactly rather than `!= 0`"* as the cheapest sound second precondition. **REJECTED — it does not discriminate.** Under reference-side suppression all three connections are still accepted and counted while the whole `ssl.*` family reads 0, so `cx_total == 3` is satisfied EQUALLY by *"all three arms worked"* and *"all three arms were accepted and `ssl.*` was wholly suppressed"* — an input consistent with both hypotheses (`reference_probe_must_discriminate`). Tightening `!= 0` to `== 3` buys determinism and **zero** discrimination for the hazard it is meant to guard.

**The adopted guard is a precondition on the TLS path itself — `envoy_listener_ssl_handshake > 0`, per side, `Fatalf`** — and it is **PROVEN to fire**: with the reference cluster pointed at `127.0.0.1:1`, `ref: TLS decode did NOT run — envoy_listener_ssl_handshake == 0 while envoy_listener_downstream_cx_total == 3`. ⚠️ Per F2 it defends against the **reference** side only, and per F4 its unique contribution is narrower than the SPEC implies: it stops the `want: 0` row from passing vacuously and turns three cryptic value mismatches into one named diagnosis. **T5's comment says exactly that** rather than the SPEC's broader claim.

### 2.4 ⚠️ The `0111` question — DECIDED: add it to the NAMED-UNASSERTED lists, NOT to the value rosters. *(This REVERSES the controller's own earlier position, recorded rather than quietly amended.)*

The router required an explicit decision. Two of this PLAN's agents recommended **ADD to the rosters**, and the controller initially agreed on the ground that `0111`'s ABSENT check makes a `want: 0` a live cross-side REGISTRATION guard rather than a vacuous `0 == 0`. **Two findings then arrived that change the answer:**

1. **F4 (executed):** the ABSENT check does **not** catch a deleted registration on a counter that is actually `Inc`'d — the nil-pointer crash kills the run first. So the "live registration guard" argument for a `want: 0` is much weaker than it looked.
2. **A4-N6 (verified):** of the NINE TLS-downstream fixtures (`0002, 0004, 0018, 0027, 0103, 0108, 0109, 0110, 0111`), **only `0110` is `require=false`.** `0111` sets `require_client_certificate: true` on both sides (`envoy-go.yaml:70`, `envoy.yaml:75`), so a success-path no-cert annotation reads **0 on every arm, structurally**. Growing `0111`'s *"asserts all three"* to *"all four"* would document a **vacuous** `0 == 0` as if it were coverage — and `0110` now carries the real cross-side assertion **with a discriminating non-zero value** (`no_certificate=1` against `handshake=2`).

**But `0111` MUST still be edited (RD-0111-CLOSED).** Its prose is a **CLOSED ENUMERATION**: `README.md:167-174` says the driver *"pins all three"* and then names the remainder (*"Still out of scope: `ssl.connection_error`"*); `expectations.yaml:197-203` names *"Still UNasserted from that family: `ssl.connection_error` … and the `ssl.ciphers/curves/versions` breakdowns"*. **A name in NEITHER list reads as ASSERTED.** Leaving `no_certificate` out of both lists is the one genuinely wrong outcome.

⇒ **DECISION: `0111`'s three value rosters (`log.Printf` at `:682-686`, `names` at `:687-691`, `want` at `:692-696`) stay at THREE names. `0111`'s prose gains `ssl.no_certificate` in its NAMED-UNASSERTED list, with the reason stated** (`require=true` holds it at 0 on every arm; the cross-side assertion lives at `0110`). **T7 owns it.** The controller's reversal is recorded here because a decision changed on evidence, and hiding that would make the reasoning unauditable.

### 2.5 The stale-count sites that produce NO red — recorded and FIXED, not deferred

Five sites carry counts that go false at +1 and **none of them fails a test**: `manager.go:175`/`:177` and `:358` · `name.go:448` and `:451-452` · `manager_test.go:1936`/`:1940`/`:1987-1989`/`:1993-1997`/`:2019`/`:2126`/`:2152`/`:2203`/`:4466`/`:4491-4492`/`:4561` · `quic_test.go:65`/`:226`/`:272`/`:295` · `name_test.go:222-223`. **Plus a FIFTH, outside every prior roster (RD-STATSSINK):** `internal/statssink/registration_test.go:25`/`:51`/`:80` read *"(stays 1200 / 1196)"*, dating to **phase 49** and never updated by phase 74's +3.

**Disposition:** the first four groups are IN scope (T1/T3/T4/T9) because they go false **at this row**. **The `statssink` trio is RECORDED, NOT FIXED** — it was already 3 phases stale before this row, it is unasserted prose in a package this row does not otherwise touch, and fixing it would widen the delta into `internal/statssink`. §5 carries it forward. ⚠️ **Stating the disposition IS the deliverable**; an unnamed stale site is how a roster goes stale (`reference_fuzzer_count_docs_drift`).

### 2.6 The deferred-sentence narrow — derived from the sentence's TEXT and verified in a scratch copy

The Observability `candidates:` sentence (`ROADMAP.md:205`) **does not name `no_certificate`** — so the narrow is **NOT a name deletion**. It edits the *retired* clause. **Applied to a scratch copy and verified mechanically:** check (2) ⇒ **3** (unchanged), occurrence-count ⇒ **3**, the sentence still terminates at `force-trace.` so `[^.]*\.` still binds, match length `999 → 1033`, and the surviving prose is not self-contradicting.

⚠️ **A HARD CONSTRAINT the SPEC does not state, derived by A4:** the matched sentence contains **ZERO interior periods** — *that is why* `[^.]*\.` binds all 999 characters. **Any replacement must introduce no `.`** — no `manager.go`, no `internal/…`, no abbreviation. The adopted text satisfies it (interior periods 0 before and after). **T11 must re-verify this in a scratch copy before landing.**

### 2.7 The BEHAVIOR_CONTRACT heading — EXTENDED, on a precedent the SPEC said did not exist

SPEC §9 says *"**No two-ADR heading precedent exists in this file** — leave the heading."* **REFUTED (RD-BC-HEADING): there are 16**, and `:785` is the exact "later phase extends an earlier row" shape, semicolon-joined. ⇒ the heading is extended. Obeying SPEC §9 would have left `:1849` falsely attributing a four-name roster to a single ADR.

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-75 IMPL.
- **⚠️ THIS PLAN's OWN stage-close checklist** *(distinct from T11, which closes the IMPL — every task T1–T11 below is an IMPL task)*. At the PLAN close the controller: creates `PLAN.md` + `PROGRESS.md` (the ONLY two files in the phase-directory delta) · rolls **STATE §Current IN PLACE** (lifecycle 2 → 3; §Recent re-capped at FIVE **with its preamble updated** — the ADR-0288 rule; ⚠️ the three singleton greps return **2**, never "fix" to 1) · rolls `next-prompt.txt` to the phase-75 IMPL · **runs all three sentinel checks MECHANICALLY TWICE** (worktree + landed master post-push) · squash-pushes. **ROADMAP and DECISIONS are BYTE-UNTOUCHED at a PLAN; row 75 STAYS `in-progress`.**
- **⚠️ THE PREDICATE IS PINNED AND MUST NOT BE RE-OPENED.** The Inc gate is **`len(tlsConn.ConnectionState().PeerCertificates) == 0` ALONE, with NO client-auth term.** Settled at the WIRE at the SPEC (a `DownstreamTlsContext` with only `tls_certificates`, no `CertificateRequest` on the wire, counter fires anyway) and **proven load-bearing here** by Break F, which adds the refuted mode term and fires the positive arm. **A mode-gated predicate would UNDER-count against the reference on every one-way-TLS listener.**
- **⚠️ THE ROSTER EXTENSION IS THE ROW'S PRIMARY GUARD, NOT CLEANUP** (§1.3). Adding `"no_certificate"` to `assertSSLCrossProduct`'s negative roster is the ONLY thing that catches an unconditional Inc. Break D′ proves the package goes fully green without it.
- **⚠️ DO NOT TOUCH `handshakeOutcome`** (RD-NOOUTCOME). `no_certificate` is a success-path annotation entirely outside the `classifyHandshakeErr` switch; the classifier is consumed in the **error branch only** (`manager.go:1266`) and never sees a successful handshake. Adding a `handshakeOutcome` variant is a design error for this row. Its doc comment at `:385-386` is corrected as PROSE only (T9).
- **TWO production files, and both are functional:** `internal/listener/manager.go` (+1 field, +1 registration line inside the EXISTING `if rt.tlsMode`, +5 for the guarded Inc, 2 stale comments) and `internal/stats/name.go` (+1 `helpText` entry, +1 doc comment). **ZERO new packages. ZERO new exported symbols** — `sslNoCertificate` is unexported, `startOneWayTLSListener`/`sslLeafRoster` are test-side.
- **⚠️ ZERO NEW PRODUCTION IMPORTS.** `stdtls "crypto/tls"` (`manager.go:5`) and `internal/stats` (`:30`) are already present; `ConnectionState()` needs nothing further. **CONFIRMED BY BUILD** in three independent worktrees. ⚠️ **TEST-side imports DO grow and that is PERMITTED** — `0110`'s driver gains exactly `log`, `math`, `net/http`, `strconv`. **T10 must audit the two categories SEPARATELY**, and must use the RD-IMPGATE command, not the phase-74 one.
- **⚠️ `internal/listener/manager_test.go`, `quic_test.go`, `name_test.go` and `0110/driver/driver.go` are EDITED, not gated.** Do NOT write a sha256 gate on them — that is the `reference_plan_schedules_edits_to_a_byte_gated_file` defect, which phase 73 hit as its own SEVERE-1. Gate them on SHAPE at T10 (pre-existing test names still present, `go test` green). **Set-difference the EDIT roster against the sha256 roster before running T10.**
- **BYTE-UNTOUCHED, sha256-asserted at T10:** `internal/listener/quic.go` · `internal/listener/listenerfilter/**` · `internal/tls/**` · `internal/xds/**` · `internal/boot/**` · `internal/bootstrap/**` · `internal/tracing/**` · `internal/cluster/**` · `internal/filter/**` · `internal/statssink/**` · `validate/**` · `cmd/**` · `go.mod` · `go.sum` · `test/fixtures/0110-…/envoy-go.yaml` · `test/fixtures/0111-…/driver/driver.go` (per §2.4 — its **prose** files ARE edited).
- **The stat delta is +1 as a NAME SURFACE, not per deployment:** a plaintext-only deployment gains **ZERO** names (pinned by three plaintext guards); a QUIC listener gains it permanently **ZERO** — **PARITY, not an anomaly** (§1.4 records that this is structural, not measured). **1204 → 1205.**
- **⚠️ `ssl.no_certificate` is NOT a synonym for `ssl.fail_verify_no_cert`, and the mis-mapping is wrong in BOTH directions.** One annotates an ACCEPTED handshake, the other books a REJECTED one; they are mutually exclusive on every connection. **The wording is load-bearing** — T8 states both directions explicitly.
- **⚠️ A nil `*stats.Counter`.Inc is a PROCESS CRASH** — no nil guard in `Inc` (`internal/stats/counter.go:22`), no `recover()` anywhere in `internal/listener/`, and `serveConnection` runs in a per-connection goroutine. **Do NOT "defensively" add a nil guard at the Inc site** — that would make Break B vacuous and mask the invariant. The +2 pointer assertions in T2 are what convert the crash into a named failure.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package, **per task** — not just at T10.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting` / **`reference_bash_cwd_reset_commits_to_main`**): pin the canonical root; use **`git -C <abs-worktree-path>`** for every git command; tripwire with `pwd` + `git rev-parse --abbrev-ref HEAD` (must be the stage branch, **NEVER `master`**) + `git rev-list --count <base>..HEAD` before any commit or gate run. **A `cd` outside the repo silently resets the Bash cwd — this fired LIVE during the phase-75 BRAINSTORM and again during this PLAN's own session.**
- **Subagents commit LOCALLY only** (`feedback_subagents_no_push`); the controller squash-pushes at stage close (`feedback_push_to_origin`). Locate commits by SUBJECT (`git log --grep 'phase 75'`), never by position.
- **Known PRE-EXISTING flakes — never reflex-classify as phase-75 regressions:** the `internal/cluster` `-race` outlier flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce`, `outlier_test.go:766` — **isolate-re-run, do NOT re-classify**) · the full-suite startup flake (`subject ready: EOF` on an UNRELATED fixture) · the SDS `init_fetch_timeout` dial-budget flake (covering `internal/boot TestSDSEndToEnd_FetchFailure_BootFailsClosed` **and** the original `internal/xds` test) · and TWO still-UNINDEXED load flakes, isolate-green and not root-caused: `internal/httpclient TestOptions_ZeroValue_NoOpDefaults` and `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`. ⚠️ **None fired in any of this PLAN's three build worktrees**, including a full 400 s differential run — so a failure at the IMPL is more likely real than at most stages. Isolate-re-run before classifying, and say which.
- **ADR-0045's escape valve is armable-but-unconsumed.** Eleven tasks, two small production files, +10 production lines. If the IMPL trips the >25-task / >1500-LoC gate, STOP and split — do not defer via TODO/stub tasks (ADR-0045 §6.3).

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). **Eight of the ten breaks below were COMPILED AND RUN at this PLAN**, so the usual `[NOT pre-compiled]` caveat applies only to Breaks H and J. Where a break was substituted, the substitution AND its rationale are recorded — **and a substitution's rationale is itself a claim** (phase 72 shipped a false one).
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed. ⚠️ **This PLAN's headline is a break whose predicted firing site was wrong**, caught only because the stacked control D′ was run.
- **Breaks run AFTER committing** (`reference_break_protocol_commit_first`) — `git restore` wipes uncommitted work; being killed mid-break leaves a BROKEN tree.
- **`-count=1` on EVERY run** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **⚠️ RE-DERIVE EVERY `-run` STRING AFTER T1's RENAME** (RD-RUNSTALE). A stale selector prints `ok … [no tests to run]` and **exits 0** — it self-certifies green while executing nothing. Controller-reproduced live.
- **⚠️ Break B MUST be run under `-run` isolation** or the SIGSEGV aborts the binary before the guard's output flushes and no test is named (V2-M6).
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first. **This applies to PANICS too**: a crash must be attributed from its stack.
- **A break that does NOT fire is a FINDING** — record it in `PROGRESS.md`; do not route around it. **Breaks D′ and G are the two declared MUST-NOT-FIRE / MUST-STAY-GREEN breaks**; either one firing is equally a finding.
- **Full selector only** for the differential: `-run 'TestDifferential/0110-tls-require-client-cert-false'` — never bare `0110` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`). ⚠️ In `manager_test.go` the pointer assertions use **`t.Error`** (RD-TERROR) — match it.

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE at `cedd2f27` (repo-wide `--include='*.go'`, controller-run):** `sslNoCertificate` **0** · `startOneWayTLSListener` **0** · `sslLeafRoster` **0** · `no_certificate` under `internal/` **0** · `no_certificate` across ALL `*.go` **0**. New test identifier: `TestServeConnection_SSLNoCertificateIncrements` **0**.

⚠️ **`no_certificate` returns 104 hits under `docs/`** — a repo-wide grep WITHOUT `--include='*.go'` reads as a collision and is not one. **Scope the grep or misread the result.** ⚠️ **Re-run all six at the IMPL tip** (T1 Step 0) per `feedback_parallel_stream_mints_fresh_drift`.

**Renamed identifier:** `TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames` → `…ExactlyFourSSLNames`. Post-rename `grep` over `*.go` must be **CLEAN**; the surviving hits are historical `docs/` records (phase-74 `PLAN.md`, phase-75 `BRAINSTORM.md:229`, `SPEC.md:354`) and **must NOT be edited**.

---

## File structure

```
internal/listener/manager.go               [EDIT]  T1 (+1 *stats.Counter field at :183; +1 r.NewCounter inside the
                                                       EXISTING `if rt.tlsMode` at :382; the :175/:177 field comment;
                                                       the :351-373 doc comment — ⚠️ it starts at :351, NOT :358)
                                           [EDIT]  T3 (the guarded Inc after :1277, inside the existing
                                                       `if selected.tlsCfg != nil` block — tlsConn already in scope)
                                           [EDIT]  T9 (the :385-386 handshakeOutcome doc — PROSE ONLY, no variant)
internal/listener/manager_test.go          [EDIT]  T1 (the name-set `want` +1 in SORTED position 4 (a pure APPEND);
                                                       the RENAME at :2023 + its doc :2019 + the CROSS-REFERENCE :1940)
                                           [EDIT]  T2 (+2 `t.Error` pointer assertions after :2199 and after :2249)
                                           [EDIT]  T3 (sslLeafRoster; assertSSLCrossProduct → VARIADIC;
                                                       startOneWayTLSListener; TestServeConnection_SSLNoCertificateIncrements)
                                           [EDIT]  T9 (the stale-"three" prose sweep)
                                           ⚠️ EDITED, NOT sha256-gated. Gate on SHAPE at T10.
internal/listener/quic_test.go             [EDIT]  T1 (the `want` +1 at :274-278 — the zero-loop at :296-300 ranges
                                                       over `want`, so assertion (4) is covered FREE)
                                           [EDIT]  T9 (the :65/:226/:272/:295 stale-"three" prose)
                                           ⚠️ EDITED, NOT sha256-gated.
internal/stats/name.go                     [EDIT]  T4 (+1 helpText entry after :471 with a BLANK SEPARATOR LINE;
                                                       the :448 "14 entries" + :451-452 "last three" doc)
internal/stats/name_test.go                [EDIT]  T4 (`wantNames` +1 at :231-235; the :222-223 doc)
                                           ⚠️ EDITED, NOT sha256-gated.
test/fixtures/0110-…/driver/driver.go      [EDIT]  T5 (AssertStats + scrapeProm + `var _ fixture.StatsAsserter`;
                                                       +4 TEST-side imports: log, math, net/http, strconv)
test/fixtures/0110-…/README.md             [EDIT]  T6 (:160-163 — ⚠️ SPLIT the bullet; the /listeners guard starts
                                                       MID-LINE at :161 and is STILL LIVE)
test/fixtures/0110-…/envoy.yaml            [EDIT]  T6 (:24 — the comment only; NO config change. It GROWS multi-line)
test/fixtures/0110-…/expectations.yaml     [EDIT]  T6 (:166-171 — THREE clauses; the third INVERTS. Also a new leg (c))
test/fixtures/0111-…/README.md             [EDIT]  T7 (:167-174 — add no_certificate to the NAMED-UNASSERTED list)
test/fixtures/0111-…/expectations.yaml     [EDIT]  T7 (:197-203 — same; the CLOSED-ENUMERATION fix)
docs/envoy-go/BEHAVIOR_CONTRACT.md         [EDIT]  T8 (:831/:847 → 1205; :916; :918; :928; :1849 heading (EXTENDED —
                                                       SPEC §9 REFUTED); :1851; :1853 ⚠️ APPEND IN PLACE, no new
                                                       paragraph; :1855; :1857; :1859; a new ledger line after :5002)
docs/envoy-go/DECISIONS.md                 [EDIT]  T11 (ADR-0297 completed IN PLACE after the :17350 footer; ¶7's
                                                        self-falsifying grep; ¶9's refuted form rule; the ADR-0296
                                                        §Decision (g) INDENTED blockquote after :17308)
docs/envoy-go/ROADMAP.md                   [EDIT]  T11 (row 75 :137 → `done` + its THREE stale claims + the B5
                                                        over-breadth + the :17314→:17316 citation fixup;
                                                        the :205 narrow — ⚠️ ZERO interior periods)
docs/envoy-go/STATE.md                     [EDIT]  T11 (§Current rolled IN PLACE, lifecycle 3 → DONE; §Recent
                                                        re-capped at five WITH its preamble; the :48 citation fixup)
next-prompt.txt                            [EDIT]  T11 (rolled to the phase-76 BRAINSTORM; TRACKED despite .gitignore)
docs/envoy-go/phases/75-…/PROGRESS.md      [EDIT]  T1-T11 (the live task ledger; every task appends its ACTUAL result)
internal/listener/quic.go · internal/listener/listenerfilter/** · internal/tls/** · internal/xds/** ·
internal/boot/** · internal/bootstrap/** · internal/tracing/** · internal/cluster/** · internal/filter/** ·
internal/statssink/** · validate/** · cmd/** · go.mod · go.sum · 0110-…/envoy-go.yaml ·
0111-…/driver/driver.go                                        [BYTE-UNTOUCHED — sha256 at T10]
```

---

## Task 1 — the counter field + the `rt.tlsMode`-gated registration + the TWO exact-set pins (RED via the pins, GREEN via the registration)

**Files:**
- Modify: `internal/listener/manager.go` (field at `:183`; registration at `:382`; the `:175`/`:177` field comment; the `:351-373` doc comment)
- Modify: `internal/listener/manager_test.go` (the name-set `want` at `:2049-2053`; the RENAME at `:2023`; its doc `:2019`; the cross-reference `:1940`)
- Modify: `internal/listener/quic_test.go` (the `want` at `:274-278`)

**Interfaces:**
- Produces: field `sslNoCertificate *stats.Counter` on `listenerRuntime`, and the registered stat name `listener.<normalized-addr>.ssl.no_certificate`. Consumed by T2's pointer assertions, T3's Inc, T4's `helpText` key and T5's fixture.
- Produces: the renamed `TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames`. Every later `-run` selector depends on this name.
- Consumes: nothing new. `stdtls "crypto/tls"` (`:5`) and `internal/stats` (`:30`) are already imported ⇒ **+0 production imports.**

**Entry state:** clean `cedd2f27`-derived branch; `go test ./internal/listener/ -count=1` green.

⚠️ **ORDERING IS LOAD-BEARING (RD-XSET / V2-S4).** TWO `reflect.DeepEqual` **exact-set** pins compare a 3-element `want` against what `listenerSSLNames` collects from the registry: `manager_test.go:2055` and `quic_test.go:279`. **Landing the registration first makes both red with no guard edit written**, ending the task on a red tree and inverting the TDD record. **Extend the `want` slices FIRST.**

- [ ] **Step 0 — re-run the collision greps at the IMPL tip** (`feedback_parallel_stream_mints_fresh_drift`). All must be **0**:

```bash
W=<abs-worktree-path>
for id in sslNoCertificate startOneWayTLSListener sslLeafRoster TestServeConnection_SSLNoCertificateIncrements; do
  printf '%-46s %s\n' "$id" "$(grep -rn "$id" --include='*.go' "$W" | wc -l)"
done
printf '%-46s %s\n' 'no_certificate (GO ONLY)' "$(grep -rn 'no_certificate' --include='*.go' "$W" | wc -l)"
# ⚠️ WITHOUT --include='*.go' this returns ~104 docs hits and reads as a collision. It is not one.
git -C "$W" diff --stat e822f1ad HEAD -- '*.go' go.mod go.sum test/    # parallel-stream re-check; expect EMPTY
```

- [ ] **Step 1 — extend BOTH exact-set `want` slices and RENAME the test (this is the RED step)**

`internal/listener/manager_test.go` — the `want` literal (currently `:2049-2053`). ⚠️ `"ssl.no_certificate"` sorts **4th / LAST** (`'n'` 0x6e > `'h'` 0x68) — a **pure APPEND, nothing shifts** (RD-SORT, derived three independent ways):

```go
	want := []string{
		prefix + "ssl.fail_verify_error",
		prefix + "ssl.fail_verify_no_cert",
		prefix + "ssl.handshake",
		prefix + "ssl.no_certificate",
	}
```

Rename at `:2023` and rewrite its doc comment at `:2019-2022`:

```go
// TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames pins the EXACT
// SPELLING of all three phase-74 names plus phase 75's ssl.no_certificate on a
// TLS-bearing listener. A count-only assertion is insufficient — see
// listenerSSLNames' doc — so this compares the whole sorted name set with
// reflect.DeepEqual.
//
// ⚠️ `want` must be in SORTED order: listenerSSLNames sort.Strings'es its
// result, so DeepEqual against an unsorted want fails on ORDER even when the SET
// is right. "ssl.no_certificate" collates LAST of the four, so it appends.
func TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames(t *testing.T) {
```

⚠️ **The rename touches a THIRD site the router listed under "needs NOTHING" (RD-RENAME).** `manager_test.go:1940`, inside `TestListenerManager_AllocatesBaseListenerMetrics`' doc comment:

```go
// pinned by TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames, and
```

`internal/listener/quic_test.go` — the `want` at `:274-278`. ⚠️ **The zero-loop at `:296-300` ranges over `want`, so assertion (4) is extended FREE** — and non-vacuously, because `counterValue` `Errorf`s on an ABSENT name (RD-POLLFILE):

```go
	// (1) REGISTRATION: all four names present, spelled exactly. A cardinality
	//     assertion would pass with all four misspelled — compare the set.
	//     SORTED: listenerSSLNames sorts, so want must too; ssl.no_certificate
	//     collates LAST. The phase-75 name stays ZERO here for the SAME
	//     STRUCTURAL reason as the other three — Manager.Start never launches an
	//     accept loop for kindQUIC, so serveConnection's TLS block (the ONLY
	//     ssl.no_certificate Inc site) is unreachable. No volume of H3 traffic
	//     moves it.
	want := []string{
		prefix + "ssl.fail_verify_error",
		prefix + "ssl.fail_verify_no_cert",
		prefix + "ssl.handshake",
		prefix + "ssl.no_certificate",
	}
```

- [ ] **Step 2 — run both pins and CONFIRM RED for the right reason**

Run — ⚠️ **the selector uses the POST-rename name; the pre-rename name now prints `ok … [no tests to run]` and exits 0** (RD-RUNSTALE):

```bash
go test ./internal/listener/ -count=1 -v \
  -run 'TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames|TestQUICListener_RegistersSSLNamesAtZero'
```

Expected: **BOTH FAIL** at their `reflect.DeepEqual` lines (`manager_test.go:2056`-region and `quic_test.go:279`-region), each reporting a 3-element `got` against a 4-element `want`. ⚠️ **Confirm the DeepEqual line is what fired**, not a build error — if the binary failed to compile, no assertion ran and the red proves nothing.

- [ ] **Step 3 — add the field and the registration (this is the GREEN step)**

`internal/listener/manager.go`, the `listenerRuntime` field block. ⚠️ **gofmt trap (V1-E7): the doc comment SPLITS the alignment run, so the new field takes a SINGLE space before its type while the four above stay column-aligned. That is the desired outcome — it keeps the phase-74 lines byte-identical.** Reproduce the spacing exactly:

```go
	// registered for EVERY listener; the four ssl.* counters are registered
	// only when rt.tlsMode is set (phase 74 — TLS-chains-only, matching the
	// reference), so on a plaintext listener all four pointers stay NIL.
	downstreamCxTotal   *stats.Counter
	downstreamCxActive  *stats.Gauge
	sslHandshake        *stats.Counter // phase 74: successful downstream TLS handshakes
	sslFailVerifyError  *stats.Counter // phase 74: client cert presented, CHAIN VERIFICATION failed
	sslFailVerifyNoCert *stats.Counter // phase 74: no client cert where one was required
	// sslNoCertificate is phase 75's SUCCESS-PATH annotation: a COMPLETED
	// handshake that presented no client certificate. It is NOT a synonym for
	// sslFailVerifyNoCert (a FAILED handshake) — the two are disjoint by
	// construction, one living on each side of the HandshakeContext error check.
	sslNoCertificate *stats.Counter // phase 75: completed handshake, no client cert presented
```

⚠️ `:174` *"The two cx metrics"* stays TRUE — **do not touch it.**

The registration, inside the **EXISTING** `if rt.tlsMode` block (**NO new gate**):

```go
	if rt.tlsMode {
		rt.sslHandshake = r.NewCounter(prefix + "ssl.handshake")
		rt.sslFailVerifyError = r.NewCounter(prefix + "ssl.fail_verify_error")
		rt.sslFailVerifyNoCert = r.NewCounter(prefix + "ssl.fail_verify_no_cert")
		rt.sslNoCertificate = r.NewCounter(prefix + "ssl.no_certificate")
	}
```

The `registerListenerMetrics` doc comment. ⚠️ **It starts at `:351`, NOT `:358` (RD-REGDOC)** — exactly ONE sentence goes false, and it cannot simply become *"four phase-74"* because the fourth counter is phase-**75**:

```go
// The two cx metrics are unconditional. The three phase-74 ssl.* counters plus
// the one phase-75 ssl.* counter (ssl.no_certificate — four in total) are all
// gated on rt.tlsMode, matching the reference, which registers listener.<addr>.ssl.*
```

- [ ] **Step 4 — run and CONFIRM GREEN**

```bash
go test ./internal/listener/ -count=1 -v \
  -run 'TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames|TestQUICListener_RegistersSSLNamesAtZero'
go test ./internal/listener/... -count=1
```
Expected: both PASS; package ok. ⚠️ **`internal/stats` is expected to be GREEN here too and that is the silent-staleness hazard T4 closes** — the `helpText` entry is not yet added and nothing complains.

- [ ] **Step 5 — per-task hygiene**

```bash
gofmt -l internal/listener && go vet ./internal/listener/... && golangci-lint run ./internal/listener/...
```
All silent. ⚠️ If `gofmt -l` names `manager.go`, the field alignment is wrong — fix with `gofmt -w`, do not hand-pad.

- [ ] **Step 6 — RE-DERIVE every `-run` selector in this PLAN against the renamed test**

```bash
grep -rn 'RegistersExactlyThreeSSLNames' --include='*.go' .        # expect ZERO
grep -rn 'RegistersExactlyFourSSLNames'  --include='*.go' .        # expect 3: :1940, :2019, :2023 (post-edit numbering)
go test ./internal/listener/ -count=1 -run 'TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames'
# ⚠️ MUST print "ok … [no tests to run]" and exit 0. That is the footgun, demonstrated — not a pass.
```
⚠️ **The surviving `docs/` hits (phase-74 `PLAN.md`, phase-75 `BRAINSTORM.md:229`, `SPEC.md:354`) are HISTORICAL records and must NOT be edited.**

- [ ] **Step 7 — commit**

```bash
git -C <abs-worktree-path> rev-parse --abbrev-ref HEAD   # MUST be the stage branch, never master
git -C <abs-worktree-path> add internal/listener/manager.go internal/listener/manager_test.go internal/listener/quic_test.go
git -C <abs-worktree-path> commit -m "phase 75 T1: the sslNoCertificate field + the rt.tlsMode-gated registration, RED-first via the two exact-set name pins (+ the rename and its THIRD cross-reference)"
```

- [ ] **Step 8 — BREAK A (run AFTER committing, `-count=1`)**

Edit: move `prefix + "ssl.no_certificate"` from index 3 to index 1 in `manager_test.go`'s `want`.
**MUST fire:** the `reflect.DeepEqual` name-set line, reporting an ORDER mismatch against a sorted `got`.
**[RUN at this PLAN — FIRED as predicted.]** Then `git restore` and verify `git status --porcelain` empty.

- [ ] **Step 9 — BREAK B (⚠️ `-run` ISOLATION IS MANDATORY)**

Edit: delete `rt.sslNoCertificate = r.NewCounter(prefix + "ssl.no_certificate")` — but **only after T3 lands the Inc**, so schedule this break at T3 if running strictly in order; if run here (no Inc yet) it fires cleanly with no crash.
**⚠️ MUST be run as** `go test ./internal/listener/ -count=1 -run 'TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames|TestListenerMetrics_GateMatchesInc'`. **Full-package it is a SIGSEGV that names no test and destroys its own evidence** (V2-M6).
**MUST fire:** the name-set `DeepEqual` (3 vs 4) and — once T2 lands — the POINTER assertion. **[RUN at this PLAN — fired both ways; the isolation requirement is the finding.]**

---

## Task 2 — `TestListenerMetrics_GateMatchesInc`: +2 pointer assertions (the nil-Inc crash guard)

**Files:** Modify `internal/listener/manager_test.go` (after `:2199` and after `:2249`; the `(b)`/`(c)` sub-test banners at `:2152`/`:2203`; the doc at `:2126`)

**Interfaces:** Consumes T1's `rt.sslNoCertificate` field. Produces nothing downstream — but it is what converts Break B from an unattributed background-goroutine SIGSEGV into a **named** failure.

⚠️ **THE POINTER HALF IS THE LOAD-BEARING HALF, re-confirmed by execution at phase 75.** Under Break B the `rt.tlsMode`, `ci.tlsCfg` and `defaultChain` assertions printed **NOTHING** — all three are set at BUILD time, upstream of `registerListenerMetrics`, and cannot observe a registration bug. Phase 74's V1-M2, reproduced.

⚠️ **They use `t.Error(`, NOT `t.Errorf(` (RD-TERROR).** Match it. Non-fatal either way, so all eight fire independently.

- [ ] **Step 1 — write the two failing assertions**

After `:2199` (the non-nil half, sub-test `tls_listener`):

```go
		// phase 75: the fourth pointer. This assertion is the ONLY thing that
		// turns "registration deleted while the Inc remains" from a PROCESS
		// CRASH in a background goroutine (which fails no test and is reported
		// as a package-level panic with a confusing sync/atomic stack) into a
		// NAMED test failure — (*stats.Counter).Inc has no nil check and
		// serveConnection has no recover().
		if rt.sslNoCertificate == nil {
			t.Error("TLS listener: rt.sslNoCertificate is NIL — Inc would panic the serveConnection goroutine")
		}
```

After `:2249` (the nil half, sub-test `plaintext_listener`):

```go
		// phase 75: the fourth pointer must be NIL here too — ssl.no_certificate
		// is registered inside the SAME rt.tlsMode block, so a separate gate (or
		// an accidentally ungated registration) shows up right here.
		if rt.sslNoCertificate != nil {
			t.Error("plaintext listener: rt.sslNoCertificate is NON-NIL — the rt.tlsMode gate did not hold")
		}
```

Banners: `:2152` `all THREE counter fields are NON-NIL` → `all FOUR counter fields are NON-NIL (three phase-74 outcomes + phase 75's sslNoCertificate)`; `:2203` `all THREE counter fields are NIL` → `all FOUR counter fields are NIL`; `:2126` `the three field pointers` → `the four field pointers`.

- [ ] **Step 2 — verify RED reachability HONESTLY**

⚠️ **A one-step red is NOT available here, and recording one would be a false record.** T1 already landed the registration, so both new assertions PASS immediately. **This task's red comes from its BREAK**, not from a pre-implementation state. Say so in `PROGRESS.md` rather than manufacturing a red. *(Contrast the phase-74 trap: an assertion referencing a field a later step creates makes the binary fail to COMPILE, so zero assertions run and a build failure gets recorded as an assertion red.)*

```bash
go test ./internal/listener/ -count=1 -v -run 'TestListenerMetrics_GateMatchesInc'
```
Expected: **PASS**, with all three sub-tests (`mixed_rejected_at_build`, `tls_listener`, `plaintext_listener`) present.

- [ ] **Step 3 — hygiene + commit**

```bash
gofmt -l internal/listener && go vet ./internal/listener/... && golangci-lint run ./internal/listener/...
git -C <abs-worktree-path> commit -am "phase 75 T2: extend TestListenerMetrics_GateMatchesInc to the FOURTH counter pointer — the nil-Inc-is-a-process-crash guard"
```

- [ ] **Step 4 — BREAK C (the load-bearing demonstration; `-run` isolated, `-count=1`)**

Edit: delete T1's `r.NewCounter(prefix + "ssl.no_certificate")` line.
Run: `go test ./internal/listener/ -count=1 -v -run 'TestListenerMetrics_GateMatchesInc'`
**MUST fire:** `rt.sslNoCertificate is NIL — Inc would panic the serveConnection goroutine`, from `tls_listener`, **and ONLY it**. The `tlsMode`/`tlsCfg`/`defaultChain` predicate assertions must print **NOTHING** — that silence IS the finding.
**[RUN at this PLAN — fired exactly one line; the predicates were silent.]** `git restore`; verify clean.

---

## Task 3 — the guarded Inc + `assertSSLCrossProduct` VARIADIC + the roster→4 + the POSITIVE arm ⚠️ THE ROW'S LOAD-BEARING TASK

**Files:**
- Modify: `internal/listener/manager.go` (the guarded Inc after `:1277`)
- Modify: `internal/listener/manager_test.go` (`sslLeafRoster`; `assertSSLCrossProduct`; `startOneWayTLSListener`; `TestServeConnection_SSLNoCertificateIncrements`)

**Interfaces:**
- Consumes: T1's `rt.sslNoCertificate`; `tlsConn` (`*crypto/tls.Conn`, declared `manager.go:1259` in the block BODY, not the if-init — **in scope, no plumbing owed**); `mkDownstreamTSInline` (`manager_test.go:627`, takes **`string`** PEMs); `handshakeTestPKI` (`:4032-4038`, holds **`[]byte`**); `mkClusterMgr`/`startEchoBackend`/`mkTLSListener`/`mkTLSChain`/`mkTcpProxyFilter`/`mkBoot`/`testHTTPRegistry`/`mkTestPKI`; `pollCounter` (`quic_test.go:34`) and `counterValue` (`quic_test.go:66`).
- Produces: `var sslLeafRoster []string`; `assertSSLCrossProduct(t, reg, addr string, wantSuffixes ...string)`; `startOneWayTLSListener(t, pki) (*stats.Registry, string)`; `TestServeConnection_SSLNoCertificateIncrements`.

⚠️⚠️ **READ §1.3 BEFORE STARTING.** The SPEC and the router both say the positive arm is this row's discriminating break. **It is not.** The roster extension is. Both are landed here, together, because neither alone defends the row.

⚠️ **DO NOT add a `handshakeOutcome` variant** (RD-NOOUTCOME). The classifier is consumed in the error branch only and never sees a successful handshake.

- [ ] **Step 1 — write the roster, the variadic helper, the one-way listener and the positive arm (the RED step)**

`sslLeafRoster` — ⚠️ **it holds BARE LEAF names while the exact-set spelling pins hold FULL names in their own independent literals. That separation is a STANDING INVARIANT: never refactor the spelling pins onto this roster**, or one misspelling would satisfy both the spelling pin and the cross-product at once.

```go
// sslLeafRoster is the COMPLETE set of ssl.* leaf names registered at a
// TLS-bearing listener's scope: the three phase-74 handshake outcomes plus
// phase 75's ssl.no_certificate. assertSSLCrossProduct partitions this roster
// into the arm's expected movers and its expected non-movers, so adding a leaf
// here automatically extends the NEGATIVE half of all four call sites.
//
// ⚠️ These are BARE LEAF names. The exact-set SPELLING pins
// (TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames,
// TestQUICListener_RegistersSSLNamesAtZero) keep their own FULLY-QUALIFIED
// literals and must NEVER be refactored onto this roster: a shared roster would
// let ONE misspelling satisfy the spelling pin and this helper simultaneously.
var sslLeafRoster = []string{"handshake", "fail_verify_error", "fail_verify_no_cert", "no_certificate"}
```

`assertSSLCrossProduct` — replace the one-hot version:

```go
// assertSSLCrossProduct is the CROSS-PRODUCT assertion every increment test
// owes (reference_probe_must_discriminate): every NAMED counter reaches exactly
// 1 AND every UNNAMED counter in sslLeafRoster is exactly 0. Without the
// negative half, Break B (exchanging the verifyError and noCert case bodies)
// would fire in all three phase-74 tests and prove nothing about WHICH arm
// routed WHERE.
//
// ⚠️ wantSuffixes is VARIADIC, not a single suffix. Phase 74 shipped this as a
// ONE-HOT assertion (`wantSuffix string`), which cannot express phase 75's
// positive arm: a completed one-way-TLS handshake legitimately moves BOTH
// ssl.handshake AND ssl.no_certificate. Variadic is the minimal change that
// preserves all three landed call sites BYTE-FOR-BYTE (a one-arg call still
// compiles) while letting the new arm name two movers.
//
// ⚠️⚠️ THE NEGATIVE HALF IS THIS ROW'S PRIMARY GUARD, NOT BOOKKEEPING. Adding
// "no_certificate" to sslLeafRoster is the ONLY thing in the repo that catches
// an UNCONDITIONAL Inc (a phase-75 predicate with its len(PeerCertificates)==0
// guard removed): it makes THIS test — the phase-74 mTLS SUCCESS arm, which
// presents a trusted client cert — assert that no_certificate stayed 0 while a
// handshake completed. Proven at the phase-75 PLAN: with the guard deleted AND
// this roster left at three leaves, the ENTIRE package is GREEN. The phase-75
// positive arm does NOT catch it — an unconditional Inc still leaves
// no_certificate at 1 on the arm that wants 1.
//
// serveConnection runs in a per-connection goroutine, so the Inc LAGS the
// client's Handshake() return — the positive half POLLS, it never sleeps. The
// negative half reads once, via counterValue, which t.Errorf's on an ABSENT
// name rather than returning a silent 0 (pollCounter would return a silent 0).
func assertSSLCrossProduct(t *testing.T, reg *stats.Registry, addr string, wantSuffixes ...string) {
	t.Helper()
	if len(wantSuffixes) == 0 {
		// PRECONDITION, not a property: a zero-suffix call has no positive half
		// at all, so its all-zeros negative half would pass VACUOUSLY even with
		// every Inc point deleted.
		t.Fatalf("assertSSLCrossProduct called with no wantSuffixes — the positive half would be vacuous")
	}
	want := map[string]bool{}
	for _, s := range wantSuffixes {
		want[s] = true
	}
	prefix := "listener." + normalizeAddr(addr) + ".ssl."
	for _, s := range wantSuffixes {
		if got := pollCounter(t, reg, prefix+s, 1, 3*time.Second); got != 1 {
			t.Errorf("%s = %d, want 1", prefix+s, got)
		}
	}
	for _, s := range sslLeafRoster {
		if want[s] {
			continue
		}
		if v := counterValue(t, reg, prefix+s); v != 0 {
			t.Errorf("%s = %d, want 0 — only %v may move on this arm", prefix+s, v, wantSuffixes)
		}
	}
}
```

⚠️ **The three landed call sites change by ZERO bytes** — `:4508`, `:4539`, `:4557` (RD-CALLSITES: the SPEC's `:4493/:4519/:4544` are the enclosing test-func declaration lines). Their doc comments DO need a touch-up where they say *"the other two are exactly 0"* / *"neither failure counter moves"* (`:4466`, `:4491-4492`).

`startOneWayTLSListener` — ⚠️ **`string(...)` conversion is mandatory (RD-CONV); written as the SPEC implies it does not compile:**

```go
// startOneWayTLSListener is startMutualTLSListener's ONE-WAY sibling: same
// single-chain / no-FilterChainMatch / tcp_proxy-to-echo shape, but the
// transport socket is mkDownstreamTSInline (server cert only — NO validation
// context, NO require_client_certificate), so the server never sends a
// CertificateRequest and a certificate-less client handshake SUCCEEDS.
//
// ⚠️ This helper exists because startMutualTLSListener CANNOT supply phase 75's
// positive arm: mkDownstreamTSMutualTLS sets require_client_certificate: true
// PLUS a trusted_ca, which yields stdtls.RequireAndVerifyClientCert
// (internal/tls/config.go:65), so a no-cert client's handshake FAILS and books
// ssl.fail_verify_no_cert instead of ever reaching the success fall-through.
//
// mkTLSChain(nil, ...) leaves FilterChainMatch nil ⇒ this is the DEFAULT chain
// ⇒ no tls_inspector listener filter is needed. mkDownstreamTSInline takes
// string PEMs while handshakeTestPKI holds []byte, hence the conversions.
func startOneWayTLSListener(t *testing.T, pki handshakeTestPKI) (*stats.Registry, string) {
	t.Helper()
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", startEchoBackend(t))
	ts := mkDownstreamTSInline(t, string(pki.serverCertPEM), string(pki.serverKeyPEM))
	l := mkTLSListener("l_ssl_oneway", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain(nil, ts, mkTcpProxyFilter(t, "c_echo")),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("listener.NewManager (one-way TLS): %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("listener.Start (one-way TLS): %v", err)
	}
	t.Cleanup(mgr.Stop)

	ls := mgr.Listeners()
	if len(ls) != 1 {
		t.Fatalf("Listeners() len = %d, want 1", len(ls))
	}
	return reg, ls[0].Addr
}
```

The positive arm:

```go
// TestServeConnection_SSLNoCertificateIncrements is phase 75's POSITIVE arm and
// the ONLY test in the repo that asserts ssl.no_certificate MOVES. Every other
// ssl.* test asserts it does NOT — so without this test a registered-but-never-
// Inc'd sslNoCertificate field would pass the entire landed suite.
//
// The listener is ONE-WAY TLS: no validation context, no
// require_client_certificate, therefore no CertificateRequest on the wire. The
// client presents NO certificate and the handshake SUCCEEDS, so serveConnection
// reaches the success fall-through with ConnectionState().PeerCertificates
// empty — the exact predicate under test.
//
// ⚠️ TWO counters move here: ssl.handshake (phase 74) AND ssl.no_certificate
// (phase 75). That is why assertSSLCrossProduct had to stop being one-hot. Both
// failure counters must stay 0: this is a SUCCESSFUL handshake, and
// ssl.no_certificate is NOT a synonym for ssl.fail_verify_no_cert.
//
// ⚠️ This arm is NOT the row's discriminating break. It catches a MISSING Inc
// and a MODE-GATED Inc; it CANNOT catch an UNCONDITIONAL Inc, because an
// unconditional Inc still leaves no_certificate at 1 here. The guard for that is
// the negative half of TestServeConnection_SSLHandshakeIncrements, via
// sslLeafRoster. Neither substitutes for the other.
func TestServeConnection_SSLNoCertificateIncrements(t *testing.T) {
	pki := mkTestPKI(t)
	reg, addr := startOneWayTLSListener(t, pki)

	// NOTE: no Certificates and no GetClientCertificate — a genuinely
	// certificate-less client. Against a one-way listener there is nothing to
	// withhold (reference_go_client_cert_withholding does not apply: the server
	// sends no CertificateRequest at all), so this is not a polite-withholding
	// artifact but the wire shape the reference books the counter on.
	conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", addr, &stdtls.Config{
		ServerName: "localhost", // mkTestPKI's server leaf CN/SAN
		RootCAs:    pki.serverRoots,
		MinVersion: stdtls.VersionTLS12,
	})
	if err != nil {
		// PRECONDITION: if the handshake did not complete, serveConnection never
		// reached the success fall-through and every assertion below is vacuous.
		t.Fatalf("precondition: certificate-less client TLS dial against a ONE-WAY listener must succeed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// NON-VACUITY: prove the handshake really COMPLETED, not merely that a
	// socket opened.
	if cs := conn.ConnectionState(); !cs.HandshakeComplete {
		t.Fatalf("precondition: client-side HandshakeComplete = false")
	}

	assertSSLCrossProduct(t, reg, addr, "handshake", "no_certificate")
}
```

- [ ] **Step 2 — run and CONFIRM RED for the right reason**

```bash
go test ./internal/listener/ -count=1 -v -run 'TestServeConnection_SSLNoCertificateIncrements'
```
Expected: **FAIL** with `listener.<addr>.ssl.no_certificate = 0, want 1` from the POSITIVE half. ⚠️ **Confirm it is the positive half and not a `Fatalf` precondition** — a dial failure or `HandshakeComplete == false` means the listener shape is wrong, not that the Inc is missing. Also confirm the three phase-74 arms still PASS (the roster grew, but they legitimately hold `no_certificate` at 0 — the success arm presents a trusted cert, and the two failure arms `return` at `manager.go:1274` before the success path).

- [ ] **Step 3 — add the guarded Inc (the GREEN step)**

`internal/listener/manager.go`, inside the EXISTING `if selected.tlsCfg != nil` block:

```go
		// phase 74: a COMPLETED downstream TLS handshake.
		rt.sslHandshake.Inc()
		// phase 75 (ADR-0297): ...and whether it presented a client certificate.
		// UNCONDITIONAL on client-auth mode — the reference books this on a one-way
		// TLS listener that never sends a CertificateRequest (SPEC §3.1, wire-proven).
		if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
			rt.sslNoCertificate.Inc()
		}
		dispatchConn = tlsConn
```

- [ ] **Step 4 — run and CONFIRM GREEN**

```bash
go test ./internal/listener/... -count=1 -v -run 'TestServeConnection_SSL'
go test ./internal/listener/... -count=1
go test ./internal/listener/... -count=1 -race
```
Expected: all four `TestServeConnection_SSL*` PASS; package ok; `-race` ok.

- [ ] **Step 5 — hygiene + commit**

```bash
gofmt -l internal/listener && go vet ./internal/listener/... && golangci-lint run ./internal/listener/...
git -C <abs-worktree-path> commit -am "phase 75 T3: the guarded Inc + assertSSLCrossProduct made VARIADIC + the roster extension (the row's PRIMARY guard) + the one-way-TLS positive arm"
```

- [ ] **Step 6 — BREAK D (the row's real discriminating break) and BREAK D′ (the stacked control)**

**Break D** edit: delete the `if len(tlsConn.ConnectionState().PeerCertificates) == 0 { … }` wrapper so the Inc is unconditional. Run `go test ./internal/listener/ -count=1 -v -run 'TestServeConnection_SSL'`.
**MUST fire: `TestServeConnection_SSLHandshakeIncrements`** (the phase-74 mTLS arm) at the NEGATIVE half — `ssl.no_certificate = 1, want 0 — only [handshake] may move on this arm`. **The positive arm MUST PASS.** ⚠️ **If the positive arm fires instead, something is wrong with the arm, not with the break.**
**[RUN at this PLAN, and reproduced by the controller — fired exactly as stated.]**

**Break D′** edit: Break D **plus** `sslLeafRoster` reverted to the three phase-74 leaves. Run the FULL package.
**⚠️ MUST GO FULLY GREEN — that is the finding.** `ok github.com/pgdad/envoy-go/internal/listener`.
**[RUN at this PLAN, reproduced by the controller: `ok … 3.233s`.]** This is the demonstration that the roster extension is the row's sole guard on the pinned predicate. **A Break D′ that FAILS is equally a finding** — it would mean some other guard also covers the predicate.

Then `git restore` both files; verify clean.

- [ ] **Step 7 — BREAK E and BREAK F (they fire the SAME assertion; this is stated, not hidden)**

**Break E** edit: delete ONLY the guarded Inc block, keep the registration (the "registered but never Inc'd" counterfactual).
**MUST fire:** `TestServeConnection_SSLNoCertificateIncrements` at `ssl.no_certificate = 0, want 1`, and nothing else. **[RUN — fired as predicted.]**

**Break F** edit: add the SPEC's REFUTED mode term. ⚠️ **It COMPILES in one edit — `selected.tlsCfg` is already the `*stdtls.Config` handed to `stdtls.Server` two lines above, and `ClientAuth != NoClientCert` is exactly "the server sends a CertificateRequest", i.e. reading (b):**

```go
		if selected.tlsCfg.ClientAuth != stdtls.NoClientCert && len(tlsConn.ConnectionState().PeerCertificates) == 0 {
```
**MUST fire:** the positive arm, at the same message. **This is the demonstration that the PINNED unconditional predicate is load-bearing.** **[RUN — fired; no substitution was needed, contrary to the brief's hedge.]**

⚠️ **RECORD HONESTLY: Breaks E and F fire the IDENTICAL assertion** and are distinguishable by the EDIT, not the OUTPUT. Same defect class, so this is acceptable — **do not claim they are separately discriminated.**

`git restore` after each; verify clean.

---

## Task 4 — `internal/stats`: the `helpText` entry + `wantNames` + the two count claims (RED via `wantNames`)

**Files:** Modify `internal/stats/name.go` (the entry after `:471`; the doc at `:448` and `:451-452`); modify `internal/stats/name_test.go` (`wantNames` at `:231-235`; the doc at `:222-223`)

**Interfaces:** Produces the `helpText` key **`envoy_listener_ssl_no_certificate`** — **verified BY EXECUTION twice independently** through `ExtractTags` + `flattenToProm`, address-invariant across IPv4, wildcard and IPv6 forms. T5's fixture asserts on exactly this key.

⚠️ **THE GUARD WILL NOT CATCH YOU, AND THAT IS PROVEN.** `TestHelpText_ListenerSSLHandshakeOutcomes` iterates a HAND-LISTED roster with **no reverse direction** — nothing walks `helpText` demanding every `ssl.*` key be listed. **Executed at this PLAN: with the entry added and `wantNames` left at three, `internal/stats` is fully GREEN.** `TestHelpText_Coverage` (`:195-213`) would not catch it either — same forward-only shape, and it does not even list the three phase-74 names. ⚠️ **`grep -rn 'len(helpText)'` ⇒ 0** — the "14 entries" claim is guarded by NOTHING.

- [ ] **Step 1 — extend `wantNames` FIRST (the RED step)**

```go
	wantNames := []string{
		"envoy_listener_ssl_handshake",
		"envoy_listener_ssl_fail_verify_error",
		"envoy_listener_ssl_fail_verify_no_cert",
		"envoy_listener_ssl_no_certificate",
	}
```

And its doc at `:222-223`, recording the hazard for the next row:

```go
// ⚠️ wantNames is HAND-LISTED and there is NO REVERSE DIRECTION — nothing walks
// helpText demanding every envoy_listener_ssl_* key appear here. Landing a fifth
// ssl.* helpText entry without extending this slice leaves it SILENTLY
// UNGUARDED (EXECUTED at the phase-75 PLAN: with the phase-75 entry present and
// this slice left at three, the whole package stayed GREEN). Any future ssl.*
// leaf MUST be appended here in the same commit that adds its helpText entry.
```

- [ ] **Step 2 — run and CONFIRM RED**

```bash
go test ./internal/stats/ -count=1 -v -run 'TestHelpText'
```
Expected: **FAIL** — `helpText missing entry for "envoy_listener_ssl_no_certificate"`, then `continue`. `TestHelpText_Coverage` PASSES (it does not list the name — that asymmetry is the point).

- [ ] **Step 3 — add the `helpText` entry (the GREEN step)**

⚠️ **The BLANK SEPARATOR LINE is REQUIRED (V1-E7).** Without it gofmt re-pads all four `envoy_listener_ssl_*` keys to the longest and **dirties three phase-74 lines**:

```go
	"envoy_listener_ssl_fail_verify_no_cert": "Downstream TLS handshakes failed because no client certificate was presented where one was required.",

	"envoy_listener_ssl_no_certificate": "Successful downstream TLS handshakes in which the client presented no certificate.",
}
```

And the doc comment at `:445-455` — ⚠️ **BOTH count claims go false, not just `:448` (RD-HELPDOC):**

```go
// only. Of the 15 entries, the first 10 cover the 13 unique Prometheus names
// emitted by 06.1 (the four _Nxx counters per HCM and per cluster collapse to
// envoy_http_downstream_rq_xx and envoy_cluster_upstream_rq_xx respectively per
// Rule SN4); one is an 06.2 backpressure counter; the next three are the
// phase-74 listener-scope TLS handshake outcomes; and the last is phase 75's
// listener-scope ssl.no_certificate — a SUCCESS-PATH annotation, not a member of
// the outcome trichotomy. All four ssl.* entries have three-dot source names
// (listener.<addr>.ssl.<leaf>) that flatten under SN3 to address-free residuals
// plus an envoy_listener_address label. A name absent from this map emits its
// own name as its HELP text (prom.go), so every emitted name wants an entry.
```

- [ ] **Step 4 — run and CONFIRM GREEN**

```bash
go test ./internal/stats/... -count=1 -v -run 'TestHelpText'
go test ./internal/stats/... -count=1
gofmt -l internal/stats && go vet ./internal/stats/... && golangci-lint run ./internal/stats/...
```
⚠️ **If `gofmt -l` names `name.go`, the blank separator line is missing** — check `git diff` for unintended re-padding of the three phase-74 entries.

- [ ] **Step 5 — commit**

```bash
git -C <abs-worktree-path> commit -am "phase 75 T4: the envoy_listener_ssl_no_certificate helpText entry + wantNames (RED-first) + both stale count claims in the map's doc"
```

- [ ] **Step 6 — BREAK G, G′, G″ (the three-legged stack; `-count=1`)**

⚠️ **A break that "stays green" proves nothing unless green cannot ALSO mean "did not run"** (`reference_liveness_break_needs_failing_baseline`). All three legs are mandatory.

- **G** — delete the `helpText` entry, revert `wantNames` to three. **MUST STAY GREEN** (`ok internal/stats`) — that is the silent-staleness finding. **[RUN — green.]**
- **G′** — entry still deleted, `wantNames` extended to four. **MUST FIRE:** `helpText missing entry for "envoy_listener_ssl_no_certificate"`. **[RUN — red.]**
- **G″** — `wantNames` extended, entry RESTORED. **MUST BE GREEN.** **[RUN — green.]** This is what makes G's green mean "ran and passed" rather than "never ran".

`git restore` after each; verify clean.

---

## Task 5 — fixture `0110`: the cross-side `StatsAsserter` + `scrapeProm` + the `var _` tripwire + the SECOND precondition

**Files:** Modify `test/fixtures/0110-tls-require-client-cert-false/driver/driver.go` (+4 imports; `AssertStats`; `scrapeProm`; `var _ fixture.StatsAsserter` after `:613`)

**Interfaces:**
- Consumes: `fixture.StatsAsserter` (`test/differential/fixture/fixture.go:75-77` — `AssertStats(t TB, refAdminAddr, subjAdminAddr string)`, takes **`fixture.TB`**, returns **nothing**, order **(t, ref, subj)**); `fixture.TB` (`:64-68`, exactly `Errorf`/`Fatalf`/`Helper` — **no `Logf`**); T1's registration and T3's Inc.
- Produces: the first cross-side assertion of `envoy_listener_ssl_no_certificate`. **+0 fixtures (119).**

**⚠️ THIS LEG WAS EXECUTED AGAINST THE LIVE REFERENCE AT THIS PLAN. The headline:**

```
reference ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
subject   ssl.handshake=2 ssl.no_certificate=1 ssl.fail_verify_error=1 ssl.fail_verify_no_cert=0 (downstream_cx_total=3)
```

**Exact cross-side agreement on all five values**, and the discriminating asymmetry is real and observed on both sides (`handshake=2` vs `no_certificate=1`). Full differential suite: **`ok … 399.675s`**, exit 0, no flake. Literal wire lines:

```
reference | envoy_listener_ssl_no_certificate{envoy_listener_address="0.0.0.0_10446"} 1
subject   | envoy_listener_ssl_no_certificate{envoy_listener_address="___20016"} 1
```

⚠️ **The subject binds the IPv6 WILDCARD — the label is `___<port>`, not `0_0_0_0_<port>`.** `normalizeAddr` (`manager.go:347-349`) strips brackets and maps both `:` and `.` to `_`, so `[::]:<port>` → `___<port>`. V3 wrote the keying comment wrongly first and **caught it by execution**; the landed comment carries the scraped values inline. **Metric NAMES are byte-identical; the addresses differ ONLY in the label** — which is exactly what makes the assertion cross-side viable.

- [ ] **Step 1 — write the asserter (the RED step)**

Transplant `0111`'s `AssertStats` (`:655`) and `scrapeProm` (`:739`) with these deltas: receiver `edfDriver` → `rccfDriver` · `l_edf` → `l_rccf` · **`10447` → `10446`** (RD-0110-PORT: 10447 is `0111`'s port) · log prefix `0111` → `0110` · the arm-count prose becomes *"3 accepts / **2** handshake successes / **1** rejection"* · the roster grows to **four** names in **three** places (⚠️ the `log.Printf` carries the roster in **both** its format string **and** its args — two edits, easy to half-do) · **`scrapeProm` copies VERBATIM** apart from its doc comment's port/listener names (no collision — `0110` has no `scrapeProm`).

The `want` map, from the EXECUTED numbers above:

```go
	want := map[string]uint64{
		"envoy_listener_ssl_handshake":           2, // arms 1 AND 3 both complete a handshake
		"envoy_listener_ssl_no_certificate":      1, // arm 3 ONLY — the discriminator vs handshake=2
		"envoy_listener_ssl_fail_verify_error":   1, // arm 2, the FORCED-SEND untrusted cert
		"envoy_listener_ssl_fail_verify_no_cert": 0, // never: at require=false no-cert is HONORED
	}
```

**PRECONDITION 1** — the accept path ran, per side, `Fatalf` (the `0111`/`0097` shape).

**PRECONDITION 2 — the TLS path itself ran, per side, `Fatalf`.** ⚠️ **The comment must say WHICH SIDE it defends against (F2), because the SPEC's framing is too broad:**

```go
	// PRECONDITION 2 — the TLS path itself ran. ⚠️ downstream_cx_total > 0 is NOT
	// a sufficient decode-ran guard for ssl.* ON THE REFERENCE. Envoy C++'s
	// tcp_proxy dials the upstream at ACCEPT time, before any TLS byte is read, so
	// an instantly-refused upstream tears the downstream connection down
	// MID-handshake: downstream_cx_total still increments while the ENTIRE ssl.*
	// family stays silent. EXECUTED at the phase-75 PLAN: with the reference
	// cluster pointed at 127.0.0.1:1 the reference yields four honest zeros with
	// downstream_cx_total == 3.
	//
	// ⚠️ envoy-go does NOT share this hazard: serveConnection completes the
	// handshake at step (6) and only dispatches to the network chain at step (7),
	// so ssl.* is accounted STRICTLY BEFORE the upstream dial — the subject's
	// numbers were BYTE-IDENTICAL under a refused upstream. The guard is kept
	// per-side anyway, because a cross-side fixture is only as strong as its
	// weaker side.
	//
	// ⚠️ Its UNIQUE contribution is narrower than it looks: the three non-zero
	// rows would fail as value mismatches anyway. What only this guard does is
	// stop the want: 0 row from passing vacuously, and turn three cryptic
	// mismatches into ONE named diagnosis.
```

⚠️ **CORRECT ONE CLAIM IN THE TRANSPLANTED ABSENT-CHECK COMMENT.** The draft says collapsing to a single-value lookup *"would let the whole `ssl.no_certificate` registration be deleted while one of the four rows still passed."* **That is wrong (F4):** `ssl.no_certificate` IS `Inc`'d on this fixture's arm 3, so deleting its registration produces a nil-pointer **SIGSEGV in the subject subprocess** and the run dies at `structuralCheck` before `AssertStats` ever executes. **The counter the ABSENT check genuinely protects is `ssl.fail_verify_no_cert`** — registered here but **never `Inc`'d on any of the three arms** (arm 2 books `fail_verify_error`), so its deletion is silent and reads `0 == 0`. **Name that counter in the comment, not `no_certificate`.**

⚠️ **`var _ fixture.StatsAsserter = (*rccfDriver)(nil)` is MANDATORY** and is a **TRIPWIRE, not the dispatch mechanism** (`reference_differential_asserter_dispatch`). Dispatch is a silent type assertion at `runner_test.go:1347-1349` with **no `else`, no log, no skip**. Only **2** fixtures repo-wide carry this line today; `0110` becomes the third.

- [ ] **Step 2 — run and CONFIRM RED, then GREEN**

⚠️ **`-run '0110'` matches ZERO subtests** (`reference_differential_run_selector`). Use the full selector:

```bash
go test ./test/differential/ -count=1 -v -run 'TestDifferential/0110-tls-require-client-cert-false'
```
With T1+T3 landed this should PASS on the first run and print the two `0110 AssertStats:` log lines. **To confirm the assertion is LIVE rather than absent, run Break H first** (Step 4) — a fixture that passes is indistinguishable from one whose asserter never dispatched.

- [ ] **Step 3 — gates + commit**

```bash
gofmt -l . ; go vet ./... ; golangci-lint run ./test/...
go mod tidy -diff                                      # expect EMPTY
git -C <abs-worktree-path> diff master -- go.mod go.sum # expect EMPTY
ls -d test/fixtures/[0-9]*/ | wc -l                    # expect 119 — this row adds NO fixture
git -C <abs-worktree-path> commit -am "phase 75 T5: 0110 gains a cross-side StatsAsserter — the FIRST cross-side assertion of envoy_listener_ssl_no_certificate (ref and subj agree exactly)"
```

- [ ] **Step 4 — BREAKS H, I, J (`-count=1`, full selector, commit first)**

- **H (value):** `want["envoy_listener_ssl_no_certificate"]` 1 → 2. **MUST fire on BOTH sides** at the value check. **[RUN — fired: `ref … = 1, want 2` and `subj … = 1, want 2`.]**
- **I (the ABSENT check is load-bearing):** two legs. **I-a** — keep the two-stage form, delete the production registration of **`ssl.fail_verify_no_cert`**. **MUST fire:** `subj: envoy_listener_ssl_fail_verify_no_cert ABSENT from /stats/prometheus`. **I-b** — simplify the asserter to a single-value lookup (drop comma-ok + `continue`) with the same deletion. **MUST PASS VACUOUSLY.** **[BOTH RUN — I-a red, I-b passed while the log still printed `fail_verify_no_cert=0`.]** ⚠️ **I-b's pass is the whole point**; a PLAN that only ran I-a would have proved the check fires without proving it was necessary.
- **J (dispatch liveness):** ⚠️ **the naive form is VACUOUS** — green after a rename is consistent with both *"never ran"* and *"ran and passed"*. **Stack it:** first set a `want` value wrong and confirm **RED**; then rename `AssertStats` → `AssertStatsX` **with the `var _` line deleted** and confirm **GREEN with the `0110 AssertStats:` log lines VANISHING**. **[RUN — and with the `var _` line KEPT the rename is a COMPILE ERROR (`*rccfDriver does not implement fixture.StatsAsserter`), which is the tripwire working.]**
- **Break E (the fast-failure arm) is UNREACHABLE on `0110` and that is a FINDING, not a gap (F3).** Arms 1 and 3 must echo, so any dead upstream breaks `wantObservable` and the run dies at `structuralCheck` (`runner_test.go:1274`) long before step 10. It was reached only by neutralising `structuralCheck`, which is not a legitimate configuration. **Record it; do not spend a cycle re-attempting it.**

`git restore` after each; verify `git status --porcelain` empty.

---

## Task 6 — `0110`'s THREE stale `ssl.*` self-confessions (⚠️ SPLIT the README bullet; the third `expectations` clause INVERTS)

**Files:** Modify `test/fixtures/0110-…/README.md` (`:160-163`), `envoy.yaml` (`:24`), `expectations.yaml` (`:166-171`, plus a new leg (c))

⚠️ **No fourth stale site exists inside `0110/`** — a 14-pattern sweep confirms exactly three, and **`driver/driver.go` carries ZERO** (its `PLAN-65 C3` references at `:376`/`:544` are about ALERT TEXT and stay true).

⚠️ **DO NOT TOUCH** `README.md:164-165` and `expectations.yaml:173-175` — the `sds.<secret>.*` counters, correctly still unasserted. `0111` kept its equivalent.

- [ ] **Step 1 — `README.md:160-163`: SPLIT the bullet, do not delete it**

Current text bundles a now-FALSE `ssl.*` claim with a **STILL-LIVE** `/listeners` guard, and ⚠️ **the live half starts MID-LINE at `:161` ("Never assert")**:

```
- **No `ssl.*` stats** — envoy-go emits none, so a verdict `StatsAsserter` is
  infeasible (inherits PLAN-65 C3); a pre-existing framework gap. Never assert
  `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as
  listener liveness.
```

Replace with TWO bullets — the first RETIRED with the four asserted counts, the second **preserved verbatim** and labelled independent. Use `0111/README.md:167-174` as the form template.

- [ ] **Step 2 — `envoy.yaml:24`: the comment only, NO config change**

`# identity and NOT a stat (envoy-go emits no ssl.* family; see README).` ⚠️ **`0111`'s corrected header is MULTI-LINE (`:23-27`), so this line GROWS** (RD-0111-TEMPLATE). State that the accept/reject verdict is now ADDITIONALLY cross-checked at the counter layer, naming the four values.

- [ ] **Step 3 — `expectations.yaml:166-171`: THREE clauses, and the third INVERTS**

Clause 1 (*"envoy-go emits NO `ssl.*` stats whatsoever … a verdict StatsAsserter is therefore INFEASIBLE"*) — **FALSE**, retire. Clause 2 (the `/listeners` guard) — **LIVE**, preserve. Clause 3 (*"The accept/reject CONTRAST discharges the proof obligation and is strictly STRONGER than a subject-only stat — it is cross-side"*) — ⚠️ **INVERTS once the fixture asserts cross-side.**

⚠️ **The template AMENDS clause 3 rather than deleting it.** `0111/expectations.yaml:196-207` (the block is `:196-207`, **not** `:196-201`) did exactly this: it kept *"strictly STRONGER than a subject-only stat"* and **appended** *"— it is cross-side, and it now has a cross-side STAT beside it."* Follow that. Add the sharper reason `0110` can give and `0111` cannot: **the accept/reject contrast CANNOT distinguish arm 1 from arm 3 (both ACCEPT), whereas `no_certificate=1` against `handshake=2` does.**

Also add a **new leg (c)** to the `## Asserted` section (`:124-142`, currently legs (a) and (b)), modelled on `0111/expectations.yaml:158-166`.

- [ ] **Step 4 — verify + commit**

```bash
grep -rn 'ssl\.\|StatsAsserter\|infeasible\|INFEASIBLE\|emits no\|emits NO\|framework gap' \
  test/fixtures/0110-tls-require-client-cert-false/
# every surviving hit must be TRUE at this tip; the sds.* boundary notes must still be present
go test ./test/differential/ -count=1 -run 'TestDifferential/0110-tls-require-client-cert-false'
git -C <abs-worktree-path> commit -am "phase 75 T6: retire 0110's three stale ssl.* self-confessions — SPLIT the README bullet, AMEND the inverting cross-side clause, add leg (c)"
```

---

## Task 7 — `0111`: `ssl.no_certificate` joins the NAMED-UNASSERTED lists (⚠️ NOT the value rosters — §2.4)

**Files:** Modify `test/fixtures/0111-…/README.md` (`:167-174`) and `expectations.yaml` (`:197-203`)

⚠️ **`0111/driver/driver.go` is BYTE-UNTOUCHED by this task.** Its three value rosters stay at THREE names. **The decision and its reversal are recorded in §2.4** — in short: `0111` runs `require_client_certificate: true` on both sides, so a success-path no-cert annotation reads **0 on every arm, structurally**, and a value pin would document a vacuous `0 == 0` as coverage; the real cross-side assertion lives at `0110` with a discriminating non-zero.

⚠️ **But the prose MUST change, because it is a CLOSED ENUMERATION (RD-0111-CLOSED).** `README.md:167-174` says the driver *"pins all three"* and then names the remainder (*"Still out of scope: `ssl.connection_error`"*); `expectations.yaml:197-203` names *"Still UNasserted from that family: `ssl.connection_error` … and the `ssl.ciphers/curves/versions` breakdowns"*. **A name in NEITHER list reads as ASSERTED.** Leaving `no_certificate` out of both is the one genuinely wrong outcome.

- [ ] **Step 1 — add `ssl.no_certificate` to both named-unasserted lists, WITH THE REASON**

State, in both files: the phase-75 name exists and is asserted cross-side — **at `0110-tls-require-client-cert-false`, not here** — because this fixture's `require_client_certificate: true` shape holds it at 0 on every arm, so a value pin here would be a vacuous `0 == 0`. ⚠️ **Also fix `expectations.yaml:159`**, which enumerates the three-name family.

- [ ] **Step 2 — verify the driver really is untouched, and the fixture still passes**

```bash
git -C <abs-worktree-path> diff --stat master -- test/fixtures/0111-tls-cvc-empty-dynamic-fallback/driver/driver.go
# ⚠️ MUST BE EMPTY
go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback'
```
⚠️ **Running `0111` is MANDATORY even though its driver is untouched** — T1 adds a name to the subject's scrape, and `0111`'s asserter iterates a NAMED SUBSET (verified by code read, **not** by execution at this PLAN).

- [ ] **Step 3 — commit**

```bash
git -C <abs-worktree-path> commit -am "phase 75 T7: 0111's closed enumeration gains ssl.no_certificate as NAMED-UNASSERTED (its require=true shape would make a value pin vacuous)"
```

---

## Task 8 — BEHAVIOR_CONTRACT: eleven in-place edits + ONE new ledger line (⚠️ NO new paragraphs)

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md`

⚠️⚠️ **APPEND IN PLACE; DO NOT INSERT PARAGRAPHS INTO THE TLS SUBSECTION (F7).** `:1849/:1851/:1853/:1855/:1857/:1859/:5002` are all cited BY LINE from `ROADMAP.md:137`, `STATE.md:20/46/48`, phase-75 `SPEC.md:20/28/29/276/334-343` and `BRAINSTORM.md:136/240/321`. **The new ledger line after `:5002` must be the ONLY line-adding edit in the file** (`5744 → 5746`). Verified over an applied scratch copy: every cited anchor keeps its exact line number.

- [ ] **Step 1 — the eleven in-place edits**

| site | edit |
|---|---|
| `:831`, `:847` | `Stat surface UNCHANGED at **1204**` → **`1205`** (both). These ARE live restatements — phase 74 bumped exactly these two 1201 → 1204. |
| `:916` | extend the RETIRED-names sentence to a FOURTH name, and state that **`0111` does NOT assert it** (its `require=true` shape) — the cross-side assertion is at `0110`. ⚠️ The other two `three` tokens on `:916` are `0111`-scoped and stay TRUE. |
| `:918` | `envoy-go now emits three …ssl.* counters` → **four** (three at phase 74, a fourth at phase 75); and the departure is UNCHANGED by phase 75 too, for the same reason (envoy-go BOOT-FAILS, so no handshake ever completes). |
| `:928` | the C3 RETIRED/SURVIVING split grows to **FOUR** fixed names (three by ADR-0296, a fourth by ADR-0297); name `0110` as the assertion site and say why not `0111`. |
| **`:1849`** | **EXTEND the heading** — ⚠️ SPEC §9 says no two-ADR precedent exists; **there are 16, and `:785` is the exact "later phase extends" shape** (RD-BC-HEADING). Use `:785`'s `<what> per phase <N> ADR-XXXX` semicolon form. |
| `:1851` | the roster grows to four fully-qualified internal names + four Prometheus forms; say the fourth is a **SUCCESS-PATH annotation, NOT a failure bucket**. ⚠️ **REMOVE the `three-fifths` ratio, do not bump it to four-fifths (F9)** — it was already wrong at the phase-74 tip (`:928`, same phase, enumerates FOUR surviving families ⇒ 3+4=7). State the retirement as an ENUMERATION. |
| `:1853` | ⚠️ **APPEND IN PLACE** the semantics block: the predicate, its **NO client-auth term**, and how it differs from `fail_verify_no_cert` **in BOTH directions** (over-counting accepted anonymous connections; double-booking a genuine no-cert rejection). Cite **ADR-0297 §Context ¶2/¶3** for the wire evidence — ⚠️ **do NOT present those probe figures as re-derived here (§1.4)**. |
| `:1855` | clarify that *"three names and not four"* means **`ssl.connection_error`**, NOT `ssl.no_certificate`; and record that **this subsection is the true referent of the dangling "BEHAVIOR_CONTRACT B5/B6" citations** — there is NO B-numbered scheme in this file. |
| `:1857` | `all three counters` → `all four`; add that the fourth **INHERITS** QUIC parity from phase 74's family-level probe and was **NOT re-probed**, and that the inheritance is sound because the Inc site is unreachable for `kindQUIC` for a **structural, name-independent** reason. |
| `:1859` | the family is *"three names at phase 74, **FOUR** as of phase 75"*; `0110`'s asserter takes the identical label-ignoring posture. ⚠️ **Do NOT touch `:1859`'s other three `three` tokens** — they refer to the three DIVERGENCE CLASSES, not the counters. |

- [ ] **Step 2 — the new ledger line (the ONLY line-adding edit)**

Insert after `:5002`, before the `### Forward-pointer note (26.3)` heading. ⚠️ **Match the TAIL entry's byte form, NOT `:5000`'s** (RD-LEDGER): em dash **U+2014**, arrow **`→` U+2192** (never ASCII `->`), and **bold running THROUGH the parenthetical to the colon** — `…(+1) (<descriptor>):**`. Open with:

```
**Phase 75 — 1204 → 1205 (+1) (the FOURTH listener-scope `ssl.*` name — the FIRST SUCCESS-PATH handshake annotation):**
```

Body records: +1 name, registered in the block that **ALREADY** gates on `rt.tlsMode` (no new gate/classifier/registration function), Inc'd on the success fall-through guarded on `len(…PeerCertificates) == 0` **ALONE**; plaintext gains ZERO, QUIC gains it permanently-zero (PARITY); fixtures **119 → 119** (`0110` EXTENDED, and **NOT `0111`** — say why); BackendKind 38 → 38; fuzzers +0; ZERO new packages/modules/production-imports/exported symbols; records **ADR-0297**.

⚠️ **And it records BOTH ledger gaps without back-filling either (F10):** the known `1200 → 1201`, **and a SECOND, previously unrecorded — `Phase 46.1b` closes at `1198` while `Phase 47.1` opens at `1200`, an unattributed +2.** The **+1 DELTA** is asserted with confidence; the absolute **1205** is documentary. **Do NOT fabricate lines to close either gap** — that would be inventing a record.

- [ ] **Step 3 — verify mechanically**

```bash
B=docs/envoy-go/BEHAVIOR_CONTRACT.md
wc -l $B                                   # 5744 -> 5746 (exactly +2)
grep -n 'ssl\.handshake' $B                # anchors 916/928/1851/1857/1859 UNMOVED; 5002 unmoved
grep -c '1204' $B ; grep -c '1205' $B      # expect 2 lines / 3 lines
grep -c '1201' $B                          # expect UNCHANGED (1 line)
sed -n '962p' $B | grep -c 'ssl\.no_certificate'          # 1 — :962 must be BYTE-IDENTICAL
sed -n '5004p' $B | cat -A | head -c 200   # the new line: verify U+2192 (M-bM-^FM-^R) and `):**`
grep -cE '^#{2,4} .*ADR-[0-9]+.*ADR-[0-9]+' $B            # 16 -> 17 (the :1849 heading joins them)
```
⚠️ **Re-derive `:962`'s offsets and confirm they still read 627 (CHARACTER) / 630 (BYTE), 1002 chars / 1007 bytes.** That anchor has survived three challenges; do not become the fourth.

- [ ] **Step 4 — commit**

```bash
git -C <abs-worktree-path> commit -am "phase 75 T8: BEHAVIOR_CONTRACT — the fourth ssl.* name, both totals to 1205, an EXTENDED :1849 heading (SPEC §9's no-precedent claim refuted), and a ledger line recording BOTH chain gaps"
```

---

## Task 9 — the stale-prose sweep: every site that goes false at +1 and produces NO red

**Files:** Modify `internal/listener/manager.go` (`:385-386`), `internal/listener/manager_test.go`, `internal/listener/quic_test.go`

⚠️ **These sites fail NOTHING. That is exactly why they need a task** — the phase-74 IMPL found three such comments after the fact, one of them a dangling cross-reference its own rename had orphaned.

- [ ] **Step 1 — `manager.go:385-386`, `handshakeOutcome`'s doc — PROSE ONLY**

It reads *"classifies a downstream TLS handshake result into the **three** counted buckets plus a fourth that counts NOTHING"* — actively confusing at +1 (four *counters*, still three *outcome buckets* + `outcomeOther`). ⚠️ **State explicitly that phase 75 adds NO `handshakeOutcome` variant** — `ssl.no_certificate` is a success-path annotation outside the switch, Inc'd after the error branch has `return`ed. **Anything touching `handshakeOutcome` is a design error for this row (RD-NOOUTCOME).**

⚠️ **`classifyHandshakeErr` (`:424`) has NO doc comment** (RD-CLASSIFY) — the comment above it belongs to `const noClientCertErrText` (`:422`). **Do not "amend the comment above `classifyHandshakeErr`"; there is no such comment.**

- [ ] **Step 2 — the `manager_test.go` / `quic_test.go` roster**

`manager_test.go`: `:1936` (*"the three `listener.<addr>.ssl.*` counters"*) · `:1987-1989` (section banner, *"carries exactly three ssl.* names"*) · `:1993-1997` (`listenerSSLNames`' doc, *"WOULD PASS WITH ALL THREE NAMES MISSPELLED"*) · `:4466` and `:4491-4492` (*"the other two are exactly 0"* / *"neither failure counter moves"*) · `:4561` (*"three ssl.* pointers"*).
`quic_test.go`: **`:65`** (`counterValue`'s own doc, *"all three are zero"*) · `:226` · `:272` · `:295`.

*(`:1940`, `:2019`, `:2126`, `:2152`, `:2203` were handled at T1/T2; `name.go:448`/`:451-452` and `name_test.go:222-223` at T4.)*

- [ ] **Step 3 — verify no "three" claim about the ssl family survives, and commit**

```bash
grep -rn -i 'three' internal/listener/*.go internal/stats/name*.go | grep -i 'ssl\|counter\|pointer'
# every surviving hit must be TRUE at four names (e.g. "the three phase-74 names" is fine; "the three ssl.* counters" is not)
gofmt -l internal/listener internal/stats && go vet ./internal/listener/... ./internal/stats/... && golangci-lint run ./internal/listener/... ./internal/stats/...
go test ./internal/listener/... ./internal/stats/... -count=1
git -C <abs-worktree-path> commit -am "phase 75 T9: the stale-'three' prose sweep across two production and three test files — none of these fails a test, which is why it is a task"
```

⚠️ **RECORDED, NOT FIXED (§2.5):** `internal/statssink/registration_test.go:25`/`:51`/`:80` (*"stays 1200 / 1196"*, stale since **phase 49**, unasserted by code) and `statsd_tcp.go:78` (*"~1200 stats"*, already hedged). **Fixing them would widen the delta into a package this row does not touch.** State the disposition in `PROGRESS.md`.

---

## Task 10 — VERIFY: the six-gate + layering + the full differential + `-race` + counts + a TWO-CATEGORY envelope audit

**Files:** none modified. This task only measures — and it must measure the RIGHT things with the RIGHT commands.

- [ ] **Step 1 — the six-gate**

```bash
W=<abs-worktree-path>
pwd; git -C $W rev-parse --abbrev-ref HEAD; git -C $W rev-list --count master..HEAD   # TRIPWIRE: never master
gofmt -l .                                        # silent
go vet ./...                                      # silent
go build ./...                                    # silent
go mod tidy -diff                                 # EMPTY
git -C $W diff master -- go.mod go.sum            # EMPTY
golangci-lint run ./...                           # silent
```

- [ ] **Step 2 — the full differential, then `-race`**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -20
# expect ok, 119 subtests / 119 PASS / 0 SKIP / 0 FAIL. ~400s at this PLAN's measurement.
comm -3 <(ls -d test/fixtures/[0-9]*/ | sed 's#test/fixtures/##;s#/$##' | sort) <(<subtest names, sorted>)
# expect EMPTY — the fixture-directory set and the subtest set must be EQUAL
go test ./... -count=1 -race 2>&1 | grep -v '^ok' | head -40
```
⚠️ **`-race` over `./test/differential/` was NOT run at this PLAN (§1.4).** Either run it here or **say plainly that it was skipped and why** — do not let its absence read as coverage. ⚠️ **Known flakes must be isolate-re-run, not re-classified**: `internal/cluster` `TestOutlierDetector_ConcurrentEjectExactlyOnce`; `internal/boot TestSDSEndToEnd_FetchFailure_BootFailsClosed`; the `internal/xds` SDS init-fetch-timeout test; `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`; `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`; the full-suite `subject ready: EOF` startup flake. ⚠️ **NONE fired in any of this PLAN's three build worktrees, including a full 400 s differential run — so a failure here is more likely REAL than at most stages.** Isolate-re-run, then say which classification you reached and on what evidence.

- [ ] **Step 3 — the envelope, audited in TWO SEPARATE CATEGORIES**

⚠️ **The phase-74 gate command is UNRELIABLE (RD-IMPGATE)** — it returns false-positive hits on map-literal lines and exits 0, so anyone gating on the exit code reads a PASS as a FAIL. Use the extracted-import-block diff:

```bash
impblock() { awk '/^import \($/{f=1;next} f&&/^\)$/{exit} f&&NF{gsub(/^[ \t]+/,"");print FILENAME"\t"$0}' "$1"; }
for f in internal/listener/manager.go internal/stats/name.go; do
  mkdir -p /tmp/p75base/$(dirname $f); git -C $W show master:$f > /tmp/p75base/$f
done
{ impblock /tmp/p75base/internal/listener/manager.go; impblock /tmp/p75base/internal/stats/name.go; } | sort > /tmp/p75.base
{ impblock internal/listener/manager.go;              impblock internal/stats/name.go;              } | sort > /tmp/p75.head
diff -u /tmp/p75.base /tmp/p75.head && echo "GATE PASS: +0 PRODUCTION imports"
```
⚠️ **This gate was NEGATIVE-CONTROLLED at the PLAN**: inserting `"strconv"` into a scratch `manager.go` makes it exit 1 and print the added line — **so a green result is evidence, not silence.**

**Category 2 — TEST-side imports GREW, and that is PERMITTED.** `0110/driver/driver.go` gains exactly `log`, `math`, `net/http`, `strconv`. **Report the two categories separately and never let a test import read as a production violation.**

```bash
go doc -all ./internal/listener ./internal/stats > /tmp/p75.doc.head
git -C $W stash 2>/dev/null; go doc -all ./internal/listener ./internal/stats > /tmp/p75.doc.base; git -C $W stash pop 2>/dev/null
diff /tmp/p75.doc.base /tmp/p75.doc.head && echo "GATE PASS: ZERO new exported symbols"
go list -deps ./internal/listener | sort > /tmp/p75.deps.head    # compare against master: expect IDENTICAL, no new edge
```
*(If `git stash` is unavailable on a worktree, derive the baseline from a `git worktree add` of `master` instead — do NOT `git checkout` in place, it detaches HEAD.)*

- [ ] **Step 4 — counts, re-run MECHANICALLY (never copied)**

```bash
ls -d test/fixtures/[0-9]*/ | wc -l                                  # 119  (+0)
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l             # 55   (+0)
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go     # tail VALUE 38 (39 constants declared, 0-38)
grep -cE '^\s+[a-z]' go.mod                                          # 67 requires (lineage figure 2)
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1       # ADR-0297
grep -c '^## ADR-0298' docs/envoy-go/DECISIONS.md                    # 0
```
⚠️ **Stat surface 1205 has NO mechanical command** and is NOT re-derived — the **+1 delta** is asserted, the TOTAL is documentary, and it now rides **two** documented ledger gaps (F10).

- [ ] **Step 5 — the BYTE-UNTOUCHED roster, set-differenced against the EDIT roster FIRST**

⚠️ **`reference_plan_schedules_edits_to_a_byte_gated_file`** — phase 73 hit this as its own SEVERE-1. **Compute the set difference before running the sha256 gate**, and confirm `manager_test.go`, `quic_test.go`, `name_test.go`, `0110/driver/driver.go` and the `0111` **prose** files appear in the EDIT roster and NOT in the gate roster, while `0111/driver/driver.go` and `0110/envoy-go.yaml` appear in the GATE roster only.

- [ ] **Step 6 — record everything in `PROGRESS.md` and commit**

```bash
git -C <abs-worktree-path> commit -am "phase 75 T10: the six-gate + full differential + -race + a TWO-CATEGORY envelope audit using the negative-controlled import gate"
```

---

## Task 11 — ADR-0297 completed IN PLACE (⚠️ **and CORRECTED**) + the ADR-0296 blockquote + ROADMAP + STATE + the row flip

**Files:** Modify `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/STATE.md`, `next-prompt.txt`

- [ ] **Step 1 — complete ADR-0297 IN PLACE, and fix its TWO OWN DEFECTS**

Append `### Decision` + `### Consequences` **after the RETAINED footer** at `:17350` (the ADR-0295/0296 shape, **not** ADR-0286's); rewrite the STATUS blockquote PROPOSED → **COMPLETE** and future → past tense. **No renumber; tail stays ADR-0297; next-free ADR-0298.**

⚠️ **§Context carries ELEVEN paragraphs (¶1-¶11) — more than SPEC §14's eight-item list implies. Do NOT duplicate what ¶1-¶11 already say** (RD-ADR0297).

⚠️ **FIX ¶7's SELF-FALSIFYING GREP (F5).** ¶7 asserts *"exactly ONE hit — `:17308`, the citing sentence itself"*, but `grep -n 'VERIFYIFGIVEN'` now returns **TWO lines (`:17308`, `:17340`) / THREE occurrences** — ¶7's own text made it false. **This is the same species the phase-74 IMPL had just fixed in ADR-0296 ¶3, reproduced one ADR later.** ⇒ **restate the property with NO number, or scope the grep to ADR-0296's line range.**

⚠️ **FIX ¶9's REFUTED FORM RULE (F6).** ¶9 says the discriminator is **SELF vs OTHER-ADR (n=4)**. The real population is **n=7 with a THIRD form**, and **`:17211` is a phase-73 correction to a DIFFERENT landed ADR (ADR-0294) rendered INLINE** — so self-vs-other does not discriminate either. **The surviving discriminator is graft SCALE:** inline attaches to the specific clause it contradicts; **indented stands alone where an entire bullet or paragraph is re-characterised** (both indented instances re-characterise a whole ADR-0286 bullet). ⇒ **the INDENTED form stays correct for phase 75; replace ¶9's reason.** ⚠️ **Record that the prescription has now survived THREE different wrong justifications — family, then self-vs-other, now scale** — and that ¶9's subsidiary claims DO hold.

- [ ] **Step 2 — the ADR-0296 §Decision (g) correction blockquote**

Insert after `:17308` and its existing blank `:17309`, then a **NEW blank line** (a `###` heading follows). Leading bytes MUST be **`0x20 0x20 0x3E 0x20`** — `od -c`-verified byte-identical to `:16901` and `:16910`, the only two instances.

It must: **LEAD WITH WHAT SURVIVES** — *no `ssl.no_certificate` counter was owed **in phase 74**, whose sole stat-asserting fixture `0111` sets `require_client_certificate: true` on BOTH sides, so a success-path annotation necessarily reads 0 on every arm; **the deferral was RIGHT and phase 74's row was correctly scoped*** — then record that the stated REASON fails on both halves (phase 67 landed verify-if-presented across all three validation-source shapes, `internal/tls/config.go:79-84`, row 67 `done`, `0110` driving it since the phase-67 IMPL; and the supporting citation resolves to its own sentence). **Phrase it as a DOCUMENTARY FINDING, NOT as criticism of phase 74. Claim NO ORDINAL.** Also note, separately, ADR-0296 ¶7's uncorrected `registry.go:107` mis-cite (`:107` is the DUPLICATE panic; the INVALID-NAME panic is `:117`) and that the *"BEHAVIOR_CONTRACT B5/B6"* pointer resolves to nothing.

⚠️ **DO NOT spell the all-caps `VERIFYIFGIVEN` token and DO NOT restate any whole-file grep count** (F5) — every restatement mints another counter-example. Refer to it descriptively.

- [ ] **Step 3 — the FIVE `DECISIONS.md` citation fixups the insert forces (F8)**

The blockquote shifts `:17310 → :17312` and **`:17314 → :17316`**. The latter is cited from **`ROADMAP.md:137`** and **`STATE.md:48`** — both edited in this task anyway — plus phase-75 `SPEC.md:414` and `BRAINSTORM.md:321`. ⚠️ **Fix `ROADMAP.md:137` and `STATE.md:48`. Do NOT rewrite the frozen `SPEC.md`/`BRAINSTORM.md`** — no convention requires it and the lineage treats them as historical records. **Say so in `PROGRESS.md` rather than leaving it ambiguous.**

- [ ] **Step 4 — ROADMAP: the row flip + its THREE stale claims + the B5 over-breadth**

Row 75 (`:137`): `in-progress` → **`done`** (ADR-0106, the SOLE leg — a SINGLE FLAT ROW). **In the SAME edit, fix all three stale claims (RD-ROADMAP-75):** (i) *"(the discriminator for that form is the **ADR FAMILY, not the phase gap**)"* — refuted, and its replacement is **graft SCALE**, not self-vs-other (F6); (ii) *"This is the **THIRD** internal mis-pointer in ADR-0296"* — **not established; claim no ordinal**; (iii) *"`ProbeAdmin` at `:552`"* → **`:554`**. **Plus (iv):** the cell's *"all eight `B5` hits are `AMEND-B5`/phase-25.2 Wasm"* is **over-broad** — `:4685` is phase-**29.1 mongo** (F11); the conclusion stands.

- [ ] **Step 5 — the deferred-sentence narrow (`ROADMAP.md:205`)**

Edit the **RETIRED clause**, not the candidates list — the sentence never named `no_certificate`, so **this is NOT a name deletion** (§2.6). ⚠️ **REMOVE the `three-fifths` ratio rather than bumping it (F9).**

⚠️ **HARD CONSTRAINT: the matched sentence contains ZERO interior periods — that is WHY `[^.]*\.` binds. The replacement must introduce NO `.`** — no `manager.go`, no `internal/…`, no abbreviation.

**Verify in a SCRATCH COPY before landing:**

```bash
cp docs/envoy-go/ROADMAP.md /tmp/p75rm.md   # apply the edit to the COPY first
grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' /tmp/p75rm.md    # MUST stay 3
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:' /tmp/p75rm.md | wc -l   # MUST be 3
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' /tmp/p75rm.md | tail -1 | rev | cut -c1-20 | rev
# MUST still end at "force-trace." — if the regex stops early, an interior period was introduced
```
**[Verified at this PLAN over an applied scratch copy: check (2) ⇒ 3, occurrence-count ⇒ 3, terminator intact, match length 999 → 1033, interior periods 0 → 0.]**

- [ ] **Step 6 — STATE + `next-prompt.txt`**

**STATE §Current rolled IN PLACE** (lifecycle 3 → **DONE**; row 75 `done`); §Recent re-capped at **FIVE** **with its PREAMBLE updated** (the ADR-0288 rule); the `:48` citation fixup from Step 3. ⚠️ **The three ADR-0288 singleton greps return 2, not 1** — the second hit is `STATE.md:7`, the RULE STATEMENT itself. **Verify with `grep -n` and NEVER "fix" the count to 1; that would delete the rule.**

`next-prompt.txt` rolled to the **phase-76 BRAINSTORM** (⚠️ **TRACKED despite `.gitignore`**; edit it in the stage worktree). The roller SELF-PICKS per the standing directive; SPEC §13 records `fault.abort.grpc_status` as the recommended next opening (7-9 tasks; the only identified candidate clearing a sentinel check-(3) blocker; **ONE probe owed**).

- [ ] **Step 7 — the sentinel, run MECHANICALLY TWICE**

Worktree AND landed master post-push (the phase-72/73/74/75 precedent: a stage worktree can disagree with what landed). ⚠️ **Check (1) should go SILENT at this IMPL** (row 75 was the last non-`done` chartered row) — **but record the ACTUAL output either way.** Check (2) must stay **3**; check (3) must still print `NEVER OPENED: gRPC/Runtime/WASM`. ⚠️ **Re-derive the blind-spot figure; it is 107 rows / 103 matched / 4 misses at this tip and has been recorded WRONG in two consecutive lineages** (RD-BLINDSPOT).

- [ ] **Step 8 — verify ADR shape and commit**

```bash
D=docs/envoy-go/DECISIONS.md
awk '/^## ADR-0297/,0' $D | grep -c '^### Context'       # 1
awk '/^## ADR-0297/,0' $D | grep -c '^### Decision'      # 1  (was 0)
awk '/^## ADR-0297/,0' $D | grep -c '^### Consequences'  # 1  (was 0)
awk '/^## ADR-0297/,0' $D | grep -c '^\*(§Decision'      # 1  (the footer is RETAINED)
grep -oE '^## ADR-[0-9]+' $D | tail -1                   # ADR-0297 (tail UNCHANGED)
grep -c '^## ADR-0298' $D                                # 0
grep -c '^  > ' $D                                       # 2 -> 3
awk '/^## ADR-0296/,/^## ADR-0297/' $D | grep -c '^### Decision'   # 1 (ADR-0296 stays COMPLETE)
git -C <abs-worktree-path> commit -am "phase 75 T11: ROW 75 -> done. ADR-0297 completed IN PLACE and CORRECTED (its own para7 self-falsifying grep + para9's refuted form rule), the ADR-0296 (g) blockquote, the ROADMAP row flip + THREE stale claims + the narrow"
```

---

## Break map — TEN breaks, and EIGHT of them ALREADY RAN at this PLAN

⚠️ **Unlike phase 74's PLAN, whose breaks were untested by construction, most of these are EXECUTED results, not predictions.** `[RUN]` = executed at this PLAN in a private build worktree (and where noted, reproduced by the controller). `[IMPL]` = still owed.

| # | Task | Edit | MUST fire | Status |
|---|---|---|---|---|
| **A** | T1 | `"ssl.no_certificate"` moved to index 1 in the name-set `want` | the `reflect.DeepEqual` name-set line, on ORDER | **[RUN] fired** |
| **B** | T1/T3 | delete the registration, KEEP the Inc | ⚠️ **`-run` ISOLATED**: the name-set `DeepEqual` + the POINTER assertion. **Full-package it is a SIGSEGV naming no test** | **[RUN] fired both ways; the isolation requirement IS the finding** |
| **C** | T2 | same edit, `-run 'TestListenerMetrics_GateMatchesInc'` | the POINTER assertion and **ONLY** it; the three build-time predicates must print **NOTHING** | **[RUN] fired; predicates silent** |
| **D** | T3 | delete the `len(…)==0` wrapper ⇒ unconditional Inc | ⚠️ **`TestServeConnection_SSLHandshakeIncrements`** (the PHASE-74 arm), NEGATIVE half. **The positive arm MUST PASS** | **[RUN + controller-reproduced] fired; SPEC/router prediction INVERTED** |
| **D′** | T3 | Break D **+** roster reverted to three leaves | ⚠️ **MUST GO FULLY GREEN** — the finding | **[RUN + controller-reproduced] `ok … 3.233s`** |
| **E** | T3 | delete ONLY the Inc, keep registration | the positive arm's positive half, nothing else | **[RUN] fired** |
| **F** | T3 | add the refuted mode term `selected.tlsCfg.ClientAuth != stdtls.NoClientCert &&` | the positive arm — proving the UNCONDITIONAL predicate is load-bearing | **[RUN] fired; COMPILES in one edit, no substitution needed** |
| **G/G′/G″** | T4 | (G) entry deleted + `wantNames` at 3 · (G′) entry deleted + `wantNames` at 4 · (G″) `wantNames` at 4 + entry restored | ⚠️ G **MUST STAY GREEN**; G′ **MUST FIRE**; G″ **MUST BE GREEN** | **[RUN] all three legs as stated** |
| **H** | T5 | `want["…no_certificate"]` 1 → 2 | the value check, on BOTH sides | **[RUN] fired both sides** |
| **I** | T5 | (I-a) two-stage form + delete `fail_verify_no_cert`'s registration · (I-b) simplified single-lookup, same deletion | I-a **MUST FIRE** the ABSENT branch; I-b **MUST PASS VACUOUSLY** | **[RUN] both; I-b's pass is the demonstration** |
| **J** | T5 | rename `AssertStats`, **stacked on a failing `want`** | RED named, then **GREEN with the log lines VANISHING** when renamed | **[RUN] and with `var _` kept it is a COMPILE ERROR** |

**Declared MUST-NOT-FIRE / MUST-STAY-GREEN: D′ and G.** Either one behaving otherwise is equally a finding.
**Unreachable and recorded as such: the fast-failure arm on `0110`** — `structuralCheck` kills the run at step 8 before step 10 (F3). Do not re-attempt.
**Beyond the SPEC's implicit A–G this adds THREE: D′, I-b, and G″** — each exists because a naive version of its parent was vacuous.

---

## Self-review against SPEC-75

**Spec coverage.** §3.1 predicate → T3 (+ Break F). §3.2/§3.3/§3.4 → no code owed; recorded in T8's prose. §3.5 fixture numbers → T5, **executed**. §3.6 the race → T5's precondition 2, **and F2 corrects its scope**. §3.7 QUIC → T1's `want` + T8 `:1857`; §1.4 records it stays unprobed. §4 primitives / +0 imports → T10 (RD-IMPGATE command). §5 identifier hygiene → T1 Step 0. §6 reject roster unchanged → nothing owed. §7 stat surface → T8. §8/§8.1-§8.4 → T5, T6. §9 BC edit map → T8, **with §9's heading claim REFUTED**. §10 task surface → T1-T11 (**eleven** tasks, within the SPEC's ~9-11). §10.1 the positive arm → T3, **and §1.3 refutes its break claim**. §11 edit-site roster + the three missed guards → T1-T5, T7, and **RD-STATSSINK adds a fifth**. §12 the narrow → T11 Step 5, **verified in a scratch copy**. §13 deferred → §5 below. §14/§14.1 ADR → T11, **and ADR-0297's own ¶7/¶9 are corrected**. §15 counts → T10. §16 adversarial record → §1.5.

**Placeholder scan.** No `TBD`, no *"add appropriate error handling"*, no *"similar to Task N"*. Every code step carries real, compiled Go with real identifiers. Two deliberate non-placeholders, both flagged: T10 Step 2's `<subtest names, sorted>` (derived at run time from the actual `go test -v` output) and the `<abs-worktree-path>` token used uniformly.

**Type consistency.** `sslNoCertificate *stats.Counter` (T1) is the field T2 asserts, T3 Incs, T4 keys `helpText` on and T5 scrapes. `assertSSLCrossProduct(t *testing.T, reg *stats.Registry, addr string, wantSuffixes ...string)` is used with 1 arg at three sites and 2 args at one. `startOneWayTLSListener(t *testing.T, pki handshakeTestPKI) (*stats.Registry, string)` matches `startMutualTLSListener`'s shape exactly. `AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` matches the interface byte-for-byte. `sslLeafRoster` holds LEAF names; the spelling pins hold FULL names — **the invariant is stated at T3.**

**One spec requirement I deliberately did NOT implement, and why.** SPEC §11's third missed guard asks whether `0111`'s rosters gain a fourth asserted zero. §2.4 **decides against the value assertion** and for the prose fix, reversing the controller's own earlier position on two pieces of evidence that arrived later (F4, A4-N6). The router's instruction was *"decide explicitly"*; the decision, its reversal and its reasons are all recorded.

---

## 5. Deferred — carried forward, none chartered

- **`internal/statssink/registration_test.go:25`/`:51`/`:80`** — *"stays 1200 / 1196"*, stale since **phase 49**, unasserted by code. RECORDED, not fixed (§2.5): fixing it widens the delta into a package this row does not touch.
- **`test/fixtures/0108-xds-sds-validation-context/expectations.yaml:104-106`** — a fourth stale *"envoy-go emits NO `ssl.*` stats whatsoever"* confession, outside this row's fixtures. Prose only, no assertion (SPEC D3).
- **The phantom `B5` in two landed production comments** (`manager.go:392`, `:1265`) plus its propagation into `DECISIONS.md:17304`/`:17314` and `STATE.md`. T8 `:1855` records the TRUE referent; the comments themselves are left, because editing them is outside this row's delta.
- **`ssl.no_certificate` on a RESUMED TLS 1.3 session** — never probed; `session_reused` was 0 in every run. The one scenario in which the pinned predicate could be wrong in production.
- **`ssl.no_certificate` on QUIC, driven** — the registration is pinned and green, but no H3 connection was driven to confirm the counter does not MOVE. The parity argument is structural, not measured.
- **`-race` over `./test/differential/`** — never run (§1.4). T10 owes it or an explicit skip.
- **The stat-surface absolute total** — never mechanically re-derived, and the chain is now known to be discontinuous in **TWO** places (F10).
- **`ssl.connection_error`** — the cheapest identified follow-on; blocker retired at the BRAINSTORM, cost RAISED (a predicate is required to avoid over-counting the EOF class, and it cannot ride `0111`). ~9-11 tasks.
- **Carried from the SPEC §13 roster unchanged:** `fault.abort.grpc_status` (**the recommended NEXT opening**, 7-9 tasks, the only identified candidate clearing a sentinel check-(3) blocker; ONE probe owed) · the `upstream_cluster` span tag · `Listener.stat_prefix` · the Runtime family opening · `stats_flush_on_admin` · `hcm.access_log_options` · HCM `server_name`/`via` · the `stdout`/`stderr` loggers · `hcm.merge_slashes` (carrying the pre-existing H2 `url.Parse` routing bug) · the other TEN fixed `ssl.*` names and the FOUR dynamic families · all tracing remainders · all xDS · all HTTP/3 · gRPC · Runtime · **WASM (a ROADMAP bookkeeping artifact, deliberately left as-is)**.

---

## 6. Operative memories (cited, not decorative)

`reference_break_protocol_commit_first` · `reference_deliberate_break_wrong_assertion` (⚠️ and its panic corollary — Break B) · `reference_liveness_break_needs_failing_baseline` (Breaks G, J) · `reference_plan_break_instructions_dont_compile` (Break F needed **no** substitution) · `reference_differential_break_protocol_count1` · **`reference_differential_run_selector`** (⚠️ **NEW SHAPE: it applies to UNIT tests too — a stale `-run` prints `ok … [no tests to run]` and exits 0**) · `reference_differential_asserter_dispatch` · `reference_fatalf_makes_assertions_unreachable` · `reference_nil_stats_counter_inc_crashes_goroutine` (Breaks B, C) · `reference_probe_must_discriminate` (§2.3, and §1.3 where a *break* failed it) · `reference_a_drift_correction_is_itself_a_claim` (fired against **two of this PLAN's own agents**) · `feedback_brief_citations_not_evidence` · `reference_quoting_is_not_executing` · `reference_plan_schedules_edits_to_a_byte_gated_file` (T10 Step 5) · `feedback_pertask_gofmt_lint` · **`reference_bash_cwd_reset_commits_to_main`** (fired live in this session) · `reference_go_client_cert_withholding` (does NOT apply to the one-way arm — nothing to withhold) · `reference_netpipe_deadlocks_client_cert_handshake` (avoided — loopback TCP) · `reference_iocopy_self_splice_echo_backend` (avoided — `startEchoBackend`) · `reference_fixture_tb_has_no_logf` · `reference_listener_stat_scope_cross_side_divergence` · **`reference_ssl_stats_suppressed_by_fast_failing_upstream`** (⚠️ **re-scope: REFERENCE-ONLY; envoy-go accounts `ssl.*` before the upstream dial** — F2) · `reference_sentinel_deferred_sentence_live_vs_historical` · `reference_roadmap_split_phase_row_done` · `reference_fuzzer_count_docs_drift` · `reference_code_comment_not_evidence` · `feedback_parallel_stream_mints_fresh_drift` · `reference_parallel_subagents_private_scratch` · `feedback_subagents_no_push` · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_push_to_origin`.
