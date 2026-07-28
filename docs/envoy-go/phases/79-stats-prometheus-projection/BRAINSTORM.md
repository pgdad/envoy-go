# Phase 79 Brainstorm — `/stats/prometheus` PROJECTION completeness: thirty live registered stat names are SILENTLY DROPPED from the Prometheus endpoint by an allow-list `switch` plus a bare `return`, and the inherited claim that fixing it moves four OTHER sinks' wire output is **REFUTED BY EXECUTION** (the TWENTY-SECOND §9 Observability-family row — **+0 stats (1207 → 1207), +0 differential fixtures (120), +0 fuzzers (55), +0 BackendKinds (38), +0 go.mod modules, +0 packages, +0 new PUBLIC surface**; the shipped fix is three byte-mirror `switch` arms plus an observability leg)

**Stage:** BRAINSTORM (lifecycle-state `4` → 1). **Row 79 registered `in-progress` at this commit** per the ROADMAP §Schema invariant, RE-OPENING sentinel check (1) after its fifth-ever silent close.

**Self-picked** per the 2026-07-12 standing directive: smallest defensible candidate first. The pick and every rejected alternative are recorded in §2.1 and §11.1 with the evidence that settled each. Docs-only: **ZERO production `.go`, ZERO test `.go`.**

---

## 0. ⚠️ READ THIS FIRST — THREE INHERITED CLAIMS DIED AT THIS STAGE, AND ONE OF THEM WAS THE ROW'S HEADLINE RISK

The router handed this stage a self-pick that had already been "costed mechanically at the phase-78 close." Re-derivation at **this** tip (`1b1d07d4`) reproduced the core measurement exactly and **refuted three load-bearing claims built on top of it**. Each was killed by execution, none by review.

**(0.a) ⚠️ THE BLAST-RADIUS CLAIM IS FALSE FOR THE SHAPE THIS ROW SHIPS.** The banked text says four further non-test consumers of `ExtractTags` "all currently FALL BACK TO THE FULL DOTTED NAME on error, so adding an arm **CHANGES THEIR WIRE OUTPUT**." The premise is true; **the conclusion is false for a byte-mirror arm.** Measured on a scratch copy — all four sinks' wire output captured for `runtime.*` before and after adding a byte-mirror `runtime.` arm:

```
                     BASELINE (no arm)                    WITH byte-mirror arm
EXTRACT    residual=""  err=true                 →   residual="runtime.num_keys" err=false   CHANGED
LABELMAP   name="runtime.num_keys"               →   name="runtime.num_keys"                 IDENTICAL
DOGSTATSD  "dsdpfx.runtime.num_keys:6|g"         →   "dsdpfx.runtime.num_keys:6|g"           IDENTICAL
GRAPHITE   "grpfx.runtime.num_keys:6|g"          →   "grpfx.runtime.num_keys:6|g"            IDENTICAL
OTLP       name="runtime.num_keys" attrs=[]      →   name="runtime.num_keys" attrs=[]        IDENTICAL
PROM       present=false totalBytes=0            →   present=true  totalBytes=237            CHANGED
```

The mechanism: every sink's fallback is `residual = fullname, labels = nil`, and a byte-mirror arm produces *exactly that* on the success path. `internal/statssink/label.go:39` guards on `if err != nil || len(labels) == 0` — the **`len(labels)==0` disjunct** catches the success case; `dogstatsd.go:83-87` and `graphite.go:68-71` set `residual = fam.GetName()` and `formatTagSuffix(nil)`/`graphiteTagSuffix(nil)` both return `""`; `otlp.go:195-207` sets `base = residual` (== the full name) and `kvFromTags(nil)` returns `nil` (`otlp.go:268-270`). **The blast radius is prometheus-only PROVIDED the arms are byte-mirror.** It is larger only for a **tag-hoisting** arm — which is exactly the `sds.` work this row DEFERS (§8).

⚠️ **This is the difference between a verification task and a change task, and it is why the sink-side leg is scoped as an AUDIT, not a migration.**

**(0.b) ⚠️ "FOUR TOP-LEVEL PREFIXES" IS WRONG, AND THE CODE'S OWN ERROR STRING IS WHERE THE WRONG NUMBER COMES FROM.** The banked correction (a) says *"'SIX families' is ambiguous and, read literally, WRONG: there are FOUR top-level prefixes."* Executed — one probe per candidate root through production `ExtractTags`:

```
LIVE TOP-LEVEL PREFIXES (5): [cluster. http. listener. server. wasm.]
ARM-ABSENT: runtime. access_logs. tracing. sds. listener_manager. filesystem. main_thread.
```

**The switch has FIVE live arms, not four** (`name.go:51, :61, :83, :93, :97`) — and the banked sentence **contradicts its own opening clause**, which correctly enumerates `cluster.|http.|listener.|server.|wasm.`. "FOUR" is true only under the unstated reading *"four DROPPED prefixes."*

⚠️ **And the origin of the error is a live defect this row should fix.** `internal/stats/name.go:350` — the terminal error every one of the thirty drops emits — reads verbatim:

```go
return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment (want cluster.|http.|listener.|server.)", internal)
```

**It omits `wasm.`.** It has been stale since the phase-25.1 `wasm.` arm landed. Anyone — human or agent — who derives the arm count from the error string gets **4**. That is almost certainly how "FOUR" entered the lineage. **A row that adds three arms to this switch and does not fix this string ships a fourth generation of the same wrong number.**

