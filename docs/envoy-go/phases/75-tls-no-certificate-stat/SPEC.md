# SPEC 75 — the downstream TLS `ssl.no_certificate` success-path annotation at listener scope (the TWENTY-FIRST Observability-family row *(chain ordinal)*; ONE fixed-name counter, TLS-listener-gated and TCP-only, Inc'd on the success fall-through phase 74 left at `internal/listener/manager.go:1277`; +1 stat / +0 fixtures / +0 packages / +0 modules / +0 fuzzers / **+0 production imports**)

> **STATUS: SPEC done.** The D-TLSNC-* docket is **RESOLVED BY EXECUTION, all five items**, against `envoyproxy/envoy:contrib-v1.37.2` — including the one item the BRAINSTORM called *"genuinely open"*. ADR-0297 §Context is drafted here (**ADR-0044-as-used** — ⚠️ §1.1 D1: ADR-0044 does not contain that discipline). ROADMAP is **BYTE-UNTOUCHED**; row 75 STAYS `in-progress`.

---

## 1. Purpose / Mission

Phase 75 makes one more downstream TLS outcome countable in envoy-go: **`listener.<normalized-addr>.ssl.no_certificate`**, a **SUCCESS-PATH annotation** counting *"this COMPLETED handshake presented no client certificate."*

It is **not** a synonym for `ssl.fail_verify_no_cert`, and the mis-mapping is wrong in **both** directions — it would over-count `fail_verify_no_cert` on every accepted anonymous connection, and double-book a genuine no-cert REJECTION that the reference never routes through it. ADR-0296 §Decision (g) recorded that trap; this row implements the counter the trap was recorded to protect.

The row is a **pure observability addition**. Nothing behaves differently after it lands. It adds one field, one registration line inside a gate that already exists, one guarded `Inc`, and one `helpText` entry — and it converts a fixture's stale self-confession into a live cross-side assertion for the second row running.

### 1.1 BRAINSTORM drift ledger — what the SPEC RE-DERIVED, REFUTED, and newly found

Per `feedback_brief_citations_not_evidence`, every BRAINSTORM §6 anchor was re-opened at this tip (`e822f1ad`). `git diff --stat f8f6cd44 e822f1ad` touches only ROADMAP / STATE / the new BRAINSTORM / next-prompt ⇒ **ZERO production `.go` changed since the BRAINSTORM's derivation tip**, so any failing Go cite was **wrong when written**, not drift.

**HOLDS — every production and guard anchor, re-derived by command, ZERO drift:**
`manager.go:180-182` (the three `*stats.Counter` fields in `listenerRuntime`) · `:378-382` (the `if rt.tlsMode` block; calls at `:379-381`, `prefix` built at `:375` from `normalizeAddr` at `:347-349`) · `:1276-1278` (the success fall-through; **`Inc` is at `:1277`**) · `internal/stats/name.go:469-471` (the three `helpText` entries; map literal opens `:456`, closes `:472`) · `manager_test.go:2023` / `:2076` / `:1951` / `:2128` · `quic_test.go:277` (block `:274-278`) · `name_test.go:230-235` · `internal/tls/config.go:79-84` and `:60-68` · `DECISIONS.md:17308` / `:17274` / `:16908` / `:16901` / `1419-1462` · `BEHAVIOR_CONTRACT.md:962` / `:928` / `:1849` / `:1855` · `ROADMAP.md:137` (row 75 `in-progress`) / `:129` (row 67 `done`) / `:201` / `:203` / `:205` / `:185` / `:195`.

**`tlsConn` is genuinely in scope at `:1277`** — declared at `:1259` in the enclosing `if selected.tlsCfg != nil` block (**not** in the `if err :=` init), exact type `*crypto/tls.Conn` via the `stdtls` alias. **No plumbing is owed.** The failure classifier `classifyHandshakeErr` is consumed at `:1266`, in the **error branch only**; it never sees a successful handshake, so the success-path annotation cannot and must not reuse it.

**REFUTED — six, four of them load-bearing:**

- **R1. ⚠️ THE CORRECTION-FORM DISCRIMINATOR IS NOT THE ADR FAMILY.** The BRAINSTORM (§2.1), the router item 8 and the landed row-75 cell (`ROADMAP.md:137`) all assert *"the discriminator for that FORM is the **ADR FAMILY, not the phase gap**."* **Refuted by enumerating the whole population (n=4).** `DECISIONS.md:16901` is **ADR-0289 (FIFTH xDS-family row) correcting ADR-0286 (THIRD xDS-family row)** — **same family, and it uses the INDENTED form.** Family therefore cannot discriminate. The derivable rule: **a correction to a DIFFERENT, already-landed ADR goes INSIDE that ADR as an indented `  > [CORRECTED at phase N/ADR-XXXX: …]` blockquote (n=2: `:16901`, `:16910`); a correction of an ADR's OWN SPEC-stage text at its OWN IMPL is folded INLINE as `*(corrected at the phase-N IMPL from "…")*` (n=2, both on `:17213`).** Phase 75 correcting ADR-0296 is unambiguously the **other-ADR** case ⇒ **indented form**, which is the form the BRAINSTORM prescribed. **The prescription survives; the stated reason does not.**
- **R2. ⚠️ `DECISIONS.md:17209` IS NOT A CORRECTION AT ALL.** The BRAINSTORM and router cite it as the INLINE-form precedent. `:17209` is ADR-0294's `**Documented boundaries (PRE-EXISTING, shared by all three MetadataKinds, NOT fixed here):**` bullet — a column-0 list item about the Zipkin empty-tag drop. The two real inline instances are **both on `:17213`**, ADR-0295's heading line, and they are ADR-0295 correcting **itself**.
- **R3. ⚠️ `BEHAVIOR_CONTRACT.md:831`/`:847` ARE NOT THE `### Stat surface` LEDGER.** The ledger heading is at **`:4950`** (`grep -c '^### Stat surface'` ⇒ 1) and its entries run **`:4954-5002`**. `:831` and `:847` are the two **narrative bare-total restatements** inside the Graphite and OTLP sink sections (*"Stat surface UNCHANGED at **1204**"*). **Both identifications matter and both are load-bearing** — phase 75 must append a ledger line after **`:5002`** AND bump the two totals at `:831`/`:847`. ADR-0296's own ledger entry names those two lines as a recorded gap; missing them is the anticipated failure.
- **R4. The ledger line's FORM.** Predicted `**Phase 74 — 1201 -> 1204 (+3)**`. Actual (`:5002`) uses the **Unicode arrow `→` (U+2192)**, not ASCII `->`, and the **bold runs THROUGH the parenthetical descriptor to the colon**: `**Phase 74 — 1201 → 1204 (+3) (<descriptor>):**`. ⚠️ The form is **not uniform** in the file — `:5000` (Phase 51) closes the bold *before* the parenthetical. **Match the TAIL entry, not the file's average.**
- **R5. The `0110` fixture cites are wrong in two places.** The three arms are at **`driver/driver.go:400-435`**, not `:362-373` — `:362-374` is the **doc comment** of `driveSide`. `ProbeAdmin` is at **`:554`** (doc comment `:552-553`), not `:552`. The README bullet is at **`:160-163`**, not `:150-163`.
- **R6. The phase-74 correction blockquote sits at `DECISIONS.md:16910`, not immediately after `:16908`.** Layout is `16908` (the C3 bullet) → `16909` (blank) → `16910` (the blockquote) → `16911` (next bullet, no intervening blank).

**DOCUMENTARY findings the BRAINSTORM did not have:**

