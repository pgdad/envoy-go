# Phase 97 — `quic-chain-selection-order` — SPEC

**Stage:** SPEC (lifecycle-state **1 -> 2**). Discharges all NINE owed items of `BRAINSTORM.md` §10.

**Tip:** `01559d55` (`git rev-parse master` at session start). Fresh worktree `phase-97-spec` off that
tip per `feedback_git_worktrees`. Docs-only, ZERO production `.go`.

**One sentence.** The pinned reference obeys ADR-0080 §Decision 1 and §Decision 2 on the QUIC path and
evaluates the full `filter_chain_match` surface there; envoy-go evaluates **none** of it and
unconditionally prefers `default_filter_chain`, so this row makes the QUIC path call the algorithm the
sibling TCP path already calls, with the inputs a QUIC connection can actually supply.

**The blocking unknown is ANSWERED, and it eliminated the BRAINSTORM's own prototype shape.** See §0.1
and §0.2.

---

## 0. What this stage refuted, by execution

Eleven claims, four of them load-bearing on the repair, **two of them made by this stage's own
measurement agents**, and one of them the BRAINSTORM's measured cost floor.

### 0.1 🔴 THE BLOCKING UNKNOWN IS ANSWERED — AND THE REFERENCE HAS NO QUIC EXCEPTION

`BRAINSTORM.md` §10 item 1 and §9.6 left the reference unmeasured on QUIC and warned that the fix
shape encodes the answer. Measured at this stage on `envoyproxy/envoy:contrib-v1.37.2`, digest
`sha256:7edd5b0fd763…` verified against `ENVOY_TARGET.md` lines 3-4 before any arm was trusted, run
BY DIGEST, twelve arms per the table in §2:

- The reference **accepts** a QUIC listener carrying both a `default_filter_chain` and an eligible
  `filter_chains[]` entry, and serves from the **indexed** chain — `INDEXED_CHAIN` **1**,
  `DEFAULT_CHAIN` **0**. **ADR-0080 §Decision 2 holds on QUIC, unmodified.**
- The reference evaluates **every** `filter_chain_match` dimension tested on QUIC:
  `destination_port`, `transport_protocol`, `application_protocols` and `server_names`, each with a
  matched positive/negative pair on byte-identical listeners so that "eligible" is distinguished from
  "dimension unenforced".
- **§Decision 1 holds too**: an ineligible indexed chain leaves the default chain serving.

⇒ **There is no QUIC-specific reference semantics to discover.** The repair direction is not novel
design; it is applying the decision already written.

### 0.2 🔴 THE BRAINSTORM'S OWN PROTOTYPE SHAPE IS REFUTED BY THE ARMS IT COULD NOT RUN

`BRAINSTORM.md` §6 measured a `+7 / -10` prototype whose `quicChain()` **walks `rt.chainSpecs` first
and falls back to `rt.defaultChain`** — i.e. it prefers `filter_chains[0]` unconditionally. That
shape is correct for §Decision 2's empty-match arm and **wrong for four measured arms**: with an
indexed chain made ineligible by `destination_port`, by `transport_protocol: "tls"`, or by
`application_protocols: ["h2"]`, the reference serves the **default** chain and the prototype would
serve the indexed one. `BRAINSTORM.md` §3.2 predicted exactly this and could not test it; it is now
tested. **The measured cost floor is a floor on the WRONG SHAPE, and it is superseded by §4.**

### 0.3 🔴 THE DEFECT IS NOT ONLY AN ORDERING INVERSION — envoy-go DOES NOT EVALUATE `filter_chain_match` ON QUIC AT ALL

The BRAINSTORM frames the subject as preferring the default chain. An arm it did not run — an
**ineligible** indexed chain with **no** `default_filter_chain` at all — separates the two
hypotheses. The reference closes the connection with `Closed by application with reason: no filter
chain found` and books `INDEXED_CHAIN` **0**. envoy-go **serves it anyway**: `222`,
`"from-indexed-chain"`, `INDEXED_CHAIN` **1**.

⚠️ **THEREFORE EVERY SUBJECT ROW THAT HAPPENS TO AGREE WITH THE REFERENCE IN §2 IS A FALSE
AGREEMENT.** On the ineligible-plus-default arms the subject answers `from-default-chain` and so does
the reference — but the subject answers that on *every* arm, because it never evaluates the match at
all. **A row that agrees for a reason the mechanism cannot supply is not evidence about the
mechanism**, and any gate built on those arms alone would be vacuous.

### 0.4 🔴 THE "BOTH ACCESSORS ASSERT EXACTLY ONE CHAIN" CLAIM IS FALSE, AND THE TRUTH IS WORSE

