# Phase 96 — `listener-default-chain-tlsmode` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Subject: SELF-PICKED** under the 2026-07-12
standing directive; no human was consulted. The pick and the rejected alternatives are recorded in §1
and §4, and the pick is justified against the banked list rather than inherited from the router's
recommendation — which this stage **partly refutes**: the router named the right *file* and the wrong
*member* of the class it points at.

**One sentence.** A `default_filter_chain` carrying a `transport_socket` on a listener whose
`filter_chains[]` are plaintext or absent leaves `listenerRuntime.tlsMode` **false**, so the five
`ssl.*` counters are never registered, while the per-connection Inc guard is a *different* predicate
that is **true** — and the first completed downstream TLS handshake dereferences a nil `*stats.Counter`
and **SIGSEGVs the process**, remotely, with no `recover()` anywhere on the path.

---

## 0. What this stage refuted

Every stage owes an execution-backed refutation of its predecessor. This one produced **thirteen**,
and the four load-bearing ones are §0.1, §0.3, §0.5 and §0.10. Three concern documents this session
was explicitly told to trust.

### 0.1 ⚠️ ROW 95 SHIPPED A FALSE LINE COUNT IN THE COMMIT THAT WROTE IT — while convicting its own PLAN of the identical error

`ROADMAP.md` row 95 states the fallback landed in `internal/tls/config.go` **`582 -> 642`**, and later
in the same row: *"`config.go` 629 prediction is STALE (it landed 642)"*. Measured at every phase-95
commit:

```
7f568db2 BRAINSTORM  config.go=582
9ed6a620 SPEC        config.go=582
f647dd72 PLAN        config.go=582
0f3b98b3 IMPL        config.go=666   <- the commit that WROTE "642"
8ac19ab4 CORRECTION  config.go=666   (git show --numstat: next-prompt.txt 5/5, ONE file)
```

The file was **666** at `0f3b98b3`. This is not staleness by drift — **it was wrong at birth**, inside
the row whose entire subject was a false present-tense comment, in the same sentence that convicts its
predecessor of exactly this. ⚠️ **A CORRECTION IS NOT A MEASUREMENT.** Row 95 replaced 629 with 642 by
reasoning about which prediction was newer, not by running `wc -l` at the publishing tip.

### 0.2 ⚠️ THE ROUTER CONTRADICTS THE ROW IT CLOSED, AND BOTH MISLABEL THE MEASURE

`next-prompt.txt`'s LIVE FIGURES block reads `:1944` **357** chars. Row 95 says of the same line:
*"the `:1944` redemption being WITHIN-LINE (357 -> 1841 chars)"*. They cannot both be post-close.

```
sed -n '1944p' docs/envoy-go/BEHAVIOR_CONTRACT.md | tr -d '\n' | wc -c   ->  1841   BYTES
sed -n '1944p' docs/envoy-go/BEHAVIOR_CONTRACT.md | tr -d '\n' | wc -m   ->  1830   CHARS
```

The router carried the **pre-edit** figure into a block whose whole purpose is post-close live values;
and the row's `1841` is a **BYTE** count wearing the label *chars*. The siblings are not uniform
either — `:1971`'s published **5098** is a CHAR count (bytes **5167**); `:1961`'s **736** is
measure-agnostic. ⚠️ **A THREE-FIGURE GATE ON THESE LINES WOULD FIRE AGAINST CORRECT CODE ON TWO OF
THE THREE.** The archive guard's "name the form or the figure is meaningless" rule extends to
character counts.

### 0.3 ⚠️ THE BANKED SUBJECT'S SAFETY VERDICT IS TRUE OF ONE SHAPE AND FALSE OF THE CLASS

`PROGRESS.md` §9 F-E banks this row's recommended subject and rules it benign: *"⚠️ **NOT a safety
hole** — `internal/listener/quic.go` catches the nil config at Start … before `quic.Listen`."* The
phase-95 controller and its scoped re-reviewer measured that **independently and agreed**.

They enumerated **one config**. The class has at least two, and the Start-time net does not catch the
second — §2.3. This is the phase-95 lesson recurring one level up: F1 was *"true of the SYMBOL, false
of the PATH"*; F-E is **true of the SHAPE, false of the CLASS**. ⚠️ **TWO INDEPENDENT AGREEING
MEASUREMENTS DO NOT MAKE A CLAIM GENERAL WHEN BOTH ENUMERATED THE SAME SINGLE CONFIG.**

### 0.4 ⚠️ `internal/listener/quic.go`'s TWO CHAIN ACCESSORS DISAGREE WITH EACH OTHER

`quicTLSConfig()` (`:57` — `if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil {`) guards on
TLS presence and falls through to `chainByName`; `quicChain()` (`:75` — `if rt.defaultChain != nil {`)
returns the default chain **unconditionally**. Neither comment mentions the other's predicate. Where a
listener carries both slots the two accessors name **different chains**, and the accessor that decides
whether the listener boots is not the accessor that decides what serves it.

### 0.5 ⚠️ THE TREE STATES THE FALSE INVARIANT IN PROSE, AND ADR-0080 REPEALED ITS PREMISE SIX YEARS OF PHASES AGO

`internal/listener/manager_test.go:2325-2332` asserts, as the justification for the whole
registration/Inc design:

> *"the REGISTRATION gate (`rt.tlsMode`) and the Inc guard (`selected.tlsCfg != nil`) are EQUIVALENT,
> because a listener is all-TLS or all-plaintext and never both (`manager.go:575`, ADR-0033 cl.5 /
> ADR-0078 cl.5)."*

ADR-0080 §Decision 3 explicitly exempts the default slot from that uniformity rule:

> *"`default_filter_chain` may carry an independent `transport_socket` (TLS or plaintext) **regardless
> of the `filter_chains[]` entries' TLS posture** … a structurally-separate slot and is NOT subject to
> the cross-chain TLS uniformity rule."*

**The premise was repealed at phase 07.2 and the invariant it supports was never revisited.** The
comment is not merely stale — it is the reason the defect went unnoticed through phases 74, 75 and 94,
each of which added a counter under the same gate.

### 0.6 ⚠️ THE GUARD TEST THAT EXISTS TO CATCH THIS IS VACUOUS ON THE ONLY ARM THAT MATTERS

