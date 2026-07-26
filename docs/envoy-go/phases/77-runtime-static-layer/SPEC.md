# SPEC 77 — the bootstrap `layered_runtime` **static-layer** consumer (the **FIRST** Runtime-+-hot-restart-family row — the family OPENS, and this ADR claims the family's **FIRST ordinal**): lift a boot-REJECT of a config the reference accepts, land an `internal/runtime` `Snapshot` in the phase-00 placeholder directory, and register **two** gauges — **+2 stats (1205 → 1207) / +1 fixture (`0118`, port `10118`) / +0 packages / +0 modules / +0 fuzzers / +0 BackendKinds / +0 new PUBLIC surface**

*Stage: SPEC. Lifecycle-state 1 → 2. ROW 77 STAYS `in-progress`. `ROADMAP.md` BYTE-UNTOUCHED. Docs-only: ZERO production `.go`, ZERO test `.go` in the SPEC commit.*

---

## 1. Purpose / Mission

Today `internal/bootstrap/bootstrap.go:568-570` **boot-REJECTS any bootstrap containing the key `layered_runtime`** — a raw-YAML generic-map pre-check that fires on mere key presence, **before** `protojson.Unmarshal`. The reference accepts that config and serves it. Phase 77 lifts the reject **for the `static_layer` arm only**, lands an `internal/runtime.Snapshot` (recursive Struct → dotted-path flattening, distinct-key **UNION** across layers, precedence-collapsed), registers `runtime.num_keys` + `runtime.num_layers`, and pins the semantics cross-side with one new differential fixture.

### 1.1 ⚠️ THE HEADLINE — THE REJECT ROSTER'S FOUNDING PREMISE IS REFUTED, AND THE ROW IS NOW A *DEPARTURE*, NOT A PARITY FIX

The BRAINSTORM (§6), `STATE.md` and the router all describe the roster as **"three unsupported oneof arms"** — `disk_layer`, `admin_layer`, `rtds_layer` — in a framing that presupposes the reference rejects them and envoy-go is catching up. **EXECUTED against `envoyproxy/envoy:contrib-v1.37.2` (image id `7edd5b0fd763`, matched to the `ENVOY_TARGET.md` pin): the reference ACCEPTS all three.**

| arm | briefed | measured — `--mode validate` rc | measured — live boot |
|---|---|---|---|
| `disk_layer` | reference rejects | **0**, `configuration '…' OK` | **boots, `/ready`=200**; with a real dir it **loads keys** (`num_keys=1`, `num_layers=1`) |
| `admin_layer` | reference rejects | **0** | **boots, `/ready`=200**; `layers:["L1"]`, `num_layers=1` |
| `rtds_layer` | reference rejects | **0** (with `node` + a static non-EDS cluster) | **boots** (stays `PRE_INITIALIZING` with no xDS server; container alive) |

⚠️ **The briefed "rtds fails validate" observation is an ARTIFACT and must not be re-inherited.** The first failure was `type.googleapis.com/envoy.service.runtime.v3.Runtime: node 'id' and 'cluster' are required` — a **node** error. Adding `node` moved it to `ApiConfigSource must have a statically defined non-EDS cluster: 'xds_cluster' does not exist` — a **missing-cluster** error. Adding the cluster ⇒ **rc=0**. **Neither failure was a runtime-layer rejection at any point.**

**What this changes.** The row's stated merit is *"envoy-go boot-REJECTS a config the reference accepts and serves."* That remains true and remains the reason to take the row. But phase 77 **fixes it for one arm and deliberately PRESERVES it for three**. Those three rejects are **envoy-go DEPARTURES**, not parity, and §6 records them as such with their own justification. This is still the right call — silently accepting `rtds_layer` would mean **a config asking for DYNAMIC runtime quietly gets STATIC values**, which is the `reference_lifted_reject_hidden_enforcement` failure mode in its most severe form — but a SPEC that claimed parity here would be documenting a departure as a fix.

### 1.2 BRAINSTORM drift ledger — RE-DERIVED, REFUTED, and newly found

#### CONFIRMED by controller re-derivation (not copied)

- The reject sites: `bootstrap.go:565-567` (`dynamic_resources`, **stays byte-untouched**) and **`:568-570`** (`layered_runtime`), inside the raw-YAML pre-check block **`:558`-`:570`**, consumed at `:571` (`json.Marshal(generic)`), with `protojson.Unmarshal` at `:577`.
- `internal/runtime/doc.go` exists — **241 bytes, 5 lines, package clause only**. `go list -deps ./internal/runtime` returns **exactly one line** (itself) — a leaf with **zero** dependencies, not even stdlib. ⇒ **+0 new packages** stands.
- `go list ./internal/... | wc -l` ⇒ **73**. `internal/bootstrap`'s envoy-go deps are **`{internal/stats, internal/accesslog}`** ⇒ adding `internal/runtime` is a third, cycle-free.
- **Check (1)'s blind spot, re-derived at THIS tip (never copied):** **109 data rows / 105 matched / exactly FOUR misses** — `:31` (`| 00 | bootstrap | — |`, em-dash in the "after" cell), `:35` (`| 04 | http-1.1 |`, DOT in the slug), `:83` / `:84` (`| 28.1a |`, `| 28.1b |`, LETTER-suffixed ids). All four are `done`; **row 77 is among the MATCHED**.
- The six `runtime_key` parse-reject arms and the identifier collision (§5).
- ADR-0089's two deferral rows: **`DECISIONS.md:3543`** (`POST /runtime_modify`) and **`:3550`** (`/runtime`).
- The three `BEHAVIOR_CONTRACT.md` "rejects still stand" sites: **`:906`, `:926`, `:968`** — all three claimed line numbers CORRECT.
- `NamePattern` (`internal/stats/registry.go:48`) accepts both new names — **EXECUTED with negative controls** (§7).

#### REFUTED / CORRECTED

