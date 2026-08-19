# Phase 90 — `h2-host-authority-normalization` — SPEC

**Stage:** lifecycle-state **1 -> 2**. Base master `f15d4f4e`, branch `phase-90-spec`.
**Execution style:** subagent-driven per `feedback_execution_style` — three probe agents on disjoint
detached worktrees, disjoint sub-32768 port bands, private scratch each, **each committing nothing and
each proving its tree clean**; the controller re-derived every load-bearing claim BY EXECUTION on its own
instruments, and refuted three of them.

**Anticipated ADR-0312.** §Context is drafted here (§14) and the strict `^> **STATUS: PROPOSED` guard is
RE-ARMED **0 -> 1**. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` are **BYTE-UNTOUCHED**; row 90 stays
`in-progress` and `want` stays **122**.

---

## 1. The charter, and the bound this stage puts on it

**NORMALIZE `host` AND `:authority` ON THE HTTP/2 DOWNSTREAM LEG so the upstream request carries exactly
ONE authority, as the reference does.**

The bound, decided in §3 and stated up front so nothing downstream reads it more broadly:

- **IN:** arms **A** (`:authority` + `host` ⇒ drop the regular `host`) and **B** (`host` only ⇒ promote it
  into `:authority`), on the **H/2 downstream leg only**.
- **OUT, DEFERRED as a named follow-on:** arm **C** — the reference's reject of an *invalid* authority.
- **OUT, NAMED DEPARTURE:** the H1 duplicate-`Host` polarity (H1-D).
- **OUT, CLOSED AS A NON-DIVERGENCE:** H1-E.
- **OUT, MIS-ATTRIBUTED AND SPLIT:** H1-B (see §7 — the 426 is the HTTP/**1.0 version**, not the missing
  `Host`), with its genuine half **H1-B′** newly recorded and deferred.

---

## 2. Method, and what this stage refuted

Every stage's job is to refute its predecessor by execution. This one refuted **six** BRAINSTORM claims,
one of them the row's own headline sentence, and **one of the six is a refutation of a refutation**.

| # | BRAINSTORM claim | verdict |
|---|---|---|
| 1 | *"an empty authority is a ROUTE-MATCHING input, not merely a wire artifact"* (§3, and **ROADMAP row 90**) | ⚠️ **REFUTED** — §4 |
| 2 | *"closing C means a new reject in `buildRequest`"* (§6 note 4) | ⚠️ **REFUTED** — wrong SCOPE; §5 |
| 3 | *"an explicitly-empty `:authority` is a connection error"* (§3) | ⚠️ **REFUTED** — the rule is VALIDITY, and it is applied to `host` INDEPENDENTLY; §5 |
| 4 | arm C's reaction is a fixed teardown booking `downstream_cx_protocol_error` | ⚠️ **REFUTED** — config-dependent, and the stat set is incomplete; §5 |
| 5 | H1-B: *"HTTP/1.0, no Host ⇒ reference 426"* attributed to the missing `Host` | ⚠️ **REFUTED** — the 426 is the VERSION; §7 |
| 6 | *"a per-line grep of all six sentinel windows … returns **0**"* (§5) | ⚠️ **REFUTED** — reads **1**, at my tip AND at the BRAINSTORM's own; §11 |

**CONFIRMED, not refuted:** the four measured arms P/A/B/C themselves; the `helpers.H2RoundTrip`
structural inexpressibility; the four-fixture H2-capable downstream set; *"no existing test pins the
defect"* (now at a far tighter denominator); arm C's teardown signature.

⚠️ **Refutations 1 and 2 both landed on the row's own mechanism paragraph.** The BRAINSTORM's §3 is the
most-cited block in the row and two of its three load-bearing sentences do not survive execution.

---

## 3. Q1 — **D-90-SCOPE**: arms A and B, H/2 downstream leg. Arm C and the H1 leg are OUT.

**Decided by measurement, not symmetry.** Four grounds, each from a run:

**(a) Arm C is a DIFFERENT charter, not a larger dose of this one.** The registered charter is *"the
upstream request carries exactly ONE authority."* Arm C is not about carrying one authority; it is about
**rejecting an invalid one**. §5 shows the reference's rule there is authority **VALIDITY** applied
independently to two fields — a validation concern with its own predicate, its own stat, and its own
config switch.

**(b) Arm C costs MORE THAN THE WHOLE OF A+B, and is separable.** Measured prototypes (§5.4): the reject
is **+32/−0** at stream scope or **+31/−0 over TWO files** at connection scope, against A+B's **+34/−0**
(§9) — and neither reject shape is byte-parity with the reference default. A+B is coherent without it: the
promotion rule fires on `:authority` **ABSENT**, and arm C's shape has `:authority` **PRESENT**, so the two
never interact.

**(c) The H1 leg is three separable questions with three different verdicts**, not the one cost the
BRAINSTORM §6 note 5 booked (§7). Two of the three are not even host/authority defects.

**(d) Nothing in scope is left incoherent.** The one shape where the deferral is visible is arm C itself:
after this row, an explicit `:authority: ""` alongside a `host` emits `:authority=""` **alone** (the `host`
is suppressed by the A-rule) rather than today's two authorities. That is strictly closer to the reference
than the tip, and it does not pre-empt the deferred reject. **Stated here so the IMPL does not discover it.**

---

## 4. ⚠️ Q6 — **THE ROW'S OWN HEADLINE SENTENCE IS FALSE.** Host/authority is NOT a route-matching input.

BRAINSTORM §3 and **`ROADMAP.md` row 90** both assert: *"`buildRequest` … sets both `u.Host` and
`http.Request.Host` from `authority`. So on arm B the route table sees an empty Host … an empty authority
is a routing input, not just a wire artifact."*

**Refuted three independent ways, all re-read by the controller at `f15d4f4e`:**

1. **The route matcher cannot see the request.** `internal/filter/hcm/route.go:127`
   `(*routeTable).match` reads `req.URL.Path` and nothing else, and the `routeMatch` interface
   (`route.go:18`) is literally `matches(path string) bool` — `req.Host` is **not reachable from it**.
   `git grep '\.Host' -- internal/filter/hcm/route.go` ⇒ **rc=1, zero hits.**
2. **Domain-based virtual-host selection is unrepresentable.** `internal/filter/hcm/config.go:280`
   boot-rejects any vhost whose `domains` is not exactly `["*"]`; `:276` rejects
   `len(virtual_hosts) != 1`.
3. **A header matcher cannot exist either** — `buildMatch` rejects `match.headers` at parse time.

⚠️ **Every filter that reads the authority is unaffected, and this was checked rather than assumed.**
`csrf.go:181-185`, `jwtauthn.go:783-786` and `oauth2/decode_headers.go:250-253` all **prefer `:authority`
and fall back to `Host`**, so both promotion and suppression leave all three yielding the same string.
`extproc/check.go:248`'s `"host"` is a WRITE-protection rule, not a read.

### 4.1 What the real blast radius IS — and it is the differential arm the row should reach for

`H2Request.Authority` has exactly **two** non-test consumers:

| consumer | cite | effect |
|---|---|---|
| `(*ClientConn).RoundTrip` — **unconditional** `:authority` emit | `h2/client.go:817`, value at `:832` | **THE WIRE FIX** — this is the mechanism of arm B's present-and-empty `:authority` |
| `emitAccessLogH2` ⇒ access log **and** tracing span | `accesslog_emit.go:95/:116/:131` ⇒ `accesslog/format.go:53`, `tracing/span.go:113`, `tracing/zipkin.go:99` | **REAL, ASSERTABLE**: on a host-only request `%REQ(:AUTHORITY)%` flips `-` ⇒ the host, and the **Zipkin span NAME** (`zipkin.go:99 Name: s.Authority`) flips `""` ⇒ the host |

⇒ **The observable blast radius is OBSERVABILITY, not routing.** A route assertion cannot discriminate
this row at all; the PLAN must not write one.

⚠️ **The false sentence is in `ROADMAP.md` row 90, which this stage leaves BYTE-UNTOUCHED.** The IMPL
corrects it in the same commit that flips row 90 `done`. Recorded here so it is not inherited silently.

---

## 5. Q4 — **D-90-REJECT: arm C is DEFERRED**, and the deferral is grounded in four measured refutations

### 5.1 ⚠️ The rule is VALIDITY, not emptiness — and `host` is validated INDEPENDENTLY

Controller-reproduced on its own raw-framer client against `envoyproxy/envoy:contrib-v1.37.2`, nine arms,
each on a fresh connection, with a positive control:

```
ARM P    frames=4 status=502 OK        :authority=ok.example.com                 <- ACCEPTED (502 = routed)
ARM C6   frames=4 status=502 OK        :authority=a.example + host=b.example     <- ACCEPTED, mismatch is FINE
ARM C11  frames=4 status=502 OK        :authority=ok.example.com:8080            <- ACCEPTED
ARM B    frames=4 status=502 OK        host only                                 <- ACCEPTED
ARM C1   frames=0 end=EOF              :authority="" + host                      <- TORN DOWN
ARM C3   frames=0 end=EOF              :authority=" "  (whitespace only)         <- TORN DOWN
ARM C5   frames=0 end=EOF              :authority="bad host" (embedded space)    <- TORN DOWN
ARM C12  frames=0 end=EOF              :authority="user@ok.example.com"          <- TORN DOWN
ARM C10  frames=0 end=EOF              :authority=ok.example.com  +  host=""     <- TORN DOWN
```

⚠️ **C10 IS DECISIVE AND IT IS NEW AT THIS STAGE.** A *perfectly valid* `:authority` is killed merely
because an **empty regular `host`** sits beside it. So the reference does not compute an effective
authority and validate that — it validates **both fields independently**. Any rule of the form
`authority != ""` is falsified by C3/C5/C12 (invalid-but-non-empty) **and** by C10 (valid authority,
invalid neighbour). The 502s are an upstream-cluster artifact of the probe rig and are exactly the
discriminator wanted: **routed-and-failed-upstream vs killed-at-the-downstream-codec.**

### 5.2 The stat set is incomplete AND posture-coupled

Controller-read from the reference admin `/stats` after the nine arms:

```
http.ingress_h2.downstream_cx_protocol_error: 5      <- as the BRAINSTORM recorded
http2.rx_messaging_error:                     5      <- NOT in the BRAINSTORM
```
**5 = exactly the five rejected arms.** Under
`http2_protocol_options.stream_error_on_invalid_http_messaging: true`, `downstream_cx_protocol_error` does
**not** move at all while `http2.rx_messaging_error` still does ⇒ **`rx_messaging_error` is the classifier
that holds across both postures.** A differential asserting only `downstream_cx_protocol_error` is coupled
to a config default.

⚠️ **AND NEITHER STAT EXISTS IN THE SUBJECT'S HTTP STACK.** `git grep 'downstream_cx_protocol_error' --
'internal/**/*.go'` resolves to **one directory only — `internal/filter/network/redisproxy`**. So an
arm-C stat-parity assertion would require **minting a new stat**, moving the stat surface off 406 and
breaking the row's `+0`-on-every-axis posture.

### 5.3 ⚠️ The reference's reaction is CONFIG-DEPENDENT, and neither envoy-go shape is byte-parity with its default

The reference default returns **ZERO bytes** — no GOAWAY, no RST_STREAM, not even its own SETTINGS, and a
clean FIN (`err=<nil>`), with the control on the same connection shape returning five frames. One boolean
(`stream_error_on_invalid_http_messaging: true`) turns that into a survivable
`RST_STREAM(PROTOCOL_ERROR=1)` on which a second stream reaches the backend.

### 5.4 ⚠️ The BRAINSTORM named the WRONG SITE — `buildRequest` gives the WRONG SCOPE

BRAINSTORM.md:259: *"closing C means a new reject in `buildRequest`."* Controller-traced:

- `buildRequest`'s error is consumed at `h2/stream.go:288` by
  `_ = s.conn.writeRSTStream(s.id, ErrProtocolError)` ⇒ **STREAM scope, always.** `serverStream.dispatch`
  holds only the `streamConn` interface, whose entire surface is
  `encodeAndWriteHeaders`/`writeData`/`writeRSTStream`. **A connection-level error is not expressible from
  there at any price.**
- The connection-scoped site **does exist and the BRAINSTORM did not name it**:
  `(*serverStream).recvHeaders` (`h2/stream.go:109`) already receives the full decoded
  `[]hpack.HeaderField` **and returns an error**; its caller `conn.go:528` propagates it into the
  `hErr.Stream == 0` discriminator at `conn.go:276/:311/:342`, which selects `emitGoaway` + close over
  `writeRSTStream`. `connError` (`errors.go:99`) is exactly the Stream-0 constructor.

Measured prototypes: **P1** stream-scoped reject in `buildRequest` = **+32/−0**, one file, reproduces the
reference ACCEPT/REJECT partition on all nine arms and emits `RST_STREAM(PROTOCOL_ERROR)` — frame-exact
with the reference's **non-default** posture. **P2** connection-scoped at `conn.go::onHeaders` =
**+31/−0 over TWO files**, and emits a **GOAWAY the reference never sends**. Suppressing it would
contradict `conn.go:210-213`'s own h2spec-driven comment (*"We must send GOAWAY before closing so h2spec
3.5/2 sees it"*).

### 5.5 ⇒ DEFERRED, with the follow-on's spade-work banked

Arm C is a **named follow-on row**, not an absorbed sub-task. What it inherits, already measured: the
nine-arm partition (§5.1), the `isValidAuthority`-not-`!= ""` rule including C10's independent-`host`
finding, the two candidate sites with their scopes and costs, the `rx_messaging_error` classifier, and the
fact that **neither available shape is byte-parity with the reference default** — so that row must open
with a shape decision, not a cost decision.

---

## 6. Q2 — **D-90-INSTRUMENT: a raw-framer H/2 client. `helpers.H2RoundTrip` is REFUTED at the pinned source AND on a live listener.**

The BRAINSTORM's source cites were re-verified **in the module cache**, not quoted. `go list -m
golang.org/x/net` ⇒ **`v0.34.0`**, matching `go.sum`. In
`$GOMODCACHE/golang.org/x/net@v0.34.0/http2/transport.go`:

| cite | measured | verdict |
|---|---|---|
| `:2146 f(":authority", host)` | line **2146** exactly | EXACT |
| `:2162-2166` client-set `host` silently dropped | `2162: if asciiEqualFold(k, "host") \|\| …` / `2163: // Host is :authority, already sent.` / `2165: continue` | EXACT |