`TestListenerMetrics_GateMatchesInc` (`manager_test.go:2341`) is the load-bearing pointer test. All
three arms build **`filter_chains[]`-only** listeners, so its `defaultChain`-conditioned assertions
never execute — `defaultChain` is nil in every fixture. A test named for the equivalence of the two
predicates **does not exercise the input on which they differ**.

### 0.7 ⚠️ THE BANKED "`NegotiatedProtocol` IS THE ONLY SUCH ASSERTION IN 122 FIXTURES" IS FALSE

Re-derived: **four assertion sites across three fixtures** — `0004-h2-routing/driver/driver.go:962`
and `:1228`, `0119-grpc-unary-trailers/driver/driver.go:468`, `0120-tls-connection-error/driver/driver.go:310`.
`0120` is only distinguished by *panicking* on mismatch where 0004/0119 return a `READ-ERR` string.
The banked class was **narrowed by phase 95 into a claim that was already false when written**. Struck
from the candidate list (§4.7).

### 0.8 ⚠️ THE BANKED PORT-RACE COST IS WRONG IN ITS COUNT, ITS SPLIT, *AND* ITS PATH

Banked as *"~36 driver files under `test/differential/`, 30 `panic` + four `fmt.Errorf`"*. Measured:
**37 files** — 26 TCP probe+rebind + 4 UDP (`ResolveUDPAddr`, invisible to a TCP-only axis) + 1
source-bind (`0008`) + 6 under `inputs/` — split **31 panic / 5 `fmt.Errorf` / 1 other**. And the
path is wrong: `find test/differential -name '*.go'` reads **9**; the drivers live under
`test/fixtures/*/driver/` and `*/inputs/`. The prior figure came from `grep -rn 'driver: start' | wc -l`
(**39**) — a *line* count over a superset. ⚠️ **THE MEMORY NOTE'S OWN CORRECTION (14 -> ~36) WAS ITSELF
UNCORRECTED.**

### 0.9 ⚠️ ONE OF THE FOUR BANKED "STALE CITATIONS" IS CORRECT, THE CLAIM'S OWN ANCHOR IS WRONG, AND A FIFTH IS NEW

`manager_test.go`'s `(:653)` cite is **NOT stale** — it lands on `mkDownstreamTSRequireClientCert`'s
doc-comment first line. The banked claim anchors it at `:4482`; it is at **`:4597`**. Six others *are*
stale, including a newly-found `(:4484)` at `manager_test.go:5571`. Two controls came back clean, so
the file is not uniformly rotten — these are point failures. ⚠️ **A LIST OF STALE CITES CAN ITSELF
CARRY A STALE CITE.**

### 0.10 ⚠️ THE `len(helpText)` GUARD IS A VACUOUS-GUARD TRAP, NOT A CANDIDATE

Banked as a cheap missing guard. `TestHelpText_KeySetExact` (`helptext_test.go:121-145`) already
performs full **set equality in both directions**, reporting `missing` and `extra` separately. A
cardinality guard cannot fail without one of those firing first: `len(helpText) != 31` is
**unreachable**. Adding it would land the phase's fifth vacuous-guard cell. Struck (§4.8).

### 0.11 ⚠️ THE `go.mod` REQUIRE FORM DROPS FIVE ENTRIES BY CHARACTER CLASS

`grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` reads **62**; the true count is **67**. The class excludes
uppercase and `_`, silently dropping exactly `AdaLogics/go-fuzz-headers`, `Azure/go-ansiterm`,
`Microsoft/go-winio`, `Microsoft/hcsshim`, `prometheus/client_model`. 62 + 5 = 67. The
character-class fail-unsafe family, confirmed on a third instrument.

### 0.12 ⚠️ THE REFERENCE REFUTES THE FRAMING THE ROUTER OFFERED FOR ITS OWN RECOMMENDATION

The router bans assuming a divergence is a defect. Measured on the pinned image (§2.4): the reference
**rejects** the QUIC/default-chain/no-`transport_socket` shape at config-init with the **byte-identical**
message it uses for the `filter_chains[]` shape — *"no transport socket specified for connection
oriented UDP listener"*. The reference does not distinguish the two slots at all. So a boot reject there
is **PARITY, not a departure** — and the larger hypothesis (that the reference might refuse
`default_filter_chain` on a UDP listener outright) is **REFUTED**: it accepts it.

### 0.13 ⚠️ AN INCIDENTAL, SEPARATE CRASH FOUND WHILE MEASURING THE REFERENCE

Two listeners sharing an HCM `stat_prefix` — a normal, common Envoy configuration — make envoy-go
**panic** (`rc=2`, `stats: duplicate metric registration: "http.ingress_http.downstream_rq_total"`,
`internal/stats/registry.go:107` via `internal/filter/hcm/config.go:358`), including under
`-mode validate`. The reference **accepts** it. Isolated on a QUIC-free two-listener arm, so it is not
a confound of this row's subject. **Banked with its stack (§4.3); it deserves its own row.**

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

Make `listenerRuntime.tlsMode` account for the `default_filter_chain`'s own TLS posture, so that the
`ssl.*` registration gate and the per-connection Inc guard agree on every config ADR-0080 permits —
closing a remotely-triggerable process crash and the stats-parity gap that shadows it.

### 1.2 Why "smallest defensible" selects it — a trade-off, not a ranking

This candidate is simultaneously the **most severe** and the **smallest** on the board, which is rare
enough to be worth stating as the whole argument:

- **Severity.** It is a *crash*, not a wrong number. Post-boot, remote-triggerable by any client that
  can open a TLS connection, on a config the tree accepts silently. There is no `recover()` in
  non-test `internal/listener`, so it takes the **process**, not the goroutine. Every other live
  candidate is a doc fix, a cosmetic parity gap, or a test-harness race.
- **Size.** The production fix measured **one line, `+1/-1`** (§6.1) — smaller than the port race (37
  files), the SDS×ALPN fixture (a whole new fixture), and the citation sweep (7 sites).
- **Testability.** Deterministic. No Docker, no race, no timing. A unit test builds the listener, dials
  once, and the un-fixed tip SIGSEGVs — the strongest possible negative control, since the NC does not
  merely fail, it aborts the binary.