- **R1 — the three oneof arms are ACCEPTED by the reference** (§1.1). The largest correction in this SPEC.
- **R2 — PGV enforces only TWO of the four "PGV arms".** Empty `name` and missing `layer_specifier` are genuinely PGV (`Proto constraint validation failed (BootstrapValidationError.LayeredRuntime: … RuntimeLayerValidationError.Name: value length must be at least 1 characters)`). **Duplicate layer name** (`Duplicate layer name: L1`) and **>1 admin layer** (`Too many admin layers specified in LayeredRuntime, at most one may be specified`) are **hand-written runtime-loader checks** — their messages carry no `ValidationError` structure and no proto debug-dump.
- **R3 — the reference's error text is NOT byte-pinnable for two of the classes, so no cross-side wording assertion is possible.** Every PGV message is preceded by a proto DebugString whose redaction marker **rotates across runs** (`goo.gle/debugstr` → `goo.gle/debugonly` → `goo.gle/debugproto`), and the unknown-field message varies in **internal whitespace AND in its `near L:C (offset N)` numbers** (`1:73`, `1:164`, `1:73` on three runs of the *same file*). Only the **three hand-written** messages are byte-stable (3/3 identical). ⇒ envoy-go pins its **own** wording internally; it never compares wording cross-side.
- **R4 — the ">1 admin layer" roster arm is DEAD and is DROPPED.** If envoy-go rejects any layer carrying `admin_layer`, a *second* admin layer can never be reached. Carrying the arm would ship an untestable roster row. Recorded, not silently omitted.
- **R5 — the "+9 full name-parity with seven PERMANENTLY-ZERO names" option DOES NOT EXIST.** Measured across all three chartered arms: `runtime.load_success` = **1** and `runtime.override_dir_not_exists` = **1** in *every* arm, including the `layered_runtime`-absent arm. So +9 would be five zeros plus **two names envoy-go would have to emit non-zero values for** — i.e. implement snapshot-load accounting and override-dir probing it does not have. **D-RSL-STATS resolves +2 on evidence, not on preference** (§7).
- **R6 — "NINE `runtime.*` names in EVERY arm" is TRUE for the three chartered arms and FALSE as a universal claim.** An `rtds_layer` arm emits **22** (adding `control_plane.*`, `init_fetch_timeout`, `update_*`, `version`, `version_text`). Immaterial here — envoy-go rejects `rtds_layer` — but the universal form must not be carried forward.
- **R7 — `reference_protojson_null_decodes_to_nil` DOES NOT APPLY to this row.** EXECUTED: `k.null: null` inside a `static_layer` yields a **non-nil** `*structpb.Value` whose kind is `*structpb.Value_NullValue`; only `AsInterface()` returns `nil`. An implementer walking `st.GetFields()` **will see the key**. The memory's nil-decode case concerns a *message-typed* field, not a Struct member. The BRAINSTORM's pairing of the null arm with that memory is withdrawn.
- **R8 — a SECOND test site, in a SECOND package, that no phase-77 document names.** `validate/validate_test.go:65-79` `TestBootstrap_ReusesLoad_RejectsLayeredRuntime` is the public-package sibling of `internal/bootstrap/bootstrap_test.go:82-96`. **Both** use the fixture `name: static_layer` + `static_layer: {}` — **exactly the arm this row legalizes** — so **both flip to `err == nil` and die at `t.Fatal`.** The BRAINSTORM named only the first. Repo-wide census: `layered_runtime` occurs in exactly **three** `.go` files (`bootstrap.go` `:548`/`:568`/`:569`, `bootstrap_test.go`, `validate_test.go`) and **zero** YAML fixtures.
- **R9 — the asymmetry claim is CONFIRMED and it is worse than stated.** `TestLoad_RejectsDynamicResources` asserts the `"bootstrap: "` prefix (`bootstrap_test.go:74`) **and** `Contains("dynamic_resources")` (`:77`). `TestLoad_RejectsLayeredRuntime` asserts **only** `Contains("layered_runtime")` (`:93`). **Neither** `validate/` sibling asserts the prefix. So of the four tests guarding these two rejects, **one** checks the prefix.
- **R10 — `internal/bootstrap` has ZERO named reject constants and ZERO `ByteStable` tests.** All **47** `fmt.Errorf("bootstrap: …")` arms are inline. The BRAINSTORM's *"as byte-stable named constants **per the landed discipline**"* points at a discipline landed in **filter** packages, not this one. This row therefore **introduces** that discipline into `internal/bootstrap` for the first time, scoped to its own arms (§6). Named as a cost the 9-11 floor did not price.
- **R11 — `FuzzBootstrapLoad` does not assert the invariant its own comment states.** `fuzz_test.go:78-81` is `_, _ = Load(bytes.NewReader(data))` — both returns discarded. The comment claims *"either `(*Bootstrap, nil)` or `(nil, err starting with "bootstrap: ")"*; nothing checks it. **It is a panic-only guard and must not be cited as a prefix-invariant gate.** Independently found by the controller and by the Go-execution agent.
- **R12 — ADR-0278's heading is `DECISIONS.md:16670`, NOT `:16672`.** `:16672` is the `**§Context.**` body line. The wrong cite is carried by the BRAINSTORM §1.5 **and** by `STATE.md`.
- **R13 — the next-free reference port for THIS fixture is `10118`, not `10450`.** The router's 10450 is a correct *max+1* figure (max in use **10449**, `0113/driver/driver.go:115`) but the project does not allocate by max+1 — ports are **family-banded**, and the dominant, most-recent convention is **`10<fixture index>`**: `0114`→10114, `0115`→10115, `0116`→10116, `0117`→10117. `10118` is verified free. 10450 belongs to the TLS/SDS band and this is not a TLS fixture.
- **R14 — `expectations.yaml` is 100% comment prose, read by ZERO Go code.** `grep -rn 'expectations.yaml' --include='*.go' .` finds only doc-comments. ADR-0019: *"the driver is the enforcer; this file is documentation."* The live config pair is `envoy.yaml` + `envoy-go.yaml`.
- **R15 — a fixture needs a THIRD registration gate the BRAINSTORM does not mention.** Directory + `RegisterFixture` is **not** enough: `test/differential/runner_test.go` carries **119** blank imports, one per fixture dir. Without the blank import, `init()` never runs, the `DriverRegistry` lookup misses at `runner_test.go:192`, and the runner **`t.Skipf`s — silently green**.
- **R16 — the `var _ fixture.StatsAsserter` tripwire population is 84, not 3.** The standalone `var _` form appears **3** times; **84** sites assert it in total (most inside a `var (…)` block, which the `var _`-prefixed grep cannot see). `STATE.md`'s *"only the THIRD fixture repo-wide to carry it"* is true only of the standalone spelling. Fixtures defining an `AssertStats` method: **69** under `test/fixtures/*/driver/driver.go`, **83** under `--include='*.go' test/fixtures/` — the 14-fixture gap is the `inputs/driver.go` HTTP-filter family. ⚠️ **Both figures are stated with their commands; neither is "the" count** (`reference_a_drift_correction_is_itself_a_claim`).
- **R17 — there is NO `Phase 76` stat-ledger entry.** The ledger chain in `BEHAVIOR_CONTRACT.md` runs `46.1b (:4996) → 47.1 (:4998) → 51 (:5000) → 74 (:5002) → 75 (:5004)`. The Phase 77 line follows **Phase 75** directly. Also: `1205` occurs at **three** lines — `:831`, `:847`, `:5004` — and only `:5004` is a ledger entry; ⚠️ **`:847` additionally carries a stale *"115-dir"* differential figure against an actual 119.**
- **R18 — the `9-11` task figure is superseded upward.** With the reject roster enumerated arm-by-arm and R8's second test package folded in, §10 derives **11-13**. The BRAINSTORM said 9-11 is *a floor*; it was right, and this is the re-derivation it asked for.
- **R19 — the flattening-termination rule recorded by the BRAINSTORM, `STATE.md` AND the router is REFUTED as a rule.** *"A Struct carrying `{numerator, denominator}` terminates flattening"* is true of that instance and wrong three ways: the trigger is **either name alone**, it is **purely lexical and case-sensitive** (`{Numerator, Denominator}` recurses), and it performs **zero validation** of the values. **Field count is irrelevant** — `{foo: 1}` recurses while `{numerator: 25}` terminates. ⚠️ **An implementation that parses a `FractionalPercent` here rejects configs the reference accepts.** Fifteen arms, 3× each (§3.3.2).
- **R20 — a SECOND termination case exists that NO prior document records: the EMPTY STRUCT.** `e: {f: {}}` ⇒ **1** key `e.f`; `e: {}` ⇒ **1** key `e`. An implementation with one termination branch is wrong, and the BRAINSTORM's three-arm pin set **cannot detect it** — §8.3 adds arm **D**.
- **R21 — the within-layer key-collision winner is NON-DETERMINISTIC**, ~40/60 across 18 fresh processes with identical config bytes (§3.3.1). ⚠️ **A single-run probe reports clean last-declared-wins — an artifact.** Only the repeat count refutes it. Cross-layer collisions *are* deterministic (later layer wins, 4/4).
- **R22 — the precision hazard is INVERTED and NON-MONOTONIC.** The obvious guess (large integers lose precision through a `double`) is **REFUTED**: `9007199254740993` renders **exactly**, because out-of-int32 values are stored as `string_value`. The loss hits **small, in-int32-range** integers via 6-significant-digit `%g` — `2147483647` → `'2.14748e+09'` while the *larger* `2147483648` is exact — and the rendering is **NON-INJECTIVE** (`1000000` and `1000001` both → `'1e+06'`). Floats are safe; integers are not (§3.3.4).
- **R23 — the `goo.gle/debug*` randomization leaks into BOOT-ERROR MESSAGES, not just `/runtime` bodies.** Measured in the null-value failure text. ⇒ the reference's boot-error strings join its PGV and unknown-field messages as **not byte-assertable** (R3).

### 1.3 SPEC-time verification record — what was EXECUTED and what was NOT

**EXECUTED** (four agents on disjoint remits with **PRIVATE scratch**; two ran live reference probe fleets against the pinned image with a **fresh container per arm** on a bridge network with published ports; plus controller re-derivation of every load-bearing claim):

- The full `layered_runtime` reject roster arm-by-arm on the reference, in **both** `--mode validate` and live boot, with a **positive control** (ten arms returning rc=0 in the same session, so the gate is demonstrably not stuck-red) and **two negative controls** (a PGV violation and an unknown top-level field, both rc=1).
- The `runtime.*` stat census across three arms, with the `/stats/prometheus` line shapes.
- The Go-side unmarshal behavior of **fourteen** `layered_runtime` shapes with the pre-check temporarily lifted, in a throwaway worktree, **with a vacuity control** (`total_nonsense: 1` ⇒ error) that PASSED, and the worktree restored to `git status --porcelain` **empty**.
- `NamePattern` against both new names plus four negative controls (§7).
- Reference re-verification of the present-but-zero-layers phantom layer, including a **200-vs-503 cross-product with both controls** and a post-`runtime_modify` re-read proving the synthesized layer is genuinely writable.
- The within-layer key-collision question over **18 fresh processes** (§3.3).
- Every count in §15, each with its command; negative controls observed where constructible.

**NOT EXECUTED — carried as UNVERIFIED so the PLAN inherits no false confidence:**

- **No production code was written.** No `internal/runtime` package, no `parseLayeredRuntime`, no gauges. Every design claim about envoy-go's *future* behavior is a specification, not a measurement.
- **The fixture `0118` does not exist.** Its config pair, driver and cross-side numbers are specified in §8 and have **not** been run. The reference-side expected values are extrapolated from the probe arms, not read from a `0118` run.
- **No differential suite run at this stage** — neither full nor `-run 'TestDifferential/0118'` (there is nothing to run).
- **`disk_layer` live-reload / symlink-swap behavior** was not probed; **`rtds_layer` with a responding xDS server** was not probed (the arm was left `PRE_INITIALIZING`); **`admin_layer` override behavior via `POST /runtime_modify`** was probed only in the *synthesized* case, not the *explicitly declared* case.
- **Whether `num_keys` counts `disk_layer` keys toward the same union** was measured only in the single-`disk_layer` case (`num_keys=1` with a real dir); a **mixed static + disk** config was not probed. Immaterial here — envoy-go rejects `disk_layer` — but it is not settled.
- ⚠️ **`num_keys` is NOT a static-config-only quantity.** Measured: with an explicit `admin_layer` alongside a static layer, a `POST /runtime_modify` moved `num_keys` 1 → 2 and admin keys appeared in `layer_values`. envoy-go's gauge will therefore be **boot-fixed** where the reference's is **live**. The fixture never writes, so the arms agree; a later `/runtime_modify` row inherits the divergence.

**SETTLED at this SPEC that the BRAINSTORM listed as owed:** the flattening-termination RULE (§3.3.2), the empty-struct branch (§3.3.3), the value domain and its rendering (§3.3.4), the within-layer collision (§3.3.1), and the empty-string override semantics (§3.4) — all by execution, all with repeats where a single run would have misled.

