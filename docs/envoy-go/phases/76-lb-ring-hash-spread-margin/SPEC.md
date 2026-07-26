# SPEC 76 — the `0061-lb-ring-hash` spread-assertion statistical margin (a Load-balancing-family MAINTENANCE row on a family CLOSED at row 54, so it claims **NO family ordinal**; `sourceIPs` **4 → 16** — the `0062`/`0063` house standard, **NOT** the BRAINSTORM's 10 — plus a seeded collapse-rate MEASUREMENT, a rescue of two SILENTLY VACUOUS unit tests, and a prose sweep; +0 stats / +0 fixtures / +0 packages / +0 modules / +0 fuzzers / **+0 production `.go` files**)

> ⚠️ **The BRAINSTORM's two headline scoping claims are BOTH refuted by execution:** *"the executable delta is ONE INTEGER"* (a never-mentioned `driver_test.go` fails, and two more tests go silently vacuous — §1.1) and *"`sourceIPs = 10`"* (below the project's own 4-5σ bar; the siblings that fixed the identical defect chose **16** — §3.2).

> **STAGE:** SPEC (lifecycle-state 1 → 2). **ROW 76 STAYS `in-progress`.** ROADMAP **BYTE-UNTOUCHED** (the phase-75 `2b5eff0a` precedent). File set: `DECISIONS.md` (the ADR-0298 §Context append) + `STATE.md` + this `SPEC.md` + `next-prompt.txt`.

---

## 1. Purpose / Mission

Phase 76 gives `test/fixtures/0061-lb-ring-hash`'s **spread** assertion — *"only N backend(s) nonzero, want >= 2 (ring collapsed?)"* (`driver/driver.go:295`) — a **derived** statistical margin, and replaces the pass-count that originally certified it with a **measured rate**.

The row delivers four things — one more than the BRAINSTORM scoped:

1. **The margin.** `driver/driver.go:78`, `sourceIPs` **4 → 16**. `totalConns` (`:80`) is DERIVED and tracks to **256**. Per-run failure probability **3.7e-2 → 7.0e-8**, a **1.79σ → 5.27σ** move (§3.2).
2. **The measurement.** A seeded, deterministic collapse-rate test in `internal/cluster/ringhash_test.go` asserting the **rate** — with a stacked non-vacuity control, without which its own claim is a null result (§3.3).
3. **⚠️ The unit-test rescue.** `driver/driver_test.go` — named in **no** phase-76 document — fails outright at the new constant, and two further tests go **silently vacuous**, including the unit test for this row's own assertion (§1.1, §3.4).
4. **The sweep.** Every prose site the fix falsifies — including **`BEHAVIOR_CONTRACT.md:1326`**, which the BRAINSTORM's default posture said was probably untouched (§1.1 R3, §6).

### 1.1 BRAINSTORM drift ledger — what this SPEC RE-DERIVED, REFUTED, and newly found

Every figure below was re-derived at this SPEC's own tip (`6ef436ac`); none is carried.

#### ⚠️ THE HEADLINE — "THE EXECUTABLE DELTA IS ONE INTEGER" IS FALSE, AND THE FILE THAT FALSIFIES IT IS NAMED NOWHERE IN THE LINEAGE

The BRAINSTORM (§2.3), STATE.md's `next-skill`, and the router (item 7) all state the same thing, and the router tells the SPEC to *"verify this yourself before scoping edits"*: **`grep -n '\b64\b' driver.go` returns 7 hits, EVERY ONE A COMMENT; every assertion routes through `totalConns`/`burstPerIP`, so the executable delta is one integer and everything else is prose.**

**`test/fixtures/0061-lb-ring-hash/driver/driver_test.go` exists — 6 tests, 2879 bytes — and it hard-codes 64-summing distributions.** `grep -c 'driver_test' BRAINSTORM.md` ⇒ **0**. It appears in no edit table, no anchor roster, and no risk note in any phase-76 document.

**EXECUTED, in the worktree carrying the committed `sourceIPs = 10`:**

```
$ go test ./test/fixtures/0061-lb-ring-hash/driver/ -count=1 -v
=== RUN   TestAssertDistribution_Affinity
    driver_test.go:14: expected pass for an affine subject + conserving reference,
                       got: subject conservation: sum 64 != 160
--- FAIL: TestAssertDistribution_Affinity (0.00s)
FAIL	github.com/pgdad/envoy-go/test/fixtures/0061-lb-ring-hash/driver	0.002s
```

⚠️ **AND TWO MORE TESTS GO SILENTLY VACUOUS — including the unit test for the very assertion this row exists to fix.** `AssertDistribution` evaluates its legs in order (length → affinity → subject conservation → spread → reference conservation), so a tuple that no longer sums to `totalConns` trips **conservation** before reaching its own leg. The test still reports `--- PASS`:

| test (line) | leg it exists to prove | leg that ACTUALLY fires at the new constant |
|---|---|---|
| `_Affinity` (13) | passes clean | **HARD FAIL** — subject conservation |
| `_ScatterBitesAffinity` (23) | affinity | affinity ✓ still correct |
| **`_CollapseBitesSpread` (33)** | **spread — THIS ROW'S ASSERTION** | **subject conservation ⇒ VACUOUS** |
| `_SubjectConservation` (42) | subject conservation | subject conservation ✓ |
| **`_ReferenceConservation` (51)** | reference conservation | **subject conservation ⇒ VACUOUS** |
| `_WrongLength` (59) | length | length ✓ |

⇒ **A PLAN that fixes only the visible red lands a green suite with two dead assertions**, one of which is the spread test. `reference_deliberate_break_wrong_assertion` and `reference_fatalf_makes_assertions_unreachable`, both instantiated. **All four 64-summing tuples must be rescaled AND each test must be re-proved to fire its OWN leg** (§3.4).

⚠️ **Why the whole lineage missed it, and why this SPEC nearly did too.** The BRAINSTORM's evidence was a **`grep` over `driver.go`** — a file-scoped search that cannot see a sibling file. This SPEC's first confirmation was **`git diff master --stat` ⇒ `1 file changed, 1 insertion, 1 deletion`** — which measures what was *edited*, never what was *broken*. And the probe agent ran `go test ./test/differential/` — the differential suite — which **passed**, because it never compiles the driver package's `_test.go`. **Three independent "confirmations", all blind in the same direction.** The only thing that found it was running `go test` over the fixture's own package. ⇒ **a change-set measure is not a build measure; run the package, not the diff.**

#### CONFIRMED by controller re-derivation (not copied)

