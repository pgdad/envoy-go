# Phase 100 — `listener-filters-timeout-enforce` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Stage:** PLAN (lifecycle **2 -> 3**). Worktree `/home/esa/git/envoy-go-wt-p100plan` off master `b6aa3ab3`,
branch `wt-phase-100-plan`.
**Spec:** `docs/envoy-go/phases/100-listener-filters-timeout-enforce/SPEC.md` (853 lines). The plan argues from
the spec, and executors read both. **§0 records sixteen findings against the SPEC, eleven of them outright
refutations or amendments of a SPEC row. Where the two disagree, this PLAN wins, and §0 says why.**
**Evidence only:** `BRAINSTORM.md` (514 lines). Where it disagrees with `SPEC.md` §0, the SPEC governs; where
the SPEC disagrees with this PLAN's §0, this PLAN governs.

**Goal.** Enforce `listener_filters_timeout` on TCP listeners that have listener filters. At the deadline, a
connection whose pipeline has not finished is CLOSED under `continue_on_listener_filters_timeout: false` and
handed to chain selection under `true`. Today the pipeline waits until the client acts. Each timeout books
`listener.<addr>.downstream_pre_cx_timeout` (**+1 NAME**). An explicit `0s` DISABLES the timeout, while an
absent field keeps the 15 s default. This is what the pinned reference does.

**Architecture.** The SPEC's shape **PB1** is kept unchanged: ONE clock inside `Pipeline.Run`. A
`context.AfterFunc` on the pipeline's own `context.WithTimeout` sets the peeker's read deadline into the past.
So a blocked `Peek` is interrupted only AFTER `ctx` is done, and the existing post-`Inspect` `ctx.Err()`
check fires by construction. The callback is stopped or waited for, and the deadline is cleared, before `Run`
returns. `serveConnection` books the counter on `errors.Is(err, context.DeadlineExceeded)`, and
`parseListenerFiltersTimeout` splits nil (15000) from explicit zero (0, disabled). Everything else in the plan makes that
falsifiable: 8 new unit tests plus one strengthened in place, the help-text pair, the five-listener
differential fixture `0125`, eight NC mutants scored per arm, a layout gate shown to fire, and the
occurrence-set reconciliation. **Every code block in the appendices was built, run and reverted at this
PLAN stage (§1.2, §4); none of it is a sketch.**

**Tech stack.** Go; `go-control-plane` v3 protos; `internal/listener` and its `listenerfilter` /
`tls_inspector` packages; `internal/stats`; the differential harness against
`envoyproxy/envoy:contrib-v1.37.2` by digest.

---

## Global Constraints

These apply implicitly to every task.

- **Reference pin:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`). **Run it BY DIGEST**, after verifying `docs/envoy-go/ENVOY_TARGET.md` lines 3-4.
- **`grep` is a `ugrep` shell function in EVERY shell here.** It honours `.gitignore`, and `next-prompt.txt`
  is gitignored yet tracked. Use `/usr/bin/grep`, `git grep`, or a direct path, and name the binary.
- **Use `git -C <abs-worktree>` for every git command.** The Bash tool's cwd silently resets, and shell
  variables do not survive between tool calls. Put `pwd` + `git rev-parse --abbrev-ref HEAD` before any
  commit.
- **`-count=1` is not optional on any `go test`.** Use `-v` whenever you count; `RUN=0` beside `RC=0` is a
  vacuous green. Take rc from `PIPESTATUS[0]` or `out=$(…); rc=$?`, never from the end of a pipe. The FAIL
  matcher is `^(FAIL|--- FAIL)|^ *--- FAIL`, and the panic gate is `^panic:|DATA RACE|SIGSEGV`. A `-run`
  selector that matches nothing prints `[no tests to run]` and EXITS 0.
- **Lint:** `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`. **misspell runs in US locale**, so no British
  spellings go into `.go` comments. Check `gofmt -l` by its OUTPUT; it never exits non-zero.
- **The appendices live in THIS file as fenced blocks — EXTRACT them, never re-derive them.** The SPEC-era
  scratch patches are gone; these are the artifacts that were built and run. Extract one by its heading:
  ```bash
  ext () { awk -v want="$1" '$0 ~ "^## Appendix " want " —" {f=1; next} f && /^```/ {c++; if (c==1) {o=1; next} if (c==2) exit} o' "$2"; }
  ext A docs/envoy-go/phases/100-listener-filters-timeout-enforce/PLAN.md > $S/patch-PB1.diff
  ```
  **Verified at the PLAN close:** Appendix A applies to `master` with `git apply --check`, B applies on A,
  and H.1 applies on A+B; C.1 extracts to 338 lines and F.1 to 727. **Always `git apply --check` first**, and
  for a `.go` appendix compare `wc -l` against §1.2's figure before trusting the extraction.
- **Cost figures come from `git diff --numstat`, never `--stat`.**
- **Build with `-o <scratch>`.** A bare `go build ./cmd/envoy-go/` drops a binary in the worktree.
- **Ports:** the ephemeral range is `32768-60999`. The harness reserves `20000..31007` and `11000..14999`.
  **`0125`'s reference ports `15125 15228 15229 15230 15231` were censused at this tip** (§2.4). Re-census
  them at the IMPL tip.
- **Never tear down a container this session did not create**, and then only BY NAME. A `reaper_*` container
  belongs to the differential itself.
- **The row registers ONE stat name**, `listener.<addr>.downstream_pre_cx_timeout`. Its ledger entry is
  **delta-only (`+1`), quoting NO absolute** (`SPEC.md` §9, decided). **Land the help-text pair (`helpText` +
  `helpTextRoster`) in ONE commit.** Either half alone reddens `TestHelpText_KeySetExact`, and the roster half
  alone also reddens `TestHelpText_NoSelfEqualHelp` (§0.6).
- **`pipeline.go` layout is GATED (§6):** `:33-37` and `:43` stay byte-identical, no new import, exactly one
  CODE `context.WithTimeout` under `internal/listener/` non-test, `tls_inspector.go` comment-only at `3 3`, and
  no line-count-changing hunk at or above `pipeline.go:44`.
- **Do NOT lift the `[1s, 60s]` envelope, move `downstream_cx_total`, pin a close KIND, add a partial-byte
  `true` arm, or add the `downstream_listener_filter_*` names.** Each is measured, named, and banked
  (`SPEC.md` §14.3).

---

## 0. What this PLAN refuted, by execution

Two measurement agents built every code block in throwaway worktrees. The **subject** agent (Docker-free)
built U1-U6, the NC roster at unit level, the comment edits and the layout gate. The **fixture** agent (the
only Docker user) built fixture `0125` and ran it nine times. The controller re-ran the load-bearing results
first-hand in two more throwaway worktrees: the tip's RED set, the final tree's green suite and `-race`, the
layout gate plus one planted control, and NC6's old-versus-new U2. **Every worktree was created detached and
removed; the only one left is the stage worktree.**

### 🔴 0.1 — `SPEC.md` §4.1 (e)'s `WithTimeout` gate CANNOT PASS AS WRITTEN

The SPEC says *"`git grep -n 'context.WithTimeout' -- internal/listener/ ':!*_test.go'` reads exactly the one
`pipeline.go:43` hit before and after."* It reads **THREE** hits at the tip: `pipeline.go:21` (the `Run` doc
comment), `pipeline.go:43`, and `manager.go:476` (prose citing `:43`). Under the final patch it still reads
three, with the `manager.go` hit moved to `:478`. **The gate in §6 drops hits whose text begins with `//`**.
It then reads exactly `pipeline.go:43` at the tip and under the fix. A planted second CODE `WithTimeout`
fires it.

### 🔴 0.2 — `SPEC.md` §11 NC4 → U4(ii) IS STRUCTURALLY BLIND

In U4(ii) the peer sends at 20 ms, so `Run` returns before the deadline, `stop()` returns true, and the
callback never sets a deadline. Deleting the clear therefore has nothing to un-clear. **U4(ii) is GREEN under
NC4, as measured.** Two new arms catch NC4:

- **U4(iv)**, `TestPipelineRunDeadlineClearedAfterTimeout`: the deadline FIRES, then the peer sends, and a
  `Read` must succeed.
- **U5's silent-liveness half**: the fall-through connection must still be open after being served.

U1 cannot catch NC4, because the `false` branch closes the connection anyway. U4(ii) is kept as a
liveness pin and labelled that way (method note 7f).

### 🔴 0.3 — `SPEC.md` §11 NC8 HAS NO UNIT CATCHER

U5 runs under `continue=true`, and U1 reads no stat, so both stay GREEN under NC8 (booking only under
`true`). **A new arm, U5f `TestListenerFilterTimeoutPreCxTimeoutOnAbort`**, runs with `continue=false`, needs
the counter at 1, and is RED under NC8. It is also RED under NC2 and NC6.

### ⚠️ 0.4 — `SPEC.md` §11 NC1's rows are INCOMPLETE on both surfaces

