# BRAINSTORM 84 — grpc-unary-response-trailers

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Date:** 2026-08-06.
**Base master:** `6c2d47756f5049bbbbfa467b3053509523e1b742` (from `git rev-parse master`), branch `phase-84-brainstorm`.
**Method:** SELF-PICKED per the 2026-07-12 standing directive; no banked mid-lifecycle work existed at this tip (phase 83 CLOSED, row 83 `done`). **FIVE investigation agents** on disjoint remits, each in its own DETACHED worktree with private scratch and a private port band inside `44300-44799`. Every load-bearing claim was controller-re-derived, and **two agent claims did not survive that re-derivation** (§5.2).

---

## 1. THE HEADLINE

### 1.1 ⚠️ THIS ROW OPENS THE ONLY NEVER-OPENED FAMILY — THE FIRST SENTINEL CHECK TO MOVE IN FORTY PHASES — AND IT RAISES THE OTHER CHECK WHILE DOING IT

`gRPC` is the sole slug check (3) reports as `NEVER OPENED`, and it has been alone there since row 82 retired `WASM`. Check (2) has **never gone down** across ~39 consecutive phases (history `0 -> 1 -> 3 -> 4 -> 5`).

Registering row 84 with a summary naming `gRPC-family row` **silences check (3) entirely**. Measured on a scratch copy, not forecast: the row alone takes the data-row denominator `115 -> 116` and leaves check (2) at **5**; the `**FAMILY OPEN at phase 84**` paragraph is what moves check (2).

⚠️ **AND THE HONEST FORM OF THAT PARAGRAPH RAISES CHECK (2) FROM 5 TO 6.** This is not a defect and not avoidable without gaming the gate. Measured precedent, re-derived here rather than assumed — the phase-77 BRAINSTORM opened the Runtime family in **one** commit:

| commit | check (2) union | `Runtime-family row` hits |
|---|---|---|
| `638ef32a^` | **4** | **0** |
| `638ef32a` | **5** | **2** |

Every one of check (2)'s five current anchors (`:193 :203 :213 :219 :227`) sits inside a `**FAMILY OPEN at phase NN**` paragraph. A newly-opened family that genuinely carries un-chartered remainder — and this one carries a large one (§3.2) — records it in the same phrase the sentinel matches. **The alternative is to phrase the deferrals so the matcher misses them, which is the `reference_sentinel_matcher_string_self_clears` defect pattern committed deliberately. This row declines to do that.**

⚠️ **SO THE ROW'S NET SENTINEL EFFECT IS: check (3) goes SILENT, check (2) goes 5 -> 6, and the sentinel still does NOT fire** (the condition is a conjunction). **STATED AS A MEASUREMENT AT §6, NOT FORECAST** (`reference_a_drift_correction_is_itself_a_claim`).

### 1.2 ⚠️ THE INHERITED PROTOTYPE COST IS REFUTED **DOWNWARD** — AN UNUSUAL DIRECTION FOR THIS LINEAGE

The phase-83 BRAINSTORM §5.4 recorded the seam prototype at **`+92/−11` across 5 production files**. Re-derived by execution at this tip with a working end-to-end fix:

| variant | measured numstat |
|---|---|
| **A — minimal seam fix** (deliver response trailers downstream) | **+60 / −14 across 4 files** |
| **B — A + wire the dead `RunEncodeTrailers` hook** | **+71 / −14 across 4 files** |

**The roster is FOUR files, not five.** The fifth in the inherited figure is `internal/filter/hcm/h2/stream.go` — a `WriteTrailers` method on the `h2.StreamWriter` interface. It is **not needed**: `serverStream.WriteHeaders(headers, endStream)` is already generic and `ServerConn.encodeAndWriteHeaders` emits an arbitrary HEADERS block, so a trailing HEADERS frame is `sw.WriteHeaders(tf, true)` with **no interface change**.

⚠️ **`reference_measured_prototype_is_a_lower_bound` HAS FIRED ON FOUR CONSECUTIVE ROWS. THIS IS THE FIRST INHERITED FIGURE IN THIS LINEAGE TO BE WRONG IN THE OTHER DIRECTION, AND THAT DOES NOT REPEAL THE MEMORY** — it means the phase-83 prototype was less minimal than achievable, not that estimates here run high. The **row total** is still governed by scaffolding, and §3.4 treats every figure in this document as a lower bound.

