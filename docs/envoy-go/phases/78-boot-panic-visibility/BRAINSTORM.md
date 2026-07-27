# Phase 78 Brainstorm — boot-panic VISIBILITY: a premature `defer` swallows **every** panic in a ~70-line boot window and converts it into a SILENT HANG with zero diagnostic output (an Operational-tooling-family MAINTENANCE row; +0 stats / +0 fixtures / +0 production packages / +0 modules / +0 fuzzers / +0 BackendKinds / +0 new PUBLIC surface — the shipped fix is a **defer RELOCATION**, ~8 lines moved)

**Stage:** BRAINSTORM (lifecycle-state `DONE` → 1). **Row 78 registered `in-progress` at this commit** per the ROADMAP §Schema invariant, RE-OPENING sentinel check (1) after its fourth-ever silent close.

**Self-picked** per the 2026-07-12 standing directive: smallest defensible candidate first. The pick and every rejected alternative are recorded in §2 and §11 with the evidence that settled each.

---

## 1. Mission and scope confirmation

### 1.1 What phase 78 delivers as a self-contained whole

`cmd/envoy-go/main.go:298-303` registers a shutdown `defer` that blocks on `<-flusherDone`. That channel is closed only at `:368` (the flusher goroutine) or `:370` (the no-flusher branch). **Any panic between those two points runs the defer, which waits forever on a channel nobody will ever close.** The panic message is never printed, the process never exits, and the operator gets a hung binary with **zero bytes of output**.

The row relocates the defer to *after* the `close(flusherDone)` branches, so a boot panic surfaces and the process dies loudly. It adds a black-box regression guard whose failing baseline is already executed.

### 1.2 What phase 78 does NOT deliver (forward to §8)

Any change to *whether* duplicate stat registration should panic at all, the reject-vs-accept parity question that sits behind it, and the other panic sites' individual merits. **This row makes panics VISIBLE; it does not re-litigate which ones should exist.** That boundary is deliberate and §8 explains why the other half is riskier than it looks.

### 1.3 Phase-done as an **Operational-tooling-family MAINTENANCE row** (family STAYS OPEN)

The row's core finding is an asymmetry between the binary's two entry paths: `--mode validate` **prints** the panic and exits 2, while normal boot **swallows** it. That asymmetry is the Operational-tooling family's subject matter. **The row claims NO family ordinal** — a maintenance row does not extend a charter (the row-76 precedent).

⚠️ **The attribution is a filing decision, not a sentinel maneuver, and that is checkable:** Operational-tooling's check-(3) marker is *already* satisfied, so naming it changes no sentinel check in either direction. There is no incentive here to mis-file, and the counter-argument is recorded rather than hidden: the family charter reads *"validation usable from OUTSIDE the binary's normal boot path"*, and this defect is **in** the normal boot path. It is filed here because the defect is defined by the *difference between* the two paths, which belongs to neither alone.

⚠️ **This row narrows NO candidates sentence, and that is stated rather than predicted.** The three Operational-tooling candidates are xDS-sourced dry-validation, an admin-API live-reload-and-validate endpoint, and an RTDS/SDS validate companion. None is this. Sentinel check (2) therefore **stays at its current site count through the IMPL** — consistent with the measured record that it has never gone down across ~18 phases. Predicting a decrease here would repeat the phase-73 error.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW, escape valve not expected to be needed

5–7 tasks (§10), unit-only, one production file. No split anticipated.

### 1.5 Package placement — ALL edits in EXISTING files, ZERO new packages

`cmd/envoy-go/main.go` (the relocation), `cmd/envoy-go/main_test.go` (the guard), and project docs. **+0 packages**, so `reference_new_subpackage_pulls_transitive_module` does not bite.

---

## 2. Design decisions

### 2.1 Row + subject confirmation *(SELF-PICKED → row 78 registered)*

#### ⚠️ THE HEADLINE — THE SAME CONFIG PRODUCES A PRINTED PANIC ON ONE ENTRY PATH AND A SILENT HANG ON THE OTHER

