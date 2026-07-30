# Fixture 0118 — `layered_runtime` static layer (phase 77)

Cross-side pin of the phase-77 `layered_runtime` **static-layer** consumer,
asserted **STATS ONLY** on the two gauges `runtime.num_keys` and
`runtime.num_layers`.

| | |
|---|---|
| reference listener port | `10118` (family-banded `10<fixture index>`) |
| reference admin port | `9901` (harness hard-wires and maps `9901/tcp`) |
| subject ports | runner-allocated |
| backends | 1 × `TCPEcho` (the default kind; **+0 BackendKinds**) |
| driver legs | `fixture.Driver` + `fixture.StatsAsserter` |
| value endpoint | the **flat `/stats`** — see the finding below |
| expected gauges | `num_keys = 6`, `num_layers = 2` — **measured**, not extrapolated |

## ⚠️ FINDING — envoy-go's `/stats/prometheus` SILENTLY OMITS both gauges

The phase-77 PLAN §1.3 specified `/stats/prometheus` as the assertion endpoint,
on the strength of the **reference's** measured line shape. Executed at this
task, against the shipped config, on both sides:

| side | endpoint | result |
|---|---|---|
| reference | `/stats` | `runtime.num_keys: 6` · `runtime.num_layers: 2` |
| reference | `/stats/prometheus` | `envoy_runtime_num_keys{} 6` · `envoy_runtime_num_layers{} 2` |
| subject | `/stats` | `runtime.num_keys: 6` · `runtime.num_layers: 2` |
| subject | `/stats/prometheus` | **both names ABSENT** |

The gauges **are** registered on the subject and **do** carry the correct
values. The **prometheus renderer** dropped them through phase 78:
`internal/stats.ExtractTags` recognizes **thirteen** top-level segments and returns
an error for anything else:

| species | segments | anchor |
|---|---|---|
| prefix `switch` | `cluster.` `http.` `listener.` `server.` `runtime.` `access_logs.` `tracing.` `sds.` `wasm.` | the `case strings.HasPrefix(internal, …)` arms |
| root-anchored `strings.CutPrefix` (default arm) | `mongo.` `kafka.` `redis.` `thrift.` | the `strings.CutPrefix(internal, …)` calls |

⚠️ `runtime.`, `access_logs.` and `tracing.` are **phase-79 additions** and
`sds.` is a **phase-80 addition**. Before phase 79 the roster was nine, and
between phases 79 and 80 it was one short of the figure above — which is why
in-tree prose elsewhere still carries both stale figures. Treat any count you
find here or elsewhere as stale until you have re-counted from the switch.

⚠️ **Four further detectors are MID-NAME (INFIX), not top-level** — `.rbac.`,
`.zookeeper.`, `.http_local_rate_limit.`, `.http_bandwidth_limit.`, declared as
the `rbacSegment` / `zkSegment` / `lrlSegment` / `blSegment` consts. They match
via `strings.Index` on any **dot-free leading segment**, so they accept more
*names* but add no *root*: `ANYTHING_AT_ALL.rbac.allowed` parses clean (residual
`rbac.allowed`), while the root-anchored `rbac.allowed` does **not** parse at
all. Counting them as roots is the standing documentation error this file used to
make. The top-level answer is **thirteen**, and the two species must never be
summed.

⚠️ **No `name.go` line numbers are cited above, deliberately.** Every cite this
file previously carried went stale inside a single phase. Grep the symbols
instead. The rejection is raised from the `noRecognizedSegmentErrFmt` const in
`internal/stats/name.go` and begins
`stats: name "runtime.num_keys" has no recognized top-level segment`, followed by
parentheticals listing the recognized sets — **not quoted here on purpose**; read
the const for its current text rather than trusting a copy in this file.

`internal/stats.WriteProm`'s `Walk` callback **skips** any metric whose
`flattenToProm` errors, and `runtime.` was not in that dispatch when the gauges
landed. Through phase 78 that skip was **silent** — nothing logged, nothing
errored — which is why the gap survived registration, the unit suite and `go vet`
alike. Phase 79 fixed **both halves**: the `runtime.` arm landed, **and**
`WriteProm` now emits one aggregated log line per call naming what it skipped. It
still returns no error, so the log is the only signal.