- **D1. ADR-0044 confirmed to lack the discipline — and the scale is now measured.** ADR-0044 (`DECISIONS.md:1419-1462`) is titled *"BEHAVIOR_CONTRACT HTTP/1.1 subsection"*. A mechanical token scan of its block returns **0** for `Context drafted`, `drafted at`, `IMPL`, `append`, `in-place`, `ADR-on-impl`, `first-use`, `Future-Work`, and `convention`. Yet `ADR-0044` is cited **532 times across 504 lines**, of which **134** use the exact phrase *"ADR-0044 ADR-on-impl convention"* — and citations exist to sections it does not have (*"ADR-0044's 'first-use authoring' wording"* `:6494`, *"ADR-0044 §Decision (b)"* `:7416`, *"the ADR-0044 §Future-Work forward-pointer-and-close discipline"* `:7754`). **The discipline is a CONVENTION WITH NO WRITTEN ADR**, stated most fully at `DECISIONS.md:8534` — a reference *note* inside a phase-18 ADR — and attributed there to the phase-13/14/15/16/17 precedent, not to ADR-0044's text. **This SPEC uses `ADR-0044-as-used` throughout.**
- **D2. ⚠️ THREE GUARDS THE BRAINSTORM'S ROSTER MISSED**, found by an independent repo sweep (§11).
- **D3. ⚠️ A FOURTH STALE-CONFESSION SITE OUTSIDE `0110`.** `test/fixtures/0108-xds-sds-validation-context/expectations.yaml:104-106` still says *"envoy-go emits NO `ssl.*` stats whatsoever."* It carries **no assertion** — it is prose only — but it is the same class the row exists to retire, and no prior document names it. **Recorded, NOT chartered** (§13): `0108` is out of this row's scope and touching it would widen the delta.
- **D4. A stale-prose cluster in the files phase 75 edits**, all going false at +1: `manager.go:172-177` (*"the **three** ssl.* counters … the **three** pointers stay NIL"*) · `manager.go:358-373` (`registerListenerMetrics`'s doc comment, *"The **three** phase-74 ssl.* counters"*) · `internal/stats/name.go:448` (*"Of the **14** entries"* — **self-referentially wrong** the moment a 15th lands) · `quic_test.go:295` (*"all **three** ssl.* counters are STILL ZERO"*).
- **D5. The "THIRD internal mis-pointer in ADR-0296" ordinal is NOT ESTABLISHED.** ADR-0296's own §Decision preamble (`:17286`) names three §Context defects the IMPL corrected — *"the ¶6 ctx premise, the STATUS pointer, and ¶3's self-refuting grep"* — of which only **one** is a *pointer*. On a strict mis-pointer count, §Decision (g)'s `§Context ¶7` is the **SECOND**. There is also a **fourth, never-corrected** defect: ¶7's `internal/stats/registry.go:107` is a mis-cite (`:107` is the DUPLICATE-registration panic; the INVALID-NAME panic is `:117`), already corrected at `BEHAVIOR_CONTRACT.md:928` but **never inside ADR-0296**. ⇒ **ADR-0297 states the defects and does NOT commit to an ordinal.**
- **D6. The `:962` offset figure is right, in the unit it was stated.** `ssl.no_certificate` is the **sole** occurrence in `BEHAVIOR_CONTRACT.md` (`grep -c` ⇒ 1, line 962). Offset **627** is correct as a **0-based CHARACTER index**; the **0-based BYTE offset is 630** (the line is 1002 chars / 1007 bytes). **The BRAINSTORM's figure HOLDS.** *(⚠️ §16 records that this SPEC's controller first reported the figure as "off by 3" after measuring bytes and comparing against characters — a unit error, corrected on re-derivation rather than quietly amended.)*
- **D7. The eager-registration posture is now EXECUTED, not assumed.** On the reference, `ssl.no_certificate` was **PRESENT in every single arm across both probe fleets, including every zero case** — it is registered at TLS-context creation, never lazily on first increment. The **dynamic** families behave oppositely: `ssl.versions.*` / `ciphers.*` / `curves.*` / `sigalgs.*` are **absent entirely** until a handshake produces the value. Two further presence facts: `ssl.certificate.unnamed_ca_cert_<hash>.…` appears **only** when a `validation_context` with `trusted_ca` is configured, and `ssl.sigalgs.*` appears **only** when a client cert was actually presented.

### 1.2 SPEC-time verification record — what was EXECUTED and what was NOT

Per `reference_quoting_is_not_executing` and `reference_verification_table_launders_wrong_cites`.

**EXECUTED against the reference** (`envoyproxy/envoy:contrib-v1.37.2`, **fresh container per arm**, bridge networks, **disjoint port ranges / container prefixes / private scratch per fleet** (`reference_parallel_subagents_private_scratch`), a positive control in every bootstrap, and **each arm's discrimination criterion stated BEFORE the run**): D-TLSNC-SEMANTICS (§3.1, ARM-1 ×2 fresh containers + a wire-level `-msg` trace + an HCM `direct_response` cross-check + TLS 1.2/1.3) · D-TLSNC-CERTPRESENT (§3.2) · D-TLSNC-FAILPATH (§3.3, six failure arms incl. the same-scope success+failure discriminator) · D-TLSNC-SCOPE (§3.4, the two-listener single-process discriminator) · D-TLSNC-FIXTURE (§3.5, `0110`'s exact three-arm structure replayed against the reference) · the dead-upstream hazard (§3.6, seven single-variable arms including the **blackhole** discriminator).

**EXECUTED Go-side / in-tree at this tip:** `go build ./...` and `go vet ./internal/listener/... ./internal/stats/...` both clean · `IsValidName` and `NewCounter` against the real name in every address form incl. IPv6 (§5) · `ExtractTags`+`flattenToProm` producing `envoy_listener_ssl_no_certificate` (§5) · `go test ./test/differential/ -run 'TestDifferential/0110-tls-require-client-cert-false' -count=1` ⇒ **PASS** · the same for `0111` ⇒ **PASS**, with its `AssertStats` log lines observed live.

**NOT EXECUTED, stated plainly — the PLAN and IMPL inherit no false confidence:**
- ⚠️ **NO build of row 75 exists, in-tree or in scratch.** The field, the registration line, the predicate, the four guard edits, the two missed guards and the `0110` asserter are all **SPECIFIED and NONE is COMPILED**. Nothing in §3 is evidence for buildability.
- ⚠️ **The `0110` `StatsAsserter` was not written and not run.** Its shape is specified (§8) and its expected values are confirmed **reference-side only**; the **subject side has never emitted this counter** because it does not exist yet.
- ⚠️ **The absolute stat total `1204` was NOT re-derived mechanically.** There is no mechanical command (the contract says so), and ADR-0296's own ledger entry warns that the chain has an unattributed `+1` gap and that *"a future phase that needs an authoritative absolute total should re-derive it mechanically rather than trusting this chain."* **This SPEC asserts the DELTA (+1) with confidence and the TOTAL (1204 → 1205) on documentary grounds only.**
- **Session resumption** — `ssl.session_reused` stayed 0 in every arm. Whether `no_certificate` fires on a **resumed** session is **untested**, and it is a live question for the predicate (a resumed TLS 1.3 session may carry no peer certificate in `ConnectionState`). §13.
- **QUIC / HTTP-3 downstream** — never probed for this name. §3.7 records why the inherited posture is nonetheless correct and what remains unverified.
- **`require_client_certificate: true` with no `validation_context`** on the reference — never attempted; whether it boot-rejects is unknown.
- **`/stats/prometheus` cross-check on the probe fleets** — every probe scraped the plain `/stats` text endpoint. Every probe listener bound `0.0.0.0`, so the dots / IPv6-bracket scope divergence was **not exercised** by the probes. *(It IS exercised by the landed `0111` asserter, which is why §8 keys on the label-stripped name.)*
- **envoy-go's own emission of this counter** — no subject-side measurement exists.

---

## 2. Non-purposes (deferred; BRAINSTORM §1.2 + §8)

- **`ssl.connection_error`** — the cheapest identified follow-on; blocker RETIRED at the BRAINSTORM, cost RAISED. §13.
- **The other TEN fixed reference `ssl.*` names** and the FOUR dynamic families (`ciphers`/`versions`/`curves`/`sigalgs`). §13.
- **`Listener.stat_prefix`**, the IPv6-bracket and dot-normalization scope divergences, the stat-surface ledger gap, `0108`'s stale prose (D3), the `upstream_cluster` span tag, `fault.abort.grpc_status`, the Runtime family, and everything in the BRAINSTORM §8 roster. §13.

---

## 3. The change — the D-TLSNC-* docket disposed one-for-one

### 3.1 D-TLSNC-SEMANTICS **[RESOLVED BY EXECUTION — reading (a), UNCONDITIONAL]**

**The question.** Does the reference increment `ssl.no_certificate` on a plain one-way-TLS listener with **no `validation_context` at all** (`ClientAuth == NoClientCert`, where a certificate is never even requested)? **(a) unconditional** ⇒ the Inc gate is `len(PeerCertificates) == 0` alone. **(b) request-gated** ⇒ the gate needs the chain's client-auth mode too.

**Criterion, stated before the run:** (a) predicts `1`; (b) predicts `0`-or-absent.

**ARM-1 — a `DownstreamTlsContext` carrying ONLY `tls_certificates`; no `validation_context`, no `require_client_certificate`. Client presents no certificate. Two fresh containers, byte-identical results:**

```
listener.0.0.0.0_10000.ssl.handshake: 1          <- positive control HELD
listener.0.0.0.0_10000.ssl.no_certificate: 1     <- THE ANSWER
listener.0.0.0.0_10000.ssl.fail_verify_error: 0
listener.0.0.0.0_10000.ssl.fail_verify_no_cert: 0
listener.0.0.0.0_10000.ssl.connection_error: 0
```

