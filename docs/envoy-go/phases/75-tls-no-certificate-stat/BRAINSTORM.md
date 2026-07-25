# Phase 75 Brainstorm — the downstream TLS success-path no-client-certificate annotation (`ssl.no_certificate`) (the TWENTY-FIRST Observability-family row *(chain ordinal — §2.1 records the inherited off-by-one, KEPT)*; the SECOND consecutive downstream-`ssl.*` row and a DIRECT continuation of phase 74's seam — it adds a FOURTH fixed name to the block `registerListenerMetrics` already gates on `rt.tlsMode`, Inc'd on the SUCCESS path phase 74 left at `manager.go:1277`; +1 stat / **+0 fixtures** — `0110` is EXTENDED / +0 packages / +0 modules / +0 fuzzers / +0 production imports)

---

## 1. Mission and scope confirmation (75 — the row that RETIRES a fixture's OWN confession for the second time running, and CORRECTS the ADR that deferred it)

### 1.1 What phase 75 delivers as a self-contained whole

ONE fixed-name counter at listener scope:

- **`listener.<normalized-addr>.ssl.no_certificate`** — *"this COMPLETED handshake presented no client certificate."*

It is a **success-path annotation**, not a failure counter. It is incremented on the same fall-through phase 74 already instruments with `rt.sslHandshake.Inc()`, gated on the peer having presented no certificate. Both counters move together on an anonymous accepted connection; only `ssl.handshake` moves when a certificate was presented.

Registration joins the EXISTING `if rt.tlsMode` block at `internal/listener/manager.go:378-382`, under the prefix `registerListenerMetrics` already builds (`prefix := "listener." + normalizeAddr(rt.addr) + "."`, `manager.go:375`). **No new gate, no new registration function, no new classifier.**

### 1.2 What phase 75 does NOT deliver (forward to §8)

`ssl.connection_error` (its blocker is now RETIRED — see §2.4 — but it needs a NEW fixture, which this row does not) · the FOUR dynamic families · the other TEN fixed names · `Listener.stat_prefix` · the `upstream_cluster` span tag · the gRPC and Runtime family openings.

### 1.3 Phase-done as the TWENTY-FIRST Observability-family row (family STAYS OPEN)

The family stays open. Row 75 registers `in-progress` at this BRAINSTORM's stage-close commit per the ROADMAP §Schema invariant (`ROADMAP.md:21`), which **RE-OPENS sentinel check (1)** after its first-ever silent close at the phase-74 IMPL.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW anticipated (escape-valve armable)

~7-9 tasks, ~10 production lines across TWO files. Far under ADR-0045's >25-task / >1500-LoC valve. The valve is armable but not anticipated to fire.

### 1.5 Package placement — ALL edits in EXISTING files, ZERO new packages

`internal/listener/manager.go` and `internal/stats/name.go`. Nothing else in production.

### 1.6 No prebrainstorm-notes branch

None exists for this subject.

### 1.7 Phase 75's relationship to the existing seams — a pure extension of a seam LANDED YESTERDAY

Phase 74 built the whole apparatus: the `tlsMode` gate, the listener-scope prefix, the three counter fields, the SN3 three-dot flattening, and a cross-side `/stats/prometheus` `StatsAsserter` template. This row consumes all of it and adds one name. **The interesting content of this row is documentary, not mechanical** — see §2.2.

---

## 2. Design decisions

### 2.1 Row + subject confirmation *(SELF-PICKED per the 2026-07-12 standing directive → phase 75 row registered)*

The loop is autonomous; the roller self-picks the smallest defensible candidate and records the rejected alternatives. **There is no banked mid-lifecycle work** — phase 74 closed all four stages with no split leg outstanding — so this opens a brand-new subject.

**FIVE read-only re-costing agents ran at tip `f8f6cd44` on disjoint remits**, two of them running live reference containers and three running Go-toolchain probes. Every cost figure below was RE-DERIVED against source at this tip; none was re-read from a document. The adjudicated field:

| candidate | tasks | prod lines | prod files | +stats | +fixtures | blocker |
|---|---|---|---|---|---|---|
| **`ssl.no_certificate` (PICKED)** | **7-9** | **~10** | **2** | **+1** | **+0** | **none** |
| `fault.abort.grpc_status` | 7-9 | ~60 | 2-3 | +0 | +1 | none — **opens the gRPC family** |
| `Listener.stat_prefix` | ~8 | ~25-30 | 1 | +0 | +1 | none |
| `upstream_cluster` span tag | 7-9 | ~25 | 5 | +0 | +1 | divergence CLOSABLE (newly established) |
| `ssl.connection_error` | ~9-11 | ~10-14 | 2 | +1 | **+1 (forced)** | blocker RETIRED, cost ROSE |
| Runtime family opening | 9-11 | — | several | +0 | +1 | none, but **+1 package** |
| `stats_flush_on_admin` | 8-10 | ~60-90 | 3 | +0 | +1 | none |
| `hcm.access_log_options` | 9-12 | ~150+ | 3 | maybe | +1 | the `statusCode == 0` guard is load-bearing |
| HCM `server_name`/`via` | 10-14 | ~120+ | 2 pkgs | +0 | +1 | new cross-package config seam |
| `stdout`/`stderr` loggers | 10-12 | ~80 + harness | many | +0 | +1 | **undrained subject pipe — UNRESOLVED** |
| `hcm.merge_slashes` | 11-14 | ~100 + bugfix | several | +0 | +1 | carries an entangled routing bugfix |

