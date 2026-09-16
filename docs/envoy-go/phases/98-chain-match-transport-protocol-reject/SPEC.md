# Phase 98 — `chain-match-transport-protocol-reject` — SPEC

**Stage:** SPEC (lifecycle **1 -> 2**). Worktree off master `45728a81`, branch `phase-98-spec`.
**Governs:** `BRAINSTORM.md` (527 lines) — **read for evidence, re-derived before trusted.**

**The decision in one paragraph.** The row repairs **two** divergences on ONE `filter_chain_match`
dimension, not one. (1) `parseChainSpec` boot-rejects any `transport_protocol` outside
`{"", "tls", "raw_buffer", "quic"}`; the reference accepts any string, on TCP **and** on QUIC, and the
chain simply never matches. (2) **NEW AT THIS STAGE:** on a TCP connection that no listener filter
classified, the reference stamps the detected transport protocol `raw_buffer`, so a
`transport_protocol: raw_buffer` chain is served; envoy-go leaves the input `""`, so that chain is
**never eligible** and the connection silently falls to the default chain (or closes). The second was
found because the BRAINSTORM's chartered matched-positive arm could not be built honestly without it.
**The repair is one production file, `internal/listener/manager.go`, `+4 / -9` by `git diff --numstat`,
built, run against twelve arms on the subject and reverted, AGREEING with the reference on every TCP arm
measured.** `ADR-0320` §Context is drafted; fixture `0123-listener-transport-protocol` is chartered.

---

## 0. What this stage refuted — by execution

Every item below was produced by running something. **Two of the refutations came from this stage's own
agents**, and one of those two refuted a claim in the brief the controller gave it.

### 0.1 🔴 THE BRAINSTORM's CHARTERED POSITIVE ARM IS FALSE ON THE SUBJECT — A SECOND DIVERGENCE

`BRAINSTORM.md` §7.2 and §10 item 4 charter *"a `transport_protocol: "raw_buffer"` arm on a
byte-identical listener that **must** be served by the indexed chain."* On a listener with **no**
`listener_filters`, that arm is:

| arm (plaintext HTTP/1.1, default chain present) | reference | subject, un-fixed tip | subject, BRAINSTORM prototype (`+1/-9`) |
|---|---|---|---|
| T0 control — empty match, no listener filters | `INDEXED` | `INDEXED` | `INDEXED` |
| **T1 — `raw_buffer`, NO listener filters** | **`INDEXED`** | **`DEFAULT`** | **`DEFAULT`** |
| T2 — `raw_buffer`, WITH `tls_inspector` | `INDEXED` | `INDEXED` | `INDEXED` |
| T3 — `tls`, no listener filters (matched negative of T1) | `DEFAULT` | `DEFAULT` | `DEFAULT` |

**T1 against T3 is the proof, on the reference**: two listeners differing by one string, one served and one
not, so the dimension is enforced there and the input really is `raw_buffer`. **On the subject T1 reads
`DEFAULT` both before and after the BRAINSTORM's prototype.** Mechanism, read by symbol:
`serveConnection` builds `listenerfilter.ChainMatchInputs` with no `TransportProtocol`; only
`tls_inspector` writes that field; nothing defaults it after the pipeline. `matches()` then compares
`"raw_buffer" != ""` and excludes the chain. The reference's corresponding behaviour was **measured**, not
recalled — the subject agent's recollection of Envoy's `ActiveTcpSocket::newConnection` was a hypothesis
and is recorded as such; T1/T3 on the pinned image is the evidence.

⇒ **The fixture the BRAINSTORM chartered would have been RED on a correctly-repaired-by-its-own-plan
subject — or, had someone "fixed" it by adding `tls_inspector` to the listener, GREEN over a live divergence
in the exact dimension the fixture claims to cover.** That second outcome is the dangerous one.

### 0.2 🔴 *"THE RUNTIME IS ALREADY PARITY-CORRECT; ONLY THE PARSE-TIME GATE IS NOT"* — HALF TRUE

`BRAINSTORM.md` §2.3 and the router both carry it. **What survives:** `matches()` is correct — exact,
case-sensitive `!=`, empty chain value means unspecified. **What dies:** the runtime **input** is not
parity-correct on TCP. The BRAINSTORM's claim was about the comparison and was stated about the runtime.

### 0.3 🔴 THE BRAINSTORM's POSITIVE CONTROL NEVER DROVE THE SUBJECT

`BRAINSTORM.md` §2.2's `POS CONTROL` row reads, in the subject column, **`build OK`**. The reference column
drove a request; the subject column only built a manager. **§0.1's divergence sits in exactly that cell.**
A build is not a served request (method note 7h's class, one level up: a probe that stops before the
property under test).

### 0.4 ⚠️ THE COST FLOOR `+1 / -6` IS NOT THE MINIMAL HONEST EDIT, AND NOT THE PARITY-CORRECT ONE

