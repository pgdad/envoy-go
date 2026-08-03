# BRAINSTORM 83 — wasm-pause-arm-leak

**Stage:** BRAINSTORM (lifecycle-state `DONE` -> `1`). **Base master `8a0126d2`** (from `git rev-parse master`), branch `phase-83-brainstorm`. Docs-only.

**SELF-PICKED** per the 2026-07-12 standing directive: there was no banked mid-lifecycle work at this tip (phase 82 closed 2026-08-03, row 82 `done`). Five investigation agents ran on disjoint remits in detached worktrees with private scratch and disjoint port bands inside `43000-43399`; every load-bearing claim below was re-derived by the controller, and **two agent claims did not survive that re-derivation** (§5.2, §5.3).

---

## 1. ⚠️ THE HEADLINE: THE ROW THE PREVIOUS PHASE LANDED TO STOP A LEAK STOPPED IT IN TWO OF SIX ARMS, AND THE EVIDENCE THAT THE OTHER FOUR WERE SAFE WAS FALSIFIED BY THE COMMIT THAT WROTE IT

Phase 82 landed scope item **S9** because honoring `Action::Pause` without a resume path leaks a connection and a goroutine per request. It armed the watchdog and set the resume flag in the **two headers arms**. The **four remaining arms park the goroutine and arm nothing.**

Census over all six production `abi.ProxyActionPause` arms (denominator 6, symbol-anchored, controller-re-derived):

| arm | returns | sets the paused flag / arms the watchdog |
|---|---|---|
| `decode_headers.go:269` | `StopIteration` via `beginDecodePause()` | **YES** |
| `encode_headers.go:126` | `StopIteration` via `beginEncodePause()` | **YES** |
| `body.go:226` (DecodeData) | `DataStopIterationAndBuffer` | **no** |
| `body.go:313` (EncodeData) | `DataStopIterationAndBuffer` | **no** |
| `trailers.go:142` (DecodeTrailers) | `TrailersStopIteration` | **no** |
| `trailers.go:199` (EncodeTrailers) | `TrailersStopIteration` | **no** |

All four unflagged statuses **park** — `chain.go:430` (`DataStopIterationAndBuffer` -> `parkDecode`), `:474`, `:569`, `:635`. So the consequence is not the recorded *"the resume gate will not release them"*; it is **the S9 leak verbatim, in four arms, with no watchdog to bound it.**

Proven by execution, not by reading (synthetic guest returning Pause from `proxy_on_request_body`, driven through a real `FilterChain` with `pauseWatchdog = 250ms`):

```
decodePaused after body Pause = false          (the headers arm sets true; the body arm does not)
RunDecodeData STILL PARKED 1s after a 250ms watchdog window — no watchdog is armed for the body arm
ContinueStream(decode) = 0                     (WasmResultOk — the guest cannot even detect the failure)
RunDecodeData STILL PARKED after proxy_continue_stream — the CAS gate refused: the arm never set decodePaused
released only after HAND-SETTING decodePaused  — confirms the flag is the sole gate
```

### 1.1 ⚠️ THE LIVE BLAST RADIUS IS **TWO** ARMS, NOT FOUR — AND IT SPANS **THREE** PROTOCOL PATHS

This is a controller-level synthesis across two agents that neither reported alone, and it cuts in both directions.

```
$ git grep -nP 'Run(Decode|Encode)Trailers\(' -- '*.go' | grep -v 'func ('      # CALL SITES
internal/filter/http/chain_test.go:294
internal/filter/http/envoygotest/filter_test.go:332
internal/filter/http/wasm/pause_test.go:435
internal/filter/http/wasm/pause_test.go:436
                                                    # ZERO non-test call sites

$ git grep -nP '\bchain\.Run(Decode|Encode)Data\(' -- '*.go' | grep -v '_test\.go'
internal/filter/hcm/connection.go:637   :653   :743          # H1
internal/filter/hcm/h2dispatch.go:540   :582                 # H2
internal/filter/hcm/h3dispatch.go:303   :317   :372          # H3
                                                    # EIGHT non-test call sites
```

