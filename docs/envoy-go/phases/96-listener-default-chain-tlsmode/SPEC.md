# Phase 96 — `listener-default-chain-tlsmode` — SPEC

**Lifecycle-state 1 -> 2.** Row 96 STAYS `in-progress`; this stage does not touch `ROADMAP.md`.

**One sentence.** A `default_filter_chain` carrying a TLS `transport_socket` on a listener whose
`filter_chains[]` are plaintext or absent leaves `listenerRuntime.tlsMode` **false**, so the five
`ssl.*` counters are never registered, while the per-connection Inc guard (`selected.tlsCfg != nil`)
is **true** — the first completed downstream TLS handshake dereferences a nil `*stats.Counter` and
**SIGSEGVs the process**. This SPEC measures the reference, which settles the fix shape, and pins the
repair.

---

## 0. What this stage refuted, by execution

**TEN claims.** Five are load-bearing.

### 0.1 🔴 THE BLOCKING UNKNOWN IS ANSWERED, AND IT ELIMINATES ONE OF THE TWO FIX SHAPES

`BRAINSTORM.md` §2.5 and §3.3 hazard 1 left one arm unrun: **does the reference emit `ssl.*` on a
listener whose only TLS is its `default_filter_chain`?** It does. Measured on the pinned digest
`sha256:7edd5b0fd763…` (verified against `ENVOY_TARGET.md:3-4` before any arm was trusted), **twice
independently** — once by a measurement agent on its own ports, then by the controller from configs
it wrote itself on a disjoint port band, reading the OUTPUT (`starting main dispatch loop`) for every
boot verdict and never `timeout`'s exit code:

| arm | listener shape | boot | drive | listener-scope `ssl.*` names |
|---|---|---|---|---|
| **C1** | `default_filter_chain` with TLS, **no** `filter_chains[]` | ACCEPT | 3 × HTTPS 200 | **17**, `ssl.handshake: 3`, `ssl.no_certificate: 3` |
| **C4** | plaintext `filter_chains[0]` made ineligible by `destination_port: 65530`, **plus** TLS default chain | ACCEPT | 3 × HTTPS 200 | **17**, `ssl.handshake: 3`, `ssl.no_certificate: 3` |
| **C2** (positive control) | TLS in `filter_chains[0]`, no default chain | ACCEPT | 3 × HTTPS 200 | **17**, `ssl.handshake: 3`, `ssl.no_certificate: 3` |
| **C3** (negative control) | fully plaintext | ACCEPT | 3 × HTTP 200 | **0** |

`diff` of the two name sets with the address label stripped — **C1 ≡ C2, IDENTICAL, 17 = 17.** The
reference does not distinguish the two structural slots for stat registration at all.

⇒ **FIX SHAPE (A) — widen `tlsMode` — IS THE PARITY-CORRECT REPAIR. SHAPE (B) — nil-guard the five
`.Inc()` sites — IS NOT MERELY DIFFERENT, IT IS AFFIRMATIVELY WRONG**: it would leave a listener that
serves TLS reporting **zero** `ssl.*` names where the reference reports seventeen. §9.6 of the
BRAINSTORM framed the two as "disagreeing about an unmeasured surface"; measured, one of them is a
silent observability hole shipped as a fix.

⚠️ **NAME THE MEASURE.** *Seventeen* is the count under the anchored form `^listener\.[^:]*\.ssl\.`
on `/stats`. A count of *twenty* is also true and is a different measure — it adds the three
`server_ssl_socket_factory.*` names, which are a **different scope**. Any figure quoted without its
matcher is meaningless.

### 0.2 🔴 THE ROUTER'S OWED-LIST ITEM 7 IS OUT OF SPEC SCOPE — MEASURED, NOT INFERRED

`next-prompt.txt` instructs this stage to *"CORRECT the false invariant prose at
`manager_test.go:2325-2332` and `:4993-4998`."* A SPEC does not touch `.go` files. Measured with
`git show --numstat` across **FOUR consecutive SPEC commits** — `13ad0aa0` (92), `975e527e` (93),
`307f2e3d` (94), `9ed6a620` (95) — every one touches **exactly the same five files**:

```
docs/envoy-go/DECISIONS.md            +28 / +30 / +30 / +28,  0 deletions
docs/envoy-go/STATE.md                 in place
docs/envoy-go/STATE_HISTORY.md         +2
docs/envoy-go/phases/NN-slug/SPEC.md   new
next-prompt.txt                        rolled
```

