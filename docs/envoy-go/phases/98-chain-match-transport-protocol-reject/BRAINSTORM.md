# Phase 98 — `chain-match-transport-protocol-reject` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle **DONE -> 1**). **Self-picked** under the 2026-07-12 standing
directive, with no human consulted and no banked mid-lifecycle work to advance first.

**Subject in one sentence.** `internal/listener/manager.go`'s `parseChainSpec` boot-REJECTS any
`filter_chain_match.transport_protocol` outside `{"", "tls", "raw_buffer", "quic"}`, while the
reference **accepts an arbitrary string**, boots, binds and serves — so envoy-go refuses to start on a
static bootstrap that reference Envoy runs, and the refusal is reachable from a plain YAML file.

**Both sides were MEASURED this session**, each with a matched negative on a byte-identical input.

---

## 0. What this stage refuted — THIRTEEN claims, by execution

Every item below was produced by running something, not by reading. Where a figure is quoted, the
command that produced it is named. **Nine of the thirteen refute a claim this session's own router,
brief or agent asserted.**

### 0.1 The router's banked SNI front-runner is FALSE — the reference IS multi-label

`next-prompt.txt` banks, three separate times, that *"`sniMatchAny`'s `*.` SUFFIX IS MULTI-LABEL AND
THE REFERENCE'S IS NOT"* and calls it **UNMEASURED against a live reference**. It is measured now, and
the claim is refuted in the direction that kills the candidate.

On `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`, run BY
DIGEST after verifying `ENVOY_TARGET.md` lines 3-4, a listener with `server_names: ["*.foo.test"]` plus
a `default_filter_chain`, with the **negative control run FIRST**:

| SNI | reference body | subject `SelectChain` |
|---|---|---|
| `bar.other.test` — NEG CONTROL | `DEFAULT` | `DEFAULT(empty)` |
| `bar.foo.test` | `WILDCARD` | `WILD(*.foo.test)` |
| **`a.b.foo.test`** | **`WILDCARD`** | **`WILD(*.foo.test)`** |
| `a.b.c.foo.test` | `WILDCARD` | matches |
| `foo.test` (apex) | `DEFAULT` | `DEFAULT(empty)` |
| `xfoo.test` (no dot) | `DEFAULT` | no match |

**The two sides AGREE on every arm.** The reference implements `*.foo.test` as a case-insensitive
literal suffix match on `.foo.test`, exactly like `strings.HasSuffix(sni, p[1:])`. The multi-label
result was confirmed under **two independent clients** — `curl` and `openssl s_client -servername` —
because a single client could have mangled SNI.

⇒ **There is no defect on that axis, and the repair the banked item implies is AFFIRMATIVELY WRONG.**
Making `*.` single-label would *introduce* a divergence where none exists. This is the phase-96 shape-(B)
class: a repair rejected not for size but for being wrong.

### 0.2 The REAL SNI divergence is one no document names — longest-suffix precedence

Two chains, `*.b.foo.test` (LONG) and `*.foo.test` (SHORT), both matching SNI `a.b.foo.test`:

| side | LONG declared first | LONG declared second (pure block reorder, same byte length) |
|---|---|---|
| **reference** | `LONG` | **`LONG`** |
| **subject** | `ERR:ambiguous` | **`ERR:ambiguous`** |

**The reference prefers the longest matching suffix and is ORDER-INDEPENDENT** — proven by reversing
the array with everything else byte-identical, which is the only arm that can tell "longest wins" from
"first wins". The discriminating control is SNI `x.foo.test` returning `SHORT` in **both** configs: the
short chain demonstrably serves traffic, so its loss on `a.b.foo.test` is a precedence decision and not
a dead chain. Exact `server_names` also beat a matching wildcard **declared later** (`EXACT`).

**envoy-go returns `(nil, ErrAmbiguousChainMatch)` in both orders** and `manager.go`'s TCP path then
logs `chain-match: ambiguous filter_chain selection` and **CLOSES THE CONNECTION** (verified at the
`SelectChain` call site). Mechanism: `sniSpecificityRank` buckets every `*.`-prefixed pattern to rank
**1**, `breakTie` finds equal rank, falls through its cascade and returns `nil`.