⇒ **Reading (b) is REFUTED. The semantics are UNCONDITIONAL.**

**The solicitation premise is proven at the WIRE, not inferred.** An `openssl s_client -msg` trace of ARM-1's handshake shows the server's entire flight:

```
>>> ClientHello   <<< ServerHello   <<< EncryptedExtensions
<<< Certificate   <<< CertificateVerify   <<< Finished   >>> Finished
```

`grep -ci 'CertificateRequest|Certificate Request|Acceptable client certificate CA names'` ⇒ **0**. The client did not *decline* — **the client was never asked**, and the counter fired anyway. *(Contrast ARM-2/ARM-3, where `<<< CertificateRequest` and `Acceptable client certificate CA names` both appear.)* **This is the discrimination `reference_probe_must_discriminate` demands: an input consistent with only ONE hypothesis.**

**Three further confirmations, each removing a different confound:**
1. **Filter-stack independence** — the same TLS shape behind an HCM `direct_response` listener with **`clusters: []`** (no cluster at all) also gives `handshake: 1`, `no_certificate: 1`. `tcp_proxy` is not implicated.
2. **TLS-version independence** — forced `-tls1_2` and forced `-tls1_3` arms both give `no_certificate: 1`.
3. **Independent arrival by a second fleet** — the FAILPATH fleet's control listener (`PC0`, and again in its `S2` two-listener arm) used the identical no-`validation_context` shape and independently read `no_certificate: 1`. `grep -c 'validation_context'` over both of its bootstraps ⇒ **0**, verified by the controller. **Two fleets, disjoint scratch and ports, reached the same answer without coordination.**

**⇒ THE PREDICATE IS PINNED:**

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

**No client-auth term. No new gate. No new classifier. No new registration function.**

⚠️ **A CONSEQUENCE THE BRAINSTORM ANTICIPATED AND THE PLAN MUST HONOUR:** unconditional semantics mean the counter fires on **every** envoy-go TLS listener whose peer presents no cert — including one-way-TLS listeners with no client-auth config at all. The BRAINSTORM put this at *"the 9 fixtures carrying a downstream TLS context."* **No landed fixture asserts this name, so nothing breaks** — but §10 requires the PLAN to re-derive that fixture set at the IMPL tip rather than trusting the figure 9, and to run the FULL differential package, not just `0110`/`0111`.

⚠️ **`BEHAVIOR_CONTRACT.md:962` LEANED (a) BUT COULD NOT HAVE SETTLED IT.** Its 9-cell phase-67 probe used an **anchorless `validation_context: {}`** in which *"the reference … never sends a CertificateRequest in ANY cell."* Every cell therefore sat on one side of the (a)/(b) boundary — **a non-discriminating input for this question**, exactly as the BRAINSTORM said. It is now corroborated, not relied upon.

### 3.2 D-TLSNC-CERTPRESENT **[RESOLVED BY EXECUTION — the predicted 0 CONFIRMED]**

`require_client_certificate: false` + `trusted_ca`; the client presents a **trusted** certificate; connection accepted (HTTP 200 through the proxy):

```
listener.0.0.0.0_10000.ssl.handshake: 1
listener.0.0.0.0_10000.ssl.no_certificate: 0     <- as predicted
listener.0.0.0.0_10000.ssl.sigalgs.rsa_pss_rsae_sha256: 1
```

