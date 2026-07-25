# Phase 76 Brainstorm — the `0061-lb-ring-hash` spread-assertion statistical margin (a Load-balancing-family MAINTENANCE row: the project's THIRD-recorded flake gets a DERIVED margin instead of a louder pass-count; +0 stats / +0 fixtures — `0061` is EDITED / **+0 production `.go` files** / +0 packages / +0 modules / +0 fuzzers / +0 BackendKinds / +0 imports / +0 exported symbols)

---

## 1. Mission and scope confirmation (76 — the row that replaces a PASS-COUNT certification with a MEASURED rate, and finds the original certification had ~49% power against the very defect it certified)

### 1.1 What phase 76 delivers as a self-contained whole

Three things, all in the test/docs layer:

- **The margin.** `test/fixtures/0061-lb-ring-hash/driver/driver.go:78` — `sourceIPs` **4 → 10**. `totalConns` is a DERIVED constant (`:80`) and tracks automatically. This moves the spread assertion's per-run failure probability from **3.7% to 5.1e-5** (~730×).
- **The measurement.** A seeded, deterministic Monte Carlo in `internal/cluster/ringhash_test.go` that asserts the collapse **RATE** directly (`p̂ < 1e-3`), rather than asserting that some number of re-runs happened to pass. This is the artifact that would have caught the original defect, and it is the artifact that makes the constant's value auditable rather than folkloric.
- **The prose sweep.** `0061`'s driver doc-comments, `expectations.yaml` and `README.md` all describe a 4-source-IP / 64-connection workload; the README additionally carries two claims this row **refutes** (§2.3).

### 1.2 What phase 76 does NOT deliver (forward to §8)

`fault.abort.grpc_status` and the three live divergences the probe surfaced beside it · `ssl.connection_error` · the `upstream_cluster` span tag · `Listener.stat_prefix` · the Runtime family opening · the gRPC and WASM check-(3) blockers · any change to `0062-lb-ring-hash-http` (which is already safe by a factor of ~500 000 — §2.3) · any production `internal/cluster` change.

### 1.3 Phase-done as a Load-balancing-family row (family STAYS OPEN)

The family is already open (`grep -c 'Load-balancing-family row' docs/envoy-go/ROADMAP.md` → **8**). Row 76 registers `in-progress` at this BRAINSTORM's stage-close commit per the ROADMAP §Schema invariant (`ROADMAP.md:21`), which **RE-OPENS sentinel check (1)** after its second-ever silent close at the phase-75 IMPL.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW anticipated (escape-valve armable)

~5-7 tasks, **one line of executable fixture change** plus a new unit test and a prose sweep. This is the smallest row the project has chartered. Far under ADR-0045's >25-task / >1500-LoC valve. The valve is armable but will not fire.

### 1.5 Package placement — ALL edits in EXISTING files, ZERO new packages

`test/fixtures/0061-lb-ring-hash/{driver/driver.go,expectations.yaml,README.md}` and `internal/cluster/ringhash_test.go`. **Nothing in production.**

### 1.6 No prebrainstorm-notes branch

None exists for this subject.

### 1.7 Phase 76's relationship to the existing seams

It consumes only what phases 59-62 already built. It touches no seam, adds no seam, and removes no departure. Its interesting content is **statistical and documentary**, not mechanical — which is precisely why it is defensible at this size: the one-line edit is trivial, and the reason it is the *right* one line took a derivation.

---

## 2. Design decisions

### 2.1 Row + subject confirmation *(SELF-PICKED per the 2026-07-12 standing directive → phase 76 row registered)*

The standing directive says **smallest defensible candidate first**. Five candidates were re-costed at this tip by four parallel read-only agents plus controller re-derivation (§11). The ranking, by re-derived cost:

| candidate | re-derived cost | production `.go` touched |
|---|---|---|
| **`lb-ring-hash-spread-margin` (PICKED)** | **~5-7** | **0** |
| `Listener.stat_prefix` | ~7-10 | 1 |
| `upstream_cluster` span tag | ~7-9 (but ~85-100 lines / ~18 files) | 5-6 |
| `fault.abort.grpc_status` | **~10-13** (was advertised 7-9) | 1-3 |
| `ssl.connection_error` | **~10-13** (was advertised 9-11) | 2 |
| Runtime family opening | ~10-14 | 3+ |

It is the smallest, and it is the only one of the six that the project **already owes**: the assertion has now fired at phases 57, 62 and 75, and at each occurrence `internal/cluster/**` was sha256-verified byte-untouched and the fixture was isolate-green, so it was correctly classified as not-a-regression three times and correctly fixed zero times.

#### ⚠️ THE HEADLINE — the inherited recommendation's PRIMARY justification is REFUTED, and it was never about cost

`fault.abort.grpc_status` was carried unchanged through the phase-75 SPEC §13, the phase-75 PLAN §5 and the router, always on the same distinguishing claim: *"it is the ONLY identified candidate that clears a sentinel check-(3) blocker — it opens the **gRPC** family."*

**It does not open the gRPC family.** Re-derived by the controller at this tip:

```
$ grep -c 'gRPC-family row' docs/envoy-go/ROADMAP.md
0
$ sed -n '187,189p' docs/envoy-go/ROADMAP.md
### gRPC family

gRPC bridge, gRPC-Web, gRPC-JSON transcoding, interop conformance.
```