---

## 2. Non-purposes (deferred; BRAINSTORM §1.2 + §8)

Phase 77 delivers **none** of: RTDS · the `/runtime` and `POST /runtime_modify` admin endpoints (the ADR-0089 un-defer stays owed) · the disk layer and its override dir · the admin layer · lifting any of the six `runtime_key` filter parse-reject arms · honoring the silent-IGNORE runtime knobs in `bandwidthlimit`/`compressor`/`csrf`/`extauthz`/`fault`/`localratelimit`/`WeightedCluster.runtime_key_prefix` · hot restart · graceful-drain beyond phase 08.

⚠️ **`ADR-0089` stays BYTE-UNTOUCHED and that is deliberate.** It defers `/runtime` and `POST /runtime_modify` to this family; row 77 lands **neither**, so the un-defer is owed only by whichever later row does.

---

## 3. The change — the D-RSL-* docket disposed one-for-one

### 3.1 D-RSL-SCOPE **[RESOLVED — static-layer only, CONFIRMED]**

Scoping (a) from BRAINSTORM §2.2 stands: static-layer consumer + the two gauges + one fixture. (b) `/runtime` and (c) lifting the filter `runtime_key` arms stay deferred for the reasons the BRAINSTORM gives, both of which this SPEC re-verified: the `/runtime` body is unassertable cross-side (§8), and the two filter ADRs' absent-key defaults are **OPPOSITE** — EXECUTED: `adaptive_concurrency` absent ⇒ **OFF** (`compiled_config.go:353-362`, a plain `bool` zero value, with an in-code comment that itself reads *"REFUTES BRAINSTORM §2.1's 'absent enabled = ON' hypothesis"*), `admission_control` absent ⇒ **ON** (`compiled_config.go:336`, `cc.enabled = true // AMEND-4 default: true when absent`, seeded *before* the nil check at `:337`).

### 3.2 D-RSL-EMPTY **[RESOLVED BY EXECUTION — PARSE-REJECT, and for a BETTER reason than the BRAINSTORM gave]**

**Re-verified on the reference at this tip, four boot arms plus a writability cross-product:**

| arm | `layered_runtime` | `num_layers` | `num_keys` | `/runtime` `layers` | `POST /runtime_modify` |
|---|---|---|---|---|---|
| A | `layers: []` | **1** | 0 | `[""]` | **200** `[OK]` |
| B | **absent entirely** | **0** | 0 | `[]` | **503** `No admin layer specified` |
| C | `layered_runtime: {}` (no `layers` field) | **1** | 0 | `[""]` | **200** `[OK]` |
| D | exactly ONE declared static layer | 1 | 1 | `["L1"]` | **503** `No admin layer specified` |

**NEW at this SPEC — the phantom layer is not cosmetic; it is a functioning admin layer.** After `POST /runtime_modify?somekey=somevalue` on arm A: `/runtime` reports `{"entries":{"somekey":{"layer_values":["somevalue"],"final_value":"somevalue"}},"layers":[""]}`, and **`runtime.num_keys` 0 → 1**, `runtime.admin_overrides_active` 0 → 1. A second modify with two keys ⇒ `num_keys: 3`. Keys land in it; the gauge tracks them.

**⇒ `layered_runtime` present with zero declared layers IS AN IMPLICIT REQUEST FOR AN ADMIN LAYER.** That is the finding that settles the docket item, and it is stronger than the BRAINSTORM's *"awkward and arguably dishonest"*:

- envoy-go rejects an **explicit** `admin_layer` (§6, arm 2).
- The present-but-zero-declared case is the **implicit** form of the same request.
- **Rejecting the explicit form while silently accepting the implicit one would be incoherent.**

**RESOLUTION: option (ii) — PARSE-REJECT**, with a message that names the mechanism rather than the shape, so an operator learns *why* (§6, arm 8).

⚠️ **The honest cost, stated rather than buried: this re-introduces, in one narrow arm, exactly the defect class the row exists to fix** — envoy-go boot-rejecting a config the reference accepts and serves. It is accepted because the alternative is worse: matching the reference would make envoy-go report `num_layers: 1` for a layer that **cannot ever gain a key**, since envoy-go ships no `/runtime_modify`. A gauge that counts an unreachable layer is a false stat, and a false stat is harder to discover than a loud reject.

⚠️ **A Go-side constraint that CONSTRAINS the implementation and was measured, not assumed:** arms A and C are **indistinguishable after `protojson.Unmarshal`** — both yield a non-nil `LayeredRuntime` with `len(GetLayers()) == 0`. Arm B is distinguishable (nil `LayeredRuntime`). ⇒ the reject predicate is exactly `bs.GetLayeredRuntime() != nil && len(bs.GetLayeredRuntime().GetLayers()) == 0`, and **it correctly covers both spellings with one test.** The reference treats A and C identically too, so this is not a divergence.

### 3.3 D-RSL-NUMKEYS-DEEP **[RESOLVED BY EXECUTION — collision, no error, no double-count; the WINNER IS NON-DETERMINISTIC; and the FLATTENING RULE IS NOT WHAT THE LINEAGE RECORDS]**

#### 3.3.1 The collision question the BRAINSTORM asked

Duplicate leaf paths from different nesting shapes, in ONE layer:

| arm | shape | `num_keys` | `entries` | `final_value` |
|---|---|---|---|---|
| within-layer | `a.b: 1` **then** `a: {b: 2}` | **1** | `{a.b}` | ⚠️ **NOT STABLE** |
| within-layer, swapped | `a: {b: 2}` **then** `a.b: 1` | **1** | `{a.b}` | ⚠️ **NOT STABLE** |
| JSON flow spelling | same, `{"a.b":1,"a":{"b":2}}` | **1** | `{a.b}` | not stable |
| **cross-layer** | L1 `a: {b: 1}` + L2 `a.b: 2` | **1** (`num_layers: 2`) | `{a.b}` | **`"2"` deterministically, 4/4**, `layer_values: ["1","2"]` |
| control (no collision) | `a.b: 1` + `a: {c: 2}` | **2** | `{a.b, a.c}` | `1`, `2` |

**RESOLUTION: they COLLIDE into exactly ONE key.** No boot error at either the YAML or the Envoy level, and no double-count. Hypotheses (ii) *error* and (iii) *double-count* are both dead.

⚠️ **THE FINDING THAT ONLY REPEATS COULD PRODUCE — the surviving VALUE is NON-DETERMINISTIC across process starts.** Eleven fresh processes on the first arm and seven on the second, identical config bytes each time:

```
a.b:1 first   -> 2,2,1,2,2,1,2,1,2,2,1   (7x"2", 4x"1")
a:{b:2} first -> 1,1,1,2,1,2,2           (4x"1", 3x"2")
```

`num_keys` was **1 in all 18 runs**; only `final_value` flips. It is protobuf `Struct` map-iteration order under per-process hash seeding. **A single-run probe would have "shown" clean last-declared-wins — an artifact.** ⇒ within a single layer the winner is neither declaration-order nor shape-dependent, and **any fixture pinning it would flake at roughly 40/60**. Across two layers the collision resolves **deterministically to the later layer**.

**Implementation consequence:** envoy-go may pick any deterministic rule (map iteration in Go is *also* randomized, so it must pick one explicitly). It will then be **more defined than the reference**, which is acceptable and unassertable — the differential asserts `num_keys` and the key set, both of which agree.

#### 3.3.2 ⚠️ THE FLATTENING-TERMINATION RULE — the recorded characterization is REFUTED, and the real rule is LEXICAL

The BRAINSTORM, `STATE.md` and the router all record: *"a Struct carrying `{numerator, denominator}` **terminates** flattening and counts as exactly ONE."* That is **true of that instance and wrong as a rule, in three separate ways.** Fifteen arms, each repeated 3×, all stable:

| arm | shape | `num_keys` | keys | behavior |
|---|---|---|---|---|
| `{numerator: 25, denominator: HUNDRED}` | the recorded case | **1** | `k.frac` | TERMINATES |
| **`{numerator: 25}` alone** | | **1** | `k.frac` | **TERMINATES** |
| **`{denominator: HUNDRED}` alone** | | **1** | `k.frac` | **TERMINATES** |
| `{numerator, denominator, extra: 1}` | | **1** | `k.frac` | TERMINATES |
| `{numerator: 1, foo: 2}` | | **1** | `k.frac` | TERMINATES |
| **`{foo: 1, bar: 2}`** | two fields | **2** | `k.frac.foo`, `k.frac.bar` | **RECURSES** |
| **`{foo: 1}`** ← **the discriminator** | one field | **1** | **`k.frac.foo`** | **RECURSES** |
| **`{Numerator: 1, Denominator: HUNDRED}`** | capitalized | **2** | `k.frac.Numerator`, `k.frac.Denominator` | **RECURSES** |
| `{numerator: "notanumber", denominator: HUNDRED}` | | **1** | `k.frac` | TERMINATES, **boots OK** |
| `denominator: NOTANENUM` | | **1** | `k.frac` | TERMINATES, **boots OK** |
| `outer: {inner: {numerator, denominator}}` | | **1** | **`outer.inner`** | outer recurses, inner terminates |

**THE RULE, stated so an implementer can code it:** flattening **terminates at a Struct iff that Struct contains a field literally named `numerator` OR `denominator`** — lowercase, exact, **case-sensitive**, at any depth. **Either name alone suffices**; additional fields are irrelevant; **the field VALUES are never inspected or validated.**