- **It is where the router pointed, one member over.** The router directed this session at the
  `default_filter_chain`-vs-`filter_chains[]` divergence set in `internal/listener/manager.go` and
  named D2-QUICTS as the member. Enumerating the set first — as the router demanded — found a
  strictly worse member in the same block. **The recommendation was evidence and it paid off; it was
  not an order, and following it literally would have shipped the cosmetic half.**

### 1.3 What this row does NOT buy — stated plainly

- It does **not** fix D2-QUICTS (the QUIC no-`transport_socket` deferred reject) or D10-QUICSEL (the
  accessor split). Both are measured in §3.3 and banked in §4.1/§4.2; the SPEC may fold D2 in, since
  it is ~4 lines and reference-confirmed parity, but this BRAINSTORM does not charter it.
- It does **not** close the `stat_prefix` duplicate-registration panic (§0.13, §4.3).
- It does **not** move the sentinel: check (2) reads **SIX** before and after (§8.5).
- It lands **+0 stat names**. What changes is *whether the five already-named `ssl.*` counters exist on
  a listener that serves TLS only through its default chain. The names themselves are phase-74/75/94
  outputs.

---

## 2. The defect, MEASURED — four arms, executed, twice, independently

### 2.1 The rig and its controls, stated BEFORE the result

Probes were written into this stage's worktree, run, and deleted under `sha256sum -c`. Every arm was
run by the controller **after** a measurement agent reported the same shape, on a probe written from
the code rather than from the agent's report — the two reproductions are independent.

Controls carried, so that "it crashed" is discriminating rather than ambient:

- **ARM 3 (positive control)** — the `filter_chains[]` half of the QUIC parity **must** reject. If it
  does not, the probe harness is broken and no other arm means anything.
- **ARM 4 (confound control)** — the same default-chain-only shape on a **TCP** listener must build.
  If it did not, ARM 1's acceptance would be an artifact of the shape, not of `kind`.
- **The pointers are asserted BEFORE the dial.** `sslHandshake == nil` is the *cause*; the SIGSEGV is
  the *effect*. Asserting only the effect would leave the mechanism unproven
  ([[reference_nil_stats_counter_inc_crashes_goroutine]]).

### 2.2 The result — the crash, reproduced on two distinct config shapes

**Shape A — zero `filter_chains[]`, TLS `default_filter_chain`:**

```
BUILD OK. tlsMode=false  len(chainSpecs)=0  defaultChain.tlsCfg!=nil=true
POINTERS: sslHandshake==nil=true sslNoCertificate==nil=true sslConnectionError==nil=true
DIALING TLS at 127.0.0.1:34289
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x10 pc=0xeb137b]
sync/atomic.(*Uint64).Add(...)
github.com/pgdad/envoy-go/internal/stats.(*Counter).Inc(...)  internal/stats/counter.go:22
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).serveConnection(...)
                                                              internal/listener/manager.go:1393
created by ...acceptLoop in goroutine 50                      internal/listener/manager.go:1268
EXIT=1
```

**Shape B — a PLAINTEXT `filter_chains[]` entry made ineligible by `destination_port`, plus a TLS
`default_filter_chain`.** This is the exact **inverse** of the arrangement ADR-0080 §Consequences (c)
blesses (*"a TLS-only `filter_chains[]` entry coexisting with a plaintext `default_filter_chain`"*),
and it is the operator-realistic shape — a graceful TLS fallback behind port-matched plaintext chains:

```
BUILD OK. tlsMode=false len(chainSpecs)=1 defaultChain.tlsCfg!=nil=true
POINTERS: sslHandshake==nil=true
panic: ... SIGSEGV ... manager.go:1393 ... EXIT=1
```

⚠️ **THE DEFECT IS NOT CONFINED TO THE ZERO-CHAIN SHAPE.** Any listener whose `filter_chains[]` are
all plaintext and whose default chain carries TLS reaches it. The `len(chains) == 0` guard at
`manager.go:573` is not the boundary.

### 2.3 The class, and where the Start-time net does and does not catch it

Four members enumerated, all reachable, each classified by whether an existing guard catches it:

| member | shape | boot | Start | outcome |
|---|---|---|---|---|
| **D1-TLSMODE** | TCP, TLS default chain, no TLS filter chain | ACCEPTS | starts | **SIGSEGV on first TLS conn** |
| **D2-QUICTS** | QUIC, default chain, no `transport_socket` | ACCEPTS | **rejects** (`quic listener has no TLS config`) | fails closed, process exits 1 |
| **D10-QUICSEL** | QUIC, valid QUIC `filter_chains[0]` **plus** a `transport_socket`-less default chain | ACCEPTS | **starts** | serves `filter_chains[0]`'s cert through the **default chain's** filters |
| control | TCP, plaintext default chain | ACCEPTS | starts | correct |

**D10 is the member that refutes F-E** (§0.3). `quicTLSConfig()` falls through to the filter chain, so
the nil-config Start reject **never fires**; `quicChain()` then returns the default chain regardless.
Measured:

```
boot ACCEPTED. tlsMode=true defaultChain.tlsCfg==nil? true
quicTLSConfig()==nil? false     quicChain()==defaultChain? true
Start SUCCEEDED
```

Second-order: on QUIC the default chain becomes an unconditional **override**, inverting ADR-0080
§Decision 1 (*"consulted ONLY when no `filter_chains[]` entry … is eligible"*) and §Decision 2
(*"empty-match chain BEATS `default_filter_chain`"*). `quic.go:68-73` documents single-chain-ness as an
assumption; **nothing enforces it.**

### 2.4 The reference side, measured on the pinned image

Image pinned **by digest** `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`,
verified against `docs/envoy-go/ENVOY_TARGET.md:3-4` before any arm was trusted. A positive control
(QUIC + valid `filter_chains[]` quic transport socket) **ACCEPTED on both sides**, so "accept" is a
reachable outcome of the harness and the rejects are not a broken-config artifact. Accepts were
confirmed by `starting main dispatch loop` / `envoy-go … ready` in the OUTPUT, never by exit code —
`timeout` rc=124 is shared by a healthy server and a hung boot.