- **Unit:** U5's cancel half is RED **4/4, deterministically**, under NC1. With no clock inside the
  pipeline, nothing reacts to the manager-`ctx` cancel, and the server acts only at the 1 s raw deadline. U1
  is RED 4/4 but probabilistically (5-8 of 20 fall through each run). U4(i) and U4(iv) are RED
  deterministically. **U5's silent-value half was GREEN 4/4 under NC1**: one connection is a coin flip that
  NC1 happened to win every time (`SPEC.md` §3.2's warning, made concrete).
- **Fixture:** NC1 reddens **F1 (2/2), F2 (2/2), S `l_false` (17, 24) and CompareBytes**. It does not redden
  F2 alone, as §11 says. S `l_true`/`l_true_tls` redden on only one of two runs, so they are NOT a
  deterministic NC1 row.

### ⚠️ 0.5 — `SPEC.md` §0.10's replacement for the vacuous zero-timeout test is ITSELF blind

The SPEC says U4(iii) is *"an arm a 15 s budget would fail."* It holds for 400 ms, which a 15 s budget also
survives. `Pipeline.Run` has no zero → 15 s mapping at all; that mapping lives in
`parseListenerFiltersTimeout`. **U3 is the fold's discriminator. U4(iii) is GREEN in every NC column** and
is kept only as a `Run`-contract liveness pin, labelled so. The fixture's Z1 is the end-to-end discriminator
(RED under NC3, §0.11).

### ⚠️ 0.6 — `SPEC.md` §9's "guards the new name touches: `KeySetExact` … nothing else" MISSES ONE

The roster half alone reddens `TestHelpText_KeySetExact` **and `TestHelpText_NoSelfEqualHelp`**. The latter
renders `# HELP envoy_listener_downstream_pre_cx_timeout envoy_listener_downstream_pre_cx_timeout`. The
`name.go` half alone reddens `KeySetExact` only, and both halves together are green. `TestHelpText_Coverage`
(the `name_test.go` list) is a one-directional hand-written subset and needs no entry. **The one-commit rule
stands; its reason is now two guards.**

### ⚠️ 0.7 — "beside `envoy_listener_downstream_cx_total`" costs `12 11` in `name.go`, not `1 0`

gofmt realigns the whole 11-entry block around the longer key. A paragraph of its own, on the phase-75
`ssl_no_certificate` precedent, costs **`2 0`** (Appendix E). **The PLAN takes the paragraph.**

### ⚠️ 0.8 — `SPEC.md` §4.3's shutdown row ("falls through under `true`") is NOT OBSERVABLE as a fall-through

Under PB1 with `continue=true`, a manager-`ctx` cancel at 300 ms makes the client read **EOF at 300 ms** (4/4).
The client is never served. Whether the pipeline fell through and chain dispatch then aborted on the dead
context cannot be attributed (no log line distinguishes them). **Only the counter half is pinned (the
counter does not move), and NC7 reddens it.** The row in the IMPL's ADR-0322 §Consequences must say *"ends
at the cancel, books nothing"*, not *"falls through"*.

### ⚠️ 0.9 — `SPEC.md` §6.1 OMITS TWO COMMENT SITES, and QUIC listeners carry the name too

`manager.go:179-184` (struct doc: *"Inc/Dec'd from the accept-loop hot path. The two cx metrics are
registered for EVERY listener"*) and `manager.go:376` (*"The two cx metrics are unconditional."*) stay
literally true, but after PB1 a THIRD counter is unconditional, and it is `Inc`'d in `serveConnection`,
not the accept loop. **Both are edited, line-count-neutral, in Appendix B (built and gated).** Separately,
`quic.go:66` calls `registerListenerMetrics`, so **a QUIC listener registers the name at value 0 forever**.
Whether the reference registers `downstream_pre_cx_timeout` on a QUIC listener was **NOT measured**. That is
banked (§9), not pinned, and ADR-0322 §Consequences names it.

### 🔴 0.10 — `SPEC.md` §7.6's fixture floor "≥ ~1360" IS REFUTED: the complete fixture is **+836**

`driver.go` 727, `README.md` 79, `expectations.yaml` 30, plus 1 line in `runner_test.go`. The fixture carries
every chartered arm, the 50-connection arm and the timing classifier. `0123`'s 1361 measures how densely
that fixture is documented, not how many arms it has. It cannot be a floor for a different fixture
(`reference_banked_candidate_costs_rot_in_every_field`: re-derive by SHAPE).

### ⚠️ 0.11 — `SPEC.md` §11 NC3 reddens THREE surfaces, not "Z1 only"

Under NC3 the fixture reads **Z1 RED** (server FIN at 15012 ms, inside the 16.5 s hold), **S `l_zero` RED** (1
≠ 0; the 15 s drop books the counter before the scrape) and **CompareBytes RED**. In the unit layer U3 is
RED.

### ⚠️ 0.12 — `SPEC.md` §7.4's "docker-proxy converts the reference's RST to FIN anyway" is NOT EXERCISED

The reference closes a SILENT drop with FIN already. Across 9 runs, all **459** reference closes observed
through the harness's host `-p` path were FIN. No `0125` arm makes the reference send RST, because the RST case is a
partial-byte arm the fixture correctly omits. **The clause is unmeasured here, not confirmed.** "Pin no close
kind" still stands, on `SPEC.md` §0.1's in-network measurement alone.

### ✅ 0.13 — THE KEY UNMEASURED ITEM, ANSWERED: docker-proxy does NOT smear the reference's 50 concurrent drops

`SPEC.md` §7.5 owed F2 on the reference through the harness's `-p` path. The fixture agent measured **459
closes (9 runs × 51) at 1000-1004 ms, mean 1002.00, σ 0.92**, against the SPEC's in-network 999-1003. The
subject under PB1 and the non-racy NCs measured **347 closes at 1000-1021 ms, mean 1001.50, σ 4.22**. **The
`[700, 1800]` window is KEPT**, with margins of 328σ / 867σ on the reference and 71σ / 189σ on the subject
(low / high). The worst subject close leaves 779 ms of headroom. The low edge still excludes a mis-parsed
`0.5s` deadline (reference N7: 502 ms). **Caveat: measured with `0125` running alone; full-suite load is the
IMPL's gate (a) to observe.**

### ⚠️ 0.14 — METHOD NOTE 94 APPLIED TO `manager.go`: its live line-citations are ALREADY DRIFTED, so it gets NO layout gate

The SPEC gates `pipeline.go`'s layout because `:33-37` and `:43` are cited correctly. PB1 also shifts
`manager.go` lines, by +1 to +8 below `:185`. So the controller sampled the ten `internal/listener/manager.go:<N>`
citations under `internal/`, `test/` and `BEHAVIOR_CONTRACT.md`: `:825`, `:683`, `:1393`, `:409-410`, `:837`,
`:715`, `:658`, `:675`, `:765` and `:1073-1077`. **None points at its named symbol at the tip.** For example,
`0121`'s `manager.go:1393` names `rt.sslHandshake.Inc()`, which is at `:1410`, and `:1393` is
`rt.sslFailVerifyError.Inc()`. `anyTLS && anyPlaintext` is at `:705`, not `:683`, and `catchAllCount > 1`
is at `:724`, not `:715`. **PB1's shift therefore mints no new rot class. The drift is pre-existing, banked
(§9), and not repaired here.**

### ⚠️ 0.15 — `SPEC.md` §7.3 Z1's "hold 17 s" is a 16.5 s read deadline; the address labels are CONFIRMED and DIFFER

The driver holds exactly 16.5 s. NC3's close at 15012 ms falls 1488 ms inside that. On `/stats/prometheus` the
reference labels `envoy_listener_address="0.0.0.0_15125"` (dots KEPT), and the subject labels
`127_0_0_1_<port>` (dots FOLDED). **A driver that derived one side's label from the other's convention would
read ABSENT on every S row.** Appendix F looks each side up by its own label.

### ✅ 0.16 — CONFIRMED, not refuted

- PB1 is `pipeline.go 15 0`, `manager.go 14 6`, as the SPEC says. It was rebuilt from the SPEC's §4.1 code
  blocks and the layout holds.
- Every tip-RED arm the SPEC named is RED at the tip: U1 (`open:20`), U3 (15000), U4(i) (3.0 s), U5 (name
  absent), and fixture F1, F2 and S.
- NC5 is BLIND: the full suite ×5 read 470 RUN, 0 FAIL, and `-race` stayed clean.
- NC6's vacuity is real. The OLD U2 is GREEN under NC6: its read returns at **3.00 s on the client's own
  deadline**. The NEW U2 is RED. The controller re-ran this pair twice with the mutant's marker line
  asserted present.
  ⚠️ The controller's first attempt passed at 1.00 s because the mutant was **not** in place: a
  reverse-then-forward patch sequence left `manager.go` un-mutated. **Assert the mutant's marker BEFORE
  reading an NC result.**

---

## 1. Stage scope, MEASURED

### 1.1 What THIS PLAN commit touches — FOUR files

Precedents, by `git show --numstat --format=`: phase-99 PLAN `a62f32a0` and phase-98 PLAN `b4c5e97a`. Each
touches `STATE.md`, `STATE_HISTORY.md` `2 0`, the new `PLAN.md` and `next-prompt.txt`, and nothing else.
**No `DECISIONS.md`**, so the `PROPOSED` guard stays ARMED at `## ADR-0322`. **No `ROADMAP.md`**, so
`want` stays **132** and the file stays **250** lines.

### 1.2 What the IMPL will touch

| path | measured basis | added / removed |
|---|---|---|
| `internal/listener/listenerfilter/pipeline.go` | Appendix A (code, `15 0`) + Appendix B (comment, `2 2`) | **17 / 2** |
| `internal/listener/manager.go` | Appendix A (code, `14 6`) + Appendix B (comments, `11 10`) | **25 / 16** |
| `internal/listener/listenerfilter/tls_inspector/tls_inspector.go` | Appendix B, comment-only | **3 / 3** |
| `internal/stats/name.go` | Appendix E, its own paragraph | **2 / 0** |
| `internal/stats/helptext_test.go` | Appendix E | **1 / 0** |
| `internal/listener/listener_filters_timeout_test.go` (new) | Appendix C.1: U1, U3, U5, U5f | **338 / 0** |
| `internal/listener/listenerfilter/pipeline_deadline_test.go` (new) | Appendix C.2: U4 (i) (ii) (iii) (iv) | **209 / 0** |
| `internal/listener/manager_test.go` | Appendix D: U2 strengthened in place | **9 / 3** |
| `test/fixtures/0125-listener-filters-timeout/**` (3 files) | Appendix F, built and run 9× against the reference | **836 / 0** |
| `test/differential/runner_test.go` | the blank import | **1 / 0** |
| `docs/envoy-go/DECISIONS.md` | ADR-0322 §Decision + §Consequences; precedent phase 99 `a62f32a0`'s IMPL | ~**+100 / 0** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | `:4359` rewrite + the `+1` ledger entry after `:5148` | ~**+3 / −1** |
| `REVIEW_FINDINGS.md` | the timeout clause annotated at `:185-188` | ~**+2 / −1** |
| `docs/envoy-go/ROADMAP.md` | row 100 flip | **1 / 1** |
| `PROGRESS.md`, `STATE*.md`, `next-prompt.txt` | the close | not LoC |

**Byte-untouched roster.** The IMPL asserts each of these is EMPTY under `git diff master --numstat`:

- `internal/listener/quic.go`
- `internal/listener/listenerfilter/callbacks.go`, `types.go`, `chainmatch.go`, `doc.go`
- `internal/listener/listenerfilter/tls_inspector/` except `tls_inspector.go`
- `internal/stats/` except `name.go` and `helptext_test.go`
- every fixture directory except `0125`
- `go.mod`, `go.sum`, `.github/**`

⚠️ **The byte-untouched roster and the edit roster are not a partition** (method note 62). `STATE*.md`,
`PROGRESS.md` and `next-prompt.txt` are on neither list, and are named here for that reason. Set-difference
(`SPEC.md` §6.3): neither roster contains ADR-0082's or ADR-0296's text; both are superseded or noted, never edited. Neither contains
`ROADMAP.md`'s six sentinel windows, which are byte-identical by md5 (§2.1).

### 1.3 The split gate — EVALUATED WITH A COMMAND, NOT SPLIT

`BOOTSTRAP_PROMPT.md` §6.1 sets the thresholds at **~25 numbered tasks** or **~1500 LoC**. The task count and
the per-task sub-step maximum are measured in §10 by command.

| accounting | this row (MEASURED at this PLAN) | phase 98 IMPL MEASURED (`884e4b7c`) |
|---|---|---|
| production `.go` (code + comments + help text) | **17 + 25 + 3 + 2 = 47** added | — |
| + unit tests | **+ 338 + 209 + 9 + 1 = 557** → **604** | — |
| + fixture `driver.go` + `runner_test.go` (`.go` only) | **+ 727 + 1** → **1332** | **1218** |
| + fixture `README.md` / `expectations.yaml` | **+ 79 + 30** → **1441** | **1846** |
| + docs (ADR ~100, contract ~3, findings ~2, row 1) | **≈ 1547** | — |

**DECISION: DO NOT SPLIT.** The row is under ~1500 on the widest accounting that excludes docs, transcript
and router, the same accounting the phase-99 PLAN used. It reaches ~1547 only with docs, which are prose, not
"lines of code". The task count is well under ~25 (§10). The only clean seam is {unit layer} / {fixture}, and
splitting there would ship the production edit gated by only one of its two surfaces. Only the fixture can see
the reference, and only the unit layer can see NC4, NC7 and the cancel path. **This estimate is MEASURED, not a floor, for
every `.go` row. The doc rows are estimates.** If contact with reality pushes a task past ~10 sub-steps,
§6.1's **mid-execution** trigger applies then.

---

## 2. Sentinel and baselines — RUN AT THIS STAGE's TIP

### 2.1 The three checks, the four NCs, and the check-(2) positive control

Run at the worktree tip before any edit, with `/usr/bin/grep`, verbatim from `next-prompt.txt`, and **re-run
in the publishing tree with identical output** (a PLAN touches no `ROADMAP.md` byte):

- (1) **ONE**: `NOT DONE: row 100`.
- (2) **SIX** at `:210 :216 :222 :232 :238 :246`.
- (3) **SILENT**.
- NC-A: the substitution was inspected first (`NC LANDED? [ in-progress ]`), then **TWO** lines (`NOT DONE:
  row 62`, `NOT DONE: row 100`).
- NC-B at `want=131`: **TWO** lines (`NOT DONE: row 100`, `GATE FAIL: examined 132 data rows, expected 131`).
- NC-C: **FIRED** (residual 0).
- NC-D: **96 / 68** under `--`.
- Check-(2) positive control: **6 substitutions ASSERTED, residual 0**.
- Escape-aware malformed set: **{57, 69}**, at `:119` (NF 9) and `:131` (NF 10).
- Row 100 NF: **8** under BOTH forms.
- Per-line md5 with the **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex):
  `210 10d7807bf02d` · `216 4a92f7e62fc6` · `222 2a7eb298b9fd` · `232 242e53c6f7a3` · `238 b2680e6f4fbf` ·
  `246 6caa1c3ce0e7`.

⇒ **The sentinel does NOT fire, and `stop` was NOT created.**

### 2.2 The `PROPOSED` guard — ARMED, and this stage leaves it ARMED

`^> \*\*STATUS: PROPOSED` hits `:19491`. A backward `^## ADR-` search resolves it to **`## ADR-0322`**
(`:19489`). The ADR-0231 decoy (`^\*\*Status:\*\* PROPOSED`) still hits `:14866` and resolves to
`## ADR-0231` (`:14864`). The line's md5 is `929719b67c87…`, identical to `515856da`'s. `DECISIONS.md` reads
**19509** lines, `^---$` **216**, `^## ADR-` **321**, bare `^## ` **329**, tail `ADR-0322`, next-free
**ADR-0323** (`grep -c '^## ADR-0323'` ⇒ 0), all UNMOVED by this stage. **The IMPL disarms the guard at
Task 13.**

### 2.3 The fixture set

The blank-import extractor reads **126 = 126** at this tip, with both `comm` directions empty. With `0125`
registered it read **127 = 127** on the prototype, with `comm -23` and `comm -13` both EMPTY under the
**(imports, dirs)** argument order.

### 2.4 Ports

`git grep -lw -- <port> -- test/ internal/ cmd/` at `b6aa3ab3`:

- `15125` hits only `0123/README.md` and `0123/driver/driver.go`, the prose reserving it.
- `15228`, `15229`, `15230` and `15231` hit **zero** files.
- `ss -tan` / `ss -uan` show no socket on any of the five.

The ad-hoc band this stage reserved, `16600-16699`, was censused with `git grep -In '166[0-9][0-9]' -- test/
internal/ cmd/`, which read **zero** hits. **It was not used**: the fixture agent's runs went through the harness's own
allocation. ⚠️ **This paragraph spells the band, so it is a hit in the next census.**

---

## 3. STABLE ANCHORS — use these, never line numbers

- **A1** `func (p *Pipeline) Run(` (pipeline.go); the insertion goes directly after the `		defer cancel()`
  line inside `if timeoutMs > 0 {`.
- **A2** `func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error)` (manager.go).
- **A3** `func registerListenerMetrics(r *stats.Registry, rt *listenerRuntime)` (manager.go).
- **A4** `	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {` (manager.go,
  `serveConnection` step (4)).
- **A5** `	downstreamCxTotal   *stats.Counter` (the `listenerRuntime` metric block).
- **A6** `	// Peek yields only net/io errors (io.EOF, net.ErrClosed,` (tls_inspector.go).
- **A7** `func TestUnifiedDispatchListenerFilterTimeoutAbortsConnection(t *testing.T)` (manager_test.go).
- **A8** `var helpTextRoster = []helpTextRosterEntry{` (helptext_test.go);
  `	"envoy_server_accesslog_dropped":` (name.go, the line the new paragraph follows).
- **A9** `- Per-pipeline timeout (`Listener.listener_filters_timeout`)` and `**Phase 99 — +0, UNCHANGED` in
  `BEHAVIOR_CONTRACT.md`.
- **A10** `*§Decision and §Consequences follow at the phase-100 IMPL.*` in `DECISIONS.md`.
- **A11** `- **listener**: `listener_filters_timeout` never enforced (silent client` in `REVIEW_FINDINGS.md`.
- **A12** the row whose first field is `100` in `ROADMAP.md`.

---

## 4. Test design — MEASURED per arm, not predicted

Scored at this PLAN stage in throwaway worktrees. Each NC is applied on top of PB1, the tests and the
`name.go` half. R = RED, G = GREEN.

### 4.1 Unit arms (`go test -count=1 -v`)

| arm | TIP | PB1 | NC1 | NC2 | NC3 | NC4 | NC5 | NC6 | NC7 | NC8 |
|---|---|---|---|---|---|---|---|---|---|---|
| U1 `…RealTLSInspectorDropsSilentClients` | **R** `open:20` (4/4) | G 1.00 s | **R** 4/4 (5-8 of 20 fell through) | G | G | G | G | **R** `open:20` | G | G |
| U2-OLD (tip text) | G | G | G | G | G | G | G | **G (3.00 s — VACUOUS)** | G | G |
| U2-NEW (Appendix D) | G | G | G | G | G | G | G | **R** | G | G |
| U3 `…ZeroDisables` | **R** (15000) | G | G | G | **R** | G | G | G | G | G |
| U4(i) `…InterruptsSilentPeek` | **R** (3.0 s) | G 0.20 s | **R** | G | G | G | G | G | G | G |
| U4(ii) `…ClearedOnSuccess` — liveness only | G | G | G | G | G | **G** (§0.2) | G | G | G | G |
| U4(iv) `…ClearedAfterTimeout` — NEW | **R** | G | **R** | G | G | **R** | G | G | G | G |
| U4(iii) `…ZeroTimeoutHoldsSilentPeek` — liveness only | G | G | G | G | G | G | G | G | G | G |
| U5 name exists | **R** (absent) | G | G | G | G | G | G | G | G | G |
| U5 immediate (books 0) | not reached | G | G | G | G | G | G | G | G | G |
| U5 silent, value (books 1) | not reached | G | G 4/4 | **R** | G | G | G | G | G | G |
| U5 silent, liveness | not reached | G | G | G | G | **R** (EOF) | G | G | G | G |
| U5 cancel (books 0, acts < 900 ms) | not reached | G (EOF at 300 ms) | **R** 4/4 | R* | G | G | G | G | **R** (2) | G |
| U5f `…OnAbort` — NEW | **R** | G | G 4/4 | **R** | G | G | G | **R** | G | **R** |
| U6 `KeySetExact` + `NoSelfEqualHelp` | pair GREEN at the tip, RED on either half alone (§0.6) | G | G | G | G | G | G | G | G | G |

\* NC2 reddens U5's cancel half only as a knock-on, because it reads the cumulative value (0 ≠ 1). It is not an
independent detection.

**NC5 (skip the `<-fired` wait) is BLIND, as declared.** The full `./internal/listener/... ./internal/stats/...`
×5 read 470 RUN, 0 FAIL, rc 0; `-race` read 0 races. It is recorded as blind and **not faked**
(`reference_nc_roster_rows_can_be_structurally_vacuous`).

**Pre-fix GREEN arms, stated per arm (method note 61):**

- U2-NEW is green at the tip: the stub is ctx-aware, so the tip's pipeline aborts it.
- U4(ii) and U4(iii) are liveness pins.
- U5's immediate half reads 0 either way, which is why U5 asserts name EXISTENCE first.

Each one's falsifiability is its NC column above.

### 4.2 Suite counts

| tree | selector | RUN | FAIL (top-level) | rc |
|---|---|---|---|---|
| bare tip | `./internal/listener/...` / `./internal/stats/...` | 266 / 196 | 0 | 0 |
| tip + Appendices C, D, E | both | **470** | **6** (below) | 1 |
| PB1 + Appendices C, D, E | both | **470** | 0 | 0 |
| final (Appendices A + B + C + D + E) | both | **470** | 0 | 0 |
| final, `-race` | `./internal/listener/...` | — | 0 | **0** |

470 = 266 + 196 + 8 new top-level tests. U2 changes in place and U6 adds none. **The six tip-RED top-level
tests, re-run by the controller:**

- `TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients`
- `TestParseListenerFiltersTimeoutZeroDisables`
- `TestListenerFilterTimeoutPreCxTimeoutByValue`
- `TestListenerFilterTimeoutPreCxTimeoutOnAbort`
- `TestPipelineRunDeadlineInterruptsSilentPeek`
- `TestPipelineRunDeadlineClearedAfterTimeout`

**On the final tree:**

- `gofmt -l` prints nothing, and `go vet` is clean.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/... ./internal/stats/...` returns rc 0. A
  COMPILING planted control (`behaviour` in a test comment) fired `misspell`, rc 1, so the linters run.
- `go test ./internal/admin/... ./cmd/envoy-go/...` returns rc 0.
- ⚠️ **The five-selector `421` figure was NOT re-derived here.** It is the IMPL's to measure (Task 1), and
  that measurement names its selector with the number.

### 4.3 Fixture `0125`, per arm — 9 runs, none discarded (reference / subject)

| arm | tip | PB1 ×3 | NC1 ×2 | NC2 | NC3 | NC8 |
|---|---|---|---|---|---|---|
| F1 `l_false` silent | g / **R** open at 3000 | g/g ×3 | g / **R**, g / **R** | g/g | g/g | g/g |
| F2 `l_false` 50 concurrent silent | g / **R** 50/50 open | g/g ×3 | g / **R** 33 open, g / **R** 26 open | g/g | g/g | g/g |
| F3, T1, T2, T3, N1 | g/g | g/g | g/g | g/g | g/g | g/g |
| Z1 `l_zero` silent, 16.5 s | g/g | g/g | g/g | g/g | g / **R** FIN at 15012 | g/g |
| S `l_false` = 51 | g / **R** ABSENT | g/g | g / **R** 17, g / **R** 24 | g / **R** 0 | g/g | g / **R** 0 |
| S `l_true`, `l_true_tls` = 1 | g / **R** ABSENT | g/g | g/g, g / **R** 0 | g / **R** 0 | g/g | g/g |
| S `l_zero` = 0 | g / **R** ABSENT | g/g | g/g | g/g | g / **R** 1 | g/g |
| S `l_nofilt` = 0 | g / **R** ABSENT | g/g | g/g | g/g | g/g | g/g |
| CompareBytes | **R** | g | **R** | g | **R** | g |
| verdict (rc) | FAIL (1) | PASS (0) ×3 | FAIL ×2 | FAIL | FAIL | FAIL |

**The reference was green on every arm in all nine runs.**

- **NC2 and NC8 are invisible to CompareBytes.** Only `AssertStats` sees them, which is why S is pinned by
  VALUE.
- **Wall time:** 33.9-36.4 s per run. Each side's drive is bounded by Z1's 16.5 s, and every other arm runs
  concurrently with it.
- **Reference close spreads through `-p`:** 1000-1004 ms (§0.13).

---

## 5. Tasks

⚠️ **Order is load-bearing.** Tasks 1-7 land every falsifier and record the un-fixed tip **before** Task 8
changes production code. Nothing after Task 8 can recreate that measurement. ⚠️ **Evaluate the context budget
BEFORE starting each task** (`checking-context-budget`); commit after every task and keep `PROGRESS.md`
current. If the session stops mid-spine, say which task it reached and leave row 100 `in-progress`.

---

### Task 1: Baseline, a proven-live panic gate, and `PROGRESS.md`

**Files:**
- Create: `docs/envoy-go/phases/100-listener-filters-timeout-enforce/PROGRESS.md`

**Interfaces:**
- Produces: `base-listener.txt` / `base-stats.txt` (sorted `=== RUN` rosters at the un-fixed tip), which Task 7
  diffs against. Also the five-selector count, measured, with its selector named.

- [ ] **Step 1: Create the IMPL worktree off the CURRENT master tip.**
  `git -C /home/esa/git/envoy-go worktree add -b wt-phase-100-impl /home/esa/git/envoy-go-wt-p100impl master`.
  Then check `pwd` and `git -C /home/esa/git/envoy-go-wt-p100impl rev-parse --abbrev-ref HEAD` ⇒ `wt-phase-100-impl`.
- [ ] **Step 2: Resolve the selectors before believing any FAIL.** A nonexistent package prints
  `[setup failed]` and exits 1. `go list ./internal/listener/... ./internal/stats/... | wc -l` must be
  non-zero, and so must the same command over the five selectors (`./cmd/envoy-go/... ./internal/admin/...
  ./internal/boot/... ./internal/listener/... ./validate/...`).
- [ ] **Step 3: Record the bare-tip rosters.**
  ```bash
  W=/home/esa/git/envoy-go-wt-p100impl; S=<scratch>
  cd $W && out=$(go test -count=1 -v ./internal/listener/... 2>&1); rc=$?
  echo "$out" | /usr/bin/grep -oE '^=== RUN   [^ ]+' | sort > $S/base-listener.txt
  echo "rc=$rc RUN=$(wc -l < $S/base-listener.txt) FAIL=$(echo "$out" | /usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL')"
  ```
  Expected: `rc=0 RUN=266 FAIL=0`. Repeat for `./internal/stats/...`; expected `RUN=196`. Then run the five-selector
  suite and RECORD its count with the selector named (the router carries `421` from the phase-99 IMPL — measure,
  do not inherit).
- [ ] **Step 4: Prove the panic gate live.** Put a `panic("p100-gate")` in a throwaway `_test.go` in
  `internal/listener`. Run `go test -count=1 -run TestP100Gate ./internal/listener/` and confirm
  `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` reads ≥ 1. Delete the file, then run `git status --short` and
  confirm it prints nothing.
- [ ] **Step 5: Write `PROGRESS.md`** with a header (phase, worktree, base SHA) and a Task 1 entry quoting
  Steps 2-4's commands and outputs.
- [ ] **Step 6: Commit.**
  `git -C $W add docs/envoy-go/phases/100-listener-filters-timeout-enforce/PROGRESS.md && git -C $W commit -m "phase 100 (listener-filters-timeout-enforce) IMPL task 1: baseline rosters and a proven-live panic gate"`.

---

### Task 2: The manager-level unit file — U1, U3, U5, U5f — RED at the un-fixed tip

**Files:**
- Create: `internal/listener/listener_filters_timeout_test.go` (Appendix C.1, verbatim, 338 lines)

**Interfaces:**
- Consumes: tip helpers `startTaggedBackend`, `mkClusterMgr`, `mkTcpProxyFilter`, `mkTLSInspectorFilter`,
  `mkBoot`, `mkListener`, `NewManagerWithBaseDirAndAllowH2C`, `NewManager`, `testHTTPRegistry`,
  `testLFRegistry`, `testNetRegistryWithTerminals`; `stats.Registry.Walk`, `(*stats.Counter).Load`.
- Produces: the four test functions named in §4.2. **They read the counter BY NAME
  (`listener.<host with dots→_>_<port>.downstream_pre_cx_timeout`), never by struct field**, so the file
  compiles at the tip.

- [ ] **Step 1: Write the file** from Appendix C.1, byte for byte. U1 is `SPEC.md` §A plus exactly one
  import, `"strings"`, which U5 needs.
- [ ] **Step 2: Confirm it compiles and its selector matches:** `go vet ./internal/listener/` rc 0;
  `go test -count=1 -v -run 'TestListenerFilterTimeout|TestParseListenerFiltersTimeoutZeroDisables' ./internal/listener/ 2>&1 | /usr/bin/grep -c '^=== RUN'` ⇒ **4** top-level (plus none from other tests).
- [ ] **Step 3: Run it at the tip; expect ALL FOUR RED, each for its NAMED reason.**
  - U1: `got map[open:20]`, at 3.00 s.
  - U3: `lfTimeoutMs for an explicit 0s = 15000, want 0`.
  - U5: `name existence: counter … is not registered before traffic`.
  - U5f: the same name-existence failure.
  Record each failure line in `PROGRESS.md`. **A RED for any other reason is a finding. Stop and diagnose.**
- [ ] **Step 4: `gofmt -l internal/listener/` prints nothing.**
- [ ] **Step 5: Commit** with pathspec `internal/listener/listener_filters_timeout_test.go` and `PROGRESS.md`:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 2: U1/U3/U5/U5f RED at the un-fixed tip`.

---

### Task 3: The pipeline-level unit file — U4 (i) (ii) (iii) (iv) — (i) and (iv) RED at the tip

**Files:**
- Create: `internal/listener/listenerfilter/pipeline_deadline_test.go` (Appendix C.2, verbatim, 209 lines)

**Interfaces:**
- Consumes: `NewPeekerConn`, `AsPeeker`, `Pipeline.Run`, `Continue`, `ChainMatchInputs`. The package **cannot
  import `tls_inspector`**, so the test filter mimics its discipline: `Peek(5)`, ignore the error, and
  return `Continue, nil`.
- Produces: four tests named `TestPipelineRunDeadline*` / `TestPipelineRunZeroTimeoutHoldsSilentPeek`, over
  a loopback TCP pair (never `net.Pipe`).

- [ ] **Step 1: Write the file** from Appendix C.2, byte for byte.
- [ ] **Step 2: Selector check.**
  `go test -count=1 -v -run 'TestPipelineRunDeadline|TestPipelineRunZeroTimeoutHoldsSilentPeek' ./internal/listener/listenerfilter/ 2>&1 | /usr/bin/grep -c '^=== RUN'` ⇒ **4**.
- [ ] **Step 3: Run it at the tip.** Expect U4(i) **RED at ~3.0 s**: its own timer closes the peer, and
  the test FAILS rather than hangs. Expect U4(iv) **RED at ~1.0 s**. Expect U4(ii) and U4(iii) GREEN, as
  liveness pins (§4.1). Record all four in `PROGRESS.md`.
- [ ] **Step 4: `gofmt -l` prints nothing, and `go vet ./internal/listener/listenerfilter/` returns rc 0.**
- [ ] **Step 5: Commit** (pathspec: the new file + `PROGRESS.md`):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 3: U4 pipeline deadline arms, (i)/(iv) RED at the tip`.

---

### Task 4: U2 — the abort test strengthened IN PLACE

**Files:**
- Modify: `internal/listener/manager_test.go` at anchor **A7** (Appendix D, `9 3`, no new import)

**Interfaces:**
- Consumes: `errors`, `io`, `syscall`, already imported by `manager_test.go`.
- Produces: an assertion that the SERVER closed (EOF / `ECONNRESET`, 0 bytes) before 2 s. The old "no bytes
  arrived" branch is KEPT.

- [ ] **Step 1: Save the OLD test body** to scratch as a renamed copy (`TestU2OLDAbortsConnection` in
  `zz_u2old_test.go`, kept OUT of the tree) for Task 12's side-by-side NC6 run.
- [ ] **Step 2: Apply Appendix D** at anchor A7. `git -C $W diff --numstat -- internal/listener/manager_test.go` ⇒ `9	3`.
- [ ] **Step 3: Run it at the tip:** `go test -count=1 -v -run '^TestUnifiedDispatchListenerFilterTimeoutAbortsConnection$' ./internal/listener/` ⇒ `--- PASS` at ~1.00 s (GREEN at the tip BY DESIGN: the stub is ctx-aware). Record it as a pre-fix green whose falsifier is NC6 (Task 12).
- [ ] **Step 4: Commit** (pathspec: `manager_test.go`, `PROGRESS.md`):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 4: U2 asserts the SERVER closed, not only that no bytes arrived`.

---

### Task 5: Fixture `0125` — the driver

**Files:**
- Create: `test/fixtures/0125-listener-filters-timeout/driver/driver.go` (Appendix F.1, verbatim, 727 lines)

**Interfaces:**
- Consumes: `fixture.RegisterFixture`, `fixture.MultiListenerDriver` (`test/differential/fixture/fixture.go`,
  anchor `type MultiListenerDriver interface`), `fixture.StatsAsserter`.
- Produces: fixture name `0125-listener-filters-timeout`, byte-identical to the directory. The five listeners
  are `l_false`, `l_true`, `l_true_tls`, `l_zero` and `l_nofilt` on reference ports
  `15125 15228 15229 15230 15231`.

- [ ] **Step 1: Re-census the five ports at the IMPL tip.** Run
  `git grep -lw -- <port> -- test/ internal/ cmd/` for each, plus `ss -tan` / `ss -uan` in all states.
  Expected: `15125` only in `0123`'s reserving prose; the other four in zero files; no sockets.
- [ ] **Step 2: Write `driver.go`** from Appendix F.1, byte for byte.
- [ ] **Step 3: Compile and vet:** `go vet ./test/fixtures/0125-listener-filters-timeout/...` returns rc 0,
  and `gofmt -l test/fixtures/0125-listener-filters-timeout/` prints nothing.
- [ ] **Step 4: Commit** (pathspec: the driver file):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 5: fixture 0125 driver`.

---

### Task 6: Fixture `0125` — `README.md`, `expectations.yaml`, and the FOUR registration gates

**Files:**
- Create: `test/fixtures/0125-listener-filters-timeout/README.md` (Appendix F.2, 79 lines)
- Create: `test/fixtures/0125-listener-filters-timeout/expectations.yaml` (Appendix F.3, 30 lines)
- Modify: `test/differential/runner_test.go` (Appendix F.4, the blank import, `1 0`)

**Interfaces:**
- Produces: the fixture registered through all four gates (method note 60).

- [ ] **Step 1: Write `README.md` and `expectations.yaml`** from Appendices F.2 and F.3.
- [ ] **Step 2: Add the blank import** (Appendix F.4) in sorted position.
- [ ] **Step 3: Check the four gates by the SET, never by the exit code.** Gate 1 is `RegisterFixture` in
  `init()`. Gate 2 is the import. Gate 3 is name == directory, byte for byte. Gate 4 is the `NNNN-` shape. Run
  the extractor with arguments in the order **(imports, dirs)**:
  ```bash
  extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
  extract $W/test/differential/runner_test.go > $S/imports.txt
  ls -d $W/test/fixtures/*/ | xargs -n1 basename | sort > $S/dirs.txt
  wc -l < $S/imports.txt; wc -l < $S/dirs.txt; comm -23 $S/imports.txt $S/dirs.txt; comm -13 $S/imports.txt $S/dirs.txt
  ```
  Expected: **127**, **127**, and both `comm` outputs empty. `/usr/bin/grep -c '^0125-listener-filters-timeout$'` must read 1 in each file.
- [ ] **Step 4: NC the extractor** in a SCRATCH COPY. Rename the import's directory token; that must fire
  BOTH `comm` directions. Delete the import line; under the (imports, dirs) order that fires `comm -13`
  only.
- [ ] **Step 5: Commit** (pathspec: the two fixture files + `runner_test.go` + `PROGRESS.md`):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 6: fixture 0125 README, expectations, registration`.

---

### Task 7: RECORD the un-fixed tip — every falsifier RED, for its NAMED reason

**Files:**
- Modify: `PROGRESS.md` only

**Interfaces:**
- Consumes: Tasks 1-6.
- Produces: the pre-fix record that Task 10 compares against. **It cannot be recreated after Task 8.**

- [ ] **Step 1: Unit suite at the tip:** `./internal/listener/... ./internal/stats/...` with `-v`. Expected
  **RUN 470, rc 1, exactly the six top-level FAILs of §4.2**. Diff the sorted `=== RUN` roster against
  Task 1's: the additions must be exactly the eight new top-level names, and nothing may disappear.
- [ ] **Step 2: Fixture at the tip, ALONE:**
  `go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0125-listener-filters-timeout$' -timeout 30m`.
  First confirm the `=== RUN TestDifferential/0125-listener-filters-timeout` line and **zero `SKIP`** lines.
  Expected: subject RED on **F1** (open at 3000 ms), **F2** (50/50 open), all five **S** rows (series
  ABSENT) and CompareBytes. Everything else green. **Reference green on every arm.**
- [ ] **Step 3: Record the reference's close spread from this run** (51 closes, expected ~1000-1004 ms) and the
  subject's `open` count. Paste the failure lines into `PROGRESS.md`.
- [ ] **Step 4: Commit** `PROGRESS.md`:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 7: the un-fixed tip recorded — 6 unit tests and fixture F1/F2/S RED`.

---

### Task 8: THE PRODUCTION EDIT — PB1, code hunks only

**Files:**
- Modify: `internal/listener/listenerfilter/pipeline.go` (Appendix A, `15 0`)
- Modify: `internal/listener/manager.go` (Appendix A, `14 6`)

**Interfaces:**
- Produces:
  - the field `downstreamPreCxTimeout *stats.Counter` on `listenerRuntime`;
  - `rt.downstreamPreCxTimeout = r.NewCounter(prefix + "downstream_pre_cx_timeout")` in
    `registerListenerMetrics`, **unconditional**;
  - an `Inc` on `errors.Is(err, context.DeadlineExceeded)` in `serveConnection` step (4);
  - `parseListenerFiltersTimeout`: nil → 15000, explicit zero → 0.

- [ ] **Step 1: Extract and apply Appendix A** (`ext A <this file> > $S/patch-PB1.diff`, then
  `git -C $W apply --check` and `git -C $W apply`; or hand-edit at anchors A1-A5). `git -C $W diff --numstat` ⇒ `15 0 pipeline.go`, `14 6 manager.go`.
- [ ] **Step 2: Run the layout gate** (§6, `layout-gate.sh $W master`). Expected: sub-gates **a, b, c, e PASS**.
  Sub-gate **d FAILS** here BY DESIGN, because `tls_inspector.go` is not edited until Task 11. Record that
  exact shape.
- [ ] **Step 3: `gofmt -l internal/listener/` prints nothing; `go vet ./internal/listener/...` rc 0.**
- [ ] **Step 4: Assert the symbols landed, not only that the build passed** (method note 7). Each of these
  must read ≥ 1:
  - `git -C $W grep -c 'context.AfterFunc' -- internal/listener/listenerfilter/pipeline.go`
  - `git -C $W grep -c 'downstream_pre_cx_timeout' -- internal/listener/manager.go`
  - `git -C $W grep -c 'errors.Is(err, context.DeadlineExceeded)' -- internal/listener/manager.go`
- [ ] **Step 5: Commit** (pathspec: the two files):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 8: one clock inside Pipeline.Run; downstream_pre_cx_timeout; 0s disables`.

---

### Task 9: The help-text PAIR — ONE commit

**Files:**
- Modify: `internal/stats/name.go` (Appendix E.1, `2 0`: its own paragraph after anchor A8's
  `envoy_server_accesslog_dropped` line)
- Modify: `internal/stats/helptext_test.go` (Appendix E.2, `1 0`)

- [ ] **Step 1: Show each half alone is RED.** Apply E.2 alone; `go test -count=1 -v ./internal/stats/`
  must FAIL `TestHelpText_KeySetExact` **and** `TestHelpText_NoSelfEqualHelp`. Revert it, apply E.1 alone,
  and the same command must FAIL `KeySetExact` only. Revert.
- [ ] **Step 2: Apply both;** `go test -count=1 -v ./internal/stats/` ⇒ rc 0, RUN 146.
- [ ] **Step 3: Commit BOTH files in ONE commit** (pathspec: both):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 9: help text for downstream_pre_cx_timeout (the pair, one commit)`.

---

### Task 10: ALL ARMS GREEN — unit, race, fixture ×3

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Unit suite.** `./internal/listener/... ./internal/stats/...` with `-v` must read **RUN 470,
  0 FAIL, rc 0**. The roster must equal Task 7's, with only the outcome changed.
- [ ] **Step 2: Repeat the load-bearing arms.** Run U1 ×3 and U4 ×3 under `-race`
  (`-run 'TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients'` and `-run 'TestPipelineRun'`). All
  must be green, with 0 `DATA RACE`.
- [ ] **Step 3: `go test -count=1 -race ./internal/listener/...` returns rc 0.** This is the full package, not
  the differential: the differential's subject is an unraced subprocess.
- [ ] **Step 4: Fixture ALONE ×3.** Every run must PASS, with the `=== RUN` line present and 0 `SKIP`.
  Record both sides' F1+F2 close spreads per run. Expected: reference ~1000-1004 ms, subject ~1000-1021 ms.
  **If any close falls outside `[700, 1800]`, STOP.** Re-derive the window from the pooled spreads with the
  σ-margin method (§0.13), and never drop the arm.
- [ ] **Step 5: Commit** `PROGRESS.md`:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 10: all arms green — unit 470/0, -race clean, fixture 3/3`.

---

### Task 11: The occurrence-set reconciliation in CODE — comments, under TWO gates

**Files:**
- Modify: `internal/listener/listenerfilter/pipeline.go` (Appendix B, `2 2`: `Run` doc, line-count-neutral)
- Modify: `internal/listener/listenerfilter/tls_inspector/tls_inspector.go` (Appendix B, `3 3`)
- Modify: `internal/listener/manager.go` (Appendix B, `11 10`, five sites: field doc `:166-167`, metric-block
  doc `:179-184` and registration doc `:376` (§0.9), parse doc `:948-950`, step (4) doc `:1295-1297`)

- [ ] **Step 1: Extract and apply Appendix B** (`ext B <this file> > $S/patch-COMMENTS.diff`, then
  `git -C $W apply --check` and `git -C $W apply`; it applies on top of Appendix A). The added-line counts
  in `git -C $W diff --numstat HEAD` must be `2 2`, `3 3` and `11 10`.
- [ ] **Step 2: Gate the KIND.** Every changed line in the Task-11 diff must be a comment:
  `git -C $W diff -U0 HEAD | /usr/bin/grep -E '^[+-]' | /usr/bin/grep -vE '^(\+\+\+|---) ' | sed -E 's/^[+-][[:space:]]*//' | /usr/bin/grep -vc '^//'`
  must read **0**. ⚠️ `grep -c` on zero matches prints `0` and EXITS 1: capture the output, never chain on it.
- [ ] **Step 3: Gate the SHAPE.** `layout-gate.sh $W master` must read **0 failed sub-gates** (a-e all PASS).
- [ ] **Step 4: Show the gates fire.** In a scratch copy, plant a line-adding edit to the `Run` doc; (e) must
  FAIL. Plant a code change in `tls_inspector.go` at shape `3 3`; (d) must FAIL. Revert both.
- [ ] **Step 5: Re-run the unit suite:** RUN 470, 0 FAIL. `gofmt -l` prints nothing.
  `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/... ./internal/stats/...` returns rc 0.
- [ ] **Step 6: Commit** (pathspec: the three files):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 11: comment reconciliation, comment-only and layout-gated`.

---

### Task 12: The NC roster — eight mutants, each proven able to fire, scored PER ARM

**Files:**
- Modify: `PROGRESS.md` only. **Every mutant is applied in a THROWAWAY worktree and removed.**

**Interfaces:**
- Consumes: Appendix H's eight patches (`ext H.1` … `ext H.8`), each **relative to PB1** (they apply with an
  offset on the final tree; `patch -p1` reports the offset). H.1 was verified to apply on A+B at the PLAN close.

- [ ] **Step 1: Create a throwaway detached worktree** at the Task 11 commit. Copy in the Task 4 OLD-U2 file
  (`zz_u2old_test.go`).
- [ ] **Step 2: Write each NC's mechanism BEFORE running it,** into `PROGRESS.md`, as §7's "mechanism" column
  does. A row that cannot name one is vacuous (method note 7d).
- [ ] **Step 3: For each of NC1-NC8, apply the patch and ASSERT ITS MARKER LINE IS PRESENT.** Each patch
  carries an `NC<n>` comment; `/usr/bin/grep -c 'NC<n>'` on the patched file must read ≥ 1 **before** reading
  any result (§0.16's slip). Run the unit suite with `-v`. Record every arm of §4.1's column. Reverse the
  patch, and confirm the marker count is back to 0.
- [ ] **Step 4: NC1 ×3 (U1 is probabilistic), NC5 ×5 (declared blind; record, do not fake).** Run NC6 with
  the OLD and the NEW U2 side by side. Expected: OLD **PASS at 3.00 s**, NEW **FAIL**.
- [ ] **Step 5: Fixture NCs, serialized on Docker:** NC1 ×2, NC2, NC3, NC8, each run ALONE, scored per row
  against §4.3.
- [ ] **Step 6: Compare every cell with §4.1 / §4.3.** A cell that differs is a finding. Record it, and
  NEVER re-run until it matches.
- [ ] **Step 7: Remove the throwaway worktree;** confirm with `git worktree list`.
- [ ] **Step 8: Commit** `PROGRESS.md`:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 12: NC roster scored per arm (NC5 blind, NC6 vacuity measured)`.

---

### Task 13: `ADR-0322` completed IN PLACE — §Decision + §Consequences

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, after anchor **A10** (the retained italic footer)

- [ ] **Step 1: Append `### Decision (phase-100 IMPL)`** AFTER the RETAINED footer. Do not add a
  `**Status:**` line, a renumber or a `---`. It covers:
  - (a) PB1 as built, citing `pipeline.go:43` as unmoved;
  - (b) the counter, registered unconditionally, `Inc` on `DeadlineExceeded` under both `continue…` values;
  - (c) the `0s` split;
  - (d) the SUPERSESSIONS of ADR-0082 §Decision ¶1 "honored" / "(zero-valued duration)", ¶2 as behaviour,
    ¶3 "enforced", and §Consequences (b) — naming what ADR-0082 KEEPS (the `[1s, 60s]` envelope and the
    per-pipeline shared budget);
  - (e) ADR-0296's `pipeline.go:43` claim, which stays true by the layout gate.
- [ ] **Step 2: Append `### Consequences (phase-100 IMPL)`.** It covers:
  - the declared behaviour changes (`SPEC.md` §4.3), with the shutdown row corrected to *"ends at the
    cancel, books nothing"* (§0.8);
  - the QUIC registration, unmeasured on the reference (§0.9);
  - what was NOT bought (`SPEC.md` §1, §14.3);
  - the NC roster outcome (NC5 blind, NC6's vacuity);
  - the envelope lift as the next row.
- [ ] **Step 3: Flip the status blockquote** from `PROPOSED` to `ACCEPTED`, in the house form (the phase-99 IMPL's
  ADR-0321 is the precedent — read it, copy the SHAPE).
- [ ] **Step 4: The guard is now DISARMED; prove it by LINE and by ADR.** `^> \*\*STATUS: PROPOSED` must read
  zero hits. The ADR-0231 decoy must still hit `:14866`, resolving to `## ADR-0231`, with md5
  `929719b67c87…` unchanged. `^---$` must read 216, and next-free must read ADR-0323 (TAIL-derived).
- [ ] **Step 5: Commit** (pathspec: `DECISIONS.md`):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 13: ADR-0322 §Decision + §Consequences, ACCEPTED`.

---

### Task 14: `BEHAVIOR_CONTRACT.md` — `:4359` and the `+1` ledger entry

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` at anchor **A9**

- [ ] **Step 1: Rewrite the `Per-pipeline timeout` bullet** (in place, one line). It must say:
  - enforced by one clock inside `Pipeline.Run`;
  - absent → 15 s; explicit `0s` DISABLES;
  - the `[1s, 60s]` envelope is envoy-go's OWN, and the reference accepts `0.5s` / `61s` (the next row);
  - `continue_on_listener_filters_timeout` false closes and true falls through, both AT the deadline;
  - each timeout books `downstream_pre_cx_timeout`.
- [ ] **Step 2: Append the ledger entry after `**Phase 99 — +0, UNCHANGED`**:
  `**Phase 100 — +1 (`listener.<addr>.downstream_pre_cx_timeout`) — delta only:** …`. It must quote **NO
  absolute**, and say in its own text that it departs from phase 94's `A → B (+N)` form because three
  mutually inconsistent absolutes are live at one tip.
- [ ] **Step 3: Run `/usr/bin/grep -nE '→|->' ` over the new entry.** It must contain no `A → B` absolute.
  Then run `wc -l`: expected **5998 → 5999** (+1 entry line, with the bullet rewritten in place).
- [ ] **Step 4: Commit** (pathspec: the contract):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 14: contract — timeout enforced, 0s disables; ledger +1 (delta only)`.

---

### Task 15: `REVIEW_FINDINGS.md` — the timeout clause annotated, the rest left TRUE

**Files:**
- Modify: `REVIEW_FINDINGS.md` at anchor **A11** (`:185-188`)

- [ ] **Step 1: Annotate only the timeout clause** as fixed at phase 100 (ADR-0322). **Leave** the
  non-timeout-gating clause (recorded, not decided: `SPEC.md` §10) and the SNI-case clause (still true)
  unchanged.
- [ ] **Step 2: `git diff --numstat` reads ~`2 1`.** Re-read the four lines as a stranger would, and confirm
  neither surviving clause now reads as fixed.
- [ ] **Step 3: Commit** (pathspec: the file):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 15: REVIEW_FINDINGS timeout clause annotated fixed`.

---

### Task 16: `ROADMAP.md` — row 100 → `done`, under the FIELD-COUNT gate

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md`, row **A12** only

- [ ] **Step 1: Write the new cell in a SCRATCH copy first.** Count its fields under BOTH forms. Naive:
  `awk -F'|' '/^\| *100 /{print NF}'`. Escape-aware: `sed 's/\\|//g' F | awk -F'|' '/^\| *100 /{print NF}'`,
  with **no file argument to awk**. Both must read **8**. **Reword any pipe AWAY; never escape it.**
- [ ] **Step 2: Assert the cell spells NEITHER sentinel match phrase** (`deferred candidates:` or
  `remaining deferred (not-yet-chartered) candidates:`). Also check the bare word `deferred`,
  case-insensitively.
- [ ] **Step 3: Install it and re-run the full sentinel** (§2.1's commands). Expected, measured on both sides:
  - check (1) goes ONE → **SILENT**, and NC-A and NC-B go TWO → **ONE**;
  - `want` stays **132** and `ROADMAP.md` stays **250**;
  - the six windows keep their md5s;
  - the malformed set stays {57, 69}.
- [ ] **Step 4: Commit** (pathspec: `ROADMAP.md`):
  `phase 100 (listener-filters-timeout-enforce) IMPL task 16: row 100 done`.

---

### Task 17: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

**Files:**
- Modify: `PROGRESS.md` only

- [ ] **Step 1: Byte-untouched roster (§1.2).** `git -C $W diff master --numstat -- <each path>` must be EMPTY
  for every path. The edit roster's paths must be exactly §1.2's; list any path that is on neither.
- [ ] **Step 2: ARM roster (method note 28).** Diff the sorted `=== RUN` roster against Task 1's base. It must
  gain exactly the eight new names and lose nothing.
- [ ] **Step 3: Gate (a), the full differential** with `-count=1`, the fixture set asserted BY NAME in BOTH
  `comm` directions (**127 = 127**), and the count of PASS/FAIL/SKIP. Never trust the exit code alone.
  **A port-race abort is MASKING:** record it and rerun, but a green rerun clears nothing.
- [ ] **Step 4: Gate (b), the non-Docker sweep:**
  `go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'`, gated on `PIPESTATUS[0]`
  plus a SET RECONCILIATION of `ok` / `FAIL` / `[no test files]` against the package count. The package count
  rises by `0125/driver`, one package; measure it rather than predicting it. Keep every sweep on the record.
- [ ] **Step 5: Gate (c)**, h2spec, run verbose. Expected: `95 tests, 94 passed, 1 skipped, 0 failed`.
- [ ] **Step 6: Gate (d)**, the fuzzers: `git grep -c '^func Fuzz' -- '*.go'`. The row adds none, so expect 56
  targets across 48 FILES.
- [ ] **Step 7: Gate (e)**, the anchored panic gate: **0**, proven live at Task 1. Then run
  `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` (rc 0) with a COMPILING planted control that fires
  `errcheck`, `revive` and `ineffassign`, and remove the control.
- [ ] **Step 8: Gate (f):** no `REVIEW.md`. That is the ONE standing departure (none of 93-99 has one). Name
  it; do not claim compliance.
- [ ] **Step 9: Commit** `PROGRESS.md`:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 17: roster, arm roster and six-gate sweep recorded`.

---

### Task 18: `PROGRESS.md` close-out and the router/state roll

**Files:**
- Modify: `PROGRESS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`

- [ ] **Step 1: Close `PROGRESS.md`:** a per-task summary, and every refutation this IMPL made (method note 2).
- [ ] **Step 2: Roll `STATE.md` IN PLACE** (the §Current pointer, the lifecycle state set to DONE, next-free
  ADR-0323). Evict the oldest §Recent entry. **MEASURE the pre-roll date histogram INCLUDING the entry this
  close promotes**, and verify the eviction with the LABEL-BOUND PAIR on BOTH files, a fabricated-label NC
  and a positive control on an ARCHIVED label.
- [ ] **Step 3: Archive the evictee as ONE inline parenthetical line.** Expected: strict guard DELTA 0, raw
  delta **+2**, naming the LABEL and no figure.
- [ ] **Step 4: Roll `next-prompt.txt`.** The sentinel re-evaluates, and the next row is the `[1s, 60s]` envelope
  lift, now UNBLOCKED. Grep the result for `YOUR STAGE`, for `IMPL`, and for every figure this row moved (`want`,
  the fixture set 127, the guard polarity, the ledger chain) before committing (method note 68).
  `git add -f next-prompt.txt`.
- [ ] **Step 5: Commit** the four files:
  `phase 100 (listener-filters-timeout-enforce) IMPL task 18: close-out, state and router rolled`.

---

### Task 19: Squash, merge, push, and remove the worktree

- [ ] **Step 1: Squash the branch to ONE commit.** Its subject carries the FULL SLUG and the stage word:
  `phase 100 (listener-filters-timeout-enforce) IMPL: …`.
- [ ] **Step 2: Merge to master fast-forward.** Re-run the sentinel in the MERGED tree; it must match Task 16.
- [ ] **Step 3: Push** (`feedback_push_to_origin`, using the repo's configured email).
- [ ] **Step 4: `git worktree remove` the IMPL worktree.** `git worktree list` must then read master only. Check the
  repo root with `git status --short --untracked-files=all`; ignore the pre-existing `.claude/`.

---

## 6. The layout gate — BUILT, AND SHOWN TO FIRE

`SPEC.md` §4.1 (e) is mechanized as `layout-gate.sh <worktree> [base]` (Appendix G). There is one PASS/FAIL line
per sub-gate, and the exit status is the number of failed sub-gates. Measured at this PLAN:

| input | a | b | c | d | e | exit |
|---|---|---|---|---|---|---|
| **FINAL** (Appendices A + B) | P | P | P | P | P | **0** |
| tip / PB1 only | P | P | P | F (by design: `tls_inspector.go` not yet edited) | P | 1 |
| plant a: `// planted` on `:35` (line-count-neutral) | **F** | P | P | P | P | 1 |
| plant b: `"time"` → `"os"` (line-count-neutral) | P | **F** | P | P | P | 1 |
| plant c: a second CODE `context.WithTimeout` in `manager.go` | P | P | **F** | P | P | 1 |
| plant d1: `tls_inspector.go` comment-only at `4 3` | P | P | P | **F** | P | 1 |
| plant d2: `tls_inspector.go` at `3 3`, one line is code | P | P | P | **F** | P | 1 |
| plant e2: one line inserted between `:43` and `:44` | P | P | P | P | **F** | 1 |
| plant e: the `Run` doc grows a line | F | P | F | P | **F** | 3 |
| **controller:** `// planted` inserted after line 8 | F | P | F | P | **F** | 3 |

**Every sub-gate fires on its own planted input, alone.** Plant e2 shows that (a) cannot see an insertion
between `:43` and `defer cancel()`; only (e) catches that. Sub-gate (c) drops `//` lines (§0.1). Sub-gate (e) parses `-U0`
hunk headers: a hunk that begins at or above line 44 must be `N N`. PB1's insertion reads `@@ -44,0 +45,15 @@`,
which is legal.

---

## 7. The negative-control roster — CORRECTED against `SPEC.md` §11

| NC | mutation (compiles; Appendix H) | mechanism to a failure | must redden (MEASURED) | stays green (MEASURED) |
|---|---|---|---|---|
| NC1 | PB → P0: the AfterFunc block removed; a raw `SetReadDeadline` second clock in `serveConnection` | the socket deadline wins, `tls_inspector` maps it to `raw_buffer`/`Continue`, `Run` returns nil; nothing reacts to a ctx cancel | U1 (4/4, probabilistic), U4(i), U4(iv), **U5 cancel (4/4, deterministic)**; fixture **F1, F2, S `l_false`, CompareBytes** | U3, U6, U5 silent (4/4 — a coin flip); fixture S `l_true*` only 1 of 2 runs |
| NC2 | `Inc` deleted | the counter never moves | U5 silent value, **U5f**; fixture S `l_false`/`l_true`/`l_true_tls` | U1, F1, F2, CompareBytes |
| NC3 | the `0s` fold reverted | `0s` reads 15000, and the 15 s drop fires | U3; fixture **Z1, S `l_zero`, CompareBytes** | U1, U4, U5 |
| NC4 | the deferred clear deleted | a fired deadline survives into the handed-off connection | **U4(iv), U5 silent liveness** | **U4(ii)** (§0.2), U1, U3 |
| NC5 | the `<-fired` wait skipped | a narrow race; no deterministic path | **NONE — declared BLIND** (×5, `-race` clean) | all |
| NC6 | the abort-branch `pkConn.Close()` deleted | a timed-out `false` connection is never closed | U1, U2-NEW, **U5f** | **U2-OLD (3.00 s — the vacuity)**, U3, U5 |
| NC7 | `errors.Is(…DeadlineExceeded)` → `err != nil` | a cancel books the counter | U5 cancel (reads 2) | U1, F1 |
| NC8 | book only under `true` | a `false` timeout books nothing | **U5f**; fixture S `l_false` | U5, T-arms, CompareBytes |

---

## 8. Counts — this PLAN's own tip

- **Moved by this stage:**
  - `PLAN.md` new (its line count is re-derived by `wc -l` in the publishing commit and quoted in `STATE.md`);
  - `STATE.md` rolled IN PLACE;
  - `STATE_HISTORY.md` **590 → 592** (`2 0`);
  - `next-prompt.txt` rolled.
- **NOT moved:**
  - `ROADMAP.md` **250** / **132** rows, row 100 `in-progress` at `:162`;
  - `DECISIONS.md` **19509**, `^---$` 216, `^## ADR-` 321, bare `^## ` 329, tail ADR-0322 (`PROPOSED`),
    next-free ADR-0323;
  - `BEHAVIOR_CONTRACT.md` **5998**;
  - every `.go` file;
  - fixtures **126 = 126**;
  - phase dirs **141**.
- **None of the six gates was run — a PLAN's scope, not an omission.** Building and running the appendices in
  throwaway worktrees is MEASUREMENT. The one standing departure is no `REVIEW.md` (none of 93-99).

---

## 9. Deferred — carried, none chartered

- **Carried unchanged from `SPEC.md` §14.3:** the close KIND under `false`, the HTTP/1 codec's invalid-method-byte
  wait, the `downstream_listener_filter_{remote_close,error}` names, the `[1s, 60s]` envelope lift (the
  NEXT row, unblocked once this row's IMPL lands), and `downstream_cx_total` post-filter accounting.
- **NEW at this PLAN — QUIC listeners register `downstream_pre_cx_timeout`** (`quic.go:66` →
  `registerListenerMetrics`) at value 0 forever. The reference's QUIC behaviour is **unmeasured**; measure it
  before any pin (§0.9).
- **NEW at this PLAN — `manager.go` line-citations are pre-drifted** (§0.14): ten sampled, none exact. This is a
  citation-hygiene item, not a row.

---

## 10. `SPEC.md` §15 coverage, and self-review

| # | `SPEC.md` §15 owed | discharged at |
|---|---|---|
| 1 | build and run every code block (U2-U6, the fixture, every NC), at the tip and under PB1, in throwaway worktrees | §4 (every cell measured); Appendices A-H are the built artifacts; §0.2-0.5 and §0.10-0.11 are what the building refuted |
| 2 | F2 on the reference through `-p`; set the windows from BOTH spreads | §0.13 — 1000-1004 ms (n=459), window `[700, 1800]` KEPT with its σ arithmetic |
| 3 | gate the layout mechanically, each gate shown to fire | §6 — `layout-gate.sh`, five sub-gates, each fired by its own plant; §0.1 corrects the `WithTimeout` gate |
| 4 | order the spine (the tip recorded before PB1); the help-text pair in ONE commit | Tasks 1-7 before Task 8; Task 9 (the pair, with each half's RED shown first) |
| 5 | the ledger entry, `:4359`, the `REVIEW_FINDINGS.md` annotation, ADR-0322 §Decision + §Consequences at the IMPL | Tasks 13, 14, 15 |
| 6 | evaluate the split gate with a stated accounting | §1.3 — **1441** `.go`+fixture added, MEASURED; ~1547 with docs; **not split** |
| 7 | re-census the ports; the extractor in both `comm` directions, naming the argument order | §2.3, §2.4; Task 5 Step 1, Task 6 Steps 3-4 |

**Self-review, by command:**

- **Tasks:** 19, measured with `/usr/bin/grep -c '^### Task ' PLAN.md`.
- **Sub-steps per task:** measured with `awk` over `^- \[ \] ` per `^### Task ` heading. The maximum is
  quoted in `STATE.md` at this close, and must be ≤ 10 (§6.1).
- **Placeholder scan:** `/usr/bin/grep -nE 'TBD|TODO|implement later|fill in' PLAN.md` must read only this
  line's own mention of those words.
- **Type consistency:** the counter field (`downstreamPreCxTimeout`), the stat name
  (`downstream_pre_cx_timeout`), the eight test names and the fixture name are spelled identically in §4, §5, §7
  and the appendices. They were checked by `git grep` against the built files.

---

# Appendices — the MEASURED artifacts (built, run and reverted at this PLAN stage)

Each appendix is the byte-exact artifact the measurement ran. The diffs are against `b6aa3ab3`. Appendix H's NC
patches are relative to PB1.

## Appendix A — PB1, the production CODE hunks (`pipeline.go` `15 0`, `manager.go` `14 6`). Task 8

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index 5b3f309c..4a8b5954 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -42,6 +42,21 @@ func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Pee
 		var cancel context.CancelFunc
 		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
 		defer cancel()
+		// ONE clock: the socket read is interrupted only AFTER ctx is done, so
+		// an interrupted Peek always observes ctx.Err() != nil below.
+		if ds, ok := peeker.(interface{ SetReadDeadline(time.Time) error }); ok {
+			fired := make(chan struct{})
+			stop := context.AfterFunc(ctx, func() {
+				_ = ds.SetReadDeadline(time.Unix(1, 0))
+				close(fired)
+			})
+			defer func() {
+				if !stop() {
+					<-fired
+				}
+				_ = ds.SetReadDeadline(time.Time{})
+			}()
+		}
 	}
 	for i, f := range filters {
 		status, err := f.Inspect(ctx, peeker, inputs)
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index 82ea7c46..b784b72a 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -182,11 +182,12 @@ type listenerRuntime struct {
 	// registered for EVERY listener; the five ssl.* counters are registered
 	// only when rt.tlsMode is set (phase 74 — TLS-chains-only, matching the
 	// reference), so on a plaintext listener all five pointers stay NIL.
-	downstreamCxTotal   *stats.Counter
-	downstreamCxActive  *stats.Gauge
-	sslHandshake        *stats.Counter // phase 74: successful downstream TLS handshakes
-	sslFailVerifyError  *stats.Counter // phase 74: client cert presented, CHAIN VERIFICATION failed
-	sslFailVerifyNoCert *stats.Counter // phase 74: no client cert where one was required
+	downstreamCxTotal      *stats.Counter
+	downstreamPreCxTimeout *stats.Counter // phase 100: listener_filters_timeout fired (ADR-0322)
+	downstreamCxActive     *stats.Gauge
+	sslHandshake           *stats.Counter // phase 74: successful downstream TLS handshakes
+	sslFailVerifyError     *stats.Counter // phase 74: client cert presented, CHAIN VERIFICATION failed
+	sslFailVerifyNoCert    *stats.Counter // phase 74: no client cert where one was required
 	// sslNoCertificate is phase 75's SUCCESS-PATH annotation: a COMPLETED
 	// handshake that presented no client certificate. It is NOT a synonym for
 	// sslFailVerifyNoCert (a FAILED handshake) — the two are disjoint by
@@ -416,6 +417,7 @@ func normalizeAddr(addr string) string {
 func registerListenerMetrics(r *stats.Registry, rt *listenerRuntime) {
 	prefix := "listener." + normalizeAddr(rt.addr) + "."
 	rt.downstreamCxTotal = r.NewCounter(prefix + "downstream_cx_total")
+	rt.downstreamPreCxTimeout = r.NewCounter(prefix + "downstream_pre_cx_timeout")
 	rt.downstreamCxActive = r.NewGauge(prefix + "downstream_cx_active")
 	if rt.tlsMode {
 		rt.sslHandshake = r.NewCounter(prefix + "ssl.handshake")
@@ -950,9 +952,12 @@ func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netRe
 // error.
 func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error) {
 	const defaultMs = 15000
-	if d == nil || (d.GetSeconds() == 0 && d.GetNanos() == 0) {
+	if d == nil {
 		return defaultMs, nil
 	}
+	if d.GetSeconds() == 0 && d.GetNanos() == 0 {
+		return 0, nil
+	}
 	total := d.AsDuration()
 	ms := total / time.Millisecond
 	if ms < 1000 || ms > 60000 {
@@ -1348,6 +1353,9 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 	// (4) Run listener-filter pipeline.
 	var p listenerfilter.Pipeline
 	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
+		if errors.Is(err, context.DeadlineExceeded) {
+			rt.downstreamPreCxTimeout.Inc()
+		}
 		if !rt.continueOnLfTimeout {
 			log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, err)
 			_ = pkConn.Close()
```

## Appendix B — the comment reconciliation, comment-only (`2 2`, `3 3`, `11 10`; zero non-comment lines), relative to PB1. Task 11

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index 4a8b5954..d6aed03e 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -24,8 +24,8 @@ type Pipeline struct{}
 //   - On Continue: advances to the next filter (or finishes if last).
 //   - On StopIteration: halts the loop; remaining filters are skipped.
 //   - On non-nil error: aborts; the error is wrapped with the filter index.
-//   - On context-deadline-exceeded after a filter's Inspect returns: the
-//     pipeline returns a wrapped timeout error.
+//   - timeoutMs > 0, ctx done: a Peek blocked on a deadline-capable peeker is
+//     cut (the deadline is cleared before return); returns wrapped ctx.Err().
 //   - OnDestroy is called on every filter (in declaration order) after the
 //     loop ends, regardless of how the loop exited (Continue/StopIteration/
 //     error/timeout).
diff --git a/internal/listener/listenerfilter/tls_inspector/tls_inspector.go b/internal/listener/listenerfilter/tls_inspector/tls_inspector.go
index b4e1ffa1..d298c6d0 100644
--- a/internal/listener/listenerfilter/tls_inspector/tls_inspector.go
+++ b/internal/listener/listenerfilter/tls_inspector/tls_inspector.go
@@ -52,9 +52,9 @@ type filter struct {
 // listener-filter pipeline began running on real network connections.
 func (f *filter) Inspect(ctx context.Context, peeker listenerfilter.Peeker, inputs *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
 	// Step 1: peek the 5-byte TLS record header to learn the record length.
-	// Peek yields only net/io errors (io.EOF, net.ErrClosed,
-	// os.ErrDeadlineExceeded) — ctx is not plumbed into the socket read —
-	// so every zero-byte error is a non-TLS classification, not an abort.
+	// Peek yields only net/io errors (io.EOF, net.ErrClosed, or the read
+	// deadline Pipeline.Run sets once its ctx is done), so every zero-byte
+	// error classifies raw_buffer; the pipeline, not this filter, aborts.
 	hdr, err := peeker.Peek(5)
 	if err != nil && len(hdr) == 0 {
 		inputs.TransportProtocol = "raw_buffer"
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..6d9cb8b8 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -163,8 +163,8 @@ type listenerRuntime struct {
 	chainByName  map[string]*chainInfo
 	// 07.2 (Task 9, ADR-0079) listener-filter pipeline plumbing. Populated
 	// from listener_filters[] at build time; consumed by serveConnection's
-	// per-conn pipeline. lfTimeoutMs is in [1000, 60000] (ADR-0082); default
-	// 15000.
+	// per-conn pipeline. lfTimeoutMs is 0 (an explicit 0s: disabled, ADR-0322)
+	// or in [1000, 60000] (ADR-0082); default (nil) 15000.
 	listenerFilterFactories []listenerfilter.FilterInstanceFactory
 	lfTimeoutMs             uint32
 	continueOnLfTimeout     bool
@@ -178,8 +178,8 @@ type listenerRuntime struct {
 	lfPeekBufSize int
 	// 06.1 metric fields (per SPEC §6 — listener-scope only). Allocated by
 	// registerListenerMetrics at Start time (post-bind, pre-Freeze) and
-	// Inc/Dec'd from the accept-loop hot path. The two cx metrics are
-	// registered for EVERY listener; the five ssl.* counters are registered
+	// Inc/Dec'd from the per-connection path. The two cx metrics and
+	// downstream_pre_cx_timeout are registered for EVERY listener; the ssl.* ones
 	// only when rt.tlsMode is set (phase 74 — TLS-chains-only, matching the
 	// reference), so on a plaintext listener all five pointers stay NIL.
 	downstreamCxTotal      *stats.Counter
@@ -374,7 +374,7 @@ func normalizeAddr(addr string) string {
 // configured with port 0 don't collide on the same registered name pre-bind).
 // Pre-Freeze (Task 12 owns the Freeze call after the admin server is up).
 //
-// The two cx metrics are unconditional. The three phase-74 ssl.* counters plus
+// The two cx metrics and downstream_pre_cx_timeout are unconditional. The three phase-74 ssl.* counters plus
 // the one phase-75 ssl.* counter (ssl.no_certificate) plus the one phase-94
 // ssl.* counter (ssl.connection_error — five in total) are all
 // gated on rt.tlsMode, matching the reference, which registers listener.<addr>.ssl.*
@@ -948,8 +948,8 @@ func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netRe
 }
 
 // parseListenerFiltersTimeout parses Listener.listener_filters_timeout per
-// ADR-0082: nil/zero defaults to 15000ms; values outside [1000, 60000]ms
-// error.
+// ADR-0082/ADR-0322: nil defaults to 15000ms; an explicit zero is 0 (disabled,
+// as on the reference); other values outside [1000, 60000]ms error.
 func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error) {
 	const defaultMs = 15000
 	if d == nil {
@@ -1297,9 +1297,10 @@ func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
 //     largest build-time InitialReadBufferSizer hint when filters exist).
 //  3. Construct per-connection ListenerFilter instances from the per-listener
 //     factory slice (one allocation per filter per connection).
-//  4. Run the listener-filter pipeline; on error, honor
-//     `continue_on_listener_filters_timeout` — false aborts the connection,
-//     true falls through with whatever inputs were populated so far.
+//  4. Run the listener-filter pipeline, which cuts a blocked peek at
+//     listener_filters_timeout; a timeout books downstream_pre_cx_timeout.
+//     On any error, honor `continue_on_listener_filters_timeout` — false
+//     aborts the connection, true falls through with the inputs populated.
 //  5. Run listenerfilter.SelectChain over chainSpecs / defaultSpec; abort on
 //     ErrNoChainMatched or ErrAmbiguousChainMatch.
 //  6. If the selected chain has TLS, hand the peekerConn to stdtls.Server with
```

## Appendix C.1 — `internal/listener/listener_filters_timeout_test.go` (338 lines: U1, U3, U5, U5f). Task 2

```go
package listener

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients drives the REAL
// tls_inspector (not the ctx-aware installSlowListenerFilter stub) through a
// manager-built listener: listener_filters_timeout 1s, continue_on_... false
// (default), N concurrent SILENT clients over real loopback TCP. Each client
// holds a 3 s read deadline of its own, and the test separates the three
// outcomes the tip's abort test conflates:
//
//   - closed: the SERVER closed (EOF or ECONNRESET, zero bytes) before 2 s;
//   - fellThrough: bytes arrived — the pipeline reported success and the
//     tcp_proxy chain's tagged backend answered (the deadline-only race);
//   - open: the CLIENT's own deadline expired — the server never acted.
//
// Concurrency is load-bearing: a single connection passes a racy
// two-clock repair most of the time.
func TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients(t *testing.T) {
	const n = 20
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_real_inspector",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:        []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr

	type outcome struct {
		kind string // closed | fellThrough | open | late | other
		ms   int64
	}
	out := make([]outcome, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, derr := net.DialTimeout("tcp", addr, 2*time.Second)
			if derr != nil {
				out[i] = outcome{kind: "other: " + derr.Error()}
				return
			}
			defer func() { _ = c.Close() }()
			start := time.Now()
			_ = c.SetReadDeadline(start.Add(3 * time.Second))
			buf := make([]byte, 16)
			nr, rerr := c.Read(buf)
			ms := time.Since(start).Milliseconds()
			switch {
			case nr > 0:
				out[i] = outcome{"fellThrough", ms}
			case errors.Is(rerr, os.ErrDeadlineExceeded):
				out[i] = outcome{"open", ms}
			case errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET):
				if ms < 2000 {
					out[i] = outcome{"closed", ms}
				} else {
					out[i] = outcome{"late", ms}
				}
			default:
				out[i] = outcome{"other: " + rerr.Error(), ms}
			}
		}(i)
	}
	wg.Wait()
	counts := map[string]int{}
	for _, o := range out {
		counts[o.kind]++
	}
	if counts["closed"] != n {
		t.Errorf("want all %d silent clients closed by the server before 2s (listener_filters_timeout 1s, continue=false); got %v", n, counts)
	}
	for i, o := range out {
		if o.kind == "closed" && o.ms < 900 {
			t.Errorf("client %d closed at %d ms, before the 1s deadline", i, o.ms)
		}
	}
}

// TestParseListenerFiltersTimeoutZeroDisables pins the phase-100 0s fold: an
// EXPLICIT zero listener_filters_timeout disables the timeout (lfTimeoutMs 0,
// Pipeline.Run's no-deadline branch), as on the reference, while a NIL field
// keeps the 15000 ms default (TestParseListenerFiltersTimeoutDefault). Before
// the fold, zero and nil both read 15000.
func TestParseListenerFiltersTimeoutZeroDisables(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkListener("l_zero", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo"))
	l.ListenerFiltersTimeout = durationpb.New(0)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := mgr.runtimes[0].lfTimeoutMs; got != 0 {
		t.Errorf("lfTimeoutMs for an explicit 0s = %d, want 0 (disabled)", got)
	}
}

// TestListenerFilterTimeoutPreCxTimeoutByValue reads
// listener.<addr>.downstream_pre_cx_timeout BY NAME through the registry (a
// test naming the struct field would not compile before the counter exists)
// on a listener with the REAL tls_inspector, 1s, continue=true. The name must
// EXIST before any traffic: a missing name is a hard failure, never read as 0.
// Then, on one manager, in order:
//
//   - immediate: a client that sends at once is served and books 0;
//   - silent: a client silent for 1.3 s, then sending, is served (and its
//     fall-through connection stays live) and books exactly 1;
//   - cancel: a manager-ctx cancel during inspection ends the pipeline with
//     context.Canceled, which is NOT a timeout and books 0.
func TestListenerFilterTimeoutPreCxTimeoutByValue(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_pre_cx",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:                     []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:                  []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout:           durationpb.New(1 * time.Second),
		ContinueOnListenerFiltersTimeout: true,
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, reg, nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	name := "listener." + strings.ReplaceAll(host, ".", "_") + "_" + port + ".downstream_pre_cx_timeout"
	preCx := func() (uint64, bool) {
		var (
			v     uint64
			found bool
		)
		reg.Walk(func(m stats.Metric) {
			if c, ok := m.(*stats.Counter); ok && m.Name() == name {
				v, found = c.Load(), true
			}
		})
		return v, found
	}
	if v, ok := preCx(); !ok {
		t.Fatalf("name existence: counter %q is not registered before traffic (a missing name is a failure, never a zero)", name)
	} else if v != 0 {
		t.Fatalf("boot value: %s = %d before any traffic, want 0", name, v)
	}

	// dial opens one client connection and returns it with its dial time.
	dial := func() (net.Conn, time.Time) {
		c, derr := net.DialTimeout("tcp", addr, 2*time.Second)
		if derr != nil {
			t.Fatalf("dial: %v", derr)
		}
		return c, time.Now()
	}

	// immediate: sends at once -> classified raw_buffer, served, books 0.
	c1, _ := dial()
	if _, werr := c1.Write([]byte("hello")); werr != nil {
		t.Fatalf("immediate: write: %v", werr)
	}
	_ = c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	b := make([]byte, 1)
	if n, rerr := c1.Read(b); n != 1 || b[0] != 'A' {
		t.Errorf("immediate: want the tagged backend's 'A', got n=%d err=%v", n, rerr)
	}
	_ = c1.Close()
	if v, _ := preCx(); v != 0 {
		t.Errorf("immediate: %s = %d after a client that sent at once, want 0", name, v)
	}

	// silent: 1.3 s of silence, then a send -> the pipeline timed out at 1 s,
	// fell through (continue=true), is served, books exactly 1.
	c2, _ := dial()
	time.Sleep(1300 * time.Millisecond)
	if _, werr := c2.Write([]byte("hello")); werr != nil {
		t.Errorf("silent: write at 1.3s: %v", werr)
	}
	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, rerr := c2.Read(b); n != 1 || b[0] != 'A' {
		t.Errorf("silent: want the fall-through connection served ('A'), got n=%d err=%v", n, rerr)
	}
	// The fall-through connection must stay LIVE: no stale listener-filter
	// read deadline may survive into the tcp_proxy dispatch.
	_ = c2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, rerr := c2.Read(b); n != 0 || !errors.Is(rerr, os.ErrDeadlineExceeded) {
		t.Errorf("silent liveness: want the fall-through connection still open (client deadline), got n=%d err=%v", n, rerr)
	}
	_ = c2.Close()
	if v, _ := preCx(); v != 1 {
		t.Errorf("silent: %s = %d after one timed-out inspection, want 1", name, v)
	}

	// cancel: a manager-ctx cancel at 300 ms during inspection ends the
	// pipeline with context.Canceled (the pipeline's clock fires on the parent
	// cancel too) BEFORE the 1 s deadline; that is not a timeout: books 0.
	c3, start3 := dial()
	time.Sleep(300 * time.Millisecond)
	cancel()
	_ = c3.SetReadDeadline(start3.Add(3 * time.Second))
	n3, rerr3 := c3.Read(b)
	ms3 := time.Since(start3).Milliseconds()
	t.Logf("cancel: the server acted at %d ms (n=%d err=%v)", ms3, n3, rerr3)
	switch {
	case errors.Is(rerr3, os.ErrDeadlineExceeded):
		t.Errorf("cancel: the server never acted on the manager-ctx cancel (client deadline after %d ms)", ms3)
	case ms3 >= 900:
		t.Errorf("cancel: the server acted at %d ms — at/after the 1s deadline, not at the 300 ms cancel (n=%d err=%v)", ms3, n3, rerr3)
	}
	_ = c3.Close()
	if v, _ := preCx(); v != 1 {
		t.Errorf("cancel: %s = %d after a manager-ctx cancel, want 1 (unchanged: a cancel is not a timeout)", name, v)
	}
}

// TestListenerFilterTimeoutPreCxTimeoutOnAbort is the continue=false mirror of
// TestListenerFilterTimeoutPreCxTimeoutByValue: a timed-out inspection that
// CLOSES the connection books downstream_pre_cx_timeout too (the reference
// books it under both continue values). Read BY NAME; a missing name fails.
func TestListenerFilterTimeoutPreCxTimeoutOnAbort(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_pre_cx_abort",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:        []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, reg, nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	name := "listener." + strings.ReplaceAll(host, ".", "_") + "_" + port + ".downstream_pre_cx_timeout"
	preCx := func() (uint64, bool) {
		var (
			v     uint64
			found bool
		)
		reg.Walk(func(m stats.Metric) {
			if c, ok := m.(*stats.Counter); ok && m.Name() == name {
				v, found = c.Load(), true
			}
		})
		return v, found
	}
	if _, ok := preCx(); !ok {
		t.Fatalf("name existence: counter %q is not registered before traffic (a missing name is a failure, never a zero)", name)
	}
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	_ = c.SetReadDeadline(start.Add(3 * time.Second))
	b := make([]byte, 1)
	n, rerr := c.Read(b)
	if n != 0 || !(errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET)) || time.Since(start) >= 2*time.Second {
		t.Errorf("abort: want the SERVER to close a silent client before 2s; got n=%d err=%v after %v", n, rerr, time.Since(start))
	}
	if v, _ := preCx(); v != 1 {
		t.Errorf("abort: %s = %d after one timed-out, closed inspection (continue=false), want 1", name, v)
	}
}
```

## Appendix C.2 — `internal/listener/listenerfilter/pipeline_deadline_test.go` (209 lines: U4 i-iv). Task 3

```go
package listenerfilter

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// peekOnlyFilter mimics tls_inspector's discipline without importing it (this
// package cannot): it Peeks n bytes, IGNORES the error, never consults ctx and
// always returns Continue, nil. Only the pipeline can bound it.
type peekOnlyFilter struct{ n int }

func (f *peekOnlyFilter) Inspect(_ context.Context, p Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
	_, _ = p.Peek(f.n)
	return Continue, nil
}
func (f *peekOnlyFilter) OnDestroy() {}

// loopbackTCPPair returns a connected (peer, server) pair over REAL loopback
// TCP — never net.Pipe — so the peek reads a kernel socket.
func loopbackTCPPair(t *testing.T) (peer, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, aerr := ln.Accept()
		ch <- res{c, aerr}
	}()
	peer, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatalf("accept: %v", r.err)
	}
	return peer, r.c
}

type runResult struct {
	err     error
	elapsed time.Duration
}

// runPeekPipeline starts Pipeline.Run over a real peekerConn wrapping server
// with one peekOnlyFilter{5}; the result arrives on the returned channel.
func runPeekPipeline(server net.Conn, timeoutMs uint32) (net.Conn, <-chan runResult) {
	pc := NewPeekerConn(server)
	ch := make(chan runResult, 1)
	go func() {
		start := time.Now()
		var p Pipeline
		err := p.Run(context.Background(), []ListenerFilter{&peekOnlyFilter{n: 5}}, AsPeeker(pc), &ChainMatchInputs{}, timeoutMs)
		ch <- runResult{err, time.Since(start)}
	}()
	return pc, ch
}

// readAllBounded reads exactly n bytes from c WITHOUT setting a read
// deadline of its own (that would overwrite a stale one), bounded by a timer.
func readAllBounded(t *testing.T, c net.Conn, n int, bound time.Duration) ([]byte, error) {
	t.Helper()
	type res struct {
		b   []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		b := make([]byte, n)
		_, err := io.ReadFull(c, b)
		ch <- res{b, err}
	}()
	select {
	case r := <-ch:
		return r.b, r.err
	case <-time.After(bound):
		_ = c.Close()
		r := <-ch
		return r.b, errors.New("read did not complete within the bound")
	}
}

// TestPipelineRunDeadlineInterruptsSilentPeek: a NON-ctx-aware filter blocked
// in Peek on a silent peer must be interrupted by the pipeline at timeoutMs,
// and Run must report a timeout (an error wrapping context.DeadlineExceeded).
// Bounded: if Run has not returned by 3 s, the peer is closed (which unblocks
// the peek) and the arm FAILS rather than hangs.
func TestPipelineRunDeadlineInterruptsSilentPeek(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	var r runResult
	select {
	case r = <-ch:
	case <-time.After(3 * time.Second):
		_ = peer.Close()
		r = <-ch
		t.Errorf("deadline enforced: Run did not return within 3s of a 200 ms timeout on a silent peer (it returned only after the peer was closed, at %v)", r.elapsed)
	}
	if !errors.Is(r.err, context.DeadlineExceeded) {
		t.Errorf("timeout classification: Run err = %v, want an error wrapping context.DeadlineExceeded", r.err)
	}
	if r.elapsed < 150*time.Millisecond || r.elapsed > time.Second {
		t.Errorf("deadline window: Run returned after %v, want within [150ms, 1s] of a 200 ms timeout", r.elapsed)
	}
}

// TestPipelineRunDeadlineClearedOnSuccess: a peer that sends in time lets Run
// return nil, and no read deadline survives Run: a later Read of further peer
// bytes, well past the 200 ms budget, succeeds.
func TestPipelineRunDeadlineClearedOnSuccess(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	start := time.Now()
	time.Sleep(20 * time.Millisecond)
	if _, err := peer.Write([]byte("hello")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			t.Errorf("success path: Run err = %v after the peer sent 5 bytes at 20 ms, want nil", r.err)
		}
	case <-time.After(3 * time.Second):
		_ = peer.Close()
		<-ch
		t.Fatalf("success path: Run did not return within 3s although the peer sent 5 bytes at 20 ms")
	}
	time.Sleep(time.Until(start.Add(400 * time.Millisecond)))
	if _, err := peer.Write([]byte("more")); err != nil {
		t.Fatalf("peer write (after the budget): %v", err)
	}
	got, err := readAllBounded(t, pc, 9, 3*time.Second)
	if err != nil || string(got) != "hellomore" {
		t.Errorf("no stale deadline (success path): Read after the 200 ms budget got %q err=%v, want \"hellomore\"", got, err)
	}
}

// TestPipelineRunDeadlineClearedAfterTimeout: after the pipeline's deadline
// FIRED (Run returned a timeout), the socket must be usable again — a
// fall-through connection (continue_on_listener_filters_timeout=true) is
// read by the chain. A Read of bytes the peer sends afterwards succeeds.
// Bounded: if Run has not returned by 1 s, the peer's send unblocks it and
// the arm FAILS.
func TestPipelineRunDeadlineClearedAfterTimeout(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	select {
	case r := <-ch:
		if !errors.Is(r.err, context.DeadlineExceeded) {
			t.Errorf("timeout classification: Run err = %v, want an error wrapping context.DeadlineExceeded", r.err)
		}
		if _, err := peer.Write([]byte("hello")); err != nil {
			t.Fatalf("peer write: %v", err)
		}
	case <-time.After(time.Second):
		if _, err := peer.Write([]byte("hello")); err != nil {
			t.Fatalf("peer write: %v", err)
		}
		r := <-ch
		t.Errorf("deadline enforced: Run did not return within 1s of a 200 ms timeout on a silent peer (returned after the peer sent, at %v)", r.elapsed)
	}
	got, err := readAllBounded(t, pc, 5, 3*time.Second)
	if err != nil || string(got) != "hello" {
		t.Errorf("no stale deadline (timeout path): Read after the fired deadline got %q err=%v, want \"hello\"", got, err)
	}
}

// TestPipelineRunZeroTimeoutHoldsSilentPeek: timeoutMs 0 establishes no
// deadline, so a silent peer holds Run past 400 ms (2x a 200 ms budget); Run
// returns nil only once the peer closes.
func TestPipelineRunZeroTimeoutHoldsSilentPeek(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	pc, ch := runPeekPipeline(server, 0)
	defer func() { _ = pc.Close() }()
	select {
	case r := <-ch:
		t.Errorf("zero disables: Run returned after %v (err=%v) with timeoutMs 0 and a silent peer, want still blocked at 400 ms", r.elapsed, r.err)
		_ = peer.Close()
		return
	case <-time.After(400 * time.Millisecond):
	}
	_ = peer.Close()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Errorf("zero disables: Run err = %v after the peer closed, want nil (no deadline, a non-ctx-aware filter)", r.err)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("zero disables: Run did not return within 3s of the peer closing")
	}
}
```

## Appendix D — U2 strengthened in place, `manager_test.go` `9 3`. Task 4

```diff
diff --git a/internal/listener/manager_test.go b/internal/listener/manager_test.go
index 4723c197..532a4f5f 100644
--- a/internal/listener/manager_test.go
+++ b/internal/listener/manager_test.go
@@ -4077,16 +4077,22 @@ func TestUnifiedDispatchListenerFilterTimeoutAbortsConnection(t *testing.T) {
 	defer func() { _ = conn.Close() }()
 
 	// The listener should close the conn after the pipeline aborts (~1s).
-	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
+	start := time.Now()
+	if err := conn.SetReadDeadline(start.Add(3 * time.Second)); err != nil {
 		t.Fatalf("SetReadDeadline: %v", err)
 	}
 	buf := make([]byte, 1)
 	n, rerr := conn.Read(buf)
+	elapsed := time.Since(start)
 	if rerr == nil && n > 0 {
 		t.Errorf("expected conn closed by listener (timeout abort), got %d bytes %q", n, buf[:n])
 	}
-	// The exact error is platform-dependent (EOF, ECONNRESET, …); any non-nil
-	// err with n==0 is acceptable evidence the listener aborted.
+	// The SERVER must have closed (EOF or ECONNRESET, zero bytes) before 2s. A
+	// client-deadline expiry at 3s is NOT evidence of an abort: it is what a
+	// listener that never closes the connection looks like.
+	if n == 0 && !((errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET)) && elapsed < 2*time.Second) {
+		t.Errorf("expected the SERVER to close the conn (EOF/ECONNRESET, 0 bytes) before 2s; got err=%v after %v", rerr, elapsed)
+	}
 }
 
 // TestUnifiedDispatchListenerFilterTimeoutContinue verifies that the Task-10
```

## Appendix E.1 — `internal/stats/name.go` `2 0` (its own paragraph). Task 9

```diff
diff --git a/internal/stats/name.go b/internal/stats/name.go
index c8045fab..af0bfe48 100644
--- a/internal/stats/name.go
+++ b/internal/stats/name.go
@@ -556,6 +556,8 @@ var helpText = map[string]string{
 	"envoy_server_live":                   "1 if the server is live, 0 otherwise.",
 	"envoy_server_accesslog_dropped":      "Total access-log records dropped due to backpressure (per-process aggregate across all sinks).",
 
+	"envoy_listener_downstream_pre_cx_timeout": "Total listener-filter inspections that exceeded listener_filters_timeout.",
+
 	"envoy_listener_ssl_connection_error":    "Downstream TLS handshakes failed with an SSL protocol error.",
 	"envoy_listener_ssl_handshake":           "Total successful downstream TLS handshakes on the listener.",
 	"envoy_listener_ssl_fail_verify_error":   "Downstream TLS handshakes failed because client certificate chain verification failed.",
```

## Appendix E.2 — `internal/stats/helptext_test.go` `1 0`. Task 9

```diff
diff --git a/internal/stats/helptext_test.go b/internal/stats/helptext_test.go
index a5fecd88..d98f1422 100644
--- a/internal/stats/helptext_test.go
+++ b/internal/stats/helptext_test.go
@@ -40,6 +40,7 @@ type helpTextRosterEntry struct {
 //	sds.<secret>.*      internal/xds/stats.go RegisterSDSStats
 var helpTextRoster = []helpTextRosterEntry{
 	{internal: "listener.0_0_0_0_10000.downstream_cx_total"},
+	{internal: "listener.0_0_0_0_10000.downstream_pre_cx_timeout"},
 	{internal: "listener.0_0_0_0_10000.downstream_cx_active", gauge: true},
 	{internal: "listener.0_0_0_0_10000.ssl.connection_error"},
 	{internal: "listener.0_0_0_0_10000.ssl.handshake"},
```

## Appendix F.1 — `test/fixtures/0125-listener-filters-timeout/driver/driver.go` (727 lines). Task 5

```go
// Package driver registers the 0125-listener-filters-timeout fixture with the
// differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 100, SPEC §7): reference Envoy ENFORCES
// Listener.listener_filters_timeout. A connection whose listener filters have
// not finished inspecting when the deadline fires is CLOSED under
// continue_on_listener_filters_timeout: false, or FALLS THROUGH to chain
// selection under true; either way the listener's
// downstream_pre_cx_timeout counter books it. A timeout of 0s DISABLES the
// deadline. envoy-go (un-fixed) never enforces it: a silent client is held
// until IT acts, and the counter name does not exist.
//
// Five plaintext TCP listeners, each a ONE-LINE diff from one base (tls_inspector,
// listener_filters_timeout 1s, fc_indexed matching transport_protocol
// raw_buffer, a last-resort default_filter_chain):
//
//	listener    delta                                         pre_cx (both sides)
//	l_false     continue: false                               51 (F1 1 + F2 50 + F3 0)
//	l_true      continue: true                                1  (T2; T1 books 0)
//	l_true_tls  continue: true, fc_indexed matches "tls"      1  (T3)
//	l_zero      listener_filters_timeout: 0s, continue: false 0  (Z1: 0s disables)
//	l_nofilt    no listener_filters, continue: false          0  (N1: nothing to time)
//
// ⚠️ WHAT THIS FIXTURE DELIBERATELY DOES NOT PIN (SPEC §7.4): downstream_cx_total
// on any listener with a drop arm (the reference counts post-filter; the
// subject pre-filter), the close KIND (the reference RSTs or FINs; docker-proxy
// rewrites it on the host path anyway — it is RECORDED, never asserted), a
// partial-byte arm, a half-close arm, any exact millisecond (windows only), and
// any downstream_listener_filter_* name (the subject emits none).
//
// Every arm on a side runs CONCURRENTLY, so a side's drive costs max(arm) =
// Z1's 16.5 s once, not the sum of the arms.
package driver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0125-listener-filters-timeout"

// refAdminPort is the in-container reference admin port, fixed at 9901 by the
// harness.
const refAdminPort = 9901

// drivenPath is the path every GET arm requests. Both chains of every listener
// route "/" to a direct_response; the BODY names the chain.
const drivenPath = "/lft"

// wantStatus is the direct_response status on every chain. 200 — NOT a 1xx,
// which a Go client consumes as informational and never surfaces as final.
const wantStatus = 200

// The close window, in milliseconds from the client's dial returning.
//
// ⚠️ SET FROM MEASUREMENT, NOT FROM THE CONFIG. The configured deadline is
// 1000 ms on both sides; the window is the measured spread plus a margin on
// both sides of it (README §Window records the per-side figures and the
// σ-margin arithmetic). Widen it with a new measurement, never drop an arm.
const (
	windowLoMs = 700
	windowHiMs = 1800
)

// Arm timing.
const (
	silentHold = 3 * time.Second         // F1/F2: hold a silent client this long
	f2Conns    = 50                      // F2: concurrent silent clients
	f3Delay    = 300 * time.Millisecond  // F3: GET well inside the deadline
	tDelay     = 2500 * time.Millisecond // T2/T3: GET well after the deadline
	z1Open     = 16500 * time.Millisecond
	n1Open     = 2800 * time.Millisecond
	getTimeout = 5 * time.Second // a GET arm's read budget after its write
)

// Body tokens. The served body is "<token> <listener>\n", so one observed body
// names both the chain and the listener that produced it.
const (
	chainIndexed = "INDEXED"
	chainDefault = "DEFAULT"
)

// lspec is one listener. Order is the index-wise zip the runner performs
// between SubjectListenerNames() and ReferenceListenerPorts().
type lspec struct {
	name    string
	refPort int // in-container reference port — CENSUSED (SPEC §7.2)
	// filters: whether the listener carries the tls_inspector listener filter.
	filters bool
	// timeout is the listener_filters_timeout literal.
	timeout string
	// cont is continue_on_listener_filters_timeout.
	cont bool
	// indexedMatch is fc_indexed's filter_chain_match.transport_protocol.
	indexedMatch string
	// wantPreCx is downstream_pre_cx_timeout after the drive, BY VALUE.
	wantPreCx uint64
}

var listeners = []lspec{
	{name: "l_false", refPort: 15125, filters: true, timeout: "1s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 1 + f2Conns},
	{name: "l_true", refPort: 15228, filters: true, timeout: "1s", cont: true, indexedMatch: "raw_buffer", wantPreCx: 1},
	{name: "l_true_tls", refPort: 15229, filters: true, timeout: "1s", cont: true, indexedMatch: "tls", wantPreCx: 1},
	{name: "l_zero", refPort: 15230, filters: true, timeout: "0s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 0},
	{name: "l_nofilt", refPort: 15231, filters: false, timeout: "1s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 0},
}

func init() { fixture.RegisterFixture(fixtureName, &lftDriver{}) }

// lftDriver is STATEFUL: each side's Drive records its observations so
// AssertStats can assert every arm absolutely, per side, one Errorf per
// property. A Drive-time error would become t.Fatalf and MASK every later arm.
type lftDriver struct {
	mu    sync.Mutex
	obs   map[string]*sideObs // side -> observations
	addrs map[string]map[string]string
}

// closeObs is one silent client's outcome.
type closeObs struct {
	closed bool   // the server ended the connection before the hold elapsed
	ms     int64  // ms from dial-return to the close (or to the hold's end)
	bytes  int    // bytes the server sent before closing
	kind   string // FIN / RST / other:<err> / open / dial:<err> — RECORDED, not pinned
}

// getObs is one GET arm's outcome.
type getObs struct {
	status int
	body   string
	err    string
}

type sideObs struct {
	f1     closeObs
	f2     []closeObs
	z1, n1 closeObs
	get    map[string]getObs // arm -> outcome (F3, T1, T2, T3)
}

// getArm is one timed GET: which listener, how long to stay silent first, and
// the chain whose body must answer.
type getArm struct {
	id        string
	listener  string
	delay     time.Duration
	wantChain string
}

var getArms = []getArm{
	{id: "F3", listener: "l_false", delay: f3Delay, wantChain: chainIndexed},
	{id: "T1", listener: "l_true", delay: 0, wantChain: chainIndexed},
	{id: "T2", listener: "l_true", delay: tDelay, wantChain: chainIndexed},
	{id: "T3", listener: "l_true_tls", delay: tDelay, wantChain: chainDefault},
}

var (
	_ fixture.Driver              = (*lftDriver)(nil)
	_ fixture.MultiListenerDriver = (*lftDriver)(nil)
	_ fixture.StatsAsserter       = (*lftDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: a never-dialed placeholder (the runner rejects 0 and
// envoy-go boot-rejects an absent static_resources.clusters key).
func (*lftDriver) BackendCount() int { return 1 }

func (*lftDriver) SubjectListenerName() string { return listeners[0].name }

func (*lftDriver) ReferenceListenerPort() int { return listeners[0].refPort }

func (*lftDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig binds the five subject listeners at subjListenerPort+0..+4,
// inside the runner's probed 16-port block.
func (*lftDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject are UNREACHABLE while MultiListenerDriver is
// implemented; they refuse rather than derive sibling addresses.
func (*lftDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, errors.New("0125 drives only through DriveReferenceMulti")
}

func (*lftDriver) DriveSubject(context.Context, string) ([]byte, error) {
	return nil, errors.New("0125 drives only through DriveSubjectMulti")
}

func (*lftDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

func (*lftDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*lftDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *lftDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "ref", addrs)
}

func (d *lftDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "subj", addrs)
}

// drive runs EVERY arm on one side concurrently and emits a side-independent,
// timing-free verdict stream for the runner's CompareBytes. Only a missing
// address returns an error; every per-arm failure is recorded and asserted in
// AssertStats.
func (d *lftDriver) drive(ctx context.Context, side string, addrs map[string]string) ([]byte, error) {
	for _, l := range listeners {
		if addrs[l.name] == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
	}
	o := &sideObs{f2: make([]closeObs, f2Conns), get: map[string]getObs{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	start := time.Now()

	wg.Add(1)
	go func() { defer wg.Done(); o.z1 = silentProbe(ctx, addrs["l_zero"], z1Open) }()
	wg.Add(1)
	go func() { defer wg.Done(); o.n1 = silentProbe(ctx, addrs["l_nofilt"], n1Open) }()
	wg.Add(1)
	go func() { defer wg.Done(); o.f1 = silentProbe(ctx, addrs["l_false"], silentHold) }()
	for i := 0; i < f2Conns; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); o.f2[i] = silentProbe(ctx, addrs["l_false"], silentHold) }(i)
	}
	for _, a := range getArms {
		wg.Add(1)
		go func(a getArm) {
			defer wg.Done()
			g := getProbe(ctx, addrs[a.listener], a.delay)
			mu.Lock()
			o.get[a.id] = g
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	log.Printf("%s: %s drive wall time %d ms", fixtureName, side, time.Since(start).Milliseconds())

	d.mu.Lock()
	if d.obs == nil {
		d.obs = map[string]*sideObs{}
		d.addrs = map[string]map[string]string{}
	}
	d.obs[side] = o
	d.addrs[side] = addrs
	d.mu.Unlock()

	logTimings(side, o)

	var b bytes.Buffer
	fmt.Fprintf(&b, "F1 l_false %s\n", closeVerdict(o.f1))
	inWin, totalBytes := 0, 0
	for _, c := range o.f2 {
		if c.closed && inWindow(c.ms) {
			inWin++
		}
		totalBytes += c.bytes
	}
	fmt.Fprintf(&b, "F2 l_false closed_in_window=%d/%d bytes=%d\n", inWin, f2Conns, totalBytes)
	for _, a := range getArms {
		g := o.get[a.id]
		fmt.Fprintf(&b, "%s %s status=%d body=%q err=%q\n", a.id, a.listener, g.status, g.body, g.err)
	}
	fmt.Fprintf(&b, "Z1 l_zero open_at_%dms=%t\n", z1Open.Milliseconds(), !o.z1.closed && o.z1.kind == "open")
	fmt.Fprintf(&b, "N1 l_nofilt open_at_%dms=%t\n", n1Open.Milliseconds(), !o.n1.closed && o.n1.kind == "open")
	return b.Bytes(), nil
}

func inWindow(ms int64) bool { return ms >= windowLoMs && ms <= windowHiMs }

// closeVerdict is the timing-free, side-independent verdict for one silent arm.
func closeVerdict(c closeObs) string {
	return fmt.Sprintf("closed=%t in_window=%t bytes=%d", c.closed, c.closed && inWindow(c.ms), c.bytes)
}

// silentProbe dials addr, sends NOTHING, and reads until the server closes or
// hold elapses. The clock starts when the dial returns. The close kind is
// RECORDED (FIN = io.EOF, RST = ECONNRESET) and never asserted.
func silentProbe(ctx context.Context, addr string, hold time.Duration) closeObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return closeObs{kind: "dial:" + err.Error()}
	}
	t0 := time.Now()
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(t0.Add(hold))
	buf := make([]byte, 4096)
	n := 0
	for {
		k, rerr := c.Read(buf)
		n += k
		if rerr == nil {
			continue
		}
		ms := time.Since(t0).Milliseconds()
		var ne net.Error
		switch {
		case errors.As(rerr, &ne) && ne.Timeout():
			return closeObs{closed: false, ms: ms, bytes: n, kind: "open"}
		case errors.Is(rerr, io.EOF):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "FIN"}
		case errors.Is(rerr, syscall.ECONNRESET):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "RST"}
		default:
			return closeObs{closed: true, ms: ms, bytes: n, kind: "other:" + rerr.Error()}
		}
	}
}

// getProbe dials addr, stays silent for delay, then sends one HTTP/1.1 GET and
// reads the response. A connection the server closed during the silence
// surfaces as a recorded error, not a returned one.
func getProbe(ctx context.Context, addr string, delay time.Duration) getObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return getObs{err: "dial: " + err.Error()}
	}
	defer func() { _ = c.Close() }()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return getObs{err: "ctx: " + ctx.Err().Error()}
		}
	}
	_ = c.SetDeadline(time.Now().Add(getTimeout))
	req := "GET " + drivenPath + " HTTP/1.1\r\nHost: lft\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(c, req); err != nil {
		return getObs{err: "write: " + err.Error()}
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return getObs{err: "read: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return getObs{status: resp.StatusCode, body: string(body), err: "body: " + err.Error()}
	}
	return getObs{status: resp.StatusCode, body: string(body)}
}

// logTimings RECORDS every silent connection's raw outcome and the F1+F2 close
// spread, pass or fail, so a green run still leaves the measurement the window
// is set from. fixture.TB has no Logf.
func logTimings(side string, o *sideObs) {
	log.Printf("%s: %s F1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.f1.closed, o.f1.ms, o.f1.bytes, o.f1.kind)
	kinds := map[string]int{}
	var ms []float64
	if o.f1.closed {
		ms = append(ms, float64(o.f1.ms))
	}
	for i, c := range o.f2 {
		log.Printf("%s: %s F2[%02d] closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, i, c.closed, c.ms, c.bytes, c.kind)
		kinds[c.kind]++
		if c.closed {
			ms = append(ms, float64(c.ms))
		}
	}
	log.Printf("%s: %s Z1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.z1.closed, o.z1.ms, o.z1.bytes, o.z1.kind)
	log.Printf("%s: %s N1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.n1.closed, o.n1.ms, o.n1.bytes, o.n1.kind)
	for _, a := range getArms {
		g := o.get[a.id]
		log.Printf("%s: %s %s %s status=%d body=%q err=%q", fixtureName, side, a.id, a.listener, g.status, g.body, g.err)
	}
	kk := make([]string, 0, len(kinds))
	for k, v := range kinds {
		kk = append(kk, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(kk)
	if len(ms) == 0 {
		log.Printf("%s: %s F1+F2 SPREAD n=0 (no close observed) F2 kinds %s", fixtureName, side, strings.Join(kk, ","))
		return
	}
	lo, hi, sum := ms[0], ms[0], 0.0
	for _, v := range ms {
		lo, hi, sum = math.Min(lo, v), math.Max(hi, v), sum+v
	}
	mean := sum / float64(len(ms))
	ss := 0.0
	for _, v := range ms {
		ss += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(ss / float64(len(ms)))
	log.Printf("%s: %s F1+F2 SPREAD n=%d min=%.0f max=%.0f mean=%.1f sd=%.2f F2 kinds %s",
		fixtureName, side, len(ms), lo, hi, mean, sd, strings.Join(kk, ","))
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, every arm absolutely and the per-listener
// downstream_pre_cx_timeout BY VALUE, one Errorf per property. Fatalf is
// reserved for a broken precondition (a failed scrape, an unrecorded drive).
func (d *lftDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	d.mu.Lock()
	obs, addrs := d.obs, d.addrs
	d.mu.Unlock()
	for _, side := range []struct{ name, admin string }{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		o := obs[side.name]
		if o == nil {
			t.Fatalf("%s: no drive observations recorded — every assertion would be vacuous", side.name)
			return
		}
		assertArms(t, side.name, o)
		prom, err := scrapePromByAddr(side.admin)
		if err != nil {
			t.Fatalf("%s: scrape /stats/prometheus: %v", side.name, err)
			return
		}
		for i, l := range listeners {
			label := listenerLabel(side.name, l, addrs[side.name][l.name])
			assertPreCx(t, side.name, i, l, label, prom)
		}
	}
}

func assertArms(t fixture.TB, side string, o *sideObs) {
	t.Helper()
	// F1 — one silent client on l_false: closed, in window, zero bytes.
	if !o.f1.closed {
		t.Errorf("%s F1 l_false: silent client NOT closed by the server within %d ms (kind=%s) — "+
			"listener_filters_timeout 1s with continue:false must close it", side, silentHold.Milliseconds(), o.f1.kind)
	} else {
		if !inWindow(o.f1.ms) {
			t.Errorf("%s F1 l_false: closed at %d ms, want within [%d, %d] ms", side, o.f1.ms, windowLoMs, windowHiMs)
		}
		if o.f1.bytes != 0 {
			t.Errorf("%s F1 l_false: server sent %d bytes before closing, want 0", side, o.f1.bytes)
		}
	}
	// F2 — 50 concurrent silent clients: EVERY one closed in the window.
	var notClosed, outWin, withBytes []string
	for i, c := range o.f2 {
		switch {
		case !c.closed:
			notClosed = append(notClosed, fmt.Sprintf("#%d(%s@%dms)", i, c.kind, c.ms))
		case !inWindow(c.ms):
			outWin = append(outWin, fmt.Sprintf("#%d@%dms", i, c.ms))
		}
		if c.bytes != 0 {
			withBytes = append(withBytes, fmt.Sprintf("#%d:%dB", i, c.bytes))
		}
	}
	if len(notClosed) > 0 {
		t.Errorf("%s F2 l_false: %d of %d concurrent silent clients NOT closed within %d ms: %s",
			side, len(notClosed), f2Conns, silentHold.Milliseconds(), strings.Join(notClosed, " "))
	}
	if len(outWin) > 0 {
		t.Errorf("%s F2 l_false: %d of %d closes outside [%d, %d] ms: %s",
			side, len(outWin), f2Conns, windowLoMs, windowHiMs, strings.Join(outWin, " "))
	}
	if len(withBytes) > 0 {
		t.Errorf("%s F2 l_false: %d connections received bytes, want 0 each: %s", side, len(withBytes), strings.Join(withBytes, " "))
	}
	// F3 / T1 / T2 / T3 — the served chain, by body.
	for _, a := range getArms {
		g, ok := o.get[a.id]
		if !ok {
			t.Errorf("%s %s %s: no observation recorded", side, a.id, a.listener)
			continue
		}
		if g.err != "" {
			t.Errorf("%s %s %s: GET after %d ms failed: %s", side, a.id, a.listener, a.delay.Milliseconds(), g.err)
			continue
		}
		if g.status != wantStatus {
			t.Errorf("%s %s %s: status %d, want %d", side, a.id, a.listener, g.status, wantStatus)
		}
		if want := bodyFor(a.wantChain, a.listener); g.body != want {
			t.Errorf("%s %s %s: body %q, want %q", side, a.id, a.listener, g.body, want)
		}
	}
	// Z1 — 0s disables: still open at 16.5 s. N1 — no filters: still open.
	if o.z1.closed || o.z1.kind != "open" {
		t.Errorf("%s Z1 l_zero: silent client ended at %d ms (kind=%s), want still open at %d ms — "+
			"listener_filters_timeout 0s disables the deadline", side, o.z1.ms, o.z1.kind, z1Open.Milliseconds())
	}
	if o.n1.closed || o.n1.kind != "open" {
		t.Errorf("%s N1 l_nofilt: silent client ended at %d ms (kind=%s), want still open at %d ms — "+
			"a listener with no listener filters has nothing to time out", side, o.n1.ms, o.n1.kind, n1Open.Milliseconds())
	}
}

// assertPreCx pins downstream_pre_cx_timeout BY VALUE on this listener's own
// address label. A MISSING series is a hard failure, never read as 0.
func assertPreCx(t fixture.TB, side string, _ int, l lspec, label string, prom map[string]map[string]uint64) {
	t.Helper()
	const metric = "envoy_listener_downstream_pre_cx_timeout"
	series, ok := prom[metric][label]
	log.Printf("%s: %s S %s %s{envoy_listener_address=%q} = %d (present=%t) want %d",
		fixtureName, side, l.name, metric, label, series, ok, l.wantPreCx)
	if !ok {
		have := make([]string, 0, len(prom[metric]))
		for k := range prom[metric] {
			have = append(have, k)
		}
		sort.Strings(have)
		t.Errorf("%s S %s: %s{envoy_listener_address=%q} ABSENT (series present: %v), want present and == %d",
			side, l.name, metric, label, have, l.wantPreCx)
		return
	}
	if series != l.wantPreCx {
		t.Errorf("%s S %s: %s = %d, want %d", side, l.name, metric, series, l.wantPreCx)
	}
}

// listenerLabel is each side's OWN envoy_listener_address spelling: the
// reference renders "0.0.0.0_<in-container port>"; envoy-go renders its
// configured bind address with ':' and '.' folded to '_'.
func listenerLabel(side string, l lspec, addr string) string {
	if side == "ref" {
		return "0.0.0.0_" + strconv.Itoa(l.refPort)
	}
	return strings.NewReplacer(":", "_", ".", "_").Replace(addr)
}

func bodyFor(chain, listener string) string { return chain + " " + listener + "\n" }

// scrapePromByAddr fetches /stats/prometheus and returns
// metric -> envoy_listener_address -> value, keeping ONLY series that carry an
// envoy_listener_address label.
func scrapePromByAddr(adminAddr string) (map[string]map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	const key = `envoy_listener_address="`
	out := map[string]map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		open := strings.IndexByte(line, '{')
		closeIdx := strings.LastIndexByte(line, '}')
		if strings.HasPrefix(line, "#") || open < 0 || closeIdx < open {
			continue
		}
		labels := line[open+1 : closeIdx]
		ki := strings.Index(labels, key)
		if ki < 0 {
			continue
		}
		val := labels[ki+len(key):]
		end := strings.IndexByte(val, '"')
		if end < 0 {
			continue
		}
		rest := strings.Fields(line[closeIdx+1:])
		if len(rest) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(rest[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		name := line[:open]
		if out[name] == nil {
			out[name] = map[string]uint64{}
		}
		out[name][val[:end]] += uint64(v)
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap renders BOTH sides from one template, differing only in bind
// address, admin port and listener ports.
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bootstrapHeadTmpl, bindAddr, adminPort)
	for i, l := range listeners {
		lf := ""
		if l.filters {
			lf = listenerFiltersBlock
		}
		fmt.Fprintf(&b, listenerTmpl,
			l.name,         // 1
			bindAddr,       // 2
			portFor(i),     // 3
			l.timeout,      // 4
			l.cont,         // 5
			lf,             // 6
			l.indexedMatch, // 7
			strconv.Quote(bodyFor(chainIndexed, l.name)), // 8
			strconv.Quote(bodyFor(chainDefault, l.name)), // 9
			strings.TrimPrefix(l.name, "l_"),             // 10 stat_prefix stem
		)
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

const bootstrapHeadTmpl = `admin:
  address:
    socket_address: { address: %s, port_value: %d }
static_resources:
  listeners:
`

const listenerFiltersBlock = `      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
`

// listenerTmpl: stat_prefixes are <stem>_indexed / <stem>_default — ALL TEN
// DISTINCT (two HCMs sharing a stat_prefix panic envoy-go at boot).
const listenerTmpl = `    - name: %[1]s
      address:
        socket_address: { address: %[2]s, port_value: %[3]d }
      listener_filters_timeout: %[4]s
      continue_on_listener_filters_timeout: %[5]t
%[6]s      filter_chains:
        - name: fc_indexed
          filter_chain_match:
            transport_protocol: %[7]s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %[10]s_indexed
                route_config:
                  name: rc_%[10]s_indexed
                  virtual_hosts:
                    - name: vh_%[10]s_indexed
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: %[8]s }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: %[10]s_default
              route_config:
                name: rc_%[10]s_default
                virtual_hosts:
                  - name: vh_%[10]s_default
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 200
                          body: { inline_string: %[9]s }
              http_filters:
                - name: envoy.filters.http.router
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

const clustersTmpl = `  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
```

## Appendix F.2 — `test/fixtures/0125-listener-filters-timeout/README.md` (79 lines). Task 6

```markdown
# 0125-listener-filters-timeout

Cross-side differential for phase 100 (`listener-filters-timeout-enforce`):
reference Envoy **enforces** `Listener.listener_filters_timeout`. A connection
whose listener filters have not finished inspecting at the deadline is **closed**
under `continue_on_listener_filters_timeout: false`, or **falls through** to chain
selection under `true`; either way the listener's `downstream_pre_cx_timeout`
counter books it. `0s` **disables** the deadline. The un-fixed envoy-go never
enforces it: a silent client is held until the client acts, and the counter name
does not exist.

## Shape

Like `0123`: no YAML, no PKI, no `inputs/`. `renderBootstrap` in
`driver/driver.go` builds **both** sides from one template; only the bind
address, the admin port and the five listener ports differ cross-side.

Every listener is a **one-line diff** from one base: `tls_inspector`,
`listener_filters_timeout: 1s`, `fc_indexed` matching
`transport_protocol: raw_buffer` (body `INDEXED <listener>`), and a last-resort
`default_filter_chain` (body `DEFAULT <listener>`). All ten HCM stat prefixes are
distinct (a shared prefix panics envoy-go at boot).

| listener | delta | reference port | `downstream_pre_cx_timeout` |
|---|---|---|---|
| `l_false` | `continue…: false` | 15125 | **51** = F1 1 + F2 50 + F3 0 |
| `l_true` | `continue…: true` | 15228 | **1** (T2; T1 books 0) |
| `l_true_tls` | `true`, `fc_indexed` matches `tls` | 15229 | **1** (T3) |
| `l_zero` | `listener_filters_timeout: 0s` | 15230 | **0** |
| `l_nofilt` | no `listener_filters` | 15231 | **0** |

`BackendCount() == 1`: a `c_unused` STATIC cluster no route dials (the runner
rejects 0; envoy-go boot-rejects an absent `clusters` key).

## Arms (every arm on a side runs concurrently — one ~16.5 s drive per side)

| arm | listener | client | pin |
|---|---|---|---|
| F1 | `l_false` | silent, hold 3 s | server closed in [700, 1800] ms, 0 bytes |
| F2 | `l_false` | 50 concurrent silent, hold 3 s | **all 50** closed in [700, 1800] ms, 0 bytes each |
| F3 | `l_false` | GET at 300 ms | 200, `INDEXED l_false` |
| T1 | `l_true` | GET at 0 | 200, `INDEXED l_true` |
| T2 | `l_true` | silent 2500 ms, then GET | 200, `INDEXED l_true` |
| T3 | `l_true_tls` | silent 2500 ms, then GET | 200, `DEFAULT l_true_tls` |
| Z1 | `l_zero` | silent | still open at 16.5 s |
| N1 | `l_nofilt` | silent | still open at 2.8 s |
| S | all five | — | `downstream_pre_cx_timeout` BY VALUE on each side's own `envoy_listener_address` label on `/stats/prometheus`; a **missing** series is a hard failure |

The reference label is `0.0.0.0_<in-container port>`; the subject label is its
bind address with `:` and `.` folded to `_` (`127_0_0_1_<port>`).

## Window — MEASURED through the harness's host `-p` path

The clock starts when the client's dial returns. Pooled over every run of the
PLAN measurement (tip, PB1 x3, NC1 x2, NC2, NC3, NC8):

| side | n | min | max | mean | σ | (mean-700)/σ | (1800-mean)/σ |
|---|---|---|---|---|---|---|---|
| reference (via docker-proxy) | 459 | 1000 | 1004 | 1002.0 | 0.92 | 328 | 867 |
| subject (PB1-based runs) | 347 | 1000 | 1021 | 1001.5 | 4.22 | 71 | 189 |

`[700, 1800]` holds with far more than the 4-5σ the band rule asks for. The
reference's close through docker-proxy was **FIN in 459 of 459** connections
(recorded, never pinned).

## Not pinned (SPEC §7.4)

`downstream_cx_total` on a drop listener (reference counts post-filter, subject
pre-filter); the close kind; a partial-byte arm; a half-close arm; any exact
millisecond; any `downstream_listener_filter_*` name.

## Negative controls (measured on PB1)

| NC | mutation | red arms |
|---|---|---|
| NC1 | P0: a second socket clock | F1, F2 (33 / 26 of 50 held open), S `l_false` (17 / 24), CompareBytes; S `l_true`/`l_true_tls` on one run of two |
| NC2 | `pre_cx` `Inc` deleted | S `l_false`, `l_true`, `l_true_tls` |
| NC3 | `0s` fold reverted | Z1 (FIN at 15012 ms), S `l_zero` (1), CompareBytes |
| NC8 | `pre_cx` only under `true` | S `l_false` (0) |
```

## Appendix F.3 — `test/fixtures/0125-listener-filters-timeout/expectations.yaml` (30 lines). Task 6

```yaml
# Phase 100 fixture 0125-listener-filters-timeout expectations (ADR-0019 —
# prose; the enforcers are driver/driver.go's per-side AssertStats arms, the
# runner's cross-side CompareBytes of the timing-free verdict stream, and the
# runner's admin probe. Nothing reads this file.)
#
# Reference: envoyproxy/envoy by digest (docs/envoy-go/ENVOY_TARGET.md);
# listeners 0.0.0.0:15125, :15228, :15229, :15230, :15231.
# Subject: envoy-go, listeners 127.0.0.1:<subjListenerPort>+0..+4.

window_ms: { lo: 700, hi: 1800 }   # from dial-return; measured 1000-1004 ref, 1000-1021 subj

listeners:
  l_false:    { timeout: 1s, continue: false, filters: [tls_inspector], fc_indexed: raw_buffer, pre_cx_timeout: 51 }
  l_true:     { timeout: 1s, continue: true,  filters: [tls_inspector], fc_indexed: raw_buffer, pre_cx_timeout: 1 }
  l_true_tls: { timeout: 1s, continue: true,  filters: [tls_inspector], fc_indexed: tls,        pre_cx_timeout: 1 }
  l_zero:     { timeout: 0s, continue: false, filters: [tls_inspector], fc_indexed: raw_buffer, pre_cx_timeout: 0 }
  l_nofilt:   { timeout: 1s, continue: false, filters: [],              fc_indexed: raw_buffer, pre_cx_timeout: 0 }

arms:
  F1: { listener: l_false,    client: silent hold 3s,               expect: closed in window, 0 bytes }
  F2: { listener: l_false,    client: 50 concurrent silent hold 3s, expect: all 50 closed in window, 0 bytes }
  F3: { listener: l_false,    client: GET at 300ms,                 expect: 200 "INDEXED l_false\n" }
  T1: { listener: l_true,     client: GET at 0,                     expect: 200 "INDEXED l_true\n" }
  T2: { listener: l_true,     client: silent 2500ms then GET,       expect: 200 "INDEXED l_true\n" }
  T3: { listener: l_true_tls, client: silent 2500ms then GET,       expect: 200 "DEFAULT l_true_tls\n" }
  Z1: { listener: l_zero,     client: silent,                       expect: open at 16.5s }
  N1: { listener: l_nofilt,   client: silent,                       expect: open at 2.8s }
  S:  { metric: envoy_listener_downstream_pre_cx_timeout, keyed_by: envoy_listener_address, missing: hard failure }

not_pinned: [downstream_cx_total, close kind, partial-byte arm, half-close arm, exact ms, downstream_listener_filter_*]
```

## Appendix F.4 — `test/differential/runner_test.go` `1 0`. Task 6

```diff
diff --git a/test/differential/runner_test.go b/test/differential/runner_test.go
index 5ea037b5..69934bf9 100644
--- a/test/differential/runner_test.go
+++ b/test/differential/runner_test.go
@@ -149,6 +149,7 @@ import (
 	_ "github.com/pgdad/envoy-go/test/fixtures/0122-quic-chain-selection/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0123-listener-transport-protocol/driver"
 	_ "github.com/pgdad/envoy-go/test/fixtures/0124-listener-sni-longest-suffix/driver"
+	_ "github.com/pgdad/envoy-go/test/fixtures/0125-listener-filters-timeout/driver"
 	"github.com/pgdad/envoy-go/test/helpers"
 
 	// Blank-imported so the lua filter's init() boot-registration fires for
```

## Appendix G — `layout-gate.sh` (§6). Tasks 8, 11

```bash
#!/bin/bash
# Phase-100 layout gate (SPEC §4.1 (e)). usage: layout-gate.sh <worktree> [base-ref]
# Compares the WORKING TREE of <worktree> against base-ref (default: merge-base HEAD master).
# Prints one PASS/FAIL line per sub-gate; exit status = number of failed sub-gates.
W=${1:?worktree}
BASE=${2:-$(git -C "$W" merge-base HEAD master)}
P=internal/listener/listenerfilter/pipeline.go
T=internal/listener/listenerfilter/tls_inspector/tls_inspector.go
fail=0
ok()  { echo "PASS ($1) $2"; }
bad() { echo "FAIL ($1) $2"; fail=$((fail+1)); }

# (a) the OnDestroy defer (33-37) and the WithTimeout line (43) are byte-identical to base.
a_base=$(git -C "$W" show "$BASE:$P" | sed -n '33,37p;43p' | sha256sum)
a_now=$(sed -n '33,37p;43p' "$W/$P" | sha256sum)
if [ "$a_base" = "$a_now" ]; then ok a "pipeline.go:33-37,43 byte-identical to $BASE"; else bad a "pipeline.go:33-37,43 differ from $BASE"; diff <(git -C "$W" show "$BASE:$P" | sed -n '33,37p;43p') <(sed -n '33,37p;43p' "$W/$P") | sed 's/^/    /'; fi

# (b) the import block of pipeline.go is unchanged (no new import).
imp() { awk '/^import \(/{f=1} f{print} f&&/^\)/{exit}'; }
b_base=$(git -C "$W" show "$BASE:$P" | imp)
b_now=$(imp < "$W/$P")
if [ -n "$b_now" ] && [ "$b_base" = "$b_now" ]; then ok b "pipeline.go import block unchanged"; else bad b "pipeline.go import block changed"; diff <(echo "$b_base") <(echo "$b_now") | sed 's/^/    /'; fi

# (c) exactly ONE non-comment context.WithTimeout under internal/listener/ (non-test), at pipeline.go:43.
#     Comment hits are dropped: at the tip the raw grep reads THREE lines (pipeline.go:21 and
#     manager.go:476 are prose), so the SPEC's literal "exactly one hit" never holds.
c_hits=$(git -C "$W" grep -n 'context.WithTimeout' -- internal/listener/ ':!*_test.go' | awk -F: '{l=$0; sub(/^[^:]*:[^:]*:/,"",l); sub(/^[ \t]+/,"",l); if (l !~ /^\/\//) print $1":"$2}')
if [ "$c_hits" = "$P:43" ]; then ok c "one code WithTimeout, at $P:43"; else bad c "code WithTimeout hits = [$(echo $c_hits)] want [$P:43]"; fi

# (d) tls_inspector.go: numstat exactly "3 3" and every changed line is a // comment.
d_num=$(git -C "$W" diff --numstat "$BASE" -- "$T" | awk '{print $1" "$2}')
d_code=$(git -C "$W" diff -U0 "$BASE" -- "$T" | /usr/bin/grep -E '^[+-]' | /usr/bin/grep -vE '^(\+\+\+|---) ' | sed -E 's/^[+-][[:space:]]*//' | /usr/bin/grep -vc '^//')
if [ "$d_num" = "3 3" ] && [ "$d_code" = 0 ]; then ok d "tls_inspector.go numstat 3 3, comment-only"; else bad d "tls_inspector.go numstat [$d_num] (want 3 3), non-comment changed lines = $d_code (want 0)"; fi

# (e) pipeline.go -U0 hunks: any hunk whose change starts at/above line 44 is line-count-neutral.
e_bad=$(git -C "$W" diff -U0 "$BASE" -- "$P" | /usr/bin/grep -E '^@@ ' | awk '{
  split(substr($2,2),o,","); split(substr($3,2),n,",");
  a=o[1]+0; b=(2 in o)?o[2]+0:1; c=n[1]+0; d=(2 in n)?n[2]+0:1;
  s=(b==0)?a+1:a;
  if (s<=44 && b!=d) printf "[-%d,%d +%d,%d] ", a, b, c, d }')
if [ -z "$e_bad" ]; then ok e "no line-count-changing pipeline.go hunk at/above line 44"; else bad e "pipeline.go hunk(s) above line 45 change the line count: $e_bad"; fi

echo "layout-gate: $fail failed sub-gate(s) (base $BASE)"
exit $fail
```

## Appendix H.1 — NC1 (relative to PB1; marker `NC1`). Task 12

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index 4a8b5954..5b3f309c 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -42,21 +42,6 @@ func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Pee
 		var cancel context.CancelFunc
 		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
 		defer cancel()
-		// ONE clock: the socket read is interrupted only AFTER ctx is done, so
-		// an interrupted Peek always observes ctx.Err() != nil below.
-		if ds, ok := peeker.(interface{ SetReadDeadline(time.Time) error }); ok {
-			fired := make(chan struct{})
-			stop := context.AfterFunc(ctx, func() {
-				_ = ds.SetReadDeadline(time.Unix(1, 0))
-				close(fired)
-			})
-			defer func() {
-				if !stop() {
-					<-fired
-				}
-				_ = ds.SetReadDeadline(time.Time{})
-			}()
-		}
 	}
 	for i, f := range filters {
 		status, err := f.Inspect(ctx, peeker, inputs)
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..b7353de5 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -1352,7 +1352,12 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 
 	// (4) Run listener-filter pipeline.
 	var p listenerfilter.Pipeline
-	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
+	if rt.lfTimeoutMs > 0 { // NC1: P0 — a second clock on the socket
+		_ = raw.SetReadDeadline(time.Now().Add(time.Duration(rt.lfTimeoutMs) * time.Millisecond))
+	}
+	err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs)
+	_ = raw.SetReadDeadline(time.Time{})
+	if err != nil {
 		if errors.Is(err, context.DeadlineExceeded) {
 			rt.downstreamPreCxTimeout.Inc()
 		}
```

## Appendix H.2 — NC2 (relative to PB1; marker `NC2`). Task 12

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..ca5ce58e 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -1354,7 +1354,7 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 	var p listenerfilter.Pipeline
 	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
 		if errors.Is(err, context.DeadlineExceeded) {
-			rt.downstreamPreCxTimeout.Inc()
+			_ = rt.downstreamPreCxTimeout // NC2: Inc deleted
 		}
 		if !rt.continueOnLfTimeout {
 			log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, err)
```

## Appendix H.3 — NC3 (relative to PB1; marker `NC3`). Task 12

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..3c155894 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -956,7 +956,7 @@ func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, e
 		return defaultMs, nil
 	}
 	if d.GetSeconds() == 0 && d.GetNanos() == 0 {
-		return 0, nil
+		return defaultMs, nil // NC3: fold reverted
 	}
 	total := d.AsDuration()
 	ms := total / time.Millisecond
```

## Appendix H.4 — NC4 (relative to PB1; marker `NC4`). Task 12

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index 4a8b5954..e3b73093 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -54,7 +54,7 @@ func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Pee
 				if !stop() {
 					<-fired
 				}
-				_ = ds.SetReadDeadline(time.Time{})
+				// NC4: deadline clear deleted
 			}()
 		}
 	}
```

## Appendix H.5 — NC5 (relative to PB1; marker `NC5`). Task 12

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index 4a8b5954..1797a62a 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -51,9 +51,8 @@ func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Pee
 				close(fired)
 			})
 			defer func() {
-				if !stop() {
-					<-fired
-				}
+				_ = stop() // NC5: no wait for a fired callback
+				_ = fired
 				_ = ds.SetReadDeadline(time.Time{})
 			}()
 		}
```

## Appendix H.6 — NC6 (relative to PB1; marker `NC6`). Task 12

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..68327832 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -1358,7 +1358,7 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 		}
 		if !rt.continueOnLfTimeout {
 			log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, err)
-			_ = pkConn.Close()
+			// NC6: abort-branch close deleted
 			return
 		}
 		// continue_on_listener_filters_timeout=true: fall through with partial inputs.
```

## Appendix H.7 — NC7 (relative to PB1; marker `NC7`). Task 12

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..0fa3b7ce 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -1353,7 +1353,7 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 	// (4) Run listener-filter pipeline.
 	var p listenerfilter.Pipeline
 	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
-		if errors.Is(err, context.DeadlineExceeded) {
+		if err != nil { // NC7: any error counts
 			rt.downstreamPreCxTimeout.Inc()
 		}
 		if !rt.continueOnLfTimeout {
```

## Appendix H.8 — NC8 (relative to PB1; marker `NC8`). Task 12

```diff
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index b784b72a..cbccbc86 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -1353,7 +1353,7 @@ func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
 	// (4) Run listener-filter pipeline.
 	var p listenerfilter.Pipeline
 	if err := p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs); err != nil {
-		if errors.Is(err, context.DeadlineExceeded) {
+		if errors.Is(err, context.DeadlineExceeded) && rt.continueOnLfTimeout { // NC8: only under true
 			rt.downstreamPreCxTimeout.Inc()
 		}
 		if !rt.continueOnLfTimeout {
```
