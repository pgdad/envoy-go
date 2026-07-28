# SPEC 79 — `/stats/prometheus` PROJECTION completeness: three byte-mirror `ExtractTags` arms make TEN silently-dropped stat names project, and the silent skip itself becomes OBSERVABLE (the TWENTY-SECOND §9 Observability-family row — **+0 stats (1207 → 1207) / +0 differential fixtures (120) / +0 fuzzers (55) / +0 BackendKinds (tail 38) / +0 go.mod modules / +0 packages (73) / +0 new PUBLIC surface**)

**Stage:** SPEC (lifecycle-state `1` → 2). Row 79 stays `in-progress`; `ROADMAP.md` is **BYTE-UNTOUCHED** at this stage. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Method:** four investigation agents on disjoint remits, each in its own detached worktree with private scratch and a private port band, plus controller re-derivation of every load-bearing claim. Every figure below was **executed at this tip (`316ba895`)**; a BRAINSTORM cite is not evidence (`feedback_brief_citations_not_evidence`).

---

## 1. Purpose / Mission

### 1.1 ⚠️ HEADLINE ONE — D-SPP-1 IS **CLOSED IN THE ROW'S FAVOUR**, MEASURED AGAINST A LIVE REFERENCE, AND THE ROW SHIPS **THREE** ARMS

The BRAINSTORM called D-SPP-1 *"the row's LARGEST UNMEASURED RISK"*: if reference Envoy **hoists** the middle segment (`grpc_access_log`, `zipkin`) into a **label** rather than inlining it, byte-mirror is the wrong shape, those arms become tag-hoisting changes, and the row ships **two** arms with the rest deferred to 79.1.

**Measured, not reasoned.** Six arms against the pinned `envoyproxy/envoy:contrib-v1.37.2`, **fresh container per arm** (`feedback_probe_fresh_container_per_arm`), on a **bridge network** (`reference_docker_probe_bridge_network`), each torn down **BY NAME** (`reference_parallel_agents_shared_machine_namespaces`); teardown proof `docker ps -a --filter name=p79a1-` **EMPTY**.

| root | verdict |
|---|---|
| `runtime.` | **BYTE-MIRROR CONFIRMED** |
| `access_logs.` | **BYTE-MIRROR CONFIRMED** (both sink types) |
| `tracing.` | **BYTE-MIRROR CONFIRMED** (both tracers) |

Verbatim reference output that settles it:

```
envoy_runtime_num_keys{} 6
envoy_runtime_num_layers{} 2
envoy_access_logs_grpc_access_log_logs_written{} 3
envoy_access_logs_open_telemetry_access_log_logs_dropped{} 3
envoy_tracing_zipkin_spans_sent{} 3
envoy_tracing_opentelemetry_spans_sent{} 3
```

⚠️ **THE POSITIVE CONTROL REPRODUCED `0118`'s PIN EXACTLY** (`runtime.num_keys: 6`, `num_layers: 2`), so the harness is validated and the other arms' results stand. ⚠️ **THE MIDDLE SEGMENT IS THE SINK TYPE, NOT THE OPERATOR'S `log_name`** — proven by grep **count**, not by absence of a match: the deliberately distinctive `zz_operator_chosen_name` / `yy_operator_chosen_name` / `zz_svc` appear **ZERO** times in any of the six dumps (828–1159 lines each, so no filtered result is an empty-file artifact). **Negative control:** an arm with no `access_log` list and no `tracing` block emits **0 lines** for both roots on **both** endpoints, while the positive arms return 2/2, 6/6, 3/3.

⇒ **`ExtractTags` must return `residual == input, labels == nil` for all three roots — the exact SN5 `server.` shape. This row ships THREE arms. Nothing moves to 79.1 on this account.**

### 1.2 ⚠️ HEADLINE TWO — THE D-SPP-3 **COUNTER FORM IS A PROCESS HANG**, NOT A "+1 STAT" DECISION

The BRAINSTORM argues against the counter on two grounds: +1 stat surface, and a self-referential naming trap. **Both are the weak objections, and the real one is a lock inversion no prior document names.**

`Registry.Walk` (`internal/stats/registry.go:139-140`) holds `r.mu.RLock()` **for the whole iteration**; `getOrRegister` (`:179`) takes `r.mu.Lock()`. Go's `sync.RWMutex` is **not reentrant**. `WriteProm`'s skip site is *inside* that callback. Injecting exactly the counter D-SPP-3 describes — `r.NewCounterIfAbsent("server.prom_name_skipped").Inc()`, a name that **does** satisfy SN5, so the naming trap is avoided — under `go test -timeout 20s`:

```
panic: test timed out after 20s
goroutine 6 [sync.RWMutex.Lock]:
sync.(*RWMutex).Lock → (*Registry).getOrRegister → NewCounterIfAbsent
  → WriteProm.func1 → (*Registry).Walk → WriteProm
```

⚠️ **A lazily-registered counter at the skip site DEADLOCKS the admin scrape.** And phase 78 is the row that just finished making a silent hang visible — shipping a new one here would be an unusually poor joke. **D-SPP-3 resolves to the LOG form** (§3.3); a counter is admissible only if pre-registered **eagerly outside `Walk`**, which changes the whole shape of the leg.

### 1.3 ⚠️ HEADLINE THREE — THE `:350` ERROR-STRING FIX **AS SPECIFIED IS ITSELF WRONG**

BRAINSTORM §0.b's indictment is correct — the terminal error omits `wasm.`, has been stale since phase 25.1, and is where the lineage's "FOUR" came from. **But its proposed repair reproduces the same defect one generation on.** §0.b's root sweep probed only the roots it already suspected. Re-run with the `default`-branch `CutPrefix` detectors added:

```
LIVE (9): [cluster. http. listener. server. wasm. mongo. kafka. redis. thrift.]
ARM-ABSENT (7): [runtime. access_logs. tracing. sds. listener_manager. filesystem. main_thread.]
```

`mongo.` `kafka.` `redis.` `thrift.` are **top-level** prefix detectors (`name.go:286 :306 :323 :340`), recognized by the very function whose error claims they are not. **There are NINE recognized top-level segments, not five.** A naive two-token `+wasm.` edit ships a **fifth** generation of a wrong list. ⚠️ **The in-tree prose at `0118/expectations.yaml:41-45` and `0118/README.md:32-34` already gets closer than the code does** — and those are the documents §0.b did not consult.

### 1.4 BRAINSTORM drift ledger — RE-DERIVED, CONFIRMED, REFUTED

