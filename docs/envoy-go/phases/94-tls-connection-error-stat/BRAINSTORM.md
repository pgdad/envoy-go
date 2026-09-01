# Phase 94 — `tls-connection-error-stat` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Subject SELF-PICKED** per the 2026-07-12 standing directive; no human pick was solicited and none was given.

**Subject.** Land `listener.<normalized-addr>.ssl.connection_error`, the FIFTH downstream-TLS listener-scope counter, by giving `outcomeOther` — the fourth handshake-outcome bucket, which today increments nothing — a predicate-gated `Inc()`. This closes the NAMED DEPARTURE recorded at `BEHAVIOR_CONTRACT.md:1971` and completes the taxonomy phase 74 opened and phase 75 extended.

**Family:** Observability, `ssl.*` sub-family — the direct continuation of row 74 (`tls-handshake-outcome-stats`) and row 75 (`tls-no-certificate-stat`).

---

## 0. What this stage refuted

Eleven, each by execution at this tip, not by reading:

1. ⚠️ **THE INHERITED "REFERENCE SIDE IS AN UNPROBED DOC CLAIM" IS FALSE.** `ROADMAP.md:155` and `STATE.md:18` both say this candidate was rejected at phase 93 because *"its reference side is a doc claim, not a measurement — nobody has probed what `contrib-v1.37.2` books under that name."* **Phase 77 probed it**, in detail, on 2026-07-26. The measurement is at `phases/77-runtime-static-layer/BRAINSTORM.md:216`. Phase 93 did not know, because the result was **never propagated out of that BRAINSTORM**.
2. ⚠️ **THE BLOCKER IS NOT A TWO-DOCUMENT CONTRADICTION. IT IS A FIVE-POSITION DIVERGENCE, AND THE TWO MOST-CITED POSITIONS ARE THE OLDEST TWO.** See §3.4. The controller opened this stage believing there were two; an agent refuted that and enumerated five.
3. ⚠️ **THE CONTRADICTION WAS LANDED ATOMICALLY BY ONE AUTHOR IN ONE COMMIT.** `git blame` puts `BEHAVIOR_CONTRACT.md:1971` (*"still blocked on enumerating its membership"*) and `DECISIONS.md:17390` (*"blocker RETIRED"*) both at **`c57b98b8`**, the phase-75 IMPL. This is not stale drift between stages; it is a self-contradiction inside a single commit.
4. ⚠️ **PHASE 77'S "THE POSITIVE POPULATION CANNOT BE TYPED IN GO" IS REFUTED BY INVERSION.** Phase 77 concluded the deny-list is **open** because mismatch / malformed-DER / version-mismatch all arrive as bare `*errors.errorString`, separable only by message text. True — and irrelevant. **You do not type the positive population. You type the CLOSED transport population and `Inc()` otherwise.** See §3.1. This is the finding that makes the row tractable.
5. ⚠️ **THE PHASE-93 COST FIGURE `~4-6 PRODUCTION LINES ACROSS 2 FILES` IS REFUTED.** The **2 files** holds for production. The **4-6 lines** does not: measured **35-55 added / 12-22 removed**, because four landed prose comments become false and one of them carries an unguarded count. See §6.
6. ⚠️ **AND THAT FIGURE OMITTED THE ENTIRE TEST-PIN SURFACE — FOUR MORE FILES, TWO OF THEM EXACT-SET PINS.** `reference_measured_prototype_is_a_lower_bound` would have fired a **NINTH consecutive row** on the inherited roster. See §6.2.
7. ⚠️ **THE NEW NAME PREPENDS. IT DOES NOT APPEND.** Both roster pins are sorted; under `LC_ALL=C`, `ssl.connection_error` collates **FIRST of five** (`c` < `f` < `h` < `n`). Phase 75's precedent was a pure append and does **not** transfer.
8. ⚠️ **FOUR MUTUALLY INCONSISTENT COST FIGURES ARE LIVE IN THE TREE, IN TWO DIFFERENT UNITS.** `~9-11 tasks` · `~10-13` · `~12-15` · `~4-6 production lines`. None was measured at this tip. `reference_deferred_candidate_cost_restale` fires.
9. ⚠️ **THE `~9-11` FIGURE ENUMERATES NOTHING** — it is a single table cell with no task spine, and it **collides** with an unrelated `~9-11` at `phases/75-.../SPEC.md:349` that re-costs phase 75's OWN row. A grep for the figure conflates them.
10. ⚠️ **THE PICK-MENU INSTRUCTION THIS STAGE INHERITED IS UNACHIEVABLE AS WRITTEN.** `next-prompt.txt` says to *"prefer a candidate that EMPTIES a check-(2) window."* Phase 77 measured the **shortest** window at **~6-8 rows and ~60-70 tasks** to empty. No single row empties any window. See §4.9 — and this row does not either, stated plainly.
11. ⚠️ **A ` + ` SPLIT OF THE WINDOW SENTENCES IS WRONG ON THREE OF SIX WINDOWS** and reports the window `next-prompt.txt` calls "the SMALLEST" as having **ONE** item when it has **three**. Recorded so no gate is ever built on it. The project rule *"no gate may rest on a ` + ` split"* is thereby confirmed empirically rather than merely asserted.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 The one thing that blocked this candidate is now MEASURED, at this tip

Across **five stages** (74, 75, 77, 92, 93) this candidate was named the cheapest identified follow-on and deferred every time. The blocker was always the same: **what does the reference actually count under `ssl.connection_error`?** Phase 74 drove two arms and said the membership was otherwise unknown. Phase 77 drove nine and answered it — then left the answer in a BRAINSTORM. Phase 92 and 93 re-deferred, phase 93 explicitly on the ground that the answer did not exist.

**This stage drove twelve arms on the live pinned image and answered it independently.** §2 is that measurement. It is the first time the result exists outside a phase-77 document, and recording it is a substantial part of this row's value regardless of what the SPEC does next.

### 1.2 Why "smallest defensible" selects it — stated as a trade-off, not a ranking