`host` is derived at `transport.go:2102-2105` from `req.Host`/`req.URL.Host` — **never from a header**.
`test/helpers/h2.go` inherits both: `H2RoundTrip` builds the URL at `:63`, never sets `req.Host`, and
applies caller fields via `req.Header.Add` at `:68`, straight into the dropped-key loop.

⚠️ **AND IT WAS EXERCISED AGAINST A LIVE LISTENER, not only read.** Three arms through an
`x/net` Transport at a running subject:

```
XNET A  req.Host="auth.example.com" + Header{Host:host.example.com} -> upstream saw NO host field  (200, SILENT)
XNET B  req.Host="" + Header{Host:host.example.com}                 -> :authority="127.0.0.1:<port>"
XNET C  ":authority" as a regular header  -> RoundTrip error: invalid HTTP header name ":authority"
```

**All three shapes are inexpressible and the failure is SILENT (200).** ⇒ **DO NOT WRITE AN ARM AGAINST
`H2RoundTrip`.** This is the phase-89 `order=` class caught two stages earlier.

**The replacement was BUILT AND RUN, both sides.** `0119-grpc-unary-trailers/driver/driver.go` cites all
re-verified EXACT at this tip: `ClientPreface` **474**, `http2.NewFramer` **478**, `hpack.NewDecoder`
**479**, `fr.WriteHeaders` **491**, hand-built pseudo-header list **355-358**. A standalone raw framer
drove all four arms on **one pooled connection**, streams 1/3/5/7, one `hpack.Decoder` per connection —
the dynamic-table hazard **exercised, not dodged** (arm A's block compressed 34⇒29 bytes, arm B⇒15,
proving real dynamic indexing) — and reproduced the full subject/reference divergence.

