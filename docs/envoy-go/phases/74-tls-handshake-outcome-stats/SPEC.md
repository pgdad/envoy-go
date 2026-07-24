# SPEC 74 — downstream TLS handshake-outcome stats (`ssl.handshake` / `ssl.fail_verify_error` / `ssl.fail_verify_no_cert`) (the TWENTIETH Observability-family row *(chain ordinal)*; registers THREE fixed-name counters at listener scope, **TLS-listener-gated and TCP-only**, classifying the handshake error ALREADY BOUND at `internal/listener/manager.go:1178`; +3 stats / +0 fixtures / +0 packages / +0 modules / +0 fuzzers / **+0 imports**)

> **Stage:** SPEC. Docs-only. **ZERO production `.go`.** Exactly TWO files in this commit's delta: this `SPEC.md` (new) and the ADR-0296 §Context append to `DECISIONS.md`. **ROADMAP UNTOUCHED.**
>
> **Row 74 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, `reference_roadmap_split_phase_row_done`, §10). **ADR-0296's §Context DRAFTS HERE** (ADR-0044-as-used — ⚠️ and §1.1 item D1 records that this citation is itself misattributed); §Decision/§Consequences land at the IMPL IN PLACE. **The DECISIONS tail flips ADR-0295 → ADR-0296 AT THIS SPEC COMMIT** (§14) — the §Context append IS the tail flip; next-free becomes ADR-0297.
>
> **Evidence base:** EIGHT agents at tip `f5a38c40` — FOUR live-probe fleets against `envoyproxy/envoy:contrib-v1.37.2` on disjoint port ranges and container prefixes (`p74a-`/`p74b-`/`p74c-`/`p74d-`), a Go-toolchain execution agent, two read-only re-derivation agents, and a governance agent — plus controller re-derivation of every load-bearing cite. **~35 EXECUTED probe arms.** §1.2 records what was and was NOT executed.
>
> **Baselines re-verified mechanically at `f5a38c40`:** fixtures **119** · fuzzers **55** · stat surface **1201** · BackendKind **38** · go.mod modules **2** (single `go.mod`, **67** requires: 18 direct / 49 indirect) · DECISIONS tail **ADR-0295** COMPLETE.
>
> **Sentinel expectation at this stage:** does NOT fire; `stop` NOT created. (1) `NOT DONE: row 74` · (2) ⇒ **3** · (3) `NEVER OPENED: gRPC/Runtime/WASM`.

---

## 1. Purpose / Mission

Phase 74 makes downstream TLS handshake outcomes **countable** in envoy-go for the first time. It registers three fixed-name counters under the listener-scope prefix `registerListenerMetrics` already builds, and classifies the `error` value already bound at the handshake site:

- **`ssl.handshake`** — a downstream TLS handshake that COMPLETED SUCCESSFULLY.
- **`ssl.fail_verify_error`** — a client certificate was presented and **chain verification failed**.
- **`ssl.fail_verify_no_cert`** — **no** client certificate was presented where one was required.

The project has shipped mandatory mTLS since phase 16 with **zero** verification-outcome observability. Fixture `0111`'s own README confesses the gap. This row retires it for the three cheap names and **splits off** the blocked dynamic half.

It is a **pure observability addition** — nothing behaves differently after it lands; three previously-invisible outcomes become countable. That is the honest case, and it is a strong one.

### 1.1 BRAINSTORM drift ledger — what the SPEC RE-DERIVED and what it REFUTED

Per `feedback_brief_citations_not_evidence`, every BRAINSTORM §6 anchor was re-opened at this tip. `git diff --stat de2d7737 HEAD` touches only ROADMAP / STATE / the new BRAINSTORM / next-prompt ⇒ **ZERO production `.go` changed since the BRAINSTORM's derivation tip**, so any failing Go cite was **wrong when written**, not drift.

**HOLDS** (personally re-read at tip): `manager.go:141` struct decl (closes `:186`, 18 fields) · `:175-176` the two metric fields · `:341-343` `normalizeAddr` · `:351-355` `registerListenerMetrics` · `:352` prefix · `:353`/`:354` the two names · `:1113` `serveConnection` (`rt` is the **method receiver**; `:1114` already derefs `rt.downstreamCxActive`) · `:1176`/`:1177`/`:1178`/`:1179-1181`/`:1183` · `registry.go:48`/`:107`/`:161`/`:175-176`/`:177-188`/`:208` · `statssink/registration_test.go:26` · `DECISIONS.md:16888`/`:16890`/`:16908` · `BEHAVIOR_CONTRACT.md:916`/`:928` · `fixture.go:75` · `0111` `envoy-go.yaml:70`, `README.md:73`, `driver.go:193`/`:220`/`:246`/`:293`/`:520` · `runner_test.go:138`.

**REFUTED — five, three of them load-bearing:**

- **R1. `quicAcceptLoop` is `internal/listener/quic.go:88`, NOT `:104`.** `:104` is the body line `go rt.serveQUICConnection(ctx, conn)`. (`serveQUICConnection:120` HOLDS.) quic.go is byte-identical to `de2d7737` ⇒ wrong when written.
- **R2. `errors` is ALREADY imported** at `manager.go:6` (`stdtls "crypto/tls"` at `:5`). §6's *"possibly ONE new import line"* is false ⇒ **+0 imports**, not +1.
- **R3. ⚠️ THE STAT SPELLING. `normalizeAddr` underscores DOTS as well as colons** — `strings.NewReplacer(":", "_", ".", "_", "[", "", "]", "")`. envoy-go's full name is **`listener.0_0_0_0_10000.ssl.handshake`**; the reference's is `listener.0.0.0.0_10000.ssl.handshake`. The divergence is DELIBERATE and documented at `manager.go:326-332` (the SN3 extractor uses `strings.Index(tail, ".")`, so dots in the address would truncate the segment to its first IPv4 octet). Corroborated by three landed pins (`stats/registry_test.go:11`, `stats/prom_test.go:22`, `admin/prometheus_test.go:45`). **This decides the fixture asserter's shape (§8).**
- **R4. "`internal/listener/quic.go` is BYTE-UNTOUCHED so QUIC is merely a documentary pin" UNDERSTATES the question.** `quic.go:45` calls the SAME `registerListenerMetrics`, and `quic.go:102-103` Inc BOTH cx metrics (pinned live by `quic_test.go:92-97`). A QUIC listener today HAS listener-scope stats, so the registration posture chosen in `registerListenerMetrics` **silently decides QUIC's surface too** — the file stays byte-untouched while its behaviour changes. That made D-TLSHS-QUIC a real decision rather than a formality, and it took a probe to settle (§3.4). *(The BRAINSTORM's framing was not wrong about the bytes; it was wrong that nothing was at stake.)*
- **R5. ROADMAP anchors DRIFTED** — `### Observability family` heading `:199` → **`:200`**; family paragraph `:203` → **`:202`**; the deferred clause is at **`:204`**. Commit `f5a38c40` shifted the file.

**DOCUMENTARY findings the BRAINSTORM did not have — the row's own theme recurring:**

