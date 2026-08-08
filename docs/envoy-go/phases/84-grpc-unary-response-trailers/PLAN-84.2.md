# PLAN 84.2 — `grpc-unary-response-trailers`, **the differential leg**: ⚠️ **THE CHARTERED THREE-ARM FIXTURE IS BLIND TO THE ROW'S HIGHEST-LEVERAGE DECISION** — every gRPC response carries trailers, so variant B (unconditional END_STREAM) is invisible to all three mandated gRPC arms AND to all three existing downstream-ALPN-h2 fixtures, measured by injection; **the fixture MUST carry a FOURTH, trailer-less plain-GET arm**, and with it variant B reddens deterministically. **§6.1 INVERTS for this leg — budget ≈1050–1200 against ~1500, the FIRST non-crossing leg in five rows.** The SPEC's YAML-mirror analogue claim is REFUTED on `0004` (it ships both mirrors); **zero canonicalization iteration was needed** — the two mandated name-drops absorb every cross-side divergence including wire order, confirming the sort is vacuous; and the prototype fixture went **GREEN on its first run against the landed 84.1 seam, 3/3 deterministic**.

**Stage:** PLAN (lifecycle-state 2 -> 3 for the 84.2 leg). **Date:** 2026-08-08.
**Base master:** `546b453d2557ff1a45d5b10d0ad7d160aa024245` (from `git rev-parse master`, **not from a SHA quoted in any document**), branch `phase-84-plan-84.2`.
**The SECOND and FINAL leg of the confirmed 84.1 / 84.2 split** (SPEC §12.3). ⚠️ **The 84.2 IMPL — the session after this one — flips ROADMAP row 84 `done`** (ADR-0106-as-used, `reference_roadmap_split_phase_row_done`), **after which check (2) — six family backlogs — is the SOLE thing standing between this project and the termination sentinel.**

⚠️ **ROW 84 STAYS `in-progress` AT THIS STAGE. `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` ARE BYTE-UNTOUCHED AT THIS STAGE. The sentinel `want` STAYS 116.** ⚠️ **84.2 consumes NO new ADR** — `ADR-0306` (COMPLETE, tail block `:17928-:17990` = EOF) already binds this leg by name: §Context ¶8 (`:17948`) *"84.2 is the differential fixture `0119-grpc-unary-trailers`, and its IMPL flips row 84 `done`"*; §Consequences (iii) (`:17982`) the `in-progress`-until-84.2 rule and `want` = 116; §Consequences (v) (`:17986`) *"the gate must assert the RPC's own status through the Drive hooks and `CompareBytes`"*. Next-free is **ADR-0307, derived from the TAIL** (headings+1 collides at the ADR-0209 gap).

---

## What was EXECUTED at this stage

**Three investigation agents on disjoint remits** — A1 (harness mechanics, read-only), A2 (**a full working prototype of fixture `0119`, built and RUN at this tip in a DETACHED worktree**, with six break/control arms injected against production and the driver, then everything reverted and sha256-verified), A3 (cost re-derivation from landed buckets + doc invariants). A2 had exclusive use of docker and the differential suite; private port band `46000-46099`; **final state `git status --porcelain` = 0 lines, with `h2dispatch.go`, `router_h2.go` and `runner_test.go` each sha256-matching their `546b453d` blobs**. No commits by any agent; no pushes. Every load-bearing claim below was controller-re-derived or is cited to the agent's recorded execution output.

Docs-only landing: **ZERO production `.go`, ZERO test `.go`** in this stage's commit. The prototype driver is **not** committed (the 84.1-PLAN precedent: probes revert; the IMPL re-derives at its own tip per `reference_break_roster_goes_stale_within_its_own_row`). Its full measured content is specified in §4/§5 below.

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

**Every stage's job is to refute its predecessor by execution, not by review.** Lineage: phase-82 B26/S23/P14/I17 · phase-83 B21/S23/P19/I37 · phase-84 B22/S26/P31/**I-84.1 20 tasks** · this stage's ledger below.

### 1.1 ⚠️ HEADLINE — **THE THREE CHARTERED ARMS CANNOT SEE D-84-ENDSTREAM. THE FIXTURE NEEDS A FOURTH, TRAILER-LESS ARM, AND THE MEASUREMENT IS BY INJECTION**

The router and SPEC hand 84.2 the claim *"the differential is the ONLY layer that sees an unconditional-END_STREAM regression — the conditional-vs-unconditional choice at the differential layer is 84.2's to prove."* **Half survives. The chartered shape of the proof does not.**

Variant B (`hasTrailers := true; endStream := false` at the D-84-ENDSTREAM site, `h2dispatch.go:751`) was injected against the landed seam and run:

| target | result under variant B |
|---|---|
| `0119` prototype, the **three chartered gRPC arms** (success / notfound / unimplemented) | ⚠️ **GREEN** — every gRPC response carries trailers, so "always emit a trailing block" is behaviour-identical on every gRPC path |
| `0004-h2-routing`, `0079-h2-multiplex-pool`, `0080-h2-goaway-rotation` — **all three existing downstream-ALPN-h2 fixtures** | ⚠️ **GREEN** — they drive through Go's `x/net` client, which silently tolerates an empty trailing block |
| `0119` prototype **with a 4th, plain (non-gRPC, trailer-less) `GET /plain` arm** through the same TLS+ALPN-h2 listener | **RED, deterministic** — `first divergence at offset 940`: ref `DATA end_stream=true len=16`, subj `DATA end_stream=false len=16` + spurious `TRAILERS end_stream=true []` |

