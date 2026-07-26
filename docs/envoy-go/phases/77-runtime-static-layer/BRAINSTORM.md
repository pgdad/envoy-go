# Phase 77 Brainstorm — `layered_runtime` static-layer consumer (**the FIRST Runtime-+-hot-restart-family row; the family OPENS**) — the row that turns a **boot-REJECT of a config the reference accepts** into a served snapshot, and finds the termination sentinel's own check (2) **FAILING UNSAFE at this tip**

*Stage: BRAINSTORM. Self-picked per the 2026-07-12 standing directive. Docs-only: ZERO production `.go`, ZERO test `.go`. Anticipated at the IMPL: +2 stats / +1 fixture (`0118`) / +0 fuzzers / +0 BackendKinds / +0 go.mod modules / +0 new packages / +0 new PUBLIC surface.*

---

## 0. ⚠️ READ THIS FIRST — the sentinel finding, because it changes what a "blocker-clearing" pick is worth

The router elevated the selection calculus with this claim:

> *"A candidate that clears a check-(3) blocker is therefore now worth **strictly more** than one that does not."*

**That is REFUTED as arithmetic, by this session's own mechanical census.** Clearing a check-(3) blocker by opening a family *mints* a check-(2) sentence, because the family's un-chartered remainder has to be recorded somewhere. The sentinel item count does not fall:

| | never-opened families (check 3) | candidate sentences (check 2) | total |
|---|---|---|---|
| before phase 77 | 3 (gRPC · Runtime · WASM) | 3 (`:186` HTTP/3 · `:196` xDS · `:206` Observability) | **6** |
| after phase 77 | 2 (gRPC · WASM) | 4 (+ Runtime) | **6** |

**What survives, and it is the real reason to take the pick:** opening a family is the *only* way that family's work becomes enumerable at all. The work exists either way; the opening converts an invisible blocker into a visible, chartered one. That is a prerequisite, not a shortcut — and it is a strictly weaker claim than "worth strictly more". **Recorded here so no later stage re-inherits the stronger version.**

### 0.0 ⚠️ THE MEASURED RECORD: check (2) HAS NEVER GONE DOWN, AND NARROWING RE-POPULATES IT

This is the session's strongest single piece of evidence, and it was obtained by **replaying the sentinel over every ROADMAP commit** rather than by reasoning:

- **The sentence count has been 3 in EVERY commit since `cbda648b` (2026-07-12)** — phases 60 through 76, ~17 phases, 14 days. Its entire history is `0 → 1 → 3`. **It has never once decreased.**
- The Observability clause count over the same span oscillated **2 ↔ 3** and **rose as often as it fell**.
- **The decisive case is phase 74, which landed three `ssl.*` stat names and made its own sentence LONGER**: before (`771dff02`) the clause read *"the downstream TLS handshake-outcome `ssl` stat family (…) + tracing …"* — **2 clauses**; after (`f8f6cd44`) the family clause had **split** into *"the DYNAMIC half"* **plus a new `connection_error` clause** — **3 clauses**.
- **Phase 73 is the same pattern from the other side.** Its own SPEC (`:44`) flagged that *"`CLUSTER` is the LAST item in its clause, so that narrow REMOVES the clause"* — and the clause was **REPLACED, not removed** (`:352`), because the tracing backlog held more items than the clause named. **It still does:** the phase-73 BRAINSTORM (`:283`) lists **six** tracing follow-ons where the ROADMAP clause names three.

⇒ **the candidates sentence is a WINDOW onto a larger deferred backlog, not an inventory of it.** Every narrow in this lineage has re-populated it. This is what makes §2.6's rejection of a check-(2) narrowing row an evidence-based call rather than a preference.

### 0.1 ⚠️ NEW — check (2) is FAIL-UNSAFE, and it is failing unsafe RIGHT NOW

Check (2) matches **only** the long phrase `remaining deferred (not-yet-chartered) candidates:`. The **Operational-tooling** family records genuinely un-chartered work at **`ROADMAP.md:218`** in the SHORT form **`deferred candidates:`** — three items (xDS-sourced dry-validation · an admin-API-exposed live-reload-and-validate endpoint · an RTDS/SDS validate companion) — in the *same paragraph* that reads `**The family STAYS OPEN**`. **Check (2) cannot see them.**

Census, EXECUTED at `f48975c4`. SIX family sections carry a `FAMILY OPEN`/`NEW FAMILY` marker — Load balancing `:158` · Upstream robustness `:178` · HTTP/3 `:186` · xDS `:196` · Observability `:206` · Operational tooling `:218` — but only THREE carry the long form. The non-matching phrasings in the file are `Remaining candidates after 34:` (`:158`), `remaining candidates UNBLOCKED after …` (`:168`), `deferred candidates:` (`:218`), plus four **historical** `candidates were:` recaps inside `:206`.

Broadened matcher, with all three negative controls EXECUTED:

```sh
R='deferred candidates:|remaining deferred \(not-yet-chartered\) candidates:'
grep -noE "$R" docs/envoy-go/ROADMAP.md      # => 4 : :186, :196, :206, AND :218
```
- **NC1** a `candidates were:` recap line ⇒ **0**.
- **NC2** a short-form-only line ⇒ **1**.
- **NC3** an empty file ⇒ **0**.

⚠️ **AND THE DOCUMENTED TRAP'S STATED MECHANISM IS REFUTED, while the trap itself is real.** `reference_sentinel_deferred_sentence_live_vs_historical` warns that a naive `grep -c 'candidates:'` returns a larger number *because of* the `candidates were:` historical recaps. It does return a larger number — **11**, EXECUTED — but **not for that reason**: the string `candidates were:` does not contain the substring `candidates:` at all (`grep -c 'candidates were:'` ⇒ **1**, and it is invisible to the naive count). The inflation comes from **ROADMAP table rows carrying BRAINSTORM prose** (`:74`, `:130`-`:135`, `:218`). **Cite the command, and do not re-inherit the explanation.**

**Why this matters more than a wrong number.** Check (3) fails **SAFE** by construction (an unlisted family slug reads as "never opened" and BLOCKS `stop`). Check (1)'s four-miss regex blind spot is currently **harmless** (all four missed rows are `done`). **Check (2)'s blind spot is LIVE**: if the three visible sentences were ever emptied and the three families opened, `stop` would fire with three un-chartered Operational-tooling candidates still on the books. **A false "done" ends the project early** — the one failure mode the router says costs more than a wasted session.