The client genuinely sent a certificate — the wire trace shows a real `>>> Certificate` message of 0x649 bytes (contrast ARM-2's **empty** 0x0008 `Certificate`, sent in response to a `CertificateRequest` by a client with nothing to offer, where `no_certificate` **did** fire). **The `sigalgs` line is an independent corroborator: it appears only when a client cert was actually presented.**

### 3.3 D-TLSNC-FAILPATH **[RESOLVED BY EXECUTION — it does NOT move on a failed handshake]**

Six arms. `ssl.no_certificate` read **0** in every one:

| arm | config | client | result |
|---|---|---|---|
| F1 | `require=true` + trusted CA | no cert → `alert certificate required` | `fail_verify_no_cert:1`, `handshake:0`, **`no_certificate:0`** |
| **F1b** | `require=true` + trusted CA | good cert (**succeeds**) **then** no cert (**fails**), same scope | `handshake:1`, `fail_verify_no_cert:1`, **`no_certificate:0`** |
| F2 | `require=true` + trusted CA | untrusted cert → `alert unknown ca` | `fail_verify_error:1`, **`no_certificate:0`** |
| F3 | `require=false` + validation_context | untrusted cert → rejected (verify-if-given still rejects a bad cert) | `fail_verify_error:1`, `handshake:0`, **`no_certificate:0`** |
| F4 | `require=true` + trusted CA | good cert (control) + plaintext garbage + connect-close | `handshake:1`, `connection_error:1`, **`no_certificate:0`** |
| F4c | — | 3 bare TCP connects, zero bytes | `downstream_cx_total:3`, **entire `ssl.*` block 0** |

⚠️ **The "an all-zero block proves nothing" trap was addressed head-on, three independent ways** — an all-zeros `ssl.*` block is equally consistent with *"did not move"* and *"never wired"*:
1. **In-scope, in-arm:** `fail_verify_no_cert` moved 0→1 on the arm's own scope, so that scope's `ssl.*` block is demonstrably being written during that very connection.
2. **F1b is the sharpest single arm:** a **successful** handshake was driven **first on the same scope**, moving `ssl.handshake` 0→1. The success-path counters are wired on that exact scope and fired — and `no_certificate` still stayed 0.
3. **Process-level:** a control listener in the same process read `handshake:1`, `no_certificate:1` at the same moment.

**F1b also sharpens the semantics beyond "success-path":** the counter is not *"peer presented no cert"* — it is *"**completed** handshake **and** peer presented no cert."* The connection that completed presented a cert (→0); the connection that presented none failed (→0).

**ARM-4 corroborates from the other direction** — three connections against one `require=false` listener (trusted / untrusted / none): `handshake=2`, `no_certificate=1`, `fail_verify_error=1`, `fail_verify_no_cert=0`, `downstream_cx_total=3`. **`ciphers`/`curves`/`versions` all read 2, not 3**, while `sigalgs` reads 1 — the rejected connection contributes to **no** success-path stat. `ssl.no_certificate` sits on **exactly the same population as `ssl.handshake`**.

**Side finding (attribution, settled by F4c):** a bare connect-and-close moves **nothing** in `ssl.*`, not even `connection_error`. So F4's single `connection_error:1` is attributable to the **plaintext-garbage** connection, not the immediate close. **This independently corroborates the BRAINSTORM's `connection_error` boundary** (*"books NOTHING when the peer left without delivering a complete ClientHello"*) from a fleet that was not looking for it. §13.

### 3.4 D-TLSNC-SCOPE **[RESOLVED BY EXECUTION — ABSENT on plaintext, verified PER NAME]**

The discriminating arm is **S2: a plaintext listener and a TLS listener in ONE process**, one connection driven to each. Both scopes counted (`downstream_cx_total: 1` each). The complete `grep -F 'ssl.'` over the whole `/stats`:

```
listener.0.0.0.0_10001.ssl.handshake: 1
listener.0.0.0.0_10001.ssl.no_certificate: 1
listener.0.0.0.0_10001.ssl.<... the full family ...>
                                        <- and NOT ONE ssl.* line for 10002
```

The plaintext scope's full 90-line stat listing carries `downstream_cx_*`, `worker_N.*`, `no_filter_chain_match` and histograms — **no `ssl.` name of any kind**. **ABSENT, not present-and-zero.**

⚠️ **This was verified FOR THIS NAME SPECIFICALLY, not inherited from the family** (which is what the docket asked). It is only meaningful because §1.1 D7 established that the reference registers these names **eagerly at listener-config time** — an untouched TLS listener rendered its full `ssl.*` block at zero *before receiving any traffic*. Absence therefore means non-registration, not an admin-endpoint rendering artifact.

⇒ **envoy-go's existing `rt.tlsMode` gate is EXACT PARITY for this name.** No new gate; no per-name divergence.

### 3.5 D-TLSNC-FIXTURE **[RESOLVED BY EXECUTION — Shape A SOUND, and the predicted numbers CONFIRMED reference-side]**

**Shape A** (scrape once at the end, assert **absolute** counts) is sound on `0110` — nothing pre-moves its `ssl.*`. Five independent lines of evidence:
1. Reference readiness is `wait.ForHTTP("/ready").WithPort("9901/tcp")` (`test/differential/harness.go:133`) — **admin only, never the TLS port**.
2. Subject readiness is stdout-sentinel parsing (`harness.go:59-105`, called at `:266`) — **no dial at all**.
3. `ProbeAdmin` (`driver.go:554`) runs at **step 9**, strictly after both Drives, and does only `GET /ready`.
4. The only polling dialer, `waitTCPDial` (`runner_test.go:2250-2267`), is called **exclusively for subprocess backend ports**. `0110` takes the in-process `fixture.TCPEcho` branch (`runner_test.go:271-280`), which never calls it.
5. **Empirical:** the live `0111` run logged `downstream_cx_total=3` on the reference for a 3-arm fixture — exactly 3, **no spurious docker-proxy accept**.

**The predicted per-side numbers were CONFIRMED by replaying `0110`'s exact three-arm structure against the reference** (`require_client_certificate: false`, trusted CA, **live** upstream):

```
=== baseline ===                     === after the three arms ===
ssl_handshake            0           ssl_handshake            2
ssl_no_certificate       0           ssl_no_certificate       1
ssl_fail_verify_error    0           ssl_fail_verify_error    1
ssl_fail_verify_no_cert  0           ssl_fail_verify_no_cert  0
downstream_cx_total      0           downstream_cx_total      3
```

⚠️ **Arms 1 and 3 DISCRIMINATE: `no_certificate` is 1, NOT 2.** That asymmetry is the row's whole non-vacuity argument, and `0111` (`require=true`) could never have supplied it. **`downstream_cx_total` is deterministically 3.**

⚠️ **`ssl.fail_verify_no_cert == 0` is a SAFE assertion on both sides** — because both sides register eagerly (the reference per D7; envoy-go unconditionally-on-`tlsMode` at `manager.go:378-382`). **But a wanted value of ZERO is exactly where the ABSENT check becomes load-bearing rather than defensive** (§8).

### 3.6 The dead-upstream hazard **[CHARACTERISED — and the two fleets RECONCILED against each other]**

The BRAINSTORM recorded this as *"two observations vs one; the mechanism is NOT established."* It is now established, and **the two fleets initially reached different accounts of it.**

**The observation set (single-variable, everything else identical):**

| upstream for `tcp_proxy` | `ssl.handshake` | `ssl.no_certificate` | dynamic `versions`/`ciphers` | `downstream_cx_total` |
|---|---|---|---|---|
| `127.0.0.1:9901` (live) | 1 | 1 | present | 1 |
| `127.0.0.1:1` (instant ECONNREFUSED) ×4 arms | **0** | **0** | **absent entirely** | 1 |
| **`192.0.2.1:81` (blackhole — equally DEAD, but SLOW to fail), `connect_timeout: 10s`** | **1** | **1** | **present** | 1 |
| no cluster at all (HCM `direct_response`) | 1 | 1 | present | 1 |

Two further observations pin it: a **bare TCP connection sending zero bytes** (never a ClientHello) still moved `cluster.up.upstream_cx_total` 0→1 — so `tcp_proxy` dials the upstream **at accept time, before any TLS byte is exchanged**; and in every instant-refusal arm the client reported `ssl3_read_n: unexpected eof while reading`, i.e. the downstream connection was closed under it **mid-handshake**.

**⇒ IT IS NOT DEADNESS. IT IS FAST FAILURE.** The upstream connect and the downstream handshake run **concurrently**. An *instantly* refused connect tears the downstream connection down before handshake completion, so the completion-logging path never runs and the whole family stays silent. A *slow* failure loses the race and everything fires normally. The blackhole row is the discriminator.

⚠️ **THE CROSS-FLEET RECONCILIATION, recorded because one fleet's account was under-determined.** The fixture fleet independently reproduced the phenomenon and reported the mechanism as *"tcp_proxy … tears the downstream connection down first"* — a **deterministic sequencing** claim. **The controller checked whether that fleet had run the discriminating arm: it had not.** `grep -rl '192.0.2\|blackhole\|connect_timeout: 10' ` over its scratch returns **nothing**; it tested only `127.0.0.1:1`. Its input was consistent with **both** "deadness suppresses" and "fast failure suppresses", so it could not distinguish them — `reference_probe_must_discriminate`, instantiated against a probe fleet rather than a document. **The semantics fleet's blackhole arm settles it, and the correct statement is the race, not the sequencing.**

⚠️ **It is a RACE, therefore in principle load-dependent.** Observed 100% deterministic across four instant-refusal containers (two TLS versions, two client-auth shapes), but no Envoy source was read to confirm the ordering, and `127.0.0.1:1` on loopback is the fastest possible failure. **A fixture must not rely on a particular dead upstream failing fast enough.**

**⇒ THE RULE FOR `0110`, AND FOR EVERY FUTURE `ssl.*` FIXTURE:** the listener must have a **live upstream or no upstream at all**. **`0110` satisfies this on both sides — NOT BLOCKING.** Reference `envoy.yaml:105-116` (`STRICT_DNS` → `host.docker.internal:{{.BackendPort}}`) and subject `envoy-go.yaml:102-111` (`STATIC` → `127.0.0.1:{{.BackendPort}}`) both point at the runner-allocated in-process `TCPEcho` backend (`BackendCount() == 1`, `driver.go:289`; `runner_test.go:271-280`). Structurally mandatory for green, too: `wantObservable` (`driver.go:89`) requires `trusted=ok echo=…` and `none=ok echo=…` from **both** sides, so a byte round trip through `tcp_proxy` to the backend must succeed.

⚠️ **AND A COROLLARY THAT OUTLIVES THIS ROW:** **`downstream_cx_total > 0` is NOT a sufficient decode-ran guard for an `ssl.*` assertion.** Every suppressed arm still incremented it. `0111`'s asserter uses exactly that precondition (`driver.go:672-674`); it is correct there only because `0111` has a live backend. **§8 requires `0110`'s asserter to add a second precondition that is specific to the TLS path.**

### 3.7 QUIC — the phase-74 resolution is INHERITED UNCHANGED, and must not be re-litigated

`registerListenerMetrics` is shared with `quic.go:45`, so the fourth name is registered on a QUIC listener too and stays permanently zero (a QUIC handshake never surfaces at `serveConnection`). Phase 74 established by probe that the reference registers the full `ssl.*` family on a QUIC listener at boot and never moves it ⇒ **eagerly-zero registration is EXACT PARITY**, and gating QUIC out would MANUFACTURE a departure. `quic.go` stays **byte-untouched**; `quic_test.go`'s `want` set gains the fourth entry and its zero-loop then covers it for free (§11 B5).

⚠️ **NOT re-probed at this SPEC for THIS name.** The posture is inherited from phase 74's family-level probe and from §3.4's per-name registration finding. §1.2 lists it as unverified; §13 records it.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules, **0 new production imports**

Every primitive exists. The row consumes: `registerListenerMetrics`'s `rt.tlsMode` gate · the `prefix` variable · `stats.NewCounter` · the SN3 flattening · `helpText` · `0111`'s `AssertStats`/`scrapeProm` template · `0110`'s existing three-arm PKI and admin plumbing.

`crypto/tls` (as `stdtls`, `manager.go:5`) and `internal/stats` are **already imported** by `manager.go`, and `ConnectionState()` needs nothing further. `internal/stats/name.go` gains a map entry, not an import.

⚠️ **The "+0 production imports" gate command phase 74 used is UNRELIABLE** (BRAINSTORM §10): `git diff master -- … | grep -E '^\+' | grep -E '^\+\s*(_|[a-z]+ )?"'` returns false-positive hits on map-literal lines and exits 0, so gating on the exit code reads a PASS as a FAIL. **Use hunk headers plus a direct extracted-import-block diff.** ⚠️ **TEST-side imports WILL grow** — `0110`'s asserter needs `log`, `math`, `net/http`, `strconv`, none currently imported by its driver. **That is PERMITTED; the +0 claim is a PRODUCTION claim and the two categories must be audited SEPARATELY.**

---

## 5. Identifier hygiene *(collision checks RE-DERIVED repo-wide at tip, `reference_spec_drafted_identifier_collision_check`)*

- **Field `sslNoCertificate`** — `grep -rn 'sslNoCertificate' --include='*.go' .` ⇒ **0 hits**. No collision. Matches the landed convention (`sslHandshake`, `sslFailVerifyError`, `sslFailVerifyNoCert`).
- **Stat name `listener.<addr>.ssl.no_certificate`** — `grep -rn 'no_certificate' --include='*.go' internal/` ⇒ **0 hits**. The name is new to the Go tree.
- **⚠️ `stats.IsValidName` PASSES — EXECUTED, not reasoned.** `NamePattern` is `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (`internal/stats/registry.go:48`). Run against a byte-identical (sha256-verified) copy of the real source:
  ```
  IsValidName("ssl.no_certificate")                          = true
  IsValidName("listener.127_0_0_1_10000.ssl.no_certificate") = true
  IsValidName("listener.0_0_0_0_10000.ssl.no_certificate")   = true
  IsValidName("listener.___45259.ssl.no_certificate")        = true   <- IPv6 "[::]:45259"
  IsValidName("listener.__1_8080.ssl.no_certificate")        = true   <- IPv6 "[::1]:8080"
  NewCounter("listener.127_0_0_1_10000.ssl.no_certificate")  OK, ptr non-nil = true
  ```
  **No charset hazard, no panic** (`reference_dynamic_stat_name_charset_guard`). ⚠️ Registration uses plain `NewCounter`, **not** `NewCounterIfAbsent` — it panics on an invalid name (`registry.go:117`) **and** on a duplicate (`registry.go:107`). Both are avoided; the name is fixed, valid, and registered exactly once per listener.
- **Prometheus form — EXECUTED.** `ExtractTags("listener.127_0_0_1_10000.ssl.no_certificate")` ⇒ residual `listener.ssl.no_certificate`, label `envoy_listener_address="127_0_0_1_10000"`; `flattenToProm` ⇒ **`envoy_listener_ssl_no_certificate`**. That is the exact `helpText` key.

---

## 6. Reject roster — **UNCHANGED, and that is the point**

The row consumes **no new config field**. There is no parse arm, no boot-reject, no fuzz-seed classification to flip (`reference_fuzzer_count_docs_drift`: an `f.Add` seed is not a fuzzer). Fuzzers stay **55**. `internal/bootstrap/` is byte-untouched.

---

## 7. Stat surface **+1** (1204 → 1205) · Fuzz **+0**

A **NAME-surface** delta, not a flat per-deployment one: registration is TLS-chains-only, so a **plaintext-only** deployment gains **ZERO** names (pinned by the two plaintext guards) while a **QUIC** listener gains it permanently-zero (parity, §3.7).

**The ledger edit map is THREE sites, not one** (§1.1 R3):
- **`BEHAVIOR_CONTRACT.md:831`** — `Stat surface UNCHANGED at **1204**` → **1205**.
- **`BEHAVIOR_CONTRACT.md:847`** — `Stat surface UNCHANGED at **1204**` → **1205**.
- **After `:5002`** — append a new ledger line, **arrow `→` U+2192, bold running through the parenthetical to the colon**, matching the TAIL entry's variant (⚠️ **not** `:5000`'s):
  `**Phase 75 — 1204 → 1205 (+1) (the downstream TLS `ssl.no_certificate` success-path annotation):** …`

⚠️ The **unattributed `1200 → 1201` gap** phase 74 RECORDED rather than invented stays recorded. **Do not fabricate a line to close it.** ⚠️ The absolute total is doc-sourced, not counted (§1.2).

---

## 8. Differential fixture — **`0110` EXTENDED, +0 fixtures (119)**

No new directory, no new YAML, no new BackendKind, no new port. `0118` and reference port `10450` stay free. *(⚠️ The next-free reference port is **10450** — max allocated is 10449 at `0113/driver.go:115`; ports are **not** fixture-index aligned. Moot here, load-bearing for the deferred roster.)*

### 8.1 The asserter's shape — `/stats/prometheus`, Shape A, cross-side

Transplant `0111`'s `AssertStats` (`driver.go:655`) + `scrapeProm` (`:739`) into `0110`. Both admin endpoints already exist and are already threaded (`runner_test.go:1348` passes both), so **no plumbing is owed**: subject `envoy-go.yaml:54-56` (runner-allocated), reference `envoy.yaml:57-59` (`9901`, exposed and health-waited by the harness).

`scrapeProm` keys on the metric **name with the entire `{…}` label set stripped**, and values collide-SUM. That is what makes the assertion cross-side viable: the reference's flat name keeps dots (`listener.0.0.0.0_10447.ssl.…`) where envoy-go underscores them, but **both sides hoist the address into the `envoy_listener_address` LABEL** in Prometheus form, leaving the metric name address-free and identical (`reference_listener_stat_scope_cross_side_divergence`).

**The `want` map, from §3.5's executed reference numbers:**

| name | want |
|---|---|
| `envoy_listener_ssl_handshake` | **2** |
| `envoy_listener_ssl_no_certificate` | **1** |
| `envoy_listener_ssl_fail_verify_error` | **1** |
| `envoy_listener_ssl_fail_verify_no_cert` | **0** |

### 8.2 Mandatory guards

- ⚠️ **The ABSENT check MUST stay SEPARATE from the value check**, via the two-value comma-ok lookup plus `continue` that `0111` already implements (`:698-713`). Its own comment names the vacuity: *"a counter that fails to REGISTER reads as 0 == 0 and would pass VACUOUSLY."* **On `0110` this guard is LOAD-BEARING, not defensive** — `fail_verify_no_cert` is asserted as a genuine **0**, where ABSENT and the wanted value are otherwise indistinguishable. **It must be transplanted, not simplified away.**
- ⚠️ **`var _ fixture.StatsAsserter = (*rccfDriver)(nil)` is a TRIPWIRE, not the dispatch mechanism** (`reference_differential_asserter_dispatch`). Dispatch is a **silent type assertion with no `else`, no log, no skip** — `runner_test.go:1347-1349`. A misspelled method, `*testing.T` instead of `fixture.TB`, a returned `error`, or swapped params makes `ok == false` and the whole stats leg vanishes while the compiler, `go vet` and `golangci-lint` all stay quiet. Phase 74 proved this live. The `var _` line converts that silent failure into a compile error.
- ⚠️ **A SECOND precondition beyond `downstream_cx_total`** (§3.6): that counter increments even when the whole `ssl.*` family is suppressed. Assert something on the TLS path itself — `envoy_listener_ssl_handshake > 0` on both sides before comparing values — so a suppressed run fails loudly instead of reading as four honest zeros.
- **`Errorf` per property, never `Fatalf` except for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`). `0111`'s template already obeys this.
- `-count=1` on every verification run (`reference_differential_break_protocol_count1`), and the full selector `TestDifferential/0110-tls-require-client-cert-false` — `-run '0110'` matches **zero** subtests (`reference_differential_run_selector`).

