# Phase 94 — `tls-connection-error-stat` — SPEC

**Subject.** Give `outcomeOther` — the fourth downstream-TLS handshake-outcome bucket, which today increments nothing — a **predicate-gated `Inc()`** landing `listener.<normalized-addr>.ssl.connection_error`, the **FIFTH** listener-scope TLS counter. This closes the NAMED DEPARTURE at `BEHAVIOR_CONTRACT.md:1971` and completes the fixed-name taxonomy phase 74 opened (ADR-0296) and phase 75 extended (ADR-0297). The **TWENTY-FIFTH Observability-family row** *(chain ordinal)*; the family **STAYS OPEN**.

**Stage.** Lifecycle-state **1 -> 2**. **ROW 94 STAYS `in-progress`**; `want` STAYS **126**. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` are **BYTE-UNTOUCHED at this SPEC** — see §0.1, which is a finding, not an assumption.

**Method.** Three parallel measurement agents, each on its own worktree and branch off `3b10490a` with a private scratch dir, all Docker-free by instruction, **all four trees proven clean and NOTHING committed by any of them**. Every claim below that says EXECUTED was executed.

---

## 0. What this stage refuted, by execution — TWELVE claims

The project discipline is that **every stage's job is to refute its predecessor by execution** (92 IMPL twelve · 93 BRAINSTORM eleven · 93 SPEC eleven · 93 PLAN twelve · 93 IMPL nine · 94 BRAINSTORM eleven). This stage refutes **TWELVE**, and **three of them refute this stage's own agents rather than the BRAINSTORM.**

1. ⚠️ **THE BRAINSTORM'S §7.2 VERDICT TABLE IS A CATEGORY ERROR, AND ITS OWN PREDICATE CONTRADICTS IT ON ONE ROW.** Run against the exact values the five landed rows construct, the proposed `errors.Is`-only predicate **`Inc()`s on `{"unrelated error", errors.New("connection reset by peer"), outcomeOther}`**, which §7.2 says must EXCLUDE. Isolation, executed: `errors.Is(errors.New("connection reset by peer"), syscall.ECONNRESET)` = **false**, while `syscall.ECONNRESET.Error()` = `"connection reset by peer"` — **string equality true**. The row's TEXT is byte-identical to the sentinel's; its VALUE is a bare `*errors.errorString` with no `Is`/`Unwrap`. See §3.3 — the resolution is that §7.2 projected COUNTER verdicts onto a CLASSIFIER table, and derived the offending verdict **by matching message text, the exact thing the design forbids**.
2. ⚠️ **"THE TRANSPORT CLASS IS CLOSED AND FULLY TYPED, EVERY MEMBER `errors.Is`-ABLE" IS TRUE OF REAL ERRORS AND FALSE OF THE TEST TABLE.** On **REAL** server-side `HandshakeContext` errors the predicate is **6/6 correct** (§2.2). On the synthetic table it is **4/5**. The single miss is entirely the hand-written string at `manager_test.go:4398`.
3. ⚠️ **"UNIQUE REPO-WIDE" IS FALSE FOR BOTH EDIT ANCHORS — THEY ARE UNIQUE ONLY UNDER `-- '*.go'`.** Unscoped: `return outcomeOther` reads **4** (`74/PLAN.md:368`, `:377`, `94/BRAINSTORM.md:137`, `manager.go:455`); `case outcomeVerifyError:` reads **3** (`74/PLAN.md:686`, `94/BRAINSTORM.md:139`, `manager.go:1295`). **An unscoped `sed -i` on either would corrupt two landed phase documents.** `reference_symbol_assertion_needs_qualified_name` in its pathspec form.
4. ⚠️ **THE `manager.go` PROSE ROSTER IS SIX, NOT FOUR — AND ONE LOOK-ALIKE MUST BE LEFT ALONE.** The BRAINSTORM named four; agent A found a fifth (`:408-412`); agent B, sweeping independently, found a sixth (`:363-364`, *"four in total"*). Controller-verified. ⚠️ **AND `:414` — *"crypto/tls exports four error TYPES and ZERO error VALUES"* — STAYS TRUE**: that `four` counts `crypto/tls` error types, not `ssl.*` counters. A naive `four` sweep edits it and is wrong. §6.3.
5. ⚠️ **THE INHERITED TEST-PIN ROSTER MISSES `helpTextRoster` ENTIRELY, AND WITHOUT IT THE PACKAGE IS RED.** `internal/stats/helptext_test.go:41-83` is a **matched pair** with `helpText`; either alone reddens `internal/stats`. `reference_measured_prototype_is_a_lower_bound` **FIRES A NINTH CONSECUTIVE TIME, again by under-enumerating FILES** — caught here, as at the BRAINSTORM.
6. ⚠️ **`name_test.go:232-237`'s TRAP COMMENT IS HALF STALE: ITS GATE MOVED, AND ITS OWN CITED EVIDENCE NO LONGER HOLDS.** It records that at the phase-75 PLAN a fifth `helpText` entry with the slice left short kept *"the whole package GREEN"*. **That package-level claim is FALSE at this tip** — `helptext_test.go` (phases 79/80, landed AFTER phase 75) reddens on a bare `helpText` addition with `extra: [envoy_listener_ssl_connection_error]`. The silently-green hazard is REAL but now keyed on `helpTextRoster`. §6.2.
7. ⚠️ **`internal/stats/name.go` IS NOT A ONE-LINE ADD — `gofmt` REWRITES THE WHOLE MAP BLOCK.** The new key is longer than the current alignment column, so a bare insertion left `gofmt -l internal/stats/` printing `internal/stats/name.go`. Budget a realignment diff across the group.
8. ⚠️ **THE PORT PICK WAS NOT SETTLED, AND THE RUNNER-UP'S PREMISE IS FALSIFIED BY MEASUREMENT.** `0118/driver/driver.go:31` reserves `10450` as *"the TLS/SDS band (0108-0113)"*. Measured at this tip: `0112-stats-sink-otlp` and `0113-stats-sink-otlp-knobs` carry **ZERO** `DownstreamTlsContext`. **That range is a sequential run whose last two members are not TLS fixtures at all**, so the stated reason for the reservation does not hold. §7.2.
9. ⚠️ **THERE IS NO `tls_params` YAML ANYWHERE IN THIS REPO.** All ten hits are under `docs/`. Fixture `0120` would be the **FIRST** config in the tree to ship `tls_params`, so "the reference container accepts this block" is **ASSERTED, NOT MEASURED**, and is a boot risk the PLAN must clear first. §7.3.
10. ⚠️ **THE BAD-VERSION ARM CANNOT BE BUILT THE OBVIOUS WAY: `TLSv1_0`/`TLSv1_1` IN THE YAML BOOT-REJECTS envoy-go.** `internal/tls/params.go:62-83` returns `"%s is not supported in phase 03"` for both. The arm must be produced **CLIENT-side**, never by lowering the server floor. §7.4.
11. ⚠️ **THE "VACUOUS `0 == 0`" ARGUMENT IS TRUE ONLY POST-IMPL, AND AS INHERITED IT IS IMPRECISE.** *Today* a `connection_error: 0` pin on `0110`/`0111` hits the ABSENT branch and goes **RED**, because the counter is not registered. The vacuity bites only after this row registers it. The precise statement is that a zero pin gates **REGISTRATION ONLY and can never gate the INCREMENT** (`reference_counter_cannot_gate_a_value`). **The conclusion — a new fixture is FORCED — is UPHELD; the reasoning is CORRECTED.** §7.
12. ⚠️ **THE INHERITED `clusters: []` BOOT-REJECT DID NOT REPRODUCE AS STATED.** No literal reject was located; the nearest hits are a dangling-cluster-reference error (`bootstrap.go:970`) and `hcm/config.go:880`'s `weighted_clusters has no clusters`, a different check. Moot for this row — `BackendCount() == 1` forces a cluster — but recorded rather than repeated.

**And this stage's own agents were refuted twice more:** agent C recommended port `10450` on a premise refutation 8 destroys, and agents A and B disagreed on the prose-roster size — resolved by the controller reading the disputed site rather than picking a winner (`reference_contradicting_agents_find_the_variable`: **both were right about what they were asked**; the variable is that A verified a handed list while B swept independently).

### 0.1 ⚠️ A SCOPE FINDING THAT CONSTRAINS ITEMS 3 AND 4 OF `BRAINSTORM.md` §10

`BRAINSTORM.md` §10 item 3 reads as an instruction to **delete** a clause from `BEHAVIOR_CONTRACT.md:1971` at this stage. **Measured against precedent, the SPEC stage cannot land that edit.** The files touched by the two nearest SPEC commits:

| SPEC commit | files |
|---|---|
| `975e527e` (phase 93) | `DECISIONS.md`, `STATE.md`, `STATE_HISTORY.md`, `phases/93/SPEC.md`, `next-prompt.txt` |
| `2b5eff0a` (phase 75) | `DECISIONS.md`, `STATE.md`, `phases/75/SPEC.md`, `next-prompt.txt` |

**`ROADMAP.md` and `BEHAVIOR_CONTRACT.md` are BYTE-UNTOUCHED at the SPEC in both**, and ADR-0315's own STATUS line asserts that discipline for the SPEC and the PLAN alike. Contract edits land at the **IMPL**, with the ADR as their **MANDATED vehicle** per ADR-0052 `:1821`.

⇒ **Item 3 is discharged here as a PINNED EDIT MAP (§11), executed at the IMPL.** ⇒ **Item 4 — "propagate the reference rule into a GOVERNING document" — is discharged IN FULL at THIS stage, because `ADR-0316 §Context` IS a governing document and it lands in this commit.** That is exactly the mirror-failure repair `BRAINSTORM.md` §9.1 demands: phase 77's measurement was lost for seventeen rows precisely because it stayed in a phase directory.

---

## 1. Scope, restated as a decision

**IN:** one struct field; one `NewCounter` on the existing `rt.tlsMode` gate; one `case outcomeOther:` arm carrying a closed transport-exclusion predicate; one `helpText` entry and its `helpTextRoster` twin; the test-pin edit roster of §6; differential fixture `0120`; `ADR-0316 §Context`; the pinned contract edit map of §11.

**OUT, named not omitted:** the other TEN fixed `ssl.*` names · the FOUR dynamic `ssl` families (blocked on NAMING — the stats name pattern bans the hyphen, so an OpenSSL cipher name panics at registration; `sigalgs` may be unimplementable, `crypto/tls.ConnectionState` exposing no signature-scheme field) · `Listener.stat_prefix` · a `len(helpText)` guard · the `0108` pre-existing drift of §6.5 · the phantom `B5` in `DECISIONS.md:17316` (append-only per ADR-0288 §Decision 4).

**Non-goal, stated so the IMPL does not drift into it:** this row **adds no `handshakeOutcome` variant**. The outcome taxonomy stays **FOUR** while the counters go to **FIVE**. See §3.1.

---

## 2. The reference rule — MEASURED, and PROPAGATED HERE

### 2.1 The rule

> **The reference increments `ssl.connection_error` IFF BoringSSL reports an actual SSL protocol error** — an `ssl_socket.cc` `TLS_error` line. **A transport-level EOF or reset during the handshake takes `ConnectionImpl`'s generic `remote close` path and books NOTHING under `ssl.*`.**

Established at the phase-94 BRAINSTORM by twelve arms on `envoyproxy/envoy:contrib-v1.37.2`, digest `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8` (verified against `docs/envoy-go/ENVOY_TARGET.md` before any arm was trusted), two listeners in one process scraped together, with a positive control, a discriminating negative control, and a **full two-listener set reconciliation** in which both listeners reconcile exactly and no stray increments appear.

⚠️ **This rule, and the `SSL_ERROR_SSL` token that names it, appear ZERO times in `DECISIONS.md` and ZERO times in `BEHAVIOR_CONTRACT.md` at this tip** — re-derived by the controller. Phase 77 measured it on 2026-07-26 and the result never left its BRAINSTORM, so a later row rejected the candidate for lacking evidence that already existed. **`ADR-0316 §Context ¶2` lands it in a governing document for the first time. That is this row's most durable deliverable and it is not optional.**

### 2.2 The subject side, measured independently at THIS stage — the predicate is 6/6 on REAL errors

A live in-process `crypto/tls` server (`MinVersion` TLS 1.2) over a **loopback TCP pair** — never `net.Pipe`, which deadlocks a client-cert handshake — on `127.0.0.1:12777`, band-checked with `ss -tan` (ALL states), below the 32768-60999 ephemeral range and outside the harness reservation `20000..31007`:

| # | arm | `%T` | `%v` | `isTransport` | verdict | reference |
|---|---|---|---|---|---|---|
| 1 | TLS 1.0 client vs TLS 1.2 floor | `*errors.errorString` | `tls: client offered only unsupported versions: [301]` | false | **Inc** | Inc ✅ |
| 2 | plaintext HTTP to the TLS port | `tls.RecordHeaderError` | `tls: first record does not look like a TLS handshake` | false | **Inc** | Inc ✅ |
| 3 | garbage bytes | `tls.RecordHeaderError` | same | false | **Inc** | Inc ✅ |
| 4 | partial ClientHello then FIN | `*errors.errorString` | `unexpected EOF` | **true** | no Inc | no Inc ✅ |
| 5 | zero bytes then FIN | `*errors.errorString` | `EOF` | **true** | no Inc | no Inc ✅ |
| 6 | partial ClientHello then **RST** (`SO_LINGER 0`) | `*net.OpError` | `read: connection reset by peer` | **true** | no Inc | no Inc ✅ |

**Zero wrong answers on real errors.** Three specifics the PLAN must carry: arms 4/5 return the **bare sentinels** (`io.ErrUnexpectedEOF` / `io.EOF`), so `errors.Is` matches by identity with no wrapper to walk; arm 6 proves `syscall.ECONNRESET` is genuinely reachable and `errors.Is`-able through `*net.OpError -> *os.SyscallError -> syscall.Errno`, so it is **not dead weight**; arms 2/3 return the **bare `tls.RecordHeaderError` VALUE** server-side, with no `permanentError` wrapper (that appears client-side only).

⚠️ **`net.ErrClosed` was NOT observed on any natural arm.** It stays in the exclusion set as a **DEFENSIVE, UNEXERCISED member and must be LABELLED as such in the code comment** — an unexercised predicate term is exactly the shape `reference_passing_test_is_not_a_guard` warns about, and calling it "measured" would be false.

---

## 3. The D-TLSCE docket

### 3.1 D-TLSCE-SEAM **[RESOLVED — the Inc site, NOT the classifier]**

The seam verbatim at this tip, `internal/listener/manager.go:1294-1299` — note there is **no `default:` and no `case outcomeOther:`**, and `outcomeOK` is unreachable here because `err != nil`:

```go
switch classifyHandshakeErr(err) {
case outcomeVerifyError:
	rt.sslFailVerifyError.Inc()
case outcomeNoCert:
	rt.sslFailVerifyNoCert.Inc()
}
```

**CHOSEN:** add `case outcomeOther:` to this switch, guarded by a package-level `isTransportHandshakeErr(err error) bool`.

**REJECTED — a FIFTH `handshakeOutcome` variant with the predicate inside `classifyHandshakeErr`.** Superficially tidier: it would restore a 1:1 outcome-to-counter mapping. **Its measured cost is that it flips the expected outcome of THREE of the five landed rows at `manager_test.go:4398-4402`, reddening a table that is currently a CORRECT pin of a DIFFERENT question** (the error-path taxonomy). It also converts a pure taxonomy into a counter-routing function. **Phase 75 set the precedent of landing a name WITHOUT adding a variant, and stated it in the landed doc comment; this row follows it.**

⚠️ **CONSEQUENCE THE CONTRACT MUST RECORD:** the taxonomy stays FOUR while the counters go to FIVE, and **`outcomeOther` becomes the ONLY outcome that maps to a counter CONDITIONALLY** — to `{ssl.connection_error, nothing}`. The 1:1 reading is now wrong in both directions; it was already wrong in one, via phase 75's variant-less fourth name.

### 3.2 D-TLSCE-PREDICATE **[RESOLVED BY EXECUTION — `errors.Is`-only, exclusion, no message text]**

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
func isTransportHandshakeErr(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) || // DEFENSIVE: not produced by any measured arm
		errors.Is(err, context.DeadlineExceeded)
}
```