### 1.3 ⚠️ A LIVE PRODUCTION PROTOCOL DEFECT, NAMED BY NO DOCUMENT, SITTING UNDER A CONFORMANCE SECTION ALREADY REPORTED GREEN

Found in passing while probing for a third blocker. `internal/filter/hcm/h2/conn.go:255-259` discards `*http2.ContinuationFrame`:

```go
case *http2.ContinuationFrame:
    // Handled by framer as part of ReadMetaHeaders; reaching here means the
    // framer gave us a raw ContinuationFrame (shouldn't happen in normal usage,
    // but be safe and ignore).
    return nil
```

⚠️ **BOTH CLAUSES OF THAT COMMENT ARE FALSE.** `ReadMetaHeaders` is assigned in **exactly one place in the whole tree** — `test/differential/runner_test.go:3349`, a **test** helper — and **nowhere in `internal/`**. The production framer therefore never has it set, CONTINUATION frames **do** reach that case, and they **are** dropped. `onHeaders` decodes only the first fragment and dispatches the stream on an incomplete header block.

Measured against legal RFC 9113 §6.10 splits of a 116-byte HPACK block (control = the gRPC backend, reference = pinned Envoy — **both 200 at every split point**):

| split offset | subject | control / reference |
|---|---|---|
| 20 | **RST_STREAM(PROTOCOL_ERROR)** | 200 |
| 40 | **`:status 415`** (`content-type` was in the dropped fragment) | 200 |
| 60 / 70 / 80 / 90 / 100 / 110 | 200, continuation fields **silently dropped** | 200 |

Reachable from a **stock grpc-go client with no raw framing**: with metadata above ~27000 Huffman-compressible bytes the encoded block crosses the subject's own advertised `MAX_FRAME_SIZE=16384`; the bisect landed between 26000 (forwarded) and 27000 (dropped), and **the RPC still reports success while the header vanishes upstream**.

⚠️ **AND `http2/6/10` — "Frame Definitions: CONTINUATION" — IS INSIDE THE h2spec SECTION LIST** at `test/conformance/h2spec/h2spec.go:34`, the gate that reported **53/53** green at the phase-83 IMPL. **Why h2spec passes over a defect in its own declared section is UNMEASURED and is this row's sharpest open question (§4 Q1).** This is `reference_code_comment_not_evidence` firing again: the comment asserted a framer behavior that the call graph refutes.

**Disposition: RECORDED AND DEFERRED, NOT CHARTERED HERE** (§3.2). It is not on the unary critical path for small metadata, and folding an h2spec-adjacent framing fix into a family-opening row would cross §6.1 on a second axis.

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

All costs below were **re-derived at this tip by execution** (`reference_deferred_candidate_cost_restale`); none is inherited from the router's prose.

**PICKED — `grpc-unary-response-trailers`.** It is the only candidate in the project that can move a sentinel check; its inherited HARD-BLOCKED verdict is dead and was re-killed here by execution; its failing-first anchor was **observed RED on the harness-legal shape, 3/3 deterministic**; it needs **no new BackendKind, no new module, no new guest blob and no toolchain**; and the two blockers that define the family's ceiling are **provably outside** an H2-downstream unary carve (§3.3).

### 2.1 ⚠️ THIS IS NOT THE SMALLEST CANDIDATE, AND THE DIRECTIVE SAYS "SMALLEST **DEFENSIBLE**"

The cheapest candidate on the board is a **1-task, 76-line** structural gate (row 1 below). This row is larger. The argument for picking it anyway, stated so a later stage can attack it:

1. **It is the only candidate that can move a sentinel check.** Every alternative below moves **none** — stated, not forecast.
2. **`gRPC` is the last `NEVER OPENED` slug**, and this would be the **first sentinel movement in ~40 phases**.
3. **Phase 83 deferred it on cost alone and recorded regret** — *"the strongest candidate in the project and the one this row most regrets deferring."* Deferring it a **third** consecutive row, after its blocking verdict has now been refuted twice by execution, is not defensible.
4. **It IS carved to the smallest defensible leg** of the family (§3.1/§3.2): unary only, H2 only, four production files, **~46 net production lines measured**, both ceiling blockers proven outside, and the tempting `RunEncodeTrailers` wiring deliberately excluded on a measurement.
5. **Nothing below is lost.** All nine alternatives are recorded with measured cost and remain available; three of them are now *cheaper* than recorded because this stage refuted their blockers.

### 2.2 The rejected alternatives, with re-derived cost