⚠️ **The same `breakTie` compares CIDR prefixes BY LENGTH in slots 1 and 6 and refuses to compare SNI
suffixes by length in slot 2.** The asymmetry is inside one 37-line function.

### 0.3 `chainmatch.go:56-58` claims a build-time detection that does not happen

The doc comment on `ErrAmbiguousChainMatch` says the listener manager *"detects this at NewManager-build
time … and rejects the bootstrap."* Measured on the two-wildcard shape:
`findIdenticalChainSpecs=(0,0,false)` — the two specs serialise to **different** `chainSpecKey`s, so
boot accepts the config. **The failure is per-CONNECTION, on the first client whose SNI matches both.**
A config can pass `--mode validate` and still drop live traffic.

### 0.4 `catchAllCount > 1` is DEAD CODE, not a message typo

The banked item calls this *"`catchAllCount`'s ERROR MESSAGE NAMES `server_names` WHILE ITS PREDICATE IS
`spec.Empty` — one line, outside every sentinel window."* Right about the size, wrong about the nature.

A 91-pair sweep over 14 chain shapes (deliberately including `direct_source_prefix_ranges`, the one
match field `parseChainSpec` drops) found **45 pairs where `catchAllCount > 1` fires and ZERO where it
fires without `findIdenticalChainSpecs` also firing**. ⇒ **NOT CONSTRUCTIBLE.** The foreclosing
mechanism, named rather than assumed: `spec.Empty` is set iff `isAllZeroChainSpec` holds over exactly
the nine fields `chainSpecKey` serialises, and `chainSpecKey` short-circuits `if s.Empty { return
"EMPTY" }` — so any two Empty specs collapse to one constant key, unconditionally.

Deleting the check is `0 insertions / 7 deletions` with **0 of 392 tests RED**.

### 0.5 The only test guarding that message is VACUOUS — satisfied by its own fixture's name

`TestNewManager_MultiChain_TooManyCatchAlls_Errors` guards with
`!strings.Contains(err.Error(), "catch") && !strings.Contains(err.Error(), "server_names")`. With the
catch-all check removed the error becomes the identical-spec message, and the predicate was measured
directly:

```
contains catch: true      <-- from the test's OWN listener name "l_2catch"
contains server_names: false
guard fires: false
```

**The assertion passes because the fixture named its listener `l_2catch`.** A gate that reads its own
input. It would stay green through the deletion of the production check it claims to pin.

### 0.6 The reference does NOT validate `filter_chain_match` uniformly — and envoy-go errs in BOTH directions

Measured on one image, one function's worth of adjacent proto fields:

| field | reference | envoy-go | direction |
|---|---|---|---|
| `server_names` shape | **REJECTS** `*`, `bar.*.test`, `*foo.test`, `foo.test.*` at listener-add: `partial wildcards are not supported in "server_names"` | accepts **all** of them, plus `**.test`, `*.`, `""` | **too PERMISSIVE** |
| `transport_protocol` value | **ACCEPTS** any string; chain simply goes dead | boot-rejects outside a four-member set | **too STRICT** |

⇒ *"the reference validates `filter_chain_match`"* is not a uniform claim, and the two defects sit in
**adjacent branches of `parseChainSpec`** pointing opposite ways. Neither could have been predicted from
the other.

### 0.7 envoy-go implements a `"*"` tier the reference will not boot

`sniMatchAny:282` and `sniSpecificityRank:321` both special-case `p == "*"`, and ADR-0033 clause 9,
ADR-0078 clause 9 and `BEHAVIOR_CONTRACT.md` all document a *"universal wildcard"* precedence tier.
The reference **rejects `server_names: ["*"]` outright**, rc=1. The tier describes a config that cannot
run on the thing this project is a differential of. **RECORDED, NOT REPAIRED** — see §4.3.