The family charters **four** subjects — bridge, gRPC-Web, JSON transcoding, interop conformance — and an HTTP fault-injection knob delivers **none** of them. `ROADMAP.md:48` (row 09) files `abort.grpc_status` under family-expansion deferral, but filing a deferral is not chartering a family. A row that landed the knob and then wrote *"gRPC-family row"* into its own summary would clear check (3) **by writing the string the check greps for**, not by opening the family — and check (3) is explicitly designed to fail SAFE, which makes gaming it the one failure mode it cannot detect.

⚠️ This is `reference_brainstorm_adjective_acquires_adr_authority` in its purest observed form: an adjective (*"the only candidate that clears a blocker"*) travelled three stages and two documents unchallenged, acquiring more authority at each hop, and dissolved on the first `grep -c`. **The candidate remains viable and interesting — it simply is not privileged, and it is not cheap.**

#### The second reason the pick is right — the flake's own certification is statistically invalid, and that is a project-methodology finding

`test/fixtures/0061-lb-ring-hash/README.md:143-148`, verbatim:

> `go test ./test/differential/ -run 'TestDifferential/0061' -count=20` → **20/20 PASS** (66 s; 20 fresh reference containers). The affinity leg is DETERMINISTIC (**fixed ring** + fixed source-IP keys → never flakes); the spread leg (`>= 2`) is overwhelmingly stable (4 source-IP keys over 3 backends). No assertion loosened.

Both load-bearing claims are false, and the second is false in a way the project should generalize:

1. **"fixed ring" is FALSE.** Ring points are keyed on the endpoint address **including its port** (`internal/cluster/ringhash.go:88-91`: `addr := endpoints[j].Addr()`; `key := fmt.Sprintf("%s_%d", addr, i)`), and the harness assigns backends **OS-ephemeral** ports (`test/differential/runner_test.go:272`: `net.Listen("tcp", "0.0.0.0:0")`). The ring is therefore a **fresh random 3-way partition of the hash circle on every single run**. Only the *keys* are fixed.
2. **"20/20 PASS" had ~49% power.** At the true per-run failure rate (§2.3), P(20 consecutive passes) = `(1 - 0.0365)^20` ≈ **0.48**. A coin flip. The check that certified the assertion as *"overwhelmingly stable"* was **more likely than not to pass even if the assertion were exactly as broken as it in fact is** — and it did pass, and the fixture shipped, and it has flaked three times since.

⇒ **A pass-count is not a margin.** That is the row's reusable content, and it is why the deliverable is a *measurement* (§1.1) rather than a bigger `-count=N`.

### 2.2 Scope: the constant, the measurement, the sweep *(SPEC pins — D-RHSM-SCOPE)*

**IN:** `0061`'s `sourceIPs` constant · a seeded collapse-rate test in `internal/cluster/ringhash_test.go` · `0061`'s prose (driver doc-comments, `expectations.yaml`, `README.md`).