⚠️ **THIS ROW IS NOT THE SMALLEST CANDIDATE BY PRODUCTION LINES, AND THIS DOCUMENT DOES NOT CLAIM IT IS.** 1xx drop-and-deliver is ~5-15 production lines against this row's 35-55. What selects this row is the **defensibility** term:

| | this row | 1xx | GET `/runtime` |
|---|---|---|---|
| production lines (measured) | 35-55 | **5-15** | ~160 |
| reference side | **MEASURED, 12 arms, this tip** | in-code spec only | measured at phase 77 |
| closes a NAMED DEPARTURE | **yes** (`BEHAVIOR_CONTRACT` B5) | no | no |
| sits in a check-(2) window | yes (`:225`) | **NO — none** | yes (`:231`) |
| new fixture forced | yes, +1 | yes, +1 **and a new BackendKind** | no, extends `0118` |
| new stat name | +1 | +0 | +0 |

**The decisive factor is (2), not size.** This row's blocking unknown has been re-opened and re-deferred five times; it has now been closed twice independently (phase 77, and this stage) and the second measurement is fresh. Deferring a sixth time would discard a measurement whose first instance was already lost once.

### 1.3 What this row does NOT buy — stated plainly

⚠️ **THIS ROW MOVES THE TERMINATION SENTINEL BY EXACTLY NOTHING, AND IT MUST NOT BE CHARTERED AS THOUGH IT DOES.** It narrows the `:225` Observability sentence by one clause. Phase 77 recorded the decisive asymmetry and it is re-confirmed here by this stage's own positive control (§8.4): **check (2) keys on the PHRASE `deferred candidates:`, not on clause content.** Doctoring the phrase out of a scratch copy drives the check to `0`; deleting a clause leaves the sentence — and the phrase — in place, so the check still prints **6**. A narrowing row buys **zero** sentinel progress. This row is chartered as **narrowing**, never as a step toward silencing check (2).

---

## 2. The reference side, MEASURED — twelve arms on the live pinned image

### 2.1 The rig and its controls, stated BEFORE the result

Image `envoyproxy/envoy:contrib-v1.37.2`, digest **`sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`**, **verified against `docs/envoy-go/ENVOY_TARGET.md` before any arm was trusted** (`reference_cost_figure_measured_at_publishing_commit` applies to reference pins too).

Two listeners in ONE Envoy process, so every arm is scraped from the same admin endpoint in the same run:
- `:10000` — plain TLS, `minimum_protocol_version: TLSv1_2`
- `:10001` — `require_client_certificate: true` + `trusted_ca`

Controls, both required by `reference_absence_claim_needs_positive_control`:
- **POSITIVE CONTROL:** arms (a) and (b) are known-firing from phase 74. If they had read 0, the rig never reached the listener and every other zero would be worthless.
- **DISCRIMINATING NEGATIVE CONTROL:** a full two-listener **set reconciliation** — every connection accounted for, so a zero is provably "reached the listener and booked nothing", not "never arrived".

### 2.2 The result

| arm | reference delta | Envoy's own log line |
|---|---|---|
| (a) TLS-1.0-only client | `ssl.connection_error +1` | `TLS_error: 268435696 UNSUPPORTED_PROTOCOL` |
| (b) plaintext HTTP to the TLS port | `ssl.connection_error +1` | `TLS_error: 268435612 HTTP_REQUEST` |
| (d) garbage bytes | `ssl.connection_error +1` | `TLS_error: 268435703 WRONG_VERSION_NUMBER` |
| (f1) client cert with mismatched key | `ssl.connection_error +1` | `RSA:LAST_OCTET_INVALID, SSL:BAD_SIGNATURE` |
| (f2) malformed-DER client cert | `ssl.connection_error +1` | `SSL:CANNOT_PARSE_LEAF_CERT` |
| **(c) partial ClientHello then clean FIN** | **NOTHING under `ssl.*`** | `connection_impl.cc:781 remote close` |
| **(c2) connect, zero bytes, clean FIN** | **NOTHING under `ssl.*`** | `remote close` |
| **(c3) partial ClientHello then RST** | **NOTHING under `ssl.*`** | `remote close` |
| **(e) connect, zero bytes, RST** | **NOTHING under `ssl.*`** | `remote close` |
| (e2) full ClientHello then RST | `ssl.handshake +1`, no `connection_error` | RST landed POST-handshake |
| (control) no client cert on mTLS | `ssl.fail_verify_no_cert +1` | `PEER_DID_NOT_RETURN_A_CERTIFICATE` |
| (control) valid cert, happy path | `ssl.handshake +1` | — |

**Set reconciliation, the discriminating control.** `:10000` closed at `downstream_cx_total: 17` / `ssl.connection_error: 8`; exactly eight arms were SSL-protocol errors, and the other **nine** connections each booked `downstream_cx_total +1` and **zero** `ssl.*`. `:10001` closed at total 9 = `connection_error 3` + `fail_verify_no_cert 3` + `handshake 2` + RST 1. **Both listeners reconcile exactly; there are no stray increments.**

### 2.3 THE RULE, stated so an implementer can code it

> The reference increments `ssl.connection_error` **iff BoringSSL reports an actual SSL protocol error** — an `ssl_socket.cc` `TLS_error` line. A transport-level EOF or reset during the handshake takes `ConnectionImpl`'s generic `remote close` path and books **nothing** under `ssl.*`.

### 2.4 ⚠️ A MID-PROBE READING WAS WRONG AND WAS CAUGHT BY ITS OWN CONTROL

Arm (f2) initially read `fail_verify_no_cert +1` under TLS 1.3. That was an **artifact of the probe client, not of the reference**: Go silently sent an EMPTY certificate chain because it cannot select a signature algorithm for an unparseable leaf, so the malformed certificate never went on the wire. Re-run under TLS 1.2 with the client printing `SENDING chain of 1 cert(s), leaf 200 bytes` and receiving `remote error: tls: error decoding message`, the reference books `connection_error`.

⚠️ **THIS IS `reference_go_client_cert_withholding` FIRING AGAIN, IN A NEW DIRECTION** — previously recorded for untrusted certs, now observed for **unparseable** ones. The SPEC's probe arms must force-send in both cases. **A single-run (f2) probe would have shipped a wrong reference mapping.**

