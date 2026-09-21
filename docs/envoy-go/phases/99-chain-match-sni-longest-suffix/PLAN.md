# Phase 99 — `chain-match-sni-longest-suffix` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Stage:** PLAN (lifecycle **2 -> 3**). Worktree `/home/esa/git/envoy-go-wt-99-plan` off master `ebc3fc0c`,
branch `wt-phase-99-plan`.
**Spec:** `docs/envoy-go/phases/99-chain-match-sni-longest-suffix/SPEC.md` (697 lines). The plan argues from
the spec, and executors read both. **§0 below records nine findings against the SPEC, five of them outright refutations; where the two
disagree, this PLAN wins, and it says why.**
**Evidence only:** `BRAINSTORM.md` (537 lines). Its `22 0` prototype is refuted (`SPEC.md` §0.1).

**Goal.** When several filter chains tie on the specificity bitmask and their `server_names` match one
SNI, serve the chain whose **matched** pattern is most specific: exact first, then the longest matching
`*.` suffix, in either declaration order and on TCP and QUIC alike. This is what the pinned reference
does. envoy-go currently closes the connection or serves the wrong chain.

**Architecture.** There is one production function change in
`internal/listener/listenerfilter/chainmatch.go`. It replaces `sniSpecificityRank(patterns)` (the rank of
the whole set) with `sniMatchedRank(patterns, sni) (rank, suffixLen)` and adds a suffix-length compare in
`breakTie` slot 2. The rest of the plan makes that falsifiable: 19 unit rows, one QUIC wiring arm, the
four-listener differential fixture `0124`, five NC mutants scored per arm, the comment and normative-doc
reconciliation, `ADR-0321`, and the toolchain fold-in. **Every code block in the appendices was built,
run and reverted at this PLAN stage** (§0, §1.2); none of it is a sketch.

**Tech stack.** Go; `go-control-plane` v3 protos; the `listenerfilter` chain-match package; `quic-go`
(pinned in `go.mod`) for the QUIC arm; the differential harness against `envoyproxy/envoy:contrib-v1.37.2`
by digest.

---

## Global Constraints

These apply implicitly to every task.

- **Reference pin:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`). **Run it BY DIGEST**, after verifying `docs/envoy-go/ENVOY_TARGET.md` lines 3-4.
- **`grep` is a `ugrep` shell function in EVERY shell here.** It honours `.gitignore`, and
  `next-prompt.txt` is gitignored yet tracked. Use `/usr/bin/grep`, `git grep`, or a direct path, and name
  the binary.
- **Use `git -C <abs-worktree>` for every git command.** The Bash tool's cwd silently resets, and shell
  variables do not survive between tool calls.
- **`-count=1` is not optional on any `go test`.** Use `-v` whenever you count; `RUN=0` beside `RC=0` is a
  vacuous green. Take rc from `PIPESTATUS[0]`, never from the end of a pipe. The FAIL matcher is
  `^(FAIL|--- FAIL)|^ *--- FAIL` and the panic gate is `^panic:|DATA RACE|SIGSEGV`.
- **Lint:** `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`. The ambient go1.27.1 breaks the linter's
  export-data reader (`SPEC.md` §10), and rebuilding the linter does not fix it. **misspell runs in US
  locale.** Check `gofmt -l` by its OUTPUT; it never exits non-zero.
- **Cost figures come from `git diff --numstat`, never `--stat`.**
- **Build with `-o <scratch>`.** A bare `go build ./cmd/envoy-go/` drops a binary in the worktree.
- **Ports:** the ephemeral range is `32768-60999`; the harness reserves `20000..31007` and `11000..14999`.
  **`0124`'s reference ports are `15124 15225 15226 15227`, censused at this tip** (§2.4).
- **Never tear down a container this session did not create**, and then only BY NAME. A `reaper_*`
  container belongs to the differential itself.
- **The row registers no stat name.** It owes a `+0, UNCHANGED` `BEHAVIOR_CONTRACT.md` ledger entry in the
  phase-96/97/98 form, **quoting no absolute**.
- **Do not repair `BEHAVIOR_CONTRACT.md:4359`** (`listener_filters_timeout` *"honored"*). It is known false
  and belongs to a banked row.
- **Do not fold in the order-dependent pairwise tie fold** (`SPEC.md` §0.4). It is banked.

---

## 0. What this PLAN refuted, by execution

Two agents built and ran every artifact this plan embeds, in throwaway worktrees off `ebc3fc0c` that
have since been removed. One was a Docker fixture agent. The other was a no-Docker agent covering unit
arms, mutants and citations. **Nine findings; five of them (0.1, 0.2, 0.3, 0.4, 0.8) refute a SPEC claim
outright.**

### 🔴 0.1 — `SPEC.md` §11 ROW 3 IS WRONG ABOUT WHAT MUST STAY GREEN

Row 3 (P1, whole-set rank) says *"must NOT redden: everything else"* beyond `M1/M2`. **Measured: P1 also
reddens `O1_shared_matched_wildcard_is_ambiguous`.** Under whole-set rank, O1's chain
A=`["*.foo.test","q.test"]` ranks 0 through `q.test` and wins, so the expected ambiguity never happens.
**Corrected in §7, row 3.**

### 🔴 0.2 — `SPEC.md` §11 ROW 4's NAIVE MUTANT DOES NOT COMPILE, SO IT IS A VACUOUS CONTROL

Deleting the two suffix-length compares leaves `la`/`lb` unused, which fails the build. Every arm would
then read red for a reason unrelated to the mutation. **The working mutant also rewrites
`ra, la :=`/`rb, lb :=` to `ra, _ :=`/`rb, _ :=`** (Appendix F). Built that way, it reddens exactly row 1's
set, including the QUIC arm. **A control that fails to COMPILE is a masked control** (router method note
88's sibling).

### ⚠️ 0.3 — `SPEC.md` §6.2's `quic.go:181-183` ANCHOR IS OFF, AND ITS TEXT HAS A SECOND FALSE HALF

The paragraph is **`quic.go:182-185`**; `:181` is a bare `//`. It also ends *"mirroring the reference's
'no filter chain found'"*, which is false for the ambiguous nil source, so the repair drops that phrase.
The replacement is **4/4, comment-only**, and gated (§6). **Every live `quic.go:<N>` citation points above
`:181`** (max `:144`), so neutrality here is hygiene, not a live-cite constraint.

### ⚠️ 0.4 — `SPEC.md` §5.2's *"tip 17 RED"* COUNTS OUTPUT LINES, NOT ROWS

The figure came from the FAIL matcher over output lines, which included the parent and bare `FAIL` lines.
**Per row at the tip, measured:** 13 precedence subtests plus the O1 row makes **14 failing unit
subtests**, and the QUIC arm adds **2 failing subtests** (one per declaration order). **Count ROWS by name,
never FAIL lines** (§5).

### 🔴 0.5 — P2's SLOT-2 INSERT SHIFTS TWO LIVE CITATIONS, AND THE SPEC's `26 23` IS A FLOOR THAT MOVED

`chainmatch.go:<N>` appears 66 times across the repo. **The live set is 18 lines, all in
`internal/listener/quic_test.go`.** The `SPEC.md` §6 comment edits are each **1:1 in line count**
(`@@ -25,4 +25,4`, `-54,5 +54,5`, `-187,2 +187,2`, `-197,4 +197,4`, `-213 +213`). P2's slot-2 hunk inserts
six lines at `:222`, which shifts everything below it by **+6**. Only two live cites point below:
**`quic_test.go:952` and `:1033`, both to `alpnMatchAny` at `293-302` → `299-308`**. Both are fixed in the
same commit as the shift (Task 10). The whole-file edit measures **`42 39`**: code `+18 -14`, comments
`+24 -25`. A line-neutral slot-2 variant (`39 42`, no cite fixes) was also built and passes. **It is
rejected** because it merges the rank and length compares into one condition, which makes NC rows 2 and 4
no longer single-line mutations.

### ⚠️ 0.6 — THE SPEC's `:25-28` IS `:24-28`, AND ITS §6.1 `:213` EDIT WAS NOT IN THE SPEC's PROTOTYPE

The `ServerNames` field comment is the **5-line** block at `:24-28`. The slot-2 line comment at `:213` is
on SPEC §6.1's edit list, but the SPEC-stage P2 diff did not touch it. Appendix A does.

### ⚠️ 0.7 — THE FIXTURE FLOOR `~1300-1500` WAS AN OVERSTATEMENT, AND THE LEAF NEEDED A SAN THE SPEC OMITTED

`reference_measured_prototype_is_a_lower_bound` also fires in the other direction (router method note 20).
One Go renderer serves both sides, so no committed YAML is needed, and the fixture measured **~941 added
lines** (`driver.go` 609, `pki/gen/main.go` 169, `README.md` 87, `expectations.yaml` 46, three PEMs 29,
plus `runner_test.go` `1 0`). **The leaf's SANs must include `nomatch.example`.** Without it, the
`l_default` DEFAULT row fails client-side verification and reads exactly like a close. SPEC §7.4's SAN list
omits it.

### ⚠️ 0.8 — THE INVERTED PATCH ALSO REDDENS `l_mixed`

SPEC §11 row 1's fixture list names the three LONG rows only. **Measured: the inverted P2 also serves X for
`l_mixed a.b.foo.test`**, because shortest-wins picks X's `*.foo.test` over Y's `*.b.foo.test`. SPEC §3.1
already implied this. **Corrected in §7.**

### ⚠️ 0.9 — THE QUIC ARM MUST NOT ADD AN IMPORT

Adding an import to `quic_test.go` shifts every line below the import block, and so stales every
`quic_test.go`-internal and external line cite. The arm (Appendix C) reuses the file's existing helpers,
**needs no import change**, and is **appended at the end of the file**, so no existing line moves.

---

## 1. Stage scope, MEASURED

### 1.1 What THIS PLAN commit touches — FOUR files

Precedent `b4c5e97a` (phase-98 PLAN), `git show --numstat`: `STATE.md`, `STATE_HISTORY.md` `2 0`, the new
`PLAN.md`, `next-prompt.txt`. **No `DECISIONS.md`**, so the `PROPOSED` guard stays ARMED at `## ADR-0321`.

### 1.2 What the IMPL will touch

| path | measured basis | added / removed |
|---|---|---|
| `internal/listener/listenerfilter/chainmatch.go` | Appendix A, built and run | **42 / 39** |
| `internal/listener/listenerfilter/chainmatch_sni_test.go` (new) | Appendix B, built and run | **102 / 0** |
| `internal/listener/quic_test.go` | Appendix C (arm, appended) + the two cite fixes | **112 / 2** |
| `internal/listener/quic.go` | Appendix D, comment-only | **4 / 4** |
| `internal/filter/http/compressor/compressor_test.go` | Appendix G, built and run under both toolchains | **53 / 49** |
| `test/fixtures/0124-listener-sni-longest-suffix/**` (7 files) | Appendix E, built and run against the reference | **~941 / 0** |
| `test/differential/runner_test.go` | the blank import | **1 / 0** |
| `docs/envoy-go/DECISIONS.md` | ADR-0321 §Decision + §Consequences; precedent phase 98 `126 2` | ~**+110 / −2** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | `:4367` edit, `:4368` addition, ledger entry; precedent `3 0` | ~**+4 / −1** |
| `docs/envoy-go/ROADMAP.md` | row 99 flip | **1 / 1** |
| `PROGRESS.md`, `STATE*.md`, `next-prompt.txt` | the close | not LoC |

**Byte-untouched roster (the IMPL asserts each is EMPTY under `git diff master --numstat`):**
`internal/listener/manager.go`, `internal/listener/listenerfilter/types.go`,
`internal/listener/listenerfilter/tls_inspector/**`, `internal/filter/http/compressor/compressor.go` (the
fold-in is test-only), every fixture directory except `0124`, `go.mod`, `go.sum`, `.github/**`.
⚠️ **The byte-untouched roster and the edit roster are not a partition.** `STATE*.md`, `PROGRESS.md` and
`next-prompt.txt` are on neither, and are named here for that reason.

### 1.3 The split gate — EVALUATED WITH A COMMAND, NOT SPLIT

`BOOTSTRAP_PROMPT.md` §6.1 sets the thresholds at **~25 numbered tasks** or **~1500 LoC**.
**Tasks: 19**, measured with
`/usr/bin/grep -c '^### Task ' PLAN.md`. The per-task sub-step histogram and its maximum are measured in
§10 by command.

| accounting | this row (measured, except the two doc rows) | phase 98 IMPL MEASURED (`884e4b7c`) |
|---|---|---|
| `.go` only | **≈ 42+102+112+4+53+609+169+1 = 1092** added | **1218** (`chainmatch.go 3`, `types.go 3`, `manager.go 4`, `chainmatch_test.go 34`, `manager_test.go 340`, `quic_test.go 100`, `runner_test.go 1`, `driver.go 733`) |
| + fixture YAML/README/PEM | **≈ 1254** | **1846** (+ `expectations.yaml 294`, `README.md 334`) |

**DECISION: DO NOT SPLIT.** The row is under ~1500 on the widest accounting that excludes transcript and
router, and 19 tasks sit against ~25. The only clean seam is {unit layer} / {fixture}, and splitting there
would ship the production edit gated by only one of its two surfaces. **This estimate is a FLOOR.** If
contact with reality pushes a task past ~10 sub-steps, §6.1's **mid-execution** trigger applies then.

---

## 2. Sentinel and baselines — RUN AT THIS STAGE's TIP

### 2.1 The three checks, the four NCs, and the check-(2) positive control

These were run at the worktree tip before any edit, with `/usr/bin/grep`, verbatim from `next-prompt.txt`,
and **re-run in the publishing tree with identical output** (a PLAN touches no `ROADMAP.md` byte):
(1) **ONE**, `NOT DONE: row 99` · (2) **SIX** at `:209 :215 :221 :231 :237 :245` · (3) **SILENT** ·
NC-A: the substitution was inspected first (`NC LANDED? [ in-progress ]`), then **TWO** lines ·
NC-B at `want=130`: **TWO** lines · NC-C: **FIRED** · NC-D: **96 / 68** under `--` · check-(2) positive
control: **6 substitutions, residual 0** · escape-aware malformed set: **{57, 69}** at `:119`/`:131` ·
row 99 NF: **8**. Per-line md5 with the **trailing newline INCLUDED**: `209 10d7807bf02d` ·
`215 4a92f7e62fc6` · `221 2a7eb298b9fd` · `231 242e53c6f7a3` · `237 b2680e6f4fbf` · `245 6caa1c3ce0e7`.
⇒ **The sentinel does NOT fire, and `stop` was NOT created.**