⚠️ **A recorded hazard is NARROWER than the index states.** `hpack.NewDecoder(n, nil)` SIGSEGVs in
`callEmit` **only for a standalone `dec.Write()`**. Installed as `Framer.ReadMetaHeaders` it is safe:
`frame.go:1541` calls `SetEmitFunc` before decoding and `:1578` restores a no-op, not nil. Both `0119:479`
and the probe pass `nil` and neither crashes.

**Minimal reusable shape:** ~125 lines of client + ~60 of arm table; imports are stdlib +
`golang.org/x/net/http2` + `.../http2/hpack` only.

---

## 7. Q5 — **D-90-H1: the H1 leg is THREE questions, not one, and only one of them is in charter**

The BRAINSTORM §6 note 5 books *"the H1 leg entirely"* as a single deferred cost. Controller-reproduced on
its own raw-TCP Go client (**never `bash /dev/tcp`** — the recorded false-empty class), reference H1
listener:

```
P-1.1        Host: ok.example.com                 -> 502  (accepted, routed)
B-1.0-noH    HTTP/1.0, no Host                    -> 426 Upgrade Required
1.0-withH    HTTP/1.0, WITH a valid Host          -> 426 Upgrade Required     <-- THE DISCRIMINATOR
Bprime-1.1   HTTP/1.1, no Host                    -> 400 Bad Request
E-1.1-empty  HTTP/1.1, Host: <empty>              -> 502  (accepted, routed)
D-dupHost    HTTP/1.1, Host: twice                -> 502  (accepted, routed)
```

