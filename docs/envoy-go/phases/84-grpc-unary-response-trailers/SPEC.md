# SPEC 84 — grpc-unary-response-trailers

**Stage:** SPEC (lifecycle-state 1 -> 2). **Date:** 2026-08-06.
**Base master:** `e1c07208778afe88020b317d20891b146cdfb45b` (from `git rev-parse master`), branch `phase-84-spec`.
**Method:** FIVE investigation agents on disjoint remits, each in its own DETACHED worktree with private scratch and a private port band inside `44800-45299`. Every load-bearing claim controller-re-derived. **Three agent claims did not survive that re-derivation, and one of them was a refutation of a claim the BRAINSTORM never made** (§2.12).

⚠️ **ROW 84 STAYS `in-progress`. `ROADMAP.md` IS BYTE-UNTOUCHED AT THIS STAGE AND THE SENTINEL `want` STAYS 116.** This SPEC declares a **84.1 / 84.2 split** and adds **NO ROADMAP ROW** — the precedent for both is exact (§12.3).

---

## 0. SENTINEL — RE-RUN MECHANICALLY AT THIS TIP. IT DOES **NOT** FIRE; `stop` WAS **NOT** CREATED

Input measured **BEFORE** anything was written — `234` lines / `116` data rows — so an empty result could not read as a zero result (`reference_empty_output_is_not_a_zero_result`).

| check | result at this tip |
|---|---|
| **(1)** | **`NOT DONE: row 84`** at `want=116`, denominator printed (`examined 116 data rows`) — **CORRECT AND EXPECTED while the phase is open** |
| **(2)** | **SIX** — `:194 :200 :206 :216 :222 :230` |
| **(3)** | **SILENT** — `NEVER OPENED` prints nothing |

⇒ the condition is a **CONJUNCTION**; checks (1) and (2) print, so **the sentinel does NOT fire**. `ls stop` => `No such file or directory`. It must not be created while (1) or (2) prints.

**SIX NEGATIVE CONTROLS, ALL FIRED.** Row 62 doctored => `NOT DONE: row 62` **and** `NOT DONE: row 84`, with `NC LANDED? [ in-progress ]` **inspected before the result was trusted** · `want=115` => `GATE FAIL: examined 116 data rows, expected 115` · the mandatory check-(3) doctoring (`sed 's/gRPC-family row/gRPC-XXXXXX row/g'`, residual occurrences confirmed **0** first) => **`NEVER OPENED: gRPC`** restored, which is the only thing distinguishing check (3)'s new silence from a broken loop · an invented slug fires while `gRPC`/`WASM`/`HTTP-filters` correctly do not · check-(2) **one-arm** strip => **6 -> 5, NOT 6 -> 0** · both arms stripped => **0**.

### 0.1 The leak check is **VERIFIED BY WHOLE-FILE BEFORE/AFTER COUNT**, not by a diff grep

`ROADMAP.md` is byte-untouched at this stage, so every axis is trivially invariant — **but the baseline is recorded here so the PLAN and IMPL can diff against it**: check-(2) union **6** · `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · lines **234** · data rows **116**.

⚠️ **THE FLAG TRAP REPRODUCED LIVE AT THIS STAGE.** `command grep -oiE '-family row' …` fails with `grep: amily row: No such file or directory`, and the surrounding arithmetic reads `base=0 now=0 delta=0` — **which is indistinguishable from "no change"**. Only `--` before the pattern discriminates. **A gate that reads zero on BOTH sides is not evidence of invariance** (`reference_leading_hyphen_pattern_reads_as_flag`).

### 0.2 ⚠️ THE ARCHIVE-ABSENCE GUARD'S "TOLERANT" FORM IS FAIL-UNSAFE, RE-CONFIRMED ON REAL TARGETS

Cross-product run at this stage over `STATE_HISTORY.md` (**456** lines) on **four REAL targets** (`reference_probe_input_is_a_claim` — an invented target reads 0 on every arm and looks like agreement):

| target | raw `grep -cF` | OLD colon form | **ROBUST form** |
|---|---|---|---|
| `phase 82 (…) PLAN done` — an ANNOTATED-label entry | **1 — PRESENT** | **0 — FALSE ABSENT** | **1** ✅ |
| `phase 82 (…) IMPL done` — **this stage's eviction target** | 0 | 0 | **0** ✅ |
| `phase 83 (…) BRAINSTORM done` | 0 | 0 | **0** ✅ |
| an invented target | 0 | 0 ✅ | **0** ✅ |

**The eviction target is ABSENT on all three forms ⇒ the append is safe.** ⚠️ `grep -c` **exits 1 on zero matches**, so `|| echo ERR` corrupts the reading — capture the value, never chain on the exit code (`reference_grep_c_zero_is_a_broken_gate`).

---

## 1. SCOPE — AND THE THREE CENTRAL FINDINGS

### 1.1 ⚠️ FIRST: THE CONFORMANCE GATE THIS ROW WAS TOLD TO REASON ABOUT HAS **NEVER RUN THE SECTION IN QUESTION** — AND HAS NEVER RUN **ANY** OF RFC 9113 §6

The BRAINSTORM's sharpest open question (§4 Q1) asks why h2spec reports `http2/6/10` green over the CONTINUATION-discard defect. **The premise is false. `http2/6/10` selects zero test cases and always has.**

h2spec addresses sections with **dotted** numbers (`http2/6.10`). `test/conformance/h2spec/h2spec.go:22-36` declares **ten** `http2/6/N` strings. Each matches **nothing**, and h2spec treats an unmatched positional argument as a **silent no-op** — it prints the parent heading and runs no cases.

**The arithmetic proves it from inside the repository, without running anything.** The gate's famous total decomposes two independent ways and both exclude section 6:

- per selector: `http2/3`=2 + `http2/4`=9 + `http2/5`=22 + `http2/7`=2 + `http2/8`=18 = **53**
- `docs/envoy-go/CONFORMANCE_PINS.md:37-57`, the gate's own audit trail, tabulates the **18 suites actually observed** since 2026-04-25 — **`3.5, 4.1, 4.2, 4.3, 5.1, 5.1.1, 5.1.2, 5.3.1, 5.4.1, 5.5, 7, 8.1, 8.1.2, 8.1.2.1, 8.1.2.2, 8.1.2.3, 8.1.2.6, 8.2`** — with **zero 6.x rows** and `**Total** | **53**`.

⚠️ **`DECISIONS.md` ADR-0051 §2 asserts the opposite in the same breath**: *"The gate runs sections `http2/3`, `http2/4`, `http2/5`, `http2/6/1–6/5`, `http2/6/7–6/10`, `http2/7`, `http2/8` (53 test cases)."* **The authoritative pin document and the ADR have contradicted each other for ~80 phases, and neither was noticed because both end in "53".**

**Measured consequences, each with its control:**

- Running the **correct** selector `http2/6` against envoy-go: **`42 tests, 37 passed, 1 skipped, 4 failed`.** The four are envoy-go-specific — three `6.5.2` SETTINGS-validation cases (`ENABLE_PUSH` non-0/1, `MAX_FRAME_SIZE` below-initial and above-maximum: expected GOAWAY(PROTOCOL_ERROR), got SETTINGS ACK) and one `6.9.2` flow-control case (`INITIAL_WINDOW_SIZE` changed after HEADERS: expected `DATA len=1`, got `len=3`). The pinned reference on the same command fails a **disjoint** set of 3. **Section 6 is the only failing section in the whole suite.**
- Running the correct `http2/6.10`: **`6 tests, 6 passed`** — so hypothesis (a) holds *independently*. Its only reassembly case (`6.10.1`) puts every routing-relevant pseudo-header in the **first** fragment, pads the CONTINUATION with 3513 dummy bytes, and asserts only that *a* HEADERS response returns. **It passes over a live drop.** The other five cases are error-handling and are caught by `x/net/http2.Framer`'s own sequencing validation before envoy-go's code is reached.
- The gate **cannot see** a zero-case suite: `assertThreshold` (`h2spec_test.go:309-311`) does `if s.Tests == 0 { continue }`, and the JUnit XML contains `<testsuite name="6.10. CONTINUATION" id="6.10" tests="0" …/>`. **Nothing asserts that a declared section produced any cases.** That is the missing guard.
- ⚠️ **AND THE GATE IS NOT RUN IN CI AT ALL.** `command grep -rn -E 'conformance|h2spec|proxy-wasm' .github/` returns **0** (NC: `go test|golangci` returns **8**, so the grep landed). `TestH2Spec` skips under `-short` (`h2spec_test.go:32-34`) and CI runs `go test -short -race ./...`. **h2spec is controller-run, not CI-enforced.**

⚠️ **THE STANDING RULE THIS ESTABLISHES: NO ROW MAY CITE "h2spec 53/53" AS EVIDENCE ABOUT FRAME-LEVEL BEHAVIOUR.** 42 strict cases — **44% of the gate's declared scope**, covering DATA, HEADERS, PRIORITY, RST_STREAM, SETTINGS, PING, GOAWAY, WINDOW_UPDATE and CONTINUATION — have been silently unrun since 2026-04-25. The number is arithmetically honest; **the scope claim attached to it is not.**

**DISPOSITION: RECORDED, NOT FIXED HERE (§13).** The one-character-per-string repair is trivial, but it turns a green gate **red with four pre-existing failures** that are not this row's regressions, and disposing of them is a §6.1 axis of its own. **It is named as a deferral, not silently carried.**

### 1.2 ⚠️ SECOND: ADR-0058 ALREADY PRESCRIBES THIS ROW'S EXACT FIX — SO Q4 IS "NARROW", AND THE ROW IS A **FULFILMENT**, NOT A REVERSAL

`DECISIONS.md:1987-2033` (bounded by the next `^## ADR-` heading of **any** id, `:2034` = ADR-0057 — the headings are **not** in numeric order, and a naive `/ADR-0058/,/ADR-0059/` range **over-captures 96 lines** by swallowing ADR-0057 rather than running to EOF). Its §Consequences says, verbatim:

> future trailer-forwarding work will widen `H2Response` with a `Trailers []hpack.HeaderField` field and the router will emit a trailing HEADERS frame conditionally when the upstream provided one. The widening is forward-compatible — no field renames; only an additive field.

**That is variant A, line for line.** The row consumes a forward-pointer the ADR itself planted; §Consequences bullet 1 even names the consumer: *"the gRPC family land trailer forwarding (where `grpc-status` is carried in trailers and forwarding is the load-bearing benefit)."*

⚠️ **AND A BINDING CONSTRAINT NO DOCUMENT IN THIS LINEAGE HAS NAMED.** `DECISIONS.md` **ADR-0052 `:1821`**:

> Future phases that extend the H2 equivalence surface (e.g., trailers in phase 07, **gRPC in a gRPC-family phase**) add sub-sections here (or a subsection sibling) **via a new ADR, not by editing 05.1's `### Not asserted` block silently**.

It names *this row's exact situation*. **`ADR-0306` is therefore the MANDATED vehicle for the `BEHAVIOR_CONTRACT.md ## HTTP/2` edit, not merely the conventional one.**

### 1.3 ⚠️ THIRD: THE §6.1 LoC TRIGGER **CROSSES AT THE FLOOR**, BEFORE THE PLAN IS WRITTEN — AND THIS SPEC SPLITS THE ROW RATHER THAN RECORDING A CROSSING AFTER THE FACT

The BRAINSTORM's naive composition lands **~950-1200 net `.go`**, inside `:290`'s ~1500. **It is below the empirical floor.** Measured (§12): the floor is **≈1738** and the central estimate **≈2290**, computed with the *smallest* unit-test bucket and the *smallest* fixture any comparable row produced.

Three measured reasons the naive figure is low, all category errors rather than magnitude errors:

1. **The 4.27 / 2.66 / 3.83 ratio's denominator counts fixture drivers as "production"** — reproduced exactly as `added TEST .go ÷ added NON-test .go`. §3.4 then applies it to production **only** (~46) and adds the fixture leg **separately**, double-discounting. ⚠️ **This is the very species §5.3 flags for phase 75** (*"production-only was +30; the verified +444 is the whole-`.go` figure. Fix the category before the next estimate inherits it"*) — **it was inherited, one document later.**
2. **All three rows in the median are the wrong shape.** Phases 81 and 83 shipped **zero** fixture `.go`; 82 shipped 64. Phase 84 is fixture-bearing — the shape of **phase 80**, the one row the median **excludes**, whose ratio is **1.22**. A ratio swinging 1.22 ↔ 4.27 on row shape cannot carry a budget.
3. **The comment fraction is 39.2% and rising, not ~30%** — measured over the added `.go` of the four IMPL diffs: **34.8% / 38.0% / 40.4% / 46.6%**, monotonic across all four overruns. Budgeting at 30% understates the code-derived term by **1.31x** on its own.

⚠️ **`reference_measured_prototype_is_a_lower_bound` HAS FIRED ON FOUR CONSECUTIVE ROWS AND MEASURING DID NOT NEUTRALISE IT.** This SPEC's response is not a bigger number; it is **§12's enumeration** plus **the split**, because §6.1 gates *"`PLAN.md` **estimates**"* and phases 80 and 81 both escaped the trigger purely by writing down a figure that was too low.

### 1.4 What the row must therefore contain

**Deliver HTTP/2 response trailers downstream on the H2 path so a successful unary gRPC RPC completes**, split into two legs (§12.3):

- **84.1 — the seam.** Four production files, unit tests, `ADR-0306`, the `BEHAVIOR_CONTRACT` reconciliation. RED anchor is the grpc-go client probe, **outside** the differential harness.
- **84.2 — the differential.** Fixture `0119-grpc-unary-trailers`, reference port **10119**, reusing `GRPCHealthResponder` **BackendKind 34**. **The final leg; its IMPL flips row 84 `done`.**

---

## 2. WHAT THIS SPEC REFUTES BY EXECUTION

**Twenty-six refutations, eleven load-bearing.** The eleven are §1.1 (the h2spec selector defect, five independent measurements), §1.2 (ADR-0058 as charter; ADR-0052 as binding), §1.3 (the ratio category error, the wrong-shape median, the rising comment fraction), plus §2.1-§2.6 below. The remainder are §2.7-§2.13.

### 2.1 ⚠️ THE BRAINSTORM'S "gRPC ERROR RPCs ALREADY PASS UNPATCHED" IS **REFUTED BY EXECUTION**

§5.1 item 9 claims error RPCs already pass because the error arm is a **Trailers-Only** response. **Against `GRPCHealthResponder` that is false.** `serveGRPCHealth` (`runner_test.go:3106`) serves via `gs.ServeHTTP` behind `h2c.NewHandler` — grpc-go's **http-handler transport, which never emits Trailers-Only**. Measured on four sides:

| arm | control | reference | subject UNPATCHED | subject PATCHED |
|---|---|---|---|---|
| `Check(service="nope")` → `grpc-status 5` | HEADERS+TRAILERS(2) | HEADERS+TRAILERS(2) | **HEADERS END_STREAM, NO TRAILERS** | HEADERS+TRAILERS(2) ✓ |
| `/Health/Nope` → `grpc-status 12` | HEADERS+TRAILERS(2) | HEADERS+TRAILERS(2) | **HEADERS END_STREAM, NO TRAILERS** | HEADERS+TRAILERS(2) ✓ |

**All three fixture arms are RED unpatched.** This is *good* for the gate — there is no already-green arm to explain — but **the SPEC must not repeat the Trailers-Only claim for this backend**, and the fixture must not be designed around an error arm that was expected to pass.

### 2.2 ⚠️ THREE OF THE FOUR "COSMETIC HEADER DIVERGENCES" ARE **REQUEST** HEADERS AND ARE NOT ON THE ASSERTION SURFACE AT ALL

Captured at the backend: `x-envoy-expected-rq-timeout-ms`, `x-forwarded-proto` and `x-request-id` are added by the reference **going upstream**. `GRPCHealthResponder` does not echo request headers, so they never appear downstream. **Only `x-envoy-upstream-service-time` is real**; the other two genuine items are `date`'s **value** and the `server`/`date` **wire order**. A fixture that "explicitly un-asserts the four" would be un-asserting three fields that cannot appear — vacuous coverage documented as care.

### 2.3 ⚠️ THE TRAILER BLOCK IS BYTE-EXACT CROSS-SIDE; ONLY THE **HEADER** BLOCK NEEDS CANONICALIZATION

Post-fix subject trailing HEADERS = `grpc-status: 0`, n=1 — **identical to reference and to control**, values and wire order. Error arms likewise (`grpc-message` then `grpc-status`). **Envoy adds nothing to a trailer block.** ⇒ trailers are pinned **verbatim**; the drop-filter applies to headers only.

### 2.4 ⚠️ `0079` IS **1,039** LINES, NOT 923 — AND THE 923 OMITS EXACTLY THE TWO COMPONENTS THIS ROW ALSO NEEDS

Measured per file: `driver.go` 731 + `driver_test.go` 86 + `expectations.yaml` 106 = **923**; **README.md 90 + pki 26 = 116 OMITTED**; total **1,039**. Competing reconstructions do not match (no-`driver_test` 953, no-`expectations` 933), so this is not coincidence. ⚠️ **The BRAINSTORM's own named analogue was under-enumerated by precisely the README and the PKI — `reference_measured_prototype_is_a_lower_bound` firing inside the figure the row used to price itself.** ⚠️ **And it mixes units:** 923 is **817 `.go` + 106 YAML**, scaled as if it were a `.go` budget.

### 2.5 ⚠️ THE ROUTER'S PORT OCCUPANCY IS **REFUTED**: 146 DISTINCT PORTS, NOT 39 — AND THE CONVENTION IS DOCUMENTED IN-TREE