| # | claim | verdict at this tip |
|---|---|---|
| R1 | 30/30 dropped; controls 16/16 + 4/4 near-miss projected | **CONFIRMED** — re-run, not cited. Family composition `access_logs.` 4 / `tracing.` 4 / `sds.` 20 / `runtime.` 2 exact; **1** distinct error shape across all 30 |
| R2 | the near-miss control `http.<sp>.tracing.spans_sent` | ⚠️ **REFUTED AS AN INPUT — the name is FABRICATED.** It is registered nowhere. The real HCM-scoped set is `{client_enabled, health_check, not_traceable, random_sampling, service_forced}` (`internal/tracing/stats.go:27-38`). **The finding survives on real inputs**; the SPEC swaps them (`reference_probe_input_is_a_claim`) |
| R3 | `WriteProm` emits **116 B** on a 5-metric registry | ⚠️ **CORRECTED to 114 B.** The probe registered `server.live` as a **counter**, copied from a *test* file; production registers a **gauge** (`internal/admin/admin.go:64`). `counter`(7) − `gauge`(5) = the 2-byte delta exactly. **Cite 114** |
| R4 | the `switch` sits at `~:100-110` | ⚠️ **REFUTED — off by ~50 lines.** `switch {` is at **`:50`**; `:100-110` is inside the **`wasm.` arm's comment block**. An IMPL agent following this cite edits a comment |
| R5 | *"FOUR top-level prefixes" is wrong; there are FIVE* | **CONFIRMED as a correction, INSUFFICIENT as a fix** — see §1.3. There are **NINE** |
| R6 | four non-prometheus sinks byte-identical under a byte-mirror arm | **CONFIRMED and EXTENDED** to all three arms with a **firing** negative control — §3.7 |
| R7 | the blast radius is prometheus-only | **CONFIRMED**, and the denominator is stated: **exactly 5** non-test `ExtractTags` call sites repo-wide ⇒ four non-prometheus consumers, the complete set |
| R8 | `0118` is a live, executing absence assertion that goes RED on success | **CONFIRMED BY THE FULL RED-THEN-GREEN CYCLE** — §3.5 |
| R9 | *"delete the pin in favour of the generic prometheus comparison"* is an option | ⚠️ **REFUTED — no such comparison exists.** `0118`'s `ProbeAdmin` returns `/ready` **only**; there is no generic prometheus differential anywhere in the runner. Deleting leaves that surface with **zero** assertions on **either** side ⇒ **CONVERT, not delete** |
| R10 | the ADR range-extraction hazard (`^## ADR-0107` matches nothing) | ⚠️ **REFUTED AT THIS TIP.** It matches **1** line (`:4304`); `:4858` begins `##` at byte 0; **zero** lines in the file begin space-then-`#`. Controller-verified independently. `^## ADR-` is safe. *(The one number with no heading is **ADR-0209**, an unconsumed reserve, not a gap.)* |
| R11 | cost calibration is *"three-for-three at or above the SPEC ceiling"* | ⚠️ **CORRECTED to TWO-for-three.** 76: 7-9 → **9** (at); 77: 11-13 → **12** (*inside*, one below); 78: 7-9 → PLAN 10 → **10** (above). The three numbers are right; the label on them is wrong |
| R12 | `go test ./internal/... -count=1` is green with the arms added | **CONFIRMED ON RE-RUN, but NOT one-shot.** Attempt 1 exited 1 on `TestServerConn_TinyWindowDelivery`; diagnosed unrelated by `go list -deps` (that package has **zero** dependency on `internal/stats`), isolated re-run green, attempt 2 = 70 `ok` |
| R13 | §6's edit-site table is complete | ⚠️ **REFUTED — it has NO `BEHAVIOR_CONTRACT.md` row at all**, and `:5020` is a **mandatory** edit regardless of D-SPP-3 (§7) |
| R14 | `prom.go:38-41` is the skip block | **NEAR-MISS.** `:38` is the `flattenToProm` call; the block is **`:39-41`**, bare `return` at **`:40`**. ⚠️ **And it is a per-metric `continue` inside the `Walk` closure, NOT an early exit from `WriteProm`** — any text saying "WriteProm returns early" is wrong |
| R15 | pin block 1 spans `:142-166` | **CORRECTED** — `:166` is mid-sentence. The natural units are **`:142-162`** and **`:164-169`** |
| R16 | `0091:164-166`; *"31 test files carry `envoy_` literals"* | **NEAR-MISS / QUALIFIED.** The `subset` var is `:163-167`. The 31 is **files under `test/`**, of which **0** are `_test.go` — not 31 `_test.go` files and not repo-wide. ⚠️ **And `"envoy_` is the WRONG detector**: `0091`/`0112`/`0113` carry **zero** such literals (they key on dotted residuals), so re-deriving the roster that way reproduces the undercount it was meant to fix |

### 1.5 SPEC-time verification record

**EXECUTED at this tip:** the live-reference probe, six arms + cross-controls + a null arm · the 30/30 defect re-run with a 16/16 + 4/4 discriminating control · the `WriteProm` byte measurement (114 B, 1 of 5 metrics, `nil` returned) · the counter **deadlock** with a goroutine trace · the counter's **walk-order-stale** value · the self-referential trap, both arms · the `helpText` degradation (`# HELP envoy_runtime_num_keys envoy_runtime_num_keys`) · a green suite with a deliberately **wrong 16th** `helpText` entry · the observability stacked control with **three** negative controls, two tripping **different** legs · the two `helpText` reverse guards with a **four-arm** control matrix · `0118` red-then-green · the four-sink byte-identity with a **firing** hoisting negative control · the nine-segment root sweep · all three sentinel checks with firing negative controls.

**NOT verified — carried as claims:** the absolute stat surface **1207** (documentary; **assert the DELTA**) · the full 120-fixture differential (deliberately not run at a docs-only stage; **mandatory at the IMPL**) · h2spec.

---

## 2. Non-purposes (deferred; BRAINSTORM §1.2 + §8)

1. **The `sds.` LABEL-HOISTING arm — 20 of the 30 names.** ⚠️ **It is the one arm that BREAKS §3.7's no-op result, and that is now EXECUTED rather than argued** (§3.7's negative control). Needs `envoy_xds_resource_name` re-pinned against a live dockerized reference.
2. **Registration-time validation** — the "impossible by construction" endgame; it panics at boot. ⚠️ **Per ADR-0300 §Consequences (ii), any injected-defer probing there MUST VARY THE INSERTION POINT across the pre- and post-anchor windows** — a single pre-anchor injection reports "caught" and conceals the genuinely uncovered case.
3. Both go to a banked row **79.1**, costed at **~7-9**.
4. **The `# HELP` block-level departure is NOT closed here** (§3.4).

---

## 3. The change — the D-SPP-* docket disposed one-for-one

### 3.1 D-SPP-1 **[CLOSED BY EXECUTION — three arms, byte-mirror, nothing deferred]**

See §1.1. All three arms take the SN5 shape: `residual = input`, `labels = nil`. No dot→underscore pre-transform belongs in `ExtractTags` — `flattenToProm` already does that at projection time, and the reference confirms every internal dot becomes `_`.

⚠️ **ONE NAME HAS NO REFERENCE COUNTERPART.** The reference's zipkin family is `{reports_dropped, reports_failed, reports_sent, reports_skipped_no_cluster, spans_sent, timer_flushed}` — there is **no `spans_dropped`**. envoy-go registers `tracing.zipkin.spans_dropped` (`internal/tracing/stats.go:71`). ⇒ **of the ten names this row makes project, NINE have a reference counterpart and ONE is envoy-go-only.** Harmless under the named-subset doctrine, but **any SPEC/IMPL/contract text asserting "these ten now match the reference" is wrong**, and a cross-side assertion on that name would be unsatisfiable.

⚠️ **ZERO-LABEL BRACE DIVERGENCE.** The reference writes `envoy_runtime_num_keys{} 6`; envoy-go's `writeMetricLine` **omits `{}`** for empty label sets. Semantically equivalent, **not byte-identical** — so §3.5's parity flip is safe only *through* `0118`'s existing parser (`driver.go:399-403`, which handles both forms), **never** via a raw-line comparison.

### 3.2 D-SPP-2 **[RECORDED for 79.1 — the rule REPRODUCES, and the naive derivation is right for the wrong reason]**