| rejected alternative | re-derived cost | why rejected |
|---|---|---|
| **Add/Store structural gate** (`go/ast` over `pause.go`) | **76 test lines MEASURED**, 3 ms, NC fires on both sides; 1 task | Smallest on the board and a genuine gate for one of phase 83's two ungated statements. **Too small to charter as a row**; should be swept into the next WASM-touching row. Moves no sentinel check. |
| **`logf` swap gate** | prototype **73 lines MEASURED** covering 2 of **22** arms; lower bound **~220**, budget **250-450 test / 0 production**; 2-4 tasks | Closes phase 83's *other* ungated statement. Verified in full: `var logf` exists at `decode_headers.go:105`, **22 call sites across 5 non-test files**, and **zero** log-capture idioms in **22** test files. Real, cheap — but moves no sentinel check and adds no behavior. |
| **`ContinueStream` cannot report a lost resume** | ~6 prod + ~40 test (UNMEASURED); 1-2 tasks | Real: `abi_callbacks.go:916-939` **discards the bool** from `resumeDecode()`/`resumeEncode()` and always returns `WasmResultOk`. Blocked on an open ABI question — proxy-wasm has no result code for *"nothing to resume"*, so any non-`Ok` return is a **wire-visible departure**. Needs its own row, not a fold-in. |
| **`chain.go` encode-cursor reset** | <20 lines (UNMEASURED); <1 task | Not production-reachable (`beginLocalReply` is `localReplyOnce`/`encodeStarted`-guarded). ⚠️ **The recorded cite `chain.go:490-491` is STALE** — the three real sites are **`:524` / `:588` / `:668`**. Comment-or-guard, not a row. |
| **`validate` nil-`sdsProvider` bug** | ~30-40 prod + 60-120 test; **3-5 tasks**; `go list -deps ./validate` already carries `internal/xds` ⇒ **zero new package edges** | ⚠️ **REPRODUCED INDEPENDENTLY, with the discriminator run**: `--mode validate` rejects (`exit=1`) a config whose **boot path builds a live provider and dials** (arm 4 never reaches the reject), while a plain static-TLS positive control returns `configuration OK` (`exit=0`) so the reject is not vacuous. Cause: `validate/validate.go:48-49` passes literal `nil`; `internal/tls/config.go:436` rejects on `provider == nil`. **The strongest sub-row-sized candidate — still true — but it moves no sentinel check.** |
| **Pause watchdog through the `clock.Clock` seam** | ~30-50 prod + 80-150 test (UNMEASURED); 3-5 tasks | ⚠️ **ITS BLOCKING PREMISE IS REFUTED.** The record says `clock.Clock` exposes `Now()`/`After(d)` but **no `AfterFunc`** — false: `AfterFunc(d, fn) Stop` is declared at **`clock.go:78`** and implemented by **both** clocks (`RealClock:109`, `FakeClock:249`), and the wasm filter package already imports `internal/clock`. **It is UNBLOCKED, not blocked.** Residual real cost: the seam is at `*RootVM` scope, so a clock must be threaded to the per-stream filter. |
| **xDS config-seam strict-reject sweep** | **14 of 29** silently-ignored fields RE-DERIVED by proto reflection; ~40-60 prod + 150-250 test **UNMEASURED**; 5-8 tasks | The **14** is confirmed exactly, with a firing NC on six getters that *are* read. ⚠️ **Refinement the record lacks:** `ConfigSource.path`/`.path_config_source` are **not** among the 14 — they are oneof arms already caught by an existing reject. Lands *rejects*, not behavior; moves no sentinel check. |
| **The trailers two-seam WASM row** | ~25-40 prod + 150-250 test for a **unit-scope-only** row | ⚠️ **THE HANDOFF'S "PRICE BOTH SEAMS" IS ITSELF INCOMPLETE — THERE ARE THREE.** Both named seams verified (`headerMapForType`'s `default:` arm covers map types 1/3/4/5 and blinds **7** hostcall consumers; `trailers.go:109`/`:229` are `_ = trailers` and the documented fields do not exist). But **`RunDecodeTrailers`/`RunEncodeTrailers` have 28 non-test hits and ZERO invocation sites** (NC: `RunDecodeData` = 38), so fixing both named seams **yields nothing observable**. An end-to-end gate needs a third, larger seam **and a new Rust guest blob** — of **35** crates, **0** declare either trailers callback. |
| **`ssl.connection_error`** | floor **+444 net `.go` VERIFIED** (phase-75 span, 6 files, +488/−44); 11-13 tasks | ⚠️ **A category error found in the record:** phase 75's true **production** `.go` was **+30** (`manager.go` +30/−4, `name.go` +8/−4); the other +414 is tests plus a **+231-line fixture driver**. So the recorded *"~230 lower bound"* compared a **production-only** estimate against a **whole-`.go`** measurement. **+444 remains the right floor for a row's `.go` footprint.** Largest on the board. |