Extracted from every driver's `ReferenceListenerPort()` body rather than by grepping literals: **117 of 120 fixtures bind ≥1 port, 184 slots, 146 DISTINCT ports, 19 bands, range 10000-19172.** Missing entirely from the router's list: `10014-10032`, `10034-10040`, `10081-10094`, `10100-10118`, `10120-10125`, `10200-10202`, `15042-15056`, and **`19140-19172` (33 ports / 32 fixtures — the largest band, where `0079`=19168 and `0080`=19169 live)**. Five of the router's entries (`12345`, `18000-18002`, `18007`, `19000`, `19999`) are **not reference listener ports at all** — they are dummy subject/admin args in unit tests and host-side backend ports in documentary YAML.

**The convention is `10<fixture index>`, stated in a landed comment** at `0118/driver/driver.go:29-31`: *"Convention `10<fixture index>` — 0114→10114 … so 0118→10118. ⚠️ NOT 10450: that is the TLS/SDS band (0108-0113), and this is not a TLS fixture."* ⚠️ **"10443 = TLS" is a leaked mnemonic, not a rule** — `0112`/`0113` are plaintext OTLP fixtures in that band, and the three TLS+ALPN-h2 fixtures sit at 15004 / 19168 / 19169. **`10119` is confirmed free** (0 hits in `test/`; NC `10118` = 3, fires).

⚠️ **Collisions are a documentation concern only.** `ReferenceListenerPort` is **container-internal**; `harness.go:157-163` reads back `c.MappedPort(...)` and the runner dials the random host port. **31 ports are already shared across fixtures** and `t.Parallel` count in `runner_test.go` is **0**.

### 2.6 ⚠️ MY OWN CONTROLLER BRIEF WAS WRONG: THE STAT SURFACE IS **NOT** GUARDED BY `TestNoNewStat*`

The brief told two agents that *"the stat surface is frozen and guarded by `TestNoNewStat*` tests asserting a delta of ZERO."* **False.** All five (`internal/statssink/registration_test.go:26/53/81/109/137`) assert that constructing **one specific stats sink** against a **fresh** registry registers zero metrics. They are **structurally blind** to a counter added in `internal/filter/hcm/`. **There is no global stat-surface freeze test in this repository.** Phases 80-83 all discharged +0 by **call-site enumeration** instead. **A controller brief is a claim too** — the phase-84 BRAINSTORM said the same of its own brief, and the species repeated one stage later.

### 2.7 ⚠️ A REFUTATION THAT ANSWERED A CLAIM THE BRAINSTORM NEVER MADE

One agent reported *"'H2→H2 unary works at the tip' — REFUTED AS STATED"*, reasoning that phase 84 is docs-only at HEAD. **The BRAINSTORM claims no such thing.** Its §3.3 row reads *"H2 listener -> H2 cluster (the discriminator) | 200 + correct 7-byte frame `00000000020801`"* — an **HTTP-layer** result establishing that blocker 1 (H1→H2 = 502) does not bind H2→H2. Its §5.1 item 1 states the opposite of the refuted claim explicitly: the subject fails `Internal: server closed the stream without sending trailers`. **The refutation is directed at a stronger claim than the one made** (`reference_refutation_must_answer_the_claim_as_stated`). Recorded because the *method* failure matters more than the verdict: two other findings from the same agent survived re-derivation intact.

### 2.8 ⚠️ AND THE COLLIDING CITE RESOLVES TO "NEITHER" — THE CAUSE IS AN **ABSENCE**

Two agents disagreed about `connection.go:467`. One called the cite wrong; the other called it exact. **Both are partly right, and the axis they held differently is *what counts as the cause*.** Re-derived: `connection.go:467` **is** `rf.SetAction(action)` — the cite is exact. But the H1 selection happens at `actions.go:211-216`, where `asRouterAction()` builds `router.H1ClusterAction(...)` inside a `sync.Once` — **there is no branch to quote**. The decisive measurement subsumes both: **`UseH2` has ZERO non-test, non-comment occurrences anywhere in `internal/filter/hcm/`** (NC: the same pipeline finds `SetAction` at two sites, so it is not blind). The only live consultation tree-wide is `extauthz/check.go:571`; `config.go:683-689` concedes the hole **in comments only**. ⇒ **no single line is the cause; the absence is.**

### 2.9 ⚠️ THE PLAINTEXT-H2 BOOT-REJECT IS **CODEC-CONDITIONAL** — WITH `AUTO` IT IS A SILENT RUNTIME FAILURE

| config | result |
|---|---|
| plaintext + `codec_type: HTTP2` | **BOOT REJECT** (exit 1): *"hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c…)"* — `config.go:239` |
| plaintext + `codec_type: HTTP2` + `--allow-h2c` | `configuration OK` (NC fires) |
| **plaintext + `codec_type: AUTO`** | **BOOTS FINE**; the gRPC client then fails at runtime with *"error reading server preface: http2: frame too large"* |

**The conclusion (fixture must be TLS+ALPN-h2) survives; the stated mechanism does not.** `--allow-h2c` occurrences in `test/differential/`: **0** (NC: the flag exists at `cmd/envoy-go/main.go:40`).

### 2.10 ⚠️ `test/conformance/grpc/`'s COST IS **RETIRED AS A BUDGET ITEM** — BUT NOT BY THE ARGUMENT THE BRAINSTORM ANTICIPATED

The BRAINSTORM calls it *"the single largest unpriced item and the strongest candidate to be this row's under-enumerated line."* It does not bind, on **two independent grounds** reached by two agents on unrelated remits (§4).

### 2.11 ⚠️ THE BRAINSTORM'S OWN CHARTER MISCOUNTS ITS OPEN QUESTIONS

§4 enumerates **FIVE** numbered questions; §10 (`NEXT`) says *"the four §4 open questions"* and `PROGRESS.md`'s handoff says *"the four open questions"*. The router says five. **This SPEC disposes of five** (§3-§7). Recorded because a stage that answered "the four" would have silently dropped one — and the dropped one would have been Q5 (PKI), the only one with a landed-cost fork.

### 2.12 ⚠️ THE SECOND `BOOTSTRAP_PROMPT.md` IS NOT A COPY — IT IS THE **GENERATOR PLAN**, AND IT IS CONTENT-DIVERGENT

`docs/superpowers/plans/2026-04-21-envoy-go-bootstrap-prompt.md` (**1024** lines) carries an **unexpanded placeholder** `<family-list-from-spec-§7.2-verbatim>` at its `:689` where the root file carries the actual family list. **`BOOTSTRAP_PROMPT.md:402` — the gRPC charter bullet the ROADMAP cites — has NO counterpart in it at all.** Offsets measured at four anchors: **+228** (§7.3 bullets, §7.5 gate (c), §7.4 sentence) but **+165** at a §4 anchor. **Non-constant offset confirmed; "content-divergent" is the accurate hazard, and it is stronger than "wrong anchors".**

### 2.13 Also refuted — eight more, each with its cite

- ⚠️ **`STATE_HISTORY.md` is 456, not the router's 454** — 454 was the *pre*-BRAINSTORM figure; that stage's own append took it to 456 and the router's counts block was not rolled with it. `STATE.md` §Current has the transition right (`454 -> 456`). **Third consecutive roll to carry a stale absolute here.** Every other doc count in that block verifies exactly (`STATE.md` 64, `ROADMAP.md` 234, `DECISIONS.md` 17926, `BEHAVIOR_CONTRACT.md` 5900).
- ⚠️ **`expectations.yaml` is NOT a registration gate** — present in **96 of 120**, and `git grep -n 'expectations\.yaml' -- '*.go'` minus comment lines returns **zero**; all 17 raw hits are `//` comments. NCs in the same form fire (`envoy.yaml` → 78 files including real code at `0004/driver/driver.go:177`). **Documentation convention, zero programmatic consumers.**
- ⚠️ **The `0004` generator is not a build step** — the repo's only `//go:generate` (`0004/doc.go:8`) is documented *"CI does NOT run `go generate`; only humans run it"*. So Q5's "3 PEMs vs a 173-line generator" is a **false dichotomy** (§7).
- ⚠️ **`0068` does not import grpc-go at all** — it drives HTTP/1.1 via `helpers.HTTPRoundTrip`; the grpc-go server is the **runner's**. ⚠️ **0 of 179 fixture `.go` files import grpc-go** ⇒ `0119`'s driver is **first-of-kind, with no shape to copy**.
- ⚠️ **The driver-receiver port-race roster "FOURTEEN" is superseded** — 14 is the `ensureServer`-symbol roster; the behavioural roster is **42**. Phase 84 is **outside the class**: `GRPCHealthResponder` is runner-spawned on a kernel-ephemeral port (`runner_test.go:1024-1037`), and `0119`'s driver adds no `ensureServer`.
- ⚠️ **CI budgets are not a constraint, and the brief's worry is unfounded** — `ci.yml:60` `-timeout 20m`, `:63` `timeout-minutes: 30`, `runner_test.go:237` 90 s per fixture, all verified **not stale**. Three comparables measured with `-count=1 -v`: `0004` **2.74 s**, `0068` **1.83 s**, `0079` **2.17 s**. Headroom ≈ **254 more fixtures**; a 121st costs ~0.83%. ⚠️ **Real finding instead: CI has NO `docker pull` step** — the 299 MB image is pulled lazily inside the **first fixture's 90 s ctx** while `ci.yml:57` reasons about it against the 20 m budget. ⚠️ **And `-race` never covers the differential** (`-short` skips it at `runner_test.go:143`).
- ⚠️ **The xDS cycle guard is not automated anywhere** — 13 `go list -deps` hits, all documentation, plus one **unexecuted comment** at `upstreampool_test.go:104-111`. It cannot trip here regardless: `internal/filter/hcm/h2` is a leaf and `internal/filter/http/router` already imports it (`router.go:15`).
- ⚠️ **`git grep` with a quoted pathspec ending in `/` returns 0 lines / 0 files** where the shell-glob form returns 70 — **a fail-unsafe zero**, same family as the leading-hyphen trap. Added to §14.