**OUT:** `burstPerIP` (see §2.3 — it is the *other* assertion's discriminating modulus and must NOT move) · `0062` · any `internal/cluster` production file · any change to the assertion's **threshold** (`>= 2` stays; the row fixes the *input distribution*, never the bar).

### 2.3 The fix and its derivation *(D-RHSM-MARGIN)*

**The failure mode is neither "too few requests" nor "threshold too tight".** The workload drives 64 connections, but `tcp_proxy`'s `hash_policy: [{source_ip: {}}]` resolves through `cluster.HashSourceIP` (`internal/cluster/hash.go:133`), which **strips the port** — so 16 connections from one source IP produce **one** hash key, not 16. The number that governs the assertion is `K` = **distinct keys** = `sourceIPs` = **4**. Raising the connection count cannot help at all.

Collapse = all `K` fixed keys land in the same host's arcs of a randomly-interleaved ring:

> **P(collapse) = 3 · (1/3)^K = 3^(1−K)**

Controller-executed Monte Carlo (200 000 trials per K, fixed seed, model built from the code — 3 hosts × 342 arcs, random interleaving, K uniform lookups):

```
K= 4  trials=200000  collapse=  7307  p=0.036535   analytic 3^(1-K)=0.037037   1 in 27
K= 6  trials=200000  collapse=   828  p=0.004140   analytic 3^(1-K)=0.004115   1 in 241
K= 8  trials=200000  collapse=    80  p=0.000400   analytic 3^(1-K)=0.000457   1 in 2500
K=10  trials=200000  collapse=    12  p=0.000060   analytic 3^(1-K)=0.000051   1 in 16666
```

A research agent independently measured **0.0349** at K=4 using a different replica (real `xxHash64`, ports drawn from 32768-60999, 60 000 trials) and reproduced the fixture's own gauge wants (ring size 1026, 342 per host) as its sanity check. **Three independent derivations — analytic, controller Monte Carlo, agent Monte Carlo — agree to within noise.** Three observed flakes over the project's full-suite history is exactly what a ~3.5% assertion looks like.

**The fix — ONE constant:**

```go
	sourceIPs  = 10   // 127.0.0.2 .. 127.0.0.11   (was 4)
	burstPerIP = 16   // UNCHANGED — see below
	totalConns = sourceIPs * burstPerIP // 160 (was 64) — DERIVED, tracks automatically
```

⚠️ **`burstPerIP` must NOT be reduced to hold `totalConns` at 64.** It is the modulus of the *affinity* assertion (`driver.go:284`: `c % burstPerIP != 0`), and it is that assertion's entire discriminating power. Rebalancing to `sourceIPs=8, burstPerIP=8` would trade a 3.7% spread flake for a **~15× weaker affinity leg** (P(a scattered break still lands on all-multiples-of-m) rises from 0.096% at m=16 to 1.48% at m=8). Wrong trade. The connection count rises to 160 and that is the intended cost.

**All of `127.0.0.0/8` is loopback on Linux**, so `.2`-`.11` bind exactly as `.2`-`.5` do today (the README already states this for the current four).

**Blast radius inside the fixture is ZERO code sites.** Verified: `grep -n '\b64\b' driver.go` returns **7 hits, every one a comment** (`:13`, `:23`, `:80`, `:272`, `:308`, `:312`, plus `:364`; `:405`'s `64` is `ParseUint`'s bit-size). Every assertion — conservation (`:291`, `:302`), affinity (`:284`), and both stat checks (`:335`, `:367`) — routes through `totalConns`/`burstPerIP`. `expectations.yaml`'s `64`s (`:6`, `:26`, `:27`, `:28`, `:32`, `:40`) are all `#` comments. ⇒ **the executable delta is one integer; everything else is prose.** This is the *inverse* of `reference_fixture_workload_constant_desync` — the driver is already clean and only the narration desyncs.

**Why `0062-lb-ring-hash-http` needs nothing:** it carries the byte-identical assertion text but drives `hashValues = 16` distinct `X-Hash` values (`0062/driver/driver.go:90`), so its K is 16 and P = 3⁻¹⁵ ≈ 7e-8 — ~500 000× safer than `0061`. **The entire delta between a fixture that has flaked three times and one that never has is `K`.** That contrast is itself strong evidence for the diagnosis, and the SPEC should record it as such.

### 2.4 Fixture posture: +0 new fixtures *(D-RHSM-FIXTURE)*

`0061` is **EDITED IN PLACE**. No new directory, no new YAML, no new reference port, no new BackendKind. Fixture count **STAYS 119** and `0118` remains free. `reference_differential_fixture_dispatch_constraint` is not engaged (one dir, one runner branch, unchanged).

⚠️ One cross-side prong to watch, and the SPEC must pin it: `AssertStats` asserts `cluster.c_echo.upstream_cx_total == totalConns` **cross-equal** (`driver.go:335`) and `upstream_rq_total == totalConns` **reference-side** (`:367`). Both become 160. `tcp_proxy` is 1:1 downstream-conn→upstream-dial on both sides, so this should hold unchanged — but it is the one assertion whose *value* moves, and it must be executed, not reasoned about.

The spread leg itself is **SUBJECT-ONLY** (`driver.go:297`: *"REFERENCE: conservation only (Docker NAT → single source IP → single-key pin)"*), so the margin fix does not need to hold reference-side — the reference is NAT-collapsed to one gateway source IP by construction (`reference_differential_hash_key_cross_side_infeasible`).

### 2.5 The verification design — a MEASUREMENT, not a pass count *(D-RHSM-VERIFY)*

This is the row's load-bearing design decision, and it exists because the project already fell into the alternative once (§2.1).

**Re-running the fixture `-count=N` is a weak and expensive instrument.** At the *fixed* K=10 it will pass; at the *broken* K=4 a `-count=20` re-run passes ~48% of the time. To distinguish the two empirically you need `-count≈100` (0.9635¹⁰⁰ ≈ 2.4%), i.e. ~100 fresh reference containers, and you still get a binary answer with no confidence interval.

**The deliverable instead is a seeded unit measurement** in `internal/cluster/ringhash_test.go`: build the ring over 3 endpoints at randomized ephemeral ports, draw the fixture's actual `HashSourceIP("127.0.0.2".."127.0.0.11")` key set, count collapse over M trials with a **fixed seed**, and assert `p̂` against a derived bar. Deterministic, seconds to run, and it turns *"it stopped failing"* into a number.

⚠️ **The existing unit test cannot serve this purpose and must not be mistaken for it.** `internal/cluster/ringhash_test.go:50-64` (`TestRingHash_DistinctKeysSpread`) builds `eps(3)` — **fixed** addresses `a:1000/b:1001/c:1002` — and walks 200 keys on a fixed arithmetic sequence. It is fully deterministic and cannot flake, which is correct for what it tests, but it holds fixed **the exact variable the fixture randomizes**. It is structurally incapable of measuring this defect. *(A landed test is not evidence about a property it does not vary.)*

**The break trio the PLAN must schedule** — and it is a trio because the load-bearing one is an *asymmetry*, not a failure:

- **Break α (LIVENESS — does the spread leg still bite at K=10?).** Force `m = 0` in `ringhash.go`'s `Pick` (`:129`) so every key returns `ring[0]`. Expected FAIL, verbatim: `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)`. ⚠️ **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`): under a total collapse the *affinity* leg survives (160 % 16 == 0) and *conservation* survives (the sum is unchanged), so an affinity- or conservation-shaped failure line means the break tested something else. Run `-count=1` (`reference_differential_break_protocol_count1`).
- **Break β (THE LOAD-BEARING ONE).** With the new measurement test in place, revert `sourceIPs` 10 → 4. **Expected: the unit measurement FAILS (`p̂ ≈ 0.037 ≫ 1e-3`) while the differential fixture stays GREEN.** That asymmetry *is* the proof — it demonstrates mechanically that the fixture alone cannot distinguish K=4 from K=10, and that the constant is load-bearing on a **statistical** property rather than on any single pass/fail outcome. Without β, a reviewer cannot tell this fix from a coincidence.
- **Break γ (ANTI-VACUITY on β).** Restore `sourceIPs = 10` and confirm the measurement test **PASSES** rather than never having run. β's failing baseline is what makes γ's green mean *"ran and passed"* rather than *"did not run"* (`reference_liveness_break_needs_failing_baseline`; and note this row's β/γ pair is the same shape as phase 75's G/G′).

### 2.6 The rivals, re-costed at this tip — TWO advertised costs REFUTED, TWO premises found UNVERIFIED

Every figure below was re-derived at `c57b98b8`; none is carried from the phase-75 PLAN §5 or SPEC §13 (`reference_deferred_candidate_cost_restale`).

**(a) `fault.abort.grpc_status` — ~10-13, not 7-9; and its family claim is refuted (§2.1).** The blocker *is* retired, and by live execution: an agent ran 23 arms against `envoyproxy/envoy:contrib-v1.37.2` and confirmed **Trailers-Only is real** — `grpc-status` arrives as a response **HEADER**, on both H1 and h2c, with zero trailers on all 23 arms. So ADR-0058 (`DECISIONS.md:1987`) genuinely does not block it. The cost grew because the probe surfaced **three live divergences on already-shipped code**, not one missing knob: (i) with `grpc_status` set, `abortEnabled` stays false (`internal/filter/http/fault/fault.go:135-137`) so envoy-go **forwards upstream** where the reference aborts; (ii) the reference **transcodes an `abort.http_status` local reply into the gRPC shape** when the downstream request is gRPC (503→14, 404→12, 418→2, all HTTP 200 + `application/grpc`), which envoy-go's landed `http_status` path does not do; (iii) `grpc_status` + a **non**-gRPC downstream still aborts, with HTTP **200**. Plus (iv) the content-type sniff rule was pinned exactly (`application/grpc` then end-or-`+`, case-**sensitive**) and the one in-tree implementation — `internal/filter/http/extproc/check.go:459`'s `strings.EqualFold` — is **wrong in both directions**. A row cannot honestly land the knob and leave (ii) and (iii) asserting a shape the reference does not produce. ⚠️ Also recorded: the anchor this recommendation carried for three stages, `extproc/check.go:518-530`, is **ragged** — the block is `:520-530`; and its own comment calling the header a *"pragmatic divergence from a trailer"* is **wrong**, an error ADR-0172 §Consequences inherits.

**(b) `ssl.connection_error` — ~10-13, not 9-11. Phase 75's substrate did NOT make it cheaper.** Both advertised savings verify — `assertSSLCrossProduct` **is** variadic (`internal/listener/manager_test.go:4530`) and `sslLeafRoster` **is** a one-line extension point (`:4500`) — but *"single point of extension"* is false: a fifth leaf still needs ~8 hand-edited rosters including a test **rename** (`…ExactlyFourSSLNames` → `…Five…`, a `-run`-selector footgun, `reference_differential_run_selector`) and the `helpText` guard whose own prose warns it goes **silently unguarded** if missed (`internal/stats/name_test.go:232-235`). ⚠️ **And the roster is INERT for this leaf**: none of the four landed cross-product arms drives a non-certificate handshake failure, so adding `"connection_error"` catches nothing and a stacked-control arm must be built from scratch. Worse, the recorded predicate is **incomplete** — an agent measured that a peer sending **RST** with zero bytes yields `*net.OpError`, for which **both** `errors.Is(io.EOF)` and `errors.Is(io.ErrUnexpectedEOF)` are **false**, so the phase-75 BRAINSTORM's recorded fix lets the exact population it was designed to exclude straight through. The working extension needs `syscall.ECONNRESET`, i.e. **+1 production import** where every prior costing said +0. Two landed table rows must flip (`manager_test.go:4318` and `:4319`), not one, and the previously-cited `:4294` anchor is stale by 25 lines.

**(c) The `upstream_cluster` span tag — ~7-9 tasks but ~85-100 lines across ~18 files, and its central premise is UNVERIFIED.** The tag is **already emitted** (`internal/tracing/span.go:77-78`); the value is hardcoded empty at three sites (`internal/filter/hcm/accesslog_emit.go:51`, `:112`, `:173`). The router's warning is **CONFIRMED**: `grep -c 'GetClusterName()' internal/cluster/manager.go` → **0** (repo-wide 11 hits, none in `internal/cluster/`), so the inherited break would not have compiled, let alone fired. The hidden cost is 42 `emitAccessLog` test call sites plus a new 10th parameter on three helpers — unlike phase 72's HOST tag, which rode an existing parameter. ⚠️ **UNVERIFIED and owed a live probe:** that the reference emits a route-resolved cluster name on a **local-reply span where no upstream was dialed**. Every fixture asserts key-presence only, and `0087`'s recorded *"reference emits `c_backend`"* comes from a **successfully routed** request. If the reference emits `""` there too, the five-zero-endpoint-sites problem dissolves and the row shrinks. ⚠️ Separately, five fixtures (`0087:51`, `0102:55`, `0105:60`, `0106:65`, `0107:65`) mis-attribute the gap to a **Lua bridge** — `grep -c lua 0087/driver/driver.go` → **0**; 0087 configures no Lua filter. That copy-pasted misattribution must not be inherited.

**(d) `Listener.stat_prefix` — ~7-10, and its blast radius is a red herring.** All ~50 `"listener."` sites are **address-parameterized**, and no fixture sets the field, so the row is purely additive. Reference behavior is already live-probed and landed (`BEHAVIOR_CONTRACT.md:1859`; `phases/74-.../SPEC.md:87` — `stat_prefix: MYSTATPREFIX` made the scope `listener.MYSTATPREFIX.ssl.handshake`, **address gone**). The two genuine risks are that SN3's `envoy_listener_address` label would carry a non-address (`internal/stats/name.go:83-92`), and that an **operator-supplied** string reaches the panicking name validator (`internal/stats/registry.go:117`) via the swallowed-panic boot-hang path — `reference_dynamic_stat_name_charset_guard` applied to config input.

**(e) The Runtime family opening — ~10-14, and it is the only evaluated candidate that genuinely clears a check-(3) blocker.** `### Runtime + hot restart family` (`ROADMAP.md:207-210`) has **zero rows**. `internal/runtime/doc.go` is a live-but-empty placeholder. The cheap framings do **not** survive: a read-only `/runtime` endpoint is **vacuous** alone (with no layer parsed it can only serve an empty snapshot against the reference's hundreds of built-in `reloadable_features` defaults, so it cannot discriminate), and `/runtime_modify` is strictly larger. The defensible opener is **static `layered_runtime` + one consumer**: lift the hand-written reject at `internal/bootstrap/bootstrap.go:568-569`, restrict to `static_layer`, and thread a snapshot to **one** of the **six pre-built, ADR-anchored `runtime_key` PARSE-REJECT arms** that ADR-0187 (`DECISIONS.md:11280`) and ADR-0195 (`:12413`) already forward-point at this exact family. **That standing asset is the strongest argument for this being the next non-maintenance row**, and §8 carries it forward as such.

### 2.7 Stat surface hypothesis: **+0** (1205 → 1205)

No name is added, removed or renamed. `TestNoNewStat*` guards assert a delta of zero and will.

---

## 3. Framework-survey result

- **3.1 New framework seam:** NONE. No new interface, no new dispatch, no new asserter type.
- **3.2 New packages:** NONE.
- **3.3 New go.mod modules:** NONE. The measurement test uses `math/rand` and the existing in-package `newRingHash`/`Pick`/`HashSourceIP`; all stdlib and in-tree.
- **3.4 REUSES:** `internal/cluster`'s existing ring construction and `eps(...)` test helper; `0061`'s existing `drive`/`AssertDistribution`/`AssertStats` structure unchanged.

---

## 4. Bootstrap-level applicability — NONE

No bootstrap field is parsed, rejected or lifted. `internal/bootstrap` is byte-untouched.

---

## 5. Stat surface hypothesis — **+0** (76): 1205 → 1205

⚠️ Restating the standing caveat rather than the number: the **absolute** stat-surface total is **documentary**, has no mechanical command, and rides **two** recorded ledger gaps (the known `1200 → 1201`, and `Phase 46.1b` closing at 1198 while `Phase 47.1` opens at 1200). This row asserts a **delta of zero**, which is mechanically checkable; it does **not** re-derive the total and must not present it as measured. A future phase should COUNT rather than trust the chain — and a +0 row is, notably, a cheap place to do it, though this row does not charter that.

---

## 6. Anticipated edit sites (the SPEC RE-DERIVES each at ITS OWN tip — a BRAINSTORM cite is not evidence)

| file | anticipated change |
|---|---|
| `test/fixtures/0061-lb-ring-hash/driver/driver.go:78` | `sourceIPs` 4 → 10 + its inline comment |
| `.../driver/driver.go:11-13`, `:23`, `:182`, `:267-275`, `:272`, `:308-312`, `:364` | doc-comment prose naming "4 source IPs" / "64 total" — **~7 comment sites, all verified as comments** |
| `.../expectations.yaml:6`, `:26-28`, `:32`, `:40` | prose workload description + the `== 64` narration |
| `.../README.md:20-27`, `:41`, `:66-71`, `:96-102`, **`:143-148`** | workload prose **and the refuted flake-check paragraph** (§2.1) |
| `internal/cluster/ringhash_test.go` | NEW seeded collapse-rate measurement test (~40-60 lines) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | ⚠️ **UNKNOWN whether any edit is owed** — the SPEC must determine whether a probabilistic-fixture margin discipline belongs in the contract at all. Default posture: **no edit**, since no behavior changes. |

⚠️ **Anchors drift within a phase's own tasks** (phase 75's ran +1 to +168, NON-MONOTONIC). Every anchor above must be re-derived with `grep` at the tip being edited, including any this document later "corrects" (`reference_a_drift_correction_is_itself_a_claim`).

⚠️ **After landing, sweep for prose that said the fixture did NOT have a margin** — and grep for the **SHAPE** of the old claim, not just its numerals (`4`, `64`, "fixed ring", "20/20", "overwhelmingly stable"). A sweep that greps only the stale numerals cannot see a site falsified in the opposite direction (the phase-75 `expectations.yaml` lesson).

---

## 7. BRAINSTORM-time open questions to the SPEC (the D-RHSM-* docket)

- **D-RHSM-SCOPE** — Confirm the three-part scope of §2.2 and that `burstPerIP`, the `>= 2` threshold, and `0062` are all OUT.
- **D-RHSM-MARGIN** — Pin `sourceIPs`. **10** is proposed (p ≈ 5.1e-5). **8** is the cheaper variant (p ≈ 4.6e-4, 128 conns). The SPEC must state the target rate as a **policy** (e.g. "≤ 1e-4 per run, so that ~1000 full-suite runs carry <10% cumulative risk") and derive the constant from it, rather than picking the constant first.
- **D-RHSM-VERIFY** — Pin the measurement test's shape: M (trials), the seed, the asserted bar, and where it lives. Confirm the β-asymmetry is executable as described.
- **D-RHSM-CXTOTAL** — **EXECUTE**, do not reason: confirm `upstream_cx_total` and reference `upstream_rq_total` both track to 160 cross-side (§2.4). This is the only assertion whose value moves.
- **D-RHSM-LOOPBACK** — Confirm `127.0.0.6`-`127.0.0.11` bind as `LocalAddr` in this environment. Expected trivially true on Linux; it is one `go test` away and must not be assumed.
- **D-RHSM-RUNTIME** — Measure the wall-clock delta. 64 → 160 connections on a sub-second drive loop should be noise against a ~3.3 s/run container-dominated cost, but the full-suite budget is a real constraint and the figure should be recorded.
- **D-RHSM-ADR** — Does this row warrant **ADR-0298**? Proposed **YES**: not for the constant, but for the reusable decision *"a probabilistic fixture assertion carries a DERIVED margin and is verified by a measured rate, never by a pass-count"*. That decision has now cost the project three misclassified flakes. The SPEC drafts §Context per ADR-0044-**as-used**.
- **D-RHSM-SWEEP** — Determine whether any OTHER fixture carries a probabilistic assertion with an underived margin. `0062` is known-safe (K=16); the SPEC should sweep for siblings rather than assume `0061` is unique. ⚠️ Scope this to *reporting*; widening the row to fix them is out.

---

## 8. What phase 76 does NOT deliver (forward)

Carried forward with the costs re-derived at §2.6, **not** at the phase-75 tip:

- **The Runtime family opening (~10-14)** — the strongest non-maintenance next row, and the only evaluated candidate that genuinely clears a sentinel check-(3) blocker. It has a standing asset: **six** landed, ADR-anchored `runtime_key` PARSE-REJECT arms already forward-pointing at it.
- **`fault.abort.grpc_status` (~10-13)** — viable, blocker retired by execution, but it does **not** open the gRPC family and it drags three live divergences plus a wrong content-type sniff with it.
- **`ssl.connection_error` (~10-13)** — needs a corrected predicate (`ECONNRESET`, +1 production import), a from-scratch stacked-control arm, and a new fixture.
- **The `upstream_cluster` span tag (~7-9 / ~85-100 lines)** — owed one live reference probe on its central premise before it can be scoped.
- **`Listener.stat_prefix` (~7-10)** — reference behavior already landed; blocked on a naming/label design question, not on evidence.
- The gRPC and WASM check-(3) blockers (the WASM line remains a deliberate ROADMAP bookkeeping artifact).
- The phase-75 residue, none chartered: `ssl.no_certificate` on a **RESUMED TLS 1.3 session** (never probed — the one scenario where the pinned predicate could be wrong in production) · on **QUIC** (never driven; the parity argument is structural, not measured) · at `require_client_certificate: true` (`0109` not run) · a **mechanical count** of the stat surface to replace the documentary total · `internal/statssink`'s four *"stays 1200 / 1196"* prose sites, stale since phase 49 · the phantom `B5` in two landed production comments.

---

## 9. ADR-0045 split readiness + ADR roster

**Split readiness:** a SINGLE FLAT ROW. ~5-7 tasks; one integer of executable fixture change, one new test, one prose sweep. The ADR-0045 escape-valve is armable and will not fire.

**ADR roster:** **ADR-0298** anticipated (next-free per `STATE.md:21`; `grep -c '^## ADR-0298' docs/envoy-go/DECISIONS.md` → 0), subject to **D-RHSM-ADR**. §Context is drafted at the **SPEC**, not here. **`DECISIONS.md` stays BYTE-UNTOUCHED at this BRAINSTORM** (the phase-75 precedent, verified: commit `e822f1ad` touched exactly `ROADMAP.md`, `STATE.md`, the new `BRAINSTORM.md`, and `next-prompt.txt`).

⚠️ If ADR-0298 is written, **do not put a whole-file grep count in it.** That species has now self-falsified in ADR-0296 ¶3, ADR-0297 ¶7 **and** ADR-0297 ¶9 — three times in two consecutive ADRs — each fixed one phase later. State the property with **no number**, or scope the grep to a line range (`reference_document_hygiene_claim_not_evidence`).

⚠️ And if a correction to a landed ADR is needed, **enumerate the population before asserting a form rule.** The indented-blockquote prescription has now outlived **three** wrong justifications (ADR family → refuted; self-vs-other-ADR → refuted at n=7 by `DECISIONS.md:17211`; graft scale → current). The form keeps surviving; the reason keeps failing (`reference_a_drift_correction_is_itself_a_claim`).

---

## 10. Envelope + counts (anticipated at the phase-76 IMPL; docs-only at this BRAINSTORM)

| axis | anticipated delta | at-tip value (re-run mechanically at close) |
|---|---|---|
| stat surface | **+0** | 1205 (**documentary**, see §5) |
| differential fixtures | **+0** (`0061` EDITED) | 119 |
| fuzzers | **+0** | 55 |
| BackendKinds | **+0** | tail 38 (39 constants, 0-38) |
| go.mod modules | **+0** | lineage figure 2; the single `go.mod` requires 67 |
| new packages | **0** | — |
| new exported symbols | **0** | — |
| **production `.go` files touched** | **0** | — |
| production imports | **+0** | — |
| ADRs | 0 or 1 (**ADR-0298**, per D-RHSM-ADR) | tail ADR-0297 COMPLETE |

**This BRAINSTORM is docs-only: ZERO production `.go`, ZERO test `.go`.**

---

## 11. Sized-against-source — the cost derivations (FOUR agents at tip `c57b98b8`, plus controller re-derivation)

### 11.1 What was and was NOT verified by execution

**VERIFIED BY EXECUTION (agents):** the reference's gRPC-abort wire shape (23 live arms, fresh container per fleet, `contrib-v1.37.2`) · the Go-side handshake-failure error taxonomy for `ssl.connection_error` (9 arms, real loopback TCP pair, go1.26.5) · the ring-collapse rate at K∈{4,6,8} (60 000 trials/K against a `xxHash64` replica reproducing the fixture's own 1026/342 ring-size gauge).