**ZERO `.go` bytes, and `ROADMAP.md` / `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED, in all four.** The
prose corrections are PINNED at §11 and LANDED by the IMPL. This is the same species of router error
the phase-95 SPEC refuted about `BEHAVIOR_CONTRACT.md:1944`, one row later and in the same field.

### 0.3 🔴 THE OCCURRENCE SET IS NINE LIVE SITES, NOT TWO — AND THREE ARE IN GOVERNING DOCUMENTS

The router and `BRAINSTORM.md` §9.8 both name two sites, both in `manager_test.go`. A tree-wide
case-insensitive sweep across production `.go`, test `.go` and `docs/` finds **nine live carriers**,
and the false invariant is **NORMATIVE**, not merely commentary:

| # | site | enclosing symbol / ADR | verdict |
|---|---|---|---|
| 1 | `DECISIONS.md:17296` | **ADR-0296 §Decision (a)** (heading resolved by backward search to `:17256`) | **FALSE** — *"rejects mixed TLS+plaintext chains on one listener, so a listener is wholly TLS or wholly not"* |
| 2 | `DECISIONS.md:17276` | **ADR-0296 §Context ¶8(ii)** | **FALSE** — *"provably sufficient … every QUIC listener that boots has `tlsMode == true`"* |
| 3 | `BEHAVIOR_CONTRACT.md:1973` | §QUIC — permanently ZERO | **FALSE** on the parenthetical (the conclusion survives for a different reason — §0.4) |
| 4 | `manager_test.go:2325-2332` | doc on `TestListenerMetrics_GateMatchesInc` | **FALSE** — the head of the class |
| 5 | `manager_test.go:2342-2344` | arm (a) comment | **FALSE** — the "hence … equivalent" is a non-sequitur |
| 6 | `manager_test.go:2433-2436` | arm (c) comment | **FALSE and actively harmful** — *"do not add nil guards"* |
| 7 | `manager_test.go:4993-4998` | doc on `TestServeConnection_PlaintextListenerIncrementsNoSSL` | **FALSE** — endorses `tlsCfg != nil` as the sufficient guard, the exact predicate that fails |
| 8 | `manager.go:384-393` | doc on `registerListenerMetrics` | **FALSE** on its last sentence (§0.4) |
| 9 | `quic_test.go:225-232` | doc on `TestQUICListener_RegistersSSLNamesAtZero` | **FALSE** — same corollary |

**Deliberately LEFT, with reasons** (method note 31 requires naming these, not silently skipping
them): `manager.go:179-184` and `manager_test.go:2189-2194` are MISLEADING-BUT-DEFENSIBLE scoping
prose whose mechanism is stated correctly; `manager_test.go:2281-2282`, `:2414-2419`, `:2423-2427`,
`quic_test.go:274-282`, `BEHAVIOR_CONTRACT.md:1042` and `:1967` are **TRUE** — they describe gate
placement and the crash mechanism without asserting the equivalence. `DECISIONS.md:18842`
(**ADR-0316 D-TLSCE-NILGATE**) is TRUE and is the strongest existing citation this row has: it already
records that a nil-pointer Inc SIGSEGVs `serveConnection` *"while `TestListenerMetrics_GateMatchesInc`
itself PASSES"* — attributed only to a hypothetical code deletion, never to a **config shape**.
Historical copies under `docs/envoy-go/phases/74-*`, `92-*`, `95-*` are records, not live contract,
and are left byte-untouched.

### 0.4 🔴 A SECOND FALSE COROLLARY, REFUTED FIRST-HAND — AND THE SAME ONE-LINER CLOSES IT

*"`startQUIC` hard-errors when the chain carries no TLS config, so every QUIC listener that boots has
`tlsMode == true`"* is **FALSE**. `quicTLSConfig()` (`quic.go:56-58`) returns `rt.defaultChain.tlsCfg`
**FIRST**, before consulting `chainByName`; `anyTLS` is written only inside the `filter_chains[]` loop.
Controller probe, run at this stage's own tip, written from the code:

```
QUIC default-chain-only: tlsMode=false kind==kindQUIC=true len(chainSpecs)=0 defaultChain.tlsCfg!=nil=true
QUIC default-chain-only: quicTLSConfig()!=nil=true (so startQUIC does NOT hard-error)  quicChain()==defaultChain=true
```

A QUIC listener with zero `filter_chains[]` and a QUIC-wrapped `default_filter_chain` **boots, starts,
and registers ZERO `ssl.*` names** where the reference registers its full set. The shape is not
hypothetical: `TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos`
(`manager_test.go:1062`) already builds it and asserts nothing about `tlsMode` or the counters.

This is a **stat-registration divergence, not a crash** (`Manager.Start` launches no accept loop for
`kindQUIC`, so `serveConnection`'s Inc sites are structurally unreachable), so it is a *distinct
symptom* — but it has the **same root cause and is closed by the same one-line predicate**. It rides,
and §4 says so explicitly rather than leaving it to be discovered.

⚠️ **THE CLAIM HAS FOUR LIVE CARRIERS** — `manager.go:391`, `quic_test.go:229`, `DECISIONS.md:17276`,
`DECISIONS.md:17296` — and correcting one of them without the other three would be the phase-95 F-C
failure again.

### 0.5 ⚠️ `manager_test.go:2468-2470` DOES NOT MERELY FAIL TO EXERCISE THE CRASHING SHAPE — IT ASSERTS THE SHAPE IS IMPOSSIBLE

```go
if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil {
    t.Error("plaintext listener: defaultChain has tlsCfg != nil")
}
// THE LOAD-BEARING HALF.
```

On a `tlsMode == false` listener that is **exactly the crashing configuration**, and this arm calls it
a failure. It is dead code today (all three arms build `filter_chains[]`-only listeners, so
`rt.defaultChain` is always nil — verified by reading `:2345-2492` and by the repo-wide
`DefaultFilterChain` roster, which has no site in that range). **The IMPL must INVERT this assertion,
not merely add an arm.** `BRAINSTORM.md` §10 item 4 says "give it a default-chain arm"; that is
necessary and not sufficient.

### 0.6 ⚠️ FOUR ANCHOR CITES INSIDE THE AFFECTED PROSE ARE STALE

Each will mislead the IMPL's auditor, and each is corrected here by LITERAL text rather than by
arithmetic:

| cite | where it appears | what that line actually is now | the true site |
|---|---|---|---|
| `manager.go:575` | `manager_test.go:2328` (for the mixed-TLS check) | `// filter_chains[] and default_filter_chain must contribute at least` | the check is `manager.go:683-690` |
| `manager.go:692` | `manager_test.go:2337` (for the `tlsMode` write) | `// ADR-0033 clause 6 / ADR-0078 clause-6 (PARTIALLY SUPERSEDED): a plaintext` | `manager.go:825` (`tlsMode: anyTLS,`) |
| `manager.go:562` | `manager_test.go:2337` (for the `tlsCfg` write) | the `func buildListenerRuntimeWithCtx(...)` signature | `manager.go:675` and `:765` |
| `manager.go:516-525` | `DECISIONS.md:17296` (for the mixed-TLS reject) | `validateQUICOptions`' doc comment and its `proof_source_config` reject | `manager.go:683-690` |
| `manager_test.go:2137` | `DECISIONS.md:17359` (for `GateMatchesInc`) | mid-comment in an unrelated helper | `manager_test.go:2341` |

### 0.7 ⚠️ THE ROUTER'S `BEHAVIOR_CONTRACT.md:5130` CONVENTION CITATION IS WRONG TWICE

