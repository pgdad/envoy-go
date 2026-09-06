# Phase 95 — `tls-alpn-mismatch-fallback` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Subject SELF-PICKED** per the 2026-07-12 standing directive; no human pick was solicited and none was given. There was no banked mid-lifecycle work, PROVEN not assumed: all **126** rows read `done`, check (1) was SILENT with the denominator asserted at 126, NC-A returned the healthy one-line `NOT DONE: row 62` (§8).

**Subject.** When a downstream TLS client offers ALPN values that overlap NONE of the chain's `alpn_protocols`, reference Envoy **completes the handshake with no protocol selected** and serves the connection; envoy-go **aborts the handshake** with `no_application_protocol` and, since phase 94, **books `listener.<addr>.ssl.connection_error`** for it. Both the connection outcome and the stat diverge cross-side. The tree records this divergence as a MATCH (`internal/listener/tls_handshake_negative_test.go:117-118`: *"matching reference Envoy, which alerts no_application_protocol on ALPN mismatch"*). This row makes the TCP downstream path match the reference, flips the test that pins the wrong behaviour, and adds a discriminating differential arm.

**Family:** Core-TLS / downstream-listener **MAINTENANCE row claiming NO family ordinal** (§5).

---

## 0. What this stage refuted

Nine, each by execution at this tip (`7964f620`), not by reading:

1. ⚠️ **THE INHERITED CHARACTERISATION OF CANDIDATE 1 — "the cheapest, a seventh unit-table row" — IS WRONG IN KIND.** `next-prompt.txt`, `STATE.md:18`, `ROADMAP.md:156` and `DECISIONS.md:18862` all describe the ALPN arm as a coverage gap: an existing test increments `ssl.connection_error` unasserted. Measured on the pinned reference image (§2): **the reference does NOT abort on ALPN mismatch and books NOTHING under `connection_error`.** The unasserted increment is not a missing assertion; it is the stat-level shadow of a **production behaviour divergence**. Asserting `+1` in the unit table, as the banked candidate proposed, would have PINNED the divergence.
2. ⚠️ **THE LANDED CODE COMMENT CLAIMING PARITY IS FALSE.** `tls_handshake_negative_test.go:117-118` says reference Envoy *"alerts no_application_protocol on ALPN mismatch."* It does not (§2.2, arms 1 and 5). The test's name (`…_Aborts`) and its `t.Fatal` message at `:169` pin the wrong side. `reference_code_comment_not_evidence` fires.
3. ⚠️ **"NO MEASUREMENT EXISTS" WAS TRUE AND IS NOW FALSE.** A grep of all four phase-94 documents for `alpn` finds NO reference arm (the twelve-arm table at `phases/94-…/BRAINSTORM.md:74-90` has none); the only ALPN-vs-reference statement in the tree was the unmeasured comment above. This stage measured it, on both sides, with a positive control per side (§2).
4. ⚠️ **A FIRST PROTOTYPE PLACEMENT "AT THE RETURN" LANDED IN THE WRONG RETURN.** `internal/tls.NewDownstreamConfig` has **SIX** `return &DownstreamConfig{…}` sites at this tip (`config.go:129 :196 :221 :230 :249 :259`; the QUIC builder's is a seventh at `:291`). A patch inserted before the *first* landed inside the SDS validate-mode early return at `:128`, built, booted, and **changed nothing** — the mismatch arm still aborted. Placed at the single ENTRY point (immediately after `commonTLSContextToConfig` returns, `:53-56`) it works on every arm (§3.2, §6.1). **The SPEC must anchor the install on the entry, never on an exit.**
5. ⚠️ **THE MEMORY NOTE'S PORT-RACE ROSTER UNDER-COUNTS BY MORE THAN 2x.** `reference_driver_receiver_port_race_aborts_binary` says FOURTEEN fixtures via `ensureServer`. Re-derived here: **36** driver files carry both the `"127.0.0.1:0"` probe and a `0.0.0.0:%d` re-bind, and **30** carry the `panic(fmt.Sprintf("driver: start` string; the measurement agent read 37 / 31 (the variable is the UDP `ResolveUDPAddr` form and `0098`'s own panic text). Either way the candidate is **~36 files wide**, not 14 (§4.1).
6. ⚠️ **THE MEASUREMENT AGENT'S "NEXT-FREE ADR IS IN THE ADR-0292 AREA" IS WRONG BY 25.** Tail-derived at this tip: `## ADR-0316` is the tail, `^## ADR-0317` reads 0, so next-free is **ADR-0317**. An agent report is not evidence (`feedback_brief_citations_not_evidence`); this one was checked and corrected before use.
7. ⚠️ **AN OMITTED `clusters` KEY BOOT-REJECTS envoy-go** (`cluster manager: cluster: zero clusters in bootstrap`, exit 1). `next-prompt.txt` §12 records that the inherited `clusters: []` reject claim *"did not reproduce"*. These are DIFFERENT inputs — an empty list versus an absent key — and this stage measured only the absent-key form. Recorded as an observation, not as a reversal of the router's note: the two probes report on different things (`reference_contradicting_agents_find_the_variable`).
8. ⚠️ **THE "BANKED CANDIDATE" HAS A TEST-PIN SURFACE OF EXACTLY ONE TEST, MEASURED, NOT ASSUMED.** With the prototype in place the selector `ALPNNegotiationFailure|SSLConnectionErrorCounter|QUICListener_ALPNMismatch|TestNewManager_LiveHandshake` runs **11** tests and exactly **one** is red — `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` at `:169`. The six-row counter table, its stacked control, and the QUIC ALPN-mismatch test are all green (§6.2). `reference_measured_prototype_is_a_lower_bound` applies: this is a floor on TESTS; the doc-site roster in §3.4 is the wider surface.
9. ⚠️ **GO ALREADY HALF-IMPLEMENTS THE REFERENCE RULE.** `crypto/tls` `negotiateALPN` (`handshake_server.go:334-360`, go1.26.7) carries an `http11fallback` special case (`:342-358`): a client offering ONLY `http/1.1` to an `h2`-only server is let through with no protocol, citing Go issue 46310. So the subject ALREADY matches the reference for that one shape and diverges for every other non-overlapping offer. Any unit table this row builds must include that shape as a **discriminating negative control** — a fix that "works" only on `http/1.1`-vs-`h2` is the stdlib, not the row.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