Consequences, both of which this row acts on:

1. **The value assertions read the flat `/stats` endpoint instead.** That is a
   sound cross-side seam *here specifically* because the two names carry no
   address and no dynamic segment, so the internal name is cross-side
   identical — the hazard behind
   `reference_listener_stat_scope_cross_side_divergence` (which forces listener
   stats through `/stats/prometheus`, where the address is a *label*) does not
   arise. Asserting `/stats/prometheus` at phase 77, as that PLAN specified,
   would have left this row permanently red for a reason unrelated to
   `layered_runtime`. The flat legs are **kept** even now that the prometheus
   side works: they are what distinguishes a wrong *gauge* from a wrong
   *renderer*.
2. **The prometheus exposition is asserted, symmetrically, by
   `assertPrometheusExpositionParity`** — prose alone would not hold it. ⚠️ **That
   function superseded a departure pin, and the flip is the point.** Through
   phase 78 it asserted the subject was *missing* both prometheus names, and it
   was written to go RED the day `internal/stats` learned a `runtime.` segment.
   **Phase 79 is that day**: the arm landed, the pin went RED exactly as
   designed, and it was **converted — not deleted** — into a cross-side parity
   assertion (present on both sides, equal values, still zero-label). Deleting it
   would have left the prometheus projection of these gauges with **zero**
   assertions on either side while reading as cleanup.

The fix — teaching `ExtractTags` a `runtime.` passthrough — is a change to a
shared, byte-gated component with its own name-mapping tests and is **not** part
of this fixture's scope.

## The four arms, and what each discriminates

Both config files carry a **byte-identical** `layered_runtime` block with two
layers, `L1` (four arms) and `L2` (the overlap re-declaration).

| arm | config | contributes | what it discriminates |
|---|---|---|---|
| **A** | `ov.key: "from_L1"` in **L1** *and* `ov.key: "from_L2"` in **L2** | **1** key | **UNION vs per-layer SUM.** An implementation that ADDS per-layer key counts instead of unioning them reads 7 here, not 6. It also pins that a literal `.` inside a field NAME is **not re-split** — `ov.key` is ONE key whose name contains a dot, on both sides. |
| **B** | `nest: {mid: {leaf1: 1, leaf2: 2}}` | **2** keys | **Unbounded-depth leaf flattening** — descend until a scalar, joining path segments with `.` → `nest.mid.leaf1`, `nest.mid.leaf2`. |
| **C** | `frac: {numerator: 25, foo: 2, bar: 3}` | **1** key | **The LEXICAL termination rule.** A *single* lowercase `numerator` beside two unrelated siblings. An implementation matching the `{numerator, denominator}` **pair** recurses here and yields **3**; the real rule terminates and yields **1**. Spelling the full pair would pass against **both** implementations and discriminate **nothing**. The values are **never inspected** — a `FractionalPercent` parse here would reject configs the reference accepts. |
| **D** | `emp: {e1: {}, e2: {}}` | **2** keys | **The SECOND termination branch.** An **empty** Struct is a **counted LEAF**, not zero keys: this yields `emp.e1` and `emp.e2`. No document before the phase-77 SPEC recorded this branch, and the inherited three-arm pin set could not have detected its absence. |

`1 + 2 + 1 + 2 = 6` distinct keys across **2** declared layers.

The reference numbers are **measured**, not extrapolated: the exact shipped
block was booted on the pinned reference image with a **fresh container per
arm, three runs per arm**. The combined config read `6 / 2` on all three runs,
and the four isolation arms contributed `1 / 2 / 1 / 2`, summing **exactly** to
the combined total — so no arm's contribution is mis-attributed and there is no
cross-arm interaction. Two controls bracketed the readout (a single-key
positive control at `1 / 1`, and a no-`layered_runtime` baseline at `0 / 0`), so
a stuck-red *and* a stuck-constant gauge would both have been visible.

## ⚠️ The L1/L2 `ov.key` overlap is LOAD-BEARING

Remove L2's re-declaration of `ov.key` and every key becomes unique, so the
per-layer **SUM** equals the **UNION**. Arm A then goes **vacuous while still
passing**: a build that adds per-layer counts reads 6 and the fixture stays
green. The overlap is the only thing in the config that separates the two
semantics, and it must not be "simplified away".