---

## 3. D-84-H2SPEC — Q1 DISPOSED: **THE GATE NEVER RAN THE SECTION. RECORD IT; DO NOT FIX IT HERE.**

Full disposition at §1.1. **Decision:**

1. **This row does NOT repair the selectors.** The repair is one character per string, but it converts a green gate into a red one over **four pre-existing envoy-go failures** (three SETTINGS-validation, one flow-control) that belong to neither leg of this row. Folding them in crosses §6.1 on a second axis.
2. **This row does NOT cite "h2spec 53/53" as evidence of anything.** Gate (c) for both legs states the figure **with its scope caveat** (§15).
3. **The finding is handed forward by name** (§13) — the repair, the four failures, and the missing `tests == 0` guard are one coherent future row.
4. ⚠️ **`ADR-0051` §2 is factually wrong and is NOT edited** — `DECISIONS.md` is append-only per ADR-0288 §Decision 4. `ADR-0306` §Context records the contradiction instead.

---

## 4. D-84-CONFORMANCE — Q2 DISPOSED: **DEFER IN WRITING. ZERO LINES. TWO INDEPENDENT GROUNDS.**

**Ground (i) — TEXTUAL.** §7.5(c) reads *"the phase's conformance suites pass **at the declared threshold**."* §7.3 declares a threshold for exactly **two** of its four suites: h2spec (*"pass threshold is a phase gate"*) and proxy-wasm (*"62.5% threshold; phase-done gate = all 10 PASS"*). The h3spec and gRPC lines are **bare** — `test/conformance/grpc/` — gRPC interop client.` **No threshold is declared, so gate (c) has nothing to bind to.**

**Ground (ii) — LANDED PRECEDENT, ON ALL FOURS.** `test/conformance/h3spec/` is also §7.3-declared and **has never existed**. Yet the **HTTP/3 + QUIC family OPENED at phase 61**, and:

- `phases/61-http3-downstream-listener/BRAINSTORM.md:94` — the reusable formula: *"**h3spec conformance gate first** — a conformance harness with no implementation to test is vacuous. Deferred (§8)."*
- `SPEC.md:39` §Non-purposes: *"NO **h3spec** conformance gate."*
- ⚠️ **Phase 61 mentions `h2spec`/`proxy-wasm`/"gate (c)"/"declared threshold" ZERO times across all EIGHT of its documents** — it did not even assert the two existing suites unaffected.
- `ROADMAP.md:194` still carries *"h3spec conformance gate"* as a deferred candidate **23 phases later**, and the family is still open.

**Ground (iii), supporting.** The §9 charter (`BOOTSTRAP_PROMPT.md:402`) names four gRPC bullets — bridge, gRPC-Web, JSON transcoding, **interop conformance**. **This row is none of them; it is their prerequisite.** Interop conformance was never in this phase's scope, so §6.3's "no third option" anti-pattern does not bite.

**Supporting measurement, so the deferral is priced rather than waved through.** Of the **26** interop cases in `grpc-go@v1.70.0` (`interop/client/client.go`, 25 `Do*` functions): **9 reachable (34.6%)**, **8 structurally un-runnable** behind the response-buffering seam, **9 not proxy tests** (cloud creds / client-side LB). And the canonical driver package **cannot be imported without adding `golang.org/x/oauth2`** — measured: `google.golang.org/grpc/interop/grpc_testing` (the protos) builds with **zero go.mod delta**, while `google.golang.org/grpc/interop` fails with *"missing go.sum entry … golang.org/x/oauth2"*. Comparable harness sizes, re-measured: h2spec **388** Go lines (293 code, 13.4% comment), proxy-wasm **1131** (652 code, 32.9%).

⚠️ **DECISION: NOT BUILT. The deferral is already written at `ROADMAP.md:200`; this SPEC ratifies it and gate (c) states it with the denominator** (§15). **Cost: 0 Go lines.**

⚠️ **The `BOOTSTRAP_PROMPT.md:350` forward-pointer sentence is NOT edited here** — that file is outside every stage's edit surface in this lineage, and phase 61 set the precedent by deferring `h3spec` without touching it.

---

## 5. D-84-ASSERT — Q3 DISPOSED: **A CANONICAL TEXT TRANSCRIPT. HEADERS FILTERED BY NAME; TRAILERS VERBATIM.**

`CompareBytes` (`test/differential/diff.go:18`) is a plain byte comparator with a hex window — **the harness offers no semantic diff, so canonicalization lives entirely in the driver's `Drive*` return value.** `HTTPExpectations` is unusable (its single dispatch site at `runner_test.go:1302` calls `helpers.HTTPRoundTrip`: plaintext HTTP/1.1, no TLS, no ALPN, no trailer read). A `StatsAsserter`-only fixture is **vacuously green** (§10).

**Mandated shape.** `DriveReference`/`DriveSubject` both call one shared `drive()` which, per arm, opens a fresh TLS+ALPN-h2 connection, speaks raw HTTP/2, and appends:

```
ARM <name>
  HEADERS  end_stream=<bool> [<filtered, name=value …>]
  DATA     end_stream=<bool> len=<n> hex=<payload>
  TRAILERS end_stream=<bool> [<VERBATIM wire-order name=value …>]