**(0.c) ⚠️ THE WASM RIDER'S FRAMING IS REFUTED, AND THE RIDER IS DECLINED.** The router proposed folding a WASM row-summary slug into this row as one task, on the premise that *"the work is BUILT; what is missing is a row-summary string containing the slug."* Every supporting figure re-derived TRUE (`internal/wasm/` **21 550** lines, rows 25/25.1/25.2/25.3 all `done`, fixtures `0034`-`0039`, a live `wasm.` arm at `name.go:97`, proxy-wasm conformance **executed 10/10 at this tip**). **The characterization is false.** `ROADMAP.md:76` states verbatim: *"phase 25 is the **FINAL §9 HTTP-filters-family row**"*, and rows 25.x carry 23 uses of "family" including *"the §9 HTTP-filters family is CLOSED"*. **The WASM work was landed INSIDE the HTTP-filters family and was used to CLOSE it.** Registering WASM as its own family is therefore not a blank to fill — it is a doctrine adjudication against a landed, load-bearing closure statement. **DECLINED. See §11.1(E).**

---

## 1. Mission and scope confirmation

### 1.1 What phase 79 delivers as a self-contained whole

Thirty live, registered, incrementing stat names are absent from `/stats/prometheus` with **no log, no counter, no error, and no partial line**. Phase 79 makes **ten** of them project, and makes the silent-skip mechanism **observable** so the remaining twenty cannot hide.

1. **Three byte-mirror `switch` arms in `internal/stats.ExtractTags`** — `runtime.` (2 names), `access_logs.` (4), `tracing.` (4). Each mirrors the shape of the existing SN5 `server.` arm: `residual == input`, zero labels.
2. **The stale terminal error string at `name.go:350`** corrected to enumerate the arms that actually exist (§0.b).
3. **`WriteProm`'s silent skip made OBSERVABLE** — `internal/stats/prom.go:38-41` currently does a bare `return` under a comment claiming the behavior is "log+ignore". It neither logs nor counts. §2.3 sets the BRAINSTORM's hypothesis (a log, not a counter) and §7 dockets the choice.
4. **`helpText` entries** for the ten newly-projecting names, plus the prose entry-count sentence that goes stale with them.
5. **`internal/stats/name_test.go` guards** for the three new arms — these do **not** exist by default (§2.3).
6. **`test/fixtures/0118-runtime-static-layer`'s DEPARTURE PIN flipped to a parity assertion** — the row's closing gate (§2.4).
7. **A sink-side regression AUDIT** proving §0.a's no-op result holds at the shipped tip for all four non-prometheus consumers.

### 1.2 What phase 79 does NOT deliver (forward to §8)

The `sds.` **label-hoisting** arm (20 of the 30 names) and the **registration-time-validation endgame**. Both go to a banked row **79.1**.

### 1.3 Phase-done as an Observability-family row

Row 79 is the **TWENTY-SECOND §9 Observability-family row** (row 75 holds TWENTY-FIRST, verified mechanically at `ROADMAP.md:137`). Flat top-level row, **SOLE leg** per ADR-0106 — which governs the sole-leg property and does **not** define the phase-done gate. ⚠️ **The gate is `BOOTSTRAP_PROMPT.md` §7.5, gates (a)-(f) at `:360-365`** (the heading is `:357`, the closing sentence `:367`; the inherited cite `:357-366` is off by two). ADR-0106 contains **ZERO** occurrences of "six" — verified with two negative controls (the same matcher over the whole `DECISIONS.md` ⇒ 242; `ADR-0106` inside the extracted block ⇒ 1, guarding against an empty extraction reading as a zero result).

### 1.4 ADR-0045 split readiness

ADR-0045's gate trips at **~25 tasks OR ~1500 LoC**. This row is banded at **9-11 tasks** (§10) against a full scope costed at 13-15, and the split is **taken deliberately at the BRAINSTORM** rather than at the gate: `sds.` + registration-time validation go to 79.1. Precedent for a deliberate pre-gate split: 28.1a/b, 46.1b, 56.1/56.2.

### 1.5 Package placement

No new package. Edits land in the existing `internal/stats` (production), `test/fixtures/0118-runtime-static-layer/driver` (fixture), and the existing `internal/stats/name_test.go`.

---

## 2. Design decisions

### 2.1 Row + subject confirmation *(SELF-PICKED per the 2026-07-12 standing directive)*

**The defect is REPRODUCED BY EXECUTION at this tip**, driving **production** `ExtractTags` / `flattenToProm` / `WriteProm` through a temporary in-package test (created, run, deleted; the probe worktree finished with `git status --porcelain` EMPTY).

| set | n | projected | dropped |
|---|---|---|---|
| candidates (harvested from real registration sites) | 30 | **0** | **30** |
| controls (one per live top-level arm + one per second-pass detector) | 16 | **16** | **0** |
| near-miss controls | 4 | **4** | **0** |

⚠️ **THE PROBE DISCRIMINATES** — it is not the case that everything drops. The near-misses sharpen the finding to the ROOT SEGMENT alone: the HCM-scoped sibling `http.<sp>.tracing.spans_sent` **PROJECTS** while the tracer-scoped `tracing.opentelemetry.spans_sent` **DROPS**; `server.accesslog_dropped` **PROJECTS** while `access_logs.grpc_access_log.logs_dropped` **DROPS**.

⚠️ **INPUTS WERE DERIVED FROM OBSERVED DATA, NOT INVENTED** (`reference_probe_input_is_a_claim`). Every candidate name comes from a real registration site; every control from a landed golden table (`name_test.go` / `prom_test.go`) or a real prefix builder.

**The drop roster — 30/30, and the banked 4+4+20+2 composition REPRODUCES EXACTLY:**

| family | n | origin |
|---|---|---|
| `access_logs.{grpc,open_telemetry}_access_log.logs_{written,dropped}` | 4 | `internal/accesslog/stats.go:24,25,34,35` |
| `tracing.{opentelemetry,zipkin}.spans_{sent,dropped}` | 4 | `internal/tracing/stats.go:53,54,70,71` |
| `sds.<secret>.{update_success,update_failure,update_rejected,update_attempt,init_fetch_timeout}` | 20 | `internal/xds/stats.go:28` (templates `:34-38`) |
| `runtime.num_{keys,layers}` | 2 | `internal/bootstrap/bootstrap.go` |