**Consequence:** the standing production comment at `internal/listener/manager.go:409-412` — *"A cert/private-key mismatch and a malformed DER never reach `certs[0].Verify()` … The reference books those under `ssl.connection_error`"* — is **CORRECT AS WRITTEN**, and is now measured rather than asserted.

### 2.5 What this refutes about the inherited framing

Agent B's pre-probe hazard — that the reference might book mismatch/malformed-DER under `ssl.fail_verify_error`, which would make a naive mapping **open a new departure rather than close one** — is **REFUTED BY MEASUREMENT**. Both land in `connection_error`. The hazard was real and correctly raised; the answer is favourable.

### 2.6 Arms that could NOT be determined — recorded, not glossed

- **`context.DeadlineExceeded`** has no reference counterpart that could be exercised. Envoy books handshake timeouts under `listener.<addr>.downstream_cx_transport_socket_connect_timeout` (present in the scrape, stayed **0**), not under `ssl.connection_error`. Excluding it is the behaviour-matching choice, and it is structurally unreachable in envoy-go today (`manager.go:423-429`).
- **(e2)** did not achieve a genuine pre-completion reset after a FULL ClientHello — TLS 1.3 completes server-side in ~2 ms. **Arm (c3) covers the mid-handshake-reset question properly** and answers it: nothing booked.

---

## 3. The mechanism, stated precisely

### 3.1 ⚠️ THE PREDICATE IS AN EXCLUSION LIST, NOT AN INCLUSION LIST — AND THAT IS THE WHOLE FINDING

Phase 77 concluded the row was hard because *"the positive population cannot be typed in Go (mismatch / malformed-DER / version-mismatch all arrive as bare `*errors.errorString` distinguishable only by message text)"* and that *"the deny-list is OPEN"*.

**The premise is TRUE. The conclusion does not follow.** The positive population need never be typed, because §2.3's rule partitions the space into exactly two classes and **the OTHER class is closed and fully typed**:

- **Protocol errors** (increment) — open-ended, untypeable, message-text-only. **Never enumerated.**
- **Transport errors** (do not increment) — `io.EOF`, `io.ErrUnexpectedEOF`, `syscall.ECONNRESET`, `net.ErrClosed`, `context.DeadlineExceeded`. **Closed, and every member is `errors.Is`-able.**

So the predicate is: **exclude the typed transport set, `Inc()` otherwise.** No message-text matching, no open deny-list, no fragile string tripwire.

⚠️ **THIS INVERTS THE FRAGILITY THE EXISTING CLASSIFIER ALREADY CARRIES.** `outcomeNoCert` is matched by exact error TEXT (`noClientCertErrText`, `manager.go:442`) precisely because `crypto/tls` exports four error TYPES and ZERO error VALUES. The new arm needs **no** such tripwire, because it matches the complement. **The SPEC must not copy the `outcomeNoCert` string-matching pattern into the new arm; doing so would import a fragility the design does not need.**

⚠️ **A DRAFTING FOOTGUN, INHERITED FROM PHASE 77 AND WORTH CARRYING:** `tls.RecordHeaderError` is returned **BY VALUE** — `errors.As(err, &val)` is true, `errors.As(err, &ptr)` is **false**, wrapped or not. A predicate drafted against a pointer **compiles and never matches**. This is a silent-vacuity hazard of exactly the shape `reference_passing_test_is_not_a_guard` describes.

### 3.2 Where the change goes — every anchor SYMBOL-VERIFIED and UNIQUENESS-CHECKED at this tip

| what | anchor | verified |
|---|---|---|
| outcome enum | `type handshakeOutcome int` | `manager.go:431` |
| the `outcomeOther` variant | `outcomeOther` in the const block | `manager.go:437` |
| classifier fall-through | `return outcomeOther` | `manager.go:455` — **UNIQUE repo-wide** |
| the consuming switch | `switch classifyHandshakeErr(err)` | `manager.go:1294` |
| where the new `case` lands | `case outcomeVerifyError:` | `manager.go:1295` — **UNIQUE repo-wide (`git grep -c` = 1)** |
| the sole Inc site | inside `serveConnection`'s `if selected.tlsCfg != nil` | `manager.go:1286` block |

⚠️ **`serveConnection` IS PROVABLY THE SOLE `Inc` SITE FOR THE WHOLE FAMILY** — `git grep` on all four field names returns exactly two `.go` files, and in `manager.go` the only `.Inc()` occurrences are `:1296`, `:1298`, `:1305`, `:1310`, all inside that one block. This is the same structural fact `BEHAVIOR_CONTRACT`'s QUIC permanent-zero argument rests on, so **the fifth name inherits QUIC parity by construction**, exactly as the fourth did.

⚠️ **ANCHOR UNIQUENESS WAS CHECKED, NOT ASSUMED.** At the phase-93 IMPL a supposedly drift-proof anchor matched TWO production lines and a line-targeted edit would have corrupted an unrelated driver. Both edit targets above return exactly one match.

### 3.3 The nil-pointer hazard is REAL and must land with the arm

`rt.sslConnectionError` must be registered on the same `rt.tlsMode` gate as its four peers (`manager.go:384-388`). A nil `*stats.Counter` `Inc()` is a **PROCESS CRASH** in a background goroutine with no `recover()` (`reference_nil_stats_counter_inc_crashes_goroutine`). `TestListenerMetrics_GateMatchesInc` (`manager_test.go:2281-2298`, `:2340-2355`) carries per-field nil/non-nil guards and **must gain the fifth field in the same commit as the `Inc`**.

### 3.4 ⚠️ THE FIVE LANDED POSITIONS THAT MUST BE RECONCILED

None of these is stale drift between distant stages; positions 1 and 2 were landed by **one author in one commit**.