`next-prompt.txt` states that *"by its own convention at `:5130` a `+0` row earns NO chain entry."*
`:5130` is a **BLANK LINE**. And the convention is refuted by the ledger itself: **phases 44.2, 44.3,
45.2, 47.1 and 51 each carry a `+0, UNCHANGED` chain entry.** A `+0` row earning an entry is
established practice, not a violation. §11 decides the ledger question on the merits rather than on
the mis-cited rule.

### 0.8 ⚠️ THE BARE-`ssl` GREP TRAP IS REAL, AND ITS WITNESS IS CONFIG-DEPENDENT

An unanchored `grep ssl` over a **plaintext** listener's `/stats` returns **4** lines on the
controller's rig — `http.<prefix>.downstream_cx_ssl_active` / `_total` and the admin pair — none of
them a listener-scope `ssl.*` name. A measurement agent hit the same trap with a **different** witness
(`server.accesslog_dropped`, matching `acce-ssl-og`) on a config carrying an access log. **The trap is
general; its witness is not.** Every gate in this row anchors on `^listener\.[^:]*\.ssl\.`, and the
anchored form was NC'd against the plaintext arm: **0**.

### 0.9 ⚠️ `no_filter_chain_match` STAYS 0 ON THE INELIGIBLE-CHAIN SHAPE

Reference, arm C4: `listener.0.0.0.0_15407.downstream_cx_total: 3` and
`listener.0.0.0.0_15407.no_filter_chain_match: 0`. A default chain **absorbs** the connection; it is
not booked as a match miss. Any pin asserting `no_filter_chain_match > 0` for that shape would fail
against correct code on both sides ([[reference_pin_can_fail_against_correct_code]]).

### 0.10 ⚠️ THE CAUSE IS CONFIRMED AT THIS STAGE'S OWN TIP, ON BOTH SHAPES, WITH A DISCRIMINATING CONTROL

Not inherited from the BRAINSTORM. Controller probe, pointers asserted **before** any dial
([[reference_nil_stats_counter_inc_crashes_goroutine]], method note 7k):

```
shapeA_zero_chains_tls_default:                BUILD OK tlsMode=false len(chainSpecs)=0 defaultChain.tlsCfg!=nil=true
shapeA_zero_chains_tls_default:                POINTERS sslHandshake==nil=true sslNoCertificate==nil=true sslFailVerifyError==nil=true sslFailVerifyNoCert==nil=true sslConnectionError==nil=true
shapeB_ineligible_plaintext_chain_tls_default: BUILD OK tlsMode=false len(chainSpecs)=1 defaultChain.tlsCfg!=nil=true
shapeB_ineligible_plaintext_chain_tls_default: POINTERS sslHandshake==nil=true sslNoCertificate==nil=true sslFailVerifyError==nil=true sslFailVerifyNoCert==nil=true sslConnectionError==nil=true
control_tls_filter_chains:                     BUILD OK tlsMode=true  len(chainSpecs)=1 defaultChain.tlsCfg!=nil=false
control_tls_filter_chains:                     POINTERS sslHandshake==nil=false sslNoCertificate==nil=false sslFailVerifyError==nil=false sslFailVerifyNoCert==nil=false sslConnectionError==nil=false
```

The control discriminates: same package, same registry, same build path, **all five pointers
non-nil**. Both probes were deleted and the worktree proven clean under `sha256sum -c`.

---

## 1. Scope, restated as a decision

**IN.** One production predicate — the `tlsMode` write site — widened so that a `default_filter_chain`
carrying TLS registers the listener's five `ssl.*` counters. The repair closes **two** members of the
same root cause: **D1-TLSMODE** (the TCP SIGSEGV) and the **QUIC default-chain registration gap**
(§0.4). Unit pins for both crashing shapes and for the QUIC shape; the inverted assertion at
`manager_test.go:2468-2470` corrected; the nine-site prose set reconciled; one differential fixture
pinning the now-measured cross-side stat parity; `ADR-0318` drafted here and completed at the IMPL.

**OUT.** **D2-QUICTS** (§4) and **D10-QUICSEL** (`BRAINSTORM.md` §4.2) do not ride. **D4-MIXEDTLS is
NOT the bug and its comment must not be changed** — ADR-0080 §Decision 3 authorises the exemption
verbatim, and "fixing" D4 would break ADR-0080 parity. **D8-FCM** is proto-mandated. The
`stat_prefix` duplicate-registration panic (`BRAINSTORM.md` §0.13) deserves its own row and is not
folded in.

---

## 2. The mechanism, and why ADR-0080 authorises the shape that crashes

Four anchors, quoted by LITERAL text (line numbers rot; see §10 for the tip-derived positions):

```
internal/listener/manager.go   anyTLS = true                        (inside the filter_chains[] loop ONLY)
internal/listener/manager.go   tlsMode:                 anyTLS,     (the single write site)
internal/listener/manager.go   if rt.tlsMode {                      (registerListenerMetrics — the REGISTRATION gate)
internal/listener/manager.go   if selected.tlsCfg != nil {          (serveConnection — the INC guard)
```

**ADR-0080 §Decision 3, verbatim:** *"`default_filter_chain` may carry an independent
`transport_socket` (TLS or plaintext) **regardless of the `filter_chains[]` entries' TLS posture**.
The cross-chain mixed-TLS-and-plaintext rule … applies WITHIN `filter_chains[]` only;
`default_filter_chain` is a structurally-separate slot and is NOT subject to the cross-chain TLS
uniformity rule."*

⚠️ **§Consequences (c) illustrates only ONE direction** — *"a TLS-only `filter_chains[]` entry
coexisting with a plaintext `default_filter_chain`"* — and the crashing shape is the **inverse**.
§Decision 3 is symmetric and authorises both; the SPEC records this so nobody reads the one-sided
illustration as the limit of the decision.

The root cause is **variable reuse**: `anyTLS` was correct as the *mixed-TLS cross-check accumulator*
(`anyTLS && anyPlaintext` at `manager.go:683-690`, fed only by `cis`, which only the `filter_chains[]`
loop populates). Phase 74 reused it as the *stat-registration gate*, inheriting an invariant ADR-0080
had already repealed at phase 07.2. **The comment at the D4 site states the repeal in terms** —
*"Per ADR-0080: `default_filter_chain` has an INDEPENDENT TLS posture from `filter_chains[]` — no
mixed-TLS-rule cross-check here"* — thirty-odd lines above the write site that depends on it.