| arm | reference | envoy-go | verdict |
|---|---|---|---|
| QUIC, `filter_chains[]` with quic TS (control) | ACCEPT | ACCEPT | AGREE |
| QUIC, default chain, **no** TS | REJECT rc=1 *"no transport socket specified for connection oriented UDP listener"* | REJECT **at Start** *"quic listener has no TLS config (mandatory TLS not built)"* | agree on serve, **DIVERGE on stage** |
| QUIC, default chain, quic TS | ACCEPT | ACCEPT | AGREE |
| QUIC, default chain, **plain tls** TS | REJECT rc=1 | REJECT rc=1 (phase-95 F1 fix) | AGREE |
| QUIC, `filter_chains[0]` no TS (control) | REJECT rc=1 (same message) | REJECT rc=1 | AGREE |
| TCP, default chain, no TS (control) | ACCEPT | ACCEPT | AGREE |

⚠️ **THE DIVERGENCE BITES IN VALIDATE MODE, AND THAT IS THE ONLY EXTERNALLY-OBSERVABLE HALF.**
`envoy-go -mode validate` returns **rc=0 `configuration OK`** for the shape the reference validator
rejects **rc=1**. Serve-mode posture already agrees — envoy-go's Start failure aborts the whole
process (verified with a two-listener probe: rc=1, the port was **not** left bound), so it does not
degrade to "TCP serving, QUIC silently missing". A control plane using validate-mode as an admission
check would pass a bootstrap the reference refuses **and that envoy-go itself will not serve**.

⚠️ **THE REFERENCE USES ONE MESSAGE FOR BOTH SLOTS; envoy-go USES THREE SLOT-QUALIFIED ONES.** No pin
may assume the other side's wording.

### 2.5 Arms NOT run — recorded, not glossed

- **No reference measurement of D1-TLSMODE's stat surface.** Whether the reference emits `ssl.*` on a
  listener that serves TLS only through its default chain is **UNMEASURED**, and it decides whether
  the one-line fix is also the *parity-correct* fix (§6.2). **The SPEC owes this arm.**
- **No reference measurement of D10-QUICSEL's dispatch.** Which chain the reference serves for that
  shape is unknown.
- **No end-to-end QUIC drive of D10.** The accessor split is proven at the accessor level only.

---

## 3. The mechanism, stated precisely

### 3.1 Where the divergence is produced

Three anchors, each quoted by LITERAL text because line numbers rot:

```
internal/listener/manager.go   anyTLS = true                                    (inside the filter_chains[] loop ONLY)
internal/listener/manager.go   tlsMode:                 anyTLS,                 (the single write site)
internal/listener/manager.go   if rt.tlsMode {                                  (registerListenerMetrics — the REGISTRATION gate)
internal/listener/manager.go   if selected.tlsCfg != nil {                      (serveConnection — the INC guard)
```

`anyTLS` is written **only** inside the `filter_chains[]` loop. The `default_filter_chain` block writes
`dfcTLS` and never touches it. The Inc guard is per-connection and consults the **selected** chain,
which may be the default one. The two predicates are equal exactly when the default chain's TLS
posture matches the loop's — which ADR-0080 §Decision 3 explicitly permits it not to.

**Symbol-vs-path check performed** (the phase-95 burn, method note 27): the five counters have **one**
write site, **one** read gate, and **five** Inc sites, all five inside `if selected.tlsCfg != nil` and
none nil-guarded. `(*stats.Counter).Inc` is `func (c *Counter) Inc() { c.v.Add(1) }` — no receiver nil
check — and `internal/listener` non-test carries no `recover()`.

### 3.2 The full divergence set between the two blocks — and MOST OF IT IS CORRECT

The router required this enumeration and warned that a report of *"N divergences, all defects"* has not
read ADR-0080. It is right. Ten divergences; **two** unexplained:

| id | divergence | classification |
|---|---|---|
| **D1-TLSMODE** | default chain's TLS presence sets no `anyTLS` / `tlsMode` | **UNEXPLAINED — this row** |
| **D10-QUICSEL** | `quicChain()` prefers the default chain unconditionally, `quicTLSConfig()` conditionally | **UNEXPLAINED — banked** |
| D2-QUICTS | no QUIC mandatory-TLS reject on the default slot | DELIBERATE, banked at ADR-0317 `D-ALPNFB-TCPONLY` |
| D3-PLAINTLS | default chain called the TCP builder unconditionally on QUIC | **RECENTLY CLOSED** (phase 95 F1) |
| D4-MIXEDTLS | default chain exempt from the mixed TLS+plaintext cross-check | **DELIBERATE — ADR-0080 §Decision 3. CORRECT AS IT STANDS.** |
| D5-CATCHALL | default spec does not feed `catchAllCount` | DELIBERATE — ADR-0080 §Consequences (b) |
| D6-DUP | default spec never enters `findIdenticalChainSpecs` | DELIBERATE — follows from D5 |
| D7-SNI | default chain's `serverNames` is always nil | DELIBERATE — follows from D8 |
| D8-FCM | `dfc.GetFilterChainMatch()` is ignored entirely | DELIBERATE, **proto-mandated** |
| D9-CIS | default chain not appended to `cis` | DELIBERATE — consequence of D4 |

⚠️ **D4 IS NOT THE BUG, AND THE COMMENT AT ITS SITE MUST NOT BE CHANGED.** *"Per ADR-0080:
`default_filter_chain` has an INDEPENDENT TLS posture from `filter_chains[]` — no mixed-TLS-rule
cross-check here"* is correct. The defect is that the **stat-registration flag was derived from the
mixed-TLS cross-check's accumulator** — a variable reused for a second purpose whose invariant it no
longer satisfies. Fixing D4 would be the wrong repair and would break ADR-0080 parity.

D8 is proto-mandated, upstream's own comment (`go-control-plane@v1.37.0`
`config/listener/v3/listener.pb.go:267`): *"The filter chain match is ignored in this field."* No ADR
sentence records it — a documentation gap, not a defect. Residual nit: a *structurally invalid*
`filter_chain_match` inside `default_filter_chain` would be PGV-rejected by the reference and is
silently ignored here. **UNMEASURED against a live reference; recorded, not chartered.**

### 3.3 Hazards the SPEC must carry