`ssl.no_certificate` wins on every axis and is **the only candidate needing no new fixture directory**.

#### ⚠️ THE HEADLINE — THE ADR THAT DEFERRED THIS ROW DEFERRED IT ON A CLAIM THAT WAS FALSE WHEN WRITTEN, AND THE CITATION SUPPORTING IT POINTS AT NOTHING

`DECISIONS.md:17308` (ADR-0296 §Decision **(g)**, landed at the phase-74 IMPL on 2026-07-24 — ONE DAY before this BRAINSTORM) reads:

> envoy-go does not implement `require_client_certificate: false` (verify-if-present) at all — §Context ¶7 / `VERIFYIFGIVEN` is explicitly OUT OF SCOPE — so no `ssl.no_certificate` counter is owed here

**Both halves fail mechanically.**

**(i) The factual claim is FALSE.** Phase 67 (`tls-require-client-cert-false`, **row 67 `done`**) landed exactly this. `internal/tls/config.go:79-84`:

```go
clientAuthFor := func(require bool) stdtls.ClientAuthType {
	if require {
		return stdtls.RequireAndVerifyClientCert
	}
	return stdtls.VerifyClientCertIfGiven
}
```

The three-way mapping is documented in place at `config.go:60-68` and applies across **all three** validation-source shapes (inline, SDS-VC, CVC). A working differential fixture — `test/fixtures/0110-tls-require-client-cert-false/` — has driven it since phase 67.

**(ii) The supporting citation is a MIS-POINTER, and self-referential.** `grep -n 'VERIFYIFGIVEN' docs/envoy-go/DECISIONS.md` returns **exactly one hit: `:17308`** — the citing sentence itself. The token exists nowhere else in the file. And §Context ¶7 (`DECISIONS.md:17274`) is about the four-family dynamic split; `awk 'NR==17274' … | grep -c 'require_client_certificate'` ⇒ **0**.

⚠️ **This is the THIRD internal mis-pointer found in ADR-0296.** The phase-74 IMPL already fixed two while completing it (the STATUS line's `¶6`→`¶4(i)`, and ¶3's self-refuting grep). §Decision(g) itself opens by noting that SPEC §13.1's hygiene claim was false *for exactly this item* — and then reproduces the same defect one level down. ⇒ **`reference_document_hygiene_claim_not_evidence`, in its sharpest form yet: the paragraph that documents a hygiene failure was itself the next hygiene failure.**

**Why it is load-bearing rather than trivia:** (g) is the SOLE stated reason the counter was "not owed", and it is the paragraph a future roller would read when costing this row. Left standing, it would have deterred the pick of the cheapest available candidate indefinitely. The narrower claim — *not owed **in phase 74***, whose fixture arms were all `require_client_certificate: true` — survives and is correct. **The reason given does not.**

⚠️ Recorded as a **documentary finding, not as criticism of phase 74**, whose row was correctly scoped. ADR-0297 owes the correction; the FORM is the indented `> [CORRECTED at phase 75/ADR-0297: …]` blockquote, since a later phase is correcting an earlier, different ADR's bullet (the discriminator is the **ADR FAMILY, not the phase gap** — `DECISIONS.md:16901` precedent, as re-established at the phase-74 SPEC).

#### The second reason the pick is right: a fixture's OWN confession has gone stale, for the second row running

`test/fixtures/0110-tls-require-client-cert-false/README.md:160-161`, under `## Coverage boundaries (named, unasserted)`:

> - **No `ssl.*` stats** — envoy-go emits none, so a verdict `StatsAsserter` is
>   infeasible (inherits PLAN-65 C3); a pre-existing framework gap. Never assert
>   `/listeners` or `total_listeners_active`; …

**"envoy-go emits none" went FALSE at the phase-74 IMPL** — it emits three. Phase 74 retired the identical confession in `0111`'s README; `0110` carries the same stale sentence and nobody has touched it.

⚠️ **The bullet BUNDLES a retirable half with a still-live guard.** The `ssl.*` clause is retirable; *"Never assert `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as listener liveness"* remains **LIVE and correct**. Phase 74's SPEC found this exact bundling at FOUR sites in `0111`. **The edit SPLITS the bullet; it does not delete it.**

### 2.2 Scope: ONE fixed name, registered in the EXISTING gate *(SPEC pins — D-TLSNC-SCOPE)*