### 8.3 The transplant delta

Receiver `edfDriver` → `rccfDriver` · the 4-name roster (three sites in the template: the `log.Printf` line, `names`, `want`) · the values above · log prefix `0111` → `0110` and `l_edf` → `l_rccf` · `scrapeProm` copied **verbatim** (no name collision — `0110/driver.go` has no `scrapeProm`) · **+4 test-side imports** (§4) · the arm-count prose (`0110` is "3 accepts / **2** handshake successes / **1** rejection") · and keep the *"must not reach into `sds.*` / `cluster.sds_cluster.*`"* confinement note verbatim — `0110`'s `DriveSubject` (`:334-341`) also hard-stops both SDS receivers before step 10.

### 8.4 The self-confessed boundary notes — **THREE sites in `0110`, not one**

The BRAINSTORM knew of one. A whole-fixture sweep found three:

1. **`README.md:160-163`** — *"**No `ssl.*` stats** — envoy-go emits none, so a verdict `StatsAsserter` is infeasible (inherits PLAN-65 C3); a pre-existing framework gap. Never assert `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as listener liveness."* ⚠️ **BUNDLED**: the `ssl.*` half went FALSE at the phase-74 IMPL; the `/listeners` half is **LIVE and correct**. **SPLIT the bullet; do not delete it.**
2. **`envoy.yaml:24`** — `# identity and NOT a stat (envoy-go emits no ssl.* family; see README).` — single-clause, stale, no bundling.
3. **`expectations.yaml:166-171`** — the same two claims **plus a THIRD**: *"The accept/reject CONTRAST discharges the proof obligation and is strictly STRONGER than a subject-only stat — it is cross-side."* ⚠️ **That third clause also inverts** once the fixture asserts stats cross-side. The rewrite must handle all three claims, not two.

**`driver/driver.go` carries ZERO stale claims** — which is why `0110` has three sites where `0111` had four. **Not stale, do not touch:** `README.md:164-165` and `expectations.yaml:173-175` (the `sds.<secret>.*` counters, correctly still unasserted — `0111` kept its equivalent).

**The `0111` fixed text is the template** for both forms: `0111/README.md:168-175` (the correct split — retire the first half, preserve the `/listeners` guard verbatim in the same bullet) and `0111/expectations.yaml:196-201` (the YAML-comment form).

---

## 9. Behavior-contract edit map — pinned

`grep -n 'ssl\.handshake' docs/envoy-go/BEHAVIOR_CONTRACT.md` ⇒ **916, 928, 1851, 1857, 1859, 5002** — that is the full roster naming the phase-74 triple, and therefore the full phase-75 edit surface.