Measured by `git diff --numstat` (lines, not `--stat`): the reject-lift alone is **`+1 / -9`** — the
three-line comment above the switch narrates the enum gate and dies with it. The parity-correct shape
(§4) is **`+4 / -9`**. `reference_measured_prototype_is_a_lower_bound` fires for the **nineteenth**
consecutive row — this time on shape, not only on size.

### 0.5 ⚠️ THE ROUTER's *"STAMPED QUIC ALPN CONSTANT LIVES IN NO PRODUCTION GO FILE"* IS FALSE AS STATED

`git grep -n '"h3"' -- '*.go' ':!*_test.go'` returns `internal/listener/quic.go:128`,
`quicApplicationProtocol = "h3"`, beside `quicTransportProtocol = "quic"` at `:127`. The banked concern
(a listener advertising a different ALPN is evaluated against a hardcoded one) may survive; the sentence
carrying it does not. **Not repaired here** — recorded so the next reader does not search for a constant
that is in plain sight.

### 0.6 ⚠️ `BRAINSTORM.md` CONTRADICTS ITSELF ON THE QUIC ARM

§2.4 says *"`transport_protocol` on a QUIC listener — phase 97 §2 covers it; not re-run."* §10 item 1 says
the SPEC owes that measurement because phase 97's is *"adjacent evidence, not this arm."* **§10 is right**:
phase 97 measured `"quic"` and `"tls"` only. A bogus value and `raw_buffer` on QUIC had never been run —
they are now (§2.2).

### 0.7 ⚠️ THE QUIC NARRATION IS FOUR SITES, NOT ONE, AND IT CARRIES A STALE ANCHOR TWICE

The BRAINSTORM names `quic_test.go:691`. The enum-gate claim lives at **four** `quic_test.go` sites —
`:690-694` (comment), `:702` (`t.Fatalf` text), `:768-770` (comment), `:779` (`t.Fatalf` text) — and both
comments cite `manager.go:985-991` for a switch that sits at **`:994-999`** at this tip. §6 has the union.

### 0.8 ⚠️ THE OBVIOUS SHAPE OF THE RE-POINTED TEST IS NOT CONSTRUCTIBLE — MEASURED BY THIS STAGE's AGENT

The brief suggested a `raw_buffer` or empty-match sibling chain. **Both fail as gates**, and the agent
proved it by running the negative controls rather than reasoning: with a `raw_buffer` sibling, an NC that
drops every parsed value to `""` leaves **two empty chains**, and the boot rejects on the identical-spec /
catch-all check, so property (a) masks property (b); with the dimension deleted from `matches()`, two
`transport_protocol`-bearing chains **tie** and `SelectChain` returns an ambiguity instead of naming the
wrong chain. The working sibling uses `source_type: SAME_IP_OR_LOOPBACK`, which ranks **below**
`transport_protocol` (§5.1).

### 0.9 ⚠️ A FLAKE OUTSIDE EVERY REGISTER FIRED TWICE IN THIS SESSION

`TestEnvoyGoBinary_TwoListenerCutover` (`cmd/envoy-go`) failed **twice at two different tips**: at the
un-fixed tip in the subject agent's run (`bind 127.0.0.1:36601: address already in use`) and under the
controller's combined prototype (`bind 127.0.0.1:33215: address already in use`). Both ports lie inside
`net.ipv4.ip_local_port_range` `32768-60999`; three isolated reruns at the prototype read `ok`. **It is not
attributable to this row** — it fired at the un-fixed tip too — and **a green rerun clears nothing.** It is
absent from method note 4's list. Recorded here so the IMPL does not rediscover it as a regression.

### 0.10 ⚠️ THIS STAGE's OWN FIRST DRAFT OF THE NC ROSTER WAS WRONG — CAUGHT BEFORE PUBLISHING

The first draft of §11 said an unconditional `"tls"` stamp reddens (s1) and (s3) but not (s2), and that a
plaintext `tls_inspector` arm proves the stamp does not overwrite a classified input. **Both were false on
tracing the mechanism:** a `"tls"` stamp makes the `tls` chain of (s2) eligible, so (s2) reddens; and a
plaintext arm cannot detect an overwrite, because the inspector already wrote the same `raw_buffer` the
stamp would. §5.2 (s3) was re-designed around a TLS client and §11 gained row 5b. **Method note 7d, fired
against this document's author: name the mechanism that carries a mutation to a failure BEFORE writing
the row.**

---

## 1. Scope, restated as a decision

**IN:** (a) accept any `transport_protocol` string at parse, storing it byte-exactly; (b) on the **TCP**
path, default the detected transport protocol to `raw_buffer` when the listener-filter pipeline left it
empty, before chain selection; (c) one re-pointed unit test, one new stamp-site unit test group, one
differential fixture; (d) `ADR-0320`; (e) the prose reconciliation of §6.

**OUT, and why:**
- **QUIC input stamping** — unchanged. The reference stamps `quic` on QUIC and treats `raw_buffer` there as
  non-matching (§2.2 Q3); envoy-go already stamps `quic` and nothing else. The default applies to TCP only.
- **Case folding** — none. The reference is **case-SENSITIVE** on this dimension (§2.1 T5), unlike SNI
  (`BRAINSTORM.md` §0.8). ⚠️ **Agreeing case semantics on one dimension do not generalise to another.**