Reproduced by the controller at this tip, three arms, with a discriminating negative control:

| arm | config | invocation | result |
|---|---|---|---|
| **1** | two listeners sharing `stat_prefix: SAME` | normal boot | **EXIT=124 (timed out — HUNG). ZERO bytes of output.** |
| **2** | *negative control:* second prefix renamed | normal boot | boots cleanly — `envoy-go listener l1 ready … envoy-go listener l2 ready … envoy-go ready` |
| **3** | **the SAME config as arm 1** | `--mode validate` | **EXIT=2**, panic **PRINTED**: `panic: stats: duplicate metric registration: "http.SAME.downstream_rq_total"`, full trace through `internal/stats/registry.go:107` |

**Arm 1 versus arm 3 is the decisive pair.** Same config, same panic, same registry line — printed on one path, swallowed on the other. That isolates the cause to the boot path's `defer`, not to the panic itself and not to the config. Arm 2 proves the harness discriminates: without the duplicate, the binary boots and runs.

#### The mechanism, read first-hand

`main.go:298-303`:

```go
	defer func() {
		<-flusherDone // wait for the Flusher goroutine to stop Submitting before closing the sink channels
		for _, s := range statsSinks {
			_ = s.Close()
		}
	}()
```

`flusherDone` is closed only at `main.go:364-371` — inside `go func() { defer close(flusherDone); statsFlusher.Start(ctx) }()`, or in the `else` branch `close(flusherDone)` when no flusher is configured. **Between `:303` and `:368` the channel has no closer.** A panic in that window unwinds into a defer that blocks on it permanently, so the runtime never reaches the point where it would print the panic and exit.

⚠️ **The window is NOT stat-specific.** It spans `boot.Construct`, `admin.New`, `lm.Start` and `bs.Stats.Freeze` — it swallows a panic from any of them. The stats registry merely supplies the easiest trigger: `internal/stats/registry.go` carries **five** `panic(` sites (`:107` duplicate registration, `:117` invalid metric name, `:129` registry frozen, `:165`/`:212` the `IfAbsent` type-mismatch pair).

#### Why this is the right pick, in order

1. **It is the smallest verified candidate** — 5–7 tasks against 10–14 for the nearest rivals (§11.1), and the production change is a **relocation**, not new logic.
2. **The failure mode is the worst available**: not a wrong value, not a missing metric, but a process that hangs with **no output at all**. Every diagnostic an operator would reach for is absent by construction.
3. ⚠️ **It is a PREREQUISITE for the safe endgame of the strongest rival.** The banked `/stats/prometheus` projection row (§11.1(A)) has as its "impossible by construction" fix a **registration-time validation** that rejects unprojectable names — i.e. a **boot panic**. On today's `main.go` that panic becomes a silent hang. **Landing that fix before this one converts a missing-metric bug into a hung binary.** The ordering is not a preference; it is forced by the evidence.
4. It is unit-testable end to end with **no reference-parity question at all** (§2.3), so it carries none of the Docker-probe cost that dominated this stage's other candidates.

### 2.2 Scope: the relocation *(SPEC pins — D-BPV-FIX)*

Move the `:298-303` defer block to immediately **after** the `close(flusherDone)` branches at `:364-371`. **Nothing Submits to `statsSinks` before `:368`**, so the early placement protects nothing — the defer's own doc comment describes a shutdown-ordering contract that is equally satisfied at the later position, because Go runs defers LIFO and the later registration still precedes every shutdown path that can Submit.

⚠️ **The SPEC must verify the LIFO consequence explicitly rather than assume it.** Relocating a defer changes its position in the LIFO stack relative to `lm.Stop()` and `tracingProvider.CloseAll()` (`:305`). The existing comment at `:304-308` documents an intended ordering (`lm.Stop()` → tracing → access-log sinks); the SPEC owes a statement of the post-move order and a check that the sink contract *"no Submit after Close"* still holds. **A fix that restores panic visibility while silently reordering shutdown would be a bad trade.**