`(*stats.Counter).Inc` is, complete:

```go
// Inc atomically increments by 1.
func (c *Counter) Inc() { c.v.Add(1) }
```

No receiver nil check. Non-test `internal/listener` and `internal/tls` carry **no `recover()`** —
verified, with a positive control proving the matcher can find one (`internal/wasm/root_vm.go:1127`,
`internal/lua/vm.go:300`, and a scratch-copy sentinel), so the zero is a real absence and not a broken
command.

---

## 3. The production edit — DECIDED

```go
tlsMode:                 anyTLS || (defaultChain != nil && defaultChain.tlsCfg != nil),
```

One line, `+1 / -1`, one file. `defaultChain` is built at `manager.go:765` and the composite literal
is at `:822-825`, so it is in scope with no reordering.

**Why (A) and not (B), stated as parity rather than as diff size** (§0.1, and method note 37): the
reference emits the same seventeen listener-scope `ssl.*` names for the default-chain-only shape as
for the normal slot. (A) reproduces that surface at the subject's five-name resolution; (B) reproduces
zero. Both stop the crash; only one of them is what the reference does.

⚠️ **THIS IS A LOWER BOUND** ([[reference_measured_prototype_is_a_lower_bound]], fourteen consecutive
rows once this one lands). It excludes: the inverted assertion of §0.5, the new arms of §5, the
nine-site prose reconciliation of §11, the fixture of §6, and `ADR-0318`.

⚠️ **DO NOT WIDEN THE `len(chains) == 0` GUARD AT `manager.go:573` INSTEAD.** It is not the boundary —
shape B has `len(chainSpecs) == 1` and crashes identically (§0.10).

---

## 4. D2-QUICTS — **DECIDED: IT DOES NOT RIDE**

`BRAINSTORM.md` §4.1 left this to the SPEC's discretion and required a decision either way.

**It does not ride.** Four reasons, in order of weight:

1. **It fails CLOSED.** envoy-go's Start failure aborts the whole process and leaves no port bound
   (`BRAINSTORM.md` §2.4, probed with a two-listener arm). Its only externally-observable half is
   validate-mode rc=0-vs-rc=1. This row's subject is a remotely-triggerable process crash on a
   config both implementations accept. Mixing the two dispositions in one ADR blurs both.
2. **The row already grew, and grew in the direction of the same root cause.** §0.4 adds a second
   member closed by the same predicate; §0.3 adds seven prose sites and two governing documents;
   §6 adds a fixture. D2-QUICTS shares neither the predicate nor the reconciliation set.
3. **Its observable half needs its own cross-side instrument.** A validate-mode rc pin is not a
   served-traffic assertion and does not fit this row's fixture, whose whole point is a
   *completing handshake's* stat surface.
4. **It is already banked in a governing document** — ADR-0317 `D-ALPNFB-TCPONLY` — with its reference
   measurement recorded. It is not at risk of being lost.

⚠️ **THE VALIDATE-MODE DIVERGENCE IS RESTATED HERE SO IT IS NOT RE-DISCOVERED:** `envoy-go -mode
validate` returns rc=0 `configuration OK` for a QUIC `default_filter_chain` with no
`transport_socket`, where the reference validator returns rc=1 with the byte-identical message it uses
for `filter_chains[]` (*"no transport socket specified for connection oriented UDP listener"* — **the
reference does not distinguish the two slots**, so a boot reject there would be PARITY, not a
departure). ⚠️ **envoy-go's flag is `-mode validate`; the reference's is `--mode validate`.**

---

## 5. Unit-test design

⚠️ **THE UN-FIXED TIP IS THE NEGATIVE CONTROL AND IT ABORTS THE BINARY RATHER THAN FAILING AN
ASSERTION** (`BRAINSTORM.md` §9.4). Gate every new test's NC on the anchored
`^panic:|DATA RACE|SIGSEGV` form, and prove that gate live before trusting a zero.
⚠️ **ASSERT THE POINTERS FIRST, THEN DRIVE** — `sslHandshake == nil` is the cause, the SIGSEGV is the
effect; a probe that observes only the crash proves something happened, not what.

### 5.1 `TestListenerMetrics_GateMatchesInc` — INVERT, then extend

- **INVERT `manager_test.go:2468-2470`** (§0.5). On a `tlsMode == false` listener,
  `defaultChain.tlsCfg != nil` is no longer a failure — it is the configuration this row makes safe.
  Under the fix, that combination cannot arise (a TLS default chain now sets `tlsMode`), so the
  correct replacement asserts the **post-fix invariant**: `rt.tlsMode == (anyTLS-equivalent) ||
  defaultChain-carries-TLS`, expressed as *the five pointers are non-nil iff any chain reachable on
  this listener — including the default slot — carries TLS.*
- **ADD arm (d) `default_chain_tls_zero_filter_chains`** — shape A. Assert, in order:
  `rt.tlsMode == true`; `rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil`;
  `len(rt.chainSpecs) == 0`; then **all five pointers non-nil**, each with its own `t.Errorf` (never
  `t.Fatalf` — a `Fatalf` makes the later assertions dead code,
  [[reference_fatalf_makes_assertions_unreachable]]).
- **ADD arm (e) `default_chain_tls_ineligible_plaintext_chain`** — shape B, the plaintext chain made
  ineligible by `filter_chain_match.destination_port`. Same five assertions plus
  `len(rt.chainSpecs) == 1`. ⚠️ **BOTH ARMS ARE REQUIRED**: pinning only shape A would re-mint the
  §0.3 error — narrowing a class into a claim about one member.