Statically **5** name templates (`internal/xds/stats.go:34-38`); the 20 is `5 × 4` distinct secrets. **The rule is *"distinct secrets reaching `internal/boot/boot.go:201`"*, NOT *"distinct secrets in the corpus"*.** `RegisterSDSStats` has **exactly one** non-test caller — confirmed. The four contributing secrets re-derived from the fixtures whose `sds_config` is an `api_config_source`: `server_cert` (`0103`), `validation_ca` (`0108`/`0109`), `rccf_validation_ca` (`0110`), `edf_validation_ca` (`0111`). `0024-http-oauth2`'s `client_secret`/`hmac` use `path_config_source` and are served by `internal/sdsfile.Watcher`, never the xds gRPC provider. ⚠️ **The corpus rule yields `6 × 5 = 30` and reads as confirmation of the 30-name total by coincidence.** Applies to 79.1.

### 3.3 D-SPP-3 **[RESOLVED BY EXECUTION — the LOG, and the counter is disqualified on FOUR independent executed grounds]**

**DECISION: an aggregated `log.Printf`, ONE line per `WriteProm` call, names sorted and joined.** Stat surface stays **+0**.

The counter is disqualified by, in descending order of severity:

1. ⚠️ **It DEADLOCKS** (§1.2) — executed, with a goroutine trace.
2. ⚠️ **Its own emitted value is WALK-ORDER DEPENDENT and can be a full scrape stale.** `Registry.metrics` is a slice in registration order; incrementing inside `Walk` means the counter's own line reflects only increments occurring *before its own position*. Executed: registered **first** ⇒ scrapes read `0, 2, 4` (**always one scrape behind**); registered **last** ⇒ `2, 4, 6` (current). Registration order is a boot-sequencing accident. **This defect is intrinsic to instrumenting a walk with a metric inside that walk and has no analogue in the log form**, which fires after the walk completes.
3. **The self-referential trap is real** — executed both arms: under a naive name the instrumentation counter is **ABSENT** from its own output; under `server.` (SN5) it is **PRESENT**. The dropped control drops in both arms, so the probe discriminates.
4. **It would owe a `helpText` entry too**, or ship degraded on the very line it adds — observed: `# HELP envoy_server_stats_prom_name_skipped envoy_server_stats_prom_name_skipped`.

**A logger is reachable at zero envelope cost — measured.** `internal/stats` (non-test) imports only `fmt`, `io`, `sort`, `strings`, `regexp`, `sync`, `sync/atomic`. The canonical project sink is the stdlib `log` package, and `WriteProm`'s own caller already uses it (`internal/admin/prometheus.go:24`). With the log form applied: `go build ./...` **0** · `git diff go.mod go.sum` **0 bytes** · zero non-stdlib deps added · `golangci-lint run ./internal/stats/...` **0** · `gofmt -l` empty. `reference_new_subpackage_pulls_transitive_module` does not fire.

**Frequency: ONE call per admin scrape.** `WriteProm`'s only non-test production caller is `internal/admin/prometheus.go:23` (`handlePrometheus`, registered at `admin.go:94`). Nothing polls it. Aggregated, the real 30-name roster produces **1 line / 1151 bytes**; per-name would produce **30 lines per scrape**. ⚠️ **The noise is SELF-EXTINGUISHING: this row takes 30 → 20, and 79.1 takes it to 0 names ⇒ 0 lines.**

⚠️ **`sync.Once` IS REJECTED, AND THE REASON IS EXECUTED.** With a package-level `Once`, two in-package tests that each exercise the signal cannot both pass — the first to run consumes it (`INNER EXIT=1`, `skip signal fired 0 time(s), want EXACTLY 1`); the same two tests pass against the aggregate form (`INNER EXIT=0`). A once-only signal is **order-dependent, untestable beyond one test, and suppresses a later regression forever.** **Do not specify `sync.Once`.**

⚠️ **THE `prom.go:18-22` COMMENT CONTRADICTS ITSELF INSIDE ONE SENTENCE** — *"silently skipped … log+ignore"*. It is not merely wrong about the code; it is internally inconsistent. **It must be rewritten to describe what the code now does.** Leaving it after adding the log ships a third generation of the inconsistency.

### 3.4 D-SPP-4 **[RESOLVED BY EXECUTION — YES, it rides this row, but with the NINE-segment enumeration of §1.3]**

**No test asserts the string, byte-for-byte or partially.** Repo-wide, `has no recognized top-level segment` → **1** `.go` source hit (`name.go:350`) plus two non-executing in-fixture hits (`0118/expectations.yaml:46`, a `#` comment; `0118/README.md:35`, prose), neither containing the `(want …)` parenthetical. `want cluster.` → the same line plus 5 unrelated `statssink` test messages of the form `want cluster.upstream_rq_total`, which match the substring only.

**Proven by execution** — string replaced with the nine-segment form: `go build ./...` **0** · `go test ./internal/stats/ -count=1` **0** · `go test ./internal/... -count=1` **0**, 70 `ok`. Reverted; byte-identical to pre-edit.

⚠️ **The IMPL must ship a BYTE-STABLE guard on the corrected string** (the phase-77 `TestParseRejectConstants_ByteStable` precedent). Without one this row's own fix is exactly as unguarded as the defect it repairs, and a tenth arm ships a sixth generation. ⚠️ **`golangci-lint` runs `misspell` with `locale: US`** — the replacement text is US-spelled and contains only identifier tokens. **Do not paste SPEC prose into code.**

### 3.5 D-SPP-5 **[RESOLVED — CONVERT the pin, do NOT delete it; liveness PROVEN first]**

⚠️ **THE LIVENESS CAVEAT WAS RUN BEFORE ANY EDIT** (`reference_liveness_break_needs_failing_baseline`), and the full cycle is the load-bearing artifact:

| step | `INNER_EXIT` | evidence it RAN |
|---|---|---|
| baseline, unmodified tip | **0** | `--- PASS: TestDifferential/0118-runtime-static-layer`; driver logged `subj_num_keys_present=false`. **No `[no tests to run]`**; subtest name matches the fixture dir exactly |
| **break** — inject the `runtime.` arm | **1** | `subj_num_keys_present=false → true`; **exactly** the two subject-absence `t.Errorf`s fired (`… is NOW PRESENT on /stats/prometheus (= 6) … has been CLOSED`), and **nothing else** — reference-side legs, flat-`/stats` value legs and transposition legs all stayed silent |
| revert | **0** | back to `--- PASS`, `subj_num_keys_present=false` |

**Subject emitted 6 and 2 — identical to the reference. A parity assertion will pass.**

**DECISION: convert, do not delete.** Deleting is unsafe (R9: no generic prometheus comparison exists in this fixture or the runner), and the **reference-side** leg has no substitute — the driver's own comment records that the value assertions moved *off* `/stats/prometheus`, so a reference-side regression would otherwise be invisible. Rename to `assertPrometheusExpositionParity`; keep the reference loop unchanged; replace the subject-absence loop with the same present-and-equal check, absence-check separate from value-check and `continue`-ing (mirroring the `:225-232` vacuity guard). **Keep the flat-`/stats` legs as a second seam** — they are the only thing that distinguishes "gauge wrong" from "renderer wrong".

⚠️ **A GAP THE IMPL MUST CLOSE: `scrapeProm` (`:407-447`) is LABEL-BLIND** — it keys on the bare name, splitting at `{` (`:429-435`). A parity assertion built on it would pass silently against a future hoisting arm that attaches labels. Given §3.1's brace divergence, the assertion needs a label-**aware** scrape, or an explicit stated limitation.

⚠️ **FAILURE-ATTRIBUTION NOTE FOR THE BREAK ROSTER:** failures report at **`runner_test.go:1349`**, not `driver.go:308` — `t.Helper()` plus the `fixture.TB` indirection collapses the driver frames. **A gate grepping for a `driver.go` line number will not match.**