**VERIFIED BY EXECUTION (controller, independently — not copied):** `grep -c 'gRPC-family row'` → **0** and the gRPC family's four-subject charter · `grep -c 'Load-balancing-family row'` → **8** · `0061`'s constants and the verbatim spread assertion · the ring-key-includes-ephemeral-port mechanism (`ringhash.go:88-91` + `runner_test.go:272`) · `HashSourceIP`'s port stripping · `0062`'s `hashValues = 16` · the README's verbatim flake-check paragraph · the absence of any literal `64` in any `0061` assertion · the collapse rate at K∈{4,6,8,10} over **200 000 trials/K** with a model built from the code rather than from the agent's replica · ROADMAP table geometry (row 75 at `:137`, `:138` blank, `:139` `---` ⇒ row 76 inserts at **`:138`**).

**NOT verified — carried as UNVERIFIED and forwarded:** the reference's `upstream_cluster` value on a **local-reply** span (§2.6c) · the full `httpToGrpcStatus` mapping table beyond 503→14 / 404→12 / 418→2 · RST-shaped abandonment **reference-side** for `ssl.connection_error` · `127.0.0.6`-`.11` loopback binding in this environment (D-RHSM-LOOPBACK) · the 160-connection wall-clock cost (D-RHSM-RUNTIME).