⚠️ **NONE of the nine moves any sentinel check. STATED, NOT FORECAST** (`reference_a_drift_correction_is_itself_a_claim`).

---

## 3. SCOPE

### 3.1 IN

**Deliver HTTP/2 response trailers downstream on the H2 path, so a successful unary gRPC RPC completes.** Four production files, measured (§1.2 variant A):

| file | role |
|---|---|
| `internal/filter/hcm/h2/client.go` | capture the upstream trailing HEADERS block (today: `:440` `// else: trailing HEADERS — observed-and-discarded per ADR-0058.`); add `H2Response.Trailers` |
| `internal/filter/http/router/router.go` | `ActionResponse.Trailers` carrier |
| `internal/filter/http/router/router_h2.go` | populate it in `doH2ClusterAction` |
| `internal/filter/hcm/h2dispatch.go` | `writeH2Reply` — hold END_STREAM off HEADERS/DATA, emit a trailing HEADERS(END_STREAM) |

Plus **one differential fixture** (`0119-…`, reference port **10119**) asserting the RPC's own status cross-side.

### 3.2 OUT — each exclusion with its measured basis

| excluded | measured basis |
|---|---|
| **Wiring the dead `RunEncodeTrailers` hook** | §5.1 — it converts phase-83's LATENT trailers arms into LIVE production paths, waking a guest about trailers it **cannot read** |
| **All gRPC streaming** | Response buffering is **structural** (`[]byte` at three layers), and unary is provably unaffected (§3.3) |
| **`grpc_http1_bridge`, browser-path `grpc_web`** | H1-downstream -> H2-upstream returns a **measured 502** (§3.3) |
| **All eight gRPC filter type URLs** | **0 of 8** appear in any `.go` file; none has a parse-reject arm; the failure is a **protojson type-resolution** failure upstream of any filter registry — **no seam is pre-paid** |
| **`test/conformance/grpc/` (interop conformance)** | Declared at `BOOTSTRAP_PROMPT.md:350`, **does not exist**; `test/conformance/` holds only `doc.go`, `h2spec`, `proxy-wasm`. §4 Q2 |
| **The CONTINUATION defect (§1.3)** | Live and reference-diverging, but off the unary critical path and h2spec-adjacent |
| **H3 gRPC** | Structurally unreachable: `SetH2Action` has **one** non-test call site (`h2dispatch.go:402`), so an H3 listener cannot reach an H2 upstream at all |
| **The four cosmetic header divergences** | `x-envoy-upstream-service-time` / `x-request-id` / `x-forwarded-proto` / `x-envoy-expected-rq-timeout-ms`, plus `server`/`date` ordering — cross-side visible, must be **explicitly un-asserted** in the fixture |

### 3.3 ⚠️ THE TWO CEILING BLOCKERS ARE PROVABLY OUTSIDE THIS CARVE — MEASURED, WITH THE DISCRIMINATOR RUN

**Blocker 1 (H1 -> H2 = 502): CONFIRMED, and it does NOT bind H2 -> H2.**