**Make a TCP downstream chain with `alpn_protocols` complete the handshake with NO negotiated protocol when the client's ALPN offer overlaps none of them, exactly as the pinned reference does; flip the test that pins the abort; add a differential arm that reddens on the old behaviour.**

### 1.2 Why "smallest defensible" selects it — stated as a trade-off, not a ranking

| | this row | driver receiver port race | `0108` prose / `len(helpText)` / `0118:31` |
|---|---|---|---|
| production `.go` (measured) | **~25-31 lines, ONE file** (§6.1) | 0 | 0 |
| test files touched | 1 flip + `0120` (§6.2, §7) | **~36 driver files** | 1-2 prose sites |
| reference side | **MEASURED, both sides, six arms, this tip** | n/a (harness) | n/a |
| is it a live cross-side BEHAVIOUR divergence | **yes — connection accepted vs refused** | no (harness robustness) | no (prose) |
| the tree claims the opposite of the truth | **yes** (§0.2) | no | yes, but documentary |
| sits in a check-(2) window | no | no | no |
| new fixture forced | **no** — extends `0120` | no | no |
| new stat name / BackendKind / fuzzer / module | +0 / +0 / +0 / +0 | +0 | +0 |

**The decisive factor is the third and fourth rows.** This is the ONLY candidate on the board where the subject **refuses a connection the reference serves**, and where the tree's own comment says the two agree. A proxy that closes a TLS client the reference would have served is a production correctness defect of the same species as rows 91 and 93, not a coverage gap. It is also small: the working prototype is 31 added lines in one file.

⚠️ **THIS ROW IS NOT THE SMALLEST CANDIDATE BY LINE COUNT, AND THIS DOCUMENT DOES NOT CLAIM IT IS.** `0108`'s two false confessions are ~2 prose lines. A roller optimising purely for size would take them. This row rejects that trade because a prose correction that repairs nothing observable is the weaker use of a phase, and because rows 89-91 established the "repair a landed deliverable" reading of "defensible".

### 1.3 What this row does NOT buy — stated plainly