- **C1 — the anchors.** `sourceIPs` `driver.go:78` · `burstPerIP` `:79` · `totalConns` `:80` · spread assertion **`:295`** · affinity `:284` · subject conservation `:291` · reference conservation `:302` · `upstream_cx_total` `:335` · reference `upstream_rq_total` `:367`. `Pick` DECLARED at `internal/cluster/ringhash.go:133`, DOC COMMENT `:129`, ring lookup `m := sort.Search(...)` **`:140`**, legitimate wrap `m = 0 // wrap` `:142`. **The router's break-α anchor correction (`6ef436ac`) is correct as landed.**
- **C2 — the diagnosis.** `HashSourceIP` (`hash.go:133`) hashes `ipOnly(addr)`, stripping the port ⇒ `burstPerIP` connections from one source IP are **ONE** key ⇒ **K = `sourceIPs`**. The ring is keyed on `endpoints[j].Addr()` *including* the ephemeral port (`ringhash.go:88-92`), and the harness binds backends with `net.Listen("tcp","0.0.0.0:0")` (`runner_test.go:272`) ⇒ **a fresh random 3-way partition every run.**
- **C3 — the rate, re-measured independently.** Controller Monte Carlo, 200 000 trials/K, seed 20260725, model built from the code: K=4 → **0.036525** (analytic 0.037037) · K=6 → 0.004295 · K=8 → 0.000460 · K=10 → 0.000035. A **fourth** derivation against the REAL `newRingHashWithRNG`/`xxHash64`/`HashSourceIP` measured **0.0355** (§3.3). Analytic **P = 3^(1−K)** stands, now four ways.
- **C4 — the source-IP binding needs no new plumbing.** `net.IPv4(127, 0, 0, byte(2+s))` (`driver.go:196`) yields `127.0.0.2..127.0.0.17` at K=16 arithmetically. **All ten of `.2`-`.11` were EXECUTED as `LocalAddr` binds, 10/10** (§3.5). Payload `"rh-%d-%d\n"` gains a second digit at s≥10 — **noted, and immaterial**: both sides run the identical `drive`, so cross-side byte-equivalence is structurally unaffected.
- **C5 — check (1)'s blind spot, RE-DERIVED not copied.** **108 data rows (`:31`-`:138`) / 104 matched / FOUR misses** — `00` (em-dash "after" column), `04` (DOT in slug `http-1.1`), `28.1a`, `28.1b` (LETTER suffix). All four `done` ⇒ no current impact.

#### REFUTED / CORRECTED

- **R1 — the Load-balancing family is CLOSED, not "already open."** BRAINSTORM §1.3 says *"the family is already open"*. `ROADMAP.md:116` (row 54) reads **"EIGHTH and FINAL Load-balancing-family row — the family CLOSES at phase-done."** The eight ordinal-claiming rows are 34/35/36/37/38/52/53/54; **row 76's cell claims no ordinal, and that is CORRECT.** ⇒ row 76 is **MAINTENANCE on a CLOSED family**; **ADR-0298 must claim NO family ordinal**, and the family heading stays unamended for the reason *"maintenance rows do not extend a charter"*, not the BRAINSTORM's. Check (3) is unaffected (the marker occurs 8× regardless).
- **R2 — `sourceIPs = 10` is BELOW the project's own recorded bar, and the house standard is 16.** See §3.2. K=10 is a **3.89σ**-equivalent margin; `reference_differential_band_sigma_margin` records **4-5σ**; siblings `0062`/`0063` both use **16** (5.27σ) for the byte-identical defect. **The proposed constant is revised 10 → 16.**
- **R3 — ⚠️ `BEHAVIOR_CONTRACT.md` IS owed an edit; the BRAINSTORM's default is WRONG.** BRAINSTORM §6 records it as *"UNKNOWN … Default posture: no edit, since no behavior changes."* **`BEHAVIOR_CONTRACT.md:1326` states the fixture's workload verbatim** — *"source IPs `127.0.0.2..5` (16 conns each = 64 total)"*, *"conservation `sum == 64`"*, *"all 64 on ONE backend"*, *"asserted on `sum == 64`"*. Mechanically **four `64`s and one `127.0.0.2..5` on that ONE line**, and it is the **only** such line. The error is the inference *"no behavior changes ⇒ no contract edit"*: **the contract documents the fixture's WORKLOAD as well as the proxies' behavior.** ⚠️ `% 16 == 0` on the same line must **STAY** (`burstPerIP` unmoved).
- **R4 — an OPPOSITE-DIRECTION false claim, in FOUR places, invisible to a numeral sweep.** `0061/driver.go:270-271`, `0061/expectations.yaml:21-22`, `0062/driver.go:300`, `0063/driver.go:300` all assert *"DETERMINISTIC/EXACT — not a σ-band"*. **True for affinity; FALSE for spread**, which is exactly a probabilistic assertion — as `0062`'s own README concedes by deriving its margin. These sites contain **none** of the stale numerals (`4`, `64`, `20/20`, "fixed ring"), so the BRAINSTORM §6 sweep vocabulary **cannot reach them**. The phase-75 `0110/expectations.yaml` lesson, reproduced exactly.
- **R5 — `README.md:147`'s "fixed ring" is FACTUALLY FALSE, not merely stale**, and it is the root cause in one phrase: the ring is keyed on `Endpoint.Addr()` including the OS-ephemeral port. The README certifies stability on the exact property that does not hold.
- **R6 — the grep count is 8, not 7.** `grep -c '\b64\b' driver.go` ⇒ **8**. The BRAINSTORM's *seven* enumerated comment sites are individually correct and `:405` (`ParseUint`'s bit-size) is correctly excluded — but the router's *"returns 7 hits, EVERY ONE A COMMENT"* is wrong as stated: grep returns 8 and one of them is code.
- **R7 — `0062`'s constant is at `driver.go:91`, not the BRAINSTORM's `:90`.**
- **R8 — the sibling spread assertions are NOT byte-identical.** `0061:295` is subject-only; `0062:329`/`0063:329` prepend a `%s` side label and assert on **BOTH** sides (so their per-run risk is 2× the per-side figure). The BRAINSTORM's *"byte-identical assertion text"* is a near-miss, and the per-run/per-side distinction matters when comparing margins.

## 2. Non-purposes