| arm | subject | reference |
|---|---|---|
| H1 listener -> H2 cluster, gRPC POST | **502**, `content-length: 0` | **200**, `application/grpc` |
| H1 listener -> H2 cluster, plain GET | **502** | **415** (the backend's own gRPC reply) |
| **H2 listener -> H2 cluster (the discriminator)** | **200 + correct 7-byte frame `00000000020801` in 0.002 s** | — |

Root cause proven by byte capture, not inference: `connection.go:467` calls `rf.SetAction(action)` unconditionally — the **H1** closure — without consulting `Cluster.UseH2()`, and the wire carries `POST … HTTP/1.1` with no HTTP/2 preface. `config.go:687` already concedes it in a landed comment. **`UseH2` is silently ignored, not mis-dialled.**

**Blocker 2 (full response buffering): CONFIRMED structural — and UNARY IS UNAFFECTED, including at interop size.**

| probe | control | subject | reference |
|---|---|---|---|
| `Health/Watch` first `Recv` | 0.001 s / nil | **5.004 s / DeadlineExceeded** | 0.001 s / nil |
| 3-message server-stream, 400 ms apart | 0.001 / 0.401 / 0.802 s | **1.204 / 1.204 / 1.204 s** (all after END_STREAM) | 0.002 / 0.403 / 0.804 s |
| **interop `large_unary`** (271828 req / 314159 resp) | — | **314159 bytes, sha256 prefix `ace5f0f02a864d4e`, 0.004 s** | identical prefix, 0.006 s |

⚠️ **The `large_unary` arm is the load-bearing one:** it is byte-identical to control and reference, so the row **need not touch** the `[]byte` carriers at `h2/client.go:44,53`, `router.go:82` or `h2dispatch.go:575-602`. The only error on every unary arm is the trailers seam.

### 3.4 ⚠️ COST POSTURE — AND WHY NO RATIO IS CARRIED

Measured inputs: production patch **~46 net** (variant A, `+60/−14`); fixture leg **~500-750** from the named analogue `0079-h2-multiplex-pool` (923 LoC actual); test:production scaffolding ratio, measured over the last three landed rows, **4.27 / 2.66 / 3.83 — median 3.83**; comment fraction of landed `.go` **23.5-46.2%, median ~30%**, mechanically enforced by `revive` `package-comments` + `exported` under `run.tests: true`.

Naive composition lands **~950-1200 net `.go`**, inside `:290`'s ~1500. ⚠️ **DO NOT TREAT THAT AS THE ANSWER.** Three of the last four rows crossed `:290` (1636 / 2229 / 2675 / 3536), the last against a band its own PLAN had **measured**. **The cause is under-ENUMERATION, not under-SCALING.** The SPEC's job is to **enumerate**, and §4 Q2 names the item most likely to blow this row's budget.

---

## 4. OPEN QUESTIONS FOR THE SPEC

1. ⚠️ **Why does h2spec report `http2/6/10` green over the CONTINUATION defect of §1.3?** Either the section's cases do not exercise a split header block, or the gate is not running what it reports. **UNMEASURED here.** This decides whether the h2spec 53/53 figure is trustworthy for any future row.
2. ⚠️ **Does opening the gRPC family trigger §7.5 gate (c)?** `BOOTSTRAP_PROMPT.md:350` declares `test/conformance/grpc/`; it does not exist. Comparables bracket it 3x: `h2spec` = **388** Go lines, `proxy-wasm` = **1131**. The row must either build one or ship an explicit deferral — **this is the single largest unpriced item and the strongest candidate to be this row's under-enumerated line.**
3. **Does the fixture assert trailers cross-side byte-exactly, or canonicalized?** A2 established the mechanism (Drive hooks -> `CompareBytes`); the four cosmetic header divergences (§3.2) mean a naive diff reds on a correct tree.
4. **`ADR-0058` is the recorded basis for discarding trailing HEADERS.** The SPEC must state whether this row **supersedes** it or narrows it, and draft `ADR-0306` §Context accordingly.
5. **Does the row need TLS PKI?** The harness never passes `--allow-h2c` (§5.3), so the fixture is TLS + ALPN-h2. `0079` ships 3 PEM files and **no generator**; `0004` ships a **173-line** `pki/gen/main.go`. Which pattern applies is unpriced.

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE FOUND BY EXECUTION

### 5.1 Load-bearing

1. **The seam is LIVE at this tip**, verbatim: subject `Internal: server closed the stream without sending trailers`; reference and direct-control both `SERVING`. Reproduced on **both** plaintext h2c and the harness-legal TLS+ALPN shape, **3/3 deterministic**.
2. **The inherited prototype cost is refuted downward** — `+60/−14` across **4** files, not `+92/−11` across 5 (§1.2).
3. **A live CONTINUATION-discard defect under a false comment, inside a green conformance section** (§1.3).
4. ⚠️ **THE WASM SEAMS ARE DECOUPLED FOR VARIANT A AND COUPLED THE MOMENT `RunEncodeTrailers` IS WIRED.** Measured with the same instrumented binary, same guest, only `h2dispatch.go` differing: variant B printed the full chain walk into `wasm filter.EncodeTrailers` with `HasGlobalFunc(proxy_on_response_trailers)=true`; **variant A printed 0 lines** as the negative control. Both returned `SERVING`. **This is the scope-carve decision, and it was made on a measurement rather than on topology.**
5. ⚠️ **EVERY SUBJECT STAT IS GREEN WHILE THE RPC FAILS** — `cluster.c_grpc.upstream_rq_2xx: 2`, `http.ingress_grpc.downstream_rq_2xx: 2`, scraped after **two failed RPCs**. **A `StatsAsserter`-only fixture — the dominant idiom in this repo — would be VACUOUSLY GREEN on exactly this defect.** See §8 broken-gate shape 31.
6. **`RunEncodeTrailers` has zero callers, and the dead subtree is deeper than recorded.** Denominators: **34** occurrence lines tree-wide, **11** non-test = 10 comments + 1 definition + **0 callers**. `f.EncodeTrailers(` has exactly **one** non-test occurrence — `chain.go:675`, *inside* `RunEncodeTrailers` — so **every filter's `EncodeTrailers` implementation in this repo is production-dead**.
7. **Blocker 1 does not bind H2 -> H2; blocker 2 does not bind unary** (§3.3), each with its discriminator executed.
8. **Request trailers are not on the gRPC path** — confirmed with a **firing** negative control: the only `REQUEST-TRAILERS` line in 3 requests came from a purpose-built `x/net/http2` client declaring `req.Trailer`. Neither gRPC RPC produced one. **The probe is proven able to fire, so the gRPC silence is a result and not blindness.**
9. **gRPC error RPCs already pass unpatched, and the mechanism is more specific than recorded:** the error arm is a **Trailers-Only** response — a *single* HEADERS block with END_STREAM carrying `grpc-status: 5` — which envoy-go forwards verbatim. The defect is bounded to **successful** RPCs. Counts: `upstream RESPONSE-TRAILERS = 1` of 3 streams.
10. **The eight gRPC filter type URLs are unregistered and un-rejected**, boot-probed **8/8** with a discriminating positive control (`…http.tap.v3.Tap` in the same slot resolves and proceeds to a later error).

### 5.2 ⚠️ TWO AGENT CLAIMS THE CONTROLLER DID NOT ACCEPT

- ⚠️ **"The second `BOOTSTRAP_PROMPT.md` copy does not exist and the two-copy hazard is not live" — REFUTED.** The agent searched by exact filename. The copy is live: `docs/superpowers/plans/2026-04-21-envoy-go-bootstrap-prompt.md`, **1024 lines**, and the offsets are **NOT a constant shift** (§6.1 Δ**+197**, §7.5 Δ**+228**). `reference_bootstrap_prompt_two_copies_wrong_anchors` stands.
- ⚠️ **"`STATE.md:33`'s stat surface is 1205" — the agent was RIGHT that it is stale, but the live absolute needed its own derivation.** `BEHAVIOR_CONTRACT.md` states **1207** (`1207` x6 / `1205` x2): phase 77 landed a `runtime.*` pair. **`STATE.md:33` has been stale by 2 since the phase-76 close, and the router inherits it.** ⚠️ **The absolute rides two unaudited ledger gaps and remains DOC-SOURCED — only the DELTA is ever asserted.**

### 5.3 Also refuted (this stage's own corrections)

- ⚠️ **`STATE_HISTORY.md` is 454 lines and `STATE.md` is 64 — NOT the 455 / 65 the router and `STATE.md` §Current both carry.** The real phase-83 transition was **452 -> 454**. The **+2 delta and the append-only property are correct** (re-verified: `--numstat` = `2 0`, and the base blob is a byte-exact **prefix** of the new one); **both absolute endpoints are wrong, propagated across two documents.**
- ⚠️ **The `-family row` figure is REGEX-DEPENDENT and the router's `65` is a LINE count.** Occurrences = **93**; lines = **65**. `grep -c` counts LINES, not occurrences. **State which form.** Per-slug: Observability **53**, Network-filters 9, Load-balancing 8, xDS 8, Upstream-robustness 5, Operational-tooling 3, HTTP/3 2, Runtime 2, WASM 2, HTTP-filters 1, **gRPC 0**.
- ⚠️ **`expectations.yaml` IS NOT A REGISTRATION GATE.** It is present in **96 of 120** fixtures and read by **zero** Go code (`test/differential/**.go` refs = 0, NC `discoverFixtures` = 3 hits in the same tree). It is a documentation convention.
- ⚠️ **The differential harness never passes `--allow-h2c`** (0 hits in `test/differential/`, while the flag exists at `cmd/envoy-go/main.go:40`), so a plaintext-H2 fixture **boot-rejects**. The fixture must be TLS + ALPN-h2 — and the RED anchor was produced in exactly that shape.
- ⚠️ **`HTTPExpectations` is unusable for this row** — its only dispatch site calls `helpers.HTTPRoundTrip`: plaintext HTTP/1.1, no TLS, no trailers, no gRPC framing.
- ⚠️ **BackendKind reuse is normal, and "one dir = one runner branch" does NOT bind the backend switch.** `HTTPFixedBody` is returned by **26** distinct fixture dirs, `HTTPEcho` by 16. **`GRPCHealthResponder = 34` is already a real grpc-go server** (`runner_test.go:3106`) used by exactly one fixture (0068) — **no new BackendKind.** Tail value **38** over **39** declarations, NC fires 39 -> 40.
- **The §9 charter (`BOOTSTRAP_PROMPT.md:402`) names gRPC bridge, gRPC-Web, gRPC-JSON transcoding and interop conformance — and this row is NONE of the four.** It is their prerequisite. The row is justified as a **family opener**, and says so rather than claiming a charter bullet.
- ⚠️ **`clock.Clock` DOES declare `AfterFunc` — the deferred candidate's blocking premise is FALSE.** `AfterFunc(d time.Duration, fn func()) Stop` at **`internal/clock/clock.go:78`**, implemented by `RealClock` (`:109`) and `FakeClock` (`:249`). The candidate is **unblocked**, and the record has carried the opposite for at least two phases.
- ⚠️ **`RunDecodeTrailers`/`RunEncodeTrailers` have ZERO invocation sites — 28 non-test hits, none of them a call** (NC: `RunDecodeData` = **38**). Independently reached by two agents on unrelated remits. ⚠️ **This makes the phase-83 handoff's *"any future row must price BOTH seams"* INCOMPLETE: there are THREE.**
- ⚠️ **`pause.go:159`'s *"no window exists"* is FALSE.** Measured over 60 000 trials: correct order **0-1** steals, inverted order **9-19**. The `Add`-before-`Store` ordering **narrows the window ~12x; it does not close it.** ⚠️ **And the probe that found this was CONFOUNDED on its first run** — gen-2's own 40 µs watchdog produced the same signature until gen-2 was pinned to a 10 s window (`reference_timing_gate_needs_pinned_pacer`).
- ⚠️ **`chain.go:490-491` is a STALE CITE.** The three encode-cursor resets are at **`:524` / `:588` / `:668`**; `:490-491` is inside `RunDecodeData`'s `localReplyDone` check.
- ⚠️ **Phase 75's *"~230 lower bound"* was a CATEGORY ERROR, not a magnitude error** — production-only was **+30**; the verified **+444** is the whole-`.go` figure. **Fix the category before the next estimate inherits it.**
- **My own BRAINSTORM brief mis-stated the package path** as `internal/hcm/`; it is `internal/filter/hcm/`. It **also** sent an agent to `internal/wasm/abi_callbacks.go`, which does not exist — the file is `internal/filter/http/wasm/abi_callbacks.go`, and **the router's prose carries the same wrong path**. Caught by two agents independently. **A controller brief is a claim too.**

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE. IT DOES **NOT** FIRE

Input measured **231 lines / 115 data rows** BEFORE anything was written, so an empty result could not read as a zero result.

| check | BEFORE edits | AFTER edits |
|---|---|---|
| **(1)** | **SILENT** at `want=115` | **`NOT DONE: row 84`** at `want=116`, `examined 116 data rows` — **correct while the phase is open** |
| **(2)** | **FIVE** — `:193 :203 :213 :219 :227` | **SIX** — `:194 :200 :206 :216 :222 :230` |
| **(3)** | `NEVER OPENED: gRPC` — alone | ⚠️ **SILENT** |

⇒ the condition is a **CONJUNCTION**, checks (1) and (2) still print, so **the sentinel does NOT fire.** `stop` was **NOT** created (`ls stop` => `No such file or directory`) and must not be.

**FIVE negative controls before, ALL FIRED** — row 62 doctored => `NOT DONE: row 62` (with `NC LANDED? [ in-progress ]` **inspected before the result was trusted**); `want=114` => `GATE FAIL: examined 115 data rows, expected 114`; an invented slug fires while `WASM`/`HTTP-filters` correctly do not; check (2) **one-arm** strip moves **5 -> 4, NOT 5 -> 0**; both arms stripped => 0. ⚠️ **A SIXTH, AFTER:** doctoring the `gRPC-family row` mention restores `NEVER OPENED: gRPC` — **that is what proves check (3)'s new silence is a result and not a broken check.**

**Leak check as a whole-file before/after count, NOT a grep of the diff's `+` lines:** check-(2) union **5 -> 6** (deliberate, §1.1), `-family row` **93 -> 95** (both from `gRPC` **0 -> 2**; WASM and Observability **invariant**), lines **231 -> 234**, data rows **115 -> 116**. Row well-formedness: ARM-A flags **only** the pre-existing lines 119 and 131 — **row 84 does not appear.**

⚠️ **ONE LEAK AXIS MIS-RAN, AND IT IS RECORDED RATHER THAN HIDDEN.** `grep -oiE '-family row'` parsed its own pattern as a **flag** (`grep: amily row: No such file or directory`) and printed `base=0 now=0 delta=0` — **which reads exactly like "no change".** Only `--` made it discriminate. **A gate that reads zero on BOTH sides is not evidence of invariance**, and this one was one careless reading away from certifying a leak it never measured.

---

## 7. COUNTS RE-DERIVED AT THIS TIP

Each with the command that produced it; **never copied forward.**

- fixtures **120** (`^[0-9]{4}[a-z]?-`; ⚠️ the naive `^[0-9]{4}-` gives **118**) · next-free dir `0119`, next-free reference port **10119** (0 hits; `10118` present in 3 files as the firing control)
- fuzzers **55** (scoped `-- '*.go'`) · phase dirs **124**, of which **37** carry `REVIEW.md` and **87** do not
- `DECISIONS.md` **17926** lines · **304** `^## ADR-` headings · tail **ADR-0305**, next-free **ADR-0306** (`^## ADR-0306` = 0, NC `^## ADR-0305` = 1) · `^---$` **216** · STATUS census **18** · strict PROPOSED guard **0**
- `ROADMAP.md` **231** lines / **115** data rows · **`STATE.md` 64** · **`STATE_HISTORY.md` 454** (⚠️ **both corrected — see §5.3**) · `BEHAVIOR_CONTRACT.md` **5900**
- **stat surface 1207** (⚠️ **not the 1205 `STATE.md:33` carries** — DOC-SOURCED, only the DELTA is ever asserted)
- `BackendKind` **39** declarations, tail value **38** · `BOOTSTRAP_PROMPT.md` **522** lines at the repo root

---

## 8. BROKEN-GATE LEDGER — ONE NEW SHAPE

**THE THIRTY-FIRST: A COUNTER-BASED FIXTURE ASSERTION IS VACUOUS AGAINST A TRAILER-CARRIED STATUS.** The subject books `upstream_rq_2xx: 2` and `downstream_rq_2xx: 2` while **both** RPCs fail, because the HTTP response *is* a 200 — the gRPC failure rides trailers the proxy never sends. **82 fixtures define an `AssertStats` method; a stats-only fixture here would report green on the exact defect the row exists to fix.** The gate must assert the RPC's own status through the Drive hooks and `CompareBytes`. This is `reference_counter_cannot_gate_a_value` in a new location: *a 2xx counter cannot gate a status carried outside the status line.*

**The thirty carried forward, unchanged** — see the phase-83 record.

---

## 9. HYGIENE

Five agents in DETACHED worktrees with private scratch and disjoint port bands inside `44300-44799`; every agent reverted its probes and confirmed `git status --porcelain` = **0 lines**; docker containers torn down **BY NAME** (`a2-ref-grpc`, `a2-ref-grpc-tls`, `a3-ref`), never by an `ancestor=`/image filter. `go.mod`/`go.sum` untouched (**grpc-go v1.70.0 was already a direct require** — no new module).

⚠️ **A stale-worktree observation for the next session:** the `phase-84-*` worktrees and the `phase-84-brainstorm` branch **predated this session by ~10 minutes** — an aborted prior attempt that produced **zero commits** and left every worktree clean at base. They were verified clean and reused rather than recreated. **Check `git worktree list` and `ps` before assuming a phase-N worktree is yours.**

---

## 10. NEXT

**SPEC.** It owes: the four §4 open questions, `ADR-0306` §Context drafted STATUS `PROPOSED`, the fixture's assertion shape, and — most importantly — **an ENUMERATION rather than a scaling** of this row's cost, with §4 Q2 (`test/conformance/grpc/`) priced or explicitly deferred in writing.