Registration joins `manager.go:378-382` inside `if rt.tlsMode`. **No new gate is introduced and none is owed** — phase 74 established by live probe that the reference registers `listener.<addr>.ssl.*` TLS-chains-only, and `rt.tlsMode` is provably sufficient (envoy-go rejects mixed TLS+plaintext chains on one listener; `startQUIC` hard-errors without a TLS config).

⚠️ **QUIC inherits phase 74's resolution UNCHANGED and it must NOT be re-litigated.** The reference registers the full `ssl.*` family on a QUIC listener at boot and never moves it, so eagerly-zero QUIC registration is **EXACT PARITY**. Gating QUIC out would MANUFACTURE a departure — the SPEC's own first reading at phase 74, refuted by probe. `quic.go` stays byte-untouched; `quic_test.go`'s `want` set at `:277` gains the fourth name and its "still zero after a real H3 round trip" loop then covers it for free.

### 2.3 The increment site and its predicate *(SPEC pins the exact predicate — D-TLSNC-PREDICATE)*

The success fall-through, `manager.go:1276-1278`:

```go
// phase 74: a COMPLETED downstream TLS handshake.
rt.sslHandshake.Inc()
dispatchConn = tlsConn
```

`tlsConn` is a `*stdtls.Conn` in scope. The anticipated shape is a `len(tlsConn.ConnectionState().PeerCertificates) == 0` guard beside the existing Inc.

**EXECUTED at this session's toolchain** (`go1.26.5`, four live loopback TLS handshakes, server side, `VerifyClientCertIfGiven`): `PeerCertificates` is length **0** for a no-cert client and **1** for a presenting client, at **both** TLS 1.2 and TLS 1.3. The signal is available and version-independent.

⚠️ **`net.Pipe()` is NOT usable** for these probes — it deadlocks a client-cert handshake silently to the test timeout (`reference_netpipe_deadlocks_client_cert_handshake`). A loopback TCP pair is mandatory, and was used.

⚠️ **An untrusted/no-cert arm needs `GetClientCertificate` forced-send** or it degrades into a vacuous second no-cert arm (`reference_go_client_cert_withholding`). `0110`'s driver already does this (`driver.go:69-81`).

### 2.4 The rivals, re-costed — and TWO of them were materially changed by re-derivation

**`ssl.connection_error` — its recorded blocker is REFUTED BY EXECUTION, and the row got BIGGER, not smaller.**

The landed record (ADR-0296 §Decision(e), §Context ¶8(i), §Consequences; `BEHAVIOR_CONTRACT.md:1855`; `STATE.md:18`) states the bucket's membership is **UNENUMERATED** and that *"a counter whose membership cannot be enumerated cannot be cross-side asserted honestly."*

**18 Go arms + 16 live reference arms, every round control-validated, established a crisp one-dimensional boundary:**

> The reference books `ssl.connection_error` for every input producing a **decodable but rejected** handshake attempt — including record-layer garbage, SSLv2, and certificate-shaped failures that never reach chain verification. It books **NOTHING** when the peer went away without delivering a complete ClientHello.

So the blocker is dead. But the finding **raised** the cost rather than lowering it: a naive catch-all `else` **OVER-COUNTS** the EOF class (Go returns `io.EOF` / `io.ErrUnexpectedEOF`; the reference counts zero), and that class — health-checkers, LB probes, port scanners — is the highest-frequency one in production. The correct implementation is a predicate, not a counter attached to the existing `outcomeOther` arm:

```go
if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
    return outcomeUncounted
}
```

⚠️ `manager_test.go:4294` currently pins `{"io.EOF", io.EOF, outcomeOther}` — **that landed row IS the over-count** and must flip. And the row **cannot ride `0111`**: a 4th arm changes `wantObservable` (`0111/driver.go:99`), moves `downstream_cx_total` 3→4, and invalidates the Shape-A "deterministically 3 accepts" reasoning. ⇒ **a NEW fixture is FORCED.** Phase 74 was +0 fixtures; this cannot be. Net: comparable to phase 74, not smaller. **Deferred on COST, with its blocker RETIRED and its boundary now written down** — a strictly better-specified deferral than the one it replaces.

**`upstream_cluster` span tag (the phase-74 RUNNER-UP) — its RESIDUAL DIVERGENCE is CLOSABLE.** Phase 74 deferred it because the reference's tag is SELECTION-gated, not pick-gated, leaving 5 span-capable zero-endpoint emit sites. All five anchors verified EXACT at tip (`connection.go:597`/`:699`, `h2dispatch.go:530`, `h3dispatch.go:280`/`:341`), and the 6/5/7 partition CONFIRMED. But a **route-resolved** cluster name is a different source from a pick-derived one, and it is already in scope at 4 of the 5 sites (`entry`), one 3-line thread from the 5th. Since SELECTION-gating is precisely what makes the route-resolved name correct, the row becomes *"plumb the tag at all 18 sites"* — ~25 lines, 7-9 tasks, zero new stats. **No longer blocked; deferred on size only.** Residual: `weighted_clusters` at a local-reply site, un-exercised by any landed fixture.