### 2.3 The verification design — a black-box guard with an ALREADY-EXECUTED failing baseline *(D-BPV-GUARD)*

A test in `cmd/envoy-go/main_test.go` that spawns the real binary on a duplicate-`stat_prefix` bootstrap and asserts it **exits non-zero within a bounded timeout with the panic text on stderr**.

- ⚠️ **Its failing baseline is already measured, not predicted** (`reference_liveness_break_needs_failing_baseline`): today arm 1 produces `EXIT=124` and **0 bytes**. A green-by-default test is impossible here — the pre-fix state is unambiguously red.
- **A ready-made template exists**: `main_test.go` already spawns the binary via `buildBinaryOrSkip` + `exec.CommandContext` (the `TestMain_StatsPrometheusEndpointResponds` shape).
- ⚠️ **The assertion must be on BOTH the exit status AND the output.** Exit-status alone is satisfiable by any failure; output alone is satisfiable by a process that prints and then hangs. The defect is precisely the conjunction, so the test must be too.
- ⚠️ **The timeout must be generous and the failure message must distinguish "hung" from "exited wrong".** A bare timeout failure reads identically to a slow machine.

### 2.4 Fixture posture: **+0 new fixtures**, and no differential fixture at all *(D-BPV-FIXTURE)*

This is a subject-side-only defect: it concerns what the **envoy-go binary** does when it panics. There is no cross-side behaviour to compare. ⚠️ **And the obvious differential harness is the WRONG SHAPE**: `BootRejectFixture`'s contract is that **both** sides exit non-zero, but the reference **boots green** on a duplicate-`stat_prefix` config (Envoy's stat scope is get-or-create, not register-once). Using it would assert a divergence this row is not making. Next-free fixture index stays **0119**.

### 2.5 Stat surface hypothesis: **+0** (1207 → 1207)

No `NewCounter`/`NewGauge` call site moves. The row changes *when a defer runs*, nothing else.

---

## 3. Framework-survey result

No framework gap. The seam is a defer's position in one `main()`.

---

## 4. Bootstrap-level applicability — NONE

No bootstrap field is consumed; no parse arm, no reject arm, no fuzz target. ⇒ **+0 fuzzers.**

---

## 5. Stat surface hypothesis — **+0** (1207 → 1207)

See §2.5. Asserted as a DELTA; the absolute is documentary and rides two unaudited ledger gaps.

---

## 6. Anticipated edit sites *(the SPEC RE-DERIVES each at ITS OWN tip — a BRAINSTORM cite is not evidence)*

| file | what |
|---|---|
| `cmd/envoy-go/main.go` | relocate the `:298-303` defer to after `:364-371`; correct the two doc comments that describe the old placement |
| `cmd/envoy-go/main_test.go` | the black-box guard (§2.3) |
| `docs/envoy-go/DECISIONS.md` | **ADR-0300** — the panic-visibility contract and the shutdown-ordering argument |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | a boot-failure-visibility statement; **no stat-ledger line** (the surface is unchanged) |
| `docs/envoy-go/ROADMAP.md` | row 78 (this commit) + the §11.1(A) deferral is recorded as a NAMED candidate so the audit is not lost |

---

## 7. BRAINSTORM-time open questions to the SPEC — the D-BPV-* docket