`next-prompt.txt` and `BRAINSTORM.md` §3.3 both say the two accessors carry a comment asserting a
one-chain precondition. Read at this tip, **only `quicChain` does** (*"harmless here because the
minimal QUIC slice supports exactly one chain"*). `quicTLSConfig`'s comment says the **opposite** —
*"if multiple chains exist, the first non-nil TLS config is used"* — which contemplates the
multi-chain case rather than forbidding it. **The two comments contradict each other**, so owed item
4 is not "repair one claim" but "reconcile two that disagree". A census of the phrase
`exactly one chain`, case-insensitively and tree-wide including `docs/` and `next-prompt.txt`, finds
its only *normative* carrier is `DECISIONS.md` ADR-0025 clause 1, which still reads `Accepted` while
being superseded in substance by ADR-0033 / ADR-0078 / ADR-0080. ⚠️ **A case-SENSITIVE grep is blind
to `test/fixtures/0008-listener-chain-match/expectations.yaml`, which spells it `EXACTLY one chain`
and is about a different subject entirely.**

### 0.5 ⚠️ `transport_protocol: "quic"` PARSES AND CAN NEVER MATCH

`parseChainSpec` accepts exactly `{"", "tls", "raw_buffer", "quic"}` and its comment promises that
*"the runtime chain-match semantics for a QUIC connection land at leg 61.2"*. They did not land.
The **only** writer of `ChainMatchInputs.TransportProtocol` anywhere in the tree is `tls_inspector`,
which writes `"tls"` or `"raw_buffer"` and never runs on QUIC — and the QUIC path never constructs a
`ChainMatchInputs` at all. So a chain matching on `transport_protocol: "quic"` is **silently
unreachable today**: it parses, is stored on the `ChainSpec`, and matches nothing. §2 measures that
the reference **does** serve such a chain. This row closes that gap as a corollary, and §4 says how.

### 0.6 ⚠️ THE COVERAGE FINDING IS REAL BUT THE BRAINSTORM'S DIAGNOSIS OF IT IS WRONG

`BRAINSTORM.md` §0.4 says *"No existing test asserts the current behaviour, and none would have
caught the repair … the guard is absent."* The second half is confirmed first-hand at this tip: a
`+5 / -0` patch that reverses QUIC chain selection leaves `go test ./internal/listener/... -count=1`
at **rc=0** with all three packages `ok`, `gofmt` clean and `go vet` rc=0 — the fix and the un-fix
produce identical greens.

But the framing "the guard is absent" implies the sites are untested ground. A **`panic()`
reachability control** says otherwise: **both accessors are executed by the existing suite.**
`quicTLSConfig` is reached directly from `manager_test.go:1075`
(`TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos`), and `quicChain` is
reached from `serveQUICConnection` **on a real accepted QUIC connection**, first fired inside
`TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives`. The sites are live, driven, and blind.

⚠️ **THE FIRST CONTROL RUN MASKED THE SECOND SITE.** With a panic installed in *both* accessors the
run aborted on `quicTLSConfig` and said nothing about `quicChain`; the second site was only
established by removing the first panic and re-running. **A fail-fast control measures one site per
run — isolate, or you will report an absence you never tested.**

### 0.7 ⚠️ AN AGENT REFUTED AN INHERITED FIGURE BY RUNNING A DIFFERENT COMMAND — AND IT IS THE SECOND ROW RUNNING

`next-prompt.txt` method note 33 says the character-class form on `go.mod` reads **62** where the
structural count is **67**. A measurement agent reported that claim **not reproducible**, both forms
reading 67. Re-run by the controller at this tip: the router's **named** form,
`^\s+[a-z0-9./-]+ v[0-9]`, reads **62**, exactly as claimed; the agent had run `^\t[a-z]`, which
reads **67**. ⚠️ **THE VARIABLE WAS THE MATCHER, NOT THE FILE** — the same lesson as the phase-97
BRAINSTORM's own §9.1, now firing against this stage's agent instead of that one. **When an agent
refutes an inherited figure, ask what it RAN, not only what it found.** Method note 33 stands
unchanged.

### 0.8 ⚠️ THE ROUTER'S QUIC PORT-BAND CLAIM IS FALSE AS STATED

`next-prompt.txt` says *"`0104`, the tree's ONLY QUIC fixture, sits at reference port `15104`"* and
frames `15xxx` as a QUIC band. `0104` **is** the only QUIC fixture — it is the sole implementor of
`fixture.ReferenceListenerIsUDP` — but **28 distinct `15xxx` literals** live under
`test/fixtures/*/driver` and `*/inputs`, spanning `15000`-`15104` plus `15360`. The defensible
statement is narrower: **`15104` is the only reference port at or above `15100`**, and the family
convention that produced it is `15000 + <fixture index>`, a **derived observation that no document in
this tree states**. See §7.2.

### 0.9 ⚠️ THERE ARE FOUR REGISTRATION GATES, NOT THREE, AND ALL OF THEM SKIP RATHER THAN FAIL

Owed item 5 asks for three. Measured: (1) `fixture.RegisterFixture` from the driver's `init()`;
(2) the blank import in `test/differential/runner_test.go`; (3) **byte-identity between the directory
name and the string passed to `RegisterFixture`**, because `discoverFixtures` yields directory names
and `fixture.DriverRegistry` is keyed by the registered string; and (4) the directory name must match
the `NNNN-` / `NNNNa-` shape `discoverFixtures` enumerates, or **no subtest is created at all**.
Gates 1-3 converge on a `t.Skipf` — **a SKIP, not a FAIL** — and gate 4 produces not even a skip
line. At this tip the extractor reads **123 imports against 123 directories**, both `comm`
directions EMPTY, split **99 `driver/` + 24 `inputs/`**.

### 0.10 ⚠️ ADR-0044 DOES NOT CONTAIN THE DISCIPLINE THIS PROJECT CITES IT FOR

Read in full: ADR-0044 is *"BEHAVIOR_CONTRACT HTTP/1.1 subsection"*, `**Status:** Accepted`,
2026-04-25, and is entirely about which HTTP/1.1 equivalence dimensions the differential gate
asserts. It says nothing about drafting an ADR §Context at a SPEC and appending §Decision at an IMPL.
The phase-75 note that first flagged this is **CONFIRMED**. The discipline is real and consistently
practised — it is the `> **STATUS: PROPOSED` blockquote plus a RETAINED italic footer, the
ADR-0294-through-0318 shared block form — but **no ADR documents it**, and the misattribution is
carried by a very large number of lines tree-wide. ⚠️ **NO COUNT OF THOSE CITATIONS IS QUOTED HERE**:
this SPEC is itself about to become one more carrier, so any figure it named would be falsified by
its own landing. Recorded, deliberately not repaired by this stage.

### 0.11 ⚠️ AN ANCHORED IMPL-STAGE COMMIT ROSTER READS ZERO FOR PHASE 95

`next-prompt.txt` prescribes anchoring stage-commit locates on `^phase NN (slug) STAGE:`. Phase 95's
IMPL landed as `0f3b98b3`, subject *"phase 95 (tls-alpn-mismatch-fallback): an ALPN-mismatching
client is now SERVED…"* — **no stage token at all**. An anchored `^phase 95 (.*) IMPL` roster finds
**zero** phase-95 IMPL commits. The anchored form is still right for SPEC/PLAN/BRAINSTORM commits, and
this SPEC's own scope measurement used it — but **the anchor is blind wherever an author omitted the
stage word, and reconciling against the loose form is what catches it.**

---

## 1. Scope, restated as a decision

**IN.** The QUIC/HTTP-3 serving path selects its filter chain by
`listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)` — the same call the TCP path
makes — with a `ChainMatchInputs` the QUIC path builds itself. Both `quicChain()` and
`quicTLSConfig()` are re-expressed in terms of that one selector, so they can no longer name
different chains. The map iteration over `rt.chainByName` is **removed**, not merely documented.

**OUT, and stated plainly:**

- **Per-connection TLS *identity* dispatch.** `quic.Listen` takes one `*stdtls.Config` at Start.
  Selecting a different **certificate** per connection needs `GetConfigForClient` on the QUIC config,
  which ADR-0317 (`D-ALPNFB-TCPONLY`) forbids on the QUIC path for an unrelated reason and which the
  existing pin at `manager_test.go` asserts stays nil. That reconciliation is a separate row. §4.4
  states the exact residual this leaves and §9 banks it.
- **D2-QUICTS**, the `default_filter_chain` no-`transport_socket` acceptance divergence. Now
  reference-confirmed with its exact message (§2, arm B). It fails **closed** and is separately
  banked; this row does not repair it.
- **`ROADMAP.md` and `BEHAVIOR_CONTRACT.md`.** Byte-untouched at this stage — a scope **measured**,
  not inferred, at §10.2.

**Charter, unchanged from `BRAINSTORM.md` §1.1 and now measured rather than argued:** make the chain
that supplies the **filters** and the chain that supplies the **TLS identity** the same chain, and
stop an eligible indexed chain being silently bypassed.

---

## 2. The reference, MEASURED — twelve arms, both sides, with a negative control FIRST

**Rig.** Reference `envoyproxy/envoy:contrib-v1.37.2` run **by digest**
`sha256:7edd5b0fd763…`, verified against `ENVOY_TARGET.md` lines 3-4 before any arm was trusted.
Subject built from `01559d55` with `-o` into scratch, never into a worktree. Config shape copied from
`test/fixtures/0104-http3-downstream-get`'s own `referenceTmpl` / `subjectTmpl`, so no YAML is
invented. Ports `15800`-`15899`, re-censused free with **both** `ss -tan` and `ss -uan` (all states)
before binding. H3 client: `helpers.H3RoundTrip` from a **standalone scratch Go module** with a
`replace` onto the worktree, so nothing was written into any tree.

⚠️ **THE CLIENT SETS `ServerName` EXPLICITLY.** The `0104` driver does not, so SNI would otherwise be
absent from the ClientHello and **every `server_names` arm would have been vacuous**.

⚠️ **`direct_response` STATUS IS `222`, NOT A 1xx.** `BRAINSTORM.md` §9.7 measured a `111` coming back
as `200` on the H3 path with the body intact. `222` came back as `222` on both sides in all twelve
arms.

### 2.1 The negative control, stated FIRST — arm D

Indexed chain only, **no** `default_filter_chain`. Reference and subject both: validate rc=0, boot
clean, `222`, body `"from-indexed-chain"`, `http.INDEXED_CHAIN.downstream_rq_total` **1**, no
`DEFAULT_CHAIN` scope. **The rig can observe the indexed chain winning on both sides, so every zero
below is interpretable.** Without this row the `INDEXED_CHAIN 0` readings in arm A would be
indistinguishable from a broken rig.

### 2.2 The twelve arms

Counter matcher throughout: `http.{INDEXED,DEFAULT}_CHAIN.downstream_rq_total` scraped from each
side's admin `/stats` **after** a single H3 `GET /health`. The response body is the corroborator and
agreed with the counters in every one of the twenty-four runs.

| arm | `filter_chains[0].filter_chain_match` | default slot | REFERENCE serves | SUBJECT serves |
|---|---|---|---|---|
| **D** control | absent | **none** | indexed | indexed |
| **A** | absent (empty-match) | QUIC TS | **indexed** | **default** |
| **B** | absent (empty-match) | **plaintext** | **BOOT-REJECT** | default |
| **C** | `server_names: ["nomatch…"]` | QUIC TS | default | default |
| **C2** | `destination_port: 15999` | QUIC TS | default | default |
| **E** | `server_names: ["nomatch…"]` | **none** | **connection CLOSED** | **indexed** |
| **F** | `transport_protocol: "quic"` | QUIC TS | **indexed** | default |
| **F2** | `transport_protocol: "tls"` | QUIC TS | default | default |
| **G** | `application_protocols: ["h3"]` | QUIC TS | **indexed** | default |
| **G2** | `application_protocols: ["h2"]` | QUIC TS | default | default |
| **H** | `server_names: ["alpha…"]`, SNI **matches** | QUIC TS | **indexed** | default |

All twenty-four runs validate rc=0 except arm B's reference side. On the reference the winning scope
also carries `downstream_cx_total: 1` while the loser carries `0`, so the QUIC connection itself
terminated on the winning chain's transport socket — TLS and filters land on one chain there.

### 2.3 What each PAIR rules out — a single positive arm proves nothing

⚠️ **`F` ALONE CANNOT DISTINGUISH "the reference stamps `quic`" FROM "the dimension is unenforced".**
Each dimension therefore has a matched negative on a byte-identical listener:

- **F + F2** rule out "`transport_protocol` is ignored on QUIC". `"quic"` is eligible, `"tls"` is not.
  ⇒ **the reference stamps the literal `"quic"`**, and it is not the value the TCP path's
  `tls_inspector` produces.
- **G + G2** rule out "`application_protocols` is ignored on QUIC". `["h3"]` is eligible, `["h2"]` is
  not. ⇒ **the reference stamps `h3`** on the ALPN input.
- **C + H** rule out "the reference does no per-connection SNI dispatch on QUIC". The two differ only
  in the literal name and flip the winner. ⇒ **the reference reads SNI out of the QUIC ClientHello and
  dispatches on it.**
- **C2** is consistent with a destination-port input that is knowable before any connection.

### 2.4 Arm B — phase 96 CONFIRMED, with the exact message, and a second divergence

The reference rejects a `transport_socket`-less `default_filter_chain` on a QUIC listener at
listener-add time, **rc=1**, both in `--mode validate` and at live boot:

```
error adding listener '0.0.0.0:15811': no transport socket specified for connection oriented UDP listener
```

envoy-go accepts it (`-mode validate` rc=0), boots, **and serves from that plaintext default chain
over QUIC**. That is D2-QUICTS, now measured on both sides rather than inherited. It is out of scope
for this row (§1) and stays banked with this message attached.

### 2.5 Arms NOT run — recorded, not glossed

- **Per-chain DISTINCT certificates were not used.** Every arm's chains carry the same
  `alpha.envoy-go.test` leaf, so the arms discriminate which chain's **filters** ran, never which
  chain's **certificate** was presented. The residual in §4.4 is therefore reasoned, not measured, and
  §9 banks the measurement that would settle it.
- **No `source_type` / `source_prefix_ranges` / `source_ports` arm was run.** Those dimensions are
  in the same class as `server_names` — per-connection — and §4 handles them by the same mechanism,
  but the reference was not measured on them.
- **`transport_protocol` values outside `{"", "tls", "raw_buffer", "quic"}` were not tested**, so the
  banked `parseChainSpec` over-strictness stays UNMEASURED.

---

## 3. The mechanism, read by SYMBOL at this tip

### 3.1 Three call sites, two accessors, TWO MOMENTS

| call site | symbol | moment |
|---|---|---|
| `startQUIC`, `tlsCfg := rt.quicTLSConfig()` | `quicTLSConfig` | **Start** — before any connection exists |
| `serveQUICConnection`, `ci := rt.quicChain()` | `quicChain` | **per connection** |
| `serveQUICConnection`, `TLSConfig:  rt.quicTLSConfig()` | `quicTLSConfig` | per connection |

⚠️ **ONE SYMBOL IS CALLED AT BOTH MOMENTS.** Any repair that makes `quicTLSConfig` depend on
per-connection state is unimplementable at the first site; any repair that lets its two invocations
disagree re-creates the cross-wiring this row exists to close. **The two accessors do not have one
job between them — they have two, and §4 separates them explicitly rather than collapsing them.**

### 3.2 The two predicates, and the conjunct that splits them

`quicTLSConfig()` returns `rt.defaultChain.tlsCfg` when **both** `rt.defaultChain` and its `tlsCfg`
are non-nil, else walks `rt.chainByName` for the first non-nil TLS config. `quicChain()` returns
`rt.defaultChain` whenever it is non-nil **at all**, else returns an arbitrary entry of
`rt.chainByName`. They differ by exactly the `tlsCfg != nil` conjunct — which is why
`BRAINSTORM.md` §2.2's plaintext-default arm splits them and cross-wires TLS and filters inside one
connection.

### 3.3 `rt.chainByName` is a MAP and it CONTAINS the default slot

`buildListenerRuntime` writes `chainByName[defaultSpec.Name] = defaultChain`, keying the indexed
chains `"<listener>/filter_chains[i]"` and the default slot `"<listener>/default_filter_chain"`. The
map is therefore **not** the `filter_chains[]` set: it is the union. Both fallback loops range over a
set that includes the very slot the ordering rule says to consult last, in nondeterministic order,
and **can return the default chain by map-order accident while the code reads as if it were choosing
an indexed one.** Removing the iteration removes the hazard; documenting it does not.

### 3.4 Nothing enforces "exactly one chain"

`catchAllCount > 1` rejects two empty-match entries **within `filter_chains[]`**, and ADR-0080
§Consequences (b) states explicitly that the default slot does not count toward it and that
`(1 empty-match chain, default present)` is **valid**. `findIdenticalChainSpecs` rejects duplicate
match shapes, not chain counts. `validateQUICOptions` inspects only `quic_options` sub-fields. **There
is no QUIC chain-count check anywhere on the boot path**, and every probe config in §2 carries two
chains, validates rc=0 and boots. The precondition asserted in `quicChain`'s comment has never held.

### 3.5 What each input is knowable from, and WHEN — the crux of owed item 2

`rt.addr` is written at build time to the **configured** `address:port`, which for a `port_value: 0`
listener is literally `"…:0"`. `startQUIC` overwrites it with the resolved address **after** it has
already called `quicTLSConfig()`.

| `ChainMatchInputs` field | at Start (`conn == nil`) | per connection |
|---|---|---|
| `DestinationIP` / `DestinationPort` | from `rt.addr` — ⚠️ only correct **after** the resolve | `conn.LocalAddr()` |
| `TransportProtocol` | **`"quic"`**, a constant of the listener kind | same |
| `ApplicationProtocols` | **`["h3"]`**, a constant of the listener kind | same |
| `ServerName` | **not knowable** | **`conn.ConnectionState().TLS.ServerName`** |
| `SourceIP` / `SourcePort` | not knowable | `conn.RemoteAddr()` |

⚠️ **`ServerName` IS KNOWABLE PER CONNECTION, AND THE BRAINSTORM'S PREMISE THAT IT IS NOT IS
REFUTED BY EXECUTION.** quic-go v0.54.1 populates `conn.ConnectionState().TLS.ServerName`
server-side; §4's prototype reaches the indexed chain on arm H through
`sniMatchAny(["alpha.envoy-go.test"], inputs.ServerName)` and reaches the default chain on arm C with
the non-matching name, so the field is not merely non-empty — **it carries the actual SNI and is
compared.** The QUIC path runs no listener-filter pipeline, but it does not need one: the SNI arrives
with the connection.

⚠️ **`sniMatchAny` RETURNS TRUE FOR THE LITERAL PATTERN `"*"` REGARDLESS OF THE SNI**, empty string
included. Any statement about `server_names` eligibility under an empty `ServerName` must carry that
exception, so "a `server_names` chain never matches at Start" is **false as an absolute**.

### 3.6 `transport_protocol: "quic"` parses and matches nothing

`parseChainSpec` accepts `{"", "tls", "raw_buffer", "quic"}`. The **only** writer of
`ChainMatchInputs.TransportProtocol` in the tree is `tls_inspector`, which writes `"tls"` or
`"raw_buffer"` and never runs on QUIC. `chainmatch.matches` compares the field by **exact string**, so
stamping `"quic"` into the inputs makes such a chain eligible with **no parser change** — the enum
domain already admits it. §2 arms F and F2 measure that this is what the reference does.

---

## 4. The production edit — DECIDED, and MEASURED against every arm

**One production file: `internal/listener/quic.go`.** Built, run against all twelve arms, and
reverted; the worktree it was built in was removed and its absence proven. **`git diff --numstat`:
`87  23  internal/listener/quic.go`.**

⚠️ **THAT IS A FLOOR, NOT AN ESTIMATE** ([[reference_measured_prototype_is_a_lower_bound]], now
**sixteen** consecutive rows, and at phase 96 it inverted a comparison rather than widening a range).
It excludes every unit arm of §5, the comment reconciliation of §4.5, the fixture of §6 and all
documents.

### 4.1 The shape, in five points

1. **Move `rt.addr = udpConn.LocalAddr().String()` to immediately after the successful
   `net.ListenUDP`**, above the `quicTLSConfig()` call. Without this the Start-time selection sees
   `port_value: 0` for a port-0 listener while the per-connection selection sees the resolved port,
   and a `destination_port` chain would be selected differently at the two moments — **the same class
   of defect being repaired.** §4.6 states the one consequence and the evidence that nothing pins it.
2. **`quicChainMatchInputs(conn *quic.Conn) listenerfilter.ChainMatchInputs`.** Always stamps
   `TransportProtocol: "quic"` and `ApplicationProtocols: []string{"h3"}` (measured: §2 arms F/F2 and
   G/G2). `conn == nil` fills destination from `rt.addr` via `net.ResolveUDPAddr`; `conn != nil` fills
   destination and source from comma-ok-asserted `*net.UDPAddr` and `ServerName` from
   `conn.ConnectionState().TLS.ServerName`.
3. **`selectQUICChain(conn) *chainInfo`** — `listenerfilter.SelectChain(inputs, rt.chainSpecs,
   rt.defaultSpec)`, resolved through `rt.chainByName[spec.Name]`, **nil on error**.
4. **`quicChain` takes the conn** and returns that. `serveQUICConnection`'s existing
   nil-to-`CloseWithError` is **kept** — a nil selection must CLOSE, which is exactly what the
   reference does with `no filter chain found` (§2 arms E and E2).
5. **`quicTLSConfig()` stays connection-independent**, because `quic.Listen` demands one config
   before any connection: it returns the Start-time selection's `tlsCfg`, else the first TLS-bearing
   chain **in `rt.chainSpecs` SLICE ORDER**, else the default slot. ⚠️ **BOTH
   `for _, ci := range rt.chainByName` LOOPS ARE DELETED** — see §3.3.

### 4.2 The arms, measured against the banked reference

Twelve arms; the prototype **AGREES with the reference on eleven** and diverges on one, which is out
of scope. The five arms that diverged before the patch — **A, E, F, G, H** — all flipped to
agreement, and **none of the seven that already agreed regressed**.

| flipped to AGREE | already agreed, unchanged | still DIVERGES |
|---|---|---|
| A, E, F, G, H | C, C2, D, E2, F2, G2 | **B** (D2-QUICTS, §1 OUT, §2.4) |

⚠️ **C AND C2 ARE THE LOAD-BEARING NON-REGRESSIONS.** They are the arms that refute the BRAINSTORM's
`chainSpecs[0]`-first prototype (§0.2). That refutation is now **positively measured**: this shape
keeps the no-match fallback that shape would have destroyed.

⚠️ **THE MEASUREMENT AGENT'S OWN SUMMARY SAID "12 OF 13" AND ITS OWN TABLE SAYS ELEVEN OF TWELVE.**
Counted from its table: twelve arms, eleven `AGREES`, one `DIVERGES`. **Eleven of twelve is the
figure; a summary line is not a measurement, even the measuring agent's own.**

### 4.3 Build gates under the prototype, actual output

`gofmt -l internal/listener/` **empty** · `go build ./...` rc **0** · `go vet ./internal/listener/...`
rc **0** · `go test ./internal/listener/... -count=1` rc **0** via `PIPESTATUS[0]`, all three packages
`ok`. `TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos` was **run by name**
to prove it was not silently skipped: `--- PASS`. It calls `quicTLSConfig()` on a **never-Started**
runtime, so `rt.addr` is the configured string, `net.ResolveUDPAddr` still parses it, the Start-time
selection still runs, and the default slot still comes back.

⚠️ **A GREEN SUITE HERE IS THE COVERAGE FINDING OF §0.6 RESTATED, NOT A PASS.** The same suite is
green under a patch that reverses selection. §5 exists because of that, and §12 NCs it.

### 4.4 The residual this shape leaves — stated, not hidden

Selection is per connection; the **certificate** `quic.Listen` presents is chosen once at Start. When
a per-connection dimension moves the winner to a chain whose leaf differs from the Start-time chain's,
the connection is served by the right filters under the **other chain's certificate**.

⚠️ **THIS RESIDUAL WAS NOT MEASURED** (§2.5: every arm's chains carry the same leaf), and it is
reasoned from the mechanism, not observed. It is the deferred per-connection TLS-identity half, and
§9 banks it with the measurement that would settle it. Note the reconciliation it requires: closing it
means `GetConfigForClient` on the QUIC config, which ADR-0317 `D-ALPNFB-TCPONLY` forbids there for an
unrelated reason and which `manager_test.go`'s existing pin asserts stays nil.

### 4.5 The two comments — RECONCILED BY DELETION, which is owed item 4's answer

Owed item 4 offers two options: enforce the precondition at boot, or delete the claim. **Neither, as
posed.** §0.4 shows the two comments *contradict each other*, and §3.3 shows the map iteration is what
made the precondition load-bearing. **The repair removes the iteration, so the precondition stops
being a precondition** — the code now supports N chains by the mandated algorithm. Both comments are
rewritten to describe the two-moment split of §3.1. **No boot-time chain-count reject is added**;
adding one would refuse configs the reference accepts and serves (§2 arms A, F, G, H).

### 4.6 The one declared behaviour change

Moving the resolve leaves `rt.addr` holding the **resolved** address on `startQUIC`'s two failure
paths (`tlsCfg == nil`, `quic.Listen` error), where it previously held the configured one.
`Manager.Start` interpolates `rt.addr` into `listener: %q: bind %s: %w` on that path, so the boot-error
text changes for a port-0 listener from `:0` to the resolved port.

**Accepted deliberately**, on evidence: the socket **is** bound on both paths, so the resolved address
is the more accurate one; and **no test and no fixture pins either message** — the Start-time
`quic listener has no TLS config (mandatory TLS not built)` reject is pinned by nothing (phase 96's
BRAINSTORM says so and it re-verifies at this tip), and `bind %s` appears in no test or fixture
assertion. The IMPL states this in the commit body rather than leaving it to be discovered.

---

## 5. Unit-test design — arms on BOTH sides of the eligibility conditional

**No helper in `internal/listener` builds the shape this row is about.** `mkQUICListener` and
`mkQUICListenerHCM` set `FilterChains` only; `mkQUICListenerDefaultChain` sets `DefaultFilterChain`
only, with zero `FilterChains` — its own doc says so. **The both-slots shape has never been
constructed in this package**, which is why §0.6's green is possible. The PLAN adds a builder that
takes a `filter_chain_match` and a default-slot flag.

⚠️ **TEST-STRUCTURE FACTS, READ RATHER THAN PRESUMED.** `quic_test.go` and `quic_negative_test.go`
contain **zero** `[]struct{` and **zero** `for _, tc := range` — they have no tables.
`manager_test.go` has six `for _, tc := range` loops and **zero** single-line `[]struct{` matches,
because its tables are written with the brace on the following line — ⚠️ **do not conclude from
`[]struct{` reading zero that a file has no tables; the matcher's vocabulary is wrong, not the
file.** Both QUIC files import `quic-go` and `http3` directly and drive real H3 in-package; neither
imports `test/helpers`, so a driven arm needs no new dependency.

### 5.1 The arm roster — every dimension gets a MATCHED NEGATIVE

⚠️ **A SINGLE POSITIVE ARM CANNOT DISTINGUISH "the input was stamped and matched" FROM "the dimension
is unenforced"** — the same trap §2.3 disarms on the reference side, and a guard that tests one side
of a branch is the vacuous-guard family.

| # | shape | expected selection | what its ABSENCE would let through |
|---|---|---|---|
| a | empty-match indexed **+** default | **indexed** | the headline defect (§2 arm A) |
| b | `destination_port` ineligible **+** default | **default** | a `chainSpecs[0]`-first repair (§0.2) |
| c | `destination_port` ineligible, **no** default | **nil ⇒ connection closed** | §0.3's total non-evaluation |
| d | `transport_protocol: "quic"` **+** default | **indexed** | the stamped constant being absent |
| e | `transport_protocol: "tls"` **+** default | **default** | d passing because the dimension is ignored |
| f | `application_protocols: ["h3"]` **+** default | **indexed** | the stamped ALPN being absent |
| g | `application_protocols: ["h2"]` **+** default | **default** | f passing because the dimension is ignored |
| h | matching `server_names`, **driven H3 connection** | **indexed** | `ServerName` never being read |
| i | non-matching `server_names`, driven | **default** | h passing because `server_names` is ignored |
| j | both-slots listener: `quicTLSConfig()` and `quicChain(conn)` | **the SAME `*chainInfo`** | the accessors silently re-diverging |

⚠️ **ARMS h AND i CANNOT BE TABLE ROWS OVER A NIL CONN.** `ServerName` arrives only with a real
connection, so those two must **drive** — `mgr.Start`, dial H3, assert which chain served. A
nil-conn arm asserting `server_names` behaviour would be an invented input no production path
produces.

⚠️ **ARM j IS THE ONE THAT PINS THE ROW'S CHARTER.** Arms a-i pin ordering; only j pins that the two
accessors name one chain. It must assert **pointer identity**, not that both are non-nil.

### 5.2 Assertion discipline

Per-property `t.Errorf`, never `t.Fatalf` — a `Fatalf` makes every later assertion in the arm dead
code. `Fatalf` is reserved for a broken precondition (the listener failed to build, the dial failed),
where continuing would assert against a shape the arm never created. **Each message names its own
property**, because a shared `t.Helper()` body collapses every failure onto one line and the message
becomes the only discriminator. Prefer exact equality (which chain) to a floor (non-nil).

### 5.3 What is deliberately NOT added

- **No `source_type` / `source_prefix_ranges` / `source_ports` arms.** The mechanism covers them —
  `conn.RemoteAddr()` fills both source fields — but **the reference was not measured on them**
  (§2.5), and a cross-side claim on an unmeasured dimension is exactly the hazard this stage's §2.3
  exists to avoid. Banked at §9.
- **No per-chain distinct-certificate arm.** That is §4.4's residual and it needs a reference
  measurement first.
- **No boot-reject arm for a multi-chain QUIC listener** — §4.5 decides not to add such a reject.

---

## 6. Differential fixture `0122-quic-chain-selection` — **CHARTERED**

**Chartered**, answering owed item 5 in the affirmative. `0104-http3-downstream-get` is the only QUIC
fixture in the tree and builds a single `filter_chains[0]` with no default slot, so **no existing
fixture can reach the disagreement** — measured: the set of fixtures mentioning `default_filter_chain`
and the set mentioning `quic_options` have an **EMPTY intersection**.

### 6.1 Port — **`15122`**, and the band is CENSUSED rather than inherited

⚠️ **THE `10<index>` BAND DOES NOT COVER THIS FAMILY, AND THE ROUTER'S QUIC-BAND CLAIM IS FALSE AS
STATED** (§0.8). Censused at this tip: **28 distinct `15xxx` literals** live under
`test/fixtures/*/driver` and `*/inputs`, spanning `15000`-`15104` plus `15360`. `15104` is the only
one at or above `15100`. The family convention that produced it is `15000 + <fixture index>` — a
**derived observation no document states**, so the PLAN records it rather than assuming it.
`15122` is **FREE**: `grep -rn '15122' test/` reads **0** occurrences.

### 6.2 Shape — ONE listener, the ELIGIBLE arm only

`filter_chains[0]` with **no** `filter_chain_match`, its own `envoy.transport_sockets.quic`
transport socket and HCM `stat_prefix: chain_indexed`, `direct_response` body distinguishing it;
plus a `default_filter_chain` with its **own** QUIC transport socket and `stat_prefix: chain_default`.
Config shape copied from `0104`'s `referenceTmpl` / `subjectTmpl`.

⚠️ **THE INELIGIBLE ARM DOES NOT RIDE IN THIS FIXTURE, AND THE REASON IS STRUCTURAL, NOT EDITORIAL.**
One fixture directory dispatches to exactly one runner branch, so a second arm needs a second
listener — and `0104` is the sole implementor of `fixture.ReferenceListenerIsUDP`, which is the
**single-port** form. Whether the harness supports **two UDP listeners** in one fixture is
**UNVERIFIED**. The ineligible arm therefore lives in §5's unit arms b and c, which can build any
shape freely. **The PLAN must verify the multi-UDP question before any future row assumes it.**

### 6.3 Assertions — a NAMED SUBSET, never a whole map

⚠️ **A WHOLE-MAP COMPARISON WOULD REDDEN AGAINST CORRECT CODE.** Measured on arm A: the reference
emits **156** lines across the two HCM scopes (`downstream_cx_*`, `rq_direct_response`, `no_route`,
`tracing.*`, histograms) while envoy-go emits **10** — exactly
`http.{indexed,default}.downstream_rq_{2xx,3xx,4xx,5xx,total}`. The reference **additionally** emits a
listener-qualified `listener.<addr>.http.<prefix>.*` scope, **12** lines, that envoy-go does not emit
at all. ⚠️ **AND THE LISTENER ADDRESS TOKEN DIFFERS CROSS-SIDE** even where both emit a listener
scope — `0.0.0.0_…` on the reference against `127_0_0_1_…` on the subject — so any assertion keyed on
the full name is cross-side infeasible.

⇒ Pin exactly two names per chain, on both sides: `http.chain_indexed.downstream_rq_total` **>= 1**
and `http.chain_default.downstream_rq_total` **== 0**, plus the response body via the runner's
byte comparison. ⚠️ **`HTTPExpectations` IS TCP-ONLY** — an H3 arm drives through the driver's own
hooks, on the `0104` precedent.

⚠️ **`BackendCount()` MUST BE >= 1** even though the route is a pure `direct_response`: the runner
rejects 0, and envoy-go boot-rejects a config with no `clusters` key, so the subject template carries
a throwaway static cluster the route never references. Both constraints are `0104`'s, inherited by
measurement.

### 6.4 The FOUR registration gates — and the one that fails without even a skip line

§0.9 measured four, not three: `RegisterFixture` in the driver `init()`; the blank import in
`test/differential/runner_test.go`; **byte-identity between the directory name and the registered
string**; and the `NNNN-` directory-name shape `discoverFixtures` enumerates. **Gates 1-3 produce a
`t.Skipf` — a SKIP, not a FAIL — and gate 4 produces no subtest at all.** The PLAN asserts the
fixture set BY NAME in both `comm` directions after adding it (**123 -> 124**), and NCs the extractor
by renaming one import in a scratch copy. ⚠️ **A COUNT-ONLY CHECK IS VACUOUS — the import count is
invariant under a rename** — and ⚠️ **a pure DELETION fires only `comm -23`**, so "both directions"
belongs to the rename control alone.

---

## 7. Gates

⚠️ **A SPEC RUNS NONE OF THE SIX GATES. THAT IS SCOPE, NOT OMISSION** — they belong to the IMPL. What
this SPEC owes is the posture the IMPL must hit, and the departures it must name rather than claim
compliance with.

- **(a) Differential** — the full suite, `-count=1` (**not optional**: the harness's failure mode is a
  silent pass), with the fixture set asserted BY NAME in both `comm` directions, **123 -> 124**.
- **(b) Non-Docker sweep** — gated on `PIPESTATUS[0]` plus a SET RECONCILIATION, over
  `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`. ⚠️ **BOTH Docker
  drivers must be excluded**, and a `[setup failed]` from a package selector that does not resolve
  reads as a real FAIL.
- **(c) h2spec** · **(d) fuzzers** · **(f)** no `REVIEW.md` — standing departure.
- **(e) The ANCHORED panic gate**, `^panic:|DATA RACE|SIGSEGV`, and ⚠️ **PROVEN LIVE**, not merely
  read as 0. There is no `recover()` in non-test `internal/listener` or `internal/tls`, so a panic
  aborts the binary — which is what makes the reachability control of §0.6 work at all.

⚠️ **`-race` ON THE DIFFERENTIAL SUITE IS VACUOUS** — the subject there is an unraced subprocess. Run
`-race` on the **full `internal/listener` package**, not on a `-run`-narrowed selection: a selector
matching nothing prints `[no tests to run]` and **exits 0**.

⚠️ **`go test` WITHOUT `-v` PRINTS ZERO `=== RUN`**, so `RUN=0` beside `RC=0` is a vacuous green; and
on `-v` output an unanchored `grep -c FAIL` reads nonzero on a fully green tree — use
`grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`.

---

## 8. `ADR-0319` — §Context drafted HERE, §Decision and §Consequences at the IMPL

**`ADR-0319` is the next-free id, TAIL-derived** — `grep -oE '^## ADR-[0-9]+' … | tail -1` reads
`## ADR-0318`, and `grep -c '^## ADR-0319'` reads **0**. ⚠️ **NEVER DERIVE NEXT-FREE FROM THE HEADING
COUNT**: the id space is sparse at the single `0209` gap, so heading arithmetic yields a TAKEN id.
⚠️ **AND THE HEADING REGEX ITSELF HAS A HOLE** — `^## ADR-[0-9]+[:—]` misses `## ADR-0127 v2: …`,
which carries a version suffix between the id and the colon.

**The house form, measured rather than inherited** (§0.10: ADR-0044 does **not** contain this
discipline): a `## ADR-NNNN — <title> (phase N)` heading; a one-line `> **STATUS: PROPOSED — …`
blockquote; `### Context (drafted at the phase-97 SPEC)`; the numbered `**§Context ¶K — …**`
paragraphs; and a **RETAINED** italic footer `*§Decision and §Consequences follow at the phase-97
IMPL.*` which the IMPL appends **after**, never replaces. **No `**Status:**` line, no renumber, and
NO `---` separator** — `^---$` does not move.

⚠️ **THIS BLOCK RE-ARMS THE HOUSE `PROPOSED` GUARD, WHICH CURRENTLY READS ITS RESTING VALUE.** The
guard is disarmed at every IMPL and re-armed at every SPEC; that cycle was verified by reading the
phase-96 SPEC commit, where it is armed, against HEAD, where it is not. ⚠️ **NO COUNT OF EITHER
`PROPOSED` MATCHER IS WRITTEN ANYWHERE IN PROSE THE GREP MATCHES** — this SPEC's own landing would
falsify any figure it named, and the phase-93 SPEC demonstrated that by doing it. ⚠️ **The ADR-0231
decoy at `DECISIONS.md:14866` is a DIFFERENT MATCHER** (`^\*\*Status:\*\* PROPOSED`), resolves by
backward heading search to `## ADR-0231`, and is **BYTE-UNTOUCHED** by this row. **Verify by LINE and
by ADR, never by the count alone.**

**§Context outline, eleven paragraphs:** ¶1 the defect and the alternative ADR-0080 rejects · ¶2
**THE REFERENCE MEASUREMENT CARRIED INTO A GOVERNING DOCUMENT** (a measurement that governs a
decision must reach one, and a BRAINSTORM is not one) · ¶3 the defect is total non-evaluation, not
only inversion · ¶4 the root cause: reaching around `SelectChain` into a map that contains the
default slot · ¶5 the two accessors' two moments, and why that split is load-bearing · ¶6 the fix
shape, measured against eleven arms, and the prototype it refutes · ¶7 `ServerName` IS knowable per
connection · ¶8 `transport_protocol: "quic"` parses and matches nothing · ¶9 the contradicting
comments and the precondition nothing enforces · ¶10 what is banked with its measurement · ¶11 what
this ADR does not decide.

---

## 9. What this SPEC does not decide

- **Per-connection TLS *identity* dispatch** — §4.4's residual. Bounded by §2 arm H on the reference
  side and **unmeasured** on the certificate axis (§2.5). Closing it means `GetConfigForClient` on the
  QUIC config, which ADR-0317 `D-ALPNFB-TCPONLY` forbids there for an unrelated reason and which an
  existing pin asserts stays nil. **That reconciliation is the deferred row's first task.**
- **D2-QUICTS** — reference-confirmed at §2.4 with its exact message. Fails closed; banked.
- **The `source_type` / `source_prefix_ranges` / `source_ports` dimensions on QUIC** — the mechanism
  covers them, the reference was not measured on them, and no arm asserts them (§5.3).
- **`parseChainSpec`'s `transport_protocol` over-strictness** — it rejects any value outside four
  literals while the proto field is an unconstrained string. **UNMEASURED against a live reference.**
- **The HCM `stat_prefix` duplicate-registration panic** — the strongest banked candidate, measured on
  both sides at `BRAINSTORM.md` §4.1. Its own row, and probably a split.
- **The `0066` / `0067` / `0068` / `0071` dead-port false-green** — a silent wrong PASS, a worse
  failure mode than the banked port race, and still unchartered.
- **ADR-0025's stale `Accepted` status** (§0.4) and **the ADR-0044 misattribution** (§0.10) — both
  recorded, neither repaired here.

---

## 10. Counts, re-derived at THIS stage's own tip

Every figure below was produced by running its command in the commit that ships these edits, not
copied forward. ⚠️ **A SECTION HEADED "RE-DERIVED AT THIS STAGE'S OWN TIP" IS FALSE IF IT NAMES A
COMMIT THAT IS NOT THE ONE IT SHIPS ON** — a stage's own edits are part of its tip.

### 10.1 Figures this stage MOVED

| file | before | after | note |
|---|---|---|---|
| `DECISIONS.md` | 19050 | **19080** | **+30**, the ADR-0319 §Context block — the SAME delta as all three precedent SPEC commits |
| `^---$` | 216 | **216** | ⚠️ **UNMOVED** — the block adds no separator |
| `^## ADR-` | 317 | **318** | |
| bare `^## ` | 325 | **326** | §Context is a `###` |
| ADR tail | ADR-0318 | **ADR-0319** | next-free **ADR-0320**, `grep -c '^## ADR-0320'` reads **0** |
| `STATE_HISTORY.md` | 564 | **566** | raw delta **+2** — a blank line PLUS the entry line |
| archive strict | 163 | **163** | ⚠️ **DELTA 0**, as a correctly-shaped parenthetical append must be |
| archive parenthetical | 69 | **70** | |
| archive loose | 232 | **233** | **163 + 70 = 233** exactly, under the anchored-occurrence forms |
| `STATE.md` | 65 | **65** | rolled IN PLACE, net 0 |
| `SPEC.md` | — | **874** | new |
| the house `PROPOSED` guard | its resting value | **re-armed**, one hit resolving by backward heading search to `## ADR-0319` | ⚠️ **NO COUNT NAMED** (§8) |

### 10.2 Figures this stage did NOT move — and the scope was MEASURED, not inferred

`ROADMAP.md` **247** lines / **129** data rows / row 97 `in-progress` at file line **159** ·
`BEHAVIOR_CONTRACT.md` **5991** · both **BYTE-UNTOUCHED**.

That scope is measured, not read off the router's wording: `git show --numstat` on the phase-94, -95
and -96 SPEC commits (`307f2e3d`, `9ed6a620`, `da6ea191`) shows each touching **exactly five files** —
`DECISIONS.md`, `STATE.md`, `STATE_HISTORY.md`, its own `SPEC.md`, and `next-prompt.txt` — **and
neither `ROADMAP.md` nor `BEHAVIOR_CONTRACT.md` appears in any of the three.** `DECISIONS.md` reads
`30 0` in **all three**. ⚠️ **The anchored locate form was reconciled against the loose form for each
phase**; the anchored set is a strict subset and nothing is lost here, but see §0.11 for where that
anchor **is** blind.

### 10.3 Unmoved elsewhere

`BRAINSTORM.md` **396** · phase dirs **138** · fixtures **123** (`ls -d test/fixtures/*/ | wc -l`;
⚠️ a `^[0-9]{4}-` character class reads 121, dropping `0007a`/`0007b`), tail
`0121-listener-default-chain-tls`, **`0122` FREE** · blank-import extractor **123 = 123**, both `comm`
directions EMPTY, split **99 `driver/` + 24 `inputs/`** · fuzzers **56 targets / 48 FILES** — ⚠️ **48 is
FILES and 56 is TARGETS; six files carry more than one, and two targets live outside `*fuzz*`
filenames** · `go.mod` **67** require entries under a structural `awk` extractor — ⚠️ **the router's
named character-class form `^\s+[a-z0-9./-]+ v[0-9]` reads 62 at this tip, exactly as method note 33
says (§0.7)** · `go list ./...` **239**, **237** excluding the two Docker drivers · BackendKind tail
**38** · `-family row` **96 occurrences / 68 lines** (⚠️ pass `--` before the pattern) ·
`internal/listener/quic.go` **168** · `internal/listener/manager.go` **1649**.

**Anticipated axis deltas for the IMPL, each an ANTICIPATION and not a measurement:** stat NAMES
**+0** · fixtures **123 -> 124** · BackendKinds **+0** · fuzzers **+0** (the row consumes no new
config field, so there is no new parse arm) · `go.mod` modules **+0** · new packages **0** · ADRs
**+1**, completed in place at the IMPL.

**Anchors this row will move**, to be re-located by LITERAL text and never by a scalar shift
([[reference_line_shift_after_insert_is_banded]]): every `quic.go` line number in this document and
in `BRAINSTORM.md`. ⚠️ **A MULTI-INSERT LINE SHIFT IS BANDED, NOT A CONSTANT.**

---

## 11. The pinned edit map — WHICH STAGE LANDS WHAT

**This SPEC lands five files, the measured precedent scope of §10.2:** `SPEC.md`, `DECISIONS.md`
(ADR-0319 §Context), `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`.

**The PLAN lands `PLAN.md` only** — no `.go`, no `ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no
`DECISIONS.md`.

**The IMPL lands this roster:**

| # | file | edit |
|---|---|---|
| 1 | `internal/listener/quic.go` | the resolve move, the inputs builder, the selector, both accessors (§4.1) |
| 2 | `internal/listener/quic.go` | both accessor doc-comments rewritten to the two-moment split (§4.5) |
| 3 | `internal/listener/manager_test.go` | a both-slots QUIC listener builder (§5) |
| 4 | `internal/listener/quic_test.go` | arms a-g and j (§5.1) |
| 5 | `internal/listener/quic_test.go` | driven arms h and i — these need `mgr.Start` and a real dial |
| 6 | `test/fixtures/0122-quic-chain-selection/**` | the new fixture (§6) |
| 7 | `test/differential/runner_test.go` | the blank import — **the gate that is silently green if missed** |
| 8 | `docs/envoy-go/DECISIONS.md` | ADR-0319 §Decision + §Consequences appended in place after the RETAINED italic footer; STATUS `PROPOSED` -> `ACCEPTED`; **no renumber, no `---`** |
| 9 | `docs/envoy-go/ROADMAP.md` | row 97 -> `done` |
| 10 | `docs/envoy-go/phases/97-.../PROGRESS.md` | new |

**Byte-untouched set, to be asserted by `sha256sum` at the IMPL and set-differenced against the roster
above:** `internal/listener/manager.go` (⚠️ **no boot reject is added — §4.5**),
`internal/listener/listenerfilter/**`, `internal/tls/**`, `internal/stats/**`, and
`test/fixtures/0104-http3-downstream-get/**`.

⚠️ **`BEHAVIOR_CONTRACT.md` IS NOT ON THE IMPL ROSTER EITHER.** The row lands **+0 stat names** — it
changes which chain serves, not which counters exist. Whether a `+0, UNCHANGED` ledger entry is owed
is a question for the IMPL against the phase-96 precedent, and this SPEC does not decide it.

⚠️ **AN UNESCAPED `|` IN THE ROADMAP ROW PASSES CHECK (1) AND SILENTLY BREAKS THE FIELD COUNT.** Count
fields (want **8**) under BOTH the naive and the escape-aware form, BEFORE and AFTER the row-97 flip,
and **reword a pipe away rather than escaping it**. This fired against the phase-96 IMPL's own author,
who wrote a Go operator into a summary cell. ⚠️ **DORMANT at this stage, which edits no row — LIVE at
the IMPL, which flips one.**

⚠️ **NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW.** Also dormant here, live at the
IMPL.

---

## 12. Negative-control roster — neutralise, never revert

⚠️ **BEFORE RUNNING AN NC, NAME THE MECHANISM THAT WOULD CARRY ITS MUTATION TO A FAILURE.** Phase 96
found two roster rows whose specified mutation was structurally incapable of reddening. **If you
cannot name one, the control is vacuous.** Neutralise so the package still compiles, and check the NC
is EXECUTABLE — a `-run` selector matching nothing prints `[no tests to run]` and **exits 0**.

| # | neutralise | arm that must redden | mechanism |
|---|---|---|---|
| 1 | make the selector prefer `rt.defaultChain` first again | **a** | the tip's own behaviour — this is the un-fix |
| 2 | make the selector prefer `rt.chainSpecs[0]` unconditionally | **b, c** | the BRAINSTORM's refuted shape (§0.2) |
| 3 | drop `TransportProtocol: "quic"` from the inputs | **d** | ⚠️ e must STAY GREEN — if e also reddens the arm pair is not isolating |
| 4 | drop `ApplicationProtocols: ["h3"]` from the inputs | **f** | ⚠️ g must STAY GREEN, same reason |
| 5 | drop `ServerName` from the per-connection inputs | **h** | ⚠️ i must STAY GREEN — i passes under a blank SNI too, which is exactly why h is the discriminating arm |
| 6 | revert the `rt.addr` resolve move | the port-0 arm of **b** | Start-time selection sees `:0` and a `destination_port` chain flips |
| 7 | make `quicTLSConfig()` return the default slot unconditionally | **j** | the accessors re-diverge |
| 8 | delete the fixture's blank import from `runner_test.go` | the fixture-set name assertion | ⚠️ **the suite stays GREEN — this NC fires only on the by-name check, never on a pass/fail** |

⚠️ **ROW 8 IS THE ONE MOST LIKELY TO BE MIS-SCORED.** Its mutation produces a `t.Skipf`, so a run that
reports rc=0 with no FAIL line is exactly what a missing gate looks like. **Score it on the fixture-set
set-difference, not on the exit code.**

⚠️ **AN NC THAT LEAVES A CONTROL GREEN IS NOT EVIDENCE THE CONTROL DOES ANY WORK** — rows 3, 4 and 5
each carry a companion that must stay green, and a run where BOTH members redden means the pair is
not isolating what it claims.

⚠️ **A `+0/+0` ARM CAN BE SILENTLY DELETED WITH EVERY GATE STAYING GREEN.** The IMPL diffs the **arm
roster**, a-j and 1-8, not the counters.

---

## 13. Sentinel — RUN MECHANICALLY AT THIS STAGE'S OWN TIP, ACTUAL OUTPUT

A SPEC edits no `ROADMAP.md` row, so the shapes **should** stay — and *should stay* is still a
measurement, so all three checks, all four NCs and the check-(2) positive control were RUN.

### 13.1 The three checks

- **check (1)**, `want=129`: **ONE** line, `NOT DONE: row 97`. Non-silent is the NORMAL mid-phase
  state — this row is registered `in-progress` and goes silent again at the IMPL.
- **check (2)**: **SIX**, at `:207 :213 :219 :229 :235 :243`.
- **check (3)**: **SILENT**.

### 13.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

- **NC-A** (row 62 doctored in a scratch copy, `want=129`): substitution inspected first —
  `NC LANDED? [ in-progress ]` — then **TWO** lines, `NOT DONE: row 62` and `NOT DONE: row 97`.
- **NC-B** (`want=128` on the real file): **TWO** lines, `NOT DONE: row 97` and
  `GATE FAIL: examined 129 data rows, expected 128`.
- **NC-C**: residual **0**, and `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D**: `-family row` **96** occurrences / **68** lines, with `--` before the pattern.
- **check-(2) positive control**: **BOTH** phrases substituted and the substitution ASSERTED rather
  than assumed — residual **0**, `candidatesXX` **6**. ⚠️ **Substituting only the shorter phrase
  leaves a residual of 5 and reads like a finding**; the longer phrase does not contain it.

**Per-line md5 of the six windows, trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12
hex) — ⚠️ **the digest is METHOD-SENSITIVE and the method is stated because of it** — **ALL SIX
BYTE-IDENTICAL to the phase-97 BRAINSTORM close:**

`207 10d7807bf02d` · `213 4a92f7e62fc6` · `219 2a7eb298b9fd` · `229 242e53c6f7a3` ·
`235 b2680e6f4fbf` · `243 6caa1c3ce0e7`

**The escape-aware malformed set is UNCHANGED at exactly two rows** — file line **119** (row **57**,
NF **9**) and **131** (row **69**, NF **10**). ⚠️ **The correct form passes NO file argument to `awk`;
passing one makes `awk` ignore stdin and print the NAIVE count under the escape-aware label.**

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent
at the git root **and** in this stage's worktree.

⚠️⚠️ **THE MARGIN IS STILL ONE.** Checks (1) and (3) carry no structural weight — (1) is merely
borrowing a voice from this open row. **Only check (2)'s SIX stands between this project and `stop`.
Do not tidy a candidate line, and do not "fix" the six.** ⚠️ **Check (2) matches the WHOLE FILE**, so a
row-97 summary cell spelling either phrase at the IMPL would mint a seventh hit and read as a finding.

⚠️ **THE STALE PRESENT-TENSE CLAIM INSIDE WINDOW `:229` IS RECORDED AND DELIBERATELY UNFIXED.** It
calls the duplicate-`stat_prefix` panic a swallowed-panic boot hang; row 78 fixed that and the
phase-97 BRAINSTORM re-measured it as rc=2 in 0 seconds. Repairing it means editing a line the
sentinel matches on a margin of one. ⚠️ **A MEASUREMENT AGENT ONCE REPORTED THAT CLAIM ABSENT AND WAS
WRONG — it searched for the panic MESSAGE while the line spells the CONFIG FIELD.**

---

## 14. What the PLAN owes

1. **Derive the task count itself.** This SPEC deliberately quotes none. Evaluate the BOOTSTRAP §6.1
   split gate (**~25 tasks OR ~1500 LoC**) and record the evaluation, tripped or not.
2. **Order the spine so the un-fixed tip is SPENT as the negative control.** NC roster rows 1 and 2
   are only cheap while the tip is still un-fixed; the phase-96 PLAN's ordering is the precedent.
3. **Verify the multi-UDP-listener question** (§6.2) before any future row assumes it, and record the
   answer whichever way it goes.
4. **Read the tests before writing tasks against them.** §5's structure figures are measured; ⚠️ **the
   `[]struct{` matcher reads zero on a file that HAS tables**, so re-derive with a matcher that sees a
   brace on the following line.
5. **Re-derive every figure in this document at the PLAN's own publishing commit.** These are true at
   this stage's tip and expire when the PLAN edits anything.
6. **Do NOT inherit the NC shapes.** A PLAN adds no ROADMAP row and flips none, so `want` **129**,
   NC-A and NC-B each **TWO** — but *should stay* is still a measurement.
7. **Land `PLAN.md` only** (§11). `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` stay
   byte-untouched; a PLAN adds no ADR, so the house `PROPOSED` guard stays **ARMED** through it.
8. **Carry §2's reference table forward intact.** It cost a Docker-serialized agent three batches and
   must not be re-derived. ⚠️ **And carry §2.5 with it** — the arms that were NOT run are part of the
   measurement, not a footnote to it.