## ⚠️ STATS ONLY — the `/runtime` admin body is not comparable either

`ProbeAdmin` returns **`/ready` only**. The runner's `compareAdminResponses`
compares the body **byte-exact**, and the reference's `/runtime` body fails that
in four independent ways, all measured on the pinned image:

1. the JSON key order is randomized **per request**;
2. the Struct debug-string marker is randomized **per process** (8 distinct
   strings across 13 fresh processes);
3. an empty-map value renders as a leaked, non-deterministic `DebugString` — and
   **still counts** as a key;
4. the **within-layer collision winner is non-deterministic** (~40/60 over 18
   fresh processes), so any fixture asserting a `final_value` is unrunnable.

All four contaminate the **body only**. The two gauges are immune, which is why
this row asserts through `fixture.StatsAsserter` instead.

## ⚠️ No precondition is available from the other `runtime.*` names

`runtime.load_success` and `runtime.override_dir_not_exists` both read **1** on
the reference *even with no `layered_runtime` block at all*, so **neither is a
"a static layer actually loaded" guard**. The driver's honest substitute is the
**separate ABSENT check** — which `continue`s, so a gauge that failed to
*register* cannot pass vacuously as `0 == 0` — plus `num_layers == 2`.

## ⚠️ envoy-go publishes 2 `runtime.*` names; the reference publishes 9

Measured on the pinned image: `admin_overrides_active`,
`deprecated_feature_seen_since_process_start`, `deprecated_feature_use`,
`load_error`, `load_success`, `num_keys`, `num_layers`, `override_dir_exists`,
`override_dir_not_exists`. envoy-go publishes only the last two of those (well,
`num_keys` and `num_layers`).

The project asserts **named subsets** cross-side, never full-set equality
(`reference_stats_sink_emits_used_only`), so the asymmetry creates no divergence
here. A future row asserting full `runtime.*` name-set equality **will** fail,
and correctly so.

## ⚠️ This row REJECTS three sibling `oneof` arms the reference ACCEPTS

`layered_runtime.layers[].layer_specifier` is a four-arm `oneof`. envoy-go
accepts **`static_layer` only** and rejects the other three with byte-stable
`bootstrap: …` messages:

| arm | reference (pinned image) | envoy-go |
|---|---|---|
| `static_layer` | boots | **accepted** |
| `disk_layer` | **boots cleanly** | rejected |
| `admin_layer` | **boots cleanly** | rejected |
| `rtds_layer` | **boots cleanly** | rejected |

These are **DEPARTURES, not parity**. The founding premise of the earlier reject
roster — that the reference also refused these arms — was refuted by probe: all
three unmarshal *and boot*. They are rejected here as a deliberate scope
decision (row 77 ships no disk, admin or RTDS runtime layer), and the roster
also rejects an empty `layers` list, an empty layer `name`, a duplicate layer
name, an unset `layer_specifier`, and list- or null-valued static-layer leaves.
This fixture exercises **only the accepted arm**; the nine reject arms are
pinned at the unit level in `internal/bootstrap`.

## Break coverage

`num_keys` alone cannot identify *which* arm broke (arms B and D both yield 4),
so the discriminating breaks live at the unit level where the key **SET** is
available. This fixture carries the breaks the unit test cannot see:

| break | expected |
|---|---|
| delete the `num_keys` gauge **registration** | `subj: runtime.num_keys ABSENT from /stats` (the ABSENT branch, not a value mismatch). ⚠️ Break `num_keys` only — `num_layers` is `Set` to a non-zero value, so breaking *its* registration nil-pointers the subject process before `AssertStats` runs. |
| transpose the two gauge `Set` calls | `subj … = 2, want 6` **and** `subj: the two gauges look TRANSPOSED` |
| remove the blank import in `runner_test.go` | the guarded run prints `GATE FAIL: selector matched NOTHING`; the **bare** `-run` form prints `[no tests to run]` and **exits 0** |
| rename `AssertStats` | the guarded run prints `GATE FAIL: AssertStats did NOT run` — and the suite otherwise stays **GREEN** |
| change `wantNumKeys` 6 → 5 | **both** `ref … = 6, want 5` and `subj … = 6, want 5`, proving the leg is live on both sides |