### 2.2 The `PROPOSED` guard — ARMED, and this stage leaves it ARMED

`^> \*\*STATUS: PROPOSED` hits one line, `:19363`. A backward `^## ADR-` search resolves it to
**`## ADR-0321`**. The ADR-0231 decoy (`^\*\*Status:\*\* PROPOSED`) still hits `:14866`. The IMPL
disarms the guard at Task 14.

### 2.3 The fixture set

The blank-import extractor reads **125 = 125** at this tip, with both `comm` directions empty. After
`0124` lands it must read **126 = 126** (measured on the prototype).

### 2.4 Ports

`git grep -lw -- <port> -- test/ internal/ cmd/`: `15124` hits only `0123/README.md` and
`0123/driver/driver.go`, the prose reserving it. `15225`, `15226` and `15227` hit **zero** files. `ss`
shows zero sockets. **Re-census at the IMPL tip.** This stage used no ad-hoc probe band of its own; its
agents used the harness's allocation and throwaway worktrees only.

---

## 3. STABLE ANCHORS — use these, never line numbers

- **A1** `func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec` (chainmatch.go)
- **A2** the literal `// Slot 2 — ServerNames:` (the slot-2 comment line)
- **A3** `func sniSpecificityRank(patterns []string) int` → becomes `func sniMatchedRank(patterns []string, sni string) (int, int)`
- **A4** `// ErrAmbiguousChainMatch is returned by SelectChain`
- **A5** `// ServerNames: empty means unspecified; non-empty means SNI must match`
- **A6** `func (rt *listenerRuntime) selectQUICChain(conn *quic.Conn) *chainInfo` (quic.go); the doc paragraph
  above it begins `// Returns nil when no chain is selectable`
- **A7** `func alpnMatchAny(` — the target of the two shifting `quic_test.go` cites, re-derived by
  `/usr/bin/grep -n 'chainmatch.go:293-302' internal/listener/quic_test.go`
- **A8** `func TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot` — the current
  last test in `quic_test.go`; the new arm is appended after it
- **A9** `func TestEncodeData_LevelMapping_DifferentGzippedSizes` (compressor_test.go)
- **A10** `### Chain-match algorithm` and `**Phase 98 — +0, UNCHANGED` in `BEHAVIOR_CONTRACT.md`
- **A11** `*§Decision and §Consequences follow at the phase-99 IMPL.*` in `DECISIONS.md`

---

## 4. Test design — MEASURED per arm, not predicted

Scored at this PLAN stage in a throwaway worktree. **P2** is Appendix A. **inv** swaps the
suffix-length compare. **rank-mut** swaps the rank compare. **P1** is whole-set rank (Appendix F.3).
**drop-len** is Appendix F.4, the compiling form.

| row | tip | P2 | inv | rank-mut | P1 | drop-len |
|---|---|---|---|---|---|---|
| `C0_long/short_…_longer_suffix_wins` (2) | F | P | F | P | P | F |
| `B_desc/asc` × deepest/middle (4) | F | P | F | P | P | F |
| `M1/M2_rank_of_matched_pattern…` (2) | F | P | F | P | **F** | F |
| `M3/M4_longest_matching_member…` (2) | F | P | F | P | P | F |
| `M3/M4_deeper_member_of_mixed_set_wins` (2) | F | P | F | P | P | F |
| `E_default_present_two_wildcards…` | F | P | F | P | P | F |
| `R_exact_beats_wildcard_*` (2) | P | P | P | **F** | P | P |
| `C0_matched_negative` / `M1_exact_member` / `E_default_serves_only_no_match` | P | P | P | P | P | P |
| **`O1_shared_matched_wildcard_is_ambiguous`** | **F** | P | P | P | **F** | P |
| `TestSelectChainAmbiguousReturnsError` (unchanged) | P | P | P | P | P | P |
| QUIC arm, both subtests | F | P | F | P | P | F |

⚠️ **The three eligibility-only rows are GREEN under every mutant.** Only one chain is eligible for them, so
they never reach `breakTie`. They pin liveness, not precedence, and are labelled that way in the file.
⚠️ **The two `R_*` rows are live only under rank-mut.** Only O1 separates P2 from inv on the "stays
ambiguous" axis.

**Five-selector suite** (`./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/...
./validate/...`) with the new tests: at the tip, rc 1 with **421** `=== RUN` (398 + 23 new), RED on exactly
the new failing rows. At full P2, rc 0, 421, 0 RED. `go test -race -count=1 ./internal/listener/...` at P2
gave rc 0.

**Fixture `0124`, per row, measured** (subject side; the reference had zero errors in all six runs):

| row | ref | tip | P2 (×3) | P1 | inv |
|---|---|---|---|---|---|
| `l_long_first` a.b.foo.test | LONG | **CLOSED** | LONG | LONG | **SHORT** |
| `l_short_first` a.b.foo.test | LONG | **CLOSED** | LONG | LONG | **SHORT** |
| `l_mixed` a.b.foo.test | Y | **X** | Y | **X** | **X** |
| `l_default` a.b.foo.test | LONG | **CLOSED** | LONG | LONG | **SHORT** |
| six single-candidate rows | as named | green | green | green | green |
| rc | | 1 | 0,0,0 | 1 | 1 |

---

## 5. Tasks

⚠️ **Order is load-bearing.** Tasks 1-9 land every falsifier and record the un-fixed tip **before**
Task 10 changes production code. Nothing after Task 10 can recreate that measurement.

---

### Task 1: Baseline, a proven-live panic gate, and `PROGRESS.md`

**Files:**
- Create: `docs/envoy-go/phases/99-chain-match-sni-longest-suffix/PROGRESS.md`

**Interfaces:**
- Produces: the un-fixed baseline roster (`base.txt`), which Task 9 diffs against.

- [ ] **Step 1: Resolve the selectors before believing any FAIL.** A nonexistent package prints
      `[setup failed]` and exits 1.

```sh
go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...
```

- [ ] **Step 2: Record the baseline.**

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > "$SCRATCH/base.txt" 2>&1; echo "RC=${PIPESTATUS[0]}"
/usr/bin/grep -c '=== RUN' "$SCRATCH/base.txt"                              # expect 398
/usr/bin/grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' "$SCRATCH/base.txt"        # expect 0
/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV' "$SCRATCH/base.txt"           # expect 0
/usr/bin/grep -oE '^=== RUN +[^ ]+' "$SCRATCH/base.txt" | sort > "$SCRATCH/base.roster"
```

- [ ] **Step 3: Prove the panic gate FIRES.** Put a bare `panic("p99 gate probe")` as the first statement
      of `breakTie` (anchor A1). Run `go test -count=1 -v -run 'TestSelectChainBreakTieFollowsPriorityOrder'
      ./internal/listener/listenerfilter/`, confirm the gate reads ≥ 1, revert, and confirm it reads 0.
      Confirm the selector resolves first: `-run` matching nothing exits 0.
- [ ] **Step 4: Record the known flake if it fires.** `TestEnvoyGoBinary_TwoListenerCutover` can fail with
      `bind 127.0.0.1:<port>` inside the ephemeral range. It is not this row's regression, and a green
      rerun clears nothing. Record the port.
- [ ] **Step 5: Create `PROGRESS.md`** with the Task 1 entry: selectors, RC, RUN, FAIL, and the panic gate
      before, during and after the probe.
- [ ] **Step 6: Commit.**

```sh
git -C "$W" add docs/envoy-go/phases/99-chain-match-sni-longest-suffix/PROGRESS.md
git -C "$W" commit -m "phase 99 (chain-match-sni-longest-suffix) IMPL Task 1: un-fixed baseline + panic gate proven live"
```

---

### Task 2: Fold-in (a) — the compressor level assertion, RE-POINTED

**Files:**
- Modify: `internal/filter/http/compressor/compressor_test.go` (anchor A9, the one hunk only)

**Interfaces:**
- Produces: `TestEncodeData_LevelMapping_LevelReachesEncoder`, which replaces
  `TestEncodeData_LevelMapping_DifferentGzippedSizes`.

- [ ] **Step 1: Confirm the RED under the ambient toolchain and the green under the pinned one.**

```sh
cd "$W" && go version                                                     # expect go1.27.1
go test -count=1 -v -run 'TestEncodeData_LevelMapping_DifferentGzippedSizes' ./internal/filter/http/compressor/   # expect --- FAIL … both = 2121
GOTOOLCHAIN=go1.26.2 go test -count=1 -v -run 'TestEncodeData_LevelMapping_DifferentGzippedSizes' ./internal/filter/http/compressor/   # expect --- PASS
```

- [ ] **Step 2: Apply Appendix G** (`git apply`). The result must be exactly **`53 49`** on `--numstat`,
      with only that file touched.
- [ ] **Step 3: Green on BOTH toolchains.** Run the package's full test set under each, `-count=1 -v`.
      Expect **186** `=== RUN`, 0 FAIL on each.
- [ ] **Step 4: Prove it LIVE under two plumbing mutations, each on both toolchains.** M1: in
      `compressor.go` `acquireWriter`, change `gzip.NewWriterLevel(buf, g.level)` to
      `gzip.NewWriterLevel(buf, gzip.DefaultCompression)`. The re-pointed test must FAIL, with
      `XFL 0, want 4 or 2` on responses 0 and 1. M2: make the pooled branch build a fresh writer at the
      default level. It must FAIL on response 1 only. ⚠️ **The original test PASSED M2 under go1.26.2**, which
      is why this is not a relaxation. **Revert `compressor.go` and assert it byte-untouched**
      (`git diff --numstat -- internal/filter/http/compressor/compressor.go` is EMPTY).
- [ ] **Step 5: Record the matrix in `PROGRESS.md`** ({original, re-pointed} × {go1.26.2, go1.27.1} ×
      {unmutated, M1, M2}) and commit.

---

### Task 3: Fold-in (b) — lint under the pinned toolchain, with a COMPILING planted control

**Files:** none (runs and records only)

- [ ] **Step 1: Baseline.**
      `cd "$W" && GOTOOLCHAIN=go1.26.2 golangci-lint run ./...; echo RC=$?`. Expect rc 0 and no output.
- [ ] **Step 2: Planted control that COMPILES.** Create `internal/listener/zz_planted.go`:

```go
package listener

import "os"

func PlantedExported() int {
	os.Remove("/nonexistent")
	x := 1
	x = 2
	return x
}
```

      Run the same command. Expect rc 1 and **each of `errcheck`, `revive`, `ineffassign` named** in the
      output. ⚠️ **If only `typecheck` fires, the control is masked** (router method note 88).
- [ ] **Step 3: Delete the file, re-run, and expect rc 0.** Record all three runs in `PROGRESS.md`, along
      with the fact that CI (`.github/workflows/ci.yml`: `setup-go 1.23`, `golangci-lint-action` v1.64.8)
      needs no change. Commit.

---

### Task 4: The unit file — `chainmatch_sni_test.go`, RED at the un-fixed tip

**Files:**
- Create: `internal/listener/listenerfilter/chainmatch_sni_test.go` (Appendix B, verbatim)

**Interfaces:**
- Consumes: `SelectChain`, `ChainSpec`, `ChainMatchInputs`, `ErrAmbiguousChainMatch` (existing).
- Produces: `TestSelectChainSNILongestMatchedSuffix` with 19 named rows, plus helpers `sniInputs(sni string) ChainMatchInputs`
  and `sniChain(name string, patterns ...string) *ChainSpec`.

- [ ] **Step 1: Write the file** from Appendix B. Check that the helper names do not collide:
      `git grep -n 'func sniInputs\|func sniChain' -- internal/listener/listenerfilter/` must list only the
      new file.
- [ ] **Step 2: Run it at the un-fixed tip.**

```sh
go test -count=1 -v -run 'TestSelectChainSNILongestMatchedSuffix|TestSelectChainAmbiguousReturnsError' \
  ./internal/listener/listenerfilter/ > "$SCRATCH/t4.txt" 2>&1; echo "RC=${PIPESTATUS[0]}"