### 11.2 Controller re-derivation of the agents' load-bearing claims

The pick rests on two agent claims, and both were re-derived first-hand rather than accepted: the **collapse rate** (re-measured at 200 000 trials/K with an independently-constructed model — 0.0365 vs the agent's 0.0349 vs analytic 0.0370) and the **refutation of the gRPC-family claim** (re-run as a bare `grep -c`, then confirmed against the family's charter text). Neither is quoted from a report.

### 11.3 Contested and corrected claims

- **The agent's `ProbeAdmin` count was self-corrected mid-report** (3 → 121) and the correction is right: `ProbeAdmin` is a mandatory `Driver` method, so `grep -rln` over `test/differential/` finds only the declaration and two stubs. The reusable admin-differential pattern is `0009-admin-config-dump`, not `ProbeAdmin`. Recorded because the *first* number is the one a naive grep returns.
- **One agent's own probe fleet failed the discrimination discipline and it said so**: fleet 1 paired `grpc_status: 14` against `http_status: 503`, and 503 **maps to** gRPC 14, so the two arms were byte-identical for the wrong reason. It discarded that fleet's central conclusion and re-ran with `grpc_status: 7`. `reference_probe_must_discriminate`, instantiated against the prober's own work — and the reason (a) is reportable at all.
- **The router's ADR-0288 grep warning is right on the count and inverted on the order.** All three tokens return **2**, but the RULE STATEMENT is `STATE.md:7`, the **FIRST** hit — the live value is the second. A close script taking `head -1` would read the rule, not the value. ⚠️ **Never "fix" the count to 1**; that deletes the rule.
- **Anchor drift found, recorded, not silently absorbed:** `extproc/check.go:518-530` → the block is `:520-530` · `manager_test.go:4294` (the `io.EOF` table row) → `:4319`, stale by 25 lines.