⚠️ **The DISCRIMINATING break phase 74 handed forward DOES NOT EXIST.** It named *"`extractEndpoints`'s `la.GetClusterName()` vs the cluster's own `name`"* (`manager.go:875`, call site `:433`). Both anchors are exact, but `extractEndpoints` makes **ZERO** `GetClusterName()` calls — the name arrives as a **parameter** and is used only in five `fmt.Errorf` boot-reject messages. Crossing it changes error text only. **The break would have been VACUOUS.** Verified by the controller directly.

**`fault.abort.grpc_status` — the ONLY candidate that moves the termination sentinel.** ROADMAP row 09 already assigns it to the gRPC family. It is live divergence, not an unimplemented knob: `internal/filter/http/fault/fault.go:132-140` sets `abortEnabled` **only** for the `FaultAbort_HttpStatus` arm, so a `grpc_status` abort does nothing while the reference aborts. It escapes the family's hard blocker (trailers are observed-and-discarded per ADR-0058, `h2/client.go:440`; `RunDecodeTrailers`/`RunEncodeTrailers` have **zero** production callers — all five non-test hits are comments) because a gRPC error local reply is a **Trailers-Only, headers-only** response, which envoy-go already emits at `extproc/check.go:518-530`. **7-9 tasks, no gRPC client library needed.** Deferred here only because the standing directive says smallest-first — but see §8, it is the recommended NEXT opening.

**Runtime family opening — correctly deferred at 9-11 tasks.** ⚠️ **`layered_runtime` is NOT silently ignored** — it is a hand-written reject at `internal/bootstrap/bootstrap.go:568-569`, firing *before* protojson, pinned by two tests. The parse side is nearly free (a 2-line deletion; a scratch protojson probe unmarshalled `static_layer` cleanly under `DiscardUnknown:false`). The cost is that **nothing in the codebase is waiting to consume a runtime value** — all 12 runtime sites *reject* rather than wait, so a runtime layer lights up zero existing code. Needs +1 production package.

**`Listener.stat_prefix` — CONFIRMED, and phase 74 made it CHEAPER, not dearer.** Exactly ONE production prefix-construction site (`manager.go:375`); 21 `GetStatPrefix` hits, **zero** on a `*listenerv3.Listener`. Live probe: the reference performs a pure **RENAME** (no coexistence) and hoists a non-address-shaped prefix into the `envoy_listener_address` **label** — so envoy-go's SN3 rule needs **zero** change, refuting the divergence that was expected. Phase 74's three names ride the same `prefix` variable, so the row gained a 5-name assertion surface at zero cost. Its unique payoff: it would make `envoy_listener_address` **cross-side identical**, dissolving the limitation `0111/driver.go:648-654` documents in its own comment.

### 2.5 Fixture posture: +0 new fixtures — `0110` is EXTENDED *(SPEC confirms — D-TLSNC-FIXTURE)*

`0110-tls-require-client-cert-false` drives **three arms** against one `require_client_certificate: false` listener (`driver.go:362-373`, `wantObservable` at `:89`):