```

- **Headers: drop `x-envoy-upstream-service-time` and `date` BY NAME.** Keep everything else, including `server=envoy`, `content-type`, and the three `trailer:` announcements — all byte-identical cross-side.
- **Trailers: verbatim, wire order, no filtering** (§2.3).
- ⚠️ **`end_stream` per frame is the LOAD-BEARING field** — it is where the RED actually lands: `first divergence at offset 182 · ref "DATA end_stream=false" · subj "DATA end_stream=true"`.
- **Read errors are recorded INTO the transcript (`READ-ERR`), never returned** — a divergence must surface as a `CompareBytes` mismatch, not a one-sided `t.Fatalf` (`reference_fatalf_makes_assertions_unreachable`).

**Verified by construction: the prototype ran RED at this tip and GREEN with the fix, 3/3 deterministic**, and `golangci-lint run` on the fixture package exits 0.

⚠️ **AND THE CANONICALIZATION WAS ITSELF NEGATIVE-CONTROLLED, WITH ONE ARM THAT DID NOT FIRE.**

| NC | fired? |
|---|---|
| remove the header **drop-filter** on a CORRECT tree | **✅ FIRED** — FAIL, divergence landing exactly on `x-envoy-upstrea…` |
| remove `sort.Strings` on a CORRECT tree | **❌ DID NOT FIRE** |
| remove the **blank import** | **✅ FIRED** — `SKIP` + overall `PASS` |

⚠️ **THE SORT IS VACUOUS AND THE SPEC SAYS SO.** Once `date` is dropped, `server` is last on both sides and wire order already agrees. **The PLAN must either omit the sort or justify it explicitly as future-proofing — it must NOT claim the sort is what fixes the order divergence**, because the measurement says the drop-filter is.

---

## 6. D-84-ADR0058 — Q4 DISPOSED: **NARROW, NOT SUPERSEDE — AND THE CLAUSE SPLIT IS STATED**

ADR-0058's §Decision sentence *"Trailers are observed-and-discarded **in both directions**"* is a two-clause conjunction. **Row 84 falsifies exactly one clause.** Superseding would silently retire three things the row does not touch.

| ADR-0058 clause | in the carve? |
|---|---|
| Upstream-from-server trailing HEADERS discarded during `H2Response` assembly | **YES — this is what the row fixes** |
| Downstream-from-client (**request**) trailing HEADERS discarded by `serverStream` dispatch (`recvTrailingHeaders`) | **NO — explicitly OUT** |
| M-4 carry-forward (`readClientPreface` not ctx-aware) | **NO — unrelated bundled item** |
| M-10 carry-forward (`SETTINGS_TIMEOUT` absent) | **NO — unrelated bundled item** |

- **SURVIVES:** the downstream-from-client half, including `stream.go:recvTrailingHeaders`'s two RFC 9113 validations; the M-4 and M-10 carry-forwards verbatim.
- **NARROWED:** *"the assembled `H2Response` carries only the FIRST HEADERS block"* and *"the router … never via a trailing HEADERS frame"* — both become false **for the H2-downstream case only**.
- **FULFILLED, not contradicted:** the §Consequences forward-pointer (§1.2). `ADR-0306` cites it as a **fulfilment**.
- **STILL TRUE after the row:** H1, H3, request trailers, and the WASM `RunEncodeTrailers` hook.

⚠️ **ADR-0058 CARRIES A STALE CITE THAT MUST NOT BE PROPAGATED** — it names `internal/filter/hcm/actions.go:routerActionH2.doH2` twice; **that symbol is not there.** `routerActionH2` is at `internal/filter/http/router/router_h2.go:254` and the emit is `writeH2Reply` at `internal/filter/hcm/h2dispatch.go:671`. Its other two cites (`h2/client.go dispatchFrame`, `h2/stream.go:recvTrailingHeaders`) are live and correct. **Recorded, not fixed** — append-only.

---

## 7. D-84-PKI — Q5 DISPOSED: **COPY THE THREE PEMs. THE FORK IS A FALSE DICHOTOMY.**

⚠️ **`0004`, `0079` and `0080` ship BYTE-IDENTICAL PEMs** — `sha256sum` over `ca.pem` / `listener.pem` / `listener.key.pem` matches across all three, and `0079/driver.go:209` states the provenance in a landed comment (*"the 0004 PKI, copied"*). So the real choice is **"copy 26 lines"** vs **"copy 26 lines + 173 lines of tooling CI never runs"** (§2.13).

**Population, measured across 120 fixtures:** 10 ship PKI files (4 static-only, 3 generator+committed, 2 generator-only with gitignored PEMs); **4 more (`0108`-`0111`) generate in-memory with nothing on disk** — a class the BRAINSTORM's taxonomy omits. **There is no shared PKI helper**; `test/helpers/tls.go` is client-side only.

**DECISION: copy `0079`'s three PEMs + ~31-60 lines of driver plumbing** (`readPEM`, `fixtureDir`, `indentPEM`, `ensureCertPool`, `tlsConfig`). **No generator.**

- **No time bomb:** both certs are `notBefore=2026-01-01`, `notAfter=2046-01-01`, ECDSA P-256 — **19.4 years remaining**. SANs `DNS:localhost, DNS:host.docker.internal, IP:127.0.0.1`, so **one cert validates against both the containerised reference and the host subject** with `ServerName: "localhost"`.
- **No Docker mount work:** `git grep ReferenceHostMounts` over `0004`/`0079`/`0080` → **0 hits** (NC: 12 other fixtures implement it). PEMs are read host-side and spliced into **both** bootstraps as `inline_string`.

---

## 8. THREE NEW FORKS THIS STAGE OPENED

### 8.1 ⚠️ D-84-ENDSTREAM — **THE HIGHEST-LEVERAGE UNPRICED DECISION IN THE ROW. DISPOSED: CONDITIONAL.**

`writeH2Reply` currently computes `endStream := len(body) == 0` (`h2dispatch.go:699/:703`). The row must hold END_STREAM off HEADERS/DATA so a trailing block can carry it. **Two implementations, one line apart, with a two-order-of-magnitude difference in blast radius:**

| variant | blast radius |
|---|---|
| hold END_STREAM off **unconditionally** | the wire frame sequence changes for **every** H2 response; **40 of 120 fixtures** declare `http2_protocol_options` |
| hold it off **only when trailers are non-empty** | **nil** — byte-identical wire output on every existing path |

**DECISION: CONDITIONAL.** Neither the BRAINSTORM nor the router names this fork; it was found by census.

⚠️ **AND THE ENCODE CHAIN IS OTHERWISE TOLD A LIE.** `h2dispatch.go:575` passes `len(resp.Body) == 0` to `RunEncodeHeaders` and `:583` passes a hardcoded `true` to `RunEncodeData`. With trailers following, END_STREAM is on **neither** frame. A bodyless-response-with-trailers — reachable when a gRPC error is raised *after* headers — would tell an encode filter `endStream=true` while a trailing block still follows. **The PLAN must specify what the chain is told, and it is not the same question as what the wire carries.**

### 8.2 ⚠️ D-84-CONTINUATION-ENCODE — A **MIRROR** DEFECT ON THE ROW'S OWN NEW CALL PATH

The BRAINSTORM's §1.3 defect is on the **decode** side. The row's new emit lands on an **encode**-side path with the symmetric hole: `ServerConn.encodeAndWriteHeaders` (`h2/conn.go:681-695`) hardcodes `EndHeaders: true`, writes one `WriteHeaders`, and **never consults `s.clientS.MaxFrameSize`** — whereas `writeData` **does** cap at `peer.MaxFrameSize` with the RFC default 16384 (`conn.go:721`). `WriteContinuation` has **zero call sites tree-wide** (NC: `WriteHeaders(` appears in 4 production files).

**Measured**, 60 response headers × 1200 bytes, client advertising the default `MAX_FRAME_SIZE=16384`:

```
envoy-go   : RECV HEADERS len=58469 flags=0x04       ← ONE frame, 3.6x the peer's limit
reference  : HEADERS 16384 → CONTINUATION 16384 ×2 → CONTINUATION 9315 (flags 0x04)
```
A stock `x/net/http2.Transport` — the transport grpc-go is built on — gets **`http2: frame too large`** from envoy-go and **200** from the reference.

⚠️ **DISPOSITION: OUT OF SCOPE, EXPOSURE STATED.** A successful unary trailer set (`grpc-status: 0`, short `grpc-message`) encodes to **tens of bytes — three orders of magnitude under the threshold — so the row's success path is safe.** But the row **adds a new call site to an already-defective path**, and upstream-controlled trailer bytes become downstream-re-emittable. Exposure begins at ~16 KB of trailer metadata (a long `grpc-message` or a large `grpc-status-details-bin`). **This must be a stated non-goal in `ADR-0306`, not silence.** ⚠️ The 16384 figure is **derived from source plus the reference's observed split boundary, not bisected.**

### 8.3 ⚠️ D-84-VALIDATE — THE CLIENT SIDE ENFORCES **NEITHER** RFC RULE THE SERVER SIDE DOES

`serverStream.recvTrailingHeaders` (`h2/stream.go:138-165`) enforces two rules on inbound trailers — they MUST carry END_STREAM, and MUST NOT carry pseudo-headers (both `PROTOCOL_ERROR`). **The upstream client path enforces neither**, and a minimal `else` branch accepts anything with last-one-wins on a second block. Separately, RFC 9110 §6.5.1 bars `content-length`, `transfer-encoding`, `te`, `trailer`, `host` and others from a trailer section, and RFC 9113 §8.2.2 bars connection-specific fields — **an upstream can smuggle `content-length` into the downstream trailer block.**

**DECISION: IN SCOPE FOR 84.1**, at ~27 production + ~110 test lines. It is cheap, it is on the path the row opens, and shipping an unvalidated upstream→downstream header conduit is a worse outcome than the row's stated defect.

---

## 9. BLAST RADIUS

**Production roster — FOUR files confirmed by two independent builds**, and the fifth inherited file confirmed **unnecessary**: `StreamWriter.WriteHeaders` (`h2/stream.go:31`) is already generic and `serverStream.WriteHeaders` (`:233`) has **no guard against a second call**, so `sw.WriteHeaders(tf, true)` needs **zero interface change**. `h2/stream.go` stays byte-untouched.

| file | role |
|---|---|
| `internal/filter/hcm/h2/client.go` | capture the trailing HEADERS block (today `:440`, observed-and-discarded); add `H2Response.Trailers` |
| `internal/filter/http/router/router.go` | `ActionResponse.Trailers` carrier |
| `internal/filter/http/router/router_h2.go` | populate in `doH2ClusterAction` |
| `internal/filter/hcm/h2dispatch.go` | `writeH2Reply` — conditional END_STREAM + trailing HEADERS emit |

**Two independent measurements of variant A:** `+67/−18` (net +49) and `+53/−11`. **The spread is the comment fraction, not the logic** — the heavier build wrote the doc comments a landed version needs (31.3%, already at the house median), so the "add comments later" headroom §3.4 implies **does not exist for this patch**.

**Arm census, run rather than assumed.** `sw.WriteHeaders(|sw.WriteData(` over non-test `internal/*.go` → **3 server-side END_STREAM emit sites in 2 files** (NC: the same pattern in `_test.go` returns 9 files / 41 lines):

| site | disposition |
|---|---|
| `h2dispatch.go:699 + :703` — `writeH2Reply` | **the chartered arm** |
| `h2dispatch.go:723` — `write500H2` | must be disposed ("no upstream ⇒ no trailers") |
| ⚠️ **`actions.go:135 + :138` — `directResponseAction.writeH2`** | **A SECOND FILE, NOT IN THE 4-FILE ROSTER** |

**Good news, stated as a measurement: the census is bounded at 3, not 11.** Phase 83's equivalent widened one chartered arm to eleven; this one does not.

**Fan-out.** `ActionResponse` is referenced in **19 `.go` files** and constructed at **22 sites (18 non-test)** — a new field is *additive*, no compile break, but the PLAN must say which sites populate it. Precedent for a codec-scoped field is a **comment**, not a type: `ActionResponse.Close` carries *"(H1 only; H2 ignores)"*. `H2Response` fan-out is **2 files / 15 lines**.

**Existing-test churn.** `writeH2Reply`'s own doc comment (`h2dispatch.go:669-670`) states the rule the row breaks verbatim. Churn surface: **69 lines mentioning `endStream`/`END_STREAM` across 7 test files** in the two packages. Test-function denominators: `internal/filter/hcm` **211**, `internal/filter/hcm/h2` **77**, `internal/filter/http/router` **68**.

**Provably outside.** H1 (`writeH1Reply`, `codec.go:74`) and H3 (`writeH3Reply`, `h3dispatch.go:33`) keep 4-arg signatures and are byte-untouched; `doH1ClusterAction` never populates `Trailers`. ⚠️ **Both are now a *visible* asymmetry rather than a uniform rule and each needs one explicit non-change sentence plus a regression test** (`reference_one_sided_gate_for_a_two_sided_fix`). H3 is structurally reinforced: `SetH2Action` has one non-test call site.

---

## 10. DIFFERENTIAL AND FIXTURE POSTURE

**ONE new fixture, `0119-grpc-unary-trailers`, reference port `10119`, `GRPCHealthResponder` BackendKind 34 reused unmodified** (verified: `fixture.go:576`; a real grpc-go server at `runner_test.go:3106`; used by exactly one fixture, `0068`). **No new BackendKind. No new module** — `google.golang.org/grpc v1.70.0`, `golang.org/x/net`, `google.golang.org/protobuf` and `genproto/googleapis/rpc` are all already **direct** requires, verified by building a realistic driver in a throwaway copy: `go mod tidy -diff` → **0 bytes** (NC: adding go-cmp → rc=1, 404 bytes, **fires**).

**The three registration gates, all located** — (1) `discoverFixtures` `runner_test.go:1461-1497`, predicate `^[0-9]{4}[a-z]?-`; (2) `fixture.RegisterFixture` from the driver's `init()`, `fixture/fixture.go:92`, name **must equal** the directory name; (3) the blank-import block at `runner_test.go:26`. **All three currently 120/120/120.** ⚠️ **Gate-3's silent-green path was reproduced live**: with the blank import removed the run prints *"no driver registered"* and reports **`--- SKIP` + overall `PASS`** (`runner_test.go:199`).

⚠️ **BROKEN-GATE SHAPE 31 IS CONFIRMED AND STRENGTHENED TO A CROSS-SIDE RESULT.** The subject books `upstream_rq_2xx: 2` / `downstream_rq_2xx: 2` after **two failed RPCs**, because the HTTP response *is* a 200 and the gRPC failure rides trailers. **82 of 120 fixtures define an `AssertStats` method.** And the reference A/B (trailers vs no-trailers, pinned image) moves an **identical 44-name set** out of 375 — **symmetric difference ∅** — while `cluster.*.http2.trailers` stays **0** and a firing positive control (downstream *request* trailers) moves root `http2.trailers` 0→1. ⇒ **a stats comparison cannot discriminate a correct tree from a broken one on EITHER side.** Envoy books nothing for response trailers.

**Layout: `0119-grpc-unary-trailers/driver/driver.go`** — the `driver/` layout is **96 of 120**; `inputs/` is 24 of 120 and confined to fixtures 0016-0039.

⚠️ **AN UNDECIDED ITEM WORTH ±220 LINES, RULED HERE.** `envoy.yaml`/`envoy-go.yaml` documentary mirrors are shipped by **74/75 of 120** fixtures — but **not by `0004`, `0079`, `0080` or `0068`**, the four closest analogues, all of which build their bootstrap in Go. **DECISION: no YAML mirrors**, matching the analogue set. The PLAN must not re-open this silently.

---

## 11. CONTRACT AND ADR EDITS

### 11.1 `ADR-0306` — §Context drafted at this SPEC, STATUS `PROPOSED`

Written into `DECISIONS.md` at this stage per **ADR-0044-as-used** (⚠️ ADR-0044 does **not** contain the discipline — re-verified by token scan over its 47 lines; the fullest written statement is a **reference note** at `DECISIONS.md:8534`, not an ADR clause). Counts this block moves: `^## ADR-` headings **304 -> 305** · retained italic footers **11 -> 12** · STATUS census **18 -> 19** · **the strict guard `^> \*\*STATUS: PROPOSED` goes 0 -> 1, which is CORRECT and is disarmed by the phase-84 IMPL** · `^---$` **STAYS 216** (last at `:17020`).

⚠️ **`next-free = headings + 1` COLLIDES AND MUST NOT BE "FIXED"** — ids span 0001-0305 with **exactly one gap, at ADR-0209** (verified: zero duplicates), so heading arithmetic yields 0305, an id already taken. **Derive from the TAIL.**
⚠️ **Do NOT use the loose matcher `^> \*\*STATUS: .*PROPOSED`** — it returns **5** at this tip and **all five are false positives** (COMPLETE blocks narrating the word). ⚠️ **Completing a PROPOSED ADR does not restore the pre-draft count; it converts the block into one more false positive. CARRY NO LIVE WHOLE-FILE COUNT OF THAT PATTERN.**
⚠️ **The retained-footer set is NOT contiguous** — 11 instances at phases 72-78 and 80-83; **phase 79 has none**. Anyone deriving "one per phase since 72" gets a wrong denominator.

### 11.2 `BEHAVIOR_CONTRACT.md` — **BYTE-UNTOUCHED at this stage; OWED at the 84.1 IMPL, and it is NOT a cheap tail append**

⚠️ **ADR-0052 `:1821` makes `ADR-0306` the mandated vehicle** (§1.2). Phase 84 **falsifies five landed statements**: `:16` (the `| Response trailers | Set-equal … |` row, an *asserted* equivalence that is currently unreachable), `:682`, `:2043` (*"Trailers — observed but not forwarded per ADR-0058"*), `:2068`, and the `#### Trailers and :scheme coverage boundaries` block at `:4291-4311`. The file's own preamble: *"either the contract is updated via ADR or the implementation is fixed — never both silently."*

**Cite-shift arithmetic, measured:** 197 `BEHAVIOR_CONTRACT.md:NNNN` cites across 71 files (max **5078**) plus 76 `BC:NNNN` cites (max 5020), against a 5900-line file. **A tail append shifts 0 of 273. An INSERT at `:2043`/`:2068` shifts 23 of 197.** ⇒ **prefer an in-place net-zero rewrite at the falsified statements plus a tail append for the new subsection.** Precedent: 6 of 7 recent IMPLs touched the file (+2…+61, median **+21**); only phase 82 was a pure tail append. **0 of 7 SPECs and 0 of 7 PLANs touched it.**

⚠️ **`5078` is itself already stale** — phase 80's own mid-file inserts moved that sentence to `:5124`. ⚠️ **And its companion denominator ("195 cite occurrences") is wrong — 196 tracked** — because it was measured with the recursive grep that is **blind to the tracked-but-gitignored `next-prompt.txt`**, which carries `BEHAVIOR_CONTRACT.md:1967`.

### 11.3 ⚠️ COUNTS THIS SPEC AND `ADR-0306` DELIBERATELY DO **NOT** CARRY

The `ROADMAP.md:<line>` and `BEHAVIOR_CONTRACT.md:<line>` cite totals (self-falsifying — **five** prior firings, the most recent *inside the count that documented a false-positive gate*); the `^> \*\*STATUS: .*PROPOSED` whole-file count; the stat-surface absolute (**1207**, DOC-SOURCED across two unaudited ledger gaps — ⚠️ **not** the **1205** `STATE.md:33` carries, which has been stale since phase 76 and must never be sourced); the `STATE_HISTORY.md` archive-gap total; `allCallbacksNoOp` occurrences. **Only DELTAS are asserted.**

---

## 12. COST AND SPLIT — **§6.1 CROSSES AT THE FLOOR, AND THIS SPEC SPLITS THE ROW**

### 12.1 The enumeration — floor, ceiling, and what each rests on

| bucket | **floor** | **ceiling** | anchored on |
|---|---|---|---|
| production `.go` | **46** | **200** | two independent variant-A builds; ceiling adds the 39.2% comment fraction, the 3-arm census, 3 call sites, the H1/H3 non-change comments, 4 ADR-0058 comment rewrites, and `writeH2Reply`'s own falsified doc comment |
| unit-test `.go` | **892** | **1400** | **892 is the SMALLEST unit-test bucket any of the last four rows produced** (phase 80 — a *three*-production-file row). Phase 84 spans 3 packages (211/77/68 test funcs), needs a frame-sequence capture double, and carries 69 lines / 7 files of churn |
| fixture driver `.go` (0119) | **764** | **1150** | **764 = the FLOOR of every H2/TLS/gRPC analogue** (`0080` 764, `0068` 766, `0004` 792, `0079` 817). Corpus n=120: mean 650, median 656, p75 776, p90 977; **recent-10 mean 795**. ⚠️ **FIRST-OF-KIND — 0 of 179 fixture `.go` files import grpc-go** |
| PKI plumbing `.go` | **31** | **60** | measured from `0079` |
| harness registration `.go` | **5** | **30** | blank import + registration |
| **NET `.go` TOTAL** | **≈ 1738** | **≈ 2840** | **midpoint ≈ 2290** |

**LABEL: LOWER BOUND.** Measured prototype fixture leg alone: **559 lines** (driver 532 + PKI 26 + 1 import), against an enumerated **860-1180** once README (118/120 carry one, median 135), `expectations.yaml` (96/120, median 96), `driver_test.go` (34/120, median 81), an `AssertStats` surface leg, error-path hardening and landed break arms are added.

**Non-`.go`, real but outside §6.1's "lines of code":** fixture README ~90-213 · `expectations.yaml` ~84-278 · 3 PEMs · `ADR-0306` ~66 (ADR-0301..0305 span 62/66/72/62/67) · `BEHAVIOR_CONTRACT` ~+21 plus the 5-statement reconciliation · `STATE.md` ±9 · `STATE_HISTORY.md` +2 · `next-prompt.txt` · `PROGRESS.md` per stage.

### 12.2 The post-mortem that produced those floors

| phase | PRODUCTION | UNIT-TEST | FIXTURE | **TOTAL** | budget | ratio |
|---|---|---|---|---|---|---|
| **80** | 128 (3 f) | 892 (7 f) | 616 (3 f) | **1636** | ~640 | **2.6x** |
| **81** | 424 (7 f) | 1805 (9 f) | 0 | **2229** | ~850 | **3.07x** |
| **82** | 597 (8 f) | 2014 (8 f) | 64 (1 f) | **2675** | ~1050 | **2.55x** |
| **83** | 615 (10 f) | 2921 (11 f) | 0 | **3536** | ~1950 | **1.81x** |

Method: stage commits located **by subject**; `git show --numstat` per commit. **12 of 12 BRAINSTORM/SPEC/PLAN commits across 80-83 carry ZERO `.go`** — every line lands in the squashed IMPL, so the IMPL alone is a sufficient denominator. Totals reproduce the lineage's own published figures exactly. ⚠️ **This is a change-set measure, not a build measure** (`reference_change_set_measure_not_build_measure`) — correct for a budget question, wrong for a gate.

⚠️ **The phase-83 IMPL's own split does not reconcile:** `83/PROGRESS.md:171` records *"production +202, test +3334"*; numstat gives **615 / 2921** — a 413-line reassignment on an identical total. **The lineage's post-hoc production/test accounting is not a reproducible measure**, which matters precisely because §3.4's model is built on a ratio derived from it.

**Nine item-categories present in a landing and absent from the budget that preceded it** — arm-count under-enumeration (bounded at 3 here), out-of-roster packages (`actions.go` is a 3rd emit site), comments priced at zero, call-site fan-out of a signature change, existing-test churn, prose mandates the budget table omits, conditionals priced at zero (**Q3 and Q5 were exactly this shape until §5/§7 disposed them**), gate-construction lines invented at the gate stage, and **the mirror** (H1/H3, decode↔encode). A tenth is new to this row: **contradicted-document reconciliation** (§11.2).

### 12.3 ⚠️ THE VERDICT — AND THE SPLIT

> **THE ~1500 NET-`.go` TRIGGER CROSSES.** The empirical floor is **≈1738**, computed with the smallest unit-test bucket and the smallest fixture any comparable row produced; the central estimate is **≈2290**. **No row in the last four landed under 1636, and that minimum was itself a §6.1 crossing.**
>
> **THE ~25-TASK TRIGGER DOES NOT FIRE** — enumerated at **15-20**. (Precedent: 13 tasks → 1636; 12 → 2229; 18 → 3536.)
>
> **THE MID-EXECUTION TRIGGER IS AT RISK** on the fixture-driver task — first-of-kind grpc-go client, TLS+ALPN, Drive hooks, `CompareBytes`. It fired on phase 80's T4 (6 enumerated, 17 executed).

**SPLIT ON AXIS A, and — unlike phase 83 — the axis does NOT cut a correctness constraint.** Phase 83 refused to retro-split because *"every available split axis cuts through a correctness constraint."* **That was checked here and it is not true:**

| axis | cuts a correctness constraint? |
|---|---|
| **A. `84.1` = the seam (4 production files + validation + unit tests) · `84.2` = differential fixture `0119`** | ⚠️ **NO.** No compile dependency, no ordering constraint, no shared file. **84.1's RED anchor already exists OUTSIDE the harness** — grpc-go → `Internal: server closed the stream without sending trailers` → `SERVING`, 3/3 deterministic. Splits ≈940-1600 from ≈800-1240. |
| B. capture upstream / emit downstream | **REJECT** — 84.1 alone would be dead code (captured, never emitted): §6.3's *"incomplete stubs the differential can't exercise."* |
| C. docs/ADR as a leg | **REJECT** — not a row. |
| D. fold in the CONTINUATION defect | Not a split; already an out-of-scope deferral (§13). |

⚠️ **THE PRECEDENT IS EXACT, AND IT IS A FAMILY OPENER.** Phase **61 — the HTTP/3 family opener — split 61.1/61.2/61.3 AT ITS SPEC**; the commit subject reads *"…ADR-0279 Context drafted, **a 61.1/61.2/61.3 split**"*. Its legs map one-to-one onto this row's: 61.1 substrate (no differential), 61.2 codec — *"`runH3`/`ServeH3`/**`writeH3Reply`**"*, the direct sibling of the function phase 84 edits — proven **subject-side only**, and **61.3 the differential leg, "the FINAL leg… flips ROADMAP row 61 done."**

⚠️ **AND THE SPLIT COSTS NO ROADMAP ROW — MEASURED, NOT ASSUMED.**

- Phase 61's splitting SPEC commit `6a26909b` touched **exactly one file**: `SPEC.md`, **+274/−0**. **`ROADMAP.md` BYTE-UNTOUCHED.**
- **Zero dotted rows exist above 32.2** — the highest are 29.x, 32.1, 32.2. Split legs are recorded in the parent row's **`sub-phases` PROSE** (row 60 does exactly this), and the final leg's IMPL flips the parent `done` (**ADR-0106-as-used**, `reference_roadmap_split_phase_row_done`).
- ⚠️ **This DIVERGES from §6.2 steps 3 and 4**, which prescribe a `SPEC.md` per sub-phase and a `planned` row per sub-phase. **Practice has diverged since 32.2 and this SPEC follows practice, naming the divergence rather than silently inheriting it.** Split-file precedent is mixed (44/45/47/56 carry per-leg SPECs; **60 and 61 — the two most recent — carry one shared SPEC**).

⇒ **`ROADMAP.md` stays BYTE-UNTOUCHED, the sentinel `want` stays 116, check (3) stays SILENT** (row 84's `gRPC-family row` summary is untouched, so **the family stays open and the row's headline sentinel achievement survives the split intact**), and **`ADR-0306` covers both legs** — no split ADR is minted, because no ROADMAP restructuring occurs.

**Handoff: `PLAN-84.1.md` then `PLAN-84.2.md`, each a TDD spine, in that order.**

---

## 13. WHAT THIS ROW NAMES BUT DOES NOT FIX

1. ⚠️ **The h2spec selector defect** (§1.1) — ten malformed strings, 42 unrun cases, 4 pre-existing envoy-go failures behind them, the missing `tests == 0` guard, and `ADR-0051` §2's false scope claim. **One coherent future row.**
2. ⚠️ **The CONTINUATION decode discard** (`h2/conn.go:255-259`) — live, reference-diverging, comment false in both clauses.
3. ⚠️ **The CONTINUATION encode hole** (§8.2) — `encodeAndWriteHeaders` never splits; a stock `x/net/http2.Transport` gets `frame too large`.
4. **H1→H2 = 502** — `UseH2` consulted nowhere in HCM (§2.8). Forecloses `grpc_http1_bridge` and browser `grpc_web`.
5. **Full response buffering** — structural `[]byte` at three layers; forecloses all streaming. Unary provably unaffected at interop size.
6. **`test/conformance/grpc/`** (§4) and the eight unregistered gRPC filter type URLs.
7. **The dead `RunEncodeTrailers` hook** — **deliberately NOT wired** (§14).
8. ⚠️ **`internal/filter/hcm/h2`'s client response path has ZERO fuzz coverage** — both existing h2 fuzzers are server-side. Newly named here.
9. ⚠️ **All 55 fuzzers live under `internal/`, none under `test/`** — §7.4's own location clause is 100% violated and no stage in the 77-83 window records it.
10. **The stale `STATE.md` §Project block** (fixtures 119, port 10450) and `harness_test.go:208`'s false port inventory — **recorded, not fixed** (do not source from §Project).

---

## 14. HAZARDS CARRIED INTO THE PLAN

1. ⚠️ **DO NOT WIRE `RunEncodeTrailers` — CONFIRMED ON THREE INDEPENDENT MEASUREMENTS.** (a) **Zero non-test callers**: 34 occurrence lines tree-wide, 11 non-test, exactly 1 call-shaped and it is the **definition** at `chain.go:667` (NC: `RunDecodeData` = 87 lines / 6 non-test call-shaped, **fires**). (b) `headerMapForType` — a **method** on `*abiCallbacks` (`internal/filter/http/wasm/abi_callbacks.go:204`; grep the **symbol**, not `func headerMapForType`) — switches on map types 0/2/6/7 only; **response trailers = type 3 → `default:` → `active=false`** for all **seven** consumers. **A woken guest cannot read the trailers it is woken about.** (c) Variant A leaves the seams dead, measured with a firing control in the **same** binary: `RunEncodeTrailers` **0 lines** while `RunEncodeHeaders` printed **6** and the RPC returned `SERVING`. **ADR-0273 already boot-rejects the two trailer match arms on exactly this basis; row 84 does not disturb it.**
2. ⚠️ **The break roster must be RE-DERIVED AT THE IMPL TIP, not carried from the PLAN** (`reference_break_roster_goes_stale_within_its_own_row` — phase 83's PLAN recorded three S3 arms green; the guard-alone break reddens **400/400** at the landed tip). Minimum roster: drop the capture (`client.go:440`) · drop the carrier (`router_h2.go`) · drop the emit (`writeH2Reply`) · restore `endStream = len(body)==0` **as a different statement in the same function, isolating it from the emit arm** · emit trailers with END_STREAM off · ⚠️ **a VACUITY control asserting the stats legs stay GREEN under every arm** (proves shape 31 live; `reference_positive_arm_cannot_catch_overfiring`) · ⚠️ **a SYMMETRIC control injecting the same wrong value on BOTH sides, which must PASS** · ⚠️ **a liveness arm with a FAILING baseline** (`reference_liveness_break_needs_failing_baseline`) · the un-asserted cosmetic headers, which must **NOT** fire. **`-count=1` on every arm; confirm WHICH assertion fired; restore verified by sha256, not by eye; expect 1-2 arms to be un-reddenable and NAME them rather than report them green.**
3. ⚠️ **`INNER_EXIT` is not optional.** The phase-83 IMPL's first differential launch **aborted** (`INNER_EXIT=1`, panic on `0084-otlp-access-log`, 85 PASS + 1 FAIL of 120) **while the surrounding tooling reported success**. Budget ~3 launches.
4. ⚠️ **A `golangci-lint` NC can fail to fire and read as "the gate discriminates."** A British-spelled **identifier** produced empty output/exit 0 (`misspell` does not flag CamelCase); only British spellings in **prose** fired. **And a `typecheck` failure short-circuits `misspell` entirely** — a *compiling* defect is required to exercise the style linters. `misspell` locale **US**; `.golangci.yml` is `disable-all: true` with **9** linters at v1.64.8 (`gosec`/`nolintlint`/`depguard`/`forbidigo` are **none** of them).
5. ⚠️ **`rc=$?` after a pipe returns the LAST command's status** (`reference_harness_exit_code_is_not_command_exit_code`). Use `out=$(…); rc=$?` or `PIPESTATUS`. Observed live at two agents this stage.
6. ⚠️ **`git grep` with a quoted pathspec ending in `/` returns 0 lines** where the shell-glob form returns 70 — a **fail-unsafe zero** (§2.13).
7. **Flake register:** `reference_sds_init_fetch_timeout_dial_budget_flake` (two packages) · the pre-existing `internal/cluster` `-race` outlier · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults` · the driver-receiver port race (**42**-fixture behavioural roster; phase 84 is outside the class, §2.13). **Check for sibling sessions before blaming your own row.**
8. **Baselines to beat, measured at the phase-83 IMPL, not inherited:** full differential **120/120 in 384.311 s** (⚠️ **DOC-SOURCED — competing 388.961 s / 402 s figures exist and no artifact is committed**) · `0036` alone **19.33-20.36 s ENVELOPE, not a point** · `go test ./...` **126 ok / 0 FAIL** · `-race` as a second run **125 ok / 1 FAIL, DATA RACE count 0**.

---

## 15. GATES — THE SIX-GATE POSTURE, WITH DEPARTURES NAMED RATHER THAN COMPLIANCE CLAIMED

**(a)/(b)** — **not exercised at this SPEC** (docs-only; zero `.go` changed). At **84.1** (a) is vacuous-but-honest and (b) is the full 120-fixture suite; at **84.2** (a) is `0119` and (b) is the full suite, which becomes 121 fixtures.
**(c)** — ⚠️ **`test/conformance/grpc/` NOT BUILT, deferred by name** (§4), on the §7.5 "declared threshold" reading plus the phase-61/h3spec precedent. Existing suites **ASSERTED-UNAFFECTED** (this row touches no HPACK, framer or wasm path): proxy-wasm **10 of the cpp-host's 16 families (62.5%), 6 deferred**; h2spec **53/53** ⚠️ **stated with §1.1's scope caveat — 42 cases in RFC 9113 §6 have never run, so this figure is NOT evidence about frame-level behaviour.**
**(d)** — ⚠️ **VACUOUS, and the word is "vacuous", not "green".** §7.4 binds a phase that introduces a *parser, codec, or filter*; phase 84 introduces none — the trailing-HEADERS HPACK decode **already runs** at `client.go:425` (the discard is at `:440`), the frame is emitted through the already-landed `encodeAndWriteHeaders`, and no filter is registered. Precedent: **0 of 7** phases (77-83) added one; the count has been **55** for the whole window (⚠️ `-- '*.go'`-scoped; unrestricted gives 161, whose 106-line delta is **100% Markdown code fences**).
**(e)** — not exercised at this SPEC; owed in full at each leg's IMPL.
**(f)** — ⚠️ **STANDING LINEAGE DEPARTURE, named not claimed.** No `REVIEW.md`; **37 of 124** phase dirs carry one and **none since 25.3**.

**Stat surface: +0, and the +0 is REFERENCE-BACKED rather than argued.** All four touched files register zero stats; the diff contains **0** `NewCounter`/`NewGauge`/`NewHistogram` lines; a cross-side A/B on the pinned reference moved an **identical 44-name set** with symmetric difference **∅** (§10). ⚠️ **Do NOT discharge this via `TestNoNewStat*`** (§2.6) — use **call-site enumeration**, as phases 80-83 all did: **208 code sites across 36 production files** (⚠️ cite 208/36, never 208/84 — the 84 is a file count including tests).

---

## 16. HYGIENE

Five agents in **DETACHED** worktrees with private scratch and disjoint port bands inside `44800-45299`; each reverted its probes and confirmed `git status --porcelain` = **0 lines**. Docker containers torn down **BY NAME** (`a1-h2spec-asgate`, `a3-ref-84`, `a5c-ref`, `a5c-backend`, `a5c-probe`, `a5c-net`) — never `prune`, never an `ancestor=`/image filter. `go.mod`/`go.sum` untouched, verified by snapshot-and-restore around two module probes. No commits by any agent; no pushes.

⚠️ **A stale-worktree note for the next stage:** no `phase-84-*` worktree predated this session, unlike the BRAINSTORM's. **Check `git worktree list` and `ps` before assuming a `phase-84-*` worktree is yours.**

---

## 17. NEXT

**`PLAN-84.1.md`** — the seam leg. It owes a TDD spine over the four production files plus §8.3's validation, the D-84-ENDSTREAM conditional implementation, the H1/H3 non-change regression tests, `ADR-0306`'s §Decision/§Consequences shape, and the `BEHAVIOR_CONTRACT` reconciliation plan for its five falsified statements.

⚠️ **It must open with the §6.1 arithmetic already crossed** — §12's floor is **≈1738** and §6.1 gates *"`PLAN.md` estimates"*. **Phases 80 and 81 escaped the trigger purely by writing down a number that was too low. A PLAN that re-derives a figure under 1500 is repeating that error, not refuting this SPEC.**