- **D1. ⚠️ `ADR-0044` DOES NOT SAY WHAT THE PROJECT CITES IT FOR.** ADR-0044 (`DECISIONS.md:1419-1462`) is titled **"BEHAVIOR_CONTRACT HTTP/1.1 subsection"** and is entirely about which HTTP/1.1 equivalence dimensions the differential asserts. A mechanical scan of its whole block for Context-draft/append-at-IMPL language returns **0**. The convention runs under the names *"ADR-0044 ADR-on-impl convention"* / *"§Context-draft discipline"*, articulated only in DOWNSTREAM citations (e.g. `DECISIONS.md:8534`, `:5926`). **This is `reference_brainstorm_adjective_acquires_adr_authority` one level up — an adjective that accrued ADR authority by citation rather than by text.** Phase 73's SPEC already hedged to **"ADR-0044-as-used"** (`SPEC.md:5`, `:387`) while its `:389` STATUS template still says *"per ADR-0044"* unhedged. **This SPEC uses the hedge throughout and ADR-0296 records the finding.** The operative rule AS PRACTICED: §Context is drafted and appended at the SPEC commit — and that append IS the tail flip; §Decision + §Consequences append IN PLACE at the IMPL, no renumber, no new ADR.
- **D2. ⚠️ "a correction ACROSS ADRs, a first for the project" is REFUTED.** THREE precedents exist, in two formats. Decisively, **`DECISIONS.md:16901` is a phase-67/ADR-0289 correction sitting INSIDE ADR-0286 itself** — the very ADR phase 74 must annotate — as an indented blockquote: `  > [CORRECTED at phase 67/ADR-0289: …]`, closing with a sentence stating what still stands. The other two (`:17209`, `:17185`, both ADR-0294) use an INLINE `**[CORRECTED at the phase-NN IMPL: … — see <pointer>]**` form for a clause within a sentence in the same ADR family. **⇒ Phase 74 correcting ADR-0286 C3 is the LATER-PHASE-CORRECTS-EARLIER-ADR case ⇒ use the INDENTED BLOCKQUOTE form, citing `phase 74/ADR-0296`.** §9.
- **D3. C3's roster is FIVE names, not six.** `DECISIONS.md:16908` lists `handshake`/`fail_verify_error`/`fail_verify_no_cert`/`ciphers.*`/`versions.*`. The six-name roster (with the cert-expiry gauge) is from **SPEC-65 §11**, not from C3. The BRAINSTORM conflated them.
- **D4. ⚠️ `BEHAVIOR_CONTRACT.md:962` ALREADY NAMES `ssl.no_certificate`** — the phase-67/ADR-0289 anchorless-`validation_context` departure reads *"accepts every client with `ssl.no_certificate` incrementing"*. **`ssl.no_certificate` appears NOWHERE in the BRAINSTORM.** It was observed live at phase 67 and is unaccounted for by the row's scope. §3.2 / §13.
- **D5. ⚠️ THE STAT-SURFACE LEDGER HAS A HOLE.** `1201` appears in BEHAVIOR_CONTRACT.md at exactly TWO lines (`:831`, `:847`). There is **NO recorded `1200 → 1201` transition** — the ledger reads `1198 → 1200` at `:732`, holds at `1200` through `:815`, then jumps to `1201` at `:831`. The `+1` is attributable on documentary grounds to the tap filter's `tap.rq_tapped` (phase 56.1, `:4155-4161`) but **no ledger line records it**. Three further **stale `1200`** figures survive at `:1429`, `:1463`, `:1495`. §9 pins the real edit map.
- **D6. TWO stale comments in the file phase 74 edits**, both going false at +3: `manager.go:172-174` (*"2 metrics per listener"*) and `:345` (*"allocates the 2 listener-scope metrics"*).
- **D7. A guard §6 omits — `manager_test.go:1928` `TestListenerManager_AllocatesTwoMetricsPerListener`** (doc `:1911-1913`). ⚠️ Its NAME and doc say *"exactly the 2"* but its BODY (`:1947-1948`) only asserts PRESENCE of two suffixes and **never counts** ⇒ **+3 produces NO RED from it** while its name and doc silently go false.
- **D8. BRAINSTORM §11.3 item 4's `\bssl\b` hit list is INCOMPLETE** — it omits production `lua/bridge.go` (10 hits) and `lua/streaminfo.go` (5) plus three test files. The CONCLUSION survives (107 hits, all the Lua/Wasm `Connection:ssl()` surface, **ZERO stat names**, confirmed two further ways); the ENUMERATION does not — the same defect class item 4 was written to fix.

### 1.2 SPEC-time verification record — what was EXECUTED and what was NOT

Per `reference_quoting_is_not_executing` and `reference_verification_table_launders_wrong_cites`.

**EXECUTED against the reference** (`contrib-v1.37.2`, fresh container per arm, bridge networks, positive control in every bootstrap, discrimination criterion written BEFORE each run): D-TLSHS-SCOPE (arms 1/2/3 + IPv6 + `stat_prefix` + `name:` control) · D-TLSHS-SEMANTICS (the 1-success + 1-reject discriminator) · D-TLSHS-REGSCOPE (the two-listener single-scrape discriminator + the zero-visibility confound) · D-TLSHS-VERIFYIFGIVEN (3 arms) · D-TLSHS-ALERTMAP (12 arms) · D-TLSHS-QUIC (§3.4).

**EXECUTED Go-side at THIS session's toolchain** (`go1.26.5 linux/amd64`, GOROOT `/snap/go/11227`): the full classifier matrix at BOTH TLS 1.2 and 1.3, the forced-send control, the synchronicity proof by ORDERING, the `IsValidName`/`NewCounter`/`ExtractTags`/`WriteProm` checks on the real names.