**Also in scope:** `internal/admin/stats.go:12-22`. It *does* document the silent-skip mechanism, so the BRAINSTORM's corroboration claim holds in substance — but its motivating example is `redis.`, and its parenthetical *"(the redis. Prometheus tag-extractor arm is 32.2)"* is **STALE**: that arm landed (`name.go:323`). It is a second live instance of the §1.3 species — stale documentation propagating a wrong picture — and is cheap to fix in a file this row already reasons about.

### 3.6 D-SPP-OBS **[NEW AT THIS SPEC — the observability leg's test, designed, built and negative-controlled in BOTH directions]**

⚠️ **A POSITIVE ASSERTION CANNOT CATCH AN OVER-FIRING SIGNAL** (`reference_positive_arm_cannot_catch_overfiring`). The design is a **stacked control**: one projecting name and one dropped name in the same registry.

- Capture: `log.SetOutput(&buf)` + `log.SetFlags(0)`, restored via `t.Cleanup`. **Confirmed constructible** — `go test ./internal/stats/... -count=1 -race` **INNER EXIT 0** with the helper live. Not `t.Parallel()`; `internal/stats` has no other `log` user.
- Registry: leg A `server.live` (live SN5 arm) must **NOT** be signaled; leg B `runtime.num_keys` (no arm at this tip) must be.
- **Four assertions, each its own `t.Errorf`** so none is dead code (`reference_fatalf_makes_assertions_unreachable`): (1) **exactly one** non-empty log line; (2) the line **names** `runtime.num_keys`; (3) **negative leg** — it must **NOT** name `server.live`; (4) liveness cross-check that `envoy_server_live 1` really is in the exposition, so leg 3 is not vacuous.

**Executed matrix — and this is why leg 3 exists:**

| arm | `INNER EXIT` | which assertion fired |
|---|---|---|
| positive (aggregate log) | **0 PASS** | captured `stats: WriteProm skipped 1 registered metric name(s) …: runtime.num_keys` |
| **NC-1a** over-fire, aggregate over every name | **1 FAIL** | **leg 3** — *"OVER-FIRES: it names `server.live`, which PROJECTED successfully"* |
| **NC-1b** over-fire, per-name (2 lines) | **1 FAIL** | **leg 1** — *"fired 2 time(s), want EXACTLY 1"* |
| **NC-2** never fire — **the shipped tip** | **1 FAIL** | **leg 1** — *"fired 0 time(s)"*, `captured log (0 bytes)` |

⚠️ **The two over-fire arms trip DIFFERENT legs** (`reference_deliberate_break_wrong_assertion`), and **a positive-only test would have passed all three negative controls.** NC-2 doubles as the required failing baseline: the assertion is proven RED on today's tree before anything is fixed.

### 3.7 D-SPP-SINK **[the sink-side audit — §0.a CONFIRMED, EXTENDED, and NOT vacuous]**

**Denominator stated, and this is an AUDIT not a sample** (`reference_sample_is_not_an_audit`). Exactly **5** non-test `ExtractTags` call sites repo-wide: `internal/stats/name.go:371` (prometheus, via `flattenToProm` — the function itself begins at **`:370`**), plus `internal/statssink/{label.go:38, dogstatsd.go:82, graphite.go:67, otlp.go:189}`. ⇒ **four** non-prometheus consumers, the complete set. `StatsdSink`/`TCPStatsdSink` do not call `ExtractTags` and are unaffected by construction.

**Executed BEFORE/AFTER with all three arms**, real wire bytes (loopback UDP for dogstatsd/graphite, prototext for metrics_service, a captured `ExportMetricsServiceRequest` for OTLP over the **full `useTagExtractedName` × `emitTagsAsAttributes` 2×2 cross-product**), inputs derived from registration sites, determinism proven by two identical BEFORE runs (`diff` exit 0):

```
### EXTRACT   10 × residual="" err=true  →  residual=<full name> err=false
### PROM      2 lines / 304 B            →  12 lines / 2171 B
### LABELMAP / DOGSTATSD / GRAPHITE / OTLP ×4 knob arms  →  ZERO diff lines
```

Controls: `listener_manager.lm_total` **stayed dropped** (the arms do not over-fire); `cluster.*` **kept its hoisted label** (no collateral).

⚠️ **NEGATIVE CONTROL — THE AUDIT CAN FAIL.** Replacing the `tracing.` byte-mirror with a **tag-hoisting** arm moved **all four** sinks (**152** diff lines): `LABELMAP` gained `label:{name:"envoy.tracer_name"}`, dogstatsd gained `|#envoy.tracer_name:opentelemetry`, graphite gained `;envoy.tracer_name=opentelemetry`, OTLP proto bytes moved `1154 → 1292`. **This is direct executed evidence for §2's `sds.` deferral rationale.**

⚠️ **THE GATE MUST BE A UNIT TEST, NOT A FIXTURE GATE — and that is a NEW finding.** Mechanically: **10** fixtures configure `stats_sinks`; **0 of 10** configure tracing or a gRPC/OTel access log. **A fixture-level sink audit would be VACUOUS for two of the three arms.** (`runtime.*` *is* live-covered — those gauges are registered unconditionally and are already on every stats-sink fixture's wire via the error-fallback path; `0091`/`0093`/`0101`/`0112` were run green with the arms in.) The IMPL ships a golden **byte** gate in `internal/statssink` asserting each consumer's emitted bytes equal `<prefix>.<full dotted name>` with **zero** labels — asserting the **whole emitted set**, not per-name presence (`reference_stat_count_guard_blind_to_rename`) — with `cluster.*` and `listener_manager.*` as stacked controls, the OTLP 2×2 knob cross-product, OTLP `*_unix_nano` normalized, and the hoisting arm re-run **at PLAN time** as the mandatory negative control.

### 3.8 D-SPP-HELP **[the `helpText` leg — the "completeness assertion" DOES NOT EXIST, and ONE guard is not enough]**

**The BRAINSTORM's claim is CONFIRMED BY EXECUTION.** The map is 15 entries (`name.go:458-476`), the prose *"Of the 15 entries"* is at `:448`, and `TestHelpText_Coverage` (`name_test.go:195`) iterates a **hand-listed 10-name slice** (`:196-206`) with the membership check at `:209`. There is **no reverse direction**; the file says so at `:231-236`. Proof: a deliberately wrong 16th entry (typo'd key, nonsense value) leaves `go test ./internal/stats/... -count=1` at **INNER EXIT 0** and `go build ./...` at 0. The degradation is real and observed — with the arms in, `# HELP envoy_runtime_num_keys envoy_runtime_num_keys`.

⚠️ **THE ROW MUST SHIP TWO GUARDS, AND THE FOUR-ARM MATRIX PROVES ONE IS INSUFFICIENT.**

- **Guard A — `TestHelpText_KeySetExact`**: set **equality** against a golden key slice, reporting `missing` and `extra` separately (not a count — `reference_stat_count_guard_blind_to_rename`).
- **Guard B — `TestHelpText_NoSelfEqualHelp`**: drives the **real projection**, parses every `# HELP <name> <help>` line and asserts none is **self-equal** — `prom.go:59-61`'s degradation signature.

| arm | Guard A | Guard B |
|---|---|---|
| clean tree | PASS | PASS |
| extra unlisted entry | **FAIL** | n/a |
| entry deleted | **FAIL** | **FAIL** |
| ⚠️ **key TYPO'd *and the typo copy-pasted into the golden list*** | ⚠️ **PASS — BLIND** | **FAIL** |