| site | edit |
|---|---|
| **`:1851`** | **PRIMARY** — the canonical roster (internal + Prometheus forms, gate, increment site) under the `:1849` heading. `ssl.no_certificate` prose belongs here first, with a cross-reference to `:962`. |
| `:1853` | the semantics block — add the success-path predicate and, explicitly, **how it differs from `fail_verify_no_cert` in BOTH directions**. |
| `:1857` | QUIC-is-parity — the fourth name inherits the registered-and-permanently-zero posture. |
| `:1859` | the cross-side scope divergences — `envoy_listener_ssl_no_certificate` joins the label-keyed `/stats/prometheus` recipe. |
| `:928` | the C3 RETIRED/SURVIVING split — the retired half's name list grows to four. |
| `:916` | check for a roster to extend. |
| `:831`, `:847` | ⚠️ the two bare `**1204**` totals → **1205** (§7). |
| after `:5002` | the new ledger line (§7). |

⚠️ `:1849`'s heading reads `### Downstream TLS handshake-outcome stats (per phase 74, ADR-0296)`. **No two-ADR heading precedent exists in this file** — leave the heading and record the addition at `:1851`. ⚠️ **There is NO `B5` step in this document** — see §11 E.

---

## 10. Test plan + task surface *(a SINGLE FLAT ROW; the ADR-0045 valve is armable but not anticipated to fire)*

**~9-11 tasks** — up from the BRAINSTORM's 7-9, because §11 found three missed guards and this section adds a positive unit arm the BRAINSTORM's spine did not name.