- **D-BPV-ORDER.** State the post-relocation LIFO shutdown order explicitly and prove the sink contract (*no Submit after Close*) still holds. ⚠️ **This is the one way the fix could do harm**, and it must be settled by execution, not by reading the comment that describes the old order.
- **D-BPV-WINDOW.** Enumerate what else the window swallows. The four named entry points (`boot.Construct`, `admin.New`, `lm.Start`, `bs.Stats.Freeze`) are agent-reported; the SPEC should derive the window's true span at its own tip and say which of the five `registry.go` panic sites are boot-reachable. ⚠️ Reported as "3 certainly + 1 conditionally" — **that classification is INHERITED and unverified by the controller.**
- **D-BPV-OTHER-DEFERS.** Is this the only defer in `main()` that can block indefinitely? A fix that relocates one and leaves a sibling is a partial fix wearing a full fix's commit message.
- **D-BPV-LOGFATAL.** `log.Fatalf` paths call `os.Exit`, which **skips defers entirely** — so they are unaffected. The SPEC should confirm that and state which boot failures take which path, because it determines what the guard can and cannot catch.
- **D-BPV-GUARD-TRIGGER.** The duplicate-`stat_prefix` config is a convenient trigger. Is it the *stable* one? If a future row makes duplicate registration non-fatal, the guard goes vacuous. Prefer a trigger that cannot be legislated away, or pin the dependency explicitly.

---

## 8. What phase 78 does NOT deliver (forward)

- **Whether duplicate `stat_prefix` should be REJECTED rather than panic.** ⚠️ **Deliberately forwarded, and it is the riskier half.** Envoy's stat scope is **get-or-create**, so the reference **boots green** on that config. Making envoy-go reject it is a **deliberate divergence** needing its own ADR, and — per §2.4 — the `BootRejectFixture` harness is the wrong shape for it. Folding it in would smuggle a parity decision into a visibility fix.
- **The `/stats/prometheus` projection-completeness gap** — §11.1(A). **Recorded as a named ROADMAP deferral at this commit** so the audit behind it survives.
- **The other panic sites' individual merits.** This row makes them visible; it does not judge them.

---

## 9. ADR-0045 split readiness + ADR roster

Single flat row; no split anticipated. **ONE new ADR: ADR-0300** (next-free; DECISIONS tail is ADR-0299 COMPLETE). ⚠️ **It carries NO whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297, and escalated at the phase-76 BRAINSTORM from a wrong number to a flipped sentinel check. Every count line-scoped or stated with no numeral.

⚠️ **`ADR-0089` stays BYTE-UNTOUCHED** — this row lands neither `/runtime` nor `POST /runtime_modify`.

---

## 10. Envelope + counts (anticipated at the phase-78 IMPL; docs-only at this BRAINSTORM)

**Counts re-derived MECHANICALLY by the controller in the stage worktree at this tip — never copied:** fixtures **120** · fuzzers **55** · internal packages **73** · blank imports in `runner_test.go` **120** · ROADMAP **225 lines / 109 data rows** → **110** at this commit · BEHAVIOR_CONTRACT **5750** · DECISIONS **17531** · ADR tail **ADR-0299** (next-free **ADR-0300**) · next-free fixture **0119** · next-free reference port **10119** (family-banded `10<index>`).

⚠️ **`STATE.md` §Project counts still records fixtures as 119 while the mechanical count returns 120.** Not drift — phase 77 landed `0118` after that block was written, and `BEHAVIOR_CONTRACT.md:847` already carries 120. Recorded because the two numbers coexist in the tree.

Anticipated deltas: **stats +0 / fixtures +0 / fuzzers +0 / BackendKind +0 / go.mod modules +0 / packages +0 / new PUBLIC surface +0.** Task count **5–7**.

---

## 11. Sized-against-source — the derivations (FOUR agents at tip `4d7f63c2`, plus controller re-derivation)

### 11.1 The rejected alternatives, with the evidence that settled each

**(A) The `/stats/prometheus` PROJECTION-COMPLETENESS gap — REJECTED AS THE PICK, BANKED AS THE STRONGEST SUCCESSOR, and its audit is recorded in the ROADMAP so it cannot be lost.**

This was the router's named candidate and this stage's own draft pick. It is real, large, and now **fully measured** — which is exactly why it is not the smallest defensible row.

envoy-go silently discards every registered stat whose top-level segment the name projection does not recognise. **Thirty live registered names across six families:**