/usr/bin/grep -oE '^ *--- (PASS|FAIL): [^ ]+' "$SCRATCH/t4.txt" | sort
```

- [ ] **Step 3: Score PER ROW against §4's `tip` column.** Expect **14 FAIL** (13 precedence rows plus O1)
      and **5 PASS** among the rows (2 `R_*`, 3 eligibility-only), with `TestSelectChainAmbiguousReturnsError`
      PASS. ⚠️ **Count ROWS by name, never FAIL lines** (§0.4). ⚠️ **O1 was PREDICTED at the SPEC and
      MEASURED at this PLAN.** If it reads anything else, stop and find the variable.
- [ ] **Step 4: `gofmt -l` (by output), `go vet`, and lint for the package.** Record, then commit.

---

### Task 5: The QUIC wiring arm — RED at the un-fixed tip

**Files:**
- Modify: `internal/listener/quic_test.go`, appended after anchor A8. Appendix C, verbatim. **No import
  change.**

**Interfaces:**
- Consumes: `assertH3ClientTransmitsSNI`, `mkQUICListenerChains`, `mkQUICDownstreamTS`,
  `mkHCMFilterQUICChain`, `mkClusterMgr`, `mkBoot`, `testHTTPRegistry`, `assertQUICAcceptCounterPointers`,
  `mkH3ClientTLS`, `testAlphaCertPEM`, `testAlphaKeyPEM` (all existing in the package).
- Produces: `TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins`, with subtests `LONG_declared_first`
  and `SHORT_declared_first`.

- [ ] **Step 1: Append the arm.** Then prove no existing line moved: `git diff -U0 -- internal/listener/quic_test.go`
      must show exactly one hunk, `@@ -1739,3 +1739,113 @@` or its re-derived equivalent at the file end,
      and **zero `-` lines**.
- [ ] **Step 2: Run it at the tip.** Expect **both subtests FAIL**, on PROPERTY 1 only, with an
      `H3 error (0x0)` / closed-connection error. PROPERTY 2 (the `x.foo.test` matched negative) must PASS
      in both, which proves SHORT is live.
- [ ] **Step 3: Record and commit.**

---

### Task 6: Fixture `0124` — PKI

**Files:**
- Create: `test/fixtures/0124-listener-sni-longest-suffix/pki/gen/main.go` (Appendix E.2)
- Create (generated): `pki/ca.pem`, `pki/server.pem`, `pki/server.key.pem`

- [ ] **Step 1: Write the generator** and run it: `cd "$W"/test/fixtures/0124-listener-sni-longest-suffix && go run ./pki/gen`.
- [ ] **Step 2: Assert the leaf's SANs**:
      `openssl x509 -in pki/server.pem -noout -ext subjectAltName` must list `*.foo.test`, `*.b.foo.test`,
      `*.c.b.foo.test`, `x.test` **and `nomatch.example`** (§0.7).
- [ ] **Step 3: Determinism.** Run the generator a second time and confirm all three PEM sha256 sums are
      unchanged.
- [ ] **Step 4: Commit.**

---

### Task 7: Fixture `0124` — the driver

**Files:**
- Create: `test/fixtures/0124-listener-sni-longest-suffix/driver/driver.go` (Appendix E.1, verbatim)

**Interfaces:**
- Consumes: `fixture.RegisterFixture`, `fixture.Driver`, `fixture.MultiListenerDriver`,
  `fixture.StatsAsserter`, `helpers.HTTPGetReadyRaw`.
- Produces: the registered name `0124-listener-sni-longest-suffix`, listeners
  `l_long_first l_short_first l_mixed l_default` at reference ports `15124 15225 15226 15227`, and nine
  distinct stat prefixes `lf_long lf_short sf_short sf_long mx_x mx_y df_long df_short df_default`.

- [ ] **Step 1: Re-census the four ports** (§2.4) and check `ss -tan` for them.
- [ ] **Step 2: Write the driver.** ⚠️ **Contract points the runner enforces** (`test/differential/runner_test.go`):
      it `t.Fatalf`s when `len(SubjectListenerNames()) != len(ReferenceListenerPorts())`, and it zips the
      two index-wise. It still calls the single-address `SubjectListenerName()`/`ReferenceListenerPort()`
      (listener[0]). A `Drive` error becomes `t.Fatalf`, **so the driver RECORDS closed or wrong rows and
      asserts them in `AssertStats` with one `Errorf` per property.** Otherwise one row would mask the
      rest.
- [ ] **Step 3: Validate the rendered subject config** by building the binary with `-o "$SCRATCH/eg"` and
      running `"$SCRATCH/eg" -mode validate -c <rendered>` (expect rc 0). Render it with a throwaway
      `go run` of `renderBootstrap` or by dumping from a test log.
- [ ] **Step 4: `gofmt -l`, `go vet`, and lint** (`GOTOOLCHAIN=go1.26.2 golangci-lint run ./test/fixtures/0124-listener-sni-longest-suffix/...`).
      Commit.

---

### Task 8: Fixture `0124` — `README.md`, `expectations.yaml`, and the FOUR registration gates

**Files:**
- Create: `test/fixtures/0124-listener-sni-longest-suffix/README.md` (Appendix E.3)
- Create: `test/fixtures/0124-listener-sni-longest-suffix/expectations.yaml` (Appendix E.4)
- Modify: `test/differential/runner_test.go`: one blank import,
  `_ "github.com/pgdad/envoy-go/test/fixtures/0124-listener-sni-longest-suffix/driver"`, directly after
  the `0123` import

- [ ] **Step 1: Write both documents and the import.**
- [ ] **Step 2: Satisfy all FOUR gates:** `RegisterFixture` in `init()`; the blank import; byte-identity
      between the directory name and the registered string; and the `NNNN-` directory shape. ⚠️ **Gates
      1-3 SKIP rather than fail, and gate 4 produces no subtest at all.**
- [ ] **Step 3: Score on the SET-DIFFERENCE.**

```sh
extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
extract test/differential/runner_test.go > "$SCRATCH/imports.txt"
ls -d test/fixtures/*/ | sed -E 's#test/fixtures/##; s#/$##' | sort > "$SCRATCH/dirs.txt"
wc -l < "$SCRATCH/imports.txt"; wc -l < "$SCRATCH/dirs.txt"        # expect 126 and 126
comm -23 "$SCRATCH/imports.txt" "$SCRATCH/dirs.txt"; comm -13 "$SCRATCH/imports.txt" "$SCRATCH/dirs.txt"   # both EMPTY
```

- [ ] **Step 4: NC the extractor by RENAME** in a scratch copy (both `comm` directions fire), and **by
      DELETION** (only `comm -23` fires). These are not the same control, so run both.
- [ ] **Step 5: Commit.**

---

### Task 9: RECORD the un-fixed tip — every falsifier RED, and the roster SPENT

**Files:**
- Modify: `PROGRESS.md`

- [ ] **Step 1: Run the fixture alone at the un-fixed tip.**

```sh
go test -count=1 -v -run 'TestDifferential/0124-listener-sni-longest-suffix' ./test/differential/ > "$SCRATCH/f124_tip.txt" 2>&1
echo "RC=${PIPESTATUS[0]}"
/usr/bin/grep -c '=== RUN' "$SCRATCH/f124_tip.txt"    # must be > 0: a -run matching nothing EXITS 0
/usr/bin/grep -c -- '--- SKIP' "$SCRATCH/f124_tip.txt" # must be 0: a registration miss SKIPS
/usr/bin/grep -cE 'connection CLOSED without a response|served body' "$SCRATCH/f124_tip.txt"   # subject row failures; read each
```

      Expect rc 1. **The subject is RED on exactly the four `a.b.foo.test` rows** (three CLOSED, and
      `l_mixed` serving X), with the counter pins `lf_long`/`sf_long`/`df_long` at 0 instead of 1 and
      `mx_x`=3/`mx_y`=0 instead of 2/1. **The reference is GREEN on every row.** Record the per-row table.
- [ ] **Step 2: Re-run the five-selector suite.** Expect rc 1 and **421** `=== RUN`. Diff the sorted
      `=== RUN` roster against Task 1's: exactly the 23 new names are added and none removed. The RED roster
      must be exactly the new failing rows (§4 `tip`).
- [ ] **Step 3: Write the per-arm un-fixed roster into `PROGRESS.md`.** ⚠️ **Nothing after Task 10 can
      recreate it.** Commit.

---

### Task 10: THE PRODUCTION EDIT — `sniMatchedRank` and slot 2, plus the two cites it shifts

**Files:**
- Modify: `internal/listener/listenerfilter/chainmatch.go`: the CODE hunks of Appendix A only (slot 2 and
  the function replacement with its own doc comment). The other comment hunks land at Task 12.
- Modify: `internal/listener/quic_test.go`: `chainmatch.go:293-302` → `chainmatch.go:299-308` on the two
  cite lines located by anchor A7

**Interfaces:**
- Produces: `func sniMatchedRank(patterns []string, sni string) (int, int)`. It removes
  `sniSpecificityRank`, which has no other caller (`git grep -n sniSpecificityRank` must then list no `.go`
  hit).

- [ ] **Step 1: Apply the slot-2 hunk and the function-replacement hunk of Appendix A.** Keep the matching
      predicate **byte-identical to `sniMatchAny`'s**: `strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:])`.
- [ ] **Step 2: Re-derive the shift and fix the cites in the SAME commit.** Confirm
      `/usr/bin/grep -n '^func alpnMatchAny' internal/listener/listenerfilter/chainmatch.go` now reads `:299`,
      then `sed -i 's/chainmatch\.go:293-302/chainmatch.go:299-308/'` on the two lines only
      (`git diff --numstat -- internal/listener/quic_test.go` for this step: `2 2`, counting the Task 5
      arm separately).
- [ ] **Step 3: Run the unit file and the QUIC arm.** Every row must be green, and O1 must return
      `ErrAmbiguousChainMatch`.
- [ ] **Step 4: Commit.**

---

### Task 11: ALL ARMS GREEN — fixture, suite, race, lint

**Files:** none (runs and records only)

- [ ] **Step 1: The fixture, three times**, each fully GREEN (rc 0, 0 SKIP).
- [ ] **Step 2: The five-selector suite.** Expect rc 0, 421, 0 RED, and the panic gate at 0.
- [ ] **Step 3: `go test -race -count=1 ./internal/listener/...`**, rc 0. ⚠️ **`-race` on the differential is
      vacuous**, because the subject there is an unraced subprocess.
- [ ] **Step 4: Lint on the pinned toolchain; `gofmt -l` empty; `go mod tidy -diff` empty; `git diff master -- go.mod go.sum` empty.**
- [ ] **Step 5: Record and commit.**

---

### Task 12: The occurrence-set reconciliation in CODE — comments, under TWO gates

**Files:**
- Modify: `internal/listener/listenerfilter/chainmatch.go`: the remaining Appendix A comment hunks
  (`:24-28`, `:54-58`, `:186-188`, `:196-200`, `:213`)
- Modify: `internal/listener/quic.go`: Appendix D (`:182-185`)

- [ ] **Step 1: Apply both.** Every hunk is 1:1 in line count.
- [ ] **Step 2: Gate `quic.go` with §6's gate** over `RANGE=<Task 11 commit>`. Expect
      `inspected 8 … 0 violation(s)` and `neutral +4 -4`, rc 0.
- [ ] **Step 3: Gate `chainmatch.go` with the SAME gate over the same range.** This commit touches only
      comments in that file, **so the comment-only gate applies to this commit's diff even though the file
      carries a code edit from Task 10.** Expect rc 0 and a neutral count.
- [ ] **Step 4: Re-verify every live `chainmatch.go:<N>` cite.** For each of the 18 live cite lines in
      `quic_test.go`, the cited line must still carry the text it named at Task 10.

```sh
git grep -nE 'chainmatch\.go:[0-9]+' -- internal/ cmd/ test/ | /usr/bin/grep -v '/phases/'
```

- [ ] **Step 5: Run the listener packages** (green) **and commit.**

---

### Task 13: NC roster rows 1-5 — built as patches, each proven able to fire, scored PER ARM

**Files:** none (patches are applied, scored, and reverted; the scores go in `PROGRESS.md`)

- [ ] **Step 1: For each row of §7, apply the mutation** from Appendix F onto the Task-12 tree, confirm it
      **COMPILES** (`go vet ./internal/listener/...` rc 0), run the unit file and the QUIC arm, and score
      **each row by name** against §7.
- [ ] **Step 2: Rows 1 and 3 also run fixture `0124`**, once each. Row 1 must redden the three LONG rows
      **and `l_mixed`** (§0.8). Row 3 must redden `l_mixed` only.
- [ ] **Step 3: Revert after each row** and confirm `git diff --numstat -- internal/` shows the Task-12 state.
- [ ] **Step 4: Record the per-arm matrix and commit `PROGRESS.md`.**

---

### Task 14: `ADR-0321` completed IN PLACE

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`: append after anchor A11

- [ ] **Step 1: Append `### Decision (landed at the phase-99 IMPL)`** after the retained footer. It
      **amends ADR-0081 clause 4's `server_names` bullet** (the rank of the MATCHED pattern, then the
      longest matching suffix) and **clause 5** (non-identical ties are per-connection outcomes that close
      the connection; only structurally identical specs are rejected at build). It **notes ADR-0078 clause
      9** (the logic is no longer "preserved verbatim"). It names `sniMatchedRank`.
- [ ] **Step 2: Append `### Consequences (landed at the phase-99 IMPL)`** with items **(a)-(g)**:
      (a) equal-rank wildcard pairs go from closed to served by the longest suffix, on TCP and QUIC, with or
      without a default chain; (b) a mixed exact+wildcard chain that matched only through its wildcard
      loses to a longer matching wildcard, which **moves traffic on a config that served before**;
      (c) two chains whose best matched patterns are the same string go from an arbitrary served winner to
      CLOSED, while the reference refuses that class at validate and the boot reject is banked;
      (d) ADR-0319 (d)'s reach widens, since two-wildcard QUIC shapes now select a chain and the
      Start-time certificate still serves; (e) `+0` stat names; (f) the NOT-bought list (nested descent,
      case folding, overlap reject, order-dependent fold, `no_filter_chain_match`, `"*"` tier and partial
      wildcards, per-connection QUIC certificate identity); (g) the toolchain fold-in and why `go.mod`
      gains no `toolchain` line.
- [ ] **Step 3: Flip the status IN PLACE** (`> **STATUS: PROPOSED` → `> **STATUS: ACCEPTED`, with the rest of
      the line reworded on the ADR-0320 precedent). Use no `**Status:**` line, no renumber and no `---`.
- [ ] **Step 4: Verify BY LINE AND BY ADR.** `/usr/bin/grep -nE '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md`
      must be EMPTY. **Then prove that the empty result is a disarmed guard and not a broken matcher**: read
      the tail ADR's status line directly. The decoy at `:14866` must stay byte-identical
      (`sed -n '14866p' | md5sum` before and after).
- [ ] **Step 5: Commit.**

---

### Task 15: `BEHAVIOR_CONTRACT.md` — the contract edits and the `+0` ledger entry

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (anchor A10)

- [ ] **Step 1: `:4367`.** Replace the SNI clause with: *SNI: the rank of the `server_names` pattern that
      MATCHED (exact > `*.` suffix > `*`), then the LONGEST matching suffix, independent of declaration
      order and of any non-matching pattern the chain lists (ADR-0321).*
- [ ] **Step 2: `:4368`. ADD, do not replace** (`SPEC.md` §0.9). Keep the build-time sentence and append:
      *Non-identical chains that still tie on a connection's inputs are resolved per connection; an
      unresolved tie closes the connection (ADR-0321).*
- [ ] **Step 3: The ledger.** Add `**Phase 99 — +0, UNCHANGED (…)**` directly after the phase-98 entry, in
      its form. It states that the repair registers, renames and retires nothing, that the close path
      increments no stat, that the observable effect is a **redistribution** across existing per-chain
      counters, and that **`no_filter_chain_match` is again deliberately not added**. **Quote no absolute.**
- [ ] **Step 4: Leave `:4359` alone** (it is known false, and the row that owns it is banked). Commit.

---

### Task 16: `ROADMAP.md` — row 99 → `done`, under the FIELD-COUNT gate

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md`, row 99 (locate by ID: `awk -F'|' '/^\| *99 /{print NR}'`)

- [ ] **Step 1: Flip the status to `done` and rewrite the summary cell.** This is a FLIP: `want` stays
      **131** and `ROADMAP.md` stays **249** lines.
- [ ] **Step 2: Count the fields under BOTH forms before installing** (want **8** for each):
      `awk -F'|' '{print NF}'` and `sed 's/\\|//g' | awk -F'|' '{print NF}'`. Reword any pipe away.
- [ ] **Step 3: Assert that the new cell spells NEITHER sentinel match phrase** (`deferred candidates:` /
      `remaining deferred (not-yet-chartered) candidates:`). Check (2) matches the whole file.
- [ ] **Step 4: Re-run the sentinel and all four NCs, and expect the shapes to CHANGE:** check (1) goes
      **SILENT**, NC-A goes to **ONE** (`NOT DONE: row 62`), and NC-B goes to **ONE** at `want=130`. Check (2)
      stays **SIX** and check (3) stays **SILENT**. ⚠️⚠️ **Do not touch the six windows. The margin is ONE.**
- [ ] **Step 5: Verify that the six windows are byte-identical** (md5 with the trailing newline included,
      §2.1). Commit.

---

### Task 17: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

**Files:** none (runs and records only)

- [ ] **Step 1: Assert the byte-untouched roster PER PATH** (§1.2), each EMPTY under
      `git diff master --numstat -- <path>`.
- [ ] **Step 2: Diff the ARM roster.**
      `git diff master -- internal/ test/ | /usr/bin/grep -E '^\+func (Test|Fuzz)' | sort` must list exactly
      `TestSelectChainSNILongestMatchedSuffix`, `TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins`
      and `TestEncodeData_LevelMapping_LevelReachesEncoder`. Also check that
      `-func TestEncodeData_LevelMapping_DifferentGzippedSizes` is the only `-func` line.
- [ ] **Step 3: Gate (a), the full differential, `-count=1`.** Expect **126 PASS / 0 FAIL / 0 SKIP**, with the
      fixture set asserted BY NAME in both `comm` directions. ⚠️ **A driver-owned receiver port race can
      ABORT the binary and mask every later fixture.** Rerun, and say so.
- [ ] **Step 4: Gate (b), the non-Docker sweep,** under the **ambient** toolchain:
      `go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'`, gated on
      `PIPESTATUS[0]` plus a SET reconciliation. Expect the denominator to rise from **239** to **240**,
      because `0124`'s driver is a new package. **Gate (b) must now be GREEN**, which is the fold-in's
      purpose.
- [ ] **Step 5: Gates (c)-(e).** h2spec `95 tests, 94 passed, 1 skipped, 0 failed` (verbose run); fuzzers
      **56 targets / 48 files** (+0); panic gate **0**. **Lint** on the pinned toolchain, rc 0.
- [ ] **Step 6: Gate (f): no `REVIEW.md`.** This is a standing departure; name it. Record everything and
      commit.

---

### Task 18: `PROGRESS.md` close-out and the router/state roll

**Files:**
- Modify: `PROGRESS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`

- [ ] **Step 1: `STATE.md` §Current, edited IN PLACE:** lifecycle-state **3 → DONE** and the next stage. Do
      not prepend.
- [ ] **Step 2: Evict the oldest §Recent entry with the LABEL-BOUND PAIR on BOTH files**, plus a
      fabricated-label NC and a positive control naming an ARCHIVED label. **Measure the histogram,
      including the entry this close promotes.**
- [ ] **Step 3: Archive it as ONE inline PARENTHETICAL line** (raw `+2`, strict guard DELTA 0), naming the
      LABEL and no count. Roll the §Recent preamble sentence.
- [ ] **Step 4: Roll `next-prompt.txt`** (`git add -f`). Grep it for `YOUR STAGE`, for the stage word, and
      for every figure the row moved (fixtures 126, gate (b) 240, `ROADMAP` check (1) SILENT, ADR tail
      ACCEPTED, and lint now RUNS). Add the order-dependent fold to the banked list if it is not already
      there.
- [ ] **Step 5: Re-derive every quoted line count in the same commit**, then commit.

---

### Task 19: Squash, merge, push, and remove the worktree

- [ ] **Step 1: Squash to ONE commit** whose subject carries the **full slug AND the stage word**
      (`phase 99 (chain-match-sni-longest-suffix) IMPL: …`).
- [ ] **Step 2: Merge fast-forward to master and push.** The commit email must be the repo's configured
      one.
- [ ] **Step 3: `git worktree remove` the stage worktree.** `git worktree list` must show only the
      canonical root. Check `git status --porcelain --untracked-files=all`, filtering out the pre-existing
      `.claude/`.

---

## 6. The comment-only + line-neutral gate — BUILT, AND SHOWN TO FIRE

This is the phase-98 PLAN §7.2 awk gate, retargeted per file, with a numstat-neutrality check added.

**The hazard question, answered first:** `/usr/bin/grep -n -- '"[^"]*//' internal/listener/quic.go` →
rc 1 and no output, and the backtick form is also rc 1. The positive control, the same pattern over
`*.go`, hits `cmd/envoy-go/main_test.go:410`. **A line-prefix matcher is therefore safe for `quic.go`.**
Answer the same question for `chainmatch.go` at Task 12, since the phase-98 PLAN answered it with no hits.