⚠️ **H1-B IS MIS-ATTRIBUTED.** HTTP/1.0 **with** a perfectly good `Host` still 426s ⇒ **the 426 is caused
by the HTTP/1.0 VERSION, not by the missing `Host`.** The record's "H1-B" conflates two independent
divergences. They are stat-distinguishable too: the 426 arms booked
`http.ingress_h1.downstream_cx_protocol_error: 2` while the 400 arm booked only `downstream_rq_4xx`.

⚠️ **The genuine host/authority arm on that leg is ABSENT FROM THE RECORD.** Call it **H1-B′**: HTTP/1.1
with no `Host` at all ⇒ reference **400**, subject **200** forwarding an empty `Host:`. **New at this
stage. DEFERRED** — it is in charter but its fix site is `connection.go::dispatchRequest`, a different
file and a different codec from the row's stated mechanism.

**H1-E — CLOSED AS A NON-DIVERGENCE.** Both sides 200, both forward the empty `host`. ⚠️ Its value is as
the standing refutation of any *"the reference guarantees a non-empty authority"* generalization — and
§5.1 sharpens that into a **cross-leg asymmetry inside the reference itself**: an empty authority is a
**connection kill on H/2** (C10) and an **accepted 200 on H/1**. It belongs in prose, not in an arm.

### 7.1 H1-D — **NAMED DEPARTURE, stated affirmatively**

Three measured grounds:

1. **envoy-go is the RFC-conformant side.** RFC 7230 §5.4: a server MUST reject a message with multiple
   `Host` fields. The reference comma-coalesces in wire order (`host: one.example.com,two.example.com`,
   read at the backend) and returns 200. **Parity would make envoy-go stop rejecting something it
   correctly rejects.**
2. **There is nothing in envoy-go to relax — only a stdlib parser to defeat.** The 400 comes from
   `$GOROOT/src/net/http/request.go:1139` `"too many Host headers"`, surfaced at
   `internal/filter/hcm/connection.go:163` (`http.ReadRequest` ⇒ `writeStatusReply(bw, 400, "")`).
   `git grep 'too many Host headers'` over this repo ⇒ **rc=1, zero hits.** Cost of parity, measured:
   **+76/−1** for one arm — roughly **5× the entire A+B floor**.
3. ⚠️ **The parity fix is a request-smuggling-surface change and the existing suite is provably blind to
   it.** The prototype buffers the whole request head and rewrites it before the framing parser sees it;
   `go test -run TestH1Robustness -v` then reports **12 `=== RUN` rows, all PASS** (denominator asserted).
   A full-head-buffering rewrite passes the entire CWE-444 suite untouched.

⚠️ **A structural gap the PLAN must price if it ever takes H1-D:**
`connection_robustness_test.go` carries `TestH1Robustness_ConformantRejections` (`:85`) and
`TestH1Robustness_KnownDivergencesFromEnvoy` (`:143`) — **and no "subject stricter" group exists.** H1-B
and H1-B′ drop into the second group with zero new machinery; **H1-D fits neither.**

`BEHAVIOR_CONTRACT.md:698` already carries a *"Known departure (H1)"* for a different stdlib-driven Host
interaction — precedent for **naming** these rather than fixing them.

---

## 8. Q3 — **D-90-FIXTURE: extend `0004-h2-routing` IN PLACE.** Fixtures stay 121; `0120` stays unconsumed.

### 8.1 The H2-capable downstream set is FOUR — re-derived on TWO independent axes

The phase-89 census failed by counting README prose and Go comments. The method here is comment-immune: a
`go/parser` + `go/printer` stripper re-prints every fixture `.go` with comments removed and string literals
intact. **NCs:** in `0119/driver.go` the phrase `h2 client preface` reads 1 raw ⇒ **0** stripped (comments
really gone) while `:authority` reads 1 raw ⇒ **1** stripped (literals really kept).

- **Axis 1 — who DRIVES H/2.** Import facts (prose-immune) via `go list`: direct `golang.org/x/net/http2`
  ⇒ `{0004, 0119}`; comment-stripped `H2RoundTrip|H2CRoundTrip` call sites ⇒ `{0004, 0079, 0080}`.
- **Axis 2 — what the CONFIG permits.** `codec_type` over stripped Go **and** stripped YAML ⇒ `AUTO` in
  `{0004 (yaml), 0079, 0080, 0119 (go)}`; `alpn_protocols` containing `"h2"` ⇒ **the same four**.

**Union on both axes = `{0004, 0079, 0080, 0119}`.** ⚠️ A YAML-only census reads **1**; a Go-only census
reads **3**; **46 of 121 fixtures ship no `envoy-go.yaml` at all.** The 36 fixtures declaring no
`codec_type` are not H/2-capable despite the proto default of `AUTO`, because
`internal/filter/hcm/filter.go:120-138` runs H/2 on `AUTO` **only** for a `*stdtls.Conn` negotiating
`h2` — there is **no downstream h2c preface sniff**.

### 8.2 Extend vs mint, priced

