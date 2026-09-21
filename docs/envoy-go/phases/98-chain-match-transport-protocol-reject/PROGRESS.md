# Phase 98 — chain-match `transport_protocol` reject — IMPL PROGRESS

| What | Value |
| --- | --- |
| Phase | 98 — `chain-match-transport-protocol-reject` |
| Stage | **IMPL** |
| Worktree | `/home/esa/git/wt-phase-98-impl` |
| Branch | `phase-98-impl` |
| Base commit | `9c7bee0b` (`9c7bee0b02289c7df5ace3f8f5c8a89d1ab7668f`) |

Every figure in this document is stated beside the command that produced it, and
every one of them was RUN. Nothing is quoted from the BRAINSTORM, the SPEC or the
PLAN as a measurement.

Environment check — `type grep` reports **`grep is a function`** (a `ugrep`
wrapper that honours `.gitignore`), so every grep figure below was produced with
`/usr/bin/grep`.

---

## Task 1 — un-fixed-tip baseline and panic-gate liveness

⚠️ **The baseline recorded here is the evidence Task 10 diffs against, and it is
UNRECOVERABLE after Task 11** — once the production repair lands, the un-fixed tip
cannot be re-measured in this worktree.

### Tip and cleanliness, before anything ran

| What | Command | Output |
| --- | --- | --- |
| Worktree | `pwd` (after `cd /home/esa/git/wt-phase-98-impl`) | `/home/esa/git/wt-phase-98-impl` |
| Branch | `git -C /home/esa/git/wt-phase-98-impl rev-parse --abbrev-ref HEAD` | `phase-98-impl` |
| Tip SHA | `git -C /home/esa/git/wt-phase-98-impl rev-parse HEAD` | `9c7bee0b02289c7df5ace3f8f5c8a89d1ab7668f` |
| Working tree | `git -C /home/esa/git/wt-phase-98-impl status --porcelain` | *(empty — clean)* |

### Step 1 — selector resolution

A `go test` selector naming a package that does not exist prints
`FAIL … [setup failed]` and exits 1, which reads exactly like a real failure. The
selectors were therefore resolved BEFORE the baseline run.

`cd /home/esa/git/wt-phase-98-impl && go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...` — **RC=0**, **SEVEN** packages:

```
github.com/pgdad/envoy-go/cmd/envoy-go
github.com/pgdad/envoy-go/internal/admin
github.com/pgdad/envoy-go/internal/boot
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
github.com/pgdad/envoy-go/validate
```

Note the selector expands to seven packages, not five: `./internal/listener/...`
contributes `listenerfilter` and `listenerfilter/tls_inspector` beyond
`internal/listener` itself. Any later `[setup failed]` from this selector is a
REAL failure.

### Step 2 — the un-fixed-tip baseline

Run in the FOREGROUND, `-count=1 -v`, all seven packages in one invocation:

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > $SCRATCH/base.txt 2>&1
rc=${PIPESTATUS[0]}; echo "RC=$rc"
```

| Figure | Command | Value |
| --- | --- | --- |
| RC | `rc=${PIPESTATUS[0]}; echo "RC=$rc"` | **0** |
| `=== RUN` count | `/usr/bin/grep -c '=== RUN' $SCRATCH/base.txt` | **392** |
| anchored FAIL | `/usr/bin/grep -cE '^(FAIL\|--- FAIL)\|^ *--- FAIL' $SCRATCH/base.txt` | **0** |
| anchored panic gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV' $SCRATCH/base.txt` | **0** |
| unanchored `FAIL` | `/usr/bin/grep -c 'FAIL' $SCRATCH/base.txt` | **0** |

**`RUN=392` beside `RC=0` — the green is NOT vacuous**: 392 test/subtest entries
actually ran. The unanchored count is reported as a cross-check only; it happens
to agree here because the tree is fully green, and it is NOT a substitute for the
anchored matcher.

Per-package result lines — `/usr/bin/grep -E '^(ok|FAIL|\?)\s' $SCRATCH/base.txt`:

```
ok  	github.com/pgdad/envoy-go/cmd/envoy-go	11.989s
ok  	github.com/pgdad/envoy-go/internal/admin	1.471s
ok  	github.com/pgdad/envoy-go/internal/boot	0.259s
ok  	github.com/pgdad/envoy-go/internal/listener	3.263s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
ok  	github.com/pgdad/envoy-go/validate	0.266s
```

**All seven packages report `ok`. ZERO tests failed at the un-fixed tip**, so
there are no `--- FAIL` lines to quote.

### The un-fixed roster — parked for Task 10

Sorted, deduped, top-level test names (subtest suffixes after the first `/`
stripped):

```sh
/usr/bin/grep -E '^\s*=== RUN' $SCRATCH/base.txt \
  | sed -E 's/^\s*=== RUN\s+//' | cut -d/ -f1 | sort -u \
  > $SCRATCH/roster-unfixed-task1.txt
```

| What | Value |
| --- | --- |
| Roster path | `/tmp/claude-1000/-home-esa-git-envoy-go/9e45b2a6-8d74-4fb9-9a8e-9023366d0d96/scratchpad/t1/roster-unfixed-task1.txt` |
| Duplicate copy | `…/scratchpad/t1/roster-unfixed-task1.bak.txt` |
| Raw `-v` log | `…/scratchpad/t1/base.txt` |
| Roster size | **309** distinct top-level test names (`wc -l`) |

309 top-level names against 392 `=== RUN` lines — the difference is subtests, and
the two figures are NOT interchangeable. Task 10 must diff the ROSTER, not the
`=== RUN` count: a renamed or deleted arm can leave the count unchanged.

### Step 3 — the anchored panic gate PROVEN LIVE

A gate that reads 0 has not been shown to work. Three readings follow, each with
the command that produced it. ⚠️ A fail-fast control measures ONE site per run,
so exactly one site was probed, in a run of its own.

Probe site, relocated by LITERAL (not by a remembered line number):

`/usr/bin/grep -nF -- 'func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {' internal/listener/manager.go`

```
1322:func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
```

#### Reading 1 — BEFORE (no probe in tree)

`go test -count=1 -v -run TestUnifiedDispatchPlaintextChainSelectByDestPort ./internal/listener/ > $SCRATCH/probe-before.txt 2>&1`

- RC = **0**
- `=== RUN` count = **1** (the selector matched — it did not print `[no tests to run]`)
- `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV' $SCRATCH/probe-before.txt` = **0**

```
=== RUN   TestUnifiedDispatchPlaintextChainSelectByDestPort
--- PASS: TestUnifiedDispatchPlaintextChainSelectByDestPort (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.006s
```

#### Reading 2 — DURING (bare `panic` as the first statement of `serveConnection`)

Mutation: `panic("p98 gate probe")` inserted as the FIRST statement of
`serveConnection`, i.e. as the new line 1323, and **nothing else** changed —
`git -C /home/esa/git/wt-phase-98-impl status --porcelain` read exactly:

```
 M internal/listener/manager.go
```

`go test -count=1 -v -run TestUnifiedDispatchPlaintextChainSelectByDestPort ./internal/listener/ > $SCRATCH/probe.txt 2>&1`

- RC = **1**
- `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV' $SCRATCH/probe.txt` = **1** — **>= 1 as required, the gate FIRES**
- anchored FAIL count = **2**
- the matched line, with its line number — `/usr/bin/grep -nE '^panic:|DATA RACE|SIGSEGV' $SCRATCH/probe.txt`:

```
2:panic: p98 gate probe
```

The stack trace NAMES `serveConnection` at the inserted line, proving the probe
site was actually reached and that the gate is reading THIS mutation:

```
=== RUN   TestUnifiedDispatchPlaintextChainSelectByDestPort
panic: p98 gate probe

goroutine 18 [running]:
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).serveConnection(...)
	/home/esa/git/wt-phase-98-impl/internal/listener/manager.go:1323
created by github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).acceptLoop in goroutine 10
	/home/esa/git/wt-phase-98-impl/internal/listener/manager.go:1290 +0x15f
FAIL	github.com/pgdad/envoy-go/internal/listener	0.007s
FAIL
```

⚠️ **There is no `--- FAIL:` line for the test itself.** The panic fires on a
BACKGROUND goroutine created by `acceptLoop`, and there is no `recover()` in
non-test `internal/listener`, so it aborts the whole test binary before the test
can be marked failed. The two anchored FAIL hits are the package-level
`FAIL\tgithub.com/…` line and the bare trailing `FAIL`. **A test-result matcher
alone would have MISSED this**; the anchored panic gate is what reads it.

#### Reading 3 — AFTER (probe reverted)

`git -C /home/esa/git/wt-phase-98-impl checkout -- internal/listener/manager.go`

`go test -count=1 -v -run TestUnifiedDispatchPlaintextChainSelectByDestPort ./internal/listener/ > $SCRATCH/probe-after.txt 2>&1`

- RC = **0**
- `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV' $SCRATCH/probe-after.txt` = **0**

```
=== RUN   TestUnifiedDispatchPlaintextChainSelectByDestPort
--- PASS: TestUnifiedDispatchPlaintextChainSelectByDestPort (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.006s
```

Revert verified two ways:

| Check | Command | Output |
| --- | --- | --- |
| Whole tree | `git -C /home/esa/git/wt-phase-98-impl status --porcelain` | *(empty)* |
| The probed path | `git -C … status --porcelain -- internal/listener/manager.go` | *(empty)* |
| Diff | `git -C … diff --numstat` | *(empty)* |
| File digest | `sha256sum …/internal/listener/manager.go` | `a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f` |

**Gate summary: 0 (before) → 1 (during) → 0 (after).** The gate is LIVE, and it
returns to 0 — it is neither self-clearing nor stuck.

### Step 4 — the known `TwoListenerCutover` flake: it did NOT fire

`TestEnvoyGoBinary_TwoListenerCutover` (`cmd/envoy-go`) has fired twice in one
session at the phase-98 SPEC with `bind 127.0.0.1:<port>: address already in use`
on a port inside the ephemeral range `32768-60999`.

| Check | Command | Value |
| --- | --- | --- |
| The test's result | `/usr/bin/grep -nE 'TwoListenerCutover' $SCRATCH/base.txt` | `--- PASS: TestEnvoyGoBinary_TwoListenerCutover (1.54s)` |
| Bind collisions anywhere in the run | `/usr/bin/grep -c 'address already in use' $SCRATCH/base.txt` | **0** |

**It did not fire in this run, so there is no port to record.** ⚠️ **This green
clears NOTHING.** The hazard is a host-port bind race inside the ephemeral range;
a passing run is not evidence it is fixed, and if it fires at a later task it is a
**known non-regression for this row** — it fires at the un-fixed tip too and must
not be chased.

### What contradicted the brief

- **Nothing was refuted.** The brief's `~392` estimate for `=== RUN` was exact
  (**392**), the literal for `serveConnection` was at **`:1322`** exactly as
  stated, and the selector, the probe test and the revert all behaved as written.
- One thing worth stating explicitly rather than as a refutation: the selector
  resolves to **seven** packages, not the five its five path arguments suggest at
  a glance. Any per-package accounting downstream must use seven.

---

## Task 2 — the §5.1 re-pointed parse test, RED at the un-fixed tip

**Commit scope:** `internal/listener/manager_test.go` (the only `.go` file
touched) + this `PROGRESS.md`. `git -C /home/esa/git/wt-phase-98-impl status
--porcelain` before staging read exactly:

```
 M internal/listener/manager_test.go
```

### 🔴 The PLAN is REFUTED: `runtimeFor` DOES NOT EXIST

`PLAN.md` §6 Task 2 Step 2's code block instructs:

```go
rt := mgr.runtimeFor("l_tp") // resolve by the package's own accessor; see manager_test.go:1236
```

There is no such accessor anywhere in the repository.

| Check | Command | Output |
| --- | --- | --- |
| The claimed accessor | `git grep -n 'runtimeFor' -- '*.go'` | *(no hits)*, **rc=1** |
| Discriminating positive control on the SAME files | `git grep -c 'runtimes' -- internal/listener/manager.go internal/listener/manager_test.go` | `internal/listener/manager.go:11`, `internal/listener/manager_test.go:55`, **rc=0** |

The control fires on the same two paths the null result covers, so the `rc=1`
is a real absence, not a blind matcher.

The field is a plain slice — `internal/listener/manager.go:234`:

```go
	runtimes  []*listenerRuntime
```

and the PLAN's own cited precedent, `selectByServerNameFromMgr`
(`manager_test.go:1229-1247`), reaches it by **direct field access**, not an
accessor:

```go
	rt := mgr.runtimes[0]
```

⚠️ **The line cite `:1236` is itself CORRECT** — it points at
`spec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)`,
the caller shape §5.1 quotes. Only the method name was invented.

**Used instead:** `rt := mgr.runtimes[0]`, guarded by an explicit
`if rt.name != "l_tp" { t.Fatalf(...) }` so the arm cannot silently drift onto a
different runtime if the §5.1 shape ever grows a listener. The field holding the
name really is `name` — `manager.go:148-149`:

```go
type listenerRuntime struct {
	name    string
```

`NewManager`'s signature matches the PLAN verbatim — `manager.go:262`:

```go
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, registry *stats.Registry, httpRegistry *filter_http.HTTPRegistry) (*Manager, error)
```

### Step 1 — the deleted block, boundaries VERIFIED before deleting

Located by literal anchor, never by line number:

```sh
/usr/bin/grep -nF -- 'func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {' internal/listener/manager_test.go
```

```
1691:func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {
```

The splice re-derived all three boundaries from literals (doc-comment first
line, the `func` line, and the first column-0 `}` at or after it) and asserted
the doc comment sits exactly 3 lines above the `func`, printing:

```
VERIFIED boundaries: start line 1688 func line 1691 end line 1715 total 28
FIRST: '// TestParseChainSpecRejectsUnknownTransportProtocol verifies the'
LAST : '}'
```

**28 lines, `:1688-1715`** — matching anchor **A9** (`PLAN.md:661`) exactly.
Replacement block: **73 lines**.

No `.go` reference to the old test name survives —
`git grep -n 'TestParseChainSpecRejectsUnknownTransportProtocol' -- '*.go'` has
**no hits**; the remaining 18 hits are all under `docs/` (ROADMAP row 98, the
07.2 and 61.1 phase docs, and this phase's own BRAINSTORM/PLAN/SPEC), which
Task 2 does not own.

### Steps 2-4 — the replacement

`TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue`, built to
the §5.1 shape: listener `l_tp`, plaintext, port 0; `filter_chains[0]` =
`FilterChainMatch{TransportProtocol: "sctp"}`; `filter_chains[1]` =
`FilterChainMatch{SourceType: listenerv3.FilterChainMatch_SAME_IP_OR_LOOPBACK}`
and nothing else; **no** `default_filter_chain`. Four properties, **one
`t.Errorf` each, each message naming its own property**, (a) a `t.Fatalf`.

Both §5.1 hazards were re-verified against source before the test was written,
not taken on the PLAN's word:

| Hazard | Verified at | Reads |
| --- | --- | --- |
| nil `SourceIP` is NOT loopback | `internal/listener/listenerfilter/types.go:56-58` | `return c.SourceIP != nil && c.SourceIP.IsLoopback()` |
| the index-derived chain name | `PLAN.md` §0.14 | assertions use `"l_tp/filter_chains[0]"` / `"l_tp/filter_chains[1]"`, never a proto `Name` |

Every probe input carries `SourceIP: net.ParseIP("127.0.0.1")` — four inputs,
three in the (c) loop and one in (d).

### Step 5 — RED, at (a), for the RIGHT reason

```sh
go test -count=1 -v -run TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue ./internal/listener/
```

RC = **1**. Verbatim:

```
=== RUN   TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
    manager_test.go:1727: (a) NewManager must ACCEPT transport_protocol "sctp", got error: listener: "l_tp": filter_chains[0]: transport_protocol "sctp" must be "tls", "raw_buffer", "quic", or empty
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.005s
FAIL
```

- **The test actually RAN** — the `=== RUN` line is present, so this is not the
  `[no tests to run]` / exit-0 selector footgun.
- **The site was reached, proven by the MESSAGE, not the exit code.** The
  failure names the un-lifted enum gate verbatim —
  `transport_protocol "sctp" must be "tls", "raw_buffer", "quic", or empty` —
  the exact reject Task 11 lifts. An unrelated boot reject (e.g. the catch-all
  `at most one filter_chain may omit filter_chain_match.server_names` that the
  `raw_buffer` sibling shape would have produced) would have masked it; it did
  not appear.

**Property reachability — scored per property, not per run:**

| prop | reachable at this tip? | why |
| --- | --- | --- |
| (a) | **REACHED, RED** | the reject fires at `NewManager` |
| (b) | **NOT reachable** | (a) is a `t.Fatalf`; the test binary stops there |
| (c) | **NOT reachable** | same |
| (d) | **NOT reachable** | same |

This is by design: without a manager there is no `rt.chainSpecs` for (b)/(c)/(d)
to read. (b), (c) and (d) first become reachable at Task 11, and the NC roster
must therefore score rows 2 and 3 **after** the lift, never against this run.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Format | `gofmt -l internal/listener/manager_test.go` | *(empty output — gated on OUTPUT, not rc)* |
| Vet | `go vet ./internal/listener/` | rc=**0**, no output |
| Build | `go build ./...` | rc=**0**, no output |

⚠️ **`golangci-lint` is BROKEN in this environment and it is PRE-EXISTING.**
`golangci-lint run ./internal/listener/...` exits **1** on a file Task 2 never
touched:

```
internal/listener/listenerfilter/registry.go:6:2: could not import sync/atomic (-: could not load export data: internal error in importing "sync/atomic" (cannot decode "sync/atomic", export data version 4 is greater than maximum supported version 2); please report an issue) (typecheck)
```

That is the linter's bundled type-checker failing to read the installed Go
stdlib's export data — a toolchain-version mismatch, not a finding against this
change. **Control, on a package Task 2 does not touch at all:**
`golangci-lint run ./internal/stats/...` also exits **1** with the identical
`could not import sync/atomic` typecheck error at `internal/stats/counter.go:5:2`
— so the failure is environmental and repo-wide, not scoped to the edit.
`go vet` and `go build` (which use the real toolchain) are both green.
**Downstream tasks must not read this rc=1 as a lint regression.**

---

## Task 3 — §5.2 arms (s1) and (s2), a MATCHED PAIR one string apart

Both arms land in `internal/listener/manager_test.go`, spliced immediately
after the Task 2 block (`}` at `:1760` before the splice). Post-splice:

| arm | test | line |
| --- | --- | --- |
| (s1) | `TestServeConnection_NoListenerFilter_RawBufferChainServes` | `manager_test.go:1785` |
| (s2) | `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` | `manager_test.go:1867` |

```sh
/usr/bin/grep -n 'func TestServeConnection_NoListenerFilter_' internal/listener/manager_test.go
1785:func TestServeConnection_NoListenerFilter_RawBufferChainServes(t *testing.T) {
1867:func TestServeConnection_NoListenerFilter_TLSChainDoesNotServe(t *testing.T) {
```

File grew `6654 -> 6822` lines (`wc -l`), **+168**.

### Step 1 — the template, RELOCATED, never computed

`PLAN.md` §5.2 cites the template at `manager_test.go:3459-3538`. That figure
was **correct at the PLAN commit and is stale now** — Task 2's edit was
`+58 / -13` (`git show --numstat a1f4cd20`), a net **+45** at `:1685`, which
lands the template at **`:3504`**, and `3504 - 45 = 3459` reconciles exactly.
Relocated by LITERAL, not arithmetic:

```sh
/usr/bin/grep -nF -- 'func TestUnifiedDispatchPlaintextChainSelectByDestPort(t *testing.T) {' internal/listener/manager_test.go
3504:func TestUnifiedDispatchPlaintextChainSelectByDestPort(t *testing.T) {
```

After this task's own +168 splice it sits at **`:3672`**. Any later task must
re-locate it again by literal.

### What was cloned, and the ONE deliberate deviation

Cloned from the template: `startTaggedBackend` as the per-chain discriminator,
`twoClusterMgr` / `mkTcpProxyFilter` / `mkBoot` / `NewManager(boot, cm,
stats.NewRegistry(), testHTTPRegistry())`, `mgr.Start(ctx)`, a **real loopback
TCP** `net.DialTimeout`, and `readByteWithTimeout`. The `default_filter_chain`
slot shape is cloned from `TestUnifiedDispatchDefaultFilterChainFallback`
(`:3657` pre-splice).

**Deviation — the probe listener is DROPPED.** The template binds a probe
listener at port 0 first only because its match dimension *is*
`destination_port`, so it must know the resolved port before building the
spec. These arms match on `transport_protocol`, which is port-independent, so
the listener is built with `PortValue: 0` and the address read back from
`mgr.Listeners()[0].Addr` — the same shape `TestUnifiedDispatchTLSWithSNI`
uses. This removes a port-reservation step; it changes nothing about the
discriminator.

### Two boot traps that did NOT fire, and why

| trap | site | why it cannot fire here |
| --- | --- | --- |
| the enum reject Task 11 lifts | `manager.go:993` `case "", "tls", "raw_buffer", "quic":` | both arms use values **inside** the current domain, so neither boot-rejects; (s1)'s RED is a **runtime** verdict, not a parse one |
| the catch-all guard that masked Task 2's first shape | `manager.go:722-725` | each arm has exactly **one** `filter_chains[]` entry, so `catchAllCount == 1`; `default_filter_chain` is a separate slot per ADR-0080, as that comment states |

A chain carrying only `transport_protocol` is also **not** `Empty`:
`isAllZeroChainSpec` (`manager.go:1057-1067`) includes
`spec.TransportProtocol == ""` in its conjunction, so the chain is not promoted
to universally-eligible. Had it been, (s1) would have been green at the
un-fixed tip for the wrong reason.

### Step 5 — the run, verdict recorded PER ARM

```sh
go test -count=1 -v -run 'TestServeConnection_NoListenerFilter_' ./internal/listener/
```

RC = **1**. Verbatim:

```
=== RUN   TestServeConnection_NoListenerFilter_RawBufferChainServes
    manager_test.go:1841: tag byte = 'B', want 'A': a TCP connection no listener filter classified must be stamped transport_protocol=raw_buffer, which makes filter_chains[0] eligible. 'B' is default_filter_chain, meaning the detected transport protocol stayed empty
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
=== RUN   TestServeConnection_NoListenerFilter_TLSChainDoesNotServe
--- PASS: TestServeConnection_NoListenerFilter_TLSChainDoesNotServe (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.006s
FAIL
```

**Both arms RAN** — two `=== RUN` lines, so this is not the `[no tests to run]`
/ exit-0 selector footgun.

| arm | verdict at the un-fixed tip | what it measures |
| --- | --- | --- |
| **(s1)** | 🔴 **RED** — served `'B'` | **This is the divergence.** The tag byte is `'B'`, the `default_filter_chain` backend, proving the detected transport protocol reached `SelectChain` as `""`, the `c.TransportProtocol != inputs.TransportProtocol` branch (`listenerfilter/chainmatch.go:128`) rejected the `raw_buffer` chain, and the connection fell through. The reference would have served `'A'`. This RED is NC roster row 4's evidence and is **unrecoverable after Task 12**. |
| **(s2)** | ⚪ **structurally GREEN — NOT a pass** | The subject answers "default chain" here for **exactly the reason it answers it in (s1)**: the detected value is `""`, so no indexed chain is eligible. *default* merely happens to be the right answer for (s2). **The subject could not have answered anything else**, so this green measures nothing about the transport-protocol dimension until (s1) goes green beside it at Task 12. The same reasoning is written into (s2)'s doc comment so a later reader cannot mistake it for evidence. |

### Step 7 — the disarming hazard, written into (s1)'s doc comment

`tls_inspector` stamps `raw_buffer` on a plaintext connection **by itself**.
So adding ANY listener filter to (s1)'s listener makes it read `'A'` and
**pass at the un-fixed tip** — a green arm sitting on top of a live divergence
in the exact dimension it claims to cover, with nothing reddening to announce
it. The absence of `listener_filters` is therefore load-bearing, and (s1)'s
doc comment says so in those terms, as does (s2)'s by reference.

### Collateral check — the pair broke nothing

```sh
go test -count=1 ./internal/listener/...
```

RC = **1**. Anchored FAIL matcher `^(FAIL|--- FAIL)|^ *--- FAIL`:

```
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.266s
FAIL
```

```
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
```

**Exactly TWO** `--- FAIL` rows, and both are this phase's own planted REDs:
Task 2's re-pointed parse test and this task's (s1). No collateral.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Format | `gofmt -l internal/listener/manager_test.go` | *(empty output — gated on OUTPUT, not rc)* |
| Vet | `go vet ./internal/listener/` | rc=**0**, no output |
| Build | `go build ./...` | rc=**0**, no output |

`golangci-lint` was **not run**: it is broken repo-wide in this environment
(`could not import sync/atomic … export data version 4`), already measured and
controlled under Task 2's Gates section. It is not a gate for this task.

### What contradicted the brief and the PLAN

1. **`PLAN.md` §5.2 / Task 3 Step 2 says "Dial, write a byte, read the tag".
   The write is WRONG — or at best inert.** `startTaggedBackend`
   (`manager_test.go:3398-3423`) is **server-speaks-first**: it does
   `conn.Write([]byte{tag})` immediately on accept, *before* reading anything.
   The client must dial and read; a client-side write is unnecessary. Worse, a
   write-first client on a `tcp_proxy` chain would force an upstream dial
   before the tag arrives and could reorder the failure mode. The arms read
   without writing, and the controller's brief already carried this correction.
2. **`PLAN.md` §5.2's template line range `3459-3538` is stale at this tip** —
   it is `:3504` pre-splice and `:3672` post-splice. The figure was accurate at
   the PLAN commit; Task 2's `+45` moved it. Re-located by literal, reconciled
   arithmetically only as a *check*, never as the method.
3. **`default_filter_chain` behaves exactly as the PLAN assumes** when no
   indexed chain is eligible — measured, not inferred: (s1) served `'B'`, the
   default slot's backend. The PLAN's §5.2 expectation row for (s2) is
   therefore sound.
4. **The two arms are not literally "one string apart".** They differ in
   **three** places: the `transport_protocol` value, the expected tag byte, and
   the failure message. Listener name (`l_tp_nolf`), cluster names (`c_tp` /
   `c_default`) and backend tags (`'A'` / `'B'`) are held **identical** across
   the pair — they are separate `Manager` instances with separate
   `stats.NewRegistry()`, so the shared names collide with nothing. Three
   differences is the floor for a matched negative that asserts a different
   outcome; the PLAN's "byte-identical except ONE string" is an ideal the shape
   cannot literally reach.

---

## Task 4 — §5.2 arm (s3), GREEN at the un-fixed tip and PROVEN FALSIFIABLE

The arm lands in `internal/listener/manager_test.go`, spliced immediately after
Task 3's (s2) block and before `TestParseChainSpec_QUICTransportProtocolAccepted`
(relocated by literal, never by line number):

```sh
/usr/bin/grep -nF -- 'func TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten(t *testing.T) {' internal/listener/manager_test.go
1962:func TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten(t *testing.T) {
```

Doc comment opens at `:1930`. File grew `6822 -> 6938` lines (`wc -l`),
`git diff --numstat` = **`116  0`**. `PLAN.md` §1.3's per-file estimate for
this arm was **+110**; actual **+116**.

### Step 1 — the wiring, and where each piece came from

Taken from **`TestUnifiedDispatchTLSWithSNI`** (`manager_test.go:3868`
post-splice; located by literal, the PLAN quotes no line for it), which is the
only committed test that drives a real TLS client through a listener carrying
`tls_inspector`:

| piece | source |
| --- | --- |
| `l.ListenerFilters = []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)}` | `TestUnifiedDispatchTLSWithSNI`; `mkTLSInspectorFilter` at `:3228` builds an `anypb.Any` with `TypeUrl: tls_inspector.TypeURL` and a **nil** `Value` (the parser tolerates nil ⇒ default config) |
| `testLFRegistry()` (`:74`), passed as the listener-filter registry | same test — without it `NewManager` rejects a non-empty `listener_filters[]` |
| `NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)` | copied verbatim from the same test |
| `stdtls.DialWithDialer` + `testCAPool(t)` (`:789`) + `ServerName: "alpha.envoy-go.test"` | same test's `dialAndCheckTag`, **preferred over the brief's suggested `InsecureSkipVerify`** because the package already has the CA pool helper |
| `mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)` (`:629`) | same test; **inline** PEM, so no PKI is shipped |
| `startTaggedBackend` / `twoClusterMgr` / `mkTcpProxyFilter` / `mkBoot` / `readByteWithTimeout` | Task 3's arms, unchanged |

Deviation from `TestUnifiedDispatchTLSWithSNI`: it discriminates on
`server_names` via `mkTLSChain`, which sets **only** `ServerNames` on the
`FilterChainMatch`. This arm needs `TransportProtocol: "tls"`, so the
`FilterChain` literal is written out and handed to `mkTLSListener`, and the
`default_filter_chain` slot is attached afterwards.

**No PKI was created.** `testAlphaCertPEM` (`:587`) and `testAlphaKeyPEM`
(`:600`) already existed at exactly the lines `PLAN.md` Task 4 cites — the only
figure in the brief that had **not** drifted, because both consts sit *above*
Task 2's `:1685` and Task 3's `:1760` splice points.

### Step 3 — the mechanism, verified in the source and not assumed

`tls_inspector` really does write `"tls"` on this path:

```sh
/usr/bin/grep -rn 'inputs.TransportProtocol = "tls"' internal/listener/listenerfilter/tls_inspector/
internal/listener/listenerfilter/tls_inspector/tls_inspector.go:86:	inputs.TransportProtocol = "tls"
```

`:86` is reached only after `parseClientHello(buf)` returns `ok`; the three
early returns above it (`:78`, `:83`, and the header checks before them) write
`"raw_buffer"`. So the classified value on this arm's path is `"tls"`, and the
stamp Task 12 adds at `manager.go:1367` (`// (5) Run chain-match algorithm.`)
must leave it alone.

### Step 5 — the run at the un-fixed tip

```sh
go test -count=1 -v -run TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten ./internal/listener/
```

RC = **0**. Verbatim:

```
=== RUN   TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
--- PASS: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.006s
```

The `=== RUN` line is present, so this is **not** the `-run`-matches-nothing /
`[no tests to run]` / exit-0 footgun.

⚠️ **THIS GREEN IS NOT EVIDENCE.** `tls_inspector` already writes `"tls"` here
and nothing on this path reads the stamp, so the arm is green **before and
after** the production edits. Its falsifiability lives entirely in NC roster
rows 5 and 5b. It is recorded as **GREEN, independent of the stamp** — never as
a pass for the stamp.

Proof the TLS handshake actually happened rather than a byte merely arriving:
the arm asserts `ConnectionState().HandshakeComplete`, a non-empty
`PeerCertificates`, and `PeerCertificates[0].Subject.CommonName ==
"alpha.envoy-go.test"` — i.e. the chain's own inline alpha leaf was presented —
**and only then** reads the tag byte. The tag assertion is the verdict; the
handshake assertions are the precondition.

### Step 5, PRE-BELIEVING FALSIFIABILITY CONTROL — the NC row 5b mutation, run NOW

Before accepting the green, both NC roster rows that reference this arm were
run as **throwaway** local mutations at the `// (5) Run chain-match algorithm.`
anchor in `internal/listener/manager.go` (`:1367`), one line inserted
immediately before `listenerfilter.SelectChain`.

Baseline hash first:

```sh
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go
```

**Row 5b** — drop the `== ""` guard, stamp `"raw_buffer"` unconditionally:

```go
	inputs.TransportProtocol = "raw_buffer" // NC-ROW-5B THROWAWAY MUTATION
```

```
=== RUN   TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
    manager_test.go:2017: TLS dial: context deadline exceeded — the tls chain did not terminate TLS. If the transport-protocol stamp OVERWROTE the input tls_inspector classified as "tls", filter_chains[0] became ineligible and the plaintext default_filter_chain served, which cannot complete a handshake
--- FAIL: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (5.01s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	5.014s
FAIL
```

RC = **1**. 🔴 **The arm REDDENS under the exact mutation it exists to
detect.** The observable is the dial failing, not a `'B'` tag: the default
chain is plaintext, so the client's `ClientHello` is proxied to backend B,
which (`startTaggedBackend`) writes **one** byte and then only reads — one byte
cannot fill a 5-byte TLS record header, so the client blocks until the 5 s
dialer timeout. The failure message names the mechanism either way.

**Row 5** — stamp unconditionally with `"tls"`:

```go
	inputs.TransportProtocol = "tls" // NC-ROW-5 THROWAWAY MUTATION
```

```
=== RUN   TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
--- PASS: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.006s
```

RC = **0**. ⚪ **Stays GREEN**, exactly as `PLAN.md` §8's roster row 5 requires
((s3) is listed under "must NOT redden"). Row 5 was not required by the brief;
running it costs one line and de-risks Task 16 on **both** of its rows.

**Revert proof — `manager.go` is byte-untouched:**

```sh
git checkout -- internal/listener/manager.go
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go

git diff --numstat -- internal/listener/manager.go
(empty)

/usr/bin/grep -c "NC-ROW-5" internal/listener/manager.go
0

git status --porcelain
 M internal/listener/manager_test.go
```

Identical sha256 to the pre-mutation baseline, empty `--numstat`, zero residual
markers, and the only modified path is this task's own test file. Tasks 11/12
own `manager.go` and inherit it untouched.

That digest is also **the same one Task 1 recorded** for the un-fixed baseline
(this file, "File digest" row) — so the revert restored not merely "no diff
against HEAD" but the exact byte image the phase started from.

Green confirmed to return after the revert:

```
=== RUN   TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
--- PASS: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.007s
```

### Collateral check — the arm broke nothing

```sh
go test -count=1 ./internal/listener/...
```

RC = **1**. All `--- FAIL` rows:

```
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.259s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
FAIL
```

**Exactly TWO** `--- FAIL` rows, unchanged from Task 3: Task 2's re-pointed
parse test and Task 3's (s1). No third row appeared. No collateral.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Format | `gofmt -l internal/listener/manager_test.go` | *(empty output — gated on OUTPUT, not rc)* |
| Vet | `go vet ./internal/listener/` | rc=**0**, no output |
| Build | `go build ./...` | rc=**0**, no output |

`golangci-lint` was **not run** — broken repo-wide in this environment, already
measured and controlled under Task 2's Gates section.

### What contradicted the brief and the PLAN

1. **`PLAN.md` Task 4's cited lines `:587` / `:600` for `testAlphaCertPEM` /
   `testAlphaKeyPEM` are STILL CORRECT** — the one figure in this phase's docs
   that has not drifted, because both consts precede every splice point so far.
   Verified by literal (`const testAlphaCertPEM = `) anyway; the agreement was
   checked, not assumed.
2. **The brief's suggested `InsecureSkipVerify` was NOT used.** The package
   already ships `testCAPool(t)` (`:789`) over `testCAPEM`, and
   `TestUnifiedDispatchTLSWithSNI` verifies the served leaf's CN. Real
   verification is strictly stronger here and costs nothing: it is what lets
   the arm assert *which* certificate the selected chain presented, turning
   "a handshake completed" into "the **tls** chain's own leaf was served".
3. **There is no pre-existing reusable TLS-client helper** — `dialAndCheckTag`
   is a closure **inside** `TestUnifiedDispatchTLSWithSNI`, not a package-level
   function, so its shape was re-implemented rather than called. Nothing was
   extracted or refactored: Task 4's file scope is additive only.
4. **`tls_inspector` writing `"tls"` was verified in its source, not assumed** —
   `tls_inspector.go:86`, reachable only past a successful `parseClientHello`.
5. **NC row 5b's observable is a DIAL FAILURE, not a `'B'` tag byte.** The
   PLAN's §8 roster says only "(s3) reddens"; it does. But Task 16 must not
   expect a tag-byte mismatch message — the arm fails at `t.Fatalf` on the TLS
   dial, ~5 s in, because the plaintext default chain cannot produce a
   ServerHello. Recorded here so Task 16 does not read the differing message
   shape as the wrong mutation having fired.

---

## Task 5 — §5.3's QUIC bogus-value arm, RED at the BOOT step

The arm lands in `internal/listener/quic_test.go` as a **third sibling**,
spliced immediately after `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch`'s
closing brace and before the `quicChainSpecByName` helper. Every anchor was
relocated by **literal text**, never by the PLAN's line numbers:

```sh
/usr/bin/grep -n "TestQUICChainSelection_TransportProtocol" internal/listener/quic_test.go   # BEFORE the splice
695:func TestQUICChainSelection_TransportProtocolQUICMatches(t *testing.T)
772:func TestQUICChainSelection_TransportProtocolTLSDoesNotMatch(t *testing.T)
```

🟢 **Both sibling anchors were EXACTLY where `PLAN.md` §5.3 and §6 Task 5 say**
(`:695` and `:772`) — unlike `manager_test.go`, whose numbers have moved three
times. They are above every Task 2/3/4 splice point, all of which landed in
`manager_test.go`, so nothing in this file had drifted.

After the splice:

```sh
/usr/bin/grep -n "TestQUICChainSelection_TransportProtocolBogusDoesNotMatch" internal/listener/quic_test.go
820:// TestQUICChainSelection_TransportProtocolBogusDoesNotMatch is phase-98's
859:func TestQUICChainSelection_TransportProtocolBogusDoesNotMatch(t *testing.T)
```

File grew `1652 -> 1741` lines (`wc -l`); `git diff --numstat` = **`89  0`**.

### Step 1 — the wiring, verified against the REAL helpers and not assumed

Every helper the brief named was checked to exist, with its real signature,
before a line was written:

```sh
/usr/bin/grep -n "func mkQUICListenerChains\|func quicChainKeys\|func mkClusterMgr\|func mkBoot\|func testHTTPRegistry" internal/listener/*.go
internal/listener/manager_test.go:61:  func testHTTPRegistry() *filter_http.HTTPRegistry
internal/listener/manager_test.go:85:  func mkBoot(adminPort uint32, listeners []*listenerv3.Listener, clusters []*clusterv3.Cluster) *bootstrapv3.Bootstrap
internal/listener/manager_test.go:116: func mkClusterMgr(t testing.TB, name, host string, port uint32) *cluster.Manager
internal/listener/manager_test.go:1049:func mkQUICListenerChains(t *testing.T, fcm *listenerv3.FilterChainMatch, body string, withDefault bool, defaultTLS bool, defaultBody string) *listenerv3.Listener
internal/listener/quic_test.go:407:    func quicChainKeys(rt *listenerRuntime) []string
```

`quicChainSpecByName(t *testing.T, rt *listenerRuntime, name string) *listenerfilter.ChainSpec`
exists too, immediately below the sibling pair.

🟢 `mkQUICListenerChains`'s signature **is** `(t, fcm, body, withDefault,
defaultTLS, defaultBody)` exactly as the brief states, and it sits at exactly
`manager_test.go:1049` as `PLAN.md` §5.3 cites — a second figure in this phase's
docs that has **not** drifted. Called `mkQUICListenerChains(t, fcm, "FC0\n",
true, true, "DFC\n")`, i.e. **QUIC-TLS default slot present**, byte-identical to
the `tls` sibling's call.

**No table was introduced.** Both QUIC test files remain table-free under every
matcher form; the arm is a straight-line function like its two siblings.

### Step 2 — the justification, and the weakness, both in the doc comment

The one-sentence justification (`PLAN.md` §4(c)) is recorded verbatim in the
comment: *it is the only committed arm that reddens under NC roster row 1 via
the QUIC listener-construction path, and without it the four `quic_test.go`
narration edits of Task 14 are an unpinned prose change.*

⚠️ The **weakness is recorded beside it, not smoothed**: under NC row 1 the
§5.1 parse arm **also** reddens, so this arm's *marginal* power is limited to a
kind-scoped restoration of the gate. At the runtime layer it adds nothing
(`"tls" != "quic"` and `"totally_bogus_value" != "quic"` traverse the identical
`chainmatch.go:128` branch); at the parse layer it adds nothing either
(`parseChainSpec` takes no listener-kind argument). What it uniquely traverses
is the **QUIC-kind `buildListenerRuntime` boot path carrying a bogus value**.

### Step 3 — the run at the un-fixed tip

```sh
go test -count=1 -v -run TestQUICChainSelection_TransportProtocolBogusDoesNotMatch ./internal/listener/
```

RC = **1**. Verbatim:

```
=== RUN   TestQUICChainSelection_TransportProtocolBogusDoesNotMatch
    quic_test.go:866: NewManager(quic, transport_protocol="totally_bogus_value" filter_chains[0] + QUIC-TLS default slot) BOOT-REJECTED: listener: "quic_listener_chains": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty — an unknown transport_protocol is a NON-MATCHING VALUE, not a config error, so parseChainSpec must accept it and let chain selection decide