**The last row is decisive**: the golden-set guard alone is defeated by the single most likely authoring mistake. **Only the projection-driven guard catches it.**

⚠️ **THE ROSTER MUST BE DERIVED, NOT HAND-WRITTEN.** A first attempt used `listener.0.0.0.0_10000.…` and Guard B fired on 6 names — **a false positive from invented input**. SN3 splits on the first dot after `listener.`, so the real shape is the underscore-normalized `listener.0_0_0_0_10000.…`. **Derive it from `internal/listener/manager.go:381` `normalizeAddr` and the landed golden at `name_test.go:10`** (`reference_probe_input_is_a_claim`, which fired in practice here).

⚠️ **AND THE JUSTIFICATION FOR THIS LEG IS NOT PARITY.** The reference emits **ZERO `# HELP` lines** — `grep -c '^# HELP'` is **0** on all six reference dumps. envoy-go emits one per group. **The `helpText` leg is an envoy-go-internal quality choice that WIDENS an existing block-level departure; it cannot be justified as matching the reference.** §7 states it as a departure rather than implying parity.

---

## 4. Framework primitives — 0 new packages, 0 new modules, 0 new production deps

No framework gap. `ExtractTags` is a pure function over a string; the arms are additive `case` clauses in an existing `switch` (`:50`), and every consumer already handles both branches (§3.7). The observability leg adds **one stdlib import** (`log`) to `internal/stats` — measured at +0 go.mod modules, +0 transitive, +0 packages.

---

## 5. Identifier hygiene

New identifiers introduced by this row, checked for collision before adoption (`reference_spec_drafted_identifier_collision_check`): `assertPrometheusExpositionParity`, `TestHelpText_KeySetExact`, `TestHelpText_NoSelfEqualHelp`, `wantHelpKeys`, and the `internal/statssink` sink-identity test name. **The IMPL greps each before landing it.** No new **exported** symbol: `ExtractTags`'s signature is unchanged and the arms are internal `case` clauses ⇒ **+0 new PUBLIC surface**.

---

## 6. Stat surface **+0** (1207 → 1207) · Fuzz **+0** (55) · Fixtures **+0** (120)

Adding a `switch` arm is a **PROJECTION** change, not a **REGISTRATION** change: the thirty names are already registered and already incrementing. With D-SPP-3 resolved to the **log** (§3.3), no `NewCounter`/`NewGauge` call site is added anywhere. **+0 is now UNCONDITIONAL** — the BRAINSTORM's stated condition is discharged.

⚠️ **THE `+0` CLAIM CANNOT BE DISCHARGED BY THE `TestNoNewStat*` GUARDS, AND THE BLINDNESS IS NOW PROVEN, NOT ASSERTED.** All five live in `internal/statssink/registration_test.go` (`:26 :53 :81 :109 :137`, header `package statssink`, the exhaustive repo-wide set), each walking that package's own Registry. **None reaches `internal/stats`.** Executed with a **failing baseline**:

- **Arm A** — a genuine new registration injected inside `internal/stats`: **all five PASS** (`GUARD_EXIT=0`). ⚠️ **And they were green against a build that DEADLOCKS `WriteProm`** — they are blind not only to a stat-surface regression but to a process hang in the code they nominally cover.
- **Arm B** — the same registration moved to `NewRegistry()`: **all five FAIL** (`GUARD_EXIT=1`, *"fresh registry should have 0 metrics, got 1"*). ⇒ Arm A's green is **genuine blindness, not a no-run**.

⇒ **The argument is STRUCTURAL and must be stated as such**: enumerate the diff's registration call sites and show the set is empty. ⚠️ **The blindness cuts both ways — these guards would also miss a regression here.**

**Stat surface 1207 is DOCUMENTARY and CONFIG-CONDITIONAL — assert the DELTA, never the absolute.**

---

## 7. Behavior-contract edit map

⚠️ **THIS SECTION EXISTS BECAUSE THE BRAINSTORM'S EDIT-SITE TABLE HAS NO `BEHAVIOR_CONTRACT.md` ROW AT ALL** (R13).

1. **`BEHAVIOR_CONTRACT.md:5020` — MANDATORY, and owed on the +0 log path too.** It is a standalone note on the phase-77 ledger row titled *"⚠️ PHASE-77 DEPARTURE … the two `runtime.*` gauges are served on `/stats` but are SILENTLY ABSENT from `/stats/prometheus`"*, closing *"**The day `internal/stats` learns `runtime.`, fixture 0118 goes RED on purpose**, naming the follow-up."* **Phase 79 is that day.** The note must record that the departure is CLOSED for `runtime.`, `access_logs.` and `tracing.`, and that it **persists for `sds.`** pending 79.1. ⚠️ It also independently enumerates the dispatch as *"(`cluster.` / `http.` / `listener.` / `server.` / `wasm.`, plus the SN9 `local_ratelimit` default arm)"* — an **in-tree corroboration of FIVE arms**, independent of §0.b's probe, and now itself superseded by §1.3's NINE.
2. **A new stat-name-mapping statement** for the three arms, written as the SN5-shaped byte-mirror rule, naming the **nine** recognized top-level segments.
3. **The `# HELP` departure stated as a DEPARTURE** (§3.8), not as parity.
4. ⚠️ **`tracing.zipkin.spans_dropped` named as envoy-go-only** (§3.1) so no later row writes an unsatisfiable cross-side assertion.
5. **NO ledger row** — the surface is +0. ⚠️ **And per R3-of-A4, the non-H2 parallel total was abandoned three rows ago**: `grep -nE 'non-H2 \*\*[0-9]+\*\*'` returns 11 lines, newest `:5010` (phase 47.1), while the tail rows `:5014`/`:5016`/`:5018` carry none. **A future `+1` row would append a single-absolute row and need not touch a non-H2 total.**
6. ⚠️ **The three tail ledger rows each carry a `[LEDGER GAPS — RECORDED, NOT RESOLVED]` block** naming two unattributed steps (`1198→1200`, `1200→1201`) and each says the absolute *"must be re-derived MECHANICALLY"*. **Any future row that moves the ledger inherits that warning verbatim rather than silently asserting a new absolute.**

---

## 8. Sentinel maintenance — **this row narrows NOTHING**

Re-run mechanically at this tip by the controller, **with firing negative controls**, and **recorded rather than forecast** (`reference_sentinel_deferred_sentence_live_vs_historical`):