- **SNI longest-suffix, SNI case, `server_names` partial wildcards, `catchAllCount`** — banked, untouched.
- **`no_filter_chain_match`** — envoy-go emits no such counter; the fixture carries a default chain on every
  listener so no arm depends on it.

**Why widen instead of banking the second divergence.** It is the same dimension, the same function's
caller, `+3` lines, measured on both sides, and — decisively — the row's own gate cannot be built honestly
without it (§0.1). Banking it would mean chartering a fixture that either fails against the row's own
repair or steps around a known divergence with a listener filter. **Size was not the tiebreaker; the
gate's honesty was.** The slug stays `chain-match-transport-protocol-reject` — renaming it would break every
slug-anchored locate form — and the IMPL's row flip must name the fold-in.

---

## 2. The reference, MEASURED — both transports, controls FIRST

**Rig.** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`, run BY
DIGEST after verifying `ENVOY_TARGET.md` lines 3-4. Admin `16000/tcp`, TCP listener `16001/tcp`, QUIC
listener `16002/udp`, censused free with `ss -tan` and `ss -uan` over all states. `-p` publishing, one
container at a time, prefix `p98spec-ref-`, removed BY NAME. Readiness on `/ready` = `LIVE`. Every arm
**validated rc=0** (`configuration '/cfg.yaml' OK`) and **booted** to `starting main dispatch loop`, with no
warning naming `transport_protocol` in any validate or boot log. Every body corroborated by
`http.<stat_prefix>.downstream_rq_total` on **distinct** per-chain prefixes; `no_filter_chain_match` read
**0** on every arm (every listener carried a default chain).

### 2.1 TCP — control T0 run FIRST

| arm | `fc_indexed` match | `tls_inspector` | body | `chain_indexed` rq | `chain_default` rq |
|---|---|---|---|---|---|
| **T0** control | `{}` | no | `INDEXED` | 1 | 0 |
| **T1** | `raw_buffer` | no | **`INDEXED`** | 1 | 0 |
| **T2** | `raw_buffer` | yes | `INDEXED` | 1 | 0 |
| **T3** matched negative of T1 | `tls` | no | `DEFAULT` | 0 | 1 |
| **T4** | `totally_bogus_value` | no | `DEFAULT` | 0 | 1 |
| **T5** case | `RAW_BUFFER` | yes | **`DEFAULT`** | 0 | 1 |

⚠️ **One unexplained transient, recorded not glossed:** T3's first run read listener `downstream_cx_total`
**2** for one `curl`; the rerun read **1** with the same `DEFAULT` body. Not reproduced. No arm's verdict
rests on `downstream_cx_total`.

### 2.2 QUIC — control Q0 run FIRST (discharges owed item 1)

Listener shape copied from fixture `0122`'s reference template (QUIC transport socket on **both** slots —
the reference rejects a socketless default slot, D2-QUICTS); only the one string differs. Driven by a Go
HTTP/3 client on the `go.mod`-pinned `quic-go`, `ServerName` set, ALPN `h3`; **every response negotiated
`HTTP/3.0`**, status `222` (non-1xx by design).

| arm | `transport_protocol` | body | `chain_indexed` rq / cx | `chain_default` rq / cx |
|---|---|---|---|---|
| **Q0** control | `quic` | `INDEXED` | 1 / 1 | 0 / 0 |
| **Q1** matched negative | `tls` | `DEFAULT` | 0 / 0 | 1 / 1 |
| **Q2** THE ARM | `totally_bogus_value` | **`DEFAULT`** | 0 / 0 | 1 / 1 |
| **Q3** | `raw_buffer` | **`DEFAULT`** | 0 / 0 | 1 / 1 |

⇒ **On QUIC the reference accepts a bogus value at validate AND boot, and the chain is never eligible.** Q3
proves the TCP `raw_buffer` default does **not** apply to QUIC: the input there is `quic`, and only `quic`.

### 2.3 Arms NOT run

- **The listener-filter TIMEOUT path** (`continue_on_listener_filters_timeout: true` with a filter that
  never classifies). The repair stamps after the pipeline regardless of how it ended; the reference's
  behaviour on that path is **inferred, not measured.** The PLAN must either measure it or state it as
  unasserted.
- **A TLS client against a `raw_buffer` chain with no `tls_inspector`** — the stamp would make that chain
  eligible for a TLS ClientHello on both sides; not driven.
- **Which certificate was presented** — every chain shares one leaf (the phase-97 SPEC §2.5 boundary).

---

## 3. The subject, MEASURED

### 3.1 The reverse-dependency suite under each shape

Selectors resolved with `go list` first: `./cmd/envoy-go/... ./internal/admin/... ./internal/boot/...
./internal/listener/... ./validate/...`. `-count=1 -v`, rc from `PIPESTATUS[0]`, fail lines by
`^(--- FAIL|FAIL)|^ *--- FAIL`, panic gate `^panic:|DATA RACE|SIGSEGV`.

| tip | rc | `=== RUN` | distinct RED | panic gate |
|---|---|---|---|---|
| un-fixed | 1 | 392 | `TestEnvoyGoBinary_TwoListenerCutover` (port flake, §0.9) | 0 |
| reject-lift only, `+1/-9` | 1 | 392 | `TestParseChainSpecRejectsUnknownTransportProtocol` | 0 |
| **combined, `+4/-9`** | 1 | 392 | `TestParseChainSpecRejectsUnknownTransportProtocol` + the §0.9 flake | 0 |

The sorted `=== RUN` rosters of the un-fixed and reject-lift runs were `diff`ed by the agent and are
identical; the combined run was checked by COUNT only (392), which is weaker and is stated as such.
⚠️⚠️ **THE STAMP IS `+3` LINES THAT TURN ZERO TESTS RED. THAT IS A COVERAGE FINDING, NOT A CLEAN BILL**
(method note 50): no unit test in the tree drives a no-listener-filter connection at a `raw_buffer` chain.
Its falsifiability is currently carried by §3.2 alone, which is why §5.2 is owed.

### 3.2 The same six TCP arms through the REAL binary

Binary built `-o` into scratch; listeners `16050-16055`, admin `16060-16065`; plaintext `curl`.

| arm | un-fixed | reject-lift `+1/-9` | **combined `+4/-9`** | reference |
|---|---|---|---|---|
| T0 | INDEXED | INDEXED | INDEXED | INDEXED |
| T1 | **DEFAULT** | **DEFAULT** | **INDEXED** | INDEXED |
| T2 | INDEXED | INDEXED | INDEXED | INDEXED |
| T3 | DEFAULT | DEFAULT | DEFAULT | DEFAULT |
| T4 | **rc=1 reject** | DEFAULT | DEFAULT | DEFAULT |
| T5 | **rc=1 reject** | DEFAULT | DEFAULT | DEFAULT |

T4's reject message, verbatim: `listener: "l_tp": filter_chains[0]: transport_protocol
"totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty`. **The combined shape agrees with the
reference on all six; each earlier shape disagrees on at least one**, and the T1 column is reproduced
independently by the agent and by the controller.