- **trusted** — `client_X`, forced-send → ACCEPTED **with** a cert
- **untrusted** — `client_Y`, forced-send → REJECTED
- **none** — no cert → **ACCEPTED** (the flag's discriminator)

Predicted per side: `handshake=2`, **`no_certificate=1`**, `fail_verify_error=1`, `fail_verify_no_cert=0`.

⚠️ **Arms 1 and 3 DISCRIMINATE — `no_certificate` is 1, not 2.** That asymmetry is the whole non-vacuity argument, and `0111` (require=true) could never have supplied it. The fixture already carries `subjAdminPort` plumbing (`driver.go:313`) and `ProbeAdmin` (`:552`); it has **no** `AssertStats`. `0111`'s `AssertStats` (`:655`) + `scrapeProm` (`:739`) is a directly transplantable template.

⚠️ **`var _ fixture.StatsAsserter = (*rccfDriver)(nil)` is a TRIPWIRE, not the dispatch mechanism** — proven live at phase 74. Dispatch is a silent type assertion; compiler, `go vet` and `golangci-lint` are all silent on a renamed method. **The ABSENT check must be SEPARATE from the value check**, since an unregistered counter reads `0 == 0` and passes VACUOUSLY.

### 2.6 Anticipated task spine (~7-9; the PLAN decomposes)

Field + registration → the guarded Inc → the `helpText` entry → the four guards → the `0110` `AssertStats` → docs/ADR → the six-gate → the row flip.

### 2.7 Stat surface hypothesis: **+1** (1204 → 1205)

A NAME-surface delta, not a flat per-deployment one: registration is TLS-chains-only, so a plaintext-only deployment gains **ZERO** names, and a QUIC listener gains it permanently-zero (parity).

---

## 3. Framework-survey result — a stat addition on a seam landed yesterday; ZERO new packages/modules

### 3.1 Framework: no seam at all
The gate, the prefix, the counter-field pattern, the SN3 flattening and the cross-side asserter template all exist. This row consumes them.

### 3.2 NEW packages: NONE.
### 3.3 go.mod modules: NONE. `crypto/tls` and `stats` are already imported by `manager.go`; **+0 production imports** anticipated.
### 3.4 REUSES
`registerListenerMetrics`'s `rt.tlsMode` gate · the `prefix` variable · `stats.NewCounter` · the SN3 three-dot flattening · `helpText` · `0111`'s `AssertStats`/`scrapeProm` · `0110`'s existing three-arm PKI and admin plumbing.

---

## 4. Bootstrap-level applicability — NONE

This is a LISTENER-RUNTIME row. It consumes no new config field, so there is no parse arm, no boot-reject, and **no fuzz-seed classification to flip** (`reference_fuzzer_count_docs_drift`: an `f.Add` seed is not a fuzzer). Fuzzers stay **55**.

---

## 5. Stat surface hypothesis — **+1** (75): 1204 → 1205

There is NO mechanical counting command; the contract says so itself. Enforcement is per-phase `TestNoNewStat*`-style guards asserting an exact delta, never the absolute total. The ledger line `**Phase 75 — 1204 -> 1205 (+1)**` is appended after the tail at `BEHAVIOR_CONTRACT.md:831`/`:847`.

⚠️ The **unattributed `1200 → 1201` gap** phase 74 RECORDED rather than invented stays recorded. **Do not fabricate a line to close it.**

---

## 6. Anticipated edit sites (the SPEC RE-DERIVES each at ITS OWN tip — a BRAINSTORM cite is not evidence)

**Production (TWO files):**

| site | anchor at THIS tip | edit |
|---|---|---|
| counter field | `internal/listener/manager.go:180-182` | +1 `sslNoCertificate *stats.Counter` |
| registration | `manager.go:378-382` (inside `if rt.tlsMode`) | +1 `r.NewCounter(prefix + "ssl.no_certificate")` |
| increment | `manager.go:1276-1278` (success fall-through) | +3 guarded on `len(...PeerCertificates) == 0` |
| **HELP text** | `internal/stats/name.go:469-471` | **+1 `envoy_listener_ssl_no_certificate` entry** |

⚠️ **The `helpText` entry is MANDATORY, and the guard that enforces it will NOT catch you.** `prom.go` falls back to the metric name, degrading `/stats/prometheus` to `# HELP envoy_listener_ssl_handshake envoy_listener_ssl_handshake`. `TestHelpText_ListenerSSLHandshakeOutcomes` (`internal/stats/name_test.go:230`) asserts exactly that degradation signature — **but over a HAND-LISTED three-name roster** (`:231-235`). A fourth name added without extending `wantNames` is **silently unguarded**. This is the same silent-staleness class phase 74 found in `TestListenerManager_AllocatesTwoMetricsPerListener`, and it means **this row edits two files in `internal/stats/` as well as the test**.

**Guards (all RE-DERIVED at this tip):**

| guard | anchor | what a +1 name delta requires |
|---|---|---|
| `TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames` | `manager_test.go:2023` | +1 `want` entry **in SORTED position**, and a **RENAME** (…`FourSSLNames`) plus its doc comment |
| `TestListenerMetrics_GateMatchesInc` | `manager_test.go:2128` | **+2 pointer assertions** (one non-nil, one nil) — the pointer half is the LOAD-BEARING half |
| `TestListenerMetrics_PlaintextListenerRegistersNoSSLNames` | `manager_test.go:2076` | nothing (`len(got) != 0`, count-free) |
| `TestListenerManager_AllocatesBaseListenerMetrics` | `manager_test.go:1951` | nothing (presence-only) |
| QUIC name-set guard | `internal/listener/quic_test.go:277` | +1 `want` entry; the zero-loop then covers it free |
| `TestHelpText_ListenerSSLHandshakeOutcomes` | `internal/stats/name_test.go:230` | +1 `wantNames` entry — **or it goes silently unguarded** |

⚠️ **A nil `*stats.Counter`.Inc is a PROCESS CRASH**, not a no-op — no nil guard in `Inc`, no `recover()` on the `serveConnection` goroutine (confirmed live at phase 74, stack `stats.(*Counter).Inc` under `listener.(*listenerRuntime).serveConnection`). The pointer assertions are mandatory, not stylistic.

**Fixture:** `test/fixtures/0110-tls-require-client-cert-false/driver/driver.go` (+`AssertStats`, +`scrapeProm`, +the `var _` tripwire) and `README.md:158-163` (**SPLIT** the bundled bullet).

**Docs:** `BEHAVIOR_CONTRACT.md` (`:928` boundary restatement, `:831`/`:847` ledger, the `:1849` handshake-outcome subsection), `DECISIONS.md` (**ADR-0297**), `ROADMAP.md`, `STATE.md`.

---

## 7. BRAINSTORM-time open questions to the SPEC (the D-TLSNC-* docket)

**D-TLSNC-SEMANTICS — THE ONE THAT MATTERS, and it is genuinely open.** Does the reference increment `ssl.no_certificate` on a plain **one-way-TLS** listener with **no `validation_context` at all** (i.e. `ClientAuth == NoClientCert`, where a certificate is never even requested)? Two readings are consistent with everything currently on record:
  - **(a) unconditional** — every completed handshake without a peer certificate, regardless of whether one was requested. Then the Inc gate is `len(PeerCertificates) == 0` alone, and the counter fires on the 9 fixtures carrying a downstream TLS context.
  - **(b) request-gated** — only where a certificate was *solicited* (`ClientAuth != NoClientCert`). Then the gate needs the chain's client-auth mode too.
  `BEHAVIOR_CONTRACT.md:962` records the reference incrementing it in an **anchorless `validation_context: {}`** shape where *"the reference … never sends a CertificateRequest in ANY cell"* — which **leans (a)** but was probed for a different purpose and is not decisive. **This must be settled by probe before the PLAN, because it decides the predicate.**

**D-TLSNC-CERTPRESENT.** Confirm the reference books **0** on the cert-presented arm (arm 1). Predicted 0; unprobed.

**D-TLSNC-FAILPATH.** Confirm it does **not** move on a *failed* handshake — the success-path reading depends on it.

**D-TLSNC-SCOPE.** Re-confirm registration is TLS-chains-only for this name specifically (phase 74 established it for the family; do not assume per-name).

**D-TLSNC-FIXTURE.** Confirm Shape A (scrape once, absolute counts) is sound on `0110` — specifically that nothing pre-moves its `ssl.*` before the driver's arms.

⚠️ **A HAZARD FOR THE SPEC'S OWN PROBES, discovered by execution this session and NOT previously recorded.** With the reference's tcp_proxy upstream cluster pointed at a **dead** endpoint, a fully-successful TLS 1.3 handshake left `ssl.handshake` at **0** across two runs — along with `ssl.no_certificate` and the whole `logHandshake` family. Repointing at a live endpoint made all of them fire. **Two observations vs one; the mechanism is NOT established.** `0111` has a live echo backend, which is why phase 74's cross-side match held. **Any probe or fixture asserting `ssl.*` on a failing-upstream arm must treat this as live**, and `0110`'s backend liveness must be confirmed before its counts are trusted.

---

## 8. What phase 75 does NOT deliver (forward)

**Deferred, with costs re-derived at THIS tip — read these as current, not as carried adjectives:**

- **`ssl.connection_error`** — blocker **RETIRED BY EXECUTION** (§2.4); now deferred on cost (~9-11 tasks, a FORCED new fixture). Its boundary and the EOF-exclusion predicate are written down for whoever takes it.
- **`fault.abort.grpc_status` — the RECOMMENDED NEXT OPENING.** 7-9 tasks; the **only** identified candidate that clears a sentinel check-(3) blocker. One probe owed: whether the reference's gRPC fault abort is headers-only or emits real trailers. **If it emits trailers the row collapses into the ADR-0058-blocked set and gRPC becomes uniformly large.**
- **Runtime family opening** — 9-11 tasks, +1 package; the correct *second* opening.
- **The FOUR dynamic `ssl.*` families — and the recorded deferral is WRONG IN BOTH DIRECTIONS** (§11.3): `ssl.versions.*` is effectively **UNBLOCKED**, while `ssl.sigalgs.*` is **STRUCTURALLY IMPOSSIBLE**, not merely deferred.
- The other TEN fixed `ssl.*` names — `session_reused` (needs session-cache driver machinery); `fail_verify_san` / `fail_verify_cert_hash` / `was_key_usage_invalid` (**blocked behind feature rows** — `internal/tls/config.go:482`/`:485`/`:488` boot-reject); the four `ocsp_staple_*` (**zero** OCSP anywhere in the repo); the two `certificate.*` expiry **gauges**.
- `Listener.stat_prefix` · `upstream_cluster` span tag · pick-time `picked`-propagation · `access_log[].filter` · `hcm.merge_slashes` (whose blast radius contains a **PRE-EXISTING H2 ROUTING BUG**, §11.3) · `hcm.access_log_options` · `stdout`/`stderr` loggers · `stats_flush_on_admin` · HCM `server_name`/`via` + the `Server` OVERWRITE-vs-APPEND defect · the swallowed-panic BOOT HANG · route-level header manipulation · `prefix_rewrite`/`host_rewrite_*` · all tracing remainders · all xDS · all HTTP/3.
- The IPv6-bracket and dot-normalization scope divergences (**PRE-EXISTING**, already affecting `downstream_cx_total`).

---

## 9. ADR-0045 split readiness + ADR roster

A SINGLE FLAT ROW of ~7-9 tasks; the valve is armable but not anticipated to fire. **ONE ADR: ADR-0297** (next-free — `DECISIONS.md` tail is **ADR-0296**, `grep -c '^## ADR-0297'` ⇒ **0**). §Context is drafted at the **SPEC** per **ADR-0044-as-used** — ⚠️ **ADR-0044 does NOT actually contain that discipline** (established at the phase-74 SPEC: it is titled *"BEHAVIOR_CONTRACT HTTP/1.1 subsection"* and a mechanical scan returns zero such language). **Use the hedge.** §Decision + §Consequences land IN PLACE at the IMPL, after a RETAINED footer, mirroring ADR-0295/0296. **DECISIONS.md stays BYTE-UNTOUCHED at this BRAINSTORM.**

ADR-0297 additionally owes the **ADR-0296 §Decision(g) correction** (§2.1), as an indented `> [CORRECTED at phase 75/ADR-0297: …]` blockquote.

---

## 10. Envelope + counts (anticipated at the phase-75 IMPL; docs-only at this BRAINSTORM)

**+1 stat (1204 → 1205)** · **+0 fixtures (119** — `0110` EXTENDED; no new YAML, no new directory, no new BackendKind, no new port**)** · +0 fuzzers (**55**) · +0 BackendKinds (**38** — a TAIL value; the file declares 39 constants, 0-38) · +0 go.mod modules (**2** — the phase-61.2 lineage figure; the single `go.mod` requires 67) · **+0 production imports** · **ZERO new packages** · **ZERO new exported symbols** (to be asserted BY COMMAND — an extracted import-block diff and a `go doc -all` set-diff — not by inspection). DECISIONS tail → **ADR-0297**.

⚠️ **TEST-side imports may grow and that is PERMITTED** — the +0 claim is a PRODUCTION claim, and the two categories must be audited SEPARATELY.

⚠️ **The "+0 production imports" gate COMMAND phase 74 used is UNRELIABLE** — `git diff master -- … | grep -E '^\+' | grep -E '^\+\s*(_|[a-z]+ )?"'` returns false-positive hits on map-literal lines and exits 0, so gating on the exit code reads a PASS as a FAIL. Use hunk headers plus a direct extracted-import-block diff.

---

## 11. Sized-against-source — the cost derivations (FIVE agents at tip `f8f6cd44`)

### 11.1 What was and was NOT verified by execution

**EXECUTED this session:**
- The three sentinel checks, mechanically, by the controller.
- The check-(1) blind-spot figure, **RE-DERIVED not copied**: **106 data rows, 102 matched, FOUR misses** (`00` em-dash, `04` dotted slug, `28.1a`, `28.1b` letter suffix), all four `done`, identified by set-difference.
- `NamePattern` applied by the controller to representative names; four live loopback TLS handshakes reading server-side `ConnectionState`; a reflect probe on the unexported sigalg field; a protojson `layered_runtime` probe; 18 Go handshake-failure arms with three discriminating controls.
- **Live reference containers**: `Listener.stat_prefix` rename + label-hoist behaviour, and 16 `connection_error` arms across three rounds, each round control-validated.
- Every anchor in §6 and §2, re-derived by command at this tip.

**NOT EXECUTED — and nothing below may be cited as evidence for it:**
- ⚠️ **NO probe was run against the reference for THIS ROW'S OWN SUBJECT.** The `ssl.no_certificate` success-path semantics are **DOC-SOURCED** from phase 74's probe fleets and from `BEHAVIOR_CONTRACT.md:962`, **not re-executed**. D-TLSNC-SEMANTICS is genuinely open.
- ⚠️ **NO build of row 75 exists, in-tree or in scratch.** The field, registration, predicate, guards and asserter are all SPECIFIED and NONE is COMPILED.
- The reference's `/runtime` JSON shape; the reference's gRPC fault-abort wire response.

### 11.2 Controller re-derivation of the agents' load-bearing claims

The controller independently re-ran, rather than accepting: the `:107`-vs-`:117` panic split · `NamePattern` against seven representative names incl. `TLSv1.2` and Go's `VersionName` · `0110`'s three-arm structure and `wantObservable` · `extractEndpoints`' zero `GetClusterName()` calls · `layered_runtime`'s hand-written reject · the trailer discard and the zero production trailer callers · `fault.go`'s HttpStatus-only `abortEnabled` · the `helpText` roster and its guard · `VerifyClientCertIfGiven` in production · the `VERIFYIFGIVEN` and `¶7` greps.

### 11.3 Contested claims resolved BY THE CONTROLLER — two agents disagreed, and one was wrong

⚠️ **`BEHAVIOR_CONTRACT.md:962` DOES contain `ssl.no_certificate`.** One agent reported the ADR-0296 `:962` anchor as FALSE, claiming the token appears nowhere in the file. **That refutation is itself wrong.** The controller extracted it directly: the token sits at **character offset 627** of line 962 (`…accepts every client with `ssl.no_certificate` incrementing…`), and `grep -c` over the whole file returns **1**, that one hit being line 962. A second agent independently quoted the same text. **The `:962` anchor STANDS** — the misreading is explained by the same line also containing `require_client_certificate`. Recorded because *a drift correction is itself a claim*, and this one failed.

**CONFIRMED refutations, controller-verified:**
1. **`registry.go:107` is the DUPLICATE-registration panic; the invalid-name panic is `:117`.** Cited as `:107` in ADR-0296 §Context ¶7, the phase-74 BRAINSTORM and the ROADMAP row-74 cell.
2. **There is NO `B5` step in `BEHAVIOR_CONTRACT.md` for this departure.** All eight `B5` hits are `AMEND-B5`/phase-25.2 Wasm. The departure lives at `:1855` under the `:1849` heading. ⚠️ **"BEHAVIOR_CONTRACT B5" leaked into TWO LANDED PRODUCTION CODE COMMENTS** — `internal/listener/manager.go:392` and `:1265` — plus `DECISIONS.md:17304`/`:17314` and `STATE.md:18`. PLAN-internal step numbering became a permanent cross-reference pointing at nothing. **`reference_code_comment_not_evidence`, newly instantiated in production.**
3. **The four-family deferral is WRONG IN BOTH DIRECTIONS.** `ssl.versions.*` has **NEITHER** recorded blocker — Envoy's own `TLSv1.2` form passes `NamePattern` cleanly, and the charset trip is in *Go's* `VersionName` (`"TLS 1.2"`), which fails on a **SPACE**, not a hyphen. Conversely `ssl.sigalgs.*` carries a **THIRD, HARDER blocker the record does not name**: the peer signature algorithm is `testingOnlyPeerSignatureAlgorithm`, **unexported** on `tls.ConnectionState`, with no accessor — **structurally unimplementable**, not deferred. And `ConnectionState.CurveID` **is** exported at go1.26.5, so any deferral resting on "Go doesn't expose the curve" is stale.
4. **The `upstream_cluster` discriminating break does not exist** (§2.4) — it would have been vacuous.
5. **`layered_runtime` is REJECTED, not silently ignored** (`bootstrap.go:568-569`).
6. **The next-free reference port is `10450`, not `10118`.** Max allocated is **10449** (`0113/driver.go:115`); ports are **not** fixture-index aligned. Both are unused, but max+1 is the convention. *(The controller initially repeated the `10118` figure and corrected it on re-derivation.)*
7. **A PRE-EXISTING H2 ROUTING BUG is CONFIRMED and now proven by execution.** `internal/filter/hcm/h2/stream.go:440` uses `url.Parse`, not `url.ParseRequestURI`. Executed: `"//foo/bar"` parses to `host="foo" path="/bar"`. `:444` repairs the authority, but `u.Path` stays truncated — and routing reads exactly that field (`route.go:128`). **A `:path` of `//foo/bar` routes as `/bar`.** Only a *leading* `//` is affected. Filed here regardless of whether the `merge_slashes` row is ever picked.

### 11.4 Cite drift found — recorded, not silently fixed

`:107`→`:117` (three sites) · the phantom `B5` (five sites, two in production code) · ADR-0296 §Decision(g)'s false claim and self-referential citation · the phase-74 BRAINSTORM's dead `GetClusterName` break · two stale *"Task 18 will…"* comments in `hcm/connection.go:565` and `h2dispatch.go:503`, since `chain.go:455` now DOES expose `RunDecodeTrailers`.

### 11.5 Verifier fold — corrections to THIS SESSION'S OWN earlier reasoning

The controller stated *"reference port 10118"* and *"every production edit lands in ONE file"* before re-deriving both. **Both were wrong**: the port is **10450** (§11.3 item 6), and the `helpText` entry makes it **TWO** production files. Recorded rather than quietly amended, because a BRAINSTORM's own hygiene claim is not evidence either.

---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these)