### 11.4 Verifier fold — a correction to this session's own reasoning

The session opened by treating `fault.abort.grpc_status` as the presumptive pick, because three prior documents agreed on it and the router named it first. Agreement across documents is what a **carried** claim looks like, not what a **verified** one looks like — all three cites trace to one unexecuted assertion. The refutation cost one `grep -c`. ⇒ **The strength of a recommendation's provenance chain is not evidence about the recommendation**, and a claim that has survived several stages is *more* suspect than one written yesterday, not less (`feedback_brief_citations_not_evidence`).

### 11.5 ⚠️ THIS ROW COMMITTED THE EXACT FAILURE MODE IT DOCUMENTS — TWICE, IN ONE COMMIT, AND CAUGHT ONLY BY RE-RUNNING THE CHECK

§2.1 argues that a row could clear sentinel check (3) *"by writing the string the check greps for"*, and calls that the one failure mode a fail-SAFE check cannot detect. **The first draft of row 76 did precisely that, by accident, in the sentence making the argument.** Check (3) is `grep -qi -- "$slug-family row"` against `ROADMAP.md`; the draft summary quoted that marker phrase verbatim while explaining why quoting it would be gaming. Immediately after insert:

```
-- (3) --   NEVER OPENED: Runtime
            NEVER OPENED: WASM        ← gRPC silently GONE
```