### 3.3 QUIC did not depend on the reject (discharges owed item 2)

Under the reject-lift prototype, `go test -count=1 -v -run 'QUIC' ./internal/listener/` → rc=0, **25 of 25**
top-level tests PASS, including every `TestQUICChainSelection_*` and
`TestParseChainSpec_QUICTransportProtocolAccepted`. A scratch test modelled on
`TestQUICChainSelection_TransportProtocolQUICMatches` with `totally_bogus_value` on `filter_chains[0]` plus a
QUIC-TLS default slot: **prototype PASS** — `NewManager` succeeds, the spec keeps the value, `quicChain(nil)`
returns the default slot — and **un-fixed FAIL at the boot step** with the reject message. The four
`quic_test.go` narration sites (§0.7) describe a **precondition** that the repair keeps true (`"quic"` and
`"tls"` still parse); what dies is their claim that parsing is **exhaustive over four values**.

---

## 4. The production edit — DECIDED

### 4.1 The shape

In `internal/listener/manager.go`:

1. **`func parseChainSpec(`** — replace the comment + `switch` with
   `spec.TransportProtocol = fm.GetTransportProtocol()`. **No case folding, no trimming, no validation.**
2. **`func (rt *listenerRuntime) serveConnection(`** — between the listener-filter pipeline (step 4) and
   `listenerfilter.SelectChain` (step 5), on **every** path that reaches selection including the
   `continue_on_listener_filters_timeout` fall-through:
   `if inputs.TransportProtocol == "" { inputs.TransportProtocol = "raw_buffer" }`. A short comment names the
   reference behaviour and ADR-0320. ⚠️ **Anchor the install on the ENTRY of selection, not "after the
   pipeline"** (method note 3f): the fall-through path must be covered too.
3. **`quic.go` is BYTE-UNTOUCHED.** The QUIC path already stamps `quic`, which is exactly the reference.

Measured: `4 / 9` by `git diff --numstat`, `go build ./...` OK. **`inputs` is read by nothing after
`SelectChain`** in `serveConnection` (checked by symbol over the rest of the function), so the stamp
changes chain selection and no other observable.

### 4.2 Rejected shapes

- **(A) Reject-lift only, bank the stamp** — REJECTED: fails T1 (§3.2), and its fixture is either RED or
  dishonest (§0.1).
- **(B) Stamp inside `listenerfilter.SelectChain`** — REJECTED: `SelectChain` is shared with the QUIC path,
  where the reference's input is `quic`; a default there would be dormant today and wrong the day a QUIC
  input arrives empty.
- **(C) Add a `tls_inspector`-like implicit filter** — REJECTED: invents a pipeline stage the reference does
  not run and moves `lfPeekBufSize` semantics for every listener.
- **(D) Case-fold the value** — REJECTED: the reference is case-sensitive (T5).

### 4.3 The declared behaviour changes