1. ⚠️ **THE ONE-LINE FIX CHANGES THE STAT SURFACE OF A LISTENER, AND THAT IS A CROSS-SIDE QUESTION.**
   With the fix, a listener whose only TLS is its default chain registers five `ssl.*` names it does
   not register today. Whether the reference does the same is **UNMEASURED** (§2.5). The fix is
   unambiguously right about the *crash*; it is **unproven** about the *parity*. A SPEC that lands it
   without the reference arm is asserting parity it has not measured.
2. ⚠️ **THE ALTERNATIVE FIX SHAPE IS NOT EQUIVALENT.** Nil-guarding the five Inc sites also stops the
   crash, but leaves the listener emitting **zero** `ssl.*` names — silently choosing the opposite
   parity answer. The two shapes differ only in a stat surface neither has measured. **Measure first,
   then choose; do not pick by diff size.**
3. ⚠️ **`0008-listener-chain-match` IS FULLY PLAINTEXT** (`grep -c transport_socket … envoy-go.yaml`
   reads **0**), so no existing fixture can regress under either shape — and none can catch a
   regression either.
4. **`TestListenerMetrics_GateMatchesInc` must gain a default-chain arm** (§0.6), or the fix ships
   under a guard that still does not exercise it.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 D2-QUICTS, the router's own recommendation — **REJECTED as the subject; may ride at the SPEC's discretion**

Reference-confirmed parity (§2.4), ~4 lines mirroring the arm the loop already carries. Rejected as
*the* subject because it is strictly less severe: it fails **closed** (process exits 1, no port left
bound), so its only externally-observable half is validate-mode rc=0-vs-rc=1. A crash outranks a
validate-mode disagreement. It is in the same block and the same class, so folding it in is cheap;
this stage does not decide that.

### 4.2 D10-QUICSEL — **REJECTED for scope; banked with its measurement**

The accessor split (§2.3) is real, reachable, and unexplained, but repairing it means deciding what
multi-chain QUIC dispatch *should* do — ADR-0080's fallback semantics vs. `quic.go`'s documented
single-chain assumption. That is a design question, not a repair. **Banked; it needs its own ADR.**

### 4.3 The `stat_prefix` duplicate-registration panic — **REJECTED for this row; the strongest banked candidate**

Two listeners sharing an HCM `stat_prefix` panic envoy-go rc=2 where the reference accepts (§0.13).
Boot-time and fails closed, so less severe than D1, but it is a **crash on a common, valid config** and
it reproduces under `-mode validate`. Not folded in: different package (`internal/stats` /
`internal/filter/hcm`), different mechanism, and the fix is a policy decision (scope the name, or
tolerate re-registration) rather than a predicate repair.

### 4.4 The driver-owned receiver port race — **REJECTED for size; cost RE-DERIVED and the banked figure corrected**

**37 files**, split **31 panic / 5 `fmt.Errorf` / 1 other** (§0.8). Uniform mechanism, one shared
helper plus a per-driver swap, but a 37-file blast radius and a *race* that is hard to gate
deterministically. ⚠️ **Its trigger condition has still NOT fired** — it did not recur across phase
95's two full 122/122 runs. Do not open it on the strength of the note.

### 4.5 An SDS × downstream `alpn_protocols` fixture — **REJECTED; the runner-up, and genuinely close**

The intersection **is** empty under every reading, so the gap is real. But the banked set sizes are
both wrong: SDS reads **6** fixtures (**5** scoped to TLS-SDS), and downstream `alpn_protocols` in
*config YAML* reads **2** (`0004`, `0120`), not five — the other four carry ALPN only in driver Go or
README, i.e. the **client's** offer, not a listener setting. **No reading yields five.** Rejected
because it adds coverage rather than fixing a defect, and this board has a live crash on it.

### 4.6 The doc riders — `0108`'s two false *"emits NO `ssl.*` stats whatsoever"* confessions (plus a third soft one at `envoy.yaml:15`), `0118/driver/driver.go:31`'s falsified *"TLS/SDS band"* (the 0108-0113 band is **10444-10449**; 10450 is one **past** its end, not a member), and `0120/expectations.yaml`'s *"Phase 94 fixture"* first line where `README.md` says *"Phase 94, extended by phase 95"* — **all REJECTED as rows; each is 1-3 prose lines.** Carried.

### 4.7 `NegotiatedProtocol` cross-side coverage — **STRUCK from the candidate list**

The gap does not exist as banked (§0.7): four assertion sites, three fixtures.

### 4.8 The `len(helpText)` guard — **STRUCK as a vacuous-guard trap**

Unreachable behind existing set equality (§0.10). The two ungated prose counts at `name.go:517` and
`:531-532` were re-verified and are **currently correct** (31 entries, 31 roster, 5 `ssl.*`;
10+1+1+3+1+10+5 = 31 reconciles), so even the doc half has nothing to repair today.

### 4.9 The `GetConfigForClient` comment consolidation and the stale-citation sweep — **REJECTED as un-gatable**

The comment count is **contested** — 10 under a history-marker classifier, 12 under a wider one — so
**no number is quoted** for it. The citations are 6-stale-of-7 with one banked entry that is actually
correct (§0.9). Both are mechanical prose work no gate can assert. Low defensibility.

### 4.10 The six windows as a pool — **REJECTED, unchanged**

Provenance is outside all six (§8.5); this row makes zero sentinel progress, and that is disclosed
rather than dressed up.

---

## 5. Family attribution

**Core-listener / downstream-TLS MAINTENANCE row claiming NO family ordinal.** A maintenance row
repairs a landed deliverable — here the phase-74/75/94 `ssl.*` counter surface and the phase-07.2
`default_filter_chain` slot — and does not extend a charter; the row-85 through row-91 and row-95
precedent. The Observability ordinal chain is **not** extended: this row lands **+0 stat names** (the
five `ssl.*` names already exist; what changes is whether they are registered on this listener shape).
The HTTP/3, gRPC, xDS, Runtime, Observability and Operational-tooling charters are untouched, so check
(2) must still read **SIX** at close.

---

## 6. The cost FLOOR — a built, run, and reverted prototype, explicitly a LOWER BOUND

### 6.1 Production: ONE line, `+1 / -1`