### 0.8 Case-sensitivity diverges, on the PATTERN side

The reference is case-insensitive on both sides: it accepted `server_names: ["*.FOO.test"]` and matched
`bar.foo.test`, and an uppercase `BAR.FOO.TEST` was captured **on the wire** (`openssl -trace` showing
`server_name` extension bytes `42 41 52 2e 46 4f 4f 2e 54 45 53 54`) and still matched `*.foo.test`.
envoy-go: `sniMatchAny("*.FOO.test", "bar.foo.test")` reads **false**. Banked, not chartered.

### 0.9 A repair's SHAPE encodes an unmeasured parity answer — and here the measurement picks the LARGER one

Two candidate SNI repairs were prototyped and sized: **A1** single-label wildcard, `3 / 1`; **A2**
longest-suffix tie-break, `24 / 0`. They are not interchangeable — A1 changes *which SNIs match at all*,
A2 changes only *which of several already-matching chains wins*. **Size is not the tiebreaker and the
smaller one is wrong** (§0.1). `reference_fix_shape_encodes_unmeasured_parity`, live again.

### 0.10 Three green repairs, and the green is a COVERAGE finding

A1, A2 and the `catchAllCount` deletion each turned **0 of 392 tests RED**. That is not a clean bill.
The negative control — disarming the wildcard branch outright with `if false && strings.HasPrefix(...)`
— reddened `TestNewManager_MultiChain_SNIWildcard` and `TestNewManager_MultiChain_Specificity`,
`RC=1 FAILlines=5 RUN=392`. **The site is live and merely unpinned on the axes that matter.** `RUN=392`
was identical in every arm, so no arm silently dropped tests.

### 0.11 My own occurrence-set matcher was blind — twice, in opposite directions

Enumerating the universal-wildcard tier's carriers: `universal.wildcard|universal wildcard` found
**five** sites; `exact *> *suffix|> *universal` found **seven**; **neither found all of them.** The
first missed `BEHAVIOR_CONTRACT.md`'s `exact > suffix > universal > catch-all`, which omits the word
after *universal*; the second missed two `chainmatch.go` comments that hyphenate it. The union is
**eight**. Method notes 49 and 56, reproduced by this stage against itself. **Quote the union, and say
which matchers produced it.**

### 0.12 The router's own port-band census figure is self-falsifying

`next-prompt.txt` records *"a text-source search reads TWO `156xx` hits"*. Re-run at this tip,
`git grep -nE '\b156[0-9][0-9]\b'` reads **THREE** — a fuzz rate in phase 05.1's PROGRESS, a frame byte
size in phase 88's PLAN, **and the router's own sentence recording the census**, which spells both
numbers. The figure was false at its own publishing commit. Same self-incrementing class as the archive
guard. **The band is still free**: none of the three hits is a port, and `ss -tan` + `ss -uan` over all
states read zero live sockets in `15600-15699`.

### 0.13 Method note 26's post-roll prediction is STALE, and both predictions are still in the tree

Note 26 (written at the phase-97 PLAN close) predicts the post-roll list `09-09, 09-09, 09-08, 09-08,
09-07` — *"a SINGLETON at the tail, so the next close's date read may pick alone."* The phase-97 IMPL
then rolled `STATE.md` again. Measured pre-roll histogram at this tip: **three at 2026-09-09, two at
2026-09-08** — a **TIE AT THE TAIL, TWO-WIDE**. The date read does **not** pick alone; list position
must break it. The IMPL's own roll predicted this correctly in `next-prompt.txt` while leaving note 26's
contradicting sentence standing. **Two live predictions, one right.** Method note 68's class.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

Bring envoy-go's `filter_chain_match.transport_protocol` **config-acceptance** into parity with the
reference: stop boot-rejecting values outside `{"", "tls", "raw_buffer", "quic"}`, accept the field as
the free-form string the proto declares, and let a non-matching value make the chain ineligible rather
than make the process refuse to start.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

Measured repair costs, all one production file, all from `git diff --numstat` on a built and run
prototype:

| candidate | prod diff | tests RED / 392 | reference measured? | differential surface |
|---|---|---|---|---|
| `catchAllCount` deletion | `0 / 7` | 0 | **NO** | **none** — no cross-side behaviour changes |
| **`transport_protocol` reject (THIS ROW)** | **`1 / 6`** | **1** | **YES, 7 arms** | **boot + serve, cross-side** |
| SNI longest-suffix (A2) | `24 / 0` | 0 | YES, 5 arms | needs a new fixture arm |

The `catchAllCount` deletion is cheaper by every figure and is **not chartered**, because it has **no
cross-side surface at all** — it deletes an unreachable check and changes only which of two subject-side
error messages an already-rejected config produces. This project's phase-done gate is a differential
one; a row that cannot move a cross-side arm is a fold-in, not a phase. It is banked in §4.1 **with its
measurements**, so the next pick is better informed than this one was.

The SNI longest-suffix repair is the **most severe** defect on the table — a live connection the
reference serves and envoy-go drops. It is deliberately **not** picked: it is 24 lines against 1, it
carries three entangled siblings (§0.7, §0.8, and the shape question of §0.9), and the standing
directive says smallest defensible **first**, not most severe first. Phase 97 rejected its own strongest
candidate on exactly this ground.

What this row buys, plainly: an operator whose bootstrap carries any `transport_protocol` string outside
four literals gets a **process that will not start** where reference Envoy starts and serves. It is
fail-CLOSED, it is reachable from a plain static YAML with no internal API, and the exit code is 1 with
a message naming the value.

### 1.3 What this row does NOT buy — stated plainly

- It does **not** touch SNI matching, specificity, or the `"*"` tier. Those stay banked (§4.2, §4.3).
- It does **not** add a `no_filter_chain_match` counter. envoy-go emits **none** — confirmed at this tip;
  `test/fixtures/0121-listener-default-chain-tls/driver/driver.go` already records the absence. See §3.2.
- It does **not** decide whether the reference's *acceptance* is a good idea. Parity is the contract.

---

## 2. The defect, MEASURED — both sides, matched negatives on both

### 2.1 The rigs and their controls, stated BEFORE the results

**Reference.** Image run BY DIGEST after verifying `ENVOY_TARGET.md` lines 3-4. Admin `15600`, listener
`15601`, both inside the reserved `15600-15649` band, censused free with `ss -tan` and `ss -uan` over
all states before first bind. Readiness gated on `/ready` returning `LIVE` — never a sleep. Certs
delivered `inline_string`, never `filename:`, per the phase-97 precedent. Distinguishable
`direct_response` bodies, corroborated on every arm by `http.<prefix>.downstream_rq_total` from
`/stats`; **the counters agreed with the bodies on every arm.**

**Subject.** Detached worktree at master tip `8194291e`. Unit arms through the real `NewManager`; the
operator arm through a binary built `-o` into scratch. Two bootstraps whose `diff` is a **single line**.

**The controls that make the results readable, run first:** an arm with `transport_protocol: "tls"`
(the shape is otherwise valid) and an arm with `"raw_buffer"` on a plaintext connection (the chain slot
is *reachable*, so a zero elsewhere is interpretable, not structural).

### 2.2 The result

| arm | the ONE field that differs | reference | subject |
|---|---|---|---|
| NEG CONTROL | `transport_protocol: "tls"` | validate rc=**0**; boots; plaintext GET falls to default | build **OK** |
| POS CONTROL | `transport_protocol: "raw_buffer"` | plaintext GET served **by that chain** | build **OK** |
| **THE ARM** | `transport_protocol: "totally_bogus_value"` | validate **rc=0**, `configuration OK`; boots to `starting main dispatch loop`; `/ready` = `LIVE`; plaintext GET → default chain, `no_filter_chain_match: 0` | **rc=1**, `listener: "l_tp": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty` |
| TLS variant | bogus value, both chains TLS, TLS client | serves the default chain | same reject |
| no-default variant | bogus value, **no** default chain | connection dropped, `no_filter_chain_match: **1**` | same reject |
| matched neg for the above | `"raw_buffer"`, no default chain | **served** by the chain | build OK |