| dropped family | names | landed | pre-dates 77? |
|---|---|---|---|
| `access_logs.grpc_access_log.*` | 2 | phase 44.1, 2026-06-25 | **YES** |
| `access_logs.open_telemetry_access_log.*` | 2 | phase 45.1, 2026-06-26 | **YES** |
| `tracing.opentelemetry.*` | 2 | phase 46.1b, 2026-06-28 | **YES** |
| `tracing.zipkin.*` | 2 | phase 46.2, 2026-06-28 | **YES** |
| `sds.<secret>.*` | **20** | phase 60.1, 2026-07-13 | **YES** |
| `runtime.*` | 2 | phase 77 | no |

⚠️ **28 of the 30 pre-date phase 77** — so the inherited framing (*"the runtime prometheus fix"*, a two-name row) is **REFUTED**. Phase 77 was the **sixth** family to hit the gap and the **first to notice**. Two independent methods agreed exactly: a static read of the full accept set (5 prefix arms + 8 second-pass detections, **3 carrying closed allow-lists**) across 293 registration sites, and a dynamic sweep of **4145 distinct flat names over 107 bootable fixtures** with **271 end-to-end confirmations and zero mispredictions in either direction**, controls shown. Controller-re-derived independently with its own controls.

⚠️ **The gap was already documented in code at phase 32.1 and never enumerated:** `internal/admin/stats.go:12-22` says the flat `/stats` endpoint exists precisely for *"bypassing the /stats/prometheus path which silently skips unrecognized top-level segments (the redis. Prometheus tag-extractor arm is 32.2)"* — **the project has run this exact play once already.**

Additional measured facts the successor row inherits: `sds.` needs a **label-hoisting** arm (`envoy_xds_resource_name`), not a plain flatten — ⚠️ **measured ONCE, by one agent; the SPEC must re-pin it** · the other three are byte-mirrors of the SN5 `server.` arm · a trial edit applied cleanly (71 packages green) and made fixture `0118`'s red-on-fix pin **FIRE exactly as designed** · **statssink blast radius is ZERO and provably so** — all four sink consumers already fall back to the full dotted name · the stat surface delta is **+0**, a *projection* delta not a *registration* delta · the reference emits **no `# HELP` lines at all** (controller-measured: 800 lines, 255 `# TYPE`, **0** `# HELP`, 0 blank lines) so ~30 helpText entries carry **zero parity content** · reference Envoy's formatter is **TOTAL** (it sanitizes; a cluster named `c.echo-weird:name#1` emitted a present-but-mis-split metric, sample count 680 == 680) while envoy-go's is **PARTIAL** — the asymmetry is self-inflicted, not a parity accommodation.

**Rejected as the pick on two grounds, both evidence-based:** it is 11–14 tasks against 5–7, and — decisively — **its own endgame is blocked behind the pick** (§2.1 point 3).

**(B) `fault.abort.grpc_status` — REJECTED, RE-BANKED with a CORRECTED cost.** `abortEnabled` is set on **exactly one line** (`fault.go:144`) inside the `HttpStatus` arm, so a `grpc_status` variant is *accepted-and-inert*: `abortPercentage` **is** populated and attached to a permanently-false flag, and the request reaches the backend. ⚠️ **Its inherited ~7–9 cost is REFUTED; re-derived 10–13** — the ~7–9 figure silently dropped a ~10–13 finding recorded one row earlier, and the omitted task is **a SECOND live divergence on already-shipped code**: the reference **transcodes an `http_status` abort into the gRPC shape** when the request carries `content-type: application/grpc` (503 → HTTP **200** + `grpc-status: 14`), measured across 20 arms; envoy-go emits `text/plain` unconditionally. Fixture `0011` exercises that path and is green only because its probes send no `content-type`. ⚠️ Confirmed it would **NOT** open the gRPC family — but the contrary claim's provenance is **two** sites, not the one previously recorded (`ROADMAP.md:48` routing label **and** `BEHAVIOR_CONTRACT.md:2142`).