| option | new files | driver | backend | registration | risk |
|---|---|---|---|---|---|
| **(a) extend `0004`** ✅ | **0** | append a raw-framer arm block (~125 + ~60 lines); must be ADDITIVE — `0004`'s 9 `H2RoundTrip` sites cannot express the arms | **~2 lines** — add `r.Host` to `backends/main.go:119-129` | **none** | `AssertDistribution` demands exactly `[3,3,3]` from body-derived counts (`driver.go:713-733`); raw-framer arms open their own connections and must not hit `/api` |
| (a2) extend `0119` | 0 | framer already present | `GRPCHealthResponder` reflects nothing ⇒ still needs a new BackendKind | none | off-charter for a gRPC-trailers fixture |
| (b) mint `0120` | **≥5** | full driver from scratch (peers measure **669-763** lines + **150-196** of `driver_test`) | new `BackendKind = 39` + a 39th runner `case` | **all three gates** | largest surface, and **gate 2's failure mode is a silent PASS** |

**⇒ (a).** `0004` is the only fixture that is H/2-downstream **and** carries a header-reflecting backend
**and** whose reference container is proven to serve hand-built frames; arm-append into it has three
phases of precedent (87/88/89).

### 8.3 The three registration gates, concrete — and the silent-PASS hazard EXECUTED

1. **Directory + self-registration** matching `discoverFixtures` (`runner_test.go:1462-1497`), with an
   `init()` calling `fixture.RegisterFixture(<name identical to the directory>, drv)`.
2. **Blank import** in `test/differential/runner_test.go` — **121** by the narrowed form,
   set-difference against on-disk dirs **EMPTY**.
3. **Runner/BackendKind wiring** — a new backend process needs a `BackendKind` const (tail **38**) and a
   matching runner `case` (**38** today), plus `BackendCount() >= 1`.

⚠️ **Gate 2's failure mode is a silent PASS, and it was reproduced, not asserted.** `runner_test.go:194-201`
`t.Skipf`s an unregistered fixture, and **no fixture-count gate exists anywhere** — `git grep
'len(fixture.DriverRegistry)\|fixtureCount' -- 'test/**/*.go'` ⇒ **rc=1**. Executed: a bare
`mkdir test/fixtures/0120-nc-probe` yields `--- SKIP … PASS … ok … rc=0`. Directory removed, porcelain
re-verified empty. ⚠️ Note `want` (ROADMAP 122) is a **ROADMAP data-row count, unrelated to the fixture
count** — the two must never be conflated.

### 8.4 **D-90-BACKEND: extend `0004`'s reflection with `r.Host`. BackendKind stays 38.**

`0004/backends/main.go:119-129` reflects `r.Header` only — `git grep 'r\.Host'` over that file ⇒ **rc=1**.
Measured against Go's `x/net` h2 **server**, that costs the row exactly one axis:

| downstream | `r.Host` | `r.Header["Host"]` |
|---|---|---|
| `:authority=A`, no host | `A` | `[]` |
| `:authority=A` + `host=H` | `A` | **`[H]`** ⇐ arm A's defect is visible TODAY |
| no `:authority`, `host=H` | `H` | `[H]` |
| `:authority=""` + `host=H` | `H` | `[H]` ⇐ **collides with the row above** |

⇒ **Arm A discriminates with a zero-line backend edit** (a `Host:` line present vs absent). **Arm B needs
`r.Host`** (~2 lines) to see the promotion. ⚠️ **The `:authority` VALUE axis — ABSENT vs PRESENT-AND-EMPTY
when a `host` co-exists — is UNRECOVERABLE at any price through a `net/http` backend**; it needs a
raw-framer backend, i.e. `BackendKind = 39`. **The row does NOT buy that**: the `host`-presence and
`r.Host` axes together discriminate pre-fix from post-fix and from the reference on both in-scope arms.
**Stated so "asserted" is never read more broadly than the tests support.**

### 8.5 ⚠️ Two constraints the PLAN must carry

1. **If arm C is ever added to this fixture it must be LAST and must NOT be pipelined.** Measured at the
   reference: `P,A,B,C` with all HEADERS written before any read ⇒ **EOF and ZERO backend deliveries — P/A/B
   never arrive even though C was last.** Strictly sequential with C last ⇒ P/A/B all 200 with 3
   deliveries. **Safest shape: one fresh downstream connection per arm.**
2. **Arm C cannot be asserted by stat parity** (§5.2) — the subject has no such stat.

---

## 9. Cost — MEASURED BY COMPILING PROTOTYPE, and the BRAINSTORM's floor is OVERRUN 2.3×

Controller-built at `f15d4f4e` in a detached probe worktree, applied, run, and reverted with
`sha256sum -c` proving the restore:

```
git diff --numstat  ->  34   0   internal/filter/hcm/h2/stream.go     (post-gofmt; gofmt -l EMPTY)
go build ./internal/...  -> rc=0
grep -c 'authoritySeen' -> 6      grep -c 'hostField' -> 7             (SYMBOL-asserted, not build-asserted)
```

**+34 / −0, ONE file, ONE package.** Three independent measurements of the same fix:

| form | cost |
|---|---|
| BRAINSTORM floor | +15 / −1 |
| minimal, comment-free | +14 / −1 |
| **controller's, with promotion semantics written explicitly** | **+34 / −0** |

⚠️ **`reference_measured_prototype_is_a_lower_bound` FIRES AGAIN, and the cause is visible.** The floor is
reproducible as a *minimum*, and it doubles the moment the rule's two guards (`authoritySeen` and
first-occurrence-wins) and their provenance comments are written. **IMPL bands: production ~+35-60, unit
~+120-250, differential ~+150-250** (anchored on phase 89's MEASURED `+688/−24` into this same fixture,
discounted because this row adds arms rather than a reconciler).

**PRICED SEPARATELY AND NOT IN THE BAND:** arm C (**+32/−0** or **+31/−0**/2 files), H1-D (**+76/−1**),
H1-B′, and the H3 leg.