`burstPerIP` (it is the AFFINITY leg's discriminating modulus, `driver.go:284`) · the `>= 2` threshold (the row fixes the input distribution, never the bar) · `0062`'s **constants** · any `internal/cluster` **production** file · `fault.abort.grpc_status` · `ssl.connection_error` · the `upstream_cluster` span tag · `Listener.stat_prefix` · the Runtime family opening · a mechanical re-count of the stat surface.

---

## 3. The change — the D-RHSM-* docket disposed one-for-one

### 3.1 D-RHSM-SCOPE **[RESOLVED — three-part scope CONFIRMED, ONE file added by necessity, ONE widening REFUSED]**

**IN:**
- `0061/driver/driver.go` — the `sourceIPs` constant **plus ~11 comment sites**.
- **`0061/driver/driver_test.go`** — ⚠️ **added by necessity, not by choice** (§1.1 headline). Four 64-summing tuples must be rescaled to 256, **and each test re-proved to fire its OWN leg**.
- `internal/cluster/ringhash_test.go` — the new seeded measurement.
- `0061/expectations.yaml` and `0061/README.md` — prose.
- **`BEHAVIOR_CONTRACT.md:1326`** — one in-place line (§6).

**OUT:** `burstPerIP` · the `>= 2` threshold · `0062`/`0063` **entirely** · every `internal/cluster` **production** file.

**⚠️ A widening this SPEC considered and REFUSED, recorded rather than silently dropped.** `0062/driver.go:300` and `0063/driver.go:300` carry the same false *"DETERMINISTIC/EXACT — not a σ-band"* adjective over their spread leg that `0061` does (§1.1 R4). An earlier draft of this SPEC brought those two comment lines IN, reasoning that shipping ADR-0298 while leaving landed comments asserting its negation is the `reference_code_comment_not_evidence` hazard authored deliberately. **That reasoning is rejected:** ADR-0298 ¶11 **records** the contradiction explicitly, which discharges the hazard without widening a maintenance row into two sibling fixtures it has no other business touching. The BRAINSTORM put `0062` out; the standing directive says smallest defensible; **the sites are DEFERRED and NAMED in §7**, which is what makes them findable. ⇒ **A recorded contradiction is cheaper than an unscoped edit.**

### 3.2 D-RHSM-MARGIN **[RESOLVED — `sourceIPs = 16`, NOT the proposed 10; the project ALREADY HAS a policy and it is LANDED TWICE]**

The BRAINSTORM asks the SPEC to *"state the target rate as a POLICY and DERIVE the constant from it, rather than picking the constant and reporting its rate."* It then proposes **10**, with **8** as a cheaper variant.

**Both are wrong, and the reason is that the policy did not need inventing — it is already landed in two sibling fixtures.**

#### The house standard, quoted from what shipped

`test/fixtures/0062-lb-ring-hash-http/README.md:57-63`, verbatim:

> …the harness allocates the 3 backend ports **DYNAMICALLY per run**, so the ring layout varies run-to-run. With N distinct values over 3 backends, the per-side probability that ALL N values collapse onto a SINGLE backend (failing spread) is `≈ 3·(1/3)^N`. The original **N=4** gave `3·(1/3)^4 ≈ 3.7%` per side and flaked the spread prong (18/20 — see below). Raising to **N=16** drops it to `3·(1/3)^16 ≈ 7e-8` per side — past the **5σ-equivalent flake-free margin** (`reference_differential_band_sigma_margin`, applied to a spread threshold rather than a σ-band). Empirically 30/30 PASS at N=16.

`0062` and `0063-lb-maglev` BOTH set `hashValues = 16` (`0062/driver/driver.go:91`, `0063/driver/driver.go:91` — ⚠️ **line 91, not the BRAINSTORM's `:90`**).

⇒ **This exact defect — same root cause, same formula, same original K=4, same 3.7% — was already diagnosed and fixed in this repo, twice. `0061` was simply left behind.** The row is not discovering a policy; it is finishing a migration.

#### The constant, checked against the project's own recorded bar

`reference_differential_band_sigma_margin` records that a differential band needs **~4-5σ, not ~2.5σ**. Converting each candidate to a one-sided Gaussian equivalent (controller-computed):

| K | p = 3^(1−K) | one-sided σ-equivalent | clears the recorded 4-5σ bar? | `totalConns` |
|---|---|---|---|---|
| 4 (today) | 3.70e-02 | **1.79σ** | **NO** — this is why it has flaked 3× | 64 |
| 8 (BRAINSTORM's "cheaper") | 4.57e-04 | 3.32σ | **NO** | 128 |
| 10 (BRAINSTORM's proposal) | 5.08e-05 | **3.89σ** | **NO — it does not even reach 4σ** | 160 |
| **16 (the house standard)** | **6.97e-08** | **5.27σ** | **YES** | **256** |

⇒ **`sourceIPs = 16`.**

⚠️ **A SELF-CORRECTION, recorded rather than quietly dropped.** This SPEC first derived **10** from a policy it invented here (*"per-run p ≤ 1e-4, so 1000 full-suite runs carry <10% cumulative risk"* — under which 10 is the unique smallest integer, and 8 fails at 36.7% cumulative risk). **The arithmetic was right and the premise was wrong:** inventing a fresh policy for an assertion shape the project had already ruled on twice is precisely the failure this lineage keeps recording. The invented policy is retained above **only** as the record of a rejected alternative. ⇒ **Before deriving a constant, grep for a sibling that already derived one.**

#### The cost, measured and computed — and one agent figure REFUTED

- **Connections: 64 → 256.** Wall-clock measured at K=10/160 conns was **+0.14 s** against a ~3.3 s container-dominated run; K=16 is re-measured at §3.6 rather than extrapolated.
- **The affinity leg's scatter-discrimination WEAKENS, and the SPEC must not claim otherwise.** `README.md:80` states that the probability a scattered break still lands on an all-multiples-of-16 tuple is `< 1%`. Controller-computed exactly (multinomial(n, ⅓,⅓,⅓), all three counts ≡ 0 mod 16):

  | n | P(scatter survives affinity) |
  |---|---|
  | 64 (today) | **0.0962%** |
  | 160 (K=10) | 0.3327% |
  | **256 (K=16)** | **0.3814%** |

  ⚠️ **This REFUTES the research agent's own figure of 0.53% at n=256** — re-derived first-hand as **0.381%**. The `< 1%` claim at `README.md:80` therefore **survives** and needs no edit, but the row **weakens** that leg ~4× and must say so plainly. This is an acceptable trade: the spread flake is *observed* (3 occurrences), the scatter adversary is *hypothetical* and `README.md:82` already concedes the invariant would not catch it.

**`burstPerIP` MUST NOT MOVE** — it is the modulus of the affinity assertion (`driver.go:284`). Rebalancing to hold `totalConns` down would trade an observed spread flake for a materially weaker affinity leg. The connection count rising to **256** is the intended and measured cost.

### 3.3 D-RHSM-VERIFY **[RESOLVED BY EXECUTION — the test is BUILT and MEASURED; and ⚠️ BREAK β IS *NOT* EXECUTABLE AS SPECIFIED]**

**The measurement was BUILT and RUN**, not designed on paper. It lives in `internal/cluster/ringhash_test.go` (package `cluster`, so `newRingHashWithRNG` / `HashSourceIP` / `hashXX` are reachable without exporting anything — **+0 exported symbols holds**).

**Pinned parameters:**

| parameter | value | why |
|---|---|---|
| `collapseTrials` (M) | **2000** | control-leg expectation M·3⁻³ ≈ 74 (far from 0); whole test **0.63 s** measured |
| `collapseSeed` | **20260725** | `rand.New(rand.NewSource(n))` is sequence-stable; the test is DETERMINISTIC and cannot itself flake |
| `collapseBar` | **1e-3** | ~14 000× above analytic 7.0e-8 at K=16, ~36× below 3.7e-2 at K=4 — separates the two K's in **both** directions |
| `collapseFixtureK` | **16** | mirrors the fixture's `sourceIPs` (⚠️ **by convention only — see §3.3.1**) |
| `collapseControlK` | **4** | the fixture's HISTORICAL K, retained permanently as the non-vacuity control |
| control band | **[0.015, 0.070]** | counts [30,140] against a mean of 71 ⇒ **4.95σ low / 8.3σ high**. ⚠️ A first draft used [0.02,0.06] = **3.7σ** low, which does **not** clear `reference_differential_band_sigma_margin`'s 4-5σ; one of 12 probe seeds landed at 52 collapses. **Widened, and the widening costs nothing** — the leg only separates ~3.6% from 0 |
| seed robustness | **12 seeds checked** | control spans 52..90 collapses, all inside the band; measured never exceeded 1 (threshold 2) ⇒ **the seed is not cherry-picked** |
| location | after `TestRingHash_DistinctKeysSpread`, before `TestRingHash_WrapAround` | groups with the ring-spread tests |

**MEASURED, at the committed parameters:**

```
$ go test ./internal/cluster/ -run 'TestRingHash_EphemeralPortRing_KeyCollapseRate' -count=1 -v
    ringhash_test.go:223: collapse rate: control K=4 -> 71/2000 = 0.0355 (analytic 0.03704);
                          measured K=16 -> 0/2000 = 0 (analytic 6.969e-08, bar 0.001)
--- PASS  (0.62s)
```

**And the law itself was measured over 1 000 000 trials** against the real `newRingHashWithRNG`/`xxHash64`/`HashSourceIP`:

| K | collapses / 1e6 | p̂ | analytic 3^(1−K) | p̂ / analytic |
|---|---|---|---|---|
| 4 | 35 555 | 3.556e-2 | 3.704e-2 | 0.960 |
| 6 | 3 909 | 3.909e-3 | 4.115e-3 | 0.950 |
| 8 | 381 | 3.81e-4 | 4.572e-4 | 0.833 |
| 10 | 38 | 3.80e-5 | 5.081e-5 | 0.748 |

⚠️ **The measured rate runs slightly BELOW `3^(1−K)`, monotonically more so as K grows** — a finite 1026-point ring gives sibling keys a mild negative correlation. ⇒ **`3^(1−K)` is a CONSERVATIVE UPPER BOUND**, which is the safe direction for a margin derivation. (At K=10 only 38 events landed, so that ratio carries ±16%.)

The control leg's **0.0355** independently reproduces the analytic **0.0370**, the controller Monte Carlo **0.0365** and the BRAINSTORM agent's **0.0349** — a **fourth** independent derivation, this one against the REAL `newRingHashWithRNG`, `xxHash64` and `HashSourceIP` rather than a replica.

**⚠️ THE NON-VACUITY CONTROL IS MANDATORY, AND THE VACUITY WAS MEASURED, NOT ASSUMED.** At M=2000 and K=16 the expected collapse count is **0.00014**. So `0/2000` — the leg the row's claim rests on — is *also* exactly what a test that never ran, a ring that stopped being redrawn, or a broken collapse detector would report. **The measured leg alone is a NULL RESULT.**

This was **demonstrated by execution**, not argued: freezing the ephemeral ports so the ring is static (the `eps(3)` posture) leaves the measured leg reporting `0/2000` and **still GREEN**, while the control leg goes red:

```
control leg (K=4): collapse rate 0/2000 = 0 outside [0.015,0.07] … The control leg must FIRE —
if it reports 0, the ring is no longer being redrawn per trial and the measured leg below is vacuous.
… measured K=16 -> 0/2000 = 0        ← still green with the randomization DESTROYED
```

The stacked K=4 control, evaluated over the SAME M ring draws so the two legs cannot disagree about the harness, is what converts the measured leg into evidence: **71 observed collapses** prove the ports really are redrawn, the ring really is repartitioned per trial, and the detector really fires. Both legs use `t.Errorf`, not `Fatalf`, so a control failure still reports the measured number (`reference_fatalf_makes_assertions_unreachable`). This is `reference_liveness_break_needs_failing_baseline` and `reference_positive_arm_cannot_catch_overfiring` applied at **design** time rather than at break time.

⇒ ⚠️ **Any PLAN wording of the form *"the unit test proves the ring randomizes"* is true only of the CONTROL leg. The measured leg proves nothing standing alone.**

#### ⚠️ 3.3.1 BREAK β IS NOT EXECUTABLE AS THE BRAINSTORM, STATE.md AND THE ROUTER ALL SPECIFY IT — REFUTED BY EXECUTION

All three documents specify break β identically: *"revert `sourceIPs` 10 → 4. **Expected: the unit MEASUREMENT FAILS (`p̂ ≈ 0.037 ≫ 1e-3`) while the differential fixture stays GREEN.**"*

**Executed. The measurement does NOT fail.** In a worktree carrying the committed measurement test with the fixture's `sourceIPs` at **4**:

```
$ grep -oP 'sourceIPs\s*=\s*\K[0-9]+'       .../0061-lb-ring-hash/driver/driver.go   ->  4
$ grep -oP 'collapseFixtureK\s*=\s*\K[0-9]+' internal/cluster/ringhash_test.go        -> 10
$ go test ./internal/cluster/ -run 'TestRingHash_EphemeralPortRing_KeyCollapseRate' -count=1
ok  	github.com/pgdad/envoy-go/internal/cluster	0.623s          # ← GREEN
```

**The unit measurement is entirely DECOUPLED from the fixture's constant.** `collapseFixtureK` is an independent literal in a different package; the fixture's `sourceIPs` is not read, imported or observed by it. Reverting the fixture alone leaves the measurement green, so **the asymmetry the row calls "THE PROOF" does not exist as written.**

⚠️ **This is `reference_fixture_workload_constant_desync` — and the BRAINSTORM §2.3 explicitly claims this row is *"the INVERSE"* of that hazard because the driver is already clean.** That was true of the driver. **The new test RE-INTRODUCES the hazard in a new place**, and the comment *"`collapseFixtureK` MUST equal `sourceIPs`"* is a **comment, not a mechanism** (`reference_code_comment_not_evidence`).

**RESOLUTION — the PLAN inherits BOTH of these, not one:**

1. **Break β is a TWO-EDIT break.** Revert `sourceIPs` **and** `collapseFixtureK` to 4. **EXECUTED — it fires correctly:**
   ```
   --- FAIL: TestRingHash_EphemeralPortRing_KeyCollapseRate (0.63s)
       spread-flake margin (K=4): collapse rate 71/2000 = 0.0355 >= bar 0.001
       (analytic 3^(1-4)=0.03704). ... Did sourceIPs in
       test/fixtures/0061-lb-ring-hash/driver/driver.go shrink?
   ```
   ⚠️ Record it as a two-edit break. A two-edit β proves the *unit test's own* constant is load-bearing — it does **not**, by itself, prove the *fixture's* constant is.

2. **A LINKAGE GATE closes what β cannot.** The single-edit case (fixture reverted, unit test not) is precisely the drift a future session would commit, and nothing in the tree detects it. Add to the phase-76 gate set:
   ```sh
   a=$(grep -oP 'sourceIPs\s*=\s*\K[0-9]+'        test/fixtures/0061-lb-ring-hash/driver/driver.go)
   b=$(grep -oP 'collapseFixtureK\s*=\s*\K[0-9]+' internal/cluster/ringhash_test.go)
   [ "$a" = "$b" ] || { echo "DESYNC: sourceIPs=$a collapseFixtureK=$b"; exit 1; }
   ```
   **NEGATIVE-CONTROLLED AND POSITIVE-CONTROLLED, both EXECUTED** (`reference_gate_command_negative_control` — the phase-75 lesson that a `+0` row is exactly where a broken gate goes unnoticed):
   ```
   desynced tree (4 vs 10):  GATE RED: DESYNC sourceIPs=4 collapseFixtureK=10     # fires
   synced tree  (10 vs 10):  GATE GREEN (10 == 10)                                # passes
   ```
   ⚠️ The PLAN should consider promoting this from a shell gate to a **Go test in the fixture's own driver package** if it can be done without exporting a symbol; if not, the shell gate stands and must be listed in the six-gate, not left in prose.

**Break α and γ are unchanged and remain as the router specifies** — α neuters the ring lookup at `internal/cluster/ringhash.go:140` (`m := sort.Search(...)` → `m := 0`; **NOT** `:129` the doc comment, **NOT** `:142` the legitimate wrap), expecting `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` and confirming WHICH assertion fired (under total collapse affinity SURVIVES because 160 % 16 == 0 and conservation SURVIVES because the sum is unchanged); γ restores 10/10 and confirms the measurement PASSES rather than never having run.

### 3.4 The `driver_test.go` rescale **[RESOLVED BY EXECUTION — and TWO tests were ALREADY vacuous before this SPEC noticed]**

Four tuples rescale from 64-summing to **256**-summing. Every subject tuple in an affinity-passing test keeps each element a multiple of `burstPerIP`=16.

⚠️ **The firing leg was MEASURED per test, not inferred** — a temporary probe called `AssertDistribution` with each test's exact tuples and printed the returned error. **Before/after:**

| test | leg fired at the `sourceIPs=10` tip | leg fired after the rescale to 256 |
|---|---|---|
| `_Affinity` | `subject conservation: sum 64 != 160` — **HARD FAIL** | `<nil>` ✅ |
| `_ScatterBitesAffinity` | `subject affinity: backend[0]=20 not a multiple of 16` ✅ | `subject affinity: backend[0]=84 not a multiple of 16` ✅ |
| **`_CollapseBitesSpread`** | `subject conservation: sum 64 != 160` — 🔴 **VACUOUS** | `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` ✅ |
| `_SubjectConservation` | `subject conservation: sum 48 != 160` ✅ | `subject conservation: sum 192 != 256` ✅ |
| **`_ReferenceConservation`** | `subject conservation: sum 64 != 160` — 🔴 **VACUOUS** | `reference conservation: sum 128 != 256` ✅ |
| `_WrongLength` | `expected 3 backend counts, got ref=2 subj=3` ✅ | unchanged ✅ |

**Controller-verified independently** at the committed K=16 tip, with its own probe: all six fire their own leg, and the fixture's unit package is `ok … 0.001s`, 6/6 PASS.

⚠️ **`_CollapseBitesSpread` is the unit test for the very assertion phase 76 exists to fix, and it had silently stopped testing spread.** ⇒ **A rescale is not complete until each test has been re-proved to fire its own leg.** Final tuples: `{256,0,0}/{128,64,64}` · `{256,0,0}/{84,108,64}` · `{256,0,0}/{256,0,0}` · `{256,0,0}/{64,64,64}` · `{128,0,0}/{128,64,64}` · `{256,0}/{128,64,64}`.

### 3.5 D-RHSM-LOOPBACK **[RESOLVED BY EXECUTION for `.2`-`.11`; ⚠️ `.12`-`.17` NOT re-probed after the K=16 revision]**

The driver binds via `net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IPv4(127,0,0,byte(2+s))}}` (`driver.go:196`) — **confirmed by reading the code, not assumed**. A standalone probe dialed a local listener from each address:

```
s=0  127.0.0.2   OK  observed-LocalAddr=127.0.0.2:54479
…
s=9  127.0.0.11  OK  observed-LocalAddr=127.0.0.11:50913
RESULT: 10/10 bound successfully, 0 failures
```

⚠️ **This probe was run at the superseded K=10, so it covers `.2`-`.11` only. `127.0.0.12`-`.17` are UNPROBED** (§13). All of `127.0.0.0/8` is loopback on Linux and the K=16 fixture ran green end-to-end (§3.6), which is strong indirect evidence — but the direct bind probe over the extension is **one `go test` away and is owed at the PLAN**.

> ⚠️ **A probe-harness lesson worth carrying:** the first loopback probe reported `i/o timeout` on all ten arms. The binds had already succeeded; the fault was the probe's own echo backend using `io.Copy(c, c)`, which self-splices and deadlocks (`reference_iocopy_self_splice_echo_backend`). **It presented as a connectivity failure and was a backend bug.**

### 3.6 D-RHSM-CXTOTAL and D-RHSM-RUNTIME **[BOTH RESOLVED BY EXECUTION at 256]**

**CXTOTAL — the only assertion whose VALUE moves.** `upstream_cx_total` is asserted **cross-equal** (`driver.go:335`) and reference `upstream_rq_total` per-side (`:367`); both track `totalConns` and become **256**.

⚠️ **A green PASS is not evidence — it can also mean "did not run".** The values were read out via a **discriminating break** (re-pin the `want`s to literals, let the failure lines print the OBSERVED values):

```
ref  cluster.c_echo.upstream_cx_total = 256, want 64
subj cluster.c_echo.upstream_cx_total = 256, want 64
ref  cluster.c_echo.upstream_rq_total = 256, want 256 (rq-per-cx)
subj cluster.c_echo.upstream_rq_total = 0,   want 0
```

A second break positively observed the stats that must NOT move: `membership_total`=3 · `upstream_cx_active`=0 · `ring_hash_lb.size`=1026 · `min_hashes_per_host`=342 · `max_hashes_per_host`=342, **identical on both sides**. ⚠️ The ring gauges are a function of the **host count**, not the key count, so K=16 leaves them untouched — verified, not assumed. Breaks reverted; green reconfirmed; **three warm `-count=1` runs all exit 0** (3.650 / 3.665 / 3.696 s).

**RUNTIME — and a measurement-methodology correction the SPEC adopts.**

| K | conns | same-session mean | cross-session recorded |
|---|---|---|---|
| 4 | 64 | **3.512 s** | 3.672 s |
| 10 | 160 | 3.638 s | 3.810 s |
| **16** | **256** | **3.670 s** | — |

⚠️ **The honest cost is +0.158 s (+4.5%) over K=4** — measured in one session against a same-session K=4 control. Quoting the *cross-session* baseline instead yields **−0.002 s**, i.e. "apparently free", which is a **machine-speed artifact**: the recorded baselines sit ~0.16 s above this session's uniformly. **The absolute figures do not transfer across sessions; the deltas do** (recorded 4→10 = +0.138 s, re-measured 4→10 = +0.126 s — same slope). ⇒ **quote the delta against a same-session control, never an absolute against a recorded one.** Runtime is dominated by reference-container startup and the fixed 750 ms `settleDelay`, neither of which moves.

### 3.7 D-RHSM-SWEEP **[RESOLVED — `0061` is the ONLY underived, reachable probabilistic margin left; REPORT-ONLY]**

A sweep of all **119** fixtures for assertions whose pass/fail depends on a random draw. Ranked by how likely each is to actually flake:

| # | fixture | site | random variable | per-run P(fail) | margin DERIVED? |
|---|---|---|---|---|---|
| **1** | **`0061-lb-ring-hash`** | `driver.go:295` | ephemeral ports → ring layout; K=4 keys over 3 backends | **3.7e-2** (measured 3.56e-2) | **NO** — bare `2`; the README asserts the OPPOSITE |
| 2 | `0059-lb-least-request` | `driver.go:308`, `:311` | P2C `choice_count:10` over 60 conns | not computed | **PARTIAL** — `driver.go:72-73` records *observed* margins (10 and ≥5) from ≥20 runs, but **no analytic tail** |
| 3 | `0013-http-local-ratelimit` | `driver.go:332` | CI wall-clock jitter | not analytic | **NO** — only 10 ms slack on a 250 ms target |
| 4 | `0017-http-bandwidth-limit` | `inputs/driver.go:360`, `:371`, `:593` | CI timer jitter | n/a | PARTIAL — `Tolerance = 70ms` |
| 5 | `0011-http-fault` | `driver.go:71-72` | CI timer jitter | n/a | PARTIAL — ±10 ms |
| 6 | `0095-lb-locality-weighted` | `driver.go:486` | weighted draw, n=900 | ~1e-6 | **YES** — `driver.go:78-85` derives 5σ |
| 7 | `0065-weighted-clusters` | `driver.go:397` | weighted draw, n=500 | ~4.5σ | **YES** — `driver.go:97-104` |
| 8 | `0060-lb-random` | `driver.go:319`, `:322` | Binom(64, ⅓), 6 samples | **1.21e-5** | **YES — the gold standard**; `driver.go:78-89` records the flake history, the σ figures AND the superseded band |
| 9 | `0062` / `0063` | `:329` / `:329` | ephemeral ports, K=16, **2 sides** | **1.39e-7** | **YES** — the precedent §3.2 adopts |
| 10 | `0064-lb-subset` | `driver.go:363`, `:394`, `:469` | **none** — `ROUND_ROBIN` | 0 | N/A, and `driver.go:449` says so correctly |
| 11 | `0097-lb-panic-threshold` | `driver.go:706` | **none** — `ROUND_ROBIN` | ~0 | N/A |

**VERDICT: `0061` is the only genuinely-reachable underived margin.** Everything below rank 3 is either derived or numerically unreachable. ⚠️ **REPORT-ONLY per the BRAINSTORM's scoping — none of ranks 2-5 is fixed by this row.** Named deferrals recorded in §7 so a future sweep finds them:

- **`0059`'s empirical-only margins** (rank 2) — the one other LB fixture that would benefit from an analytic tail replacing *"observed over ≥20 runs"*. ⚠️ **This is the same evidentiary shape as `0061`'s refuted `20/20`** (§1.1 R5): an observed-pass count standing in for a derivation. It has not flaked, so it is not owed — but it is the next candidate if one appears.
- **`0013`'s 10 ms band** (rank 3) — the highest timing-flake risk in the tree.
- **the four *"DETERMINISTIC/EXACT — not a σ-band"* sites** — `0061/driver.go:270-271` and `0061/expectations.yaml:21-22` are **IN** (this row's own fixture, §3.1); **`0062/driver.go:300` and `0063/driver.go:300` are OUT and DEFERRED**, recorded here rather than fixed, because widening a maintenance row into two sibling fixtures is not warranted by a false adjective that ADR-0298 ¶11 now records in the ledger.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules, **0 new production imports**

The measurement lives in `package cluster` and reaches `newRingHashWithRNG`, `ringHashCfg`, `hashXX`, `Endpoint` and `HashSourceIP` **without exporting anything**. Test-side imports grow by **`math/rand`** and **`strconv`** — both stdlib, both test-side; an import LINE is not a go.mod MODULE and there is no new sub-package, so `reference_new_subpackage_pulls_transitive_module` does not bite. New identifiers `ringCollapseHosts`, `allSame`, `pow3` were collision-checked repo-wide (`reference_spec_drafted_identifier_collision_check`) — unique.

## 5. Stat surface **+0** (1205 → 1205) · Fuzz **+0**

No name is added, removed or renamed; `TestNoNewStat*` asserts a delta of **zero**, which is mechanically checkable. ⚠️ The **absolute** total 1205 is **documentary**, has no mechanical command, and rides **two** recorded ledger gaps (`1200 → 1201`, and Phase 46.1b closing at 1198 while 47.1 opens at 1200). **This row asserts the delta, not the total, and must not present 1205 as re-derived.**

## 6. Behavior-contract edit map — **ONE line, IN PLACE, no line-count change**

⚠️ **This section exists because the BRAINSTORM said it would not** (§1.1 R3).

| anchor | edit |
|---|---|
| `BEHAVIOR_CONTRACT.md:1326` | `127.0.0.2..5` → `127.0.0.2..17`; **four** `64`s → `256`; `% 16 == 0` **UNCHANGED** |

**IN-PLACE rewrite, zero lines added or removed**, so every by-line citation into the section stays valid (the phase-75 discipline). It is the **only** such line: `grep -n 'source IPs .127\|16 conns each' BEHAVIOR_CONTRACT.md` ⇒ 1 hit.

## 7. Deferred items

`0059`'s empirical-only margin · `0013`'s 10 ms band · the `0062`/`0063` σ-band adjective (§3.7) · `README.md:80`'s scatter figure (survives; **do not claim improvement** — it weakens 0.096% → 0.381%) · a mechanical **count** of the stat surface to replace the documentary total · the phase-75 residue, none chartered. **The non-maintenance rivals**, all re-costed at the phase-76 BRAINSTORM and **not** re-costed here (`reference_deferred_candidate_cost_restale` — the PLAN must re-derive before adopting): the **Runtime family opening ~10-14**, the only evaluated candidate that genuinely clears a check-(3) blocker · `fault.abort.grpc_status` ~10-13 (blocker retired by execution; ⚠️ it does **NOT** open the gRPC family) · `ssl.connection_error` ~10-13 · `upstream_cluster` ~7-9 but ~85-100 lines / ~18 files with an UNVERIFIED premise · `Listener.stat_prefix` ~7-10.

## 8. Sentinel maintenance — **this row narrows NOTHING**

⚠️ **Unlike phase 75, phase 76 is NOT a sentence-narrowing row.** Verified mechanically: none of the three live `candidates:` sentences (`ROADMAP.md:186`, `:196`, `:206`) names the ring-hash margin or any load-balancing candidate — the only `ring`/`Load-balanc` substrings on those lines are `buffering`, `Load-bearing`, and a historical *"the Load-balancing family closed at phase 54"*. ⇒ **check (2) STAYS 3 at every phase-76 stage, including the IMPL.** ROADMAP is **BYTE-UNTOUCHED at this SPEC**; row 76 flips `in-progress` → `done` at the IMPL and nothing else.

⚠️ **Re-run all three sentinel checks AFTER this stage's edits land, not only at session open** — the pre-edit run is clean and meaningless, and the phase-76 BRAINSTORM's near-miss (a row that silently deleted `gRPC` from check (3) by quoting the marker phrase) was invisible to review by eye. **Never write a sentinel's own matcher string into a file the sentinel greps.** This SPEC does not touch `ROADMAP.md` at all, which removes the exposure but not the obligation to re-check.

---

## 9. Test plan + task surface *(a SINGLE FLAT ROW; the ADR-0045 valve is armable but will not fire)*

**~7-9 tasks**, up from the BRAINSTORM's ~5-7 — the increase is `driver_test.go` (§1.1 headline), the `BEHAVIOR_CONTRACT` line (§6), and the linkage gate (§3.3.1), none of which the BRAINSTORM scoped.

| # | task | red-first artifact |
|---|---|---|
| T1 | the measurement test at K=16, with its stacked K=4 control | control leg RED on a frozen ring (break γ) |
| T2 | `sourceIPs` 4 → 16 | `driver_test.go` goes RED — **expected, and it is T3's input** |
| T3 | rescale `driver_test.go`'s four tuples to 256 **and re-prove each test fires its OWN leg** | per-test error-string table, before/after |
| T4 | the linkage gate, negative- AND positive-controlled | gate RED on a desynced tree |
| T5 | `0061` comment sweep (~11 sites in `driver.go`, `expectations.yaml`, `README.md`) | — |
| T6 | `README.md:143-148` — the refuted flake-check paragraph | — |
| T7 | `BEHAVIOR_CONTRACT.md:1326`, in place, no line-count change | — |
| T8 | breaks α / β / γ | see below |
| T9 | six-gate + full differential + counts + ADR-0298 completion + row 76 → `done` | — |

### 9.1 The break roster — **α, β, γ, and β is a TWO-EDIT break**

- **α (LIVENESS).** `internal/cluster/ringhash.go:140`, `m := sort.Search(...)` → `m := 0`. ⚠️ **NOT `:129`** (the doc comment) and **NOT `:142`** (`m = 0 // wrap`, the legitimate wrap). Expected verbatim: `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)`. ⚠️ **Confirm WHICH assertion fired** — under total collapse the **affinity** leg SURVIVES (256 % 16 == 0) and **conservation** SURVIVES (the sum is unchanged), so an affinity- or conservation-shaped line means the break tested something else. `-count=1` (`reference_differential_break_protocol_count1`). ⚠️ **Run it AFTER committing** (`reference_break_protocol_commit_first`).
- **β (THE ASYMMETRY) — ⚠️ TWO EDITS, and the SPEC records why.** Revert **both** `sourceIPs` **and** `collapseFixtureK` to 4. Expected: the unit measurement **FAILS** (`71/2000 = 0.0355 ≫ 1e-3`, a 35× margin over the bar) while the differential fixture stays **GREEN**. **EXECUTED at K=10 and confirmed** (§3.3.1). ⚠️ **A two-edit β proves the UNIT TEST's constant is load-bearing; it does NOT by itself prove the FIXTURE's is.** The single-edit case is covered by the linkage gate (T4), not by β. **Do not describe β as proving more than it does.**
- **γ (ANTI-VACUITY).** Restore both to 16 and confirm the measurement PASSES rather than never having run (`reference_liveness_break_needs_failing_baseline`). ⚠️ **A second γ is owed and is the sharper one:** freeze the ephemeral ports so the ring is static, and confirm the **control** leg goes RED while the measured leg stays green — **executed at K=10, and it is the measurement that proves the measured leg is a null result standing alone** (§3.3).

### 9.2 Gates — negative-control every one

⚠️ **Phase 76 is a +0-production-file row, so every production-envelope gate is trivially green — which is exactly when a broken gate goes unnoticed** (the phase-75 lesson: one gate failed CLOSED on a `+0`-import tree, the other failed OPEN over the very package it audited). **A green gate is evidence only if you have seen it go red.** Specifically:
- The **import gate** must be normalised to basenames (the phase-75 `impblock` prints `FILENAME`, so a `/tmp` baseline vs an in-tree HEAD differs on every line).
- **`go doc -all <pkgA> <pkgB>` FAILS OPEN** — `go doc` takes `<pkg> [symbol]`, so the second package is read as a symbol and silently dropped, exit 0. **Audit each package SEPARATELY.**
- The **linkage gate** (§3.3.1) is negative- and positive-controlled and belongs in the six-gate, not in prose.

### 9.3 Known live hazards — never reflex-classify any of these as a regression

The PRE-EXISTING `internal/cluster` `-race` flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce`) · the full-suite startup flake (`subject ready: EOF`, and at phase 75 also `bind: address already in use` on an UNRELATED fixture, failing BEFORE any assertion) · `reference_sds_init_fetch_timeout_dial_budget_flake` · two still-UNINDEXED load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`). ⚠️ **`0061-lb-ring-hash` is THIS ROW'S OWN SUBJECT: a spread firing BEFORE the fix is expected at ~3.6%/run; AFTER the fix it is a FINDING.** ⚠️ **A stage brief's flake list is not the index — NEITHER flake that fired at phase 75 was on the PLAN's roster of six.** Isolate-re-run, then state the classification and its evidence.

---

## 10. Edit-site roster — RE-DERIVED at tip `6ef436ac`

⚠️ **Anchors drift within a phase's own tasks** (phase 75's ran +1 to +168, NON-MONOTONIC). Every anchor below must be re-derived at the tip being edited, **including any this document later "corrects"** (`reference_a_drift_correction_is_itself_a_claim`).

**`test/fixtures/0061-lb-ring-hash/driver/driver.go`** — 1 code line + ~11 comment sites: `:11` (`127.0.0.2..5`) · `:13` (`= 64 total`) · `:23` (`all 64`) · **`:78` — THE ONE CODE EDIT** + its trailing comment · `:80` (`// 64 — the conservation target`; the code is self-updating) · `:182` (`the 4 source IPs`) · `:183` (`127.0.0.2..5`) · `:270-271` (the false `DETERMINISTIC/EXACT` σ-band claim, §1.1 R4) · `:272` (`all 64`) · `:308` (`upstream_cx_total==64`) · `:312` (`ref=64`) · `:364` (`→ 64`). ⚠️ `:405`'s `64` is `ParseUint`'s bit-size — **CODE, do not touch**.

**`test/fixtures/0061-lb-ring-hash/driver/driver_test.go`** — ⚠️ **EXECUTABLE, not prose**: tuples at `:13`, `:23`, `:33`, `:42`, `:51`, `:59`, plus the narrating comments at `:12`, `:22`. **Each test must be re-proved to fire its own leg** (§1.1 headline table).

**`test/fixtures/0061-lb-ring-hash/expectations.yaml`** — `:5`, `:6`, `:21-22` (the σ-band claim), `:25`, `:26`, `:27`, `:28`, `:32`, `:40`. ⚠️ `:21`'s `D-S36-4` is a **token ID**, not a stale numeral.

**`test/fixtures/0061-lb-ring-hash/README.md`** — `:20`, `:21`, `:23`, `:25`, `:40`, `:45`, `:54` (σ-band), `:58`, `:66`, `:68`, `:70`, `:71`, `:80-82` (⚠️ the `< 1%` scatter claim **SURVIVES** — do not delete, and do not claim improvement), `:96`, `:102`, `:129`, `:131`, **`:143-148` — the REFUTED flake-check paragraph**.

**`internal/cluster/ringhash_test.go`** — the new measurement, placed immediately after `TestRingHash_DistinctKeysSpread` (the sibling that holds the ring FIXED; adjacency makes the contrast legible).

**`docs/envoy-go/BEHAVIOR_CONTRACT.md:1326`** — one in-place line (§6).

⚠️ **AFTER LANDING, sweep for the SHAPE of the old claim, not just its numerals** (`4`, `64`, `20/20`, "fixed ring", "overwhelmingly stable", **and `DETERMINISTIC/EXACT`**). A numeral-only sweep is structurally blind to a site falsified in the opposite direction — the phase-75 `0110/expectations.yaml` lesson, and §1.1 R4 is this row's instance of it.

---

## 11. ADR continuity — D-RHSM-ADR **[RESOLVED: YES]** — the ADR-0298 §Context DRAFT

**D-RHSM-ADR resolves YES**, and not for the constant. The reusable decision is:

> **A probabilistic differential-fixture assertion carries a DERIVED margin and is verified by a MEASURED RATE, never by a pass-count.**

**The precedent for a process/discipline ADR is established**, not novel: ADR-0045 (phase-split discipline), ADR-0106 (family-expansion shape) and ADR-0288 (*"a navigation/correctness move, NOT a phase"*) are all rulings on how the project works rather than on how a proxy behaves.

Per **ADR-0044-as-used** (⚠️ ADR-0044 does **not** itself contain that discipline — ADR-0297 §Context ¶8 measured the misattribution), **ADR-0298's §Context lands at THIS SPEC commit**: the DECISIONS tail flips ADR-0297 → **ADR-0298**, and the §Context append **IS** that tail flip. §Decision + §Consequences append **IN PLACE** at the phase-76 IMPL, **no renumber**; next-free becomes **ADR-0299**.

**Block shape** (mirroring ADR-0295/0296/0297): heading · a `> **STATUS: PROPOSED …**` blockquote · **exactly ONE** `### Context (drafted at the phase-76 SPEC)` · N paragraphs · the italic footer `*(§Decision + §Consequences land at the phase-76 IMPL.)*`. **NO `### Decision`, NO `### Consequences`, NO `---` separator** — the last `^---$` is at `DECISIONS.md:17020`; separators stopped in the ADR-0289 era. The IMPL rewrites `PROPOSED` → `COMPLETE` and **RETAINS the footer**, appending after it.

**Verify-target after the append:**
```sh
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Context'       # 1
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'      # 0
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Consequences'  # 0
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^\*(§Decision'      # 1
```

⚠️ **ADR-0298 carries NO whole-file grep count.** That species has self-falsified in ADR-0296 ¶3, ADR-0297 ¶7 **and** ADR-0297 ¶9 — three times in two consecutive ADRs — and at the phase-76 BRAINSTORM it escalated from a wrong *number* to a **flipped termination-sentinel check**. Every count in the draft is either scoped to a single line, or stated as a property with no number.

⚠️ **ADR-0298 claims NO family ordinal** (§1.1 R1): the Load-balancing family was declared **CLOSED at row 54**, and a maintenance row does not extend a charter.

---

## 12. Exit — counts + expectations at SPEC-DONE

**Re-run MECHANICALLY in the stage worktree; never copied.** At this close (docs-only — **ZERO production `.go`, ZERO test `.go` in the SPEC commit**), every count is UNCHANGED:

| axis | value at this close | command | phase-76 IMPL delta (anticipated) |
|---|---|---|---|
| differential fixtures | **119** | `ls -d test/fixtures/[0-9]*/ \| wc -l` | **+0** (`0061` EDITED) |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | **+0** |
| stat surface | **1205** | ⚠️ **NO mechanical command — documentary, rides TWO recorded ledger gaps** | **+0** (delta IS checkable) |
| BackendKind tail | **38** | `fixture.go:614` — ⚠️ a TAIL VALUE; the file declares **39** constants (0-38) | **+0** |
| go.mod modules | **2** (lineage figure; the single `go.mod` requires **67**) | — | **+0** |
| DECISIONS tail | **ADR-0298 PROPOSED** (was ADR-0297 COMPLETE) | `grep -oE '^## ADR-[0-9]+' … \| tail -1` | completes at the IMPL |
| next-free ADR | **ADR-0299** | `grep -c '^## ADR-0299'` ⇒ 0 | — |
| production `.go` files | **0 touched** | — | **+0** |

**SPEC commit file set** (the phase-75 `2b5eff0a` precedent): `DECISIONS.md` (§Context append) + `STATE.md` + this `SPEC.md` + `next-prompt.txt`. **ROADMAP BYTE-UNTOUCHED; row 76 STAYS `in-progress`.** **BEHAVIOR_CONTRACT BYTE-UNTOUCHED at this SPEC** — its one owed line lands at the IMPL (§6).

**Sentinel, re-run MECHANICALLY at this stage's open — it does NOT fire and `stop` was NOT created:**
- **(1)** prints **`NOT DONE: row 76`** — live since the BRAINSTORM, and it stays live until the phase-76 IMPL.
- **(2)** prints **3** live `candidates:` sentences (HTTP/3, xDS, Observability). ⚠️ **This row narrows NOTHING** (§8) — it stays 3 at the IMPL too.
- **(3)** prints `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`.

⚠️ **RE-RUN ALL THREE AFTER THIS STAGE'S EDITS LAND.** This SPEC does not touch `ROADMAP.md`, which removes the exposure but not the obligation.

---

## 13. Adversarial-pass record — what was EXECUTED and what was NOT

**EXECUTED (agents in isolated worktrees with private scratch, plus controller re-derivation):**
- The fixture run at `sourceIPs=10` **and** at `16`, against a live `envoyproxy/envoy:contrib-v1.37.2` reference, with the stat values read out via a **discriminating break** rather than inferred from a green PASS.
- All ten `LocalAddr` binds `127.0.0.2`-`.11`, 10/10.
- The collapse rate over **1 000 000** trials against the real ring builder, plus a controller Monte Carlo over 200 000 trials/K with an independently-constructed model.
- The measurement test: green arm, break-β arm, and a **frozen-ring break that MEASURED the measured leg's vacuity**.
- The linkage gate, **negative- and positive-controlled**.
- `driver_test.go`'s failure and the two vacuous tests, in the committed worktree.
- `gofmt`, `go vet`, `golangci-lint`, `go test ./internal/cluster/ -race` (green — no sign of the known outlier flake on this tip).

**NOT EXECUTED — carried as UNVERIFIED so the PLAN inherits no false confidence:**
- **The FULL differential suite has not been run at `sourceIPs=16`.** Only `-run 'TestDifferential/0061'` was. The full-suite wall-clock impact and any cross-fixture port interaction are **unmeasured**.
- **Break α has NOT been run** — it needs the production edit the SPEC does not land.
- **No `-race` run over `./test/differential/`** at the new constant.
- **The `BEHAVIOR_CONTRACT:1326` edit has not been applied**, only specified.
- **`127.0.0.12`-`.17` have NOT been bind-tested** — only `.2`-`.11` were, at the superseded K=10. ⚠️ **The K=16 revision moved the range and the loopback probe was not re-run over the extension.** Expected trivially fine on Linux (all of `127.0.0.0/8` is loopback) but it is **one `go test` away and must not be assumed** — the BRAINSTORM said exactly that about `.6`-`.11`, and it was right to.

**A CONTROLLER SELF-CORRECTION, recorded not quietly dropped:** this SPEC first derived `sourceIPs = 10` from a policy it invented here, and the arithmetic was right. The premise was wrong — the project had already ruled on this assertion shape twice, and the answer was 16 (§3.2). ⇒ **before deriving a constant, grep for a sibling that already derived one.** This is the same failure the phase-76 BRAINSTORM recorded against itself in a different key: a claim's provenance is not evidence about the claim, and a fresh derivation is not evidence that no derivation exists.