**(C) Agent-reported candidates the controller did NOT verify** — recorded as leads, explicitly not as findings: an H2 `//`-path `ParseRequestURI` defect (~4–6 tasks, reportedly the sole outlier across H1/H2/H3); `host_rewrite_literal` (~5–6, re-sliced away from `prefix_rewrite`, which needs matched-prefix plumbing); the `upstream_cluster` span tag (~6/~10); `ssl.connection_error` (~11–12); an undocumented `server_name` silent-ignore with an `APPEND_IF_ABSENT`-vs-`OVERWRITE` divergence masked by an allow-list in `test/helpers/http_diff.go`. ⚠️ **Each is a single-agent claim at this tip. None is costed here and none should be adopted without re-derivation.**

**(D) The documentary-defect rows** (the non-existent public import path; a mechanical stat-surface count; the unresolved `BEHAVIOR_CONTRACT` stale-cite audit) — cheaper than the pick but touching no live behaviour. Deferred, still named.

**(E) Opening the gRPC or WASM families to clear check (3)** — rejected, unchanged from phase 77's finding. No zero-code WASM relabel is legitimate (writing the marker string without a genuine row is the MENTION-not-USE failure the sentinel cannot detect), and gRPC's charter-satisfying candidates remain blocked on the same seam.

### 11.2 What was and was NOT verified by execution

**EXECUTED BY THE CONTROLLER:** the three-arm reproduction of the hang with its negative control and the `--mode validate` contrast · the first-hand read of `main.go:292-310` / `:360-375` and the five `registry.go` panic sites · the independent re-derivation of the §11.1(A) drop set with controls · the live reference probe for HELP/line-shape (image `7edd5b0fd763…` verified against the pin) · all project counts · the sentinel `want=110` rehearsal with three arms · the absence of a per-fixture runner branch.

**EXECUTED BY AGENTS, controller-reviewed but not re-run:** the 4145-name / 107-fixture sweep · the 293-site registration classification · the `fault` 20-arm reference cross-product · the `0118` trial edit and pin firing · the statssink byte-identity argument.

**INFERRED, NOT EXECUTED:** the 5–7 task count (a costing, not a measurement — `reference_deferred_candidate_cost_restale` says re-derive it at the adopting tip) · the claim that relocating the defer preserves shutdown ordering (**D-BPV-ORDER owes this a run**) · the window's exact span and the boot-reachability classification of the five panic sites · every item in §11.1(C).

### 11.3 ⚠️ A SAMPLE IS NOT AN AUDIT — AND THE TWO POINTED IN OPPOSITE DIRECTIONS

Two agents worked the §11.1(A) audit question at the same tip. One **sampled** (14/14 names on one config, plus five prom-sensitive fixtures green) and wrote that the answer *"looks like no … but that is a sample, not an audit, and I am not reporting it as one."* The other **audited** (107 fixtures, 4145 names) and found **28 pre-existing dropped names**.

**The sample leaned the wrong way, and the discipline that saved it was refusing to report a sample as an audit.** Had that restraint not held, this stage would have recorded a two-name deferral on a false negative — and the false negative would have been *corroborated* by a green test run. **Coverage is a property of the input set, not of the run's exit code.**

### 11.4 ⚠️ TWO CONTROLLER SELF-CORRECTIONS, RECORDED RATHER THAN QUIETLY AMENDED

**(1) A hand-invented control input manufactured a phantom defect.** The controller fed `listener.0.0.0.0_10000.downstream_cx_total` through `ExtractTags` and observed the address truncated to `0` with residual `0.0.0_10000` — which reads exactly like a live SN3 bug. **It is not.** `normalizeAddr` (`internal/listener/manager.go:352`) replaces every `.` and `:` with `_` before the name is built, and its doc comment at `:337-342` says it does so *precisely because* SN3's `strings.Index(tail, ".")` would otherwise truncate to the first IPv4 octet. **The probed name shape never occurs.** Reporting it would have been a fabricated finding. ⚠️ **A probe's INPUT is a claim too** — inventing a plausible-looking name is not observing a registered one.