1. Field + registration + the `helpText` entry (**two production files**).
2. The guarded `Inc` at `manager.go:1277` (§3.1's exact predicate).
3. **A NEW positive unit test for the counter** — see below.
4. `TestListenerMetrics_TLSListenerRegistersExactlyThreeSSLNames`: `want` entry **in SORTED position** (4th, after `ssl.handshake`), **RENAME** to `…FourSSLNames`, and its doc comment.
5. `TestListenerMetrics_GateMatchesInc`: **+2 pointer assertions** — the load-bearing half.
6. `assertSSLCrossProduct` + the QUIC `want` set + `TestHelpText_ListenerSSLHandshakeOutcomes`'s `wantNames`.
7. The `0110` `AssertStats` + `scrapeProm` + the `var _` tripwire.
8. The three `0110` boundary-note rewrites (§8.4).
9. Docs: BEHAVIOR_CONTRACT (§9), ADR-0297 §Decision + §Consequences **in place**, the ADR-0296 §Decision (g) correction blockquote, the D4 stale-prose cluster.
10. The six-gate + the row flip + the deferred-sentence narrow (§12).

### 10.1 ⚠️ The positive arm — a gap in the BRAINSTORM's spine, and it is CONSTRUCTIBLE

**Every landed increment test asserts this counter does NOT move; none would assert that it DOES.** `assertSSLCrossProduct` (`manager_test.go:4474-4488`) checks one named suffix is 1 and all others are 0 — so extending its `:4480` roster only ever adds a **negative**. Without a positive arm, a `sslNoCertificate` field that is registered but **never Inc'd** passes every unit guard.

**It is safe to extend that roster.** Controller-verified per arm: the `handshake` arm presents a trusted cert (`Certificates: []stdtls.Certificate{pki.clientTrusted}`, `manager_test.go:4498-4503`) ⇒ `PeerCertificates` non-empty ⇒ 0; the two failure arms never reach the success path ⇒ 0. **The contradiction the roster sweep warned of does not materialise for the three existing arms** — but it *would* for a success-with-no-cert arm, where `handshake` and `no_certificate` are **legitimately non-zero together**. ⇒ **`assertSSLCrossProduct`'s SHAPE, not just its roster, must accommodate two expected-non-zero counters.**

**The positive arm needs a non-mandatory-mTLS listener, and one is cheap.** `startMutualTLSListener` (`:4437`) is the only registry-returning TLS helper and it builds **mandatory** mTLS (`mkDownstreamTSMutualTLS`, `RequireClientCertificate: true`) — under which a no-cert client **fails**, so `no_certificate` stays 0. But **`mkDownstreamTSInline` (`manager_test.go:627`) already builds a ONE-WAY TLS transport socket — cert only, no validation context: exactly the §3.1 ARM-1 shape proven to fire the counter.** ⇒ the PLAN adds a ~20-line sibling helper (`startOneWayTLSListener`) mirroring `:4437` with `mkDownstreamTSInline` + `startEchoBackend`, dials with no client cert, and asserts `ssl.handshake == 1` **and** `ssl.no_certificate == 1`. **No new PKI, no new TS builder.**

⚠️ **This arm is also the row's DISCRIMINATING break.** Deleting the `len(…) == 0` guard (making the Inc unconditional) leaves every existing test green and fails only this one. **A break that merely deletes the registration fires the ABSENT check, not the value check** — the two must be demonstrated separately (`reference_deliberate_break_wrong_assertion`).

⚠️ **Breaks run AFTER committing** (`reference_break_protocol_commit_first`), with `-count=1`, and a break INSTRUCTION that does not compile proves nothing (`reference_plan_break_instructions_dont_compile`).

⚠️ **`net.Pipe()` is unusable** for any client-cert handshake test — it deadlocks silently to the test timeout (`reference_netpipe_deadlocks_client_cert_handshake`). Use a loopback TCP pair, as the landed helpers do. ⚠️ **`io.Copy(conn, conn)` is not an echo server** (`reference_iocopy_self_splice_echo_backend`); `startEchoBackend` already exists.

⚠️ **A nil `*stats.Counter`.Inc is a PROCESS CRASH** — no nil guard in `Inc`, no `recover()` on the `serveConnection` goroutine (`reference_nil_stats_counter_inc_crashes_goroutine`). **The +2 pointer assertions in task 5 are mandatory, not stylistic.**

---

## 11. Edit-site roster — RE-DERIVED at tip

**Production (TWO files):**

| site | anchor | edit |
|---|---|---|
| counter field | `manager.go:180-182` (struct `listenerRuntime`) | +1 `sslNoCertificate *stats.Counter` |
| registration | `manager.go:378-382`, calls `:379-381` | +1 `r.NewCounter(prefix + "ssl.no_certificate")` |
| increment | `manager.go:1277` | +3 guarded on `len(tlsConn.ConnectionState().PeerCertificates) == 0` |
| **HELP text** | `internal/stats/name.go:469-471` (map literal, `:456-472`, **14 entries**) | **+1 `"envoy_listener_ssl_no_certificate"`** |

⚠️ **The `helpText` entry is MANDATORY and its guard will NOT catch you.** Without it `prom.go` degrades HELP to the metric name. `TestHelpText_ListenerSSLHandshakeOutcomes` asserts exactly that degradation signature — **but iterates a HAND-LISTED three-name slice and only ever reads `helpText[n]` for `n` in it.** There is no reverse direction; `TestHelpText_Coverage` (`name_test.go:195-213`) is a second hand-listed roster of the same shape. **CONFIRMED TRUE: a fourth name added without extending `wantNames` is silently unguarded.**

**Guards — the BRAINSTORM's roster, all EXACT:**

| guard | anchor | what +1 requires |
|---|---|---|
| `…RegistersExactlyThreeSSLNames` | `manager_test.go:2023` (`want` `:2049-2053`) | +1 entry **sorted 4th**; **RENAME** + doc comment. Sortedness enforced by `listenerSSLNames`'s `sort.Strings` (`:1998-2008`) |
| `TestListenerMetrics_GateMatchesInc` | `manager_test.go:2128-2251` | **+2** of the existing **6** pointer assertions (3 non-nil at `:2190-2199`, 3 nil at `:2240-2249`) — `Errorf` per assertion, so all fire independently |
| `…PlaintextListenerRegistersNoSSLNames` | `manager_test.go:2076` (`:2098`, `len(got) != 0`) | nothing |
| `…AllocatesBaseListenerMetrics` | `manager_test.go:1951` (`:1971`) | nothing |
| QUIC name-set guard | `quic_test.go:274-278` (test `:249`) | +1 `want` entry; **the zero-loop at `:296-300` ranges over `want`, so it covers the new name for free.** `counterValue` `Errorf`s on an ABSENT name, so the loop is non-vacuous |
| `TestHelpText_ListenerSSLHandshakeOutcomes` | `name_test.go:230-235` | +1 `wantNames` entry — **or silently unguarded** |

**⚠️ THREE GUARDS THE ROSTER MISSED (D2):**

| guard | anchor | what +1 requires |
|---|---|---|
| **`assertSSLCrossProduct`** | `manager_test.go:4474-4488`, hard-coded 3-leaf negative roster at `:4480`; callers `:4493`, `:4519`, `:4544` | **+1 roster entry AND a shape change** (§10.1). Without it, every existing arm silently stops asserting `no_certificate` did not move — the exact discrimination failure the helper exists to prevent |
| `…PlaintextListenerIncrementsNoSSL` | `manager_test.go:4568` (`:4619`, `len(got) != 0`) | nothing — but it is a **fourth** plaintext guard the roster did not enumerate |
| **`0111`'s cross-side `AssertStats`** | `0111/driver.go:655`; **THREE** hard-coded rosters — the `log.Printf` at `:682-684`, `names` at `:687-691`, `want` at `:692-696`; plus prose at `README.md:168-172` and `expectations.yaml:122-124,159,196-212` | Under §3.1's unconditional semantics `0111` (`require=true`, success arm presents a cert) reads `no_certificate == 0`, so **no value changes** — but the PLAN should decide explicitly whether to add it as a 4th asserted **zero**. **Not deciding is how a roster goes stale.** |

**Swept and CLEAN:** no other listener-metric registration site (`registerListenerMetrics` has exactly two production callers, `manager.go:1074` and `quic.go:45`) · the five `TestNoNewStat*` guards (`internal/statssink/registration_test.go`) never boot a listener · no golden file or testdata enumerates listener stat names · the `internal/filter/http/lua/ssl.go` hits are the unrelated Lua `Connection:ssl()` surface.

**⚠️ E — the phantom `B5`, and the account is now sharper than "points at nothing".** `manager.go:392` and `:1265` both read `// … a NAMED DEPARTURE (ADR-0296, BEHAVIOR_CONTRACT B5).` — the only two `BEHAVIOR_CONTRACT` mentions in that file. All **eight** `B5` hits in `BEHAVIOR_CONTRACT.md` are `AMEND-B5`/phase-25.2 Wasm; **the document has no B-numbered step scheme at all**, and the departure actually lives at `:1855` under the `:1849` heading. **But `B5` is not invented:** it is a **phase-74 PLAN edit-bundle label**, defined at `phases/74-…/SPEC.md:304` and `PLAN.md:133`/`:896`, and the two production comments were **pre-authored verbatim in the PLAN** (`PLAN.md:325`, `:684`) with the string already baked in. It then propagated into `DECISIONS.md:17304`/`:17314` (*"BEHAVIOR_CONTRACT B5/B6"*) and `STATE.md`. **A PLAN-internal task label acquired ADR authority and landed in production code** — `reference_code_comment_not_evidence`, newly instantiated **in production**. Correct target: `BEHAVIOR_CONTRACT.md:1855`. **Recorded, NOT fixed at this SPEC** (§13) — it is a docs/comment correction outside this row's delta, and ADR-0297 §Context records it.

---

## 12. Sentinel maintenance — a SENTENCE-NARROWING row, narrowed AT THE IMPL

**NO deferred sentence is narrowed at ANY pre-IMPL stage** (the phase-57 precedent). **ROADMAP is BYTE-UNTOUCHED at this SPEC and row 75 STAYS `in-progress`.**

The three live sentences are at `ROADMAP.md:185` (HTTP/3), `:195` (xDS), `:205` (Observability). ⚠️ Use the canonical `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` ⇒ **3**; a naive `grep -c 'candidates:'` returns more, and the **four** `candidates were:` recaps inside `:205` are HISTORICAL (`reference_sentinel_deferred_sentence_live_vs_historical`). Occurrence-counting (`grep -oE … | wc -l`) also returns **3**, so no huge line hides a second live sentence.

⚠️ **The Observability sentence does NOT currently name `no_certificate`** — it names *"the DYNAMIC half"*, *"the uncounted non-certificate handshake-failure bucket (`connection_error`…)"* and the tracing remainders. **So this row's IMPL narrow is NOT a name deletion.** The PLAN must derive the exact IMPL edit from the sentence's actual text and **verify mechanically in a scratch copy** that check (2) still returns **3** and that the sentence still terminates at `force-trace.` so the `[^.]*\.` regex still binds.

---

## 13. Deferred items

**Newly surfaced by THIS SPEC, none chartered:**

- **`ssl.connection_error`** — the BRAINSTORM's boundary is now **independently corroborated by a second fleet**: a bare connect-and-close moves **nothing**, not even `connection_error` (§3.3, F4c), while plaintext garbage moves it. The over-count hazard and the forced new fixture stand. ~9-11 tasks.
- **The dead-upstream / fast-failure RACE** (§3.6) — characterised, not fixed. It is reference-side behaviour, not an envoy-go defect, but it is a standing precondition on every future `ssl.*` fixture, and the "`downstream_cx_total` is not a sufficient decode-ran guard" corollary applies to `0111`'s landed asserter too.
- **`0108-xds-sds-validation-context/expectations.yaml:104-106`** — a fourth stale *"envoy-go emits NO `ssl.*` stats whatsoever"* confession, outside this row's fixture (D3). Prose only, no assertion.
- **The `B5` phantom in two production code comments** (§11 E) plus its propagation into `DECISIONS.md` and `STATE.md`.
- **The D4 stale-prose cluster** — `manager.go:172-177`, `:358-373`, `name.go:448` (*"Of the 14 entries"*), `quic_test.go:295`. These go false **at this row** and so are IN scope for the IMPL (§10 task 9), unlike the items above.
- **`ssl.no_certificate` on a RESUMED TLS 1.3 session** — never probed; a resumed session may carry no peer certificate in `ConnectionState`, which would make the counter fire on a connection whose original handshake presented one (§1.2).
- **QUIC's reference behaviour for this name specifically** — inherited from phase 74's family probe, not re-probed (§3.7).
- **The stat-surface absolute total** — never mechanically re-derived; ADR-0296's own ledger entry asks a future phase to count it (§1.2, §7).
- **ADR-0296's uncorrected `registry.go:107` mis-cite in §Context ¶7** (D5) — corrected at `BEHAVIOR_CONTRACT.md:928`, never inside the ADR.

**Carried from the BRAINSTORM, costs as re-derived there:** `fault.abort.grpc_status` (**the recommended NEXT opening**, 7-9 tasks, the only identified candidate that clears a sentinel check-(3) blocker; ONE probe owed) · the `upstream_cluster` span tag (no longer blocked, ~25 lines / 7-9 tasks) · `Listener.stat_prefix` (~8) · the Runtime family opening (9-11, +1 package) · `stats_flush_on_admin` (8-10) · `hcm.access_log_options` (9-12) · HCM `server_name`/`via` (10-14) · the `stdout`/`stderr` loggers (10-12) · `hcm.merge_slashes` (11-14, carrying the pre-existing H2 `url.Parse` routing bug) · the other TEN fixed `ssl.*` names and the FOUR dynamic families · all tracing remainders · all xDS · all HTTP/3 · gRPC · Runtime · **WASM (a ROADMAP bookkeeping artifact, deliberately left as-is)**.

### 13.1 Deferred-item hygiene

Every item above is recorded HERE and in ADR-0297 §Context. **None is added to the ROADMAP `candidates:` sentence at this stage** (§12). ⚠️ **This is exactly the hygiene claim ADR-0296 §Decision (g) caught its own SPEC making falsely** — so it is stated as a *commitment the IMPL must verify by grep*, not as an accomplished fact.

---

## 14. ADR continuity — the ADR-0297 §Context DRAFT (anchored here; full entry at the IMPL)

Per **ADR-0044-as-used** (⚠️ D1: a convention with no written ADR — 532 citations, 0 supporting tokens in ADR-0044's own block), **ADR-0297's §Context lands at THIS SPEC commit** — the DECISIONS tail flips ADR-0296 → **ADR-0297** here, and the §Context append **IS** that tail flip. §Decision + §Consequences append **IN PLACE** at the phase-75 IMPL, **no renumber**; next-free becomes **ADR-0298**.

**Block shape, pinned by what ACTUALLY LANDED at ADR-0295/0296** (recovered from `git show ab13fc19:docs/envoy-go/DECISIONS.md`, the phase-74 SPEC commit): heading line · a `> **STATUS: PROPOSED — §Context drafted at the phase-75 SPEC (ADR-0044-as-used; …).**` blockquote · **exactly ONE** `### Context (drafted at the phase-75 SPEC)` heading · N paragraphs, one per line, blank-separated · the italic footer `*(§Decision + §Consequences land at the phase-75 IMPL.)*`. **NO `### Decision`. NO `### Consequences`. No `---` separator** (the last `^---$` in the file is at `:17020`; separators stopped with the ADR-0289 era). The SPEC uses **`STATUS: PROPOSED`** and future tense; the IMPL rewrites to `COMPLETE` and past tense, **RETAINING the footer** with the appends following it (the ADR-0295/0296 shape, **not** ADR-0286's).

**Verify-target after the append:**
```sh
awk '/^## ADR-0297/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Context'       # 1
awk '/^## ADR-0297/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'      # 0
awk '/^## ADR-0297/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Consequences'  # 0
awk '/^## ADR-0297/,0' docs/envoy-go/DECISIONS.md | grep -c '^\*(§Decision'      # 1
```

**ADR-0297 records:** (a) the SEMANTICS resolution and its wire-level discrimination (§3.1) · (b) the predicate, and why it carries **no** client-auth term · (c) FAILPATH/CERTPRESENT/SCOPE (§3.2-3.4) and the eager-registration finding (D7) · (d) the fast-failure race and its fixture precondition (§3.6), including the cross-fleet reconciliation · (e) the ADR-0296 §Decision (g) correction (§14.1) · (f) the ADR-0044 misattribution at measured scale (D1) · (g) the correction-FORM discriminator refutation (R1) · (h) the three missed guards and the `helpText` silent-staleness class (§11).

### 14.1 ⚠️ The ADR-0296 §Decision (g) correction — the FORM lands at the IMPL, the FINDING is drafted here

ADR-0296 §Decision (g) (`DECISIONS.md:17308`) says: *"envoy-go does not implement `require_client_certificate: false` (verify-if-present) at all — §Context ¶7 / `VERIFYIFGIVEN` is explicitly OUT OF SCOPE — so no `ssl.no_certificate` counter is owed here."*

**BOTH halves fail mechanically, and the SPEC re-derived each independently:**
- **Half one is FALSE.** `clientAuthFor` returns `stdtls.VerifyClientCertIfGiven` at `internal/tls/config.go:79-84`, documented in place at `:60-68`, applied via `installPool` across **all three** validation-source shapes (inline / SDS-VC / CVC). ROADMAP row 67 (`:129`) is **`done`**. Fixture `0110` has driven it since the phase-67 IMPL (commit `92cd1647`).
- **Half two's citation resolves to its own sentence.** `grep -n 'VERIFYIFGIVEN' docs/envoy-go/DECISIONS.md` ⇒ **exactly one hit, `:17308`** — controller-verified. §Context ¶7 (`:17274`) is the four-family **dynamic-split** paragraph; `grep -c 'require_client_certificate'` over it ⇒ **0**.

**⚠️ WHAT SURVIVES, and ADR-0297 must say so first:** *"no `ssl.no_certificate` counter was owed **in phase 74**"* is **CORRECT**. Phase 74's sole stat-asserting fixture was `0111`, whose listener sets `require_client_certificate: true` on both sides (`envoy.yaml:75`, `envoy-go.yaml:70`) — and ADR-0296's own §Context says so verbatim. At `require=true` a no-cert client is rejected, so a success-path annotation necessarily reads 0 on every phase-74 arm. **The conclusion held; the reason did not.** ⇒ **Phrase it as a documentary finding, NOT as criticism of phase 74, whose row was correctly scoped.**

**FORM — the INDENTED blockquote, and the reason is R1's, not the BRAINSTORM's.** Insert after `:17308`, preserving the blank line, as `  > [CORRECTED at phase 75/ADR-0297: …]`. Literal leading bytes **`0x20 0x20 0x3E 0x20`** (two SPACE, `>`, one SPACE), `cat -A`/`od -c`-verified byte-identical across both landed instances (`:16901`, `:16910`), which are the **only** two in the file (`grep -c '^  > '` ⇒ 2). A `###` heading follows at `:17310`, so a blank line **after** the blockquote is required (the bullet-follows precedents have none). **This edit lands at the IMPL, not here** — both precedents landed their correction at the correcting phase's IMPL, and this SPEC's DECISIONS delta is the ADR-0297 §Context append and nothing else. **ADR-0296 is BYTE-UNTOUCHED at this commit.**

---

## 15. Exit — counts + expectations at SPEC-DONE

**Docs-only. ZERO production `.go`.** Exactly TWO files in the delta: this `SPEC.md` + the ADR-0297 §Context append. ROADMAP **BYTE-UNTOUCHED**; row 75 STAYS `in-progress`.

| count | value | command |
|---|---|---|
| fixtures | **119** | `ls -d test/fixtures/[0-9]*/ \| wc -l` |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` |
| stat surface | **1204** | BEHAVIOR_CONTRACT doc count — **NO mechanical command**, and **not re-derived** (§1.2) |
| BackendKind | **38** | tail VALUE (`H2GoawayResponder`, `fixture.go:614`); the file declares **39** constants, 0-38 |
| go.mod modules | **2** (lineage figure) | single `go.mod`, **67** requires |
| DECISIONS tail | **ADR-0296 → ADR-0297** | at THIS commit; next-free **ADR-0298** |
| build / vet | **clean** | `go build ./...`, `go vet ./internal/listener/... ./internal/stats/...` |

**Anticipated at the IMPL, NOT YET LANDED:** +1 stat (**1204 → 1205**) · +0 fixtures (**119**) · +0 fuzzers (**55**) · +0 BackendKinds (**38**) · +0 modules · **+0 production imports** (test-side WILL grow, §4) · ZERO new packages · ZERO new exported symbols.

---

## 16. Adversarial-pass record

**The docket item the BRAINSTORM called "genuinely open" was resolved, and it went the way the weaker evidence leaned — but only a purpose-built probe could establish it.** `BEHAVIOR_CONTRACT.md:962` leaned (a) and was **structurally incapable** of settling it: all nine of its cells sat on one side of the boundary. Had this SPEC accepted the lean, it would have reached the right answer for a reason that does not survive inspection. **The `-msg` wire trace — proving no `CertificateRequest` was ever sent — is the only evidence in the record that discriminates.**

**FOUR anticipations REFUTED, all by re-derivation:**
1. **The correction-form discriminator is the ADR FAMILY** — refuted by the n=4 population; `:16901` is a same-family case using the indented form (R1). The prescribed form survives on a different rule.
2. **`:17209` is the inline-correction precedent** — it is not a correction at all (R2).
3. **`:831`/`:847` are the stat-surface ledger** — the ledger is at `:4950`/`:4954-5002`; those two lines are narrative totals. **Both sets are load-bearing and phase 74's own ADR flagged them** (R3).
4. **The `0110` arm and `ProbeAdmin` cites** point at a doc comment and at the wrong line (R5).

**⚠️ A PROBE FLEET FAILED THE ROW'S OWN DISCIPLINE, and the controller caught it by checking inputs rather than conclusions.** The fixture fleet reproduced the dead-upstream hazard and reported a **deterministic sequencing** mechanism. Its evidence was four arms at `127.0.0.1:1` — all instant-refusal, all consistent with **both** "deadness suppresses" and "fast failure suppresses". The semantics fleet's **blackhole** arm (equally dead, slow to fail) fires the whole family normally and settles it: **it is a race, not a sequence.** `reference_probe_must_discriminate` is usually applied to documents; here it applied to a probe fleet's own experimental design. **A subagent's mechanism claim is not evidence either — the discriminating arm is.**

**⚠️ THE CORRECTION THIS SPEC MADE TO ITS OWN REASONING, recorded rather than quietly replaced.** The controller re-derived the contested `BEHAVIOR_CONTRACT.md:962` offset with `grep -bo`, read **630**, and reported the BRAINSTORM's **627** as "off by 3". **That was a unit error, not a drift finding**: 627 is the 0-based **character** index; 630 is the 0-based **byte** offset, and the line contains multibyte characters. **The BRAINSTORM's figure HOLDS.** Recorded because the phase-75 BRAINSTORM itself adjudicated a *failed* refutation of this exact anchor (an agent claimed the token was absent; it is not) — and the SPEC's controller then produced a second, milder failed correction of the same line. **A drift correction is itself a claim, and this lineage has now generated two false ones against a single anchor.**

**Two documentary findings that mirror the row's own theme one level up:** ADR-0044's discipline is cited **532 times** and appears **zero times** in ADR-0044 (D1); and the phantom `B5` is now traced to its origin — a **phase-74 PLAN edit-bundle label**, pre-authored into the production comments by the PLAN itself and propagated onward into an ADR (§11 E). **The row is about a counter an ADR deferred on a false premise; the SPEC found the same species twice more while verifying it.**

**What NO ONE checked, stated plainly: NO BUILD OF ROW 75 EXISTS.** The field, the registration, the predicate, the four guard edits, the three missed guards, the new positive helper and the `0110` asserter are all SPECIFIED and **none is COMPILED**. Every reference number in §3 and §8 is a **reference-side** measurement; **the subject side has never emitted this counter.** The PLAN must not treat any of §3 as evidence for buildability.