⚠️ **The trailers hooks have ZERO non-test callers**, so `trailers.go:142/199` cannot be reached in production today. ⚠️ **The body hooks have EIGHT, across H1, H2 AND H3** — so the leak is live on every protocol path the proxy serves, which no prior document states.

**Both halves still belong in this row, and the reason is not bundling.** The trailers arms are *latent*, and the thing that would activate them is a measured, in-tree prototype: a phase-83 sibling investigation revived the response-trailer seam in **+92/−11 production lines across 5 files** and added *the first non-test `RunEncodeTrailers` caller in the repository's history* (§5.4). Whoever lands that seam inherits two leaking arms on day one unless they are flagged first. Fixing latent-but-adjacent arms in the row that fixes the live ones is cheaper and more honest than leaving a tripwire for a row that is now demonstrably close.

### 1.2 ⚠️ A SECOND PRODUCTION DEFECT, NAMED BY NO PRIOR DOCUMENT, THAT ONLY BECOMES REACHABLE ONCE THIS ROW LANDS

`beginDecodePause` assigns `f.decodePauseTimer = t` under `pauseMu` **without stopping the previous handle** (`pause.go:107-109`; mirror at `:132-134`). Today at most one arm per side can set it, so it is latent. With four arms flagged, a headers-pause-then-body-pause stream **drops the first timer**, which later fires and force-resumes a stream it no longer owns. ⚠️ **This is a defect the fix creates if the fix is naive** — it must land in the same row, not after it.

### 1.3 ⚠️ AND THE RECORDED MITIGATION IS FALSE — FALSIFIED BY THE COMMIT THAT WROTE IT

The phase-82 IMPL recorded the deferral as safe because *"ZERO guest crates call any resume hostcall, so nothing reaches that path today."*

```
$ find . -name '*.rs' -not -path './.git/*' -exec grep -nP 'resume_http_request|resume_http_response|proxy_continue_stream' {} +
./test/fixtures/0036-http-wasm-body-and-advanced/scripts/l_httpcall_success/src/lib.rs:43:        self.resume_http_request();
NEG CONTROL (files containing on_http_request_headers): 22 of 35 .rs files
$ git log --oneline -- .../l_httpcall_success/src/lib.rs
fb93845f   368bc9fb
```

⚠️ **`fb93845f` IS THE PHASE-82 IMPL COMMIT ITSELF.** The claim was false at the moment it was written, in the same commit.

⚠️ **And it is false a second way, which matters more:** `a_body_read_only`, `b_body_mutate_passthrough` and `c_body_mutate_replace` all return `Action::Pause` from `on_http_request_body` on any non-final chunk and none of them resumes. Reaching the leak does **not** require a resume hostcall — it requires only a Pause, and three vendored guests already do it.

### 1.4 ⚠️ HALF THE DERIVATION OF A LANDED CONSTANT IS VOID

`pause.go:65-70` is one of the **two measured constraints** pinning `defaultPauseWatchdog = 10 * time.Second`. It reads: *"scenario (l) … NEVER calls `resume_http_request` from its `on_http_call_response`, so with Pause honored its probe is released only by this watchdog."* Lines 40-43 of that very guest call `self.resume_http_request()`, with a comment explaining why. **The constant may still be right; its stated justification is not, and this row must re-derive it rather than inherit it.**

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

The standing directive is **smallest defensible candidate first**. All costs below were **re-derived at this tip** (`reference_deferred_candidate_cost_restale`); none is inherited from the router's prose.