1. A bootstrap carrying any `transport_protocol` outside the four literals now **boots** (rc=0 validate and
   serve) where it exited 1.
2. **A no-listener-filter TCP connection now matches `transport_protocol: raw_buffer` chains.** Any existing
   config with such a chain and no `tls_inspector` changes which chain serves: from the fallback to the
   `raw_buffer` chain. **That is the parity direction**, and it is the only behaviour change that can move
   traffic on a config that booted before this row. It is stated here so the IMPL's ledger and ADR
   §Consequences cannot omit it.

---

## 5. Unit-test design

### 5.1 The re-pointed parse test (discharges owed item 3) — RE-POINTED, NOT RELAXED

`TestParseChainSpecRejectsUnknownTransportProtocol` is **replaced** by
`TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue`. Listener `l_tp`: `filter_chains[0]`
`transport_protocol: "sctp"`; `filter_chains[1]` `source_type: SAME_IP_OR_LOOPBACK` and nothing else. Every
probe input carries loopback `SourceIP`. **Four properties, one `t.Errorf` each, each message naming its
property:**

| prop | asserts | caught by |
|---|---|---|
| (a) | `NewManager` succeeds (`t.Fatalf` — nothing below is reachable without a manager) | un-fixed tip |
| (b) | parsed `TransportProtocol` is **byte-exactly** `"sctp"` | NC: parse drops the value to `""` |
| (c) | for detected `""`, `"tls"`, `"raw_buffer"`, real `SelectChain` over `rt.chainSpecs` / `rt.defaultSpec` does NOT pick fc[0] **and** does pick fc[1] | NC: TransportProtocol clause deleted from `matches()` |
| (d) | detected `"sctp"` DOES pick fc[0] — the dimension is enforced, not ignored | NC: parse drops the value |

**Measured by the agent** (source in `/tmp/claude-1000/p98spec-sub-scratch/newtest.go.txt`, to be re-derived by
the PLAN, not pasted): (i) un-fixed → FAIL at (a); (ii) prototype → PASS; (iii) parse-drops NC → FAIL at (b)
and (d); (iv) `matches()`-clause-deleted NC → FAIL at (c) for all three detected values; (v) restored →
PASS. ⚠️ **(c) is BLIND to NC (iii) and (d) is BLIND to NC (iv)** — an emptied fc[0] loses to the sibling on
specificity, and with the clause gone `"sctp"` still wins on specificity. **Neither half is a gate alone;
the pair is.** Score the NC roster PER PROPERTY. **Why strictly stronger than the reject it replaces:** the
old test pinned only that a message named the value; the new one pins storage, non-eligibility against
three realistic inputs, and enforcement.

### 5.2 The stamp-site arms — OWED, because the stamp turns zero tests RED

Through the **real** `serveConnection` (Start + a real loopback TCP client; port 0; never `net.Pipe`),
**no `listener_filters`**, a default chain present, arms on **both sides** of the conditional:

- **(s1)** chain `raw_buffer`, plaintext client → the `raw_buffer` chain serves. **RED at the un-fixed tip.**
- **(s2)** chain `tls`, byte-identical otherwise → the default chain serves. **Matched negative of (s1):**
  green for a stamp that writes `raw_buffer`, RED for a stamp that writes any constant a `tls` chain equals
  and for a selector ignoring the dimension.
- **(s3)** chain `transport_protocol: tls` terminating TLS, **with** `tls_inspector`, a **TLS** client → the
  `tls` chain serves. **Proves the stamp does not overwrite a classified input.** ⚠️ A plaintext arm
  CANNOT carry this proof: `tls_inspector` stamps `raw_buffer` on plaintext, so an unconditional
  `raw_buffer` stamp overwrites it with the same value and every plaintext arm stays green. Only an input
  the inspector classified as something OTHER than the default discriminates (method note 7d: name the
  mechanism before running the control).
- **(s4)** the `continue_on_listener_filters_timeout` fall-through — **only if** the PLAN can construct a
  pipeline that times out without classifying; otherwise state it NOT CONSTRUCTIBLE with a named mechanism
  (method note 73), never as an absence.

Which chain served must be observed by a **per-chain discriminator the test controls** (distinct upstream
or distinct response bytes), never by "the connection was not closed" — a default chain also keeps it open.

### 5.3 What is deliberately NOT added

- No QUIC unit arm for a bogus value **in the committed suite** unless the PLAN finds an uncovered path —
  §3.3's scratch test showed the existing accessor already returns the default slot; the fixture does not
  cover QUIC (§7.3), so the PLAN must **decide** this explicitly rather than drop it.
- No `TestNoNewStat*` guard: the row registers no stat (§9).

---

## 6. Occurrence set of every falsified claim (discharges owed item 6)

**Matchers run, and the UNION quoted** (method note 49 — each matcher alone was blind to some site):
M1 `must be "tls"` literal · M2 `enum (domain|gate)|four-member|4-member` (case-insensitive) ·
M3 the four-value set listed in either quoting · M4 `transport_protocol` within 80 chars of
`reject|validat|unknown|invalid|bogus` (case-insensitive) · M5 `no listener.?filter … transport`, `only writer
of that input`, `TransportProtocol … (empty|"")` · M6 bare `raw_buffer` over `BEHAVIOR_CONTRACT.md` and
`DECISIONS.md`. All via `git grep` (ugrep-blindness does not apply to `git grep`).