**The subject rejection is attributable to the VALUE alone**: the two bootstraps differ by one line, and
the `"tls"` twin exits 0.

### 2.3 The mechanism

`internal/listener/manager.go`, in `parseChainSpec`, immediately after the `server_names` copy:

```go
switch tp := fm.GetTransportProtocol(); tp {
case "", "tls", "raw_buffer", "quic":
    spec.TransportProtocol = tp
default:
    return nil, fmt.Errorf("transport_protocol %q must be \"tls\", \"raw_buffer\", \"quic\", or empty", tp)
}
```

The proto field is a **free-form string**, not an enum. `chainmatch.go`'s `matches()` already does the
right thing for an unknown value — `c.TransportProtocol != "" && c.TransportProtocol != inputs.TransportProtocol`
returns false, so the chain is simply never eligible, which is precisely the reference's observed
behaviour. **The runtime is already parity-correct; only the parse-time gate is not.**

### 2.4 Arms NOT run — recorded, not glossed

- **Which chain's certificate was presented.** Every chain shares one self-signed leaf, so no arm
  discriminates transport-socket identity. Same boundary the phase-97 SPEC §2.5 records.
- **`transport_protocol` on a QUIC listener** — phase 97 §2 covers it; not re-run.
- **Whether the reference's acceptance is versioned behaviour.** One pinned image, one version.
- **Three-way wildcard depth precedence.** Two-way was measured in both orders; the three-way
  generalisation is inferred and **agreeing measurements do not generalise to the class**.

---

## 3. Hazards for the SPEC

### 3.1 The repair must not silently widen to the QUIC path's `"quic"` literal

`internal/listener/quic_test.go:691` narrates the four-member set as load-bearing for two arms. Removing
the reject removes the *guard*, not the *semantics* — but the SPEC must confirm by execution that no
QUIC arm depended on the reject firing.

### 3.2 The no-default-chain arm depends on a stat envoy-go does not have

The reference books `listener.<addr>.no_filter_chain_match: 1` when the bogus chain is the only chain.
envoy-go emits **no such counter anywhere** — `git grep 'no_filter_chain_match' -- '*.go'` returns only
comments, one of which is `0121`'s driver recording the absence. **The fixture must carry a default
chain** so the arm does not depend on an unimplemented stat surface, or the row grows into a stat row.

### 3.3 One test must be RE-POINTED, not relaxed

`TestParseChainSpecRejectsUnknownTransportProtocol` (`manager_test.go:1691`) is the single RED. It is a
real pin on real behaviour that this row deliberately deletes. It must be re-pointed to assert
**acceptance** plus **non-eligibility**, which is a stronger property than the reject it replaces. The
comment at `manager_test.go:1690` narrating the four-member set dies with it.

### 3.4 The differential fixture shape is an ordinary cross-side one, not a boot-reject one

Today the reference boots and the subject rejects, which is the *inverse* of
`SubjectOnlyBootRejectFixture`. **After** the repair both sides boot and serve, so **one** ordinary
cross-side fixture directory suffices — one directory dispatches to one runner branch, and this row
needs only the repaired arm. That is the material difference from the banked `stat_prefix` candidate,
which needs two.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The `catchAllCount` redundancy — **REJECTED: cheapest by every figure and has NO cross-side surface**

`0 / 7`, 0 tests RED, NOT CONSTRUCTIBLE without the identical-spec check also firing (§0.4), and its only
guard is vacuous (§0.5). It changes no observable cross-side behaviour. **Banked as a fold-in with its
measurements**, not as a row.

### 4.2 SNI longest-suffix precedence (A2) — **REJECTED for size; now the STRONGEST banked candidate, and MEASURED on both sides**