Three refutations of the plausible alternatives, each killed by a specific arm:
- **Not a `{numerator, denominator}` PAIR match** — either name alone terminates.
- **Not "any two-field struct"** — `{foo: 1, bar: 2}` recurses. **Not "any single-field struct"** — `{foo: 1}` recurses while `{numerator: 25}` terminates. **Field count is irrelevant.**
- **Not a FractionalPercent SEMANTIC special case** — `{numerator: "notanumber", denominator: NOTANENUM}` boots cleanly. There is **zero validation**: no enum check, no numeric check. ⚠️ **An implementation that parses or validates a `FractionalPercent` here will REJECT configs the reference accepts.**

#### 3.3.3 A SECOND termination case that no prior document records — the EMPTY STRUCT

| shape | `num_keys` | key |
|---|---|---|
| `e: {f: {}}` | **1** | **`e.f`** |
| `e: {}` | **1** | **`e`** |

**An empty Struct is a COUNTED LEAF, not zero keys.** ⇒ the implementation needs **two** termination branches, not one:

```
flatten(prefix, s):
    if s has a field named "numerator" or "denominator":  emit(prefix); return
    if s has zero fields:                                  emit(prefix); return
    for name, v := range s.fields:
        if v is a Struct: flatten(prefix+"."+name, v)
        else:             emit(prefix+"."+name)
```

**Unbounded depth CONFIRMED:** `deep: {l2: {l3: {l4: 5}}}` ⇒ `num_keys: 1`, key `deep.l2.l3.l4`. Scalars and nests coexist: `m: {n: 1}` + `m2: 7` ⇒ **2**.

#### 3.3.4 The value domain and its rendering — measured, and the precision hazard is INVERTED

Accepted: int, float, string, bool. **Rejected with a hard boot failure:** `null` and list, both `Invalid runtime entry value for <key>`.

**Storage boundary is ±2³¹, NOT ±2⁵³:** integers in int32 range → `number_value`; **everything else — out-of-int32 integers, ALL floats, strings — → `string_value` holding the raw YAML text**; bools → `bool_value`.

⚠️ **The precision hazard is real but sits in the OPPOSITE place from the obvious guess, and it is NON-MONOTONIC in magnitude.** `number_value` renders through a **6-significant-digit `%g`**:

```
999999  -> '999999'      1000000 -> '1e+06'      1000001 -> '1e+06'   <-- COLLIDES
1234567 -> '1.23457e+06'      2147483647 -> '2.14748e+09'
2147483648 -> '2147483648'  (EXACT — string_value)   int64max -> exact
```

⇒ **the LARGER value renders exactly while the smaller one is lossy**, because the larger one never goes through a double. ⚠️ **And the rendering is NON-INJECTIVE** — `1000000` and `1000001` are indistinguishable through `/runtime`. Floats never suffer `%g` at all (stored as raw strings), so **the float is safe and the integer is not**. Quoting carries no information: `"hello"` and bare `hello` are byte-identical `string_value`s.

⚠️ **The `goo.gle/debug*` randomization leaks into BOOT-ERROR MESSAGES too** — the null-value failure read `error 'Invalid runtime entry value for t.null' initializing config 'goo.gle/debugproto  \n /cfg/f3_null.yaml'`. ⇒ **neither `final_value` for a terminated struct nor the reference's boot-error text is byte-assertable**; match on the stable substring only.

### 3.4 D-RSL-EMPTYSTR **[RESOLVED BY EXECUTION — `""` FALLS THROUGH; it neither shadows nor deletes]**

The BRAINSTORM framed this as *clear vs shadow*. **Measured: it is neither.**

| arm | config | `num_keys` | `final_value` | raw `entries` |
|---|---|---|---|---|
| overlap | L1 `k.same: "lower"`, L2 `k.same: ""` | **1** | **`"lower"`** | `{"k.same":{"layer_values":["lower",""],"final_value":"lower"}}` |
| **control** | L1 `"lower"`, L2 `"upper"` | 1 | **`"upper"`** | `{"k.same":{"layer_values":["lower","upper"],"final_value":"upper"}}` |
| single-layer | ONE layer: `k.empty: ""`, `k.other: "x"` | **2** | `""`, `"x"` | `{"k.empty":{"final_value":"","layer_values":[""]}}` |
| non-overlap | L1 `k.other:"x"`, L2 `k.onlyempty:""` | **2** | `"x"`, `""` | `{"k.onlyempty":{"layer_values":["",""],"final_value":""}}` |

**The control is load-bearing** — without it, `final_value: "lower"` could equally mean "override is broken in my setup". It proves the override mechanism works and that the right field is being read.

**RESOLUTION.** An empty string in the overriding layer **does not shadow** the lower value (`final_value` stays `"lower"`) and **does not delete the key** — the key is still stored, still in `entries`, and **still counts toward `num_keys`**. It is a real, counted key whose value simply never wins override resolution.

⚠️ **The warned trap is CONFIRMED and is worse than described.** `layer_values` for the non-overlap arm reads `["",""]` — **byte-identical to "neither layer defines this key"** — while the key demonstrably exists. ⇒ **the `/runtime` body cannot discriminate presence from emptiness, ever.** Any future row that asserts that body must read `final_value`, never `layer_values`. Recorded here because that row is not this one and the trap will otherwise be rediscovered.

⚠️ **A SURPRISE that a later row must not walk into: `""` has two OPPOSITE meanings inside the same subsystem.** In a **static layer** it is a stored, counted key (above). Written through the **admin** path, `POST /runtime_modify?somekey=` is a **DELETE** — measured: `num_keys` 3 → 2 and the entry removed entirely. Same character, same subsystem, opposite effect depending on the write path. Row 77 ships no write path, so it is not exposed; the `/runtime_modify` row will be.

### 3.5 D-RSL-REJECT **[RESOLVED BY EXECUTION — see §6 for the roster; the founding premise is REFUTED, see §1.1]**

**The ADR-0016 sub-question, EXECUTED rather than assumed** (the BRAINSTORM asked for exactly this). With the pre-check temporarily lifted in a throwaway worktree:

| case | outcome |
|---|---|
| valid `static_layer` `k.one: 1` | **nil** — `layers=1`, oneof `*RuntimeLayer_StaticLayer`, `k.one` → `Value_NumberValue` |
| unknown field **inside the layer entry** (`bogus_field: 3`) | **ERROR** `bootstrap: protojson: proto: (line 1:32): unknown field "bogus_field"` |
| unknown field **under `layered_runtime`** (`bogus: 1`) | **ERROR** `… unknown field "bogus"` |
| **arbitrary key inside the `static_layer` Struct** | **nil — ACCEPTED** (correct: `static_layer` is `google.protobuf.Struct`) |
| **VACUITY CONTROL** `total_nonsense: 1` | **ERROR** ⇒ the experiment is valid |

**VERDICT: ADR-0016's `DiscardUnknown:false` is CONFIRMED for unknown FIELD NAMES at any proto-typed depth, and is NOT a substitute for semantic validation.** The following all **unmarshal cleanly** and therefore need hand-written rejects:

| shape | Go-side result |
|---|---|
| `disk_layer` | ACCEPTED — `oneof=*RuntimeLayer_DiskLayer_` |
| `admin_layer` | ACCEPTED — `oneof=*RuntimeLayer_AdminLayer_` |
| `rtds_layer` | ACCEPTED — `oneof=*RuntimeLayer_RtdsLayer_` |
| **no oneof arm at all** (`name: L1` only) | ACCEPTED — `GetLayerSpecifier()` is a **nil INTERFACE** |
| **two layers with the same name** | ACCEPTED — `layers=2`, both `name="L1"` |
| `k.list: [1,2,3]` | ACCEPTED — `Value_ListValue` |
| `k.null: null` | ACCEPTED — **non-nil** `Value_NullValue` (R7) |

⚠️ **A nil *interface*, not a typed nil.** A bare `switch spec := l.GetLayerSpecifier().(type)` needs an explicit `case nil:` arm or the unset-oneof layer falls silently through `default`.

⚠️ **DO NOT PIN THE FULL `protojson` ERROR STRING.** Its `line L:C` columns are derived from the **marshaled JSON**, not the source YAML, and `json.Marshal` of the generic map **sorts keys** — so the same unknown key reported `1:32`, `1:21`, `1:74` and `1:2` across cases. It is deterministic for a fixed document (verified identical over three `-count=1` runs) but shifts whenever *any other key in the document* changes. Assert with `strings.Contains` on the field name; never `==` on the whole message.

### 3.6 D-RSL-STATS **[RESOLVED BY EXECUTION — +2, and the +9 alternative DOES NOT EXIST as described]**

See **R5**. The census, all three arms, exactly **nine** `runtime.*` names each with an identical name set:

| stat | (a) 1 layer / 2 keys | (b) absent | (c) `layers: []` |
|---|---|---|---|
| `runtime.num_keys` | **2** | **0** | **0** |
| `runtime.num_layers` | **1** | **0** | **1** ⚠ |
| `runtime.load_success` | **1** | **1** | **1** |
| `runtime.override_dir_not_exists` | **1** | **1** | **1** |
| `runtime.admin_overrides_active` | 0 | 0 | 0 |
| `runtime.deprecated_feature_use` | 0 | 0 | 0 |
| `runtime.deprecated_feature_seen_since_process_start` | 0 | 0 | 0 |
| `runtime.load_error` | 0 | 0 | 0 |
| `runtime.override_dir_exists` | 0 | 0 | 0 |