- **ADD arm (f) `quic_default_chain_tls`** — the §0.4 shape, built with the existing
  `mkQUICListenerDefaultChain` + `mkQUICDownstreamTS`. Assert `rt.tlsMode == true`,
  `rt.kind == kindQUIC`, and the five pointers non-nil. ⚠️ **This arm is a REGISTRATION pin, not a
  traffic pin** — `Manager.Start` launches no accept loop for `kindQUIC`, so the counters stay
  permanently zero and that is PARITY, unchanged by this row.
- ⚠️ **`t.Helper()` on any shared assertion body**, so each arm's failure carries a distinct call-site
  line (method note 7e).

### 5.2 A LIVE-HANDSHAKE test — the crash pin

New test in `internal/listener`, both shapes, one sub-test each:

1. Build the listener; assert `rt.tlsMode` and the five pointers **before dialling**.
2. Complete a real TLS handshake **and drive a request through it** — ⚠️ `openssl s_client`-style
   "did the handshake complete" is not "was the connection served"
   ([[reference_go_client_cert_withholding]] and method note 7h). Assert the payload round-trips.
3. Assert `ssl.handshake == 1` **and** `ssl.no_certificate == 1` on the registry.
   ⚠️ **A COMPLETING HANDSHAKE ON A LISTENER THAT SENDS NO `CertificateRequest` BOOKS BOTH** — the
   phase-95 PLAN's (d) finding; a `{handshake: 1, rest: 0}` map would fail against correct code
   ([[reference_pin_can_fail_against_correct_code]]). Assert the other three are `0` explicitly.

**NC:** revert the one-line predicate; the test must abort the binary with `SIGSEGV` at the `.Inc`.
Record the anchored panic-gate count, and prove that gate live rather than reading a zero as evidence
(method note 7i).

### 5.3 `TestServeConnection_PlaintextListenerIncrementsNoSSL` — prose only

Its behaviour is correct and stays. Its doc comment (`:4993-4998`) is corrected at §11: the guard
`if selected.tlsCfg != nil` is **not** what keeps the plaintext listener safe — the *registration*
gate and the *Inc* guard agreeing on that shape is. Keeping the Inc sites inside the guard remains
right; asserting the guard is *sufficient* is what was wrong.

### 5.4 What is deliberately NOT added

- No test for the D2-QUICTS reject (§4).
- No test for D10-QUICSEL dispatch — it needs its own ADR.
- No `len(helpText)` cardinality guard: `TestHelpText_KeySetExact` already does set equality in both
  directions, so it is **UNREACHABLE** (`BRAINSTORM.md` §0.10) — a vacuous-guard trap, struck.

---

## 6. Differential fixture `0121-listener-default-chain-tls` — **CHARTERED**

`BRAINSTORM.md` §6.3 forecast `+0` fixtures and §7.2 explicitly deferred the decision to this stage,
requiring it be *"argued from the reference arm, not from the shape of the existing suite."* The
reference arm is now measured (§0.1), and it argues **for** a fixture.

**Why.** Without one, **deleting the fix leaves all 122 fixtures green** — the row would ship its
central cross-side claim with no differential gate at all. The claim is now precisely stated and
precisely measurable: *both sides register the five `ssl.*` names on a listener whose only TLS is its
default chain, and both book `handshake` and `no_certificate` once per completed handshake.*

⚠️ **THIS REVISES TWO INHERITED STATEMENTS.** `BRAINSTORM.md` §6.3's *"+0 expected"* and ROADMAP row
96's *"Row lands +0 fixtures"* cell are both superseded. A SPEC does not touch `ROADMAP.md`; **the
IMPL must correct that cell when it flips row 96 to `done`** — pinned at §11.

**Shape.** Modelled on `0120-tls-connection-error`, which is the only precedent for a cross-side
`ssl.*` assertion.

- **Index `0121`** (`ls -d test/fixtures/*/ | wc -l` reads **122**, tail `0120-tls-connection-error`,
  `0121` FREE — ⚠️ **use the `ls -d` form; `^[0-9]{4}-` reads 120, dropping `0007a`/`0007b`**).
- **Reference in-container listener port `10127`.** ⚠️ **NOT `10121`** — `0028-http-lua-multi-script-
  and-per-route` holds `10120`-`10125` as a contiguous six-listener run (`inputs/driver.go:65-70`) and
  `0120` holds `10126`. `10127` is the minimal index-preserving repair, exactly the `0120` precedent.
  **Censused free in CODE SCOPE at this tip**; every repo-wide hit is docs prose, including
  `0002/PROGRESS.md:362`'s `101276/sec` false positive. The census was NC'd against `10126`, which
  correctly reads as taken.
- **Config**: one listener, **no `filter_chains[]`**, a `default_filter_chain` carrying an
  `envoy.transport_sockets.tls` `DownstreamTlsContext` and an HCM. ⚠️ **The reference container cannot
  read `filename:` cert paths** — inline the PEM, **indented one level deeper than `inline_string:`**,
  and keep PEM substitutions out of YAML comments. ⚠️ **An omitted `clusters:` key BOOT-REJECTS
  envoy-go** — carry a placeholder STATIC cluster. `BackendCount` must be ≥ 1.
- **Drive**: N completed TLS handshakes, each with a full application round trip. `HTTPExpectations`
  is TCP-only and this fixture is TCP, so it applies.
