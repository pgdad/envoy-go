# PLAN 74 — downstream TLS handshake-outcome stats (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Stage:** PLAN (lifecycle-state 2 → 3). Docs-only — **ZERO production `.go`**; the only two files in the phase-directory delta are this `PLAN.md` and `PROGRESS.md`. Worktree `.worktrees/phase-74-plan`, branch `phase-74-plan`, tip **`ab13fc19`** (the phase-74 SPEC squash — master), per `feedback_git_worktrees`.
>
> **Row 74 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, `reference_roadmap_split_phase_row_done`). **ADR-0296's §Context is ALREADY DRAFTED** at the SPEC squash (`DECISIONS.md:17254-17280`, STATUS **PROPOSED**, §Context only, closing at `:17280` with `*(§Decision + §Consequences land at the phase-74 IMPL.)*` — **the LAST line of the file**). The IMPL **COMPLETES ADR-0296 IN PLACE**: append `### Decision` + `### Consequences` after the RETAINED footer. No renumber, no new ADR; tail stays **ADR-0296**, next-free **ADR-0297**.
>
> **Baselines RE-DERIVED at `ab13fc19` (`[RUN]` in this worktree, NOT copied):** fixtures **119** (`ls -d test/fixtures/[0-9]*/ | wc -l`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · BackendKind tail **38** (`fixture.go:614`) · stat surface **1201** (BEHAVIOR_CONTRACT doc count — NO mechanical command) · DECISIONS tail **ADR-0296** PROPOSED · `go build ./...` clean · `go test ./internal/listener/ ./internal/stats/ -count=1` **ok / ok**.
>
> **Sentinel expectation:** check (1) prints `NOT DONE: row 74`; check (2) prints **3** via the full-phrase form ONLY — `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` (`reference_sentinel_deferred_sentence_live_vs_historical` — **cite the command, never the adjective**; a naive `grep -c 'candidates:'` returns 11); check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`. **All three RUN at THIS PLAN close, TWICE** (worktree + landed master post-push).
>
> **⚠️ NO PARALLEL STREAM.** Master (`ab13fc19`) IS the SPEC squash. `git diff --stat f5a38c40 HEAD -- '*.go' go.mod go.sum test/` is **EMPTY** — the only delta over the SPEC's derivation tip is docs (`SPEC.md` + the ADR-0296 §Context append + STATE + the router). So the production tree is byte-identical to what the SPEC re-derived, and §1 is a full independent re-verification rather than a drift sweep. `feedback_parallel_stream_mints_fresh_drift` does not bite at the PLAN — **but re-run the check at the IMPL tip anyway.**
>
> **⚠️ RE-DERIVE, do not execute.** A PLAN is not evidence; a SPEC is not evidence either (`feedback_brief_citations_not_evidence`, `reference_quoting_is_not_executing`). Where this document cites, go look; where it claims control flow, walk the call graph; default to REFUTED. **Take every `file:line` from THIS document or from SPEC 74 — never from the phase-65/67/68 documents, whose cites are one-to-four stages stale.**

---

## 1. Re-derivation ledger — every SPEC §3/§7/§8/§9/§11/§12/§14 anchor re-opened at `ab13fc19`

**All SPEC anchors RE-DERIVED at `ab13fc19` by FOUR read-only agents on disjoint remits** (A: `internal/listener` + `internal/stats` + `internal/statssink` + identifier collisions; B: the ctx-override call-graph trace + the stdlib block; C: `test/differential/fixture` + `runner_test.go` + fixture `0111` + the precedent scrapers; D: `BEHAVIOR_CONTRACT.md` + `DECISIONS.md` + `ROADMAP.md` + `STATE.md` + the phase-73 PLAN/PROGRESS templates), **plus controller re-verification of every load-bearing correction** (`reference_a_drift_correction_is_itself_a_claim` — a correction is itself a claim; the controller personally re-ran the collision greps, the parallel-stream diff, the `crypto/tls` symbol enumeration, the `registerListenerMetrics` call-site enumeration, the `tlsMode`-ordering trace and the baseline build/test).

**RESULT: TWELVE SPEC claims REFUTED and TEN new findings (F1–F10), THREE of the findings SEVERE.** Unlike phase 73 — which found zero code drift — **this SPEC's code anchors do NOT all hold.** Two refutations are load-bearing enough to change instructions (**RD-QUICSTALE**, **RD-CTXBOUND**), one is a self-refuting grep the IMPL would otherwise misread as drift (**RD-GREP0**), and three of the new findings (**F1/F2/F3**) mean two tasks cannot be written the way the SPEC's spine implies. All seven identifier-collision greps are **0** at tip.

### 1.1 The RD-* ledger

| # | Anchor / SPEC claim | RE-DERIVED at `ab13fc19` | Where |
|---|---|---|---|
| **RD-TREE** | Is there a parallel stream since the SPEC's derivation tip `f5a38c40`? | **NO.** `git diff --stat f5a38c40 HEAD -- '*.go' go.mod go.sum test/` ⇒ **EMPTY**. The full delta is 4 docs files (`DECISIONS.md` +28/-0, `STATE.md`, `SPEC.md` new, `next-prompt.txt`). ⇒ the SPEC's `f5a38c40`-dated code anchors are re-derivable at this tip, and any failing one was **wrong when written**. | §1 |
| **RD-IDENT** | SPEC §5: seven drafted identifiers free | **ALL SEVEN FREE — 0 hits each**, controller-run over `--include='*.go'` repo-wide: `classifyHandshakeErr` **0** · `handshakeOutcome` **0** · `sslHandshake` **0** · `sslFailVerifyError` **0** · `sslFailVerifyNoCert` **0** · word-boundary `verifyError` **0** · word-boundary `noCert` **0**. ⚠️ **Use the word-boundary form for `verifyError`/`noCert`** — both are common substrings; a naive grep is not a collision check. | T1 |
| **RD-STRUCT** | SPEC §1.1: `listenerRuntime` `:141`–`:186`, 18 fields; `tlsMode` `:145` | **ALL THREE CONFIRMED.** `:141 type listenerRuntime struct {`, `:186 }`, exactly 18 fields (name, addr, netLn, **tlsMode**, kind, chainSpecs, defaultSpec, defaultChain, chainByName, listenerFilterFactories, lfTimeoutMs, continueOnLfTimeout, lfPeekBufSize, downstreamCxTotal, downstreamCxActive, dm, udpConn, quicCloser). `:145 tlsMode bool`. ⇒ +3 fields takes it to **21**. | T2 |
| **RD-TLSMODE-ORDER** ⚠️ | SPEC §3.3: `tlsMode` set at `:639` from `anyTLS`; is it set BEFORE `registerListenerMetrics` reads it? | **CONFIRMED SAFE, and this is the `reference_retention_field_populate_before_value_copy` hazard class discharged.** `tlsMode: anyTLS` is a field of the `listenerRuntime` **composite literal at `:639`**, inside the build-time runtime constructor — which completes before `Manager.Start` runs. `registerListenerMetrics` is called from `Start` (`:993`) and from `startQUIC` (`quic.go:45`), both strictly later. **No late-write hazard.** | T2 |
| **RD-ANYTLS** | SPEC §3.4: `anyTLS` set at `:478` "from the same per-chain `ci.tlsCfg`" | **LINE HOLDS, PROSE REFUTED (harmless).** `:477 chainTLS = dc.TLSConfig` / `:478 anyTLS = true`. It is set from the chain's **built** `dc.TLSConfig`; `ci` is not constructed until `:510`. Same object, different name. **Do not repeat the SPEC's phrasing in ADR-0296.** | T9 |
| **RD-REGFN** | SPEC §1.1: `registerListenerMetrics` `:351-355`, prefix `:352`, the two names `:353`/`:354` | **ALL CONFIRMED, verbatim.** `:351 func registerListenerMetrics(r *stats.Registry, rt *listenerRuntime) {` · `:352 prefix := "listener." + normalizeAddr(rt.addr) + "."` · `:353` counter `downstream_cx_total` · `:354` gauge `downstream_cx_active` · `:355 }`. | T2 |
| **RD-REGCALLERS** ⚠️ | Not in the SPEC: how many call sites does `registerListenerMetrics` have, and can a QUIC listener DOUBLE-register (⇒ duplicate-registration PANIC)? | **EXACTLY TWO, and they are MUTUALLY EXCLUSIVE — no double-register.** `manager.go:993` (the TCP path) and `quic.go:45` (inside `startQUIC`). `Start`'s FIRST loop `continue`s on `rt.kind == kindQUIC` at **`:973`** (the guarded block is `:963-974`) after calling `startQUIC`, so a QUIC runtime never reaches `:993`. ⇒ **the `rt.tlsMode` gate in ONE function covers BOTH runtimes with no kind check and no special case — SPEC §3.4's conclusion is structurally confirmed, not merely probed.** | T2, T3 |
| **RD-NORMADDR** | SPEC §1.1 R3: `normalizeAddr` `:341-343` underscores dots too; documented at `:326-332` | **CONFIRMED EXACTLY.** `:342 return strings.NewReplacer(":", "_", ".", "_", "[", "", "]", "").Replace(addr)` — the exact 8-arg list. The `:326-332` rationale comment is verbatim as the SPEC quotes it (the SN3 `strings.Index(tail, ".")` truncation argument). ⇒ envoy-go emits `listener.0_0_0_0_10000.ssl.handshake`; the reference emits `listener.0.0.0.0_10447.ssl.handshake`. **This decides T6's asserter shape.** | T2, T6 |
| **RD-MIXED** | SPEC §3.3: mixed TLS+plaintext chains rejected at `:516-525` | **CONFIRMED.** `:518 if anyTLS && anyPlaintext {` … `:522` `listener: %q: filter_chains[%d]: mixed TLS and plaintext chains on one listener are not supported` (ADR-0033 clause 5 / ADR-0078 clause 5, named in the `:516-517` comment). ⇒ `rt.tlsMode` ⟺ *every* chain has `tlsCfg != nil`. **This is the invariant that makes the registration gate and the Inc guard equivalent — see F3, which is why it is load-bearing rather than incidental.** | T2, T4 |
| **RD-HANDSHAKE** | SPEC §11: `:1176` the TLS branch, `:1178` `HandshakeContext` with `err` in the `if` init, `:1179-1181` error branch, `:1183` success | **ALL CONFIRMED, byte-exact.** `:1175 var dispatchConn net.Conn = pkConn` · `:1176 if selected.tlsCfg != nil {` · `:1177 tlsConn := stdtls.Server(pkConn, selected.tlsCfg)` · `:1178 if err := tlsConn.HandshakeContext(ctx); err != nil {` · `:1179` log · `:1180` close · `:1181` return · `:1183 dispatchConn = tlsConn` · `:1184 }`. **`err` is scoped to the `if` init** ⇒ the classify+Inc goes at `:1179` (before the `log.Printf`); the success Inc goes at `:1183`. Config is `selected.tlsCfg`, server conn is `tlsConn`. | T4 |
| **RD-ERRORS** | SPEC §1.1 R2 / §4: `errors` already imported at `manager.go:6` ⇒ +0 imports | **CONCLUSION HOLDS, LINE REFUTED.** `errors` is at **`:7`**; `:6` is `"crypto/x509"`; `stdtls "crypto/tls"` is at `:5` (HOLDS). Both needed symbols are present ⇒ **+0 production imports CONFIRMED**. | T1 |
| **RD-QUICGUARD** | SPEC §3.4: `acceptLoop`'s sole call site `:1009`, "guarded at `:997-1001` by `if rt.kind == kindQUIC { continue }`" | **CONCLUSION HOLDS, LOCATION REFUTED.** The `:997-1001` guard sits in **`Start`'s SECOND loop (the accept-loop launch loop)**, not inside `acceptLoop` — which is declared at **`:1034`**. `:1009 go rt.acceptLoop(ctx, ln)` is the sole production call site (the only other is `manager_test.go:3726`). `:1081 go rt.serveConnection(ctx, raw)` is `serveConnection`'s sole call site. | T3 |
| **RD-QUICTEST** ⚠️ | SPEC §1.1 R4: `quic.go:102-103` Incs BOTH cx metrics, "pinned live by `quic_test.go:92-97`" | **HALF REFUTED — LOAD-BEARING FOR T3.** `quic.go:102 rt.downstreamCxTotal.Inc()` / `:103 rt.downstreamCxActive.Inc()` both exist. But `quic_test.go:92-97` asserts **ONLY `downstream_cx_total`** (via `pollCounter`); **`downstream_cx_active` is asserted NOWHERE in `quic_test.go`**, and `pollCounter` (`:32`) type-asserts `*stats.Counter` — **there is NO gauge equivalent.** ⇒ **T3 must NOT claim an existing pin for the gauge half.** Either add a gauge poller or narrow T3's assertion to the counter; this PLAN narrows (T3 Step 1). | T3 |
| **RD-QUICBOOT** | SPEC §3.4: `startQUIC` hard-errors without TLS (`quic.go:33-36`); `quicAcceptLoop` `:88`; `serveQUICConnection` `:120`; the post-handshake comments `:84-85`/`:109` | **ALL CONFIRMED.** `:35 return errors.New("quic listener has no TLS config (mandatory TLS not built)")` · `:45 registerListenerMetrics(reg, rt)` · `:49 go rt.quicAcceptLoop(ctx, ql)` · `:56-66 quicTLSConfig()` · `:88 func (rt *listenerRuntime) quicAcceptLoop(...)` (the BRAINSTORM's `:104` stays refuted) · `:120 serveQUICConnection`. `:84-85` reads *"accepts QUIC connections whose handshake has already completed (quic-go's Accept returns post-handshake)"*; `:109` *"The QUIC/TLS-1.3 handshake is complete (Accept returned)."* ⇒ **every QUIC listener that boots has `tlsMode == true`, structurally.** | T3 |
| **RD-QUICSTALE** ⚠️⚠️ | SPEC §10's spine still carries the REFUTED first QUIC reading | **REFUTED — AND THIS IS THE ONE THAT WOULD HAVE BROKEN THE IMPL.** THREE sites in `SPEC.md` contradict its own authoritative §3.4/§16: **`:1`** (the title — *"TLS-listener-gated and **TCP-only**"*), **`:315`** (*"§3.4 **gates QUIC out** inside `registerListenerMetrics`"*), **`:320`** (spine item 2 — *"the **TLS-and-TCP-gated** registration (`rt.tlsMode && rt.kind != kindQUIC`)"*). §3.4, §16 and **ADR-0296 §Context ¶8(ii) (`DECISIONS.md:17274`, verified verbatim: *"QUIC is SHARED PARITY, not a departure … The gate is `rt.tlsMode` alone"*)** all say the gate is **`rt.tlsMode` ALONE**. ⚠️ **A PLAN that copied §10 item 2 verbatim would implement the refuted gate — and then Break D would be UNFIREABLE, because there would be nothing left to *add*.** **THE GATE IS `rt.tlsMode` ALONE. NO KIND CHECK.** | T2, T3, T9 |
| **RD-CTXBOUND** ⚠️⚠️ | SPEC §3.5 item 3: *"`internal/listener/` has ZERO `context.WithTimeout`/`WithDeadline` in production code"* — the bound that makes the classifier shippable | **PREMISE REFUTED; CONCLUSION SURVIVES ON STRONGER GROUND.** There **is** one production hit: **`internal/listener/listenerfilter/pipeline.go:43`** — `ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)` inside `(*Pipeline).Run`. **But it cannot escape:** `Run`'s signature returns **only `retErr error`** — no `context.Context` return value — so the derived ctx is function-local; and `serveConnection` **never rebinds `ctx`** (`grep` for `ctx =` / `ctx :=` / `ctx, … =` in `manager.go` ⇒ **ZERO hits**; the four occurrences in `serveConnection` are the `:1113` parameter decl, `:1149` pass-by-value into `p.Run`, `:1178` `HandshakeContext`, `:1189` `serveNetworkChain` — all uses, no assignments). Also **ZERO `SetDeadline`/`SetReadDeadline`/`SetWriteDeadline` in ANY non-test file under `internal/listener/`** (the i/o-timeout variant of the hazard does not exist). ⇒ **RESTATE the bound as *"no production deadline REACHES `HandshakeContext`"*, and NAME the guard: `Pipeline.Run`'s single-return signature. The code is one refactor away from the hazard** — if `Run` ever returned its derived ctx and the caller assigned it back, `defer cancel()` would already have fired and EVERY TLS handshake on a listener with `lfTimeoutMs > 0` would fail `context.Canceled` with the socket closed underneath it. **B6/ADR-0296 must say this.** | T4, T7, T9 |
| **RD-CONNLINES** | SPEC §3.5: the override block at `crypto/tls/conn.go:1539-1547` | **REFUTED (off by three).** At `go1.26.5` (GOROOT `/snap/go/11227`): `HandshakeContext` **`:1513`** → `handshakeContext` **`:1519`**; the guarded block is **`:1536-1547`** — `else if ctx.Done() != nil {` `:1536`, `context.AfterFunc` **`:1538`** (its func closes `c.conn` at `:1539`), the override `defer func(){ if !stop() { ret = ctx.Err() } }()` **`:1541-1546`**. `ret` is a **named return**, clobbering the real error set at `:1562`. | T1, T7 |
| **RD-NOCERT** | SPEC §3.5: the no-cert error is a bare `errors.New("tls: client didn't provide a certificate")` at `handshake_server.go:964`; `crypto/tls` exports FOUR error-ish symbols and ZERO error VALUES | **BOTH CONFIRMED at this session's toolchain (`go1.26.5 linux/amd64`), controller-run.** `handshake_server.go:964: return errors.New("tls: client didn't provide a certificate")`. `go doc crypto/tls` error-ish exports: **`AlertError`**, **`CertificateVerificationError`**, **`ECHRejectionError`**, **`RecordHeaderError`** — four TYPES, **zero `var` sentinels**. ⇒ **a string comparison is FORCED**, and the live-handshake test construction (Break F) is the only thing that makes it a tripwire. | T1 |
| **RD-CHECKNAME** | SPEC §3.6 blocker 1: a hyphenated name PANICS via `checkName` at `registry.go:107` | **CONCLUSION HOLDS, LINE REFUTED.** `:107` is the **duplicate-registration** panic inside `register()`. `checkName` is declared **`:115`** and panics at **`:117`** (`stats: invalid metric name: %q (must match %s)`). `NamePattern` at `:48` confirmed: `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` — **no hyphen**. `NewCounterIfAbsent` `:161`, the ADR-0117 post-Freeze comment `:175-176`, `getOrRegister` `:177-188`, `NewGaugeIfAbsent` `:208` — all HOLD. `Walk` is `:138` (`func (r *Registry) Walk(fn func(Metric))`, registration-order, RLock-held); `NewCounter` is `:84`. **⚠️ Cite `:117`, not `:107`, in B1's restated blocker text.** | T7, T9 |
| **RD-SN3** | SPEC §8.1: SN3 rule `name.go:37`, impl `:85-93`, `flattenToProm` `:370-376` | **`:37` and `:370-376` HOLD; the impl is `:83-92`, not `:85-93`.** SN3's `rest` is NOT dot-flattened in `ExtractTags`; flattening happens in `flattenToProm` (`:375 "envoy_" + strings.ReplaceAll(residual, ".", "_")`). ⇒ `listener.<addr>.ssl.handshake` → residual `listener.ssl.handshake` → **`envoy_listener_ssl_handshake{envoy_listener_address="<addr>"}`**. **Both sides land on the SAME metric NAME; only the label VALUE differs.** T6's shape is confirmed by construction. | T6 |
| **RD-HELPTEXT** | SPEC §7: `name.go:452-464` `helpText` has no `ssl.*` entry | **CONFIRMED — and the map ENDS at `:464`, the file's LAST line**, so three entries append inside `452..464` with no structural move. Exactly **11** entries; **no `ssl.*` metric-name entry** (⚠️ `grep -rn 'ssl' internal/stats/` returns **3** hits, all inside `acce**ssl**og_dropped` — state the property, not the grep count; this is RD-GREP0's class one level down). Entry format `"<prom_name>": "<English sentence>",`. | T5 |
| **RD-GUARD** | SPEC §10.1: `statssink/registration_test.go:26`, one of FIVE, counts via `reg.Walk` and never inspects `m.Name()` | **CONFIRMED EXACTLY.** The five are `:26 TestNoNewStat_RegistrationGuard` · `:53 …Statsd…` · `:81 …DogStatsd…` · `:109 …Graphite…` · `:137 …OTLP…` (file = 164 lines). All route through the file-local `countMetrics` (`:13`): `n := 0; reg.Walk(func(stats.Metric) { n++ }); return n` — **an unfiltered total, asserted `== 0`**. ⇒ **a count-only inversion is INSUFFICIENT; T2's guard must assert the NAME SET.** | T2 |
| **RD-TWOMETRICS** | SPEC §1.1 D7: `manager_test.go:1911-1913` doc / `:1928` decl / `:1947-1948` presence-only | **CONFIRMED, with the doc span CORRECTED: the doc comment runs `:1911-1927`, not `:1911-1913`** (it carries a PLAN-deviation note through `:1927`). `:1928 func TestListenerManager_AllocatesTwoMetricsPerListener(t *testing.T)`; `:1947 r.Walk(...)` collects `seen`; `:1948 wantSubstr := []string{".downstream_cx_total", ".downstream_cx_active"}`; the loop asserts `HasPrefix("listener.") && HasSuffix(w)` **presence only — never counts.** ⇒ **+3 produces NO RED from it** while its name and doc silently go false. | T2 |
| **RD-STATSASSERTER** | SPEC §8.2: `fixture.go:75` `StatsAsserter`; `runner_test.go:1342-1349` the sole silent dispatch | **BOTH CONFIRMED, and the signature is now pinned VERBATIM.** `fixture.go:75-77`: `type StatsAsserter interface { AssertStats(t TB, refAdminAddr, subjAdminAddr string) }` — **method `AssertStats`; params `t TB`, `refAdminAddr string`, `subjAdminAddr string`, in that order; NO return value; reference addr FIRST.** Dispatch `runner_test.go:1347-1348`: `if sa, ok := d.(fixture.StatsAsserter); ok { sa.AssertStats(t, ref.AdminAddr(), subj.AdminAddr()) }` — **no `else`, no log, no skip message: `ok == false` is TOTALLY SILENT.** `fixture.TB` is `:64-68` (doc `:61-63`), exactly **Errorf / Fatalf / Helper** — no `Logf`, no `Cleanup`. **There is NO `fixture.StatsArgs` — it does not exist anywhere in the repo.** | T6 |
| **RD-RUNORDER** | SPEC §8.1: nothing pre-moves `l_edf`'s `ssl.*`; `closeServers()` fires before `AssertStats` | **CONFIRMED BY STEP ORDER.** `runner_test.go`: step 5 `DriveReference` `:1245` → step 6 `DriveSubject` `:1271` (which calls `d.closeServers()` at `driver.go:340`) → step 7 `CompareBytes` `:1282` → step 9 `ProbeAdmin` `:1330` → **step 10 `AssertStats` `:1348`**. ⇒ Shape A scrapes admin only, long after all three arms have settled, and is **unaffected** by the SDS teardown. **Shape A's justification is now execution-ordered rather than asserted.** | T6 |
| **RD-0111** | SPEC §8.4/§11: the `0111` anchors | **ALL HOLD.** `driver/driver.go` = **616** lines: RD3 disclaimer `:72-82` (SPEC's `:73-81` is the inner span) · `mustCA` `:193` · `mustLeaf` `:220` · `mustClientCert` `:246` · `BackendCount() int { return 1 }` `:293` · `SubjectConfig(_, subjListenerPort, …)` `:317` (**the ref port IS discarded**) · `d.closeServers()` `:340` · forced-send `GetClientCertificate` `:520` · `var _ fixture.Driver = (*edfDriver)(nil)` `:615-616` — **the ONLY compile-time assertion in the file.** `README.md` **176** lines (`:100-110`, `:162-165` HOLD) · `expectations.yaml` **200** lines (`:109-119`, `:133-153`, `:177-184`, `:189` all HOLD) · `envoy.yaml:24-25` HOLD verbatim · `envoy-go.yaml:70 require_client_certificate: true` HOLD. Reference listener port **10447** (`driver.go:42`, `envoy.yaml:67`). | T6 |
| **RD-BC** | SPEC §9: the B1–B8 anchors | **ALL CONFIRMED.** `:916` phase-65 three-arm narration closing *"envoy-go's accept/reject decisions match"* · `:918` phase-67 init-hold (*"no TLS alert; no `ssl.*` movement"*) · **structure exact: `:926` rejects para · `:927` blank · `:928` the C3 coverage boundary · `:929` blank · `:930` `**Differential coverage.**`** · `:962` item 13 naming `ssl.no_certificate`. `1201` occurs at **exactly two** lines, `:831` and `:847`. File = **5730** lines. | T7 |
| **RD-1200** ⚠️ | SPEC §9 B7 / D5: "THREE stale `1200`s survive at `:1429`/`:1463`/`:1495`" | **THE THREE HOLD — BUT THE SET IS BIGGER AND MOSTLY *NOT* STALE.** `\b1200\b` has **13** hits. `:1429`/`:1463`/`:1495` are the three **narrative** stale ones (they assert a *current* surface). The other ten — `:732` (`1198 → 1200`), `:757`, `:763`, `:767`, `:779`, `:795`, `:805`, `:815`, and the two LEDGER lines **`:4986` (`Phase 47.1 — 1200 → 1200`)** and **`:4988` (`Phase 51 — 1200 → 1200`)** — are **HISTORICALLY CORRECT** statements about their own phases. ⚠️ **DO NOT "fix" them.** B7 touches `:831`/`:847` (→ 1204), the three narrative stale ones, and ADDS a ledger line. | T7 |
| **RD-LEDGER** | SPEC D5: no recorded `1200 → 1201` step | **CONFIRMED, AND THE LEDGER'S SHAPE IS NOW PINNED.** The canonical ledger is under `### Stat surface` at **`:4938`**, running `:4942` (`Phase 36.1 — 1116 → 1119`) through **`:4988` (`Phase 51 — 1200 → 1200`) = THE LEDGER TAIL**, followed by `### Forward-pointer note (26.3)` at **`:4990`**. **`1201` never appears in the ledger at all**, and `1198 → 1200` appears only at `:732` (inside the Zipkin section), never in the ledger. ⇒ **a `**Phase 74 — 1201 → 1204**` line belongs immediately after `:4988`, before `:4990`.** The IMPL must decide EXPLICITLY whether to also close the `1200 → 1201` hole (see T7 Step 3 — this PLAN says record it, do not invent it). | T7 |
| **RD-ADR0286** | SPEC §14: ADR-0286 heading, §Context `:16890`, C3 `:16908`, roster of FIVE | **ALL CONFIRMED.** Heading `:16888`; §Context `:16890`; §Decision `:16892`; §Consequences `:16907`; **C3 `:16908`**. C3's roster is **exactly FIVE** — `ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert` / `ssl.ciphers.*` / `ssl.versions.*`. **No six-name roster, NO cert-expiry gauge in C3** (that roster is `phases/65-…/SPEC.md:282`). C3 also cites `downstream_cx_total` at `manager.go:353` — **which still HOLDS at this tip.** | T9 |
| **RD-CORRFORM** | SPEC §1.1 D2: the correction FORM is `DECISIONS.md:16901`'s indented blockquote | **CONFIRMED, indentation measured by `cat -A`: exactly TWO literal spaces, then `> [`.** Form: `  > [CORRECTED at phase 67/ADR-0289: … ]`, closing with a sentence stating what still stands. It sits directly beneath the `:16900` bullet, indented as a continuation of that list item. The two INLINE precedents (`:17185`, `:17209`, both ADR-0294) use `**[CORRECTED at the phase-NN IMPL: …]**` for a clause **within the same ADR family**. ⚠️ **The discriminator is the ADR FAMILY, NOT the phase gap** — V2 falsified the naive reading: `:17209` is a **phase-73** correction sitting inside **ADR-0294** (phase 72's ADR) and it uses the **INLINE** form. So "a later phase corrects an earlier ADR" does **not** by itself imply the blockquote. ⇒ **Phase 74 correcting ADR-0286 is a correction ACROSS ADR FAMILIES (an Observability stats row annotating an xDS/SDS row) ⇒ copy `:16901`'s blockquote.** | T9 |
| **RD-ADR0296** | SPEC §14: the ADR-0296 block is §Context-only | **CONFIRMED.** Block = **`:17254`–`:17280` (EOF)**. Heading `:17254`; STATUS blockquote `:17256`; **exactly ONE** `### Context (drafted at the phase-74 SPEC)` at `:17258`; nine body paragraphs `:17260`–`:17276`; *The design (§Decision anticipated)* `:17278`; the italic footer `*(§Decision + §Consequences land at the phase-74 IMPL.)*` at **`:17280`, the LAST line of the file**. **`### Decision` ⇒ 0, `### Consequences` ⇒ 0.** Tail confirmed ADR-0296; `grep -c '^## ADR-0297'` ⇒ 0. Mirror **ADR-0295** (`:17211`/`:17213`/`:17215`, footer `:17229` RETAINED, then `### Decision (landed at the phase-73 IMPL)` `:17231` + `### Consequences (…)` `:17243`). | T9 |
| **RD-ADR0296-POINTER** ⚠️ | Not in the SPEC | **A LIVE INTERNAL MIS-POINTER inside ADR-0296.** Its STATUS blockquote (`:17256`) says *"see §Context ¶6, which records that this citation is itself a misattribution"* — but the ADR-0044 misattribution is at **¶4(i) (`:17266`)**; **¶6 (`:17270`) is the classifier-fragility paragraph.** ⇒ fix at T9 while completing the ADR. | T9 |
| **RD-GREP0** ⚠️ | SPEC §14 / ADR-0296 ¶3: `grep -c 'VerifyPeerCertificate\|handshake-error callback' docs/envoy-go/DECISIONS.md` ⇒ **0** | **REFUTED — IT IS NOW `1`, AND THE SPEC'S OWN COMMIT CAUSED IT.** The single hit is **`DECISIONS.md:17264`** — ADR-0296 §Context ¶3, *the paragraph that asserts the count is 0*. The §Context append made its own grep self-refuting. **The finding's SUBSTANCE still holds** (the phrase is not in ADR-0286, and the only occurrence in the file is the quotation of it). ⚠️ **The IMPL must NOT read `1` as drift.** ⇒ **state the rule with NO number, or scope the grep to ADR-0286's line range** (`awk 'NR>=16888 && NR<16930' docs/envoy-go/DECISIONS.md \| grep -c 'VerifyPeerCertificate\|handshake-error callback'` ⇒ 0). T9 fixes ¶3's wording. | T9 |
| **RD-ADR0044** | SPEC §1.1 D1: ADR-0044 does not contain the §Context-draft discipline | **CONFIRMED.** `:1419 ## ADR-0044: BEHAVIOR_CONTRACT HTTP/1.1 subsection`; its `### Context` (`:1426`) and `### Decision` (`:1430`) are entirely about which HTTP/1.1 equivalence dimensions the differential asserts and six header allow-list rows. **ZERO §Context-draft / append-at-IMPL language.** ⇒ **the "ADR-0044-as-used" hedge is WARRANTED; use it throughout.** ADR-0045 `:1466` (the >25-task / >1500-LoC split valve) · ADR-0106 `:4788` (flat top-level family rows) · ADR-0117 `:5424` (incl. `NewCounterIfAbsent` post-Freeze at `:5444` item 2) · ADR-0288 `:16981`. | T9 |
| **RD-ROADMAP** | SPEC §12: row 74 `:136`; the three live sentences `:184`/`:194`/`:204`; the family heading `:200`, paragraph `:202` | **ROW + SENTENCES CONFIRMED; THE EDIT SURFACE CORRECTED.** Row 74 is `:136`, **8 pipe-delimited fields**, status field **`in-progress`**. `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` ⇒ **3**, at `:184`/`:194`/`:204`. `:200` is `### Observability family`; **`:202` is the family's 92-char one-line DESCRIPTOR — the `candidates:` sentence and the phase-74 charter BOTH live on `:204`, the 42,342-char family-history line.** ⇒ **the narrow's edit surface is `:204`, not `:202`.** §Schema: status values `:12`; *"Families (§9 headings) are not rows"* **`:21`**. File = **220** lines. | T9 |
| **RD-ROADMAP-FOUR** ⚠️ | Not in the SPEC's §12 | **THE SURVIVING PARENTHETICAL IS WRONG BY TWO NAMES.** Both the row-74 cell (`:136`) and the `:204` deferred sentence say *"three of five sub-names roll out, `ciphers`/`versions` SURVIVE"*. But **SPEC §2/§3.6 and ADR-0296 ¶7 say the deferred family is FOUR** — `ciphers` / `versions` / **`curves`** / **`sigalgs`** (the latter two *"were never named in any prior document"*). ⇒ **the T9 narrow must ADD `curves`/`sigalgs`, not merely delete three names**, and the row-74 cell's "five/two" framing must be corrected in the same edit. | T9 |
| **RD-STATE** | SPEC §15: the ADR-0288 singleton greps return 2, not 1 | **CONFIRMED — 2 each, and `STATE.md:7` is the FIRST hit (the RULE STATEMENT), not the second.** `next-skill:` `:7`,`:18` · `lifecycle-state:` `:7`,`:17` · `next-free ADR:` `:7`,`:21`. **NEVER "fix" the count to 1 — that would delete the rule.** ADR-0288's rule text: `DECISIONS.md:17006` (five-stage lineage cap) + `:17008` (*"A stage close EDITS §Current pointer IN PLACE and never prepends"*). | PLAN close |
| **RD-BASELINE** | SPEC §15 counts | **ALL CONFIRMED at tip, controller-run:** fixtures **119** · fuzzers **55** · BackendKind tail **38** (`fixture.go:614 H2GoawayResponder BackendKind = 38`) · DECISIONS tail **ADR-0296** PROPOSED · stat surface **1201** (`:831`/`:847`; no mechanical command) · `go build ./...` **clean** · `go test ./internal/listener/ ./internal/stats/ -count=1` ⇒ **ok 3.200s / ok 0.003s**. | T8 |

### 1.2 Findings this re-derivation surfaced that the SPEC does NOT carry

**Every finding below is itself a claim** (`reference_a_drift_correction_is_itself_a_claim`) — re-run the grep before any of them becomes an `old_string`.

- **F1 (SEVERE) — `internal/listener/`'s test corpus contains NO mTLS PKI, so two tasks cannot be written the way the SPEC's spine implies.** `grep -n 'ValidationContext\|TrustedCa\|trusted_ca' internal/listener/*_test.go` ⇒ **ZERO hits**. The available material is `testCAPEM` (`manager_test.go:564` — **the CA CERTIFICATE ONLY; its private key is not in the repo**) plus `testAlphaCertPEM`/`testAlphaKeyPEM` (`:576`/`:589`) and `testBetaCertPEM`/`testBetaKeyPEM` (`:596`/`:609`) — all **SERVER leafs**. There is **no client certificate and no foreign CA**. ⇒ **T1's live-handshake no-cert case and T4's three increment tests require a NEW runtime PKI generator in the test file.** Copyable patterns: `internal/tls/config_test.go:42-80` (`ecdsa.GenerateKey` → `x509.CreateCertificate` self-signed CA → leaf) and `test/fixtures/0111-…/driver/driver.go:193-260` (`mustCA` / `mustLeaf` / `mustClientCert`). ⚠️ **This adds TEST-side imports** (`crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509/pkix`, `encoding/pem`, `math/big`) — **the SPEC's "+0 imports" is a PRODUCTION claim and it SURVIVES intact; say so explicitly at T8 so the envelope audit does not read a test import as a violation.** ⇒ **T1, T4, T8.**
- **F2 (SEVERE) — `mkDownstreamTSRequireClientCert` (`manager_test.go:644`) is NOT reusable for the increment tests, and its own doc says so.** Its comment reads *"builds a transport socket whose require_client_certificate is true **(should be rejected at build time)**"* — it carries `RequireClientCertificate: wrapperspb.Bool(true)` with **NO `validation_context.trusted_ca`**, which is precisely the phase-67/ADR-0289 boot-reject (`tls: downstream: require_client_certificate=true requires validation_context.trusted_ca`, `BEHAVIOR_CONTRACT.md:962`). **A test reusing it would fail at `NewManager`, not at the handshake.** ⇒ a NEW `mkDownstreamTSMutualTLS(t, certPEM, keyPEM, caPEM)` helper is required. ⇒ **T4.**
- **F3 (SEVERE) — a nil `*stats.Counter`.Inc() PANICS, `serveConnection` runs in a GOROUTINE, and there is NO `recover()` anywhere in `internal/listener/`.** `internal/stats/counter.go:22`: `func (c *Counter) Inc() { c.v.Add(1) }` — **no nil guard**; `grep -n 'recover()' internal/listener/*.go` ⇒ **ZERO**; `manager.go:1081 go rt.serveConnection(ctx, raw)`. **THREE consequences the PLAN must carry:** (i) **Break C's observable is a PROCESS CRASH of the whole test binary, not a clean assertion failure** — that still counts as firing, but T4 must state it in advance and require the panic stack to name the ssl Inc (`reference_deliberate_break_wrong_assertion` applies to panics too); (ii) the registration gate (`rt.tlsMode`) and the Inc guard (`selected.tlsCfg != nil`) **MUST remain equivalent**, which they are by RD-MIXED — and because the failure mode of breaking that equivalence is *a production crash*, not a wrong count, **it earns its own named test** (T2 test 5); (iii) do **not** "defensively" add nil guards to the Inc sites — that would make Break C vacuous and mask the very invariant being tested. ⇒ **T2, T4.**
- **F4 (MODERATE) — a FOURTH stale-comment site the SPEC's D6 roster misses: `helpText`'s own doc comment.** `name.go:445-451` reads *"The **11 entries** cover the 13 unique Prometheus names emitted by 06.1 … plus one 06.2 backpressure counter."* The map has exactly 11 entries; +3 makes **14**. ⇒ **T5** must update the doc comment, not only the map.
- **F5 (MODERATE) — the D7 rename target is `:1911-1927`, not `:1911-1913`.** `TestListenerManager_AllocatesTwoMetricsPerListener`'s doc comment runs `:1911` through `:1927` (it carries a multi-paragraph PLAN-deviation note). Renaming the test while leaving `:1914-1927` describing "exactly the 2" would leave the same defect one paragraph down. ⇒ **T2.**
- **F6 (MODERATE, SIMPLIFYING) — `AssertStats` receives ONLY the two admin addresses, and Shape A needs nothing else.** There is **no `fixture.StatsArgs`** anywhere in the repo; **listener addresses are NOT passed.** The `0005` precedent stashes them itself (`driver.go:234-242`, under a mutex). `edfDriver` (`driver.go:116-130`) has **no stash field and no mutex beyond `sync.Once`**. ⇒ **Shape A needs NO new driver field, NO mutex and NO change to `driveSide`** — an additional, previously-unrecorded argument for Shape A over Shape B. ⇒ **T6.**
- **F7 (MODERATE) — SPEC §8.2's *"there is NO shared scraper … the Prometheus precedent is `0005`"* UNDERSTATES the precedent, and a sanctioned fallback exists.** At least **28** fixtures build `"http://" + adminAddr + "/stats/prometheus"` inside or beneath `AssertStats` (0005, 0011, 0013, 0014, 0016–0023, 0026, 0030, 0032, 0034, 0036, 0038, 0043, 0046, 0048, 0049, 0051, 0052, 0053, 0055, …). ⚠️ **FOUR of them deliberately discard the reference addr** — `0030-http-admission-control/inputs/driver.go:290`, `0032-http-ratelimit/inputs/driver.go:827`, `0086/driver/driver.go:321` and `0098/driver/driver.go:511`, all `AssertStats(t fixture.TB, _ /*refAdminAddr*/, subjAdminAddr string)`. **That is the sanctioned SUBJECT-ONLY shape and is the documented fallback if the cross-side leg proves infeasible at IMPL time** — but it is a LAST resort: it is strictly weaker (C3's own correction says so) and T6 must report the downgrade explicitly rather than take it silently.
- **F8 (MINOR, CONFIRMING) — `0111`'s reference `envoy.yaml` has NO listener-level `stat_prefix`.** Its only `stat_prefix` is the HCM's `ingress_edf` at `:101`. ⇒ the reference keys the listener scope by ADDRESS (`listener.0.0.0.0_10447.*`) — which is exactly why the label-agnostic Prometheus keying is required, and it means **SPEC §3.1's *"the `0111` fixture must NOT set `stat_prefix`"* is already satisfied by construction: NO edit is owed.** ⇒ **T6.**
- **F9 (MINOR) — the `0055` ABSENT-check precedent uses `continue`, and that is the load-bearing detail.** `0055/driver.go:655-669`: on `!refOK` / `!subjOK` it `t.Errorf(… ABSENT …)` **and `continue`s**, so an absent name never reaches the value comparison and can never produce a bogus `0 == 0` pass. Note the contrast: **`0005`'s parser defaults absent names to a zero-valued `Snapshot` field** — the exact vacuity SPEC §8.2 warns about. ⇒ **T6 must copy `0055`'s map-plus-`continue` shape, NOT `0005`'s struct-snapshot shape**, even though the scrape URL comes from `0005`.
- **F10 (INFORMATIONAL) — "BackendKind tail 38" is a TAIL VALUE, not a count.** `fixture.go:614 H2GoawayResponder BackendKind = 38`; the file declares **39** constants (values 0–38, `TCPEcho = 0` at `:137`). The lineage figure **38** is correct as written. Recorded so no future session "fixes" it to 39.

### 1.3 What is still NOT verified — the IMPL inherits no false confidence

Per SPEC §1.2 and `reference_quoting_is_not_executing`:

- **NO build of row 74 exists.** This PLAN did **not** compile the classifier, the fields, the gate, the guard or the asserter. Its `[RUN]` evidence is limited to: the baseline build + `internal/listener`/`internal/stats` test runs, the collision greps, the parallel-stream diff, the `crypto/tls` symbol/string enumeration at `go1.26.5`, and the file/line/call-graph re-derivations above. **Everything about buildability is still the IMPL's job.**
- **Nothing reference-side was executed at this PLAN** — no Docker, no Envoy probes. Every reference claim rests on the SPEC's own ~35-arm probe record.
- **The `0111` `StatsAsserter` has still never been written or run.** Its shape is specified twice now; it is demonstrated zero times.
- **Breaks A–G are all untested.** Break D in particular is untested *by construction* — its target test does not exist yet.
- The SPEC's own unresolved list carries forward unchanged: session resumption, the undriven `fail_verify_san`/`fail_verify_cert_hash`/`was_key_usage_invalid`/`ocsp_staple_*`, `ssl.connection_error`'s full membership, whether the reference keys `ssl.*` off the listener or the filter chain, and the multi-`tls_certificate` `e3b0c442` name-collision risk.

### 1.4 Adversarial-pass record

*(This section is written AFTER the adversarial pass, from what the verifiers actually found — **never asserted in advance over a placeholder**. That failure mode is the phase-69 cited-but-unwritten class, which phase 72's V2 caught being self-certified against. ⚠️ **An earlier draft of THIS document deleted that sentence and then committed the exact failure mode it names** — it shipped `STATUS: COMPLETE` citing a `PROGRESS.md` that did not yet exist. V2 caught it as its own SEVERE-1. The guardrail is restored here verbatim.)*

**STATUS: COMPLETE** — populated from two verifiers' ACTUAL reports. Both ran in PRIVATE scratch (`reference_parallel_subagents_private_scratch`) against this branch's tip, on disjoint remits.

**V1 (code claims, BY EXECUTION — built T1→T4 from this document's skeletons pasted VERBATIM into a private repo copy and ran every break): 2 SEVERE, 2 MODERATE, 4 MINOR.** **V2 (process, consistency, SPEC-coverage, stage-close mechanics — re-ran ~120 anchors): 1 SEVERE, 5 MAJOR, 11 MODERATE, 10 MINOR.**

⚠️ **V2's headline verdict on the ledger, recorded because it cuts both ways:** *"every one of the PLAN's twelve claimed corrections of the SPEC holds"* — `errors` really is at `:7`, `checkName` really panics at `:117`, the SN3 impl really is `:83-92`, `quic_test.go` really pins only the counter half, the `1200`/`1201` counts are exactly 13 and 2, the ctx bound really is `Pipeline.Run`'s single return, and RD-QUICSTALE is correct **and exhaustive within `SPEC.md`**. *"That earned trust is what makes S1 disqualifying."*

**The SIX that changed this document's instructions:**

- **V1-S1 (SEVERE) — `net.Pipe()` DEADLOCKS; T1's live-handshake helper could not work as written.** The no-cert arm hangs forever: `net.Pipe` is synchronous and unbuffered, so the server blocks writing alert 116 from `processCertsFromClient` (`handshake_server.go:982`) while the client is still blocked mid-`Write` on the remainder of its second flight — neither side is reading. V1 hit a 45-second timeout with **no failing assertion to read**, and confirmed it is structural, not fixable by adding a post-handshake `Read`. ⇒ **T1 Step 1 now mandates a loopback-TCP `connPair`; with that substitution every T1 claim passes in 0.008s.**
- **V1-S2 (SEVERE) — Break F **DOES** fire at the scope this document originally gave it**, i.e. the one break declared "must not fire" would have tripped the stage-stopping protocol on a false alarm. `TestClassifyHandshakeErr_TLS12` carries its **own** live no-cert arm which Break F never converted, so mutating the const turned it red. ⇒ **Break F's scope now covers BOTH live no-cert arms**, and its "mutate in both places" sub-step is corrected (once the table references the const by name there is exactly ONE place).
- **V1-M2 (MODERATE) — `TestListenerMetrics_GateMatchesInc` could not detect the failure it exists to prevent.** Its two predicates are both set at **build time** (`:639` / `:510`), entirely upstream of `registerListenerMetrics`; under Break E the literal version **stayed GREEN**. V1 added three lines asserting the *counter pointers* and it then fired. ⇒ **legs (b)/(c) now assert the `*stats.Counter` fields themselves.**
- **V2-M1 (MAJOR) — T7's own verification gate was unsatisfiable by T7's own instruction.** Step 3 mandates a ledger line reading `Phase 74 — 1201 → 1204`; Step 5 and T8 Step 5 then gated on `grep -n '1201' … ⇒ 0 hits`. The mandated line **contains** `1201`. ⇒ **both gates rewritten to expect exactly 1 hit (the new ledger line), with a scoped `awk` form for the pre-ledger region.**
- **V2-M2 (MAJOR) — the FOURTH stale QUIC site, and it is in the very cell T9 opens.** `ROADMAP.md:136` still reads *"leaving QUIC handshakes uncounted is a **DEPARTURE that must be written down**, not merely omitted"* — the pre-probe reading SPEC §3.4 refuted. The same cell also carries *"CROSS ADRs — a first for the project"* (refuted by SPEC §1.1 D2's three precedents) and is **already self-inconsistent** about the split family (`ciphers`/`versions`/`curves` in one clause, *"`ciphers`/`versions` SURVIVE"* two clauses later). ⇒ **T9 Step 4 now names all four stale claims in `:136`, not one.** *(V2 also confirmed the sweep is otherwise exhaustive: no fourth stale site in `SPEC.md`, and **`STATE.md` is CLEAN** — its `:17` already carries "the gate is `rt.tlsMode` ALONE, no kind check".)*
- **V2-M3 (MAJOR) — Break H was internally contradictory and would have killed the fixture before `AssertStats` ever ran.** `mtlsEcho` (`0111/driver.go:494`) has **no `side` parameter** — both sides route through `driveSide` — so the break is symmetric and the cross-side assertion **cannot** fire; and `clientCertMode` has only `sendForced`/`sendNone`, so deleting the forced-send strips the **trusted** arm too, failing `structuralCheck` (`:441`) ⇒ the runner `t.Fatalf`s in the **drive** and step 10 never executes. ⇒ **Break H rewritten as a new `sendPolite` mode applied to the untrusted arm on ONE side only, flagged `[NOT pre-compiled]`, with a corrected firing set.**

**Also folded:** V2-M4 (T6's cross-side comparison is ENTAILED by the two absolute checks and can never fire alone — stated as a redundant tripwire rather than an independent property) · V2-M5 (the placeholder count was **3**; it is **11** — corrected in the Self-review) · V1-M1 (Break F does not compile as worded: `declared and not used: liveNoCertErr`; the substitution is now named in advance) · V2-Mo1 (**"SPEC §3.10" does not exist** — SPEC runs §3.1–§3.7; all three citations repointed) · V1-m2/V2-Mo2 (Break A fires **five** rows, not four — a count mismatch is exactly what a break-record audit trips on) · V2-Mo3 (E′ and C′ were missing from the Self-review's break map; the "extras" count was 3, actually 5) · V1-m3 (`counterValue` must `t.Errorf` on an absent name or T3 assertion (4) passes vacuously under Break D) · V2-Mo4 (T5's test file was a TBD in disguise — pinned to `internal/stats/name_test.go:217`) · V2-Mo5 (T3's copy-source is `quic_test.go:121-126`, not `:106`) · V2-Mo6 (`sort` and `reflect` are missing from BOTH the test-import roster and the actual files) · V2-Mo7 (the identifier roster missed six names, one with a live in-file clash: `manager_test.go:559` already has a doc comment naming a **different** `testPKI`, and a naive `counterValue` re-run returns **120** repo-wide and would read as a collision) · V2-Mo8 (`ssl.no_certificate`, the naming trap SPEC §13.1 says is recorded in ADR-0296 §Context, is **in neither the ADR nor any B-step** — `awk 'NR>=17254&&NR<=17280' | grep -c 'no_certificate'` ⇒ **0**) · V2-Mo9 (the Self-review's SPEC walk skipped §1/§2/§13) · V2-Mo10 (five memories owed a citation) · V2-Mo11 (the File-structure block was missing four files the tasks edit) · V1-m1 (the T2 field block was not gofmt-clean — this document's own Step-5 gate failed on its own paste) · V2-m1 (ADR-0044's headings are `:1426`/`:1430`; `:1427`/`:1431` are blank lines) · **V2-m2 (RD-CORRFORM's arrow is falsified by its own evidence: `:17209` IS a later-phase-corrects-earlier-ADR case using the INLINE form — the discriminator is the ADR FAMILY, not the phase gap)** · V2-m3 (RD-HELPTEXT's "zero `ssl` substring" is self-refuting — 3 hits, all inside `acce**ssl**og_dropped`; the same class RD-GREP0 flags) · V2-m4/m5 (`:973` for the QUIC `continue`; one anchor spelled two ways) · V2-m6 (**four** subject-only `AssertStats` shapes, not two) · V2-m7 · V2-m8 (check (1)'s blind spot was dropped from the lineage) · V2-m9 (nested backticks would trigger command substitution in `git commit -m`) · V2-m10.

**Two findings ACCEPTED AS-IS, with the reasoning recorded rather than the instruction changed:**
- **V1-m4 — Deviation #2 is a STYLE call, not a forced one.** V1 renamed all four members to the SPEC §3.5 table's bare `ok`/`verifyError`/`noCert`/`other` and got `go vet` rc=0, `golangci-lint` rc=0, tests `ok 0.166s`. **A package-level `ok` is legally shadowed by every `v, ok := m[k]` and nothing breaks.** The deviation stands on readability grounds only, and Deviation #2 now says so explicitly so no reader mistakes it for a constraint.
- **V2-M4's cross-side leg is retained** even though entailed — a redundant tripwire costs three lines and survives a future refactor to per-side `want` values.

⚠️ **NOT verified — the IMPL must not inherit false confidence.** V1 ran **nothing reference-side** (no Docker, no Envoy probes): every reference/parity claim still rests on the SPEC's own probe record, and V1 verified only that *envoy-go* behaves that way. **T5–T9 were not executed**; the `0111` `AssertStats` **still has never been written or run**, so F6/F7/F8/F9 and Breaks G/G′/H remain read-derived. V1 also worked in sibling `phase74_t*_test.go` files rather than in `manager_test.go` proper, so **the T2 rename and the `:1911-1927` doc rewrite were never exercised in place**, and it never established whether `net.Pipe` also deadlocks in the *untrusted* arm (the no-cert arm hangs first).

---

## Global Constraints

- **ONE stage per session.** This session: the PLAN only. No production `.go`. After it lands: roll to the phase-74 IMPL.
- **⚠️ THIS PLAN's OWN stage-close checklist** *(distinct from T9, which closes the IMPL — every task T1–T9 below is an IMPL task)*. At the PLAN close the controller: creates `PLAN.md` + `PROGRESS.md` (the ONLY two files in the phase-directory delta) · rolls **STATE §Current IN PLACE** (lifecycle 2 → 3; §Recent re-capped at FIVE **with its preamble updated** — the ADR-0288 rule; ⚠️ the three singleton greps return **2**, never "fix" to 1) · rolls `next-prompt.txt` to the phase-74 IMPL · **runs all three sentinel checks MECHANICALLY TWICE** (worktree + landed master post-push) · squash-pushes. **ROADMAP and DECISIONS are UNTOUCHED at a PLAN; row 74 STAYS `in-progress`.**
- **⚠️ THE REGISTRATION GATE IS `rt.tlsMode` ALONE — NO KIND CHECK.** This is the single instruction most likely to be got wrong, because **SPEC `:1`, `:315` and `:320` still carry the refuted first reading** (RD-QUICSTALE). §3.4, §16 and ADR-0296 §Context ¶8(ii) (`DECISIONS.md:17274`) are authoritative: the reference registers the full `ssl.*` family on a QUIC listener at boot and never moves it, so **envoy-go doing the same is EXACT PARITY and gating QUIC out would be the DEPARTURE.** It is provably sufficient because `startQUIC` hard-errors without a TLS config (`quic.go:33-36`) ⇒ every QUIC listener that boots has `tlsMode == true`. **Never write the gate as "has a TCP-style TLS transport socket" in any form that excludes `QuicDownstreamTransport`.**
- **ONE functionally-edited production file for the row's core, TWO in total:** `internal/listener/manager.go` (the 3 fields + the gated registration + the classifier + the 2 Inc points + 2 stale comments) and `internal/stats/name.go` (`helpText` ×3 + its doc comment). **ZERO new packages. ZERO new exported symbols** — `classifyHandshakeErr`, `handshakeOutcome`, `verifyError`, `noCert`, `ok`, `other` are all unexported.
- **⚠️ ZERO NEW PRODUCTION IMPORTS.** `stdtls "crypto/tls"` (`manager.go:5`) and `errors` (`manager.go:7`) are already present; `internal/listener` already imports `internal/stats` (`:30`) ⇒ **no new layering edge**. **+0 go.mod modules** — verify `go mod tidy -diff` EMPTY and `go.mod`/`go.sum` no-diff at T8. ⚠️ **TEST-side imports DO grow** (F1): the new runtime PKI generator needs `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509/pkix`, `encoding/pem`, `math/big`. **That is not a violation** — the envelope claim is about production. T8 must audit the two categories SEPARATELY.
- **BYTE-UNTOUCHED, sha256-asserted at T8:** `internal/listener/quic.go` · `internal/listener/listenerfilter/**` · `internal/tls/**` · `internal/xds/**` · `internal/boot/**` · `internal/bootstrap/**` · `internal/tracing/**` · `internal/cluster/**` · `internal/filter/**` · `validate/**` · `cmd/**` · `go.mod` · `go.sum` · `test/fixtures/0111-…/envoy-go.yaml`. ⚠️ **`internal/stats` is NOT on this list** (SPEC §7 corrects BRAINSTORM §10) — `name.go` is edited at T5.
- **⚠️ `internal/listener/manager_test.go` is EDITED, not gated.** T2 and T4 both add tests to it and T2 renames a test inside it. **Do NOT write a sha256 gate on it** — that is the `reference_plan_schedules_edits_to_a_byte_gated_file` defect, which phase 73 hit as its own SEVERE-1. Gate it on SHAPE instead (T8: the pre-existing test names all still present, `go test` green).
- **The stat delta is +3 as a NAME SURFACE, not per deployment** (SPEC §3.3): a plaintext-only deployment gains **ZERO** names; a QUIC listener gains all three and they stay permanently **ZERO** — parity, not an anomaly. **1201 → 1204** in the BEHAVIOR_CONTRACT ledger.
- **`other` INCREMENTS NOTHING, and that is a NAMED DEPARTURE**, not a neutral omission — the reference books non-cert handshake failures under `ssl.connection_error`. Record it (B5); do not quietly add a fourth counter.
- **`ssl.fail_verify_error` means *"certificate CHAIN VERIFICATION failed"*, NOT *"client cert rejected"*.** A cert/private-key mismatch and a malformed DER never reach `certs[0].Verify()` and land in `other`. **The wording is load-bearing** — B6 uses the exact phrase.
- **The classifier's accuracy is CONDITIONAL, and the bound must be restated correctly** (RD-CTXBOUND): *"no production deadline REACHES `HandshakeContext`"* — guarded by `Pipeline.Run`'s single-return signature and by `serveConnection` never rebinding `ctx`. **NOT** *"there are no production deadlines in `internal/listener/`"*, which is false. Any future row adding a handshake deadline re-opens it.
- **Per-task hygiene** (`feedback_pertask_gofmt_lint`): `gofmt -l` + `go vet` + `golangci-lint run` on every touched package, per task — not just at T8.
- **Worktree discipline** (`feedback_git_worktrees` / `feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting` / **`reference_bash_cwd_reset_commits_to_main`**): pin the canonical root; use **`git -C <abs-worktree-path>`** for every git command; tripwire with `pwd` + `git rev-parse --abbrev-ref HEAD` (must be the stage branch, **NEVER `master`**) + `git rev-list --count <base>..HEAD` before any commit or gate run. A `cd` outside the repo silently resets the Bash cwd.
- **Subagents commit LOCALLY only** (`feedback_subagents_no_push`); the controller squash-pushes at stage close (`feedback_push_to_origin`). Locate commits by SUBJECT (`git log --grep 'phase 74'`), never by position.
- **Known PRE-EXISTING flakes — never reflex-classify as phase-74 regressions:** the `internal/cluster` `-race` outlier flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce`, `internal/cluster/outlier_test.go:766` — **isolate-re-run, do NOT re-classify**, `reference_cluster_race_outlier_flake`) · the full-suite startup flake (`subject ready: EOF` on an UNRELATED fixture) · the SDS `init_fetch_timeout` dial-budget flake · and TWO still-UNINDEXED load flakes, isolate-green and not root-caused: `internal/httpclient TestOptions_ZeroValue_NoOpDefaults` and `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`.
- **ADR-0045's escape valve is armable-but-unconsumed.** Nine tasks, one core production file; the one thing that could have forced a split (a posture requiring the QUIC runtime) does not, because §3.4 needs **no kind check at all**. If the IMPL trips the >25-task / >1500-LoC gate, STOP and split — do not defer via TODO/stub tasks (ADR-0045 §6.3).

### Break protocol (binding on every task)

- **A break must COMPILE** (`reference_plan_break_instructions_dont_compile`). Breaks flagged `[NOT pre-compiled — substitution rule applies]`: at IMPL time, if it does not compile, **substitute a compiling equivalent, REPORT the substitution, and record the TRUE result** — and remember that **a substitution's RATIONALE is itself a claim** (phase 72 shipped a false one).
- **A break must DISCRIMINATE** (`reference_probe_must_discriminate`): before recording it as proof, ask what the OTHER hypothesis would have printed. Run the cross-product where one is available (Break B).
- **Breaks run AFTER committing** (`reference_break_protocol_commit_first`) — `git restore` wipes uncommitted work; being killed mid-break leaves a BROKEN tree.
- **`-count=1` on EVERY differential break** (`reference_differential_break_protocol_count1`); caching serves a stale PASS.
- **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`) — and whether a second property's firing is ENTAILED by the first. ⚠️ **This applies to PANICS too** (F3): a Break-C process crash must be confirmed to originate at the ssl `Inc`, from its stack.
- **A break that does NOT fire is a FINDING** — record it honestly in `PROGRESS.md`; do not route around it. **Break F is the one break that MUST NOT fire** (it proves the live-handshake construction is load-bearing); a Break F that DOES fire is equally a finding.
- **Full selector only:** `-run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback'` — never bare `0111` (`reference_differential_run_selector`).
- **`Errorf` per independent property; `Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`).

### Identifier roster (`reference_spec_drafted_identifier_collision_check`)

**Verified FREE at `ab13fc19` (repo-wide `--include='*.go'`, all 0 hits — RD-IDENT):** `classifyHandshakeErr` · `handshakeOutcome` · `sslHandshake` · `sslFailVerifyError` · `sslFailVerifyNoCert` · word-boundary `verifyError` · word-boundary `noCert`. ⚠️ **Use `grep -rnE '(^|[^A-Za-z_])verifyError([^A-Za-z0-9_]|$)'` for the last two** — a naive substring grep is not a collision check. **Re-run all seven at the IMPL tip** (T1 Step 0).

New identifiers this row also introduces, collision-checked at the same time: the `handshakeOutcome` members **`outcomeOK`**, **`outcomeVerifyError`**, **`outcomeNoCert`**, **`outcomeOther`** (⚠️ **named `outcome*` rather than the SPEC §3.5 table's bare `ok`/`verifyError`/`noCert`/`other`**, because a package-level `ok` in `package listener` would shadow the universally-used `v, ok := m[k]` idiom at every site in the file — a readability hazard the SPEC's table does not consider; the SPEC's names are a *table*, not an identifier pin). **Test-side identifiers, ALSO collision-checked (V2-Mo7 — an earlier draft's roster missed all six):** `mkDownstreamTSMutualTLS` **0** · `driveH3` **0** · `noClientCertErrText` **0** · `listenerSSLNames` **0** · `connPair` (check at IMPL tip) · ⚠️ **`testPKI` — repo-wide 5, but ALL in `internal/tls/config_test.go` (a DIFFERENT package), so package-safe. HOWEVER `manager_test.go:559` already carries a doc comment naming a *different* `testPKI`** (*"testPKI holds the inline PEM bytes loaded from test/fixtures/0002-tls-tcp/pki/"*) — declaring `type testPKI struct` in the same file 340 lines below it is a readability trap. **Rename this row's type to `handshakeTestPKI`** (grep-checked 0) and leave that comment alone. ⚠️ **`counterValue` — repo-wide 120, but ZERO in `internal/listener`**, so it is safe; **a naive "expect 0" re-run at the IMPL tip returns 120 and MUST NOT be read as a collision** (scope the grep to `internal/listener/`).

---

## File structure

```
internal/listener/manager.go                  [EDIT]  T1 (handshakeOutcome + classifyHandshakeErr; no import)
                                              [EDIT]  T2 (+3 *stats.Counter fields at :175-176; the rt.tlsMode-gated
                                                          registration inside registerListenerMetrics :351-355;
                                                          the stale comments :172-174 and :345)
                                              [EDIT]  T4 (the classify+Inc at :1179 and the success Inc at :1183 —
                                                          BOTH inside the existing `if selected.tlsCfg != nil` block)
internal/listener/manager_test.go             [EDIT]  T1 (the classifier table + the LIVE-handshake no-cert case + mkTestPKI)
                                              [EDIT]  T2 (the NAME-SET registration guard; plaintext adds ZERO;
                                                          the gate-equivalence test; the D7 rename incl. :1911-1927)
                                              [EDIT]  T4 (mkDownstreamTSMutualTLS + the 3 increment tests + the
                                                          plaintext no-Inc test)
                                              ⚠️ EDITED, NOT sha256-gated. Gate on SHAPE at T8.
internal/listener/quic_test.go                [EDIT]  T3 (the QUIC registration test: all THREE names present at
                                                          value ZERO across a completed H3 handshake)
internal/stats/name.go                        [EDIT]  T5 (helpText ×3 at :452-464 + the ":445-451 11 entries" doc — F4)
internal/stats/name_test.go                   [EDIT]  T5 (the three HELP-entry assertions; clone the :217-218 shape)
test/fixtures/0111-…/driver/driver.go         [EDIT]  T6 (AssertStats + `var _ fixture.StatsAsserter = (*edfDriver)(nil)`
                                                          + a 0055-shaped scraper; the :72-82 RD3 revision)
test/fixtures/0111-…/README.md                [EDIT]  T6 (:100-110 RD3 · :162-165 the ssl.* bullet — KEEP the /listeners guard)
test/fixtures/0111-…/expectations.yaml        [EDIT]  T6 (:109-119 RD3 · :133-153 ADD leg (c) · :177-184 · :189)
test/fixtures/0111-…/envoy.yaml               [EDIT]  T6 (:24-25 — the comment only; NO config change, NO stat_prefix)
docs/envoy-go/BEHAVIOR_CONTRACT.md            [EDIT]  T7 (B1-B8; :831/:847 → 1204; a ledger line after :4988)
docs/envoy-go/DECISIONS.md                    [EDIT]  T9 (ADR-0296 completed IN PLACE after the :17280 footer;
                                                          the :17256 ¶6→¶4(i) mis-pointer; ¶3's self-refuting grep;
                                                          the ADR-0286 C3 indented blockquote after :16908)
docs/envoy-go/ROADMAP.md                      [EDIT]  T9 (row 74 :136 → done + its FOUR stale claims; the :204 narrow ADDING curves/sigalgs)
docs/envoy-go/STATE.md                        [EDIT]  T9 (§Current rolled IN PLACE, lifecycle 3 → DONE; §Recent re-capped at five)
next-prompt.txt                               [EDIT]  T9 (rolled to the phase-75 BRAINSTORM; TRACKED despite .gitignore)
docs/envoy-go/phases/74-…/PROGRESS.md         [EDIT]  T1-T9 (the live task ledger; every task appends its actual result)
internal/listener/quic.go · internal/listener/listenerfilter/** · internal/tls/** · internal/xds/** ·
internal/boot/** · internal/bootstrap/** · internal/tracing/** · internal/cluster/** · internal/filter/** ·
validate/** · cmd/** · go.mod · go.sum · test/fixtures/0111-…/envoy-go.yaml   [BYTE-UNTOUCHED — sha256 at T8]
```

---

## Task 1 — the classifier: `handshakeOutcome` + `classifyHandshakeErr`, with a LIVE-handshake-constructed no-cert case

**Files:**
- Modify: `internal/listener/manager.go` (append the type + function near `registerListenerMetrics`, or immediately above `serveConnection` — pick one and keep it; **no import change**)
- Test: `internal/listener/manager_test.go` (the table + `mkTestPKI`)

**Interfaces:**
- Produces: `type handshakeOutcome int` with `outcomeOK` / `outcomeVerifyError` / `outcomeNoCert` / `outcomeOther`, and `func classifyHandshakeErr(err error) handshakeOutcome`. Both **unexported**. Consumed by T4's Inc points.
- Produces (test): `func mkTestPKI(t *testing.T) testPKI` — a fresh in-process ECDSA P-256 PKI: a self-signed CA (`caCertPEM`, `caKeyPEM`), a server leaf signed by it, a client leaf signed by it (**trusted**), and a SECOND self-signed foreign CA plus a client leaf signed by *that* (**untrusted**). Consumed by T4.
- Consumes: `errors.As` (`errors` already imported at `:7`) and `stdtls.CertificateVerificationError` (`stdtls` at `:5`). **ZERO new production imports.**

**Entry state:** clean `ab13fc19`-derived branch; `go test ./internal/listener/ -count=1` green.

- [ ] **Step 0 — re-run the collision greps at the IMPL tip** (`feedback_parallel_stream_mints_fresh_drift`; SPEC §5 requires the re-run at the PLAN *and* at the IMPL tip). All must be **0**:

```bash
cd <abs-worktree-path>
for id in classifyHandshakeErr handshakeOutcome sslHandshake sslFailVerifyError sslFailVerifyNoCert \
          outcomeOK outcomeVerifyError outcomeNoCert outcomeOther mkTestPKI mkDownstreamTSMutualTLS; do
  printf '%-26s %s\n' "$id" "$(grep -rn "$id" --include='*.go' . | wc -l)"
done
grep -rnE '(^|[^A-Za-z_])verifyError([^A-Za-z0-9_]|$)' --include='*.go' . | wc -l   # expect 0
grep -rnE '(^|[^A-Za-z_])noCert([^A-Za-z0-9_]|$)'      --include='*.go' . | wc -l   # expect 0
# parallel-stream re-check
git -C <abs-worktree-path> diff --stat f5a38c40 HEAD -- '*.go' go.mod go.sum test/   # expect EMPTY
```

**Design (RE-DERIVED at `ab13fc19`).** The classifier is a pure function over the error already bound at `manager.go:1178`. Its shape is forced by RD-NOCERT: `crypto/tls` exports **four error TYPES and ZERO error VALUES**, so there is no sentinel for the no-cert case and a string comparison is unavoidable. The text is version-invariant *structurally* — single producer `processCertsFromClient` (`handshake_server.go:940`), reached from `:703` (≤TLS 1.2) and `handshake_server_tls13.go:1056` (TLS 1.3) — but that is an argument, not a guarantee, which is exactly why the test must derive the string from a live handshake rather than hard-code it.

⚠️ **`tls.AlertError` is UNREACHABLE on a non-QUIC server conn** — the wrap at `crypto/tls/conn.go:1598` sits inside `if c.quic != nil`; a remote alert arrives as `*tls.permanentError` wrapping the *unexported* `tls.alert`. **Do not attempt to classify by alert code.** SPEC §3.5's ALERTMAP proved independently that the alert is not the bucket key: five distinct alerts (116, 48, 45, 43, 40) collapse onto two cert buckets, and the SAME no-cert input forced to TLS 1.2 emits alert 40 instead of 116 while still incrementing `fail_verify_no_cert`.

- [ ] **Step 1 — write the failing tests (red-first).** In `manager_test.go`:

```go
// mkTestPKI builds a fresh in-process ECDSA P-256 PKI for the handshake-outcome
// tests. internal/listener's existing corpus has a CA CERTIFICATE (testCAPEM)
// but NOT its private key, and no client certificate at all, so a mutual-TLS
// arm cannot be assembled from it (PLAN-74 F1). Shape follows
// internal/tls/config_test.go:42-80 and test/fixtures/0111/driver/driver.go:193-260.
type testPKI struct {
	caCertPEM, caKeyPEM         []byte             // trusted anchor
	serverCertPEM, serverKeyPEM []byte             // leaf signed by the trusted CA
	clientTrusted               stdtls.Certificate // chains to caCertPEM  -> accepted
	clientUntrusted             stdtls.Certificate // chains to a FOREIGN CA -> fail_verify_error
	serverRoots                 *x509.CertPool     // verifies the proxy's leaf
}

func mkTestPKI(t *testing.T) testPKI { /* generate: CA_A, server leaf/A, client leaf/A,
                                         CA_B (foreign), client leaf/B */ }

// TestClassifyHandshakeErr is the row's classification table. The no-cert case
// is CONSTRUCTED BY RUNNING A LIVE IN-PROCESS HANDSHAKE, never by writing the
// string: crypto/tls exports zero error VALUES, so a string match is forced and
// a hand-written string would sail through a toolchain that reworded it.
func TestClassifyHandshakeErr(t *testing.T) {
	pki := mkTestPKI(t)

	// (a) the LIVE no-cert error — the tripwire. Run a real net.Pipe handshake
	// with ClientAuth=RequireAndVerifyClientCert and a client that sends none.
	liveNoCertErr := liveHandshakeErr(t, pki, noClientCert)      // helper, below
	// (b) the LIVE untrusted-cert error, forced-send.
	liveVerifyErr := liveHandshakeErr(t, pki, untrustedClientCert)

	cases := []struct {
		name string
		err  error
		want handshakeOutcome
	}{
		{"nil", nil, outcomeOK},
		{"live no-cert (TLS 1.3)", liveNoCertErr, outcomeNoCert},
		{"live untrusted cert", liveVerifyErr, outcomeVerifyError},
		{"wrapped CVE", fmt.Errorf("listener: %w", &stdtls.CertificateVerificationError{}), outcomeVerifyError},
		{"unrelated error", errors.New("connection reset by peer"), outcomeOther},
		{"io.EOF", io.EOF, outcomeOther},
		{"cert/key mismatch shape", errors.New("tls: invalid signature by the client certificate: ECDSA verification failure"), outcomeOther},
		{"malformed DER shape", errors.New("tls: failed to parse client certificate: x509: malformed certificate"), outcomeOther},
		{"ctx deadline", context.DeadlineExceeded, outcomeOther},
	}
	for _, tc := range cases {
		if got := classifyHandshakeErr(tc.err); got != tc.want {
			t.Errorf("classifyHandshakeErr(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}

	// NON-VACUITY, proven not asserted: the two live errors must be DISTINCT
	// texts, and the untrusted one must carry exactly one unverified cert.
	if liveNoCertErr.Error() == liveVerifyErr.Error() {
		t.Fatalf("the two live arms produced the SAME error text %q — the table is vacuous", liveNoCertErr)
	}
	var cve *stdtls.CertificateVerificationError
	if !errors.As(liveVerifyErr, &cve) {
		t.Fatalf("live untrusted arm is not a *tls.CertificateVerificationError: %T", liveVerifyErr)
	}
	if n := len(cve.UnverifiedCertificates); n != 1 {
		t.Errorf("CVE.UnverifiedCertificates length = %d, want 1", n)
	}

	// The no-cert arm must NOT be a CVE — that is what makes errors.As a TOTAL
	// discriminator rather than a partial one.
	if errors.As(liveNoCertErr, &cve) {
		t.Errorf("live no-cert arm unexpectedly matched *tls.CertificateVerificationError")
	}
}

// TestClassifyHandshakeErr_TLS12 repeats the two live arms with the client and
// server both pinned to TLS 1.2, proving the classification is version-invariant.
// This is the arm that refutes any alert-code-derived implementation: the same
// no-cert input emits alert 40 at TLS 1.2 and 116 at TLS 1.3 (SPEC §3.5 ALERTMAP).
func TestClassifyHandshakeErr_TLS12(t *testing.T) { /* MaxVersion/MinVersion = VersionTLS12 */ }
```

`liveHandshakeErr` drives a real handshake over a **LOOPBACK TCP PAIR** — `net.Listen("tcp","127.0.0.1:0")` + `Dial`/`Accept`, wrapped in a `connPair(t) (client, server net.Conn)` helper — then `stdtls.Server(srvSide, cfg)` in a goroutine returning its `HandshakeContext(context.Background())` error over a channel, `stdtls.Client(cliSide, ccfg)` on the test goroutine. ⚠️ **DO NOT USE `net.Pipe()` — IT DEADLOCKS, AND IT DEADLOCKS SILENTLY.** V1 executed it: `net.Pipe` is synchronous and unbuffered, so in the `RequireAndVerifyClientCert` no-cert arm the server blocks writing alert 116 out of `processCertsFromClient` (`handshake_server.go:982`) while the client is still blocked mid-`Write` on the remainder of its second flight — neither side reads, and the test hits the 45-second panic timeout with **no failing assertion to read**. It is structural, not fixable by adding a post-handshake client `Read`. With the loopback pair every T1 assertion below passes in ~0.008s. ⚠️ **The untrusted arm MUST install `GetClientCertificate`** (`reference_go_client_cert_withholding`): without forced-send it degrades into the no-cert arm and the two table rows stop discriminating — SPEC §3.5 observed exactly that collapse in its control arm. ⚠️ **Pass `context.Background()`, never a `WithTimeout` ctx** — a firing ctx REPLACES the outcome (`crypto/tls/conn.go:1536-1547`, RD-CONNLINES) and every arm would classify `outcomeOther`.

Run `go test ./internal/listener/ -run 'TestClassifyHandshakeErr' -count=1`. **Expected: FAIL** — `undefined: classifyHandshakeErr`, `undefined: handshakeOutcome`, `undefined: outcomeOK`. **Record the verbatim red.**

- [ ] **Step 2 — implement.** In `manager.go`:

```go
// handshakeOutcome classifies a downstream TLS handshake result into the three
// counted buckets plus a fourth that counts NOTHING.
//
// ⚠️ outcomeVerifyError means "certificate CHAIN VERIFICATION failed" — NOT
// "client cert rejected". A cert/private-key mismatch and a malformed DER never
// reach certs[0].Verify() inside crypto/tls and land in outcomeOther, which
// increments nothing. The reference books those under ssl.connection_error; that
// asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5).
//
// ⚠️ The no-cert arm is matched by STRING. crypto/tls exports four error TYPES
// and ZERO error VALUES, so there is no sentinel to compare against. The text is
// produced at exactly one site (processCertsFromClient, handshake_server.go:940,
// reached from :703 for <=TLS 1.2 and handshake_server_tls13.go:1056 for TLS 1.3),
// which is why it is version-invariant — but that is an argument, not a promise.
// TestClassifyHandshakeErr constructs this error by running a LIVE handshake, so
// a toolchain that rewords the message turns that test red. Do NOT "simplify" it
// to a hand-written string; that would delete the tripwire.
//
// ⚠️ Accuracy is CONDITIONAL on the handshake ctx carrying no deadline: a firing
// ctx REPLACES the real error via the named-return override at
// crypto/tls/conn.go:1541-1546. Today the ctx reaching HandshakeContext is
// cmd/envoy-go/main.go:339's cancel-only signal.NotifyContext — the one
// production context.WithTimeout under internal/listener/
// (listenerfilter/pipeline.go:43) cannot escape, because Pipeline.Run returns
// only an error and serveConnection never rebinds ctx. Any future row that adds
// a handshake deadline re-opens this.
type handshakeOutcome int

const (
	outcomeOK handshakeOutcome = iota
	outcomeVerifyError
	outcomeNoCert
	outcomeOther
)

// noClientCertErrText is crypto/tls's bare errors.New for "the client was asked
// for a certificate and sent none" (handshake_server.go:964, go1.26.5).
const noClientCertErrText = "tls: client didn't provide a certificate"

func classifyHandshakeErr(err error) handshakeOutcome {
	if err == nil {
		return outcomeOK
	}
	var cve *stdtls.CertificateVerificationError
	if errors.As(err, &cve) {
		return outcomeVerifyError
	}
	if err.Error() == noClientCertErrText {
		return outcomeNoCert
	}
	return outcomeOther
}
```

⚠️ **Order matters: `errors.As` FIRST.** SPEC §3.5 confirmed `errors.As` is an exact, total discriminator across ten distinct non-cert shapes at both TLS versions, so the CVE check can never steal a no-cert error — but writing the string check first would make the table's ordering load-bearing for no reason.

- [ ] **Step 3 — run the tests.** `go test ./internal/listener/ -run 'TestClassifyHandshakeErr' -count=1 -v`. **Expected: PASS**, both the TLS 1.3 and TLS 1.2 tables plus all four non-vacuity assertions.
- [ ] **Step 4 — hygiene + commit.** `gofmt -l internal/listener` silent · `go vet ./internal/listener/` · `golangci-lint run ./internal/listener/` · `go test ./internal/listener/ -count=1` fully green. ⚠️ **Verify `git diff internal/listener/manager.go` shows NO import-block hunk** — that is the mechanical proof of "+0 production imports". Commit.
- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break A [`other` collapsed into `verifyError`]:** change the final `return outcomeOther` to `return outcomeVerifyError` ⇒ **the FIVE `outcomeOther` rows (`unrelated error`, `io.EOF`, `cert/key mismatch shape`, `malformed DER shape`, `ctx deadline`) FIRE while the four cert-ish rows (`nil`, `live no-cert`, `live untrusted`, `wrapped CVE`) STAY GREEN.** ⚠️ **FIVE, not four** — V1 executed it and got exactly five firings; a wrong count in a break record is precisely what a break audit trips on. `git restore`; re-green. *(Discriminates: proves `outcomeOther` is a real fourth bucket and not an unreachable default — SPEC §3.5's departure claim depends on it.)*
  - **Break F [hand-written no-cert string — THE ONE THAT MUST NOT FIRE]:** replace the live no-cert error with `errors.New(noClientCertErrText)` **in BOTH live no-cert arms — `TestClassifyHandshakeErr` AND `TestClassifyHandshakeErr_TLS12`** — and delete the two non-vacuity assertions that reference `liveNoCertErr` ⇒ **the suite must be SHOWN to stay GREEN.** Then, still under Break F, mutate `noClientCertErrText` by one character — **the suite must STILL be green**, which is the demonstration that the hand-written form is self-consistent and therefore worthless. `git restore`; re-green.
    ⚠️ **SCOPE IS LOAD-BEARING — V1 executed the single-test version and IT FIRED:** `TestClassifyHandshakeErr_TLS12` carries its **own** live no-cert arm, so mutating the const with only the 1.3 arm converted turns it red (`TLS1.2 live no-cert = 3, want outcomeNoCert`) — a false alarm that would trip this document's own stage-stopping "Break F must not fire" rule. **Convert both arms.**
    ⚠️ **"in both places" is wrong and is corrected here:** once the table row reads `errors.New(noClientCertErrText)` it references the const **by name**, so there is exactly ONE place; mutating it moves both sides together. That is the point, not a gap.
    ⚠️ **NAMED SUBSTITUTION (do not re-derive it):** after the swap, `liveNoCertErr` becomes unused and the build fails `vet: declared and not used: liveNoCertErr`. **Delete its `:=` binding** (or add `_ = liveNoCertErr`). V1 hit this deterministically. `[NOT pre-compiled — substitution rule applies]`
  - **Break F′ [the control that proves F's point]:** with the LIVE construction restored, mutate `noClientCertErrText` by one character in the **production** file only ⇒ **the `live no-cert (TLS 1.3)` and `live no-cert` TLS-1.2 rows FIRE.** `git restore`; re-green. *(Together F and F′ are the cross-product: hand-written ⇒ green under mutation; live ⇒ red under mutation. Either alone is not evidence.)*

**Commit:** `listener(phase 74 T1): classify the downstream TLS handshake error ALREADY BOUND at manager.go:1178 into three counted buckets plus a fourth that counts NOTHING — an unexported handshakeOutcome + classifyHandshakeErr using errors.As for *tls.CertificateVerificationError and a FORCED string match for the no-cert arm (crypto/tls exports four error TYPES and ZERO error VALUES, so there is no sentinel), with the no-cert case CONSTRUCTED BY A LIVE IN-PROCESS HANDSHAKE at both TLS 1.2 and 1.3 so a reworded stdlib message turns the test red; +0 production imports (errors :7 and stdtls :5 already present)`

---

## Task 2 — the three counter fields + the `rt.tlsMode`-gated registration + the NAME-SET guard

**Files:**
- Modify: `internal/listener/manager.go` (the `:172-174` comment · +3 fields beside `:175-176` · the `:345` comment · the gated block inside `registerListenerMetrics` `:351-355`)
- Test: `internal/listener/manager_test.go` (the name-set guard, the plaintext-zero pin, the gate-equivalence test, and the D7 rename at `:1911-1927`/`:1928`)

**Interfaces:**
- Produces: `rt.sslHandshake`, `rt.sslFailVerifyError`, `rt.sslFailVerifyNoCert`, all `*stats.Counter`, **nil on a plaintext listener** and non-nil on every TLS listener (TCP or QUIC). Consumed by T4's Inc points.
- Consumes: `rt.tlsMode` (`:145`, set at `:639`, RD-TLSMODE-ORDER confirms it is populated first) and `r.NewCounter` (`registry.go:84`).

**Entry state:** T1 landed; `go test ./internal/listener/ -count=1` green.

**⚠️ THE GATE IS `rt.tlsMode` ALONE. NO KIND CHECK.** See RD-QUICSTALE — SPEC `:1`/`:315`/`:320` still carry the refuted first reading and must be ignored in favour of §3.4/§16 and ADR-0296 §Context ¶8(ii).

- [ ] **Step 1 — write the failing tests (red-first).** In `manager_test.go`:

```go
// listenerSSLNames returns the ssl.* metric names registered at the given
// listener's scope. It asserts on the NAME SET, not on a cardinality: the
// landed precedent (internal/statssink/registration_test.go:26, one of five,
// all routing through a countMetrics helper that never inspects m.Name())
// counts via reg.Walk and WOULD PASS WITH ALL THREE NAMES MISSPELLED.
func listenerSSLNames(reg *stats.Registry, addr string) []string {
	prefix := "listener." + normalizeAddr(addr) + ".ssl."
	var out []string
	reg.Walk(func(m stats.Metric) {
		if strings.HasPrefix(m.Name(), prefix) {
			out = append(out, m.Name())
		}
	})
	sort.Strings(out)
	return out
}

// TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames pins the EXACT
// SPELLING of all three names. A count-only assertion is insufficient.
func TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames(t *testing.T) {
	// one TLS listener (mkDownstreamTSInline + mkTLSChain + mkTLSListener), Start, Walk.
	addr := ls[0].Addr
	want := []string{
		"listener." + normalizeAddr(addr) + ".ssl.fail_verify_error",
		"listener." + normalizeAddr(addr) + ".ssl.fail_verify_no_cert",
		"listener." + normalizeAddr(addr) + ".ssl.handshake",
	}
	got := listenerSSLNames(reg, addr)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ssl name set = %v, want %v", got, want)
	}
	// and the two pre-existing names are UNDISTURBED (this row is additive).
	for _, n := range []string{".downstream_cx_total", ".downstream_cx_active"} { /* present */ }
}

// TestListenerMetrics_PlaintextListenerRegistersNoSSLNames is the INVERTED
// guard: registration is TLS-chains-only, matching the reference (SPEC §3.3,
// settled by a one-container/two-listener/one-scrape probe after the
// zero-visibility confound was eliminated first).
func TestListenerMetrics_PlaintextListenerRegistersNoSSLNames(t *testing.T) {
	// a plaintext tcp_proxy listener, Start, Walk.
	if got := listenerSSLNames(reg, addr); len(got) != 0 {
		t.Errorf("plaintext listener registered ssl names %v, want none", got)
	}
	// ...while it DOES carry its two cx names — proving the walk is not vacuous.
}

// TestListenerMetrics_GateMatchesInc pins the invariant that makes the whole
// design safe: the REGISTRATION gate (rt.tlsMode) and the Inc guard
// (selected.tlsCfg != nil) are EQUIVALENT, because a listener is all-TLS or
// all-plaintext and never both (manager.go:516-525, ADR-0033 cl.5/ADR-0078 cl.5).
// If they ever diverge the failure mode is a NIL-POINTER PANIC in the
// serveConnection GOROUTINE — *stats.Counter.Inc has no nil guard
// (internal/stats/counter.go:22) and internal/listener has no recover() —
// i.e. a process crash, not a wrong count. That is why this is its own test.
func TestListenerMetrics_GateMatchesInc(t *testing.T) {
	// (a) a mixed TLS+plaintext listener is REJECTED at build time, with the
	//     exact substring "mixed TLS and plaintext chains on one listener".
	// (b) for a TLS listener:      rt.tlsMode == true  AND every chainInfo has tlsCfg != nil
	//                               AND all THREE *stats.Counter fields are NON-NIL.
	// (c) for a plaintext listener: rt.tlsMode == false AND every chainInfo has tlsCfg == nil
	//                               AND all THREE *stats.Counter fields are NIL.
	// (b)/(c) reach rt via mgr.runtimes (same package).
	//
	// ⚠️ THE POINTER ASSERTIONS ARE THE LOAD-BEARING HALF, and without them this
	// test is decorative. V1 executed the tlsMode-vs-tlsCfg-only version under
	// Break E (the gate dropped) and it STAYED GREEN: both predicates are set at
	// BUILD time (manager.go:639 and :510), entirely upstream of
	// registerListenerMetrics, so neither one can observe a registration bug.
	// Only the three field pointers -- non-nil iff tlsMode -- actually witness
	// the invariant whose violation is a PRODUCTION CRASH.
}
```

Also in this step: **rename `TestListenerManager_AllocatesTwoMetricsPerListener` → `TestListenerManager_AllocatesBaseListenerMetrics`** and rewrite its doc comment across **`:1911-1927`** (F5 — the doc runs to `:1927`, not `:1913`) so that neither the name nor the doc claims "exactly the 2". ⚠️ **Its body is presence-only (`:1947-1948`), so +3 produces NO red from it** (D7) — nothing else will catch this; it is a deliberate documentation fix, not a test fix.

Run `go test ./internal/listener/ -count=1`. **Expected: FAIL** — the three name-set assertions fail with an empty `got` (the names do not exist yet). ⚠️ **The plaintext test will PASS immediately** — that is expected and is exactly why Break E exists: an assertion that is green before the change is not evidence until a break shows it can go red. **Record the verbatim red.**

- [ ] **Step 2 — add the three fields.** Beside `:175-176`, and **rewrite the `:172-174` comment** (D6) — it currently reads *"2 metrics per listener"*:

```go
	// 06.1 metric fields (per SPEC §6 — listener-scope only). Allocated by
	// registerListenerMetrics at Start time (post-bind, pre-Freeze) and
	// Inc/Dec'd from the accept-loop hot path. The two cx metrics are
	// registered for EVERY listener; the three ssl.* counters are registered
	// only when rt.tlsMode is set (phase 74 — TLS-chains-only, matching the
	// reference), so on a plaintext listener the three pointers stay NIL.
	downstreamCxTotal   *stats.Counter
	downstreamCxActive  *stats.Gauge
	sslHandshake        *stats.Counter // phase 74: successful downstream TLS handshakes
	sslFailVerifyError  *stats.Counter // phase 74: client cert presented, CHAIN VERIFICATION failed
	sslFailVerifyNoCert *stats.Counter // phase 74: no client cert where one was required
```

- [ ] **Step 3 — add the gated registration**, and **rewrite the `:345` comment** (D6 — it says *"allocates the 2 listener-scope metrics"*):

```go
// registerListenerMetrics allocates the listener-scope metrics per SPEC §6 and
// stores the pointers on rt for the accept-loop hot path. Called once per
// listener at Start time, after net.Listen resolves the configured port (so the
// metric name reflects the actual bound address, and so two listeners
// configured with port 0 don't collide on the same registered name pre-bind).
// Pre-Freeze (Task 12 owns the Freeze call after the admin server is up).
//
// The two cx metrics are unconditional. The three phase-74 ssl.* counters are
// gated on rt.tlsMode, matching the reference, which registers listener.<addr>.ssl.*
// on TLS-bearing chains ONLY (a plaintext listener carries zero ssl.* names while
// carrying 15+ other zero-valued names in the same scope — probed at the phase-74
// SPEC, after first proving that plain /stats does render zeros).
//
// ⚠️ The gate is rt.tlsMode ALONE — there is deliberately NO kind check. The
// reference registers the full ssl.* family on a QUIC listener at boot and NEVER
// moves it (ssl.handshake: 0 after five successful HTTP/3 connections;
// connection_error: 0 on a failure arm where the TCP comparator in the same
// process and the same scrape fired), so envoy-go doing the same is EXACT
// PARITY, and gating QUIC out would be the DEPARTURE. This is sufficient for
// QUIC by construction: startQUIC hard-errors when the chain carries no TLS
// config (quic.go:33-36), so every QUIC listener that boots has tlsMode == true.
// Do NOT re-express this as "has a TCP-style TLS transport socket" — that form
// would wrongly exclude QuicDownstreamTransport.
func registerListenerMetrics(r *stats.Registry, rt *listenerRuntime) {
	prefix := "listener." + normalizeAddr(rt.addr) + "."
	rt.downstreamCxTotal = r.NewCounter(prefix + "downstream_cx_total")
	rt.downstreamCxActive = r.NewGauge(prefix + "downstream_cx_active")
	if rt.tlsMode {
		rt.sslHandshake = r.NewCounter(prefix + "ssl.handshake")
		rt.sslFailVerifyError = r.NewCounter(prefix + "ssl.fail_verify_error")
		rt.sslFailVerifyNoCert = r.NewCounter(prefix + "ssl.fail_verify_no_cert")
	}
}
```

⚠️ **No `NewCounterIfAbsent`, no `IfAbsent` anywhere.** `registerListenerMetrics` has exactly TWO call sites and they are mutually exclusive (RD-REGCALLERS: `Start`'s first loop `continue`s on `kindQUIC` at `:963-972`, so a QUIC runtime registers only via `quic.go:45`), so plain `NewCounter` is correct and a duplicate registration would rightly panic (`registry.go:107`).

⚠️ **Charset:** all three FULL names pass `IsValidName` (`NamePattern` `registry.go:48` — `checkName` panics at **`:117`**, not `:107`; RD-CHECKNAME). Verified EXECUTED at the SPEC for both spellings and the IPv6 form. **The all-underscore address form is what saves it** — a dotted address would truncate at the first octet in SN3.

- [ ] **Step 4 — run the tests.** `go test ./internal/listener/ -count=1`. **Expected: PASS** — the exact three-name set on the TLS listener, zero on the plaintext listener, the gate-equivalence predicates agreeing, the renamed base-metrics test still green, and every pre-existing listener test green.
- [ ] **Step 5 — hygiene + commit.** `gofmt -l` · `go vet` · `golangci-lint run ./internal/listener/` · `git diff internal/listener/manager.go` shows **no import hunk**. Commit.
- [ ] **Step 6 — breaks (AFTER committing).**
  - **Break E [`rt.tlsMode` gate dropped]:** delete the `if rt.tlsMode {` wrapper, registering all three unconditionally ⇒ **`TestListenerMetrics_PlaintextListenerRegistersNoSSLNames` FIRES** (it now sees three names). **`TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames` must STAY GREEN.** `git restore`; re-green. *(Discriminates: this is the only assertion that catches an ungated registration; and its firing here is what converts the plaintext test from "green by accident" into a live guard. Its counterpart half — that the QUIC test must NOT also fire — is checked at T3, where that test exists.)*
  - **Break E′ [misspell one name]:** change `"ssl.fail_verify_no_cert"` to `"ssl.fail_verify_nocert"` ⇒ **the name-set assertion FIRES.** `git restore`; re-green. *(Discriminates: this is precisely the break a count-only guard — the landed `statssink` precedent — would MISS. Run it and record that a cardinality assertion would have stayed green.)*

**Commit:** `listener(phase 74 T2): register listener.<addr>.ssl.{handshake,fail_verify_error,fail_verify_no_cert} on TLS-bearing listeners ONLY — three *stats.Counter fields gated on the ALREADY-EXISTING rt.tlsMode (manager.go:145, set :639), whose FIRST production consumer this is, unambiguous because mixed TLS+plaintext chains on one listener are rejected at :516-525; the gate is rt.tlsMode ALONE with NO kind check, so a QUIC listener registers all three and leaves them permanently zero — EXACT PARITY with the reference, which does the same. The guard asserts the exact NAME SET, not a cardinality: the landed statssink precedent counts via reg.Walk and never inspects m.Name(), so it would pass with all three names misspelled`

---

## Task 3 — the QUIC registration test (Break D's target — the row's DISTINGUISHING break)

**Files:**
- Test: `internal/listener/quic_test.go` (new test; `mkQUICListener*` helpers live in `manager_test.go`, same package)
- **`internal/listener/quic.go` stays BYTE-UNTOUCHED** — there is nothing to gate.

**Interfaces:**
- Consumes: T2's gated `registerListenerMetrics` (reached via `quic.go:45`, the QUIC call site), `mkQUICListenerHCM` (`manager_test.go:790`), `listenerSSLNames` (T2), `pollCounter` (`quic_test.go:32`).

**Entry state:** T2 landed; `go test ./internal/listener/ -count=1` green.

**Why this is its own task.** It is the only assertion that discriminates SPEC §3.4 — the decision this SPEC *reversed on itself* — and Break D is deliberately framed as **ADDING** the wrong gate rather than removing the right one, because the correct implementation has no kind check to delete. A reviewer can meaningfully reject this task while approving T2.

⚠️ **RD-QUICTEST: do NOT claim an existing pin for the cx-gauge half.** `quic_test.go:92-97` asserts **only** `downstream_cx_total`; `downstream_cx_active` is asserted nowhere in that file, and `pollCounter` type-asserts `*stats.Counter` with **no gauge equivalent**. This task asserts the counter half only and records the gauge half as unasserted — do not invent a claim the corpus does not support.

- [ ] **Step 1 — write the test (red-first).**

```go
// TestQUICListener_RegistersSSLNamesAtZero pins SPEC §3.4: a QUIC listener
// registers all THREE ssl.* counters (because startQUIC hard-errors without a
// TLS config, so tlsMode is necessarily true) and they stay permanently ZERO
// across a COMPLETED HTTP/3 handshake — because quic-go's Accept returns
// post-handshake (quic.go:84-85, :109), so a QUIC handshake never surfaces as a
// per-connection event that could increment them.
//
// ⚠️ This is PARITY, not a departure. The reference behaves IDENTICALLY:
// ssl.handshake: 0 after five successful H3 connections, and connection_error: 0
// on a failure arm where the TCP comparator in the SAME process and the SAME
// scrape fired (phase-74 SPEC §3.4, EXECUTED). Gating QUIC out — which this
// SPEC's own first reading did, and which SPEC :1/:315/:320 still stale-carry —
// would have been the departure.
func TestQUICListener_RegistersSSLNamesAtZero(t *testing.T) {
	// a QUIC listener via mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, HTTP3), Start.
	addr := /* the resolved UDP addr */

	// (1) REGISTRATION: all three names present, spelled exactly.
	got := listenerSSLNames(reg, addr)
	want := []string{ ".ssl.fail_verify_error", ".ssl.fail_verify_no_cert", ".ssl.handshake" /* fully qualified */ }
	if !reflect.DeepEqual(got, want) {
		t.Errorf("QUIC listener ssl name set = %v, want %v", got, want)
	}

	// (2) drive a REAL H3 request (the http3.Transport shape at quic_test.go:121-126),
	//     and Fatalf if it does not succeed — a failed drive would make (3)
	//     vacuous, so it is a PRECONDITION, not a property.
	if err := driveH3(t, addr); err != nil {
		t.Fatalf("precondition: H3 round trip failed, so the zero-assertion below would be vacuous: %v", err)
	}

	// (3) the cx counter DID move — proving the connection was accounted, so the
	//     zeros in (4) are a real observation and not "nothing happened".
	if got := pollCounter(t, reg, "listener."+normalizeAddr(addr)+".downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}

	// (4) ...and all three ssl.* counters are STILL ZERO.
	for _, n := range want {
		if v := counterValue(t, reg, n); v != 0 {
			t.Errorf("%s = %d after a completed H3 handshake, want 0", n, v)
		}
	}
	// (5) and nothing panicked — implicit, but state it: a nil-Counter Inc from
	//     the QUIC accept goroutine would crash the binary, not fail this test.
}
```

⚠️ **Assertion (3) is what makes (4) non-vacuous** (`reference_probe_must_discriminate`): without it, "all three are zero" is equally consistent with "the listener never accepted anything". ⚠️ **`counterValue` is a NEW helper** — `pollCounter` polls for a MINIMUM and cannot assert an exact zero. Add it beside `pollCounter` in `quic_test.go`. ⚠️ **It MUST `t.Errorf` on an ABSENT name, never return a silent 0** — otherwise assertion (4) passes VACUOUSLY under Break D (nothing registered ⇒ every read is 0 ⇒ "all three are zero" is trivially true). This is F9's lesson applied one layer in. V1 confirmed that with the absent-name error in place, Break D produces `ssl name set = [], want [...]` **plus** three `counter "…" is not registered` lines, and (1) is unambiguously the discriminating failure.

Run `go test ./internal/listener/ -run 'TestQUICListener_RegistersSSLNamesAtZero' -count=1`. **Expected: PASS immediately** — T2 already registers them. ⚠️ **A test that is green on arrival is not evidence.** That is precisely what Break D is for, and it is why this test ships in the same commit as its break rather than before it.

- [ ] **Step 2 — hygiene + commit.** `gofmt -l` · `go vet` · `golangci-lint run ./internal/listener/` · **`sha256sum internal/listener/quic.go` matches master** (record both hashes). Commit.
- [ ] **Step 3 — breaks (AFTER committing).**
  - **Break D [ADD the refuted `rt.kind != kindQUIC` gate — THE ROW'S DISTINGUISHING BREAK]:** change T2's `if rt.tlsMode {` to `if rt.tlsMode && rt.kind != kindQUIC {` — i.e. implement SPEC `:320`'s stale first reading ⇒ **`TestQUICListener_RegistersSSLNamesAtZero` assertion (1) FIRES** (the name set is empty) **while `TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames` and the plaintext test both STAY GREEN.** `git restore`; re-green. *(Discriminates: it is the ONLY assertion that separates "gate on tlsMode" from "gate on tlsMode AND kind". Confirm the firing assertion is (1), the NAME-SET comparison — not (4)'s zero check, which would still pass vacuously with no names registered at all. If (4) fires instead of (1), the test is mis-ordered and must be fixed before the break counts.)*
  - **Break E, second half [run here, where both tests exist]:** re-apply Break E (drop `if rt.tlsMode`) ⇒ **the plaintext test FIRES and `TestQUICListener_RegistersSSLNamesAtZero` must NOT.** *(QUIC has `tlsMode == true`, so dropping the gate cannot change its outcome. **If BOTH fire, the two tests are entangled and the gate is being tested at the wrong layer** — that is a FINDING, record it and stop.)* `git restore`; re-green.

**Commit:** `listener(phase 74 T3): pin the QUIC coverage boundary as SHARED PARITY, not a departure — a QUIC listener registers all THREE ssl.* counters (tlsMode is necessarily true; startQUIC hard-errors without a TLS config) and they stay permanently ZERO across a COMPLETED H3 handshake, with downstream_cx_total ASSERTED NON-ZERO in the same test so the zeros are an observation rather than an absence. Break D — ADDING the refuted kind != kindQUIC gate, which SPEC :1/:315/:320 still stale-carry against their own §3.4 — is the row's distinguishing break; quic.go stays BYTE-UNTOUCHED because there is nothing to gate`

---

## Task 4 — the two Inc points in `serveConnection` + the three increment tests

**Files:**
- Modify: `internal/listener/manager.go` (`:1179` classify+Inc inside the error branch · `:1183` success Inc — **BOTH inside the existing `if selected.tlsCfg != nil` block**)
- Test: `internal/listener/manager_test.go` (`mkDownstreamTSMutualTLS` + the three increment tests + the plaintext no-Inc test)

**Interfaces:**
- Consumes: T1's `classifyHandshakeErr` / `outcome*`; T2's three fields; T1's `mkTestPKI`.
- Produces (test): `func mkDownstreamTSMutualTLS(t *testing.T, certPEM, keyPEM, caPEM []byte) *corev3.TransportSocket` — `require_client_certificate: true` **PLUS `common_tls_context.validation_context.trusted_ca`**. ⚠️ **`mkDownstreamTSRequireClientCert` (`manager_test.go:644`) is NOT reusable** (F2): it has no `trusted_ca` and its own doc says it *"should be rejected at build time"* — the phase-67/ADR-0289 anchorless-VC boot reject.

**Entry state:** T1–T3 landed; `go test ./internal/listener/ -count=1` green.

- [ ] **Step 1 — write the failing tests (red-first).**

```go
// The three arms drive a REAL handshake against a REAL Start'ed listener whose
// chain is require_client_certificate: true + validation_context.trusted_ca,
// then poll the registry. serveConnection runs in a goroutine (manager.go:1081)
// so the Inc lags the client's Handshake() return — poll, never sleep.
func TestServeConnection_SSLHandshakeIncrements(t *testing.T)        { /* trusted client cert -> ssl.handshake == 1 */ }
func TestServeConnection_SSLFailVerifyErrorIncrements(t *testing.T)  { /* FORCED-SEND untrusted -> ssl.fail_verify_error == 1 */ }
func TestServeConnection_SSLFailVerifyNoCertIncrements(t *testing.T) { /* no client cert -> ssl.fail_verify_no_cert == 1 */ }
```

**Each of the three must assert the CROSS-PRODUCT, not just its own counter** (`reference_probe_must_discriminate`): its own counter is **1** *and the other two are **0***. Without the negative half, Break B (swapping the two failure arms) would fire in three tests instead of two and prove nothing about which arm went where.

⚠️ **The untrusted arm MUST install `GetClientCertificate`** (`reference_go_client_cert_withholding`): the server advertises only CA_A's DN, so a polite client withholds CA_B's cert and the arm **degrades into the no-cert arm** — SPEC §3.5 observed exactly this collapse in its control. Assert the non-degradation directly: in the untrusted test, `ssl.fail_verify_no_cert` must be **0**.

```go
// TestServeConnection_PlaintextListenerIncrementsNoSSL is Break C's target.
// A plaintext listener's three ssl.* pointers are NIL (T2's gate), so the guard
// keeping the Inc points inside `if selected.tlsCfg != nil` is not a style
// choice: *stats.Counter.Inc has NO nil check (internal/stats/counter.go:22) and
// internal/listener has NO recover(), so an Inc on a plaintext connection is a
// nil-pointer PANIC in the serveConnection GOROUTINE — a process crash.
func TestServeConnection_PlaintextListenerIncrementsNoSSL(t *testing.T) {
	// plaintext tcp_proxy listener; drive one plain TCP round trip; assert
	// downstream_cx_total == 1 (the connection WAS served -- non-vacuity)
	// and listenerSSLNames(reg, addr) is EMPTY.
}
```

Run `go test ./internal/listener/ -count=1`. **Expected: FAIL** — all three increment tests report their counter at 0. **Record the verbatim red.** ⚠️ The plaintext test passes immediately; Break C is what makes it evidence.

- [ ] **Step 2 — add the two Inc points.** Inside the existing `:1176` block, changing **no** control flow:

```go
	if selected.tlsCfg != nil {
		tlsConn := stdtls.Server(pkConn, selected.tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			// phase 74: book the outcome before logging. `err` is scoped to this
			// if-init, so this is the only place it is in scope.
			// outcomeOther deliberately increments NOTHING — the reference books
			// those under ssl.connection_error, a name this row does not land.
			// That asymmetry is a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5).
			switch classifyHandshakeErr(err) {
			case outcomeVerifyError:
				rt.sslFailVerifyError.Inc()
			case outcomeNoCert:
				rt.sslFailVerifyNoCert.Inc()
			}
			log.Printf("listener %q: handshake: %v", rt.name, err)
			_ = pkConn.Close()
			return
		}
		// phase 74: a COMPLETED downstream TLS handshake.
		rt.sslHandshake.Inc()
		dispatchConn = tlsConn
	}
```

⚠️ **Both Incs stay INSIDE the `if selected.tlsCfg != nil` block, and no nil guards are added.** The three pointers are non-nil exactly when `rt.tlsMode`, and `rt.tlsMode ⟺ selected.tlsCfg != nil` by `manager.go:516-525` (T2's `TestListenerMetrics_GateMatchesInc`). Adding a defensive `if rt.sslHandshake != nil` would make Break C vacuous and would mask the very invariant under test.

- [ ] **Step 3 — run the tests.** `go test ./internal/listener/ -count=1`. **Expected: PASS** — all three arms with their cross-products, the plaintext test, and every pre-existing listener test.
- [ ] **Step 4 — hygiene + `-race` + commit.** `gofmt -l` · `go vet` · `golangci-lint run ./internal/listener/` · **`go test ./internal/listener/ -race -count=1`** (the Inc is on the accept-loop hot path from a per-connection goroutine — `-race` here is not optional) · `git diff internal/listener/manager.go` shows **no import hunk**. Commit.
- [ ] **Step 5 — breaks (AFTER committing).**
  - **Break B [swap the `verifyError` and `noCert` arms]:** exchange the two `case` bodies ⇒ **must fire in EXACTLY TWO of the three increment tests** — the untrusted arm and the no-cert arm — **while `TestServeConnection_SSLHandshakeIncrements` STAYS GREEN.** `git restore`; re-green. *(Discriminates: the cross-product is what discriminates. Exactly two firings proves the two failure arms are independently routed; three firings would mean the success arm is entangled; one firing would mean one of the negative halves is missing.)*
  - **Break C [move the success Inc outside the TLS block]:** move `rt.sslHandshake.Inc()` from inside the `if selected.tlsCfg != nil` block to just before `rt.serveNetworkChain(...)` at `:1189` ⇒ **the plaintext test FIRES.** ⚠️ **EXPECT A PROCESS CRASH, NOT AN ASSERTION FAILURE** (F3): `rt.sslHandshake` is nil on a plaintext listener and `Inc()` dereferences it inside the `serveConnection` goroutine with no `recover()`, so the whole test binary panics. **That IS the firing — but CONFIRM IT FROM THE STACK**: the panic must be `runtime error: invalid memory address or nil pointer dereference` with `stats.(*Counter).Inc` and `listener.(*listenerRuntime).serveConnection` on it. A crash from any other site is a different bug, not this break. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]`
  - **Break C′ [the non-crashing variant, if C's crash obscures which test fired]:** instead, change the guard from `if selected.tlsCfg != nil` to `if true` **and** add a temporary `if rt.sslHandshake != nil` around the Inc ⇒ the plaintext test fires as a clean assertion failure. *(Run this only if C's crash makes attribution ambiguous; record which variant was used and why — **a substitution's rationale is itself a claim**.)*

**Commit:** `listener(phase 74 T4): book every downstream TLS handshake outcome at its ONE existing site — classify+Inc in the :1178 error branch (where err is scoped to the if-init) and a success Inc at :1183, both INSIDE the existing `if selected.tlsCfg != nil` block and with NO nil guards, because the three pointers are non-nil exactly when rt.tlsMode and the two predicates are equivalent by the :516-525 mixed-chain reject. Three increment tests, each asserting the CROSS-PRODUCT (its own counter 1, the other two 0) so Break B fires in exactly two of three; the untrusted arm forces GetClientCertificate or it silently degrades into the no-cert arm. outcomeOther increments NOTHING — a named departure, not an omission`

---

## Task 5 — `internal/stats/name.go`: three `helpText` entries + its stale doc comment

**Files:**
- Modify: `internal/stats/name.go` (`:452-464` the map; **`:445-451` the doc comment — F4**)
- Test: **`internal/stats/name_test.go`** — existing `helpText` coverage is at **`:217-218`** (`helpText["envoy_server_accesslog_dropped"]`), which is the shape to clone. *(An earlier draft said "locate at IMPL time"; V2 correctly called that a TBD in disguise — one grep resolves it.)*

**Interfaces:** none — this is a pure admin-surface fix. **`internal/stats` is NOT byte-untouched** (SPEC §7 corrects BRAINSTORM §10).

**Entry state:** T1–T4 landed; `go test ./internal/stats/ -count=1` green.

**Why it is owed.** Without it `/stats/prometheus` degrades to `# HELP envoy_listener_ssl_handshake envoy_listener_ssl_handshake` — cosmetic, but a real admin-surface delta. **These are the first three-dot listener names in the project**; SN3 flattening was EXECUTED-verified at the SPEC (residual `listener.ssl.handshake` + label `{envoy_listener_address 0_0_0_0_10000}`; `WriteProm` ⇒ `envoy_listener_ssl_handshake{envoy_listener_address="0_0_0_0_10000"}`).

- [ ] **Step 1 — write the failing test (red-first).** Assert `helpText` has an entry for each of `envoy_listener_ssl_handshake`, `envoy_listener_ssl_fail_verify_error`, `envoy_listener_ssl_fail_verify_no_cert`, and that each is non-empty and **not equal to its own metric name** (the degradation signature). Run it. **Expected: FAIL.** Record the red.
- [ ] **Step 2 — add the three entries** to the map (it ends at `:464`, the file's last line, so this is a pure append inside the literal) **and rewrite the `:445-451` doc comment**, which currently says *"The **11 entries** cover the 13 unique Prometheus names emitted by 06.1 … plus one 06.2 backpressure counter"* — after this task it is **14 entries**, and the added three are phase-74 listener-scope TLS handshake outcomes, not 06.1 names.

```go
	"envoy_listener_ssl_handshake":           "Total successful downstream TLS handshakes on the listener.",
	"envoy_listener_ssl_fail_verify_error":   "Downstream TLS handshakes failed because client certificate chain verification failed.",
	"envoy_listener_ssl_fail_verify_no_cert": "Downstream TLS handshakes failed because no client certificate was presented where one was required.",
```

⚠️ **The `fail_verify_error` HELP text must say *"certificate chain verification failed"*, NOT *"client certificate rejected"*** — the wording is load-bearing (a cert/private-key mismatch and a malformed DER land in `outcomeOther` and are counted nowhere). This is the same sentence B6 pins.

- [ ] **Step 3 — run the tests.** `go test ./internal/stats/ -count=1`. **Expected: PASS.**
- [ ] **Step 4 — hygiene + commit.** `gofmt -l internal/stats` · `go vet ./internal/stats/` · `golangci-lint run ./internal/stats/`. Commit.

**Commit:** `stats(phase 74 T5): three helpText entries for the phase-74 listener-scope ssl.* names — without them /stats/prometheus degrades to `# HELP envoy_listener_ssl_handshake envoy_listener_ssl_handshake`; the fail_verify_error text says "certificate chain verification failed", NOT "client certificate rejected", because a cert/key mismatch and a malformed DER never reach certs[0].Verify() and are counted nowhere. The map's own doc comment (":445-451, 11 entries") goes to 14 — a fourth stale-comment site the SPEC's roster missed`

---

## Task 6 — fixture `0111`: the `StatsAsserter` + the FOUR boundary-note retirements + the RD3 revision

**Files:**
- Modify: `test/fixtures/0111-tls-cvc-empty-dynamic-fallback/driver/driver.go` (`AssertStats` + a `0055`-shaped Prometheus scraper + `var _ fixture.StatsAsserter = (*edfDriver)(nil)` beside `:615-616` + the `:72-82` RD3 revision)
- Modify: `README.md` (`:100-110` RD3 · `:162-165` the `ssl.*` bullet) · `expectations.yaml` (`:109-119` RD3 · `:133-153` ADD leg (c) · `:177-184` · `:189`) · `envoy.yaml` (`:24-25`)
- **`envoy-go.yaml` stays BYTE-UNTOUCHED.** **NO new YAML, NO new directory, NO new BackendKind, NO new port. Fixtures stay 119.**

**Interfaces:**
- Produces: `func (d *edfDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` — **exactly** this signature (RD-STATSASSERTER, verbatim from `fixture.go:76`).
- Consumes: the two admin addresses and nothing else. **There is no `fixture.StatsArgs`; listener addresses are NOT passed** (F6) — and Shape A needs neither, so **no new driver field and no mutex are required.**

**Entry state:** T1–T5 landed; `go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback'` green.

**⚠️ The single most important guard in this task, stated first.** `var _ fixture.StatsAsserter = (*edfDriver)(nil)` is **MANDATORY**. Dispatch is a silent type assertion (`runner_test.go:1347-1348`) with **no `else`, no log, no skip message**; a signature typo (`*testing.T` instead of `fixture.TB`, a returned `error`, wrong parameter order, a misspelled method) makes `ok == false` and the branch **never fires**, and **the compiler, `go vet` AND `golangci-lint` are ALL silent**. `0111` today carries only `var _ fixture.Driver = (*edfDriver)(nil)`.

**Shape — DECIDED, do not re-open** (SPEC §8.1), with its anchors re-derived:
- **Scrape `/stats/prometheus`, not flat `/stats`.** A flat cross-side comparison is impossible by NAME: reference `listener.0.0.0.0_10447.ssl.handshake` vs subject `listener.0_0_0_0_<runner-allocated-port>.ssl.handshake` — different normalization (RD-NORMADDR) **and** a per-run port the driver discards (`driver.go:317`). Both sides hoist the address into a LABEL and leave the metric name address-free (RD-SN3), so both flatten to **`envoy_listener_ssl_handshake{envoy_listener_address=…}`** — **name-identical, differing only in the label value**. ⇒ **key on the metric NAME, IGNORE `envoy_listener_address`**, exactly as `0005/driver/driver.go:537-541` does for `envoy_listener_downstream_cx_total`.
- **Shape A (scrape once, absolute counts).** Justified because nothing pre-moves `l_edf`'s `ssl.*`, and now confirmed by run-step ORDER (RD-RUNORDER): `AssertStats` is step 10, strictly after both Drives and `CompareBytes`. Reference readiness polls admin `9901`, not the TLS port; subject readiness parses a stdout sentinel; `startSubjectWithRetry` retries with a fresh process (stats reset); the three arms are the only connections to `l_edf`. ⇒ deterministic **3 accepts / 1 success / 2 rejections per side**.
- **Expected per side:** `ssl.handshake` **1** · `ssl.fail_verify_error` **1** (arm 2, the forced-send untrusted cert) · `ssl.fail_verify_no_cert` **1** (arm 3). **Cross-side EQUAL on all three.**
- ⚠️ **The assertion MUST stay confined to `listener.<addr>.ssl.*`** and must not touch SDS or `sds_cluster` scopes, which are reconnecting against a closed port during step 10.
- ⚠️ **F8: no `stat_prefix` edit is owed.** `0111`'s `envoy.yaml` has no listener-level `stat_prefix` (its only one is the HCM's `ingress_edf` at `:101`), so SPEC §3.1's constraint is already satisfied by construction.

- [ ] **Step 1 — write `AssertStats` (red-first is INVERTED here: it should pass, so prove it CAN fail via Break G).**

```go
// Compile-time interface assertions. The StatsAsserter one is MANDATORY:
// runner_test.go:1347 dispatches via a SILENT type assertion with no else
// branch, so a signature typo makes the whole assertion never run while the
// compiler, go vet and golangci-lint all stay quiet.
var _ fixture.Driver = (*edfDriver)(nil)
var _ fixture.StatsAsserter = (*edfDriver)(nil)

func (d *edfDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	ref, err := scrapeProm(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus: %v", err)   // broken PRECONDITION -> Fatalf
	}
	subj, err := scrapeProm(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus: %v", err)
	}

	// Decode-ran guard (the 0097/driver.go:687-690 shape): if the reference
	// never accepted a connection on l_edf, every assertion below would be
	// vacuous. A broken precondition, so Fatalf.
	if ref["envoy_listener_downstream_cx_total"] == 0 {
		t.Fatalf("reference did NOT accept on l_edf: envoy_listener_downstream_cx_total == 0")
	}

	names := []string{
		"envoy_listener_ssl_handshake",
		"envoy_listener_ssl_fail_verify_error",
		"envoy_listener_ssl_fail_verify_no_cert",
	}
	want := map[string]uint64{
		"envoy_listener_ssl_handshake":           1, // arm 1, the trusted client cert
		"envoy_listener_ssl_fail_verify_error":   1, // arm 2, the FORCED-SEND untrusted cert
		"envoy_listener_ssl_fail_verify_no_cert": 1, // arm 3, no client cert
	}
	for _, n := range names {
		refVal, refOK := ref[n]
		subjVal, subjOK := subj[n]
		// ⚠️ THE ABSENT CHECK IS SEPARATE FROM THE VALUE CHECK, and `continue`s.
		// For a row whose entire purpose is ADDING counters this is the single
		// most important guard in the fixture: a counter that fails to REGISTER
		// reads as 0 == 0 and would pass VACUOUSLY. (The 0055/driver.go:655-669
		// shape. Note 0005's struct-snapshot parser defaults absent names to
		// zero — exactly the vacuity being guarded against here.)
		if !refOK {
			t.Errorf("ref: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if !subjOK {
			t.Errorf("subj: %s ABSENT from /stats/prometheus", n)
			continue
		}
		if refVal != want[n] {
			t.Errorf("ref %s = %d, want %d", n, refVal, want[n])
		}
		if subjVal != want[n] {
			t.Errorf("subj %s = %d, want %d", n, subjVal, want[n])
		}
		// ⚠️ ENTAILED, and kept deliberately as a redundant tripwire: given
		// ABSOLUTE want values, this cannot fire unless one of the two checks
		// above already did (V2 verified it stays green under both Break G and
		// Break H). It is three lines, it survives a future refactor to per-side
		// want values, and it makes the cross-side claim legible at the call
		// site -- but do NOT report it as an independently-firing property.
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", n, refVal, subjVal)
		}
	}
}
```

`scrapeProm(adminAddr)` is a file-local `map[string]uint64` scraper over `http://<addr>/stats/prometheus`: skip `#` lines, split `name{labels} value` (labels optional), **strip the `{...}` entirely — the `envoy_listener_address` label is DELIBERATELY ignored** (RD-NORMADDR/RD-SN3), sum on collision (there is only one listener here, but summing makes the address-agnostic keying explicit rather than accidental). Base it on `0055/driver.go:795-824`'s `scrapeProm`, whose URL construction and error handling are already the house shape; `0005/driver.go:427`'s `parseMetricLine` handles the labelled/bare/timestamped variants. **New driver imports: `net/http`, `strconv`, `strings` (already present), `bufio` or `bytes` (already present).**

⚠️ **`t.Errorf` per property; `t.Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`) — the two scrapes and the decode-ran guard are preconditions; every counter comparison is a property. ⚠️ **`fixture.TB` has exactly Errorf/Fatalf/Helper — no `Logf`, no `Cleanup`** (`reference_fixture_tb_has_no_logf`); record diagnostics with `log.Printf`. Shape A opens no connections, so no `defer` guard is needed (contrast `0055/driver.go:613-625`).

- [ ] **Step 2 — retire the FOUR boundary notes, KEEPING the still-live guards.**

⚠️ **EACH note BUNDLES a retirable half with a STILL-LIVE guard. A blanket delete drops a live guard.**

| site | retire | **KEEP** |
|---|---|---|
| `README.md:162-165` | *"No `ssl.*` stats — envoy-go emits none, so a verdict `StatsAsserter` is infeasible (inherits PLAN-65 C3); a pre-existing framework gap."* | *"Never assert `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as listener liveness."* |
| `expectations.yaml:177-184` | *"The ssl.\* stat family … envoy-go emits NO ssl.\* stats whatsoever. A verdict StatsAsserter is therefore INFEASIBLE (inherits PLAN-65 C3). A PRE-EXISTING framework gap."* | the whole `/listeners` + `total_listeners_active` + docker-proxy-accept clause, **and** *"the accept/reject CONTRAST … is strictly STRONGER than a subject-only stat — it is cross-side"* (which stays true and now has a cross-side stat beside it) |
| `expectations.yaml:189` | *"The `sds.<secret>.*` stat counters (no StatsAsserter)"* — the parenthetical becomes false | the `sds.<secret>.*` counters themselves remain unasserted; reword to *"(not asserted by this fixture's StatsAsserter, which is confined to `listener.<addr>.ssl.*`)"* |
| `envoy.yaml:24-25` | *"and NOT a stat (envoy-go emits no `ssl.*` family; see README)"* | the rest of the sentence — the observable IS still the normalized three-arm verdict, **now additionally** cross-checked at the counter layer |

Also **ADD a third leg (c) to `expectations.yaml:133-153`'s "## Asserted" block**, which today enumerates exactly (a) per-side structural and (b) cross-side `CompareBytes`:

> ```
> #   (c) CROSS-SIDE, at the STAT layer (the driver's AssertStats, runner step 10):
> #       listener.<addr>.ssl.{handshake,fail_verify_error,fail_verify_no_cert}
> #       each read exactly 1 on BOTH sides, scraped from /stats/prometheus and
> #       keyed on the metric NAME with envoy_listener_address IGNORED (the two
> #       sides normalize the listener address differently — the reference keeps
> #       dots, envoy-go underscores them — but both hoist it into a LABEL, so the
> #       metric NAME is cross-side identical; the 0005 precedent).
> #       The ABSENT check is separate from the value check: a counter that fails
> #       to REGISTER would read 0 == 0 and pass vacuously.
> ```

- [ ] **Step 3 — revise the RD3 disclaimer at THREE prose sites: it INVERTS at the stats layer.** `driver.go:72-82`, `README.md:100-110`, `expectations.yaml:109-119` all currently say, in substance, *"at require=true the forced-send is NOT the observable's discriminator … **Do NOT claim forced-send flips the observable here.**"* That remains TRUE **at the byte observable** — both negative arms still read `rejected`. **But at the `ssl.*` COUNTER layer arms 2 and 3 hit DIFFERENT counters** (`fail_verify_error` vs `fail_verify_no_cert`), and the Go-side control proved that without forced-send arm 2 **degrades into arm 3**. ⇒ **phase 74 UPGRADES the forced-send from "retained for symmetry" to LOAD-BEARING.** Revise all three — do not merely leave them standing, and do not delete the byte-layer half, which is still correct.
- [ ] **Step 4 — run the fixture.** `go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback' -v` (`reference_differential_run_selector` — a bare `-run '0111'` matches ZERO subtests and reports a vacuous green). **Expected: PASS.**
- [ ] **Step 5 — hygiene + commit.** `gofmt -l` on the driver package · `go vet` · `golangci-lint run` on it. Commit.
- [ ] **Step 6 — breaks (AFTER committing, `-count=1`).**
  - **Break G [break one asserted counter]:** change `"envoy_listener_ssl_fail_verify_error": 1` to `: 2` in `want` ⇒ **the fixture FIRES.** ⚠️ **CONFIRM IT IS THE STATS ASSERTION THAT FIRED, not a `CompareBytes` mismatch** — the failure text must be `ref envoy_listener_ssl_fail_verify_error = 1, want 2` (and its subj twin), NOT an admin/byte-diff hex dump. `git restore`; re-green.
  - **Break G′ [THE DISPATCH BREAK — prove `AssertStats` runs at all]:** comment out `var _ fixture.StatsAsserter = (*edfDriver)(nil)` **and** rename the method to `AssertStatsX` ⇒ **the fixture must stay GREEN**, proving the assertion silently vanished. **That green is the finding**, and it is the whole reason the compile-time assertion is mandatory. Then restore ONLY the method name, leaving the `var _` commented out ⇒ green again with the assertion live (the `var _` is a tripwire, not the dispatch mechanism). Restore both. *(Discriminates: G alone proves the assertion can fail; G′ proves it is actually being invoked — without G′, a passing G is consistent with a fixture that never calls it.)* `[NOT pre-compiled — substitution rule applies]`
  - **Break H [drop the forced-send on the untrusted arm, ONE SIDE ONLY]:** ⚠️ **Do NOT simply delete the `sendForced` case — that variant is broken two ways and V2 proved both.** `mtlsEcho` (`driver.go:494`) has **no `side` parameter** (both sides route through `driveSide`, `:393`), so deleting forced-send is **symmetric** and the cross-side assertion could never fire; and `clientCertMode` has only `sendForced` (`:82`) and `sendNone` (`:87`), with `:517-522` the **only** place any cert reaches the wire — so deleting it also strips the **trusted** arm (`:409`), which then fails `structuralCheck` (`:441`), so `DriveReference` errors, the runner `t.Fatalf`s **in the drive**, and `AssertStats` (step 10) **never runs at all**.
    **Write it instead as:** ADD a third `clientCertMode` member `sendPolite` that sets `cfg.Certificates` **without** `GetClientCertificate`, and route **only the untrusted arm, and only when `side == "subject"`**, through it. **Predicted firing set:** on the subject side `fail_verify_error` falls to **0** and `fail_verify_no_cert` rises to **2**, so the subject value checks fire for **both** those names **and** the (entailed) cross-side check now fires too — **while `CompareBytes` stays GREEN**, since both negative arms still read `rejected`. `git restore`; re-green. `[NOT pre-compiled — substitution rule applies]` *(Discriminates: this is the row's best fixture finding made executable — the proof that the RD3 disclaimer genuinely INVERTS at the stats layer, and the reason Step 3's revision is owed rather than optional. It is also the only break in the row that makes the cross-side leg fire on its own.)*

**Commit:** `test(phase 74 T6): give 0111 the cross-side StatsAsserter its own README confessed was infeasible — Shape A over /stats/prometheus keyed on the metric NAME with envoy_listener_address IGNORED (both sides hoist the address into a LABEL, so the name is cross-side identical even though the reference keeps dots and envoy-go underscores them), with the MANDATORY `var _ fixture.StatsAsserter` compile-time assertion (dispatch is a silent type assertion; compiler, vet AND lint are all quiet) and the ABSENT check SEPARATE from the value check (an unregistered counter reads 0 == 0 and passes vacuously). All FOUR boundary notes retired WITHOUT dropping the /listeners guard each one bundles, and the RD3 forced-send disclaimer REVISED because it INVERTS at the counter layer: arms 2 and 3 hit different counters, so forced-send is now load-bearing. +0 fixtures (119), no new YAML, no new directory`

---

## Task 7 — BEHAVIOR_CONTRACT delta (B1–B8) — pinned

**Files:** `docs/envoy-go/BEHAVIOR_CONTRACT.md` (5730 lines at tip)

**Entry state:** T1–T6 landed and green.

⚠️ **Every anchor below was re-derived at `ab13fc19` (RD-BC/RD-1200/RD-LEDGER). Re-derive again at the IMPL tip before any of them becomes an `old_string`** — and note the region's structure is `:926` rejects para · `:927` blank · `:928` the boundary · `:929` blank · `:930` `**Differential coverage.**`, so `:928` is its own physical line and its own `old_string`.

- [ ] **Step 1 — B1 (`:928`), the C3 coverage boundary: RETIRE for the three fixed names, RESTATE for the dynamic half.** ⚠️ Its current text says `listener.<name>.ssl.*` — **the scope segment is the normalized bind ADDRESS, not the listener name; fix that in the same edit.** The restated boundary must name **FOUR** surviving families (`ciphers` / `versions` / **`curves`** / **`sigalgs`** — the last two were never named in any prior document) and both blockers: **(1) charset** — `NamePattern` (`registry.go:48`) has **no hyphen**, so the OpenSSL form `ECDHE-RSA-AES128-GCM-SHA256` fails `IsValidName` and `checkName` **PANICS at `registry.go:117`** (⚠️ **cite `:117`, not `:107` — RD-CHECKNAME; `:107` is the duplicate-registration panic**); **(2) name mismatch** — Go's `tls.CipherSuiteName` yields IANA forms, Envoy yields OpenSSL forms, so a cross-side exact assertion needs a hand-maintained table whose mapped names then trip blocker 1. **State explicitly that LIFECYCLE is NOT a blocker** (`NewCounterIfAbsent` routes through `getOrRegister`, whose `:175-176` comment says post-Freeze registration is *"PERMITTED post-Freeze by design (ADR-0117)"*) — **the blocker is NAMING.**
- [ ] **Step 2 — B2 through B6.**
  - **B2 (`:916`)** — after *"envoy-go's accept/reject decisions match"*, ADD that envoy-go now **emits these three counters too**, and that fixture `0111` asserts them cross-side.
  - **B3 (`:918`)** — the phase-67/ADR-0289 init-hold departure says *"no TLS alert; no `ssl.*` movement"*. Its implicit "envoy-go has none anyway" framing needs a re-read now that envoy-go emits `ssl.*`: the reference's `downstream_context_secrets_not_ready` path still moves no `ssl.*`, and envoy-go still boot-FAILS, so the departure is unchanged — **say so explicitly rather than leaving the reader to infer it.**
  - **B4 — the QUIC coverage boundary, as SHARED PARITY.** ⚠️ **Do NOT write this as an envoy-go departure — that was the SPEC's own first, refuted reading.** A QUIC listener registers all three and they stay permanently ZERO, **and the reference does exactly the same** (EXECUTED: `ssl.handshake: 0` after five successful H3 connections; `connection_error: 0` on a failure arm where the TCP comparator in the same process and the same scrape fired). State the mechanism (quic-go's `Accept` returns post-handshake, `quic.go:84-85`/`:109`; `serveConnection` is unreachable for QUIC because `Start`'s launch loop `continue`s at `:997-1001`), state that it is PARITY, and name `0104-http3-downstream-get` as the site for any future closure.
  - **B5 — the `other`-bucket departure.** The reference books non-cert handshake failures under **`ssl.connection_error`**; envoy-go's `outcomeOther` increments **NOTHING**. Record the two EXECUTED arms (a TLS-1.0-only client ⇒ alert 70; a plaintext HTTP request to the TLS port) **and** the explicit note that `connection_error`'s full membership is **unenumerated**, which is exactly why the row stayed at three names — a counter whose membership cannot be enumerated cannot be cross-side asserted honestly.
  - **B6 — the classifier's exact semantics.** `ssl.fail_verify_error` means ***"certificate chain verification failed"***, **NOT** *"client cert rejected"*: a cert/private-key mismatch (`tls: invalid signature by the client certificate`) and a malformed DER (`tls: failed to parse client certificate`) never reach `certs[0].Verify()` and land in `other`. **The wording is load-bearing.** ⚠️ **Also record the ctx-override conditionality in its CORRECTED form** (RD-CTXBOUND): *"no production deadline REACHES `HandshakeContext`"* — the one production `context.WithTimeout` under `internal/listener/` (`listenerfilter/pipeline.go:43`) cannot escape because `Pipeline.Run` returns only an `error` and `serveConnection` never rebinds `ctx`; the ctx that arrives is `main.go:339`'s cancel-only `signal.NotifyContext`. **Do NOT write "there are no production deadlines in `internal/listener/`" — that is false.** Cite the override at `crypto/tls/conn.go:1541-1546` (**not `:1539-1547`** — RD-CONNLINES).
- [ ] **Step 3 — B7, the stat-surface ledger. This is bigger than "change 1201 to 1204".**
  - `1201` → **1204** at **exactly two** lines: **`:831`** and **`:847`** (`grep -n '1201'` returns these two and no others).
  - The **three narrative stale `1200`s** at **`:1429`**, **`:1463`**, **`:1495`** — each asserts a *current* surface (*"stat surface STAYS 1200"*) and is now wrong by four. Update or annotate.
  - ⚠️ **DO NOT touch the other ten `\b1200\b` hits** (`:732`, `:757`, `:763`, `:767`, `:779`, `:795`, `:805`, `:815`, `:4986`, `:4988`) — **they are HISTORICALLY CORRECT statements about their own phases**, and `:4986`/`:4988` are LEDGER lines (`Phase 47.1 — 1200 → 1200`, `Phase 51 — 1200 → 1200`). The SPEC's "three stale 1200s" is right about the three; a sweep would corrupt ten correct lines (RD-1200).
  - **ADD a ledger line** `**Phase 74 — 1201 → 1204 (+3)**` **immediately after `:4988`** (the ledger tail) and **before `:4990`** (`### Forward-pointer note (26.3)`).
  - ⚠️ **The `1200 → 1201` hole: RECORD it, do NOT invent it.** There is no `1200 → 1201` step anywhere in the file, and `1201` never appears in the ledger at all; the `+1` is attributable on documentary grounds to the tap filter's `tap.rq_tapped` (phase 56.1, `:4155-4161`) but **no ledger line records it and no probe confirmed it.** Add a bracketed note beside the new phase-74 line stating that the ledger jumps from `Phase 51 — 1200 → 1200` to a bare narrative `1201`, with the gap unattributed. **Fabricating a `Phase 56.1 — 1200 → 1201` line would be inventing a record.**
- [ ] **Step 4 — B8, the three cross-side scope divergences**, recorded ONCE so a future asserter author does not rediscover them: **(i) dots** — the reference emits `listener.0.0.0.0_10447.*`, envoy-go `listener.0_0_0_0_<port>.*` (`normalizeAddr`, `manager.go:342`, deliberate and documented at `:326-332` because SN3's `strings.Index(tail, ".")` would otherwise truncate the address to its first octet); **(ii) IPv6 brackets** — the reference RETAINS them (`listener.[__]_10002.*`), envoy-go STRIPS them (`[::]:45259` → `___45259`); **(iii) `Listener.stat_prefix`** — the reference honours it and **drops the address from the scope entirely** (`listener.MYSTATPREFIX.ssl.handshake`), while envoy-go has **ZERO `GetStatPrefix()` consumers** in `internal/listener/` or `internal/bootstrap/` (accepted-and-silently-ignored). **All three are PRE-EXISTING and all three already affect the landed `downstream_cx_total`** — none is introduced by this row.
- [ ] **Step 5 — verify + commit.** ⚠️ **`grep -n '1201' … ⇒ 0` IS THE WRONG GATE and an earlier draft used it** — Step 3 mandates a ledger line reading `Phase 74 — **1201** → 1204`, so zero hits is unsatisfiable by this task's own instruction (V2-M1). Use:

```bash
grep -c '1201' docs/envoy-go/BEHAVIOR_CONTRACT.md            # exactly 1 — the NEW ledger line only
awk 'NR<4980' docs/envoy-go/BEHAVIOR_CONTRACT.md | grep -c '1201'   # 0 — the two narrative sites are gone
grep -c '1204' docs/envoy-go/BEHAVIOR_CONTRACT.md            # >= 3 — :831, :847, the ledger line
grep -cE '\b1200\b' docs/envoy-go/BEHAVIOR_CONTRACT.md      # 10 — was 13; the THREE narrative stale ones are gone
```
and confirm the file's line count changed only by the lines deliberately added. Commit.

**Commit:** `docs(phase 74 T7): BEHAVIOR_CONTRACT B1-B8 — RETIRE three-fifths of ADR-0286 C3's coverage boundary and RESTATE the rest over FOUR surviving dynamic families (ciphers/versions/curves/sigalgs) with both blockers named (charset PANIC at registry.go:117, not :107; IANA-vs-OpenSSL naming) and lifecycle explicitly NOT a blocker. B4 records QUIC as SHARED PARITY, not a departure — the SPEC's own first reading of it was refuted by probe. B7 is bigger than "1201 -> 1204": three NARRATIVE stale 1200s are corrected while TEN historically-correct ones are deliberately left alone, a ledger line is added after the :4988 tail, and the missing 1200 -> 1201 step is RECORDED as an unattributed gap rather than invented`

---

## Task 8 — VERIFY: the six-gate + layering + full differential + `-race` + counts + envelope audit

**Files:** none (a gate run at the frozen HEAD). ⚠️ **Run every command with `git -C <abs-worktree-path>` and tripwire `pwd` + `git rev-parse --abbrev-ref HEAD` (must be the stage branch, NEVER `master`) + `git rev-list --count <base>..HEAD` first** (`reference_bash_cwd_reset_commits_to_main` — at the phase-72 close this exact hazard made a "final gates" run validate master's pre-row tree: GREEN while proving nothing).

- [ ] **Step 1 — the six gate.** `gofmt -l .` SILENT · `go vet ./...` clean · `go build ./...` clean · `go mod tidy -diff` **EMPTY** and `git diff master -- go.mod go.sum` **EMPTY** (⇒ **+0 modules**) · `golangci-lint run ./...` clean · **the FULL 119-directory differential** `go test ./test/differential/ -count=1`.
- [ ] **Step 2 — `-race`.** `go test ./internal/listener/ ./internal/stats/ -race -count=1`, then `go test ./... -race -count=1`. ⚠️ **Known PRE-EXISTING flakes — isolate-re-run, do NOT re-classify:** `internal/cluster` `TestOutlierDetector_ConcurrentEjectExactlyOnce` (`outlier_test.go:766`), the full-suite startup flake (`subject ready: EOF` on an UNRELATED fixture), the SDS `init_fetch_timeout` dial-budget flake, and the two unindexed load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`).
- [ ] **Step 3 — the layering check.** `go list -deps ./internal/listener` (⚠️ **no `...`** — `reference_xds_config_seam_transitive_cycle_guard`) and confirm no new edge appeared. `internal/listener` already imported `internal/stats` (`manager.go:30`), so this is DISCIPLINE, not a cycle check.
- [ ] **Step 4 — the ENVELOPE AUDIT, in TWO SEPARATE CATEGORIES** (F1):
  - **PRODUCTION: +0 imports.** `git diff master -- internal/listener/manager.go internal/stats/name.go | grep -E '^\+' | grep -E '^\+\s*(_|[a-z]+ )?"'` ⇒ **EMPTY**. Equivalently: neither file's diff contains an import-block hunk.
  - **TEST: imports DO grow, and that is permitted.** The new PKI generator adds `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509/pkix`, `encoding/pem`, `math/big` to `manager_test.go`, and the driver adds `net/http`/`strconv`. ⚠️ **Also `sort` and `reflect`** — T2's `listenerSSLNames` uses `sort.Strings` and T2/T3 use `reflect.DeepEqual`, and **neither is in `manager_test.go`'s import block (`:3-12`) and `reflect` is not in `quic_test.go`'s (`:3-18`)** (V2-Mo6). **Record all of them explicitly** so the +0 claim is unambiguous rather than merely unstated.
- [ ] **Step 5 — the counts, re-run MECHANICALLY (never copied).**

```bash
ls -d test/fixtures/[0-9]*/ | wc -l                                  # 119 (+0)
grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l             # 55  (+0)
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1       # ## ADR-0296
grep -c '^## ADR-0297' docs/envoy-go/DECISIONS.md                    # 0
grep -c '1204' docs/envoy-go/BEHAVIOR_CONTRACT.md                    # >= 3 hits
grep -c '1201' docs/envoy-go/BEHAVIOR_CONTRACT.md                    # exactly 1 — the NEW ledger line ONLY
awk 'NR<4980' docs/envoy-go/BEHAVIOR_CONTRACT.md | grep -c '1201'    # 0 — see T7 Step 5 (V2-M1)
```
**ZERO new exported symbols — assert it with a command, not by inspection** (the phase-73 precedent): `go doc -all ./internal/listener` and `go doc -all ./internal/stats`, set-differenced against the same output from `master`, must be EMPTY.
BackendKind tail **38** (`fixture.go:614`) · go.mod modules **2** (lineage figure) · stat surface **1201 → 1204** (doc count; **no mechanical command**) · **ZERO new packages, ZERO new exported symbols.**
- [ ] **Step 6 — the BYTE-UNTOUCHED sha256 assertions.** For each path in the Global Constraints list, compare `git show master:<path> | sha256sum` against the worktree file. ⚠️ **`internal/listener/manager_test.go` is NOT on this list — it is EDITED by T1/T2/T4** (`reference_plan_schedules_edits_to_a_byte_gated_file`; set-difference the EDIT roster against the sha256 roster before running, and report the set-difference as part of this step rather than assuming it is empty). ⚠️ **`internal/stats/name.go` is NOT on it either** — T5 edits it.
- [ ] **Step 7 — record everything in `PROGRESS.md`**, including any gate that needed an isolate-re-run and WHY it was classified pre-existing.

---

## Task 9 — ADR-0296 completed IN PLACE + the ADR-0286 C3 correction + ROADMAP + stage close

**Files:** `docs/envoy-go/DECISIONS.md` · `docs/envoy-go/ROADMAP.md` · `docs/envoy-go/STATE.md` · `next-prompt.txt` · `PROGRESS.md`

- [ ] **Step 1 — complete ADR-0296 IN PLACE.** **APPEND** `### Decision (landed at the phase-74 IMPL)` then `### Consequences (landed at the phase-74 IMPL)` **after the RETAINED footer at `:17280`** (mirroring ADR-0295's `:17229` footer → `:17231` Decision → `:17243` Consequences; mirror **ADR-0295/0287, NOT ADR-0286**, which predates the heading format and uses inline `**§Context.**` bolds). Verify `### Decision` inside the block reads **0 before / 1 after**. Flip the `:17256` STATUS from **PROPOSED** to `COMPLETE — landed at the phase-74 IMPL.` **No renumber; tail stays ADR-0296; next-free ADR-0297.**

  **§Decision + §Consequences must record:** (a) the gate is **`rt.tlsMode` ALONE**, and why (§3.4 parity, `startQUIC`'s hard-error); (b) the classifier fragility — **no exported sentinel, string match FORCED, live-handshake test MANDATORY**, and the **conditional** ctx-override accuracy **in its corrected form** (RD-CTXBOUND: *"no production deadline REACHES `HandshakeContext`"*, guarded by `Pipeline.Run`'s single-return signature — **not** *"there are no production deadlines"*); (c) the family SPLIT and why the blocker is **NAMING, not lifecycle**; (d) why breaking the +0-stat streak is correct (an envelope is not an argument); (e) the ONE new named departure — the `other` bucket — and, **distinctly**, the QUIC coverage boundary as **SHARED PARITY**, including that this SPEC's own first reading of it was refuted by probe; (f) the C3 misattribution and its correct provenance; **(g) ⚠️ the `ssl.no_certificate` NAMING TRAP** — SPEC §13.1 asserts every deferred item is *"recorded HERE **and in ADR-0296 §Context**"*, but `awk 'NR>=17254&&NR<=17280' docs/envoy-go/DECISIONS.md | grep -c 'no_certificate'` ⇒ **0** (V2-Mo8): it is in **neither** the ADR **nor** any B-step. Record it here — at `require_client_certificate: false` a no-cert client yields `handshake: 1` **AND** `no_certificate: 1` with all four `fail_verify_*` at 0, so **`no_certificate` is a SUCCESS-PATH annotation and treating it as a synonym for `fail_verify_no_cert` is wrong in BOTH directions**; the LANDED record already names it at `BEHAVIOR_CONTRACT.md:962`.
- [ ] **Step 2 — fix ADR-0296's two internal defects** (RD-ADR0296-POINTER, RD-GREP0): the `:17256` STATUS says *"see §Context ¶6"* but the ADR-0044 misattribution is at **¶4(i) (`:17266`)** — ¶6 (`:17270`) is the classifier paragraph; and ¶3 (`:17264`) asserts `grep -c 'VerifyPeerCertificate\|handshake-error callback'` ⇒ **0**, which **the SPEC's own append made false — it is now 1, the sole hit being that very paragraph.** ⇒ **restate ¶3 with NO number, or scope the grep to ADR-0286's line range** (`awk 'NR>=16888 && NR<16930' … | grep -c …` ⇒ 0). The finding's substance is unchanged; only its self-refuting instrumentation is.
- [ ] **Step 3 — the ADR-0286 C3 correction, as an INDENTED BLOCKQUOTE.** Add immediately after C3 (`:16908`), copying `DECISIONS.md:16901`'s form **exactly — two literal spaces, then `> [`** (RD-CORRFORM; the INLINE `**[CORRECTED …]**` form at `:17185`/`:17209` is for a clause within the *same* ADR family, which this is not):

  `  > [CORRECTED at phase 74/ADR-0296: …]`

  It must record: (i) that C3's *"adding a stat family is a framework-surgery row of its own"* is **REFUTED** — the three fixed names are an inline add to ONE function in ONE file, gated on a field (`listenerRuntime.tlsMode`) that already existed; (ii) that the *"opaque `crypto/tls` handshake-error callback wired into every per-chain `*stdtls.Config`"* half **was never in C3, nor in `DECISIONS.md` at all** — it originated in phase-70 BRAINSTORM prose and was upgraded to *"CONFIRMED"* by phase 72 (⚠️ **state this without the self-refuting grep count**, RD-GREP0); (iii) that C3's *"would blow the +0-stat envelope"* is a budget statement, not an argument; and (iv) **closing with what STILL STANDS: the `ciphers`/`versions` half — now known to be a FOUR-family surface including `curves`/`sigalgs` — survives on its own two blockers.** ⚠️ **Phrase it as a documentary finding, NOT as criticism of phase 65, whose own text was accurate.**
- [ ] **Step 4 — ROADMAP: flip row 74 `done` and narrow the deferred sentence.**
  - **Row 74 (`:136`)**: status field `in-progress` → **`done`**; append the IMPL close-out to the cell. ⚠️ **The cell carries FOUR stale claims, not one** (RD-ROADMAP-FOUR + V2-M2 — and `:136` is **the FOURTH stale QUIC site**, the one the SPEC sweep does not reach because it is not in `SPEC.md`):
    1. ⚠️ *"leaving QUIC handshakes uncounted is a **DEPARTURE that must be written down**, not merely omitted"* — **REFUTED by SPEC §3.4. It is SHARED PARITY.** This is the same refuted reading RD-QUICSTALE tracks, carried forward into the very cell this step opens.
    2. *"three of five sub-names roll out, `ciphers`/`versions` SURVIVE"* — the surviving family is **FOUR** (`ciphers`/`versions`/`curves`/`sigalgs`).
    3. The cell is **already self-inconsistent** about that family: one clause lists `ciphers`/`versions`/`curves` (three), another says *"`ciphers`/`versions` SURVIVE"* (two). Make both read four.
    4. *"CROSS ADRs — a first for the project"* — **REFUTED by SPEC §1.1 D2**, which found three precedents, one of them already inside ADR-0286 itself.
  - **The narrow, on `:204`** (⚠️ **`:204`, the 42,342-char family-history line — NOT `:202`, which is the 92-char family descriptor**; RD-ROADMAP). Delete `handshake/fail_verify_error/fail_verify_no_cert/` from the parenthetical **and ADD `curves`/`sigalgs`** — and ⚠️ **REPLACE THE PROSE, not just the names.** After deleting three names the clause still asserts *"envoy-go emits **ZERO such stats**"* (now FALSE) and *"a **framework-surgery row, NOT an inline add**"* (the very adjective this row refutes). The SPEC verified in a scratch copy that check (2) **STAYS 3** and the `candidates:` marker survives — **but that mechanical check is not the whole edit.**
  - **Re-run check (2) after the edit:** `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` ⇒ **must still be 3**, and the sentence must still terminate at `force-trace.` so the `[^.]*\.` regex still binds.
- [ ] **Step 5 — the six-gate close.** Row 74 flips `done` at THIS point (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, `reference_roadmap_split_phase_row_done`). Roll STATE §Current **IN PLACE** (lifecycle 3 → DONE; §Recent re-capped at FIVE with its preamble updated; ⚠️ **the three ADR-0288 singleton greps return 2, never "fix" to 1**). Roll `next-prompt.txt` to the phase-75 BRAINSTORM (the roller SELF-PICKS per the 2026-07-12 standing directive).
- [ ] **Step 6 — memory writes.** Candidates surfaced by this lineage, each to be written only if it survives the "is this derivable from the repo?" test: **(a)** the nil-`*stats.Counter` + goroutine + no-`recover()` crash mode (F3) — this is a general envoy-go hazard, not phase-74-specific; **(b)** the `Pipeline.Run` single-return containment of the handshake-ctx hazard (RD-CTXBOUND), including that the code is one refactor away from it; **(c)** the "an ADR §Context append can make its own grep self-refuting" pattern (RD-GREP0), which generalizes `reference_a_drift_correction_is_itself_a_claim`.
- [ ] **Step 7 — the sentinel, run MECHANICALLY TWICE** — in the stage worktree AND on landed master after the squash-push (the phase-72/73/74 precedent: a stage worktree can disagree with what actually landed, and the post-push re-run is the one that counts). ⚠️ **Check (1) goes SILENT at this close** — row 74 is currently the ONLY non-`done` chartered row, so once it flips, check (1) prints nothing. **CONFIRMED BY EXECUTION at this PLAN**: V2 flipped row 74 to `done` in a scratch copy and check (1) printed nothing.
  ⚠️ **CARRY THE CHECK-(1) BLIND SPOT FORWARD — this row leans on check (1) harder than any before it, and an earlier draft dropped it.** The regex does **not** see every row: re-derived at this tip, **104 table rows, 102 matched**, the two misses being `| 00 | bootstrap | — | done |` (the `[0-9.,  ]*` field does not match an **em-dash** "after" column) and `| 04 | http-1.1 | 03 | done |` (the `[a-z0-9-]+` slug field does not match the **DOT**). Both are `done`, so there is no current impact — **but check (1) is not exhaustive**, and any future dotted slug, em-dash cell or letter-suffixed row number goes invisible too. *(The phase-73 close recorded four misses out of 105 including `28.1a`/`28.1b`; the figure moves with the table and must be RE-DERIVED, not copied.)* **Checks (2) and (3) must still print** (⇒ 3 live `candidates:` sentences; `NEVER OPENED: gRPC/Runtime/WASM`), so the sentinel still does **NOT** fire and `stop` is **NOT** created. **Create `stop` if and ONLY if all three print nothing.**

**Commit:** `docs(phase 74 T9): ROW 74 -> done. ADR-0296 COMPLETED IN PLACE (§Decision + §Consequences appended after the RETAINED footer; STATUS PROPOSED -> COMPLETE; tail stays ADR-0296) and ADR-0286 §Consequences C3 CORRECTED via an indented blockquote in the :16901 form — its "framework-surgery row of its own" refuted by an inline add to ONE function gated on a field that already existed, its "opaque handshake-error callback" half shown never to have been in C3 at all, and what STILL STANDS (a FOUR-family ciphers/versions/curves/sigalgs boundary on its own two blockers) stated in closing. The ROADMAP narrow REPLACES THE PROSE, not just three names: the surviving clause's "envoy-go emits ZERO such stats" is now false and its "framework-surgery row, NOT an inline add" is the very adjective this row refutes. Check (2) STAYS 3`

---

## Self-review against SPEC-74

*(Rewritten after the adversarial pass. V2 found the previous version's coverage walk skipped §1/§2/§13, its placeholder count understated by eight, and its break map missing two breaks. All three are corrected below.)*

**Spec coverage — the FULL walk, §1 through §16.**
**§1** (purpose; §1.1's drift ledger; §1.2's what-was-and-was-not-executed) → §1.1's RD-* ledger re-derives it independently and REFUTES twelve claims; §1.3 restates the not-executed list and adds what THIS stage did not do. **§2** (non-purposes) → Global Constraints (three names only, `other` counts nothing) + T7 B1's restated FOUR-family boundary + T9's narrow. **§3.1/§3.2** (spelling; the reference's 15-eager + 4-dynamic surface; the `no_certificate` trap) → T2 names, T7 B8, **T9 Step 1(g)**. **§3.3** (REGSCOPE, TLS-chains-only) → T2. **§3.4** (QUIC parity) → T2's gate, T3's test + Break D, T7 B4, T9 ADR. **§3.5** (SEMANTICS/ALERTMAP/CLASSIFY; the named fragility; the three mis-classifications; the `other` departure) → T1, T4, T5's HELP wording, T7 B5/B6, T9 ADR. **§3.6** (the SPLIT, its two blockers, lifecycle explicitly NOT one) → T7 B1, T9 ADR + narrow. **§3.7** (VERIFYIFGIVEN) → explicitly OUT OF SCOPE; recorded, no task, no `require=false` behavior, no `no_certificate` counter. **§4/§5** (primitives; identifier hygiene) → Global Constraints, T1 Step 0, T8 Step 4. **§6** (+0 rejects/fuzzers) → T8 Step 5. **§7** (+3 stats; the `helpText` gap) → T5, T7 B7. **§8** (the `0111` asserter, its FOUR boundary sites, RD3, the mandatory guards, the `-run` selector) → T6. **§9** (B1–B8) → T7. **§10** (the ~9-task spine; breaks A–G) → T1–T9. **§10.1** (the NAME-SET guard) → T2. **§11** (edit-site roster + BYTE-UNTOUCHED) → File structure, T8 Step 6. **§12** (the narrow, at the IMPL only) → T9 Step 4. **§13/§13.1** (deferred items; the "recorded in ADR-0296 §Context" hygiene claim) → T9 Step 1, **including the `ssl.no_certificate` gap V2 found in neither the ADR nor any B-step**. **§14** (ADR-0296 + the C3 correction) → T9 Steps 1–3. **§15** (counts) → T8 Step 5. **§16** (the adversarial record, incl. the SPEC's reversal of its own QUIC decision) → §1.1 RD-QUICSTALE, T3, T7 B4.

**Break map — TWELVE breaks, each defined once and assigned to exactly one task.** T1: **A** (`other` collapsed), **F** (hand-written string — MUST NOT fire), **F′** (the mutation control). T2: **E** (gate dropped), **E′** (name misspelled). T3: **D** (the refuted kind gate ADDED — the row's distinguishing break) and **E, second half** (run where both tests exist, to check the QUIC test does NOT also fire). T4: **B** (arms swapped), **C** (Inc outside the TLS block — a PROCESS CRASH), **C′** (the non-crashing variant, only if C's attribution is ambiguous). T6: **G** (a broken counter), **G′** (the dispatch break — proves `AssertStats` runs at all), **H** (forced-send dropped, one side). **Beyond the SPEC's A–G this adds FIVE: C′, E′, F′, G′, H.**

**Deviations from the SPEC's §10 spine, each deliberate and justified above:**
1. **The gate is `rt.tlsMode` ALONE** — SPEC §10 item 2's `rt.tlsMode && rt.kind != kindQUIC` is the REFUTED first reading, contradicted by the SPEC's own §3.4/§16 and by ADR-0296 §Context ¶8(ii). **RD-QUICSTALE**, independently confirmed by V2 as correct and exhaustive within `SPEC.md` — with the FOURTH site outside it, at `ROADMAP.md:136`.
2. **The SPEC §3.5 table's names become `outcomeOK`/`outcomeVerifyError`/`outcomeNoCert`/`outcomeOther`.** ⚠️ **This is a READABILITY choice, NOT a constraint — V1 verified both spellings compile:** with the bare `ok`/`verifyError`/`noCert`/`other`, `go vet` rc=0, `golangci-lint` rc=0, tests green. A package-level `ok` is legally shadowed by every `v, ok := m[k]`. The deviation stands because a shadowed package-level `ok` in a 1300-line file is a trap, not because the compiler forbids it.
3. **Five breaks beyond the SPEC's A–G** (C′, E′, F′, G′, H) — F alone is not evidence without its mutation control; G alone is consistent with an `AssertStats` that never runs; E′ is what shows a cardinality guard would miss a misspelling; C′ is the escape hatch if C's process crash obscures attribution; H makes the RD3 inversion executable.
4. **The ctx-override bound is RESTATED, not repeated** — the SPEC's premise is false (RD-CTXBOUND); the conclusion survives on the stronger `Pipeline.Run` single-return ground.
5. **T3 asserts only the cx COUNTER, not the gauge** — RD-QUICTEST: the claimed existing pin covers only `downstream_cx_total`.
6. **Four new test-side helpers the SPEC's spine does not mention** (`handshakeTestPKI`+`mkTestPKI`, `connPair`, `mkDownstreamTSMutualTLS`, `counterValue`) — F1/F2/RD-QUICTEST: `internal/listener`'s corpus has no mTLS PKI, no client cert, no CA key and no gauge poller; its one require-client-cert helper is a boot-reject fixture; and **`net.Pipe` deadlocks**, so the live handshake needs a loopback pair.

**Placeholder scan — the honest count is ELEVEN, not three.** No "TBD", no "implement later", no "similar to Task N", no "add appropriate error handling", and T5's former *"locate at IMPL time"* is now pinned to `name_test.go:217-218`. But **eleven constructs ship as signature-plus-commented-step-list rather than full code**: `mkTestPKI`, `liveHandshakeErr`/`connPair`, `TestClassifyHandshakeErr_TLS12` (T1); the two registration tests' setup and `TestListenerMetrics_GateMatchesInc` (T2); the listener setup and `driveH3` (T3); the three increment tests and the plaintext test (T4); `scrapeProm` (T6). **Each names its copy-source with file:line** — `internal/tls/config_test.go:42-80` and `0111/driver.go:193-260` (PKI), `tls_handshake_negative_test.go:121-175` (the live-handshake scaffold), `quic_test.go:121-126` (H3), `0111/driver.go:393-445`+`:520` (the arms), `0055/driver.go:795-824`+`0005/driver.go:427` (the scraper) — which is the house convention for "clone this landed shape". **V1 executed T1–T4 from exactly these descriptions and reached green**, so they are executable; but the earlier claim of "three" was itself the kind of understatement this section exists to catch.

**Type consistency.** `handshakeOutcome` / `outcomeOK` / `outcomeVerifyError` / `outcomeNoCert` / `outcomeOther` and `classifyHandshakeErr(err error) handshakeOutcome` are spelled identically at T1, T4 and in every break. `sslHandshake` / `sslFailVerifyError` / `sslFailVerifyNoCert` are identical at T2, T4 and in `listenerSSLNames`. `AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` matches `fixture.go:76` verbatim. The three wire names and their Prometheus forms are consistent at T2, T3, T5, T6, T7. `manager.go:516-525` is spelled one way throughout. **The BYTE-UNTOUCHED roster and the EDIT roster were set-differenced by V2: EMPTY intersection** — `manager_test.go` is correctly gated on shape not sha256, `internal/stats` is correctly off the list, `quic.go` is correctly on it, so `reference_plan_schedules_edits_to_a_byte_gated_file` is not present.

**Memories this row is bound by, cited where they apply** (V2-Mo10 flagged five that were operative but unnamed): `reference_probe_must_discriminate` · `reference_go_client_cert_withholding` · `reference_dynamic_stat_name_charset_guard` · `reference_retention_field_populate_before_value_copy` · `reference_brainstorm_adjective_acquires_adr_authority` · `reference_code_comment_not_evidence` · `feedback_brief_citations_not_evidence` · `reference_quoting_is_not_executing` · `reference_a_drift_correction_is_itself_a_claim` · `reference_verification_table_launders_wrong_cites` · **`reference_differential_asserter_dispatch`** · **`reference_panic_counter_differential_delta_assertion`** (the rule Shape A knowingly departs from, and why) · **`reference_listener_stat_scope_cross_side_divergence`** (⚠️ **B8 is a RE-statement, not a first recording — this memory already carries all three divergences and the "/stats/prometheus, address-as-LABEL" shape**) · `reference_fixture_tb_has_no_logf` · `reference_fatalf_makes_assertions_unreachable` · `reference_differential_run_selector` · `reference_differential_break_protocol_count1` · `reference_break_protocol_commit_first` · `reference_deliberate_break_wrong_assertion` · `reference_plan_break_instructions_dont_compile` · `reference_plan_schedules_edits_to_a_byte_gated_file` · `reference_spec_drafted_identifier_collision_check` · `reference_parallel_stream_mints_fresh_drift` · `reference_parallel_subagents_private_scratch` · `reference_sentinel_deferred_sentence_live_vs_historical` · **`reference_next_prompt_tracked_despite_gitignore`** · `reference_roadmap_split_phase_row_done` · `reference_cluster_race_outlier_flake` · `reference_bash_cwd_reset_commits_to_main` · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `feedback_push_to_origin` · `feedback_pertask_gofmt_lint`.

**⚠️ Commit-message hygiene** (V2-m9): the `**Commit:**` lines below each task contain backticks. **Do not paste them into `git commit -m "…"`** — the backticks trigger command substitution. Use `git commit -F -` with a heredoc, or strip them.

**⚠️ The one thing no one has done end to end: NO FULL BUILD OF ROW 74 EXISTS IN THE CANONICAL TREE.** V1 built T1–T4 in a private scratch copy and ran every break there, which is why S1/S2/M2 were caught — but **T5–T9 were never executed, the `0111` `AssertStats` has still never been written or run, Breaks G/G′/H are untested, and nothing reference-side was executed at any point in this stage.** §1.3 states it; this line repeats it because the temptation to read §1.1's ledger as buildability evidence is exactly what `reference_verification_table_launders_wrong_cites` describes.