```sh
gate () {  # $1 = RANGE, $2 = path
git diff --no-color --unified=0 "$1" -- "$2" \
| awk '
    /^--- /          { next }
    /^\+\+\+ /       { next }
    /^\\ No newline/ { next }
    /^[+-]/ {
      insp++; if (substr($0,1,1)=="+") add++; else del++
      s = substr($0, 2); sub(/^[[:blank:]]+/, "", s)
      if (s == "")     next
      if (s ~ /^\/\//) next
      printf "VIOLATION (non-comment %s line): %s\n", (substr($0,1,1)=="+" ? "ADDED" : "REMOVED"), $0; v++
    }
    END {
      printf "GATE: comment-only -- inspected %d changed line(s), %d violation(s); neutral +%d -%d\n", insp+0, v+0, add+0, del+0
      if (v+0 > 0 || add+0 != del+0) exit 1
    }'
}
```

**Controls, run at this stage on `quic.go`:**

| # | input | output | rc |
|---|---|---|---|
| A | empty diff | `inspected 0 … 0 violation(s); neutral +0 -0` | 0 |
| B | the real Appendix D edit | `inspected 8 … 0 violation(s); neutral +4 -4` | **0** |
| C | a compiling code rename (`spec` → `sel`), `2 2` | 4 `VIOLATION` lines, `inspected 4 … 4` | **1** |
| D | B and C stacked, `6 6` | 4 violations, `inspected 12` | **1** |
| E | comment-only with one line added | 0 violations, **`neutral +5 -4`** | **1** |

⚠️ **Control A is why the `inspected` counter exists; D is why the gate must not stop at the first legal
line; and E is why neutrality is a second, independent condition.**

---

## 7. The negative-control roster — CORRECTED against `SPEC.md` §11

Every row names the mechanism that carries it to a failure, and every row was **built and scored at this
stage** (§4).

| # | mutation (Appendix F) | must redden (MEASURED) | must stay green (MEASURED) | mechanism |
|---|---|---|---|---|
| 1 | **inv**: suffix-length compare reversed | all 13 precedence rows, the QUIC arm (both), and fixture `l_long_first`/`l_short_first`/`l_default` **and `l_mixed`** a.b rows | `R_*`, the 3 eligibility rows, O1, `TestSelectChainAmbiguousReturnsError` | length decides every same-rank wildcard pair |
| 2 | **rank-mut**: rank compare reversed | `R_exact_beats_wildcard_*` (2) | everything else, **including M1/M2** (both match via wildcard, rank 1) | only exact-vs-wildcard pairs differ in matched rank |
| 3 | **P1**: whole-set rank | `M1/M2_rank_*`, **O1** (§0.1), fixture `l_mixed` a.b | everything else | the mixed set ranks 0 through its non-matching exact name |
| 4 | **drop-len**: length compares removed, **`la`/`lb` → `_`** (compiles; §0.2) | the same set as row 1, except fixture | the same set as row 1 | same-rank wildcards tie → nil |
| 5 | **un-fix**: the Task 9 tip record | the `O1` row (the tip returns A) | — | whole-set rank |

---

## 8. Counts — this PLAN's own tip

**Moved by this stage:** `PLAN.md` (new; its line count is in `STATE.md`, re-derived in the publishing
commit); `STATE.md` (rolled in place); `STATE_HISTORY.md` **+2**; `next-prompt.txt`.
**Not moved:** `ROADMAP.md` **249**, `DECISIONS.md` **19381**, `BEHAVIOR_CONTRACT.md` **5996**, fixtures
**125**, no `.go` file.

---

## 9. Deferred — carried, none chartered

- **The order-dependent pairwise fold in `SelectChain`** (`SPEC.md` §0.4): subject-only, with no reference
  arm measured.
- The duplicate/overlap boot reject; nested-descent precedence; SNI case folding; `listener_filters_timeout`
  enforcement; the `"*"` tier and partial wildcards; the other items in the router's banked list, all
  unchanged.

---

## 10. `SPEC.md` §15 coverage, and self-review

| §15 item | discharged by |
|---|---|
| 1 un-fixed tip first, per arm, O1 scored | Tasks 4, 5, 9 (O1 **measured** at this stage, §4) |
| 2 inherit §6 tables | Task 12 (the Appendix A/D texts are the §6.1/§6.2 EDIT rows), Tasks 14-15 (the LEAVE rows are amended at the ADR) |
| 3 `quic.go` gate | §6, Task 12 |
| 4 NC rows 1-5 executable | §7, Task 13, Appendix F (row 4 corrected, §0.2) |
| 5 counter names on both sides | measured at this stage (all nine prefixes present on both sides, at 0 before any request, so values are pinned), and asserted by the driver's presence guard |
| 6 §6.1 split gate by command | §1.3 |
| 7 fold-in as its own tasks | Tasks 2, 3 |
| 8 keep §0.4 out | §9, Global Constraints |

**Self-review:** the placeholder scan (`TBD|TODO|implement later|similar to Task`) reads zero outside
quoted prose. The type names `sniMatchedRank`, `sniInputs`, `sniChain` and the test names match across
Tasks 4, 5, 10, 13 and 17 and the appendices. The sub-step histogram, measured by command (`awk` counting `^- \[ \] ` per `^### Task `), reads
`6 5 3 4 3 4 4 5 3 4 5 5 4 5 4 5 6 5 3`: **83 sub-steps over 19 tasks, maximum SIX**, so no task approaches
§6.1's ~10 line.

---

# Appendices — the MEASURED artifacts (built, run and reverted at this PLAN stage)


## Appendix A — `chainmatch.go`, the full row edit (`42 39`). Tasks 10 (code hunks: slot 2 + the function) and 12 (the five comment hunks)