- **Phase-directory delta:** the new `BRAINSTORM.md` only.
- **ROADMAP:** row 75 registered `in-progress` (RE-OPENING sentinel check (1)) **plus** a phase-75 charter sentence APPENDED to the `### Observability family` heading paragraph (`:200`/`:204`) — the phase-70/72/73/74 paired-edit precedent.
- **NO deferred sentence narrowed** — the phase-57 precedent; the narrow lands at the **IMPL only**. Check (2) must **STAY 3**.
- **DECISIONS.md stays BYTE-UNTOUCHED.**
- **STATE.md** §Current rolled **IN PLACE** (lifecycle DONE → 1); §Recent re-capped at FIVE **with its PREAMBLE updated** (ADR-0288). ⚠️ **The ADR-0288 singleton greps return 2, not 1**, for each of the three field-name tokens — the second hit is `STATE.md:7`, the RULE STATEMENT itself. **Never "fix" the count to 1**; that would delete the rule.
- **`next-prompt.txt`** rolled to the phase-75 SPEC (**TRACKED despite `.gitignore`**; edited in this worktree).
- **Sentinel re-run MECHANICALLY TWICE** — in the stage worktree AND on landed master after the squash-push.

Fresh worktree off master per `feedback_git_worktrees`; subagent-driven per `feedback_execution_style`; subagents commit LOCALLY only; controller squash-pushes at close. ⚠️ `git -C <abs-worktree-path>` for every git command — the cwd-reset hazard **fired live during this session** and was contained by that discipline.