**RESOLUTION: +2.** The two names this row can compute honestly are exactly `num_keys` and `num_layers`. Of the other seven, **two are non-zero on the reference in every arm** and would require implementing snapshot-load accounting and override-directory probing; the remaining five annotate mechanisms this row does not build. The project asserts **NAMED SUBSETS** cross-side, never the full set (`reference_stats_sink_emits_used_only`), so +2 creates **no test-visible divergence**.

⚠️ **The name-set divergence is REAL and is recorded rather than denied:** envoy-go will publish **2** `runtime.*` names where the reference publishes **9**, in every arm. A future row that asserts full `runtime.*` name-set equality will fail, correctly. §13 carries this as a named deferral.

⚠️ **`runtime.load_success` and `runtime.override_dir_not_exists` are per-snapshot-load counters, not boot-once** — each `runtime_modify` bumped them 1 → 2. Anything treating `load_success == 1` as a boot health assertion breaks the moment an admin write happens. Not exposed here; named for the `/runtime_modify` row.

### 3.7 D-RSL-SPLIT **[RESOLVED — a SINGLE FLAT ROW; the ADR-0045 valve is armable and is NOT expected to fire]**

§10 derives **11-13 tasks**, up from the BRAINSTORM's 9-11 floor (R18). The modern cadence is 9 or 11 (`grep -cE '^#+ *Task [0-9]+' docs/envoy-go/phases/*/PLAN.md` gives 9 for phases 62-64, 66-68, 70-74, 76 and 11 for 65, 69, 75), so 11-13 is at the **upper edge of ordinary**, not beyond it. The natural split seam if the PLAN needs one is *(snapshot + reject roster)* / *(stats + fixture)*.

### 3.8 D-RSL-PKG **[NEW at this SPEC — RESOLVED: `internal/runtime` imports `structpb` and NOTHING else]**

The BRAINSTORM §1.5 constrained `internal/runtime` to `bootstrapv3` + `structpb` + `internal/stats`. **This SPEC tightens it to `structpb` alone**, and the reason is measured: `go list -deps ./internal/runtime` currently returns **one line** — the package has *zero* dependencies, not even stdlib.

- `internal/runtime` takes `[]map[string]*structpb.Value` (one map per declared layer, already precedence-ordered) and returns a `*Snapshot`. It never sees a bootstrap proto.
- The oneof walk, the reject roster and the two `NewGauge` calls all live in `internal/bootstrap`, which is where the roster has to live anyway.
- ⇒ `internal/runtime` stays a single-import leaf, is unit-testable without constructing bootstrap protos, and the cycle guard (`reference_xds_config_seam_transitive_cycle_guard`) is satisfied **structurally** rather than by assertion.

**Registration window, EXECUTED:** `bs.Stats.Freeze()` is at `cmd/envoy-go/main.go:356`; `Load` allocates a **fresh** `stats.NewRegistry()` per call at `bootstrap.go:580`. Registering inside `Load` is therefore pre-Freeze by construction and **cannot double-register**, because no two `Load` calls share a registry. The insertion point mirrors the two existing parse hooks exactly:

```go
result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}   // :580
if err := parseAccessLogConfigs(bs, result); err != nil { ... } // :581
if err := parseStatsSinks(bs, result); err != nil { ... }       // :584
if err := parseLayeredRuntime(bs, result); err != nil { ... }   // NEW
return result, nil                                              // :587
```

⚠️ **`parseLayeredRuntime` MUST be called UNCONDITIONALLY, not gated on `layered_runtime` presence** — the reference emits both gauge names with value **0** in the absent arm, and §8 asserts the name set unconditionally.

### 3.9 D-RSL-ADR **[RESOLVED: YES — ADR-0299, claiming the FIRST Runtime-family ordinal]** — see §14.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules, 0 new production module deps

No new framework seam. Four existing ones, used verbatim:

1. The raw-YAML generic-map pre-check block (`bootstrap.go:558-570`) — the `layered_runtime` arm is **replaced**, the `dynamic_resources` arm at `:565-567` stays **byte-untouched**.
2. `internal/stats` — `(*Registry).NewGauge` (`registry.go:92`), pre-Freeze, mirroring `internal/admin/admin.go:64` (`registry.NewGauge("server.live")`).
3. `internal/runtime` — the existing phase-00 placeholder directory (**+0 packages**).
4. `fixture.StatsAsserter` (`test/differential/fixture/fixture.go:75-77`).

**+0 go.mod modules:** `structpb` and `bootstrapv3` are already imported by production code; `go.mod` has **two** `require` blocks (`:5`, `:26`) totalling **67** (18 direct + 49 indirect) — ⚠️ a single-block grep undercounts. There is no new sub-package, so `reference_new_subpackage_pulls_transitive_module` does not bite; the PLAN must still re-check `git diff go.mod`.

---

## 5. Identifier hygiene *(collision checks RE-DERIVED repo-wide at tip — `reference_spec_drafted_identifier_collision_check`)*

**EXECUTED:** `^type Snapshot`, `^type snapshot`, `^type Layer`, `^type Flatten` ⇒ **0 declarations each** across `internal/` and `cmd/`. All **46** word-hits for `Snapshot` are in comments. ⇒ `runtime.Snapshot` and the drafted helper names are collision-free.

⚠️ **Two live hazards found, neither blocking this row:**

- **`type runtimeConfig` already exists in FOUR filter packages** — `header_mutation`, `csrf`, `localratelimit`, `fault` — and three of those (`csrf`, `localratelimit`, `fault`) are exactly the packages carrying the silent-IGNORE runtime knobs of BRAINSTORM §1.7. In those packages the name already means *"the parsed-and-discarded runtime knob config"*. A future row honoring those knobs must not reuse it.
- **`internal/admin` imports stdlib `"runtime"`** (`version.go:4`). The deferred `/runtime` endpoint row will need an import alias. **Neither `internal/bootstrap` nor `cmd/envoy-go/main.go` imports stdlib `runtime`**, so this row needs none — verified by command.

⚠️ **`parseRejectEnabledRuntimeKey` is declared TWICE with DIFFERENT values** — `admission_control/compiled_config.go:179` and `adaptive_concurrency/compiled_config.go:171`. Across the six `runtime_key` arms there are **5 distinct constant names** but **6 distinct message strings**. ⇒ **a roster keyed on constant NAME undercounts six to five; key on the message text.** CONFIRMED by command.

---

## 6. Reject roster — **the row CONVERTS as well as ADDS, and THREE arms are DEPARTURES**

**Naming convention, taken from the landed precedent rather than invented** (`internal/filter/http/{admission_control,ratelimit,wasm}/compiled_config.go`, `internal/filter/network/mongoproxy/config.go`): unexported `const` strings, identifier `parseReject<Field><Condition>`, passed to `errors.New` (**not** `fmt.Errorf` format strings, so they compare byte-exact). Message opens with the wire name + `": "`, then the dotted snake_case proto path verbatim, lowercase, **no trailing period**; remediation as `; use <alternative>`.

⚠️ **R10: `internal/bootstrap` has none of this today** — 47 inline `fmt.Errorf` arms, zero constants, zero `ByteStable` tests. This row introduces the discipline **scoped to its own arms only**; it does **not** convert the other 46. The one conversion it does perform is the `layered_runtime` message it is replacing anyway.

| # | arm | envoy-go | reference | class |
|---|---|---|---|---|
| 1 | `disk_layer` present | **REJECT** | **ACCEPTS** (and loads keys) | ⚠️ **DEPARTURE** |
| 2 | `admin_layer` present | **REJECT** | **ACCEPTS** | ⚠️ **DEPARTURE** |
| 3 | `rtds_layer` present | **REJECT** | **ACCEPTS** | ⚠️ **DEPARTURE** — the most important of the three |
| 4 | `layer_specifier` unset | **REJECT** | rejects (PGV) | parity |
| 5 | `name` empty | **REJECT** | rejects (PGV) | parity |
| 6 | duplicate layer `name` | **REJECT** | rejects (loader) | parity |
| 7 | `static_layer` value is a LIST | **REJECT** | rejects (loader) | parity |
| 8 | `static_layer` value is NULL (`null` / `~` / bare-empty — one case) | **REJECT** | rejects (loader) | parity |
| 9 | `layered_runtime` present, ZERO declared layers | **REJECT** | ACCEPTS (synthesizes a writable admin layer) | ⚠️ **DEPARTURE** — §3.2 |
| ~~10~~ | ~~more than one `admin_layer`~~ | — | — | **DROPPED — UNREACHABLE (R4)** |

**Why arm 3 is the one that matters.** Silently ignoring `rtds_layer` means a config asking for **DYNAMIC** runtime quietly gets **STATIC** values — a silent, wrong-answer divergence rather than a loud failure. `reference_lifted_reject_hidden_enforcement` says the guard must land **ATOMICALLY** with the lift; the PLAN must not schedule the lift and the roster as separate tasks with a green gate between them.

**Byte-stability discipline.** All nine messages land as named constants pinned by a **`TestParseRejectConstants_ByteStable`** table in `internal/bootstrap`. ⚠️ **Copy the `wasm` variant, not the `admission_control` one** — `wasm/compiled_config_test.go:160` guards `len(cases)` so that silently deleting a row fails; `admission_control` has no such guard.