### 6.1 Claim K1 — *"transport_protocol is validated against a closed set / unknown values error"*

| site | kind | disposition |
|---|---|---|
| `internal/listener/manager.go` — comment + `switch` in `func parseChainSpec(` | CODE | **DELETED** by §4.1(1) |
| `internal/listener/manager_test.go` — `TestParseChainSpecRejectsUnknownTransportProtocol` + its 3-line doc comment | TEST | **RE-POINTED** (§5.1) |
| `internal/listener/quic_test.go:690-694` — `PARSE PRECONDITION … accepts exactly {…} (manager.go:985-991)` | COMMENT | **EDIT** — keep the precondition (`"quic"` parses), drop "exactly" and the stale anchor |
| `internal/listener/quic_test.go:702` — `t.Fatalf` text `parseChainSpec's enum gate must accept` | TEST MESSAGE | **EDIT** — no enum gate exists after the repair |
| `internal/listener/quic_test.go:768-770` — `"tls" is in parseChainSpec's accepted enum domain … (manager.go:985-991)` | COMMENT | **EDIT**, same as `:690` |
| `internal/listener/quic_test.go:779` — same `t.Fatalf` text | TEST MESSAGE | **EDIT** |
| `internal/listener/listenerfilter/chainmatch.go:30-32` — `"tls" or "raw_buffer" means …` | COMMENT | **EDIT** — any non-empty string; name the TCP default |
| `docs/envoy-go/DECISIONS.md` ADR-0279 (`:16736`, `:16740`) — the `"quic"` reject *"is LIFTED"*, *"the ONE … reject 61.1 lifts"* | NORMATIVE, HISTORICAL | **LEAVE** — true of phase 61.1; ADR-0320 records the later full lift |
| `docs/envoy-go/DECISIONS.md` ADR-0319 (`:19085`, `:19210`) — *"does not decide `parseChainSpec`'s `transport_protocol` over-strictness … which remains unmeasured"* | NORMATIVE, ACCEPTED | **LEAVE** — accurate about what ADR-0319 decided; ADR-0320 §Context names it as the question now answered |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md:5881` — *"The `transport_protocol: "quic"` … value is accepted"* | CONTRACT | **LEAVE** — still true, a subset |
| `docs/envoy-go/phases/61-*`, `97-*`, `98-*/BRAINSTORM.md` | PHASE RECORDS | **LEAVE** — governing documents of closed stages are history |

### 6.2 Claim K2 — *"with no listener filter the detected transport protocol is empty"*

| site | kind | disposition |
|---|---|---|
| `internal/listener/listenerfilter/types.go:44-46` — `… or "" if no listener filter inspected the connection` | COMMENT | **EDIT** — true when the pipeline returns, false by the time selection reads it; say both |
| `docs/envoy-go/DECISIONS.md` ADR-0319 §Context ¶8 (`:19079`) — *"The only writer of that input field … writes `"tls"` or `"raw_buffer"`"* | NORMATIVE, ACCEPTED, DATED | **LEAVE** — true at phase 97; ADR-0320 names the second writer |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` §Chain-match algorithm (`:4363-4368`) | CONTRACT | **ADD** a bullet (it states no transport-protocol input semantics today — an omission, not a false claim) |

### 6.3 Set-difference against the byte-untouched roster (method note 62)

**Edit set:** `manager.go`, `manager_test.go`, `quic_test.go`, `chainmatch.go`, `types.go`,
`BEHAVIOR_CONTRACT.md`, `DECISIONS.md`, plus the fixture directory and `test/differential/runner_test.go`.
**Byte-untouched roster for the IMPL:** `internal/listener/quic.go`, `internal/listener/listenerfilter/tls_inspector/**`,
`internal/listener/listenerfilter/chainmatch.go` **CODE** (comment-only edit — ⚠️ **mechanically gate it**: the
IMPL's `git diff` of that file must contain only `//` lines, and that gate must be run against a probe diff
that touches code), every other fixture, `go.mod`/`go.sum`. **Intersection: `chainmatch.go`**, resolved by the
comment-only constraint and its gate. No other intersection.

---

## 7. Differential fixture `0123-listener-transport-protocol` — CHARTERED (discharges owed item 4)

### 7.1 Why ONE directory, THREE listeners

One directory dispatches to one runner branch; all arms are plaintext TCP HTTP/1.1, so they share one. The
precedent for several TCP listeners in one fixture is `0008-listener-chain-match` (`ReferenceListenerPorts`
returns two). **No `listener_filters` on any listener**, and **every listener carries a default chain**
(§3.2 of the BRAINSTORM — envoy-go emits no `no_filter_chain_match`).