--- FAIL: TestQUICChainSelection_TransportProtocolBogusDoesNotMatch (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.006s
FAIL
```

⚠️ **THE SITE IS PROVEN BY THE MESSAGE, NOT BY RC.** An unrelated boot reject
would read rc=1 identically. The failure text carries
`transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic",
or empty` — the **enum gate's own `default:` arm**, whose source reads:

```sh
sed -n '994,999p' internal/listener/manager.go
	switch tp := fm.GetTransportProtocol(); tp {
	case "", "tls", "raw_buffer", "quic":
		spec.TransportProtocol = tp
	default:
		return nil, fmt.Errorf("transport_protocol %q must be \"tls\", \"raw_buffer\", \"quic\", or empty", tp)
	}
```

It is **not** a `server_names` / catch-all reject, **not** a TLS-context error,
and **not** a QUIC-transport-socket error: those produce different strings and
none names `transport_protocol`. The reproduction reached the intended site.

It fails at `quic_test.go:866`, the **`NewManager` `t.Fatalf`** — the BOOT step —
so the arm never reaches `quicChain()` and neither PROPERTY line executed. That
is precisely `PLAN.md` §8's prediction (`| §5.3 QUIC | **RED at boot** | the
enum switch rejects "totally_bogus_value" |`). Task 11 lifts the gate; after it
the arm must fail — if at all — on a PROPERTY line instead, and `PLAN.md`
§8's post-fix table says it **turns GREEN**.

### Collateral check — the arm broke nothing

```sh
go test -count=1 ./internal/listener/...
```

RC = **1**. All `--- FAIL` rows:

```
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
--- FAIL: TestQUICChainSelection_TransportProtocolBogusDoesNotMatch (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	3.264s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
FAIL
```

**Exactly THREE** `--- FAIL` rows (counted: `echo "$out" | /usr/bin/grep -c --
"--- FAIL"` → **3**), and they are exactly the three predicted: Task 2's
re-pointed parse test, Task 3's (s1), and this task's QUIC arm. **No fourth row
appeared.** No collateral.

### Per-arm roster at the un-fixed tip, after Task 5

| arm | task | at the un-fixed tip | fails where |
| --- | --- | --- | --- |
| `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` | 2 | 🔴 **RED** | property (a), `manager_test.go:1727` |
| `TestServeConnection_NoListenerFilter_RawBufferChainServes` (s1) | 3 | 🔴 **RED** | tag byte `'B'`, `manager_test.go:1841` |
| `TestServeConnection_TLSInspector_TLSChainServes` (s2) | 3 | 🟢 GREEN | — (matched negative) |
| `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` (s3) | 4 | 🟢 GREEN | — (falsifiability via NC row 5b) |
| `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | **5** | 🔴 **RED** | **BOOT**, `quic_test.go:866` |

### `manager.go` is byte-UNTOUCHED

```sh
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go

git -C /home/esa/git/wt-phase-98-impl diff master --numstat -- internal/listener/manager.go
(empty)

git -C /home/esa/git/wt-phase-98-impl status --porcelain
 M internal/listener/quic_test.go
```

Identical sha256 to the un-fixed baseline Task 1 recorded, empty `--numstat`
against `master`, and the **only** modified path is this task's own test file.
Tasks 11/12 inherit `manager.go` untouched.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Format | `gofmt -l internal/listener/quic_test.go` | *(empty output — gated on OUTPUT, not rc)* |
| Vet | `go vet ./internal/listener/` | rc=**0**, no output |
| Build | `go build ./...` | rc=**0**, no output |

`golangci-lint` was **not run** — broken repo-wide in this environment, already
measured and controlled under Task 2's Gates section.

### What contradicted the brief and the PLAN

1. 🔴 **`PLAN.md` §6 Task 5 Step 2 cites the WRONG TASK for the narration
   edits.** It reads "the four `quic_test.go` narration edits of **Task 12**";
   the four narration sites are owned by **Task 14** (`PLAN.md:1437`,
   "Modify: `internal/listener/quic_test.go` (four sites)"). Task 12 is a
   `manager.go` production edit. §4(c) itself names no task number, so only the
   Task 5 checklist carries the stale figure. **The brief is right and the PLAN
   step is wrong**; the doc comment records **Task 14**.
2. 🔴 **`PLAN.md` §1.3's LoC table has NO ROW FOR THIS ARM AT ALL.** Its only
   `quic_test.go` row is "the four narration sites — **+14 / −10**", which is
   Task 14's budget, not Task 5's. The sole estimate for this arm is §4(c)'s
   prose "**~60 lines of test**". Actual: **+89 / −0**, i.e. **+29 over** the
   prose figure and **entirely absent** from the table the split gate in §1.3
   is computed from. Recorded so Task 19's net-LoC reconciliation does not read
   the 89 lines as unbudgeted drift.
3. 🟢 **Both sibling anchors and `mkQUICListenerChains:1049` were CORRECT** —
   verified by literal anyway, not assumed. `quicChainKeys` (`quic_test.go:407`)
   and `quicChainSpecByName` both exist with the signatures the brief states.
4. ⚠️ **The un-fixed tip's reject is a `t.Fatalf`, so NEITHER property line of
   this arm has ever executed.** Its two `t.Errorf` properties are, at this tip,
   **unexercised code**. Task 13 is the first run that can observe them, and
   Task 16 must not read a green here as those properties having passed.
5. ⚠️ **A precondition this arm carries that its `tls` sibling cannot:** the
   `chainSpecs[...].TransportProtocol == "totally_bogus_value"` check pins that
   Task 11's lift **stores** the value rather than merely ceasing to reject it.
   A gate that stopped rejecting but dropped the string to `""` would make
   `chainmatch.go:128` skip the dimension, `filter_chains[0]` universally
   eligible, and PROPERTY 1 wrong for a right-looking reason. That precondition
   is the arm's one genuinely non-redundant assertion at the runtime layer.

---

## Task 6 — NC roster row 6's purity arm, GREEN and PROVEN ABLE TO FIRE

### Step 1 — DECLARED SCOPE WIDENING (not discovered later)

**`internal/listener/listenerfilter/chainmatch_test.go` is edited by this task and
is NOT in `SPEC.md` §6.3's edit set.** It is also **not** on the byte-untouched
roster, so the edit is legal — but `PLAN.md` §9.3 requires it be *declared*, and
this is the declaration. `PLAN.md` §9.3's widening table names exactly this file
against exactly this task.

**Reason (`PLAN.md` §0.13, re-stated, not re-derived):** NC roster row 6 — *"the
stamp moved into `SelectChain`"* — reddens **zero** pre-existing tests across all
seven reverse-dependency selectors. Identical greens under the mutation and the
un-mutation mean the suite is **BLIND** to it. Without an arm, row 6 is a negative
control that **cannot fire**, which this project treats as worse than deleting the
row. The only file in which such an arm is constructible is the unit test file for
`SelectChain` itself.

**Cost, measured:**

```sh
git diff master --numstat -- internal/listener/listenerfilter/chainmatch_test.go
34	0	internal/listener/listenerfilter/chainmatch_test.go
```

`212 -> 246` lines (`wc -l`). ⚠️ `PLAN.md` §1.3 budgets this file **zero**, because
it is outside the SPEC edit set the split gate is computed from. **+34 is
unbudgeted by construction, not drift** — recorded here so Task 19's net-LoC
reconciliation reads it as the declared widening rather than as leakage.

### Step 2 — the arm, and the two claims in the PLAN's draft that were CHECKED

Appended at end of file (no splice into existing bodies, so no anchor drift for any
other task). The `errors` import was already present (`errors`, `net`, `testing`);
**no import was added.**

Every symbol was verified in source before use rather than trusted from the PLAN:

| symbol | verified at | shape |
| --- | --- | --- |
| `SelectChain` | `chainmatch.go:80` | `func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)` — `inputs` **by value**, which is the whole mechanism of row 6 |
| `ChainSpec.Name`, `.TransportProtocol`, `.Empty` | `chainmatch.go:12-47` | plain fields; `Empty` is an **explicit field the caller sets**, never derived |
| `ErrNoChainMatched` | `chainmatch.go:52` | `errors.New("no filter_chain matches connection")` |
| `ChainMatchInputs` | `types.go:32-52` | `TransportProtocol string` at `:47` |

🟢 **The "green for the wrong reason" hazard was checked and does NOT apply.**
Task 3's agent found `isAllZeroChainSpec` (in `internal/listener/manager.go`) treats
`TransportProtocol == ""` as part of all-zero. There is **no analogous logic in
package `listenerfilter`**:

```sh
/usr/bin/grep -rn 'isAllZero\|func NewChainSpec\|Empty =' internal/listener/listenerfilter/*.go
(no matches)
```

`ChainSpec.Empty` is populated by `manager.go` at build time and is simply `false`
on a composite literal that sets only `Name` and `TransportProtocol`. So
`matches()` reaches its `c.TransportProtocol != "" && c.TransportProtocol !=
inputs.TransportProtocol` clause (`chainmatch.go:128`) and returns false — the arm
is `ErrNoChainMatched` **for the intended reason**, not because the chain was
silently classified as a catch-all.

⚠️ **The PLAN's doc-comment draft cites `quic.go:187`. The line anchor was NOT
copied.** `quic.go:187` *is* in fact the `listenerfilter.SelectChain` call today —
checked — but line anchors drift and symbol anchors do not, so the committed
comment cites **`listenerRuntime.selectQUICChain`** by name instead.

### Step 3 — the MATCHED NEGATIVE

`TestSelectChain_ClassifiedTransportProtocolStillMatches`: with
`ChainMatchInputs{TransportProtocol: "tls"}` and a single `tls` chain, `SelectChain`
must return that chain and a nil error. **This is what proves the purity arm is not
a blanket "SelectChain never matches transport_protocol" detector.** It is a sibling
function, not a sub-case, so a failure in one cannot mask the other.

### Step 4 — GREEN at this tip

```sh
go test -count=1 -v -run 'TestSelectChain_' ./internal/listener/listenerfilter/
```

RC = **0**. Verbatim:

```
=== RUN   TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain
--- PASS: TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain (0.00s)
=== RUN   TestSelectChain_ClassifiedTransportProtocolStillMatches
--- PASS: TestSelectChain_ClassifiedTransportProtocolStillMatches (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.002s
```

Both `=== RUN` lines are present, so this is **not** the `-run`-matches-nothing /
`[no tests to run]` / exit-0 footgun. (The pre-existing
`TestSelectChainTransportProtocol` has no underscore and is correctly **not**
selected by this pattern.)

⚠️⚠️ **THIS GREEN PROVES NOTHING ON ITS OWN.** Neither production edit (Task 11's
lift, Task 12's stamp) touches `chainmatch.go`, so this arm is green before and
after them by construction. **Its entire falsifiability is NC roster row 6**, run
below BEFORE the green was believed.

### Step 4, PRE-BELIEVING FALSIFIABILITY CONTROL — NC roster row 6, RUN

Baseline digest first:

```sh
sha256sum internal/listener/listenerfilter/chainmatch.go
e5c0c6ccc31890d19f3a713995f09784a6bf65230f9a58bf22a979743c036ac3  internal/listener/listenerfilter/chainmatch.go
```

**The mutation** — the stamp moved INTO the callee, inserted as the first statement
of `SelectChain` (located by the function's literal signature, never by line
number). `inputs` is passed **by value**, so this is local to the callee; that is
precisely what makes the mutation invisible to `serveConnection`:

```go
func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error) {
	if inputs.TransportProtocol == "" {
		inputs.TransportProtocol = "raw_buffer"
	} // NC-ROW-6 THROWAWAY MUTATION
	// Pass 1: eligibility.
```

`gofmt -l` on the mutated file printed **nothing**. **Neutralise, never break:**

```sh
go build ./...   # BUILD_RC=0 under the mutation
```

**Result under the mutation — the purity arm goes RED, verbatim:**

```
=== RUN   TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain
    chainmatch_test.go:224: SelectChain(empty TP, raw_buffer chain, no default) = (&{rb false 0 [] [] raw_buffer [] false false [] []}, <nil>); want (nil, ErrNoChainMatched) — SelectChain must NOT default the input
    chainmatch_test.go:227: SelectChain(empty TP, raw_buffer chain, no default) returned chain &{rb false 0 [] [] raw_buffer [] false false [] []}; want nil
--- FAIL: TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain (0.00s)
=== RUN   TestSelectChain_ClassifiedTransportProtocolStillMatches
--- PASS: TestSelectChain_ClassifiedTransportProtocolStillMatches (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.002s
```

RC = **1**. The mutated line is proven **executable**, not merely present: the
failure prints the `rb` chain as the returned value, which can only happen if the
defaulted `"raw_buffer"` made it eligible.

**The matched negative stayed `--- PASS` under the mutation** (visible in the block
above) **and passes un-mutated** (Step 4's block). ⇒ **the arm is not a blanket
detector**, and NC roster row 6's "must NOT redden: every other arm" column holds
for its own companion.

**REVERT — complete, and proven complete:**

```sh
git checkout -- internal/listener/listenerfilter/chainmatch.go
sha256sum internal/listener/listenerfilter/chainmatch.go
e5c0c6ccc31890d19f3a713995f09784a6bf65230f9a58bf22a979743c036ac3  internal/listener/listenerfilter/chainmatch.go   # == baseline
git diff master --numstat -- internal/listener/listenerfilter/chainmatch.go
(empty)
/usr/bin/grep -rc 'NC-ROW-6' internal/listener/listenerfilter/chainmatch.go
0
```

`internal/listener/manager.go` was **never opened for write** by this task; its
digest is unchanged at
`a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f`, matching the
byte-untouched roster. The post-revert run reproduces Step 4's green exactly.

### Step 5 — gates and collateral

| gate | command | result |
| --- | --- | --- |
| `gofmt` | `gofmt -l internal/listener/listenerfilter/chainmatch_test.go` | **no output** (gated on OUTPUT, not exit code) |
| `go vet` | `go vet ./internal/listener/...` | RC = **0**, no output |
| `go build` | `go build ./...` | RC = **0**, no output |

⚠️ `golangci-lint` **was not run** — it cannot run in this environment
(v1.64.8/go1.26.2 vs go1.27.1; `typecheck` fails on `sync/atomic` even on untouched
master).

**Collateral** — `go test -count=1 ./internal/listener/...`, RC = 1, with
**exactly THREE** `--- FAIL` rows, the three this stage expects at the un-fixed tip:

```
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue   (Task 2)
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes             (Task 3, (s1))
--- FAIL: TestQUICChainSelection_TransportProtocolBogusDoesNotMatch             (Task 5)
```

`grep -c` over the run output = **3**. No fourth RED. `./internal/listener/listenerfilter`
and `./internal/listener/listenerfilter/tls_inspector` are both `ok`.

### Findings

1. ⚠️ **`PLAN.md` §0.13 CONTRADICTS §4 row (a), §6 and §8 on which task lands this
   arm.** §0.13's verdict paragraph says *"**Task 8** lands the arm"*; §4 row (a),
   §6 `### Task 6` and §9.3's widening table all say **Task 6**, and Task 8 is the
   fixture `expectations.yaml`/`README.md` task, which cannot host a Go unit test.
   **Task 6 is correct; §0.13's "Task 8" is a stale figure from an earlier task
   numbering.** Recorded rather than silently obeyed, because obeying it would have
   left this arm unwritten through two more tasks.
2. 🟢 **The brief's stated facts all held.** `chainmatch_test.go` was 212 lines and
   already imported `errors`; `ErrNoChainMatched` is at `:52` and
   `ErrAmbiguousChainMatch` at `:59`; `TestSelectChainTransportProtocol` at `:188`
   passes `"tls"` and never `""`; the package is `listenerfilter` so all three
   symbols are called unqualified. Nothing in the PLAN's code block needed adapting
   beyond the `quic.go:187` anchor.
3. ⚠️ **This arm is `+34 / −0` in a file no gate counts, and Task 19 must diff the
   ARM ROSTER, not the counters.** Deleting both functions would leave `gofmt`,
   `vet`, `build` and every other arm green. The two test names are the only
   artifact that records their existence.
4. 🟢 **The purity arm carries a second assertion the PLAN's draft omits**
   (`got != nil`). The PLAN's block asserts only the error. A mutation that returned
   a non-nil chain *alongside* `ErrNoChainMatched` would have passed the drafted
   arm; the contract in `chainmatch.go:52`'s comment is `(nil, ErrNoChainMatched)`,
   so both halves are pinned. Under the row-6 mutation **both** lines fired, which
   is why the RED above has two `chainmatch_test.go:` rows.

---

## Task 7 — fixture `0123` driver: three plaintext listeners, no listener filters

**Sub-step count: 7** against the PLAN's chartered **8**. `BOOTSTRAP_PROMPT.md`
§6.1's mid-execution split trigger (~10) **did NOT fire**. Task 7 was one of the two
the PLAN flagged as the real split risk (it ran ELEVEN at phase 97); it did not
recur, because the two sub-steps phase 97 spent on per-side YAML divergence were
collapsed into one shared renderer (Step 3 below).

### Step 1 — the two PLAN symbol claims, RESOLVED against the tree

🔴 **C1 CONFIRMED — `ReferenceConfig()` DOES NOT EXIST.** `PLAN.md` §6 Task 7's
"Produces" list names it, and `SubjectConfig()` with no arguments.

```sh
git grep -c 'ReferenceConfig' -- '*.go'
(no output, rc=1)

git grep -c 'ReferenceBootstrap' -- '*.go'      # positive control
test/differential/fixture/fixture.go:4
test/differential/fixture/fixture_test.go:3
test/differential/harness.go:3
test/differential/runner_test.go:7
... 140 driver files
(rc=0)
```

The real `fixture.Driver` (`test/differential/fixture/fixture.go:15-39`) declares
`ReferenceBootstrap(backendPorts []int) string` and
`SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string`
— **four arguments, not zero.** Both are implemented at the real signatures. This is
the **third** PLAN "Interfaces" symbol this stage to turn out not to exist
(`runtimeFor`, `ReferenceConfig`); every symbol below was resolved in source before
use.

🔴 **C2 CONFIRMED, AND THE MISCOUNT REPRODUCED.** `PLAN.md` §6 Task 8 Step 2 directs
the scrape at `/stats/prometheus`. The shape precedent `0122` uses the **FLAT**
`/stats`:

```sh
git grep -n 'url := "http://" + adminAddr' -- test/fixtures/0122-quic-chain-selection/driver/driver.go
test/fixtures/0122-quic-chain-selection/driver/driver.go:227:	url := "http://" + adminAddr + "/stats"

git grep -n '/stats/prometheus' -- test/fixtures/0122-quic-chain-selection/driver/driver.go
test/fixtures/0122-quic-chain-selection/driver/driver.go:222:// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
```

⚠️ **0122's ONLY `/stats/prometheus` occurrence is a COMMENT saying it does not use
it** — a naive name-grep scores 0122 as a prometheus scraper and is wrong by
inversion. `0123` scrapes the **flat `/stats`**. Census denominator re-derived at
this tip:

```sh
git grep -l 'func.*AssertStats(t fixture.TB' -- 'test/fixtures/*' | grep -v 0123 | wc -l
88
```

— matching the brief's stated 88 exactly (the unfiltered count reads 89 now that
`0123` is on disk, which is the new file, not drift).

### Step 2 — the multi-listener contract, MEASURED not assumed

| claim | verified at | reading |
| --- | --- | --- |
| runner zips index-wise, `t.Fatalf` on length mismatch | `runner_test.go:1238-1249` | confirmed verbatim |
| `refPorts = mld.ReferenceListenerPorts()` | `runner_test.go:1161-1165` | **all three ports are published automatically** — the fixture needs no publishing work of its own |
| `ref.ListenerAddr(port)` | `harness.go:209` | `r.tcpAddrs[containerPort]` — the HOST-side Docker mapping, keyed by the IN-CONTAINER port |
| `subj.ListenerAddr(name)` | `harness.go:280` | `s.listenerAddrs[name]`, populated from the ADR-0026 ready sentinel (`harness.go:63-92`) |
| `subjectPortBlockSpan` | `harness_test.go:270` | **16** — `subjPort+1/+2` sits well inside the probed reservation |
| singular accessors still needed | `runner_test.go:1212,1276` | confirmed — both are read before the multi branch |

⚠️ **A FINDING AGAINST THE ARITY PRECEDENT.** `0018`'s `deriveAddrsFromRef`
(`inputs/driver.go:966-977`) derives sibling REFERENCE addresses by string-replacing
one in-container port for another. That is **wrong** on the reference side: host
ports are Docker-assigned and bear no arithmetic relation to the container ports or
to each other. It is harmless there only because it is unreachable
(`MultiListenerDriver` is implemented, so the runner never calls the singular
`DriveReference`). **`0123` does not copy it** — its singular `DriveReference` /
`DriveSubject` drive `arms[0]` against the one address they are handed, which is
correct if ever reached. Arity itself (three names, three ports) is taken from
`0018:266-274` as chartered.

### Step 3 — the driver

`test/fixtures/0123-listener-transport-protocol/driver/driver.go`, **733 lines**,
**no committed YAML** (the `0122` shape — both bootstraps rendered in the driver).

```sh
git diff --numstat -- test/fixtures/0123-listener-transport-protocol/
733	0	test/fixtures/0123-listener-transport-protocol/driver/driver.go
```

**ONE renderer, not two literal templates.** `0122` carries the reference and
subject bootstraps as two hand-maintained blobs. At three listeners x two chains
that would be ~250 lines of YAML per side, and the proposition under test is that
two proxies given the **same** listener shape select **different** chains — so
shape-identity is load-bearing and must not be a review exercise that rots.
`renderBootstrap` builds both sides and the only cross-side differences are bind
address, admin port and the three listener ports. This is the one deliberate
departure from the `0122` shape, and it is what kept the sub-step count at 7.

The roster (`arms`) is the single ordered source for BOTH `SubjectListenerNames()`
and `ReferenceListenerPorts()`, so the index-wise zip the runner performs cannot be
desynchronised by editing one accessor.

### Step 4 — ports, RE-CENSUSED at this tip

```sh
for p in 15123 15223 15224; do git grep -l "\b$p\b" -- test/ internal/ cmd/ | wc -l; done
0
0
0

ss -tanH | grep -cE ':(15123|15223|15224)\b'
0
```

Zero files and zero live sockets for each. `15123` on the `15000 + <index>`
convention; `15223`/`15224` off-convention so `0124`/`0125` keep theirs.

### Step 5 — build, vet, format

```sh
go build ./test/fixtures/0123-listener-transport-protocol/...   # rc=0
go vet   ./test/fixtures/0123-listener-transport-protocol/...   # rc=0
gofmt -l test/fixtures/0123-listener-transport-protocol/driver/driver.go
(no output)
```

⚠️ `gofmt -l` is gated on **OUTPUT**, not exit code. It listed the file on the first
pass (argument-comment alignment in `renderBootstrap`); after `gofmt -w` it prints
nothing.

Compile-time interface assertions, the `0122:215-219` precedent block:

```go
var (
	_ fixture.Driver              = (*tpDriver)(nil)
	_ fixture.MultiListenerDriver = (*tpDriver)(nil)
	_ fixture.StatsAsserter       = (*tpDriver)(nil)
)
```

All three runner dispatches are **silent type assertions with no else branch**
(`runner_test.go:1160`, `:1238`, `:1352`), so a signature typo would downgrade the
fixture to the single-addr path or skip the whole stats leg with no diagnostic.

**The blank import in `runner_test.go` was NOT added** — that is Task 9's, which must
demonstrate the four registration gates firing from a clean starting state.

### Step 6 — the rendered shape, and a LIVE boot probe (no Docker)

The configs were rendered through a `go test -overlay` injection (scratch-only; **no
temp file entered the worktree**) and inspected:

```
ref.yaml  6585 bytes   subj.yaml  6594 bytes
l_bogus 0.0.0.0:15123  transport_protocol: totally_bogus_value
l_raw   0.0.0.0:15223  transport_protocol: raw_buffer
l_tls   0.0.0.0:15224  transport_protocol: tls
```

`grep -nE 'listener_filters|transport_socket'` over **both** rendered files reads
**zero hits** — no listener filter and no TLS on any listener, as chartered. Six
HCMs, six distinct `stat_prefix`es, six distinct `direct_response` bodies, all at
status **200**.

**ARM A — the config AS RENDERED, against the un-fixed subject:**

```
rc=1
listener manager: listener: "l_bogus": filter_chains[0]: transport_protocol
  "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty
```

Exactly ONE reject, and it is the §4.1(1) reject this phase lifts — so **every other
construct in the bootstrap parsed.**

**ARM B — the MATCHED CONTROL, `totally_bogus_value` -> `raw_buffer`, nothing else
changed:**

```
envoy-go listener l_bogus ready on 127.0.0.1:20500
envoy-go listener l_raw   ready on 127.0.0.1:20501
envoy-go listener l_tls   ready on 127.0.0.1:20502
envoy-go ready
rc=124
```

⚠️ rc=124 is `timeout`'s, shared by a healthy server and a hung boot — **the OUTPUT
discriminates**, and it carries all three ADR-0026 sentinels plus the terminal one.
This answers the brief's open question: **envoy-go ACCEPTS the
`direct_response` + `default_filter_chain` + placeholder-`clusters` shape**, the
three listener names match the sentinel format `harness.go:63` parses, and the six
distinct `stat_prefix`es do **not** trip the banked duplicate-metric-registration
panic.

🟢 **SPEC §7.3's "prove each counter NAME is emitted by BOTH sides before pinning
it" is DISCHARGED FOR THE SUBJECT SIDE**, by measurement rather than by assumption.
One GET per listener against ARM B, then the flat `/stats`:

```
http.bogus_default.downstream_rq_total: 1     l_bogus -> bogus-default
http.bogus_indexed.downstream_rq_total: 0
http.raw_default.downstream_rq_total: 1       l_raw   -> raw-default
http.raw_indexed.downstream_rq_total: 0
http.tls_default.downstream_rq_total: 1       l_tls   -> tls-default
http.tls_indexed.downstream_rq_total: 0
```

**All six names are present, including the three non-serving `*_indexed` ones at
0** — so the `== 0` half of each pin is guarded by a presence check that is
satisfiable rather than vacuous. The reference half of §7.3 needs Docker and belongs
to Task 9.

That reading also **pre-measures the un-fixed tip's verdict**: with the reject
lifted by hand, the subject answers `DEFAULT` on **all three** listeners. So at
Task 9 `l_raw` is the **RED** arm; `l_bogus` and `l_tls` are **structurally GREEN**
— the false-agreement class SPEC §7.1 names — and neither, alone, is evidence.

### Step 7 — byte-untouched proof and commit

```sh
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go

git diff master --numstat -- internal/listener/manager.go
(empty)
```

Digest matches the mandated value; Tasks 11/12 retain ownership.

### Findings

1. 🔴 **`ReferenceConfig()` does not exist (C1), and the naive prometheus census is
   wrong by INVERSION (C2).** Both controller corrections held under re-measurement.
   C2's failure mode is worth recording in its own right: the grep that miscounts
   `0122` matches a comment whose text **denies** the very thing the match asserts.
   A name-presence grep cannot tell a use from a disclaimer.
2. ⚠️ **`0018`'s `deriveAddrsFromRef` is wrong-but-unreachable**, and copying it
   would have shipped a latent defect into `0123`. Reference-side host ports are
   Docker-assigned; no offset derivation from a sibling's host address is valid.
   Recorded because `0018` is the arity precedent the PLAN and this brief both name,
   so the next fixture is likely to read it.
3. 🟢 **The subject emits all six per-chain counter names**, non-serving chains
   included. Had it not, every `== 0` pin in `AssertStats` would have been silently
   vacuous on the subject rather than red — the presence checks exist for exactly
   that case and are now known to be satisfiable, not decorative.
4. ⚠️ **`AssertStats`, not `Drive`, carries the absolute body assertions.** A
   wrong-chain body returned as an error from `DriveXMulti` becomes `t.Fatalf` in
   the runner, and a `Fatalf` on the first failing listener would **mask the other
   two arms**. With three arms and a known-RED one (`l_raw`), first-divergence
   masking would have hidden precisely the two arms that distinguish "matched" from
   "unenforced". The driver is therefore stateful by design, and `AssertStats` runs
   at `runner_test.go:1352` — after the byte diff, which is `Errorf` not `Fatalf`,
   so it is reached even on a mismatch.
5. ⚠️ **`PLAN.md` §6 Task 7's Step 3 table has a mangled final cell** — the `l_tls`
   row reads *"or stamps a constant `tls` matches"*. `SPEC.md` §7.1's copy is
   identical, so the defect is in the SPEC and was inherited by the PLAN. The
   intended reading (a subject that stamps a constant `tls` would match this chain
   and is caught only here) is what the driver's comment records.

---

## Task 8 — fixture `0123`: `expectations.yaml` and `README.md`, VALUE pairs not name presence

**Sub-step count: 10**, against the PLAN's chartered **6** for this task.
`BOOTSTRAP_PROMPT.md` §6.1's mid-execution split trigger is **~10**, so this task
**reached the line**. It was not split: sub-steps 7-8 were an unplanned
re-derivation and correction of a published figure (below), and by the time the
trigger was reachable the only work left was one `PROGRESS.md` append and the
commit. Recorded rather than rounded down.

### 🔴 The brief's Step 5 premise is REFUTED: NOTHING consumes `expectations.yaml`

The brief directs: *"read how the runner consumes `expectations.yaml` … if
`0122`'s schema and the runner's reader disagree, believe the runner."* **There
is no reader.**

```sh
git grep -n 'expectations' -- 'test/differential/*.go'
(no output, rc=1)

git grep -rln 'gopkg.in/yaml\|sigs.k8s.io/yaml' -- test/
(no output)
```

Zero lines in the runner package mention the file, and **no package under
`test/` imports a YAML library at all**. The enforcers are the driver's
`AssertStats`, the runner's `CompareBytes` and `ProbeAdmin`. ⇒ **There is no
schema to conform to, and no "believe the runner" adjudication to make** — the
question is undefined, not unanswered. The file is prose under ADR-0019, which
`0122`'s own header states in its first three lines.

**The shape precedent census, run because "prose" is not universal in this
tree:**

| fixture | non-comment lines / total |
| --- | --- |
| `0118-runtime-static-layer` | **0** / 251 |
| `0119-grpc-unary-trailers` | 3 / 106 |
| `0120-tls-connection-error` | 107 / 388 |
| `0121-listener-default-chain-tls` | **0** / 175 |
| `0122-quic-chain-selection` | **0** / 174 |

100 fixtures carry an `expectations.yaml`; the three most recent (`0121`,
`0122`, and `0118`) are **pure comment prose**. `0123` follows them.

**It parses**, which is the only mechanical property the file has:

```sh
python3 -c "import yaml;print(repr(yaml.safe_load(open('.../0123-.../expectations.yaml'))))"
None
```

`None` — an all-comment document is a valid YAML document that loads to null.
The `0122` control reads `None` too, so `0123` is byte-behaviourally identical
to its precedent on the only surface that can be measured.

### Step 1 — the pins are VALUE pairs; presence is a GUARD, never the assertion

The driver already enforces this (Task 7); Task 8's job was to document it
without weakening it. Both files state the distinction explicitly. The pinned
values, per side, per listener:

| name | pinned | name | pinned |
| --- | --- | --- | --- |
| `http.raw_indexed.downstream_rq_total` | **1** | `http.raw_default.downstream_rq_total` | **0** |
| `http.tls_indexed.downstream_rq_total` | **0** | `http.tls_default.downstream_rq_total` | **1** |
| `http.bogus_indexed.downstream_rq_total` | **0** | `http.bogus_default.downstream_rq_total` | **1** |

The rationale recorded in both files: a name-presence pin is **satisfied from
boot** — §0.6's pre-traffic reference scrape read **168** `chain_(indexed|default)`
lines with **zero** connections, because the per-chain HCM stat scope is created
at **config** time. Presence is kept only as the guard that keeps each `== 0`
non-vacuous (a scrape map returns zero for a missing key), and Task 7's
subject-side measurement showed all six names **present**, the three non-serving
ones **present at 0** — so the guard is satisfiable, not decorative.

### Step 2 — the flat `/stats` kept, and 🔴 THE BRIEF'S OWN FIGURE CORRECTED

The controller's correction to `PLAN.md` §6 Task 8 Step 2 **holds**: `0123`
keeps the flat `/stats`. But the brief's **"31 use prometheus, 57 use the flat
`/stats`"** does not survive re-derivation at this commit. Comment-stripped
(whole-line `//` only — ⚠️ a naive `s://.*::` strip **destroys `http://`** and
scored `prometheus=10 flat=0`, a self-inflicted false reading caught and
discarded):

| endpoint | drivers |
| --- | --- |
| `/stats/prometheus` | **31** |
| the flat `"/stats"` | **47** |
| **neither** | **10** |

Denominator **88**, confirmed. The **31 is exact**; the **57 is not** — it folds
in ten stats-**sink** drivers (`0089`-`0094`, `0098`, `0101`, `0112`, `0113`)
that scrape **no admin endpoint at all** and assert against a receiver
(`test/helpers/statsdrecv` and siblings). `0112` reads **zero** occurrences of
`"/stats`. Both new files publish **31 / 47 / 10** and name the "31 / 57" split
as the error, because a figure republished without its denominator's definition
rots the same way the original did.

The qualitative conclusion is unchanged and in fact strengthened: **47 > 31**,
and `0122` is one of the 47.

### Steps 3-4 — the body as primary discriminator, and the two disarming hazards

`README.md` states both hazards in its own words, each under its own heading:

- **(a)** adding a listener filter to **any** listener disarms `l_raw` —
  `tls_inspector` stamps `raw_buffer` on a plaintext preamble from envoy-go's
  **own** pipeline, so both sides agree at the un-fixed tip and the fixture runs
  **GREEN over a live divergence in the exact dimension it claims to cover**.
- **(b)** `l_bogus` **alone** is a false-agreement arm — a subject that ignores
  the dimension answers `DEFAULT` too. Only the `l_raw` / `l_tls` pair excludes a
  constant answer, recorded as a four-row table (stamps nothing / constant `tls`
  / constant `raw_buffer` / correct) showing which half fails in each case.
- **(c)**, added: the six `stat_prefix` values must stay distinct or envoy-go
  panics at boot on duplicate metric registration.

The un-fixed-tip verdict is recorded in both files: the subject answers
`DEFAULT` on **all three** listeners, so **`l_raw` is the RED arm** and
`l_bogus` / `l_tls` are **structurally GREEN** — neither is evidence on its own.

The **body** is documented as the primary discriminator and the counters as
corroboration, with the reason: a body is controlled by this fixture, a counter
is controlled by each proxy's stats implementation.

### Step 5 — gates

```sh
python3 -c "import yaml; yaml.safe_load(open('expectations.yaml'))"   # None, rc=0
go build ./test/fixtures/0123-listener-transport-protocol/...          # rc=0
go vet   ./test/fixtures/0123-listener-transport-protocol/...          # rc=0
gofmt -l test/fixtures/0123-listener-transport-protocol/
(no output)

git diff --numstat -- test/fixtures/0123-listener-transport-protocol/
334	0	test/fixtures/0123-listener-transport-protocol/README.md
276	0	test/fixtures/0123-listener-transport-protocol/expectations.yaml
```

No `.go` file was touched this task; `build`/`vet`/`gofmt` are run anyway as the
unchanged-still-green control. Ports re-censused: `15123`, `15223`, `15224` each
read **zero** files outside `0123` itself.

⚠️ **The blank import in `runner_test.go` was NOT added** — Task 9's, and it must
start from a clean state.

```sh
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go

git diff master --numstat -- internal/listener/manager.go
(empty)
```

### Findings

1. 🔴 **`expectations.yaml` has no consumer.** The brief's and the PLAN's framing
   ("matches what the runner expects", "believe the runner") presupposes a reader
   that does not exist anywhere in `test/`. Recorded so the next fixture stage
   does not spend a sub-step looking for it again.
2. 🔴 **The brief's "57 flat" is wrong; the figure is 47, with 10 scraping
   neither endpoint.** The *correction* to the PLAN was right and the *count*
   inside it was not — a corrected row can still ship a false figure. Re-derived
   at this commit rather than cited.
3. ⚠️ **A comment-stripping one-liner can manufacture the wrong answer
   silently.** `sed 's://.*::'` deletes the `//` in `http://`, which turned
   `url := "http://" + adminAddr + "/stats"` into a line matching neither
   endpoint and produced `31 -> 10`, `47 -> 0`. It failed *quietly*, with a
   plausible-looking split. Strip only whole-line `//` comments.
4. ✅ **`no_filter_chain_match` re-verified as having no production site**:
   `git grep -n` over `internal/**` and `cmd/**` returns exactly **one** line,
   `internal/listener/tls_handshake_negative_test.go:25`, and it is a **comment
   in a test file**. The driver's refusal to pin that name is therefore correct,
   and both new files record why.

## Task 9 — the FOUR registration gates, PROVEN and NC'd

**Change:** exactly one line, the blank import, in `test/differential/runner_test.go`.

```
git diff --numstat -- test/differential/runner_test.go
1	0	test/differential/runner_test.go
```

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0123-listener-transport-protocol/driver"
```

Inserted between the `0122-quic-chain-selection/driver` line and
`"github.com/pgdad/envoy-go/test/helpers"`; `gofmt -l` is silent, so the import
group is still sorted.

### Step 1 — the four gates, each with the check that established it

| # | gate | how it was checked | result |
|---|---|---|---|
| 1 | `fixture.RegisterFixture` in the driver's `init()` | `grep -n 'RegisterFixture'` on the driver | `driver.go:212: func init() { fixture.RegisterFixture(fixtureName, &tpDriver{}) }` |
| 2 | the blank import in `runner_test.go` | the set-difference below, **plus a matched negative** that deletes the import and measures the `SKIP` | present; its removal SKIPS (see Step 4b) |
| 3 | **byte-identity** dir name vs registered string | `od -c` on both, then `[ "$CONST" = "$DIR" ]` | both are the identical 32 bytes `0123-listener-transport-protocol`; **BYTE_IDENTICAL** |
| 4 | the `NNNN-` shape `discoverFixtures` enumerates | read `runner_test.go:1465-1500`: needs `len>=5`, `isNumeric(name[:4])`, `name[4]=='-'`; `"0123"` is numeric and `name[4]=='-'` — **and** empirically the subtest `TestDifferential/0123-listener-transport-protocol` appeared in the roster, which only an enumerated directory can do | satisfied |

⚠️ Gate 4 is the one that leaves **no trace at all** when it fails — no subtest,
therefore not even a skip line. It is the only gate here verified both by
reading the enumerator and by observing the subtest name in a real run; a
gate-4 miss is invisible to the `SKIP` counter that catches gates 1-3.

### Step 2 — the set-difference, BEFORE and AFTER

The extractor (the PLAN's, verbatim), scoring on the **set difference**, never
on a count and never on an exit code:

```sh
extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
```

**BEFORE** (tip `953f0b9c`):

```
imports=124  dirs=125
comm -23 (registered, no dir)  -> (empty)
comm -13 (dir, not registered) -> 0123-listener-transport-protocol
split: driver=100  inputs=24
```

**AFTER**:

```
imports=125  dirs=125
comm -23 -> (empty)
comm -13 -> (empty)
split: driver=101  inputs=24   total=125
```

**125 = 125, split 101 + 24**, both directions empty — as PLAN §6 Task 9 requires.
The router's method note 3e (`123/123`, `99+24`) is confirmed **STALE** by two
generations, not one: the pre-`0123` tip already read `124 = 124` / `100+24`.

### Step 3 — NC by RENAME (both directions fire)

Scratch copy `$SCRATCH/nc_rename.go`, `0122-quic-chain-selection/driver` renamed
to `0122-quic-chain-selectionX/driver`:

```
imports=125   <- INVARIANT vs the AFTER state's 125
comm -23 -> 0122-quic-chain-selectionX      (registered, no dir)
comm -13 -> 0122-quic-chain-selection       (dir, not registered)
```

Both directions fired while **the count did not move at all** — the PLAN's
warning is confirmed empirically: a count-only registration check is vacuous
against a rename, and would have passed a fixture wired to a directory that
does not exist.

### Step 4 — NC by DELETION

Scratch copy `$SCRATCH/nc_delete.go`, the same import line deleted:

```
imports=124
comm -23 -> (empty)
comm -13 -> 0122-quic-chain-selection
```

🔴 **The PLAN's and the brief's Step 4 name the WRONG DIRECTION.** Both say a
deletion "fires ONLY `comm -23`". It fires only **`comm -13`**. The direction is
forced by `comm`'s own semantics, which the PLAN's Step-2 comments state
correctly two steps earlier: `comm -23` suppresses columns 2 and 3 and therefore
prints lines **unique to file 1 = `imports.txt`** (registered but no dir), while
`comm -13` prints lines unique to file 2 = `dirs.txt` (dir but not registered).
Deleting an import removes a name from `imports.txt` while the directory stays
on disk, so the name can only surface as **dir-but-not-registered**. The two
controls remain genuinely distinct — rename fires **both**, deletion fires
**one** — so the PLAN's point stands; only its label was inverted. Anyone who
had scored Step 4 by the PLAN's stated direction would have recorded an empty
`comm -23` as a **failed NC** and concluded the extractor was blind.

### Step 4b — matched negative for gate 2 (added; not chartered)

Gates 1-3 are asserted to "converge on a `t.Skipf`". That was a claim about
code, so it was measured. The blank import was deleted from the real
`runner_test.go` and the same selector re-run:

```
=== RUN   TestDifferential/0123-listener-transport-protocol
    runner_test.go:203: no driver registered for fixture "0123-listener-transport-protocol" (driver package not yet blank-imported in runner_test.go)
--- PASS: TestDifferential (0.00s)
    --- SKIP: TestDifferential/0123-listener-transport-protocol (0.00s)
ok  	github.com/pgdad/envoy-go/test/differential	0.077s        rc=0
```

A registration miss is **rc=0, PASS, one SKIP** — it does not FAIL. The file was
restored from a pre-NC copy and `sha256sum -c` re-verified **OK**, so the
committed line is byte-for-byte the one that was measured.

### Step 5 — the fixture runs as a real subtest

```sh
go test -count=1 -v -run 'TestDifferential/0123-listener-transport-protocol' ./test/differential/
```

```
=== RUN count  = 2     (TestDifferential, TestDifferential/0123-listener-transport-protocol)
SKIP count     = 0     (grep exit 1 on zero matches — captured, not chained)
'no tests to run' = 0  (the EXITS-0 no-match footgun did not fire)
```

Reference image resolved as `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`,
byte-matching `docs/envoy-go/ENVOY_TARGET.md` line 4. It was already cached; no pull.

### Step 6 — the un-fixed-tip FAILURE, verbatim, scored by MESSAGE

```
2026/09/20 22:10:01 listener manager: listener: "l_bogus": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty
    runner_test.go:1219: subj start attempt 1 failed (subject ready: EOF); retrying with fresh ports
2026/09/20 22:10:02 listener manager: listener: "l_bogus": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty
    runner_test.go:1219: subj start attempt 2 failed (subject ready: EOF); retrying with fresh ports
2026/09/20 22:10:04 listener manager: listener: "l_bogus": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty
    runner_test.go:1219: subj start (attempt 3): subject ready: EOF
--- FAIL: TestDifferential (5.42s)
    --- FAIL: TestDifferential/0123-listener-transport-protocol (5.42s)
FAIL	github.com/pgdad/envoy-go/test/differential	5.477s
```

**This is the CORRECT un-fixed outcome.** `l_bogus` boot-rejects the subject, so
the fixture dies at the boot step and **no arm is scored** — `l_raw`'s divergence
is not even reached yet. Task 11's lift is what makes the subject boot, and only
then does `l_raw` become a scored RED arm.

The reproduction is proven to have reached the **intended site by the message**,
not by the exit code: the reject names `transport_protocol "totally_bogus_value"`
on listener `"l_bogus"`, `filter_chains[0]`, three times (once per port-retry
attempt). An unrelated boot reject, a Docker failure or a port-band collision
would be indistinguishable at rc level — all of them are rc=1. The reference
container started and passed its `/ready` probe before the subject was even
attempted, so the reference side is not implicated.

### Containers

Baseline **91** containers before the run (69 of them foreign, from a sibling
`cache-platform` session). After: **92**. The single new one is
`reaper_8156a6a2af2abb5b55cab3dad101b81b3bbdbf35264cb45d73c22db3a3e31430`, the
testcontainers Ryuk the differential creates and REUSES — **deliberately left
alone**. The reference container `affac7b5d3e1` was created and terminated by the
test itself. **Nothing was torn down by this task**; no foreign container was
touched, and `comm -23` over the before/after name lists is empty, proving no
container disappeared.

### Gates

```sh
gofmt -l test/differential/runner_test.go     # (no output)
go vet   ./test/differential/                 # rc=0

sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go
```

`manager.go` is byte-untouched; Tasks 11/12 own it.

### Findings

1. 🔴 **PLAN §6 Task 9 Step 4 (and the brief) invert the `comm` direction.** A
   deletion fires `comm -13`, not `comm -23`. Measured, not argued. The control
   itself is sound; only its expected-direction label is wrong.
2. ✅ **The rename NC is not merely stronger than a count — the count is exactly
   invariant.** 125 before, 125 after the rename. Recorded as a measurement
   rather than a restatement of the PLAN's warning.
3. ⚠️ **The un-fixed failure repeats THREE times**, because the runner's
   port-retry loop re-boots the subject on fresh ports before giving up
   (`runner_test.go:1219`). A boot-reject is not a port problem, so the retries
   are pure noise here — but a reader counting occurrences of the reject message
   should not read "3" as three listeners.
4. ✅ **Gate 4 was verified twice, by different means**, because it is the only
   one of the four whose failure leaves no observable trace: reading the
   `discoverFixtures` predicate, and observing the enumerated subtest name.

**Sub-step count: 10.** Seven chartered plus three: the gate-2 matched negative
(Step 4b), the container/image baseline, and the final gate sweep. **It did not
blow past ~10**, so §6.1's mid-execution split trigger did not fire.

## Task 10 — the un-fixed-tip PER-ARM roster, SPENT and unrecoverable

⚠️ **This section is the last measurement that can ever be taken at the un-fixed
tip.** Task 11 lifts the enum reject and Task 12 adds the TCP `raw_buffer` stamp;
from that commit forward nothing in the tree can reproduce what is recorded here.
Everything below is ACTUAL output from a run at `1081f786`, not a restatement of
the PLAN's expectations.

### Step 1 — the full reverse-dependency sweep, re-run exactly as Task 1 ran it

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > $SCRATCH/task10.txt 2>&1
rc=${PIPESTATUS[0]}   # RC=1
```

The selector still resolves to the **same SEVEN packages** Task 1 recorded
(`go list` over the identical selector):

```
cmd/envoy-go
internal/admin
internal/boot
internal/listener
internal/listener/listenerfilter
internal/listener/listenerfilter/tls_inspector
validate
```

| measure | Task 1 (un-fixed, no arms) | Task 10 (un-fixed, arms landed) | delta |
|---|---|---|---|
| packages resolved | 7 | **7** | 0 |
| `RC` | 0 | **1** | **0 → 1** |
| `=== RUN` | 392 | **398** | **+6** |
| anchored `FAIL` (`^(FAIL\|--- FAIL)\|^ *--- FAIL`) | 0 | **6** | **+6** |
| panic gate (`^panic:\|DATA RACE\|SIGSEGV`) | 0 | **0** | 0 |
| `address already in use` | 0 | **0** | 0 |

The six anchored FAIL lines are **three failing tests** plus three structural
lines — `--- FAIL:` ×3, the package's own `FAIL`, `FAIL<TAB>…internal/listener`,
and the trailing binary-level `FAIL`. **Six is not six failures.** Per-package:

```
ok   github.com/pgdad/envoy-go/cmd/envoy-go                              11.213s
ok   github.com/pgdad/envoy-go/internal/admin                             1.475s
ok   github.com/pgdad/envoy-go/internal/boot                              0.250s
FAIL github.com/pgdad/envoy-go/internal/listener                          3.269s
ok   github.com/pgdad/envoy-go/internal/listener/listenerfilter           0.043s
ok   github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector 0.003s
ok   github.com/pgdad/envoy-go/validate                                   0.241s
```

Internal reconciliation of the run: 394 `--- PASS` + 3 `--- FAIL` + 1 `--- SKIP`
= **398**, exactly the `=== RUN` count. Nothing was silently dropped.

### Step 2 — the ROSTER diff, not the count

⚠️ **The `+6` in the table above is a COUNT measure and is weaker than what
follows.** A count cannot distinguish "added 7, removed 1" from "added 6" or from
"added 8, removed 2 including a rename". The roster was therefore produced with
the **identical pipeline** Task 1 used —

```sh
/usr/bin/grep -E '^\s*=== RUN' task10.txt | sed -E 's/^\s*=== RUN\s+//' \
  | cut -d/ -f1 | sort -u          # 315 distinct top-level names (Task 1: 309)
```

— and diffed against `$SCRATCH/t1/roster-unfixed-task1.txt` with `comm`.

**ADDED (7)** — `comm -13 t1 t10`:

```
TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
TestQUICChainSelection_TransportProtocolBogusDoesNotMatch
TestSelectChain_ClassifiedTransportProtocolStillMatches
TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain
TestServeConnection_NoListenerFilter_RawBufferChainServes
TestServeConnection_NoListenerFilter_TLSChainDoesNotServe
TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
```

**REMOVED (1)** — `comm -23 t1 t10`:

```
TestParseChainSpecRejectsUnknownTransportProtocol
```

**The diff matches the expected set EXACTLY — 7 added, 1 removed, nothing else.**
309 − 1 + 7 = 315 ✓. The one removal is Task 2's re-point of the old
reject-asserting parse test onto the accept assertion, and because it is a
RENAME the roster sees it as a matched add+remove pair — which a count of `+6`
would have concealed entirely.

### Step 3 — the PER-ARM roster at the un-fixed tip

Every row below was **confirmed against the actual run**, by locating the arm's
`=== RUN` line and its own `--- PASS` / `--- FAIL` verdict in `task10.txt`.

| arm | test name | measured | reason (verified in source + output) |
|---|---|---|---|
| §5.1 (a) | `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` | **RED** | the enum switch still rejects `"sctp"` |
| §5.1 (b)(c)(d) | *(same function, inline)* | **unreachable** | (a) is a `t.Fatalf` |
| §5.2 (s1) | `TestServeConnection_NoListenerFilter_RawBufferChainServes` | **RED** | the empty input never matches `raw_buffer` |
| §5.2 (s2) | `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` | **structurally GREEN** | the default chain is the right answer for the wrong reason |
| §5.2 (s3) | `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` | **GREEN** | `tls_inspector` already writes `"tls"`; independent of the stamp |
| §5.3 QUIC | `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | **RED at boot** | the enum switch rejects `"totally_bogus_value"` |
| Task 6 purity arm | `TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` | **GREEN** | pins that `SelectChain` does *not* default; true before and after |
| Task 6 matched negative | `TestSelectChain_ClassifiedTransportProtocolStillMatches` | **GREEN** | proves the purity arm is not a blanket detector |
| fixture `0123` | `TestDifferential/0123-listener-transport-protocol` | **FAILS at boot** | `l_bogus` boot-rejects the subject (Task 9; **not re-run here**) |

**The measured roster agrees with the PLAN's predicted table in every row. There
is no disagreement to report.**

#### The three REDs, with their verbatim failure messages

```
=== RUN   TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
    manager_test.go:1727: (a) NewManager must ACCEPT transport_protocol "sctp", got error: listener: "l_tp": filter_chains[0]: transport_protocol "sctp" must be "tls", "raw_buffer", "quic", or empty
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
```

```
=== RUN   TestServeConnection_NoListenerFilter_RawBufferChainServes
    manager_test.go:1841: tag byte = 'B', want 'A': a TCP connection no listener filter classified must be stamped transport_protocol=raw_buffer, which makes filter_chains[0] eligible. 'B' is default_filter_chain, meaning the detected transport protocol stayed empty
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
```

```
=== RUN   TestQUICChainSelection_TransportProtocolBogusDoesNotMatch
    quic_test.go:866: NewManager(quic, transport_protocol="totally_bogus_value" filter_chains[0] + QUIC-TLS default slot) BOOT-REJECTED: listener: "quic_listener_chains": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty — an unknown transport_protocol is a NON-MATCHING VALUE, not a config error, so parseChainSpec must accept it and let chain selection decide
--- FAIL: TestQUICChainSelection_TransportProtocolBogusDoesNotMatch (0.00s)
```

Each RED failed **by MESSAGE, at the intended site** — not merely by exit code.
The two parse arms name the enum reject; (s1) names the tag byte `'B'` (the
default slot) against the wanted `'A'` (the `raw_buffer` chain). A RED for an
unrelated reason would be indistinguishable at rc level.

**§5.1 (b)(c)(d) unreachability is MEASURED, not assumed.** The run emits exactly
one log line for that test — the `(a)` message — and no `(b)`, `(c)` or `(d)`
line. `(b)/(c)/(d)` are inline in the same function (not subtests), below a
`t.Fatalf`, so they are dead code at the un-fixed tip. **They will only ever be
scored for the first time at Task 11's tip**, and are therefore unproven today.

#### The four pre-fix GREENs — "could the subject have answered anything else?"

⚠️ **"Structurally GREEN" is not a pass, and neither is a plain green here.** For
each, the question Tasks 15/16 depend on:

- **§5.2 (s2)** — chain match is `transport_protocol: tls`, no listener filter, a
  `default_filter_chain` behind tag `'B'`. Un-fixed, the input is `""`, which is
  `!= "tls"`, so the chain is ineligible and the default serves → `'B'`. Fixed,
  the input is `"raw_buffer"`, still `!= "tls"`, so the default serves → `'B'`.
  **The subject could NOT have answered anything else, before or after.** Its
  green is the right answer for the wrong reason pre-fix, and carries **zero**
  evidence about the repair. Its entire falsifiability is the NC roster: it must
  redden under an unconditional `"tls"` stamp (row 5).
- **§5.2 (s3)** — at the un-fixed tip there is **no stamp code at all**, so no
  path exists that could overwrite the inspector's `"tls"`. **The subject could
  not have answered anything else.** Its green pre-fix is structurally
  guaranteed; its only falsifiability is NC row 5b (drop the `== ""` guard), the
  one row under which it is the *only* arm that must redden. The arm's own header
  comment says this outright, and this run confirms it empirically.
- **Task 6 purity arm** — `SelectChain` contains no defaulting code at the
  un-fixed tip, so `ChainMatchInputs{}` against a lone `raw_buffer` chain can
  only return `ErrNoChainMatched`. **The subject could not have answered anything
  else.** It is a *guard against a future regression* (NC row 6 — moving the TCP
  stamp into `SelectChain`), not evidence about this row's repair. Task 6 already
  proved it CAN fire; today it merely does not.
- **Task 6 matched negative** — the matcher already compares
  `TransportProtocol` exactly, so a classified `"tls"` input against a `"tls"`
  chain can only match. **Could not have answered anything else.** Its job is to
  stay green under NC row 6 while the purity arm reddens, proving the purity arm
  is not a blanket "never matches on transport_protocol" detector. A green here
  is *expected* and is not evidence of anything on its own.

**Summary: 3 arms RED (all three by message, at the intended site); 4 arms GREEN,
and NOT ONE of the four could have answered differently at this tip.** No green
in this row is evidence of the repair. All four are falsifiable only through the
NC roster, which is what Tasks 15/16 score.

### Step 4 — panic gate and flake status

```
panic gate (^panic:|DATA RACE|SIGSEGV)   0      (Task 1: 0)
```

`TestEnvoyGoBinary_TwoListenerCutover` **ran and PASSED** (1.39s), and
`grep -c 'address already in use'` over the whole log is **0** — identical to
Task 1, where it also did not fire. The known flake did **not** occur.

⚠️ **A green rerun clears nothing.** This flake is bimodal and fires at the
un-fixed tip too; today's non-occurrence is a single sample, not a refutation. It
is recorded as *did not fire*, never as *fixed*.

One `--- SKIP` is present, unchanged from the baseline shape.

### Gates

```sh
sha256sum internal/listener/manager.go
a8128569457a7afb0a3af694803306b0788a66f49d463f1ab288236514ba2b6f  internal/listener/manager.go
```

`manager.go` is **byte-untouched**, matching the digest the brief pins. Tasks
11/12 own that file. This task wrote **no `.go` file at all** — only this
document — so `gofmt`/`vet` have no new surface, and **no Docker ran**; the
fixture row above is Task 9's recorded result, cited, not re-measured.

### Findings

1. ✅ **The roster diff matches the predicted set exactly**, 7 added / 1 removed,
   with nothing unexpected in either direction. Recorded as a measurement.
2. ⚠️ **`anchored FAIL = 6` is THREE failures.** The anchored matcher counts the
   package-level `FAIL<TAB>…` line and the binary-level trailing `FAIL` as well
   as the three `--- FAIL:` lines. A reader scoring "6 failures" against "7 new
   arms" would reach a wrong conclusion. The per-test verdicts, not the anchored
   count, are the evidence.
3. ⚠️ **`RC` flipped 0 → 1, and that is the INTENDED outcome**, not a regression:
   Tasks 2–6 deliberately landed RED arms. An `RC=0` here would have been the
   finding — it would have meant the RED arms were not running.
4. ⚠️ **Not one of the four pre-fix GREENs could have answered differently.**
   This is the single most load-bearing line in this section: Tasks 15/16 must
   not read any of these four greens as evidence of the repair. Their
   falsifiability lives entirely in the NC roster.
5. ⚠️ **§5.1 (b)(c)(d) have NEVER been scored.** They sit below a `t.Fatalf` and
   the run proves it (exactly one log line, the `(a)` message). Their first-ever
   execution is at Task 11's tip; nothing about them is proven today.
6. ✅ **Nothing in the Task 10 brief was contradicted.** The predicted per-arm
   table, the expected added/removed names, the seven-package selector and the
   panic/flake expectations all held against the actual run.

**Sub-step count: 7.** Five chartered plus two: the `go list` re-derivation of
the selector (the brief asserts SEVEN packages; re-derived rather than trusted)
and the per-arm source reading behind the "could it have answered anything else"
judgements. **It did not cross ~10**, so §6.1's mid-execution split trigger did
not fire.

## Task 11 — EDIT 1 of 2: lift the parse-time `transport_protocol` reject

**The key assertion of this task is what must STAY RED.** This task lifts the
parse-time reject and nothing else; the `raw_buffer` stamp lands at Task 12. If
§5.2 (s1) had turned green here, Task 12's stamp would have zero falsifiable
coverage. **(s1) stayed RED.**

### Step 1 — relocation by LITERAL

Both literals were unique in `internal/listener/manager.go`, inside the sole
enclosing `func parseChainSpec(` at `:969`:

| literal | line | occurrences |
|---|---|---|
| `// transport_protocol: validate against the v3 enum domain. "quic" is` | 991 | 1 |
| `switch tp := fm.GetTransportProtocol(); tp {` | 994 | 1 |

The region was the predicted 3-line comment at `:991-993` plus the 6-line switch
at `:994-999` — **9 lines**, matching the brief exactly.

### Step 2/3 — the edit and its DIFF SHAPE

All 9 lines were replaced by the single line
`spec.TransportProtocol = fm.GetTransportProtocol()` — no case folding, no
trimming, no validation. The reference is case-SENSITIVE and accepts any string.

```
git diff --numstat -- internal/listener/manager.go
1	9	internal/listener/manager.go
```

`1	9` exactly, as predicted. `fmt.` uses in the file went 48 → 47, confirming
the removed `fmt.Errorf` did not orphan the import; `tp` was scoped entirely to
the switch and left no dangling reference. `go build ./...` OK.

### Step 4 — the symbol gate ⚠️ THE BRIEF'S GATE WAS NON-DISCRIMINATING

The brief specified `git grep -c -- 'must be \"tls\"'` (no `-F`) as "the real
gate", and the unescaped spelling as the decoy. **Both readings are 0 at BOTH
tips.** `git grep` defaults to a basic regular expression, in which `\"` reduces
to a bare `"`; the file text contains a literal backslash before the quote, so
the reduced pattern cannot match. The decoy and the "real gate" are the same
non-measurement.

| form | pre-edit | post-edit | discriminates? |
|---|---|---|---|
| `git grep -c -- 'must be \"tls\"'` (brief's gate, no `-F`) | 0 (rc=1) | 0 (rc=1) | ❌ **NO** |
| `git grep -c -- 'must be "tls"'` (brief's decoy, no `-F`) | 0 (rc=1) | 0 (rc=1) | ❌ no |
| **`git grep -c -F -- 'must be \"tls\"'`** | **1** | **0** | ✅ **YES** |
| `git grep -c -F -- 'must be "tls"'` | 0 (rc=1) | 0 (rc=1) | ❌ no |

**The real gate needs `-F`.** A regex-mode grep for a Go string literal
containing `\"` silently self-clears. Recorded as a grep-mechanics finding.

### Step 5 — the arms

| arm | test | predicted | ACTUAL |
|---|---|---|---|
| §5.1 (a)(b)(c)(d) | `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` | GREEN | ✅ PASS |
| §5.3 QUIC | `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | GREEN | ✅ PASS |
| **§5.2 (s1)** | `TestServeConnection_NoListenerFilter_RawBufferChainServes` | **STAY RED** | ✅ **RED** |
| §5.2 (s2) | `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` | GREEN | ✅ PASS |
| §5.2 (s3) | `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` | GREEN | ✅ PASS |

**(s1) verbatim, still red:**

```
    manager_test.go:1841: tag byte = 'B', want 'A': a TCP connection no listener
    filter classified must be stamped transport_protocol=raw_buffer, which makes
    filter_chains[0] eligible. 'B' is default_filter_chain, meaning the detected
    transport protocol stayed empty
```

**§5.1 (b)(c)(d) executed for the FIRST TIME here** and all three passed. This
is a first measurement, not a confirmation. The evidence they ran is that zero
`(a)`/`(b)`/`(c)`/`(d)` diagnostics were emitted while the enclosing test
reported PASS — at the un-fixed tip the `(a)` `t.Fatalf` fired and the other
three were unreachable dead code.

### Step 6 — the sweep, and an unrelated FLAKE

Sweep: `go test -count=1 -v` over the seven packages behind
`./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`.

**The first sweep carried a SECOND failure,**
`TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server` (`internal/boot`),
asserting on initial-fetch-timeout wording. Four measurements established it as
the known SDS dial-budget flake and not a consequence of this edit:

1. edited tip, sweep #1 → (s1) **and** the SDS arm
2. edited tip, SDS arm isolated, 3 × `-count=1` → **3/3 green**
3. **pre-edit control** (file restored to HEAD, identical sweep) → Task 10's
   three failures exactly, **no SDS failure**
4. edited tip, sweep #2 (byte-identical file, sha256 verified) → **(s1) alone**

`parseChainSpec`'s `transport_protocol` field cannot reach SDS fetch timing or
its error wording; the arm is non-deterministic across runs of the same bytes.

Final sweep verdict: **exactly one `--- FAIL:` — (s1)**. Panic gate
`^panic:|DATA RACE|SIGSEGV` = **0**.

### Step 6b — the ROSTER diff (names, not counts)

⚠️ The raw `=== RUN` set read **398** against Task 10's parked **315**, which
looks like 83 added arms. It is not. **Task 10's parked roster is TOP-LEVEL
ONLY** — it contains zero rows with `/`, so every one of the 83 "additions" is a
subtest name that Task 10 never recorded. Compared at the same scope:

```
t11 top-level unique: 315
diff <t10 315> <t11 315>  →  IDENTICAL
```

**Zero names added, zero removed** — correct for a production-only edit that
touches no test file.

| transition | names |
|---|---|
| newly GREEN | `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue`, `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` |
| still RED | `TestServeConnection_NoListenerFilter_RawBufferChainServes` (s1) |
| newly RED | *(none)* |

The newly-green set is exactly the predicted `{§5.1 (a), §5.3 QUIC}`.

### Gates

| gate | result |
|---|---|
| `go build ./...` | OK |
| `go vet ./internal/listener/...` | OK |
| `gofmt -l internal/listener/manager.go` | empty output |
| `golangci-lint` | NOT RUN — toolchain mismatch in this worktree |
| Docker / differential suite | NOT RUN — Tasks 13 and 19 own those |
| panic gate | 0 |
| `--- FAIL:` (failures, not anchored lines) | 1 — (s1), as required |

### Findings

1. ⚠️ **The brief's symbol gate does not discriminate in EITHER spelling.**
   `git grep` without `-F` reduces `\"` to `"` and self-clears against a Go
   string literal. The gate that actually measures is `git grep -c -F`. The
   brief's claim that "the escaped form is the real gate" is refuted.
2. ⚠️ **The brief's "exactly one `--- FAIL:`" prediction did not hold on the
   first sweep** — a second, unrelated failure appeared. It took a pre-edit
   control plus a byte-identical re-run to classify it as the known SDS
   dial-budget flake rather than a regression. A single sweep would have been
   ambiguous in both directions.
3. ⚠️ **A roster compared at MISMATCHED SCOPE reads as 83 added arms.** Task
   10's parked roster is top-level-only; this task's raw capture included
   subtests. The `315 → 398` delta is a measurement artifact, not a change. Any
   later task diffing against these files must equalise scope first.
4. ✅ **The two-edit separation survived.** (s1) stayed RED, so Task 12's stamp
   retains full falsifiable coverage.
5. ✅ **§5.1 (b)(c)(d) passed on their first-ever execution** — a first
   measurement. Their green says the repair's storage is byte-exact and the
   dimension is enforced rather than ignored; it confirms nothing measured
   earlier.

**Sub-step count: 10.** Seven chartered plus three forced by finding 2: the
isolated SDS re-runs, the pre-edit control sweep, and the byte-identical
re-sweep. **It reached ~10 but did not cross it**, so §6.1's mid-execution split
trigger did not fire.

---

## Task 12 — Production edit 2 of 2: THE STAMP. **(s1) TURNS GREEN.**

### The edit, relocated by LITERAL

`/usr/bin/grep -nF -- '// (5) Run chain-match algorithm.' internal/listener/manager.go`
returned **`1359`** at the Task 11 tip — **not** `1367` (anchor A7's figure at
`bd303d87`) and **not** any arithmetic from it. Task 11's `-9/+1` moved it by 8.
The literal was the only usable locator.

Installed immediately ABOVE that comment, inside A5
(`func (rt *listenerRuntime) serveConnection`), as **exactly 3 net added lines**:

```go
	if inputs.TransportProtocol == "" { // ADR-0320: a TCP connection no listener filter classified is raw_buffer on the reference.
		inputs.TransportProtocol = "raw_buffer" // Stamped at the ENTRY of selection, so every path that reaches SelectChain is covered.
	}
```

⚠️ **The PLAN's own quoted form does not fit its own budget.** PLAN §6 Task 12
Step 2 prints a **3-line block comment plus a 3-line `if`** = **6 lines**, then
requires "**3 net added lines** so the total reads `4	9`". The two are
irreconcilable as printed. The only 3-line form that keeps both comment clauses
is **trailing comments on the guard and the assignment**. Folding was mandatory,
not stylistic; `gofmt` accepts it and the file already carries lines of 430 and
394 characters, so the 129-character guard line is well inside house range.

### The cumulative diff shape — the cheapest gate on the production change

| check | command | result |
|---|---|---|
| cumulative numstat | `git diff master --numstat -- internal/listener/manager.go` | **`4	9`** — exactly as §3 requires |

### Why it goes at the entry of selection — and what that does NOT buy

The `continue_on_listener_filters_timeout` fall-through (anchor A6) does **not**
return; it falls out of the enclosing `if` and reaches `SelectChain` (A8) like
any other connection. **One install at the entry of selection therefore covers
every path** — the no-filter path, the filter-ran path and the timeout
fall-through — for three lines.

⚠️ **But per §0.2 the fall-through coverage is STRUCTURALLY UNOBSERVABLE at this
tip.** `tls_inspector` is the only `ListenerFilter` implementation in the tree
and **all five of its return paths write `TransportProtocol`**, so the `== ""`
guard can never fire on the timeout path today. The install is here because it
is **correct and free**, not because an arm proves it, and the committed comment
makes **no claim that the timeout path is covered by test** — it claims only
that the placement covers every path that reaches `SelectChain`, which is a
control-flow fact, not a coverage claim.

### Gates

| gate | result |
|---|---|
| `go build ./...` | OK |
| `go vet ./internal/listener/...` | OK |
| `gofmt -l internal/listener/manager.go` | **empty output** (gated on OUTPUT; gofmt never exits non-zero) |
| `golangci-lint` | NOT RUN — cannot run in this worktree |
| Docker / differential suite | NOT RUN — Tasks 13 and 19 own those |

### The sweep

`go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`

| measure | command | result |
|---|---|---|
| exit code | `${PIPESTATUS[0]}` | **0** |
| failures | `/usr/bin/grep -c -- '--- FAIL:'` | **0** |
| panic gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV'` | **0** |
| per-package | `^ok\|^FAIL` | **7 `ok`, 0 `FAIL`** |

**Neither known flake fired.** `TestSDSEndToEnd_FetchFailure_BootFailsClosed`
PASSED (0.20s) and `TestEnvoyGoBinary_TwoListenerCutover` PASSED (1.27s), so no
isolation re-run was needed and none was performed. ⚠️ Their passing **clears
nothing** about either flake; it only means neither confounded this reading.

### The arm table — every arm by NAME

| arm | test | Task 11 tip | this tip |
|---|---|---|---|
| **§5.2 (s1)** | `TestServeConnection_NoListenerFilter_RawBufferChainServes` | 🔴 **RED** | ⚪ **`--- PASS` (0.00s)** |
| §5.2 (s2) | `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` | GREEN | **PASS** |
| §5.2 (s3) | `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` | GREEN | **PASS** |
| §5.1 (a) | `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` | GREEN | **PASS** |
| §5.3 QUIC | `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | GREEN | **PASS** |
| §5.3 companion | `TestQUICChainSelection_TransportProtocolQUICMatches` | GREEN | **PASS** |
| Task 6 purity pin | `TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` | GREEN | **PASS** |
| Task 6 matched negative | `TestSelectChain_ClassifiedTransportProtocolStillMatches` | GREEN | **PASS** |
| dispatch template | `TestUnifiedDispatchPlaintextChainSelectByDestPort` | GREEN | **PASS** |
| fallback template | `TestUnifiedDispatchDefaultFilterChainFallback` | GREEN | **PASS** |
| dimension pin | `TestSelectChainTransportProtocol` | GREEN | **PASS** |

`TestParseChainSpecRejectsUnknownTransportProtocol` appears **0** times in the
log — the re-pointed arm replaced it at Task 2, as intended.

**The Task 6 purity pin staying GREEN is the load-bearing negative here.** The
stamp went into `manager.go`, not `chainmatch.go`; `SelectChain` itself still
refuses to match a `raw_buffer` chain against an empty input. If the stamp had
been installed one layer down, that arm would have reddened.

### ⚠️ (s2) CARRIES INFORMATION FOR THE FIRST TIME AT THIS TIP

Before this edit, (s2) was green **structurally**: the input was `""`, so the
`tls` chain and the `raw_buffer` chain were **both** ineligible and the
`default_filter_chain` served either way. Green said nothing about `tls`.

At this tip the input really is `"raw_buffer"`. The `tls` chain is now excluded
**by comparison** — `c.TransportProtocol != inputs.TransportProtocol` with two
non-empty, unequal strings — rather than by mutual ineligibility. **This is the
first tip at which (s2)'s green is evidence that the stamp writes
`raw_buffer` and not `tls`.** Its value before Task 12 was zero; recording that
matters because a reader diffing PASS lines across tasks would see no change and
wrongly conclude nothing happened.

### Roster diff, at EQUAL SCOPE

Task 10's parked roster is **top-level only**. Raw `=== RUN` at this tip yields
subtests too, so the scopes were equalised
(`sed 's/.*=== RUN[[:space:]]*//' | cut -d/ -f1 | sort -u`) before diffing.

| measure | value |
|---|---|
| Task 10 parked roster | **315** |
| Task 12 roster, equal scope | **315** |
| added | **0** |
| removed | **0** |
| `diff` rc | **0** |

A production-only edit adds no arm, and none was added.

### Findings

1. 🔴 **The PLAN's Step 2 code block is not installable as printed.** Six lines
   quoted against a three-line budget. The budget (`4	9`) is the gate three
   separate PLAN sections assert against, so the budget wins and the comment
   folds. Any later task re-deriving the stamp from §6's quoted block will
   produce `7	9` and fail the shape gate.
2. ✅ **The two-edit separation paid off exactly as designed.** (s1) was RED at
   the Task 11 tip and GREEN here, with the only intervening change being these
   three lines. That is full falsifiable coverage of edit 2, which a single
   combined commit could not have produced.
3. ⚠️ **(s2)'s green changed meaning without changing text** — structural before
   Task 12, informative after. A PASS line is not a constant-value measurement.
4. ✅ **Neither known flake fired**, so this sweep needed no isolation runs.
   Recorded as non-occurrence, not as clearance.
5. ⚠️ **Anchor A7's line number was wrong by 8** at this tip, exactly as §3
   warned. The literal relocation was not ceremony.

**Sub-step count: 8.** Six chartered plus two forced by the parked roster not
being where the brief implied — Task 10's roster path is recorded in
`PROGRESS.md` only as a Task 1 scratch path, so locating
`…/scratchpad/t10/roster-task10.txt` took two extra probes. **It did not cross
~10**, so no mid-execution split trigger fired.

---

## Task 13 — ALL ARMS GREEN, plus the build, race and lint gates

**This task changed no code.** Both production edits were already in at
`169d9e91`; `git diff master --numstat -- internal/listener/manager.go` reads
**`4	9`**, unchanged from Task 12. The only file this task writes is this one.

### Step 0 — resolve the selector BEFORE trusting the sweep

A nonexistent package selector prints `FAIL … [setup failed]` and exits 1, which
reads exactly like a real failure, so the selector was resolved first:

```
$ go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...
github.com/pgdad/envoy-go/cmd/envoy-go
github.com/pgdad/envoy-go/internal/admin
github.com/pgdad/envoy-go/internal/boot
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
github.com/pgdad/envoy-go/validate
RC=0
```

**SEVEN packages**, as expected. Every one of them reports below.

### Step 1 — the full reverse-dependency sweep

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > $SCRATCH/t13.txt 2>&1
# RC taken from ${PIPESTATUS[0]}
```

| measure | command | result |
|---|---|---|
| exit code | `${PIPESTATUS[0]}` | **0** |
| log size | `wc -l` | **857** lines |
| `=== RUN` | `/usr/bin/grep -c -- '=== RUN'` | **398** (top-level **and** subtests) |
| **failures** | `/usr/bin/grep -c -- '--- FAIL:'` | **0** |
| anchored FAIL **LINES** | `/usr/bin/grep -cE '^(FAIL\|--- FAIL)\|^ *--- FAIL'` | **0 LINES** |
| panic gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV'` | **0** |
| `--- PASS:` | | **397** |
| `--- SKIP:` | | **1** |

⚠️ **The anchored figure is LINES, not failures** (Task 10's 6 lines were 3
failures). Here both read 0, so the distinction costs nothing — but it is
recorded in the right units so a later reader does not inherit a category error.

Per-package, all seven `ok`, zero `FAIL`:

```
ok  	github.com/pgdad/envoy-go/cmd/envoy-go	11.180s
ok  	github.com/pgdad/envoy-go/internal/admin	1.479s
ok  	github.com/pgdad/envoy-go/internal/boot	0.252s
ok  	github.com/pgdad/envoy-go/internal/listener	3.270s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	0.043s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
ok  	github.com/pgdad/envoy-go/validate	0.245s
```

The single `SKIP` is **`TestHandleListeners_IPv6BindAddrPassthrough`**, a
pre-existing environment-conditional skip unrelated to this row.

**Roster at equal scope** (`sed 's/.*=== RUN[[:space:]]*//' | cut -d/ -f1 | sort -u | wc -l`)
= **315**, identical to Task 10's parked roster and to Task 12. **A run-only
task adds no arm, and none was added.**

### The arm table at this tip

| arm | test | result |
|---|---|---|
| §5.2 (s1) | `TestServeConnection_NoListenerFilter_RawBufferChainServes` | **PASS** (0.00s) |
| §5.2 (s2) | `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` | **PASS** (0.00s) |
| §5.2 (s3) | `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` | **PASS** (0.00s) |
| §5.1 (a) | `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` | **PASS** (0.00s) |
| §5.3 QUIC | `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | **PASS** (0.00s) |
| §5.3 companion | `TestQUICChainSelection_TransportProtocolQUICMatches` | **PASS** (0.00s) |
| Task 6 purity pin | `TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` | **PASS** (0.00s) |
| Task 6 matched negative | `TestSelectChain_ClassifiedTransportProtocolStillMatches` | **PASS** (0.00s) |
| dimension pin | `TestSelectChainTransportProtocol` | **PASS** (0.00s) |

`TestParseChainSpecRejectsUnknownTransportProtocol` appears **0** times — the
re-pointed arm replaced it at Task 2, still true at this tip.

### Step 2 — `-race` on the FULL `internal/listener` package

⚠️ **`-race` on the differential suite is VACUOUS** — the subject there is an
unraced subprocess. It was run where the code actually is:

```
$ go test -race -count=1 ./internal/listener/...
ok  	github.com/pgdad/envoy-go/internal/listener	4.482s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter	1.049s
ok  	github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector	1.012s
RC=0
```

| gate over the `-race` log | result |
|---|---|
| `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| `--- FAIL:` | **0** |

### Step 3 — the touched set, DERIVED not guessed

`git diff master --numstat`, `.go` files only:

```
34	0	internal/listener/listenerfilter/chainmatch_test.go
4	9	internal/listener/manager.go
340	11	internal/listener/manager_test.go
89	0	internal/listener/quic_test.go
1	0	test/differential/runner_test.go
733	0	test/fixtures/0123-listener-transport-protocol/driver/driver.go
```

⇒ **six `.go` files in FOUR packages**: `internal/listener`,
`internal/listener/listenerfilter`, `test/differential`, and
`test/fixtures/0123-listener-transport-protocol/driver`. (The row also touches
three non-`.go` files — this `PROGRESS.md`, the fixture `README.md` and
`expectations.yaml` — which no Go linter reads.)

| gate | command | result |
|---|---|---|
| build | `go build ./...` | **RC=0** |
| vet | `go vet ./internal/listener/...` | **RC=0**, no output |
| vet | `go vet ./internal/listener/listenerfilter/...` | **RC=0**, no output |
| vet | `go vet ./test/differential/...` | **RC=0**, no output |
| vet | `go vet ./test/fixtures/0123-listener-transport-protocol/driver/...` | **RC=0**, no output |
| gofmt | `gofmt -l <the six files>` | **EMPTY OUTPUT** — gated on OUTPUT, not on rc (gofmt never exits non-zero for formatting) |

### Step 4 — `go.mod` / `go.sum` untouched

```
$ go mod tidy -diff
RC=0                            # and EMPTY output

$ git diff master -- go.mod go.sum
                                # EMPTY

$ git diff master --numstat -- go.mod go.sum
                                # EMPTY — not even a zero-delta row
```

⚠️ **An import LINE is not a go.mod MODULE**, and this row *does* add a new
package — `test/fixtures/0123-listener-transport-protocol/driver`. It was
checked, not assumed. Its own import block is stdlib plus two in-repo packages:

```go
	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
```

Transitively the new package reaches six external modules
(`github.com/quic-go/quic-go`, `github.com/quic-go/qpack`, `golang.org/x/crypto`,
`golang.org/x/net`, `golang.org/x/sys`, `golang.org/x/text`) **via those two
in-repo packages** — but every one was already required, which is exactly what
the empty `go mod tidy -diff` proves. **No module was introduced, and no
require line flipped indirect→direct.**

### Step 5 — the known flakes: NON-OCCURRENCE, which is not clearance

| flake | package | this run |
|---|---|---|
| `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server…` | `internal/boot` | **PASS** (0.20s) — subtest `silent_SDS_server: validation context fetch times out, boot fails` PASSED (0.20s); sibling `unreachable_SDS_server…` PASSED (0.00s) |
| `TestEnvoyGoBinary_TwoListenerCutover` | `cmd/envoy-go` | **PASS** (1.41s) |

⚠️ **Neither fired. A green rerun CLEARS NOTHING.** The SDS arm fired once at
Task 11 and was classified there, by a pre-edit control sweep, as the known SDS
dial-budget flake; the cutover test's failure mode is an ephemeral-range bind
collision. Both remain open. **This entry records non-occurrence, and the
classification is inherited from Task 11's control — not re-established by a
green.** No isolation re-run was performed, because none was warranted.

---

## 🔴 `golangci-lint` — A DEPARTURE, NAMED. NOT A PASS.

**`golangci-lint` CANNOT RUN IN THIS ENVIRONMENT, on any package.** The house
posture is *name departures, do not claim compliance*, so this section records
the measurement rather than the absence of one.

```
$ type golangci-lint
golangci-lint is /home/esa/go/bin/golangci-lint
$ golangci-lint --version
golangci-lint has version v1.64.8 built with go1.26.2 …
$ go version
go version go1.27.1 linux/amd64
```

The linter is built against **go1.26.2**; the toolchain here is **go1.27.1**,
whose export-data format the linter's bundled type-checker cannot decode. It
therefore fails on a **stdlib** import, i.e. on **every** package:

```
$ golangci-lint run ./internal/listener/...          # TOUCHED, in wt-phase-98-impl
internal/listener/listenerfilter/registry.go:6:2: could not import sync/atomic (-: could not load export data: internal error in importing "sync/atomic" (cannot decode "sync/atomic", export data version 4 is greater than maximum supported version 2); please report an issue) (typecheck)
	"sync/atomic"
	^
RC=1
```

**Four controls, re-measured by this task rather than inherited from the brief:**

| # | scope | touched by this row? | result |
|---|---|---|---|
| — | `./internal/listener/...` in `wt-phase-98-impl` | **yes** | rc=**1**, `sync/atomic` typecheck |
| A | `./internal/stats/...` in `wt-phase-98-impl` | no | rc=**1**, same error at `internal/stats/counter.go:5:2` and `internal/stats/dynamic/dynamic.go:15:2` |
| B | `./internal/httpclient/...` in `wt-phase-98-impl` | no | rc=**1**, same error at `internal/httpclient/httpclient_test.go:39:2` |
| C | `./internal/listener/...` in the **UNTOUCHED `master` worktree** `/home/esa/git/envoy-go` (at `9c7bee0b`) | n/a | rc=**1**, **byte-identical** error at `internal/listener/listenerfilter/registry.go:6:2` |

Control **C** is the load-bearing one: the *same command* on the *same package
path* in a tree that contains **none** of this row's edits fails **identically**.

⇒ **PRE-EXISTING AND ENVIRONMENTAL. Not this row's regression — and not this
row's compliance either.** The correct reading of this section is *"the project's
lint gate was not run for phase 98,"* not *"phase 98 passed lint."*

### The substitutes, LABELLED AS SUBSTITUTES

`go vet` (4 packages, all rc=0) and `gofmt -l` (6 files, empty output) are run
**in place of** `golangci-lint`, and they do **not** cover its linter set —
notably `misspell`, `revive`, `errcheck`, `unused` and `staticcheck` are
**unrun** for this row. Only `misspell` is discharged by hand below; the others
are simply **not measured**, and this entry says so rather than implying
otherwise.

### The by-hand `misspell` sweep — and its POSITIVE CONTROL

`golangci-lint` runs `misspell` in locale **US**, so British spellings in `.go`
comments would normally FAIL the gate. ⚠️ **A gate that reads 0 has not been
shown to work**, so the matcher was proven to fire before its zero was trusted.

**The word list was DERIVED, not invented.** Rather than trust a hand-written
list, the authoritative keys were extracted from the linter's own dictionary
source, present in this machine's module cache:

```sh
D=/home/esa/go/pkg/mod/github.com/golangci/misspell@v0.6.0
sed -n '28094,29713p' $D/words.go \
  | sed -nE 's/^[[:space:]]*"([a-zA-Z]+)",[[:space:]]*"[a-zA-Z]+",?[[:space:]]*$/\1/p' \
  | sort -u > $SCRATCH/us-locale-keys.txt
```

`DictAmerican` is misspell's UK→US rule set — exactly what locale US enforces.
**1619** British-spelling keys extracted. Sanity-checked both ways:

- **present** (must be, and are): `behaviour`, `colour`, `cancelled`, `labelled`,
  `centre`, `licence`, `defence`, `catalogue`, `dialogue`, `organised`,
  `normalised`, `analyse`, `honour`
- **absent** (must be, and are): `behavior`, `color`, and — see the finding
  below — `grey`

⚠️ **A first extraction was WRONG and was caught by its own output.** The `sed`
range swept up the array's closing `}` as a "key"; `grep -F -w` then matched `}`
against essentially every line of Go source, producing a flood of bogus hits.
The fix was to constrain the key pattern to `[a-zA-Z]+`. **A sweep that
"finds everything" is as broken as one that finds nothing** — it was the
implausible volume, not a passing gate, that exposed it.

**Positive control** — a scratch file (`$SCRATCH/misspell-control.go`, never
committed) carrying deliberate British spellings:

```go
package control

// This comment describes the behaviour of the colour analyser.
// We cancelled the labelled centre and checked the licence defence.
// The dialogue catalogue was organised and normalised.
const x = 1 // grey
```

```
$ /usr/bin/grep -oiwFf $SCRATCH/us-locale-keys.txt $SCRATCH/misspell-control.go | sort -u
behaviour cancelled catalogue centre colour defence dialogue labelled licence normalised organised
control hit LINES: 3
```

**11 distinct British spellings caught across 3 of the 4 comment lines. THE
MATCHER FIRES.** Only then is its zero worth anything.

**The audit** — the same matcher over the six touched `.go` files:

```
$ /usr/bin/grep -niwFf $SCRATCH/us-locale-keys.txt \
    internal/listener/listenerfilter/chainmatch_test.go \
    internal/listener/manager.go \
    internal/listener/manager_test.go \
    internal/listener/quic_test.go \
    test/differential/runner_test.go \
    test/fixtures/0123-listener-transport-protocol/driver/driver.go
# (no output)
files with >0 hits: []
```

**ZERO hits, against a live matcher, over all six files.** The `misspell`
concern is discharged. Note the scope: this audits the **whole file**, not only
comments, so it is strictly broader than the comment-only concern.

---

### Findings

1. 🔴 **`golangci-lint` is unrunnable here, and this entry NAMES that rather than
   claiming a pass.** Re-measured by this task with four controls, including the
   untouched `master` worktree at `9c7bee0b` failing **byte-identically** on the
   same package path. Environmental, pre-existing, and **not** discharged by the
   substitutes. `revive`, `errcheck`, `unused` and `staticcheck` are **unrun**
   for phase 98 and are recorded as unrun.
2. ⚠️ **`grey` is NOT in misspell's US locale.** The obvious hand-written British
   word list includes it; `DictAmerican` does not. The hand list was therefore
   both over- and under-inclusive, which is exactly why the dictionary was
   derived from the linter's own source instead of written from memory. **A
   plausible word list is not the gate's word list.**
3. ⚠️ **A broad stem-based pre-pass produced FOUR FALSE POSITIVES** —
   `realistic` ×2 and `realism` (this row's own additions, matching the stem
   `realis`) and `cancellable` (matching `cancell`, and **not** added by this
   row). All four were confirmed **absent** from `DictAmerican`. The
   exact-dictionary audit reads **0**. **Stem matching is not the gate's
   matching**, and the difference here is the whole result.
4. ✅ **The new fixture driver package introduced no module.** It reaches six
   external modules transitively through `test/differential/fixture` and
   `test/helpers`, but `go mod tidy -diff` is empty and `git diff master --
   go.mod go.sum` is empty. The hazard was checked, not waved off — and the
   transitive reach is exactly why waving it off would have been wrong.
5. ✅ **Roster unchanged at 315 and failures 0 across seven packages**, with the
   anchored figure recorded in **LINES** (0) separately from **failures** (0),
   so no later reader inherits Task 10's units confusion.
6. ⚠️ **Neither known flake fired — recorded as non-occurrence, never as
   clearance.** The SDS dial-budget flake and the cutover bind collision both
   remain open; a green run is not evidence about either.
7. ✅ **`-race` was run where the code is** (`./internal/listener/...`, rc=0,
   race gate 0), not on the differential suite, where the subject is an unraced
   subprocess and the flag would have been vacuous.

**Nothing in the Task 13 brief was refuted.** Every figure it supplied —
`4	9` on `manager.go`, seven packages from the selector, `golangci-lint`
v1.64.8-vs-go1.27.1 and its three controls, both flake identities — was
re-measured and held. The only corrections are to the *misspell word list the
brief suggested* (finding 2: `grey` is not in the gate's dictionary) and to the
brief's parenthetical that the corpus was positive-controlled "4/4" — the
derived dictionary fires on **3** of the control file's 4 lines, because the
4th line's only British word is `grey`, which misspell's US locale does not
flag. That is a property of the linter, not a hole in the sweep.

**Sub-step count: 8.** Six chartered (resolve, sweep, race, vet/gofmt, go.mod,
flakes) plus two forced: the `golangci-lint` re-measurement with the
master-worktree control, and the misspell dictionary derivation — which itself
needed one retry after the stray `}` key. **It did not cross ~10**, so no
mid-execution split trigger fired.

---

## Task 14 — the §6 occurrence-set reconciliation: SIX inherited sites, TWO grown, `chainmatch.go` under a DOUBLE gate

**Step 1 — the roster was INHERITED, not re-derived.** `SPEC.md` §14 item 1's instruction
(*"re-run §6's matchers — the union may grow"*) was **not obeyed as a derivation**, per `PLAN.md` §0.10:
all six matchers are blind to `internal/listener/listenerfilter/types.go:44-46`, one of the two K2 **code**
sites, because the claim spans a line break and `git grep` is line-based. The tables of `SPEC.md` §6.1/§6.2
are the roster; the matchers were run **only for growth** (step 6).

### The six inherited sites, each relocated by LITERAL (line numbers had drifted)

| # | file | relocating literal | what changed |
|---|---|---|---|
| 1 | `quic_test.go` (was `:690-694`) | `` PARSE PRECONDITION: `parseChainSpec`'s enum gate accepts exactly `` | precondition KEPT (`"quic"` still parses ⇒ both arms are genuine runtime arms); `exactly` + the four-value set DROPPED; the stale `manager.go:985-991` anchor DROPPED, replaced by a symbol reference (`parseChainSpec`) and `ADR-0320`. **5 lines → 5 lines.** |
| 2 | `quic_test.go` (was `:702`) | `parseChainSpec's enum gate must accept %q` | → `parseChainSpec must accept %q verbatim (there is no enum gate after ADR-0320)` |
| 3 | `quic_test.go` (was `:768-771`) | `` // PARSE PRECONDITION: `"tls"` is in parseChainSpec's accepted enum domain `` | same treatment as (1). ⚠️ `SPEC.md`'s range `:768-770` **is short by one** — confirmed: the block ran to `// listener, not by reading the switch.`, which is line **771**. **4 lines → 4 lines.** |
| 4 | `quic_test.go` (was `:779`) | same literal as (2) — the two `t.Fatalf` texts are byte-identical in the falsified clause, replaced together (`count=2`, asserted) | same as (2) |
| 5 | `listenerfilter/types.go:44-46` | `or "" if no listener` (the §0.10 positive control's own literal) | says **BOTH**: the `""` is true *as the pipeline returns*, and `serveConnection` is named as the **SECOND writer** that stamps `raw_buffer` at the entry of chain selection (ADR-0320). **3 lines → 3 lines** (see finding 2). |
| 6 | `listenerfilter/chainmatch.go:30-32` | `// TransportProtocol: "" means unspecified; "tls" or "raw_buffer" means` | the controller's pre-proven replacement, taken **verbatim**. Both falsified halves die: the closed set `{"tls","raw_buffer"}`, and *"the listener-filter pipeline"* as the only writer. **3 lines → 3 lines.** |

Both comments (1) and (3) cited `manager.go:985-991` for the enum switch. Re-confirmed: after Task 11 that
switch **does not exist**; `manager.go:991` is now `spec.TransportProtocol = fm.GetTransportProtocol()`, a
verbatim copy-through. Both citations were **removed**, not re-pointed to a number — a symbol reference
cannot drift.

### Step 5 — THE DOUBLE GATE on `chainmatch.go`, both arms RUN

**Gate A — line-count neutrality (`PLAN.md` §0.12):**

```
$ git diff --numstat -- internal/listener/listenerfilter/chainmatch.go
3	3	internal/listener/listenerfilter/chainmatch.go
```

**Gate B — comment-only (`PLAN.md` §7.2), run INLINE, never committed:**

```
$ git diff --no-color --unified=0 -- internal/listener/listenerfilter/chainmatch.go | awk '...'
GATE: chainmatch.go comment-only -- inspected 6 changed line(s), 0 violation(s)
gate rc=0
```

`inspected 6 > 0` — the diff was **not empty**, so the green is not the vacuous `inspected 0` shape that
control A of §7.3 exists to expose.

**⚠️ Gate B was NEGATIVE-CONTROLLED AT THIS TIP, not merely cited.** A compiling code edit was injected into
`chainmatch.go` **on top of** the comment edit (the stacked shape of §7.3 control D) and the gate was re-run:

```
$ # with `var ErrNoChainMatched = ...` split into a commented line + `var _ = ErrNoChainMatched`
$ git diff --numstat -- internal/listener/listenerfilter/chainmatch.go
5	4	internal/listener/listenerfilter/chainmatch.go
VIOLATION (non-comment REMOVED line): -var ErrNoChainMatched = errors.New("no filter_chain matches connection")
VIOLATION (non-comment ADDED line): +var ErrNoChainMatched = errors.New("no filter_chain matches connection") //nc
VIOLATION (non-comment ADDED line): +var _ = ErrNoChainMatched
GATE: chainmatch.go comment-only -- inspected 9 changed line(s), 3 violation(s)
NC gate rc=1
```

**Both gates fired, and they fired INDEPENDENTLY**: gate A caught the line-count change (`5	4`, not `3	3`)
and gate B caught the code lines. Gate B **did not short-circuit** — it inspected 9 lines, passed the six
legal comment lines, and still failed on the three code lines. The file was then restored from a scratch
copy and both gates re-read green (`3	3`, `inspected 6 … 0 violation(s)`, rc=0). No `.sh` file was created:
the repo carries **zero** tracked `.sh` files and this record is the artefact.

**The 15 → 18 `chainmatch.go:<line>` cites are all still valid.** The comment still occupies lines
**30-32**, the file is still **334** lines, and the minimum cited line is still **87**
(`SelectChain`'s pass-1 empty-eligible branch, cited as `:87-92`). Full census, post-edit:
`:87`×2 · `:119`×2 · `:125`×1 · `:128`×8 · `:131`×4 · `:293`×1 = **18**.

### Step 6 — the GROWTH re-run

**M1 was run in BOTH spellings, and the `-F` trap was re-measured as a control.**

| matcher | result |
|---|---|
| M1a `git grep -nF -- 'must be "tls"'` | **25** hits, rc=0 |
| M1b `git grep -nF -- 'must be \"tls\"'` (**escaped, `-F` REQUIRED**) | **11** hits, rc=0 — incl. `PLAN.md:656`'s record of the now-deleted `manager.go:998` |
| **M1b control, `-F` REMOVED** | output **byte-identical to M1a** — BRE reduced `\"` to `"`, so the un-`-F`'d form is **not M1b at all**. The brief's warning HELD. |
| M2 `enum (domain\|gate)\|four-member\|4-member` over `internal/ test/ cmd/` | 7 hits |
| M3 the four-value set, both quotings, over `internal/ test/ cmd/` | 4 hits |
| M4 / M5a / M5b / M5c | no un-dispositioned code hit beyond those below |
| M6 bare `raw_buffer` over `BEHAVIOR_CONTRACT.md` + `DECISIONS.md` | `BEHAVIOR_CONTRACT.md` **0**, rc=1 (re-confirms §0.11) · `DECISIONS.md` **8** |

⚠️ **The M1 figures are measured at `HEAD`, deliberately — this record SELF-INCREMENTS both of them.** Run
against the working tree they read **26** and **12**, because the Task 14 section quotes each spelling once.
A matcher census taken at the tip that includes its own write-up is not a census of the tree.

**Residuals deliberately LEFT, each with its reason** (an unexplained residual reads as a miss):

- `DECISIONS.md` **ADR-0279** `:16736`/`:16740` — accurate about **phase 61.1**, which lifted `"quic"` only.
- `DECISIONS.md` **ADR-0319** `:19085`/`:19210` and **§Context ¶8** `:19079` (*"the only writer of that input
  field"*) — accurate about **phase 97**; ADR-0320 names the second writer. Task 17 owns ADR-0320.
- `BEHAVIOR_CONTRACT.md:5881` — *"the `transport_protocol: "quic"` value is accepted"* — **still true, a
  subset** of the new free-form rule.
- Every hit under `docs/envoy-go/phases/**` (61-*, 97-*, and this phase's own BRAINSTORM/SPEC/PLAN/PROGRESS)
  — the **governing documents of closed stages**, and this stage's own record. History, not claims.
- `ROADMAP.md:160` and `next-prompt.txt:254` — stage records quoting the reject **as the divergence found**.
  Row 98 is Task 18's; method note 79 is about matcher escaping and stays true.
- `test/fixtures/0123-listener-transport-protocol/driver/driver.go:162` and `README.md:152` — **explicitly
  tense-anchored** (*"At the un-fixed tip …"*, *"Before the reject is lifted …"*). They narrate why the
  fixture's boot-reject probe was red, which is the fixture's whole design. Not falsified, and Task 7/8's
  files, not this task's.
- `internal/listener/manager_test.go:1690` (*"there is no enum domain"*) and `quic_test.go:832` (*"a
  kind-scoped restoration of the enum gate"*) — both written **post-repair** by Tasks 2 and 5 and **already
  correct**.

**🔴 TWO GROWN SITES WERE FOUND AND REPAIRED — see findings 1 and 3.**

### Step 7 — build, vet, gofmt, sweep

| gate | command | result |
|---|---|---|
| gofmt | `gofmt -l` on all four touched files | **empty output**, rc=0 |
| build | `go build ./internal/...` | rc=**0** |
| vet | `go vet ./internal/listener/...` | rc=**0** |
| sweep | `go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...` | rc=**0** |
| `=== RUN` | `/usr/bin/grep -c` | **398** — unchanged from Task 13 |
| **failures** | `/usr/bin/grep -c -- '--- FAIL:'` | **0** |
| anchored FAIL **LINES** | `/usr/bin/grep -cE '^(FAIL\|--- FAIL)\|^ *--- FAIL'` | **0 LINES** |
| panic/race gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV'` | **0** |
| `ok ` packages | | **7** |
| `manager.go` | `git diff --numstat 6065b534~1 -- internal/listener/manager.go` | **`4	9`** — unchanged, and `git diff HEAD -- internal/listener/manager.go` is **empty**: the file was never opened for writing. |

### 🔴 THREE FINDINGS — TWO OF THEM CONTRADICT THE BRIEF

**🔴 FINDING 1 — THE §6 ROSTER IS SIX SITES, BUT THE TREE NOW CARRIES EIGHT. TWO FALSIFIED CLAIMS WERE BORN
AFTER `SPEC.md` WAS WRITTEN, AND THE GROWTH RE-RUN FOUND THEM.** The brief said *"THE SIX SITES"*. That is
right about the inherited roster and **incomplete about the tree**, because Tasks 2-6 added test prose that
§6 could not have censused. Two of those new sites state the pre-repair behaviour in the **present tense**:

1. `internal/listener/manager_test.go` — the (s1) arm's doc comment read *"envoy-go **leaves**
   `inputs.TransportProtocol` empty, the `c.TransportProtocol != inputs.TransportProtocol` branch in
   listenerfilter.matches **rejects** the chain, and the connection **falls through** to
   `default_filter_chain`."* After Task 12 **every clause is false** — and the comment sat directly above a
   test named `TestServeConnection_NoListenerFilter_RawBufferChainServes` that is now **GREEN**, i.e. the
   prose contradicted its own assertion. Repaired to past tense, naming ADR-0320's stamp as what turns the
   arm green. **4 lines → 4 lines.**
2. `internal/listener/quic_test.go` (the bogus-value arm, was `:851-858`) — *"THIS ARM **IS** RED AT THE
   UN-FIXED TIP … `parseChainSpec`'s enum gate **accepts** exactly {…} … so NewManager **refuses** the
   listener"*. The block is tense-governed by its own heading, so it is defensible in isolation — but the
   **same file** now says *"there is no enum gate"* at two other sites **because of this task's own edits**,
   so leaving it would have made `quic_test.go` self-contradictory. Repaired to past tense.
   **8 lines → 8 lines.**

`manager_test.go` is on `SPEC.md` §6.1's edit roster (**RE-POINTED**) and on **neither** byte-untouched
roster, so this widens Task 14's **file** count from 3 to 4 while staying inside the phase's declared edit
set. **Declared here rather than done silently.** The fixture's two hits were deliberately NOT repaired:
they carry explicit *"at the un-fixed tip"* markers and belong to Tasks 7/8.

**🔴 FINDING 2 — §0.12's LINE-COUNT CONSTRAINT HAS A SECOND INSTANCE THE BRIEF DID NOT NAME:
`listenerfilter/types.go`.** The brief applied the neutrality rule to `chainmatch.go` alone. Censused at this
tip, `listenerfilter/types.go:<line>` is cited **11** times, and **exactly one of those cites lives in a
live, normative document**: `docs/envoy-go/DECISIONS.md:13930` cites `listenerfilter/types.go:91-96` for the
two-step factory pattern. The mandated `types.go` edit sits at `:44-46`, **above** it. A non-neutral edit
there would have staled a **DECISIONS.md** anchor in the commit that claims to reconcile the prose — the
identical failure shape §0.12 was written to prevent, one file over. The edit was therefore constrained to
**`3	3`** as well, at the cost of three wider comment lines (≈105 cols; precedent exists — nine comment
lines in `internal/listener/**` already exceed 90 cols, the longest at 430). The other ten cites are all
under `docs/envoy-go/phases/**` and are history. **For the same reason both `quic_test.go` edits and the
`manager_test.go` edit were made line-count-neutral:** the whole task reads `3	3` · `3	3` · `4	4` · `16	16`
— **zero net line movement in any file**, so no `<file>:<line>` citation anywhere in the tree drifts.

**⚠️ FINDING 3 — THE `chainmatch.go` CITE COUNT IS **18**, NOT 15. THE BRIEF'S FIGURE WAS TRUE AT THE PLAN
TIP AND IS STALE AT THE IMPL TIP.** `PLAN.md` §0.12 measured 15 (`:128`×5). Tasks 3-6 added three more
`:128` cites, so the census is now `:87`×2 · `:119`×2 · `:125`×1 · `:128`×**8** · `:131`×4 · `:293`×1 =
**18**. **The constraint is unaffected and in fact stronger** — the minimum cited line is still **87**, all
18 still point below the comment, and all 18 are still valid after a `3	3` edit. Flagged because
`reference_banked_candidate_costs_rot_in_every_field` is exactly this shape: a banked count rots even when
the conclusion it supports does not. **Do not re-quote "15".**

**Everything else in the brief HELD and was re-measured, not taken on trust:** the §0.10 trap is real (all
six matchers miss `types.go:44-46`; the positive control `git grep -nF -- 'no listener' -- …/types.go`
returns `:45`, rc=0); `SPEC.md`'s `:768-770` **is** short by one; the un-`-F`'d M1b **is** a self-clearing
non-gate; `manager.go:985-991` names **neither** the old comment nor the old switch and both are now moot;
the pre-proven `chainmatch.go` replacement reproduced **exactly** `3	3` and `inspected 6, 0 violations`; and
`manager.go` still reads `4	9`.

**Sub-step count: 9.** Seven chartered (inherit, four `quic_test.go` sites, `types.go`, `chainmatch.go`,
double gate, growth re-run, sweep+commit) plus two forced: the live negative control on gate B, and the
`types.go` cite census that produced finding 2. **It did not cross ~10**, so no mid-execution split trigger
fired.

---

## Task 15 — NC roster rows 1, 2 and 3, scored PER PROPERTY

**Nothing in this task lands except this record.** Every mutation was applied,
built, swept, and reverted; the two production files are byte-identical before
and after.

### Step 0 — the baseline, re-measured at this tip (not inherited)

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > $SCRATCH/base.txt 2>&1
```

| measure | result |
|---|---|
| exit code | **0** |
| `=== RUN` | **398** |
| `--- FAIL:` (FAILURES, not lines) | **0** |
| panic gate `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| packages | **7 `ok`, 0 `FAIL`** |

Digests **BEFORE** (and, in §Step 6, after):

```
2c940338275ecbefa8ef1dcfd0e58f4227a13fdb1a75832a24fb02f9443641c2  internal/listener/manager.go
8dfdc71d92ece1b3fd54e280a0a8bc352efd9eca6bd7cc24d40ad92afe1a6186  internal/listener/listenerfilter/chainmatch.go
```

### Step 1 — NEUTRALISE, and prove EXECUTABLE

Each mutation below **compiles** (`go build ./...` run before every sweep; row 3
also `go vet ./internal/listener/...`). Executability is not asserted from the
shape of the edit — **it is proven by the arm that reddened**, named per row.
Row 3 is deliberately written as a *neutralised branch* rather than a deletion:
the `if` is still entered and its body still runs, it simply no longer rejects.
A deleted clause has no line to call executable; a neutralised one does.

### Step 2 — ROW 1: restore the enum `switch`

**Mechanism, named BEFORE the run.** `parseChainSpec` returns an error for any
`transport_protocol` outside `{"", "tls", "raw_buffer", "quic"}`. §5.1's `l_tp`
carries `"sctp"`, so `NewManager` errors and **(a)**'s `t.Fatalf` fires. §5.3's
QUIC arm carries `"totally_bogus_value"`, so QUIC listener construction fails at
boot. Fixture `l_bogus` carries the same string, so the SUBJECT boot-rejects.

```
git diff --numstat  ->  7	1	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **3**, panic gate **0**.

**Verbatim, with the property label:**

```
=== RUN   TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
    manager_test.go:1727: (a) NewManager must ACCEPT transport_protocol "sctp", got error: listener: "l_tp": filter_chains[0]: transport_protocol "sctp" must be "tls", "raw_buffer", "quic", or empty
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
```

```
=== RUN   TestQUICChainSelection_TransportProtocolBogusDoesNotMatch
    quic_test.go:866: NewManager(quic, transport_protocol="totally_bogus_value" filter_chains[0] + QUIC-TLS default slot) BOOT-REJECTED: listener: "quic_listener_chains": filter_chains[0]: transport_protocol "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty — an unknown transport_protocol is a NON-MATCHING VALUE, not a config error, so parseChainSpec must accept it and let chain selection decide
--- FAIL: TestQUICChainSelection_TransportProtocolBogusDoesNotMatch (0.00s)
```

**Only (a) fired, and that is the DESIGNED shape, not under-coverage:** (a) is a
`t.Fatalf`, so (b)/(c)/(d) are unreachable beneath it (method note — `t.Fatalf`
makes later assertions dead code). Row 1 is scored on (a) alone by construction.

**Must stay green — CONFIRMED, not assumed.** `--- PASS:` observed under the
mutation for `TestServeConnection_NoListenerFilter_RawBufferChainServes` (s1),
`…_TLSChainDoesNotServe` (s2), `…TLSInspector_ClassifiedInputNotOverwritten`
(s3), and `TestParseChainSpec_QUICTransportProtocolAccepted`.

**🔴 THE THIRD FAILURE IS THE KNOWN FLAKE, AND IT IS CLEARED BY MECHANISM, NOT
BY A RERUN.**

```
=== RUN   TestEnvoyGoBinary_TwoListenerCutover
2026/09/20 22:44:34 listener start: listener: "l_tcp_b": bind 127.0.0.1:38575: listen tcp 127.0.0.1:38575: bind: address already in use
--- FAIL: TestEnvoyGoBinary_TwoListenerCutover (1.38s)
```

`38575` sits inside the ephemeral range (32768-60999) — the documented probe-band
collision. **A green rerun would clear nothing**, so the clearance is structural:
`git grep -c 'transport_protocol\|TransportProtocol' -- cmd/envoy-go/` returns
**nothing, rc=1** — **ZERO** occurrences in the entire package. Row 1's mutation
is a parse-time reject on exactly that field, so **it has no path to this
failure.** Classified: pre-existing flake, not collateral.

**Verdict: row 1 reddened EXACTLY what §8 predicts — (a) and the QUIC arm — and
nothing else.**

### Step 3 — ROW 2: parse stores `""`

**Mechanism, named BEFORE the run.** `spec.TransportProtocol = ""` unconditionally.
**(b)** compares the stored value byte-exactly against `"sctp"` and fails. `fc[0]`
then has no set dimension at all, so `isAllZeroChainSpec` marks it `Empty`
(specificity **0**); for detected `"sctp"` the `source_type` sibling (score **4**)
outranks it, so **(d)** fails. **(c)** already expected `fc[1]`, so it is blind.

```
git diff --numstat  ->  2	1	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **8**, panic gate **0**.

**Verbatim, with the property labels:**

```
=== RUN   TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
    manager_test.go:1738: (b) parsed TransportProtocol: got "", want byte-exactly "sctp"
    manager_test.go:1757: (d) detected="sctp": picked &{l_tp/filter_chains[1] false 0 [] []  [] true false [] []} (err <nil>), want "l_tp/filter_chains[0]" -- the dimension must be ENFORCED, not ignored
--- FAIL: TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue (0.00s)
```

**⚠️ (c) MUST NOT REDDEN — CONFIRMED BY MEASUREMENT.**
`/usr/bin/grep -c '(c) detected' row2.txt` → **0**. Not one `(c)` line in the
whole 8-failure log. **And (c) demonstrably RAN:** (d) is *below* (c)'s
three-iteration loop in source order and (d) fired, so control flow passed
through (c). This is the §5.1 blindness the PLAN predicted, **recorded as
EXPECTED, not as a gap** — an emptied `fc[0]` loses to the sibling on
specificity, so (c) answers `fc[1]` for the right shape and the wrong reason.

**⚠️ ROW 2 REDDENS SIX MORE ARMS THAN §8 NAMES.** §8's row 2 names only (b) and
(d) as must-redden; it makes **no exclusivity claim** for this row (unlike row
1), so this is collateral, not a contradiction — but it is recorded, because an
unexplained extra is indistinguishable from a surprise:

| collateral arm | verbatim |
|---|---|
| `TestParseChainSpecAcceptsAllEightDimensions/transport_protocol_raw_buffer` | `manager_test.go:1634: TransportProtocol = "", want "raw_buffer"` |
| `TestParseChainSpec_QUICTransportProtocolAccepted` | `manager_test.go:2054: TransportProtocol = "", want "quic"` |
| `TestQUICChainSelection_TransportProtocolQUICMatches` | `quic_test.go:724: precondition: chainSpecs[…].TransportProtocol = "", want "quic" — the configured value must survive parseChainSpec or this arm tests nothing` |
| `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` | `quic_test.go:801: precondition: … = "", want "tls" — … or this arm's ineligibility is fictional` |
| `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch` | `quic_test.go:890: precondition: … = "", want "totally_bogus_value" — the configured value must be STORED verbatim, not dropped to "", or this arm's ineligibility is fictional` |
| `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` (s2) | `manager_test.go:1923: tag byte = 'A', want 'B': … 'A' means a tls chain matched a plaintext connection` |

Every one is a **storage or precondition** assertion on the same field — exactly
what a "parse drops the value" mutation should hit. (s2) fires because an emptied
`tls` chain becomes `Empty`, i.e. a catch-all that outranks the default slot.
**This is good news about the suite, not bad news about the row:** Task 14's
`quic_test.go` preconditions are load-bearing and demonstrably fire.

### Step 4 — ROW 3: neutralise the `TransportProtocol` clause in `matches()`

**Mechanism, named BEFORE the run.** With the clause inert, a chain that *sets*
`TransportProtocol` is eligible regardless of the input. §5.1's `fc[0]` (`"sctp"`,
specificity **16**) then matches for detected `""`/`"tls"`/`"raw_buffer"` and
outranks the sibling (**4**) — **(c)** fails three times. §5.2 **(s2)**'s `tls`
chain becomes eligible on a plaintext connection and outranks the default slot,
so the tagged backend returns `'A'` instead of `'B'`. Fixture `l_tls` diverges
the same way. **(d)** still picks `fc[0]`, so it is blind.

```
git diff --numstat  ->  3	1	internal/listener/listenerfilter/chainmatch.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **5**, panic gate **0**.

**Verbatim, with the property label — (c) three times, once per detected value:**

```
    manager_test.go:1749: (c) detected="": picked "l_tp/filter_chains[0]", want "l_tp/filter_chains[1]" -- an unknown value must be INELIGIBLE, not a wildcard
    manager_test.go:1749: (c) detected="tls": picked "l_tp/filter_chains[0]", want "l_tp/filter_chains[1]" -- an unknown value must be INELIGIBLE, not a wildcard
    manager_test.go:1749: (c) detected="raw_buffer": picked "l_tp/filter_chains[0]", want "l_tp/filter_chains[1]" -- an unknown value must be INELIGIBLE, not a wildcard
```

**(s2), verbatim:**

```
    manager_test.go:1923: tag byte = 'A', want 'B': a plaintext TCP connection is never transport_protocol=tls, so filter_chains[0] must be INELIGIBLE and default_filter_chain must serve. 'A' means a tls chain matched a plaintext connection
```

**⚠️ (d) MUST NOT REDDEN — CONFIRMED BY MEASUREMENT.**
`/usr/bin/grep -c '(d) detected' row3.txt` → **0**; `(b)` → **0**; `(a)` → **0**.
Only (c) fired. **This is the other half of the pair:** with the clause inert,
`"sctp"` still wins on specificity, so (d) answers `fc[0]` for the right shape
and the wrong reason. **Rows 2 and 3 are a gate only TOGETHER** — the measured
matrix below is the proof, and it is why the roster is scored per property.

**Must stay green — CONFIRMED by explicit `--- PASS:` under the mutation:**
`TestServeConnection_NoListenerFilter_RawBufferChainServes` (s1),
`TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` (s3),
`TestQUICChainSelection_TransportProtocolQUICMatches`.

**Collateral, all in-dimension** (§8 names a subset; no exclusivity claimed):
`TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` and
`…BogusDoesNotMatch` (`quic_test.go:809`/`:816`, `:898`/`:905` — the non-matching
chain is now selected), and **`TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain`**
(`chainmatch_test.go:224`/`:227`). ⚠️ **That last one is TASK 6's PURITY ARM.**
§8 reserves it as row 6's *only* firing arm; it is **also** sensitive to row 3.
That does not weaken row 6's claim (row 6 needs an arm that fires *at all*), but
a reader must not infer from a red purity arm that the stamp moved into
`SelectChain` — **the arm is not row-6-specific.**

### Step 5 — THE MATRIX, scored PER PROPERTY

| arm / property | baseline | row 1 | row 2 | row 3 | §8 predicts |
|---|---|---|---|---|---|
| §5.1 **(a)** | green | **RED** | (unreachable) | green | row 1 ✅ |
| §5.1 **(b)** | green | (dead, `Fatalf`) | **RED** | green | row 2 ✅ |
| §5.1 **(c)** ×3 | green | (dead) | **green — BLIND, expected** | **RED ×3** | row 3 ✅, NOT row 2 ✅ |
| §5.1 **(d)** | green | (dead) | **RED** | **green — BLIND, expected** | row 2 ✅, NOT row 3 ✅ |
| §5.3 QUIC bogus arm | green | **RED (boot)** | RED (precondition) | RED (selection) | row 1 ✅ |
| §5.2 (s1) | green | green | green | green | — ✅ |
| §5.2 (s2) | green | green | RED | **RED** | row 3 ✅ |
| §5.2 (s3) | green | green | green | green | — ✅ |
| Task 6 purity arm | green | green | green | RED | row 6 (also row 3) ⚠️ |
| fixture `l_bogus` | — | **RED (boot)**, reasoned | — | — | row 1 ✅ |
| fixture `l_tls` | — | — | — | **RED**, reasoned | row 3 ✅ |
| fixture `l_raw` | — | — | — | green, reasoned | — ✅ |

**⚠️ THE FIXTURE ROW IS REASONED, NOT MEASURED.** No Docker was run in this task.
`l_bogus` carries `totally_bogus_value`, which row 1's restored switch rejects at
boot — the subject never binds, so every arm on that config fails. `l_tls`
expects **DEFAULT** and row 3 makes its `tls` chain eligible on a plaintext
connection, so it would serve **INDEXED** and diverge. `l_raw` expects INDEXED
and still gets it under row 3 (its `raw_buffer` chain is eligible either way), so
it stays green. **These three are MEASURED at Task 19; until then they are
inference.**

### Step 6 — revert, and the proof

```
$ git diff --numstat                                  (EMPTY)
$ git status --porcelain                              (EMPTY, before staging PROGRESS.md)
$ git diff --numstat master -- internal/listener/manager.go
4	9	internal/listener/manager.go          <- unchanged, matches Task 12's shape
$ sha256sum internal/listener/manager.go internal/listener/listenerfilter/chainmatch.go
2c940338275ecbefa8ef1dcfd0e58f4227a13fdb1a75832a24fb02f9443641c2  internal/listener/manager.go
8dfdc71d92ece1b3fd54e280a0a8bc352efd9eca6bd7cc24d40ad92afe1a6186  internal/listener/listenerfilter/chainmatch.go
```

**Both digests are byte-identical to the BEFORE readings in Step 0.**

Final sweep, same seven packages:

| measure | result |
|---|---|
| exit code | **0** |
| `=== RUN` | **398** — unchanged; a mutate-and-revert task adds no arm, and none was added |
| `--- FAIL:` | **0** |
| anchored FAIL **LINES** | **0** |
| panic gate | **0** |
| packages | **7 `ok`** |

`TestEnvoyGoBinary_TwoListenerCutover` passed here. **That rerun clears nothing**
— the row-1 occurrence is cleared by the zero-occurrence grep over `cmd/envoy-go`,
not by this green.

### Findings

**⚠️ FINDING 1 — ROWS 2 AND 3 EACH REDDEN MORE ARMS THAN §8 NAMES; ROW 1 DOES
NOT.** §8 asserts exclusivity only for row 1 ("must redden nothing else"), and
row 1 held exactly. Rows 2 and 3 have no exclusivity column, and each reddened
in-dimension collateral (six and three arms respectively). **Nothing contradicts
§8** — but a future reader scoring "did the row redden exactly its cell?" per run
would mark rows 2 and 3 as surprises. They are not. **Score per property.**

**⚠️ FINDING 2 — TASK 6's PURITY ARM IS NOT ROW-6-SPECIFIC.** It reddens under
row 3 as well. §8 row 6 says "Task 6's purity arm — AND NOTHING ELSE", which
constrains what **row 6** touches, not what touches **the arm**. Task 16 must not
read a red purity arm as proof that the row-6 mutation was applied.

**⚠️ FINDING 3 — §5.1's `t.Fatalf` ON (a) MAKES ROW 1 A ONE-PROPERTY ROW BY
CONSTRUCTION.** Under row 1 the log shows one `(a)` line and nothing else from
that test, because (b)/(c)/(d) are dead code beneath the fatal. That is the
intended design (nothing below is reachable without a manager), and it means
**row 1's evidence is (a) plus the QUIC arm — the §5.1 test contributes exactly
one bit to row 1.**

**Everything else in the brief HELD and was re-measured, not taken on trust:**
the baseline is 398/0/0 across 7 packages; `manager.go` reads `4	9` vs master;
(c) is blind to row 2 and (d) is blind to row 3, both measured to **zero label
occurrences**; both digests round-trip exactly.

**Sub-step count: 6.** Chartered 6, executed 6 (baseline+digests, row 1, row 2,
row 3, matrix, revert+sweep+commit). **It did not cross ~10**, so no
mid-execution split trigger fired.

## Task 16 — NC roster rows 4, 5, 5b and 6, each with its COMPANION verified green

**Nothing in this task lands except this record.** Every mutation was applied,
built (`go build ./...` rc=0), `gofmt`-clean, swept over the same seven packages,
and reverted. Both production files are byte-identical before and after.

### Step 0 — the baseline, re-measured at this tip (not inherited)

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > $SCRATCH/base.txt 2>&1
```

| measure | result |
|---|---|
| exit code | **0** |
| `=== RUN` | **398** |
| `--- FAIL:` | **0** |
| panic gate `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| packages | **7 `ok`, 0 `FAIL`** |

Matches the figure the brief predicted. Digests **BEFORE**:

```
2c940338275ecbefa8ef1dcfd0e58f4227a13fdb1a75832a24fb02f9443641c2  internal/listener/manager.go
8dfdc71d92ece1b3fd54e280a0a8bc352efd9eca6bd7cc24d40ad92afe1a6186  internal/listener/listenerfilter/chainmatch.go
```

### Step 1 — NEUTRALISE, and PROVE the mutated line EXECUTABLE

Rows 5, 5b and 6 mutate to an **unconditional assignment**, so executability is
not in question. **Row 4 is the one that needs proof**, and `if false { … }` was
refused. The neutraliser is a second conjunct on a value that cannot occur in
practice but is a genuine runtime field:

```go
if inputs.TransportProtocol == "" && inputs.DestinationPort == 0 { // NC row 4
	inputs.TransportProtocol = "raw_buffer"
}
```

`inputs.DestinationPort` is `localPort(raw)` (`manager.go:1320`) — the bound port
of a real socket, never a compile-time constant, so the comparison **cannot** be
folded away. **Proven by execution, not by reading the shape.** A one-off probe
variant carrying `log.Printf("NCROW4PROBE site reached: tp=%q dport=%d", …)`
immediately above the guard, run against (s1):

```
2026/09/20 22:52:17 NCROW4PROBE site reached: tp="" dport=34023
```

The site is reached with `tp == ""`, so Go's left-to-right `&&` **necessarily
evaluated** the second conjunct; it read `34023`, so the body did not run. The
line is **live and false**, not dead. The probe `log.Printf` was then stripped
(`git diff --numstat` → `1	1`) and the sweep run against the clean neutraliser.

### Step 2 — ROW 4: delete (neutralise) the stamp

**Mechanism, named BEFORE the run.** Without the stamp `inputs.TransportProtocol`
stays `""`. (s1)'s `filter_chains[0]` spells `raw_buffer`, and `matches()`
requires `c.TransportProtocol == inputs.TransportProtocol`, so `"raw_buffer" != ""`
makes it **ineligible** and `default_filter_chain` serves → tag `'B'`, not `'A'`.
(s2)'s chain spells `tls`; `"tls" != ""` was already ineligible, and the default
chain is still the right answer, so (s2) is **structurally green**. (s3)'s input
was classified `"tls"` by `tls_inspector`, so the guard never fired there in the
first place — unchanged.

```
git diff --numstat  ->  1	1	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **1**, panic gate **0**.

**(s1) — RED, verbatim:**

```
=== RUN   TestServeConnection_NoListenerFilter_RawBufferChainServes
    manager_test.go:1841: tag byte = 'B', want 'A': a TCP connection no listener filter classified must be stamped transport_protocol=raw_buffer, which makes filter_chains[0] eligible. 'B' is default_filter_chain, meaning the detected transport protocol stayed empty
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
```

**Must NOT redden — VERIFIED by explicit `--- PASS:`, and each one EXECUTED**
(it carries its own `=== RUN` line and the sweep's only `--- SKIP` is
`TestHandleListeners_IPv6BindAddrPassthrough`, present identically at baseline):

```
--- PASS: TestServeConnection_NoListenerFilter_TLSChainDoesNotServe (0.00s)        (s2)
--- PASS: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (0.00s)   (s3)
```

**Collateral: NONE.** `--- FAIL:` = 1 and it is (s1). Row 4 reddened **exactly**
§8's unit-layer prediction.

### Step 3 — ROW 5: stamp unconditionally with `"tls"`

**Mechanism, named BEFORE the run.** Every TCP input becomes `"tls"`.
(s1): `"raw_buffer" != "tls"` → the indexed chain is ineligible → default serves →
`'B'` instead of `'A'`. (s2): `"tls" == "tls"` → the indexed chain becomes eligible
and outranks the default slot → `'A'` instead of `'B'` — **(s2) fails in the
OPPOSITE direction to row 4's (s1)**, which is what makes the pair a gate. (s3):
`tls_inspector` had already written `"tls"`, so the unconditional stamp writes the
**same value** and the `tls` chain still terminates TLS — green.

```
git diff --numstat  ->  1	3	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **2**, panic gate **0**.

**(s1) and (s2) — RED, verbatim:**

```
    manager_test.go:1841: tag byte = 'B', want 'A': a TCP connection no listener filter classified must be stamped transport_protocol=raw_buffer, which makes filter_chains[0] eligible. 'B' is default_filter_chain, meaning the detected transport protocol stayed empty
--- FAIL: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)
```

```
    manager_test.go:1923: tag byte = 'A', want 'B': a plaintext TCP connection is never transport_protocol=tls, so filter_chains[0] must be INELIGIBLE and default_filter_chain must serve. 'A' means a tls chain matched a plaintext connection
--- FAIL: TestServeConnection_NoListenerFilter_TLSChainDoesNotServe (0.00s)
```

**Must NOT redden — VERIFIED:** `--- PASS: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (0.00s)`.
⚠️ It **executed**: its 0.00s is the healthy shape, and Step 4 shows the same arm
taking **5.00s** when the TLS dial actually fails — the two are distinguishable,
so a green here is not a silently-skipped arm.

**Collateral: NONE.** Notably `TestServeConnection_DefaultFilterChainTLS_ShapeB_IneligibleFilterChain`
and the whole `TestServeConnection_SSL*` family stayed green: none of them
discriminates on `transport_protocol`, so a `"tls"` stamp has no path to them.

### Step 4 — ROW 5b: drop the `== ""` guard, stamp `"raw_buffer"` unconditionally

**Mechanism, named BEFORE the run.** (s1) and (s2) arrive at the stamp with `""`,
so an unconditional `"raw_buffer"` is **byte-identical in effect** to the real
guarded stamp — both green, and that is the point: this row is invisible to every
plaintext arm. (s3) arrives with `"tls"` written by `tls_inspector`; overwriting it
with `"raw_buffer"` makes the TLS-terminating `filter_chains[0]` ineligible, so the
**plaintext default chain** serves and can never produce a ServerHello.

```
git diff --numstat  ->  1	3	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **1**, panic gate **0**.

**(s3) — RED, verbatim. ⚠️ The observable is a DIAL FAILURE (5.00 s), not a tag
byte** — Task 4's carried-forward finding, confirmed, and the test's own message
names the mechanism:

```
=== RUN   TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
    manager_test.go:2017: TLS dial: context deadline exceeded — the tls chain did not terminate TLS. If the transport-protocol stamp OVERWROTE the input tls_inspector classified as "tls", filter_chains[0] became ineligible and the plaintext default_filter_chain served, which cannot complete a handshake
--- FAIL: TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten (5.00s)
```

**Must NOT redden — VERIFIED by explicit `--- PASS:`:**

```
--- PASS: TestServeConnection_NoListenerFilter_RawBufferChainServes (0.00s)   (s1)
--- PASS: TestServeConnection_NoListenerFilter_TLSChainDoesNotServe (0.00s)   (s2)
```

**Collateral: NONE.** Row 5b reddened **(s3) ONLY**, exactly as §8 claims
exclusively.

### Step 5 — ROW 6: the stamp MOVED into `SelectChain`

**Mechanism, named BEFORE the run.** `SelectChain` takes `inputs` **by value**, and
`serveConnection` never reads `inputs` after the call (§0.13), so moving the stamp
from just-before to just-inside is observationally identical on every production
path. Task 6's purity arm calls `SelectChain` **directly** with `ChainMatchInputs{}`
and one `raw_buffer` chain and no default: under the mutation the callee defaults
the input, the chain matches, and `(rb, nil)` is returned where `(nil, ErrNoChainMatched)`
is required.

```
git diff --numstat  ->  3	0	internal/listener/listenerfilter/chainmatch.go
                        0	3	internal/listener/manager.go
```

Sweep: rc=**1**, `=== RUN` **398**, `--- FAIL:` **1**, panic gate **0**.

**Task 6's purity arm — RED, verbatim (both `t.Errorf`s fired):**

```
=== RUN   TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain
    chainmatch_test.go:224: SelectChain(empty TP, raw_buffer chain, no default) = (&{rb false 0 [] [] raw_buffer [] false false [] []}, <nil>); want (nil, ErrNoChainMatched) — SelectChain must NOT default the input
    chainmatch_test.go:227: SelectChain(empty TP, raw_buffer chain, no default) returned chain &{rb false 0 [] [] raw_buffer [] false false [] []}; want nil
--- FAIL: TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain (0.00s)
```

**Matched negative — GREEN under the mutation AND at the clean tip:**
`--- PASS: TestSelectChain_ClassifiedTransportProtocolStillMatches (0.00s)` in both
`row6.txt` and `base.txt`. The purity arm is therefore not a blanket detector.

#### 🔴 Row 6 confirmed INDEPENDENTLY of its arm turning red

Task 15 established that the purity arm **also** reddens under roster row 3, so a
red purity arm is **not** proof the stamp moved. Three confirmations, none of which
is the arm's colour:

1. **The applied diff**, `+3/-0` in `chainmatch.go` and `-3/-0` in `manager.go`,
   with `/usr/bin/grep -c 'raw_buffer' internal/listener/manager.go` → **0**
   (it reads **2** in the pristine file). The manager-side stamp is provably gone.
2. **The paired control against row 4.** Row 4 also removes the manager-side stamp
   from the execution path and (s1) goes **RED**. Under row 6, with the manager
   stamp *deleted outright*, **(s1) stays `--- PASS:`**. The only difference between
   the two trees is the `chainmatch.go` insertion, so the `raw_buffer` default must
   now be supplied by `SelectChain`. **No other roster row produces
   {manager stamp absent} ∧ {(s1) green}.**
3. **Row 3 is excluded by set difference.** Task 15 measured row 3 as **5**
   failures; row 6 has **1**. Under row 6 the four row-3 casualties are all green —
   `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue` (property
   (c) ×3), `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` (s2),
   `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` and
   `…BogusDoesNotMatch`. So `matches()` still **enforces** the dimension; only the
   empty input is being defaulted. That is row 6's mechanism and not row 3's.

**Collateral: NONE.** Row 6 reddened **Task 6's purity arm AND NOTHING ELSE**,
exactly as §8 claims exclusively.

### Step 6 — THE MATRIX, scored PER ARM

| arm | baseline | row 4 | row 5 | row 5b | row 6 | §8 predicts |
|---|---|---|---|---|---|---|
| §5.2 **(s1)** `…RawBufferChainServes` | green | **RED** | **RED** | green | green | 4 ✅, 5 ✅, NOT 5b ✅, NOT 6 ✅ |
| §5.2 **(s2)** `…TLSChainDoesNotServe` | green | green | **RED** | green | green | NOT 4 ✅, 5 ✅, NOT 5b ✅, NOT 6 ✅ |
| §5.2 **(s3)** `…ClassifiedInputNotOverwritten` | green | green | green | **RED (5.00s dial)** | green | NOT 4 ✅, NOT 5 ✅, 5b ✅, NOT 6 ✅ |
| §5.1 (a)–(d) `…AsNonMatchingValue` | green | green | green | green | green | NOT 6 ✅ |
| §5.3 QUIC bogus arm | green | green | green | green | green | NOT 6 ✅ |
| Task 6 **purity arm** | green | green | green | green | **RED** | 6 ✅ |
| Task 6 **matched negative** | green | green | green | green | green | must stay green ✅ |
| `--- FAIL:` total | 0 | **1** | **2** | **1** | **1** | — |
| fixture `l_raw` | — | **RED**, INFERRED | **RED**, INFERRED | green, INFERRED | green, INFERRED | 4 ✅, 5 ✅, NOT 5b ✅, NOT 6 ✅ |
| fixture `l_tls` | — | green, INFERRED | **RED**, INFERRED | green, INFERRED | green, INFERRED | NOT 4 ✅, 5 ✅, NOT 5b ✅ |
| fixture `l_bogus` | — | green, INFERRED | green, INFERRED | green, INFERRED | green, INFERRED | — ✅ |

**All four rows reddened EXACTLY §8's prediction, with ZERO collateral.** Unlike
Task 15's rows 2 and 3 (which reddened 6 and 3 extra arms), rows 4, 5, 5b and 6
each fired precisely the named set — so §8's exclusivity claims for rows 5b and 6
are **confirmed by measurement**, and rows 4 and 5 turn out to be exclusive too,
which §8 did not claim.

### ⚠️ THE FIXTURE ROWS ARE INFERENCE, NOT MEASUREMENT — no Docker ran in this task

Per-arm mechanism, named rather than guessed. All three `0123` listeners are
**plaintext with no `listener_filters` and no `transport_socket`** (Task 7 Step 6,
re-read here), so on every one of them the detected input reaching the stamp is `""`:

- **Row 4** — nothing stamps, input stays `""`. `l_raw`'s `raw_buffer` chain is
  ineligible → **DEFAULT**, but `expectations.yaml:24` wants **INDEXED** → `l_raw`
  **RED**. `l_tls` (`tls` chain) and `l_bogus` (`totally_bogus_value` chain) were
  never eligible and still answer DEFAULT, which is what both expect → green.
- **Row 5** — input becomes `"tls"`. `l_raw`: `"raw_buffer" != "tls"` → DEFAULT
  vs INDEXED → **RED**. `l_tls`: `"tls" == "tls"` → the indexed chain wins →
  **INDEXED** vs the expected DEFAULT → **RED**. `l_bogus`:
  `"totally_bogus_value" != "tls"` → DEFAULT → green.
- **Row 5b** — input becomes `"raw_buffer"`, which is what the real guarded stamp
  writes on all three anyway. Every listener answers its expected chain →
  **all three green**.
- **Row 6** — `SelectChain` supplies the identical default for the TCP path →
  **all three green**.

**These four rows are MEASURED at Task 19; until then they are inference.**

### 🔴 FINDING — `expectations.yaml` CONTRADICTS `PLAN.md` §8 ON ROW 5b, AND `expectations.yaml` IS WRONG

`test/fixtures/0123-listener-transport-protocol/expectations.yaml:162-163` reads:

> a subject stamping a constant `"tls"` passes `l_tls` and fails `l_raw`; one
> stamping a constant `"raw_buffer"` passes `l_raw` and **fails `l_tls`**; one
> stamping nothing fails `l_raw` alone.

The middle clause's second half is **false**, and it is exactly roster row 5b.
A subject stamping the constant `"raw_buffer"`: `l_tls`'s chain spells `tls`, and
`"tls" != "raw_buffer"`, so the chain is ineligible and the **default** chain
serves — which is precisely what `expectations.yaml:25` expects for `l_tls`.
`l_tls` **passes**. `PLAN.md` §8 says the opposite of the fixture doc — row 5b must
**NOT** redden *"every fixture listener"* — and §8 is the one that survives. The
unit layer corroborates it: row 5b left (s1) **and** (s2) green, and (s2) is the
same shape as `l_tls` (a `tls`-spelling chain on a plaintext connection with no
listener filter). Only the `"tls"` constant fails `l_tls`; the `"raw_buffer"`
constant does not. ⚠️ **The clause reads as a stronger discrimination claim for the
`l_raw`/`l_tls` pair than the pair actually supports** — the pair excludes a
constant `"tls"`, and it excludes *stamping nothing*, but it is **blind to a
constant `"raw_buffer"`**. That blindness is not a defect: it is why §5.2 (s3)
exists and must carry a TLS client. **Task 19 owns the fix**; this task does not
touch fixture files.

### 🔴 FINDING — THE BRIEF'S `l_raw` PREMISE IS REFUTED: `l_raw` HAS NEVER BEEN DOCKER-RUN AT ANY TIP

The brief asserts *"this spine deliberately ran the tree at the Task 11 tip
(reject-lift landed, stamp NOT yet landed) — which is row 4's condition"* and asks
whether fixture `l_raw` was executed there. **It was not, and no Docker ran at that
tip at all.** The record:

- **Task 9** is the only Docker execution in this IMPL. It ran at the **un-fixed**
  tip (pre-Task-11), and the subject **boot-REJECTED on `l_bogus`**
  (`totally_bogus_value`), three port-retry attempts, `--- FAIL: TestDifferential/0123-listener-transport-protocol (5.42s)`.
  Task 9's own record states it plainly: *"the fixture dies at the boot step and
  **no arm is scored** — `l_raw`'s divergence is not even reached yet."*
- **Task 10**'s roster row for the fixture reads **"FAILS at boot … (Task 9; not
  re-run here)"**.
- **Tasks 11 and 12** both carry an explicit gate row: **`Docker / differential
  suite | NOT RUN — Tasks 13 and 19 own those`**.
- **Tasks 13, 14 and 15** ran no differential either (`grep -i docker|differential`
  over their sections finds only `go vet`/`go build`/dependency-graph mentions).

⇒ **`l_raw` has been RED only by inference, never by a scored Docker arm**, at the
un-fixed tip or any other. (s1)'s measured red is a **unit-layer** measurement of
the same mechanism and **must not be upgraded** into a measurement of `l_raw`.
Task 19 is the first opportunity to score it.

### Step 7 — revert, the round-trip proof, and TWO pre-registered flakes

```
$ sha256sum internal/listener/manager.go internal/listener/listenerfilter/chainmatch.go
2c940338275ecbefa8ef1dcfd0e58f4227a13fdb1a75832a24fb02f9443641c2  internal/listener/manager.go
8dfdc71d92ece1b3fd54e280a0a8bc352efd9eca6bd7cc24d40ad92afe1a6186  internal/listener/listenerfilter/chainmatch.go
$ git diff --numstat                                        (EMPTY)
$ git status --porcelain                                    (EMPTY, before staging PROGRESS.md)
$ git diff --numstat master -- internal/listener/manager.go
4	9	internal/listener/manager.go                <- unchanged, Task 12's shape
$ gofmt -l internal/listener/manager.go internal/listener/listenerfilter/chainmatch.go
                                                            (EMPTY OUTPUT)
```

**Both digests are byte-identical to the BEFORE readings in Step 0.**

⚠️ **The post-revert sweep was NOT clean, and both failures are the two flakes
this task was briefed on. Neither was cleared by rerunning until green.**

| sweep | tree | rc | `=== RUN` | `--- FAIL:` | panic | failure |
|---|---|---|---|---|---|---|
| final #1 | reverted, digests verified | 1 | 398 | 2 (one parent + one subtest = **one** failure) | 0 | `TestSDSEndToEnd_FetchFailure_BootFailsClosed/silent_SDS_server…` (`internal/boot`) |
| final #2 | **byte-identical**, digests re-verified | 1 | 398 | 1 | 0 | `TestEnvoyGoBinary_TwoListenerCutover` (`cmd/envoy-go`) |

**Structural clearance — by identity, by symbol gate, and by isolation control:**

1. **Identity.** The two mutated files are sha256-identical to the Step 0 baseline
   that swept **rc=0**, and `git diff --numstat` is EMPTY. The **same bytes** swept
   `internal/boot` **`ok`** five times in this task (baseline + rows 4, 5, 5b, 6)
   and `cmd/envoy-go` **`ok`** six times. A code cause is excluded by identity, not
   by colour.
2. **Symbol gate.** `git grep -c -E 'transport_protocol|TransportProtocol' -- internal/boot/`
   → **rc=1, no output**; same gate over `cmd/envoy-go/` → **rc=1, no output**.
   Positive control over `internal/listener/` → **rc=0, 12 files**. Neither package
   references the dimension, so no mutation in this task had a path to either.
3. **Named mechanisms, both pre-registered.** The SDS arm is the known
   **SDS dial-budget flake**, the *same test and same subtest* classified at Task 11
   by a four-measurement control including a pre-edit control sweep. The cutover arm
   is the known in-band ephemeral collision, verbatim:
   `admin start 127.0.0.1:42019: listen tcp 127.0.0.1:42019: bind: address already in use`
   — **42019 sits inside `net.ipv4.ip_local_port_range` = `32768 60999`**, the banked
   mechanism.
4. **Isolation controls** (non-determinism measurements, not retries-to-green):
   `TestSDSEndToEnd_FetchFailure_BootFailsClosed` 3 × `-count=1` → **3/3 `ok`**;
   `TestEnvoyGoBinary_TwoListenerCutover` 3 × `-count=1 -v` → **3/3 `--- PASS:`**
   (`-v` used deliberately so the `-run`-matches-nothing-exits-0 footgun could not
   read as a pass).
5. **Set reconciliation across the two byte-identical final sweeps:** all **7**
   packages are `ok` in at least one, each of the two failures is `ok` in the other,
   `=== RUN` is **398** in both, and the panic gate is **0** in both. **No third
   sweep was run** — chasing an all-green run would have been the
   rerun-until-green error this project forbids.

⚠️ **One brief inaccuracy worth recording:** the brief locates the SDS dial-budget
flake in `internal/listener`. It is in **`internal/boot`**
(`TestSDSEndToEnd_FetchFailure_BootFailsClosed`, `boot_sds_e2e_test.go:551`).

### Gates

| gate | result |
|---|---|
| `go build ./...` (before every sweep) | rc=**0**, all five mutations |
| `gofmt -l` on both mutated files | **empty output**, every mutation |
| panic gate `^panic:\|DATA RACE\|SIGSEGV` | **0** in all six sweeps |
| `golangci-lint` | NOT RUN — the environmental departure NAMED at Task 13 |
| Docker / differential suite | **NOT RUN** — Task 19 owns it |

**Sub-step count: 8.** Seven chartered (baseline, row 4, row 5, row 5b, row 6,
matrix, revert+sweep+commit) plus one forced: the flake classification after the
post-revert sweep came back non-green. **It did not cross ~10**, so `BOOTSTRAP_PROMPT.md`
§6.1's mid-execution split trigger did not fire.

## Task 17 — `ADR-0320` completed IN PLACE, and `BEHAVIOR_CONTRACT.md` — plus the §Context ¶7 adjudication

**Two files, one commit (three with this one).** No code changed:
`git diff master --numstat -- internal/ test/` is byte-identical to Task 16's.

### Step 1 — §Decision and §Consequences APPENDED after the RETAINED italic footer

The footer `*§Decision and §Consequences follow at the phase-98 IMPL.*` was located
**by literal**, not by line, and was the **last line of the file** (`19235`, and
`wc -l` read 19235). It is RETAINED; the new sections follow it.

**Form taken from the neighbours, not from the brief.** `ADR-0318` (`:18944`),
`ADR-0319` (`:19059`) and their §Decision / §Consequences blocks were read at this
tip: `### Decision (landed at the phase-NN IMPL)` / `### Consequences (landed at
the phase-NN IMPL)`, bold-lead paragraph, numbered `**N — …**` decision items,
lettered `**(a) …**` consequences, **no `**Status:**` line inside the body, no
renumber, no `---` separator**. Both invariants were then MEASURED rather than
assumed:

| gate | master | after |
|---|---|---|
| `/usr/bin/grep -cE '^---$' docs/envoy-go/DECISIONS.md` | **216** | **216** |
| `/usr/bin/grep -cE '^## ADR-' docs/envoy-go/DECISIONS.md` | **319** | **319** |

⇒ no separator added, no ADR renumbered or added.

### Step 2 — the house guard flipped, VERIFIED BY LINE AND BY ADR

```
$ /usr/bin/grep -nE '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md
rc=1, EMPTY                                    (rc captured in its own $(...), not through a pipe)

$ HIT=$(/usr/bin/grep -nE '^> \*\*STATUS: ACCEPTED — §Context drafted at the phase-98 SPEC' … | cut -d: -f1)
HITLINE=19217
$ awk -v h=$HIT 'NR<=h && /^## ADR-/ {hd=$0; n=NR} END {print n, hd}' docs/envoy-go/DECISIONS.md
19215 ## ADR-0320 — `filter_chain_match.transport_protocol` is a free-form string, …
```

⇒ the flipped line belongs to **ADR-0320**, established by backward heading
search, **never by a count**.

**The `ADR-0231` decoy is BYTE-UNTOUCHED.** It uses the DIFFERENT matcher
`^\*\*Status:\*\* PROPOSED` and was never run as the gate. Per-line md5,
**trailing newline INCLUDED** (`sed -n '14866p' f | md5sum`):

| | line | md5 |
|---|---|---|
| before | 14866 | `929719b67c87aa16ac1e406fed7eba6b` |
| after | **14866** | `929719b67c87aa16ac1e406fed7eba6b` |

The line did not move either — this row's insertion is at `:19215+`, ~4300 lines
below it, and that was **verified, not assumed**.

⚠️ The flip also re-tensed the guard's own narration on the phase-97 precedent
(`d4940d68`): *"THIS BLOCK RE-ARMS …"* → *"THE HOUSE `PROPOSED` GUARD WAS RE-ARMED
BY THIS BLOCK AT THE SPEC AND IS DISARMED BY THIS FLIP"*, and *"would be falsified
by its own landing"* → *"would have been"*. **No count of either matcher is
written anywhere in the block**, deliberately.

### 🔴 Step 2b — THE F3 ADJUDICATION: §Context ¶7 IS RE-TENSED IN PLACE

**The defect.** `ADR-0320` §Context ¶7 (`:19233`), drafted at the SPEC, reads that
this ADR *"does not measure the listener-filter-timeout fall-through on the
reference, which the repair also covers by construction and which **remains
inferred**"*. `PLAN.md` §0.3 **MEASURED it** — so the ADR's own §Context carried a
statement its own stage had since falsified, and §Decision was about to be
appended beneath it.

**What §0.3 actually measured, read before deciding.** On the reference, with
`listener_filters_timeout: 1s` and `tls_inspector` installed: a client silent for
2500 ms is served by the `raw_buffer` chain (`chain_indexed` 1, `chain_default` 0,
`downstream_pre_cx_timeout` 1, with `listener filter times out after 1000 ms` /
`fallback to default listener filter` in the log); the **matched negative** (a
2-line-diff listener whose chain spells `tls`) falls to the DEFAULT chain, so the
dimension is enforced on that path rather than chain 0 winning by index; and
`continue_on_listener_filters_timeout: false` **drops** the connection
(`cx_total` 0).

**What it LICENSES is narrower than "the timeout path is covered".** §0.3 states
it itself: the timeout arm and the immediate-send arm agree **by two mechanisms,
not one path** (the immediate arm is `raw_buffer` because the inspector actively
classified; the timeout arm with no classification at all), discriminated by the
`pre_cx_timeout` delta and the inspector's `recv` log lines. Established: *the
repair's "stamp after the pipeline regardless of how it ended" matches the
reference's observable*, **and no more**. And in the other direction, §0.1 and
§0.2 forbid the opposite overclaim: on the SUBJECT the timeout is not enforced at
all, and the `== ""` guard is **unreachable** on any timeout path, because
`tls_inspector` is the only listener filter in the tree and all five of its return
paths write the field.

**The house precedent, VERIFIED rather than taken from the brief.**
`/usr/bin/grep -c 'RE-TENSED' docs/envoy-go/DECISIONS.md` reads **1**, at
`:19071` — `ADR-0319` §Context ¶4, landed by `d4940d68` (phase-97 IMPL). The same
commit corrected an `ADR-0318` **§Consequences** carrier in place under a
different bracket label, `**[MECHANISM CORRECTED at the phase-97 IMPL — …]**`
(⚠️ so the brief's *"re-tensed"* is right about the ACT and wrong about the
WORD for that second carrier; the label differs). Both are the same shape:
the original sentence is **left standing**, a bold bracket is appended to the end
of the paragraph, it **leads with what survives**, and it explicitly refuses the
adjacent fresh false claim.

**Verdict: RE-TENSE IN PLACE, on the ADR-0319 ¶4 form, AND record it in
§Consequences (f)** — which is also what the phase-97 row did (it re-tensed ¶4 in
place *and* stated the amendment in its §Consequences (e)). Not chosen:
overwriting ¶7 (it would silently destroy a SPEC-stage record), and leaving ¶7
alone (it would leave a live governing document asserting *inferred* about
something this row's own stage measured).

**The exact text appended to ¶7** (one bracket, at the end of the paragraph, the
SPEC's sentence untouched):

> **[RE-TENSED at the phase-98 IMPL — THE NON-DECISION SURVIVES; THE WORD
> *INFERRED* DOES NOT.** This ADR still decides nothing about
> `listener_filters_timeout`. What changed is the evidence: the phase-98 PLAN §0.3
> MEASURED the fall-through ON THE REFERENCE … ⚠️ **What that licenses is bounded,
> and the bound is the point:** … **What is established is that the repair's
> "stamp after the pipeline regardless of how it ended" matches the reference's
> observable, and no more.** ⚠️ **NOT** *"the subject's timeout fall-through is now
> covered"* … The clause *"which the repair also covers by construction"* survives
> only as a statement about PLACEMENT … That subject-side gap is BANKED as a
> separate divergence (§Consequences (g)), not folded in here.**]**

### Step 3 — §Consequences (a) names the parity direction

`**(a) THE ONE BEHAVIOUR CHANGE THAT CAN MOVE TRAFFIC ON A CONFIG THAT ALREADY
BOOTED**`: a listener with no `listener_filters` carrying a
`transport_protocol: raw_buffer` chain used to fall to `default_filter_chain` (or
be CLOSED where there is none); after this row **the `raw_buffer` chain serves
instead of the fallback**, which is the reference's behaviour and therefore the
parity direction. The paragraph also states the complement: the lifted parse
reject can only turn a **boot failure** into a boot, so it cannot move traffic on
a config that already started.

### 🔴 Step 3b — the (s3) / constant-stamp line: INCLUDED, as §Consequences (d)

Task 16 refuted `expectations.yaml:162-163`'s claim that a subject stamping a
constant `"raw_buffer"` *"fails `l_tls`"*. **Re-verified here from the fixture
itself, not inherited:** `expectations.yaml:25` expects `l_tls` → `DEFAULT`, and
a constant `"raw_buffer"` leaves `l_tls`'s `tls`-spelling indexed chain
ineligible, so the default chain serves — which is exactly what that line expects.
⇒ the `l_raw`/`l_tls` pair is **BLIND to the very constant this row installs**,
and NC roster row 5b (Task 16 Step 4) confirmed that unit arm **(s3)** is the only
thing that reddens under it, at a 5.00 s dial timeout.

**Decision: §Consequences (d) records it.** Justification, weighed against scope
creep: (1) it is a property of **this row's own gate**, not of another row's code —
what this row's evidence can and cannot exclude belongs in this row's governing
record, on the same principle as ADR-0319 §Consequences (a), which recorded a
control that contradicted its own predicted roster; (2) it is stated as a
**discrimination limit plus the arm that closes it**, not as a defect report and
not as a repair — it charters nothing and moves no file; (3) omitting it would
leave the ADR implying the differential pair is a stronger gate than it is, which
is the exact class of claim this project treats as unevidenced. **The fixture file
itself was NOT touched — Task 19 owns that fix.**

§Consequences (c) additionally states, rather than implies, that fixture `0123`
has been Docker-run **only at the un-fixed tip**, where the subject boot-rejected
on `l_bogus` and **no arm was scored**; its green is owed by Task 19 and is not
asserted in the ADR.

### Step 4 — the `+0, UNCHANGED` ledger entry, QUOTING NO ABSOLUTE

Inserted **by literal** immediately after the phase-97 entry (which sat at `:5143`
before this task's other insertion and at `:5144` after it — **the line number was
never reused across edits**). Landed at **`:5146`**.

It follows the phase-96/97 shape exactly: `**Phase 98 — +0, UNCHANGED (…):**`,
the ZERO claim, the mechanism (no registry touched by the parse edit; the stamp
writes a local struct field read only by `SelectChain`), the observable
(redistribution across existing per-chain HCM names, never a new one), the
deliberate non-additions, and the two ⚠️ clauses the precedent carries.

**No absolute appears anywhere in it** — no `A → B`, no total. Mechanically:

```
$ sed -n '5146p' docs/envoy-go/BEHAVIOR_CONTRACT.md | /usr/bin/grep -coE '[0-9]{3,4} (→|->) [0-9]{3,4}'
0
```

The only digits in the row are the phase numbers, the `+4 / -9` diff shape, and
the settled chain-entry precedent list (`44.2, 44.3, 45.2, 47.1, 51, 96, 97`).
⚠️ **The `+0` row DOES earn a chain entry — settled, not re-litigated.**

### Step 5 — the `### Chain-match algorithm` bullet ADDED

The section stated **no** transport-protocol input semantics. That was an
**omission, not a false claim**, and the PLAN's corroborating census was re-run
here at this tip, **both halves**:

| census | before | after |
|---|---|---|
| `/usr/bin/grep -c 'raw_buffer' docs/envoy-go/BEHAVIOR_CONTRACT.md` | **0** (rc=1) | **2** (rc=0) |
| positive control `/usr/bin/grep -c 'transport_protocol' …` | **3** (rc=0) | — |

⇒ the census reaches the file, and the string was genuinely absent. ⚠️ **`grep -c`
counts LINES, not occurrences** — the `2` is two LINES (the new bullet and the new
ledger row), which between them spell the string four times.

The bullet landed at **`:4365`**, second in the section, directly under the
8-dimensions line:

> `- transport_protocol INPUT: whatever the listener-filter pipeline wrote —
> tls_inspector writes tls on a detected ClientHello and raw_buffer on a non-TLS
> preamble — or raw_buffer on a TCP connection the pipeline left unclassified,
> stamped at the ENTRY of chain selection so it also covers a listener carrying no
> listener_filters at all (QUIC stamps quic). The CHAIN's value is a free-form
> string: any value parses and is stored, and one no input can equal simply never
> matches. The comparison is EXACT and CASE-SENSITIVE (RAW_BUFFER does not match a
> plaintext connection, unlike SNI matching), and a chain's empty value means
> unspecified, not raw_buffer. Per ADR-0320.`

### Step 6 — `BEHAVIOR_CONTRACT.md:4359` DELIBERATELY NOT REPAIRED

§0.1 falsified it (`listener_filters_timeout` is not enforced on the subject), and
it is a **banked** row (§10), not this one. Repairing it here would smuggle an
unmeasured third divergence into this row's scope. It is recorded instead — in
ADR-0320 §Consequences (g) and in ¶7's bracket — **naming the falsehood and the
reason it stands.**

**Proved untouched by md5 of the line, trailing newline INCLUDED**, with the line
RE-LOCATED by literal after both insertions (it is above them, so it did not
move — verified, not assumed):

| | line | md5 |
|---|---|---|
| before | 4359 | `0230485b1ad3ddb13dcee599d7734071` |
| after (literal re-locate) | **4359** | `0230485b1ad3ddb13dcee599d7734071` |

### ⚠️ F12 — the mangled cell is OUT OF SCOPE, and here is where it lives

`/usr/bin/grep -rn 'stamps a constant' docs/envoy-go/phases/98-*/` (plus a
whole-repo `git grep`) resolves it to **two** carriers of the defect itself:

- `docs/envoy-go/phases/98-…/SPEC.md:412` — the origin
- `docs/envoy-go/phases/98-…/PLAN.md:1165` — inherited verbatim

both reading `| a subject that ignores the dimension, or stamps a constant `tls`
matches |` — a cell missing a word (intended: *…or stamps a constant `tls`, this
chain matches*). A third hit, `PROGRESS.md:1515-1517`, is this phase's own record
**discussing** the defect, and is correct as written.

**Scope call: NOT fixed by this row, and not by this task.** Neither file is one
of Task 17's two, and both are **closed stage records** — SPEC.md and PLAN.md are
the inputs this IMPL is being measured against; silently editing either mid-spine
would mean a later reader cannot reproduce what this stage was actually given.
The defect is documentation-only, cannot mislead a reader into a wrong action (the
row it annotates is right), and is already recorded at `PROGRESS.md:1515`. **Named
here, edited nowhere.**

### Line-shift discipline — no line number was carried across an edit

Every target was re-located **by literal string** immediately before its edit, and
every assertion is a Python `assert` on the match count (all `== 1`), so a moved
or duplicated anchor aborts rather than lands in the wrong place. The italic
footer, the `STATUS:` line, the ¶7 clause, the 8-dimensions bullet and the
phase-97 ledger line were each matched as exact strings. `BEHAVIOR_CONTRACT.md`
grew `5993 → 5996` (+3: bullet, blank line, ledger row) and `DECISIONS.md`
`19235 → 19359` (+124).

### Gates

```
$ git diff --numstat
3	0	docs/envoy-go/BEHAVIOR_CONTRACT.md
126	2	docs/envoy-go/DECISIONS.md
```

⚠️ **`DECISIONS.md` reads `126 2`, NOT `124 0`, and the `2` is not a mistake:**
two EXISTING lines were rewritten in place — the `STATUS:` line and §Context ¶7 —
and `--numstat` scores each as one deletion plus one addition. So the file's line
count grows by **124** (`19235 → 19359`) while the add column reads **126**.
The figure was MEASURED after the edits, not predicted before them.

**No file outside the three is touched.** `git diff master --numstat -- internal/
test/` is byte-identical to Task 16's roster.

**Sub-step count: 8.** (1) context + baselines, (2) `DECISIONS.md` edits,
(3) `DECISIONS.md` verification, (4) `BEHAVIOR_CONTRACT.md` edits,
(5) `BEHAVIOR_CONTRACT.md` + structural verification, (6) this section,
(7) commit, (8) post-commit re-verification. **It did not approach ~10**, so
`BOOTSTRAP_PROMPT.md` §6.1's mid-execution split trigger did not fire.

---

## Task 18 — `ROADMAP.md` row 98 flipped `in-progress` -> `done`, under the FIELD-COUNT gate

**Scope: exactly two files** — `docs/envoy-go/ROADMAP.md` (`1	1`) and this file.
No production file, no test, no fixture, no `DECISIONS.md`, no
`BEHAVIOR_CONTRACT.md`.

### Step 1 — the row relocated BY ID, never by line

```
$ awk -F'|' '/^\| *98 /{print NR": "$0}' docs/envoy-go/ROADMAP.md
160: | 98 | chain-match-transport-protocol-reject | 97 | in-progress | …
```

`:160` is where the router said it was, and the relocation was run anyway
rather than trusted. The install itself re-derives the index by ID a second
time (a Python pass asserting exactly ONE row whose field 2 strips to `98`),
so no line number was carried across the edit.

### Step 2 — it is a FLIP, not an ADD

| measure | before | after |
|---|---|---|
| `wc -l docs/envoy-go/ROADMAP.md` | **248** | **248** |
| check (1) denominator `want` | **130** | **130** |
| `git diff --numstat -- docs/envoy-go/ROADMAP.md` | — | **`1	1`** |
| `git diff -U0` hunk header | — | **`@@ -160 +160 @@`** — one line, in place |

The status field went `[ in-progress ]` -> `[ done ]` and the IMPL paragraph
was appended to the existing summary cell in the row-96 / row-97 house form
(`**IMPL done <date>** — lifecycle-state 3 -> DONE; …`), which was read off
rows 96 and 97 rather than invented. Row 98's cell carried **no** `SPEC done`
or `PLAN done` marker — measured, not assumed — so the IMPL paragraph
follows the BRAINSTORM paragraph directly, exactly as row 97's does.

### Step 3 — FIELD COUNTS UNDER BOTH FORMS, BEFORE AND AFTER

```
$ sed -n '160p' docs/envoy-go/ROADMAP.md | awk -F'|' '{print "naive NF="NF}'
$ sed -n '160p' docs/envoy-go/ROADMAP.md | sed 's/\\|//g' | awk -F'|' '{print "escape NF="NF}'
```

| form | before | after | want |
|---|---|---|---|
| naive | **8** | **8** | 8 |
| escape-aware | **8** | **8** | 8 |

The cell text was gated as a standalone file BEFORE installation: `|`
occurrences **0**, literal tabs **0**, newlines **1**. **No pipe was escaped,
because none was written** — the phase-96 IMPL's own author wrote a Go `||`
into a row narrative and took the field count 8 -> 10, and the house repair is
to word the pipe away, not to escape it. Every place this cell needed one it
spells the operator or uses ` / ` (`+4 / -9`, `3 / 0`).

### Step 4 — the SENTINEL-PHRASE assertion on the new cell

Check (2) matches the **whole file**, not only the six windows, so a row cell
spelling either phrase would mint a **seventh** hit and read as a finding.

```
$ sed -n '160p' docs/envoy-go/ROADMAP.md | /usr/bin/grep -coE 'deferred candidates:|remaining deferred \(not-yet-chartered\) candidates:'
0                                        # before AND after
```

Run on the draft file before install as well: **0**. ⚠️ `grep -c` exits 1 on
zero matches, so the `0` was read from stdout and the rc discarded, not the
other way round.

### Step 5 — the sentinel, all four NCs and the positive control, ON BOTH SIDES

Everything below was run with `/usr/bin/grep` (this shell's `grep` is a
`ugrep` wrapper; `type grep` reports **`grep is a function`**), against this
worktree's own `ROADMAP.md`, verbatim from `next-prompt.txt`.

| probe | BEFORE the flip | AFTER the flip |
|---|---|---|
| check (1) | **ONE** line: `NOT DONE: row 98` | **SILENT** |
| check (2) | **SIX**: `:208 :214 :220 :230 :236 :244` | **SIX**, same six lines |
| check (3) | **SILENT** | **SILENT** |
| NC-A (row 62 doctored) | `NC LANDED? [ in-progress ]`, then **TWO**: `NOT DONE: row 62`, `NOT DONE: row 98` | `NC LANDED? [ in-progress ]`, then **ONE**: `NOT DONE: row 62` |
| NC-B (`want=129`) | **TWO**: `NOT DONE: row 98`, `GATE FAIL: examined 130 data rows, expected 129` | **ONE**: `GATE FAIL: examined 130 data rows, expected 129` |
| NC-C (gRPC token) | **FIRED**, residual **0** | **FIRED**, residual **0** |
| NC-D (`-family row`, under `--`) | **96** occurrences / **68** lines | **96** / **68** |
| check-(2) positive control | **6** substitutions asserted, residual **0** | **6** substitutions asserted, residual **0** |
| escape-aware malformed set | exactly **{57, 69}** at file lines **119**, **131** | exactly **{57, 69}** at file lines **119**, **131** |

**Every one of the eight shapes moved, or stayed, exactly as predicted.** The
two that MOVE are the two that must: check (1) and NC-A/NC-B each shed the
`row 98` voice, which is the whole observable content of this flip. NC-A's
substitution was **inspected** (`NC LANDED? [ in-progress ]`) on both sides
before its output was believed — an NC that mutated nothing reads as a
passing gate.

NC-D is unmoved because the new cell spells no `-family row` token at all;
this row claims **no family ordinal**, so that is the correct answer and not
an instrument failure.

### Step 6 — the six deferred-candidate windows, BYTE-IDENTICAL

Method, stated because the digest is method-sensitive: **per line,
`sed -n 'Np' docs/envoy-go/ROADMAP.md | md5sum`, trailing newline INCLUDED**,
first 12 hex characters.

| line | before | after | router's figure at the PLAN close |
|---|---|---|---|
| 208 | `10d7807bf02d` | `10d7807bf02d` | `10d7807bf02d` |
| 214 | `4a92f7e62fc6` | `4a92f7e62fc6` | `4a92f7e62fc6` |
| 220 | `2a7eb298b9fd` | `2a7eb298b9fd` | `2a7eb298b9fd` |
| 230 | `242e53c6f7a3` | `242e53c6f7a3` | `242e53c6f7a3` |
| 236 | `b2680e6f4fbf` | `b2680e6f4fbf` | `b2680e6f4fbf` |
| 244 | `6caa1c3ce0e7` | `6caa1c3ce0e7` | `6caa1c3ce0e7` |

**All six unchanged, and unchanged across four consecutive stage closes now.**
The `git diff -U0` hunk header independently confirms the edit touched line
**160** and nothing else, so the windows could not have moved. **The margin is
still ONE.**

### 🔴 FINDINGS

**🔴 FINDING 1 — THE BRIEF'S "Production diff is exactly `4	9` in
`internal/listener/manager.go`" IS TRUE OF THAT FILE AND INCOMPLETE ABOUT THE
PRODUCTION SET.** Re-derived at this tip:

```
$ git diff --numstat 9c7bee0b HEAD -- internal/ | /usr/bin/grep -v '_test.go'
3	3	internal/listener/listenerfilter/chainmatch.go
3	3	internal/listener/listenerfilter/types.go
4	9	internal/listener/manager.go
```

`manager.go` really is `4	9`. But **THREE** production `.go` files are
touched, not one — the other two carrying comment-only, deliberately
line-count-neutral `3	3` edits (Task 14). The row cell therefore says "ONE
production file carries the BEHAVIOUR change" and names the other two, rather
than repeating a one-file claim that a reader could falsify in one command.
`internal/listener/quic.go` **is** byte-untouched, as claimed.

**🔴 FINDING 2 — THE ROW FLIPS `done` ONE TASK BEFORE THE SIX-GATE SWEEP RUNS,
SO THE CELL CAN QUOTE NO SUITE FIGURE.** `PLAN.md` §6 orders Task 18 (this
flip) BEFORE Task 19 (gate (a), the differential suite; gates (b)-(f); the
byte-untouched roster; the ARM roster diff). Rows 96 and 97 wrote their
differential pass counts into the cell because their flips came after the
sweep. **This row's does not, and the cell says so explicitly** rather than
quoting a number that has not been measured. Recorded as an ordering
observation, not a deviation: the order is what the PLAN charters, and it is
followed.

**🔴 FINDING 3 — THE CELL HAD TO CORRECT A DRAFT CLAIM THAT NO DOCKER HAD RUN
AT ANY TIP.** A first draft of the IMPL paragraph asserted the fixture had
never been Docker-run. That is **false**: Task 9 ran the differential suite
against `0123` at the **un-fixed** tip, where the subject boot-rejected on
`l_bogus` and **no arm was scored**. The true and narrower statement — the one
installed — is that the fixture has been Docker-run exactly once, at the
un-fixed tip, with no arm scored, and that **`l_raw` has never been driven
against the reference at any tip** (Task 16). The near-miss is worth recording
because the two statements differ by one run and the wrong one would have read
as a stronger claim than the evidence supports.

### Gates

| gate | command | result |
|---|---|---|
| line count | `wc -l docs/envoy-go/ROADMAP.md` | **248** before, **248** after |
| edit scope | `git diff --numstat -- docs/envoy-go/ROADMAP.md` | **`1	1`** |
| edit locality | `git diff -U0 -- docs/envoy-go/ROADMAP.md` | **`@@ -160 +160 @@`** |
| field count, naive | `awk -F'\|'` on `:160` | **8** -> **8** |
| field count, escape-aware | `sed 's/\\\|//g'` then `awk -F'\|'`, **no file argument to awk** | **8** -> **8** |
| sentinel phrase in the new cell | `/usr/bin/grep -coE` on `:160` | **0** |
| six windows | per-line `md5sum`, trailing newline included | **all six unchanged** |
| commit scope | `git show --numstat` | **exactly two files** |

**Sub-step count: 9.** (1) worktree state + `PLAN.md` §Task 18, (2)
`next-prompt.txt` §sentinel/NC definitions + row relocation, (3) the row-97 /
row-96 house-form read, (4) the full BEFORE-side suite, (5) re-derivation of
the production diff, ADR-0320 status and fixture/contract figures, (6) the
`PROGRESS.md` digest that supplied the arm and NC detail, (7) draft the cell
and gate it standalone for pipes, tabs and sentinel phrases, (8) install +
the full AFTER-side suite, (9) this section + commit. **It did not cross
~10**, so `BOOTSTRAP_PROMPT.md` §6.1's mid-execution split trigger did not
fire.

---

## Task 19 — the byte-untouched roster, the ARM ROSTER, and the SIX-GATE sweep

**The headline: fixture `0123-listener-transport-protocol` has now been SCORED.**
Task 9 was the only differential execution in this IMPL and it ran at the
**un-fixed** tip, where the subject boot-REJECTED on `l_bogus` and **no arm was
scored**; Tasks 11 and 12 both carry `Docker / differential suite | NOT RUN`;
Tasks 15 and 16 labelled every fixture cell **inference**. Gate (a) below is the
**first and only measurement of this row's primary cross-side evidence**, and all
three listeners answered exactly what §7.1 chartered — on **both sides**.

### Step 1 — the byte-untouched roster, PER PATH

`git diff master --numstat -- <path>` at the committing tree:

| path | `--numstat` | verdict |
|---|---|---|
| `internal/listener/quic.go` | (empty) | **BYTE-UNTOUCHED** |
| `internal/listener/listenerfilter/tls_inspector/**` | (empty) | **BYTE-UNTOUCHED** |
| `go.mod` | (empty) | **BYTE-UNTOUCHED** |
| `go.sum` | (empty) | **BYTE-UNTOUCHED** |
| every fixture directory other than `0123` | **123 of 123 empty** | **BYTE-UNTOUCHED** |

The fixture sweep was run as a loop over `git ls-tree -d --name-only master
test/fixtures/` (**124** directories at master), skipping only `0123`; the count
of non-`0123` directories carrying any edit is **0**. Scored on the SET, by path,
never on a total.

**`chainmatch.go` — on the roster for CODE only.** `--numstat` reads `3 3`, and
Task 14 Step 5's awk gate re-run at this tip reads
`GATE: chainmatch.go comment-only -- inspected 6 changed line(s), 0 violation(s)`,
rc=0. The `inspected 6` counter is why this is not a vacuous green (control A of
PLAN §7.3 is the empty-diff case that reads `inspected 0`). The gate was shown to
FIRE at Task 14 (controls C and D, rc=1); it is not re-proved here.

### Step 1b — 🔴 PATHS ON **NEITHER** ROSTER (PLAN §9's phase-97 `BEHAVIOR_CONTRACT.md` hazard)

The byte-untouched roster and `SPEC.md` §6.3's edit set are **not a partition**.
`SPEC.md` §6.3's edit set is: `manager.go`, `manager_test.go`, `quic_test.go`,
`chainmatch.go`, `types.go`, `BEHAVIOR_CONTRACT.md`, `DECISIONS.md`, the fixture
directory, `test/differential/runner_test.go`. Set-differenced against the 14
paths this branch actually touches, **three sit on neither roster**:

| path | on neither because | status |
|---|---|---|
| `docs/envoy-go/phases/98-…/PROGRESS.md` | BOOTSTRAP §5 state 3 requires it | **DECLARED** in PLAN §9's widening table |
| `internal/listener/listenerfilter/chainmatch_test.go` | NC roster row 6's only possible arm | **DECLARED** in PLAN §9's widening table |
| `docs/envoy-go/ROADMAP.md` | Task 18's `in-progress -> done` row flip | 🔴 **DECLARED NOWHERE** — named here |

⚠️ **`ROADMAP.md` is this row's phase-97-`BEHAVIOR_CONTRACT.md` repeat.** It is
neither on `SPEC.md` §6.3's byte-untouched roster (§6.3 is silent about it) nor on
§6.3's edit set nor in PLAN §9's declared-widening table — and PLAN §9.2 in fact
lists it under *"figures this stage must NOT move"*, where the thing pinned is its
**line count (248)** and its **data-row count**, not its bytes. Both invariants
hold at this tip: `wc -l` **248** at master and at HEAD; the same awk over both
revisions returns an identical data-row count (stated as invariance, not as an
absolute — the §2 awk and a naive `^\| *[0-9]+ *\|` awk disagree, so no number is
quoted here). Row 98 field 4 reads ` done `, and the row still has **8**
pipe-fields (the unescaped-`|` hazard).

### Step 2 — the ARM ROSTER, diffed BY NAME in BOTH directions

`git diff master -- internal/listener/ | /usr/bin/grep -E '^\+func (Test|Fuzz)' | sort`
yields **7** names. Against the parked expectation, both `comm` directions:

```
comm -23 (expected, NOT present) -> (empty)
comm -13 (present, NOT expected) -> (empty)
comm -12 (in both)               -> 7
```

The seven, by name:

```
TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue
TestQUICChainSelection_TransportProtocolBogusDoesNotMatch
TestSelectChain_ClassifiedTransportProtocolStillMatches
TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain
TestServeConnection_NoListenerFilter_RawBufferChainServes
TestServeConnection_NoListenerFilter_TLSChainDoesNotServe
TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten
```

⚠️ **Task 6's `+0/+0`-shaped purity arm is PRESENT** —
`TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` and its matched
negative `TestSelectChain_ClassifiedTransportProtocolStillMatches` both appear. A
deleted negative control of that shape leaves every counter gate green, which is
exactly why this step diffs the roster and not a count.

**The re-pointing, measured independently of the `+func` diff.** Top-level
`^func (Test|Fuzz)` names under `internal/listener/**/*_test.go`, master vs HEAD:

```
master 178  ->  HEAD 184   (+6 net)
REMOVED (master-only): TestParseChainSpecRejectsUnknownTransportProtocol
ADDED   (HEAD-only)  : the seven above
```

**7 added − 1 removed = +6**, so the row RE-POINTED one arm rather than adding
eight. ⚠️ **The brief's `309 -> 315` is not this scope** — that figure is the
repo-wide top-level roster (ROADMAP row 98 quotes it); the `internal/listener`
subtree reads **178 -> 184**. The **delta `+6` and the named add/remove sets are
identical**, so the two measurements agree on mechanism while differing on
denominator. No number was inherited; both were re-derived here.

### Step 3 — GATE (a): the differential suite, `-count=1`

**Registration, all four gates, before the run** (the PLAN's extractor, verbatim,
scored on the set difference):

```
imports=125  dirs=125
comm -23 (registered, no dir)  -> (empty)
comm -13 (dir, not registered) -> (empty)
split: driver=101  inputs=24
runner_test.go:150: _ ".../test/fixtures/0123-listener-transport-protocol/driver"
```

**The run:** `go test -count=1 -v -timeout 40m ./test/differential/`, foreground,
**23:20:27 → 23:27:35**, wall **427.280s**, `ok`, rc=0.

```
PASS: 125    FAIL: 0    SKIP: 0
--- FAIL anywhere in the log: 0
--- SKIP anywhere in the log: 0
anchored ^panic:|DATA RACE|SIGSEGV over the suite log: 0
tail: ok  github.com/pgdad/envoy-go/test/differential  427.280s
```

⚠️ **Scored by NAME, both `comm` directions, never by the exit code.** The set of
fixtures that printed `--- PASS: TestDifferential/<name>` was extracted and
differenced against the registered-import set:

```
passed unique = 125
comm -23 (registered, NOT passed) -> (empty)
comm -13 (passed, NOT registered) -> (empty)
```

**125 = 125 = 125**, three-way. **The run did not abort** — the tail is a normal
`ok … 427.280s` and the passed-set is complete, so the driver-receiver port race
did **not** fire and nothing is masked. `0 SKIP` is a measured assertion, not an
assumption: zero `--- SKIP` lines anywhere, and gate 4 (the invisible one — the
`NNNN-` shape `discoverFixtures` enumerates, which leaves no skip line when it
fails) is discharged by the subtest `TestDifferential/0123-listener-transport-protocol`
having **appeared and PASSED**, which only an enumerated directory can do.

#### 🔴 The three `0123` listeners, BY NAME, BOTH SIDES — the first scoring at any tip

Verbatim from the driver's own log lines:

| listener | side | status | body | `*_indexed.downstream_rq_total` | `*_default.downstream_rq_total` | want | verdict |
|---|---|---|---|---|---|---|---|
| `l_bogus` | **ref**  | 200 | `bogus-default\n` | 0 (present=true) | **1** (present=true) | DEFAULT | **DEFAULT** ✅ |
| `l_bogus` | **subj** | 200 | `bogus-default\n` | 0 (present=true) | **1** (present=true) | DEFAULT | **DEFAULT** ✅ |
| `l_raw`   | **ref**  | 200 | `raw-indexed\n`   | **1** (present=true) | 0 (present=true) | INDEXED | **INDEXED** ✅ |
| `l_raw`   | **subj** | 200 | `raw-indexed\n`   | **1** (present=true) | 0 (present=true) | INDEXED | **INDEXED** ✅ |
| `l_tls`   | **ref**  | 200 | `tls-default\n`   | 0 (present=true) | **1** (present=true) | DEFAULT | **DEFAULT** ✅ |
| `l_tls`   | **subj** | 200 | `tls-default\n`   | 0 (present=true) | **1** (present=true) | DEFAULT | **DEFAULT** ✅ |

`--- PASS: TestDifferential/0123-listener-transport-protocol (1.85s)`.

Every counter reads `present=true`, so no cell is the zero-for-a-missing-key
vacuity method note 46 warns about, and `recorded=true` on all six. **Tasks 15
and 16's inference is CONFIRMED by measurement on both sides** — `l_raw` in
particular, which had never been driven against the reference at any tip, now
serves its **INDEXED** chain on the subject exactly as the reference does. That
is ADR-0320 §Consequences (a)'s one traffic-moving behaviour change, witnessed
cross-side for the first time.

**Re-run at the COMMITTING tip** (after the Step 8 comment edit), same six lines,
same verdicts, `--- PASS … (2.35s)`, `ok … 2.415s`.

### Step 4 — GATE (b): the non-Docker package sweep

**The denominator, RECONCILED AS A SET — and the brief's `238` is WRONG for this tip.**

```
raw `go list ./...`                       = 241
excluded (both Docker drivers, by name)   = test/conformance/h2spec, test/differential
non-Docker denominator at HEAD            = 239
non-Docker denominator at master worktree = 238
comm -13 (added at HEAD)   -> github.com/pgdad/envoy-go/test/fixtures/0123-listener-transport-protocol/driver
comm -23 (removed at HEAD) -> (empty)
```

**239, not 238**, and the set difference is **exactly one package**: this row's own
new fixture driver. PLAN §9.2's `238` is master's figure and is correct there; the
brief carried it forward to "this tip", where it is off by the package the row
adds. This is the same mechanism PLAN §9.2 warns about (*"it was 237 for two rows
and phase 97's own fixture package moved it"*) recurring one generation later.
**Reconciled by set; no count trusted.**

`go test -count=1 $(cat pkgs-head.txt)`, `PIPESTATUS[0]` = **1**.

```
ok = 124   no-test-files (?) = 114   FAIL = 1
124 + 114 + 1 = 239  ✔ reconciles with the denominator
```

#### 🔴 GATE (b) IS RED AT EXACTLY ONE PACKAGE — and the redness is NOT this row's

```
--- FAIL: TestEncodeData_LevelMapping_DifferentGzippedSizes (0.00s)
    compressor_test.go:2144: expected different compressed sizes for BestSpeed
    vs BestCompression on a non-repetitive input; both = 2121
FAIL  github.com/pgdad/envoy-go/internal/filter/http/compressor  0.031s
```

**Classified structurally — by mechanism and by a byte-identical control — never
by rerunning until green.** Three independent discriminators:

1. **Scope.** `git diff master --numstat -- internal/filter/http/compressor` is
   **EMPTY**. This row touched zero bytes of the package, and `internal/listener`
   is not in its import graph.
2. **It is NOT A FLAKE — it is DETERMINISTIC.** The input is
   `body[i] = byte((i*17) ^ (i>>3))` over a fixed 4096 bytes: no randomness, no
   clock, no port, no goroutine. Three separate executions returned the **identical
   figure `both = 2121`**. (This is not rerunning-until-green: the prediction was
   that the figure would be the SAME, and it was.)
3. **MASTER-WORKTREE CONTROL.** `go test -run TestEncodeData_LevelMapping_DifferentGzippedSizes
   ./internal/filter/http/compressor/` in the untouched `master` worktree at
   `/home/esa/git/envoy-go` fails with the **byte-identical message and the
   identical `2121`**, rc=1. **Pre-existing.**
4. **ISOLATED TO THE STDLIB.** A 20-line standalone program outside the repo,
   under the installed toolchain, gzipping that exact byte pattern:
   `BestSpeed=2121 BestCompression=2121 equal=true`. **Zero repo code is
   involved.** The test (written at phase 14) encodes an assumption about
   `compress/flate`'s output length across levels that is not an API contract and
   that **go1.27.1** no longer satisfies on this input.

⇒ **NAMED DEPARTURE 3** (below). Gate (b) is **238 of 239 packages green**, with
the single red package byte-untouched by this row, deterministic, red on master,
and reproduced outside the repo entirely.

**`-race` on the FULL `internal/listener` package** (not a `-run` subset — a
background mutator is only caught by the full package):

```
go test -count=1 -race -timeout 20m ./internal/listener/
ok  github.com/pgdad/envoy-go/internal/listener  4.447s     rc=0
anchored ^panic:|DATA RACE|SIGSEGV over the -race log: 0
```

`gofmt -l` over all eight touched `.go` files: **empty**. (⚠️ `gofmt -l` never
exits non-zero — the emptiness of its OUTPUT is the gate.) `go vet
./internal/listener/... ./test/fixtures/0123-…/...`: **no output**.

### Step 5 — GATES (c), (d), (e)

**(c) h2spec — run VERBOSE**, because a non-verbose run prints no summary line at
all, only `ok … Ns`:

```
go test -count=1 -v -timeout 20m ./test/conformance/h2spec/
h2spec_test.go: 95 tests, 94 passed, 1 skipped, 0 failed
ok  github.com/pgdad/envoy-go/test/conformance/h2spec  2.917s    rc=0
anchored panic gate over the h2spec log: 0
```

Exactly the chartered triple. The section roster shows `6.9.2. Initial
Flow-Control Window Size: 3/3 passed`, consistent with the invariant single skip
being 6.9.2/2 (the summary is the scored line; the per-section roster does not
itemise the skip).

**(d) fuzzers — TARGETS and FILES kept DISTINCT:**

| | TARGETS (`git grep -h '^func Fuzz'`) | FILES (`git grep -l '^func Fuzz'`) |
|---|---|---|
| master | **56** | **48** |
| HEAD | **56** | **48** |

**+0 / +0**, as chartered — the row consumes no new config field, so there is no
parse arm to fuzz.

**(e) the anchored panic gate**, `^panic:|DATA RACE|SIGSEGV`, over every log this
task produced:

| log | reading |
|---|---|
| differential suite (2181 lines) | **0** |
| non-Docker sweep | **0** |
| `-race ./internal/listener/` | **0** |
| h2spec | **0** |

**0 everywhere.** ⚠️ At the phase-97 IMPL this gate legitimately read **1**, which
was the gate FIRING on a real abort; a 0 here is the gate reading clean over four
logs, not the gate being absent.

### Step 6 — GATE (f): `REVIEW.md` — a NAMED STANDING DEPARTURE, not compliance

```
phase directories            : 139
carrying REVIEW.md           : 37
carrying REVIEW.md in 93-98  : 0
  93-h2-local-reply-content-length        ABSENT
  94-tls-connection-error-stat            ABSENT
  95-tls-alpn-mismatch-fallback           ABSENT
  96-listener-default-chain-tlsmode       ABSENT
  97-quic-chain-selection-order           ABSENT
  98-chain-match-transport-protocol-reject ABSENT
```

**This row ships no `REVIEW.md`.** That is **37 of 139** directories carrying one
and **none of the last six consecutive phases**. It is stated as a **standing
departure**, not claimed as a gate that passed.

### The NAMED DEPARTURES — three, none smoothed

**DEPARTURE 1 — `golangci-lint` CANNOT RUN, on any package, and it is
ENVIRONMENTAL.** Re-confirmed at this tip:

```
golangci-lint v1.64.8, built with go1.26.2      |  installed toolchain: go version go1.27.1 linux/amd64
HEAD worktree   ./internal/listener/... :
  registry.go:6:2: could not import sync/atomic (-: could not load export data:
  internal error in importing "sync/atomic" (cannot decode "sync/atomic", export
  data version 4 is greater than maximum supported version 2); …) (typecheck)
MASTER-WORKTREE CONTROL, same selector :
  <byte-identical message, same file, same line>
```

The **master-worktree control is what makes this environmental rather than ours** —
an untouched tree fails identically, on a **stdlib** import, before any repo code
is typechecked. ⇒ **`revive`, `errcheck`, `unused` and `staticcheck` are UNRUN
for phase 98** and are recorded as unrun, not as passed. (`misspell` was
discharged by hand at Task 16 against a 1619-key dictionary with a positive
control.)

**DEPARTURE 2 — no `REVIEW.md`** (Step 6 above).

**DEPARTURE 3 — gate (b) is RED at `internal/filter/http/compressor`**, one test,
`TestEncodeData_LevelMapping_DifferentGzippedSizes`. Deterministic, byte-untouched
by this row, red on the untouched master worktree, and reproduced by a standalone
`compress/gzip` program with no repo code at all. **Pre-existing and
toolchain-driven. NOT fixed here, and NOT claimed as green.** It is a real,
separate defect in the tree (a test asserting a non-contractual stdlib property)
and is banked, not smuggled.

### Step 8 — 🔴 DECLARED SCOPE WIDENING: `expectations.yaml`'s own false claim, CORRECTED AFTER MEASUREMENT

⚠️ **PLAN Task 19 says "Files: none — this task only verifies and records."**
This widening is therefore **declared here and in the commit subject**, in the
style PLAN §9 declared Task 6's `chainmatch_test.go`, and **not smuggled**.

**What was false.** `expectations.yaml`'s hazard (b) asserted *"one stamping a
constant `raw_buffer` passes `l_raw` and fails `l_tls`"*. It does **not** fail
`l_tls`: `l_tls`'s indexed chain matches on the string `tls`, a constant
`raw_buffer` stamp leaves it **INELIGIBLE**, the DEFAULT chain serves, and
**DEFAULT is exactly what this file's own table expects for `l_tls`** — so such a
subject **passes both halves of the pair**. The `l_raw` / `l_tls` pair is
**BLIND to a constant `raw_buffer` stamp**, which is precisely the constant this
row installs (guarded by `== ""`). Unit arm **(s3)**
`TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten` is the only arm
that excludes it — NC roster row 5b reddened **(s3) alone**, observable as a 5.00 s
dial timeout. `DECISIONS.md` ADR-0320 §Consequences (d) already records this
correctly; the fixture's own prose did not.

**Measured FIRST, corrected SECOND.** The correction was written only after gate
(a) had actually scored the three listeners — the fixture's real behaviour is the
table above, and the corrected comment is consistent with it.

**The edit:** `expectations.yaml` `26 / 8`, **comment-only** (a `#`-prefix gate
over the unified diff reads `inspected 34 line(s), 0 violation(s)`, rc=0); the
block moves from `:158-165` to `:158-183`; `wc -l` 276 → 294; `yaml.safe_load`
still parses the file.

**Two facts that bound the blast radius, both measured:**
- **`expectations.yaml` has NO CONSUMER.** Every `expectations.yaml` occurrence in
  any `*.go` file in the tree is inside a **comment**; nothing opens it. Precisely:
  `gopkg.in/yaml.v3` **is** in `go.mod`, but its only importer anywhere is
  `internal/bootstrap/bootstrap.go` — **no package under `test/` imports a YAML
  library at all.** (⚠️ The brief's phrasing "no package under `test/` imports a
  YAML library" is exactly right; a reader must not widen it to "the repo has no
  YAML library", which is false.)
- The fixture was **re-run at the committing tip** after the edit and produced the
  identical six log lines and `--- PASS`.

**A complication the brief did not anticipate, handled in the SAME commit.**
`ROADMAP.md:160` — written by Task 18 — cites this very block in the **present
tense**: *"`expectations.yaml:162-163` CONTRADICTS `PLAN.md` §8 …"*. Correcting the
comment makes that cite false at the new tip. Per the project's
re-derive-every-count-you-move-in-the-same-commit rule, the ROADMAP clause is
**re-tensed in place** to CONTRADICTED / was-the-wrong-one, with the correction and
the **new `:158-183` span** named. `ROADMAP.md` stays `1 / 1`, `wc -l` stays
**248**, row 98 field 4 stays ` done `, the row still carries **8** pipe-fields, and
the data-row count is identical under the same awk at master and at HEAD.

### Sub-step count

PLAN charters **7 steps**; executed as **8 sub-steps** (the seven, plus the
declared `expectations.yaml` + `ROADMAP.md` correction recorded as Step 8). Under
`BOOTSTRAP_PROMPT.md` §6.1's mid-execution trigger of ~10; **no split**.

### Gate summary

| gate | reading | verdict |
|---|---|---|
| (a) differential suite `-count=1` | 125 PASS / 0 FAIL / 0 SKIP, both `comm` directions empty, 427.280s, no abort | **GREEN** |
| (a′) fixture `0123`, both sides | `l_bogus` DEFAULT · `l_raw` **INDEXED** · `l_tls` DEFAULT | **GREEN, and FIRST-EVER SCORED** |
| (b) non-Docker sweep, 239 pkgs | 238 green, **1 RED** (`compressor`, pre-existing, master-red, stdlib-isolated) | 🔴 **DEPARTURE 3** |
| (b′) `-race ./internal/listener/` | `ok … 4.447s`, rc=0 | **GREEN** |
| (c) h2spec | `95 tests, 94 passed, 1 skipped, 0 failed` | **GREEN** |
| (d) fuzzers | 56 TARGETS / 48 FILES, **+0 / +0** | **GREEN** |
| (e) anchored panic gate | **0** over all four logs | **GREEN** |
| (f) `REVIEW.md` | absent; 37/139, none of 93-98 | 🔴 **STANDING DEPARTURE 2** |
| — `golangci-lint` | typecheck failure on every package, master control identical | 🔴 **DEPARTURE 1 (UNRUN)** |

---

## Task 20 — the close: `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`

⚠️ **PLAN Task 20 Step 6 says "squash … merge and push." THAT PART IS NOT THIS
TASK'S.** In this project subagents make **local commits only**; the controller
squashes, merges and pushes at the close. **Steps 1-5 were executed; nothing was
pushed, merged or squashed.**

### Step 1 — `STATE.md` §Current edited **IN PLACE**

Seven bullets replaced seven bullets at file lines **15-21**; **no block was
prepended**, which is the exact failure ADR-0288 exists to prevent. `wc -l` reads
**66** before and **66** after — the pointer moved without the file growing.
Lifecycle-state **3 -> DONE**; `next-skill` now reads **OPEN PHASE 99 BY SELF-PICK
AND WRITE ITS `BRAINSTORM.md`** (`superpowers:brainstorming`, BOOTSTRAP §5 state
0 -> 1).

`/usr/bin/grep -n 'next-skill:' docs/envoy-go/STATE.md` returns **exactly TWO**
lines, and **naming what each hit IS matters more than the count** (method note
72, the self-counting instrument):

| line | what it is |
|---|---|
| `:7` | the **ADR-0288 charter blockquote**, which spells the token while stating the rule. Pre-existing, unchanged by this close, and **not a pointer**. |
| `:18` | 🟢 **the ONE live pointer.** |

**ZERO stale pointer hits.** The defect ADR-0288 records — two fossilised blocks
answering and no live one — does not exist here.

### Step 2 — the eviction, by the LABEL-BOUND PAIR on BOTH files

🔴 **THE PLAN'S OWN STEP 2 WAS WRONG ABOUT THE TIE WIDTH, AND SO WAS THE ROUTER'S
METHOD NOTE 26 — IN THE SAME DIRECTION, FOR THE SAME REASON.** Both said the tail
was a **four-wide** tie. **Measured here:**

```
3 2026-09-09
1 2026-09-12
1 2026-09-16
```

⇒ **a THREE-WIDE tie at the tail.** The controller's own note (three-wide,
`09-09`) and `STATE.md`'s preceding preamble were right; the PLAN and note 26
were not. **The mechanism of their error is worth more than the correction:** both
restated the phase-98 **PLAN close's PRE-roll list** as though it were that
close's **POST-roll list**, silently dropping the entry that close was about to
promote. **A projection that forgets the entry your own close promotes is wrong by
exactly one, every time.** ⚠️ **The figure was inherited from neither; it was
measured.**

The date narrowed the field to three and **LIST POSITION picked the tail**:
`phase 97 (quic-chain-selection-order) SPEC done` (2026-09-09).

**The pair, and both controls — `/usr/bin/grep -F -c --`, run BEFORE and AFTER:**

| label | `STATE.md` before -> after | `STATE_HISTORY.md` before -> after | reading |
|---|---|---|---|
| `phase 97 (quic-chain-selection-order) SPEC done` — **the evictee** | **1 -> 0** | **0 -> 1** | ✅ the pair discriminates |
| `phase 97 (quic-chain-selection-order) TEASPOON done` — **fabricated NC** | 0 -> 0 | 0 -> 0 | ✅ reads zero in BOTH |
| `phase 93 (h2-local-reply-content-length) PLAN done` — **positive control** | 0 -> 0 | present, and **self-incremented by being named** | ✅ the form CAN match |

⚠️ **The bare forms answer nothing and were not relied on:** the strict
`^- \*\*prior active-phase:\*\* ` form reads **5** on `STATE.md` both before and
after, being invariant under *which* entry leaves.

### Step 3 — the archive, ONE INLINE LINE, PARENTHETICAL form

`STATE_HISTORY.md` **576 -> 578**: raw delta **`+2`** (a blank line **plus** the
entry line), exactly as chartered — not `+1`.

| form (house, **copied not paraphrased**) | before | after |
|---|---|---|
| strict `^- \*\*prior active-phase:\*\* ` | 163 | **163 — DELTA 0** ✅ |
| parenthetical `^- \*\*prior active-phase \(` | 75 | **76** |
| loose `^- \*\*prior active-phase` | 238 | **239** |

**163 + 76 = 239 exactly.** ⚠️ **The PLAN §9.2 baseline (74 / 237) is the
`bd303d87` reading and was already one behind at this tip** — the PLAN's own roll
moved it; re-derived here rather than inherited.

⚠️ **THE POSITIVE CONTROL'S LABEL IS NAMED IN THE ARCHIVE LINE AND ITS COUNT IS
NOT** — the house rule is *name the LABEL, never the COUNT*. **Naming it moved
that label's own count in the archive by one in this very commit**, which is
precisely why no figure may be written beside it. The control remains valid
because it was measured **before** the append. The line's first draft claimed the
control was "NAMED, never counted" **without naming it**; that was a false
statement about itself and was corrected before the commit.

### Step 4 — the §Recent preamble rolled

Rolled, **without spelling the evictee's label**, and it now records the measured
three-wide histogram, the two documents that projected four, and the **TWO-WIDE**
tie the next close will face (`09-20, 09-16, 09-12, 09-09, 09-09`).

### Step 5 — `next-prompt.txt` rolled, and AUDITED

**`git add -f` — the file is tracked but gitignored, and the ugrep wrapper in
every shell here is blind to it. Every check below used `/usr/bin/grep`.**

**The audit, run as method note 68 requires, then re-read as the next reader:**

| probe | hits | disposition |
|---|---|---|
| `YOUR STAGE` | **12** | **ALL TWELVE REWRITTEN OR VERIFIED.** Every one now addresses the **BRAINSTORM**: the header, the check-(1) comment, the sentinel-shape block, the six-gate scope line, method notes 1/10/24/47, the `PROPOSED`-guard heading, the port-band line, and the two standing "when you roll this file" rules, which are stage-agnostic by design. |
| own stage word `IMPL` | 24 lines | Each read in place. Those that were **imperatives addressed to a spent stage** are gone; those that remain are **historical narration** (`phase-96 IMPL 15500-15599`, `phase 95's IMPL landed as 0f3b98b3`, `the phase-97 IMPL repaired :136`) or **scope statements** (`AN IMPL RUNS ALL SIX; A BRAINSTORM RUNS NONE`). |
| `\b238\b` | 4 | All four are the **corrected** form (`239 … NOT the 238 this note carried`) or the archive guard's `238 -> 239`. |
| `123 / 123`, `99 + 24` | 1 / 0 | The surviving one is note 3e's own **correction history**; the live figures read **125 / 125** and **101 + 24**. |
| `19235`, `5993`, `574`, `parenthetical 74` | **0 each** | Every superseded count is gone. |
| `TWENTY-TASK`, `STARTING AT TASK 1`, `PLAN.md §6`, `Task 11/12/19`, `TASKS 1-10` | **0 each** | **No spine imperative survives.** |
| duplicate list numbers | **0** | Checked per section (`sort \| uniq -d`) — the phase-98 PLAN roll's own defect, not repeated. |

🔴 **THREE SPENT CLAIMS FOUND ONLY BY READING, NOT BY GREPPING A FIGURE** — each
would have read as live instruction to the next session:

1. *"check (1) is merely borrowing a voice from the open row 98"* — **row 98 is no
   longer open.** Replaced with the sharper true statement: with row 98 `done`,
   **checks (1) and (3) are BOTH silent, so two of the three sentinel checks are
   currently indistinguishable from broken ones** — which is the argument for the
   NCs, not against them.
2. *"Row 98 anticipates +0 stat NAMES"* — it **landed** +0. Re-tensed.
3. The Mechanics commit-locate forms still named **phase 98** as the phase to
   locate. Re-pointed at phase 99, keeping the closed row's form beside them.

**Method note 3e corrected in the same commit as the figures it names**
(`123 / 99+24` -> **125 / 101+24**), re-derived here with the PLAN's extractor
verbatim, scored on the SET and not the count:

```
imports=125  dirs=125
comm -23 (registered, no dir)  -> (empty)
comm -13 (dir, not registered) -> (empty)
split: driver=101  inputs=24
```

### Every figure quoted in this close, re-derived in the commit that ships it

| figure | value | how |
|---|---|---|
| `STATE.md` | **66** (in place) | `wc -l` |
| `STATE_HISTORY.md` | **576 -> 578** | `wc -l` |
| `next-prompt.txt` | **343 -> 341** | `wc -l` |
| `ROADMAP.md` | **248** lines / **130** rows / row 98 `done` at `:160` / NF **8** both forms | `wc -l`; the §2 awk |
| `DECISIONS.md` | **19359**, `^---$` **216**, `^## ADR-` **319**, bare `^## ` **327**, tail **ADR-0320** | `wc -l`; `/usr/bin/grep -cE`; tail-derived |
| house `PROPOSED` guard | 🟢 **ZERO — DISARMED, and the zero is CORRECT** | `^> \*\*STATUS: PROPOSED` |
| ADR-0231 decoy | still hits at `:14866` | `^\*\*Status:\*\* PROPOSED` |
| `BEHAVIOR_CONTRACT.md` | **5996** | `wc -l` |
| phase dirs | **139** | `ls -d … \| wc -l` |
| fixtures | **125**, tail `0123-…` | `ls -d test/fixtures/*/ \| wc -l` |
| non-Docker packages | **239** (raw `go list` **241**) | `go list ./... \| grep -vE …` |
| fuzzers | **56 targets / 48 files** | `git grep -c '^func Fuzz'` |
| production set | `manager.go` **`4	9`** · `chainmatch.go` **`3	3`** · `types.go` **`3	3`** · `quic.go` **absent** | `git diff master --numstat` |
| tree paths touched | **14** at the Task 19 tip, **17** with this close | `git diff master --numstat \| wc -l` |
| top-level roster | **315** over the five-selector / seven-package run; `=== RUN` **398**; rc **0** | the Task 1 pipeline, re-run |
| `internal/listener` subtree | **184** | `git grep -hoE '^func (Test\|Fuzz)…'` |
| six sentinel windows | md5s **byte-identical** to the PLAN close | `sed -n 'Np' \| md5sum` |

🔴 **A THIRD READING OF THE `309 -> 315` FIGURE, AND THE FIRST CORRECT ONE.** The
brief called it repo-wide; Task 19 called it repo-wide after refuting an earlier
"`internal/listener`" reading. **It is neither.** Repo-wide `^func Test` reads
**5276**; the `internal/listener` subtree reads **184**. `315` is the **distinct
top-level name roster of ONE `go test -count=1 -v` run over five selectors and
seven packages** (`./cmd/envoy-go/... ./internal/admin/... ./internal/boot/...
./internal/listener/... ./validate/...`). **NAME THE SELECTOR OR THE ROSTER FIGURE
IS MEANINGLESS** — and a correction is not a measurement until it is run.

### The sentinel, re-run at THIS tip (post-flip), with `/usr/bin/grep`

(1) **SILENT** · (2) **SIX** at `:208 :214 :220 :230 :236 :244` · (3) **SILENT** ·
NC-A **ONE** (`NOT DONE: row 62`; substitution inspected first,
`NC LANDED? [ in-progress ]`) · NC-B **ONE**
(`GATE FAIL: examined 130 data rows, expected 129`) · NC-C **FIRED** (residual 0) ·
NC-D **96 / 68** under `--` · check-(2) positive control **6 substitutions
ASSERTED, residual 0** · escape-aware malformed set exactly **{57, 69}**.
⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS NOT CREATED** (verified absent at the
git root and in this worktree). ⚠️ **THE MARGIN IS STILL ONE, and the six
deferred-candidate windows were READ and NOT EDITED — `ROADMAP.md` is `1	1` for
this branch, entirely Task 18's row flip.**

### The three standing departures, carried forward unchanged

1. **`golangci-lint` CANNOT RUN** — v1.64.8 (go1.26.2) vs installed go1.27.1,
   `export data version 4 > max 2 (typecheck)` on the `sync/atomic` stdlib import,
   **every** package; untouched `master` fails byte-identically.
   `revive`/`errcheck`/`unused`/`staticcheck` UNRUN for phase 98.
2. **No `REVIEW.md`** — 37 of 139 phase dirs carry one; none of 93-98.
3. 🔴 **Gate (b) RED at `internal/filter/http/compressor`** —
   `TestEncodeData_LevelMapping_DifferentGzippedSizes`, `both = 2121`.
   **Re-confirmed independently at this task:** rc=1, identical figure, under
   `go version go1.27.1 linux/amd64`. Pre-existing, toolchain-driven, and now
   **BANKED as a candidate row in `next-prompt.txt`** together with the lint
   breakage, since both are the same class: *the toolchain moved under a gate*.

### Sub-step count

PLAN charters **6 steps**; **Step 6 is the controller's, not this task's**, so
**5 were executed**, as **10 sub-steps**: (1) measure the histogram, pair and
guard triple; (2) re-derive every figure quoted; (3) `STATE.md` §Current in place;
(4) `STATE.md` §Recent + preamble; (5) archive append; (6) post-roll pair and
triple; (7) roll `next-prompt.txt`; (8) audit `next-prompt.txt` and fix the three
spent claims; (9) this section; (10) commit. **At `BOOTSTRAP_PROMPT.md` §6.1's
~10 mid-execution trigger, not past it — stated rather than rounded down**
(method note 66).