The **second** occurrence was subtler and is the more instructive one: the summary also quoted **the command and its result** — `` `grep -c '<marker>' ROADMAP.md` → **0** `` — a claim the document's own landing makes false. That is the identical species the project has now fixed in ADR-0296 ¶3, ADR-0297 ¶7 and ADR-0297 ¶9 (`reference_document_hygiene_claim_not_evidence`), **but escalated**: previously it produced a wrong *number*; here it flipped a **termination-sentinel check**. A self-falsifying count misinforms a reader; a self-falsifying *sentinel* string moves the project one check closer to declaring itself finished.

**Remedy applied** — the ADR-0297 ¶7 form: the searched token is **deliberately not spelled anywhere in `ROADMAP.md`**, and the row states the property with the token elided rather than restating it (every restatement mints another counter-example). Both occurrences were removed and all three checks re-run: (1) `NOT DONE: row 76` · (2) **3** · (3) `gRPC`, `Runtime`, `WASM` — all three families restored.

⇒ Two rules this lineage should now treat as standing:
1. **Never write a sentinel's own matcher string into a file the sentinel greps** — not in prose, not in a quoted command, not inside quotation marks that a reader would read as a mention rather than a use. `grep` cannot tell mention from use.
2. **Re-run every sentinel check AFTER the stage's own edits land, not only before.** This near-miss was invisible to review by eye — the sentence is *about* not doing the thing — and visible instantly to the check. The pre-edit run at session open would have been clean and meaningless. *(This document deliberately does spell the token in §2.1: `BRAINSTORM.md` is not a file check (3) greps. That asymmetry is the point — the rule is file-scoped, not word-scoped.)*