⚠️ **THE `tls.RecordHeaderError` BY-VALUE FOOTGUN, CONFIRMED BY EXECUTION AND GUARDED HERE.** It is returned **BY VALUE**: `crypto/tls/conn.go:565` declares the type, `:578` gives it a **value receiver** `Error()`, and `:580` `newRecordHeaderError` returns `(err RecordHeaderError)` by value, boxed at the four construction sites `:647 :661 :669 :675`. Measured in all six combinations (direct / `%w`-wrapped / double-wrapped, hand-constructed and live): `errors.As(err, &value)` = **true**, `errors.As(err, &pointer)` = **false**, every time. `var p *tls.RecordHeaderError; errors.As(err, &p)` **compiles** — the pointer type carries the promoted method — **and is permanently false.** **If any future arm matches this type it MUST use the value form.** This predicate uses `errors.Is` only and therefore does not touch it; the footgun is recorded so the PLAN does not reintroduce it.

⚠️ **`context.DeadlineExceeded` is excluded for a reason that is behaviour-matching, not defensive.** The reference books handshake timeouts under `listener.<addr>.downstream_cx_transport_socket_connect_timeout` — present in the phase-94 scrape and stayed **0** — never under `ssl.connection_error`. It is additionally structurally unreachable in envoy-go today: the ctx that reaches `HandshakeContext` is `cmd/envoy-go/main.go:339`'s cancel-only `signal.NotifyContext`, and the one production `context.WithTimeout` under `internal/listener/` (`listenerfilter/pipeline.go:43`) cannot escape, because `Pipeline.Run` returns only an `error` and `serveConnection` never rebinds `ctx`. **Any future row adding a handshake deadline re-opens this**, and would additionally mis-bucket every outcome through the named-return override at `crypto/tls/conn.go:1541-1546`.