### 9.1 ⚠️ NO EXISTING TEST PINS THE DEFECT — confirmed at a FAR tighter denominator, and the "69" is EXPLAINED

`go test ./internal/... -count=1` with the prototype installed: **RC=0, 70 `ok` packages, 0 `FAIL`**,
anchored panic gate (`^panic:|DATA RACE|SIGSEGV`) ⇒ **0**. An independent agent run with `-v` on both
sides read **8201 `=== RUN` rows, 0 FAIL, IDENTICAL** baseline and prototype.

⚠️ **The BRAINSTORM's "69 packages ok" is not a refutation but a MEASUREMENT ARTIFACT: the clean-tip
denominator is 70.** 69 is what the tree reads *while one package is red* — it was captured during the
`TestAcquireH2Stream_PromoteSkipsDrainingConn` flake. **That flake did not recur in any run at this stage.**

### 9.2 The RED baseline — the discriminating evidence, with a positive control that PASSES

A throwaway table over **both** symbols, run against the UNPATCHED tip:

```
--- PASS: TestZZCtrlProbe_Promotion/P_authority_only        <-- POSITIVE CONTROL: not vacuously failing
--- FAIL: TestZZCtrlProbe_Promotion/A_both
        host on carrier = true, want false
        req.Header Host = "h.example", want absent
--- FAIL: TestZZCtrlProbe_Promotion/B_host_only
        Authority = "", want "h.example"
        req.Host  = "", want "h.example"
--- FAIL: TestZZCtrlProbe_Promotion/C_empty_authority
        host on carrier = true, want false
```
With the prototype installed the same five `=== RUN` rows go **5/5 PASS**. The probe file was deleted and
the worktree proven clean.

### 9.3 ⚠️ `buildRequest`'s authority output is COMPLETELY UNPINNED — the sharpest guard finding

Negative control, executed: setting `authority = "NC2-BROKEN-AUTHORITY"` **unconditionally** in
`buildRequest` — corrupting `http.Request.Host` **and** `u.Host` on **every** H/2 request — leaves
**RC=0, 8201 `=== RUN`, 0 FAIL, 70 ok**. By contrast an unconditional promote in `buildH2Request` reddens
`TestBuildH2Request_PseudoHeaderExclusion` (+4 subtests).

⇒ **The row's unit surface is NOT optional and must cover `buildRequest` specifically.** `buildH2Request`
has demonstrated sensitivity; `buildRequest` has **demonstrated zero**, in both directions.

---

## 10. Q7 — **D-90-SKIP: leave `h2ReconcileSkipKey` UNTOUCHED**, and retire one of its stated grounds

`h2ReconcileSkipKey` (`h2dispatch.go:762`) skips every `:`-prefixed key and `host` case-insensitively.
Phase 89's PLAN §2.1 gives three grounds; ground 3 reads *"the reference-exact fix is to make `host` an
alias of the authority scalar on BOTH legs … Out of scope at `want=121`."*

⚠️ **D-89-HOST WAS NEVER A BEHAVIOURAL PREFERENCE — IT WAS A SCOPE DEFERRAL POINTING AT THIS ROW**, a row
in advance. ADR-0311 §Context ¶4 says it outright: *"`Host`/`host` is deliberately left OPEN for the PLAN
rather than settled by omission"*, and §Consequences **(v)** and **(x)** name arms A and B as *"NAMED, NOT
CHARTERED"*. **That is this row's true provenance cite** — stronger than the `STATE.md:18` the BRAINSTORM
used.

**DECISION: (a) leave the SKIP untouched.** Three measured grounds:

1. **The reference's `host`⇔`:authority` fold has THREE distinct rules for FILTER writes, and
   `reconcileH2DecodeDelta` can express none of them.** Measured against the reference with Lua, one
   `hpack.Decoder` per connection, `x-lua-ran: yes` as a per-arm positive control:

   | filter action | upstream `:authority` |
   |---|---|
   | `replace("host", X)` | **overwritten** to `X` |
   | `add("host", X)` | **comma-joined**: `orig,X` |
   | ⚠️ `remove("host")` | **ABSENT — the authority is DELETED, and the request is forwarded with NO authority at all, 200** |

   ⚠️ **`remove("host")` deletes a value the client sent as `:authority` and never as `host`** — the alias
   is TOTAL, not directional. The reconciler today has two dispositions (`replaced`/`extra`), returns only
   `[]hpack.HeaderField`, and **never touches `h2req.Authority`**; there is no scalar write-back path for
   any of the three.
2. **The reference's empty-authority reject is DECODE-ONLY and does not govern filter writes.** A
   *filter*-written empty authority is forwarded **present-and-empty with a 200**, while a *client-sent*
   one tears the connection down (§5.1). ⇒ **the decode-side rule cannot be extended to filters by a
   parity argument.**
3. **Out of charter.** A filter-written header is a different input from a client-sent one.