- **(1)** `NOT DONE: row 79` — **no `GATE FAIL` at `want=111`**. NCs: `want=109`, `110`, `112` each ⇒ `GATE FAIL: examined 111 data rows, expected <want>`.
- **(2)** **FIVE**, at `:189 :199 :209 :215 :223` — **UNCHANGED**. ⚠️ **Check (2) has NEVER gone down across ~22 phases. A stage that predicts a decrease repeats the phase-73 error.** The `:209` candidate sentence keeps its text until row 79 **CLOSES**.
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` — unchanged. NC: invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`.
- Input measured: **227 lines / 1 003 291 bytes / 13** bare `candidates:` hits (vs the sentinel's narrower 5) — so an empty result could not read as a zero result.

⚠️ **`want` STAYS 111. The SPEC adds no row and does not touch it.** ⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage**, so `reference_sentinel_matcher_string_self_clears` is dormant for one stage; **it re-arms at the IMPL**, which flips row 79 to `done`.

⚠️ **A NEW TRAP, FOUND AT THIS STAGE: `ROADMAP.md:209` carries SIX distinct task bands** (`5-7`, `9–13`, `10–14`, `7–11`, `8–12`, `11-14`). A line-scoped grep for the row's banked cost returns the **wrong** one — the projection band `11-14` sits at character offset **~45763** of a 45 959-character line. **Locate it positionally.** (The controller made exactly this error and caught it by re-reading with context.)

---

## 9. Test plan + task surface — ⚠️ **I DISAGREE WITH 9-11. THE BAND IS 10-12, BUDGET 12** — and the disagreement is the headline

⚠️ **THE BRAINSTORM'S 9-11 IS A RE-SCOPE OF A RE-SCOPE, AND `reference_deferred_candidate_cost_restale` CUTS AGAINST INHERITING IT.** Phase 78's PLAN refuted its own SPEC's band in **both** directions. This SPEC re-derives from a task decomposition.

⚠️ **AN INDEPENDENT AGENT DERIVED 11-13 FROM AN 11-TASK ROSTER — AND ITS ROSTER CONTAINED A TASK THIS SPEC HAS ALREADY DISCHARGED.** That agent's T5 was *"the D-SPP-1 dockerized reference probe … must precede T2."* **D-SPP-1 was executed at THIS stage (§1.1), so it is not IMPL work.** Its two ceiling-expanding contingencies are also both foreclosed by that same result: T2 *"splits into two tasks if the two arms differ in shape"* — they do not, all three are byte-mirror; and the ledger split is foreclosed by D-SPP-3 resolving to the log at +0. **Removing a discharged task and two foreclosed contingencies from 11-13 gives 10-12.** ⚠️ **This is a derivation, not a split-the-difference between 9-11 and 11-13.**

| # | task | gate |
|---|---|---|
| T1 | The three byte-mirror arms in `ExtractTags` (`:50` switch) + the SN-rule doc block (`:24-46`) | builds; `gofmt -l` **output** empty |
| T2 | `name_test.go` arm guards for the three arms — ⚠️ **absent by default; work this row CREATES, not tests it must keep passing** | red-then-green |
| T3 | The `:350` error string → the **NINE-segment** enumeration (§1.3) **+ a byte-stable guard** | red-then-green |
| T4 | `WriteProm` observability: the aggregated log + **rewrite the self-contradicting `:18-22` comment** | builds; lint clean |
| T5 | The observability stacked control + its **three** negative controls (§3.6) | each NC red, positive green |
| T6 | `helpText` ×10 + the `:448` prose count + **BOTH** reverse guards, roster derived from `normalizeAddr` (§3.8) | four-arm matrix |
| T7 | The `internal/statssink` sink-identity byte gate + stacked controls + the OTLP 2×2 knob cross-product; **hoisting NC run at PLAN time** (§3.7) | NC red across all four consumers |
| T8 | `0118` pin → parity (`assertPrometheusExpositionParity`), **liveness RED-first**, label-aware scrape; + `internal/admin/stats.go:12-22`'s stale parenthetical | red-then-green |
| T9 | `BEHAVIOR_CONTRACT.md` — `:5020` (**mandatory**, §7) + the mapping statement + the `# HELP` departure + the envoy-go-only name | grep-verified per site |
| T10 | The break roster, **each arm proven to fire its OWN assertion** (`reference_deliberate_break_wrong_assertion`) | each break red, restore green |
| T11 | Gates: `gofmt` + `golangci-lint` on touched packages; `go test ./internal/... -count=1`; **the FULL 120-fixture differential**; `-race` as a **second** run; h2spec | see below |
| T12 | ADR-0301 §Decision + §Consequences **IN PLACE**; row 79 → `done`; sentinel re-runs; `STATE.md` + `next-prompt.txt` roll; six-gate | ADR-0106 sole leg |

*(Floor **10** if T2 folds into T1 and T3 into T1; ceiling **12** as enumerated. **Budget 12.**)*

⚠️ **WHY 9-11 IS TOO LOW, GROUNDED IN WHAT WAS BUILT RATHER THAN ESTIMATED.** The §2.3 measurement that produced 7-9 established only that *adding arms breaks nothing*; it scoped none of T4-T7 or T9. **T5 and T6 are each larger than a one-line BRAINSTORM item** — T5 needs three negative-control builds run and re-run, T6 needs **two** guards (the four-arm matrix proves one is insufficient) plus a derived roster. **T9 is mandatory and was entirely unlisted.** And the corrected calibration (R11) still points up, just less hard: **2 of 3** at or above the ceiling, with the one that moved (78) moving **+1 above** after its PLAN re-derived.

**ADR-0045 does NOT trip.** 10-12 ≪ ~25 tasks; ~40 production + ~300-400 test + ~60 fixture ≈ **~500 LoC** ≪ ~1500. ⚠️ **CITE CORRECTION: ADR-0045 (`DECISIONS.md:1466`) does not STATE the gate — it QUOTES it** (`:1475`) from `BOOTSTRAP_PROMPT.md`, which carries the string twice (`:225`, `:472`). The figures are confirmed; **citing them to ADR-0045 alone is a laundered cite.**

⚠️ **THE FULL 120-FIXTURE DIFFERENTIAL IS MANDATORY** — `internal/stats` links into `cmd/envoy-go`, which is built at **three** sites: `test/differential/harness.go:240`, `:594`, and `test/conformance/h2spec/h2spec_test.go:210` (`TestH2Spec` entry `:30`), all three confirmed. ⚠️ **h2spec is a FOURTH consumer of the same binary and is NOT covered by `./test/differential/`** — the IMPL runs it explicitly rather than excluding it silently. Budget **~400-420 s** per green attempt; `-race` is a **second** run, not a substitute.

**The mandated recipe** (each clause grounded in a recorded failure mode):

```sh
( go test ./test/differential/ -count=1 -v > "$SCRATCH/full.log" 2>&1; echo "INNER_EXIT=$?" )
grep -cE '^    --- PASS: TestDifferential/' "$SCRATCH/full.log"          # want 120
grep -E  '^    --- (FAIL|SKIP): TestDifferential/' "$SCRATCH/full.log"   # want EMPTY
grep -c  'no driver registered for fixture' "$SCRATCH/full.log"          # want 0
grep -o  'TestDifferential/[^ ]*' "$SCRATCH/full.log" | sed 's|TestDifferential/||' | sort -u \
  | comm -3 - <(ls -1 test/fixtures/ | grep -E '^[0-9]{4}[a-z]?-' | sort)  # want EMPTY
```

`-count=1` (a cached PASS is not a run) · `-v` (**without it a green log is indistinguishable from a suite that ran nothing**) · **capture the INNER exit code**, never a harness's · tally scoped to `TestDifferential/` so the bare parent line is excluded · **the `comm -3` cross-check is the load-bearing gate, not the raw count** (120 with one fixture renamed and another skipped still reads 120) · assert `SKIP` empty explicitly, since `t.Skipf` on an unregistered driver is the silent-green path. **Verified at this tip: fixture dirs 120, blank imports 120, `comm -3` EMPTY.**

⚠️ **Known live hazards — never reflex-classify any as a regression.** The full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **before any assertion**, the latter a **PANIC that can abort the whole binary**, firing more readily under `-race` and as the fixture count grows — now **120**) · `reference_sds_init_fetch_timeout_dial_budget_flake` (**TWO** packages) · the pre-existing `internal/cluster` `-race` outlier flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce`) · **`internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`, which FIRED AT THIS STAGE** and is now evidenced rather than merely carried as prose: `go list -deps` shows that package has **zero** dependency on `internal/stats`, and the isolated re-run was green · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`. ⚠️ **A stage brief's flake list is not the index.** Isolate-re-run, then state the classification **and its evidence**. ⚠️ **`0061-lb-ring-hash` is NOT a live flake; a spread failure there is a FINDING.**