| # | site | position | verdict at this tip |
|---|---|---|---|
| 1 | `BEHAVIOR_CONTRACT.md:1971` | *"still **blocked** on enumerating its membership"* | ⚠️ **WRONG — the SPEC must delete this clause** |
| 1b | `BEHAVIOR_CONTRACT.md:1971` | *"The full membership … is UNENUMERATED"* | ✅ **CORRECT — keep it.** The correction is SURGICAL, one clause, not the paragraph |
| 2 | `DECISIONS.md:17390` (same commit `c57b98b8`) | blocker RETIRED; `~9-11` tasks; ONE `io.EOF` predicate | partly right, figures superseded |
| 3 | `phases/77-.../BRAINSTORM.md:216` | `~12-15`; THREE predicates; deny-list **OPEN** | measurement CORRECT, conclusion refuted (§3.1) |
| 4 | `ROADMAP.md:155` + `STATE.md:18` | `~4-6 production lines`; reference side **unprobed** | ⚠️ **BOTH FALSE** |
| 5 | `phases/93-.../BRAINSTORM.md:235-244` | same as 4, marked **MEASURED** | ⚠️ a verification table laundering a wrong cite — `reference_verification_table_launders_wrong_cites` |

⚠️ **`SSL_ERROR_SSL` — the token naming the actual rule — appears ZERO times in `DECISIONS.md` and ZERO times in `BEHAVIOR_CONTRACT.md`.** It exists only in the phase-77 BRAINSTORM, its ROADMAP row-77 cell, and `STATE_HISTORY.md:29`. This is `reference_brainstorm_adjective_acquires_adr_authority` in its most literal form: **a measurement that never reached the documents that govern the decision, and was then contradicted by a row that did not know it existed.**

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 GET `/runtime` (window `:231`) — **the runner-up, and it is genuinely close**

⚠️ **THIS IS THE STRONGEST REJECTED CANDIDATE AND ITS REJECTION IS THE LEAST COMFORTABLE.** Measured: `internal/runtime/snapshot.go` is **115 lines**; `Snapshot` holds only `keys []string` + `numLayers int`; `NewSnapshot` **discards every value and layer name** — deliberately, and its own doc comment says why: *"The override VALUE is not retained: **this row serves no `/runtime` endpoint**."* That is a **landed forward-pointer inviting exactly this row.**

Cost: **~+160/-15 production across 4 files**. **No constructor widening** — `admin.Server.bs` (`admin.go:40`) already reaches `Bootstrap.Runtime` (`bootstrap.go:552`). **+0 fixtures** (`0118-runtime-static-layer` already scrapes admin on both sides, declares 2 layers / 6 keys, drives no backend traffic), **+0 stats**, **+0 BackendKinds**.

**REJECTED on two grounds, neither of them size.** (a) Its reference side has a **measured non-determinism**: within-layer collisions flip **~40/60 across process starts** (phase-77 SPEC §3.3.1, 18 fresh processes), so a cross-side body comparison is only sound on a **collision-free** config — a constraint that must be designed in, not discovered at IMPL. (b) It is **~4x this row's production delta** and lands a new admin endpoint rather than closing a named departure. ⚠️ **It is the best next pick after this one, its reference side is already measured, and its forward-pointer is landed in production code.** **MEASURED.**

### 4.2 `POST /runtime_modify` — **strictly larger, and it drags a reject-lift**