`24 / 0`, 0 tests RED (a coverage finding, §0.10), reference measured order-independent longest-suffix
with the reversal control. **This is the most severe defect on the table** — the reference serves, envoy-go
closes the connection. It supersedes the HCM `stat_prefix` panic as the front-runner: comparable value,
and its reference side is already measured where the panic's repair needs a get-or-create across **ten**
files. Everything the next BRAINSTORM needs is in §0.2, §0.3 and §0.9.

### 4.3 `server_names` partial-wildcard acceptance — **REJECTED; entangled with a documented tier**

Reference rejects four shapes envoy-go accepts (§0.6). The repair deletes envoy-go's `"*"` tier, which is
documented at **eight** sites across ADR-0033 clause 9, ADR-0078 clause 9, `BEHAVIOR_CONTRACT.md` and
`chainmatch.go`'s own comments (union of two matchers, §0.11), and pinned by two assertions in
`chainmatch_test.go` plus three narrating comments in `quic_test.go`. Larger than it looks.

### 4.4 SNI case-sensitivity — **REJECTED; banked (§0.8)**, and it should land WITH A2, not alone.

### 4.5 The HCM `stat_prefix` duplicate-registration panic — **REJECTED; NOT re-derived, and deliberately so**

Phase 97's BRAINSTORM §0.1 and §4.1 adjudicated it in full with measurements: ten files register through
the panicking constructor, the repair mints a boot panic that `TestMain_BootPanicIsVisible` explicitly
says must be re-pointed, and it needs **two** fixture directories at a measured floor of ~1359-1393 added
lines each. **Re-deriving a closed row's measurement is waste.** Cited, not repeated.

### 4.6 The driver-owned receiver port race — **REJECTED for size; unchanged from phase 97's adjudication.**

### 4.7 The doc riders — **REJECTED as rows.** `:229`'s stale swallowed-panic claim stays **deliberately
unfixed**: it sits inside a sentinel window on a margin of one. Recorded, not tidied.

---

## 5. Family attribution

**A Listener / chain-match MAINTENANCE row claiming NO family ordinal**, on the row-85-through-91,
95, 96 and 97 precedent. It extends no family charter and opens none. `BOOTSTRAP_PROMPT.md` §9's
Listener surface closed at 07.2; this is a parity repair inside it.

---

## 6. The cost FLOOR — prototyped, run, reverted

`1 insertion / 6 deletions` in `internal/listener/manager.go`, built and run against the full
reverse-dependency package set (`cmd/envoy-go`, `internal/admin`, `internal/boot`,
`internal/listener/...`, `validate`): `RC=1 FAILlines=4 RUN=392`, one distinct test RED.

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE.** `reference_measured_prototype_is_a_lower_bound` has fired
**eighteen consecutive rows**; phase 97's floor moved 12x between its BRAINSTORM and its SPEC **on a
shape that was also wrong**. The row additionally owes a re-pointed test, a new fixture directory (recent
floors: **1359** and **1393** added lines), an ADR, and the ledger/roadmap edits.

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

`git grep -ln 'server_names' -- 'test/fixtures/*'` over **124** fixture directories returns exactly
**one**: `0002-tls-tcp`, and it uses two **exact** names. `0008-listener-chain-match` covers the
8-dimension surface via `destination_port`, `source_prefix_ranges`, `source_ports` and an empty match —
**no `server_names`, and no `transport_protocol` arm anywhere**. The only `transport_protocol` string
under `test/` is a comment in `0002`'s driver. **Nothing in the differential suite would move under this
row's repair, which is exactly why the row needs a new fixture.**

### 7.2 What the gate must do, and the trap in it

The fixture must boot **both** sides on a listener carrying a bogus-`transport_protocol` chain **and** a
default chain, then drive one request and assert it is served by the default chain on both sides. The
trap: **that arm is green on the reference today and would be green on a repaired subject for the wrong
reason** — a subject that ignored the field entirely also answers "default". Pair it with a
`transport_protocol: "raw_buffer"` arm on a byte-identical listener that **must** be served by the
indexed chain. One positive arm cannot tell "the value was accepted and did not match" from "the
dimension is unenforced".

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN INSERTION