**PICKED — `wasm-pause-arm-leak`.** Smallest of the candidates that fix a live defect; highest severity per line; completes a charter the immediately-prior row opened and left half-done; needs **no new guest blob, no new fixture, no new BackendKind, no port, and no toolchain** (the pinned rustup 1.94.0 is off this row's critical path — the `0036` re-run uses the already-vendored blob unchanged). Its failing-first anchor already exists in shape and was **observed RED**.

| rejected alternative | re-derived cost | why rejected |
|---|---|---|
| **`grpc-unary-interop`** (open the gRPC family) | **92 prod `.go` MEASURED** -> budget 130-200; tests **450-800**; **12-16 tasks**; +1 fixture | ⚠️ **The strongest candidate in the project and the one this row most regrets deferring** — it would retire the sentinel's LAST check-(3) failure. Rejected on the smallest-first rule at ~4x this row's cost, **not** on the inherited HARD-BLOCKED verdict, which is now **REFUTED** (§5.4). |
| **`ssl.connection_error`** (the router's carried "strongest" pick) | **700-900 net `.go`**, 11-13 tasks | Cost refuted at the floor: the strictly-*smaller* landed analogue (phase 75) measured **net +444**, so "~230 lower bound" is not a lower bound — **444 is**. Also larger in kind than recorded: it is not "add a counter", it is **split `outcomeOther` three ways** (§5.5). |
| **xDS config-seam strict-reject sweep** | ~40-60 prod + 150-250 test | Genuinely cheap and fixture-free (**14 silently-ignored fields** enumerated by proto reflection), but it lands *rejects*, not behavior; the one real divergence in it (`Node` on the ACK) is a two-line fix that does not need a row. |
| **upstream SDS (server-cert)** | moderate; **un-prototyped boot ordering inversion** | The only xDS item whose sentence text is realistically consumable, but `main.go:105` builds the cluster manager **before** `boot.NewSDSProvider` at `:156`, and upstream SDS inverts that dependency. The investigating agent flagged it as possibly "the whole row" and did **not** prototype it. Too much variance for a smallest-first pick. |
| **`validate` nil-`sdsProvider` bug** | ~3-5 tasks | ⚠️ **Real, cheap, and a genuine user-facing divergence** (§5.6) — `--mode validate` rejects a config the reference validates OK *and* that envoy-go's own boot path accepts. Rejected only because it is a bug fix inside a landed package with no differential surface; it is **recorded as the strongest sub-row-sized candidate** and should be swept into the next Operational-tooling row. |
| **Runtime disk layer** | ~10-13 tasks, +1 fixture (3 gates) | Larger, and its deferred item names an `override_subdirectory` field that **does not exist** in the pinned dep (§5.7), so the item is partly unconsumable by construction. |
| **`runtime.num_layers` LOADED-vs-DECLARED divergence** | small | ⚠️ Real (§5.8), but it is only *observable* through a layer that can fail to load — i.e. it needs the disk-layer row to have a gate. Recorded, not chartered. |
| **wasm http-call trap arm is SILENT** | 5 / 60-90 / 20-30 prod `.go` for three different shapes | ⚠️ **Not an implementation row at all** — it is a convention decision (`internal/wasm` has **0** `log.Printf` and **0** `"log"` imports across 29 non-test files, NC firing on the sibling package). Every attributable shape either creates the logging surface or duplicates the ambiguity, and **none has a clean failing-first anchor**. Routed to a SPEC open question, not a row. |
| **hardcoded test port `42552`** | **net −14 lines**, 1 file | Too small to charter alone. **Folded into this row on a causal premise, not a directory premise** — see §3. |

### 2.1 What this row does NOT claim

It **narrows nothing**. The WASM family carries no candidates sentence among the sentinel's five, so check (2) cannot move, and it is **stated, not forecast** (`reference_sentinel_deferred_sentence_live_vs_historical`). Check (3)'s `WASM` slug was retired by row 82; this row does not move check (3) either. **This row moves no sentinel check, and says so.**

---

## 3. THE ONE FOLD-IN, AND WHY IT IS A PREMISE RATHER THAN AN EXCUSE

`internal/filter/http/wasm/http_call_response_cache_test.go:138` hardcodes `const httpCallBackendPort = 42552`. It is the **only** hardcoded bound port in the entire Go test tree (denominator established with a firing NC; the two other 5-digit literals in the package are fabricated `net.TCPAddr` values that are never bound; 33 test files already use the `127.0.0.1:0` idiom).

It is folded in because **this row's own new tests are the ones it would flake.** S1/S3 add parallel-safe pause tests to exactly that package; a sibling session holding 42552 fails the package before those tests run. The link is causal, not topological.

⚠️ **HALF THE RECORDED JUSTIFICATION FOR IT IS FALSE, AND THE ROW SAYS SO.** The claim that it collides "across back-to-back runs of the same suite through `TIME-WAIT`" does not reproduce:

```
--- run 1 --- ok 0.033s   sockets on 42552: 1
--- run 2 --- ok 0.033s   sockets on 42552: 2
--- run 3 --- ok 0.032s   sockets on 42552: 3
$ ss -tan | awk '/42552/{print $1}' | sort | uniq -c
      3 TIME-WAIT
```

Three consecutive `-count=1` runs **all passed** while TIME-WAIT entries accumulated — Go's `net.Listen` sets `SO_REUSEADDR`, so TIME-WAIT on a local port never blocks a listening bind. **Only a live listener collides**, which is the parallel-session mode. The candidate is real; its stated mechanism was not.

⚠️ **Do NOT copy `freeTCPPort`.** All three live definitions (`cmd/envoy-go/main_test.go:180`, `test/conformance/h2spec/h2spec_test.go:219`, `test/differential/harness_test.go:246`+`:292`) are pick-close-rebind, which exists only because a **subprocess** performs the bind, and the differential one carries a documented residual race window. This test binds **in-process**, so `net.Listen("tcp","127.0.0.1:0")` + `ln.Addr().(*net.TCPAddr).Port` has **no window at all**. Measured fix: **9 insertions / 23 deletions, net −14 lines, zero production `.go`**, verified green with a squatter still holding 42552.

---

## 4. SCOPE

- **S1 — flag and arm the two LIVE body arms** (`body.go:226`, `body.go:313`). The leak is live on H1, H2 and H3.
- **S2 — flag and arm the two LATENT trailers arms** (`trailers.go:142`, `trailers.go:199`), so the response-trailer seam revival cannot inherit the leak.
- **S3 — stop the previous timer before reassigning** in `beginDecodePause`/`beginEncodePause` (`pause.go:107-109`, `:132-134`). Latent today; **created by S1/S2 if omitted.**
- **S4 — rebuild `TestFilter_Pause_CensusOfHonoredArms` (`pause_test.go:130`), which is VACUOUS** (§5.1) — it is the named guard against exactly the edit this row makes.
- **S5 — rewrite the false comment surface**: `pause.go`'s file header (`:8-29`), the watchdog upper-bound rationale (`:65-70`, half-void per §1.4), and `decode_headers.go:256`'s *"FOUR of the six … arms"* sentence.
- **S6 — re-derive `defaultPauseWatchdog`** now that one of its two stated constraints is falsified. The constant may survive; the derivation must be rebuilt.
- **S7 — de-hardcode the test port** (§3).

### 4.1 Open questions for the SPEC

1. ⚠️ **Does arming a 10 s watchdog on the `0036` (n) body-cap arm change that scenario's timing?** `n_body_cap_exceeded` returns `Action::Pause` unconditionally from `on_http_request_body`, and today the cap branch (`body.go:143`) fires *before* dispatch on an oversized chunk. A scenario whose whole point is indefinite accumulation now gains a bounded release. **This must be MEASURED, not reasoned** — it is the row's principal risk.
2. Do the two trailers arms need the flag, or a documented no-op, given the hooks have no production caller? (§1.1 argues flag; the SPEC should test the alternative.)
3. Should S6 keep 10 s, and on what surviving evidence?
4. Where does the trap-arm logging-surface convention question get answered — an ADR amendment, or a §Context paragraph on this row's ADR? (It is **not** in this row's scope; it needs a home.)

### 4.2 Anticipated surface

**Production:** `internal/filter/http/wasm/{body.go,trailers.go,pause.go}` (3 files). **Test:** `pause_test.go`, `http_call_response_cache_test.go`, plus new arms built from the existing Go `fixBuildModule` vocabulary in `wasm_fixtures_test.go` — `proxy_on_request_body` has the same `(i32,i32,i32)->i32` signature as `proxy_on_request_headers`, so **no new Rust blob and no toolchain dependency.** **Stat surface: anticipated +0.** **New fixtures: 0.** **go.mod: +0.**

### 4.3 Cost and split posture

**Band 350-600 net `.go`, budget ~450, 6-8 tasks.** Basis: a measured ~15-25 production lines for the four call sites, plus S3/S4/S5/S6 and ~250-300 test lines, plus a measured net −14 for S7.

⚠️ **THE BAND IS A LOWER BOUND AND IS DECLARED AS ONE.** `reference_measured_prototype_is_a_lower_bound` has now fired on **two consecutive rows** — phase 81 at **3.07x** and phase 82 at **2.55x** — with **test scaffolding dominating both times**, and this row's own estimate is again production-measured with a modelled test side. At budget **neither §6.1 trigger fires** (`:289` ~25 tasks, `:290` ~1500 LoC). At a 3.07x repeat the LoC trigger **would** cross. **The PLAN owes an explicit §6.1 re-evaluation against a measured test side, and the precedent if it crosses is RECORD, DO NOT RETRO-SPLIT** (phases 81 and 82).

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE FOUND BY EXECUTION

**Load-bearing (8):**

1. **§1.3 — "ZERO guest crates call any resume hostcall" is FALSE**, and the call was added by the phase-82 IMPL commit itself. The stated safety argument for deferring this work never held.
2. **§1.3 — reaching the leak does not require a resume hostcall at all.** Three vendored guests Pause from `on_http_request_body` and never resume.
3. **§1.1 — the trailers hooks have ZERO non-test call sites and the body hooks have EIGHT, spanning H1/H2/H3.** The live blast radius is two arms on three protocol paths; no prior document states either half.
4. **§1.4 — half the derivation of `defaultPauseWatchdog` is void.**
5. **§1.2 — the timer-handle overwrite is a second production defect that this row's own fix creates if written naively.** Named by no prior document.
6. **§5.1 — `TestFilter_Pause_CensusOfHonoredArms` is VACUOUS while its docstring asserts the opposite.** Its comment claims *"This is a behavioral assertion, not a grep: it drives the real dispatch and reads the returned status."* Its body compares three pairs of **package constants** (`DataStopIterationAndBuffer == DataContinue`, …). It constructs no filter, no chain and no guest, and dispatches nothing. **It is the named guard "that keeps a future edit from silently changing the frozen body/trailers dispositions" and it would not catch any such edit.** ⚠️ **This is BROKEN-GATE SHAPE 25: a gate whose own docstring claims it is behavioral when it is a tautology over constants.**
7. **§5.4 — the gRPC family's HARD-BLOCKED verdict is REFUTED BY A WORKING PROTOTYPE.** The inherited verdict rested on a grep (*"`RunEncodeTrailers` has zero non-test callers"*) that is true but was never converted into a cost. Measured: **+92/−11 across 5 production files**, `go vet ./...` clean, 3 packages green, and a real `grpc-go` client through envoy-go to a real health server goes from `Internal: server closed the stream without sending trailers` to **`SERVING`**. "16-22+ tasks for the seam" is refuted; **12-16 for a whole `grpc-unary-interop` row** is the re-derived figure.
8. **§3 — the `TIME-WAIT` half of the port claim is FALSE** (three passing runs with TIME-WAIT accumulating; `SO_REUSEADDR`).

**Also refuted (13):**

- **`*RootVM.dispatchHttpCallResponse` DOES NOT EXIST** — cited in **three landed code comments** (`abi_callbacks.go:1101`, `stream_context.go:249`, `:254`) plus two phase-25.2 documents. The symbol is `(*RootVM).handleHttpCallResponse`. **Anchor phase-83 documents on the real name.**
- **A FOURTH false non-test comment about the trailers seam**, beyond the two already recorded: `internal/filter/http/router/router.go:285` states *"Called by HCM dispatch AFTER chain.RunDecodeHeaders + RunDecodeData + RunDecodeTrailers complete"* — `RunDecodeTrailers` is never called. (`connection.go:300` carries a softer fifth.)
- **gRPC *error* RPCs already pass through unpatched** — `grpc-status` rides the first HEADERS block; cross-side identical. The seam breaks **successful** RPCs only. Recorded nowhere.
- **Request trailers are not on the gRPC path at all** (clients half-close with END_STREAM on DATA), so *"the entire trailers pair is unreachable"* over-scopes the work by half.
- **The gRPC filter type-URL denominator is EIGHT, not seven** — `connect_grpc_bridge` appears in no record and nowhere in the tree. And none of the eight has a parse-reject arm: the failure is a **protojson type-resolution** failure, so **no seam has been pre-paid**.
- **Two blockers larger than trailers, neither previously named:** an H2-upstream cluster from an H1 downstream listener returns a **measured 502** (kills `grpc_http1_bridge` and browser-path `grpc_web`), and the proxy **fully buffers the response body** (measured: server-streaming first `Recv` at **5 s / DeadlineExceeded** on the subject vs **0 s / nil** on both the control and the reference), which forecloses all gRPC streaming and full interop conformance.
- **`ssl.curves` is charset-BLOCKED, not passing** — the reference emits `ssl.curves.P-256`; `IsValidName` is false and `NewCounter` **panics**. The recorded four-way disposition is wrong on this arm.
- **`ssl.sigalgs` IS emitted** — `ssl.sigalgs.rsa_pss_rsae_sha256: 2` under mTLS. *"Never observed emitted"* was probe coverage (no client cert), not a property. It is nonetheless blocked, for a different reason: the peer signature algorithm is **unexported** in `crypto/tls` (`testingOnlyPeerSignatureAlgorithm`) — a **Go framework gap, not a naming gap**.
- **All four dynamic `ssl` arms diverge cross-side, not just `ciphers`** — on `/stats/prometheus` the reference carries the value in a **label** (`envoy_listener_ssl_curves{envoy_ssl_curve="P-256"}`), where hyphens and dots are unconstrained. A name-carrier design ships the wrong Prometheus name on **every** arm and **collides** `TLSv1.2` with `TLSv1_2`. An `ExtractTags` value-hoist rule fixes it and **one already exists in-tree** (`name.go:144`, the `sds.` hoist) — so "IMPOSSIBLE" is too strong for `ciphers`/`curves` and too weak for the arms called passing.
- **`ssl.curves` additionally needs Go >= 1.25** (`ConnectionState.CurveID`, `api/go1.25.txt`); CI pins `go-version: '1.23'` at three sites. An undeclared prerequisite.
- **`initial_fetch_timeout` is ALREADY FULLY LANDED**, including the `0 = unbounded` edge (`config.go:74`, `provider.go:57`/`:100`). Treating it as open work is stale.
- **The Runtime deferred item's `override_dir` names a field that does not exist** — `RuntimeLayer_DiskLayer` in the pinned dep has exactly `SymlinkRoot / Subdirectory / AppendServiceCluster`; `override_subdirectory` live-rejects (`no such field`, exit 1) and exists only on the **deprecated** top-level `Runtime` message, which the pinned `Bootstrap` does not carry.
- **`ROADMAP.md`'s Operational-tooling cell cites `github.com/esalaine/envoy-go/validate`**; the module is `github.com/pgdad/envoy-go` (`go.mod:1`). Wrong-on-arrival for anyone re-deriving from that cell.

### 5.6 A cheap live divergence found in passing, recorded for its owner

`--mode validate` **rejects a config the reference validates OK and that envoy-go's own boot path accepts.** Driven on fixture `0108`'s own rendered configs, not hand-invented input (`reference_probe_input_is_a_claim`):

```
$ ./envoy-go --mode validate -c sds0108.yaml
  tls: downstream: SDS-bound validation_context_sds_secret_config is not supported in phase 03   exit=1
$ docker run … contrib-v1.37.2 -c ref_sds.yaml --mode validate
  configuration '/etc/envoy/envoy.yaml' OK                                                        exit=0   (SDS server UNREACHABLE)
```

Cause: `validate/validate.go:49` threads a literal `nil` `sdsProvider` into `boot.Construct`, and `internal/tls/config.go:436` rejects on `provider == nil`. Both controls fired (removing the SDS block yields a *different* error; a static `trusted_ca` yields `nil`; the reference's own NC errors on a missing cert file, so the OK is not vacuous). **~3-5 tasks. It consumes no named deferred item. It belongs to the next Operational-tooling row.**

### 5.7 / 5.8 — see the "also refuted" entries above for the `override_subdirectory` and `num_layers` findings

⚠️ **`runtime.num_layers` counts SUCCESSFULLY LOADED layers, not declared ones.** Two declared layers (static + disk with a nonexistent root) give `runtime.num_layers: 1` with `runtime.load_error: 0`. Phase 77's *"`num_layers == len(layers)` held in every arm"* is true **and non-discriminating** — all eleven of its arms were static-only, and a static layer always loads, so the axis was held fixed (`reference_independent_probes_can_share_a_blind_axis`).

### 5.2 / 5.3 — two agent claims the controller did NOT accept

- **"Four arms leak" is right about the code and wrong about the blast radius.** The controller's own call-site enumeration (§1.1) shows two of the four are unreachable in production. The agent's framing would have over-stated the severity of the row.
- **A blob-level `strings` census of `proxy_continue_stream` is NON-DISCRIMINATING** — 33 of 36 `.wasm` blobs contain the string, including `e_log_only.wasm`, because the Rust SDK emits the whole hostcall extern block regardless of use. **Only the source-level grep discriminates** (`reference_probe_must_discriminate`). Reported by the agent against itself; adopted.

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE. IT DOES **NOT** FIRE

Input measured **before** anything was written: `ROADMAP.md` **230 lines / 114 data rows**, so an empty result could not read as a zero result (`reference_empty_output_is_not_a_zero_result`).

- **(1) SILENT** at `want=114`, denominator printed (`examined 114 data rows`). ⚠️ **Silence is otherwise indistinguishable from a broken check, so THREE negative controls were run and ALL THREE FIRED:** row 62 doctored on a scratch copy ⇒ `NOT DONE: row 62` (with `NC LANDED? [ in-progress ]` inspected first); **row 82 doctored back ⇒ `NOT DONE: row 82`**; `want=113` ⇒ `GATE FAIL: examined 114 data rows, expected 113`.
- **(2) FIVE — `:192 :202 :212 :218 :226`, UNCHANGED.** The **thirty-sixth** consecutive phase without a decrease. **This row narrows nothing, stated and not forecast.**
- **(3) `NEVER OPENED: gRPC` — ALONE.** NC: an invented slug fires; `WASM` and `HTTP-filters` correctly do not.
- `ls stop` ⇒ `No such file or directory`. **`stop` was NOT created and MUST NOT be** — the condition is a CONJUNCTION and checks (2) and (3) are both live.

**Leak check — armed, and this row writes a ROADMAP row.** Row 83's summary contains **neither** of check (2)'s fail-strings, and check (3)'s literal `WASM-family row` is preserved: it occurred exactly once at line 144 (row 82, field 7) at base and **row 82's field 7 is byte-untouched by this stage**. ⚠️ **`want` goes 114 -> 115 in the SAME commit that adds the row.**

⚠️ **THE CITE-SHIFT ENUMERATION THIS ROW OWES** (measured before the edit, per the phase-82 precedent): a row appended after line 144 shifts **exactly 38 of the 117** `ROADMAP.md:<line>` cites; **79 are safe**; NC at insertion-point-1 gives 117. ⚠️ **All 38 sit at >= 184 — which means the five check-(2) anchors `:192 :202 :212 :218 :226` become `:193 :203 :213 :219 :227`.** Every document citing the old anchors goes stale the moment this row lands, so the renumber is part of this stage rather than a discovery for the next one.

---

## 7. COUNTS RE-DERIVED AT THIS TIP

Re-run mechanically; never copied. ⚠️ **Use GNU `command grep` or `git grep` repo-wide, and NOT `command grep` inside `xargs`.**

fixtures **120** (next-free **0119**; ⚠️ a naive `^[0-9]{4}-` gives **118**) / fuzzers **55** (⚠️ `-- '*.go'`-scoped) / internal packages **73** (⚠️ `ls -d internal/*/` gives **29** — that is top-level directories, not packages) / phase dirs **123** / `DECISIONS.md` **17858**, **303** `^## ADR-` headings, tail **ADR-0304 COMPLETE**, next-free **ADR-0305**, `^---$` **216**, STATUS census **17**, recurrence guard `^> \*\*STATUS: PROPOSED` at **0** / `ROADMAP.md` **230 lines / 114 data rows** / `BEHAVIOR_CONTRACT.md` **5900**, max cited line **5078** / `STATE.md` **63** / `STATE_HISTORY.md` **446** / `ROADMAP.md:<line>` cites **117**.

**Three corrections to figures the router carried forward:**

1. ⚠️ **`BEHAVIOR_CONTRACT.md:<line>` cites are 196, not 195.** Bisected: **195** at `61f4f5a3` (the phase-82 SPEC tip), **196** from `71fc86d7` (the PLAN) onward. The router carried a two-commit-stale figure.
2. ⚠️ **The contested "next-free REFERENCE port" DISSOLVES rather than resolves.** Measured occupancy is **39 distinct ports** in family bands — `10000-10001`, `10012-10013`, `10131-10140`, `10443-10447`, then `12345 / 15000-15011 / 15104 / 18000-18002 / 18007 / 19000 / 19999`. The 104xx band tops out at **10447**. **Neither `10119` nor `10450` is bound**, so both router figures are "free" and the choice is band-driven. There is no single next-free port to contest (`reference_differential_fixture_port_convention`).
3. **Row 82's field 7 is 1998 UNTRIMMED / 1996 TRIMMED.** The router's 1998 is the untrimmed convention — recorded so a successor does not "correct" it wrongly.

**Row well-formedness reproduced exactly** (the gate must be a DISJUNCTION): ARM-A (escape-aware, `pieces != 8`) flags lines **119 and 131** only; ARM-B (escape-aware trailing piece) flags **140** only; naive `NF==8` flags **SEVENTEEN** — wrong in both directions. **This row's own new row was checked against both arms and is clean.**

---

## 8. SIX-GATE (§7.5, `BOOTSTRAP_PROMPT.md` AT THE REPO ROOT — `:357`, `:360-365`, `:367` re-verified exact)

⚠️ **A DOCS-ONLY BRAINSTORM OWES (a)-(f) AS A POSTURE STATEMENT, NOT AS A GREEN.**

(a)/(b) **no differential fixture changed and no `.go` committed** — inherited from the phase-82 IMPL's 120/120, not re-run and not claimed. (c) proxy-wasm **INHERITED, not run** — ⚠️ and when a stage does run it, the denominator is **10 of the cpp-host's 16 files (62.5%), 6 deferred**. (d) **VACUOUS** — this stage adds no fuzzer (55, `-- '*.go'`-scoped; unrestricted gives 161, of which 106 are Markdown code blocks). (e) `go.mod`/`go.sum` byte-untouched; no `.go` touched. (f) `REVIEW.md` **ABSENT — the STANDING LINEAGE DEPARTURE**, named rather than papered over: **86 of 123 phase dirs carry none**, 37 carry one, and there has been none since 25.3.

§6.1 (`:285`, triggers at `:289` and `:290`, `:291` BLANK, third trigger `:292`) — **neither trigger fires at budget**; see §4.3 for the declared lower-bound posture.

---

## 9. HYGIENE

Five investigation agents, each in its **own detached worktree** off `8a0126d2` with **private scratch** and a **private port band** inside `43000-43399` (clear of `20000-31007`, `11000-14999`, `10000-10447`, `15000-15011`, `18001-18007` and `42552`). All five reported `git status --porcelain` = **0 lines** and removed their own worktrees; every probe file was reverted and every docker container was torn down **BY NAME** (`p83b-a1-*` … `p83b-a5-*`), never by an image or ancestor filter (`reference_parallel_agents_shared_machine_namespaces`). **Nothing was committed by any agent and nothing was pushed.** ⚠️ One agent recorded a trap worth carrying: `kill` on a `go run` parent leaves the compiled child holding the socket, which produced one false failing measurement before it was caught.

A5's measured trailers prototype is preserved at `scratchpad/a5/trailers-prototype.diff` — ⚠️ **scratch is session-local and will not survive; the next session must re-derive it, and the figure `+92/−11 across 5 files` is a LOWER BOUND with zero tests shipped.**

---

## 10. NEXT

**SPEC** for phase 83, per the one-stage-per-session discipline. It drafts ADR-0305 §Context (⚠️ **no ADR is added at the BRAINSTORM**; §Decision + §Consequences are appended at the IMPL, ADR-0044-**as-used**) and must answer §4.1's four open questions — **question 1 (the `0036` (n) timing change) by MEASUREMENT, not by reasoning.**