Needs a mutable admin layer, but `bootstrap.go:575` hard-rejects `admin_layer` and its own comment says *"envoy-go ships no POST /runtime_modify, so the layer could never gain a key."* Landing it means **lifting that reject**, which `reference_lifted_reject_hidden_enforcement` requires to land **atomically** with the write path. It also makes `Snapshot` mutable against its documented immutability, turns `runtime.num_keys`/`num_layers` **LIVE** (invalidating `0118`'s assertions and the boot-fixed contract), and needs an ADR-0090 amendment. ~2-3x §4.1. **REJECTED as strictly larger.** **MEASURED.**

### 4.3 Lifting the six `runtime_key` parse-reject arms — **a strict SUPERSET of §4.1, not a peer**

All six enumerated at this tip, in **two** packages: `adaptive_concurrency/compiled_config.go:171`; `admission_control/compiled_config.go:179, :182, :185, :189, :192`. ⚠️ **`parseRejectEnabledRuntimeKey` is defined INDEPENDENTLY IN BOTH PACKAGES** — a bare-name grep conflates them; `reference_symbol_assertion_needs_qualified_name` applies to any roster or gate here.

Honoring a `runtime_key` needs, in order: values retained in `Snapshot` (**the whole of §4.1's work first**), a decision at the `numerator`/`denominator` flattening terminator (`sr_threshold` is a `RuntimePercent`), and **a new `FactoryCtx` field** — verified: `internal/filter/http/types.go` contains **no runtime field of any kind**, and the only consumer of `internal/runtime` in the entire tree is `internal/bootstrap/bootstrap.go`. That is a framework seam needing its own ADR. **≥ +250/-40 across ≥6 files, two ADRs amended, 12 doc files, 2 fuzzers re-baselined — phase-77 class.** **REJECTED: costing it as an alternative to §4.1 is a category error; the honest ordering is §4.1 then this.** **MEASURED.**

### 4.4 1xx interim responses — **the cheapest CODE, and it buys nothing**

Symbol located: `(*ClientConn).onResponseHeaderBlock` (`internal/filter/hcm/h2/client.go:631`), the branch at `:655`, field `respHeadersSeen` at `:199`, in-code spec at `:729-750`. The **~5-15 production line, one-file estimate is CONFIRMED** for the diff itself.

⚠️ **BUT ITS WINDOW CLAIM IS REFUTED: `git grep -i -- '1xx' docs/envoy-go/ROADMAP.md` returns EXACTLY lines 154 and 155 — both phase-ROW cells. It sits in NO deferred-candidate window at all.** And the differential cost is **not** zero: no `BackendKind` emits an interim H2 HEADERS block (tail is `H2GoawayResponder = 38`), so it needs **a new BackendKind and a new fixture** — three registration gates. Plus an undecided arm: a 1xx block carrying END_STREAM would `finish(nil)` with a zero response.

**REJECTED: cheapest code, ~+900 all-in, and zero window effect.** ⚠️ **`next-prompt.txt` lists it as a banked candidate at "~5-15 lines, one symbol" without either caveat.** **MEASURED.**

### 4.5 Window `:239` (Operational tooling) — **fewest items, WORST pick, and the reason is measured**

⚠️ **"SMALLEST WINDOW" AND "CLOSEST TO EMPTY" ARE NOT THE SAME PROPERTY, AND THIS WINDOW PROVES IT.** Its three items are the **most blocked** in the inventory:
- *xDS-sourced dry-validation* — `internal/xds/` is **SDS-only**; there is no CDS/LDS/RDS client anywhere. Blocked behind the entire `:215` window.
- *admin-API live-reload-and-validate* — `internal/listener/manager.go` exposes `Start`/`Listeners`/`Drain`/`Stop` and **no replace or reconfigure path exists in the tree**. Live reload is a subsystem, filed by the ROADMAP itself under "hot restart proper" at `:231`.
- *RTDS/SDS validate companion* — the SDS half is already substantially delivered (phase 86, ADR-0308) but row 86 is a **MAINTENANCE row claiming no family ordinal**, so it consumed no candidate and the sentence was never updated. The RTDS half is blocked on a `:231` item.

⚠️ **`:239` CANNOT EMPTY WITHOUT `:231` WORK — a CROSS-WINDOW DEPENDENCY the PICK MENU does not record.** **REJECTED, and no single row can empty it.** **MEASURED.**

### 4.6 A narrow admin `POST /config_validate` — **REJECTED: it would LENGTHEN `:239`**

Genuinely cheap (~+75 production lines, one new `internal/admin/validate.go` on the `drain.go` model, one `mux.HandleFunc`; `validate.Bootstrap` already exists at `validate/validate.go:39` and `Server.bs.ConfigPath` is already set). **But upstream Envoy ships no such endpoint, so there is NO REFERENCE SIDE** — it is not differentially provable, which is this project's core selection criterion. And it is not live-reload, so it would not consume the item as written. **REJECTED.** **MEASURED.**

### 4.7 The other `:225` clauses — **blocked on NAMING, not on lifecycle**

The four dynamic `ssl` families (`ciphers`/`versions`/`curves`/`sigalgs`) remain blocked exactly as recorded: the stats name pattern **bans the hyphen**, so an OpenSSL-form cipher name fails `stats.IsValidName` and **panics at registration**, and Go's IANA spellings disagree with Envoy's OpenSSL ones. Phase 77 additionally measured that `sigalgs` may be **unimplementable**: `crypto/tls.ConnectionState` exposes `Version`, `CipherSuite`, `CurveID` and **no signature-scheme field**. **≥3 rows, ~30+ tasks. REJECTED.**

### 4.8 The tracing trio (`spawn_upstream_span` / `http_service` / force-trace) — **3 rows, ~27 tasks**

The lineage's own sizing calls force-trace *"a whole new subsystem. HIGH scope-risk."* **REJECTED as a pool.**

### 4.9 The six windows as a pool — **REJECTED, and the PICK-MENU instruction is unachievable**

Phase 77 costed the **shortest** window (Observability, 3 clauses) at **~6-8 rows and ~60-70 tasks**. ⚠️ **No single row empties any window**, so `next-prompt.txt`'s instruction to *"prefer a candidate that EMPTIES a check-(2) window"* cannot be satisfied by any row this project could charter. It should be re-worded to *"narrows"* — and narrowing, per §1.3, buys zero sentinel progress. **The instruction is not merely unmet; it is unmeetable.**

---

## 5. Family attribution

**The TWENTY-FIFTH Observability-family row** *(chain ordinal — the inherited off-by-one is KEPT, exactly as phases 73, 74 and 75 kept it)*. The family **STAYS OPEN**. This is the **THIRD** downstream-`ssl.*` row and a direct continuation of the phase-74 seam, following phase 75.

Chain re-derived at this tip, not inherited: **74 = TWENTIETH · 75 = TWENTY-FIRST · 79 = TWENTY-SECOND · 80 = TWENTY-THIRD · 81 = TWENTY-FOURTH.** Nothing between 82 and 93 claims an Observability ordinal (rows 85-93 are Core-HCM/H2 maintenance rows claiming none; row 78 explicitly claims none).

⚠️ **THE ORDINAL PHRASE HAS TWO LANDED FORMS AND A NAIVE GREP READS ONLY ONE.** Rows 73-75 write `The TWENTIETH Observability-family row`; rows 79-81 write `TWENTY-SECOND §9 Observability-family row` — an interposed `§9`. A pattern anchoring the ordinal **immediately** before the phrase **silently returns row 75 as the tail** and would have made this row the TWENTY-SECOND, colliding with row 79. **Measured live at this stage: the naive form was run first and did exactly that.** `topic_roster_golden_pitfalls` fires; the fix is to locate the phrase and read backwards, not to anchor forwards.

---

## 6. The cost FLOOR — measured, and explicitly a LOWER BOUND

### 6.1 Production: 2 files, and the line count is NOT 4-6

| file | what changes | added | removed |
|---|---|---|---|
| `internal/listener/manager.go` | struct field, `NewCounter` registration, new `case outcomeOther:` arm **with the exclusion predicate**, and **four prose comments that become FALSE** | 30-45 | 10-18 |
| `internal/stats/name.go` | `helpText` entry + the doc-comment enumeration | 5-10 | 2-4 |
| **total production** | **2 files** | **35-55** | **12-22** |

**The four false comments are what refutes `~4-6`:** `manager.go:175-177` (*"the four `ssl.*` counters … all four pointers stay NIL"*), `:392-393` (*"three counted buckets"*), `:398-400` (*"carries FOUR `ssl.*` counters"*), and `:1291-1293` (the *"outcomeOther deliberately increments NOTHING"* comment — which is the change itself).

⚠️ **`internal/stats/name.go` CARRIES AN UNGUARDED PROSE COUNT THAT ROTS SILENTLY.** Its doc comment says *"Of the 30 entries…"* and *"All four `ssl.*` entries have three-dot source names"*. **Both verified at this tip: `helpText` holds exactly 30 entries.** Adding a 31st rots both, and **no test asserts `len(helpText)`** — `git grep 'len(helpText)'` returns **NONE**.

### 6.2 ⚠️ THE TEST-PIN SURFACE — FOUR FILES THE INHERITED ESTIMATE NEVER NAMED

| file:line | pin | effect |
|---|---|---|
| `manager_test.go:2136-2145` | `want := []string{…4 names…}` + `reflect.DeepEqual` | **RED.** Exact set. |
| `quic_test.go:280-288` | second exact-set pin, same shape | **RED.** |
| `stats/helptext_test.go:44-47` + `TestHelpText_KeySetExact:100` | **bidirectional SET EQUALITY**, reports `missing` and `extra` separately | **RED either way** — entry without roster line ⇒ `extra`; roster line without entry ⇒ `missing`. |
| `stats/name_test.go:239-243` | forward-only lookups | ⚠️ **SILENTLY GREEN if omitted** |
| `manager_test.go:4580` `sslLeafRoster` | negative half of `assertSSLCrossProduct` (4 call sites) | must extend, or `counterValue` errors on an absent name |
| `manager_test.go:2281-2298`, `:2340-2355` | `TestListenerMetrics_GateMatchesInc` nil guards | must extend — §3.3 |

⚠️ **`stats/name_test.go` IS A KNOWN, DOCUMENTED TRAP THAT FIRES ON EXACTLY THIS CHANGE.** Its own comment at `:232-237` records the phase-75 execution verbatim: *"Landing a fifth `ssl.*` helpText entry without extending this slice leaves it SILENTLY UNGUARDED (EXECUTED at the phase-75 PLAN: with the phase-75 entry present and this slice left at three, the whole package stayed GREEN)."* **The trap names this row's exact change and predicts its exact failure.**

⚠️ **`helptext_test.go` DID NOT EXIST AT PHASE 75** — created 2026-07-29 at `895f0be2` (phase 79), four days after the phase-75 IMPL (`c57b98b8`, 2026-07-25). **So the phase-75 diff STRUCTURALLY UNDERSTATES this row**, and any estimate anchored on it inherits that understatement. This is `reference_measured_prototype_is_a_lower_bound` in its file-enumeration form, caught before it fired a ninth time.

⚠️ **A TEST RENAME IS FORCED, ACROSS 4 SITES:** `TestListenerMetrics_TLSListenerRegistersExactlyFourSSLNames` -> `…Five…` (`manager_test.go:2021`, `:2102`, `:2111`, `:4576`). `reference_differential_run_selector`: a stale `-run` selector carrying the OLD name **prints `ok` and exits 0 having run nothing.**

### 6.3 The fixture: +1, and it is FORCED

⚠️ **NEITHER `0110` NOR `0111` CAN CARRY THIS, AND THE REASON IS STRUCTURAL, NOT STYLISTIC.** Both are `fixture.StatsAsserter` legs reading `/stats/prometheus` with a **named subset**, so a fifth name does **not** redden them — but every arm they drive is certificate-shaped and classifies `outcomeOK` / `outcomeVerifyError` / `outcomeNoCert`. **No existing arm anywhere produces an `outcomeOther`.**

Adding a drive arm to either breaks its own determinism argument: `0111/driver/driver.go:710-716` reasons that *"The three arms of driveSide are therefore the ONLY connections `l_edf` ever sees ⇒ deterministically 3 accepts"*, and a fourth arm moves `downstream_cx_total` 3 -> 4 and invalidates `wantObservable`. `0110` has the identical structure. **Phase 75 reached the same conclusion and stated it: a NEW fixture is FORCED.** ⚠️ Its cite `0111/driver.go:99` for `wantObservable` is **STALE — it is `:181` at this tip** (`:99` is now `sdsProjectedNames`).

A `connection_error: 0` pin on an existing fixture instead would be a **vacuous `0 == 0`**, precisely the shape `0111`'s own README rejects for `ssl.no_certificate`.

**Anticipated:** fixture **`0120`** (verified FREE as a directory), reference port in the free in-band range **`10126-10129`** (all four verified unused repo-wide). `BackendCount` must be **>= 1** even though this fixture drives no backend traffic (`reference_differential_backendcount_min_one`), on the `0118` precedent. **+0 BackendKinds** — the drive arms are raw TLS clients, not backend shapes.

### 6.4 Anticipated counts — every axis re-derived at this tip

| axis | now | after this row |
|---|---|---|
| ROADMAP data rows / `want` | **125** | **126** (this BRAINSTORM's own insertion) |
| ROADMAP lines | **243** | 244 |
| stat surface | *(contested — DELTA ONLY, no absolute)* | **+1 name** |
| fixtures | **121** (tail `0119`) | **122** (`0120`) at IMPL; **+0 at this stage** |
| BackendKind tail | **38** | **38** |
| fuzzers | **56 targets / 48 files** | unchanged — no new config field, so no parse arm |
| `go.mod` requires | **67** (18 direct + 49 indirect) | unchanged |
| phase dirs | **134** | **135** |
| next-free ADR | **`ADR-0316`** (TAIL-derived) | ADR-0316 drafted at the SPEC |

⚠️ **THE STAT SURFACE IS QUOTED AS A DELTA AND NEVER AS AN ABSOLUTE** — three different absolutes are live in this tree at one tip; `reference_a_drift_correction_is_itself_a_claim` says on a contested count, **no number**.

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

Unlike phase 93, which reddened a pin already in the tree, **this row has no existing failing gate.** The five `outcomeOther` rows at `manager_test.go:4398-4402` pin the CLASSIFIER, not the counter, and they pass today. The new fixture (§6.3) is the gate, and it does not exist yet.

⚠️ **THIS IS A REAL WEAKNESS OF THE PICK AND IT IS NOT MINIMISED HERE.** It means the SPEC must build its own instrument and prove it non-vacuous, rather than inheriting a red one.

### 7.2 The five landed `outcomeOther` rows, with their reference verdicts NOW KNOWN

| row (verbatim) | reference books | new classification |
|---|---|---|
| `{"unrelated error", errors.New("connection reset by peer"), outcomeOther}` | **nothing** (arm c3/e) | must **EXCLUDE** |
| `{"io.EOF", io.EOF, outcomeOther}` | **nothing** (arm c/c2) | must **EXCLUDE** |
| `{"cert/key mismatch shape", …ECDSA verification failure…}` | **`connection_error`** (arm f1) | must **INCREMENT** |
| `{"malformed DER shape", …x509: malformed certificate…}` | **`connection_error`** (arm f2) | must **INCREMENT** |
| `{"ctx deadline", context.DeadlineExceeded, outcomeOther}` | no counterpart; timeouts book `downstream_cx_transport_socket_connect_timeout` | must **EXCLUDE** |

**Before this stage, zero of these five had a measured reference verdict. All five now do** (four measured, one determined to have no counterpart). ⚠️ **Three of the five must EXCLUDE — a naive `case outcomeOther: Inc()` would be wrong on a MAJORITY of the rows the tree already pins.**

⚠️ **THE TABLE MUST GAIN A `wantErrAbsent`-STYLE NEGATIVE COLUMN**, per phase 93's finding: asserting a pin fired says nothing about which pins did **not**. A row proving `connection_error` incremented must also prove `fail_verify_error` did not.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN INSERTION

### 8.1 PRE-INSERTION, at `b50bb7f7`

- **(1)** SILENT
- **(2)** SIX: `203 209 215 225 231 239`, per-line md5 `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 4ad940205410 b2680e6f4fbf 6caa1c3ce0e7` — **byte-identical to the phase-93 close**, proving no candidate line was tidied between rows
- **(3)** SILENT

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

- **NC-A** (row 62 doctored to `in-progress`, `want=125`): **ONE** line, `NOT DONE: row 62`
- **NC-B** (`want=124` on the real file): **ONE** line, `GATE FAIL: examined 125 data rows, expected 124`
- **NC-C** (`gRPC-family row` doctored out): **FIRED**, `NEVER OPENED: gRPC`; **WASM control 2** (still present, so the NC is targeted, not global)
- **NC-D** (`-family row` with `--`): occurrences **95**, lines **67**

⚠️ **NC-A AND NC-B ARE BOTH ONE-LINE SHAPES**, as `next-prompt.txt` predicted — they collapsed from two at the phase-93 IMPL's own row flip. **This stage's insertion changes them again: see §8.4.**

### 8.3 ⚠️ THE CHECK-(2) POSITIVE CONTROL — AND WHAT IT PROVES ABOUT WINDOW-NARROWING

Doctoring **both** candidate phrases out of a scratch copy drives check (2) to **0**. ⇒ the check **can** reach its passing state, so its SIX is real signal and not a stuck gate.

⚠️ **AND IT PROVES SOMETHING THE PICK MENU DOES NOT SAY.** The control had to remove the **PHRASE** to silence the check. Removing a *clause* from a window sentence leaves the phrase — and therefore the line, and therefore the count — untouched. **This is the mechanical confirmation of phase 77's decisive asymmetry (§1.3): narrowing a window CANNOT move check (2).** Only deleting the line can, and deleting it is forbidden.

### 8.4 POST-INSERTION — measured on both sides of this stage's own row

Row 94 is an **ADD**, not a flip, so unlike phase 93 the denominator **MOVES: `want` 125 -> 126**, and `ROADMAP.md` 243 -> 244.

**Field count of the new row, under BOTH forms, BEFORE and AFTER installing** (`reference_markdown_row_unescaped_pipe_passes_gate`, which fired for real at the phase-93 IMPL): the row is written as a **comma list carrying no pipe character at all**, giving **NF=8 naive / NF=8 escape-aware**.

⚠️ **A POSITIVE CONTROL WAS RUN ON THE FIELD-COUNT MECHANISM ITSELF**, because a check that agrees with itself proves nothing. A deliberately-malformed draft row containing `(a|b)` reads **NF=9 under BOTH forms** — and **check (1) does NOT complain**, exactly as the memory warns. ⚠️ **The precise mechanism, newly stated: a pipe in the SUMMARY (field 6) leaves `$5` intact, so STATUS parsing is unaffected and check (1) stays happy while `NF` is wrong.** A pipe in the TITLE would corrupt `$5` and change the failure mode entirely. **The field count is the only instrument that sees either.**

Post-insertion results are recorded in `STATE.md` at this stage's close.

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 ⚠️ A MEASUREMENT WAS LOST FOR SEVENTEEN ROWS BECAUSE IT LANDED ONLY IN A BRAINSTORM

Phase 77 measured the reference's `ssl.connection_error` rule on 2026-07-26 and wrote it into its own BRAINSTORM. It was **never propagated to `DECISIONS.md` or `BEHAVIOR_CONTRACT.md`** — `SSL_ERROR_SSL` reads **0** in both. Seventeen rows later, phase 93 rejected the candidate on the ground that *"nobody has probed"* it, and wrote that into `ROADMAP.md` and `STATE.md`, where it became the inherited framing this stage started from.

**The general rule:** a measurement that governs a *deferral decision* must land in a *governing document*. A BRAINSTORM is a stage artifact; nothing downstream reads it unless told to. `reference_brainstorm_adjective_acquires_adr_authority` covers the case where BRAINSTORM prose gains authority it should not have — **this is the mirror failure: BRAINSTORM prose that FAILED to gain authority it should have had.**

### 9.2 ⚠️ AN "OPEN DENY-LIST" MAY BE A CLOSED ALLOW-LIST WEARING A DISGUISE

Phase 77's blocker — *"the deny-list is OPEN, the positive population cannot be typed"* — was correct on its facts and wrong in its conclusion. **When a classification's positive class is open and untypeable, check whether its COMPLEMENT is closed and typed.** Here it is, exactly. **Before accepting "this cannot be typed" as a blocker, ask which side of the partition was tried.**

### 9.3 ⚠️ A ROSTER-ORDINAL GREP THAT ANCHORS FORWARD SILENTLY RETURNS THE WRONG TAIL

Deriving this row's family ordinal, a pattern anchoring the ordinal word **immediately** before `Observability-family row` returned **TWENTY-FIRST (row 75)** as the tail. The true tail is **TWENTY-FOURTH (row 81)**, because rows 79-81 interpose a `§9` token. **The row would have been numbered TWENTY-SECOND and collided with row 79.** Locate the phrase and read **backwards**; never anchor forwards on a roster whose prose form has drifted. Same species as `reference_adr_doctrine_misattribution`.

### 9.4 ⚠️ A PROBE CLIENT CAN WITHHOLD THE VERY THING THE ARM EXISTS TO TEST

Arm (f2) read the wrong bucket under TLS 1.3 because Go **silently sent an empty certificate chain** rather than an unparseable leaf — it cannot select a signature algorithm for a cert it cannot parse. The arm was measuring client behaviour, not reference behaviour. Caught only by making the client **print what it sent**. `reference_go_client_cert_withholding` previously recorded this for *untrusted* certs; **it extends to UNPARSEABLE ones, and the SPEC's arms must force-send in both cases.**

### 9.5 ⚠️ TWO DOCUMENTS CAN CONTRADICT EACH OTHER FROM INSIDE ONE COMMIT

`git blame` puts the *"still blocked"* clause and the *"blocker RETIRED"* clause both at `c57b98b8`. **A consistency check that assumes contradictions arise from drift between stages will not look for this one.** When reconciling divergent claims, blame them **before** assuming a chronology.

### 9.6 ⚠️ "SMALLEST WINDOW" IS NOT "CLOSEST TO EMPTY"

Window `:239` has the fewest items (3) and the most blocked ones: two are gated on subsystems that do not exist and one depends on a **different** window. The item count is uncorrelated with the distance to empty, and the PICK MENU presents only the count. **Cross-window dependencies exist and are unrecorded.**

### 9.7 Method findings, each found by execution

- A ` + ` split of the window sentences is **wrong on 3 of 6 windows**, reporting `:239` as **1 item** when it has **3** (that sentence contains no ` + ` at all) and `:225` as **2** when it has **>= 6**. The project rule banning gates on a ` + ` split is now **empirically** confirmed.
- `ROADMAP.md:225` is **46 KB on one line** and contains **six** separate `remaining deferred … candidates` sentences, **not in chronological order** — an earlier one was retro-edited in place. **There is no single authoritative current deferred sentence for `:225`**, and any count taken from one is a lower bound. A further deferred candidate added at the phase-78 BRAINSTORM appears in **none** of them.
- A naive `'remaining deferred[^.]*\.'` extraction **truncates `:209`** at `` `BOOTSTRAP_PROMPT.` `` and silently drops its last two items.
- `internal/stats/name.go`'s *"Of the 30 entries"* is an **unguarded prose count inside the file it counts**; `git grep 'len(helpText)'` returns NONE.

### 9.8 Record defects found in passing — RECORDED, deliberately NOT fixed by this stage

- `phases/75-.../BRAINSTORM.md:150` cites `0111/driver.go:99` for `wantObservable`; it is **`:181`** at this tip.
- `phases/93-.../BRAINSTORM.md` cites `manager_test.go:4394-4402` for the five `outcomeOther` rows; the rows are at **`:4398-4402`** (count correct, anchor drifted).
- The phantom **"BEHAVIOR_CONTRACT B5/B6"** pointers in two landed production comments resolve to nothing — **there is no B-numbered scheme in that file at all**; the true referent is the subsection at `BEHAVIOR_CONTRACT.md:1965`. Already recorded at phase 75 and still unfixed; **this row's SPEC edits that subsection and should fix them then.**

---

## 10. What the SPEC owes

1. **Draft `ADR-0316`** (TAIL-derived; ⚠️ headings+1 reads **315**, a TAKEN id, because the space is sparse at the `0209` gap). §Context only, `PROPOSED` in the **house form** `^> \*\*STATUS: PROPOSED` — currently **0** in the file; the ADR-0231 **decoy** at `:14866` is **1** and must not be confused with it. ⚠️ **Do NOT quote a count for either form in prose the grep matches — the phase-93 SPEC falsified itself doing exactly that.**
2. **The predicate, written out** — the closed transport-exclusion set of §3.1, with the `tls.RecordHeaderError`-by-value footgun explicitly guarded, and **no message-text matching**.
3. **Correct all five divergent positions (§3.4)** — surgically. Delete only the *"still blocked"* clause from `BEHAVIOR_CONTRACT.md:1971`; **keep** the UNENUMERATED sentence beside it.
4. **Propagate the reference rule into a GOVERNING document**, per §9.1 — this is the row's most durable deliverable and the SPEC must not leave it in a phase directory.
5. **Design fixture `0120`** collision-free with respect to its arms, port from `10126-10129`, `BackendCount >= 1`, **+0 BackendKinds**; assert **both** directions (`connection_error` moved, `fail_verify_error` did **not**).
6. **Enumerate the full test-pin roster (§6.2) as an EDIT roster**, including the 4-site rename, and set-difference it against any byte-untouched gate (`reference_plan_schedules_edits_to_a_byte_gated_file`).
7. **NC every new pin, and neutralise rather than revert** — a NC that is a build break proves nothing (`7d`).
8. **Re-derive every count in §6.4 at the SPEC's own tip.** Every figure here is this stage's; `reference_cost_figure_measured_at_publishing_commit` went stale twice in one row before.

---

## 11. Probe hygiene

- Reference image digest **verified against `ENVOY_TARGET.md`** before any arm was trusted: `sha256:7edd5b0fd763…`.
- All probe containers named `p94probe-*` and **torn down BY NAME**. ⚠️ **Thirteen foreign containers were observed and LEFT UNTOUCHED** (`curl-world-*` x9 plus others); a sibling `curl-world` session owns them and a **sibling `envoy-rust` session was BUSY concurrently** — recorded per standing method note 5, since a port flake in this window would have a sibling explanation before it had a local one.
- Probe ports taken from the **`12000-19000`** ad-hoc band (13000/13001/13901), **below** `net.ipv4.ip_local_port_range` 32768-60999 and **outside** the differential harness reservation `20000..31007`. Availability checked with `ss -tan` (ALL states).
- **No repo file was written by any probe.** Tree verified clean at close: `git status --porcelain` shows only the pre-existing untracked `.claude/`.
- All four measurement agents committed **nothing** and proved their trees clean.
