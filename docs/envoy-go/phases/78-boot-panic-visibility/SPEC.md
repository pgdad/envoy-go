# SPEC 78 — boot-panic VISIBILITY: a premature `defer` swallows **every** panic in a 68-line boot window and converts it into a SILENT HANG (an Operational-tooling-family MAINTENANCE row claiming NO family ordinal — **+0 stats (1207 → 1207) / +0 fixtures (120) / +0 fuzzers (55) / +0 BackendKinds (tail 38) / +0 go.mod modules / +0 packages (73) / +0 new PUBLIC surface**)

**Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; **ZERO production `.go`, ZERO test `.go` in the SPEC commit.** Fresh worktree `/home/esa/git/envoy-go-wt-p78spec` off master `e9fb2088`, branch `phase-78-spec`, per `feedback_git_worktrees`. **ROW 78 STAYS `in-progress`** — it flips `done` only at the phase-78 IMPL six-gate (**`BOOTSTRAP_PROMPT.md` §7.5**; the SOLE leg per **ADR-0106** — see §1.2 R9). **`ROADMAP.md` and `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED; `DECISIONS.md` gains ADR-0300 §Context.**

**FOUR investigation agents on disjoint remits with private scratch and private throwaway worktrees, plus controller re-derivation of every load-bearing claim.**

---

## 1. Purpose / Mission

### 1.1 ⚠️ THE HEADLINE — THE FIX AS SPECIFIED IS **INSUFFICIENT**, AND EXECUTION SAYS SO

The BRAINSTORM §2.2 specifies the shipped change as: *"Move the `:298-303` defer block to immediately after the `close(flusherDone)` branches at `:364-371`."* It also flags D-BPV-ORDER as *"the one way this fix could do HARM"* and demands it be settled by a run.

**It was, and the relocation ALONE does not remove the hang. It MOVES it.**

Relocating the defer makes it the **last** registered defer in `main()`, hence **first** in LIFO — i.e. it now runs **before** `defer cancel()` (`main.go:340`). Its `<-flusherDone` is released only when `statsFlusher.Start(ctx)` returns, which requires `ctx` to be cancelled. On the naive relocation a panic in the *new* post-anchor window unwinds into a wait for a flusher that nothing has told to stop.

Controller cross-product, EXECUTED — **the fourth row is the finding**:

| tree | panic position | stats sink configured | EXIT | stderr | verdict |
|---|---|---|---|---|---|
| baseline `e9fb2088` | pre-anchor (`boot.Construct`) | yes | **124** | **0 B** | the phase-78 defect |
| naive relocation | pre-anchor (`boot.Construct`) | yes | **2** | 2277 B, panic printed | **FIXED** |
| naive relocation | **post**-anchor | **no** | **2** | 140 B | no defect *possible* — the `else` branch closes `flusherDone` **before** the defer is registered |
| naive relocation | **post**-anchor | **yes** | **124** | — | ⚠️ **THE MOVED HANG** |
| relocation **+ `cancel()`** | **post**-anchor | **yes** | **2** | 140 B, panic printed, **0.0105 s** | fixed |

⚠️ **The moved hang is invisible to a SIGTERM-based harness, and that is why three of the four agents missed it.** Measured on the same binary and config:

- `timeout -s TERM 15` ⇒ `EXIT=124`, wall **15.005 s**, and the panic text **is** present — because the SIGTERM cancelled `ctx`, which released the wait, which let the panic finally print. Read naively this looks like *"printed, just slow."*
- `timeout -s KILL 10` ⇒ `EXIT=137`, **panic text ABSENT**. With no rescuing signal it never prints.

⇒ the moved defect is *"hang until a signal arrives"*. **An unsupervised boot never prints, and any harness whose deadline sends SIGTERM cannot see it.**

**RESOLUTION — D-BPV-FIX is redefined.** The shipped change is **not** "move the defer". It is **"move the defer AND make its wait unable to outlive an unwinding `main`"** (§3.1).

### 1.2 ⚠️ THE SECOND HEADLINE — BOTH GUARDS THE STAGE DESIGNED **PASS ON THE BUGGY TREE**

The behavioural guard (duplicate `stat_prefix`) and the structural guard (`<-flusherDone` must be positioned after every `close(flusherDone)`) were each designed, implemented and break-tested by an agent, and each is sound for what it targets. **Neither can see the moved hang**, and this was verified rather than reasoned:

- The behavioural guard's trigger panics in `boot.Construct`, i.e. **pre**-anchor. On the naive relocation that path is genuinely fixed — EXECUTED: `EXIT=2`, panic text present ⇒ **PASSES**.
- The structural guard asserts *every `<-flusherDone` receive line > max `close(flusherDone)` line*. On the naive relocation the receive is at `:367` and the closes at `:362`/`:364` ⇒ `367 > 364` ⇒ **PASSES**.

⇒ a tree that still hangs would have shipped with **two green purpose-built guards and a passing IMPL six-gate**. §3.7 adds the third arm that closes this, and it is a **new docket item this SPEC opens rather than inherits**.

### 1.3 BRAINSTORM drift ledger — RE-DERIVED, CONFIRMED, REFUTED

#### CONFIRMED by controller re-derivation (not copied)

- **C1.** The three-arm reproduction. Re-run at this tip: arm 1 `EXIT=124 / 0 B / 0 B` (2× repeats, deterministic); arm 2 negative control boots (`envoy-go ready`); arm 3 same config under `--mode validate` ⇒ `EXIT=2`, panic printed, trace `internal/stats/registry.go:107` ← `internal/listener/manager.go:317` ← `internal/boot/boot.go:90` ← `validate/validate.go:49` ← `cmd/envoy-go/main.go:62`.
- **C2.** The five `internal/stats/registry.go` `panic(` sites are at `:107 :117 :129 :165 :212` **exactly** as inherited.
- **C3.** `buildBinaryOrSkip` (`main_test.go:191`), `TestMain_StatsPrometheusEndpointResponds` (`:335`) and `TestEnvoyGoBinary_ModeValidate` (`:938`) all exist as named. **No name in the BRAINSTORM is wrong.**
- **C4.** The window's span is `main.go:304-371` — **68 lines**. The BRAINSTORM's *"~70-line"* is accurate.
- **C5.** The defer at `:298` is registered **unconditionally** — it sits *outside* the sink `if`, which ends at `:288`. A boot with **zero** stats sinks still arms it, and still hangs. (This is what makes the *original* defect sink-independent, in contrast to the *moved* one.)
- **C6.** The §11.1(A) projection-gap audit did land as a NAMED deferral — `ROADMAP.md:208`, appended to the Observability family's **existing** sentence, which is why check (2) held at five sites rather than gaining a sixth. It records the blocking order explicitly.

#### REFUTED / CORRECTED

- **R1 — THE FIX AS SPECIFIED.** §1.1. The relocation alone moves the hang. **This is the most consequential correction in the row.**
- **R2 — "3 certainly + 1 conditionally boot-reachable" of the five registry panic sites.** The BRAINSTORM labels this INHERITED and UNVERIFIED. **True answer: ONE is config-reachable, four are not**, established independently and in agreement by two agents. `:117` is unreachable because every config-derived name segment passes `stats.IsValidName` at the input boundary (~13–34 guard sites depending on the counting method — the SPEC states the property, not a number); `:129` is unreachable because `Freeze()` has exactly one non-test call site and everything after it in-window only spawns goroutines; `:165`/`:212` survived a bounded structural search over **all 70** `*IfAbsent` call sites (per-family counter-leaf and gauge-leaf rosters are disjoint and roots differ) — **labelled as "no construction found", not as "impossible".**
- **R3 — "the window is NOT stat-specific … registry.go carries FIVE panic sites"** is true of the *window* but **misleading about the trigger inventory**. There is exactly **one** config-reachable boot-window panic trigger (`registry.go:107`, reached by two filter instances sharing a `stat_prefix`) and **no second independent trigger to fall back on**.
- **R4 — the mechanism is panic-GENERIC, not `panic(`-literal-specific.** A nil-deref runtime panic inserted in the window hangs identically (`EXIT=124`, 0 B). ⇒ **scoping the guard to "the five registry sites" or "the 18 `internal/` `panic(` sites" scopes it to the wrong set** — runtime panics (nil deref, index range, failed type assertion, nil-map write) carry no `panic(` token and are the dominant exposure.
- **R5 — "exit-status alone is satisfiable by any failure"** is too weak in one direction and the BRAINSTORM missed the other. Measured: under a bounded-timeout harness the **healthy** negative control *also* exits 124, so exit-status alone is satisfied by **success**. And independently, a deliberately-built *print-then-hang* tree (recover + print the exact panic + `select{}`) **passes an output-only assertion**. ⇒ **both halves of the conjunction are proven load-bearing, each by its own executed counter-example.**
- **R6 — "the fix … plus correct the two doc comments that describe the old placement"** understates the prose burden. `flusherDone` occurs at **9 sites in one file** (4 code, 5 comment), and a full audit of the 7 candidate `defer`/LIFO comment sites in `main()` returns **3 FALSE, 2 MISLEADING/INCOMPLETE, 2 SURVIVE** (§3.8).
- **R7 — the dispatch brief's own hypotheses about sibling defers are REFUTED.** `admSrv.Close()` calls `httpSrv.Close()`, the **immediate** variant — `.Shutdown(` has **zero** production hits (all four are `test/`). `lm.Stop()` contains no `Wait()` and no channel receive; its doc comment states in-flight handlers are deliberately not waited on, and the drain rendezvous lives in `main()` **before** the defer chain, itself bounded by `time.After(drainMgr.Timeout())`.
- **R8 — a second config-reachable duplicate-registration trigger does NOT exist.** Two listeners on the **same address** was the obvious candidate; it is refuted — `registerListenerMetrics` runs **post-bind**, so the second listener dies at `bind: address already in use` (`EXIT=1`) and never reaches registration.
- **R9 — the six-gate citation the lineage carries is compressed, and the router collapsed it further.** Recent stages write *"the IMPL six-gate (ADR-0106)"*; the router's Read-first list reads *"ADR-0106 (the six-gate)"*. **ADR-0106 does not define a six-gate** — its block `DECISIONS.md:4788-4857` contains **zero** occurrences of "six", and its subject is the §9 family-expansion shape (flat top-level rows, no-sibling-stub, the SOLE-leg property). The six gates (a)–(f) are defined at **`BOOTSTRAP_PROMPT.md:357-366`**, at the **repo root**, not under `docs/envoy-go/`. The compound cite is sound only read as *"six-gate (`BOOTSTRAP_PROMPT` §7.5) … the SOLE leg (ADR-0106)"*. **This SPEC cites both correctly.**
- **R10 — the inherited 5–7 task cost is too LOW at the bottom. Re-derived: 7–9** (§9).
- **R11 — `main.go:304-308` (the Phase 46.1b LIFO comment) does NOT become false.** The controller's own dispatch brief suggested it would. Post-move, tracing still runs after `lm.Stop()` and before the access-log sinks close — **both clauses survive.** Recorded because a prose sweep driven by the brief would have "corrected" a true comment.

### 1.4 SPEC-time verification record

**EXECUTED BY THE CONTROLLER:** the three-arm reproduction with repeats · the `{panic window} × {stats-sink presence}` cross-product on baseline, naive-relocated and `cancel()`-paired trees · the SIGTERM-vs-SIGKILL deadline contrast that exposes the signal-rescue · the D-BPV-LOGFATAL call-site-fixed control · the demonstration that both proposed guards pass on the buggy tree · every count in §10 · all three sentinel checks · the ADR-0106/`BOOTSTRAP_PROMPT` §7.5 cite audit with matcher discrimination · the `flusherDone` code/comment site split · the repo-wide `package main` scope check.

**EXECUTED BY AGENTS, controller-reviewed:** the post-anchor hang with SIGQUIT goroutine dumps naming the blocked frame · the deliberate contract-violating build (`panic: send on closed channel` from `statssink.(*TCPStatsdSink).Submit`) proving a panic-path harness discriminates where a SIGTERM one does not · the AST defer census (two methods reconciled) and the repo-wide blocking-shape sweep · the injected-blocking-sibling positive control · the boundary three-point probe (`pre`/`mid`/`post`) · the nil-deref generic-panic probe · the registry reachability negative controls · the six-arm guard break roster including print-then-hang · the structural guard's rename anti-vacuity arm.

**INFERRED, NOT EXECUTED:** the 7–9 task count (a costing, not a measurement) · the unreachability of `registry.go:165`/`:212` (a bounded structural search over the full 70-site enumeration — no construction found, impossibility **not** proven) · the unreachability of `:129` · the claim that the full differential suite is unaffected by the shutdown-order change (**the IMPL owes this a run** — §9 T8).

---

## 2. Non-purposes (deferred; BRAINSTORM §1.2 + §8)

1. **Whether duplicate `stat_prefix` should be REJECTED rather than panic.** The reference **boots green** (its stat scope is get-or-create, and a live probe recorded at row 77 found it **merges** two listeners sharing a prefix into one scope with the counter **summed**). A reject is a deliberate divergence owed its own ADR, and `BootRejectFixture`'s contract (**both** sides exit non-zero) is the wrong harness shape for it. Folding it in would smuggle a parity decision into a visibility fix.
2. **The other panic sites' individual merits.** This row makes them visible; it does not judge them.
3. **The `/stats/prometheus` projection-completeness gap** (`ROADMAP.md:208`). Banked, and **blocked behind this row**.
4. **The two new hazards this SPEC's audit found** (§12) — an unbounded shutdown-class sink `Close`, and a second silent zero-byte boot hang that is not a defer at all.

---

## 3. The change — the D-BPV-* docket disposed one-for-one

### 3.1 D-BPV-FIX **[REDEFINED BY EXECUTION — relocation ALONE is insufficient]**

**The shipped production change is the relocation PLUS a release of the wait on the unwind path.**

Pin the `cancel()`-paired form (agent-executed as "V4", controller-confirmed):

```go
defer func() {
    cancel()      // release the wait: a panic must never leave this blocked on a flusher
    <-flusherDone // wait for the Flusher goroutine to stop Submitting before closing the sink channels
    for _, s := range statsSinks {
        _ = s.Close()
    }
}()
```

- `cancel` is declared at `main.go:339` (`ctx, cancel := signal.NotifyContext(...)`), i.e. **in scope** at the relocation anchor. Confirmed by build.
- `context.CancelFunc` is **idempotent**, so the normal shutdown path — where `<-ctx.Done()` has already fired — is unaffected. Measured: SIGTERM-path behaviour is byte-identical between the baseline and the paired tree.
- ⚠️ **The `cancel()` must come BEFORE the receive.** Placed after, it is unreachable during the hang.

**Recorded alternative, NOT pinned:** a bounded `select { case <-flusherDone: case <-time.After(d): }`. It also works, but it converts a deadlock into a *silent truncation of the final sink drain* on a timer the SPEC would then have to justify, and it leaves the LIFO inversion in place rather than neutralising it. A second recorded alternative — registering an additional `defer cancel()` **after** the relocated defer, so LIFO runs it first — is behaviourally equivalent to the pinned form and is a matter of taste; the IMPL may take it if it reads better *in situ*, and must say so.

### 3.2 D-BPV-ORDER **[RESOLVED BY EXECUTION — the order DOES change, one pair matters, and it is the one that breaks]**

Defer roster in `main()` at this tip — **six own-frame defers**, two methods (grep and an AST walk of the `main` `FuncDecl`) reconciled exactly. The seventh `DeferStmt` lexically inside `main` (`defer close(flusherDone)`, `:368`) is registered on the **spawned goroutine's** frame and never participates in `main`'s unwind. Nineteen lines in `main()` contain the string "defer"; twelve are prose.

| reg. line | body | LIFO rank BEFORE | LIFO rank AFTER |
|---|---|---|---|
| `:186` | close access-log `sinks` | 6 | 6 |
| **`:298`** | **`<-flusherDone`; close `statsSinks`** | **5** | **1** |
| `:309` | `tracingProvider.CloseAll()` | 4 | 5 |
| `:337` | `admSrv.Close()` | 3 | 4 |
| `:340` | `cancel()` | 2 | 3 |
| `:345` | `lm.Stop()` | 1 | 2 |

Every changed pair involves the stats-sink close, which moves from rank 5 to rank 1. Per-pair disposition, each walked into the callee rather than stopped at the call site:

- **vs `cancel()` — CHANGED, MATTERS, HAZARDOUS.** §1.1. This is the pair the pinned fix neutralises.
- **vs `lm.Stop()` — CHANGED, benign.** `manager.go:1531-1539` takes `startedMu` and calls `rt.closeBind()` per runtime; no stats-sink interaction, no `Wait()`.
- **vs `admSrv.Close()` — CHANGED, benign.** `admin.go:113-121` calls `httpSrv.Close()`; admin reads the stats *registry*, never a sink.
- **vs `tracingProvider.CloseAll()` — CHANGED, benign.** Disjoint from `statssink`.
- **vs the access-log `sinks` close — UNCHANGED.** Stats sinks still close first.

**The sink contract (*no Submit after Close*) still holds**, and the argument is structural rather than merely observational: the **only** non-test caller of a `statssink.Sink.Submit` is `internal/statssink/flusher.go:49`, so no shutdown-path code other than the Flusher can violate it.

⚠️ **AND THE HARNESS THAT "PROVED" IT GREEN IS VACUOUS — this is a transferable finding.** A deliberately contract-violating build (the wait removed **and** a hold spliced in to widen the race) exits **0 with no panic, 3/3, under SIGTERM**. Reason: `signal.NotifyContext` cancels `ctx` at signal delivery, so `Flusher.Start` has already returned before any defer runs. **Under SIGTERM the contract is unfalsifiable.** The same build on the **panic** path fails loudly with `panic: send on closed channel` in `statssink.(*TCPStatsdSink).Submit`. ⇒ **any future validation of this contract must use the panic path; a SIGTERM arm is a green that proves nothing** (`reference_probe_must_discriminate`).

**Input measured, so the green is not an empty-input artifact:** the sink arms ran two live statsd sinks (UDP + the channel-based TCP sink, the tree's only stats-sink background mutator) at a 0.25 s flush interval and recorded **135 datagrams / 5775 bytes / 1 TCP connection / 135 lines**, identically on both trees.

### 3.3 D-BPV-WINDOW **[RESOLVED BY EXECUTION — span 68 lines; the entry-point list is EXTENDED; the reachability classification is REFUTED]**

**Boundaries.** Defer registered `main.go:298-303`, blocking at `:299`. `flusherDone` created `:205` (unconditionally). Closed **only** at `:368` (inside the flusher goroutine, when `Start` returns) or `:370` (the `else`). **Span: `:304-371`, 68 lines.**

**Three-point boundary proof, EXECUTED with the same panic at three positions on an otherwise-unmodified tree:**

| probe position | EXIT | stdout | stderr |
|---|---|---|---|
| before `:298` (defer not yet armed) | **2** | 0 B | 125 B, panic printed |
| inside the window | **124** | **0 B** | **0 B** |
| after `:371` | **2** | 0 B | 125 B, panic printed |

Two negative controls, three-way discrimination, one hang.

**Entry points — the BRAINSTORM's four are INCOMPLETE but not wrong.** The complete set of in-window calls that can execute arbitrary code: `boot.Construct` `:316` · `admin.New` `:333` · `admSrv.Start()` `:334` · `signal.NotifyContext` `:339` · `lm.Start` `:342` · `admSrv.MarkReady()` `:347` · `bs.Stats.Freeze()` `:356` · `cm.StartHealthChecks` `:358` · `cm.StartOutlierDetection` `:359` · `statssink.Flusher.Start` `:368`. ⚠️ **`:358`/`:359`/`:368` only SPAWN goroutines** — a panic in a spawned goroutine bypasses `main`'s stack and crashes the process **visibly**; it cannot be swallowed.

**Reachability of the five registry sites — R2.** One config-reachable (`:107`), four not.

**Panic-site denominator, stated as an AUDIT.** `internal/` non-test: **18** `panic(` sites in 8 files. Repo-wide: **522** (504 in `test/`, 0 in `cmd/`, 0 in `validate/`). Fourteen of the eighteen lie in the window's call graph — `boot.Construct` constructs and freezes all three filter registries in-window — and **exactly one is config-triggerable**. ⚠️ **This denominator is structurally incomplete and must be labelled as such wherever it is quoted: R4's runtime panics carry no `panic(` token.**

### 3.4 D-BPV-OTHER-DEFERS **[RESOLVED — NO SIBLING, and the null is load-bearing]**

Classification of all six own-frame defers:

| # | line | verdict | evidence |
|---|---|---|---|
| 1 | `:186` access-log `sinks` close | **BOUNDED-BY-5s-grace** (gRPC/OTLP arms); **BOUNDED-BY-file-write** (file arm) | `grpcsink.go:145-159`, `otlpsink.go:135-149` use `select { <-done; case <-time.After(grace): cancel(); <-done }`; `writer.go:91-98` has no grace — §12(1) |
| 2 | `:298` `<-flusherDone` | **UNBOUNDED** | the defect. The `statsSinks[].Close()` calls *after* the receive are all bounded by a 2 s grace |
| 3 | `:309` `tracingProvider.CloseAll()` | **BOUNDED-BY-5s-grace × N** | `exporter.go:376-386`, same pattern |
| 4 | `:337` `admSrv.Close()` | **TRIVIAL** | `admin.go:113-121`, immediate `Close` — R7 |
| 5 | `:340` `cancel()` | **TRIVIAL** | — |
| 6 | `:345` `lm.Stop()` | **TRIVIAL** | `manager.go:1531-1539` + `closeBind` `:203-216` — R7 |

**Repo-wide non-test AST sweep: 171 `DeferStmt`, exactly 2 of blocking shape** — `main.go:298` (the defect) and `extproc/processor.go:561` (a request-path mutex, neither boot- nor shutdown-class). ⚠️ **The sweep detects by SHAPE, not by CALLEE** — a `defer x.Close()` whose `Close` blocks internally is invisible to it, which is why every callee above was walked by hand.

**Hazard class.** At the known trigger's panic site only defers #1, #2, #3 are registered; unwind order #3 → #2 → #1, confirmed by an instrumented run. **Exactly one (a)-class boot-window hazard exists and it is the one this row targets.**

⚠️ **DETECTOR POSITIVE CONTROL — the null answer is not a blind probe's silence.** Injecting one extra `defer func() { <-make(chan struct{}) }()` before `boot.Construct` into the *fixed* build reproduced the hang instantly (`EXIT=124`, no panic text) across arms. **The audit could have found a sibling; there is none.**

⚠️ Even against a **blackholed** peer, every boot-window `Close()` completes in **microseconds** — at boot-panic time the sinks' channels are empty, so their writer goroutines exit on `close(ch)` without entering a flush and the grace timers never engage.

### 3.5 D-BPV-LOGFATAL **[RESOLVED BY EXECUTION — confirmed, and it bounds the guard]**

`log.Fatalf` calls `os.Exit`, which **skips defers entirely**. Confirmed three independent ways, all in-window and all *deeper* into the window than the panic:

| trigger | fires at | EXIT | wall | stderr |
|---|---|---|---|---|
| route naming a non-existent cluster (`boot.Construct` error) | `main.go:318` | **1** | prompt | 149 B, message printed |
| admin port already bound (`admSrv.Start` error) | `main.go:335` | **1** | **0.00 s** | 106 B |
| listener port already bound (`lm.Start` error) | `main.go:343` | **1** | **0.00 s** | 132 B |

⚠️ **The first of these is the strongest control in the row because it holds the CALL SITE FIXED.** The same `boot.Construct` call at the same line either returns an error (prints, exits 1) or panics (hangs, 0 bytes). **⇒ the hang is PANIC-UNWIND-specific — not window-specific, not `boot.Construct`-specific, and not config-shape-specific.**

**Which class takes which path, and what the guard can therefore catch:**

| failure class | path | in-window? | observed |
|---|---|---|---|
| bootstrap parse/semantic reject | `log.Fatalf` → `os.Exit(1)` | pre-window or `:318` | prompt exit 1, message on stderr |
| admin / listener bind failure | `log.Fatalf` → `os.Exit(1)` | **in-window** | prompt exit 1 |
| stats-registry duplicate registration | `panic` | **in-window** | **HANG, 124, 0 B** |
| same, under `--mode validate` | `panic`, defer never armed | n/a | exit 2, full trace |
| **any runtime panic (nil deref etc.) in-window** | `panic` | **in-window** | **HANG, 124, 0 B** |
| panic in a goroutine spawned at `:358`/`:359`/`:368` | `panic` | in-window | crashes **visibly** |

**Can catch and must:** any panic — explicit or runtime — unwinding through `main` inside the window. **Cannot catch and must not try:** the `log.Fatalf` paths, which already behave correctly; and goroutine panics, which already print.

### 3.6 D-BPV-GUARD-TRIGGER **[RESOLVED — the vacuity PREMISE is REFUTED; keep the trigger, strengthen the assertion]**

**The risk is real and documented in both directions.** `ROADMAP.md:140` (row 78's own cell) records that a reject is *"NOT delivered, deliberately"* because the reference boots green; `ROADMAP.md:139` (row 77) banks `Listener.stat_prefix` at 10–12 tasks precisely because *"the reference MERGES two listeners sharing a prefix … where envoy-go's registry PANICS."* `DECISIONS.md` describes the panic contract in four places but **contemplates changing it nowhere**; `BEHAVIOR_CONTRACT.md` has **zero** hits for the whole family of duplicate-registration phrasings.

**But the vacuity concern is dischargeable by assertion strength, and this was proven by simulating both futures:**

| simulated future row | how simulated | guard result |
|---|---|---|
| duplicate registration → **get-or-create** (reference parity) | the two `stat_prefix` values made unique | **RED** — hangs the deadline with the three ready sentinels on stdout |
| duplicate registration → **clean parse reject** | an invalid `stat_prefix` name | **RED** — three failures at once: exit 1 ≠ 2; stderr lacks the panic; no goroutine stack |

⇒ **with an assertion that names the panic text AND the exit code, the trigger is a red-on-change pin, not a vacuity hazard.**

**DECISION: keep duplicate-`stat_prefix` as the primary trigger** (two HCM chains, **distinct addresses**, identical `stat_prefix` — distinct addresses matter, so the listener-scope stats keyed on address do not also collide and the collision is exactly and only the HCM one). It is the **only** config-reachable trigger (R3, R8) — there is no fallback, which the SPEC states rather than hides.

**What pins the dependency — three mechanisms, none prose-only:**
1. The assertion names `stats: duplicate metric registration` **and** demands exit code exactly **2**.
2. A doc comment naming the panic site (`internal/stats/registry.go:107`) and the registration site (`internal/filter/hcm/config.go:352-358`), instructing *"re-point the trigger; do NOT relax the assertion."*
3. The structural companion (§3.7), which survives the trigger's removal entirely.

### 3.7 D-BPV-GUARD-COVERAGE **[NEW AT THIS SPEC — opened because §1.2 found the gap]**

Neither designed guard sees the moved hang. The IMPL ships a **third arm**, and it is structural because there is no config-reachable post-anchor panic to drive a behavioural one.

**Requirement.** Assert on `main.go`'s AST that the deferred `<-flusherDone` receive is **preceded, inside the same function literal, by a call to `cancel`** (or, if the IMPL takes the bounded-`select` alternative, that the receive occurs inside a `SelectStmt` with a second arm). The assertion must carry the same anti-vacuity discipline as the other structural arm: **`found == 0` is a `t.Fatalf`, never a silent pass.**

**Negative control the IMPL must observe:** delete the `cancel()` line and see this arm go RED while the other two stay GREEN. That is the pairing that proves it adds coverage rather than duplicating it.

⚠️ **State the residual honestly.** All three arms are blind to a *different* blocking defer introduced elsewhere in the window; the §3.4 detector positive control is the method that would find one, and it is a method, not a shipped test.

### 3.8 D-BPV-PROSE **[the comment audit — denominator stated]**

Seven candidate `defer`/LIFO comment sites in `main()` (one further match, `:93`, is an unrelated use of the word "deferred" and is excluded). **3 FALSE · 2 MISLEADING/INCOMPLETE · 2 SURVIVE.**

**FALSE — must be rewritten:**
- **`:289-297`** — the block that travels *with* the moved lines, and the most important correction in the row. Post-move it is false in **both** load-bearing clauses: `defer cancel()` is now registered **earlier**, so it runs **later**, not first; and *"cancel() fires ctx.Done()"* is no longer the mechanism that releases the wait. **The sink-contract conclusion still holds, but for a different reason, and the rewrite must say which.**
- **`:393`** — `// (LIFO: lm.Stop, admSrv.Close, sinks-close)`. The first element is now wrong. (Already incomplete pre-move — it omits tracing and the stats-sink defer.)
- **`:370`** — `// no flusher: unblock the sink-close defer immediately`. Post-move the defer is registered *after* this line; there is nothing yet to unblock.

**MISLEADING / INCOMPLETE:**
- **`:326-329`** — the numbered 1/2/3 defer list gains a fourth entry ahead of all three. (Already imprecise pre-move: item 3's *"shuts listeners after admin"* is backwards in **execution** order.)
- **`:198`** — *"closed via a dedicated defer"*; the pointer should be re-aimed ~70 lines down.

**SURVIVE — do NOT touch:** `:304-308` (R11) and `:112-113`.

⚠️ **Pre-existing drift found while enumerating, cheap to fix in the same pass because the IMPL is already editing this block:** `:200-202` says *"when **both** config slices are empty, statsFlusher stays nil"*, but the guard at `:206` now tests **five** slices.

---

## 4. Framework primitives — 0 new packages, 0 new modules, 0 new production deps

All edits land in **existing** files: `cmd/envoy-go/main.go` and `cmd/envoy-go/main_test.go`. `reference_new_subpackage_pulls_transitive_module` does not bite. The structural arms need `go/ast`, `go/parser`, `go/token` — **stdlib, test-side**; an import line is not a `go.mod` module. Precedent for the technique: `test/fixtures/0061-lb-ring-hash/driver/linkage_test.go`.

---

## 5. Identifier hygiene

New test identifiers must be collision-checked at the IMPL tip (`reference_spec_drafted_identifier_collision_check`). Drafted names: `TestMain_BootPanicIsVisible`, `TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose`, and a third for §3.7. `grep -c` each before adopting; `cmd/envoy-go` currently declares 8 test functions plus 4 helpers.

---

## 6. Stat surface **+0** (1207 → 1207) · Fuzz **+0** (55) · Fixtures **+0** (120)

No `NewCounter`/`NewGauge` call site moves — the row changes *when a defer runs*. The `+0` is **structural**: the production diff is confined to `main()`, which registers no stats. Assert the **DELTA**; the absolute 1207 is DOCUMENTARY and rides two unaudited ledger gaps.

No bootstrap field is consumed ⇒ no parse arm, no reject arm, no fuzz target.

**No differential fixture, and the obvious harness is the WRONG SHAPE** — `BootRejectFixture` requires **both** sides to exit non-zero, but the reference **boots green** on a duplicate-`stat_prefix` config. Using it would assert a divergence this row is not making. Next-free fixture index stays **0119**.

---

## 7. Behavior-contract edit map

**ONE new subsection**, placed after `## Bootstrap config validation (per phase 51 ADR-0268)` (`:860`) and before its `### Does not yet apply to` (`:874`) — or as its own `##` section immediately following. Content:

1. **A boot failure is never silent.** Every boot-path failure either exits non-zero with a diagnostic on stderr, or panics with Go's standard panic dump. Neither hangs.
2. **The two failure mechanisms and their exit codes** — `log.Fatalf`/`os.Exit(1)` with a one-line message (config, bind, construction errors), versus an unrecovered panic with exit status 2 and a goroutine dump.
3. **The entry-path symmetry**: `--mode validate` and normal boot report the same panic the same way. This is the property the row installs; before it, they diverged.
4. **The `### Does not yet apply to` bullets it gains** — the two §12 residuals.

⚠️ **No stat-ledger line** (the surface is unchanged), and **no whole-file grep count anywhere.**

⚠️ **`BEHAVIOR_CONTRACT.md:862` carries the non-existent public import path `github.com/esalaine/envoy-go/validate`** — live at this tip, inside the very section this row extends. It is a standing documented deferral and this row **does not fix it**; the IMPL must not silently inherit the wrong path into the new subsection.

---

## 8. Sentinel maintenance — **this row narrows NOTHING**

The three Operational-tooling deferred candidates are xDS-sourced dry-validation, an admin-API live-reload-and-validate endpoint, and an RTDS/SDS validate companion. **None is this row.** ⇒ **check (2) stays at its current site count through the IMPL.** Stating this rather than forecasting a decrease is deliberate: the measured record is that check (2) has never gone down across ~19 phases, and predicting a decrease repeats the phase-73 error. **`want` STAYS 110** — this row adds no ROADMAP row.

**Sentinel re-run MECHANICALLY by the controller at this stage's open, tip `e9fb2088`. It does NOT fire; `stop` was NOT created:**
- **(1)** `NOT DONE: row 78` — live since the BRAINSTORM, silenced only at the phase-78 IMPL. **No `GATE FAIL`** ⇒ the denominator 110 holds.
- **(2)** FIVE sites: `:188 :198 :208 :214 :222`.
- **(3)** `NEVER OPENED: gRPC` and `NEVER OPENED: WASM`.

⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this SPEC**, which removes the exposure but not the obligation: **re-run all three after any edit lands**, and never write a matcher string into the file the sentinel greps.

---

## 9. Test plan + task surface — **7–9 tasks; a SINGLE FLAT ROW**

⚠️ **The inherited 5–7 is REFUTED at the bottom (R10).** The BRAINSTORM itself labels it *"INFERRED, NOT EXECUTED … a costing, not a measurement — re-derive it at the adopting tip"*. **Calibration, which is the load-bearing evidence rather than intuition: phase 76 was also a small maintenance row, also anticipated ~5–7, and its PLAN shipped NINE tasks.** The project's own most recent 5–7 anticipation under-counted by two at this exact granularity.

| # | task | gate |
|---|---|---|
| T1 | The production change in `cmd/envoy-go/main.go` — relocation **+ the `cancel()` pairing** (§3.1) — and the rewrite of the `:289-297` comment block that travels with the moved lines | builds; `gofmt -l` **output** empty |
| T2 | The remaining prose sweep — §3.8's other two FALSE sites, two MISLEADING sites, and the pre-existing `:200-202` "both"→five drift. ⚠️ **`:304-308` and `:112-113` stay BYTE-UNTOUCHED (R11)** | grep-verified per site |
| T3 | The black-box guard + its trigger config (§3.6), assertions as a **conjunction**, hang detected via `ctx.Err() == context.DeadlineExceeded` **distinctly** from an exit code. ⚠️ **The repo has NO existing precedent for this** — zero `DeadlineExceeded` hits in `main_test.go` — so it is introduced here. ⚠️ Use `t.Errorf` per property, not `t.Fatalf` (`reference_fatalf_makes_assertions_unreachable`); leg 1 is the sole `t.Fatalf` because after a hang there is nothing left to assert | red-then-green |
| T4 | The structural arm (`<-flusherDone` after every `close`) **and the §3.7 coverage arm**, both with `found == 0` anti-vacuity fatals | red-then-green |
| T5 | The break roster — **at minimum seven arms**, each proven to fire its **own** assertion: revert-the-relocation · trigger-legislated-to-get-or-create · trigger-legislated-to-reject · **print-then-hang** (R5: proves the exit-status half load-bearing) · structural-revert · structural-rename (anti-vacuity) · **§3.7's delete-the-`cancel()` arm, which must go RED while the other two stay GREEN** | each break red, restore green |
| T6 | `BEHAVIOR_CONTRACT.md` boot-failure-visibility subsection (§7) | grep-verified |
| T7 | ADR-0300 §Decision + §Consequences appended **IN PLACE** after the retained italic footer; **no renumber, no `---` separator** | block shape 1/1/1/1 |
| T8 | Gates: `gofmt` + `golangci-lint` on `cmd/envoy-go`; `go test ./cmd/envoy-go/ -count=1`; **and the FULL 120-fixture differential suite** | see the blast-radius note below |
| T9 | `ROADMAP.md` row 78 `in-progress` → `done` + IMPL summary; `STATE.md` roll; six-gate (`BOOTSTRAP_PROMPT.md` §7.5; the SOLE leg per ADR-0106) | ADR-0106 sole-leg |

*(Floor 7 if T2 folds into T1 and T9 into T8; ceiling 9 as enumerated.)*

⚠️ **THE FULL DIFFERENTIAL SUITE IS A MANDATORY GATE, NOT AN OPTIONAL ONE.** `test/differential/harness.go:240` and `:594` build and spawn `./cmd/envoy-go` for **every** fixture, so a shutdown-ordering change to `main()` has a **120-fixture blast radius** — and the stats-sink fixtures (`0089-stats-sink-metrics-service` and the statsd / dogstatsd / graphite / OTLP siblings) are exactly the ones whose assertions depend on flush-before-close. **A green `cmd/envoy-go` package is not evidence about those fixtures.** ⚠️ Run with `-count=1` (`reference_differential_break_protocol_count1`) and `-v` (a full-suite log **without `-v` is VACUOUS** — `grep -c -- '--- PASS'` and `--- FAIL` both return 0 on a fully green run).

⚠️ **Known live hazards — never reflex-classify any of these as a regression.** The full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **before any assertion**, the latter as a **PANIC that can abort the whole binary**, firing more readily under `-race` and as the fixture count grows — now **120**) · `reference_sds_init_fetch_timeout_dial_budget_flake` · the pre-existing `internal/cluster` `-race` outlier flake · two still-**UNINDEXED** load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`). ⚠️ **A stage brief's flake list is not the index — the NINTH consecutive stage at which that has held.** Isolate-re-run, then state the classification **and its evidence**. ⚠️ **`0061-lb-ring-hash` is NOT a live flake; a spread failure there is a FINDING.**

⚠️ **Gate hygiene — the lineage's broken-gate count is THIRTEEN, unchanged at this docs-only stage.** `gofmt -l` **never exits non-zero** (gate on OUTPUT) · a full-suite recipe without `-v` is vacuous · a sha256 roster built from the tip's `ls-files` alone is desynced by construction against a DELETED file (span both trees) · `go doc -all <A> <B>` silently swallows arg2 (one package per invocation) · a `+0 exported symbols` gate over an EMPTY package goes red on a correct tree · a RANGE gate cannot detect anchor drift (use a content/symbol anchor) · a roster's naive `[ -f ] || continue` exits 0 on a deleted file · a count-only stat guard passes a build with BOTH names wrong. ⚠️ **A harness's exit code is not the command's** — capture the inner status and derive the tally from the log.

---

## 10. Edit-site roster — RE-DERIVED at tip `e9fb2088`

**Production (ONE file):**
1. `cmd/envoy-go/main.go` — the `:298-303` defer relocated past the `close(flusherDone)` branches **with the `cancel()` pairing**; the `:289-297` comment block rewritten; the §3.8 prose sites.

⚠️ **Blast radius is mechanically one file:** `flusherDone` occurs at **9 sites, all in `cmd/envoy-go/main.go`** — 4 code (`:205` declare, `:299` receive, `:368`/`:370` close) and 5 comment (`:290 :292 :294 :365 :366`). The tree's other 16 `package main` files are fixture backends and PKI generators; none carries the pattern.

**Test (ONE file):**
2. `cmd/envoy-go/main_test.go` — three new tests (§3.6, §3.7, the structural ordering arm) + the trigger config helper.

**Docs:** `DECISIONS.md` (ADR-0300) · `BEHAVIOR_CONTRACT.md` (§7) · `ROADMAP.md` (row 78 → `done`, **at the IMPL only**) · `STATE.md` · `next-prompt.txt`.

⚠️ **`internal/stats/**` stays BYTE-UNTOUCHED** — this row makes panics visible; it does not re-litigate which should exist (§2).

---

## 11. Deferred items — named so no later stage re-derives them

1. **A shutdown-class unbounded defer.** `AsyncFileSink.Close` (`internal/accesslog/writer.go:91-98`) is the **only** sink `Close` with **no grace timeout** — `close(s.ch); <-s.done`, and `run()` does a bare `s.f.Write`. On a FIFO or a hung filesystem with records pending, defer #1 is unbounded **at shutdown**. Out of scope: this row is boot-class.
2. ⚠️ **A SECOND SILENT ZERO-BYTE BOOT HANG THAT IS NOT A DEFER AT ALL.** `os.OpenFile` at `internal/accesslog/writer.go:56`, reached from `main.go:117`, blocks forever when an `access_log` `path` is a FIFO with no reader — **before any defer is registered**, so this row's fix cannot touch it. EXECUTED with a discriminating control: FIFO with no reader ⇒ `EXIT=124`, **0 bytes**; the byte-identical config with a reader attached ⇒ boots to `envoy-go ready`. ⇒ **"fix the `flusherDone` defer" does NOT exhaust "a boot failure is never silent"**, and §7's contract statement must be worded to say what it actually covers.
3. **The `/stats/prometheus` projection-completeness gap** (`ROADMAP.md:208`) — 30 names / 6 families / 28 pre-dating phase 77. Unblocked by this row.
4. **Whether duplicate `stat_prefix` should be rejected or aggregated** (§2.1), and the `Listener.stat_prefix` row at `ROADMAP.md:139` that would settle it.
5. **The documentary defects**, unchanged and still not fixed: the non-existent public import path (~20 lines across four docs, including an ADR that *decides* it); a mechanical stat-surface count (8–11 tasks); the unresolved half of the `BEHAVIOR_CONTRACT` stale-cite claim; `BEHAVIOR_CONTRACT.md:501`'s SN9 collision; `internal/stats/name.go:350`'s already-wrong error enumeration.

---

## 12. ADR continuity — the **ADR-0300 §Context** DRAFT

Drafted at this SPEC per `ADR-0044-as-used` (⚠️ **ADR-0044 itself does not contain the Context-draft discipline**). §Decision + §Consequences append **IN PLACE** at the phase-78 IMPL after the retained italic footer — **no renumber, and no `---` separator** (the convention was abandoned; ADR-0296 through ADR-0299 all carry none). DECISIONS tail flips ADR-0299 → **ADR-0300 PROPOSED** at this commit; next-free becomes **ADR-0301**.

⚠️ **ADR-0300 carries NO whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297, and escalated at the phase-76 BRAINSTORM from a wrong number to a flipped sentinel check. Every count is line-scoped or stated with no numeral.

⚠️ **`ADR-0089` stays BYTE-UNTOUCHED** — this row lands neither `/runtime` nor `POST /runtime_modify`.

The drafted text is appended to `DECISIONS.md` at this commit.

---

## 13. Exit — counts + expectations at SPEC-DONE

**Re-run MECHANICALLY by the controller in the stage worktree; never copied.** Docs-only close.

| axis | value at this close | command | phase-78 IMPL delta (anticipated) |
|---|---|---|---|
| differential fixtures | **120** (tail `0118-runtime-static-layer`) | `ls -d test/fixtures/[0-9]*/ \| wc -l` ⚠️ the faithful `discoverFixtures` predicate is `^[0-9]{4}[a-z]?-` | **+0** |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | **+0** |
| stat surface | **1207** | ⚠️ **NO mechanical command; DOCUMENTARY, two recorded ledger gaps** | **+0** — assert the DELTA |
| BackendKind | **tail 38** | ⚠️ a TAIL VALUE — 39 constants, `TCPEcho = 0`; do NOT "fix" to 39 | **+0** |
| go.mod modules | **2** (the phase-61.2 lineage figure) | ⚠️ NOT a repo total — the single `go.mod` requires 67; do NOT "fix" 2 to 67 | **+0** |
| internal packages | **73** | — | **+0** |
| `runner_test.go` blank imports | **120** | ⚠️ anchor on the FULL `^\t_ "github.com/pgdad/envoy-go/test/fixtures/` prefix — a naive `^\t_ ` gives 126 | **+0** |
| ROADMAP | **226 lines / 110 data rows** | `want=110` **STAYS** | **+0 rows** |
| BEHAVIOR_CONTRACT | **5750** | — | grows by §7 |
| DECISIONS | **17531** | — | grows by ADR-0300 |
| DECISIONS tail | **ADR-0299 COMPLETE** → **ADR-0300 PROPOSED** at this commit | `grep -oE '^## ADR-[0-9]+' … \| tail -1` | completes at the IMPL |
| next-free ADR | **ADR-0301** after this commit | `grep -c '^## ADR-0300'` ⇒ was **0** | — |
| next-free fixture index | **0119** | numeric tail `0118` | unchanged |
| production `.go` files | **0 touched at this SPEC** | — | **1** at the IMPL |

**SPEC commit file set** (the phase-76/77 precedent): `DECISIONS.md` (ADR-0300 §Context) + `STATE.md` + this `SPEC.md` + `next-prompt.txt`. **`ROADMAP.md` BYTE-UNTOUCHED; row 78 STAYS `in-progress`. `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED.**

---

## 14. Adversarial-pass record

**What refuted what.** Eleven claims were re-derived (R1–R11). The four that would have caused wrong work if carried:

1. **R1** — the IMPL would have shipped a tree that **still hangs**, with a narrower and sink-conditional window, under a commit message announcing the hang was fixed.
2. **§1.2** — and it would have shipped with **two purpose-built guards green** and a passing six-gate, because neither guard can see the moved hang. A partial fix wearing a full fix's commit message is precisely the risk D-BPV-OTHER-DEFERS was written to catch; it materialised in the *fix*, not in a sibling defer.
3. **R2/R3** — the SPEC would have recorded four unreachable panic sites as boot-reachable and implied a trigger inventory of five where there is **one**, leaving a future row to discover there is no fallback trigger at the moment it needs one.
4. **R5** — an exit-status-only or output-only assertion would each have passed on a defective tree, and each has an **executed** counter-example.

**⚠️ THE METHOD FINDING THAT MATTERS MORE THAN ANY RESULT: THREE INDEPENDENT INVESTIGATIONS AGREED, AND THEY WERE ALL TESTING THE SAME BLIND SPOT.** Two agents concluded the relocation was safe or sufficient; the controller's own first probe agreed with them. All three had exercised only **pre-anchor** panics or the **SIGTERM** path — the two conditions under which the moved hang cannot appear. The agreement was not corroboration; it was a shared input defect. It broke only when one agent varied the panic *position* and the controller varied the *deadline signal* and the *sink presence*. ⇒ `reference_probe_must_discriminate` is necessary but not sufficient: **each probe discriminated perfectly along the axis it varied, and every one of them held the decisive axis fixed.**

**A CONTROLLER SELF-CORRECTION, recorded rather than quietly amended.** My own post-anchor probe returned `EXIT=2` with the panic printed and appeared to refute the agent who was right. **Its config had no stats sink**, which selects the `else` branch that closes `flusherDone` *before* the defer is registered — a shape in which the defect is structurally impossible. **A probe's INPUT is a claim** (`reference_probe_input_is_a_claim`): the arm was not a negative result, it was a control that could not fail.

**A SECOND CONTROLLER SELF-CORRECTION.** My first three-arm reproduction used `clusters: []`; every arm died at `cluster: zero clusters in bootstrap`, and the arm meant to hang printed a message and exited — which reads as a clean refutation of the whole row. Caught only because the probe reported input byte counts beside its outputs. **A zero-match result and a broken arm are indistinguishable unless you measure the input.**

**A THIRD CONTROLLER SELF-CORRECTION.** I first wrote that `--mode validate` prints *"because it returns via `os.Exit` before the defer at `:298` is ever registered."* True, but not the mechanism, and stated alone it implies the validate path runs no defers. It does: `validate/validate.go:47` registers a tracing-close defer immediately before the `boot.Construct` call that panics, and that defer **runs during the unwind** without blocking. **Both entry paths run deferred calls while the panic unwinds; one blocks and one does not.** The hazard is a *blocking* defer, not defers as such — which is also why the pinned fix is about releasing the wait rather than about removing the defer.

**A REFUTED HYPOTHESIS THIS SPEC'S OWN DISPATCH BRIEF INTRODUCED.** The brief told an agent that `admSrv.Close()` likely blocks via `http.Server.Shutdown` and that `lm.Stop()` likely waits on WaitGroups. **Both are false** (R7), and an agent that had deferred to the brief would have manufactured two hazards. A brief's hypothesis is not evidence either.

**A CITE THE LINEAGE HAS BEEN INHERITING WRONG** — R9, found by reading ADR-0106 rather than citing it.

⚠️ **The Bash cwd reset fired AGAIN — the THIRTEENTH consecutive session.** Observed live (`Shell cwd was reset to /home/esa/git/envoy-go` after a `cd` into a throwaway worktree). Every git command in this session used `git -C <abs-worktree-path>`.

⚠️ **The machine-global-namespace hazard held.** Four agents ran concurrently with private scratch directories and private throwaway worktrees; ports were banded per agent (21000/21100/21200/21300, controller 22000) and no agent tore anything down by image or ancestor filter. No collision occurred.