⇒ **Without the 4th arm, the entire differential surface — 121 fixtures — passes variant B, and D-84-ENDSTREAM stays exactly as unmeasured as the 84.1 PLAN found the unit surface to be.** The 4-cell unit matrix (84.1 Task 7) gates the *subject's* frame shape; only this arm gates it **cross-side**. **The PLAN mandates FOUR arms.** The `GRPCHealthResponder` backend's mux serves exactly this: a non-gRPC request gets a plain 200 `backend-<idx>:<path>` body (`runner_test.go:3105-3120`), so the arm costs no new backend.

### 1.2 ⚠️ SECOND — **§6.1 INVERTS FOR THIS LEG: THE TRIGGER DOES NOT CROSS, AND WRITING THAT DOWN HONESTLY REQUIRES NAMING THE OVERRUN HISTORY IN THE SAME BREATH**

Priced from landed buckets at this tip (A3, every anchor re-verified):

| bucket | floor | **budget** | ceiling | anchored on |
|---|---|---|---|---|
| fixture `.go` (driver + driver_test) | 764 | **930–977** | 1141 | the four analogues **0080=764 / 0068=766 / 0004=792 / 0079=817, all four VERIFIED exact**; budget = the 0117/p90 band (930/977); ceiling = the corpus max (`0110`, 1141, never exceeded in 120 fixtures). **Prototype measured: 529 lines, 4 arms — a LOWER BOUND** (no driver_test, no expectations wiring, minimal AssertStats) |
| PKI (3 PEMs, copied) | 26 | **26** | 60 | `0079`'s files, sha256-identical to `0004`/`0080`'s; plumbing ≈42 lines is *inside* the driver bucket (0079 `:201-234` + `:400-424`) |
| harness registration | 1 | **1** | 30 | **+1 line (the blank import) in BOTH measured fixture-adding IMPLs** (phases 73 and 77) |
| **NET `.go`, 84.2 ONLY** | **≈ 790** | **≈ 1050–1200** | **≈ 1370–1400** | ⚠️ the budget deliberately sits ABOVE the bucket sum (957–1004): the delta is the enumerated-but-unpriced driver-side work the corpus shows (driver_test at the 81.5 median, the AssertStats leg, expectations wiring, break-gate scaffolding) — priced as headroom ≈ floor × the 84.1-realized 1.50x, NOT invented |
| *(non-`.go`, recorded not gated)* | README (median **136.5**, n=118) · `expectations.yaml` (median **96**, n=96) · `ROADMAP.md` **1 line** · `STATE.md` ±10 · `STATE_HISTORY.md` +2 · `PROGRESS.md` · `next-prompt.txt` | | | |

> **⇒ THE ~1500 NET-`.go` TRIGGER DOES NOT CROSS — not at the floor, not at the budget, not at the corpus-anchored ceiling.** It crosses only if the lineage's 1.81–3.07x overrun multiplier is stacked on the budget — and the 84.1 PLAN's rule stands: **a budget built from landed buckets must NOT have the overrun multiplier stacked on top of it.**
> **⇒ THE ~25-TASK TRIGGER DOES NOT CROSS — TWELVE tasks** (§4), ≥2x margin; fixture-adding precedent is 9 (phases 72/73) and 12 (phase 77).
> ⚠️ **⇒ THE MID-EXECUTION TRIGGER IS AT RISK on Task 4 (the first-of-kind raw-h2 driver)** — the shape that fired on phase 80's T4 (6 enumerated, 17 executed). Named, not waved away.

**The honesty clause:** the last five rows landed 1.5–3.07x above their budgets, and **84.1 itself just realized net +3414 `.go` (or +3124 excluding the committed probe) against its ≈2280 budget — 1.50x/1.37x, the SIXTH consecutive firing of `reference_measured_prototype_is_a_lower_bound`** (both denominators stated; the ceiling verdict flips on whether the probe counts, and the PLAN adjudicates neither). Fixture-adding IMPLs specifically ran **~2.0x** (phase 77: ~1406 realized vs ~700 budgeted). **If 84.2 lands at 2x its budget it is still under the trigger.** That headroom — not a smaller number — is what makes this the first defensibly non-crossing leg in five rows.

### 1.3 ⚠️ THIRD — **ZERO CANONICALIZATION ITERATION WAS NEEDED, AND THE MEASUREMENT SETTLES THREE SPEC QUESTIONS AT ONCE**

The prototype went **GREEN on its first run** against the landed seam — no divergence-chasing loop. The complete canonicalization rule list, measured rather than designed:

1. **Drop `x-envoy-upstream-service-time` by name** (SPEC-mandated) — load-bearing: the reference emits `x-envoy-upstream-service-time: 0`; **the subject NEVER emits it** (framework divergence, already the SPEC's "only real cosmetic item").
2. **Drop `date` by name** (SPEC-mandated) — load-bearing: per-second values.
3. Scrub the dial addr out of `READ-ERR` text — **defensive only; never fired.**

⚠️ **Nothing else.** The response-header wire ORDER divergence (ref `…service-time, date, server` vs subj `…server, date`) is **exactly absorbed by the two drops** — the two differently-placed names are the two dropped names. **This confirms SPEC §5's finding that the sort is vacuous, and the PLAN rules: NO SORT.** Both sides identically forward the backend's `trailer: Grpc-Status/Grpc-Message/Grpc-Status-Details-Bin` announce headers and `server: envoy`. And the trailer blocks are **byte-exact cross-side including wire order** (`grpc-message` BEFORE `grpc-status`, grpc-go's order, preserved by both proxies) — re-confirming SPEC §2.3's trailers-verbatim rule with the landed seam in the loop.

### 1.4 ⚠️ FOURTH — **THE SPEC'S YAML-MIRROR ANALOGUE CLAIM IS REFUTED ON `0004`, AND THE DECISION SURVIVES ON A CORRECTED GROUND**

SPEC §10: *"`envoy.yaml`/`envoy-go.yaml` documentary mirrors are shipped by 74/75 of 120 — but NOT by `0004`, `0079`, `0080` or `0068`, the four closest analogues."* **Measured: `0004-h2-routing` SHIPS BOTH mirrors** (plus `doc.go`, `backends/`, a PKI generator). The census at this tip is **76 / 76 of 120**. **The decision — `0119` ships no YAML mirrors — STANDS**, on the corrected ground: 3 of 4 analogues (0068/0079/0080, the Go-built-bootstrap set) ship none, and the mirrors would be a third copy of config that exists only as Go template strings. **The PLAN must not silently re-inherit the false census.**

### 1.5 ⚠️ FIFTH — **HARNESS FACTS RE-DERIVED, THREE OF THEM CORRECTING STANDING PROSE**

- **`discoverFixtures` is a hand-rolled predicate, not a regex** (`runner_test.go:1461-1498`: `len>=5 && isNumeric(name[:4])` then `-` or lower-letter+`-`). The shell-side `^[0-9]{4}[a-z]?-` remains the faithful *grep equivalent* (both admit `0007a-`); cite it as such, not as the code.
- **The registration silent-green is CONFIRMED BY EXECUTION at this tip** (it was semantics-only until now): with `0119`'s blank import removed, the scoped run prints `no driver registered for fixture "0119-grpc-unary-trailers" (driver package not yet blank-imported in runner_test.go)` + `--- SKIP` + **exit 0, in 0.81 s**.
- **Two reference-port conventions coexist** and the PLAN names both: the newer `10<fixture index>` band (0103+; `0118/driver/driver.go:29-31` states it) and the older sequential 19xxx band (`0068`=19157, `0079`=19168). `0119` follows the stated convention: **10119, confirmed free** (`/usr/bin/grep -rn "10119" test/` = **0**; NC `10118` = **5**, fires).
- **Drive hooks receive only `ctx` + one `addr` string and return `([]byte, error)`** (`fixture.go:42/:46`); the runner byte-compares at `runner_test.go:1283` and renders a 32-byte hex window (`diff.go:48-66`). Backend addresses reach a driver ONLY via the `backendPorts []int` argument of `ReferenceBootstrap`/`SubjectConfig` (reference dials `host.docker.internal:<port>` via ExtraHosts host-gateway, subject `127.0.0.1:<port>`); drivers stash what they need (0079's `stashBackendPort` pattern).
- **No fixture driver imports `x/net/http2` today** — the only Framer use is harness-side (`runner_test.go:3198-3342`, whose landed comment `:3311` explicitly permits driver-side Framer use in test code). `helpers.H2RoundTrip` (`test/helpers/h2.go:33`) exposes neither trailers nor frame boundaries — **first-of-kind is confirmed, and the escape hatch (extending the helper) is rejected: four other fixture families depend on its shape.**
- **Census corrections, matcher stated:** `driver_test.go` carriers are **32/120 at `driver/driver_test.go` (median 81.5)** and **34/120 under any `*_test.go`** — the SPEC's "34/120" is the broad matcher; **all four analogues carry one**. `AssertStats` carriers are **84/120 at this tip** (`) AssertStats(` per-dir), vs the documentary "82 of 120" measured at the SPEC tip — count drifted +2 (0118 among them); only the matcher+tip pair is meaningful.
- ⚠️ **A3's "row 140 is well-formed" is a MISREAD, not a refutation, and is recorded so it does not propagate:** line 140's defect is an **EMPTY cell** — exactly what ARM-B of the well-formedness disjunction exists to catch; ARM-A (pieces != 8) catches 119/131 only, **which is what the router already says**. The escape-BLIND check fired on 17 rows — the standing false-positive count, reproduced.

### 1.6 ⚠️ SIXTH — **BASELINE AND GATE-HYGIENE NOTES FROM THIS STAGE'S OWN RUNS**

- **Full 120-fixture baseline at `546b453d`: attempt 1 ABORTED** — the documented driver-receiver port race, this time on **`0081-grpc-access-log`** (`panic: driver: start ALS receiver on 0.0.0.0:42039: bind: address already in use`, `INNER_EXIT=1`, 246 s, only 82 PASS / 96 RUN flushed). No sibling `go test` before or after (checked). **Attempt 2 CLEAN: `INNER_EXIT=0`, 120/120 `--- PASS: TestDifferential/`, 0 FAIL/SKIP, 0 `no driver registered`, 389.4 s.** `reference_harness_exit_code_is_not_command_exit_code` and the ~3-launch budget both re-confirmed live.
- ⚠️ **The panic gate must stay ANCHORED.** An unanchored `panic|DATA RACE|SIGSEGV` grep returns **14** hits on a fully-green log — all fixture noise (a lua "panic recovered" log line, wasm rust-panic symbol names, `0097-lb-panic-threshold` fixture names). The canonical `^panic:|DATA RACE|SIGSEGV` form returns 0. **A gate that fires on a green log is worse than no gate.**
- Scoped `0119` runs cost **~2.5–3.0 s** wall; the 0004+0079+0080 batch **8.4 s** — break-arm iteration at the IMPL is cheap; only full-suite launches are expensive.
- No TLS/ALPN surprises against the reference's mapped port (0079 PKI + `ServerName: "localhost"` works as-is); no WINDOW_UPDATE handling needed at these payload sizes; writing request HEADERS before the server SETTINGS arrives is accepted by both sides; no MetaHeadersFrame pitfalls.

---

## 2. THE ITEMS THE SPEC AND THE 84.1 HANDOFF SAY THIS PLAN OWES — ALL DISPOSED

| owed item | disposition |
|---|---|
| assert the RPC's own status via Drive hooks + `CompareBytes`, never stats (ADR-0306 (v), shape 31) | **§4 Task 4/5** — the transcript grammar (§3) carries `grpc-status` verbatim in the TRAILERS line; **executed at this stage**: F1/F3 redden ONLY via `runner_test.go:1289 differential mismatch` |
| the VACUITY control — stats legs stay green under every arm | **EXECUTED (F5)**: under the F1 broken tree the minimal `AssertStats` leg passed while `CompareBytes` failed — **shape 31 proven live on this very fixture**. Task 8 re-derives at the IMPL tip |
| the SYMMETRIC control — same wrong value both sides must PASS | **EXECUTED (F4)**: a bogus constant line appended in the shared `drive()` → both transcripts carry it → **PASS**, bogus line verified present in both dumps |
| a liveness arm with a FAILING baseline | **EXECUTED (F6)**: blank import removed → `no driver registered` + SKIP + exit 0 in 0.81 s — **green-means-did-not-run reproduced**; Task 2 makes observing it the registration gate's RED |
| the frame-parity notes (empty-trailer `DATA len=0`; response-header conduit) | **§3 rules**: no empty-trailer arm is chartered (the reference's extra `DATA len=0 END_STREAM` on an empty block makes it frame-divergent BY DESIGN on a correct tree); the arms send no custom response headers, so the unvalidated response-HEADER conduit is never on the wire — **row 84 claims no credit for it** |
| do NOT wire `RunEncodeTrailers` | carried — the fixture is test-side only; zero production `.go` in this leg |
| `ROADMAP.md` byte-untouched at the PLAN; row 84 flips at the IMPL | honoured; Task 11 specifies the flip + leak check |

---

## 3. Global constraints

1. **The tree is already CORRECT for this leg — TDD's RED comes from the registration gate and from injected breaks, not from the tip.** Task 2's RED is the observed silent-green; every discriminating assertion's RED is a §5 break arm re-derived at the IMPL tip. `reference_liveness_break_needs_failing_baseline` governs throughout.
2. **The transcript grammar is FIXED** (measured GREEN at this stage; do not redesign it at the IMPL):
   ```
   ARM <name>
     HEADERS  end_stream=<bool> [<name=value …>]     # FILTERED: drop x-envoy-upstream-service-time, date BY NAME; keep all else in wire order
     DATA     end_stream=<bool> len=<n> hex=<payload>
     TRAILERS end_stream=<bool> [<name=value …>]     # VERBATIM, wire order, NO filtering, NO sorting
   ```
   Read errors are recorded INTO the transcript as `READ-ERR` lines with the dial addr scrubbed — never returned (`reference_fatalf_makes_assertions_unreachable`). **NO sort anywhere** (§1.3 — vacuous, measured twice).
3. **FOUR arms** (§1.1): `success` = `Check("")` → `grpc-status 0` after `DATA 00000000020801` · `notfound` = `Check("nope")` → `grpc-status 5` (HEADERS + TRAILERS, **no DATA frame** — this backend never emits gRPC Trailers-Only, SPEC §2.1) · `unimplemented` = POST `/grpc.health.v1.Health/Nope` → `grpc-status 12` · **`plain` = `GET /plain`, non-gRPC, trailer-less — the D-84-ENDSTREAM gate.** gRPC request framing: 5-byte prefix `00 xxxxxxxx` + protobuf (`Check("")` = `0000000000`; `"nope"` = prefix `00000006` + `0a046e6f7065`); request headers `:method POST`, `:scheme https`, `:path`, `:authority localhost`, `content-type application/grpc`, `te trailers`; request DATA carries END_STREAM; per-arm read deadline ~5 s; fresh TLS conn per arm (RootCAs = fixture CA, `ServerName "localhost"`, `NextProtos ["h2"]`).
4. **`-count=1` on every gate and every break arm**; a `-run` selector matching nothing prints `[no tests to run]` and **exits 0** — assert `=== RUN   TestDifferential/0119-grpc-unary-trailers` is present on every scoped run.
5. **`INNER_EXIT` is mandatory on every differential launch and on `go test ./...`. Budget ~3 full launches** — attempt 1 aborted at this very stage (§1.6). The panic gate stays **anchored** (`^panic:|DATA RACE|SIGSEGV`); the unanchored form false-fires 14 times on a green log.
6. **Restoration after every break arm verified by sha256 against the base blob** (`git show <base>:<path>`), never by eye; injection-site occurrence asserted `== 1` before writing; **confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`).
7. **Per-task `gofmt` + `golangci-lint run`** on the fixture package; a `typecheck` failure short-circuits `misspell`; locale US, prose-only.
8. **Subagent-driven**; subagents commit locally only; controller squash-pushes at close. One worktree per agent off a common base; private scratch; private port bands outside `20000-31007`, `11000-14999` and the static fixture range `10000-19172` (the `42000-46099` band has served phases 80–84.2 — ⚠️ **note `0081`'s ALS receiver binds `0.0.0.0:42039` inside it**: never park a private listener there while the differential runs).
9. **`git -C <abs-worktree>` for every git command** — the cwd-reset hazard has fired for 37 consecutive sessions. Tripwire `pwd` + branch + `rev-list --count` before any commit or gate run.
10. **Check `git worktree list` + `ps` for siblings before any differential launch** — phase 84 is OUTSIDE the driver-receiver port-race class (`GRPCHealthResponder` is runner-spawned kernel-ephemeral), so a port flake on `0119` itself is a FINDING; a flake on `0081`/`0084` is the documented pre-existing race.

---

## 4. File structure and the TWELVE-task spine

**Edit surface — entirely test-side.** `test/fixtures/0119-grpc-unary-trailers/{driver/driver.go, driver/driver_test.go, pki/{ca,listener,listener.key}.pem, README.md, expectations.yaml}` + **exactly one line** in `test/differential/runner_test.go` (the blank import, in numeric position in the `:26-145` block). **ZERO production `.go`. No new BackendKind (34 reused; tail stays 38 over 39 declarations). No new module (`git diff go.mod go.sum` = empty, MEASURED). No new ADR. No YAML mirrors (§1.4).**

Driver shape (measured prototype, 529 lines, preserved this stage at `/tmp/claude-1000/a2-84p2-scratch/0119-driver.go.reference` — ⚠️ **ephemeral scratch; a convenience, not a source of truth — the IMPL re-derives at its tip**): `RegisterFixture("0119-grpc-unary-trailers", …)` from `init()` (name MUST equal the directory name — the lookup at `runner_test.go:194` keys on the dir) · `BackendCount()=1` · `BackendKind()=fixture.GRPCHealthResponder` (34, `fixture.go:576`) · `ReferenceListenerPort()=10119` · `SubjectListenerName()="l_h2"` · bootstraps on 0079's `h2ListenerFilterChain` shape (`0079/driver/driver.go:286-319`: DownstreamTlsContext, `alpn_protocols ["h2","http/1.1"]`, PEMs via `inline_string` at indent 24, HCM `codec_type: AUTO` + router) with cluster `explicit_http_config.http2_protocol_options{}` — reference STRICT_DNS `host.docker.internal:backendPorts[0]`, subject STATIC `127.0.0.1:backendPorts[0]` · PKI plumbing on 0079's helpers (`fixtureDir/readPEM/indentPEM/ensureCertPool/tlsConfig`, `:201-234` + `:400-424`, ≈42 lines) · shared `drive(ctx, addr)` used by both hooks, raw `x/net/http2` Framer + `hpack` (both already in `go.mod` at v0.34.0) · minimal `AssertStats` (subject `http.ingress_http.downstream_rq_2xx >= 1`), **documented in the driver as NON-discriminating** (shape 31).

### Task 1 — baselines, sentinel, siblings
Re-run the three sentinel checks + the four NCs (input measured first); record ACTUAL output. `git worktree list` + `ps` sibling check. Full 120-fixture run with `INNER_EXIT` (expect the §1.6 envelope; the port-race flake on `0081`/`0084` is documented — re-run once, in-band recurrence is a FINDING).

### Task 2 — scaffold + the registration gate, RED FIRST
Create the fixture dir + copy the three PEMs from `0079/pki/` and **assert sha256 equality**: `ca.pem fc89684f…758d52` · `listener.pem e22f4eaa…547d9b7c3e` · `listener.key.pem 3a2d87a3…ae931f03` (the 0004-provenance PKI; notAfter 2046, SANs localhost/host.docker.internal/127.0.0.1). Write the driver skeleton with `RegisterFixture`. **RED, observed BEFORE the blank import**: scoped run → `no driver registered for fixture "0119-grpc-unary-trailers"` + `--- SKIP` + exit 0 (measured 0.81 s this stage). **GREEN**: add the one-line blank import → `=== RUN TestDifferential/0119-grpc-unary-trailers` appears. This is the fixture's liveness baseline — from here on, SKIP can no longer masquerade as coverage.

### Task 3 — bootstraps
Reference + subject YAML per §4 header. Gate: scoped run reaches both proxies (reference container healthy on mapped 10119; subject `l_h2` ready-sentinel parsed). Iterate config only inside this task.

### Task 4 — the shared `drive()` + transcript canonicalizer ⚠️ *mid-execution trigger risk lives here*
The §3 grammar verbatim. The canonicalization rule list is **CLOSED at three** (§1.3); any new rule the IMPL is forced to add is a FINDING to record in `PROGRESS.md`, not a silent edit. Unit-test the frame-encode helpers + canonicalizer in `driver/driver_test.go` against fixed inputs (median carrier is 81.5 lines; all four analogues carry one).

### Task 5 — the FOUR arms, GREEN 3/3
Expected transcripts (identical both sides, MEASURED at this stage): success = `HEADERS end_stream=false [:status=200 content-type=application/grpc trailer=… server=envoy]` + `DATA end_stream=false len=7 hex=00000000020801` + `TRAILERS end_stream=true [grpc-status=0]` · notfound = HEADERS + `TRAILERS end_stream=true [grpc-message=unknown service grpc-status=5]`, no DATA · unimplemented = `grpc-message=unknown method Nope for service grpc.health.v1.Health grpc-status=12` · plain = 200 + `DATA end_stream=true len=16` + no TRAILERS line. Gate: 3 consecutive scoped runs, `-count=1`, `INNER_EXIT=0`, `=== RUN` present each time (~2.5–3.0 s each measured).

### Task 6 — the `AssertStats` leg
Minimal named subset, with the in-driver comment stating it is NON-discriminating and WHY (shape 31; the reference books `upstream_rq_200`/`downstream_rq_2xx` even for streams it resets).

### Task 7 — README + expectations.yaml
Documentary convention (medians 136.5 / 96). The README must state: the four arms and why the plain arm exists (D-84-ENDSTREAM); the closed canonicalization list; that stats are non-discriminating here; no YAML mirrors and why (§1.4).

### Task 8 — the break roster, ⚠️ **RE-DERIVED AT THE IMPL TIP** (§5 is this stage's proof it CAN redden)
All six arms of §5, `-count=1`, occurrence==1 pre-assert, WHICH-assertion confirmed, sha256 restore. Expect the F2-without-plain-arm cell to stay GREEN — **that vacuity is the load-bearing measurement; record it, do not "fix" it by weakening the arm.**

### Task 9 — full 121-fixture run + gates
`go test ./test/differential/ -count=1 -v` with `INNER_EXIT` (PASS-count gate goes 120 → **121**); `go test ./... -count=1` with `INNER_EXIT` (cite the port race honestly, both ways); `-race` as a second run on the seam+fixture packages; `go vet`; `golangci-lint`; `gofmt -l`.

### Task 10 — stat surface +0, by call-site enumeration
ARM 1 (added production registration sites in the diff) is **empty-by-construction** for a test-only leg — so ⚠️ **the input measure is mandatory**: `git diff --unified=0 $BASE..HEAD -- '*.go' ':!*_test.go' | wc -l` will be **0 for this leg, and that 0 must be shown to come from an empty production diff, not a broken pipeline** — run the NC (inject one line into a production file in a scratch copy of the diff pipeline and watch ARM 1 fire). ARM 2: census invariant **208 / 36** (cite 208/36, never 208/84). ⚠️ Never via `TestNoNewStat*` (proven blind by execution at 84.1).

### Task 11 — ⚠️ **THE ROW FLIP — the one `ROADMAP.md` edit of the leg**
Row 84 (`:146` at this tip): status cell `in-progress` → `done`, and the `sub-phases` prose records the 84.2 leg — **update-in-place per §Schema `:18`, ONE line changed, `git diff --numstat` = `1 1`**. `want` **STAYS 116** (no row added or removed). ⚠️ **The whole-file leak check goes LIVE**: before/after counts with `--` before every pattern — data rows **116 → 116** · check-(2) union **6 → 6** · `-family row` **95 occurrences / 67 LINES → unchanged** · `gRPC-family row` **2 → 2**. ⚠️ **No sentinel matcher string may be written into the row's prose** — `done` prose naming "deferred candidates:" would MANUFACTURE a check-(2) hit; a `-family row` phrase would inflate check (3)'s pass census. **Sentinel state after the flip: check (1) SILENT for the first time in the phase · check (2) SIX · check (3) SILENT ⇒ the sentinel STILL does not fire (check 2 prints), and `stop` must NOT be created.** The row-doctoring and check-(3)-doctoring NCs are still mandatory — two of the three checks are now silent, and silence without a firing NC is indistinguishable from a broken check.

### Task 12 — stage close
`STATE.md` §Current IN PLACE (all seven fields; lifecycle-state 3 → DONE; **phase 84 CLOSED, row 84 `done`**; next-skill = the next stage per the standing directive — **with row 84 closed there is no banked mid-lifecycle work, so the roller SELF-PICKS, smallest defensible candidate first, recording the pick + rejected alternatives in the next BRAINSTORM**) · §Recent demotion + eviction with the ROBUST archive-absence guard (bullet-anchored, any-run-before-backticked-target, corroborated by `grep -cF`; NC on a REAL present annotated entry; guard regex DESCRIBED never spelled) · `STATE_HISTORY.md` strictly-append-only (`N 0` + byte-exact prefix) · `PROGRESS.md` IMPL-84.2 section · `next-prompt.txt` roll (**`git add -f`** — tracked but gitignored) · `ADR-0306` amend **in place ONLY if the IMPL learned something the ADR states wrongly** (ADR-0296 indented-blockquote precedent; expected byte-untouched) · `BEHAVIOR_CONTRACT.md` expected **byte-untouched** (the prototype surfaced zero new contract statements; if the IMPL's fixture surfaces one, ADR-0052 `:1821` makes an ADR the vehicle — do not edit silently).

---

## 5. The break/control roster — **SIX ARMS EXECUTED AT THIS STAGE, WITH THE INJECTION SITES AND VERBATIM RESULTS**

Every arm below was injected at `546b453d` against the prototype, `-count=1`, occurrence asserted `== 1` before writing, restoration sha256-verified. **The IMPL re-derives all of them at its own tip.**

| arm | injection (site at this tip) | result | verbatim evidence |
|---|---|---|---|
| **F1** drop-the-emit | `h2dispatch.go:751` `hasTrailers := len(trailers) > 0` → `false` | **RED**, exit 1 | `runner_test.go:1289: differential mismatch: first divergence at offset 192` — ref `end_stream=false len=7` vs subj `end_stream=true len=7`; TRAILERS line gone; collateral `hcm: h2: EOF` in subject log |
| **F2** variant B (unconditional) | same site → `hasTrailers := true; endStream := false` | ⚠️ **GREEN on the 3 gRPC arms AND on 0004/0079/0080** — §1.1's headline | — |
| **F2′** variant B + the plain arm | as F2, 4-arm driver | **RED**, exit 1 | `first divergence at offset 940` — ref `DATA end_stream=true len=16`, subj `DATA end_stream=false len=16` + spurious `TRAILERS end_stream=true []` |
| **F3** drop the carrier | `router_h2.go:250` `Trailers: resp.Trailers,` → `nil` | **RED**, exit 1 | same divergence shape as F1 (offset 192) — proves the fixture sees the *carrier* layer, not just the emit layer |
| **F4** SYMMETRIC control | bogus constant line appended in shared `drive()` — BOTH sides | **PASS**, exit 0 | bogus line verified present in both transcript dumps — the gate compares sides, not absolutes |
| **F5** VACUITY control | F1 re-injected, full fixture incl. `AssertStats` | stats leg **GREEN**, `CompareBytes` **RED** | the ONLY failing assertion is `runner_test.go:1289 differential mismatch`; zero `subject stats:` failures — **shape 31 live on this fixture** |
| **F6** registration liveness | `0119` blank import removed | **SKIP + exit 0 in 0.81 s** | `no driver registered for fixture "0119-grpc-unary-trailers" (driver package not yet blank-imported in runner_test.go)` |

⚠️ **NAMED AS VACUOUS, NOT REPORTED GREEN:** F2 against the three gRPC arms alone is **un-reddenable by construction** — that is the finding that mandates the plain arm, and the IMPL must reproduce the GREEN (proving the vacuity is real) before reproducing F2′'s RED (proving the plain arm closes it). F1 and F3 produce byte-identical divergence text — the fixture localises the *symptom*, not the layer; layer localisation belongs to 84.1's unit surface (its B1/B2/B3 arms). Acceptable, stated.

---

## 6. Differential and fixture posture

**(a) is NOT vacuous for this leg — the leg IS gate (a).** `0119` raises the downstream-ALPN-h2 fixture count 3 → 4 and is the only fixture asserting frame-level end_stream placement cross-side. **(b)** becomes the 121-fixture suite. **(c)** `test/conformance/grpc/` stays deferred IN WRITING (SPEC §4's two grounds; `ROADMAP.md:200` carries it); h2spec **53/53 stated WITH the scope caveat** — the gate has never run RFC 9113 §6; NINE slash-form selectors match zero cases; not run in CI. **(d)** ⚠️ **VACUOUS, and the word is "vacuous"** — §7.4 binds a phase introducing a parser/codec/filter; a fixture driver is none; fuzzers stay **55** in 48 files, all under `internal/` (the §7.4-location violation stays named). **(e)** owed in full at the IMPL, `INNER_EXIT` on every launch. **(f)** STANDING DEPARTURE — no `REVIEW.md`; **37 of 125** dirs carry one, none since 25.3.

---

## 7. Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

Input measured BEFORE anything was written — **234 lines / 116 data rows**.

| check | result at `546b453d` |
|---|---|
| **(1)** | **`NOT DONE: row 84`** at `want=116`, denominator printed — **CORRECT while the phase is open; goes silent at the 84.2 IMPL, not before** |
| **(2)** | **SIX** — `:194 :200 :206 :216 :222 :230` |
| **(3)** | **SILENT** |

⇒ conjunction; (1) and (2) print ⇒ **does NOT fire**. `ls stop` → `No such file or directory`.

**FOUR NEGATIVE CONTROLS, ALL FIRED:** row 62 doctored ⇒ `NOT DONE: row 62` **and** `row 84`, with `NC LANDED? [ in-progress ]` inspected first · `want=115` ⇒ `GATE FAIL: examined 116 data rows, expected 115` · the mandatory check-(3) doctoring (residual occurrences confirmed **0** first) ⇒ `NEVER OPENED: gRPC` while `WASM` stayed silent · check-(2) one-arm strip ⇒ **6 → 5, NOT 6 → 0**.

`ROADMAP.md` is byte-untouched at this stage, so the leak check is trivially invariant; baselines carried for the IMPL's Task 11, all measured with `--` before the pattern: check-(2) union **6** · `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · **234 / 116**.

**Archive guard for Task 12 and for THIS stage's close:** the eviction target is **`phase 83 (wasm-pause-arm-leak) PLAN done` (2026-08-03)** — the oldest §Recent entry by DIRECT READ of the five dates (08-07, 08-06 ×3, 08-03). ROBUST form + `grep -cF` cross-product on it plus a REAL annotated present entry plus an invented target, before appending; `STATE_HISTORY.md` **462 → 464**, `numstat` `2 0`, base a byte-exact prefix.

---

## 8. Counts at this tip — re-derived mechanically

fixtures **120** (next-free **0119**, port **10119** free with firing NC) · `driver/` layout **96**/120, `inputs/` **24**/120 · README **118**/120 (median 136.5) · expectations.yaml **96**/120 (median 96) · YAML mirrors **76/76** of 120 (⚠️ SPEC's 74/75 and its 0004 claim both corrected) · `driver/driver_test.go` **32**/120 (median 81.5; any-`*_test.go` 34) · `AssertStats` carriers **84**/120 at this tip (documentary 82 = SPEC-tip) · fuzzers **55** in 48 files · phase dirs **125**, REVIEW.md **37** · BackendKind tail **38** over 39 declarations, `GRPCHealthResponder = 34` (`fixture.go:576`) · `DECISIONS.md` **17990** / 305 headings / tail **ADR-0306 COMPLETE** (block `:17928`–EOF) / next-free **ADR-0307 from the tail** / strict `PROPOSED` guard **0** / `^---$` **216** (last `:17020`) / STATUS census **19** · `BEHAVIOR_CONTRACT.md` **5955**, `## HTTP/2` subsection at `:5902`, ledger-tail `1207` at `:5118`/`:5122` (DOC-SOURCED; only deltas asserted) · `ROADMAP.md` **234 / 116**, row 84 at `:146` `in-progress` · `STATE.md` **64** · `STATE_HISTORY.md` **462**, `prior active-phase` bullets **181** · `next-prompt.txt` **239** lines (tracked-but-gitignored) · full-suite wall **389.4 s** at this tip · scoped `0119` **~2.5–3.0 s**.

⚠️ **STILL CONTESTED, NO NUMBER CARRIED:** the `STATE_HISTORY.md` archive-gap total · production `stats.IsValidName` guard sites · the `ROADMAP.md:<line>` cite count · `allCallbacksNoOp` occurrences · the loose `PROPOSED` matcher's whole-file count · whether 84.1's realized net counts its committed probe (3414 vs 3124 — both stated, neither adjudicated).

---

## 9. Deferred — carried forward by name

Unchanged from PLAN-84.1 §11 (the h2spec selector defect as one coherent future row · the CONTINUATION decode discard and encode hole · H1→H2 502 · full response buffering · `test/conformance/grpc/` + the eight unregistered gRPC filter type URLs · the dead `RunEncodeTrailers` hook · zero client-path h2 fuzz coverage and the fuzzer-location violation · the response-HEADER conduit · the stale `STATE.md` §Project block and `harness_test.go:208` port inventory). **This leg retires none of them and adds none.** After the 84.2 IMPL flips row 84, **check (2)'s six family backlogs are the sentinel's sole blocker** — the next session self-picks from them per the 2026-07-12 standing directive.

---

## 10. Self-review against the SPEC and the 84.1 handoff

- Every owed item is disposed (§2), and the two the handoff left implicit are ruled: **four arms, not three** (§1.1 — the one place this PLAN materially exceeds its charter, on measurement), and **no sort** (§1.3).
- Nothing was accepted without re-derivation: the SPEC's YAML-mirror census is corrected (§1.4), its analogue line counts re-verified exact (§1.2), the router's port-convention prose sharpened (§1.5), A3's row-140 misread caught before propagation (§1.5), and the §6.1 sentence inherited from the row is **inverted for the leg with the overrun history named in the same breath** (§1.2).
- **`ROADMAP.md`, `DECISIONS.md`, `BEHAVIOR_CONTRACT.md` byte-untouched at this stage** — verified by `git status` before commit; the PLAN stage precedent (0 of 8 recent PLANs touched any of them) holds.
- ⚠️ **What this PLAN does NOT gate, stated:** the layer-localisation of F1-vs-F3 (owned by 84.1's unit arms) · the empty-trailer-block frame shape (excluded by design, §2) · the response-HEADER conduit (out of scope, named) · any canonicalization rule beyond the closed list of three (a new one at the IMPL is a recorded FINDING, not a silent edit).

---

## 11. NEXT

**The 84.2 IMPL** — the twelve-task spine above, subagent-driven, off the then-current master tip. It lands fixture `0119-grpc-unary-trailers` (four arms), re-derives the §5 roster at its tip, runs the 121-fixture suite, and **flips ROADMAP row 84 `done`** with the whole-file leak check live. **After it: check (1) goes silent, check (2) — six family backlogs — is the sole sentinel blocker, and the roller SELF-PICKS the next subject per the standing directive.**