| listener | `fc_indexed` match | expected, both sides | what it falsifies |
|---|---|---|---|
| `l_bogus` | `transport_protocol: totally_bogus_value` | `DEFAULT` | un-fixed subject **boot-rejects** — the whole fixture fails |
| `l_raw` | `transport_protocol: raw_buffer` | `INDEXED` | a reject-lift-only subject (§3.2 T1) |
| `l_tls` | `transport_protocol: tls` | `DEFAULT` | a subject that ignores the dimension, or stamps a constant `tls` matches |

⚠️ **`l_bogus` alone is a FALSE-AGREEMENT arm** (method note 58): a subject that ignores the field answers
`DEFAULT` too. Only the `l_raw` / `l_tls` pair, one string apart, excludes a constant answer. ⚠️ **Adding
`tls_inspector` to any listener would silently disarm `l_raw`** (§2.1 T2 agrees on both sides) — the README
must say so.

### 7.2 Ports — CENSUSED, not inherited

Primary reference listener **`15123`** on the `15000 + <index>` convention; the two further listeners take
**`15223`** and **`15224`**, off-convention so that fixtures `0124`/`0125` keep theirs. At this tip
`git grep -c '\b<port>\b' -- test/ internal/ cmd/` reads **zero files** for each of `15123 15124 15125 15126
15223 15224 15225`. **The PLAN re-censuses at its own tip** and checks `ss -tan`/`ss -uan`.

### 7.3 Assertions — a NAMED SUBSET

Per listener: the response body (distinct `direct_response` bodies, status **200**) and the two per-chain
`http.<prefix>.downstream_rq_total` counters on distinct prefixes. ⚠️ **Prove each counter NAME is emitted
by BOTH sides before pinning it** (method note 46) — a scrape map returns zero for a missing key. **QUIC is
NOT in this fixture**: the harness cannot mix TCP and UDP listeners in one fixture (`97/PLAN.md` §3), and a
second directory for Q2 is not chartered; §2.2 is the governing measurement and §5.3 obliges the PLAN to
decide the unit-level QUIC arm explicitly.

### 7.4 Registration

All **four** gates (method note 60): `fixture.RegisterFixture` in `init()`, the blank import in
`test/differential/runner_test.go`, byte-identity of the registered name with the directory name, and the
`NNNN-` directory shape. **Score on the fixture-set set-difference, both `comm` directions, never on the exit
code.** Fixtures **124 -> 125** at the IMPL.

---

## 8. `ADR-0320` — §Context drafted HERE (discharges owed item 5)

Appended to `DECISIONS.md` after `ADR-0319`, in the ADR-0294-through-0319 shared block form: the
`> **STATUS: PROPOSED` blockquote, `### Context`, the paragraphs, and a **RETAINED** italic footer. The IMPL
appends §Decision + §Consequences **after** the footer, with no `**Status:**` line, no renumber and **no
`---`**. **This arms the house guard.** Figures re-derived in §10.

---

## 9. `BEHAVIOR_CONTRACT.md` ledger (discharges owed item 7)

**The row owes a `+0, UNCHANGED` ledger entry at the IMPL**, in the phase-96/97 form, **quoting no
absolute.** Neither edit registers, renames or removes a stat name: the parse edit touches no registry, and
the stamp writes a local struct field read only by `SelectChain`. It **also** owes the §Chain-match
algorithm bullet of §6.2. A SPEC edits neither; `BEHAVIOR_CONTRACT.md` is byte-untouched at this stage.

---

## 10. Counts, re-derived at THIS stage's own tip

### 10.1 Moved by this stage

- `DECISIONS.md` lines **19213 -> 19235** (`+22 / -0` by `--numstat` — ⚠️ **NOT the `+30` of all four
  precedents**: a shape, not a law; this §Context has seven paragraphs, not eleven); `^## ADR-` **318 -> 319**; bare `^## ` **326 -> 327**; tail **ADR-0319 -> ADR-0320**;
  next-free **ADR-0321**; `^---$` **216, UNMOVED**. The house guard `^> \*\*STATUS: PROPOSED` is **ARMED**,
  verified by line and by backward heading search to `## ADR-0320`; the ADR-0231 decoy form is
  **byte-untouched**. No count of either matcher is written here.
- `STATE.md` rolled in place; `STATE_HISTORY.md` **572 -> 574**; this `SPEC.md` new.

### 10.2 Not moved — and the scope was MEASURED

`git show --numstat` on the phase-94, -95, -96 **and -97** SPEC commits (`307f2e3d`, `9ed6a620`, `da6ea191`,
`331c3524`): each touches **exactly five** paths — `DECISIONS.md` (`+30/-0` each), `STATE.md`,
`STATE_HISTORY.md` (`+2/-0` each), the new `SPEC.md`, `next-prompt.txt`. **This stage touches the same five** — the
four precedents agree on the PATH set and the `STATE_HISTORY.md` shape, not on the `DECISIONS.md` line count.
`ROADMAP.md` (**248** lines, row 98 `in-progress` at `:160`), `BEHAVIOR_CONTRACT.md`, `internal/**`, `test/**`
are byte-untouched. Fixtures **124**, tail `0122-quic-chain-selection`. Phase dirs **139**.