⚠️ **All nine messages MUST carry the `"bootstrap: "` prefix**, and the prefix must be asserted — R9 shows only one of the four existing guards checks it, and R11 shows the fuzzer does not. §10 T-fuzz upgrades the fuzz body to assert it, which is a two-line change that closes both gaps at once.

⚠️ **No cross-side wording assertion is possible or attempted** (R3). envoy-go pins its own wording internally; the reference's is non-deterministic for two of the classes.

**Values that MUST be ACCEPTED** (rejecting any is a new divergence): int, negative int, float, quoted string, bare string, `true`, `false`, numeric string, **empty string**, and nested maps (flattened, parent emits no entry).

---

## 7. Stat surface **+2** (1205 → 1207) · Fuzz **+0**

`runtime.num_keys` and `runtime.num_layers`, registered unconditionally at `Load` time (§3.8).

**EXECUTED against `NamePattern` (`internal/stats/registry.go:48`, `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`), with negative controls:**

| name | valid |
|---|---|
| `runtime.num_keys` | **true** |
| `runtime.num_layers` | **true** |
| `runtime.` | false ✓ |
| `9runtime.num_keys` | false ✓ |
| `runtime.num-keys` (the hyphen `getOrRegister` panics on) | false ✓ |

⚠️ **A NEW finding about the validator itself, not about these names: `runtime..num_keys` — an EMPTY INTERIOR SEGMENT — is VALID.** The pattern rejects a *trailing* empty segment and its doc comment (`registry.go:44-47`) explains dots as "separators, not terminators", but it silently permits an interior empty. **This row is not exposed** (it derives no stat name from any runtime key), but it sharpens the hazard BRAINSTORM §2.7 flagged for a later row from *"the validator will catch it"* to **"the validator catches a hyphen and does NOT catch `a..b`"** — and runtime keys are arbitrary operator-chosen dotted strings. §13 carries it.

⚠️ **The 1205 total is DOCUMENTARY with no mechanical counting command, and the ledger chain is discontinuous in TWO recorded places.** **Assert the DELTA, never the total.** The delta is checkable by a `TestNoNewStat*`-class guard asserting exactly two new registrations.

**Panic sites the implementation must stay clear of** (all in `internal/stats/registry.go`): invalid name `:117`, duplicate registration `:107`, frozen registry `:129`. ⚠️ `getOrRegister` (`:177`) itself contains **no** panic — it delegates to `checkName`; the type-mismatch panics live in the callers (`:165`, `:212`).

**Fuzz +0.** A corpus seed is not a `func Fuzz` (`reference_fuzzer_count_docs_drift`). No existing seed contains `layered_runtime` and `internal/bootstrap/testdata` does not exist, so the seed is a new `f.Add` line at `fuzz_test.go:66-75`.

---

## 8. Differential fixture — **`0118-runtime-static-layer`, +1 (119 → 120), port `10118`**

### 8.1 It must NOT go through `ProbeAdmin` — RE-VERIFIED, and now for THREE reasons

`compareAdminResponses` (`runner_test.go:1394`) compares the body **byte-exact** (`// Body: byte-exact.` + `CompareBytes`). The reference `/runtime` body cannot survive it:

1. **JSON key order randomized PER REQUEST** — three consecutive GETs gave three distinct md5s with an *identical* sort-keys md5.
2. **The Struct debug-string prefix randomized PER PROCESS** — `goo.gle/debugstr` / `debugonly` / `debugproto`, stable within a process, flipping across a `docker restart`.
3. **NEW at this SPEC — a `static_layer` value that is an empty map renders as a leaked, non-deterministic proto DebugString** (`"goo.gle/debugproto   \n"`) *and counts toward `num_keys`*. A reference wart; envoy-go must count the key and must **not** replicate the garbage value.

⇒ the fixture asserts **STATS ONLY**, via `fixture.StatsAsserter` over `/stats/prometheus`, on a **named subset** — the phase-75 `0110` Shape-A precedent. All three randomizations contaminate the **body only**, never the gauge values.

⚠️ **A FOURTH, independent reason to stay stats-only, and it is the strongest:** §3.3's within-layer collision winner is **non-deterministic across processes**, and §8.4's number→string formatting diverges between the YAML and JSON front-ends. **Any fixture asserting a `final_value` is unrunnable.** `num_keys` and `num_layers` are immune to all four.

### 8.2 The asserter's shape

Clone `0110`'s `AssertStats` + `scrapeProm` (`test/fixtures/0110-tls-require-client-cert-false/driver/driver.go:653-773` and `:780-833`) verbatim in structure:

- Signature **exactly** `AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` — dispatched at `runner_test.go:1347-1349`, **first addr = REFERENCE, second = SUBJECT**. ⚠️ **The dispatch is a silent type assertion with no `else`, no log and no skip** — a signature typo makes `ok == false` and the whole leg vanishes **green**. The PLAN must prove the asserter *runs*, not merely that it passes.
- `scrapeProm` keys on the metric **name** with the label set **stripped entirely**, and collides by **summing** — which is what makes it address-agnostic.
- Scrapes are **preconditions ⇒ `Fatalf`**; every property is a separate **`Errorf`** (`reference_fatalf_makes_assertions_unreachable`).
- The **absent check is separate from the value check and `continue`s**, so an unregistered gauge cannot pass as `0 == 0`.
- ⚠️ `fixture.TB` has **exactly** `Errorf` / `Fatalf` / `Helper` — **no `Logf`** (`reference_fixture_tb_has_no_logf`). Diagnostics go through `log.Printf`.

**Names asserted (unconditionally — measured present in every arm, including the absent arm at value 0):** `envoy_runtime_num_keys`, `envoy_runtime_num_layers`. **Prometheus line shape, measured: NO labels at all** — `envoy_runtime_num_keys{} 2`. ⚠️ `grep -c` on either name returns **2** per scrape (the `# TYPE` line plus the sample); the parser skips `#` lines, so this is a hazard only for a hand-rolled grep gate.

### 8.3 The minimum honest pin set — a single-layer config is NOT sufficient

With one layer, the *union* and *per-layer-sum* hypotheses are numerically identical for every input. The config must carry all three discriminating arms in one bootstrap:

| # | arm | pins | correct | what a naive impl gives |
|---|---|---|---|---|
| **A** | **2-layer overlap** (same key in both) | UNION vs per-layer SUM; the only arm where `num_keys ≠ num_layers`, so it also catches a **transposed-gauge** bug | counts **once** | 2 |
| **B** | **nested key** `a: {b: {c: 1, d: 2}}` | unbounded-depth leaf flattening | **2** | 1 |
| **C** | **`numerator`/`denominator`-named struct** | the **lexical** termination rule (§3.3.2) | **1** | 2 |
| **D** | **empty struct** `e: {f: {}}` | the **SECOND** termination case (§3.3.3) — key `e.f` | **1** | **0** |

⚠️ **Arm D is NEW at this SPEC and no prior document names it.** An implementation that treats an empty Struct as "nothing to emit" gives **0** where the reference gives **1**, and arms A-C cannot detect that — every one of them has non-empty leaves. Without D the fixture is blind to a whole termination branch.

⚠️ **Arm C must use a LOWERCASE, SINGLE `numerator` (or `denominator`) field to be maximally discriminating.** Spelling it as the full `{numerator, denominator}` pair still passes, but it fails to distinguish the real lexical rule from a pair-match implementation — the very confusion §3.3.2 refutes. A single lowercase `numerator` alongside an unrelated sibling field (`{numerator: 25, foo: 2}` ⇒ **1**) is the arm that discriminates, because a pair-matching implementation gives **2**.

**A fifth arm is available and cheap, and the PLAN should consider it:** `n: 2147483647` renders `'2.14748e+09'` on the reference where any Go `strconv`-based implementation renders `'2147483647'` (§3.3.4). It is **not** assertable by this fixture (which asserts no values) but it is the highest-value arm for whichever row first serves `/runtime`. Recorded in §13, not scheduled here.

⚠️ **Each leg must be proved to fire its OWN assertion at the IMPL, not assumed to** (`reference_ordered_assertion_legs_vacuous_on_constant_change`, `reference_probe_must_discriminate`). ⚠️ **The four arms live in ONE bootstrap and all feed ONE gauge**, so a break must move `num_keys` by a distinguishable amount per arm — the PLAN must choose the per-arm key counts so that no two arms' breakage produces the same total.

### 8.4 Fixture-construction constraints, measured