```diff
diff --git a/internal/listener/listenerfilter/chainmatch.go b/internal/listener/listenerfilter/chainmatch.go
index 1cafa6ec..0554b86b 100644
--- a/internal/listener/listenerfilter/chainmatch.go
+++ b/internal/listener/listenerfilter/chainmatch.go
@@ -22,10 +22,10 @@ type ChainSpec struct {
 	// requires conn.LocalAddr().IP to fall in at least one CIDR.
 	PrefixRanges []*net.IPNet
 	// ServerNames: empty means unspecified; non-empty means SNI must match
-	// at least one entry per chainSpecificityRank semantics (exact > suffix
-	// > universal > catch-all). Re-uses the existing
-	// internal/listener/manager.go:chainSpecificityRank logic at the
-	// tie-breaker level.
+	// at least one entry (exact name, "*." suffix wildcard, or "*"). When
+	// chains tie, breakTie ranks the pattern that MATCHED the SNI, not the
+	// chain's whole set: an exact name beats any wildcard, then the longest
+	// matching suffix wins (sniMatchedRank; ADR-0321).
 	ServerNames []string
 	// TransportProtocol: "" means unspecified; any non-empty string requires
 	// the connection's detected transport protocol to equal it exactly, set by
@@ -51,11 +51,11 @@ type ChainSpec struct {
 // eligible AND defaultChain is nil.
 var ErrNoChainMatched = errors.New("no filter_chain matches connection")
 
-// ErrAmbiguousChainMatch is returned by SelectChain when two chains have
-// identical specificity vectors AND identical sub-ordering values for the
-// highest-priority specific dimension. The listener manager detects this at
-// NewManager-build time (it pre-runs SelectChain on a sample input or
-// duplicate-matches the chain specs structurally) and rejects the bootstrap.
+// ErrAmbiguousChainMatch is returned by SelectChain, per connection, when two
+// eligible chains have identical specificity vectors and breakTie cannot
+// separate them on that connection's inputs; the connection is then closed.
+// Only structurally identical chain specs are caught at NewManager build time
+// (findIdenticalChainSpecs), which rejects the bootstrap.
 var ErrAmbiguousChainMatch = errors.New("ambiguous filter_chain selection")
 
 // priorityOrder is the 8-dimension specificity priority vector per SPEC
@@ -184,8 +184,8 @@ func specificityScore(c *ChainSpec) uint8 {
 
 // breakTie compares a vs b on the per-dimension finer-grain criteria when
 // their specificity vectors are identical. Returns the winner; returns nil
-// if a and b are entirely indistinguishable (a NewManager-time config error
-// the listener manager surfaces as ErrAmbiguousChainMatch).
+// if a and b are indistinguishable on these inputs: a per-connection outcome
+// SelectChain surfaces as ErrAmbiguousChainMatch, not a build-time error.
 //
 // Cascade order follows SPEC §5.5 line 519 ("walk down the priority list
 // with finer-grain tie-breakers") and §7.3 line 524 ("more-specific value
@@ -194,10 +194,10 @@ func specificityScore(c *ChainSpec) uint8 {
 // each only if both chains specify it (otherwise the specificityScore would
 // already have decided the winner). Only dimensions with a meaningful
 // finer-grain sub-ordering are listed: PrefixRanges (slot 1, longest CIDR),
-// ServerNames (slot 2, SNI rank), SourcePrefixRanges (slot 6, longest CIDR).
-// Dimensions that are exact-value match (DestinationPort, TransportProtocol,
-// ApplicationProtocols, SourceType, SourcePorts) have no sub-ordering and
-// are skipped.
+// ServerNames (slot 2, rank of the MATCHED pattern, then longest matching
+// suffix), SourcePrefixRanges (slot 6, longest CIDR). Dimensions that are
+// exact-value match (DestinationPort, TransportProtocol, ApplicationProtocols,
+// SourceType, SourcePorts) have no sub-ordering and are skipped.
 func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec {
 	// Slot 1 — PrefixRanges: longer prefix wins (smaller IPNet).
 	if len(a.PrefixRanges) > 0 && len(b.PrefixRanges) > 0 {
@@ -210,16 +210,22 @@ func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec {
 			return b
 		}
 	}
-	// Slot 2 — ServerNames: SNI specificity (exact > suffix > universal > catch-all).
+	// Slot 2 — ServerNames: rank of the MATCHED pattern, then longest matching suffix.
 	if len(a.ServerNames) > 0 && len(b.ServerNames) > 0 {
-		ra := sniSpecificityRank(a.ServerNames)
-		rb := sniSpecificityRank(b.ServerNames)
+		ra, la := sniMatchedRank(a.ServerNames, inputs.ServerName)
+		rb, lb := sniMatchedRank(b.ServerNames, inputs.ServerName)
 		if ra < rb {
 			return a
 		} // lower rank = more specific
 		if rb < ra {
 			return b
 		}
+		if la > lb { // same rank: longer matched suffix wins
+			return a
+		}
+		if lb > la {
+			return b
+		}
 	}
 	// Slot 6 — SourcePrefixRanges: longer prefix wins.
 	if len(a.SourcePrefixRanges) > 0 && len(b.SourcePrefixRanges) > 0 {
@@ -301,34 +307,31 @@ func alpnMatchAny(want, offered []string) bool {
 	return false
 }
 
-// sniSpecificityRank mirrors internal/listener/manager.go:chainSpecificityRank
-// (preserved from phase 03 per ADR-0033 clause 9 → ADR-0078). Lower rank =
-// more specific. Used as the SNI sub-ordering tie-breaker WITHIN the
-// server_names priority slot per SPEC §5.5.
+// sniMatchedRank ranks the pattern of patterns that MATCHES sni (not the
+// chain's whole pattern set). Lower rank = more specific:
 //
-//	0: any non-wildcard pattern
-//	1: any suffix-wildcard ("*.foo.test")
-//	2: universal-wildcard ("*")
-//	3: catch-all (empty patterns slice — unused here since
-//	   matches() rejects this case before breakTie sees it)
-func sniSpecificityRank(patterns []string) int {
-	if len(patterns) == 0 {
-		return 3
-	}
-	rank := 4
+//	0: an exact pattern equal to sni (suffix length 0)
+//	1: a "*." suffix wildcard matching sni; the second result is the length
+//	   of the LONGEST matching suffix (len of pattern minus "*"), so a
+//	   longer suffix is more specific
+//	2: the universal wildcard "*"
+//	3: nothing matched (unreachable: matches() rejects it before breakTie)
+func sniMatchedRank(patterns []string, sni string) (int, int) {
+	rank, suffix := 3, 0
 	for _, p := range patterns {
 		switch {
+		case p == sni:
+			return 0, 0
 		case p == "*":
-			if 2 < rank {
+			if rank > 2 {
 				rank = 2
 			}
-		case strings.HasPrefix(p, "*."):
-			if 1 < rank {
-				rank = 1
+		case strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:]):
+			rank = 1
+			if n := len(p) - 1; n > suffix {
+				suffix = n
 			}
-		default:
-			return 0
 		}
 	}
-	return rank
+	return rank, suffix
 }
```


The two cite fixes that land WITH Task 10, in `internal/listener/quic_test.go` (lines located by anchor A7):

```diff
-// (chainmatch.go:293-302) is a genuine any-of intersection over it.
+// (chainmatch.go:299-308) is a genuine any-of intersection over it.
-// finds no intersection (chainmatch.go:131, 293-302) and filter_chains[0] is
+// finds no intersection (chainmatch.go:131, 299-308) and filter_chains[0] is
```


## Appendix B — `internal/listener/listenerfilter/chainmatch_sni_test.go` (Task 4, 102 lines)

```go
package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

// sniInputs builds the inputs a TLS connection carries after tls_inspector:
// SNI, transport_protocol "tls" and an ALPN offer. Loopback addressing mirrors
// the TCP serve path; none of the chains below set an address dimension.
func sniInputs(sni string) ChainMatchInputs {
	return ChainMatchInputs{
		DestinationIP:        net.ParseIP("127.0.0.1"),
		DestinationPort:      10000,
		SourceIP:             net.ParseIP("127.0.0.1"),
		SourcePort:           40000,
		ServerName:           sni,
		TransportProtocol:    "tls",
		ApplicationProtocols: []string{"h2", "http/1.1"},
	}
}

func sniChain(name string, patterns ...string) *ChainSpec {
	return &ChainSpec{Name: name, ServerNames: patterns}
}

// TestSelectChainSNILongestMatchedSuffix pins the server_names sub-ordering:
// among chains that tie on the specificity bitmask, the chain whose MATCHED
// server_names pattern is most specific wins — an exact name beats any
// wildcard, and between wildcards the LONGEST matching suffix wins, in either
// declaration order. Each row is one property, scored on its own.
//
// Rows with wantErr set pin the one slot-2 outcome that stays ambiguous: two
// chains whose best MATCHED patterns are the same string tie on rank AND
// suffix length, so SelectChain returns (nil, ErrAmbiguousChainMatch) and the
// connection closes (the reference refuses this config class at validate).
func TestSelectChainSNILongestMatchedSuffix(t *testing.T) {
	long := sniChain("LONG", "*.b.foo.test")
	short := sniChain("SHORT", "*.foo.test")
	l3 := sniChain("L3", "*.c.b.foo.test")
	l2 := sniChain("L2", "*.b.foo.test")
	l1 := sniChain("L1", "*.foo.test")
	x := sniChain("X", "x.test", "*.foo.test")
	y := sniChain("Y", "*.b.foo.test")
	x2 := sniChain("X2", "*.foo.test", "*.c.b.foo.test")
	e1 := sniChain("E1", "a.b.foo.test")
	w := sniChain("W", "*.b.foo.test")
	def := &ChainSpec{Name: "DEFAULT"}
	oa := sniChain("OA", "*.foo.test", "q.test")
	ob := sniChain("OB", "*.foo.test")

	cases := []struct {
		name    string
		chains  []*ChainSpec
		def     *ChainSpec
		sni     string
		want    string
		wantErr error
	}{
		{"C0_long_declared_first_longer_suffix_wins", []*ChainSpec{long, short}, nil, "a.b.foo.test", "LONG", nil},
		{"C0_short_declared_first_longer_suffix_wins", []*ChainSpec{short, long}, nil, "a.b.foo.test", "LONG", nil},
		{"C0_matched_negative_only_short_matches", []*ChainSpec{long, short}, nil, "x.foo.test", "SHORT", nil},
		{"B_desc_three_levels_deepest_suffix_wins", []*ChainSpec{l3, l2, l1}, nil, "z.c.b.foo.test", "L3", nil},
		{"B_desc_middle_suffix_beats_shortest", []*ChainSpec{l3, l2, l1}, nil, "y.b.foo.test", "L2", nil},
		{"B_asc_three_levels_deepest_suffix_wins", []*ChainSpec{l1, l2, l3}, nil, "z.c.b.foo.test", "L3", nil},
		{"B_asc_middle_suffix_beats_shortest", []*ChainSpec{l1, l2, l3}, nil, "y.b.foo.test", "L2", nil},
		{"M1_rank_of_matched_pattern_not_whole_set_X_first", []*ChainSpec{x, y}, nil, "a.b.foo.test", "Y", nil},
		{"M2_rank_of_matched_pattern_not_whole_set_Y_first", []*ChainSpec{y, x}, nil, "a.b.foo.test", "Y", nil},
		{"M1_exact_member_of_mixed_set_still_wins_its_name", []*ChainSpec{x, y}, nil, "x.test", "X", nil},
		{"M3_longest_matching_member_not_longest_member_X2_first", []*ChainSpec{x2, y}, nil, "a.b.foo.test", "Y", nil},
		{"M4_longest_matching_member_not_longest_member_Y_first", []*ChainSpec{y, x2}, nil, "a.b.foo.test", "Y", nil},
		{"M3_deeper_member_of_mixed_set_wins", []*ChainSpec{x2, y}, nil, "z.c.b.foo.test", "X2", nil},
		{"M4_deeper_member_of_mixed_set_wins", []*ChainSpec{y, x2}, nil, "z.c.b.foo.test", "X2", nil},
		{"E_default_present_two_wildcards_resolve_not_default", []*ChainSpec{long, short}, def, "a.b.foo.test", "LONG", nil},
		{"E_default_serves_only_no_match", []*ChainSpec{long, short}, def, "nomatch.example", "DEFAULT", nil},
		{"R_exact_beats_wildcard_exact_first", []*ChainSpec{e1, w}, nil, "a.b.foo.test", "E1", nil},
		{"R_exact_beats_wildcard_wildcard_first", []*ChainSpec{w, e1}, nil, "a.b.foo.test", "E1", nil},
		{"O1_shared_matched_wildcard_is_ambiguous", []*ChainSpec{oa, ob}, nil, "a.b.foo.test", "", ErrAmbiguousChainMatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectChain(sniInputs(tc.sni), tc.chains, tc.def)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("%s: SNI %q: SelectChain error %v, want %v", tc.name, tc.sni, err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("%s: SNI %q: got chain %v, want nil (ambiguous)", tc.name, tc.sni, got)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: SNI %q: SelectChain error %v, want chain %s", tc.name, tc.sni, err, tc.want)
				return
			}
			if got == nil || got.Name != tc.want {
				t.Errorf("%s: SNI %q: got chain %v, want %s", tc.name, tc.sni, got, tc.want)
			}
		})
	}
}
```


## Appendix C — the QUIC wiring arm, appended to `internal/listener/quic_test.go` (Task 5)

```diff
diff --git a/internal/listener/quic_test.go b/internal/listener/quic_test.go
index 8bbe6faf..07926171 100644
--- a/internal/listener/quic_test.go
+++ b/internal/listener/quic_test.go
@@ -1739,3 +1739,113 @@ func TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot(t *te
 		t.Errorf("default slot pre-empted: quicTLSConfig() = %p = default_filter_chain.tlsCfg — the default slot was consulted BEFORE the Start-time chain selection, which is the pre-Task-11 resolution order this arm exists to exclude", got)
 	}
 }
+
+// TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins is the phase-99
+// QUIC wiring arm (SPEC §5.3).
+//
+// SHAPE: one QUIC listener with two QUIC-TLS filter_chains and NO default slot:
+// LONG carries server_names ["*.b.foo.test"] (body "LONG\n", stat_prefix
+// quic_fc0) and SHORT carries ["*.foo.test"] (body "SHORT\n", quic_fc1). Both
+// share one bitmask (server_names only), so SNI a.b.foo.test makes BOTH
+// eligible and the pair reaches breakTie slot 2 on the PER-CONNECTION path
+// (selectQUICChain(conn), SNI taken from the real ClientHello).
+//
+// PROPERTY: the longest matching suffix wins, in EITHER declaration order —
+// a.b.foo.test is served by LONG. The matched negative x.foo.test matches only
+// SHORT and must be served by SHORT, proving SHORT is live and LONG is not a
+// constant answer.
+//
+// 🔴 EXPECTED RED AT THE UN-FIXED TIP: both chains rank 1 on the whole-set
+// scan, breakTie returns nil, SelectChain returns ErrAmbiguousChainMatch,
+// selectQUICChain returns nil and serveQUICConnection closes the connection,
+// so the a.b.foo.test request fails instead of returning a body. The matched
+// negatives are green at the tip by construction (one chain eligible).
+//
+// ⚠️ This cannot be a nil-conn arm: SNI exists only on a real ClientHello
+// (see arm (h) above), so the manager is Started and real H3 traffic driven.
+// The Start-time selection sees no SNI, so neither chain is eligible there and
+// quicTLSConfig falls back to the first TLS-bearing chain; both chains carry
+// the same certificate, so that fallback cannot change which chain SERVES.
+func TestQUICChainSelection_TwoWildcardsLongestMatchedSuffixWins(t *testing.T) {
+	const sniDeep, sniShallow = "a.b.foo.test", "x.foo.test"
+
+	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
+	defer cancel()
+
+	// PRECONDITION: the client shape transmits the deep name.
+	assertH3ClientTransmitsSNI(t, ctx, sniDeep)
+
+	for _, order := range []struct {
+		name      string
+		longFirst bool
+	}{
+		{"LONG_declared_first", true},
+		{"SHORT_declared_first", false},
+	} {
+		t.Run(order.name, func(t *testing.T) {
+			long := &listenerv3.FilterChainMatch{ServerNames: []string{"*.b.foo.test"}}
+			l := mkQUICListenerChains(t, long, "LONG\n", false, false, "")
+			short := &listenerv3.FilterChain{
+				FilterChainMatch: &listenerv3.FilterChainMatch{ServerNames: []string{"*.foo.test"}},
+				TransportSocket:  mkQUICDownstreamTS(t, testAlphaCertPEM, testAlphaKeyPEM, []string{"h3"}),
+				Filters:          []*listenerv3.Filter{mkHCMFilterQUICChain(t, "quic_fc1", "SHORT\n")},
+			}
+			if order.longFirst {
+				l.FilterChains = append(l.FilterChains, short)
+			} else {
+				l.FilterChains = []*listenerv3.FilterChain{short, l.FilterChains[0]}
+			}
+
+			cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
+			reg := stats.NewRegistry()
+			mgr, err := NewManager(mkBoot(0, []*listenerv3.Listener{l}, nil), cm, reg, testHTTPRegistry())
+			if err != nil {
+				t.Fatalf("precondition: NewManager(quic, LONG [*.b.foo.test] + SHORT [*.foo.test], %s): %v", order.name, err)
+			}
+			if err := mgr.Start(ctx); err != nil {
+				t.Fatalf("precondition: Start: %v", err)
+			}
+			defer mgr.Stop()
+			// BEFORE THE FIRST BYTE.
+			assertQUICAcceptCounterPointers(t, mgr.runtimes[0])
+			infos := mgr.Listeners()
+			if len(infos) != 1 {
+				t.Fatalf("precondition: Listeners() = %d, want 1", len(infos))
+			}
+			addr := infos[0].Addr
+
+			// get performs one H3 GET with the given SNI. Unlike h3GetChainBody a
+			// transport failure is NOT a precondition here: a closed connection is
+			// exactly the tip's defect, so it is returned and scored per property.
+			get := func(sni string) (string, error) {
+				rctx, rcancel := context.WithTimeout(ctx, 10*time.Second)
+				defer rcancel()
+				tr := &http3.Transport{TLSClientConfig: mkH3ClientTLS(sni), QUICConfig: &quic.Config{}}
+				defer func() { _ = tr.Close() }()
+				req, err := http.NewRequestWithContext(rctx, http.MethodGet, "https://"+addr+"/health", nil)
+				if err != nil {
+					return "", err
+				}
+				resp, err := (&http.Client{Transport: tr}).Do(req)
+				if err != nil {
+					return "", err
+				}
+				defer func() { _ = resp.Body.Close() }()
+				b, err := io.ReadAll(resp.Body)
+				return string(b), err
+			}
+
+			// PROPERTY 1 — precedence: both chains match a.b.foo.test; the
+			// longer matched suffix (*.b.foo.test) must serve.
+			if body, err := get(sniDeep); err != nil || body != "LONG\n" {
+				t.Errorf("%s: SNI %q matches both *.b.foo.test and *.foo.test: got body %q err %v, want %q — the longest matching suffix must win, not close the connection as ambiguous", order.name, sniDeep, body, err, "LONG\n")
+			}
+
+			// PROPERTY 2 — matched negative: only *.foo.test matches
+			// x.foo.test, so SHORT must serve (SHORT is live).
+			if body, err := get(sniShallow); err != nil || body != "SHORT\n" {
+				t.Errorf("%s: SNI %q matches only *.foo.test: got body %q err %v, want %q", order.name, sniShallow, body, err, "SHORT\n")
+			}
+		})
+	}
+}
```