**(2) An EMPTY result read as a CONFIRMING result.** The controller's first reference probe returned a file with **0 `# HELP` lines** — the agent's exact claim, apparently confirmed. The container had never started: `node: { id: n }`, where YAML 1.1 parses the bare `n` as boolean `false`, and Envoy rejected `id: false`. **The "confirmation" came from an empty file.** Caught only because the probe printed the total line count beside the match count. ⚠️ **A zero-match grep and an empty input are indistinguishable unless you measure the input.** The re-run on a verified-`LIVE` container gave 800 / 255 / **0** / 0 — the claim is true, but the first evidence for it was worthless. Same species as a sibling agent's report that a **sandboxed** `docker run` + `curl` returned a full, plausible stats body for a container stuck in `Created` with nothing bound.

### 11.5 ⚠️ A NEW SHARED-RESOURCE HAZARD, FOUND BY CAUSING IT

One agent's cleanup ran `docker ps | grep contrib-v1.37.2 | xargs docker kill` and killed **three containers belonging to concurrently-running sibling agents** — its own `--rm` containers had already self-terminated, so every container it actually killed was someone else's. `reference_parallel_subagents_private_scratch` covers scratch *directories*; **it does not cover docker containers, ports, or any other machine-global namespace.** Parallel agents must tear down **by their own container NAME**, never by an `ancestor`/image filter. The controller's own probes were removed by name for exactly this reason.

### 11.6 ⚠️ A "MEMORY CORRECTION" THAT WAS ITSELF WRONG — AND THE CONTROLLER PUBLISHED IT BEFORE CHECKING

An agent reported that `reference_differential_fixture_three_registration_gates` records a **runner branch** as one of its three gates, and that no such branch exists at this tip. The controller confirmed the *code* half by grep (`fx == "` returns nothing; dispatch is `discoverFixtures` at `runner_test.go:187` plus `fixture.DriverRegistry[fx]` at `:193`, and a `switch` branch is needed only for a new **BackendKind** at `:272`) — **and shipped the "correction" into two documents without reading the memory it claimed to correct.**

**The memory text says no such thing.** Its three gates are (1) the directory, (2) `RegisterFixture` with the name equal to the directory, (3) the blank import in `runner_test.go`. **All three are accurate at this tip.** The word *branch* comes from a NEIGHBOURING memory, `reference_differential_fixture_dispatch_constraint`, where it means a **dispatch MODE** — the if/return chain selecting reference-less XOR boot-reject XOR cross-side — **not a per-fixture name branch**. That memory is also correct as written.

⚠️ **So the grep result is consistent with BOTH memories being right, and it was read as evidence that one was wrong.** A negative grep for a thing no document ever claimed is not a refutation of anything. `reference_a_drift_correction_is_itself_a_claim` names exactly this; the controller had the rule, cited it elsewhere in this very document, and still applied a correction it had not re-derived. **The memories are UNCHANGED and both stand.**
---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these)

1. `ROADMAP.md`: register row 78 `in-progress`; record the §11.1(A) projection-gap audit as a NAMED deferral so it survives; ⚠️ **bump the sentinel's `want=109` → `want=110` in `next-prompt.txt` in the SAME commit.** The bump was rehearsed with controls BEFORE the edit: `want=110` over a 110-row file prints only `NOT DONE: row 78`; `want=109` over the same file additionally prints `GATE FAIL: examined 110 … expected 109`; the positive control (row 78 `done`) is silent.
2. ⚠️ **Re-run ALL THREE sentinel checks AFTER the ROADMAP edit lands** and verify mechanically that **no matcher string leaked** into `ROADMAP.md` — check (2) must still return exactly its known sites and check (3) must still report gRPC and WASM, which a leaked `-family row` token would silence.
3. Roll `STATE.md` §Current pointer **IN PLACE** (ADR-0288) and `next-prompt.txt`.
4. Commit + push (`feedback_push_to_origin`).