### 3.3 D-TLSCE-VERDICTS **[RESOLVED — `BRAINSTORM.md` §7.2 is a CATEGORY ERROR, and one verdict is REVISED]**

`TestClassifyHandshakeErr` calls `classifyHandshakeErr(tc.err)` **directly** and asserts the **outcome only** — read at this tip, `manager_test.go:4404-4408`. **It never reaches an `Inc` site.** Therefore:

- **The five `outcomeOther` rows at `:4398-4402` stay GREEN and BYTE-UNCHANGED by this row.** (The `:4394-4402` cite inherited from the phase-93 BRAINSTORM is wrong; `:4394` is the `{"nil", nil, outcomeOK}` row and the `cases` literal opens at `:4385`. The BRAINSTORM's correction to `:4398-4402` is CONFIRMED.)
- **`BRAINSTORM.md` §7.2's `must EXCLUDE` / `must INCREMENT` column describes a COUNTER table that does not yet exist.** It was projected onto a classifier table.
- ⚠️ **The offending verdict was derived BY MATCHING THE ROW'S MESSAGE TEXT to reference arm (c3/e). That is message-text matching — the exact practice §3.2 forbids.** The BRAINSTORM's own verdict table commits the error its design principle prohibits.

**REVISED verdict for `{"unrelated error", errors.New("connection reset by peer"), outcomeOther}`: NO REFERENCE COUNTERPART** — the same disposition `BRAINSTORM.md` §2.6 already gave `context.DeadlineExceeded`. A **real** reset arrives as `*net.OpError` and IS excluded (§2.2 arm 6); the synthetic value has no producer in the measured set. **Recorded as an open residual, NOT claimed impossible.**

⚠️ **CONSEQUENCE — THE NEW COUNTER TABLE MUST USE PRODUCTION-REPRESENTATIVE VALUES**, never the classifier table's hand-written strings. Build its exclusion arms from arm 6's `*net.OpError` shape and its inclusion arms from **live handshakes**, following the precedent already in the same file: `TestClassifyHandshakeErr` builds its no-cert and untrusted arms with `liveHandshakeErr` and carries a NON-VACUITY block proving the two live errors are distinct.

⚠️ **THE NC THAT PROVES THE TABLE READS IDENTITY AND NOT TEXT:** substitute the synthetic `errors.New("connection reset by peer")` for the real `*net.OpError` value in the exclusion arm. **The arm MUST flip.** If it does not, the table is reading text and the guard is fictional.

### 3.4 D-TLSCE-NILGATE **[RESOLVED BY EXECUTION — and the failure mode is a PROCESS CRASH]**

The fifth counter registers on the existing gate, `manager.go:384-389`, landing at a new `:389`. On a plaintext listener the pointer stays **NIL**; the `Inc()` site is inside `if selected.tlsCfg != nil` (`:1286`) and is therefore unreachable when nil — but that is an argument, not a guard.

**EXECUTED:** delete the registration while retaining the `Inc` and the package dies with `panic: runtime error: invalid memory address or nil pointer dereference` / `SIGSEGV`, **while `--- PASS: TestListenerMetrics_GateMatchesInc` PASSES as the process crashes**. ⇒ **The fifth field MUST be added to both arms of `TestListenerMetrics_GateMatchesInc` (`:2281-2298` TLS, `:2340-2354` plaintext) in the SAME commit as the `Inc`**, per `reference_nil_stats_counter_inc_crashes_goroutine`.

### 3.5 D-TLSCE-HELPTEXT **[RESOLVED — a CHOICE, and it is declared as one]**

⚠️ **The implicit assumption that `helpText` is stored sorted is REFUTED.** Source order is `handshake` `:556`, `fail_verify_error` `:557`, `fail_verify_no_cert` `:558`, blank line, `no_certificate` `:560`. **The file gives no precedent either way for the `ssl` group.**

**CHOSEN:** insert **into the contiguous `ssl.*` group, FIRST within it**, matching the `LC_ALL=C` order the roster and test slices use. **NOT appended at EOF** — an EOF append breaks the tail-anchored doc clause *"the last five are phase 80's `sds.` root"*. Declared as a convention for this row, not inherited as a rule.

⚠️ **The new name PREPENDS, confirmed by execution** under both `LC_ALL=C sort` and Go's `sort.Strings`, in both the dotted and the Prometheus projections:

```
ssl.connection_error      <-- FIRST of five
ssl.fail_verify_error
ssl.fail_verify_no_cert
ssl.handshake
ssl.no_certificate
```

**Phase 75's append precedent does NOT transfer.** Every exact-set `want` slice in §6.2 takes the new name in **position 0**, not appended.

---

## 4. The five divergent positions, disposed one by one

`BRAINSTORM.md` §3.4 recorded five landed positions to reconcile. Their disposition at this tip:

| # | site | position | disposition |
|---|---|---|---|
| 1 | `BEHAVIOR_CONTRACT.md:1971` | *"still **blocked** on enumerating its membership"* | **DELETE THE CLAUSE — pinned §11, landed at the IMPL** |
| 1b | `BEHAVIOR_CONTRACT.md:1971` | *"The full membership … is UNENUMERATED"* | ✅ **KEEP.** Different proposition; still true. The correction is ONE CLAUSE, not the paragraph |
| 2 | `DECISIONS.md:17390` (ADR-0296) | blocker RETIRED; `~9-11` tasks; ONE `io.EOF` predicate | **NOT EDITED** — `DECISIONS.md` is append-only (ADR-0288 §Decision 4). **ADR-0316 §Context names the figures SUPERSEDED** |
| 3 | `phases/77-…/BRAINSTORM.md:216` | `~12-15`; THREE predicates; deny-list **OPEN** | **NOT EDITED** — historical phase artifact. Measurement correct, conclusion refuted (§3.2). ADR-0316 records it |
| 4 | `ROADMAP.md:155` (row 93's cell) | *"~4-6 production lines … its reference side is an unprobed doc claim"* | ⚠️ **BOTH HALVES FALSE. PINNED §11, landed at the IMPL** |
| 5 | `phases/93-…/BRAINSTORM.md:235-244` | position 4, marked MEASURED | **NOT EDITED** — `reference_verification_table_launders_wrong_cites`; ADR-0316 records it |

⚠️ **POSITION 4 IS NOW ONE SITE, NOT TWO.** Its `STATE.md` half was already corrected by the phase-94 BRAINSTORM; `STATE.md:15` carries the refutation at this tip. Re-derived, not inherited.

⚠️ **Positions 1 and 2 were landed by ONE AUTHOR IN ONE COMMIT** (`c57b98b8`, the phase-75 IMPL): the contract says "still blocked" while the ADR says "blocker RETIRED". **`git blame` before assuming a chronology** — the divergence is not stale drift between distant stages.

---

## 5. Identifier hygiene *(re-derived repo-wide at this tip)*

`git grep` over `*.go` for every identifier this row introduces: `sslConnectionError` **0** · `SSLConnectionError` **0** · `connectionError` **0** · `envoy_listener_ssl_connection_error` **0** · `isTransportHandshakeErr` **0** (and the shorter `isTransportError`, `transportErr`, `classifyTransport` all **0**). **All free.** The field name `sslConnectionError` matches the family convention set at `:180-187`.

`ssl.connection_error` occurs in exactly **one** `.go` file today — `internal/listener/manager.go:411` and `:1292`, **both prose comments**, both inside the §6.3 roster.

---

## 6. The edit roster, as an EDIT roster — classifications PROVEN BY EXECUTION

### 6.1 Production — 2 files

| file | change |
|---|---|
| `internal/listener/manager.go` | `sslConnectionError *stats.Counter` field beside `:187`; `NewCounter(prefix + "ssl.connection_error")` at a new `:389`; `case outcomeOther:` + the predicate helper; **SIX prose blocks** (§6.3) |
| `internal/stats/name.go` | `helpText` entry in the `ssl` group + the doc-comment enumeration at `:517`, `:521-523`, `:529` (+ a `gofmt` realignment diff, §0 refutation 7) |

### 6.2 Tests — the RED SET, MEASURED

**Definitive red set from a production-only prototype** (field + `NewCounter` + `case outcomeOther: Inc()` + the `helpText` entry, **no test edits**), repo-wide `go test` excluding `test/differential` and `test/conformance/h2spec`: **`RC=1`, anchored `FAIL=11`**:

- `--- FAIL: TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames` (`internal/listener`)
- `--- FAIL: TestQUICListener_RegistersSSLNamesAtZero` (`internal/listener`)
- `--- FAIL: TestHelpText_KeySetExact` (`internal/stats`)

⚠️ **A FOURTH FAILURE IN THAT RUN IS AN UNRELATED FLAKE AND IS NOT PART OF THE RED SET — SEE §14.**

⚠️ **Registration alone — with no `Inc` — already reddens both spelling pins**, so the two `want` slices must be edited whether or not the increment lands.

| # | file | site | change | class | evidence |
|---|---|---|---|---|---|
| 1 | `manager_test.go` | `want` `:2136-2141`, `DeepEqual` `:2144` | add the name **FIRST** (prepends) | **REDDENS** | live |
| 2 | `quic_test.go` | `want` `:280-285`, `DeepEqual` `:287` | same, prepend | **REDDENS** | live |
| 3 | `stats/helptext_test.go` | **`helpTextRoster` `:41-83`, after `:47`** | add the roster entry | **REDDENS** if omitted (`extra:`) | `helptext_test.go:141` fired |
| 4 | `stats/name_test.go` | `wantNames` `:239-244` | add the Prometheus name | ⚠️ **SILENTLY GREEN if omitted** | roster+entry, slice at 4 ⇒ `RC=0 RUN=196 FAIL=0` |
| 5 | `stats/name_test.go` | comment `:232-237` | rewrite — its cited evidence is stale (§0 refutation 6) | prose-in-test | §0.6 |
| 6 | `manager_test.go` | `sslLeafRoster` `:4580`; calls `:4663 :4694 :4712 :4805` | add `"connection_error"` | ⚠️ **SILENTLY GREEN if omitted — and LOAD-BEARING once added** | 2×2 below |
| 7 | `manager_test.go` | `:2281-2298`, `:2340-2354` | fifth nil / non-nil assertion | ⚠️ **SILENTLY GREEN; real failure is SIGSEGV** | §3.4 |
| 8 | `manager_test.go` | `:2021 :2102 :2111 :4576` | `…ExactlyFourSSLNames` -> `…Five…` | **REDDENS by rename** | §6.4 |

⚠️ **`sslLeafRoster` IS WHAT MAKES THE PREDICATE GUARDED AT ALL. The full 2×2 was EXECUTED:**

| roster | predicate | result |
|---|---|---|
| 4 leaves | correct | only the 2 spelling pins red |
| 4 leaves | **removed** (unconditional `Inc`) | ⚠️ **still only the 2 spelling pins red — THE DEFECT IS INVISIBLE** |
| 5 leaves | correct | only the 2 spelling pins red |
| 5 leaves | **removed** | ⚠️ **+2 RED**: `TestServeConnection_SSLFailVerifyErrorIncrements`, `TestServeConnection_SSLFailVerifyNoCertIncrements`, with `manager_test.go:4694: …ssl.connection_error = 1, want 0 — only [fail_verify_error] may move on this arm` |

**This is the executed form of "assert WHICH error fired, and also which did NOT."** Without the roster extension the row ships a predicate that no test can falsify.

⚠️ **ORDERING CONSTRAINT:** `helpText` (`name.go`) and `helpTextRoster` (`helptext_test.go`) are a **MATCHED PAIR** — either alone reddens `internal/stats`. They land in the same commit, together with `name_test.go`'s slice, which is silent but is a live guard once present (NC: slice at five with the `helpText` entry removed ⇒ **3 RED**).

### 6.3 Prose in `manager.go` — SIX sites, and ONE guarded NON-site

| site | what goes false |
|---|---|
| `:175-177` | *"the four `ssl.*` counters … all four pointers stay NIL"* |
| `:363-364` | *"the one phase-75 `ssl.*` counter (`ssl.no_certificate` — **four in total**)"* ⚠️ **found by agent B; the BRAINSTORM and agent A both missed it** |
| `:392-393` | *"the three counted buckets plus a fourth that **counts NOTHING**"* |
| `:398-400` | *"the listener scope carries **FOUR** `ssl.*` counters as of phase 75"* |
| `:408-412` | *"land in `outcomeOther`, **which increments nothing**. The reference books those under `ssl.connection_error`; that asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5)"* |
| `:1291-1293` | *"`outcomeOther` deliberately increments NOTHING — … **a name this row does not land**. That asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5)"* |

⚠️ **DO NOT EDIT `:414`** — *"`crypto/tls` exports **four** error TYPES and ZERO error VALUES"*. That `four` counts `crypto/tls` error types, **not** `ssl.*` counters, and it stays TRUE. **A naive `four` sweep edits it and is wrong.**

⚠️ **THE PHANTOM `B5` IS FIXED HERE BECAUSE THIS ROW REWRITES ITS TWO CARRIERS ANYWAY.** `:412` and `:1293` both cite *"BEHAVIOR_CONTRACT B5"*, and **there is no B-numbered step scheme in that file at all** — its `B5` hits are `:1971` (the narration itself) and four unrelated `AMEND-B5` contexts. Replace with the real anchor: the subsection heading **`### Downstream TLS handshake-outcome stats`** (`BEHAVIOR_CONTRACT.md:1965`). The propagated copy at `DECISIONS.md:17316` is **NOT** edited (append-only); ADR-0316 records the correction.

Also prose: `manager_test.go` `:2016-2021 :2068-2072 :2102-2110 :2349-2351 :4569-4579 :4596-4604 :4809`; `quic_test.go` `:225-226 :234 :275-281 :303`.

### 6.4 The rename — 4 sites, and the hazard REPRODUCED LIVE

`TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames` -> `…ExactlyFiveSSLNames` at `manager_test.go:2021 :2102 :2111 :4576`. **The roster is COMPLETE**: `git grep -n 'RegistersExactlyFourSSLNames' -- . ':(exclude)docs/'` returns exactly those four plus `next-prompt.txt:116`, with **ZERO** hits in Makefiles, scripts, or `.github/workflows/ci.yml`. **Historical occurrences in `75/PLAN.md` (10), `75/PROGRESS.md` (12), `DECISIONS.md` (1) and `94/BRAINSTORM.md` (1) MUST NOT be renamed.**

⚠️ **The stale-selector hazard, reproduced live at this tip:**

```
$ go test ./internal/listener/ -count=1 -run 'TestListenerMetrics_TLSListenerRegistersExactlyFiveSSLNames'
ok  github.com/pgdad/envoy-go/internal/listener  0.003s [no tests to run]
exit=0
```

`reference_differential_run_selector`: **a selector matching nothing prints `ok` and EXITS 0 having run nothing.** Every gate command in the PLAN must assert a non-zero `=== RUN` denominator.

### 6.5 Fixture prose the inherited roster missed — FOUR files

`0110/README.md:171-173` · `0110/expectations.yaml:224-226` · `0111/README.md:172-174` · `0111/expectations.yaml:229-231` each state in landed prose that `ssl.connection_error` *"increments nothing — a named departure"*. **All four go FALSE and are IN SCOPE.**

`0110/driver/driver.go:794-799` and `0111/driver/driver.go:774-778` need **NO change** — they iterate closed named subsets, so a fifth metric name cannot redden them.

⚠️ **PRE-EXISTING DRIFT, RECORDED AND DELIBERATELY NOT FIXED:** `0108/README.md:136-140` and `0108/expectations.yaml:104-106` assert *"envoy-go emits NO `ssl.*` stats whatsoever"* — **already false since phase 74**. `0110`/`0111` retired that wording; `0108` was never updated. Outside this row's delta.

### 6.6 Byte-gate / golden set-difference — **EMPTY**

Per `reference_plan_schedules_edits_to_a_byte_gated_file`: the only golden in the tree is `internal/filter/hcm/testdata/direct_response_h1.golden` (unrelated); `internal/statssink/golden_bytemirror_test.go`'s sole listener row is `listener_manager.listener_create_success`; no `wc -l` / line-count / checksum assertion covers any roster file; `ci.yml` carries no `-run` selector and no `ssl` reference; the `FuzzPromTextFormat` corpus entry is name-agnostic. **Set difference = ∅.**

---

## 7. Differential fixture `0120` — and the row inherits NO red gate

⚠️ **STATED PLAINLY, NOT MINIMISED: this row has no existing failing gate.** The five `outcomeOther` rows pin the classifier and pass today (§3.3), and **adding the `Inc()` on top of the registration changed the red set by ZERO tests** — measured. **`outcomeOther` is produced by ZERO fixture arms anywhere in the tree.** ⇒ **The row owes a new positive arm, or the `Inc` ships unexecuted.** The fixture is that arm, and the SPEC must prove it non-vacuous rather than inherit a red one.

**A new fixture is FORCED, with the reasoning CORRECTED (§0 refutation 11).** `0110`/`0111` cannot carry it: both read `/stats/prometheus` through a **closed named subset**, so a fifth name never reddens them; and adding a drive arm breaks their own determinism argument, quoted at `0111/driver/driver.go:710-717` — *"The three arms of `driveSide` are therefore the ONLY connections `l_edf` ever sees ⇒ deterministically 3 accepts…"* — whose `want` values are **ABSOLUTE, not deltas**, so a fourth arm invalidates `downstream_cx_total`, `ssl.handshake` and the arm arithmetic in **both** fixtures at once.

### 7.1 The three registration gates — enumeration VERIFIED

1. **The directory** — `discoverFixtures`, `test/differential/runner_test.go:1462`.
2. **`fixture.RegisterFixture(<name>, drv)` from the driver's `init()`**, name **string-equal to the directory name** (`fixture/fixture.go:92`).
3. **A blank import in `runner_test.go`** — ⚠️ **the ONLY file outside the fixture's own directory that must be touched** (`:144-146` for `0117`/`0118`/`0119`).

**Reconciliation EXECUTED:** blank imports **121** = fixture dirs **121**, set difference **empty in both directions**. The silent-green failure mode is live at `runner_test.go:200` (`t.Skipf("no driver registered for fixture %q …")`), and **no fixture-count gate exists anywhere in the tree** — so the PLAN must **assert the fixture set BY NAME, in BOTH directions**.

⚠️ **A ROSTER-GATE TRAP FOUND IN FLIGHT:** a naive `test/fixtures/[^/]+/driver` extraction reports **24 phantom "unregistered" dirs**, because **there are TWO directory layouts — 97 fixtures use `<dir>/driver/` and 24 use `<dir>/inputs/`**. Any roster gate the PLAN authors must match both. `0110`-`0119` all use `driver/`; **`0120` uses `driver/`.**

### 7.2 Port — **`10126`**, and the runner-up's premise is FALSIFIED

The `10<index>` convention is dominant and twice ratified (phase-77 SPEC R13; phase-84 SPEC `:138`, both citing the landed comment at `0118/driver/driver.go:29-33`). For `0120` it yields `10120`, which **`0028` holds** as part of its run `10120-10125` (`0028/inputs/driver.go:65-70`).

**CHOSEN: `10126`** — the minimal index-preserving repair, the first free port above the occupying run. **VERIFIED FREE: `10126`, `10127`, `10128`, `10129` each read ZERO hits in CODE SCOPE (`test/`, `*.go`, `*.yaml`).** Every repo-wide hit is docs prose; `0002/PROGRESS.md:362`'s `101276/sec` is a false positive for `10127`.

⚠️ **REJECTED: `10450`, because the reason for reserving it is measurably false.** `0118/driver/driver.go:31` reads *"⚠️ NOT 10450: that is the TLS/SDS band (0108-0113), and this is not a TLS fixture."* Measured at this tip: `0108` **4** files with `DownstreamTlsContext`, `0109` **4**, `0110` **4**, `0111` **4**, **`0112` ZERO**, **`0113` ZERO**. `0112-stats-sink-otlp` and `0113-stats-sink-otlp-knobs` are not TLS fixtures. **The range is a sequential run, not a TLS band**, so citing that carve-out for `0120` would propagate a measured-false premise. Recorded, deliberately **not** fixed — a landed comment in another fixture, outside this row's delta.

⚠️ **This question was carried as CONTESTED WITH NO NUMBER at phases 81, 82 and 83. This SPEC settles it FOR THIS ROW ONLY, mechanically, and says so** rather than minting a general rule (`reference_generalizing_a_measured_table_into_a_rule`).

### 7.3 Config shape

**Use the `0018-http-rbac/envoy.yaml:216-226` INLINE `validation_context` + `trusted_ca: {filename: …}` form**, with `require_client_certificate: true` and a `tcp_proxy` chain. **Deliberately NOT `combined_validation_context`**: `0110`/`0111` drag in an SDS `sds_cluster` and two per-side SDS receivers, which is exactly where their known value-instability lives (`0111:718-733`), and `0120` needs none of it.

⚠️ **`tls_params.tls_minimum_protocol_version: TLSv1_2` HAS NO YAML PRECEDENT IN THIS TREE** (§0 refutation 9). The feature is implemented and wired — `applyTLSParams` at `internal/tls/params.go:19`, called from `internal/tls/config.go:575` for both directions, with `mapTLSVersion` at `:62-83` mapping `TLSv1_2 -> stdtls.VersionTLS12`. **But no fixture has ever shipped the block, so "the reference container accepts it" is ASSERTED, NOT MEASURED. THE PLAN MUST BOOT BOTH SIDES ON THIS CONFIG BEFORE WRITING ANY ARM.**

⚠️ **Keep PEM substitutions out of YAML comments** — `0108/envoy.yaml:40-45` records that a multi-line PEM expanded inside a `#` comment splatters continuation lines outside the comment and yields invalid YAML.

**`BackendCount()` returns 1** — the runner `t.Fatalf`s on 0 (`runner_test.go:242-245`), and `0118/driver/driver.go:64-68` is the stated precedent for a fixture that drives no backend traffic. **+0 BackendKinds** (tail stays **38**, 39 declared): the drive arms are raw TLS clients, the `0110`/`0111`/`0118` posture. **`0118`'s `drive()` (`:105-124`) is the template** — dial the listener once, close it, return a **non-nil empty** `[]byte{}` on both sides so `CompareBytes` has a defined result, the dial itself proving the listener bound.

### 7.4 The drive arms — ⚠️ **NO PRECEDENT EXISTS, AND THAT IS A FINDING**

**No fixture anywhere in this tree drives a deliberately failing TLS handshake of the kind this row needs.** Every fixture TLS client (`0004 0018 0045 0079 0080 0108-0111 0119`) pins `MinVersion: VersionTLS12` — every existing arm is configured to **succeed** at the protocol layer. `0110`/`0111` do drive failing handshakes, but exclusively **certificate-verification** ones (`outcomeVerifyError` / `outcomeNoCert`). `0045/driver/driver.go:286` merely *tolerates* a failure and asserts nothing. The `garbage`-bytes precedents (`0046`, `0049`) are plaintext L7 codec arms. **`0120` would be the FIRST.** Build the arms from `0110`/`0111`'s **client-harness shape** while inventing the failure modes.

⚠️ **`reference_go_client_cert_withholding` EXTENDS FROM UNTRUSTED TO UNPARSEABLE CERTS, AND EVERY ARM MUST FORCE-SEND.** At the phase-94 BRAINSTORM, Go silently sent an EMPTY chain for an unparseable leaf under TLS 1.3, so arm (f2) initially measured the CLIENT, not the reference; it was caught only by making the client print what it sent, and resolved under TLS 1.2. `0111:165-167` records the sibling failure: without the forced send *"arm 2 DEGRADES INTO arm 3 … while `CompareBytes` stays green."* **Every arm must print what it sent, and the fixture must assert it.**

⚠️ **The bad-version arm is CLIENT-side ONLY.** `TLSv1_0`/`TLSv1_1` in the YAML **boot-rejects envoy-go** (§0 refutation 10). Pin the Go client to `MaxVersion: VersionTLS11` against the TLS-1.2 floor; the resulting server-side error was measured at §2.2 arm 1.

**Anticipated arms, all sequenced inside the SINGLE `Drive` pair** (§7.1: one directory = one runner branch, so `0120` gets exactly one `DriveReference`/`DriveSubject` pair and at most one `AssertStats`): (i) TLS 1.1-max client vs the TLS 1.2 floor; (ii) plaintext HTTP to the TLS port; (iii) garbage bytes; (iv) **a clean-FIN transport arm that must NOT move the counter** — the discriminating negative control, without which the fixture proves only that the counter can move, never that the predicate discriminates; (v) a valid handshake as the positive control.

### 7.5 The asserter, and the guard that keeps it from vanishing

Read `/stats/prometheus` and key on the **metric NAME**, deliberately ignoring the `envoy_listener_address` **label value** — the mechanism quoted at `0111/driver/driver.go:737-745`, parser `scrapeProm` at `:828-878`. This resolves all three cross-side scope divergences at once (dots, IPv6 brackets, `stat_prefix`), because the Prometheus name carries none of them. Copy `scrapeProm`'s essentials: split on `{` / `LastIndexByte('}')`, key on `line[:open]`, handle the bare and trailing-timestamp variants, **`strconv.ParseFloat` not `ParseUint`** (histogram lines carry `nan`/`inf`), skip non-finite and negative, accumulate `out[name] += uint64(v)`.

⚠️ **`AssertStats` DISPATCH IS AN UNGUARDED TYPE ASSERTION WITH NO `else`** — `runner_test.go:1349`. **A signature typo makes `ok == false` and the entire stats leg vanishes GREEN.** `0118/driver/driver.go:588-594` marks the compile-time `fixture.StatsAsserter` assertion **MANDATORY**; `0120` must carry it.

⚠️ **ASSERT BOTH DIRECTIONS ON EVERY ARM.** A pin proving `connection_error` moved says nothing about whether `fail_verify_error` also moved. Every arm asserts the counter that MUST move **and the set that must NOT** — the same discipline §6.2's `sslLeafRoster` 2×2 proved load-bearing on the unit side. **Keep table rows single-cause.**

---

## 8. NC roster — neutralise, never revert

Per method note 7d, **a NC that is a build break proves nothing**; every NC below leaves the package compiling.

| # | NC | must produce |
|---|---|---|
| 1 | Remove the predicate (unconditional `Inc` on `outcomeOther`) | **RED** — and it is red ONLY with `sslLeafRoster` extended (§6.2 2×2). **Run it BOTH ways** to prove the roster edit is what does the work |
| 2 | Swap the exclusion arm's real `*net.OpError` for the synthetic string error | **the arm FLIPS** — proves the table reads identity, not text (§3.3) |
| 3 | Delete the fifth `NewCounter` registration, keep the `Inc` | **SIGSEGV**, with `GateMatchesInc` still PASSING unless extended (§3.4) |
| 4 | `helpText` entry present, `helpTextRoster` absent | **RED** `extra:` |
| 5 | `helpTextRoster` present, `helpText` entry absent | **RED** `missing:` |
| 6 | Both present, `name_test.go` slice left at four | ⚠️ **GREEN** — this is the documented silent gap; the slice edit is mandatory precisely because nothing catches its absence |
| 7 | Fixture `0120` registered but blank import omitted | **`t.Skipf` — SILENTLY GREEN.** Assert the fixture set by name, both directions |
| 8 | `0120`'s clean-FIN arm asserted as *moving* the counter | **RED** — proves the discriminating arm discriminates |

⚠️ **`reference_gate_command_negative_control`: NC the gate command itself.** Every `-run` selector in the PLAN must be shown to select something, because a selector matching nothing prints `ok` and exits 0 (§6.4, reproduced live).

---

## 9. Cost — MEASURED and ESTIMATED, labelled separately

### MEASURED at this tip
- Production files: **2**. Prototype ran and produced the §6.2 red set.
- `manager.go` prose blocks that go false: **6** (plus one guarded non-site).
- Mandatory test-edit sites: **8** (§6.2), across **5** files — `manager_test.go`, `quic_test.go`, `name.go`, `name_test.go`, `helptext_test.go`.
- Fixture-prose files that go false: **4** (§6.5).
- Rename sites: **4**, roster complete, zero hits in CI or scripts.
- `helpText` entries: **30** today (AST walk and textual count agree); `len(helpText)` guarded by **nothing**.
- Byte-gate / golden set difference: **∅**.

### ESTIMATED — labelled, not measured
- Production lines: the BRAINSTORM's **35-55 added / 12-22 removed** stands as a floor; the sixth prose block and the `gofmt` realignment push toward the upper end. ⚠️ **`~4-6 production lines` is REFUTED** (position 4).
- Fixture `0120`: a new directory on the `0118` + `0110` composite shape. **No number is quoted for its line count**, because no fixture in this tree drives these arms (§7.4) and an estimate anchored on `0110` would understate an unprecedented drive layer — `reference_measured_prototype_is_a_lower_bound`, which has now fired **nine consecutive rows**.
- Task count: ⚠️ **NO FIGURE IS CARRIED.** Four mutually inconsistent estimates are live in two different units (`~9-11` / `~10-13` / `~12-15` tasks / `~4-6` lines), and the `~9-11` **collides** with an unrelated `~9-11` at `phases/75-…/SPEC.md:349`. Per `reference_a_drift_correction_is_itself_a_claim`, on a contested count: **no number.** **The PLAN derives its own.**

### Axis deltas
fixtures **121 -> 122** (`0120`, at the IMPL; **+0 at this stage**) · stat surface **+1 name** *(delta only — three different absolutes are live in this tree at one tip)* · BackendKinds **+0** (tail stays 38) · fuzzers **+0** (no new config field, so no parse arm) · `go.mod` **+0** · packages **+0** · phase dirs **135** (unchanged; the directory already exists) · ROADMAP rows **126**, unchanged.

---

## 10. `ADR-0316` and the house guard

**Next-free is `ADR-0316`, TAIL-derived** — `grep -oE '^## ADR-[0-9]+' | tail -1` gives `## ADR-0315`, and `grep -c '^## ADR-0316'` gives **0**. ⚠️ **NEVER derive from the heading count:** the id space is sparse with exactly one gap at `0209`, so headings+1 reads **315** — an id already TAKEN.

The block appends at the tail in the ADR-0294-0315 shared form: em-dash heading, a single STATUS blockquote, **no `---` separator**, and a retained italic footer. ⇒ `^---$` **STAYS 216**; `^## ADR-` **314 -> 315**; bare `^## ` **322 -> 323** (§Context is a `###`); tail **ADR-0315 -> ADR-0316**; next-free becomes **ADR-0317**.

⚠️ **THE STRICT HOUSE-FORM GUARD `^> \*\*STATUS: PROPOSED` IS RE-ARMED BY THIS BLOCK AND IS DISARMED BY THE PHASE-94 IMPL.** ⚠️ **NO COUNT FOR EITHER `PROPOSED` FORM IS WRITTEN INTO THE ADR'S STATUS LINE**, because that line is itself a hit of both forms and any figure it named would be falsified by its own landing — which is exactly what the phase-93 SPEC did. **Verify by LINE and by ADR, never by the count alone.** The unrelated **ADR-0231 decoy** `^\*\*Status:\*\* PROPOSED` at `:14866` is a different matcher and must not be conflated; **never gate on the unanchored form nor on the middle-ground `^\*\*Status:\*\*.*PROPOSED`**.

### 10.1 §Context outline, drafted at this SPEC

¶1 the departure and what actually blocked it · ¶2 **the reference rule, propagated into a governing document for the first time** · ¶3 the closed-complement inversion · ¶4 the verdict-table category error · ¶5 the by-value footgun · ¶6 the five positions · ¶7 the six-site prose roster and the phantom `B5` · ¶8 the `helpText` matched pair and the moved trap · ¶9 the instrument, and that the row inherits no red gate · ¶10 cost, and the ninth firing of the lower-bound species · ¶11 what this ADR does not decide.

---

## 11. Behavior-contract edit map — **PINNED here, LANDED at the IMPL**

Per §0.1 and ADR-0052 `:1821`, ADR-0316 is the **MANDATED vehicle** for these edits, which land at the IMPL:

| site | edit |
|---|---|
| `BEHAVIOR_CONTRACT.md:1971` | **DELETE ONLY** the clause *"and is still blocked on enumerating its membership"*. ⚠️ **KEEP** the sentence *"The full membership … is UNENUMERATED"* beside it — they are different propositions and only the second survives |
| `BEHAVIOR_CONTRACT.md:1971` | Retitle the departure: it is **CLOSED**, not standing. Record the §2.1 rule and the FOUR-outcomes / FIVE-counters asymmetry of §3.1 |
| `BEHAVIOR_CONTRACT.md:1967` | *"three listener-scope counters … EXTENDED by phase 75 … to a FOURTH"* -> the FIFTH, with the phase-94 attribution |
| `BEHAVIOR_CONTRACT.md:1973` | The QUIC permanent-zero parity **INHERITS to the fifth name BY CONSTRUCTION, not by measurement** — `serveConnection` is the sole `Inc` site for the whole family and `Manager.Start`'s accept-loop launch loop `continue`s on `rt.kind == kindQUIC`. **State it as inheritance, exactly as phase 75 did, and do NOT claim a QUIC probe that was not run** |
| `BEHAVIOR_CONTRACT.md:1975` | The cross-side scope divergences apply to the fifth name **as pre-existingly as to the first four**; none is re-opened |
| `ROADMAP.md:155` | Position 4: mark the *"~4-6 production lines"* and *"reference side is an unprobed doc claim"* claims **SUPERSEDED**, citing ADR-0316. ⚠️ **COUNT THE ROW'S FIELDS UNDER BOTH FORMS BEFORE AND AFTER** — row 93 must stay NF=8 |

⚠️ **The contract's `:1971` is ONE enormous paragraph line.** Every edit above is a **within-line** surgical change. `sed`-style line replacement will destroy neighbouring propositions; edit by literal text, and **re-verify that `:1971` still carries the `UNENUMERATED` sentence afterwards**.

---

## 12. Sentinel maintenance — a SENTENCE-NARROWING row, narrowed AT THE IMPL

**No deferred sentence is narrowed at any pre-IMPL stage** (the phase-57 precedent, `reference_sentinel_deferred_sentence_live_vs_historical`). **`ROADMAP.md` is BYTE-UNTOUCHED at this SPEC and row 94 STAYS `in-progress`.**

The window this row narrows is **`:226`**, under `### Observability family` (heading `:222`). Its live sentence carries **THREE** items, **read rather than split** — ⚠️ **no gate may rest on a ` + ` split, which is empirically wrong on 3 of the 6 windows**:

1. the DYNAMIC half of the downstream TLS `ssl` family (ciphers/versions/curves/sigalgs);
2. **"the uncounted non-certificate handshake-failure bucket (`connection_error` on the reference, where envoy-go's `other` arm counts nothing)"** — **THIS ROW**;
3. tracing `spawn_upstream_span` / `http_service` / force-trace.

⚠️ **NARROWING IS NOT SENTINEL PROGRESS, AND THIS IS MECHANICALLY CONFIRMED.** Check (2) keys on the **phrase**, not the clause. This stage re-ran the positive control: doctoring both candidate phrases out of a scratch copy drives check (2) to **0**, so it is not a stuck gate — **but the control had to remove the PHRASE.** Removing a clause from a window sentence leaves the phrase, the line and the count untouched. ⇒ **Only deleting the line could move check (2), and deleting it is FORBIDDEN.** Charter this row as narrowing; never as sentinel progress.

---

## 13. Sentinel — RUN MECHANICALLY AT `3b10490a`, ACTUAL OUTPUT

```
(1) NOT DONE: row 94                                   <- ONE line, CORRECT and EXPECTED while row 94 is open
(2) 204: 210: 216: 226: 232: 240:                      <- SIX
(3) (silent)
```

**All four mandated NCs re-run, all four FIRED:**

| NC | result |
|---|---|
| **A** — row 62 doctored to `in-progress` | **TWO** lines: `NOT DONE: row 62`, `NOT DONE: row 94` (NC landed, inspected: `[ in-progress ]`) |
| **B** — `want=125` on the real file | **TWO**: `NOT DONE: row 94` + `GATE FAIL: examined 126 data rows, expected 125` |
| **C** — `gRPC-family row` doctored out | NC landed (**0** residual), fired `NEVER OPENED: gRPC` |
| **D** — `-family row` with the `--` guard | occurrences **96**, lines **68** |

**CHECK-(2) POSITIVE CONTROL:** doctoring **both** phrases out of a scratch copy drives check (2) to **0**. Not a stuck gate.

⚠️ **The six window lines are BYTE-IDENTICAL to the phase-94 BRAINSTORM's post-insertion record** — md5 (first 12) `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 4ad940205410 b2680e6f4fbf 6caa1c3ce0e7`. **That digest match is the proof no deferred-candidate line was tidied.**

**Field-count instrument, baselined before any edit:** row 94 is **NF=8 under BOTH the naive and the escape-aware form**, with **7** pipe characters (the delimiters only). Malformed-row baseline under the **escape-aware** form: exactly **two** rows, **57** (`NF=9`) and **69** (`NF=10`); the **naive** form reads **17**. ⚠️ **Any gate must state WHICH form it uses.**

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately not created** (verified absent at the git root and in the stage worktree).

⚠️ **THE MARGIN IS TWO, NOT ONE.** Termination is blocked **two independent ways** — check (1) by an OPEN ROW and check (2) by six lines. **Closing row 94 removes only the first**, and the margin returns to ONE the moment row 94 flips `done`. ⚠️ **DO NOT "TIDY" A DEFERRED-CANDIDATE LINE — DELETING THE LAST ONE ENDS THE PROJECT.**

---

## 14. Deferred items — newly surfaced by THIS SPEC, none chartered

- ⚠️ **A NEW FLAKE FOR THE REGISTER: `TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure`** (wasm) — a 1/400-trial, 2 ms-watchdog concurrency flake at `pause_gen_test.go:475` (`late=1/400 … want 0/400`). It surfaced in this stage's prototype run, that package contains **no `ssl` code**, and the test **passes at the clean tip**. **Not part of this row's red set.** Recorded because `reference_recurring_flake_may_be_production_bug` says a green run clears nothing.
- **`0108`'s two *"envoy-go emits NO `ssl.*` stats whatsoever"* confessions** (§6.5) — false since phase 74, outside this row's delta.
- **`0118/driver/driver.go:31`'s falsified *"TLS/SDS band"* characterization** (§7.2).
- **No `len(helpText)` guard exists**, and the file carries two ungated prose counts (§6.1).
- **`manager.go:441` cites `handshake_server.go:964, go1.26.5`** for `noClientCertErrText`; at the live toolchain (**go1.26.7**) that text is at **`:970`** and `:964` is an unrelated line. A stdlib-line cite that drifts with the toolchain; **the tripwire itself is sound** because the test builds the error from a LIVE handshake.
- **`net.ErrClosed` is an unexercised predicate member** (§2.2).
- **The synthetic `errors.New("connection reset by peer")` residual** (§3.3) — recorded as having no measured production producer, not claimed impossible.
- **Carried, costs as re-derived at the BRAINSTORM:** GET `/runtime` (the strongest runner-up, `~+160/-15` across 4 files, +0 fixtures, +0 stats) · `POST /runtime_modify` · lifting the six `runtime_key` rejects (a strict SUPERSET of GET `/runtime`) · 1xx interim responses (cheapest CODE, but **in no window at all** and needing a new BackendKind + fixture) · the four dynamic `ssl` families · the other TEN fixed `ssl.*` names · `0061-lb-ring-hash`'s σ-margin second occurrence.

**None is added to any ROADMAP `candidates:` sentence at this stage** (§12) — stated as a commitment the IMPL must verify by grep, not as an accomplished fact.

---

## 15. Cite hygiene — what the PLAN must NOT inherit

**Wrong at this tip, corrected here:** `helptext_test.go`'s `TestHelpText_KeySetExact` is at **`:120`**, not `:100` (`:101` is the doc-comment head) · the five `outcomeOther` rows are `:4398-4402`, not `:4394-4402` · `0111`'s `wantObservable` is at **`:181`**, not `:99` (which is now inside `sdsProjectedNames`, opening `:93`), **and the path is `0111-tls-cvc-empty-dynamic-fallback/driver/driver.go`, not `0111/driver.go`** · `manager.go:441`'s stdlib cite (§14).

**Standing rules this row exercised:**
- **PATHSPEC-SCOPE every symbol assertion** — both edit anchors are unique only under `-- '*.go'` (§0 refutation 3).
- **A "drift-proof" anchor is itself a claim.** Both were uniqueness-checked; both failed the unscoped form.
- **`grep -c` prints `0` AND exits 1** — capture with `v=$(… || true)`, never `$(… || echo 0)`, which emits two zeros.
- **`gofmt -l` never exits non-zero — gate on OUTPUT.** `golangci-lint` does exit non-zero, but still gate on output. Its **misspell runs in locale US** and has fired on three consecutive stages' prototypes: sweep British spellings in `.go` comments before the gate; Markdown prose may use them freely.
- **`go test` without `-v` prints zero `=== RUN`** — `RUN=0` beside `RC=0` is a vacuous green. On `-v` output use the anchored `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`; an unanchored `grep -c FAIL` reads nonzero on a fully green tree.
- **`-count=1` is NOT optional for the differential** — the harness builds envoy-go as a **subprocess**, so a production edit is not a compile-time input to that test binary and the cache serves a stale PASS.
- **`go test ./...` drives Docker in TWO places** — exclude both: `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`.
- **`-race` on the differential suite is VACUOUS** — the subject is an unraced subprocess.
- **`rc=$?` after a pipe returns the LAST command's status** — use `out=$(…); rc=$?` or `PIPESTATUS`. ⚠️ **`INNER_EXIT` does not exist in this repo.**
- **`pgrep -f` / `pkill -f` match your own shell and kill the tool call (exit 144)** — kill only PIDs you captured.

---

## 16. What the PLAN owes

1. **Derive its own task count.** No figure is inherited (§9) — four inconsistent ones are live and one collides with an unrelated figure.
2. **Boot both sides on the `tls_params` config BEFORE writing any arm** (§7.3). It is unprecedented in this tree and is the row's largest unmeasured risk.
3. **Build the counter unit table on production-representative values**, with the identity-vs-text NC of §3.3 and the `sslLeafRoster` 2×2 of §6.2. **Run NC 1 BOTH ways** — the roster edit is what makes the predicate falsifiable at all.
4. **Sequence all `0120` arms inside the single `Drive` pair**, including the **discriminating clean-FIN arm** without which the fixture proves only that the counter can move.
5. **Assert the fixture set BY NAME in BOTH directions** — no fixture-count gate exists, and an unregistered fixture is `t.Skipf`'d silently.
6. **Land `helpText` + `helpTextRoster` + `name_test.go`'s slice in ONE commit**, and budget the `gofmt` realignment.
7. **Land the fifth `GateMatchesInc` assertion in the same commit as the `Inc`** — the omission's failure mode is a process crash that a PASSING test does not catch.
8. **Set-difference the §6 edit roster against the tree again at the PLAN's own tip**, and **re-derive every count in §9 and §13.** ⚠️ **`feedback_brief_citations_not_evidence` applies to incidental figures: re-derive every number restated from this document, including this sentence's own claim that there are none left to check.**