## Appendix D — `internal/listener/quic.go`, comment-only `4 4` (Task 12)

```diff
diff --git a/internal/listener/quic.go b/internal/listener/quic.go
index f2b65936..30659b74 100644
--- a/internal/listener/quic.go
+++ b/internal/listener/quic.go
@@ -179,10 +179,10 @@ func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn) listenerfilter.
 // spec -> info mapping through rt.chainByName. SelectChain returns a
 // *ChainSpec, never a *chainInfo, so the map lookup is mandatory.
 //
-// Returns nil when no chain is selectable (SelectChain's
-// (nil, ErrNoChainMatched) branch: no indexed chain eligible AND no default
-// slot). A nil return is what makes serveQUICConnection close the connection,
-// mirroring the reference's "no filter chain found".
+// Returns nil when no chain is selectable: SelectChain's ErrNoChainMatched (no
+// indexed chain eligible AND no default slot) or ErrAmbiguousChainMatch (tied
+// eligible chains breakTie cannot separate on this connection's inputs). A nil
+// return is what makes serveQUICConnection close the connection.
 func (rt *listenerRuntime) selectQUICChain(conn *quic.Conn) *chainInfo {
 	spec, err := listenerfilter.SelectChain(rt.quicChainMatchInputs(conn), rt.chainSpecs, rt.defaultSpec)
 	if err != nil {
```


## Appendix E — fixture `0124-listener-sni-longest-suffix` (Tasks 6-8)

### E.1 `driver/driver.go` (609 lines)