- **`AssertStats`**: a **NAMED SUBSET** over `/stats/prometheus`, keyed on the metric **NAME** with
  the address label IGNORED — the names are byte-identical cross-side while the labels differ
  (`0.0.0.0_10127` reference vs the subject's IPv6-wildcard form). ⚠️ **NEVER a name-SET equality**:
  the reference emits seventeen listener-scope `ssl.*` names and the subject five; the subject's five
  are a strict subset, and the assertion is *those five present, with these values, on both sides.*
  Pin `ssl.handshake` and `ssl.no_certificate` at the handshake count and the other three at `0`.
- **Three registration gates** ([[reference_differential_fixture_three_registration_gates]]): the
  fixture directory, the `fixture.RegisterFixture` call in `driver/init()`, **and the blank import in
  `test/differential/runner_test.go`**. ⚠️ **A MISSING BLANK IMPORT IS SILENTLY GREEN.** Prove all
  three by the extractor of method note 3e, and **NC the extractor** — a rename fires both `comm`
  directions while the count stays constant, so a count-only check is vacuous (re-proven at this tip:
  imports **122 -> 122** under a rename, `comm -23` **1**, `comm -13` **1**).
- ⚠️ **`-count=1` IS NOT OPTIONAL** and the fixture set must be asserted BY NAME in both directions.
- ⚠️ **`ssl.*` CAN BE SILENCED BY A FAST-FAILING UPSTREAM** on the reference side
  ([[reference_ssl_stats_suppressed_by_fast_failing_upstream]]) — use `direct_response` or a warmed
  backend, and confirm the drive returns 200 before believing any counter.

**Axis deltas this fixture moves:** fixtures **122 -> 123**, `0121` consumed; ports `+1` (`10127`);
BackendKinds **+0** (in-process `TCPEcho` branch); stat names **+0**; fuzzers **+0**; modules **+0**.

---

## 7. Gates

A SPEC runs **none** of the six, and that is scope, not omission (method note 10). The IMPL runs all
six and **names departures rather than claiming compliance**:

(a) differential — **123/123** expected once `0121` lands (122 today);
(b) non-Docker sweep gated on `PIPESTATUS[0]` plus a SET RECONCILIATION — **235 packages**
    (`go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`);
(c) h2spec `95 tests, 94 passed, 1 skipped, 0 failed`;
(d) fuzzers **56 / 48**;
(e) the ANCHORED panic gate `^panic:|DATA RACE|SIGSEGV`, **0**, *and PROVEN LIVE*;
(f) no `REVIEW.md` — the standing departure.

⚠️ **`-race` ON THE DIFFERENTIAL SUITE IS VACUOUS** — the subject is an unraced subprocess.
⚠️ **`go test` WITHOUT `-v` PRINTS ZERO `=== RUN`**: `RUN=0` beside `RC=0` is a vacuous green.
⚠️ **A `-run` SELECTOR MATCHING NOTHING PRINTS `[no tests to run]` AND EXITS 0**; a selector naming a
package that does not exist prints `FAIL … [setup failed]` and EXITS 1 — confirm every selector
resolves before believing either colour.
⚠️ **`gofmt -l` NEVER EXITS NON-ZERO — gate on OUTPUT.**
⚠️ **`golangci-lint`'s misspell runs in locale US** — sweep British spellings from `.go` comments
before the gate; markdown prose may use them freely.

---

## 8. `ADR-0318` — drafted here, completed at the IMPL

**`ADR-0318` is the next-free id, TAIL-derived** (`grep -oE '^## ADR-[0-9]+' … | tail -1` -> `##
ADR-0317`; `grep -c '^## ADR-0318'` reads 0). ⚠️ **NEVER derive from the heading count** — the id
space is sparse at the `0209` gap, so headings+1 collides with a TAKEN id.

Status in the **house form**, which this stage re-arms. ⚠️ **A ZERO ON THAT GUARD IS THE RESTING
STATE, NOT EVIDENCE IT WORKS** — it was proven live at this tip on a scratch copy (appending a
house-form line moves it 0 -> 1). ⚠️ **NO COUNT FOR EITHER GUARD FORM IS WRITTEN ANYWHERE IN PROSE THE
GREP MATCHES** — the phase-93 SPEC falsified itself doing exactly that. The ADR-0231 decoy at
`:14866`, resolved by backward heading search to `14864 ## ADR-0231`, is byte-untouched.

**§Context outline, drafted at this SPEC (§Decision and §Consequences follow at the phase-96 IMPL):**

¶1 the defect and its blast radius · ¶2 **THE REFERENCE MEASUREMENT, CARRIED INTO A GOVERNING
DOCUMENT** (method note 21 — `BRAINSTORM.md` §2.4 and this SPEC's §0.1 are not governing documents, and
the phase-94 lesson is that a measurement left only in a BRAINSTORM was lost for seventeen rows) ·
¶3 the root cause as variable reuse and the ADR that repealed the invariant · ¶4 why ADR-0080
§Decision 3 is symmetric where §Consequences (c) is one-sided · ¶5 the two fix shapes and why the
measurement, not the diff size, chooses · ¶6 the QUIC corollary and why it rides · ¶7 the nine-site
occurrence set and the three governing carriers · ¶8 what ADR-0296 §Decision (a) and §Context ¶8(ii)
get wrong, and the narrowing that replaces them · ¶9 the vacuous guard and the inverted assertion ·
¶10 D2-QUICTS and D10-QUICSEL, banked with their measurements · ¶11 what this ADR does not decide.

⚠️ **ADR-0296 IS AMENDED, NOT SUPERSEDED.** Its decision — *the registration gate is `rt.tlsMode`
alone, no kind check* — is **CORRECT and survives**. What is repealed is its stated **justification**
(*"a listener is wholly TLS or wholly not"* and *"every QUIC listener that boots has `tlsMode ==
true"*). The amendment leads with what survives, per the ADR-0296/0297 in-place-correction precedent.

---

## 9. What this SPEC does not decide

- **D10-QUICSEL** — the `quicChain()`/`quicTLSConfig()` accessor split. Repairing it means deciding
  multi-chain QUIC dispatch (ADR-0080 §Decision 1/2 fallback semantics vs `quic.go:68-73`'s documented
  single-chain assumption). **Needs its own ADR.** Note that this row makes the disagreement
  *observable* — a QUIC default-chain listener now registers counters — without resolving it.
- **The `stat_prefix` duplicate-registration panic** — the strongest banked candidate; its own row.
- **The other TEN fixed `ssl.*` names and FOUR dynamic families** — still blocked on NAMING (the stat
  name charset bans the hyphen).
- **D8-FCM's PGV-strictness nit** — a structurally invalid `filter_chain_match` inside
  `default_filter_chain` is PGV-rejected by the reference and silently ignored here. **UNMEASURED
  against a live reference**; recorded, not chartered.
- Whether the reference distinguishes the two slots for **any other** stat family. Only `ssl.*` was
  measured.

---

## 10. Counts, re-derived at THIS stage's own tip

Every figure below was produced by running its command at `f6b50462`, not copied forward
(method note 3c — every number in `next-prompt.txt` is an incidental figure).

`ROADMAP.md` **246** lines / **128** data rows / row 96 at file line **158**, `in-progress` ·
`DECISIONS.md` **18942**, `^---$` **216**, `^## ADR-` **316**, bare `^## ` **324**, tail **ADR-0317**,
next-free **ADR-0318** · `BEHAVIOR_CONTRACT.md` **5989** · `STATE.md` **65** · `STATE_HISTORY.md`
**556**, strict **163** / parenthetical **65** / loose **228** (163 + 65 = 228 exactly, under the
named anchored-occurrence forms) · phase dirs **137** · fixtures **122** via `ls -d test/fixtures/*/ |
wc -l`, tail `0120-tls-connection-error`, **`0121` FREE** · extractor **122 = 122**, both `comm`
directions EMPTY, split **98 `driver/` + 24 `inputs/`**, and NC'd · fuzzers **56 targets / 48 files**
· `go.mod` **67** require entries under a structural `awk` extractor (⚠️ the character-class form
`^\s+[a-z0-9./-]+ v[0-9]` reads **62**, dropping five entries with uppercase or `_`) · `go list ./...`
**237**, **235** excluding the two Docker drivers · BackendKind tail **38**
(`test/differential/fixture/fixture.go:614`) · `-family row` **96 occurrences / 68 lines** (⚠️ pass
`--` before the pattern) · `internal/tls/config.go` **666** (⚠️ **NOT the 642 row 95 asserts twice**)
· `internal/listener/manager.go` **1636**.

**Anchors this row will move**, to be re-located by LITERAL text and never by a scalar shift
([[reference_line_shift_after_insert_is_banded]]): `manager.go:825` (the `tlsMode:` write),
`manager.go:398` (`if rt.tlsMode {`), `manager.go:1363` (`if selected.tlsCfg != nil {`) and the five
Inc sites at `:1376 :1378 :1385 :1393 :1398`.

---

## 11. The pinned edit map — WHICH STAGE LANDS WHAT

**This SPEC lands (five files, the measured precedent scope of §0.2):** `SPEC.md`, `DECISIONS.md`
(ADR-0318 §Context), `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`.

**The PLAN lands:** `PLAN.md` only. No `.go`, no `ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no
`DECISIONS.md`.

**The IMPL lands — this is the roster, as an EDIT roster:**

| # | file | edit |
|---|---|---|
| 1 | `internal/listener/manager.go` | the one-line `tlsMode` predicate (§3) |
| 2 | `internal/listener/manager.go` | doc-comment correction at `registerListenerMetrics` (site 8 of §0.3) |
| 3 | `internal/listener/manager_test.go` | **INVERT** `:2468-2470` (§0.5) |
| 4 | `internal/listener/manager_test.go` | arms (d) (e) (f) on `TestListenerMetrics_GateMatchesInc` (§5.1) |
| 5 | `internal/listener/manager_test.go` | the live-handshake crash pin, both shapes (§5.2) |
| 6 | `internal/listener/manager_test.go` | prose at `:2325-2332`, `:2342-2344`, `:2433-2436`, `:4993-4998` (sites 4-7) |
| 7 | `internal/listener/quic_test.go` | prose at `:225-232` (site 9) |
| 8 | `docs/envoy-go/DECISIONS.md` | ADR-0296 §Decision (a) and §Context ¶8(ii) amended in place (sites 1-2); stale cite `manager.go:516-525` corrected; stale cite `manager_test.go:2137` at `:17359` corrected |
| 9 | `docs/envoy-go/DECISIONS.md` | ADR-0318 §Decision + §Consequences appended in place after the RETAINED italic footer — **no renumber, no `---` separator** |
| 10 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` | `:1973` parenthetical corrected (site 3) |
| 11 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` | stat-surface ledger: **a `+0, UNCHANGED` chain entry** — the row adds no NAME but changes WHICH listener shapes register the existing five. Established practice (phases 44.2, 44.3, 45.2, 47.1, 51 each carry one); §0.7 refutes the "no entry for +0" rule the router cited to a blank line |
| 12 | `test/fixtures/0121-listener-default-chain-tls/**` | the new fixture (§6) |
| 13 | `test/differential/runner_test.go` | the blank import — **the gate that is silently green if missed** |
| 14 | `docs/envoy-go/ROADMAP.md` | row 96 -> `done`, **and correct its "+0 fixtures" cell to +1** (§6) |
| 15 | `docs/envoy-go/phases/96-.../PROGRESS.md` | new |

**Byte-untouched set, to be asserted by `sha256sum` at the IMPL and set-differenced against the edit
roster above:** `internal/listener/quic.go`, `internal/tls/**`, `internal/stats/**`, and — critically
— **the D4 comment block at `manager.go:746-747`, which MUST NOT CHANGE.**

⚠️ **AN UNESCAPED `|` IN THE ROADMAP ROW PASSES CHECK (1) AND SILENTLY BREAKS THE FIELD COUNT.** Count
fields (want **8**) under BOTH the naive and the escape-aware form, BEFORE and AFTER installing, and
reword a pipe away rather than escaping it. Baseline at this tip: rows 94, 95, 96 read **8 / 8**;
malformed rows are IDs **57** and **69** only (naive **17**, escape-aware **2**).
⚠️ **NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW** — dormant this stage, LIVE at
the IMPL.

---

## 12. Negative-control roster

Every control below must be shown to FIRE, and a control that leaves its target green is not evidence.

| # | control | what it must do |
|---|---|---|
| 1 | revert the `tlsMode` predicate, run §5.2 shape A | binary aborts, anchored panic gate non-zero |
| 2 | revert the predicate, run §5.2 shape B | binary aborts — **proves `len(chains) == 0` is not the boundary** |
| 3 | revert the predicate, run §5.1 arm (f) | five pointers nil on the QUIC shape (a REGISTRATION failure, **no crash**) |
| 4 | delete the blank import from `runner_test.go` | extractor `comm` fires in BOTH directions while the count is unchanged |
| 5 | rename one fixture import in a scratch copy | both `comm` directions fire; the count-only form stays vacuous |
| 6 | delete `0121`'s `AssertStats` | the fixture must go RED, not silently green |
| 7 | drop one of the five names from `0121`'s expectation subset | RED — proves the subset is asserted, not merely scraped |
| 8 | run the anchored panic gate over a known-panicking input | non-zero, proving the gate live before any zero is believed |
| 9 | the house `PROPOSED` guard on a scratch copy of `DECISIONS.md` | 0 -> 1 on an appended house-form line |

⚠️ **NEUTRALISE, NEVER REVERT, FOR THE TEST-SIDE NCs** — the package must still compile, and the NC
itself must be shown to be EXECUTABLE.
⚠️ **DIFF THE ARM ROSTER, NOT THE COUNTERS**: a `+0/+0` control arm can be deleted with every gate
staying green ([[reference_deleted_zero_delta_control_is_invisible]]).

---

## 13. Sentinel — RUN MECHANICALLY AT `f6b50462`, ACTUAL OUTPUT

### 13.1 The three checks

```
(1) want=128            NOT DONE: row 96                    <- ONE line
(2)                     206 212 218 228 234 242             <- SIX
(3)                     (silent)
```

Verbatim check-(2) anchors: `206:remaining deferred (not-yet-chartered) candidates:` ·
`212:` · `218:` · `228:` · `234:` (same phrase) · `242:deferred candidates:`.

**Per-line md5 of the six windows, trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — ⚠️ **the
digest is METHOD-SENSITIVE and the method is stated because of it:**

`206 10d7807bf02d` · `212 4a92f7e62fc6` · `218 2a7eb298b9fd` · `228 242e53c6f7a3` ·
`234 b2680e6f4fbf` · `242 6caa1c3ce0e7`

**All six byte-identical to the phase-96 BRAINSTORM close.** ⇒ **THE SENTINEL DOES NOT FIRE, for TWO
independent reasons** (check (1) is non-silent AND check (2) reads SIX). **`stop` WAS EVALUATED AND
DELIBERATELY NOT CREATED** — verified absent at the git root and in both stage worktrees.

### 13.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

```
NC-A  doctor row 62      TWO lines:  NOT DONE: row 62 / NOT DONE: row 96
                         (landing inspected first: NC LANDED? [ in-progress ])
NC-B  want=127           TWO lines:  NOT DONE: row 96 / GATE FAIL: examined 128 data rows, expected 127
NC-C  gRPC substitution  residual 0; "NEVER OPENED: gRPC   <- NC FIRED"
NC-D  -family row        96 occurrences / 68 lines   (⚠️ `--` before the pattern; without it rc=2 and
                         the surrounding arithmetic prints 0, which reads exactly like "no change")
C2-PC both phrases       6 -> 0, with 6 substitutions ASSERTED
```

⚠️ **NC SHAPES CHANGE ACROSS A ROW CHANGE AND WERE NOT INHERITED.** A SPEC does not touch
`ROADMAP.md`, so these shapes are expected to hold across this stage — **and that was measured on both
sides of the stage, not assumed.** When row 96 eventually flips `done` at the IMPL, check (1) goes
SILENT and NC-A and NC-B each drop from TWO lines to ONE.

### 13.3 The archive guard and the eviction

**Evictee: `phase 94 (tls-connection-error-stat) IMPL done` (2026-09-05).** The five §Recent dates
read `09-07, 09-07, 09-07, 09-06, 09-05`; the unique oldest is also the TAIL, so date and position
agree — ⚠️ **but only because the three-way tie sits at the NEWEST positions, where it cannot affect
which entry is oldest. That agreement is a coincidence of this tip, not a rule.**

⚠️ **THE BARE FORMS ANSWER NOTHING**: on `STATE.md` the strict form reads 5 and the naive substring
form reads 5, and **both are invariant under which entry is evicted** (five in, five out). Only the
LABEL-BOUND PAIR discriminates, and both halves were measured:

```
'phase 94 (tls-connection-error-stat) IMPL done'   STATE.md 1 -> 0   STATE_HISTORY.md 0 -> 1
positive control, a sibling label already archived: STATE.md 0        STATE_HISTORY.md 1
fabricated-label NC:                                STATE.md 0        STATE_HISTORY.md 0
```

The archive append is **ONE INLINE LINE in the PARENTHETICAL form**, raw delta **+2** (a blank line
PLUS the entry line), with the **strict guard DELTA 0**. ⚠️ **NO POSITIVE-CONTROL FIGURE IS NAMED IN
THE ARCHIVE LINE** — the archive's controls are self-incrementing, and a figure recorded in the file it
measures is invalidated by the act of recording it. The §Recent preamble is rolled **without spelling
the evictee's label**, or the eviction check matches its own prose.

---

## 14. What the PLAN owes

1. A TDD spine discharging every item of §5, §6 and §11, with the task count DERIVED there — this SPEC
   deliberately quotes none.
2. **EVALUATE THE BOOTSTRAP §6.1 SPLIT GATE EXPLICITLY** (~25 tasks / ~1500 LoC) and record the
   verdict either way. The fixture of §6 makes this a real question rather than a formality.
3. **Re-derive every count in §10 at the PLAN's own tip** and re-locate every anchor by LITERAL text —
   §3's one line moves everything below it, and a multi-insert shift is BANDED, never a scalar.
4. **Order the tasks so the NC of §12 row 1 is available before the fix lands** — the un-fixed tip is
   the negative control, and it is consumed the moment the predicate is widened.
5. **Decide `0121`'s drive count and the exact expectation map**, and MEASURE the leaf set a completing
   handshake actually books on each side before writing it — a pin can fail against correct code.
6. **Prove the D4 comment block is on the byte-untouched roster** and set-difference that roster
   against §11's edit roster.
7. **Refute this SPEC by execution** and record it. This stage refuted TEN claims, three of them in
   documents the router told it to trust — including the router itself, twice (§0.2, §0.7).