`28 = 30 − 2` (runtime is phase 77's own) holds. **Every one of the thirty failed with the SAME error** — none partially matched any arm; there is no near-miss inside the roster.

**The silent-skip mechanism, CONFIRMED BY EXECUTION with actual bytes.** A registry holding 5 metrics (1 projecting, 4 dropping); `WriteProm` returned `nil`:

```
===== WriteProm OUTPUT (116 bytes, registry has 5 metrics) =====
# HELP envoy_server_live 1 if the server is live, 0 otherwise.
# TYPE envoy_server_live counter
envoy_server_live 1
===== END =====
registered metrics = 5 ; emitted metric lines = 1 ; silently dropped = 4
```

Asserted ABSENT and all passing: `spans_sent`, `logs_written`, `update_success`, `num_keys`, `tracing`, `access_logs`, `sds`, `runtime`. **Not one byte of any dropped metric appears, and `WriteProm` returns `nil`.**

#### Why this is the right pick, in order

1. **Its prerequisite is DISCHARGED, and the ordering was forced by evidence rather than preference.** `ROADMAP.md:140` — which *is* row 78 — states verbatim: *"THE ORDERING IS FORCED BY EVIDENCE, NOT PREFERENCE: this row is a PREREQUISITE for the banked `/stats/prometheus` projection row."* Row 78 is `done`. The endgame (79.1) is registration-time validation, i.e. a **boot panic** — precisely the failure mode that was an undiagnosable zero-byte hang until phase 78 landed.
2. **The defect is already reproduced by an EXECUTING assertion in the shipped tree**, so the "reproduce before writing the BRAINSTORM" bar was met on day zero and the closing gate is pre-built (§2.4).
3. **It is the smallest candidate that changes observable behavior.** The strongest runner-up (§11.1(A)) is documentary and prospective — `go build ./...` exits **0** under the real module path today.
4. **The blast radius shrank under measurement** (§0.a), which is the rare direction for this project's costings to move.

### 2.2 Scope *(the SPEC pins each — D-SPP-* in §7)*

**IN:** three byte-mirror arms · the `:350` error string · `helpText` + its prose count · `name_test.go` arm guards · `WriteProm` observability · the `0118` pin flip · the sink-side audit.

**OUT (to 79.1):** `sds.` label-hoisting · registration-time validation.

⚠️ **THE `sds.` DEFERRAL IS NOT ARBITRARY — IT IS THE ONE ARM THAT BREAKS §0.a's NO-OP RESULT.** A label-hoisting arm returns non-empty labels, which means `label.go:39`'s `len(labels)==0` disjunct no longer fires and the metrics_service / dogstatsd / graphite / otlp wire output **genuinely moves**. Shipping it alongside byte-mirror arms would fuse a verification task and a migration task into one row and destroy the audit's control.

### 2.3 Verification design

**⚠️ THE DEFAULT STATE IS UNGUARDED, AND THAT IS MEASURED, NOT ASSUMED.** All three arms were added on a scratch copy and the real gates run:

```
go build ./...                   → clean
go test ./internal/... -count=1  → ALL GREEN, zero failures
go vet ./test/...                → clean (every fixture driver compiles)
```

**ZERO test or fixture edits were required.** Nothing in the tree guards against a new arm: the only negative assertions use `"unknown_top_segment.foo"` (`name_test.go:141`) and `"listener_manager.*"` (`:956`), neither of which collides. ⇒ **the guards in §1.1(5) are work this row must CREATE, not tests it must keep passing.**

⚠️ **AND THE `helpText` "COMPLETENESS ASSERTION" DOES NOT EXIST.** The inherited claim is *"a `helpText` completeness assertion at `:209` over a 15-entry map."* The map **is** 15 entries (`name.go:458-476`) and `:209` **is** a real line — but it sits inside `TestHelpText_Coverage` (`:195`) iterating a **hand-listed 10-name slice** (`:196-206`). **There is NO reverse direction.** The file says so itself at `:230-236`: *"nothing walks helpText demanding every envoy_listener_ssl_* key appear here … leaves it SILENTLY UNGUARDED (EXECUTED at the phase-75 PLAN)."* ⇒ **new `helpText` entries are unguarded by construction; a HELP line silently degrades to the bare metric name.**

⚠️ **THE OBSERVABILITY LEG NEEDS A NEGATIVE ASSERTION, NOT A POSITIVE ONE** (`reference_positive_arm_cannot_catch_overfiring`). A test that asserts "the skip signal fired for a dropped name" passes on a build that fires it for *every* name. The design must be a stacked control: one dropped name and one projecting name in the same registry, asserting the signal fires **exactly once**.

⚠️ **AND THE OBSERVABILITY LEG CARRIES A SELF-REFERENTIAL TRAP.** If the skip is made observable via a *counter*, that counter's own name must satisfy `ExtractTags` or it is silently skipped by the very code it instruments. That is the argument for the **log** form, which is also what `prom.go:18-22` already claims the code does (see §2.5).

### 2.4 Fixture posture — **+0 fixtures; the closing gate is ALREADY BUILT and is currently GREEN-BY-DEPARTURE**

`test/fixtures/0118-runtime-static-layer/driver/driver.go` carries a departure pin at `:272-276`, verbatim:

> *"⚠️ THIS IS A DEPARTURE PIN, NOT A PARITY CLAIM. The reference emits both gauges to /stats/prometheus; envoy-go emits NEITHER, because `internal/stats.ExtractTags` does not recognize a `runtime.` top-level segment and `internal/stats.WriteProm` silently skips any metric whose name fails to flatten."*

⚠️ **It is not prose — it is an EXECUTING ABSENCE ASSERTION** in `assertPrometheusExpositionDeparture` (`:290`), called unconditionally at `:266`, failing at `:306-314` if the gauges appear:

```go
for _, name := range []string{promNumKeys, promNumLayers} {
    if v, ok := subjProm[name]; ok {
        t.Errorf("subj: %s is NOW PRESENT on /stats/prometheus (= %d) — the phase-77 prometheus-exposition "+
            "gap has been CLOSED. That is good news and this row is deliberately RED to force the follow-up: ...
```

⇒ **`0118` goes RED the moment the `runtime.` arm lands, BY DESIGN.** That is the row's cleanest closing gate: a pre-existing, executing, failing-on-success assertion authored by a prior phase specifically to force this one. Corroborated in-tree at `internal/admin/stats.go:12-22`, which documents the gap as the reason the flat `/stats` endpoint exists at all.

⚠️ **A LIVENESS CAVEAT** (`reference_liveness_break_needs_failing_baseline`): the SPEC must confirm `0118` is RED *before* the pin is edited. A green `0118` after the arm lands is ambiguous between "parity achieved and pin correctly flipped" and "the assertion did not run."

### 2.5 Stat surface hypothesis — **+0 (1207 → 1207), CONDITIONAL, and the condition is stated**

Adding a `switch` arm is a **PROJECTION** change, not a **REGISTRATION** change: the thirty names are *already registered and already incrementing*; they simply never reach the endpoint. No new `NewCounter`/`NewGauge` call site is implied by items 1-2 and 4-7 of §1.1.

⚠️ **ITEM 3 IS THE CONDITION.** If `WriteProm`'s skip is made observable with a **counter**, the delta becomes **+1** and the ledger must move. **The BRAINSTORM's hypothesis is the LOG form, holding +0** — and the argument is documentary rather than merely convenient: `prom.go:18-22` already claims the behavior is "log+ignore" per a prior BRAINSTORM §5.3, and the code implements **neither** the log nor the ignore-with-signal. Adding the log **restores documented intent at +0 stats**; adding a counter is new public surface plus the self-referential trap of §2.3. **Docketed as D-SPP-3; the SPEC decides and must move the ledger if it chooses the counter.**

⚠️ **THE `+0` CLAIM CANNOT BE DISCHARGED BY THE `TestNoNewStat*` GUARDS.** All five live in `internal/statssink/registration_test.go` (`:26, :53, :81, :109, :137`), header `package statssink`, asserting `countMetrics(reg) == 0` over that package's own sinks via a Registry walk. **None reaches `internal/stats`.** ⚠️ **And the blindness cuts both ways** — they would also fail to catch a regression here. The available argument is **STRUCTURAL**: enumerate the diff's registration call sites and show the set is empty. **State it as structural.**

---

## 3. Framework-survey result

No framework gap. `ExtractTags` is a pure function over a string; the three arms are additive `case` clauses in an existing `switch`, and every consumer already handles both branches (§0.a). No new seam, no interface change, no plumbing.

---

## 4. Bootstrap-level applicability — NONE at this row

Nothing in `internal/bootstrap` or `internal/boot` changes. ⚠️ **The bootstrap-level work is precisely what 79.1 carries** (registration-time validation is a boot-path panic), which is why the split in §1.4 falls where it does.

---

## 5. Stat surface hypothesis — **+0** (1207 → 1207)

Restated per the §1-12 convention; the derivation and its stated condition are in §2.5. ⚠️ **1207 is a DOCUMENTARY figure carried by the lineage, not a mechanically re-derived one — assert the DELTA, never the absolute.** ⚠️ **And the ledger it lives in carries TWO PARALLEL TOTALS (H2 and non-H2 — `BEHAVIOR_CONTRACT.md:795`, root cause `internal/cluster/manager.go:194-202`, 4 stats behind `if c.useH2`), so the absolute is config-conditional.**

---

## 6. Anticipated edit sites *(the SPEC RE-DERIVES each at ITS OWN tip — a BRAINSTORM cite is not evidence)*

| file | what | required? |
|---|---|---|
| `internal/stats/name.go` — `switch` (~`:100-110`) + SN-rule doc block (`:24-45`) | the three arms | **Yes — the only mechanically required edit** |
| `internal/stats/name.go:350` | the stale error string (§0.b) | **Yes** |
| `internal/stats/name.go` — `helpText` (`:458-476`) **and its prose "Of the 15 entries" at `:448`** | 10 new entries + the count sentence | Quality; the prose count goes stale silently |
| `internal/stats/name_test.go` | new arm guards + helpText coverage | Quality; **absent by default** (§2.3) |
| `internal/stats/prom.go:38-41` | the observability leg | Yes |
| `test/fixtures/0118-runtime-static-layer/driver/driver.go` `:142-166`, `:266`, `:269-315` | pin → parity | **`runtime.` only** |

**Resulting names (10):** `envoy_runtime_num_keys`, `envoy_runtime_num_layers`, `envoy_access_logs_{grpc,open_telemetry}_access_log_logs_{written,dropped}` (4), `envoy_tracing_{opentelemetry,zipkin}_spans_{sent,dropped}` (4).

---

## 7. BRAINSTORM-time open questions to the SPEC — the D-SPP-* docket

- **D-SPP-1 — ⚠️ IS BYTE-MIRROR THE RIGHT SHAPE FOR `access_logs.` AND `tracing.`? THIS IS THE ROW'S LARGEST UNMEASURED RISK.** Reference parity is measured for `runtime.` **only**: `0118:145-152` recorded the reference emitting `envoy_runtime_num_keys{} 6`, so byte-mirror matches there. **No in-repo measurement exists for the other two.** The reference may hoist the sink-type segment (`grpc_access_log`, `zipkin`) as a **label** rather than inlining it — which would make byte-mirror the wrong shape and drag both arms into the tag-hoisting category, destroying §0.a's no-op result for them. **The SPEC MUST pin this against a live dockerized reference — fresh container per arm** (`feedback_probe_fresh_container_per_arm`), **on a bridge network** (`reference_docker_probe_bridge_network`), **torn down BY NAME** (`reference_parallel_agents_shared_machine_namespaces`). If either arm turns out to hoist, that arm moves to 79.1 and this row ships two arms, not three.
- **D-SPP-2 — the `sds.` static/dynamic split, so the number does not drift.** Statically there are **5** name templates (`internal/xds/stats.go:34-38`). The banked **20** is `5 × 4` where 4 is the distinct secret count. ⚠️ **The banked rule is under-specified and a naive re-derivation gets the right total for the WRONG REASON:** grepping `test/fixtures/**` for SDS secret names returns **SIX** distinct values — the four real ones (`server_cert`, `validation_ca`, `rccf_validation_ca`, `edf_validation_ca`) **plus `client_secret` and `hmac`** from `0024-http-oauth2`. The oauth2 pair does not contribute because those use `path_config_source` and are served by `internal/sdsfile.Watcher`, never by the xds gRPC provider; `RegisterSDSStats` has exactly **one** non-test caller, `internal/boot/boot.go:201`. **The rule is "distinct secrets reaching `boot.go:201`", not "distinct secrets in the corpus"** — and anyone using the corpus rule gets `6 × 5 = 30` and mistakes it for confirmation of the 30-name total. Applies to 79.1; recorded here so it cannot be lost.
- **D-SPP-3 — log or counter for the `WriteProm` skip?** §2.5 hypothesizes the log at +0. If the SPEC chooses a counter: the stat-surface delta becomes +1, the ledger must move, and the counter's own name must live under an arm that exists (`server.` per SN5) or it is skipped by the code it instruments.
- **D-SPP-4 — does the `:350` error-string fix belong in this row?** BRAINSTORM position: **yes** (§0.b). It is a two-token edit in a function this row is already editing, and leaving it ships a fourth generation of the wrong arm count. The SPEC should confirm no test asserts the current string byte-for-byte.
- **D-SPP-5 — flip or delete the `0118` pin?** The pin block spans `:142-166` and `:269-315`; the assertion is an absence check. The SPEC must decide between converting it to a presence/parity assertion and deleting it in favour of the generic prometheus comparison, and must run the liveness caveat in §2.4 first.

---

## 8. What phase 79 does NOT deliver (forward)

1. **The `sds.` label-hoisting arm — 20 of the 30 names.** Needs `envoy_xds_resource_name`, **measured ONCE**; the ROADMAP's own text says the SPEC must re-pin it against a live dockerized reference. ⚠️ **It is the arm that BREAKS the four-sink no-op result** (§2.2).
2. **Registration-time validation** — the "impossible by construction" endgame. It panics at boot. ⚠️ **Per ADR-0300 §Consequences (ii), any injected-defer probing there MUST VARY THE INSERTION POINT across the pre- and post-anchor windows** — a single pre-anchor injection reports "caught" and conceals the genuinely uncovered case.
3. Both go to a banked row **79.1**, costed at **~7-9**.

---

## 9. ADR-0045 split readiness + ADR roster

**ADR-0301** (next-free confirmed: `grep -c '^## ADR-0301' docs/envoy-go/DECISIONS.md` ⇒ **0**; negative control `^## ADR-0300` ⇒ 1). New-form heading (`## ADR-NNNN — <title>`, em-dash) with the single `> **STATUS: …**` blockquote, **no `---` separator** — mirroring ADR-0300, the most recent genuinely-COMPLETE block. §Context drafted at the SPEC; §Decision + §Consequences appended IN PLACE at the IMPL (**ADR-0044-as-used**; ⚠️ ADR-0044 does not itself contain that discipline).

⚠️ **Two ADR formats coexist** — old blocks (≤~0250) use `## ADR-NNNN: <title>` with plain `**Status:**` lines. **Phase 79 uses the NEW form.**

⚠️ **A range-extraction hazard for any SPEC/IMPL agent reading `DECISIONS.md`:** `^## ADR-0107` matches **nothing** — ADR-0106's block ends where ` ## ADR-0108` begins at `:4858`, **with a leading space**. An awk/sed range gate assuming contiguous space-free `^## ADR-` headings silently mis-bounds.

---

## 10. Envelope + counts (anticipated at the phase-79 IMPL; docs-only at this BRAINSTORM)

**+0 stats (1207 → 1207, conditional per §2.5)** · **+0 differential fixtures (120)** · **+0 fuzzers (55)** · **+0 BackendKinds (38 — a TAIL VALUE; 39 constants, 0-38)** · **+0 go.mod modules** · **+0 packages** · **+0 new PUBLIC surface** (`ExtractTags`'s signature is unchanged; the arms are internal `case` clauses).

**SPEC band: 9-11 tasks. Budget 11.**

⚠️ **THE BAND IS A DELIBERATE RE-SCOPE, AND BOTH FIGURES IT SITS BETWEEN ARE STATED.** The banked candidate at `ROADMAP.md:208` says **11-14 tasks** for the FULL scope. The §2.3 measurement — three arms added, all gates green, **zero** test or fixture edits forced — argues for **7-9**. **9-11 is chosen over 7-9 for two stated reasons**, not split-the-difference: (i) the §2.3 measurement excludes **D-SPP-1's dockerized reference probing**, which is real, serial, container-per-arm work that no in-repo measurement covers; and (ii) the cost calibration is **three-for-three at or above the SPEC ceiling** — 76: 7-9 → shipped 9; 77: 11-13 → 12; 78: 7-9 → PLAN re-derived 10 → shipped 10. ⚠️ **Phase 78's PLAN refuted its SPEC's band in BOTH directions, so the SPEC must re-derive rather than inherit** (`reference_deferred_candidate_cost_restale`).

**Risk to budget for:** the full **120**-fixture differential is **mandatory** (`internal/stats` links into `cmd/envoy-go`) at **~400-420 s** per green attempt, with `-race` a **second** run, not a substitute; ⚠️ **and there is a THIRD build site of this binary** — `test/conformance/h2spec/h2spec_test.go:210` (`TestH2Spec`, entry `:30`), alongside `test/differential/harness.go:240` and `:594`. Run with `-count=1` **AND `-v`** (⚠️ without `-v` a green log is indistinguishable from a suite that ran nothing), capture the **INNER** exit status, scope the tally to `TestDifferential/`, and cross-check with `comm -3` against the fixture-dir set — **the `comm -3` cross-check is the load-bearing gate, not the raw count.**

---

## 11. Sized-against-source — the derivations (FOUR agents on disjoint remits at tip `1b1d07d4`, PRIVATE scratch and a PRIVATE probe worktree, plus controller re-derivation)

### 11.1 The rejected alternatives, with the evidence that settled each

**(A) `esalaine/envoy-go` import-path sweep — STRONG RUNNER-UP, genuinely the smallest surface. REJECTED on *defensibility*, not size.** Re-measured at this tip: **36 live occurrences across SEVEN files** (live = everything except the frozen archives under `docs/envoy-go/phases/**`), per-file **11 / 8 / 7 / 4 / 3 / 2 / 1** — every banked figure exact, including `BEHAVIOR_CONTRACT.md` at `:862` and `:5012`, the repo-wide bare count **5392**, and the archive share **5356 (99.33 %)**. **ZERO in compiled Go** (`.go` non-test 0, `_test.go` 0, `go.mod`/`go.sum` 0, testdata 0) and **`go build ./...` exits 0 with a zero-byte log** — exit status captured explicitly (`reference_harness_exit_code_is_not_command_exit_code`). **The tree builds green under the real path (`github.com/pgdad/envoy-go`); the defect is documentary and prospective.** It cannot reach a clean end state: `DECISIONS.md:142` is `## ADR-0006: module path \`github.com/esalaine/envoy-go\`` — an ADR that DECIDES the wrong path, **never superseded** — and is immutable under the never-edit doctrine; ADR-0301 must *name* the wrong path to supersede it; `ROADMAP.md` is append-only yet holds 4 of the 36; and any recurrence guard asserting `count==0` is **unsatisfiable by construction** (`reference_sentinel_matcher_string_self_clears`). ⚠️ **BANKED FIGURE WRONG: "18 `ADR-0006` hits" — at this tip it is 24 matching lines / 28 occurrences across 15 files.** The conclusion (none is a supersession) is unaffected. ⚠️ **AND A NEW OBSERVATION THAT CHANGES WHAT THE EDIT MEANS:** all 8 root `PROGRESS.md` occurrences (`:85-91`, `:104`) are **pasted `go test` output lines** (`ok  github.com/esalaine/envoy-go/internal/admin (cached)`) — a recorded execution log under a module path that does not exist, not prose asserting a path. Rewriting recorded output is a different doctrinal act from correcting a statement. **SPEC 8-10, expect 10-11. Pick this instead if a docs-only, zero-production-Go phase is wanted.**

**(B) Mechanical stat-surface count — REJECTED, and the rejection is UNDERSTATED rather than overstated.** Every documented obstacle reproduces, and the arithmetic drifts *against* the mechanical approach. ⚠️ **Two banked figures are WRONG: `214` registration sites → **210** (`NewCounter` 113 + `NewGauge` 30 + `NewCounterIfAbsent` 62 + `NewGaugeIfAbsent` 5; no `NewHistogram`/`NewTextReadout` exist), and "the 17-row ledger" → **27 rows** (`BEHAVIOR_CONTRACT.md:4962`, rows `:4966`-`:5018`, plus a 28th detached at `:805`).** Also: **11** sites pass a string literal (not 12), yielding **10 unique names** — a ~0.8 % sample of 1207, which *is* the measurement proving grep cannot do this. **5** `statroster.New` fan-out sites confirmed exactly (zookeeper alone **201** names). **2** method-value registrations confirmed invisible to any regex — `reg.NewCounter,` has no `(`, so `\.NewCounter\(` cannot match. **The fourth obstacle CONFIRMED: the ledger carries TWO PARALLEL TOTALS** (`:795` — *"UNCHANGED at 1200 (non-H2 1196…)"*, root cause `internal/cluster/manager.go:194-202`). **The +3 unattributed gap CONFIRMED EXACTLY**: `:5008` phase 46.1b ends 1198; `:5010` phase 47.1 opens 1200 (**+2, no row**); `:5012` phase 51 ends 1200; `:5014` phase 74 opens 1201 (**+1, no row**). **8-11 tasks, pure meta-work, no behavioral payoff. Revisit after 79 closes.**

**(C) Symmetric bind hardening — REJECTED for 79, and the banked scope is materially WRONG in the row's favour.** Anchors verified exact: `runner_test.go:237` is `context.WithTimeout(..., 90*time.Second)` inside `runFixture`; `freeTCPPortBlock` at `harness_test.go:235` (`subjectPortBlockSpan = 16`, bases `20000..31007`); `startSubjectWithRetry` at `runner_test.go:219`, 3 attempts, **sole caller** of `freeTCPPortBlock`, three call sites `:1214/:1377/:2066` — **subject-proxy-only scope confirmed**; and the NOTE at `:2215-2217` verbatim: *"a bind-collision retry … must NEVER be added here — this path asserts that the start FAILS, and a retry loop would mask the expected boot rejection."* ⚠️ **BANKED FIGURE WRONG: `mustAllocatePort()` in "42 packages" → actually 10** (drivers `0089, 0090, 0091, 0103, 0108-0113`); the racy helper is copy-pasted under **four** names totalling ~15 definitions. ⚠️ **BANKED CLAIM OVERBROAD: "fixture BACKENDS still use a bare `freeTCPPort` with NO retry" is only partly true** — `fixture.TCPEcho`/`HTTPEcho` (the default for nearly every fixture) `net.Listen` and **hold the listener open** for the test's lifetime (`runner_test.go:274-283`), so they cannot close-then-rebind race at all. **Only `fixture.HTTPSH2` (`:288-293`) does `freeTCPPort` → `startHTTPSH2Backend` with `t.Fatalf` and no retry.** The 70/44/43 triple measures 68/46/44 (heuristic-dependent). **Still rejected: it CANNOT be verified by a green suite run** — `0e9cc680` needed `-count=6`. Its right home is a dedicated harness-hardening phase with a `-count=N` budget.

**(D) Open the gRPC family — REJECTED, HARD-BLOCKED, and the blocker is CONFIRMED and now GENERALIZES.** `RunEncodeTrailers` has **exactly two non-test hits**, both `internal/filter/http/chain.go` — `:621` (its own doc comment) and `:622` (its own definition). A whole-repo search for the call form `\.RunEncodeTrailers(` returns **0 non-test, 1 test** (`chain_test.go:294`). `RunDecodeHeaders`: **178** hits, exactly as banked. ⚠️ **NEW, and it strengthens the rejection: the DECODE side is equally dead** — `\.RunDecodeTrailers(` is also **0 non-test, 1 test**. **The entire trailers pair is unreachable from production code.** Both charter-satisfying candidates (`grpc_http1_reverse_bridge`, `grpc_web`) sit behind that seam: **16-22+ tasks** of trailer-framework surgery.

**(E) The WASM row-summary rider — CONSIDERED EXPLICITLY AND DECLINED. The banked characterization is REFUTED.** Every figure re-derived TRUE: `internal/wasm/` **21 550** lines (55 files), rows 25/25.1/25.2/25.3 all `done` at `ROADMAP.md:73-76`, six fixtures `0034`-`0039`, a live `wasm.` arm at `name.go:97`, and proxy-wasm conformance **EXECUTED at this tip: exit 0, 10/10 PASS**. `grep -i 'wasm-family' ROADMAP.md` ⇒ **no match**, so the slug is genuinely absent. **But the missing item is NOT "merely a row-summary string."** `ROADMAP.md:216` carries `### WASM host family` as a forward-looking §9 section with **no chartered rows**, while **`ROADMAP.md:76` states verbatim that "phase 25 is the FINAL §9 HTTP-filters-family row"** and rows 25.x use "family" 23 times to declare that family CLOSED. **The WASM work was landed inside the HTTP-filters family and was used to close it.** Registering WASM as its own family therefore requires reconciling a landed, load-bearing closure statement — a doctrine adjudication, not a string edit. ⚠️ **AND THE TRAP IS LIVE: writing the slug silences sentinel check (3) for WASM BY MENTION** (`reference_sentinel_matcher_string_self_clears`), which `grep` cannot distinguish from a genuine registration. **DECLINED. Recorded here so the next roller does not re-adopt it as cheap.** For the record, the convention it would have to satisfy is `<ORDINAL-IN-CAPS> [§9 ]<Family>-family row`; exactly **nine** slugs satisfy check (3) today, and the only two §9 family headings without one are `### gRPC family` (`:190`) and `### WASM host family` (`:216`) — precisely alternatives (D) and (E).

### 11.2 What was and was NOT verified by execution

**VERIFIED BY EXECUTION at this tip:** the 30/30 drop with a 16/16 + 4/4 discriminating control · the `WriteProm` silent skip with actual bytes (116 B, 1 of 5 metrics emitted, `nil` returned) · the five live `switch` arms · the four-sink byte-identity result under a byte-mirror arm · `go build ./...` = 0 · `go test ./internal/... -count=1` green with three arms added · `go vet ./test/...` clean · proxy-wasm conformance 10/10 · the sentinel's three checks with two negative controls.

**NOT VERIFIED — carried as claims for the SPEC:** what reference Envoy actually emits for `envoy_access_logs_*` and `envoy_tracing_*` (**D-SPP-1, the row's largest risk**) · that `0118` is RED at the shipped tip rather than not-run (§2.4's liveness caveat) · the absolute stat surface 1207 (documentary; assert the DELTA) · every cost figure in §11.1, which is a re-derivation of a re-derivation and stale the moment the tip moves.

### 11.3 ⚠️ A SAMPLE IS NOT AN AUDIT — and this one is an audit

The 30 is not a sample. The sweep covered every `New{Counter,Gauge}[IfAbsent](` call site in `internal/` + `cmd/` (non-test), then every prefix/base literal root used to build stat names. **The complete set of stat-name roots in the codebase is `{cluster., http., listener., server., wasm., kafka., thrift., mongo., redis., sds., access_logs., tracing., runtime.}`** plus operator-`stat_prefix`-rooted names that route through the second-pass detectors. **No drop family exists beyond the four reported.** The denominator is stated.

### 11.4 ⚠️ CONTROLLER SELF-CORRECTIONS AND CITE REPAIRS, RECORDED RATHER THAN QUIETLY AMENDED

1. **`internal/bootstrap/bootstrap.go:621,622` are the CONST DECLARATIONS** (`runtimeNumKeysStat` / `runtimeNumLayersStat`), **not the registration sites** — registration is `result.Stats.NewGauge(...)` at **`:752,753`**. Fine for name identity, wrong for "where the stat is registered."
2. **The prometheus consumer is `flattenToProm` at `name.go:371`**, not `prom.go`; `prom.go:38` calls it. The full `ExtractTags` call-site set is **five**, not four-plus-prom.
3. **Fixtures `0099` and `0100` do NOT assert post-`ExtractTags` names** — both contain **zero** `"envoy_` literals and deliberately scrape the flat `/stats` (`0099:366`, `0100:426`), with a comment saying they do so *"NOT /stats/prometheus, whose names are mangled by ExtractTags."* **They exist precisely to avoid this surface.** `test/helpers/statsdrecv/statsdrecv.go` likewise asserts nothing by hand — it is a generic parser keyed by the caller's `subsetNames`.
4. **The banked six-fixture roster is an UNDERCOUNT with wrong members.** It misses **`0091-stats-sink-metrics-service-labels`** (`:164-166`) — the **only** fixture exercising `label.go`'s labelMapper (`emit_tags_as_labels`, ADR-0264; `0089`/`0090` use raw dotted names) — plus **`0112-stats-sink-otlp`** (`:144-146`) and **`0113-stats-sink-otlp-knobs`**. Repo-wide, **31** test files carry `"envoy_` literals.
5. **`name_test.go` is 959 lines and 55 `^func Test` — but 56 top-level funcs** (helper `sameLabels` at `:762`); `^func Fuzz` is **0** (fuzzers live in `fuzz_test.go`).
6. **The `0118` pin's cited range is a near-miss**: block 1 is `:142-166` (not `:154-166`) and the quoted sentence is `:272-276` (not `:274-280`).
7. **The six-gate range `:357-366` is off by two** — gates (a)-(f) occupy `:360-365`; `:357` is the heading and `:367` the closing sentence. The file is `BOOTSTRAP_PROMPT.md` (with the `.md` extension) at the repo root.
8. ⚠️ **`ROADMAP.md:140` — row 78 — is the ONLY MALFORMED ROW OF 110.** It has `NF=8` with **2 877 characters of IMPL summary sitting in field 8** and **no trailing `|`** (rows 138/139 both have `NF=8` with field 8 empty, i.e. a proper terminator; 109 of 110 rows end in `|`). GFM renders it as 7 cells against a 6-column header, so **the entire phase-78 IMPL summary is dropped from the rendered table.** ⚠️ **RECORDED, DELIBERATELY NOT FIXED:** the §Schema invariant at `ROADMAP.md:18` is *"Rows are never deleted. Only `status` and `sub-phases` columns are updated in place"* — repairing row 140 means editing a closed row's `summary` cell, which that invariant forbids. **Row 79 must not copy the shape: 7 pipes, one summary cell, terminated with ` |`.**
9. ⚠️ **A FOURTH `STATE.md` §Project-counts staleness axis, unflagged by any prior document.** Alongside the three self-declared at `STATE.md:22` (fixtures 119 vs **120**, stat surface 1205 vs 1207, DECISIONS tail ADR-0298 vs **ADR-0300**), `STATE.md:33` also asserts *"Next-free REFERENCE port is `10450`, NOT `10118` — ports are NOT fixture-index aligned."* **REFUTED by the landed fixture's own driver**: `0118/driver/driver.go:26-35` reads *"Convention `10<fixture index>` — 0114→10114 … so 0118→10118. ⚠️ NOT 10450: that is the TLS/SDS band (0108-0113)"* and sets `refListenerPort = 10118`. The §Project-counts sentence is right about `10450` **for the TLS band** and wrong as a general claim. **Next-free reference port is `10119`** (`grep -rn '10119' test/` ⇒ 0 hits; negative control `10118` ⇒ 5).
10. **There has NEVER been a BRAINSTORM-stage `PROGRESS.md` in this project.** `git ls-tree` at the phase-78 BRAINSTORM commit `c40cbddd` returns exactly one path (`BRAINSTORM.md`); `grep -rln '^# BRAINSTORM record' docs/envoy-go/phases/*/PROGRESS.md` ⇒ **0 files** across 120 phase dirs. ⚠️ **The router's instruction to "open `BRAINSTORM.md` + `PROGRESS.md`" is therefore WRONG for this stage** — `PROGRESS.md` is born at the PLAN. **This stage's file set is exactly four files** (§12), matching `c40cbddd`'s own `--stat` exactly.

### 11.5 ⚠️ WHAT AGREEMENT DOES AND DOES NOT BUY HERE

Four agents worked disjoint remits, so their agreement is **not** cross-validation of a shared claim (`reference_independent_probes_can_share_a_blind_axis`). Where two of them touched the same object — `ExtractTags`'s arm count — **they disagreed with the banked text in the same direction and for the same reason** (the stale `:350` error string), which is a shared *source*, not independent confirmation. **The 30/30 result rests on one probe with one control set.** The SPEC should re-run it rather than cite it.

---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these)

1. `ROADMAP.md`: register row 79 `in-progress`, depends-on `78`, sub-phases empty, **7 pipes terminated with ` |`** (§11.4 item 8). ⚠️ **Bump the sentinel's `want=110` → `want=111` in `next-prompt.txt` in the SAME commit** — a row-adding phase that forgets this hands every later session a `GATE FAIL`. Rehearse with controls BEFORE the edit.
2. ⚠️ **Re-run ALL THREE sentinel checks AFTER the `ROADMAP.md` edit lands**, and verify mechanically that **no matcher string leaked** into `ROADMAP.md`: the new cell text must contain **zero** occurrences of `deferred candidates:`, `remaining deferred (not-yet-chartered) candidates:`, and any `<Family>-family row` slug not already registered. **Negative-control each grep against a deliberately doctored copy** (`reference_gate_command_negative_control`) — a whole-file grep can falsify itself. ⚠️ **`Observability-family row` IS already registered and its use here is deliberate; `WASM` and `gRPC` slugs must NOT appear** (§11.1(E)).
3. Roll `STATE.md` §Current pointer **IN PLACE** (ADR-0288 — *"a stage close EDITS §Current pointer IN PLACE and never prepends a block above it"*, `DECISIONS.md:17010`). ⚠️ **§Recent lineage is AT its five-entry cap**; this close must evict the oldest bullet to `STATE_HISTORY.md`.
4. Roll `next-prompt.txt`.
5. **Do NOT create `PROGRESS.md`** (§11.4 item 10).
6. Commit + push (`feedback_push_to_origin`).