⚠️ **Gate hygiene — the lineage's broken-gate count is FOURTEEN.** `gofmt -l` never exits non-zero (gate on OUTPUT) · a full-suite recipe without `-v` is vacuous · a sha256 roster from one tree is desynced against a DELETED file · `go doc -all <A> <B>` swallows arg2 · a `+0 exported symbols` gate over an EMPTY package goes red on a correct tree · a RANGE gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` exits 0 on a deleted file · a count-only stat guard passes a build with BOTH names wrong · a `-run` no-match exits 0 with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests exceeds the fixture denominator · a stat-delta claim cannot be discharged by guards scoped to another package · a stderr-VOLUME assertion passes on the hang · `misspell` runs `locale: US` (⚠️ **LIVE this row — it edits Go comments AND an error string**; it fired on *"signalled"* during SPEC-time prototyping) · a harness's exit code is not the command's.

---

## 10. Edit-site roster — RE-DERIVED at tip `316ba895`

**Production (2 files):**
1. `internal/stats/name.go` — the three arms in the `switch` at **`:50`** (⚠️ **NOT `~:100-110`, which is inside the `wasm.` comment block**); the SN-rule doc block **`:24-46`**; the terminal error at **`:350`**; `helpText` **`:458-476`** (15 entries) and its prose count at **`:448`**. *(`flattenToProm` at `:370`, call at `:371` — unchanged.)*
2. `internal/stats/prom.go` — the skip block at **`:39-41`** (bare `return` at **`:40`**) and the self-contradicting comment at **`:18-22`**.

**Test (3 files):** `internal/stats/name_test.go` (959 lines, 55 `^func Test`, 56 top-level funcs, 0 `^func Fuzz`) — arm guards, the byte-stable string guard, both `helpText` guards; a new observability test in `internal/stats`; a new sink-identity test in `internal/statssink`.

**Fixture (1 file):** `test/fixtures/0118-runtime-static-layer/driver/driver.go` — pin blocks **`:142-162`** and **`:164-169`**, call site `:266`, function `:269-315`, `scrapeProm` `:407-447`.

**Also:** `internal/admin/stats.go:12-22` (stale `redis.` parenthetical).

**Docs:** `DECISIONS.md` (ADR-0301) · `BEHAVIOR_CONTRACT.md` (§7) · `ROADMAP.md` (row 79 → `done`, **at the IMPL only**) · `STATE.md` · `next-prompt.txt`.

⚠️ **Non-colliding negative assertions confirmed:** `name_test.go:141` (`unknown_top_segment.foo`) and `:956` (`listener_manager.listener_create_success`). Neither begins with `runtime.`/`access_logs.`/`tracing.`.

---

## 11. Deferred items — named so no later stage re-derives them

1. **Row 79.1** — the `sds.` label-hoisting arm (20 names) + registration-time validation (~7-9).
2. **The `# HELP` block-level departure** — the reference emits none; envoy-go emits one per group. Named, not closed.
3. **The documentary defects, unchanged and still not fixed:** the non-existent public import path (**36 live occurrences across SEVEN files**; ⚠️ all 8 root `PROGRESS.md` hits are **pasted `go test` output**, so rewriting them is a different doctrinal act from correcting a statement; `DECISIONS.md:142` is an ADR that *decides* the wrong path and was never superseded) · a mechanical stat-surface count (8-11 tasks) · the unresolved half of the `BEHAVIOR_CONTRACT` stale-cite claim · `BEHAVIOR_CONTRACT.md:501`'s SN9 collision · **`ADR-0299`'s STATUS line still reads `PROPOSED`** although its §Decision and §Consequences landed · `BEHAVIOR_CONTRACT.md` carries **14** `### Does not yet apply to` headings, not 15.
4. ⚠️ **`ROADMAP.md`'s row 78 is the ONLY MALFORMED ROW of 111** — `NF=8` with 2 877 characters of IMPL summary in field 8 and **no trailing `|`**, so GFM drops the entire phase-78 IMPL summary from the rendered table. **NOT FIXED**: the §Schema invariant at `:18` forbids editing a closed row's `summary` cell. **Row 79's shape was verified canonical at this stage** — 7 pipes, `NF=8`, field 8 empty, pipe-terminated.
5. ⚠️ **`STATE.md` §Project counts is STALE ON FOUR AXES** (fixtures 119 vs **120**; stat surface 1205 vs **1207**; DECISIONS tail ADR-0298 vs **ADR-0300**; and `:33`'s *"next-free REFERENCE port is `10450`"*, refuted by `0118/driver/driver.go:26-35`'s own `refListenerPort = 10118` — right for the TLS band, wrong as a general claim; **next-free is `10119`**, `grep -rn '10119' test/` ⇒ 0, NC `10118` ⇒ 5). **Deliberately NOT repaired — repairing a count by editing the sentence that states it is how the ADR-0296/0297 species starts. Anchor on §Current, which IS live.**
6. **Normalizing the Operational-tooling short-form deferred-candidate paragraph** to the long form — a named, deliberately-untaken follow-up.
7. **The WASM row-summary rider stays DECLINED** (BRAINSTORM §11.1(E)) — `ROADMAP.md:76` declares phase 25 the FINAL §9 HTTP-filters-family row and rows 25.x use "family" 23 times to declare it CLOSED, so registering WASM is a doctrine adjudication against a landed closure statement, and writing the marker would silence check (3) **BY MENTION**. **Do not re-adopt it as cheap.**

---

## 12. ADR continuity — the **ADR-0301 §Context** DRAFT

Drafted at this SPEC per **`ADR-0044-as-used`** (⚠️ **ADR-0044 itself does not contain the Context-draft discipline**). §Decision + §Consequences append **IN PLACE** at the phase-79 IMPL after the retained footer — **no renumber, no `---` separator**. DECISIONS tail flips **ADR-0300 → ADR-0301 PROPOSED** at this commit; next-free becomes **ADR-0302**.

**Form, mirroring ADR-0300 (`DECISIONS.md:17532`), verified mechanically:** `## ADR-NNNN — <title>` with an **EM-DASH** (U+2014) and the bold parenthetical envelope tail; blank line; **one** `> **STATUS: …**` blockquote; blank line; `### Context (drafted at the phase-79 SPEC)`. **No `---`.** At this tip: tail **ADR-0300** at `:17532`, `^## ADR-0301` ⇒ **0**, heading forms old `NNNN:` **232** / new `NNNN —` **66** / total **299**.

⚠️ **ADR-0301 carries NO whole-file grep count.** That species self-falsified in ADR-0296 ¶3 and twice in ADR-0297. Every count is line-scoped or stated with no numeral.

⚠️ **The range-extraction hazard is REFUTED at this tip (R10)** — `^## ADR-` is a safe anchor. *(ADR-0209 has no heading by design; `## ADR-0127 v2:` at `:5973` is the sole heading matching neither form.)*

---

## 13. Exit — counts + expectations at SPEC-DONE

**Re-run MECHANICALLY by the controller in the stage worktree, each with a firing negative control; never copied.** Docs-only close.