**NOT EXECUTED, stated plainly — the IMPL inherits no false confidence:**
- **NO build of row 74 exists, in-tree or in scratch.** No probe has confirmed the row compiles in-tree, that the three counters register without colliding at boot, or that the classifier fires from `serveConnection`. **That is the PLAN's and the IMPL's job.** (Phase 73's SPEC carried a scratch build; this one does not.)
- The `0111` `StatsAsserter` was **not written and not run**. Its shape is SPECIFIED (§8), not demonstrated.
- Session resumption (`ssl.session_reused`) — never driven on either side.
- `fail_verify_san` / `fail_verify_cert_hash` / `was_key_usage_invalid` / the four `ocsp_staple_*` — present on the reference, never DRIVEN; reading 0 is consistent with "unconfigured", NOT evidence about triggers.
- Whether `ssl.connection_error` absorbs non-cert failures beyond the two driven (§3.5).
- Whether the reference's `ssl.*` block keys off the LISTENER or the individual FILTER CHAIN — every probe arm used one chain per listener. **MOOT for envoy-go** (§3.3) but unresolved reference-side.
- Multi-`tls_certificate` listeners: both observed cert-expiry gauges already share the `e3b0c442` suffix, so a NAME COLLISION is a live risk, unprobed.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

- **`ssl.ciphers.*` / `ssl.versions.*` / `ssl.curves.*` / `ssl.sigalgs.*`** — the split half (§3.6). ⚠️ It is a **FOUR**-family split, not two: `curves` and `sigalgs` were never named in any prior document.
- **The other TWELVE fixed reference `ssl.*` names** — `no_certificate`, `session_reused`, `connection_error`, `fail_verify_san`, `fail_verify_cert_hash`, `was_key_usage_invalid`, four `ocsp_staple_*`, two `certificate.*` expiry gauges (§3.2, §13).
- The `upstream_cluster` span tag (the RUNNER-UP) · pick-time `picked`-propagation · `access_log[].filter` · `hcm.access_log_options` · `stdout`/`stderr` loggers · `stats_flush_on_admin` · `hcm.merge_slashes` · all tracing remainders · all xDS · all HTTP/3 · the never-opened families. §13.

---

## 3. The change — the D-TLSHS-* docket disposed one-for-one

### 3.1 D-TLSHS-SCOPE **[RESOLVED BY EXECUTION]** — spelling CONFIRMED, the doc roster REFUTED on one name

The reference's form is `listener.<scope>.ssl.<name>`. EXECUTED facts:

- **IPv4:** `listener.0.0.0.0_10000.ssl.handshake` — `:` → `_`, **dots PRESERVED**. The phase-65 doc-sourced spelling is correct for the REFERENCE.
- **The listener's `name:` field does NOT drive the scope** (config `name: listener_tls`, scope still `0.0.0.0_10000`).
- ⚠️ **`Listener.stat_prefix` DOES override it** — with `stat_prefix: MYSTATPREFIX` the scope became `listener.MYSTATPREFIX.ssl.handshake`, **no address at all**. **envoy-go has ZERO `GetStatPrefix()` consumers** in `internal/listener/` or `internal/bootstrap/` ⇒ accepted-and-SILENTLY-IGNORED. NEWLY SURFACED, not chartered (§13). **The `0111` fixture must NOT set `stat_prefix`.**
- ⚠️ **IPv6 brackets:** the reference RETAINS them — verbatim `listener.[__]_10002.ssl.handshake: 1`; envoy-go STRIPS them (`[::]:45259` → `___45259`).

⚠️ **REFUTED — `ssl.certificate.validation_ca.expiration_unix_time_seconds` DOES NOT EXIST at this pin.** No arm produced it. The reference emits **TWO** gauges with a different shape: `ssl.certificate.unnamed_ca_cert_e3b0c442.expiration_unix_time_seconds` and `ssl.certificate.unnamed_cert_e3b0c442.expiration_unix_time_seconds` (CA *and* leaf). The `e3b0c442` suffix is IDENTICAL across two DIFFERENT certs ⇒ **not content-derived** (executed); the "SHA-256 of the empty string / unset SDS name" reading is INFERENCE. **The landed `phases/65-…/SPEC.md:282` roster is wrong on this name and omits 12 more.** The other five doc names are spelled exactly right.

⇒ **The three names phase 74 lands are CONFIRMED live at this pin, in this spelling.**

### 3.2 The reference's ACTUAL `ssl.*` surface — FIFTEEN eager names + FOUR dynamic families

Created eagerly at zero, before any TLS connection: `handshake` · `no_certificate` · `session_reused` · `connection_error` · `fail_verify_error` · `fail_verify_no_cert` · `fail_verify_san` · `fail_verify_cert_hash` · `was_key_usage_invalid` · `ocsp_staple_{failed,omitted,requests,responses}` · the two `certificate.*` expiry gauges. A TLS listener also gets a sibling `server_ssl_socket_factory.*` scope.

Minted ONLY on a SUCCESSFUL handshake: `ssl.ciphers.<n>` · `ssl.versions.<n>` · `ssl.curves.<n>` · `ssl.sigalgs.<n>` (sigalgs tracks the CLIENT cert's sigalg — absent when no client cert is verified). Rejected arms minted NONE of the four.

⚠️ **`ssl.no_certificate` IS A SUCCESS-PATH ANNOTATION, NOT A FAILURE COUNTER — and this is a naming trap.** Established independently by THREE probe fleets and corroborated by the LANDED record at `BEHAVIOR_CONTRACT.md:962` (D4). At `require_client_certificate: false` a no-cert client yields `handshake: 1` **AND** `no_certificate: 1`, with all four `fail_verify_*` at 0. **Treating `no_certificate` and `fail_verify_no_cert` as synonyms is wrong in BOTH directions.** Phase 74 lands NEITHER a `no_certificate` counter nor any `require=false` behavior; the name is recorded here so a future row does not conflate them.

### 3.3 D-TLSHS-REGSCOPE **[RESOLVED BY EXECUTION — THE BRAINSTORM'S ANTICIPATION IS REFUTED]**

**Anticipated: unconditional ⇒ a flat +3. The reference does the OPPOSITE.**

The zero-visibility confound was settled FIRST, because an absent name is only evidence of non-registration if zeros are known to render: plain `/stats` returned **429 lines** vs `?usedonly` **85**, and the plaintext-only container carried **92 zero-valued `listener.*` counters** (`no_filter_chain_match: 0`, `downstream_cx_overflow: 0`, …). **Zeros DO render.**

The clean discriminator was ONE container, TWO listeners, ONE scrape, both driven with green positive controls:

| listener | `ssl.*` names |
|---|---|
| `listener.0.0.0.0_10001.*` (TLS) | the FULL block **including its zero members** (`fail_verify_error: 0`, `fail_verify_san: 0`, `session_reused: 0`, …) plus `handshake: 1`, `no_certificate: 1` |
| `listener.0.0.0.0_10000.*` (plaintext) | **ZERO `ssl.*` names**, while carrying 15+ other zero-valued names in the same scope |

⇒ **Registration is TLS-CHAINS-ONLY. DECIDED: envoy-go MATCHES it.**

**And the gate ALREADY EXISTS as a field, which makes the refuted anticipation the CHEAPER outcome, not the dearer one.** `listenerRuntime.tlsMode bool` is declared at `manager.go:145` and set at `:639` from `anyTLS`. Its only current reader is `manager_test.go:981` — phase 74 gives it its **first production consumer**. It is UNAMBIGUOUS because envoy-go **REJECTS mixed TLS+plaintext chains on one listener** (`manager.go:516-525`, ADR-0033 clause 5 / ADR-0078 clause 5): a listener is all-TLS or all-plaintext, never both. **⇒ the reference-side "listener scope vs filter-chain scope" question (§1.2) is MOOT for envoy-go — the two coincide by construction.**

⚠️ **CONSEQUENCE FOR THE STAT SURFACE, stated plainly: the delta is CONFIG-DEPENDENT, not flat.** A plaintext-only deployment gains ZERO names. The doc figure moves 1201 → **1204** because the BEHAVIOR_CONTRACT ledger counts the NAME SURFACE (what a TLS-configured proxy can emit), not a per-config instantiation. §7 pins this.

### 3.4 D-TLSHS-QUIC **[RESOLVED BY EXECUTION — AND IT REVERSED THIS SPEC'S OWN FIRST DECISION]**

**Registration gate: `rt.tlsMode` ALONE. NO kind check. QUIC listeners DO get the three names.**

⚠️ **This SPEC first decided the opposite** — gate on `rt.tlsMode && rt.kind != kindQUIC` — reasoning that *"a counter reading 0 while the underlying event demonstrably occurs is a false measurement, and an absent counter is an honest coverage gap."* **A probe run specifically to settle it REFUTED that reasoning on the facts.** The argument was sound in the abstract and wrong about the reference. Recorded rather than quietly replaced (§16).

**EXECUTED — verdict (b): the reference registers the family on QUIC and NEVER moves it.** One container, a TCP-TLS listener (19410) and a QUIC listener (19411) in the SAME process and the SAME scrape, so there is no cross-container confound; positive controls green on both (`TCPTLS-OK` over TLS, `QUIC-OK` over `HTTP/3.0`; client was a scratch Go `http3.Transport` against the cached `quic-go v0.54.1`, since local curl has no HTTP/3):

- **At boot** the QUIC listener already carries **14** `ssl.*` names — a byte-identical family to the TCP listener's. ⇒ eager registration is **NOT** TCP-gated; hypothesis (c) REFUTED.
- **After 5 successful, independent H3 connections** (`downstream_cx_total: 5`, `http.quic_hcm.downstream_rq_2xx: 5`): `listener.0.0.0.0_19411.ssl.handshake: **0**` against `listener.0.0.0.0_19410.ssl.handshake: 1`. The QUIC block is **byte-for-byte identical to its boot block**, and the dynamic `ciphers`/`versions`/`curves`/`sigalgs` families are **entirely absent** despite QUIC mandating TLS 1.3.
- **Failure arm (fresh container), the cleanest discriminator** — the same failure class driven at both listeners (client rejects the self-signed cert): `…19410.ssl.connection_error: 1` vs `…19411.ssl.connection_error: **0**`, while `…19411.downstream_cx_total: 1` proves the connection WAS accounted. QUIC client error verbatim: `CRYPTO_ERROR 0x12a (local): tls: failed to verify certificate: x509: certificate signed by unknown authority`.

⇒ **The reference's own QUIC listener presents a permanently-zero `ssl.handshake` beside a live `downstream_cx_total`. envoy-go doing exactly the same is EXACT PARITY, not a false measurement — the reference makes precisely the same "claim" and never satisfies it. Kind-gating QUIC OUT would be the DEPARTURE.**

**The gate is therefore `rt.tlsMode` alone, and that is provably sufficient for QUIC:** `anyTLS` is set at `manager.go:478` from the same per-chain `ci.tlsCfg` that `quicTLSConfig()` reads (`quic.go:56-66`), and `startQUIC` **hard-errors** when that config is nil (`quic.go:33-36`, *"quic listener has no TLS config (mandatory TLS not built)"*). ⇒ **every QUIC listener that successfully boots necessarily has `tlsMode == true`.** No kind check, no special case, and `registerListenerMetrics` stays a single uniform function for both runtimes.
⚠️ **The corollary the probe agent flagged and this SPEC adopts:** the gate must NOT be written as "has a TCP-style TLS transport socket" in any form that excludes `QuicDownstreamTransport` — the reference plainly treats it as TLS-bearing for registration while never instrumenting it. `rt.tlsMode` satisfies this by construction; a chain-walking reimplementation might not.

**What remains a genuine, SHARED limitation** (record as such, NOT as an envoy-go departure): neither implementation books QUIC handshake outcomes under `listener.<addr>.ssl.*`. In envoy-go it is mechanically unavoidable — `serveConnection` has exactly ONE call site (`manager.go:1081`, inside `acceptLoop`), `acceptLoop` has exactly one (`:1009`) guarded at `:997-1001` by `if rt.kind == kindQUIC { continue }`, QUIC launches `quicAcceptLoop` at `quic.go:49`, and the QUIC handshake completes **inside quic-go before `ql.Accept(ctx)` returns** (the code's own comments, `quic.go:84-85`, `:109`), so a failed QUIC handshake never surfaces as a per-connection event at all.

⚠️ **NEWLY SURFACED, not chartered:** the reference DOES account QUIC connections as SSL **one scope up** — `http.<stat_prefix>.downstream_cx_ssl_total: 5` on the QUIC HCM — plus a large QUIC-specific surface (`http3.*`, `quic.connection.*`, `quic.dispatcher.*`, `udp.downstream_rx_datagram_dropped`, `quic_server_transport_socket_factory.*`). envoy-go emits none of it. §13.

`internal/listener/quic.go` stays **BYTE-UNTOUCHED**, and now for a stronger reason than before: there is nothing to gate.

**NOT established:** whether a QUIC **client-cert** verification failure (`require_client_certificate` over QUIC) reaches `ssl.fail_verify_*`. Only the client-rejects-server-cert direction was driven. Given `connection_error` stayed 0 there while the TCP comparator fired, an entirely-inert block is the strong reading — but it is a reading, not a result.

### 3.5 D-TLSHS-SEMANTICS + D-TLSHS-ALERTMAP + D-TLSHS-CLASSIFY **[ALL RESOLVED BY EXECUTION]**

**SEMANTICS — `ssl.handshake` counts SUCCESSES ONLY.** One fresh container, criterion stated first: after 1 success `cx_total=1, handshake=1`; after adding 1 rejection `cx_total=**2**, handshake=**1**, fail_verify_error=1`. A second success took it 1→2 ⇒ per-handshake, not a one-shot flag. **A failed handshake increments ONLY its specific failure counter.** The BRAINSTORM's doc-sourced INFERENCE is now an EXECUTED fact.

**ALERTMAP — the two taxonomies AGREE on EVERY cert-shaped input.** 12 arms:

| input | Envoy alert | Envoy counter | Go shape | verdict |
|---|---|---|---|---|
| valid CA-1 cert | none, 200 | `ssl.handshake` | nil | AGREE |
| no cert (TLS 1.3) | **116** `certificate_required` | `fail_verify_no_cert` | `*errors.errorString` | AGREE |
| untrusted CA-2 | **48** `unknown_ca` | `fail_verify_error` | `*tls.CertificateVerificationError` | AGREE |
| **EXPIRED, CA-1** | **45** `certificate_expired` | `fail_verify_error` | `*tls.CertificateVerificationError` | **AGREE** |
| **wrong EKU, CA-1** | **43** `unsupported_certificate` | `fail_verify_error` | `*tls.CertificateVerificationError` | **AGREE** |
| no cert, **TLS 1.2** | **40** `handshake_failure` | `fail_verify_no_cert` | `*errors.errorString` | AGREE |
| ALPN mismatch | none — handshake **SUCCEEDS** | `ssl.handshake` | nil | AGREE |

The expired and wrong-EKU arms are exactly the two the BRAINSTORM named as most likely to diverge. **They do not.** The wrong-EKU arm also settles an unasked sub-question: BoringSSL DOES enforce the clientAuth EKU purpose, so the sides agree on accept-vs-reject, not merely on the bucket. Every arm moved EXACTLY ONE counter.

⚠️ **THE IMPLEMENTATION-CRITICAL COROLLARY: THE ALERT CODE IS NOT THE BUCKET KEY.** FIVE distinct alerts (116, 48, 45, 43, 40) collapse onto TWO cert buckets, proven by construction — the SAME no-cert input forced to TLS 1.2 emits alert **40** instead of **116** yet still increments `fail_verify_no_cert`. **Any implementation deriving the counter from the alert would mis-bucket every TLS 1.2 connection.** envoy-go's shape-based `errors.As` classification is independently VALIDATED by this arm. Reinforcing it: **`tls.AlertError` is UNREACHABLE on a non-QUIC server conn** — the wrap at `crypto/tls/conn.go:1598` sits inside `if c.quic != nil`; a remote alert arrives as `*tls.permanentError` wrapping the *unexported* `tls.alert`, so `errors.As(&tls.AlertError)` matches **nothing**.

**CLASSIFY — the pinned table.** `classifyHandshakeErr(err error) handshakeOutcome`, package-private, in `internal/listener/manager.go`:

| condition | outcome | counter |
|---|---|---|
| `err == nil` | `ok` | `ssl.handshake` |
| `errors.As(err, &cve)`, `cve *tls.CertificateVerificationError` | `verifyError` | `ssl.fail_verify_error` |
| `err.Error() == "tls: client didn't provide a certificate"` | `noCert` | `ssl.fail_verify_no_cert` |
| everything else | `other` | **NOTHING** |

**CONFIRMED at `go1.26.5`:** `errors.As` is an EXACT, TOTAL discriminator between the two arms at BOTH TLS versions, and **NO non-cert failure shape produced a CVE** across 10 distinct shapes ⇒ `verifyError` **does not over-count**. Non-vacuity PROVEN, not asserted: the untrusted arm's text differs from the no-cert arm's AND `CVE.UnverifiedCertificates` has length **1**. The control arm (no forced-send) **degraded into the no-cert arm**, confirming `reference_go_client_cert_withholding` live.

**Synchronicity CONFIRMED by ORDERING, not assertion:** the client's second flight was stalled 300 ms inside `GetClientCertificate`; the server's `HandshakeContext` blocked **301 ms (1.2) / 302 ms (1.3)** and returned only after the flight. Every failing arm's error came from `HandshakeContext` itself. **⇒ no half-RTT server-side deferral; the row does NOT inherit the `0108`/`0111` client-side workaround.**

**THE NAMED FRAGILITY (ADR-0296 must record it, not bury it).** The no-cert arm is a bare `errors.New("tls: client didn't provide a certificate")` at `$(go env GOROOT)/src/crypto/tls/handshake_server.go:964` — no wrapping, no alert on the returned value. **`crypto/tls` exports exactly four error-ish symbols and ZERO error VALUES** (`AlertError`, `CertificateVerificationError`, `ECHRejectionError`, `RecordHeaderError`), so **there is no sentinel and a string comparison is FORCED**. The text is version-invariant and that is **structurally guaranteed**: single producer `processCertsFromClient` (`handshake_server.go:940`), called from `:703` (≤1.2) and `handshake_server_tls13.go:1056` (1.3). **⇒ the no-cert test case MUST construct its error by running a LIVE in-process handshake, never by hand-writing the string** — a hand-written string tests nothing and would sail through a toolchain that reworded the message. Written correctly, **the test IS the tripwire.**

⚠️ **THREE KNOWN MIS-CLASSIFICATIONS, ALL UNDER-COUNT — recorded, not hidden:**

1. **cert/private-key mismatch → `other`.** `tls: invalid signature by the client certificate: ECDSA verification failure` — a bare `errors.New` (`handshake_server.go:794`/`:805`; `_tls13.go:1101`).
2. **malformed cert DER → `other`.** `tls: failed to parse client certificate: x509: malformed certificate` (`handshake_server.go:947`).
   ⇒ **`verifyError` counts ONLY failures that reached `certs[0].Verify()`.** **WORDING IS LOAD-BEARING:** documenting it as *"certificate chain verification failed"* is EXACT; *"client cert rejected"* would UNDER-REPORT. §9 uses the exact wording.
3. **The ctx override can mask ANY bucket — DEMONSTRATED.** `crypto/tls/conn.go:1539-1547` installs `context.AfterFunc` + `defer func(){ if !stop() { ret = ctx.Err() } }()`, so a firing ctx REPLACES the real outcome. Executed: an untrusted cert + a 150 ms server deadline returned `context deadline exceeded` ⇒ classified `other`.
   **CONTROLLER-BOUNDED, and this is what makes it shippable:** `internal/listener/` has **ZERO `context.WithTimeout`/`WithDeadline` in production code** (all such hits are `_test.go`), and the ctx originates at `cmd/envoy-go/main.go:339` — `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`, **cancel-only, NO deadline** — flowing `Start(ctx)` `:342` → `acceptLoop` → `serveConnection` `:1081` → `HandshakeContext` `:1178`. ⇒ **the override can fire ONLY on SIGINT/SIGTERM shutdown**, where misclassifying an in-flight handshake is immaterial. **But the accuracy is CONDITIONAL, not unconditional: any future row adding a handshake deadline re-opens it.** ADR-0296 names it.

⚠️ **`other` INCREMENTS NOTHING, and that is a DEPARTURE, not a neutral omission.** The reference's `other` bucket **exists**: `ssl.connection_error`. EXECUTED — a client offering only TLS 1.0 (alert 70) and a plaintext HTTP request to the TLS port BOTH landed there, with `fail_verify_error`/`fail_verify_no_cert`/`handshake` all staying 0; conversely the cert arms left `connection_error` at 0. **The cert path and the non-cert path are DISJOINT** — a genuine third bucket, not a double-counting catch-all.
**DECIDED: the row stays at THREE names and NAMES the departure.** Grounds: only TWO of N non-cert failure modes were driven, and whether `connection_error` absorbs the others is **UNTESTED** — a counter whose membership cannot be enumerated cannot be cross-side asserted honestly. `ssl.connection_error` is recorded as the **cheapest identified follow-on** (§13). ⚠️ **A break that collapses `other` into `fail_verify_error` MUST fire** (§10).

### 3.6 D-TLSHS-SPLIT — the dynamic half stays DEFERRED on two named blockers

`ssl.ciphers.*` / `ssl.versions.*` / **`ssl.curves.*`** / **`ssl.sigalgs.*`** (a FOUR-family split):

1. **Charset.** `NamePattern` = `` `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` `` (`registry.go:48`) — **no hyphen**. The Envoy/OpenSSL TLS1.2 cipher form `ECDHE-RSA-AES128-GCM-SHA256` fails `IsValidName` ⇒ `checkName` **PANICS** (`registry.go:107`). This is the landed `reference_dynamic_stat_name_charset_guard` hazard.
2. **Name mismatch.** Go's `tls.CipherSuiteName` yields IANA forms; Envoy yields OpenSSL forms. A cross-side EXACT assertion is INFEASIBLE without a hand-maintained table whose mapped names then trip blocker 1.

**Lifecycle is explicitly NOT a blocker** — `NewCounterIfAbsent` (`:161`) / `NewGaugeIfAbsent` (`:208`) route through `getOrRegister` (`:177-188`), whose comment at `:175-176` states *"PERMITTED post-Freeze by design (ADR-0117)"*. **The blocker is NAMING.**

⇒ **ADR-0286 C3's boundary CONFLATES a cheap half with a blocked half. Splitting it is part of this row's contribution**, and the surviving boundary is **RESTATED** at `BEHAVIOR_CONTRACT.md:928`, not deleted.

### 3.7 D-TLSHS-VERIFYIFGIVEN **[RESOLVED — the anticipation was wrong, and it is OUT OF SCOPE]**

Anticipated: at `require_client_certificate: false` a no-cert connection counts `ssl.handshake` and nothing else. **EXECUTED: it is `ssl.handshake: 1` PLUS `ssl.no_certificate: 1`.** An UNTRUSTED presented cert at `require=false` is **still fully rejected** (`fail_verify_error: 1`, `handshake: 0`) — so `require=false` means *"a cert is optional, but if given it is FULLY verified"*; it does not relax verification. The reference sends a `CertificateRequest` even at `require=false`.

The Go-side trap was checked and the landed memory **CONFIRMED at go1.26.5, DISCRIMINATED rather than inferred**: with `VerifyClientCertIfGiven` + a **nil** `ClientCAs`, even the CORRECTLY-signed cert fails, so such an arm cannot discriminate anything. Mechanism proven directly — `x509.SystemCertPool()` has 122 subjects, and `Verify(Roots: nil) ⇒ chains=1, err=<nil>` vs `Verify(Roots: EMPTY pool) ⇒ chains=0, unknown authority`. `Roots: nil` genuinely falls back to the system store (`x509/verify.go:577-580`).

**Phase 74 implements NO `require=false` behavior and lands NO `no_certificate` counter.** `0111` is `require_client_certificate: true`. Recorded so a future row inherits the finding rather than re-probing it.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules, **0 new imports**

No new interface, no new package-level type, **no new exported symbol anywhere** (`classifyHandshakeErr` and `handshakeOutcome` are unexported). Three `*stats.Counter` fields, three gated registrations, one helper, two call points.

`errors` and `stdtls "crypto/tls"` are ALREADY imported (`manager.go:6`/`:5`) ⇒ **+0 imports** (R2). `internal/listener` already imports `internal/stats` ⇒ **no new layering edge**; re-run `go list -deps` (no `...`) at the IMPL as DISCIPLINE, not as a cycle check. `go mod tidy -diff` anticipated EMPTY; `reference_new_subpackage_pulls_transitive_module` does not bite (no new sub-package).

---

## 5. Identifier hygiene *(collision checks — RE-DERIVED repo-wide at tip, `reference_spec_drafted_identifier_collision_check`)*

`classifyHandshakeErr` **0** · `handshakeOutcome` **0** · `verifyError` **0** · `noCert` **0** · `sslHandshake` / `sslFailVerifyError` / `sslFailVerifyNoCert` **0**. **All clear.**

---

## 6. Reject roster — **UNCHANGED, and that is the point**

Phase 74 consumes **no new config field at all**. There is no parse arm, no reject to lift, no `DiscardUnknown` interaction, no fuzz-seed classification to flip. It adds observability for behavior already fully implemented and already config-driven (`require_client_certificate` + a trust anchor, static or SDS-delivered). ⇒ **+0 rejects, +0 fuzzers** (§7).

---

## 7. Stat surface **+3** (1201 → 1204) · Fuzz **+0**

**The first non-zero stat delta in this lineage, and breaking the streak is CORRECT.** That streak was never a goal — it was a property of the rows that happened to be cheapest. ADR-0286 C3 cited *"would blow the +0-stat envelope"* as a reason to defer; **an envelope is not an argument, and the row is worth three names.** ADR-0296 says so rather than apologising for it.

**Charset — EXECUTED, not "by inspection":** in a scratch module with a `replace` onto this worktree (nothing written inside the repo), `IsValidName` returned **true** for all three FULL names in BOTH spellings *and* for the IPv6 form `listener.___45259.ssl.handshake`; `NewCounter` succeeded for all three. **SN3 flattening is also clean** — these are the first three-dot listener names, and `ExtractTags` yields residual `listener.ssl.handshake` + label `{envoy_listener_address 0_0_0_0_10000}`, `WriteProm` yields `envoy_listener_ssl_handshake{envoy_listener_address="0_0_0_0_10000"}`. **The all-underscore address form is what saves it** (a dotted address would truncate at the first octet).

⚠️ **`internal/stats/name.go:452-464` `helpText` has no entry for the three names**, so `/stats/prometheus` degrades to `# HELP envoy_listener_ssl_handshake envoy_listener_ssl_handshake`. **⇒ `internal/stats` is NOT byte-untouched** (contra BRAINSTORM §10): a 3-entry `helpText` addition is OWED. Cosmetic but a real admin-surface delta.

⚠️ **The +3 is a NAME-SURFACE delta, not a per-deployment one** (§3.3): a **plaintext-only** deployment gains ZERO names. A **QUIC** listener DOES gain all three (`tlsMode` is necessarily true, §3.4) and they stay permanently zero — which is parity with the reference, not an accounting anomaly.

**Fuzzers +0** — `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` ⇒ **55**, unchanged. No config surface ⇒ no parse arm.

---

## 8. Differential fixture — **`0111` EXTENDED, +0 fixtures (119)**

`test/fixtures/0111-tls-cvc-empty-dynamic-fallback/` already drives all three arms against ONE `require_client_certificate: true` listener and **already passes**. Phase 74 adds a `fixture.StatsAsserter` (`test/differential/fixture/fixture.go:75`). **NO new YAML, NO new directory, NO new BackendKind, no new port** (`0111`'s reference port is **10447**, not the `101xx` series; `10118` stays free but is not needed).

### 8.1 The asserter's shape — **DECIDED: `/stats/prometheus`, Shape A (no re-drive), cross-side**

⚠️ **A flat `/stats` cross-side comparison is IMPOSSIBLE by NAME** (R3): reference `listener.0.0.0.0_10447.ssl.handshake` vs subject `listener.0_0_0_0_<runner-allocated-port>.ssl.handshake` — different normalization *and* a per-run port the driver currently **discards** (`SubjectConfig`'s first param is `_`, `driver.go:317`, nothing stashed).

**Both sides hoist the address into a LABEL and leave the metric NAME address-free** (envoy-go rule SN3, `internal/stats/name.go:37`, impl `:85-93`, `flattenToProm` `:370-376`). So both flatten to **`envoy_listener_ssl_handshake{envoy_listener_address=…}`** — **name-identical, differing only in the label value**. ⇒ **scrape `/stats/prometheus`, key on the metric NAME, IGNORE `envoy_listener_address`.** Precedent: `0005-prometheus-stats/driver/driver.go:537-541` does exactly this for `envoy_listener_downstream_cx_total`. This avoids BOTH the divergence and the port-plumbing.

**Shape A (scrape once, assert absolute counts) is CHOSEN over Shape B (re-drive + delta).** Justification, because `reference_panic_counter_differential_delta_assertion` says counter assertions should be deltas: **that rule exists for counters a PRE-WORKLOAD phase can move, and nothing pre-moves `l_edf`'s `ssl.*` here.** Traced exhaustively: reference readiness polls admin `9901` via `wait.ForHTTP("/ready").WithPort("9901/tcp")` (`harness.go:133`), **not** the TLS port; subject readiness parses a stdout sentinel (`harness.go:264-274`), no dial; `waitTCPDial`'s ~12 call sites all target BACKEND ports; `ProbeAdmin` hits admin only; `startSubjectWithRetry` retries with a **fresh process** ⇒ stats reset, no residue; no health checks, no warmup; one fresh TCP conn per arm. The proxies are freshly booted per run and the three arms are the **only** connections to `l_edf` ⇒ deterministic **3 accepts / 1 success / 2 rejections per side**.
⚠️ Shape B would additionally require RELOCATING `closeServers()`, which fires inside `DriveSubject` (`driver.go:340`) **before** `AssertStats` and closes BOTH sides' SDS receivers. Shape A is unaffected by that (it only scrapes admin) — **but the assertion MUST stay confined to `listener.<addr>.ssl.*` and MUST NOT touch SDS or `sds_cluster` scopes**, which are reconnecting against a closed port during step 10.

**Expected per side:** `ssl.handshake` **1** · `ssl.fail_verify_error` **1** (arm 2, the forced-send untrusted cert) · `ssl.fail_verify_no_cert` **1** (arm 3). Cross-side EQUAL on all three.

### 8.2 Mandatory guards

- ⚠️ **`var _ fixture.StatsAsserter = (*edfDriver)(nil)` IS REQUIRED.** Dispatch is a **silent** type assertion — `runner_test.go:1342-1349`, the ONLY dispatch in the package. A signature typo (`*testing.T` instead of `fixture.TB`, a returned `error`, wrong order, misspelled name) makes `ok == false` and the branch never fires, and **the compiler, `go vet` AND `golangci-lint` are ALL silent**. `0111` today has only `var _ fixture.Driver = (*edfDriver)(nil)` (`:615-616`).
- ⚠️ **The ABSENT check MUST be separate from the value check** (`0055/driver.go:655-669` precedent). A counter that fails to REGISTER reads as `0 == 0` and passes **VACUOUSLY**. **For a row whose entire purpose is ADDING counters, this is the single most important guard in the fixture.**
- **`t.Errorf` per property; `t.Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`). Add a decode-ran guard in the `0097/driver.go:687-690` style.
- **`fixture.TB` has exactly THREE methods** — `Errorf`/`Fatalf`/`Helper` (`fixture.go:62-68`). **No `Logf`, no `Cleanup`.** Record diagnostics with `log.Printf` (`reference_fixture_tb_has_no_logf`); any conn opened in `AssertStats` needs a `defer` guard, not `t.Cleanup` (`0055/driver.go:613-625`).
- **There is NO shared scraper** — none in `test/helpers/`. Every fixture copies its own; the canonical flat body is `0055/driver.go:865-900` and the Prometheus precedent is `0005`. The harness DOES already read the REFERENCE's admin `/stats` (`0055:627`, `0097:651`, `0043:331`) ⇒ **no new capability needed**.

### 8.3 ⚠️ The RD3 disclaimer INVERTS — the row's best fixture finding

`driver.go:73-81` currently says, verbatim, *"at require=true the forced-send is NOT the observable's discriminator … **Do NOT claim forced-send flips the observable here.**"* That is TRUE at the byte observable — both negative arms read `rejected`. **But at the `ssl.*` COUNTER layer arms 2 and 3 hit DIFFERENT counters** (`fail_verify_error` vs `fail_verify_no_cert`), and the Go-side control arm proved that without forced-send arm 2 **degrades into arm 3**. ⇒ **phase 74 UPGRADES the forced-send from "retained for symmetry" to LOAD-BEARING**, and the disclaimer must be revised, not merely left standing.

### 8.4 The self-confessed boundary notes — **FOUR sites, not two**

⚠️ Both BRAINSTORM cites DRIFTED and both under-count:

| site | tip lines | note |
|---|---|---|
| `README.md` | **`:162-165`** (cited `:161-162`; `:161` is the alert-text bullet) | the `ssl.*` bullet |
| `expectations.yaml` | **`:177-184`** (cited `:178`, mid-sentence) | the `ssl.*` block |
| `expectations.yaml` | **`:189`** | *"The `sds.<secret>.*` stat counters (no StatsAsserter)"* — a bare fact that becomes false |
| **`envoy.yaml`** | **`:24-25`** | *"NOT a stat (envoy-go emits no `ssl.*` family; see README)"* — in the REFERENCE bootstrap's own header |

⚠️ **EACH note BUNDLES a retirable half with a STILL-LIVE guard.** `README.md:164-165` and `expectations.yaml:181-184` carry *"Never assert `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as listener liveness"* — which stays TRUE. **A blanket delete would silently drop a live guard.** Also `expectations.yaml:133-153` ("## Asserted") enumerates exactly (a) per-side structural + (b) cross-side CompareBytes; a **third leg (c)** must be ADDED.

Three further prose sites need revision for §8.3: `driver.go:73-81`, `README.md:100-110`, `expectations.yaml:109-119`.

### 8.5 The `-run` selector, pinned

```bash
go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback' -v
```
The subtest name is the DIRECTORY name verbatim (`discoverFixtures` → `t.Run(fx, …)`, `runner_test.go:191`, `:1460-1495`). A bare `-run '0111'` matches **ZERO** subtests and reports a vacuous green (`reference_differential_run_selector`); `-count=1` is separately mandatory for break protocol (`reference_differential_break_protocol_count1`).

---

## 9. Behavior-contract edit map — pinned

- **B1 — `BEHAVIOR_CONTRACT.md:928`** (the C3 coverage boundary). **RETIRE for the three fixed names; RESTATE for the dynamic half** with its two blockers (§3.6). ⚠️ Its current text says `listener.<name>.ssl.*` — **the scope segment is the normalized bind ADDRESS, not the listener name**; fix that in the same edit. Structure for a precise rewrite: `:926` rejects para · `:927` blank · **`:928` the boundary** · `:929` blank · `:930` `**Differential coverage.**`.
- **B2 — `:916`** narrates the reference's three arms as a phase-65 probe and closes *"envoy-go's accept/reject decisions match"*. ADD an "and envoy-go now emits these three counters" clause.
- **B3 — `:918`** (phase-67/ADR-0289 init-hold) says *"no TLS alert; no `ssl.*` movement"*. Its implicit "envoy-go has none anyway" framing needs a re-read once envoy-go emits `ssl.*`. §6 named neither `:918` nor `:962` (D4).
- **B4 — the QUIC coverage boundary (§3.4), recorded as SHARED PARITY, not a departure.** A QUIC listener registers all three counters and they stay permanently ZERO — **and the reference does exactly the same** (EXECUTED: `ssl.handshake: 0` after 5 successful H3 connections, `connection_error: 0` on a failure arm where the TCP comparator fired). State the mechanism (quic-go's `Accept` returns post-handshake; a failed QUIC handshake never surfaces), state that it is PARITY, and name `0104-http3-downstream-get` as the site for any future closure. ⚠️ Do NOT write this up as an envoy-go departure — that was this SPEC's own first, refuted reading.
- **B5 — the `other`-bucket departure (§3.5)** — the reference books non-cert handshake failures under `ssl.connection_error`; envoy-go's `other` increments NOTHING. Record with the two EXECUTED arms and the explicit note that `connection_error`'s full membership is unenumerated.
- **B6 — the classifier's exact semantics.** `ssl.fail_verify_error` means *"certificate chain verification failed"*, NOT *"client cert rejected"* — cert/private-key mismatch and malformed DER land in `other` (§3.5). **The wording is load-bearing.** Also record the ctx-override conditionality.
- **B7 — ⚠️ THE STAT-SURFACE LEDGER (D5).** `1201` lives at exactly TWO lines, **`:831`** and **`:847`** → **1204**. **AND** the IMPL must decide about the missing `1200 → 1201` ledger step and the **three stale `1200`s at `:1429`, `:1463`, `:1495``. This is a bigger edit map than "change 1201 to 1204" and the PLAN must task it explicitly.
- **B8 — the three cross-side scope divergences** (dots, IPv6 brackets, `Listener.stat_prefix`), all PRE-EXISTING and all affecting the landed `downstream_cx_total` too. Record once, here, so a future asserter author does not rediscover them.

**Code-comment edits owed** (`reference_code_comment_not_evidence`): `manager.go:172-174` and `:345` (D6), and `manager_test.go:1911-1913` + the test NAME at `:1928` (D7 — it will NOT go red, so nothing else will catch it).

---

## 10. Test plan + task surface *(D-TLSHS-SPLIT — a SINGLE FLAT ROW; ADR-0045 valve armable-but-unconsumed)*

**~9 tasks anticipated.** The production diff is confined to ONE file; the count is dominated by the discriminating increment tests, the guard, and the fixture extension. The one thing that could have forced a split — a registration posture requiring the QUIC runtime — **does not**: §3.4 gates QUIC out inside `registerListenerMetrics` and leaves `quic.go` byte-untouched.

Anticipated spine (the PLAN decomposes, red-first → green → break-verified per task):

1. `handshakeOutcome` + `classifyHandshakeErr`, table-driven, with the **live-handshake-constructed** no-cert case.
2. The three counter fields + the **TLS-and-TCP-gated** registration (`rt.tlsMode && rt.kind != kindQUIC`).
3. Success / verify-error / no-cert increment tests through `serveConnection`.
4. The plaintext-listener test: registers NOTHING and Incs NOTHING.
5. The QUIC-listener test: registers all THREE names (§3.4 parity), leaves them at ZERO across a completed H3 handshake, still Incs both cx metrics, and does not panic.
6. The INVERTED registration guard (§10.1).
7. `internal/stats/name.go` `helpText` ×3.
8. The `0111` `StatsAsserter` + the FOUR boundary-note retirements + the RD3 revision (§8.3/§8.4).
9. Docs: BEHAVIOR_CONTRACT B1-B8, ADR-0296 completion, row-74 `done`, the ROADMAP narrow, STATE/next-prompt roll.

**Breaks that MUST fire** (`reference_deliberate_break_wrong_assertion` — confirm WHICH assertion fired; run with `-count=1` AFTER committing, `reference_break_protocol_commit_first`):

- **A.** Collapse `other` into `verifyError` ⇒ must fire on the non-cert arms and NOT on the two cert arms.
- **B.** Swap the `verifyError` and `noCert` arms ⇒ must fire in **exactly two of the three** increment tests (the cross-product is what discriminates, per `reference_probe_must_discriminate`).
- **C.** Move the success `Inc` outside the `if selected.tlsCfg != nil` block ⇒ the plaintext test must fire.
- **D.** ADD a `rt.kind != kindQUIC` gate (i.e. implement this SPEC's refuted first reading) ⇒ the QUIC registration test must fire. **This is the row's distinguishing break** — it is the only one that discriminates the §3.4 decision, and it is deliberately framed as *adding* the wrong gate rather than removing the right one, because the correct implementation has no kind check to delete.
- **E.** Drop the `rt.tlsMode` gate ⇒ the plaintext-registration test must fire. ⚠️ It must NOT also fire the QUIC test — QUIC has `tlsMode == true`, so a break here discriminates plaintext specifically. If both fire, the tests are entangled and the gate is being tested at the wrong layer.
- **F.** Hand-write the no-cert string in the test instead of deriving it from a live handshake ⇒ must be shown to leave the suite GREEN, proving the live-handshake construction is load-bearing rather than decorative.
- **G.** Inside the `0111` `AssertStats`, break one asserted counter ⇒ confirm it fires AND that it is the stats assertion firing, not a `CompareBytes` mismatch (F7's five silent-skip causes make this non-optional).

### 10.1 The registration guard — INVERTED, and it must assert NAMES

Precedent `internal/statssink/registration_test.go:26` `TestNoNewStat_RegistrationGuard` (one of **five** such guards, all in that one file: `:26`/`:53`/`:81`/`:109`/`:137`). ⚠️ **It asserts a CARDINALITY DELTA via `reg.Walk` counting, and NEVER inspects `m.Name()`.**

⇒ **A count-only inversion is INSUFFICIENT: it would PASS if the three names were misspelled.** Phase 74's guard must assert the exact **NAME SET** added at listener scope — EXACTLY THREE, spelled `…ssl.handshake` / `…ssl.fail_verify_error` / `…ssl.fail_verify_no_cert` — and must separately pin that a plaintext listener and a QUIC listener add **ZERO**.

---

## 11. Edit-site roster — RE-DERIVED at tip

| file | sites |
|---|---|
| `internal/listener/manager.go` | `:172-174` comment (D6) · `:175-176` +3 fields · `:345` comment (D6) · `:351-355` gated registration · `:1178` classify+Inc (inside the `if err` block — `err` is scoped to the `if` init) · `:1183` success Inc · the new `classifyHandshakeErr` + `handshakeOutcome` |
| `internal/listener/manager_test.go` | the classifier table · 3 increment tests · plaintext + QUIC tests · the name-set guard · `:1911-1913`/`:1928` rename (D7) |
| `internal/stats/name.go` | `:452-464` `helpText` ×3 |
| `test/fixtures/0111-…/driver/driver.go` | `AssertStats` + the compile-time assertion · `:73-81` RD3 revision |
| `test/fixtures/0111-…/README.md` | `:100-110` · `:162-165` |
| `test/fixtures/0111-…/expectations.yaml` | `:109-119` · `:133-153` · `:177-184` · `:189` |
| `test/fixtures/0111-…/envoy.yaml` | `:24-25` |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | B1-B8 (§9) |
| `docs/envoy-go/DECISIONS.md` | ADR-0296 · the ADR-0286 C3 blockquote correction (§14) |
| `docs/envoy-go/ROADMAP.md` | row 74 → `done` + the narrow — **AT THE IMPL ONLY** (§12) |

**BYTE-UNTOUCHED, verify by sha256 not by reading** (the phase-72/73 precedent): `internal/listener/quic.go` · `internal/tls` · `internal/xds` · `internal/boot` · `internal/bootstrap` · `internal/tracing` · `internal/cluster` · `internal/filter/**` · `validate/` · `cmd/` · `go.mod` · `go.sum` · `test/fixtures/0111-…/envoy-go.yaml`.

---

## 12. Sentinel maintenance — a SENTENCE-NARROWING row, narrowed AT THE IMPL

**NO deferred sentence is narrowed at ANY pre-IMPL stage** (the phase-57 precedent) — narrowing early breaks check (2)'s meaning. **ROADMAP is UNTOUCHED at this SPEC and row 74 STAYS `in-progress`.**

**The narrow was VERIFIED MECHANICALLY, not predicted** — applied in a scratch copy and both checks re-run on it:

- BEFORE: `(handshake/fail_verify_error/fail_verify_no_cert/ciphers/versions — a NAMED COVERAGE BOUNDARY …)`
- AFTER: `(ciphers/versions — a NAMED COVERAGE BOUNDARY …)`
- **`grep -c` on the scratch copy ⇒ `3`. Check (2) STAYS 3. CONFIRMED.** The `candidates:` marker survives and the sentence still terminates at `force-trace.`, so the `[^.]*\.` regex still binds. Check (1) unchanged.

⚠️ **BUT THE MECHANICAL CHECK IS NOT THE WHOLE EDIT.** After removing the three names the clause still reads *"a NAMED COVERAGE BOUNDARY recorded at phase 65 per ADR-0286 §Consequences C3: **envoy-go emits ZERO such stats** while the reference emits the family live, so it is a **framework-surgery row, NOT an inline add**"* — the first claim becomes FALSE and the second is precisely the adjective this row refutes. **The IMPL must replace the PROSE, not just delete three names.** The PLAN must task this explicitly.

Live sentences are at `ROADMAP.md:184` (HTTP/3), `:194` (xDS), `:204` (Observability). ⚠️ Use the canonical `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` — a naive `grep -c 'candidates:'` returns **11**, and one `candidates were:` recap at `:204` is HISTORICAL, not live (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 13. Deferred items

**Newly surfaced by THIS SPEC, none chartered:**

- **`ssl.connection_error`** — the cheapest identified follow-on; envoy-go's `other` bucket already computes the classification (§3.5). Blocked only on enumerating the bucket's membership.
- **The other ELEVEN fixed reference `ssl.*` names** (§3.2), incl. `ssl.no_certificate`, which the LANDED record already names at `BEHAVIOR_CONTRACT.md:962` (D4).
- **`Listener.stat_prefix`** — accepted-and-silently-ignored; the reference honours it and drops the address from the scope entirely (§3.1).
- **The IPv6-bracket and dot-normalization scope divergences** — pre-existing, affecting `downstream_cx_total` today (§3.1, B8).
- **The stat-surface ledger hole and three stale `1200`s** (D5, B7).
- **The reference's QUIC accounting surface, entirely unimplemented in envoy-go** (§3.4): `http.<stat_prefix>.downstream_cx_ssl_total` (the reference DOES mark QUIC connections as SSL one scope up), `http3.*`, `quic.connection.*`, `quic.dispatcher.*`, `udp.downstream_rx_datagram_dropped`, `quic_server_transport_socket_factory.*`.
- **The `TestListenerManager_AllocatesTwoMetricsPerListener` name/body mismatch** (D7) — a test whose name over-claims what it asserts.

**Carried from the BRAINSTORM, unchanged:** the `upstream_cluster` span tag (the RUNNER-UP — deferred on a RESIDUAL DIVERGENCE, **not on cost**; a +3-line `NameOnlyEndpoint` variant was BUILT with 70/70 packages green and cross-side PARITY on the `UF`/`UO` arms) · pick-time `picked`-propagation (2-vs-4 lines CONTESTED) · `access_log[].filter` (10-13) · `hcm.merge_slashes` (10-13, carries a pre-existing H2 `url.Parse` routing bug) · `hcm.access_log_options` (9-11) · the `stdout`/`stderr` loggers (7-9) · `stats_flush_on_admin` (8-10) · the swallowed-panic BOOT HANG · route-level header manipulation · `prefix_rewrite`/`host_rewrite_*` · HCM `server_name`/`via` · the default-config header injection divergence · all tracing remainders · all xDS · all HTTP/3 · gRPC · Runtime · **WASM (a ROADMAP bookkeeping artifact, deliberately left as-is)**.

### 13.1 Deferred-item hygiene

Every item above is recorded HERE and in ADR-0296 §Context; **none is added to the ROADMAP `candidates:` sentence at this stage** (§12). The `ssl` clause SHORTENS at the IMPL and check (2) must STAY **3**.

---

## 14. ADR continuity — the ADR-0296 §Context DRAFT (anchored here; full entry at the IMPL)

Per **ADR-0044-as-used** (⚠️ D1: the citation is a convention, not an ADR-0044 sentence), **ADR-0296's §Context lands at THIS SPEC commit** — the DECISIONS tail flips ADR-0295 → **ADR-0296** here, and the §Context append IS the tail flip; §Decision + §Consequences append IN PLACE at the phase-74 IMPL, **no renumber**; next-free becomes **ADR-0297**.

**The SPEC-stage block shape is pinned by what ACTUALLY LANDED at phases 71/72/73:** heading line · a `> **STATUS: PROPOSED — §Context drafted at the phase-74 SPEC (ADR-0044-as-used).** …` blockquote · **exactly ONE** `### Context (drafted at the phase-74 SPEC)` heading · N context paragraphs · the italic footer `*(§Decision + §Consequences land at the phase-74 IMPL.)*`. **NO `### Decision` heading. NO `### Consequences` heading.** The IMPL's `grep -c '^### Decision'` inside the block must read **0** before it appends. ⚠️ Mirror **ADR-0295/0287**, NOT ADR-0286 — ADR-0286 predates the heading format and uses inline `**§Context.**` bolds.

**ADR-0296 must record:** (a) the C3 misattribution and its **correct** provenance (§1.1 D1/D2) · (b) the classifier fragility — no exported sentinel, string match forced, live-handshake test mandatory, and the **conditional** ctx-override accuracy (§3.5) · (c) the family SPLIT and why the blocker is NAMING not lifecycle (§3.6) · (d) why breaking the +0-stat streak is correct (§7) · (e) the ONE new named departure — the `other` bucket (§3.5) — and, distinctly, the QUIC coverage boundary as **SHARED PARITY** (§3.4), including that this SPEC's own first reading of it was refuted by probe.

⚠️ **The ADR-0286 C3 correction is a SEPARATE edit that lands AT THE IMPL, not at this SPEC** — both precedents (`DECISIONS.md:16901` phase-67/ADR-0289, and `:17209`/`:17185` phase-73/phase-72) landed their correction at the correcting phase's **IMPL**. This SPEC's DECISIONS delta is the ADR-0296 §Context append and **nothing else**; `ADR-0286` is BYTE-UNTOUCHED at this commit. At the IMPL, add an indented blockquote immediately after C3, in the `DECISIONS.md:16901` style:
`  > [CORRECTED at phase 74/ADR-0296: …]` — recording that C3's *"framework-surgery row of its own"* is refuted; that the *"opaque `crypto/tls` handshake-error callback"* half attributed to C3 **was never in C3, nor in DECISIONS.md at all** (`grep -c 'VerifyPeerCertificate\|handshake-error callback'` ⇒ **0**, re-confirmed at this tip), having originated in phase-70 BRAINSTORM prose and been upgraded to *"CONFIRMED"* by phase 72; and closing with what STILL STANDS — **the `ciphers`/`versions` half of C3's boundary survives on its own two blockers**. Phrase it as a documentary finding, **not** as criticism of phase 65, whose own text was accurate.

---

## 15. Exit — counts + expectations at SPEC-DONE

**Docs-only. ZERO production `.go`.** Exactly TWO files in the delta: this `SPEC.md` + the ADR-0296 §Context append. ROADMAP **UNTOUCHED**; row 74 STAYS `in-progress`.

| count | value | command |
|---|---|---|
| fixtures | **119** | `ls -d test/fixtures/[0-9]*/ \| wc -l` (tail `0117-…`) |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` |
| stat surface | **1201** | BEHAVIOR_CONTRACT doc count — **NO mechanical command** (D5) |
| BackendKind | **38** | `H2GoawayResponder`, `fixture.go:614` |
| go.mod modules | **2** (lineage figure) | single `go.mod`, **67** requires (18 direct / 49 indirect) |
| DECISIONS tail | **ADR-0295 → ADR-0296** | at THIS commit; next-free **ADR-0297** |

**Anticipated at the IMPL, NOT YET LANDED:** +3 stats (**1201 → 1204**) · +0 fixtures (**119**) · +0 fuzzers (**55**) · +0 BackendKinds (**38**) · +0 modules · **+0 imports** · ZERO new packages · ZERO new exported symbols.

**Sentinel re-run MECHANICALLY at this close** (does NOT fire; `stop` NOT created): (1) `NOT DONE: row 74` · (2) ⇒ **3** · (3) `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`.

---

## 16. Adversarial-pass record

**The four anticipations this SPEC REFUTED, three of them by execution:**

1. **D-TLSHS-REGSCOPE — "unconditional ⇒ a flat +3."** The reference is **TLS-chains-only**, and the zero-visibility confound was eliminated before the conclusion was drawn. The refutation made the row CHEAPER (`tlsMode` already exists) but made the surface delta **config-dependent**.
2. **D-TLSHS-VERIFYIFGIVEN — "`ssl.handshake` and nothing else."** It is `handshake` **+ `no_certificate`**, and `no_certificate` is a **success-path** counter whose name invites exactly the wrong mapping.
3. **The phase-65 six-name roster** — `ssl.certificate.validation_ca.…` **does not exist**; the reference emits two differently-shaped gauges, and the real surface is 15 eager names + **four** dynamic families.
4. **"QUIC is merely a documentary pin"** — QUIC shares `registerListenerMetrics` and Incs both cx metrics today, so the registration posture silently decides QUIC's surface (R4).

**⚠️ THE CORRECTION THIS SPEC MADE TO ITS OWN REASONING, recorded rather than quietly replaced.** §3.4 was FIRST decided as *"gate QUIC out; a permanently-zero counter is a false measurement, an absent counter is an honest coverage gap."* A probe run specifically to settle it **REFUTED that on the facts**: the reference registers the family on QUIC and leaves it permanently zero on BOTH the success and failure paths, so envoy-go doing the same is **exact parity** and gating QUIC out would have been the departure — envoy-go would have been missing names the reference emits. The argument was sound in the abstract, was cheap to check, and was wrong. **This is the row's own thesis turned on the SPEC itself: an adjective ("a zero is misleading") that felt authoritative because it was well-reasoned, not because it was verified.** The decision it produced was reversed before landing; the reasoning is preserved here so the PLAN does not re-derive it and reach the same wrong answer.

**Two documentary findings that mirror the row's own theme one level up:** ADR-0044 does not contain the discipline the whole project cites it for (D1), and the "first cross-ADR correction" claim is refuted by three precedents — one of them already inside ADR-0286 itself (D2). **The row is about an adjective that acquired ADR authority by repetition; the SPEC found two more of the same species while verifying it.**

**What NO ONE checked, stated plainly:** **no build of row 74 exists** (§1.2). The classifier, the gate, the guard and the asserter are all SPECIFIED and none is COMPILED. The PLAN must not treat any of §3 as execution evidence for buildability.