---

## 11. Negative-control roster the PLAN inherits — neutralise, never revert

| # | mutation (compiles) | must redden | must NOT redden |
|---|---|---|---|
| 1 | restore the enum `switch` | §5.1 (a); fixture `l_bogus` (boot) | — |
| 2 | parse stores `""` | §5.1 (b), (d) | §5.1 (c) |
| 3 | delete the TransportProtocol clause in `matches()` | §5.1 (c); §5.2 (s2); fixture `l_tls` | §5.1 (d) |
| 4 | delete the stamp | §5.2 (s1); fixture `l_raw` | §5.2 (s2), (s3) |
| 5 | stamp unconditionally with `"tls"` | §5.2 (s1), (s2); fixture `l_raw`, `l_tls` | §5.2 (s3) |
| 5b | drop the `== ""` guard — stamp `"raw_buffer"` unconditionally | §5.2 (s3) **only** | §5.2 (s1), (s2); every fixture listener (all plaintext) |
| 6 | stamp moved into `SelectChain` | **name the arm, or delete the row as vacuous** — no current arm distinguishes it on TCP | — |

Row 6 is listed **so the PLAN adjudicates it**, not because it is known to fire: ask what mechanism would
carry the mutation to a failure (method note 7d) before running it.

---

## 12. Sentinel — RUN MECHANICALLY AT THIS STAGE's TIP (`45728a81` + this stage's docs), `/usr/bin/grep`

A SPEC adds no row and flips none; measured anyway, not inherited.

- **Check (1)**, `want=130`: **ONE** line, `NOT DONE: row 98` — this row's own `in-progress` status.
- **Check (2): SIX**, at `:208 :214 :220 :230 :236 :244`. Per-line md5, **trailing newline INCLUDED**
  (`sed -n 'Np' f | md5sum`, first 12 hex): `208 10d7807bf02d` · `214 4a92f7e62fc6` · `220 2a7eb298b9fd` ·
  `230 242e53c6f7a3` · `236 b2680e6f4fbf` · `244 6caa1c3ce0e7` — **byte-identical to the BRAINSTORM close.**
- **Check (3): SILENT.**
- **NC-A** (row 62 doctored, substitution inspected first: `NC LANDED? [ in-progress ]`): **TWO** lines,
  `NOT DONE: row 62`, `NOT DONE: row 98`.
- **NC-B** (`want=129`): **TWO** lines, `NOT DONE: row 98`, `GATE FAIL: examined 130 data rows, expected 129`.
- **NC-C:** residual **0**, `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D:** `-family row` under `--`: **96** occurrences / **68** lines.
- **Check-(2) positive control:** both phrases substituted, residual **0**, `candidatesXX` **6**.
- **Escape-aware malformed set:** exactly **{57, 69}** at file lines **119** and **131**, NF **9** and **10**.
- `stop` verified **ABSENT** at the git root and in the stage worktree.

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED.** The margin remains ONE.

---

## 13. Probe hygiene

Two agents in parallel, **split on the Docker axis**: the reference agent alone used Docker (ports
`16000-16049`, containers `p98spec-ref-*` removed BY NAME, none foreign touched); the subject agent used no
Docker (ports `16050-16099`); the controller reused the subject agent's rig on the same ports **after** that
agent had exited and proved them free. Band `16000-16099` censused first: `git grep -nE '\b160[0-9][0-9]\b'`
hits are a phase-00 plan's example `port: 16000`, fuzz exec rates, HPACK byte sizes, and the router's own
census sentence — **no live port** — and `ss` read zero sockets. All three throwaway worktrees were removed;
`git status --porcelain --untracked-files=all` at the canonical root lists only the pre-existing `.claude/`.
**Nobody committed anything but this stage.**

---

## 14. What the PLAN owes

1. **Re-derive every anchor in §6 by LITERAL TEXT at its tip**, and re-run §6's matchers — the union may grow.
2. **Order the spine so the un-fixed tip is measured FIRST**: land §5.1, §5.2 and the fixture, run them RED,
   record which arms are RED and which are structurally GREEN (`l_bogus`-alone class), then land §4.1(1), then
   §4.1(2), running between the two — `l_raw` and (s1) must stay RED after (1) and turn GREEN only at (2).
3. **Adjudicate NC roster row 6** and decide §2.3's timeout arm and §5.3's QUIC unit arm, each explicitly.
4. **Prove both per-chain counter NAMES exist on both sides** before `0123` pins them.
5. **Gate the `chainmatch.go` comment-only constraint mechanically**, NC'd against a code-touching probe.
6. **Budget honestly**: `+4/-9` production is a FLOOR; the fixture floors are `1359` and `1393` added lines.
   Evaluate BOOTSTRAP §6.1 (**~25 tasks / ~1500 LOC**) with a command, not by eye.
7. **Carry §0.9's flake** into the gate posture: a `TestEnvoyGoBinary_TwoListenerCutover` bind failure inside
   the ephemeral range is not this row's regression, and a green rerun clears nothing.