⚠️ **BUT WRITE THE RESIDUE DOWN.** D-89-HOST's **ground 2** — *"two contradictory authorities on one
request"* — **is RETIRED BY THIS ROW**, because after promotion there is only ever one authority. Option
(b) therefore becomes *newly defensible at row-done*. Bank it as a named follow-on carrying the
replace/add/**remove** table, so a later reader does not re-derive *"SKIP is still justified"* from a
rationale row 90 has already invalidated.

### 10.1 The other two named under-enumeration sites — both CLAIMS VERIFIED

- **`h2/client.go :: (*ClientConn).RoundTrip`** — `:authority` is one of four **unconditionally** appended
  pseudo-headers (`client.go:817`, value at `:832`). **This is the mechanism of arm B's
  present-and-empty `:authority`.** ⚠️ **A gate lesson found here: `^func (c *ClientConn) RoundTrip(`
  reads 0 — the receiver is `cc`, not `c`.** Anchoring the open paren is not sufficient; **the receiver
  name is part of the anchor.** A new species alongside `reference_symbol_assertion_needs_qualified_name`.
- **`h2dispatch.go :: (*chainDispatchAction).WriteH2`** — injection at `:484-486`, guarded
  `!ok && c.req.Host != ""`, so **today a host-only request gets no `:authority` in the decode map at
  all**; post-promotion it gets the right one. Mirrors at `connection.go:495` (H1) and
  `h3dispatch.go:226` (H3).

### 10.2 The H3 leg — **READ, NOT MEASURED. Labelled so it is never inferred.**

From the pinned `quic-go@v0.54.1` `http3/headers.go`: `parseHeaders` (`:165`) already rejects an empty or
absent `:authority`, and `:209` sets `http.Request.Host = hdr.Authority`. ⇒ **arm B cannot occur on H/3.**
But a regular `host` falls through the `default:` arm into `hdr.Headers` un-suppressed, so **arm A is
READ-PREDICTED to reproduce on H/3 — UNVERIFIED.** And H/3 has an **emptiness** reject but **no validity**
reject, so §5.1's C3/C5/C12 shapes would be accepted there. **Zero probes were run. Nothing here is a
measurement.**

---

## 11. Sentinel — RUN MECHANICALLY, ALL FOUR NCs, ACTUAL OUTPUT RECORDED

`ROADMAP.md` is **BYTE-UNTOUCHED** this stage, so there is one side to run, not two.

```
(1) want=122 -> NOT DONE: row 90                    <-- HEALTHY while row 90 is open
(2)          -> SIX, at :200 :206 :212 :222 :228 :236
(3)          -> SILENT
```
⇒ **TWO checks block the sentinel. It does NOT fire; `stop` was NOT created** (verified absent at the git
root and in the stage worktree).

**All four NCs fired:**

| NC | reading |
|---|---|
| **A** doctor row 62 ⇒ `in-progress` (`NC LANDED? [ in-progress ]` **inspected first**) | `NOT DONE: row 62` **AND** `NOT DONE: row 90` — **BOTH required** |
| **B** denominator `want=121` on the real file | `GATE FAIL: examined 122 data rows, expected 121` |
| **C** strip `gRPC-family row` (residual **2 ⇒ 0** confirmed first) | `NEVER OPENED: gRPC`; **WASM correctly silent** |
| **D** per-arm strip of check (2) | long **5** / short **1** / union **6** |

### 11.1 ⚠️ A SIXTH REFUTATION: the BRAINSTORM's provenance grep reads 1, not 0

BRAINSTORM §5 records *"a per-line grep of all six windows for `authority`, `host header` or `h2-host`
returns **0**."* **Measured: window `:222` returns 1**, at this tip **and** at the BRAINSTORM's own
pre-shift `:221`. The positive control (`deferred`) fires **6/6**, so the grep is live.

**The CONCLUSION survives; the stated MEASUREMENT does not.** The single match is the English word
*"authority"* inside *"acquired ADR authority by proximity"*. A **discriminating** form
(`:authority` + backticked `` `host` ``/`` `Host` `` + `h2-host`) reads **0 across all six**, fires **1**
on row 90's own text, and **0** on a fabricated token. ⚠️ **Use the discriminating form; the recorded one
is contaminated by ordinary English.**

### 11.2 The ARM-A malformed-row guard reconciles exactly, escape-aware

Rows **57** (`NF=9`, line **119**) and **69** (`NF=10`, line **131**); the naive `awk -F'|'` form reads
**17**; row 90 is **`NF=8`, well-formed**. NC: a fabricated extra pipe in row 90 moves it to `NF=10`.

---

## 12. Counts re-derived MECHANICALLY at `f15d4f4e` — never copied

| axis | reading |
|---|---|
| `ROADMAP.md` | **240** lines / **122** data rows; row 90 `in-progress`, all others `done` |
| `-family row` | **95** occurrences / **67** lines · `gRPC-family row` **2** · `Operational-tooling-family row` **3** |
| `DECISIONS.md` | **18277** · **310** headings · tail **ADR-0311** · `^## ADR-0312` ⇒ **0** · `^---$` **216** |
| strict `PROPOSED` guard | **0 at stage start; this SPEC RE-ARMS it 0 -> 1** |
| `BEHAVIOR_CONTRACT.md` | **5962** — BYTE-UNTOUCHED |
| `STATE.md` **63** · `STATE_HISTORY.md` **508** · `BOOTSTRAP_PROMPT.md` **522** · phase dirs **131** | |
| `REVIEW.md` | **37 FILES** (not lines); newest `phases/25.3-…` — the standing REVIEW departure, NAMED |
| fixtures | **121** at `test/fixtures/` (⚠️ `test/differential/fixtures/` returns a SILENT **0**); tail `0119`; **`0120` STAYS UNCONSUMED** |
| fuzzers **55 / 48 files** · BackendKind tail **38** · `go.mod` **+0** · stat surface **406** | |
| blank imports | **121** — ⚠️ **SCOPE-BOUND, see §12.1** |
| slice-only-writer gate | **6** (companion `*H2Request` counter **0**; fabricated selector **0**) |
| module path | `github.com/pgdad/envoy-go` — CONFIRMS `ROADMAP.md`'s five `esalaine` cites as a live documentary defect |

### 12.1 ⚠️ A carried COMMAND is scope-bound even though its NUMBER is right

The index records fixture blank imports as **121** via `grep -cP '^\t_ "[^"]*test/fixtures/'`. **Run
repo-wide that command reads 122**; scoped to `test/differential/runner_test.go` it reads **121**. The
extra hit is `test/fixtures/0018-http-rbac/inputs/driver.go:105`, a `pki` sub-package import — **not a
fixture registration**. ⚠️ **The number is correct only with the file scope stated.** Same species as the
`406`-vs-`403` unit-and-scope lesson: **before accepting or restating a count, reproduce BOTH the unit and
the SCOPE.**

### 12.2 The row's anticipated deltas

Stat surface **406** delta **0** · fuzzers **55/48** +0 · BackendKind tail **38** +0 · `go.mod` +0 ·
config fields +0 · blank imports **121** +0 · **fixtures 121 +0 and `0120` STAYS UNCONSUMED** (now
DECIDED, per §8) · slice-only-writer gate **6** — ⚠️ **the row MODIFIES writer #1
(`h2/stream.go:345 out.Headers = append(...)`) rather than adding a seventh; the gate's expected reading
at row-done is STILL 6, and the IMPL must state that.** ADR count **310 -> 311 headings**, tail
**ADR-0312**, `^---$` stays **216** (⚠️ **a new ADR takes NO `---` separator**).

---

## 13. What the PLAN owes

1. **The TDD task decomposition**, with §9.2's RED census observed FIRST and its denominators asserted.
2. **The unit roster over BOTH symbols** — §9.3 proves `buildRequest` is unpinned in both directions, so a
   `buildH2Request`-only table would ship a vacuous guard.
3. **The `0004` arm roster**, raw-framer only (§6), with a **break protocol per arm and the injection site
   named** — and the `AssertDistribution [3,3,3]` constraint respected (§8.2).
4. **The `r.Host` backend edit** and an explicit statement of the ONE axis it cannot recover (§8.4).
5. **The differential assertion target: the `host`-presence and `r.Host` axes** — ⚠️ **not a route
   assertion**, which §4 proves cannot discriminate; the access-log / Zipkin-span axis (§4.1) is the
   second candidate and the PLAN must choose deliberately.
6. **The ROADMAP row-90 text correction** (§4) scheduled into the IMPL commit that flips the row `done`,
   plus the `BEHAVIOR_CONTRACT.md:2034` *"forwarded verbatim"* sentence, which promotion makes untrue.
7. **Cost re-measured at the PLAN's OWN publishing commit** — `reference_cost_figure_measured_at_publishing_commit`
   has fired in three consecutive rows.
8. **The deferred follow-ons written as rows, not gestures:** arm C (§5.5), H1-B′ (§7), H1-D as a NAMED
   DEPARTURE (§7.1), the D-89-HOST residue (§10), and the H3 arm-A prediction (§10.2).

---

## 14. ADR-0312 §Context — DRAFTED HERE; the strict `PROPOSED` guard is RE-ARMED 0 -> 1

Drafted into `DECISIONS.md` as `## ADR-0312`, with `> **STATUS: PROPOSED` and **no `---` separator**.
§Decision and §Consequences are appended IN PLACE at the IMPL, after the retained italic footer, per the
ADR-0294-0311 shared block form.

---

## 15. Explicitly NOT MEASURED — stated so it is never inferred

- **The H3 leg** — READ ONLY, zero probes (§10.2). The arm-A reproduction there is a **prediction**.
- **The encode/response direction** and upstream-side authority handling.
- **Downstream TLS/ALPN H/2** — every H/2 arm at this stage used **h2c prior knowledge** on both sides.
  Whether the reference's authority validation differs under a TLS listener is **untested**.
- **The exact charset the reference accepts in an authority.** §5.1 partitions nine arms; it does not
  characterise the predicate, and the deferred arm-C row must.
- **Any differential-fixture run** — no fixture was built or executed at this stage.
- **`-race`** on any run; **h2spec** — and note it is *structurally* incapable of anchoring this row (its
  harness configures `envoy.filters.http.router` ALONE with `direct_response` routes and never goes
  upstream).
- **Whether a conformant H/2 peer accepts the authority-less request** the reference forwards after a
  filter `remove("host")` (§10 R7/R11).
- **Whether the reference emits a GOAWAY before closing on arm C** — the reader saw a plain EOF and no
  `GoAwayFrame`; no packet capture was taken to prove nothing was written.

---

## 16. Documentary defects — recorded, deliberately NOT fixed here

⚠️ **`ROADMAP.md` row 90's own *"ROUTE-MATCHING input"* sentence** (§4) — the IMPL fixes it ·
⚠️ **`0004-h2-routing/envoy-go.yaml:3` says *"This file is documentation only"* while `driver/driver.go:214`
does `readYAML("envoy-go.yaml")`** — the file IS the live subject bootstrap template ·
⚠️ **the subject injects `User-Agent: Go-http-client/1.1` on every H1 upstream request where the reference
injects none** — observed in every backend head capture at this stage, uninvestigated and unrecorded
until now · ADR-0310 §Consequences (x)'s C3 *"deferred parity"* framing (REFUTED) · ADR-0310
§Consequences (xi)'s *"~64 KiB band"* (NOT REPRODUCIBLE) · `DECISIONS.md`'s `INNER_EXIT=0` at phase 87, a
value nothing in the tree emits · the phantom `h2.parseHeadersForRequest` cited at `h2dispatch.go:463`,
`:480`, `:492` against a repo-wide count of **0** (positive control `buildH2Request` ⇒ 1) ·
`internal/filter/http/types.go`'s false *"per ADR-0071"* comment · the H1⇒H2 502 cite naming the wrong
site and its unrecorded H3 leg · `ROADMAP.md` rows 57/69 malformed · `ROADMAP.md`'s five `esalaine` cites ·
`STATE.md` §Project counts frozen at phase 76 · ⚠️ **window `:222` carries TWO CLOSED candidates** —
re-verified at this tip: `url.ParseRequestURI` is present in `h2/stream.go:498` (row 87 closed the `//`
bug) and rows 79/80 both read `done`.