**The fix landed by this stage is in `next-prompt.txt`, not in `ROADMAP.md`** — broaden the matcher, which fixes the **class** and immediately surfaces the `:218` **instance** with no prose edit at all. Normalising `:218` to the long form is recorded as a named follow-up, deliberately NOT done here (it is a ROADMAP prose edit beyond this row's registration).

⚠️ **After this stage lands, check (2) prints FOUR under the old matcher and FIVE under the broadened one.** Both are *increases*, and both are **correct**: the ledger got more accurate, not worse. Do not "fix" either count down.

---

## 1. Mission and scope confirmation

### 1.1 What phase 77 delivers as a self-contained whole

**Today envoy-go boot-REJECTS a config the reference accepts and serves.** `internal/bootstrap/bootstrap.go:568-569`:

```go
if _, ok := generic["layered_runtime"]; ok {
    return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
}
```

Phase 77 lifts that reject **for the `static_layer` arm only** and lands the runtime layer behind it:

1. **`internal/runtime`** — a `Snapshot` built at boot from the parsed layers: recursive Struct→dotted-path flattening, **distinct-key union across layers**, precedence-collapsed. (The package **directory already exists** — a phase-00 `doc.go`-only placeholder — so this is **+0 new packages**.)
2. **The bootstrap consumer** — the wholesale reject replaced by the `static_layer` arm plus a byte-stable reject roster for everything not yet supported (§6).
3. **Two gauges, `runtime.num_keys` + `runtime.num_layers`** — registered unconditionally at boot, measured on the reference to be present in **every** probe arm (§2.3).
4. **ONE differential fixture `0118-runtime-static-layer`** asserting those two gauges cross-side through the existing `fixture.StatsAsserter` seam over `/stats/prometheus` — **not** through `ProbeAdmin` (§2.4 explains why that would flake by construction).
5. **Corpus seeds for the EXISTING `FuzzBootstrapLoad`**, which newly reaches the `layered_runtime` parse path. **+0 `func Fuzz`.**

### 1.2 What phase 77 does NOT deliver (forward to §8)

RTDS · the `/runtime` and `POST /runtime_modify` admin endpoints · the disk layer and its override dir · the admin layer · lifting any of the six `runtime_key` filter parse-reject arms or the silent-IGNORE runtime knobs · hot restart · graceful-drain beyond phase 08.

### 1.3 Phase-done as the FIRST Runtime-family row — the family OPENS

The charter is **two lines, quoted in full** (`ROADMAP.md:208` + `:210`; `:209` is blank, `:212` is the next heading):

> `### Runtime + hot restart family`
> `Runtime layer (RTDS consumer); hot-restart / graceful-drain semantics beyond phase 08's minimum.`

There is **no `FAMILY OPEN` marker and no candidates sentence** in that section today.

**The honesty test, answered directly.** Check (3) greps `ROADMAP.md` for the literal `<Family>-family row`; writing it for a family a row does not deliver silently satisfies the grep without doing the work — the failure that fired LIVE at the phase-76 BRAINSTORM, twice in one commit. **This row delivers the charter's first clause — the runtime LAYER — and defers only its RTDS transport.** So `Runtime-family row` in the row-77 summary is a **legitimate USE**, on the ratified phase-60 precedent (SDS opened the xDS family without lifting `dynamic_resources`, and 60.1 landed substrate with *zero* production wiring and *zero* differential; this row lands strictly more — a real bootstrap consumer, a behavior change, and a cross-side differential).

⚠️ **Two candidate scopings were tested against the same question and FAILED it, and both are recorded as rejected rather than quietly dropped:**
- **A graceful-drain / hot-restart-only row would NOT honestly open this family.** The charter says *"beyond phase 08's **minimum**"* — but phase 08 was not a minimum: it landed `internal/drain/` (a LIVE→DRAINING state machine with atomic inflight and a `Done()` rendezvous), a SIGTERM upgrade, `Drain` on both managers, `POST /drain_listeners`, `/ready` and `/server_info` transitions, and fixture `0010`. A row adding `?graceful`/`?skip_exit`/`?inboundonly`/`/healthcheck/fail` is **admin-surface work whose natural home is the already-open Operational-tooling family**; hot restart proper is a process-model rewrite (shared-memory stat merge + socket handoff).
- **`fault.abort.grpc_status` would NOT open the gRPC family** — re-confirmed independently (§2.6), and its inherited justification's PROVENANCE is now located.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW anticipated, escape-valve armable

Anticipated at **9-11 tasks**, which is exactly this project's modern cadence — `grep -cE '^#+ *Task [0-9]+' docs/envoy-go/phases/*/PLAN.md` gives **9** for phases 62, 63, 64, 66, 67, 68, 70, 71, 72, 73, 74, 76 and **11** for 65, 69, 75. A single flat row is the default; the natural split seam if one is needed is *(snapshot + reject roster)* / *(stats + fixture)*.

⚠️ **The cost estimates this BRAINSTORM inherited were calibrated against a MIXED ERA** and must not be re-inherited: the early MVP trunk phases run **17-26** tasks (00, 01, 03, 04, 05.1, 06.2, 07.1, 07.2, 09, 10, 11, 14, 15, 22.2, 25.1, 25.2, 26.1, 28.1, 29.1, 31), and one re-costing agent anchored partly on 08.1=16 / 08.2=14 alongside 60.1=10. **Against the modern cadence, ~9-11 is an ORDINARY row, not a large one** — and the ±2 spread is the resolution limit of this project's own task-count granularity, which is why a 2-task cost delta against a rival cannot by itself settle a pick.

### 1.5 Package placement — ALL implementation in EXISTING directories, ZERO new packages

`internal/runtime/` exists (phase-00 `doc.go`, 241 bytes, commit `f2e4576f`), already resolves under `go list`, and `go list -deps ./internal/runtime` returns **itself alone** — a clean leaf. `go list ./internal/... | wc -l` ⇒ **73** at this tip. ADR-0278 §Context (`DECISIONS.md:16672`) already names it: *"the `internal/xds` (and `internal/runtime`) `doc.go` placeholders have named this family expansion since phase 00."*

**Cycle-guard design constraint (`reference_xds_config_seam_transitive_cycle_guard`), stated up front rather than discovered at IMPL:** `internal/runtime` must import **only** `bootstrapv3` + `structpb` + `internal/stats`. Measured at this tip: `internal/bootstrap` deps = `{stats, accesslog}`; `internal/admin` deps include `bootstrap`. With that constraint, `bootstrap→runtime` and `admin→runtime` are both safe, TYPE-level only. Both `bootstrapv3` and `structpb` are **already imported by production code**, so this is **+0 go.mod modules** and no new dependency edge outside the module.

### 1.6 No prebrainstorm-notes branch

None exists for this subject; this BRAINSTORM is written from first principles at `f48975c4` plus four measured re-costings (§11).

### 1.7 Relationship to the existing seams — EIGHT landed forward-pointers discharged

The project has been accumulating ADR-anchored debt pointed **at this family** since phase 21:

- **SIX `runtime_key` PARSE-REJECT arms**, CONFIRMED by enumeration: five in `internal/filter/http/admission_control/compiled_config.go` (`enabled` `:179`, `aggression` `:182`, `sr_threshold` `:185`, `max_rejection_probability` `:189`, `rps_threshold` `:192` — ADR-0195, `DECISIONS.md:12413`) and one in `internal/filter/http/adaptive_concurrency/compiled_config.go:171` (ADR-0187, `DECISIONS.md:11280`). ADR-0187 `:11303` and ADR-0195 `:11369` both carry an explicit *"Forward-pointer to the Runtime/RTDS family phase"*. ⚠️ **`parseRejectEnabledRuntimeKey` is the SAME identifier in BOTH packages** — a roster keyed on constant NAME undercounts six to five (`reference_spec_drafted_identifier_collision_check`).
- **TWO ADR-0089 admin rows** (`DECISIONS.md:3514` block) that name this family *by name*: `/runtime` ⇒ *"unscheduled (Runtime + hot restart family)"* and `POST /runtime_modify` ⇒ *"unscheduled (Runtime layer / RTDS consumer family)"*. **This is a second, independent forward-pointer class that the inherited recommendation did not name at all.**

⚠️ **A THIRD posture exists that neither the router nor the inherited recommendation mentions: silent-IGNORE, not reject.** `RuntimeFeatureFlag`/`RuntimeFractionalPercent` knobs that are parsed and discarded live in `bandwidthlimit` (`runtime_enabled`), `compressor` (`runtime_enabled`), `csrf` (`filter_enabled`), `extauthz` (`filter_enabled`/`filter_enabled_metadata`), `fault` (four runtime-key fields + `filter_enabled_runtime`), `localratelimit` (`filter_enabled`/`filter_enforced`), plus `WeightedCluster.runtime_key_prefix`. **These have no reject to lift**, so a future row honoring them converts IGNORE→HONOR — a live behavior change with no atomic-guard problem but a real cross-side divergence risk. Forwarded in §8; **explicitly out of scope here.**

---

## 2. Design decisions

### 2.1 Row + subject confirmation *(SELF-PICKED per the 2026-07-12 standing directive)*

**Row 77 is registered `in-progress` at `ROADMAP.md`, immediately after the last data row `:138`, INSIDE the table.** Mechanics re-derived at `f48975c4`, not inherited:

- Row id **`77`**; slug **`runtime-static-layer`**; the "after" cell is **`76`**.
- Insertion point is directly after `:138` (`| 76 | lb-ring-hash-spread-margin | 75 | done |`), which is followed by a blank line, a `---`, and `## Feature Families (09+)`. **Inserting after the `---` would put the row outside the table and make it invisible to all three checks.**
- ⚠️ **Checked against check (1)'s FOUR-miss blind spot BY EXECUTION, not by argument.** Blind spot re-derived at this tip: **108 data rows (`:31`-`:138`), 104 matched, FOUR misses** — `| 00 | bootstrap | — |` (em-dash in the "after" cell), `| 04 | http-1.1 |` (DOT in the slug), `| 28.1a |`, `| 28.1b |` (LETTER suffix on the id); all four are `done`, so there is no current impact. The prospective row-77 line was then run through the check-(1) regex **in isolation**: it MATCHES and the awk arm prints `NOT DONE: row 77`. **Negative control:** the same line with `done` substituted prints **nothing**. The row is genuinely visible to check (1).

### 2.2 Scope — the smallest defensible Runtime-family opening *(SPEC pins: D-RSL-SCOPE)*

Three scopings were costed. The pick is the **narrowest that still delivers the charter's first clause**:

| | scoping | tasks | verdict |
|---|---|---|---|
| **(a) PICKED** | static-layer consumer + `num_keys`/`num_layers` + one fixture | **9-11** | the runtime layer, load-bearing and differentially pinned |
| (b) | (a) + `/runtime` admin endpoint | +2-3 | **DEFERRED** — needs an ADR-0089 amendment, and the endpoint body is doubly non-deterministic (§2.4) |
| (c) | (a) + lifting filter `runtime_key` rejects | +4-5 | **DEFERRED** — each lift needs its own fallback guard, and the two ADRs' absent-key defaults are **OPPOSITE** (adaptive_concurrency absent⇒OFF, admission_control absent⇒ON) |

⚠️ **One hidden cost the inherited "~10-14" did not name, and it sits at the BOOTSTRAP seam rather than the filter seam:** today `:568` blocks **all four** `layer_specifier` oneof arms wholesale. Lifting it for `static_layer` must land, **atomically** (`reference_lifted_reject_hidden_enforcement`), byte-stable rejects for everything else — see §6 for the full roster and why **envoy-go must hand-write arms the reference gets from PGV** (`reference_pgv_forecloses_go_hazard`).

### 2.3 The semantics — MEASURED on the reference across eleven arms, not read from upstream docs

Reference `envoyproxy/envoy:contrib-v1.37.2` (image id `7edd5b0fd763`, matched against the `ENVOY_TARGET.md` pin). Fresh container per arm, bridge network, published admin ports (`feedback_probe_fresh_container_per_arm`, `reference_docker_probe_bridge_network`).

| arm | `static_layer` | `num_keys` | `num_layers` | `entries` keys |
|---|---|---|---|---|
| 1 | `L1{k.one:1}` | 1 | 1 | `k.one` |
| 2 | `L1{k.one:1, k.two:2}` | 2 | 1 | `k.one`,`k.two` |
| **3** | `L1{k.same:"from1"}` + `L2{k.same:"from2"}` | **1** | **2** | `k.same` |
| **4** | `L1{a:{b:{c:1,d:2}}}` | **2** | 1 | `a.b.c`,`a.b.d` |
| 5 | `L1{k.frac:{numerator:25,denominator:HUNDRED}}` | **1** | 1 | `k.frac` |
| **6a** | `layers: []` | 0 | **1** ⚠️ | — |
| **6b** | `layered_runtime` absent | 0 | **0** | — |

**The rules an implementer can code from:**
- `num_keys` = size of the **union of flattened leaf paths across all layers**. Flattening is **unbounded depth**, dot-joined (a four-level probe yields the single leaf `deep.l2.l3.l4`); a Struct carrying `{numerator, denominator}` **terminates flattening and counts as exactly ONE**; a key whose value is the empty string **still counts**. A key present in N layers counts **once** — arm 3 is the only arm that separates union from per-layer sum, and it gives 1, not 2.
- `num_layers` = the **declared** layer count, in declaration order.
- **In all eleven arms, `num_keys == len(entries)` and `num_layers == len(layers)` held without exception.**

⚠️ **THE OFF-BY-ONE TRAP, isolated rather than reasoned about.** `layered_runtime` **present with zero declared layers** reads `num_layers: 1` with `layers: [""]` — an unnamed layer absent from the config. Shape-independent across three spellings (`layers: []` flow, `layers: []` block, and `layered_runtime: {}` with no `layers` field). A 200-vs-503 cross-product with **both** controls proves what it is: `POST /runtime_modify` returns **200** in that case and **503 `No admin layer specified`** both when `layered_runtime` is absent *and* when one static layer is declared. **It is a synthesized, writable ADMIN layer with an empty name.** ⇒ the honest "empty" arm for a fixture is **`layered_runtime` absent entirely**, never `layers: []`. §7 carries the design question this forces.

⚠️ **Two loader-time boot-rejects found incidentally, NOT PGV:** a `static_layer` value that is a **list** or **null** boot-FAILS with `Invalid runtime entry value for <key>`. Each reproduces alone. The `null` case pairs with `reference_protojson_null_decodes_to_nil`.

**What was NOT isolated, flagged rather than filled in:** *why* Envoy synthesizes that admin layer (no upstream source read, no mechanism claim) · whether `num_keys` counts `disk_layer`/`rtds_layer` keys (not probed — no disk mount, no RTDS server) · whether duplicate leaf paths from different nesting shapes in one layer collide, error, or double-count · whether a key set to `""` in an overriding layer clears or shadows a lower layer's value (⚠️ `""` is *also* the positional sentinel for "this layer does not define the key", so the two are **indistinguishable in the `/runtime` body** — an assertion trap for any future row that asserts that body).

### 2.4 Fixture posture — ONE new fixture, and it must NOT go through `ProbeAdmin` *(D-RSL-FIXTURE)*

**`compareAdminResponses` (`test/differential/runner_test.go:1394`) compares the body BYTE-EXACT** (`// Body: byte-exact.` + `CompareBytes`). The reference `/runtime` body **cannot survive that**, for two independently measured reasons:
1. **JSON key order is randomized PER REQUEST** — three consecutive GETs on one container gave three distinct md5s with an **identical** sort-keys md5.
2. **The Struct debug-string prefix is randomized PER PROCESS** — `goo.gle/debugstr` in one arm, `goo.gle/debugonly` in another, stable within a process and flipping across a `docker restart`.

⇒ the fixture asserts **stats**, via `fixture.StatsAsserter` (dispatched at `runner_test.go:1347-1349` with **both** admin addrs), over `/stats/prometheus`, on a **named subset** — the phase-75 `0110` Shape-A precedent. Both randomizations contaminate only the `/runtime` **body**, never the gauge values, so a stats-only assertion is immune by construction.

**Name-presence divergence: NONE, and this is load-bearing.** Swept across all eleven arms, `envoy_runtime_num_keys{` and `envoy_runtime_num_layers{` each returned exactly **1**, and `^runtime\.` in `/stats` returned **9**, in *every* arm — including the arm with `layered_runtime` absent, where both gauges are present with value **0**. ⇒ the fixture may assert the name set unconditionally, and envoy-go should register the names at boot regardless of config.

**The fixture's minimum honest pin set** — a single-layer config is **NOT sufficient**, because with one layer the union and per-layer-sum hypotheses are numerically identical for every input:
- a **2-layer overlap** arm (the only arm separating union from sum, and the only one where `num_keys ≠ num_layers`, so it also catches a transposed-gauge bug),
- a **nested** key (pins leaf-flattening: 2, where a naive implementation gives 1),
- a **fractional** key (pins the non-flattening exception: 1, where a naive implementation gives 2).

### 2.5 Verification design — what makes each gauge non-vacuous

`num_keys` is a **direct function of the flattening and union rules**, so the differential pins the semantics rather than pinning a token. The three-arm pin set above is chosen so each arm's *expected* value differs from the value a plausible wrong implementation would produce — this is the `reference_probe_must_discriminate` discipline applied at design time, and `reference_ordered_assertion_legs_vacuous_on_constant_change` says each leg must be proved to fire its own assertion at the IMPL, not assumed to.

### 2.6 The rivals, re-costed at THIS tip — FOUR inherited costs refuted, in BOTH directions

Every figure below was re-derived at `f48975c4`; none is inherited (`reference_deferred_candidate_cost_restale`).

**`fault.abort.grpc_status` — the strongest rival, re-costed DOWN to ~7-9 tasks, and REJECTED on scope, not on cost.**
Its current state is **worse than "ignored"**: `internal/filter/http/fault/fault.go:132-146` sets `abortEnabled = true` *only inside* the `*faultv3.FaultAbort_HttpStatus` type switch, so a `grpc_status` variant leaves it **false** and **no abort fires at all** — the request reaches the backend. A live cross-side divergence.
A 4-arm codec×content-type cross-product on the reference discriminates three things: the branch is on the **request `content-type: application/grpc`, NOT on the codec**; the HTTP status is **200 in every arm**; and the reference emits `grpc-status`/`grpc-message` as response **HEADERS**, not trailers. **That third finding retires the framework blocker** — `SendLocalReply(status, body, headers)` already expresses the reference shape exactly, so no new primitive is needed, which is why the inherited ~10-13 was too high. Envelope **+0 on every axis** (`0011-http-fault` already drives five abort scenarios over an H1 downstream and is extensible; arm C fired the full gRPC-shaped abort over plain HTTP/1.1 with no gRPC client, backend, or framing).
⚠️ **It does NOT open the gRPC family — and the PROVENANCE of the claim that it does is now LOCATED.** `ROADMAP.md:48` (row 09) reads *"Deferred per `BOOTSTRAP_PROMPT.md` §9 family-expansion: `response_rate_limit`, `abort.grpc_status` **(gRPC family)**, all four runtime-key fields **(Runtime + hot restart family)**, …"*. That parenthetical is a **ROUTING LABEL** — which family will eventually own the item — in a list whose sibling entry labels runtime keys the same way. **Reading a filing label as a charter is the error that rode three stages and two documents.** The clinching evidence is the probe itself: a feature that needs no gRPC to exercise cannot open the gRPC family.

**The gRPC family cannot be opened defensibly at this tip — the cheapest honest route is 16-22+ tasks.**
The charter (`ROADMAP.md:190`, the section is exactly three lines with **no** marker and **no** candidates sentence) names bridge · gRPC-Web · gRPC-JSON transcoding · interop conformance. `grep -c 'gRPC-family row'` ⇒ **0** (negative control: `Observability-family row` ⇒ 23). Zero of the seven gRPC filter type-URLs appear anywhere in the tree; the repo is extensively a gRPC *client* (`google.golang.org/grpc` is a **direct** require) but that is orthogonal — every such site is envoy-go calling *out*, not proxying gRPC for a downstream.
- `grpc_http1_reverse_bridge` (charter subject #1) and `grpc_web` (#2) both **cleanly** satisfy the charter and are both blocked on the **same missing framework seam**: **`RunEncodeTrailers` has ZERO production callers** (against 51 `RunDecodeHeaders`, 19 `RunEncodeHeaders`, 10 `RunDecodeTrailers`, 9 `RunEncodeData`); its only call site anywhere is `chain_test.go:294`. Response trailers are never delivered to filters or emitted, upstream response trailers are never captured, and the harness client cannot read trailers at all. **16-22+ tasks, ADR-0045 split likely.**
- `grpc_stats` is the cheapest actual gRPC *filter* at ~12-14 (this project's floor for a new HTTP filter package is **12**) but is **none of the charter's four named subjects**, and carries a **remote-client-triggerable stat-name panic**: with `stats_for_all_methods: true` the reference derives names verbatim from the client-controlled `:path`, and an observed real name `cluster.c_grpc.grpc.we-ird.svc.Do-Thing.total` **fails** `NamePattern` on the hyphen, where `Registry.getOrRegister` **panics**.

**`Listener.stat_prefix` — re-costed UP to 10-12 tasks (inherited ~7-10 was too LOW), and one of its two named risks RETIRED.**
Survives: zero `GetStatPrefix()` consumers in `internal/listener/` or `internal/bootstrap/`; the field is proto field 28, so accepted-and-silently-ignored; the blast radius is a red herring and *stronger* than stated (**4** production `"listener\.` sites, with `registerListenerMetrics` the single choke point). **REFUTED:** the inherited cite `BEHAVIOR_CONTRACT.md:1859` is really **`:1857`**; the `envoy_listener_address`-carries-a-non-address "risk" is **PARITY, not a design question** (the reference itself renders `envoy_listener_downstream_cx_total{envoy_listener_address="SHAREDPFX"}`); and the "swallowed-panic boot-hang" mechanism is wrong — there is **no `recover()` anywhere** in the boot path, so it is a loud process crash.
⚠️ **NEW, and it is why the cost went up: the reference MERGES two listeners sharing a `stat_prefix` into ONE aggregated scope** — both addresses vanish and `listener.SHAREDPFX.downstream_cx_total` is the **SUM**. envoy-go's registry **panics on duplicate registration**, so a naive implementation **crashes at boot on a config the reference accepts and serves**. Parity needs `NewCounterIfAbsent` **plus shared counter pointers**.
⚠️ **Strategic cross-link worth carrying:** its hyphen/charset guard is the **same validator** already blocking the Observability family's `ssl.*` dynamic families on the live check-(2) list. **Solving it once counts twice** — the only rival with a blocker genuinely shared with a check-(2) item.

**`upstream_cluster` — the cost DRIVER refuted hard; the central premise still UNVERIFIED.**
The inherited "~85-100 lines across ~18 files / a new 10th parameter / 42 test call sites" is **REFUTED**: phase 73 already built this seam, and `extractEndpoints(la, clusterName)` (`internal/cluster/manager.go:875`) **already receives the cluster name**, constructing the `Endpoint` literal at `:896` where the name can be stamped as one field. Real delta ≈ **20-25 lines / 3 files**; **6-8 tasks**. Both inherited premise checks survive (`grep -c 'GetClusterName()' internal/cluster/manager.go` ⇒ **0**, so the proposed break would not have compiled), and the row stays **gated on one live probe** of whether the reference emits `""` on a no-upstream local reply — if it does, envoy-go's zero-`Endpoint` sites are exact parity and the row collapses.
⚠️ **The Lua misattribution is TEN sites, not five** (5 `driver.go` + 5 `README.md`), and it is *doubly* misleading: `bridge.go:1391`/`:1553` do return `""` as a real Lua-side gap, so the sentence names a genuine adjacent defect while pointing at the wrong causal path. ⚠️ **`%UPSTREAM_CLUSTER%` is a different row** — a parse-REJECT (`internal/accesslog/otlpformat_test.go:105`), carrying `reference_lifted_reject_hidden_enforcement`.

**The check-(2) narrowing rivals — the cheapest is `ssl.connection_error`, re-costed UP to ~12-15, and it CANNOT silence the check it targets.**
Clause counts, EXECUTED with a paren-depth-aware split: HTTP/3 `:186` = **7** clauses · xDS `:196` = **9** · **Observability `:206` = 3, the shortest and therefore the cheapest sentence to eventually empty.** Costing that sentence clause-by-clause: the dynamic `ssl` half is **≥3 rows, ~30+ tasks, with one leg having NO Go seam at all** (`go doc crypto/tls.ConnectionState` exposes `Version`, `CipherSuite`, `CurveID` but **no signature-scheme field**, so `sigalgs` may be unimplementable; and `ssl.sigalgs.*` appears only under mTLS on the reference); the tracing trio is **3 rows, ~27 tasks**, the lineage's own sizing calling force-trace *"a whole new subsystem. HIGH scope-risk."* **Sentence total ≈ 6-8 rows and ~60-70 tasks — and that is before the §0.0 re-population base rate.**
`ssl.connection_error` itself re-costs to **~12-15** (both inherited figures — ~9-11 and ~10-13 — are too low), and ⚠️ **the inherited FRAMING is refuted in its direction**: the reference's `ssl.connection_error` is BoringSSL **`SSL_ERROR_SSL`, a PROTOCOL error, explicitly NOT `SSL_ERROR_SYSCALL`**. Measured on the reference with a `direct_response` route (so the fast-failing-upstream suppression hazard is structurally eliminated) and with `ssl.handshake >= 1` in the same scrape as the guard — **NOT `downstream_cx_total`**, which `reference_ssl_stats_suppressed_by_fast_failing_upstream` warns is not a decode-ran guard: mid-handshake RST ⇒ **0**, clean close ⇒ **0**, zero-byte close ⇒ **0**, while garbage bytes ⇒ **1**, a TLS-1.1-only client ⇒ **1**, cert/key mismatch ⇒ **1**. ⇒ **every transport-disconnect shape the inherited estimate was organised around increments NOTHING**, so `syscall.ECONNRESET` is needed not to make the counter *fire* but to stop it **over-firing**. Three exclusion predicates are needed, not one, and the deny-list is **open** — the positive population cannot be typed in Go (mismatch / malformed-DER / version-mismatch all arrive as bare `*errors.errorString` distinguishable only by message text). ⚠️ **A drafting footgun worth carrying: `tls.RecordHeaderError` is returned BY VALUE** — `errors.As(err, &val)` is true, `errors.As(err, &ptr)` is **false**, wrapped or not, so a predicate drafted against a pointer compiles and never matches.
⚠️ **THE DECISIVE ASYMMETRY:** narrowing `:206` from three clauses to two **leaves the sentence in place**, so `grep -c` still returns 3 and **check (2) still prints**. A check-(2) narrowing row buys *zero* progress on the sentinel, at a higher cost than the pick. Recorded so that no later stage charters one *believing* it advances termination — if one is chartered, it must be chartered as **narrowing**, never as a step toward silencing check (2).

**The maintenance bundle — the cheapest defensible ANYTHING at this tip, and it advances NOTHING on the sentinel.**
Six items, each **1-2 tasks with ZERO production lines**, all genuinely owed: the **10-site** Lua misattribution · `0062`/`0063`'s false *"DETERMINISTIC/EXACT — not a σ-band"* adjective (⚠️ **`:299` CONFIRMED, the SPEC's `:300` REFUTED**; lines 299-301, byte-identical between the two files; ✅ negative control — `0064-lb-subset/driver/driver.go:449` carries the same adjective and is **correct**, so the false surface is exactly those two) · `0061/driver/driver_test.go`'s **exactly TWO** provably-vacuous negative arms, now identified by name (`CollapseBitesSpread` trips conservation, `ReferenceConservation` trips *subject* conservation) · the rate-1.0 CONTROL diagnostic (`internal/cluster/ringhash_test.go:222-229`) · `0013`'s 10 ms band (⚠️ the verdict string is written **into the compared byte stream**, so a jitter blip past 260 ms surfaces as a cross-side **byte divergence**) · `0059`'s empirical margin, now **UPGRADED to near-owed** because `concentrationMin=16` is **below the project's own 4-5σ bar** while an exact Binomial margin *is* derivable (`c2 = 1 + min(X, 60−X)`, `X ~ Binom(60, ½)`).
⚠️ **A NEW item nobody had named, and it meets the "would cause wrong work" bar:** `BEHAVIOR_CONTRACT.md:1857`'s structural evidence for the settled QUIC `ssl.*` PARITY determination cites `manager.go:1078-1082` and `:1044-1054`; the real branches are **`:1098`** and **`:1064`** — both drifted **+20**, and `:1078-1082` now lands on the bind-failure unwind block, which does not support the claim at all. ⚠️ **The cheap mechanical gate does NOT catch this** — an out-of-range check over all BEHAVIOR_CONTRACT Go cites printed **nothing**, so a range gate is a **false negative** for the drift species; only content-anchoring fixes it (`reference_gate_command_negative_control`). Three of the file's twelve `*.go:NNN` cites are stale (also `chain.go:19`→`:26`, `chain.go:25`→`:32`).

**Why the pick beats the field, stated as a trade-off rather than as a ranking.** `fault.abort.grpc_status` is 2 tasks cheaper — **inside the ±2 resolution of this project's own task-count granularity** (§1.4) — and the maintenance bundle is cheaper still. What settles it is not cost:
1. **Severity of the divergence fixed.** This row turns a **total boot failure** on a config the reference accepts and serves into a served snapshot. That is a strictly more severe class than a filter knob that fails to fire. ⚠️ **Neither the router nor the re-costing that recommended this family named this as the merit** — both framed the pick as "no filter-behavior change". **It IS a behavior change, and it is the strongest argument for the row.**
2. **It is the only defensibly-priced route to clearing a check-(3) blocker** (gRPC's cheapest honest opening is 16-22+; WASM is a documented bookkeeping artifact).
3. **It discharges eight landed forward-pointers** (§1.7) — six ADR-anchored reject arms plus two ADR-0089 admin rows.
4. **Its semantics are already pinned BY MEASUREMENT** across eleven probe arms, so the SPEC starts from measured ground rather than from upstream prose.

`fault.abort.grpc_status` at **~7-9 tasks / +0 on every envelope axis** is **BANKED as the recommended phase-78 opening**, on its own merits as an HTTP-filters-family row and explicitly **not** as a gRPC-family opening.

### 2.7 Stat surface hypothesis — **+2** (1205 → 1207)

`runtime.num_keys` + `runtime.num_layers`. Both pass `NamePattern = ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (`internal/stats/registry.go:48`), verified.

⚠️ **The reference emits NINE `runtime.*` names in every arm, so a +9 full-name-parity alternative exists and has precedent** — phase 41 landed several knobs as *parse-and-register-emit-0-but-DEFER*. The hypothesis is **+2** because the other seven (`load_success`, `load_error`, `override_dir_exists`, `override_dir_not_exists`, `admin_overrides_active`, `deprecated_feature_use`, `deprecated_feature_seen_since_process_start`) annotate mechanisms this row does not build, and registering permanently-zero names inflates a tightly-tracked surface while buying nothing testable. **The project asserts NAMED SUBSETS cross-side, never the full set** (`reference_stats_sink_emits_used_only`), so +2 does not create a name-set divergence. **§7 carries this as a SPEC pin, not as a settled fact.**

⚠️ **A deferred hazard, named now so a later row does not walk into it:** runtime KEYS are arbitrary operator-chosen dotted strings. Any future row that derives a **stat name** from a runtime key hits the same panicking validator that blocks `Listener.stat_prefix` and the `ssl.*` dynamic families. This row derives no stat name from any key, so it is not exposed.

---

## 3. Framework-survey result

No new framework seam. The row uses four existing ones verbatim: the bootstrap raw-YAML generic-map pre-check block (`bootstrap.go:564-570`); the `internal/stats` registry (`NewGauge` at boot, pre-Freeze); the `admin.New` constructor-widening convention (ADR-0085, already applied twice at 08.1/08.2) **only if** a later row adds `/runtime` — **not** needed here; and `fixture.StatsAsserter`.

⚠️ **One favorable property of the existing reject mechanic, worth stating because it shrinks the reject roster:** the pre-check is a **raw-YAML generic-map** test that runs **BEFORE** `protojson.Unmarshal`, so it fires on mere key presence. Lifting it lets the message unmarshal, where ADR-0016's `DiscardUnknown:false` **already** rejects unknown subfields at any depth. ⇒ the row must hand-write only the **semantic** arms, not an unknown-field roster.

---

## 4. Bootstrap-level applicability — THIS IS the bootstrap row

Unlike most rows in this lineage, the primary edit site *is* `internal/bootstrap`. The sibling arm `dynamic_resources` (`:565-566`) stays **byte-untouched**.

---

## 5. Stat surface hypothesis — **+2** (1205 → 1207)

See §2.7. ⚠️ The total **1205 is DOCUMENTARY with no mechanical counting command**, and the ledger chain is discontinuous in **two** recorded places. **Assert the DELTA, never the total.**

⚠️ A re-costing pass established that a mechanical count is only **partially** constructible, which is why this stays a named deferral rather than a cheap row: registration call sites outside `internal/stats` number in the low hundreds while the documented surface is ~1205, for three independent structural reasons — the overwhelming majority of sites build non-literal names (`prefix + "leaf"`); **table fan-out** means one `statroster.New` grep hit expands to hundreds of names across five callers; and some names are invisible to **any** regex because the registration function is passed as a **method value** (`newFilterStatsWith(reg.NewCounter, prefix)`), which needs `go/types` rather than grep. ⚠️ **`NewHistogram` does not exist and never has** (deferred per ADR-0060), so a row greping for it would report a meaningless zero. A defensible count row needs an AST/type-checked counter plus a written counting-unit definition plus a committed golden — **8-11 tasks**, not the small row "count it mechanically" sounds like.

---

## 6. Anticipated edit sites *(the SPEC RE-DERIVES each at ITS OWN tip — a BRAINSTORM cite is not evidence: `feedback_brief_citations_not_evidence`)*

**Production:**
1. `internal/bootstrap/bootstrap.go` — replace the `:568-569` wholesale reject with the `static_layer` arm plus the reject roster below.
2. `internal/runtime/snapshot.go` + `layer.go` (NEW files in the EXISTING directory) — flattening, union, precedence, accessors.
3. `internal/runtime/doc.go` — replace the phase-00 placeholder text.
4. `internal/stats` call site for the two gauges (or `internal/runtime` registering against the passed registry — a SPEC pin).
5. `cmd/envoy-go/main.go` — thread the snapshot if any consumer needs it (may be **+0** at this scope).

**The reject roster that must land ATOMICALLY with the lift** — three unsupported oneof arms (`disk_layer`, `admin_layer`, `rtds_layer`; ⚠️ `rtds_layer` matters most, because silently ignoring it means a config asking for *dynamic* runtime quietly gets *static* values) plus four semantic arms the reference enforces via **PGV, which envoy-go does not run** — empty `name`, missing `layer_specifier`, duplicate layer name, more than one admin layer — plus the **two loader-time value arms** measured in §2.3 (list-valued and null-valued entries). All as **byte-stable named constants** with a `TestParseRejectConstants_ByteStable` pin, per the landed discipline. ⚠️ Today's message is an **inline `fmt.Errorf`, NOT a named constant**, so the row converts as well as adds.

**Test / fixture:** `test/fixtures/0118-runtime-static-layer/` (config pair, `expectations.yaml`, driver with `AssertStats`, README) · `internal/bootstrap/bootstrap_test.go` (⚠️ **`TestLoad_RejectsLayeredRuntime` at `:82-96` must be REPLACED, not extended** — it asserts only `strings.Contains(err, "layered_runtime")` and, unlike its sibling `TestLoad_RejectsDynamicResources:74`, does **not** assert the `"bootstrap: "` prefix) · a `FuzzBootstrapLoad` corpus seed (no existing seed contains `layered_runtime`).

**Docs / prose the row falsifies:** `BEHAVIOR_CONTRACT.md:906`, `:926`, `:968` (three *"`dynamic_resources` / `layered_runtime` rejects STILL stand"* assertions) + the stat ledger + `ROADMAP.md` (row 77 and the family-open paragraph) + `STATE.md` + `DECISIONS.md` (a new ADR) + `next-prompt.txt`.

⚠️ **Set-difference the sha256 byte-untouched roster against this EDIT roster** (`reference_plan_schedules_edits_to_a_byte_gated_file`) — `internal/bootstrap/**` and `internal/runtime/**` must NOT be byte-gated at the IMPL, and the roster needs a **MISSING** leg as well as a **MISMATCH** leg, because a deletion otherwise reads as *"no mismatch"*.

---

## 7. BRAINSTORM-time open questions to the SPEC — the D-RSL-* docket

Each is to be settled **BY EXECUTION**, not by reasoning (the phase-76 lesson: its docket was settled by execution on every item, and that is what refuted its own headline claims).

- **D-RSL-EMPTY** ⚠️ **the load-bearing one.** How does envoy-go treat `layered_runtime` present with zero declared layers, given the reference synthesizes a **writable admin layer** and reports `num_layers: 1`? Options: (i) match the reference (⇒ envoy-go synthesizes a layer it cannot write to — awkward and arguably dishonest); (ii) **PARSE-REJECT the present-but-zero-declared case** as an explicit unsupported arm, which sidesteps the divergence entirely and is honest; (iii) exclude the case from the fixture and leave it undefined (**rejected in advance — an undefined arm is a latent divergence**). *Recommendation to the SPEC: (ii).*
- **D-RSL-STATS.** +2 (`num_keys`/`num_layers`) or +9 (full reference name parity, seven permanently zero)? Settle against the phase-41 register-emit-0 precedent and the named-subset assertion practice.
- **D-RSL-NUMKEYS-DEEP.** Do duplicate leaf paths arising from different nesting shapes in one layer (`a.b: 1` alongside `a: {b: 2}`) collide, error, or double-count on the reference? **Not probed. OWED.**
- **D-RSL-EMPTYSTR.** In a two-layer overlap, does a key set to `""` in the overriding layer clear or shadow the lower value? ⚠️ `""` is also the positional "not defined here" sentinel, so the `/runtime` body **cannot** discriminate — this needs a `final_value` probe, not a body read.
- **D-RSL-REJECT.** Confirm the full reject roster (§6) arm-by-arm against the reference, each with the byte-stable wording, and confirm ADR-0016 really does cover unknown subfields once the pre-check is lifted (**do not assume it — execute it**).
- **D-RSL-SPLIT.** Single flat row, or split at *(snapshot + rejects)* / *(stats + fixture)*?
- **D-RSL-ADR.** One ADR (next-free **ADR-0299**), anchoring the family opening + the static-layer-only scope + the D-RSL-EMPTY decision.

---

## 8. What phase 77 does NOT deliver (forward)

Recorded in the ROADMAP's new Runtime family paragraph in the **LONG form**, so check (2) sees them: **RTDS layer · `/runtime` + `POST /runtime_modify` admin endpoints (ADR-0089 un-defer) · disk layer + override dir · admin layer · lifting the six `runtime_key` filter parse-reject arms · honoring the silent-IGNORE runtime knobs (§1.7) · hot restart / graceful-drain beyond phase 08.**

⚠️ **Writing that sentence in the long form is a deliberate, honest choice with a visible cost** (§0): it takes check (2) from three sentences to four. The alternative — recording the same remainder in a wording check (2) cannot match — is **exactly the live defect this session found in the Operational-tooling family**, and reproducing a defect in the commit that diagnoses it is the phase-76 species repeating.

---

## 9. ADR-0045 split readiness + ADR roster

Single flat row anticipated (§1.4), escape valve armable at the §7 D-RSL-SPLIT seam. **ONE new ADR: `ADR-0299`** (`DECISIONS.md` tail is **ADR-0298 COMPLETE**; `grep -c '^## ADR-0299'` ⇒ **0**). §Context drafted at the SPEC per **ADR-0044-as-used** (⚠️ ADR-0044 itself does **not** contain that discipline); §Decision + §Consequences appended IN PLACE at the IMPL after the RETAINED italic footer, mirroring ADR-0295/0296/0297/0298.

⚠️ **It claims the FIRST Runtime-family ordinal** — unlike ADR-0298, which correctly claimed none because the Load-balancing family had closed at row 54 and maintenance rows do not extend a charter. **ADR-0089 must be amended** only if a later row lands `/runtime`; **this row does not**, so ADR-0089 stays byte-untouched and that is deliberate.

---

## 10. Envelope + counts (anticipated at the phase-77 IMPL; docs-only at this BRAINSTORM)

Re-run MECHANICALLY in the stage worktree at `f48975c4`, each with its negative control where one exists:

| axis | now | anticipated | command |
|---|---|---|---|
| differential fixtures | **119** | 120 (`0118`) | `ls -d test/fixtures/[0-9]*/ \| wc -l` |
| fuzzers | **55** | 55 (**+0** — a seed is not a fuzzer) | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` |
| stat surface | **1205** (DOCUMENTARY) | 1207 (**+2**) | ⚠️ no mechanical command; assert the DELTA |
| BackendKind tail | **38** | 38 | `H2GoawayResponder`, `test/differential/fixture/fixture.go:614` |
| go.mod modules | **2** (lineage figure) | 2 | ⚠️ NOT a repo total — the single `go.mod` requires **67** |
| internal packages | **73** | 73 (**+0** — the directory exists) | `go list ./internal/... \| wc -l` |
| DECISIONS tail | **ADR-0298 COMPLETE** | ADR-0299 | next-free **ADR-0299** |
| next-free fixture index | **0118** | 0119 | numeric tail `0117` |
| next-free reference port | **10450** | 10451 | ⚠️ **NOT** fixture-index aligned |

⚠️ **Do NOT "fix" the tail values.** BackendKind **38** is a TAIL, and the file declares **39** constants (0-38). go.mod modules **2** is the phase-61.2 lineage figure (`quic-go` + `qpack`), not a repo total.
⚠️ **A port-derivation artifact worth recording:** the controller's first probe for the next-free reference port printed **10485** — a **grep artifact**, because `104[0-9][0-9]` matched digits inside unrelated prose in `0015` and `0082`. The real `104xx` maximum in use is **10449** (fixture `0113`), so **10450** stands. **A probe must discriminate** (`reference_probe_must_discriminate`).

---

## 11. Sized-against-source — the cost derivations

### 11.1 Method
FOUR read-only re-costing agents at tip `f48975c4` on disjoint remits with **PRIVATE scratch** (`reference_parallel_subagents_private_scratch`), plus controller re-derivation of every load-bearing claim. Two agents ran **LIVE reference probe fleets** against `envoyproxy/envoy:contrib-v1.37.2` (image id verified against the `ENVOY_TARGET.md` pin), fresh container per arm on a bridge network.

### 11.2 Controller re-derivation of the agents' load-bearing claims
Independently re-executed by the controller, all CONFIRMED: the `layered_runtime` reject at `bootstrap.go:568-569` and the enclosing pre-check block · `internal/runtime/doc.go` existing with clean leaf deps · the **8** admin routes at `internal/admin/admin.go:92-99` · **zero** `runtime.*` stats in production · the six `runtime_key` arms and their two anchoring ADRs · ADR-0089's two forward-pointing admin rows · the three `BEHAVIOR_CONTRACT` reject-still-stands sites · `NamePattern` · `compareAdminResponses`' byte-exact body compare and the `StatsAsserter` dispatch · the family/candidates census · check (1)'s blind spot · the counts in §10.

### 11.3 Contested and corrected claims — a drift correction is itself a claim
- ⚠️ **Task-count calibration was MIXED-ERA and is corrected here** (§1.4). The modern cadence is 9 or 11.
- ⚠️ **The recorded ~10-14 for this family is SUPERSEDED, and the reason matters more than the number.** `phases/76-…/PLAN.md:1311` and `STATE.md` both carry **~10-14**; this BRAINSTORM records **9-11**. The recorded figure is a **document read — a CARRIED claim** — while 9-11 comes from a fresh re-costing at this tip that found the groundwork already laid (the package directory exists, `bootstrapv3`/`structpb` are already imported, `FuzzBootstrapLoad` already exists). One re-costing agent noticed the discrepancy against the documents and **deferred to the fresh derivation rather than to the documents**, which is the correct direction (`reference_deferred_candidate_cost_restale`, `feedback_brief_citations_not_evidence`). ⚠️ **But the fresh figure ALSO under-counted in one place** — it did not price the bootstrap reject roster (§6), which is the row's real hidden cost — so **9-11 is a floor, and the SPEC must re-derive it once the roster is enumerated arm-by-arm.**
- ⚠️ **A count that is glob-dependent is stated with its command, not as a fact.** Fixtures defining an `AssertStats` method: the controller's `grep -rln 'func.*AssertStats' test/fixtures/*/driver/*.go | wc -l` ⇒ **70**; an agent reported **82** under a different glob. **Neither number is quoted as "the" count** (`reference_a_drift_correction_is_itself_a_claim`).
- ⚠️ **A σ-conversion that two derivations disagree on gets NO numeral.** `0059`'s `concentrationMin=16` margin: two independent conversions disagreed (differing conventions). **The qualitative conclusion holds under both — 16 does not reach 4σ — and no single σ figure is recorded.**
- ⚠️ **`0062`/`0063` is `:299`, and the SPEC-76 `:300` is REFUTED** — already corrected in-tree at `PLAN.md:88`/`PROGRESS.md:51`, but the stale `:300` survives at `SPEC.md:69`, `:94`, `:319`.

### 11.4 ⚠️ A NEW documentary defect found while re-deriving — the documented public import path does not exist
`ROADMAP.md:218` and `BEHAVIOR_CONTRACT.md:862` document the project's public package as **`github.com/esalaine/envoy-go/validate`**. The module is **`github.com/pgdad/envoy-go`** (`head -1 go.mod`), and the live import is `github.com/pgdad/envoy-go/validate` (`cmd/envoy-go/main.go:35`, `internal/boot/boot.go:4`). `grep -rn 'esalaine' docs/envoy-go/*.md | wc -l` ⇒ **20**, across BEHAVIOR_CONTRACT · DECISIONS · ROADMAP · STATE_HISTORY. **A session copying the documented path writes code that does not compile.** Named as a maintenance deferral; **deliberately NOT fixed here** (a 4-file prose sweep is beyond a BRAINSTORM's chartered edit set), and recorded prominently rather than folded in silently.

### 11.5 Broken gates re-verified at THIS tip rather than inherited
The lineage's count stands at **seven**. Two were re-executed by the controller at `f48975c4` and **both reproduce**:
- **`gofmt -l` exits 0 while printing the unformatted file.** Observed directly. Any gate chaining it with `&&` is **INERT**; the output-gated form `[ "$(gofmt -l . | wc -l)" -eq 0 ]` goes **RED correctly**.
- **G-A reproduces: `go doc -all` diffed RAW is not symbol-scoped.** On `./test/fixtures/0061-lb-ring-hash/driver`: **51** raw output lines versus **1** matching `^(func|type|var|const)|^    [A-Z]`. The package is essentially all package-doc PROSE, so any legitimate comment edit surfaces as diff lines and a raw-diff gate exits 1 on a correct tree — a **FALSE POSITIVE**, which is worse than a fail-open because it either stops a close script or teaches a later session to suppress the gate.
- ⚠️ **An EIGHTH gate defect is added by this stage, and it is a false NEGATIVE:** an out-of-range check over `BEHAVIOR_CONTRACT`'s Go line-cites (`line > wc -l`) prints **nothing** on a tree carrying **three** stale cites, because a `+20` drift stays in range. **A range gate cannot detect anchor drift; only content-anchoring can** (`reference_gate_command_negative_control`).

---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these)

- `docs/envoy-go/phases/77-runtime-static-layer/BRAINSTORM.md` — **this file** (new).
- `ROADMAP.md` — row **77** registered `in-progress` after `:138` INSIDE the table + a `**FAMILY OPEN at phase 77**` paragraph in the Runtime section carrying the **long-form** candidates sentence (§8).
- `STATE.md` — §Current rolled **IN PLACE** (lifecycle **DONE → 1**); §Recent re-capped at FIVE **with its PREAMBLE updated** (the ADR-0288 rule).
- `DECISIONS.md` — **BYTE-UNTOUCHED** (normal at a BRAINSTORM).
- `next-prompt.txt` — rolled to the **phase-77 SPEC**, and ⚠️ **check (2)'s matcher BROADENED** per §0.1. `Runtime` is already in check (3)'s slug list, so that list needs no edit.
- ⚠️ **RE-RUN all three sentinel checks AFTER the ROADMAP edit lands**, not only at session open — `grep` cannot tell a mention from a use, and the pre-edit run is meaningless for the post-edit tree (`reference_sentinel_matcher_string_self_clears`).

Fresh worktree off master per `feedback_git_worktrees`; subagent-driven per `feedback_execution_style`; subagents commit LOCALLY only (`feedback_subagents_no_push`); controller squash-push at close (`feedback_push_to_origin`). ⚠️ **Every git command uses `git -C <abs-worktree-path>`** — the Bash cwd reset has fired in **seven consecutive sessions** (`reference_bash_cwd_reset_commits_to_main`).