- ⚠️ **ZERO sentinel progress.** Its provenance is outside every check-(2) window (§8.5), so check (2) reads **SIX** before and must read **SIX** after. The row narrows nothing.
- It does NOT touch QUIC/H3. RFC 9001 §8.1 makes the `no_application_protocol` alert MANDATORY on QUIC, Go enforces it (`handshake_server.go:336-338`), and `TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives` (`quic_negative_test.go:90-113`) pins that the h3 listener refuses. The QUIC builder (`NewQUICDownstreamConfig`, `config.go:268`) is a separate function and stays byte-untouched.
- It does NOT touch the upstream/client side (`internal/cluster/dial_h2_test.go:281`'s "DialH2 — ALPN mismatch" is the CLIENT refusing a server that selected nothing; unrelated).
- It does NOT change the phase-94 predicate or the five-name `ssl.*` roster. After this row the ALPN arm simply never reaches `outcomeOther`, because the handshake succeeds.

---

## 2. The reference side, MEASURED — six arms on the live pinned image, and the same six on the subject

### 2.1 The rig and its controls, stated BEFORE the result

- Image `envoyproxy/envoy@sha256:7edd5b0fd763…`, digest verified against `docs/envoy-go/ENVOY_TARGET.md:4` before any arm was trusted. Container `p95probe-alpn`, ports `13100` (TLS listener) / `13101` (admin), taken from the ad-hoc `12000-19000` band after `ss -tan` showed both free (the sibling `curl-world` session holds `18080-19000`; seventeen of its containers were present throughout and never touched).
- Listener: one TLS chain, `alpn_protocols: ["h2", "http/1.1"]`, no client-cert requirement, HCM `codec_type: AUTO`, a `direct_response` 200 route. Certificates delivered `inline_string:` (the reference cannot read host `filename:` paths).
- Subject: `envoy-go` built from this tip into scratch (`-o`, no binary in the tree), same YAML with ports `13110/13111` plus a placeholder STATIC cluster (§0.7). Stats read from the subject admin `/stats`.
- Clients: `openssl s_client -alpn …` for the first pass, then a 40-line Go probe (`crypto/tls` `Dialer` with `NextProtos`, then a raw `GET / HTTP/1.1` and `http.ReadResponse`) so that the *served-or-not* question is answered, not just the handshake.
- **Positive control for `connection_error`, per side:** a TLS 1.0-only client (`-tls1`). **Positive controls for the handshake path:** overlapping offers `h2` and `http/1.1`.
- Stats snapshotted after EVERY arm; every figure below is a per-arm delta read from the live admin, not predicted.

### 2.2 The result

| arm (client offer) | reference: handshake / negotiated / HTTP | reference `connection_error` / `handshake` | subject AT THIS TIP: handshake / HTTP | subject `connection_error` / `handshake` |
|---|---|---|---|---|
| 1. `foo` only | **OK / "" / 200 over HTTP/1.1** | **+0 / +1** | **FAIL `remote error: tls: no application protocol`** | **+1 / +0** |
| 2. no ALPN extension | OK / "" / 200 | +0 / +1 | OK / 200 | +0 / +1 |
| 3. `h2` | OK / `h2` / (H1 bytes on an h2 conn — expected non-200 both sides) | +0 / +1 | OK / same | +0 / +1 |
| 4. `http/1.1` | OK / `http/1.1` / 200 | +0 / +1 | OK / 200 | +0 / +1 |
| 5. `foo,bar` | **OK / "" / 200** | **+0 / +1** | **FAIL, same alert** | **+1 / +0** |
| 6. TLS 1.0 client (positive control) | FAIL | **+1 / +0** | FAIL | **+1 / +0** |

Arms 2, 3, 4 and 6 agree cross-side to the counter. **Arms 1 and 5 diverge on the CONNECTION OUTCOME and on BOTH counters.** The reference's `openssl s_client` transcript for arm 1 reads `No ALPN negotiated` with a completed handshake; the subject's log reads `handshake: tls: client requested unsupported application protocols (["foo"])`.

A corroborating tell on both sides: `ssl.no_certificate` moved **+1 per completed handshake** (the listener requested no client cert), so the reference's arm-1 connection is booked as a *successful, cert-less handshake* — the phase-75 success-path annotation — not as any failure.

### 2.3 THE RULE, stated so an implementer can code it

**Reference behaviour (TCP downstream):** if the client offers NO ALPN, or offers an ALPN list with at least one overlap, negotiation proceeds as today. If the client offers a NON-EMPTY list with NO overlap, the reference **selects nothing and completes the handshake** (BoringSSL's `alpn_select_cb` returning `SSL_TLSEXT_ERR_NOACK`); the HCM's AUTO codec then dispatches on the bytes. **No alert. No `connection_error`. `ssl.handshake +1`.**

**Go's behaviour (`negotiateALPN`, `handshake_server.go:334-360`):** identical for the empty and overlapping cases; for the non-overlapping case it returns the `no_application_protocol` alert — EXCEPT the `http/1.1`-vs-`h2`-only special case, which it lets through (§0.9).

**The fix is therefore:** on the TCP downstream path, when `hi.SupportedProtos` is non-empty and overlaps none of `cfg.NextProtos`, hand `crypto/tls` a clone of the config with `NextProtos = nil`, via `GetConfigForClient`. The stdlib then takes its own "server has no protocols" branch and completes the handshake with `NegotiatedProtocol == ""`. The prototype in §6.1 is exactly that and matches the reference on all six arms.

### 2.4 Arms that were NOT run — recorded, not glossed

- The reference's QUIC listener on ALPN mismatch — not re-measured; RFC 9001 §8.1 and the landed h3 test are relied on, and the row does not touch QUIC.
- `codec_type: HTTP2` (non-AUTO) with a no-ALPN connection carrying HTTP/1.1 bytes — both sides should fail at the codec, not the handshake; out of scope, unmeasured.
- An SDS-bound chain with `alpn_protocols` — no fixture combines them (`0103`/`0108`-`0111` carry no `alpn_protocols`); §10 item 6 asks the SPEC to reason about `Clone()` under a live provider.

---

## 3. The mechanism, stated precisely

### 3.1 Where the divergence is produced

`internal/listener/manager.go:1337` — `tlsConn := stdtls.Server(pkConn, selected.tlsCfg)` then `HandshakeContext`. `selected.tlsCfg` is built ONCE per chain at manager construction (`manager.go:631` via `internaltls.NewDownstreamConfig` for TCP chains; `:617` via `NewQUICDownstreamConfig` for QUIC; `:713` for the `default_filter_chain`). `alpn_protocols` reaches `cfg.NextProtos` at `internal/tls/config.go:580`, inside `commonTLSContextToConfig`, which BOTH builders share. The handshake failure then falls into `outcomeOther` and, since phase 94, past `isTransportHandshakeErr` (the error is a bare `*fmt.wrapError`; none of the five closed transport members match) into `sslConnectionError.Inc()` at `manager.go:1352-1359`.

### 3.2 Where the change goes — every anchor SYMBOL-VERIFIED at this tip

- **`internal/tls/config.go`, `NewDownstreamConfig`, immediately after `cfg, err := commonTLSContextToConfig(…, "downstream", provider)` returns (`:53-56`).** This is the ONE point every TCP downstream chain passes through and the QUIC builder does not. ⚠️ **NOT at any `return`** — see §0.4.
- A new unexported helper (prototype name `installALPNMismatchFallback(cfg *stdtls.Config)`): no-op when `len(cfg.NextProtos) == 0` or `cfg.GetConfigForClient != nil` (there is no non-comment `GetConfigForClient` anywhere under `internal/` at this tip — `manager.go:138` and `config.go:25` / `doc.go:5` are comments describing a phase-03 design that no longer holds a callback, a documentary drift the SPEC should correct in passing).
- The callback: return `(nil, nil)` when the offer is empty or overlaps; otherwise `cfg.Clone()` with `NextProtos = nil`.

### 3.3 Hazards the SPEC must carry

- **`Clone()` per mismatched connection.** Cheap, and only on the mismatch path; but the SPEC should state it and decide whether to pre-build the alternate config once per chain (it can: `NextProtos` is immutable after build) rather than per handshake. The prototype clones per call.
- **Interplay with SDS.** `cfg.Certificates` is populated by the provider at build time and the row's `Clone()` copies the slice header. If a future SDS rotation mutates `cfg` in place, the alternate config would need rebuilding; at this tip no rotation path exists (SDS is initial-fetch, `reference_sds_fetchfail_posture_init_hold`). Record, do not solve.
- **Go's `http11fallback` special case** stays as a stdlib behaviour and coincides with the reference; the unit table must include it as a negative control (§0.9).
- **The four-outcome taxonomy is untouched**; after the row the ALPN arm classifies as `outcomeOK`.

### 3.4 THE LANDED POSITIONS THAT MUST BE RECONCILED — enumerated by grepping the literal claim text outward

| site | what it says now | what the row does |
|---|---|---|
| `internal/listener/tls_handshake_negative_test.go:113-120` (doc comment) | *"must ABORT … matching reference Envoy, which alerts no_application_protocol"* | rewrite: must COMPLETE with no protocol, matching the reference |
| `…tls_handshake_negative_test.go:121-170` (`…_Aborts`) | `t.Fatal` if the handshake succeeds (`:167-170`) | rename + invert; assert `NegotiatedProtocol == ""`, `ssl.connection_error == 0`, `ssl.handshake == 2` after hoisting the inline `stats.NewRegistry()` at `:130` into a local |
| `internal/listener/manager_test.go:5209-5246` (six-row counter table) | no ALPN row | add a SEVENTH row with an ALPN-configured listener, **want `connection_error` +0 / `handshake` +1**, plus the `http/1.1`-vs-`h2` control row |
| `test/fixtures/0120-tls-connection-error/driver/driver.go:73-92` | *"THESE ARE ARM ARITHMETIC. Adding a sixth arm INVALIDATES them."* `wantConnectionError = 3`, `wantHandshake = 1` | add arm (vi); re-measure both sides; `wantHandshake` 1 -> 2 (prediction — MEASURE) |
| `test/fixtures/0120-…/envoy.yaml` + `envoy-go.yaml` | no `alpn_protocols` | add `alpn_protocols` to the `l_conn_err` chain on BOTH sides |
| `test/fixtures/0120-…/expectations.yaml:63-72, :118-119`, `README.md` | five-arm table, pins | six-arm table, pins |
| `DECISIONS.md:18862` (ADR-0316 §Consequences (vi)) | *"the `Inc` was firing unasserted"* — framed as a coverage gap | ADR-0317 §Context corrects the framing; ADR-0316 text is retained verbatim (append-only history) |
| `STATE.md:18`, `ROADMAP.md:156` | "cheapest follow-on … unit table" | superseded by this row's registration; not edited (append-only) |
| memory `reference_driver_receiver_port_race_aborts_binary` | FOURTEEN fixtures | corrected to the measured ~36 at this close (memory, not repo) |

⚠️ `BEHAVIOR_CONTRACT.md` carries **NO** claim about ALPN-mismatch handling at this tip — `:1944` says the negotiated value *"is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection."* **This row is that later phase**; the SPEC extends `:1944` (WITHIN-LINE — it is one paragraph line) rather than adding a departure, because after the row there is none.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The driver-owned receiver port race — **the runner-up; REJECTED for size, and its cost is now MEASURED**

Real, twice-observed, and it cost phase 94 a full 400-second differential run. Mechanism confirmed by reading: every affected driver probes `net.Listen("tcp", "127.0.0.1:0")`, reads the port, `Close()`s it, and later re-binds on `0.0.0.0:%d` — a loopback probe for a wildcard bind, inside `net.ipv4.ip_local_port_range` (`32768 60999` on this box), with no retry (`harness_test.go:240-244` predicts exactly this: *"A start-retry loop … would be the second line of defense if this ever recurs"*). The harness's own `freeTCPPort` (`harness_test.go:246`, band `11000..14999`) and `freeTCPPortBlock` (`:292-296`, `20000..31007`) were fixed for precisely these two properties and the drivers never were.

**Cost, re-derived:** **36** driver files with probe+rebind (30 of them `panic` the binary; the four `fmt.Errorf` variants in `0021/0022/0023/0032` degrade to one failing subtest). Two fix shapes: (A) one exported banded allocator in `test/differential/fixture/` + a ~2-line swap per driver; (B) bind `0.0.0.0:0` in each `ensureServer`/`mustStartReceiver` and read the port back from the helper's `Addr()` (every helper already exposes it; every template already substitutes `{{.XPort}}`). Either shape is **~36 files**, an IMPL that is wide rather than deep, and its gate is a probabilistic recurrence rate, not a deterministic red arm. **Banked with its measured cost.** It remains the most defensible NEXT pick if a future row's differential aborts again.

### 4.2 `0108`'s two *"emits NO `ssl.*` stats whatsoever"* confessions — **REJECTED as a row; ~2 prose lines**

False since phase 74; `0110`/`0111` already retired the wording. Fold into whichever row next edits `0108`.

### 4.3 `len(helpText)` guard + `internal/stats/name.go`'s two ungated prose counts — **REJECTED as a row**

A ~5-line test plus two comment edits. No behaviour. Fold into the next row that touches `name.go`.

### 4.4 `0118/driver/driver.go:31`'s falsified *"TLS/SDS band"* — **REJECTED; one comment**

### 4.5 `0061-lb-ring-hash`'s σ-margin second occurrence — **REJECTED again; probabilistic gate**

Row 76 already derived the margin once; a second occurrence needs a recurrence count before it can be sized. No new data since phase 94.

### 4.6 The carried set — 1xx interim responses, the H/1 no-`Host` divergence, the pooled-upstream-lifetime defect, the ten remaining fixed `ssl.*` names and four dynamic families

Unchanged from the phase-94 BRAINSTORM §4; no cost was re-derived here because none moved and none is smaller than §1.2's table. The `ssl.*` names stay blocked on NAMING (the stat-name charset bans the hyphen).

### 4.7 The six windows as a pool — **REJECTED, unchanged**

Phase 94 §4.9 stands: no single row empties any window, so the pool offers no sentinel-progress candidate, and every candidate in it is larger than this row.

---

## 5. Family attribution

**Core-TLS / downstream-listener MAINTENANCE row claiming NO family ordinal.** A maintenance row repairs a landed deliverable (here the phase-03/07.2 ALPN pass-through, `BEHAVIOR_CONTRACT.md:1944`, and the phase-94 counter's semantics) and does not extend a charter — the row-85 through row-91 precedent, interrupted by rows 92-94 which were chartered rows. The Observability ordinal chain is NOT extended: this row lands +0 stat names. The HTTP/3, gRPC, xDS, Runtime, Observability and Operational-tooling charters are untouched, and check (2) must therefore still read **SIX** at close.

---

## 6. The cost FLOOR — a built, run, and reverted prototype, explicitly a LOWER BOUND

### 6.1 Production: ONE file, `+31 / -0`, and it matches the reference on every arm

Prototype patched into `internal/tls/config.go` in this stage's worktree, built with `-o` into scratch, booted on the §2.1 subject YAML, driven by the same six arms:

| arm | prototype `connection_error` / `handshake` | reference (§2.2) |
|---|---|---|
| `foo` | **+0 / +1**, HTTP 200 | +0 / +1 |
| none | +0 / +1 | +0 / +1 |
| `h2` | +0 / +1, negotiated `h2` | same |
| `http/1.1` | +0 / +1, negotiated `http/1.1` | same |
| `foo,bar` | **+0 / +1**, HTTP 200 | +0 / +1 |
| TLS 1.0 (control) | **+1 / +0** | +1 / +0 |

`gofmt -l` printed nothing; `go vet ./internal/tls/` clean; `go test ./internal/tls/` ok. The subject log contains **0** `unsupported application protocols` lines under the prototype versus **2** at this tip. **Reverted under a `sha256sum -c` guard; both trees proven clean** (§11). The row's landed form should be a few lines shorter than 31 once the prototype's self-description is replaced by a doc comment citing ADR-0317.

### 6.2 The test-pin surface — measured with the prototype in place

`go test ./internal/listener/ -run 'ALPNNegotiationFailure|SSLConnectionErrorCounter|QUICListener_ALPNMismatch|TestNewManager_LiveHandshake' -count=1 -v` — `RUN=11`, `RC=1`, exactly ONE red: `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` at `tls_handshake_negative_test.go:169` (*"SUCCEEDED; expected an aborted handshake"*). Green: the six-row counter table, its stacked control, the QUIC mismatch test, and the SNI-abort sibling. **That red is the row's built-in negative control for the flip**: the renamed test must pass on the fix and fail on the tip.

⚠️ A FULL `./internal/...` sweep was NOT run under the prototype; §6.2 is a selector run and therefore a LOWER BOUND on red tests (`reference_measured_prototype_is_a_lower_bound`). The SPEC's PLAN must run the package sweep.

### 6.3 The fixture: +0 — `0120` gains a sixth arm

`0120-tls-connection-error` is a `tcp_proxy` echo listener with `require_client_certificate: true` and a `tls_params` block, port `10126`, five arms in ONE `Drive` pair, all discrimination in `AssertStats` keyed on metric name. A sixth arm **(vi) valid client cert + ALPN offer `["bogus/9"]`** against a chain that gains `alpn_protocols: ["h2", "http/1.1"]` on BOTH sides is expected to read **`connection_error +0`, `handshake +1`** on both sides after the fix, and **`+1 / +0`** on the subject at this tip — a deterministic red arm. The five existing arms are predicted unaffected by adding `alpn_protocols` (none of them offers ALPN), ⚠️ **but the driver says its pins are ARM ARITHMETIC and must be RE-MEASURED per arm on both sides, not predicted.** No new directory, no new BackendKind, no new port, so the fixture-set reconciliation gates are untouched.

### 6.4 Anticipated counts — every axis re-derived at this tip

| axis | now | after this row |
|---|---|---|
| ROADMAP data rows / `want` | **126** | **127** (this BRAINSTORM's own insertion) |
| ROADMAP lines | **244** | **245** |
| stat surface | *(contested — DELTA ONLY, no absolute)* | **+0** |
| fixtures | **122** (tail `0120`) | **122** — `0120` extended, `0121` stays free |
| BackendKind tail | **38** | **38** |
| fuzzers | **56 targets / 48 files** | unchanged — no new config field |
| `go.mod` requires | **67** | unchanged |
| phase dirs | **135** | **136** |
| `go list ./...` packages | **237** (235 excluding the two Docker drivers) | unchanged |
| next-free ADR | **`ADR-0317`** (TAIL-derived; `^## ADR-0317` reads 0) | ADR-0317 drafted at the SPEC |
| `DECISIONS.md` lines / `^---$` / `^## ADR-` / bare `^## ` | **18872 / 216 / 315 / 323** | SPEC moves lines and the two heading counts by +1; `^---$` STAYS 216 |
| house `PROPOSED` guard `^> \*\*STATUS: PROPOSED` | **0 (DISARMED)** | SPEC re-arms to 1; ADR-0231 decoy at `:14866` stays 1 |

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

Exactly ONE fixture configures downstream `alpn_protocols` (`0004-h2-routing`) and it drives the overlapping success path only. No fixture, unit test, or contract line exercises a non-overlapping offer against the reference. The divergence has been live since phase 03 and invisible because nothing looked.

### 7.2 The gate this row adds

The `0120` arm (vi) of §6.3, asserted on BOTH sides, keyed on metric NAME (`envoy_listener_address` diverges cross-side: ref `0.0.0.0_10126`, subj `___10126`), with the tcp echo round-trip as the *served-or-not* witness. Over-firing control: 2x arm (vi) -> `handshake +2`, `connection_error +0`.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN INSERTION

### 8.1 PRE-INSERTION, at `7964f620`

- **(1)** SILENT (`want=126`)
- **(2)** SIX: `204 210 216 226 232 240`, per-line md5 (trailing newline INCLUDED) `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 242e53c6f7a3 b2680e6f4fbf 6caa1c3ce0e7` — **byte-identical to the phase-94 IMPL close**, proving no candidate line was tidied between rows
- **(3)** SILENT
- `stop` ABSENT at the git root

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

- **NC-A** (row 62 doctored to `in-progress`, `NC LANDED? [ in-progress ]` inspected first, `want=126`): **ONE** line, `NOT DONE: row 62`
- **NC-B** (`want=125` on the real file): **ONE** line, `GATE FAIL: examined 126 data rows, expected 125`
- **NC-C** (`gRPC-family row` doctored out; residual **0**): **FIRED**, `NEVER OPENED: gRPC`
- **NC-D** (`-family row` with `--`): occurrences **96**, lines **68**

### 8.3 The check-(2) positive control

Both phrases substituted in a scratch copy: check (2) reads **0** there, with **6** substitutions asserted. The SIX on the real file is real signal, not a stuck gate.

### 8.4 POST-INSERTION

Row 95 is an **ADD**, so `want` **126 -> 127** and `ROADMAP.md` **244 -> 245**; the six windows shift by one line each. The new row is a comma-and-semicolon narrative carrying **no pipe character** (delimiters only: **7**), so **NF=8 under BOTH the naive and the escape-aware form**, counted BEFORE and AFTER installing. Post-insertion actual output is recorded in `STATE.md` §Current at this stage's close, not here, because this file's line count is quoted in the row and a figure recorded in the file it measures self-increments.

### 8.5 Provenance of the pick against the six windows — and the instrument NC'd

Per-window fixed-string counts at this tip: `ALPN` / `alpn` / `port race` / `ephemeral` / `ip_local_port_range` / `0108` / `helpText` / `ring-hash` / `TLS/SDS band` — **0 in all six**. Tokens that DID hit, DISCLOSED: `receiver` **1** in `:226` (the landed `0092-stats-sink-statsd` receiver narrative, not a candidate) · `outcomeOther` **1** and `connection_error` **2** in `:226` (the phase-94 CONSUMED clause). The instrument is therefore live — it finds tokens known to be present — and the pick's provenance is **outside every window**: it comes from row 94's banked follow-on list and the reference measurement of §2, not from a deferred clause.

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 ⚠️ A "CHEAP ASSERTION GAP" CAN BE A BEHAVIOUR DIVERGENCE WEARING A DISGUISE

An unasserted counter increment is only a coverage gap if the increment is CORRECT. The banked candidate would have asserted `+1` and pinned a divergence as parity. **Before asserting any cross-side stat, measure the reference for that arm** — the phase-94 rule (*"the reference side is not a doc claim"*) applies to every arm individually, not to the counter as a whole.

### 9.2 ⚠️ "AT THE RETURN" IS NOT A PLACEMENT

A function with six exits has no "the return". The prototype's first placement compiled, built, booted, served, and silently did nothing. **Anchor installs on the ENTRY of the code path that must be covered, and prove the install by an arm that reddens without it** (§6.1's arm 1 is that arm).

### 9.3 ⚠️ THE STDLIB CAN COINCIDENTALLY MATCH THE REFERENCE ON ONE SHAPE

Go's `http11fallback` makes exactly one non-overlapping shape succeed today. A unit table with only that shape would read green at the tip and prove nothing (`reference_coincidental_method_agreement`). Use `bogus/9`-class offers as the positive rows and the `http/1.1`-vs-`h2` shape as a control row.

### 9.4 Method findings, each found by execution

- A memory note's roster (14) and a fresh enumeration (36) differed by 2.5x on the same defect; the note enumerated one helper name (`ensureServer`) and the defect spans four idioms. **Enumerate by the DEFECT'S SHAPE (probe + rebind), not by one helper's name.**
- A measurement agent stated a next-free ADR 25 ids stale. Every incidental figure in an agent report was re-derived before use (`feedback_brief_citations_not_evidence`).
- `openssl s_client` alone answers "did the handshake complete"; it does NOT answer "was the connection served". The Go probe's `GET` after the handshake is what turned "no alert" into "200 OK over HTTP/1.1".
- An `inline_string:` PEM indented SHALLOWER than its key ends the block scalar early and the reference reports `yaml-cpp: … illegal map value` at a line inside the PEM. Indent the PEM one level DEEPER than `inline_string:`.
- `pgrep -la a b` errors (*"only one pattern can be provided"*); pattern-alternate with `-f 'a|b'` or run two calls. The kill itself was by PID captured from the launch, never by pattern.

### 9.5 Record defects found in passing — RECORDED, deliberately NOT fixed by this stage

- `internal/listener/manager.go:138`, `internal/tls/config.go:25`, `internal/tls/doc.go:5` all describe a `GetConfigForClient` SNI-dispatch callback; **no non-comment `GetConfigForClient` exists under `internal/` at this tip** (chain selection is `listenerfilter.SelectChain` on pre-parsed ClientHello inputs, `manager.go:1319`). The SPEC, which installs the FIRST real `GetConfigForClient`, should correct the three comments.
- `harness_test.go:206-208` enumerates every claimed port range (`10000..10447`, `15000..15011`, `18001..18007`, `20000..31007`, `11000..14999`); the driver-owned receiver ports appear in none of them, because there is no such band — §4.1's defect stated in the harness's own words.

---

## 10. What the SPEC owes

1. **Draft `ADR-0317`** (TAIL-derived; ⚠️ headings+1 reads **316**, a TAKEN id, because the space is sparse at the `0209` gap). §Context only, `PROPOSED` in the **house form** `^> \*\*STATUS: PROPOSED` — currently **0** in the file; the ADR-0231 **decoy** at `:14866` is **1** and must not be confused with it. ⚠️ **Do NOT quote a count for either form in prose the grep matches.**
2. **The rule of §2.3, written as code**: entry-point install in `NewDownstreamConfig` (§3.2), TCP-only, no-op on empty or overlapping offers, `Clone()` with `NextProtos = nil` otherwise; decide per-chain pre-build vs per-handshake clone (§3.3) and say why.
3. **Flip `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts`** — rename, invert, hoist the registry, assert `NegotiatedProtocol == ""`, `ssl.connection_error == 0`, `ssl.handshake == 2` — and add the SEVENTH and the control rows to `TestServeConnection_SSLConnectionErrorCounter` via an ALPN-capable listener helper (none exists; `mkDownstreamTSInlineALPN` at `tls_handshake_negative_test.go:88` is reusable, same package).
4. **Design `0120` arm (vi)** per §6.3 and §7.2; **re-measure ALL SIX arms per side** and rewrite the arm table at `driver.go:73-92` from measurement; `+0` fixtures, `+0` BackendKinds, port unchanged.
5. **Extend `BEHAVIOR_CONTRACT.md:1944` WITHIN-LINE** (it is one paragraph line) with the measured rule and the fixture opt-in it promised; add the stat-surface ledger a `+0` entry only if the ledger's convention requires one (check its tail).
6. **Reason about SDS interplay** (§3.3) in the ADR — record, do not solve.
7. **Enumerate the full edit roster of §3.4 as an EDIT roster**, set-differenced against any byte-untouched gate (`reference_plan_schedules_edits_to_a_byte_gated_file`); correct the three `GetConfigForClient` comments of §9.5.
8. **NC every new pin, and neutralise rather than revert**; the tip itself is the NC for the flipped test (§6.2).
9. **Re-derive every count in §6.4 at the SPEC's own tip.**

---

## 11. Probe hygiene

- Reference image digest **verified against `ENVOY_TARGET.md:4`** before any arm was trusted: `sha256:7edd5b0fd763…`.
- One probe container, `p95probe-alpn`, **torn down BY NAME**; **seventeen foreign `curl-world-*` containers were observed and LEFT UNTOUCHED** (a sibling session owns them). No `reaper_*` container was created or touched.
- Probe ports `13100/13101` (reference) and `13110/13111` (subject), from the `12000-19000` ad-hoc band, **below** `net.ipv4.ip_local_port_range` (`32768 60999`, read from `sysctl`) and **outside** the harness reservations; availability checked with `ss -tan` (ALL states) before use and release confirmed after.
- Subject processes were launched from scratch binaries (`-o`, never in the tree) and killed **by the PID captured at launch**, never by pattern.
- The prototype was applied ONLY in this stage's worktree, measured by `git diff --numstat` (**31 / 0**, one file), and **reverted under `sha256sum -c`**. `git status --porcelain --untracked-files=all` on the worktree: EMPTY; on the main root: only the pre-existing untracked `.claude/`.
- Both measurement agents committed **nothing**; their figures were re-derived here before use (§0.5, §0.6).