```
tlsMode:                 anyTLS || (defaultChain != nil && defaultChain.tlsCfg != nil),
```
```
git diff --numstat  ->  1	1	internal/listener/manager.go
```

`defaultChain` is in scope at the composite literal — it is built above it — so no reordering is
needed. With the prototype in place, the shape that SIGSEGV'd:

```
WITH FIX: tlsMode=true sslHandshake==nil=false
SERVED: n=5 err=<nil> payload="hello"
REGISTERED: listener.127_0_0_1_39101.ssl.handshake
REGISTERED: listener.127_0_0_1_39101.ssl.fail_verify_error
REGISTERED: listener.127_0_0_1_39101.ssl.fail_verify_no_cert
REGISTERED: listener.127_0_0_1_39101.ssl.no_certificate
REGISTERED: listener.127_0_0_1_39101.ssl.connection_error
ssl.* names registered = 5
```

Reverted under `sha256sum -c` (`internal/listener/manager.go: OK`); `./internal/listener/` green after
revert.

⚠️ **THIS IS A LOWER BOUND AND HAS BEEN WRONG THIRTEEN CONSECUTIVE ROWS**
([[reference_measured_prototype_is_a_lower_bound]]). It excludes: the reference arm of §3.3 hazard 1,
the default-chain arm `TestListenerMetrics_GateMatchesInc` needs, the false invariant comment at
`manager_test.go:2325-2332` and its sibling at `:4993-4998`, and any `BEHAVIOR_CONTRACT.md` /
`DECISIONS.md` text. **The SPEC must re-derive it, not inherit it.**

### 6.2 The test-pin surface — measured

- The `filter_chains[]` QUIC reject is pinned by **exactly one** test,
  `TestBuildListenerRuntime_QUICMandatoryTLS` (`manager_test.go:959`).
- The Start-time `quic listener has no TLS config` reject is pinned by **NO test at all** — the only
  hit for that literal is the production string in `quic.go`.
- `TestListenerMetrics_GateMatchesInc`'s default-chain assertions are **vacuous** (§0.6).
- `TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain` (`manager_test.go:2731`) covers only the
  **safe** direction (TLS chains + plaintext default). **The crashing inverse is untested.**
- The phase-95 helper `mkQUICListenerDefaultChain` has exactly two callers, both pinning D3 (closed);
  neither pins D1 or D2.

### 6.3 Fixtures: +0 expected

No fixture carries a TLS `default_filter_chain` (`0008` is fully plaintext) and none combines QUIC with
`default_filter_chain`. **The differential cannot see D1, D2 or D10 today**, and a boot-reject/crash
parity is poorly shaped for a suite that asserts served traffic. Expect the pin in unit tests and
`+0` fixtures — but §7 is where the SPEC must argue that, not assume it.

### 6.4 Anticipated counts — every axis re-derived at this tip

`ROADMAP.md` **245** lines / **127** rows (**246 / 128** after this row's ADD) · `DECISIONS.md` **18942**,
`^---$` **216**, `^## ADR-` **316**, bare `^## ` **324**, tail **ADR-0317**, next-free **ADR-0318**
(`^## ADR-0318` reads 0) · house `PROPOSED` **DISARMED at 0**, **proven live** on a scratch copy
(append -> reads 1); the ADR-0231 decoy reads 1 at `:14866`, resolved by BACKWARD heading search to
`14864 ## ADR-0231` · `BEHAVIOR_CONTRACT.md` **5989** · `STATE.md` **65** · `STATE_HISTORY.md` **554**,
strict **163** / parenthetical **64** / loose **227** (163 + 64 = 227 exactly, under the named
anchored-occurrence forms) · phase dirs **136** (**137** after this row) · fixtures **122**, tail
`0120-tls-connection-error`, `0121` FREE and **not taken by this row** · extractor **122 = 122**, both
`comm` directions EMPTY, split **98 `driver/` + 24 `inputs/`**, and the extractor **NC'd** (a rename
fires both `comm` arms while the count stays 122 — the count-only form is VACUOUS) · fuzzers **56 / 48**
· `go.mod` **67** require entries (⚠️ the lossy form reads 62, §0.11) · `go list ./...` **237** (**235**
excluding the two Docker drivers) · BackendKind tail **38** (`test/differential/fixture/fixture.go:614`)
· `-family row` **96 occurrences / 68 lines** (⚠️ `--` before the pattern; without it ugrep errors
rc=2) · `internal/tls/config.go` **666** (⚠️ NOT the 642 row 95 asserts, §0.1) ·
`internal/listener/manager.go` **1636**.

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

`default_filter_chain` appears in exactly **one** fixture (`0008-listener-chain-match`, fully
plaintext, 8 files); `quic_options` in exactly **one** (`0104-http3-downstream-get`, driver-generated
bootstrap, `filter_chains[]` only, no default chain). **The intersection is empty and neither carries
a TLS default chain.** Deleting the fix would leave all 122 fixtures green.

### 7.2 What a gate would have to do

The subject is a **crash on a shape no fixture builds**, so the differential is the wrong instrument
for the primary pin: the harness asserts served traffic and cross-side stats, and a nil-counter SIGSEGV
aborts the subject binary before any assertion runs. The SPEC should pin D1 in unit tests — where the
un-fixed tip's SIGSEGV is an unusually strong negative control — and reserve the differential for the
**stat-surface parity** question of §3.3 hazard 1, which is genuinely cross-side and currently
unmeasured. ⚠️ **That decision must be argued from the reference arm, not from the shape of the
existing suite.**

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN INSERTION

### 8.1 PRE-INSERTION, at `8ac19ab4`

- **(1)** `want=127` — **SILENT**.
- **(2)** **SIX**, at `:205 :211 :217 :227 :233 :241`.
- **(3)** — **SILENT**.

Per-line md5 of the six windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — all six
**byte-identical to the phase-95 IMPL close**:
`205 10d7807bf02d` · `211 4a92f7e62fc6` · `217 2a7eb298b9fd` · `227 242e53c6f7a3` ·
`233 b2680e6f4fbf` · `241 6caa1c3ce0e7`.
⚠️ **THE DIGEST IS METHOD-SENSITIVE** — they match only with the trailing newline included.