- **`BackendCount()` must be ≥ 1** — enforced at `runner_test.go:240-243` (`t.Fatalf`). This fixture drives no backend traffic; `return 1` with the default `TCPEcho` kind is the minimum viable shape (`0110`'s exact posture). **+0 BackendKinds.**
- **THREE registration gates, all required (R15):** the `NNNN-…` directory (`discoverFixtures`, `runner_test.go:1460-1495`) · `fixture.RegisterFixture` from `init()` with the name **equal to the directory name** (looked up at `:192`, miss ⇒ **`t.Skipf`, silently green**) · **a blank import in `runner_test.go`** (currently 119, one per fixture).
- **`expectations.yaml` is prose only (R14)** — 100% `#` comments, read by no Go code. Include it for consistency with the other 95 fixtures; do not encode behavior in it.
- **`envoy.yaml` + `envoy-go.yaml`** are the live config pair, read through a **per-fixture-private** `mustReadFixtureFile` helper (each fixture copies it; it is not shared).
- ⚠️ **Choose fixture keys whose values are small integers or short strings.** Measured: the reference renders in-int32-range YAML integers as 6-significant-digit `%g` (`2147483647` → `"2.14748e+09"`, `1000000` → `"1e+06"`), out-of-int32 integers and all YAML floats **verbatim as strings**, and the YAML and JSON front-ends **disagree** (`4.0` → `"4.0"` vs `"4"`). The fixture asserts no values, but a config using large or float literals would make any future value assertion unreproducible.

---

## 9. Behavior-contract edit map

| site | current | edit |
|---|---|---|
| `BEHAVIOR_CONTRACT.md:906` | *"`dynamic_resources` / `layered_runtime` rejects still stand"* (phase 60) | amend: the `layered_runtime` reject is **partially lifted at phase 77** — `static_layer` only; `dynamic_resources` **unchanged** |
| `:926` | same claim (phase 65) | same amendment |
| `:968` | same claim (phase 66) | same amendment |
| stat ledger, after **`:5004` (Phase 75)** | — | **NEW** `**Phase 77 — 1205 → 1207 (+2) …**` line ⚠️ **there is NO Phase 76 entry (R17)**; the new line follows Phase 75 **directly** |
| `:831`, `:847` | narrative `1205` | update to `1207` ⚠️ **`:847` also carries a stale *"115-dir"* against an actual 119** — fix or explicitly leave, but do not silently carry |

⚠️ **A FOURTH documentary site the BRAINSTORM did not name: `DECISIONS.md:16430`.** ADR-0268 records that `validate/validate_test.go` carries *"4 reused `internal/bootstrap` reject-arm cases (dynamic_resources, **layered_runtime**, YAML syntax error, empty document)"* — a **live test roster inside an ADR**, which R8's rewrite falsifies. The IMPL must amend it or record why not.

⚠️ **`BEHAVIOR_CONTRACT.md` stays BYTE-UNTOUCHED at this SPEC.** All five edits land at the IMPL.

---

## 10. Test plan + task surface — **11-13 tasks; a SINGLE FLAT ROW** *(R18: the BRAINSTORM's 9-11 was a declared floor; this is the re-derivation it asked for)*

| # | task | gate |
|---|---|---|
| T1 | `internal/runtime`: `Snapshot`, flattening (⚠️ **TWO** termination branches — §3.3.3), union, precedence — TDD, `structpb`-only (§3.8) | unit tests covering all **four** §8.3 arms, both termination branches, the case-sensitivity arm (`{Numerator}` recurses), the invalid-value arms (`{numerator: "notanumber"}` still terminates), and the §3.3.1 collision |
| T2 | `internal/runtime/doc.go` — replace the phase-00 placeholder | `go doc` symbol-scoped |
| T3 | The **nine** reject constants + `TestParseRejectConstants_ByteStable` with the **wasm-style roster-size guard** | table green; roster-size guard proved to fire |
| T4 | `parseLayeredRuntime` + the lift, **ATOMICALLY** (`reference_lifted_reject_hidden_enforcement`) | every arm of §6 red-then-green |
| T5 | **REPLACE** `TestLoad_RejectsLayeredRuntime` (`bootstrap_test.go:82-96`) — assert the `"bootstrap: "` prefix, which it does not today (R9) | package green |
| T6 | **REPLACE** `TestBootstrap_ReusesLoad_RejectsLayeredRuntime` (`validate/validate_test.go:65-79`) — **the second package (R8)** | `go test ./validate/` |
| T7 | The two gauges + a `TestNoNewStat*`-class delta guard | delta exactly +2 |
| T8 | `FuzzBootstrapLoad`: a `layered_runtime` corpus seed **and** upgrade the body to assert the `"bootstrap: "` prefix (R11) | `-fuzz` short budget |
| T9 | Fixture `0118` — config pair, driver, README, `expectations.yaml`, **blank import (R15)** | `-run 'TestDifferential/0118' -count=1` |
| T10 | Break roster: prove each of the three §8.3 legs fires its **own** assertion | each break red, restore green |
| T11 | Prose sweep — §9's five sites + `DECISIONS.md:16430` | grep-verified |
| T12 | ADR-0299 §Decision + §Consequences **in place after the RETAINED footer**; row 77 → `done`; six-gate | ADR-0106 |

*(T2 and T11 may fold into neighbours; hence 11-13.)*

⚠️ **Known live hazards — never reflex-classify any of these as a regression.** The full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **before any assertion**, the latter as a **PANIC that aborts the whole binary**, firing **more readily under `-race`**) · `reference_sds_init_fetch_timeout_dial_budget_flake` · the pre-existing `internal/cluster` `-race` outlier flake · two still-**UNINDEXED** load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`). ⚠️ **A stage brief's flake list is not the index — the FIFTH consecutive stage at which that held.** Isolate-re-run, then state the classification **and its evidence**. ⚠️ **`0061-lb-ring-hash` is NO LONGER a live flake**; a spread failure there is now a **FINDING**.

⚠️ **Gate hygiene — the lineage's broken-gate count is EIGHT.** `gofmt -l` **never exits non-zero** (gate on OUTPUT: `[ "$(gofmt -l . | wc -l)" -eq 0 ]`) · `go doc -all` diffed RAW is not symbol-scoped and **false-positives on a correct tree** · `go doc -all <pkgA> <pkgB>` fails open with a `./` prefix (**one package per invocation**) · a **range** gate cannot detect anchor drift (**content-anchor instead**) · the `impblock` import gate has three defects · a sha256 byte-untouched roster needs a **MISSING** leg as well as a MISMATCH leg, and must be **set-differenced** against the EDIT roster — ⚠️ **`internal/bootstrap/**`, `internal/runtime/**` and `validate/**` must NOT be byte-gated for this row.** ⚠️ **A harness's exit code is not the command's** — capture the inner status (`cmd; echo "EXIT=$?" >> log`) and derive the tally from the log.

---

## 11. Edit-site roster — RE-DERIVED at tip `638ef32a`

**Production:**
1. `internal/bootstrap/bootstrap.go` — `:568-570` replaced; `:548` doc comment amended; `parseLayeredRuntime` added after `:584`; a `Runtime *runtime.Snapshot` field on the `Bootstrap` struct; the nine reject constants.
2. `internal/runtime/snapshot.go` (**NEW**, existing directory).
3. `internal/runtime/doc.go` — replace the 241-byte placeholder.

**Test:**
4. `internal/bootstrap/bootstrap_test.go:82-96` — **REPLACED**.
5. **`validate/validate_test.go:65-79` — REPLACED (R8).**
6. `internal/bootstrap/fuzz_test.go:66-75` (seed) + `:78-81` (the invariant assertion, R11).
7. `internal/runtime/snapshot_test.go` (**NEW**).
8. `test/fixtures/0118-runtime-static-layer/**` (**NEW**) + the blank import in `test/differential/runner_test.go`.

**Docs:** `BEHAVIOR_CONTRACT.md` ×5 · `DECISIONS.md` (ADR-0299 + the `:16430` amendment) · `ROADMAP.md` (row 77 → `done`) · `STATE.md` · `next-prompt.txt`.

⚠️ **`internal/bootstrap/bootstrap.go:565-567` (`dynamic_resources`) stays BYTE-UNTOUCHED.**

---

## 12. Sentinel maintenance — **this row narrows NOTHING**

The Runtime family's candidates sentence (`ROADMAP.md:213`, minted at the BRAINSTORM) lists RTDS · the two admin endpoints · disk layer · admin layer · the six `runtime_key` arms · the silent-IGNORE knobs · hot restart · graceful drain. **Row 77 delivers none of them** — it delivers the static layer, which the sentence does not name. ⇒ **check (2) stays at 4 (old matcher) / 5 (broadened) through the IMPL.**

**Sentinel re-run MECHANICALLY by the controller at this stage's open — it does NOT fire and `stop` was NOT created:**
- **(1)** prints **`NOT DONE: row 77`** — live since the BRAINSTORM, silenced only at the phase-77 IMPL.
- **(2)** prints **5** under the broadened matcher (`:187`, `:197`, `:207`, `:213`, `:221`) and **4** under the old one. ⚠️ **Both are correct; do NOT "fix" either down.** ⚠️ The Operational-tooling short-form site has drifted `:218` → **`:221`** under the BRAINSTORM's own ROADMAP edit — a live instance of `reference_a_drift_correction_is_itself_a_claim`.
- **(3)** prints `NEVER OPENED: gRPC` and `NEVER OPENED: WASM`.

⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this SPEC**, which removes the exposure but not the obligation: **re-run all three after any edit lands.**

---

## 13. Deferred items

Named here so no later stage re-derives them from scratch:

1. **The `runtime.*` name-set divergence** — envoy-go publishes 2 names where the reference publishes 9 (§3.6).
2. **`NamePattern` accepts an empty INTERIOR segment** (`a..b`) while rejecting a trailing one (§7). Exposed by any row deriving a stat name from an operator-chosen runtime key.
3. **`""` means "stored, counted, loses resolution" in a static layer and "DELETE" through `/runtime_modify`** (§3.4). The `/runtime_modify` row must handle both.
4. **`load_success` / `override_dir_not_exists` are per-load counters, not boot-once** (§3.6).
5. **The `/runtime` body cannot discriminate absent from empty** — `layer_values` renders both as `""` (§3.4).
6. **The reference's empty-map-value wart** — a garbage DebugString as `final_value`, still counted (§8.1).
7. **The documented PUBLIC IMPORT PATH does not exist** — CONFIRMED at this SPEC: `head -1 go.mod` is `github.com/pgdad/envoy-go`; the docs say `github.com/esalaine/envoy-go/validate` on **20 lines / 24 occurrences** across four docs (DECISIONS **11**, ROADMAP **4**, STATE_HISTORY **3**, BEHAVIOR_CONTRACT **2**; STATE.md **0**). ⚠️ **It is not merely a typo: `DECISIONS.md:142` is `## ADR-0006: module path github.com/esalaine/envoy-go` — an ADR that DECIDES the wrong path, never superseded.** ⚠️ **Phase 77's own BRAINSTORM propagates it.** A session copying the documented path writes code that does not compile. **Deliberately NOT fixed here** — beyond a SPEC's chartered edit set.
8. **`BEHAVIOR_CONTRACT.md:1857`'s two stale `+20` cites**, plus two more in the same file; fix by replacing line numbers with **symbol anchors**.
9. **Normalising the Operational-tooling paragraph** (`ROADMAP.md:221`) to the long form so both check-(2) matchers agree.
10. **A mechanical stat-surface count** — only *partially* constructible (non-literal names, `statroster` table fan-out, registration functions passed as **method values**); ⚠️ `NewHistogram` has never existed. **8-11 tasks**, not the small row it sounds like.
11. **The `%g` rendering divergence** — whichever row first serves `/runtime` must reproduce 6-significant-digit `%g` for in-int32-range integers and verbatim for everything else (§3.3.4). The highest-value single arm is `n: 2147483647` ⇒ reference `'2.14748e+09'` vs a Go `strconv` default `'2147483647'`. ⚠️ **The rendering is NON-INJECTIVE**, so that row cannot round-trip a value ≥ 10⁶ to verify what it set.
12. **envoy-go's `num_keys` will be BOOT-FIXED where the reference's is LIVE** (§1.3) — inherited by the `/runtime_modify` row, not by this one.
13. **Within-layer collision determinism** — envoy-go will pick an explicit rule and be *more* defined than the reference (§3.3.1). If a later row serves `/runtime`, that extra determinism becomes visible and must be documented rather than asserted cross-side.

---

## 14. ADR continuity — the **ADR-0299 §Context** DRAFT

**ADR-0299 claims the FIRST Runtime-+-hot-restart-family ordinal.** The reusable decision it records:

> **Lifting a wholesale bootstrap pre-check for ONE oneof arm converts the sibling arms from "unreachable" to "silently accepted", so the arm-lift and the sibling reject roster must land in ONE commit — and where the reference ACCEPTS a sibling, the reject is a DEPARTURE that must be recorded as one, not as parity.**

Per **ADR-0044-as-used** (⚠️ **ADR-0044 itself does NOT contain that discipline** — ADR-0297 §Context ¶8 measured the misattribution), **§Context lands at THIS SPEC commit**; **§Decision + §Consequences append IN PLACE at the phase-77 IMPL after the RETAINED italic footer**, mirroring ADR-0295/0296/0297/0298. No renumber; next-free becomes ADR-0300.

**Block shape** — heading · a `> **STATUS: PROPOSED …**` blockquote · **exactly ONE** `### Context (drafted at the phase-77 SPEC)` · N paragraphs · the italic footer `*(§Decision + §Consequences land at the phase-77 IMPL.)*`. **NO `### Decision`, NO `### Consequences`, and NO `---` separator.**

⚠️ **The separator convention was ABANDONED and a naive "append after the last `---`" would place ADR-0299 ~440 lines too early.** The last `^---$` in `DECISIONS.md` is **`:17020`**; ADR-0296 (`:17256`), ADR-0297 (`:17324`) and ADR-0298 (`:17394`) all carry **none**. **Append after `:17462` (EOF).**

⚠️ **ADR-0298's retained italic footer sits at `:17422` — BETWEEN §Context and §Decision, not at the end of the ADR.** That is the position the IMPL appends *after*.

**Verify-target after the append:**
```sh
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Context'       # 1
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'      # 0
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Consequences'  # 0
awk '/^## ADR-0299/,0' docs/envoy-go/DECISIONS.md | grep -c '^\*(§Decision'      # 1
```

⚠️ **ADR-0299 carries NO whole-file grep count.** That species self-falsified in ADR-0296 ¶3 and ADR-0297 ¶7 **and** ¶9, and at the phase-76 BRAINSTORM it escalated from a wrong number to a **flipped termination-sentinel check**. Every count in the draft is line-scoped or stated with no numeral.

---

## 15. Exit — counts + expectations at SPEC-DONE

**Re-run MECHANICALLY by the controller; never copied.** Docs-only close — **ZERO production `.go`, ZERO test `.go` in the SPEC commit**:

| axis | value at this close | command | phase-77 IMPL delta (anticipated) |
|---|---|---|---|
| differential fixtures | **119** (tail `0117-…`) | `ls -d test/fixtures/[0-9]*/ \| wc -l` ⚠️ **not equivalent to `discoverFixtures`** — the faithful predicate is `^[0-9]{4}[a-z]?-` | **+1** (`0118`) |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | **+0** (a seed is not a fuzzer) |
| stat surface | **1205** | ⚠️ **NO mechanical command; DOCUMENTARY, two recorded ledger gaps** | **+2** — assert the DELTA |
| BackendKind | **tail 38 / 39 declared constants** (`fixture.go:614`; `TCPEcho = 0` at `:137`) | ⚠️ two different numbers, **both correct** | **+0** |
| go.mod modules | **2** (phase-61.2 lineage figure) — the single `go.mod` requires **67** = 18 direct + 49 indirect, in **TWO** `require` blocks | ⚠️ **do NOT "fix" 2 to 67** | **+0** |
| internal packages | **73** | `go list ./internal/... \| wc -l` | **+0** (the directory exists) |
| DECISIONS tail | **ADR-0298 COMPLETE** → **ADR-0299 PROPOSED** at this commit | `grep -oE '^## ADR-[0-9]+' … \| tail -1` | completes at the IMPL |
| next-free ADR | **ADR-0300** after this commit | `grep -c '^## ADR-0299'` ⇒ was **0** | — |
| next-free fixture index | **0118** | numeric tail `0117` | → 0119 |
| **reference port for `0118`** | **10118** ⚠️ **NOT 10450** (R13) | family-banded; `10<index>` is the dominant convention | — |
| production `.go` files | **0 touched** | — | — |

**SPEC commit file set** (the phase-76 precedent): `DECISIONS.md` (ADR-0299 §Context) + `STATE.md` + this `SPEC.md` + `next-prompt.txt`. **`ROADMAP.md` BYTE-UNTOUCHED; row 77 STAYS `in-progress`. `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED.**