### 8.1 PRE-INSERTION, at `8194291e`

Check (1), `want=129`: **SILENT.** Check (2): **SIX**, at `:207 :213 :219 :229 :235 :243`. Check (3):
**SILENT.** `stop` verified **ABSENT** at the git root and in this stage's worktree, and **deliberately
NOT created**.

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

- **NC-A** — row 62 doctored to `in-progress`, substitution **inspected before trusting the result**
  (`NC LANDED? [ in-progress ]`): **ONE** line, `NOT DONE: row 62`.
- **NC-B** — `want=128` on the real file: **ONE** line,
  `GATE FAIL: examined 129 data rows, expected 128`.
- **NC-C** — check (3): residual **0**, `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D** — `-family row` under `--`: **96** occurrences / **68** lines.

### 8.3 The check-(2) positive control

Both phrases substituted, and the substitution **asserted**: residual **0**, `candidatesXX` **6**.

### 8.4 The escape-aware malformed set

`sed 's/\\|//g' … | awk` with **no file argument to awk**: exactly **{57, 69}**, at file lines **119**
and **131**, NF **9** and **10**. Every other data row reads 8.

### 8.5 Per-line digests

Trailing newline **INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex) — **all six BYTE-IDENTICAL to the
phase-97 IMPL close**: `207 10d7807bf02d` · `213 4a92f7e62fc6` · `219 2a7eb298b9fd` · `229 242e53c6f7a3`
· `235 b2680e6f4fbf` · `243 6caa1c3ce0e7`.

### 8.6 Provenance of the pick against the six windows

The subject came from **no** deferred-candidate window. All six were read in full: HTTP/3 (`:207`), gRPC
(`:213`), xDS (`:219`), Observability (`:229`), Runtime (`:235`) and Operational tooling (`:243`). None
names chain-match config acceptance. The pick comes from the **banked, not-chartered** list carried in
`next-prompt.txt` — and **its stated framing was refuted before it was adopted** (§0.1, §0.6).
⚠️ **This row's ROADMAP cell was written so that it spells NEITHER sentinel match phrase.**

### 8.7 POST-INSERTION — measured on the other side of this stage's own ADD

This stage **ADDS** a row, so the denominator moves and the NC shapes change. Measured, not forecast, at
the commit that ships these edits. `ROADMAP.md` **247 -> 248** lines, data rows **129 -> 130**, row 98 at
file line **160**, field count **8 under BOTH forms**.

- **Check (1), `want=130`: NON-SILENT, ONE line — `NOT DONE: row 98`.** That is this row's own
  `in-progress` status, the normal mid-phase state, not a fault.
- **Check (2): still SIX**, at `:208 :214 :220 :230 :236 :244` — every window shifted **+1** by the
  insertion. ⚠️ **All six per-line digests are BYTE-IDENTICAL to §8.5** (trailing newline included):
  `208 10d7807bf02d` · `214 4a92f7e62fc6` · `220 2a7eb298b9fd` · `230 242e53c6f7a3` ·
  `236 b2680e6f4fbf` · `244 6caa1c3ce0e7`. **The ADD moved the lines and changed none of their bytes.**
- **Check (3): SILENT.**
- **NC-A: TWO lines** — `NOT DONE: row 62` and `NOT DONE: row 98`, substitution inspected first.
- **NC-B (`want=129`): TWO lines** — `NOT DONE: row 98` and
  `GATE FAIL: examined 130 data rows, expected 129`.
- **NC-C: FIRED**, residual 0. **NC-D: 96 / 68** under `--`, unchanged by the ADD.
- **Check-(2) positive control: 6 substitutions ASSERTED, residual 0.**
- **Escape-aware malformed set: still exactly {57, 69}**, at file lines **119** and **131**, NF 9 and 10.

⇒ **THE SENTINEL DOES NOT FIRE ON EITHER SIDE OF THIS STAGE'S OWN EDIT. `stop` WAS EVALUATED AND
DELIBERATELY NOT CREATED.** ⚠️ **The margin remains ONE: check (2)'s six is still the only structural
barrier, and check (1) is now merely borrowing a voice from this open row.**

⚠️ **A demonstration, not a warning: `grep -c` on zero matches PRINTS `0` AND EXITS 1.** Gating this
row's cell for sentinel phrases with `grep -coE ... || echo "0 (none)"` emitted **two** zeros. The cell
is clean — it spells neither match phrase — but the instrument that said so was miscounted by its own
fallback. Capture with `v=$(cmd || true)`.

---

## 9. Findings the next stage must not re-learn

1. **A banked "UNMEASURED" claim can be false in the direction that kills the candidate.** Measure before
   adopting, not after chartering.
2. **The reversal arm is the only one that separates "longest wins" from "first wins."** Reorder the
   array and change nothing else.
3. **A test can be satisfied by its own fixture's name.** §0.5. When a guard uses `Contains`, check what
   else in scope contains the token.
4. **`NOT CONSTRUCTIBLE` is a real verdict and needs a named mechanism.** §0.4 names `chainSpecKey`'s
   `EMPTY` short-circuit; a 91-pair sweep without it would be an absence claim, not a proof.
5. **Three repairs went green and the green was a coverage finding.** The disarming NC is what made the
   zero readable. Score per axis, never per run.
6. **An occurrence-set matcher can be blind in two different ways.** Run more than one and quote the
   union (§0.11).
7. **A router's own census sentence can be a hit in the census.** §0.12.
8. **Two contradicting predictions can both be live in the tree.** §0.13 — the newer one was right, and
   the older one is still standing where a reader will obey it.

---

## 10. What the SPEC owes

1. **Measure the reference's `transport_protocol` acceptance on a QUIC listener** — this BRAINSTORM
   measured TCP only, and phase 97's §2 is TCP-adjacent evidence, not this arm.
2. **Confirm by execution that no QUIC arm depended on the reject firing** (§3.1).
3. **Decide and justify the re-pointed shape of `TestParseChainSpecRejectsUnknownTransportProtocol`** —
   acceptance **plus** non-eligibility, not merely deletion (§3.3).
4. **Charter exactly one fixture directory** with the paired bogus/`raw_buffer` arms of §7.2, and
   **allocate its reference port from a censused band** — fixtures are at **124**, tail
   `0122-quic-chain-selection`, so `0123` is free and the derived convention gives reference port
   `15123`, verified free before use.
5. **Draft the ADR §Context and ARM the house `PROPOSED` guard** — a BRAINSTORM drafts none, so the guard
   is correctly at zero right now; next-free is **ADR-0320**.
6. **Enumerate the occurrence set of every claim the repair falsifies** and set-difference it against the
   byte-untouched roster **before** writing the task list (method note 62). The four-member set is
   narrated in at least `manager_test.go:1690` and `quic_test.go:691`.
7. **State whether the row owes a `BEHAVIOR_CONTRACT.md` ledger entry.** This row anticipates **+0 stat
   names**; the phase-96/97 `+0, UNCHANGED` form applies, quoting no absolute.

---

## 11. Probe hygiene

Reference measurement ran in **one Docker-serialised agent**; the subject agent used **no Docker**. Every
container this stage created carried the `p98ref-` prefix and was removed **BY NAME**; the sibling
`curl-world` session's **17** containers were verified present and untouched before and after. A
transient `zealous_dewdney` and a `reaper_*` testcontainers Ryuk appeared mid-window and were **not
touched** — the reaper is created by the differential harness itself and is reused.

Ports: reference agent `15600-15649`, subject agent `15650-15699`, both inside the band this stage
censused (§0.12) and below the 32768 ephemeral floor. Both agents removed their worktrees and proved
`git status --porcelain --untracked-files=all` clean apart from the pre-existing untracked `.claude/`
directory, which was already present in this session's opening snapshot. **Neither agent committed
anything.**