Malformed-row baseline reconciled under BOTH forms: naive **17**, escape-aware **2** (row IDs **57**
`NF=9` and **69** `NF=10`), using `sed 's/\\|//g' F | awk -F'|' …` with **no file argument to awk**.

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

- **NC-A** (doctor row 62 to `in-progress`): landed (`NC LANDED? [ in-progress ]`), check (1) on the
  copy printed **ONE** line, `NOT DONE: row 62`. ⚠️ **ONE, not two — row 95 is now `done`.** The shape
  was re-measured here, never inherited.
- **NC-B** (`want=126` on the real file): **ONE** line,
  `GATE FAIL: examined 127 data rows, expected 126`.
- **NC-C** (check-(3) NC): residual **0**, and `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D**: `-family row` **96 occurrences / 68 lines**, with `--` before the pattern.

### 8.3 The check-(2) positive control

Both phrases substituted (the longer does not contain the shorter as a substring, so a one-phrase
control reports a residual of 5 and reads like a finding): residual **0**, substitutions asserted at
**6**.

### 8.4 POST-INSERTION — measured on the other side of this stage's own row

An **ADD**, not a flip, so the denominator MOVES: `want` **127 -> 128**, `ROADMAP.md` **245 -> 246**,
row 96 at file line **158**, NF **8** under both the naive and the escape-aware form, and the
malformed-row baseline unchanged (naive **17**, escape-aware **2**).

- **(1)** `want=128` — **NON-SILENT**, exactly one line: `NOT DONE: row 96`. (It was SILENT
  pre-insertion; this row is `in-progress` by construction.)
- **(2)** **SIX**, at `:206 :212 :218 :228 :234 :242` — every anchor **shifted down by one** because
  the row inserts above the windows, while the six per-line md5s are **byte-identical** to their
  pre-insertion values: `206 10d7807bf02d` · `212 4a92f7e62fc6` · `218 2a7eb298b9fd` ·
  `228 242e53c6f7a3` · `234 b2680e6f4fbf` · `242 6caa1c3ce0e7`.
- **(3)** — **SILENT**.

⚠️ **BOTH DENOMINATOR NCs CHANGED SHAPE ACROSS THE ADD, AND WERE RE-MEASURED RATHER THAN INHERITED:**

- **NC-A** read **ONE** line pre-insertion (`NOT DONE: row 62`) and reads **TWO** after —
  `NOT DONE: row 62` **and** `NOT DONE: row 96`, because this row is `in-progress`.
- **NC-B** (`want=127` on the real file) read **ONE** line pre-insertion and reads **TWO** after:
  `NOT DONE: row 96` **then** `GATE FAIL: examined 128 data rows, expected 127`.
- **NC-C** fired unchanged (residual 0). **NC-D** unchanged at **96 occurrences / 68 lines** — this
  row's summary carries no `-family row`, deliberately. The check-(2) positive control still reads
  **6 -> 0 with 6 substitutions asserted**.

⚠️ **THE NEXT SESSION INHERITS THE TWO-LINE SHAPE, AND WILL LOSE IT AGAIN THE MOMENT ROW 96 FLIPS
`done`** — at which point NC-A and NC-B each drop back to ONE line. **NEVER INHERIT AN NC SHAPE
ACROSS A ROW CHANGE.**

### 8.5 Provenance of the pick against the six windows — and the instrument NC'd

A per-line, case-insensitive, fixed-string sweep of all six windows for `default_filter_chain`,
`default filter chain`, `mandatory TLS`, `transport_socket`, `config-parity`, `config parity`,
`quicChain`, `chain selection`, `deferred-reject` returns **exactly one hit**: `transport_socket` in
window `:227`, and it is the **L4 tap** transport socket
(*"L4 tap is a transport socket (`envoy/extensions/transport_sockets/tap`, which remains a deferred
candidate)"*) — unrelated to this row.

⚠️ **A ZERO-RESULT SWEEP IS INDISTINGUISHABLE FROM A BROKEN ONE.** The instrument was NC'd: the same
per-line grep for `quic` against window `:205` (the HTTP/3+QUIC family line) reads **3**.

⇒ **PROVENANCE IS OUTSIDE EVERY SENTINEL WINDOW. This row makes ZERO sentinel progress** — check (2)
reads SIX before and after — exactly as phase 95 did, and it is disclosed rather than dressed up.

### 8.6 The sentinel verdict

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** (verified absent
at the git root and in every stage worktree).

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 ⚠️ A VARIABLE REUSED FOR A SECOND PURPOSE OUTLIVES THE INVARIANT THAT MADE IT SAFE

`anyTLS` was correct as a *mixed-TLS cross-check accumulator*. Phase 74 reused it as the *stat
registration gate*. ADR-0080 had already exempted the default slot from the cross-check — so the
second purpose inherited an invariant the first had explicitly abandoned. **When you gate a new
feature on an existing boolean, prove that boolean still means what the new use needs, and name the
ADR that could repeal it.**

### 9.2 ⚠️ TWO INDEPENDENT AGREEING MEASUREMENTS DO NOT MAKE A CLAIM GENERAL

Phase 95's controller and its scoped re-reviewer both measured F-E and agreed it was benign. Both
enumerated the **same single config**. Agreement between two readers is evidence about the *shape they
both examined*, never about the class. **Enumerate the class, then measure; do not measure, then
generalise.**

### 9.3 ⚠️ THE TEST NAMED FOR AN INVARIANT MAY NOT EXERCISE THE INPUT ON WHICH IT FAILS

`TestListenerMetrics_GateMatchesInc` exists precisely to assert the equivalence this row refutes, and
every one of its arms builds a `filter_chains[]`-only listener. **A guard's name is not its coverage —
read the arms.**

### 9.4 ⚠️ A CRASH IS AN UNUSUALLY STRONG NEGATIVE CONTROL, AND AN UNUSUALLY SILENT ONE

The un-fixed tip does not fail an assertion; it **aborts the binary**. That makes the NC unmissable
under `go test`, and it makes the same defect invisible to any gate that only reads exit codes from a
long-running server. Gate on the anchored `^panic:|DATA RACE|SIGSEGV` form, and prove it live.

### 9.5 ⚠️ ASSERT THE POINTERS, NOT ONLY THE EFFECT

`sslHandshake == nil` is the cause; the SIGSEGV is the effect. A probe that only observes the crash
proves something happened, not what. Both were asserted, in that order
([[reference_nil_stats_counter_inc_crashes_goroutine]]).

### 9.6 ⚠️ THE FIX SHAPE ENCODES A PARITY ANSWER NOBODY HAS MEASURED

Widening `tlsMode` and nil-guarding the Inc sites both stop the crash and disagree about whether the
listener emits five `ssl.*` names or zero. **Two repairs that differ only in an unmeasured stat
surface are not interchangeable, and diff size is not the tiebreaker.**

### 9.7 Method findings, each found by execution

- **A banked cost list rots in every field at once** — the port race was wrong in its count (37, not
  ~36), its split (31/5, not 30/4) **and its directory** (`test/differential` holds 9 `.go` files;
  the drivers are under `test/fixtures/*/`). §0.8.
- **A list of stale citations can carry a stale citation** — and one of its four entries was correct.
  §0.9.
- **A "cheap missing guard" can be unreachable behind an existing set-equality test.** §0.10.
- **A character class silently drops what it does not spell** — `[a-z0-9./-]` dropped five `go.mod`
  entries with uppercase or `_`. §0.11.
- **A green rerun of a registered flake clears nothing**: `TestFramer_ReaderGoroutineDoesNotLeak` ran
  5/5 and 257/257 green (both non-vacuous, `RUN` asserted beside `RC`), but its guard carries slack 2
  over 40 connections and reported `delta=0` every iteration — a small real leak would sit inside that
  margin. **Unreproduced, not disproven.**

### 9.8 Defects found in passing — RECORDED, deliberately NOT fixed by this stage

The `stat_prefix` duplicate-registration panic (§0.13/§4.3) · D10-QUICSEL (§4.2) · the false invariant
comments at `manager_test.go:2325-2332` and `:4993-4998` · the six stale citations plus the newly-found
`(:4484)` at `manager_test.go:5571` · `0108`'s three false `ssl.*` confessions · `0118/driver/driver.go:31`'s
falsified band · `0120/expectations.yaml`'s phase attribution · the undocumented D8 proto-mandated
divergence and its PGV-strictness nit.

---

## 10. What the SPEC owes

1. **Draft `ADR-0318`** (TAIL-derived — ⚠️ headings+1 reads **316**, a TAKEN id, because the space is
   sparse at the `0209` gap). §Context only, status in the **house form** — currently disarmed, and a
   zero there is the RESTING STATE, so **prove the guard live on a scratch copy before trusting it**.
   ⚠️ **Do NOT quote a count for either guard form in prose the grep matches** (the phase-93 SPEC
   falsified itself doing exactly that).
2. **MEASURE THE REFERENCE ARM OF §3.3 HAZARD 1** — does the reference emit `ssl.*` on a listener whose
   only TLS is its `default_filter_chain`? This decides between the two fix shapes and it is the one
   blocking unknown. **A SPEC that lands the one-liner without this arm is asserting unmeasured parity.**
3. **Choose the fix shape from that measurement**, and say why the rejected shape is wrong rather than
   merely larger (§9.6).
4. **Give `TestListenerMetrics_GateMatchesInc` a default-chain arm** and NC it — the tip itself is the
   NC, and it aborts the binary rather than failing an assertion (§9.4).
5. **Pin both crashing shapes** of §2.2 — the zero-chain shape AND the ineligible-plaintext-chain
   shape. The `len(chains) == 0` guard is not the boundary and a single-shape pin would re-mint §0.3.
6. **Decide explicitly whether D2-QUICTS rides** (§4.1), stating the reference measurement that makes
   it parity and the validate-mode rc=0-vs-rc=1 divergence that makes it worth anything.
7. **Correct the false invariant prose** at `manager_test.go:2325-2332` and `:4993-4998` — and
   ⚠️ **reconcile the corrected claim across its WHOLE occurrence set**, case-insensitively, or state
   which hits are deliberately left (the phase-95 F-C failure: fixed in two of three copies).
8. **Re-derive every count in §6.4 at the SPEC's own tip** and every anchor by LITERAL text — this
   row's own §6.1 line will move the anchors below it.
9. **Enumerate the edit roster as an EDIT roster**, set-differenced against any byte-untouched gate.

---

## 11. Probe hygiene

- Reference image digest **verified against `ENVOY_TARGET.md:3-4`** before any arm was trusted:
  `sha256:7edd5b0fd763…`. A positive control established that "accept" is reachable on both sides
  before any reject was believed; accepts were read from OUTPUT, never from `timeout`'s rc=124.
- Probe containers `egprobe-b-r0…r7` and `egprobe-b-v0…v5` were created and **removed BY NAME**;
  post-run `docker ps -a` shows none. ⚠️ **SEVENTEEN foreign `curl-world-*` containers were observed,
  owned by a sibling session (up 2 days), and LEFT UNTOUCHED.** No `reaper_*` container was created or
  contacted.
- Probe ports **15100-15109 / 15200-15207**, from the 15000-17999 band — **below**
  `net.ipv4.ip_local_port_range` (`32768 60999`), **outside** the harness reservations
  (`20000..31007`, `11000..14999`), and clear of `18080-19000` which the sibling holds. Availability
  checked with `ss -tan` **and** `ss -uan` (ALL states) before use.
- Unit probes bound **port 0** throughout; the subject binary was built with `-o` into scratch, never
  into a worktree root, and processes were killed by the PID captured at launch — never by pattern
  (`pkill -f` matches the tool call's own shell, exit 144).
- The one-line prototype was applied ONLY in this stage's worktree, measured by `git diff --numstat`
  (**1 / 1**, one file), and **reverted under `sha256sum -c`** (`internal/listener/manager.go: OK`),
  with `./internal/listener/` re-run green afterwards. Five probe files were created and deleted.
- `git status --porcelain --untracked-files=all` on the stage worktree: **EMPTY**; on the main root:
  only the pre-existing untracked `.claude/`.
- **All four measurement agents committed NOTHING** and proved their trees clean. Every headline figure
  they reported was **re-derived by the controller before use** — and doing so caught the router's own
  `:1944` and `config.go` figures (§0.1, §0.2). ⚠️ **Docker was serialised to exactly ONE agent.**