```go
// Package driver registers the 0124-listener-sni-longest-suffix fixture with
// the differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 99, SPEC §7): when two or more filter chains'
// server_names patterns match one SNI, reference Envoy serves the chain whose
// MATCHING pattern is most specific — an exact name first, then the LONGEST
// matching "*." suffix — independent of declaration order and independent of
// any other, non-matching pattern the chain also lists. envoy-go ranked each
// chain's WHOLE pattern set (exact > suffix > universal) and, on a tie between
// two wildcard chains, CLOSED the connection; on a mixed set it served the
// chain that merely LISTED an exact name.
//
// Four TLS listeners, each with `listener_filters: [tls_inspector]`, every
// chain an HCM with a DISTINCT stat_prefix and a direct_response whose body
// names the listener and the chain:
//
//	listener       chains (declared order)                      SNI -> chain
//	l_long_first   LONG [*.b.foo.test], SHORT [*.foo.test]       a.b.foo.test -> LONG, x.foo.test -> SHORT
//	l_short_first  SHORT, LONG                                   a.b.foo.test -> LONG, x.foo.test -> SHORT
//	l_mixed        X [x.test, *.foo.test], Y [*.b.foo.test]      a.b.foo.test -> Y, q.foo.test -> X, x.test -> X
//	l_default      LONG, SHORT + default_filter_chain DEFAULT    a.b.foo.test -> LONG, nomatch.example -> DEFAULT
//
// ⚠️ l_long_first / l_short_first are the REVERSAL PAIR: identical but for
// declaration order, so a subject answering by order is red on exactly one.
//
// ⚠️ l_mixed a.b.foo.test is the ONLY row that tells "rank of the MATCHED
// pattern" (the reference, P2) from "rank of the chain's whole set + matched
// length" (P1): X's exact x.test does not match a.b.foo.test and must not help
// X. Deleting l_mixed leaves the fixture unable to see P1.
//
// ⚠️ The single-candidate rows (x.foo.test, q.foo.test, x.test,
// nomatch.example) are STRUCTURALLY GREEN at the un-fixed tip: only one chain
// is eligible, so no tie-break runs. They prove each losing chain is live and
// reachable; they are NOT evidence of precedence.
//
// ⚠️ NO ROW MAY BE A NO-MATCH CLOSE. nomatch.example reaches l_default's
// default chain. The reference books a TCP no-match close in
// no_filter_chain_match, a name envoy-go does not emit, so a close row could
// only be pinned vacuously.
//
// ⚠️ tls_inspector IS LOAD-BEARING. Without it neither side reads the SNI
// before chain selection and every server_names chain is ineligible.
package driver

import (
	"bufio"
	"bytes"
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0124-listener-sni-longest-suffix"

// refAdminPort is the harness-fixed in-container reference admin port.
const refAdminPort = 9901

// wantStatus is the direct_response status of every chain. NOT a 1xx (the Go
// client consumes a 1xx without surfacing it as the final status).
const wantStatus = 200

// armDeadline bounds one probe's dial + handshake + request + response.
const armDeadline = 5 * time.Second

// closedBody is what a probe records when the connection did not produce an
// HTTP response (handshake reset, EOF, timeout). It is a fixed token, NOT the
// error text: the two sides word a close differently, and the error text would
// make CompareBytes diverge on wording rather than on behavior.
const closedBody = "<CLOSED>"

// chain is one filter chain. isDefault marks the listener's
// default_filter_chain (no filter_chain_match, no server_names).
type chain struct {
	id          string   // short name used in failure text: LONG, SHORT, X, Y, DEFAULT
	prefix      string   // HCM stat_prefix — DISTINCT across the whole fixture
	serverNames []string // filter_chain_match.server_names
	isDefault   bool
}

// probe is one (SNI -> expected chain) row.
type probe struct {
	sni  string // lowercase only: case folding is out of scope
	want string // chain id
}

// listener is one listener under test. The roster order is the index-wise zip
// the runner performs between SubjectListenerNames() and
// ReferenceListenerPorts(); do not permute one accessor without the other.
type listener struct {
	name string

	// refPort is the IN-CONTAINER reference port. The runner publishes each
	// and hands back the Docker-assigned host mapping per listener.
	//
	// CENSUSED at this tip: 15124 is the `15000 + <index>` convention slot
	// (reserved for this fixture in 0123's prose); 15225-15227 read zero
	// files under `git grep -lw -- <port> -- test/ internal/ cmd/`.
	refPort int

	chains []chain
	probes []probe
}

func body(l, c string) string { return l + "/" + c + "\n" }

var (
	long = func(p string) chain { return chain{id: "LONG", prefix: p, serverNames: []string{"*.b.foo.test"}} }
	shrt = func(p string) chain { return chain{id: "SHORT", prefix: p, serverNames: []string{"*.foo.test"}} }
)

// listeners is the roster. 🔴 ALL NINE stat_prefixes MUST BE DISTINCT: two
// HCMs sharing one prefix panic envoy-go at boot with a duplicate metric
// registration.
var listeners = []listener{
	{
		name:    "l_long_first",
		refPort: 15124,
		chains:  []chain{long("lf_long"), shrt("lf_short")},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"x.foo.test", "SHORT"}},
	},
	{
		name:    "l_short_first",
		refPort: 15225,
		chains:  []chain{shrt("sf_short"), long("sf_long")},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"x.foo.test", "SHORT"}},
	},
	{
		name:    "l_mixed",
		refPort: 15226,
		chains: []chain{
			{id: "X", prefix: "mx_x", serverNames: []string{"x.test", "*.foo.test"}},
			{id: "Y", prefix: "mx_y", serverNames: []string{"*.b.foo.test"}},
		},
		probes: []probe{{"a.b.foo.test", "Y"}, {"q.foo.test", "X"}, {"x.test", "X"}},
	},
	{
		name:    "l_default",
		refPort: 15227,
		chains:  []chain{long("df_long"), shrt("df_short"), {id: "DEFAULT", prefix: "df_default", isDefault: true}},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"nomatch.example", "DEFAULT"}},
	},
}

func init() { fixture.RegisterFixture(fixtureName, &sniDriver{}) }

// sniDriver is STATEFUL: the Drive methods record each side's per-row
// observation and AssertStats asserts every row with one Errorf per property.
// A Drive that returned an error on a wrong or closed row would become
// t.Fatalf in the runner and MASK every later row's verdict.
type sniDriver struct {
	mu  sync.Mutex
	obs map[string]map[string]observation // side -> "listener sni" -> observation
}

type observation struct {
	status int
	body   string
	err    string
}

var (
	_ fixture.Driver              = (*sniDriver)(nil)
	_ fixture.MultiListenerDriver = (*sniDriver)(nil)
	_ fixture.StatsAsserter       = (*sniDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: the runner rejects 0, and envoy-go boot-rejects an absent
// static_resources.clusters key, so both bootstraps carry a never-dialed
// c_unused cluster pointed at this port.
func (*sniDriver) BackendCount() int { return 1 }

// SubjectListenerName / ReferenceListenerPort return listener[0]; the runner
// computes the primary addresses before its multi-listener branch.
func (*sniDriver) SubjectListenerName() string { return listeners[0].name }
func (*sniDriver) ReferenceListenerPort() int  { return listeners[0].refPort }

func (*sniDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig: the subject listener ports are subjListenerPort + i. The
// runner allocates the base via freeTCPPortBlock, which probes [base, base+16)
// bindable; four of sixteen is inside the reservation (the 0123 derivation).
func (*sniDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject drive listener[0] only. UNREACHABLE while
// MultiListenerDriver is implemented.
func (d *sniDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "ref", map[string]string{listeners[0].name: addr}, listeners[:1])
}

func (d *sniDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "subj", map[string]string{listeners[0].name: addr}, listeners[:1])
}

func (*sniDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

func (*sniDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*sniDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *sniDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "ref", addrs, listeners)
}

func (d *sniDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "subj", addrs, listeners)
}

// driveAll issues ONE TLS round trip per probe row, each on a fresh
// connection with an explicit ServerName, and emits a side-label-free byte
// stream for CompareBytes. Only a missing address returns an error; a closed
// or wrong-chain row is RECORDED and asserted in AssertStats.
func (d *sniDriver) driveAll(ctx context.Context, side string, addrs map[string]string, roster []listener) ([]byte, error) {
	pool, err := serverCAPool()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	seen := map[string]observation{}
	for _, l := range roster {
		addr := addrs[l.name]
		if addr == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
		for _, p := range l.probes {
			o := tlsGet(ctx, pool, addr, p.sni)
			log.Printf("%s: %s %s sni=%s status=%d body=%q err=%q", fixtureName, side, l.name, p.sni, o.status, o.body, o.err)
			fmt.Fprintf(&b, "listener %s sni %s status=%d body=%q\n", l.name, p.sni, o.status, o.body)
			seen[l.name+" "+p.sni] = o
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.obs == nil {
		d.obs = map[string]map[string]observation{}
	}
	d.obs[side] = seen
	return b.Bytes(), nil
}

func (d *sniDriver) recorded(side string) map[string]observation {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.obs[side]
}

// tlsGet is one row: fresh TCP connection, TLS handshake with ServerName =
// sni verified against the committed CA, one HTTP/1.1 GET, Connection: close.
// Any failure records body = closedBody and the error text.
func tlsGet(ctx context.Context, pool *x509.CertPool, addr, sni string) observation {
	fail := func(err error) observation { return observation{body: closedBody, err: err.Error()} }
	raw, err := (&net.Dialer{Timeout: armDeadline}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fail(fmt.Errorf("dial: %w", err))
	}
	defer func() { _ = raw.Close() }()
	if err := raw.SetDeadline(time.Now().Add(armDeadline)); err != nil {
		return fail(err)
	}
	conn := stdtls.Client(raw, &stdtls.Config{
		RootCAs:    pool,
		ServerName: sni,
		MinVersion: stdtls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		return fail(fmt.Errorf("tls handshake: %w", err))
	}
	req := "GET / HTTP/1.1\r\nHost: " + sni + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return fail(fmt.Errorf("write: %w", err))
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return fail(fmt.Errorf("read response: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	bb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fail(fmt.Errorf("read body: %w", err))
	}
	return observation{status: resp.StatusCode, body: string(bb)}
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, per row: status and the BODY (primary), then
// per chain the VALUE of http.<prefix>.downstream_rq_total, which must equal
// the number of rows expected to land on that chain (0 for a chain no row
// targets). Every counter pin is guarded by a presence check: a missing key
// reads 0 and would make a `== 0` pin vacuous.
//
// Errorf per property; Fatalf only for a failed scrape or a missing drive.
func (d *sniDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	for _, side := range []struct{ name, addr string }{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		st, err := scrapeHCMTotals(side.addr)
		if err != nil {
			t.Fatalf("%s: scrape: %v", side.name, err)
		}
		obs := d.recorded(side.name)
		if len(obs) == 0 {
			t.Fatalf("%s: no drive observations recorded — every assertion below would be vacuous", side.name)
			return
		}
		for _, l := range listeners {
			want := map[string]uint64{}
			for _, p := range l.probes {
				want[p.want]++
				assertRow(t, side.name, l, p, obs)
			}
			for _, c := range l.chains {
				name := "http." + c.prefix + ".downstream_rq_total"
				v, ok := st[name]
				log.Printf("%s: %s %s %s=%d(present=%t) want=%d", fixtureName, side.name, l.name, name, v, ok, want[c.id])
				if !ok {
					t.Errorf("%s %s: %s ABSENT from /stats/prometheus, want present and == %d", side.name, l.name, name, want[c.id])
					continue
				}
				if v != want[c.id] {
					t.Errorf("%s %s: %s = %d, want %d (chain %s)", side.name, l.name, name, v, want[c.id], c.id)
				}
			}
		}
	}
}

func assertRow(t fixture.TB, side string, l listener, p probe, obs map[string]observation) {
	t.Helper()
	got, ok := obs[l.name+" "+p.sni]
	if !ok {
		t.Errorf("%s %s sni=%s: no observation recorded", side, l.name, p.sni)
		return
	}
	wantBody := body(l.name, p.want)
	if got.body == closedBody {
		t.Errorf("%s %s sni=%s: connection CLOSED without a response (%s), want chain %s (body %q)",
			side, l.name, p.sni, got.err, p.want, wantBody)
		return
	}
	if got.status != wantStatus {
		t.Errorf("%s %s sni=%s: status = %d, want %d", side, l.name, p.sni, got.status, wantStatus)
	}
	if got.body != wantBody {
		t.Errorf("%s %s sni=%s: served body %q, want %q — the chain whose MATCHING server_names pattern is "+
			"most specific (exact, then longest *. suffix) must serve, regardless of declaration order",
			side, l.name, p.sni, got.body, wantBody)
	}
}

// scrapeHCMTotals fetches /stats/prometheus and RE-PROJECTS every
// envoy_http_downstream_rq_total sample onto its dotted name
// http.<envoy_http_conn_manager_prefix>.downstream_rq_total (the 0005
// precedent). Other metrics are ignored.
func scrapeHCMTotals(adminAddr string) (map[string]uint64, error) {
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
	const metric = "envoy_http_downstream_rq_total{"
	const label = `envoy_http_conn_manager_prefix="`
	out := map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, metric) {
			continue
		}
		closeIdx := strings.LastIndexByte(line, '}')
		if closeIdx < 0 {
			continue
		}
		labels := line[len(metric):closeIdx]
		i := strings.Index(labels, label)
		if i < 0 {
			continue
		}
		prefix := labels[i+len(label):]
		j := strings.IndexByte(prefix, '"')
		if j < 0 {
			continue
		}
		prefix = prefix[:j]
		val := strings.Fields(line[closeIdx+1:])
		if len(val) == 0 {
			continue
		}
		f, err := strconv.ParseFloat(val[0], 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			continue
		}
		out["http."+prefix+".downstream_rq_total"] += uint64(f)
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap builds one side's complete bootstrap. BOTH sides use this one
// renderer, differing only in bind address, admin port and listener ports, so
// the listener/chain shape is identical by construction. Certificates are
// inline_string on both sides (the reference container cannot read host
// filename: paths).
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "admin:\n  address:\n    socket_address: { address: %s, port_value: %d }\nstatic_resources:\n  listeners:\n", bindAddr, adminPort)
	for i, l := range listeners {
		fmt.Fprintf(&b, listenerHeadTmpl, l.name, bindAddr, portFor(i))
		var def *chain
		for ci := range l.chains {
			c := l.chains[ci]
			if c.isDefault {
				def = &l.chains[ci]
				continue
			}
			quoted := make([]string, len(c.serverNames))
			for k, n := range c.serverNames {
				quoted[k] = strconv.Quote(n)
			}
			b.WriteString("        - name: fc_" + c.prefix + "\n")
			b.WriteString("          filter_chain_match:\n")
			b.WriteString("            server_names: [" + strings.Join(quoted, ", ") + "]\n")
			b.WriteString(indent(chainBody(l.name, c), 10))
		}
		if def != nil {
			b.WriteString("      default_filter_chain:\n")
			b.WriteString("        name: fc_" + def.prefix + "\n")
			b.WriteString(indent(chainBody(l.name, *def), 8))
		}
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

// chainBody renders one chain's transport_socket + filters at column 0; the
// caller indents it, and indent() splices the PEMs in at their final column.
func chainBody(listenerName string, c chain) string {
	return fmt.Sprintf(chainTmpl, c.prefix, strconv.Quote(body(listenerName, c.id)))
}

// indent prefixes every line with n spaces, replacing the @@CERT@@ / @@KEY@@
// placeholder lines with the PEM indented to the placeholder's own final
// column (n + 14), so the block scalar stays well-formed at any depth.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		switch strings.TrimSpace(ln) {
		case "@@CERT@@":
			lines[i] = indentPEM(mustReadFixtureBytes("pki/server.pem"), n+14)
		case "@@KEY@@":
			lines[i] = indentPEM(mustReadFixtureBytes("pki/server.key.pem"), n+14)
		default:
			lines[i] = pad + ln
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// listenerHeadTmpl: name, bind address, port. tls_inspector is LOAD-BEARING.
const listenerHeadTmpl = `    - name: %s
      address:
        socket_address: { address: %s, port_value: %d }
      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
      filter_chains:
`

// chainTmpl: 1 stat_prefix, 2 quoted body. Rendered at column 0.
const chainTmpl = `transport_socket:
  name: envoy.transport_sockets.tls
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
    common_tls_context:
      alpn_protocols: ["http/1.1"]
      tls_certificates:
        - certificate_chain:
            inline_string: |
              @@CERT@@
          private_key:
            inline_string: |
              @@KEY@@
filters:
  - name: envoy.filters.network.http_connection_manager
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
      codec_type: HTTP1
      stat_prefix: %[1]s
      route_config:
        name: rc_%[1]s
        virtual_hosts:
          - name: vh_%[1]s
            domains: ["*"]
            routes:
              - match: { prefix: "/" }
                direct_response:
                  status: 200
                  body: { inline_string: %[2]s }
      http_filters:
        - name: envoy.filters.http.router
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// clustersTmpl: the never-dialed placeholder cluster. Arg: backend port.
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

// --- file helpers (the 0121 idiom) ---

func serverCAPool() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(mustReadFixtureBytes("pki/ca.pem")) {
		return nil, errors.New("pki/ca.pem: no certificate appended")
	}
	return pool, nil
}

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureBytes(name string) []byte {
	b, err := os.ReadFile(filepath.Join(fixtureDir(), filepath.FromSlash(name))) //nolint:gosec // fixture-relative, test-only
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return b
}

// indentPEM prefixes every line of a PEM with `spaces` spaces for a YAML block
// scalar; the trailing newline is trimmed first.
func indentPEM(pemBytes []byte, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(string(pemBytes), "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}
```


### E.2 `pki/gen/main.go` (169 lines). Run `go run ./pki/gen` from the fixture directory to write the three PEMs, deterministically

```go
// Package main regenerates fixture 0124-listener-sni-longest-suffix's TLS PKI
// deterministically.
//
// Usage (from the repo root):
//
//	cd test/fixtures/0124-listener-sni-longest-suffix && go run ./pki/gen
//
// Produces byte-identical PEMs on every run. CI never invokes this command;
// the committed PEMs are authoritative. Mirrors fixture 0121's generator
// except for the seed bytes, the serial, and the leaf's SAN set.
//
// ONE leaf serves EVERY chain of EVERY listener. Which certificate was served
// is out of scope (SPEC §2.4): the fixture discriminates the SERVING CHAIN by
// its direct_response body, never by the certificate.
package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
)

var (
	notBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter  = time.Date(2046, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Deterministic seed for this fixture's PKI. Flipping any byte invalidates
// every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x01, 0x24, 0x5a, 0xc1, 0x7a, 0x4b, 0x91, 0x2e,
	0x63, 0xb8, 0x05, 0xd7, 0xaa, 0x1c, 0x38, 0xf6,
	0x52, 0x9d, 0x14, 0xe0, 0x6b, 0xc3, 0x77, 0x28,
	0x8f, 0x40, 0xa5, 0xd9, 0x11, 0x36, 0xec, 0x74,
}

var serials = map[string]int64{
	"server": 124,
}

// newChaCha8 returns a ChaCha8 PRNG seeded from the master seed XOR'd with the
// tag bytes. Each (tag, role) pair gets an independent deterministic stream.
func newChaCha8(tag string) *rand.ChaCha8 {
	var s [32]byte
	copy(s[:], seed[:])
	for i, b := range []byte(tag) {
		s[i%32] ^= b
	}
	return rand.NewChaCha8(s)
}

// genKey generates a deterministic P-256 ECDSA private key.
//
// Mirrors fixture-0002/0004's genKey verbatim: rejection-samples a 32-byte
// scalar from the ChaCha8 stream and hands it to ecdh.P256().NewPrivateKey,
// which is the ONE construction path that bypasses Go 1.26's CustomReader
// DRBG-replace behavior.
func genKey(tag string) *ecdsa.PrivateKey {
	rng := newChaCha8(tag + "-key")
	var scalar [32]byte
	var buf [8]byte
	for {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint64(buf[:], rng.Uint64())
			copy(scalar[i*8:], buf[:])
		}
		ecdhKey, err := ecdh.P256().NewPrivateKey(scalar[:])
		if err != nil {
			continue // scalar was zero or >= n; astronomically rare
		}
		curve := elliptic.P256()
		d := new(big.Int).SetBytes(ecdhKey.Bytes())
		pub := ecdhKey.PublicKey().Bytes()
		byteLen := (curve.Params().BitSize + 7) / 8
		x := new(big.Int).SetBytes(pub[1 : 1+byteLen])
		y := new(big.Int).SetBytes(pub[1+byteLen:])
		return &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
			D:         d,
		}
	}
}

func main() {
	outDir := "pki"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	outDir = filepath.Clean(outDir)
	must(os.MkdirAll(outDir, 0o755))

	caKey, caPEM, caCert := genCA("ca")
	writePEM(filepath.Join(outDir, "ca.pem"), caPEM)

	// ⚠️ The leaf MUST carry a DNS SAN covering EVERY SNI the driver dials,
	// or that arm fails verification CLIENT-side and reads as a server CLOSE
	// with no server-side fault at all (a vacuous red that looks like the
	// real one). Go's verifier matches a wildcard against ONE label, so:
	//   *.foo.test      covers x.foo.test, q.foo.test
	//   *.b.foo.test    covers a.b.foo.test
	//   x.test          covers x.test
	//   nomatch.example covers the l_default fall-through arm
	// *.c.b.foo.test is harmless and kept for the unit-only M3/M4 shape.
	leafDNS := []string{"*.foo.test", "*.b.foo.test", "*.c.b.foo.test", "x.test", "nomatch.example"}

	genLeaf(outDir, "server", "envoy-go 0124 sni-longest-suffix listener", leafDNS, caCert, caKey)

	fmt.Println("ok: 3 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go 0124 sni-longest-suffix fixture CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation.
	der, err := x509.CreateCertificate(nil, tmpl, tmpl, &key.PublicKey, key)
	must(err)
	cert, err := x509.ParseCertificate(der)
	must(err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert
}

func genLeaf(outDir, tag, cn string, dnsNames []string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serials[tag]),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation.
	der, err := x509.CreateCertificate(nil, tmpl, caCert, &key.PublicKey, caKey)
	must(err)
	writePEM(filepath.Join(outDir, tag+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	must(err)
	writePEM(filepath.Join(outDir, tag+".key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func writePEM(path string, pemBytes []byte) {
	must(os.WriteFile(path, pemBytes, 0o644)) //nolint:gosec // fixture PEMs are public test material
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
```


### E.3 `README.md`

````markdown
# 0124-listener-sni-longest-suffix

Phase 99 (`chain-match-sni-longest-suffix`), SPEC §7. This fixture checks
which filter chain serves when **two or more chains' `server_names` patterns
match one SNI**.

## The proposition

Reference Envoy serves the chain whose **matching** pattern is the most
specific one. An exact name ranks first, then the **longest** matching `*.`
suffix. Declaration order does not matter. Any pattern the chain lists that
does *not* match the SNI does not count either. At the un-fixed tip, envoy-go
ranked each chain's **whole** pattern set (exact > suffix > universal):

- two wildcard chains that match one SNI tied, and envoy-go **closed** the
  connection (the TLS handshake got EOF);
- when a mixed set listed an exact name that does not match, envoy-go served
  that chain anyway.

## Topology

Four TLS listeners. Every listener has `listener_filters: [tls_inspector]`.
Each chain is an HCM with its own `stat_prefix` and a `direct_response` 200
whose body is `<listener>/<CHAIN>\n`. All chains share one leaf certificate
(`pki/`, SANs `*.foo.test`, `*.b.foo.test`, `*.c.b.foo.test`, `x.test`,
`nomatch.example`). Both sides deliver it through `inline_string:`.

| listener | ref port | chains (declared order) | SNI -> chain |
|---|---|---|---|
| `l_long_first` | 15124 | LONG `*.b.foo.test`, SHORT `*.foo.test` | a.b.foo.test -> LONG; x.foo.test -> SHORT |
| `l_short_first` | 15225 | SHORT, LONG | a.b.foo.test -> LONG; x.foo.test -> SHORT |
| `l_mixed` | 15226 | X `[x.test, *.foo.test]`, Y `[*.b.foo.test]` | a.b.foo.test -> Y; q.foo.test -> X; x.test -> X |
| `l_default` | 15227 | LONG, SHORT + `default_filter_chain` DEFAULT | a.b.foo.test -> LONG; nomatch.example -> DEFAULT |

The subject listener ports are `subjListenerPort + i` (i = 0..3), inside the
16-port block that `freeTCPPortBlock` probes. This is the 0123 derivation.

## Load-bearing design points

- **The reversal pair.** `l_long_first` and `l_short_first` differ only in
  declaration order. A subject that chooses by order is red on exactly one of
  them.
- **The P1-vs-P2 discriminator is `l_mixed` a.b.foo.test.** X's exact
  `x.test` does not match, so it must not help X. P1 ranks the whole set and
  only then compares matched length, so it serves X and is red **on this row
  only**. Delete `l_mixed` and the fixture cannot see P1.
- **The single-candidate rows are structurally green.** These are x.foo.test,
  q.foo.test, x.test and nomatch.example. Only one chain is eligible, so no
  tie-break runs, and they are green at the un-fixed tip. They prove that each
  losing chain is live. They are **not** evidence of precedence.
- **No row is a no-match close.** nomatch.example lands on `l_default`'s
  default chain. The reference books a TCP no-match close in
  `no_filter_chain_match`, and envoy-go does not emit that name, so a close
  row could only be pinned vacuously. `downstream_cx_total` is not pinned
  across sides.
- **`tls_inspector` is required.** Without it neither side reads the SNI
  before chain selection.
- The driver sets `ServerName` explicitly on every request and verifies the
  leaf against `pki/ca.pem`. All SNIs are lowercase.
- All nine `stat_prefix`es are distinct. A shared prefix panics envoy-go at
  boot with a duplicate metric registration.

## Assertions (driver `AssertStats`, per side, Errorf per property)

1. Per (listener, SNI): the connection was not closed, the status is 200, and
   the body is `<listener>/<want>\n`.
2. Per chain: the **value** of `http.<prefix>.downstream_rq_total` equals the
   number of rows that target that chain (0 for a chain no row targets). The
   value is scraped from `/stats/prometheus` as
   `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<prefix>"}`
   and re-projected. Both sides emit every name at boot (value 0), so the
   check pins values. Checking only that a name is present would be vacuous.
   It is still guarded by a presence check.
3. The runner's cross-side `CompareBytes` compares the per-row
   `listener/sni/status/body` stream. A close is recorded as the fixed token
   `<CLOSED>`, never as the error text.

## Measured (prototype, phase-99 PLAN)

| arm | red rows (subject) | fixture |
|---|---|---|
| un-fixed tip | a.b on l_long_first, l_short_first, l_default = CLOSED; l_mixed a.b = X | RED |
| P2 (the fix) | none | GREEN (3 runs) |
| P1 | l_mixed a.b = X | RED |
| inverted P2 | a.b on l_long_first, l_short_first, l_default = SHORT; l_mixed a.b = X | RED |

The reference side was green in every run.
````


### E.4 `expectations.yaml`

```yaml
# Phase 99 fixture 0124-listener-sni-longest-suffix expectations (ADR-0019 —
# prose; no code path reads this file. The enforcers are driver/driver.go's
# per-side AssertStats, the runner's cross-side CompareBytes and the admin
# probe).
#
# ## Topology — FOUR TLS listeners, each `listener_filters: [tls_inspector]`
#
#   listener       ref port  chains (declared order)                     server_names
#   l_long_first   15124     fc_lf_long, fc_lf_short                     [*.b.foo.test], [*.foo.test]
#   l_short_first  15225     fc_sf_short, fc_sf_long                     [*.foo.test], [*.b.foo.test]
#   l_mixed        15226     fc_mx_x, fc_mx_y                            [x.test, *.foo.test], [*.b.foo.test]
#   l_default      15227     fc_df_long, fc_df_short + default fc_df_default
#
#   Subject ports: subjListenerPort + 0..3 (0123 derivation). One shared leaf
#   (pki/), inline_string: on both sides. c_unused: never-dialed STATIC
#   placeholder cluster (runner needs BackendCount >= 1; envoy-go boot-rejects
#   an absent clusters key).
#
# ## Workload — ONE fresh TLS connection + GET / per row, explicit ServerName
#
# ## Asserted, BOTH sides, ABSOLUTELY (status 200 + body):
#
#   l_long_first   a.b.foo.test    -> "l_long_first/LONG\n"      RED at tip (subject CLOSES)
#   l_long_first   x.foo.test      -> "l_long_first/SHORT\n"     single-candidate (green at tip)
#   l_short_first  a.b.foo.test    -> "l_short_first/LONG\n"     RED at tip (subject CLOSES)
#   l_short_first  x.foo.test      -> "l_short_first/SHORT\n"    single-candidate
#   l_mixed        a.b.foo.test    -> "l_mixed/Y\n"              RED at tip AND under P1 (subject serves X)
#   l_mixed        q.foo.test      -> "l_mixed/X\n"              single-candidate
#   l_mixed        x.test          -> "l_mixed/X\n"              single-candidate
#   l_default      a.b.foo.test    -> "l_default/LONG\n"         RED at tip (subject CLOSES)
#   l_default      nomatch.example -> "l_default/DEFAULT\n"      default chain; NOT a no-match close
#
# ## Asserted counters (VALUE, presence-guarded), re-projected from
# ## envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<p>"}:
#
#   http.lf_long.downstream_rq_total    1    http.lf_short.downstream_rq_total   1
#   http.sf_short.downstream_rq_total   1    http.sf_long.downstream_rq_total    1
#   http.mx_x.downstream_rq_total       2    http.mx_y.downstream_rq_total       1
#   http.df_long.downstream_rq_total    1    http.df_short.downstream_rq_total   0
#   http.df_default.downstream_rq_total 1
#
# ## NOT asserted
#
#   no_filter_chain_match (envoy-go emits no such name), downstream_cx_total
#   cross-side, which certificate was served, mixed-case SNI, QUIC (carried by
#   the unit arm; the harness cannot mix TCP + UDP listeners).
```


## Appendix F — the NC mutants (Task 13). Rows 1, 2 and 4 are applied to the post-Task-12 tree; row 3 to MASTER's `chainmatch.go`

**F.1 — row 1, `inv`.** In slot 2, reverse the two suffix-length compares:

```go
		if la < lb { // MUTANT: shorter matched suffix wins
			return a
		}
		if lb < la {
			return b
		}
```

**F.2 — row 2, `rank-mut`.** In slot 2, reverse the two rank compares:

```go
		if rb < ra { // MUTANT: was ra < rb
			return a
		} // lower rank = more specific
		if ra < rb { // MUTANT: was rb < ra
			return b
		}
```

**F.3 — row 3, P1 (whole-set rank + matched length).** Scored exactly as it was measured, on MASTER's file
with P1 applied (the tests stay in place):

```sh
git -C "$W" show master:internal/listener/listenerfilter/chainmatch.go > "$W"/internal/listener/listenerfilter/chainmatch.go
git -C "$W" apply "$SCRATCH/p1.diff"     # the diff below, saved to scratch first
```

```diff
diff --git a/internal/listener/listenerfilter/chainmatch.go b/internal/listener/listenerfilter/chainmatch.go
index 1cafa6ec..72a88ab4 100644
--- a/internal/listener/listenerfilter/chainmatch.go
+++ b/internal/listener/listenerfilter/chainmatch.go
@@ -220,6 +220,16 @@ func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec {
 		if rb < ra {
 			return b
 		}
+		if ra == 1 {
+			la := longestWildcardSuffix(a.ServerNames, inputs.ServerName)
+			lb := longestWildcardSuffix(b.ServerNames, inputs.ServerName)
+			if la > lb {
+				return a
+			}
+			if lb > la {
+				return b
+			}
+		}
 	}
 	// Slot 6 — SourcePrefixRanges: longer prefix wins.
 	if len(a.SourcePrefixRanges) > 0 && len(b.SourcePrefixRanges) > 0 {
@@ -332,3 +342,15 @@ func sniSpecificityRank(patterns []string) int {
 	}
 	return rank
 }
+
+// longestWildcardSuffix returns the length of the longest "*." pattern in
+// patterns that matches sni; 0 if none.
+func longestWildcardSuffix(patterns []string, sni string) int {
+	best := 0
+	for _, p := range patterns {
+		if strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:]) && len(p) > best {
+			best = len(p)
+		}
+	}
+	return best
+}
```

**F.4 — row 4, `drop-len`, the COMPILING form (§0.2).** Delete the two suffix-length `if` blocks, **and**
rewrite the two assignments so the build still succeeds:

```go
		ra, _ := sniMatchedRank(a.ServerNames, inputs.ServerName) // MUTANT: length discarded
		rb, _ := sniMatchedRank(b.ServerNames, inputs.ServerName)
```

⚠️ **Before scoring any row, run `go vet ./internal/listener/...` and require rc 0.** A mutant that fails to
build reddens every arm for a reason unrelated to the mutation.


## Appendix G — the compressor re-point, `internal/filter/http/compressor/compressor_test.go` `53 49` (Task 2)

```diff
diff --git a/internal/filter/http/compressor/compressor_test.go b/internal/filter/http/compressor/compressor_test.go
index d2bf9bc9..6e1b0cc8 100644
--- a/internal/filter/http/compressor/compressor_test.go
+++ b/internal/filter/http/compressor/compressor_test.go
@@ -2109,63 +2109,67 @@ func TestEncodeData_AllowPath_GzipEncodes_OverwriteBodyCalled_CountersIncremente
 	}
 }
 
-func TestEncodeData_LevelMapping_DifferentGzippedSizes(t *testing.T) {
-	// Sanity check that f.config.gzip.level threads through to gzip.NewWriterLevel:
-	// BestSpeed (1) vs BestCompression (9) on a compressible body produce
-	// different compressed-byte sizes. Both round-trip correctly. Per
-	// ADR-0130 §Decision (iv): the compression_level enum maps to int that
-	// passes through to compress/gzip verbatim.
+func TestEncodeData_LevelMapping_LevelReachesEncoder(t *testing.T) {
+	// Asserts that f.config.gzip.level is PLUMBED to gzip.NewWriterLevel, not
+	// that the stdlib emits different sizes per level: compress/flate's output
+	// per level is a toolchain property (go1.27.1's rewritten fast encoders
+	// emit 2121 bytes at BOTH level 1 and level 9 on this input). The
+	// discriminator is the gzip header's XFL byte (RFC 1952 §2.3.1, offset 8),
+	// which compress/gzip derives from the level the Writer was built at:
+	// 4 at BestSpeed, 2 at BestCompression, 0 otherwise. A level dropped on
+	// the way to the encoder (e.g. forced to DefaultCompression) yields XFL 0.
+	// Per ADR-0130 §Decision (iv) the level passes to compress/gzip verbatim;
+	// the enum→int mapping itself is pinned by the Group 3 tests.
 	body := make([]byte, 4096)
 	for i := range body {
-		// Non-repetitive enough that level choice meaningfully affects output.
 		body[i] = byte((i * 17) ^ (i >> 3))
 	}
 
-	encodeWithLevel := func(level int) []byte {
-		t.Helper()
-		cc := defaultCompiledConfig()
-		cc.gzip = &compiledGzipConfig{level: level}
-		f, cb := freshEncodeDataFilter(t, cc)
-		f.willCompress = true
-		if status := f.EncodeData(body, true); status != envoyhttp.DataContinue {
-			t.Fatalf("status @ level=%d = %v; want DataContinue", level, status)
-		}
-		if cb.overwriteBodyCallCount != 1 {
-			t.Fatalf("OverwriteBody calls @ level=%d = %d; want 1", level, cb.overwriteBodyCallCount)
-		}
-		return cb.overwriteBodyCalls[0]
-	}
-
-	bestSpeed := encodeWithLevel(gzip.BestSpeed)
-	bestComp := encodeWithLevel(gzip.BestCompression)
-
-	// Different levels should produce different compressed sizes on this input.
-	if len(bestSpeed) == len(bestComp) {
-		t.Errorf("expected different compressed sizes for BestSpeed vs BestCompression on a non-repetitive input; both = %d", len(bestSpeed))
-	}
-
-	// Both must round-trip correctly.
 	for _, tc := range []struct {
-		name string
-		data []byte
+		name    string
+		level   int
+		wantXFL byte
 	}{
-		{"BestSpeed", bestSpeed},
-		{"BestCompression", bestComp},
+		{"BestSpeed", gzip.BestSpeed, 4},
+		{"BestCompression", gzip.BestCompression, 2},
 	} {
-		gr, err := gzip.NewReader(bytes.NewReader(tc.data))
-		if err != nil {
-			t.Errorf("gzip.NewReader @ %s: %v", tc.name, err)
-			continue
-		}
-		decompressed, err := io.ReadAll(gr)
-		if err != nil {
-			t.Errorf("io.ReadAll @ %s: %v", tc.name, err)
-			continue
-		}
-		_ = gr.Close()
-		if !bytes.Equal(decompressed, body) {
-			t.Errorf("round-trip mismatch @ %s", tc.name)
-		}
+		t.Run(tc.name, func(t *testing.T) {
+			cc := defaultCompiledConfig()
+			cc.gzip = &compiledGzipConfig{level: tc.level}
+			// Two responses through ONE compiledGzipConfig: the second
+			// acquires the pooled writer (Reset path), which must keep the
+			// configured level too.
+			for i := 0; i < 2; i++ {
+				f, cb := freshEncodeDataFilter(t, cc)
+				f.willCompress = true
+				if status := f.EncodeData(body, true); status != envoyhttp.DataContinue {
+					t.Fatalf("response %d: status = %v; want DataContinue", i, status)
+				}
+				if cb.overwriteBodyCallCount != 1 {
+					t.Fatalf("response %d: OverwriteBody calls = %d; want 1", i, cb.overwriteBodyCallCount)
+				}
+				got := cb.overwriteBodyCalls[0]
+				if len(got) < 10 {
+					t.Fatalf("response %d: output %d bytes; shorter than a gzip header", i, len(got))
+				}
+				if got[8] != tc.wantXFL {
+					t.Errorf("response %d: gzip XFL = %d; want %d (level %d did not reach the encoder)",
+						i, got[8], tc.wantXFL, tc.level)
+				}
+				gr, err := gzip.NewReader(bytes.NewReader(got))
+				if err != nil {
+					t.Fatalf("response %d: gzip.NewReader: %v", i, err)
+				}
+				decompressed, err := io.ReadAll(gr)
+				if err != nil {
+					t.Fatalf("response %d: io.ReadAll: %v", i, err)
+				}
+				_ = gr.Close()
+				if !bytes.Equal(decompressed, body) {
+					t.Errorf("response %d: round-trip mismatch", i)
+				}
+			}
+		})
 	}
 }
 
```