| axis | value at this close | negative control observed | phase-79 IMPL delta (anticipated) |
|---|---|---|---|
| differential fixtures | **120** (tail `0118-runtime-static-layer`) | probe dir ⇒ **121** | **+0** |
| fuzzers | **55** | appended `func Fuzz` ⇒ **56** | **+0** |
| stat surface | **1207** | ⚠️ **NO mechanical command; DOCUMENTARY + CONFIG-CONDITIONAL** | **+0** — assert the DELTA |
| BackendKind | **tail 38** | ⚠️ a TAIL VALUE — 39 constants, `TCPEcho = 0`; do NOT "fix" to 39 | **+0** |
| go.mod modules | **2** (phase-61.2 lineage figure) | ⚠️ NOT a repo total — the single `go.mod` requires 67; do NOT "fix" 2 to 67 | **+0** |
| internal packages | **73** | — | **+0** |
| `runner_test.go` blank imports | **120** | naive `^\t_ ` ⇒ **126** | **+0** |
| ROADMAP | **227 lines / 111 data rows** | `want=109/110/112` ⇒ `GATE FAIL` | **+0 rows**; `want` **STAYS 111** |
| BEHAVIOR_CONTRACT | **5762** | — | grows by §7 |
| DECISIONS | **17596** | — | grows by ADR-0301 |
| DECISIONS tail | **ADR-0300 COMPLETE** → **ADR-0301 PROPOSED** at this commit | `^## ADR-0301` ⇒ **0** | completes at the IMPL |
| next-free ADR | **ADR-0302** after this commit | — | — |
| next-free fixture index | **0119** | numeric tail `0118` | unchanged |
| next-free reference port | **10119** | `10118` ⇒ **5** hits | unchanged |
| production `.go` files | **0 touched at this SPEC** | — | **2** at the IMPL |

**SPEC commit file set — FIVE files, and that is ONE MORE than the precedent, for a stated reason.** `DECISIONS.md` (ADR-0301 §Context) + `STATE.md` + this `SPEC.md` + `next-prompt.txt` + **`STATE_HISTORY.md`**. ⚠️ **The phase-76, -77 and -78 SPEC commits were each exactly FOUR files and none touched `STATE_HISTORY.md`** — verified by `git show --stat` on all three. This one is five because **§Recent lineage was AT its five-entry cap**, so ADR-0288 mandates evicting the oldest bullet rather than growing the list; the phase-77 BRAINSTORM bullet was moved **verbatim** (10 077 bytes, byte-for-byte) to `STATE_HISTORY.md` (`412 → 414`) and the phase-79 BRAINSTORM bullet took its place, leaving the list at exactly five. **A SPEC that reported "four files, matching the precedent" here would have been asserting a hygiene claim it had already falsified.**

**`ROADMAP.md` BYTE-UNTOUCHED; row 79 STAYS `in-progress`. `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED** — both verified with `git diff --stat master --`, EMPTY, and the same command over `STATE.md` returned a non-empty diff as its negative control. **`PROGRESS.md` is NOT created — it is born at the PLAN.**

---

## 14. Adversarial-pass record

**What refuted what.** Sixteen claims were re-derived (R1-R16). The five that would have caused wrong work if carried:

1. **§1.2** — the IMPL could have shipped the counter form and introduced a **deadlock in the admin scrape**, in the row immediately after the one that made silent hangs visible. The BRAINSTORM argued against the counter for two reasons, **neither of which was the real one**.
2. **§1.3** — the IMPL would have "fixed" the stale error string **to another wrong list**, under a commit message announcing the fix, and a future tenth arm would inherit it. §0.b's own indictment applied to §0.b's own remedy.
3. **R9** — deleting the `0118` pin would have left that surface with **zero** assertions on either side, silently, while reading as cleanup.
4. **R13/§7** — a `BEHAVIOR_CONTRACT.md` edit that is **mandatory on the +0 path** was absent from the edit-site table entirely; the row would have closed leaving a note that says *"the day `internal/stats` learns `runtime.`"* still pointing at a future that had already happened.
5. **§3.7** — a fixture-level sink audit would have been **vacuous for two of three arms** (0 of 10 stats-sink fixtures configure tracing or a gRPC/OTel access log) and would have read as coverage.

**⚠️ THE METHOD FINDING: THE STAGE'S DECISIVE RESULT CAME FROM THE ONE AGENT WHOSE INPUT NOBODY ELSE COULD SUPPLY.** D-SPP-1 was unanswerable in-repo — no amount of reading `internal/stats` could establish what reference Envoy emits. It took a live container. **Everything else this SPEC decided was downstream of that one measurement**: the arm count, whether the sink audit is an audit or a migration, and both of the band's ceiling contingencies.

**A CONTROLLER SELF-CORRECTION, recorded rather than quietly amended.** I read `ROADMAP.md:209`'s banked cost as **`10–14 tasks`** and was one keystroke from writing a drift correction against the BRAINSTORM's `11-14`. **`cut -c` is byte-based and the line is multibyte UTF-8**, so my slice truncated; re-reading with character offsets showed the line carries **SIX** task bands and I had matched phase 63's. **The BRAINSTORM's cite was right and my correction would have been the drift** (`reference_a_drift_correction_is_itself_a_claim`). The trap is now recorded in §8.

**A SECOND CONTROLLER SELF-CORRECTION.** From the same truncated slice I concluded line 209 *"ends mid-word"* and nearly recorded a corrupted-ROADMAP finding. It does not; it ends `…a boot panic is a SILENT HANG.` **An artifact of my own measurement instrument read as a defect in the measured object.**

**A THIRD CONTROLLER SELF-CORRECTION — and this one was caught by a gate rather than by re-reading.** Both §13 and the router roll were drafted saying the SPEC file set is *"FOUR files, exactly the phase-76/77/78 precedent."* The actual commit carries **FIVE**: the §Recent lineage was at its five-entry cap, so ADR-0288 forced an eviction to `STATE_HISTORY.md`, and the sentence claiming precedent-conformance was written **in the same document as the edit that broke it**. Caught only because the close-out ran `git status --porcelain` and compared the result against the claim instead of trusting it, then checked all three prior SPEC commits with `git show --stat`. ⚠️ **`reference_document_hygiene_claim_not_evidence` — a document's statement about its own file set is not evidence about its file set.**

**AGREEMENT BOUGHT NOTHING, AND WAS NOT RELIED ON.** The four agents held disjoint remits, so their concurrence is not cross-validation (`reference_independent_probes_can_share_a_blind_axis`). Where two touched the same object it is recorded as such: the `wasm.`-arm count was independently corroborated by an in-tree `BEHAVIOR_CONTRACT.md:5020` enumeration **and both were superseded** by §1.3's nine. Conversely, the band was derived by one agent at **11-13** *without knowledge of* the D-SPP-1 result that discharges one of its tasks — **so this SPEC adopts neither its number nor the BRAINSTORM's**, and says why in §9.

**A REFUTED HYPOTHESIS THIS SPEC'S OWN DISPATCH BRIEF INTRODUCED.** The brief told an agent that the reference *"may hoist the sink-type segment as a LABEL"* and framed that as the likely outcome. It does not, for any of the three roots. **A brief's hypothesis is not evidence either** — the agent was instructed to measure rather than confirm, and did.

⚠️ **The Bash cwd reset hazard was assumed live and handled** — every git command in this session used `git -C <abs-worktree-path>`; five worktrees (one controller, four agent) each stayed on their own HEAD.

⚠️ **The machine-global-namespace hazard held.** Four agents ran concurrently with private scratch directories and private detached worktrees; the docker-using agent named every container `p79a1-*` and tore down **BY NAME**, and its final `docker ps -a --filter name=p79a1-` was **EMPTY**. No sibling was disturbed. All four worktrees closed with `git status --porcelain` empty.