---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these)

- **Phase-directory delta:** the new `BRAINSTORM.md` only.
- **ROADMAP:** row 76 registered `in-progress` at **`:138`** (RE-OPENING sentinel check (1)), mirroring the row-75 cell's six-column form. ⚠️ The `### Load balancing family` heading paragraph (`:153`) is **NOT** amended — the family is already open and this is a maintenance row, not a charter expansion (the phase-70/72/73/74 paired-edit precedent applies to family-charter rows; this is not one).
- **NO deferred sentence narrowed** — the phase-57 precedent; narrows land at the **IMPL only**. Check (2) must **STAY 3**.
- **DECISIONS.md stays BYTE-UNTOUCHED.**
- **STATE.md** §Current rolled **IN PLACE** (lifecycle DONE → 1); §Recent re-capped at FIVE **with its PREAMBLE updated** (ADR-0288). ⚠️ The ADR-0288 singleton greps return **2**, not 1, for each of the three field-name tokens — and the rule statement is the **FIRST** hit (`STATE.md:7`), not the second (§11.3).
- **`next-prompt.txt`** rolled to the phase-76 SPEC (**TRACKED despite `.gitignore`**; edited in this worktree).
- **Sentinel re-run MECHANICALLY TWICE** — in the stage worktree AND on landed master after the squash-push. ⚠️ Check (1) goes **live again** the moment row 76 registers, so its output changes at this commit.
- ⚠️ **Check (1)'s blind spot must be RE-DERIVED, never copied.** It read 107 rows / 103 matched / 4 misses at the phase-75 IMPL, and was recorded **wrong in two consecutive lineages** before that. Row 76's own addition changes the denominator.

Fresh worktree off master per `feedback_git_worktrees`; subagent-driven per `feedback_execution_style`; subagents commit LOCALLY only (`feedback_subagents_no_push`); controller squash-pushes at close (`feedback_push_to_origin`); parallel subagents got **PRIVATE scratch** (`reference_parallel_subagents_private_scratch`). ⚠️ `git -C <abs-worktree-path>` for every git command — **the cwd-reset hazard fired live during this session** (a `cd`-prefixed compound command reset the shell back to the main checkout, reported inline by the tool) and was contained by that discipline.