---

## 16. Adversarial-pass record

**What refuted what.** Twenty-three claims were re-derived; **R1-R23** record the corrections. The five that would have caused wrong work if carried:

1. **R1** — the SPEC would have documented three **departures** as parity, and the ADR would have recorded a false rationale.
2. **R19** — an implementer coding the recorded *"`{numerator, denominator}` terminates"* rule would have written a **pair match** (or worse, a `FractionalPercent` parse) and been wrong on `{numerator: 25}` alone, on `{numerator: 1, foo: 2}`, on `{Numerator, Denominator}`, and on every invalid-value config the reference accepts.
3. **R20** — the empty-struct branch would have been missed entirely, and **the BRAINSTORM's three-arm pin set could not have caught it**: the fixture would have shipped green with a wrong implementation.
4. **R8** — the IMPL would have shipped a red `./validate/` package, discovered only at the full-suite gate, in a package the row's own documents never mention.
5. **R13** — the fixture would have taken a TLS-band port against a `10<index>` convention four consecutive fixtures follow.

**Two findings that only REPEATS could produce, recorded because the method matters more than the results.** R21's within-layer collision reports clean *last-declared-wins* on a single run and is in fact **~40/60 non-deterministic** over 18 runs; R23's debug-marker randomization is invisible until a process restarts. **Both would have been recorded as settled facts by a competent single-pass probe.** ⇒ where a wrong answer would be embarrassing, repeat the arm — `reference_probe_must_discriminate` is necessary but not sufficient, because a probe can discriminate perfectly and still measure a coin flip once.

**A refuted hypothesis this SPEC itself introduced, recorded rather than dropped.** The controller's follow-up probe brief predicted that `9007199254740993` would **lose precision** through protobuf `Struct`'s double storage, and asked for it as a finding. It does not: out-of-int32 values are stored as `string_value` and render exactly. **The prediction was wrong in direction** — the loss hits *small* integers instead (R22). A brief's hypothesis is not evidence either.

**A controller self-correction, recorded rather than quietly amended.** This SPEC's first reading of D-RSL-EMPTY carried the BRAINSTORM's rationale — *"matching would be awkward and arguably dishonest"* — which is an aesthetic judgment, not evidence. The probe that measured the synthesized layer as **genuinely writable** (`num_keys` 0 → 1 → 3) replaced it with a structural argument: the zero-declared case **is** an implicit `admin_layer` request, and envoy-go already rejects the explicit form. Same resolution, different and checkable reason.

⚠️ **The Bash cwd reset fired AGAIN — the NINTH consecutive session.** Observed live (`Shell cwd was reset to /home/esa/git/envoy-go` after a `cd` into scratch). Every git command in this session used `git -C <abs-worktree-path>`.
